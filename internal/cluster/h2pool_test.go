package cluster

import (
	"context"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	"github.com/pgdad/envoy-go/internal/stats"

	"golang.org/x/net/http2"
)

// newTestH2Cluster builds a Cluster carrying an injected connPool (with real
// stat handles so the gauge/counter transitions are asserted, not nil-guarded
// away) plus the H2-pool maps + a streams_active gauge. h2MaxConcurrentStreams
// is the per-conn stream cap C. The connPool's pending handles are reused by
// the H2 queue (per SPEC §3.1 — the H2 queue shares the 43.1 pending budget +
// stats).
func newTestH2Cluster(maxConns, maxPending, maxStreams int64) *Cluster {
	p := newTestConnPool(maxConns, maxPending)
	r := stats.NewRegistry()
	return &Cluster{
		circuitBreaker:         &circuitBreaker{pool: p},
		h2MaxConcurrentStreams: maxStreams,
		h2Pool:                 make(map[string][]*pooledH2Conn),
		h2Waiters:              make(map[string][]*h2Waiter),
		http2StreamsActive:     r.NewGauge("http2.streams_active"),
	}
}

// newNoPoolH2Cluster builds a Cluster with NO circuit_breakers (no connPool) —
// the unbounded-conn-growth posture. tryAcquireConnSlot must always return true
// and the pending helpers must be nil-safe.
func newNoPoolH2Cluster(maxStreams int64) *Cluster {
	r := stats.NewRegistry()
	return &Cluster{
		h2MaxConcurrentStreams: maxStreams,
		h2Pool:                 make(map[string][]*pooledH2Conn),
		h2Waiters:              make(map[string][]*h2Waiter),
		http2StreamsActive:     r.NewGauge("http2.streams_active"),
	}
}

