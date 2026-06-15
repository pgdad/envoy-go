# Phase 40 Brainstorm — outlier detection (the SECOND Upstream-robustness-family row; PASSIVE upstream health — the counterpart to phase-39 active health checks)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 40 (`outlier-detection`), the **SECOND Upstream-robustness-family row** (the family opened at phase 39 with active health checks). Phase 40 lands `Cluster.outlier_detection` (`cluster.v3.OutlierDetection`, `Cluster` field 19) — **passive** upstream health: hosts that fail on *real traffic* (consecutive 5xx, consecutive gateway failures, consecutive local-origin failures, or statistically anomalous success-rate/failure-percentage) are *ejected* from the LB-eligible set for a time-based interval, then automatically returned. Where phase-39 active health checking *probes* hosts out-of-band, outlier detection *observes* the in-band request stream — the two are independent, composable health dimensions.

Phase 40 is the natural follow-on to the phase-39 keystone: it REUSES the host health-state registry (`internal/cluster/health.go`'s `clusterHealth`/`hostHealth`, ADR-0242) and the build-time-injected health-aware LB pick (ADR-0243), adding ONE genuinely new seam — a per-request **upstream-outcome callback** from the router back to the cluster (`internal/cluster` has no per-host post-request hook today; the response status is observed only in the router's action path and Inc'd into the `upstream_rq_*` stats, never threaded back to a cluster-level recorder).

The load-bearing facts that shape this brainstorm:

- **`Cluster.outlier_detection` is a single `cluster.v3.OutlierDetection` message (Cluster field 19).** Verified present in the EXISTING core `/envoy` go-control-plane dep (`config/cluster/v3/outlier_detection.pb.go`) — ZERO new go.mod MODULE. Its fields span four detector families plus the ejection lifecycle: the CONSECUTIVE detectors (`consecutive_5xx`, `consecutive_gateway_failure`, `consecutive_local_origin_failure` + `split_external_local_origin_errors`); the STATISTICAL detectors (`success_rate_*` — `success_rate_minimum_hosts`/`success_rate_request_volume`/`success_rate_stdev_factor`; `failure_percentage_*` — `failure_percentage_threshold`/`_minimum_hosts`/`_request_volume`); the ejection LIFECYCLE (`interval`, `base_ejection_time`, `max_ejection_time`, `max_ejection_percent`); and the per-detector `enforcing_*` rollout-percentage knobs. The exact field roster + the PGV/default pins are SPEC-time obligations (§10).
- **Outlier ejection is a SECOND host-state dimension, distinct from active-HC health.** A host can be active-HC-healthy yet outlier-ejected; a host's outlier *recovery is time-based* (`base_ejection_time × num_ejections` linear backoff, capped by `max_ejection_time`), NOT the consecutive-success threshold that drives active-HC recovery. So ejection is its own per-host sub-state, parallel to (not folded into) the ADR-0242 `healthy` bit. This is the central architecture decision (§2.2, Approach A).
- **The passive seam is the ONE new primitive — a per-request upstream-outcome callback.** Outlier detection observes real per-request results: the upstream HTTP status (5xx / gateway 502-503-504) and the connect/reset outcome (local-origin). Today the router knows `(picked endpoint, resp.StatusCode)` at the point it calls `a.cluster.IncStatusClass(code)` (the H1 path) / the H2 equivalent, but NOTHING threads that `(host, outcome)` pair back to a cluster-level recorder. Phase 40 adds `cluster.RecordUpstreamResult(ep, UpstreamResult{...})`, called from the H1 + H2 router paths on EVERY request (not just failures — so the 40.3 statistical detectors, which need per-host success+failure counts over a window, require no further router surgery), with a forward-compatible `UpstreamResult` struct so 40.2/40.3 do not re-touch the call sites (the lesson of the 39.2 `parseHealthChecks` re-signature churn).
- **The LB pick filter is REUSED, not rebuilt.** The six LB constructs already consult the cluster health view at `Pick` and skip non-`isHealthy` hosts (ADR-0243). Phase 40 introduces `available(ep) = isHealthy(ep) && !isEjected(ep)` and the constructs change their predicate `isHealthy → available`. `isHealthy` (and therefore the `membership_healthy` gauge) KEEPS its 39.1 active-HC-only meaning — outlier ejection is tracked by the separate `outlier_detection.ejections_*` stats, matching reference Envoy (the membership gauge is not decremented by ejection). When no `outlier_detection` is configured `isEjected` is always false ⇒ `available == isHealthy` ⇒ **byte-identical to today** (every existing fixture stays green; the 39.1 byte-stability preserved).
- **Determinism is again the novel differential risk — but now TRAFFIC-driven, not probe-driven.** Active HC ejects after `unhealthy_threshold` failed *probes*; outlier detection ejects after `consecutive_5xx` failed *real requests*, and un-ejects after a wall-clock `base_ejection_time`. The `0069` differential must drive enough requests to trigger ejection on BOTH sides, then POLL `/stats` until `ejections_active` converges (the `0066` poll-to-convergence + warmup + delta-counter pattern, `reference_health_check_propagation_warmup`), then assert traffic concentrates on the non-ejected hosts. The time-based recovery (un-eject) arm is a flake risk and a candidate-defer (§6).

The next sessions author the 40.1 SPEC then the PLAN then the IMPL. The SPEC executes the §10 empirical-pin obligations (D-OD1..) IN-SESSION against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) via the live-probe precedent (`reference_docker_probe_bridge_network`), and anchors the ADR-0245 §Context draft.

