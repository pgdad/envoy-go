package router

import (
	"context"
	stdtls "crypto/tls"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/http2/hpack"

	"github.com/pgdad/envoy-go/internal/cluster"
)

// ---------------------------------------------------------------------------
// Phase 92 Tasks 13 + 14 — the ROUTER DISPOSITION pins and the COUNTER pin for
// a locally-detected malformed LEADING response header block (ADR-0313).
//
// Every arm here rides the Task-9 caller-supplied-leading-block seam on
// runH2TrailerBackend / startH2TrailerBackend. That seam is what makes the
// shapes reachable at all: the rejected block below carries content-length
// TWICE, which no map-shaped carrier can express and which the pre-seam
// hard-coded block could never emit.
// ---------------------------------------------------------------------------

// malformedLeadingBlock is a LEADING response header block that
// validateResponseHeaders rejects on its duplicate-content-length leg. It is
// otherwise a completely ordinary 200 block — the ONLY defect is the repeated
// field — so an arm that reddens here cannot be explained by a missing
// pseudo-header or an unparseable status.
func malformedLeadingBlock(body []byte) []hpack.HeaderField {
	cl := strconv.Itoa(len(body))
	return []hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
		{Name: "content-length", Value: cl},
		{Name: "content-length", Value: cl},
	}
}

// startCountingH2TrailerBackend is startH2TrailerBackend plus a
// BACKEND-OBSERVED attempt counter, incremented once per ACCEPTED connection
// before the serving goroutine is spawned.
//
// Counting connections is a faithful attempt count on THIS path specifically:
// the malformed-response-headers arm runs EvictH2ConnOnError before it
// returns, so the pooled conn is gone and every retried attempt must dial a
// fresh TCP connection (the PLAN's measured "3 backend-observed attempts on 3
// separate TCP conns"). The count is stable at assertion time because the
// client's TLS + H2 handshake cannot complete until this loop has accepted and
// the serving goroutine has read the preface, and the action under test does
// not return until its last attempt's RoundTrip has completed.
//
// ⚠️ Deliberately measured at the BACKEND rather than from a cluster stat: an
// attempt count read from a counter would be an assertion about a stat this
// test has not proven live. (The upstream_rq_total cross-check below is
// corroboration, not the pin.)
func startCountingH2TrailerBackend(t *testing.T, pki *h2BackendPKI, behavior h2TrailerBehavior, body []byte, trailers, leading []hpack.HeaderField) (net.Listener, *atomic.Int64) {
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
	var accepted atomic.Int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			go runH2TrailerBackend(c, behavior, body, trailers, leading)
		}
	}()
	return ln, &accepted
}