// newTestH2ClientConn builds a real *h2.ClientConn over a TCP loopback conn
// whose peer runs the from-scratch driver-side h2 handshake (h2ServerPrefacePeer,
// reused from dial_h2_test.go). TCP loopback (buffered) is used rather than
// net.Pipe (fully synchronous → deadlocks the handshake's interleaved
// read/write sequence). The returned conn reports Closed()==false until it is
// Close()d, after which Closed()==true — the lightweight way to control the
// liveness predicate the promotion scan reads without standing up a TLS
// listener.
func newTestH2ClientConn(t *testing.T) *h2.ClientConn {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	srvErr := make(chan error, 1)
	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			srvErr <- aerr
			return
		}
		t.Cleanup(func() { _ = conn.Close() })
		if herr := h2ServerPrefacePeer(conn); herr != nil {
			srvErr <- herr
			return
		}
		srvErr <- nil
		// Drain the client's GOAWAY-on-Close until the conn drops.
		_, _ = io.Copy(io.Discard, conn)
	}()

	raw, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cc, err := h2.NewClientConn(context.Background(), raw)
	if err != nil {
		_ = raw.Close()
		t.Fatalf("NewClientConn: %v", err)
	}
	if err := <-srvErr; err != nil {
		_ = cc.Close()
		t.Fatalf("server handshake: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	return cc
}

// ---------------------------------------------------------------------------
// Task 5: AcquireH2Stream + makeRelease lifecycle (real in-process h2c backend)
// ---------------------------------------------------------------------------

// mkLifecycleH2Cluster builds a dial-capable plaintext-h2c *Cluster (lb +
// endpoints + the cx gauges wired via mkTestCluster) and grafts on an injected
// connPool + the H2-pool maps + a real streams_active gauge + a real
// upstream_cx_http2_total counter. The cluster dials the in-process h2c backend
// at ep via dialPooledH2To. C is the per-conn stream cap (h2MaxConcurrentStreams).
func mkLifecycleH2Cluster(t *testing.T, ep Endpoint, maxConns, maxPending, C int64) *Cluster {
	t.Helper()
	c := mkTestCluster("test-h2-lifecycle", nil, ep) // nil upstreamCfg → plaintext h2c
	c.circuitBreaker = &circuitBreaker{pool: newTestConnPool(maxConns, maxPending)}
	c.h2MaxConcurrentStreams = C
	c.h2Pool = make(map[string][]*pooledH2Conn)
	c.h2Waiters = make(map[string][]*h2Waiter)
	r := stats.NewRegistry()
	c.http2StreamsActive = r.NewGauge("http2.streams_active")
	c.upstreamCxHTTP2Total = r.NewCounter("upstream_cx_http2_total")
	return c
}

// poolConnCount returns the number of pooled H2 conns for addr (lock-guarded).
func poolConnCount(c *Cluster, addr string) int {
	c.h2PoolMu.Lock()
	defer c.h2PoolMu.Unlock()
	return len(c.h2Pool[addr])
}

// mkMultiEndpointH2Cluster builds a dial-capable plaintext-h2c *Cluster over the
// given endpoints with the supplied loadBalancer (Task 7.5). Mirrors
// mkLifecycleH2Cluster but takes an explicit LB + endpoint set so a test can
// drive round-robin over two backends or a recording stub LB.
func mkMultiEndpointH2Cluster(t *testing.T, lb loadBalancer, maxConns, maxPending, C int64, eps ...Endpoint) *Cluster {
	t.Helper()
	c := mkTestCluster("test-h2-multi", nil, eps...) // nil upstreamCfg → plaintext h2c
	c.lb = lb
	c.endpoints = eps
	c.circuitBreaker = &circuitBreaker{pool: newTestConnPool(maxConns, maxPending)}
	c.h2MaxConcurrentStreams = C
	c.h2Pool = make(map[string][]*pooledH2Conn)
	c.h2Waiters = make(map[string][]*h2Waiter)
	r := stats.NewRegistry()
	c.http2StreamsActive = r.NewGauge("http2.streams_active")
	c.upstreamCxHTTP2Total = r.NewCounter("upstream_cx_http2_total")
	return c
}

// ---------------------------------------------------------------------------
// Task 7.5: multi-endpoint correctness — single ctx-aware LB pick threaded
// through the dial seam (no re-pick); conns keyed + attributed to the dialed
// endpoint; restored hash/subset affinity. (phase 43.2a, ADR-0253)
// ---------------------------------------------------------------------------

// recordingLB is a loadBalancer that records the (hashKey, hasHash) args of every
// Pick and selects an endpoint by hashKey%len when hasHash, else round-robins.
// The release is a once-guarded counter so the test can prove LB-release
// conservation (active returns to 0 at quiescence). It proves the ctx-derived
// hash key REACHES the pool's pick (the Task 7.5 affinity property).
type recordingLB struct {
	eps    []Endpoint
	mu     sync.Mutex
	keys   []uint64 // every hashKey seen, in Pick order
	hasHsh []bool   // the matching hasHash flags
	rr     atomic.Uint64
	active atomic.Int64
}

func (l *recordingLB) Pick(hashKey uint64, hasHash bool, _ SubsetMatch, _ bool) (Endpoint, func(), error) {
	l.mu.Lock()
	l.keys = append(l.keys, hashKey)
	l.hasHsh = append(l.hasHsh, hasHash)
	l.mu.Unlock()
	l.active.Add(1)
	var once sync.Once
	rel := func() { once.Do(func() { l.active.Add(-1) }) }
	if len(l.eps) == 0 {
		return Endpoint{}, rel, errNoEndpoints
	}
	var i uint64
	if hasHash {
		i = hashKey % uint64(len(l.eps))
	} else {
		i = l.rr.Add(1) - 1
	}
	return l.eps[int(i)%len(l.eps)], rel, nil
}

// (Task 7.5, case a) Correct keying + attribution over TWO endpoints. With
// round-robin over 2 backends + C=1 (one stream per conn → a dial per concurrent
// hold), two overlapping holds land one conn per endpoint, each in its OWN
// bucket. Proves the conn is keyed under the addr of the endpoint it ACTUALLY
// dials and the returned ep matches that conn's real endpoint — the bug (storing
// a conn to epB under epA's bucket, returning the ctx-blind pick) is caught here.
func TestAcquireH2Stream_MultiEndpoint_CorrectKeying(t *testing.T) {
	lnA := listenH2C(t)
	defer func() { _ = lnA.Close() }()
	lnB := listenH2C(t)
	defer func() { _ = lnB.Close() }()
	epA := endpointFromAddr(lnA.Addr())
	epB := endpointFromAddr(lnB.Addr())
	addrA, addrB := epA.Addr(), epB.Addr()

	// roundRobin: first pick → epA, second → epB (counter formula makes pick #1
	// endpoints[0]). C=1 forces a fresh dial per concurrent hold.
	rr := &roundRobin{endpoints: []Endpoint{epA, epB}}
	c := mkMultiEndpointH2Cluster(t, rr, 16, 16, 1, epA, epB)

	ctx := context.Background()
	cc1, rel1, gotEp1, err := c.AcquireH2Stream(ctx)
	if err != nil {
		t.Fatalf("acquire #1: %v", err)
	}
	cc2, rel2, gotEp2, err := c.AcquireH2Stream(ctx)
	if err != nil {
		t.Fatalf("acquire #2: %v", err)
	}

	// The two holds must land on the two distinct endpoints (round-robin).
	if gotEp1.Addr() == gotEp2.Addr() {
		t.Fatalf("both acquires returned the same endpoint %s; want distinct epA/epB", gotEp1.Addr())
	}
	if gotEp1.Addr() != addrA || gotEp2.Addr() != addrB {
		t.Fatalf("attribution: got ep1=%s ep2=%s, want %s then %s", gotEp1.Addr(), gotEp2.Addr(), addrA, addrB)
	}

	// Each conn must live in the bucket of the endpoint it actually dialed: one
	// conn in addrA's bucket, one in addrB's. The mis-keying bug would put both
	// under a single (ctx-blind first-pick) bucket.
	c.h2PoolMu.Lock()
	nA, nB := len(c.h2Pool[addrA]), len(c.h2Pool[addrB])
	// The pooled conn under addr1 must be the conn returned for that addr.
	pcA := c.h2Pool[addrA]
	pcB := c.h2Pool[addrB]
	c.h2PoolMu.Unlock()
	if nA != 1 || nB != 1 {
		t.Fatalf("bucket keying: len(h2Pool[A])=%d len(h2Pool[B])=%d, want 1 and 1", nA, nB)
	}
	if pcA[0].cc != cc1 {
		t.Fatalf("addrA bucket holds the wrong conn (mis-keyed): want the conn returned for epA")
	}
	if pcB[0].cc != cc2 {
		t.Fatalf("addrB bucket holds the wrong conn (mis-keyed): want the conn returned for epB")
	}

	rel1()
	rel2()
	if got := c.upstreamCxHTTP2Total.Load(); got != 2 {
		t.Fatalf("upstream_cx_http2_total = %d, want 2 (one dial per endpoint)", got)
	}
}

// (Task 7.5, case b) ctx hash-key affinity: two requests carrying the SAME ctx
// hash key route to the SAME endpoint AND multiplex onto the SAME conn; the
// recording LB proves the ctx-derived key REACHES the pick (hasHash==true with
// the exact key). A different key routes to the other endpoint (a fresh conn).
// This locks the affinity the pre-7.5 ctx-blind PickEndpoint dropped.
func TestAcquireH2Stream_MultiEndpoint_HashKeyAffinity(t *testing.T) {
	lnA := listenH2C(t)
	defer func() { _ = lnA.Close() }()
	lnB := listenH2C(t)
	defer func() { _ = lnB.Close() }()
	epA := endpointFromAddr(lnA.Addr())
	epB := endpointFromAddr(lnB.Addr())

	lb := &recordingLB{eps: []Endpoint{epA, epB}}
	// C=4 so two same-key holds multiplex onto one conn (no forced second dial).
	c := mkMultiEndpointH2Cluster(t, lb, 16, 16, 4, epA, epB)

	// Choose keys that map to distinct endpoints under key%2.
	const keyEven = uint64(40) // 40 % 2 == 0 → epA
	const keyOdd = uint64(41)  // 41 % 2 == 1 → epB

	ctxA := WithHashKey(context.Background(), keyEven)
	cc1, rel1, ep1, err := c.AcquireH2Stream(ctxA)
	if err != nil {
		t.Fatalf("acquire #1 (keyEven): %v", err)
	}
	cc2, rel2, ep2, err := c.AcquireH2Stream(ctxA)
	if err != nil {
		t.Fatalf("acquire #2 (keyEven): %v", err)
	}
	// Same key → same endpoint AND the same multiplexed conn (one dial).
	if ep1.Addr() != ep2.Addr() {
		t.Fatalf("same hash key routed to different endpoints: %s vs %s", ep1.Addr(), ep2.Addr())
	}
	if cc1 != cc2 {
		t.Fatalf("same hash key did not multiplex onto the same conn (affinity lost)")
	}
	if ep1.Addr() != epA.Addr() {
		t.Fatalf("keyEven routed to %s, want epA %s", ep1.Addr(), epA.Addr())
	}

	// A different key → the other endpoint, a fresh conn.
	ctxB := WithHashKey(context.Background(), keyOdd)
	cc3, rel3, ep3, err := c.AcquireH2Stream(ctxB)
	if err != nil {
		t.Fatalf("acquire #3 (keyOdd): %v", err)
	}
	if ep3.Addr() != epB.Addr() {
		t.Fatalf("keyOdd routed to %s, want epB %s", ep3.Addr(), epB.Addr())
	}
	if cc3 == cc1 {
		t.Fatalf("different hash key rode the same conn as the other endpoint")
	}

	// Prove the ctx hash key REACHED every pick (the Task 7.5 property: the key
	// threads to a SINGLE ctx-aware pick — exactly one pick per acquire).
	lb.mu.Lock()
	keys := append([]uint64(nil), lb.keys...)
	hasHsh := append([]bool(nil), lb.hasHsh...)
	lb.mu.Unlock()
	if len(keys) != 3 {
		t.Fatalf("LB.Pick called %d times, want 3 (one ctx-aware pick per acquire, no re-pick)", len(keys))
	}
	wantKeys := []uint64{keyEven, keyEven, keyOdd}
	for i := range keys {
		if !hasHsh[i] {
			t.Fatalf("pick #%d: hasHash=false; the ctx hash key did not reach the pick", i)
		}
		if keys[i] != wantKeys[i] {
			t.Fatalf("pick #%d: hashKey=%d, want %d (ctx key not threaded)", i, keys[i], wantKeys[i])
		}
	}

	rel1()
	rel2()
	rel3()
	if got := c.http2StreamsActive.Load(); got != 0 {
		t.Fatalf("streams_active = %d at quiescence, want 0", got)
	}
	// LB-release conservation (ADR-0232 OPTION C): the MISS-dial path TRANSFERS
	// lbRelease to the dialed conn's connWithGauge dec — it fires at conn Close,
	// NOT at stream release. Two conns are still pooled here (one per endpoint),
	// so exactly two LB picks remain held; the same-key second hold's pick fired
	// immediately on the stream-HIT. Anything other than 2 means a leak/double.
	if got := lb.active.Load(); got != 2 {
		t.Fatalf("LB active picks = %d with 2 conns pooled, want 2 (held-until-Close)", got)
	}
	// Closing the conns fires their transferred lbRelease → active drains to 0.
	_ = cc1.Close()
	_ = cc3.Close()
	if got := lb.active.Load(); got != 0 {
		t.Fatalf("LB active picks = %d after conn Close, want 0 (lbRelease leak/double)", got)
	}
}

// (Task 5, case 1) Stream HIT / multiplex: C=4, max_connections high; 4
// overlapping AcquireH2Stream all multiplex onto ONE conn (streams_active==4,
// one dial / upstream_cx_total==1).
func TestAcquireH2Stream_MultiplexHit(t *testing.T) {
	ln := listenH2C(t)
	defer func() { _ = ln.Close() }()
	ep := endpointFromAddr(ln.Addr())
	c := mkLifecycleH2Cluster(t, ep, 16, 16, 4)
	addr := ep.Addr()

	ctx := context.Background()
	releases := make([]func(), 0, 4)
	for i := 0; i < 4; i++ {
		cc, release, gotEp, err := c.AcquireH2Stream(ctx)
		if err != nil {
			t.Fatalf("acquire #%d: %v", i, err)
		}
		if cc == nil || release == nil {
			t.Fatalf("acquire #%d: nil cc/release", i)
		}
		if gotEp.Addr() != addr {
			t.Fatalf("acquire #%d: ep = %s, want %s", i, gotEp.Addr(), addr)
		}
		releases = append(releases, release)
	}

	if n := poolConnCount(c, addr); n != 1 {
		t.Fatalf("pool conns = %d, want 1 (all multiplexed)", n)
	}
	if got := c.http2StreamsActive.Load(); got != 4 {
		t.Fatalf("streams_active = %d, want 4", got)
	}
	if got := c.upstreamCxTotal.Load(); got != 1 {
		t.Fatalf("upstream_cx_total = %d, want 1 (one dial)", got)
	}
	if got := c.upstreamCxHTTP2Total.Load(); got != 1 {
		t.Fatalf("upstream_cx_http2_total = %d, want 1", got)
	}
	if got := c.upstreamCxActive.Load(); got != 1 {
		t.Fatalf("upstream_cx_active = %d, want 1", got)
	}

	for _, rel := range releases {
		rel()
	}
	if got := c.http2StreamsActive.Load(); got != 0 {
		t.Fatalf("after release: streams_active = %d, want 0", got)
	}
}

// (Task 5, case 2) Conn growth ceil(K/C): C=2, 5 overlapping holds ⇒ exactly 3
// conns (ceil(5/2)).
func TestAcquireH2Stream_ConnGrowthCeil(t *testing.T) {
	ln := listenH2C(t)
	defer func() { _ = ln.Close() }()
	ep := endpointFromAddr(ln.Addr())
	c := mkLifecycleH2Cluster(t, ep, 16, 16, 2) // C=2
	addr := ep.Addr()

	ctx := context.Background()
	releases := make([]func(), 0, 5)
	for i := 0; i < 5; i++ {
		_, release, _, err := c.AcquireH2Stream(ctx)
		if err != nil {
			t.Fatalf("acquire #%d: %v", i, err)
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, rel := range releases {
			rel()
		}
	}()

	if n := poolConnCount(c, addr); n != 3 {
		t.Fatalf("pool conns = %d, want 3 (ceil(5/2))", n)
	}
	if got := c.http2StreamsActive.Load(); got != 5 {
		t.Fatalf("streams_active = %d, want 5", got)
	}
	if got := c.upstreamCxTotal.Load(); got != 3 {
		t.Fatalf("upstream_cx_total = %d, want 3", got)
	}
	if got := c.upstreamCxHTTP2Total.Load(); got != 3 {
		t.Fatalf("upstream_cx_http2_total = %d, want 3", got)
	}
}

// (Task 5, case 3) Pend + wake-on-stream-free: C=1, max_connections=1. One held
// stream saturates the single conn + permit; a 2nd AcquireH2Stream PENDS
// (pending_active==1); releasing the 1st wakes the 2nd onto the SAME conn (no
// new dial), pending_active back to 0.
func TestAcquireH2Stream_PendAndWakeOnStreamFree(t *testing.T) {
	ln := listenH2C(t)
	defer func() { _ = ln.Close() }()
	ep := endpointFromAddr(ln.Addr())
	c := mkLifecycleH2Cluster(t, ep, 1, 4, 1) // C=1, maxConns=1
	p := c.circuitBreaker.pool
	addr := ep.Addr()

	// Hold #1 (saturates the lone conn + the lone permit).
	_, rel1, _, err := c.AcquireH2Stream(context.Background())
	if err != nil {
		t.Fatalf("acquire #1: %v", err)
	}
	if got := c.upstreamCxTotal.Load(); got != 1 {
		t.Fatalf("after #1: upstream_cx_total = %d, want 1", got)
	}

	// #2 PENDS (no free stream slot, no free permit).
	type res struct {
		cc  *h2.ClientConn
		rel func()
		err error
	}
	done := make(chan res, 1)
	go func() {
		cc, rel, _, err := c.AcquireH2Stream(context.Background())
		done <- res{cc, rel, err}
	}()

	pollUntil(t, func() bool { return p.upstreamRqPendingActive.Load() == 1 },
		"2nd acquire should pend (pending_active==1)")
	select {
	case r := <-done:
		t.Fatalf("2nd acquire returned early (err=%v) instead of pending", r.err)
	default:
	}

	// Release #1 → wakes #2 onto the SAME conn (stream-grant; no new dial).
	rel1()

	var r2 res
	select {
	case r2 = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("2nd acquire never woke after release")
	}
	if r2.err != nil {
		t.Fatalf("2nd acquire: %v", r2.err)
	}
	if n := poolConnCount(c, addr); n != 1 {
		t.Fatalf("pool conns = %d, want 1 (woken onto same conn, no new dial)", n)
	}
	if got := c.upstreamCxTotal.Load(); got != 1 {
		t.Fatalf("upstream_cx_total = %d, want 1 (no second dial)", got)
	}
	if got := p.upstreamRqPendingActive.Load(); got != 0 {
		t.Fatalf("pending_active = %d, want 0", got)
	}
	if got := c.http2StreamsActive.Load(); got != 1 {
		t.Fatalf("streams_active = %d, want 1", got)
	}
	r2.rel()
	if got := c.http2StreamsActive.Load(); got != 0 {
		t.Fatalf("after final release: streams_active = %d, want 0", got)
	}
}

// (Task 5, case 4) Overflow: C=1, max_connections=1, max_pending_requests=1.
// 1 held + 1 pending + a 3rd ⇒ the 3rd gets errConnPoolOverflow + the overflow
// counter Inc's.
func TestAcquireH2Stream_Overflow(t *testing.T) {
	ln := listenH2C(t)
	defer func() { _ = ln.Close() }()
	ep := endpointFromAddr(ln.Addr())
	c := mkLifecycleH2Cluster(t, ep, 1, 1, 1) // C=1, maxConns=1, maxPending=1
	p := c.circuitBreaker.pool

	// Hold #1.
	_, rel1, _, err := c.AcquireH2Stream(context.Background())
	if err != nil {
		t.Fatalf("acquire #1: %v", err)
	}
	defer rel1()

	// #2 PENDS (fills the single pending slot). Cancelable ctx + defer-cancel so
	// the blocked goroutine unwinds at test end (no goroutine leak).
	pendCtx, pendCancel := context.WithCancel(context.Background())
	defer pendCancel()
	go func() {
		_, _, _, _ = c.AcquireH2Stream(pendCtx)
	}()
	pollUntil(t, func() bool { return p.upstreamRqPendingActive.Load() == 1 },
		"2nd acquire should pend (pending_active==1)")

	// #3 OVERFLOWS (queue full).
	_, _, _, err = c.AcquireH2Stream(context.Background())
	if err == nil {
		t.Fatal("3rd acquire: want errConnPoolOverflow, got nil")
	}
	if !IsConnPoolOverflow(err) {
		t.Fatalf("3rd acquire: err = %v, want IsConnPoolOverflow", err)
	}
	if got := p.upstreamRqPendingOverflow.Load(); got != 1 {
		t.Fatalf("upstream_rq_pending_overflow = %d, want 1", got)
	}
}

// (Task 5, case 5) ctx-cancel while pending unwinds cleanly: gauge to 0, no
// leaked permit (activeConns returns to its pre-cancel value).
func TestAcquireH2Stream_CtxCancelWhilePending(t *testing.T) {
	ln := listenH2C(t)
	defer func() { _ = ln.Close() }()
	ep := endpointFromAddr(ln.Addr())
	c := mkLifecycleH2Cluster(t, ep, 1, 4, 1) // C=1, maxConns=1
	p := c.circuitBreaker.pool

	// Hold #1 (saturates conn + permit).
	_, rel1, _, err := c.AcquireH2Stream(context.Background())
	if err != nil {
		t.Fatalf("acquire #1: %v", err)
	}
	defer rel1()

	// #2 pends under a cancelable ctx.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, _, err := c.AcquireH2Stream(ctx)
		done <- err
	}()
	pollUntil(t, func() bool { return p.upstreamRqPendingActive.Load() == 1 },
		"2nd acquire should pend")

	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("ctx-cancel: want a ctx error, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ctx-cancel acquire never returned")
	}

	pollUntil(t, func() bool { return p.upstreamRqPendingActive.Load() == 0 },
		"pending_active should return to 0 after cancel")
	// The lone permit is still held by conn #1; no leak means activeConns==1.
	c.h2PoolMu.Lock()
	ac := p.activeConns
	c.h2PoolMu.Unlock()
	if ac != 1 {
		t.Fatalf("activeConns = %d, want 1 (no leaked permit)", ac)
	}
	if got := len(c.h2Waiters[ep.Addr()]); got != 0 {
		t.Fatalf("waiter queue len = %d, want 0", got)
	}
}

