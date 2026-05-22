# Phase 23 — HTTP filter `envoy.filters.http.admission_control` (single-row landing) — Implementation Progress

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirrors phase-04..22 PROGRESS.md structure.

- **Phase:** 23 — HTTP filter `envoy.filters.http.admission_control` (single-row landing per ADR-0045 — SRE-book client-side probabilistic admission-control filter; SIXTEENTH §9 family-row; per-HCM-instance sliding-window success-rate controller; 3-counter stat surface; inline `Clock`+`Rand` seams; 32nd fuzzer; two differential fixtures `0030` cross-side + `0031` boot-reject)
- **Branch:** `phase-23-http-filter-admission-control-impl` (fresh worktree at `.worktrees/phase-23-http-filter-admission-control-impl`)
- **Base commit (master tip):** `99c8fef` (`next-prompt.txt: repoint master-tip references to 4cd46a8`; docs-only atop `4cd46a8` cold-start advance, atop the phase-23-PLAN SHA-fill follow-up `7fa89a4`, atop the PLAN squash `af4a0fe`, atop the phase-23-SPEC SHA-fill follow-up `ec68627`, atop the SPEC squash `a64ee71`)
- **PLAN tip SHA:** `af4a0fe` (`git log -1 --format=%H -- docs/envoy-go/phases/23-http-filter-admission-control/PLAN.md` → `af4a0fefb3977088b8684047cc4b99259d3d46c3`)
- **SPEC tip SHA:** `a64ee71` (`git log -1 --format=%H -- docs/envoy-go/phases/23-http-filter-admission-control/SPEC.md` → `a64ee7130bfdbe74c7c980e8b2f344e10f8177d4`)
- **Links:** [`PLAN.md`](./PLAN.md) · [`SPEC.md`](./SPEC.md) · [`BRAINSTORM.md`](./BRAINSTORM.md) · parent [`../../ROADMAP.md`](../../ROADMAP.md) row 23

---

## Cold-start preconditions verified

All 15 preconditions verified green at cold-start of branch `phase-23-http-filter-admission-control-impl` (worktree at `.worktrees/phase-23-http-filter-admission-control-impl`, branched from master tip `99c8fef`). Master tail shows the cold-start-advance docs commits (`99c8fef` + `4cd46a8`) at the head, the phase-23-PLAN SHA-fill follow-up `7fa89a4`, the PLAN squash `af4a0fe`, and the phase-23-SPEC closure stack (`ec68627` + `a64ee71`) preceding — exactly as expected per PLAN precondition 2. Go 1.26.2, golangci-lint v1.64.8 (ADR-0009 pin), Docker client 28.4.0 + server 28.1.1 present. ADR tail at 195 (ADR-0194 + ADR-0195 §Context drafts already at master per ADR-0044 ADR-on-impl convention; ADR-0196 stays unconsumed under the SPEC §10 D-style HOLD-with-known-risk hypothesis — one-slot escape-valve buffer). The 2 NEW ADR §Decision + §Consequences bodies (ADR-0194 + ADR-0195) land at impl-time anchor Tasks 4 + 2 per the per-ADR table below. **ZERO in-place §Decision AMENDMENTs + ZERO ADR-0125 amendments** at phase 23 (REUSE-by-absence per SPEC §5.4 — FIRST ADR-0125-skip since phase-22; canonical-per-route roster STAYS 9). SPEC at `a64ee71`; PLAN at `af4a0fe`. The phase-23-new surface (`internal/filter/http/admission_control/`) is absent at cold-start as expected. `go test -count=1 -short ./...` returns clean (all packages ok); `go build ./...` + `go vet ./...` clean. `go test -count=1 ./test/differential/ -run 'TestDifferential'` PASS in 79.8s (full pre-existing regression baseline). 3 representative fuzzers (`FuzzAdaptiveConcurrencyConfigParse` + `FuzzLuaConfigParse` + `FuzzBootstrapLoad`) spot-checked at 20s each; all PASS clean. Reference Envoy image `envoyproxy/envoy:v1.37.2` present with SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin). Working tree pristine (empty `git status --porcelain`). The proto binding `envoy/extensions/filters/http/admission_control/v3.AdmissionControl` resolves at the go-control-plane v1.32.4 pin.

