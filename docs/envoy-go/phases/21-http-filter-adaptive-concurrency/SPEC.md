# Phase 21 SPEC — `envoy.filters.http.adaptive_concurrency`

> **Lifecycle state:** SPEC.md authored; ROADMAP row `21` already at `in-progress` (registered at the phase-21 BRAINSTORM commit per the phase-20-established BRAINSTORM-time row-registration precedent; per-cell narrative updated at this SPEC commit with the SPEC-done annotation; status stays `in-progress` until IMPL phase-done with all 6 gates GREEN). Per ADR-0045 the SPEC author settled the split disposition: **SINGLE-ROW landing** (no sub-rows `21.1`/`21.2`; precedent at phase-14 compressor, phase-17 jwt_authn, and phase-20 oauth2 single-row landings). LoC envelope re-estimated post-empirical-scrape at ~1300-1600 (slightly above the BRAINSTORM's ~1200-1500 envelope due to the algorithmic-invariant AMENDs at AMEND-2 — extra state-machine cases for the 5-consecutive-min forced-recalc trigger + first-tick semantics — but well below the ADR-0045 split-gate). Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09–20 precedent. This SPEC is the authoritative input to the phase-21 PLAN.
>
> **Predecessor:** `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/BRAINSTORM.md` (the 5-question Q1-Q5 settled context + the §3 framework-survey result + the §8 deferred-items register + the §10 4 SPEC-time D-questions D1-D4). The §10 empirical pins are resolved in this SPEC's §11; the §1.1 amendment-block records the BRAINSTORM corrections (AMEND-1..AMEND-7) driven by the empirical scrape. NO off-master prebrainstorm-notes branch was authored for phase 21.
>
> **Scope (per BRAINSTORM §1.1 + the SPEC-time empirical-pin scrape):** phase 21 lands `envoy.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency` (the canonical Envoy v1.37.2 Gradient-1 adaptive-concurrency filter) as the FOURTEENTH §9 family-row under the 07.1 framework, the **LEANEST framework-delta §9 row to date** (the FIRST §9 row since phase 14 compressor to introduce ZERO new `internal/` framework primitives). MVP envelope: the full `GradientControllerConfig` sub-surface (`sample_aggregate_percentile`, `concurrency_limit_params`, `min_rtt_calc_params` at all tunable parameters) + `concurrency_limit_exceeded_status` (default 503) + the static `enabled.default_enabled` honored; PARSE-REJECT for `enabled.runtime_key != ""` (deferring the RTDS RuntimeFeatureFlag to a future Runtime/RTDS family phase per ADR-0187); PARSE-REJECT for `min_rtt_calc_params.fixed_value` (deferring the static-minRTT alternative path per AMEND-1 C4). **Single algorithmic-invariant carve-out:** envoy-go uses **sorted-slice percentile aggregation** in place of upstream's **CircllHist** (BRAINSTORM §8 item 4; numeric-divergence ≤ 1 bin-width at the percentile boundary; documented forward-pointer for a future algorithmic-fidelity-extension phase). **Listener-scoped only** (REUSE-by-absence per §5.4 + §21.P-PARSE-REJECT; no `AdaptiveConcurrencyPerRoute` proto message exists in v1.37.x). **FOURTH CONSECUTIVE §9 row** to skip ADR-0125 amendment (extending the third-consecutive run from phase-20).
>
> **ADR continuity:** Phase 20 closed at ADR-0185. Phase 21 anticipates **2 NEW ADRs** (ADR-0186 + ADR-0187) + **1 IN-PLACE §Decision AMENDMENT** (ADR-0059 per D1 resolution; the BRAINSTORM-anticipated ADR-0188-candidate **COLLAPSES** per §21.P-D1 → in-place §Decision AMENDMENT route). §Context drafts anchor at this SPEC commit; §Decision + §Consequences bodies land at each ADR's Lands-in-Task per ADR-0044. The 1 in-place AMENDMENT-anticipation paragraph at ADR-0059 anchors at this SPEC commit; AMENDMENT body lands at IMPL Task 4 (the controller-state-machine task that first calls `Gauge.Set` on a non-int64-natural value). **Next-free ADR after phase 21 SPEC commit stays `ADR-0188`** (2 numbers consumed: ADR-0186..ADR-0187). ADR-0044 escape-valve held in reserve for ~0-2 impl-time-unanticipated ADRs at ADR-0188+. **D11-style hypothesis** (BRAINSTORM §7.4) STRENGTHENED at SPEC commit: ADR-0188 + ADR-0189 stay UNCONSUMED at phase-21 phase-done (next-free escape valve carried forward TWO slots to phase 22; the D1 resolution collapsed the BRAINSTORM-anticipated ADR-0188 candidate, so the escape-valve buffer is now 2 slots wide instead of 1).
>
> **Authored:** 2026-05-18.

---

## 1. Purpose

Phase 21 lands `envoy.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency` — the canonical Envoy v1.37.2 Gradient-1 adaptive-concurrency filter that estimates `minRTT` from sampled request round-trip latency and continuously adjusts a per-HCM-instance concurrency limit to bound tail latency under load — under the 07.1 framework, as the FOURTEENTH §9 production HTTP filter. It establishes the entire `internal/filter/http/adaptive_concurrency/` package, the per-HCM-instance Gradient-1 controller state machine (in-flight token discipline via lock-free CAS + per-`concurrency_update_interval` tick recalc + per-`min_rtt_calc_params.interval` minRTT recalc window with `sample_aggregate_percentile`-quantile aggregation per AMEND-2 + the 5-consecutive-min forced-recalc trigger per AMEND-2), the inline `Clock` interface seam at `clock.go` (NO new `internal/clock/` framework primitive per BRAINSTORM Q3), the 7-name stat surface (1 counter + 6 gauges; names byte-exact-upstream per AMEND-3), the 27th project-wide fuzzer `FuzzAdaptiveConcurrencyConfigParse`, the differential fixture `0025-http-adaptive-concurrency` (with **partial cross-side byte-exact promotion** on the 503-overflow leg per AMEND-6 + §21.P-D2 RATIFIED), and the listener-scoped-only enforcement (REUSE-by-absence per AMEND-5 + §5.4 — FOURTH CONSECUTIVE §9 row to skip ADR-0125 amendment). **It introduces ZERO new `internal/` framework primitives** (LEANEST framework-delta §9 row to date) and adds ONE in-place §Decision AMENDMENT to ADR-0059 (the float-valued gauge int64-encoding convention per D1 resolution).

**3 architectural primitives that make this work:**

1. **NEW `internal/filter/http/adaptive_concurrency/` package** owning the filter + controller implementation. Package directory + Go-package identifier are both `adaptive_concurrency` (Go-style underscored; matches `header_mutation` precedent — the two §9 filters with a multi-word canonical name; ADR-0114 stylistic license). ~10 production Go files + ~6 test files per §6.8. Anticipated ~1100-1400 LoC filter+controller proper. Exposes `TypeURL` (canonical `"type.googleapis.com/envoy.extensions.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency"`) + `New` (the `HTTPFilterFactory`). ADR-0186 codifies the package shape + Gradient-1 controller state machine + clock seam inline + algorithmic invariants line-cited against upstream `gradient_controller.cc`.

2. **NEW in-package `Clock` interface seam** at `internal/filter/http/adaptive_concurrency/clock.go` per BRAINSTORM Q3. ~30-60 LoC. `Clock` interface (`Now() time.Time` + `AfterFunc(d time.Duration, fn func()) Stop`) with `defaultClock` wrapping `time.Now` + `time.AfterFunc`. NOT extracted as `internal/clock/` framework primitive (consumer count = 1; YAGNI per phase-17 jwt_authn EXTRACT-NOW-only-when-trigger-fires lesson). Forward-pointer to a future-phase EXTRACT-NOW trigger when a second timer-driven filter (admission_control, global rate limit, or similar) lands. `fakeClock` lives in test scope only (~80-150 LoC at `clock_test.go` + `controller_test.go` — deterministic step-driven exercise of the controller state machine for the algorithmic-fidelity unit-test layer per §14.1).

3. **IN-PLACE ADR-0059 §Decision AMENDMENT** per D1 resolution. The `*stats.Gauge` primitive is int64-only by ADR-0059 §Decision (unchanged); phase-21 introduces 4 value-classes that are not int64-natural (1 ratio-typed `gradient` + 2 time-typed `sample_rtt_msecs` / `min_rtt_msecs` + 1 bool-typed `min_rtt_calculation_active`). The AMENDMENT codifies the operator-readable encoding convention layered atop the unchanged primitive: time-typed → int64 nanoseconds direct via `Gauge.Set(rtt.Nanoseconds())`; ratio-typed → int64 ×1000 via `Gauge.Set(int64(gradient * 1000))`; bool-typed → int64 0/1 via `Gauge.Set(boolToInt(b))`. ~20-30 LoC delta in `internal/stats/` (a sibling `boolToInt` helper + comment-only `gauge.go` doc-comment extension; no signature change to `*stats.Gauge`). **AMENDMENT-anticipation paragraph anchors at this SPEC commit**; AMENDMENT body lands at IMPL Task 4 alongside the first `stats.go` registration callsite. **NOTE on RTT unit:** envoy-go encodes RTT gauges as **nanoseconds** (Go-stdlib-natural; `time.Duration.Nanoseconds()`) while upstream encodes as **milliseconds** (per AMEND-3 + §21.P-D4). This is documented as an **envoy-go-strict departure** per §13 item C5 and BEHAVIOR_CONTRACT §13.D Phase 21 forward-pointer notes — operators see envoy-go gauges in ns and upstream gauges in ms with the per-metric `# HELP` text disambiguating; the divergence stems from envoy-go's Go-stdlib-natural choice vs upstream's `duration_cast<milliseconds>` per-callsite cast.

After phase 21, the project has the foundational adaptive-concurrency filter: a per-request filter that acquires an in-flight token at `decodeHeaders` entry (via lock-free CAS against `num_rq_outstanding < concurrency_limit`), returns the configured `concurrency_limit_exceeded_status` (default 503) with the byte-exact 25-byte body `"reached concurrency limit"` on overflow + increments the `rq_blocked` counter, samples the per-request RTT at full-encode completion and appends to the current concurrency-update window, periodically recalculates the per-HCM-instance concurrency limit via the gradient formula `gradient = clamp(0.5, min_rtt_with_buffer / sample_rtt, 2.0)` (line-cited per ADR-0186 against upstream `gradient_controller.cc:190-192`), periodically re-measures minRTT via the recalc window (clamp limit to `min_concurrency` + collect ≥`request_count` samples + take the `sample_aggregate_percentile`-quantile via sorted-slice aggregation per AMEND-2 — NOT MIN as BRAINSTORM hypothesized), force-arms an immediate minRTT recalc when 5 consecutive ticks pin the limit at `min_concurrency` (per AMEND-2 + ADR-0186 §Decision (e)), and PARSE-REJECTs the RTDS `enabled.runtime_key != ""` runtime-keying surface (per ADR-0187) + the `fixed_value` static-minRTT alternative path (per AMEND-1 C4). Observable-outcomes byte-equivalent to reference Envoy v1.37.2 adaptive_concurrency on the 503-overflow leg (per AMEND-6 + §21.P-D2 RATIFIED) and stat-name byte-equivalent on the 7-name stat-surface (per AMEND-3 + §21.P-D4). Numeric divergence in algorithmic outputs (gradient values + new-limit values + sampled-RTT values) is ≤ 1-bin-width at percentile boundaries (sorted-slice vs CircllHist) per the documented BRAINSTORM §8 item 4 carry-forward.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The 5 §11 empirical pins (executed at this SPEC session via parallel-subagent fan-out against v1.37.2 reference Envoy) generated the following 7 amendment-block entries — load-bearing record of empirical-scrape-driven design revisions to the BRAINSTORM:

- **AMEND-1 (proto-schema corrections):** Four sub-corrections vs BRAINSTORM §1.1:
  - **C1 field rename**: `concurrency_limit_calculation_params` → `concurrency_limit_params` (the upstream proto field name has no `_calculation_` infix; `adaptive_concurrency.proto:56-57`).
  - **C2 proto-file consolidation**: GradientControllerConfig lives **inline** in `adaptive_concurrency.proto` at v1.37.2; the BRAINSTORM-anticipated separate `gradient_controller.proto` file **does not exist** (verified via empirical 404 scrape).
  - **C3 `interval` validation nuance**: `min_rtt_calc_params.interval` is PGV `gte {nanos: 1000000}` (must be ≥1ms when set) — NOT proto-required, because `fixed_value` is the alternative path.
  - **C4 `fixed_value` alternative-path disposition**: BRAINSTORM did not enumerate the `min_rtt_calc_params.fixed_value` field; SPEC **PARSE-REJECT** as a deferred-future-extension per §5.3 (the static-minRTT path is an alternative to the dynamic-recalc path and warrants its own brainstorm if/when an operator surface materializes).

- **AMEND-2 (algorithmic-invariant corrections — driven by line-cited `gradient_controller.cc` scrape per §21.P-D3):** Four sub-corrections vs BRAINSTORM §1.1 + §5.1:
  - **C1 minRTT recalc aggregation**: REFUTES BRAINSTORM's "take the MIN as the new minRTT" claim. Upstream computes the new minRTT as the **`sample_aggregate_percentile`-quantile** (default p50) of the recalc-window samples — the *same* percentile used for per-tick sample-RTT aggregation (`gradient_controller.cc:176-182`). The two surfaces share the `processLatencySamplesAndClear` helper.
  - **C2 jitter semantics**: REFINES BRAINSTORM's "uniform [0, interval × jitter]" framing. Upstream applies jitter as **additive uniform** `[0, interval × jitter_pct)` to the **next-interval delay** (not to the recalc-window length nor to the window-start time directly; `gradient_controller.cc:152-160`).
  - **C3 5-consecutive-min forced-recalc trigger**: BRAINSTORM omitted this path. Upstream force-arms `min_rtt_calc_timer_` to 0ms when `updateConcurrencyLimit` is called with `new_limit == old_limit == min_concurrency` for **5 consecutive ticks** outside a minRTT window (`gradient_controller.cc:281-283`). Guards against a stale-high minRTT pinning the system at min_concurrency.
  - **C4 first-tick semantics**: BRAINSTORM did not specify. At construction, if `isMinRTTSamplingEnabled()`, the controller **immediately enters the minRTT sampling window** with `concurrency_limit_` pinned to `min_concurrency`. The `sample_reset_timer_` is enabled but its callback short-circuits while `inMinRTTSamplingWindow()` and does not re-arm itself; re-arming happens inside `updateMinRTT` once the minRTT window closes (`gradient_controller.cc:66-80, 149`). No gradient is computed during the initial minRTT window.

- **AMEND-3 (stat-name + encoding + template corrections — driven by §21.P-D4):** Five sub-corrections vs BRAINSTORM §5:
  - **C1 stat-name renames trio**: `sampled_rtt` → `sample_rtt_msecs`; `min_rtt` → `min_rtt_msecs`; `recalculating_min_rtt` → `min_rtt_calculation_active` (the upstream names; verified via `ALL_GRADIENT_CONTROLLER_STATS` macro at `controller/gradient_controller.h:27-34`).
  - **C2 stat-prefix template simplification**: REFUTES BRAINSTORM's `<filter_stat_prefix>.adaptive_concurrency.<config_stat_prefix>.gradient_controller.<stat>` template. Upstream template is `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.<stat>` — the leading prefix is the **HCM-injected per-filter prefix** (SN2 ADR-0143 convention, same as every other §9 filter), with `adaptive_concurrency.` and `gradient_controller.` as **two hardcoded literal infixes** (no `<config_stat_prefix>` field exists in the v1.37.2 proto). Net: phase-21 stat-prefix mirrors phase-20 oauth2's `http.<HCM_stat_prefix>.oauth2.*` shape with the additional `gradient_controller.` literal infix.
  - **C3 RTT-gauge units — envoy-go-strict departure**: Upstream encodes `sample_rtt_msecs` + `min_rtt_msecs` as **milliseconds** via `duration_cast<milliseconds>` (`gradient_controller.cc:75-76, 154-155`). envoy-go encodes as **nanoseconds** (Go-stdlib-natural via `time.Duration.Nanoseconds()`). Documented as envoy-go-strict departure per §13 item C5. **NOTE on naming**: phase-21 envoy-go keeps the upstream byte-exact names `sample_rtt_msecs` / `min_rtt_msecs` (do NOT rename to `_nanos`); operators consulting both proxies see the same stat name with different units (with `# HELP` text disambiguating).
  - **C4 `gradient` encoding**: RATIFIED — upstream `gradient * 1000` int64 (`gradient_controller.cc:176`). Operators divide by 1000 to recover the ratio.
  - **C5 `min_rtt_calculation_active` import mode**: Upstream marks this gauge as `Accumulate` (cross-restart-state preservation), NOT `NeverImport`. envoy-go has no hot-restart surface at MVP (per ADR-0009 absence) so the distinction is forward-pointer only; documented as deferred per §8 item 7.

- **AMEND-4 (`enabled` empty-default semantics — driven by §21.P-PARSE-REJECT):** REFUTES BRAINSTORM §2.1's claim that "absent `enabled` field treats filter as enabled per upstream". Per the empirical scrape: `enabled = RuntimeFeatureFlag{}` whose `default_enabled` defaults to `BoolValue{value: false}` proto-zero ⇒ filter is **OFF by default** when `enabled` is absent entirely. envoy-go MUST match upstream semantics: filter is OFF unless `enabled.default_enabled = true` is explicitly set OR (for runtime-keyed configs — PARSE-REJECTed per ADR-0187) the runtime layer returns true.

- **AMEND-5 (PARSE-REJECT roster expansion — driven by §21.P-PARSE-REJECT):** BRAINSTORM enumerated 6 PARSE-REJECT arms; empirical scrape surfaced 5 additional arms phase-21 SPEC §5 anchors. Additional arms: (i) `concurrency_limit_params` required-message (`adaptive_concurrency.proto:56-57`); (ii) `min_rtt_calc_params` required-message (`adaptive_concurrency.proto:59`); (iii) `max_concurrency_limit == 0` when set (`adaptive_concurrency.proto:32`); (iv) `min_concurrency == 0` when set (`adaptive_concurrency.proto:46`); (v) `min_rtt_calc_params.fixed_value` set (envoy-go-strict deferral per AMEND-1 C4 + §5.3 — stricter than upstream which honors the alternative path).

- **AMEND-6 (503-overflow cross-side byte-exact promotion — driven by §21.P-D2 RATIFIED):** The fixture `0025-http-adaptive-concurrency` 503-overflow leg promotes from BRAINSTORM's REFERENCE-LESS subject-only structural to **partial cross-side byte-exact** per §21.P-D2 RATIFIED. Trap soundness confirmed: when `max_concurrency_limit=1 + min_concurrency=1`, the controller HARD-clamps the effective limit at 1 in every phase (constructor + minRTT-window-entry + every post-sample `calculateNewLimit` clamp at `gradient_controller.cc:153-154`); the in-flight-CAS at `forwardingDecision` (`gradient_controller.cc:209-233`) resolves arrival-order deterministically. Byte-pinned wire shape: status `503 Service Unavailable`; response body `"reached concurrency limit"` (25 bytes verbatim; no trailing newline; no JSON wrapping); `content-length: 25` + `content-type: text/plain` headers (HCM `SendLocalReply` defaults). The `response_code_details` field `"reached_concurrency_limit"` is NOT byte-pinned (surfaces only via access-log format strings; envoy-go MVP may not implement; treat as ABSENT-by-config per §12 item A3).

- **AMEND-7 (D1 resolution — ADR-0188-candidate collapses):** Per §21.P-D1 RECOMMEND IN-PLACE §Decision AMENDMENT on ADR-0059, the BRAINSTORM-anticipated **ADR-0188-candidate collapses**. The float-valued gauge encoding convention (ns for time-typed; ×1000 for ratio-typed; 0/1 for bool-typed) anchors as an in-place §Decision AMENDMENT paragraph on ADR-0059 (the canonical Internal Stats Store architecture ADR), mirroring phase-20's IN-PLACE AMENDMENT pattern at ADR-0150 + ADR-0159. **ADR-0188 stays UNCONSUMED at phase-21 phase-done**; next-free advances only by 2 (ADR-0186 + ADR-0187) leaving next-free at **ADR-0188**. D11-style hypothesis (BRAINSTORM §7.4) STRENGTHENED: ADR-0188 + ADR-0189 stay UNCONSUMED at phase-done (two-slot escape-valve buffer forward to phase 22).

---

## 2. Non-purposes

Phase 21 is single-row per ADR-0045 (no sub-phases). It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land adaptive_concurrency under the existing 07.1 framework + the 1 in-place AMENDMENT to ADR-0059.

- **2.1 RTDS `enabled.runtime_key` runtime keying OUT OF SCOPE + PARSE-REJECT.** Per BRAINSTORM Q1 Pragmatic-middle + ADR-0187 (RTDS deferral). `enabled.runtime_key != ""` triggers HCM-parse-time PARSE-REJECT. The static-default path (`enabled.default_enabled` consumed; **absent `enabled` field treats filter as OFF per AMEND-4 + upstream BoolValue proto-default**) is the only honored arm at MVP. Closes after the Runtime/RTDS family phase lands (per ROADMAP "Runtime + hot restart family" row).
- **2.2 Cross-side byte-exact algorithmic parity OUT OF SCOPE.** Gradient formula numeric outputs depend on minRTT timing + sample-window phase + sorted-slice-vs-CircllHist percentile-aggregation choice (BRAINSTORM §8 item 4). Phase-21 ships the two-layer test taxonomy (FAKE-TIME subject-only algorithmic-fidelity + structural cross-side `0025-http-adaptive-concurrency` differential fixture with **partial cross-side byte-exact on the 503-overflow leg** per AMEND-6 + §21.P-D2 RATIFIED) as the chosen compromise. Full cross-side byte-exact algorithmic parity (gradient values + new-limit values + per-sample RTT values cross-side equal) requires reference Envoy clock injection (not available) and is documented as deferred per §8 item 2.
- **2.3 Alternative `ConcurrencyControllerConfig` oneof arms OUT OF SCOPE + RATIFIED-AS-ABSENT.** The proto oneof has **only one arm** at v1.37.2: `gradient_controller_config` (verified per §21.P-PARSE-REJECT; `adaptive_concurrency.proto:63-67`). If upstream adds a second controller type (Vegas / PI / TCP-cwnd-style) in a future release, that's a forward-pointer per §8 item 3. Phase-21's parse layer treats the oneof as effectively required-with-one-arm; PARSE-REJECT the oneof-absent case.
- **2.4 CircllHist (or HDR Hist) percentile aggregation OUT OF SCOPE.** Phase 21 uses **sorted-slice percentile aggregation** (per-tick window + per-recalc-window; ≤ `request_count` default 50 samples bounded). Upstream uses CircllHist (`gradient_controller.h:19` + `gradient_controller.h:288-289`). Numeric divergence is **≤ 1 bin-width at the percentile boundary** (CircllHist log-linear bin precision ~4%); phase-21 explicitly accepts this divergence per Q1 (Pragmatic middle) + AMEND-3 + BRAINSTORM §8 item 4. Future algorithmic-fidelity-extension task could swap to CircllHist if cross-side byte-exact algorithmic parity becomes load-bearing.
- **2.5 `min_rtt_calc_params.fixed_value` static-minRTT alternative path OUT OF SCOPE + PARSE-REJECT.** Per AMEND-1 C4 + §5.3. Upstream offers `fixed_value` as an alternative to dynamic minRTT recalc — the controller uses the configured fixed value as a constant minRTT and skips dynamic recalculation. envoy-go phase-21 PARSE-REJECTs `fixed_value` set (stricter than upstream — operators who want static-minRTT behavior must wait for a future phase). The static-path landing surface is small (~50-100 LoC) but warrants its own brainstorm given the very different controller-state-machine shape it produces.
- **2.6 `internal/clock/` framework primitive extraction OUT OF SCOPE.** Per BRAINSTORM Q3 + §3.1. The `Clock` interface stays **inline** at `internal/filter/http/adaptive_concurrency/clock.go`. Consumer count = 1 at phase-21; YAGNI per phase-17 jwt_authn EXTRACT-NOW-only-when-trigger-fires lesson. Future EXTRACT-NOW trigger fires when a second timer-driven filter (admission_control, global rate limit, or similar) lands.
- **2.7 Per-route override NEVER-DEFERRED.** The v1.37.x adaptive_concurrency proto has **NO `AdaptiveConcurrencyPerRoute` message** at all per §21.P-PARSE-REJECT + BRAINSTORM §4. Listener-scoped only; HCM-parse-time PARSE-REJECT for route-level placement (the framework's existing `typed_per_filter_config` proto-deserialization-time check catches this without any phase-21-specific code). **FOURTH CONSECUTIVE §9 row** to make this REUSE-by-absence decision (after phase 18 + phase 19 + phase 20). Permanent absence.
- **2.8 NEVER-DEFERRED — Runtime feature-flag layer.** envoy-go has no runtime-features layer per S2 (phase-20 precedent settled). Upstream's potential `envoy.reloadable_features.*` reloadable-features gates NOT modeled. MVP relies on `enabled.default_enabled` proto-field as the sole switch (per AMEND-4).
- **2.9 NEVER-DEFERRED — Multi-listener controller-state isolation explicit verification.** Each listener's HCM instantiates its own filter instance with its own `gradient_controller` state per the existing per-HCM filter-factory pattern (REUSE-by-existing-framework). A future fixture-extension task could exercise per-listener-scoped controller-state isolation explicitly (two listeners with identical config produce 2 independent controllers); phase-21's single-listener fixture is sufficient for phase-done. Documented per §8 item 6.
- **2.10 MVP confirmations (positive consumption assertions).** `sample_aggregate_percentile` IN MVP (full 0..100 range; default p50). `max_concurrency_limit` IN MVP (default 1000). `concurrency_update_interval` IN MVP (required-positive). `min_rtt_calc_params.interval` IN MVP (PGV ≥1ms when set; required iff `fixed_value` absent per AMEND-1 C3+C4 + §5.3). `request_count` IN MVP (default 50). `jitter` IN MVP (default 15%; per AMEND-2 C2 next-interval-additive semantics). `min_concurrency` IN MVP (default 3). `buffer` IN MVP (default 25%). `concurrency_limit_exceeded_status` IN MVP (default 503; per AMEND-6 byte-pinned wire shape). `enabled.default_enabled` IN MVP (default false per AMEND-4 — filter is OFF unless explicitly enabled).
- **2.11 Framework REUSES NOT consumed.** ADR-0144 `DownstreamPrincipal()` NOT consumed (no TLS-principal interaction). ADR-0150 jwks NOT consumed. ADR-0151 jwt verifier NOT consumed. ADR-0125 5th canonical NOT consumed (REUSE-by-absence per §5.4). ADR-0177 `internal/httpclient/` NOT consumed (no outbound HTTP). ADR-0178 `internal/sdsfile/` NOT consumed (no SDS surface). ADR-0165 `DecoderFilterCallbacks` extensions NOT consumed (no TLS/principal-attribute envelope to populate).

---

## 3. Framework survey result (ZERO NEW top-level primitives + ONE IN-PLACE §Decision AMENDMENT + 7 REUSES)

The framework survey evaluated REUSE of phase-04-through-20 primitives BEFORE proposing NEW (per the phase-16/17/18.x/19.x/20 discipline). Findings:

### 3.1 In-package: inline `Clock` interface seam at `clock.go` *(per BRAINSTORM Q3 — NO new framework primitive; documented in ADR-0186 §Decision)*

In-package `Clock` interface at `internal/filter/http/adaptive_concurrency/clock.go`. Public surface (settled at SPEC; the IMPL confirms the exact signature):

```go
// Clock is the controller's seam over wall-clock + timer operations. The
// in-package interface lets controller_test.go inject a deterministic
// FakeClock without depending on a framework-wide primitive.
type Clock interface {
    Now() time.Time
    AfterFunc(d time.Duration, fn func()) Stop
}

// Stop cancels a scheduled timer. Returns true if the timer was canceled
// before firing (matches time.Timer.Stop semantics).
type Stop interface {
    Stop() bool
}

// defaultClock wraps time.Now + time.AfterFunc. Used at production wiring.
type defaultClock struct{}

func (defaultClock) Now() time.Time                                 { return time.Now() }
func (defaultClock) AfterFunc(d time.Duration, fn func()) Stop      { return &timerStop{time.AfterFunc(d, fn)} }

type timerStop struct{ t *time.Timer }
func (s *timerStop) Stop() bool { return s.t.Stop() }
```

**Consumer count at introduction time: 1** (the in-package `gradientController` only). Per phase-17 jwt_authn EXTRACT-NOW-only-when-trigger-fires lesson, the seam stays inline. Forward-pointer to a future-phase EXTRACT-NOW trigger when a second timer-driven filter (admission_control, global rate limit, or similar §9 row) lands — documented in ADR-0186 §Consequences (g).

`fakeClock` lives in test scope only (~80-150 LoC at `clock_test.go` + `controller_test.go`). The fakeClock exposes `Advance(d time.Duration)` to deterministically step the clock and synchronously fire all timers whose deadline ≤ current+d. Used by `TestController_FAKE_TIME_*` algorithmic-fidelity tests per §14.1 layer A.

### 3.2 IN-PLACE: ADR-0059 §Decision AMENDMENT *(per D1 resolution — float-valued gauge int64 encoding convention)*

The ADR-0059 `internal/stats/` framework primitive (phase 06.1) gains a §Decision body **AMENDMENT paragraph** documenting the float-valued-gauge int64 encoding convention. §Decision body gains the AMENDMENT paragraph mirroring the existing "Amendment (per phase 11 ADR-0118)" precedent at ADR-0061 + the phase-20 ADR-0150 + ADR-0159 §Decision-AMENDMENT precedents. **~20-30 LoC delta** in `internal/stats/` (a sibling `boolToInt(b bool) int64` helper at `internal/stats/conv.go` (NEW small file) + comment-only `gauge.go` doc-comment extension cross-referencing the convention; no signature change to `*stats.Gauge`).

**AMENDMENT paragraph wording** (anchored at this SPEC commit; AMENDMENT body lands at IMPL Task 4):

> **Amendment (per phase 21 ADR-0186): float-valued-gauge int64 encoding convention.** The `*stats.Gauge` primitive is int64-only by §Decision above; phase-21's `gradient_controller` surface introduces three value classes that are not int64-natural and require an operator-readable encoding convention layered atop the unchanged primitive. The convention: **Time-typed gauges** (e.g., `sample_rtt_msecs`, `min_rtt_msecs` — `time.Duration` in envoy-go; `std::chrono::nanoseconds` upstream) encode as **int64 nanoseconds direct** in envoy-go via `Gauge.Set(rtt.Nanoseconds())`; operators divide by 1e9 for seconds. **NOTE on envoy-go-strict departure**: upstream encodes time-typed gauges as milliseconds via `duration_cast<milliseconds>`; envoy-go uses nanoseconds for Go-stdlib-naturalness; the byte-exact stat NAME is preserved (`sample_rtt_msecs` / `min_rtt_msecs`) and the per-metric `# HELP` text documents the unit divergence. **Ratio-typed gauges** (e.g., `gradient` — double bounded `[0.5, 2.0]` upstream) encode as **int64 ×1000** via `Gauge.Set(int64(gradient * 1000))`; gradient=1.5 → stored 1500. The ×1000 scale matches upstream Envoy's `gradient_controller.cc` integer-millis convention and gives 3 decimal places of precision over the bounded domain. **Bool-typed gauges** (e.g., `min_rtt_calculation_active`) encode as **int64 0/1** via `Gauge.Set(boolToInt(b))` where `boolToInt` is a sibling helper at `internal/stats/conv.go`. Operator-readability footnote: all three classes scrape as Prometheus integers via the unchanged `Gauge.Format()` int64-text path; the per-class divisor is documented at each metric's `# HELP` text (Rule SN6 best-effort English).

Per ADR-0044 in-place edit discipline — NOT a new ADR; ADR-0059 evolves in-place with the AMENDMENT paragraph clearly dated 2026-05-18 and cross-referenced to phase 21 + ADR-0186. **AMENDMENT-anticipation paragraph anchors at this SPEC commit**; AMENDMENT body lands at IMPL Task 4 (the `stats.go` registration callsite per §6.6).

### 3.3 Framework REUSES — 7 reuses + 6 NOT-CONSUMED items

REUSES (load-bearing per BRAINSTORM §3.2-§3.8):

- **REUSE 1: `internal/stats/` Counter + Gauge support**: `internal/stats/registry.go::Registry` + `internal/stats/counter.go::Counter` + `internal/stats/gauge.go::Gauge` already host first-class counter + gauge support. The 7-name stat surface is constructed via `Registry.NewCounter` + `Registry.NewGauge` at boot-time. Gauge support already first-class in `internal/stats/gauge.go::Gauge` over `atomic.Int64`. ADR-0059 in-place §Decision AMENDMENT per §3.2 documents the encoding convention; no framework signature change.
- **REUSE 2: HTTPRegistry boot-time registration**: `internal/filter/http/registry.go::HTTPRegistry` supports `Register(typeURL, factory)` + `Freeze` + `Lookup`. Boot wires `adaptiveconcurrency.New` at `cmd/envoy-go/main.go` between `router.New` (alphabetical first) and `bandwidthlimit.New` (alphabetical second; current line 125). **16 HTTP filters wired post-phase-21**. NOTE on package-identifier choice: envoy-go's existing precedent for multi-word canonical names uses `header_mutation` (underscored) at `internal/filter/http/header_mutation/`; phase-21 mirrors this with `adaptive_concurrency` (underscored). The Go package identifier is `adaptive_concurrency` (Go allows underscores; ADR-0114 stylistic license).
- **REUSE 3: Per-request filter interface (decode/encode hooks)**: The existing HCM filter framework already supports per-request filter instances with `DecodeHeaders` / `EncodeHeaders` / `EncodeData` / `EncodeComplete` callbacks. Adaptive_concurrency's acquire-at-decode + release-at-full-encode pattern fits cleanly without framework extension. **Decoder-and-encoder-both filter** (5th §9 row to ship encoder-side hooks after phase 13 buffer + phase 14 compressor + phase 15 bandwidth_limit + phase 19.2 ext_proc body) — needed because the per-request RTT sample is computed at full-encode completion (`onEncodeComplete()`) and appended to the controller's current concurrency-update window.
- **REUSE 4: HCM-parse-time PARSE-REJECT path**: The existing HCM parser already rejects unknown type_urls + invalid typed_config bodies at parse time. Adaptive_concurrency's PARSE-REJECT roster (per §5) layers on top via the standard `compiledConfig` constructor returning an error.
- **REUSE 5: REUSE-by-absence per-route enforcement**: The adaptive_concurrency proto has NO `AdaptiveConcurrencyPerRoute` message. Any attempt to configure per-route adaptive_concurrency is a proto-deserialization-time PARSE-REJECT via the existing HCM filter framework. Same REUSE-by-absence pattern as oauth2 / jwt_authn / extauthz / rbac listener-scoped-only at chain entry. **FOURTH CONSECUTIVE §9 row to skip ADR-0125 amendment** (extending the third-consecutive run from phase-20).
- **REUSE 6: Existing fuzzer-corpus framework**: `internal/filter/http/fuzz_test.go` already hosts the cross-filter fuzzer registry. Adaptive_concurrency adds `FuzzAdaptiveConcurrencyConfigParse` as the 27th project-wide fuzzer (current count 26 post-phase-20 → 27 post-phase-21) at the standard ~30-corpus-seed baseline + the standard PARSE-REJECT arm coverage (per §5).
- **REUSE 7: Existing differential-fixture framework**: `test/fixtures/` + `test/differential/runner_test.go` already host the differential fixture runner. Adaptive_concurrency adds `0025-http-adaptive-concurrency/` per the partial-cross-side-byte-exact pattern per AMEND-6 + §21.P-D2 RATIFIED (matching phase-20's 0024-http-oauth2 fixture template with the 503-overflow leg promoted to cross-side byte-exact). Per §7.

NOT-CONSUMED (documented for cross-phase audit clarity):

- **ADR-0144 `DownstreamPrincipal()`**: NOT CONSUMED — adaptive_concurrency has no TLS-principal interaction.
- **ADR-0150 `internal/jwks/Fetcher`**: NOT CONSUMED — no JWT validation.
- **ADR-0151 `internal/jwt/` verifier**: NOT CONSUMED — no JWT validation.
- **ADR-0177 `internal/httpclient/`**: NOT CONSUMED — no outbound HTTP. (Note: phase-20 introduced this primitive with 3 consumers at extraction time; phase-21 makes no new consumer call; the primitive's consumer count stays at 3 post-phase-21.)
- **ADR-0178 `internal/sdsfile/`**: NOT CONSUMED — no SDS surface.
- **ADR-0165 + ADR-0174 `DecoderFilterCallbacks` + `EncoderFilterCallbacks` cross-phase-reusable extensions**: NOT CONSUMED — adaptive_concurrency has no TLS/principal-attribute envelope to populate.

### 3.4 Boot-registration — alphabetical insertion at position-1 (before `bandwidthlimit`)

`cmd/envoy-go/main.go` (currently registering 15 entries post-phase-20 at lines 124-138) gains a sixteenth `httpReg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)` call. Insertion alphabetical per ADR-0100 §2.2 convention: `adaptive_concurrency` inserts at **line 125** (between `router` at line 124 — alphabetical-first by special-case + `bandwidthlimit` at line 125 → shifts to 126). The Go package identifier `adaptive_concurrency` sorts alphabetically before `bandwidthlimit` (`a` < `b`); per ADR-0072, registration order does NOT affect runtime behavior; stylistic discipline only. **NO ADR** anchored for the boot-registration insertion — straight alphabetical per the phase-09..20 convention. **NO filter-chain ordering surgery.**

### 3.5 Total framework footprint table

| Surface | Items | Anticipated LoC |
|---|---|---|
| NEW `internal/filter/http/adaptive_concurrency/` package | 10 production + 6 test Go files per §6.8 | ~1100-1400 LoC |
| IN-PLACE ADR-0059 §Decision AMENDMENT | boolToInt helper + gauge.go doc-comment cross-reference | ~20-30 LoC delta |
| Boot-registration insertion | cmd/envoy-go/main.go alphabetical line 125 | ~1 LoC |
| Differential fixture `0025-http-adaptive-concurrency` | 4 scenarios × envoy.yaml + envoy-go.yaml + expectations.yaml + README | ~250-400 LoC |
| **Subtotal framework** | | **~20-30 LoC** |
| **Subtotal filter + fixture** | | **~1350-1800 LoC** |
| **GRAND TOTAL phase 21** | | **~1370-1830 LoC** |

The LoC envelope ticks above BRAINSTORM's ~1200-1500 estimate due to the algorithmic-invariant AMENDs at AMEND-2 (extra state-machine cases for the 5-consecutive-min forced-recalc trigger + first-tick semantics + the percentile-based-not-MIN minRTT-recalc aggregation). Still **below the ADR-0045 split-gate** (LoC > 1500 OR tasks > 25): the ~1800 worst-case is bounded by the gate, and the natural split axes don't carve cleanly per BRAINSTORM §1.4 (controller state machine + FAKE-TIME tests tightly coupled; framework delta is zero). **Single-row landing settled** per the ADR-0045 split disposition.

---

## 4. State machine + algorithmic invariants (gradient_controller.cc line-cited lemmata per D3)

This section codifies the Gradient-1 controller's algorithmic invariants with **line-exact citations** against upstream `source/extensions/filters/http/adaptive_concurrency/controller/gradient_controller.cc` at v1.37.2 per ADR-0186 anchor. The line-exact citation depth is itself the D3 RATIFIED disposition per §21.P-D3.

### 4.1 In-flight token discipline (lock-free CAS at `decodeHeaders` entry)

Per upstream `gradient_controller.cc:209-233` (`forwardingDecision()` method):

```
RequestForwardingAction GradientController::forwardingDecision() {
  const uint32_t limit = concurrencyLimit();
  uint32_t current_outstanding = num_rq_outstanding_.load(std::memory_order_relaxed);
  while (current_outstanding < limit) {
    if (num_rq_outstanding_.compare_exchange_weak(current_outstanding,
                                                  current_outstanding + 1, ...)) {
      return RequestForwardingAction::Forward;
    }
  }
  stats_.rq_blocked_.inc();
  return RequestForwardingAction::Block;
}
```

envoy-go mirrors via `gradientController.forwardingDecision() (forward bool)` over an `atomic.Uint32` (`num_rq_outstanding`). On `Block`: increment `rq_blocked` counter + return `Block`; the filter then emits the configured `concurrency_limit_exceeded_status` (default 503) per AMEND-6 byte-pinned wire shape (`SendLocalReply(503, "reached concurrency limit", {Content-Type: text/plain, Content-Length: 25})`). On `Forward`: the filter proceeds with normal decode-chain dispatch + records the entry timestamp (`f.entryTime = clock.Now()`).

### 4.2 Sample recording + concurrency-update tick

**Sample recording** at `onEncodeComplete()` (the encoder-side hook): compute `rtt := clock.Now().Sub(f.entryTime)`; if controller is NOT in minRTT sampling window, append the RTT to the controller's `latencySamples []time.Duration` slice (under `sampleMutationMtx`); decrement `num_rq_outstanding`. If controller IS in minRTT sampling window, append to the recalc-window's `minRTTSamples []time.Duration` slice instead; same decrement.

**Concurrency-update tick** runs on each `concurrency_update_interval` per upstream `sample_reset_timer_` callback at `gradient_controller.cc:66-80`. Per AMEND-2 C4 first-tick semantics: at construction (if minRTT sampling enabled), the controller immediately enters the minRTT sampling window with `concurrency_limit` pinned to `min_concurrency` + `sample_reset_timer_` enabled but its callback short-circuits while `inMinRTTSamplingWindow()`; re-arming happens inside `updateMinRTT()` once the minRTT window closes (`gradient_controller.cc:149`). The tick algorithm:

1. If `inMinRTTSamplingWindow()` → bail (timer not re-enabled here; `updateMinRTT` re-enables once window closes).
2. Acquire `sampleMutationMtx` (envoy-go: `sync.Mutex` named `sampleMutationMtx`).
3. `resetSampleWindow()` per `gradient_controller.cc:162-174`: if zero samples → release mutex + re-enable timer + return.
4. `sampleRTT = processLatencySamplesAndClear()` per `gradient_controller.cc:176-182`: compute the `sample_aggregate_percentile`-quantile of `latencySamples` (envoy-go: sorted-slice quantile per §3.3 BRAINSTORM §8 item 4 acceptable divergence); clear `latencySamples`.
5. Update `sample_rtt_msecs` gauge (envoy-go: stores `sampleRTT.Nanoseconds()` per AMEND-3 C3 + ADR-0059 AMENDMENT — envoy-go-strict departure from upstream's millisecond encoding; name stays `sample_rtt_msecs` for byte-exact upstream parity).
6. `updateConcurrencyLimit(calculateNewLimit())` per §4.3 + §4.4.
7. Re-enable `sample_reset_timer_` for next `concurrency_update_interval`.

### 4.3 Gradient formula (clamp `[0.5, 2.0]`; line-cited)

Per upstream `gradient_controller.cc:190-192` (verbatim):

```
const auto buffered_min_rtt = min_rtt_.count() + min_rtt_.count() * config_.minRTTBufferPercent();
const double raw_gradient = static_cast<double>(buffered_min_rtt) / sample_rtt_.count();
const double gradient = std::max<double>(0.5, std::min<double>(2.0, raw_gradient));
```

**Clamp constants:** hard-coded `0.5` (lower) and `2.0` (upper) — NOT configurable. **Buffer application:** `buffered_min_rtt = min_rtt × (1 + min_rtt_buffer_pct)` where `min_rtt_buffer_pct` is normalized to `[0.0, 1.0]` (default 25% → 0.25; per `gradient_controller.h:96-100` + `gradient_controller.cc:46-47` accessor). The doc-comment at `gradient_controller.h:170` writes the idealized formula `gradient = minRTT / sampleRTT` (no buffer, no clamp) — the implementation differs from the doc-comment; envoy-go follows the implementation.

envoy-go mirror at `controller.go::computeGradient`:

```go
func computeGradient(minRTT, sampleRTT time.Duration, bufferPct float64) float64 {
    bufferedMinRTT := float64(minRTT) * (1.0 + bufferPct)
    raw := bufferedMinRTT / float64(sampleRTT)
    return math.Max(0.5, math.Min(2.0, raw))
}
```

Update `gradient` gauge via `Gauge.Set(int64(gradient * 1000))` per AMEND-3 C4 + ADR-0059 AMENDMENT (ratio-typed encoding).

### 4.4 New-limit calculation (sqrt-burst-headroom + double-clamp; line-cited)

Per upstream `gradient_controller.cc:198-206` (verbatim):

```
const double limit = concurrencyLimit() * gradient;
const double burst_headroom = sqrt(limit);
stats_.burst_queue_size_.set(burst_headroom);
const uint32_t new_limit = limit + burst_headroom;
return std::max<uint32_t>(config_.minConcurrency(),
                          std::min<uint32_t>(config_.maxConcurrencyLimit(), new_limit));
```

**Sqrt-headroom formula:** `burst_headroom = sqrt(currentLimit × gradient)`. Final: `newLimit = currentLimit × gradient + sqrt(currentLimit × gradient)`, double-clamped to `[min_concurrency, max_concurrency_limit]`. The implicit conversion `double → uint32_t` at `cc:204` truncates toward zero.

**`burst_queue_size` semantics correction:** the gauge stores `sqrt(currentLimit × gradient)` = the **burst headroom** added to the new limit calculation — NOT actual in-flight queued requests above the limit (BRAINSTORM §5.1 semantics row was loose; this section pins the empirical meaning). Operator interpretation: "the controller's per-tick burst-buffer above the multiplicatively-scaled limit."

envoy-go mirror at `controller.go::calculateNewLimit`:

```go
func (c *gradientController) calculateNewLimit(gradient float64) uint32 {
    limit := float64(c.concurrencyLimit()) * gradient
    burstHeadroom := math.Sqrt(limit)
    c.stats.burstQueueSize.Set(int64(burstHeadroom))
    newLimit := uint32(limit + burstHeadroom)
    if newLimit < c.cfg.minConcurrency { return c.cfg.minConcurrency }
    if newLimit > c.cfg.maxConcurrencyLimit { return c.cfg.maxConcurrencyLimit }
    return newLimit
}
```

### 4.5 minRTT recalc window (per AMEND-2 — percentile-aggregation NOT MIN; jitter to next-interval delay; 5-consecutive-min forced trigger; first-tick semantics)

**Trigger paths (two):**

1. **Timer-scheduled periodic** per `gradient_controller.cc:64`: `min_rtt_calc_timer_` set at construction; after each successful `updateMinRTT()` re-armed at `cc:147-148`:
   ```
   min_rtt_calc_timer_->enableTimer(applyJitter(config_.minRTTCalcInterval(), config_.jitterPercent()));
   ```
2. **Forced (5-consecutive-min path)** per AMEND-2 C3 + `gradient_controller.cc:281-283`:
   ```
   if (consecutive_min_concurrency_set_ >= 5 && !inMinRTTSamplingWindow() &&
       config_.isMinRTTSamplingEnabled()) {
     min_rtt_calc_timer_->enableTimer(std::chrono::milliseconds(0));
   }
   ```
   Force-arms `min_rtt_calc_timer_` to 0ms when `updateConcurrencyLimit` is called with `new_limit == old_limit == min_concurrency` for **5 consecutive ticks** outside a minRTT window. Guards against a stale-high minRTT pinning the system at min_concurrency.

**Jitter semantics** per AMEND-2 C2 + `gradient_controller.cc:152-160`:

```
std::chrono::milliseconds GradientController::applyJitter(std::chrono::milliseconds interval,
                                                          double jitter_pct) const {
  if (jitter_pct == 0) { return interval; }
  const uint32_t jitter_range_ms = std::ceil(interval.count() * jitter_pct);
  return std::chrono::milliseconds(interval.count() + (random_.random() % jitter_range_ms));
}
```

Additive uniform `[0, interval × jitter_pct)` applied to the **next-interval delay** (not to the recalc-window length nor to the window-start time directly). Default 15%. Next minRTT window starts in `[interval, interval × (1 + jitter_pct))`.

**Recalc-window behavior** per `enterMinRTTSamplingWindow` (`cc:100-126`) + `updateMinRTT` (`cc:128-150`):

(a) Save current limit to `deferred_limit_value_`;
(b) Clamp `concurrency_limit_` to `config_.minConcurrency()`;
(c) Clear sample histogram (envoy-go: clear `minRTTSamples` slice);
(d) Set `min_rtt_epoch_`;
(e) `recordLatencySample` adds samples while in-window;
(f) Once sample count ≥ `min_rtt_aggregate_request_count_` (default 50), `updateMinRTT` calls `processLatencySamplesAndClear()` which takes the **`sample_aggregate_percentile` quantile** (default p50) per AMEND-2 C1 + `cc:176-182` (NOT the MIN as BRAINSTORM hypothesized);
(g) Set `min_rtt_`, update `min_rtt_msecs` gauge (envoy-go: stores `minRTT.Nanoseconds()` per AMEND-3 C3 — envoy-go-strict departure);
(h) Set `min_rtt_calculation_active` gauge to 0 (was 1 during window);
(i) Restore `deferred_limit_value_` to `concurrency_limit_`;
(j) Re-arm both timers (`min_rtt_calc_timer_` per applyJitter; `sample_reset_timer_` per `concurrency_update_interval`).

### 4.6 Initial state + first-tick semantics (per AMEND-2 C4)

Per `gradient_controller.cc:55-92` constructor:

```
concurrency_limit_(config_.minConcurrency()),
deferred_limit_value_(0),
num_rq_outstanding_(0),
// ...
if (config_.isMinRTTSamplingEnabled()) {
  enterMinRTTSamplingWindow();
} else {
  min_rtt_ = config_.fixedValue();
}
```

envoy-go MUST mirror: `concurrencyLimit = config.minConcurrency`; `deferredLimitValue = 0`; `numRqOutstanding = 0`; `minRTT = 0` (Go-zero `time.Duration`). If `isMinRTTSamplingEnabled` (always TRUE at phase-21 MVP since `fixed_value` is PARSE-REJECTed per §5.3 + AMEND-1 C4), the constructor immediately calls `enterMinRTTSamplingWindow()` per AMEND-2 C4 first-tick semantics. **No gradient is computed during the initial minRTT window** — the controller is pinned at `min_concurrency` until the first minRTT-recalc completes.

---

## 5. PARSE-REJECT roster (HCM-parse-time)

### 5.1 RATIFIED-from-PGV arms (proto-level rejection — would be PGV-rejected upstream too)

| Arm | PGV rule (upstream) | envoy-go-error wording |
|---|---|---|
| `concurrency_controller_config` oneof absent | `(validate.required) = true` (proto:63-67) | `"adaptive_concurrency: concurrency_controller_config oneof required"` |
| `concurrency_limit_params` absent (required message) | `(validate.rules).message = {required: true}` (proto:56-57) | `"adaptive_concurrency: concurrency_limit_params required"` |
| `min_rtt_calc_params` absent (required message) | `(validate.rules).message = {required: true}` (proto:59) | `"adaptive_concurrency: min_rtt_calc_params required"` |
| `concurrency_update_interval` absent or 0 | `duration = {required: true, gt {}}` (proto:33-36) | `"adaptive_concurrency: concurrency_update_interval must be > 0"` |
| `max_concurrency_limit == 0` (when set) | `uint32 = {gt: 0}` (proto:32) | `"adaptive_concurrency: max_concurrency_limit must be > 0"` |
| `min_concurrency == 0` (when set) | `uint32 = {gt: 0}` (proto:46) | `"adaptive_concurrency: min_concurrency must be > 0"` |
| `request_count == 0` (when set) | `uint32 = {gt: 0}` (proto:44) | `"adaptive_concurrency: request_count must be > 0"` |
| `min_rtt_calc_params.interval < 1ms` (when set) | `duration = {gte {nanos: 1000000}}` (proto:42) | `"adaptive_concurrency: min_rtt_calc_params.interval must be >= 1ms"` |
| `sample_aggregate_percentile` out of `[0, 100]` | `type.v3.Percent` PGV (proto:54) | `"adaptive_concurrency: sample_aggregate_percentile must be in [0, 100]"` |
| `jitter.value > 100` | `type.v3.Percent` PGV (proto:45) | `"adaptive_concurrency: jitter must be in [0, 100]"` |
| `buffer.value > 100` | `type.v3.Percent` PGV (proto:47) | `"adaptive_concurrency: buffer must be in [0, 100]"` |

Per phase-11 ADR-0115 + phase-15 ADR-0136 + phase-16 ADR-0141 envoy-go-defensive-PGV-mirror precedent — envoy-go does its own validation rather than relying on go-control-plane PGV middleware to guarantee the deserialized message is valid. Byte-stable error wording per ADR-0080.

### 5.2 envoy-go-strict project-local arms (stricter than upstream)

| Arm | envoy-go behavior | envoy-go-error wording | ADR anchor |
|---|---|---|---|
| `enabled.runtime_key != ""` | PARSE-REJECT (defers RTDS RuntimeFeatureFlag to a future Runtime/RTDS family phase) | `"adaptive_concurrency: enabled.runtime_key is not yet supported; use enabled.default_enabled"` | ADR-0187 |
| `min_rtt_calc_params.fixed_value` set | PARSE-REJECT (defers static-minRTT alternative path per AMEND-1 C4) | `"adaptive_concurrency: min_rtt_calc_params.fixed_value is not yet supported; use min_rtt_calc_params.interval"` | ADR-0186 §Consequences (d) |
| Per-route placement (any non-listener-level `typed_per_filter_config` map entry for adaptive_concurrency) | PARSE-REJECT via REUSE-by-absence — the proto has NO `AdaptiveConcurrencyPerRoute` message; the HCM parser's `typed_per_filter_config` deserializer fails because the wire bytes can't be deserialized | Existing HCM-parse-time error path (per §5.4) | (no NEW ADR — REUSE-by-absence per §5.4) |

The PARSE-REJECT discipline at `enabled.runtime_key` is stricter than upstream (upstream honors the runtime_key + falls back to `default_enabled`); the deferral is operator-visible per ADR-0187 §Decision body wording at IMPL.

### 5.3 `fixed_value` disposition (PARSE-REJECT per AMEND-1 C4)

The `min_rtt_calc_params` proto allows TWO alternative paths for the minRTT source:
- `interval` field (dynamic recalc per §4.5) — the standard Gradient-1 path
- `fixed_value` field (static minRTT; controller skips dynamic recalc entirely)

Phase-21 PARSE-REJECTs `fixed_value` set. Rationale: (a) the static-path controller-state-machine shape is materially different (no minRTT recalc window; no 5-consecutive-min forced-trigger; no jitter); (b) the operator surface for static-minRTT is small (~50-100 LoC) but warrants its own brainstorm; (c) PARSE-REJECT-now-defer-the-brainstorm matches the BRAINSTORM Q1 Pragmatic-middle posture for proto-defined-but-not-MVP fields. Closes after a future fixture-extension or adaptive_concurrency-static-minRTT phase per §8 item 5.

### 5.4 Per-route REUSE-by-absence — FOURTH CONSECUTIVE §9 row to skip ADR-0125 amendment

The adaptive_concurrency proto defines **NO `AdaptiveConcurrencyPerRoute` message** in v1.32.4 or v1.37.x (verified per §21.P-PARSE-REJECT). Per-route adaptive_concurrency configuration is a proto-deserialization-time PARSE-REJECT via the existing HCM filter framework. The HCM parser's existing `typed_per_filter_config` deserializer fails because the wire bytes can't be deserialized as `AdaptiveConcurrency`. No phase-21-specific PARSE-REJECT code; no framework extension.

**Classification:** REUSE-by-absence. Phase 21 makes this the **FOURTH CONSECUTIVE §9 row** to skip ADR-0125 (per-route canonical roster) amendment (after phase 18 + phase 19 + phase 20). Phase 21 extends the run; the absence itself becomes a recurring pattern across §9 listener-scoped-only filters.

**No new canonical entry** in ADR-0125 §canonical-per-route-roster. **No ADR-0125 amendment** anchored at phase 21.

---

## 6. compiledConfig + code shapes (IMPL blueprint)

### 6.1 `compiledConfig` shape

```go
type compiledConfig struct {
    // Outer envelope
    enabled                       bool // honored: enabled.default_enabled (per AMEND-4 default FALSE when absent)
    concurrencyLimitExceededStatus uint32 // honored: concurrency_limit_exceeded_status.code (default 503)

    // GradientControllerConfig sub-surface (the only oneof arm per AMEND-1 C1; field renamed)
    sampleAggregatePercentile float64 // [0.0, 1.0]; default 0.50
    maxConcurrencyLimit       uint32  // default 1000
    concurrencyUpdateInterval time.Duration // required > 0

    // min_rtt_calc_params sub-block (per AMEND-1 C3 + C4: fixed_value PARSE-REJECTed; interval required)
    minRTTCalcInterval time.Duration // PGV >= 1ms; required at phase-21 (fixed_value PARSE-REJECTed)
    minRTTRequestCount uint32        // default 50
    minRTTJitterPct    float64       // [0.0, 1.0]; default 0.15
    minRTTMinConcurrency uint32      // default 3
    minRTTBufferPct      float64     // [0.0, 1.0]; default 0.25
}
```

`buildCompiledConfig(typedConfig *anypb.Any) (*compiledConfig, error)` performs the PARSE-REJECT roster per §5.1 + §5.2 + §5.3 with byte-stable error wording per ADR-0080.

### 6.2 `gradientController` state shape

```go
type gradientController struct {
    cfg   *compiledConfig
    stats *filterStats
    clock Clock

    // Hot-path atomics (lock-free)
    concurrencyLimit  atomic.Uint32 // current limit; init = cfg.minRTTMinConcurrency per §4.6
    numRqOutstanding  atomic.Uint32 // current in-flight count; init = 0

    // Cold-path state (under sampleMutationMtx)
    mu                              sync.Mutex
    latencySamples                  []time.Duration // appended on encodeComplete when NOT in minRTT window
    minRTTSamples                   []time.Duration // appended on encodeComplete when IN minRTT window
    minRTT                          time.Duration   // last computed minRTT
    deferredLimitValue              uint32          // saved limit during minRTT window; 0 = NOT in minRTT window
    consecutiveMinConcurrencySet    uint32          // 5-consecutive-min counter per AMEND-2 C3
    sampleResetTimer                Stop            // periodic concurrency-update tick
    minRTTCalcTimer                 Stop            // periodic minRTT-recalc trigger
}
```

`newGradientController(cfg, stats, clock)` per AMEND-2 C4 first-tick semantics: sets initial `concurrencyLimit = cfg.minRTTMinConcurrency`; calls `enterMinRTTSamplingWindow()` immediately; enables `sampleResetTimer` (its callback short-circuits while in-window per §4.5).

### 6.3 `clock.go` interface seam (inline per §3.1)

Per §3.1 — in-package `Clock` interface (`Now()` + `AfterFunc`) with `defaultClock` production wiring + `fakeClock` test-scope-only implementation.

### 6.4 `decode_headers.go` — acquire-at-decode

```go
func (f *filter) DecodeHeaders(headers Headers, endStream bool) FilterStatus {
    if !f.cc.enabled {
        return Continue
    }
    if forward := f.controller.forwardingDecision(); !forward {
        // 503 overflow path per AMEND-6 byte-pinned wire shape
        f.cb.SendLocalReply(int(f.cc.concurrencyLimitExceededStatus),
                            "reached concurrency limit",
                            map[string]string{"content-type": "text/plain"},
                            nil, // grpc_status absent
                            "reached_concurrency_limit") // response_code_details — not emitted at MVP
        return StopAndBuffer
    }
    f.entryTime = f.clock.Now()
    f.acquired = true
    return Continue
}
```

### 6.5 `encode_complete.go` — release-at-encode-complete + sample recording

```go
func (f *filter) OnEncodeComplete() {
    if !f.acquired { return }
    rtt := f.clock.Now().Sub(f.entryTime)
    f.controller.recordLatencySample(rtt) // routes to latencySamples or minRTTSamples per §4.2
    f.controller.releaseInFlight()        // decrements numRqOutstanding
    f.acquired = false
}
```

### 6.6 `stats.go` — 7-name roster (per AMEND-3)

```go
type filterStats struct {
    rqBlocked                 *stats.Counter // 1 counter
    concurrencyLimit          *stats.Gauge   // 6 gauges
    gradient                  *stats.Gauge   // int64 ×1000 (per ADR-0059 AMENDMENT)
    burstQueueSize            *stats.Gauge
    sampleRTTMsecs            *stats.Gauge   // int64 ns (envoy-go-strict per AMEND-3 C3)
    minRTTMsecs               *stats.Gauge   // int64 ns (envoy-go-strict per AMEND-3 C3)
    minRTTCalculationActive   *stats.Gauge   // int64 0/1
}

func newFilterStats(reg *stats.Registry, hcmPrefix string) *filterStats {
    p := hcmPrefix + ".adaptive_concurrency.gradient_controller."
    return &filterStats{
        rqBlocked:               reg.NewCounter(p + "rq_blocked"),
        concurrencyLimit:        reg.NewGauge(p + "concurrency_limit"),
        gradient:                reg.NewGauge(p + "gradient"),
        burstQueueSize:          reg.NewGauge(p + "burst_queue_size"),
        sampleRTTMsecs:          reg.NewGauge(p + "sample_rtt_msecs"),
        minRTTMsecs:             reg.NewGauge(p + "min_rtt_msecs"),
        minRTTCalculationActive: reg.NewGauge(p + "min_rtt_calculation_active"),
    }
}
```

Stat-prefix template per AMEND-3 C2: `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.<stat>` (the `http.<HCM_stat_prefix>` is HCM-injected via the `hcmPrefix` constructor argument per ADR-0143 SN2-reuse). Project stat count: **92 → 99** (+1 counter +6 gauges).

### 6.7 `fuzz_test.go` — 27th project-wide fuzzer

`FuzzAdaptiveConcurrencyConfigParse` at `internal/filter/http/adaptive_concurrency/fuzz_test.go` (~50 LoC). Corpus seed roster ~30 entries covering:
- Valid full Gradient-1 config (all sub-blocks present + valid)
- Each PARSE-REJECT arm per §5.1 + §5.2 + §5.3 (12 arms × valid-edge-cases ≈ 24 seeds)
- Empty config; oneof-absent; nested-message-missing variants

Must-never-panic across `buildCompiledConfig`. Clean at 30s per seed per the standard fuzzer-corpus framework (REUSE 6 per §3.3).

### 6.8 Source-file roster (~10 production + 6 test = 16 Go files)

| File | Purpose | Anticipated LoC |
|---|---|---|
| `internal/filter/http/adaptive_concurrency/doc.go` | Package doc | ~20 |
| `internal/filter/http/adaptive_concurrency/adaptive_concurrency.go` | TypeURL constant + `New` factory + `HTTPFilter` value | ~80-120 |
| `internal/filter/http/adaptive_concurrency/compiled_config.go` | `compiledConfig` shape + `buildCompiledConfig` + PARSE-REJECT roster per §5 | ~200-280 |
| `internal/filter/http/adaptive_concurrency/controller.go` | `gradientController` state machine per §4 + §6.2 | ~350-450 |
| `internal/filter/http/adaptive_concurrency/clock.go` | `Clock` interface + `defaultClock` per §3.1 + §6.3 | ~30-60 |
| `internal/filter/http/adaptive_concurrency/decode_headers.go` | `DecodeHeaders` per §6.4 | ~50-80 |
| `internal/filter/http/adaptive_concurrency/encode_complete.go` | `OnEncodeComplete` + `OnDestroy` per §6.5 | ~50-80 |
| `internal/filter/http/adaptive_concurrency/stats.go` | `filterStats` shape + `newFilterStats` per §6.6 | ~50-80 |
| `internal/filter/http/adaptive_concurrency/filter.go` | per-stream `filter` struct + glue | ~60-100 |
| `internal/filter/http/adaptive_concurrency/percentile.go` | Sorted-slice percentile aggregation (per BRAINSTORM §8 item 4 carve-out) | ~50-80 |
| `internal/filter/http/adaptive_concurrency/adaptive_concurrency_test.go` | `TypeURL` + `New` integration tests | ~80-120 |
| `internal/filter/http/adaptive_concurrency/compiled_config_test.go` | PARSE-REJECT table-driven tests per §14.1 | ~250-350 |
| `internal/filter/http/adaptive_concurrency/controller_test.go` | FAKE-TIME algorithmic-fidelity tests per §14.1 Layer A | ~400-550 |
| `internal/filter/http/adaptive_concurrency/clock_test.go` | `fakeClock` implementation (test-scope) + unit tests | ~80-150 |
| `internal/filter/http/adaptive_concurrency/percentile_test.go` | Sorted-slice quantile vector tests | ~80-120 |
| `internal/filter/http/adaptive_concurrency/fuzz_test.go` | 27th fuzzer per §6.7 | ~50 |
| `internal/stats/conv.go` (NEW small file per §3.2 AMENDMENT) | `boolToInt` helper + comment cross-reference | ~10-15 |
| **Subtotal `internal/filter/http/adaptive_concurrency/`** | | **~1850-2700 LoC** (incl. tests) |
| **Subtotal production-only** | | **~940-1320 LoC** |
| **Subtotal tests** | | **~890-1390 LoC** |

Production-to-test ratio ~1.0 (matches phase-20 oauth2 ~1.0-1.3 envelope per phase-20 SPEC §14.1).

---

## 7. Differential fixture envelope — `0025-http-adaptive-concurrency`

### 7.1 Fixture shape (4 scenario directories + partial cross-side byte-exact on 503-overflow leg)

Per BRAINSTORM §6.1 + AMEND-6 + §21.P-D2 RATIFIED — REFERENCE-LESS subject-only structural for 3 scenarios + **partial cross-side byte-exact for 1 scenario** (the 503-overflow leg). 4 scenario directories at `test/fixtures/0025-http-adaptive-concurrency/`:

| Scenario | Disposition | Wire-level expectation |
|---|---|---|
| **(a) `parse_ok`** | REFERENCE-LESS subject-only structural | adaptive_concurrency filter loads with full Gradient-1 config; admin `/stats` exposes the 7-name surface; HTTP 200 to a normal GET |
| **(b) `overflow_503`** | **partial cross-side byte-exact per AMEND-6 + §21.P-D2** | Config: `max_concurrency_limit=1 + min_concurrency=1 + concurrency_limit_exceeded_status=503`. 2 concurrent slow requests (serialized connection-establishment ordering — request 2 dispatched after request 1's first byte is observed in the upstream). Request 1 → 200 OK; Request 2 → **503 + body `"reached concurrency limit"` (25 bytes verbatim) + `content-type: text/plain` + `content-length: 25`** (byte-pinned against both Envoy v1.37.2 AND envoy-go subjects per §21.P-D2 RATIFIED) |
| **(c) `stat_surface`** | REFERENCE-LESS subject-only structural | Admin `/stats` exposes the full 7-name surface with expected starting values (concurrency_limit at `min_concurrency`; gradient gauge present though value depends on first-tick semantics; min_rtt_calculation_active = 1 during initial window) |
| **(d) `pass_through_when_disabled`** | REFERENCE-LESS subject-only structural | Config: `enabled` absent (or `enabled.default_enabled=false`) per AMEND-4 default-OFF semantics. All requests pass through; no 503; `rq_blocked` counter stays at 0 |

### 7.2 503-overflow cross-side byte-exact promotion per AMEND-6 + §21.P-D2 RATIFIED

The 503-overflow leg promotion is anchored at AMEND-6. Per §21.P-D2 RATIFIED evidence:

- **Trap soundness**: `min_concurrency=1 + max_concurrency_limit=1` HARD-clamps the effective limit at 1 in every phase (constructor + minRTT-window-entry + every `calculateNewLimit` clamp at `gradient_controller.cc:153-154`). No grace window; no first-tick bypass.
- **CAS arrival-order**: `forwardingDecision()` CAS at `gradient_controller.cc:209-233` resolves arrival order at hardware-atomic granularity; first-arriving request Forwards (CAS 0→1 wins), second blocks deterministically (CAS-loser observes `current_outstanding=1`, loop falls through to Block).
- **503 wire shape**: `decoder_callbacks_->sendLocalReply(503, "reached concurrency limit", nullptr, absl::nullopt, "reached_concurrency_limit")` at `adaptive_concurrency_filter.cc:50-54`. HCM `SendLocalReply` defaults populate `content-type: text/plain` + `content-length: 25` headers.
- **`response_code_details` NOT byte-pinned**: surfaces only via access-log format strings; envoy-go MVP may not implement; treat as ABSENT-by-config per §12 item A3.

The scenario (b) fixture pins all 4 byte-exact items (status + body + 2 headers) against BOTH Envoy v1.37.2 reference + envoy-go subject. This is the **first §9 family-row to land a cross-side byte-exact fixture leg with timing-dependent setup** (the slow upstream is the testbed serialization mechanism; the 2-concurrent-request configuration trips the trap deterministically once the limit-clamp invariant holds).

### 7.3 Listener topology

Single listener with a single HCM containing the adaptive_concurrency filter (placed alphabetical-first in the filter chain) + router terminator. Backend is a synthetic slow-response cluster (1-second response latency for scenario (b); fast-response for (a) + (c) + (d)). No multi-listener topology at phase-21 (per §2.9 + §8 item 6 deferred).

### 7.4 Six-gate checklist (A/B/C/D/E/F) per §7.5 BRAINSTORM §1.1

Identical to phase-20 oauth2 + phase-19.x ext_proc + phase-17 jwt_authn six-gate matrix:

- **Gate A — build**: `go build ./...` clean
- **Gate B — vet + lint**: `go vet ./...` + `golangci-lint run` clean; no new suppressions
- **Gate C — race**: `go test -race ./...` clean across all packages including the new `internal/filter/http/adaptive_concurrency/` + `internal/stats/` (post-AMENDMENT regression)
- **Gate D — differential**: 26/26 differential fixtures GREEN (0000-0024 pre-existing + 0025 new); cross-side byte-exact on the 503-overflow leg per scenario (b)
- **Gate E — fuzz**: `FuzzAdaptiveConcurrencyConfigParse` clean at 30s per seed; no panics across the 27 project-wide fuzzers
- **Gate F — h2spec**: 53/53 PASS at ADR-0051 pin

---

## 8. Deferred items (~8 items; comparable to phase-20's 6-8 register)

The following are explicitly NOT delivered at phase 21 and are documented forward-pointers carried into a future phase.

1. **RTDS `enabled.RuntimeFeatureFlag` runtime keying** — PARSE-REJECT at config-load per AMEND-4 + ADR-0187. Closes after the Runtime/RTDS family phase lands (per ROADMAP "Runtime + hot restart family" row).

2. **Cross-side byte-exact algorithmic parity** — gradient formula numeric outputs depend on minRTT timing + sample-window phase + sorted-slice-vs-CircllHist percentile-aggregation choice. Phase-21's two-layer strategy (FAKE-TIME subject-only + structural cross-side with partial-byte-exact on the 503-overflow leg per AMEND-6) is the chosen compromise. Full cross-side byte-exact algorithmic parity requires reference Envoy clock injection (not available) or a CircllHist-equivalent percentile-aggregation swap. Deferred indefinitely (no current consumer demand).

3. **Alternative `ConcurrencyControllerConfig` oneof arms** — the proto oneof has only one defined arm (`GradientControllerConfig`) in v1.32.4 + v1.37.x per §21.P-PARSE-REJECT RATIFIED. If upstream adds a second controller type (Vegas / PI / TCP-cwnd-style) in a future release, phase 21 would need an oneof-fallthrough extension. Currently a no-op forward-pointer.

4. **CircllHist (or HDR Hist) percentile aggregation** — phase 21 uses sorted-slice percentile per `percentile.go` + §6.8 (request_count default 50 + tunable). Upstream uses CircllHist (log-linear histogram with ~4% bin precision per `gradient_controller.h:19, 288-289`). Numeric divergence is **≤ 1 bin-width at the percentile boundary** per BRAINSTORM §8 item 4 + AMEND-3; phase-21 explicitly accepts this divergence per Q1 Pragmatic-middle. A future algorithmic-fidelity-extension task could swap to CircllHist if cross-side byte-exact algorithmic parity becomes load-bearing.

5. **`min_rtt_calc_params.fixed_value` static-minRTT alternative path** — PARSE-REJECT at config-load per AMEND-1 C4 + §5.3 + ADR-0186 §Consequences (d). Closes via a future fixture-extension or adaptive_concurrency-static-minRTT phase brainstorm.

6. **Multi-listener controller-state-isolation explicit verification** — per §2.9. Each listener's HCM instantiates its own filter instance with its own `gradient_controller` state per the existing per-HCM filter-factory pattern; a future fixture-extension task could exercise this explicitly. Phase-21's single-listener fixture is sufficient for phase-done.

7. **`min_rtt_calculation_active` `Accumulate` import-mode cross-restart-state preservation parity** — upstream marks this gauge with `Accumulate` import mode (cross-restart-state preservation per AMEND-3 C5). envoy-go has no hot-restart surface at MVP (per ADR-0009 absence); the import-mode distinction is forward-pointer only. Closes when envoy-go gains hot-restart (per ROADMAP "Hot restart family" row).

8. **`response_code_details` emission** — upstream surfaces the access-log slot `"reached_concurrency_limit"` on the 503-overflow leg (`adaptive_concurrency_filter.cc:50-54`). envoy-go MVP has no access-log surface; the field is treated as ABSENT-by-config per §12 item A3. Closes when envoy-go gains access-log support (per ROADMAP "Observability family" row).

---

## 9. Cross-references against phase-17 + phase-18 + phase-19 + phase-20 deferred-items lists — closure pickup

Phase-21 picks up **ZERO closures** from prior phases' deferred-items lists. The various per-phase deferral lists (phase-17 jwt_authn carry-forwards; phase-18.x ext_authz carry-forwards; phase-19.1+19.2 ext_proc carry-forwards including the 18+ item §8 deferral list; phase-20 oauth2 6-8 future-work register) continue unchanged through phase 21 — phase 21 lands a NEW filter (adaptive_concurrency) and does NOT pick up cross-filter deferred items.

**Notable: the ADR-0159 §Future Work CLOSURE-AT-PHASE-20 milestone (the FIRST §9 family-row to CLOSE a prior-phase load-bearing forward-pointer) is NOT extended at phase 21.** Phase 21 closes NO prior forward-pointer because it introduces NO new framework primitives that would trigger one. This is healthy for the project's accretion shape: not every phase needs a CLOSURE-AT milestone, and forced-closure pickups would distort the EXTRACT-NOW trigger discipline.

The phase-20 future-work items that come CLOSEST to phase-21 relevance are: (a) the **HMAC `domain` empty-string subtlety** (phase-20 REVIEW §6 item iv) — purely an oauth2-package concern; phase 21 leaves alone. (b) The **2-listener vs 3-listener topology** for phase-20's `l_test_b disable_token_encryption=true` scenario — blocked by go-control-plane v1.32.4 proto field absence; phase 21 doesn't touch. (c) The **6 reviewer observations** from phase-20 REVIEW §6 — all oauth2-package concerns; phase 21 leaves alone.

---

## 10. ADR anchor map (2 NEW + 1 IN-PLACE AMENDMENT; D11 hypothesis STRENGTHENED)

Per ADR-0044 ADR-on-impl convention: ADR-0186 + ADR-0187 §Context drafts anchor at this SPEC commit; §Decision + §Consequences bodies land at each ADR's Lands-in-Task at IMPL. The 1 in-place AMENDMENT-anticipation paragraph at ADR-0059 anchors at this SPEC commit; AMENDMENT body lands at IMPL Task 4.

### A. 2 NEW ADRs (ADR-0186, ADR-0187)

| ADR | Subject | Anchors §§ | Lands-in-Task |
|---|---|---|---|
| **ADR-0186** | Gradient-1 controller state machine + inline `Clock` seam (NOT framework primitive) + FAKE-TIME differential strategy + sorted-slice percentile aggregation (NOT CircllHist; ≤ 1 bin-width divergence acceptable) + gradient formula + new-limit calculation + minRTT recalc with `sample_aggregate_percentile`-quantile (NOT MIN per AMEND-2 C1) + jitter additive-to-next-interval-delay (per AMEND-2 C2) + 5-consecutive-min forced-recalc trigger (per AMEND-2 C3) + first-tick semantics (per AMEND-2 C4) + line-cited algorithmic lemmata against `gradient_controller.cc` per D3 RATIFIED + `fixed_value` PARSE-REJECT (per §Consequences (d)) | §3.1; §4; §6.1-6.3; §1.1 AMEND-2 + AMEND-6 | Task 3 (controller materialization) |
| **ADR-0187** | RTDS `enabled.RuntimeFeatureFlag` deferral PARSE-REJECT (static-default honored; `runtime_key != ""` triggers HCM-parse-time PARSE-REJECT with forward-pointer to the future Runtime/RTDS family phase) + `enabled` empty-default OFF semantics (per AMEND-4 — refutes BRAINSTORM "absent enabled = ON" claim) | §5.2; §1.1 AMEND-4 | Task 2 (compiled_config) |

### B. 1 IN-PLACE §Decision AMENDMENT (per D1 resolution → ADR-0188-candidate COLLAPSES)

| ADR | AMENDMENT scope | Lands-in-Task |
|---|---|---|
| **ADR-0059** | §Decision body gains AMENDMENT paragraph documenting the float-valued-gauge int64 encoding convention per §3.2 (ns for time-typed — envoy-go-strict departure from upstream's milliseconds; ×1000 for ratio-typed; 0/1 for bool-typed); +20-30 LoC delta (NEW `internal/stats/conv.go` `boolToInt` helper + comment-only `gauge.go` cross-reference) | Task 4 (stats.go materialization) |

### C. ADR-0044 escape-valve reserve

~0-2 impl-time-unanticipated ADRs per phase. Phase 21's most-likely surfaces (all SPEC-time CLOSED via §3 + §4 + §6 + §14): sorted-slice-percentile edge cases at sample-window boundaries (samples count = 1 edge case; quantile of empty-slice edge case); fakeClock determinism race-test edge cases (timer-fire ordering when multiple timers expire same tick); ADR-0059 §Decision AMENDMENT scope-creep (whether `Format()` int64-text path needs per-class-divisor `# HELP` emission). The escape-valve reserve is held against ORTHOGONAL surfaces.

### D. Anchor map summary

| Disposition | Count | ADR numbers |
|---|---|---|
| NEW ADR §Context drafts | 2 | ADR-0186; ADR-0187 |
| IN-PLACE §Decision AMENDMENT-anticipation | 1 | ADR-0059 (per D1 resolution) |
| ADR-0125 amendments | 0 | NONE — REUSE-by-absence per §5.4 (FOURTH CONSECUTIVE §9 row) |
| ADR-0044 escape-valve reserve | 0-2 | reserved at ADR-0188+ if fired |

**Next-free ADR post-SPEC commit:** **ADR-0188** (only 2 NEW consumed: ADR-0186 + ADR-0187; D1 collapsed the BRAINSTORM-anticipated ADR-0188 candidate per §3.2 IN-PLACE AMENDMENT route).

**D11-style hypothesis STRENGTHENED at SPEC commit**: ADR-0188 + ADR-0189 stay UNCONSUMED at phase-21 phase-done (next-free escape valve carried forward TWO slots to phase 22; the D1 resolution collapsed the BRAINSTORM-anticipated ADR-0188 candidate, so the escape-valve buffer is now 2 slots wide instead of 1). Per ADR-0044 escape-valve hypothesis: HOLD-with-known-risk, not GUARANTEED-HOLD. If a surprise lands at IMPL (analogous to phase-20 Task 5/8 hand-off items or HMAC `domain` empty-string subtlety), it may force ADR-0188 consumption — the buffer absorbs one such event without breaking the next-phase escape-valve invariant.

---

## 11. Empirical-pin block reference (5 pins resolved at this SPEC session)

### A. Pin disposition matrix

| Pin | Disposition | Wire-level finding | ADR anchor |
|---|---|---|---|
| **§21.P1** 503-overflow cross-side promotion (D2) | RATIFIED | Trap sound: `min=max=1` HARD-clamps in every phase; CAS arrival-order deterministic; body `"reached concurrency limit"` (25 bytes); `content-type: text/plain`; `content-length: 25`. PROMOTE 503-overflow leg to partial cross-side byte-exact per AMEND-6. | ADR-0186; AMEND-6 |
| **§21.P2** Algorithmic invariants (D3) | PARTIAL | Gradient formula RATIFIED (clamp [0.5, 2.0]; buffer application × (1 + bufferPct)); new-limit RATIFIED (sqrt-burst-headroom; double-clamp); minRTT recalc AMEND (percentile not MIN; jitter additive-to-next-interval; 5-consec forced trigger; first-tick semantics); sorted-slice-vs-CircllHist divergence acceptable per BRAINSTORM §8 item 4. ADR-0186 anchored with line-exact lemmata citations. | ADR-0186; AMEND-2 |
| **§21.P3** Stat surface (D4) | PARTIAL | 3 stat-name renames: `sampled_rtt`→`sample_rtt_msecs`; `min_rtt`→`min_rtt_msecs`; `recalculating_min_rtt`→`min_rtt_calculation_active`. RTT in MILLISECONDS upstream (envoy-go-strict departure: nanoseconds). `gradient × 1000` int64 RATIFIED. `min_rtt_calculation_active` Accumulate import mode (deferred per §8 item 7). Stat-prefix template SIMPLIFIED (no `<config_stat_prefix>` segment exists in proto v1.37.2). | ADR-0186; AMEND-3 |
| **§21.P4** PARSE-REJECT + defaults + `enabled` semantics + ConcurrencyControllerConfig oneof | PARTIAL | Proto field rename `concurrency_limit_calculation_params` → `concurrency_limit_params`. No separate `gradient_controller.proto` (inline in `adaptive_concurrency.proto`). `enabled` defaults OFF when absent (REFUTES BRAINSTORM). `min_rtt_calc_params.interval` not proto-required (`fixed_value` alternative); PGV `gte: 1ms`. 5 additional PGV PARSE-REJECT arms BRAINSTORM missed (per AMEND-5). `concurrency_controller_config` oneof solo (only `gradient_controller_config` arm at v1.37.2; RATIFIED-AS-ABSENT for alternative arms per §8 item 3). | ADR-0186; ADR-0187; AMEND-1; AMEND-4; AMEND-5 |
| **§21.P5** Float-gauge encoding convention anchor (D1) | RESOLVED-to-AMENDMENT (no new ADR consumed) | IN-PLACE §Decision AMENDMENT on ADR-0059 (NOT new ADR-0188; the BRAINSTORM-anticipated ADR-0188 candidate COLLAPSES). Convention: ns for time-typed; ×1000 for ratio-typed; 0/1 for bool-typed. ~20-30 LoC delta in `internal/stats/` (boolToInt helper + comment cross-reference; no signature change). | ADR-0059 §Decision AMENDMENT; AMEND-7 |

### B. Pin disposition summary

| Disposition | Count |
|---|---|
| RATIFIED | 1 |
| PARTIAL (with AMEND blocks) | 3 |
| RESOLVED-to-IN-PLACE-AMENDMENT (no new ADR consumed) | 1 |
| **TOTAL** | **5** |

All 5 pins CLOSED at SPEC time. **No RATIFIED-PENDING-IMPL-TIME pin disposition deferred to IMPL phase** at the pin-disposition level (the §12 residual byte-confirmations are SUB-PIN-LEVEL refinements, not deferred pin closures).

### C. Pin-to-AMEND-block traceability

| AMEND-N | Sources | Recipient ADRs |
|---|---|---|
| AMEND-1 | §21.P4 (proto field rename + proto-file consolidation + interval validation nuance + fixed_value disposition) | ADR-0186 |
| AMEND-2 | §21.P2 (4 sub-AMENDs: percentile-aggregation; jitter; 5-consec trigger; first-tick) | ADR-0186 |
| AMEND-3 | §21.P3 (5 sub-AMENDs: stat-name renames; template simplification; RTT-ms-vs-ns envoy-go-strict departure; gradient encoding; Accumulate import mode deferral) | ADR-0186 |
| AMEND-4 | §21.P4 enabled-default-OFF (REFUTES BRAINSTORM §2.1) | ADR-0187 |
| AMEND-5 | §21.P4 PARSE-REJECT roster expansion (5 additional PGV arms) | ADR-0186 |
| AMEND-6 | §21.P1 RATIFIED 503 wire shape byte-exact promotion | ADR-0186 |
| AMEND-7 | §21.P5 D1 resolution — ADR-0188-candidate COLLAPSES to ADR-0059 §Decision AMENDMENT | ADR-0059 |

---

## 12. Deferred decisions (the planner / implementer settles these)

8 RATIFIED-PENDING-IMPL-TIME items — sub-pin-level refinements of already-closed pins, settled at IMPL Tasks 2-13 + Task 14 (six-gate verification). None block phase-21 phase-done.

### A. Wire-shape byte-confirmation items (4)

1. **503 body `"reached concurrency limit"` 25-byte byte-exact** — §11 §21.P1 + AMEND-6. Settles at IMPL Task 12 fixture-0025 scenario (b) cross-side byte-comparison.
2. **503 `content-type: text/plain` + `content-length: 25` header byte-exact** — §11 §21.P1 + AMEND-6. Settles at IMPL Task 12 fixture-0025 scenario (b) cross-side header-comparison.
3. **`response_code_details "reached_concurrency_limit"` ABSENT-by-config disposition** — §11 §21.P1. Settles at IMPL Task 12 by NOT-byte-pinning the field (envoy-go MVP has no access-log surface; treat as ABSENT-by-config).
4. **`min_rtt_calculation_active` Accumulate import-mode divergence acknowledgement** — §11 §21.P3 + AMEND-3 C5. Settles at IMPL Task 13 BEHAVIOR_CONTRACT.md envoy-go-strict departure record (forward-pointer only; no behavioral check).

### B. Library-behavioral confirmation items (3)

5. **Sorted-slice quantile numeric divergence vs CircllHist** — §8 item 4 + AMEND-3. Settles at IMPL Task 3 `percentile_test.go` vector tests (cross-quantile-against-known-reference; accept ≤ 1 bin-width difference at percentile boundary).
6. **fakeClock timer-fire determinism under multi-timer same-tick** — ADR-0186 + §C escape-valve surface. Settles at IMPL Task 3 `controller_test.go` race tests + `clock_test.go` deterministic-ordering tests.
7. **CAS-vs-mutex contention behavior at scale** — §4.1. Settles at IMPL Task 3 race tests `TestController_ConcurrentForwardingDecision_*` (no deadlock; arrival-order determinism holds under N concurrent forwarders).

### C. Cross-phase regression-window items (1)

8. **Cross-package regression for ADR-0059 §Decision AMENDMENT** — §3.2 + §9. Settles at IMPL Task 4 (`stats.go` materialization) + Task 14 six-gate (Gate C race tests for `internal/stats/` post-AMENDMENT regression). Expected outcome: zero regression (AMENDMENT is pure convention-extension; `boolToInt` helper addition; no signature change to `*stats.Gauge`).

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052; lands at phase-21 phase-done)

7-edit bundle landing at the IMPL phase-done commit per ADR-0052. None at this SPEC commit. Edits:

### A. NEW top-level subsection (1)

1. **NEW `### envoy.filters.http.adaptive_concurrency` subsection** inserted after `### envoy.filters.http.oauth2` (current line 1948). Subsections: filter scope; populated-vs-deferred field map (full GradientControllerConfig sub-surface; PARSE-REJECT for `enabled.runtime_key` + `min_rtt_calc_params.fixed_value`); state-machine summary (first-tick; concurrency-update tick; minRTT recalc with percentile-aggregation + 5-consec trigger); 7-name stat surface; deny-path wire shape (503 + `"reached concurrency limit"` body); listener-scoped discipline; envoy-go-strict departures (RTT-ms-vs-ns per AMEND-3 C3; sorted-slice-vs-CircllHist per BRAINSTORM §8 item 4; CAS-vs-upstream-impl). Anticipated ~150-250 LoC.

### B. Per-section additions in existing subsections (2)

2. **Stat-name mapping 92-name → 99-name table extension** — 1 new counter (`rq_blocked`) + 6 new gauges (per AMEND-3). Table caption updated. Per-row units annotated (ns for envoy-go RTT gauges; ×1000 for gradient; raw-uint32 for limit/burst_queue_size; 0/1 for active flag).
3. **ADR-0059 §Decision AMENDMENT cross-reference** — extends the existing `## Internal Stats Store` umbrella subsection with a paragraph noting the float-valued-gauge int64 encoding convention added at phase-21 per AMEND-7.

### C. NEW envoy-go-strict departure records (2)

4. **RTT-gauge units divergence** per AMEND-3 C3: envoy-go uses nanoseconds while upstream uses milliseconds; stat NAMES preserve upstream byte-exact (`sample_rtt_msecs` / `min_rtt_msecs`); per-metric `# HELP` text disambiguates the unit.
5. **Sorted-slice-vs-CircllHist percentile-aggregation divergence** per BRAINSTORM §8 item 4 + AMEND-3: numeric outputs may differ by ≤ 1 bin-width at the percentile boundary; gradient values + new-limit values + sampled-RTT values are NOT cross-side byte-exact (only the 503-overflow wire shape is — per AMEND-6).

### D. Phase-21 forward-pointer notes (1)

6. **NEW `### Phase 21 forward-pointer notes` subsection** placed immediately after `### Phase 20 forward-pointer notes`. Documents: RTDS runtime keying (§8 item 1); cross-side byte-exact algorithmic parity (§8 item 2); alternative ConcurrencyControllerConfig oneof arms (§8 item 3); CircllHist percentile-aggregation upgrade (§8 item 4); `fixed_value` static-minRTT path (§8 item 5); multi-listener controller-state-isolation explicit verification (§8 item 6); `min_rtt_calculation_active` Accumulate import-mode parity (§8 item 7); `response_code_details` emission (§8 item 8).

### E. Per-route canonical patterns cross-reference table update (1)

7. **Per-route canonical patterns cross-reference table caption update** — "updated through phase 20" → "updated through phase 21"; phase-21 cross-reference paragraph added documenting the REUSE-by-absence (FOURTH CONSECUTIVE §9 row; no roster extension; the absence-as-recurring-pattern note).

### F. Edit-bundle summary

| Category | Count |
|---|---|
| NEW top-level subsection | 1 |
| Per-section additions | 2 |
| NEW envoy-go-strict departure records | 2 |
| Phase-21 forward-pointer notes | 1 |
| Per-route canonical patterns cross-reference table update | 1 |
| **TOTAL** | **7** |

Anticipated total LoC delta: ~250-350 LoC added (current size ~3058; post-phase-21 ~3300-3400). All 7 edits land at the SAME IMPL commit per ADR-0052; none mutate pre-phase-21 paragraphs (in-place-by-append discipline).

---

## 14. Testing strategy

### 14.1 Unit tests — two-layer taxonomy (Layer A subject-only algorithmic-fidelity + Layer B cross-side fixture)

Test surface at `internal/filter/http/adaptive_concurrency/*_test.go` per §6.8. Anticipated ~890-1390 LoC test code (production-to-test ratio ~1.0; matches phase-20 envelope).

**Layer A — Subject-only algorithmic-fidelity tests via FakeClock** (per BRAINSTORM Q2 + §3.1):

1. **`TestController_FAKE_TIME_FirstTickSemantics`** — per AMEND-2 C4: controller immediately enters minRTT window at construction; concurrency-update tick callback short-circuits while in-window; first gradient computation only after first minRTT recalc.
2. **`TestController_FAKE_TIME_GradientFormula_*`** — per §4.3 + AMEND-2: vector tests for gradient computation under varying sample-RTT / min-RTT / buffer combinations; verify clamp at 0.5 + 2.0; verify buffer application × (1 + bufferPct).
3. **`TestController_FAKE_TIME_NewLimitCalculation_*`** — per §4.4: vector tests for newLimit computation; verify sqrt-burst-headroom; verify double-clamp at min_concurrency + max_concurrency_limit.
4. **`TestController_FAKE_TIME_MinRTTRecalcWindow_*`** — per §4.5 + AMEND-2 C1: verify minRTT recalc takes the `sample_aggregate_percentile`-quantile (default p50) of recalc-window samples, NOT MIN. Vector tests with crafted sample sets.
5. **`TestController_FAKE_TIME_JitterApplication_*`** — per AMEND-2 C2: verify jitter is additive `[0, interval × jitter_pct)` to the next-interval delay (not to the recalc-window length).
6. **`TestController_FAKE_TIME_FiveConsecutiveMinForcedRecalc`** — per AMEND-2 C3: verify the 5-consecutive-min forced-recalc trigger fires `min_rtt_calc_timer_` at 0ms.
7. **`TestController_ConcurrentForwardingDecision_*`** — race tests per §12 item B7: N concurrent forwarders against limit=K; verify exactly K succeed + N-K block; verify arrival-order determinism under serialized connection-establishment.
8. **`TestController_503_BodyAndHeaders_ByteExact`** — per AMEND-6 + §12 items A1+A2: verify the 503 body `"reached concurrency limit"` (25 bytes) + `content-type: text/plain` + `content-length: 25` against the subject-only emission.
9. **`TestPercentile_SortedSlice_*`** — per §6.8 `percentile_test.go`: vector tests for sorted-slice quantile aggregation; verify p50/p90/p99 against known references; verify edge cases (empty slice; single-sample slice).
10. **`TestBuildCompiledConfig_PARSE_REJECT_*`** — table-driven PARSE-REJECT tests per §5.1 + §5.2 + §5.3. ~20-30 table rows. Byte-stable error wording per ADR-0080 + the §5.1 + §5.2 + §5.3 wording table.

**Layer B — Structural cross-side differential fixture** (per BRAINSTORM Q2 + §7).

Per §7.1 — `0025-http-adaptive-concurrency/` with 4 scenarios; scenario (b) `overflow_503` with partial cross-side byte-exact pinning per AMEND-6 + §21.P-D2 RATIFIED. Scenarios (a) + (c) + (d) REFERENCE-LESS subject-only structural.

### 14.2 Race detector + lint

- `go test -race ./...` clean across all packages including the new `internal/filter/http/adaptive_concurrency/` + `internal/stats/` (post-AMENDMENT regression test surface)
- `TestController_ConcurrentForwardingDecision_*` + `TestController_FAKE_TIME_*` show zero race violations
- `go vet ./...` clean
- `golangci-lint run` clean (no new suppressions)
- `go build ./...` clean

### 14.3 Fuzzer

27th fuzzer `FuzzAdaptiveConcurrencyConfigParse` per §6.7. Must-never-panic discipline across `buildCompiledConfig`. Clean at 30s per seed.

### 14.4 h2spec + differential

- h2spec 53/53 PASS at ADR-0051 pin (no regression)
- Differential fixture `0025-http-adaptive-concurrency` per §7 — 4 scenarios; cross-side byte-exact on scenario (b) 503-overflow leg per AMEND-6
- Cross-package regression matrix per §12 item C8 (post-ADR-0059 AMENDMENT)

### 14.5 Six-gate checklist

Per §7.4 — gates A/B/C/D/E/F as the load-bearing IMPL Task N verification. All MUST be GREEN for the row-21 status flip.

---

## 15. Acceptance checklist (for the reviewer)

The phase-21 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following against the landed artefacts. All 18 items MUST be GREEN for row-21 status flip from `in-progress` to `done`.

### A. Six-gate verification (6 items — atomic GREEN per gate)

1. **Gate A — build**: `go build ./...` clean across `internal/filter/http/adaptive_concurrency/`, `internal/stats/` (post-AMENDMENT), all pre-existing packages
2. **Gate B — vet + lint**: `go vet ./...` + `golangci-lint run` clean; no new lint suppressions
3. **Gate C — race**: `go test -race ./...` clean; zero data-race violations across all packages including `internal/stats/` (post-ADR-0059 AMENDMENT regression suite)
4. **Gate D — differential**: 26/26 differential fixtures GREEN (0000-0024 pre-existing + 0025 new); scenario (b) 503-overflow leg cross-side byte-exact per AMEND-6 + §21.P-D2; cross-package regression matrix GREEN
5. **Gate E — fuzz**: `FuzzAdaptiveConcurrencyConfigParse` clean at 30s per seed; no panics across the 27 project-wide fuzzers
6. **Gate F — h2spec**: 53/53 PASS at ADR-0051 pin

### B. Fixture-0025 4-scenario coverage (1 item — atomic GREEN over 4 scenarios)

7. **Fixture-0025 scenario matrix** per §7.1 — all 4 scenario directories landed: (a) parse_ok REFERENCE-LESS; (b) overflow_503 PARTIAL CROSS-SIDE BYTE-EXACT (503 + 25-byte body + 2 headers); (c) stat_surface REFERENCE-LESS; (d) pass_through_when_disabled REFERENCE-LESS

### C. 7-name stat-surface verification (1 item — atomic GREEN over 7 names + envoy-go-strict departure record)

8. **7-name stat-surface byte-exact** per ADR-0186 + AMEND-3 + §11 §21.P3: 1 counter (`rq_blocked`) + 6 gauges (`concurrency_limit`, `gradient`, `burst_queue_size`, `sample_rtt_msecs`, `min_rtt_msecs`, `min_rtt_calculation_active`) all registered under `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.<stat>` prefix; project stat count 92 → 99; envoy-go-strict departure for RTT gauges in nanoseconds (vs upstream milliseconds) documented at BEHAVIOR_CONTRACT §13 item C4

### D. Gradient-1 algorithmic-fidelity verification (1 item — atomic GREEN over 7 Layer A test families)

9. **Gradient-1 algorithmic-fidelity** per §14.1 Layer A: TestController_FAKE_TIME_FirstTickSemantics + TestController_FAKE_TIME_GradientFormula_* + TestController_FAKE_TIME_NewLimitCalculation_* + TestController_FAKE_TIME_MinRTTRecalcWindow_* (with percentile-aggregation NOT MIN per AMEND-2 C1) + TestController_FAKE_TIME_JitterApplication_* (per AMEND-2 C2) + TestController_FAKE_TIME_FiveConsecutiveMinForcedRecalc (per AMEND-2 C3) + TestController_ConcurrentForwardingDecision_* — all GREEN

### E. PARSE-REJECT roster verification (1 item — atomic GREEN over §5.1 RATIFIED-PGV + §5.2 envoy-go-strict + §5.3 fixed_value arms)

10. **PARSE-REJECT roster** per §5 + ADR-0187: 11 RATIFIED-from-PGV arms per §5.1 + 3 envoy-go-strict arms per §5.2 + the `fixed_value` deferral per §5.3 — all with byte-stable error wording per ADR-0080 + TestBuildCompiledConfig_PARSE_REJECT_* table-driven coverage

### F. Byte-exact 503 wire shape confirmation (1 item — atomic GREEN over body + 2 headers + cross-side at scenario b)

11. **Byte-exact 503 wire shape** per §11 §21.P1 + AMEND-6: scenario (b) emits status `503 Service Unavailable` + body `"reached concurrency limit"` (25 bytes verbatim; no trailing newline) + `content-type: text/plain` + `content-length: 25`; cross-side byte-comparison against Envoy v1.37.2 reference passes

### G. ADR landing (2 items)

12. **2 NEW ADR §Context drafts + §Decision + §Consequences bodies landed** at per-Task Lands-in-Tasks: ADR-0186 (controller state machine + line-cited lemmata) + ADR-0187 (RTDS deferral + enabled-default-OFF AMEND-4)
13. **1 IN-PLACE §Decision AMENDMENT body landed at IMPL Task 4**: ADR-0059 §Decision AMENDMENT (float-valued gauge encoding convention; `internal/stats/conv.go` `boolToInt` helper + `gauge.go` cross-reference)

### H. BEHAVIOR_CONTRACT.md edit-bundle (1 item)

14. **7-edit BEHAVIOR_CONTRACT.md bundle landed at IMPL Task 13** per §13 (atomic landing per ADR-0052)

### I. DECISIONS + STATE + ROADMAP advance (3 items)

15. **DECISIONS.md final-state alignment** at IMPL Task 11: 2 NEW ADRs + 1 IN-PLACE AMENDMENT at final state; cross-references intact; next-free ADR-0188 unconsumed
16. **STATE.md re-advanced** at IMPL Task 14: `active-phase` updated per phase-done convention; `lifecycle-state: phase 21 IMPL done`; `next-skill: superpowers:brainstorming` (or per next-phase identity); `last-commit` SHA-fill placeholder; `next-free ADR: ADR-0188`
17. **ROADMAP.md row 21 flipped** to `done` at IMPL Task 14: per-cell IMPL-done annotation added; row stays single-row per ADR-0045

### J. Audit-trail verification (1 item)

18. **End-to-end audit-trail** at phase-done review: SPEC → PLAN → PROGRESS → REVIEW chain landed; per-task PROGRESS records map 1:1 to PLAN tasks; each §11 pin + each §12 item recorded at PROGRESS + REVIEW; D11 hypothesis disposition recorded (ADR-0188 + ADR-0189 stay UNCONSUMED at phase-done — strengthened two-slot buffer); six-gate verbatim outputs at REVIEW

---
