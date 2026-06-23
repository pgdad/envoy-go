package cluster

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// ---------------------------------------------------------------------------
// Task 9.5: connect-time dial coalescing — concurrent demand attaches to an
// establishing conn instead of each acquirer opening its own. K concurrent
// streams at cap C must open exactly ceil(K/C) conns (the reference invariant;
// the 0079 differential bites the one-conn-per-stream defect). (phase 43.2a)
// ---------------------------------------------------------------------------

// gatedH2CListener is an in-process plaintext-h2c listener whose per-conn
// handshake is held until the test RELEASES the gate. This forces every
// concurrent AcquireH2Stream to overlap inside the "connecting" window: the
// dial of conn#1 is in flight (handshake parked on the gate) while the other
// K-1 acquirers reach h2PoolMu and must decide HIT/attach/dial — exactly the
// burst the coalescing logic governs. Without coalescing each acquirer dials
// its own conn (all parked on the gate); with coalescing only ceil(K/C) dials
// are ever started.
type gatedH2CListener struct {
	ln       net.Listener
	gate     chan struct{} // closed by release() → all parked handshakes proceed
	accepted atomic.Int64  // raw TCP accepts (== number of dials actually started)
}

func newGatedH2CListener(t *testing.T) *gatedH2CListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	g := &gatedH2CListener{ln: ln, gate: make(chan struct{})}
	go func() {
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			g.accepted.Add(1)
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				<-g.gate // hold the handshake until the test releases the gate
				if herr := h2ServerPrefacePeer(c); herr != nil {
					return
				}
				_, _ = io.Copy(io.Discard, c)
			}(conn)
		}
	}()
	return g
}

func (g *gatedH2CListener) release() { close(g.gate) }
func (g *gatedH2CListener) close()   { _ = g.ln.Close() }
func (g *gatedH2CListener) ep() Endpoint {
	return endpointFromAddr(g.ln.Addr())
}

// acqResult bundles one concurrent AcquireH2Stream outcome.
type acqResult struct {
	cc  *h2.ClientConn
	rel func()
	err error
}

// committedSlots returns the total committed stream slots for addr — the sum of
// every connecting entry's promised count PLUS every pooled conn's inFlight.
// Under the lock. With the dial gate held this equals the number of acquirers
// that have reached + committed their connecting/attach decision (nothing has
// converted yet, so the count lives entirely in h2Connecting; the pooled term
// makes it robust if a dial ever proceeds). (phase 43.2a, Task 9.5)
func committedSlots(c *Cluster, addr string) int64 {
	c.h2PoolMu.Lock()
	defer c.h2PoolMu.Unlock()
	var n int64
	for _, cn := range c.h2Connecting[addr] {
		n += cn.promised
	}
	for _, pc := range c.h2Pool[addr] {
		n += pc.inFlight
	}
	return n
}

// runBurst launches K concurrent AcquireH2Stream calls against c, holding each
// stream (not releasing), and returns once all K have returned. The dial gate on
// g is held until ALL K acquirers have reached + COMMITTED their connecting/
// attach decision (committedSlots(addr)==K) — a deterministic poll rather than a
// fixed sleep, so the coalescing-overlap window is fully open before the gate
// releases regardless of scheduler load (the strict anti-flake idiom this file
// already uses in CtxCancelWhileAttaching). The dial is parked on the gate, so
// the K held streams stay committed-but-not-converted throughout the poll.
func runBurst(t *testing.T, c *Cluster, g *gatedH2CListener, K int) []acqResult {
	t.Helper()
	addr := g.ep().Addr()
	results := make([]acqResult, K)
	var wg sync.WaitGroup
	wg.Add(K)
	for i := 0; i < K; i++ {
		go func(i int) {
			defer wg.Done()
			cc, rel, _, err := c.AcquireH2Stream(context.Background())
			results[i] = acqResult{cc: cc, rel: rel, err: err}
		}(i)
	}
	// Wait until all K acquirers have committed a slot (each is now a connecting
	// initiator or an attacher parked on a connecting entry's ready chan) — the
	// coalescing decision is fully resolved. Only THEN release the parked dials.
	pollUntil(t, func() bool { return committedSlots(c, addr) == int64(K) },
		"all K acquirers should commit their connecting/attach decision before release")
	g.release()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("burst deadlocked")
	}
	return results
}

// mkGatedH2Cluster builds a dial-capable plaintext-h2c cluster pointed at the
// gated listener (single endpoint), with the H2-pool maps + real gauges.
func mkGatedH2Cluster(t *testing.T, g *gatedH2CListener, maxConns, maxPending, C int64) *Cluster {
	t.Helper()
	return mkLifecycleH2Cluster(t, g.ep(), maxConns, maxPending, C)
}

