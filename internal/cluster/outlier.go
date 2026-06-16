package cluster

import (
	"fmt"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"

	"github.com/esalaine/envoy-go/internal/stats"
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
	// interval is parse-accepted + validated (gt:0s) but its role is deferred at 40.1.
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
}

// registerStats allocates the 5 outlier_detection.* handles under prefix
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
	if ch != nil {
		ch.ejectionsActive = d.ejectionsActive // same instance (lazy un-eject Dec target)
	}
}

// record applies one upstream result for ep (SPEC §3.3). Per-host atomics; the
// cap read is a racy best-effort snapshot; the CAS makes the eject exactly-once.
func (d *outlierDetector) record(ep Endpoint, statusCode int) {
	h, ok := d.health.states[ep.Addr()]
	if !ok {
		return
	}
	_ = d.health.isEjected(ep) // fast-path lazy-uneject refresh
	is5xx := statusCode >= 500 && statusCode < 600
	if !is5xx {
		h.consec5xx.Store(0) // reset on any non-5xx FROM THIS HOST
		return
	}
	if !d.cfg.consec5xxEnabled {
		return
	}
	n := h.consec5xx.Add(1)
	if n < d.cfg.consecutive5xx {
		return
	}
	// threshold reached
	if d.ejectionsDetected5xx != nil {
		d.ejectionsDetected5xx.Inc()
	}
	if h.ejected.Load() {
		return // already ejected
	}
	// enforce roll (D-S40.1-3): short-circuit 0/>=100 so the rng is not consumed
	// in the common case.
	enforce := d.cfg.enforcing5xx >= 100 || (d.cfg.enforcing5xx != 0 && d.enforceRoll() < d.cfg.enforcing5xx)
	if !enforce {
		return // detect-only
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
		return
	}
	// eject — CAS so exactly one goroutine wins (the streak check above is not
	// atomic as a unit).
	if !h.ejected.CompareAndSwap(false, true) {
		return
	}
	h.unejectAtNanos.Store(d.health.nowNanos() + d.cfg.baseEjectionTime.Nanoseconds())
	h.ejectCount.Add(1)
	if d.ejectionsActive != nil {
		d.ejectionsActive.Inc()
	}
	if d.ejectionsEnforcedTotal != nil {
		d.ejectionsEnforcedTotal.Inc() // the double-count
	}
	if d.ejectionsEnforced5xx != nil {
		d.ejectionsEnforced5xx.Inc() // (AMEND-OD4)
	}
}
