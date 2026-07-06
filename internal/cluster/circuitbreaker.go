package cluster

import (
	"fmt"
	"sync/atomic"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	"github.com/pgdad/envoy-go/internal/stats"
)

// circuitBreaker holds per-RoutingPriority concurrency budgets + live counters.
// Nil on Cluster when no circuit_breakers is configured (TryAcquireRequest is a
// pass-through). Phase 41 enforces max_requests only (ADR-0248, AMEND-CB1);
// max_connections/max_pending_requests register-for-parity-but-defer.
type circuitBreaker struct {
	prio [2]cbPriority // [0]=DEFAULT, [1]=HIGH (HIGH parses+registers but never binds)
	// pool is the DEFAULT-priority connection-creation budget + bounded pending
	// wait-queue (ADR-0252). Non-nil for every cluster WITH circuit_breakers
	// (the budgets default to 1024 — AMEND-CP5, so it is effectively off unless
	// configured small). DEFAULT-only: every request is DEFAULT at 43.1.
	pool *connPool
	// the LIVE cluster-level overflow counter (the SAME counter max_pending_requests
	// would use — there is NO upstream_rq_overflow; AMEND-CB2).
	upstreamRqPendingOverflow *stats.Counter
	// retry_budget accounting (DEFAULT-only — ADR-0249, AMEND-RT3). hasRetryBudget
	// gates tryAcquireRetry/releaseRetry; the cap is max(minRetryConcurrency,
	// ⌈budgetPercent% × prio[0].activeRequests⌉).
	upstreamRqRetryOverflow *stats.Counter // LIVE when a retry_budget is configured
	activeRetries           atomic.Int64   // in-flight DEFAULT-priority retries
	budgetPercent           float64        // retry-budget percent of active requests (default 20)
	minRetryConcurrency     int64          // retry-budget floor (default 3)
	hasRetryBudget          bool           // true when the DEFAULT threshold has a retry_budget
}

type cbPriority struct {
	maxRequests    int64        // the enforced budget (default 1024 — AMEND-CB5)
	activeRequests atomic.Int64 // in-flight DEFAULT-priority requests
	rqOpen         *stats.Gauge // 1 while activeRequests >= maxRequests (LIVE for DEFAULT)
	rqRetryOpen    *stats.Gauge // 1 while activeRetries >= retry-budget cap (LIVE for DEFAULT)
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
	maxConns := int64(1024)   // AMEND-CP5 default
	maxPending := int64(1024) // AMEND-CP5 default
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
		if rb := th.GetRetryBudget(); rb != nil {
			if rb.GetBudgetPercent() != nil && rb.GetBudgetPercent().GetValue() > 100 {
				return nil, fmt.Errorf("cluster: %q: circuit_breakers: retry_budget: budget_percent: value must be inside range [0, 100]", name)
			}
			if idx == 0 { // retry_budget binds DEFAULT-only (ADR-0249, AMEND-RT3)
				out.hasRetryBudget = true
				out.budgetPercent = 20 // default
				if bp := rb.GetBudgetPercent(); bp != nil {
					out.budgetPercent = bp.GetValue()
				}
				out.minRetryConcurrency = 3 // default
				if mrc := rb.GetMinRetryConcurrency(); mrc != nil {
					out.minRetryConcurrency = int64(mrc.GetValue())
				}
			}
		}
		if v := th.GetMaxRequests(); v != nil {
			out.prio[idx].maxRequests = int64(v.GetValue())
		}
		if idx == 0 { // DEFAULT-only: the connection budget binds DEFAULT (43.1)
			if v := th.GetMaxConnections(); v != nil {
				maxConns = int64(v.GetValue())
			}
			if v := th.GetMaxPendingRequests(); v != nil {
				maxPending = int64(v.GetValue())
			}
		}
	}
	out.pool = &connPool{maxConnections: maxConns, maxPendingRequests: maxPending}
	// per_host_thresholds: silent-ignored (AMEND-CB1).
	return out, nil
}

