package cluster

import (
	"sync"
	"testing"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestParseOutlierDetection_Absent(t *testing.T) {
	c := &clusterv3.Cluster{}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config for cluster with no outlier_detection, got %+v", cfg)
	}
}

func TestParseOutlierDetection_Full(t *testing.T) {
	c := &clusterv3.Cluster{
		OutlierDetection: &clusterv3.OutlierDetection{
			Consecutive_5Xx:          wrapperspb.UInt32(7),
			BaseEjectionTime:         durationpb.New(45 * time.Second),
			MaxEjectionPercent:       wrapperspb.UInt32(25),
			EnforcingConsecutive_5Xx: wrapperspb.UInt32(80),
			Interval:                 durationpb.New(5 * time.Second),
		},
	}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if !cfg.consec5xxEnabled {
		t.Errorf("consec5xxEnabled = false, want true")
	}
	if cfg.consecutive5xx != 7 {
		t.Errorf("consecutive5xx = %d, want 7", cfg.consecutive5xx)
	}
	if cfg.baseEjectionTime != 45*time.Second {
		t.Errorf("baseEjectionTime = %v, want 45s", cfg.baseEjectionTime)
	}
	if cfg.maxEjectionPct != 25 {
		t.Errorf("maxEjectionPct = %d, want 25", cfg.maxEjectionPct)
	}
	if cfg.enforcing5xx != 80 {
		t.Errorf("enforcing5xx = %d, want 80", cfg.enforcing5xx)
	}
}

func TestParseOutlierDetection_Defaults(t *testing.T) {
	c := &clusterv3.Cluster{OutlierDetection: &clusterv3.OutlierDetection{}}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for empty outlier_detection")
	}
	if cfg.consecutive5xx != 5 {
		t.Errorf("consecutive5xx = %d, want 5", cfg.consecutive5xx)
	}
	if !cfg.consec5xxEnabled {
		t.Errorf("consec5xxEnabled = false, want true (absent ⇒ default 5 enabled)")
	}
	if cfg.baseEjectionTime != 30*time.Second {
		t.Errorf("baseEjectionTime = %v, want 30s", cfg.baseEjectionTime)
	}
	if cfg.maxEjectionPct != 10 {
		t.Errorf("maxEjectionPct = %d, want 10", cfg.maxEjectionPct)
	}
	if cfg.enforcing5xx != 100 {
		t.Errorf("enforcing5xx = %d, want 100", cfg.enforcing5xx)
	}
}

func TestParseOutlierDetection_ExplicitZeroDisables(t *testing.T) {
	c := &clusterv3.Cluster{
		OutlierDetection: &clusterv3.OutlierDetection{
			Consecutive_5Xx: wrapperspb.UInt32(0),
		},
	}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.consec5xxEnabled {
		t.Errorf("consec5xxEnabled = true, want false (explicit 0 ⇒ detector OFF)")
	}
	if cfg.consecutive5xx != 0 {
		t.Errorf("consecutive5xx = %d, want 0", cfg.consecutive5xx)
	}
}

