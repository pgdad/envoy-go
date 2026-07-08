package cluster

import (
	"sync"
	"testing"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pgdad/envoy-go/internal/stats"
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

// --- Task 2 (40.3): statistical detector parse fields + interval-as-load-bearing ---

func TestParseOutlierDetection_Statistical_Full(t *testing.T) {
	c := &clusterv3.Cluster{
		OutlierDetection: &clusterv3.OutlierDetection{
			SuccessRateMinimumHosts:        wrapperspb.UInt32(3),
			SuccessRateRequestVolume:       wrapperspb.UInt32(80),
			SuccessRateStdevFactor:         wrapperspb.UInt32(1500),
			EnforcingSuccessRate:           wrapperspb.UInt32(70),
			FailurePercentageThreshold:     wrapperspb.UInt32(90),
			FailurePercentageMinimumHosts:  wrapperspb.UInt32(4),
			FailurePercentageRequestVolume: wrapperspb.UInt32(40),
			EnforcingFailurePercentage:     wrapperspb.UInt32(55),
			Interval:                       durationpb.New(2 * time.Second),
		},
	}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.successRateMinHosts != 3 {
		t.Errorf("successRateMinHosts = %d, want 3", cfg.successRateMinHosts)
	}
	if cfg.successRateReqVolume != 80 {
		t.Errorf("successRateReqVolume = %d, want 80", cfg.successRateReqVolume)
	}
	if cfg.successRateStdevFactor != 1500 {
		t.Errorf("successRateStdevFactor = %d, want 1500", cfg.successRateStdevFactor)
	}
	if cfg.enforcingSuccessRate != 70 {
		t.Errorf("enforcingSuccessRate = %d, want 70", cfg.enforcingSuccessRate)
	}
	if cfg.failurePctThreshold != 90 {
		t.Errorf("failurePctThreshold = %d, want 90", cfg.failurePctThreshold)
	}
	if cfg.failurePctMinHosts != 4 {
		t.Errorf("failurePctMinHosts = %d, want 4", cfg.failurePctMinHosts)
	}
	if cfg.failurePctReqVolume != 40 {
		t.Errorf("failurePctReqVolume = %d, want 40", cfg.failurePctReqVolume)
	}
	if cfg.enforcingFailurePct != 55 {
		t.Errorf("enforcingFailurePct = %d, want 55", cfg.enforcingFailurePct)
	}
	if cfg.interval != 2*time.Second {
		t.Errorf("interval = %v, want 2s", cfg.interval)
	}
}

func TestParseOutlierDetection_Statistical_Defaults(t *testing.T) {
	c := &clusterv3.Cluster{OutlierDetection: &clusterv3.OutlierDetection{}}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for empty outlier_detection")
	}
	if cfg.successRateMinHosts != 5 {
		t.Errorf("successRateMinHosts = %d, want 5", cfg.successRateMinHosts)
	}
	if cfg.successRateReqVolume != 100 {
		t.Errorf("successRateReqVolume = %d, want 100", cfg.successRateReqVolume)
	}
	if cfg.successRateStdevFactor != 1900 {
		t.Errorf("successRateStdevFactor = %d, want 1900", cfg.successRateStdevFactor)
	}
	if cfg.enforcingSuccessRate != 100 {
		t.Errorf("enforcingSuccessRate = %d, want 100", cfg.enforcingSuccessRate)
	}
	if cfg.failurePctThreshold != 85 {
		t.Errorf("failurePctThreshold = %d, want 85", cfg.failurePctThreshold)
	}
	if cfg.failurePctMinHosts != 5 {
		t.Errorf("failurePctMinHosts = %d, want 5", cfg.failurePctMinHosts)
	}
	if cfg.failurePctReqVolume != 50 {
		t.Errorf("failurePctReqVolume = %d, want 50", cfg.failurePctReqVolume)
	}
	if cfg.enforcingFailurePct != 0 {
		t.Errorf("enforcingFailurePct = %d, want 0 (default detect-only)", cfg.enforcingFailurePct)
	}
	if cfg.interval != 10*time.Second {
		t.Errorf("interval = %v, want 10s (default)", cfg.interval)
	}
}