// (Task 5, case 6) Releasing the last stream of a Closed() conn evicts + Closes
// it: the permit frees (activeConns back to 0) and upstream_cx_active Decs via
// connWithGauge.
func TestAcquireH2Stream_ReleaseClosedConnEvicts(t *testing.T) {
	ln := listenH2C(t)
	defer func() { _ = ln.Close() }()
	ep := endpointFromAddr(ln.Addr())
	c := mkLifecycleH2Cluster(t, ep, 2, 4, 4)
	p := c.circuitBreaker.pool
	addr := ep.Addr()

	cc, release, _, err := c.AcquireH2Stream(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if got := c.upstreamCxActive.Load(); got != 1 {
		t.Fatalf("after acquire: upstream_cx_active = %d, want 1", got)
	}
	c.h2PoolMu.Lock()
	ac := p.activeConns
	c.h2PoolMu.Unlock()
	if ac != 1 {
		t.Fatalf("after acquire: activeConns = %d, want 1", ac)
	}

	// Simulate the conn going dead (e.g. RoundTrip error / GOAWAY): Close it so
	// Closed()==true. The release of its last stream must evict + free the permit.
	_ = cc.Close()

	release()

	if n := poolConnCount(c, addr); n != 0 {
		t.Fatalf("after release of closed conn's last stream: pool conns = %d, want 0 (evicted)", n)
	}
	c.h2PoolMu.Lock()
	ac = p.activeConns
	c.h2PoolMu.Unlock()
	if ac != 0 {
		t.Fatalf("after evict: activeConns = %d, want 0 (permit freed)", ac)
	}
	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Fatalf("after evict: upstream_cx_active = %d, want 0", got)
	}
	if got := c.http2StreamsActive.Load(); got != 0 {
		t.Fatalf("after evict: streams_active = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Task 3 (phase 43.2b): admission-skip draining conns (cc.GoneAway()) on BOTH
// established-conn sites + (Closed||GoneAway)&&inFlight==0 eviction. A draining
// conn is one whose codec observed a peer GOAWAY; it takes NO new streams but
// closes on the last in-flight stream-release. `draining` is DERIVED from
// cc.GoneAway() — no new pooledH2Conn field. The connecting tier stays unguarded.
// ---------------------------------------------------------------------------

// goawayBackend is a plaintext-h2c listener that runs the from-scratch driver
// handshake on each accepted conn AND captures the post-handshake server-side
// net.Conn (keyed by the client's remote addr) so a test can drive a peer
// GOAWAY frame down a SPECIFIC pooled conn. Without capturing the server conn
// (runH2Server discards it) there is no way to make a real *h2.ClientConn report
// GoneAway()==true. Driver-side use of golang.org/x/net/http2.Framer is permitted
// in test code per D-3.2.
type goawayBackend struct {
	ln    net.Listener
	mu    sync.Mutex
	conns []net.Conn // accepted server-side conns, post-handshake, in accept order
}

// newGoawayBackend starts the capturing h2c listener. Caller defers Close.
func newGoawayBackend(t *testing.T) *goawayBackend {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	b := &goawayBackend{ln: ln}
	go func() {
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			go func(c net.Conn) {
				if herr := h2ServerPrefacePeer(c); herr != nil {
					_ = c.Close()
					return
				}
				b.mu.Lock()
				b.conns = append(b.conns, c)
				b.mu.Unlock()
				// Post-handshake: drain inbound bytes (incl. the client's GOAWAY on
				// Close) until the client drops the conn.
				_, _ = io.Copy(io.Discard, c)
			}(conn)
		}
	}()
	return b
}

func (b *goawayBackend) Close() { _ = b.ln.Close() }

func (b *goawayBackend) endpoint() Endpoint { return endpointFromAddr(b.ln.Addr()) }

// connCount reports how many server conns have been accepted + handshaken.
func (b *goawayBackend) connCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.conns)
}

