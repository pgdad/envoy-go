# Phase 40.3 SPEC — `outlier_detection`: the STATISTICAL detectors (`success_rate` + `failure_percentage`) + the per-interval cross-host aggregation runtime — the THIRD (FINAL) leg of the phase-40 by-detector-class split

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-40.2 IMPL (`docs/envoy-go/phases/40.2-outlier-detection-other-consecutive/`, squash `4aed3e1`; ADR-0246). This SPEC charters phase **40.3** — the STATISTICAL detectors, the FINAL leg of the pre-authorized 40.1/40.2/40.3 by-detector-class split (the BRAINSTORM is DONE for the whole family; the SPEC is authored directly — `docs/envoy-go/phases/40-outlier-detection/BRAINSTORM.md`, §1.4/§8). Counts at SPEC commit UNCHANGED (stat surface **1141** / fixtures **73** / fuzzers **42** / BackendKind tail **35** / DECISIONS tail **ADR-0246**, next-free **ADR-0247**). The §11 D-OD3-* empirical pins were EXECUTED IN-SESSION (2026-06-18) live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network`.

---

## 1. Purpose / Mission

Land the **statistical** passive-outlier detectors over the 40.1/40.2 substrate, introducing the project's FIRST outlier background goroutine (a per-interval cross-host aggregation sweep), with NO router-seam re-design and byte-identical behavior for clusters without `outlier_detection`:

- **`success_rate`** — each `interval`, over the hosts that received ≥ `success_rate_request_volume` requests in the just-completed interval, compute the cross-host **mean** success-rate and **stddev**; eject any host whose success rate is below `mean − (success_rate_stdev_factor / 1000) × stddev`. Gated by ≥ `success_rate_minimum_hosts` eligible hosts. **Active-and-ejecting by default** whenever `outlier_detection` is present (`enforcing_success_rate` default **100**).
- **`failure_percentage`** — each `interval`, over the hosts with ≥ `failure_percentage_request_volume` requests, eject any host whose failure percentage is **≥ `failure_percentage_threshold`** (default 85). Gated by ≥ `failure_percentage_minimum_hosts` hosts. **Active-by-default but detect-only by default** (`enforcing_failure_percentage` default **0** — counts `ejections_detected_failure_percentage` but never ejects until enforcing > 0; the gateway-detector posture of 40.2).

Both detectors are **SWEEP-driven** (evaluated on the periodic `interval` timer), NOT per-request — distinct from the 40.1/40.2 consecutive detectors which fire synchronously inside `record`. 40.3 adds the per-interval aggregation runtime: a per-cluster background goroutine (the first outlier goroutine — the 39.1 `healthChecker` `StartHealthChecks`/`Drain` lifecycle precedent), windowed per-host request/success counters fed by the EXISTING `RecordUpstreamResult` seam (which already fires on EVERY request — the 40.1 forward-compatibility design, so 40.3 re-touches NEITHER the router NOR the seam), and the cross-host mean/stddev arithmetic. The 40.1/40.2 seam, the ejection dimension on `hostHealth`, the `available` LB-pick predicate, the lazy un-eject, the `max_ejection_percent` cap, and `tryEject` are REUSED UNCHANGED.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 live pins drove these amendments to the BRAINSTORM design:

- **AMEND-OD3-1 (the success_rate detector ejects-by-default; the failure_percentage detector is detect-only-by-default).** Live pins confirm `enforcing_success_rate` defaults to **100** (a configured `outlier_detection` cluster with enough hosts EJECTS statistical-success-rate outliers by default) while `enforcing_failure_percentage` defaults to **0** (detect-only — counts `ejections_detected_failure_percentage` without ejecting). This mirrors the 40.2 gateway-vs-5xx asymmetry (`enforcing_consecutive_gateway_failure` default 0). envoy-go 40.3 replicates: the success_rate detector enforces at its default (100); the failure_percentage detector counts-but-does-not-eject at its default (0). Both detectors' DETECTED counters increment whenever the statistical threshold is crossed, regardless of enforcing.

- **AMEND-OD3-2 (the statistical detectors are SWEEP-driven — the first outlier goroutine).** Live pin: a bad host is ejected ~1 `interval` after traffic accrues ≥ `request_volume` requests on it (≈1.5s with `interval: 1s`), confirming the ejection decision runs on the periodic interval SWEEP, not inside the per-request path. envoy-go 40.3 adds a per-cluster background goroutine (`Manager.StartOutlierDetection(ctx)` boot-start, joined in `Drain` — the 39.1 `StartHealthChecks` lifecycle precedent) that, each `interval`: snapshots-and-resets the per-host windowed (total, success) counters, computes the eligible-host set + the cross-host statistics, and ejects. The window is the just-completed interval (counters reset each sweep). The consecutive detectors (40.1/40.2) stay per-request-synchronous — the two paths coexist on the same `outlierDetector`.

- **AMEND-OD3-3 (the per-detector eject double-count is NOT replicated — single-enforcing-detector fixtures).** Live pin (a surprise): when BOTH the success_rate and the failure_percentage detectors flag the SAME host in ONE sweep (e.g. `enforcing_success_rate: 100` + `enforcing_failure_percentage: 100`, a 0%-success host), the reference bumps `ejections_active` and `ejections_enforced_total` by **2** (once per enforcing detector) while `ejections_total` (the legacy per-host counter) bumps by **1**. envoy-go's `tryEject` CAS-once-only ejects a host AT MOST ONCE per sweep (the second detector's CAS returns false → no second `ejections_active`/`ejections_enforced_total` increment) — a host is one active ejection. To avoid the divergence, the 40.3 differential fixtures **isolate a single enforcing statistical detector** (the success_rate fixture uses default enforcing ⇒ success_rate enforces + failure_percentage detect-only; the failure_percentage fixture sets `enforcing_success_rate: 0` ⇒ failure_percentage enforces + success_rate detect-only). The reference's both-enforce-same-host double-increment of `ejections_active`/`ejections_enforced_total` is a **recorded departure** (envoy-go's one-host-one-ejection semantics is the more correct model; the `ejections_total` per-host count would match, but envoy-go does not emit that legacy counter — see AMEND-OD3-4).

- **AMEND-OD3-4 (stat surface — +8, the full statistical name block; surface 1141 → 1149).** Live `/stats` scrape pinned the full **20-name** reference `outlier_detection.*` roster. envoy-go emits 9 today (40.1's 5 + 40.2's 4). 40.3 registers the **+8 statistical** detected/enforced names UNCONDITIONALLY on any outlier cluster (the 40.2 unconditional-registration precedent — the reference exposes every name regardless of which detectors are configured): `ejections_{detected,enforced}_success_rate`, `ejections_{detected,enforced}_failure_percentage`, `ejections_{detected,enforced}_local_origin_success_rate`, `ejections_{detected,enforced}_local_origin_failure_percentage`. The **external** `success_rate` + `failure_percentage` LOGIC is implemented (those 4 counters are driven); the **local-origin** statistical variants' LOGIC is DEFERRED (their 4 counters are registered for surface parity and emit **0** — matching the reference on every split=false fixture; a recorded departure, see §2). The double-count is EXTENDED: a success_rate ejection ⇒ `ejections_enforced_total` AND `ejections_enforced_success_rate` both ++; a failure_percentage ejection ⇒ `ejections_enforced_total` AND `ejections_enforced_failure_percentage` both ++. The 3 legacy aliases (`ejections_total`, `ejections_consecutive_5xx`, `ejections_success_rate`) stay recorded departures (deprecated legacy — envoy-go's per-detector `enforced_*` + `enforced_total` set carries the semantics). Surface **1141 → 1149**.

- **AMEND-OD3-5 (the aggregation arithmetic + the gauge surface).** Live + proto-doc pin: the success-rate ejection threshold is `mean − (success_rate_stdev_factor / 1000) × stddev` over the eligible hosts (so the default factor 1900 ⇒ 1.9 stddevs below the mean); the failure_percentage ejection fires when a host's failure% ≥ `failure_percentage_threshold`; each detector is gated independently by its own `*_minimum_hosts` (eligible-host count) AND `*_request_volume` (per-host volume). `success_rate_stdev_factor: 0` is ACCEPTED (no PGV lower bound) — with factor 0 the threshold collapses to the mean (any below-mean host ejects). The success-rate threshold can go **negative** with few hosts + high variance (e.g. 2 good + 1 bad ⇒ threshold < 0 ⇒ NO host ejects) — so a success_rate fixture needs enough good hosts (or a tuned `stdev_factor`) for a positive threshold (§8). The reference's `success_rate_average` / `success_rate_ejection_threshold` (and local-origin twins) readouts are exposed via the admin `/clusters` endpoint ONLY (NOT named `/stats` gauges) — envoy-go does NOT emit them at 40.3 (the `/clusters` per-host enrichment stays the BRAINSTORM-§8 deferred item; no stat-surface impact).

- **AMEND-OD3-6 (ZERO new BackendKind).** The fixtures reuse the 40.1 `HTTP503Responder` (BackendKind 35) for the failing host + `HTTPEcho` for the healthy hosts (a statistical fixture needs ≥ `minimum_hosts` hosts — the `PerHostBackendKind` per-index runner override, the `0069` mechanism, spawns the mixed topology). BackendKind tail STAYS **35**.

- **AMEND-OD3-7 (retro-impact: NONE on `0069`/`0070`/`0071`).** Live pin (D-OD3-RETRO): a 3-host cluster (3 < `success_rate_minimum_hosts`/`failure_percentage_minimum_hosts` default 5) does NOT run the statistical detectors — the bad host is never statistically ejected — but the full statistical stat-name set IS registered and emits **0**. The existing `0069`/`0070`/`0071` fixtures are 3-host clusters; at 40.3 envoy-go will emit the +8 statistical names at 0 on them, MATCHING the reference's 0. Their `StatsAsserter`s cross-assert specific consecutive-detector names (not the statistical names, and not an exhaustive name-set) → **NO `0069`/`0070`/`0071` change is required**, and their routing/ejection is byte-identical.

### 1.2 ADR continuity + D-disposition at SPEC commit

ADR-0247 (the statistical `success_rate` + `failure_percentage` detectors + the per-interval cross-host aggregation runtime [the first outlier goroutine] + the windowed per-host counters + the +8 statistical stat registrations) — §Context DRAFT anchored here (§13); the full §Decision/§Consequences land at the 40.3 IMPL per ADR-0044. DECISIONS tail STAYS ADR-0246 at this SPEC; next-free ADR-0247. The §10 BRAINSTORM D-OD pins for the statistical detectors are RESOLVED in §11 (D-OD3-*); the PLAN/IMPL D-questions are §12. ADR-0245 (the 40.1 seam + ejection dimension + `available` predicate) + ADR-0246 (the consecutive gateway/local-origin detectors + the live `LocalOriginErr` seam) are REUSED UNCHANGED.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8)

- **The local-origin STATISTICAL variants** — `local_origin_success_rate` + `local_origin_failure_percentage` (gated by `enforcing_local_origin_success_rate` / `enforcing_failure_percentage_local_origin`, split=true only). Their LOGIC requires a SECOND windowed success-tracking axis (per-host local-origin total/success under split=true) and a second aggregation pass. 40.3 registers their 4 stat NAMES for surface parity (emit 0 on all split=false fixtures, matching the reference) but DEFERS the LOGIC as a recorded departure. They are a bounded follow-on (a candidate 40.4 or a recorded family departure); they do NOT gate closing the 3-leg by-detector-class split (40.3 delivers the statistical detector CLASS's core machinery + both EXTERNAL statistical detectors + the windowed runtime).
- The legacy stat aliases `ejections_total` / `ejections_consecutive_5xx` / `ejections_success_rate` (deprecated; envoy-go's per-detector `enforced_*` + `enforced_total` set carries the semantics — recorded departures).
- The `interval`-driven un-eject SWEEP returning hosts to service + the `base_ejection_time × num_ejections` linear backoff + its decay (40.1 AMEND-OD1/OD2 — STAY recorded departures), `max_ejection_time` ENFORCEMENT (the cap on the backoff) + `max_ejection_time_jitter`, `successful_active_health_check_uneject_host`, `always_eject_one_host`, outlier event logging, the `/clusters` per-host ejection + `success_rate_average`/`success_rate_ejection_threshold` readout enrichment, outlier detection on non-HTTP upstreams.
- The recovery/un-eject-timing differential arm (the lazy-vs-sweep timing diverges cross-side — STAYS deferred, the 40.1/40.2 posture). At 40.3 the statistical sweep EJECTS; un-eject stays the 40.1 lazy time-based path (`base_ejection_time`). An ejected host receives no traffic ⇒ 0 requests next interval ⇒ below `request_volume` ⇒ not re-evaluated by the sweep ⇒ stays ejected until the lazy time-based un-eject (consistent, no recovery-arm assertion).
- `max_ejection_time` ENFORCEMENT (40.3 adds only the `max_ejection_time <= 0s` PARSE-REJECT arm for parity — §6 — the field stays parse-accepted-and-ignored at the flat-`base_ejection_time` MVP).

---

## 3. The two statistical detectors + the per-interval aggregation runtime (ADR-0247)

### 3.0 Split disposition — 40.3 (the FINAL leg of the 3-leg split)

40.3 = the external `success_rate` + `failure_percentage` detectors + the per-interval cross-host aggregation runtime (the first outlier goroutine) + the windowed per-host counters + the +8 statistical stat registrations + the statistical-field reject arms + 1–2 differential fixtures. The ADR-0045 split-gate is re-checked at the PLAN (anticipated ~280–360 prod LoC / ~12–14 tasks — comfortably under `> ~25 tasks OR > ~1500 LoC`; single flat 40.3 leg). REUSES the 40.1/40.2 `tryEject` + ejection lifecycle + the seam wholesale. **NO FURTHER SPLIT** anticipated (D-S40.3-7). Landing 40.3 consumes the 3-leg by-detector-class split → ROADMAP row 40 flips `in-progress → done` at the 40.3 IMPL six-gate (`reference_roadmap_split_phase_row_done`; NO parent rollup per ADR-0106).

### 3.1 Parse extension (`parseOutlierDetection`)

`outlierConfig` (`internal/cluster/outlier.go`) gains (defaults per §11 D-OD3-PROTO):
```go
// success_rate detector — sweep-driven; ejects-by-default (enforcing 100).
successRateMinHosts   uint32 // success_rate_minimum_hosts (default 5)
successRateReqVolume  uint32 // success_rate_request_volume (default 100)
successRateStdevFactor uint32 // success_rate_stdev_factor (default 1900; /1000 ⇒ 1.9)
enforcingSuccessRate  uint32 // enforcing_success_rate (default 100)
// failure_percentage detector — sweep-driven; detect-only-by-default (enforcing 0).
failurePctThreshold   uint32 // failure_percentage_threshold (default 85)
failurePctMinHosts    uint32 // failure_percentage_minimum_hosts (default 5)
failurePctReqVolume   uint32 // failure_percentage_request_volume (default 50)
enforcingFailurePct   uint32 // enforcing_failure_percentage (default 0 ⇒ detect-only)
interval              time.Duration // the sweep cadence (default 10s) — NOW LOAD-BEARING (was parse-accepted-only at 40.1/40.2)
```
Default population mirrors the 40.1/40.2 wrapper pattern (absent ⇒ the proto default). `interval` (already parsed + validated `> 0s` at 40.1) becomes the sweep cadence (default 10s when absent). The `enforcing_success_rate`/`enforcing_failure_percentage` rolls reuse the 40.1 `enforceRoll()` mechanism (≥100 ⇒ always; 0 ⇒ never; intermediate ⇒ PCG roll). The local-origin statistical fields (`enforcing_local_origin_success_rate`, `enforcing_failure_percentage_local_origin`) stay parse-accepted-and-ignored (logic deferred — §2).

### 3.2 The windowed per-host counters + the seam (REUSED — no router touch)

The statistical detectors need each host's (total requests, successful requests) over the current interval. The EXISTING `RecordUpstreamResult` seam already fires on EVERY completed request (success sites at 40.1; failure sites at 40.2) — so 40.3 re-touches NEITHER the router NOR the seam. `record(ep, statusCode, localOriginErr)` gains a windowed-count side-effect on the per-host record:

- New `hostHealth` fields: `intervalTotal atomic.Uint64`, `intervalSuccess atomic.Uint64` (the current-interval window; reset at each sweep).
- On each `record`: `intervalTotal++`; if the outcome is a **success** (NOT a 5xx external status AND NOT a `localOriginErr`) `intervalSuccess++`. (The consecutive-counter logic of 40.1/40.2 is UNCHANGED; the windowed counters are an additional, orthogonal accumulation.) The success/failure classification matches the reference's externalized-result classification (a 5xx response or a local-origin failure is a failure; everything else is a success). Under split=false a `localOriginErr` maps to a gateway-class 5xx (a failure either way); the windowed classification is split-agnostic for the EXTERNAL statistical detectors (local-origin-vs-external partitioning is the deferred local-origin-statistical axis, §2).

### 3.3 The per-interval aggregation sweep (the first outlier goroutine)

A per-cluster background goroutine (one per cluster whose `outlier_detection` is present), launched by `Manager.StartOutlierDetection(ctx)` post-Freeze (alongside / mirroring `StartHealthChecks`) and stopped by `Drain` (cancel the derived ctx + join via a WaitGroup — the 39.1 lifecycle, ADR-0242/0243). Each `interval` tick:

1. **Snapshot + reset** every host's `(intervalTotal, intervalSuccess)` window atomically (swap to 0); compute each host's success rate `= intervalSuccess / intervalTotal` (as a fraction) and failure% `= 100 × (intervalTotal − intervalSuccess) / intervalTotal`.
2. **success_rate detector** — collect hosts with `intervalTotal ≥ success_rate_request_volume` (the eligible set). If `len(eligible) ≥ success_rate_minimum_hosts`: compute the mean success-rate + the **POPULATION stddev** (divisor `N`, NOT the sample `N−1` — matching Envoy's implementation; the PLAN's success_rate unit test pins the population divisor — D-S40.3-2) over the eligible set; `threshold = mean − (success_rate_stdev_factor/1000) × stddev`; for each eligible host with success rate `< threshold`: `ejections_detected_success_rate++` and `tryEject(ep, h, enforcing_success_rate, ejections_enforced_success_rate)` (reuses the cap + CAS + the `ejections_active`/`ejections_enforced_total` double-count). If `threshold ≤ 0` no host can be below it (no eject) — a benign no-op (AMEND-OD3-5).
3. **failure_percentage detector** — collect hosts with `intervalTotal ≥ failure_percentage_request_volume`. If `len(eligible) ≥ failure_percentage_minimum_hosts`: for each eligible host with failure% `≥ failure_percentage_threshold`: `ejections_detected_failure_percentage++` and `tryEject(ep, h, enforcing_failure_percentage, ejections_enforced_failure_percentage)`. (No mean/stddev — an absolute per-host threshold.)
4. The sweep reuses the lazy un-eject (a host already past `unejectAtNanos` is refreshed via `isEjected` before the snapshot, the 40.1 path). The sweep does NOT itself un-eject at the MVP (recovery arm deferred, §2). The two detectors are evaluated in a fixed order (success_rate then failure_percentage); `tryEject`'s CAS makes a host ejectable AT MOST ONCE per sweep (AMEND-OD3-3 — the second detector's CAS no-ops, so `ejections_active`/`ejections_enforced_total` count one ejection per host per sweep).

`max_ejection_percent` (the 40.1 cross-multiplied cap), the CAS-once-only eject, the `enforceRoll`, and the lazy un-eject are REUSED for both statistical detectors via the EXISTING `tryEject`.

### 3.4 Concurrency

The sweep goroutine reads the windowed counters concurrently with the per-request `record` writes — both via `atomic.Uint64` (Add on the write side; Swap-to-0 on the sweep side). The ejection-state mutation (`tryEject`) is the EXISTING CAS-guarded path (40.1) — the sweep and the per-request consecutive-detector eject both go through `tryEject`, whose CAS makes ejection exactly-once regardless of which path wins. No new lock; the windowed snapshot is a best-effort per-host atomic swap (a request landing mid-sweep is counted in the next window — acceptable, matches the reference's interval-boundary semantics).

---

## 4. Framework primitives — the two statistical detectors + the per-interval aggregation runtime over the 40.1/40.2 substrate + 0 new packages + 0 new go.mod deps

- NEW: the `success_rate` + `failure_percentage` detector logic + the per-interval aggregation sweep + the windowed per-host counters (`intervalTotal`/`intervalSuccess` on `hostHealth`) + the sweep goroutine lifecycle (`Manager.StartOutlierDetection`/`Drain` join) + the statistical-field parse + the +8 stat registrations + the statistical-field reject arms — all in `internal/cluster` (`outlier.go` + small `health.go`/`manager.go`/`cluster.go` edits). The first outlier background goroutine.
- REUSED UNCHANGED (ADR-0245/0246): the `RecordUpstreamResult` seam + `UpstreamResult` struct (fires on EVERY request — NO router touch); the ejection dimension on `hostHealth`; the `available = isHealthy && !isEjected` predicate + the five leaf consult sites + the `availableCount` panic denominator; the lazy un-eject in `isEjected`; the `max_ejection_percent` cap + `tryEject` + the `ejections_active`/`ejections_enforced_total`/`ejections_overflow` stats; the `enforceRoll` PCG mechanism; the consecutive detectors (`record`/`recordExternal5xx`/`recordLocalOrigin`); the `clusterHealth` registry-creation widening.
- ZERO new Go packages. ZERO new go.mod modules (all statistical fields present in the existing go-control-plane v1.32.4 dep; `go mod tidy -diff` EMPTY — D-OD3-PROTO).

---

## 5. Proto-field roster (per §11 D-OD3-PROTO)

The 40.3-CONSUMED subset of `cluster.v3.OutlierDetection` (in addition to the 40.1 fields 1–5 + the 40.2 fields 10–14):

| # | Field | Type | Default | 40.3 role |
|---|-------|------|---------|-----------|
| 6 | `enforcing_success_rate` | UInt32Value | **100** | success_rate enforce-roll % (ejects by default) |
| 7 | `success_rate_minimum_hosts` | UInt32Value | 5 | min eligible hosts to run the success_rate detector |
| 8 | `success_rate_request_volume` | UInt32Value | 100 | min per-host interval requests to be eligible (success_rate) |
| 9 | `success_rate_stdev_factor` | UInt32Value | 1900 | threshold = mean − (factor/1000)×stddev; NO lower bound (0 accepted) |
| 16 | `failure_percentage_threshold` | UInt32Value | 85 | eject host with failure% ≥ this |
| 17 | `enforcing_failure_percentage` | UInt32Value | **0** | failure_percentage enforce-roll % (detect-only by default) |
| 19 | `failure_percentage_minimum_hosts` | UInt32Value | 5 | min eligible hosts to run the failure_percentage detector |
| 20 | `failure_percentage_request_volume` | UInt32Value | 50 | min per-host interval requests to be eligible (failure_percentage) |
| 2 | `interval` | Duration | 10s | the sweep cadence (was parse-accepted-only at 40.1/40.2; NOW load-bearing) |

PARSE-ACCEPTED-and-IGNORED at 40.3 (logic deferred — §2): `enforcing_local_origin_success_rate` [15], `enforcing_failure_percentage_local_origin` [18], `max_ejection_time` [21] (+ its `<=0s` reject arm at §6), `max_ejection_time_jitter` [22], `successful_active_health_check_uneject_host` [23], `monitors` [24], `always_eject_one_host` [25]. All present in the existing dep; `go mod tidy -diff` EMPTY → ZERO new module (D-OD3-PROTO).

---

## 6. PARSE-REJECT roster (per §11 D-OD3-REJECT + ADR-0080)

NEW reject arms (the house prefix `cluster: %q: outlier_detection: <reason>`), mirroring the reference PGV bounds (all live-confirmed via `--mode validate`):
- `enforcing_success_rate > 100` (reference: `OutlierDetectionValidationError.EnforcingSuccessRate: value must be less than or equal to 100`).
- `enforcing_failure_percentage > 100` (reference: `…EnforcingFailurePercentage: value must be less than or equal to 100`).
- `failure_percentage_threshold > 100` (reference: `…FailurePercentageThreshold: value must be less than or equal to 100`).
- `enforcing_local_origin_success_rate > 100` (reference: `…EnforcingLocalOriginSuccessRate: …`).
- `enforcing_failure_percentage_local_origin > 100` (reference: `…EnforcingFailurePercentageLocalOrigin: …`).
- `max_ejection_time <= 0s` (reference: `…MaxEjectionTime: value must be greater than 0s`; a NEGATIVE duration is a parse-level `Invalid duration: Expected positive duration` — envoy-go's duration parse already rejects negatives).

NO reject for `success_rate_stdev_factor` (the reference ACCEPTS 0 — no PGV lower bound; live-confirmed). The 40.1/40.2 reject arms are UNCHANGED. All unit-level (no boot-reject dir — the `0069` precedent). Exact house wording pinned at the PLAN (§12, D-S40.3-1).

---

## 7. Stat surface — +8 (1141 → 1149) (per §11 D-OD3-STATS + AMEND-OD3-4)

Emitted ONLY on clusters with `outlier_detection`, registered UNCONDITIONALLY (the 40.2 precedent). Scoped `cluster.<name>.outlier_detection.`:
1. `ejections_detected_success_rate` — counter (success_rate threshold crossings).
2. `ejections_enforced_success_rate` — counter (actual success_rate ejections).
3. `ejections_detected_failure_percentage` — counter (failure_percentage threshold crossings; increments even when detect-only at `enforcing` 0).
4. `ejections_enforced_failure_percentage` — counter (actual failure_percentage ejections).
5. `ejections_detected_local_origin_success_rate` — counter (LOGIC DEFERRED — emits 0; name for surface parity).
6. `ejections_enforced_local_origin_success_rate` — counter (LOGIC DEFERRED — emits 0).
7. `ejections_detected_local_origin_failure_percentage` — counter (LOGIC DEFERRED — emits 0).
8. `ejections_enforced_local_origin_failure_percentage` — counter (LOGIC DEFERRED — emits 0).

The double-count is EXTENDED (D-OD3-STATS): a success_rate ejection ⇒ `ejections_enforced_total++` AND `ejections_enforced_success_rate++`; a failure_percentage ejection ⇒ `ejections_enforced_total++` AND `ejections_enforced_failure_percentage++`. The 40.1 `ejections_active`/`ejections_enforced_total`/`ejections_overflow` are cross-detector (REUSED). DEFERRED departures (NOT emitted at 40.3): the legacy `ejections_total`/`ejections_consecutive_5xx`/`ejections_success_rate`; the reference's `/clusters`-only `success_rate_average`/`success_rate_ejection_threshold` readouts (AMEND-OD3-5). With 40.3 envoy-go emits **17 of the reference's 20** `outlier_detection.*` names (the 3 deprecated legacy aliases remain departures). Surface **1141 → 1149**.

---

## 8. Differential fixture taxonomy (+1 or +2; ZERO new BackendKind)

The statistical fixtures are TRAFFIC-driven over a SWEEP, so they extend the 40.1/40.2 poll-to-converge pattern with a per-host volume floor and a multi-host topology.

### 8.1 `0072-outlier-detection-success-rate` (cross-side; ≥`minimum_hosts` hosts; reuses `HTTP503Responder`)

A cluster `{K healthy `HTTPEcho`, 1 `HTTP503Responder`}` (K chosen so the success-rate threshold is positive — see below) with `outlier_detection: { interval: <short>, success_rate_minimum_hosts: <≤ total hosts>, success_rate_request_volume: <low>, success_rate_stdev_factor: 1900, enforcing_success_rate: 100, enforcing_failure_percentage: 0 (default ⇒ failure_percentage detect-only), max_ejection_percent: 100, base_ejection_time: <long>, consecutive detectors disabled (enforcing 0) }`, on BOTH sides. The driver drives round-robin GET / until every host accrues ≥ `success_rate_request_volume` requests over ≥1 interval, POLLS `/stats` until `ejections_active == 1` on BOTH sides (the `0069` poll-to-converge + warmup-until-K-200s + delta-counter pattern), then asserts the measured load phase routes 100% to the K healthy hosts + cross-side parity on `{ejections_active == 1, ejections_enforced_total == 1, ejections_detected_success_rate >= 1, ejections_enforced_success_rate >= 1}` AND `ejections_enforced_failure_percentage == 0` (failure_percentage detect-only at default enforcing 0 — AMEND-OD3-1, a LIVE `== 0` assertion) AND `ejections_enforced_success_rate` is the SOLE enforcer (the single-enforcing-detector posture, AMEND-OD3-3 ⇒ `ejections_active == 1`). Verify `upstream_rq_total > 0` reference-side. **Volume-floor caveat (avoid a vacuous failure_percentage assertion):** the `ejections_detected_failure_percentage >= 1` cross-assertion (the 503 host's failure% 100% ≥ 85) is LIVE only if the bad host accrues ≥ `failure_percentage_request_volume` requests BEFORE the success_rate detector ejects it and stops its traffic; since `failure_percentage_request_volume` defaults to **50** (vs the low `success_rate_request_volume`), the fixture MUST set `failure_percentage_request_volume` ≤ `success_rate_request_volume` (and drive enough volume) so the failure_percentage detector is eligible at the same sweep — OTHERWISE drop the `detected_failure_percentage` cross-assertion from `0072` and assert it only in `0073` (D-S40.3-5 single-sources both detectors' volume floors). 2 `-count=1` deliberate breaks: (A) the sweep no-ops / the success_rate detector never ejects → `ejections_active` never converges; (B) the LB ignores `available` → the ejected 503 host stays in rotation → the measured phase leaks 5xx.

**Threshold positivity (AMEND-OD3-5):** with K healthy at 100% + 1 bad at 0%, mean `= 100K/(K+1)`, and the bad host (0%) must be below `mean − 1.9×stddev > 0`. K=5 (6 hosts total) gives threshold ≈ 12.5% > 0 (live-proven by the probe). The PLAN single-sources K + `stdev_factor`; if a smaller topology is preferred, a lower `stdev_factor` keeps the threshold positive (D-S40.3-5).

### 8.2 `0073-outlier-detection-failure-percentage` (cross-side; ≥`minimum_hosts` hosts; reuses `HTTP503Responder`) — OR folded into `0072` (D-S40.3-6)

A cluster `{K healthy `HTTPEcho`, 1 `HTTP503Responder`}` with `outlier_detection: { …, enforcing_failure_percentage: 100, enforcing_success_rate: 0 (success_rate detect-only ⇒ failure_percentage is the SOLE enforcer), failure_percentage_threshold: 85, failure_percentage_minimum_hosts: <≤ total>, failure_percentage_request_volume: <low> }`, on BOTH sides. Same drive/poll/warmup; asserts cross-side parity on `{ejections_active == 1, ejections_enforced_total == 1, ejections_detected_failure_percentage >= 1, ejections_enforced_failure_percentage >= 1}` AND `ejections_enforced_success_rate == 0` (success_rate detect-only ⇒ a LIVE `== 0` assertion, isolating the failure_percentage enforcer). 2 `-count=1` deliberate breaks: (A) the failure_percentage detector never ejects → no convergence; (B) the failure% threshold comparison is wrong (e.g. `>` instead of `>=`, or a per-host-volume gate bug) → the bad host never ejects. The PLAN decides whether `0073` is a separate dir or `0072` + `0073` fold into one dir with two cluster/listener arms (the `0067`/`0068` two-dir precedent vs a folded dir — D-S40.3-6).

### 8.3 Total + no new fuzzer

Differential fixtures **73 → 74 or 75** (`0072` [+ `0073`]). NO new wire decoder → fuzzers STAY **42**. ZERO new BackendKind (reuse `HTTP503Responder` 35 + the `PerHostBackendKind` runner override for the K+1 topology — AMEND-OD3-6). The existing `0069`/`0070`/`0071` are UNCHANGED (3-host < `minimum_hosts`; the statistical names emit 0 on both sides, not asserted — AMEND-OD3-7).

---

## 9. Behavior-contract delta (the 40.3 bundle; ADR-0052 atomic landing)

Extend the `### Cluster — passive health (outlier detection)` subsection: the `success_rate` detector (sweep-driven; `mean − (stdev_factor/1000)×stddev` threshold; ejects-by-default at `enforcing_success_rate` 100; gated by `success_rate_minimum_hosts`/`success_rate_request_volume`); the `failure_percentage` detector (sweep-driven; per-host `failure% ≥ threshold`; detect-only-by-default at `enforcing_failure_percentage` 0; gated similarly); the per-interval aggregation runtime (the first outlier goroutine, `StartOutlierDetection`/`Drain` lifecycle) + the windowed per-host `(intervalTotal, intervalSuccess)` counters fed by the EXISTING seam (NO router touch); the +8 statistical stats (4 external driven + 4 local-origin name-present-logic-deferred) + the extended double-count; the single-enforcing-detector / one-host-one-ejection semantics (AMEND-OD3-3, the per-detector double-eject departure); the threshold-positivity note. The stat-surface block advances 1141 → 1149. Record the deferred local-origin-statistical departure + the 3 legacy-alias departures + the `success_rate_stdev_factor: 0`-accepted finding.

