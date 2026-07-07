package cluster

import (
	"context"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"

	"github.com/pgdad/envoy-go/internal/stats"
)

// outlierConfig is the parsed, validated outlier_detection envelope (passive
// health checking; cluster.v3.OutlierDetection, Cluster field 19). Task 6
// consumes it to build the detector; Task 4 only parses + validates.
type outlierConfig struct {
	consec5xxEnabled bool          // false when consecutive_5xx threshold == 0 (D-S40.1-2)
	consecutive5xx   uint32        // threshold (default 5 when field absent)
	baseEjectionTime time.Duration // default 30s (flat)
	maxEjectionPct   uint32        // default 10
	enforcing5xx     uint32        // default 100
	// gateway detector — active-by-default (detect-only by default; AMEND-OD2-1).
	consecGwEnabled bool   // false iff consecutive_gateway_failure explicitly 0
	consecutiveGw   uint32 // threshold (default 5)
	enforcingGw     uint32 // enforce-roll % (default 0 ⇒ detect-only)
	// local-origin detector — takes effect only when split is true.
	splitLocalOrigin bool   // split_external_local_origin_errors (default false)
	consecLOEnabled  bool   // false iff consecutive_local_origin_failure explicitly 0
	consecutiveLO    uint32 // threshold (default 5)
	enforcingLO      uint32 // enforce-roll % (default 100)
	// success_rate detector — sweep-driven; ejects-by-default (enforcing 100).
	successRateMinHosts    uint32 // success_rate_minimum_hosts (default 5)
	successRateReqVolume   uint32 // success_rate_request_volume (default 100)
	successRateStdevFactor uint32 // success_rate_stdev_factor (default 1900; /1000 ⇒ 1.9)
	enforcingSuccessRate   uint32 // enforcing_success_rate (default 100)
	// failure_percentage detector — sweep-driven; detect-only-by-default (enforcing 0).
	failurePctThreshold uint32 // failure_percentage_threshold (default 85)
	failurePctMinHosts  uint32 // failure_percentage_minimum_hosts (default 5)
	failurePctReqVolume uint32 // failure_percentage_request_volume (default 50)
	enforcingFailurePct uint32 // enforcing_failure_percentage (default 0 ⇒ detect-only)
	// interval is parse-validated (gt:0s) at 40.1; NOW load-bearing as the sweep
	// cadence (default 10s).
	interval time.Duration
}

