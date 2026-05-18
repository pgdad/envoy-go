# Phase 21 — HTTP filter `envoy.filters.http.adaptive_concurrency` (single-row landing) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `envoy.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency` — the canonical Envoy v1.37.2 Gradient-1 adaptive-concurrency filter (estimates `minRTT` from sampled request RTTs and continuously adjusts a per-HCM-instance concurrency limit to bound tail latency under load) — as the FOURTEENTH §9 family-row under the 07.1 framework by shipping the NEW `internal/filter/http/adaptive_concurrency/` package (10 production + 6 test Go files; ~940-1320 LoC production + ~890-1390 LoC tests per SPEC §6.8) with the in-package Gradient-1 controller state machine + the inline `Clock` interface seam (NOT a new `internal/clock/` framework primitive per BRAINSTORM Q3 + SPEC §3.1) + the sorted-slice percentile aggregation helper (NOT CircllHist; ≤ 1 bin-width divergence acceptable per SPEC §2.4 + BRAINSTORM §8 item 4) + the 7-name HCM-rooted stat surface (1 counter `rq_blocked` + 6 gauges `concurrency_limit` / `gradient` / `burst_queue_size` / `sample_rtt_msecs` / `min_rtt_msecs` / `min_rtt_calculation_active`; stat-prefix template `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.<stat>` per AMEND-3 C2) + the 27th project-wide fuzzer `FuzzAdaptiveConcurrencyConfigParse` + the differential fixture `0025-http-adaptive-concurrency` (4 scenario directories per SPEC §7.1; **partial cross-side byte-exact on the 503-overflow leg per AMEND-6 + §21.P-D2 RATIFIED**) + the listener-scoped-only enforcement via REUSE-by-absence (FOURTH CONSECUTIVE §9 row to skip ADR-0125 amendment per SPEC §5.4), the IN-PLACE §Decision AMENDMENT to ADR-0059 (float-valued-gauge int64 encoding convention per SPEC §3.2 + D1 resolution — the BRAINSTORM-anticipated ADR-0188 candidate COLLAPSES per AMEND-7) with the NEW `internal/stats/conv.go` `boolToInt` helper + comment-only `gauge.go` cross-reference (~20-30 LoC delta), the boot-registration insertion at `cmd/envoy-go/main.go` line 125 (alphabetical between `router` at line 124 and `bandwidthlimit` which shifts to 126), the BEHAVIOR_CONTRACT.md 7-edit bundle per SPEC §13, and DECISIONS.md 2 NEW ADR §Decision + §Consequences bodies (ADR-0186 + ADR-0187) + 1 IN-PLACE AMENDMENT body (ADR-0059) — with byte-equivalent wire outcomes against reference Envoy v1.37.2 on the 503-overflow leg (status + 25-byte body + 2 headers per AMEND-6 + §21.P1 RATIFIED) and stat-name byte-equivalent on the 7-name surface (per AMEND-3 + §21.P3 PARTIAL), accepting two documented envoy-go-strict departures (RTT-ns-vs-ms per AMEND-3 C3 + sorted-slice-vs-CircllHist per BRAINSTORM §8 item 4). **Single-row landing settled at SPEC per ADR-0045** (LoC envelope ~1370-1830 production+fixture+AMENDMENT; PLAN-time re-evaluation per §Scope-check below CONFIRMS single-row — task count 14 well below 25-gate; LoC straddles ~1500 split-gate at the upper estimate but natural split axes don't carve cleanly per SPEC §3.5 — controller state machine + FAKE-TIME tests + percentile aggregation are tightly coupled; framework delta is ZERO).

**Architecture:** The IMPL adds ONE new package (`internal/filter/http/adaptive_concurrency/`) + ONE small new file (`internal/stats/conv.go`) + ONE in-place §Decision AMENDMENT to an existing ADR (ADR-0059) — the **LEANEST framework-delta §9 row to date** (FIRST §9 row since phase 14 compressor to introduce ZERO new `internal/` framework primitives per SPEC §3). The NEW package follows the phase-20 oauth2 / phase-17 jwt_authn multi-file split: `doc.go` (package doc) + `adaptive_concurrency.go` (TypeURL constant + `New` factory + `HTTPFilter` value + compile-time interface assertions per SPEC §6.1) + `compiled_config.go` (`compiledConfig` per SPEC §6.1 + `buildCompiledConfig` + PARSE-REJECT path with byte-stable error wording per planner-time D2 — 11 RATIFIED-from-PGV arms per SPEC §5.1 + 3 envoy-go-strict arms per SPEC §5.2 + the `fixed_value` deferral per SPEC §5.3) + `controller.go` (`gradientController` state machine per SPEC §4 + §6.2 — hot-path lock-free CAS via `atomic.Uint32` on `numRqOutstanding`; cold-path `sync.Mutex` guarding `latencySamples` / `minRTTSamples` / `deferredLimitValue` / `consecutiveMinConcurrencySet`; periodic concurrency-update tick via `sample_reset_timer`; periodic minRTT-recalc trigger via `min_rtt_calc_timer`; gradient formula `clamp(0.5, min_rtt × (1 + buffer) / sample_rtt, 2.0)` per SPEC §4.3 + `gradient_controller.cc:190-192`; new-limit `clamp(min_concurrency, currentLimit × gradient + sqrt(currentLimit × gradient), max_concurrency_limit)` per SPEC §4.4 + `cc:198-206`; minRTT recalc via `sample_aggregate_percentile`-quantile NOT MIN per AMEND-2 C1 + `cc:176-182`; jitter additive `[0, interval × jitter_pct)` to next-interval delay per AMEND-2 C2 + `cc:152-160`; 5-consecutive-min forced-recalc trigger per AMEND-2 C3 + `cc:281-283`; first-tick-immediate-window-entry semantics per AMEND-2 C4 + `cc:55-92`) + `clock.go` (in-package `Clock` interface seam per SPEC §3.1 + §6.3 — `Now() time.Time` + `AfterFunc(d time.Duration, fn func()) Stop` with `defaultClock` wrapping `time.Now` + `time.AfterFunc`; NOT framework primitive per consumer-count=1 + YAGNI per phase-17 jwt_authn EXTRACT-NOW-only-when-trigger-fires lesson; `fakeClock` lives in test scope only at `clock_test.go`) + `decode_headers.go` (`DecodeHeaders` per SPEC §6.4 — disabled pass-through; `controller.forwardingDecision()` CAS; on Block emit `SendLocalReply(concurrencyLimitExceededStatus, "reached concurrency limit", {content-type: text/plain}, ...)` per AMEND-6 byte-pinned wire shape + increment `rq_blocked` counter; on Forward record `entryTime = clock.Now()` + set `acquired = true`) + `encode_complete.go` (`OnEncodeComplete` per SPEC §6.5 — if `acquired`, compute `rtt := clock.Now().Sub(entryTime)`; route to `controller.recordLatencySample(rtt)` which appends to `latencySamples` or `minRTTSamples` depending on in-window state per SPEC §4.2; `controller.releaseInFlight()` decrements `numRqOutstanding`) + `stats.go` (`filterStats` 7-name roster per SPEC §6.6 + AMEND-3 — 1 counter + 6 gauges constructed via `Registry.NewCounter` + `Registry.NewGauge` under the `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.` prefix; **anchors the ADR-0059 §Decision AMENDMENT body** at Task 4 documenting the float-valued-gauge int64 encoding convention — ns for time-typed via `Gauge.Set(rtt.Nanoseconds())`; ×1000 for ratio-typed via `Gauge.Set(int64(gradient * 1000))`; 0/1 for bool-typed via `Gauge.Set(boolToInt(active))`) + `filter.go` (per-stream `filter` struct glue + `New` factory body wiring) + `percentile.go` (sorted-slice percentile aggregation helper per SPEC §6.8 + BRAINSTORM §8 item 4 carve-out — `Quantile(samples []time.Duration, p float64) time.Duration` returning the p-quantile via `sort.Slice` + index interpolation; edge cases per planner-time D10 — empty slice returns 0; single-sample slice returns that sample; p clamped to `[0.0, 1.0]`). The NEW `internal/stats/conv.go` adds `boolToInt(b bool) int64` (a 3-line helper) + the existing `internal/stats/gauge.go` gains a comment-only doc-extension cross-referencing the ADR-0059 AMENDMENT convention (NO signature change to `*stats.Gauge`). Listener-scoped-only enforcement is REUSE-by-absence per SPEC §5.4: the `envoy.extensions.filters.http.adaptive_concurrency.v3` proto package defines NO `AdaptiveConcurrencyPerRoute` message at v1.32.4 or v1.37.x, so any attempt to place adaptive_concurrency at the route or virtualHost level fails proto-deserialization-time PARSE-REJECT via the existing HCM framework — NO phase-21-specific PARSE-REJECT code; NO `RegisterPerRouteValidator` hook (unlike phase-10 header_mutation + phase-20 oauth2). Two algorithmic test layers per SPEC §14.1: **Layer A — Subject-only FAKE-TIME algorithmic-fidelity unit tests** at `controller_test.go` exercise the controller state machine deterministically via the `fakeClock` step driver (~10 test families per SPEC §14.1; `TestController_FAKE_TIME_FirstTickSemantics` + `TestController_FAKE_TIME_GradientFormula_*` + `TestController_FAKE_TIME_NewLimitCalculation_*` + `TestController_FAKE_TIME_MinRTTRecalcWindow_*` + `TestController_FAKE_TIME_JitterApplication_*` + `TestController_FAKE_TIME_FiveConsecutiveMinForcedRecalc` + `TestController_ConcurrentForwardingDecision_*` race tests + `TestController_503_BodyAndHeaders_ByteExact` + `TestPercentile_SortedSlice_*` vector tests + `TestBuildCompiledConfig_PARSE_REJECT_*` table-driven tests); **Layer B — Structural cross-side differential fixture** at `test/fixtures/0025-http-adaptive-concurrency/` with 4 scenario directories per SPEC §7.1 — (a) `parse_ok` REFERENCE-LESS + (b) `overflow_503` partial cross-side byte-exact per AMEND-6 + (c) `stat_surface` REFERENCE-LESS + (d) `pass_through_when_disabled` REFERENCE-LESS. Single-listener topology per SPEC §7.3 (adaptive_concurrency alphabetical-first + router terminator + synthetic slow-response backend cluster — 1-second response latency for scenario (b); fast-response for (a) + (c) + (d)). The phase-21 SPEC anchored 2 NEW ADRs (ADR-0186 + ADR-0187 §Context drafts) + 1 IN-PLACE §Decision AMENDMENT-anticipation paragraph (ADR-0059) at the SPEC commit `49ba034` (re-anchored at SHA-fill follow-up `3f0f768`); the IMPL lands the §Decision + §Consequences bodies + the 1 AMENDMENT body at their respective Tasks per ADR-0044 (ADR-0186 at Task 3 controller materialization; ADR-0187 at Task 2 compiled_config; ADR-0059 §Decision AMENDMENT body at Task 4 stats.go materialization). ADR-0044 escape-valve held in reserve for ~0-2 IMPL-time-unanticipated ADRs; PLAN's strong hypothesis per planner-time D8: NO additional ADR fires at phase-21 IMPL (next-free ADR-0188 stays unconsumed at phase-21 phase-done; STRENGTHENED two-slot buffer with ADR-0189 also UNCONSUMED).

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module (proto pin per ADR-0008; `envoy/extensions/filters/http/adaptive_concurrency/v3` for the filter config — `AdaptiveConcurrency` message + `GradientControllerConfig` sub-message at the only `concurrency_controller_config` oneof arm); stdlib `crypto/rand` (NOT consumed — no crypto surface); stdlib `math` (`math.Max` + `math.Min` + `math.Sqrt` for the gradient formula clamp + sqrt-burst-headroom per SPEC §4.3 + §4.4); stdlib `sort` (`sort.Slice` for the sorted-slice quantile per `percentile.go`); stdlib `sync` (`sync.Mutex` for the cold-path samples + recalc-window state; `sync.Once` is NOT needed at phase-21); stdlib `sync/atomic` (`atomic.Uint32` for `concurrencyLimit` + `numRqOutstanding` hot-path lock-free CAS); stdlib `time` (`time.Time` + `time.Duration` + `time.AfterFunc` + `time.Now` for the production `defaultClock`; `time.Nanoseconds` for RTT gauge encoding per AMEND-3 C3 + ADR-0059 §Decision AMENDMENT); stdlib `math/rand` (for the jitter randomization per SPEC §4.5 + `gradient_controller.cc:152-160`; `*rand.Rand` constructed at controller construction with a fixed-but-monotonic seed); reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 + ENVOY_TARGET.md — unchanged); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext upstream backend fixture (NO TLS surface at phase-21). **NO new go.mod direct deps** (LEANEST §9 row).

---

## Scope check — why phase 21 ships as one row (single-row settled at SPEC per ADR-0045)

