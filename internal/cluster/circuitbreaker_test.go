package cluster

import (
	"sync"
	"sync/atomic"
	"testing"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestParseCircuitBreakers_Absent(t *testing.T) {
	c := &clusterv3.Cluster{}
	cb, err := parseCircuitBreakers(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb != nil {
		t.Fatalf("expected nil circuitBreaker for cluster with no circuit_breakers, got %+v", cb)
	}
}

func TestParseCircuitBreakers_DefaultMaxRequests(t *testing.T) {
	c := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{Priority: corev3.RoutingPriority_DEFAULT, MaxRequests: wrapperspb.UInt32(4)},
			},
		},
	}
	cb, err := parseCircuitBreakers(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb == nil {
		t.Fatal("expected non-nil circuitBreaker")
	}
	if cb.prio[0].maxRequests != 4 {
		t.Errorf("prio[0].maxRequests = %d, want 4", cb.prio[0].maxRequests)
	}
	if cb.prio[1].maxRequests != 1024 {
		t.Errorf("prio[1].maxRequests = %d, want 1024 (default)", cb.prio[1].maxRequests)
	}
}

func TestParseCircuitBreakers_AbsentMaxRequestsDefaults1024(t *testing.T) {
	c := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{Priority: corev3.RoutingPriority_DEFAULT},
			},
		},
	}
	cb, err := parseCircuitBreakers(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb == nil {
		t.Fatal("expected non-nil circuitBreaker")
	}
	if cb.prio[0].maxRequests != 1024 {
		t.Errorf("prio[0].maxRequests = %d, want 1024 (absent default)", cb.prio[0].maxRequests)
	}
}

func TestParseCircuitBreakers_ExplicitZero(t *testing.T) {
	c := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{Priority: corev3.RoutingPriority_DEFAULT, MaxRequests: wrapperspb.UInt32(0)},
			},
		},
	}
	cb, err := parseCircuitBreakers(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb == nil {
		t.Fatal("expected non-nil circuitBreaker")
	}
	if cb.prio[0].maxRequests != 0 {
		t.Errorf("prio[0].maxRequests = %d, want 0 (explicit)", cb.prio[0].maxRequests)
	}
}

func TestParseCircuitBreakers_InvalidPriority(t *testing.T) {
	c := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{Priority: corev3.RoutingPriority(2)},
			},
		},
	}
	_, err := parseCircuitBreakers(c, "c")
	if err == nil {
		t.Fatal("expected error for priority value 2")
	}
	want := `cluster: "c": circuit_breakers: priority: value must be one of the defined enum values`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseCircuitBreakers_DuplicatePriority(t *testing.T) {
	c := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{Priority: corev3.RoutingPriority_DEFAULT},
				{Priority: corev3.RoutingPriority_DEFAULT},
			},
		},
	}
	_, err := parseCircuitBreakers(c, "c")
	if err == nil {
		t.Fatal("expected error for duplicate DEFAULT thresholds")
	}
	want := `cluster: "c": circuit_breakers: duplicate threshold priority`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseCircuitBreakers_RetryBudgetPercentOverflow(t *testing.T) {
	c := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{
					Priority: corev3.RoutingPriority_DEFAULT,
					RetryBudget: &clusterv3.CircuitBreakers_Thresholds_RetryBudget{
						BudgetPercent: &typev3.Percent{Value: 150},
					},
				},
			},
		},
	}
	_, err := parseCircuitBreakers(c, "c")
	if err == nil {
		t.Fatal("expected error for budget_percent 150")
	}
	want := `cluster: "c": circuit_breakers: retry_budget: budget_percent: value must be inside range [0, 100]`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseCircuitBreakers_PerHostThresholdsAccepted(t *testing.T) {
	c := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			PerHostThresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{Priority: corev3.RoutingPriority_DEFAULT, MaxRequests: wrapperspb.UInt32(7)},
			},
		},
	}
	cb, err := parseCircuitBreakers(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb == nil {
		t.Fatal("expected non-nil circuitBreaker (per_host_thresholds silent-ignored, not rejected)")
	}
	// per_host_thresholds is ignored; the priority budgets keep their defaults.
	if cb.prio[0].maxRequests != 1024 {
		t.Errorf("prio[0].maxRequests = %d, want 1024 (per_host ignored)", cb.prio[0].maxRequests)
	}
}