**Brainstorm session:** worktree `phase-40-brainstorm` off master (per `feedback_git_worktrees`). Substantive predecessor on master: the phase-39.2 IMPL squash `5ddc588` (the multi-codec active-HC `prober` dispatch — TCP + gRPC checkers; ADR-0244), with the docs-only routing tip `f5824e0` (the ROADMAP row-39 → done flip + the next-prompt roll-forward) as the literal live tip. Counts at master tip: stat surface **1132**, differential fixtures **70** (tail `0068-health-check-grpc`), fuzzers **42**, BackendKind tail **34** (`GRPCHealthResponder`), DECISIONS tail **ADR-0244** (next-free **ADR-0245**). ALL counts stay UNCHANGED at this brainstorm.

**Brainstorm mode:** interactive with a live human. The user picked the subject + the scope + the split via a multi-question dialogue:

- **Q-subject** — the next subject is **outlier detection** (chosen over the 3 remaining Upstream-robustness candidates {circuit breakers, retries + hedging, per-protocol connection pooling} and the 3 now-unblocked health-gated Load-balancing candidates {locality-weighted LB, priority LB, panic-threshold refinements}). Rationale: outlier detection composes most directly with the phase-39 host health-state primitive — it is *passive* health to phase-39's *active* health, reusing the registry + the LB filter and pairing the active/passive dimensions.
- **Q-scope** — **full outlier detection** (all detector variants: the consecutive family + the statistical family). Chosen over a `consecutive_5xx`-keystone-only MVP and a consecutive-only (no statistical) scope. The larger envelope drives a pre-authorized split (§1.4).
- **Q-split** — **3 legs (finer keystone)** (chosen over a 2-leg by-detector-class split and a single flat phase). **40.1** = the passive seam + the ejection lifecycle + the single `consecutive_5xx` detector + the LB-filter integration + the first fixture (the minimal keystone, mirroring 39.1's single-HTTP-codec keystone); **40.2** = the other consecutive detectors (`consecutive_gateway_failure` + `consecutive_local_origin_failure` + `split_external_local_origin_errors`, threading the connect/reset outcome through the seam); **40.3** = the statistical detectors (`success_rate` + `failure_percentage`) + the new per-interval cross-host mean/stddev aggregation runtime.

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `ROADMAP.md`, `ENVOY_TARGET.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 .. ADR-0244 — especially ADR-0242 [the host health-state dimension + the active-HC runtime — the registry phase 40 REUSES], ADR-0243 [the health-aware LB pick — the filter phase 40 EXTENDS from `isHealthy` to `available`], ADR-0230 [the redis upstream-pool request-driven runtime seam — the prior per-request callback-from-router-to-cluster precedent], ADR-0235/0239 [the per-request ctx-carry pick-inputs — CONTRASTED: ejection is shared cluster state, not a per-request input], ADR-0106/0045/0044/0052/0080/0227), the as-built `internal/cluster` package (`health.go` [the `hostHealth`/`clusterHealth` registry + `recordResult` + `isHealthy`/`healthyCount`/`inPanic` + the `healthChecker` runtime], `cluster.go` [the `Endpoint` type + the `IncStatusClass` status-class stat + the LB build], `loadbalancer.go`/`ringhash.go`/`maglev.go`/`leastrequest.go`/`random.go` [the six constructs whose pick filter moves `isHealthy → available`]), the router upstream-call path (`internal/filter/http/router/router.go` [the H1 response path + `IncStatusClass`] + `router_h2.go` [the H2 path]), and `internal/admin/admin.go` (the `/stats` endpoint the differential poll-to-converge reads). Empirical pins requiring evidence against the contrib reference Envoy are enumerated in §10 and deferred to SPEC-drafting time per the phase 09–39 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/39-upstream-health-check/BRAINSTORM.md` section-for-section, reframed for the SECOND row of the Upstream-robustness family: a SECOND per-host state dimension (ejection) parallel to active-HC health, a NEW per-request upstream-outcome seam (the first router→cluster post-request callback for health), an EXTENSION of the existing health-aware LB pick (`isHealthy → available`, behavior-neutral when no outlier config), a PRE-AUTHORIZED 40.1/40.2/40.3 by-detector-class split, and a traffic-driven-ejection cross-side differential. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-15.

---

## 1. Mission and scope confirmation (40 only)

ROADMAP row `40 | outlier-detection | 39 | in-progress | | …` (added by this brainstorm) is a **flat top-level Upstream-robustness-family row** (per ADR-0106 — sibling family rows are NOT pre-populated) WITH a **pre-authorized 40.1/40.2/40.3 by-detector-class split** (the phase-36/38/39 valve; §1.4). The row's `depends-on` anchor is phase 39 (the last completed phase; substantive predecessor `5ddc588` — outlier detection REUSES the 39.1 health registry + the 39.1 LB filter).

This is the **second row of the Upstream-robustness family**. The family roster at `ROADMAP.md` (§ Feature Families → Upstream robustness): `{active health checks [DONE — phase 39], outlier detection, circuit breakers, retries + hedging, per-protocol connection pooling}`. Phase 40 lands **outlier detection**. After phase 40 phase-done, **3** family candidates remain: {circuit breakers, retries + hedging, connection pooling} — each its own future brainstorm.

Branch/directory identifiers: directory `40-outlier-detection/`. The 40.1 work lands in the EXISTING `internal/cluster` package (a new `outlier.go` sibling file for the detector + the ejection lifecycle; new ejection fields on `hostHealth` + the `available`/`isEjected` methods on `clusterHealth` in `health.go`; the `Cluster`/`Manager` wiring that creates the health registry for outlier-configured clusters) and touches the router (`internal/filter/http/router/router.go` + `router_h2.go` — the `RecordUpstreamResult` call sites). It is NOT a new Go package (outlier detection is a cluster subsystem, not a filter).

Phase 40 is also: (i) the project's **SECOND per-host state dimension** (host ejection — parallel to the phase-39 active-HC `healthy` bit). (ii) the **FIRST per-request upstream-outcome callback from the router back to the cluster for health** (the response status is observed in the router today but never threaded back; ADR-0230's redis upstream-pool seam is the prior router→cluster request-driven callback precedent, but for pooling not health). (iii) a **behavior-neutral extension of all six LB constructs** (the pick predicate moves `isHealthy → available`, a no-op when no `outlier_detection` is configured — every existing fixture stays byte-identical). (iv) anticipated a ONE-to-TWO-ADR phase per leg (the seam+ejection-dimension ADR primary).