// sendGOAWAY writes a graceful GOAWAY(NO_ERROR) with a high last-stream-id (so
// it does NOT finish any in-flight client stream — it only flips the client's
// goawayCh, the GoneAway() predicate) down the idx-th accepted server conn.
func (b *goawayBackend) sendGOAWAY(t *testing.T, idx int) {
	t.Helper()
	b.mu.Lock()
	if idx >= len(b.conns) {
		b.mu.Unlock()
		t.Fatalf("sendGOAWAY: conn idx %d out of range (have %d)", idx, len(b.conns))
	}
	conn := b.conns[idx]
	b.mu.Unlock()
	fr := http2.NewFramer(conn, conn)
	// last-stream-id 1<<30: well above any allocated id → no in-flight stream is
	// finished; the client's dispatchFrame only closes goawayCh.
	if err := fr.WriteGoAway(1<<30, http2.ErrCodeNo, nil); err != nil {
		t.Fatalf("sendGOAWAY: %v", err)
	}
}

// pollGoneAway spins until cc.GoneAway() (the client readLoop processed the peer
// GOAWAY) or the deadline.
func pollGoneAway(t *testing.T, cc *h2.ClientConn) {
	t.Helper()
	pollUntil(t, func() bool { return cc.GoneAway() }, "conn should observe peer GOAWAY (GoneAway()==true)")
}

// (Task 3, case 1) Admission-skip: a single pooled conn with a free stream slot;
// drive its peer GOAWAY; once GoneAway()==true the draining conn is SKIPPED by
// findStreamHitLocked (returns nil) AND a fresh AcquireH2Stream MISSes → dials a
// SECOND conn (pool len == 2, upstream_cx_http2_total == 2). Before the
// admission-skip guard the GOAWAY'd conn (Closed()==false, inFlight<C) would be
// re-used and no second conn would dial.
func TestAcquireH2Stream_DrainSkipsAdmission(t *testing.T) {
	b := newGoawayBackend(t)
	defer b.Close()
	ep := b.endpoint()
	addr := ep.Addr()
	c := mkLifecycleH2Cluster(t, ep, 16, 16, 4) // C=4 → free slot exists

	// First acquire dials conn #1 and holds one stream slot.
	cc1, rel1, _, err := c.AcquireH2Stream(context.Background())
	if err != nil {
		t.Fatalf("acquire #1: %v", err)
	}
	defer rel1()
	if n := poolConnCount(c, addr); n != 1 {
		t.Fatalf("after acquire #1: pool conns = %d, want 1", n)
	}

	// Drive conn #1's peer GOAWAY; wait for the client to observe it.
	pollUntil(t, func() bool { return b.connCount() >= 1 }, "backend should accept conn #1")
	b.sendGOAWAY(t, 0)
	pollGoneAway(t, cc1)

	// findStreamHitLocked must now SKIP the draining conn (even though it has a
	// free slot and is not Closed()).
	c.h2PoolMu.Lock()
	hit := c.findStreamHitLocked(addr)
	c.h2PoolMu.Unlock()
	if hit != nil {
		t.Fatalf("findStreamHitLocked returned the draining conn %p, want nil (admission-skip)", hit.cc)
	}

	// A fresh acquire MISSes the draining conn → dials a SECOND conn.
	cc2, rel2, _, err := c.AcquireH2Stream(context.Background())
	if err != nil {
		t.Fatalf("acquire #2: %v", err)
	}
	defer rel2()
	if cc2 == cc1 {
		t.Fatal("acquire #2 rode the draining conn (admission-skip failed)")
	}
	if n := poolConnCount(c, addr); n != 2 {
		t.Fatalf("after acquire #2: pool conns = %d, want 2 (second dial past the draining conn)", n)
	}
	if got := c.upstreamCxHTTP2Total.Load(); got != 2 {
		t.Fatalf("upstream_cx_http2_total = %d, want 2 (two dials)", got)
	}
}