func TestCircuitBreakerTryAcquireOverflow(t *testing.T) {
	reg := stats.NewRegistry()
	cb := &circuitBreaker{}
	cb.prio[0].maxRequests = 2
	cb.prio[0].rqOpen = reg.NewGauge("test.rq_open")
	cb.upstreamRqPendingOverflow = reg.NewCounter("test.overflow")

	if !cb.tryAcquire(0) {
		t.Fatal("1st tryAcquire(0): got false, want true")
	}
	if !cb.tryAcquire(0) {
		t.Fatal("2nd tryAcquire(0): got false, want true")
	}
	if cb.tryAcquire(0) {
		t.Fatal("3rd tryAcquire(0): got true, want false (overflow)")
	}
	if got := cb.prio[0].rqOpen.Load(); got != 1 {
		t.Errorf("rqOpen = %d, want 1 after overflow", got)
	}
	if got := cb.upstreamRqPendingOverflow.Load(); got != 1 {
		t.Errorf("upstreamRqPendingOverflow = %d, want 1 after overflow", got)
	}
}

func TestCircuitBreakerReleaseReopensBudget(t *testing.T) {
	reg := stats.NewRegistry()
	cb := &circuitBreaker{}
	cb.prio[0].maxRequests = 2
	cb.prio[0].rqOpen = reg.NewGauge("test.rq_open")
	cb.upstreamRqPendingOverflow = reg.NewCounter("test.overflow")

	// saturate
	cb.tryAcquire(0)
	cb.tryAcquire(0)
	if cb.tryAcquire(0) {
		t.Fatal("expected overflow before release")
	}
	if got := cb.prio[0].rqOpen.Load(); got != 1 {
		t.Fatalf("rqOpen = %d, want 1 (saturated)", got)
	}

	cb.release(0)
	if got := cb.prio[0].rqOpen.Load(); got != 0 {
		t.Errorf("rqOpen = %d, want 0 after release (back under budget)", got)
	}
	if !cb.tryAcquire(0) {
		t.Error("tryAcquire(0) after release: got false, want true")
	}
}

func TestCircuitBreakerZeroMaxRequestsRejectsAll(t *testing.T) {
	reg := stats.NewRegistry()
	cb := &circuitBreaker{}
	cb.prio[0].maxRequests = 0
	cb.prio[0].rqOpen = reg.NewGauge("test.rq_open")
	cb.upstreamRqPendingOverflow = reg.NewCounter("test.overflow")

	if cb.tryAcquire(0) {
		t.Fatal("1st tryAcquire(0) with maxRequests=0: got true, want false (reject-all)")
	}
	if got := cb.prio[0].rqOpen.Load(); got != 1 {
		t.Errorf("rqOpen = %d, want 1 (reject-all)", got)
	}
}

func TestCircuitBreakerConcurrentNeverExceedsBudget(t *testing.T) {
	reg := stats.NewRegistry()
	cb := &circuitBreaker{}
	cb.prio[0].maxRequests = 10
	cb.prio[0].rqOpen = reg.NewGauge("test.rq_open")
	cb.upstreamRqPendingOverflow = reg.NewCounter("test.overflow")

	var holders atomic.Int64
	var peak atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				if cb.tryAcquire(0) {
					cur := holders.Add(1)
					for {
						p := peak.Load()
						if cur <= p || peak.CompareAndSwap(p, cur) {
							break
						}
					}
					holders.Add(-1)
					cb.release(0)
				}
			}
		}()
	}
	wg.Wait()

	if got := peak.Load(); got > 10 {
		t.Errorf("peak concurrent holders = %d, want <= 10", got)
	}
}

// TestClusterTryAcquireReleaseRequest exercises the exported router-facing seam
// (Task 5): a Cluster WITH a registered circuitBreaker (budget 1) admits one
// request then overflows, and a release reopens the budget. (ADR-0248)
func TestClusterTryAcquireReleaseRequest(t *testing.T) {
	reg := stats.NewRegistry()
	cb := &circuitBreaker{}
	cb.prio[0].maxRequests = 1
	cb.prio[0].rqOpen = reg.NewGauge("test.rq_open")
	cb.upstreamRqPendingOverflow = reg.NewCounter("test.overflow")
	cl := &Cluster{name: "cb", circuitBreaker: cb}

	if !cl.TryAcquireRequest() {
		t.Fatal("1st TryAcquireRequest: got false, want true (budget 1)")
	}
	if cl.TryAcquireRequest() {
		t.Fatal("2nd TryAcquireRequest: got true, want false (overflow)")
	}
	cl.ReleaseRequest()
	if !cl.TryAcquireRequest() {
		t.Fatal("TryAcquireRequest after ReleaseRequest: got false, want true (budget reopened)")
	}
}

