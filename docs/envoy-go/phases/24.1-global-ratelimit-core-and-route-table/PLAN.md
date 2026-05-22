# Phase 24.1 — `envoy.filters.http.ratelimit` (core decision path + route-table exposure) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the core decision path of `envoy.extensions.filters.http.ratelimit.v3.RateLimit` (the SEVENTEENTH §9 production HTTP filter, GLOBAL rate limit) — a working, differential-green, both-sides-byte-exact filter on the common path: the NEW `internal/filter/http/ratelimit/` package + DELTA-1 `RateLimitClient` + DELTA-2 route-table exposure + the descriptor engine for the 5 CORE actions + `ShouldRateLimit` dispatch + OK/OVER_LIMIT/error dispositions + the cluster-scoped 4-counter stat surface + the FULL §5 PARSE-REJECT roster + the `0032`/`0033` differential fixtures.

**Architecture:** A NEW filter package (`internal/filter/http/ratelimit/`, single-token Go package `ratelimit`) owns the descriptor-action engine + dispositions + cluster-scoped stats, consuming TWO framework deltas: DELTA 1 = a NEW `internal/grpcclient/ratelimit_client.go` typed wrapper (the THIRD ADR-0158 two-tier wrapper; `ShouldRateLimit` is UNARY ⇒ clones `AuthClient` verbatim); DELTA 2 = a NEW chain-seeded HCM route-table exposure (parse + retain the matched Route's `RouteAction.rate_limits[]` + the VirtualHost's `rate_limits[]` as RAW `[]*routev3.RateLimit`, seed onto the per-stream `FilterChain` at HCM dispatch per the ADR-0165 set-once pattern, expose via a NEW `RouteRateLimits()`/`VirtualHostRateLimits()` `DecoderFilterCallbacks` accessor pair). The §5.1 filter-config PARSE-REJECT arms live in `buildCompiledConfig`; the §5.2 route/vhost-strict arms (`disable_key`/`extension`/`dynamic_metadata`) are enforced at HCM-parse-time via an exported `ratelimit.ValidateRouteRateLimits` validator the HCM parser calls (the existing `hcm → cors`/`hcm → filter_http` coupling precedent; NO import cycle — `internal/filter/http` does not import `internal/filter/hcm`). The X-RateLimit DRAFT_VERSION_03 headers, the remaining 5 actions, `RateLimitPerRoute`, the `stage` multi-stage path, and the Axis-B `vh_rate_limits` table are DEFERRED to 24.2.

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 (proto pin per ADR-0008/ADR-0051; `envoy/extensions/filters/http/ratelimit/v3` for the `RateLimit` filter proto; `envoy/service/ratelimit/v3` for `RateLimitServiceClient`/`RegisterRateLimitServiceServer`/`RateLimitRequest`/`RateLimitResponse` — verified present in v1.32.4, no codegen; `envoy/config/route/v3` for the `RateLimit` policy + `RateLimit_Action` oneof; `envoy/extensions/common/ratelimit/v3` for `RateLimitDescriptor`). Reuses `internal/grpcclient` (`Dialer` + `AuthClient` clone), `internal/cluster` (cluster-load gates), `internal/stats` (`Registry.NewCounterIfAbsent`), `internal/filter/http` (callbacks + chain), `internal/filter/hcm` (route-table). Shared fake gRPC `RateLimitService` for the deterministic cross-side differential (the `test/helpers/extauthzgrpc/` `0021` precedent). golangci-lint 1.64.8 (ADR-0009). Reference Envoy `v1.37.2` (ADR-0008/ENVOY_TARGET.md). Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext synthetic backend.

---

## Scope check — why phase 24.1 ships as one sub-phase row

Phase 24 was SPLIT at PLAN time (this is the FIRST PLAN-time ADR-0045 split application; ADR-0201) into `24.1-global-ratelimit-core-and-route-table` (this PLAN) + `24.2-global-ratelimit-perroute-and-headers`. The split FIRED at the parent-SPEC §16 core-path / remaining-surface axis: the parent post-empirical LoC envelope (~1900–2700) is above the ~1500 split-gate. This 24.1 PLAN is for the foundational sub-phase ONLY; no further nested split per ADR-0106 (sub-sub-phase splits are structurally awkward — matches the phase-18.1 + 19.1 + 22.1 sub-phase PLAN precedent). The 24.2 sibling SPEC at `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/SPEC.md` documents the deferred surface; its PLAN is authored at 24.2's lifecycle-state 2 after 24.1 phase-done.

The PLAN-time re-evaluation per `superpowers:writing-plans` GATE + SKILL_ROUTING state-2 GATE + ADR-0045 §6 confirms single-sub-phase landing:

- **Task count: 12** (Pre-Task 0 + Tasks 1–12) — comfortably under the ADR-0045 25-task split-gate.
- **PLAN.md size** sits below the ~1500-LoC PLAN-size soft-gate. The IMPL LoC envelope for 24.1 (the §4 24.1-slice file roster: ~1100–1700 prod + ~600–900 test + ~400–650 fixtures/helper) is sized per the 24.1 SPEC §4 roster; the PLAN gate is about PLAN.md size, not IMPL LoC.
- **24.1 ships as the single sub-phase row it is** — no further nested split. The 24.1 phase-done squash-merge **CLOSES row 24.1** (`in-progress → done`); the parent row `24` STAYS `in-progress` until 24.2 IMPL phase-done per the sub-row rollup discipline (ADR-0106 + phase-18.1/19.1/22.1 precedent).

**Net change estimate for 24.1** (mirroring the phase-09..23 + 22.1 PLAN component-table convention):