// (Task 3, case 2) Promote-skip: a queued waiter + the ONLY free-slot conn is
// draining → h2PromoteLocked does NOT hand that conn; it falls through to the
// permit/dial-grant path (cc:nil). Mirrors TestH2PoolPromoteSkipsClosedConn but
// for the GoneAway() (not Closed()) predicate, on a real GOAWAY'd conn.
func TestAcquireH2Stream_PromoteSkipsDrainingConn(t *testing.T) {
	b := newGoawayBackend(t)
	defer b.Close()
	ep := b.endpoint()
	addr := ep.Addr()
	c := mkLifecycleH2Cluster(t, ep, 4, 4, 4) // C=4, a free permit remains

	// Dial conn #1 (real GOAWAY-able conn) and immediately release its stream so
	// it has inFlight==0 < C (a "free" slot the promote scan would otherwise hand).
	cc1, rel1, _, err := c.AcquireH2Stream(context.Background())
	if err != nil {
		t.Fatalf("acquire #1: %v", err)
	}
	rel1()

	pollUntil(t, func() bool { return b.connCount() >= 1 }, "backend should accept conn #1")
	b.sendGOAWAY(t, 0)
	pollGoneAway(t, cc1)

	// Enqueue a waiter + promote: the only free-slot conn is draining → the
	// promote must skip it and hand a dial-grant (cc:nil), reserving a permit.
	// activeConns reads are guarded by h2PoolMu: since Task 4, watchDrain evicts
	// the idle GOAWAY'd conn in the BACKGROUND (evictH2ConnLocked→cc.Close→connDec
	// →releaseConn writes activeConns-- under the lock), so an unguarded read
	// races it. Snapshot under the lock (mirrors TestAcquireH2Stream_DrainCloseOnLastRelease).
	p := c.circuitBreaker.pool
	c.h2PoolMu.Lock()
	activeBefore := p.activeConns
	c.h2PoolMu.Unlock()
	w := &h2Waiter{ch: make(chan h2Grant, 1)}
	c.h2PoolMu.Lock()
	c.enqueueWaiterLocked(addr, w)
	c.h2PromoteLocked(addr)
	c.h2PoolMu.Unlock()

	select {
	case g := <-w.ch:
		if g.cc != nil {
			t.Fatalf("promote handed the draining conn %p instead of a dial-grant", g.cc)
		}
	default:
		t.Fatal("promote sent no grant (expected a dial-grant past the draining conn)")
	}
	c.h2PoolMu.Lock()
	activeAfter := p.activeConns
	c.h2PoolMu.Unlock()
	if activeAfter != activeBefore+1 {
		t.Fatalf("dial-grant: activeConns = %d, want %d (a permit reserved)", activeAfter, activeBefore+1)
	}
}

// (Task 3, case 3) In-flight drain-close: a draining conn with inFlight==1; its
// release() must evict + Close it (the generalized (Closed||GoneAway)&&inFlight==0
// eager-close) and free its permit (upstream_cx_active Decs, activeConns back to
// pre-acquire). Before the eviction generalization the conn (Closed()==false)
// would survive the release and leak.
func TestAcquireH2Stream_DrainCloseOnLastRelease(t *testing.T) {
	b := newGoawayBackend(t)
	defer b.Close()
	ep := b.endpoint()
	addr := ep.Addr()
	c := mkLifecycleH2Cluster(t, ep, 2, 4, 4)
	p := c.circuitBreaker.pool

	cc, release, _, err := c.AcquireH2Stream(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if got := c.upstreamCxActive.Load(); got != 1 {
		t.Fatalf("after acquire: upstream_cx_active = %d, want 1", got)
	}
	c.h2PoolMu.Lock()
	acBefore := p.activeConns
	c.h2PoolMu.Unlock()
	if acBefore != 1 {
		t.Fatalf("after acquire: activeConns = %d, want 1", acBefore)
	}

	// Drive the peer GOAWAY: the conn is now draining (GoneAway()==true) but NOT
	// Closed() (its in-flight stream keeps it alive).
	pollUntil(t, func() bool { return b.connCount() >= 1 }, "backend should accept the conn")
	b.sendGOAWAY(t, 0)
	pollGoneAway(t, cc)
	if cc.Closed() {
		t.Fatal("draining conn is Closed() before its last stream released (GoneAway should not imply Closed)")
	}

	// Releasing the last in-flight stream of a draining conn must evict + Close it.
	release()

	if n := poolConnCount(c, addr); n != 0 {
		t.Fatalf("after release of draining conn's last stream: pool conns = %d, want 0 (evicted)", n)
	}
	if !cc.Closed() {
		t.Fatal("draining conn was not Closed() on last-stream release (eviction generalization failed)")
	}
	c.h2PoolMu.Lock()
	acAfter := p.activeConns
	c.h2PoolMu.Unlock()
	if acAfter != 0 {
		t.Fatalf("after drain-close: activeConns = %d, want 0 (permit freed)", acAfter)
	}
	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Fatalf("after drain-close: upstream_cx_active = %d, want 0", got)
	}
	if got := c.http2StreamsActive.Load(); got != 0 {
		t.Fatalf("after drain-close: streams_active = %d, want 0", got)
	}
}

// (1) tryAcquireConnSlot: true under cap (activeConns++), false at cap (NO
// enqueue — connPool.waiters unchanged), with the AMEND-CP1 soft-signal parity
// (cx_open set + upstream_cx_overflow Inc on the at-cap failure).
func TestH2PoolTryAcquireConnSlot(t *testing.T) {
	c := newTestH2Cluster(2, 4, 4)
	p := c.circuitBreaker.pool

	if !c.tryAcquireConnSlot() {
		t.Fatal("acquire #1 under cap: want true")
	}
	if p.activeConns != 1 {
		t.Fatalf("activeConns = %d, want 1", p.activeConns)
	}
	if got := p.cxOpen.Load(); got != 0 {
		t.Fatalf("after #1 (under cap): cxOpen = %d, want 0", got)
	}
	if !c.tryAcquireConnSlot() {
		t.Fatal("acquire #2 (reaches cap): want true")
	}
	if p.activeConns != 2 {
		t.Fatalf("activeConns = %d, want 2", p.activeConns)
	}
	if got := p.cxOpen.Load(); got != 1 {
		t.Fatalf("after #2 (at cap): cxOpen = %d, want 1", got)
	}

	// #3 at cap: false, no permit, NO enqueue, soft signals raised.
	waitersBefore := len(p.waiters)
	if c.tryAcquireConnSlot() {
		t.Fatal("acquire #3 at cap: want false")
	}
	if p.activeConns != 2 {
		t.Fatalf("after at-cap failure: activeConns = %d, want 2 (no reserve)", p.activeConns)
	}
	if len(p.waiters) != waitersBefore {
		t.Fatalf("at-cap failure enqueued in connPool.waiters: len %d->%d (must NOT enqueue)",
			waitersBefore, len(p.waiters))
	}
	if got := p.cxOpen.Load(); got != 1 {
		t.Fatalf("at-cap failure: cxOpen = %d, want 1", got)
	}
	if got := p.upstreamCxOverflow.Load(); got != 1 {
		t.Fatalf("at-cap failure: upstreamCxOverflow = %d, want 1", got)
	}
}

