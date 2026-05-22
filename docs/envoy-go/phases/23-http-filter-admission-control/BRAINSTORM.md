# Phase 23 Brainstorm — `envoy.filters.http.admission_control`

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 23 (`http-filter-admission-control`), the SIXTEENTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at 07.1, `fault` at 09, `header_mutation` at 10, `local_ratelimit` at 11, `csrf` at 12, `buffer` at 13, `compressor` at 14, `bandwidth_limit` at 15, `rbac` at 16, `jwt_authn` at 17, `ext_authz` at 18 with its ADR-0045 18.1+18.2 split, `ext_proc` at 19 with its ADR-0045 19.1+19.2 split, `oauth2` at 20, `adaptive_concurrency` at 21, and `lua` at 22 with its ADR-0045 22.1+22.2+22.3 three-way split). The next session (lifecycle-state 1 → 2 for phase 23, skill `superpowers:writing-plans` scoped to SPEC authoring per the phase 09..22 precedent) authors `docs/envoy-go/phases/23-http-filter-admission-control/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Predecessor master tip:** `032510a` (phase 22.3 IMPL follow-up: STATE.md SHA-fill → `c999312`; pushed to origin). Pre-existing baseline at master tip: **31 differential fixture directories** GREEN (0000-0029; lua fixtures 0026/0027/0028/0029 GREEN in isolation — combined multi-listener runs may hit the documented pre-existing `freeTCPPort` port-allocation flake per 22.2 REVIEW §7.4, not a defect); **31 fuzzers** green; h2spec 53/53 PASS at ADR-0051 v1.32.4 pin; build/vet/lint clean; race-tests clean; **17 HTTP filters wired** through boot-registration; **107 stat names**; **14 envoy-go-strict departure records** at BEHAVIOR_CONTRACT `### envoy.filters.http.lua`. ADR tail at **ADR-0193 full body + ADR-0125 §(xiv) amended**; **next-free ADR-0194**. Gauge support is first-class in `internal/stats/` (`internal/stats/gauge.go`). The phase-21 inline `Clock` interface seam (`internal/filter/http/adaptive_concurrency/clock.go`) is the reuse template for phase 23's time seam.

**Phase 23 in one sentence:** Land `envoy.filters.http.admission_control` as a single-phase §9 family-row exposing the FULL operator-visible SRE-book client-side admission-control surface byte-exactly against upstream Envoy v1.37.2 EXCEPT for the RTDS `runtime_key` deferral (PARSE-REJECT of any non-empty `runtime_key` inside the `Runtime{FeatureFlag,Double,Percent,UInt32}` wrappers, honoring the static default), via two inline interface seams in the filter package (a `Clock` seam mirroring phase-21 + a NEW `Rand` seam for the probabilistic rejection dice-roll — both in-package, NO new `internal/` framework primitive) and a test strategy of subject-only deterministic-regime algorithmic tests + a two-directory differential (`00NN-http-admission-control` cross-side on deterministic regimes + `00NN+1-http-admission-control-boot-reject` boot-reject).

---

## 1. Mission and scope confirmation (23 only)

### 1.1 What 23 delivers as a self-contained whole

Phase 23 lands the full operator-visible surface of `envoy.filters.http.admission_control`:

- **Proto surface** — `AdmissionControl` with `enabled` (`RuntimeFeatureFlag`-deferred — static default honored; non-empty `runtime_key` PARSE-REJECT), the `evaluation_criteria` oneof (only arm currently defined: `success_criteria`), `sampling_window` (Duration; default 30s), `aggression` (`RuntimeDouble`; default 1.0), `sr_threshold` (`RuntimePercent`; default 95%), `rps_threshold` (`RuntimeUInt32`; default 0), `max_rejection_probability` (`RuntimePercent`; default 80%).
- **SuccessCriteria sub-surface** — `http_criteria` (`HttpCriteria.http_success_status`: repeated `Int32Range`; default 2xx) + `grpc_criteria` (`GrpcCriteria.grpc_success_status`: repeated uint32; default OK=0). Both arms landed (full `success_criteria`, not HTTP-only).
- **Admission-control algorithm (SRE-book client-side rejection)** — over the `sampling_window` sliding count of `{requests, successes}`: `P_reject = max(0, (requests − aggression⁻¹·successes) / (requests + 1))`, clamped above by `max_rejection_probability`, suppressed while the windowed request rate is below `rps_threshold`, and gated by the `sr_threshold` success-rate floor. Exact formula + clamp/gate ordering bit-exact-cited against upstream `source/extensions/filters/http/admission_control/` at SPEC-time (§10 D-question).
- **Both-sides per-request discipline** — `DecodeHeaders` computes `P_reject`, draws from the inline `Rand` seam, and on a reject outcome short-circuits with `SendLocalReply` 503 (`rq_rejected` counter + the upstream-exact local-reply body/headers, pinned at SPEC). `EncodeComplete` classifies the upstream response per `success_criteria` and records it into the current window bucket (`rq_success` / `rq_error`).
- **Two inline interface seams (NO framework primitive)** — `Clock` (`Now()` + bucket-expiry timing; mirrors phase-21's inline-Clock decision) + NEW `Rand` (`Float64()` in [0,1) for the dice-roll). Both in-package; FAKE implementations live in the test scope only.
- **Stat surface 107 → 110** — anticipated 3 counters (`rq_rejected`, `rq_success`, `rq_error`) under the upstream-byte-exact `http.<HCM_stat_prefix>.admission_control.<stat>` prefix template. Byte-exact upstream parity; **NO extra-upstream gauges** (the mission is conformance — upstream exposes no rejection-probability gauge, so neither do we). Exact roster + prefix pinned at SPEC-time per ADR-0004.
- **Two-directory differential** — `00NN-http-admission-control` (cross-side, deterministic regimes only: P_reject=0 healthy-backend all-admit byte-exact + a forced-reject leg via threshold manipulation → deterministic 503 + subject-only structural for the sampling math) + `00NN+1-http-admission-control-boot-reject` (boot-reject, a shared PGV-mirror reject e.g. `sr_threshold > 100%`, common stderr substring). **Fixture count 31 → 33.** Per project memory `reference_differential_fixture_dispatch_constraint`, the runner dispatches ONE branch per fixture directory (cross-side XOR boot-reject), so the cross-side and boot-reject surfaces are SEPARATE directories from the start.
- **32nd project-wide fuzzer** — `FuzzAdmissionControlConfigParse` at the standard ~30-corpus-seed baseline, with PARSE-REJECT arms for non-empty `runtime_key` (each of the four Runtime* wrappers), `evaluation_criteria` oneof-absent, out-of-range `sr_threshold` / `max_rejection_probability` percent, non-positive `aggression`, and malformed `Int32Range`. **Fuzzer count 31 → 32.**
- **Subject-only deterministic-regime algorithmic test layer** — exact `P_reject` boundary math via the injected `Rand` seam + window expiry via the injected `Clock` seam + success/error classification (http + grpc) in `internal/filter/http/admission_control/*_test.go`.
- **6-gate phase-done verification** — build / vet+lint / race / differential / fuzzers / h2spec 53/53. Same matrix as phase-09..22.

### 1.2 What 23 does NOT deliver (forward to §8)

See §8 for the explicit deferred-items list. Highlights: RTDS `runtime_key` live-reload (PARSE-REJECT at config-load — Runtime/RTDS family); any future `AdmissionControlPerRoute` if upstream adds one (none exists in v1.37.2); alternative/extra observability gauges beyond the 3 upstream counters; cross-side byte-exact parity of the *probabilistic* path (intrinsically un-matchable against a foreign RNG — only the deterministic P=0/forced-reject regimes are cross-side).

### 1.3 Phase-done as a §9 family-row landing

Phase 23 closes the SIXTEENTH §9 family-row. The remaining §9 row count drops from 3 to 2 post-phase-23 (`wasm`, `global rate limit`). Phase 23 retires the `admission control` line item from the ROADMAP §9 HTTP-filters family list.

### 1.4 ADR-0045 split-by-surface readiness — LOW anticipation for phase 23

Per ADR-0045 §6, the split-gate fires when `PLAN.md > ~25 tasks OR > ~1500 LoC estimated`. Phase 23's anticipated surface (one filter package, two inline seams, two differential directories, one new fuzzer, ~900-1300 LoC) puts it at **LOW** split-readiness — smaller than phase-21 adaptive_concurrency (no gradient state machine, no minRTT recalc windows, no per-window timer cascade; the admission-control algorithm is a single sliding-window success-rate count + one probability formula). **Single-phase landing** (Q-split below). SPEC author re-evaluates if scope drifts.

### 1.5 Seed-stub alignment

No seed-stub for admission_control exists in `internal/filter/http/` (consistent with the §9 family-row pattern; each row creates its own package). Phase 23 creates `internal/filter/http/admission_control/` from scratch.

### 1.6 No prebrainstorm-notes branch

No `phase-23-*-prebrainstorm-notes` branch exists. Phase 23 starts cleanly from this BRAINSTORM.md.

### 1.7 Framework-delta posture — ZERO NEW primitives + ZERO IN-PLACE AMENDMENTs

Phase 23 returns to the LEAN framework-delta posture of phase 21 (after phase 22's substantial `internal/lua/` framework primitive + the ADR-0125 §(xiv) amendment). The filter is algorithmically self-contained (the success-rate sliding window + the rejection-probability formula are in-package per-HCM-instance state) and uses only the existing `internal/stats/` (counter support) + the existing HTTP filter framework (per-request decode/encode hooks; HCM-parse-time PARSE-REJECT; freeze-after-boot registry). The two inline seams (`Clock` + `Rand`) are in-package only (per the seam decision below). Anticipated delta: **NONE NEW + ZERO IN-PLACE AMENDMENTs.**

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The brainstorm dialogue settled 2 user-decided Q-decisions (MVP envelope + the probabilistic-rejection seam) and 5 precedent-settled defaults (per-route disposition, stat surface ambition, phase split, Clock seam, test taxonomy). Each is anchored here.

### 2.1 Scope ambition: Full surface minus RTDS *(Q1, user-decided → ADR-0194 + ADR-0195)*

**Decision:** Land the FULL operator-visible algorithm — `success_criteria` (both `http_criteria` + `grpc_criteria` arms), `sampling_window`, the `aggression` curve, `sr_threshold`, `rps_threshold`, `max_rejection_probability` — BUT defer the RTDS `runtime_key` inside every `Runtime{FeatureFlag,Double,Percent,UInt32}` wrapper. Honor the static default value of each wrapper; PARSE-REJECT if any `runtime_key` is non-empty.

**Rationale:** Matches the phase-17..22 precedent of deferring RTDS-coupled subfields with a PARSE-REJECT + a forward-pointer ADR to the Runtime family. The `aggression` curve and `rps_threshold` are core to the algorithm's operator-visible behavior (a linear-only or rps-less MVP would be a non-canonical partial algorithm), and `grpc_criteria` is a first-class proto arm whose omission would leave a visible gap. Full surface minus RTDS is the established §9 standard.

**Anticipated ADRs:** ADR-0194 (admission-control algorithm + package shape + seams + differential strategy); ADR-0195 (RTDS `runtime_key` deferral PARSE-REJECT).

### 2.2 Probabilistic-rejection seam: Inline `Rand` seam + deterministic-regime fixtures *(Q2, user-decided → ADR-0194 same anchor)*

**Decision:** Add an inline injectable `Rand` interface seam in the filter package (`Float64() float64` in [0,1)), with a `defaultRand` wrapping `math/rand/v2`. NO new framework primitive. Unit tests inject a deterministic draw to exercise exact `P_reject` boundaries (e.g. draw=0.0 always-admit-edge, draw just-below/just-above `P_reject`). Differential fixtures drive the filter into DETERMINISTIC regimes ONLY: P_reject=0 (healthy backend → always admit, byte-exact cross-side) and a forced-reject leg (threshold manipulation → P_reject≈1 → deterministic 503). **No reliance on matching reference Envoy's RNG draws.**

**Rationale:** Probabilistic rejection cannot be byte-matched cross-side against a foreign RNG. Splitting the test surface into a deterministic-draw subject-only layer (exact boundary math) + a deterministic-regime cross-side layer (P=0 and forced-reject) gives both algorithmic fidelity AND wire-shape parity without coupling them to RNG state. This is the direct analog of phase-21's "FAKE-TIME subject-only + structural cross-side" split, extended for the randomness axis. The `Rand` seam stays inline (consumer count = 1) per the same EXTRACT-NOW-only-when-≥2-consumers discipline that kept phase-21's Clock seam inline.

**Anticipated ADR anchor:** ADR-0194 §Decision includes the inline `Rand` + `Clock` seams; §Consequences includes the deterministic-regime differential strategy + the inline-vs-extracted future trigger.

### 2.3 Clock seam: Inline `Clock` interface *(default, precedent-settled → ADR-0194 same anchor)*

**Decision:** Define a small in-package `Clock` interface (`Now() time.Time`; plus whatever bucket-expiry helper the sliding window needs), `defaultClock` wrapping stdlib. NO new `internal/clock/` framework primitive. Mirrors phase-21 §2.3 verbatim. The `sampling_window` sliding success-rate count expires buckets by wall-clock; the FAKE-TIME implementation lives in the test scope only.

**Rationale:** YAGNI-aligned + identical to phase-21's reasoning. Consumer count for a Clock seam is now 2 (adaptive_concurrency + admission_control), which is the documented EXTRACT-NOW trigger threshold — BUT the two seams live in different packages with different shapes (phase-21 needs `AfterFunc` timer cascades; phase-23 needs only `Now()` + window-bucket expiry), so a premature shared extraction would over-fit. The SPEC author records whether the two Clock shapes have converged enough to justify a future `internal/clock/` extraction (a forward-pointer, not a phase-23 obligation).

### 2.4 Per-route shape: REUSE-by-absence *(default, precedent-settled → NO ADR-0125 amendment)*

**Decision:** The admission_control proto defines NO `AdmissionControlPerRoute` message in v1.37.2, so the filter is listener-scoped only; any `typed_per_filter_config` entry fails proto-deserialization at HCM parse time via the existing framework PARSE-REJECT path. No per-route parsing code; no ADR-0125 roster amendment.

**Rationale:** Identical to the phase-17..21 REUSE-by-absence pattern. Note: phase-22 ENDED the four-consecutive ADR-0125-skip streak (18/19/20/21) by amending the roster 8 → 9 at 22.3. Phase 23 therefore RESTARTS the skip — the **first ADR-0125-skip since phase-22's amendment**. Canonical-per-route roster STAYS 9.

### 2.5 Stat surface: Byte-exact upstream 3-counter parity, NO extra gauges *(default, precedent-settled → ADR-0194)*

**Decision:** Expose ONLY the upstream stat surface — anticipated 3 counters (`rq_rejected`, `rq_success`, `rq_error`) under the `http.<HCM_stat_prefix>.admission_control.<stat>` prefix template, byte-exact against upstream Envoy v1.37.2. **NO extra-upstream observability gauge** (e.g. a current-rejection-probability gauge), because the project mission is conformance/byte-exact parity and upstream admission_control publishes no such gauge. Project stat count 107 → 110.

**Rationale:** Phase-21 added gauges because upstream adaptive_concurrency *publishes* gauges; admission_control upstream publishes only counters, so byte-exact parity means counters only. Adding an extra gauge would be a gratuitous departure inviting a BEHAVIOR_CONTRACT record for no conformance benefit. The exact counter roster + prefix template is a SPEC-time §10 empirical-pin obligation per ADR-0004 (the names above are the BRAINSTORM hypothesis, pinned at SPEC).

### 2.6 Phase split: Single phase *(default, precedent-settled → ADR-0045 split-gate AT REST)*

**Decision:** Land the filter package + both seams + parse + PARSE-REJECT + 3-counter stat surface + deterministic-regime unit tests + the two differential directories + the new fuzzer in a single phase 23. The natural split axes (algorithm vs fixture; decode vs encode) don't carve cleanly, and the framework delta is zero. SPEC author revisits per ADR-0045 §6 if LoC drifts past the gate.

**Rationale:** Phase-23's anticipated LoC (~900-1300) + task count (well under ~25) sits comfortably below the split-gate — LOWER than phase-21, which landed single-phase. No protocol/transport/headers-vs-body axis exists.

### 2.7 Test taxonomy: deterministic-regime subject-only + two-directory differential *(default, derived from Q2)*

**Decision:** Two-layer test strategy: (A) subject-only deterministic-draw + FAKE-TIME algorithmic tests for exact `P_reject` boundaries, window expiry, and success/error classification; (B) a two-directory differential (cross-side on P=0/forced-reject regimes + boot-reject on a PGV-mirror reject). Per `reference_differential_fixture_dispatch_constraint`, the cross-side and boot-reject surfaces are SEPARATE directories from the start (the runner dispatches one branch per directory).

---

## 3. Framework-survey result — ZERO NEW package-level + ZERO IN-PLACE AMENDMENTs + REUSES

Phase 23 introduces **NO new `internal/` packages** and **NO in-place §Decision amendments**. The full surface is constructed from existing primitives + two in-package inline interface seams.

- **NO NEW `internal/clock/` or `internal/rand/`** — both seams inline in `internal/filter/http/admission_control/`. Forward-pointer to a future EXTRACT-NOW trigger if a third timer-driven / probabilistic filter materializes with a convergent seam shape.
- **REUSE 1: `internal/stats/` Counter support** — the 3-counter surface via `Registry.NewCounter`. No framework work.
- **REUSE 2: HTTPRegistry boot-time registration** — `admissioncontrol.New` wired at `cmd/envoy-go/main.go` at its alphabetical position (between `adaptive_concurrency` and `bandwidthlimit`). **18 HTTP filters wired post-phase-23** (17 → 18).
- **REUSE 3: Per-request filter interface (decode/encode hooks)** — the acquire-decision-at-decode + classify-at-encode-complete pattern fits the existing per-request instance framework without extension.
- **REUSE 4: HCM-parse-time PARSE-REJECT path** — adds the admission_control parse arms (runtime_key non-empty ×4 wrappers; oneof-absent; percent out-of-range; non-positive aggression; malformed Int32Range).
- **REUSE 5: REUSE-by-absence per-route enforcement** — no `AdmissionControlPerRoute` message; per-route placement is a proto-deserialization PARSE-REJECT. First ADR-0125-skip since phase-22's amendment.
- **REUSE 6: Existing fuzzer-corpus framework** — `FuzzAdmissionControlConfigParse` as the 32nd fuzzer.
- **REUSE 7: Existing differential-fixture framework** — two new directories per the dispatch-constraint memory.

---

## 4. Per-route shape — REUSE-by-absence (NO new canonical, NO ADR-0125 amendment)

The admission_control proto defines NO `AdmissionControlPerRoute` message in v1.37.2. Per-route configuration is a proto-deserialization-time PARSE-REJECT via the existing HCM filter framework. **Classification: REUSE-by-absence.** First ADR-0125-skip since phase-22's roster amendment (8 → 9 at 22.3). No new canonical entry; no ADR-0125 amendment anchored at phase 23. Roster STAYS 9.

---

## 5. Stat surface hypothesis

### 5.1 3-counter surface roster (BRAINSTORM hypothesis; SPEC-time empirical pin)

| Name | Type | Semantics | Encoding |
|---|---|---|---|
| `rq_rejected` | Counter | Requests rejected by the admission-control dice-roll (503) | int64 monotonic |
| `rq_success` | Counter | Upstream responses classified successful per `success_criteria` | int64 monotonic |
| `rq_error` | Counter | Upstream responses classified unsuccessful per `success_criteria` | int64 monotonic |

### 5.2 Stat-prefix template

Anticipated `http.<HCM_stat_prefix>.admission_control.<stat>`. Exact template byte-exactness vs upstream Envoy v1.37.2 is a SPEC-time §10 empirical-pin obligation per ADR-0004.

### 5.3 Project stat count delta

107 → 110 (+3 counters; no gauges).

### 5.4 envoy-go-strict departure flag

One anticipated BEHAVIOR_CONTRACT record: the RTDS `runtime_key` PARSE-REJECT departure (a new `### envoy.filters.http.admission_control` subsection). The 3-counter surface itself matches upstream byte-exactly (no departure). Any gopher-style numeric-divergence record is N/A here (integer counter arithmetic). Departure-record count 14 → 15 anticipated (the single RTDS-departure record; SPEC may refine).

---

## 6. Differential fixture envelope — two directories

Per project memory `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch, cross-side XOR boot-reject), the cross-side and boot-reject surfaces are SEPARATE directories from the start.

### 6.1 `00NN-http-admission-control` (cross-side)

Deterministic regimes only:
- **Parse OK + all-admit (P_reject=0)** — healthy backend (all responses success per `success_criteria`) drives `P_reject=0`, so every request is admitted; byte-exact cross-side vs reference Envoy v1.37.2 (no RNG dependence when P_reject=0).
- **Forced-reject leg** — threshold manipulation (e.g. all-error backend + `sr_threshold` high + `rps_threshold=0` + a window primed so `P_reject` clamps to `max_rejection_probability`≈deterministic) drives a deterministic 503 path. SPEC-time D-question: whether the forced-reject leg can be made fully RNG-independent (P_reject clamped to 1.0) for byte-exact cross-side, or stays subject-only structural.
- **Subject-only structural** — sampling-window math + stat-surface delta asserted against the encoded probe block.

### 6.2 `00NN+1-http-admission-control-boot-reject` (boot-reject)

A shared PGV-mirror reject where upstream Envoy ALSO rejects at boot (so the boot-reject is byte-comparable, NOT an envoy-go departure): e.g. `sr_threshold.value > 100` or `max_rejection_probability.value > 100` or non-positive `aggression`. Common stderr substring pinned at SPEC. **NOTE:** the RTDS `runtime_key` reject is NOT a boot-reject fixture candidate — upstream Envoy ACCEPTS `runtime_key`, so an envoy-go reject would DIVERGE by design; that departure is unit-tested + BEHAVIOR_CONTRACT-recorded, not differential.

### 6.3 Fixture count

31 → 33 (two directories).

### 6.4 Listener topology

Single listener with a single HCM containing the admission_control filter (alphabetical position) + router terminator. No multi-listener topology anticipated (avoids the `freeTCPPort` combined-run flake surface).

---

## 7. Anticipated ADRs — 2 ADRs (ADR-0194 + ADR-0195)

### 7.1 ADR-0194 — Admission-control algorithm + package shape + inline Clock/Rand seams + deterministic-regime differential strategy

**Decision body summary:** The SRE-book client-side admission-control algorithm (in-package per-HCM-instance sliding-window `{requests, successes}` count over `sampling_window`; the `P_reject = max(0, (n − aggression⁻¹·k)/(n+1))` formula with `max_rejection_probability` clamp + `rps_threshold` suppression + `sr_threshold` gate); the both-sides decode-decision/encode-classify discipline with `SendLocalReply` 503 on reject; the two inline interface seams (`Clock` + `Rand`, NOT framework primitives); the success/error classification per `success_criteria` (http + grpc arms); formula + clamp/gate ordering bit-exact-cited against upstream `admission_control.cc` (SPEC-time D-question on citation depth); the deterministic-regime differential strategy + the two-layer test taxonomy.

**Consequences summary:** No new framework primitive; documented EXTRACT-NOW trigger for a third probabilistic/timer-driven filter with a convergent seam shape; 3-counter byte-exact stat surface (no gauges); SPEC-time D-question slate (D1 stat-roster/prefix empirical pin; D2 forced-reject cross-side promotion; D3 admission_control.cc line-citation depth; D4 local-reply body/header byte-pin).

### 7.2 ADR-0195 — RTDS `runtime_key` deferral PARSE-REJECT

**Decision body summary:** Per the Q1 full-surface-minus-RTDS decision, every `Runtime{FeatureFlag,Double,Percent,UInt32}` wrapper is honored only for its static default value; any non-empty `runtime_key` triggers HCM-parse-time PARSE-REJECT with a forward-pointer to the future Runtime/RTDS family phase. Mirrors phase-21's ADR-0187.

**Consequences summary:** Operators configure static thresholds via the default values; runtime keying is a forward-pointer; the PARSE-REJECT uses the existing HCM-parse-time framework; one BEHAVIOR_CONTRACT departure record (the RTDS reject is an envoy-go departure since upstream accepts `runtime_key`).

### 7.3 Next-free-ADR hypothesis

Following the phase-20/21 D11-style hypothesis (next-free ADR stays UNCONSUMED at phase-done), phase-23 hypothesizes **ADR-0196 stays UNCONSUMED at phase-23 phase-done** (2 ADRs consumed: ADR-0194 + ADR-0195; next-free advances 0194 → 0196). Most-likely escape-valve surfaces (the forced-reject cross-side promotion edge case; the local-reply byte-pin subtleties; the aggression⁻¹ float-rounding edge at the clamp boundary) all close via in-place §Decision wording at ADR-0194. HOLD-with-known-risk, not GUARANTEED-HOLD: a surprise at IMPL (e.g. an upstream stat name that doesn't match the hypothesis, or a 4th counter) may force ADR-0196 consumption.

---

## 8. Deferred items

1. **RTDS `runtime_key` runtime keying** — PARSE-REJECT at config-load per Q1 + ADR-0195. Closes after the Runtime/RTDS family phase.
2. **Probabilistic-path cross-side byte-exact parity** — intrinsically un-matchable against a foreign RNG; only the deterministic P=0/forced-reject regimes are cross-side. No future work expected unless reference-Envoy RNG injection becomes available.
3. **`AdmissionControlPerRoute`** — no such message in v1.37.2; if upstream adds one, that's a future ADR-0125 amendment.
4. **Extra-upstream observability gauges** — deliberately omitted for conformance; a future operator-observability extension could add a current-rejection-probability gauge as an envoy-go-strict departure if ever justified.
5. **Stat-roster + prefix-template byte-exact pin** — SPEC-time §10 empirical-pin obligation per ADR-0004 (not a deferred future-work item; closes at SPEC).
6. **admission_control.cc line-exact lemmata citation depth** — SPEC-time D-question.
7. **Local-reply body/header byte-pin** — the 503 reject local-reply body + header set pinned at SPEC-time per ADR-0004.
8. **Forced-reject cross-side promotion** — SPEC-time D-question on whether the forced-reject leg is RNG-independent enough for byte-exact cross-side or stays subject-only structural.
9. **Shared `internal/clock/` extraction** — Clock-seam consumer count is now 2 (phase-21 + phase-23) but the shapes differ; a future convergent third consumer triggers the extraction decision.

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

Phase-23 picks up ZERO closures from prior phases' deferred-items lists. It lands a NEW filter (admission_control) and does not pick up cross-filter deferred items. The phase-21 Clock-seam EXTRACT-NOW forward-pointer is NOTED (consumer count reaches 2) but NOT consumed — the two Clock-seam shapes differ enough that a shared extraction would over-fit; phase-23 records the convergence question as a forward-pointer (§8 item 9) rather than forcing an extraction. This preserves the EXTRACT-NOW-only-when-the-trigger-genuinely-fires discipline.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D1 (Stat roster + prefix empirical pin)** — verify the 3-counter roster (`rq_rejected`, `rq_success`, `rq_error`) + the `http.<HCM_stat_prefix>.admission_control.<stat>` prefix template IN-SESSION against reference Envoy v1.37.2 per ADR-0004. Default hypothesis: roster + template as above; SPEC author ratifies or amends.
- **D2 (Forced-reject cross-side promotion)** — can the forced-reject differential leg be made fully RNG-independent (P_reject clamped to 1.0 via threshold + window priming) for byte-exact cross-side, or does it stay subject-only structural? Default hypothesis: promotable (clamp to 1.0 removes the dice-roll); SPEC author verifies via §10 empirical pin.
- **D3 (admission_control.cc line-citation depth)** — anchor ADR-0194 with line-exact lemmata citations for the `P_reject` formula + clamp/gate ordering, or stay SPEC-prose-only? Default hypothesis: line-exact for the formula + clamp ordering; prose-only for the per-request token discipline.
- **D4 (Local-reply byte-pin)** — pin the 503 reject local-reply body + header set byte-exactly against reference Envoy v1.37.2 per ADR-0004.

Additional SPEC-time D-questions may surface during the SPEC's phase-23-specific D-question slate; these 4 are the BRAINSTORM-anchored set.

---

## 11. Prior-phase lessons applied

- **Phase-17 lesson — framework primitives extract NOW only when consumer count is ≥2 at extraction time AND the shape is convergent.** Applied to the seams: `Rand` is consumer-count 1 (stays inline); `Clock` reaches consumer-count 2 but the shapes differ (stays inline; convergence recorded as a forward-pointer).
- **Phase-18/19 lesson — pre-emptive phase splits are expensive and only justify themselves with a clear axis.** Applied to §2.6: single phase, no clean split axis, LoC below the gate.
- **Phase-20/21 lesson — full proto surface with PARSE-REJECT for RTDS-coupled subfields is the standard.** Applied to Q1 + ADR-0195.
- **Phase-21 lesson — deterministic subject-only + structural cross-side splits the test surface cleanly when full cross-side parity is intrinsically blocked (timing there; RNG here).** Applied to Q2 + §2.7.
- **Phase-22 lesson — the differential runner dispatches ONE branch per fixture directory (cross-side XOR boot-reject), so plan them as SEPARATE directories from the start** (project memory `reference_differential_fixture_dispatch_constraint`, which bit the 22.3 PLAN). Applied to §6 — two directories from the start.
- **Phase-20/21 lesson — the next-free-ADR-stays-UNCONSUMED hypothesis is a useful planning anchor + phase-done check.** Applied to §7.3 — ADR-0196 hypothesized UNCONSUMED.
- **Phase-22 lesson — conformance/byte-exact parity is the mission; don't add extra-upstream surface.** Applied to §2.5 — 3 counters only, no extra gauge.

---

## 12. Section closeout

This BRAINSTORM.md is **lifecycle-state 0 → 1 complete** for phase 23. The next session (lifecycle-state 1 → 2, skill `superpowers:writing-plans` scoped to SPEC authoring) authors `docs/envoy-go/phases/23-http-filter-admission-control/SPEC.md` based on:

1. The 2 user-decided Q-decisions (§2.1 full-surface-minus-RTDS; §2.2 inline `Rand` seam + deterministic-regime fixtures) + the 5 precedent-settled defaults (§2.3-§2.7).
2. The framework-survey result (§3): ZERO new packages, ZERO in-place amendments, REUSES only.
3. The per-route REUSE-by-absence classification (§4): first ADR-0125-skip since phase-22's amendment; roster STAYS 9.
4. The stat surface hypothesis (§5): 107 → 110; 3 counters, no gauges, prefix template pinned at SPEC.
5. The two-directory differential envelope (§6): `00NN-http-admission-control` cross-side + `00NN+1-http-admission-control-boot-reject` boot-reject; fixtures 31 → 33.
6. The anticipated ADRs (§7): ADR-0194 + ADR-0195; ADR-0196 hypothesized UNCONSUMED at phase-done.
7. The deferred-items register (§8): 9 forward-pointers.
8. The 4 SPEC-time D-questions (§10): D1 stat empirical pin; D2 forced-reject cross-side promotion; D3 line-citation depth; D4 local-reply byte-pin.
9. The prior-phase lessons applied (§11).

The SPEC author is responsible for executing the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004, including the stat-roster/prefix pin (D1) and the local-reply byte-pin (D4).

**Hand-off:** BRAINSTORM-time scope is complete. SPEC author proceeds.