- `internal/filter/http/ratelimit/doc.go` ~25 (Task 1)
- `internal/filter/http/ratelimit/ratelimit.go` ~120–180 cumulative (Task 1 skeleton ~80; Task 7 full `New` body)
- `internal/filter/http/ratelimit/compiled_config.go` ~250–350 (Task 3 — `compiledConfig` + `buildCompiledConfig` + §5.1 + §5.2 wording constants + `ValidateRouteRateLimits` export)
- `internal/filter/http/ratelimit/descriptors.go` ~200–300 (Task 6 — 5 CORE actions + empty-action-drop + Axis-A early-return + OVERRIDE-default vhost walk)
- `internal/filter/http/ratelimit/decode_headers.go` ~120–180 (Task 7 — descriptor build + async `ShouldRateLimit` + `StopIteration` + OnDestroy cancel)
- `internal/filter/http/ratelimit/dispositions.go` ~120–180 (Task 7 — OK/OVER_LIMIT/error + §4.7 byte-shape; X-RateLimit stubbed)
- `internal/filter/http/ratelimit/stats.go` ~40–60 (Task 4 — cluster-scoped `filterStats`)
- `internal/grpcclient/ratelimit_client.go` ~60–90 (Task 2 — DELTA 1)
- `internal/filter/hcm/{config.go,route.go,chain.go}` + `internal/filter/http/{callbacks.go,chain.go}` ~250–400 (Task 5 — DELTA 2)
- `cmd/envoy-go/main.go` ~+2 (Task 7 — boot-registration oauth2↔rbac; 19 filters)
- `internal/filter/http/ratelimit/*_test.go` + `ratelimit_client_test.go` + HCM DELTA-2 tests ~600–900 (across tasks)
- `internal/filter/http/ratelimit/fuzz_test.go` + corpus ~80–130 (Task 8 — 33rd fuzzer)
- `test/helpers/ratelimitgrpc/` ~150–250 (Task 9 — shared fake `RateLimitService`)
- `test/differential/fixture/fixture.go` + `test/differential/runner_test.go` ~+30 (Task 9 — `HTTPGlobalRateLimitGRPC BackendKind = 24` + switch-case + blank import)
- `test/fixtures/0032-http-ratelimit/` ~400–600 (Task 10 — README + envoy.yaml + envoy-go.yaml + expectations.yaml + driver.go)
- `test/fixtures/0033-http-ratelimit-boot-reject/` ~150–250 (Task 11)
- `docs/envoy-go/DECISIONS.md` — ADR-0200 (Task 3) + ADR-0198 (Task 5) + ADR-0197[core] (Task 7) §Decision + §Consequences bodies; CONDITIONAL ADR-0202 only if the DELTA-2 escape-valve fires
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` ~+150–250 (Task 12 — partial bundle: NEW subsection core + 3 departure records 15→18 + stat 110→114)
- `docs/envoy-go/{ROADMAP.md,STATE.md}` (Task 12)
- `docs/envoy-go/phases/24.1-.../{PROGRESS.md,REVIEW.md}` (Pre-Task 0 + Task 12)

---

## ADRs introduced/landed by this plan

The 4 phase-24 §Context drafts (ADR-0197..0200) are already anchored at the parent-SPEC commit per ADR-0044. 24.1 lands the §Decision + §Consequences bodies for the THREE 24.1-mapped ADRs at their materializing Tasks:

| ADR | Subject | §Decision + §Consequences body lands at |
|---|---|---|
| **ADR-0197 (CORE slice)** | filter package shape + 5-core-action engine + `RateLimitClient` + OK/OVER_LIMIT/error dispositions + OVER_LIMIT/error byte-shape + cluster-scoped 4-counter stat surface + deterministic shared-fake differential. (The X-RateLimit-header + remaining-actions slice lands at 24.2.) | **Task 7** (decode/dispatch/dispositions + full `New` + boot-reg — completes the core decision path) |
| **ADR-0198 (FULL)** | DELTA-2 HCM route-table `rate_limits` exposure — parse/retain RAW `[]*routev3.RateLimit`, seed onto the chain (ADR-0165 set-once), `RouteRateLimits()`/`VirtualHostRateLimits()` accessor pair | **Task 5** (DELTA-2 route-table parse/seed + accessor pair) |
| **ADR-0200 (FULL)** | RTDS/action-deferral PARSE-REJECTs — route `disable_key` non-empty; `extension` action; deprecated `dynamic_metadata` action; the 3 envoy-go-strict departures (15→18) | **Task 3** (`compiled_config` + §5 PARSE-REJECT roster) |

**ADR-0199** (`RateLimitPerRoute` 10th canonical + ADR-0125 9→10) and the **X-RateLimit/remaining-actions slice of ADR-0197** land at **24.2** — the canonical-per-route roster STAYS 9 through 24.1.

**Escape-valve reserve: ADR-0202** (next-free). The SPEC §12 item-1 highest-risk byte-confirmation — the exact DELTA-2 chain-seed type + accessor return-type — settles at Task 5. PLAN hypothesis (per the parent §10-C D-style hypothesis, re-mapped): ADR-0202 stays **UNCONSUMED** at 24.1 phase-done. It FIRES only if the Task-5 raw-`[]*routev3.RateLimit` seeding shape must diverge from the ADR-0165 `DownstreamRemoteAddr` set-once primitive (e.g., the seed needs pre-compilation or a non-proto carrier type). If it fires, ADR-0202 §Context + §Decision + §Consequences all land at the Task-5 commit per ADR-0044.

---

## Planner-time deferred-decision resolution (the PLAN author settles these; recorded in PROGRESS preamble at Pre-Task 0)

The parent SPEC §12 sub-pin-level byte-confirmations + the PLAN-process decisions, resolved here so the IMPL subagents inherit them:

- **D-RL1 (DELTA-2 chain-seed type + accessor return-type; parent §12 item 1 — HIGHEST RISK).** **RECOMMENDED:** seed the **RAW `[]*routev3.RateLimit`** proto slices (matched route's `RouteAction.GetRateLimits()` + the vhost's `GetRateLimits()`) onto the per-stream `FilterChain`, exposed via `RouteRateLimits() []*routev3.RateLimit` + `VirtualHostRateLimits() []*routev3.RateLimit`. Rationale: ADR-0198 §Context narrow-exposure/YAGNI — the framework surfaces raw policy; the filter owns ALL descriptor interpretation (the §4 engine). The seed plumbing mirrors the ADR-0165 `DownstreamRemoteAddr`/`DownstreamLocalAddr` set-once-by-dispatch primitives (chain field + setter + accessor; single-dispatch-goroutine invariant per ADR-0071). Task 5's FIRST action confirms this against the ADR-0165 plumbing; if the raw-proto seed proves insufficient, fire ADR-0202.
- **D-RL2 (§5.2 route/vhost PARSE-REJECT placement).** The route/vhost-level strict rejects (`disable_key != ""`; `extension` action; deprecated `dynamic_metadata` action) fire at **HCM-parse-time** (boot). **RECOMMENDED:** the `ratelimit` package EXPORTS `ValidateRouteRateLimits(rls []*routev3.RateLimit) error` (defined in `compiled_config.go`, Task 3, single-sourcing the byte-stable wording constants per ADR-0080); the HCM parser (`internal/filter/hcm/config.go`) CALLS it during `buildRouteTable` + vhost parse (Task 5). This avoids duplicating the byte-stable wording in `hcm` and reuses the existing `hcm → cors`/`hcm → filter_http` import coupling (no cycle: `internal/filter/http` does not import `internal/filter/hcm`). The §5.1 FILTER-config arms (empty `domain`; missing `rate_limit_service`; `stage > 10`; bad `request_type`; >10 `response_headers_to_add`; cluster-load) stay in `buildCompiledConfig` (Task 3).
- **D-RL3 (PARSE-REJECT byte-stable wording; parent §12 item 3).** The §5.1 + §5.2 wording constants are finalized at Task 3 verbatim from the SPEC §5.1/§5.2 tables, asserted by `TestParseRejectConstants_ByteStable` per ADR-0080.
- **D-RL4 (boot-reject common stderr substring; parent §12 item 4).** The `0033` shared substring is the `domain`-empty arm. Both upstream (PGV/`ASSERT`) and envoy-go reject at boot; the fixture pins the common distinctive substring (finalized at Task 11 against the captured both-sides stderr). Reuses the 22.1 `BootRejectFixture` harness interface (`test/differential/harness.go:340`).
- **D-RL5 (proto-number-faithful fake encoding; parent §12 items 6+7).** The shared fake `RateLimitService` (`test/helpers/ratelimitgrpc/`, Task 9) emits `RateLimitResponse` by proto field NUMBER + omits unset optionals (`raw_body`/`dynamic_metadata`/`quota`/per-descriptor `hits_addend` `UInt64Value`) per AMEND-6. The fake's deterministic script keys on the canonical descriptor string (entries in action-list order). Cross-side byte-exactness depends on this.
- **D-RL6 (24.1 descriptor source).** 24.1's descriptor source is the route/vhost `rate_limits` surfaced by DELTA 2 (the only descriptor source at 24.1 — `RateLimitPerRoute` Axis-A embedded policy + the per-route `domain` override land at 24.2). The engine walks the route policy + (under the OVERRIDE default only) the vhost policy; the full Axis-B `vh_rate_limits` table + the `stage` multi-stage bucketing + legacy `include_vh_rate_limits` land at 24.2. 24.1 evaluates the filter's default stage-0 bucket only (still PARSE-REJECTs `stage > 10`).
- **D-RL7 (X-RateLimit deferral).** 24.1 parses `enable_x_ratelimit_headers` into `compiledConfig` but does NOT emit the headers (no `encode.go`/`headers.go` at 24.1); `dispositions.go` leaves the X-RateLimit injection point STUBBED with a forward-pointer to 24.2.
- **D-P1 (task numbering).** Pre-Task 0 (PROGRESS preamble + preconditions) is the ritual prefix; the functional tasks are Tasks 1–12. Each Task maps 1:1 to a PROGRESS.md entry.
- **D-P2 (subagent dispatch).** Per `superpowers:subagent-driven-development`, each Task is dispatched to a fresh `general-purpose` subagent with the Task's dispatch outline + a two-stage review between Tasks.
- **D-P3 (PROGRESS discipline).** Each Task appends a PROGRESS.md entry quoting the six-gate-relevant command outputs verbatim + the commit SHA.

---

## Task graph (sequential vs parallelizable per D-P2)

- **Pre-Task 0** (PROGRESS preamble + precondition verification) — sequential prerequisite for everything.
- **Task 1** (package skeleton) — sequential prerequisite for Tasks 2–12.
- **Tasks 2, 3, 4** — **PARALLELIZABLE** (3-way) after Task 1 (file-disjoint; depend only on Task 1's skeleton):
  - **Task 2** — DELTA-1 `RateLimitClient` (`internal/grpcclient/`).
  - **Task 3** — `compiled_config.go` + §5 PARSE-REJECT roster + `ValidateRouteRateLimits` export + **ADR-0200**.
  - **Task 4** — `stats.go` cluster-scoped 4-counter surface.
- **Task 5** (DELTA-2 route-table exposure + **ADR-0198**) — depends on Task 3 (`ValidateRouteRateLimits`). HIGHEST-RISK; ADR-0202 escape-valve.
- **Task 6** (`descriptors.go` 5-core-action engine) — depends on Task 3 (compile/validate). PARALLELIZABLE with Task 5 (unit-tested over raw policy inputs; integration is Task 7).
- **Task 7** (`decode_headers.go` + `dispositions.go` + full `New` + boot-reg + **ADR-0197[core]**) — depends on Tasks 2, 3, 4, 5, 6.
- **Tasks 8, 9** — **PARALLELIZABLE** (2-way) after Task 7:
  - **Task 8** — 33rd fuzzer `FuzzRateLimitConfigParse` (depends on Tasks 3 + 6 for the compile/engine surface; can start once those land but green-run needs Task 7's wired `New`).
  - **Task 9** — shared fake `test/helpers/ratelimitgrpc/` + `HTTPGlobalRateLimitGRPC BackendKind = 24` + runner wiring.
- **Tasks 10, 11** — **PARALLELIZABLE** (2-way) after Task 9:
  - **Task 10** — fixture `0032-http-ratelimit` (a/b/c/d-core/e/h).
  - **Task 11** — fixture `0033-http-ratelimit-boot-reject` (`domain` empty; reuse `BootRejectFixture`).
- **Task 12** (atomic landing — BEHAVIOR_CONTRACT partial bundle + STATE + ROADMAP + REVIEW) — depends on everything.

**Sequential bottlenecks:** Pre-Task 0 → Task 1 → {2,3,4}; Task 3 → Task 5; Task 3 → Task 6; {2,3,4,5,6} → Task 7; Task 7 → {8,9}; Task 9 → {10,11}; {8,10,11} → Task 12.

---

## Execution preconditions

The IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (project memory `feedback_git_worktrees.md`):

```bash
git worktree add /home/esa/git/envoy-go/.worktrees/phase-24.1-global-ratelimit-core-and-route-table-impl \
                 -b phase-24.1-global-ratelimit-core-and-route-table-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-24.1-global-ratelimit-core-and-route-table-impl