// (2) nil-pool cluster: tryAcquireConnSlot always true (no gating).
func TestH2PoolTryAcquireConnSlotNoPool(t *testing.T) {
	c := newNoPoolH2Cluster(4)
	for i := 0; i < 100; i++ {
		if !c.tryAcquireConnSlot() {
			t.Fatalf("no-pool cluster acquire #%d: want true (unbounded)", i)
		}
	}
}

// (3) h2PromoteLocked with a queued waiter + a LIVE conn with a free stream slot:
// hands h2Grant{cc:<that conn>}, inFlight++, streams_active++, NO permit movement.
func TestH2PoolPromoteStreamHandoff(t *testing.T) {
	c := newTestH2Cluster(1, 4, 4)
	p := c.circuitBreaker.pool
	const addr = "10.0.0.1:443"

	cc := newTestH2ClientConn(t)
	pc := &pooledH2Conn{cc: cc, inFlight: 1}
	c.h2Pool[addr] = []*pooledH2Conn{pc}

	w := &h2Waiter{ch: make(chan h2Grant, 1)}
	c.h2PoolMu.Lock()
	c.enqueueWaiterLocked(addr, w)
	if got := p.upstreamRqPendingActive.Load(); got != 1 {
		c.h2PoolMu.Unlock()
		t.Fatalf("after enqueue: pendingActive = %d, want 1", got)
	}
	activeBefore := p.activeConns
	c.h2PromoteLocked(addr)
	c.h2PoolMu.Unlock()

	select {
	case g := <-w.ch:
		if g.cc != cc {
			t.Fatalf("stream handoff: grant.cc = %v, want the pooled conn %v", g.cc, cc)
		}
	default:
		t.Fatal("stream handoff: no grant sent")
	}
	if pc.inFlight != 2 {
		t.Fatalf("after handoff: inFlight = %d, want 2", pc.inFlight)
	}
	if got := c.http2StreamsActive.Load(); got != 1 {
		t.Fatalf("after handoff: streams_active = %d, want 1", got)
	}
	if p.activeConns != activeBefore {
		t.Fatalf("stream handoff moved a permit: activeConns %d->%d", activeBefore, p.activeConns)
	}
	if got := p.upstreamRqPendingActive.Load(); got != 0 {
		t.Fatalf("after handoff: pendingActive = %d, want 0", got)
	}
}

// (4) h2PromoteLocked with a queued waiter + all conns saturated + a FREE permit:
// hands h2Grant{cc:nil} (dial-grant), reserves the permit (activeConns++).
func TestH2PoolPromoteDialGrant(t *testing.T) {
	c := newTestH2Cluster(2, 4, 1) // C=1
	p := c.circuitBreaker.pool
	const addr = "10.0.0.2:443"

	cc := newTestH2ClientConn(t)
	pc := &pooledH2Conn{cc: cc, inFlight: 1} // saturated at C=1
	c.h2Pool[addr] = []*pooledH2Conn{pc}
	// One permit already used by the existing conn.
	if !c.tryAcquireConnSlot() {
		t.Fatal("seed: reserve permit for existing conn")
	}
	if p.activeConns != 1 {
		t.Fatalf("seed: activeConns = %d, want 1", p.activeConns)
	}

	w := &h2Waiter{ch: make(chan h2Grant, 1)}
	c.h2PoolMu.Lock()
	c.enqueueWaiterLocked(addr, w)
	c.h2PromoteLocked(addr)
	c.h2PoolMu.Unlock()

	select {
	case g := <-w.ch:
		if g.cc != nil {
			t.Fatalf("dial-grant: grant.cc = %v, want nil", g.cc)
		}
	default:
		t.Fatal("dial-grant: no grant sent")
	}
	if p.activeConns != 2 {
		t.Fatalf("after dial-grant: activeConns = %d, want 2 (permit reserved)", p.activeConns)
	}
	if pc.inFlight != 1 {
		t.Fatalf("dial-grant must not touch existing inFlight: got %d, want 1", pc.inFlight)
	}
	if got := p.upstreamRqPendingActive.Load(); got != 0 {
		t.Fatalf("after dial-grant: pendingActive = %d, want 0", got)
	}
}

// (5) h2PromoteLocked with a queued waiter + all conns saturated + NO permit:
// leaves the waiter queued (no grant sent).
func TestH2PoolPromoteLeavesQueued(t *testing.T) {
	c := newTestH2Cluster(1, 4, 1) // C=1, maxConns=1
	p := c.circuitBreaker.pool
	const addr = "10.0.0.3:443"

	cc := newTestH2ClientConn(t)
	pc := &pooledH2Conn{cc: cc, inFlight: 1} // saturated
	c.h2Pool[addr] = []*pooledH2Conn{pc}
	if !c.tryAcquireConnSlot() { // consume the only permit
		t.Fatal("seed: reserve the single permit")
	}

	w := &h2Waiter{ch: make(chan h2Grant, 1)}
	c.h2PoolMu.Lock()
	c.enqueueWaiterLocked(addr, w)
	c.h2PromoteLocked(addr)
	queuedAfter := len(c.h2Waiters[addr])
	c.h2PoolMu.Unlock()

	select {
	case g := <-w.ch:
		t.Fatalf("no-capacity promote handed a grant: %v", g)
	default:
	}
	if queuedAfter != 1 {
		t.Fatalf("waiter dropped from queue: len = %d, want 1", queuedAfter)
	}
	if p.activeConns != 1 {
		t.Fatalf("no-capacity promote moved a permit: activeConns = %d, want 1", p.activeConns)
	}
	if got := p.upstreamRqPendingActive.Load(); got != 1 {
		t.Fatalf("waiter still queued: pendingActive = %d, want 1", got)
	}
}

// (6) D-H2-EVICTORDER: the ONLY free-slot conn is Closed() → promote must NOT
// hand it; it falls through to the permit (dial-grant) path.
func TestH2PoolPromoteSkipsClosedConn(t *testing.T) {
	c := newTestH2Cluster(2, 4, 4) // C=4, the closed conn has a "free" slot
	p := c.circuitBreaker.pool
	const addr = "10.0.0.4:443"

	dead := newTestH2ClientConn(t)
	_ = dead.Close() // now Closed() == true, but inFlight(0) < C(4)
	pc := &pooledH2Conn{cc: dead, inFlight: 0}
	c.h2Pool[addr] = []*pooledH2Conn{pc}

	w := &h2Waiter{ch: make(chan h2Grant, 1)}
	c.h2PoolMu.Lock()
	c.enqueueWaiterLocked(addr, w)
	c.h2PromoteLocked(addr)
	c.h2PoolMu.Unlock()

	select {
	case g := <-w.ch:
		if g.cc != nil {
			t.Fatalf("EVICTORDER: handed a Closed conn %v instead of a dial-grant", g.cc)
		}
		// dial-grant: a permit must have been reserved.
		if p.activeConns != 1 {
			t.Fatalf("EVICTORDER dial-grant: activeConns = %d, want 1", p.activeConns)
		}
	default:
		t.Fatal("EVICTORDER: no grant sent (expected a dial-grant past the dead conn)")
	}
	if pc.inFlight != 0 {
		t.Fatalf("EVICTORDER: incremented inFlight on the Closed conn (got %d, want 0)", pc.inFlight)
	}
}