// TestRouterActionH2_MalformedResponseHeadersDisposition is the phase-92 Task
// 13 pin on all THREE properties of the router's new
// h2.ErrMalformedResponseHeaders arm in doH2ClusterAction. Each property gets
// its own t.Errorf — a t.Fatalf would make every later property dead code, so
// a single run would only ever report the first regression.
//
//	(1) 502 DOWNSTREAM, not the trailer sentinel's Status: 0. This is the whole
//	    reason the codec carries a SEPARATE sentinel; reusing
//	    ErrMalformedTrailers would take the arm one above and return Status: 0.
//	(2) NOT RETRIABLE. The route CONFIGURES retry_on: connect-failure with
//	    num_retries: 2 — without a live retry policy this pin cannot fire under
//	    any input and passes vacuously — and the BACKEND must observe exactly
//	    ONE attempt. Setting ActionResponse.localOrigin on the new arm would
//	    classify a perfectly reachable but malformed upstream as a connect
//	    failure and drive 3 attempts.
//	(3) EVICTION, with a STACKED CONTROL. A malformed response must evict the
//	    pooled conn exactly once AND a legal response must evict zero times; a
//	    passing arm alone cannot catch an evict that fires on every response.
func TestRouterActionH2_MalformedResponseHeadersDisposition(t *testing.T) {
	body := []byte("upstream-ok\n")

	// Property 1 (502, not Status: 0) and the FIRST half of property 3
	// (a malformed response evicts exactly once) share one drive.
	t.Run("malformed-502-and-evicts-once", func(t *testing.T) {
		pki := mkH2BackendPKI(t)
		ln := startH2TrailerBackend(t, pki, h2TrailerNone, body, nil, malformedLeadingBlock(body))
		defer func() { _ = ln.Close() }()

		c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
		a := &routerActionH2{cluster: c}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())

		// PROPERTY 1. Status, not merely "an error came back": the trailer
		// sentinel's arm ALSO returns an error, and it returns Status: 0.
		if resp.Status != 502 {
			t.Errorf("PROPERTY 1: status = %d, want 502 (a malformed LEADING block is a 502 local reply, NOT the trailer sentinel's Status: 0 stream reset)", resp.Status)
		}
		if err != nil {
			t.Errorf("PROPERTY 1: err = %v, want nil (the 502 local reply finalizes a response; no *h2.Error travels upward)", err)
		}
		if len(resp.Body) == 0 {
			t.Errorf("PROPERTY 1: body is empty, want the 502 local-reply body")
		}

		// PROPERTY 3, half A. The pool has no exported size accessor, so
		// eviction is observed via REUSE: upstream_cx_http2_total counts dials,
		// so a second request that must RE-DIAL moves it by 1 while one that
		// HITS the pooled conn leaves it alone. Read a BASELINE and assert the
		// DELTA — an absolute would be an assertion about how many dials the
		// first request happened to need.
		beforeDials := counterValue(t, reg, "cluster.c_h2_backend.upstream_cx_http2_total")
		if beforeDials < 0 {
			t.Fatalf("cluster.c_h2_backend.upstream_cx_http2_total is not registered (counterValue = %d); the eviction delta below would be vacuous", beforeDials)
		}
		if _, _, err2 := doH2ClusterAction(ctx, a, h2RequestForTest()); err2 != nil {
			t.Errorf("second request: err = %v, want nil", err2)
		}
		afterDials := counterValue(t, reg, "cluster.c_h2_backend.upstream_cx_http2_total")
		if got := afterDials - beforeDials; got != 1 {
			t.Errorf("PROPERTY 3a: upstream_cx_http2_total delta across a second request = %d (%d -> %d), want 1 (the malformed response must EVICT the pooled conn, forcing a fresh dial)", got, beforeDials, afterDials)
		}
	})

	// Property 3, half B — the STACKED CONTROL. Identical shape, LEGAL leading
	// block. Without this arm an EvictH2ConnOnError that fired on every
	// response (or one hoisted out of the error path entirely) would still
	// satisfy half A.
	t.Run("legal-response-evicts-zero-times", func(t *testing.T) {
		pki := mkH2BackendPKI(t)
		ln := startH2TrailerBackend(t, pki, h2TrailerNone, body, nil, nil /* the default, well-formed leading block */)
		defer func() { _ = ln.Close() }()

		c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
		a := &routerActionH2{cluster: c}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
		if err != nil {
			t.Errorf("first request: err = %v, want nil", err)
		}
		if resp.Status != 200 {
			t.Errorf("first request: status = %d, want 200 (the control arm must actually take the SUCCESS path, or 'no eviction' is measured on the wrong path)", resp.Status)
		}
		beforeDials := counterValue(t, reg, "cluster.c_h2_backend.upstream_cx_http2_total")
		if beforeDials < 0 {
			t.Fatalf("cluster.c_h2_backend.upstream_cx_http2_total is not registered (counterValue = %d); the delta below would be vacuous", beforeDials)
		}
		if _, _, err2 := doH2ClusterAction(ctx, a, h2RequestForTest()); err2 != nil {
			t.Errorf("second request: err = %v, want nil", err2)
		}
		afterDials := counterValue(t, reg, "cluster.c_h2_backend.upstream_cx_http2_total")
		if got := afterDials - beforeDials; got != 0 {
			t.Errorf("PROPERTY 3b: upstream_cx_http2_total delta across a second request = %d (%d -> %d), want 0 (a LEGAL response must NOT evict the pooled conn)", got, beforeDials, afterDials)
		}
	})

	// Property 2 — NOT RETRIABLE, driven through the LIVE retry executor.
	t.Run("not-retried-under-connect-failure-policy", func(t *testing.T) {
		pki := mkH2BackendPKI(t)
		ln, attempts := startCountingH2TrailerBackend(t, pki, h2TrailerNone, body, nil, malformedLeadingBlock(body))
		defer func() { _ = ln.Close() }()

		c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
		// ⚠️ WITHOUT THIS the +5 retry counters are never registered, every Inc
		// is a silent no-op, and the ==0 assertions below — including
		// upstream_rq_retry — are VACUOUS.
		c.EnsureRetryStats()
		// ⚠️ THE POLICY IS LOAD-BEARING. retry_on: connect-failure is exactly
		// the flag that ActionResponse.localOrigin would match
		// (retry.go: rp.on&(retryConnectFail|retryReset) != 0 && localOrigin),
		// and num_retries: 2 gives the misclassification room to show up as 3
		// backend-observed attempts. With no policy at all this pin could not
		// fire under ANY input.
		rp, err := NewRetryPolicy("connect-failure", 2, nil, 0, 0, 0)
		if err != nil {
			t.Fatalf("NewRetryPolicy: %v", err)
		}
		act := H2ClusterAction(c, nil, cluster.SubsetMatch{}, rp, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp, _, aerr := act(ctx, h2RequestForTest())

		// PROPERTY 2, measured at the BACKEND.
		if got := attempts.Load(); got != 1 {
			t.Errorf("PROPERTY 2: backend-observed attempts = %d, want 1 (a malformed response header block is NOT a connect failure; localOrigin must stay UNSET on the new arm)", got)
		}
		// Property 1 again, this time on the far side of retryExecutorH2 — the
		// executor must not launder the 502 into anything else.
		if resp.Status != 502 {
			t.Errorf("PROPERTY 2: status through retryExecutorH2 = %d, want 502", resp.Status)
		}
		if aerr != nil {
			t.Errorf("PROPERTY 2: err through retryExecutorH2 = %v, want nil", aerr)
		}
		if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_rq_retry"); got != 0 {
			t.Errorf("PROPERTY 2: upstream_rq_retry = %d, want 0 (matched no retry_on flag)", got)
		}
		if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_rq_total"); got != 1 {
			t.Errorf("PROPERTY 2: upstream_rq_total = %d, want 1 (exactly one attempt)", got)
		}
	})
}

