# Phase 21 Brainstorm — `envoy.filters.http.adaptive_concurrency`

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 21 (`http-filter-adaptive-concurrency`), the FOURTEENTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at phase 07.1, `fault` at phase 09, `header_mutation` at phase 10, `local_ratelimit` at phase 11, `csrf` at phase 12, `buffer` at phase 13, `compressor` at phase 14, `bandwidth_limit` at phase 15, `rbac` at phase 16, `jwt_authn` at phase 17, `ext_authz` at phase 18 with its ADR-0045 18.1+18.2 split, `ext_proc` at phase 19 with its ADR-0045 19.1+19.2 split, and `oauth2` at phase 20). The next session (lifecycle-state 1 → 2 for phase 21, skill `superpowers:writing-plans` scoped to SPEC authoring per the phase 09/10/11/12/13/14/15/16/17/18/19/20 precedent) authors `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Predecessor master tip:** `4deaa5c` (phase-20 IMPL follow-up STATE.md SHA-fill; pushed to origin). Pre-existing baseline: 25 differential fixtures (0000-0024) green; 26 fuzzers green at 30s; h2spec 53/53 PASS at ADR-0051 pin; build/vet/lint clean; race-tests clean across all packages; **15 HTTP filters wired through boot-registration** (oauth2 added at phase-20); ADR tail at `ADR-0185` (9 NEW landed at phase-20 IMPL + 2 IN-PLACE AMENDMENTs to ADR-0150 + ADR-0159); next-free `ADR-0186`. Gauge support is first-class in `internal/stats/` (`internal/stats/gauge.go::Gauge` over `atomic.Int64` with `Inc`/`Dec`/`Add`/`Set`/`Load`/`Format`).

**Phase 21 in one sentence:** Land `envoy.filters.http.adaptive_concurrency` as a single-phase MEDIUM-readiness §9 family-row, exposing the full Gradient-1 controller proto surface byte-exactly against upstream Envoy v1.37.2 except for the RTDS `enabled.RuntimeFeatureFlag` deferral (PARSE-REJECT of `enabled.runtime_key != ""`), via an inline `Clock` interface seam in the filter package (NO new `internal/clock/` framework primitive) and a two-layer test strategy (subject-only FAKE-TIME algorithmic-fidelity tests + a `0025-http-adaptive-concurrency` structural differential fixture with SPEC-time D-question on partial cross-side byte-exact on the 503-overflow leg).

---

## 1. Mission and scope confirmation (21 only)

### 1.1 What 21 delivers as a self-contained whole

Phase 21 lands the full operator-visible surface of `envoy.filters.http.adaptive_concurrency`:

- **Proto surface** — `AdaptiveConcurrency` wrapping `GradientControllerConfig` (the only oneof arm currently defined in v1.32.4 + v1.37.x), plus `concurrency_limit_exceeded_status` (default 503) and `enabled` (the `RuntimeFeatureFlag`-deferred field — static-default honored; non-empty `runtime_key` PARSE-REJECT).
- **GradientControllerConfig sub-surface** — `sample_aggregate_percentile` (default p50; honored at any percentile in 0..100), `concurrency_limit_calculation_params` (`max_concurrency_limit` default 1000; `concurrency_update_interval` required-positive), `min_rtt_calc_params` (`interval` required-positive; `request_count` default 50; `jitter` default 15%; `min_concurrency` default 3; `buffer` default 25%).
- **Gradient-1 controller state machine** — rolling latency sample window per `concurrency_update_interval` tick; gradient formula `gradient = max(0.5, min(2.0, minRttWithBuffer / sampleAggregateRtt))`; new-limit `max(min_concurrency, currentLimit × gradient + sqrt(currentLimit × gradient))`; periodic minRTT-recalc windows (clamp limit to `min_concurrency`, sample `request_count` requests, take min as new minRTT, restore calculation).
- **In-flight token discipline** — acquire at `decodeHeaders` entry; on overflow, increment `rq_blocked` counter + return `concurrency_limit_exceeded_status` (default 503); release at full-encode completion with RTT sample appended to current window.
- **Stat surface 92 → 99 names** — 1 counter (`rq_blocked`) + 6 gauges (`concurrency_limit`, `gradient`, `burst_queue_size`, `sampled_rtt`, `min_rtt`, `recalculating_min_rtt`) under the upstream-byte-exact `adaptive_concurrency.<config_stat_prefix>.gradient_controller.<stat>` prefix template.
- **Inline `Clock` interface seam** — in-package `Clock` interface (`Now()` + `AfterFunc(d, fn)`) wrapping stdlib; FAKE-TIME implementation lives in the test scope only.
- **Differential fixture `0025-http-adaptive-concurrency`** — REFERENCE-LESS subject-only structural at first cut (parse OK, 503 status when limit exceeded under synthetic high-concurrency load, stat-surface delta, pass-through-when-disabled). SPEC-time D-question on whether the 503-overflow leg can be promoted to partial cross-side byte-exact (via `max_concurrency_limit=1 + min_concurrency=1` + 2 concurrent slow requests — the second 503s deterministically without timing dependency).
- **27th project-wide fuzzer** — `FuzzAdaptiveConcurrencyConfigParse` at the standard ~30-corpus-seed baseline, with PARSE-REJECT arms for `enabled.runtime_key` non-empty, invalid percentile, zero `interval`, zero `request_count`, jitter > 100%, and the oneof-absent case.
- **Subject-only algorithmic-fidelity test layer** — step-driven FAKE-TIME exercise of gradient calc + minRTT recalc + jitter range + burst queue accounting + the 503-on-overflow path, in `internal/filter/http/adaptive_concurrency/controller_test.go`.
- **6-gate phase-done verification** — build / vet+lint / race / 26/26 differential / 27 fuzzers / h2spec 53/53. Same matrix as phase-20.

### 1.2 What 21 does NOT deliver (forward to §8)

See §8 for the explicit deferred-items list. Highlights:

- **RTDS `enabled.RuntimeFeatureFlag` runtime keying** — PARSE-REJECT at config-load; closes after the Runtime/RTDS family lands (per ROADMAP "Runtime + hot restart family" row).
- **Cross-side byte-exact algorithmic parity** — the gradient formula's numeric outputs depend on minRTT timing and sample-window phase; full cross-side byte-comparable algorithmic parity requires either reference Envoy clock injection (not available) or a deterministic-load trap (deferred fixture-extension task).
- **Alternative `ConcurrencyControllerConfig` oneof arms** — the proto oneof has only one defined arm (`GradientControllerConfig`) in v1.32.4 + v1.37.x; if upstream adds a second controller type in a future release, that becomes its own forward-pointer.

### 1.3 Phase-done as a §9 family-row landing

Phase 21 closes the FOURTEENTH §9 family-row (after the 13 listed in the header). The remaining §9 row count drops from 5 to 4 post-phase-21 (`lua`, `wasm`, `admission_control`, `global rate limit`). Phase 21 also retires the `adaptive concurrency` line item from ROADMAP §9 (`Header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit`).

### 1.4 ADR-0045 split-by-surface readiness — LOW-MEDIUM anticipation for phase 21

Per ADR-0045 §6, the split-gate fires when `PLAN.md > ~25 tasks OR > ~1500 LoC estimated`. Phase 21's anticipated surface (one filter package, one inline `Clock` seam, one differential fixture, one new fuzzer, ~1200-1500 LoC) puts it at **LOW-MEDIUM** split-readiness anticipation. Compared to phase-20 (MEDIUM-HIGH at BRAINSTORM, single-phase at SPEC) phase-21 has a less complex surface (no SDS, no AES envelope, no cookie composition, no token-endpoint POST templating, no PKCE-deferred surface) and is more likely to fit a single phase cleanly. The SPEC author re-evaluates if scope drifts.

### 1.5 Seed-stub alignment

No seed-stub for adaptive_concurrency exists in `internal/filter/http/` (consistent with the §9 family-row pattern; each row creates its own package). Phase 21 creates `internal/filter/http/adaptive_concurrency/` from scratch.

### 1.6 No prebrainstorm-notes branch

No `phase-21-http-filter-adaptive-concurrency-prebrainstorm-notes` branch exists. Phase 21 starts cleanly from this BRAINSTORM.md.

### 1.7 Phase 21's relationship to prior framework deltas + framework-delta accretion shape

Phase 21 is the **FIRST §9 family-row since phase 14 (compressor) to introduce ZERO new `internal/` framework primitives**. The prior §9 row landings carried at least one framework delta each:

- Phase 15 bandwidth_limit — REUSE of `internal/ratelimit/` token-bucket primitive
- Phase 16 rbac — NEW `internal/matcher/` (typed matcher framework) + several in-place ADR amendments
- Phase 17 jwt_authn — NEW `internal/jwks/` Fetcher + NEW `internal/jwt/` verifier + extensive matcher reuse
- Phase 18.1 + 18.2 ext_authz — NEW `internal/grpcclient/` framework primitive (gRPC dial pool)
- Phase 19.1 + 19.2 ext_proc — REUSE of the phase-18 `internal/grpcclient/` primitive + extensive matcher reuse
- Phase 20 oauth2 — NEW `internal/httpclient/` + NEW `internal/sdsfile/` framework primitives + IN-PLACE AMENDMENTs to ADR-0150 (jwks Fetcher refactor) + ADR-0159 (extauthz httpAuthClient refactor) + ADR-0159 §Future Work CLOSURE-AT-PHASE-20

Phase 21's framework delta is **NONE NEW + ZERO IN-PLACE AMENDMENTs**. The filter is algorithmically self-contained (the Gradient-1 controller is in-package state) and uses only the existing `internal/stats/` (gauge support already first-class) + the existing HTTP filter framework (per-request decode/encode hooks; HCM-parse-time PARSE-REJECT; freeze-after-boot registry). The inline `Clock` interface seam is in-package only (per Q3 decision below). This makes phase 21 the LOWEST framework-delta §9 row to date.

This is healthy for the project's accretion shape: phase 20 was a framework-delta-heavy row, phase 21 returns to a leaner framework-delta posture, and the next §9 rows (lua, wasm, adaptive_concurrency, admission_control, global rate limit) can be sequenced by their own framework-delta weight without coupling.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The brainstorm dialogue settled 5 Q-decisions. Each is anchored here with rationale + the anticipated ADR or REUSE classification.

### 2.1 Scope ambition: Pragmatic middle *(Q1 → ADR-0186 + ADR-0187)*

**Decision:** Land the FULL Gradient-1 algorithmic surface (all 3 sub-blocks at their tunable parameters: `sample_aggregate_percentile`, full `concurrency_limit_calculation_params`, full `min_rtt_calc_params`) BUT defer the runtime `enabled.RuntimeFeatureFlag` (the Runtime/RTDS family isn't wired yet per ROADMAP). Honor the static-enable case (`enabled.default_enabled` consumed; absent `enabled` field treats filter as enabled per upstream); PARSE-REJECT if `enabled.runtime_key` is non-empty.

**Rationale:** Matches phase-20's RBAC + jwt_authn + ext_authz + ext_proc precedent of deferring RTDS-coupled subfields with a PARSE-REJECT and a forward-pointer ADR to the Runtime family. Avoids landing a RuntimeFeatureFlag-shaped silent fall-through that would mislead operators.

**Anticipated ADRs:** ADR-0186 (gradient controller state machine + the full Gradient-1 surface decision); ADR-0187 (RTDS `enabled.RuntimeFeatureFlag` deferral with PARSE-REJECT).

### 2.2 Differential-fixture strategy: Two-layer taxonomy — FAKE-TIME subject-only + structural cross-side *(Q2 → ADR-0186 same anchor + SPEC §10 D-questions)*

**Decision:** Two-layer test strategy:
- **Layer A — Subject-only algorithmic-fidelity tests** via step-driven `FakeClock` in `internal/filter/http/adaptive_concurrency/controller_test.go`. Deterministic exercise of gradient calc, minRTT recalc windows, jitter range, burst queue accounting, 503-on-overflow.
- **Layer B — Structural cross-side differential** at `test/fixtures/0025-http-adaptive-concurrency/`. REFERENCE-LESS subject-only structural at first cut (parse OK, 503 status when limit exceeded under synthetic high-concurrency load, stat-surface delta, pass-through-when-disabled). SPEC-time D-question on partial cross-side byte-exact on the 503-overflow leg.

**Rationale:** The Gradient-1 algorithm is timer-driven + load-dependent, so cross-side byte-exact algorithmic parity requires either (a) reference Envoy clock injection (not available) or (b) a deterministic-load trap. Splitting the test surface into a deterministic FAKE-TIME subject-only layer + a structural cross-side layer gives both algorithmic fidelity AND wire-shape parity without coupling them.

**Anticipated ADR anchor:** ADR-0186 §Consequences includes the FAKE-TIME differential strategy + the two-layer test taxonomy.

### 2.3 Clock seam: Inline `Clock` interface *(Q3 → ADR-0186 same anchor)*

**Decision:** Define a small `Clock` interface in `internal/filter/http/adaptive_concurrency/clock.go` (`Now() time.Time` + `AfterFunc(d time.Duration, fn func()) Stop`), with `defaultClock` wrapping `time.Now` + `time.AfterFunc`. NO new `internal/clock/` framework primitive. NO new ADR for the primitive. Defers the reuse decision until a second consumer materializes.

**Rationale:** YAGNI-aligned. Phase 20 introduced 2 new framework primitives (`internal/httpclient/`, `internal/sdsfile/`) where the consumer count was already ≥2 at extraction time. The Clock seam has only ONE current consumer (adaptive_concurrency); the next plausible consumers (admission_control, global rate limit) are both speculative §9 rows. Following the EXTRACT-NOW-only-when-the-trigger-fires discipline from ADR-0159's §Future Work clause, phase 21 stays inline. If the Clock seam ends up serving a second timer-driven filter in a future §9 row, that phase's BRAINSTORM revisits the extraction-NOW trigger.

**Anticipated ADR anchor:** ADR-0186 §Decision includes the inline-Clock-vs-extracted-primitive decision; the extraction-NOW trigger (≥2 consumers) is documented as a forward-pointer in §Consequences.

### 2.4 Phase split: Single phase *(Q4 → ADR-0045 split-gate stays AT REST at BRAINSTORM)*

**Decision:** Land all of the gradient controller + clock seam + filter wiring + parse + PARSE-REJECT + 7-name stat surface + FAKE-TIME unit tests + the `0025-*` structural cross-side fixture in a single phase 21. Matches phase-20 oauth2 precedent (one phase, 14 tasks + 4 follow-ups). If LoC drifts past the ADR-0045 gate during PLAN, split then per ADR-0045 §6 discipline.

**Rationale:** Phase-21's anticipated LoC (~1200-1500) + task count (~14-16) sits below the split-gate. The natural split axes (controller vs fixture; framework vs filter) don't carve cleanly because the controller state machine and the FAKE-TIME tests are tightly coupled, and the framework delta is zero. SPEC author may revisit at SPEC-time if scope drifts.

### 2.5 Stat surface: Full upstream 7-name parity *(Q5 → ADR-0186 + ADR-0188-candidate)*

**Decision:** Expose the full upstream stat surface: 1 counter (`rq_blocked`) + 6 gauges (`concurrency_limit`, `gradient`, `burst_queue_size`, `sampled_rtt`, `min_rtt`, `recalculating_min_rtt`) under the `adaptive_concurrency.<config_stat_prefix>.gradient_controller.<stat>` prefix template byte-exactly against upstream Envoy v1.37.2. Project stat count 92 → 99.

**Rationale:** Phase-20 set the precedent of full operator-visible stat surface (6 counters wire-exact upstream). Gauge support is already first-class in `internal/stats/gauge.go`. The 7-name surface gives operators full capacity-planning + incident-triage observability (counter for rejection rate; gauges for limit/gradient/burst/sampled-RTT/minRTT/recalc-state).

**Anticipated ADRs:** ADR-0186 §Consequences includes the 7-name stat surface roster. **ADR-0188 candidate** (speculative; could be in-place §Decision body on a stats-family ADR instead of a new one) for the float-valued-gauge int64-encoding convention (nanoseconds for time-typed gauges; `×1000` scaling for ratio-typed `gradient`). The SPEC author makes the ADR-0188-vs-in-place-amendment call.

---

## 3. Framework-survey result — ZERO NEW package-level + ZERO IN-PLACE AMENDMENTs + 7 REUSES

Phase 21 is the **LEANEST framework-delta §9 row to date**. Per the §1.7 accretion-shape analysis, phase 21 introduces **NO new `internal/` packages** and **NO in-place §Decision amendments** to prior ADRs. The full surface is constructed from existing primitives + a single in-package inline `Clock` interface.

### 3.1 NO NEW: `internal/clock/` framework primitive *(per Q3 decision; future-trigger forward-pointer)*

**Decision:** Inline `Clock` interface in `internal/filter/http/adaptive_concurrency/clock.go`. NOT extracted as a framework primitive. Forward-pointer to a future-phase EXTRACT-NOW trigger when a second timer-driven filter (admission_control, global rate limit, or similar) lands.

### 3.2 REUSE 1: `internal/stats/` Counter + Gauge support

`internal/stats/registry.go` and `internal/stats/gauge.go` already host first-class counter + gauge support. The 7-name stat surface is constructed via `Registry.NewCounter` + `Registry.NewGauge` (or the `NewGaugeIfAbsent` variant for the per-HCM-instance scoping). No framework work.

### 3.3 REUSE 2: HTTPRegistry boot-time registration

`internal/filter/http/registry.go::HTTPRegistry` already supports `Register(typeURL, factory)` + `Freeze` + `Lookup`. Boot wires `adaptiveconcurrency.New` at `cmd/envoy-go/main.go` between `router.New` (alphabetical first) and `bandwidthlimit.New` (alphabetical second). 16 HTTP filters wired post-phase-21.

### 3.4 REUSE 3: Per-request filter interface (decode/encode hooks)

The existing HCM filter framework already supports per-request filter instances with decodeHeaders/encodeHeaders/encodeData callbacks. Adaptive_concurrency's acquire-at-decode + release-at-encode-complete pattern fits cleanly without framework extension.

### 3.5 REUSE 4: HCM-parse-time PARSE-REJECT path

The existing HCM parser already rejects unknown type_urls + invalid typed_config bodies at parse time. Adaptive_concurrency's parse logic adds: invalid percentile (out of 0..100), zero `interval`, zero `request_count`, jitter > 100%, `enabled.runtime_key != ""`, and the oneof-absent case.

### 3.6 REUSE 5: REUSE-by-absence per-route enforcement

The adaptive_concurrency proto has NO `AdaptiveConcurrencyPerRoute` message. Any attempt to configure per-route adaptive_concurrency is already a proto-deserialization-time PARSE-REJECT via the existing HCM filter framework. Same REUSE-by-absence pattern as oauth2 / jwt_authn / extauthz / rbac listener-scoped-only at chain entry. **FOURTH CONSECUTIVE §9 row to skip ADR-0125 amendment** (extending the third-consecutive run from phase-20).

### 3.7 REUSE 6: Existing fuzzer-corpus framework

`internal/filter/http/fuzz_test.go` already hosts the cross-filter fuzzer registry. Adaptive_concurrency adds `FuzzAdaptiveConcurrencyConfigParse` as the 27th project-wide fuzzer at the standard ~30-corpus-seed baseline + the standard PARSE-REJECT arm coverage.

### 3.8 REUSE 7: Existing differential-fixture framework

`test/fixtures/` + `test/differential/runner_test.go` already host the differential fixture runner. Adaptive_concurrency adds `0025-http-adaptive-concurrency/` per the REFERENCE-LESS subject-only structural pattern (matching phase-20's 0024-http-oauth2 fixture precedent). SPEC-time D-question on partial cross-side byte-exact for the 503-overflow leg.

---

## 4. Per-route shape — REUSE-by-absence (NO new canonical, NO ADR-0125 amendment)

The adaptive_concurrency proto defines NO `AdaptiveConcurrencyPerRoute` message in v1.32.4 or v1.37.x. This makes per-route adaptive_concurrency configuration a proto-deserialization-time PARSE-REJECT (the `typed_per_filter_config` map entry would fail to deserialize as `AdaptiveConcurrency` because the wire bytes wouldn't match the message). The HCM filter framework's existing PARSE-REJECT path catches this without framework extension.

**Classification:** REUSE-by-absence. Phase 21 makes this the **FOURTH CONSECUTIVE §9 row** to skip ADR-0125 (per-route canonical roster) amendment. Phase 20's classification was THIRD-CONSECUTIVE; phase 21 extends the run.

**No new canonical entry** in ADR-0125 §canonical-per-route-roster. **No ADR-0125 amendment** anchored at phase 21.

---

## 5. Stat surface hypothesis

### 5.1 7-name surface roster

Per Q5 decision, phase 21 exposes the FULL upstream stat surface. The roster is anchored against upstream Envoy v1.37.2 `source/extensions/filters/http/adaptive_concurrency/`:

| Name | Type | Semantics | Encoding (envoy-go) |
|---|---|---|---|
| `rq_blocked` | Counter | Requests rejected with `concurrency_limit_exceeded_status` (default 503) | int64 monotonic |
| `concurrency_limit` | Gauge | Current concurrency limit | int64 (raw uint32 value) |
| `gradient` | Gauge | Current gradient (last calculated) | int64 (×1000 scale; e.g., gradient=1.5 → stored 1500) — SPEC-time D-question |
| `burst_queue_size` | Gauge | Current burst queue size (in-flight count above limit) | int64 (signed) |
| `sampled_rtt` | Gauge | Aggregated sample RTT (percentile-summarized) | int64 (nanoseconds) — SPEC-time D-question |
| `min_rtt` | Gauge | Current minRTT estimate | int64 (nanoseconds) — SPEC-time D-question |
| `recalculating_min_rtt` | Gauge | 1 if in minRTT recalc window, 0 otherwise | int64 (0/1) |

### 5.2 Stat-prefix template

Upstream Envoy publishes the surface under `<filter_stat_prefix>.adaptive_concurrency.<config_stat_prefix>.gradient_controller.<stat>`. The exact template byte-exactness against upstream Envoy v1.37.2 is a **SPEC-time §10 empirical pin** obligation per ADR-0004.

### 5.3 Project stat count delta

92 → 99 names. Comparable to phase-20's 86 → 92 delta (+6) — phase-21 adds +7 (1 counter + 6 gauges; phase-20 was 6 counters + 0 gauges).

### 5.4 Float-valued gauge encoding convention — ADR-0188-candidate

The 3 float-valued gauges (`gradient`, `sampled_rtt`, `min_rtt`) need int64 encoding because `internal/stats/gauge.go::Gauge` is int64-typed. Hypothesis:
- Time-typed gauges (`sampled_rtt`, `min_rtt`) → encode as nanoseconds int64 (ns gives 292-year range at int64; ample headroom).
- Ratio-typed gauge (`gradient`) → encode as `×1000` int64 (gradient is bounded 0.5..2.0; ×1000 gives 500..2000 with 3-digit precision).

The SPEC author makes the call between (a) anchoring this as ADR-0188 (new framework convention) or (b) anchoring as in-place §Decision body on an existing stats-family ADR. Either way, this is a forward-pointer for any future float-gauge consumer.

### 5.5 envoy-go-strict departure flag — NONE anticipated

No `envoy-go-strict` departure flag is anticipated at phase 21. The 7-name surface matches upstream byte-exactly; the only departures are the encoding convention (above; ADR-0188-candidate) and the SPEC-time D-question on stat-prefix template byte-exactness (which closes via §10 empirical pin).

---

## 6. Differential fixture envelope — `0025-http-adaptive-concurrency`

### 6.1 Fixture shape

REFERENCE-LESS subject-only structural at first cut (matching phase-20's 0024-http-oauth2 precedent). The fixture asserts wire-shape invariants against the encoded probe block emitted by `DriveSubject`:

- **Parse OK** — adaptive_concurrency filter loads with full Gradient-1 config; admin endpoint exposes the 7-name stat surface.
- **503 on overflow** — driving 2 concurrent slow requests through a config with `max_concurrency_limit=1 + min_concurrency=1` produces 1 success + 1 503 (with the configured `concurrency_limit_exceeded_status` body).
- **Stat surface delta** — admin `/stats` exposes the full 7-name surface with the expected starting values (limit at `min_concurrency`, gradient at 1.0, etc.).
- **Pass-through when disabled** — config with `enabled.default_enabled = false` passes all requests without 503 + without counter increment.

### 6.2 Cross-side byte-exact promotion candidate — SPEC-time D-question

The 503-overflow leg may be promotable to partial cross-side byte-exact via the `max_concurrency_limit=1 + min_concurrency=1 + 2 concurrent slow requests` trap. The trap is deterministic without timing dependency (the controller's static config forces the second request to overflow regardless of latency), so reference Envoy v1.37.2 would produce the same wire shape on the 503 leg. **Default hypothesis: YES** (the trap is timing-independent, so the cross-side promotion should materialize). SPEC-time D-question: SPEC author IN-SESSION verifies via §10 empirical pin against reference Envoy v1.37.2 (per §10 D2). If verification holds, the fixture promotes from REFERENCE-LESS subject-only structural to partial cross-side byte-exact on the 503 leg; if verification falsifies the hypothesis, the fixture stays REFERENCE-LESS subject-only structural.

### 6.3 Listener topology

Single listener with a single HCM containing the adaptive_concurrency filter (placed alphabetical-first in the filter chain) + router terminator. No multi-listener topology anticipated.

---

## 7. Anticipated ADRs — 2-3 ADRs (ADR-0186..ADR-0188; ADR-0188 escape-valve reserve)

### 7.1 ADR-0186 — Gradient-1 controller state machine + clock seam inline + FAKE-TIME differential strategy

**Decision body summary:** The Gradient-1 controller state machine (in-package per-HCM-instance state with `currentLimit`/`inFlight`/`samples`/`minRTT`/`recalculatingMinRTT` + per-window concurrency-update timer + per-interval minRTT-recalc timer); inline `Clock` interface seam (NOT framework primitive); sorted-slice percentile aggregation (NOT CircllHist); gradient formula bit-exact citation against upstream `source/extensions/filters/http/adaptive_concurrency/controller/gradient_controller.cc` (SPEC-time D-question on line-exact lemmata citation depth); two-layer test taxonomy (FAKE-TIME subject-only algorithmic-fidelity + structural cross-side differential).

**Consequences summary:** No new framework primitive; documented EXTRACT-NOW trigger for a second timer-driven filter; full 7-name stat surface; SPEC-time D-question slate (D1: float-gauge encoding convention; D2: 503-overflow cross-side promotion; D3: gradient_controller.cc line-citation depth; D4: stat-prefix template byte-exact pin).

### 7.2 ADR-0187 — RTDS `enabled.RuntimeFeatureFlag` deferral PARSE-REJECT

**Decision body summary:** Per the Pragmatic-middle Q1 decision, the `enabled.RuntimeFeatureFlag` field is honored only for the static-default case (`enabled.default_enabled` consumed; absent `enabled` field treats filter as enabled per upstream). Any `enabled.runtime_key != ""` triggers HCM-parse-time PARSE-REJECT with a forward-pointer to the future Runtime/RTDS family phase.

**Consequences summary:** Operators can statically enable/disable the filter via config; runtime feature-flag keying is a forward-pointer; the PARSE-REJECT path uses the existing HCM-parse-time framework.

### 7.3 ADR-0188 *(candidate; speculative)* — Float-valued gauge encoding convention

**Decision body summary (if anchored):** Time-typed gauges encode as nanoseconds int64; ratio-typed gauges encode as `×1000` int64. Documents a project-wide convention for any future float-gauge consumer.

**Consequences summary (if anchored):** Establishes the convention for the `internal/stats/` framework without modifying `gauge.go`. If the SPEC author opts for in-place §Decision body on an existing stats-family ADR instead, this slot stays UNCONSUMED.

### 7.4 D11-style hypothesis

Following phase-20's D11 hypothesis pattern (next-free ADR stays UNCONSUMED at phase-done), phase-21 hypothesizes that **ADR-0189 stays UNCONSUMED at phase-21 phase-done** (carrying the next-free escape valve forward to phase 22). Most-likely escape-valve surfaces hypothesized at PLAN time (PKCS#7-padding-like edge cases; fsnotify-event-debounce-like edge cases; urlEncode-charset-like edge cases) all close via in-place §Decision body wording at existing ADRs (ADR-0186 algorithmic edge cases; ADR-0187 runtime-key validation edge cases; ADR-0188 encoding edge cases).

If a surprise lands at IMPL (analogous to phase-20's auth-code POST wire-up gap or HMAC `domain` empty-string subtlety), it may force ADR-0189 consumption. The hypothesis is HOLD-with-known-risk, not GUARANTEED-HOLD.

---

## 8. Deferred items (~6-8 items; comparable to phase-20's 6-item)

The following are explicitly NOT delivered at phase 21 and are documented forward-pointers carried into a future phase.

1. **RTDS `enabled.RuntimeFeatureFlag` runtime keying** — PARSE-REJECT at config-load per Q1 + ADR-0187. Closes after the Runtime/RTDS family phase (per ROADMAP).

2. **Cross-side byte-exact algorithmic parity** — the gradient formula's numeric outputs depend on minRTT timing + sample-window phase; full cross-side byte-comparable algorithmic parity requires reference Envoy clock injection (not available) or a deterministic-load trap. Phase 21's two-layer strategy (FAKE-TIME subject-only + structural cross-side) is the chosen compromise. A future fixture-extension task could explore the cross-side promotion via the 503-overflow leg deterministic trap (SPEC-time D-question 2).

3. **Alternative `ConcurrencyControllerConfig` oneof arms** — the proto oneof has only one defined arm (`GradientControllerConfig`) in v1.32.4 + v1.37.x. If upstream adds a second controller type (e.g., Vegas, PI, or a TCP-style cwnd controller) in a future release, phase 21 would need an oneof-fallthrough extension. Currently a no-op forward-pointer.

4. **CircllHist (or HDR Hist) percentile aggregation** — phase 21 uses sorted-slice percentile (request_count default 50 + tunable). Upstream Envoy uses CircllHist (log-linear histogram with ~4% bin precision). The numeric divergence is at most one bin-width at the percentile boundary; phase-21 explicitly accepts this divergence per Q1 (Pragmatic middle). A future algorithmic-fidelity-extension task could swap to CircllHist if cross-side byte-exact algorithmic parity becomes load-bearing.

5. **Stat-prefix template byte-exact pin** — the upstream stat-prefix template (`<filter_stat_prefix>.adaptive_concurrency.<config_stat_prefix>.gradient_controller.<stat>`) is anchored as a SPEC-time §10 empirical pin obligation per ADR-0004. Closure happens at SPEC-time IN-SESSION verification, not as a deferred future-work item.

6. **Float-gauge encoding convention anchor** — ADR-0188-candidate or in-place §Decision amendment on an existing stats-family ADR. SPEC-time decision.

7. **gradient_controller.cc line-exact lemmata citation depth** — SPEC-time D-question 3. Either anchor as ADR-0186 line-exact citations (e.g., "gradient formula per gradient_controller.cc:142-145") or stay SPEC-prose-only.

8. **Multi-listener topology variant** — if a future fixture-extension task needs to exercise per-listener-scoped controller state isolation (e.g., two listeners with identical config produce 2 independent controllers), that's a forward-pointer. Phase-21's single-listener fixture is sufficient for phase-done.

---

## 9. Cross-references against phase-17 + phase-18 + phase-19 + phase-20 deferred-items lists — closure pickup

Phase-21 picks up ZERO closures from prior phases' deferred-items lists. The various per-phase deferral lists (phase-17 jwt_authn carry-forwards; phase-18.x ext_authz carry-forwards; phase-19.1+19.2 ext_proc carry-forwards including the 18+ item §8 deferral list; phase-20 oauth2 6-item future-work register) continue unchanged through phase 21 — phase 21 lands a NEW filter (adaptive_concurrency) and does NOT pick up cross-filter deferred items.

The phase-20 future-work item that comes CLOSEST to phase-21 relevance is the **HMAC `domain` empty-string subtlety** (phase-20 REVIEW.md §6 item 4) which is purely an oauth2-package concern and does NOT touch adaptive_concurrency. Phase 21 leaves it alone.

**Notable: the ADR-0159 §Future Work CLOSURE-AT-PHASE-20 milestone (the FIRST §9 family-row to CLOSE a prior-phase load-bearing forward-pointer) is NOT extended at phase 21.** Phase 21 closes NO prior forward-pointer because it introduces NO new framework primitives that would trigger one. This is healthy for the project's accretion shape: not every phase needs a CLOSURE-AT milestone, and forced-closure pickups would distort the EXTRACT-NOW trigger discipline.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

The following 4 D-questions surface during BRAINSTORM and are deferred to SPEC-time:

- **D1 (Float-gauge encoding convention)** — anchor as ADR-0188 new or in-place §Decision on an existing stats-family ADR? Default hypothesis: in-place §Decision on a stats-family ADR (e.g., ADR-0059 or a sibling) to avoid ADR consumption for an encoding convention. SPEC author decides.

- **D2 (503-overflow cross-side promotion)** — can the 503-overflow leg of the `0025-http-adaptive-concurrency` fixture be promoted from REFERENCE-LESS subject-only structural to partial cross-side byte-exact via the `max_concurrency_limit=1 + min_concurrency=1 + 2 concurrent slow requests` deterministic trap? Default hypothesis: YES (the trap is timing-independent), but the SPEC author IN-SESSION verifies via §10 empirical pin against reference Envoy v1.37.2.

- **D3 (gradient_controller.cc line-citation depth)** — anchor ADR-0186 with line-exact lemmata citations (e.g., "gradient formula per gradient_controller.cc:142-145") or stay SPEC-prose-only? Default hypothesis: line-exact citations for the gradient formula + minRTT recalc trigger; SPEC-prose-only for the in-flight token discipline (which is implementation-style choice, not algorithmic invariant).

- **D4 (Stat-prefix template byte-exact pin)** — the `<filter_stat_prefix>.adaptive_concurrency.<config_stat_prefix>.gradient_controller.<stat>` template needs IN-SESSION verification against reference Envoy v1.37.2 per ADR-0004. SPEC author runs the empirical pin and either ratifies the template or amends it.

Additional SPEC-time D-questions may surface during the SPEC's §13 phase-21-specific D-question slate; these 4 are the BRAINSTORM-anchored set.

---

## 11. Phase-17 + phase-18 + phase-19 + phase-20 §10/§11 lessons applied

Phase-21 BRAINSTORM is shaped by lessons from the prior 4 §9 row brainstorms:

- **Phase-17 (jwt_authn) lesson — framework primitives extract NOW only when the consumer count is ≥2 at extraction time.** Applied to Q3: the Clock seam stays inline because the consumer count is 1. Future EXTRACT-NOW trigger documented in ADR-0186 §Consequences.

- **Phase-18 (ext_authz HTTP+gRPC split) lesson — pre-emptive phase splits are expensive (2 SPECs, 2 PLANs, 2 REVIEWs) and only justify themselves when the surface has a clear protocol-axis split.** Applied to Q4: single phase because the Gradient-1 algorithmic surface has no clean split axis (no protocol axis; no transport axis; no headers-vs-body axis).

- **Phase-19 (ext_proc headers+body split) lesson — same as phase-18; the protocol-axis split was justified there because the body-streaming logic genuinely required a separate landing.** Phase 21 has no analog — the in-flight token discipline + the controller state machine are tightly coupled and cannot be cleanly split.

- **Phase-20 (oauth2) lesson — full proto surface with PARSE-REJECT for proto-defined-but-RTDS-coupled subfields is the standard pattern.** Applied to Q1: full Gradient-1 surface, PARSE-REJECT for `enabled.runtime_key != ""`. ADR-0187 documents the deferral.

- **Phase-20 (oauth2) lesson — REFERENCE-LESS subject-only structural differential fixtures are acceptable when cross-side byte-exact algorithmic parity is timing-dependent.** Applied to Q2: phase-21 fixture starts REFERENCE-LESS subject-only structural; SPEC-time D-question 2 explores the cross-side promotion candidate.

- **Phase-20 (oauth2) lesson — the D11 next-free-ADR-stays-UNCONSUMED hypothesis is a useful planning anchor + a useful phase-done check.** Applied to §7.4: phase-21 hypothesizes ADR-0189 stays UNCONSUMED at phase-done.

- **Phase-20 (oauth2) lesson — the 18-item SPEC §15 acceptance checklist style is a reliable closure-discipline anchor.** Applied to the SPEC author's prompt: phase-21 SPEC will have a similar 15-20-item §15 acceptance checklist anchored to the BRAINSTORM Q-decisions + the §10 empirical-pin obligations + the ADR-0186/0187 §Consequences + the deferred-items §8 register.

- **Phase-20 (oauth2) lesson — `internal/httpclient/` + `internal/sdsfile/` framework primitives were extracted cleanly because the consumer count was ≥2 at extraction time.** Applied to §3: phase-21's Clock seam stays inline (consumer count = 1).

---

## 12. Section closeout

This BRAINSTORM.md is **lifecycle-state 0 → 1 complete** for phase 21. The next session (lifecycle-state 1 → 2, skill `superpowers:writing-plans` scoped to SPEC authoring) authors `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/SPEC.md` based on:

1. The 5 BRAINSTORM-time Q-decisions (§2.1-§2.5).
2. The framework-survey result (§3): ZERO new packages, ZERO in-place amendments, 7 REUSES.
3. The per-route REUSE-by-absence classification (§4): FOURTH-CONSECUTIVE §9 row to skip ADR-0125 amendment.
4. The stat surface hypothesis (§5): 92 → 99; 1 counter + 6 gauges under the upstream byte-exact prefix template.
5. The differential fixture envelope (§6): `0025-http-adaptive-concurrency` REFERENCE-LESS subject-only structural at first cut; SPEC-time D-question 2 on the 503-overflow cross-side promotion.
6. The anticipated ADRs (§7): ADR-0186 + ADR-0187 + ADR-0188-candidate; D11-style hypothesis that ADR-0189 stays UNCONSUMED at phase-21 phase-done.
7. The deferred-items register (§8): 6-8 forward-pointers carried to future phases.
8. The 4 SPEC-time D-questions (§10): D1 encoding convention; D2 cross-side promotion; D3 line-citation depth; D4 stat-prefix template empirical pin.
9. The phase-17/18/19/20 lessons applied (§11).

The SPEC author is responsible for executing the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004, including the stat-prefix template pin (D4) and any other surface-level pins that surface during SPEC drafting.

**Hand-off:** BRAINSTORM-time scope is complete. SPEC author proceeds.