// parseOutlierDetection validates + converts the cluster's outlier_detection.
// Returns (nil, nil) when the cluster has no outlier_detection. Byte-stable
// rejects (ADR-0080) under the house prefix `cluster: %q: outlier_detection: `;
// the reference's PGV bounds are hand-rolled here. All non-validated fields are
// parse-accepted-and-ignored (silent).
func parseOutlierDetection(c *clusterv3.Cluster, name string) (*outlierConfig, error) {
	od := c.GetOutlierDetection()
	if od == nil {
		return nil, nil
	}

	// Reject roster — validate only when the field is SET (non-nil).
	if v := od.GetMaxEjectionPercent(); v != nil && v.GetValue() > 100 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: max_ejection_percent: value must be less than or equal to 100", name)
	}
	if v := od.GetEnforcingConsecutive_5Xx(); v != nil && v.GetValue() > 100 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_consecutive_5xx: value must be less than or equal to 100", name)
	}
	if d := od.GetInterval(); d != nil && d.AsDuration() <= 0 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: interval: value must be greater than 0s", name)
	}
	if d := od.GetBaseEjectionTime(); d != nil && d.AsDuration() <= 0 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: base_ejection_time: value must be greater than 0s", name)
	}
	if v := od.GetEnforcingConsecutiveGatewayFailure(); v != nil && v.GetValue() > 100 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_consecutive_gateway_failure: value must be less than or equal to 100", name)
	}
	if v := od.GetEnforcingConsecutiveLocalOriginFailure(); v != nil && v.GetValue() > 100 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_consecutive_local_origin_failure: value must be less than or equal to 100", name)
	}
	// statistical detector reject arms (40.3). NO arm for success_rate_stdev_factor
	// (the reference ACCEPTS 0).
	if v := od.GetEnforcingSuccessRate(); v != nil && v.GetValue() > 100 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_success_rate: value must be less than or equal to 100", name)
	}
	if v := od.GetEnforcingFailurePercentage(); v != nil && v.GetValue() > 100 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_failure_percentage: value must be less than or equal to 100", name)
	}
	if v := od.GetFailurePercentageThreshold(); v != nil && v.GetValue() > 100 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: failure_percentage_threshold: value must be less than or equal to 100", name)
	}
	if v := od.GetEnforcingLocalOriginSuccessRate(); v != nil && v.GetValue() > 100 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_local_origin_success_rate: value must be less than or equal to 100", name)
	}
	if v := od.GetEnforcingFailurePercentageLocalOrigin(); v != nil && v.GetValue() > 100 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_failure_percentage_local_origin: value must be less than or equal to 100", name)
	}
	if d := od.GetMaxEjectionTime(); d != nil && d.AsDuration() <= 0 {
		return nil, fmt.Errorf("cluster: %q: outlier_detection: max_ejection_time: value must be greater than 0s", name)
	}

	cfg := &outlierConfig{
		consecutive5xx:   5,
		baseEjectionTime: 30 * time.Second,
		maxEjectionPct:   10,
		enforcing5xx:     100,
	}
	// D-S40.1-2: absent ⇒ default 5 enabled; explicit value ⇒ enabled iff != 0
	// (explicit 0 turns the consecutive_5xx detector OFF).
	if v := od.GetConsecutive_5Xx(); v == nil {
		cfg.consecutive5xx = 5
		cfg.consec5xxEnabled = true
	} else {
		cfg.consecutive5xx = v.GetValue()
		cfg.consec5xxEnabled = cfg.consecutive5xx != 0
	}
	if d := od.GetBaseEjectionTime(); d != nil {
		cfg.baseEjectionTime = d.AsDuration()
	}
	if v := od.GetMaxEjectionPercent(); v != nil {
		cfg.maxEjectionPct = v.GetValue()
	}
	if v := od.GetEnforcingConsecutive_5Xx(); v != nil {
		cfg.enforcing5xx = v.GetValue()
	}
	// gateway (default 5; active-by-default; enforcing default 0).
	if v := od.GetConsecutiveGatewayFailure(); v == nil {
		cfg.consecutiveGw = 5
		cfg.consecGwEnabled = true
	} else {
		cfg.consecutiveGw = v.GetValue()
		cfg.consecGwEnabled = cfg.consecutiveGw != 0
	}
	cfg.enforcingGw = 0 // default
	if v := od.GetEnforcingConsecutiveGatewayFailure(); v != nil {
		cfg.enforcingGw = v.GetValue()
	}
	// split (default false).
	cfg.splitLocalOrigin = od.GetSplitExternalLocalOriginErrors()
	// local-origin (default 5; enforcing default 100).
	if v := od.GetConsecutiveLocalOriginFailure(); v == nil {
		cfg.consecutiveLO = 5
		cfg.consecLOEnabled = true
	} else {
		cfg.consecutiveLO = v.GetValue()
		cfg.consecLOEnabled = cfg.consecutiveLO != 0
	}
	cfg.enforcingLO = 100 // default
	if v := od.GetEnforcingConsecutiveLocalOriginFailure(); v != nil {
		cfg.enforcingLO = v.GetValue()
	}
	// success_rate detector defaults (40.3).
	cfg.successRateMinHosts = 5
	if v := od.GetSuccessRateMinimumHosts(); v != nil {
		cfg.successRateMinHosts = v.GetValue()
	}
	cfg.successRateReqVolume = 100
	if v := od.GetSuccessRateRequestVolume(); v != nil {
		cfg.successRateReqVolume = v.GetValue()
	}
	cfg.successRateStdevFactor = 1900
	if v := od.GetSuccessRateStdevFactor(); v != nil {
		cfg.successRateStdevFactor = v.GetValue() // 0 accepted — no reject
	}
	cfg.enforcingSuccessRate = 100
	if v := od.GetEnforcingSuccessRate(); v != nil {
		cfg.enforcingSuccessRate = v.GetValue()
	}
	// failure_percentage detector defaults (40.3).
	cfg.failurePctThreshold = 85
	if v := od.GetFailurePercentageThreshold(); v != nil {
		cfg.failurePctThreshold = v.GetValue()
	}
	cfg.failurePctMinHosts = 5
	if v := od.GetFailurePercentageMinimumHosts(); v != nil {
		cfg.failurePctMinHosts = v.GetValue()
	}
	cfg.failurePctReqVolume = 50
	if v := od.GetFailurePercentageRequestVolume(); v != nil {
		cfg.failurePctReqVolume = v.GetValue()
	}
	cfg.enforcingFailurePct = 0
	if v := od.GetEnforcingFailurePercentage(); v != nil {
		cfg.enforcingFailurePct = v.GetValue()
	}
	// interval — load-bearing as the sweep cadence (40.3); already validated > 0s
	// by the reject arm above.
	cfg.interval = 10 * time.Second
	if d := od.GetInterval(); d != nil {
		cfg.interval = d.AsDuration()
	}
	return cfg, nil
}

