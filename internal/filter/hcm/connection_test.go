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
	"strconv"
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/stats"
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