// assertBurstCeil runs a K-concurrent burst at cap C and asserts EXACTLY
// ceil(K/C) conns + streams_active==K + upstream_cx_http2_total==ceil(K/C) +
// accepted dials==ceil(K/C). This is the core coalescing invariant — it FAILS
// against the pre-Task-9.5 code (one conn per stream → K conns).
func assertBurstCeil(t *testing.T, K, C int) {
	t.Helper()
	g := newGatedH2CListener(t)
	defer g.close()
	c := mkGatedH2Cluster(t, g, 64, 256, int64(C)) // max_connections high
	addr := g.ep().Addr()

	results := runBurst(t, c, g, K)
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("acquire #%d: %v", i, r.err)
		}
		if r.cc == nil || r.rel == nil {
			t.Fatalf("acquire #%d: nil cc/rel", i)
		}
	}

	wantConns := (K + C - 1) / C // ceil(K/C)
	if n := poolConnCount(c, addr); n != wantConns {
		t.Fatalf("K=%d C=%d: pool conns = %d, want %d (ceil(K/C)) — coalescing failed",
			K, C, n, wantConns)
	}
	if got := c.http2StreamsActive.Load(); got != int64(K) {
		t.Fatalf("K=%d C=%d: streams_active = %d, want %d", K, C, got, K)
	}
	if got := c.upstreamCxHTTP2Total.Load(); got != uint64(wantConns) {
		t.Fatalf("K=%d C=%d: upstream_cx_http2_total = %d, want %d", K, C, got, wantConns)
	}
	if got := c.upstreamCxTotal.Load(); got != uint64(wantConns) {
		t.Fatalf("K=%d C=%d: upstream_cx_total = %d, want %d", K, C, got, wantConns)
	}
	if got := g.accepted.Load(); got != int64(wantConns) {
		t.Fatalf("K=%d C=%d: dials actually started = %d, want %d (extra dials => no coalescing)",
			K, C, got, wantConns)
	}

	// Release all streams → everything drains to 0 (no leaked permit/stream).
	for _, r := range results {
		r.rel()
	}
	if got := c.http2StreamsActive.Load(); got != 0 {
		t.Fatalf("K=%d C=%d: after release streams_active = %d, want 0", K, C, got)
	}
	c.h2PoolMu.Lock()
	ac := c.circuitBreaker.pool.activeConns
	nConn := len(c.h2Pool[addr])
	c.h2PoolMu.Unlock()
	// Conns are NOT closed by release (they stay pooled, live, idle) → permits
	// remain held by the pooled conns. activeConns == ceil(K/C) at quiescence.
	if ac != int64(wantConns) {
		t.Fatalf("K=%d C=%d: activeConns = %d, want %d (pooled conns hold their permits)",
			K, C, ac, wantConns)
	}
	if nConn != wantConns {
		t.Fatalf("K=%d C=%d: pooled conns after release = %d, want %d", K, C, nConn, wantConns)
	}
}

// TestAcquireH2Stream_ConcurrentBurstCeil_6_2 is the 0079 shape: K=6, C=2 →
// exactly 3 conns. The defect (no connect-time coalescing) opens 6.
func TestAcquireH2Stream_ConcurrentBurstCeil_6_2(t *testing.T) { assertBurstCeil(t, 6, 2) }

// TestAcquireH2Stream_ConcurrentBurstCeil_5_2: K=5, C=2 → 3 (odd ratio).
func TestAcquireH2Stream_ConcurrentBurstCeil_5_2(t *testing.T) { assertBurstCeil(t, 5, 2) }

// TestAcquireH2Stream_ConcurrentBurstCeil_7_4: K=7, C=4 → 2.
func TestAcquireH2Stream_ConcurrentBurstCeil_7_4(t *testing.T) { assertBurstCeil(t, 7, 4) }

// TestAcquireH2Stream_ConcurrentBurstCeil_8_1: K=8, C=1 → 8 (no multiplex; each
// stream needs its own conn — coalescing must NOT under-open).
func TestAcquireH2Stream_ConcurrentBurstCeil_8_1(t *testing.T) { assertBurstCeil(t, 8, 1) }

// TestAcquireH2Stream_ConcurrentBurstCeil_4_4: K=4, C=4 → 1 (full multiplex onto
// a single establishing conn — all attachers ride the one dial).
func TestAcquireH2Stream_ConcurrentBurstCeil_4_4(t *testing.T) { assertBurstCeil(t, 4, 4) }

// gatedFailH2CListener is a plaintext-h2c listener that PARKS each accepted
// conn's handshake on a gate, then on release CLOSES the raw conn before the
// preface exchange so h2.NewClientConn FAILS. It deterministically forces a
// connecting-dial FAILURE while attachers are parked on the connecting conn's
// ready chan — the dial-failure-with-attachers path.
type gatedFailH2CListener struct {
	ln       net.Listener
	gate     chan struct{}
	accepted atomic.Int64
}