// TestRouterActionH2_MalformedResponseHeadersIncsRxMessagingError is the
// phase-92 Task 14 pin on cluster.<name>.http2.rx_messaging_error.
//
// ⚠️ SUBJECT-SIDE ONLY. Cross-side stat scope is a known divergence axis and
// the reference spells this event differently; nothing here may be read as a
// cross-side name claim.
//
// ⚠️ THE PIN IS A DELTA FROM A BASELINE READ, never an absolute — an absolute
// passes on a dirty registry and fails on a clean one.
//
// ⚠️ THE REGISTRATION IS ASSERTED FIRST. incCounter swallows a nil handle, so
// a counter that was never registered would make the delta-0 CONTROL below
// pass for entirely the wrong reason. counterValue returns -1 for a name that
// is absent from the registry, so `baseline >= 0` IS the registration
// assertion. (The Cluster field's own non-nil-ness is pinned package-locally
// in internal/cluster/manager_test.go, which can see the unexported handle.)
func TestRouterActionH2_MalformedResponseHeadersIncsRxMessagingError(t *testing.T) {
	const counterName = "cluster.c_h2_backend.http2.rx_messaging_error"
	body := []byte("upstream-ok\n")

	t.Run("malformed-leading-block-incs-by-one", func(t *testing.T) {
		pki := mkH2BackendPKI(t)
		ln := startH2TrailerBackend(t, pki, h2TrailerNone, body, nil, malformedLeadingBlock(body))
		defer func() { _ = ln.Close() }()

		c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
		a := &routerActionH2{cluster: c}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		before := counterValue(t, reg, counterName)
		if before < 0 {
			t.Fatalf("%s is not registered on an H2 cluster (counterValue = %d); a nil handle makes incCounter a silent no-op and every delta assertion vacuous", counterName, before)
		}
		if _, _, err := doH2ClusterAction(ctx, a, h2RequestForTest()); err != nil {
			t.Errorf("doH2ClusterAction: err = %v, want nil", err)
		}
		after := counterValue(t, reg, counterName)
		if got := after - before; got != 1 {
			t.Errorf("%s delta across ONE rejected response = %d (%d -> %d), want 1", counterName, got, before, after)
		}
	})

	// The LEGAL-PATH CONTROL. Without it a counter Inc'd on every response —
	// or hoisted above the validation branch — is invisible: the delta-1
	// assertion above would still read 1.
	t.Run("legal-response-does-not-inc", func(t *testing.T) {
		pki := mkH2BackendPKI(t)
		ln := startH2TrailerBackend(t, pki, h2TrailerNone, body, nil, nil /* the default, well-formed leading block */)
		defer func() { _ = ln.Close() }()

		c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
		a := &routerActionH2{cluster: c}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		before := counterValue(t, reg, counterName)
		if before < 0 {
			t.Fatalf("%s is not registered on an H2 cluster (counterValue = %d); reading 0 from an ABSENT counter is not a measurement", counterName, before)
		}
		resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
		if err != nil {
			t.Errorf("doH2ClusterAction: err = %v, want nil", err)
		}
		if resp.Status != 200 {
			t.Errorf("status = %d, want 200 (the control must actually take the SUCCESS path)", resp.Status)
		}
		after := counterValue(t, reg, counterName)
		if got := after - before; got != 0 {
			t.Errorf("%s delta across ONE well-formed response = %d (%d -> %d), want 0 (the counter must book a REJECTION, not a response)", counterName, got, before, after)
		}
	})
}