func TestParseOutlierDetection_Rejects(t *testing.T) {
	cases := []struct {
		name string
		od   *clusterv3.OutlierDetection
		want string
	}{
		{
			name: "max_ejection_percent>100",
			od:   &clusterv3.OutlierDetection{MaxEjectionPercent: wrapperspb.UInt32(101)},
			want: `cluster: "c": outlier_detection: max_ejection_percent: value must be less than or equal to 100`,
		},
		{
			name: "enforcing_consecutive_5xx>100",
			od:   &clusterv3.OutlierDetection{EnforcingConsecutive_5Xx: wrapperspb.UInt32(101)},
			want: `cluster: "c": outlier_detection: enforcing_consecutive_5xx: value must be less than or equal to 100`,
		},
		{
			name: "interval<=0",
			od:   &clusterv3.OutlierDetection{Interval: durationpb.New(0)},
			want: `cluster: "c": outlier_detection: interval: value must be greater than 0s`,
		},
		{
			name: "base_ejection_time<=0",
			od:   &clusterv3.OutlierDetection{BaseEjectionTime: durationpb.New(0)},
			want: `cluster: "c": outlier_detection: base_ejection_time: value must be greater than 0s`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOutlierDetection(&clusterv3.Cluster{OutlierDetection: tc.od}, "c")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != tc.want {
				t.Errorf("error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

// --- Task 2: gateway / local-origin / split parse fields ---

func TestParseOutlierDetection_GatewayLocalOriginSplit_Full(t *testing.T) {
	c := &clusterv3.Cluster{
		OutlierDetection: &clusterv3.OutlierDetection{
			ConsecutiveGatewayFailure:              wrapperspb.UInt32(7),
			EnforcingConsecutiveGatewayFailure:     wrapperspb.UInt32(80),
			SplitExternalLocalOriginErrors:         true,
			ConsecutiveLocalOriginFailure:          wrapperspb.UInt32(9),
			EnforcingConsecutiveLocalOriginFailure: wrapperspb.UInt32(60),
		},
	}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if !cfg.consecGwEnabled {
		t.Errorf("consecGwEnabled = false, want true")
	}
	if cfg.consecutiveGw != 7 {
		t.Errorf("consecutiveGw = %d, want 7", cfg.consecutiveGw)
	}
	if cfg.enforcingGw != 80 {
		t.Errorf("enforcingGw = %d, want 80", cfg.enforcingGw)
	}
	if !cfg.splitLocalOrigin {
		t.Errorf("splitLocalOrigin = false, want true")
	}
	if !cfg.consecLOEnabled {
		t.Errorf("consecLOEnabled = false, want true")
	}
	if cfg.consecutiveLO != 9 {
		t.Errorf("consecutiveLO = %d, want 9", cfg.consecutiveLO)
	}
	if cfg.enforcingLO != 60 {
		t.Errorf("enforcingLO = %d, want 60", cfg.enforcingLO)
	}
}

func TestParseOutlierDetection_GatewayLocalOrigin_Defaults(t *testing.T) {
	c := &clusterv3.Cluster{OutlierDetection: &clusterv3.OutlierDetection{}}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for empty outlier_detection")
	}
	// gateway: absent ⇒ threshold 5 + enabled + enforcing default 0.
	if cfg.consecutiveGw != 5 {
		t.Errorf("consecutiveGw = %d, want 5", cfg.consecutiveGw)
	}
	if !cfg.consecGwEnabled {
		t.Errorf("consecGwEnabled = false, want true (absent ⇒ default 5 enabled)")
	}
	if cfg.enforcingGw != 0 {
		t.Errorf("enforcingGw = %d, want 0 (default detect-only)", cfg.enforcingGw)
	}
	// split: absent ⇒ false.
	if cfg.splitLocalOrigin {
		t.Errorf("splitLocalOrigin = true, want false (default)")
	}
	// local-origin: absent ⇒ threshold 5 + enabled + enforcing default 100.
	if cfg.consecutiveLO != 5 {
		t.Errorf("consecutiveLO = %d, want 5", cfg.consecutiveLO)
	}
	if !cfg.consecLOEnabled {
		t.Errorf("consecLOEnabled = false, want true (absent ⇒ default 5 enabled)")
	}
	if cfg.enforcingLO != 100 {
		t.Errorf("enforcingLO = %d, want 100 (default)", cfg.enforcingLO)
	}
}

func TestParseOutlierDetection_GatewayExplicitZeroDisables(t *testing.T) {
	c := &clusterv3.Cluster{
		OutlierDetection: &clusterv3.OutlierDetection{
			ConsecutiveGatewayFailure: wrapperspb.UInt32(0),
		},
	}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.consecGwEnabled {
		t.Errorf("consecGwEnabled = true, want false (explicit 0 ⇒ detector OFF)")
	}
	if cfg.consecutiveGw != 0 {
		t.Errorf("consecutiveGw = %d, want 0", cfg.consecutiveGw)
	}
}

func TestParseOutlierDetection_LocalOriginExplicitZeroDisables(t *testing.T) {
	c := &clusterv3.Cluster{
		OutlierDetection: &clusterv3.OutlierDetection{
			ConsecutiveLocalOriginFailure: wrapperspb.UInt32(0),
		},
	}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.consecLOEnabled {
		t.Errorf("consecLOEnabled = true, want false (explicit 0 ⇒ detector OFF)")
	}
	if cfg.consecutiveLO != 0 {
		t.Errorf("consecutiveLO = %d, want 0", cfg.consecutiveLO)
	}
}

func TestParseOutlierDetection_SplitFlag(t *testing.T) {
	c := &clusterv3.Cluster{
		OutlierDetection: &clusterv3.OutlierDetection{
			SplitExternalLocalOriginErrors: true,
		},
	}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if !cfg.splitLocalOrigin {
		t.Errorf("splitLocalOrigin = false, want true")
	}
}

func TestParseOutlierDetection_GatewayLocalOriginRejects(t *testing.T) {
	cases := []struct {
		name string
		od   *clusterv3.OutlierDetection
		want string
	}{
		{
			name: "enforcing_consecutive_gateway_failure>100",
			od:   &clusterv3.OutlierDetection{EnforcingConsecutiveGatewayFailure: wrapperspb.UInt32(101)},
			want: `cluster: "c": outlier_detection: enforcing_consecutive_gateway_failure: value must be less than or equal to 100`,
		},
		{
			name: "enforcing_consecutive_local_origin_failure>100",
			od:   &clusterv3.OutlierDetection{EnforcingConsecutiveLocalOriginFailure: wrapperspb.UInt32(101)},
			want: `cluster: "c": outlier_detection: enforcing_consecutive_local_origin_failure: value must be less than or equal to 100`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOutlierDetection(&clusterv3.Cluster{OutlierDetection: tc.od}, "c")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != tc.want {
				t.Errorf("error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

// --- outlierDetector unit tests (Task 5) ---

// mkOutlierEndpoints builds n endpoints 127.0.0.1:9000..9000+n-1.
func mkOutlierEndpoints(n int) []Endpoint {
	eps := make([]Endpoint, n)
	for i := 0; i < n; i++ {
		eps[i] = Endpoint{Host: "127.0.0.1", Port: uint32(9000 + i)}
	}
	return eps
}

// detectorFixture builds a deterministic detector over eps with cfg, a fixed
// clock at t=0, real (non-nil) stat handles from a fresh registry, and the given
// enforceRoll stub.
type detectorFixture struct {
	d   *outlierDetector
	ch  *clusterHealth
	now *int64
}

func newDetectorFixture(cfg outlierConfig, eps []Endpoint, enforceRoll func() uint32) detectorFixture {
	var now int64
	ch := newClusterHealth(eps, 0.5)
	ch.nowNanos = func() int64 { return now }
	reg := stats.NewRegistry()
	ch.ejectionsActive = reg.NewGauge("outlier_detection.ejections_active")
	d := &outlierDetector{
		cfg:                    cfg,
		health:                 ch,
		endpoints:              eps,
		enforceRoll:            enforceRoll,
		ejectionsActive:        ch.ejectionsActive,
		ejectionsEnforcedTotal: reg.NewCounter("outlier_detection.ejections_enforced_total"),
		ejectionsOverflow:      reg.NewCounter("outlier_detection.ejections_overflow"),
		ejectionsDetected5xx:   reg.NewCounter("outlier_detection.ejections_detected_consecutive_5xx"),
		ejectionsEnforced5xx:   reg.NewCounter("outlier_detection.ejections_enforced_consecutive_5xx"),
	}
	return detectorFixture{d: d, ch: ch, now: &now}
}

// cfgConsec5xx builds a cfg with the given threshold enabled, enforcing=100,
// maxEjectionPct=100, base=30s.
func cfgConsec5xx(threshold uint32) outlierConfig {
	return outlierConfig{
		consec5xxEnabled: true,
		consecutive5xx:   threshold,
		baseEjectionTime: 30 * time.Second,
		maxEjectionPct:   100,
		enforcing5xx:     100,
	}
}

func panicRoll() uint32 { panic("enforceRoll consumed despite short-circuit") }

func TestOutlierDetector_EjectsAfterNConsecutive5xx(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	f := newDetectorFixture(cfgConsec5xx(5), eps, panicRoll) // enforcing=100 short-circuits
	ep := eps[0]
	for i := 0; i < 4; i++ {
		f.d.record(ep, 503, false)
		if !f.ch.available(ep) {
			t.Fatalf("ejected after %d 5xx, want eject only on the 5th", i+1)
		}
	}
	f.d.record(ep, 503, false) // the 5th -> eject
	if f.ch.available(ep) {
		t.Fatal("host still available after 5 consecutive 5xx; want ejected")
	}
	if got := f.d.ejectionsActive.Load(); got != 1 {
		t.Errorf("ejections_active = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 1 {
		t.Errorf("ejections_enforced_total = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforced5xx.Load(); got != 1 {
		t.Errorf("ejections_enforced_consecutive_5xx = %d, want 1", got)
	}
	if got := f.d.ejectionsDetected5xx.Load(); got != 1 {
		t.Errorf("ejections_detected_consecutive_5xx = %d, want 1 (the double-count)", got)
	}
}

func TestOutlierDetector_2xxMidStreakResets(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	f := newDetectorFixture(cfgConsec5xx(5), eps, panicRoll)
	ep := eps[0]
	for i := 0; i < 4; i++ {
		f.d.record(ep, 500, false)
	}
	f.d.record(ep, 200, false) // reset
	for i := 0; i < 4; i++ {
		f.d.record(ep, 500, false)
	}
	if !f.ch.available(ep) {
		t.Fatal("host ejected despite a 2xx breaking the streak")
	}
	if got := f.d.ejectionsActive.Load(); got != 0 {
		t.Errorf("ejections_active = %d, want 0", got)
	}
}

func TestOutlierDetector_Consec5xxDisabledNeverEjects(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	cfg := outlierConfig{
		consec5xxEnabled: false,
		consecutive5xx:   0,
		baseEjectionTime: 30 * time.Second,
		maxEjectionPct:   100,
		enforcing5xx:     100,
	}
	f := newDetectorFixture(cfg, eps, panicRoll)
	ep := eps[0]
	for i := 0; i < 50; i++ {
		f.d.record(ep, 503, false)
	}
	if !f.ch.available(ep) {
		t.Fatal("host ejected despite consec5xx detector disabled")
	}
	if got := f.d.ejectionsDetected5xx.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_5xx = %d, want 0 (disabled)", got)
	}
}

func TestOutlierDetector_CapOverflowBlocksThenAllows(t *testing.T) {
	// 3 endpoints, cap=33: the REAL fraction for the first eject is 1/3 = 33.33%,
	// which EXCEEDS cap 33 => overflow (the live-pinned reference boundary: 1-of-3
	// ejects iff cap >= 34). The cross-multiplied production form (ejected+1)*100 >
	// cap*total => 100 > 99 => blocked, matching the reference (the truncating
	// 100/3==33 form would have wrongly allowed cap 33).
	eps := mkOutlierEndpoints(3)
	cfg := cfgConsec5xx(3)
	cfg.maxEjectionPct = 33
	f := newDetectorFixture(cfg, eps, panicRoll)
	ep := eps[0]
	for i := 0; i < 3; i++ {
		f.d.record(ep, 503, false)
	}
	if !f.ch.available(ep) {
		t.Fatal("host ejected despite cap overflow (1/3 = 33.33% > cap 33)")
	}
	if got := f.d.ejectionsOverflow.Load(); got != 1 {
		t.Errorf("ejections_overflow = %d, want 1", got)
	}
	if got := f.d.ejectionsDetected5xx.Load(); got != 1 {
		t.Errorf("ejections_detected_consecutive_5xx = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 0 {
		t.Errorf("ejections_enforced_total = %d, want 0 (blocked)", got)
	}

	// cap=34 -> 1/3 = 33.33% <= 34% -> ejects ((0+1)*100=100 <= 34*3=102).
	cfg2 := cfgConsec5xx(3)
	cfg2.maxEjectionPct = 34
	f2 := newDetectorFixture(cfg2, eps, panicRoll)
	for i := 0; i < 3; i++ {
		f2.d.record(eps[0], 503, false)
	}
	if f2.ch.available(eps[0]) {
		t.Fatal("host not ejected with cap=34 (33 <= 34)")
	}
	if got := f2.d.ejectionsActive.Load(); got != 1 {
		t.Errorf("ejections_active = %d, want 1", got)
	}
}

func TestOutlierDetector_EnforceRoll(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	ep := eps[0]

	// enforcing=50, roll=49 (<50) -> enforce (eject).
	{
		cfg := cfgConsec5xx(3)
		cfg.enforcing5xx = 50
		f := newDetectorFixture(cfg, eps, func() uint32 { return 49 })
		for i := 0; i < 3; i++ {
			f.d.record(ep, 503, false)
		}
		if f.ch.available(ep) {
			t.Fatal("roll=49 < enforcing=50 should enforce (eject)")
		}
		if got := f.d.ejectionsEnforced5xx.Load(); got != 1 {
			t.Errorf("ejections_enforced_consecutive_5xx = %d, want 1", got)
		}
	}

	// enforcing=50, roll=50 (>=50) -> detect-only.
	{
		cfg := cfgConsec5xx(3)
		cfg.enforcing5xx = 50
		f := newDetectorFixture(cfg, eps, func() uint32 { return 50 })
		for i := 0; i < 3; i++ {
			f.d.record(ep, 503, false)
		}
		if !f.ch.available(ep) {
			t.Fatal("roll=50 >= enforcing=50 should be detect-only (not ejected)")
		}
		if got := f.d.ejectionsDetected5xx.Load(); got != 1 {
			t.Errorf("ejections_detected_consecutive_5xx = %d, want 1", got)
		}
		if got := f.d.ejectionsEnforcedTotal.Load(); got != 0 {
			t.Errorf("ejections_enforced_total = %d, want 0 (detect-only)", got)
		}
		if got := f.d.ejectionsEnforced5xx.Load(); got != 0 {
			t.Errorf("ejections_enforced_consecutive_5xx = %d, want 0 (detect-only)", got)
		}
	}

	// enforcing=0 -> never enforce (roll not consumed).
	{
		cfg := cfgConsec5xx(3)
		cfg.enforcing5xx = 0
		f := newDetectorFixture(cfg, eps, panicRoll)
		for i := 0; i < 3; i++ {
			f.d.record(ep, 503, false)
		}
		if !f.ch.available(ep) {
			t.Fatal("enforcing=0 should never eject")
		}
		if got := f.d.ejectionsDetected5xx.Load(); got != 1 {
			t.Errorf("ejections_detected_consecutive_5xx = %d, want 1", got)
		}
	}

	// enforcing=100 -> always enforce without consuming the roll (panicRoll proves it).
	{
		cfg := cfgConsec5xx(3)
		cfg.enforcing5xx = 100
		f := newDetectorFixture(cfg, eps, panicRoll)
		for i := 0; i < 3; i++ {
			f.d.record(ep, 503, false)
		}
		if f.ch.available(ep) {
			t.Fatal("enforcing=100 should always eject")
		}
	}
}

func TestOutlierDetector_AlreadyEjected(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	f := newDetectorFixture(cfgConsec5xx(3), eps, panicRoll)
	ep := eps[0]
	for i := 0; i < 3; i++ {
		f.d.record(ep, 503, false)
	}
	if f.ch.available(ep) {
		t.Fatal("host not ejected after 3 5xx")
	}
	// active/enforced snapshot after eject.
	if got := f.d.ejectionsActive.Load(); got != 1 {
		t.Fatalf("ejections_active = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 1 {
		t.Fatalf("ejections_enforced_total = %d, want 1", got)
	}
	detectedBefore := f.d.ejectionsDetected5xx.Load()

	// Further 5xx: re-hits the threshold (detected re-increments per the sketch),
	// but the h.ejected.Load() guard returns before re-eject -> no new enforced.
	f.d.record(ep, 503, false)
	if got := f.d.ejectionsActive.Load(); got != 1 {
		t.Errorf("ejections_active = %d after further 5xx, want still 1 (no re-eject)", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 1 {
		t.Errorf("ejections_enforced_total = %d after further 5xx, want still 1", got)
	}
	if got := f.d.ejectionsDetected5xx.Load(); got != detectedBefore+1 {
		t.Errorf("ejections_detected_consecutive_5xx = %d, want %d (detected re-increments on re-hit)", got, detectedBefore+1)
	}
}

func TestOutlierDetector_ConcurrentEjectExactlyOnce(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	f := newDetectorFixture(cfgConsec5xx(3), eps, panicRoll)
	ep := eps[0]
	var wg sync.WaitGroup
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				f.d.record(ep, 503, false)
			}
		}()
	}
	wg.Wait()
	if f.ch.available(ep) {
		t.Fatal("host not ejected after concurrent 5xx")
	}
	if got := f.d.ejectionsActive.Load(); got != 1 {
		t.Errorf("ejections_active = %d, want exactly 1 (CAS exactly-once)", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 1 {
		t.Errorf("ejections_enforced_total = %d, want exactly 1", got)
	}
}

// --- Task 3: gateway detector (gateway-first ordering) ---

// cfgGateway builds a cfg with the gateway detector enabled at gwThreshold
// (enforcing enfGw) and the consecutive_5xx detector enabled at fiveXxThreshold
// (enforcing 100). maxEjectionPct=100, base=30s.
func cfgGateway(gwThreshold, enfGw, fiveXxThreshold uint32) outlierConfig {
	return outlierConfig{
		consec5xxEnabled: true,
		consecutive5xx:   fiveXxThreshold,
		baseEjectionTime: 30 * time.Second,
		maxEjectionPct:   100,
		enforcing5xx:     100,
		consecGwEnabled:  true,
		consecutiveGw:    gwThreshold,
		enforcingGw:      enfGw,
	}
}

// withGatewayHandles assigns the +2 gateway stat handles onto the detector from
// a fresh registry (Task 3 adds them to the struct; Task 5 allocates them in
// registerStats — so unit tests inject them directly to observe the counts).
func (f detectorFixture) withGatewayHandles() detectorFixture {
	reg := stats.NewRegistry()
	f.d.ejectionsDetectedGw = reg.NewCounter("outlier_detection.ejections_detected_consecutive_gateway_failure")
	f.d.ejectionsEnforcedGw = reg.NewCounter("outlier_detection.ejections_enforced_consecutive_gateway_failure")
	return f
}

// TestOutlierDetector_GatewayEjectsFirst: N consecutive 503 with
// enforcing_consecutive_gateway_failure=100 ejects via the gateway detector and
// bumps the gateway counters + the cross-detector double-count, AND leaves
// detected_5xx == 0 (the gateway-first short-circuit; both thresholds equal).
func TestOutlierDetector_GatewayEjectsFirst(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	f := newDetectorFixture(cfgGateway(3, 100, 3), eps, panicRoll).withGatewayHandles()
	ep := eps[0]
	for i := 0; i < 3; i++ {
		f.d.record(ep, 503, false)
	}
	if f.ch.available(ep) {
		t.Fatal("host not ejected after 3 consecutive 503 (gateway detector)")
	}
	if got := f.d.ejectionsDetectedGw.Load(); got != 1 {
		t.Errorf("ejections_detected_consecutive_gateway_failure = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedGw.Load(); got != 1 {
		t.Errorf("ejections_enforced_consecutive_gateway_failure = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 1 {
		t.Errorf("ejections_enforced_total = %d, want 1 (double-count)", got)
	}
	if got := f.d.ejectionsActive.Load(); got != 1 {
		t.Errorf("ejections_active = %d, want 1", got)
	}
	// The load-bearing invariant: gateway-first short-circuit ⇒ the 5xx detector
	// never fires this call.
	if got := f.d.ejectionsDetected5xx.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_5xx = %d, want 0 (gateway-first short-circuit)", got)
	}
	if got := f.d.ejectionsEnforced5xx.Load(); got != 0 {
		t.Errorf("ejections_enforced_consecutive_5xx = %d, want 0 (gateway-first short-circuit)", got)
	}
}

// TestOutlierDetector_GatewayDetectOnlyFallsThroughTo5xx: gateway detect-only
// (enforcing_gateway=0) bumps detected_gateway then falls through to the 5xx
// detector, which ejects (the 0069 behavior).
func TestOutlierDetector_GatewayDetectOnlyFallsThroughTo5xx(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	// gateway enforcing=0 (detect-only); 5xx threshold 3 enforcing 100.
	f := newDetectorFixture(cfgGateway(3, 0, 3), eps, panicRoll).withGatewayHandles()
	ep := eps[0]
	for i := 0; i < 3; i++ {
		f.d.record(ep, 503, false)
	}
	if f.ch.available(ep) {
		t.Fatal("host not ejected after 3 consecutive 503 (5xx fall-through)")
	}
	if got := f.d.ejectionsDetectedGw.Load(); got != 1 {
		t.Errorf("ejections_detected_consecutive_gateway_failure = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedGw.Load(); got != 0 {
		t.Errorf("ejections_enforced_consecutive_gateway_failure = %d, want 0 (detect-only)", got)
	}
	// fell through: the 5xx detector detected AND enforced the eject.
	if got := f.d.ejectionsDetected5xx.Load(); got != 1 {
		t.Errorf("ejections_detected_consecutive_5xx = %d, want 1 (fall-through)", got)
	}
	if got := f.d.ejectionsEnforced5xx.Load(); got != 1 {
		t.Errorf("ejections_enforced_consecutive_5xx = %d, want 1 (fall-through eject)", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 1 {
		t.Errorf("ejections_enforced_total = %d, want 1", got)
	}
}

// TestOutlierDetector_NonGateway5xxResetsGwStreak: a 500 (non-gateway 5xx)
// resets consecGw and counts only via consec5xx — never ejects via gateway.
func TestOutlierDetector_NonGateway5xxResetsGwStreak(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	// gateway threshold 3 enforcing 100; 5xx threshold 5 enforcing 100.
	f := newDetectorFixture(cfgGateway(3, 100, 5), eps, panicRoll).withGatewayHandles()
	ep := eps[0]
	for i := 0; i < 4; i++ {
		f.d.record(ep, 500, false) // non-gateway 5xx: only consec5xx accrues
	}
	if !f.ch.available(ep) {
		t.Fatal("host ejected on non-gateway 5xx (gateway must not fire on 500)")
	}
	if got := f.d.ejectionsDetectedGw.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_gateway_failure = %d, want 0 (500 is not gateway)", got)
	}
	if got := f.d.ejectionsDetected5xx.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_5xx = %d, want 0 (only 4 of 5)", got)
	}
	f.d.record(ep, 500, false) // the 5th 500 -> 5xx eject
	if f.ch.available(ep) {
		t.Fatal("host not ejected after 5 consecutive 500 (consec5xx)")
	}
	if got := f.d.ejectionsEnforced5xx.Load(); got != 1 {
		t.Errorf("ejections_enforced_consecutive_5xx = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedGw.Load(); got != 0 {
		t.Errorf("ejections_enforced_consecutive_gateway_failure = %d, want 0", got)
	}
}

// TestOutlierDetector_NonGatewayBreaksGwStreakMidway: 503,503,500 then 503,503
// must NOT gateway-eject at threshold 3 — the 500 resets consecGw.
func TestOutlierDetector_NonGatewayBreaksGwStreakMidway(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	// gateway 3 enforcing 100; 5xx disabled-via-high-threshold (100) so only the
	// gateway detector can eject within this window.
	f := newDetectorFixture(cfgGateway(3, 100, 100), eps, panicRoll).withGatewayHandles()
	ep := eps[0]
	f.d.record(ep, 503, false)
	f.d.record(ep, 503, false)
	f.d.record(ep, 500, false) // breaks the gateway streak (consecGw -> 0)
	f.d.record(ep, 503, false)
	f.d.record(ep, 503, false)
	if !f.ch.available(ep) {
		t.Fatal("host ejected: the 500 should have reset consecGw below threshold 3")
	}
	if got := f.d.ejectionsDetectedGw.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_gateway_failure = %d, want 0 (streak broken)", got)
	}
}

// TestOutlierDetector_2xxResetsBothStreaks: a 2xx resets both consec5xx and
// consecGw.
func TestOutlierDetector_2xxResetsBothStreaks(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	f := newDetectorFixture(cfgGateway(3, 100, 3), eps, panicRoll).withGatewayHandles()
	ep := eps[0]
	f.d.record(ep, 503, false)
	f.d.record(ep, 503, false)
	f.d.record(ep, 200, false) // reset both
	f.d.record(ep, 503, false)
	f.d.record(ep, 503, false)
	if !f.ch.available(ep) {
		t.Fatal("host ejected: the 2xx should have reset both streaks below threshold 3")
	}
	if got := f.d.ejectionsDetectedGw.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_gateway_failure = %d, want 0", got)
	}
	if got := f.d.ejectionsDetected5xx.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_5xx = %d, want 0", got)
	}
}

// TestManager_OutlierOnlyBuildsHealthRegistry asserts a cluster with ONLY
// outlier_detection (no health_checks) now materializes a non-nil clusterHealth
// registry (the Task-4 creation-site widening). Observed via the unexported
// Cluster.health field.
func TestManager_OutlierOnlyBuildsHealthRegistry(t *testing.T) {
	c := mkStaticCluster("od_only", mkLbEndpoint("127.0.0.1", 9001))
	c.OutlierDetection = &clusterv3.OutlierDetection{}

	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(c), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, ok := m.Get("od_only")
	if !ok {
		t.Fatal("od_only not found in manager")
	}
	if cl.health == nil {
		t.Fatal("expected non-nil health registry for outlier-detection-only cluster")
	}
}

// --- Task 4: local-origin detector (split_external_local_origin_errors) ---

// cfgLocalOrigin builds a cfg with split enabled, the local-origin detector
// enabled at loThreshold (enforcing enfLO), and the gateway + 5xx detectors
// enabled at high thresholds (so only the local-origin detector can fire within
// the test windows). maxEjectionPct=100, base=30s.
func cfgLocalOrigin(loThreshold, enfLO uint32) outlierConfig {
	return outlierConfig{
		consec5xxEnabled: true,
		consecutive5xx:   100,
		baseEjectionTime: 30 * time.Second,
		maxEjectionPct:   100,
		enforcing5xx:     100,
		consecGwEnabled:  true,
		consecutiveGw:    100,
		enforcingGw:      100,
		splitLocalOrigin: true,
		consecLOEnabled:  true,
		consecutiveLO:    loThreshold,
		enforcingLO:      enfLO,
	}
}

// withLocalOriginHandles assigns the +2 local-origin stat handles onto the
// detector from a fresh registry (Task 4 adds them to the struct; Task 5
// allocates them in registerStats — so unit tests inject them directly).
func (f detectorFixture) withLocalOriginHandles() detectorFixture {
	reg := stats.NewRegistry()
	f.d.ejectionsDetectedLO = reg.NewCounter("outlier_detection.ejections_detected_consecutive_local_origin_failure")
	f.d.ejectionsEnforcedLO = reg.NewCounter("outlier_detection.ejections_enforced_consecutive_local_origin_failure")
	return f
}

// TestOutlierDetector_LocalOriginEjectsViaLODetector: split=true, N consecutive
// local-origin failures (record(ep, 502, true)) eject via the local-origin
// detector and bump the LO counters + the cross-detector double-count, AND leave
// the external detectors (5xx + gateway) at 0 (split routes local-origin away).
func TestOutlierDetector_LocalOriginEjectsViaLODetector(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	f := newDetectorFixture(cfgLocalOrigin(3, 100), eps, panicRoll).withGatewayHandles().withLocalOriginHandles()
	ep := eps[0]
	for i := 0; i < 2; i++ {
		f.d.record(ep, 502, true)
		if !f.ch.available(ep) {
			t.Fatalf("ejected after %d local-origin failures, want eject only on the 3rd", i+1)
		}
	}
	f.d.record(ep, 502, true) // the 3rd -> eject
	if f.ch.available(ep) {
		t.Fatal("host still available after 3 consecutive local-origin failures; want ejected")
	}
	if got := f.d.ejectionsDetectedLO.Load(); got != 1 {
		t.Errorf("ejections_detected_consecutive_local_origin_failure = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedLO.Load(); got != 1 {
		t.Errorf("ejections_enforced_consecutive_local_origin_failure = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 1 {
		t.Errorf("ejections_enforced_total = %d, want 1 (double-count)", got)
	}
	if got := f.d.ejectionsActive.Load(); got != 1 {
		t.Errorf("ejections_active = %d, want 1", got)
	}
	// split routes local-origin AWAY from the external detectors.
	if got := f.d.ejectionsDetected5xx.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_5xx = %d, want 0 (split routes LO away)", got)
	}
	if got := f.d.ejectionsDetectedGw.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_gateway_failure = %d, want 0 (split routes LO away)", got)
	}
}

// TestOutlierDetector_LocalOriginSuccessResetsStreak: split=true, a successful
// external response (record(ep, 200, false)) mid-streak resets consecLO (the
// connection succeeded), so the LO detector does not eject after fewer than N
// more failures.
func TestOutlierDetector_LocalOriginSuccessResetsStreak(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	f := newDetectorFixture(cfgLocalOrigin(3, 100), eps, panicRoll).withGatewayHandles().withLocalOriginHandles()
	ep := eps[0]
	f.d.record(ep, 502, true)
	f.d.record(ep, 502, true)
	f.d.record(ep, 200, false) // a completed external response ⇒ resets consecLO
	f.d.record(ep, 502, true)
	f.d.record(ep, 502, true)
	if !f.ch.available(ep) {
		t.Fatal("host ejected: the 200 should have reset consecLO below threshold 3")
	}
	if got := f.d.ejectionsDetectedLO.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_local_origin_failure = %d, want 0 (streak reset)", got)
	}
}

// TestOutlierDetector_LocalOriginSplitFalseDelegatesToGateway: split=false
// (default), N consecutive local-origin failures with a gateway-class code
// (503) eject via the gateway/5xx detectors (the local-reply code is mapped to a
// gateway-class 5xx), and the local-origin detector stays inactive
// (detected_LO == 0).
func TestOutlierDetector_LocalOriginSplitFalseDelegatesToGateway(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	// split=false: gateway enabled at 3 enforcing 100; 5xx at 3 enforcing 100.
	cfg := cfgGateway(3, 100, 3)
	cfg.splitLocalOrigin = false
	cfg.consecLOEnabled = true
	cfg.consecutiveLO = 3
	cfg.enforcingLO = 100
	f := newDetectorFixture(cfg, eps, panicRoll).withGatewayHandles().withLocalOriginHandles()
	ep := eps[0]
	for i := 0; i < 3; i++ {
		f.d.record(ep, 503, true) // local-origin failure mapped to gateway-class 5xx
	}
	if f.ch.available(ep) {
		t.Fatal("host not ejected after 3 local-origin failures (split=false delegates to gateway)")
	}
	// delegated to the gateway/5xx detectors (gateway-first ordering).
	if got := f.d.ejectionsDetectedGw.Load(); got != 1 {
		t.Errorf("ejections_detected_consecutive_gateway_failure = %d, want 1 (split=false delegation)", got)
	}
	if got := f.d.ejectionsEnforcedGw.Load(); got != 1 {
		t.Errorf("ejections_enforced_consecutive_gateway_failure = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 1 {
		t.Errorf("ejections_enforced_total = %d, want 1", got)
	}
	// the local-origin detector is inactive under split=false.
	if got := f.d.ejectionsDetectedLO.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_local_origin_failure = %d, want 0 (LO inactive under split=false)", got)
	}
}

// TestOutlierDetector_LocalOriginThresholdZeroNeverEjects: split=true with
// consecutive_local_origin_failure == 0 (consecLOEnabled=false) never ejects on
// local-origin failures.
func TestOutlierDetector_LocalOriginThresholdZeroNeverEjects(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	cfg := cfgLocalOrigin(0, 100)
	cfg.consecLOEnabled = false // explicit 0 ⇒ detector OFF
	f := newDetectorFixture(cfg, eps, panicRoll).withGatewayHandles().withLocalOriginHandles()
	ep := eps[0]
	for i := 0; i < 50; i++ {
		f.d.record(ep, 502, true)
	}
	if !f.ch.available(ep) {
		t.Fatal("host ejected despite the local-origin detector disabled (threshold 0)")
	}
	if got := f.d.ejectionsDetectedLO.Load(); got != 0 {
		t.Errorf("ejections_detected_consecutive_local_origin_failure = %d, want 0 (disabled)", got)
	}
}

// --- Task 6: RecordUpstreamResult seam + detector construction ---

// TestRecordUpstreamResult_NoOpWithoutDetector verifies the public seam is a
// safe no-op for a cluster with no outlier_detection (cl.outlier == nil): no
// panic, no observable effect.
func TestRecordUpstreamResult_NoOpWithoutDetector(t *testing.T) {
	c := mkStaticCluster("od_none", mkLbEndpoint("127.0.0.1", 9100))
	m, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, ok := m.Get("od_none")
	if !ok {
		t.Fatal("od_none not found")
	}
	if cl.outlier != nil {
		t.Fatal("expected nil outlier detector for cluster without outlier_detection")
	}
	// Must not panic.
	cl.RecordUpstreamResult(cl.endpoints[0], UpstreamResult{StatusCode: 503})
}

// TestRecordUpstreamResult_EjectsAfterConsecutive5xx drives the public seam and
// confirms the detector was constructed + wired: 3 consecutive 503s to the same
// endpoint eject it (cl.health.available(ep) becomes false). max_ejection_percent
// is 100 so the cap never blocks; enforcing defaults to 100 so the roll always
// enforces.
func TestRecordUpstreamResult_EjectsAfterConsecutive5xx(t *testing.T) {
	c := mkStaticCluster("od_eject",
		mkLbEndpoint("127.0.0.1", 9200),
		mkLbEndpoint("127.0.0.1", 9201),
	)
	c.OutlierDetection = &clusterv3.OutlierDetection{
		Consecutive_5Xx:    wrapperspb.UInt32(3),
		MaxEjectionPercent: wrapperspb.UInt32(100),
	}
	m, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, ok := m.Get("od_eject")
	if !ok {
		t.Fatal("od_eject not found")
	}
	if cl.outlier == nil {
		t.Fatal("expected non-nil outlier detector for cluster with outlier_detection")
	}
	ep := cl.endpoints[0]
	if !cl.health.available(ep) {
		t.Fatal("endpoint should start available")
	}
	for i := 0; i < 3; i++ {
		cl.RecordUpstreamResult(ep, UpstreamResult{StatusCode: 503})
	}
	if cl.health.available(ep) {
		t.Fatal("endpoint should be unavailable (ejected) after 3 consecutive 5xx")
	}
}

// --- Task 8: outlier_detection stat registration ---

// outlierStatNames is the exact 9-name scoped roster registered by
// registerClusterMetrics for a cluster named "od_stats" (the 40.1 five plus the
// +4 gateway/local-origin detector counters added unconditionally at Task 5).
func outlierStatNames() []string {
	const p = "cluster.od_stats.outlier_detection."
	return []string{
		p + "ejections_active",
		p + "ejections_enforced_total",
		p + "ejections_overflow",
		p + "ejections_detected_consecutive_5xx",
		p + "ejections_enforced_consecutive_5xx",
		p + "ejections_detected_consecutive_gateway_failure",
		p + "ejections_enforced_consecutive_gateway_failure",
		p + "ejections_detected_consecutive_local_origin_failure",
		p + "ejections_enforced_consecutive_local_origin_failure",
	}
}

// TestRegisterOutlierStats_Present asserts a cluster WITH outlier_detection
// registers the 9 fully-qualified outlier stat names and injects every
// handle onto the detector (and the gauge onto the shared clusterHealth).
func TestRegisterOutlierStats_Present(t *testing.T) {
	c := mkStaticCluster("od_stats",
		mkLbEndpoint("127.0.0.1", 9300),
		mkLbEndpoint("127.0.0.1", 9301),
	)
	c.OutlierDetection = &clusterv3.OutlierDetection{
		Consecutive_5Xx:    wrapperspb.UInt32(3),
		MaxEjectionPercent: wrapperspb.UInt32(100),
	}
	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(c), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, ok := m.Get("od_stats")
	if !ok {
		t.Fatal("od_stats not found")
	}
	for _, name := range outlierStatNames() {
		if !hasMetric(reg, name) {
			t.Errorf("expected metric %q to be registered", name)
		}
	}
	// All 9 detector handles must be injected (non-nil) — the 40.1 five plus the
	// +4 gateway/local-origin counters registered unconditionally at Task 5.
	if cl.outlier.ejectionsActive == nil ||
		cl.outlier.ejectionsEnforcedTotal == nil ||
		cl.outlier.ejectionsOverflow == nil ||
		cl.outlier.ejectionsDetected5xx == nil ||
		cl.outlier.ejectionsEnforced5xx == nil ||
		cl.outlier.ejectionsDetectedGw == nil ||
		cl.outlier.ejectionsEnforcedGw == nil ||
		cl.outlier.ejectionsDetectedLO == nil ||
		cl.outlier.ejectionsEnforcedLO == nil {
		t.Fatal("detector stat handles must all be injected (non-nil)")
	}
	// The gauge MUST be the SAME instance on the detector and the clusterHealth
	// (the lazy un-eject in clusterHealth.isEjected decrements this handle).
	if cl.outlier.ejectionsActive != cl.health.ejectionsActive {
		t.Fatal("ejections_active gauge must be the SAME instance on detector and clusterHealth")
	}
}

// TestRegisterOutlierStats_Absent asserts a cluster WITHOUT outlier_detection
// registers NONE of the 9 outlier stat names (stat surface unchanged).
func TestRegisterOutlierStats_Absent(t *testing.T) {
	c := mkStaticCluster("od_stats", mkLbEndpoint("127.0.0.1", 9400))
	reg := stats.NewRegistry()
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	for _, name := range outlierStatNames() {
		if hasMetric(reg, name) {
			t.Errorf("metric %q must NOT be registered for a cluster without outlier_detection", name)
		}
	}
}

// TestRegisterOutlierStats_GaugeReflectsEjection drives an ejection through the
// public seam and confirms the REGISTERED ejections_active gauge reads 1, AND
// that the gauge the detector incremented is the same instance clusterHealth
// holds (so a lazy un-eject would decrement the visible stat).
func TestRegisterOutlierStats_GaugeReflectsEjection(t *testing.T) {
	c := mkStaticCluster("od_stats",
		mkLbEndpoint("127.0.0.1", 9500),
		mkLbEndpoint("127.0.0.1", 9501),
	)
	c.OutlierDetection = &clusterv3.OutlierDetection{
		Consecutive_5Xx:    wrapperspb.UInt32(3),
		MaxEjectionPercent: wrapperspb.UInt32(100),
	}
	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(c), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, _ := m.Get("od_stats")
	ep := cl.endpoints[0]
	for i := 0; i < 3; i++ {
		cl.RecordUpstreamResult(ep, UpstreamResult{StatusCode: 503})
	}
	const gn = "cluster.od_stats.outlier_detection.ejections_active"
	if got, ok := gaugeValue(reg, gn); !ok || got != 1 {
		t.Fatalf("%s = %d (found=%v), want 1", gn, got, ok)
	}
	if cl.outlier.ejectionsActive != cl.health.ejectionsActive {
		t.Fatal("ejections_active gauge instance diverged between detector and clusterHealth")
	}
	// enforced_total and enforced_consecutive_5xx both tick on enforce (double-count).
	if got, ok := counterValue(reg, "cluster.od_stats.outlier_detection.ejections_enforced_total"); !ok || got != 1 {
		t.Fatalf("ejections_enforced_total = %d (found=%v), want 1", got, ok)
	}
	if got, ok := counterValue(reg, "cluster.od_stats.outlier_detection.ejections_enforced_consecutive_5xx"); !ok || got != 1 {
		t.Fatalf("ejections_enforced_consecutive_5xx = %d (found=%v), want 1", got, ok)
	}
	if got, ok := counterValue(reg, "cluster.od_stats.outlier_detection.ejections_detected_consecutive_5xx"); !ok || got != 1 {
		t.Fatalf("ejections_detected_consecutive_5xx = %d (found=%v), want 1", got, ok)
	}
}