### 1.1 What phase 40.1 delivers as a self-contained whole (envelope: the consecutive_5xx keystone)

Phase 40.1 lands passive outlier detection on the single `consecutive_5xx` detector, with the full keystone substrate, as a self-contained whole:

1. **The host ejection-state dimension** (ADR-0245) — new fields on the EXISTING `hostHealth` (`internal/cluster/health.go`): `ejected atomic.Bool`, `unejectAtNanos atomic.Int64` (the un-eject deadline), `ejectCount atomic.Uint32` (for the linear `base_ejection_time` backoff), `consec5xx atomic.Uint32` (the per-host consecutive-5xx counter). Distinct from the ADR-0242 `healthy` bit. `clusterHealth` gains `isEjected(ep)` + `available(ep) = isHealthy(ep) && !isEjected(ep)` + the currently-ejected-count accounting for the `max_ejection_percent` cap.
2. **The passive upstream-outcome seam** (ADR-0245) — `cluster.RecordUpstreamResult(ep Endpoint, r UpstreamResult)` where `UpstreamResult{StatusCode int; LocalOriginErr bool}` (forward-compatible — `LocalOriginErr` is unread until 40.2). Called from the H1 (`router.go`) + H2 (`router_h2.go`) paths on EVERY completed upstream request, at the point `(picked endpoint, resp.StatusCode)` is known. A no-op for clusters without `outlier_detection`.
3. **The `consecutive_5xx` detector + the ejection lifecycle** (ADR-0245) — per-cluster `outlierDetector` (new `outlier.go`) holding the parsed config; on each result: 5xx ⇒ `consec5xx++`, and if `consec5xx >= consecutive_5xx` AND the `enforcing_consecutive_5xx` roll passes AND the `max_ejection_percent` cap is not hit ⇒ eject (set `ejected`, `unejectAt = now + base_ejection_time × (ejectCount+1)`, `ejectCount++`, Inc stats); non-5xx ⇒ reset `consec5xx`. Un-eject is **lazy + time-based**: before each record/pick, if `now >= unejectAt`, clear `ejected`.
4. **The health-aware LB pick extension** (ADR-0245) — the six constructs' pick predicate moves `isHealthy → available`; an ejected host is skipped exactly like an unhealthy one (skip + re-pick; ring_hash/maglev walk-to-next-healthy preserving key stability), and the panic threshold is computed on the `available` set. Byte-identical when no `outlier_detection` is configured.
5. **The registry-creation extension** — `clusterHealth` is today created only for clusters with `health_checks`; 40.1 extends creation to clusters with `health_checks` **OR** `outlier_detection`, and injects the health view into the LB constructs in both cases.
6. **The differential fixture `0069-outlier-detection-consecutive-5xx`** — a cluster with 2 healthy backends + 1 always-5xx backend, `outlier_detection{consecutive_5xx, interval, base_ejection_time, max_ejection_percent}`, on BOTH sides; the driver drives traffic, polls `/stats` until `ejections_active` converges on both sides, then asserts traffic concentrates on the healthy hosts + cross-side `ejections_*` stat parity — §6.
7. **The BEHAVIOR_CONTRACT 40.1 bundle** + the STATE/ROADMAP advance + the row-40 (40.1 leg) `in-progress → done` flip at the IMPL six-gate (a flat family row — NO parent rollup per ADR-0106; the row flips fully `done` when all three legs land, the row-36/39 precedent).