// outlierDetector applies upstream results to the shared host-health registry,
// performing consecutive_5xx detect/eject/cap/enforce-roll (SPEC §3.3). The
// ejection state lives on hostHealth (shared with active HC); the detector owns
// the streak accounting and the eject decision. Stat handles are nil-guarded for
// bare unit constructions; Task 8 injects the real ones.
type outlierDetector struct {
	cfg         outlierConfig
	health      *clusterHealth // shared registry (ejection lives on hostHealth)
	endpoints   []Endpoint     // for the max_ejection_percent denominator
	enforceRoll func() uint32  // [0,100); injectable; default PCG-seeded (wired in Task 6)

	ejectionsActive        *stats.Gauge
	ejectionsEnforcedTotal *stats.Counter
	ejectionsOverflow      *stats.Counter
	ejectionsDetected5xx   *stats.Counter
	ejectionsEnforced5xx   *stats.Counter
	// gateway detector handles (allocated in registerStats at Task 5; nil-guarded
	// here so Task-3 runtime constructions tolerate the absence). (ADR-0246)
	ejectionsDetectedGw *stats.Counter
	ejectionsEnforcedGw *stats.Counter
	// local-origin detector handles (allocated in registerStats at Task 5; nil-
	// guarded here so Task-4 runtime constructions tolerate the absence). (ADR-0246)
	ejectionsDetectedLO *stats.Counter
	ejectionsEnforcedLO *stats.Counter
	// success_rate detector handles (allocated in registerStats at Task 7; nil-
	// guarded here so unit constructions tolerate the absence). (ADR-0247)
	ejectionsDetectedSR *stats.Counter // ejections_detected_success_rate
	ejectionsEnforcedSR *stats.Counter // ejections_enforced_success_rate
	// failure_percentage detector handles (allocated in registerStats at Task 7;
	// nil-guarded here so unit constructions tolerate the absence). (ADR-0247)
	ejectionsDetectedFP *stats.Counter // ejections_detected_failure_percentage
	ejectionsEnforcedFP *stats.Counter // ejections_enforced_failure_percentage
	// Local-origin statistical variants (LOGIC DEFERRED — registered for surface
	// parity, emit 0; SPEC §2). (ADR-0247)
	ejectionsDetectedLOSR *stats.Counter // ejections_detected_local_origin_success_rate
	ejectionsEnforcedLOSR *stats.Counter // ejections_enforced_local_origin_success_rate
	ejectionsDetectedLOFP *stats.Counter // ejections_detected_local_origin_failure_percentage
	ejectionsEnforcedLOFP *stats.Counter // ejections_enforced_local_origin_failure_percentage
}

// hostWindow is one host's snapshotted interval window (the sweep's per-host row).
type hostWindow struct {
	ep      Endpoint
	h       *hostHealth
	total   uint64
	success uint64
}

// sweep is one per-interval aggregation pass (SPEC §3.3): snapshot+reset every
// host's window (atomic Swap-to-0), then run the success_rate then the
// failure_percentage detector over the same snapshot. tryEject's CAS makes a host
// ejectable AT MOST ONCE per sweep (AMEND-OD3-3 — if both detectors flag one host,
// the second tryEject no-ops, so ejections_active counts one ejection per host).
func (d *outlierDetector) sweep() {
	snap := make([]hostWindow, 0, len(d.endpoints))
	for _, ep := range d.endpoints {
		h, ok := d.health.states[ep.Addr()]
		if !ok {
			continue
		}
		_ = d.health.isEjected(ep) // lazy un-eject refresh before the snapshot (SPEC §3.3 step 4)
		snap = append(snap, hostWindow{
			ep:      ep,
			h:       h,
			total:   h.intervalTotal.Swap(0),
			success: h.intervalSuccess.Swap(0),
		})
	}
	d.evalSuccessRate(snap)
	d.evalFailurePercentage(snap)
}

