# Phase 40.1 SPEC — `outlier_detection.consecutive_5xx`: passive ejection of hosts returning consecutive 5xx — the keystone leg of the phase-40 by-detector-class split

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-40 BRAINSTORM (`docs/envoy-go/phases/40-outlier-detection/BRAINSTORM.md`, commit `332bdce`). This SPEC charters phase **40.1** — the `consecutive_5xx` keystone of the pre-authorized 40.1/40.2/40.3 by-detector-class split. Counts at SPEC commit UNCHANGED (stat surface **1132** / fixtures **70** / fuzzers **42** / BackendKind tail **34** / DECISIONS tail **ADR-0244**, next-free **ADR-0245**). The §11 D-OD-* empirical pins were EXECUTED IN-SESSION (2026-06-15) live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network`.

---

## 1. Purpose / Mission

Land **passive** upstream health for the single `consecutive_5xx` detector: a host that returns `consecutive_5xx` consecutive 5xx responses on real traffic is **ejected** from the LB-eligible set for `base_ejection_time`, then automatically returned. This is the counterpart to the phase-39 *active* health checks (probes), built over the SAME `clusterHealth`/`hostHealth` registry (ADR-0242) and the SAME build-time-injected health-aware LB pick (ADR-0243). The one genuinely new primitive is a per-request **upstream-outcome callback** from the router back to the cluster.

40.1 is the keystone: it builds the passive seam + the ejection-state dimension + the LB-filter extension that 40.2 (the other consecutive detectors) and 40.3 (the statistical detectors + the windowed runtime) reuse.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 live pins drove these amendments to the BRAINSTORM design:

- **AMEND-OD1 (un-eject — lazy, not sweep; `interval` parse-only at 40.1).** The reference un-ejects via a periodic `interval` sweep (measured un-eject ≈ `base_ejection_time + ≤1×interval`; D-OD-LIFECYCLE), NOT lazily. envoy-go 40.1 uses a **lazy, observation-driven un-eject** (checked before each `RecordUpstreamResult` and before each LB pick: if `now >= unejectAt`, clear `ejected`) — goroutine-free (the BRAINSTORM design; the first new outlier goroutine is the 40.3 statistical sweep). `interval` is PARSE-ACCEPTED + validated (`gt:0s`) but its sweep-cadence role is a **recorded departure** at 40.1 (under the `0069` continuous-traffic fixture the lazy un-eject is observationally equivalent; the recovery arm is DEFERRED so no cross-side un-eject-timing assertion — §8).
- **AMEND-OD2 (ejection duration — flat `base_ejection_time` at 40.1; backoff deferred).** The reference's ejection duration is `base_ejection_time × num_ejections` with a per-non-ejected-`interval`-sweep DECAY of the multiplier (D-OD-LIFECYCLE — the linear-backoff×decay only manifests on rapid re-ejection). envoy-go 40.1 un-ejects at a **flat `now + base_ejection_time`** (NO backoff) — the linear backoff AND its decay both need the `interval` sweep and are DEFERRED (a recorded departure). The `ejectCount` field is tracked (incremented per ejection) for future backoff + the stat accounting, but does not scale the 40.1 un-eject deadline.
- **AMEND-OD3 (`max_ejection_percent` cap — pinned boundary).** A host is ejected only if `(currentlyEjected + 1) × 100 / totalHosts <= max_ejection_percent`; otherwise the ejection is BLOCKED and `ejections_overflow` increments (the host stays in rotation, `ejections_enforced_*` stay 0, `ejections_detected_consecutive_5xx` still increments). Boundary pinned LIVE: 1-of-3 (33.33%) ejects iff cap ≥ 34; cap 33 → overflow. `0069` uses `max_ejection_percent: 100` so the single failing host ejects.
- **AMEND-OD4 (stat surface — +5, the double-count replicated; the gateway-detected-on-503 NOT replicated).** A single consecutive-5xx ejection bumps FIVE reference counters + the gauge; envoy-go 40.1 emits a focused **+5** set (§7): `ejections_active` (gauge), `ejections_enforced_total`, `ejections_overflow`, `ejections_detected_consecutive_5xx`, `ejections_enforced_consecutive_5xx`, with the double-count (`ejections_enforced_total` AND `ejections_enforced_consecutive_5xx` both ++) REPLICATED. The reference also trips `ejections_detected_consecutive_gateway_failure` on a 503 (503 ∈ {502,503,504}); envoy-go 40.1 has NO gateway detector so it does NOT emit that counter — the `0069` StatsAsserter must NOT cross-assert it (a 40.2 departure). The legacy `ejections_total`/`ejections_consecutive_5xx`/`ejections_success_rate` + the 14 other-variant `detected_`/`enforced_` counters are DEFERRED departures. Surface **1132 → 1137**.
- **AMEND-OD5 (the seam — the live success sites only at 40.1).** The seam is placed in the LIVE drivers `doH1ClusterAction` (`router.go`) + `doH2ClusterAction` (`router_h2.go`) ONLY — NOT the test-only legacy `(*routerAction).do`/`(*routerActionH2).doH2`. At 40.1 the detector fires at the two SUCCESS-completion sites where `picked` is guaranteed non-zero AND the upstream status is known (`router.go:655` `IncStatusClass(resp.StatusCode)`, `router_h2.go:100` `IncStatusClass(resp.Status)`), recording `(picked, statusCode)` on EVERY response (2xx resets `consec5xx`, 5xx increments). The pre-pick dial/acquire-failure paths (H1 503 `router.go:610-613`, H2 502 `router_h2.go:76-79`) have `picked == Endpoint{}` and are the LOCAL-ORIGIN outcomes DEFERRED to 40.2 — at 40.1 they do not call the seam. Weighted-cluster routes reuse the same `do*ClusterAction` → covered for free.
- **AMEND-OD6 (+1 BackendKind — an always-503 in-process responder).** No existing BackendKind returns 5xx unconditionally (`HTTPStatusHeader` is per-request header-controlled, not per-host; D-OD-BACKEND). `0069` needs a +1 BackendKind: an in-process always-503 HTTP/1.1 responder (the `acceptHTTPEchoCounting` precedent — writes `backend-<idx>:` body for attribution). BackendKind tail **34 → 35**. The 2 healthy backends reuse `HTTPEcho`.

### 1.2 ADR continuity + D-disposition at SPEC commit

ADR-0245 (the passive outlier-detection seam + the ejection dimension + the `available` LB-pick extension + the `consecutive_5xx` detector & ejection lifecycle) — §Context DRAFT anchored here (§13); the full §Decision/§Consequences land at the 40.1 IMPL per ADR-0044. DECISIONS tail STAYS ADR-0244 at this SPEC; next-free ADR-0245. The §10 BRAINSTORM D-OD pins are RESOLVED in §11; the PLAN/IMPL D-questions are §12.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §1.1 amendments)

- **40.2:** `consecutive_gateway_failure`, `consecutive_local_origin_failure`, `split_external_local_origin_errors` (threads `UpstreamResult.LocalOriginErr` through the pre-pick failure sites; their `detected_`/`enforced_` stat pairs).
- **40.3:** `success_rate_*` + `failure_percentage_*` statistical detectors + the per-interval cross-host mean/stddev aggregation runtime (the first new outlier goroutine).
- The `interval`-driven un-eject SWEEP (AMEND-OD1), the `base_ejection_time × num_ejections` linear backoff + its decay (AMEND-OD2), `max_ejection_time` + `max_ejection_time_jitter`, `successful_active_health_check_uneject_host`, `always_eject_one_host`, the legacy/other-variant stats (AMEND-OD4), outlier event logging, the `/clusters` per-host ejection readout, outlier detection on non-HTTP upstreams.

---

## 3. The `consecutive_5xx` detector + the passive seam + the ejection dimension (ADR-0245)

### 3.0 Split disposition — 40.1 keystone (the 3-leg split)

40.1 = the seam + the ejection lifecycle + the LB-filter extension + the `consecutive_5xx` detector + `0069`. The ADR-0045 split-gate is re-checked at the PLAN (anticipated ~250–450 prod LoC / ~12–16 tasks — comfortably under `> ~25 tasks OR > ~1500 LoC`; single flat 40.1 leg).

### 3.1 The passive seam (`RecordUpstreamResult`)

A new cluster method:
```go
// UpstreamResult is one completed upstream request's outcome, fed to passive
// outlier detection. LocalOriginErr (a connect/reset failure) is unread at 40.1
// (reserved for 40.2's local-origin detector). (ADR-0245)
type UpstreamResult struct {
	StatusCode     int
	LocalOriginErr bool
}