```

where `<PLAN-tip-SHA>` is the master tip after this PLAN.md squash-merge + its SHA-fill follow-up.

The 12 preconditions verified at Pre-Task 0 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-24.1-global-ratelimit-core-and-route-table-impl`.
2. **Master tail.** `git log --oneline master | head -6` shows this 24.1 PLAN.md squash + SHA-fill follow-up at the head, with the split squash `bf868a6` + `5d0c601` in the recent history. If not, `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` ≥ `go1.26.2`; `golangci-lint version` = `1.64.8` (ADR-0009); `docker version` reports client + server.
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `201` (ADR-0201 at master tip). Higher → another phase landed; re-verify next-free numbers.
5. **ADR §Context drafts present.** `grep -cE '^## ADR-0197' docs/envoy-go/DECISIONS.md` returns `1` (§Context anchored at the parent SPEC commit). Same for ADR-0198, ADR-0199, ADR-0200. `grep -cE '^## ADR-0202' docs/envoy-go/DECISIONS.md` returns `0` (ADR-0202 stays unconsumed unless the Task-5 escape-valve fires).
6. **NO 24.2-bound code at this worktree.** Per BOOTSTRAP §4.1 invariant 2 — 24.2 surfaces (`encode.go`/`headers.go` X-RateLimit emission; `compiled_perroute.go` `RateLimitPerRoute`; the remaining 5 actions; the `stage` multi-stage path; the Axis-B `vh_rate_limits` table; `0032` scenarios f/g) MUST NOT land at 24.1. If a 24.2-surface partial has been started, halt + escalate.
7. **Parent SPEC + 24.1 SPEC SHAs.** `git log -1 --format=%H -- docs/envoy-go/phases/24-http-filter-global-ratelimit/SPEC.md` + `.../24.1-global-ratelimit-core-and-route-table/SPEC.md` return the split commit (or descendant). If different, re-read.
8. **Pristine tree.** `git status --porcelain` returns empty.
9. **Pre-existing suite green.** `go test -count=1 -short ./...` clean.
10. **Pre-existing differential baseline green.** The 33 fixture directories 0000-0031 PASS (lua 0026-0029 + multi-listener combined runs may hit the documented `freeTCPPort` flake per 22.2 REVIEW §7.4 — re-run in isolation). 24.1 adds the 24th `BackendKind` + the 34th/35th-by-directory fixtures (`0032` + `0033`).
11. **Fuzzer baseline.** `find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l` returns `32`. 24.1 adds the 33rd (`FuzzRateLimitConfigParse`).
12. **NEW 24.1 surfaces absent.** `test ! -d internal/filter/http/ratelimit && test ! -f internal/grpcclient/ratelimit_client.go && test ! -d test/helpers/ratelimitgrpc && test ! -d test/fixtures/0032-http-ratelimit && ! grep -q 'HTTPGlobalRateLimitGRPC' test/differential/fixture/fixture.go && echo "ok: phase-24.1-new-surfaces absent"` returns success.

If all 12 pass, proceed to Pre-Task 0 + Task 1.

---

## Pre-Task 0: PROGRESS.md preamble + 12-precondition verification

**Files:**
- Create: `docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md`

This pre-task verifies the `## Execution preconditions` block and creates PROGRESS.md so subsequent tasks have an append target. Records the 3-ADR landing map + the D-RL1..D-RL7 + D-P1..D-P3 resolutions verbatim.

**Precondition:** worktree exists at `phase-24.1-...-impl`; all 12 preconditions report green.
**Acceptance:** all 12 preconditions green; PROGRESS.md preamble committed.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` and confirm expected output.
- [ ] **Step 2: Author `PROGRESS.md` preamble** — (a) 12-precondition verification (verbatim outputs); (b) the 3-ADR landing table + CONDITIONAL ADR-0202 reproduced; (c) D-RL1..D-RL7 + D-P1..D-P3 reproduced verbatim; (d) a Pre-Task 0 entry slot for the commit-SHA.
- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Pre-Task 0: PROGRESS.md preamble + 12-precondition verification"
```

---

## Task 1: Package skeleton (`internal/filter/http/ratelimit/`)

**Files:**
- Create: `internal/filter/http/ratelimit/doc.go` (~25 LoC)
- Create: `internal/filter/http/ratelimit/ratelimit.go` (skeleton ~80 LoC; `TypeURL` + `New` stub + filter struct + interface assertions; full `New` at Task 7)
- Create: `internal/filter/http/ratelimit/ratelimit_test.go` (skeleton tests)
- Append: PROGRESS.md (Task 1 entry per D-P3)

Lands the package directory skeleton. **Sequential prerequisite for Tasks 2–12.** Boot-registration is DEFERRED to Task 7 (do NOT wire a non-functional `New` into boot).

**Precondition:** Pre-Task 0 complete.
**Acceptance:** `go build ./internal/filter/http/ratelimit/...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/ratelimit/...` clean; `go test -count=1 ./internal/filter/http/ratelimit/...` skeleton tests pass.

**Subagent dispatch outline** (D-P2 `general-purpose`):
> Author the Task 1 skeleton at the 3 listed paths per 24.1 SPEC §4 + parent SPEC §6 production signatures. Single-token Go package identifier `ratelimit` (matches `cors`/`fault`/`rbac` precedent). `New` returns `nil, errors.New("ratelimit: not yet implemented")` at skeleton (full body at Task 7). Declare the filter struct with compile-time interface assertions. Commit per the Step 5 template; quote build/vet/lint/test outputs in PROGRESS Task 1 entry.

- [ ] **Step 1: Write the failing test** in `ratelimit_test.go`:

```go
func TestTypeURL(t *testing.T) {
	want := "type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}

func TestNew_NotYetImplemented(t *testing.T) {
	_, err := New(http.FilterFactoryContext{})
	if err == nil {
		t.Fatal("New: want error at skeleton stage, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./internal/filter/http/ratelimit/ -run 'TestTypeURL|TestNew_NotYetImplemented' -v`. Expected: FAIL (package does not compile / symbols undefined).
- [ ] **Step 3: Author `doc.go`** — package overview; the SEVENTEENTH §9 row (external-gRPC global rate limit); cross-refs to ADR-0197/0198/0200 + AMEND-1/3/6/9/10; the 24.1/24.2 split boundary.
- [ ] **Step 4: Author `ratelimit.go` SKELETON** — `TypeURL` const; `filterName = "envoy.filters.http.ratelimit"`; the filter struct (per-stream state: `cc *compiledConfig`, `dcb`/`ecb` callbacks, `client *grpcclient.RateLimitClient`, `callCancel context.CancelFunc`); `New(ctx http.FilterFactoryContext) (http.FilterInstanceFactory, error)` stub returning the not-yet-implemented error; compile-time assertions (`var _ http.StreamDecoderFilter = (*filter)(nil)`; encoder assertion only if the encode-side is wired at 24.1 — at 24.1 the filter is decode-side + a no-op encode hook stub for the 24.2 X-RateLimit injection point, so declare `var _ http.StreamEncoderFilter = (*filter)(nil)` with stub `EncodeHeaders` returning Continue).
- [ ] **Step 5: Run test to verify it passes** — `go test ./internal/filter/http/ratelimit/ -run 'TestTypeURL|TestNew_NotYetImplemented' -v`. Expected: PASS.
- [ ] **Step 6: Verify gates + append PROGRESS + commit**

