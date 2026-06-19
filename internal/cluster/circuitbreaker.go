package cluster

import (
	"fmt"
	"sync/atomic"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	"github.com/esalaine/envoy-go/internal/stats"
)

// circuitBreaker holds per-RoutingPriority concurrency budgets + live counters.
// Nil on Cluster when no circuit_breakers is configured (TryAcquireRequest is a
// pass-through). Phase 41 enforces max_requests only (ADR-0248, AMEND-CB1);
// max_connections/max_pending_requests register-for-parity-but-defer.
type circuitBreaker struct {
	prio [2]cbPriority // [0]=DEFAULT, [1]=HIGH (HIGH parses+registers but never binds)
	// the LIVE cluster-level overflow counter (the SAME counter max_pending_requests
	// would use — there is NO upstream_rq_overflow; AMEND-CB2).
	upstreamRqPendingOverflow *stats.Counter
}

type cbPriority struct {
	maxRequests    int64        // the enforced budget (default 1024 — AMEND-CB5)
	activeRequests atomic.Int64 // in-flight DEFAULT-priority requests
	rqOpen         *stats.Gauge // 1 while activeRequests >= maxRequests (LIVE for DEFAULT)
}

// parseCircuitBreakers builds the per-priority budgets from Cluster.circuit_breakers.
// Returns (nil, nil) when absent. Phase 41: max_requests parsed + enforced; the
// other budgets parse-accepted (defaults applied) but enforcement deferred. (ADR-0248)
func parseCircuitBreakers(c *clusterv3.Cluster, name string) (*circuitBreaker, error) {
	cb := c.GetCircuitBreakers()
	if cb == nil {
		return nil, nil
	}
	out := &circuitBreaker{}
	out.prio[0].maxRequests = 1024
	out.prio[1].maxRequests = 1024
	seen := [2]bool{}
	for _, th := range cb.GetThresholds() {
		p := th.GetPriority()
		if p != corev3.RoutingPriority_DEFAULT && p != corev3.RoutingPriority_HIGH {
			return nil, fmt.Errorf("cluster: %q: circuit_breakers: priority: value must be one of the defined enum values", name)
		}
		idx := int(p) // DEFAULT=0, HIGH=1
		if seen[idx] {
			return nil, fmt.Errorf("cluster: %q: circuit_breakers: duplicate threshold priority", name)
		}
		seen[idx] = true
		if rb := th.GetRetryBudget(); rb != nil && rb.GetBudgetPercent() != nil && rb.GetBudgetPercent().GetValue() > 100 {
			return nil, fmt.Errorf("cluster: %q: circuit_breakers: retry_budget: budget_percent: value must be inside range [0, 100]", name)
		}
		if v := th.GetMaxRequests(); v != nil {
			out.prio[idx].maxRequests = int64(v.GetValue())
		}
		// max_connections/max_pending_requests/max_retries/max_connection_pools:
		// parse-accepted, enforcement DEFERRED (AMEND-CB1) — no fields stored.
	}
	// per_host_thresholds: silent-ignored (AMEND-CB1).
	return out, nil
}

// registerStats allocates the +14 circuit_breakers stats (SPEC §7), on clusters
// WITH circuit_breakers only. Per priority (default, high): 5 *_open gauges. Only
// default.rq_open binds a LIVE handle (the enforced max_requests gauge); high's
// rq_open + all cx/cx_pool/rq_pending/rq_retry gauges register-and-stay-at-0
// (their budgets parse-but-defer per AMEND-CB1). The 4 cluster overflow counters:
// only upstream_rq_pending_overflow binds LIVE (AMEND-CB2); the other 3 emit-0.
func (cb *circuitBreaker) registerStats(r *stats.Registry, prefix string) {
	for idx, name := range []string{"default", "high"} {
		gp := prefix + "circuit_breakers." + name + "."
		cb.prio[idx].rqOpen = r.NewGauge(gp + "rq_open") // LIVE for default; high registered but never set
		r.NewGauge(gp + "cx_open")                       // emit-0 (max_connections deferred)
		r.NewGauge(gp + "cx_pool_open")                  // emit-0 (max_connection_pools deferred)
		r.NewGauge(gp + "rq_pending_open")               // emit-0 (max_pending_requests deferred)
		r.NewGauge(gp + "rq_retry_open")                 // emit-0 (max_retries deferred)
	}
	cb.upstreamRqPendingOverflow = r.NewCounter(prefix + "upstream_rq_pending_overflow") // LIVE
	r.NewCounter(prefix + "upstream_cx_overflow")                                        // emit-0
	r.NewCounter(prefix + "upstream_cx_pool_overflow")                                   // emit-0
	r.NewCounter(prefix + "upstream_rq_retry_overflow")                                  // emit-0
}

// tryAcquire reserves a max_requests slot for the given priority. Returns false
// (overflow) when the budget is exhausted, incrementing the overflow counter +
// setting rq_open. (ADR-0248)
func (cb *circuitBreaker) tryAcquire(prio int) bool {
	p := &cb.prio[prio]
	for {
		cur := p.activeRequests.Load()
		if cur >= p.maxRequests {
			if cb.upstreamRqPendingOverflow != nil {
				cb.upstreamRqPendingOverflow.Inc()
			}
			if p.rqOpen != nil {
				p.rqOpen.Set(1)
			}
			return false
		}
		if p.activeRequests.CompareAndSwap(cur, cur+1) {
			if cur+1 >= p.maxRequests && p.rqOpen != nil {
				p.rqOpen.Set(1)
			}
			return true
		}
	}
}

// release returns a slot; clears rq_open when back under budget. (ADR-0248)
func (cb *circuitBreaker) release(prio int) {
	p := &cb.prio[prio]
	if p.activeRequests.Add(-1) < p.maxRequests && p.rqOpen != nil {
		p.rqOpen.Set(0)
	}
}
