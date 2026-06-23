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

// TestCircuitBreakerConcurrentRetryNeverExceedsBudget is the retry-budget twin of
// the phase-41 max_requests concurrency test: under heavy contention the peak
// concurrent retry holders must NEVER exceed the cap. budget_percent:20 with a
// FIXED prio[0].activeRequests=100 stored up front makes the cap deterministic:
// max(minRetryConcurrency=3, ceil(20% × 100)) = max(3,20) = 20. (ADR-0249)
// activeRequests is held constant for the whole test so the cap can be asserted
// against; the CAS loop's never-exceed property must hold under -race.
func TestCircuitBreakerConcurrentRetryNeverExceedsBudget(t *testing.T) {
	reg := stats.NewRegistry()
	cb := &circuitBreaker{}
	cb.hasRetryBudget = true
	cb.budgetPercent = 20
	cb.minRetryConcurrency = 3
	cb.prio[0].rqRetryOpen = reg.NewGauge("test.rq_retry_open")
	cb.upstreamRqRetryOverflow = reg.NewCounter("test.retry_overflow")
	cb.prio[0].activeRequests.Store(100) // fixed ⇒ deterministic cap = 20

	const cap = 20
	var holders atomic.Int64
	var peak atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				if cb.tryAcquireRetry() {
					cur := holders.Add(1)
					for {
						p := peak.Load()
						if cur <= p || peak.CompareAndSwap(p, cur) {
							break
						}
					}
					holders.Add(-1)
					cb.releaseRetry()
				}
			}
		}()
	}
	wg.Wait()

	if got := peak.Load(); got > cap {
		t.Errorf("peak concurrent retry holders = %d, want <= %d", got, cap)
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

// circuitBreakerStatNames returns the 16 fully-qualified stat names that
// registerClusterMetrics allocates for a cluster named "cb_stats" WITH
// circuit_breakers (SPEC §7): the 10 per-priority *_open gauges (default + high,
// each {cx_open, cx_pool_open, rq_open, rq_pending_open, rq_retry_open}) plus the
// 4 cluster overflow counters, plus the 2 new pending-queue stats
// (upstream_rq_pending_active gauge + upstream_rq_pending_total counter).
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
		// AMEND-CP3: 2 new pending-queue stats (the +2 surface delta: 1181→1183).
		p+"upstream_rq_pending_active",
		p+"upstream_rq_pending_total",
	)
	return names
}

// TestRegisterCircuitBreakerStats_Present asserts a cluster WITH circuit_breakers
// registers EXACTLY the 16 circuit_breakers stat names (Task 4: 14 original + 2
// new pending-queue names) and binds the LIVE handles: default.rq_open gauge,
// upstream_rq_pending_overflow counter, plus the 6 activated/new pool handles
// (the 6th, pool.upstreamRqPendingOverflow, is the shared cb handle).
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
	// The two original LIVE handles must be injected (non-nil).
	if cl.circuitBreaker.prio[0].rqOpen == nil {
		t.Error("default.rq_open gauge handle must be injected (non-nil)")
	}
	if cl.circuitBreaker.upstreamRqPendingOverflow == nil {
		t.Error("upstream_rq_pending_overflow counter handle must be injected (non-nil)")
	}
	// Task 4: the 6 activated + new pool handles must also be non-nil.
	if cl.circuitBreaker.pool.cxOpen == nil {
		t.Error("pool.cxOpen gauge handle must be injected (non-nil)")
	}
	if cl.circuitBreaker.pool.rqPendingOpen == nil {
		t.Error("pool.rqPendingOpen gauge handle must be injected (non-nil)")
	}
	if cl.circuitBreaker.pool.upstreamCxOverflow == nil {
		t.Error("pool.upstreamCxOverflow counter handle must be injected (non-nil)")
	}
	if cl.circuitBreaker.pool.upstreamRqPendingActive == nil {
		t.Error("pool.upstreamRqPendingActive gauge handle must be injected (non-nil)")
	}
	if cl.circuitBreaker.pool.upstreamRqPendingTotal == nil {
		t.Error("pool.upstreamRqPendingTotal counter handle must be injected (non-nil)")
	}
	if cl.circuitBreaker.pool.upstreamRqPendingOverflow == nil {
		t.Error("pool.upstreamRqPendingOverflow counter handle must be injected (non-nil, shared with cb)")
	}
}