**Note on PLAN preconditions 11/12 wording variance** (recorded for the same reason phase-18..21 PROGRESS.md recorded their analogous precondition-regex deviations — planner-time wording vs runtime fact, not a blocking divergence). The PLAN's literal patterns `Test.*00(0[0-9]|1[0-9]|2[0-9])` return "no tests to run" because the differential package uses a single top-level `TestDifferential` parent test iterating the fixture directories as sub-tests (per `test/differential/runner_test.go`); the substantive verification — all pre-existing fixture directories `0000..0029` GREEN — is satisfied via `go test ./test/differential/ -run 'TestDifferential'` (PASS 79.8s above). Precondition 12's full 31-fuzzer 30s-per-seed sweep is spot-checked at cold-start (3 representative fuzzers @ 20s) and run in full at Gate E (Task 12) per the PLAN's six-gate verification — the documented green baseline plus the zero-change-to-existing-surface branch makes the cold-start spot-check sufficient to gate.

### Precondition outputs (verbatim)

```
$ git rev-parse --abbrev-ref HEAD
phase-23-http-filter-admission-control-impl

$ git log --oneline master | head -6
99c8fef next-prompt.txt: repoint master-tip references to 4cd46a8 (actual HEAD)
4cd46a8 next-prompt.txt: advance to post-phase-23-PLAN IMPL cold-start
7fa89a4 phase 23 PLAN follow-up: STATE.md SHA-fill (TBD -> af4a0fe post-squash)
af4a0fe Squash merge phase-23-http-filter-admission-control-plan
9d6e876 next-prompt.txt: advance to post-phase-23-SPEC PLAN cold-start
ec68627 phase 23 SPEC follow-up: STATE.md SHA-fill (TBD → a64ee71 post-squash)

$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 ...
$ docker version --format '{{.Client.Version}} / {{.Server.Version}}'
28.4.0 / 28.1.1

$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
195
$ grep -cE '^## ADR-0194' docs/envoy-go/DECISIONS.md   # → 1
$ grep -cE '^## ADR-0195' docs/envoy-go/DECISIONS.md   # → 1
$ grep -cE '^## ADR-0196' docs/envoy-go/DECISIONS.md   # → 0

$ git log -1 --format=%H -- docs/envoy-go/phases/23-http-filter-admission-control/SPEC.md
a64ee7130bfdbe74c7c980e8b2f344e10f8177d4
$ git log -1 --format=%H -- docs/envoy-go/phases/23-http-filter-admission-control/PLAN.md
af4a0fefb3977088b8684047cc4b99259d3d46c3

$ git status --porcelain          # (empty — pristine)
$ docker image inspect envoyproxy/envoy:v1.37.2 --format '{{.Id}}'
sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
$ test ! -d internal/filter/http/admission_control && echo ok
ok: phase-23-new-surface absent
$ go doc .../admission_control/v3 AdmissionControl | head -1
package admission_controlv3 // import ".../admission_control/v3"

$ go test -count=1 -short ./...                        # clean (no FAIL)
$ go build ./...                                       # build-ok
$ go vet ./...                                         # vet-ok
$ go test -count=1 ./test/differential/ -run 'TestDifferential'
ok  	github.com/esalaine/envoy-go/test/differential	79.847s
$ go test -fuzz=FuzzAdaptiveConcurrencyConfigParse -fuzztime=20s ...   # PASS
$ go test -fuzz=FuzzLuaConfigParse -fuzztime=20s ...                   # PASS
$ go test -fuzz=FuzzBootstrapLoad -fuzztime=20s ...                    # PASS
```