---

## 10. Per-task structure (~12–14 tasks; PLAN decomposes)

Anticipated spine: (1) baselines/PROGRESS; (2) the `parseOutlierDetection` extension (the 8 statistical fields + `interval`-as-load-bearing + defaults + the statistical reject arms) + unit tests; (3) the windowed `(intervalTotal, intervalSuccess)` counters on `hostHealth` + the success/failure classification side-effect in `record` + unit tests; (4) the success_rate aggregation (mean/stddev/threshold/eligibility) + `tryEject` wiring + unit tests (incl. the negative-threshold no-op + the minimum_hosts/request_volume gates); (5) the failure_percentage aggregation + unit tests (the `>=` boundary); (6) the per-interval sweep goroutine + `Manager.StartOutlierDetection`/`Drain` lifecycle (the 39.1 precedent; no goroutine leak under -race) + unit tests; (7) the +8 stat registrations; (8) the `0072` success-rate fixture (K+1 topology via `PerHostBackendKind`); (9) the `0073` failure-percentage fixture (or fold); (10) deliberate-breaks + 20-run flake (`-count=1`, `TestDifferential/0072`/`0073`); (11) full 74/75-dir differential + six-gate; (12) ADR-0247 body + BEHAVIOR_CONTRACT; (13) completion bundle + ROADMAP row 40 → `done`. The PLAN runs the FINAL ADR-0045 split-gate re-check (anticipated NO FURTHER SPLIT — D-S40.3-7).