// TestRegisterCircuitBreakerStats_Absent asserts a cluster WITHOUT circuit_breakers
// registers NONE of the 16 circuit_breakers stat names (including the 2 new
// pending-queue names added in Task 4).
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

// TestCircuitBreakerTryAcquireRetryNoBudget: with no retry_budget configured,
// tryAcquireRetry is an unconditional no-op true (and releaseRetry never panics).
func TestCircuitBreakerTryAcquireRetryNoBudget(t *testing.T) {
	cb := &circuitBreaker{}
	for i := 0; i < 5; i++ {
		if !cb.tryAcquireRetry() {
			t.Fatalf("tryAcquireRetry #%d with no retry_budget: got false, want true (no-op)", i)
		}
	}
	cb.releaseRetry() // no-op; must not panic
}

// TestCircuitBreakerRetryBudgetOverflow: retry_budget{budget_percent:0,
// min_retry_concurrency:1} ⇒ cap=1. First acquire true; second (no release) false
// + overflow counter == 1 + rq_retry_open == 1; after releaseRetry the next is
// true + rq_retry_open == 0.
func TestCircuitBreakerRetryBudgetOverflow(t *testing.T) {
	reg := stats.NewRegistry()
	cb := &circuitBreaker{}
	cb.hasRetryBudget = true
	cb.budgetPercent = 0
	cb.minRetryConcurrency = 1
	cb.prio[0].rqRetryOpen = reg.NewGauge("test.rq_retry_open")
	cb.upstreamRqRetryOverflow = reg.NewCounter("test.retry_overflow")

	if !cb.tryAcquireRetry() {
		t.Fatal("1st tryAcquireRetry: got false, want true")
	}
	if cb.tryAcquireRetry() {
		t.Fatal("2nd tryAcquireRetry (no release): got true, want false (overflow)")
	}
	if got := cb.upstreamRqRetryOverflow.Load(); got != 1 {
		t.Errorf("upstreamRqRetryOverflow = %d, want 1 after overflow", got)
	}
	if got := cb.prio[0].rqRetryOpen.Load(); got != 1 {
		t.Errorf("rqRetryOpen = %d, want 1 after overflow", got)
	}

	cb.releaseRetry()
	if got := cb.prio[0].rqRetryOpen.Load(); got != 0 {
		t.Errorf("rqRetryOpen = %d, want 0 after releaseRetry (back under min)", got)
	}
	if !cb.tryAcquireRetry() {
		t.Error("tryAcquireRetry after release: got false, want true")
	}
}

// TestCircuitBreakerRetryBudgetCeil: budget_percent:20, min_retry_concurrency:3
// with prio[0].activeRequests=100 ⇒ cap = max(3, ceil(20% × 100)) = max(3,20) = 20.
// 20 acquires succeed; the 21st overflows.
func TestCircuitBreakerRetryBudgetCeil(t *testing.T) {
	reg := stats.NewRegistry()
	cb := &circuitBreaker{}
	cb.hasRetryBudget = true
	cb.budgetPercent = 20
	cb.minRetryConcurrency = 3
	cb.prio[0].rqRetryOpen = reg.NewGauge("test.rq_retry_open")
	cb.upstreamRqRetryOverflow = reg.NewCounter("test.retry_overflow")
	cb.prio[0].activeRequests.Store(100)

	for i := 0; i < 20; i++ {
		if !cb.tryAcquireRetry() {
			t.Fatalf("tryAcquireRetry #%d: got false, want true (cap=20)", i)
		}
	}
	if cb.tryAcquireRetry() {
		t.Fatal("21st tryAcquireRetry: got true, want false (cap=20 exhausted)")
	}
	if got := cb.activeRetries.Load(); got != 20 {
		t.Errorf("activeRetries = %d, want 20", got)
	}
	// The overflow set rq_retry_open at the DYNAMIC cap (20).
	if got := cb.prio[0].rqRetryOpen.Load(); got != 1 {
		t.Errorf("rqRetryOpen = %d, want 1 after overflow at cap=20", got)
	}
	// Asymmetric soft-budget gauge: set-at-cap / clear-below-min. One release drops
	// 20→19, still >= minRetryConcurrency=3, so rq_retry_open must STAY 1 (it only
	// clears once activeRetries falls below the min floor, not below the dynamic cap).
	cb.releaseRetry()
	if got := cb.prio[0].rqRetryOpen.Load(); got != 1 {
		t.Errorf("rqRetryOpen = %d, want 1 after one release (19 >= min=3, still open)", got)
	}
}