func newGatedFailH2CListener(t *testing.T) *gatedFailH2CListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	g := &gatedFailH2CListener{ln: ln, gate: make(chan struct{})}
	go func() {
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			g.accepted.Add(1)
			go func(c net.Conn) {
				<-g.gate
				_ = c.Close() // fail the handshake (peer vanished mid-preface)
			}(conn)
		}
	}()
	return g
}

func (g *gatedFailH2CListener) release()     { close(g.gate) }
func (g *gatedFailH2CListener) close()       { _ = g.ln.Close() }
func (g *gatedFailH2CListener) ep() Endpoint { return endpointFromAddr(g.ln.Addr()) }

// TestAcquireH2Stream_ConcurrentBurst_DialFailureWithAttachers: K=8 acquirers
// burst at C=16 → ALL coalesce onto ONE connecting conn (1 initiator + 7
// attachers). The single dial FAILS. Every acquirer (initiator + every attacher)
// must surface the dial error CLEANLY — symmetric with the pre-9.5 MISS-dial
// failure (the router owns retry) — with ZERO leaked permit/streams_active/
// lbRelease: streams_active back to 0, activeConns back to 0 (the failed dial's
// permit was returned), and the recording LB's active count back to 0 (every
// attacher fired its own lbRelease immediately; the initiator's was returned by
// dialPooledH2To on the failure).
func TestAcquireH2Stream_ConcurrentBurst_DialFailureWithAttachers(t *testing.T) {
	const K = 8
	const C = 16 // all K coalesce onto ONE establishing conn
	g := newGatedFailH2CListener(t)
	defer g.close()
	ep := g.ep()
	lb := &recordingLB{eps: []Endpoint{ep}}
	c := mkMultiEndpointH2Cluster(t, lb, 64, 256, C, ep)
	addr := ep.Addr()

	results := make([]acqResult, K)
	var wg sync.WaitGroup
	wg.Add(K)
	for i := 0; i < K; i++ {
		go func(i int) {
			defer wg.Done()
			cc, rel, _, err := c.AcquireH2Stream(context.Background())
			results[i] = acqResult{cc: cc, rel: rel, err: err}
		}(i)
	}
	// Wait until all K coalesce onto the single connecting conn (committedSlots==K,
	// parked on the gate) — deterministic, no fixed sleep — then release → the
	// dial fails for everyone.
	pollUntil(t, func() bool { return committedSlots(c, addr) == int64(K) },
		"all K should coalesce onto the single connecting conn before release")
	g.release()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("dial-failure burst deadlocked")
	}

	// Exactly one dial was started (all coalesced); it failed → everyone errors.
	if got := g.accepted.Load(); got != 1 {
		t.Fatalf("dials started = %d, want 1 (all coalesced onto one connecting conn)", got)
	}
	fails := 0
	for i, r := range results {
		if r.err == nil {
			t.Fatalf("acquire #%d succeeded; want the dial error surfaced cleanly", i)
		}
		if r.cc != nil || r.rel != nil {
			t.Fatalf("acquire #%d: non-nil cc/rel on a failed dial", i)
		}
		fails++
	}
	if fails != K {
		t.Fatalf("failed acquires = %d, want %d (every attacher must surface the error)", fails, K)
	}

	// Conservation at quiescence: no leaked stream slot, permit, or LB slot.
	if got := c.http2StreamsActive.Load(); got != 0 {
		t.Fatalf("streams_active = %d, want 0 (a failed attacher leaked a slot)", got)
	}
	c.h2PoolMu.Lock()
	nConn := len(c.h2Pool[addr])
	nConnecting := len(c.h2Connecting[addr])
	ac := c.circuitBreaker.pool.activeConns
	c.h2PoolMu.Unlock()
	if nConn != 0 {
		t.Fatalf("pooled conns = %d, want 0 (the dial failed)", nConn)
	}
	if nConnecting != 0 {
		t.Fatalf("connecting conns = %d, want 0 (the failed entry must be removed)", nConnecting)
	}
	if ac != 0 {
		t.Fatalf("activeConns = %d, want 0 (failed dial leaked a permit)", ac)
	}
	if got := lb.active.Load(); got != 0 {
		t.Fatalf("LB active = %d, want 0 (a failed attacher/initiator leaked an LB slot)", got)
	}
}