// (8) ctx-cancel while pending: the helper enqueue + cancel-cleanup primitive.
// A clean removal decrements pending_active; a raced grant (already dequeued)
// is drained-and-given-back (stream-grant → release the slot; dial-grant →
// release the permit). Exercised directly here in isolation; the lifecycle
// (Task 5) wires it into AcquireH2Stream.
func TestH2PoolWaiterCancelCleanRemoval(t *testing.T) {
	c := newTestH2Cluster(1, 4, 1)
	p := c.circuitBreaker.pool
	const addr = "10.0.0.5:443"

	w := &h2Waiter{ch: make(chan h2Grant, 1)}
	c.h2PoolMu.Lock()
	c.enqueueWaiterLocked(addr, w)
	c.h2PoolMu.Unlock()
	if got := p.upstreamRqPendingActive.Load(); got != 1 {
		t.Fatalf("after enqueue: pendingActive = %d, want 1", got)
	}

	// Clean removal: no grant was sent, so removeH2WaiterLocked finds it.
	c.h2PoolMu.Lock()
	removed := c.removeH2WaiterLocked(addr, w)
	c.h2PoolMu.Unlock()
	if !removed {
		t.Fatal("removeH2WaiterLocked: want true (waiter still queued)")
	}
	if got := p.upstreamRqPendingActive.Load(); got != 0 {
		t.Fatalf("after clean removal: pendingActive = %d, want 0", got)
	}
	if len(c.h2Waiters[addr]) != 0 {
		t.Fatalf("after clean removal: queue len = %d, want 0", len(c.h2Waiters[addr]))
	}
}

// (8-variant) Race the ctx-cancel-style removal against h2PromoteLocked many
// times to exercise the drain-and-give-back branch. Model: a head waiter is
// queued; a permit is free (so promote hands a dial-grant); cancel and promote
// fire concurrently. Either removeH2WaiterLocked wins (clean removal: no grant)
// or promote wins (grant en route: the waiter drains ch + gives the permit
// back). The load-bearing invariant: activeConns + pending_active both return
// to 0 at quiescence regardless of who won — no permit leak, no lost wakeup.
func TestH2PoolWaiterCancelDialGrantRace(t *testing.T) {
	const iters = 3000
	const addr = "10.0.0.6:443"
	cleanHits := 0
	grantHits := 0
	for i := 0; i < iters; i++ {
		c := newTestH2Cluster(1, 4, 1) // one free permit, no conns → promote dial-grants
		p := c.circuitBreaker.pool

		w := &h2Waiter{ch: make(chan h2Grant, 1)}
		c.h2PoolMu.Lock()
		c.enqueueWaiterLocked(addr, w)
		c.h2PoolMu.Unlock()

		// Race: a "cancel" (remove-or-drain) against a promote (dial-grant).
		grantedToWaiter := make(chan bool, 1)
		var wg sync.WaitGroup
		wg.Add(2)
		// The canceller: try clean removal; if already dequeued, drain the grant
		// and give the permit back (the 43.1 drain-give-back discipline).
		go func() {
			defer wg.Done()
			c.h2PoolMu.Lock()
			if c.removeH2WaiterLocked(addr, w) {
				c.h2PoolMu.Unlock()
				grantedToWaiter <- false
				return
			}
			c.h2PoolMu.Unlock()
			// Already dequeued → a grant is en route on the buffered channel.
			g := <-w.ch
			if g.cc == nil {
				c.releaseConnSlot() // dial-grant: give the permit back
			}
			grantedToWaiter <- true
		}()
		// The promoter.
		go func() {
			defer wg.Done()
			c.h2PoolMu.Lock()
			c.h2PromoteLocked(addr)
			c.h2PoolMu.Unlock()
		}()
		wg.Wait()

		if <-grantedToWaiter {
			grantHits++
		} else {
			cleanHits++
			// Clean removal won → the promote, finding an empty queue, did nothing.
			// (If the promote ran first it would have dequeued, so the canceller
			// would have taken the drain branch instead.)
		}

		if got := p.activeConns; got != 0 {
			t.Fatalf("iter %d: activeConns = %d, want 0 (permit leak)", i, got)
		}
		if got := p.upstreamRqPendingActive.Load(); got != 0 {
			t.Fatalf("iter %d: pendingActive = %d, want 0", i, got)
		}
		if len(c.h2Waiters[addr]) != 0 {
			t.Fatalf("iter %d: waiter still queued after race", i)
		}
	}
	t.Logf("race outcomes over %d iters: clean-removal=%d, drain-give-back=%d", iters, cleanHits, grantHits)
	if cleanHits == 0 {
		t.Fatal("never hit clean-removal — race not exercised (vacuous)")
	}
	if grantHits == 0 {
		t.Fatal("never hit drain-give-back — race not exercised (vacuous)")
	}
}

// (7) Race matrix: N goroutines enqueue-and-wait (each acquires a stream slot or
// dial-grant via the promote path) while the freed slots/permits are recycled by
// release+promote. Assert no lost wakeups (every waiter is served), no double-
// grant (each grant consumed once), and streams_active/inFlight/activeConns all
// return to 0 at quiescence. Models the 43.1 drain-give-back race shape on the
// stream-aware queue.
//
// Setup: ONE live conn with C stream slots + maxConns=1 (no extra conn can dial).
// W workers each: enqueue a waiter, wait for a stream-grant on the conn, "use"
// the slot, then release it (inFlight--, streams_active--, promote the next).
// Because every grant here is a stream-grant on the single conn, this stresses
// the stream-slot handoff + recycle path under -race.
func TestH2PoolStreamRecycleRace(t *testing.T) {
	const (
		C       = 4
		workers = 200
	)
	c := newTestH2Cluster(1, int64(workers)+8, C)
	p := c.circuitBreaker.pool
	const addr = "10.0.0.7:443"

	cc := newTestH2ClientConn(t)
	pc := &pooledH2Conn{cc: cc, inFlight: 0}
	c.h2Pool[addr] = []*pooledH2Conn{pc}
	// Reserve the single permit for the existing conn so promote can only ever
	// hand stream slots (never a dial-grant).
	if !c.tryAcquireConnSlot() {
		t.Fatal("seed permit for the lone conn")
	}

	var peakInFlight atomic.Int64
	var served atomic.Int64

	acquire := func() {
		w := &h2Waiter{ch: make(chan h2Grant, 1)}
		c.h2PoolMu.Lock()
		// Fast path: a free slot now? grab it directly. Else enqueue + promote
		// will hand it when a slot frees.
		if pc.inFlight < C && !cc.Closed() {
			pc.inFlight++
			c.h2StreamsActiveInc()
		} else {
			c.enqueueWaiterLocked(addr, w)
			c.h2PoolMu.Unlock()
			<-w.ch // a stream-grant rides the lone conn
			c.h2PoolMu.Lock()
		}
		// record peak under the lock (inFlight is lock-guarded)
		n := pc.inFlight
		c.h2PoolMu.Unlock()
		for {
			old := peakInFlight.Load()
			if n <= old || peakInFlight.CompareAndSwap(old, n) {
				break
			}
		}
		served.Add(1)
		// release the slot + wake the next waiter.
		c.h2PoolMu.Lock()
		pc.inFlight--
		c.h2StreamsActiveDec()
		c.h2PromoteLocked(addr)
		c.h2PoolMu.Unlock()
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			acquire()
		}()
	}
	doneCh := make(chan struct{})
	go func() { wg.Wait(); close(doneCh) }()
	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		t.Fatalf("race matrix deadlocked: served %d/%d", served.Load(), workers)
	}

	if got := served.Load(); got != workers {
		t.Fatalf("served = %d, want %d (lost wakeup)", got, workers)
	}
	if got := peakInFlight.Load(); got > C {
		t.Fatalf("peak inFlight = %d, exceeds C = %d (double-grant)", got, C)
	}
	if pc.inFlight != 0 {
		t.Fatalf("after quiescence: inFlight = %d, want 0", pc.inFlight)
	}
	if got := c.http2StreamsActive.Load(); got != 0 {
		t.Fatalf("after quiescence: streams_active = %d, want 0", got)
	}
	if got := p.upstreamRqPendingActive.Load(); got != 0 {
		t.Fatalf("after quiescence: pendingActive = %d, want 0", got)
	}
	// activeConns stays at 1 (the lone conn's permit is never released here).
	if p.activeConns != 1 {
		t.Fatalf("after quiescence: activeConns = %d, want 1", p.activeConns)
	}
}

// Task 4 (phase 43.2b): per-conn drain-watcher goroutine (watchDrain).
// ---------------------------------------------------------------------------