// TestParseCircuitBreakers_RetryBudgetDefaults: retry_budget absent ⇒
// hasRetryBudget == false; present (empty) ⇒ defaults 20 / 3 applied (DEFAULT only).
func TestParseCircuitBreakers_RetryBudgetDefaults(t *testing.T) {
	// absent
	cAbsent := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{Priority: corev3.RoutingPriority_DEFAULT, MaxRequests: wrapperspb.UInt32(4)},
			},
		},
	}
	cbAbsent, err := parseCircuitBreakers(cAbsent, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cbAbsent.hasRetryBudget {
		t.Error("hasRetryBudget = true, want false when retry_budget absent")
	}

	// present, empty ⇒ defaults
	cPresent := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{
					Priority:    corev3.RoutingPriority_DEFAULT,
					RetryBudget: &clusterv3.CircuitBreakers_Thresholds_RetryBudget{},
				},
			},
		},
	}
	cbPresent, err := parseCircuitBreakers(cPresent, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cbPresent.hasRetryBudget {
		t.Fatal("hasRetryBudget = false, want true when retry_budget present")
	}
	if cbPresent.budgetPercent != 20 {
		t.Errorf("budgetPercent = %v, want 20 (default)", cbPresent.budgetPercent)
	}
	if cbPresent.minRetryConcurrency != 3 {
		t.Errorf("minRetryConcurrency = %d, want 3 (default)", cbPresent.minRetryConcurrency)
	}
}

// ---------------------------------------------------------------------------
// Task 2: connPool parsing tests (max_connections / max_pending_requests)
// ---------------------------------------------------------------------------

// TestParseCircuitBreakers_ConnPoolBudgets: DEFAULT threshold with
// max_connections:2 / max_pending_requests:3 ⇒ pool fields set accordingly.
func TestParseCircuitBreakers_ConnPoolBudgets(t *testing.T) {
	c := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{
					Priority:           corev3.RoutingPriority_DEFAULT,
					MaxConnections:     wrapperspb.UInt32(2),
					MaxPendingRequests: wrapperspb.UInt32(3),
				},
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
	if cb.pool == nil {
		t.Fatal("cb.pool must be non-nil for a cluster with circuit_breakers")
	}
	if cb.pool.maxConnections != 2 {
		t.Errorf("pool.maxConnections = %d, want 2", cb.pool.maxConnections)
	}
	if cb.pool.maxPendingRequests != 3 {
		t.Errorf("pool.maxPendingRequests = %d, want 3", cb.pool.maxPendingRequests)
	}
}

// TestParseCircuitBreakers_ConnPoolDefaults1024: absent budgets ⇒ both default
// to 1024 (AMEND-CP5).
func TestParseCircuitBreakers_ConnPoolDefaults1024(t *testing.T) {
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
	if cb.pool == nil {
		t.Fatal("cb.pool must be non-nil")
	}
	if cb.pool.maxConnections != 1024 {
		t.Errorf("pool.maxConnections = %d, want 1024 (default)", cb.pool.maxConnections)
	}
	if cb.pool.maxPendingRequests != 1024 {
		t.Errorf("pool.maxPendingRequests = %d, want 1024 (default)", cb.pool.maxPendingRequests)
	}
}