```bash
go build ./internal/filter/http/ratelimit/... && go vet ./... && golangci-lint run ./internal/filter/http/ratelimit/...
git add internal/filter/http/ratelimit/ docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Task 1: ratelimit package skeleton (TypeURL + New stub + filter struct)"
```

---

## Task 2: DELTA-1 — `internal/grpcclient/ratelimit_client.go` `RateLimitClient`

**Files:**
- Create: `internal/grpcclient/ratelimit_client.go` (~60–90 LoC)
- Create: `internal/grpcclient/ratelimit_client_test.go` (~120–200 LoC)
- Append: PROGRESS.md (Task 2 entry)

DELTA 1 (parent §3.1; ADR-0197). The THIRD ADR-0158 two-tier typed wrapper. `ShouldRateLimit` is UNARY ⇒ clone `AuthClient` (`grpcclient.go:178/212/231`) verbatim, NOT the bidi `ProcessorClient`. NO `Dialer` API change. **PARALLELIZABLE with Tasks 3 + 4.**

**Precondition:** Task 1 complete.
**Acceptance:** `go build ./internal/grpcclient/...` clean; `go vet ./...` + `golangci-lint run` clean; `go test -race -count=1 ./internal/grpcclient/ -run 'TestRateLimitClient'` clean.

**Subagent dispatch outline:**
> Clone the `AuthClient` shape (`internal/grpcclient/grpcclient.go:157` struct / `:178` `NewAuthClient` / `:212` `Check` / `:231` `Close`) into `ratelimit_client.go`, swapping the stub to `ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"` → `ratelimitv3.NewRateLimitServiceClient(conn)` (verified present in v1.32.4). Mirror the `AuthClient` test shape (`grpcclient` test for a unary client: per-call timeout, verbatim error propagation, idempotent `sync.Once` Close). The per-stream OnDestroy cancellation lives at the FILTER layer (Task 7), NOT in the wrapper.

- [ ] **Step 1: Write the failing test** in `ratelimit_client_test.go` — `TestRateLimitClient_ShouldRateLimit_Unary` (in-process fake `RegisterRateLimitServiceServer` returning a canned `RateLimitResponse`; assert the wrapper round-trips the unary call), `TestRateLimitClient_Timeout` (per-call `context.WithTimeout` when `timeout > 0`), `TestRateLimitClient_ErrorPropagation` (transport error returned verbatim), `TestRateLimitClient_Close_Idempotent` (double-Close is safe via `sync.Once`). Mirror the `AuthClient` test fixture for the `Dialer` setup.
- [ ] **Step 2: Run test to verify it fails** — `go test ./internal/grpcclient/ -run 'TestRateLimitClient' -v`. Expected: FAIL (symbols undefined).
- [ ] **Step 3: Author `ratelimit_client.go`** — `NewRateLimitClient(d *Dialer, clusterName string, timeout time.Duration) (*RateLimitClient, error)` → `d.DialContext(...)` + `ratelimitv3.NewRateLimitServiceClient(conn)`; `(*RateLimitClient).ShouldRateLimit(ctx context.Context, req *ratelimitv3.RateLimitRequest) (*ratelimitv3.RateLimitResponse, error)` (per-call `context.WithTimeout` when `timeout > 0`; propagate transport errors verbatim); `Close() error` (`sync.Once`-guarded). NO `Dialer` API change.
- [ ] **Step 4: Run test to verify it passes** — `go test -race ./internal/grpcclient/ -run 'TestRateLimitClient' -v`. Expected: PASS.
- [ ] **Step 5: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/grpcclient/...
git add internal/grpcclient/ratelimit_client.go internal/grpcclient/ratelimit_client_test.go docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Task 2: DELTA-1 RateLimitClient (ADR-0158 third typed wrapper; unary AuthClient clone)"
```

---

## Task 3: `compiled_config.go` + §5 PARSE-REJECT roster + `ValidateRouteRateLimits` + ADR-0200

**Files:**
- Create: `internal/filter/http/ratelimit/compiled_config.go` (~250–350 LoC)
- Create: `internal/filter/http/ratelimit/compiled_config_test.go` (~350–500 LoC; table-driven §5.1 + §5.2 + `TestParseRejectConstants_ByteStable` + valid-config rows)
- Modify: `docs/envoy-go/DECISIONS.md` — ADR-0200 §Decision + §Consequences body (per ADR-0044 ADR-on-impl)
- Append: PROGRESS.md (Task 3 entry)

Lands `compiledConfig` (parent §6.1 + AMEND-3 13-field roster + defaults/clamps) + `buildCompiledConfig` + the FULL §5 PARSE-REJECT roster (both §5.1 filter-config arms + the §5.2 byte-stable wording constants) + the EXPORTED `ValidateRouteRateLimits` (D-RL2 — consumed by HCM at Task 5). **ADR-0200 §Decision + §Consequences body lands at this Task's commit.** **PARALLELIZABLE with Tasks 2 + 4.**

**Precondition:** Task 1 complete.
**Acceptance:** `go build` + `go vet` + `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/ratelimit/ -run 'TestBuildCompiledConfig|TestParseRejectConstants|TestValidateRouteRateLimits'` clean; ADR-0200 §Decision + §Consequences bodies present (`grep -A2 '^### Decision' DECISIONS.md` under ADR-0200 is non-placeholder).

**Subagent dispatch outline:**
> Author `compiledConfig` per parent SPEC §6.1 (the 13-field AMEND-3 roster: `domain`, `stage`, `requestType`, `timeout` 20ms, `failureModeDeny` false⇒fail-open, `rateLimitedAsResourceExhausted`, `rlsClusterName`, `enableXRateLimitHeaders` [parsed but NOT emitted at 24.1 per D-RL7], `disableXEnvoyRateLimitedHeader`, `rateLimitedStatus` 429/<400⇒429, `statusOnError` 500/clamp[100,511], `statPrefix`, `responseHeadersToAdd` max 10). `buildCompiledConfig` runs the §5.1 arms (verbatim SPEC §5.1 wording) + the cluster-load gates (REUSE 1; the ext_authz `buildGRPCCheckFn` precedent: google_grpc-arm reject, non-empty `envoy_grpc.cluster_name`, cluster-manager `Get` + `UseH2` gates). Define ALL byte-stable wording constants (§5.1 + §5.2) as package consts. Export `ValidateRouteRateLimits(rls []*routev3.RateLimit) error` running the §5.2 arms (disable_key non-empty; extension action; dynamic_metadata action) for HCM to call at Task 5. Per ADR-0085 nil-tolerance: guard `ctx.Stats != nil`. Then write the ADR-0200 §Decision + §Consequences body in DECISIONS.md replacing the placeholder.

- [ ] **Step 1: Write the failing tests** in `compiled_config_test.go` — table-driven `TestBuildCompiledConfig_PARSE_REJECT` (one row per §5.1 arm with byte-exact `wantErrSubstring`: `"ratelimit: domain is required"`; `"ratelimit: rate_limit_service is required"`; `"ratelimit: stage must be <= 10"`; `"ratelimit: request_type must be one of internal|external|both"`; `"ratelimit: response_headers_to_add accepts at most 10 items"`; the cluster-load wording per the REUSE-1 precedent) + 4-6 valid-config rows (defaults/clamps: timeout 20ms, status_on_error 500, rate_limited_status 429, request_type empty⇒both); `TestValidateRouteRateLimits` (one row per §5.2 arm: `"ratelimit: rate_limits[].disable_key is not yet supported (runtime keying deferred)"`; `"ratelimit: the 'extension' descriptor action is not yet supported"`; `"ratelimit: the deprecated 'dynamic_metadata' descriptor action is not supported; use 'metadata'"`); `TestParseRejectConstants_ByteStable` (asserts every wording const byte-for-byte per ADR-0080).
- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/filter/http/ratelimit/ -run 'TestBuildCompiledConfig|TestParseRejectConstants|TestValidateRouteRateLimits' -v`. Expected: FAIL (symbols undefined).
- [ ] **Step 3: Author `compiled_config.go`** — `compiledConfig` struct + the wording consts + `buildCompiledConfig` (§5.1 + defaults/clamps + cluster-load gates) + `ValidateRouteRateLimits` (§5.2).
- [ ] **Step 4: Run tests to verify they pass** — `go test ./internal/filter/http/ratelimit/ -run 'TestBuildCompiledConfig|TestParseRejectConstants|TestValidateRouteRateLimits' -v`. Expected: PASS.
- [ ] **Step 5: Land ADR-0200 §Decision + §Consequences body** in `docs/envoy-go/DECISIONS.md` — replace the `_(Lands at phase-24 IMPL ...)_` placeholders under ADR-0200 with the codified §Decision (the 3 envoy-go-strict PARSE-REJECT arms + the AMEND-2/7 hardcoded-runtime-key honor-as-static + byte-stable wording per ADR-0080) + §Consequences (departures 15→18; the `disable_key`/`extension`/`dynamic_metadata` rejects are NOT differential boot-rejects — upstream accepts them — unit-tested + BEHAVIOR_CONTRACT-recorded).
- [ ] **Step 6: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/filter/http/ratelimit/...
git add internal/filter/http/ratelimit/compiled_config.go internal/filter/http/ratelimit/compiled_config_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Task 3: compiled_config + FULL §5 PARSE-REJECT roster + ValidateRouteRateLimits [ADR-0200]"
```

---

## Task 4: `stats.go` — cluster-scoped cross-namespace 4-counter surface

**Files:**
- Create: `internal/filter/http/ratelimit/stats.go` (~40–60 LoC)
- Create: `internal/filter/http/ratelimit/stats_test.go` (~80–140 LoC)
- Append: PROGRESS.md (Task 4 entry)

The cluster-scoped 4-counter surface (parent §6.8 + AMEND-1 + AMEND-10): `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].{ok,error,over_limit,failure_mode_allowed}` self-registered via `ctx.Stats.NewCounterIfAbsent(...)`. The FIRST landed cross-namespace cluster-stat-charge. Project stat count **110 → 114**. **PARALLELIZABLE with Tasks 2 + 3.**

**Precondition:** Task 1 complete.
**Acceptance:** `go build` + `go vet` + `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/ratelimit/ -run 'TestFilterStats'` clean.

**Subagent dispatch outline:**
> Author `newFilterStats(reg *stats.Registry, clusterName, statPrefix string) *filterStats` exactly per parent §6.8: `base := "cluster." + clusterName + ".ratelimit."`; if `statPrefix != ""` append `statPrefix + "."`; the 4 counters via `reg.NewCounterIfAbsent(base + "{ok,error,over_limit,failure_mode_allowed}")`. Idempotent (safe across listeners sharing one RLS cluster). Package-level const declarations for the 4 stat-name leaves; `TestStatNames_ByteStable` asserts them per ADR-0143 SN2-reuse. Per ADR-0085 nil-tolerance.

- [ ] **Step 1: Write the failing test** in `stats_test.go` — `TestFilterStats_ClusterScopedNames` (empty `statPrefix`: names are exactly `cluster.rls.ratelimit.ok` etc.; non-empty `statPrefix=foo`: `cluster.rls.ratelimit.foo.ok` etc.); `TestFilterStats_NewCounterIfAbsent_Idempotent` (two `newFilterStats` calls against the same registry+cluster return the same underlying counters — no double-register panic); `TestStatNames_ByteStable` (the 4 leaf consts byte-exact).
- [ ] **Step 2: Run test to verify it fails** — `go test ./internal/filter/http/ratelimit/ -run 'TestFilterStats|TestStatNames' -v`. Expected: FAIL.
- [ ] **Step 3: Author `stats.go`** per parent §6.8.
- [ ] **Step 4: Run test to verify it passes** — Expected: PASS.
- [ ] **Step 5: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/filter/http/ratelimit/...
git add internal/filter/http/ratelimit/stats.go internal/filter/http/ratelimit/stats_test.go docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Task 4: cluster-scoped cross-namespace 4-counter stat surface (110->114)"
```

