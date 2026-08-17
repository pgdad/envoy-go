package hcm

import (
	"bufio"
	"bytes"
	"context"
	stdtls "crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"golang.org/x/net/http2/hpack"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/http/router"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/tracing"
)

// mkFilterForTable builds a minimal *Filter that wraps the given route table
// with throwaway HCM-scope counters allocated on a fresh Registry. Used by
// the H1-codec connection_test.go suite (which formerly drove runConnection
// with a bare *routeTable). Phase 06.1 Task 11: runConnection now takes
// *Filter so it can Inc the 5 HCM-scope counters per SPEC §5.5; legacy
// tests are forwarded through this wrapper so the existing assertions
// (status code, keep-alive shape) still drive the same code path.
//
// Phase 07.1 Task 15: also threads a minimal chainConfig containing only the
// terminal router filter, so dispatchRequest can allocate a *FilterChain at
// request time and run the chain-mediated H1 path. Without this, the
// runConnection→dispatchRequest call path would crash on the empty
// chainConfig (the chain validation invariant is "non-empty; last is router";
// tests in this file pre-Task-15 bypassed the chain entirely).
func mkFilterForTable(t *testing.T, tt *routeTable) *Filter {
	t.Helper()
	r := stats.NewRegistry()
	// Each test gets its own Registry so the per-test name uses a fixed
	// "ingress_http" stat_prefix without colliding across tests.
	prefix := "http.ingress_http."
	rf, err := router.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	return &Filter{
		table:             tt,
		statPrefix:        "ingress_http",
		downstreamRqTotal: r.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   r.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   r.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   r.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   r.NewCounter(prefix + "downstream_rq_5xx"),
		chainConfig:       []chainEntry{{name: "envoy.filters.http.router", factory: rf}},
	}
}

// singleEndpointCluster builds a *cluster.Cluster pointing at addr by going
// through cluster.NewManager with a minimal Bootstrap. Mirrors the helper
// in internal/filter/http/router/router_test.go. Used by the upstream-close
// regression test below; Task 15 reintroduced this helper here (it lived
// in this file pre-Task-11 when routerAction also lived in package hcm).
func singleEndpointCluster(t *testing.T, addr string) *cluster.Cluster {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort %q: %v", addr, err)
	}
	port64, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatalf("ParseUint %q: %v", portStr, err)
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 "c_test",
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: "c_test",
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       host,
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: uint32(port64)},
									},
								}},
							}},
						}},
					}},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	c, ok := cm.Get("c_test")
	if !ok {
		t.Fatal("cluster.Manager.Get(c_test) returned !ok")
	}
	return c
}

// connPair returns a connected pair of net.Conn, both ends in-process.
func connPair(t *testing.T) (clientSide, serverSide net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	type result struct {
		c   net.Conn
		err error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := ln.Accept()
		ch <- result{c, err}
	}()
	c1, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	return c1, r.c
}

func writeRequest(t *testing.T, w io.Writer, method, path string, headers ...string) {
	t.Helper()
	hdr := "Host: example\r\n"
	for _, h := range headers {
		hdr += h + "\r\n"
	}
	_, err := io.WriteString(w, method+" "+path+" HTTP/1.1\r\n"+hdr+"Content-Length: 0\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
}

func readResponseStatus(t *testing.T, r io.Reader) int {
	t.Helper()
	resp, err := http.ReadResponse(bufio.NewReader(r), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestRunConnection_DirectResponseHappy(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()

	go runConnection(context.Background(), server, mkFilterForTable(t, tt))

	writeRequest(t, client, "GET", "/health", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Errorf("status: got %d, want 200", got)
	}
}

func TestRunConnection_KeepAliveTwoRequests(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, mkFilterForTable(t, tt))

	writeRequest(t, client, "GET", "/health")
	if got := readResponseStatus(t, client); got != 200 {
		t.Fatalf("first status: got %d, want 200", got)
	}
	writeRequest(t, client, "GET", "/health", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Fatalf("second status: got %d, want 200", got)
	}
}

func TestRunConnection_RouteNotFoundReturns404(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, mkFilterForTable(t, tt))

	writeRequest(t, client, "GET", "/missing", "Connection: close")
	if got := readResponseStatus(t, client); got != 404 {
		t.Errorf("status: got %d, want 404", got)
	}
}

func TestRunConnection_ExpectHeaderReturns417(t *testing.T) {
	tt := &routeTable{}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, mkFilterForTable(t, tt))

	writeRequest(t, client, "GET", "/x", "Expect: 100-continue", "Connection: close")
	if got := readResponseStatus(t, client); got != 417 {
		t.Errorf("status: got %d, want 417", got)
	}
}