// TestParseCircuitBreakers_ConnPoolExplicitZero: max_connections:0 ⇒
// pool.maxConnections == 0 (explicit zero overrides the default).
func TestParseCircuitBreakers_ConnPoolExplicitZero(t *testing.T) {
	c := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{
					Priority:       corev3.RoutingPriority_DEFAULT,
					MaxConnections: wrapperspb.UInt32(0),
				},
			},
		},
	}
	cb, err := parseCircuitBreakers(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb.pool == nil {
		t.Fatal("cb.pool must be non-nil")
	}
	if cb.pool.maxConnections != 0 {
		t.Errorf("pool.maxConnections = %d, want 0 (explicit zero)", cb.pool.maxConnections)
	}
}

// TestParseCircuitBreakers_ConnPoolNonNil: any cluster WITH circuit_breakers
// must have a non-nil pool.
func TestParseCircuitBreakers_ConnPoolNonNil(t *testing.T) {
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
	if cb.pool == nil {
		t.Error("cb.pool must be non-nil for any cluster with circuit_breakers")
	}
}

// TestParseCircuitBreakers_ConnPoolHighIgnored: HIGH-priority budgets are
// IGNORED by the pool — only DEFAULT binds them. A HIGH threshold with
// max_connections:7 / max_pending_requests:9 + a DEFAULT with max_connections:2 /
// max_pending_requests:3 ⇒ pool.maxConnections == 2 && pool.maxPendingRequests ==
// 3 (proving neither HIGH value bled in; both idx==0 parse branches are covered).
func TestParseCircuitBreakers_ConnPoolHighIgnored(t *testing.T) {
	c := &clusterv3.Cluster{
		CircuitBreakers: &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
				{
					Priority:           corev3.RoutingPriority_DEFAULT,
					MaxConnections:     wrapperspb.UInt32(2),
					MaxPendingRequests: wrapperspb.UInt32(3),
				},
				{
					Priority:           corev3.RoutingPriority_HIGH,
					MaxConnections:     wrapperspb.UInt32(7),
					MaxPendingRequests: wrapperspb.UInt32(9),
				},
			},
		},
	}
	cb, err := parseCircuitBreakers(c, "c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb.pool == nil {
		t.Fatal("cb.pool must be non-nil")
	}
	if cb.pool.maxConnections != 2 {
		t.Errorf("pool.maxConnections = %d, want 2 (HIGH budget ignored)", cb.pool.maxConnections)
	}
	if cb.pool.maxPendingRequests != 3 {
		t.Errorf("pool.maxPendingRequests = %d, want 3 (HIGH budget ignored)", cb.pool.maxPendingRequests)
	}
}

// ---------------------------------------------------------------------------
// End Task 2 tests
// ---------------------------------------------------------------------------

// TestClusterTryAcquireReleaseRetry exercises the Cluster-level nil-guards:
// no circuitBreaker ⇒ always true / no-op; with a budget ⇒ delegates.
func TestClusterTryAcquireReleaseRetry(t *testing.T) {
	cl := &Cluster{name: "no_cb"}
	if !cl.TryAcquireRetry() {
		t.Error("TryAcquireRetry with no circuitBreaker: got false, want true (nil-guard)")
	}
	cl.ReleaseRetry() // no-op; must not panic

	cb := &circuitBreaker{}
	cb.hasRetryBudget = true
	cb.budgetPercent = 0
	cb.minRetryConcurrency = 1
	reg := stats.NewRegistry()
	cb.prio[0].rqRetryOpen = reg.NewGauge("test.rq_retry_open")
	cb.upstreamRqRetryOverflow = reg.NewCounter("test.retry_overflow")
	cl2 := &Cluster{name: "cb", circuitBreaker: cb}
	if !cl2.TryAcquireRetry() {
		t.Error("1st TryAcquireRetry: got false, want true")
	}
	if cl2.TryAcquireRetry() {
		t.Error("2nd TryAcquireRetry: got true, want false (cap=1)")
	}
	cl2.ReleaseRetry()
	if !cl2.TryAcquireRetry() {
		t.Error("TryAcquireRetry after release: got false, want true")
	}
}
