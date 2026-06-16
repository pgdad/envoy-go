# Phase 40.2 SPEC — `outlier_detection`: the other consecutive detectors (`consecutive_gateway_failure` + `consecutive_local_origin_failure` + `split_external_local_origin_errors`) — the SECOND leg of the phase-40 by-detector-class split

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-40.1 IMPL (`docs/envoy-go/phases/40.1-outlier-detection-consecutive-5xx/`, squash `bb4da7b`; ADR-0245). This SPEC charters phase **40.2** — the OTHER consecutive detectors of the pre-authorized 40.1/40.2/40.3 by-detector-class split (the BRAINSTORM is DONE for the whole family; the SPEC is authored directly — `docs/envoy-go/phases/40-outlier-detection/BRAINSTORM.md`, §1.4/§8). Counts at SPEC commit UNCHANGED (stat surface **1137** / fixtures **71** / fuzzers **42** / BackendKind tail **35** / DECISIONS tail **ADR-0245**, next-free **ADR-0246**). The §11 D-OD2-* empirical pins were EXECUTED IN-SESSION (2026-06-16) live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network`.

---

## 1. Purpose / Mission

Land the remaining **consecutive** passive-outlier detectors over the 40.1 substrate, with NO router-seam re-design:

- **`consecutive_gateway_failure`** — eject a host after N consecutive **gateway-class** 5xx (`{502, 503, 504}`, a subset of the 5xx range). Active-by-default whenever `outlier_detection` is present (the reference default `consecutive_gateway_failure: 5`), but **detect-only by default** (`enforcing_consecutive_gateway_failure` default **0** ⇒ counts `ejections_detected_consecutive_gateway_failure` but never ejects until enforcing > 0).
- **`consecutive_local_origin_failure`** — eject a host after N consecutive **local-origin** failures (a connect-refused / connect-timeout / connection-reset failure originating in the proxy, NOT an HTTP status from the upstream). Takes effect **only when `split_external_local_origin_errors: true`** (the reference default `consecutive_local_origin_failure: 5`, `enforcing_consecutive_local_origin_failure: 100`).
- **`split_external_local_origin_errors`** (bool, default false) — the accounting switch. When **false** (default): a local-origin failure is mapped to a gateway-class 5xx and counted by the `consecutive_5xx` + `consecutive_gateway_failure` detectors (the local-origin detector is inactive). When **true**: local-origin failures feed ONLY the `consecutive_local_origin_failure` detector; the `consecutive_5xx`/`consecutive_gateway_failure` detectors count ONLY externally-originated HTTP 5xx.

40.2 makes `UpstreamResult.LocalOriginErr` (a 40.1-reserved field, unread at 40.1) **live**: the router populates it at the connection-failure sites, and the detector branches on it. The one genuinely-new framework touch is surfacing the **picked endpoint on a connect failure** (today the shared `Cluster.Dial`/`AcquireH1` connect-failure path discards it, returning `Endpoint{}`) so a local-origin failure can be attributed to the host the LB selected. The 40.1 seam (`RecordUpstreamResult` at the two live-driver SUCCESS sites), the ejection dimension on `hostHealth`, the `available` LB-pick predicate, the lazy un-eject, and the `max_ejection_percent` cap are REUSED UNCHANGED.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 live pins drove these amendments to the BRAINSTORM design:

- **AMEND-OD2-1 (the gateway detector is active-by-default + detect-only-by-default).** The reference's `consecutive_gateway_failure` defaults to 5 and `enforcing_consecutive_gateway_failure` to **0** — so on ANY outlier-configured cluster a gateway-class 5xx (`{502,503,504}`) increments `ejections_detected_consecutive_gateway_failure` (detect-only) without ejecting (live pin: a 503 trips `detected_consecutive_gateway_failure` regardless of whether the gateway detector enforces). envoy-go 40.2 replicates this: the gateway detector is active whenever `outlier_detection` is present (its detected counter increments on gateway 5xx); it ejects only when `enforcing_consecutive_gateway_failure > 0`. **Consequence for the EXISTING `0069` fixture:** at 40.1 envoy-go emitted NO gateway-detected counter (no gateway detector — AMEND-OD4) and the `0069` `StatsAsserter` EXCLUDED it; at 40.2 envoy-go WILL emit `ejections_detected_consecutive_gateway_failure` on `0069`'s 503s (matching the reference, which already did) — so a 40.2 task WIDENS the `0069` `StatsAsserter` to cross-assert the gateway-detected counter (no longer a departure). `0069`'s routing + ejection (via the 5xx detector — gateway stays detect-only at `enforcing_consecutive_gateway_failure` default 0) is UNCHANGED.

- **AMEND-OD2-2 (gateway-checked-first + the once-ejected counter reset).** Live pin: with BOTH `consecutive_5xx: 3` and `consecutive_gateway_failure: 3` + `enforcing_consecutive_gateway_failure: 100` configured against a 503 host, the host ejects via the **gateway** detector and `ejections_detected_consecutive_5xx` stays **0** (the gateway detector is evaluated FIRST per `putHttpResponseCode`, and the ejection clears the host's consecutive counters + removes it from rotation before the 5xx counter reaches its threshold). envoy-go 40.2 replicates the OBSERVABLE behavior: a gateway-class external 5xx increments BOTH the gateway and the 5xx consecutive counters; the gateway detector is evaluated FIRST; an ejection by EITHER detector clears both consecutive counters for that host (and the host leaves rotation). A non-gateway 5xx (`{500,501,505..599}`) increments only the 5xx counter and resets the gateway counter. The exact reset ordering is an IMPL detail (§12); the SPEC pins the cross-side stat-parity targets.

- **AMEND-OD2-3 (the `split_external_local_origin_errors` accounting — the load-bearing pin).** Live pins: (split=FALSE, the default) a connect-refused failure to a dead host → `ejections_detected_consecutive_5xx` + `ejections_enforced_consecutive_5xx` (the local-origin failure is mapped to a 5xx; the local-origin detector is INACTIVE → `detected_consecutive_local_origin_failure: 0`). (split=TRUE) the same connect failure → `ejections_detected_consecutive_local_origin_failure` + `ejections_enforced_consecutive_local_origin_failure` ONLY (the 5xx/gateway detectors are untouched → `detected_consecutive_5xx: 0`, `detected_consecutive_gateway_failure: 0`). envoy-go 40.2 replicates: when `split=false` a `LocalOriginErr` result is treated as a gateway-class 5xx (feeds the gateway + 5xx detectors); when `split=true` a `LocalOriginErr` result feeds ONLY the local-origin detector, and external HTTP 5xx feed ONLY the gateway/5xx detectors. Externally-originated 5xx ALWAYS feed the gateway/5xx detectors regardless of `split`.

- **AMEND-OD2-4 (the local-origin attribution seam — surface the picked endpoint on connect failure).** The 40.1 seam fires only at the two SUCCESS sites where `picked` is non-zero. 40.2 ALSO fires at the connection-failure sites with `LocalOriginErr: true` — H1 `AcquireH1` dial/TLS failure (`router.go:610-614`, the 503 path), H1 `req.Write` failure (`router.go:637-640`, 502), H1 `http.ReadResponse` failure (`router.go:648-652`, 502); H2 `DialH2` failure (`router_h2.go:76-80`, 502), H2 `RoundTrip` non-ctx-cancel failure (`router_h2.go:96-98`, 502). Two of these (the H1 acquire 503 + the H2 dial 502) have `picked == Endpoint{}` today because the shared `Cluster.Dial`/`AcquireH1` connect-failure path returns `Endpoint{}` (it knows the dialed `ep` at the `net.Dialer.DialContext` failure but discards it). 40.2's one framework touch: surface the picked endpoint on the connect-failure path so the local-origin failure attributes to the host the LB selected. When the **LB pick itself fails** (no host available — panic/empty set), there is NO host to attribute → NO `RecordUpstreamResult` call (matching the reference: a no-host-selected failure is not a per-host outlier event). The `req.Write`/`ReadResponse`/`RoundTrip` failure sites already have a non-zero `picked`.

- **AMEND-OD2-5 (stat surface — +4, the double-count extended; surface 1137 → 1141).** A gateway ejection bumps `ejections_enforced_total` + `ejections_enforced_consecutive_gateway_failure` (the double-count, live-verified); a local-origin ejection bumps `ejections_enforced_total` + `ejections_enforced_consecutive_local_origin_failure` (live-verified). 40.2 adds the **+4** detected/enforced pairs (§7): `ejections_detected_consecutive_gateway_failure`, `ejections_enforced_consecutive_gateway_failure`, `ejections_detected_consecutive_local_origin_failure`, `ejections_enforced_consecutive_local_origin_failure`. The 40.1 `ejections_active` gauge + `ejections_enforced_total` + `ejections_overflow` are REUSED (cross-detector). The legacy `ejections_total`/`ejections_consecutive_5xx`/`ejections_success_rate` + the 8 statistical-detector `detected_`/`enforced_` counters (success_rate / failure_percentage / local_origin_success_rate / local_origin_failure_percentage) stay DEFERRED (40.3). Surface **1137 → 1141**.

- **AMEND-OD2-6 (ZERO new BackendKind).** No new responder is needed: the gateway fixture reuses the 40.1 `HTTP503Responder` (BackendKind 35 — 503 ∈ the gateway set `{502,503,504}`); the local-origin fixture reuses the driver-side `allocDeadPort()` dead-host mechanism (a host:port with no listener → connect refused, agreed cross-side; the `0066`/`0067` precedent — NOT a runner backend). BackendKind tail STAYS **35**.

### 1.2 ADR continuity + D-disposition at SPEC commit

ADR-0246 (the gateway + local-origin consecutive detectors + the `split_external_local_origin_errors` accounting switch + the live `LocalOriginErr` seam population at the connection-failure sites + the picked-endpoint-on-connect-failure attribution) — §Context DRAFT anchored here (§13); the full §Decision/§Consequences land at the 40.2 IMPL per ADR-0044. DECISIONS tail STAYS ADR-0245 at this SPEC; next-free ADR-0246. The §10 BRAINSTORM D-OD pins for the 40.2 detectors are RESOLVED in §11 (D-OD2-*); the PLAN/IMPL D-questions are §12. ADR-0245 (the 40.1 seam + ejection dimension + `available` predicate) is REUSED UNCHANGED.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8)

- **40.3:** the STATISTICAL detectors — `success_rate_*` (`success_rate_minimum_hosts`/`success_rate_request_volume`/`success_rate_stdev_factor`) + `failure_percentage_*` (`failure_percentage_threshold`/`_minimum_hosts`/`_request_volume`) + their local-origin variants (`local_origin_success_rate` / `local_origin_failure_percentage`, gated by `enforcing_local_origin_success_rate`) + the NEW per-interval cross-host mean/stddev aggregation runtime (the first new outlier goroutine) + the 8 statistical `detected_`/`enforced_` stat pairs.
- The `interval`-driven un-eject SWEEP + the `base_ejection_time × num_ejections` linear backoff + its decay (40.1 AMEND-OD1/OD2 — STAY recorded departures), `max_ejection_time` + `max_ejection_time_jitter`, `successful_active_health_check_uneject_host`, `always_eject_one_host`, the legacy/statistical stats, outlier event logging, the `/clusters` per-host ejection readout, outlier detection on non-HTTP upstreams.
- The recovery/un-eject-timing differential arm (the lazy-vs-sweep timing diverges cross-side — STAYS deferred, the 40.1 `0069` posture).
- Envoy's finer local-origin sub-classification (connect-failed vs connect-timeout vs connection-termination): envoy-go 40.2 classifies ALL connection-level failures (connect refused/timeout, write reset, read/response reset) as a single local-origin failure (a recorded simplification — the consecutive_local_origin_failure detector counts them identically).

---

## 3. The two detectors + the split accounting + the live `LocalOriginErr` seam (ADR-0246)

### 3.0 Split disposition — 40.2 (the 3-leg split)

40.2 = the gateway + local-origin consecutive detectors + the split switch + the live `LocalOriginErr` seam + 2 differential fixtures. The ADR-0045 split-gate is re-checked at the PLAN (anticipated ~150–300 prod LoC / ~10–14 tasks — comfortably under `> ~25 tasks OR > ~1500 LoC`; single flat 40.2 leg). REUSES the 40.1 ejection lifecycle wholesale.

### 3.1 Parse extension (`parseOutlierDetection`)

`outlierConfig` (`internal/cluster/outlier.go`) gains:
```go
// gateway detector — active-by-default (detect-only by default).
consecGwEnabled  bool   // false iff consecutive_gateway_failure explicitly 0
consecutiveGw    uint32 // threshold (default 5)
enforcingGw      uint32 // enforce-roll % (default 0 ⇒ detect-only)
// local-origin detector — takes effect only when split is true.
splitLocalOrigin bool   // split_external_local_origin_errors (default false)
consecLOEnabled  bool   // false iff consecutive_local_origin_failure explicitly 0
consecutiveLO    uint32 // threshold (default 5)
enforcingLO      uint32 // enforce-roll % (default 100)
```
Default population mirrors 40.1's `consecutive_5xx` handling (absent ⇒ the proto default, enabled; explicit 0 ⇒ that detector off). The `enforcing_consecutive_gateway_failure`/`enforcing_consecutive_local_origin_failure` rolls reuse the 40.1 `enforceRoll()` mechanism (≥100 ⇒ always; 0 ⇒ never — for gateway, the default; intermediate ⇒ the PCG roll).

### 3.2 Detector extension (`outlierDetector.record`) — branch on `LocalOriginErr` + `split`

`RecordUpstreamResult(ep, UpstreamResult{StatusCode, LocalOriginErr})` → `record(ep, statusCode, localOriginErr)`:

1. **Lazy un-eject fast-path** (unchanged from 40.1).
2. **`localOriginErr == true`:**
   - if `splitLocalOrigin`: feed the **local-origin** detector only — `consecLO++`; if `consecLO >= consecutiveLO` → `ejections_detected_consecutive_local_origin_failure++`; if not-already-ejected AND the `enforcingLO` roll passes AND the cap permits → eject (`ejections_enforced_total++` + `ejections_enforced_consecutive_local_origin_failure++`, the double-count; `ejections_active++`); else cap-blocked → `ejections_overflow++`. The external (5xx/gateway) counters are NOT touched.
   - else (`split=false`): treat as a **gateway-class 5xx** — run the gateway + 5xx detector path below as if `statusCode` were a gateway error (the local-origin detector is inactive).
3. **`localOriginErr == false` (an externally-originated HTTP status):**
   - if `splitLocalOrigin`: a completed external response means the connection SUCCEEDED → reset `consecLO = 0`.
   - if NOT 5xx (`statusCode` outside `[500,599]`): reset `consec5xx = 0` AND `consecGw = 0` (a good response clears the external streaks); return.
   - if 5xx: BOTH consecutive counters advance per the gateway-first ordering (AMEND-OD2-2 — only an *ejection* clears them, NOT a mere detection):
     - **gateway-class** (`statusCode ∈ {502,503,504}`): `consecGw++`; if `consecGw >= consecutiveGw` → `ejections_detected_consecutive_gateway_failure++`; if not-already-ejected AND the `enforcingGw` roll passes AND the cap permits → eject (gateway double-count: `ejections_enforced_total++` + `ejections_enforced_consecutive_gateway_failure++`); else if cap-blocked → `ejections_overflow++`.
     - **non-gateway 5xx** (`{500,501,505..599}`): reset `consecGw = 0` (a non-gateway 5xx breaks the gateway streak).
     - then the **5xx** detector (the 40.1 path, UNCHANGED): UNLESS the gateway detector just EJECTED the host, control FALLS THROUGH and `consec5xx++` runs (both consecutive counters advance on every 5xx — the AMEND-OD2-2 invariant; this is the DEFAULT-config path `0069`/`0070` exercise, where `enforcing_consecutive_gateway_failure` is 0/detect-only so the gateway detector counts but does not eject, and the 5xx detector at `enforcing` 100 is the one that ejects). The 5xx detector is SKIPPED ONLY when the gateway detector just ejected the host — that eject clears `consec5xx` + removes the host from rotation, so `consec5xx` never reaches its threshold, replicating the live `detected_consecutive_5xx: 0` pin (the gateway-enforcing-100 case). **Note (split=false mapping):** a `LocalOriginErr` result under split=false reaches this path with `statusCode` = the local-reply code (H1 acquire ⇒ 503, all other sites ⇒ 502), which is ALWAYS gateway-class — so it correctly flows gateway-first then 5xx (D-S40.2-2).

The `max_ejection_percent` cap (the 40.1 cross-multiplied `(ejected+1)*100 <= cap*total`), the CAS-once-only eject, the `enforceRoll`, and the lazy un-eject are REUSED for all three consecutive detectors.

### 3.3 The seam — populate `LocalOriginErr` at the connection-failure sites (AMEND-OD2-4)

The router sets `UpstreamResult{StatusCode: <the local-reply code>, LocalOriginErr: true}` and calls `RecordUpstreamResult(picked, …)` at:
- **H1** (`router.go`): `AcquireH1` dial/TLS failure → 503 (`:610-614`); `req.Write` failure → 502 (`:637-640`); `http.ReadResponse` failure → 502 (`:648-652`).
- **H2** (`router_h2.go`): `DialH2` failure → 502 (`:76-80`); `RoundTrip` non-ctx-cancel failure → 502 (`:96-98`). The ctx-cancel/deadline sentinel path (`:90-95`) is a DOWNSTREAM cancel, NOT an upstream failure → NO `RecordUpstreamResult` call.

The picked endpoint is surfaced from the shared `Cluster.Dial`/`AcquireH1` connect-failure path (the framework touch — today returns `Endpoint{}`). When the LB pick itself fails (no host), `picked` stays `Endpoint{}` and the seam is NOT called (no per-host attribution). The 40.1 SUCCESS sites (`router.go:655` + `router_h2.go:100`) stay UNCHANGED (`LocalOriginErr: false`, the real status). `RecordUpstreamResult` stays a no-op when `c.outlier == nil` (byte-identical for non-outlier clusters — the connect-failure local-reply behavior is otherwise unchanged).

---

## 4. Framework primitives — the two detectors + the split accounting + the connect-failure attribution over the 40.1 substrate + 0 new packages + 0 new go.mod deps

- NEW: the gateway + local-origin detector logic + the split branch in `outlier.go`'s `record`/`parseOutlierDetection`; the per-host `consecGw`/`consecLO` counters on `hostHealth`; the +4 stat registrations; the `LocalOriginErr` population at the five connection-failure sites; the picked-endpoint surfacing on the shared `Cluster.Dial`/`AcquireH1` connect-failure path (the one framework touch).
- REUSED UNCHANGED (ADR-0245): the `RecordUpstreamResult` seam + `UpstreamResult` struct (`LocalOriginErr` now READ); the ejection dimension on `hostHealth` (`ejected`/`unejectAtNanos`/`ejectCount`); the `available = isHealthy && !isEjected` predicate + the five leaf consult sites + the `availableCount` panic denominator; the lazy un-eject in `isEjected`; the `max_ejection_percent` cap + `ejections_active`/`ejections_enforced_total`/`ejections_overflow` stats; the `enforceRoll` PCG mechanism; the `clusterHealth` registry-creation widening.
- ZERO new Go packages. ZERO new go.mod modules (`cluster.v3.OutlierDetection` already in the existing go-control-plane v1.32.4 dep — all 40.2 fields present; `go mod tidy -diff` EMPTY — D-OD2-PROTO).

---

## 5. Proto-field roster (per §11 D-OD2-PROTO)

The 40.2-CONSUMED subset of `cluster.v3.OutlierDetection` (in addition to the 40.1 fields 1–5):

| # | Field | Type | Default | 40.2 role |
|---|-------|------|---------|-----------|
| 10 | `consecutive_gateway_failure` | UInt32Value | 5 | gateway detector threshold (0 ⇒ off); gateway codes `{502,503,504}` |
| 11 | `enforcing_consecutive_gateway_failure` | UInt32Value | **0** | gateway enforce-roll % (0 ⇒ detect-only — the default) |
| 12 | `split_external_local_origin_errors` | bool | false | the accounting switch (AMEND-OD2-3) |
| 13 | `consecutive_local_origin_failure` | UInt32Value | 5 | local-origin detector threshold; takes effect only when split=true |
| 14 | `enforcing_consecutive_local_origin_failure` | UInt32Value | 100 | local-origin enforce-roll %; takes effect only when split=true |

The statistical-detector fields (`success_rate_*`, `failure_percentage_*`, `enforcing_local_origin_success_rate` [15], `max_ejection_time` [+ jitter], `successful_active_health_check_uneject_host`, `always_eject_one_host`) stay PARSE-ACCEPTED-and-IGNORED at 40.2 (40.3 / deferred). Gateway codes `{502,503,504}` confirmed in the proto comment + live (a 503 trips the gateway detector); the defaults confirmed LIVE (D-OD2-PROTO).

---

## 6. PARSE-REJECT roster (per §11 D-OD2-REJECT + ADR-0080)

Two NEW reject arms (the 40.1 house prefix `cluster: %q: outlier_detection: <reason>`), mirroring the reference PGV bounds:
- `enforcing_consecutive_gateway_failure > 100` (reference: `OutlierDetectionValidationError.EnforcingConsecutiveGatewayFailure: value must be less than or equal to 100`).
- `enforcing_consecutive_local_origin_failure > 100` (reference: `…EnforcingConsecutiveLocalOriginFailure: value must be less than or equal to 100`).

The 40.1 reject arms (`max_ejection_percent`/`enforcing_consecutive_5xx > 100`; `interval`/`base_ejection_time <= 0s`) are UNCHANGED. The statistical-field rejects (`failure_percentage_threshold`/`enforcing_local_origin_success_rate > 100`; `max_ejection_time <= 0s` — all live-confirmed present) are DEFERRED to 40.3. All unit-level (no boot-reject dir — the `0069` precedent). Exact house wording pinned at the PLAN/IMPL (§12).

---

## 7. Stat surface — +4 (1137 → 1141) (per §11 D-OD2-STATS + AMEND-OD2-5)

Emitted ONLY on clusters with `outlier_detection`. Scoped `cluster.<name>.outlier_detection.`:
1. `ejections_detected_consecutive_gateway_failure` — counter (gateway threshold crossings; increments on gateway-class 5xx even when detect-only).
2. `ejections_enforced_consecutive_gateway_failure` — counter (actual gateway-triggered ejections).
3. `ejections_detected_consecutive_local_origin_failure` — counter (local-origin threshold crossings; split=true only).
4. `ejections_enforced_consecutive_local_origin_failure` — counter (actual local-origin-triggered ejections; split=true only).

The double-count is EXTENDED (D-OD2-STATS): a gateway ejection ⇒ `ejections_enforced_total++` AND `ejections_enforced_consecutive_gateway_failure++`; a local-origin ejection ⇒ `ejections_enforced_total++` AND `ejections_enforced_consecutive_local_origin_failure++`. The 40.1 `ejections_active`/`ejections_enforced_total`/`ejections_overflow` are cross-detector (REUSED). DEFERRED departures (NOT emitted at 40.2): the legacy `ejections_total`/`ejections_consecutive_5xx`/`ejections_success_rate`; the 8 statistical `ejections_{detected,enforced}_{success_rate,failure_percentage,local_origin_success_rate,local_origin_failure_percentage}` (40.3).

---

## 8. Differential fixture taxonomy (+2: gateway + local-origin; ZERO new BackendKind)

### 8.1 `0070-outlier-detection-consecutive-gateway-failure` (cross-side; reuses `HTTP503Responder`)

A cluster `{2 healthy `HTTPEcho`, 1 `HTTP503Responder`}` with `outlier_detection: { consecutive_gateway_failure: <N>, enforcing_consecutive_gateway_failure: 100, consecutive_5xx: <high — so the 5xx detector does NOT fire first>, interval, base_ejection_time, max_ejection_percent: 100 }`, on BOTH sides. The driver drives ≥N+ requests so the 503 host accrues `consecutive_gateway_failure` consecutive gateway 5xx, POLLS `/stats` until `ejections_active == 1` on BOTH sides (the `0066`/`0069` poll-to-converge + warmup-until-K-200s + delta-counter pattern, `reference_health_check_propagation_warmup`), then asserts the measured load phase routes 100% to the 2 healthy hosts + cross-side parity on `{ejections_active, ejections_enforced_total, ejections_detected_consecutive_gateway_failure, ejections_enforced_consecutive_gateway_failure}` AND `ejections_detected_consecutive_5xx == 0` / `ejections_enforced_consecutive_5xx == 0` (the gateway-ejects-first pin, AMEND-OD2-2). Verify `upstream_rq_total > 0` reference-side. 2 `-count=1` deliberate breaks: (A) the gateway detector no-ops → `ejections_active` never converges; (B) the gateway eject does not clear/replicate the 5xx-stays-0 behavior → cross-side `detected_consecutive_5xx` parity fails.

### 8.2 `0071-outlier-detection-local-origin` (cross-side; reuses the `allocDeadPort` dead host; split=true)

A cluster `{2 healthy `HTTPEcho`, 1 DEAD host (an unbound `host:port` → connect refused, agreed cross-side via `allocDeadPort()` — NOT a runner backend, the `0066`/`0067` precedent)}` with `outlier_detection: { consecutive_local_origin_failure: <N>, enforcing_consecutive_local_origin_failure: 100, split_external_local_origin_errors: true, interval, base_ejection_time, max_ejection_percent: 100 }`, on BOTH sides. The driver drives ≥N+ requests so the dead host accrues `consecutive_local_origin_failure` consecutive connect failures, POLLS `/stats` until `ejections_active == 1` on BOTH sides, then asserts traffic concentrates on the 2 healthy hosts + cross-side parity on `{ejections_active, ejections_enforced_total, ejections_detected_consecutive_local_origin_failure, ejections_enforced_consecutive_local_origin_failure}` AND `ejections_detected_consecutive_5xx == 0` (split=true routes local-origin away from the 5xx detector — AMEND-OD2-3). 2 `-count=1` deliberate breaks: (A) the local-origin detector no-ops (or `LocalOriginErr` never populated) → `ejections_active` never converges; (B) the connect failure attributes to the wrong host / no host (picked endpoint not surfaced) → the dead host never ejects. This break is LIVE (not vacuous): an un-surfaced `Endpoint{}` (zero addr) misses `record`'s unknown-addr guard (`outlier.go:115-117 if !ok { return }` — the registry is keyed on real host addrs, so the zero addr is genuinely unknown), so `consecLO` never increments and the dead host never ejects (the PLAN's break-liveness proof, `reference_differential_break_protocol_count1`).

### 8.3 The split=false local-origin-as-5xx arm — a UNIT test (not a fixture)

The split=false mapping (a local-origin failure counted by the `consecutive_5xx`/gateway detectors) is covered by a `record`-level unit test (the cross-side observable would duplicate the dead-host driver of `0071` with only a config flip; the per-request mapping is best asserted directly). The EXISTING `0069` (split default false, external 503s) is WIDENED to cross-assert `ejections_detected_consecutive_gateway_failure` (AMEND-OD2-1 — now that envoy-go emits it).

### 8.4 Total + no new fuzzer

Differential fixtures **71 → 73** (`0070` + `0071`). NO new wire decoder → fuzzers STAY **42**. The PLAN re-checks whether `0070`+`0071` fold into one dir with two cluster/listener arms vs two dirs (the `0067`/`0068` two-dir precedent favors two dirs; the differential-fixture-dispatch one-dir-one-branch constraint permits either since both are cross-side).

---

## 9. Behavior-contract delta (the 40.2 bundle; ADR-0052 atomic landing)

Extend the `### Cluster — passive health (outlier detection)` subsection: the `consecutive_gateway_failure` detector (gateway codes `{502,503,504}`; active-by-default, detect-only by default at `enforcing` 0; gateway-checked-first + the once-ejected counter reset); the `consecutive_local_origin_failure` detector (split=true only); the `split_external_local_origin_errors` accounting switch (false ⇒ local-origin mapped to 5xx/gateway; true ⇒ local-origin to its own detector, externals to the 5xx/gateway detectors); the `LocalOriginErr` seam population at the connection-failure sites + the picked-endpoint-on-connect-failure attribution; the +4 stats + the extended double-count. The stat-surface block advances 1137 → 1141. Record the connection-failure-classification simplification (all connection-level failures = one local-origin class) and the `0069` gateway-detected widening.