// TestAcquireH2Stream_ConcurrentBurst_CtxCancelWhileAttaching: K acquirers burst
// onto one establishing conn (C high); some have their ctx canceled WHILE
// attached + waiting on the connecting conn's ready chan. The canceled ones must
// release their promised slot + streams_active + lbRelease cleanly; survivors
// ride the established conn. At quiescence streams_active == survivors and no
// permit leaks.
func TestAcquireH2Stream_ConcurrentBurst_CtxCancelWhileAttaching(t *testing.T) {
	g := newGatedH2CListener(t)
	defer g.close()
	const K = 8 // 1 initiator + 7 attachers (4 of the attachers get canceled)
	const C = 16
	c := mkGatedH2Cluster(t, g, 64, 256, C)
	addr := g.ep().Addr()

	type res struct {
		rel func()
		err error
	}
	// Launch the INITIATOR first under an UNcancelable ctx + let it become the
	// connecting entry's owner and start dialing (parked on the gate). The
	// connecting dial is owned by this acquirer's ctx, so canceling an ATTACHER
	// must not abort the shared dial; pinning the initiator to Background isolates
	// the test from the (valid) "initiator-canceled → dial aborts for all" case so
	// it deterministically exercises the ATTACHER cancel-while-waiting path.
	var initRes res
	var initWG sync.WaitGroup
	initWG.Add(1)
	go func() {
		defer initWG.Done()
		_, rel, _, err := c.AcquireH2Stream(context.Background())
		initRes = res{rel: rel, err: err}
	}()
	// Wait until the initiator registers the connecting entry (promised==1).
	pollUntil(t, func() bool {
		c.h2PoolMu.Lock()
		defer c.h2PoolMu.Unlock()
		return len(c.h2Connecting[addr]) == 1
	}, "initiator should register a connecting entry")

	const attachers = K - 1
	ares := make([]res, attachers)
	ctxs := make([]context.Context, attachers)
	cancels := make([]context.CancelFunc, attachers)
	for i := 0; i < attachers; i++ {
		ctxs[i], cancels[i] = context.WithCancel(context.Background())
	}
	var wg sync.WaitGroup
	wg.Add(attachers)
	for i := 0; i < attachers; i++ {
		go func(i int) {
			defer wg.Done()
			_, rel, _, err := c.AcquireH2Stream(ctxs[i])
			ares[i] = res{rel: rel, err: err}
		}(i)
	}
	// Wait until all attachers have committed their promised slot (promised==K).
	pollUntil(t, func() bool {
		c.h2PoolMu.Lock()
		defer c.h2PoolMu.Unlock()
		q := c.h2Connecting[addr]
		return len(q) == 1 && q[0].promised == int64(K)
	}, "all attachers should attach (promised==K)")

	// Cancel HALF the attachers while they wait on ready, then release the gate.
	canceledIdx := 0
	for i := 0; i < attachers; i += 2 {
		cancels[i]()
		canceledIdx++
	}
	g.release()

	done := make(chan struct{})
	go func() { wg.Wait(); initWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("cancel-while-attaching burst deadlocked")
	}

	// The initiator must succeed (Background ctx, dial succeeded) → release it.
	if initRes.err != nil {
		t.Fatalf("initiator: %v (the shared dial was wrongly aborted by an attacher cancel)", initRes.err)
	}
	initRes.rel()
	// Survivors (non-canceled attachers) succeed; canceled ones return ctx.Err()
	// (or, racing ready, succeed). Count successes by nil err.
	wantAttachSurv := attachers - canceledIdx
	gotAttachSurv := 0
	for i := 0; i < attachers; i++ {
		if ares[i].err == nil {
			gotAttachSurv++
			ares[i].rel()
		}
	}
	if gotAttachSurv < wantAttachSurv {
		t.Fatalf("attacher survivors = %d, want >= %d (a non-canceled attacher failed)",
			gotAttachSurv, wantAttachSurv)
	}
	// All slots released → streams_active back to 0 (no canceled attacher leaked).
	if got := c.http2StreamsActive.Load(); got != 0 {
		t.Fatalf("after release: streams_active = %d, want 0 (canceled attacher leaked a slot)", got)
	}
	// Exactly one conn was dialed (all coalesced); it stays pooled holding 1 permit.
	if n := poolConnCount(c, addr); n != 1 {
		t.Fatalf("pool conns = %d, want 1 (all coalesced onto one establishing conn)", n)
	}
	c.h2PoolMu.Lock()
	ac := c.circuitBreaker.pool.activeConns
	nConnecting := len(c.h2Connecting[addr])
	c.h2PoolMu.Unlock()
	if ac != 1 {
		t.Fatalf("activeConns = %d, want 1 (no leaked permit from canceled attachers)", ac)
	}
	if nConnecting != 0 {
		t.Fatalf("connecting entries = %d, want 0 (converted on success)", nConnecting)
	}
	if len(c.h2Waiters[addr]) != 0 {
		t.Fatalf("waiter queue len = %d, want 0", len(c.h2Waiters[addr]))
	}
}