The phase-21 SPEC author settled the split disposition per SPEC §3.5 + ROADMAP row 21 + ADR-0045: **SINGLE-ROW landing** (no sub-rows `21.1`/`21.2`). The LoC envelope re-estimated post-empirical-scrape at ~1370-1830 production+fixture+AMENDMENT (slightly above BRAINSTORM's ~1200-1500 envelope due to the algorithmic-invariant AMENDs at AMEND-2 — extra state-machine cases for the 5-consecutive-min forced-recalc trigger per AMEND-2 C3 + first-tick-immediate-window-entry semantics per AMEND-2 C4 + the percentile-based-not-MIN minRTT-recalc aggregation per AMEND-2 C1) — but well below the ADR-0045 split-gate at the low end (1370 < 1500) and brushes the gate at the high end (1830). The PLAN-time re-evaluation per `superpowers:writing-plans` GATE + ADR-0045 §6 confirms single-row landing:

- **Task count: 14** — comfortably under the ADR-0045 25-task split-gate (matches phase-20 oauth2's 14-task PLAN despite phase-21's significantly leaner framework surface — phase-21 has fewer ADRs and no new framework primitives but the same per-task structure mirrors phase-20's TDD-disciplined per-step shape).
- **LoC: ~1370-1830 production+fixture+AMENDMENT** — straddles the ~1500-LoC split-gate at the upper estimate. Per SPEC §3.5 and BRAINSTORM §1.4: the natural split axes (controller vs fixture; framework vs filter; subject-only tests vs cross-side fixture) **don't carve cleanly** — the Gradient-1 controller state machine + the FAKE-TIME algorithmic-fidelity tests + the sorted-slice percentile aggregation are tightly coupled (the percentile helper is consumed BOTH by the per-tick sample-RTT aggregation AND by the per-recalc-window minRTT aggregation; splitting them out would force cross-task state-machine coupling); the framework delta is ZERO (no clear "framework vs filter" split axis — phase-21 IS just one filter with no framework primitives); the 4 fixture scenarios are co-located in one fixture directory (`0025-http-adaptive-concurrency/<scenario>/`) per the phase-19.2 + phase-20 differential-fixture-directory pattern (no clear "fixture vs filter" split axis).
- **Phase 21 ships as the single row it is** — no further split. The phase-21 phase-done squash-merge **CLOSES row 21** (in-progress → done) at the same commit; there is no parent-row rollup discipline (mirrors phase-20 oauth2 single-row landing; phase-17 jwt_authn single-row at 3855 LoC precedent).

Net change estimate for phase 21 (mirroring the phase-09..20 PLAN component-table convention):

- `internal/filter/http/adaptive_concurrency/doc.go` ~15-25 (package doc per SPEC §6.8)
- `internal/filter/http/adaptive_concurrency/adaptive_concurrency.go` ~80-120 (TypeURL constant + `New` factory + compile-time interface assertions per SPEC §6.1; populated at Task 9 integration)
- `internal/filter/http/adaptive_concurrency/compiled_config.go` ~200-280 (`compiledConfig` struct shape per SPEC §6.1 + `buildCompiledConfig` body + PARSE-REJECT path with byte-stable error messages per D2; populates `enabled` + `concurrencyLimitExceededStatus` + `sampleAggregatePercentile` + `maxConcurrencyLimit` + `concurrencyUpdateInterval` + `minRTTCalcInterval` + `minRTTRequestCount` + `minRTTJitterPct` + `minRTTMinConcurrency` + `minRTTBufferPct`)
- `internal/filter/http/adaptive_concurrency/controller.go` ~350-450 (`gradientController` state machine per SPEC §4 + §6.2: lock-free `forwardingDecision()` CAS loop per §4.1; `recordLatencySample(rtt)` route-to-active-slice per §4.2; `concurrencyUpdateTick()` timer callback per §4.2; `enterMinRTTSamplingWindow()` + `updateMinRTT()` per §4.5; `calculateNewLimit(gradient float64) uint32` per §4.4; `computeGradient(minRTT, sampleRTT, bufferPct)` per §4.3; 5-consecutive-min forced-recalc bookkeeping per AMEND-2 C3; first-tick semantics per AMEND-2 C4)
- `internal/filter/http/adaptive_concurrency/clock.go` ~30-60 (`Clock` interface + `Stop` interface + `defaultClock` + `timerStop` per SPEC §3.1 + §6.3)
- `internal/filter/http/adaptive_concurrency/decode_headers.go` ~50-80 (`DecodeHeaders` dispatch + 503-overflow `SendLocalReply` per SPEC §6.4 + AMEND-6 byte-pinned wire shape)
- `internal/filter/http/adaptive_concurrency/encode_complete.go` ~50-80 (`OnEncodeComplete` sample-recording + `releaseInFlight` per SPEC §6.5)
- `internal/filter/http/adaptive_concurrency/stats.go` ~80-120 (`filterStats` 7-name roster + `newFilterStats` constructor per SPEC §6.6 + AMEND-3 + ADR-0059 §Decision AMENDMENT consumer)
- `internal/filter/http/adaptive_concurrency/filter.go` ~60-100 (per-stream `filter` struct + glue between `DecodeHeaders` / `OnEncodeComplete` / controller)
- `internal/filter/http/adaptive_concurrency/percentile.go` ~50-80 (sorted-slice `Quantile(samples []time.Duration, p float64) time.Duration` per SPEC §6.8 + BRAINSTORM §8 item 4)
- `internal/filter/http/adaptive_concurrency/adaptive_concurrency_test.go` ~80-120 (`TypeURL` constant assertion + `New` factory integration tests)
- `internal/filter/http/adaptive_concurrency/compiled_config_test.go` ~250-350 (`TestBuildCompiledConfig_PARSE_REJECT_*` table-driven tests per SPEC §14.1; ~25-30 rows covering RATIFIED-from-PGV + envoy-go-strict + `fixed_value` deferral arms per D2)
- `internal/filter/http/adaptive_concurrency/controller_test.go` ~400-550 (`TestController_FAKE_TIME_*` algorithmic-fidelity tests per SPEC §14.1 Layer A; `TestController_ConcurrentForwardingDecision_*` race tests per SPEC §12 item B6 + D3; `TestController_503_BodyAndHeaders_ByteExact` per AMEND-6 + §12 item A1+A2)
- `internal/filter/http/adaptive_concurrency/clock_test.go` ~80-150 (`fakeClock` test-scope implementation + `Advance(d time.Duration)` step driver + deterministic-ordering tests per planner-time D9)
- `internal/filter/http/adaptive_concurrency/percentile_test.go` ~80-120 (`TestPercentile_SortedSlice_*` vector tests + edge cases per planner-time D10 — empty slice; single-sample; p=0; p=1; out-of-range p clamp)
- `internal/filter/http/adaptive_concurrency/fuzz_test.go` ~50 (27th fuzzer `FuzzAdaptiveConcurrencyConfigParse` per SPEC §6.7 + planner-time D6)
- `internal/filter/http/adaptive_concurrency/testdata/fuzz/FuzzAdaptiveConcurrencyConfigParse/` (corpus seeds per D6; ~30 seeds)
- `internal/stats/conv.go` ~10-15 NEW (`boolToInt(b bool) int64` helper per SPEC §3.2 + ADR-0059 §Decision AMENDMENT)
- `internal/stats/gauge.go` ~+5-10 (comment-only doc-extension cross-referencing the ADR-0059 §Decision AMENDMENT convention; NO signature change)
- `cmd/envoy-go/main.go` ~+1 LoC + +1 import (`adaptive_concurrency "github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency"`; `httpReg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)` inserted alphabetical at line 125 between `router` at line 124 and `bandwidthlimit` which shifts from 125 to 126 per ADR-0100 §2.2 + planner-time D7). **NO `RegisterPerRouteValidator` call** — REUSE-by-absence per SPEC §5.4 (FOURTH CONSECUTIVE §9 row).
- `test/differential/fixture/fixture.go` ~+15 (NEW `BackendKind` enum value `HTTPAdaptiveConcurrency BackendKind = 21` after `HTTPOAuth2 = 20`)
- `test/differential/runner_test.go` ~+12 (blank import + switch-case for `HTTPAdaptiveConcurrency`)
- `test/fixtures/0025-http-adaptive-concurrency/` (NEW DIRECTORY) — 4 scenario sub-directories + shared `inputs/driver.go`; per-scenario `envoy.yaml` ~80-120 + `envoy-go.yaml` ~80-120 + `expectations.yaml` ~30-60 + `README.md` ~40-80 per scenario (a) + (b) + (c) + (d); the (b) scenario includes the byte-pinned cross-side expectations per AMEND-6. Subtotal ~250-400 LoC fixture material.
- `docs/envoy-go/DECISIONS.md` — 2 NEW ADR §Decision + §Consequences bodies (ADR-0186 + ADR-0187) + 1 IN-PLACE §Decision AMENDMENT body (ADR-0059); ~+200-300 LoC. NO new ADR numbers consumed at IMPL under D8 hypothesis (next-free stays ADR-0188).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` ~+250-350 (§13 7-edit bundle per SPEC §13 — NEW `### envoy.filters.http.adaptive_concurrency` subsection ~150-250 LoC + stat-table 92→99 extension + ADR-0059 §Decision AMENDMENT cross-reference + 2 envoy-go-strict departure records + NEW `### Phase 21 forward-pointer notes` subsection + Per-route canonical patterns cross-reference table caption update)
- `docs/envoy-go/ROADMAP.md` row 21 flips `in-progress → done` at phase-done; per-cell IMPL-done annotation added; ~+1 net
- `docs/envoy-go/STATE.md` rewrite-in-place at Task 14
- `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (NEW) ~600-900 across 14 task entries
- `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/REVIEW.md` (NEW) ~300

**Production code: ~940-1320 LoC** (`internal/filter/http/adaptive_concurrency/` ~890-1290 + `internal/stats/conv.go` ~10-15 + `internal/stats/gauge.go` comment-only ~5-10 + boot-registration + enum +2 net) **+ ~890-1390 LoC tests = ~1830-2710 LoC production+test** + ~250-400 LoC fixture + ~450-650 LoC docs ≈ **~2530-3760 LoC total**. **Task count below is 14** — comfortably under the ADR-0045 25-task split-gate (mirrors phase-20 oauth2 14-task PLAN exactly). Single-row landing settled.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/stats/conv.go` | NEW | `boolToInt(b bool) int64` helper (~3 LoC body + ~7 LoC package preamble; returns 1 if true, 0 otherwise). Documented as the sibling helper consumed by phase-21's `filterStats` per ADR-0059 §Decision AMENDMENT (float-valued-gauge int64 encoding convention). Cross-phase-reusable for any future bool-typed gauge. ~10-15 LoC. ADR-0059 §Decision AMENDMENT body lands here at Task 4 (the IN-PLACE AMENDMENT to the existing ADR-0059 §Decision body extends the canonical Internal Stats Store architecture per ADR-0044 in-place edit discipline). |
| `internal/stats/gauge.go` | IN-PLACE COMMENT-ONLY | Add doc-comment paragraph cross-referencing the ADR-0059 §Decision AMENDMENT for the float-valued-gauge int64 encoding convention. NO signature change to `*stats.Gauge`. ~+5-10 LoC delta. Per ADR-0044 in-place edit discipline. |
| `internal/filter/http/adaptive_concurrency/doc.go` | NEW | Package doc per SPEC §6.8 — enumerates: package purpose (Gradient-1 adaptive-concurrency filter); canonical TypeURL; the per-HCM-instance controller-state-machine semantics; the 7-name stat surface; cross-reference to ADR-0186 + ADR-0187 + ADR-0059 §Decision AMENDMENT. ~15-25 LoC. |
| `internal/filter/http/adaptive_concurrency/adaptive_concurrency.go` | NEW | Main file. **Public surface** per SPEC §6.1: `const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency"` + `func New(message proto.Message) (api.HTTPFilterFactory, error)`. `New` calls `buildCompiledConfig` (in `compiled_config.go`); constructs the `*filterStats` via `newFilterStats(ctx.HCMStatPrefix())` + the `*gradientController` via `newGradientController(cc, stats, defaultClock{})`; returns the factory closure that produces per-stream `*filter` instances (the `filter` struct lives at `filter.go`). Compile-time interface assertions: `var _ api.StreamFilter = (*filter)(nil)` + `var _ api.StreamDecoderFilter = (*filter)(nil)` (the encoder-side hook is via `OnEncodeComplete` per SPEC §6.5 — the encode-side hook participates per the existing HCM framework; check Task 9 integration for the exact compile-time-assertion roster against the project's `api.StreamFilter` shape post-phase-19.2). ~80-120 LoC. ADR-0186 §Decision + §Consequences cross-references at Task 3 + Task 9. **NO `RegisterPerRouteValidator` function** — REUSE-by-absence per SPEC §5.4 (FOURTH CONSECUTIVE §9 row). |
| `internal/filter/http/adaptive_concurrency/compiled_config.go` | NEW | `compiledConfig` struct per SPEC §6.1 (verbatim — `enabled bool` + `concurrencyLimitExceededStatus uint32` + GradientControllerConfig sub-surface `sampleAggregatePercentile float64` + `maxConcurrencyLimit uint32` + `concurrencyUpdateInterval time.Duration` + `minRTTCalcInterval time.Duration` + `minRTTRequestCount uint32` + `minRTTJitterPct float64` + `minRTTMinConcurrency uint32` + `minRTTBufferPct float64`) + `buildCompiledConfig(typedConfig *anypb.Any) (*compiledConfig, error)` body covering all PARSE-REJECT cases per D2: (1) Unmarshal the `*anypb.Any` to `AdaptiveConcurrency`; (2) PARSE-REJECT `concurrency_controller_config` oneof absent; (3) PARSE-REJECT `concurrency_limit_params` required-message absent; (4) PARSE-REJECT `min_rtt_calc_params` required-message absent; (5) PARSE-REJECT `concurrency_update_interval == 0`; (6) PARSE-REJECT `max_concurrency_limit == 0` when set; (7) PARSE-REJECT `min_concurrency == 0` when set; (8) PARSE-REJECT `request_count == 0` when set; (9) PARSE-REJECT `min_rtt_calc_params.interval < 1ms` when set; (10) PARSE-REJECT `sample_aggregate_percentile` out of `[0, 100]`; (11) PARSE-REJECT `jitter > 100`; (12) PARSE-REJECT `buffer > 100`; (13) PARSE-REJECT `enabled.runtime_key != ""` per ADR-0187; (14) PARSE-REJECT `min_rtt_calc_params.fixed_value` set per AMEND-1 C4 + ADR-0186 §Consequences (d); (15) Apply defaults per SPEC §6.1 + AMEND-4 (enabled false when absent; status 503 default; sampleAggregatePercentile 0.5; maxConcurrencyLimit 1000; minRTTRequestCount 50; minRTTJitterPct 0.15; minRTTMinConcurrency 3; minRTTBufferPct 0.25). ~200-280 LoC. ADR-0187 §Decision + §Consequences body anchored here at Task 2. |
| `internal/filter/http/adaptive_concurrency/controller.go` | NEW | `gradientController` state machine per SPEC §4 + §6.2 (verbatim). Hot-path: `forwardingDecision() (forward bool)` per SPEC §4.1 — lock-free CAS loop on `numRqOutstanding` against `concurrencyLimit.Load()`; on Block returns `false` + caller increments `rq_blocked`; on Forward returns `true` + CAS-succeeds-with-incremented-value. `recordLatencySample(rtt time.Duration)` per SPEC §4.2 — acquires `mu`; if `deferredLimitValue != 0` (in minRTT window) appends to `minRTTSamples`; else appends to `latencySamples`; releases `mu`. `releaseInFlight()` — atomic decrement of `numRqOutstanding`. Cold-path: `concurrencyUpdateTick()` per SPEC §4.2 — acquires `mu`; if `len(latencySamples) == 0` re-arms timer + returns; else `sampleRTT := Quantile(latencySamples, cfg.sampleAggregatePercentile)`; clears `latencySamples`; updates `sample_rtt_msecs` gauge (`int64 sampleRTT.Nanoseconds()` per AMEND-3 C3); calls `updateConcurrencyLimit(calculateNewLimit(computeGradient(minRTT, sampleRTT, cfg.minRTTBufferPct)))`; re-arms `sampleResetTimer` for next `cfg.concurrencyUpdateInterval`. `enterMinRTTSamplingWindow()` per SPEC §4.5 (a)-(e) — saves `concurrencyLimit.Load()` to `deferredLimitValue`; clamps `concurrencyLimit` to `cfg.minRTTMinConcurrency`; clears `minRTTSamples`; sets `min_rtt_calculation_active` gauge to 1. `updateMinRTT()` per SPEC §4.5 (f)-(j) — if `len(minRTTSamples) < cfg.minRTTRequestCount` returns (more samples needed); else `minRTT := Quantile(minRTTSamples, cfg.sampleAggregatePercentile)`; updates `min_rtt_msecs` gauge (ns per AMEND-3 C3); clears `minRTTSamples`; sets `min_rtt_calculation_active` gauge to 0; restores `concurrencyLimit` to `deferredLimitValue`; sets `deferredLimitValue = 0`; re-arms `minRTTCalcTimer` per `applyJitter(cfg.minRTTCalcInterval, cfg.minRTTJitterPct)`; re-arms `sampleResetTimer`. `calculateNewLimit(gradient float64) uint32` per SPEC §4.4 — `limit := float64(concurrencyLimit) * gradient`; `burstHeadroom := math.Sqrt(limit)`; `stats.burstQueueSize.Set(int64(burstHeadroom))`; `newLimit := uint32(limit + burstHeadroom)`; clamp `[cfg.minRTTMinConcurrency, cfg.maxConcurrencyLimit]`. `computeGradient(minRTT, sampleRTT time.Duration, bufferPct float64) float64` per SPEC §4.3 — `bufferedMinRTT := float64(minRTT) * (1.0 + bufferPct)`; `raw := bufferedMinRTT / float64(sampleRTT)`; `math.Max(0.5, math.Min(2.0, raw))`. `updateConcurrencyLimit(newLimit uint32)` per AMEND-2 C3 — if `newLimit == oldLimit == cfg.minRTTMinConcurrency` increment `consecutiveMinConcurrencySet`; else reset to 0; if counter >= 5 AND `deferredLimitValue == 0` (NOT in minRTT window) AND `isMinRTTSamplingEnabled()` (TRUE at MVP per SPEC §4.6 since `fixed_value` PARSE-REJECTed) → `minRTTCalcTimer.Stop()` + re-arm at 0ms (force-arms an immediate recalc). `applyJitter(interval time.Duration, jitterPct float64) time.Duration` per AMEND-2 C2 — `jitterRange := time.Duration(float64(interval) * jitterPct)`; `interval + time.Duration(rand.Int63n(int64(jitterRange)))`. `newGradientController(cfg, stats, clock)` per AMEND-2 C4 first-tick semantics — initial `concurrencyLimit = cfg.minRTTMinConcurrency`; `deferredLimitValue = 0`; `numRqOutstanding = 0`; calls `enterMinRTTSamplingWindow()` immediately; enables `sampleResetTimer` (its callback short-circuits while `deferredLimitValue != 0` per SPEC §4.5). ~350-450 LoC. ADR-0186 §Decision + §Consequences body anchored here at Task 3. |
| `internal/filter/http/adaptive_concurrency/clock.go` | NEW | `Clock` interface (`Now() time.Time` + `AfterFunc(d time.Duration, fn func()) Stop`) + `Stop` interface (`Stop() bool`) + `defaultClock struct{}` (production wrapping `time.Now` + `time.AfterFunc`) + `timerStop{t *time.Timer}` per SPEC §3.1 + §6.3. ~30-60 LoC. ADR-0186 §Decision + §Consequences sub-paragraph anchored here at Task 3 (the in-package-NOT-framework-primitive decision; the future EXTRACT-NOW trigger when a second timer-driven filter materializes). |
| `internal/filter/http/adaptive_concurrency/decode_headers.go` | NEW | `DecodeHeaders(headers Headers, endStream bool) FilterStatus` per SPEC §6.4. If `!f.cc.enabled` returns `Continue` (filter disabled per AMEND-4 default-OFF). Else calls `f.controller.forwardingDecision()`; on Block calls `f.cb.SendLocalReply(int(f.cc.concurrencyLimitExceededStatus), "reached concurrency limit", map[string]string{"content-type": "text/plain"}, nil, "reached_concurrency_limit")` per AMEND-6 byte-pinned wire shape (status default 503; body 25 bytes verbatim; content-type + content-length headers set by HCM `SendLocalReply` defaults per §11 §21.P1 RATIFIED); increments `f.cc.stats.rqBlocked` counter; returns `StopAndBuffer`. On Forward sets `f.entryTime = f.clock.Now()` + `f.acquired = true`; returns `Continue`. **NOTE on `response_code_details`**: emits `"reached_concurrency_limit"` to the access-log slot — envoy-go MVP may not surface this; treat as ABSENT-by-config per SPEC §12 item A3 (NOT byte-pinned). ~50-80 LoC. |
| `internal/filter/http/adaptive_concurrency/encode_complete.go` | NEW | `OnEncodeComplete()` per SPEC §6.5. If `!f.acquired` (Block path or disabled) returns. Else computes `rtt := f.clock.Now().Sub(f.entryTime)`; calls `f.controller.recordLatencySample(rtt)` (routes to `latencySamples` or `minRTTSamples` depending on in-window state per SPEC §4.2 + controller logic); calls `f.controller.releaseInFlight()` (atomic decrement of `numRqOutstanding`); sets `f.acquired = false`. **NOTE on stream-reset / abort paths**: `OnDestroy()` MUST call `f.controller.releaseInFlight()` if `f.acquired` to prevent token leak — settled at planner-time D11 (the destroy-without-encode-complete path is non-negligible per the existing HCM framework; the filter's `OnDestroy` callback must symmetrically release any acquired token). ~50-80 LoC. |
| `internal/filter/http/adaptive_concurrency/stats.go` | NEW | `filterStats` struct + `newFilterStats` constructor per SPEC §6.6 + AMEND-3. 7 fields: `rqBlocked *stats.Counter`; `concurrencyLimit *stats.Gauge`; `gradient *stats.Gauge` (int64 ×1000 per ADR-0059 §Decision AMENDMENT — Task 4 anchor); `burstQueueSize *stats.Gauge`; `sampleRTTMsecs *stats.Gauge` (int64 ns per AMEND-3 C3 — envoy-go-strict departure; stat-name keeps upstream byte-exact `sample_rtt_msecs`); `minRTTMsecs *stats.Gauge` (int64 ns per AMEND-3 C3); `minRTTCalculationActive *stats.Gauge` (int64 0/1 per ADR-0059 §Decision AMENDMENT). `newFilterStats(reg *stats.Registry, hcmPrefix string) *filterStats` constructs each via `reg.NewCounter` + `reg.NewGauge` under the prefix `hcmPrefix + ".adaptive_concurrency.gradient_controller."` per AMEND-3 C2 (the `http.<HCM_stat_prefix>` is HCM-injected via `hcmPrefix` per ADR-0143 SN2-reuse). Stat-name compile-time guards per planner-time D5 — package-level `const` declarations for each of the 7 stat names; `newFilterStats` reads constants directly when registering each. A table-driven `TestStatNames_Equal_*` test in `adaptive_concurrency_test.go` asserts the 7 constants byte-exact against the wire-expected names per ADR-0143 SN2-reuse + the phase-17/18.x/19.x/20 precedent. ~80-120 LoC. **ADR-0059 §Decision AMENDMENT body lands here at Task 4** (the IN-PLACE AMENDMENT to ADR-0059's existing §Decision body extends the canonical Internal Stats Store architecture per ADR-0044 + the AMENDMENT-anticipation paragraph at the SPEC commit `49ba034`). |
| `internal/filter/http/adaptive_concurrency/filter.go` | NEW | Per-stream `filter` struct + glue. Fields: `cc *compiledConfig`; `controller *gradientController`; `clock Clock`; `cb api.DecoderFilterCallbacks`; `entryTime time.Time` (zero unless `acquired`); `acquired bool` (true between successful `forwardingDecision()` Forward at `DecodeHeaders` and `OnEncodeComplete` / `OnDestroy`); `parentCtx context.Context`. Methods route per SPEC §6.3 (`DecodeHeaders` per `decode_headers.go`; `OnEncodeComplete` per `encode_complete.go`; `OnDestroy` for the abort-cleanup token-release per planner-time D11). The shared `*gradientController` instance is hoisted to the factory level (one per `compiledConfig` instance — i.e., one per HCM filter chain mounting an adaptive_concurrency filter); each per-stream `*filter` instance captures the shared pointer. ~60-100 LoC. |
| `internal/filter/http/adaptive_concurrency/percentile.go` | NEW | `Quantile(samples []time.Duration, p float64) time.Duration` per SPEC §6.8 + BRAINSTORM §8 item 4 carve-out. Sorted-slice quantile aggregation (NOT CircllHist; ≤ 1 bin-width divergence acceptable per SPEC §2.4 + ADR-0186 §Decision). Algorithm: copy input slice to a fresh `[]time.Duration` (preserve caller's input); `sort.Slice` ascending; clamp `p` to `[0.0, 1.0]`; compute index `idx := int(p * float64(len(sorted)-1))`; return `sorted[idx]`. Edge cases per planner-time D10: `len(samples) == 0` returns `0`; `len(samples) == 1` returns `samples[0]`; out-of-range p (negative / > 1.0) clamps. ~50-80 LoC. ADR-0186 §Decision sub-paragraph anchored here at Task 7 (the sorted-slice-NOT-CircllHist decision; the ≤ 1 bin-width divergence acceptance). |
| `internal/filter/http/adaptive_concurrency/adaptive_concurrency_test.go` | NEW | Integration tests for `TypeURL` + `New` + the 7-stat-name compile-time guards per planner-time D5. ~80-120 LoC. |
| `internal/filter/http/adaptive_concurrency/compiled_config_test.go` | NEW | `TestBuildCompiledConfig_PARSE_REJECT_*` table-driven tests per SPEC §14.1 + planner-time D2 byte-stable wording. ~20-25 rows covering: 13 distinct PARSE-REJECT arms per SPEC §5.1 + §5.2 + §5.3 (concurrency_controller_config oneof required; concurrency_limit_params required absent; min_rtt_calc_params required absent; concurrency_update_interval == 0; max_concurrency_limit == 0; min_concurrency == 0; request_count == 0; min_rtt_calc_params.interval < 1ms; sample_aggregate_percentile out of [0, 100]; jitter > 100; buffer > 100; enabled.runtime_key != ""; min_rtt_calc_params.fixed_value set). Each row asserts `err != nil && err.Error() == "<expected D2 string>"`. PLUS Default-applied tests for AMEND-4 + defaults per SPEC §6.1 (enabled absent → false; concurrencyLimitExceededStatus default 503; sampleAggregatePercentile default 0.5; etc.). ~250-350 LoC. |
| `internal/filter/http/adaptive_concurrency/controller_test.go` | NEW | `TestController_FAKE_TIME_*` algorithmic-fidelity tests per SPEC §14.1 Layer A — 10 test families: FirstTickSemantics (per AMEND-2 C4); GradientFormula_* (per SPEC §4.3 — clamp at 0.5 / 2.0 + buffer application); NewLimitCalculation_* (per SPEC §4.4 — sqrt-burst-headroom + double-clamp); MinRTTRecalcWindow_* (per SPEC §4.5 + AMEND-2 C1 — percentile-aggregation NOT MIN; vector tests with crafted sample sets); JitterApplication_* (per AMEND-2 C2 — additive `[0, interval × jitter_pct)` to next-interval delay); FiveConsecutiveMinForcedRecalc (per AMEND-2 C3 — force-arms `minRTTCalcTimer` at 0ms when 5 consecutive ticks pin limit at minConcurrency). `TestController_ConcurrentForwardingDecision_*` race tests per SPEC §12 item B6 + planner-time D3 — N concurrent forwarders against limit=K; verify exactly K succeed + N-K block; verify arrival-order determinism under serialized connection-establishment. `TestController_503_BodyAndHeaders_ByteExact` per AMEND-6 + §12 items A1+A2 — verify the 503 body `"reached concurrency limit"` (25 bytes) + `content-type: text/plain` + `content-length: 25` against the subject-only emission. ~400-550 LoC. |
| `internal/filter/http/adaptive_concurrency/clock_test.go` | NEW | `fakeClock` test-scope implementation per SPEC §3.1 + planner-time D9: `fakeClock` struct holding `now time.Time` + `[]*fakeTimer` heap; `Now() time.Time` returns `c.now`; `AfterFunc(d time.Duration, fn func()) Stop` returns `*fakeTimer` registered in the heap; `Advance(d time.Duration)` advances `c.now` + synchronously fires all timers whose deadline ≤ current+d in deadline-asc order (deterministic ordering per D9). Tests: `TestFakeClock_Now_*`; `TestFakeClock_AfterFunc_FiresAtDeadline`; `TestFakeClock_Advance_FiresInDeadlineOrder` (the determinism test per D9); `TestFakeClock_StopBeforeFire_PreventsCallback`. ~80-150 LoC. |
| `internal/filter/http/adaptive_concurrency/percentile_test.go` | NEW | `TestPercentile_SortedSlice_*` vector tests + edge cases per planner-time D10: p=0.0 returns min; p=1.0 returns max; p=0.5 returns median for known sample set; p out of range clamps; empty slice returns 0; single-sample slice returns sample. ~80-120 LoC. |
| `internal/filter/http/adaptive_concurrency/fuzz_test.go` | NEW | 27th fuzzer `FuzzAdaptiveConcurrencyConfigParse` per SPEC §6.7 + planner-time D6 corpus seeds — must-never-panic across `buildCompiledConfig`. Clean at 30s per seed. ~50 LoC. |
| `internal/filter/http/adaptive_concurrency/testdata/fuzz/FuzzAdaptiveConcurrencyConfigParse/` | NEW | Corpus seeds per D6 — ~30 seeds covering each PARSE-REJECT arm + the valid full-config edge cases + empty config + oneof-absent + nested-message-missing variants. |
| `cmd/envoy-go/main.go` | MODIFY | +1 LoC + +1 import. Add import `adaptive_concurrency "github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency"` (alphabetical-first per Go import ordering; before `bandwidthlimit`). Add `httpReg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)` at line 125 (between `router` at line 124 and `bandwidthlimit` which shifts from 125 to 126) per ADR-0100 §2.2 + planner-time D7. **NO `RegisterPerRouteValidator` call** — REUSE-by-absence per SPEC §5.4. **16 HTTP filters wired post-phase-21** (router + 15 §9 filters). |
| `test/differential/fixture/fixture.go` | MODIFY | +1 enum value `HTTPAdaptiveConcurrency BackendKind = 21` (after `HTTPOAuth2 = 20`). ~+15 LoC including the doc-comment per the existing BackendKind comment style. |
| `test/differential/runner_test.go` | MODIFY | +blank import + switch-case for `HTTPAdaptiveConcurrency`. ~+12 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/parse_ok/envoy.yaml` | NEW | Scenario (a) reference Envoy config — single-listener HCM with adaptive_concurrency filter (alphabetical-first) + full Gradient-1 config + router terminator. ~80-120 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/parse_ok/envoy-go.yaml` | NEW | Scenario (a) envoy-go-side config — mirrors the upstream config. ~80-120 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/parse_ok/expectations.yaml` | NEW | Scenario (a) — REFERENCE-LESS subject-only structural; HTTP 200 to a normal GET + admin `/stats` exposes the 7-name surface with starting values. ~30-60 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/parse_ok/README.md` | NEW | Scenario narrative. ~40-80 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/overflow_503/envoy.yaml` | NEW | Scenario (b) reference Envoy config — single-listener HCM with adaptive_concurrency filter; config: `max_concurrency_limit=1 + min_concurrency=1 + concurrency_limit_exceeded_status=503`; backend cluster with synthetic slow 1-second response latency per SPEC §7.3. ~80-120 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/overflow_503/envoy-go.yaml` | NEW | Scenario (b) envoy-go-side config — mirrors the upstream. ~80-120 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/overflow_503/expectations.yaml` | NEW | Scenario (b) — **partial cross-side byte-exact per AMEND-6 + §21.P-D2 RATIFIED**. Request 1 → 200 OK; Request 2 → 503 + body `"reached concurrency limit"` (25 bytes verbatim) + `content-type: text/plain` + `content-length: 25` byte-pinned against BOTH reference Envoy v1.37.2 + envoy-go. ~60-90 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/overflow_503/README.md` | NEW | Scenario narrative explaining the 2-concurrent-slow-requests deterministic trap + the cross-side byte-exact promotion per AMEND-6. ~50-100 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/stat_surface/envoy.yaml` | NEW | Scenario (c) reference Envoy config — single-listener HCM with adaptive_concurrency filter + default Gradient-1 config + admin endpoint exposed. ~80-120 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/stat_surface/envoy-go.yaml` | NEW | Scenario (c) envoy-go-side config. ~80-120 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/stat_surface/expectations.yaml` | NEW | Scenario (c) — REFERENCE-LESS subject-only structural; admin `/stats` exposes the full 7-name surface with the expected starting values (concurrency_limit at minConcurrency; gradient gauge present; min_rtt_calculation_active = 1 during initial window). ~30-60 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/stat_surface/README.md` | NEW | Scenario narrative. ~40-80 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/pass_through_when_disabled/envoy.yaml` | NEW | Scenario (d) reference Envoy config — single-listener HCM with adaptive_concurrency filter where `enabled` field is absent (or `enabled.default_enabled=false`) per AMEND-4 default-OFF semantics. ~80-120 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/pass_through_when_disabled/envoy-go.yaml` | NEW | Scenario (d) envoy-go-side config. ~80-120 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/pass_through_when_disabled/expectations.yaml` | NEW | Scenario (d) — REFERENCE-LESS subject-only structural; all requests pass through; NO 503; `rq_blocked` counter stays at 0. ~30-60 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/pass_through_when_disabled/README.md` | NEW | Scenario narrative. ~40-80 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/inputs/driver.go` | NEW | Test-driver invoking the 4-scenario matrix against both reference Envoy + envoy-go; for scenario (b) issues 2 concurrent slow requests (the second dispatched after the first byte of request 1 is observed in the backend); asserts byte-exact wire shape per per-scenario `expectations.yaml`. ~250-400 LoC. |
| `test/fixtures/0025-http-adaptive-concurrency/README.md` | NEW | Top-level fixture-directory README — scenario matrix narrative; cross-references SPEC §7.1; documents the single-listener topology per SPEC §7.3 + planner-time D13. ~80-150 LoC. |
| `docs/envoy-go/DECISIONS.md` | MODIFY | 2 ADR §Decision + §Consequences bodies anchored at IMPL Tasks (ADR-0186 at Task 3 + ADR-0187 at Task 2). 1 IN-PLACE §Decision AMENDMENT body (ADR-0059) at Task 4. Cross-references intact per SPEC §15 item 15. NO new ADR numbers consumed at IMPL under D8 hypothesis (next-free stays ADR-0188). |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY | §13 7-edit bundle per SPEC §13 (NEW `### envoy.filters.http.adaptive_concurrency` subsection + stat-table 92→99 extension + ADR-0059 §Decision AMENDMENT cross-reference + 2 envoy-go-strict departure records + NEW `### Phase 21 forward-pointer notes` subsection + Per-route canonical patterns cross-reference table caption update). Lands at Task 13 (atomic landing per ADR-0052). |
| `docs/envoy-go/ROADMAP.md` | MODIFY | row 21 per-cell IMPL-done annotation + status flips `in-progress → done` at Task 14. |
| `docs/envoy-go/STATE.md` | MODIFY | rewrite-in-place at Task 14 (advance to post-phase-21 state per BOOTSTRAP §4.1 invariant 1). |
| `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` | NEW | append-only task log; ~600-900 LoC across 14 task entries. |
| `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/REVIEW.md` | NEW | Task 14 reviewer artifact per `superpowers:requesting-code-review`. ~300 LoC. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + PLAN-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's 8 RATIFIED-PENDING-IMPL-TIME items before implementation; this PLAN settles those (delegated to the IMPL Task that closes each per SPEC §12 column) plus the planner-time-emerged decisions. The resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced here so the implementer at each Task can act without re-deriving them.

1. **D1 — Task 3 + Task 7 sub-grouping LOCKED at SEPARATE PLAN-TASKS (NEW; surfaces at PLAN-time).** Settle: `controller.go` (Task 3) and `percentile.go` (Task 7) land as SEPARATE PLAN tasks even though the controller consumes the percentile helper at runtime. Rationale: (a) the percentile helper is a self-contained pure-function library with vector tests (no controller-state dependency); (b) separating them enables Task 7 to land in parallel with Task 2 + Task 4 per D12 (parallel-dispatch opportunity); (c) the controller test surface (FAKE-TIME algorithmic-fidelity) is significantly larger than the percentile test surface (vector tests over sorted slices), so keeping them in separate Tasks keeps each Task's test-surface tractable; (d) the controller's `concurrencyUpdateTick()` + `updateMinRTT()` callsites depend on `Quantile` being available — the implementer at Task 3 imports the helper landed at Task 7. NO new ADR fires — this is an IMPL-level decomposition choice. *Anchored: PLAN-time emerge + phase-20 D1 multi-commit precedent inverse (phase-20 used 3 sub-commits within one Task for cross-package work; phase-21 uses 2 separate Tasks for in-package parallel-dispatch).*

2. **D2 — PARSE-REJECT byte-stable error message exact strings LOCKED per SPEC §5.1 + §5.2 + §5.3 + ADR-0080 discipline (NEW; surfaces at PLAN-time).** Settle: all adaptive_concurrency PARSE-REJECT messages use the prefix `"adaptive_concurrency:"` followed by a colon-delimited subject and reason. Reference strings (the implementer's authoritative list at IMPL Task 2):

   - `"adaptive_concurrency: concurrency_controller_config oneof required"` (per §5.1; proto:63-67)
   - `"adaptive_concurrency: concurrency_limit_params required"` (per §5.1; proto:56-57)
   - `"adaptive_concurrency: min_rtt_calc_params required"` (per §5.1; proto:59)
   - `"adaptive_concurrency: concurrency_update_interval must be > 0"` (per §5.1; proto:33-36)
   - `"adaptive_concurrency: max_concurrency_limit must be > 0"` (per §5.1; proto:32)
   - `"adaptive_concurrency: min_concurrency must be > 0"` (per §5.1; proto:46)
   - `"adaptive_concurrency: request_count must be > 0"` (per §5.1; proto:44)
   - `"adaptive_concurrency: min_rtt_calc_params.interval must be >= 1ms"` (per §5.1 + AMEND-1 C3; proto:42)
   - `"adaptive_concurrency: sample_aggregate_percentile must be in [0, 100]"` (per §5.1; proto:54)
   - `"adaptive_concurrency: jitter must be in [0, 100]"` (per §5.1; proto:45)
   - `"adaptive_concurrency: buffer must be in [0, 100]"` (per §5.1; proto:47)
   - `"adaptive_concurrency: enabled.runtime_key is not yet supported; use enabled.default_enabled"` (per §5.2 + ADR-0187)
   - `"adaptive_concurrency: min_rtt_calc_params.fixed_value is not yet supported; use min_rtt_calc_params.interval"` (per §5.2 + §5.3 + AMEND-1 C4 + ADR-0186 §Consequences (d))

   Pattern mirrors ext_authz / ext_proc / oauth2 PARSE-REJECT prefixes; operator-grep-friendly `adaptive_concurrency:` prefix; each message terminated WITHOUT a trailing period. The Task 2 `compiled_config_test.go` table-driven test asserts each byte-exact via `err.Error() == expected`. **NO `RegisterPerRouteValidator`-fired PARSE-REJECT string** — REUSE-by-absence per §5.4 (the per-route placement PARSE-REJECT is via proto-deserialization-time failure, NOT a custom wording). *Anchored: SPEC §5 + ADR-0080 + PLAN-time emerge.*

3. **D3 — Race-test surface roster LOCKED per SPEC §14.2 + §12 item B6 (NEW; surfaces at PLAN-time).** Settle: TWO race-test groups under `go test -race ./...`:

   - **`TestController_ConcurrentForwardingDecision_*`** (controller; lives at `internal/filter/http/adaptive_concurrency/controller_test.go`; lands at Task 3) — 3-5 tests covering: N concurrent forwarders against limit=K via `forwardingDecision()` CAS loop; verify exactly K succeed + N-K block; verify arrival-order determinism (the CAS-loser observes `current_outstanding=K` + falls through to Block deterministically per SPEC §4.1 + `gradient_controller.cc:209-233`); verify no deadlock under N=1000 concurrent forwarders. SPEC §12 item B6 closes here.
   - **`TestController_FAKE_TIME_TimerOrdering_*`** (clock + controller cross-cut; lives at `clock_test.go`; lands at Task 3 cross-cuts Task 9) — 2-3 tests covering: `fakeClock.Advance(d)` fires multiple timers whose deadlines fall within `[c.now, c.now+d]` in deadline-asc order (deterministic per D9); concurrent `Advance()` calls from N goroutines (NOT a supported usage — test asserts the documented single-caller discipline holds; not a race test per se but a determinism-under-misuse test).

   Cumulative race-test surface: 5-8 tests across 2 groups; ALL clean under `-race` at Gate C per §14.2 + §14.5. **NO `*sdsfile.Watcher` analog** — adaptive_concurrency has no SDS surface (unlike phase-20). **NO `TestRefreshTokenRotation_Concurrent_*` analog** — adaptive_concurrency has no async token rotation (unlike phase-20). *Anchored: SPEC §14.2 + ADR-0186 + PLAN-time emerge.*

4. **D4 — Cross-package regression-test command shape LOCKED at single test pattern (NEW; surfaces at PLAN-time).** Settle: after Task 4 (`stats.go` materialization + ADR-0059 §Decision AMENDMENT body lands) the implementer runs `go test -count=1 -race ./internal/stats/...` to verify the existing `internal/stats/` test surface stays GREEN post-AMENDMENT (the AMENDMENT is comment-only on `gauge.go` + a new `conv.go` helper file; expected zero regression). At Task 14 Gate D the full regression `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-5])'` runs all 27 fixtures (the 26 pre-existing — 0000-0024 + 0007a/b sub-fixtures — plus the new 0025). Per SPEC §12 item C8 expected outcome: zero regression. *Anchored: SPEC §12 item C8 + SPEC §7.4 + PLAN-time emerge.*

5. **D5 — Stat-name compile-time guard pattern LOCKED at constant-declaration + table-driven assertion (NEW; surfaces at PLAN-time confirming).** Settle: stat names declared as package-level `const` declarations in `stats.go` (one constant per stat — `const statNameRqBlocked = "rq_blocked"`; `const statNameConcurrencyLimit = "concurrency_limit"`; etc.); `newFilterStats(reg, hcmPrefix)` reads the constants directly when registering each (no string-literal duplication). A table-driven `TestStatNames_Equal_*` test in `adaptive_concurrency_test.go` asserts the 7 constants byte-exact against the wire-expected names (mirrors phase-17 jwt_authn + phase-18.x ext_authz + phase-19.x ext_proc + phase-20 oauth2 precedent). The "compile-time" guard is the constant declaration itself: any drift between the constant and a string literal at the registration site fails the build via the constant-pointer convention. *Anchored: SPEC §6.6 + ADR-0143 SN2-reuse + phase-17/18.2/19.x/20 precedent + PLAN-time emerge.*

6. **D6 — Fuzzer corpus seed roster for `FuzzAdaptiveConcurrencyConfigParse` LOCKED per SPEC §6.7 + §14.3 (NEW; surfaces at PLAN-time).** Settle: corpus seeds at `internal/filter/http/adaptive_concurrency/testdata/fuzz/FuzzAdaptiveConcurrencyConfigParse/` covering:

   - Valid full Gradient-1 config (all sub-blocks present + valid; 1 seed)
   - Each PARSE-REJECT arm per §5.1 (11 RATIFIED-PGV arms × 1-2 valid-edge-case neighbor variants ≈ 14 seeds)
   - Each envoy-go-strict arm per §5.2 (2 arms × 1-2 variants ≈ 4 seeds — `enabled.runtime_key` non-empty; `min_rtt_calc_params.fixed_value` set)
   - Empty config (proto-zero `AdaptiveConcurrency`; 1 seed)
   - Oneof-absent (`concurrency_controller_config` field unset; 1 seed)
   - Nested-message-missing (`concurrency_limit_params` present + `min_rtt_calc_params` absent or vice-versa; 2 seeds)
   - Boundary values (`concurrency_update_interval = 1ns` minimum positive; `max_concurrency_limit = math.MaxUint32`; `sample_aggregate_percentile = 0.0` + `= 100.0` boundaries; ≈ 5 seeds)
   - Default-applied (`enabled` absent → filter OFF per AMEND-4; 1 seed)

   Total corpus floor: ~29 seeds. Must-never-panic across `buildCompiledConfig`. Clean at 30s per seed. *Anchored: SPEC §6.7 + §14.3 + PLAN-time emerge.*

7. **D7 — Boot-registration position LOCKED at line-125 between router and bandwidthlimit per SPEC §3.4 (NEW; surfaces at PLAN-time confirming).** Settle: `cmd/envoy-go/main.go` gains the `httpReg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)` call at line 125 alphabetically between `router` (line 124 — special-case alphabetical-first per existing convention) and `bandwidthlimit` (which shifts from 125 to 126) per ADR-0100 §2.2. The 16th `httpReg.Register` call after phase 20's 15. Per ADR-0072 + ADR-0100 §2.2 — registration order does not affect runtime behavior; stylistic discipline only. The Go package identifier `adaptive_concurrency` (underscored per ADR-0114 stylistic license; matches `header_mutation` precedent) sorts alphabetically before `bandwidthlimit` (`a` < `b`). **NO `RegisterPerRouteValidator` call** — REUSE-by-absence per §5.4 (FOURTH CONSECUTIVE §9 row). *Anchored: SPEC §3.4 + ADR-0100 §2.2 + ADR-0114 + PLAN-time emerge.*

8. **D8 — ADR-0044 escape-valve disposition: PLAN-time HYPOTHESIS that NO additional ADR fires at phase-21 IMPL (NEW; surfaces at PLAN-time).** Per the SPEC-time closure of all 5 §21.P pins (1 RATIFIED + 3 PARTIAL + 1 RESOLVED-to-IN-PLACE-AMENDMENT) — the most-likely escape-valve surfaces are REMOVED at SPEC time per BRAINSTORM §11 lesson (h) + SPEC §10 C. PLAN's strong hypothesis: NO additional ADR fires at phase-21 IMPL — next-free ADR-0188 stays unconsumed at phase-21 phase-done; STRENGTHENED two-slot buffer with ADR-0189 also UNCONSUMED per SPEC §10 D D11-hypothesis-STRENGTHENED note. The remaining possible IMPL surfaces are: (i) sorted-slice quantile edge-cases at sample-window boundaries (samples count = 1 edge case; quantile of empty-slice edge case) — low-probability per `percentile_test.go` Group D10 coverage; if a delta surfaces, ADR-0186 §Decision is AMENDED in-place at Task 7 per ADR-0044 (NO new ADR); (ii) fakeClock determinism race-test edge-cases when multiple timers expire same tick — low-probability per `clock_test.go` Group D9 deterministic-ordering tests; if a delta surfaces, ADR-0186 §Decision is AMENDED in-place at Task 3; (iii) ADR-0059 §Decision AMENDMENT scope-creep (e.g., whether `Format()` int64-text path needs per-class-divisor `# HELP` emission) — low-probability per the comment-only `gauge.go` extension; if a delta surfaces, ADR-0059 §Decision is AMENDED-AGAIN-in-place at Task 4 (NO new ADR). If at IMPL time a surface DOES warrant a new ADR (highly unlikely per the SPEC-time scrape closure), it is ADR-0188 + PLAN's D8 hypothesis is recorded as falsified in PROGRESS.md. *Anchored: SPEC §10 D + §10 C escape-valve note + BRAINSTORM §7.4 + PLAN-time emerge.*

9. **D9 — fakeClock test-helper API shape LOCKED per SPEC §3.1 (NEW; surfaces at PLAN-time).** Settle: `fakeClock` struct at `clock_test.go` exposes: `NewFakeClock(start time.Time) *fakeClock`; `(c *fakeClock) Now() time.Time` returns `c.now`; `(c *fakeClock) AfterFunc(d time.Duration, fn func()) Stop` returns a `*fakeTimer` registered in the heap; `(c *fakeClock) Advance(d time.Duration)` advances `c.now += d` + synchronously fires all timers whose deadline ≤ `c.now` (post-advance) in deadline-ascending order (deterministic per D3 race-test group). The `*fakeTimer` exposes `(t *fakeTimer) Stop() bool` (matches `time.Timer.Stop` semantics). Internal state: `now time.Time`; `timers []*fakeTimer` (sorted heap by deadline); `mu sync.Mutex` (single-caller discipline — documented; the fakeClock is NOT designed for concurrent `Advance()` calls). Tests at `clock_test.go` cover the deterministic-ordering invariant + the Stop-before-fire invariant + the single-caller discipline. ~80-150 LoC. *Anchored: SPEC §3.1 + ADR-0186 §Decision + PLAN-time emerge.*

10. **D10 — Sorted-slice quantile edge-case enumeration LOCKED at `percentile_test.go` vector roster (NEW; surfaces at PLAN-time).** Settle: `percentile_test.go` vector tests cover:

    - `TestPercentile_SortedSlice_Empty` — `Quantile(nil, 0.5)` returns `0` (no panic; documented edge case)
    - `TestPercentile_SortedSlice_SingleSample` — `Quantile([]time.Duration{1*time.Millisecond}, 0.5)` returns `1ms`
    - `TestPercentile_SortedSlice_P50_KnownSet` — vector test against a known sample set; `Quantile([]{1, 2, 3, 4, 5}*time.Millisecond, 0.5)` returns `3ms` (middle element via the `int(p * float64(len-1))` formula)
    - `TestPercentile_SortedSlice_P0_ReturnsMin` — p=0.0 returns sorted[0]
    - `TestPercentile_SortedSlice_P1_ReturnsMax` — p=1.0 returns sorted[len-1]
    - `TestPercentile_SortedSlice_PNegative_ClampsToZero` — p=-0.5 clamped to 0.0; returns sorted[0]
    - `TestPercentile_SortedSlice_PGreaterThanOne_ClampsToOne` — p=1.5 clamped to 1.0; returns sorted[len-1]
    - `TestPercentile_SortedSlice_UnsortedInput_DoesNotMutate` — caller's slice is NOT mutated (the helper copies before sort)

    SPEC §12 item B5 closes here. ~80-120 LoC. *Anchored: SPEC §6.8 + SPEC §12 item B5 + BRAINSTORM §8 item 4 + PLAN-time emerge.*

11. **D11 — `OnDestroy` token-release LOCKED at filter-glue level (NEW; surfaces at PLAN-time).** Settle: per the existing HCM filter framework, a per-stream filter's `OnDestroy()` callback fires on any termination path (full encode-complete OR stream-reset OR client-disconnect OR HCM-side abort). The adaptive_concurrency filter's `OnDestroy()` must call `f.controller.releaseInFlight()` if `f.acquired == true` to prevent token leak (a leaked token would permanently consume one slot from `numRqOutstanding`, lowering effective concurrency capacity for the controller's lifetime). The symmetric pair: `DecodeHeaders` sets `f.acquired = true` on Forward; `OnEncodeComplete` clears `f.acquired = false` after `releaseInFlight()`; `OnDestroy` clears `f.acquired = false` after `releaseInFlight()` only if still acquired. Tests at `controller_test.go` cover the leak-prevention invariant via a `TestFilter_OnDestroy_ReleasesAcquiredToken_*` test (~2 rows: encoded-then-destroyed; reset-mid-decode). *Anchored: PLAN-time emerge + SPEC §6.5.*

12. **D12 — Task graph parallelization LOCKED per planner-time emerge (NEW).** Settle: Tasks 2 + 4 + 7 can run in PARALLEL after Task 1 lands (independent surfaces; all depend on Task 1 for PROGRESS log but NOT on each other) — `compiled_config.go` (Task 2; depends only on Task 1) + `internal/stats/conv.go` + `stats.go` (Task 4; depends only on Task 1) + `percentile.go` (Task 7; depends only on Task 1). Task 3 (`controller.go`) depends on Task 2 (`compiledConfig` struct) + Task 4 (`filterStats` struct) + Task 7 (`Quantile` helper). Tasks 5 + 6 + 8 depend sequentially on Task 3. Task 9 (full filter integration + boot-registration) depends on all of Tasks 2-8. Tasks 10-14 are sequential at the tail. **Parallel-dispatch opportunity at Tasks 2+4+7** — three agents can run concurrently on disjoint files. **Sequential bottleneck at Task 3 → Task 5 → Task 6 / Task 9 / Task 10 / Task 11 / Task 12 / Task 13 / Task 14** — the controller materialization + the filter integration + the fixture + the BEHAVIOR_CONTRACT bundle + the six-gate verification are the critical path. The IMPL session per `superpowers:subagent-driven-development` per project memory `feedback_execution_style.md` exploits the parallel opportunity at Tasks 2+4+7. *Anchored: PLAN-time emerge + phase-19.1/20 task-graph precedent.*

13. **D13 — Fixture 0025 listener topology LOCKED at single listener per SPEC §7.3 (NEW; surfaces at PLAN-time confirming).** Settle: 1 HCM listener per SPEC §7.3's single-listener disposition (vs phase-20's 3-listener topology for the per-listener `forward_bearer_token` permutations). The single listener hosts the adaptive_concurrency filter (alphabetical-first in the filter chain) + router terminator. Per-scenario the `envoy.yaml` + `envoy-go.yaml` swap in the scenario-specific config knobs (e.g., scenario (b) sets `max_concurrency_limit=1 + min_concurrency=1 + concurrency_limit_exceeded_status=503`; scenario (d) sets `enabled` absent or `enabled.default_enabled=false`). The backend cluster is a synthetic slow-response upstream for scenario (b) (1-second response latency; the test driver enforces request-1-then-request-2 ordering via observed-first-byte) and a fast-response upstream for (a) + (c) + (d). Per-scenario fixture directories live at `test/fixtures/0025-http-adaptive-concurrency/<scenario>/` per the phase-19.2 + phase-20 scenario-subdirectory pattern. *Anchored: SPEC §7.3 + phase-19.2 fixture-0023 precedent + PLAN-time emerge.*

14. **D14 — Wire-shape byte-confirmation items in SPEC §12 A1-A4 LOCKED at fixture-0025 scenario coverage (NEW; surfaces at PLAN-time).** Settle: each of the 4 wire-shape items from SPEC §12 A closes at Task 10 fixture-0025 scenarios as follows: (A1) 503 body `"reached concurrency limit"` 25-byte byte-exact closes at scenario (b) `overflow_503` cross-side byte-comparison; (A2) 503 `content-type: text/plain` + `content-length: 25` header byte-exact closes at scenario (b) cross-side header-comparison; (A3) `response_code_details "reached_concurrency_limit"` ABSENT-by-config disposition closes at scenario (b) by NOT-byte-pinning the field (envoy-go MVP has no access-log surface; treat as ABSENT-by-config per SPEC §12 item A3); (A4) `min_rtt_calculation_active` Accumulate import-mode divergence acknowledgement closes at Task 13 BEHAVIOR_CONTRACT.md envoy-go-strict departure record (forward-pointer only; no behavioral check). The IMPL captures both reference Envoy AND envoy-go responses at scenario (b); differential harness asserts byte-equivalent on the 503-leg per AMEND-6. *Anchored: SPEC §12 items A1-A4 + PLAN-time emerge.*

15. **D15 — Library-behavioral items in SPEC §12 B5 + B6 + B7 LOCKED at unit-test + race-test coverage (NEW; surfaces at PLAN-time).** Settle: (B5) sorted-slice quantile numeric divergence vs CircllHist closes at Task 7 `percentile_test.go` vector tests per D10 (cross-quantile-against-known-reference; accept ≤ 1 bin-width difference at percentile boundary; envoy-go-strict departure per BRAINSTORM §8 item 4); (B6) CAS-vs-mutex contention behavior at scale closes at Task 3 `controller_test.go` race tests `TestController_ConcurrentForwardingDecision_*` per D3 (no deadlock; arrival-order determinism holds under N=1000 concurrent forwarders); (B7) fakeClock timer-fire determinism under multi-timer same-tick closes at Task 3 `clock_test.go` deterministic-ordering tests + the `TestController_FAKE_TIME_TimerOrdering_*` group per D3. All three items report RATIFIED at Task 14 PROGRESS log. *Anchored: SPEC §12 items B5 + B6 + B7 + PLAN-time emerge.*

16. **D16 — Cross-phase regression matrix item C8 LOCKED per D4 + Task 14 6-gate (NEW; surfaces at PLAN-time confirming).** Settle: SPEC §12 item C8 (cross-package regression matrix for ADR-0059 §Decision AMENDMENT post-AMENDMENT regression) closes at Task 4 regression check (per D4 — `go test -count=1 -race ./internal/stats/...` post-AMENDMENT) + Task 14 Gate C full `go test -race -count=1 ./...` clean across all packages + Gate D full 27-fixture regression run. Expected outcome per SPEC §12 C8: zero regression (AMENDMENT is pure convention-extension; `boolToInt` helper addition + comment-only `gauge.go` cross-reference; no signature change to `*stats.Gauge`). RATIFIED at Task 14 PROGRESS log. *Anchored: SPEC §12 item C8 + D4 + PLAN-time emerge.*

17. **D17 — `concurrencyLimit` atomic-vs-mutex choice LOCKED at atomic.Uint32 (NEW; surfaces at PLAN-time confirming).** Settle: the controller's `concurrencyLimit` field is `atomic.Uint32` (NOT mutex-guarded). Rationale: (a) `forwardingDecision()` reads `concurrencyLimit` once per request on the hot path — atomic load avoids mutex acquisition cost; (b) writes happen only from `updateConcurrencyLimit` (cold path, ~once per `concurrency_update_interval` ≈ 100ms-1s default) and `enterMinRTTSamplingWindow` / `updateMinRTT` — atomic stores are race-safe vs hot-path atomic loads; (c) the read-modify-write semantics needed at the cold path are bundled with `numRqOutstanding` CAS at the hot path — there's no compound-state invariant that requires mutex-bundled R+W on `concurrencyLimit`. Mirrors upstream `gradient_controller.cc:209` which reads `num_rq_outstanding_.load(std::memory_order_relaxed)` lock-free in the hot path. *Anchored: SPEC §4.1 + §6.2 + PLAN-time emerge.*

18. **D18 — `*rand.Rand` seed source LOCKED at fixed-seed-NOT-time-derived (NEW; surfaces at PLAN-time).** Settle: the jitter randomization at `applyJitter()` per SPEC §4.5 + AMEND-2 C2 uses a per-`*gradientController` `*rand.Rand` instance constructed via `rand.New(rand.NewSource(seed))` with `seed = time.Now().UnixNano()` at controller construction. Concurrent callers acquire `controller.mu` before invoking the rand source (the jitter computation lives in `updateMinRTT()` which is under `mu` already). Alternative considered: package-level `rand.Int63n` — REJECTED because the global rand source is shared across all goroutines (Go 1.20+ auto-seeds; per-process determinism is acceptable but explicit per-controller source gives slightly better FAKE-TIME test reproducibility). NO new ADR fires — IMPL-level choice; documented in `controller.go` doc-comments. *Anchored: SPEC §4.5 + AMEND-2 C2 + PLAN-time emerge.*

---

## ADRs introduced/landed by this plan

The phase-21-landing ADRs per SPEC §10 + the 1 IN-PLACE AMENDMENT — **§Context drafts already at the SPEC commit `49ba034`** (re-anchored at SHA-fill follow-up `3f0f768`) per ADR-0044 ADR-on-impl convention; **§Decision + §Consequences land at each ADR's Lands-in-Task at phase-21 IMPL**. The 1 IN-PLACE §Decision AMENDMENT-anticipation paragraph at ADR-0059 anchors at the SPEC commit; **AMENDMENT body lands at IMPL Task 4** per ADR-0044. PLAN's strong hypothesis per D8: **NO conditional impl-time-unanticipated ADR fires at phase-21 IMPL** (next-free ADR-0188 stays unconsumed at phase-21 phase-done; STRENGTHENED two-slot buffer per SPEC §10 D).

| ADR | Subject (phase-21 portion) | Lands-in-Task |
|---|---|---|
| **ADR-0186** | Gradient-1 controller state machine + inline `Clock` seam (NOT framework primitive) + FAKE-TIME differential strategy + sorted-slice percentile aggregation (NOT CircllHist; ≤ 1 bin-width divergence acceptable per BRAINSTORM §8 item 4) + gradient formula `clamp(0.5, min_rtt × (1 + buffer) / sample_rtt, 2.0)` + new-limit `clamp(min_concurrency, currentLimit × gradient + sqrt(currentLimit × gradient), max_concurrency_limit)` + minRTT recalc with `sample_aggregate_percentile`-quantile (NOT MIN per AMEND-2 C1) + jitter additive-to-next-interval-delay (per AMEND-2 C2) + 5-consecutive-min forced-recalc trigger (per AMEND-2 C3) + first-tick semantics (per AMEND-2 C4) + line-cited algorithmic lemmata against `gradient_controller.cc` per §21.P-D3 RATIFIED + `min_rtt_calc_params.fixed_value` PARSE-REJECT (per §Consequences (d)) | Task 3 (controller materialization; cross-references at Task 7 percentile + Task 4 stats) |
| **ADR-0187** | RTDS `enabled.RuntimeFeatureFlag` deferral PARSE-REJECT (static-default honored; `runtime_key != ""` triggers HCM-parse-time PARSE-REJECT with forward-pointer to the future Runtime/RTDS family phase) + `enabled` empty-default OFF semantics (per AMEND-4 — REFUTES BRAINSTORM "absent enabled = ON" claim per `RuntimeFeatureFlag.default_enabled` proto-default `BoolValue{value: false}`) | Task 2 (compiled_config materialization) |

### IN-PLACE §Decision AMENDMENT (per ADR-0044)

| ADR | AMENDMENT scope | Lands-in-Task |
|---|---|---|
| **ADR-0059** | §Decision body gains AMENDMENT paragraph (already anticipated at SPEC commit per ADR-0044) documenting the float-valued-gauge int64 encoding convention per SPEC §3.2 — ns for time-typed (envoy-go-strict departure from upstream's milliseconds); ×1000 for ratio-typed; 0/1 for bool-typed; +20-30 LoC delta (NEW `internal/stats/conv.go` `boolToInt` helper + comment-only `gauge.go` cross-reference); NO signature change to `*stats.Gauge`. AMENDMENT body lands at IMPL Task 4 paired with `stats.go` materialization | Task 4 |

The implementer at each impl-anchor task AUTHORS the ADR §Decision + §Consequences bodies in DECISIONS.md (the §Context drafts are already at the SPEC commit per ADR-0044), includes the ADR in the commit message, and verifies via `grep -nE '^## ADR-0186' docs/envoy-go/DECISIONS.md` returning the expected single match (similarly for ADR-0187) + via `grep -nE 'Amendment \(per phase 21 ADR-0186\)' docs/envoy-go/DECISIONS.md` returning ≥ 1 match within the ADR-0059 §Decision body block.

**NO in-place ADR-0125 amendment required by phase 21** (REUSE-by-absence per §5.4 — FOURTH CONSECUTIVE §9 row after phase 18 + phase 19 + phase 20 to skip; phase 21's REUSE-by-absence is identical-form to phase-20's — there is no per-route surface at all in the proto, so the listener-scoped-only enforcement is itself a proto-deserialization-time PARSE-REJECT discipline rather than a roster-REUSE classification).

**ADR-0044 escape-valve held in reserve per D8** — `ADR-0188` is reserved for any phase-21-IMPL-unanticipated surface; `ADR-0189` is the STRENGTHENED-two-slot-buffer slot per SPEC §10 D. If at IMPL time a surface DOES warrant a new ADR (highly unlikely per the SPEC-time scrape closure of all 5 §21.P pins), it is ADR-0188 + the PLAN's D8 hypothesis is recorded as falsified in PROGRESS.md. If ADR-0186 / ADR-0187 / ADR-0059 require IMPL-time §Decision AMENDMENTs (e.g., sorted-slice quantile edge-case at boundary; fakeClock multi-timer same-tick determinism; ADR-0059 `Format()` per-class-divisor `# HELP` emission), the AMENDMENT lands in-place — NO new ADR number consumed.

---

## Task graph (sequential vs parallelizable)

The IMPL session subagent-dispatches per `superpowers:subagent-driven-development` (project memory `feedback_execution_style.md`). Per-task graph:

- **Task 1** (PROGRESS.md preamble + 15-precondition verification) — sequential prerequisite for everything; sets up the append-only log.
- **Tasks 2, 4, 7** — **PARALLELIZABLE** (independent surfaces; all depend on Task 1 PROGRESS log but NOT on each other per D12):
  - **Task 2** — `compiled_config.go` + PARSE-REJECT roster + ADR-0187 §Decision + §Consequences body.
  - **Task 4** — `internal/stats/conv.go` NEW + `internal/stats/gauge.go` comment extension + `stats.go` + ADR-0059 §Decision AMENDMENT body.
  - **Task 7** — `percentile.go` sorted-slice quantile helper + `percentile_test.go` vector tests (cross-references ADR-0186 §Decision sub-paragraph on the sorted-slice-NOT-CircllHist choice; the full §Decision + §Consequences body anchors at Task 3).
- **Task 3** (`controller.go` + `clock.go` + `controller_test.go` + `clock_test.go` + ADR-0186 §Decision + §Consequences body) — depends on Task 2 (`compiledConfig` struct), Task 4 (`filterStats` struct), Task 7 (`Quantile` helper).
- **Task 5** (`decode_headers.go` + 503-overflow `SendLocalReply` wire shape) — depends on Task 3 (`controller.forwardingDecision`) + Task 2 + Task 4.
- **Task 6** (`encode_complete.go` + sample-recording + `releaseInFlight` + `OnDestroy` token-release per D11) — depends on Task 3 + Task 5 (`filter` struct extends `f.acquired` + `f.entryTime` from Task 5's `decode_headers.go`).
- **Task 8** (`fuzz_test.go` 27th fuzzer + corpus seeds per D6) — depends on Task 2 (`buildCompiledConfig` is what's fuzzed).
- **Task 9** (full filter integration in `adaptive_concurrency.go` + `filter.go` + `doc.go` + boot-registration at `cmd/envoy-go/main.go` + ADR final-state alignment) — depends on Tasks 2-8 (consumes all prior surfaces); produces a fully-functional `api.HTTPFilterFactory` from `New()`.
- **Task 10** (differential fixture `0025-http-adaptive-concurrency` + 4-scenario directories + driver + RATIFIED-PENDING-IMPL-TIME pin closures per D14 + D15) — depends on Task 9 (full filter integration); CLOSES SPEC §12 items A1-A4 + B5 + B6 + B7 (with cross-package C8 closing at Task 4 + Task 14).
- **Task 11** (ADR final-state alignment + DECISIONS.md cross-reference audit) — depends on Tasks 2-10 (ADR-0186 + ADR-0187 + ADR-0059 AMENDMENT bodies all anchored).
- **Task 12** (cross-package regression matrix verification per D4 + D16) — depends on Task 4 (post-AMENDMENT regression check) + Task 10 (full 27-fixture regression run).
- **Task 13** (BEHAVIOR_CONTRACT.md 7-edit bundle per SPEC §13) — depends on Task 10 (some §13 paragraphs reference the fixture-0025 wire-shape closures from Task 10).
- **Task 14** (six-gate phase-done verification A/B/C/D/E/F per §7.4 + STATE.md re-advance + ROADMAP row 21 flip + REVIEW.md authoring per `superpowers:requesting-code-review`) — depends on everything.

**Parallel-dispatch opportunity at Tasks 2+4+7** — three agents can run concurrently on disjoint files. **Sequential bottleneck at Task 3 → Tasks 5 → 6 → 9 → 10 → 11 → 12 → 13 → 14** — the controller materialization + the integration + the fixture + the BEHAVIOR_CONTRACT bundle + the six-gate verification are the critical path. **Task 8** (fuzzer) can run partially in parallel with Tasks 3 + 5 + 6 (depends only on Task 2's `buildCompiledConfig`; file-disjoint from the controller/filter/encode work).

---

## Execution preconditions

Before Task 1 the implementer cold-starts and verifies. **Worktree spawn discipline:** the IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (project memory `feedback_git_worktrees.md`). The expected sequence (executed by the orchestrating session before invoking the IMPL session, OR by the IMPL session at cold-start if standalone):

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-21-http-filter-adaptive-concurrency-impl \
                 -b phase-21-http-filter-adaptive-concurrency-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-21-http-filter-adaptive-concurrency-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md squash-merge commit + its SHA-fill follow-up.

The 15 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-21-http-filter-adaptive-concurrency-impl`. If only a SPEC-stage or PLAN-stage worktree is present, branch a fresh impl worktree from master HEAD per ADR-0003.
2. **Master tail.** `git log --oneline master | head -8` shows the phase-21-PLAN.md squash commit + its SHA-fill follow-up at the head, with the phase-21-SPEC.md squash commit `49ba034` + its SHA-fill follow-up `3f0f768` immediately before. If not, resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.26.2` or newer; `golangci-lint version` reports `1.64.8` (ADR-0009 pin); `docker version` reports both client + server.
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `187` (ADR-0187 — the highest ADR anchored as of master tip per the phase-21 SPEC commit). Higher → another phase landed concurrently; re-verify next-free numbers.
5. **ADR §Context drafts present.** `grep -cE '^## ADR-0186' docs/envoy-go/DECISIONS.md` returns `1` (ADR-0186 §Context already at SPEC commit per ADR-0044). Same for ADR-0187. `grep -nE '^## ADR-0188' docs/envoy-go/DECISIONS.md` returns 0 (ADR-0188 stays unconsumed at phase-21 IMPL under D8 hypothesis).
6. **1 IN-PLACE §Decision AMENDMENT-anticipation paragraph present.** `grep -nE 'Amendment \(per phase 21 ADR-0186\)' docs/envoy-go/DECISIONS.md` returns ≥1 match in the ADR-0059 §Decision body block — confirms the SPEC-time AMENDMENT-anticipation paragraph anchored.
7. **NO ADR-0125 amendment.** Phase 21 lands NO ADR-0125 amendment (REUSE-by-absence per §5.4 — FOURTH CONSECUTIVE §9 row to skip). If a phase-21-specific cross-reference to ADR-0125 lands at DECISIONS.md during IMPL, investigate before proceeding.
8. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/21-http-filter-adaptive-concurrency/SPEC.md` returns `49ba034` (or descendant). If different, re-read SPEC.
9. **PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PLAN.md` returns the PLAN commit's SHA. If earlier than the SPEC, PLAN has been amended — re-read PLAN.
10. **Pristine tree.** `git status --porcelain` returns empty.
11. **Pre-existing suite green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
12. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-4])'` returns every fixture 0000-0024 PASS — the 25 pre-existing fixtures (counting 0007a + 0007b sub-fixtures as separate is 26 by directory count) are the regression baseline. Phase 21 adds the 26th `BackendKind` enum value + the 27th-by-directory fixture (`0025-http-adaptive-concurrency` per Task 10).
13. **Pre-existing fuzzers run clean at 30s.** The 26 fuzzers from phases 02-20 run clean. Phase 21 adds the 27th (`FuzzAdaptiveConcurrencyConfigParse` per Task 8).
14. **Reference Envoy image present.** `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin; unchanged).
15. **Pre-existing `internal/filter/http/adaptive_concurrency/` directory + `internal/stats/conv.go` file do NOT exist.** `test ! -d internal/filter/http/adaptive_concurrency && test ! -f internal/stats/conv.go && echo "ok: phase-21-new-surfaces absent"` returns success.

If all 15 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044, ADR-0186 + ADR-0187 §Context drafts are at the SPEC commit `49ba034` (re-anchored at SHA-fill follow-up `3f0f768`); the IN-PLACE §Decision AMENDMENT-anticipation paragraph at ADR-0059 is at the same commit; ADR-0188 is CONDITIONAL (PLAN hypothesis per D8: it does NOT fire at phase-21 IMPL). The PROGRESS preamble ANTICIPATES the 2 NEW ADR landings + the 1 IN-PLACE AMENDMENT landing (each with its Lands-in-Task anchor reproduced from this PLAN's per-ADR table) and records the 18 planner-time decisions D1-D18.

**Precondition:** worktree exists at `phase-21-http-filter-adaptive-concurrency-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 15 preconditions report green.
**Artifact:** `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (new file).
**Acceptance:** all 15 preconditions report green; PROGRESS.md preamble committed; `git log -1 --format=%H -- docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` above and confirm the expected output.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` with: (a) Preamble summarizing the 15-precondition verification (verbatim command outputs captured); (b) the 2-NEW-ADR + 1-IN-PLACE-AMENDMENT table from `## ADRs introduced/landed by this plan` reproduced verbatim; (c) the 18 planner-time decisions D1-D18 reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) a Task 1 entry slot for the commit-SHA fill-in.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 1: PROGRESS.md preamble + 15-precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
# expect: a 40-char SHA (Task 1 commit)
```

---

## Task 2: NEW `internal/filter/http/adaptive_concurrency/compiled_config.go` + PARSE-REJECT roster + ADR-0187

**Files:**
- Create: `internal/filter/http/adaptive_concurrency/compiled_config.go` (~200-280 LoC)
- Create: `internal/filter/http/adaptive_concurrency/compiled_config_test.go` (~250-350 LoC; `TestBuildCompiledConfig_PARSE_REJECT_*` table-driven tests per SPEC §14.1 + D2)
- Modify: `docs/envoy-go/DECISIONS.md` (~+100 LoC: ADR-0187 §Decision + §Consequences body — EXTENDS the SPEC-commit §Context draft per ADR-0044)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 2 entry)

This task lands the `compiledConfig` struct + the `buildCompiledConfig` parser + the full 13-arm PARSE-REJECT roster per D2 byte-stable wording (11 RATIFIED-from-PGV arms per SPEC §5.1 + 2 envoy-go-strict arms per SPEC §5.2; the `fixed_value` deferral per SPEC §5.3 is the SAME arm as the §5.2 row 2). **Parallelizable with Tasks 4 + 7** per D12 (disjoint files; depends only on Task 1 being landed).

**Precondition:** Task 1 complete.
**Artifact:** `compiled_config.go` with full PARSE-REJECT roster; ADR-0187 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./internal/filter/http/adaptive_concurrency/...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/adaptive_concurrency/...` clean; `go test -count=1 ./internal/filter/http/adaptive_concurrency/... -run 'TestBuildCompiledConfig'` clean (25-30 PARSE-REJECT rows + default-applied rows pass); `grep -cE '^## ADR-0187' docs/envoy-go/DECISIONS.md` returns `1` AND §Decision body non-empty.

- [ ] **Step 1: Write failing tests** in `internal/filter/http/adaptive_concurrency/compiled_config_test.go` per SPEC §14.1 + D2. Table-driven format: each row is `{name string, configMutator func(*adaptive_concurrencyv3.AdaptiveConcurrency), wantErrSubstring string}`. ~20-25 rows covering each of the 13 distinct PARSE-REJECT arms per D2 reference strings + 5-10 default-applied rows per SPEC §6.1 + AMEND-4.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/filter/http/adaptive_concurrency/... 2>&1 | head -20
# Expect: FAIL with "no such package" or similar
```

- [ ] **Step 3: Author `internal/filter/http/adaptive_concurrency/compiled_config.go`** per the File-structure table row above + SPEC §6.1 + ADR-0187 §Context. Includes: `compiledConfig` struct shape per SPEC §6.1 (verbatim); `buildCompiledConfig(typedConfig *anypb.Any) (*compiledConfig, error)` function with each PARSE-REJECT arm per D2 byte-stable wording; default application per SPEC §6.1 + AMEND-4 (enabled default false; concurrencyLimitExceededStatus default 503; sampleAggregatePercentile default 0.5; etc.).

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -count=1 ./internal/filter/http/adaptive_concurrency/... -run 'TestBuildCompiledConfig'
# Expect: PASS — 25-30 PARSE-REJECT rows + 5-10 default-applied rows
```

- [ ] **Step 5: Author ADR-0187 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft at the SPEC commit `49ba034`. §Decision body covers: the `enabled.runtime_key != ""` HCM-parse-time PARSE-REJECT discipline; the `enabled.default_enabled` static-arm honored; the `enabled` empty-default OFF semantics per AMEND-4 (REFUTES BRAINSTORM §2.1's "absent enabled = ON" claim per the `RuntimeFeatureFlag.default_enabled` `BoolValue{value: false}` proto-default); the byte-stable error wording per D2 (`"adaptive_concurrency: enabled.runtime_key is not yet supported; use enabled.default_enabled"`). §Consequences body covers: operators can statically enable/disable via config; runtime feature-flag keying is a forward-pointer to the future Runtime/RTDS family phase; PARSE-REJECT path uses the existing HCM-parse-time framework (REUSE 4 per SPEC §3.3); the absent-enabled-OFF semantics ALIGNS with upstream wire-compat per AMEND-4 empirical scrape.

- [ ] **Step 6: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 7: Append PROGRESS.md Task 2 entry** — record build/test output + `git log -1 --format=%H` for the Task 2 commit.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/adaptive_concurrency/compiled_config.go \
        internal/filter/http/adaptive_concurrency/compiled_config_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 2: compiled_config.go + PARSE-REJECT roster + ADR-0187

Lands compiledConfig + buildCompiledConfig with the 13-arm PARSE-REJECT
roster per D2 byte-stable wording (11 RATIFIED-PGV arms per SPEC §5.1 +
2 envoy-go-strict arms per SPEC §5.2; the fixed_value deferral per
SPEC §5.3 + AMEND-1 C4 is the same arm as §5.2 row 2). Defaults applied
per SPEC §6.1 + AMEND-4 — enabled absent = OFF (RuntimeFeatureFlag.
default_enabled BoolValue{value: false} proto-default; REFUTES BRAINSTORM
§2.1). 20-25 PARSE-REJECT table-driven tests + 5-10 default-applied tests
pass. ADR-0187 §Decision + §Consequences anchored (EXTENDS SPEC-commit
§Context draft per ADR-0044)."
```

---

## Task 3: NEW `internal/filter/http/adaptive_concurrency/controller.go` + `clock.go` + ADR-0186

**Files:**
- Create: `internal/filter/http/adaptive_concurrency/controller.go` (~350-450 LoC)
- Create: `internal/filter/http/adaptive_concurrency/clock.go` (~30-60 LoC)
- Create: `internal/filter/http/adaptive_concurrency/controller_test.go` (~400-550 LoC; Layer A FAKE-TIME algorithmic-fidelity tests per SPEC §14.1 + race tests per D3)
- Create: `internal/filter/http/adaptive_concurrency/clock_test.go` (~80-150 LoC; `fakeClock` test-scope implementation per D9 + determinism tests)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150 LoC: ADR-0186 §Decision + §Consequences body — EXTENDS the SPEC-commit §Context draft per ADR-0044)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 3 entry)

This task lands the Gradient-1 controller state machine + the inline `Clock` seam + the `fakeClock` test-scope step driver + the algorithmic-fidelity test layer. Depends on Task 2 (`compiledConfig` struct), Task 4 (`filterStats` struct), Task 7 (`Quantile` helper).

**Precondition:** Task 2 + Task 4 + Task 7 complete.
**Artifact:** `controller.go` + `clock.go` + algorithmic-fidelity tests; ADR-0186 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./internal/filter/http/adaptive_concurrency/...` clean; `go vet ./...` clean; `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/adaptive_concurrency/...` clean (Layer A — ~10 test families pass); `go test -race -count=1 ./internal/filter/http/adaptive_concurrency/...` clean (race tests `TestController_ConcurrentForwardingDecision_*` + `TestController_FAKE_TIME_TimerOrdering_*` pass per D3); `grep -cE '^## ADR-0186' docs/envoy-go/DECISIONS.md` returns `1` AND §Decision body non-empty.

- [ ] **Step 1: Author `internal/filter/http/adaptive_concurrency/clock.go`** per SPEC §3.1 + §6.3 — `Clock` interface (`Now() time.Time` + `AfterFunc(d time.Duration, fn func()) Stop`); `Stop` interface (`Stop() bool`); `defaultClock` struct + methods wrapping `time.Now` + `time.AfterFunc`; `timerStop{t *time.Timer}` with `Stop() bool` returning `t.Stop()`.

- [ ] **Step 2: Author `internal/filter/http/adaptive_concurrency/clock_test.go`** per D9 — `fakeClock` struct with `now time.Time` + sorted-by-deadline `timers []*fakeTimer` + `mu sync.Mutex`; `NewFakeClock(start time.Time) *fakeClock`; `(c *fakeClock) Now() time.Time` returns `c.now`; `(c *fakeClock) AfterFunc(d, fn) Stop` registers a `*fakeTimer` in the sorted heap; `(c *fakeClock) Advance(d time.Duration)` advances + fires expired timers in deadline-asc order; `(t *fakeTimer) Stop() bool` matches `time.Timer.Stop` semantics. Tests: `TestFakeClock_Now_*`; `TestFakeClock_AfterFunc_FiresAtDeadline`; `TestFakeClock_Advance_FiresInDeadlineOrder` (the D9 determinism test); `TestFakeClock_StopBeforeFire_PreventsCallback`.

- [ ] **Step 3: Write failing tests** in `internal/filter/http/adaptive_concurrency/controller_test.go` per SPEC §14.1 Layer A:
  - `TestController_FAKE_TIME_FirstTickSemantics` — per AMEND-2 C4: controller immediately enters minRTT window at construction; concurrency-update tick callback short-circuits while in-window; first gradient computation only after first minRTT recalc.
  - `TestController_FAKE_TIME_GradientFormula_*` — per SPEC §4.3: vector tests for `computeGradient` under varying sample-RTT / min-RTT / buffer combinations; verify clamp at 0.5 + 2.0; verify buffer application × (1 + bufferPct).
  - `TestController_FAKE_TIME_NewLimitCalculation_*` — per SPEC §4.4: vector tests for `calculateNewLimit`; verify sqrt-burst-headroom; verify double-clamp at minConcurrency + maxConcurrencyLimit.
  - `TestController_FAKE_TIME_MinRTTRecalcWindow_*` — per SPEC §4.5 + AMEND-2 C1: verify minRTT recalc takes the `sample_aggregate_percentile`-quantile (default p50) of recalc-window samples, NOT MIN. Vector tests with crafted sample sets.
  - `TestController_FAKE_TIME_JitterApplication_*` — per AMEND-2 C2: verify jitter is additive `[0, interval × jitter_pct)` to the next-interval delay (not to the recalc-window length).
  - `TestController_FAKE_TIME_FiveConsecutiveMinForcedRecalc` — per AMEND-2 C3: verify the 5-consecutive-min forced-recalc trigger fires `minRTTCalcTimer` at 0ms.
  - `TestController_ConcurrentForwardingDecision_*` — race tests per D3: N concurrent forwarders against limit=K; verify exactly K succeed + N-K block; arrival-order determinism; no deadlock at N=1000.
  - `TestController_503_BodyAndHeaders_ByteExact` — per AMEND-6 + §12 items A1+A2: verify the 503 body `"reached concurrency limit"` (25 bytes) + `content-type: text/plain` + `content-length: 25` against the subject-only emission (via a stub `api.DecoderFilterCallbacks` recording `SendLocalReply` args).
  - `TestFilter_OnDestroy_ReleasesAcquiredToken_*` per D11 — verify `OnDestroy` releases the in-flight token if `f.acquired == true` (encoded-then-destroyed; reset-mid-decode).

- [ ] **Step 4: Run tests to verify they fail.**

- [ ] **Step 5: Author `internal/filter/http/adaptive_concurrency/controller.go`** per the File-structure table row above + SPEC §4 + §6.2 + ADR-0186 §Context. Includes: `gradientController` struct per SPEC §6.2; `newGradientController(cfg, stats, clock)` per AMEND-2 C4 first-tick semantics; `forwardingDecision()` per SPEC §4.1 (hot-path lock-free CAS loop on `numRqOutstanding`); `recordLatencySample(rtt)` per SPEC §4.2; `releaseInFlight()` (atomic decrement); `concurrencyUpdateTick()` callback per SPEC §4.2; `enterMinRTTSamplingWindow()` per SPEC §4.5 (a)-(e); `updateMinRTT()` per SPEC §4.5 (f)-(j); `calculateNewLimit(gradient)` per SPEC §4.4; `computeGradient(minRTT, sampleRTT, bufferPct)` per SPEC §4.3; `updateConcurrencyLimit(newLimit)` with 5-consecutive-min bookkeeping per AMEND-2 C3; `applyJitter(interval, jitterPct)` per AMEND-2 C2 (using per-controller `*rand.Rand` per D18).

- [ ] **Step 6: Run tests to verify they pass.**

```bash
go test -count=1 ./internal/filter/http/adaptive_concurrency/...
# Expect: PASS — Layer A ~10 test families
go test -race -count=1 ./internal/filter/http/adaptive_concurrency/...
# Expect: PASS — race tests clean
```

- [ ] **Step 7: Author ADR-0186 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft at the SPEC commit `49ba034`. §Decision body covers: the Gradient-1 controller state machine shape (hot-path atomics + cold-path mutex); the inline `Clock` seam (consumer count 1; NOT framework primitive per phase-17 EXTRACT-NOW-only-when-trigger-fires lesson); the FAKE-TIME differential strategy (subject-only step-driven `fakeClock`); the sorted-slice percentile aggregation choice (NOT CircllHist; ≤ 1 bin-width divergence acceptable per BRAINSTORM §8 item 4); the gradient formula `clamp(0.5, min_rtt × (1 + buffer) / sample_rtt, 2.0)` per `gradient_controller.cc:190-192`; the new-limit `clamp(min_concurrency, currentLimit × gradient + sqrt(currentLimit × gradient), max_concurrency_limit)` per `cc:198-206`; the minRTT recalc with `sample_aggregate_percentile`-quantile NOT MIN per AMEND-2 C1 + `cc:176-182`; the jitter additive-to-next-interval-delay per AMEND-2 C2 + `cc:152-160`; the 5-consecutive-min forced-recalc trigger per AMEND-2 C3 + `cc:281-283`; the first-tick-immediate-window-entry semantics per AMEND-2 C4 + `cc:55-92`; the `min_rtt_calc_params.fixed_value` PARSE-REJECT per §Consequences (d). §Consequences body covers: ZERO new framework primitive (LEANEST framework-delta §9 row to date per BRAINSTORM §1.7); cross-phase forward-pointer to a future EXTRACT-NOW trigger when a second timer-driven filter materializes (admission_control, global rate limit, or similar §9 row); the 7-name stat surface (1 counter + 6 gauges per AMEND-3); the partial cross-side byte-exact 503-overflow leg per AMEND-6; the sorted-slice quantile edge-case coverage per D10; the fakeClock determinism-under-multi-timer-same-tick discipline per D9.

- [ ] **Step 8: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 9: Append PROGRESS.md Task 3 entry** — record build/test output + `git log -1 --format=%H` for the Task 3 commit + the D3 race-test surface RATIFIED status (SPEC §12 items B5 + B6 + B7 closing).

- [ ] **Step 10: Commit**

```bash
git add internal/filter/http/adaptive_concurrency/controller.go \
        internal/filter/http/adaptive_concurrency/clock.go \
        internal/filter/http/adaptive_concurrency/controller_test.go \
        internal/filter/http/adaptive_concurrency/clock_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 3: controller.go + clock.go + ADR-0186 (Gradient-1 state machine)

Lands the per-HCM-instance Gradient-1 controller state machine per SPEC §4
+ ADR-0186: hot-path lock-free CAS on numRqOutstanding via atomic.Uint32;
cold-path sync.Mutex over latencySamples / minRTTSamples / deferredLimitValue
/ consecutiveMinConcurrencySet; periodic concurrency-update tick;
periodic minRTT-recalc trigger; gradient formula clamp(0.5, min_rtt ×
(1 + buffer) / sample_rtt, 2.0) per gradient_controller.cc:190-192;
new-limit per cc:198-206 with sqrt-burst-headroom + double-clamp;
minRTT recalc via sample_aggregate_percentile-quantile NOT MIN per
AMEND-2 C1 + cc:176-182; jitter additive-to-next-interval-delay per
AMEND-2 C2 + cc:152-160; 5-consecutive-min forced-recalc trigger per
AMEND-2 C3 + cc:281-283; first-tick-immediate-window-entry semantics
per AMEND-2 C4 + cc:55-92.

Inline Clock seam at clock.go (consumer count 1; NOT framework primitive
per phase-17 EXTRACT-NOW-only-when-trigger-fires lesson); fakeClock
step-driven test-scope implementation at clock_test.go per D9.

Layer A FAKE-TIME algorithmic-fidelity tests (10 families) + race tests
TestController_ConcurrentForwardingDecision_* per D3 + D11 OnDestroy
token-release tests all pass under -race.

CLOSES SPEC §12 items B5 + B6 + B7 RATIFIED-PENDING-IMPL-TIME at
PROGRESS log per D3 + D15. ADR-0186 §Decision + §Consequences anchored."
```

---

## Task 4: NEW `internal/stats/conv.go` + comment-only `internal/stats/gauge.go` + NEW `internal/filter/http/adaptive_concurrency/stats.go` + ADR-0059 §Decision AMENDMENT body

**Files:**
- Create: `internal/stats/conv.go` (~10-15 LoC; `boolToInt(b bool) int64` helper)
- Modify: `internal/stats/gauge.go` (~+5-10 LoC; comment-only doc-extension cross-referencing ADR-0059 §Decision AMENDMENT)
- Create: `internal/filter/http/adaptive_concurrency/stats.go` (~80-120 LoC; `filterStats` 7-name roster + `newFilterStats` constructor + stat-name `const` declarations per D5)
- Modify: `docs/envoy-go/DECISIONS.md` (~+50-80 LoC: ADR-0059 §Decision AMENDMENT body — anchored at this Task per ADR-0044 in-place edit discipline)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 4 entry)

This task lands the `filterStats` 7-name roster + the ADR-0059 §Decision AMENDMENT body (float-valued-gauge int64 encoding convention) at the same commit per ADR-0044 in-place edit discipline. **Parallelizable with Tasks 2 + 7** per D12.

**Precondition:** Task 1 complete.
**Artifact:** `internal/stats/conv.go` NEW + `internal/stats/gauge.go` comment extension + `internal/filter/http/adaptive_concurrency/stats.go` + ADR-0059 §Decision AMENDMENT body anchored.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run` clean; `go test -count=1 -race ./internal/stats/...` clean (per D4 cross-package regression check); `grep -nE 'Amendment \(per phase 21 ADR-0186\)' docs/envoy-go/DECISIONS.md` returns ≥1 match — AND the `ANTICIPATION` marker is removed/replaced with the final AMENDMENT body wording per ADR-0044 in-place edit discipline.

- [ ] **Step 1: Author `internal/stats/conv.go`** — ~10-15 LoC NEW small file containing `package stats` + `boolToInt(b bool) int64` helper (returns 1 if true, 0 otherwise). Doc-comment cross-references the ADR-0059 §Decision AMENDMENT.

- [ ] **Step 2: Modify `internal/stats/gauge.go`** — add a doc-comment paragraph (file-level OR `Gauge.Set` callsite) cross-referencing the ADR-0059 §Decision AMENDMENT for the float-valued-gauge int64 encoding convention (ns for time-typed; ×1000 for ratio-typed; 0/1 for bool-typed via `boolToInt` helper). **NO signature change** to `*stats.Gauge`.

- [ ] **Step 3: Author `internal/filter/http/adaptive_concurrency/stats.go`** per the File-structure table row above + SPEC §6.6 + AMEND-3 + ADR-0059 §Decision AMENDMENT consumer. Includes: 7 package-level `const` declarations for the stat names per D5 (`const statNameRqBlocked = "rq_blocked"`; `const statNameConcurrencyLimit = "concurrency_limit"`; `const statNameGradient = "gradient"`; `const statNameBurstQueueSize = "burst_queue_size"`; `const statNameSampleRTTMsecs = "sample_rtt_msecs"`; `const statNameMinRTTMsecs = "min_rtt_msecs"`; `const statNameMinRTTCalculationActive = "min_rtt_calculation_active"`); `filterStats` struct with 1 counter + 6 gauges; `newFilterStats(reg *stats.Registry, hcmPrefix string) *filterStats` constructor using `reg.NewCounter` + `reg.NewGauge` under the prefix `hcmPrefix + ".adaptive_concurrency.gradient_controller."` per AMEND-3 C2.

- [ ] **Step 4: Run cross-package regression check per D4 + D16**

```bash
go test -count=1 -race ./internal/stats/...
# Expect: PASS — existing test coverage preserved; SPEC §12 item C8 RATIFIED
```

- [ ] **Step 5: Author ADR-0059 §Decision AMENDMENT body in DECISIONS.md** — REPLACE the existing AMENDMENT-anticipation paragraph at the SPEC commit `49ba034` with the final AMENDMENT body per ADR-0044 in-place edit discipline. The AMENDMENT paragraph anchors the float-valued-gauge int64 encoding convention per SPEC §3.2 — Time-typed gauges encode as int64 nanoseconds direct via `Gauge.Set(rtt.Nanoseconds())` (envoy-go-strict departure from upstream milliseconds; stat NAMES preserve byte-exact `sample_rtt_msecs` / `min_rtt_msecs`; per-metric `# HELP` text disambiguates the unit); Ratio-typed gauges encode as int64 ×1000 via `Gauge.Set(int64(gradient * 1000))` (matches upstream's `gradient × 1000` integer-millis convention; gives 3 decimal places over the bounded `[0.5, 2.0]` domain); Bool-typed gauges encode as int64 0/1 via `Gauge.Set(boolToInt(b))` (sibling helper at `internal/stats/conv.go` per ADR-0044). The AMENDMENT is clearly dated 2026-05-18 + cross-referenced to phase 21 + ADR-0186.

- [ ] **Step 6: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 7: Append PROGRESS.md Task 4 entry — RATIFIES SPEC §12 item C8 (cross-package regression matrix for ADR-0059 §Decision AMENDMENT) per D16.**

- [ ] **Step 8: Commit**

```bash
git add internal/stats/conv.go \
        internal/stats/gauge.go \
        internal/filter/http/adaptive_concurrency/stats.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 4: stats.go + ADR-0059 §Decision AMENDMENT body (float-valued-gauge convention)

Per ADR-0044 in-place edit discipline + the phase-21 SPEC §3.2. Lands the
ADR-0059 §Decision AMENDMENT body (REPLACES the SPEC-commit ANTICIPATION
paragraph) documenting the float-valued-gauge int64 encoding convention:
ns for time-typed (envoy-go-strict departure from upstream milliseconds;
stat NAMES preserve byte-exact upstream sample_rtt_msecs / min_rtt_msecs);
×1000 for ratio-typed (gradient bounded [0.5, 2.0]; matches upstream
integer-millis convention); 0/1 for bool-typed via NEW boolToInt helper
at internal/stats/conv.go.

NO signature change to *stats.Gauge — gauge.go doc-comment extension only.

internal/filter/http/adaptive_concurrency/stats.go lands the 7-name
filterStats roster (1 counter rq_blocked + 6 gauges concurrency_limit /
gradient / burst_queue_size / sample_rtt_msecs / min_rtt_msecs /
min_rtt_calculation_active) under the http.<HCM_stat_prefix>.adaptive_
concurrency.gradient_controller.* prefix per AMEND-3 C2. Stat-name
compile-time guards via const declarations per D5.

CLOSES SPEC §12 item C8 RATIFIED-PENDING-IMPL-TIME (cross-package
regression for ADR-0059 AMENDMENT) — internal/stats/ tests GREEN
post-AMENDMENT per D16."
```

---

## Task 5: NEW `internal/filter/http/adaptive_concurrency/filter.go` (per-stream struct) + `decode_headers.go` + 503-overflow wire shape

**Files:**
- Create: `internal/filter/http/adaptive_concurrency/filter.go` (~60-100 LoC; per-stream struct + field declarations; `DecodeHeaders` + `OnEncodeComplete` + `OnDestroy` method DECLARATIONS only — bodies for `OnEncodeComplete` + `OnDestroy` deferred to Task 6 as `// TODO Task 6` stubs that compile-build-pass via empty bodies)
- Create: `internal/filter/http/adaptive_concurrency/decode_headers.go` (~50-80 LoC)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 5 entry)

This task lands the per-stream `filter` struct + the `DecodeHeaders` dispatch + the 503-overflow `SendLocalReply` wire shape per AMEND-6 byte-pinned bytes. The `filter` struct is established here (NOT Task 6) so that Task 5's commit is self-buildable per the receiving-code-review observation; Task 6 fills in the `OnEncodeComplete` body + the `OnDestroy` token-release per D11.

**Precondition:** Task 3 (controller `forwardingDecision`) + Task 2 (`compiledConfig`) + Task 4 (`filterStats`) complete.
**Artifact:** `filter.go` per-stream struct skeleton + `decode_headers.go` with full dispatch.
**Acceptance:** `go build ./...` clean (Task 5 produces a self-buildable commit); `go vet ./...` clean; `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/adaptive_concurrency/...` clean (the `DecodeHeaders` dispatch unit tests from Task 3's `controller_test.go` cover this surface end-to-end via test-side stub callbacks).

- [ ] **Step 1: Author `internal/filter/http/adaptive_concurrency/filter.go`** per the File-structure table row above + SPEC §6.3 + planner-time D11. Includes: per-stream `filter` struct with fields `cc *compiledConfig`; `controller *gradientController`; `clock Clock`; `cb api.DecoderFilterCallbacks`; `entryTime time.Time`; `acquired bool`; `parentCtx context.Context`. Method declarations: `DecodeHeaders` (body lives in `decode_headers.go` — same package); `OnEncodeComplete` (stub `// TODO Task 6` returning nothing; just enough to satisfy any interface assertion landing at Task 9); `OnDestroy` (stub `// TODO Task 6` returning nothing). The shared `*gradientController` instance is hoisted to the factory level (one per `compiledConfig` instance); each per-stream `*filter` instance captures the shared pointer.

- [ ] **Step 2: Author `internal/filter/http/adaptive_concurrency/decode_headers.go`** per the File-structure table row above + SPEC §6.4 + AMEND-6 byte-pinned wire shape. The `DecodeHeaders` function: (1) if `!f.cc.enabled` return `Continue` (filter disabled per AMEND-4 default-OFF); (2) call `f.controller.forwardingDecision()`; (3) on Block emit `SendLocalReply(int(f.cc.concurrencyLimitExceededStatus), "reached concurrency limit", map[string]string{"content-type": "text/plain"}, nil, "reached_concurrency_limit")`; increment `f.cc.stats.rqBlocked` counter; return `StopAndBuffer`; (4) on Forward set `f.entryTime = f.clock.Now()` + `f.acquired = true`; return `Continue`.

- [ ] **Step 3: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean** (Task 5 is self-buildable per the receiving-code-review observation — the `OnEncodeComplete` + `OnDestroy` stubs satisfy the compiler).

- [ ] **Step 4: Append PROGRESS.md Task 5 entry.**

- [ ] **Step 5: Commit**

```bash
git add internal/filter/http/adaptive_concurrency/filter.go \
        internal/filter/http/adaptive_concurrency/decode_headers.go \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 5: filter.go (per-stream struct) + decode_headers.go + 503 wire shape

Per SPEC §6.3 + §6.4 + AMEND-6 byte-pinned wire shape per §21.P1 RATIFIED.
filter.go: per-stream filter struct + DecodeHeaders + OnEncodeComplete +
OnDestroy method declarations (OnEncodeComplete + OnDestroy bodies are
// TODO Task 6 stubs that satisfy the compiler so this commit is self-
buildable per the reviewer observation).
decode_headers.go: DecodeHeaders dispatch: disabled pass-through;
controller.forwardingDecision() CAS; on Block emit SendLocalReply(503,
\"reached concurrency limit\", {content-type: text/plain}) + rq_blocked++ +
StopAndBuffer; on Forward record entryTime + acquired + Continue.
response_code_details \"reached_concurrency_limit\" emitted to access-log
slot but ABSENT-by-config at envoy-go MVP per SPEC §12 item A3."
```

---

## Task 6: NEW `internal/filter/http/adaptive_concurrency/encode_complete.go` + `filter.go` OnEncodeComplete + OnDestroy body landings (D11 token-release)

**Files:**
- Create: `internal/filter/http/adaptive_concurrency/encode_complete.go` (~50-80 LoC; the `OnEncodeComplete` function body — invoked from `filter.go`'s method)
- Modify: `internal/filter/http/adaptive_concurrency/filter.go` (replace the Task 5 `// TODO Task 6` stubs in `OnEncodeComplete` + `OnDestroy` with the production bodies)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 6 entry)

This task lands the encoder-side sample-recording hook body + the `OnDestroy` token-release per D11. The `TestFilter_OnDestroy_ReleasesAcquiredToken_*` test from Task 3 covers the leak-prevention invariant; this Task wires the production callsite.

**Precondition:** Task 5 (filter struct + DecodeHeaders + stub OnEncodeComplete/OnDestroy) + Task 3 (controller `recordLatencySample` + `releaseInFlight`) complete.
**Artifact:** `encode_complete.go` body + `filter.go` OnEncodeComplete + OnDestroy bodies (replacing Task 5 stubs).
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run` clean; `go test -count=1 -race ./internal/filter/http/adaptive_concurrency/...` clean (Layer A tests + D11 OnDestroy tests pass).

- [ ] **Step 1: Author `internal/filter/http/adaptive_concurrency/encode_complete.go`** per the File-structure table row above + SPEC §6.5. The `onEncodeComplete()` function (package-private helper invoked from `filter.go`'s `OnEncodeComplete` method): if `!f.acquired` return (Block path or disabled); else compute `rtt := f.clock.Now().Sub(f.entryTime)`; call `f.controller.recordLatencySample(rtt)`; call `f.controller.releaseInFlight()`; clear `f.acquired = false`.

- [ ] **Step 2: Modify `internal/filter/http/adaptive_concurrency/filter.go`** — replace the Task 5 `// TODO Task 6` stubs: `OnEncodeComplete` now invokes the `onEncodeComplete()` helper from `encode_complete.go`; `OnDestroy()` now (per D11) — if `f.acquired` call `f.controller.releaseInFlight()` + clear `f.acquired = false` (the symmetric pair to `DecodeHeaders` Forward; prevents token leak on abort paths).

- [ ] **Step 3: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 4: Run the Task 3 unit tests + race tests** to ensure the production filter struct integration matches the test-side stub used at Task 3:

```bash
go test -count=1 -race ./internal/filter/http/adaptive_concurrency/...
# Expect: PASS — Layer A tests + D11 OnDestroy tests still GREEN
```

- [ ] **Step 5: Append PROGRESS.md Task 6 entry.**

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/adaptive_concurrency/encode_complete.go \
        internal/filter/http/adaptive_concurrency/filter.go \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 6: encode_complete.go + OnEncodeComplete/OnDestroy bodies per D11

Per SPEC §6.5 + D11. encode_complete.go: onEncodeComplete() helper records
the per-request RTT sample via controller.recordLatencySample(rtt) and
releases the in-flight token via controller.releaseInFlight(); routes to
latencySamples or minRTTSamples per controller-state.

filter.go: replaces Task 5 stubs — OnEncodeComplete invokes the helper;
OnDestroy() implements D11 token-release symmetry (if f.acquired on any
termination path — stream-reset, client-disconnect, HCM-side abort —
releaseInFlight() to prevent permanent slot-leak from numRqOutstanding).
TestFilter_OnDestroy_ReleasesAcquiredToken_* covers the leak-prevention
invariant."
```

---

## Task 7: NEW `internal/filter/http/adaptive_concurrency/percentile.go` + sorted-slice quantile helper

**Files:**
- Create: `internal/filter/http/adaptive_concurrency/percentile.go` (~50-80 LoC)
- Create: `internal/filter/http/adaptive_concurrency/percentile_test.go` (~80-120 LoC; vector tests + edge cases per D10)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 7 entry)

This task lands the sorted-slice percentile aggregation helper per BRAINSTORM §8 item 4 + SPEC §6.8 carve-out (NOT CircllHist; ≤ 1 bin-width divergence acceptable per ADR-0186 §Decision). **Parallelizable with Tasks 2 + 4** per D12.

**Precondition:** Task 1 complete.
**Artifact:** `percentile.go` + vector tests covering edge cases per D10.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/adaptive_concurrency/... -run 'TestPercentile'` clean (~8 vector tests + edge cases pass per D10).

- [ ] **Step 1: Write failing tests** in `internal/filter/http/adaptive_concurrency/percentile_test.go` per D10 — covering: `TestPercentile_SortedSlice_Empty` (returns 0); `TestPercentile_SortedSlice_SingleSample` (returns sample); `TestPercentile_SortedSlice_P50_KnownSet` ([1, 2, 3, 4, 5]*ms returns 3ms); `TestPercentile_SortedSlice_P0_ReturnsMin`; `TestPercentile_SortedSlice_P1_ReturnsMax`; `TestPercentile_SortedSlice_PNegative_ClampsToZero`; `TestPercentile_SortedSlice_PGreaterThanOne_ClampsToOne`; `TestPercentile_SortedSlice_UnsortedInput_DoesNotMutate` (caller's slice unchanged).

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `internal/filter/http/adaptive_concurrency/percentile.go`** per the File-structure table row above + SPEC §6.8 + BRAINSTORM §8 item 4. The `Quantile(samples []time.Duration, p float64) time.Duration` function: handle empty slice (return 0); copy input to fresh slice; `sort.Slice` ascending; clamp `p` to `[0.0, 1.0]`; compute index `idx := int(p * float64(len(sorted)-1))`; return `sorted[idx]`. ~50-80 LoC.

- [ ] **Step 4: Run tests to verify they pass.**

```bash
go test -count=1 ./internal/filter/http/adaptive_concurrency/... -run 'TestPercentile'
# Expect: PASS — ~8 vector tests + edge cases per D10
```

- [ ] **Step 5: Append PROGRESS.md Task 7 entry — RATIFIES SPEC §12 item B5 (sorted-slice quantile numeric divergence vs CircllHist) per D15.**

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/adaptive_concurrency/percentile.go \
        internal/filter/http/adaptive_concurrency/percentile_test.go \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 7: percentile.go + sorted-slice quantile helper

Per SPEC §6.8 + BRAINSTORM §8 item 4 carve-out + ADR-0186 §Decision sub-
paragraph. Quantile(samples []time.Duration, p float64) time.Duration via
sort.Slice + index interpolation; edge cases per D10 (empty slice returns
0; single-sample returns sample; out-of-range p clamps to [0.0, 1.0];
caller's input slice NOT mutated). Consumed by controller's
concurrencyUpdateTick() + updateMinRTT() callsites per SPEC §4.2 + §4.5.

≤ 1 bin-width divergence vs upstream CircllHist accepted per BRAINSTORM
§8 item 4. CLOSES SPEC §12 item B5 RATIFIED-PENDING-IMPL-TIME per D15."
```

---

## Task 8: NEW `internal/filter/http/adaptive_concurrency/fuzz_test.go` + 27th fuzzer + corpus seeds

**Files:**
- Create: `internal/filter/http/adaptive_concurrency/fuzz_test.go` (~50 LoC; 27th fuzzer per D6)
- Create: `internal/filter/http/adaptive_concurrency/testdata/fuzz/FuzzAdaptiveConcurrencyConfigParse/` (corpus seeds per D6; ~30 seeds)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 8 entry)

This task lands the 27th project-wide fuzzer `FuzzAdaptiveConcurrencyConfigParse` per SPEC §6.7 + D6. Must-never-panic across `buildCompiledConfig`. Clean at 30s per seed. Depends on Task 2 (`buildCompiledConfig` is what's fuzzed); **partially parallelizable with Tasks 3 + 5 + 6** (file-disjoint).

**Precondition:** Task 2 complete (`buildCompiledConfig` is callable).
**Artifact:** 27th fuzzer + ~30 corpus seeds.
**Acceptance:** `go test -fuzz=FuzzAdaptiveConcurrencyConfigParse -fuzztime=30s ./internal/filter/http/adaptive_concurrency/` clean (no panics across all seeds).

- [ ] **Step 1: Author `internal/filter/http/adaptive_concurrency/fuzz_test.go`** per SPEC §6.7 + D6. The fuzzer reads `data []byte`; attempts `proto.Unmarshal(data, &adaptive_concurrencyv3.AdaptiveConcurrency{})`; on Unmarshal success calls `buildCompiledConfig(typedConfig)` — recovers panic via `defer recover()` + `t.Fatalf` on panic. Must-never-panic discipline.

- [ ] **Step 2: Author corpus seeds** at `internal/filter/http/adaptive_concurrency/testdata/fuzz/FuzzAdaptiveConcurrencyConfigParse/` per D6 — ~30 seeds covering: valid full Gradient-1 config (1 seed); each PARSE-REJECT arm × valid-edge-case neighbor (~14 seeds for the 11 RATIFIED-PGV arms); each envoy-go-strict arm × variant (~4 seeds for the 2 envoy-go-strict arms); empty config + oneof-absent + nested-message-missing (~4 seeds); boundary values (~5 seeds); default-applied (~1 seed). Each seed file is a `go test -fuzz` corpus file (raw bytes — the `proto.Marshal` of a crafted `AdaptiveConcurrency` proto OR a deliberately-malformed byte sequence).

- [ ] **Step 3: Run the fuzzer**

```bash
go test -fuzz=FuzzAdaptiveConcurrencyConfigParse -fuzztime=30s ./internal/filter/http/adaptive_concurrency/
# Expect: clean — no panics at 30s per seed
```

- [ ] **Step 4: Append PROGRESS.md Task 8 entry.**

- [ ] **Step 5: Commit**

```bash
git add internal/filter/http/adaptive_concurrency/fuzz_test.go \
        internal/filter/http/adaptive_concurrency/testdata/ \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 8: 27th fuzzer FuzzAdaptiveConcurrencyConfigParse + corpus seeds

Per SPEC §6.7 + D6 — must-never-panic across buildCompiledConfig. ~30 corpus
seeds covering each PARSE-REJECT arm + envoy-go-strict arms + boundary
values + empty/oneof-absent/nested-missing variants + default-applied
edge cases. Clean at 30s per seed. 27th project-wide fuzzer (current count
26 post-phase-20 → 27 post-phase-21)."
```

---

## Task 9: Full filter integration in `adaptive_concurrency.go` + `doc.go` + boot-registration at `cmd/envoy-go/main.go`

**Files:**
- Create: `internal/filter/http/adaptive_concurrency/doc.go` (~15-25 LoC)
- Create: `internal/filter/http/adaptive_concurrency/adaptive_concurrency.go` (~80-120 LoC)
- Create: `internal/filter/http/adaptive_concurrency/adaptive_concurrency_test.go` (~80-120 LoC; integration tests + stat-name compile-time guards per D5)
- Modify: `cmd/envoy-go/main.go` (~+1 LoC + +1 import per D7)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 9 entry)

This task wires everything from Tasks 2-8 into a fully-functional `api.HTTPFilterFactory` from `New()`. Boot-registration inserts at line 125 alphabetical between `router` and `bandwidthlimit` per D7. **NO `RegisterPerRouteValidator` call** — REUSE-by-absence per §5.4.

**Precondition:** Tasks 2-8 complete.
**Artifact:** Fully-functional adaptive_concurrency filter; boot-registration alphabetical at line 125; stat-name compile-time guards green.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run` clean; `go test -count=1 -race ./internal/filter/http/adaptive_concurrency/...` ALL clean (full unit-test surface passes); `grep -nE 'httpReg.Register\(adaptive_concurrency.TypeURL' cmd/envoy-go/main.go` returns the boot-registration line at line 125; stat-name `TestStatNames_Equal_*` tests pass per D5.

- [ ] **Step 1: Author `internal/filter/http/adaptive_concurrency/doc.go`** per the File-structure table row above + SPEC §6.8. ~15-25 LoC.

- [ ] **Step 2: Author `internal/filter/http/adaptive_concurrency/adaptive_concurrency.go`** per the File-structure table row above + SPEC §6.1. Public surface: `const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency"`; `func New(message proto.Message, ctx api.HTTPFilterFactoryCtx) (api.HTTPFilterFactory, error)` factory body: calls `buildCompiledConfig(message)` per Task 2; constructs `*filterStats` via `newFilterStats(ctx.StatRegistry(), ctx.HCMStatPrefix())` per Task 4; constructs `*gradientController` via `newGradientController(cc, stats, defaultClock{})` per Task 3; returns the factory closure that produces per-stream `*filter` instances (the `filter` struct from Task 6). Compile-time interface assertions: `var _ api.StreamFilter = (*filter)(nil)` + `var _ api.StreamDecoderFilter = (*filter)(nil)` (the encoder-side hook participates per the existing HCM framework). **NO `RegisterPerRouteValidator` function** — REUSE-by-absence per §5.4. (Note: the exact factory signature + ctx interface methods are subject to the existing project's `api.HTTPFilterFactoryCtx` shape — check `internal/filter/http/registry.go` + the phase-20 oauth2 factory signature at IMPL time for the exact contract.)

- [ ] **Step 3: Author `internal/filter/http/adaptive_concurrency/adaptive_concurrency_test.go`** — integration tests for `TypeURL` constant assertion + `New` factory smoke tests + the 7-stat-name compile-time guards `TestStatNames_Equal_*` per D5 (table-driven assertion that the 7 `const statName*` declarations from Task 4's `stats.go` byte-exact-match the expected wire names).

- [ ] **Step 4: Modify `cmd/envoy-go/main.go`** per D7 — add the import `adaptive_concurrency "github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency"` (alphabetical-first in the filter-import block; before `bandwidthlimit`). Add `httpReg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)` at line 125 (between `router` at line 124 and `bandwidthlimit` which shifts from 125 to 126) per ADR-0100 §2.2. **NO `RegisterPerRouteValidator` call** — REUSE-by-absence per §5.4.

- [ ] **Step 5: Verify all unit tests + build + vet + lint clean**

```bash
go build ./... && go vet ./... && golangci-lint run
go test -count=1 -race ./internal/filter/http/adaptive_concurrency/...
# Expect: PASS — full unit-test surface
grep -nE 'httpReg\.Register\(adaptive_concurrency\.TypeURL' cmd/envoy-go/main.go
# Expect: line 125
```

- [ ] **Step 6: Append PROGRESS.md Task 9 entry.**

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/adaptive_concurrency/doc.go \
        internal/filter/http/adaptive_concurrency/adaptive_concurrency.go \
        internal/filter/http/adaptive_concurrency/adaptive_concurrency_test.go \
        cmd/envoy-go/main.go \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 9: full filter integration + boot-registration

Wires Tasks 2-8 into a fully-functional api.HTTPFilterFactory. NEW
adaptive_concurrency.go (TypeURL constant + New factory) + doc.go. Boot-
registration alphabetical at line 125 between router and bandwidthlimit
per D7 + ADR-0100 §2.2 (16 HTTP filters wired post-phase-21). NO
RegisterPerRouteValidator call — REUSE-by-absence per §5.4 (FOURTH
CONSECUTIVE §9 row to skip ADR-0125 amendment). Stat-name TestStatNames_
Equal_* compile-time guards pass per D5."
```

---

## Task 10: Differential fixture `0025-http-adaptive-concurrency` + 4-scenario directories + RATIFIED-PENDING-IMPL-TIME pin closures

**Files:**
- Create: `test/fixtures/0025-http-adaptive-concurrency/README.md` (~80-150 LoC)
- Create: `test/fixtures/0025-http-adaptive-concurrency/parse_ok/envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` (scenario a)
- Create: `test/fixtures/0025-http-adaptive-concurrency/overflow_503/envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` (scenario b; partial cross-side byte-exact per AMEND-6)
- Create: `test/fixtures/0025-http-adaptive-concurrency/stat_surface/envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` (scenario c)
- Create: `test/fixtures/0025-http-adaptive-concurrency/pass_through_when_disabled/envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` (scenario d)
- Create: `test/fixtures/0025-http-adaptive-concurrency/inputs/driver.go` (~250-400 LoC)
- Modify: `test/differential/fixture/fixture.go` (+1 enum value `HTTPAdaptiveConcurrency BackendKind = 21`)
- Modify: `test/differential/runner_test.go` (+blank import + switch-case)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 10 entry — RATIFIES SPEC §12 items A1-A4 per D14)

Per SPEC §7 + §14.3 + §14.4. Lands the 4-scenario differential fixture + the partial cross-side byte-exact 503-overflow leg per AMEND-6 + §21.P-D2 RATIFIED. CLOSES SPEC §12 items A1-A4 per D14.

**Precondition:** Task 9 complete (full filter integration GREEN).
**Artifact:** Fixture 0025 + 4-scenario byte-exact GREEN against reference Envoy v1.37.2 on scenario (b).
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `go test -count=1 ./test/differential/ -run 'Test.*0025'` clean (4-scenario expectations all GREEN; scenario (b) cross-side byte-exact on 503 + 25-byte body + 2 headers); SPEC §12 items A1-A4 status RATIFIED at PROGRESS log.

- [ ] **Step 1: Modify `test/differential/fixture/fixture.go`** — add the `HTTPAdaptiveConcurrency BackendKind = 21` enum value (after `HTTPOAuth2 = 20`) per SPEC §7 + D13.

- [ ] **Step 2: Modify `test/differential/runner_test.go`** — add the blank import + switch-case for `HTTPAdaptiveConcurrency`.

- [ ] **Step 3: Author scenario (a) `parse_ok/` directory** — `envoy.yaml` + `envoy-go.yaml` with full Gradient-1 default config; `expectations.yaml` REFERENCE-LESS subject-only structural (HTTP 200 to a normal GET; admin `/stats` exposes the 7-name surface with starting values); `README.md` narrative.

- [ ] **Step 4: Author scenario (b) `overflow_503/` directory** — `envoy.yaml` + `envoy-go.yaml` with `max_concurrency_limit=1 + min_concurrency=1 + concurrency_limit_exceeded_status=503` + backend cluster with synthetic slow 1-second response latency per SPEC §7.3; `expectations.yaml` **partial cross-side byte-exact per AMEND-6** (Request 1 → 200 OK; Request 2 → 503 + body `"reached concurrency limit"` (25 bytes verbatim) + `content-type: text/plain` + `content-length: 25` byte-pinned against BOTH reference Envoy v1.37.2 + envoy-go); `README.md` narrative explaining the 2-concurrent-slow-requests deterministic trap + the cross-side byte-exact promotion per §21.P-D2 RATIFIED.

- [ ] **Step 5: Author scenario (c) `stat_surface/` directory** — `envoy.yaml` + `envoy-go.yaml` with default Gradient-1 config + admin endpoint; `expectations.yaml` REFERENCE-LESS subject-only structural (admin `/stats` exposes the full 7-name surface; `concurrency_limit` at `min_concurrency`; `min_rtt_calculation_active = 1` during initial window per AMEND-2 C4 first-tick semantics); `README.md` narrative.

- [ ] **Step 6: Author scenario (d) `pass_through_when_disabled/` directory** — `envoy.yaml` + `envoy-go.yaml` with `enabled` absent (or `enabled.default_enabled=false`) per AMEND-4 default-OFF; `expectations.yaml` REFERENCE-LESS subject-only structural (all requests pass through; NO 503; `rq_blocked` counter stays at 0); `README.md` narrative.

- [ ] **Step 7: Author `inputs/driver.go`** — test-driver invoking the 4-scenario matrix against both reference Envoy + envoy-go; for scenario (b) issues 2 concurrent slow requests (the second dispatched after the first byte of request 1 is observed in the backend, enforcing arrival order); asserts byte-exact wire shape per per-scenario `expectations.yaml`; consumes the shared synthetic backend cluster.

- [ ] **Step 8: Author top-level `README.md`** — scenario matrix narrative; cross-references SPEC §7.1; documents the single-listener topology per D13.

- [ ] **Step 9: Run the differential fixture against both stacks**

```bash
go test -count=1 ./test/differential/ -run 'Test.*0025'
# Expect: PASS — 4 scenarios; scenario (b) cross-side byte-exact on 503-overflow leg
```

- [ ] **Step 10: Append PROGRESS.md Task 10 entry** — RATIFIES SPEC §12 items A1-A4 per D14:
  - A1 (503 body `"reached concurrency limit"` 25-byte byte-exact) — RATIFIED at scenario (b) cross-side
  - A2 (503 `content-type: text/plain` + `content-length: 25` headers byte-exact) — RATIFIED at scenario (b) cross-side
  - A3 (`response_code_details "reached_concurrency_limit"` ABSENT-by-config) — RATIFIED at scenario (b) by NOT-byte-pinning the field (envoy-go MVP has no access-log surface)
  - A4 (`min_rtt_calculation_active` Accumulate import-mode divergence) — RATIFIED forward-pointer-only at Task 13 BEHAVIOR_CONTRACT.md envoy-go-strict departure record

- [ ] **Step 11: Commit**

```bash
git add test/fixtures/0025-http-adaptive-concurrency/ \
        test/differential/fixture/fixture.go \
        test/differential/runner_test.go \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 10: fixture 0025-http-adaptive-concurrency + 4-scenario matrix + pin closures

Lands the 4-scenario differential fixture per SPEC §7 + AMEND-6:
(a) parse_ok REFERENCE-LESS subject-only structural;
(b) overflow_503 PARTIAL CROSS-SIDE BYTE-EXACT per AMEND-6 + §21.P-D2
    RATIFIED (Request 1 → 200; Request 2 → 503 + body \"reached
    concurrency limit\" 25 bytes + content-type: text/plain + content-
    length: 25 byte-pinned against BOTH reference Envoy v1.37.2 + envoy-go);
(c) stat_surface REFERENCE-LESS subject-only structural;
(d) pass_through_when_disabled REFERENCE-LESS subject-only structural.

Single-listener topology per D13. NEW BackendKind enum
HTTPAdaptiveConcurrency = 21.

CLOSES SPEC §12 items A1-A4 RATIFIED-PENDING-IMPL-TIME at PROGRESS log
per D14. First §9 family-row to land a cross-side byte-exact fixture leg
with timing-dependent setup (deterministic trap via min=max=1)."
```

---

## Task 11: ADR final-state alignment + DECISIONS.md cross-reference audit

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (final-state alignment for ADR-0186 + ADR-0187 + ADR-0059 §Decision AMENDMENT body; cross-references intact per SPEC §15 item 15)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 11 entry)

This task audits the ADR final-state landing per SPEC §15 item 15. All 2 NEW ADR §Decision + §Consequences bodies (ADR-0186 + ADR-0187) + the 1 IN-PLACE AMENDMENT body (ADR-0059) MUST be present + non-empty + cross-references intact. PLAN's D8 hypothesis is verified HOLDING.

**Precondition:** Tasks 2-10 complete (ADR bodies all anchored at their respective Tasks).
**Artifact:** ADR final-state aligned; DECISIONS.md cross-reference audit complete.
**Acceptance:** `grep -cE '^## ADR-0186' docs/envoy-go/DECISIONS.md` returns `1`; `grep -cE '^## ADR-0187' docs/envoy-go/DECISIONS.md` returns `1`; `grep -nE 'Amendment \(per phase 21 ADR-0186\)' docs/envoy-go/DECISIONS.md` returns ≥1 match within the ADR-0059 §Decision body block; `grep -nE '^## ADR-0188' docs/envoy-go/DECISIONS.md` returns 0 (D8 hypothesis HOLDS); §Decision + §Consequences bodies all non-empty for ADR-0186 + ADR-0187 + ADR-0059 AMENDMENT.

- [ ] **Step 1: Verify all 2 NEW ADR §Decision + §Consequences bodies are present + non-empty**

```bash
grep -cE '^## ADR-0186' docs/envoy-go/DECISIONS.md
# Expect: 1
grep -cE '^## ADR-0187' docs/envoy-go/DECISIONS.md
# Expect: 1
grep -A 5 '^## ADR-0186' docs/envoy-go/DECISIONS.md | tail -3
# Expect: §Context + §Decision + §Consequences blocks present
```

- [ ] **Step 2: Verify the 1 IN-PLACE §Decision AMENDMENT body at ADR-0059 is present**

```bash
grep -nE 'Amendment \(per phase 21 ADR-0186\)' docs/envoy-go/DECISIONS.md
# Expect: ≥1 match within the ADR-0059 §Decision body block (NOT the ANTICIPATION-marker variant from SPEC commit)
```

- [ ] **Step 3: Verify D8 hypothesis HOLDS — ADR-0188 stays unconsumed**

```bash
grep -nE '^## ADR-0188' docs/envoy-go/DECISIONS.md
# Expect: 0 (D8 hypothesis HOLDS — next-free stays ADR-0188 at phase-21 IMPL phase-done)
```

- [ ] **Step 4: Verify cross-references intact** — spot-check that ADR-0186 references ADR-0059 (for the §Decision AMENDMENT consumer); ADR-0187 references ADR-0044 (for the in-place edit discipline) + ADR-0080 (for byte-stable error wording); ADR-0059 §Decision AMENDMENT body references ADR-0186 + ADR-0044 + the phase-21 SPEC §3.2 + SPEC §11 §21.P5 + AMEND-7.

- [ ] **Step 5: Append PROGRESS.md Task 11 entry — record D8 hypothesis (NO additional ADR fires at IMPL) status: HOLDING as of Task 11.**

- [ ] **Step 6: Commit** (if any DECISIONS.md cross-reference cleanup required — otherwise documentation-only audit commit recording the verification)

```bash
git add docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
# AND docs/envoy-go/DECISIONS.md ONLY IF cross-reference cleanup edits were needed
git commit -m "phase 21 Task 11: ADR final-state audit (ADR-0186 + ADR-0187 + ADR-0059 AMENDMENT)

Per SPEC §15 item 15 + ADR-0044. ADR-0186 §Decision + §Consequences body
present + non-empty (anchored at Task 3). ADR-0187 §Decision +
§Consequences body present + non-empty (anchored at Task 2). ADR-0059
§Decision AMENDMENT body present + non-empty (anchored at Task 4;
REPLACES the SPEC-commit ANTICIPATION marker per ADR-0044 in-place edit
discipline). Cross-references intact.

D8 hypothesis HOLDING: ADR-0188 stays unconsumed at phase-21 IMPL phase-
done; STRENGTHENED two-slot buffer with ADR-0189 also UNCONSUMED per
SPEC §10 D."
```

---

## Task 12: Cross-package regression matrix verification per D4 + D16

**Files:**
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 12 entry — RATIFIES SPEC §12 item C8 per D16)

This task closes SPEC §12 item C8 (cross-package regression matrix for ADR-0059 §Decision AMENDMENT) per D4 + D16. The Task 4 regression check (`internal/stats/` post-AMENDMENT) is complemented here by the full-suite regression run.

**Precondition:** Task 4 + Task 10 complete (post-AMENDMENT regression check landed at Task 4; full 27-fixture regression run available via Task 10's `0025` fixture).
**Artifact:** Cross-package regression matrix verified; SPEC §12 item C8 RATIFIED.
**Acceptance:** `go test -count=1 -race ./internal/stats/...` clean (post-AMENDMENT regression); `go test -count=1 -race ./internal/filter/...` clean (post-Task-9 filter regression); `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-5])'` clean (26 pre-existing fixtures + new 0025 GREEN).

- [ ] **Step 1: Run `internal/stats/` regression**

```bash
go test -count=1 -race ./internal/stats/...
# Expect: PASS — zero regression post-ADR-0059 §Decision AMENDMENT
```

- [ ] **Step 2: Run cross-package filter regression**

```bash
go test -count=1 -race ./internal/filter/...
# Expect: PASS — all 16 filters (15 pre-existing + new adaptive_concurrency) GREEN
```

- [ ] **Step 3: Run full differential regression matrix**

```bash
go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-5])'
# Expect: PASS — 27 fixture directories (0000-0025 incl. 0007a/b) all GREEN
```

- [ ] **Step 4: Append PROGRESS.md Task 12 entry — RATIFIES SPEC §12 item C8 (cross-package regression matrix) per D16.**

- [ ] **Step 5: Commit** (documentation-only — Task 12 is verification + PROGRESS recording; no code changes)

```bash
git add docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 12: cross-package regression matrix verification per D4 + D16

Per SPEC §12 item C8. Post-ADR-0059 §Decision AMENDMENT regression check:
internal/stats/ GREEN (zero regression — AMENDMENT is pure convention-
extension; boolToInt helper addition + comment-only gauge.go cross-
reference; no signature change). Cross-package filter regression: all 16
filters GREEN. Full differential regression: 27 fixture directories
(0000-0025) GREEN.

CLOSES SPEC §12 item C8 RATIFIED-PENDING-IMPL-TIME at PROGRESS log."
```

---

## Task 13: BEHAVIOR_CONTRACT.md 7-edit bundle per SPEC §13

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (~+250-350 LoC; 7-edit bundle per SPEC §13.A-§13.E)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 13 entry)

Per SPEC §13.F — all 7 edits land at the SAME IMPL commit per ADR-0052; none mutate pre-phase-21 paragraphs (in-place-by-append discipline).

**Precondition:** Task 10 complete (some §13 paragraphs reference the fixture-0025 wire-shape closures from Task 10).
**Artifact:** BEHAVIOR_CONTRACT.md 7-edit bundle landed atomically.
**Acceptance:** `grep -cE '^### envoy\.filters\.http\.adaptive_concurrency' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns `1` (NEW subsection); 7-name stat-table rows present; ADR-0059 §Decision AMENDMENT cross-reference paragraph present; 2 envoy-go-strict departure records present; NEW `### Phase 21 forward-pointer notes` subsection present; Per-route canonical patterns cross-reference table caption updated.

- [ ] **Step 1: §13.A.1 — NEW `### envoy.filters.http.adaptive_concurrency` subsection** inserted after `### envoy.filters.http.oauth2`. ~150-250 LoC subsection per SPEC §13.A: filter scope + populated-vs-deferred field map (full GradientControllerConfig sub-surface; PARSE-REJECT for `enabled.runtime_key` per ADR-0187 + `min_rtt_calc_params.fixed_value` per ADR-0186 §Consequences (d)); state-machine summary (first-tick semantics per AMEND-2 C4; concurrency-update tick per §4.2; minRTT recalc with `sample_aggregate_percentile`-quantile per AMEND-2 C1 + 5-consec-trigger per AMEND-2 C3); 7-name stat surface; deny-path wire shape (503 + `"reached concurrency limit"` body per AMEND-6); listener-scoped discipline (REUSE-by-absence per §5.4 — FOURTH CONSECUTIVE §9 row); envoy-go-strict departures (RTT-ns-vs-ms per AMEND-3 C3; sorted-slice-vs-CircllHist per BRAINSTORM §8 item 4).

- [ ] **Step 2: §13.B.2 — Stat-name mapping 92-name → 99-name table extension** — 1 new counter (`rq_blocked`) + 6 new gauges (`concurrency_limit` / `gradient` / `burst_queue_size` / `sample_rtt_msecs` / `min_rtt_msecs` / `min_rtt_calculation_active`) per AMEND-3. Table caption updated. Per-row units annotated (ns for envoy-go RTT gauges; ×1000 for gradient; raw-uint32 for limit/burst_queue_size; 0/1 for active flag).

- [ ] **Step 3: §13.B.3 — ADR-0059 §Decision AMENDMENT cross-reference paragraph** — extends the existing `## Internal Stats Store` umbrella subsection with a paragraph noting the float-valued-gauge int64 encoding convention added at phase-21 per AMEND-7. Cross-references to ADR-0186 + ADR-0059 + SPEC §3.2.

- [ ] **Step 4: §13.C.4 — NEW envoy-go-strict departure record for RTT-gauge units divergence** per AMEND-3 C3: envoy-go uses nanoseconds while upstream uses milliseconds; stat NAMES preserve upstream byte-exact (`sample_rtt_msecs` / `min_rtt_msecs`); per-metric `# HELP` text disambiguates the unit.

- [ ] **Step 5: §13.C.5 — NEW envoy-go-strict departure record for sorted-slice-vs-CircllHist percentile-aggregation divergence** per BRAINSTORM §8 item 4 + AMEND-3: numeric outputs may differ by ≤ 1 bin-width at the percentile boundary; gradient values + new-limit values + sampled-RTT values are NOT cross-side byte-exact (only the 503-overflow wire shape is — per AMEND-6).

- [ ] **Step 6: §13.D.6 — NEW `### Phase 21 forward-pointer notes` subsection** placed immediately after `### Phase 20 forward-pointer notes`. Documents the 8 SPEC §8 forward-pointers: RTDS runtime keying (§8 item 1); cross-side byte-exact algorithmic parity (§8 item 2); alternative ConcurrencyControllerConfig oneof arms (§8 item 3); CircllHist percentile-aggregation upgrade (§8 item 4); `fixed_value` static-minRTT path (§8 item 5); multi-listener controller-state-isolation explicit verification (§8 item 6); `min_rtt_calculation_active` Accumulate import-mode parity (§8 item 7); `response_code_details` emission (§8 item 8).

- [ ] **Step 7: §13.E.7 — Per-route canonical patterns cross-reference table caption update** — "updated through phase 20" → "updated through phase 21"; phase-21 cross-reference paragraph added documenting the REUSE-by-absence (FOURTH CONSECUTIVE §9 row; no roster extension; the absence-as-recurring-pattern note).

- [ ] **Step 8: Verify edit-bundle completeness** — `grep -cE '^### envoy\.filters\.http\.adaptive_concurrency' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns `1`; all 7 edit items present.

- [ ] **Step 9: Append PROGRESS.md Task 13 entry.**

- [ ] **Step 10: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md
git commit -m "phase 21 Task 13: BEHAVIOR_CONTRACT.md 7-edit bundle per SPEC §13

Lands all 7 edits atomically per ADR-0052 (in-place-by-append): NEW
envoy.filters.http.adaptive_concurrency subsection (~150-250 LoC); stat-
table 92 → 99 extension (1 counter + 6 gauges); ADR-0059 §Decision
AMENDMENT cross-reference paragraph at the Internal Stats Store umbrella;
2 envoy-go-strict departure records (RTT-ns-vs-ms per AMEND-3 C3; sorted-
slice-vs-CircllHist per BRAINSTORM §8 item 4); NEW Phase 21 forward-pointer
notes subsection (8 SPEC §8 items); Per-route canonical patterns cross-
reference table caption update (FOURTH CONSECUTIVE §9 row REUSE-by-
absence)."
```

---

## Task 14: Six-gate phase-done verification + STATE.md re-advance + ROADMAP row 21 flip + REVIEW.md

**Files:**
- Modify: `docs/envoy-go/STATE.md` (rewrite-in-place to post-phase-21 state per BOOTSTRAP §4.1 invariant 1)
- Modify: `docs/envoy-go/ROADMAP.md` (row 21 status flips `in-progress → done` + per-cell IMPL-done annotation)
- Create: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/REVIEW.md` (~300 LoC; per `superpowers:requesting-code-review`)
- Append: `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (Task 14 entry — final task; 6-gate outputs captured verbatim)

Per SPEC §7.4 + §14.5 + §15. The 6 phase-done gates A/B/C/D/E/F MUST be GREEN for the row-21 status flip. The 18-item SPEC §15 acceptance checklist is the reviewer's authoritative validation per `superpowers:requesting-code-review` invocation.

**Precondition:** Tasks 1-13 complete.
**Artifact:** All 6 gates green; STATE.md + ROADMAP advanced; REVIEW.md authored; phase-done ready for squash-merge.
**Acceptance:** All 6 gates GREEN (captured verbatim in PROGRESS.md Task 14 entry); SPEC §15 all 18 items checked + green; STATE.md updated with post-phase-21 state (`lifecycle-state: phase 21 IMPL done`; `next-skill: superpowers:brainstorming` for next-phase; `last-commit: <TBD — SHA-fill follow-up after squash-merge>`; `next-free ADR: ADR-0188`); ROADMAP row 21 status `done`; REVIEW.md authored.

- [ ] **Step 1: Gate A — build** — `go build ./...` clean. Capture output verbatim in PROGRESS.md.

- [ ] **Step 2: Gate B — vet + lint** — `go vet ./...` + `golangci-lint run` clean; no new suppressions. Capture output verbatim.

- [ ] **Step 3: Gate C — race** — `go test -race -count=1 ./...` clean; zero data-race violations across all packages including the new `TestController_ConcurrentForwardingDecision_*` + `TestController_FAKE_TIME_TimerOrdering_*` race groups per D3 + the `internal/stats/` post-AMENDMENT regression suite per D4. Capture output verbatim.

- [ ] **Step 4: Gate D — differential + cross-package regression matrix per D4** — `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-5])'` clean (all 27 fixture directories GREEN: 26 pre-existing + new 0025). Per SPEC §12 item C8: `internal/stats/` post-ADR-0059 AMENDMENT GREEN (RATIFIED at Task 4 + Task 12). Capture output verbatim.

- [ ] **Step 5: Gate E — fuzz** — `go test -fuzz=FuzzAdaptiveConcurrencyConfigParse -fuzztime=30s ./internal/filter/http/adaptive_concurrency/` clean (no panics). 26 pre-existing fuzzers re-run clean at 30s per seed via `go test -fuzz=Fuzz -fuzztime=30s ./...` or per-package. Capture output verbatim.

- [ ] **Step 6: Gate F — h2spec** — `make test-h2spec` 53/53 PASS at ADR-0051 pin. Capture output verbatim.

- [ ] **Step 7: Update STATE.md** to post-phase-21 state per BOOTSTRAP §4.1 invariant 1:
  - `active-phase`: next-phase identifier (TBD by user; placeholder `"to-be-determined-at-next-session"`)
  - `lifecycle-state`: `phase 21 IMPL done; awaiting next-phase identification`
  - `next-skill`: `superpowers:brainstorming` (the next-phase initial skill; OR per the user's next-phase direction)
  - `last-commit`: `<TBD — SHA-fill follow-up after squash-merge>` placeholder
  - `last-updated`: today's date
  - `next-free ADR`: `ADR-0188` (UNCHANGED — D8 hypothesis HOLDS; no additional ADR consumed at IMPL; STRENGTHENED two-slot buffer with ADR-0189 also UNCONSUMED per SPEC §10 D)
  - Verbose summary captures: 14 tasks landed; 2 NEW ADRs anchored (ADR-0186 + ADR-0187); 1 IN-PLACE AMENDMENT body (ADR-0059); LEANEST framework-delta §9 row to date (ZERO new framework primitives); 27th fuzzer; 27/27 differential fixtures green (26 pre-existing + new 0025); all 6 phase-done gates green; SPEC §15 18 items all GREEN.

- [ ] **Step 8: Update ROADMAP.md row 21** — status flips `in-progress → done`; per-cell IMPL-done annotation appended documenting the 14-task IMPL landing + the 6-gate green outputs + the LEANEST-framework-delta-§9-row milestone + the SPEC §15 18-item acceptance + the FOURTH-CONSECUTIVE-§9-row-to-skip-ADR-0125 lesson.

- [ ] **Step 9: Author REVIEW.md** per `superpowers:requesting-code-review` — ~300 LoC reviewer artifact covering: the 6-gate outputs verbatim; the SPEC §15 18-item checklist verification with cite-to-PROGRESS-entry per item; the D1-D18 planner-decision-disposition record (which decisions HELD, which were AMENDED at IMPL); the next-phase handoff state.

- [ ] **Step 10: Append final PROGRESS.md Task 14 entry** with all 6 gate outputs verbatim + the SPEC §15 18-item closure checklist + the D8 final hypothesis status (HOLDING — ADR-0188 + ADR-0189 stay unconsumed at phase-21 IMPL phase-done).

- [ ] **Step 11: Verify nothing left uncommitted**

```bash
git status --porcelain
# Expect: empty
```

- [ ] **Step 12: Commit (Task 14 final IMPL-worktree commit)**

```bash
git add docs/envoy-go/STATE.md \
        docs/envoy-go/ROADMAP.md \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md \
        docs/envoy-go/phases/21-http-filter-adaptive-concurrency/REVIEW.md
git commit -m "phase 21 Task 14: 6-gate phase-done verification + STATE/ROADMAP advance + REVIEW

All 6 phase-done gates GREEN: A build / B vet+lint / C race / D differential
(27/27 fixture directories incl. new 0025) / E fuzz (27 fuzzers clean) /
F h2spec 53/53 PASS. Cross-package regression matrix per SPEC §12 item C8
GREEN (internal/stats/ post-ADR-0059 AMENDMENT).

SPEC §15 18-item acceptance checklist all GREEN. D8 hypothesis HOLDING:
ADR-0188 + ADR-0189 both stay unconsumed at IMPL phase-done (STRENGTHENED
two-slot buffer per SPEC §10 D). STATE.md re-advanced to post-phase-21
state. ROADMAP row 21 flipped in-progress → done. REVIEW.md authored per
superpowers:requesting-code-review.

FOURTEENTH §9 family-row landed. LEANEST framework-delta §9 row to date
(ZERO new internal/ framework primitives). FOURTH CONSECUTIVE §9 row to
skip ADR-0125 amendment (REUSE-by-absence per §5.4)."
```

---

## Phase-done squash-merge + push to origin

After Task 14 completes:

1. **Squash-merge to master** (from the master worktree):

```bash
cd /home/esa/git/envoy-go  # the master worktree
git merge --squash phase-21-http-filter-adaptive-concurrency-impl
# Resolve commit message — body must include the 14-task summary + the 2-NEW-ADR
# + 1-IN-PLACE-AMENDMENT roster + the closes-row-21 + LEANEST-framework-delta milestone.
git commit -m "$(cat <<'EOF'
Squash merge phase-21-http-filter-adaptive-concurrency-impl

Closes ROADMAP row 21 (in-progress → done) — FOURTEENTH §9 family-row.

14 tasks landed. 2 NEW ADRs anchored (ADR-0186 Gradient-1 controller state
machine + inline Clock seam + FAKE-TIME differential strategy + sorted-
slice percentile aggregation + line-cited algorithmic lemmata against
gradient_controller.cc; ADR-0187 RTDS enabled.RuntimeFeatureFlag deferral
PARSE-REJECT + enabled empty-default OFF semantics per AMEND-4). 1 IN-
PLACE §Decision AMENDMENT body anchored (ADR-0059 float-valued-gauge
int64 encoding convention; ns for time-typed; ×1000 for ratio-typed;
0/1 for bool-typed via NEW internal/stats/conv.go boolToInt helper). 27th
fuzzer FuzzAdaptiveConcurrencyConfigParse clean at 30s. 27/27 differential
fixture directories GREEN (0000-0025). All 6 phase-done gates GREEN.
SPEC §15 18-item acceptance checklist all GREEN.

LEANEST framework-delta §9 row to date — ZERO new internal/ framework
primitives. FIRST §9 row since phase 14 compressor to introduce zero
framework primitives. Inline Clock seam in adaptive_concurrency package
(consumer count 1; future EXTRACT-NOW trigger when second timer-driven
filter materializes).

FOURTH CONSECUTIVE §9 row to skip ADR-0125 amendment (REUSE-by-absence
per §5.4; no AdaptiveConcurrencyPerRoute proto message in v1.32.4 or
v1.37.x). Stat surface 92 → 99 names per AMEND-3 (1 counter + 6 gauges
under http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.*).
Partial cross-side byte-exact on 503-overflow leg per AMEND-6 + §21.P-D2
RATIFIED (status + 25-byte body + 2 headers byte-pinned against both
reference Envoy v1.37.2 + envoy-go).

Two envoy-go-strict departures documented per SPEC §13: RTT-ns-vs-ms
per AMEND-3 C3 (stat NAMES preserve byte-exact upstream sample_rtt_msecs
/ min_rtt_msecs); sorted-slice-vs-CircllHist percentile aggregation per
BRAINSTORM §8 item 4 (≤ 1 bin-width divergence acceptable).

D8 hypothesis HOLDING: ADR-0188 + ADR-0189 both stay unconsumed at IMPL
phase-done. STRENGTHENED two-slot escape-valve buffer carried forward to
phase 22.
EOF
)"
```

2. **SHA-fill follow-up** (per the phase-09..20 convention):

```bash
# Update STATE.md last-commit field with the real squash SHA (was TBD at Task 14):
# Edit docs/envoy-go/STATE.md replacing "<TBD — SHA-fill follow-up after squash-merge>"
# with the actual squash commit SHA from `git log -1 --format=%H master`.
git add docs/envoy-go/STATE.md
git commit -m "phase 21 IMPL follow-up: STATE.md SHA-fill (TBD → <squash SHA> post-squash)"
```

3. **Push to origin** (per project memory `feedback_push_to_origin.md` — always-push-to-origin without asking):

```bash
git push origin master
```

4. **Worktree cleanup** (optional but tidy):

```bash
git worktree remove /home/esa/git/envoy-go/.worktrees/phase-21-http-filter-adaptive-concurrency-impl
# Keep the branch alive for reference; do NOT delete unless cleanup is explicit
```

---

## Remember
- Exact file paths always.
- Complete code shapes are in the SPEC §6 references — the PLAN points to SPEC §6 rather than reproducing the full code (per the SPEC-vs-PLAN division of labor); the per-Task File-structure table rows + per-Task Step bodies above describe the IMPL surface in implementer-actionable detail.
- Exact commands with expected output for each Step.
- Reference relevant skills with @ syntax where applicable: `@superpowers:subagent-driven-development` (recommended IMPL execution per project memory `feedback_execution_style.md`), `@superpowers:executing-plans` (alternative inline), `@superpowers:systematic-debugging` (when race-test flakes surface at Task 3), `@superpowers:test-driven-development` (every code task is Write-failing-test → Run-FAIL → Implement → Run-PASS → Commit), `@superpowers:requesting-code-review` (Task 14), `@superpowers:verification-before-completion` (the 6 phase-done gates at Task 14).
- DRY, YAGNI, TDD, frequent commits.