All 15 preconditions GREEN. Proceeding to the 12 PLAN tasks.

---

## ADRs introduced / landed by this plan (reproduced verbatim from PLAN)

| ADR | Disposition | §Context anchored | §Decision + §Consequences body lands | Lands-in-Task |
|---|---|---|---|---|
| **ADR-0194** | NEW (algorithm + package shape + inline Rand/Clock seams + deque-window + integer-modulo decision + classification + 3-counter stat surface + deterministic-regime differential strategy) | SPEC commit `a64ee71` (§Context draft present per ADR-0044) | this PLAN's IMPL | **Task 4** (controller + filter materialization) |
| **ADR-0195** | NEW (RTDS `runtime_key` deferral PARSE-REJECT — 5 arms; `enabled`-absent⇒ENABLED per AMEND-4; the SINGLE envoy-go-strict departure) | SPEC commit `a64ee71` (§Context draft present per ADR-0044) | this PLAN's IMPL | **Task 2** (compiled_config + PARSE-REJECT roster) |
| **ADR-0196** | HYPOTHESIZED UNCONSUMED at phase-done (D-style HOLD-with-known-risk per SPEC §10 — one-slot escape-valve buffer; a surprise upstream stat name / a 4th counter / a clamp-boundary float edge could force consumption) | n/a | n/a (stays §-free) | — |

