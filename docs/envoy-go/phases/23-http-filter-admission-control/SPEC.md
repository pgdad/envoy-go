# Phase 23 SPEC — `envoy.filters.http.admission_control`

> **Lifecycle state:** SPEC.md authored; ROADMAP row `23` already at `in-progress` (registered at the phase-23 BRAINSTORM commit per the phase-20-established BRAINSTORM-time row-registration precedent; per-cell narrative updated at this SPEC commit with the SPEC-done annotation; status stays `in-progress` until IMPL phase-done with all 6 gates GREEN). Per ADR-0045 the SPEC author settled the split disposition: **SINGLE-ROW landing** (no sub-rows `23.1`/`23.2`; precedent at phase-14 compressor, phase-17 jwt_authn, phase-20 oauth2, phase-21 adaptive_concurrency single-row landings). LoC envelope re-estimated post-empirical-scrape at ~1000-1400 (slightly above the BRAINSTORM's ~900-1300 envelope due to the empirical-scrape AMENDs — the `std::deque`-of-per-second-buckets sliding window per AMEND-6, the gRPC-trailers classification path per AMEND-10, and the integer-modulo decision plumbing per AMEND-2 — but well below the ADR-0045 split-gate, and LOWER than phase-21). Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09–22 precedent. This SPEC is the authoritative input to the phase-23 PLAN.
>
> **Predecessor:** `docs/envoy-go/phases/23-http-filter-admission-control/BRAINSTORM.md` (the 2-question Q1-Q2 settled context + 5 precedent-settled defaults + the §3 framework-survey result + the §8 9-item deferred-items register + the §10 4 SPEC-time D-questions D1-D4). The §10 empirical pins are resolved in this SPEC's §11; the §1.1 amendment-block records the BRAINSTORM corrections (AMEND-1..AMEND-11) driven by the empirical scrape. NO off-master prebrainstorm-notes branch was authored for phase 23.
>
> **Scope (per BRAINSTORM §1.1 + the SPEC-time empirical-pin scrape):** phase 23 lands `envoy.extensions.filters.http.admission_control.v3.AdmissionControl` (the canonical Envoy v1.37.2 SRE-book client-side admission-control filter) as the SIXTEENTH §9 family-row under the 07.1 framework, returning to the **LEAN framework-delta posture of phase 21** (ZERO new `internal/` framework primitives, after phase-22's substantial `internal/lua/` + `internal/dynamicmetadata/` primitives). MVP envelope: the FULL operator surface minus RTDS — `success_criteria` (BOTH `http_criteria` + `grpc_criteria` arms) + `sampling_window` + `aggression` + `sr_threshold` + `rps_threshold` + `max_rejection_probability`; honoring the static default value of each `Runtime{FeatureFlag,Double,Percent,UInt32}` wrapper; PARSE-REJECT for any non-empty `runtime_key` inside each of the four wrappers (deferring the RTDS keying to a future Runtime/RTDS family phase per ADR-0195). **Two inline interface seams** in the filter package: a `Clock` seam (mirroring phase-21; in-package, NOT a shared `internal/clock/` primitive) + a NEW `Rand` seam (per AMEND-2 the seam exposes `Uint64()` — NOT `Float64()` — to faithfully mirror upstream's integer-modulo reject decision). **Listener-scoped only** (REUSE-by-absence per §5.4 — no `AdmissionControlPerRoute` proto message exists in v1.37.2). **FIRST ADR-0125-skip since phase-22's roster amendment** (8 → 9 at 22.3); canonical-per-route roster STAYS 9. **Downstream HCM filter only** at MVP (per AMEND-9 — the dual-factory upstream-HTTP-filter-chain registration is deferred).
>
> **ADR continuity:** Phase 22 closed at ADR-0193 (full body) + ADR-0125 §(xiv) amended; next-free `ADR-0194`. Phase 23 anticipates **2 NEW ADRs** (ADR-0194 + ADR-0195) + **ZERO IN-PLACE §Decision AMENDMENTs** + **ZERO ADR-0125 amendments**. §Context drafts anchor at this SPEC commit (appended to `DECISIONS.md` per ADR-0044 §Context-draft discipline); §Decision + §Consequences bodies land at each ADR's Lands-in-Task at IMPL. **Next-free ADR after phase-23 SPEC commit advances `ADR-0194` → `ADR-0196`** (2 numbers consumed: ADR-0194 + ADR-0195). **D-style hypothesis** (BRAINSTORM §7.3): ADR-0196 stays UNCONSUMED at phase-23 phase-done — HOLD-with-known-risk, not GUARANTEED-HOLD (a surprise upstream stat name, a 4th counter, or a clamp-boundary float edge at IMPL could force ADR-0196 consumption). The empirical scrape CLOSED every BRAINSTORM-anticipated escape-valve surface in-§Decision at ADR-0194, so the buffer is one slot wide.
>
> **Authored:** 2026-05-21.

---

## 1. Purpose

Phase 23 lands `envoy.extensions.filters.http.admission_control.v3.AdmissionControl` — the canonical Envoy v1.37.2 admission-control filter implementing the SRE-book client-side probabilistic request-rejection algorithm: over a sliding `sampling_window` of `{requests, successes}` counts, it computes a rejection probability and probabilistically short-circuits requests with an HTTP 503 to shed load when the downstream success rate drops — under the 07.1 framework, as the SIXTEENTH §9 production HTTP filter. It establishes the entire `internal/filter/http/admission_control/` package, the per-HCM-instance sliding-window success-rate controller (a `std::deque`-of-per-second-buckets mirror per AMEND-6), the inline `Clock` interface seam (`clock.go`; NO new `internal/clock/` framework primitive per BRAINSTORM §2.3) + the NEW inline `Rand` interface seam (`rand.go`; NO new `internal/rand/` framework primitive per BRAINSTORM §2.2; `Uint64()` shape per AMEND-2), the 3-counter stat surface (`rq_rejected` / `rq_success` / `rq_failure` per AMEND-3; names byte-exact-upstream), the 32nd project-wide fuzzer `FuzzAdmissionControlConfigParse`, the two differential fixture directories `0030-http-admission-control` (cross-side, deterministic regimes) + `0031-http-admission-control-boot-reject` (boot-reject), and the listener-scoped-only enforcement (REUSE-by-absence per §5.4 — FIRST ADR-0125-skip since phase-22's roster amendment). **It introduces ZERO new `internal/` framework primitives** (returning to phase-21's LEAN posture) and adds ZERO in-place §Decision AMENDMENTs.

**3 architectural primitives that make this work:**

1. **NEW `internal/filter/http/admission_control/` package** owning the filter + controller implementation. Package directory + Go-package identifier are both `admission_control` (Go-style underscored; matches `header_mutation` + `adaptive_concurrency` precedent — the §9 filters with a multi-word canonical name; ADR-0114 stylistic license). ~9 production Go files + ~6 test files per §6.8. Anticipated ~1000-1400 LoC filter+controller proper. Exposes `TypeURL` (canonical `"type.googleapis.com/envoy.extensions.filters.http.admission_control.v3.AdmissionControl"`) + `New` (the `HTTPFilterFactory`). ADR-0194 codifies the package shape + the admission-control algorithm + both inline seams + the deterministic-regime differential strategy, line-cited against upstream `admission_control.cc` + `thread_local_controller.cc` + `evaluators/success_criteria_evaluator.cc`.

2. **NEW in-package `Rand` interface seam** at `internal/filter/http/admission_control/rand.go` per BRAINSTORM §2.2 + AMEND-2. ~25-50 LoC. `Rand` interface (`Uint64() uint64` — mirroring upstream `Random::RandomGenerator::random()` which returns a `uint64`) with `defaultRand` wrapping `math/rand/v2`. **NOT** `Float64()` (AMEND-2 REFUTES the BRAINSTORM hypothesis: upstream's reject decision is an integer-modulo comparison `(accuracy · P) > (r % accuracy)` with `accuracy = 1e4`, NOT a float `random() < P`). NOT extracted as `internal/rand/` framework primitive (consumer count = 1; YAGNI per phase-17 jwt_authn EXTRACT-NOW-only-when-trigger-fires lesson). `fakeRand` lives in test scope only (deterministic injected `uint64` for the exact-boundary algorithmic tests per §14.1 Layer A).

3. **NEW in-package `Clock` interface seam** at `internal/filter/http/admission_control/clock.go` per BRAINSTORM §2.3 (mirrors phase-21 §3.1 verbatim in spirit; the shape differs — phase-23 needs only `Now()` + window-bucket expiry, NOT phase-21's `AfterFunc` timer cascade). ~25-50 LoC. `Clock` interface (`Now() time.Time`) with `defaultClock` wrapping `time.Now`. The sliding success-rate window expires per-second buckets by monotonic wall-clock per AMEND-6. **Consumer count for a Clock-shaped seam reaches 2** (phase-21 adaptive_concurrency + phase-23 admission_control) — the documented EXTRACT-NOW trigger threshold — BUT the two seams live in different packages with materially different shapes (phase-21 = `Now()` + `AfterFunc`; phase-23 = `Now()` only over a monotonic clock), so a premature shared `internal/clock/` extraction would over-fit; phase-23 records the convergence question as a forward-pointer (§8 item 8), NOT an obligation. `fakeClock` lives in test scope only (deterministic step-driven window-bucket expiry for the algorithmic-fidelity unit-test layer per §14.1).

After phase 23, the project has the foundational admission-control filter: a both-sides per-request filter that, on `DecodeHeaders`, checks `filterEnabled()` + the `healthCheck()` short-circuit (per AMEND-4), short-circuits with `Continue` (no record) when the windowed average RPS is below `rps_threshold` (per AMEND-1 + the `admission_control.cc:87-91` suppression gate), otherwise computes `P_reject` over the sliding `{requests, successes}` window via the empirically-pinned formula `P_reject = max(0, min(max_rejection_probability, ((n − s/sr_threshold) / (n + 1))^(1/aggression)))` (line-cited per ADR-0194 against `admission_control.cc:161-179`), draws an integer from the inline `Rand` seam and rejects iff `(1e4 · P_reject) > (r % 1e4)` (per AMEND-2), and on a reject increments `rq_rejected` + emits the byte-exact local reply (503, **empty body**, `response_code_details = "denied_by_admission_control"`, no added headers, no grpc_status — per AMEND-7 + D4). On `EncodeHeaders` (+ `EncodeTrailers` for the gRPC-status-in-trailers case per AMEND-10) it classifies the upstream response per `success_criteria` (HTTP default = all codes `< 500`; gRPC default = the 11-code well-known set — per AMEND-5) and records `{success|failure}` into the current window bucket (`rq_success` / `rq_failure`), EXCEPT for rejected / health-check / disabled-filter requests which are deliberately not recorded (per AMEND-11). It PARSE-REJECTs any non-empty `runtime_key` inside the four `Runtime*` wrappers (per ADR-0195). Observable-outcomes byte-equivalent to reference Envoy v1.37.2 admission_control on the **P_reject=0 all-admit healthy-backend leg** (RNG-independent per AMEND-2: `0 > (r % 1e4)` is false for every `r`, so a healthy backend never rejects on either side — byte-exact cross-side per §7), and stat-name byte-equivalent on the 3-counter surface (per AMEND-3 + §11 D1). The probabilistic reject path is NOT cross-side byte-exact (intrinsically un-matchable against a foreign RNG; the forced-reject byte-exact promotion is impractical per AMEND-2 + §11 D2) — its byte shape is asserted subject-only against the SPEC-time D4 empirical capture.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §10 empirical pins (executed at this SPEC session via parallel-subagent fan-out against v1.37.2 reference Envoy source on GitHub at tag `v1.37.2` + the v1.32.4 go-control-plane proto bindings in the local module cache) generated the following 11 amendment-block entries — load-bearing record of empirical-scrape-driven design revisions to the BRAINSTORM. **Five are substantive REFUTATIONS** (AMEND-1 formula; AMEND-2 integer-modulo decision + `Rand` seam shape; AMEND-4 `enabled`-absent-ENABLED; AMEND-7 empty reject body; AMEND-8 boot-reject condition).

- **AMEND-1 (rejection-probability formula — REFUTES BRAINSTORM §1.1 + §2.1, driven by §11 D3):** The BRAINSTORM hypothesized `P_reject = max(0, (n − aggression⁻¹·k)/(n+1))` with `sr_threshold` as a separate "success-rate floor gate". The empirical scrape REFUTES this on three counts (`admission_control.cc:161-179`):
  - **`aggression` is an EXPONENT, not a linear multiplier.** Upstream applies `probability = std::pow(probability, 1.0 / aggression)` to the whole clamped fraction (`admission_control.cc:170-171`), and only when `aggression != 1.0` (the `pow` is skipped for the default).
  - **`sr_threshold` DIVIDES the success count inside the numerator.** The numerator is `total_requests − successful_requests / sr_threshold` (`admission_control.cc:167`), where `sr_threshold = min(pct, 100) / 100.0` (a fraction; `admission_control.cc:61-64`). It is **NOT** a separate reject-or-not gate (the only standalone `sr_threshold` check is the boot-time `< 1.0%` validation per AMEND-8).
  - **`aggression` is floored to 1.0.** `aggression() = std::max<double>(1.0, configured)` (`admission_control.cc:57-59`; default 1.0 at `:30`), so `1/aggression ∈ (0, 1]` always.
  - **Corrected formula (as implemented):** `P_reject = max(0, min(max_rejection_probability, ((n − s/sr_threshold) / (n + 1))^(1/aggression)))`, where `n = total_requests`, `s = successful_requests`, `max_rejection_probability = pct/100` (default 0.80; clamp via `std::min` at `:173`), and the `max(0, …)` floor is applied at the decision site (`:178`).

- **AMEND-2 (reject decision is INTEGER-MODULO; `Rand` seam is `Uint64()` not `Float64()` — REFUTES BRAINSTORM §2.2, driven by §11 D2 + D3):** The reject decision is NOT a float comparison `random() < P`. It is (`admission_control.cc:175-178`):
  ```cpp
  static constexpr uint64_t accuracy = 1e4;
  auto r = config_->random().random();          // uint64
  return (accuracy * std::max(probability, 0.0)) > (r % accuracy);
  ```
  `r % accuracy ∈ [0, 9999]`; the comparison is strict `>`. **Consequences:** (a) the inline `Rand` seam exposes `Uint64() uint64` (mirroring `Random::RandomGenerator::random()`), and the filter computes `float64(accuracy) * math.Max(P, 0.0) > float64(r % accuracy)` with `accuracy = 10000` — a faithful byte-for-byte mirror that lets the deterministic unit tests pin the exact reject/admit boundary (`r % 1e4 == floor(1e4·P)` is the admit/reject knife-edge). (b) When `P_reject = 0` (healthy backend, all-success window), `0 > (r % 1e4)` is **false for every `r`** ⇒ the all-admit leg is fully RNG-INDEPENDENT ⇒ byte-exact cross-side (per §7.1). (c) Forced-reject byte-exactness requires `1e4·P > 9999`, i.e. `P > 0.9999`; with an all-failure window (`s = 0`, `aggression = 1.0`, `max_rejection_probability = 100%`) `P = N/(N+1)`, so `N ≥ 10000` recorded failures are needed in a single window on a single worker — impractical and fragile in a differential harness — so the **forced-reject leg stays SUBJECT-ONLY structural** (per §11 D2; the BRAINSTORM's "promotable" default hypothesis is refuted on practicality grounds). The N≥10000 theoretical promotability is recorded as a forward-pointer (§8 item 2).

- **AMEND-3 (stat-name correction — driven by §11 D1):** Third counter is **`rq_failure`**, NOT `rq_error` (the BRAINSTORM hypothesis). Verified via `ALL_ADMISSION_CONTROL_STATS(COUNTER)` macro at `admission_control.h:35-38`. The roster is exactly **3 counters** (`rq_rejected`, `rq_success`, `rq_failure`); the macro takes only a `COUNTER` argument (no `GAUGE`/`HISTOGRAM` parameters), so **NO gauges/histograms** — the 107 → 110 stat-count delta (+3 counters) HOLDS. The filter's own prefix contribution is the literal infix `"admission_control."` (`config.cc:29`), with the HCM supplying the `http.<HCM_stat_prefix>.` root; full template `http.<HCM_stat_prefix>.admission_control.<stat>` RATIFIED (no `gradient_controller.`-style sub-infix; no config-driven stat-prefix segment).

- **AMEND-4 (`enabled`-absent ⇒ ENABLED — OPPOSITE of phase-21; driven by §11 D-followup A):** The BRAINSTORM did not pin the `enabled` default. The empirical scrape establishes the OPPOSITE of phase-21 adaptive_concurrency: an absent `enabled` message ⇒ filter **ENABLED** (per the proto doc comment "If the message is unspecified, the filter will be enabled" + the mechanism `default_value_(PROTOBUF_GET_WRAPPED_OR_DEFAULT(feature_flag_proto, default_value, true))` at `runtime_protos.h:46` — the fallback when the `default_value` `BoolValue` wrapper is absent is hard-coded `true`). `filterEnabled() = admission_control_feature_.enabled()` (`admission_control.h:64`), checked together with `decoder_callbacks_->streamInfo().healthCheck()` at `admission_control.cc:81`. The behavioral matrix is in §5.3.

- **AMEND-5 (`success_criteria` defaults — REFINES BRAINSTORM §1.1; driven by §11 D3):** HTTP default success = **all codes `< 500`** (NOT `2xx` as the BRAINSTORM hypothesized; `success_criteria_evaluator.cc:43`); configurable `http_success_status` ranges are half-open `[start, end)` with bounds `[100, 600)` (`success_criteria_evaluator.h:31-33`). gRPC default success = the **11-code well-known set** {Ok, Canceled, Unknown, InvalidArgument, NotFound, AlreadyExists, Unauthenticated, FailedPrecondition, OutOfRange, PermissionDenied, Unimplemented} (NOT just `OK=0`; `success_criteria_evaluator.cc:57-69`); configured `grpc_success_status` lists are validated `≤ 16` entries (`:49-50`).

- **AMEND-6 (sliding-window mechanics — REFINES BRAINSTORM §5; driven by §11 D3):** The window is a **`std::deque` of `<MonotonicTime, RequestData{requests, successes}>` per-second buckets** (`thread_local_controller.h:111`; granularity `defaultHistoryGranularity{1}` at `thread_local_controller.cc:14`), NOT a fixed circular buffer. On each record: stale buckets older than `sampling_window_` are purged (decrementing the running `global_data_` aggregate), a new bucket rolls over once the newest is ≥1s old, then the back bucket + `global_data_` are incremented (`thread_local_controller.cc:30-54`). `requestCounts()` purges then returns `global_data_` — the `{n, s}` fed into the formula. `sampling_window` default 30s (`config.cc:18`), "rounded to the nearest second" via integer `ms/1000` truncation (`config.cc:33-35`). `averageRps() = global_data_.requests / max(sampling_window, age_of_oldest_sample)` in whole seconds (`thread_local_controller.cc:20-28`); returns 0 when the window is empty.

- **AMEND-7 (503 reject local-reply byte-pin — REFUTES the BRAINSTORM "upstream-exact local-reply body" implication; driven by §11 D4):** The reject local reply is `decoder_callbacks_->sendLocalReply(Http::Code::ServiceUnavailable, "", nullptr, absl::nullopt, "denied_by_admission_control")` (`admission_control.cc:101-102`). Byte-pin: status **503**; body **EMPTY** (`""`, 0 bytes — NOT a descriptive string); `response_code_details = "denied_by_admission_control"` (27 bytes, a hard-coded string literal at the call site — there is **NO** `RcDetails` `constexpr` constant in this filter); `modify_headers = nullptr` (no filter-added headers); `grpc_status = absl::nullopt` (the reject path does NOT branch on gRPC — a rejected gRPC request gets the same 503/empty-body reply). `rq_rejected` is incremented immediately before, at `admission_control.cc:100`.

- **AMEND-8 (boot-reject roster — REFUTES BRAINSTORM §6.2's `sr_threshold > 100%` hypothesis; driven by §11 D-followup B):** Upstream does **NOT** reject `sr_threshold > 100%` / `max_rejection_probability > 100%` / non-positive `aggression` at config-load — those are CLAMPED at runtime (`std::min(.,100)` / `std::max(1.0,.)`), not rejected. The actual config-load rejects (all `absl::InvalidArgumentError`, no `EnvoyException`) are: (i) `sr_threshold.default_value.value < 1.0` → `"Success rate threshold cannot be less than 1.0%."` (`config.cc:25-27`); (ii) `evaluation_criteria` oneof not set → `"Evaluation criteria not set"` (`config.cc:49-50`); (iii) delegated `SuccessCriteriaEvaluator::create` range-validation errors (`config.cc:43-45`). The **boot-reject differential fixture uses arm (i)** (`sr_threshold < 1.0%`) — the cleanest single-field shared reject with a distinctive, byte-stable stderr substring.

- **AMEND-9 (dual-factory; downstream-only MVP — driven by §11 D-followup C):** The filter is a `Common::DualFactoryBase` registered for BOTH the downstream HCM (`Server::Configuration::NamedHttpFilterConfigFactory`) and the upstream HTTP filter chain (`Server::Configuration::UpstreamHttpFilterConfigFactory` via the alias `UpstreamAdmissionControlFilterFactory = AdmissionControlFilterFactory`); config name `"envoy.filters.http.admission_control"` (`config.h:20`; `config.cc:66-69`). **envoy-go MVP targets the downstream HCM filter only** (consistent with the project's downstream-HCM-only filter framework); upstream-HTTP-filter-chain placement is deferred (§8 item 7).

- **AMEND-10 (gRPC-status-in-trailers classification — REFINES BRAINSTORM §1.1; driven by §11 D3):** Success/error classification happens at `encodeHeaders` (`admission_control.cc:118-140`), but a gRPC response may carry its status in **trailers** rather than headers; upstream sets `expect_grpc_status_in_trailer_` and classifies at `encodeTrailers` for that case (`admission_control.cc:145-159`). envoy-go MUST classify at both encode-headers and encode-trailers for the gRPC arm — the BRAINSTORM's "`EncodeComplete` classifies" framing is replaced by the encodeHeaders-or-encodeTrailers discipline (§4.4).

- **AMEND-11 (record discipline — REFINES BRAINSTORM §1.1; driven by §11 D3):** Rejected requests, health-check requests, and disabled-filter requests are deliberately **NOT** recorded into the window (`record_request_ = false` at `admission_control.cc:81-85, 98`). Only admitted, non-health-check, filter-enabled requests are classified + recorded at encode time. This keeps the success-rate window measuring the backend's behavior, not the filter's own shedding.

---

## 2. Non-purposes

Phase 23 is single-row per ADR-0045 (no sub-phases). It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land admission_control under the existing 07.1 framework. ZERO new `internal/` framework primitives; ZERO in-place §Decision AMENDMENTs; ZERO ADR-0125 amendments.

- **2.1 RTDS `runtime_key` runtime keying OUT OF SCOPE + PARSE-REJECT.** Per BRAINSTORM §2.1 (Q1) + ADR-0195. A non-empty `runtime_key` inside ANY of the four `Runtime*` wrappers (`enabled.RuntimeFeatureFlag`, `aggression.RuntimeDouble`, `sr_threshold.RuntimePercent` / `max_rejection_probability.RuntimePercent`, `rps_threshold.RuntimeUInt32`) triggers HCM-parse-time PARSE-REJECT. The static-default path (each wrapper's `default_value` honored) is the only honored arm at MVP. **NOTE:** this is an envoy-go-strict departure — upstream ACCEPTS `runtime_key` (consults the runtime layer); the divergence is unit-tested + BEHAVIOR_CONTRACT-recorded (§13), NOT a differential boot-reject fixture (upstream does not reject it). Closes after the Runtime/RTDS family phase lands.
- **2.2 Probabilistic-path cross-side byte-exact parity OUT OF SCOPE.** Per BRAINSTORM §2.2 + AMEND-2. The reject decision draws from an RNG; cross-side byte-exactness of the *probabilistic* path is intrinsically un-matchable against a foreign RNG, and the forced-reject byte-exact promotion requires ≥10000 primed failures in a single window on a single worker (impractical/fragile). Phase-23 ships the all-admit `P_reject=0` leg as the cross-side byte-exact leg (RNG-independent per AMEND-2) + the forced-reject leg as subject-only structural + the reject byte shape pinned subject-only against the D4 SPEC-time capture. Full probabilistic cross-side parity is deferred indefinitely (no consumer demand; would require reference-Envoy RNG injection).
- **2.3 Extra-upstream observability gauges OUT OF SCOPE.** Per BRAINSTORM §2.5. Upstream admission_control publishes ONLY 3 counters (no rejection-probability gauge); the mission is byte-exact conformance, so envoy-go publishes the same 3 counters and NO extra gauge. A future operator-observability extension could add a current-rejection-probability gauge as an envoy-go-strict departure if ever justified (§8 item 4).
- **2.4 `AdmissionControlPerRoute` per-route override NEVER-DEFERRED (REUSE-by-absence).** The v1.37.2 admission_control proto has **NO `AdmissionControlPerRoute` message** at all (verified per §11 + BRAINSTORM §4). Listener-scoped only; route-level placement is a proto-deserialization-time PARSE-REJECT via the existing HCM filter framework (the `typed_per_filter_config` deserializer fails — no phase-23-specific code). **FIRST ADR-0125-skip since phase-22's roster amendment** (8 → 9 at 22.3); canonical-per-route roster STAYS 9. Permanent absence; no ADR-0125 amendment at phase 23.
- **2.5 `internal/clock/` + `internal/rand/` framework-primitive extraction OUT OF SCOPE.** Per BRAINSTORM §2.2 + §2.3 + §3. Both seams stay **inline** in the filter package. `Rand` consumer count = 1 (stays inline trivially). `Clock`-shaped consumer count reaches 2 (phase-21 + phase-23) — the documented EXTRACT-NOW trigger — BUT the shapes differ (phase-21 `Now()`+`AfterFunc` over an injectable timer; phase-23 `Now()`-only over a monotonic clock), so a shared extraction would over-fit; the convergence question is a forward-pointer (§8 item 8), evaluated when a third timer/clock-driven filter with a convergent shape lands.
- **2.6 Upstream HTTP filter chain placement OUT OF SCOPE.** Per AMEND-9. The dual-factory upstream registration (`UpstreamHttpFilterConfigFactory`) is deferred; envoy-go MVP wires admission_control into the downstream HCM only, matching the project's existing downstream-HCM-only filter framework. Closes if/when envoy-go gains an upstream-HTTP-filter-chain surface (§8 item 7).
- **2.7 Multi-worker / cross-thread window-state semantics OUT OF SCOPE (single-instance parity).** Upstream's controller is thread-local (per-worker windows). envoy-go's existing per-HCM-instance filter model owns one controller per HCM instance; the single-listener fixture exercises a single controller. Cross-worker window aggregation parity is not modeled (§8 item 6).
- **2.8 NEVER-DEFERRED — Runtime feature-flag layer.** envoy-go has no runtime-features layer (per phase-20 S2 settled). The `Runtime*` wrappers are consumed for their static `default_value` only (per AMEND-4 for `enabled`; per the wrapper defaults for the numeric knobs); `runtime_key` PARSE-REJECTs (§2.1 + ADR-0195).
- **2.9 Framework REUSES NOT consumed.** ADR-0144 `DownstreamPrincipal()` NOT consumed (no TLS-principal interaction). ADR-0188 `internal/lua/` NOT consumed. ADR-0190 `internal/dynamicmetadata/` NOT consumed. ADR-0177 `internal/httpclient/` NOT consumed (no outbound HTTP). ADR-0178 `internal/sdsfile/` NOT consumed (no SDS). ADR-0059 `*stats.Gauge` NOT consumed (counters only, no gauges per §2.3). ADR-0165/ADR-0174 `DecoderFilterCallbacks`/`EncoderFilterCallbacks` extensions NOT consumed.

---

## 3. Framework survey result (ZERO NEW top-level primitives + ZERO IN-PLACE §Decision AMENDMENTs + 7 REUSES)

The framework survey evaluated REUSE of phase-04-through-22 primitives BEFORE proposing NEW (per the phase-16/17/18.x/19.x/20/21/22 discipline). Findings: phase-23 returns to phase-21's LEAN posture — ZERO new `internal/` packages, ZERO in-place §Decision amendments.

### 3.1 In-package: inline `Rand` interface seam at `rand.go` *(NEW seam; per BRAINSTORM §2.2 + AMEND-2 — NO framework primitive; documented in ADR-0194 §Decision)*

In-package `Rand` interface at `internal/filter/http/admission_control/rand.go`. Public surface (settled at SPEC; the IMPL confirms the exact signature):

```go
// Rand is the controller's seam over the rejection dice-roll. It mirrors
// upstream Envoy's Random::RandomGenerator::random() (uint64). The in-package
// interface lets the unit tests inject a deterministic draw to pin the exact
// reject/admit boundary without depending on a framework-wide primitive.
type Rand interface {
    Uint64() uint64
}

// defaultRand wraps math/rand/v2. Used at production wiring.
type defaultRand struct{}

func (defaultRand) Uint64() uint64 { return rand.Uint64() } // math/rand/v2
```

**Consumer count at introduction time: 1.** Stays inline. `fakeRand` (test-scope only) returns a chosen `uint64` so `TestShouldReject_Boundary_*` can pin the `(1e4·P) > (r % 1e4)` knife-edge exactly. **NOTE (AMEND-2):** the seam is `Uint64()`, NOT the BRAINSTORM-hypothesized `Float64()` — to faithfully mirror upstream's integer-modulo decision (`admission_control.cc:175-178`).

### 3.2 In-package: inline `Clock` interface seam at `clock.go` *(NEW seam; per BRAINSTORM §2.3 — NO framework primitive; documented in ADR-0194 §Decision)*

In-package `Clock` interface at `internal/filter/http/admission_control/clock.go`. Public surface:

```go
// Clock is the controller's seam over the monotonic clock used to expire
// per-second window buckets (mirrors upstream's TimeSource::monotonicTime()).
type Clock interface {
    Now() time.Time
}

// defaultClock wraps time.Now (which carries a monotonic reading). Production wiring.
type defaultClock struct{}

func (defaultClock) Now() time.Time { return time.Now() }
```

**Consumer count for a Clock-shaped seam at introduction time: 2** (phase-21 + phase-23). Per §2.5 the two seams stay inline (different shapes; a shared `internal/clock/` extraction would over-fit). Forward-pointer to the convergence question (§8 item 8). `fakeClock` (test-scope only) exposes `Advance(d)` to step the clock deterministically + drive window-bucket expiry for the FAKE-TIME algorithmic tests (§14.1 Layer A).

### 3.3 Framework REUSES — 7 reuses + 8 NOT-CONSUMED items

REUSES (load-bearing per BRAINSTORM §3):

- **REUSE 1: `internal/stats/` Counter support** — the 3-counter surface via `Registry.NewCounter` at boot-time. No framework work. (Gauge support exists but is NOT consumed per §2.3.)
- **REUSE 2: HTTPRegistry boot-time registration** — `admission_control.New` wired at `cmd/envoy-go/main.go` at its alphabetical position (between `adaptive_concurrency` and `bandwidthlimit`). **18 HTTP filters wired post-phase-23** (17 → 18). Straight alphabetical per the phase-09..22 convention; NO ADR for the insertion; NO filter-chain ordering surgery.
- **REUSE 3: Per-request filter interface (decode/encode hooks)** — the acquire-decision-at-`DecodeHeaders` + classify-at-`EncodeHeaders`/`EncodeTrailers` pattern fits the existing per-request instance framework. **Decoder-and-encoder-both filter** (joins the encoder-side cohort: buffer / compressor / bandwidth_limit / ext_proc-body / adaptive_concurrency) — the encode-side hooks are needed for the success/error classification per AMEND-10.
- **REUSE 4: HCM-parse-time PARSE-REJECT path** — adds the admission_control parse arms (per §5): the four `runtime_key`-non-empty arms; the `evaluation_criteria` oneof-absent arm; the `sr_threshold < 1.0%` arm; the `http_success_status` range-validity arms; the `grpc_success_status` count arm. Layered via the standard `compiledConfig` constructor returning an error with byte-stable wording per ADR-0080.
- **REUSE 5: REUSE-by-absence per-route enforcement** — no `AdmissionControlPerRoute` message; per-route placement is a proto-deserialization PARSE-REJECT. FIRST ADR-0125-skip since phase-22's amendment (§5.4); roster STAYS 9.
- **REUSE 6: Existing fuzzer-corpus framework** — `FuzzAdmissionControlConfigParse` as the 32nd project-wide fuzzer (31 → 32) at the standard ~30-corpus-seed baseline + the PARSE-REJECT arm coverage (§6.7).
- **REUSE 7: Existing differential-fixture framework** — two new directories (`0030-http-admission-control` cross-side + `0031-http-admission-control-boot-reject` boot-reject) per the dispatch-constraint memory (one fixture dir = one runner branch); fixtures 31 → 33 (§7).

NOT-CONSUMED (documented for cross-phase audit clarity): ADR-0144 `DownstreamPrincipal()`; ADR-0059 `*stats.Gauge`; ADR-0188 `internal/lua/`; ADR-0190 `internal/dynamicmetadata/`; ADR-0177 `internal/httpclient/`; ADR-0178 `internal/sdsfile/`; ADR-0165 `DecoderFilterCallbacks` extensions; ADR-0174 `EncoderFilterCallbacks` extensions.

### 3.4 Total framework footprint table

| Surface | Items | Anticipated LoC |
|---|---|---|
| NEW `internal/filter/http/admission_control/` package | ~9 production + ~6 test Go files per §6.8 | ~1000-1400 LoC (prod) |
| Boot-registration insertion | `cmd/envoy-go/main.go` alphabetical | ~1 LoC |
| Differential fixtures `0030` + `0031` | 2 directories (cross-side + boot-reject) | ~250-400 LoC |
| **Subtotal framework primitives** | NONE NEW | **0 LoC** |
| **GRAND TOTAL phase 23 (prod + fixtures, excl. tests)** | | **~1250-1800 LoC** |

Well **below the ADR-0045 split-gate** (LoC > ~1500 OR tasks > ~25): production proper is ~1000-1400; the worst-case grand total is bounded and the natural split axes don't carve cleanly (the window controller + the FAKE-TIME/Rand tests are tightly coupled; framework delta is zero). **Single-row landing settled** per the ADR-0045 disposition. SPEC author re-evaluates only if IMPL LoC drifts past the gate.

---

## 4. Algorithm + invariants (`admission_control.cc` line-cited lemmata per D3)

This section codifies the admission-control algorithm with **line-exact citations** against upstream `source/extensions/filters/http/admission_control/` at v1.37.2 per the ADR-0194 anchor. The line-exact citation depth is the D3 disposition (§11).

### 4.1 Decode-path gating + reject decision (`admission_control.cc:80-107`)

`decodeHeaders` gates in this exact order:

1. **Enable / health-check gate** (`:81-85`): `if (!config_->filterEnabled() || decoder_callbacks_->streamInfo().healthCheck())` → set `record_request_ = false`; `return Continue`. (`filterEnabled()` per AMEND-4; absent `enabled` ⇒ enabled.)
2. **RPS suppression gate** (`:87-91`): `if (config_->getController().averageRps() < config_->rpsThreshold())` → `return Continue` (NOT recorded as a reject; the request proceeds and IS classified at encode). Default `rps_threshold = 0` (`admission_control.cc:32`) ⇒ gate never suppresses unless configured.
3. **Reject decision** (`:93-104`): `if (shouldRejectRequest())` → set `record_request_ = false`; `stats_.rq_rejected_.inc()` (`:100`); `sendLocalReply(503, "", nullptr, absl::nullopt, "denied_by_admission_control")` (`:101-102`); `return StopIteration`.
4. Otherwise → `return Continue` (the request is admitted; `record_request_` stays true; classified at encode).

`shouldRejectRequest()` (`:161-179`) — the formula (AMEND-1) + the integer-modulo decision (AMEND-2):

```cpp
const double total_requests = request_counts.requests;       // n
const double successful_requests = request_counts.successes;  // s
double probability = total_requests - successful_requests / config_->successRateThreshold();  // :167
probability = probability / (total_requests + 1);             // :168
if (aggression != 1.0) { probability = std::pow(probability, 1.0 / aggression); }  // :170-171
probability = std::min<double>(probability, config_->maxRejectionProbability());   // :173
static constexpr uint64_t accuracy = 1e4;                     // :176
auto r = config_->random().random();                          // :177
return (accuracy * std::max(probability, 0.0)) > (r % accuracy);  // :178
```

envoy-go mirror at `controller.go::shouldReject(now time.Time) bool` (computes `{n,s}` from the window at `now`, applies the formula, draws `r := f.rand.Uint64()`, returns `float64(10000) * math.Max(p, 0.0) > float64(r % 10000)`). Accessors mirror: `aggression() = max(1.0, configured)` (`:57-59`, default 1.0 `:30`); `successRateThreshold() = min(pct,100)/100` (`:61-64`, default 95.0 `:31`); `maxRejectionProbability() = pct/100` (`:70-74`, default 80.0 `:33`); `rpsThreshold()` default 0 (`:32`).

### 4.2 Sliding-window controller (`thread_local_controller.{h,cc}` per AMEND-6)

`std::deque<std::pair<MonotonicTime, RequestData>>` of per-second buckets (`thread_local_controller.h:111`; granularity 1s `:14`). `recordRequest(bool success)` (`:44-54`): `maybeUpdateHistoricalData()` (purge stale buckets older than `sampling_window_`, decrementing `global_data_`; roll a new bucket if newest ≥1s old — `:30-54`); then `++back.requests` + `++global_data_.requests`; if success `++back.successes` + `++global_data_.successes`. `requestCounts()` (`:67-70`) purges then returns `global_data_` = `{n, s}`. `averageRps()` (`:20-28`): `0` if empty; else `global_data_.requests / max(sampling_window_, age_of_oldest_sample)` in whole seconds.

envoy-go mirror at `controller.go`: a slice/deque of `bucket{ts time.Time; requests, successes uint64}` + a `global struct{requests, successes uint64}` aggregate, mutated under a `sync.Mutex`, with `clock.Now()` driving bucket rollover + expiry. `sampling_window` rounded to whole seconds via integer `ms/1000` (mirrors `config.cc:33-35`).

### 4.3 `sampling_window` parse + defaults (`config.cc`)

`sampling_window` default 30s (`config.cc:18`), parsed via `PROTOBUF_GET_MS_OR_DEFAULT(..., 1000*30)/1000` → whole seconds (`:33-35`). The numeric-knob defaults live at `admission_control.cc:30-33`: `defaultAggression=1.0`, `defaultSuccessRateThreshold=95.0`, `defaultRpsThreshold=0`, `defaultMaxRejectionProbability=80.0`.

### 4.4 Encode-path classification (`admission_control.cc:109-159` + `success_criteria_evaluator.{h,cc}` per AMEND-5 + AMEND-10)

`encodeHeaders` (`:118-140`): if `record_request_` is false (rejected / health-check / disabled per AMEND-11) → no-op. Else: if `Grpc::Common::isGrpcResponseHeaders(headers, end_stream)` → if the gRPC status is present in headers, classify via `responseEvaluator().isGrpcSuccess(status)`; if the status is expected in trailers, set `expect_grpc_status_in_trailer_ = true` and defer (`:123-124`). Else (HTTP) → classify via `responseEvaluator().isHttpSuccess(http_status)` (`:132-133`). On classification: `successful_response ? recordSuccess() : recordFailure()` (`:136-140`), which call `recordRequest(true|false)` and `rq_success_.inc()` / `rq_failure_.inc()` (`admission_control.h:111,116`). `encodeTrailers` (`:145-159`): for the `expect_grpc_status_in_trailer_` case, read the gRPC status from trailers and classify+record.

**Classification predicates** (`success_criteria_evaluator.cc`): `isHttpSuccess` = `std::any_of` over the configured half-open `[start,end)` ranges, default all codes `< 500` (`:28-44`, default `:43`); `isGrpcSuccess` = membership test, default the 11-code well-known set (`:47-70`); range validity `start ≤ end && 100 ≤ start < 600 && 100 ≤ end ≤ 600` (`success_criteria_evaluator.h:31-33`).

envoy-go mirror: `compiledConfig` precompiles the http range-set + the grpc code-set at parse time; `controller.go::classify(...)` records into the window at encode. The encode hooks land at `encode_headers.go` + `encode_trailers.go`.

---

## 5. PARSE-REJECT roster (HCM-parse-time) + boot-reject + per-route

### 5.1 RATIFIED-from-PGV / config-validation arms (proto-level or config-load rejection — upstream rejects too)

| Arm | upstream rule | envoy-go-error wording (byte-stable per ADR-0080; finalized at IMPL) |
|---|---|---|
| `evaluation_criteria` oneof absent | `config.cc:49-50` `absl::InvalidArgumentError` + PGV `(validate.required)` on the oneof (`admission_control.pb.validate.go`) | `"admission_control: evaluation_criteria is required"` |
| `sr_threshold.default_value < 1.0%` | `config.cc:25-27` `"Success rate threshold cannot be less than 1.0%."` | `"admission_control: sr_threshold cannot be less than 1.0%"` |
| `http_success_status` range invalid (start>end, or outside `[100,600)`) | `success_criteria_evaluator.h:31-33` range-validity (delegated `config.cc:43-45`) | `"admission_control: http_success_status range invalid (must be within [100,600) and start<=end)"` |
| `grpc_success_status` list > 16 entries | `success_criteria_evaluator.cc:49-50` | `"admission_control: grpc_success_status accepts at most 16 codes"` |

Per the phase-11 ADR-0115 + phase-15 ADR-0136 + phase-21 envoy-go-defensive-PGV-mirror precedent — envoy-go does its own validation rather than relying on go-control-plane PGV middleware. The boot-reject differential fixture (§7.2) exercises the `sr_threshold < 1.0%` arm (cleanest shared reject; distinctive substring). Exact wording finalized at IMPL + asserted by a `TestParseRejectConstants_ByteStable` table.

### 5.2 envoy-go-strict project-local arms (stricter than upstream — RTDS deferral per ADR-0195)

| Arm | envoy-go behavior | envoy-go-error wording (finalized at IMPL) | ADR anchor |
|---|---|---|---|
| `enabled.runtime_key != ""` | PARSE-REJECT (defers RTDS `RuntimeFeatureFlag`) | `"admission_control: enabled.runtime_key is not yet supported; use enabled.default_value"` | ADR-0195 |
| `aggression.runtime_key != ""` | PARSE-REJECT (defers RTDS `RuntimeDouble`) | `"admission_control: aggression.runtime_key is not yet supported; use aggression.default_value"` | ADR-0195 |
| `sr_threshold.runtime_key != ""` | PARSE-REJECT (defers RTDS `RuntimePercent`) | `"admission_control: sr_threshold.runtime_key is not yet supported; use sr_threshold.default_value"` | ADR-0195 |
| `max_rejection_probability.runtime_key != ""` | PARSE-REJECT (defers RTDS `RuntimePercent`) | `"admission_control: max_rejection_probability.runtime_key is not yet supported; use max_rejection_probability.default_value"` | ADR-0195 |
| `rps_threshold.runtime_key != ""` | PARSE-REJECT (defers RTDS `RuntimeUInt32`) | `"admission_control: rps_threshold.runtime_key is not yet supported; use rps_threshold.default_value"` | ADR-0195 |
| Per-route placement (any `typed_per_filter_config` map entry for admission_control) | PARSE-REJECT via REUSE-by-absence (no `AdmissionControlPerRoute` message; the `typed_per_filter_config` deserializer fails) | Existing HCM-parse-time error path (§5.4) | (no NEW ADR — REUSE-by-absence) |

The `runtime_key` PARSE-REJECT is stricter than upstream (which honors `runtime_key`); the deferral is operator-visible via the byte-stable wording + BEHAVIOR_CONTRACT record (§13). This is the SINGLE anticipated envoy-go-strict departure record (count 14 → 15).

### 5.3 `enabled` honored-matrix (per AMEND-4 — absent ⇒ ENABLED)

| Case | `enabled` field | `default_value` | `runtime_key` | upstream | envoy-go |
|---|---|---|---|---|---|
| 1 | absent entirely | n/a | n/a | ENABLED (`PROTOBUF_GET_WRAPPED_OR_DEFAULT(...,true)`) | ENABLED (matches) |
| 2 | present | `false` | `""` | DISABLED (pass-through) | DISABLED (matches) |
| 3 | present | `true` | `""` | ENABLED | ENABLED |
| 4 | present | any | `"key"` | runtime consults; falls back | **PARSE-REJECT** (ADR-0195) |

Cases 1+2+3 honored at MVP. The `cc.enabled` default in `buildCompiledConfig` is `true` when `enabled` is absent (the AMEND-4 inversion vs phase-21).

### 5.4 Per-route REUSE-by-absence — FIRST ADR-0125-skip since phase-22's amendment

The admission_control proto defines **NO `AdmissionControlPerRoute` message** in v1.32.4 or v1.37.2 (verified per §11). Per-route configuration is a proto-deserialization-time PARSE-REJECT via the existing HCM filter framework (the `typed_per_filter_config` deserializer fails — the wire bytes can't deserialize as a known per-route message). No phase-23-specific PARSE-REJECT code; no framework extension.

**Classification:** REUSE-by-absence. Phase 23 is the **FIRST §9 row to skip ADR-0125 since phase-22's 22.3 amendment** (which ended the four-consecutive 18/19/20/21 skip streak by growing the roster 8 → 9). **No new canonical entry; no ADR-0125 amendment at phase 23. Canonical-per-route roster STAYS 9.**

---

## 6. compiledConfig + code shapes (IMPL blueprint)

### 6.1 `compiledConfig` shape

```go
type compiledConfig struct {
    // Outer envelope (per AMEND-4 — enabled defaults TRUE when the message is absent)
    enabled bool

    // Numeric knobs (static default_value honored; runtime_key PARSE-REJECTed per §5.2)
    samplingWindow          time.Duration // whole seconds; default 30s (per AMEND-6 ms/1000)
    aggression              float64       // floored to 1.0 (per AMEND-1); default 1.0
    srThreshold             float64       // fraction min(pct,100)/100; default 0.95; boot-reject if < 0.01 (§5.1)
    rpsThreshold            uint32        // default 0
    maxRejectionProbability float64       // fraction pct/100; default 0.80

    // success_criteria (the only evaluation_criteria oneof arm; both sub-arms compiled)
    httpSuccessRanges []int32Range // half-open [start,end); default {[100,500)} per AMEND-5
    grpcSuccessCodes  map[uint32]struct{} // default the 11-code well-known set per AMEND-5
}
```

`buildCompiledConfig(typedConfig *anypb.Any) (*compiledConfig, error)` performs the §5.1 + §5.2 PARSE-REJECT roster with byte-stable error wording per ADR-0080, applies the AMEND-4 enabled default, precompiles the http range-set + grpc code-set, and rounds `sampling_window` to whole seconds.

### 6.2 `controller` state shape

```go
type controller struct {
    cfg   *compiledConfig
    stats *filterStats
    clock Clock
    rand  Rand

    mu      sync.Mutex
    buckets []bucket // per-second deque (oldest..newest) per AMEND-6
    global  struct{ requests, successes uint64 }
}

type bucket struct {
    ts                 time.Time // bucket start (1s granularity)
    requests, successes uint64
}
```

Methods mirror upstream: `recordRequest(success bool)` (§4.2), `requestCounts() (n, s uint64)`, `averageRps() uint32`, `shouldReject() bool` (§4.1). All window mutation under `mu`. `clock.Now()` drives rollover/expiry; `rand.Uint64()` drives the dice-roll.

### 6.3 `rand.go` + `clock.go` seams (inline per §3.1 + §3.2)

In-package `Rand` (`Uint64()`) + `Clock` (`Now()`) interfaces with `defaultRand`/`defaultClock` production wirings + `fakeRand`/`fakeClock` test-scope-only implementations.

### 6.4 `decode_headers.go` — gate + reject (per §4.1)

```go
func (f *filter) DecodeHeaders(h Headers, endStream bool) FilterStatus {
    if !f.cc.enabled || f.cb.StreamInfo().IsHealthCheck() {
        f.record = false
        return Continue
    }
    if f.controller.averageRps() < f.cc.rpsThreshold {
        return Continue // not a reject; request proceeds + is classified at encode
    }
    if f.controller.shouldReject() {
        f.record = false
        f.stats.rqRejected.Inc()
        f.cb.SendLocalReply(503, "", nil /*headers*/, nil /*grpc_status*/, "denied_by_admission_control")
        return StopIteration
    }
    return Continue
}
```

(Exact `SendLocalReply` signature + the empty-body / nil-headers / nil-grpc / `denied_by_admission_control` rc-details mapping per AMEND-7 + D4; the IMPL confirms the framework's local-reply API shape.)

### 6.5 `encode_headers.go` + `encode_trailers.go` — classify + record (per §4.4 + AMEND-10)

`EncodeHeaders` classifies HTTP (or gRPC-status-in-headers) responses and records; sets `expectGRPCStatusInTrailer` when a gRPC response defers its status to trailers; `EncodeTrailers` handles that deferred case. Both guard on `f.record` (per AMEND-11).

### 6.6 `stats.go` — 3-counter roster (per AMEND-3)

```go
type filterStats struct {
    rqRejected *stats.Counter
    rqSuccess  *stats.Counter
    rqFailure  *stats.Counter
}

func newFilterStats(reg *stats.Registry, hcmPrefix string) *filterStats {
    p := hcmPrefix + ".admission_control."
    return &filterStats{
        rqRejected: reg.NewCounter(p + "rq_rejected"),
        rqSuccess:  reg.NewCounter(p + "rq_success"),
        rqFailure:  reg.NewCounter(p + "rq_failure"),
    }
}
```

Stat-prefix template `http.<HCM_stat_prefix>.admission_control.<stat>` (the `http.<HCM_stat_prefix>` is HCM-injected via the `hcmPrefix` constructor argument per ADR-0143 SN2-reuse; the `admission_control.` literal infix per AMEND-3 + `config.cc:29`). Project stat count **107 → 110** (+3 counters; no gauges).

### 6.7 `fuzz_test.go` — 32nd project-wide fuzzer

`FuzzAdmissionControlConfigParse` at `internal/filter/http/admission_control/fuzz_test.go` (~50 LoC). Corpus seed roster ~30 entries covering: a valid full config (both success-criteria arms + all knobs); each PARSE-REJECT arm per §5.1 + §5.2 (the four+1 `runtime_key` arms; oneof-absent; `sr_threshold < 1.0%`; malformed http range; >16 grpc codes); empty config; oneof-absent variants. Must-never-panic across `buildCompiledConfig`. Clean at 30s per seed (REUSE 6). Fuzzer count **31 → 32**.

### 6.8 Source-file roster (~9 production + ~6 test = ~15 Go files)

| File | Purpose | Anticipated LoC |
|---|---|---|
| `internal/filter/http/admission_control/doc.go` | Package doc | ~20 |
| `internal/filter/http/admission_control/admission_control.go` | `TypeURL` + `New` factory + `HTTPFilter` value | ~80-120 |
| `internal/filter/http/admission_control/compiled_config.go` | `compiledConfig` + `buildCompiledConfig` + PARSE-REJECT roster per §5 | ~220-300 |
| `internal/filter/http/admission_control/controller.go` | sliding-window controller + formula per §4 + §6.2 | ~250-350 |
| `internal/filter/http/admission_control/rand.go` | `Rand` interface + `defaultRand` per §3.1 | ~25-50 |
| `internal/filter/http/admission_control/clock.go` | `Clock` interface + `defaultClock` per §3.2 | ~25-50 |
| `internal/filter/http/admission_control/decode_headers.go` | `DecodeHeaders` per §6.4 | ~50-80 |
| `internal/filter/http/admission_control/encode.go` | `EncodeHeaders` + `EncodeTrailers` per §6.5 | ~80-120 |
| `internal/filter/http/admission_control/stats.go` | `filterStats` + `newFilterStats` per §6.6 | ~30-50 |
| `internal/filter/http/admission_control/*_test.go` | algorithmic-fidelity + PARSE-REJECT + classification + fuzzer (Layer A per §14.1) | ~700-1100 |

Production-to-test ratio ~1.0 (matches the phase-20/21 envelope).

---

## 7. Differential fixture envelope — two directories (`0030` + `0031`)

Per project memory `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch, cross-side XOR boot-reject), the cross-side and boot-reject surfaces are SEPARATE directories from the start. Fixture directory count **31 → 33**.

### 7.1 `0030-http-admission-control` (cross-side, deterministic regimes only)

| Scenario | Disposition | Wire-level expectation |
|---|---|---|
| **(a) `parse_ok`** | REFERENCE-LESS subject-only structural | admission_control filter loads with full config (both success-criteria arms + all knobs); admin `/stats` exposes the 3-counter surface; HTTP 200 to a normal GET |
| **(b) `all_admit_healthy`** | **CROSS-SIDE byte-exact** (RNG-independent per AMEND-2) | Healthy backend (all responses classified success per `success_criteria`) ⇒ `P_reject = 0` ⇒ `0 > (r % 1e4)` is false for every `r` ⇒ EVERY request admitted; status + body byte-exact cross-side vs reference Envoy v1.37.2 (no RNG dependence at P=0). `rq_success` increments; `rq_rejected` stays 0 |
| **(c) `stat_surface`** | REFERENCE-LESS subject-only structural | Admin `/stats` exposes exactly the 3 counters under `http.<prefix>.admission_control.{rq_rejected,rq_success,rq_failure}` with expected values after a small healthy burst (`rq_success` > 0; `rq_rejected` = 0; `rq_failure` = 0) |
| **(d) `pass_through_disabled`** | REFERENCE-LESS subject-only structural | Config `enabled.default_value=false` (pass-through per §5.3 case 2). All requests pass through; no 503; counters stay 0 (not recorded per AMEND-11) |

The **forced-reject leg is NOT a cross-side scenario** (per AMEND-2 + §11 D2 — byte-exact would require ≥10000 primed failures in one window on one worker; impractical/fragile). The reject path is covered subject-only at the unit layer (§14.1 Layer A `TestShouldReject_Boundary_*` with injected `Rand`) + the reject byte shape (503 / empty body / `denied_by_admission_control`) is asserted against the D4 SPEC-time empirical capture.

### 7.2 `0031-http-admission-control-boot-reject` (boot-reject)

A SHARED config-load reject where upstream Envoy ALSO rejects at boot (byte-comparable, NOT an envoy-go departure): **`sr_threshold.default_value < 1.0%`** → upstream stderr substring `"Success rate threshold cannot be less than 1.0%."` (`config.cc:25-27`; per AMEND-8). envoy-go's PGV-mirror rejects with the byte-stable §5.1 wording; the fixture pins the common distinctive stderr substring. **NOTE:** the RTDS `runtime_key` reject is NOT a boot-reject fixture candidate — upstream ACCEPTS `runtime_key`, so an envoy-go reject DIVERGES by design; that departure is unit-tested + BEHAVIOR_CONTRACT-recorded (§13), not differential.

### 7.3 Listener topology

Single listener with a single HCM containing the admission_control filter (alphabetical position) + router terminator. Backend: a synthetic always-200 cluster for `0030` scenarios (a)-(c); `enabled=false` config for (d). No multi-listener topology (avoids the `freeTCPPort` combined-run flake surface per 22.2 REVIEW §7.4).

### 7.4 Six-gate checklist (A/B/C/D/E/F)

Identical matrix to phase-09..22:

- **Gate A — build**: `go build ./...` clean
- **Gate B — vet + lint**: `go vet ./...` + `golangci-lint run` clean; no new suppressions
- **Gate C — race**: `go test -race ./...` clean (incl. the new `internal/filter/http/admission_control/`; the window controller's `mu`-guarded mutation under concurrent decode/encode)
- **Gate D — differential**: 33/33 fixtures GREEN (0000-0029 pre-existing + 0030 + 0031 new); cross-side byte-exact on `0030` scenario (b); boot-reject substring on `0031`
- **Gate E — fuzz**: `FuzzAdmissionControlConfigParse` clean at 30s per seed; no panics across the 32 project-wide fuzzers
- **Gate F — h2spec**: 53/53 PASS at ADR-0051 v1.32.4 pin

---

## 8. Deferred items (9 items)

1. **RTDS `runtime_key` runtime keying** — PARSE-REJECT at config-load per §2.1 + ADR-0195. Closes after the Runtime/RTDS family phase lands.
2. **Probabilistic-path cross-side byte-exact parity (incl. forced-reject promotion)** — intrinsically un-matchable against a foreign RNG; the forced-reject byte-exact promotion requires ≥10000 primed failures in a single window on a single worker (per AMEND-2 + §11 D2) — impractical/fragile. The all-admit `P=0` leg is the cross-side byte-exact leg; the reject path is subject-only. Deferred indefinitely (no consumer demand; would require reference-Envoy RNG injection).
3. **`AdmissionControlPerRoute`** — no such message in v1.37.2 (§2.4 + §5.4); if upstream adds one, a future ADR-0125 amendment.
4. **Extra-upstream observability gauges** — deliberately omitted for conformance (§2.3); a future operator-observability extension could add a current-rejection-probability gauge as an envoy-go-strict departure if justified.
5. **`success_criteria_evaluator` delegated range-validation error wording** — the exact `SuccessCriteriaEvaluator::create` error strings (delegated at `config.cc:43-45`) are mirrored defensively at envoy-go parse time; byte-stable wording finalized at IMPL (§5.1 last row).
6. **Multi-worker / cross-thread window-state aggregation parity** — upstream's controller is thread-local per-worker; envoy-go's per-HCM-instance model is single-controller (§2.7). A future fixture-extension could exercise multi-worker window semantics.
7. **Upstream HTTP filter chain placement** — the dual-factory upstream registration (per AMEND-9) is deferred; envoy-go MVP wires the downstream HCM only (§2.6). Closes if/when envoy-go gains an upstream-HTTP-filter-chain surface.
8. **Shared `internal/clock/` extraction** — Clock-shaped consumer count is now 2 (phase-21 + phase-23) but the shapes differ (§2.5); a future convergent third consumer triggers the extraction decision.
9. **gRPC reject-path local reply** — the reject path emits the same 503/empty-body reply regardless of gRPC-ness (per AMEND-7 — no grpc_status passed); a future gRPC-aware reject extension is not anticipated (upstream itself does not differentiate).

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

Phase-23 picks up **ZERO closures** from prior phases' deferred-items lists. It lands a NEW filter (admission_control) and does not pick up cross-filter deferred items. The phase-21 Clock-seam EXTRACT-NOW forward-pointer is NOTED (Clock-shaped consumer count reaches 2) but NOT consumed — the two seam shapes differ enough that a shared extraction would over-fit; phase-23 records the convergence question as a forward-pointer (§8 item 8) rather than forcing an extraction. This preserves the EXTRACT-NOW-only-when-the-trigger-genuinely-fires discipline (phase-17 lesson). Phase-23 also ENDS the phase-22 framework-primitive-introducing posture by introducing ZERO new primitives — returning to phase-21's LEAN baseline.

---

## 10. ADR anchor map (2 NEW §Context drafts; ZERO IN-PLACE AMENDMENTs; D-hypothesis)

Per ADR-0044 ADR-on-impl convention: ADR-0194 + ADR-0195 §Context drafts anchor at this SPEC commit (appended to `DECISIONS.md`); §Decision + §Consequences bodies land at each ADR's Lands-in-Task at IMPL.

### A. 2 NEW ADRs (ADR-0194, ADR-0195)

| ADR | Subject | Anchors §§ | Lands-in-Task |
|---|---|---|---|
| **ADR-0194** | admission-control algorithm + `internal/filter/http/admission_control/` package shape + inline `Rand` (`Uint64()`, per AMEND-2) + inline `Clock` (`Now()`) seams (NOT framework primitives) + the sliding-window controller (deque-of-per-second-buckets per AMEND-6) + the empirically-pinned formula `max(0, min(max_rej, ((n−s/sr)/(n+1))^(1/aggr)))` + the integer-modulo reject decision `(1e4·P) > (r%1e4)` + the success/error classification (http `<500` default; grpc 11-code default; gRPC-trailers per AMEND-10) + line-cited lemmata against `admission_control.cc` / `thread_local_controller.cc` / `success_criteria_evaluator.cc` per D3 + the 3-counter byte-exact stat surface (no gauges) + the deterministic-regime differential strategy (all-admit P=0 cross-side; forced-reject subject-only per AMEND-2) | §1; §3.1; §3.2; §4; §6; §7; §1.1 AMEND-1/2/3/5/6/7/9/10/11 | IMPL Task: controller + filter materialization |
| **ADR-0195** | RTDS `runtime_key` deferral PARSE-REJECT — every `Runtime{FeatureFlag,Double,Percent,UInt32}` wrapper honored only for its static `default_value`; any non-empty `runtime_key` triggers HCM-parse-time PARSE-REJECT with a forward-pointer to the future Runtime/RTDS family phase; `enabled`-absent-ENABLED semantics (per AMEND-4 — OPPOSITE of phase-21 adaptive_concurrency's ADR-0187 enabled-default-OFF); the SINGLE envoy-go-strict departure record | §2.1; §5.2; §5.3; §1.1 AMEND-4 | IMPL Task: compiled_config + PARSE-REJECT roster |

### B. ZERO IN-PLACE §Decision AMENDMENTs + ZERO ADR-0125 amendments

No `internal/stats/` AMENDMENT (counters only; ADR-0059 NOT touched). No ADR-0125 amendment (REUSE-by-absence per §5.4; roster STAYS 9; first skip since phase-22). No other in-place §Decision edits.

### C. ADR-0044 escape-valve reserve + D-hypothesis

~0-2 impl-time-unanticipated ADRs per phase. Phase-23's most-likely surfaces (all SPEC-time CLOSED via §4 + §5 + §6 + §11 in-§Decision at ADR-0194): the `sampling_window` deque rollover edge at window-start; the `(1e4·P) > (r%1e4)` float-rounding edge at the admit/reject knife-edge; the gRPC-trailers classification edge. **D-style hypothesis:** ADR-0196 stays UNCONSUMED at phase-23 phase-done. **HOLD-with-known-risk, not GUARANTEED-HOLD** — a surprise upstream stat name, a 4th counter, or a clamp-boundary float edge at IMPL could force ADR-0196 consumption. The buffer is one slot wide.

### D. Anchor map summary

| Disposition | Count | ADR numbers |
|---|---|---|
| NEW ADR §Context drafts | 2 | ADR-0194; ADR-0195 |
| IN-PLACE §Decision AMENDMENT-anticipation | 0 | NONE |
| ADR-0125 amendments | 0 | NONE — REUSE-by-absence per §5.4 (first skip since phase-22) |
| ADR-0044 escape-valve reserve | 0-2 | reserved at ADR-0196+ if fired |

**Next-free ADR post-SPEC commit: `ADR-0196`** (2 NEW consumed: ADR-0194 + ADR-0195).

---

## 11. Empirical-pin block (4 D-pins + 1 follow-up resolved at this SPEC session)

### A. Pin disposition matrix

| Pin | Disposition | Wire-level finding | ADR anchor |
|---|---|---|---|
| **D1** Stat roster + prefix | AMEND | 3 counters `rq_rejected` / `rq_success` / **`rq_failure`** (NOT `rq_error`); NO gauges/histograms (macro is `COUNTER`-only `admission_control.h:35-38`); prefix `http.<HCM_stat_prefix>.admission_control.<stat>` (literal infix `config.cc:29`); 107 → 110 HOLDS | ADR-0194; AMEND-3 |
| **D2** Forced-reject cross-side promotion | REFUTED-on-practicality | reject decision is integer-modulo `(1e4·P) > (r%1e4)` (`admission_control.cc:175-178`); P=0 ⇒ never reject (RNG-independent ⇒ all-admit cross-side); forced-reject byte-exact needs `P>0.9999` ⇒ N≥10000 primed failures in one window/worker (impractical) ⇒ STAYS subject-only structural | ADR-0194; AMEND-2 |
| **D3** Formula + clamp/gate ordering line-citation | RATIFIED-with-REFUTATION | full formula `max(0, min(max_rej, ((n−s/sr)/(n+1))^(1/aggr)))` line-cited (`:161-179`); aggression EXPONENT floored to 1.0 (`:57-59`); sr_threshold DIVIDES successes (`:167`); rps suppression gate (`:87-91`); deque-of-buckets window (`thread_local_controller`); classification (`success_criteria_evaluator`). REFUTES BRAINSTORM formula (exponent vs multiplier; sr_threshold-divides; aggression-floor) | ADR-0194; AMEND-1/5/6/10/11 |
| **D4** 503 reject local-reply byte-pin | RATIFIED-with-REFUTATION | `sendLocalReply(503, "", nullptr, absl::nullopt, "denied_by_admission_control")` (`:101-102`); body **EMPTY**; rc_details hard-coded literal (no constant); no headers; no grpc_status; `rq_rejected.inc()` at `:100`. REFUTES the BRAINSTORM "upstream-exact body" implication (body is empty) | ADR-0194; AMEND-7 |
| **D-followup A/B/C** enabled-default + boot-reject roster + dual-factory | AMEND | enabled-absent ⇒ ENABLED (`runtime_protos.h:46` `…,true`); boot-rejects = `sr_threshold<1.0%` (`config.cc:25-27`) + `evaluation_criteria` oneof-absent (`:49-50`); dual-factory downstream+upstream, config-name `envoy.filters.http.admission_control` (`config.h:20`), downstream-only MVP | ADR-0194; ADR-0195; AMEND-4/8/9 |

### B. Pin disposition summary

| Disposition | Count |
|---|---|
| RATIFIED-with-REFUTATION | 2 (D3, D4) |
| AMEND | 2 (D1, D-followup) |
| REFUTED-on-practicality | 1 (D2) |
| **TOTAL** | **5** |

All pins CLOSED at SPEC time. No pin disposition deferred to IMPL at the pin-disposition level; the §12 residual byte-confirmations are SUB-PIN-LEVEL refinements (exact error-string wording; the local-reply API-shape mapping).

### C. Pin-to-AMEND-block traceability

| AMEND-N | Sources | Recipient ADRs |
|---|---|---|
| AMEND-1 | D3 (formula refutation) | ADR-0194 |
| AMEND-2 | D2 + D3 (integer-modulo decision; `Rand` seam shape; forced-reject subject-only) | ADR-0194 |
| AMEND-3 | D1 (stat name `rq_failure`; 3 counters no gauges; prefix) | ADR-0194 |
| AMEND-4 | D-followup A (enabled-absent ENABLED) | ADR-0195 |
| AMEND-5 | D3 (success defaults http `<500` / grpc 11-code) | ADR-0194 |
| AMEND-6 | D3 (deque-of-per-second-buckets window) | ADR-0194 |
| AMEND-7 | D4 (empty reject body + rc_details) | ADR-0194 |
| AMEND-8 | D-followup B (boot-reject `sr_threshold<1.0%`) | ADR-0194 (fixture) / §5.1 |
| AMEND-9 | D-followup C (dual-factory; downstream-only MVP) | ADR-0194 / §2.6 |
| AMEND-10 | D3 (gRPC-trailers classification) | ADR-0194 |
| AMEND-11 | D3 (record discipline — reject/health/disabled not recorded) | ADR-0194 |

---

## 12. Deferred decisions (the planner / implementer settles these)

Sub-pin-level refinements of already-closed pins; settled at IMPL Tasks + the six-gate verification. None block phase-23 phase-done.

### A. Wire-shape / API byte-confirmation items
1. **503 local-reply API mapping** — the exact envoy-go framework `SendLocalReply` signature mapping for (empty body, nil added-headers, nil grpc_status, `response_code_details = "denied_by_admission_control"`) per AMEND-7 + D4. Settles at IMPL (filter materialization) + `0030`/unit assertion against the D4 capture.
2. **PARSE-REJECT byte-stable error wording** — the §5.1 + §5.2 wording finalized + asserted by `TestParseRejectConstants_ByteStable` per ADR-0080. Settles at IMPL (compiled_config materialization).
3. **Boot-reject common stderr substring** — the exact shared substring for `0031` (`"Success rate threshold cannot be less than 1.0%."` upstream vs the envoy-go-mirror wording) per AMEND-8. Settles at IMPL (fixture authoring).

### B. Algorithmic byte-confirmation items
4. **`(1e4·P) > (r%1e4)` admit/reject knife-edge** — the exact float-rounding behavior at the boundary `r%1e4 == floor(1e4·P)`. Settles at IMPL `controller_test.go` `TestShouldReject_Boundary_*` with injected `Rand` (mirror upstream's strict `>` + the `accuracy=1e4` integer modulo exactly).
5. **`sampling_window` deque rollover/expiry determinism** — bucket rollover at the 1s granularity boundary + stale-purge correctness under the injected `Clock`. Settles at IMPL `controller_test.go` FAKE-TIME tests.
6. **gRPC-trailers classification path** — the `expect_grpc_status_in_trailer_` deferral per AMEND-10. Settles at IMPL `encode` tests (gRPC-status-in-headers vs in-trailers).

### C. Cross-phase regression-window items
7. **Zero framework regression** — phase-23 touches no shared `internal/` primitive (counters-only via existing `internal/stats/`); the six-gate Gate C race tests confirm zero regression. Expected outcome: zero regression (no framework signature change).

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052; lands at phase-23 phase-done)

Edit-bundle landing at the IMPL phase-done commit per ADR-0052. None at this SPEC commit. Edits:

### A. NEW top-level subsection (1)
1. **NEW `### envoy.filters.http.admission_control` subsection** inserted after `### envoy.filters.http.lua` (current line 2212). Subsections: filter scope (SRE-book client-side admission control; SIXTEENTH §9 row); the empirically-pinned formula + integer-modulo decision; the both-sides decode-gate/encode-classify discipline (per AMEND-1/2/4/5/10/11); the 3-counter stat surface (per AMEND-3); the reject wire shape (503 / empty body / `denied_by_admission_control` per AMEND-7); listener-scoped discipline (REUSE-by-absence; first ADR-0125-skip since phase-22); the SINGLE envoy-go-strict departure (RTDS `runtime_key` PARSE-REJECT). Anticipated ~120-200 LoC.

### B. NEW envoy-go-strict departure record (1)
2. **RTDS `runtime_key` PARSE-REJECT departure** — upstream ACCEPTS `runtime_key` (consults the runtime layer); envoy-go PARSE-REJECTs any non-empty `runtime_key` inside the four `Runtime*` wrappers (per ADR-0195), honoring the static `default_value`. Departure-record count **14 → 15**.

### C. Per-section additions (2)
3. **Stat-name mapping 107 → 110 table extension** — 3 new counters (`rq_rejected`, `rq_success`, `rq_failure`) under `http.<HCM_stat_prefix>.admission_control.*`.
4. **Per-route canonical patterns cross-reference table caption update** — "updated through phase 22" → "updated through phase 23"; phase-23 cross-reference paragraph documenting the REUSE-by-absence (first ADR-0125-skip since phase-22; no roster extension; roster STAYS 9).

### D. Edit-bundle summary

| Category | Count |
|---|---|
| NEW top-level subsection | 1 |
| NEW envoy-go-strict departure record | 1 |
| Per-section additions | 2 |
| **TOTAL** | **4** |

All edits land at the SAME IMPL commit per ADR-0052; none mutate pre-phase-23 paragraphs (in-place-by-append discipline).

---

## 14. Testing strategy

### 14.1 Unit tests — two-layer taxonomy (Layer A subject-only algorithmic-fidelity + Layer B cross-side fixture)

Test surface at `internal/filter/http/admission_control/*_test.go` per §6.8.

**Layer A — Subject-only algorithmic-fidelity tests via FakeClock + FakeRand** (per BRAINSTORM §2.2 + §3.1 + §3.2):

1. **`TestShouldReject_Boundary_*`** — per §4.1 + AMEND-2: with a primed window `{n, s}` and injected `Rand`, verify the exact `(1e4·P) > (r % 1e4)` reject/admit knife-edge (`r % 1e4 == floor(1e4·P)` admits; one less rejects); verify P=0 ⇒ never reject for any `r`.
2. **`TestProbabilityFormula_*`** — per §4.1 + AMEND-1: vector tests for `P_reject` under varying `{n, s, sr_threshold, aggression, max_rejection_probability}`; verify the exponent (`^(1/aggression)`, skipped at aggression=1.0), the sr_threshold-divides-successes term, the aggression floor (configured 0.5 → 1.0), the `max_rejection_probability` clamp, the `max(0,…)` floor.
3. **`TestController_FAKE_TIME_Window_*`** — per §4.2 + AMEND-6: verify per-second bucket rollover + stale-purge over the `sampling_window`; verify `requestCounts()` aggregate; verify `averageRps()` (0 when empty; `n/secs` else).
4. **`TestRpsSuppression_*`** — per §4.1: verify the `averageRps() < rps_threshold` gate returns admit-without-reject (request proceeds, classified at encode).
5. **`TestClassification_HTTP_*` + `TestClassification_GRPC_*`** — per §4.4 + AMEND-5 + AMEND-10: verify HTTP default (`<500` success) + configured ranges `[100,600)`; gRPC default (11-code set) + configured lists; gRPC-status-in-headers vs in-trailers (`expect_grpc_status_in_trailer`).
6. **`TestRecordDiscipline_*`** — per AMEND-11: verify rejected / health-check / disabled requests are NOT recorded.
7. **`TestRejectLocalReply_ByteShape`** — per AMEND-7 + D4: verify the reject emits 503 + empty body + `denied_by_admission_control` rc-details + no added headers (against the subject-only emission; pinned to the D4 capture).
8. **`TestBuildCompiledConfig_PARSE_REJECT_*`** — table-driven per §5.1 + §5.2. Byte-stable error wording per ADR-0080.
9. **`TestEnabledMatrix_*`** — per §5.3 + AMEND-4: absent ⇒ enabled; default_value=false ⇒ disabled; default_value=true ⇒ enabled; runtime_key ⇒ PARSE-REJECT.

**Layer B — Structural + cross-side differential fixtures** (per §7): `0030-http-admission-control` (scenario (b) cross-side byte-exact all-admit; (a)/(c)/(d) subject-only structural) + `0031-http-admission-control-boot-reject`.

### 14.2 Race detector + lint
- `go test -race ./...` clean including the new package (the window controller's `mu`-guarded mutation under concurrent decode/encode); `go vet ./...` + `golangci-lint run` clean (no new suppressions); `go build ./...` clean.

### 14.3 Fuzzer
- 32nd fuzzer `FuzzAdmissionControlConfigParse` per §6.7. Must-never-panic across `buildCompiledConfig`. Clean at 30s per seed.

### 14.4 h2spec + differential
- h2spec 53/53 PASS at ADR-0051 pin (no regression); differential 33/33 GREEN (0030 cross-side byte-exact on the all-admit leg; 0031 boot-reject substring).

### 14.5 Six-gate checklist
Per §7.4 — gates A/B/C/D/E/F as the load-bearing IMPL verification. All MUST be GREEN for the row-23 status flip.

---

## 15. Acceptance checklist (for the reviewer)

The phase-23 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following against the landed artefacts. All 16 items MUST be GREEN for row-23 status flip from `in-progress` to `done`.

### A. Six-gate verification (6 items — atomic GREEN per gate)
1. **Gate A — build**: `go build ./...` clean across `internal/filter/http/admission_control/` + all pre-existing packages.
2. **Gate B — vet + lint**: `go vet ./...` + `golangci-lint run` clean; no new lint suppressions.
3. **Gate C — race**: `go test -race ./...` clean; zero data-race violations (incl. the window controller).
4. **Gate D — differential**: 33/33 fixtures GREEN (0000-0029 pre-existing + 0030 + 0031); `0030` scenario (b) all-admit cross-side byte-exact; `0031` boot-reject substring matches.
5. **Gate E — fuzz**: `FuzzAdmissionControlConfigParse` clean at 30s per seed; no panics across the 32 project-wide fuzzers.
6. **Gate F — h2spec**: 53/53 PASS at ADR-0051 pin.

### B. Fixture coverage (1 item)
7. **Two-directory differential** per §7 — `0030-http-admission-control` (4 scenarios: (a) parse_ok / (b) all_admit_healthy CROSS-SIDE byte-exact / (c) stat_surface / (d) pass_through_disabled) + `0031-http-admission-control-boot-reject` (`sr_threshold<1.0%` shared reject). Fixture dir count 31 → 33.

### C. Stat-surface verification (1 item)
8. **3-counter stat surface byte-exact** per ADR-0194 + AMEND-3 + §11 D1: `rq_rejected` + `rq_success` + `rq_failure` (NOT `rq_error`) under `http.<HCM_stat_prefix>.admission_control.<stat>`; project stat count 107 → 110; NO gauges.

### D. Algorithm-fidelity verification (1 item)
9. **Algorithmic fidelity** per §14.1 Layer A: the formula (exponent + sr-divides + aggression-floor + clamp + floor per AMEND-1) + the integer-modulo decision (`(1e4·P) > (r%1e4)` per AMEND-2) + the deque-window (per AMEND-6) + classification (http `<500` / grpc 11-code / gRPC-trailers per AMEND-5 + AMEND-10) + record discipline (per AMEND-11) — all GREEN via FakeClock + FakeRand.

### E. PARSE-REJECT roster verification (1 item)
10. **PARSE-REJECT roster** per §5: the §5.1 RATIFIED-from-config arms (oneof-absent; `sr_threshold<1.0%`; http-range; grpc-count) + the §5.2 envoy-go-strict arms (5× `runtime_key` per ADR-0195) — all byte-stable per ADR-0080 + table-driven coverage.

### F. Reject wire-shape verification (1 item)
11. **503 reject wire shape** per AMEND-7 + D4: status 503 + EMPTY body + `response_code_details = "denied_by_admission_control"` + no added headers + no grpc_status; `rq_rejected` incremented at the reject site (subject-only assertion against the D4 capture).

### G. enabled-semantics verification (1 item)
12. **`enabled` honored-matrix** per §5.3 + AMEND-4: absent ⇒ ENABLED (OPPOSITE of phase-21); default_value=false ⇒ DISABLED; default_value=true ⇒ ENABLED; `runtime_key` ⇒ PARSE-REJECT.

### H. ADR landing (1 item)
13. **2 NEW ADR §Context drafts + §Decision + §Consequences bodies landed** at per-Task Lands-in-Tasks: ADR-0194 (algorithm + package shape + inline Rand/Clock seams + line-cited lemmata) + ADR-0195 (RTDS `runtime_key` deferral + enabled-absent-ENABLED). ZERO in-place §Decision AMENDMENTs; ZERO ADR-0125 amendments (roster STAYS 9).

### I. BEHAVIOR_CONTRACT.md edit-bundle (1 item)
14. **4-edit BEHAVIOR_CONTRACT.md bundle landed** per §13 (NEW `### envoy.filters.http.admission_control` subsection + 1 envoy-go-strict departure record [count 14 → 15] + 2 per-section additions; atomic landing per ADR-0052).

### J. DECISIONS + STATE + ROADMAP advance (1 item)
15. **Doc-state alignment**: DECISIONS.md ADR-0194 + ADR-0195 full bodies at final state; next-free ADR-0196 unconsumed (D-hypothesis: ADR-0196 stays UNCONSUMED at phase-done — HOLD-with-known-risk); STATE.md re-advanced (lifecycle-state phase-23 IMPL done; next-skill per next-phase identity); ROADMAP row 23 flipped to `done` (per-cell IMPL-done annotation; single-row per ADR-0045); 18 HTTP filters wired.

### K. Audit-trail verification (1 item)
16. **End-to-end audit-trail** at phase-done review: SPEC → PLAN → PROGRESS → REVIEW chain landed; per-task PROGRESS records map 1:1 to PLAN tasks; each §11 pin + each §12 item recorded; D-hypothesis disposition recorded (ADR-0196 UNCONSUMED at phase-done); six-gate verbatim outputs at REVIEW.

---