func TestRunConnection_UpgradeReturns501(t *testing.T) {
	tt := &routeTable{}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, mkFilterForTable(t, tt))

	writeRequest(t, client, "GET", "/x", "Upgrade: websocket", "Connection: Upgrade")
	if got := readResponseStatus(t, client); got != 501 {
		t.Errorf("status: got %d, want 501", got)
	}
}

func TestRunConnection_BadRequestReturns400(t *testing.T) {
	tt := &routeTable{}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, mkFilterForTable(t, tt))

	if _, err := io.WriteString(client, "GARBAGE\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if got := readResponseStatus(t, client); got != 400 {
		t.Errorf("status: got %d, want 400", got)
	}
}

// loopbackHTTPCloseEcho is a tiny HTTP/1.1 server that accepts upstream
// connections one at a time, reads one request, and writes a 200 response
// carrying `Connection: close`. The server returns its address and a stop
// function. Used to verify that routerAction.do propagates the upstream's
// Connection: close back to the connection loop via errCloseAfterAction
// (REVIEW.md I-1 from REVIEW.md 04527eb; SPEC §5.3).
func loopbackHTTPCloseEcho(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
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
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				body := "echo:" + req.URL.Path
				resp := fmt.Sprintf(
					"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
					len(body), body,
				)
				_, _ = c.Write([]byte(resp))
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close(); <-done }
}

// TestRunConnection_UpstreamConnectionCloseClosesDownstream is the I-1
// regression test: when the upstream's response carries `Connection: close`
// and the downstream request did NOT, runConnection must close the
// downstream after delivering that response (per SPEC §5.3 — "also break if
// the action's response carried Connection: close"). Pre-fix the connection
// loops back to read another request; post-fix it returns errCloseAfterAction
// from routerAction.do and the connection loop exits.
func TestRunConnection_UpstreamConnectionCloseClosesDownstream(t *testing.T) {
	addr, stop := loopbackHTTPCloseEcho(t)
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

	// Send TWO HTTP/1.1 requests back-to-back without Connection: close on
	// either. Pre-fix: both round-trips succeed (loop re-reads after each
	// upstream's Connection: close is silently ignored downstream). Post-fix:
	// only the first round-trip produces a response; the second request's
	// bytes are dropped on the floor when the loop closes.
	writeRequest(t, client, "GET", "/x")
	writeRequest(t, client, "GET", "/y")

	// Read the first response — must be 200.
	if got := readResponseStatus(t, client); got != 200 {
		t.Fatalf("first response status: got %d, want 200", got)
	}

	// Now bound the read for what comes next so we don't hang. We want to
	// see EOF (or a use-of-closed-connection style error) — NOT a second
	// "HTTP/1.1 200 OK" status line, which is what pre-fix code produced.
	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	n, err := client.Read(buf)
	if err == nil {
		// Got more bytes — that means the loop did NOT close after the
		// upstream's Connection: close. Read the rest and surface what we got.
		extra, _ := io.ReadAll(client)
		t.Errorf("expected EOF after first response (downstream should close per upstream Connection: close); got byte %q + %q",
			string(buf[:n]), string(extra))
	}

	// Confirm the connection loop returned (closed downstream).
	select {
	case <-loopDone:
		// good — runConnection returned
	case <-time.After(2 * time.Second):
		t.Error("runConnection did not return; expected close after upstream Connection: close")
	}
}

func TestRunConnection_BodyDrainedBetweenRequests(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/post"), action: &directResponseAction{status: 200, bodyText: "ok\n"}},
	}}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, mkFilterForTable(t, tt))

	body := strings.Repeat("x", 64)
	if _, err := io.WriteString(client,
		"POST /post HTTP/1.1\r\nHost: example\r\nContent-Length: "+strconv.Itoa(len(body))+"\r\n\r\n"+body); err != nil {
		t.Fatal(err)
	}
	if got := readResponseStatus(t, client); got != 200 {
		t.Fatalf("first status: got %d, want 200", got)
	}
	writeRequest(t, client, "GET", "/post", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Errorf("second status (post-drain): got %d, want 200", got)
	}
}