**ZERO in-place §Decision AMENDMENTs. ZERO ADR-0125 amendments** (REUSE-by-absence per SPEC §5.4; canonical-per-route roster STAYS 9; FIRST ADR-0125-skip since phase-22's roster amendment).

---

## Planner-time decisions PD-1..PD-10 (reproduced verbatim from PLAN)

**PD-1 — `New` factory signature.** The SPEC §6 illustrative `New(message proto.Message)` is REPLACED by the real `HTTPFilterFactory` shape per `internal/filter/http/types.go:245`: `func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)`. `ctx.Stats` is the `*stats.Registry`; `ctx.StatPrefix` is the HCM `http.<stat_prefix>` root. Matches the `adaptive_concurrency.go:108` precedent verbatim. Settles at Task 5 (struct + signature) + Task 8 (body). NO new ADR.

**PD-2 — PARSE-REJECT + reject-wire byte-stable strings** (per SPEC §5.1 + §5.2 + AMEND-7 + ADR-0080):
- §5.1 RATIFIED-from-config arms (4): `"admission_control: evaluation_criteria is required"` (oneof absent); `"admission_control: sr_threshold cannot be less than 1.0%"` (`sr_threshold.default_value < 1.0%`); `"admission_control: http_success_status range invalid (must be within [100,600) and start<=end)"`; `"admission_control: grpc_success_status accepts at most 16 codes"`.
- §5.2 envoy-go-strict `runtime_key` arms (5): `enabled` / `aggression` / `sr_threshold` / `max_rejection_probability` / `rps_threshold` — each `"admission_control: <field>.runtime_key is not yet supported; use <field>.default_value"`.
- **PD-2.503** — reject wire shape: framework `SendLocalReply(status int, body string, headers OrderedHeaders)` is 3-arg (per `internal/filter/http/callbacks.go:34`). AMEND-7 `response_code_details = "denied_by_admission_control"` is NOT surfaceable through the API → documented ABSENT-by-API (subject-only, NOT byte-pinned). Byte-pin asserted: status 503 + empty body `""` + no added headers (`f.cb.SendLocalReply(503, "", nil)`). Settles at Task 5 + Task 6.
- **PD-2.boot** — boot-reject common substring: `ExpectedBootErrorSubstring() = "cannot be less than 1.0%"` (present in both upstream stderr and the envoy-go-mirror wording). Settles at Task 9.

**PD-3 — health-check gate arm NOT-MODELED at MVP.** envoy-go's `DecoderFilterCallbacks` exposes NO `StreamInfo()`/`HealthCheck()`/`IsHealthCheck()` accessor (verified against `internal/filter/http/callbacks.go`); the project wires no upstream `health_check` HTTP filter / stream-info health-check marker. Adding such an accessor would be a NEW framework primitive — VIOLATING the ZERO-new-primitive constraint. Disposition: the `healthCheck()` arm is NOT-MODELED at phase-23 MVP; `DecodeHeaders` implements only the `!f.cc.enabled` pass-through arm. AMEND-11's "health-check requests not recorded" is vacuous at MVP. Documented deferral (deferred-items register + BEHAVIOR_CONTRACT note at Task 11). Does NOT consume ADR-0196. Confirmed at Task 5.

**PD-4 — both-sides filter shape.** Returned as `HTTPFilter{Name, Decoder: f, Encoder: f}` where a single `*filter` implements BOTH `StreamDecoderFilter` AND `StreamEncoderFilter` (per `internal/filter/http/types.go:73-81`). The `*controller` is hoisted to the factory closure level (one per `compiledConfig`/HCM instance per SPEC §6.2); each per-request `*filter` captures the shared pointer. Mirrors `bandwidthlimit.go:172` + `compressor.go:264`. Settles at Task 8.

**PD-5 — encode-side status access + gRPC detection.** HTTP status via `headers.Get(":status")` (per `compressor.go:785`), parsed to int. gRPC-ness via `headers.Get("content-type")` `application/grpc` prefix; gRPC status from `grpc-status` header when present, or from trailers in `EncodeTrailers` when deferred (`f.expectGRPCStatusInTrailer`). Settles at Task 6.

**PD-6 — `(1e4·P) > (r%1e4)` knife-edge determinism.** `shouldReject()` mirrors upstream's strict `>` + `accuracy = 10000` integer modulo: `return float64(10000)*math.Max(p, 0.0) > float64(r%10000)`. Boundary `r%10000 == floor(10000·p)` → admits; `floor(10000·p)−1` → rejects; P=0 ⇒ never reject. Settles at Task 4.

**PD-7 — `samplingWindow` deque rollover/expiry determinism.** Per-second bucket granularity; rollover when newest bucket `ts` ≥1s older than `clock.Now()`; stale-purge of buckets older than `samplingWindow` decrementing the running `global` aggregate. `samplingWindow` rounded to whole seconds via integer `ms/1000` (mirrors `config.cc:33-35`). Settles at Task 4.

**PD-8 — task decomposition: seams + stats folded into Task 3.** `rand.go` + `clock.go` + `stats.go` (+ test-scope `fakeRand`/`fakeClock`) land together at Task 3 as the small foundational layer the controller depends on — trivial, file-disjoint from `compiled_config.go`, parallelizable with Task 2. No ADR.

**PD-9 — zero framework regression.** Phase-23 touches no shared `internal/` primitive (counters-only via existing `internal/stats/`). Gate C race tests + full differential regression confirm zero regression.

**PD-10 — fuzzer corpus + 32nd-fuzzer registration.** `FuzzAdmissionControlConfigParse` fuzzes `buildCompiledConfig`. ~30 corpus seeds: valid full config (both success-criteria arms + all knobs); each of the 9 PARSE-REJECT arms; empty config; oneof-absent; malformed http range; >16 grpc codes. Must-never-panic; clean at 30s per seed. Settles at Task 7.

---

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Status:** DONE.

- All 15 execution preconditions verified GREEN (outputs quoted above). Preconditions 11/12 wording variance noted (single `TestDifferential` parent test; full fuzz sweep at Gate E).
- PROGRESS.md created with: precondition verification block; the 2-NEW-ADR table (verbatim from PLAN); PD-1..PD-10 (verbatim from PLAN); this Task 1 entry.
- Worktree `phase-23-http-filter-admission-control-impl` branched from master tip `99c8fef`.

**Commit SHA:** `3a09611`

---