// registerStats allocates the +16 circuit_breakers stats (SPEC §7, AMEND-CP3),
// on clusters WITH circuit_breakers only. Per priority (default, high): 5 *_open
// gauges. default.rq_open + the 2 DEFAULT-only pool open-signal handles (cx_open,
// rq_pending_open) bind LIVE; rq_retry_open binds LIVE on DEFAULT via prio[0].
// high's gauges register-and-stay-at-0 (their budgets parse-but-defer per
// AMEND-CB1). Cluster-scoped counters: upstream_rq_pending_overflow binds LIVE
// (AMEND-CB2) and is shared into cb.pool; the activated upstream_cx_overflow plus
// the 2 new pending-queue names (upstream_rq_pending_active gauge +
// upstream_rq_pending_total counter) bind LIVE via cb.pool (AMEND-CP2/CP3).
// cb.pool is always non-nil here (Task 2 guarantee).
func (cb *circuitBreaker) registerStats(r *stats.Registry, prefix string) {
	for idx, name := range []string{"default", "high"} {
		gp := prefix + "circuit_breakers." + name + "."
		cb.prio[idx].rqOpen = r.NewGauge(gp + "rq_open")
		cxOpen := r.NewGauge(gp + "cx_open")                // ACTIVATED (was emit-0)
		r.NewGauge(gp + "cx_pool_open")                     // emit-0 (max_connection_pools deferred)
		rqPendingOpen := r.NewGauge(gp + "rq_pending_open") // ACTIVATED (was emit-0)
		g := r.NewGauge(gp + "rq_retry_open")
		if idx == 0 { // DEFAULT-only live handles
			// cx_open + rq_pending_open are pool open-signals → cb.pool.*
			cb.pool.cxOpen = cxOpen
			cb.pool.rqPendingOpen = rqPendingOpen
			// rq_retry_open is a per-priority retry-budget signal → cb.prio[0].*
			cb.prio[0].rqRetryOpen = g
		}
	}
	cb.upstreamRqPendingOverflow = r.NewCounter(prefix + "upstream_rq_pending_overflow") // LIVE
	cb.pool.upstreamRqPendingOverflow = cb.upstreamRqPendingOverflow                     // shared (AMEND-CP2)
	cb.pool.upstreamCxOverflow = r.NewCounter(prefix + "upstream_cx_overflow")           // ACTIVATED (was emit-0)
	r.NewCounter(prefix + "upstream_cx_pool_overflow")                                   // emit-0
	cb.upstreamRqRetryOverflow = r.NewCounter(prefix + "upstream_rq_retry_overflow")     // LIVE (phase 42)
	// AMEND-CP3: 2 NEW CB-scoped pending-queue names (the +2 surface delta: 1181→1183).
	cb.pool.upstreamRqPendingActive = r.NewGauge(prefix + "upstream_rq_pending_active")
	cb.pool.upstreamRqPendingTotal = r.NewCounter(prefix + "upstream_rq_pending_total")
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

// tryAcquireRetry reserves a retry-budget slot (DEFAULT). Returns false (overflow)
// when active retries reach max(minRetryConcurrency, ⌈budgetPercent% × activeRequests⌉).
// No-op true when no retry_budget configured. (ADR-0249, AMEND-RT3)
func (cb *circuitBreaker) tryAcquireRetry() bool {
	if !cb.hasRetryBudget {
		return true
	}
	for {
		// Soft/advisory cap: activeRequests is re-read each iteration and may shift
		// between this cap-read and a successful CAS below — these are the reference's
		// advisory retry-budget semantics, not a bug.
		active := cb.prio[0].activeRequests.Load()
		limit := cb.minRetryConcurrency
		if byPct := int64((cb.budgetPercent*float64(active) + 99) / 100); byPct > limit { // ⌈pct%×active⌉ via the +99)/100 integer-ceil idiom (PLAN-pinned; sub-1% products truncate toward the min floor)
			limit = byPct
		}
		cur := cb.activeRetries.Load()
		if cur >= limit {
			cb.upstreamRqRetryOverflow.Inc()
			if cb.prio[0].rqRetryOpen != nil {
				cb.prio[0].rqRetryOpen.Set(1)
			}
			return false
		}
		if cb.activeRetries.CompareAndSwap(cur, cur+1) {
			return true
		}
	}
}

// releaseRetry returns a retry slot; clears rq_retry_open when back under the
// minRetryConcurrency floor. No-op when no retry_budget configured. (ADR-0249)
func (cb *circuitBreaker) releaseRetry() {
	if !cb.hasRetryBudget {
		return
	}
	if cb.activeRetries.Add(-1) < cb.minRetryConcurrency && cb.prio[0].rqRetryOpen != nil {
		cb.prio[0].rqRetryOpen.Set(0)
	}
}