// ---------------------------------------------------------------------------
// Phase 07.1 Task 15 — chain-mediated dispatch assertions
// ---------------------------------------------------------------------------

// TestDispatchRequest_DirectResponseRunsChain exercises the chain-mediated
// H1 path directly: build a *Filter with chainConfig=[router], match a route
// to a directResponseAction, drive dispatchRequest, and assert the wire
// output matches the legacy direct-call shape AND the chain ran (the router
// filter's terminal action fired and surfaced the configured 200 status).
// Per PLAN Task 15 acceptance: byte-equivalent wire output + chain
// machinery runs.
func TestDispatchRequest_DirectResponseRunsChain(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f := mkFilterForTable(t, tt)

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	status, err := f.dispatchRequest(context.Background(), nil, req, bw)
	if err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	_ = bw.Flush()
	if !strings.HasPrefix(buf.String(), "HTTP/1.1 200 OK\r\n") {
		t.Errorf("expected 200 OK status line, got: %q", buf.String())
	}
	if !strings.HasSuffix(buf.String(), "OK\n") {
		t.Errorf("expected body 'OK\\n' suffix, got: %q", buf.String())
	}
}

// TestDispatchRequest_ChainMediatedAccessLogEmit verifies that the access-log
// emit fires from chain-completion (Decision §3.1) — a single uniform site
// that replaces the four pre-Task-15 emit-deferral sites in actions.go.
// Builds a *Filter with a capture sink + chainConfig=[router], drives one
// direct_response request, and asserts ResponseCode + BytesSent on the
// captured record.
func TestDispatchRequest_ChainMediatedAccessLogEmit(t *testing.T) {
	cs := &emitCaptureSink{}
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f := mkFilterForTable(t, tt)
	f.accessLog = []accesslog.Sink{cs}

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := f.dispatchRequest(context.Background(), nil, req, bw); err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}
	_ = bw.Flush()

	if len(cs.recs) != 1 {
		t.Fatalf("captured %d records, want 1 (chain-completion emit)", len(cs.recs))
	}
	r := cs.recs[0]
	if r.ResponseCode != 200 {
		t.Errorf("ResponseCode = %d, want 200", r.ResponseCode)
	}
	if r.BytesSent != 3 {
		t.Errorf("BytesSent = %d, want 3 (len(\"OK\\n\"))", r.BytesSent)
	}
	if r.UpstreamHost != "" {
		t.Errorf("UpstreamHost = %q, want empty (direct_response)", r.UpstreamHost)
	}
}

// ---------------------------------------------------------------------------
// Phase 22.2 Task 6 (ADR-0192) — H1 dispatchRequest tlsConnectionState seeding
// ---------------------------------------------------------------------------

// tlsStateCapturingFilter is a decode-only HTTPFilter test-double that records
// the *tls.ConnectionState observed via dcb.DownstreamTLSConnectionState() at
// DecodeHeaders time. The capture happens at DecodeHeaders (not at
// SetDecoderCallbacks) because the seeded value lives on the chain BEFORE
// RunDecodeHeaders dispatch but AFTER NewFilterChain wires callbacks — i.e.,
// the dispatch ordering is `NewFilterChain (wires callbacks)` →
// `chain.SetTLSConnectionState(state)` → `RunDecodeHeaders → DecodeHeaders`.
// Reading at SetDecoderCallbacks time would observe a stale nil; reading at
// DecodeHeaders observes the freshly-seeded value per SPEC §11.5.3 set-once-
// BEFORE-RunDecodeHeaders discipline. Used by both H1 (connection_test) + H2
// (h2dispatch_test) seeding-assertion tests.
//
// Encode-side methods are no-op pass-throughs; the chain framework requires
// both sides on filters that participate in the both-sides envelope.
type tlsStateCapturingFilter struct {
	// captured is the *tls.ConnectionState observed at DecodeHeaders entry
	// time via dcb.DownstreamTLSConnectionState(). nil when the HCM dispatch
	// path elided the SetTLSConnectionState call (plaintext / pre-handshake);
	// non-nil + bound to the HCM-seeded pointer otherwise.
	captured *stdtls.ConnectionState
	cbs      filter_http.DecoderFilterCallbacks
}