---

## 11. SPEC-time empirical-pin block (D-OD3-* — executed IN-SESSION 2026-06-18)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a shared `odnet` bridge; STRICT_DNS clusters reached by container hostname; published admin `:19600`; request path verified `upstream_rq_total > 0`) + the vendored go-control-plane v1.32.4 proto.

| Pin | Disposition |
|-----|-------------|
| **D-OD3-PROTO** | CONFIRMED. Fields 6–9 + 16–20 present in the existing dep with the documented defaults (`enforcing_success_rate` **100**, `success_rate_minimum_hosts` 5, `success_rate_request_volume` 100, `success_rate_stdev_factor` 1900 [/1000], `failure_percentage_threshold` 85, `enforcing_failure_percentage` **0**, `failure_percentage_minimum_hosts` 5, `failure_percentage_request_volume` 50). All getters present; `go build ./...` green; `go mod tidy -diff` EMPTY → ZERO new module. |
| **D-OD3-STATS** | PINNED. Full 20-name reference roster scraped verbatim. 40.3's +8: `ejections_{detected,enforced}_success_rate` + `ejections_{detected,enforced}_failure_percentage` + `ejections_{detected,enforced}_local_origin_success_rate` + `ejections_{detected,enforced}_local_origin_failure_percentage`. Double-count verified LIVE (success_rate eject ⇒ `enforced_total` + `enforced_success_rate` both ++; failure_percentage eject ⇒ `enforced_total` + `enforced_failure_percentage` both ++). The `success_rate_average`/`success_rate_ejection_threshold` readouts are `/clusters`-ONLY (NOT named `/stats` gauges) → NOT emitted at 40.3. Surface +8 → 1149. |
| **D-OD3-LIFECYCLE** | CONFIRMED. SWEEP-driven (bad host ejected ~1.5s ≈ one `interval` after volume accrues, NOT per-request). Threshold = `mean − (stdev_factor/1000)×stddev` (success_rate); `failure% ≥ threshold` (failure_percentage); each gated independently by its `*_minimum_hosts` (eligible-host count) + `*_request_volume` (per-host volume). `enforcing_success_rate` 100 ⇒ ejects; `enforcing_failure_percentage` 0 ⇒ detect-only (AMEND-OD3-1). Per-detector double-eject of the SAME host bumps `ejections_active`/`enforced_total` per-detector (the reference quirk — AMEND-OD3-3); envoy-go's CAS-once-only ejects a host once → fixtures isolate a single enforcing detector. |
| **D-OD3-REJECT** | PINNED. `EnforcingSuccessRate`/`EnforcingFailurePercentage`/`FailurePercentageThreshold`/`EnforcingLocalOriginSuccessRate`/`EnforcingFailurePercentageLocalOrigin: value must be less than or equal to 100`; `MaxEjectionTime: value must be greater than 0s` (negative ⇒ parse-level `Expected positive duration`). **`success_rate_stdev_factor: 0` is ACCEPTED** (no PGV lower bound — NO reject arm). envoy-go mirrors with house wording (§6). |
| **D-OD3-RETRO** | CONFIRMED. A 3-host cluster (3 < `minimum_hosts` 5): the statistical detectors do NOT run (bad host never ejected, served its full round-robin share) but the full statistical stat-name set is registered and emits **0**. → the 3-host `0069`/`0070`/`0071` fixtures see the +8 names at 0 on BOTH sides (cross-matching); their `StatsAsserter`s assert consecutive names only (not the statistical names, not an exhaustive set) ⇒ NO change required (AMEND-OD3-7). |
| **D-OD3-BACKEND** | PINNED. ZERO new BackendKind — the fixtures reuse `HTTP503Responder` (35) for the bad host + `HTTPEcho` for the K healthy hosts via the `PerHostBackendKind` per-index runner override (the `0069` mechanism). Tail STAYS 35 (AMEND-OD3-6). |

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S40.3-1** the exact envoy-go house reject wording for §6 (the five `enforcing_*`/`failure_percentage_threshold > 100` strings + the `max_ejection_time <= 0s` string).
- **D-S40.3-2** the windowed-counter placement + snapshot mechanism: `intervalTotal`/`intervalSuccess` as `atomic.Uint64` on `hostHealth` with a Swap-to-0 snapshot at the sweep (the §3.4 concurrency model) vs a per-detector ring/double-buffer; pick the minimal atomic form that is race-clean under `-race`.
- **D-S40.3-3** the success/failure classification in `record` for the window — confirm a `localOriginErr` is a failure, a 5xx is a failure, everything else is a success; whether to count it BEFORE or AFTER the consecutive-detector branch (must count on EVERY record path incl. the local-origin + non-5xx-reset paths).
- **D-S40.3-4** the sweep goroutine lifecycle: `Manager.StartOutlierDetection(ctx)` as a sibling of `StartHealthChecks` (one goroutine per outlier cluster) vs folding into a shared outlier-runtime; the `Drain` join (extend `hcWG`/`hcCancel` or a parallel `odWG`/`odCancel`); the boot call site in `cmd/envoy-go/main.go` (post-Freeze); no goroutine leak under `-race`.
- **D-S40.3-5** the `0072`/`0073` constants single-sourced (K healthy hosts / `stdev_factor` for a positive threshold / BOTH detectors' `*_minimum_hosts` + `*_request_volume` [the volume floors must be cleared before convergence so the `>= 1` detected assertions are LIVE — the §8.1 caveat] / `interval` short / `base_ejection_time` long / N-requests / convergeDeadline / warmupStable / `refContainerListenerPort` = **19161** for `0072` [next-free after `0071`'s 19160; 0069=19158, 0070=19159], **19162** for `0073` if a separate dir; `reference_fixture_workload_constant_desync`).
- **D-S40.3-6** whether `0072`+`0073` are two dirs or one dir with two cluster/listener arms (the `0067`/`0068` two-dir precedent vs a folded dir; both cross-side ⇒ the dispatch constraint permits either).
- **D-S40.3-7** the ADR-0045 FINAL split-gate re-check (anticipated NO FURTHER SPLIT — single flat 40.3 leg consuming the 3-leg split → row 40 flips `done`).
- **D-S40.3-8** whether to register the 4 local-origin statistical NAMES (logic-deferred, emit 0 — the §7/AMEND-OD3-4 surface-parity choice) vs emit only the 4 external names (the local-origin names then a NAME departure). SPEC position: register all 8 (the 40.2 unconditional-registration precedent + FINAL-leg surface closure); confirm at PLAN the always-0 local-origin names do not break any fixture asserter.

