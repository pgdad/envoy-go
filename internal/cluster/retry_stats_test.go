package cluster

import (
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// retryStatNames returns the 5 fully-qualified stat names that EnsureRetryStats
// allocates for a cluster named "rt_stats" (ADR-0249): the four Inc-driven retry
// counters plus the emit-0 backoff_ratelimited counter.
func retryStatNames() []string {
	const p = "cluster.rt_stats."
	return []string{
		p + "upstream_rq_retry",
		p + "upstream_rq_retry_success",
		p + "upstream_rq_retry_limit_exceeded",
		p + "upstream_rq_retry_backoff_exponential",
		p + "upstream_rq_retry_backoff_ratelimited",
	}
}

// TestEnsureRetryStats_Present asserts a cluster that has been through
// registerClusterMetrics and then EnsureRetryStats() registers EXACTLY the 5
// upstream_rq_retry* counters.
func TestEnsureRetryStats_Present(t *testing.T) {
	c := mkStaticCluster("rt_stats", mkLbEndpoint("127.0.0.1", 9800))
	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(c), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, ok := m.Get("rt_stats")
	if !ok {
		t.Fatal("rt_stats not found")
	}
	cl.EnsureRetryStats()
	if cl.retry == nil {
		t.Fatal("retry stats must be built after EnsureRetryStats()")
	}
	for _, name := range retryStatNames() {
		if !hasMetric(reg, name) {
			t.Errorf("expected metric %q to be registered", name)
		}
	}
}

// TestEnsureRetryStats_Absent asserts a cluster WITHOUT EnsureRetryStats()
// registers NONE of the 5 retry stat names (byte-stability of every existing
// non-retry fixture's /stats; the scoped-registration departure of ADR-0249).
func TestEnsureRetryStats_Absent(t *testing.T) {
	c := mkStaticCluster("rt_stats", mkLbEndpoint("127.0.0.1", 9810))
	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(c), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, ok := m.Get("rt_stats")
	if !ok {
		t.Fatal("rt_stats not found")
	}
	if cl.retry != nil {
		t.Fatal("retry stats must be nil before EnsureRetryStats()")
	}
	for _, name := range retryStatNames() {
		if hasMetric(reg, name) {
			t.Errorf("metric %q must NOT be registered for a cluster without EnsureRetryStats()", name)
		}
	}
}

// TestEnsureRetryStats_Idempotent asserts calling EnsureRetryStats() twice
// registers the 5 counters exactly once (same handles, no duplicate-registration
// panic from the registry). Config build is single-threaded, so a plain != nil
// guard is sufficient.
func TestEnsureRetryStats_Idempotent(t *testing.T) {
	c := mkStaticCluster("rt_stats", mkLbEndpoint("127.0.0.1", 9820))
	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(c), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, ok := m.Get("rt_stats")
	if !ok {
		t.Fatal("rt_stats not found")
	}
	cl.EnsureRetryStats()
	first := cl.retry
	// A second call must NOT panic (NewCounter panics on duplicate names) and
	// must leave the handles unchanged.
	cl.EnsureRetryStats()
	if cl.retry != first {
		t.Fatal("EnsureRetryStats() twice replaced the retryStats handles; want idempotent")
	}
	// Exactly one registration of each name: Inc once through the public method,
	// read back the single live handle, expect 1.
	cl.IncUpstreamRqRetry()
	if got, _ := counterValue(reg, "cluster.rt_stats.upstream_rq_retry"); got != 1 {
		t.Errorf("upstream_rq_retry = %d after one Inc, want 1 (a duplicate registration would split the handle)", got)
	}
}