func (f *tlsStateCapturingFilter) DecodeHeaders(http.Header, bool) filter_http.FilterHeadersStatus {
	if f.cbs != nil {
		f.captured = f.cbs.DownstreamTLSConnectionState()
	}
	return filter_http.Continue
}
func (f *tlsStateCapturingFilter) DecodeData([]byte, bool) filter_http.FilterDataStatus {
	return filter_http.DataContinue
}
func (f *tlsStateCapturingFilter) DecodeTrailers(http.Header) filter_http.FilterTrailersStatus {
	return filter_http.TrailersContinue
}
func (f *tlsStateCapturingFilter) SetDecoderCallbacks(cbs filter_http.DecoderFilterCallbacks) {
	f.cbs = cbs
}
func (f *tlsStateCapturingFilter) OnDestroy() {}

func (f *tlsStateCapturingFilter) EncodeHeaders(http.Header, bool) filter_http.FilterHeadersStatus {
	return filter_http.Continue
}
func (f *tlsStateCapturingFilter) EncodeData([]byte, bool) filter_http.FilterDataStatus {
	return filter_http.DataContinue
}
func (f *tlsStateCapturingFilter) EncodeTrailers(http.Header) filter_http.FilterTrailersStatus {
	return filter_http.TrailersContinue
}
func (f *tlsStateCapturingFilter) SetEncoderCallbacks(filter_http.EncoderFilterCallbacks) {}

// mkTLSStateCapturingFilterForTable wires a *Filter with chainConfig=[capture,
// router] where capture is the tlsStateCapturingFilter wired ahead of the
// terminal router. The captured *tlsStateCapturingFilter pointer is returned
// so the test can read back the observed state(s) after dispatchRequest.
//
// Per-request factory returns a SHARED capture instance (not a fresh per-request
// instance) so tests observe the captured value verbatim after the single
// dispatchRequest call. Production code uses fresh per-request instances per
// ADR-0071's two-step factory; this test fixture takes the simpler shared-instance
// route since each test drives exactly one request.
func mkTLSStateCapturingFilterForTable(t *testing.T, tt *routeTable) (*Filter, *tlsStateCapturingFilter) {
	t.Helper()
	capture := &tlsStateCapturingFilter{}
	captureFactory := func() filter_http.HTTPFilter {
		return filter_http.HTTPFilter{
			Name:    "tls-state-capture",
			Decoder: capture,
			Encoder: capture,
		}
	}
	rfFactory, err := router.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	r := stats.NewRegistry()
	prefix := "http.ingress_http."
	f := &Filter{
		table:             tt,
		statPrefix:        "ingress_http",
		downstreamRqTotal: r.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   r.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   r.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   r.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   r.NewCounter(prefix + "downstream_rq_5xx"),
		chainConfig: []chainEntry{
			{name: "tls-state-capture", factory: captureFactory},
			{name: "envoy.filters.http.router", factory: rfFactory},
		},
	}
	return f, capture
}

// TestConnection_dispatchRequest_seeds_nil_for_plaintext exercises the
// plaintext path: a nil downstream conn (mirroring test-paths that drive
// dispatchRequest without a real listener). The chain's tlsConnectionState
// stays nil — downstreamTLSConnectionState(nil) returns nil, and the seeding
// call SetTLSConnectionState(nil) is the documented nil-passthrough.
func TestConnection_dispatchRequest_seeds_nil_for_plaintext(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f, capture := mkTLSStateCapturingFilterForTable(t, tt)

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := f.dispatchRequest(context.Background(), nil, req, bw); err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}

	if capture.captured != nil {
		t.Errorf("plaintext: captured = %#v; want nil", capture.captured)
	}
}