---

## 13. ADR continuity — the ADR-0247 §Context DRAFT (anchored here; full entry lands at the 40.3 IMPL)

**ADR-0247 §Context (draft).** Phases 40.1 (ADR-0245) + 40.2 (ADR-0246) established passive outlier detection over the phase-39 host-health registry: the per-request `RecordUpstreamResult` seam (firing on EVERY completed request — success + connection-failure sites), a per-host ejection sub-state on `hostHealth`, the `available = isHealthy && !isEjected` LB-pick predicate, the `tryEject` shared eject helper (cap + CAS + the cross-detector double-count), and the per-request-synchronous CONSECUTIVE detectors (`consecutive_5xx`, `consecutive_gateway_failure`, `consecutive_local_origin_failure` + the `split` accounting). Phase 40.3 is the THIRD (FINAL) leg of the pre-authorized 3-leg by-detector-class split: the STATISTICAL detectors. The §11 live pins (D-OD3-PROTO/STATS/LIFECYCLE/REJECT/RETRO/BACKEND, executed in-session against `contrib-v1.37.2`) firmed: the statistical detectors are SWEEP-driven (the project's FIRST outlier background goroutine — a per-`interval` cross-host aggregation), distinct from the per-request consecutive detectors; `success_rate` ejects hosts below `mean − (success_rate_stdev_factor/1000)×stddev` over the eligible hosts (≥ `success_rate_request_volume` per-host, ≥ `success_rate_minimum_hosts` eligible) and is ejecting-by-default (`enforcing_success_rate` 100); `failure_percentage` ejects hosts with `failure% ≥ failure_percentage_threshold` (gated similarly) and is detect-only-by-default (`enforcing_failure_percentage` 0); the windowed per-host `(intervalTotal, intervalSuccess)` counters are fed by the EXISTING seam (NO router touch — the 40.1 forward-compatibility design pays off); the +8 statistical detected/enforced stat names extend the double-count; the reference's per-detector double-eject of one host (`ejections_active`/`enforced_total` +2) is NOT replicated (envoy-go's CAS-once-only is one-host-one-ejection; fixtures isolate a single enforcing detector). The EXTERNAL `success_rate` + `failure_percentage` logic is implemented; the LOCAL-ORIGIN statistical variants' logic is deferred (their 4 stat names registered for surface parity, emit 0); the 3 legacy stat aliases stay departures. The 40.1/40.2 seam + ejection dimension + `available` predicate + cap + lazy un-eject + `tryEject` + the consecutive detectors are REUSED UNCHANGED. Landing 40.3 consumes the 3-leg by-detector-class split → ROADMAP row 40 flips `done`. §Decision + §Consequences land at the 40.3 IMPL.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at this SPEC: stat surface **1141** / fixtures **73** / fuzzers **42** / BackendKind tail **35** / DECISIONS tail **ADR-0246** (next-free **ADR-0247**). ROADMAP row 40 STAYS `in-progress` (the 40.3 SPEC note appended; the row flips `done` at the 40.3 IMPL six-gate — the final leg consuming the 3-leg split, `reference_roadmap_split_phase_row_done`). Anticipated at the 40.3 IMPL: fixtures 73 → 74 or 75 (`0072` [+ `0073`]), BackendKind tail 35 (UNCHANGED), DECISIONS tail ADR-0246 → ADR-0247 (next-free ADR-0248), stat surface 1141 → 1149 (+8), fuzzers 42 (UNCHANGED), ZERO new packages + ZERO new go.mod modules. Next → the phase-40.3 PLAN (`superpowers:writing-plans`).
