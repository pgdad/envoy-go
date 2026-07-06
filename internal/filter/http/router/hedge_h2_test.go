package router

import (
	"bytes"
	"context"
	stdtls "crypto/tls"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	"github.com/pgdad/envoy-go/internal/stats"
)

// heldH2Backend is the H2 analog of heldBackend: a controllable in-process H2
// backend whose per-connection serve loop completes the preface/SETTINGS
// handshake, reads the client HEADERS, then PARKS on a shared release channel
// until the test closes it, after which it writes a 200 HEADERS+DATA(END_STREAM)
// response. Each hedgeExecutorH2 attempt re-dials (the per-request fresh-dial
// shape, ADR-0056), so every held attempt is a distinct parked conn — the test
// can POLL the registry until the expected number of hedges have launched (the
// hedge-trigger timers fire while the attempts are held), THEN close the gate to
// let them all return 200 and unblock the collector. Mirrors heldBackend +
// reference_concurrency_differential_release_barrier (poll a counter, never a
// fixed sleep).
type heldH2Backend struct {
	addr    string
	stop    func()
	release chan struct{}
	conns   int64
}

func startHeldH2Backend(t *testing.T, pki *h2BackendPKI) *heldH2Backend {
	t.Helper()
	pair, err := stdtls.X509KeyPair(pki.leafCertPEM, pki.leafKeyPEM)
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	cfg := &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		NextProtos:   []string{"h2"},
		MinVersion:   stdtls.VersionTLS12,
		MaxVersion:   stdtls.VersionTLS13,
	}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}
	b := &heldH2Backend{addr: ln.Addr().String(), release: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go b.serve(conn)
		}
	}()
	b.stop = func() { _ = ln.Close(); <-done }
	return b
}

// serve completes one H2 handshake, reads the client HEADERS, parks on release,
// then writes a 200. Mirrors runH2Backend's h2BackendOK arm with a release gate
// inserted before the response is written.
func (b *heldH2Backend) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	prefaceBuf := make([]byte, 24)
	if _, err := io.ReadFull(conn, prefaceBuf); err != nil {
		return
	}
	if string(prefaceBuf) != "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n" {
		return
	}
	fr := http2.NewFramer(conn, conn)
	frame, err := fr.ReadFrame()
	if err != nil {
		return
	}
	if _, ok := frame.(*http2.SettingsFrame); !ok {
		return
	}
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16384}); err != nil {
		return
	}
	if _, err := fr.ReadFrame(); err != nil { // client's SETTINGS_ACK
		return
	}
	if err := fr.WriteSettingsAck(); err != nil {
		return
	}
	var streamID uint32
	for {
		frame, err = fr.ReadFrame()
		if err != nil {
			return
		}
		if hf, ok := frame.(*http2.HeadersFrame); ok {
			streamID = hf.StreamID
			break
		}
	}
	atomic.AddInt64(&b.conns, 1)

	<-b.release // PARK until the test opens the gate

	body := []byte("h2-held-ok")
	var hbuf bytes.Buffer
	henc := hpack.NewEncoder(&hbuf)
	_ = henc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
	_ = henc.WriteField(hpack.HeaderField{Name: "content-type", Value: "text/plain"})
	_ = henc.WriteField(hpack.HeaderField{Name: "content-length", Value: strconv.Itoa(len(body))})
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: hbuf.Bytes(),
		EndStream:     false,
		EndHeaders:    true,
	}); err != nil {
		return
	}
	if err := fr.WriteData(streamID, true /* endStream */, body); err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, conn) // linger for the client GOAWAY on Close
}

// hedgeOnPTTActionH2 builds a routerActionH2 wired for the release-gated H2 hedge
// tests: a held H2 backend, hedge_on_per_try_timeout:true, a small perTryTimeout
// so the hedge-trigger timers fire promptly, and num_retries hedge slots. The H2
// twin of hedgeOnPTTAction.
func hedgeOnPTTActionH2(t *testing.T, addr string, pki *h2BackendPKI, numRetries uint32, ptt time.Duration) (*routerActionH2, *stats.Registry) {
	t.Helper()
	c, reg := h2EndpointClusterWithRegistry(t, addr, pki)
	c.EnsureRetryStats()
	a := &routerActionH2{
		cluster: c,
		rp:      mkRetryPolicyPTT(t, "5xx", numRetries, ptt),
		hp:      &HedgePolicy{InitialRequests: 1, HedgeOnPerTryTimeout: true},
	}
	return a, reg
}

// runHedgeReleaseGatedH2 fires hedgeExecutorH2 in a goroutine against a held H2
// backend, polls until num_retries hedges have launched AND the limit is hit,
// then opens the gate and joins. Returns the final status. The H2 twin of
// runHedgeReleaseGated.
func runHedgeReleaseGatedH2(t *testing.T, b *heldH2Backend, a *routerActionH2, reg *stats.Registry, numRetries int64) int {
	t.Helper()
	type out struct {
		status int
		err    error
	}
	done := make(chan out, 1)
	go func() {
		resp, _, err := hedgeExecutorH2(context.Background(), a, h2RequestForTest())
		done <- out{resp.Status, err}
	}()

	pollCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry", numRetries)
	pollCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry_limit_exceeded", 1)

	close(b.release) // let the held attempts return 200

	select {
	case o := <-done:
		if o.err != nil {
			t.Fatalf("hedgeExecutorH2: %v", o.err)
		}
		return o.status
	case <-time.After(5 * time.Second):
		t.Fatal("hedgeExecutorH2 did not return after release")
		return 0
	}
}