### 1.2 What phase 40 does NOT deliver (forward to §8)

See §8. Highlights: **the other consecutive detectors** (`consecutive_gateway_failure` + `consecutive_local_origin_failure` + `split_external_local_origin_errors`) → **40.2**; **the statistical detectors** (`success_rate_*` + `failure_percentage_*`) + the per-interval cross-host mean/stddev aggregation runtime → **40.3**; `max_ejection_time` (the exponential-cap on `base_ejection_time` backoff beyond the linear MVP); outlier event logging (`OutlierDetectionEvent` / `event_logger`); jitter; `successful_active_health_check_uneject_host` (the active-HC ↔ outlier interaction on un-eject); per-detector `enforcing_*` rollout beyond `enforcing_consecutive_5xx`; the `/clusters` admin per-host ejection readout enrichment; outlier detection on non-HTTP (TCP/network) upstreams; `max_ejection_time_jitter`.

### 1.3 Phase-done as the SECOND Upstream-robustness-family row landing

Phase 40 lands the family's second row. After phase 40, the family candidate count is 4 → **3** {circuit breakers, retries + hedging, connection pooling}. The family heading gains a one-line `outlier-detection DONE` note. Sibling rows are NOT pre-populated (ADR-0106). The host ejection-state dimension + the `RecordUpstreamResult` seam become durable family assets (the seam is the request-outcome observation point that retries/circuit-breakers may also consume; the `available` LB predicate is now the canonical pick gate).

### 1.4 ADR-0045 split readiness — a PRE-AUTHORIZED 40.1/40.2/40.3 by-detector-class split

Full outlier detection is large: the seam + the ejection lifecycle + 3 consecutive detectors + 2 statistical detectors + the windowed cross-host aggregation runtime. The brainstorm pre-authorizes a 3-leg by-detector-class split (§Q-split), each leg its own SPEC/PLAN/IMPL session, each under the ADR-0045 hard gate (`> ~25 tasks OR > ~1500 LoC`):

