package cluster

import (
	"context"
	"errors"
	"sync"

	"github.com/pgdad/envoy-go/internal/stats"
)

// connPool is the per-cluster DEFAULT-priority connection-creation budget +
// bounded pending wait-queue (ADR-0252). All state transitions happen under mu
// (single-mutator discipline; the 42.2b -race precedent); the only out-of-lock
// operation is the buffered cap-1 wake send, which never blocks. Built for every
// cluster WITH circuit_breakers; effectively off at the 1024 defaults.
//
// Fields are added incrementally across tasks:
//   - Task 2: budget limits (this file)
//   - Task 3: mu, activeConns, waiters (wait-queue primitive) + stat handles
//     (the methods read them via the nil-guarded helpers)
//   - Task 4: the stat handles are assigned by registerStats (nil until then;
//     the helpers guard the nil)
type connPool struct {
	maxConnections     int64
	maxPendingRequests int64

	mu          sync.Mutex
	activeConns int64
	waiters     []chan struct{}

	// Stat handles. Assigned in Task 4 (registerStats); nil until then, which
	// the setGauge/incGauge/decGauge/incCounter helpers guard so the primitive
	// tests can run without a registry (or inject test handles).
	cxOpen                    *stats.Gauge   // <cluster>.cx_open (at-cap soft signal)
	rqPendingOpen             *stats.Gauge   // <cluster>.rq_pending_open (queue-full soft signal)
	upstreamRqPendingActive   *stats.Gauge   // <cluster>.upstream_rq_pending_active
	upstreamCxOverflow        *stats.Counter // <cluster>.upstream_cx_overflow
	upstreamRqPendingTotal    *stats.Counter // <cluster>.upstream_rq_pending_total
	upstreamRqPendingOverflow *stats.Counter // <cluster>.upstream_rq_pending_overflow
}

// errConnPoolOverflow is returned by acquireConnOrPend when the pending wait-
// queue is full (max_pending_requests reached). Distinct from a real dial error
// so the router routes it to a fail-fast 503 (not 502) WITHOUT double-counting
// the upstream status class. (ADR-0252)
var errConnPoolOverflow = errors.New("cluster: connection pool: max_pending_requests overflow")

// IsConnPoolOverflow reports whether err is the connection-pool wait-queue
// overflow sentinel (checked via errors.Is across the DialH2 "%w" wrap).
// Exported additive predicate — no existing signature changes. (ADR-0252)
func IsConnPoolOverflow(err error) bool { return errors.Is(err, errConnPoolOverflow) }

// acquireConnOrPend reserves a connection-creation permit for one new upstream
// connection. Returns:
//   - nil                 → a permit is held; the caller MUST dial and later pair
//     it with exactly one releaseConn (via the connWithGauge
//     dec closure, or directly on a post-acquire failure).
//   - errConnPoolOverflow → the wait-queue is full; the caller fails fast (503).
//   - ctx.Err()           → ctx canceled/expired while pending; no permit held.
//
// (ADR-0252, D-S431-1)
func (p *connPool) acquireConnOrPend(ctx context.Context) error {
	p.mu.Lock()
	if p.activeConns < p.maxConnections {
		p.activeConns++
		if p.activeConns >= p.maxConnections {
			// crossed to the cap → raise the soft signal (cx_open)
			setGauge(p.cxOpen, 1)
		}
		p.mu.Unlock()
		return nil
	}
	// Cap reached: the soft-signal parity (AMEND-CP1) — cx_open + upstream_cx_overflow.
	setGauge(p.cxOpen, 1)
	incCounter(p.upstreamCxOverflow)
	if int64(len(p.waiters)) >= p.maxPendingRequests {
		p.mu.Unlock()
		incCounter(p.upstreamRqPendingOverflow)
		return errConnPoolOverflow
	}
	ch := make(chan struct{}, 1)
	p.waiters = append(p.waiters, ch)
	incGauge(p.upstreamRqPendingActive)
	incCounter(p.upstreamRqPendingTotal)
	if int64(len(p.waiters)) >= p.maxPendingRequests {
		setGauge(p.rqPendingOpen, 1)
	}
	p.mu.Unlock()

	select {
	case <-ch:
		// A reserved permit was handed off (activeConns already accounts for us).
		return nil
	case <-ctx.Done():
		p.mu.Lock()
		if p.removeWaiterLocked(ch) {
			// Still queued → canceled cleanly; no permit was handed off.
			decGauge(p.upstreamRqPendingActive)
			p.clearRqPendingOpenLocked()
			p.mu.Unlock()
			return ctx.Err()
		}
		p.mu.Unlock()
		// Already dequeued → a permit is en route on the buffered channel. Drain
		// it and give it back so it is not leaked (may wake the next waiter).
		<-ch
		p.releaseConn()
		return ctx.Err()
	}
}