// RecordUpstreamResult feeds one request outcome to the cluster's outlier
// detector. A no-op for clusters without outlier_detection. (ADR-0245)
func (c *Cluster) RecordUpstreamResult(ep Endpoint, r UpstreamResult)
```
Called from `doH1ClusterAction` (`router.go:655`) + `doH2ClusterAction` (`router_h2.go:100`) immediately beside the existing `IncStatusClass`, with the in-scope `(picked, resp.StatusCode/Status)`. Fires on EVERY response (the forward-compatible chokepoint 40.3 reuses — every response carries the per-host success/failure signal). A no-op when `c.outlier == nil`.

### 3.2 The ejection dimension on `hostHealth` + the `available` predicate (the byte-stable extension)

New fields on the EXISTING `hostHealth` (`internal/cluster/health.go`), DISTINCT from the active-HC `healthy` bit:
```go
ejected        atomic.Bool   // outlier-ejected (separate from healthy)
unejectAtNanos atomic.Int64  // un-eject deadline (now + base_ejection_time)
ejectCount     atomic.Uint32 // ejections so far (for stats + future backoff)
consec5xx      atomic.Uint32 // consecutive 5xx (reset by a 2xx from this host)
```
New `clusterHealth` methods:
- `isEjected(ep) bool` — the **lazy un-eject lives HERE** (AMEND-OD1): for a known addr, if `ejected` is set and `now >= unejectAtNanos`, clear `ejected` + `ejections_active--` and return false; else return `ejected.Load()`; unknown addr → false (not ejected, mirroring `isHealthy`'s unknown→true). Because `isEjected` is invoked on the pick path for every candidate the LB scans (via `available`), an ejected host automatically rejoins on the next LB scan PAST it once its `base_ejection_time` elapses — even with NO further traffic to that host (a host the LB skips never re-enters `RecordUpstreamResult`, so the pick-path check is the load-bearing un-eject path; the `RecordUpstreamResult`-side check is a fast-path for the hot loop).
- `available(ep) bool = isHealthy(ep) && !isEjected(ep)` — the new LB-pick predicate.
- `availableCount(eps) int` — `isHealthy && !isEjected` count (a sibling to `healthyCount`); the `max_ejection_percent` cap accounting reads the current ejected count from this surface (AMEND-OD3).

`isHealthy` and `membership_healthy` KEEP their 39.1 active-HC-only meaning (outlier ejection is surfaced by the separate `outlier_detection.ejections_*` stats, matching the reference; D-OD-STATS). The **six LB constructs move their per-pick filter `isHealthy → available`** — the literal edit surface is **five leaf consult sites** (`loadbalancer.go:61`, `random.go:55`, `ringhash.go:153`, `maglev.go:60`, `leastrequest.go:104`); `subsetLB` inherits the move transitively through its `buildLeafLB` children. **The panic-fraction denominator moves to a NEW `availableCount` helper** (so `inPanic` counts `available` hosts → an all-ejected cluster panics → routes to all, matching the reference's panic-on-too-few-available behavior; this reconciles BRAINSTORM §1.1.4): today `inPanic` (`health.go:89-94`) and the `membershipHealthy` gauge (`health.go:100`) BOTH call `healthyCount`; 40.1 splits them — `inPanic` → `availableCount`, the gauge STAYS `healthyCount` (active-HC-only). When no `outlier_detection` is configured `isEjected` is always false ⇒ `available == isHealthy`, `availableCount == healthyCount`, and the panic denominator is unchanged ⇒ **byte-identical to today** (every existing fixture stays green).

`clusterHealth` is created today only for clusters with `health_checks` (`manager.go:370-373`); 40.1 widens the condition to `health_checks` present **OR** `outlier_detection` present, injecting the health view into the LB constructs in both cases.

### 3.3 The `consecutive_5xx` detector + ejection lifecycle (per-request synchronous)

On each `RecordUpstreamResult(ep, r)` (when `c.outlier != nil`):
1. **Lazy un-eject (fast-path):** call `isEjected(ep)` (which clears an expired ejection — §3.2). NOTE the load-bearing un-eject is the pick-path `isEjected` call (§3.2): an ejected host gets no requests, so its un-eject fires when the LB next scans past it, NOT here — this step only keeps `ep`'s state current for the detector below (AMEND-OD1).
2. **5xx ⇒** `consec5xx++`; if `consec5xx >= consecutive_5xx`:
   - increment `ejections_detected_consecutive_5xx`;
   - if NOT already ejected AND the `enforcing_consecutive_5xx` roll passes (default 100 ⇒ always) AND the `max_ejection_percent` cap permits (`(ejectedCount+1)*100/total <= max_ejection_percent`): **eject** — set `ejected`, `unejectAtNanos = now + base_ejection_time`, `ejectCount++`, `ejections_active++`, `ejections_enforced_total++`, `ejections_enforced_consecutive_5xx++` (the double-count, AMEND-OD4);
   - else (cap blocks): `ejections_overflow++` (no eject) (AMEND-OD3).
3. **non-5xx (2xx/3xx/4xx) ⇒** `consec5xx = 0` (reset; only a response FROM THIS HOST resets it — D-OD-LIFECYCLE).

"5xx" = `StatusCode >= 500 && StatusCode < 600`. `consecutive_5xx == 0` disables the detector (parse: a zero/absent `consecutive_5xx` means the 5xx detector is off — D-OD-PROTO default 5 applies only when `outlier_detection` is set without the field; pin at PLAN, §12).

---

## 4. Framework primitives — the seam + the ejection dimension over the 39.1 substrate + 0 new packages + 0 new go.mod deps

- NEW: `cluster.RecordUpstreamResult` + `UpstreamResult` + the two router call sites (`doH1ClusterAction`/`doH2ClusterAction`); the `outlierDetector` config + logic in a new `internal/cluster/outlier.go`; the ejection fields/methods on `hostHealth`/`clusterHealth`.
- REUSED: the ADR-0242 `clusterHealth`/`hostHealth` registry; the ADR-0243 build-time-injected health-aware pick (the `available` predicate); the `parseHealthChecks`/`parsePanicThreshold` parse precedent; the `registerClusterMetrics` stat-injection precedent (`manager.go:154-161`).
- ZERO new Go packages. ZERO new go.mod modules (`cluster.v3.OutlierDetection` is in the existing go-control-plane v1.32.4 dep; `go mod tidy -diff` EMPTY — D-OD-PROTO).

---

## 5. Proto-field roster (per §11 D-OD-PROTO)

`Cluster.outlier_detection` = `Cluster` field 19 → `cluster.v3.OutlierDetection` (`config/cluster/v3/outlier_detection.pb.go`; `[#next-free-field: 26]`). The 40.1-CONSUMED subset:

| # | Field | Type | Default | 40.1 role |
|---|-------|------|---------|-----------|
| 1 | `consecutive_5xx` | UInt32Value | 5 | the detector threshold (0 ⇒ off) |
| 2 | `interval` | Duration | 10s | PARSE-ACCEPT + validate gt:0s; sweep role DEFERRED (AMEND-OD1) |
| 3 | `base_ejection_time` | Duration | 30s | the flat un-eject duration (AMEND-OD2) |
| 4 | `max_ejection_percent` | UInt32Value | 10 | the ejection cap (AMEND-OD3) |
| 5 | `enforcing_consecutive_5xx` | UInt32Value | 100 | the enforce-roll % (0 ⇒ detect-only) |

All other fields (the gateway/local-origin/statistical detectors, `max_ejection_time`, jitter, `successful_active_health_check_uneject_host`, `always_eject_one_host`) are PARSE-ACCEPTED-and-IGNORED at 40.1 (silent-ignore, additive — consumed by 40.2/40.3 or deferred). Defaults confirmed LIVE (D-OD-PROTO): all matched the anticipated values.

---

## 6. PARSE-REJECT roster (per §11 D-OD-REJECT + ADR-0080)

envoy-go hand-rolls its own byte-stable rejects (the `parseHealthChecks` precedent), mirroring the reference PGV constraints. The 40.1 reject arms (house wording `cluster: %q: outlier_detection: <reason>`):
- `max_ejection_percent > 100` (reference: `OutlierDetectionValidationError.MaxEjectionPercent: value must be less than or equal to 100`).
- `enforcing_consecutive_5xx > 100`.
- `interval` set and `<= 0s` (reference: `value must be greater than 0s`).
- `base_ejection_time` set and `<= 0s`.

The exact envoy-go house wording is pinned at the PLAN/IMPL (§12); all unit-level (no boot-reject dir — the `0066`/`0064` precedent). The reference PGV envelope chain (`Bootstrap → StaticResources → Clusters[0] → Cluster.OutlierDetection → OutlierDetection.<Field>`) is recorded for reference but envoy-go uses its own wording per ADR-0080.

---

## 7. Stat surface — +5 (1132 → 1137) (per §11 D-OD-STATS + AMEND-OD4)

Emitted ONLY on clusters with `outlier_detection` (existing fixtures unaffected). Scoped `cluster.<name>.outlier_detection.`:
1. `ejections_active` — **gauge** (current ejected host count; 0→1 on eject, →0 on un-eject).
2. `ejections_enforced_total` — counter.
3. `ejections_overflow` — counter (cap-blocked ejections).
4. `ejections_detected_consecutive_5xx` — counter.
5. `ejections_enforced_consecutive_5xx` — counter.

The double-count (a single consecutive-5xx ejection ⇒ `ejections_enforced_total++` AND `ejections_enforced_consecutive_5xx++`) is REPLICATED (D-OD-STATS). DEFERRED departures (NOT emitted at 40.1): the legacy `ejections_total`/`ejections_consecutive_5xx`/`ejections_success_rate`; the 14 other-variant `ejections_detected_*`/`ejections_enforced_*` (gateway/local-origin/failure-percentage/success-rate). The reference's 503-trips-`ejections_detected_consecutive_gateway_failure` behavior is NOT replicated at 40.1 (no gateway detector) — the `0069` StatsAsserter excludes it.

---

## 8. Differential fixture taxonomy (+1: `0069` cross-side traffic-driven ejection)

### 8.1 `0069-outlier-detection-consecutive-5xx` (cross-side; +1 BackendKind)

An HTTP listener → a cluster `{2 healthy `HTTPEcho` backends, 1 always-503 backend (the +1 BackendKind)}` with `outlier_detection: { consecutive_5xx: <N>, interval: <t>, base_ejection_time: <t>, max_ejection_percent: 100 }`, on BOTH the subject and the reference (`contrib-v1.37.2`). The driver: drive ≥N+ requests so the 503 host accrues `consecutive_5xx` consecutive 5xx (round-robin makes its picks deterministic), **POLL `/stats` until `cluster.<n>.outlier_detection.ejections_active == 1` on BOTH sides** (no fixed sleep — the `0066` poll-to-converge + warmup-until-stable + delta-counter pattern, `reference_health_check_propagation_warmup`), then assert: the measured load phase routes 100% to the 2 healthy hosts (delta `upstream_rq_2xx`, 0 `upstream_rq_5xx`) + cross-side `outlier_detection.{ejections_active, ejections_enforced_total, ejections_enforced_consecutive_5xx, ejections_detected_consecutive_5xx}` parity (NOT the gateway-detected counter — AMEND-OD4). Verify `upstream_rq_total > 0` reference-side (decode-ran guard). 2 `-count=1` deliberate breaks: (A) the detector no-ops (never ejects) ⇒ `ejections_active` never converges; (B) the LB ignores `available` (picks ejected hosts) ⇒ the warmup never stabilizes (5xx leaks). The constants (N / interval / base_ejection_time / backendCount / convergeDeadline / warmupStable) single-sourced (`reference_fixture_workload_constant_desync`). The time-based **un-eject/recovery arm is DEFERRED** (AMEND-OD1 — the lazy-vs-sweep timing diverges cross-side; not asserted at 40.1).

### 8.2 The +1 BackendKind (always-503 in-process responder)

`HTTP503Responder` BackendKind 35 (name pinned at PLAN) — an in-process `net.Listen` + `go accept…` responder (the `acceptHTTPEchoCounting`/`serveGRPCHealth` precedent, NOT a subprocess): per request, write `HTTP/1.1 503 Service Unavailable` + `Connection: close` + a `backend-<idx>:` body (host attribution via the `backendIdxFromBody` precedent). The 2 healthy backends reuse `HTTPEcho`.

### 8.3 NO new fuzzer

Outlier detection reads the existing HTTP status (no new wire decoder); fuzzers STAY 42.

---

## 9. Behavior-contract delta (the 40.1 bundle; ADR-0052 atomic landing)

A new `### Cluster — passive health (outlier detection)` subsection in BEHAVIOR_CONTRACT.md: the `consecutive_5xx` detector (eject on N consecutive 5xx; the per-host counter reset-on-2xx); the lazy time-based un-eject at flat `base_ejection_time` (the sweep/backoff departures); the `max_ejection_percent` cap + `ejections_overflow`; the `available = isHealthy && !isEjected` LB predicate (membership gauge unchanged); the +5 stats (the double-count) + the deferred-stat departures; the seam (`RecordUpstreamResult` at the live success sites). The stat-surface block advances 1132 → 1137.

---

## 10. Per-task structure (~12–16 tasks; PLAN decomposes)

Anticipated spine: (1) baselines/PROGRESS; (2) the ejection fields on `hostHealth` + `isEjected`/`available` on `clusterHealth` + unit tests; (3) the `available` predicate move across the five leaf constructs + the panic-denominator move + the byte-stability proof (full 70-dir differential GREEN after); (4) `parseOutlierDetection` + the reject roster + the registry-creation widening; (5) the `outlierDetector` + the `consecutive_5xx` detect/eject/lazy-uneject/cap logic + unit tests (incl. the overflow + reset-on-2xx + double-count cases); (6) the `RecordUpstreamResult` seam + the two live router call sites (NOT the legacy drivers); (7) the +5 stat registrations (scoped to outlier clusters); (8) the +1 `HTTP503Responder` BackendKind; (9) the `0069` fixture; (10) `0069` deliberate-break + 20-run flake; (11) full 71-dir differential + six-gate; (12) ADR-0245 body + BEHAVIOR_CONTRACT; (13) completion bundle. The PLAN runs the FINAL ADR-0045 split-gate re-check.

---

## 11. SPEC-time empirical-pin block (D-OD-* — executed IN-SESSION 2026-06-15)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a shared bridge network; a 3-backend cluster [2×200, 1×503]; request path verified `upstream_rq_total>0`) + the go-control-plane v1.32.4 module cache.

| Pin | Disposition |
|-----|-------------|
| **D-OD-PROTO** | CONFIRMED. `outlier_detection` = `Cluster` field 19; the full 25-field roster + defaults (consecutive_5xx 5, interval 10s, base_ejection_time 30s, max_ejection_percent 10, enforcing_consecutive_5xx 100, enforcing_consecutive_gateway_failure 0, enforcing_failure_percentage 0, max_ejection_time 300s) read from the cache; PGV: all `enforcing_*` + `max_ejection_percent` + `failure_percentage_threshold` ≤ 100, interval/base_ejection_time/max_ejection_time gt:0s; `go mod tidy -diff` EMPTY → ZERO new module. |
| **D-OD-STATS** | CONFIRMED. 20 reference `outlier_detection.*` stats; `ejections_active` the sole gauge; the double-count (`ejections_enforced_total` + `ejections_enforced_consecutive_5xx` both ++ on one ejection) verified; 503 also trips `ejections_detected_consecutive_gateway_failure` (detect-only, enforcing=0). 40.1 emits the +5 subset (§7). |
| **D-OD-LIFECYCLE** | PINNED. Eject on the Nth consecutive 5xx; per-host counter resets only on a 2xx from that host; un-eject is interval-SWEEP-driven (~base+1×interval) → envoy-go uses LAZY (AMEND-OD1); flat-vs-backoff (AMEND-OD2); `max_ejection_percent` boundary `(ejected/total)*100 <= cap` (1/3 → cap≥34; cap 33 → overflow) (AMEND-OD3). |
| **D-OD-REJECT** | PINNED. `OutlierDetectionValidationError.{MaxEjectionPercent: value must be less than or equal to 100, BaseEjectionTime: value must be greater than 0s, EnforcingConsecutive_5Xx: value must be less than or equal to 100}`; envoy-go mirrors with house wording (§6). |
| **D-OD-BACKEND** | PINNED. No existing BackendKind is unconditionally 5xx → +1 (`HTTP503Responder`, in-process); 2 healthy reuse `HTTPEcho` (AMEND-OD6). |
| **D-OD-SEAM** | PINNED. Live drivers `doH1ClusterAction`/`doH2ClusterAction` only; 40.1 detector input at the two success sites (`router.go:655`, `router_h2.go:100`) where `picked` is non-zero; the pre-pick failure paths are 40.2 local-origin; weighted routes covered free; the legacy `do`/`doH2` stay seam-free (AMEND-OD5). |

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S40.1-1** the exact envoy-go house reject wording for §6 (the `cluster: %q: outlier_detection: …` strings).
- **D-S40.1-2** the `consecutive_5xx == 0`/absent disposition (off vs the proto default 5 when `outlier_detection` is set) — pin to the reference's "field absent ⇒ default 5; explicit 0 ⇒ off" if confirmed, else the simplest (absent or 0 ⇒ detector off).
- **D-S40.1-3** the `enforcing_consecutive_5xx` roll mechanism (a deterministic 100⇒always / 0⇒never at 40.1, or a PRNG roll for intermediate %); MVP: treat ≥100 as always-enforce, <100 as a `newPCGRNG`-style roll (the `randomLB`/weighted precedent) OR defer intermediate enforcement (record departure). Resolve at PLAN.
- **D-S40.1-4** `0069` constants (N / interval / base_ejection_time / backendCount / N-requests / convergeDeadline / warmupStable / refContainerListenerPort) single-sourced.
- **D-S40.1-5** the `HTTP503Responder` name + whether it shares the `acceptHTTPEchoCounting` body-attribution helper.
- **D-S40.1-6** the ADR-0045 final split-gate re-check (anticipated NO FURTHER SPLIT).

---

## 13. ADR continuity — the ADR-0245 §Context DRAFT (anchored here; full entry lands at the 40.1 IMPL)

**ADR-0245 §Context (draft).** Phase 39 established active health checking — a per-host `healthy` dimension driven by out-of-band probes (ADR-0242) and a build-time-injected health-aware LB pick (ADR-0243). Outlier detection (`Cluster.outlier_detection`) is its passive counterpart: hosts are ejected based on in-band request outcomes. The phase-40 BRAINSTORM pre-authorized a 3-leg by-detector-class split; 40.1 is the `consecutive_5xx` keystone. The design REUSES the ADR-0242 registry + the ADR-0243 pick, and adds the project's first per-request router→cluster health callback. The §11 live pins (D-OD-*) firmed: the seam at the two live-driver success sites; ejection as a SEPARATE per-host sub-state (time-based recovery, distinct from the active-HC `healthy` bit); the `available = isHealthy && !isEjected` predicate (byte-identical when no `outlier_detection`); the +5 stat set with the double-count; and the empirical departures (lazy un-eject vs the reference sweep, flat `base_ejection_time` vs the backoff×decay) recorded for 40.x follow-ups. §Decision + §Consequences land at the 40.1 IMPL.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at this SPEC: stat surface **1132** / fixtures **70** / fuzzers **42** / BackendKind tail **34** / DECISIONS tail **ADR-0244** (next-free **ADR-0245**). ROADMAP row 40 STAYS `in-progress` (the 40.1 SPEC note appended). Anticipated at the 40.1 IMPL: fixtures 70 → 71 (`0069`), BackendKind tail 34 → 35 (`HTTP503Responder`), DECISIONS tail ADR-0244 → ADR-0245 (next-free ADR-0246), stat surface 1132 → 1137 (+5), fuzzers 42 (UNCHANGED), ZERO new packages + ZERO new go.mod modules. Next → the phase-40.1 PLAN (`superpowers:writing-plans`).