- **40.1** (keystone) — the passive seam + the ejection lifecycle + the LB-filter extension + the `consecutive_5xx` detector + `0069`. Anticipated ~250–450 prod LoC / ~12–16 tasks. The load-bearing leg (it builds the seam + the ejection dimension all subsequent legs reuse).
- **40.2** — `consecutive_gateway_failure` + `consecutive_local_origin_failure` + `split_external_local_origin_errors` (threads the connect/reset outcome through the already-built seam — the `LocalOriginErr` field becomes live; the gateway detector is a 502/503/504 subset of the existing status path). Anticipated small (~150–300 LoC) — reuses the 40.1 lifecycle wholesale.
- **40.3** — the statistical detectors (`success_rate` + `failure_percentage`) + a NEW per-interval cross-host aggregation runtime (compute each host's success rate over `interval`, the cross-host mean/stddev, and eject hosts below `mean − stdev_factor × stddev` / above the failure-percentage threshold; gated by `*_minimum_hosts`/`*_request_volume`). The largest leg — it adds a windowed runtime distinct from the per-request synchronous detectors.

The FINAL ADR-0045 split-gate re-check happens at each leg's SPEC/PLAN. The 40.2/40.3 envelopes are firmed at their own brainstorm/SPEC; this brainstorm charters the row + the keystone (40.1) in detail.

### 1.5 Seed-stub alignment + package placement

No seed stub. The 40.1 work is a sibling `outlier.go` in the EXISTING `internal/cluster` package (alongside `health.go`), plus ejection fields/methods on the existing `hostHealth`/`clusterHealth`, plus the two router call sites. ZERO new Go packages.

### 1.6 No prebrainstorm-notes branch

There is no off-master prebrainstorm-notes branch for outlier detection.

### 1.7 Phase 40's relationship to the existing seams (a SECOND health dimension + a NEW request-outcome callback)

Outlier detection composes two existing seams and adds one. It REUSES (a) the ADR-0242 `clusterHealth`/`hostHealth` per-host registry (ejection is new fields on the same record) and (b) the ADR-0243 build-time-injected health-aware LB pick (the predicate widens `isHealthy → available`). It ADDS (c) the per-request `RecordUpstreamResult` callback — the first router→cluster post-request health hook. The callback is request-driven (like ADR-0230's redis upstream-pool seam) NOT timer-driven (unlike the ADR-0242 active-HC runtime); the only timer-ish element is the lazy time-based un-eject, which piggybacks on the per-request/per-pick path (no new background goroutine at 40.1 — the 40.3 statistical runtime is the first new outlier goroutine).

---

## 2. Design decisions

### 2.1 Subject confirmation: outlier detection — the `cluster.v3.OutlierDetection` proto surface *(Q-subject → phase 40 row registered)*

`Cluster.outlier_detection` (`Cluster` field 19) is a single `cluster.v3.OutlierDetection` message in the EXISTING go-control-plane dep (ZERO new module). The 40.1 MVP consumes the consecutive-5xx core: `consecutive_5xx`, `interval`, `base_ejection_time`, `max_ejection_percent`, `enforcing_consecutive_5xx`. The full field roster + PGV/default pins are SPEC obligations (§10, D-OD-PROTO).

### 2.2 Ejection-state architecture: a SEPARATE host-state dimension + a per-request seam *(Q → Approach A → ADR-0245)*

**Approach A (chosen):** ejection is a SECOND per-host sub-state (new fields on `hostHealth`), distinct from the active-HC `healthy` bit; fed by a per-request `cluster.RecordUpstreamResult(ep, UpstreamResult)` callback from the router; the LB pick filters on `available = isHealthy && !isEjected`. Chosen over **Approach B** (feed outlier outcomes into the SAME `recordResult`/`healthy` bit — rejected: outlier recovery is time-based not consecutive-success, so a time-based ejection would wrongly "recover" via active-HC successes; conflates the two sources; cannot stat ejections separately or model active-healthy-but-ejected) and **Approach C** (a stats-poll observer that infers ejections from `/stats` — rejected: there is no per-host upstream-outcome stat to poll; imprecise + not real-time). See §1 load-bearing facts.

### 2.3 Detector scope + split: full outlier detection, 3-leg by-detector-class *(Q-scope + Q-split → §1.4)*

Full scope (all detectors), split 40.1 (consecutive_5xx keystone) / 40.2 (other consecutive) / 40.3 (statistical + windowed runtime). §1.4.

### 2.4 The ejection lifecycle: per-request synchronous eject + lazy time-based un-eject *(self-answered; pinned at SPEC, D-OD-LIFECYCLE)*

Eject is synchronous in the `RecordUpstreamResult` path (atomic counter compare → set `ejected` + deadline). Un-eject is lazy: checked before each record/pick against `now`. No new background goroutine at 40.1 (the 40.3 statistical detectors add the first per-interval sweep). The `base_ejection_time × num_ejections` linear backoff is the MVP; `max_ejection_time` (the cap) is deferred (§8). The `interval` field's exact 40.1 role (sweep cadence vs detector window) is a SPEC pin (D-OD-LIFECYCLE).

### 2.5 Differential strategy: traffic-driven ejection convergence then steady-state routing *(Q-differential → fixture envelope §6)*

Drive real 5xx traffic to trigger ejection, POLL `/stats` until `ejections_active` converges on both sides (the `0066` poll-to-convergence + warmup + delta-counter pattern, `reference_health_check_propagation_warmup`), then assert routing + cross-side stat parity. The time-based un-eject/recovery arm is a flake risk and a candidate-defer (§6).

### 2.6 Deferred-policy posture: an additive config surface (`outlier_detection`); a NEW reject surface *(self-answered; pinned at SPEC, D-OD-REJECT)*

`outlier_detection` is additive — clusters without it are byte-identical. A new `outlier_detection`-validation reject surface (envoy-go's own byte-stable rejects per ADR-0080) is anticipated; its exact arms (e.g. nonsensical thresholds, percent ranges) are a SPEC pin (D-OD-REJECT).

### 2.7 Stat surface: anticipated ~+4–6 `outlier_detection.*` cluster stats *(self-answered; SPEC pins, D-OD-STATS)*

§5.

---

## 3. Framework-survey result — a SECOND host-state dimension + a NEW per-request seam + an EXTENDED LB pick + 0 new packages + 0 new go.mod modules (40.1 anticipated)

### 3.1 Framework: a NEW per-request upstream-outcome callback + a SECOND host-state dimension *(per §1.7)*

The one genuinely new seam is `cluster.RecordUpstreamResult(ep, UpstreamResult)` wired into the H1 + H2 router paths. The ejection dimension extends the existing `hostHealth`/`clusterHealth`. The LB pick predicate extends from `isHealthy` to `available`.

### 3.2 NEW packages: NONE

40.1 is `outlier.go` + edits to `health.go`/`cluster.go`/`manager.go` + the two router files. No new package.

### 3.3 go.mod modules: anticipated ZERO new (40.1) *(verified at brainstorm; re-pinned at SPEC D-OD-PROTO)*

`cluster.v3.OutlierDetection` is in the existing go-control-plane dep; `go mod tidy -diff` anticipated EMPTY.

### 3.4 REUSES

- The ADR-0242 `clusterHealth`/`hostHealth` per-host registry (ejection = new fields).
- The ADR-0243 build-time-injected health-aware LB pick across all six constructs (`isHealthy → available`).
- The router upstream-call path (`router.go`/`router_h2.go`) where `(picked endpoint, resp.StatusCode)` is known (the `RecordUpstreamResult` call sites).
- The `internal/admin` `/stats` endpoint (the differential poll-to-converge reads `ejections_active`).
- The `0066` poll-to-convergence + warmup + delta-counter differential driver pattern (`reference_health_check_propagation_warmup`).

---

## 4. Per-cluster applicability — a NEW cluster-config surface (`Cluster.outlier_detection`)

`Cluster.outlier_detection` is the new per-cluster config surface. A cluster opts in by setting it; absent, behavior is byte-identical. The registry-creation condition widens from "`health_checks` present" to "`health_checks` OR `outlier_detection` present" (a cluster may have outlier detection without active health checks, and vice versa, and both compose — the `available` predicate ANDs them).

---

## 5. Stat surface hypothesis — anticipated ~+4–6 `outlier_detection.*` cluster stats

### 5.1 New stat names (SPEC pins, D-OD-STATS)

Anticipated for 40.1 (Envoy's `detected_`/`enforced_` per-variant pairs + the lifecycle counters), scoped to clusters with `outlier_detection` (existing fixtures unaffected):
- `cluster.<n>.outlier_detection.ejections_enforced_total` (counter)
- `cluster.<n>.outlier_detection.ejections_active` (gauge)
- `cluster.<n>.outlier_detection.ejections_overflow` (counter — `max_ejection_percent` cap hit)
- `cluster.<n>.outlier_detection.ejections_detected_consecutive_5xx` (counter)
- `cluster.<n>.outlier_detection.ejections_enforced_consecutive_5xx` (counter)

Anticipated surface **1132 → ~1137** at 40.1. The exact set is a SPEC pin from a live `/stats` scrape (D-OD-STATS); 40.2/40.3 add the per-variant detected/enforced pairs for their detectors.

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

The deferred-feature outlier stats (event-log counters, `max_ejection_time`-related, the 40.2/40.3 per-variant pairs) are recorded departures until their legs land.

---

## 6. Differential fixture envelope — anticipated ONE directory (40.1)

### 6.1 Fixtures (+1)

`0069-outlier-detection-consecutive-5xx` — an HTTP listener → a cluster `{2 healthy backends, 1 always-5xx backend}` with `outlier_detection{consecutive_5xx: N, interval, base_ejection_time, max_ejection_percent}`, on BOTH the subject and the reference (`contrib-v1.37.2`). The driver drives N+ requests (the always-5xx host must take ≥ `consecutive_5xx` consecutive picks — the round-robin order makes this deterministic), POLLS `/stats` until `ejections_active == 1` on both sides (no fixed sleep), runs a warmup-until-stable gate (closing the reference main→worker propagation window, `reference_health_check_propagation_warmup`), then asserts: the load phase routes 100% to the 2 healthy hosts (delta counters) + cross-side `outlier_detection.ejections_*` parity. Deliberate breaks: (A) the detector no-ops (never ejects) ⇒ the 5xx host stays in rotation ⇒ traffic leaks 5xx ⇒ fail at convergence; (B) the LB ignores `available` (picks ejected hosts) ⇒ same. A time-based **un-eject/recovery arm** (assert the host returns after `base_ejection_time`) is a candidate — time-sensitive; if it flakes it is deferred or runs with a tuned `base_ejection_time` (widen deadlines, never assertions).

### 6.2 Total

Differential fixtures **70 → 71** at the 40.1 IMPL.

### 6.3 New BackendKind: anticipated +1 (an always-5xx responder)

The fixture needs a backend that returns 5xx consistently (so it is ejected). Anticipated **+1 BackendKind** (an always-500 responder — the `GRPCHealthResponder` precedent) UNLESS an existing kind can be configured to 5xx (a SPEC pin, D-OD-BACKEND). BackendKind tail **34 → 35** (anticipated).

### 6.4 New fuzzer: anticipated NONE

No new wire decoder (outlier detection reads the existing HTTP status); fuzzers STAY **42**.

---

## 7. Anticipated ADRs — 1 (possibly 2) ADR for 40.1 (ADR-0245)

- **ADR-0245** (the load-bearing one) — the passive outlier-detection seam: the per-request `RecordUpstreamResult` callback (router → cluster) + the ejection-state dimension on `hostHealth` + the `available()` LB-pick extension + the `consecutive_5xx` detector & ejection lifecycle. §Context at the 40.1 SPEC, §Decision/§Consequences at the 40.1 IMPL per ADR-0044.
- A possible **second ADR** for the ejection policy/lifecycle (the `base_ejection_time` backoff + `max_ejection_percent` cap accounting) if it grows enough to warrant separation — finalized at the SPEC (the phase-39 two-ADR precedent). Next-free is ADR-0245.

---

## 8. Deferred items

- **40.2:** `consecutive_gateway_failure`, `consecutive_local_origin_failure`, `split_external_local_origin_errors` (threads the connect/reset `LocalOriginErr` outcome through the seam).
- **40.3:** `success_rate_*` + `failure_percentage_*` statistical detectors + the per-interval cross-host mean/stddev aggregation runtime + `*_minimum_hosts`/`*_request_volume` gating.
- `max_ejection_time` (the exponential cap on the `base_ejection_time` linear backoff) + `max_ejection_time_jitter`.
- Outlier event logging (`OutlierDetectionEvent` / `event_logger`).
- `successful_active_health_check_uneject_host` (the active-HC ↔ outlier un-eject interaction).
- Per-detector `enforcing_*` rollout beyond `enforcing_consecutive_5xx`.
- The `/clusters` admin per-host ejection readout enrichment.
- Outlier detection on non-HTTP (TCP/network) upstreams.

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

Phase 39's deferred list named "**passive health (outlier detection — `Cluster.outlier_detection`) → its own family row**"; phase 40 picks it up. The phase-39 BEHAVIOR_CONTRACT note that "outlier will feed the SAME health registry as a passive input" is REFINED here: outlier ejection is a SEPARATE dimension on the same `hostHealth` record (not the same `healthy` bit), surfaced via the new `available` predicate (§2.2) — the registry is shared, the sub-state is distinct.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227)

- **D-OD-PROTO** — the exact `cluster.v3.OutlierDetection` field roster + types + PGV constraints + defaults (esp. `consecutive_5xx` default 5, `interval` default 10s, `base_ejection_time` default 30s, `max_ejection_percent` default 10%, `enforcing_consecutive_5xx` default 100); `go mod tidy -diff` EMPTY confirmation.
- **D-OD-LIFECYCLE** — the precise eject/un-eject semantics: when `consecutive_5xx` ejects (the first 5xx after the Nth consecutive, inclusive), the `base_ejection_time × num_ejections` backoff formula, the `max_ejection_percent` cap arithmetic (rounding; whether host 1 of N is always ejectable), the `interval`'s role (does un-eject require the periodic sweep, or is it lazy at pick/record).
- **D-OD-STATS** — the exact `outlier_detection.*` stat names + types from a live `/stats` scrape of an outlier-configured cluster (detected vs enforced; the gauge set).
- **D-OD-REJECT** — the envoy-go-strict reject arms for `outlier_detection` config (byte-stable wording per ADR-0080).
- **D-OD-BACKEND** — whether an existing BackendKind can be configured to return 5xx, or a +1 always-5xx responder is needed for `0069`.
- **D-OD-SEAM** — the exact `RecordUpstreamResult` call-site placement in `router.go` (H1) + `router_h2.go` (H2): the point where `(picked endpoint, resp.StatusCode, connect-error)` is jointly available, on every completion path (success, upstream-5xx, write-error-502, dial-error-503), and that the picked `Endpoint` is in scope there.

These are EXECUTED IN-SESSION at the 40.1 SPEC against `envoyproxy/envoy:contrib-v1.37.2` via the live-probe precedent (`reference_docker_probe_bridge_network`), anchoring the ADR-0245 §Context draft.

---

## 11. Prior-phase lessons applied

- **The 39.2 re-signature churn** — `parseHealthChecks` changed signature twice across 39.2 tasks. Lesson applied: the `UpstreamResult` seam struct is forward-compatible (carries `LocalOriginErr` for 40.2 and fires on every request for 40.3) so 40.2/40.3 do NOT re-touch the router call sites.
- **Byte-stability via an additive predicate** — the `isHealthy → available` change is byte-identical when no `outlier_detection` is configured (the 39.1 `nil clusterHealth` byte-stability discipline). Every existing fixture stays green.
- **Poll-to-convergence, not fixed sleeps** (`reference_health_check_propagation_warmup`) — the `0069` differential gates on `ejections_active` convergence + a warmup-until-stable + delta counters, NEVER a `time.Sleep`. The traffic-driven trigger + the time-based un-eject are the new flake surfaces (un-eject arm is a candidate-defer).
- **`reference_docker_probe_bridge_network`** — the `0069` reference container reaches the backends via `host.docker.internal`; verify `upstream_rq_total > 0` reference-side.
- **The two-ADR-shape valve** (phase 36/38/39) — anticipate 1, possibly 2 ADRs for 40.1; finalize at SPEC.
- **A consumed split phase's row flips `done`** (`reference_roadmap_split_phase_row_done`, the row-36/39 precedent) — row 40 stays `in-progress` until all THREE legs land, then flips `done` (no parent rollup; the family stays open).

---

## 12. Section closeout

Phase 40 (`outlier-detection`) — PASSIVE upstream health, the SECOND Upstream-robustness-family row. A pre-authorized 3-leg by-detector-class split: 40.1 (the passive `RecordUpstreamResult` seam + the ejection-state dimension on `hostHealth` + the `available` LB-pick extension + the `consecutive_5xx` detector + ejection lifecycle + `0069`), 40.2 (the other consecutive detectors), 40.3 (the statistical detectors + the windowed cross-host aggregation runtime). REUSES the ADR-0242 health registry + the ADR-0243 LB filter; ADDS the first router→cluster per-request health callback. Byte-identical when no `outlier_detection` is configured. Anticipated at the 40.1 IMPL: fixtures 70 → 71 (`0069`), BackendKind tail 34 → 35 (an always-5xx responder, TBD at SPEC), DECISIONS tail ADR-0244 → ADR-0245, stat surface 1132 → ~1137, fuzzers 42 (UNCHANGED), ZERO new packages + ZERO new go.mod modules. ROADMAP row 40 registers `in-progress`; it flips `done` per-leg-completion when all three legs land (NO parent rollup per ADR-0106). ALL counts UNCHANGED at this brainstorm commit. The next session authors the 40.1 SPEC (execute D-OD* against `contrib-v1.37.2`; anchor the ADR-0245 §Context).