// run is the background sweep loop: tick every interval until ctx is done. Unlike
// healthChecker.run it does NOT sweep immediately at t=0 (no data has accrued yet).
func (d *outlierDetector) run(ctx context.Context) {
	t := time.NewTicker(d.cfg.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.sweep()
		}
	}
}

// evalSuccessRate runs the success_rate detector over the snapshot (SPEC §3.3 step 2):
// collect hosts with total >= success_rate_request_volume (the eligible set); if
// >= success_rate_minimum_hosts eligible, compute the cross-host mean success-rate +
// the POPULATION stddev (divisor N), threshold = mean - (stdev_factor/1000)*stddev;
// eject each eligible host with rate < threshold. A non-positive threshold is a
// benign no-op (no rate in [0,1] is below it). (ADR-0247)
func (d *outlierDetector) evalSuccessRate(snap []hostWindow) {
	var eligible []hostWindow
	for _, w := range snap {
		if w.total == 0 { // zero-traffic host: no measured rate (reference excludes it; avoids NaN poisoning the mean)
			continue
		}
		if w.total >= uint64(d.cfg.successRateReqVolume) {
			eligible = append(eligible, w)
		}
	}
	if len(eligible) < int(d.cfg.successRateMinHosts) {
		return
	}
	rates := make([]float64, len(eligible))
	var sum float64
	for i, w := range eligible {
		rates[i] = float64(w.success) / float64(w.total)
		sum += rates[i]
	}
	mean := sum / float64(len(eligible))
	var variance float64
	for _, r := range rates {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(len(eligible)) // POPULATION stddev (divisor N, not N-1)
	threshold := mean - (float64(d.cfg.successRateStdevFactor)/1000.0)*math.Sqrt(variance)
	if threshold <= 0 {
		return // no success rate in [0,1] can be below a non-positive threshold (AMEND-OD3-5)
	}
	for i, w := range eligible {
		if rates[i] < threshold {
			if d.ejectionsDetectedSR != nil {
				d.ejectionsDetectedSR.Inc()
			}
			d.tryEject(w.ep, w.h, d.cfg.enforcingSuccessRate, d.ejectionsEnforcedSR)
		}
	}
}

// evalFailurePercentage runs the failure_percentage detector over the snapshot
// (SPEC §3.3 step 3): collect hosts with total >= failure_percentage_request_volume;
// if >= failure_percentage_minimum_hosts eligible, eject each whose failure% >=
// failure_percentage_threshold. failure% >= threshold is cross-multiplied to
// (total-success)*100 >= threshold*total (integer-exact; no truncation). (ADR-0247)
func (d *outlierDetector) evalFailurePercentage(snap []hostWindow) {
	var eligible []hostWindow
	for _, w := range snap {
		if w.total == 0 { // zero-traffic host: no measured rate (reference excludes it; avoids 0>=0 spurious eject)
			continue
		}
		if w.total >= uint64(d.cfg.failurePctReqVolume) {
			eligible = append(eligible, w)
		}
	}
	if len(eligible) < int(d.cfg.failurePctMinHosts) {
		return
	}
	for _, w := range eligible {
		failures := w.total - w.success
		if failures*100 >= uint64(d.cfg.failurePctThreshold)*w.total {
			if d.ejectionsDetectedFP != nil {
				d.ejectionsDetectedFP.Inc()
			}
			d.tryEject(w.ep, w.h, d.cfg.enforcingFailurePct, d.ejectionsEnforcedFP)
		}
	}
}

// registerStats allocates the 17 outlier_detection.* handles under prefix
// (`cluster.<name>.`, trailing dot included) and assigns them onto the detector.
// The ejections_active gauge is ALSO assigned onto the shared clusterHealth ch
// so the lazy un-eject in clusterHealth.isEjected decrements the same instance
// the detector increments on eject (Task 8; ADR-0245). Called once per cluster
// at registerClusterMetrics time, pre-Freeze.
func (d *outlierDetector) registerStats(r *stats.Registry, prefix string, ch *clusterHealth) {
	op := prefix + "outlier_detection."
	d.ejectionsActive = r.NewGauge(op + "ejections_active")
	d.ejectionsEnforcedTotal = r.NewCounter(op + "ejections_enforced_total")
	d.ejectionsOverflow = r.NewCounter(op + "ejections_overflow")
	d.ejectionsDetected5xx = r.NewCounter(op + "ejections_detected_consecutive_5xx")
	d.ejectionsEnforced5xx = r.NewCounter(op + "ejections_enforced_consecutive_5xx")
	// +4 gateway/local-origin detector counters (Task 5; ADR-0246) — registered
	// UNCONDITIONALLY on any outlier cluster, matching the reference, which exposes
	// every outlier_detection.* name regardless of which detectors are configured.
	d.ejectionsDetectedGw = r.NewCounter(op + "ejections_detected_consecutive_gateway_failure")
	d.ejectionsEnforcedGw = r.NewCounter(op + "ejections_enforced_consecutive_gateway_failure")
	d.ejectionsDetectedLO = r.NewCounter(op + "ejections_detected_consecutive_local_origin_failure")
	d.ejectionsEnforcedLO = r.NewCounter(op + "ejections_enforced_consecutive_local_origin_failure")
	// +8 statistical detector counters (phase 40.3, ADR-0247) — registered
	// UNCONDITIONALLY on any outlier cluster (the reference exposes every
	// outlier_detection.* name regardless of configured detectors). The 4 external
	// names are driven by the sweep; the 4 local-origin names emit 0 (logic deferred).
	d.ejectionsDetectedSR = r.NewCounter(op + "ejections_detected_success_rate")
	d.ejectionsEnforcedSR = r.NewCounter(op + "ejections_enforced_success_rate")
	d.ejectionsDetectedFP = r.NewCounter(op + "ejections_detected_failure_percentage")
	d.ejectionsEnforcedFP = r.NewCounter(op + "ejections_enforced_failure_percentage")
	d.ejectionsDetectedLOSR = r.NewCounter(op + "ejections_detected_local_origin_success_rate")
	d.ejectionsEnforcedLOSR = r.NewCounter(op + "ejections_enforced_local_origin_success_rate")
	d.ejectionsDetectedLOFP = r.NewCounter(op + "ejections_detected_local_origin_failure_percentage")
	d.ejectionsEnforcedLOFP = r.NewCounter(op + "ejections_enforced_local_origin_failure_percentage")
	if ch != nil {
		ch.ejectionsActive = d.ejectionsActive // same instance (lazy un-eject Dec target)
	}
}

// tryEject runs the enforce-roll + max_ejection_percent cap + CAS once-only
// eject for one threshold crossing. Returns true iff THIS call ejected the host.
// The detected counter is the caller's responsibility (Inc'd before tryEject).
// (ADR-0246; refactor of the 40.1 inline eject — REUSED by all three detectors.)
func (d *outlierDetector) tryEject(ep Endpoint, h *hostHealth, enforcing uint32, enforced *stats.Counter) bool {
	_ = ep // part of the documented per-detector API; the cap denominator uses d.endpoints
	if h.ejected.Load() {
		return false // already ejected (fast-path)
	}
	// enforce roll (D-S40.1-3): short-circuit 0/>=100 so the rng is not consumed
	// in the common case.
	enforce := enforcing >= 100 || (enforcing != 0 && d.enforceRoll() < enforcing)
	if !enforce {
		return false // detect-only
	}
	// max_ejection_percent cap: eject iff (ejected+1)*100/total <= cap. Cross-
	// multiplied to (ejected+1)*100 <= cap*total to avoid Go's truncating integer
	// division shifting the boundary — the reference ejects iff the REAL fraction
	// is within the cap (live pin: 1-of-3 = 33.33% ejects iff cap >= 34; cap 33 =>
	// overflow; the truncating form would wrongly allow cap 33 since 100/3 == 33).
	total := len(d.endpoints)
	if total == 0 || (d.health.ejectedCount(d.endpoints)+1)*100 > int(d.cfg.maxEjectionPct)*total {
		if d.ejectionsOverflow != nil {
			d.ejectionsOverflow.Inc()
		}
		return false
	}
	// eject — CAS so exactly one goroutine wins (the streak check above is not
	// atomic as a unit).
	if !h.ejected.CompareAndSwap(false, true) {
		return false
	}
	h.unejectAtNanos.Store(d.health.nowNanos() + d.cfg.baseEjectionTime.Nanoseconds())
	if d.ejectionsActive != nil {
		d.ejectionsActive.Inc()
	}
	if d.ejectionsEnforcedTotal != nil {
		d.ejectionsEnforcedTotal.Inc() // the cross-detector double-count
	}
	if enforced != nil {
		enforced.Inc() // the per-detector enforced counter
	}
	return true
}

// record applies one upstream result for ep (SPEC §3.3). Per-host atomics; the
// cap read is a racy best-effort snapshot; the CAS makes the eject exactly-once.
func (d *outlierDetector) record(ep Endpoint, statusCode int, localOriginErr bool) {
	h, ok := d.health.states[ep.Addr()]
	if !ok {
		return
	}
	_ = d.health.isEjected(ep) // fast-path lazy-uneject refresh

	// Windowed per-host accumulation for the statistical sweep (ADR-0247). Counts on
	// EVERY record path (local-origin, 5xx, and non-5xx). success = NOT a local-origin
	// failure AND NOT a 5xx external status. The sweep Swap-resets these each interval.
	h.intervalTotal.Add(1)
	if !localOriginErr && (statusCode < 500 || statusCode >= 600) {
		h.intervalSuccess.Add(1)
	}

	if localOriginErr {
		d.recordLocalOrigin(ep, h, statusCode) // split-aware local-origin detector (gateway-class mapping when split=false)
		return
	}
	// external HTTP status
	if d.cfg.splitLocalOrigin {
		h.consecLO.Store(0) // a completed external response ⇒ connection succeeded ⇒ reset LO streak
	}
	if statusCode < 500 || statusCode >= 600 {
		h.consec5xx.Store(0)
		h.consecGw.Store(0)
		return
	}
	d.recordExternal5xx(ep, h, statusCode)
}

// bumpStreak is the shared consecutive-detector body (gateway / consecutive_5xx
// / local-origin arms): streak.Add(1), threshold-compare, Inc the detected
// counter (nil-guarded), then tryEject under the arm's enforce-roll %. Returns
// true iff THIS call ejected the host. A disabled detector neither bumps the
// streak nor ejects. Stat ordering (detected before enforced) is preserved from
// the three inlined copies this consolidates.
func (d *outlierDetector) bumpStreak(ep Endpoint, h *hostHealth, streak *atomic.Uint32, enabled bool, threshold uint32, detected, enforced *stats.Counter, enforcing uint32) bool {
	if !enabled {
		return false
	}
	if streak.Add(1) < threshold {
		return false
	}
	if detected != nil {
		detected.Inc()
	}
	return d.tryEject(ep, h, enforcing, enforced)
}

// recordExternal5xx applies one external 5xx, gateway detector FIRST (AMEND-OD2-2):
// a gateway eject short-circuits before the consecutive_5xx detector fires, so a
// gateway-driven eject leaves detected_consecutive_5xx at 0 for that call.
func (d *outlierDetector) recordExternal5xx(ep Endpoint, h *hostHealth, statusCode int) {
	gateway := statusCode == 502 || statusCode == 503 || statusCode == 504
	if gateway {
		if d.bumpStreak(ep, h, &h.consecGw, d.cfg.consecGwEnabled, d.cfg.consecutiveGw, d.ejectionsDetectedGw, d.ejectionsEnforcedGw, d.cfg.enforcingGw) {
			return // gateway ejected → the 5xx detector does NOT fire this call (detected_5xx stays 0)
		}
	} else {
		h.consecGw.Store(0) // a non-gateway 5xx breaks the gateway streak
	}
	// fall through to the 5xx detector (the 40.1 path, behavior-unchanged).
	d.bumpStreak(ep, h, &h.consec5xx, d.cfg.consec5xxEnabled, d.cfg.consecutive5xx, d.ejectionsDetected5xx, d.ejectionsEnforced5xx, d.cfg.enforcing5xx)
}

// recordLocalOrigin applies one local-origin failure (connect/reset). When split
// is enabled it feeds ONLY the local-origin detector; when split is disabled the
// failure is mapped to a gateway-class 5xx (the caller passes the 502/503
// local-reply code, both gateway-class) and fed to the gateway/5xx detectors.
func (d *outlierDetector) recordLocalOrigin(ep Endpoint, h *hostHealth, statusCode int) {
	if !d.cfg.splitLocalOrigin {
		d.recordExternal5xx(ep, h, statusCode) // split=false: count as gateway-class 5xx (AMEND-OD2-3)
		return
	}
	d.bumpStreak(ep, h, &h.consecLO, d.cfg.consecLOEnabled, d.cfg.consecutiveLO, d.ejectionsDetectedLO, d.ejectionsEnforcedLO, d.cfg.enforcingLO)
}