---

## Task 5: DELTA-2 — HCM route-table `rate_limits` exposure + accessor pair + ADR-0198 (HIGHEST RISK)

**Files:**
- Modify: `internal/filter/hcm/route.go:73,80` (NEW raw-policy fields on `routeEntry` + `routeTable`)
- Modify: `internal/filter/hcm/config.go` (parse `vh.GetRateLimits()` in the vhost parse path [`vh` is bound near `:221`] + each route's `r.GetRoute().GetRateLimits()` in `buildRouteTable` [`:379`]; call `ratelimit.ValidateRouteRateLimits` at parse time — re-confirm exact line numbers at IMPL, they drift with edits)
- Modify: `internal/filter/hcm/chain.go` (seed the matched route's + vhost's raw policies onto the per-stream chain at dispatch — the ADR-0165 set-once pattern)
- Modify: `internal/filter/http/callbacks.go` (NEW `RouteRateLimits()` + `VirtualHostRateLimits()` on `DecoderFilterCallbacks`, near `DownstreamRemoteAddr()`:101)
- Modify: `internal/filter/http/chain.go` (the chain-impl backing fields + setters + accessors)
- Create/Modify: HCM DELTA-2 tests (`internal/filter/hcm/route_test.go` or a new `ratelimit_routetable_test.go`)
- Modify: `docs/envoy-go/DECISIONS.md` — ADR-0198 §Decision + §Consequences body
- Append: PROGRESS.md (Task 5 entry — RECORD the D-RL1 byte-confirmation outcome + whether ADR-0202 fired)

The single most novel surface in phase 24 (AMEND-9; ADR-0198). **D-RL1 RECOMMENDED:** seed RAW `[]*routev3.RateLimit`; expose `RouteRateLimits() []*routev3.RateLimit` + `VirtualHostRateLimits() []*routev3.RateLimit`. **D-RL2:** HCM calls `ratelimit.ValidateRouteRateLimits` at `buildRouteTable` + vhost parse so the §5.2 strict rejects fire at boot. **ADR-0198 §Decision + §Consequences body lands at this Task's commit.** **Depends on Task 3.**

**Precondition:** Task 3 complete (`ValidateRouteRateLimits` exported).
**Acceptance:** `go build ./...` + `go vet` + `golangci-lint run` clean; `go test -race -count=1 ./internal/filter/hcm/... ./internal/filter/http/...` clean (incl. the new accessor + zero-regression to existing route-table tests); the full pre-existing differential baseline (0000-0031) still GREEN (Gate D regression check per parent §12 item 8); ADR-0198 §Decision + §Consequences present.

**Subagent dispatch outline:**
> FIRST ACTION (D-RL1 byte-confirmation): read the ADR-0165 `DownstreamRemoteAddr` plumbing (`internal/filter/http/callbacks.go:101,111`; the chain field + setter + accessor in `chain.go`; the dispatch-time seed in `hcm/chain.go`) and confirm the RAW-`[]*routev3.RateLimit` seed fits that exact shape. If it fits (expected): proceed with the RECOMMENDED design. If the seed shape MUST diverge (e.g., needs a pre-compiled carrier or a non-proto type), STOP, escalate to the user, and fire ADR-0202 (§Context + §Decision + §Consequences at this commit) documenting the divergence. Then: (1) add NEW raw-policy fields to `routeEntry` (`route.go:73`) + `routeTable` (`route.go:80`); (2) in `config.go` parse `vh.GetRateLimits()` (`:221`) + each route's `r.GetRoute().GetRateLimits()` (in `buildRouteTable`, `:379`), call `ratelimit.ValidateRouteRateLimits(...)` and propagate the §5.2 reject error; (3) seed the matched route's + the vhost's raw policies onto the per-stream chain at HCM dispatch (mirror the `SetRequestCtx`/`DownstreamRemoteAddr` set-once seed); (4) add `RouteRateLimits()`/`VirtualHostRateLimits()` to the `DecoderFilterCallbacks` interface + the chain impl. Then write the ADR-0198 §Decision + §Consequences body. RECORD in PROGRESS the D-RL1 outcome (raw-proto seed confirmed / ADR-0202 fired).

- [ ] **Step 1: Confirm the D-RL1 seed shape** against the ADR-0165 plumbing (FIRST action above). Record the outcome in PROGRESS.
- [ ] **Step 2: Write the failing test** — `TestRouteTableRateLimits_ParseRetainSeed` (a route + vhost config with `rate_limits[]`; assert `routeEntry`/`routeTable` retain the raw policies; assert `RouteRateLimits()`/`VirtualHostRateLimits()` return the matched policies after dispatch); `TestRouteTableRateLimits_StrictReject` (a route `RateLimit` with `disable_key`/`extension`/`dynamic_metadata` ⇒ HCM parse returns the §5.2 byte-stable error); `TestRouteTableRateLimits_ZeroRegression` (a config with NO `rate_limits` ⇒ accessors return nil/empty, existing route-table behavior unchanged).
- [ ] **Step 3: Run test to verify it fails** — `go test ./internal/filter/hcm/... ./internal/filter/http/... -run 'TestRouteTableRateLimits' -v`. Expected: FAIL.
- [ ] **Step 4: Implement DELTA-2** — the 4 sub-changes from the dispatch outline (route.go fields; config.go parse + validate; chain.go seed; callbacks.go + chain.go accessor pair).
- [ ] **Step 5: Run test to verify it passes** + the regression sweep — `go test -race ./internal/filter/hcm/... ./internal/filter/http/...` PASS; `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-9]|3[01])'` still GREEN (no regression to 0000-0031).
- [ ] **Step 6: Land ADR-0198 §Decision + §Consequences body** in DECISIONS.md — codify the parse/retain/seed/accessor primitive (NOT a `RequestRouteConfig()` reuse per AMEND-9; the ADR-0165 set-once precedent; narrow-exposure/YAGNI — only `rate_limits` exposed; filter owns interpretation) + the D-RL1 byte-confirmation outcome + whether ADR-0202 fired.
- [ ] **Step 7: Append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/filter/hcm/... ./internal/filter/http/...
git add internal/filter/hcm/ internal/filter/http/callbacks.go internal/filter/http/chain.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Task 5: DELTA-2 HCM route-table rate_limits exposure + RouteRateLimits()/VirtualHostRateLimits() accessor pair [ADR-0198]"
```

---

## Task 6: `descriptors.go` — the 5 CORE-action descriptor engine

**Files:**
- Create: `internal/filter/http/ratelimit/descriptors.go` (~200–300 LoC)
- Create: `internal/filter/http/ratelimit/descriptors_test.go` (~300–450 LoC)
- Append: PROGRESS.md (Task 6 entry)

The §4 engine for the 5 CORE actions only (parent §4.1 rows): `generic_key` (key default `"generic_key"`), `request_headers` (config key REQUIRED), `remote_address` (key `"remote_address"`), `destination_cluster` (key `"destination_cluster"`), `header_value_match` (key default `"header_match"`, `expect_match` default true) + the empty-action-drop discipline's TWO behaviors (§4.5) + Axis-A early-return + the OVERRIDE-default vhost walk (D-RL6). The remaining 5 actions + `stage` + the Axis-B table land at 24.2. Produces `[]*ratelimitv3.RateLimitDescriptor` (entries in action-list order; AMEND-6 proto-number-faithful). **PARALLELIZABLE with Task 5** (engine is unit-tested over raw policy inputs; integration is Task 7).

**Precondition:** Task 3 complete.
**Acceptance:** `go build` + `go vet` + `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/ratelimit/ -run 'TestDescriptors'` clean.

**Subagent dispatch outline:**
> Author the engine for the 5 CORE actions verbatim per parent §4.1 (the per-action key/value/drop rules, line-cited against `router_ratelimit.cc`) + the §4.5 empty-action-drop (action returns false ⇒ WHOLE descriptor dropped + loop breaks; empty-key entry skipped, descriptor survives) + Axis-A early-return (if embedded `rate_limits` present, walk only that — at 24.1 the embedded list comes from the route policy; `RateLimitPerRoute` Axis-A + `domain` override land at 24.2) + the OVERRIDE-default vhost walk (route policy always walked; vhost walked only if the route has no rate_limits — D-RL6). Build `[]*ratelimitv3.RateLimitDescriptor` with `entries` in action-list order. The remaining 5 actions return a clearly-marked "lands at 24.2" path (NOT silently dropped — a config exercising them at 24.1 has no descriptor source yet, but the engine MUST be structured so 24.2 extends it cleanly). Stage filtering: evaluate only the default stage-0 bucket at 24.1.

- [ ] **Step 1: Write the failing test** in `descriptors_test.go` — `TestDescriptors_PerAction` (one row per CORE action: exact `{key,value}` per AMEND-11; `generic_key`→"generic_key", `header_value_match`→"header_match" with `expect_match` true, `request_headers` requires config key, `remote_address`→IP string, `destination_cluster`→routeEntry cluster name); `TestDescriptors_EmptyActionDrop` (the two behaviors); `TestDescriptors_AxisA_EarlyReturn`; `TestDescriptors_OverrideDefault_VhostWalk` (route-empty ⇒ vhost walked; route-non-empty ⇒ vhost skipped under OVERRIDE default); `TestDescriptors_EntriesActionOrder`.
- [ ] **Step 2: Run test to verify it fails** — `go test ./internal/filter/http/ratelimit/ -run 'TestDescriptors' -v`. Expected: FAIL.
- [ ] **Step 3: Author `descriptors.go`** per parent §4.1/§4.5 + D-RL6.
- [ ] **Step 4: Run test to verify it passes** — Expected: PASS.
- [ ] **Step 5: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/filter/http/ratelimit/...
git add internal/filter/http/ratelimit/descriptors.go internal/filter/http/ratelimit/descriptors_test.go docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Task 6: descriptor engine for the 5 core actions + empty-action-drop + Axis-A/OVERRIDE-default walk"
```

---

## Task 7: `decode_headers.go` + `dispositions.go` + full `New` + boot-registration + ADR-0197[core]

**Files:**
- Create: `internal/filter/http/ratelimit/decode_headers.go` (~120–180 LoC)
- Create: `internal/filter/http/ratelimit/dispositions.go` (~120–180 LoC)
- Modify: `internal/filter/http/ratelimit/ratelimit.go` (full `New` body — wire config + client + stats + engine)
- Create: `internal/filter/http/ratelimit/decode_headers_test.go` + `dispositions_test.go` (~250–400 LoC)
- Modify: `cmd/envoy-go/main.go:144` (boot-registration: `httpReg.Register(ratelimit.TypeURL, ratelimit.New)` alphabetical between `oauth2.New`:144 and `rbac.New`:145 → **19 HTTP filters wired**)
- Modify: `docs/envoy-go/DECISIONS.md` — ADR-0197 §Decision + §Consequences body (CORE slice)
- Append: PROGRESS.md (Task 7 entry)

Completes the core decision path. `DecodeHeaders`: build descriptors via the engine over `RouteRateLimits()`/`VirtualHostRateLimits()`; zero ⇒ continue; else async `ShouldRateLimit` + `StopIteration` (the fault/ext_authz async-resume pattern); OnDestroy cancels the per-stream context (the ext_authz `callCtx`/`callCancel` precedent). `dispositions.go`: OK ⇒ `ok.inc()` + apply RLS `request_headers_to_add` + continue; OVER_LIMIT ⇒ `over_limit.inc()` + the §4.7 byte-shape (429/`request_rate_limited`/`x-envoy-ratelimited`/RLS+config `response_headers_to_add` in AMEND-8 order/gRPC 8 vs 14); error ⇒ `error.inc()` + (`failure_mode_deny` ? 500/`rate_limiter_error`/nullptr-mutate : `failure_mode_allowed.inc()` + continue). **X-RateLimit injection point STUBBED** with a forward-pointer to 24.2 (D-RL7). **ADR-0197[core] §Decision + §Consequences body lands at this Task's commit.** **Depends on Tasks 2, 3, 4, 5, 6.**

**Precondition:** Tasks 2–6 complete.
**Acceptance:** `go build ./...` + `go vet` + `golangci-lint run` clean; `go test -race -count=1 ./internal/filter/http/ratelimit/...` clean; `cmd/envoy-go` boot wires 19 HTTP filters (assert via a registry count test or `grep -c httpReg.Register cmd/envoy-go/main.go` = 19); ADR-0197 §Decision + §Consequences (CORE slice) present.

**Subagent dispatch outline:**
> Wire the full `New` factory (parse via `buildCompiledConfig`; build the `RateLimitClient` via the `buildGRPCCheckFn` call-site precedent — google_grpc reject, cluster `Get` + `UseH2` gates, `grpcclient.New` + `NewRateLimitClient`; allocate `filterStats` via `newFilterStats` keyed off the RLS cluster name; per ADR-0085 nil-tolerance). `decode_headers.go`: descriptor build over the DELTA-2 accessors → zero⇒Continue / else async `ShouldRateLimit` + `StopIteration`; OnDestroy cancel. `dispositions.go`: the §4.6 dispositions + the §4.7 OVER_LIMIT/error byte-shape (X-RateLimit STUBBED — leave a `// 24.2: X-RateLimit DRAFT_VERSION_03 injection (parent §6.6)` marker). Boot-registration alphabetical oauth2↔rbac. Then write the ADR-0197[core] §Decision + §Consequences body (scope the CORE slice; note the X-RateLimit-header + remaining-actions slice lands at 24.2).

- [ ] **Step 1: Write the failing tests** — `TestDecodeHeaders_ZeroDescriptors_Continue`; `TestDecodeHeaders_AsyncDispatch_StopIteration`; `TestDecodeHeaders_OnDestroy_Cancels`; `TestDispositions_OK_Continue` (ok counter inc + RLS request_headers_to_add applied); `TestDispositions_OverLimit_429_ByteShape` (429/`request_rate_limited` rc-details/`x-envoy-ratelimited: true`/AMEND-8 header order/over_limit counter); `TestDispositions_Error_FailOpen` (default: error + failure_mode_allowed counters + continue); `TestDispositions_Error_FailClosed` (`failure_mode_deny=true`: 500/`rate_limiter_error`/nullptr-mutate/error counter); `TestDispositions_GRPC_8_vs_14`; `TestNew_FullWiring` (valid config builds a working filter). Use a test-double `DecoderFilterCallbacks` (with `RouteRateLimits()`/`VirtualHostRateLimits()`) + a fake `RateLimitClient`.
- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/filter/http/ratelimit/ -run 'TestDecodeHeaders|TestDispositions|TestNew_FullWiring' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** `decode_headers.go` + `dispositions.go` + the full `New` body + boot-registration.
- [ ] **Step 4: Run tests to verify they pass** — `go test -race ./internal/filter/http/ratelimit/...`. Expected: PASS.
- [ ] **Step 5: Land ADR-0197[core] §Decision + §Consequences body** in DECISIONS.md (CORE slice; X-RateLimit/remaining-actions slice → 24.2).
- [ ] **Step 6: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./... && grep -c 'httpReg.Register' cmd/envoy-go/main.go  # expect 19
git add internal/filter/http/ratelimit/ cmd/envoy-go/main.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Task 7: decode dispatch + OK/OVER_LIMIT/error dispositions + full New + boot-reg (19 filters) [ADR-0197 core]"
```

---

## Task 8: 33rd fuzzer `FuzzRateLimitConfigParse`

**Files:**
- Create: `internal/filter/http/ratelimit/fuzz_test.go` (~50 LoC)
- Create: `internal/filter/http/ratelimit/testdata/fuzz/FuzzRateLimitConfigParse/` (corpus ~30 seeds)
- Append: PROGRESS.md (Task 8 entry)

The 33rd project-wide fuzzer (parent §6.9). Must-never-panic across `buildCompiledConfig` + the (24.1-subset) descriptor-engine compile. Corpus ~30 seeds: a valid full config; each §5.1 + §5.2 PARSE-REJECT arm; each CORE action; empty config. **PARALLELIZABLE with Task 9.**

**Precondition:** Tasks 3 + 6 complete (`buildCompiledConfig` + engine non-skeleton); green-run needs Task 7's wired `New`.
**Acceptance:** `go test -run 'XXX_NONE' -fuzz 'FuzzRateLimitConfigParse' -fuzztime 30s ./internal/filter/http/ratelimit/` clean (no panic); seed corpus committed; project fuzzer count = 33.

**Subagent dispatch outline:**
> Author `FuzzRateLimitConfigParse` per ADR-0018 baseline + parent §6.9: fuzz the raw `typed_config` bytes through `buildCompiledConfig` (+ the descriptor-engine compile for any embedded actions); must-never-panic. Seed corpus covers a valid full config + each §5.1/§5.2 reject arm + each CORE action + empty. Commit the corpus.

- [ ] **Step 1: Author `fuzz_test.go`** + seed corpus.
- [ ] **Step 2: Run the fuzzer** — `go test -run 'XXX_NONE' -fuzz 'FuzzRateLimitConfigParse' -fuzztime 30s ./internal/filter/http/ratelimit/`. Expected: no panic, clean exit.
- [ ] **Step 3: Verify fuzzer count** — `find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l` returns `33`.
- [ ] **Step 4: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/filter/http/ratelimit/...
git add internal/filter/http/ratelimit/fuzz_test.go internal/filter/http/ratelimit/testdata docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Task 8: 33rd fuzzer FuzzRateLimitConfigParse (must-never-panic)"
```

---

## Task 9: Shared fake `RateLimitService` + `HTTPGlobalRateLimitGRPC` BackendKind + runner wiring

**Files:**
- Create: `test/helpers/ratelimitgrpc/ratelimitgrpc.go` + `ratelimitgrpc_test.go` (~150–250 LoC)
- Modify: `test/differential/fixture/fixture.go` (NEW `HTTPGlobalRateLimitGRPC BackendKind = 24` after `HTTPAdmissionControl = 23`:409 + dispatcher metadata)
- Modify: `test/differential/runner_test.go` (blank import for `internal/filter/http/ratelimit`; switch-case for `HTTPGlobalRateLimitGRPC`)
- Append: PROGRESS.md (Task 9 entry)

The SHARED fake gRPC `RateLimitService` (the `test/helpers/extauthzgrpc/` `0021` precedent) dialed by BOTH sides. Per D-RL5/AMEND-6 the fake emits `RateLimitResponse` by proto field NUMBER + omits unset optionals; deterministic descriptor → response script map. **Depends on Task 7.** **PARALLELIZABLE with Task 8.**

**Precondition:** Task 7 complete (functional filter).
**Acceptance:** `go build ./...` + `go vet` + `golangci-lint run` clean; `go test -count=1 ./test/helpers/ratelimitgrpc/...` clean; `HTTPGlobalRateLimitGRPC = 24` present; the runner compiles with the new switch-case.

**Subagent dispatch outline:**
> Clone the `test/helpers/extauthzgrpc/` server shape into `test/helpers/ratelimitgrpc/`: a `RegisterRateLimitServiceServer` impl of `ShouldRateLimit` with a deterministic script map (descriptor canonical-string → `RateLimitResponse{overall_code, statuses}`); a `NewAtAddr(addr)` constructor + `Stop()` + a per-scenario `Script` setter. CRITICAL (D-RL5/AMEND-6): build the response by proto field NUMBER + omit unset optionals (`raw_body`/`dynamic_metadata`/`quota`/per-descriptor `hits_addend`). Add the `HTTPGlobalRateLimitGRPC = 24` BackendKind + dispatcher metadata + the runner blank-import + switch-case. A self-test confirms the fake round-trips a known descriptor.

- [ ] **Step 1: Author `test/helpers/ratelimitgrpc/`** (server + `NewAtAddr` + `Script` + `Stop`) + a self-test.
- [ ] **Step 2: Add `HTTPGlobalRateLimitGRPC = 24`** to `fixture.go` + dispatcher metadata; wire the runner blank-import + switch-case.
- [ ] **Step 3: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./test/...
go test -count=1 ./test/helpers/ratelimitgrpc/...
git add test/helpers/ratelimitgrpc/ test/differential/fixture/fixture.go test/differential/runner_test.go docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Task 9: shared fake RateLimitService + HTTPGlobalRateLimitGRPC BackendKind (proto-number-faithful per AMEND-6)"
```

---

## Task 10: Differential fixture `0032-http-ratelimit` (scenarios a/b/c/d-core/e/h)

**Files:**
- Create: `test/fixtures/0032-http-ratelimit/{README.md,envoy.yaml,envoy-go.yaml,expectations.yaml,inputs/driver.go}` (~400–600 LoC)
- Append: PROGRESS.md (Task 10 entry)

The cross-side fixture (parent §7.1; 24.1 scenarios). Single-listener topology (parent §7.3 — avoids the `freeTCPPort` combined-run flake): one HCM with the ratelimit filter (alphabetical) + router terminator, the RLS cluster pointing at the fake gRPC server, a synthetic always-200 backend. The fake port is allocated once + shared via `host.docker.internal` (reference)/`127.0.0.1` (subject) templating per the `0021` pattern. **24.1 scenarios:** (a) `parse_ok` [subject-only structural], (b) `ok_admit` [cross-side byte-exact], (c) `over_limit_429` [cross-side byte-exact], (d) `descriptor_actions` [cross-side, restricted to the 4 core actions `generic_key`/`request_headers`/`remote_address`/`header_value_match`], (e) `failure_mode_open` [cross-side byte-exact], (h) `stat_surface` [subject-only via `StatsAsserter.AssertStats`, **proven live via deliberate-break** per `reference_differential_asserter_dispatch` — subject-side assertions go in `StatsAsserter`, NOT `SubjectAsserter`]. 24.2 ADDS (f)/(g) + extends (d). **Depends on Task 9.** **PARALLELIZABLE with Task 11.**

**Precondition:** Task 9 complete.
**Acceptance:** `go test -count=1 ./test/differential/ -run 'Test.*0032'` GREEN; cross-side byte-exact on (b)/(c)/(d-core)/(e); scenario (h) `StatsAsserter` asserts the 4 cluster-scoped counters AND is proven live (a deliberate wrong-value edit makes it FAIL, then reverted).

**Subagent dispatch outline:**
> Author `0032-http-ratelimit` per parent §7.1 + §7.3 + the `0021` fixed-pre-allocated-port pattern. The driver implements `fixture.Driver` + `fixture.BackendKindAware` (returns `HTTPGlobalRateLimitGRPC`) + `fixture.StatsAsserter` (for scenario h). The driver allocates the fake port, renders both YAMLs, starts `ratelimitgrpc.NewAtAddr`, sets the per-scenario `Script`, runs the probes, asserts, and `Stop`s. Scenario (h)'s `AssertStats` reads `/stats` from both admin addrs and asserts the 4 `cluster.<rls>.ratelimit.*` counters at expected values after a burst; PROVE it live by temporarily asserting a wrong value (must FAIL), then revert. Cross-side scenarios assert byte-exact via the existing `CompareBytes`.

- [ ] **Step 1: Author the fixture directory** (README + both YAMLs + expectations + driver) covering scenarios a/b/c/d-core/e/h.
- [ ] **Step 2: Run the fixture** — `go test -count=1 ./test/differential/ -run 'Test.*0032' -v`. Expected: GREEN (all 6 scenarios).
- [ ] **Step 3: Prove scenario (h) live** — edit the `AssertStats` expected value to a wrong number; re-run; confirm FAIL; revert; confirm GREEN. Record the deliberate-break evidence in PROGRESS per `reference_differential_asserter_dispatch`.
- [ ] **Step 4: Verify gates + append PROGRESS + commit**

```bash
go test -count=1 ./test/differential/ -run 'Test.*0032'
git add test/fixtures/0032-http-ratelimit/ docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Task 10: differential fixture 0032-http-ratelimit (a/b/c/d-core/e/h; StatsAsserter proven live)"
```

---

## Task 11: Differential fixture `0033-http-ratelimit-boot-reject`

**Files:**
- Create: `test/fixtures/0033-http-ratelimit-boot-reject/{README.md,envoy.yaml,envoy-go.yaml,expectations.yaml,inputs/driver.go}` (~150–250 LoC)
- Append: PROGRESS.md (Task 11 entry)

The boot-reject fixture (parent §7.2): the **`domain` empty** shared reject (upstream PGV/`ASSERT` rejects; envoy-go's §5.1 mirror rejects). Reuses the 22.1 `BootRejectFixture` harness interface (`test/differential/harness.go:340`; `ExpectedBootErrorSubstring()`). NOTE per §7.2: the `disable_key`/`extension`/`dynamic_metadata` rejects are NOT boot-reject fixtures (upstream accepts them — unit-tested + BEHAVIOR_CONTRACT-recorded). **Depends on Task 9** (BackendKind + runner wiring). **PARALLELIZABLE with Task 10.**

**Precondition:** Task 9 complete.
**Acceptance:** `go test -count=1 ./test/differential/ -run 'Test.*0033'` GREEN; both sides exit non-zero AND both stderr buffers contain the common `domain`-empty substring (D-RL4).

**Subagent dispatch outline:**
> Author `0033-http-ratelimit-boot-reject` per parent §7.2. The driver implements `fixture.BootRejectFixture`: `BootRejectConfig`/`ExpectedBootErrorSubstring()` returns the common `domain`-empty substring (finalized against the captured both-sides stderr — D-RL4). Both YAMLs carry a ratelimit filter with empty `domain`. The runner's `runBootRejectFixture` branch asserts both sides exit non-zero + both stderr contain the substring.

- [ ] **Step 1: Author the fixture** + capture both-sides stderr to finalize the common substring (D-RL4).
- [ ] **Step 2: Run the fixture** — `go test -count=1 ./test/differential/ -run 'Test.*0033' -v`. Expected: GREEN.
- [ ] **Step 3: Verify gates + append PROGRESS + commit**

```bash
go test -count=1 ./test/differential/ -run 'Test.*0033'
git add test/fixtures/0033-http-ratelimit-boot-reject/ docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md
git commit -m "phase 24.1 Task 11: differential fixture 0033-http-ratelimit-boot-reject (domain empty)"
```

---

## Task 12: Atomic landing — BEHAVIOR_CONTRACT partial bundle + STATE + ROADMAP + REVIEW

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the 24.1 partial bundle per parent §13, atomic per ADR-0052)
- Modify: `docs/envoy-go/ROADMAP.md` (row `24.1` `in-progress → done`; per-cell IMPL-done annotation; parent row `24` STAYS `in-progress`; row `24.2` UNCHANGED `planned`)
- Modify: `docs/envoy-go/STATE.md` (re-advance per BOOTSTRAP §4.1 — active-phase `24.2` awaiting its PLAN, OR 24.1-done/24.2-pending; next-skill for 24.2 is `superpowers:writing-plans`; next-free ADR-0202 unless the Task-5 escape-valve fired)
- Create: `docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/REVIEW.md`
- Append: PROGRESS.md (Task 12 entry + the six-gate verbatim outputs)

The atomic-landing task per the phase-09..23 + 22.1 precedent. The ADR §Decision/§Consequences bodies ALREADY landed at their functional Tasks (ADR-0200 @ Task 3, ADR-0198 @ Task 5, ADR-0197[core] @ Task 7) — this Task does NOT re-land them; it lands the doc-state + the BEHAVIOR_CONTRACT partial bundle + REVIEW. **Depends on everything.**

**Precondition:** Tasks 1–11 complete; all six gates GREEN.
**Acceptance:** all six gates GREEN (verbatim in PROGRESS + REVIEW); BEHAVIOR_CONTRACT partial bundle landed; ROADMAP row 24.1 `done`; STATE re-advanced; REVIEW.md authored.

**Subagent dispatch outline:**
> Run the six-gate verification (A build / B vet+lint / C race / D differential 35/35 / E fuzz 33 / F h2spec 53/53) capturing verbatim outputs. Land the 24.1 BEHAVIOR_CONTRACT partial bundle per parent §13: the NEW `### envoy.filters.http.ratelimit` subsection (CORE parts — descriptor engine for the 5 core actions + dispositions + the cluster-scoped 4-counter surface + DELTA-2; the per-route + X-RateLimit allow-list parts land at 24.2) + the 3 envoy-go-strict departure records (15→18) + the stat-name mapping 110→114. Flip ROADMAP row 24.1 → done (parent row 24 stays in-progress). Re-advance STATE for 24.2. Author REVIEW.md per `superpowers:requesting-code-review`. Run the plan-document-reviewer-equivalent confirmation that all SPEC §6 gates + the 24.1 §7 acceptance subset are GREEN.

- [ ] **Step 1: Run the six gates** — capture verbatim:

```bash
go build ./...                                              # Gate A
go vet ./... && golangci-lint run                           # Gate B
go test -race -count=1 ./...                                # Gate C
go test -count=1 ./test/differential/                       # Gate D — 35/35 (0000-0033)
# Gate E — fuzz smoke across the 33 fuzzers (30s/seed for the new one)
# Gate F — h2spec 53/53 at the ADR-0051 v1.32.4 pin
```

- [ ] **Step 2: Land the BEHAVIOR_CONTRACT.md partial bundle** (NEW subsection core + 3 departures 15→18 + stat 110→114), atomic per ADR-0052.
- [ ] **Step 3: Flip ROADMAP row 24.1 → done** (parent row 24 stays in-progress; row 24.2 unchanged planned).
- [ ] **Step 4: Re-advance STATE.md** for 24.2 (next-skill `superpowers:writing-plans`; next-free ADR-0202 unless the Task-5 escape-valve fired; record the D-hypothesis disposition — ADR-0202 unconsumed at 24.1 phase-done unless fired).
- [ ] **Step 5: Author REVIEW.md** per `superpowers:requesting-code-review` (the 24.1 §7 acceptance subset confirmed; six-gate evidence).
- [ ] **Step 6: Append PROGRESS + commit** (the phase-done squash-merge to master happens after REVIEW approval per the phase-09..23 stage-close pattern):

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/ROADMAP.md docs/envoy-go/STATE.md docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/
git commit -m "phase 24.1 Task 12: BEHAVIOR_CONTRACT partial bundle (15->18, 110->114) + ROADMAP row 24.1 done + STATE re-advance + REVIEW.md"
```

---

## Audit-trail summary

SPEC (24.1 slice + parent master) → this PLAN (12 tasks) → PROGRESS (1:1 per task) → REVIEW. The 3 24.1-landing ADR bodies land at their functional Tasks (ADR-0200@3, ADR-0198@5, ADR-0197[core]@7); BEHAVIOR_CONTRACT + doc-state land atomically at Task 12. The SPEC §12 byte-confirmations are recorded at their Tasks (D-RL1@5, D-RL4@11, D-RL5@9). The D-hypothesis disposition (ADR-0202 UNCONSUMED at 24.1 phase-done unless the Task-5 DELTA-2 escape-valve fires) is recorded at Task 12. Row 24.1 flips `done`; the parent row 24 + the full §15 UNION close at 24.2 phase-done.