// TestConnection_dispatchRequest_seeds_nil_for_non_TLS_handshake_complete
// exercises the *tls.Conn-but-pre-handshake path: a tls.Server wrapper over a
// net.Pipe pair where Handshake() has NOT been called. downstreamTLSConnectionState
// returns nil per the HandshakeComplete guard so the chain's tlsConnectionState
// stays nil.
//
// Production analog: a *tls.Conn that errored during the handshake before the
// dispatch driver reached the chain. The driver never reaches dispatchRequest
// on such a conn in production (the codec drops it earlier), so this is a
// defensive shape proof — the helper does not panic / misreport on a partially-
// initialized conn-state.
func TestConnection_dispatchRequest_seeds_nil_for_non_TLS_handshake_complete(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f, capture := mkTLSStateCapturingFilterForTable(t, tt)

	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close() }()
	defer func() { _ = c2.Close() }()
	tlsConn := stdtls.Server(c1, &stdtls.Config{})
	// No Handshake().

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := f.dispatchRequest(context.Background(), tlsConn, req, bw); err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}

	if capture.captured != nil {
		t.Errorf("pre-handshake *tls.Conn: captured = %#v; want nil (HandshakeComplete=false guard)", capture.captured)
	}
}

// TestConnection_dispatchRequest_seeds_tlsConnectionState_for_TLS_handshake_complete
// exercises the TLS-handshake-complete happy path: an in-process *tls.Conn
// pair where both sides drove Handshake() to completion. The chain's
// tlsConnectionState is non-nil + carries the SNI from the client side.
//
// Per SPEC §11.5.3 the state is seeded even for server-auth-only handshakes
// (no client cert) — distinct from tlsPrincipals (ADR-0144) which fires only
// on mTLS handshakes with len(PeerCertificates)>0.
func TestConnection_dispatchRequest_seeds_tlsConnectionState_for_TLS_handshake_complete(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f, capture := mkTLSStateCapturingFilterForTable(t, tt)

	serverTLS, cleanup := runInProcessTLSHandshake(t, "h1-server-test", "h1.sni.envoy-go.test")
	defer cleanup()

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := f.dispatchRequest(context.Background(), serverTLS, req, bw); err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}

	if capture.captured == nil {
		t.Fatal("post-handshake *tls.Conn: captured = nil; want non-nil *tls.ConnectionState seeded BEFORE RunDecodeHeaders")
	}
	if !capture.captured.HandshakeComplete {
		t.Errorf("captured.HandshakeComplete = false; want true")
	}
	if capture.captured.ServerName != "h1.sni.envoy-go.test" {
		t.Errorf("captured.ServerName = %q; want %q", capture.captured.ServerName, "h1.sni.envoy-go.test")
	}
}

// ---------------------------------------------------------------------------
// Phase 46.1a Task 9 — HCM dispatch tracing wiring (Decide + x-request-id
// stamp + traceparent inject + HCM counter) on the H1 (connection.go) and H2
// (h2dispatch.go) paths. The decision mutates the UPSTREAM-forwarded request
// headers in place; the no-tracing path (tracingConfig==nil) is byte-stable.
// ---------------------------------------------------------------------------

// fakeTraceRand is a deterministic tracing.RandSource for header assertions.
// Read fills with a fixed byte; Float64 returns a fixed value. With f==0 the
// random-sampling roll (Float64*100 < RandomSampling) always samples and the
// overall_sampling cap (Float64*100 >= OverallSampling) never trips at 100%.
type fakeTraceRand struct {
	f float64
	b byte
}

func (r fakeTraceRand) Float64() float64 { return r.f }
func (r fakeTraceRand) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b
	}
	return len(p), nil
}

