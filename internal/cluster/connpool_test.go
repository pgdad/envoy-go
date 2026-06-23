package cluster

import (
	"context"
	"errors"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

// newTestConnPool builds a connPool with real injected stat handles so the
// gauge/counter transitions are asserted (not merely nil-guarded away).
func newTestConnPool(maxConns, maxPending int64) *connPool {
	r := stats.NewRegistry()
	return &connPool{
		maxConnections:            maxConns,
		maxPendingRequests:        maxPending,
		cxOpen:                    r.NewGauge("cx_open"),
		rqPendingOpen:             r.NewGauge("rq_pending_open"),
		upstreamRqPendingActive:   r.NewGauge("upstream_rq_pending_active"),
		upstreamCxOverflow:        r.NewCounter("upstream_cx_overflow"),
		upstreamRqPendingTotal:    r.NewCounter("upstream_rq_pending_total"),
		upstreamRqPendingOverflow: r.NewCounter("upstream_rq_pending_overflow"),
	}
}

// pollUntil spins until cond() or the deadline, failing the test on timeout.
func pollUntil(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pollUntil timed out: %s", msg)
}

// (a) acquire under cap.
func TestConnPoolAcquireUnderCap(t *testing.T) {
	p := newTestConnPool(2, 0)
	ctx := context.Background()

	if err := p.acquireConnOrPend(ctx); err != nil {
		t.Fatalf("acquire #1: unexpected err %v", err)
	}
	if got := p.cxOpen.Load(); got != 0 {
		t.Fatalf("after acquire #1: cxOpen = %d, want 0 (under cap)", got)
	}
	if err := p.acquireConnOrPend(ctx); err != nil {
		t.Fatalf("acquire #2: unexpected err %v", err)
	}
	if got := p.activeConns; got != 2 {
		t.Fatalf("activeConns = %d, want 2", got)
	}
	if got := p.cxOpen.Load(); got != 1 {
		t.Fatalf("after acquire #2 (at cap): cxOpen = %d, want 1", got)
	}
}

// (b) release clears cx_open; the next acquire re-sets it.
func TestConnPoolReleaseClearsCxOpen(t *testing.T) {
	p := newTestConnPool(2, 0)
	ctx := context.Background()
	if err := p.acquireConnOrPend(ctx); err != nil {
		t.Fatalf("acquire #1: %v", err)
	}
	if err := p.acquireConnOrPend(ctx); err != nil {
		t.Fatalf("acquire #2: %v", err)
	}
	if p.cxOpen.Load() != 1 {
		t.Fatalf("precondition: cxOpen want 1, got %d", p.cxOpen.Load())
	}

	p.releaseConn()
	if got := p.activeConns; got != 1 {
		t.Fatalf("after release: activeConns = %d, want 1", got)
	}
	if got := p.cxOpen.Load(); got != 0 {
		t.Fatalf("after release: cxOpen = %d, want 0 (back under cap)", got)
	}

	if err := p.acquireConnOrPend(ctx); err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	if got := p.cxOpen.Load(); got != 1 {
		t.Fatalf("after re-acquire (at cap): cxOpen = %d, want 1", got)
	}
}

// (c) pend + wake — the core handoff.
func TestConnPoolPendAndWake(t *testing.T) {
	p := newTestConnPool(1, 1)
	ctx := context.Background()
	if err := p.acquireConnOrPend(ctx); err != nil {
		t.Fatalf("acquire #1: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- p.acquireConnOrPend(ctx) }()

	// #2 pends: queue len 1 >= maxPending 1.
	pollUntil(t, func() bool {
		return p.upstreamRqPendingActive.Load() == 1 && p.rqPendingOpen.Load() == 1
	}, "waiter #2 to enqueue (pendingActive==1 && rqPendingOpen==1)")

	p.releaseConn() // hand the permit directly to #2.

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("woken acquire #2: unexpected err %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("woken acquire #2 did not return")
	}

	if got := p.upstreamRqPendingActive.Load(); got != 0 {
		t.Fatalf("after wake: pendingActive = %d, want 0", got)
	}
	if got := p.rqPendingOpen.Load(); got != 0 {
		t.Fatalf("after wake: rqPendingOpen = %d, want 0", got)
	}
	if got := p.upstreamRqPendingTotal.Load(); got != 1 {
		t.Fatalf("upstreamRqPendingTotal = %d, want 1", got)
	}
	// The permit was handed off; activeConns stays 1.
	if got := p.activeConns; got != 1 {
		t.Fatalf("after handoff: activeConns = %d, want 1", got)
	}
}

// (d) queue-full overflow.
func TestConnPoolQueueFullOverflow(t *testing.T) {
	p := newTestConnPool(1, 1)
	ctx := context.Background()
	if err := p.acquireConnOrPend(ctx); err != nil {
		t.Fatalf("acquire #1: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- p.acquireConnOrPend(ctx) }()
	pollUntil(t, func() bool { return p.upstreamRqPendingActive.Load() == 1 },
		"waiter #2 to fill the queue")

	// #3: queue full → immediate overflow, no block.
	overflowDone := make(chan error, 1)
	go func() { overflowDone <- p.acquireConnOrPend(ctx) }()
	select {
	case err := <-overflowDone:
		if !errors.Is(err, errConnPoolOverflow) {
			t.Fatalf("acquire #3: got %v, want errConnPoolOverflow", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("acquire #3 blocked instead of failing fast")
	}
	if got := p.upstreamRqPendingOverflow.Load(); got != 1 {
		t.Fatalf("upstreamRqPendingOverflow = %d, want 1", got)
	}

	// Release twice → #2 wakes (first release hands off; second drops activeConns).
	p.releaseConn()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("woken acquire #2: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("acquire #2 did not wake")
	}
	p.releaseConn()
	if got := p.activeConns; got != 0 {
		t.Fatalf("after two releases: activeConns = %d, want 0", got)
	}
}

// (e) ctx-cancel while pending — clean removal.
func TestConnPoolCtxCancelCleanRemoval(t *testing.T) {
	p := newTestConnPool(1, 5)
	if err := p.acquireConnOrPend(context.Background()); err != nil {
		t.Fatalf("acquire #1: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.acquireConnOrPend(ctx) }()
	pollUntil(t, func() bool { return p.upstreamRqPendingActive.Load() == 1 },
		"waiter #2 to enqueue")

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled acquire #2: got %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled acquire #2 did not return")
	}
	if got := p.upstreamRqPendingActive.Load(); got != 0 {
		t.Fatalf("after cancel: pendingActive = %d, want 0", got)
	}
	if got := p.rqPendingOpen.Load(); got != 0 {
		t.Fatalf("after cancel: rqPendingOpen = %d, want 0", got)
	}
	// The waiter was cleanly removed (no permit handed to it): a release now
	// drops activeConns to 0 rather than waking a ghost.
	p.releaseConn()
	if got := p.activeConns; got != 0 {
		t.Fatalf("after release: activeConns = %d, want 0 (clean removal)", got)
	}
}

// (e-variant) race the ctx-cancel against releaseConn many times to exercise
// the drain-and-give-back branch (waiter already dequeued by release). Assert
// no permit leaks: activeConns returns to 0 after all permits are released.
func TestConnPoolCtxCancelDrainGiveBackRace(t *testing.T) {
	const iters = 3000
	// drainHits counts iterations where the cancel path returned ctx.Err() yet
	// no waiter remained at cancel time AND no clean-removal decrement of
	// pendingActive happened from the cancel side — i.e. release won the dequeue
	// and the waiter took the drain-and-give-back branch. We instrument this via
	// the wakeWon flag below.
	drainHits := 0
	wokeWon := 0
	for i := 0; i < iters; i++ {
		p := newTestConnPool(1, 5)
		if err := p.acquireConnOrPend(context.Background()); err != nil {
			t.Fatalf("iter %d acquire #1: %v", i, err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- p.acquireConnOrPend(ctx) }()
		pollUntil(t, func() bool { return p.upstreamRqPendingActive.Load() == 1 },
			"waiter to enqueue")

		// Race: cancel and release fire concurrently. Sometimes release wins
		// the dequeue (waiter takes the drain-and-give-back branch); sometimes
		// cancel removes the waiter cleanly (release then drops activeConns).
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); cancel() }()
		go func() { defer wg.Done(); p.releaseConn() }()
		wg.Wait()

		err := <-done
		switch {
		case err == nil:
			// Release won and the waiter took the permit WITHOUT observing the
			// cancel (the channel-receive case won the select). The waiter owns
			// a live permit and must return it (production: via connDec).
			wokeWon++
			p.releaseConn()
		case errors.Is(err, context.Canceled):
			// ctx.Err() returned. This is EITHER clean-removal (cancel won the
			// dequeue) OR drain-and-give-back (release won the dequeue but the
			// select picked ctx.Done(); the waiter drained ch + released). Both
			// leave activeConns == 0 below; the give-back proves no leak.
			drainHits++
		default:
			t.Fatalf("iter %d waiter: unexpected err %v", i, err)
		}

		// THE load-bearing invariant: exactly one permit ever existed and it is
		// now free — no leak regardless of who won the race.
		if got := p.activeConns; got != 0 {
			t.Fatalf("iter %d: activeConns = %d, want 0 (permit leak)", i, got)
		}
		if got := p.upstreamRqPendingActive.Load(); got != 0 {
			t.Fatalf("iter %d: pendingActive = %d, want 0", i, got)
		}
		// hasWaiter must report empty — no ghost left in the FIFO.
		if p.hasWaiter() {
			t.Fatalf("iter %d: waiter still queued after race", i)
		}
	}
	t.Logf("race outcomes over %d iters: ctx.Err()-path=%d, wake-won-path=%d", iters, drainHits, wokeWon)
	// Both paths must be reachable for the test to be non-vacuous; with a real
	// concurrency race over thousands of iters both arms are hit in practice.
	if drainHits == 0 {
		t.Fatal("never hit the ctx-cancel path — race not exercised (vacuous)")
	}
	if wokeWon == 0 {
		t.Fatal("never hit the wake-won path (live-permit handoff under cancel) — race not exercised (vacuous)")
	}
}

// (f) max_connections:0 — D-S431-6.
func TestConnPoolMaxConnectionsZero(t *testing.T) {
	// maxPending 0 → the FIRST acquire overflows immediately.
	p0 := newTestConnPool(0, 0)
	if err := p0.acquireConnOrPend(context.Background()); !errors.Is(err, errConnPoolOverflow) {
		t.Fatalf("maxConns=0,maxPending=0 first acquire: got %v, want errConnPoolOverflow", err)
	}

	// maxPending 1 → the first acquire pends; cancel ⇒ ctx.Err().
	p1 := newTestConnPool(0, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p1.acquireConnOrPend(ctx) }()
	pollUntil(t, func() bool { return p1.upstreamRqPendingActive.Load() == 1 },
		"first acquire to pend under maxConns=0")
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("maxConns=0,maxPending=1 canceled acquire: got %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled acquire under maxConns=0 did not return")
	}
}

// (g) -race concurrency: peak live conns never exceeds maxConnections.
func TestConnPoolConcurrencyRace(t *testing.T) {
	const (
		maxConns = 4
		workers  = 200
	)
	p := newTestConnPool(maxConns, 1000)
	ctx := context.Background()

	var live atomic.Int64
	var peak atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if err := p.acquireConnOrPend(ctx); err != nil {
				t.Errorf("acquire: unexpected err %v", err)
				return
			}
			n := live.Add(1)
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			// tiny op
			runtime.Gosched()
			live.Add(-1)
			p.releaseConn()
		}()
	}
	wg.Wait()

	if got := peak.Load(); got > maxConns {
		t.Fatalf("peak live conns = %d, exceeds maxConnections %d", got, maxConns)
	}
	if got := p.activeConns; got != 0 {
		t.Fatalf("after all workers: activeConns = %d, want 0", got)
	}
	if got := live.Load(); got != 0 {
		t.Fatalf("live counter = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Task 5: permit overlay at the connection-CREATION boundaries (Dial +
// AcquireH1 MISS) + connDec compose. The real AcquireH1-pend test: a CB
// cluster at max_connections:1 lets the first AcquireH1 through (holds the
// single permit) and makes the second pend then time out under a short-
// deadline ctx; a no-circuit_breakers cluster never pends.
// ---------------------------------------------------------------------------

// attachConnPool gives the test cluster a circuitBreaker carrying a connPool at
// the supplied budgets (mirrors parseCircuitBreakers's pool build). Models a
// cluster configured WITH circuit_breakers so the permit gate engages.
func attachConnPool(c *Cluster, maxConns, maxPending int64) {
	c.circuitBreaker = &circuitBreaker{pool: &connPool{
		maxConnections:     maxConns,
		maxPendingRequests: maxPending,
	}}
}

// acceptAndHoldListener returns a listener that accepts connections and holds
// them open (draining bytes) until the test ends, so AcquireH1 dials succeed
// and the dialed conn (and thus its permit) stays live.
func acceptAndHoldListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Drain-only into io.Discard: src (conn) != dst (io.Discard), so the
			// Linux splice-on-loopback deadlock echoConn avoids (src==dst) does
			// not apply here.
			go func() { _, _ = io.Copy(io.Discard, conn) }()
		}
	}()
	return ln
}

func TestAcquireH1_PermitGate_PendsAtMaxConnections(t *testing.T) {
	ln := acceptAndHoldListener(t)
	stub := &stubLB{ep: endpointFromAddr(ln.Addr())}
	c := newTestClusterLB(t, stub, stub.ep)
	attachConnPool(c, 1, 1000) // max_connections:1, generous queue

	// AcquireH1 #1: a fresh dial (pool MISS) → acquires the single permit.
	p1, _, err := c.AcquireH1(context.Background())
	if err != nil {
		t.Fatalf("AcquireH1 #1: %v", err)
	}

	// AcquireH1 #2 under a short deadline: the permit is held by #1, so it
	// pends in the wait-queue then times out → ctx.DeadlineExceeded.
	ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	p2, _, err := c.AcquireH1(ctx2)
	if err == nil {
		t.Fatalf("AcquireH1 #2: expected ctx-deadline error (permit gate), got nil (p=%v)", p2)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AcquireH1 #2: err = %v, want context.DeadlineExceeded (pended then timed out)", err)
	}

	// Closing #1 releases its permit (the conn-Close wake seam) → a #3 acquire
	// now succeeds (proves connDec composed the pool release on Close).
	_ = p1.Conn.Close()
	p3, _, err := c.AcquireH1(context.Background())
	if err != nil {
		t.Fatalf("AcquireH1 #3 after #1 Close (permit freed): %v", err)
	}
	_ = p3.Conn.Close()
}

func TestAcquireH1_NoCircuitBreakers_NeverPends(t *testing.T) {
	ln := acceptAndHoldListener(t)
	stub := &stubLB{ep: endpointFromAddr(ln.Addr())}
	c := newTestClusterLB(t, stub, stub.ep) // no circuitBreaker → no pool

	// Both acquires succeed immediately — no permit gate (byte-neutral).
	p1, _, err := c.AcquireH1(context.Background())
	if err != nil {
		t.Fatalf("AcquireH1 #1 (no CB): %v", err)
	}
	ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	p2, _, err := c.AcquireH1(ctx2)
	if err != nil {
		t.Fatalf("AcquireH1 #2 (no CB) must NOT pend: %v", err)
	}
	_ = p1.Conn.Close()
	_ = p2.Conn.Close()
}

// attachConnPoolWithStats is attachConnPool but with real injected stat handles
// so the pend/wake integration test can poll upstreamRqPendingActive and assert
// upstreamRqPendingTotal. Returns the pool for direct inspection.
func attachConnPoolWithStats(c *Cluster, maxConns, maxPending int64) *connPool {
	p := newTestConnPool(maxConns, maxPending)
	c.circuitBreaker = &circuitBreaker{pool: p}
	return p
}

// TestPutIdleH1_IdleReturnWake proves BOTH connection-budget wake sources route
// through the single permit mechanism: a held conn's Close (Path A) AND a
// PutIdleH1 of a held conn while a waiter is queued (Path B, the new seam).
//
// Setup: max_connections:2, max_pending_requests:4, backend holds connections.
//   - AcquireH1 #1,#2 fill the 2 permits (conns block at the held backend).
//   - AcquireH1 #3,#4 launched in goroutines → PEND (2 waiters queued).
//   - Path A: Close p1.Conn → its connDec → pool.releaseConn → wakes #3.
//   - Path B: PutIdleH1(p2) while #4 pends → p2 is CLOSED-and-woke (NOT pooled)
//     → its connDec → pool.releaseConn → wakes #4 (which dials FRESH).
//
// Path B is distinguished from Path A because at the PutIdleH1 call only #4
// remains pending (#3 already woke via Path A and that wake is observed before
// the PutIdleH1 fires) and ONLY the PutIdleH1 call can release a permit at that
// instant — no other Close/release happens between #3's wake and #4's wake.
func TestPutIdleH1_IdleReturnWake(t *testing.T) {
	ln := acceptAndHoldListener(t)
	stub := &stubLB{ep: endpointFromAddr(ln.Addr())}
	c := newTestClusterLB(t, stub, stub.ep)
	pool := attachConnPoolWithStats(c, 2, 4)

	// Peak tracker: count successful concurrent AcquireH1s; must never exceed 2.
	var liveAcq atomic.Int64
	var peakAcq atomic.Int64
	bump := func() {
		n := liveAcq.Add(1)
		for {
			old := peakAcq.Load()
			if n <= old || peakAcq.CompareAndSwap(old, n) {
				break
			}
		}
	}

	// #1,#2: fill the 2 permits with fresh dials (conns held open by backend).
	p1, _, err := c.AcquireH1(context.Background())
	if err != nil {
		t.Fatalf("AcquireH1 #1: %v", err)
	}
	bump()
	p2, _, err := c.AcquireH1(context.Background())
	if err != nil {
		t.Fatalf("AcquireH1 #2: %v", err)
	}
	bump()

	// #3,#4: PEND (cap reached). Capture each result so we prove both return.
	type acqResult struct {
		p   *PooledH1Conn
		err error
	}
	done3 := make(chan acqResult, 1)
	done4 := make(chan acqResult, 1)
	go func() {
		p, _, e := c.AcquireH1(context.Background())
		if e == nil {
			bump()
		}
		done3 <- acqResult{p, e}
	}()
	go func() {
		p, _, e := c.AcquireH1(context.Background())
		if e == nil {
			bump()
		}
		done4 <- acqResult{p, e}
	}()

	// Wait until BOTH #3 and #4 are pending (2 waiters queued).
	pollUntil(t, func() bool { return pool.upstreamRqPendingActive.Load() == 2 },
		"both #3 and #4 to pend (pendingActive==2)")

	// ---- Path A: conn-Close wake. Close #1 → releaseConn → wakes ONE waiter.
	liveAcq.Add(-1) // #1's permit is being handed off via close; track its slot freeing
	_ = p1.Conn.Close()

	// One of #3/#4 wakes. Drain whichever returns first; the OTHER stays pending.
	var firstWoken acqResult
	var firstIsFrom3 bool
	select {
	case r := <-done3:
		firstWoken, firstIsFrom3 = r, true
	case r := <-done4:
		firstWoken, firstIsFrom3 = r, false
	case <-time.After(2 * time.Second):
		t.Fatal("Path A: no waiter woke after closing #1")
	}
	if firstWoken.err != nil {
		t.Fatalf("Path A: woken acquire returned err %v", firstWoken.err)
	}
	if firstWoken.p == nil || firstWoken.p.Conn == nil {
		t.Fatal("Path A: woken acquire returned nil conn")
	}

	// pendingActive must have dropped to exactly 1 (only one waiter remains).
	pollUntil(t, func() bool { return pool.upstreamRqPendingActive.Load() == 1 },
		"pendingActive to drop to 1 after Path A wake")

	// The remaining waiter is the OTHER goroutine; it MUST still be blocked
	// (no spurious wake). Assert its channel is empty right now.
	remaining := done4
	if !firstIsFrom3 {
		remaining = done3
	}
	select {
	case r := <-remaining:
		t.Fatalf("Path A leaked a wake to the second waiter (err=%v) — cannot distinguish Path B", r.err)
	default:
	}

	// ---- Path B: idle-return wake. PutIdleH1(#2) while the second waiter pends.
	// Snapshot the idle-pool size: Path B must NOT grow it (the conn is closed,
	// not pooled). Only THIS call can free a permit now → it must wake #4.
	c.h1PoolMu.Lock()
	idleBefore := len(c.h1Pool[p2.ep.Addr()])
	c.h1PoolMu.Unlock()

	liveAcq.Add(-1) // #2's permit freed via the close-and-wake
	c.PutIdleH1(p2)

	var secondWoken acqResult
	select {
	case r := <-remaining:
		secondWoken = r
	case <-time.After(2 * time.Second):
		t.Fatal("Path B: PutIdleH1 did not wake the second waiter (idle-return wake missing)")
	}
	if secondWoken.err != nil {
		t.Fatalf("Path B: woken acquire returned err %v", secondWoken.err)
	}
	if secondWoken.p == nil || secondWoken.p.Conn == nil {
		t.Fatal("Path B: woken acquire returned nil conn")
	}

	// The PutIdleH1 conn must NOT have been pooled (it was closed-and-woke).
	c.h1PoolMu.Lock()
	idleAfter := len(c.h1Pool[p2.ep.Addr()])
	c.h1PoolMu.Unlock()
	if idleAfter != idleBefore {
		t.Fatalf("Path B: idle pool grew %d→%d — PutIdleH1 pooled instead of closing-and-waking", idleBefore, idleAfter)
	}

	// pendingActive back to 0; both pends were counted.
	pollUntil(t, func() bool { return pool.upstreamRqPendingActive.Load() == 0 },
		"pendingActive to drop to 0 after Path B wake")
	if got := pool.upstreamRqPendingTotal.Load(); got != 2 {
		t.Fatalf("upstreamRqPendingTotal = %d, want 2", got)
	}

	// Cap was never exceeded at any instant.
	if got := peakAcq.Load(); got > 2 {
		t.Fatalf("peak concurrent AcquireH1 = %d, exceeds max_connections 2", got)
	}

	// No goroutine leak: BOTH #3 and #4 returned (drained above). Close the two
	// woken conns to free their permits cleanly.
	_ = firstWoken.p.Conn.Close()
	_ = secondWoken.p.Conn.Close()
}

func TestDial_PermitGate_PendsAtMaxConnections(t *testing.T) {
	ln := acceptAndHoldListener(t)
	stub := &stubLB{ep: endpointFromAddr(ln.Addr())}
	c := newTestClusterLB(t, stub, stub.ep)
	attachConnPool(c, 1, 1000)

	conn1, _, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial #1: %v", err)
	}
	ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	conn2, _, err := c.Dial(ctx2)
	if err == nil {
		t.Fatalf("Dial #2: expected ctx-deadline error (permit gate), got nil (conn=%v)", conn2)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Dial #2: err = %v, want context.DeadlineExceeded", err)
	}
	// Close #1 frees the permit (connDec wake seam) → #3 succeeds.
	_ = conn1.Close()
	conn3, _, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial #3 after #1 Close: %v", err)
	}
	_ = conn3.Close()
}