// tryAcquireConnSlot reserves a connection-creation permit WITHOUT blocking or
// enqueuing. Returns true with a permit held (the caller MUST pair it with
// exactly one releaseConn) when under max_connections; false (no permit, no
// waiter appended) when at the cap. The H2 pool uses this instead of
// acquireConnOrPend so an H2 request never lands in connPool.waiters (whose
// wake fires only on conn-close) — H2 waiters live in the stream-aware queue
// in h2pool.go and are woken by h2PromoteLocked. Mirrors acquireConnOrPend's
// cap-crossing soft-signal parity (cx_open + upstream_cx_overflow; AMEND-CP1).
// (phase 43.2a, ADR-0253)
func (p *connPool) tryAcquireConnSlot() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.activeConns < p.maxConnections {
		p.activeConns++
		if p.activeConns >= p.maxConnections {
			setGauge(p.cxOpen, 1)
		}
		return true
	}
	setGauge(p.cxOpen, 1)
	incCounter(p.upstreamCxOverflow)
	return false
}

// releaseConn returns one connection-creation permit. With a queued waiter the
// permit is handed off directly (FIFO) — activeConns is UNCHANGED (the freed
// permit is re-reserved for the waiter; cx_open stays set). With no waiter,
// activeConns-- and cx_open clears when back under the cap. Called from the
// connWithGauge dec closure on conn Close + the ctx-cancel give-back. (ADR-0252)
func (p *connPool) releaseConn() {
	p.mu.Lock()
	if len(p.waiters) > 0 {
		ch := p.waiters[0]
		p.waiters = p.waiters[1:]
		decGauge(p.upstreamRqPendingActive)
		p.clearRqPendingOpenLocked()
		p.mu.Unlock()
		ch <- struct{}{} // buffered cap 1 → never blocks
		return
	}
	p.activeConns--
	if p.activeConns < p.maxConnections {
		setGauge(p.cxOpen, 0)
	}
	p.mu.Unlock()
}

// removeWaiterLocked drops ch from the FIFO if present; reports whether found.
// Caller holds mu. (ADR-0252)
func (p *connPool) removeWaiterLocked(ch chan struct{}) bool {
	for i, w := range p.waiters {
		if w == ch {
			p.waiters = append(p.waiters[:i], p.waiters[i+1:]...)
			return true
		}
	}
	return false
}

// clearRqPendingOpenLocked clears rq_pending_open when the queue is back under
// the cap. Caller holds mu. (ADR-0252)
func (p *connPool) clearRqPendingOpenLocked() {
	if int64(len(p.waiters)) < p.maxPendingRequests {
		setGauge(p.rqPendingOpen, 0)
	}
}

// hasWaiter reports whether a request is currently pending. Used by PutIdleH1 to
// choose close-and-wake vs pool. Racy by nature (the truth can change after the
// lock drops) but only a missed-pooling optimization is at stake, never
// correctness. (ADR-0252, D-S431-1)
func (p *connPool) hasWaiter() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.waiters) > 0
}

// Nil-guarded stat helpers: the connPool methods drive the stat transitions
// through these so the primitive tests can run without a registry (the handles
// are nil until Task 4's registerStats assigns them). The real stats API:
// *stats.Gauge has Set(int64)/Inc()/Dec(); *stats.Counter has Inc() (verified
// against internal/stats/gauge.go + counter.go).
func setGauge(g *stats.Gauge, v int64) {
	if g != nil {
		g.Set(v)
	}
}

func incGauge(g *stats.Gauge) {
	if g != nil {
		g.Inc()
	}
}

func decGauge(g *stats.Gauge) {
	if g != nil {
		g.Dec()
	}
}

func incCounter(c *stats.Counter) {
	if c != nil {
		c.Inc()
	}
}