---

## 10. Per-task structure (~10–14 tasks; PLAN decomposes)

Anticipated spine: (1) baselines/PROGRESS; (2) the `consecGw`/`consecLO` counters on `hostHealth` + the `parseOutlierDetection` extension (gateway/local-origin/split fields + defaults + the 2 reject arms) + unit tests; (3) the `record` gateway-detector path (gateway-first ordering + the once-ejected reset + the `0069`-gateway-detected emission) + unit tests (incl. the `detected_5xx == 0`-on-gateway-eject case); (4) the `record` local-origin + split branch (split=true local-origin-only; split=false local-origin-as-5xx) + unit tests; (5) the +4 stat registrations; (6) surface the picked endpoint on the shared `Cluster.Dial`/`AcquireH1` connect-failure path + the `LocalOriginErr` population at the five router connection-failure sites (NOT the legacy drivers) + the no-host-no-attribution guard; (7) the `0070` gateway fixture; (8) the `0071` local-origin fixture (`allocDeadPort` dead host); (9) deliberate-breaks + 20-run flake (`-count=1`, `TestDifferential/0070`/`0071`); (10) widen the `0069` `StatsAsserter` (gateway-detected) + full 73-dir differential + six-gate; (11) ADR-0246 body + BEHAVIOR_CONTRACT; (12) completion bundle. The PLAN runs the FINAL ADR-0045 split-gate re-check (anticipated NO FURTHER SPLIT).