// traceparentRE matches a W3C version-00 traceparent: 00-<32hex>-<16hex>-<flags>.
var traceparentRE = regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-(00|01)$`)

func traceparentMatches(tp string, sampled bool) bool {
	if !traceparentRE.MatchString(tp) {
		return false
	}
	want := "-00"
	if sampled {
		want = "-01"
	}
	return strings.HasSuffix(tp, want)
}

// mkTracingFilter wraps mkFilterForTable and attaches a full-sampling tracing
// config + freshly-registered HCM counters + the supplied deterministic rng.
// Returns the filter and the registry so tests can read counter values.
func mkTracingFilter(t *testing.T, tt *routeTable, rng tracing.RandSource) (*Filter, *stats.Registry) {
	t.Helper()
	f := mkFilterForTable(t, tt)
	reg := stats.NewRegistry()
	counters, err := tracing.RegisterHCMCounters(reg, "ingress_http")
	if err != nil {
		t.Fatalf("RegisterHCMCounters: %v", err)
	}
	f.tracingConfig = &tracing.TracingConfig{ClientSampling: 100, RandomSampling: 100, OverallSampling: 100}
	f.tracingCounters = counters
	f.rng = rng
	return f, reg
}

// mkTracingFilterChain is mkTracingFilter with a caller-supplied chainConfig.
// mkTracingFilter (via mkFilterForTable) installs a ROUTER-ONLY chain, so none
// of the pre-phase-89 TestWriteH2_Tracing* rows runs a decode filter at all.
// Phase 89 (ADR-0311) needs the tracing seam exercised alongside a mutating
// decode filter, to prove the H/2 decode-delta reconciler does not clobber the
// tracing writes the seam makes on the SAME carrier. Additive: existing callers
// of mkTracingFilter are untouched.
func mkTracingFilterChain(t *testing.T, tt *routeTable, rng tracing.RandSource, chainConfig []chainEntry) (*Filter, *stats.Registry) {
	t.Helper()
	f, reg := mkTracingFilter(t, tt, rng)
	f.chainConfig = chainConfig
	return f, reg
}

func tracingCounterValue(t *testing.T, reg *stats.Registry, name string) uint64 {
	t.Helper()
	var v uint64
	found := false
	reg.Walk(func(m stats.Metric) {
		if m.Name() != "http.ingress_http.tracing."+name {
			return
		}
		c, ok := m.(*stats.Counter)
		if !ok {
			t.Fatalf("metric %q is not a *stats.Counter", name)
		}
		v = c.Load()
		found = true
	})
	if !found {
		t.Fatalf("tracing counter %q not registered", name)
	}
	return v
}

// h2HeaderValue returns the first case-insensitive match of name among the
// H2Request's regular headers (the upstream-forwarded set), or "".
func h2HeaderValue(req h2.H2Request, name string) string {
	for _, f := range req.Headers {
		if strings.EqualFold(f.Name, name) {
			return f.Value
		}
	}
	return ""
}

// --- H1 (connection.go dispatchRequest) -------------------------------------

func TestDispatchRequest_Tracing_SampledInjects(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f, reg := mkTracingFilter(t, tt, fakeTraceRand{f: 0, b: 0xab})

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := f.dispatchRequest(context.Background(), nil, req, bw); err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}

	rid := req.Header.Get("X-Request-Id")
	if len(rid) != 36 {
		t.Fatalf("X-Request-Id = %q, want a 36-char id", rid)
	}
	if rid[14] != '9' {
		t.Errorf("X-Request-Id index-14 = %q, want '9' (Sampled)", rid[14])
	}
	if tp := req.Header.Get("Traceparent"); !traceparentMatches(tp, true) {
		t.Errorf("Traceparent = %q, want 00-<32hex>-<16hex>-01", tp)
	}
	if got := tracingCounterValue(t, reg, "random_sampling"); got != 1 {
		t.Errorf("random_sampling = %d, want 1", got)
	}
}

func TestDispatchRequest_Tracing_Continued(t *testing.T) {
	const fixedTrace = "0af7651916cd43dd8448eb211c80319c"
	const fixedSpan = "b7ad6b7169203331"
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f, reg := mkTracingFilter(t, tt, fakeTraceRand{f: 0, b: 0x11})

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	req.Header.Set("Traceparent", "00-"+fixedTrace+"-"+fixedSpan+"-01")
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := f.dispatchRequest(context.Background(), nil, req, bw); err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}

	tp := req.Header.Get("Traceparent")
	if !strings.HasPrefix(tp, "00-"+fixedTrace+"-") {
		t.Errorf("forwarded Traceparent = %q, want continued trace-id %s", tp, fixedTrace)
	}
	if !traceparentMatches(tp, true) {
		t.Errorf("Traceparent = %q, want well-formed sampled", tp)
	}
	rid := req.Header.Get("X-Request-Id")
	if len(rid) != 36 || rid[14] != '9' {
		t.Errorf("X-Request-Id = %q, want 36-char with index-14 '9' (continued+sampled reflects the inbound sampled bit)", rid)
	}
	if got := tracingCounterValue(t, reg, "not_traceable"); got != 1 {
		t.Errorf("not_traceable = %d, want 1", got)
	}
}

func TestDispatchRequest_Tracing_PreserveRequestID(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f, _ := mkTracingFilter(t, tt, fakeTraceRand{f: 0, b: 0x22})

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	req.Header.Set("X-Request-Id", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := f.dispatchRequest(context.Background(), nil, req, bw); err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}

	if got := req.Header.Get("X-Request-Id"); got != "aaaaaaaa-bbbb-9ccc-dddd-eeeeeeeeeeee" {
		t.Errorf("X-Request-Id = %q, want aaaaaaaa-bbbb-9ccc-dddd-eeeeeeeeeeee (preserved + stamped)", got)
	}
}

func TestDispatchRequest_NoTracing_ByteStable(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f := mkFilterForTable(t, tt) // tracingConfig == nil

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := f.dispatchRequest(context.Background(), nil, req, bw); err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}

	if got := req.Header.Get("X-Request-Id"); got != "" {
		t.Errorf("X-Request-Id = %q, want empty (no-tracing byte-stable)", got)
	}
	if got := req.Header.Get("Traceparent"); got != "" {
		t.Errorf("Traceparent = %q, want empty (no-tracing byte-stable)", got)
	}
}

// --- H2 (h2dispatch.go WriteH2) ---------------------------------------------

// captureH2Action captures the (post-injection) upstream-forwarded H2Request
// the terminal router filter dispatches to, then returns a minimal 200 so the
// rest of WriteH2 completes cleanly.
func captureH2Action(captured *h2.H2Request) router.H2Action {
	return func(_ context.Context, req h2.H2Request) (router.ActionResponse, cluster.Endpoint, error) {
		*captured = req
		return router.ActionResponse{Status: 200}, cluster.Endpoint{}, nil
	}
}

func TestWriteH2_Tracing_SampledInjects(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f, reg := mkTracingFilter(t, tt, fakeTraceRand{f: 0, b: 0x33})

	var captured h2.H2Request
	hreq, _ := http.NewRequest("GET", "/health", nil)
	hreq.Proto = "HTTP/2.0"
	c := &chainDispatchAction{f: f, action: captureH2Action(&captured), req: hreq, routeIdx: 0}

	h2req := h2.H2Request{Method: "GET", Path: "/health", Authority: "localhost"}
	if err := c.WriteH2(context.Background(), h2req, &captureH2Writer{}); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	rid := h2HeaderValue(captured, "x-request-id")
	if len(rid) != 36 || rid[14] != '9' {
		t.Errorf("forwarded x-request-id = %q, want 36-char with index-14 '9'", rid)
	}
	if tp := h2HeaderValue(captured, "traceparent"); !traceparentMatches(tp, true) {
		t.Errorf("forwarded traceparent = %q, want 00-<32hex>-<16hex>-01", tp)
	}
	if got := tracingCounterValue(t, reg, "random_sampling"); got != 1 {
		t.Errorf("random_sampling = %d, want 1", got)
	}
}

func TestWriteH2_Tracing_Continued(t *testing.T) {
	const fixedTrace = "0af7651916cd43dd8448eb211c80319c"
	const fixedSpan = "b7ad6b7169203331"
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f, reg := mkTracingFilter(t, tt, fakeTraceRand{f: 0, b: 0x44})

	var captured h2.H2Request
	hreq, _ := http.NewRequest("GET", "/health", nil)
	hreq.Proto = "HTTP/2.0"
	c := &chainDispatchAction{f: f, action: captureH2Action(&captured), req: hreq, routeIdx: 0}

	h2req := h2.H2Request{
		Method:    "GET",
		Path:      "/health",
		Authority: "localhost",
		Headers:   []hpack.HeaderField{{Name: "traceparent", Value: "00-" + fixedTrace + "-" + fixedSpan + "-01"}},
	}
	if err := c.WriteH2(context.Background(), h2req, &captureH2Writer{}); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	tp := h2HeaderValue(captured, "traceparent")
	if !strings.HasPrefix(tp, "00-"+fixedTrace+"-") {
		t.Errorf("forwarded traceparent = %q, want continued trace-id %s", tp, fixedTrace)
	}
	rid := h2HeaderValue(captured, "x-request-id")
	if len(rid) != 36 || rid[14] != '9' {
		t.Errorf("forwarded x-request-id = %q, want 36-char with index-14 '9' (continued+sampled reflects the inbound sampled bit)", rid)
	}
	if got := tracingCounterValue(t, reg, "not_traceable"); got != 1 {
		t.Errorf("not_traceable = %d, want 1", got)
	}
}

// TestWriteH2_Tracing_DecodeFilterMutationSurvives — phase 89 (ADR-0311),
// row 12 of the reconciler roster (the tracing survival row).
//
// The four pre-phase-89 TestWriteH2_Tracing* rows all run a ROUTER-ONLY chain,
// so no decode filter runs in any of them. This row installs a TWO-ENTRY chain
// [headerMutatingFilter, router] over the SAME OTel tracing setup and asserts
// BOTH directions of the interaction on the shared carrier:
//
//   - the tracing seam's writes (x-request-id, traceparent), made at
//     h2dispatch.go :438-455 BEFORE RunDecodeHeaders, SURVIVE the reconcile;
//   - the decode filter's own mutation LANDS.
//
// A whole-map projection (rebuild the carrier from c.req.Header) would drop the
// tracing writes, because they are made on the CARRIER and never mirrored into
// the decode map.
func TestWriteH2_Tracing_DecodeFilterMutationSurvives(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	chainCfg := mutatingChain(t, func() *headerMutatingFilter {
		return &headerMutatingFilter{onHeaders: func(h http.Header) { h.Set("X-Filter-Added", "yes") }}
	})
	f, reg := mkTracingFilterChain(t, tt, fakeTraceRand{f: 0, b: 0x55}, chainCfg)

	var captured h2.H2Request
	hreq, _ := http.NewRequest("GET", "/health", nil)
	hreq.Proto = "HTTP/2.0"
	hreq.Header.Add("x-seed", "s")
	c := &chainDispatchAction{f: f, action: captureH2Action(&captured), req: hreq, routeIdx: 0}

	h2req := h2.H2Request{
		Method:    "GET",
		Path:      "/health",
		Scheme:    "https",
		Authority: "localhost",
		Headers:   []hpack.HeaderField{{Name: "x-seed", Value: "s"}},
	}
	if err := c.WriteH2(context.Background(), h2req, &captureH2Writer{}); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	rid := h2HeaderValues(captured, "x-request-id")
	if len(rid) != 1 || len(rid[0]) != 36 {
		t.Errorf("forwarded x-request-id = %v, want exactly one 36-char id (tracing write survives the reconcile)", rid)
	}
	tp := h2HeaderValues(captured, "traceparent")
	if len(tp) != 1 || !traceparentMatches(tp[0], true) {
		t.Errorf("forwarded traceparent = %v, want exactly one 00-<32hex>-<16hex>-01", tp)
	}
	if got := h2HeaderValues(captured, "x-filter-added"); len(got) != 1 || got[0] != "yes" {
		t.Errorf("forwarded x-filter-added = %v, want [yes] (the decode filter's mutation must land)", got)
	}
	if got := h2HeaderValues(captured, "x-seed"); len(got) != 1 || got[0] != "s" {
		t.Errorf("forwarded x-seed = %v, want [s] (untouched header unchanged)", got)
	}
	if got := tracingCounterValue(t, reg, "random_sampling"); got != 1 {
		t.Errorf("random_sampling = %d, want 1", got)
	}
}

func TestWriteH2_NoTracing_ByteStable(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f := newH2DispatchFilter(t, tt, routerOnlyChain(t), nil) // tracingConfig == nil

	var captured h2.H2Request
	hreq, _ := http.NewRequest("GET", "/health", nil)
	hreq.Proto = "HTTP/2.0"
	c := &chainDispatchAction{f: f, action: captureH2Action(&captured), req: hreq, routeIdx: 0}

	h2req := h2.H2Request{Method: "GET", Path: "/health", Authority: "localhost"}
	if err := c.WriteH2(context.Background(), h2req, &captureH2Writer{}); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	if got := h2HeaderValue(captured, "x-request-id"); got != "" {
		t.Errorf("forwarded x-request-id = %q, want empty (no-tracing byte-stable)", got)
	}
	if got := h2HeaderValue(captured, "traceparent"); got != "" {
		t.Errorf("forwarded traceparent = %q, want empty (no-tracing byte-stable)", got)
	}
}
