# Phase 23 — HTTP filter `envoy.filters.http.admission_control` (single-row landing) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `envoy.extensions.filters.http.admission_control.v3.AdmissionControl` — the canonical Envoy v1.37.2 SRE-book client-side probabilistic admission-control filter (over a sliding `sampling_window` of `{requests, successes}` counts it computes a rejection probability and probabilistically short-circuits requests with HTTP 503 to shed load when the downstream success rate drops) — as the SIXTEENTH §9 family-row under the 07.1 framework by shipping the NEW `internal/filter/http/admission_control/` package (~9 production + ~6 test Go files; ~1000-1400 LoC production + ~700-1100 LoC tests per SPEC §6.8) with the per-HCM-instance sliding-window success-rate controller (a `std::deque`-of-per-second-buckets mirror per AMEND-6) + the inline `Clock` interface seam (`Now()`; NOT a new `internal/clock/` framework primitive per SPEC §3.2) + the NEW inline `Rand` interface seam (`Uint64()` — NOT `Float64()` — to mirror upstream's integer-modulo reject decision per AMEND-2; NOT a new `internal/rand/` framework primitive per SPEC §3.1) + the 3-counter HCM-rooted stat surface (`rq_rejected` / `rq_success` / `rq_failure` per AMEND-3; NO gauges; stat-prefix template `http.<HCM_stat_prefix>.admission_control.<stat>`) + the 32nd project-wide fuzzer `FuzzAdmissionControlConfigParse` + the two differential fixture directories `0030-http-admission-control` (cross-side; 4 scenarios — (b) all-admit `P_reject=0` byte-exact cross-side per AMEND-2 + (a)/(c)/(d) subject-only structural) + `0031-http-admission-control-boot-reject` (`sr_threshold < 1.0%` shared boot-reject per AMEND-8) + the listener-scoped-only enforcement via REUSE-by-absence (FIRST §9 row to skip ADR-0125 amendment since phase-22's roster amendment per SPEC §5.4; canonical-per-route roster STAYS 9), with **ZERO new `internal/` framework primitives** (returns to phase-21's LEAN posture) + **ZERO in-place §Decision AMENDMENTs** + **ZERO ADR-0125 amendments**, the boot-registration insertion at `cmd/envoy-go/main.go` (alphabetical between `adaptive_concurrency` and `bandwidthlimit`; 18 HTTP filters post-phase-23), the BEHAVIOR_CONTRACT.md 4-edit bundle per SPEC §13, and DECISIONS.md 2 NEW ADR §Decision + §Consequences bodies (ADR-0194 + ADR-0195) — with observable-outcomes byte-equivalent against reference Envoy v1.37.2 on the `P_reject=0` all-admit healthy-backend leg (RNG-independent per AMEND-2: `0 > (r % 1e4)` is false for every `r`) and stat-name byte-equivalent on the 3-counter surface, the probabilistic reject path covered subject-only at the unit layer (forced-reject byte-exact promotion impractical per AMEND-2 — requires ≥10000 primed failures in one window/worker), accepting the SINGLE documented envoy-go-strict departure (RTDS `runtime_key` PARSE-REJECT per ADR-0195; departure count 14 → 15). **Single-row landing settled at SPEC per ADR-0045** (LoC envelope ~1250-1800 production+fixture; PLAN-time re-evaluation per §Scope-check below CONFIRMS single-row — task count 12 well below the 25-gate; LoC straddles the ~1500 split-gate only at the upper grand-total estimate but natural split axes don't carve cleanly per SPEC §3.4 — the window controller + the FAKE-TIME/Rand tests are tightly coupled; framework delta is ZERO).

**Architecture:** The IMPL adds ONE new package (`internal/filter/http/admission_control/`) and touches NO shared `internal/` framework primitive — the LEANEST framework-delta posture (matches phase-21 adaptive_concurrency; the FIRST §9 row since phase-14 to introduce zero new `internal/` primitives, now repeated). The NEW package follows the phase-20/21 multi-file split: `doc.go` (package doc) + `admission_control.go` (TypeURL constant + `New` factory + per-stream `filter` struct + compile-time interface assertions) + `compiled_config.go` (`compiledConfig` + `buildCompiledConfig` + the 9-arm PARSE-REJECT roster with byte-stable wording per ADR-0080 — 4 RATIFIED-from-config arms per SPEC §5.1 + 5 envoy-go-strict `runtime_key` arms per SPEC §5.2; the AMEND-4 `enabled`-absent⇒ENABLED default) + `controller.go` (the per-HCM-instance sliding-window success-rate controller per SPEC §4 + §6.2 — a `[]bucket` deque of per-second `{ts, requests, successes}` buckets + a `global{requests, successes}` aggregate mutated under `sync.Mutex`; `clock.Now()` drives bucket rollover + stale-purge per AMEND-6; `recordRequest(success)` per §4.2; `requestCounts() (n, s)` per §4.2; `averageRps() uint32` per §4.2; `shouldReject() bool` applying the empirically-pinned formula `P_reject = max(0, min(max_rejection_probability, ((n − s/sr_threshold)/(n+1))^(1/aggression)))` per AMEND-1 + the integer-modulo decision `float64(10000)·math.Max(P,0) > float64(r%10000)` with `r := rand.Uint64()` per AMEND-2; `classify(success bool)` recording into the window + incrementing `rq_success`/`rq_failure`) + `rand.go` (in-package `Rand` interface — `Uint64() uint64` — + `defaultRand` wrapping `math/rand/v2`; NOT framework primitive per consumer-count=1 + YAGNI; `fakeRand` lives in test scope only at `rand_test.go`) + `clock.go` (in-package `Clock` interface — `Now() time.Time` — + `defaultClock` wrapping `time.Now`; NOT framework primitive — Clock-shaped consumer count reaches 2 with phase-21 but the shapes differ, so it stays inline per SPEC §2.5; `fakeClock` with `Advance(d)` lives in test scope only at `clock_test.go`) + `decode_headers.go` (`DecodeHeaders` per SPEC §6.4 — the `!f.cc.enabled` pass-through gate; the `controller.averageRps() < rps_threshold` suppression gate that admits-without-reject; the `controller.shouldReject()` reject decision that increments `rq_rejected` + emits `SendLocalReply(503, "", nil)` per AMEND-7 + sets `f.record = false`) + `encode.go` (`EncodeHeaders` + `EncodeData` pass-through + `EncodeTrailers` per SPEC §6.5 + §4.4 + AMEND-10 — classifies the upstream response per `success_criteria` [HTTP default all codes `<500`; gRPC default the 11-code well-known set per AMEND-5] and records via `controller.classify(...)`, EXCEPT when `f.record == false` [rejected / disabled] per AMEND-11; sets `f.expectGRPCStatusInTrailer` when a gRPC response defers its status to trailers) + `stats.go` (`filterStats` 3-counter roster + `newFilterStats(reg, hcmPrefix)` per SPEC §6.6 — `rq_rejected`/`rq_success`/`rq_failure` via `Registry.NewCounter` under `http.<HCM_stat_prefix>.admission_control.`; NO gauges per AMEND-3). The shared `*controller` instance is hoisted to the factory level (one per `compiledConfig` instance — i.e., one per HCM filter chain mounting an admission_control filter); each per-stream `*filter` captures the shared pointer. The filter is a **both-sides filter** returned as `HTTPFilter{Name, Decoder: f, Encoder: f}` (joins the encoder-side cohort: buffer / compressor / bandwidth_limit / ext_proc-body / adaptive_concurrency). Listener-scoped-only enforcement is REUSE-by-absence per SPEC §5.4: the `admission_control.v3` proto defines NO `AdmissionControlPerRoute` message at v1.32.4 or v1.37.x, so any route-level placement fails proto-deserialization-time PARSE-REJECT via the existing HCM framework — NO phase-23-specific PARSE-REJECT code; NO `RegisterPerRouteValidator` hook. Two algorithmic test layers per SPEC §14.1: **Layer A — subject-only FakeClock + FakeRand algorithmic-fidelity unit tests** at `controller_test.go` + `decode_headers_test.go` + `encode_test.go` + `compiled_config_test.go` (`TestShouldReject_Boundary_*` + `TestProbabilityFormula_*` + `TestController_FAKE_TIME_Window_*` + `TestRpsSuppression_*` + `TestClassification_HTTP_*`/`TestClassification_GRPC_*` + `TestRecordDiscipline_*` + `TestRejectLocalReply_ByteShape` + `TestBuildCompiledConfig_PARSE_REJECT_*` + `TestEnabledMatrix_*`); **Layer B — structural + cross-side differential fixtures** at `test/fixtures/0030-http-admission-control/` (4 scenario directories) + `test/fixtures/0031-http-admission-control-boot-reject/` (boot-reject). The phase-23 SPEC anchored 2 NEW ADRs (ADR-0194 + ADR-0195 §Context drafts) at the SPEC commit `a64ee71`; the IMPL lands the §Decision + §Consequences bodies at their respective Tasks per ADR-0044 (ADR-0194 at Task 4 controller materialization; ADR-0195 at Task 2 compiled_config). ADR-0044 escape-valve held in reserve for ~0-2 IMPL-time-unanticipated ADRs; PLAN's hypothesis per the SPEC §10 D-style HOLD-with-known-risk: ADR-0196 stays UNCONSUMED at phase-23 phase-done (one-slot escape-valve buffer).

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module (proto pin per ADR-0008; `envoy/extensions/filters/http/admission_control/v3` for the filter config — `AdmissionControl` message + `SuccessCriteria` sub-message at the only `evaluation_criteria` oneof arm; `RuntimeFeatureFlag` / `RuntimeDouble` / `RuntimePercent` / `RuntimeUInt32` wrappers); stdlib `math` (`math.Max` for the `max(P,0)` floor; `math.Min` for the `max_rejection_probability` clamp; `math.Pow` for the `^(1/aggression)` exponent, skipped at `aggression == 1.0`); stdlib `math/rand/v2` (for the production `defaultRand.Uint64()` per SPEC §3.1); stdlib `sync` (`sync.Mutex` guarding the window deque + global aggregate per SPEC §6.2); stdlib `time` (`time.Time` + `time.Duration` + `time.Now` for the production `defaultClock`; per-second bucket granularity per AMEND-6); reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 + ENVOY_TARGET.md — unchanged); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext upstream backend fixture (NO TLS surface at phase-23). **NO new go.mod direct deps** (LEANEST §9 row, matching phase-21).

---

## Scope check — why phase 23 ships as one row (single-row settled at SPEC per ADR-0045)

The phase-23 SPEC author settled the split disposition per SPEC §3.4 + ROADMAP row 23 + ADR-0045: **SINGLE-ROW landing** (no sub-rows `23.1`/`23.2`; precedent at phase-14 compressor, phase-17 jwt_authn, phase-20 oauth2, phase-21 adaptive_concurrency). The PLAN-time re-evaluation per the `superpowers:writing-plans` GATE + ADR-0045 §6 + SKILL_ROUTING state 2→3 GATE (split if PLAN > ~25 tasks OR > ~1500 LoC) CONFIRMS single-row landing:

- **Task count: 12** — comfortably under the ADR-0045 25-task split-gate (LEANER than phase-21's 14-task PLAN: phase-23 has no `internal/stats/conv.go` AMENDMENT task and no separate percentile-helper task).
- **LoC: ~1250-1800 production+fixture** (per SPEC §3.4 grand-total table) — production proper ~1000-1400; fixtures ~250-400. Straddles the ~1500-LoC split-gate only at the upper grand-total estimate. Per SPEC §3.4 the natural split axes **don't carve cleanly**: the sliding-window controller + the FAKE-TIME (`fakeClock`) window-rollover tests + the integer-modulo (`fakeRand`) boundary tests are tightly coupled (the boundary tests prime the window the controller owns); the framework delta is ZERO (no "framework vs filter" split axis — phase-23 IS just one filter with no framework primitives); the two fixture directories are mandated separate by the dispatch-constraint memory (one fixture dir = one runner branch) but co-land in one phase.
- **Phase 23 ships as the single row it is** — no further split. The phase-23 phase-done squash-merge CLOSES row 23 (`in-progress → done`) at the same commit; the §9 HTTP-filters family closes from 3 remaining rows (`wasm`, `admission_control`, `global rate limit`) to 2 (`wasm`, `global rate limit`).

Net change estimate for phase 23 (mirroring the phase-09..21 PLAN component-table convention):

- `internal/filter/http/admission_control/doc.go` ~20 (package doc per SPEC §6.8)
- `internal/filter/http/admission_control/admission_control.go` ~80-120 (TypeURL constant + `New` factory + per-stream `filter` struct + compile-time interface assertions; New factory body populated at Task 8 integration)
- `internal/filter/http/admission_control/compiled_config.go` ~220-300 (`compiledConfig` struct per SPEC §6.1 + `buildCompiledConfig` + 9-arm PARSE-REJECT roster per §5.1 + §5.2 + AMEND-4 enabled-absent⇒ENABLED default + http-range/grpc-code precompilation + `sampling_window` ms/1000 rounding)
- `internal/filter/http/admission_control/controller.go` ~250-350 (sliding-window deque controller per SPEC §4 + §6.2: `recordRequest` / `requestCounts` / `averageRps` / `shouldReject` formula + integer-modulo decision / `classify`)
- `internal/filter/http/admission_control/rand.go` ~25-50 (`Rand` interface + `defaultRand` per SPEC §3.1)
- `internal/filter/http/admission_control/clock.go` ~25-50 (`Clock` interface + `defaultClock` per SPEC §3.2)
- `internal/filter/http/admission_control/decode_headers.go` ~50-80 (`DecodeHeaders` gate + reject per SPEC §6.4 + AMEND-7 wire shape)
- `internal/filter/http/admission_control/encode.go` ~80-120 (`EncodeHeaders` + `EncodeData` pass-through + `EncodeTrailers` classification + record per SPEC §6.5 + §4.4 + AMEND-10 + AMEND-11)
- `internal/filter/http/admission_control/stats.go` ~30-50 (`filterStats` 3-counter roster + `newFilterStats` per SPEC §6.6 + AMEND-3)
- `internal/filter/http/admission_control/admission_control_test.go` ~80-120 (`TypeURL` constant assertion + `New` factory integration tests + decode-gate tests; stat names guarded in `stats_test.go` at Task 3)
- `internal/filter/http/admission_control/stats_test.go` ~30-60 (`TestStatNames_Equal_*` 3-counter byte-exact guards per AMEND-3)
- `internal/filter/http/admission_control/compiled_config_test.go` ~250-350 (`TestBuildCompiledConfig_PARSE_REJECT_*` table-driven + `TestEnabledMatrix_*` + default-applied tests per SPEC §14.1 #8 + #9)
- `internal/filter/http/admission_control/controller_test.go` ~300-450 (`TestShouldReject_Boundary_*` + `TestProbabilityFormula_*` + `TestController_FAKE_TIME_Window_*` + `TestRpsSuppression_*` + `TestRecordDiscipline_*` per SPEC §14.1 #1-#4 + #6; race tests for the `mu`-guarded window under concurrent record/classify)
- `internal/filter/http/admission_control/rand_test.go` ~40-80 (`fakeRand` test-scope implementation + `defaultRand` distribution sanity)
- `internal/filter/http/admission_control/clock_test.go` ~60-120 (`fakeClock` test-scope implementation + `Advance(d)` step driver + determinism tests)
- `internal/filter/http/admission_control/encode_test.go` ~150-250 (`TestClassification_HTTP_*` + `TestClassification_GRPC_*` [headers vs trailers] + `TestRejectLocalReply_ByteShape` per SPEC §14.1 #5 + #7 + AMEND-10)
- `internal/filter/http/admission_control/fuzz_test.go` ~50 (32nd fuzzer `FuzzAdmissionControlConfigParse` per SPEC §6.7)
- `internal/filter/http/admission_control/testdata/fuzz/FuzzAdmissionControlConfigParse/` (corpus seeds; ~30 seeds covering each PARSE-REJECT arm + valid full config + empty + oneof-absent variants)
- `cmd/envoy-go/main.go` ~+1 LoC + +1 import (`"github.com/esalaine/envoy-go/internal/filter/http/admission_control"`; `httpReg.Register(admission_control.TypeURL, admission_control.New)` inserted alphabetical between `adaptive_concurrency` [line 128] and `bandwidthlimit` [line 129]). **NO `RegisterPerRouteValidator` call** — REUSE-by-absence per SPEC §5.4. **18 HTTP filters wired post-phase-23.**
- `test/differential/fixture/fixture.go` ~+15 (NEW `BackendKind` enum value `HTTPAdmissionControl BackendKind = 23` after `HTTPLua = 22`)
- `test/differential/runner_test.go` ~+15 (blank import + switch-case for `HTTPAdmissionControl`)
- `test/fixtures/0030-http-admission-control/` (NEW DIRECTORY) — 4 scenario sub-directories (`parse_ok` / `all_admit_healthy` / `stat_surface` / `pass_through_disabled`) + shared `inputs/driver.go`; per-scenario `envoy.yaml` ~70-110 + `envoy-go.yaml` ~70-110 + `expectations.yaml` ~30-60 + `README.md` ~40-80. The (b) `all_admit_healthy` scenario includes the cross-side byte-exact expectations per AMEND-2. Subtotal ~200-350 LoC.
- `test/fixtures/0031-http-admission-control-boot-reject/` (NEW DIRECTORY) — boot-reject driver (`inputs/driver.go` implementing `BootRejectFixture` per the 0029 precedent) + `envoy.yaml` + `envoy-go.yaml` (both carrying `sr_threshold.default_value < 1.0%`) + `expectations.yaml` + `README.md`. Subtotal ~50-100 LoC.
- `docs/envoy-go/DECISIONS.md` — 2 NEW ADR §Decision + §Consequences bodies (ADR-0194 at Task 4 + ADR-0195 at Task 2); ~+200-300 LoC. NO new ADR numbers consumed at IMPL under the D-hypothesis (next-free stays ADR-0196).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` ~+150-250 (§13 4-edit bundle — NEW `### envoy.filters.http.admission_control` subsection + 1 envoy-go-strict departure record [14 → 15] + stat-table 107→110 extension + per-route cross-reference caption update)
- `docs/envoy-go/ROADMAP.md` row 23 flips `in-progress → done` at phase-done; per-cell IMPL-done annotation; ~+1 net
- `docs/envoy-go/STATE.md` rewrite-in-place at Task 12
- `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (NEW) ~500-800 across 12 task entries
- `docs/envoy-go/phases/23-http-filter-admission-control/REVIEW.md` (NEW) ~300

**Production code: ~1000-1400 LoC** (`internal/filter/http/admission_control/` production files + boot-registration + enum +2 net) **+ ~700-1100 LoC tests = ~1700-2500 LoC production+test** + ~250-450 LoC fixture + ~350-550 LoC docs ≈ **~2300-3500 LoC total**. **Task count is 12** — comfortably under the ADR-0045 25-task split-gate. Single-row landing settled.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/admission_control/doc.go` | NEW | Package doc per SPEC §6.8 — enumerates: package purpose (SRE-book client-side admission control); canonical TypeURL; the per-HCM-instance sliding-window controller semantics; the both-sides decode-gate/encode-classify discipline; the 3-counter stat surface; cross-reference to ADR-0194 + ADR-0195. ~20 LoC. |
| `internal/filter/http/admission_control/admission_control.go` | NEW | Main file. **Public surface:** `const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.admission_control.v3.AdmissionControl"` + `func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` (the real `HTTPFilterFactory` signature per `internal/filter/http/types.go:245` + planner-time PD-1 — NOT the SPEC §6 illustrative `New(message proto.Message)`). `New` calls `buildCompiledConfig(tc)`; constructs the `*filterStats` via `newFilterStats(ctx.Stats, ctx.StatPrefix)` + the shared `*controller` via `newController(cc, stats, defaultClock{}, defaultRand{})`; returns the `FilterInstanceFactory` closure producing per-request `HTTPFilter{Name, Decoder: f, Encoder: f}` (both-sides per PD-4). Per-stream `filter` struct (fields: `cc *compiledConfig`; `controller *controller`; `cb envoyhttp.DecoderFilterCallbacks`; `record bool` [default true; cleared on disabled / reject per AMEND-11]; `expectGRPCStatusInTrailer bool`). Compile-time interface assertions: `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` + `var _ envoyhttp.StreamEncoderFilter = (*filter)(nil)`. **NO `RegisterPerRouteValidator` function** — REUSE-by-absence per SPEC §5.4. ~80-120 LoC. New factory body completed at Task 8; struct + TypeURL + assertions land at Task 5. |
| `internal/filter/http/admission_control/compiled_config.go` | NEW | `compiledConfig` struct per SPEC §6.1 (verbatim — `enabled bool` + `samplingWindow time.Duration` + `aggression float64` + `srThreshold float64` + `rpsThreshold uint32` + `maxRejectionProbability float64` + `httpSuccessRanges []int32Range` + `grpcSuccessCodes map[uint32]struct{}`) + `buildCompiledConfig(typedConfig *anypb.Any) (*compiledConfig, error)` body covering the 9-arm PARSE-REJECT roster per PD-2 byte-stable wording (4 RATIFIED-from-config arms per §5.1 + 5 envoy-go-strict `runtime_key` arms per §5.2) + defaults per SPEC §6.1 + AMEND-4 (`enabled` true when the message is absent; `aggression` 1.0 floored; `srThreshold` 0.95; `rpsThreshold` 0; `maxRejectionProbability` 0.80; `samplingWindow` 30s via integer `ms/1000`; `httpSuccessRanges` default `{[100,500)}`; `grpcSuccessCodes` default the 11-code well-known set per AMEND-5). ~220-300 LoC. **ADR-0195 §Decision + §Consequences body anchored here at Task 2.** |
| `internal/filter/http/admission_control/controller.go` | NEW | Sliding-window success-rate controller per SPEC §4 + §6.2 (verbatim). `controller` struct (`cfg *compiledConfig`; `stats *filterStats`; `clock Clock`; `rand Rand`; `mu sync.Mutex`; `buckets []bucket`; `global struct{ requests, successes uint64 }`) + `bucket struct{ ts time.Time; requests, successes uint64 }`. Methods (all window mutation under `mu`): `recordRequest(success bool)` per §4.2 (purge stale buckets older than `samplingWindow` decrementing `global`; roll a new bucket if the newest is ≥1s old; increment back-bucket + `global`); `requestCounts() (n, s uint64)` per §4.2 (purge then return `global`); `averageRps() uint32` per §4.2 (`0` if empty; else `global.requests / max(samplingWindow_secs, age_of_oldest_sample_secs)`); `shouldReject() bool` per §4.1 (compute `{n,s}`; `p := max(0, min(maxRejectionProbability, ((n − s/srThreshold)/(n+1))^(1/aggression)))` per AMEND-1 [the `^(1/aggression)` `math.Pow` skipped when `aggression == 1.0`]; draw `r := rand.Uint64()`; return `float64(10000)*math.Max(p,0.0) > float64(r%10000)` per AMEND-2); `classify(success bool)` (calls `recordRequest(success)` + increments `stats.rqSuccess`/`stats.rqFailure`). ~250-350 LoC. **ADR-0194 §Decision + §Consequences body anchored here at Task 4.** |
| `internal/filter/http/admission_control/rand.go` | NEW | In-package `Rand` interface (`Uint64() uint64` per SPEC §3.1 + AMEND-2 — NOT `Float64()`) + `defaultRand struct{}` (`func (defaultRand) Uint64() uint64 { return rand.Uint64() }` wrapping `math/rand/v2`). NOT framework primitive (consumer count 1; YAGNI). ~25-50 LoC. ADR-0194 §Decision sub-paragraph (the inline-`Rand`-seam-`Uint64()`-not-`Float64()` decision) anchored at Task 4. |
| `internal/filter/http/admission_control/clock.go` | NEW | In-package `Clock` interface (`Now() time.Time` per SPEC §3.2) + `defaultClock struct{}` (`func (defaultClock) Now() time.Time { return time.Now() }`). NOT framework primitive — the Clock-shaped consumer count reaches 2 (phase-21 + phase-23) but the shapes differ (phase-21 `Now()`+`AfterFunc`; phase-23 `Now()`-only), so a shared `internal/clock/` extraction would over-fit (SPEC §2.5 + §8 item 8). ~25-50 LoC. ADR-0194 §Decision sub-paragraph (the inline-`Clock`-seam decision + the EXTRACT-NOW forward-pointer) anchored at Task 4. |
| `internal/filter/http/admission_control/decode_headers.go` | NEW | `DecodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus` per SPEC §6.4. Gate order: (1) `if !f.cc.enabled { f.record = false; return Continue }` (the `healthCheck()` arm is NOT-MODELED per PD-3 — envoy-go exposes no stream-info health-check marker); (2) `if f.controller.averageRps() < f.cc.rpsThreshold { return Continue }` (admit-without-reject; NOT a reject; the request proceeds + is classified at encode); (3) `if f.controller.shouldReject() { f.record = false; f.stats.rqRejected.Inc(); f.cb.SendLocalReply(503, "", nil); return StopIteration }` (the reject wire shape per AMEND-7 + PD-2.503 — empty body, nil headers; the `denied_by_admission_control` rc-details is NOT surfaceable through the framework's 3-arg `SendLocalReply` per PD-2.503 — documented ABSENT-by-API, NOT byte-pinned); (4) otherwise `return Continue`. `DecodeData` / `DecodeTrailers` are `Continue` pass-throughs. `SetDecoderCallbacks(cb)` stores `f.cb`. `OnDestroy()` is a no-op (no token to release — unlike phase-21). ~50-80 LoC. |
| `internal/filter/http/admission_control/encode.go` | NEW | `EncodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus` + `EncodeData(data []byte, endStream bool) FilterDataStatus` (pass-through) + `EncodeTrailers(trailers http.Header) FilterTrailersStatus` + `SetEncoderCallbacks(cb)` per SPEC §6.5 + §4.4 + AMEND-10 + AMEND-11. `EncodeHeaders`: if `!f.record` → pass-through `Continue` (rejected / disabled per AMEND-11). Else detect gRPC via `headers.Get("content-type")` prefix `application/grpc`; if gRPC and a `grpc-status` header is present (or `endStream`/trailers-only) → classify via `f.cc.isGRPCSuccess(status)`; if gRPC and the status is deferred to trailers → set `f.expectGRPCStatusInTrailer = true` (defer, no record). Else (HTTP) → parse `headers.Get(":status")` (per PD-5 — the `:status` pseudo-header is available, per `compressor.go:785` precedent) and classify via `f.cc.isHTTPSuccess(code)`. On classification → `f.controller.classify(success)`. `EncodeTrailers`: if `f.expectGRPCStatusInTrailer` → read `grpc-status` from trailers, classify+record. ~80-120 LoC. |
| `internal/filter/http/admission_control/stats.go` | NEW | `filterStats` struct (`rqRejected *stats.Counter`; `rqSuccess *stats.Counter`; `rqFailure *stats.Counter`) + `newFilterStats(reg *stats.Registry, hcmPrefix string) *filterStats` per SPEC §6.6 + AMEND-3. Constructs each via `reg.NewCounter(hcmPrefix + ".admission_control." + name)`. Package-level `const` declarations for the 3 stat names (`rq_rejected` / `rq_success` / `rq_failure`); a `TestStatNames_Equal_*` test asserts them byte-exact. NO gauges (the `ALL_ADMISSION_CONTROL_STATS` macro is `COUNTER`-only per AMEND-3). ~30-50 LoC. |
| `internal/filter/http/admission_control/admission_control_test.go` | NEW | `TypeURL` constant assertion + `New` factory integration tests + 3-stat-name byte-exact compile-time guards (`TestStatNames_Equal_*`). ~80-120 LoC. |
| `internal/filter/http/admission_control/compiled_config_test.go` | NEW | `TestBuildCompiledConfig_PARSE_REJECT_*` table-driven (~15-20 rows: 4 §5.1 arms + 5 §5.2 `runtime_key` arms + malformed/edge variants) + `TestEnabledMatrix_*` (per SPEC §5.3 + AMEND-4 — absent⇒ENABLED; default_value=false⇒DISABLED; default_value=true⇒ENABLED; runtime_key⇒PARSE-REJECT) + default-applied tests + `TestParseRejectConstants_ByteStable` per ADR-0080. ~250-350 LoC. |
| `internal/filter/http/admission_control/controller_test.go` | NEW | Layer A algorithmic-fidelity per SPEC §14.1 #1-#4 + #6: `TestShouldReject_Boundary_*` (the `(1e4·P) > (r%1e4)` knife-edge via injected `fakeRand`; `r%1e4 == floor(1e4·P)` admits, one less rejects; P=0 ⇒ never reject for any `r`); `TestProbabilityFormula_*` (vector tests over `{n,s,srThreshold,aggression,maxRejectionProbability}` — exponent skipped at aggression=1.0; sr_threshold-divides-successes; aggression floor 0.5→1.0; max-rej clamp; max(0,·) floor); `TestController_FAKE_TIME_Window_*` (per-second bucket rollover + stale-purge over `samplingWindow` via injected `fakeClock`; `requestCounts()` aggregate; `averageRps()` 0-when-empty / n/secs); `TestRpsSuppression_*` (`averageRps() < rpsThreshold` admits-without-reject); `TestRecordDiscipline_*` (disabled/rejected not recorded per AMEND-11); plus `TestController_Concurrent_*` race tests for the `mu`-guarded window under concurrent record/classify. ~300-450 LoC. |
| `internal/filter/http/admission_control/rand_test.go` | NEW | `fakeRand` test-scope implementation (`fakeRand struct{ v uint64 }`; `func (r fakeRand) Uint64() uint64 { return r.v }`) + a `defaultRand` distribution sanity test. ~40-80 LoC. |
| `internal/filter/http/admission_control/clock_test.go` | NEW | `fakeClock` test-scope implementation (`fakeClock struct{ now time.Time }`; `Now() time.Time`; `Advance(d time.Duration)`) + determinism tests. ~60-120 LoC. |
| `internal/filter/http/admission_control/encode_test.go` | NEW | `TestClassification_HTTP_*` (default `<500` success + configured `[100,600)` ranges) + `TestClassification_GRPC_*` (default 11-code set + configured lists; gRPC-status-in-headers vs in-trailers `expectGRPCStatusInTrailer` per AMEND-10) + `TestRejectLocalReply_ByteShape` (per AMEND-7 + PD-2.503 — 503 + empty body + no added headers via a stub `DecoderFilterCallbacks` recording `SendLocalReply` args). ~150-250 LoC. |
| `internal/filter/http/admission_control/fuzz_test.go` | NEW | 32nd fuzzer `FuzzAdmissionControlConfigParse` per SPEC §6.7 — must-never-panic across `buildCompiledConfig`. Clean at 30s per seed. ~50 LoC. |
| `internal/filter/http/admission_control/testdata/fuzz/FuzzAdmissionControlConfigParse/` | NEW | Corpus seeds — ~30 seeds covering each PARSE-REJECT arm + the valid full-config (both success-criteria arms + all knobs) + empty config + oneof-absent + malformed-range + >16-grpc-codes variants. |
| `cmd/envoy-go/main.go` | MODIFY | +1 LoC + +1 import. Add import `"github.com/esalaine/envoy-go/internal/filter/http/admission_control"` (alphabetical between `adaptive_concurrency` [line 29] and `bandwidthlimit` [line 30]). Add `httpReg.Register(admission_control.TypeURL, admission_control.New)` between `adaptive_concurrency` [line 128] and `bandwidthlimit` [line 129] per the phase-09..22 alphabetical convention. **NO `RegisterPerRouteValidator` call** — REUSE-by-absence per SPEC §5.4. **18 HTTP filters wired post-phase-23.** |
| `test/differential/fixture/fixture.go` | MODIFY | +1 enum value `HTTPAdmissionControl BackendKind = 23` (after `HTTPLua = 22`). ~+15 LoC including the doc-comment per the existing `BackendKind` comment style (reuses the shared always-200 / echobackend cluster — note in the comment which backend the 0030 scenarios use). |
| `test/differential/runner_test.go` | MODIFY | +blank import + switch-case for `HTTPAdmissionControl`. ~+15 LoC. |
| `test/fixtures/0030-http-admission-control/<scenario>/{envoy.yaml,envoy-go.yaml,expectations.yaml,README.md}` + `inputs/driver.go` + `README.md` | NEW DIRECTORY | 4 scenarios — `parse_ok` (a) REFERENCE-LESS + `all_admit_healthy` (b) CROSS-SIDE byte-exact + `stat_surface` (c) REFERENCE-LESS + `pass_through_disabled` (d) REFERENCE-LESS — per SPEC §7.1. Single-listener topology per SPEC §7.3 (admission_control alphabetical position + router terminator + always-200 backend for (a)-(c); `enabled.default_value=false` config for (d)). Driver returns `BackendKind() = HTTPAdmissionControl`. ~200-350 LoC. |
| `test/fixtures/0031-http-admission-control-boot-reject/{envoy.yaml,envoy-go.yaml,expectations.yaml,README.md}` + `inputs/driver.go` | NEW DIRECTORY | Boot-reject fixture per SPEC §7.2 — `sr_threshold.default_value < 1.0%` shared reject. Driver implements the `BootRejectFixture` interface (`BootRejectScript()` + `ExpectedBootErrorSubstring()`) per the 0029 precedent (`test/differential/harness.go:340`); `ExpectedBootErrorSubstring()` returns the common substring `"cannot be less than 1.0%"` (present in BOTH upstream stderr `"Success rate threshold cannot be less than 1.0%."` and the envoy-go-mirror wording per PD-2.boot). Driver returns `BackendKind() = HTTPAdmissionControl` (the runner picks the boot-reject branch because the driver implements `BootRejectFixture`). ~50-100 LoC. |
| `docs/envoy-go/DECISIONS.md` | MODIFY | 2 ADR §Decision + §Consequences bodies anchored at IMPL Tasks (ADR-0195 at Task 2 + ADR-0194 at Task 4) — EXTEND the SPEC-commit §Context drafts per ADR-0044. NO new ADR numbers consumed at IMPL under the D-hypothesis (next-free stays ADR-0196). |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY | §13 4-edit bundle per SPEC §13 (NEW `### envoy.filters.http.admission_control` subsection + 1 envoy-go-strict departure record [14 → 15] + stat-table 107→110 extension + per-route canonical-patterns caption update). Lands at Task 11 (atomic landing per ADR-0052). |
| `docs/envoy-go/ROADMAP.md` | MODIFY | row 23 per-cell IMPL-done annotation + status flips `in-progress → done` at Task 12. |
| `docs/envoy-go/STATE.md` | MODIFY | rewrite-in-place at Task 12 (advance to post-phase-23 state per BOOTSTRAP §4.1 invariant 1). |
| `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` | NEW | append-only task log; ~500-800 LoC across 12 task entries. |
| `docs/envoy-go/phases/23-http-filter-admission-control/REVIEW.md` | NEW | Task 12 reviewer artifact per `superpowers:requesting-code-review`. ~300 LoC. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + PLAN-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's sub-pin-level refinements before implementation; this PLAN settles those (delegated to the IMPL Task that closes each per SPEC §12) plus the planner-time-emerged framework-grounding decisions surfaced by reading the real filter framework. The resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced here so the implementer at each Task can act without re-deriving them.

**PD-1 — `New` factory signature (NEW; surfaced at PLAN-time framework grounding).** The SPEC §6 illustrative `New(message proto.Message)` is REPLACED by the real `HTTPFilterFactory` shape per `internal/filter/http/types.go:245`: `func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)`. `ctx.Stats` is the `*stats.Registry`; `ctx.StatPrefix` is the HCM `http.<stat_prefix>` root. Matches the `adaptive_concurrency.go:108` precedent verbatim. Settles at Task 5 (struct + signature) + Task 8 (body). NO new ADR — within ADR-0194's envelope.

**PD-2 — PARSE-REJECT + reject-wire byte-stable strings LOCKED per SPEC §5.1 + §5.2 + AMEND-7 + ADR-0080 (NEW; settles SPEC §12 A1 + A2 + A3).** The implementer's authoritative list at Task 2 (PARSE-REJECT) + Task 5 (reject wire) + Task 9 (boot-reject substring):
   - **§5.1 RATIFIED-from-config arms (4):**
     - `"admission_control: evaluation_criteria is required"` (oneof absent)
     - `"admission_control: sr_threshold cannot be less than 1.0%"` (`sr_threshold.default_value < 1.0%`)
     - `"admission_control: http_success_status range invalid (must be within [100,600) and start<=end)"`
     - `"admission_control: grpc_success_status accepts at most 16 codes"`
   - **§5.2 envoy-go-strict `runtime_key` arms (5):**
     - `"admission_control: enabled.runtime_key is not yet supported; use enabled.default_value"`
     - `"admission_control: aggression.runtime_key is not yet supported; use aggression.default_value"`
     - `"admission_control: sr_threshold.runtime_key is not yet supported; use sr_threshold.default_value"`
     - `"admission_control: max_rejection_probability.runtime_key is not yet supported; use max_rejection_probability.default_value"`
     - `"admission_control: rps_threshold.runtime_key is not yet supported; use rps_threshold.default_value"`
   - **PD-2.503 — reject wire shape:** the framework `SendLocalReply(status int, body string, headers OrderedHeaders)` is 3-arg (per `internal/filter/http/callbacks.go:34`) — there is NO rc_details or grpc_status parameter. The AMEND-7 `response_code_details = "denied_by_admission_control"` is therefore **NOT surfaceable through the framework API** → documented ABSENT-by-API (subject-only, NOT byte-pinned), mirroring phase-21 adaptive_concurrency's `response_code_details` treatment. The byte-pin asserted is **status 503 + empty body `""` + no added headers** (`f.cb.SendLocalReply(503, "", nil)`). Settles at Task 5 + Task 6 (`TestRejectLocalReply_ByteShape`).
   - **PD-2.boot — boot-reject common substring:** `ExpectedBootErrorSubstring() = "cannot be less than 1.0%"` — present in BOTH upstream stderr (`"Success rate threshold cannot be less than 1.0%."`) and the envoy-go-mirror wording (`"admission_control: sr_threshold cannot be less than 1.0%"`). Settles at Task 9.

**PD-3 — health-check gate arm NOT-MODELED at MVP (NEW; SPEC-vs-reality gap surfaced at PLAN-time; per BOOTSTRAP "Ambiguity → ADR-or-proceed").** The SPEC §4.1 + §6.4 decode-gate pseudocode shows `decoder_callbacks_->streamInfo().healthCheck()` (the AMEND-4 health-check short-circuit). **envoy-go's `DecoderFilterCallbacks` exposes NO `StreamInfo()` / `HealthCheck()` / `IsHealthCheck()` accessor** (verified at PLAN-time against `internal/filter/http/callbacks.go`), and the project wires NO upstream `health_check` HTTP filter / stream-info health-check marker. Adding such an accessor would be a NEW framework primitive — VIOLATING the SPEC's ZERO-new-framework-primitive constraint. **Disposition:** the `healthCheck()` arm is **NOT-MODELED** at phase-23 MVP; `DecodeHeaders` implements only the `!f.cc.enabled` pass-through arm. Consequently AMEND-11's "health-check requests not recorded" is **vacuous** at MVP (no health-check requests exist in envoy-go's model). This is a documented deferral (added to the deferred-items register + a BEHAVIOR_CONTRACT note at Task 11). It does NOT consume ADR-0196 (it is a not-modeled deferral, not a §Decision-level rationale — preserving the SPEC §10 D-hypothesis that ADR-0196 stays unconsumed). The implementer confirms at Task 5; if IMPL discovers a stream-info hook that makes the arm cheaply faithful, that is a §Consequences note under ADR-0194, NOT a new primitive.

**PD-4 — both-sides filter shape (NEW; framework grounding).** The filter is returned as `HTTPFilter{Name: "...", Decoder: f, Encoder: f}` where a single `*filter` value implements BOTH `StreamDecoderFilter` (DecodeHeaders/DecodeData/DecodeTrailers/SetDecoderCallbacks/OnDestroy) AND `StreamEncoderFilter` (EncodeHeaders/EncodeData/EncodeTrailers/SetEncoderCallbacks/OnDestroy) per `internal/filter/http/types.go:73-81`. The `*controller` is hoisted to the factory closure level (one per `compiledConfig`/HCM instance per SPEC §6.2); each per-request `*filter` captures the shared pointer. Mirrors the `bandwidthlimit.go:172` + `compressor.go:264` both-sides precedent. Settles at Task 8.

**PD-5 — encode-side status access + gRPC detection (settles SPEC §12 B6 + AMEND-10).** HTTP status is read via `headers.Get(":status")` (the `:status` pseudo-header is available on the encode-side `http.Header`, per the `compressor.go:785` precedent), parsed to `int`. gRPC-ness is detected via `headers.Get("content-type")` having the `application/grpc` prefix; the gRPC status is read from the `grpc-status` header when present in headers (trailers-only / headers-encoded responses) or from the trailers `http.Header` in `EncodeTrailers` when deferred (`f.expectGRPCStatusInTrailer`). Settles at Task 6 (`TestClassification_GRPC_*`).

**PD-6 — `(1e4·P) > (r%1e4)` knife-edge determinism (settles SPEC §12 B4).** `shouldReject()` mirrors upstream's strict `>` + `accuracy = 10000` integer modulo EXACTLY: `return float64(10000)*math.Max(p, 0.0) > float64(r%10000)`. The boundary is `r%10000 == floor(10000·p)` → admits (strict `>` is false at equality); `r%10000 == floor(10000·p) − 1` → rejects. P=0 ⇒ `0 > (r%10000)` false for every `r` ⇒ never reject. Settles at Task 4 (`TestShouldReject_Boundary_*` with injected `fakeRand`).

**PD-7 — `samplingWindow` deque rollover/expiry determinism (settles SPEC §12 B5).** Per-second bucket granularity; rollover when the newest bucket's `ts` is ≥1s older than `clock.Now()`; stale-purge of buckets older than `samplingWindow` decrementing the running `global` aggregate. `samplingWindow` rounded to whole seconds via integer `ms/1000` (mirrors `config.cc:33-35`). Settles at Task 4 (`TestController_FAKE_TIME_Window_*` with injected `fakeClock`).

**PD-8 — task decomposition: seams + stats folded into Task 3 (NEW; PLAN-time).** `rand.go` + `clock.go` + `stats.go` (+ their test-scope `fakeRand`/`fakeClock`) land together at Task 3 as the small foundational layer the controller depends on — they are trivial, file-disjoint from `compiled_config.go`, and parallelizable with Task 2. No ADR — IMPL-level decomposition choice.

**PD-9 — zero framework regression (settles SPEC §12 C7).** Phase-23 touches no shared `internal/` primitive (counters-only via existing `internal/stats/`). Gate C race tests + the full differential regression run confirm zero regression. Expected outcome: zero regression (no framework signature change).

**PD-10 — fuzzer corpus + 32nd-fuzzer registration (NEW; PLAN-time).** `FuzzAdmissionControlConfigParse` fuzzes `buildCompiledConfig`. ~30 corpus seeds covering: a valid full config (both success-criteria arms + all knobs); each of the 9 PARSE-REJECT arms; empty config; oneof-absent; malformed http range; >16 grpc codes. Must-never-panic; clean at 30s per seed. Settles at Task 7.

---

## ADRs introduced / landed by this plan

| ADR | Disposition | §Context anchored | §Decision + §Consequences body lands | Lands-in-Task |
|---|---|---|---|---|
| **ADR-0194** | NEW (algorithm + package shape + inline Rand/Clock seams + deque-window + integer-modulo decision + classification + 3-counter stat surface + deterministic-regime differential strategy) | SPEC commit `a64ee71` (§Context draft present per ADR-0044) | this PLAN's IMPL | **Task 4** (controller + filter materialization) |
| **ADR-0195** | NEW (RTDS `runtime_key` deferral PARSE-REJECT — 5 arms; `enabled`-absent⇒ENABLED per AMEND-4; the SINGLE envoy-go-strict departure) | SPEC commit `a64ee71` (§Context draft present per ADR-0044) | this PLAN's IMPL | **Task 2** (compiled_config + PARSE-REJECT roster) |
| **ADR-0196** | HYPOTHESIZED UNCONSUMED at phase-done (D-style HOLD-with-known-risk per SPEC §10 — one-slot escape-valve buffer; a surprise upstream stat name / a 4th counter / a clamp-boundary float edge could force consumption) | n/a | n/a (stays §-free) | — |

**ZERO in-place §Decision AMENDMENTs. ZERO ADR-0125 amendments** (REUSE-by-absence per SPEC §5.4; canonical-per-route roster STAYS 9; FIRST ADR-0125-skip since phase-22's roster amendment).

---

## Task graph (sequential vs parallelizable)

The IMPL session subagent-dispatches per `superpowers:subagent-driven-development` (project memory `feedback_execution_style.md`). Per-task graph:

- **Task 1** (PROGRESS.md preamble + precondition verification) — sequential prerequisite for everything; sets up the append-only log + records PD-1..PD-10.
- **Tasks 2, 3, 7** — **PARALLELIZABLE** (independent surfaces; all depend on Task 1 but not on each other):
  - **Task 2** — `compiled_config.go` + 9-arm PARSE-REJECT roster + ADR-0195 §Decision + §Consequences body.
  - **Task 3** — `rand.go` + `clock.go` + `stats.go` + their test-scope `fakeRand`/`fakeClock` + stat-name byte-exact guards (per PD-8).
  - **Task 7** — `fuzz_test.go` + 32nd fuzzer + corpus seeds (depends only on Task 2's `buildCompiledConfig`; can start once Task 2 lands).
- **Task 4** (`controller.go` + `controller_test.go` + ADR-0194 §Decision + §Consequences body) — depends on Task 2 (`compiledConfig`) + Task 3 (`Clock`/`Rand`/`filterStats`).
- **Task 5** (`admission_control.go` filter struct + TypeURL + assertions + `decode_headers.go` + decode tests) — depends on Task 4 (`controller.shouldReject`/`averageRps`) + Task 2 + Task 3.
- **Task 6** (`encode.go` classification + record discipline + `encode_test.go`) — depends on Task 4 (`controller.classify` + `cc.isHTTPSuccess`/`isGRPCSuccess`) + Task 5 (the `filter` struct + `f.record`/`f.expectGRPCStatusInTrailer` fields).
- **Task 8** (full integration: complete `New` factory + `doc.go` + boot-registration at `cmd/envoy-go/main.go`) — depends on Tasks 2-7; produces a fully-functional `HTTPFilterFactory` from `New()`.
- **Task 9** (differential fixtures `0030` + `0031` + `BackendKind` enum + runner switch-case + drivers) — depends on Task 8.
- **Task 10** (ADR final-state alignment + DECISIONS.md cross-reference audit) — depends on Tasks 2-9 (both ADR bodies anchored).
- **Task 11** (BEHAVIOR_CONTRACT.md 4-edit bundle per SPEC §13 + the PD-3 health-check NOT-MODELED note) — depends on Task 9 (some paragraphs reference the fixture-0030/0031 wire-shape closures).
- **Task 12** (six-gate phase-done verification A/B/C/D/E/F + STATE.md re-advance + ROADMAP row 23 flip + REVIEW.md per `superpowers:requesting-code-review`) — depends on everything.

**Parallel-dispatch opportunity at Tasks 2+3** (then 7 once 2 lands). **Sequential bottleneck at Task 4 → 5 → 6 → 8 → 9 → 10/11 → 12** — the controller materialization + decode + encode + integration + fixtures + the six-gate verification are the critical path.

---

## Execution preconditions

Before Task 1 the implementer cold-starts and verifies. **Worktree spawn discipline:** the IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (project memory `feedback_git_worktrees.md`):

```bash
git worktree add /home/esa/git/envoy-go/.worktrees/phase-23-http-filter-admission-control-impl \
                 -b phase-23-http-filter-admission-control-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-23-http-filter-admission-control-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md squash-merge commit + its SHA-fill follow-up.

The preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-23-http-filter-admission-control-impl`. If only a SPEC/PLAN-stage worktree is present, branch a fresh impl worktree from master HEAD per ADR-0003.
2. **Master tail.** `git log --oneline master | head -6` shows the phase-23-PLAN.md squash commit + its SHA-fill follow-up at the head, with the phase-23-SPEC.md squash commit `a64ee71` + its SHA-fill follow-up `ec68627` immediately before. If not, resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.26.2` or newer; `golangci-lint version` reports `1.64.8` (ADR-0009 pin); `docker version` reports both client + server.
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `195` (ADR-0195 — the highest ADR anchored as of master tip per the phase-23 SPEC commit). Higher → another phase landed concurrently; re-verify next-free numbers.
5. **ADR §Context drafts present.** `grep -cE '^## ADR-0194' docs/envoy-go/DECISIONS.md` returns `1` (ADR-0194 §Context already at SPEC commit per ADR-0044). Same for ADR-0195. `grep -cE '^## ADR-0196' docs/envoy-go/DECISIONS.md` returns `0` (ADR-0196 stays unconsumed at phase-23 IMPL under the D-hypothesis).
6. **NO ADR-0125 amendment.** Phase 23 lands NO ADR-0125 amendment (REUSE-by-absence per §5.4 — FIRST skip since phase-22). If a phase-23-specific cross-reference to ADR-0125 lands at DECISIONS.md during IMPL beyond the existing roster-STAYS-9 note, investigate before proceeding.
7. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/23-http-filter-admission-control/SPEC.md` returns `a64ee71` (or descendant). If different, re-read SPEC.
8. **PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/23-http-filter-admission-control/PLAN.md` returns the PLAN commit's SHA. If earlier than the SPEC, PLAN has been amended — re-read PLAN.
9. **Pristine tree.** `git status --porcelain` returns empty.
10. **Pre-existing suite green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
11. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-9])'` returns every fixture 0000-0029 PASS — the 31 pre-existing fixture directories are the regression baseline (lua fixtures 0026-0029 GREEN in isolation; combined multi-listener runs may hit the documented `freeTCPPort` flake per 22.2 REVIEW §7.4 — not a defect). Phase 23 adds the 23rd `BackendKind` enum value + the 0030 + 0031 fixture directories (31 → 33).
12. **Pre-existing fuzzers run clean at 30s.** The 31 fuzzers from phases 02-22 run clean. Phase 23 adds the 32nd (`FuzzAdmissionControlConfigParse` per Task 7).
13. **Reference Envoy image present.** `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin; unchanged).
14. **Pre-existing `internal/filter/http/admission_control/` directory does NOT exist.** `test ! -d internal/filter/http/admission_control && echo "ok: phase-23-new-surface absent"` returns success.
15. **Proto binding present.** `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/admission_control/v3 AdmissionControl 2>/dev/null | head -1` returns the `AdmissionControl` type (the v1.32.4 binding for the filter config). If absent, investigate the go-control-plane pin before proceeding.

If all preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044, ADR-0194 + ADR-0195 §Context drafts are at the SPEC commit `a64ee71`; ADR-0196 is CONDITIONAL (D-hypothesis: it does NOT fire at phase-23 IMPL). The PROGRESS preamble ANTICIPATES the 2 NEW ADR landings (each with its Lands-in-Task anchor reproduced from the ADR table above) and records the 10 planner-time decisions PD-1..PD-10.

**Precondition:** worktree exists at `phase-23-http-filter-admission-control-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all preconditions report green.
**Artifact:** `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (new file).
**Acceptance:** all preconditions report green; PROGRESS.md preamble committed.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` above and confirm the expected output.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create the file with: (a) preamble summarizing the precondition verification (verbatim command outputs captured); (b) the 2-NEW-ADR table from `## ADRs introduced / landed by this plan` reproduced verbatim; (c) the 10 planner-time decisions PD-1..PD-10 reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) a Task 1 entry slot for the commit-SHA fill-in.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 1: PROGRESS.md preamble + precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
# expect: a 40-char SHA (Task 1 commit)
```

---

## Task 2: NEW `internal/filter/http/admission_control/compiled_config.go` + 9-arm PARSE-REJECT roster + ADR-0195

**Files:**
- Create: `internal/filter/http/admission_control/compiled_config.go` (~220-300 LoC)
- Create: `internal/filter/http/admission_control/compiled_config_test.go` (~250-350 LoC)
- Modify: `docs/envoy-go/DECISIONS.md` (~+100 LoC: ADR-0195 §Decision + §Consequences body — EXTENDS the SPEC-commit §Context draft per ADR-0044)
- Append: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (Task 2 entry)

This task lands the `compiledConfig` struct + the `buildCompiledConfig` parser + the full 9-arm PARSE-REJECT roster per PD-2 byte-stable wording (4 RATIFIED-from-config arms per SPEC §5.1 + 5 envoy-go-strict `runtime_key` arms per SPEC §5.2), the AMEND-4 `enabled`-absent⇒ENABLED default, the http-range + grpc-code precompilation, and the `sampling_window` ms/1000 rounding. **Parallelizable with Task 3** (disjoint files; depends only on Task 1).

**Precondition:** Task 1 complete.
**Artifact:** `compiled_config.go` with full PARSE-REJECT roster; ADR-0195 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./internal/filter/http/admission_control/...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/admission_control/...` clean; `go test -count=1 ./internal/filter/http/admission_control/... -run 'TestBuildCompiledConfig|TestEnabledMatrix|TestParseRejectConstants'` clean; `grep -cE '^## ADR-0195' docs/envoy-go/DECISIONS.md` returns `1` AND §Decision body non-empty.

- [ ] **Step 1: Write failing tests** in `compiled_config_test.go` per SPEC §14.1 #8 + #9 + PD-2. Table-driven `TestBuildCompiledConfig_PARSE_REJECT_*`: each row `{name string, configMutator func(*acv3.AdmissionControl), wantErr string}` — ~15-20 rows covering the 4 §5.1 arms + the 5 §5.2 `runtime_key` arms + edge variants. PLUS `TestEnabledMatrix_*` (per SPEC §5.3 — the 4 cases) + default-applied tests (per SPEC §6.1 + AMEND-4) + `TestParseRejectConstants_ByteStable` (byte-exact wording per ADR-0080 + PD-2).

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/filter/http/admission_control/... 2>&1 | head -20
# Expect: FAIL with "no such package" or build error
```

- [ ] **Step 3: Author `compiled_config.go`** per the File-structure table row + SPEC §6.1 + ADR-0195 §Context. Includes: the `compiledConfig` struct (verbatim per SPEC §6.1); `buildCompiledConfig(typedConfig *anypb.Any) (*compiledConfig, error)` with each PARSE-REJECT arm per PD-2 byte-stable wording; default application per SPEC §6.1 + AMEND-4 (`enabled` true when the message is absent — the AMEND-4 inversion vs phase-21; `aggression` floored to 1.0; `srThreshold` min(pct,100)/100 default 0.95; `rpsThreshold` 0; `maxRejectionProbability` pct/100 default 0.80; `samplingWindow` 30s via `ms/1000`); `httpSuccessRanges` precompiled from `http_success_status` (default `{[100,500)}`); `grpcSuccessCodes` precompiled from `grpc_success_status` (default the 11-code well-known set per AMEND-5: {0,1,2,3,5,6,7,9,11,12,16}); the `isHTTPSuccess(code int) bool` + `isGRPCSuccess(status uint32) bool` predicates (consumed at encode by Task 6).

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -count=1 ./internal/filter/http/admission_control/... -run 'TestBuildCompiledConfig|TestEnabledMatrix|TestParseRejectConstants'
# Expect: PASS
```

- [ ] **Step 5: Author ADR-0195 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the §Context draft at `a64ee71`. §Decision: the 5 `runtime_key`-non-empty PARSE-REJECT arms with byte-stable wording per SPEC §5.2; the `enabled` honored-matrix default-application per SPEC §5.3 (absent⇒ENABLED via the `PROTOBUF_GET_WRAPPED_OR_DEFAULT(...,true)` mechanism — the AMEND-4 inversion vs phase-21 ADR-0187); the `default_value`-honoring of the numeric knobs. §Consequences: operators configure static thresholds via the wrapper `default_value`s; runtime keying is a forward-pointer to the Runtime/RTDS family phase; the SINGLE envoy-go-strict departure record (count 14 → 15) lands at BEHAVIOR_CONTRACT at Task 11; the absent⇒ENABLED upstream-parity matrix.

- [ ] **Step 6: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 7: Append PROGRESS.md Task 2 entry** — build/test output + `git log -1 --format=%H`.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/admission_control/compiled_config.go \
        internal/filter/http/admission_control/compiled_config_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 2: compiled_config.go + PARSE-REJECT roster + ADR-0195

Lands compiledConfig + buildCompiledConfig with the 9-arm PARSE-REJECT
roster per PD-2 byte-stable wording (4 RATIFIED-from-config arms per
SPEC §5.1 + 5 envoy-go-strict runtime_key arms per SPEC §5.2). enabled
defaults ENABLED when the message is absent (AMEND-4 inversion vs
phase-21). ADR-0195 §Decision + §Consequences anchored (EXTENDS
SPEC-commit §Context draft per ADR-0044)."
```

---

## Task 3: NEW `rand.go` + `clock.go` + `stats.go` seams + test-scope fakes

**Files:**
- Create: `internal/filter/http/admission_control/rand.go` (~25-50 LoC)
- Create: `internal/filter/http/admission_control/clock.go` (~25-50 LoC)
- Create: `internal/filter/http/admission_control/stats.go` (~30-50 LoC)
- Create: `internal/filter/http/admission_control/rand_test.go` (~40-80 LoC; `fakeRand` test-scope)
- Create: `internal/filter/http/admission_control/clock_test.go` (~60-120 LoC; `fakeClock` test-scope + `Advance`)
- Create: `internal/filter/http/admission_control/stats_test.go` (~30-60 LoC; `TestStatNames_Equal_*` byte-exact guards — Task 3 OWNS the stat-name assertion to avoid a Task-5 duplicate)
- Append: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (Task 3 entry)

This task lands the small foundational layer the controller depends on per PD-8: the inline `Rand` (`Uint64()`) + `Clock` (`Now()`) seams + their production wirings + the 3-counter `filterStats` + the test-scope `fakeRand`/`fakeClock`. **Parallelizable with Task 2** (disjoint files; depends only on Task 1).

**Precondition:** Task 1 complete.
**Artifact:** `rand.go` + `clock.go` + `stats.go` + the test-scope fakes; stat-name byte-exact guards.
**Acceptance:** `go build ./internal/filter/http/admission_control/...` clean; `go vet ./...` + `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/admission_control/... -run 'TestFakeClock|TestFakeRand|TestStatNames'` clean.

- [ ] **Step 1: Author `rand.go`** per SPEC §3.1 + AMEND-2 — `Rand` interface (`Uint64() uint64`) + `defaultRand struct{}` (`func (defaultRand) Uint64() uint64 { return rand.Uint64() }` importing `math/rand/v2`).

- [ ] **Step 2: Author `clock.go`** per SPEC §3.2 — `Clock` interface (`Now() time.Time`) + `defaultClock struct{}` (`func (defaultClock) Now() time.Time { return time.Now() }`).

- [ ] **Step 3: Author `stats.go`** per SPEC §6.6 + AMEND-3 — package-level `const` stat names (`rq_rejected`/`rq_success`/`rq_failure`); `filterStats` struct (3 `*stats.Counter` fields); `newFilterStats(reg *stats.Registry, hcmPrefix string) *filterStats` constructing each via `reg.NewCounter(hcmPrefix + ".admission_control." + name)`. NO gauges.

- [ ] **Step 4: Write tests** — `rand_test.go` (`fakeRand struct{ v uint64 }` + `Uint64()` returning `v`; a `defaultRand` sanity test) + `clock_test.go` (`fakeClock struct{ now time.Time }` + `Now()` + `Advance(d time.Duration)`; determinism tests) + `stats_test.go` (`TestStatNames_Equal_*` asserting the 3 stat-name constants byte-exact against the wire-expected names). Task 3 OWNS the stat-name byte-exact assertion — Task 5's `admission_control_test.go` does NOT re-assert stat names (avoids a duplicate).

- [ ] **Step 5: Run tests to verify they pass.**

```bash
go test -count=1 ./internal/filter/http/admission_control/... -run 'TestFakeClock|TestFakeRand|TestStatNames'
# Expect: PASS
```

- [ ] **Step 6: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 7: Append PROGRESS.md Task 3 entry.**

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/admission_control/rand.go \
        internal/filter/http/admission_control/clock.go \
        internal/filter/http/admission_control/stats.go \
        internal/filter/http/admission_control/rand_test.go \
        internal/filter/http/admission_control/clock_test.go \
        internal/filter/http/admission_control/stats_test.go \
        docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 3: rand.go + clock.go + stats.go seams + test fakes

Inline Rand (Uint64() per AMEND-2, NOT Float64()) + Clock (Now()) seams
+ 3-counter filterStats (no gauges per AMEND-3). fakeRand/fakeClock in
test scope only. ZERO new framework primitives (LEAN posture per SPEC §3)."
```

---

## Task 4: NEW `controller.go` + sliding-window controller + formula + integer-modulo decision + ADR-0194

**Files:**
- Create: `internal/filter/http/admission_control/controller.go` (~250-350 LoC)
- Create: `internal/filter/http/admission_control/controller_test.go` (~300-450 LoC; Layer A FAKE-TIME + boundary + formula tests)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150 LoC: ADR-0194 §Decision + §Consequences body — EXTENDS the SPEC-commit §Context draft per ADR-0044)
- Append: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (Task 4 entry)

This task lands the per-HCM-instance sliding-window success-rate controller + the empirically-pinned formula + the integer-modulo reject decision + the algorithmic-fidelity test layer. Depends on Task 2 (`compiledConfig`) + Task 3 (`Clock`/`Rand`/`filterStats`).

**Precondition:** Task 2 + Task 3 complete.
**Artifact:** `controller.go` + algorithmic-fidelity tests; ADR-0194 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./internal/filter/http/admission_control/...` clean; `go vet ./...` + `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/admission_control/...` clean (Layer A families pass); `go test -race -count=1 ./internal/filter/http/admission_control/...` clean (`TestController_Concurrent_*` pass); `grep -cE '^## ADR-0194' docs/envoy-go/DECISIONS.md` returns `1` AND §Decision body non-empty.

- [ ] **Step 1: Write failing tests** in `controller_test.go` per SPEC §14.1 Layer A:
  - `TestShouldReject_Boundary_*` — per AMEND-2 + PD-6: primed window `{n,s}` + injected `fakeRand`; verify `r%1e4 == floor(1e4·P)` admits (strict `>` false at equality); `floor(1e4·P)−1` rejects; P=0 ⇒ never reject for any `r`.
  - `TestProbabilityFormula_*` — per AMEND-1: vector tests over `{n, s, srThreshold, aggression, maxRejectionProbability}`; verify exponent skipped at aggression=1.0; sr_threshold-divides-successes; aggression floor (configured 0.5 → 1.0); max-rej clamp; max(0,·) floor.
  - `TestController_FAKE_TIME_Window_*` — per AMEND-6 + PD-7: per-second bucket rollover + stale-purge over `samplingWindow` via `fakeClock.Advance`; `requestCounts()` aggregate; `averageRps()` (0 empty; n/secs else).
  - `TestRpsSuppression_*` — per §4.1: `averageRps() < rpsThreshold` returns admit-without-reject (handled at decode; here verify `averageRps()` correctness feeding the gate).
  - `TestRecordDiscipline_*` — per AMEND-11: `classify` records; the disabled/rejected paths (driven via the filter at Task 5/6) are NOT recorded — at controller scope verify `classify(true/false)` increments the right counter + the window.
  - `TestController_Concurrent_*` — race tests: concurrent `recordRequest`/`classify`/`shouldReject` under `mu`; no data race; no deadlock.

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `controller.go`** per the File-structure table row + SPEC §4 + §6.2 + ADR-0194 §Context. Includes: the `controller` + `bucket` structs per SPEC §6.2; `newController(cfg, stats, clock, rand)`; `recordRequest(success bool)` per §4.2 (purge/rollover/increment under `mu`); `requestCounts() (n, s uint64)` per §4.2; `averageRps() uint32` per §4.2; `shouldReject() bool` per §4.1 + AMEND-1 + AMEND-2 + PD-6 (formula with `math.Pow` exponent skipped at aggression=1.0; `math.Min` clamp; `math.Max(p,0)` floor; integer-modulo decision `float64(10000)*math.Max(p,0.0) > float64(r%10000)`); `classify(success bool)` (calls `recordRequest` + increments `stats.rqSuccess`/`stats.rqFailure`).

- [ ] **Step 4: Run tests to verify they pass.**

```bash
go test -count=1 ./internal/filter/http/admission_control/...
go test -race -count=1 ./internal/filter/http/admission_control/...
# Expect: PASS — Layer A families + race tests clean
```

- [ ] **Step 5: Author ADR-0194 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the §Context draft at `a64ee71`. §Decision: the package shape + `TypeURL` + `New` factory (PD-1 signature); the `controller` state machine + the formula `max(0, min(maxRej, ((n−s/sr)/(n+1))^(1/aggr)))` with line-exact lemmata (`admission_control.cc:161-179`); the integer-modulo decision `(1e4·P) > (r%1e4)` (`:175-178`); the inline `Rand` (`Uint64()`) + `Clock` (`Now()`) seam signatures (NOT framework primitives); the success/error classification (HTTP `<500` / gRPC 11-code / gRPC-trailers per AMEND-5 + AMEND-10); the deque-of-per-second-buckets window mechanics (`thread_local_controller`); the both-sides decode-gate/encode-classify discipline; the PD-3 health-check-arm-NOT-MODELED note. §Consequences: ZERO new framework primitive; the Clock-shaped EXTRACT-NOW forward-pointer (consumer count 2; shapes differ; deferred per SPEC §8 item 8); the 3-counter byte-exact stat surface (107 → 110, no gauges); the deterministic-regime differential coverage (all-admit P=0 cross-side; forced-reject subject-only per AMEND-2); the D-style hypothesis disposition — ADR-0196 UNCONSUMED at phase-done, HOLD-with-known-risk.

- [ ] **Step 6: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 7: Append PROGRESS.md Task 4 entry** — build/test output + `git log -1 --format=%H` + the SPEC §12 B4/B5 pin closures (knife-edge + window determinism RATIFIED).

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/admission_control/controller.go \
        internal/filter/http/admission_control/controller_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 4: controller.go + formula + integer-modulo decision + ADR-0194

Sliding-window deque controller (per-second buckets per AMEND-6) + the
empirically-pinned formula (AMEND-1: aggression EXPONENT floored to 1.0;
sr_threshold DIVIDES successes) + the integer-modulo reject decision
(AMEND-2: (1e4*P) > (r%1e4); Rand seam is Uint64()). Layer A FAKE-TIME +
boundary + formula tests pass; race tests clean. ADR-0194 §Decision +
§Consequences anchored (EXTENDS SPEC-commit §Context draft per ADR-0044)."
```

---

## Task 5: NEW `admission_control.go` filter struct + `decode_headers.go` gate + reject wire shape

**Files:**
- Create: `internal/filter/http/admission_control/admission_control.go` (~80-120 LoC; TypeURL + filter struct + assertions; New factory body completed at Task 8)
- Create: `internal/filter/http/admission_control/decode_headers.go` (~50-80 LoC)
- Create: `internal/filter/http/admission_control/admission_control_test.go` (~80-120 LoC; TypeURL + decode-gate tests; stat names already guarded in Task 3's `stats_test.go`)
- Append: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (Task 5 entry)

This task lands the per-stream `filter` struct + the TypeURL constant + the compile-time interface assertions + the `DecodeHeaders` gate (enable / RPS-suppression / reject) + the reject wire shape per AMEND-7 + PD-2.503 + PD-3 (health-check arm NOT-MODELED). Depends on Task 4 (`controller`) + Task 2 + Task 3.

**Precondition:** Task 4 complete.
**Artifact:** `admission_control.go` (struct + TypeURL) + `decode_headers.go`.
**Acceptance:** `go build ./...` clean; `go vet ./...` + `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/admission_control/... -run 'TestDecodeHeaders|TestTypeURL|TestStatNames'` clean.

- [ ] **Step 1: Write failing tests** in `admission_control_test.go` per SPEC §14.1 (stat-name byte-exactness is already asserted in Task 3's `stats_test.go` — do NOT duplicate here): `TestTypeURL_*` (constant byte-exact); `TestDecodeHeaders_Disabled_PassThrough` (cc.enabled=false ⇒ Continue, record cleared); `TestDecodeHeaders_RpsSuppression` (averageRps < rpsThreshold ⇒ Continue, record stays true); `TestDecodeHeaders_Reject_*` (shouldReject via injected fakeRand ⇒ StopIteration + rqRejected.Inc + SendLocalReply(503,"",nil)). Use a stub `DecoderFilterCallbacks` recording `SendLocalReply` args.

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `admission_control.go`** — `const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.admission_control.v3.AdmissionControl"`; the per-stream `filter` struct (fields per the File-structure table + PD-4); compile-time assertions `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` + `var _ envoyhttp.StreamEncoderFilter = (*filter)(nil)`; a `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` stub (full body at Task 8 — at Task 5 it may already wire `buildCompiledConfig` + `newController` + return the factory closure, since Tasks 2-4 are landed; the encode-side methods land at Task 6).

- [ ] **Step 4: Author `decode_headers.go`** per SPEC §6.4 + PD-2.503 + PD-3 — `DecodeHeaders` with the 3-gate order (enable / RPS-suppression / reject); the `healthCheck()` arm OMITTED per PD-3 (NOT-MODELED — no stream-info accessor); `DecodeData`/`DecodeTrailers` Continue pass-throughs; `SetDecoderCallbacks`; `OnDestroy` no-op.

- [ ] **Step 5: Run tests to verify they pass.**

```bash
go test -count=1 ./internal/filter/http/admission_control/... -run 'TestDecodeHeaders|TestTypeURL|TestStatNames'
# Expect: PASS
```

- [ ] **Step 6: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 7: Append PROGRESS.md Task 5 entry** — note the PD-3 health-check NOT-MODELED disposition (forward-pointer to the Task 11 BEHAVIOR_CONTRACT note).

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/admission_control/admission_control.go \
        internal/filter/http/admission_control/decode_headers.go \
        internal/filter/http/admission_control/admission_control_test.go \
        docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 5: filter struct + TypeURL + decode_headers.go gate + reject

DecodeHeaders 3-gate order (enable / RPS-suppression / reject). Reject
wire shape: SendLocalReply(503, \"\", nil) per AMEND-7 + PD-2.503 (rc-details
not surfaceable via 3-arg API; status+empty-body+no-headers byte-pinned).
Health-check arm NOT-MODELED per PD-3 (no stream-info accessor)."
```

---

## Task 6: NEW `encode.go` classification + record discipline

**Files:**
- Create: `internal/filter/http/admission_control/encode.go` (~80-120 LoC)
- Create: `internal/filter/http/admission_control/encode_test.go` (~150-250 LoC)
- Append: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (Task 6 entry)

This task lands the encode-side classification (HTTP `<500` / gRPC 11-code; gRPC-status-in-headers vs in-trailers per AMEND-10) + the record discipline (per AMEND-11) + the reject byte-shape assertion. Depends on Task 4 (`controller.classify` + `cc.isHTTPSuccess`/`isGRPCSuccess`) + Task 5 (`filter` struct + `f.record`/`f.expectGRPCStatusInTrailer`).

**Precondition:** Task 4 + Task 5 complete.
**Artifact:** `encode.go` with classification + record.
**Acceptance:** `go build ./...` clean; `go vet ./...` + `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/admission_control/...` clean (encode classification families + `TestRejectLocalReply_ByteShape` pass).

- [ ] **Step 1: Write failing tests** in `encode_test.go` per SPEC §14.1 #5 + #7 + AMEND-10: `TestClassification_HTTP_*` (default `<500` success + configured `[100,600)` ranges; `:status` parsed via `headers.Get(":status")`); `TestClassification_GRPC_Headers_*` (gRPC-status in response headers — default 11-code set); `TestClassification_GRPC_Trailers_*` (status deferred to trailers — `expectGRPCStatusInTrailer` set at EncodeHeaders, classified at EncodeTrailers); `TestRecordDiscipline_NotRecordedWhenRejected` (f.record=false ⇒ no classify); `TestRejectLocalReply_ByteShape` (the decode-path reject emits 503 + empty body + no headers — assert via the stub callbacks; per AMEND-7 + PD-2.503).

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `encode.go`** per SPEC §6.5 + §4.4 + AMEND-10 + AMEND-11 + PD-5 — `EncodeHeaders` (guard `f.record`; detect gRPC via content-type; classify HTTP via `:status` or gRPC via `grpc-status` header; defer to trailers when needed); `EncodeData` pass-through; `EncodeTrailers` (handle the deferred gRPC-trailers case); `SetEncoderCallbacks`; `OnDestroy` no-op (shares the `filter`'s OnDestroy with the decode side).

- [ ] **Step 4: Run tests to verify they pass.**

- [ ] **Step 5: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 6: Append PROGRESS.md Task 6 entry** — note the SPEC §12 B6 (gRPC-trailers) pin closure.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/admission_control/encode.go \
        internal/filter/http/admission_control/encode_test.go \
        docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 6: encode.go classification + record discipline

EncodeHeaders/EncodeTrailers classify per success_criteria (HTTP <500 /
gRPC 11-code; gRPC-status-in-headers-or-trailers per AMEND-10) and record
EXCEPT rejected/disabled (AMEND-11). :status read via headers.Get per PD-5."
```

---

## Task 7: NEW `fuzz_test.go` + 32nd fuzzer + corpus seeds

**Files:**
- Create: `internal/filter/http/admission_control/fuzz_test.go` (~50 LoC)
- Create: `internal/filter/http/admission_control/testdata/fuzz/FuzzAdmissionControlConfigParse/` (~30 corpus seeds)
- Append: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (Task 7 entry)

This task lands the 32nd project-wide fuzzer per SPEC §6.7 + PD-10. **Parallelizable with Tasks 4-6** (depends only on Task 2's `buildCompiledConfig`).

**Precondition:** Task 2 complete.
**Artifact:** `fuzz_test.go` + corpus seeds.
**Acceptance:** `go test -count=1 ./internal/filter/http/admission_control/... -run 'FuzzAdmissionControlConfigParse'` clean (seed-corpus run); `go test -fuzz=FuzzAdmissionControlConfigParse -fuzztime=30s ./internal/filter/http/admission_control/` clean (no panics).

- [ ] **Step 1: Author `fuzz_test.go`** — `FuzzAdmissionControlConfigParse(f *testing.F)` seeding the corpus + a fuzz body that marshals the bytes into an `*anypb.Any` and calls `buildCompiledConfig`, asserting must-never-panic.

- [ ] **Step 2: Author corpus seeds** per PD-10 — ~30 seeds: a valid full config (both success-criteria arms + all knobs); each of the 9 PARSE-REJECT arms; empty config; oneof-absent; malformed http range; >16 grpc codes.

- [ ] **Step 3: Run the seed-corpus + 30s fuzz to verify clean.**

```bash
go test -count=1 ./internal/filter/http/admission_control/... -run 'FuzzAdmissionControlConfigParse'
go test -fuzz=FuzzAdmissionControlConfigParse -fuzztime=30s ./internal/filter/http/admission_control/
# Expect: PASS — no panics
```

- [ ] **Step 4: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 5: Append PROGRESS.md Task 7 entry** (fuzzer count 31 → 32).

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/admission_control/fuzz_test.go \
        internal/filter/http/admission_control/testdata/ \
        docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 7: FuzzAdmissionControlConfigParse (32nd fuzzer) + corpus seeds"
```

---

## Task 8: Full filter integration in `admission_control.go` + `doc.go` + boot-registration

**Files:**
- Modify: `internal/filter/http/admission_control/admission_control.go` (complete the `New` factory body — controller hoist + both-sides HTTPFilter)
- Create: `internal/filter/http/admission_control/doc.go` (~20 LoC)
- Modify: `cmd/envoy-go/main.go` (+1 import + +1 Register line)
- Append: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (Task 8 entry)

This task completes the `New` factory (hoist the shared `*controller`; return the `FilterInstanceFactory` closure producing `HTTPFilter{Decoder: f, Encoder: f}` per PD-4), the package doc, and the boot-registration. Depends on Tasks 2-7.

**Precondition:** Tasks 2-7 complete.
**Artifact:** fully-functional `New()`; boot-registration wired.
**Acceptance:** `go build ./...` clean; `go vet ./...` + `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/admission_control/...` clean; `grep -c 'admission_control.TypeURL' cmd/envoy-go/main.go` returns `1`; the binary boots with the filter registered.

- [ ] **Step 1: Complete the `New` factory body in `admission_control.go`** — call `buildCompiledConfig(tc)`; construct `*filterStats` via `newFilterStats(ctx.Stats, ctx.StatPrefix)`; construct the shared `*controller` via `newController(cc, stats, defaultClock{}, defaultRand{})`; return a `FilterInstanceFactory` closure that allocates a fresh `*filter{cc, controller, record: true}` and returns `HTTPFilter{Name: "envoy.filters.http.admission_control", Decoder: f, Encoder: f}`.

- [ ] **Step 2: Author `doc.go`** per SPEC §6.8 (package purpose + TypeURL + controller semantics + both-sides discipline + 3-counter surface + ADR-0194/0195 cross-refs).

- [ ] **Step 3: Wire boot-registration in `cmd/envoy-go/main.go`** — add the import (alphabetical between `adaptive_concurrency` and `bandwidthlimit`) + `httpReg.Register(admission_control.TypeURL, admission_control.New)` between the `adaptive_concurrency` and `bandwidthlimit` Register lines. NO `RegisterPerRouteValidator` call.

- [ ] **Step 4: Verify build + boot.**

```bash
go build ./... && go test -count=1 ./internal/filter/http/admission_control/...
go vet ./... && golangci-lint run
# Expect: clean; 18 HTTP filters wired
```

- [ ] **Step 5: Append PROGRESS.md Task 8 entry** (18 HTTP filters wired).

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/admission_control/admission_control.go \
        internal/filter/http/admission_control/doc.go \
        cmd/envoy-go/main.go \
        docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 8: full filter integration + doc.go + boot-registration

New factory hoists the shared controller + returns both-sides
HTTPFilter{Decoder: f, Encoder: f} per PD-4. Registered alphabetical
between adaptive_concurrency and bandwidthlimit (18 HTTP filters)."
```

---

## Task 9: Differential fixtures `0030` + `0031` + BackendKind enum + runner switch

**Files:**
- Modify: `test/differential/fixture/fixture.go` (+1 enum `HTTPAdmissionControl BackendKind = 23`)
- Modify: `test/differential/runner_test.go` (+blank import + switch-case)
- Create: `test/fixtures/0030-http-admission-control/` (4 scenarios + driver + READMEs)
- Create: `test/fixtures/0031-http-admission-control-boot-reject/` (boot-reject driver + configs)
- Append: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (Task 9 entry)

This task lands the two differential fixture directories per SPEC §7 + the dispatch-constraint memory (one fixture dir = one runner branch). `0030` is the cross-side directory (4 scenarios; (b) byte-exact); `0031` is the boot-reject directory (implements `BootRejectFixture`). Depends on Task 8.

**Precondition:** Task 8 complete.
**Artifact:** `0030` + `0031` fixtures GREEN; fixture count 31 → 33.
**Acceptance:** `go test -count=1 ./test/differential/ -run 'Test.*0030'` GREEN (all 4 scenarios; (b) cross-side byte-exact); `go test -count=1 ./test/differential/ -run 'Test.*0031'` GREEN (boot-reject substring matches both sides); the full 0000-0029 regression still GREEN.

- [ ] **Step 1: Add the `BackendKind` enum** `HTTPAdmissionControl BackendKind = 23` in `fixture.go` (after `HTTPLua = 22`) with the doc-comment noting the reused always-200 / echobackend cluster.

- [ ] **Step 2: Add the runner switch-case + blank import** for `HTTPAdmissionControl` in `runner_test.go`.

- [ ] **Step 3: Author `0030-http-admission-control/`** — 4 scenario sub-directories per SPEC §7.1: `parse_ok` (a, REFERENCE-LESS — full config loads; admin /stats exposes 3 counters; HTTP 200), `all_admit_healthy` (b, CROSS-SIDE byte-exact — healthy backend ⇒ P_reject=0 ⇒ every request admitted byte-exact vs reference Envoy; rq_success increments; rq_rejected stays 0), `stat_surface` (c, REFERENCE-LESS — /stats exposes exactly the 3 counters with expected values after a healthy burst), `pass_through_disabled` (d, REFERENCE-LESS — enabled.default_value=false ⇒ all pass-through; counters stay 0). Each: `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md`. Shared `inputs/driver.go` (returns `BackendKind() = HTTPAdmissionControl`). Single-listener topology per SPEC §7.3 (no multi-listener — avoids the `freeTCPPort` flake).

- [ ] **Step 4: Author `0031-http-admission-control-boot-reject/`** — `envoy.yaml` + `envoy-go.yaml` both carrying `sr_threshold.default_value < 1.0%`; `inputs/driver.go` implementing `BootRejectFixture` per the 0029 precedent (`BootRejectScript()` + `ExpectedBootErrorSubstring() = "cannot be less than 1.0%"` per PD-2.boot); `expectations.yaml` + `README.md`. Driver returns `BackendKind() = HTTPAdmissionControl`.

- [ ] **Step 5: Run both fixtures + the regression.**

```bash
go test -count=1 ./test/differential/ -run 'Test.*003[01]'
go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-9])'
# Expect: 0030 (4 scenarios; (b) cross-side byte-exact) + 0031 GREEN; 0000-0029 regression GREEN
```

- [ ] **Step 6: Append PROGRESS.md Task 9 entry** (fixture count 31 → 33; SPEC §15 item 7 closure; the all-admit P=0 cross-side leg RATIFIED; boot-reject substring RATIFIED).

- [ ] **Step 7: Commit**

```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go \
        test/fixtures/0030-http-admission-control/ \
        test/fixtures/0031-http-admission-control-boot-reject/ \
        docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 9: differential fixtures 0030 (cross-side) + 0031 (boot-reject)

0030 4 scenarios: (b) all-admit P=0 byte-exact cross-side per AMEND-2;
(a)/(c)/(d) subject-only structural. 0031 sr_threshold<1.0% shared
boot-reject (substring 'cannot be less than 1.0%' per PD-2.boot).
BackendKind HTTPAdmissionControl=23. Fixture count 31 -> 33."
```

---

## Task 10: ADR final-state alignment + DECISIONS.md cross-reference audit

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (cross-reference audit; confirm ADR-0194 + ADR-0195 bodies at final state; ADR-0196 disposition note)
- Append: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (Task 10 entry)

This task audits the two ADR bodies (anchored at Tasks 2 + 4) for cross-reference integrity + records the ADR-0196 D-hypothesis disposition. Depends on Tasks 2-9.

**Precondition:** Tasks 2-9 complete.
**Artifact:** DECISIONS.md cross-references intact; ADR-0196 disposition recorded.
**Acceptance:** `grep -cE '^## ADR-0194' docs/envoy-go/DECISIONS.md` + `grep -cE '^## ADR-0195'` both `1` with non-empty §Decision + §Consequences bodies; `grep -cE '^## ADR-0196' docs/envoy-go/DECISIONS.md` returns `0` (D-hypothesis HELD) OR `1` with a justification if a surprise forced consumption; cross-reference links resolve.

- [ ] **Step 1: Audit ADR-0194 + ADR-0195 cross-references** — verify each §Decision + §Consequences body is non-empty + the cross-reference lists resolve (ADR-0186/0187 phase-21 precedent; ADR-0143; ADR-0080; ADR-0052; ADR-0072/0100; ADR-0114; ADR-0044; ADR-0045; ADR-0125 roster-STAYS-9 note).
- [ ] **Step 2: Record the ADR-0196 disposition** — if NO surprise fired (expected per the D-hypothesis), record ADR-0196 UNCONSUMED at phase-done in PROGRESS. If a surprise (e.g., a 4th counter, an unexpected stat name, a clamp-boundary float edge, OR a §Decision-level health-check rationale) fired, author ADR-0196 here with full §Context + §Decision + §Consequences and update the next-free pointer.
- [ ] **Step 3: Append PROGRESS.md Task 10 entry.**
- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 10: ADR final-state alignment + cross-reference audit (ADR-0196 UNCONSUMED)"
```

---

## Task 11: BEHAVIOR_CONTRACT.md 4-edit bundle per SPEC §13

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (4-edit bundle per SPEC §13)
- Append: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (Task 11 entry)

This task lands the BEHAVIOR_CONTRACT.md edit-bundle atomically per ADR-0052. Depends on Task 9 (some paragraphs reference the fixture wire-shape closures). Per ADR-0052 in-place-by-append discipline (no mutation of pre-phase-23 paragraphs).

**Precondition:** Task 9 complete.
**Artifact:** 4-edit bundle landed.
**Acceptance:** `grep -c '### envoy.filters.http.admission_control' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns `1`; the departure-record count reads 15; the stat table reads 110.

- [ ] **Step 1: Author the 4 edits** per SPEC §13:
  1. **NEW `### envoy.filters.http.admission_control` subsection** (inserted after `### envoy.filters.http.lua`) — filter scope (SRE-book client-side admission control; SIXTEENTH §9 row); the formula + integer-modulo decision; the both-sides decode-gate/encode-classify discipline (AMEND-1/2/4/5/10/11); the 3-counter stat surface (AMEND-3); the reject wire shape (503 / empty body per AMEND-7; **rc-details ABSENT-by-API per PD-2.503**); the **PD-3 health-check arm NOT-MODELED note** (no stream-info accessor; AMEND-11 health-check-not-recorded vacuous at MVP); listener-scoped discipline (REUSE-by-absence; first ADR-0125-skip since phase-22); the SINGLE envoy-go-strict departure (RTDS `runtime_key` PARSE-REJECT).
  2. **NEW envoy-go-strict departure record** — RTDS `runtime_key` PARSE-REJECT (upstream ACCEPTS; envoy-go PARSE-REJECTs; departure count 14 → 15).
  3. **Stat-name mapping 107 → 110 table extension** — 3 new counters under `http.<HCM_stat_prefix>.admission_control.*`.
  4. **Per-route canonical-patterns caption update** — "updated through phase 22" → "updated through phase 23" + a cross-reference paragraph documenting REUSE-by-absence (first ADR-0125-skip since phase-22; roster STAYS 9).
- [ ] **Step 2: Append PROGRESS.md Task 11 entry.**
- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 11: BEHAVIOR_CONTRACT.md 4-edit bundle (departure 14->15; stats 107->110)"
```

---

## Task 12: Six-gate phase-done verification + STATE.md re-advance + ROADMAP row 23 flip + REVIEW.md

**Files:**
- Modify: `docs/envoy-go/STATE.md` (rewrite-in-place — advance to post-phase-23 state)
- Modify: `docs/envoy-go/ROADMAP.md` (row 23 `in-progress → done` + per-cell IMPL-done annotation)
- Create: `docs/envoy-go/phases/23-http-filter-admission-control/REVIEW.md`
- Append: `docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md` (Task 12 entry)

This task runs the six-gate phase-done verification (A/B/C/D/E/F per SPEC §7.4 + §15) per `superpowers:verification-before-completion`, authors REVIEW.md per `superpowers:requesting-code-review`, re-advances STATE.md, and flips ROADMAP row 23. Depends on everything. **Per `superpowers:requesting-code-review`: if the review surfaces issues → back to the relevant Task (NOT this one) until REVIEW.md is approved.**

**Precondition:** Tasks 1-11 complete.
**Artifact:** all 6 gates GREEN; REVIEW.md approved; STATE + ROADMAP advanced.
**Acceptance:** the §15 16-item acceptance checklist all GREEN; REVIEW.md `Approved`.

- [ ] **Step 1: Gate A — build.** `go build ./...` clean. Capture verbatim output.
- [ ] **Step 2: Gate B — vet + lint.** `go vet ./...` + `golangci-lint run` clean; no new suppressions. Capture.
- [ ] **Step 3: Gate C — race.** `go test -race -count=1 ./...` clean (incl. the window controller's `mu`-guarded mutation). Capture.
- [ ] **Step 4: Gate D — differential.** `go test -count=1 ./test/differential/` — 33/33 GREEN (0000-0029 + 0030 + 0031); `0030` scenario (b) cross-side byte-exact; `0031` boot-reject substring. Capture. (If a combined-run `freeTCPPort` flake appears per 22.2 REVIEW §7.4, re-run the affected fixtures in isolation + note the flake — not a defect.)
- [ ] **Step 5: Gate E — fuzz.** `FuzzAdmissionControlConfigParse` clean at 30s per seed; no panics across the 32 fuzzers. Capture.
- [ ] **Step 6: Gate F — h2spec.** 53/53 PASS at ADR-0051 v1.32.4 pin. Capture.
- [ ] **Step 7: Author REVIEW.md** per `superpowers:requesting-code-review` — verify the §15 16-item acceptance checklist; quote the six-gate verbatim outputs; confirm the 1:1 PROGRESS↔Task mapping; record the ADR-0196 D-hypothesis disposition (UNCONSUMED at phase-done) + the PD-3 health-check NOT-MODELED deferral.
- [ ] **Step 8: Re-advance STATE.md** — lifecycle-state phase-23 IMPL done; next-skill per the next-phase identity (the §9 family next row: `wasm` or `global rate limit` per ROADMAP); next-free ADR-0196 (or ADR-0197 if a surprise consumed ADR-0196); 18 HTTP filters; 110 stats; 15 departure records; 33 fixtures; 32 fuzzers.
- [ ] **Step 9: Flip ROADMAP row 23** `in-progress → done` + per-cell IMPL-done annotation; §9 family closes to 2 remaining rows.
- [ ] **Step 10: Commit**

```bash
git add docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md \
        docs/envoy-go/phases/23-http-filter-admission-control/REVIEW.md \
        docs/envoy-go/phases/23-http-filter-admission-control/PROGRESS.md
git commit -m "phase 23 Task 12: six-gate phase-done verification + STATE/ROADMAP advance + REVIEW.md

All 6 gates GREEN (build/vet+lint/race/differential 33-33/fuzz/h2spec 53-53).
§15 16-item acceptance checklist GREEN. ADR-0196 UNCONSUMED at phase-done
(D-hypothesis HELD). Row 23 done; 18 HTTP filters; 110 stats; §9 family
closes to 2 remaining (wasm, global rate limit) [ADR-0194,ADR-0195]"
```

---

## Notes for the executor

- **Subagent-driven execution** per project memory `feedback_execution_style.md`: the IMPL session dispatches a fresh subagent per Task with two-stage review between tasks (per `superpowers:subagent-driven-development`). Tasks 2+3 (then 7) parallelize; Tasks 4→5→6→8→9→10/11→12 are the critical path.
- **TDD per `superpowers:test-driven-development`** on every task: write the failing test → verify it fails → minimal implementation → verify it passes → commit.
- **Empirical AMENDs are the SETTLED implementation contract** (SPEC §1.1) — do NOT re-derive or re-litigate the formula (AMEND-1), the integer-modulo decision + `Uint64()` seam (AMEND-2), the 3-counter roster (AMEND-3), enabled-absent⇒ENABLED (AMEND-4), the empty 503 reject body (AMEND-7), the `sr_threshold < 1.0%` boot-reject (AMEND-8), the deque window (AMEND-6), the gRPC-trailers path (AMEND-10), or the record discipline (AMEND-11).
- **Three SPEC-pseudocode-vs-reality gaps the implementer MUST honor** (per PD-1/PD-2.503/PD-3): the real factory signature is `New(tc *anypb.Any, ctx FactoryCtx)`; `SendLocalReply` is 3-arg (rc-details NOT surfaceable); there is NO health-check accessor (the gate arm is NOT-MODELED).
