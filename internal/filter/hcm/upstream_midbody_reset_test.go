package hcm

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// Upstream mid-RESPONSE-BODY termination coverage (ranked item 2 of the
// 2026-07-11 test-gap analysis §4). The router's H1 driver reads the response
// in two phases: http.ReadResponse (headers) then io.ReadAll (body,
// router.go). An upstream that dies AFTER headers but MID-BODY hits a distinct
// code path — `return ActionResponse{Status: resp.StatusCode}, picked, err` —
// that no test exercised: a partial status with a non-nil action error.
//
// Downstream-visible behavior CHARACTERIZED here (envoy-go current):
// because envoy-go's H1 proxy is FULLY BUFFERED, nothing has been written
// downstream when the body read fails; HCM's wire-write is gated on
// actionErr == nil, so the downstream connection is CLOSED WITH ZERO RESPONSE
// BYTES and the connection loop exits.
//
// Reference Envoy (streaming proxy) instead FORWARDS the 200 header block and
// then TRUNCATES the body / resets the stream — by the time the upstream
// dies, the headers are already on the wire. Both implementations terminate
// the exchange without delivering a complete response (no smuggling /
// desync exposure: neither can be mistaken for success by a well-formed
// client), but the wire shape differs; the divergence is recorded in
// docs/TEST_GAP_ANALYSIS.md and logged by the test. If envoy-go ever moves to
// streaming proxying, these tests fail loudly — the signal to re-pin against
// the reference (a truncated 200, not a clean 0-byte close).

// loopbackHTTPMidBodyDeath returns the addr of a backend that responds to any
// request with "HTTP/1.1 200 OK" + Content-Length: 1000, writes only 100 body
// bytes, then terminates the connection: FIN (plain Close) when rst is false,
// RST (SO_LINGER 0) when rst is true.
func loopbackHTTPMidBodyDeath(t *testing.T, rst bool) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)
				if _, err := http.ReadRequest(br); err != nil {
					return
				}
				partial := make([]byte, 100)
				for i := range partial {
					partial[i] = 'x'
				}
				_, _ = fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 1000\r\n\r\n")
				_, _ = c.Write(partial)
				if rst {
					if tc, ok := c.(*net.TCPConn); ok {
						_ = tc.SetLinger(0) // Close sends RST, not FIN
					}
				}
				// defer'd Close terminates mid-body (900 bytes short).
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close(); <-done }
}

// driveMidBodyDeath runs one GET through runConnection against the mid-body
// death backend and asserts the characterized downstream outcome: connection
// closed with ZERO response bytes, and the connection loop exits.
func driveMidBodyDeath(t *testing.T, rst bool) {
	t.Helper()
	addr, stop := loopbackHTTPMidBodyDeath(t, rst)
	defer stop()

	c := singleEndpointCluster(t, addr)
	tt := &routeTable{routes: []routeEntry{
		{match: matchPrefix("/"), action: &clusterRouteAction{cluster: c}},
	}}

	client, server := connPair(t)
	defer func() { _ = client.Close() }()

	loopDone := make(chan struct{})
	go func() {
		runConnection(context.Background(), server, mkFilterForTable(t, tt))
		close(loopDone)
	}()

	writeRequest(t, client, "GET", "/mid-body")

	if err := client.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(client)
	if err != nil {
		// A read error (rather than clean EOF) is acceptable only if no bytes
		// arrived; ReadAll returns what it got either way.
		t.Logf("downstream read ended with error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("downstream received %d bytes (%q...) after an upstream mid-body death; envoy-go's buffered proxy is expected to close with ZERO response bytes.\n"+
			"If envoy-go now streams responses, re-pin this against reference Envoy (which forwards the 200 headers and truncates the body).",
			len(got), string(got[:min(len(got), 64)]))
	}

	select {
	case <-loopDone:
		// good — the connection loop exited (no reuse of a poisoned exchange)
	case <-time.After(3 * time.Second):
		t.Error("runConnection did not return after the upstream died mid-body; the downstream connection must be closed")
	}

	t.Logf("characterized: upstream died mid-body (rst=%v) → downstream closed with 0 response bytes "+
		"(reference Envoy v1.37.2 would forward the 200 header block and truncate — divergence in wire shape, recorded in docs/TEST_GAP_ANALYSIS.md)", rst)
}

// TestRunConnection_UpstreamDiesMidResponseBody_FIN: upstream half-closes
// (FIN) 900 bytes short of its declared Content-Length.
func TestRunConnection_UpstreamDiesMidResponseBody_FIN(t *testing.T) {
	driveMidBodyDeath(t, false)
}

// TestRunConnection_UpstreamDiesMidResponseBody_RST: upstream resets
// (SO_LINGER 0 → RST) mid-body — the "connection reset by peer" flavor.
func TestRunConnection_UpstreamDiesMidResponseBody_RST(t *testing.T) {
	driveMidBodyDeath(t, true)
}