// TestHedgeExecutorH2_HedgeOnPerTryTimeout_OriginalWins — held 200 H2 backend,
// hedge_on_per_try_timeout, num_retries:3, small per_try_timeout. Poll until
// retry==3 && limit_exceeded==1; release ⇒ final 200, upstream_rq_retry==3, and
// the load-bearing AMEND-H1: upstream_rq_per_try_timeout==0. H2 twin of
// TestHedgeExecutorH1_HedgeOnPerTryTimeout_OriginalWins.
func TestHedgeExecutorH2_HedgeOnPerTryTimeout_OriginalWins(t *testing.T) {
	pki := mkH2BackendPKI(t)
	b := startHeldH2Backend(t, pki)
	defer b.stop()
	a, reg := hedgeOnPTTActionH2(t, b.addr, pki, 3, 8*time.Millisecond)

	status := runHedgeReleaseGatedH2(t, b, a, reg, 3)
	if status != 200 {
		t.Errorf("final status=%d want 200", status)
	}
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry", 3)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry_backoff_exponential", 3)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_per_try_timeout", 0) // AMEND-H1
}

// TestHedgeExecutorH2_HedgeOnPerTryTimeout_NeverIncrementsPerTryTimeout — drives
// the all-held hedge path and asserts the upstream_rq_per_try_timeout delta is 0
// (load-bearing AMEND-H1: the H2 hedge path NEVER calls IncUpstreamRqPerTryTimeout).
func TestHedgeExecutorH2_HedgeOnPerTryTimeout_NeverIncrementsPerTryTimeout(t *testing.T) {
	pki := mkH2BackendPKI(t)
	b := startHeldH2Backend(t, pki)
	defer b.stop()
	a, reg := hedgeOnPTTActionH2(t, b.addr, pki, 2, 8*time.Millisecond)

	before := counterValueOf(reg, "cluster.c_h2_backend.upstream_rq_per_try_timeout")
	_ = runHedgeReleaseGatedH2(t, b, a, reg, 2)
	after := counterValueOf(reg, "cluster.c_h2_backend.upstream_rq_per_try_timeout")
	if after-before != 0 {
		t.Errorf("upstream_rq_per_try_timeout delta=%d want 0 (AMEND-H1)", after-before)
	}
}

// TestHedgeExecutorH2_ClientCancelNotMiscounted — a parent-ctx cancel before any
// attempt completes (a hanging H2 backend + cancel after ~200ms) returns the
// Status:0 CANCEL sentinel as the final result WITHOUT a real acceptable winner
// AND does NOT increment upstream_rq_per_try_timeout (delta 0). Mirrors
// TestRetryExecutorH2_CtxCancelNotRetried's cancel-timing idiom. The H2 driver
// honors ctx.Done(), so cancelAll()/the parent cancel aborts the in-flight
// RoundTrip promptly; matches(0,false)==false ⇒ the Status:0 is returned as final.
func TestHedgeExecutorH2_ClientCancelNotMiscounted(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2Backend(t, pki, h2BackendHang, nil)
	defer func() { _ = ln.Close() }()

	c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	c.EnsureRetryStats()
	// A LONG per_try_timeout so the PARENT cancel fires first (not the hedge
	// trigger); hedge_on_per_try_timeout + retry_on 5xx so a (wrong) per-try-
	// timeout misclassification or retry would be visibly counted.
	a := &routerActionH2{
		cluster: c,
		rp:      mkRetryPolicyPTT(t, "5xx, gateway-error", 3, 5*time.Second),
		hp:      &HedgePolicy{InitialRequests: 1, HedgeOnPerTryTimeout: true},
	}

	before := counterValueOf(reg, "cluster.c_h2_backend.upstream_rq_per_try_timeout")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	resp, _, err := hedgeExecutorH2(ctx, a, h2RequestForTest())
	if err == nil {
		t.Fatal("hedgeExecutorH2 returned nil err; want the *h2.Error ctx-cancel sentinel passed through")
	}
	if _, ok := err.(*h2.Error); !ok {
		t.Fatalf("err is %T, want *h2.Error (ctx-cancel sentinel passed through unchanged)", err)
	}
	if resp.Status != 0 {
		t.Errorf("status = %d, want 0 (ctx-cancel sentinel, NOT an acceptable winner)", resp.Status)
	}
	after := counterValueOf(reg, "cluster.c_h2_backend.upstream_rq_per_try_timeout")
	if after-before != 0 {
		t.Errorf("upstream_rq_per_try_timeout delta=%d want 0 (a client cancel is never a per-try-timeout)", after-before)
	}
}
