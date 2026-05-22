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

All 15 preconditions verified green at cold-start of branch `phase-23-http-filter-admission-control-impl` (worktree at `.worktrees/phase-23-http-filter-admission-control-impl`, branched from master tip `99c8fef`). Master tail shows the cold-start-advance docs commits (`99c8fef` + `4cd46a8`) at the head, the phase-23-PLAN SHA-fill follow-up `7fa89a4`, the PLAN squash `af4a0fe`, and the phase-23-SPEC closure stack (`ec68627` + `a64ee71`) preceding — exactly as expected per PLAN precondition 2. Go 1.26.2, golangci-lint v1.64.8 (ADR-0009 pin), Docker client 28.4.0 + server 28.1.1 present. ADR tail at 195 (ADR-0194 + ADR-0195 §Context drafts already at master per ADR-0044 ADR-on-impl convention; ADR-0196 stays unconsumed under the SPEC §10 D-style HOLD-with-known-risk hypothesis — one-slot escape-valve buffer). **[SUPERSEDED at Task 9a / Task 10: the D-hypothesis BROKE — ADR-0196 CONSUMED by the `ResponseStatus()` encode-side accessor; next-free ADR-0197. See ADR-0196 + Task 9a entry.]** The 2 NEW ADR §Decision + §Consequences bodies (ADR-0194 + ADR-0195) land at impl-time anchor Tasks 4 + 2 per the per-ADR table below. **ZERO in-place §Decision AMENDMENTs + ZERO ADR-0125 amendments** at phase 23 (REUSE-by-absence per SPEC §5.4 — FIRST ADR-0125-skip since phase-22; canonical-per-route roster STAYS 9). SPEC at `a64ee71`; PLAN at `af4a0fe`. The phase-23-new surface (`internal/filter/http/admission_control/`) is absent at cold-start as expected. `go test -count=1 -short ./...` returns clean (all packages ok); `go build ./...` + `go vet ./...` clean. `go test -count=1 ./test/differential/ -run 'TestDifferential'` PASS in 79.8s (full pre-existing regression baseline). 3 representative fuzzers (`FuzzAdaptiveConcurrencyConfigParse` + `FuzzLuaConfigParse` + `FuzzBootstrapLoad`) spot-checked at 20s each; all PASS clean. Reference Envoy image `envoyproxy/envoy:v1.37.2` present with SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin). Working tree pristine (empty `git status --porcelain`). The proto binding `envoy/extensions/filters/http/admission_control/v3.AdmissionControl` resolves at the go-control-plane v1.32.4 pin.

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

## Task 2 — `compiled_config.go` + 9-arm PARSE-REJECT roster + ADR-0195 §Decision + §Consequences

**Commit SHA:** `6ec193f` (corrected from a mis-recorded intermediate SHA in a Task-2 follow-up)
**Status:** DONE.

**Files landed:**
- `internal/filter/http/admission_control/compiled_config.go` (NEW; ~290 LoC including package-level doc + per-arm comments + helper functions)
- `internal/filter/http/admission_control/compiled_config_test.go` (NEW; ~490 LoC: 15 PARSE-REJECT rows + 13 default-applied rows + happy-path + nil-typed-config + unmarshal-failure + 5 TestEnabledMatrix rows + TestIsHTTPSuccess + TestIsGRPCSuccess + 9 byte-stable constant rows)
- `docs/envoy-go/DECISIONS.md` (MODIFY; ADR-0195 §Decision + §Consequences bodies anchored — REPLACES the SPEC-commit `_(Lands at phase-23 IMPL…)_` anticipation blocks per ADR-0044; **Status:** header updated from `§Context anchored…§Decision + §Consequences bodies land at phase-23 IMPL` → `Accepted — landed at phase-23 IMPL Task 2`; +~90 LoC net delta)
- `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (THIS Task 2 entry; ~80 LoC append)

**ADR landings:** ADR-0195 §Decision + §Consequences bodies (EXTENDS SPEC-commit §Context draft per ADR-0044 in-place edit discipline).

**D-questions closed:** PD-2 (9-arm PARSE-REJECT byte-stable roster materialized as `const` declarations + asserted byte-exact at `TestParseRejectConstants_ByteStable`).

### Build / test outputs (verbatim)

```
$ go build ./internal/filter/http/admission_control/...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run ./internal/filter/http/admission_control/...
(no output — clean)

$ go test -count=1 ./internal/filter/http/admission_control/... -run 'TestBuildCompiledConfig|TestEnabledMatrix|TestParseRejectConstants'
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.003s

