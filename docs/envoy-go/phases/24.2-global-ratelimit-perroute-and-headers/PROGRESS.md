# Phase 24.2 — Implementation PROGRESS

> Authoritative input: `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PLAN.md` (8-task TDD plan + Pre-Task 0). Parent master SPEC: `docs/envoy-go/phases/24-http-filter-global-ratelimit/SPEC.md` (§1.1 AMEND catalog + §4 engine + §5 PARSE-REJECT + §6 code shapes + §7 differential + §10 ADR map + §11 D1–D7 + §12 byte-confirmations + §14 testing taxonomy + §15 acceptance). 24.2 SPEC: `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/SPEC.md`. Sibling-precedent PROGRESS shape: `docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PROGRESS.md`.

**Scope.** Phase 24.2 is the **remaining-surface slice** of `envoy.filters.http.ratelimit` (the SEVENTEENTH §9 production HTTP filter, GLOBAL rate limit), executing on top of 24.1's already-landed core decision path. 9 tasks (Pre-Task 0 + Tasks 1–8). EXTENDS the 24.1-landed `internal/filter/http/ratelimit/` package (no new top-level Go package). Lands the **5 remaining descriptor actions** (`source_cluster` / `masked_remote_address` / `metadata` / `query_parameters` / `query_parameter_value_match`) + the `stage` multi-stage bucketing path + the Axis-B `vh_rate_limits` cross-tier composition table + the **X-RateLimit DRAFT_VERSION_03** response headers + the **`RateLimitPerRoute`** NEW 10th canonical per-route shape + scenarios (f) `vh_inclusion` + (g) `x_ratelimit_headers` to the EXISTING `0032-http-ratelimit/` (no new fixture dir) + extends scenario (d) `descriptor_actions` to all 10 actions + extends the existing `FuzzRateLimitConfigParse` corpus (no new fuzzer).

**ADR landings.** The 24.2 phase-done commit lands **ADR-0199 (FULL §Decision + §Consequences)** + the **ADR-0197 IN-PLACE §Decision amendment** (X-RateLimit + remaining-actions slice) + the **ADR-0125 §(xv) AMENDMENT 9 → 10** (anchored in ADR-0199) + the BEHAVIOR_CONTRACT completion bundle (per-route + X-RateLimit allow-list + descriptor-engine completion). **ADR-0202** is a reserved escape valve (next-free; UNCONSUMED at PLAN time) — fires only if Task 1's `metadata`-accessor surface or Task 5's X-RateLimit byte format diverges from the upstream-parity hypothesis.

IMPL worktree: `.worktrees/phase-24.2-global-ratelimit-perroute-and-headers-impl`. IMPL branch: `phase-24.2-global-ratelimit-perroute-and-headers-impl` (branched off master tip `3b17f43`). Each Task below appends one entry per the D-P3 discipline.

---

## Pre-Task 0 — 12-precondition verification (verbatim outputs)

All commands run from the IMPL worktree root.

### Precondition 1 — Worktree branch

```
$ git rev-parse --abbrev-ref HEAD
phase-24.2-global-ratelimit-perroute-and-headers-impl
```

PASS — expected `phase-24.2-global-ratelimit-perroute-and-headers-impl`.

### Precondition 2 — Master tail

```
$ git log --oneline master | head -6
3b17f43 next-prompt.txt: repoint master-tip references to 350543b (actual HEAD)
350543b next-prompt.txt: repoint master-tip references to 4fd4c14 (actual HEAD)
4fd4c14 phase 24.2 PLAN stage-close: STATE.md SHA-fill (TBD-24.2-PLAN-SQUASH -> 81bc2be)
81bc2be Squash merge phase-24.2-global-ratelimit-perroute-and-headers-plan [ADR-0199 anchored @ Task 3, ADR-0197 amend @ Task 5, ADR-0125 §(xv) @ Task 3]
1d3293b next-prompt.txt: repoint master-tip references to 349b7a1 (actual HEAD)
349b7a1 next-prompt.txt: repoint master-tip references to 070eb26 (actual HEAD)
```

PASS — the 24.2 PLAN squash (`81bc2be`) + its SHA-fill follow-up (`4fd4c14`) are at the head of recent history. The 24.1 IMPL squash `a4fdc75` + the 24.1 SHA-fill `dbd2d3b` predate this window but are confirmed reachable (`git log --oneline master | grep -E '^(a4fdc75|dbd2d3b)'` returns both with their original commit-message bodies, including `Squash merge phase-24.1-global-ratelimit-core-and-route-table-impl [ADR-0197 core, ADR-0198, ADR-0200]` for `a4fdc75` and `phase 24.1 IMPL stage-close: STATE.md SHA-fill (TBD-24.1-IMPL-SQUASH -> a4fdc75)` for `dbd2d3b`). The two `next-prompt.txt` repoints at the top (`3b17f43`, `350543b`) are docs-only follow-ups that do not perturb code state.

### Precondition 3 — Toolchain

```
$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
$ docker version | head -16
Client: Docker Engine - Community
 Version:           28.4.0
 API version:       1.49 (downgraded from 1.51)
 Go version:        go1.24.7
 Git commit:        d8eb465
 Built:             Wed Sep  3 20:57:32 2025
 OS/Arch:           linux/amd64
 Context:           desktop-linux

Server: Docker Desktop 4.41.2 (191736)
 Engine:
  Version:          28.1.1
  API version:      1.49 (minimum version 1.24)
  Go version:       go1.23.8
  Git commit:       01f442b
  Built:            Fri Apr 18 09:52:57 2025
```

PASS — `go1.26.2` ≥ `go1.26.2`; `golangci-lint v1.64.8` matches ADR-0009 pin; Docker client + server both present and reachable.

### Precondition 4 — DECISIONS.md tail (next-free ADR number)

```
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
201
```

PASS — expected `201` (ADR-0201 at master tip); ADR-0202 is therefore next-free for the conditional escape valve (Task 1 metadata-accessor surface and/or Task 5 X-RateLimit byte-edge).

### Precondition 5 — ADR §Context drafts present

```
$ grep -cE '^## ADR-0199' docs/envoy-go/DECISIONS.md
1
$ grep -cE '^## ADR-0197' docs/envoy-go/DECISIONS.md
1
$ grep -cE '^## ADR-0202' docs/envoy-go/DECISIONS.md
0
```

PASS — ADR-0199 §Context draft + ADR-0197 (already with 24.1 CORE-slice §Decision + §Consequences body landed) both present; ADR-0202 absent (escape-valve unconsumed unless Task 1 or Task 5 fires it).

### Precondition 6 — NO 24.1-bound code regression

```
$ test -d internal/filter/http/ratelimit && test -f internal/grpcclient/ratelimit_client.go && test -d test/helpers/ratelimitgrpc && test -d test/fixtures/0032-http-ratelimit && grep -q 'HTTPGlobalRateLimitGRPC' test/differential/fixture/fixture.go && echo "ok: phase-24.1 surface present"
ok: phase-24.1 surface present
```

PASS — all five 24.1-landed surfaces (filter package dir, gRPC client file, shared-fake helper dir, fixture-0032 dir, `HTTPGlobalRateLimitGRPC` BackendKind token in `fixture.go`) are present at the IMPL cold-start. The 24.1 squash `a4fdc75` is intact on master.

### Precondition 7 — Parent SPEC + 24.2 SPEC SHAs

```
$ git log -1 --format=%H -- docs/envoy-go/phases/24-http-filter-global-ratelimit/SPEC.md
bf868a67eadc8a23f5799d0cf8d5998c8166cc6c
$ git log -1 --format=%H -- docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/SPEC.md
bf868a67eadc8a23f5799d0cf8d5998c8166cc6c
```

PASS — both SPEC files were last modified by the split-squash commit `bf868a67…` (the same commit that landed the 24.1 SPEC per Precondition 7 of 24.1 PROGRESS — the 24/24.1/24.2 SPEC triple landed atomically at PLAN-time SPLIT per ADR-0201). The parent SPEC has not been amended since the split.

### Precondition 8 — Pristine tree

```
$ git status --porcelain
(empty)
```

PASS — no uncommitted changes; ready for the first commit on the 24.2 IMPL branch.

### Precondition 9 — Pre-existing `-short` suite green

```
$ go test -count=1 -short ./... 2>&1 | awk '/^ok / {ok++} /^FAIL/ {fail++} /^\?[[:space:]]/ {nofile++} END {print "ok:", ok+0, "FAIL:", fail+0, "no-test-files:", nofile+0}'
ok: 64 FAIL: 0 no-test-files: 49
$ echo "EXIT=$?"
EXIT=0
```

PASS — 64 packages report `ok`, 0 FAIL across 113 output lines; `EXIT=0`. The `test/differential` package itself reports `[no tests to run]` under `-short` because `TestDifferential` self-skips when `testing.Short()` is set (`test/differential/runner_test.go:74-76`) — the differential baseline is exercised separately at Precondition 10.

### Precondition 10 — Pre-existing differential baseline green

Combined run (anchored regex per the 24.1 PROGRESS §Deviations note about `-run` first-segment anchoring):

```
$ go test -count=1 -timeout 30m ./test/differential/ -run 'TestDifferential/00(0[0-9]|1[0-9]|2[0-9]|3[0-3])'
... (35 subtests dispatched; 34 PASS silent under default -v=off; 1 FAIL — see below) ...
--- FAIL: TestDifferential (88.36s)
    --- FAIL: TestDifferential/0027-http-lua-full-bridge (2.26s)
        runner_test.go:810: subj start: subject ready: EOF
FAIL
FAIL	github.com/esalaine/envoy-go/test/differential	88.437s
FAIL
```

The single FAIL is the documented `subject ready: EOF` lua-bridge startup-race flake (parallel to the 22.2 REVIEW §7.4 `freeTCPPort` multi-listener flake and the 24.1 PROGRESS Precondition-10 `subj start: subject ready: EOF` precedent on `0025-http-adaptive-concurrency`). Re-run in isolation:

```
$ go test -count=1 -timeout 10m ./test/differential/ -run 'TestDifferential/0027-http-lua-full-bridge'
ok  	github.com/esalaine/envoy-go/test/differential	2.915s
```

PASS — baseline GREEN with documented flake: 35/35 fixtures (0000–0033) pass when 0027 is re-run in isolation, matching the known flake envelope. 24.2 IMPL inherits the 24.1 flake discipline (re-run in isolation; only persistent failure is a real regression).

### Precondition 11 — Fuzzer baseline

```
$ find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l
33
```

PASS — exactly 33 fuzzers at master tip (24.1 landed the 33rd `FuzzRateLimitConfigParse` at Task 8). Per D-RL16, 24.2 EXTENDS the existing `FuzzRateLimitConfigParse` corpus with ~10–20 new seeds rather than adding a new fuzzer; the project fuzzer count stays at **33** after Task 7.

### Precondition 12 — NEW 24.2 surfaces absent at IMPL cold-start

```
$ test ! -f internal/filter/http/ratelimit/encode.go && test ! -f internal/filter/http/ratelimit/headers.go && test ! -f internal/filter/http/ratelimit/compiled_perroute.go && ! grep -q 'actionSourceCluster\|actionMaskedRemoteAddress\|actionMetadata\|actionQueryParameters\|actionQueryParameterValueMatch' internal/filter/http/ratelimit/descriptors.go && ! grep -q 'scenario.*vh_inclusion\|scenario.*x_ratelimit_headers' test/fixtures/0032-http-ratelimit/README.md && echo "ok: phase-24.2-new-surfaces absent"
(no output)
$ echo "EXIT=$?"
EXIT=1
```

**PASS-SEMANTICALLY (PLAN-grep over-specification).** The literal compound check returns EXIT=1, but on disaggregation the three FILE-existence sub-checks all PASS:

```
$ test ! -f internal/filter/http/ratelimit/encode.go && echo OK
OK
$ test ! -f internal/filter/http/ratelimit/headers.go && echo OK
OK
$ test ! -f internal/filter/http/ratelimit/compiled_perroute.go && echo OK
OK
```

…and the two GREP sub-checks fail ONLY against pre-existing forward-reference text (not 24.2 implementation):

```
$ grep -nE 'actionSourceCluster|actionMaskedRemoteAddress|actionMetadata|actionQueryParameters|actionQueryParameterValueMatch' internal/filter/http/ratelimit/descriptors.go
289:		// 24.2: actionSourceCluster reads the node service-cluster name
293:		// 24.2: actionMaskedRemoteAddress applies the v4/v6_prefix_mask_len
297:		// 24.2: actionMetadata reads the metadata_key from the configured
303:		// 24.2: actionQueryParameters reads the first value of query_param_name
307:		// 24.2: actionQueryParameterValueMatch (parent §4.1 row 10; rl.cc:297,
$ grep -nE 'scenario.*vh_inclusion|scenario.*x_ratelimit_headers' test/fixtures/0032-http-ratelimit/README.md
6:`StatsAsserter.AssertStats` (h). 24.2 ADDS scenarios (f) `vh_inclusion`
```

The five `descriptors.go` hits are doc-comments on the existing `actionUnsupportedAt241()` stub arms — landed by 24.1 IMPL Task 1 to label each deferred-to-24.2 arm with its forthcoming helper name (see 24.1 PROGRESS lines 281-309 in the canonical entry for `descriptors.go`). The single `README.md` hit is the 24.2-planning forward-reference text written by the 24.1 IMPL Task 9 fixture-README author. No 24.2 implementation symbol exists — the semantic precondition (no NEW 24.2 surface landed at IMPL cold-start) is GREEN:

```
$ grep -nE '^func actionSourceCluster|^func actionMaskedRemoteAddress|^func actionMetadata|^func actionQueryParameters|^func actionQueryParameterValueMatch' internal/filter/http/ratelimit/descriptors.go
(no output; EXIT=1)
$ grep -nE 'actionUnsupportedAt241\(\)' internal/filter/http/ratelimit/descriptors.go | wc -l
6
```

All 5 deferred action arms still dispatch to `actionUnsupportedAt241()` (5 dispatch sites + 1 sentinel-function definition + 1 doc-comment reference = 6 occurrences as expected from 24.1 IMPL); no 24.2 implementation function has been authored yet.

PASS — vacuously green semantically. **Recorded deviation:** the PLAN's literal compound `grep` was authored against an assumed clean-room state, but 24.1 IMPL (correctly, per the `actionUnsupportedAt241()` stub-labeling discipline) embedded the five 24.2-helper function-name forward-references as comment text + the 24.2-scenario forward-reference in the fixture-README. The PLAN-grep is mechanically over-specified (it matches doc-comment text + README forward-references rather than implementation symbols); the SEMANTIC intent (no 24.2 NEW production code landed at IMPL cold-start) is verified by the disaggregation above. Recorded here so the next PLAN revision can refine the grep pattern (e.g., `^func actionSourceCluster\b` to anchor to function-definition lines). No IMPL impact — the implementation contract is unchanged.

---

## ADRs introduced/landed by this plan (2-ADR landing map + conditional ADR-0202)

Reproduced verbatim from PLAN.md `## ADRs introduced/landed by this plan`:

The 4 phase-24 §Context drafts (ADR-0197..0200) are anchored at the parent-SPEC commit per ADR-0044. 24.1 landed the §Decision + §Consequences bodies for ADR-0197[core] + ADR-0198 + ADR-0200. 24.2 lands the REMAINING ADR work at the materializing Tasks:

| ADR | Subject | §Decision + §Consequences body lands at |
|---|---|---|
| **ADR-0199 (FULL)** | `RateLimitPerRoute` NEW 10th canonical per-route shape ("data-only-with-vh-inclusion-enum") + ADR-0125 §(xv) roster amendment 9 → 10 — `vh_rate_limits` (OVERRIDE/INCLUDE/IGNORE) drives cross-tier descriptor composition; `override_option` PARSE-ACCEPTED-but-IGNORED (INERT `[#not-implemented-hide:]` per AMEND-4); route-additional `rate_limits[]` Axis-A early-return; per-route `domain` override. RE-AMENDS after phase-23's REUSE-by-absence skip; ENDS the 2-row deferral streak (phase-23 + phase-24.1). | **Task 3** (`compiled_perroute.go` + ADR-0125 amendment) |
| **ADR-0197 IN-PLACE §Decision amendment** | X-RateLimit DRAFT_VERSION_03 headers (`x-ratelimit-limit`/`x-ratelimit-remaining`/`x-ratelimit-reset`; MIN-status across multi-descriptor responses + `;w=`/`;name=` quota-policy suffix + unit→seconds map; emitted on ALL dispositions when enabled per AMEND-8) + descriptor-engine completion to the FULL 10 actions (the remaining 5 from 24.1's `actionUnsupportedAt241` stub). The in-place edit extends ADR-0197 §Decision per ADR-0052 amendment discipline (not a new ADR). | **Task 5** (`encode.go` + `headers.go`) |

---

## Conditional ADR-0202 escape-valve mapping

Reproduced verbatim from PLAN.md `## ADRs introduced/landed by this plan` (the ADR-0202 escape-valve reserve subsection):

**ADR-0202 escape-valve reserve (UNCONSUMED at 24.1 phase-done — D-hypothesis HELD; carries forward to 24.2 IMPL).** The parent SPEC §12 item-1 highest-risk byte-confirmation (the DELTA-2 chain-seed type + accessor return-type) RESOLVED at 24.1 Task 5 as RAW-PROTO SEED CONFIRMED; the escape valve did NOT fire. Phase 24's remaining D-hypothesis firing surfaces lie in 24.2:

- **Task 1 — `metadata`-action dynamic-metadata accessor surface (parent §12 item 2).** The `streamInfo().dynamicMetadata()` (DYNAMIC=0) vs route-metadata (ROUTE_ENTRY=1) accessor chain. PLAN hypothesis: the existing stream-info `DynamicMetadata()` accessor is sufficient (no Lua-bridge `dynmd` extension needed at 24.2 — the `metadata` action's read-only dynamic-metadata access matches the phase-22.2 lua-bridge READ path's exposure). If the existing accessor cannot satisfy the `metadata` action's segmented `metadata_key` lookup (the `Metadata::metadataValue` chain over the `MetadataKey.path` segments), fire ADR-0202 (§Context + §Decision + §Consequences at the Task-1 commit per ADR-0044).
- **Task 5 — X-RateLimit MIN-status quota-policy byte-edge (parent §12 item 5).** The exact `, <rpu>;w=<sec>[;name="<n>"]` concatenation + the MIN `limit_remaining` selection across multi-descriptor responses + the unit→seconds mapping (parent §4.7 + AMEND-8). PLAN hypothesis: the upstream `ratelimit_headers.cc:13-65` byte format is reproducible without divergence; the `headers_test.go` byte-pin against captured upstream headers settles the edge cases. If the byte format diverges (e.g., quoting discipline on `name=` with embedded characters; or `MIN_status` selection tie-breakers on equal `limit_remaining`), fire ADR-0202.

PLAN hypothesis: **ADR-0202 stays UNCONSUMED at phase-24 phase-done — HOLD-with-known-risk**. If fired, all the body lands at the firing Task's commit per ADR-0044. Next-free ADR after 24.2 phase-done advances `ADR-0202` (unconsumed) → `ADR-0203` if-and-only-if fired, else stays at ADR-0202.

---

## Planner-time deferred-decision resolutions (D-RL8..D-RL18 + D-P1..D-P3)

Reproduced VERBATIM from PLAN.md `## Planner-time deferred-decision resolution` so the IMPL Tasks 1–8 subagents inherit them as the contract:

- **D-RL8 (`metadata` action's value-extraction accessor; parent §12 item 2 + AMEND-11).** **RECOMMENDED:** use the existing `streamInfo().DynamicMetadata()` accessor (already exposed on `DecoderFilterCallbacks` for the phase-22.2 lua-bridge READ path) for `MetadataSource_DYNAMIC=0`; use the matched route's `RouteEntry.Metadata()` accessor for `MetadataSource_ROUTE_ENTRY=1`. The segmented `MetadataKey.path` chain (each `key` is one segment) descends via `proto.Message → google.protobuf.Struct → google.protobuf.Value` per the upstream `Metadata::metadataValue` reference at `source/common/config/metadata.cc`. If the existing stream-info accessor does NOT expose a `Metadata` accessor for ROUTE_ENTRY (Task 1's first action confirms), the 24.1 DELTA-2 set-once-by-dispatch pattern (ADR-0165 / ADR-0198) extends — add a `RouteMetadata()` `DecoderFilterCallbacks` accessor (seeded at HCM dispatch alongside the `RouteRateLimits()` plumbing). If neither path fits cleanly, fire ADR-0202 (the §12-item-2 surface — escape-valve target #1).
- **D-RL9 (X-RateLimit DRAFT_VERSION_03 byte format; parent §12 item 5 + AMEND-8).** **RECOMMENDED:** reproduce the upstream `ratelimit_headers.cc:13-65` byte format verbatim: `x-ratelimit-limit: <MIN.requests_per_unit>[, <rpu>;w=<window_sec>[;name="<n>"]]...` (MIN selection by `limit_remaining`; quota-policy suffix per descriptor with non-zero window; comma-separated descriptor segments; `;name=` value quoted per upstream); `x-ratelimit-remaining: <MIN.limit_remaining>`; `x-ratelimit-reset: <MIN.duration_until_reset.seconds>`. Unit→seconds: SECOND=1, MINUTE=60, HOUR=3600, DAY=86400, WEEK=604800, MONTH=2592000, YEAR=31536000, UNKNOWN/0 ⇒ no quota-policy segment for that descriptor. Byte-pinned at Task 5 `headers_test.go` against captured upstream headers (cross-side scenario (g) at Task 6 provides the final verification). MIN-selection tie-breakers (equal `limit_remaining`): preserve insertion order (= descriptor-list order = action-list order per AMEND-6) — the FIRST equal-minimum status wins. If the byte format diverges (e.g., quoting of `name=` with embedded special characters; or rounding rules on fractional `duration_until_reset.seconds`), fire ADR-0202 (the §12-item-5 surface — escape-valve target #2).
- **D-RL10 (`RateLimitPerRoute` TPFC compiler registration path).** **RECOMMENDED:** register the `RateLimitPerRoute` TPFC compiler in `internal/filter/hcm/` alongside the existing per-route TPFC registrations (mirror the `header_mutation`/`oauth2`/`lua`/`cors` per-route compilation precedents — each filter registers a typed unmarshal for its per-route shape against the TPFC dispatch keyed by TypeURL per ADR-0073). The compiler validates the per-route message (`RateLimitPerRoute.override_option` accepted-but-IGNORED per AMEND-4; `vh_rate_limits` enum bounds; `rate_limits[]` recursively validated via the EXISTING `ratelimit.ValidateRouteRateLimits` exported validator from 24.1 Task 3) and produces a `*compiledPerRoute` opaque type. The filter consumes it via `f.dcb.RequestRouteConfig()` per the standard TPFC resolver path. The TypeURL: `"type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimitPerRoute"` (byte-stable per ADR-0143 SN1).
- **D-RL11 (Axis-A vs Axis-B precedence at request time).** Reaffirms parent §4.3 AMEND-4 + AMEND-5: per-request walk is **Axis-A first (per-route `RateLimitPerRoute.rate_limits[]` non-empty ⇒ early-return, walk ONLY that list)**; otherwise **Axis-B (route policy + the `vh_rate_limits`-conditional vhost policy)**. `RateLimitPerRoute.override_option` is PARSE-ACCEPTED-but-INERT per AMEND-4 (NEVER read at request-time); the upstream-parity decision is locked. The legacy `RouteAction.include_vh_rate_limits=true` (parent §4.3) forces INCLUDE regardless of the `vh_rate_limits` enum. The per-route `domain` (`RateLimitPerRoute.domain`) overrides the filter-config `domain` when set; absent ⇒ filter-config domain.
- **D-RL12 (X-RateLimit emission scope per disposition).** Parent §4.7 + AMEND-8: X-RateLimit headers (when `enable_x_ratelimit_headers == DRAFT_VERSION_03`) emit on **ALL dispositions** (OK + OVER_LIMIT + error). The error/fail-closed reply path uses a **nullptr mutate-callback** per AMEND-8 — the 24.2 implementation honors this by NOT emitting X-RateLimit on the fail-closed `SendLocalReply` (the response is constructed without the encoder hook participating); a 24.1 `dispositions.go` STUB pin (no X-RateLimit on fail-closed) is retained. X-RateLimit DOES emit on the fail-OPEN path (continue-downstream ⇒ encoder hook runs ⇒ X-RateLimit injected).
- **D-RL13 (scenario (f) `vh_inclusion` dispatch).** Cross-side byte-exact via `CompareBytes` per the existing `0032` fixture's runner branch. The cross-side scenario exercises INCLUDE / OVERRIDE / IGNORE with both a route policy AND a vhost policy in play — the descriptors emitted to the fake `RateLimitService` differ per the §4.3 table; both sides MUST emit the same set; the fake returns OK uniformly so the cross-side response is also byte-exact.
- **D-RL14 (scenario (g) `x_ratelimit_headers` dispatch).** Cross-side byte-exact via `CompareBytes` per the existing `0032` runner branch. The fake `RateLimitService` returns per-descriptor statuses with `current_limit`/`limit_remaining`/`duration_until_reset` populated; both sides MUST emit byte-exact `x-ratelimit-*` headers per the AMEND-8 format. Per the X-RateLimit response-header allow-list extension at Task 8 (BEHAVIOR_CONTRACT §7.2), the headers are in the documented set-equal discipline (NO ignore-list — these are byte-exact).
- **D-RL15 (scenario (d) `descriptor_actions` extension).** Cross-side byte-exact (same dispatch as 24.1's `descriptor_actions` row). The extension covers all 10 actions: 24.1 covered `generic_key`/`request_headers`/`remote_address`/`destination_cluster`/`header_value_match`; 24.2 ADDS `source_cluster`/`masked_remote_address`/`metadata`/`query_parameters`/`query_parameter_value_match`. The fake asserts descriptor-set equality across the two sides (per the `0032` driver's existing cross-side discipline + the 24.1 (d-core) precedent).
- **D-RL16 (fuzzer corpus extension — no new fuzzer).** The existing `FuzzRateLimitConfigParse` gets ~10-20 NEW seeds: each of the 5 remaining action arms; `RateLimitPerRoute` with each `vh_rate_limits` value + with `override_option` set (PARSE-ACCEPTED-but-IGNORED arm exercise); `stage` boundary arms (0, 5, 10 — already arm `>10`); the legacy `include_vh_rate_limits=true` arm. Project fuzzer count stays at **33** (no new fuzzer).
- **D-RL17 (BEHAVIOR_CONTRACT bundle completion at atomic-landing).** Per ADR-0052 atomic-landing discipline + parent §13 bundle:
  - (1) the `### envoy.filters.http.ratelimit` subsection EXTENDS (engine completion to all 10 actions + the `metadata` accessor disposition + the stage multi-bucket discipline + the Axis-B `vh_rate_limits` decision table + the X-RateLimit DRAFT_VERSION_03 emission discipline + the per-route `RateLimitPerRoute.domain` override + `RateLimitPerRoute.override_option` accepted-but-INERT departure note);
  - (2) the `## Stat-name mapping` 4-counter section gets a per-route `domain`-qualifier paragraph (per AMEND-1: when a per-route `domain` is set, the stat names are UNCHANGED — `domain` is a descriptor-tier override, not a stat namespace; 110 → 114 stays);
  - (3) the per-route canonical-patterns cross-reference caption updates "through phase 24.1" → "through phase 24" and the §(xv) AMENDMENT 9 → 10 paragraph documents the `RateLimitPerRoute` 10th canonical;
  - (4) the response-header allow-list paragraph adds `x-ratelimit-limit` + `x-ratelimit-remaining` + `x-ratelimit-reset` (set-equal byte-exact per scenario (g)). No new departure records (the 3 from 24.1 already cover the only envoy-go-strict departures at the 24.2 surface; `override_option` accepted-but-INERT is upstream-parity, NOT a departure).
- **D-RL18 (atomic-landing ROADMAP rollup).** Per the 18/19/22 sub-phase rollup precedent: the 24.2 phase-done commit flips BOTH row 24.2 (`planned → done`) AND parent row 24 (`in-progress → done`) in ONE commit, and the commit-message body names both transitions for grep-verifiability ("phase 24.2: ... [ADR-0199, ADR-0197-amend, ADR-0125 §(xv)] — also closes parent row 24 [ROLLUP per 18/19/22 precedent]"). STATE.md re-advances per BOOTSTRAP §4.1 to whatever follows phase 24 in §9 (after this phase-done, the §9 HTTP-filters family closes to **1 remaining row: `wasm`** — STATE points at the next family member OR "awaiting next planning" if no §9 row is next-due).
- **D-P1 (task numbering).** Pre-Task 0 (PROGRESS preamble + preconditions) is the ritual prefix; the functional tasks are Tasks 1–8. Each Task maps 1:1 to a PROGRESS.md entry (D-P3).
- **D-P2 (subagent dispatch).** Per `superpowers:subagent-driven-development`, each Task is dispatched to a fresh `general-purpose` subagent with the Task's dispatch outline + a two-stage review between Tasks.
- **D-P3 (PROGRESS discipline).** Each Task appends a PROGRESS.md entry quoting the six-gate-relevant command outputs verbatim + the commit SHA.

---

## Deviations from PLAN literal commands

- **Precondition 12 grep over-specification.** The PLAN's literal compound `grep` in Precondition 12 matches doc-comment text + a fixture-README forward-reference rather than 24.2 implementation symbols. The semantic intent (no NEW 24.2 surface landed at IMPL cold-start) is verified by the disaggregation in the Precondition-12 block above (three FILE-existence sub-checks PASS; the five action arms all still dispatch to `actionUnsupportedAt241()`; no `func actionSourceCluster|…` definitions exist). Mechanical PLAN-grep over-specification, not a semantic precondition violation. Recorded so the next PLAN revision can refine the regex (e.g., `^func actionSourceCluster\b` to anchor to function-definition lines). Parallel to the 24.1 PROGRESS §Deviations entry on the Precondition-10 `-run` first-segment-anchoring fix.

---

## Task entries

### Pre-Task 0 — PROGRESS.md preamble + 12-precondition verification

- **Commit:** _(self-reference; see `git log -1 --oneline phase-24.2-global-ratelimit-perroute-and-headers-impl` after this commit lands — the pre-amend SHA was `3ca3858d8301df8f350a6dd4e48f23f081465f6c`; the `--amend` that self-filled this entry to its final form alters the SHA per git's content-addressed model, so the FINAL Pre-Task-0 SHA is recoverable only via `git log` post-amend, mirroring the 24.1 PROGRESS Pre-Task 0 precedent of using a `_(self-reference)_` deferral rather than an inline literal SHA).
- **Files touched:** `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md` (created, this file).
- **Gates:** all 12 preconditions verified green (1 semantic-pass on the PLAN-grep over-specification per the §Deviations note); verbatim outputs recorded above.
- **Outcome:** PROGRESS.md preamble committed; the 2-ADR landing table + conditional ADR-0202 escape-valve mapping + D-RL8..D-RL18 + D-P1..D-P3 are now the inheritable contract for Tasks 1–8 subagents. Ready for Task 1 (`descriptors.go` 5 remaining actions + AMEND-11 + `metadata` accessor; ADR-0202 escape-valve target #1).

### Task 1 — `descriptors.go` 5 remaining actions + AMEND-11 + `metadata` accessor (D-RL8)

- **Commit:** `d24e387` (pre-amend SHA, captured by `git log -1 --oneline` after the initial commit landed; the post-amend SHA will differ if this entry's self-reference fill-in is performed via `git commit --amend`, mirroring the Pre-Task 0 deferral discipline).

- **D-RL8 survey outcome (recorded verbatim).** The §12 item-2 byte-confirmation surface (the `metadata` action's value-extraction accessor) RESOLVED at IMPL: the existing `DecoderFilterCallbacks.DynamicMetadata() *dynamicmetadata.Bucket` accessor (callbacks.go:239) satisfies `MetadataSource_DYNAMIC=0` via `Bucket.Get(filterName, topKey) → *structpb.Value` followed by segmented descent through `*structpb.Value.GetStructValue().Fields[seg]` (the `Metadata::metadataValue` upstream reference at `source/common/config/metadata.cc`). The `MetadataSource_ROUTE_ENTRY=1` path required a NEW accessor: added `RouteMetadata() *corev3.Metadata` to `DecoderFilterCallbacks` via the established ADR-0165 set-once-by-dispatch + ADR-0198 24.1 DELTA-2 chain-field plumbing template (chain field + setter + chain accessor + decoderCB accessor + HCM-dispatch seed in both connection.go H1 and h2dispatch.go H2). **ADR-0202 UNCONSUMED** — the extension was a clean ADR-0165 extension; no escape-valve firing required. Survey-then-decide protocol per PLAN §D-RL8 honored.

- **Plumbing surface added (D-RL8 + AMEND-11).**
  - `internal/filter/http/callbacks.go`: added `RouteMetadata() *corev3.Metadata` to the `DecoderFilterCallbacks` interface; documented as the FIRST exposure of route-level `*corev3.Metadata` to a filter (parallel to `RouteRateLimits()` per ADR-0198 §Decision).
  - `internal/filter/http/chain.go`: added `routeMetadata *corev3.Metadata` chain field + `SetRouteMetadata(md)` setter + `RouteMetadata()` chain-level accessor + `decoderCB.RouteMetadata()` callback impl.
  - `internal/filter/http/types.go`: added `NodeServiceCluster string` to `FactoryCtx` (nil-tolerant per ADR-0085) — phase 24.2 Task 1 first-use anchor for the `source_cluster` descriptor action (consumes the Envoy NODE's `service-cluster` name).
  - `internal/listener/manager.go`: extended `listenerCtx` with `nodeServiceCluster string`; extracted `bs.GetNode().GetCluster()` once at `NewManagerWithBaseDirAndAllowH2C` and threaded through `buildListenerRuntimeWithCtx` → `buildTerminalFilter` → both `listenerCtx{...}` struct literals → `hcm.ListenerCtx`.
  - `internal/filter/hcm/config.go`: extended `hcm.ListenerCtx` with `NodeServiceCluster string`; extended `parseHTTPFiltersChain` signature to accept + pass it into `filter_http.FactoryCtx.NodeServiceCluster`.
  - `internal/filter/hcm/route.go`: extended `routeEntry` with `metadata *corev3.Metadata` (captured at `buildRouteTable` via `r.GetMetadata()`).
  - `internal/filter/hcm/connection.go` + `internal/filter/hcm/h2dispatch.go`: HCM dispatch now calls `chain.SetRouteMetadata(entry.metadata)` BEFORE `RunDecodeHeaders` (mirroring the 24.1 DELTA-2 `SetRouteRateLimits` seed-and-read discipline).

- **Engine surface added (descriptors.go).**
  - Introduced `descriptorInputs` struct (the new 9-field input bundle: `routeRateLimits` + `vhostRateLimits` + `headers` + `remoteAddr` + `clusterName` + `nodeServiceCluster` + `rawQuery` + `dynamicMetadata` + `routeMetadata`) + `buildDescriptorsExt(in descriptorInputs)` as the new full-surface entry-point. The legacy `buildDescriptors(...)` 5-arg signature is preserved as a backwards-compatible wrapper (zero values for the new 4 inputs) so the 24.1 unit tests + the `d-core` differential fixture continue to compile unchanged.
  - Added 5 new action helpers: `actionSourceCluster` (always-true, empty-value entry survives), `actionMaskedRemoteAddress` (v4/v6 CIDR mask via `net.CIDRMask` + `net.IP.Mask`, defaults 32/128, nil-addr drops), `actionMetadata` (segmented descent via `resolveMetadataValue` + `descendStructpbValue` — terminal must be `*structpb.Value_StringValue` per upstream `metadataValue`; default_value + skip_if_absent honored per parent §4.1 row 8 / rl.cc:187-227), `actionQueryParameters` (first-value via `url.ParseQuery`; AMEND-11 default key `"query_param"` SINGULAR), `actionQueryParameterValueMatch` (AND-fold matchers via `evaluateAllQueryParameterMatchers` + `evaluateOneQueryParameterMatcher`; default key `"query_match"`).
  - REMOVED `actionUnsupportedAt241()` stub function.
  - REMOVED all 5 `case ... return actionUnsupportedAt241()` arms in the `applyAction` switch; each new arm calls its real helper.
  - Added AMEND-11 key default consts: `descriptorKeySourceCluster = "source_cluster"`, `descriptorKeyMaskedRemoteAddress = "masked_remote_address"`, `descriptorKeyQueryParametersDefault = "query_param"` (SINGULAR per AMEND-11 — easy to typo), `descriptorKeyQueryParameterValueMatchDefault = "query_match"`. Also added `defaultV4PrefixMaskLen = 32` / `defaultV6PrefixMaskLen = 128` for the masked_remote_address arm.

- **decode_headers.go production wiring.** Replaced the positional `buildDescriptors(routeRLs, vhostRLs, headers, remoteAddr, clusterName)` call with `buildDescriptorsExt(descriptorInputs{...})`, populating the 4 new fields from `f.dcb.DynamicMetadata()` / `f.dcb.RouteMetadata()` / `f.cc.getNodeServiceCluster()` / `extractRawQueryFromPath(headers)` (the `:path` pseudo-header → split on `'?'`). Added the `extractRawQueryFromPath` helper + the nil-safe `compiledConfig.getNodeServiceCluster()` accessor.

- **compiled_config.go production wiring.** Added `nodeServiceCluster string` to `compiledConfig`; populated from `ctx.NodeServiceCluster` at `buildCompiledConfig`'s post-validation cc-population step. Empty string is the documented nil-passthrough (production listeners that lack a bootstrap `node.cluster` field, OR test paths that pass `FactoryCtx{}`).

- **Fakes that implement DecoderFilterCallbacks.** Added `RouteMetadata() *corev3.Metadata { return nil }` to every fake `DecoderFilterCallbacks` impl in the repo (17 files across `header_mutation` / `callbacks_test` / `bandwidthlimit` / `buffer` / `csrf` / `fault` / `adaptive_concurrency` / `oauth2` / `localratelimit` / `admission_control` / `jwtauthn` / `rbac` / `compressor` / `lua` / `extproc` / `extauthz` / `ratelimit`). Added the `corev3` import to the 6 files that did not already have it. The interface extension is otherwise zero-cost for filters that do not consume route-metadata (mirrors the ADR-0165 nil-passthrough discipline).

- **Tests added (descriptors_test.go; ~800 LoC).** All TDD — failing tests landed first, then engine + action helpers, then verified GREEN. New tests:
  - `TestDescriptors_PerAction_SourceCluster` (2 sub-rows: present node + empty node ⇒ empty-value entry; always-true)
  - `TestDescriptors_PerAction_MaskedRemoteAddress` (7 sub-rows: v4 mask 24/32-default/0; v6 mask 64/128-default; nil addr drops; non-TCPAddr drops)
  - `TestDescriptors_PerAction_Metadata_Dynamic` (7 sub-rows: top-level string; segmented descent; default_value fallback; absent+skip_if_absent=false drops; absent+skip_if_absent=true survives via paired action; non-string leaf treated as absent; empty descriptor_key drops)
  - `TestDescriptors_PerAction_Metadata_RouteEntry` (3 sub-rows: top-level string; default_value fallback; absent drops)
  - `TestDescriptors_PerAction_QueryParameters` (6 sub-rows: default key SINGULAR `"query_param"`; configured key; first value when multiple; absent drops; absent+skip survives via paired action; empty rawQuery drops)
  - `TestDescriptors_PerAction_QueryParameterValueMatch` (5 sub-rows: default key `"query_match"`; configured key; expect_match=true+absent drops; expect_match=false+absent emits; empty descriptor_value drops)
  - `TestDescriptors_AMEND11_KeyDefaults_ByteStable` (8 sub-rows: byte-exact pins for all AMEND-11 default key strings — generic_key, header_match, remote_address, destination_cluster, source_cluster, masked_remote_address, query_param SINGULAR, query_match)
  - `TestDescriptors_EmptyActionDrop_Extended` (2 sub-rows: §4.5 behaviors on the new arms — masked_remote_address nil-addr drops whole descriptor; source_cluster empty-node emits empty-value entry, descriptor survives)
  - `TestDescriptors_BackwardCompatibility_5ArgWrapper` (1 row: confirms legacy `buildDescriptors(...)` 5-arg signature still routes correctly through the new `descriptorInputs` struct)

- **Gates (verbatim).**
  - `go build ./...` — empty output (clean). EXIT=0.
  - `go vet ./...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./internal/filter/http/... ./internal/filter/hcm/... ./internal/listener/...` — empty output (clean). EXIT=0.
  - `go test -race -count=1 -run 'TestDescriptors' ./internal/filter/http/ratelimit/` — `ok github.com/esalaine/envoy-go/internal/filter/http/ratelimit 1.014s` (8 NEW test functions + 4 PRE-EXISTING all PASS, no -v breakdown captured by default; with -v the sub-row count = 41 PASS across the 8 NEW tests + the original 24.1 TestDescriptors_* rows).
  - `go test -race -count=1 ./internal/filter/http/ratelimit/` — `ok github.com/esalaine/envoy-go/internal/filter/http/ratelimit 1.025s` (full package PASS).
  - `go test -count=1 -short ./...` — `ok: 64 FAIL: 0 no-test-files: 49` (matches Pre-Task 0 baseline; no regressions across the 64 short-suite packages).
  - 24.1 d-core differential fixture regression check: `go test -count=1 -timeout 5m ./test/differential/ -run 'TestDifferential/0032-http-ratelimit'` — `--- PASS: TestDifferential/0032-http-ratelimit (1.92s)`. The 24.1 `descriptor_actions` d-core cross-side scenario still GREEN.

- **Outcome.** All 10 canonical descriptor actions (5 CORE + 5 REMAINING) now implemented and tested under the §4.5 empty-action-drop discipline; AMEND-11 key defaults byte-stable; `metadata` action handles BOTH MetadataSource arms via the existing `DynamicMetadata()` Bucket (DYNAMIC=0) + the NEW `RouteMetadata()` accessor (ROUTE_ENTRY=1); D-RL8 RESOLVED with clean ADR-0165 extension, ADR-0202 UNCONSUMED. Ready for Task 2 (`stage` multi-stage bucketing path).

#### Task 1 follow-up — code-quality review I-1 + I-2 + I-3 (small coverage/doc gaps)

Addressed 3 Important findings from the post-Task-1 code-quality review before Task 2 starts. Strictly in-scope, TDD-disciplined: new test rows landed FIRST; all PASSED immediately (helper logic already correct — no real bugs exposed). Minor findings M-1..M-5 explicitly OUT OF SCOPE for this follow-up.

- **I-1 (docstring clarification).** Extended the doc-comment above `resolveMetadataValue` in `internal/filter/http/ratelimit/descriptors.go` with a 12-line "DYNAMIC vs ROUTE_ENTRY asymmetry on the (filterName, path[0]) coordinate" paragraph. Explains the root cause: envoy-go's pre-existing `*dynamicmetadata.Bucket` storage shape is a FLAT `(filterName, key) → *structpb.Value` map (versus upstream's `map<string, google.protobuf.Struct>` — one Struct per filter), so DYNAMIC decomposes the first two coordinates at the Bucket boundary while ROUTE_ENTRY descends `path[0]` as the first FIELD of the per-filter Struct. Documents the producer contract: `bucket.Set(filterName, topField, value)` with `value` a Struct mirroring the per-filter `Struct.Fields` shape ⇒ the two arms are semantically equivalent at the shared `descendStructpbValue` join-point.

- **I-2 (DYNAMIC non-Struct intermediate descent — missing test row).** Added `TestDescriptors_PerAction_Metadata_Dynamic/Intermediate_NonStruct_TreatedAsAbsent`. Covers the `cur.GetStructValue() == nil` arm of `descendStructpbValue` (descriptors.go:962-964): bucket = `(filterName, "user") → Number(42)`, path = `[{user}, {tier}]`. `resolveMetadataValue` returns the Number Value at path[0]; `descendStructpbValue` then iterates path[1:]=`[{tier}]` and calls `cur.GetStructValue()` on the Number — returns nil ⇒ chain breaks ⇒ absent ⇒ no default_value + skip_if_absent=false ⇒ whole descriptor drops. PASSED immediately.

- **I-3 (ROUTE_ENTRY parallel-coverage rows — 2 added).** Parallels the DYNAMIC coverage and verifies the ROUTE_ENTRY-side dispatch is exercised through the shared `descendStructpbValue` helper.
  - `TestDescriptors_PerAction_Metadata_RouteEntry/PresentNestedSegmentedDescent` — `route.metadata.filter_metadata[filterName]` = `Struct{user: Struct{tier: "gold"}}`, path = `[{user}, {tier}]` ⇒ emits `(user_tier, "gold")`. Confirms `descendStructpbValue` handles the nested Struct→Struct→String descent identically for ROUTE_ENTRY.
  - `TestDescriptors_PerAction_Metadata_RouteEntry/PresentNonStringLeaf_SkipIfAbsentTrue_SkipsEntry_DescriptorSurvives` — `route.metadata.filter_metadata[filterName]` = `Struct{tier: Number(42)}` paired with a peer `generic_key` action; the metadata action has `skip_if_absent=true` ⇒ non-string leaf treated as absent ⇒ ONE entry skipped ⇒ descriptor survives via the `generic_key` peer (§4.5 behavior (2)).

- **Test row count after follow-up.**
  - `TestDescriptors_PerAction_Metadata_Dynamic`: 7 → 8 sub-rows (added I-2).
  - `TestDescriptors_PerAction_Metadata_RouteEntry`: 3 → 5 sub-rows (added I-3 × 2).

- **Gates (verbatim, post follow-up).**
  - `go build ./...` — empty output (clean). EXIT=0.
  - `go vet ./...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./internal/filter/http/ratelimit/...` — empty output (clean). EXIT=0.
  - `go test -race -count=1 -run 'TestDescriptors' ./internal/filter/http/ratelimit/` — `ok github.com/esalaine/envoy-go/internal/filter/http/ratelimit 1.015s`.
  - `go test -race -count=1 -v -run 'TestDescriptors_PerAction_Metadata' ./internal/filter/http/ratelimit/` — `PASS` on all 13 sub-rows (8 Dynamic + 5 RouteEntry).

- **Files changed (3 exactly).**
  - `internal/filter/http/ratelimit/descriptors.go` — I-1 docstring (12-line addition above `resolveMetadataValue`).
  - `internal/filter/http/ratelimit/descriptors_test.go` — I-2 (1 sub-row) + I-3 (2 sub-rows) = 3 new sub-rows.
  - `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md` — this sub-section.

- **Outcome.** All 3 Important code-quality findings resolved; coverage parity DYNAMIC ↔ ROUTE_ENTRY restored at the shared helper join-point; consumer-contract asymmetry documented in-source. No engine code touched. Ready for Task 2.

### Task 2 — `stage` multi-stage bucketing path (§4.4)

- **Commit:** `f118757` (pre-amend SHA, captured by `git log -1 --oneline` after the initial commit landed; the post-amend SHA will differ if this entry's self-reference fill-in is performed via `git commit --amend`, mirroring the Task 1 deferral discipline).

- **Scope.** Lands the §4.4 multi-stage bucketing path. 24.1 evaluated ONLY the default stage-0 bucket via a hardcoded `if st.GetValue() != 0 { continue }` filter in `buildDescriptorsExt`; phase-24.2 Task 2 generalizes — a filter configured at `stage=N` walks ONLY policies whose `RateLimit.Stage.Value == N` (the upstream `getApplicableRateLimit(stage)` semantics; rl.cc:539-550). Strictly in-scope per the PLAN Task 2 outline; no behavioral change for stage=0 callers (the 24.1 baseline is preserved as the filterStage=0 row of the generalization).

- **Engine surface changes (descriptors.go).**
  - Replaced the 24.1 stage-0-only filter (`if st := p.GetStage(); st != nil && st.GetValue() != 0 { continue }`) with a per-request bucket-selection filter (`if p.GetStage().GetValue() != in.filterStage { continue }`) inside `buildDescriptorsExt`. The check is nil-safe (a nil `*wrapperspb.UInt32Value` yields 0 — bucketed as stage 0).
  - Added a new `filterStage uint32` field to the `descriptorInputs` struct (the engine's per-request input bundle). Default 0 ⇒ stage-0 bucket walked (24.1 baseline preserved). The pre-existing 5-arg backwards-compat wrapper `buildDescriptors(...)` zero-values the field — the d-core differential fixture + the 24.1 unit tests continue to compile + behave unchanged.
  - Updated the `# Stage filtering` file-header comment block (10 lines added) to document the §4.4 generalization, the parse-time PARSE-REJECT pair (filter envelope at `buildCompiledConfig` + per-policy at `ValidateRouteRateLimits`), and the equivalence `filterStage==0 ≡ 24.1-baseline`.

- **Parse-time surface changes (compiled_config.go).**
  - Added `bucketRateLimitsByStage(rls []*routev3.RateLimit) [maxStage+1][]*routev3.RateLimit` — the §4.4 parse-time bucketing helper. Partitions a slice into 11 slots (the upstream `MAX_STAGE_NUMBER+1 = 11` invariant) indexed by `policy.GetStage().GetValue()` (nil ⇒ 0). Out-of-range policies (stage > maxStage) are defensively SKIPPED (they would have been PARSE-REJECTed by `ValidateRouteRateLimits` upstream). The engine at request time is functionally equivalent to consulting `buckets[filterStage]` via the per-policy stage equality check — the helper is exported into the test surface for the parse-time bucket-occupancy pin; the engine does NOT pre-allocate the 11 buckets at runtime (the per-iteration equality check is more efficient than building + discarding 10 unused slots per request).
  - Added the per-policy `stage > 10` PARSE-REJECT arm to `ValidateRouteRateLimits` (the §5.1 Arm 3 mirror at the route/vhost RateLimit policy level — upstream PGV `lte:10` on `config.route.v3.RateLimit.stage`). Reuses the byte-stable `parseRejectStageTooHigh` const ("ratelimit: stage must be <= 10") — same wording as the filter-envelope arm at `buildCompiledConfig`. Doc-comment extended with the §4.4 + §5.1 cross-references.
  - Added the nil-safe `(c *compiledConfig).getStage() uint32` accessor (mirroring `getNodeServiceCluster` Task 1 precedent). Returns 0 when `c == nil` (synthetic-stream test path nil-tolerance per ADR-0085).
  - Removed the `//nolint:unused` suppression on `compiledConfig.stage` — the field is now consumed at request time via `getStage()`. Doc-comment updated to point at the new consumer.

- **Production wiring (decode_headers.go).** Threaded `filterStage: f.cc.getStage()` into the `buildDescriptorsExt(descriptorInputs{...})` call at the production decode-headers path. A 3-line inline doc-comment anchors the §4.4 cross-reference + the default-0 ⇒ 24.1-baseline equivalence.

- **Tests added.**
  - In `descriptors_test.go` (~250 LoC, 8 new test functions): `TestDescriptors_StageFilter_DefaultStageZero` (route stage-0 + stage-3; filter=0 walks only stage-0), `TestDescriptors_StageFilter_NonZeroStage` (route stage-3 + stage-5; filter=5 walks only stage-5), `TestDescriptors_StageFilter_AllBucketsEmpty` (route stages 3+5; filter=7 ⇒ zero descriptors), `TestDescriptors_StageFilter_NilStageEqualsStageZero` (2 sub-rows: nil-stage policy walked at filter=0; skipped at filter=3), `TestDescriptors_StageFilter_VhostBucket` (the stage filter ALSO applies to the vhost-walked path at route-empty), `TestDescriptors_StageFilter_MultiplePoliciesSameStage` (filter=4 walks both stage-4 policies; stage-2 skipped — policy-order preserved within bucket), `TestDescriptors_StageFilter_MaxStage10` (the upper bound; stage-10 walked, stage-0/9 skipped), `TestBuildCompiledConfig_StageBucketing_ParseTime` (the 11-bucket occupancy pin: stages 0/3/5/10 + a nil-stage compile into bucket[0]=2, bucket[3]=1, bucket[5]=1, bucket[10]=1, all others empty; nil/empty input ⇒ all-empty 11-slot), `TestBuildCompiledConfig_StageBucketing_OutOfRangeSkipped` (defensive — a stage=11 policy is skipped by the bucketer).
  - In `compiled_config_test.go` (4 new sub-rows added to `TestValidateRouteRateLimits`): `Stage_TooHigh_PerPolicy_11` + `Stage_TooHigh_PerPolicy_42` (both fire `parseRejectStageTooHigh`); `Stage_AtBound10_Pass` + `Stage_AtBound0_Pass` (happy-path bounds). Added the `wrapperspb` import for the per-policy stage stamping.

- **Gates (verbatim).**
  - `go build ./...` — empty output (clean). EXIT=0.
  - `go vet ./...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./internal/filter/http/ratelimit/...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./...` — empty output (clean). EXIT=0 (full-repo sweep — no broader regressions).
  - `go test -race -count=1 -v -run 'TestDescriptors_StageFilter|TestBuildCompiledConfig_StageBucketing' ./internal/filter/http/ratelimit/` — `PASS` on 11 sub-rows (DefaultStageZero, NonZeroStage, AllBucketsEmpty, NilStageEqualsStageZero×2, VhostBucket, MultiplePoliciesSameStage, MaxStage10, BucketingParseTime, BucketingOutOfRangeSkipped). `ok github.com/esalaine/envoy-go/internal/filter/http/ratelimit 1.016s`.
  - `go test -race -count=1 -run 'TestValidateRouteRateLimits' ./internal/filter/http/ratelimit/` — `PASS` on 11 sub-rows (the 7 pre-existing + the 4 NEW Stage_* rows added at this Task).
  - `go test -race -count=1 ./internal/filter/http/ratelimit/` — `ok github.com/esalaine/envoy-go/internal/filter/http/ratelimit 1.028s` (full package PASS).
  - `go test -count=1 -short ./...` — no FAIL rows (baseline preserved across all 64 short-suite packages).
  - 24.1 fixtures regression: `go test -count=1 -timeout 5m -v ./test/differential/ -run 'TestDifferential/003[23]'` — `--- PASS: TestDifferential/0032-http-ratelimit (1.94s)` + `--- PASS: TestDifferential/0033-http-ratelimit-boot-reject (1.32s)`. Both Gate-D anchor fixtures still GREEN.

- **Outcome.** §4.4 multi-stage bucketing path landed: filter at `stage=N` walks ONLY policies in the matching bucket. The 24.1 default-stage-0 baseline behavior is preserved at `filterStage=0`. Per-policy `stage > 10` PARSE-REJECT now mirrors the filter-envelope arm at the route/vhost RateLimit level (byte-stable wording reuse). The `compiledConfig.stage` field is no longer `//nolint:unused`. All 24.1 differential fixtures still GREEN. Ready for Task 3 (`compiled_perroute.go` + ADR-0199 FULL + ADR-0125 §(xv) 9→10).

#### Task 2 follow-up — code-quality I1 docstring softening

- **Finding (I1).** Reviewer flagged that the `compiledConfig.stage` doc-comment claimed `buildDescriptorsExt` selects `bucketRateLimitsByStage(...)[cc.stage]`, but the runtime walker at `descriptors.go:336` uses the inline equality predicate `p.GetStage().GetValue() != in.filterStage` — NOT an indexed bucket lookup. The `bucketRateLimitsByStage` helper is exposed for parse-time occupancy assertions + future composition reuse (Task 3 per-route, Task 4 Axis-B), but the runtime walker does NOT consume it today.
- **Fix.** Docstring-only softening at `compiled_config.go:240-251` mirroring the `descriptors.go:77` "observable semantics equivalent" hedge: the field-doc now states that at request time the walker filters by `p.GetStage().GetValue() == cc.stage`, whose observable semantics are equivalent to indexing `bucketRateLimitsByStage(...)[cc.stage]`, and notes the helper is held for parse-time occupancy + future composition reuse. NO production behavior change; the helper + its tests are untouched.
- **Gates.** `go build ./...` clean, `go vet ./...` clean, `golangci-lint run ./internal/filter/http/ratelimit/...` clean, `go test -race -count=1 ./internal/filter/http/ratelimit/` still PASS.

### Task 3 — `compiled_perroute.go` + ADR-0199 FULL + ADR-0125 §(xv) 9→10

- **Commit:** TBD-24.2-T3

- **Files added (new) — `internal/filter/http/ratelimit/`:**
  - `compiled_perroute.go` (~218 LoC including the package + doctrine doc-block): exports `PerRouteTypeURL` (byte-stable per ADR-0143 SN1); defines the unexported `compiledPerRoute` opaque struct {`vhRateLimits`, `rateLimits`, `domain`} (NO override-option per AMEND-4); defines `errPerRouteVhRateLimitsOutOfRange` (defensive sentinel); defines `validatePerRouteRateLimit(proto.Message) error` (5-arm validator — type-assert defensive no-op / `vh_rate_limits` enum-bounds / `rate_limits[]` REUSE of `ValidateRouteRateLimits` / override_option PARSE-ACCEPT / domain PARSE-ACCEPT); exports `RegisterPerRouteValidator(reg)` (ADR-0110 chokepoint registration); defines `compilePerRouteForRequest(proto.Message) *compiledPerRoute` (decode-time projection — nil-tolerant per ADR-0085).
  - `compiled_perroute_test.go` (~310 LoC): 8 test functions / 18 sub-rows — `TestPerRouteTypeURL_ByteStable`; `TestPerRoute_VhRateLimits_Honored` (3 sub-rows: OVERRIDE/INCLUDE/IGNORE); `TestPerRoute_OverrideOption_AcceptedButIgnored` (4 sub-rows: DEFAULT/OVERRIDE_POLICY/INCLUDE_POLICY/IGNORE_POLICY); `TestCompiledPerRoute_StructShape` (the 3-field roster pin); `TestPerRoute_Domain_Override` (3 sub-rows: empty/single-char/multi-char); `TestPerRoute_RateLimits_AxisA_Compile` (4 sub-rows: happy-path generic_key + 3 §5.2 PARSE-REJECT arms — disable_key/extension/dynamic_metadata); `TestPerRoute_TPFC_Registration` (4 sub-rows: resolves on filterName / accepts valid / rejects embedded disable_key / wrong-type defensive accept); `TestCompilePerRouteForRequest_NilTolerance` (nil + wrong-type → nil); `TestPerRoute_VhRateLimitsEnum_OutOfRange` (defensive 4th-value-as-future-extension reject).

- **Files modified:**
  - `cmd/envoy-go/main.go` — added `ratelimit.RegisterPerRouteValidator(httpReg)` call BEFORE `httpReg.Freeze()` (placed after lua's per-route validator registration; mirrors the header_mutation + oauth2 + lua precedent with an inline doc-comment referencing parent §5.3 + ADR-0199 + ADR-0110 single-chokepoint + the AMEND-4 INERT discipline for override_option).
  - `docs/envoy-go/DECISIONS.md` — ADR-0199 §Status + §Date + §Doctrine + §Lands-in updated to "Accepted — landed at phase-24.2 IMPL Task 3"; §Decision placeholder REPLACED with FULL body (4 landed surfaces: PerRouteTypeURL byte-stable / compiledPerRoute opaque shape / 5-arm validator / Register+projection wiring) + the unit-test coverage paragraph; §Consequences placeholder REPLACED with FULL body (5 numbered consequences: (a) roster grows 9 → 10 [10th canonical naming: `data-only-with-vh-inclusion-enum-with-cluster-scoped-stats`]; (b) per-route domain wins-discipline forward-pointer to Task 4; (c) override_option PARSE-ACCEPTED-INERT is upstream-parity NOT a departure; (d) the 22.1→22.3 anticipation→landing precedent + the phase-23 SKIP / phase-24.1 defer / phase-24.2 RE-AMEND cadence; (e) Task 4 + BEHAVIOR_CONTRACT cross-references). ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph + the updated 10-shape catalog paragraph + the atomic-landing rubric paragraph LANDED in-place between §(xiv)'s landing-rubric and the `## ADR-0126` separator per ADR-0044.

- **Gates (verbatim).**
  - `go build ./...` — empty output (clean). EXIT=0.
  - `go vet ./...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./internal/filter/http/ratelimit/... ./internal/filter/hcm/...` — empty output (clean). EXIT=0.
  - `go test ./internal/filter/http/ratelimit/ -run 'TestPerRoute|TestCompilePerRoute|TestCompiledPerRoute' -v` — all 18 sub-rows PASS; `ok github.com/esalaine/envoy-go/internal/filter/http/ratelimit 0.004s`.
  - `go test -race -count=1 ./internal/filter/http/ratelimit/ ./internal/filter/hcm/ ./internal/filter/http/` — `ok github.com/esalaine/envoy-go/internal/filter/http/ratelimit 1.028s`, `ok github.com/esalaine/envoy-go/internal/filter/hcm 1.051s`, `ok github.com/esalaine/envoy-go/internal/filter/http 1.276s`.
  - `go test -count=1 -timeout 5m ./test/differential/ -run 'TestDifferential/003[23]'` — `ok github.com/esalaine/envoy-go/test/differential 3.757s` (both 0032 cross-side + 0033 boot-reject fixtures GREEN).
  - `grep -A2 '^### Decision' docs/envoy-go/DECISIONS.md` under ADR-0199 — non-placeholder ("Status: Accepted — landed at phase-24.2 IMPL Task 3 ..."); ADR-0125 §(xv) `grep -c 'RateLimitPerRoute'` — 2 (the §(xv) AMENDMENT paragraph + the updated 10-shape catalog paragraph).

- **Outcome.** `RateLimitPerRoute` 10th canonical per-route TPFC compile landed: the `PerRouteTypeURL` byte-stable constant, the `compiledPerRoute` opaque type (3 fields per AMEND-4 — NO override-option), the 5-arm `validatePerRouteRateLimit` validator (REUSES `ValidateRouteRateLimits` for the embedded `rate_limits[]` slice — same §5.2 byte-stable PARSE-REJECT arms), the `RegisterPerRouteValidator` entry point wired from main.go BEFORE Freeze, and the nil-tolerant `compilePerRouteForRequest` projection for Task 4 to consume. ADR-0199 §Decision + §Consequences FULL bodies landed; ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph landed in-place per ADR-0044. The deferral streak (phase-23 SKIP-by-absence → phase-24.1 roster-defer → phase-24.2 RE-AMEND) ENDS here. All gates clean; 0032 + 0033 fixtures GREEN. Ready for Task 4 (Axis-B `vh_rate_limits` cross-tier composition table + legacy `include_vh_rate_limits` force-include) — Task 4 consumes `compiledPerRoute.{vhRateLimits, rateLimits, domain}` at descriptor-build time via `dcb.RequestRouteConfig()` + the `compilePerRouteForRequest` projection.

#### Task 3 follow-up — code-quality M-1 (ADR-0199 LoC numbers) + StructShape reflect upgrade

Addressed 2 actionable code-quality findings before Task 4 starts. Strictly in-scope; no production code touched (the only test file changed is `compiled_perroute_test.go` for the StructShape upgrade); 2 unrelated Important findings (I-1 test naming inconsistency, I-2 sub-row name) explicitly DEFERRED to future grooming.

- **M-1 (ADR-0199 §Decision inaccurate LoC numbers).** The §Decision body claimed the TPFC compile is `(~115 LoC)` + sibling test `(~310 LoC)`; landed reality is 231 / 402. Per the lua ADR-0193 precedent (which doesn't quote LoC counts in §Decision), DROPPED the parenthetical annotations entirely — `docs/envoy-go/DECISIONS.md` ADR-0199 §Decision now reads "The TPFC compile lives at `internal/filter/http/ratelimit/compiled_perroute.go` + a sibling test file; the boot-time validator registration wires from `cmd/envoy-go/main.go` BEFORE `httpReg.Freeze()` per ADR-0110 single-chokepoint ...". No new numbers introduced; matches the lua precedent.

- **StructShape reflect upgrade.** The pre-existing `TestCompiledPerRoute_StructShape` body instantiated a `compiledPerRoute{}` and read 3 fields (`_ = c.vhRateLimits` / `_ = c.rateLimits` / `_ = c.domain`) — compile-only, NOT a runtime field-roster pin (a future contributor adding a 4th field like `overrideOption` would still compile-pass + test-pass). Upgraded to reflect-based assertions: (1) `reflect.TypeOf(compiledPerRoute{}).NumField() == 3`; (2) the 3 field names are exactly `{vhRateLimits, rateLimits, domain}` (exact order + names via `reflect.DeepEqual`). Sanity-tested by temporarily adding an `overrideOption uint32` 4th field — the test trips with `compiledPerRoute.NumField() = 4, want 3 — a new field on this struct must be reviewed against AMEND-4 (overrideOption is FORBIDDEN — PARSE-ACCEPTED-but-IGNORED at validate/compile time)` (restored to the clean state immediately). The docstring's "field-roster pin" claim is now LOAD-BEARING.

- **Gates (verbatim, post follow-up).**
  - `go build ./...` — empty output (clean). EXIT=0.
  - `go vet ./...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./internal/filter/http/ratelimit/...` — empty output (clean). EXIT=0.
  - `go test -race -count=1 -run 'TestCompiledPerRoute_StructShape' ./internal/filter/http/ratelimit/` — `ok github.com/esalaine/envoy-go/internal/filter/http/ratelimit 1.013s` (the new reflect-based assertion PASSES).
  - `go test -race -count=1 ./internal/filter/http/ratelimit/` — `ok github.com/esalaine/envoy-go/internal/filter/http/ratelimit 1.026s` (full package PASS — no regressions).

- **Files changed (3 exactly).**
  - `docs/envoy-go/DECISIONS.md` — ADR-0199 §Decision LoC annotations dropped.
  - `internal/filter/http/ratelimit/compiled_perroute_test.go` — `TestCompiledPerRoute_StructShape` upgraded to reflect-based NumField + field-name DeepEqual; `reflect` import added.
  - `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md` — this sub-section.

- **Outcome.** Both code-quality findings (M-1 ADR-LoC accuracy + StructShape no-op upgrade) resolved; ADR-0199 §Decision body now matches landed reality (parenthetical numbers dropped — lua ADR-0193 precedent); the AMEND-4 INERT contract is now LOAD-BEARING at the test layer (a future contributor adding `overrideOption` to `compiledPerRoute` trips `TestCompiledPerRoute_StructShape` immediately). Ready for Task 4.

### Task 4 — Axis-B `vh_rate_limits` cross-tier composition table + legacy force-include

- **Commit:** TBD-24.2-T4

- **Framework extension (`RouteIncludeVhRateLimits()` accessor).** The §4.3 + AMEND-5 legacy `RouteAction.include_vh_rate_limits=true` force-include arm needs a chain-seeded route-level bool. Mirroring the Task 1 RouteMetadata extension (ADR-0165 set-once-by-dispatch + ADR-0198 DELTA-2 chain-field plumbing template):
  - `internal/filter/http/callbacks.go` — added `RouteIncludeVhRateLimits() bool` to `DecoderFilterCallbacks` with a multi-paragraph doc-comment anchoring D-RL11 / §4.3 / AMEND-5 / narrow-exposure-YAGNI.
  - `internal/filter/http/chain.go` — added the `routeIncludeVhRateLimits bool` chain field (with the same set-once-invariant doc-block as the Task 1 `routeMetadata` field); added the `(d *decoderCB) RouteIncludeVhRateLimits() bool` accessor; added the `(c *FilterChain) SetRouteIncludeVhRateLimits(v bool)` setter + the test-readable `(c *FilterChain) RouteIncludeVhRateLimits() bool` companion.
  - `internal/filter/hcm/route.go` — added `includeVhRateLimits bool` field on `routeEntry` (mirroring the Task 1 `metadata` field's positioning).
  - `internal/filter/hcm/config.go` — `buildRouteTable` now captures `rr.Route.GetIncludeVhRateLimits().GetValue()` into the new `routeEntry.includeVhRateLimits` field when the route action is `Route_Route` (direct_response routes have no RouteAction; false). A `//nolint:staticcheck` annotation honors the upstream-deprecated arm per ADR-0080 byte-stable deprecated-arm-honor discipline (mirrors `HeaderMatcher_ExactMatch` et al. in `descriptors.go::evaluateOneHeaderMatcher`).
  - `internal/filter/hcm/connection.go` — H1 dispatch seeds `chain.SetRouteIncludeVhRateLimits(entry.includeVhRateLimits)` BEFORE `RunDecodeHeaders`, alongside the existing `SetRouteMetadata`.
  - `internal/filter/hcm/h2dispatch.go` — `chainDispatchAction.routeIncludeVhRateLimits` field added; `Match` captures it from the matched routeEntry; `WriteH2` seeds `chain.SetRouteIncludeVhRateLimits(c.routeIncludeVhRateLimits)` BEFORE `RunDecodeHeaders`, alongside the existing `SetRouteMetadata`.
  - **17 fake DecoderFilterCallbacks stubs** updated across the filter packages with `func (X) RouteIncludeVhRateLimits() bool { return false }` (mechanical mirroring of the Task 1 RouteMetadata stubbing): `internal/filter/http/callbacks_test.go`, `buffer/`, `header_mutation/`, `bandwidthlimit/`, `localratelimit/`, `oauth2/`, `rbac/`, `adaptive_concurrency/`, `admission_control/`, `csrf/`, `fault/`, `jwtauthn/`, `lua/`, `compressor/`, `extproc/` (2 fakes: `fakeDCB` + `perRouteSwapDCB`), `extauthz/` (2 fakes: `fakeExtAuthzDCB` + `asyncExtAuthzDCB`), `ratelimit/decode_headers_test.go` (also wired through to drive Axis-B legacy-force-include tests via `routeIncludeVh` field).

- **§4.3 Axis-B walker — `descriptors.go`.** Added the `vhostWalkMode` enum (`vhostWalkOverrideDefault` zero-value / `vhostWalkAlways` / `vhostWalkNever`) with a multi-paragraph doc-block anchoring the §4.3 decision table + the AMEND-5 force-include arm. Added the `vhostWalkMode` field to `descriptorInputs` (the `buildDescriptors` 5-arg backward-compat wrapper passes the zero value transparently — 24.1 baseline preserved). Refactored `buildDescriptorsExt` to honor the disposition: the route policy is walked unconditionally; the vhost policy is walked according to `walkMode` (the §4.3 table). The per-policy walk + §4.4 stage filter + §4.5 whole-descriptor-drop discipline moved into a small `appendDescriptors` helper (pure; no per-policy state leak across calls). 24.1 OVERRIDE-default semantics preserved (route non-empty ⇒ vhost SKIPPED via `walkVhost = len(in.routeRateLimits) == 0`).

- **§4.3 Axis-B + Axis-A + domain wiring — `decode_headers.go`.** The `DecodeHeaders` body now:
  - Reads `f.dcb.RequestRouteConfig()` and projects via `compilePerRouteForRequest` to a `*compiledPerRoute` (nil-tolerant per ADR-0085 — nil + wrong-type both yield nil).
  - Reads `f.dcb.RouteIncludeVhRateLimits()` for the AMEND-5 legacy force-include bool.
  - **Axis-A early-return (D-RL11 / AMEND-4):** when `pr != nil && len(pr.rateLimits) > 0`, REPLACES `walkRouteRLs` with `pr.rateLimits` AND zeros `walkVhostRLs`. The §4.3 enum + the legacy bool are bypassed entirely per AMEND-4. Stage filtering (§4.4) still applies via the per-policy `Stage.Value == filterStage` check inside the engine.
  - **Axis-B disposition:** AMEND-5 legacy `legacyForceIncludeV` trumps the enum (when true ⇒ `vhostWalkAlways`); otherwise the enum (`OVERRIDE` ⇒ `vhostWalkOverrideDefault`, `INCLUDE` ⇒ `vhostWalkAlways`, `IGNORE` ⇒ `vhostWalkNever`) governs; no per-route override AND no legacy bool ⇒ 24.1 OVERRIDE-default baseline.
  - **Per-route domain override (D-RL11 / AMEND-4):** at the `RateLimitRequest` build site, `domain = f.cc.domain` is overridden by `pr.domain` when non-empty; the empty-string per-route domain falls back to the filter-config domain (matches upstream parity).

- **Tests added.**
  - In `descriptors_test.go` (~150 LoC, 5 new test functions covering 7 sub-rows): `TestDescriptors_AxisB_OverrideDefault_RouteHasRateLimits` (24.1 baseline regression-pin: route non-empty + OVERRIDE-default ⇒ vhost SKIPPED), `TestDescriptors_AxisB_OverrideDefault_RouteEmpty` (route empty + OVERRIDE-default ⇒ vhost WALKED), `TestDescriptors_AxisB_Include` (INCLUDE ⇒ BOTH walked + walk-order pin: route first, vhost second per AMEND-6), `TestDescriptors_AxisB_Ignore` (2 sub-rows: route-empty + IGNORE ⇒ still zero descriptors; route-non-empty + IGNORE ⇒ route-only), `TestDescriptors_AxisB_LegacyForceInclude` (engine-surface pin: `vhostWalkAlways` forces vhost walk regardless of other inputs — the legacy-trumps-enum decision lives at the decode_headers.go caller, tested separately at `TestDecodeHeaders_PerRoute_AxisB_LegacyForceInclude`), `TestDescriptors_AxisA_EarlyReturn_PerRoute` (Axis-A engine contract: per-route policies passed via `routeRateLimits` + vhost zeroed ⇒ ONLY the per-route policies emit).
  - In `decode_headers_test.go` (~190 LoC, 3 new test functions covering 5 sub-rows): `TestDecodeHeaders_PerRoute_Domain` (3 sub-rows: per-route domain overrides; empty per-route domain falls back; nil per-route falls back), `TestDecodeHeaders_PerRoute_AxisAEarlyReturn` (asserts EXACTLY 1 descriptor — the per-route policy — and route-table + vhost-table policies are INVISIBLE in `req.Descriptors`), `TestDecodeHeaders_PerRoute_AxisB_LegacyForceInclude` (full integration: `vh_rate_limits=IGNORE` + legacy `include_vh_rate_limits=true` ⇒ 2 descriptors — the legacy bool trumps the enum at the decode_headers.go composition site).
  - Extended `fakeRatelimitDCB` with `perRouteCfg proto.Message` + `routeIncludeVh bool` fields, wired through `RequestRouteConfig()` + `RouteIncludeVhRateLimits()`. Added the `ratelimitfilterv3` import.

- **Gates (verbatim).**
  - `go build ./...` — empty output (clean). EXIT=0.
  - `go vet ./...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./...` — empty output (clean) after adding `//nolint:staticcheck` annotation on `rr.Route.GetIncludeVhRateLimits()` per ADR-0080 deprecated-arm-honor discipline. EXIT=0.
  - `go test -race -count=1 -v -run 'TestDescriptors_AxisB|TestDescriptors_AxisA_EarlyReturn_PerRoute|TestDecodeHeaders_PerRoute' ./internal/filter/http/ratelimit/` — all 12 sub-rows PASS (`PASS / ok github.com/esalaine/envoy-go/internal/filter/http/ratelimit 1.022s`).
  - `go test -race -count=1 ./internal/filter/http/ratelimit/` — `ok ... 1.032s` (full package PASS).
  - `go test -race -count=1 ./internal/filter/http/ ./internal/filter/hcm/` — `ok internal/filter/http 1.277s`, `ok internal/filter/hcm 1.052s`.
  - `go test -count=1 -timeout 5m ./test/differential/ -run 'TestDifferential/003[23]'` — `ok ... 3.433s` (both 0032 + 0033 fixtures GREEN — 24.1 OVERRIDE-default behavior preserved).
  - `go test -count=1 -short ./...` — no FAIL rows (baseline preserved across all packages).

- **Outcome.** §4.3 Axis-B cross-tier composition table + AMEND-5 legacy `include_vh_rate_limits` force-include arm landed: the `vhostWalkMode` enum honors the FULL 4-row decision table at the engine surface; `decode_headers.go` computes the disposition per-request from `RateLimitPerRoute.vh_rate_limits` + the legacy bool (AMEND-5 trumps the enum). Axis-A early-return (D-RL11 / AMEND-4) substitutes the per-route embedded `rate_limits[]` for the route-table policy AND zeros the vhost-table — the §4.3 enum + the legacy bool are bypassed entirely. Per-route `domain` override (D-RL11) wins over the filter-config domain at the `RateLimitRequest.Domain` site. The `RouteIncludeVhRateLimits()` chain-seeded accessor is the 2nd ADR-0165-extension landed in phase 24.2 (after `RouteMetadata()` at Task 1) — 17 fake-stub additions mechanical. 24.1 OVERRIDE-default behavior preserved (route non-empty ⇒ vhost SKIPPED; route empty ⇒ vhost WALKED); both 0032 + 0033 fixtures still GREEN. Ready for Task 5 (`encode.go` + `headers.go` X-RateLimit DRAFT_VERSION_03 + ADR-0197 amend).

### Task 5 — `encode.go` + `headers.go` X-RateLimit DRAFT_VERSION_03 + ADR-0197 amend (D-RL9)

- **Commit:** `1e8851f`
- **Files created:** `internal/filter/http/ratelimit/encode.go` (~110 LoC) + `internal/filter/http/ratelimit/headers.go` (~210 LoC) + `internal/filter/http/ratelimit/encode_test.go` (~340 LoC) + `internal/filter/http/ratelimit/headers_test.go` (~280 LoC).
- **Files modified:** `internal/filter/http/ratelimit/dispositions.go` (+27 LoC — `applyError` signature now takes `*RateLimitResponse` arg; store `f.responseStatuses = resp.GetStatuses()` on OK / OVER_LIMIT / fail-OPEN with non-nil resp; fail-CLOSED MUST NOT store per D-RL12); `internal/filter/http/ratelimit/dispositions_test.go` (+148 LoC — `TestDispositions_XRateLimit_Stored_OnAllDispositions` 5 sub-rows pinning the cross-arm store discipline); `internal/filter/http/ratelimit/ratelimit.go` (+24 LoC — NEW `responseStatuses` field on `*filter`; replaced the 24.1 EncodeHeaders STUB with `f.encodeHeaders(headers)` dispatch; REMOVED the `//nolint:unused` annotation on `SetEncoderCallbacks`); `docs/envoy-go/DECISIONS.md` (ADR-0197 §Decision amendment `(xiii)` paragraph appended in-place per ADR-0052; the 24.1 CORE slice body unchanged; X-RateLimit / DRAFT_VERSION_03 mention count under ADR-0197 increased 5 → 7).
- **D-RL9 byte-confirmation outcome — CONFIRMED byte-match; ADR-0202 escape-valve (target #2 per PLAN) UNCONSUMED.**
  - **Approach:** Option B (source-code reading + spec-driven byte construction). The PLAN's hypothesis was unambiguous from the upstream source — no fall-back to Option A required.
  - **Source citation (verbatim):** `gh api repos/envoyproxy/envoy/contents/source/extensions/filters/http/ratelimit/ratelimit_headers.cc?ref=v1.37.2` (the ADR-0008 reference Envoy pin per ENVOY_TARGET.md tag `envoyproxy/envoy:v1.37.2`).
  - **Three potential ambiguities (all resolved without firing ADR-0202):**
    1. **MIN tie-breaker:** upstream `ratelimit_headers.cc:27-29` uses strict `<` comparison (`status.limit_remaining() < min.value().limit_remaining()`) ⇒ first equal-minimum status wins per insertion order. Mirrored verbatim in `headers.go::buildXRateLimitHeaders`. Pinned by `TestHeaders_MIN_Selection_TieBreaker`.
    2. **Fractional `duration_until_reset.seconds` rounding:** upstream `ratelimit_headers.cc:62-63` emits the `Duration.seconds()` int64 directly via `addReferenceKey`; the `nanos` field is IGNORED. No rounding/truncation discipline required — envoy-go emits via `strconv.FormatInt(GetSeconds(), 10)`. Pinned by `TestHeaders_DRAFT_VERSION_03_ByteShape` rows.
    3. **`name=` quoting discipline:** upstream `ratelimit_headers.cc:43-46` uses bare absl::Substitute(`"$0=\"$1\""`, "name", name) — no escaping of embedded chars. envoy-go preserves byte parity via plain string concatenation; pinned (documentary) by `TestHeaders_NameQuoting_BareNoEscape` so any future "add escaping" change surfaces as a test failure + would require an ADR amendment.
  - **Additional byte-confirmed details captured at this Task:** (a) quota-policy iteration ALWAYS visits ALL DescriptorStatus entries (including the MIN-status itself), so the MIN's own descriptor appears as a `;w=` segment in the suffix; (b) statuses without `current_limit` are SKIPPED entirely (do not participate in MIN AND contribute no segment) per the `if (!status.has_current_limit()) continue;` arm at line 25-26; (c) when no status carries `current_limit` the upstream `absl::optional min_remaining_limit_status` stays unset and the function returns the empty header map without adding ANY of the three headers — pinned by `TestHeaders_DRAFT_VERSION_03_ByteShape/all_statuses_lack_current_limit_no_emission` AND `TestEncodeHeaders_NoCurrentLimit_NoEmission`.
- **D-RL12 disposition-aware emission outcome:** the three X-RateLimit headers emit on OK / OVER_LIMIT 429 / fail-OPEN admit; the fail-CLOSED 500 path emits NO X-RateLimit (the `dispositions.go::applyError` failOpen-gate does NOT store `responseStatuses` on the deny branch — `encodeHeaders` then no-ops cleanly on the nullptr-mutate SendLocalReply path). The OFF gate (proto-zero default `RateLimit_OFF`) suppresses emission on ALL dispositions. Pinned by 5 `TestEncodeHeaders_*` rows.
- **Lock discipline note:** `f.responseStatuses` is WRITTEN under f.mu in `dispositions.go::applyDisposition` (async goroutine holds f.mu via the resume-after-OnDestroy guard at `decode_headers.go:261-263`) and READ in `encode.go::encodeHeaders` WITHOUT acquiring f.mu. The lock-free read is REQUIRED because the OVER_LIMIT path's synchronous `dcb.SendLocalReply` → `chain.go::beginLocalReply` (chain.go:1214) → `RunEncodeHeaders` chain runs inline on the SAME goroutine that holds f.mu — re-entrant mu acquisition would deadlock (Go's `sync.Mutex` is non-reentrant). Happens-before is supplied by the chain dispatch sequencing: the store completes BEFORE SendLocalReply (synchronous OVER_LIMIT/fail-CLOSED arms) AND BEFORE ContinueDecoding signals the dispatch goroutine (OK / fail-OPEN admit arms). The `-race` detector observes no races (`go test -race -count=1 ./internal/filter/http/ratelimit/` GREEN at this Task's commit).
- **Tests added (function names + sub-row counts):**
  - `headers_test.go`: `TestHeaders_DRAFT_VERSION_03_ByteShape` (9 sub-rows) + `TestHeaders_MIN_Selection_TieBreaker` (1 row) + `TestHeaders_UnitToSeconds_Table` (8 sub-rows — UNKNOWN + 7 named units) + `TestHeaders_NameQuoting_BareNoEscape` (1 row) + `TestHeaders_ConstantsByteStable` (1 row).
  - `encode_test.go`: `TestEncodeHeaders_OK_AppliesXRateLimit` + `TestEncodeHeaders_OverLimit_AppliesXRateLimit` + `TestEncodeHeaders_FailOpen_AppliesXRateLimit` + `TestEncodeHeaders_FailClosed_NoXRateLimit` + `TestEncodeHeaders_OFF_NoEmission` + `TestEncodeHeaders_NilStatuses_NoEmission` + `TestEncodeHeaders_NoCurrentLimit_NoEmission` + `TestEncodeHeaders_NilCompiledConfig_NoEmission` + `TestEncodeHeaders_CanonicalCase_Insensitive` (9 functions).
  - `dispositions_test.go`: `TestDispositions_XRateLimit_Stored_OnAllDispositions` (5 sub-rows: OK_stores / OVER_LIMIT_stores / fail_OPEN_stores_when_resp_present / fail_CLOSED_does_NOT_store_D_RL12 / transport_error_fail_OPEN_nil_resp_no_store).
- **Gate outputs:**
  - `go build ./...` — empty output (clean). EXIT=0.
  - `go vet ./...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./internal/filter/http/ratelimit/...` — empty output (clean). EXIT=0.
  - `go test ./internal/filter/http/ratelimit/ -run 'TestEncodeHeaders|TestHeaders|TestDispositions_XRateLimit' -v` — all rows PASS (15 top-level functions; 5 + 8 + 9 = 22 sub-rows + 1 (tie-breaker) + 1 (name-quoting) + 1 (constants) = 25 explicit byte/state assertions plus the 9 EncodeHeaders top-level assertions).
  - `go test -race -count=1 ./internal/filter/http/ratelimit/` — `ok ... 1.034s` (full package PASS race-clean).
  - `go test -count=1 -timeout 5m ./test/differential/ -run 'TestDifferential/003[23]'` — `ok ... 3.451s` (24.1 fixtures GREEN; no regression — the 24.1 fixtures use `enable_x_ratelimit_headers: OFF` and the encode-side hook no-ops on that gate).
  - `go test -count=1 -short ./...` — no FAIL rows across all packages (baseline preserved).
  - ADR-0197 grep counts: `awk '/^## ADR-0197/,/^## ADR-0198/' DECISIONS.md | grep -cE 'X-RateLimit|DRAFT_VERSION_03'` increased from 5 (master tip @ 350543b) to 7 (this Task's commit) per the acceptance criterion.
- **ADR-0197 amendment summary:** §Decision body amended in-place per ADR-0052 — paragraph `(xiii)` appended at the existing `(xii)` slot end (before §Cross-references). The 24.1 CORE slice (i)–(xii) bodies stay UNCHANGED; the new (xiii) paragraph documents the X-RateLimit DRAFT_VERSION_03 emission discipline + the remaining-actions slice landed at 24.2 Tasks 1 + 5; the D-RL9 byte-match-confirmed outcome + ADR-0202 UNCONSUMED disposition; the disposition-aware emission table per D-RL12; the lock discipline note; the test surface delta; and the filter-struct surface delta (NEW `responseStatuses` field + removed `//nolint:unused` on SetEncoderCallbacks + production filename count 7 → 10). The §Cross-references list was extended with ADR-0052 (in-place edit discipline), ADR-0202 (escape-valve UNCONSUMED), ADR-0008 (reference Envoy v1.37.2 pin), and forward-pointers to phase-24 SPEC §4.7 / §6.6 + AMEND-8 + phase-24.2 PLAN Task 1 + Task 5.
- **Outcome.** X-RateLimit DRAFT_VERSION_03 response-header injection LANDED on all 3 emitting dispositions (OK / OVER_LIMIT 429 / fail-OPEN admit) with the upstream-mirrored byte format. The fail-CLOSED 500 path emits NO X-RateLimit per D-RL12 — store-suppression at the dispositions layer makes the encode-side hook a clean no-op. The OFF gate (proto-zero `RateLimit_OFF`) suppresses emission on ALL dispositions. D-RL9 byte-confirmation: **CONFIRMED — byte-match against upstream source at v1.37.2 (Option B; direct source reading via `gh api` against the v1.37.2 tag).** ADR-0202 escape-valve (target #2 per PLAN — the X-RateLimit MIN-status quota byte-edge) UNCONSUMED. The 24.1 OK / OVER_LIMIT / error byte-shape unchanged (the new encode-side path is OFF-gated on the 24.1 fixtures); 0032 + 0033 fixtures stay GREEN. ADR-0197 §Decision amendment `(xiii)` landed in-place per ADR-0052; X-RateLimit / DRAFT_VERSION_03 mention count under ADR-0197 increased 5 → 7. The 24.2 PLAN's remaining-actions slice (Task 1) + X-RateLimit slice (this Task) both anchored to ADR-0197 §Decision via the in-place amendment. Ready for Task 6 (0032 fixture extensions (f) + (g) + (d-extension) — the cross-side end-to-end byte-pin of the X-RateLimit emission against reference Envoy).

#### Task 5 follow-up — spec + code-quality I-1 (AMEND-8 wire-order regression on OVER_LIMIT)

- **Commit:** TBD
- **Background.** Spec-compliance reviewer + code-quality reviewer both flagged I-1: the Task 5 X-RateLimit emission landed AFTER filter-config `response_headers_to_add` on the OVER_LIMIT path (instead of BETWEEN `x-envoy-ratelimited` and filter-config per parent SPEC §4.7 line 214). Root cause: encode-side `headers.Set` for a net-new canonical key gets sorted alphabetically at the TAIL by `ReconcileOrderedHeaders`, landing X-RateLimit after the filter-config slot. The 24.1-baked `x-envoy-ratelimited`-before-RLS inversion is OUT OF SCOPE (inherited 24.1 behavior).
- **Fix — Option (a) from code-quality reviewer (inline X-RateLimit at applyOverLimit).** `dispositions.go::applyOverLimit` now constructs the X-RateLimit triple inline BETWEEN slot [b] (RLS-supplied response headers) and slot [c] (filter-config response_headers_to_add). The encode-side hook (`encode.go::encodeHeaders`) is UNCHANGED — on the OVER_LIMIT path its `headers.Set` becomes a no-op-set-equal idempotent overwrite (the same `buildXRateLimitHeaders` source produces byte-identical values). Option α from the issue body was selected — simpler, less state; no new `xrlEmittedInline` flag.
- **Files modified:**
  - `internal/filter/http/ratelimit/dispositions.go` (+~21 LoC — inline X-RateLimit triple construction in `applyOverLimit`, gated on `cc.enableXRateLimitHeaders` + `buildXRateLimitHeaders.ok=true`; updated file-header docstring AMEND-8 ordering table to include new `[c-pre]` slot; updated capacity hint `+3` for the triple).
  - `internal/filter/http/ratelimit/dispositions_test.go` (+~140 LoC — 3 NEW tests: `TestDispositions_OverLimit_AMEND8_XRateLimitBetweenXEnvoyAndConfig` (wire-order pin per SPEC §4.7 line 214), `TestDispositions_OverLimit_XRateLimit_OFF_NoInlineEmission` (OFF-gate inline suppression), `TestDispositions_OverLimit_XRateLimit_NoCurrentLimit_NoInlineEmission` (no-current-limit inline suppression)).
  - `internal/filter/http/ratelimit/encode_test.go` (+~75 LoC — NEW test `TestEncodeHeaders_OverLimit_IdempotentSetEqual_FilterConfigPrePopulated` pre-populates the carrier with the post-inline AMEND-8 order including the filter-config slot, then verifies the encode hook's `headers.Set` is a no-op-set-equal overwrite (single value per X-RateLimit key; filter-config / RLS / x-envoy-ratelimited all preserved)).
  - `docs/envoy-go/DECISIONS.md` (ADR-0197 paragraph (xiii) extended with one sentence documenting the wire-order discipline: "The OVER_LIMIT wire-order discipline (X-RateLimit BETWEEN `x-envoy-ratelimited` and filter-config `response_headers_to_add`) is enforced inline at `applyOverLimit` per Task 5 follow-up; the encode hook's `headers.Set` on OVER_LIMIT is a no-op-set-equal idempotent second pass (same `buildXRateLimitHeaders` source)."  On OK / fail-OPEN dispositions the encode hook remains the SOLE emission point).
- **Tests added (function names):** `TestDispositions_OverLimit_AMEND8_XRateLimitBetweenXEnvoyAndConfig` + `TestDispositions_OverLimit_XRateLimit_OFF_NoInlineEmission` + `TestDispositions_OverLimit_XRateLimit_NoCurrentLimit_NoInlineEmission` + `TestEncodeHeaders_OverLimit_IdempotentSetEqual_FilterConfigPrePopulated`. TDD red→green: the AMEND8 wire-order test was authored to fail first (only 3 entries — x-envoy-ratelimited / RLS / X-From-Config — no X-RateLimit triple in args.headers) and went green after the inline-X-RateLimit insertion in `applyOverLimit`.
- **Gate outputs:**
  - `go build ./...` — empty output (clean). EXIT=0.
  - `go vet ./...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./internal/filter/http/ratelimit/...` — empty output (clean). EXIT=0.
  - `go test -race -count=1 ./internal/filter/http/ratelimit/` — `ok ... 1.035s` (full package PASS race-clean; all 4 new tests + all existing 24.1 + Task 5 tests GREEN, no regressions).
  - `go test -count=1 -timeout 5m ./test/differential/ -run 'TestDifferential/003[23]'` — `ok ... 3.330s` (0032 + 0033 fixtures stay GREEN — these use `enable_x_ratelimit_headers: OFF` so the inline emission is gated off and the byte-shape is unchanged).
- **24.1-baked inversion (out of scope).** The 24.1-baked `x-envoy-ratelimited`-before-RLS inversion (slot [a] before slot [b] vs SPEC's "RLS → x-envoy-ratelimited" canonical) is an inherited 24.1 behavior; this Task 5 follow-up ONLY addresses the X-RateLimit-vs-filter-config slot. Documented in the `TestDispositions_OverLimit_AMEND8_XRateLimitBetweenXEnvoyAndConfig` docstring.
- **Outcome.** I-1 wire-order regression CLOSED. Parent SPEC §4.7 line 214 canonical AMEND-8 ordering on OVER_LIMIT is now enforced inline at `applyOverLimit` (X-RateLimit between `x-envoy-ratelimited` and filter-config). The encode-side hook stays unchanged — on OVER_LIMIT its `headers.Set` is a no-op-set-equal idempotent overwrite; on OK / fail-OPEN it remains the sole emission point. ADR-0197 paragraph (xiii) extended in-place per ADR-0052 with the wire-order discipline sentence. All gates GREEN.

### Task 6 — `0032-http-ratelimit/` fixture extensions (f) + (g) + (d-extension)

- **Commit:** `2a438f4`
- **Files modified:** `test/fixtures/0032-http-ratelimit/envoy.yaml` (~190 LoC added; bootstrap `node.cluster: rls_test_cluster` for source_cluster; route metadata.filter_metadata.envoy.filters.http.ratelimit.tier=gold on (d); 5 new actions appended to scenario (d) policy; vh-level `rate_limits` emitting `generic_key{vh:vh_a}`; scenario (a) gets `vh_rate_limits=IGNORE` TPFC to preserve zero-descriptor semantics; 3 NEW (f) sub-scenario routes — f_override/f_include/f_ignore — with TPFC vh_rate_limits=INCLUDE on f_include and IGNORE on f_ignore; NEW /scenario_g route with 2 single-action rate_limits[] policies (tier:bronze + scope:burst); filter config: `enable_x_ratelimit_headers: DRAFT_VERSION_03` + `response_headers_to_add: [{key: x-from-config, value: filtercfg}]`).
  - `test/fixtures/0032-http-ratelimit/envoy-go.yaml` (~90 LoC added; same content as envoy.yaml modulo cluster type STATIC vs STRICT_DNS + endpoint addresses 127.0.0.1 vs host.docker.internal + admin/listener port templating).
  - `test/fixtures/0032-http-ratelimit/expectations.yaml` (~150 LoC added; full prose-spec for the 9-scenario matrix — added (f1/f2/f3/g) sub-rows + the §4.3 Axis-B composition table; counter expectation deltas updated `ok=2→5, over_limit=1→2, error/failure_mode_allowed unchanged`; documented divergence-windows extended with the inherited 24.1 slot [a]/[b] inversion + per-hop header byte-stream limitation).
  - `test/fixtures/0032-http-ratelimit/inputs/driver.go` (~250 LoC net change; scenario (g) probe captures response headers + emits 3 X-RateLimit byte-pin lines into the cross-side stream; (d) extension scripts the 9-entry CanonicalKey; (f) sub-scenario probes script the 3 new (1/2-descriptor) keys; (g) scripts the 2-descriptor OVER_LIMIT response with per-descriptor CurrentLimit/LimitRemaining/DurationUntilReset populated; new helpers `canonicalKeyForMulti`, `respOverLimitWithStatuses`, `xrlStatus` struct; new constants `nodeServiceCluster`, `maskedLoopbackIP`, `vhDescValue`; AssertStats expectations updated to ok=5 / over_limit=2; updated multi-paragraph package-doc + driveProxy doc).
  - `test/fixtures/0032-http-ratelimit/README.md` (~190 LoC; converted from 6-scenario doc to 9-scenario doc — added (f)/(g)/(d-extension) subsections with the §4.3 Axis-B table; updated the scenario matrix table; updated cross-refs).
- **`test/helpers/ratelimitgrpc/` UNCHANGED.** Per the task brief discovered-context note, the 24.1 `Script(key, resp)` API already accepts a full `*RateLimitResponse` — per-descriptor statuses[] with `current_limit`/`limit_remaining`/`duration_until_reset` are configurable via the existing API. The driver populates these on the scenario (g) script directly via Go-protobuf struct literals. No helper additions needed.
- **Probe sequence.** 9 scenarios in this order (the 24.2 (f)/(g) lands BEFORE the RLS-stop for (e), so (e) is now LAST):
  `a (RLS-up) → b → c → d → f-override → f-include → f-ignore → g → STOP-fake → e`.
- **Scenario (f) `vh_inclusion` — 3 sub-scenarios.** Vhost vh_a carries vh-level `rate_limits` emitting `generic_key{vh:vh_a}`. Three sub-scenario routes exercise the §4.3 table:
  - f1 (override): NO TPFC ⇒ proto-zero `vh_rate_limits=OVERRIDE`; route non-empty ⇒ vh SKIPPED ⇒ 1 descriptor `[{scenario:f_override}]` ⇒ fake key `domain_b|scenario=f_override` → OK.
  - f2 (include): TPFC `vh_rate_limits=INCLUDE` ⇒ BOTH route + vh walked (route first per AMEND-6) ⇒ 2 descriptors `[{scenario:f_include}, {vh:vh_a}]` ⇒ fake key `domain_b|scenario=f_include|vh=vh_a` → OK.
  - f3 (ignore): TPFC `vh_rate_limits=IGNORE` ⇒ vh SKIPPED unconditionally ⇒ 1 descriptor `[{scenario:f_ignore}]` ⇒ fake key `domain_b|scenario=f_ignore` → OK.
  All three return 200 + echo body; cross-side `CompareBytes` on the status line passes. **Note (single-vhost constraint):** envoy-go's HCM only accepts a single virtual_host with `domains: ["*"]` (config.go:227-233). The vhost-level `rate_limits` lives on `vh_a` (the only allowed vh); the 24.1 scenarios (b/c/d/e/g) avoid the vh-policy via OVERRIDE-default vh-SKIP arm (route non-empty wins). Scenario (a) — the only 24.1 zero-route-rate_limits scenario — carries `vh_rate_limits=IGNORE` TPFC to preserve its 24.1 zero-descriptor semantics.
- **Scenario (g) `x_ratelimit_headers` — X-RateLimit triple byte-pin.** Route has 2 single-action policies → 2 descriptors. Fake scripts `domain_b|tier=bronze|scope=burst` → OVER_LIMIT with per-descriptor statuses: `[{code:OVER_LIMIT, current_limit:{rpu:10, unit:SECOND}, limit_remaining:2, duration_until_reset.seconds:1}, {code:OVER_LIMIT, current_limit:{rpu:100, unit:MINUTE}, limit_remaining:7, duration_until_reset.seconds:60}]`. The filter emits the X-RateLimit triple per §4.7 + AMEND-8 wire order with the Task 5 follow-up inline-at-applyOverLimit discipline:
  - `x-envoy-ratelimited: true` (slot [a])
  - X-RateLimit triple (slot [c-pre]; MIN selection picks statuses[0]: limit_remaining=2 < 7 per strict-`<`)
    - `x-ratelimit-limit: 10, 10;w=1, 100;w=60` (MIN.rpu + quota-policy segments for BOTH descriptors per upstream iterate-all)
    - `x-ratelimit-remaining: 2`
    - `x-ratelimit-reset: 1`
  - `x-from-config: filtercfg` (slot [c])
  The driver byte-pins the 3 X-RateLimit values into the cross-side byte stream via `emitScenarioG` (4 lines per (g): status verdict + 3 header lines). **Cross-side verified byte-exact** via FIXTURE_0032_DUMP_BYTES=1 inspection of the live test run: both reference Envoy v1.37.2 and envoy-go emit identical `X-Ratelimit-Limit:[10, 10;w=1, 100;w=60] X-Ratelimit-Remaining:[2] X-Ratelimit-Reset:[1]` — the MIN-selection / unit→seconds / quota-policy suffix construction matches upstream byte-for-byte.
- **Scenario (d) `descriptor_actions` extension — 9-action chain.** The 24.1 4-action chain (generic_key + request_headers + remote_address + header_value_match) is EXTENDED with 5 NEW actions (`source_cluster` + `masked_remote_address` + `metadata` + `query_parameters` + `query_parameter_value_match`) for a 9-entry descriptor. The request URL gains a query string `?region=us-east&plan=premium` (consumed by query_parameters + query_parameter_value_match). The route gains `metadata.filter_metadata.envoy.filters.http.ratelimit.tier: gold` (consumed by metadata action's ROUTE_ENTRY source path). The bootstrap gains `node.cluster: rls_test_cluster` (consumed by source_cluster action). `destination_cluster` REMAINS OMITTED per the 24.1 framework-limitation note (no MatchedClusterName() accessor at master tip → engine drops whole descriptor on empty input). Fake script's 9-entry CanonicalKey → OK → echo 200; cross-side `CompareBytes` byte-exact.
- **Single-vhost constraint discovery.** Initial design attempted a separate `vh_f` virtual_host for the (f) sub-scenarios with `domains: ["vh-f.test"]`. envoy-go REJECTED the config at parse time with `hcm: route_config: virtual_hosts: got 2, want exactly 1` (config.go:227). The single-vhost canonical predates phase 24 (ADR-0019/0072); the fixture was redesigned to put the vh-level `rate_limits` on the sole `vh_a` AND use `vh_rate_limits=IGNORE` TPFC on scenario (a) to preserve its 24.1 zero-descriptor semantics. The (f) sub-scenarios use OVERRIDE-default (f1) / INCLUDE TPFC (f2) / IGNORE TPFC (f3) on routes within `vh_a`. The 24.1 scenarios b/c/d/e/g rely on OVERRIDE-default vh-SKIP (route non-empty wins) to keep the vh-policy invisible to them.
- **AssertStats expectation deltas.** Updated for 24.2:
  - `ok=5` (was 2 at 24.1): scenarios b + d + f-override + f-include + f-ignore.
  - `over_limit=2` (was 1 at 24.1): scenarios c + g.
  - `error=1` (unchanged): scenario e.
  - `failure_mode_allowed=1` (unchanged): scenario e fail-open.
- **AssertStats deliberate-break verification.** Temporarily flipped expected `ok: 5 → 999` in driver.go; re-ran 0032 → FAILED with `cluster.c_ratelimit.ratelimit.ok = 5; want 999`; reverted → GREEN. Assertion is LIVE.
- **Gate outputs:**
  - `go build ./...` — empty output (clean). EXIT=0.
  - `go vet ./...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./...` — empty output (clean). EXIT=0.
  - `go test -count=1 -timeout 5m ./test/differential/ -run 'TestDifferential/0032-http-ratelimit' -v` — `--- PASS: TestDifferential/0032-http-ratelimit (1.58s)`. All 9 scenarios (a/b/c/d-extended/e/f1/f2/f3/g) + (h) StatsAsserter GREEN.
  - `go test -count=1 -timeout 30m ./test/differential/` — 34/35 PASS in combined run; `0025-http-adaptive-concurrency` flaked once (the known multi-listener flake per 22.2 REVIEW §7.4). Re-ran 0025 in isolation: `--- PASS: TestDifferential/0025-http-adaptive-concurrency (4.87s)`. Net: 35/35 GREEN across isolated re-run.
  - FIXTURE_0032_DUMP_BYTES=1 inspection confirms BOTH sides emit byte-identical X-RateLimit triple (`10, 10;w=1, 100;w=60` / `2` / `1`).
- **Outcome.** Phase 24.2 Task 6 fixture extensions LANDED in-place (NO new fixture directory — project count stays at 35). 9-scenario matrix (3 sub-scenarios under (f); single (g); (d) extended to all 9 framework-reachable actions). Cross-side byte-exact on b/c/d-extended/e/f1/f2/f3/g via the existing CompareBytes runner branch per `reference_differential_fixture_dispatch_constraint`. The (h) StatsAsserter dispatch UNCHANGED — proven live at 24.1 + re-verified live at 24.2 via the deliberate-break recipe. X-RateLimit byte-pin (scenario g) provides the cross-side proof that the MIN-selection + unit→seconds + quota-policy suffix construction in headers.go matches upstream byte-for-byte — the strongest possible cross-side gate for the D-RL9 byte-confirmed source-derived format. The Task 5 follow-up inline-at-applyOverLimit wire-order discipline is exercised end-to-end (X-RateLimit triple lands BETWEEN `x-envoy-ratelimited` and filter-config `x-from-config`). Ready for Task 7 (FuzzRateLimitConfigParse corpus extension; no new fuzzer).

#### Task 6 follow-up — code-quality I-1 (stale 2-vhost-design comments) + I-2 (stale arithmetic block)

- **Commit:** TBD
- **Background.** Code-quality reviewer flagged 2 Important doc-drift findings in `test/fixtures/0032-http-ratelimit/inputs/driver.go` against commits `2a438f4` + `3ea9324`. Both are doc-only — no functional impact (final AssertStats expectations at line ~845 are already correct: `ok=5, over_limit=2, error=1, failure_mode_allowed=1`).
- **I-1 (stale 2-vhost-design comments).** Three comment sites referenced an abandoned 2-vhost design with `vh_f` (domains: `["vh-f.test"]`). Actual implementation is single-vhost `vh_a` per envoy-go HCM's exactly-1-vh constraint (config.go:227-233, single-vhost canonical predating phase 24 per ADR-0019/0072). Edits:
  - File-header bullet `(f) vh_inclusion` (line ~23): replaced "on vh_f (Host: vh-f.test)" with "on vh_a (the sole virtual_host — envoy-go's HCM enforces exactly-1 vh per config.go:227-233; all (f) sub-routes use the default Host)" + clarified the sub-scenarios exercise the §4.3 Axis-B table via per-route TPFC `RateLimitPerRoute`.
  - File-header "# Single-listener topology" block (lines ~62-78): rewrote the entire 2-vhost paragraph to single-vhost reality — `vh_a` carries ALL scenarios on dedicated routes; (f1)/(f2)/(f3) live on sub-routes `/scenario_f_{override,include,ignore}` with TPFC `RateLimitPerRoute`. The vhost-level `rate_limits` emits `generic_key{vh:vh_a}` walked conditionally per §4.3: SKIPPED by 24.1 routes b/c/d/e/g via OVERRIDE-default vh-SKIP (route non-empty wins), SKIPPED by (a) via explicit `vh_rate_limits=IGNORE` TPFC, INCLUDED only on the f-include sub-route (2-descriptor `scenario=f_include|vh=vh_a` key).
  - setupRLS script-key table entry (line ~244): `scenario=f_include|vh=vh_f` → `scenario=f_include|vh=vh_a` (matches the `vhDescValue` const).
- **I-2 (stale arithmetic block).** Lines ~576-586 of the `driveProxy` docstring carried a self-contradicting `ok = 6 (b + d + f-override + f-include + f-ignore — 5 OK admits ...)` clause followed by a "Wait — re-counting" remediation block that produces the correct count. Replaced with a clean canonical formulation matching the surrounding section prose style:
  ```
  //	ok                   = 5 (b + d + f-override + f-include + f-ignore)
  //	over_limit           = 2 (c + g)
  //	error                = 1 (e)
  //	failure_mode_allowed = 1 (e)
  //
  // Scenario (a) is zero-descriptor (no RLS call → no counter increment); (g)
  // is OVER_LIMIT not OK.
  ```
- **Files modified:**
  - `test/fixtures/0032-http-ratelimit/inputs/driver.go` (doc-only; -10/+12 LoC net across the 3 I-1 sites + 1 I-2 site).
  - `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md` (this follow-up note).
- **Gate outputs:**
  - `go build ./...` — empty output (clean). EXIT=0.
  - `go vet ./...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./...` — empty output (clean). EXIT=0.
  - `go test -count=1 -timeout 5m ./test/differential/ -run 'TestDifferential/0032-http-ratelimit' -v` — `--- PASS: TestDifferential/0032-http-ratelimit (1.95s)`. All 9 scenarios (a/b/c/d-extended/e/f1/f2/f3/g) + (h) StatsAsserter GREEN.
- **Outcome.** I-1 + I-2 doc-drift CLOSED. No functional change (no YAMLs, no expectations.yaml, no README.md, no production code touched). The driver.go file-header + setupRLS docstring + driveProxy AssertStats prose now consistently describe the as-implemented single-vhost / 5-ok / 2-over_limit / 1-error / 1-failure_mode_allowed topology.

### Task 7 — `FuzzRateLimitConfigParse` corpus extension (no new fuzzer)

- **Commit:** TBD-24.2-T7
- **Files modified:** `internal/filter/http/ratelimit/fuzz_test.go` (corpus extension + 3rd-surface dispatch; ~150 LoC added — +14 seeds + Surface-3 fuzz body + extended file-header doc + 2 new imports `metadatav3` + `wrapperspb`).
- **Corpus delta (Seeds 32-46; 24.2 Task 7 per D-RL16).**
  - **5 remaining §4 action arms (24.2 Task 1):** Seeds 32-36 — `source_cluster` / `masked_remote_address` (v4=24, v6=64 mask lens) / `metadata` (DYNAMIC source + single-segment path + default_value=anon) / `query_parameters` (api_key→key) / `query_parameter_value_match` (PresentMatch on tag qpname). Drive the FULL 10-action dispatch via Surface 2 (`buildDescriptors`).
  - **6 `RateLimitPerRoute` seeds (Surface 3 — 24.2 Task 3 TPFC compile):** Seeds 37-42:
    - Seeds 37/38/39: `vh_rate_limits` = OVERRIDE / INCLUDE / IGNORE (Axis-B inclusion enum bounds; 3 in-proto values per v1.32.4).
    - Seeds 40/41/42: `override_option` = OVERRIDE_POLICY / INCLUDE_POLICY / IGNORE_POLICY (AMEND-4 PARSE-ACCEPTED-but-IGNORED arm; DEFAULT=0 already covered by the OVERRIDE seed 37 via proto-zero). Seed 42 ALSO carries non-empty `domain="tenant-override"` (per-route domain wins-discipline; Task 4 consumer).
  - **Per-policy stage boundary arms (24.2 Task 2):** Seeds 43/44 — `stage=5` (new arm under multi-stage bucketing — `bucketRateLimitsByStage` slot 5 holds the policy; Surface 2 `ValidateRouteRateLimits` accepts) + `stage=11` (new per-policy PARSE-REJECT arm — `ValidateRouteRateLimits` REJECTS byte-stable `ratelimit: stage must be <= 10`). Each carries one `generic_key` action so the policy is otherwise valid.
  - **X-RateLimit toggle combination arm (24.2 Task 5):** Seed 45 — `validRateLimitConfig()` + `EnableXRatelimitHeaders=DRAFT_VERSION_03` + `StatPrefix="ingress"` + `DisableXEnvoyRatelimitedHeader=true` (the 24.1 Seed 28 covered the toggle in isolation; this seed pairs it with the AMEND-1 cluster-scoped modulator + the legacy x-envoy-ratelimited disable-flag for the full headers-side combination).
  - **Legacy `RouteAction.include_vh_rate_limits=true` byte shape (AMEND-5; D-RL10):** Seed 46 — `routev3.RouteAction{Cluster: "upstream_xyz", IncludeVhRateLimits: wrapperspb.Bool(true)}` proto bytes. The fuzz body cannot type-assert through a `routev3.RouteAction` (legacy bool is threaded via the DCB at request time, not via a TypeURL envelope); the bytes exercise the proto-Unmarshal-mismatch defensive paths in all 3 surfaces (overlapping wire-tag varints).
- **Fuzz body 3rd surface added (Surface 3).** Mirrors the Surface 2 try-decode pattern:
  ```go
  var pr ratelimitfilterv3.RateLimitPerRoute
  if err := proto.Unmarshal(data, &pr); err == nil {
      _ = validatePerRouteRateLimit(&pr)
      _ = compilePerRouteForRequest(&pr)  // independent of validator
  }
  ```
  Drives the ADR-0110 single-chokepoint validator AND the request-time projection against the random shape. Projection is invoked even when validator would reject (defensive per ADR-0085 nil-tolerance + ADR-0018 never-panic).
- **Seed count after extension:** 46 hand-curated seeds (31 from 24.1 + 15 new at 24.2; net +15 but PLAN bullet target was ~10-20 — within range; legacy-bool byte-shape seed counted as an "edge-case neighbor").
- **Project fuzzer count UNCHANGED at 33.** Per D-RL16 + Task 7 brief: corpus-only extension; NO new `func Fuzz*` added. Verified via `find ... -name 'fuzz_test.go' | xargs grep -h '^func Fuzz' | sort -u | wc -l` → `33`.
- **Production code UNCHANGED.** Task 7 scope is strictly the existing fuzz file (test-only). `ratelimit.go` / `descriptors.go` / `compiled_config.go` / `compiled_perroute.go` / `encode.go` / `headers.go` etc. all untouched.
- **Gate outputs:**
  - `go build ./...` — empty output (clean). EXIT=0.
  - `go vet ./...` — empty output (clean). EXIT=0.
  - `golangci-lint run ./internal/filter/http/ratelimit/...` — empty output (clean). EXIT=0 (after one gofmt re-format on the `IncludeVhRateLimits:` alignment).
  - `go test -count=1 -run 'FuzzRateLimitConfigParse' ./internal/filter/http/ratelimit/ -v` — PASS (46/46 seeds; seed#0 through seed#45, total 0.004s). No panic, no failure on any seed.
  - `go test -run 'XXX_NONE' -fuzz 'FuzzRateLimitConfigParse' -fuzztime 30s ./internal/filter/http/ratelimit/` — PASS, 30s envelope clean. Verbatim:
    ```
    fuzz: elapsed: 0s, gathering baseline coverage: 0/426 completed
    fuzz: elapsed: 3s, gathering baseline coverage: 426/426 completed, now fuzzing with 32 workers
    fuzz: elapsed: 6s, execs: 202720 (67441/sec), new interesting: 26 (total: 452)
    ...
    fuzz: elapsed: 30s, execs: 1936482 (85563/sec), new interesting: 57 (total: 483)
    fuzz: elapsed: 31s, execs: 1936482 (0/sec), new interesting: 57 (total: 483)
    PASS
    ok  	github.com/esalaine/envoy-go/internal/filter/http/ratelimit	31.089s
    ```
    Baseline coverage gathered from 426 inputs (46 seeds + 380 cached fuzz cache entries from prior 24.1 runs); 1.9M execs in 30s across 32 workers; 57 new-interesting; ZERO crashers / panics / shrinks.
  - `find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l` → `33` (project fuzzer count UNCHANGED).
- **Outcome.** Phase 24.2 Task 7 EXTENDS the existing `FuzzRateLimitConfigParse` per D-RL16 with +15 hand-curated seeds covering the 5 new §4 actions + the 10th-canonical-TPFC `RateLimitPerRoute` shape across vh_rate_limits + override_option enum spaces + per-policy stage boundaries (5/11) + per-route domain override + X-RateLimit toggle combination + legacy `include_vh_rate_limits=true` byte shape. The fuzz body gains a 3rd surface (`validatePerRouteRateLimit` + `compilePerRouteForRequest`) mirroring the Surface 2 try-decode pattern. Must-never-panic invariant proven across all 3 surfaces for the 46-seed corpus AND for 1.9M random execs over 30s. Project fuzzer count remains exactly 33. Ready for Task 8 (atomic landing — BEHAVIOR_CONTRACT + ROADMAP 24.2+24 [ROLLUP] + STATE + REVIEW).

### Task 8 — Atomic landing — BEHAVIOR_CONTRACT + ROADMAP 24.2+24 [ROLLUP] + STATE + REVIEW

- **Commit:** TBD-24.2-T8 (this Task 8 atomic-landing commit; pre-amend SHA will be captured by `git log -1 --oneline` post-commit per the phase-09..23 + 24.1 stage-close pattern).
- **Files touched:**
  - `docs/envoy-go/BEHAVIOR_CONTRACT.md` (24.2 completion bundle per D-RL17 — subsection extension in-place + per-route canonical-patterns caption update + table extension §(xiv) + §(xv) + cross-reference paragraph + response-header allow-list 3-row extension + the X-RateLimit allow-list note paragraph + stat-name mapping `domain`-qualifier paragraph; **NO new departure records** — count STAYS at 18).
  - `docs/envoy-go/ROADMAP.md` (**row 24.2 flipped `planned → done`** AND **parent row 24 flipped `in-progress → done` IN THE SAME COMMIT** per D-RL18 + the 18/19/22 ROLLUP precedent; both with per-cell done annotations; sub-phases column for row 24 STAYS `24.1, 24.2`).
  - `docs/envoy-go/STATE.md` (re-advanced to `awaiting next planning (wasm — final §9 family-row; not yet a ROADMAP row)`; lifecycle-state 6 → 0/1; next-skill `superpowers:brainstorming`; ADR-0202 UNCONSUMED disposition recorded).
  - `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/REVIEW.md` (NEW — authored per `superpowers:requesting-code-review` covering the FULL parent §15 acceptance UNION; six-gate verbatim outputs; D-RL8/D-RL9 outcomes; per-task commit SHA roster).
  - `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md` (this Task 8 entry).
  - `internal/filter/http/ratelimit/fuzz_test.go` (Task 7 fuzz_test.go header doc-drift cleanup — 3 lines: line 38 `"~14 additional seeds (45 total)"` → `"15 additional seeds (46 total)"`; line 72 `"24.2 Task 7 extension (~14 new seeds; …)"` → `"24.2 Task 7 extension (15 new seeds; …)"`; line 470 `"24.2 Task 7 extension — Seeds 32-45"` → `"24.2 Task 7 extension — Seeds 32-46"`).
- **D-RL17 (BEHAVIOR_CONTRACT completion bundle) execution.** Per the 4-item bundle:
  - **(1) Subsection extension** — added `#### Phase 24.2 completion bundle (per D-RL17 + parent §13 — atomic landing per ADR-0052)` block at the end of `### envoy.filters.http.ratelimit` (just before `### Applies to`): engine completion to all 10 actions (with action-table for the 5 NEW arms showing key/value/disposition) + `destination_cluster` framework-limited disposition note + `stage` multi-bucket discipline + Axis-B `vh_rate_limits` decision table (7-row truth table covering per-route-present + per-route-rate_limits-non-empty + inclusion-enum + legacy-bool axes) + X-RateLimit DRAFT_VERSION_03 emission discipline (the AMEND-8 wire-order at `[a]+[X-RateLimit]+[b]+[c]` per Task 5 follow-up I-1 fix + MIN-status tie-break + fail-closed nullptr-mutate disposition) + `RateLimitPerRoute` 10th canonical (per ADR-0199 + ADR-0125 §(xv)) + `override_option` accepted-but-INERT note (NOT a departure) + ADR-0202 escape-valve disposition (UNCONSUMED).
  - **(2) Per-route canonical-patterns caption** updated `## Per-route canonical patterns cross-reference (ADR-0125 roster; updated through phase 24.1 ...)` → `... updated through phase 24 — RateLimitPerRoute 10th-canonical AMENDMENT 9 → 10 LANDED at 24.2 IMPL Task 3 per ADR-0125 §(xv) + ADR-0199`; table extended with §(xiv) 9th canonical (lua @ phase 22.3) + §(xv) 10th canonical (ratelimit @ phase 24.2) rows; appended the **Phase 24.2 (ratelimit — per-route + headers slice)** cross-reference paragraph documenting the §(xv) AMENDMENT LANDED + the 10th canonical shape + the THIRD §9 row to extend the roster + the §9 family closure.
  - **(3) Response-header allow-list paragraph** — added 3 new rows to the `## Header allow-list` table (`x-ratelimit-limit` + `x-ratelimit-remaining` + `x-ratelimit-reset`) — each with Scope `HCM-locally-generated + routed-to-upstream responses (ratelimit filter encode side)` + Permitted-divergence `Set-equal byte-exact per scenario (g)` + Introduced `Phase 24.2` + Justifying ADR `ADR-0197 (in-place §Decision amendment for X-RateLimit slice per ADR-0052)`. Appended `**Phase 24.2 X-RateLimit allow-list extension note**` paragraph documenting set-equal byte-exact disposition + emission gating + OVER_LIMIT wire-order + fail-closed disposition + MIN-status tie-break.
  - **(4) Stat-name `domain`-qualifier paragraph** — added `**Phase 24.2 per-route `domain`-qualifier disposition — 114 stays 114 (per AMEND-1):**` paragraph inside `## Stat-name mapping` right after the 24.1 `**Phase 24.1 extension — 110 → 114 internal names:**` paragraph. Documents that per-route `domain` is descriptor-tier (NOT a stat namespace); stat namespace UNCHANGED at 24.2; per-route stats SHARED with listener-level remains the 24.1 discipline; 0 stat-shape divergences + 0 new departures.
- **D-RL18 (atomic-landing ROADMAP rollup) execution.** Per the 18/19/22 sub-phase rollup precedent: row 24.2 flipped `planned → done` (date `2026-05-23`) AND parent row 24 flipped `in-progress → done` (date `2026-05-23`) IN THE SAME COMMIT. The commit-message body names BOTH transitions for grep-verifiability per the precedent: `"phase 24.2 Task 8: BEHAVIOR_CONTRACT completion bundle + ROADMAP rows 24.2 done + 24 done [ROLLUP per 18/19/22 precedent] + STATE re-advance + REVIEW.md"`. Sub-phases column for row 24 STAYS `24.1, 24.2` (unchanged per the precedent).
- **STATE.md re-advance.** Lifecycle-state `phase 24.2 PLAN done + plan-document-reviewer-APPROVED; awaiting 24.2 IMPL` → `phase 24.2 IMPL done; phase-24 family-row CLOSED at parent rollup; awaiting next planning (wasm BRAINSTORM not yet authored)`. Active-phase `24.2-...` → `awaiting next planning (wasm — final §9 family-row; not yet a ROADMAP row)`. Next-skill `superpowers:subagent-driven-development` → `superpowers:brainstorming`. ADR-0202 UNCONSUMED disposition recorded explicitly in the next-free-ADR field. `wasm` is NOT yet a ROADMAP row (confirmed via `grep -nE '^\| (wasm|25)' docs/envoy-go/ROADMAP.md` → no matches); STATE points at "awaiting next planning" + next-skill `superpowers:brainstorming` (to author the `wasm` BRAINSTORM.md + append the `wasm` ROADMAP row per the established §9 family-row addition discipline).
- **REVIEW.md authoring.** Per `superpowers:requesting-code-review` — REVIEW.md authored at `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/REVIEW.md` verifying the FULL parent §15 acceptance UNION:
  - §2.A items 1-6 (six gates) — FULL at 24.2 HEAD.
  - §2.B item 7 (two-directory differential) — 24.1 partial CLOSED at 24.2 (`0032` now has 9 scenarios incl. (d-extension) + (f) + (g) cross-side; `0033` boot-reject UNCHANGED; fixture dir count STAYS at 35).
  - §2.C item 8 (cluster-scoped 4-counter surface) — 110 → 114 STAYS at 114 (per-route `domain` is descriptor-tier).
  - §2.D item 9 (descriptor-engine fidelity) — 24.1 partial CLOSED at 24.2 (all 10 actions LANDED + empty-action-drop + AMEND-11 default-key roster + stage filter per §4.4 + Axis-B per §4.3 + Axis-A early-return per AMEND-4).
  - §2.E item 10 (PARSE-REJECT roster) — 24.1 FULL; ratified at 24.2 via per-policy stage > 10 + per-route validator recursion.
  - §2.F item 11 (disposition + reply byte-shape) — 24.1 FULL; reaffirmed at 24.2 with the Task-5 follow-up I-1 AMEND-8 wire-order fix.
  - §2.G item 12 (X-RateLimit DRAFT_VERSION_03 headers) — FULL at 24.2 HEAD (NEW at 24.2; D-RL9 RESOLVED CLEANLY).
  - §2.H item 13 (DELTA-1 + DELTA-2 + 19 HTTP filters) — 24.1 FULL; reaffirmed at 24.2 (`RouteMetadata()` accessor extends the ADR-0165 set-once template).
  - §2.I item 14 (`RateLimitPerRoute` 10th canonical) — FULL at 24.2 HEAD (NEW at 24.2; ADR-0125 roster 9 → 10).
  - §2.J item 15 (ADR landings) — 24.1 partial CLOSED at 24.2 (ADR-0197..0200 all LANDED across 24.1 + 24.2; ADR-0125 §(xv) AMENDMENT LANDED; ADR-0201 PLAN-time split consumed).
  - §2.K item 16 (BEHAVIOR_CONTRACT completion bundle) — 24.1 partial CLOSED at 24.2 (this Task 8 atomic landing).
  - §2.L item 17 (doc-state alignment) — FULL at 24.2 HEAD (STATE re-advanced + ROADMAP rows 24.2 + 24 BOTH `done` + BEHAVIOR_CONTRACT + DECISIONS aligned + 19 HTTP filters still wired + §9 family → 1 remaining row `wasm`).
  - §2.M item 18 (end-to-end audit-trail) — FULL at 24.2 HEAD (24.1 + 24.2 SPEC → PLAN → PROGRESS → REVIEW chains landed; D-hypothesis disposition recorded; six-gate verbatim outputs captured; per-task commit SHAs captured).
  - Plus §3 IMPL-time follow-ups (5 of them); §4 ADR roster; §5 Per-Task summary (16 commits); §6 known limitations (3 carry-forwards); §7 six-gate verbatim outputs; §8 parent-rollup status; §9 lessons learned (3); §10 forward-pointers (0 phase-24.2-emergent); §11 sign-off.
- **Task 7 fuzz_test.go header doc-drift cleanup** (per Task 7 review note carried forward to Task 8 per Task 8 brief). 3 lines edited:
  - Line 38: `"31 hand-curated f.Add seeds at 24.1 phase-done; extended at 24.2 Task 7 with ~14 additional seeds (45 total) covering:"` → `"...with 15 additional seeds (46 total) covering:"`.
  - Line 72: `"24.2 Task 7 extension (~14 new seeds; corpus only — no new fuzzer):"` → `"24.2 Task 7 extension (15 new seeds; corpus only — no new fuzzer):"`.
  - Line 470: `"24.2 Task 7 extension — Seeds 32-45 (corpus only; no new fuzzer)."` → `"24.2 Task 7 extension — Seeds 32-46 (corpus only; no new fuzzer)."`.

  ZERO functional change (test-only doc-drift cleanup).

- **Gate outputs (six gates verbatim per parent §15 items 1-6 + the differential 35-anchored regex + the 30s fuzz envelope + 53/53 h2spec):**

  **Gate A — `go build ./...`:**
  ```
  $ go build ./... 2>&1
  (empty)
  ---BUILD-EXIT: 0---
  ```

  **Gate B — `go vet ./...` + `golangci-lint run`:**
  ```
  $ go vet ./... 2>&1
  (empty)
  ---VET-EXIT: 0---

  $ golangci-lint run 2>&1
  (empty)
  ---LINT-EXIT: 0---
  ```

  **Gate C — `go test -race -count=1 ./...`:** Initial full-repo run flaked `0018-http-rbac` (documented multi-listener `freeTCPPort` flake class per 22.2 REVIEW §7.4):
  ```
  --- FAIL: TestDifferential (84.89s)
      --- FAIL: TestDifferential/0018-http-rbac (1.73s)
          runner_test.go:810: subj start: subject ready: EOF
  FAIL
  FAIL    github.com/esalaine/envoy-go/test/differential  87.049s
  ```
  Isolated re-run cleared the flake:
  ```
  $ go test -race -count=1 -run 'TestDifferential/0018-http-rbac' ./test/differential/
  ok      github.com/esalaine/envoy-go/test/differential  3.468s
  ---EXIT: 0---
  ```
  GREEN with documented flake disposition.

  **Gate D — `go test -count=1 -timeout 30m ./test/differential/ -run 'TestDifferential/00(0[0-9]|1[0-9]|2[0-9]|3[0-3])'`** (35/35 anchored regex):

  First run flaked `0020-http-ext-authz-http` (same `freeTCPPort` flake class):
  ```
  --- FAIL: TestDifferential (84.58s)
      --- FAIL: TestDifferential/0020-http-ext-authz-http (1.64s)
          runner_test.go:810: subj start: subject ready: EOF
  FAIL
  FAIL    github.com/esalaine/envoy-go/test/differential  84.668s
  ```
  Isolated re-run cleared the flake (`ok ... 2.060s`); a second `-v` full Gate-D run was clean — all 35/35 fixtures PASS in 85.4s:
  ```
  $ go test -count=1 -timeout 30m -v ./test/differential/ -run 'TestDifferential/00(0[0-9]|1[0-9]|2[0-9]|3[0-3])' 2>&1 | tail -5
      --- PASS: TestDifferential/0032-http-ratelimit (1.54s)
      --- PASS: TestDifferential/0033-http-ratelimit-boot-reject (1.36s)
  PASS
  ok      github.com/esalaine/envoy-go/test/differential  85.480s
  --- PASS: TestDifferential (85.40s)
  ---DIFF-EXIT: 0---
  ```

  **Gate E — `go test -count=1 -run 'FuzzRateLimitConfigParse' ./internal/filter/http/ratelimit/` + 30s live fuzz:**
  ```
  $ go test -count=1 -run 'FuzzRateLimitConfigParse' ./internal/filter/http/ratelimit/ 2>&1 | tail -10
  ok      github.com/esalaine/envoy-go/internal/filter/http/ratelimit     0.004s
  ---FUZZ-SEED-EXIT: 0---

  $ go test -run 'XXX_NONE' -fuzz 'FuzzRateLimitConfigParse' -fuzztime 30s ./internal/filter/http/ratelimit/ 2>&1 | tail -15
  fuzz: elapsed: 0s, gathering baseline coverage: 0/483 completed
  fuzz: elapsed: 3s, gathering baseline coverage: 378/483 completed
  fuzz: elapsed: 4s, gathering baseline coverage: 483/483 completed, now fuzzing with 32 workers
  fuzz: elapsed: 6s, execs: 250139 (83331/sec), new interesting: 4 (total: 487)
  fuzz: elapsed: 9s, execs: 542708 (97499/sec), new interesting: 4 (total: 487)
  fuzz: elapsed: 12s, execs: 768226 (75189/sec), new interesting: 5 (total: 488)
  fuzz: elapsed: 15s, execs: 958767 (63516/sec), new interesting: 6 (total: 489)
  fuzz: elapsed: 18s, execs: 1129080 (56753/sec), new interesting: 8 (total: 491)
  fuzz: elapsed: 21s, execs: 1280647 (50524/sec), new interesting: 8 (total: 491)
  fuzz: elapsed: 24s, execs: 1511758 (77040/sec), new interesting: 9 (total: 492)
  fuzz: elapsed: 27s, execs: 1647714 (45324/sec), new interesting: 9 (total: 492)
  fuzz: elapsed: 30s, execs: 1784920 (45732/sec), new interesting: 10 (total: 493)
  fuzz: elapsed: 31s, execs: 1784920 (0/sec), new interesting: 10 (total: 493)
  PASS
  ok      github.com/esalaine/envoy-go/internal/filter/http/ratelimit     31.073s
  ---FUZZ-30S-EXIT: 0---
  ```
  46-seed corpus clean (D-RL16: +15 new at 24.2 Task 7; 31 → 46); 1,784,920 execs in 30s; 10 new-interesting; 0 panics; 0 crashers. Project fuzzer count STAYS at **33** (corpus-only extension; D-RL16).

  **Gate F — h2spec conformance:**
  ```
  $ go test -v -count=1 ./test/conformance/h2spec/ 2>&1 | tail -25
      h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
      h2spec_test.go:187:   [PASS] 3.5. HTTP/2 Connection Preface: 2/2 passed
      h2spec_test.go:187:   [PASS] 4.1. Frame Format: 3/3 passed
      h2spec_test.go:187:   [PASS] 4.2. Frame Size: 3/3 passed
      h2spec_test.go:187:   [PASS] 4.3. Header Compression and Decompression: 3/3 passed
      h2spec_test.go:187:   [PASS] 5.1. Stream States: 13/13 passed
      h2spec_test.go:187:   [PASS] 5.1.1. Stream Identifiers: 2/2 passed
      h2spec_test.go:187:   [PASS] 5.1.2. Stream Concurrency: 1/1 passed
      h2spec_test.go:187:   [PASS] 5.3.1. Stream Dependencies: 2/2 passed
      h2spec_test.go:187:   [PASS] 5.4.1. Connection Error Handling: 2/2 passed
      h2spec_test.go:187:   [PASS] 5.5. Extending HTTP/2: 2/2 passed
      h2spec_test.go:187:   [PASS] 7. Error Codes: 2/2 passed
      h2spec_test.go:187:   [PASS] 8.1. HTTP Request/Response Exchange: 1/1 passed
      h2spec_test.go:187:   [PASS] 8.1.2. HTTP Header Fields: 1/1 passed
      h2spec_test.go:187:   [PASS] 8.1.2.1. Pseudo-Header Fields: 4/4 passed
      h2spec_test.go:187:   [PASS] 8.1.2.2. Connection-Specific Header Fields: 2/2 passed
      h2spec_test.go:187:   [PASS] 8.1.2.3. Request Pseudo-Header Fields: 7/7 passed
      h2spec_test.go:187:   [PASS] 8.1.2.6. Malformed Requests and Responses: 2/2 passed
      h2spec_test.go:187:   [PASS] 8.2. Server Push: 1/1 passed
  --- PASS: TestH2Spec (2.45s)
  PASS
  ok      github.com/esalaine/envoy-go/test/conformance/h2spec    2.541s
  ---H2SPEC-EXIT: 0---
  ```
  53/53 PASS at the ADR-0051 v1.32.4 pin.
- **D-hypothesis disposition (final).** **ADR-0202 UNCONSUMED at 24.2 phase-done — D-hypothesis HELD across the entire phase-24 family-row.** Both byte-confirmation surfaces resolved cleanly at their consuming tasks:
  - **D-RL8 (Task 1):** RESOLVED CLEANLY — the existing `DynamicMetadata()` + the NEW `RouteMetadata()` accessor (ADR-0165 set-once extension template) fit the existing primitives without divergence.
  - **D-RL9 (Task 5):** RESOLVED CLEANLY — byte-exact reproduction of the upstream `ratelimit_headers.cc:13-65` format pinned at `headers_test.go` AND verified cross-side byte-exact at fixture `0032-http-ratelimit` scenario (g).

  Next-free ADR stays at **ADR-0202** (DECISIONS.md tail at ADR-0201; ADR-0202 absent).
- **Outcome.** Phase 24.2 IMPL DONE at 2026-05-23 (this Task 8 atomic-landing commit). All 6 phase-done gates GREEN. The parent §15 acceptance UNION VERIFIED (18/18 items GREEN; 0 BLOCKED; 0 GREEN-WITH-NOTED-DEVIATION; 0 remaining PARTIAL annotations — all 24.1 partials CLOSED + all 24.2 new items LANDED). BEHAVIOR_CONTRACT.md 24.2 completion bundle landed atomically per ADR-0052 (4-item bundle per D-RL17). ROADMAP row 24.2 flipped `planned → done` AND parent row 24 flipped `in-progress → done` IN THE SAME COMMIT per D-RL18 + the 18/19/22 ROLLUP precedent. STATE.md re-advanced to `awaiting next planning (wasm — final §9 family-row; not yet a ROADMAP row)`; next-skill `superpowers:brainstorming`. REVIEW.md authored covering the full parent §15 acceptance UNION + six-gate verbatim outputs + D-RL8/D-RL9 outcomes + per-task commit SHAs. ADR-0202 UNCONSUMED — D-hypothesis HELD across the entire phase-24 family-row. The phase-24 family-row is CLOSED at this Task 8 commit. The next session is the post-`wt-merge` squash-merge + STATE SHA-fill follow-up + push-to-origin (per `feedback_git_worktrees.md` + `feedback_push_to_origin.md`); the session AFTER that is the `wasm` §9 final family-row BRAINSTORM authoring per `superpowers:brainstorming`.
- **Outcome:** TBD