// TestParseOutlierDetection_StdevFactorZeroAccepted: success_rate_stdev_factor 0
// is ACCEPTED by the reference (no reject arm) and surfaces verbatim.
func TestParseOutlierDetection_StdevFactorZeroAccepted(t *testing.T) {
	c := &clusterv3.Cluster{
		OutlierDetection: &clusterv3.OutlierDetection{
			SuccessRateStdevFactor: wrapperspb.UInt32(0),
		},
	}
	cfg, err := parseOutlierDetection(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.successRateStdevFactor != 0 {
		t.Errorf("successRateStdevFactor = %d, want 0 (explicit 0 accepted, no reject)", cfg.successRateStdevFactor)
	}
}

func TestParseOutlierDetection_StatisticalRejects(t *testing.T) {
	cases := []struct {
		name string
		od   *clusterv3.OutlierDetection
		want string
	}{
		{
			name: "enforcing_success_rate>100",
			od:   &clusterv3.OutlierDetection{EnforcingSuccessRate: wrapperspb.UInt32(101)},
			want: `cluster: "c": outlier_detection: enforcing_success_rate: value must be less than or equal to 100`,
		},
		{
			name: "enforcing_failure_percentage>100",
			od:   &clusterv3.OutlierDetection{EnforcingFailurePercentage: wrapperspb.UInt32(101)},
			want: `cluster: "c": outlier_detection: enforcing_failure_percentage: value must be less than or equal to 100`,
		},
		{
			name: "failure_percentage_threshold>100",
			od:   &clusterv3.OutlierDetection{FailurePercentageThreshold: wrapperspb.UInt32(101)},
			want: `cluster: "c": outlier_detection: failure_percentage_threshold: value must be less than or equal to 100`,
		},
		{
			name: "enforcing_local_origin_success_rate>100",
			od:   &clusterv3.OutlierDetection{EnforcingLocalOriginSuccessRate: wrapperspb.UInt32(101)},
			want: `cluster: "c": outlier_detection: enforcing_local_origin_success_rate: value must be less than or equal to 100`,
		},
		{
			name: "enforcing_failure_percentage_local_origin>100",
			od:   &clusterv3.OutlierDetection{EnforcingFailurePercentageLocalOrigin: wrapperspb.UInt32(101)},
			want: `cluster: "c": outlier_detection: enforcing_failure_percentage_local_origin: value must be less than or equal to 100`,
		},
		{
			name: "max_ejection_time<=0",
			od:   &clusterv3.OutlierDetection{MaxEjectionTime: durationpb.New(0)},
			want: `cluster: "c": outlier_detection: max_ejection_time: value must be greater than 0s`,
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
	ch := newClusterHealth(eps, 50)
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

// outlierStatNames is the exact 17-name scoped roster registered by
// registerClusterMetrics for a cluster named "od_stats" (the 40.1 five plus the
// +4 gateway/local-origin detector counters added unconditionally at Task 5 plus
// the +8 statistical detector counters added unconditionally at Task 7 — the 4
// external success_rate/failure_percentage names plus their 4 local-origin
// variants, whose eject logic is deferred but whose names are registered).
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
		p + "ejections_detected_success_rate",
		p + "ejections_enforced_success_rate",
		p + "ejections_detected_failure_percentage",
		p + "ejections_enforced_failure_percentage",
		p + "ejections_detected_local_origin_success_rate",
		p + "ejections_enforced_local_origin_success_rate",
		p + "ejections_detected_local_origin_failure_percentage",
		p + "ejections_enforced_local_origin_failure_percentage",
	}
}

// TestRegisterOutlierStats_Present asserts a cluster WITH outlier_detection
// registers the 17 fully-qualified outlier stat names and injects every
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
	// All 17 detector handles must be injected (non-nil) — the 40.1 five plus the
	// +4 gateway/local-origin counters registered unconditionally at Task 5 plus
	// the +8 statistical counters registered unconditionally at Task 7 (the 4
	// external SR/FP names plus their 4 local-origin variants).
	if cl.outlier.ejectionsActive == nil ||
		cl.outlier.ejectionsEnforcedTotal == nil ||
		cl.outlier.ejectionsOverflow == nil ||
		cl.outlier.ejectionsDetected5xx == nil ||
		cl.outlier.ejectionsEnforced5xx == nil ||
		cl.outlier.ejectionsDetectedGw == nil ||
		cl.outlier.ejectionsEnforcedGw == nil ||
		cl.outlier.ejectionsDetectedLO == nil ||
		cl.outlier.ejectionsEnforcedLO == nil ||
		cl.outlier.ejectionsDetectedSR == nil ||
		cl.outlier.ejectionsEnforcedSR == nil ||
		cl.outlier.ejectionsDetectedFP == nil ||
		cl.outlier.ejectionsEnforcedFP == nil ||
		cl.outlier.ejectionsDetectedLOSR == nil ||
		cl.outlier.ejectionsEnforcedLOSR == nil ||
		cl.outlier.ejectionsDetectedLOFP == nil ||
		cl.outlier.ejectionsEnforcedLOFP == nil {
		t.Fatal("detector stat handles must all be injected (non-nil)")
	}
	// The gauge MUST be the SAME instance on the detector and the clusterHealth
	// (the lazy un-eject in clusterHealth.isEjected decrements this handle).
	if cl.outlier.ejectionsActive != cl.health.ejectionsActive {
		t.Fatal("ejections_active gauge must be the SAME instance on detector and clusterHealth")
	}
}

// TestRegisterOutlierStats_Absent asserts a cluster WITHOUT outlier_detection
// registers NONE of the 17 outlier stat names (stat surface unchanged).
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

// --- Task 3 (40.3): windowed intervalTotal/intervalSuccess accumulation ---

// TestRecord_WindowedCounters verifies the per-host (intervalTotal, intervalSuccess)
// accumulation in record: it counts on EVERY path (2xx, 5xx, local-origin), exactly
// once; success = NOT local-origin AND NOT a 5xx external status. The sweep that
// Swap-resets these is Task 6 — here we read the atomics directly.
func TestRecord_WindowedCounters(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	f := newDetectorFixture(cfgConsec5xx(100), eps, panicRoll) // high threshold ⇒ no eject perturbs the window
	ep := eps[0]
	h := f.ch.states[ep.Addr()]

	for i := 0; i < 3; i++ {
		f.d.record(ep, 200, false) // 2xx success
	}
	for i := 0; i < 2; i++ {
		f.d.record(ep, 503, false) // external 5xx failure
	}
	f.d.record(ep, 0, true) // local-origin failure

	if got := h.intervalTotal.Load(); got != 6 {
		t.Errorf("intervalTotal = %d, want 6 (3 2xx + 2 5xx + 1 local-origin)", got)
	}
	if got := h.intervalSuccess.Load(); got != 3 {
		t.Errorf("intervalSuccess = %d, want 3 (only the 3 2xx are successes)", got)
	}
}

// TestRecord_WindowedCountersNon5xxAreSuccesses verifies that 4xx (<500) and 3xx
// statuses count as successes in the window (success = NOT a 5xx external status).
func TestRecord_WindowedCountersNon5xxAreSuccesses(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	f := newDetectorFixture(cfgConsec5xx(100), eps, panicRoll)
	ep := eps[0]
	h := f.ch.states[ep.Addr()]

	f.d.record(ep, 404, false) // 4xx — success (<500)
	f.d.record(ep, 302, false) // 3xx — success (<500)

	if got := h.intervalTotal.Load(); got != 2 {
		t.Errorf("intervalTotal = %d, want 2", got)
	}
	if got := h.intervalSuccess.Load(); got != 2 {
		t.Errorf("intervalSuccess = %d, want 2 (4xx and 3xx are both successes)", got)
	}
}

// TestRecord_WindowedCountersUnknownAddrCountsNothing verifies the unknown-addr
// early return increments neither window counter on any registered host.
func TestRecord_WindowedCountersUnknownAddrCountsNothing(t *testing.T) {
	eps := mkOutlierEndpoints(1)
	f := newDetectorFixture(cfgConsec5xx(100), eps, panicRoll)
	h := f.ch.states[eps[0].Addr()]

	f.d.record(Endpoint{Host: "127.0.0.1", Port: 65535}, 200, false) // not in the registry

	if got := h.intervalTotal.Load(); got != 0 {
		t.Errorf("intervalTotal = %d, want 0 (unknown addr counts nothing)", got)
	}
	if got := h.intervalSuccess.Load(); got != 0 {
		t.Errorf("intervalSuccess = %d, want 0", got)
	}
}

// --- Task 4 (40.3): success_rate detector (evalSuccessRate) ---

// withSuccessRateHandles assigns the +2 success_rate stat handles (Task 7
// allocates them in registerStats; unit tests inject directly to observe counts).
func (f detectorFixture) withSuccessRateHandles() detectorFixture {
	reg := stats.NewRegistry()
	f.d.ejectionsDetectedSR = reg.NewCounter("outlier_detection.ejections_detected_success_rate")
	f.d.ejectionsEnforcedSR = reg.NewCounter("outlier_detection.ejections_enforced_success_rate")
	return f
}

// cfgSuccessRate builds a cfg with the success_rate detector knobs set and the
// base eject machinery permissive (maxEjectionPct=100, base=30s).
func cfgSuccessRate(minHosts, reqVolume, stdevFactor, enforcing uint32) outlierConfig {
	return outlierConfig{
		baseEjectionTime:       30 * time.Second,
		maxEjectionPct:         100,
		successRateMinHosts:    minHosts,
		successRateReqVolume:   reqVolume,
		successRateStdevFactor: stdevFactor,
		enforcingSuccessRate:   enforcing,
	}
}

// snapFor builds a []hostWindow over the fixture's registered hosts: window[i]
// describes eps[i] with the given (total, success). Pulls the live *hostHealth
// from the fixture's registry so tryEject's CAS + cap accounting are real.
func snapFor(f detectorFixture, eps []Endpoint, totals, successes []uint64) []hostWindow {
	snap := make([]hostWindow, len(eps))
	for i, ep := range eps {
		snap[i] = hostWindow{
			ep:      ep,
			h:       f.ch.states[ep.Addr()],
			total:   totals[i],
			success: successes[i],
		}
	}
	return snap
}

// TestEvalSuccessRate_EjectsOutlier: 5 hosts at 100% success + 1 host at 0% over
// volume 100 (all eligible, 6 >= minHosts 2). mean=5/6≈0.833, pop stddev≈0.373,
// threshold = mean - 1.9*stddev ≈ 0.125 (POSITIVE) -> only the 0% host is below
// it -> it ejects (the +1 bumps detected/enforced SR + the cross-detector
// double-count + ejections_active); the 5 healthy hosts are NOT ejected.
func TestEvalSuccessRate_EjectsOutlier(t *testing.T) {
	eps := mkOutlierEndpoints(6)
	f := newDetectorFixture(cfgSuccessRate(2, 10, 1900, 100), eps, panicRoll).withSuccessRateHandles()
	totals := []uint64{100, 100, 100, 100, 100, 100}
	successes := []uint64{100, 100, 100, 100, 100, 0}
	snap := snapFor(f, eps, totals, successes)

	f.d.evalSuccessRate(snap)

	if f.ch.available(eps[5]) {
		t.Fatal("zero-success host should be ejected (rate 0 < threshold ≈ 0.125)")
	}
	for i := 0; i < 5; i++ {
		if !f.ch.available(eps[i]) {
			t.Errorf("healthy host %d (100%%) should NOT be ejected", i)
		}
	}
	if got := f.d.ejectionsDetectedSR.Load(); got != 1 {
		t.Errorf("ejections_detected_success_rate = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedSR.Load(); got != 1 {
		t.Errorf("ejections_enforced_success_rate = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 1 {
		t.Errorf("ejections_enforced_total = %d, want 1", got)
	}
	if got := f.d.ejectionsActive.Load(); got != 1 {
		t.Errorf("ejections_active = %d, want 1", got)
	}
}

// TestEvalSuccessRate_MinimumHostsGate: only 2 eligible hosts but minHosts=5 ->
// the eligibility gate short-circuits BEFORE any threshold math -> NO eject.
func TestEvalSuccessRate_MinimumHostsGate(t *testing.T) {
	eps := mkOutlierEndpoints(2)
	f := newDetectorFixture(cfgSuccessRate(5, 10, 1900, 100), eps, panicRoll).withSuccessRateHandles()
	snap := snapFor(f, eps, []uint64{100, 100}, []uint64{0, 100})

	f.d.evalSuccessRate(snap)

	if !f.ch.available(eps[0]) {
		t.Fatal("no host should eject: only 2 eligible < minHosts 5")
	}
	if got := f.d.ejectionsDetectedSR.Load(); got != 0 {
		t.Errorf("ejections_detected_success_rate = %d, want 0 (gate)", got)
	}
}

// TestEvalSuccessRate_RequestVolumeGate: the bad host has total 5 (< reqVolume 10)
// so it is NOT eligible -> only the 5 good hosts form the eligible set (all 100%)
// -> the bad host is never evaluated -> NO eject.
func TestEvalSuccessRate_RequestVolumeGate(t *testing.T) {
	eps := mkOutlierEndpoints(6)
	f := newDetectorFixture(cfgSuccessRate(2, 10, 1900, 100), eps, panicRoll).withSuccessRateHandles()
	totals := []uint64{100, 100, 100, 100, 100, 5} // last host below reqVolume
	successes := []uint64{100, 100, 100, 100, 100, 0}
	snap := snapFor(f, eps, totals, successes)

	f.d.evalSuccessRate(snap)

	if !f.ch.available(eps[5]) {
		t.Fatal("low-volume host (total 5 < reqVolume 10) is ineligible and must not eject")
	}
	if got := f.d.ejectionsDetectedSR.Load(); got != 0 {
		t.Errorf("ejections_detected_success_rate = %d, want 0 (volume gate)", got)
	}
}

// TestEvalSuccessRate_NegativeThresholdNoOp (AMEND-OD3-5): 2 hosts 100% + 1 host
// 0%, minHosts=2 (eligibility gate PASSES: 3 >= 2). mean≈0.667, pop stddev≈0.471,
// threshold = mean - 1.9*stddev ≈ -0.228 (NON-positive) -> benign no-op (no rate
// in [0,1] is below it) -> NO eject.
func TestEvalSuccessRate_NegativeThresholdNoOp(t *testing.T) {
	eps := mkOutlierEndpoints(3)
	f := newDetectorFixture(cfgSuccessRate(2, 10, 1900, 100), eps, panicRoll).withSuccessRateHandles()
	snap := snapFor(f, eps, []uint64{100, 100, 100}, []uint64{100, 100, 0})

	f.d.evalSuccessRate(snap)

	for i := 0; i < 3; i++ {
		if !f.ch.available(eps[i]) {
			t.Errorf("host %d should NOT eject (threshold ≈ -0.228 <= 0)", i)
		}
	}
	if got := f.d.ejectionsDetectedSR.Load(); got != 0 {
		t.Errorf("ejections_detected_success_rate = %d, want 0 (negative-threshold no-op)", got)
	}
}

// TestEvalSuccessRate_DetectOnly: enforcing_success_rate=0 -> the outlier crosses
// the (positive) threshold so detected bumps, but the enforce-roll yields
// detect-only -> enforced SR == 0 and ejections_active == 0.
func TestEvalSuccessRate_DetectOnly(t *testing.T) {
	eps := mkOutlierEndpoints(6)
	f := newDetectorFixture(cfgSuccessRate(2, 10, 1900, 0), eps, panicRoll).withSuccessRateHandles()
	totals := []uint64{100, 100, 100, 100, 100, 100}
	successes := []uint64{100, 100, 100, 100, 100, 0}
	snap := snapFor(f, eps, totals, successes)

	f.d.evalSuccessRate(snap)

	if !f.ch.available(eps[5]) {
		t.Fatal("detect-only (enforcing 0) must not eject the outlier")
	}
	if got := f.d.ejectionsDetectedSR.Load(); got != 1 {
		t.Errorf("ejections_detected_success_rate = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedSR.Load(); got != 0 {
		t.Errorf("ejections_enforced_success_rate = %d, want 0 (detect-only)", got)
	}
	if got := f.d.ejectionsActive.Load(); got != 0 {
		t.Errorf("ejections_active = %d, want 0 (detect-only)", got)
	}
}

// --- Task 5 (40.3): failure_percentage detector (evalFailurePercentage) ---

// withFailurePercentageHandles assigns the +2 failure_percentage stat handles
// (Task 7 allocates them in registerStats; unit tests inject directly to observe
// counts). Mirrors withSuccessRateHandles.
func (f detectorFixture) withFailurePercentageHandles() detectorFixture {
	reg := stats.NewRegistry()
	f.d.ejectionsDetectedFP = reg.NewCounter("outlier_detection.ejections_detected_failure_percentage")
	f.d.ejectionsEnforcedFP = reg.NewCounter("outlier_detection.ejections_enforced_failure_percentage")
	return f
}

// cfgFailurePct builds a cfg with the failure_percentage detector knobs set and
// the base eject machinery permissive (maxEjectionPct=100, base=30s).
func cfgFailurePct(minHosts, reqVolume, threshold, enforcing uint32) outlierConfig {
	return outlierConfig{
		baseEjectionTime:    30 * time.Second,
		maxEjectionPct:      100,
		failurePctMinHosts:  minHosts,
		failurePctReqVolume: reqVolume,
		failurePctThreshold: threshold,
		enforcingFailurePct: enforcing,
	}
}

// TestEvalFailurePercentage_EjectsFailingHost: 2 hosts at 0% fail + 1 host at
// 100% fail over volume 100 (all eligible, 3 >= minHosts 2). The 100%-fail host
// crosses threshold 85 -> it ejects (detected/enforced FP + the cross-detector
// double-count + ejections_active); the 2 healthy hosts are NOT ejected.
func TestEvalFailurePercentage_EjectsFailingHost(t *testing.T) {
	eps := mkOutlierEndpoints(3)
	f := newDetectorFixture(cfgFailurePct(2, 10, 85, 100), eps, panicRoll).withFailurePercentageHandles()
	totals := []uint64{100, 100, 100}
	successes := []uint64{100, 100, 0}
	snap := snapFor(f, eps, totals, successes)

	f.d.evalFailurePercentage(snap)

	if f.ch.available(eps[2]) {
		t.Fatal("fully-failing host should be ejected (failure 100 pct >= threshold 85)")
	}
	for i := 0; i < 2; i++ {
		if !f.ch.available(eps[i]) {
			t.Errorf("healthy host %d (0%% fail) should NOT be ejected", i)
		}
	}
	if got := f.d.ejectionsDetectedFP.Load(); got != 1 {
		t.Errorf("ejections_detected_failure_percentage = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedFP.Load(); got != 1 {
		t.Errorf("ejections_enforced_failure_percentage = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 1 {
		t.Errorf("ejections_enforced_total = %d, want 1", got)
	}
	if got := f.d.ejectionsActive.Load(); got != 1 {
		t.Errorf("ejections_active = %d, want 1", got)
	}
}

// TestEvalFailurePercentage_BoundaryGE (load-bearing): 3 hosts; host[2] has total
// 100 success 15 ⇒ failure% exactly 85 (== threshold) -> ejects (>= includes the
// boundary). The cross-multiplied form: (100-15)*100=8500 >= 85*100=8500 (true).
func TestEvalFailurePercentage_BoundaryGE(t *testing.T) {
	eps := mkOutlierEndpoints(3)
	f := newDetectorFixture(cfgFailurePct(2, 10, 85, 100), eps, panicRoll).withFailurePercentageHandles()
	snap := snapFor(f, eps, []uint64{100, 100, 100}, []uint64{100, 100, 15})

	f.d.evalFailurePercentage(snap)

	if f.ch.available(eps[2]) {
		t.Fatal("host at failure% exactly 85 should eject (>= includes the boundary)")
	}
	if got := f.d.ejectionsDetectedFP.Load(); got != 1 {
		t.Errorf("ejections_detected_failure_percentage = %d, want 1 (boundary)", got)
	}
}

// TestEvalFailurePercentage_BelowBoundaryNoEject: host[2] total 100 success 16 ⇒
// failure% 84 (< threshold 85) -> NOT ejected. (84*100=8400 < 85*100=8500.)
func TestEvalFailurePercentage_BelowBoundaryNoEject(t *testing.T) {
	eps := mkOutlierEndpoints(3)
	f := newDetectorFixture(cfgFailurePct(2, 10, 85, 100), eps, panicRoll).withFailurePercentageHandles()
	snap := snapFor(f, eps, []uint64{100, 100, 100}, []uint64{100, 100, 16})

	f.d.evalFailurePercentage(snap)

	if !f.ch.available(eps[2]) {
		t.Fatal("host at failure% 84 (< threshold 85) must NOT eject")
	}
	if got := f.d.ejectionsDetectedFP.Load(); got != 0 {
		t.Errorf("ejections_detected_failure_percentage = %d, want 0 (below boundary)", got)
	}
}

// TestEvalFailurePercentage_MinimumHostsGate: 3 eligible hosts but minHosts=5 ->
// the eligibility gate short-circuits BEFORE any threshold math -> NO eject.
func TestEvalFailurePercentage_MinimumHostsGate(t *testing.T) {
	eps := mkOutlierEndpoints(3)
	f := newDetectorFixture(cfgFailurePct(5, 10, 85, 100), eps, panicRoll).withFailurePercentageHandles()
	snap := snapFor(f, eps, []uint64{100, 100, 100}, []uint64{100, 100, 0})

	f.d.evalFailurePercentage(snap)

	if !f.ch.available(eps[2]) {
		t.Fatal("no host should eject: only 3 eligible < minHosts 5")
	}
	if got := f.d.ejectionsDetectedFP.Load(); got != 0 {
		t.Errorf("ejections_detected_failure_percentage = %d, want 0 (gate)", got)
	}
}

// TestEvalFailurePercentage_RequestVolumeGate: the failing host has total 5 (<
// reqVolume 10) so it is NOT eligible -> only the 2 good hosts form the eligible
// set -> the bad host is never evaluated -> NO eject.
func TestEvalFailurePercentage_RequestVolumeGate(t *testing.T) {
	eps := mkOutlierEndpoints(3)
	f := newDetectorFixture(cfgFailurePct(2, 10, 85, 100), eps, panicRoll).withFailurePercentageHandles()
	totals := []uint64{100, 100, 5} // last host below reqVolume
	successes := []uint64{100, 100, 0}
	snap := snapFor(f, eps, totals, successes)

	f.d.evalFailurePercentage(snap)

	if !f.ch.available(eps[2]) {
		t.Fatal("low-volume host (total 5 < reqVolume 10) is ineligible and must not eject")
	}
	if got := f.d.ejectionsDetectedFP.Load(); got != 0 {
		t.Errorf("ejections_detected_failure_percentage = %d, want 0 (volume gate)", got)
	}
}

// TestEvalFailurePercentage_DetectOnly: enforcing_failure_percentage=0 (the
// DEFAULT posture) -> the failing host crosses the threshold so detected bumps,
// but the enforce-roll yields detect-only -> enforced FP == 0 and active == 0.
func TestEvalFailurePercentage_DetectOnly(t *testing.T) {
	eps := mkOutlierEndpoints(3)
	f := newDetectorFixture(cfgFailurePct(2, 10, 85, 0), eps, panicRoll).withFailurePercentageHandles()
	totals := []uint64{100, 100, 100}
	successes := []uint64{100, 100, 0}
	snap := snapFor(f, eps, totals, successes)

	f.d.evalFailurePercentage(snap)

	if !f.ch.available(eps[2]) {
		t.Fatal("detect-only (enforcing 0) must not eject the failing host")
	}
	if got := f.d.ejectionsDetectedFP.Load(); got != 1 {
		t.Errorf("ejections_detected_failure_percentage = %d, want 1", got)
	}
	if got := f.d.ejectionsEnforcedFP.Load(); got != 0 {
		t.Errorf("ejections_enforced_failure_percentage = %d, want 0 (detect-only)", got)
	}
	if got := f.d.ejectionsActive.Load(); got != 0 {
		t.Errorf("ejections_active = %d, want 0 (detect-only)", got)
	}
}

// TestEvalFailurePercentage_ZeroTrafficGuard (★ the hardening): with reqVolume 0,
// a zero-traffic host (total 0) would pass the bare volume gate and the bare
// cross-multiplied test (0*100 >= threshold*0 ⇒ 0 >= 0 ⇒ true) and spuriously
// eject. The total==0 guard excludes it (the reference excludes zero-traffic
// hosts). 2 real failing hosts cross the threshold and DO eject (minHosts 2 met by
// the real hosts only).
func TestEvalFailurePercentage_ZeroTrafficGuard(t *testing.T) {
	eps := mkOutlierEndpoints(3)
	f := newDetectorFixture(cfgFailurePct(2, 0, 85, 100), eps, panicRoll).withFailurePercentageHandles()
	totals := []uint64{100, 100, 0} // host[2] is zero-traffic
	successes := []uint64{0, 0, 0}  // host[0],[1] are 100% fail; host[2] has no traffic
	snap := snapFor(f, eps, totals, successes)

	f.d.evalFailurePercentage(snap)

	if !f.ch.available(eps[2]) {
		t.Fatal("zero-traffic host (total 0) must NOT eject (the 0>=0 spurious-eject guard)")
	}
	if f.ch.states[eps[2].Addr()].ejected.Load() {
		t.Error("zero-traffic host ejected flag set; want false (excluded from eligibility)")
	}
	// the two real failing hosts still eject (guard is targeted at total==0 only).
	for i := 0; i < 2; i++ {
		if f.ch.available(eps[i]) {
			t.Errorf("real failing host %d (100%% fail) should eject", i)
		}
	}
	if got := f.d.ejectionsDetectedFP.Load(); got != 2 {
		t.Errorf("ejections_detected_failure_percentage = %d, want 2 (the two real hosts)", got)
	}
}

// --- Task 6 (40.3): per-interval sweep + StartOutlierDetection/Drain lifecycle ---

// TestSweep_EjectsOutlierAndResetsWindows: 6 hosts, 5 at 100% success + 1 at 0%
// success (each >= reqVolume). sweep() snapshots+resets every window then runs SR
// then FP over the snapshot. The bad host ejects exactly once (CAS makes a host
// ejectable at most once per sweep), and EVERY host's window is reset to 0.
func TestSweep_EjectsOutlierAndResetsWindows(t *testing.T) {
	eps := mkOutlierEndpoints(6)
	cfg := cfgSuccessRate(2, 10, 1900, 100)
	// FP detect-only (enforcing 0) so the second detector cannot double-eject; the
	// SR detector owns the eject. (Defaults leave failurePct* zero, which would make
	// every host eligible at threshold 0 — set explicit FP knobs to keep FP inert.)
	cfg.failurePctMinHosts = 100 // FP eligibility gate never met
	cfg.failurePctReqVolume = 10
	cfg.failurePctThreshold = 85
	cfg.enforcingFailurePct = 0
	f := newDetectorFixture(cfg, eps, panicRoll).withSuccessRateHandles().withFailurePercentageHandles()

	// Drive real record() traffic: hosts 0..4 all-success, host 5 all-failure.
	for i := 0; i < 5; i++ {
		for j := 0; j < 100; j++ {
			f.d.record(eps[i], 200, false)
		}
	}
	for j := 0; j < 100; j++ {
		f.d.record(eps[5], 503, false)
	}

	f.d.sweep()

	if f.ch.available(eps[5]) {
		t.Fatal("zero-success host should be ejected by the sweep (SR detector)")
	}
	for i := 0; i < 5; i++ {
		if !f.ch.available(eps[i]) {
			t.Errorf("healthy host %d (100%%) should NOT be ejected", i)
		}
	}
	if got := f.d.ejectionsActive.Load(); got != 1 {
		t.Errorf("ejections_active = %d, want 1 (one eject per host per sweep)", got)
	}
	if got := f.d.ejectionsDetectedSR.Load(); got != 1 {
		t.Errorf("ejections_detected_success_rate = %d, want 1", got)
	}
	// Every window must be reset to 0 (Swap-to-0 ran for all hosts).
	for i, ep := range eps {
		h := f.ch.states[ep.Addr()]
		if got := h.intervalTotal.Load(); got != 0 {
			t.Errorf("host %d intervalTotal = %d after sweep, want 0 (window reset)", i, got)
		}
		if got := h.intervalSuccess.Load(); got != 0 {
			t.Errorf("host %d intervalSuccess = %d after sweep, want 0 (window reset)", i, got)
		}
	}
}

// TestSweep_OneEjectPerHostEvenIfBothDetectorsFlag: a host that crosses BOTH the
// success_rate AND failure_percentage thresholds is ejected AT MOST ONCE per sweep
// (AMEND-OD3-3 — the CAS in tryEject makes the second detector's tryEject a no-op),
// so ejections_active counts one ejection for the host.
func TestSweep_OneEjectPerHostEvenIfBothDetectorsFlag(t *testing.T) {
	eps := mkOutlierEndpoints(6)
	// SR detector ejects the 0%-success host; FP detector ALSO flags it (100% fail
	// >= threshold 85). Both eligibility gates pass (6 >= minHosts 2 for both).
	cfg := cfgSuccessRate(2, 10, 1900, 100)
	cfg.failurePctMinHosts = 2
	cfg.failurePctReqVolume = 10
	cfg.failurePctThreshold = 85
	cfg.enforcingFailurePct = 100
	f := newDetectorFixture(cfg, eps, panicRoll).withSuccessRateHandles().withFailurePercentageHandles()

	for i := 0; i < 5; i++ {
		for j := 0; j < 100; j++ {
			f.d.record(eps[i], 200, false)
		}
	}
	for j := 0; j < 100; j++ {
		f.d.record(eps[5], 503, false)
	}

	f.d.sweep()

	if f.ch.available(eps[5]) {
		t.Fatal("bad host should be ejected")
	}
	// Exactly one ejection though both detectors flagged the same host.
	if got := f.d.ejectionsActive.Load(); got != 1 {
		t.Errorf("ejections_active = %d, want 1 (one eject per host even if both detectors flag)", got)
	}
	if got := f.d.ejectionsEnforcedTotal.Load(); got != 1 {
		t.Errorf("ejections_enforced_total = %d, want 1 (CAS makes the 2nd tryEject a no-op)", got)
	}
	// Both detectors DETECTED the crossing (detected is Inc'd before tryEject).
	if got := f.d.ejectionsDetectedSR.Load(); got != 1 {
		t.Errorf("ejections_detected_success_rate = %d, want 1", got)
	}
	if got := f.d.ejectionsDetectedFP.Load(); got != 1 {
		t.Errorf("ejections_detected_failure_percentage = %d, want 1", got)
	}
}

// TestSweep_ResetsEveryWindowEvenWithNoEject: with no detector configured to
// eject (all hosts identical, eligibility gates unmet), the sweep still Swap-resets
// EVERY host's window to 0.
func TestSweep_ResetsEveryWindowEvenWithNoEject(t *testing.T) {
	eps := mkOutlierEndpoints(3)
	cfg := cfgSuccessRate(100, 10, 1900, 100) // minHosts 100 ⇒ SR gate never met
	cfg.failurePctMinHosts = 100              // FP gate never met
	f := newDetectorFixture(cfg, eps, panicRoll).withSuccessRateHandles().withFailurePercentageHandles()

	for _, ep := range eps {
		for j := 0; j < 50; j++ {
			f.d.record(ep, 200, false)
		}
	}
	f.d.sweep()

	for i, ep := range eps {
		if !f.ch.available(ep) {
			t.Errorf("host %d should NOT be ejected (no eligibility)", i)
		}
		h := f.ch.states[ep.Addr()]
		if got := h.intervalTotal.Load(); got != 0 {
			t.Errorf("host %d intervalTotal = %d after sweep, want 0", i, got)
		}
		if got := h.intervalSuccess.Load(); got != 0 {
			t.Errorf("host %d intervalSuccess = %d after sweep, want 0", i, got)
		}
	}
}