// TestClusterTryAcquireRequestNoCircuitBreaker asserts the nil-guard: a Cluster
// WITHOUT circuit_breakers (circuitBreaker == nil) always admits, and
// ReleaseRequest is an inert no-op (must not panic). (ADR-0248)
func TestClusterTryAcquireRequestNoCircuitBreaker(t *testing.T) {
	cl := &Cluster{name: "no_cb"}
	for i := 0; i < 5; i++ {
		if !cl.TryAcquireRequest() {
			t.Fatalf("TryAcquireRequest #%d with no circuit_breakers: got false, want true (nil-guard)", i)
		}
	}
	cl.ReleaseRequest() // no-op; must not panic
}

// circuitBreakerStatNames returns the 14 fully-qualified stat names that
// registerClusterMetrics allocates for a cluster named "cb_stats" WITH
// circuit_breakers (SPEC §7): the 10 per-priority *_open gauges (default + high,
// each {cx_open, cx_pool_open, rq_open, rq_pending_open, rq_retry_open}) plus the
// 4 cluster overflow counters.
func circuitBreakerStatNames() []string {
	const p = "cluster.cb_stats."
	var names []string
	for _, prio := range []string{"default", "high"} {
		gp := p + "circuit_breakers." + prio + "."
		names = append(names,
			gp+"cx_open",
			gp+"cx_pool_open",
			gp+"rq_open",
			gp+"rq_pending_open",
			gp+"rq_retry_open",
		)
	}
	names = append(names,
		p+"upstream_cx_overflow",
		p+"upstream_cx_pool_overflow",
		p+"upstream_rq_pending_overflow",
		p+"upstream_rq_retry_overflow",
	)
	return names
}

// TestRegisterCircuitBreakerStats_Present asserts a cluster WITH circuit_breakers
// registers EXACTLY the 14 circuit_breakers stat names and binds the two LIVE
// handles (default.rq_open gauge + upstream_rq_pending_overflow counter).
func TestRegisterCircuitBreakerStats_Present(t *testing.T) {
	c := mkStaticCluster("cb_stats", mkLbEndpoint("127.0.0.1", 9600))
	c.CircuitBreakers = &clusterv3.CircuitBreakers{
		Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
			{Priority: corev3.RoutingPriority_DEFAULT, MaxRequests: wrapperspb.UInt32(4)},
		},
	}
	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(c), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, ok := m.Get("cb_stats")
	if !ok {
		t.Fatal("cb_stats not found")
	}
	if cl.circuitBreaker == nil {
		t.Fatal("circuitBreaker must be built for a cluster with circuit_breakers")
	}
	for _, name := range circuitBreakerStatNames() {
		if !hasMetric(reg, name) {
			t.Errorf("expected metric %q to be registered", name)
		}
	}
	// The two LIVE handles must be injected (non-nil).
	if cl.circuitBreaker.prio[0].rqOpen == nil {
		t.Error("default.rq_open gauge handle must be injected (non-nil)")
	}
	if cl.circuitBreaker.upstreamRqPendingOverflow == nil {
		t.Error("upstream_rq_pending_overflow counter handle must be injected (non-nil)")
	}
}

// TestRegisterCircuitBreakerStats_Absent asserts a cluster WITHOUT circuit_breakers
// registers NONE of the 14 circuit_breakers stat names.
func TestRegisterCircuitBreakerStats_Absent(t *testing.T) {
	c := mkStaticCluster("cb_stats", mkLbEndpoint("127.0.0.1", 9700))
	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(c), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, ok := m.Get("cb_stats")
	if !ok {
		t.Fatal("cb_stats not found")
	}
	if cl.circuitBreaker != nil {
		t.Fatal("circuitBreaker must be nil for a cluster without circuit_breakers")
	}
	for _, name := range circuitBreakerStatNames() {
		if hasMetric(reg, name) {
			t.Errorf("metric %q must NOT be registered for a cluster without circuit_breakers", name)
		}
	}
}