// (Task 4, case 1) Idle prompt-close: a pooled conn made IDLE (inFlight==0 via a
// release of its only stream) then driven to a peer GOAWAY. No release() will
// ever fire again and the conn's ctx is never canceled (GoneAway, not Closed),
// so ONLY the per-conn watchDrain goroutine can close it. Poll an observable
// (the conn leaves the pool + upstream_cx_active Decs) — proving the watcher,
// not a release(), evicted+Closed the idle draining conn.
func TestH2PoolWatcherIdlePromptClose(t *testing.T) {
	b := newGoawayBackend(t)
	defer b.Close()
	ep := b.endpoint()
	addr := ep.Addr()
	c := mkLifecycleH2Cluster(t, ep, 2, 4, 4)
	p := c.circuitBreaker.pool

	// Dial conn #1, then release its only stream → the conn is now IDLE
	// (inFlight==0) but still pooled (a non-draining release leaves it pooled).
	cc, release, _, err := c.AcquireH2Stream(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	release()
	if got := c.upstreamCxActive.Load(); got != 1 {
		t.Fatalf("after idle release: upstream_cx_active = %d, want 1 (conn still pooled)", got)
	}
	if n := poolConnCount(c, addr); n != 1 {
		t.Fatalf("after idle release: pool conns = %d, want 1 (idle, not yet drained)", n)
	}

	// Drive the peer GOAWAY on the now-idle conn. No release() can fire (no
	// in-flight stream) → the watcher is the SOLE closer.
	pollUntil(t, func() bool { return b.connCount() >= 1 }, "backend should accept the conn")
	b.sendGOAWAY(t, 0)

	// Poll until the watcher evicts the idle draining conn (no sleep-as-sync).
	pollUntil(t, func() bool { return poolConnCount(c, addr) == 0 },
		"watcher should evict the idle draining conn (no release() drove it)")
	pollUntil(t, func() bool { return c.upstreamCxActive.Load() == 0 },
		"watcher eviction should Dec upstream_cx_active (Close → connDec → releaseConn)")

	if !cc.Closed() {
		t.Fatal("watcher did not Close() the idle draining conn")
	}
	c.h2PoolMu.Lock()
	stillPooled := c.findPooledLocked(addr, cc)
	acAfter := p.activeConns
	c.h2PoolMu.Unlock()
	if stillPooled != nil {
		t.Fatal("idle draining conn still pooled after the watcher should have evicted it")
	}
	if acAfter != 0 {
		t.Fatalf("after watcher close: activeConns = %d, want 0 (permit freed)", acAfter)
	}
}

// (Task 4, case 2) In-flight → watcher no-op: a pooled conn with inFlight==1
// driven to GOAWAY. The watcher's `inFlight == 0` re-check is FALSE, so it does
// NOT close the conn (the last release()'s generalized eager-close, Task 3, owns
// that). Assert the conn STAYS pooled while in-flight; then release() evicts it.
func TestH2PoolWatcherInFlightNoOp(t *testing.T) {
	b := newGoawayBackend(t)
	defer b.Close()
	ep := b.endpoint()
	addr := ep.Addr()
	c := mkLifecycleH2Cluster(t, ep, 2, 4, 4)

	// Dial conn #1 and HOLD its stream (inFlight==1).
	cc, release, _, err := c.AcquireH2Stream(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	pollUntil(t, func() bool { return b.connCount() >= 1 }, "backend should accept the conn")
	b.sendGOAWAY(t, 0)
	pollGoneAway(t, cc)

	// The watcher fired (GoneAwayCh closed) but inFlight==1 → its re-check is
	// false → no-op. The conn must STAY pooled (the in-flight stream keeps it
	// alive; only its last release() evicts it).
	if cc.Closed() {
		t.Fatal("watcher closed an IN-FLIGHT draining conn (inFlight re-check failed)")
	}
	// Re-check across a few scheduler turns that the conn is still pooled.
	for i := 0; i < 50; i++ {
		if n := poolConnCount(c, addr); n != 1 {
			t.Fatalf("watcher evicted an in-flight draining conn: pool conns = %d, want 1", n)
		}
		runtime.Gosched()
	}

	// Now release the last stream → the Task-3 path evicts + Closes it.
	release()
	pollUntil(t, func() bool { return poolConnCount(c, addr) == 0 },
		"release of the draining conn's last stream should evict it (Task-3 eager-close)")
	if !cc.Closed() {
		t.Fatal("draining conn not Closed() on last-stream release")
	}
	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Fatalf("after last release: upstream_cx_active = %d, want 0", got)
	}
}

// (Task 4, case 3) Evicted-by-another-path → no leak / no double-evict. A tight
// loop pools a real conn, then RACES EvictH2ConnOnError (cancels the ctx →
// Done() fires) against a peer GOAWAY (GoneAwayCh fires). The watcher's
// findPooledLocked(...) != nil guard under h2PoolMu must dodge the double-evict,
// and its <-Done() arm must guarantee it exits (no goroutine leak). Run under
// -race -count=1: any data race here is a real bug. The watcher-goroutine count
// returning to baseline at quiescence proves no leak.
func TestH2PoolWatcherEvictRaceNoLeak(t *testing.T) {
	b := newGoawayBackend(t)
	defer b.Close()
	ep := b.endpoint()
	addr := ep.Addr()

	const iters = 1000
	baseGoroutines := runtime.NumGoroutine()
	for i := 0; i < iters; i++ {
		// maxConns=1 so each iteration pools exactly one conn (and frees the
		// permit on eviction, ready for the next iteration's dial).
		c := mkLifecycleH2Cluster(t, ep, 1, 4, 4)

		before := b.connCount()
		cc, release, _, err := c.AcquireH2Stream(context.Background())
		if err != nil {
			t.Fatalf("iter %d acquire: %v", i, err)
		}
		// Make the conn idle so neither release nor evict is gated by inFlight,
		// maximizing the eviction overlap window.
		release()
		// The backend appends THIS iteration's server conn; target it for GOAWAY.
		pollUntil(t, func() bool { return b.connCount() > before }, "backend should accept this iteration's conn")
		idx := b.connCount() - 1

		var wg sync.WaitGroup
		wg.Add(2)
		// Racer A: the OTHER eviction path (cancels ctx → Done() fires).
		go func() {
			defer wg.Done()
			c.EvictH2ConnOnError(cc, ep)
		}()
		// Racer B: a peer GOAWAY (GoneAwayCh fires → the watcher's other arm).
		go func() {
			defer wg.Done()
			b.sendGOAWAYBestEffort(idx)
		}()
		wg.Wait()

		// Drain to quiescence: whoever won, the conn must be evicted exactly once
		// and Closed; the pool must be empty; the watcher must have exited.
		pollUntil(t, func() bool { return poolConnCount(c, addr) == 0 },
			"conn must be evicted exactly once (no surviving pooled conn)")
		if !cc.Closed() {
			t.Fatalf("iter %d: conn not Closed after the evict/GOAWAY race", i)
		}
	}

	// No goroutine leak: every watcher took its <-Done() (or GOAWAY) arm and
	// exited. Allow a small slack for the backend's transient accept goroutines.
	pollUntil(t, func() bool { return runtime.NumGoroutine() <= baseGoroutines+8 },
		"watcher goroutines should drain to baseline (no leak)")
}

// sendGOAWAYBestEffort writes a graceful GOAWAY(NO_ERROR) on the idx-th server
// conn and SWALLOWS any error (a broken-pipe is expected when a concurrent
// eviction Closed the client conn first). For use inside racer goroutines where
// t.Fatalf is illegal (must be called from the test goroutine).
func (b *goawayBackend) sendGOAWAYBestEffort(idx int) {
	b.mu.Lock()
	if idx < 0 || idx >= len(b.conns) {
		b.mu.Unlock()
		return
	}
	conn := b.conns[idx]
	b.mu.Unlock()
	fr := http2.NewFramer(conn, conn)
	_ = fr.WriteGoAway(1<<30, http2.ErrCodeNo, nil)
}