---

## 11. SPEC-time empirical-pin block (D-OD2-* — executed IN-SESSION 2026-06-16)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a shared `odnet` bridge; three STRICT_DNS clusters [gateway: 2×200 + 1×503; local-origin split=true: 2×200 + 1×dead; local-origin split=false: 2×200 + 1×dead]; published admin `:19500`; request path verified `upstream_rq_total > 0`) + the go-control-plane v1.37.0 module cache (the v1.32.4-equivalent proto surface).

| Pin | Disposition |
|-----|-------------|
| **D-OD2-PROTO** | CONFIRMED. Fields 10–14: `consecutive_gateway_failure` (UInt32Value, default 5, codes `{502,503,504}`), `enforcing_consecutive_gateway_failure` (default **0** ⇒ detect-only), `split_external_local_origin_errors` (bool, default false), `consecutive_local_origin_failure` (default 5, split=true only), `enforcing_consecutive_local_origin_failure` (default 100, split=true only). All present in the existing dep; `go mod tidy -diff` anticipated EMPTY → ZERO new module. |
| **D-OD2-STATS** | CONFIRMED. The full 20-name reference roster scraped. 40.2's +4: `ejections_{detected,enforced}_consecutive_gateway_failure` + `ejections_{detected,enforced}_consecutive_local_origin_failure`. Double-count verified LIVE: gateway eject ⇒ `enforced_total` + `enforced_consecutive_gateway_failure` both ++; local-origin eject ⇒ `enforced_total` + `enforced_consecutive_local_origin_failure` both ++. Surface +4 → 1141. |
| **D-OD2-LIFECYCLE** | PINNED. Gateway-checked-FIRST: with `consecutive_5xx: 3` + `consecutive_gateway_failure: 3` + gateway `enforcing: 100`, a 503 host ejects via gateway and `detected_consecutive_5xx` stays **0** (AMEND-OD2-2). Split accounting (AMEND-OD2-3): split=false ⇒ a connect failure → `detected/enforced_consecutive_5xx` (local-origin detector inactive); split=true ⇒ the same connect failure → `detected/enforced_consecutive_local_origin_failure` ONLY (`detected_consecutive_5xx`/`_gateway_failure` stay 0). External 5xx always feed the gateway/5xx detectors regardless of split. |
| **D-OD2-REJECT** | PINNED. `EnforcingConsecutiveGatewayFailure`/`EnforcingConsecutiveLocalOriginFailure: value must be less than or equal to 100` (both live-confirmed via `--mode validate`); envoy-go mirrors with house wording (§6). The statistical-field rejects (`FailurePercentageThreshold`/`EnforcingLocalOriginSuccessRate <= 100`; `MaxEjectionTime > 0s`) confirmed present but DEFERRED to 40.3. |
| **D-OD2-SEAM** | PINNED. The five connection-failure sites (H1 acquire-503/write-502/read-502; H2 dial-502/roundtrip-502) populate `LocalOriginErr: true`; the H1-acquire-503 + H2-dial-502 sites need the picked endpoint surfaced from the shared `Cluster.Dial`/`AcquireH1` connect-failure path (today returns `Endpoint{}`); the LB-pick-itself-failed path attributes to no host (no seam call); the ctx-cancel H2 path is a downstream cancel (no seam call). The 40.1 success sites + the legacy `do`/`doH2` drivers stay UNCHANGED (AMEND-OD2-4). |
| **D-OD2-BACKEND** | PINNED. ZERO new BackendKind — `0070` reuses `HTTP503Responder` (35; 503 ∈ gateway set); `0071` reuses the `allocDeadPort()` dead-host mechanism (`0066`/`0067` precedent). Tail STAYS 35 (AMEND-OD2-6). |

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S40.2-1** the exact envoy-go house reject wording for §6 (the two `enforcing_consecutive_{gateway,local_origin}_failure > 100` strings).
- **D-S40.2-2** the precise `record` reset ordering replicating the `detected_consecutive_5xx == 0`-on-gateway-eject pin (AMEND-OD2-2) — gateway-first with a 5xx-streak reset on gateway eject, or an explicit "host already ejected ⇒ skip the 5xx detected increment" guard; pick the form that reproduces the live cross-side parity with the simplest invariant.
- **D-S40.2-3** the signature for surfacing the picked endpoint on the shared `Cluster.Dial`/`AcquireH1` connect-failure path (widen `AcquireH1` to `(*PooledH1Conn, Endpoint, error)` mirroring `DialH2`'s `(…, Endpoint, error)`, and surface `ep` from `Dial` on the `DialContext`/handshake-failure paths) vs an internal cluster-side record — pick the minimal change preserving the 40.1 success-site shape.
- **D-S40.2-4** the only OPEN local-origin-classification nuance — whether the H1 `req.Write`/`ReadResponse` 502 sites (post-connect resets, distinct from a pure connect failure) are in or out of the local-origin class (anticipated IN, at the MVP). The single-`LocalOriginErr`-class simplification (all of {connect-refused, connect-timeout, write-reset, read/response-reset} → `LocalOriginErr: true`) is already SETTLED in §2/§3.3 — not re-litigated here.
- **D-S40.2-5** `0070`/`0071` constants (N / `consecutive_5xx`-high-value / interval / base_ejection_time / backendCount / N-requests / convergeDeadline / warmupStable / refContainerListenerPort) single-sourced (`reference_fixture_workload_constant_desync`); the `0069` `StatsAsserter` gateway-detected widening.
- **D-S40.2-6** whether `0070`+`0071` are two dirs or one dir with two cluster/listener arms (the `0067`/`0068` two-dir precedent vs a folded dir).
- **D-S40.2-7** the ADR-0045 final split-gate re-check (anticipated NO FURTHER SPLIT).

---

## 13. ADR continuity — the ADR-0246 §Context DRAFT (anchored here; full entry lands at the 40.2 IMPL)

**ADR-0246 §Context (draft).** Phase 40.1 (ADR-0245) established passive outlier detection over the phase-39 host-health registry: the per-request `RecordUpstreamResult` seam (fired at the two live-driver SUCCESS sites), a per-host ejection sub-state on `hostHealth`, the `available = isHealthy && !isEjected` LB-pick predicate, and the single `consecutive_5xx` detector — with `UpstreamResult.LocalOriginErr` carried forward-compatibly but UNREAD. Phase 40.2 is the SECOND leg of the pre-authorized 3-leg by-detector-class split: the OTHER consecutive detectors. The §11 live pins (D-OD2-PROTO/STATS/LIFECYCLE/REJECT/SEAM/BACKEND, executed in-session against `contrib-v1.37.2`) firmed: the gateway detector counts gateway-class 5xx `{502,503,504}`, active-by-default but detect-only by default (`enforcing_consecutive_gateway_failure` default 0); the gateway detector is evaluated before the 5xx detector and an ejection clears both consecutive streaks (the live `detected_consecutive_5xx: 0`-on-gateway-eject pin); `split_external_local_origin_errors` is the accounting switch (false ⇒ local-origin failures mapped to the 5xx/gateway detectors; true ⇒ a dedicated local-origin detector, externals to the 5xx/gateway detectors); and `LocalOriginErr` becomes live by populating it at the router connection-failure sites, requiring the picked endpoint to be surfaced on the shared connect-failure path (the one framework touch). The +4 detected/enforced stat pairs extend the 40.1 double-count; the 40.1 seam + ejection dimension + `available` predicate + cap + lazy un-eject are REUSED UNCHANGED. The statistical detectors + the per-interval aggregation runtime stay deferred to 40.3. §Decision + §Consequences land at the 40.2 IMPL.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at this SPEC: stat surface **1137** / fixtures **71** / fuzzers **42** / BackendKind tail **35** / DECISIONS tail **ADR-0245** (next-free **ADR-0246**). ROADMAP row 40 STAYS `in-progress` (the 40.2 SPEC note appended; the row flips `done` only when the 40.3 final leg lands — `reference_roadmap_split_phase_row_done`). Anticipated at the 40.2 IMPL: fixtures 71 → 73 (`0070` + `0071`), BackendKind tail 35 (UNCHANGED), DECISIONS tail ADR-0245 → ADR-0246 (next-free ADR-0247), stat surface 1137 → 1141 (+4), fuzzers 42 (UNCHANGED), ZERO new packages + ZERO new go.mod modules. Next → the phase-40.2 PLAN (`superpowers:writing-plans`).