$ grep -cE '^## ADR-0195' docs/envoy-go/DECISIONS.md
1
```

Verbose form (all sub-tests):

```
$ go test -count=1 ./internal/filter/http/admission_control/... -v
=== RUN   TestBuildCompiledConfig
=== RUN   TestBuildCompiledConfig/PARSE_REJECT
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm01_EvaluationCriteria_Absent
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm02_SrThreshold_BelowOnePercent
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm02_SrThreshold_ExactlyZero
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm03_HttpRange_StartBelowMin
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm03_HttpRange_EndAtOrAboveCeiling
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm03_HttpRange_StartGreaterThanEnd
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm04_GrpcCodes_MoreThan16
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm04_GrpcCodes_Exactly17
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm05_EnabledRuntimeKey_DefaultTrue
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm05_EnabledRuntimeKey_DefaultFalse
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm06_AggressionRuntimeKey
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm07_SrThresholdRuntimeKey
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm08_MaxRejectionProbabilityRuntimeKey
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm09_RpsThresholdRuntimeKey
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm01_EvaluationCriteria_Absent (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm02_SrThreshold_BelowOnePercent (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm02_SrThreshold_ExactlyZero (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm03_HttpRange_StartBelowMin (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm03_HttpRange_EndAtOrAboveCeiling (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm03_HttpRange_StartGreaterThanEnd (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm04_GrpcCodes_MoreThan16 (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm04_GrpcCodes_Exactly17 (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm05_EnabledRuntimeKey_DefaultTrue (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm05_EnabledRuntimeKey_DefaultFalse (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm06_AggressionRuntimeKey (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm07_SrThresholdRuntimeKey (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm08_MaxRejectionProbabilityRuntimeKey (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm09_RpsThresholdRuntimeKey (0.00s)
=== RUN   TestBuildCompiledConfig/Defaults
    --- PASS: TestBuildCompiledConfig/Defaults/SamplingWindow_Absent_Defaults30s (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/SamplingWindow_60s_Preserved (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/SamplingWindow_1500ms_Truncated1s (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/Aggression_Absent_Defaults1_0 (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/Aggression_BelowFloor_ClampsTo1_0 (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/Aggression_2_0_Preserved (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/SrThreshold_Absent_Defaults0_95 (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/SrThreshold_95pct_Fraction (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/SrThreshold_Above100pct_Clamped1_0 (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/RpsThreshold_Absent_Defaults0 (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/RpsThreshold_100_Preserved (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/MaxRejectionProbability_Absent_Defaults0_80 (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/MaxRejectionProbability_50pct_Fraction (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/HttpCriteria_Absent_DefaultRange100_500 (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults/GrpcCriteria_Absent_Default11Codes (0.00s)
--- PASS: TestBuildCompiledConfig/HappyPath (0.00s)
--- PASS: TestBuildCompiledConfig/NilTypedConfig (0.00s)
--- PASS: TestBuildCompiledConfig/UnmarshalFailure (0.00s)
--- PASS: TestBuildCompiledConfig (0.00s)
=== RUN   TestEnabledMatrix
    --- PASS: TestEnabledMatrix/Case1_Absent_ENABLED (0.00s)
    --- PASS: TestEnabledMatrix/Case2_Present_DefaultFalse_DISABLED (0.00s)
    --- PASS: TestEnabledMatrix/Case3_Present_DefaultTrue_ENABLED (0.00s)
    --- PASS: TestEnabledMatrix/Case4_RuntimeKey_PARSE_REJECT (0.00s)
    --- PASS: TestEnabledMatrix/Case1b_Present_DefaultValueAbsent_ENABLED (0.00s)
--- PASS: TestEnabledMatrix (0.00s)
=== RUN   TestIsHTTPSuccess
--- PASS: TestIsHTTPSuccess (0.00s)
=== RUN   TestIsGRPCSuccess
--- PASS: TestIsGRPCSuccess (0.00s)
=== RUN   TestParseRejectConstants_ByteStable
    --- PASS: TestParseRejectConstants_ByteStable/Arm01_EvalCriteriaRequired (0.00s)
    --- PASS: TestParseRejectConstants_ByteStable/Arm02_SrThresholdTooLow (0.00s)
    --- PASS: TestParseRejectConstants_ByteStable/Arm03_HttpRangeInvalid (0.00s)
    --- PASS: TestParseRejectConstants_ByteStable/Arm04_GrpcCodesExceed16 (0.00s)
    --- PASS: TestParseRejectConstants_ByteStable/Arm05_EnabledRuntimeKey (0.00s)
    --- PASS: TestParseRejectConstants_ByteStable/Arm06_AggressionRuntimeKey (0.00s)
    --- PASS: TestParseRejectConstants_ByteStable/Arm07_SrThresholdRuntimeKey (0.00s)
    --- PASS: TestParseRejectConstants_ByteStable/Arm08_MaxRejectionProbabilityRuntimeKey (0.00s)
    --- PASS: TestParseRejectConstants_ByteStable/Arm09_RpsThresholdRuntimeKey (0.00s)
--- PASS: TestParseRejectConstants_ByteStable (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.003s
```

### Code-quality follow-up (post-review fixes I1/I2/I3/M1/M3)

Applied code-quality fixes identified by reviewer; no behavior change.

- **I1** (`compiled_config.go` ~line 404): Fixed contradictory `isValidHTTPRange` doc comment — now correctly states `start` within `[100, 600)` and `end` within `[100, 600]`; the locked error string is unchanged.
- **I2** (`compiled_config_test.go` ~line 168): `Arm04_GrpcCodes_MoreThan16` now uses `make([]uint32, 20)` (was a duplicate of `Arm04_GrpcCodes_Exactly17`'s `make([]uint32, 17)`); the two cases now test distinct counts (20 = clearly more, 17 = just-over-16 boundary).
- **I3** (`compiled_config_test.go`): Added end-of-`TestIsHTTPSuccess` boundary block confirming `[500,600)` (`end==600`) is ACCEPTED, `isHTTPSuccess(599)` returns true, and `isHTTPSuccess(600)` returns false.
- **M1** (`compiled_config.go` ~line 214): Doc comment on `defaultHTTPSuccessRanges` fixed from singular `defaultHTTPSuccessRange` to match the plural identifier.
- **M3** (`compiled_config.go` ~line 381): Removed no-op `cc.rpsThreshold = 0` assignment; replaced with brief comment `// rpsThreshold defaults to 0 (zero value)`.

Build/vet/lint/test all clean after fixes (`go test -count=1 ./internal/filter/http/admission_control/...` → `ok ... 0.003s`).

**Follow-up commit:** `phase 23 Task 2 follow-up: code-quality fixes (I1 doc, I2 dup test, I3 boundary test, M1/M3 cleanup)`

### Code-quality follow-up (I-1/I-2/M-1..M-4 applied)

Applied code-quality fixes; no behavior change, no formula change, no public method signature change.

- **I-1** (`controller.go` `purgeStaleLocked`): Fixed real backing-array growth bug — the old `c.buckets = c.buckets[1:]` advanced the slice header without releasing the front of the backing array, causing unbounded accumulation over long-running controllers. Replaced with count-k-then-compact pattern using `copy(c.buckets, c.buckets[k:])` + `clear(tail)` + reslice to `[:n]`, so the backing array tracks the live window size.
- **I-2** (`controller.go` `averageRps`): Added comment above the `if ageSecs > denomSecs` branch explaining it is effectively unreachable post-purge for integer-second windows (every surviving bucket has age ≤ samplingWindow), but is retained to mirror upstream's `std::max()` semantics and guard against any future sub-second-window change.
- **M-1** (`controller_test.go`): Renamed `TestRpsSuppression_AgeDenominatorWins` → `TestRpsSuppression_SamplingWindowDenominator`; updated doc comment to accurately describe the sampling-window-wins path (age=2s < window=5s → denom=5s).
- **M-2** (`controller_test.go` `TestProbabilityFormula_AggressionExponentSkippedAt1_0`): Replaced silent-pass `t.Logf` fallback in the `else` branch with `t.Fatalf(...)` so a degenerate parameter choice fails loudly.
- **M-3** (`controller_test.go` `primeWindow`): Added `t *testing.T` parameter + `t.Helper()` + `if successes > requests { t.Fatalf(...) }` guard to prevent silent uint64 wraparound misuse; updated all call sites.
- **M-4** (`controller_test.go` `TestController_Concurrent_ClassifyAndShouldReject`): Replaced vacuous upper-bound-only check `n > goroutines*ops` (which can never fire) with exact equality assertion `n != uint64(goroutines*ops)` since every classify increments global.requests once and there is no time advance.

Build/vet/lint/test all clean after fixes (`go test -count=1 ./internal/filter/http/admission_control/...` → `ok ... 0.005s`; `go test -race -count=1 ...` → `ok ... 1.039s`).

**Follow-up commit:** `phase 23 Task 4 follow-up: code-quality fixes (I-1 bucket backing-array growth, I-2 averageRps comment, M-1..M-4 test hygiene)`

---

## ADRs introduced / landed by this plan (reproduced verbatim from PLAN)

| ADR | Disposition | §Context anchored | §Decision + §Consequences body lands | Lands-in-Task |
|---|---|---|---|---|
| **ADR-0194** | NEW (algorithm + package shape + inline Rand/Clock seams + deque-window + integer-modulo decision + classification + 3-counter stat surface + deterministic-regime differential strategy) | SPEC commit `a64ee71` (§Context draft present per ADR-0044) | this PLAN's IMPL | **Task 4** (controller + filter materialization) |
| **ADR-0195** | NEW (RTDS `runtime_key` deferral PARSE-REJECT — 5 arms; `enabled`-absent⇒ENABLED per AMEND-4; the SINGLE envoy-go-strict departure) | SPEC commit `a64ee71` (§Context draft present per ADR-0044) | this PLAN's IMPL | **Task 2** (compiled_config + PARSE-REJECT roster) |
| **ADR-0196** | ~~HYPOTHESIZED UNCONSUMED~~ **SUPERSEDED — CONSUMED at Task 9a** (D-hypothesis BROKE: PD-5 `:status`-via-header assumption was INVALID per differential fixture 0030; project owner approved the encode-side `ResponseStatus()` framework accessor fix; next-free advances to **ADR-0197**) | authored in-full at Task 9a | Task 9a | **Task 9a** (encode-side `ResponseStatus()` framework accessor; see also Task 10 audit) |

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

## Task 3 — `rand.go` + `clock.go` + `stats.go` seams + test-scope fakes

**Status:** DONE.

**Files landed:**
- `internal/filter/http/admission_control/rand.go` (NEW; ~55 LoC; `Rand` interface `Uint64() uint64` + `defaultRand` wrapping `math/rand/v2` package-level `rand.Uint64()` per SPEC §3.1 + AMEND-2; no `//nolint:unused` — `defaultRand` exercised by `TestDefaultRand_Sanity`)
- `internal/filter/http/admission_control/clock.go` (NEW; ~60 LoC; `Clock` interface `Now() time.Time` + `defaultClock` wrapping `time.Now` per SPEC §3.2; no `//nolint:unused` — `defaultClock` exercised by `TestDefaultClock_Sanity`; both seams wired into production `New` factory at Task 8)
- `internal/filter/http/admission_control/stats.go` (NEW; ~115 LoC; `filterStats` struct 3 `*stats.Counter` fields + package-level `const` stat names + `newFilterStats(reg *stats.Registry, hcmPrefix string) *filterStats` per SPEC §6.6 + AMEND-3; COUNTER-only, NO gauges)
- `internal/filter/http/admission_control/rand_test.go` (NEW; ~80 LoC; `fakeRand struct{ v uint64 }` + `Uint64()` returning `v` + compile-time interface check; `TestDefaultRand_Sanity` smoke test)
- `internal/filter/http/admission_control/clock_test.go` (NEW; ~120 LoC; `fakeClock struct{ mu sync.Mutex; now time.Time }` + `Now()` + `Advance(d time.Duration)`; 8 determinism tests covering start-anchor, cumulative advance, zero-advance no-op, 1s bucket boundary)
- `internal/filter/http/admission_control/stats_test.go` (NEW; ~120 LoC; `TestStatNames_Equal_*` byte-exact guards for all 3 stat-name constants + `TestStatNames_NotRqError` regression guard + `TestStatNames_Count` + `TestNewFilterStats_*` constructor prefix-wiring tests; **Task 3 OWNS the stat-name assertion — Task 5 does NOT re-assert**)
- `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (THIS Task 3 entry)

**Spec-compliance highlights:**
- `Rand.Uint64()` — NOT `Float64()` per AMEND-2 (integer-modulo reject decision)
- `Clock.Now()` ONLY — no `AfterFunc`, no timers per SPEC §3.2 (bucket rollover by wall-clock reads)
- Neither `defaultClock` nor `defaultRand` carries `//nolint:unused`; both are exercised by `TestDefaultClock_Sanity` / `TestDefaultRand_Sanity` respectively; both get wired into the production `New` factory at Task 8
- Stat names exactly `rq_rejected` / `rq_success` / `rq_failure` (NOT `rq_error` per AMEND-3); `TestStatNames_NotRqError` explicitly guards the AMEND-3 correction
- `fakeRand` and `fakeClock` live in `*_test.go` files only (test scope); not in production files

### Build / test / lint outputs (verbatim)

```
$ go build ./internal/filter/http/admission_control/...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run ./internal/filter/http/admission_control/...
(no output — clean)

$ go test -count=1 ./internal/filter/http/admission_control/... -run 'TestFakeClock|TestFakeRand|TestStatNames'
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.002s

$ go build ./...
(no output — build-ok)
```

Verbose acceptance test run:

```
$ go test -count=1 ./internal/filter/http/admission_control/... -run 'TestFakeClock|TestFakeRand|TestStatNames' -v
=== RUN   TestFakeClock_Now_ReflectsStart
--- PASS: TestFakeClock_Now_ReflectsStart (0.00s)
=== RUN   TestFakeClock_Advance_MovesNow
--- PASS: TestFakeClock_Advance_MovesNow (0.00s)
=== RUN   TestFakeClock_Advance_Cumulative
--- PASS: TestFakeClock_Advance_Cumulative (0.00s)
=== RUN   TestFakeClock_Advance_Zero
--- PASS: TestFakeClock_Advance_Zero (0.00s)
=== RUN   TestFakeClock_Now_Deterministic
--- PASS: TestFakeClock_Now_Deterministic (0.00s)
=== RUN   TestFakeClock_Advance_SubSecond
--- PASS: TestFakeClock_Advance_SubSecond (0.00s)
=== RUN   TestFakeClock_BucketBoundary
--- PASS: TestFakeClock_BucketBoundary (0.00s)
=== RUN   TestFakeRand_ReturnsConfiguredValue
--- PASS: TestFakeRand_ReturnsConfiguredValue (0.00s)
=== RUN   TestFakeRand_Deterministic
--- PASS: TestFakeRand_Deterministic (0.00s)
=== RUN   TestStatNames_Equal_RqRejected
--- PASS: TestStatNames_Equal_RqRejected (0.00s)
=== RUN   TestStatNames_Equal_RqSuccess
--- PASS: TestStatNames_Equal_RqSuccess (0.00s)
=== RUN   TestStatNames_Equal_RqFailure
--- PASS: TestStatNames_Equal_RqFailure (0.00s)
=== RUN   TestStatNames_NotRqError
--- PASS: TestStatNames_NotRqError (0.00s)
=== RUN   TestStatNames_Count
--- PASS: TestStatNames_Count (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.002s
```

**Commit SHA:** `a476be7`

### Code-quality follow-up (I-2/I-1/M-1/M-2 applied)

Applied code-quality fixes; no behavior change, no production seam change.

- **I-2** (`clock.go` + `clock_test.go`): Removed both `//nolint:unused` annotations (struct + method) and the now-obsolete NOTE comment from `clock.go`. Added `TestDefaultClock_Sanity` to `clock_test.go` — compile-time `var _ Clock = defaultClock{}` check + non-zero `Now()` assertion + monotonically-non-decreasing successive-call check, mirroring `TestDefaultRand_Sanity` style.
- **I-1** (`PROGRESS.md`): Corrected the Task 3 file-list entries: `rand.go` never carried `//nolint:unused`; `clock.go` no longer does after I-2. Updated both lines to state neither seam carries `//nolint` (both exercised by their respective sanity tests; production wiring at Task 8).
- **M-1** (`stats_test.go`): Replaced tautological `TestStatNames_Count` (compile-time `len([]string{…}) != 3` always false) with a meaningful guard: builds the slice from the three consts, asserts count==3 AND that all names are distinct (catches two consts accidentally equal). COUNTER-only / AMEND-3 intent documented in comment.
- **M-2** (`stats_test.go`): Replaced hand-rolled substring loop in `TestNewFilterStats_InfixLiteral` with `strings.Contains(name, infix)`; added `strings` import.

Build/vet/lint/test all clean after fixes (`grep -rn nolint internal/filter/http/admission_control/` → zero hits; `go test -count=1 ./internal/filter/http/admission_control/...` → `ok ... 0.004s`).

---

## Task 4 — `controller.go` + sliding-window + formula + integer-modulo decision + ADR-0194 §Decision + §Consequences

**Commit SHA:** `c05ea6a4e9046ce196764a41987742aa083a4451`
**Status:** DONE.

**Files landed:**
- `internal/filter/http/admission_control/controller.go` (NEW; ~220 LoC: `controller` + `bucket` structs per SPEC §6.2; `newController(cfg, stats, clock, rand)`; `recordRequest(success bool)` per §4.2 with purge/rollover/increment under `mu`; `requestCounts() (n, s uint64)` per §4.2; `averageRps() uint32` per §4.2 with `max(samplingWindow, age_of_oldest)` denominator; `shouldReject() bool` per §4.1 + AMEND-1 + AMEND-2 + PD-6; `classify(success bool)` per AMEND-11; `purgeStaleLocked` + `maybeRolloverLocked` helpers)
- `internal/filter/http/admission_control/controller_test.go` (NEW; ~600 LoC: 6 Layer A test families per SPEC §14.1; uses `fakeRand` from `rand_test.go` + `fakeClock` from `clock_test.go` — NOT redefined)
- `docs/envoy-go/DECISIONS.md` (MODIFY; ADR-0194 §Decision + §Consequences bodies + Status header update — EXTENDS SPEC-commit §Context draft per ADR-0044 in-place edit discipline; ~+200 LoC net delta)
- `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (THIS Task 4 entry)

**ADR landings:** ADR-0194 §Decision + §Consequences bodies (EXTENDS SPEC-commit §Context draft per ADR-0044 in-place edit discipline). `grep -cE '^## ADR-0194' docs/envoy-go/DECISIONS.md` → 1; §Decision body non-empty.

**SPEC §12 B4/B5 pin closures RATIFIED:**
- **B4 (knife-edge RATIFIED):** `TestShouldReject_Boundary_AtKnifeEdge_Admits` + `..._OneLessThanKnifeEdge_Rejects` — `P=0.80` (exact integer multiple `10000*0.80=8000.0`); `r%10000=8000` → `8000.0 > 8000` is FALSE → ADMIT; `r%10000=7999` → `8000.0 > 7999` is TRUE → REJECT; strict `>` confirmed byte-exact at equality.
- **B4 (P=0 cross-side RATIFIED):** `TestShouldReject_Boundary_PZero_NeverRejects` — P=0 (healthy window + empty window); 20 r-values including 0, 9999, 10000, 2^32-1, 2^63-1, ^uint64(0); `0 > (r%10000)` FALSE for all → NEVER REJECT. RNG-independent all-admit leg confirmed.
- **B5 (window determinism RATIFIED):** `TestController_FAKE_TIME_Window_*` — per-second bucket rollover at ≥1s boundary; stale-purge decrement from global; `requestCounts()` triggers purge; `averageRps()` uses `max(samplingWindow, age_of_oldest)` denominator in whole seconds.

**Note on test boundary arithmetic:** The boundary tests use `ceil(float64(10000)*P)` (not `floor`) as the first-admit r%10000 value for non-integer P values. This is mathematically correct: the reject decision `float64(10000)*P > float64(r%10000)` → REJECT when `r%10000 < 10000*P`, ADMIT when `r%10000 >= 10000*P`. When `10000*P` is fractional, `floor(10000*P)` still rejects; `ceil(10000*P)` is the first admitting integer. The P=0.80 knife-edge test uses an exact integer multiple (`10000*0.80=8000.0`), so `floor==ceil==8000` and the `r%10000==8000` admits at strict equality — matching the SPEC's "floor(1e4·P) admits" formulation exactly for integer-multiple P values.

### Build / test / lint outputs (verbatim)

```
$ go build ./internal/filter/http/admission_control/...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run ./internal/filter/http/admission_control/...
(no output — clean)

$ go test -count=1 ./internal/filter/http/admission_control/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.005s

$ go test -race -count=1 ./internal/filter/http/admission_control/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	1.036s

$ grep -cE '^## ADR-0194' docs/envoy-go/DECISIONS.md
1
```

Verbose test output (controller families only — full package passes):

```
=== RUN   TestShouldReject_Boundary_AtKnifeEdge_Admits
--- PASS: TestShouldReject_Boundary_AtKnifeEdge_Admits (0.00s)
=== RUN   TestShouldReject_Boundary_OneLessThanKnifeEdge_Rejects
--- PASS: TestShouldReject_Boundary_OneLessThanKnifeEdge_Rejects (0.00s)
=== RUN   TestShouldReject_Boundary_PZero_NeverRejects
--- PASS: TestShouldReject_Boundary_PZero_NeverRejects (0.00s)
=== RUN   TestShouldReject_Boundary_HighR_WithModulo
--- PASS: TestShouldReject_Boundary_HighR_WithModulo (0.00s)
=== RUN   TestProbabilityFormula_DefaultParams
--- PASS: TestProbabilityFormula_DefaultParams (0.00s)
=== RUN   TestProbabilityFormula_AggressionExponentSkippedAt1_0
--- PASS: TestProbabilityFormula_AggressionExponentSkippedAt1_0 (0.00s)
=== RUN   TestProbabilityFormula_SrThresholdDividesSuccesses
--- PASS: TestProbabilityFormula_SrThresholdDividesSuccesses (0.00s)
=== RUN   TestProbabilityFormula_AggressionFloor
--- PASS: TestProbabilityFormula_AggressionFloor (0.00s)
=== RUN   TestProbabilityFormula_MaxRejPClamp
--- PASS: TestProbabilityFormula_MaxRejPClamp (0.00s)
=== RUN   TestProbabilityFormula_MaxPZeroFloor
--- PASS: TestProbabilityFormula_MaxPZeroFloor (0.00s)
=== RUN   TestProbabilityFormula_VectorTests
--- PASS: TestProbabilityFormula_VectorTests (0.00s)
=== RUN   TestController_FAKE_TIME_Window_SingleBucket
--- PASS: TestController_FAKE_TIME_Window_SingleBucket (0.00s)
=== RUN   TestController_FAKE_TIME_Window_BucketRollover
--- PASS: TestController_FAKE_TIME_Window_BucketRollover (0.00s)
=== RUN   TestController_FAKE_TIME_Window_StalePurge
--- PASS: TestController_FAKE_TIME_Window_StalePurge (0.00s)
=== RUN   TestController_FAKE_TIME_Window_MultiSecondRollover
--- PASS: TestController_FAKE_TIME_Window_MultiSecondRollover (0.00s)
=== RUN   TestController_FAKE_TIME_Window_EmptyAfterFullPurge
--- PASS: TestController_FAKE_TIME_Window_EmptyAfterFullPurge (0.00s)
=== RUN   TestController_FAKE_TIME_Window_RequestCounts_Purges
--- PASS: TestController_FAKE_TIME_Window_RequestCounts_Purges (0.00s)
=== RUN   TestRpsSuppression_EmptyWindow_ReturnsZero
--- PASS: TestRpsSuppression_EmptyWindow_ReturnsZero (0.00s)
=== RUN   TestRpsSuppression_SingleSecond_EqualsCount
--- PASS: TestRpsSuppression_SingleSecond_EqualsCount (0.00s)
=== RUN   TestRpsSuppression_MultiSecond
--- PASS: TestRpsSuppression_MultiSecond (0.00s)
=== RUN   TestRpsSuppression_AgeDenominatorWins
--- PASS: TestRpsSuppression_AgeDenominatorWins (0.00s)
=== RUN   TestRecordDiscipline_Classify_Success
--- PASS: TestRecordDiscipline_Classify_Success (0.00s)
=== RUN   TestRecordDiscipline_Classify_Failure
--- PASS: TestRecordDiscipline_Classify_Failure (0.00s)
=== RUN   TestRecordDiscipline_Classify_Multiple
--- PASS: TestRecordDiscipline_Classify_Multiple (0.00s)
=== RUN   TestRecordDiscipline_RecordRequest_DoesNotIncrement_Stats
--- PASS: TestRecordDiscipline_RecordRequest_DoesNotIncrement_Stats (0.00s)
=== RUN   TestController_Concurrent_RecordAndCount
--- PASS: TestController_Concurrent_RecordAndCount (0.00s)
=== RUN   TestController_Concurrent_ClassifyAndShouldReject
--- PASS: TestController_Concurrent_ClassifyAndShouldReject (0.00s)
=== RUN   TestController_Concurrent_NoDeadlock
--- PASS: TestController_Concurrent_NoDeadlock (0.00s)
=== RUN   TestController_Concurrent_AverageRps
--- PASS: TestController_Concurrent_AverageRps (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.005s
```

---

## Task 5 — `admission_control.go` filter struct + `decode_headers.go` gate + reject wire shape

**Status:** DONE.

**Files landed:**
- `internal/filter/http/admission_control/admission_control.go` (NEW; ~170 LoC: `const TypeURL` byte-exact; `const filterName`; `filter` struct with all 5 PD-4 LOCKED fields (`cc`, `controller`, `stats`, `cb`, `record`, `expectGRPCStatusInTrailer`); compile-time assertions `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` + `var _ envoyhttp.StreamEncoderFilter = (*filter)(nil)`; `New` factory stub (full body already wired — Tasks 2-4 are landed — per the Task 5 PLAN note permitting early wiring); minimal encoder method stubs per strategy (b) — see below)
- `internal/filter/http/admission_control/decode_headers.go` (NEW; ~90 LoC: `DecodeHeaders` with 3-gate order: disabled→RPS-suppression→reject; `DecodeData`/`DecodeTrailers` Continue pass-throughs; `SetDecoderCallbacks`)
- `internal/filter/http/admission_control/admission_control_test.go` (NEW; ~220 LoC: `acCallbacks` stub + 6 test functions: `TestTypeURL_ByteExact`, `TestDecodeHeaders_Disabled_PassThrough`, `TestDecodeHeaders_RpsSuppression`, `TestDecodeHeaders_Reject_Increments_rqRejected`, `TestDecodeHeaders_Reject_SendLocalReply_503_EmptyBody`, `TestDecodeHeaders_Admit_PassThrough`)
- `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (THIS Task 5 entry)

**StreamEncoderFilter assertion strategy — choice (b) with documented rationale:**

Option (b) was chosen: minimal encoder method stubs (EncodeHeaders/EncodeData/EncodeTrailers/SetEncoderCallbacks pass-throughs + OnDestroy no-op) + BOTH compile-time assertions landed at Task 5. Rationale:

1. The `var _ envoyhttp.StreamEncoderFilter = (*filter)(nil)` assertion requires all 5 encoder methods. Without stubs, the build fails and the assertion cannot be included.
2. Adding both assertions NOW (Task 5) is safer than deferring to Task 6: a future reviewer can immediately verify the full interface contract without needing to split the conformance check across two tasks.
3. The adaptive_concurrency precedent (`filter.go`) includes BOTH interface assertions alongside the full encoder method set. Strategy (b) mirrors this exactly.
4. Each stub carries a clear `// TODO(Task 6): ...` comment documenting that the REAL classification logic is Task 6's spec-reviewed work.

The `expectGRPCStatusInTrailer` field carries `//nolint:unused` since it is declared now per PD-4 LOCKED struct contract but initialized + consumed only at Task 6.

**PD-3 health-check NOT-MODELED disposition:**

The `healthCheck()` arm is NOT-MODELED at phase-23 MVP. `internal/filter/http/callbacks.go` confirms: `DecoderFilterCallbacks` has no `StreamInfo()`, `HealthCheck()`, or `IsHealthCheck()` accessor. Adding such an accessor would be a new framework primitive — violating the ZERO-new-primitive constraint. Gate 1 in `DecodeHeaders` implements ONLY the `!f.cc.enabled` arm. The NOT-MODELED disposition is documented in the gate 1 comment in `decode_headers.go` + a forward-pointer to the Task 11 BEHAVIOR_CONTRACT note.

**Reject wire shape (AMEND-7 + PD-2.503):**

`f.cb.SendLocalReply(503, "", nil)` — status 503, EMPTY body `""`, nil headers. The `"denied_by_admission_control"` rc-details is NOT surfaceable through the 3-arg API (ABSENT-by-API per PD-2.503); not pinned in tests.

**Counter discipline:**

`rqRejected` is incremented at filter level (in `DecodeHeaders` gate 3), NOT inside `controller.shouldReject()`. This mirrors AMEND-11's record discipline: the controller owns only the window state; the filter owns the stat increments at the decode gate.

**RPS suppression gate (SPEC §6.4 gate 2):**

`if f.controller.averageRps() < f.cc.rpsThreshold { return Continue }` — admit WITHOUT consulting `shouldReject()`. `f.record` stays true (the request proceeds to encode-side classification). NOT a reject. Mirrors `admission_control.cc:87-91`.

### Build / test / lint outputs (verbatim)

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run ./internal/filter/http/admission_control/...
(no output — clean)

$ go test -count=1 ./internal/filter/http/admission_control/... -run 'TestDecodeHeaders|TestTypeURL|TestStatNames' -v
=== RUN   TestTypeURL_ByteExact
--- PASS: TestTypeURL_ByteExact (0.00s)
=== RUN   TestDecodeHeaders_Disabled_PassThrough
--- PASS: TestDecodeHeaders_Disabled_PassThrough (0.00s)
=== RUN   TestDecodeHeaders_RpsSuppression
--- PASS: TestDecodeHeaders_RpsSuppression (0.00s)
=== RUN   TestDecodeHeaders_Reject_Increments_rqRejected
--- PASS: TestDecodeHeaders_Reject_Increments_rqRejected (0.00s)
=== RUN   TestDecodeHeaders_Reject_SendLocalReply_503_EmptyBody
--- PASS: TestDecodeHeaders_Reject_SendLocalReply_503_EmptyBody (0.00s)
=== RUN   TestDecodeHeaders_Admit_PassThrough
--- PASS: TestDecodeHeaders_Admit_PassThrough (0.00s)
=== RUN   TestStatNames_Equal_RqRejected
--- PASS: TestStatNames_Equal_RqRejected (0.00s)
=== RUN   TestStatNames_Equal_RqSuccess
--- PASS: TestStatNames_Equal_RqSuccess (0.00s)
=== RUN   TestStatNames_Equal_RqFailure
--- PASS: TestStatNames_Equal_RqFailure (0.00s)
=== RUN   TestStatNames_NotRqError
--- PASS: TestStatNames_NotRqError (0.00s)
=== RUN   TestStatNames_Count
--- PASS: TestStatNames_Count (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.003s

$ go test -count=1 ./internal/filter/http/admission_control/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.005s

$ go test -count=1 -short ./...
(no FAIL lines — clean)
```

**Commit SHA:** `526a0cd`

### Code-quality follow-up (RPS test strengthened, stale Task-8 comments, EncodeData TODO)

Applied code-quality fixes; no behavior change, no gate logic change, no struct fields or New factory signature change.

- **RPS test strengthened** (`admission_control_test.go` `TestDecodeHeaders_RpsSuppression`): Changed injected rand from `neverRejectRand()` to `alwaysRejectRand()` and added `primeWindowForRejection(t, ctrl)` so P>0. Now genuinely proves that gate 2 (RPS-suppression) short-circuits before gate 3 (shouldReject) is consulted: with all 10 primed requests in one bucket at `time.Unix(0,0)`, `averageRps()=10/30=0 < rpsThreshold=10`, so Continue is returned even though `alwaysRejectRand` (r=0) + P>0 would cause shouldReject to return true if it were reached. Comment and code now agree.
- **Stale Task-8 comments removed** (`admission_control.go`): Three comments incorrectly claimed the `New` factory body / `HTTPFilter{Decoder:f,Encoder:f}` wiring was deferred to Task 8. Reworded file-header, `filter` struct comment, and interface-assertion comment to reflect that the wiring landed at Task 5 (Tasks 2-4 were already done); boot-registration + doc.go still at Task 8. Interface-assertion comment updated from "Task 8's … wiring" to "PD-4's … wiring".
- **EncodeData TODO removed** (`admission_control.go`): `// TODO(Task 6): no body logic anticipated; this stub is final.` was contradictory (a stub marked "final" inside a TODO is confusing). Changed to a plain comment: `// EncodeData is pass-through. No body-time logic anticipated (Task 6 confirms; see encode.go).`

Build/vet/lint/test all clean after fixes (`go test -count=1 ./internal/filter/http/admission_control/...` → `ok ... 0.006s`; `go test -race -count=1 ...` → `ok ... 1.043s`).

---

## Task 6 — `encode.go` classification + record discipline

**Commit SHA:** `ecd6d26f166c3d13fe982abc66b3be3019e24782`
**Status:** DONE.

**Files landed:**
- `internal/filter/http/admission_control/encode.go` (NEW; ~120 LoC: `SetEncoderCallbacks` no-op; `EncodeHeaders` HTTP/gRPC classification with record guard + gRPC-Content-Type detection + grpc-status header parse + HTTP `:status` parse; `EncodeData` pass-through; `EncodeTrailers` deferred-gRPC-trailers path; `OnDestroy` no-op)
- `internal/filter/http/admission_control/encode_test.go` (NEW; ~370 LoC: 5 test families covering HTTP classification, gRPC-in-headers classification, gRPC-trailers deferral, double-classify guard, record-discipline, reject byte-shape, EncodeData/SetEncoderCallbacks/OnDestroy pass-throughs)
- `internal/filter/http/admission_control/admission_control.go` (MODIFY: removed Task-5 encoder stubs (SetEncoderCallbacks, EncodeHeaders, EncodeData, EncodeTrailers, OnDestroy); removed `"net/http"` import now unused; removed `//nolint:unused` annotation from `expectGRPCStatusInTrailer` field; updated doc comments to reflect Task 6 landing)
- `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (THIS Task 6 entry)

**Stub removal confirmed:** All 5 Task-5 encoder stubs (SetEncoderCallbacks, EncodeHeaders, EncodeData, EncodeTrailers, OnDestroy) removed from `admission_control.go`; real implementations landed in `encode.go`. The compile-time assertion `var _ envoyhttp.StreamEncoderFilter = (*filter)(nil)` still passes because encode.go's methods are on the same `*filter` type.

**`//nolint:unused` removal confirmed:** `expectGRPCStatusInTrailer bool` field no longer carries `//nolint:unused` — it is now consumed by encode.go's `EncodeHeaders` (setter) and `EncodeTrailers` (reader/clearer).

**SPEC §12 B6 pin closure (gRPC-trailers):** The `expectGRPCStatusInTrailer` deferral (AMEND-10) is confirmed implemented and tested:
- `EncodeHeaders` with gRPC content-type + no `Grpc-Status` header → sets `f.expectGRPCStatusInTrailer = true`; no classify yet.
- `EncodeTrailers` with `f.expectGRPCStatusInTrailer = true` → parses `Grpc-Status` from trailers, classifies, clears flag.
- Double-classify guard: `EncodeHeaders` with `Grpc-Status` present in headers → classifies immediately; `f.expectGRPCStatusInTrailer` stays false → `EncodeTrailers` is a no-op. Tested by `TestClassification_GRPC_Trailers_NoDoubleClassify`.

**`TestRejectLocalReply_ByteShape` overlap note (per PLAN instruction):** This test is the §14.1 #7 canonically-named assertion. It intentionally overlaps `TestDecodeHeaders_Reject_SendLocalReply_503_EmptyBody` in `admission_control_test.go` (Task 5). Both assert the same SPEC §14.1 #7 byte-shape requirement (status 503, empty body, nil headers). The Task-6 `TestRejectLocalReply_ByteShape` additionally asserts: (a) `f.record=false` after reject per AMEND-11, and (b) `EncodeHeaders` is a no-op on the rejected request (record=false gates classify per AMEND-11).

**Double-classify guard implementation:** Implemented via the `f.expectGRPCStatusInTrailer` flag exclusively:
- Flag is set ONLY when gRPC response has no `Grpc-Status` header at `EncodeHeaders`.
- If `EncodeHeaders` classified (grpc-status in headers), flag stays false → `EncodeTrailers` is a no-op → no double-classify.
- The `f.record = false` guard additionally ensures EncodeTrailers skips classify on rejected/disabled requests even if the flag were somehow set.

**gRPC detection:** `strings.HasPrefix(headers.Get("Content-Type"), "application/grpc")` — covers `application/grpc`, `application/grpc+proto`, `application/grpc+json` per AMEND-10 + PD-5. Verified by `TestClassification_GRPC_Trailers_Failure` which uses `application/grpc+proto`.

### Build / test / lint outputs (verbatim)

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run ./internal/filter/http/admission_control/...
(no output — clean)

$ go test -count=1 ./internal/filter/http/admission_control/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.007s

$ go test -race -count=1 ./internal/filter/http/admission_control/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	1.043s
```

Verbose test output (encode families):

```
=== RUN   TestClassification_HTTP_DefaultSuccess_LT500
    --- PASS: TestClassification_HTTP_DefaultSuccess_LT500/status_Continue (0.00s)
    --- PASS: TestClassification_HTTP_DefaultSuccess_LT500/status_OK (0.00s)
    --- PASS: TestClassification_HTTP_DefaultSuccess_LT500/status_Created (0.00s)
    --- PASS: TestClassification_HTTP_DefaultSuccess_LT500/status_Moved_Permanently (0.00s)
    --- PASS: TestClassification_HTTP_DefaultSuccess_LT500/status_Not_Found (0.00s)
    --- PASS: TestClassification_HTTP_DefaultSuccess_LT500/status_ (0.00s)
--- PASS: TestClassification_HTTP_DefaultSuccess_LT500 (0.00s)
=== RUN   TestClassification_HTTP_DefaultFailure_GTE500
    --- PASS: TestClassification_HTTP_DefaultFailure_GTE500/status_500 (0.00s)
    --- PASS: TestClassification_HTTP_DefaultFailure_GTE500/status_502 (0.00s)
    --- PASS: TestClassification_HTTP_DefaultFailure_GTE500/status_503 (0.00s)
    --- PASS: TestClassification_HTTP_DefaultFailure_GTE500/status_504 (0.00s)
    --- PASS: TestClassification_HTTP_DefaultFailure_GTE500/status_599 (0.00s)
--- PASS: TestClassification_HTTP_DefaultFailure_GTE500 (0.00s)
--- PASS: TestClassification_HTTP_ConfiguredRange (0.00s)
--- PASS: TestClassification_HTTP_UnparsableStatus (0.00s)
--- PASS: TestClassification_HTTP_MissingStatus (0.00s)
=== RUN   TestClassification_GRPC_Headers_DefaultSuccessCodes
    --- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes/0 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes/1 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes/2 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes/3 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes/5 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes/6 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes/7 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes/9 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes/11 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes/12 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes/16 (0.00s)
--- PASS: TestClassification_GRPC_Headers_DefaultSuccessCodes (0.00s)
=== RUN   TestClassification_GRPC_Headers_DefaultFailureCodes
    --- PASS: TestClassification_GRPC_Headers_DefaultFailureCodes/4 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultFailureCodes/8 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultFailureCodes/10 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultFailureCodes/13 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultFailureCodes/14 (0.00s)
    --- PASS: TestClassification_GRPC_Headers_DefaultFailureCodes/15 (0.00s)
--- PASS: TestClassification_GRPC_Headers_DefaultFailureCodes (0.00s)
--- PASS: TestClassification_GRPC_Headers_EndStream (0.00s)
--- PASS: TestClassification_GRPC_Trailers_Success (0.00s)
--- PASS: TestClassification_GRPC_Trailers_Failure (0.00s)
--- PASS: TestClassification_GRPC_Trailers_NoDoubleClassify (0.00s)
--- PASS: TestClassification_GRPC_Trailers_NonGRPC_EncodeTrailers (0.00s)
--- PASS: TestRecordDiscipline_NotRecordedWhenRejected (0.00s)
--- PASS: TestRecordDiscipline_NotRecordedWhenRejected_GRPC (0.00s)
--- PASS: TestRecordDiscipline_NotRecordedWhenRejected_Trailers (0.00s)
--- PASS: TestRejectLocalReply_ByteShape (0.00s)
--- PASS: TestEncodeData_PassThrough (0.00s)
--- PASS: TestSetEncoderCallbacks_NoOp (0.00s)
--- PASS: TestOnDestroy_NoOp (0.00s)
```

**Code-quality follow-up** (applied post-landing): I1 — replaced dead `//nolint:errcheck` directives with `_ =` discards uniformly across all encode call sites; I2 — replaced hand-coded `formatHTTPStatus`/`formatGRPCCode` switch tables with stdlib one-liners (`strconv.Itoa` / `strconv.FormatUint`); I3 — unified HTTP subtest naming to numeric (`strconv.Itoa(code)` → `"status_100"` etc.) in both HTTP families; M1 — corrected `:status` parse comment in `encode.go` to say `ParseInt` with note of deliberate divergence from compressor.go; M2 — corrected `grpc-status` header-name comment to canonical `Grpc-Status`; M3 — removed obsolete `code := code` loop re-captures (go 1.22+ per-iteration vars); M4 — added `ctrl.requestCounts()` window-state assertions to one representative positive test per family (HTTP success, gRPC-headers success, gRPC-trailers success). All checks clean: `go build`, `go vet`, `golangci-lint`, `go test -count=1`, `go test -race -count=1`.

---

## Task 7 — `fuzz_test.go` + 32nd project-wide fuzzer + corpus seeds

**Commit SHA:** `7c206a0`
**Status:** DONE.

**Files landed:**
- `internal/filter/http/admission_control/fuzz_test.go` (NEW; ~170 LoC: `FuzzAdmissionControlConfigParse` with 31 `f.Add` seeds + fuzz body; 32nd project-wide fuzzer per SPEC §6.7 + PD-10)
- `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (THIS Task 7 entry)

**No testdata files committed:** Corpus is entirely `f.Add`-based per the phase-21 adaptive_concurrency precedent (`internal/filter/http/adaptive_concurrency/fuzz_test.go`). The Go fuzz engine's new-interesting inputs live in `$GOCACHE/fuzz/` (outside the source tree); no `testdata/fuzz/FuzzAdmissionControlConfigParse/` directory was created by the 30s run, consistent with the precedent.

**Seed roster (31 seeds, seed#0..seed#30):**
- Seed 0: Valid full config (both success-criteria arms + all knobs; happy path)
- Seed 1: Arm 1 (§5.1) — evaluation_criteria oneof absent
- Seed 2: Arm 2 (§5.1) — sr_threshold = 0.0 (< 1.0%)
- Seed 3: Arm 2 (§5.1) — sr_threshold = 0.5 (< 1.0%; near-boundary)
- Seed 4: Arm 3 (§5.1) — http range start > end (invalid)
- Seed 5: Arm 3 (§5.1) — http range end = 700 (out of [100,600) bounds)
- Seed 6: Arm 3 (§5.1) — http range start = 50 (below 100)
- Seed 7: Arm 4 (§5.1) — grpc_success_status 17 codes (> 16)
- Seed 8: Arm 5 (§5.2) — enabled.runtime_key non-empty
- Seed 9: Arm 6 (§5.2) — aggression.runtime_key non-empty
- Seed 10: Arm 7 (§5.2) — sr_threshold.runtime_key non-empty
- Seed 11: Arm 8 (§5.2) — max_rejection_probability.runtime_key non-empty
- Seed 12: Arm 9 (§5.2) — rps_threshold.runtime_key non-empty
- Seed 13: Empty AdmissionControl (arm 1 fires)
- Seed 14: SuccessCriteria present but empty sub-messages (defaults apply)
- Seed 15: enabled absent (AMEND-4 enabled=true default)
- Seed 16: sr_threshold = 1.0 exactly (boundary; passes arm 2)
- Seed 17: http range [100,600) (valid maximum end boundary)
- Seed 18: grpc exactly 16 codes (arm 4 boundary; passes)
- Seed 19: enabled present + default_value absent + runtime_key="" (AMEND-4 true)
- Seed 20: enabled default_value=false (pass-through per §5.3 case 2)
- Seed 21: grpc_criteria absent, http_criteria absent (all defaults fire)
- Seed 22: All optional wrappers absent (full defaults fire)
- Seed 23: sampling_window absent (default 30s)
- Seed 24: sampling_window = 60s (non-default; preserved)
- Seed 25: sampling_window = 1500ms (rounded to 1s via ms/1000 truncation)
- Seed 26: aggression = 0.5 (below floor; clamped to 1.0)
- Seed 27: sr_threshold > 100.0% (clamped at apply time)
- Seed 28: rps_threshold non-zero
- Seed 29: multiple valid http ranges (two ranges)
- Seed 30: Raw garbage bytes `{0xff,0xff,0xff,0xff,0xff}` (Unmarshal failure path)

**Fuzzer count: 31 → 32.**

### Build / test / lint outputs (verbatim)

```
$ go build ./internal/filter/http/admission_control/...
(no output — clean)

$ go vet ./internal/filter/http/admission_control/
(no output — clean)

$ golangci-lint run ./internal/filter/http/admission_control/...
(no output — clean)

$ go test -count=1 ./internal/filter/http/admission_control/... -run 'FuzzAdmissionControlConfigParse' -v
=== RUN   FuzzAdmissionControlConfigParse
=== RUN   FuzzAdmissionControlConfigParse/seed#0
=== RUN   FuzzAdmissionControlConfigParse/seed#1
=== RUN   FuzzAdmissionControlConfigParse/seed#2
=== RUN   FuzzAdmissionControlConfigParse/seed#3
=== RUN   FuzzAdmissionControlConfigParse/seed#4
=== RUN   FuzzAdmissionControlConfigParse/seed#5
=== RUN   FuzzAdmissionControlConfigParse/seed#6
=== RUN   FuzzAdmissionControlConfigParse/seed#7
=== RUN   FuzzAdmissionControlConfigParse/seed#8
=== RUN   FuzzAdmissionControlConfigParse/seed#9
=== RUN   FuzzAdmissionControlConfigParse/seed#10
=== RUN   FuzzAdmissionControlConfigParse/seed#11
=== RUN   FuzzAdmissionControlConfigParse/seed#12
=== RUN   FuzzAdmissionControlConfigParse/seed#13
=== RUN   FuzzAdmissionControlConfigParse/seed#14
=== RUN   FuzzAdmissionControlConfigParse/seed#15
=== RUN   FuzzAdmissionControlConfigParse/seed#16
=== RUN   FuzzAdmissionControlConfigParse/seed#17
=== RUN   FuzzAdmissionControlConfigParse/seed#18
=== RUN   FuzzAdmissionControlConfigParse/seed#19
=== RUN   FuzzAdmissionControlConfigParse/seed#20
=== RUN   FuzzAdmissionControlConfigParse/seed#21
=== RUN   FuzzAdmissionControlConfigParse/seed#22
=== RUN   FuzzAdmissionControlConfigParse/seed#23
=== RUN   FuzzAdmissionControlConfigParse/seed#24
=== RUN   FuzzAdmissionControlConfigParse/seed#25
=== RUN   FuzzAdmissionControlConfigParse/seed#26
=== RUN   FuzzAdmissionControlConfigParse/seed#27
=== RUN   FuzzAdmissionControlConfigParse/seed#28
=== RUN   FuzzAdmissionControlConfigParse/seed#29
=== RUN   FuzzAdmissionControlConfigParse/seed#30
--- PASS: FuzzAdmissionControlConfigParse (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#0 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#1 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#2 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#3 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#4 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#5 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#6 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#7 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#8 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#9 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#10 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#11 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#12 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#13 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#14 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#15 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#16 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#17 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#18 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#19 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#20 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#21 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#22 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#23 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#24 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#25 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#26 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#27 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#28 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#29 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#30 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.004s

$ go test -fuzz=FuzzAdmissionControlConfigParse -fuzztime=30s ./internal/filter/http/admission_control/
fuzz: elapsed: 0s, gathering baseline coverage: 0/31 completed
fuzz: elapsed: 0s, gathering baseline coverage: 31/31 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 161773 (53924/sec), new interesting: 80 (total: 111)
fuzz: elapsed: 6s, execs: 466440 (101540/sec), new interesting: 156 (total: 187)
fuzz: elapsed: 9s, execs: 830753 (121438/sec), new interesting: 186 (total: 217)
fuzz: elapsed: 12s, execs: 1108842 (92708/sec), new interesting: 195 (total: 226)
fuzz: elapsed: 15s, execs: 1553668 (148250/sec), new interesting: 202 (total: 233)
fuzz: elapsed: 18s, execs: 1801885 (82741/sec), new interesting: 209 (total: 240)
fuzz: elapsed: 21s, execs: 1965622 (54574/sec), new interesting: 211 (total: 242)
fuzz: elapsed: 24s, execs: 2233476 (89301/sec), new interesting: 212 (total: 243)
fuzz: elapsed: 27s, execs: 2575445 (113997/sec), new interesting: 213 (total: 244)
fuzz: elapsed: 30s, execs: 2889087 (104526/sec), new interesting: 214 (total: 245)
fuzz: elapsed: 31s, execs: 2889087 (0/sec), new interesting: 214 (total: 245)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	31.095s
```

No panics. No crashers. ~2.9M executions in 30s. 31 seeds gathered at baseline; 214 new-interesting inputs explored (all within Go fuzz cache, not committed).

**Code-quality follow-up:** I-1/I-2/I-3/M-1/M-3 comment fixes applied. Section banner "Seeds 2-10" corrected to "Seeds 2-13" (arms 2 and 3 carry extra boundary sub-variants); "Seeds 23-28" extended to "Seeds 23-30: All-defaults-applied + additional valid variants" (Seeds 29-30 no longer orphaned); Seed 25 inline comment updated to read "non-default value preserved as-is (no clamp/default)"; file header doc count corrected from "~30" to "31"; "i.e. 700" corrected to "e.g. 700" in Arm 3 out-of-bounds seed comment. Seed-corpus test still PASSes (`go test -count=1 -run FuzzAdmissionControlConfigParse`). All three quality gates (build/vet/lint) clean.

---

## Task 8 — Full filter integration + `doc.go` + boot-registration

**Commit SHA:** `dd5ab4e`
**Status:** DONE.

**Files landed:**
- `internal/filter/http/admission_control/doc.go` (NEW; ~20 LoC: package doc per SPEC §6.8 enumerating package purpose + TypeURL + per-HCM-instance sliding-window controller semantics + both-sides decode-gate/encode-classify discipline + 3-counter stat surface + ADR-0194/ADR-0195 cross-refs; styled after `adaptive_concurrency/doc.go` precedent)
- `cmd/envoy-go/main.go` (MODIFY; +1 import `admission_control` in alphabetical position between `adaptive_concurrency` and `bandwidthlimit`; +1 `httpReg.Register(admission_control.TypeURL, admission_control.New)` in matching alphabetical position; NO `RegisterPerRouteValidator` call — REUSE-by-absence per SPEC §5.4)
- `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (THIS Task 8 entry)

**`New` factory verification (Step 1):** Confirmed fully wired from Task 5. `admission_control.go` contains the complete `New` factory body: `tc != nil` guard → `buildCompiledConfig(tc)` → `ctx.Stats != nil` guard → `newFilterStats(ctx.Stats, "http."+ctx.StatPrefix)` → `newController(cc, st, defaultClock{}, defaultRand{})` → per-stream closure returning `HTTPFilter{Name: filterName, Decoder: f, Encoder: f}`. No change was made to `admission_control.go`.

**HTTP filter count: 17 → 18.** `grep -c 'httpReg.Register(' cmd/envoy-go/main.go` returns `18`. Confirmed: `router` + `adaptive_concurrency` + `admission_control` (NEW) + `bandwidthlimit` + `buffer` + `compressor` + `cors` + `csrf` + `envoygotest` + `extauthz` + `extproc` + `fault` + `header_mutation` + `jwtauthn` + `localratelimit` + `lua` + `oauth2` + `rbac` = 18 filters wired.

**Import + Register insertion points (verbatim from main.go after edit):**

```go
// import block:
"github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency"
"github.com/esalaine/envoy-go/internal/filter/http/admission_control"
"github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit"

// Register block:
httpReg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)
httpReg.Register(admission_control.TypeURL, admission_control.New)
httpReg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New)
```

**`grep -c 'admission_control.TypeURL' cmd/envoy-go/main.go`** → `1` (exactly the boot-registration line).

### Build / test / lint outputs (verbatim)

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run ./internal/filter/http/admission_control/... ./cmd/envoy-go/...
(no output — clean)

$ go test -count=1 ./internal/filter/http/admission_control/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.007s

$ grep -c 'admission_control.TypeURL' cmd/envoy-go/main.go
1

$ go build ./cmd/envoy-go/
(no output — clean)
```

---

## Task 9a — encode-side `ResponseStatus()` framework accessor (root-cause fix; ADR-0196)

**Commit SHA:** `d630cb3` (PROGRESS SHA backfilled via `--amend`; final SHA differs — see `git log`).

### Systematic-debugging root cause (PD-5 INVALID)

Phase-23 Task 9 differential bring-up of fixture **0030-http-admission-control** (the `all_admit_healthy` (b) cross-side leg) FAILED: every HTTP response misclassified as failure → the success-rate window collapsed → spurious 503 rejects on the envoy-go side that upstream did not produce. Root cause (per `superpowers:systematic-debugging`): planner-time decision **PD-5** assumed the encode-side HTTP response status was readable via `headers.Get(":status")`, modeled on the phase-14 compressor. That assumption is INVALID — envoy-go's encode chain does NOT convey the response status to encode-side filters. The status lives in `resp.Status` (an `int`) at HCM dispatch and is written to the wire status-line separately; the `http.Header` map handed to `RunEncodeHeaders(ctx, headers, endStream)` never contains `:status`. The compressor read it only as a best-effort optimization bucket (silent no-op when absent), so the gap never surfaced; admission_control needs it for CORE classification. The gRPC path (Content-Type + Grpc-Status header/trailer) is REAL and was never affected.

The project owner APPROVED a root-cause framework fix (a new accessor) over a filter-local workaround.

### Framework change (set-once-by-dispatch / read-via-accessor; mirrors ADR-0165/ADR-0174)

- `internal/filter/http/callbacks.go`: added `ResponseStatus() int` to the `EncoderFilterCallbacks` interface.
- `internal/filter/http/chain.go`: added the `FilterChain.encodeResponseStatus int` set-once field; the `func (c *FilterChain) SetEncodeResponseStatus(status int)` setter; the `func (e *encoderCB) ResponseStatus() int` accessor; and seeded `c.encodeResponseStatus = status` in the `beginLocalReply` path before its `RunEncodeHeaders` call (so a local-reply 503 from another filter classifies consistently).
- `internal/filter/hcm/connection.go` (H1) + `internal/filter/hcm/h2dispatch.go` (H2): added `chain.SetEncodeResponseStatus(status)` immediately before each `chain.RunEncodeHeaders(...)` action-path call (`status := resp.Status` already in scope).

### Filter fix (admission_control)

- `internal/filter/http/admission_control/admission_control.go`: added the `ecb envoyhttp.EncoderFilterCallbacks` field to the `*filter` struct.
- `internal/filter/http/admission_control/encode.go`: `SetEncoderCallbacks` now stores `f.ecb` (was a no-op); the HTTP classification path reads `f.ecb.ResponseStatus()` (an `int`) → `f.cc.isHTTPSuccess(code)` → `f.controller.classify(success)` instead of the absent `:status` header. Defensive `f.ecb == nil` guard + zero/unset status both default to failure (preserves PD-5's prior missing-status safe-default). gRPC path UNCHANGED. Doc comments updated to ADR-0196 (PD-5 superseded).
- `internal/filter/http/admission_control/encode_test.go`: added an `encodeTestCB` encode-side stub with a settable `status` field + `setEncodeHTTPStatus` helper; the HTTP classification tests now drive the status via the accessor (not a `:status` header); the former unparsable/missing-status tests became the nil-callbacks + zero-status defensive tests; all encode tests still assert controller counter + window changes.

### Test-stub conformance (a missing method is a compile error)

Added a `ResponseStatus() int` (zero-returning, or settable in the admission_control encode stub) to every test double implementing `EncoderFilterCallbacks`: `internal/filter/http/callbacks_test.go` (`fakeEncoderCB`), `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go` (`fakeEncoderCB`), `internal/filter/http/compressor/compressor_test.go` (`fakeCallbacks`), `internal/filter/http/extproc/extproc_test.go` (`fakeECB`; `ecbStub` inherits via embedding), `internal/filter/http/lua/lua_test.go` (`recordedECB`), `internal/filter/http/admission_control/encode_test.go` (`encodeTestCB`). The remaining listed files (`hcm/chain_dispatch_test.go`, `hcm/chain_integration_test.go`, `hcm/connection_test.go`, `http/chain_test.go`, `http/cors/cors_test.go`, `http/envoygotest/filter_test.go`, `http/types_test.go`) only ACCEPT `EncoderFilterCallbacks` as filter stubs and do not implement the interface — no change needed; `go build` + `go vet` confirmed.

### ADR-0196 consumption (next-free 0196 → 0197)

`docs/envoy-go/DECISIONS.md`: authored full ADR-0196 (§Context + §Decision + §Consequences) at the tail. REVISES the phase-23 "ZERO new framework primitives" headline — phase-23 introduces ONE new encode-side callback primitive. ADR-0196 CONSUMED; next-free advances to ADR-0197.

### Verification (verbatim)

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run ./internal/filter/http/... ./internal/filter/hcm/...
(no output — clean)

$ go test -count=1 -short ./...
(all packages ok / [no test files]; no FAIL)

$ go test -race -count=1 ./internal/filter/http/admission_control/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	1.043s

$ go test -count=1 -run 'TestDifferential/0030' ./test/differential/
PASS
ok  	github.com/esalaine/envoy-go/test/differential	1.915s
```

THE PROOF: fixture 0030 (the `all_admit_healthy` (b) cross-side leg) now PASSES — the spurious-503-reject misclassification regime is gone.

NOTE: this Task 9a entry does NOT commit `test/differential/` or `test/fixtures/` (the in-flight Task 9 fixture work stays uncommitted); only framework/filter/ADR/PROGRESS files are committed.

**Code-quality follow-up** (I-1/I-2/I-3/M-1/M-2/M-3 applied): I-1 — `beginLocalReply` now calls `c.SetEncodeResponseStatus(status)` (setter) instead of the direct field assignment, making all three seeding sites consistent. I-2 — deepened `ResponseStatus()` interface doc in `callbacks.go` with seeding-discipline + cross-phase-reusability prose (matching sibling depth). I-3 — stripped the verbose 6-line impl comment on `encoderCB.ResponseStatus()` in `chain.go` down to one line, matching the no-doc sibling convention. M-1 — replaced `+ PD-5` with `+ ADR-0196` in both citations in `encode.go` (file header + EncodeHeaders doc); PD-5 was superseded by ADR-0196. M-2 — dropped `(yet)` from the compressor stub comment in `compressor_test.go`; now reads `// ADR-0196; compressor reads :status via its best-effort header bucket, not this accessor.` M-3 — shortened the 7-line inline comment in `encode.go`'s HTTP path to two lines; implementation-detail prose belongs in chain.go not the filter. No behavior change. Build/vet/lint/short-tests/0030 all clean.

---

## Task 9 — differential fixtures 0030 + 0031 + BackendKind enum + runner switch

**Commit SHA:** `9622283`

**Depends on:** Task 9a `ResponseStatus()` framework fix (commit `85e03ba` — the encode-side HTTP classification accessor that makes the (b) cross-side byte-exact leg possible; without it every response was misclassified as failure, collapsing the success-rate window and generating spurious 503s on the envoy-go side only).

**Prior subagent note:** A prior Task 9 subagent authored the fixture files but stopped when it found the Task 9a filter bug. That bug was root-caused and fixed at Task 9a (commit `85e03ba`). This entry supersedes any incomplete Task 9 stop notes; the fixtures are now FINALIZED and GREEN.

### Files committed

- `test/differential/fixture/fixture.go` — `BackendKind HTTPAdmissionControl = 23` enum constant (+ comment)
- `test/differential/runner_test.go` — `case fixture.HTTPAdmissionControl:` switch arm wiring the `HTTPSlowStream` backend subprocess + blank import for the two fixture init packages
- `test/fixtures/0030-http-admission-control/` — flat cross-side fixture directory (4 scenarios):
  - `envoy.yaml` — reference Envoy bootstrap (template; 2 listeners: `l_test_a` full config + `l_test_d` enabled=false)
  - `envoy-go.yaml` — envoy-go bootstrap (template; same 2-listener topology)
  - `inputs/driver.go` — `acDriver`: `RequiresReference=true`, `BackendKind=HTTPAdmissionControl`; drives 4 scenarios via `driveScenarios`; `SubjectAsserter` does stats scrape + scenario (d) real dial
  - `expectations.yaml` — human-readable scenario matrix (documentation; not parsed by runner)
  - `README.md` — fixture README
- `test/fixtures/0031-http-admission-control-boot-reject/` — flat boot-reject fixture directory:
  - `envoy.yaml`, `envoy-go.yaml` — unused (inline bootstrap rendered by `renderBootRejectBootstrap`)
  - `inputs/driver.go` — `acBootRejectDriver`: `BootRejectFixture` interface; `ExpectedBootErrorSubstring() = "cannot be less than 1.0%"`; `sr_threshold.default_value.value = 0.5` trigger
  - `expectations.yaml`, `README.md` — documentation

**Fixture count:** 31 → 33

### 4 logical scenarios covered by 0030

Within a single flat fixture directory, the driver's `driveScenarios` function encodes all 4 scenarios sequentially in one byte stream using `=== scenario_<id> ===` block headers (matching the 0025 adaptive-concurrency multi-scenario flat pattern):

| Scenario | Disposition | Coverage in driver |
|---|---|---|
| **(a) `parse_ok`** | Subject-only structural | `driveScenarios` emits `scenario_a_parse_ok` block; `AssertSubject` calls `requireAllStatusInBlock(..., 200)` — 1 request, status 200; stats scrape confirms `rq_rejected==0` |
| **(b) `all_admit_healthy`** | **CROSS-SIDE byte-exact** (RATIFIED per AMEND-2 RNG-independence at P=0) | `driveScenarios` emits `scenario_b_all_admit_healthy` block; 5x `req_N: status=200` lines emitted IDENTICALLY on both ref + subj sides; `CompareBytes` byte-exact; `AssertSubject` confirms all 200 |
| **(c) `stat_surface`** | Subject-only structural | `driveScenarios` emits `scenario_c_stat_surface` block (3 requests); `AssertSubject` scrapes `/stats/prometheus` after the request sequence; verifies all 3 counters present (`rq_rejected`, `rq_success`, `rq_failure`) + `rq_rejected==0` + `rq_failure==0` |
| **(d) `pass_through_disabled`** | Subject-only structural | `driveScenarios` emits `scenario_d_pass_through_disabled` placeholder on BOTH sides (byte-exact `"placeholder: subject-only via SubjectAsserter\n"`); `AssertSubject` dials `l_test_d` directly (port = `subjListenerPort+1`) → asserts 200; scrapes stats → asserts `hcm_d.rq_rejected==0` |

All 4 logical scenarios per SPEC §7.1 are covered in the flat single-dir structure. No sub-directories added (harness flat-dir convention confirmed via 0025 precedent). No scenario was added or removed — the prior subagent's coverage was complete; only gofmt formatting was fixed.

### 0031 boot-reject RATIFIED on both sides

The `ExpectedBootErrorSubstring()` = `"cannot be less than 1.0%"` appears in:
- Reference Envoy v1.37.2 stderr: `"Success rate threshold cannot be less than 1.0%."` (config.cc:25-27 per AMEND-8)
- envoy-go stderr: `"admission_control: sr_threshold cannot be less than 1.0%"` (parseRejectSrThresholdTooLow constant)

### Verification (verbatim)

```
$ gofmt -l test/fixtures/0030-http-admission-control/ test/fixtures/0031-http-admission-control-boot-reject/
(no output — clean after gofmt -w fix)

$ go test -count=1 -run 'TestDifferential/0030' ./test/differential/
--- PASS: TestDifferential/0030-http-admission-control (1.86s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	1.940s

$ go test -count=1 -run 'TestDifferential/0031' ./test/differential/
Success rate threshold cannot be less than 1.0%.
[...envoy-go stderr: admission_control: sr_threshold cannot be less than 1.0%...]
--- PASS: TestDifferential/0031-http-admission-control-boot-reject (1.77s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	1.851s

$ go test -count=1 -run 'TestDifferential' ./test/differential/
--- FAIL: TestDifferential/0028-http-lua-multi-script-and-per-route (1.81s)
    runner_test.go:756: subj start: subject ready: EOF
NOTE: 0028 is the known freeTCPPort combined-run port-bind race flake (22.2 REVIEW §7.4).
      Isolated re-run: PASS (2.22s). All other 33 fixtures GREEN.

$ go test -count=1 -run 'TestDifferential/0028' ./test/differential/
--- PASS: TestDifferential/0028-http-lua-multi-script-and-per-route (2.22s)
PASS

$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run ./...
(no output — clean)
```

---

## Task 9 follow-up (review fixes)

A reviewer pass found that fixture-0030's scenario (a)/(c)/(d) assertions were **dead code** plus several documentation/YAML defects. All fixed in this follow-up. Filter implementation + framework untouched (committed + reviewed).

### Root cause — dead `AssertSubject` (the load-bearing finding)

0030 implemented `fixture.SubjectAsserter.AssertSubject` and put its (a)/(c)/(d) structural + 3-counter stat assertions there. **`AssertSubject` was NEVER CALLED for this fixture.** The runner invokes `SubjectAsserter` ONLY on the reference-less path (`runReferenceLessFixture`, `runner_test.go:1395`), which fires only when the driver implements `ReferenceLessFixture` AND `RequiresReference()==false`. 0030 is a REAL CROSS-SIDE fixture (live `DriveReference` + load-bearing `CompareBytes` for the (b) all_admit_healthy byte-exact leg), so `runFixture` takes the cross-side path (`runner_test.go:787-938`), which calls `DistributionAsserter`, `HTTPExpectations`, `StatsAsserter`, `AccessLogAsserter`, `AlternateConfigDriver` — but NOT `SubjectAsserter`. The test was passing purely on the (b) `CompareBytes` + admin diff; the (a)/(c)/(d) checks (and the secondary double-prefix bug in `scenarioBlock`) were vacuous.

### Fix — move subject-side assertions to `StatsAsserter` (which the cross-side path DOES call)

- Removed `AssertSubject` + the broken/dead `scenarioBlock` + `requireAllStatusInBlock` helpers + the `_ fixture.SubjectAsserter` compile-time assertion.
- Implemented `func (d *acDriver) AssertStats(t fixture.TB, _ refAdminAddr, subjAdminAddr string)` (the cross-side path invokes it at step 10, `runner_test.go:889-890`):
  - **(c) stat_surface:** scrape SUBJECT `/stats/prometheus`; assert the 3 counters (`rq_rejected`/`rq_success`/`rq_failure`) are present under `hcm_a`; assert `rq_rejected==0`, `rq_failure==0`, and **`rq_success > 0`** (the spec reviewer's missing-positivity finding — added a new `requireStatIsPositive` helper, with a shared `statValue` extractor that distinguishes absent-vs-present).
  - **(d) pass_through_disabled:** dial the SUBJECT `l_test_d` (disabled listener) FIRST so the disabled pass-through path is actually exercised, assert 200, then assert ALL THREE `hcm_d` counters `rq_rejected==0` AND `rq_success==0` AND `rq_failure==0` (the spec reviewer's I-3 — previously only `rq_rejected` was checked).
- (a)/(b) status correctness remains the runner's `CompareBytes` (both sides emit identical `status=200` lines); no separate status assertion is needed or wireable on the cross-side path. The l_test_d dial is subject-only (inside `AssertStats`), so the (b) cross-side byte stream stays identical on both sides (scenario (d) is still a fixed placeholder line in `driveScenarios`).
- Dropped the now-redundant `subjAdminPort` stash (the runner passes `subjAdminAddr` to `AssertStats`); kept `subjLDPort` (still needed for the (d) dial).

### Liveness proof (the whole point — no more vacuous assertions)

Deliberately broke ONE assertion (`requireStatIsPositive` → `requireStatIsZero` on `hcm_a/rq_success`) and re-ran:

```
$ go test -count=1 -run 'TestDifferential/0030' ./test/differential/
--- FAIL: TestDifferential (1.84s)
    --- FAIL: TestDifferential/0030-http-admission-control (1.84s)
        runner_test.go:890: stat hcm_a/rq_success: want 0, got 9
FAIL
FAIL	github.com/esalaine/envoy-go/test/differential	1.932s
```

The failure originates at `runner_test.go:890` (the `sa.AssertStats(t, ...)` call site), proving `AssertStats` executes on the cross-side path and observes the real subject counter (`rq_success==9` = the 9 healthy requests from scenarios a+b+c on `l_test_a`). Restored the correct assertion:

```
$ go test -count=1 -run 'TestDifferential/0030' ./test/differential/
ok  	github.com/esalaine/envoy-go/test/differential	2.034s
```

### Other reviewer findings fixed (same pass)

- **I-1 (§7.3 misattribution):** `0030/inputs/driver.go`, `0030/envoy.yaml`, `0030/README.md` cited "SPEC §7.3" as justifying the TWO-listener topology, but §7.3 prescribes a SINGLE listener. Reworded all sites to the real reasoning: "two listeners in one bootstrap to host the enabled (a-c) + disabled (d) configs; a single bootstrap (no MultiListenerDriver) avoids the freeTCPPort combined-run flake per 22.2 REVIEW §7.4 — a documented extension of SPEC §7.3's single-listener intent." Also rewrote the stale 0030 envoy.yaml comment block that described the (now-removed) reference-less/SubjectAsserter path.
- **I-3 (0030 scenario d):** now checks all three `hcm_d` counters (was `rq_rejected` only) — see Fix above.
- **I-4 (0031 invalid YAML):** `0031/expectations.yaml` line 21 `trigger: sr_threshold.default_value.value: 0.5  # (< 1.0%)` had an unquoted nested colon (invalid YAML). Quoted it: `trigger: "sr_threshold.default_value.value: 0.5  (< 1.0%)"`. Verified parses via `python3 yaml.safe_load`.
- **M-2 (0030 README phase labels):** fixed transposed/wrong phase labels to "fixture-0025 (phase 21)" / "fixture-0026 (phase 22.1)" / "fixture-0031 (phase 23)".
- **M-3 (0031 nonexistent script path):** `BootRejectScript()` returned `"scripts/boot_reject_config.yaml"`, which does not exist. The 0029 precedent's returned path (`scripts/bad_compile.lua`) DOES exist on disk as a symmetry artifact, but 0031 embeds its trigger entirely inline (no script file). Since the runner discards the return value, renamed the constant to a non-path description `bootRejectScriptDesc = "inline sr_threshold.default_value.value=0.5 (< 1.0%)"` and documented why it differs from 0029.
- **M-1 (0031 dead bootRejectMode field):** the 0029 precedent KEEPS the write-only `bootRejectMode` field, so 0031 keeps it for consistency. No change.

### Verification (verbatim)

```
$ go build ./...        (clean)
$ go vet ./...          (clean)
$ golangci-lint run ./test/...   (clean)

$ go test -count=1 -run 'TestDifferential/0030' ./test/differential/
ok  	github.com/esalaine/envoy-go/test/differential	2.034s

$ go test -count=1 -run 'TestDifferential/0031' ./test/differential/
ok  	github.com/esalaine/envoy-go/test/differential	1.753s

$ go test -count=1 -run 'TestDifferential' ./test/differential/
ok  	github.com/esalaine/envoy-go/test/differential	80.723s
(33 fixtures all PASS; no 0028 freeTCPPort flake on this combined run)
```

---

## Task 10 — ADR final-state alignment + DECISIONS.md cross-reference audit (ADR-0196 CONSUMED; D-hypothesis broke; next-free ADR-0197)

**Commit SHA:** `e212cb8`
**Status:** DONE.

**Files modified:**
- `docs/envoy-go/DECISIONS.md` (MODIFY: stale ADR-0194 §Consequences (e) "ADR-0196 UNCONSUMED" annotated as SUPERSEDED; no structural changes to ADR-0195 or ADR-0196)
- `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (THIS Task 10 entry; stale preamble line-16 "ADR-0196 stays unconsumed" annotated as SUPERSEDED; stale ADR table row for ADR-0196 updated to CONSUMED final state)

### Three-ADR final-state audit

**grep assertions:**
```
$ grep -cE '^## ADR-0194' docs/envoy-go/DECISIONS.md
1
$ grep -cE '^## ADR-0195' docs/envoy-go/DECISIONS.md
1
$ grep -cE '^## ADR-0196' docs/envoy-go/DECISIONS.md
1
$ grep -cE '^## ADR-0197' docs/envoy-go/DECISIONS.md
0
```

All three ADRs present exactly once; ADR-0197 absent (next-free unconsumed). ✓

**Non-empty §Decision + §Consequences bodies:**

| ADR | §Decision | §Consequences | Landed at |
|---|---|---|---|
| ADR-0194 | Non-empty — 9 lettered sub-items (i)–(ix) covering package shape + TypeURL + controller struct + sliding-window mechanics + rejection-probability formula + integer-modulo decision + inline seams + classification discipline + test taxonomy | Non-empty — 5 lettered items (a)–(e) covering ZERO-new-primitives (now SUPERSEDED/annotated at (e)) + clock-seam forward-pointer + stat surface + deterministic differential + ADR-0196 disposition | Task 4 |
| ADR-0195 | Non-empty — PARSE-REJECT 5-arm table + `enabled`-honored-matrix + numeric-knob defaults + unit-test coverage | Non-empty — 5 lettered items (a)–(e) covering operator migration + static-threshold config + departure count 14→15 + absent⇒ENABLED parity + Runtime/RTDS forward-pointer | Task 2 |
| ADR-0196 | Non-empty — 3 sub-items (i)–(iii): chain field/setter/accessor + HCM dispatch seeding (H1+H2+local-reply) + admission_control consumption | Non-empty — 5 lettered items (a)–(e): REVISES ZERO-new-primitives claim + ADR-0196 CONSUMED/next-free 0197 + 0030 (b) cross-side PASSES + PD-5 superseded + cross-phase reusable | Task 9a |

### Cross-reference integrity audit

Every ADR-XXXX reference cited in the §Decision + §Consequences bodies of ADR-0194, ADR-0195, ADR-0196 was verified against `grep -n "^## ADR-XXXX\b" docs/envoy-go/DECISIONS.md`. All resolved. Checked:

**ADR-0194 cross-refs:** ADR-0044 ✓, ADR-0045 ✓, ADR-0052 ✓, ADR-0072 ✓, ADR-0080 ✓, ADR-0100 ✓, ADR-0114 ✓, ADR-0125 ✓, ADR-0143 ✓, ADR-0186 ✓, ADR-0187 ✓, ADR-0195 ✓, ADR-0196 ✓.

**ADR-0195 cross-refs:** ADR-0044 ✓, ADR-0052 ✓, ADR-0072 ✓, ADR-0080 ✓, ADR-0187 ✓, ADR-0194 ✓.

**ADR-0196 cross-refs:** ADR-0044 ✓, ADR-0071 ✓, ADR-0131 ✓, ADR-0144 ✓, ADR-0165 ✓, ADR-0174 ✓, ADR-0175 ✓, ADR-0194 ✓.

**Result: ZERO dangling cross-references.** All cited ADR numbers resolve to existing headings in DECISIONS.md.

### D-hypothesis-broken disposition

The planner-time SPEC §10 hypothesis — "ADR-0196 stays unconsumed; next-free ADR-0196" — **BROKE** during differential bring-up at Task 9. Root cause: PD-5's assumption that the encode-side HTTP response status was available via `headers.Get(":status")` was INVALID. The HCM encode path writes the status to the wire status-line separately from the header map; encode-side filters never see `:status` in their header map. Differential fixture 0030 (b) cross-side leg surfaced the gap as spurious 503 rejects. The project owner approved the `ResponseStatus()` encode-side framework accessor fix at Task 9a.

**Final ADR count for phase-23: THREE** (ADR-0194 + ADR-0195 + ADR-0196 all CONSUMED). Next-free: **ADR-0197**.

### Stale-preamble reconciliation

Two historical stale claims were annotated (NOT destructively rewritten):

1. **PROGRESS.md preamble (former Task 1 / cold-start block, line 16):** "ADR-0196 stays unconsumed under the SPEC §10 D-style HOLD-with-known-risk hypothesis" — annotated inline with `[SUPERSEDED at Task 9a / Task 10: the D-hypothesis BROKE — ADR-0196 CONSUMED by the ResponseStatus() encode-side accessor; next-free ADR-0197. See ADR-0196 + Task 9a entry.]`

2. **PROGRESS.md ADR table row for ADR-0196 (line 225):** "HYPOTHESIZED UNCONSUMED" cell — replaced with "~~HYPOTHESIZED UNCONSUMED~~ SUPERSEDED — CONSUMED at Task 9a (D-hypothesis BROKE …)" preserving the historical hypothesis via strikethrough and recording the final-state.

3. **DECISIONS.md ADR-0194 §Consequences (e):** The "ADR-0196 UNCONSUMED at phase-done — HOLD-with-known-risk" item was annotated inline — the heading is struck through and a SUPERSEDED notice appended directing to ADR-0196 + Task 9a + Task 10.

The Task 9a PROGRESS.md entry already records ADR-0196 consumption authoritatively; Task 10 adds the formal audit + cross-reference verification layer.

### Build verification

```
$ go build ./...
(no output — clean)
```

Docs-only change; build unaffected. ✓

---

## Task 11 — BEHAVIOR_CONTRACT.md 4-edit bundle (departure 14→15; stats 107→110; PD-3 health-check NOT-MODELED; ADR-0196 ResponseStatus classification)

**Commit SHA:** `669d349`
**Status:** DONE.

**Files modified:**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (4-edit bundle per SPEC §13)
- `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (this entry)

### Four edits landed

**Edit 1 — NEW `### envoy.filters.http.admission_control` subsection** inserted after `### envoy.filters.http.lua` (and before `### Applies to`). Covers:
- Filter scope: SIXTEENTH §9 row; SRE-book client-side admission control; ADR-0194 + ADR-0195; returns to LEAN framework-delta posture with ONE new framework primitive (ADR-0196).
- Algorithm: P_reject formula + integer-modulo decision (`(1e4·P) > (r%1e4)` per AMEND-2); RPS-threshold gate (AMEND-1).
- Both-sides decode-gate/encode-classify discipline (AMEND-1/2/4/5/10/11): DecodeHeaders gate with `filterEnabled()` + rps-threshold + shouldReject; EncodeHeaders classify using `ResponseStatus()` accessor (HTTP) and `grpc-status` header/trailer (gRPC).
- PD-3 health-check arm NOT-MODELED note: no `StreamInfo().IsHealthCheck()` accessor in envoy-go's `DecoderFilterCallbacks`; AMEND-11 "health-check requests not recorded" vacuous at MVP; documented deferral.
- 503 / empty-body / `denied_by_admission_control` reject wire shape (AMEND-7 + D4; rc-details ABSENT-by-API per PD-2.503 at wire).
- 3-counter stat surface (AMEND-3): `rq_rejected` / `rq_success` / `rq_failure` HCM-rooted.
- REUSE-by-absence per-route discipline; FIRST ADR-0125-skip since phase-22; roster STAYS 9.
- SINGLE envoy-go-strict departure: RTDS `runtime_key` PARSE-REJECT (departure count 14 → 15).
- ADR-0196 `ResponseStatus()` note: ONE new encode-side framework primitive introduced at IMPL (supersedes SPEC-time ZERO-new-primitives plan claim; PD-5 `:status`-via-header assumption superseded).

**Edit 2 — RTDS `runtime_key` PARSE-REJECT departure record** embedded in the new subsection at the `#### envoy-go-strict departure` block. Explicitly states departure count 14 → 15; documents upstream ACCEPTS vs envoy-go PARSE-REJECTs for all four `Runtime*` wrappers.

**Edit 3 — Stat-name mapping 107 → 110 table extension:**
- Added `### 60-name table` header trailing entry for "extended by phase 23".
- Added `**admission_control filter — 3 names (introduced by phase 23; 3 counters):**` table with 3 rows (`rq_rejected`, `rq_success`, `rq_failure`).
- Added `**Phase 23 extension — 107 → 110 internal names:**` paragraph with full running tally.

**Edit 4 — Per-route canonical-patterns caption update + cross-reference paragraph:**
- Caption bumped: `"updated through phase 22.1; 9th canonical AMENDMENT-anticipation paragraph ..."` → `"updated through phase 23"`.
- New `**Phase 23 (admission_control) — FIRST ADR-0125-skip since phase-22's roster amendment**` paragraph appended documenting REUSE-by-absence; canonical-per-route roster STAYS 9; ADR-0195 records the explicit no-amendment classification.

### Acceptance-grep verification

```
$ grep -c '### envoy.filters.http.admission_control' docs/envoy-go/BEHAVIOR_CONTRACT.md
1

$ grep -n "departure count 14 → 15" docs/envoy-go/BEHAVIOR_CONTRACT.md
2603:#### envoy-go-strict departure — RTDS `runtime_key` PARSE-REJECT (per ADR-0195; departure count 14 → 15)

$ grep -n "107 → 110" docs/envoy-go/BEHAVIOR_CONTRACT.md
392:**Phase 23 extension — 107 → 110 internal names:** ...

$ grep -n "updated through phase 23" docs/envoy-go/BEHAVIOR_CONTRACT.md
3594:## Per-route canonical patterns cross-reference (ADR-0125 roster; updated through phase 23)
```

All 4 acceptance greps confirm the expected state.

### Key accuracy notes

- **ResponseStatus()-based classification (PD-5 superseded by ADR-0196):** The subsection accurately describes that HTTP classification on the encode side uses the `ResponseStatus() int` framework accessor (NOT a `:status` header read). This reflects the ACTUAL implemented behavior after Task 9a / ADR-0196. The subsection notes that phase 23 is NOT "zero new framework primitives" — it introduced ONE (ADR-0196 `ResponseStatus()`).
- **PD-3 health-check NOT-MODELED:** Subsection contains explicit note that the `healthCheck()` arm from AMEND-4 is NOT-MODELED at MVP; no `StreamInfo().IsHealthCheck()` accessor exists; AMEND-11 "health-check requests not recorded" is vacuous at MVP; documented deferral.

### Build verification

```
$ go build ./...
(no output — clean)
```

Docs-only change; build unaffected. ✓

---

## Task 12 — Six-gate phase-done verification + STATE/ROADMAP advance + REVIEW.md

**Commit SHA:** `6600e54`
**Status:** DONE.

**Files landed:**
- `docs/envoy-go/phases/23-http-filter-admission-control/REVIEW.md` (NEW; ~400 LoC: six-gate verbatim outputs + SPEC §15 16-item acceptance checklist + per-Task summary + 3 IMPL-time deviations narrative + ADR roster + sign-off)
- `docs/envoy-go/STATE.md` (MODIFY; rewrite-in-place to post-phase-23 state: `active-phase: to-be-determined-at-next-session`; `lifecycle-state: phase 23 IMPL done; awaiting next-phase identification`; `next-skill: superpowers:brainstorming`; `next-free ADR: ADR-0197`; 18 HTTP filters; 110 stats; 15 departures; 33 fixtures; 32 fuzzers; ADR-0196 consumed note)
- `docs/envoy-go/ROADMAP.md` (MODIFY; row 23 flipped `in-progress → done` + date `2026-05-22` + per-cell IMPL-done annotation documenting 12+9a-task landing + 3 ADR landings + 6-gate outputs + SPEC §15 summary + notable IMPL-time findings + §9 family closes to 2 remaining rows)
- `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (THIS Task 12 entry)

### Six-gate outputs (verbatim)

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

**Gate C — `go test -race -count=1 ./...`:** Single clean run (exit 0; no retry):
```
$ go test -race -count=1 ./... > /tmp/race-full.log 2>&1
RACE-EXIT: 0

$ grep -cE "^FAIL|^--- FAIL" /tmp/race-full.log
0

$ grep "^ok" /tmp/race-full.log | wc -l
62

$ grep "^ok" /tmp/race-full.log | grep -E "admission_control|hcm"
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.102s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.544s
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	1.092s
```

**Gate D — `go test -count=1 -timeout=15m ./test/differential/ -run 'TestDifferential'`:**
```
$ go test -count=1 -timeout=15m ./test/differential/ -run 'TestDifferential'
ok  	github.com/esalaine/envoy-go/test/differential	81.440s
---DIFF-EXIT: 0---
```
33/33 fixtures PASS. No 0028 freeTCPPort flake on this combined run.

**Gate E — Seed corpus + 30s fuzz:**
```
$ go test -count=1 -run 'FuzzAdmissionControlConfigParse' ./internal/filter/http/admission_control/ -v 2>&1 | tail -3
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.004s
---FUZZ-SEED-EXIT: 0---

$ go test -fuzz=FuzzAdmissionControlConfigParse -fuzztime=30s ./internal/filter/http/admission_control/
fuzz: elapsed: 0s, gathering baseline coverage: 0/262 completed
fuzz: elapsed: 2s, gathering baseline coverage: 262/262 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 231826 (77267/sec), new interesting: 3 (total: 265)
fuzz: elapsed: 6s, execs: 650362 (139493/sec), new interesting: 5 (total: 267)
fuzz: elapsed: 9s, execs: 1018490 (122708/sec), new interesting: 11 (total: 273)
fuzz: elapsed: 12s, execs: 1323682 (101727/sec), new interesting: 11 (total: 273)
fuzz: elapsed: 15s, execs: 1569208 (81846/sec), new interesting: 13 (total: 275)
fuzz: elapsed: 18s, execs: 1784477 (71752/sec), new interesting: 14 (total: 276)
fuzz: elapsed: 21s, execs: 1966112 (60552/sec), new interesting: 15 (total: 277)
fuzz: elapsed: 24s, execs: 2129752 (54551/sec), new interesting: 15 (total: 277)
fuzz: elapsed: 27s, execs: 2265659 (45292/sec), new interesting: 15 (total: 277)
fuzz: elapsed: 30s, execs: 2416440 (50265/sec), new interesting: 15 (total: 277)
fuzz: elapsed: 31s, execs: 2416440 (0/sec), new interesting: 15 (total: 277)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	31.059s
---FUZZ-30S-EXIT: 0---
```
31 seeds gathered at baseline; 2,416,440 execs in 30s; 0 panics. Total fuzzer count = 32.

**Gate F — h2spec conformance:**
```
$ go test -v -count=1 ./test/conformance/h2spec/
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
--- PASS: TestH2Spec (2.30s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.384s
---H2SPEC-EXIT: 0---
```
53/53 PASS at ADR-0051 pin.

### SPEC §15 16-item closure

All 16 items verified:
- Items 1-6 (six gates): all GREEN — see gate outputs above.
- Item 7 (fixture coverage): GREEN — 0030 (4 scenarios: (b) cross-side byte-exact AMEND-2 ratified; (a)/(c)/(d) subject-only via StatsAsserter + dial) + 0031 (boot-reject substring) GREEN at Gate D.
- Item 8 (3-counter stat surface): GREEN — `rq_rejected`/`rq_success`/`rq_failure` byte-exact per AMEND-3; 107 → 110 confirmed at BEHAVIOR_CONTRACT.md Task 11 edit 3.
- Item 9 (algorithmic fidelity): GREEN — 6 Layer A test families + race tests all clean at Gate C.
- Item 10 (PARSE-REJECT roster): GREEN — 9 arms (4 §5.1 + 5 §5.2) byte-stable per ADR-0080.
- Item 11 (503 reject wire shape): GREEN — `SendLocalReply(503, "", nil)` + `rqRejected.Inc()` per AMEND-7 + PD-2.503.
- Item 12 (enabled-matrix): GREEN — absent⇒ENABLED per AMEND-4; TestEnabledMatrix 5-case coverage.
- Item 13 (ADR landing): GREEN-WITH-NOTED-DEVIATION — 3 ADRs consumed (ADR-0194 + ADR-0195 + ADR-0196); 2 anticipated + 1 unanticipated (D-hypothesis BROKE).
- Item 14 (BEHAVIOR_CONTRACT bundle): GREEN — 4-edit atomic landing at Task 11.
- Item 15 (doc-state alignment): GREEN-WITH-NOTED-DEVIATION — next-free ADR-0197 (not ADR-0196; D-hypothesis BROKE); STATE.md + ROADMAP updated at this Task 12 commit.
- Item 16 (audit-trail): GREEN — SPEC → PLAN → PROGRESS → REVIEW chain complete; all task entries 1-12+9a present; D-hypothesis disposition recorded.

### ADR final-state verification
```
$ grep -cE '^## ADR-0194' docs/envoy-go/DECISIONS.md  # → 1
$ grep -cE '^## ADR-0195' docs/envoy-go/DECISIONS.md  # → 1
$ grep -cE '^## ADR-0196' docs/envoy-go/DECISIONS.md  # → 1
$ grep -cE '^## ADR-0197' docs/envoy-go/DECISIONS.md  # → 0
```
All 3 phase-23 ADRs present exactly once; ADR-0197 absent (next-free unconsumed). ✓
