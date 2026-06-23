package cluster

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// rearmHoldBackend is an in-process h2c hold backend mirroring the 0079
// H2HoldResponder: each normal GET blocks on a re-armable gate until /__release
// fires (close-and-swap). It lets a cluster-level integration test reproduce the
// 0079 overflow prong (C=1, maxConns=1, maxPending=1) with REAL upstream
// RoundTrips through AcquireH2Stream — pend → wake → ride-the-same-conn.
type rearmHoldBackend struct {
	ln   net.Listener
	mu   sync.Mutex
	gate chan struct{}
}

func newRearmHoldBackend(t *testing.T) *rearmHoldBackend {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	b := &rearmHoldBackend{ln: ln, gate: make(chan struct{})}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__release" {
			b.mu.Lock()
			old := b.gate
			b.gate = make(chan struct{})
			b.mu.Unlock()
			close(old)
			w.WriteHeader(http.StatusOK)
			return
		}
		b.mu.Lock()
		g := b.gate
		b.mu.Unlock()
		<-g
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Handler: h2c.NewHandler(h, &http2.Server{MaxConcurrentStreams: 1000})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = ln.Close() })
	return b
}

func (b *rearmHoldBackend) ep() Endpoint { return endpointFromAddr(b.ln.Addr()) }
func (b *rearmHoldBackend) release(t *testing.T) {
	t.Helper()
	releaseAddr := b.ln.Addr().String()
	// h2c prior-knowledge GET /__release via our own client conn.
	raw, err := net.Dial("tcp", releaseAddr)
	if err != nil {
		t.Fatalf("release dial: %v", err)
	}
	cc, err := h2.NewClientConn(context.Background(), raw)
	if err != nil {
		t.Fatalf("release client conn: %v", err)
	}
	defer func() { _ = cc.Close() }()
	if _, err := cc.RoundTrip(context.Background(), h2.H2Request{
		Method: "GET", Path: "/__release", Scheme: "http", Authority: releaseAddr,
	}); err != nil {
		t.Fatalf("release roundtrip: %v", err)
	}
}

// TestAcquireH2Stream_OverflowProng_WokenPendingRoundTrips reproduces the 0079
// overflow prong at the cluster level with REAL upstream RoundTrips: C=1,
// maxConns=1, maxPending=1. One held filler occupies the C=1 conn; a 2nd PENDS;
// release frees the filler → the pending request is woken onto the SAME conn
// (stream-grant) and must complete its OWN RoundTrip cleanly (200), with
// streams_active + pending_active draining to 0. This is the path the 0079
// overflow prong exercised; it must not EOF/hang after Task 9.5.
func TestAcquireH2Stream_OverflowProng_WokenPendingRoundTrips(t *testing.T) {
	b := newRearmHoldBackend(t)
	ep := b.ep()
	c := mkLifecycleH2Cluster(t, ep, 1, 1, 1) // C=1, maxConns=1, maxPending=1
	p := c.circuitBreaker.pool
	addr := ep.Addr()

	req := func(path string) h2.H2Request {
		return h2.H2Request{Method: "GET", Path: path, Scheme: "http", Authority: addr}
	}

	// Held filler: acquire + RoundTrip in a goroutine (blocks on the gate).
	fillerDone := make(chan error, 1)
	go func() {
		cc, rel, _, err := c.AcquireH2Stream(context.Background())
		if err != nil {
			fillerDone <- err
			return
		}
		_, rerr := cc.RoundTrip(context.Background(), req("/of/0"))
		rel()
		fillerDone <- rerr
	}()
	pollUntil(t, func() bool { return c.http2StreamsActive.Load() == 1 },
		"filler should occupy the C=1 conn (streams_active==1)")

	// Pending request: acquire (PENDS — no permit) then RoundTrip once woken.
	pendDone := make(chan error, 1)
	go func() {
		cc, rel, _, err := c.AcquireH2Stream(context.Background())
		if err != nil {
			pendDone <- err
			return
		}
		_, rerr := cc.RoundTrip(context.Background(), req("/of/1"))
		rel()
		pendDone <- rerr
	}()
	pollUntil(t, func() bool { return p.upstreamRqPendingActive.Load() == 1 },
		"2nd request should PEND (pending_active==1)")

	// Release → the filler completes → the pending is woken onto the SAME conn.
	// The pending's stream then reaches the backend; release AGAIN so it too
	// drains (the 2nd stream re-blocks on the re-armed gate — mirrors the fixture
	// shape where the woken request needs the gate open).
	b.release(t)
	if err := <-fillerDone; err != nil {
		t.Fatalf("filler RoundTrip: %v", err)
	}
	// The pending request was promoted onto the same conn; release once more so
	// its (re-blocked) stream is served.
	pollUntil(t, func() bool { return c.http2StreamsActive.Load() == 1 },
		"woken pending should ride the same conn (streams_active==1)")
	b.release(t)

	select {
	case err := <-pendDone:
		if err != nil {
			t.Fatalf("woken pending RoundTrip: %v (the 0079-overflow EOF reproduction)", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("woken pending RoundTrip never completed (hang)")
	}

	pollUntil(t, func() bool {
		return c.http2StreamsActive.Load() == 0 && p.upstreamRqPendingActive.Load() == 0
	}, "streams_active + pending_active should drain to 0")
	if n := poolConnCount(c, addr); n != 1 {
		t.Fatalf("pool conns = %d, want 1 (woken onto the same conn, no new dial)", n)
	}
}
