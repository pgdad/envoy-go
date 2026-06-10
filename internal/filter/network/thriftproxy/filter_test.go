package thriftproxy

import (
	"bufio"
	"context"
	"io"
	"net"
	"testing"
	"time"

	thrift_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

// ---- NewFactory boot arms (the redisproxy precedent) ----

func validThriftAny(t *testing.T) *anypb.Any {
	t.Helper()
	msg := &thrift_proxyv3.ThriftProxy{
		StatPrefix:         "tp",
		PayloadPassthrough: true,
		RouteConfig: &thrift_proxyv3.RouteConfiguration{
			Routes: []*thrift_proxyv3.Route{{
				Match: &thrift_proxyv3.RouteMatch{MatchSpecifier: &thrift_proxyv3.RouteMatch_MethodName{MethodName: "ping"}},
				Route: &thrift_proxyv3.RouteAction{ClusterSpecifier: &thrift_proxyv3.RouteAction_Cluster{Cluster: "tc"}},
			}},
		},
	}
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func TestNewFactory_TypeURLReject(t *testing.T) {
	reg := stats.NewRegistry()
	f := NewFactory(nil, reg)
	bad := &anypb.Any{TypeUrl: "type.googleapis.com/wrong.Type"}
	if _, err := f(bad, network.FactoryCtx{}); err == nil {
		t.Fatal("NewFactory accepted a wrong type_url; want a reject")
	}
}

func TestNewFactory_ValidConfig(t *testing.T) {
	reg := stats.NewRegistry()
	f := NewFactory(nil, reg)
	fif, err := f(validThriftAny(t), network.FactoryCtx{})
	if err != nil {
		t.Fatalf("NewFactory returned error for valid config: %v", err)
	}
	if fif == nil {
		t.Fatal("NewFactory returned nil FilterInstanceFactory for valid config")
	}
	if fif() == nil {
		t.Fatal("FilterInstanceFactory returned a nil NetworkFilter")
	}
}

// newTestFilter builds a *filter directly with an injected upstream resolver (the
// cluster.Manager path is exercised in the differential; the unit test injects a
// fake upstream so the pump logic is tested in isolation). The production path
// resolves cm.Get → PickEndpoint → Cluster.Dial (SPEC §3.3).
func newTestFilter(ts *thriftStats, routes *routeTable, passthrough bool, resolve resolveFunc) *filter {
	return &filter{
		cfg:     &compiledConfig{statPrefix: "tp", payloadPassthrough: passthrough, routes: routes},
		st:      ts,
		resolve: resolve,
	}
}

func oneRoute(method, cluster string) *routeTable {
	return &routeTable{entries: []routeEntry{{methodName: method, cluster: cluster}}}
}

// dialResolver wires a resolveFunc whose dial closure returns the given conn.
func dialResolver(dialed *bool, conn net.Conn, dialErr error) resolveFunc {
	return func(_ context.Context, _ string) (network.UpstreamDialFunc, func(), resolveStatus) {
		return func(context.Context) (net.Conn, error) {
			if dialed != nil {
				*dialed = true
			}
			return conn, dialErr
		}, nil, resolveOK
	}
}

func TestHandle_RouteHit(t *testing.T) {
	reg := stats.NewRegistry()
	ts := newThriftStats(reg, "tp")
	// scripted upstream: read the CALL frame, write a void-success REPLY back.
	upSrv, upClient := net.Pipe()
	go func() {
		br := bufio.NewReader(upSrv)
		_, _ = decodeFrame(br)
		_, _ = upSrv.Write(framedBinaryReply("ping", 7, []byte{0x00})) // void success
	}()
	dialed := false
	f := newTestFilter(ts, oneRoute("ping", "tc"), true, dialResolver(&dialed, upClient, nil))

	down, client := net.Pipe()
	wantReply := framedBinaryReply("ping", 7, []byte{0x00})
	got := make(chan []byte, 1)
	go func() {
		_, _ = client.Write(framedBinaryCall(msgTypeCall, "ping", 7))
		buf := make([]byte, len(wantReply))
		_, _ = io.ReadFull(client, buf)
		_ = client.Close()
		got <- buf
	}()
	f.Handle(context.Background(), down)

	if g := <-got; string(g) != string(wantReply) {
		t.Errorf("downstream reply = %x, want %x", g, wantReply)
	}
	if !dialed {
		t.Error("route hit did not dial upstream")
	}
	for _, suf := range []string{"request", "request_call", "request_passthrough", "response", "response_reply", "response_success", "response_passthrough"} {
		if got := ts.counters[suf].Load(); got != 1 {
			t.Errorf("counter %q = %d, want 1", suf, got)
		}
	}
	if got := ts.counters["request_oneway"].Load(); got != 0 {
		t.Errorf("request_oneway = %d, want 0 (CALL not ONEWAY)", got)
	}
	if got := ts.active.Load(); got != 0 {
		t.Errorf("request_active = %d after Handle, want 0 (defer-balanced)", got)
	}
}

func TestHandle_RouteMiss(t *testing.T) {
	reg := stats.NewRegistry()
	ts := newThriftStats(reg, "tp")
	dialed := false
	f := newTestFilter(ts, oneRoute("ping", "tc"), false, dialResolver(&dialed, nil, io.EOF))

	down, client := net.Pipe()
	wantExc := encodeUnknownMethod("other", 3)
	got := make(chan []byte, 1)
	go func() {
		_, _ = client.Write(framedBinaryCall(msgTypeCall, "other", 3))
		buf := make([]byte, len(wantExc))
		_, _ = io.ReadFull(client, buf)
		_ = client.Close() // conn stays open after the exception; EOF ends the pump
		got <- buf
	}()
	f.Handle(context.Background(), down)

	if g := <-got; string(g) != string(wantExc) {
		t.Errorf("downstream miss reply = %x, want UnknownMethod exception %x", g, wantExc)
	}
	if dialed {
		t.Error("route miss dialed upstream; want zero upstream (local reply)")
	}
	if ts.counters["route_missing"].Load() != 1 || ts.counters["response_exception"].Load() != 1 {
		t.Errorf("route_missing/response_exception = %d/%d, want 1/1",
			ts.counters["route_missing"].Load(), ts.counters["response_exception"].Load())
	}
	if ts.counters["request"].Load() != 1 || ts.counters["request_call"].Load() != 1 {
		t.Errorf("request/request_call = %d/%d, want 1/1 (the miss still counts the request)",
			ts.counters["request"].Load(), ts.counters["request_call"].Load())
	}
	// AMEND-T6: these stay 0 (framework-zero-touch miss — conn kept open).
	for _, suf := range []string{"cx_destroy_local_with_active_rq", "cx_destroy_remote_with_active_rq", "downstream_response_drain_close"} {
		if got := ts.counters[suf].Load(); got != 0 {
			t.Errorf("counter %q = %d, want 0 (AMEND-T6 exist-at-0)", suf, got)
		}
	}
	if got := ts.active.Load(); got != 0 {
		t.Errorf("request_active = %d after Handle, want 0 (defer-balanced)", got)
	}
}

func TestHandle_DecodingError(t *testing.T) {
	reg := stats.NewRegistry()
	ts := newThriftStats(reg, "tp")
	f := newTestFilter(ts, oneRoute("ping", "tc"), false, dialResolver(nil, nil, io.EOF))

	down, client := net.Pipe()
	go func() {
		// valid length prefix, bad magic in payload → errDecode (not EOF, not invalid-type).
		bad := framedBinaryCall(msgTypeCall, "ping", 1)
		bad[4] = 0x00
		bad[5] = 0x00
		_, _ = client.Write(bad)
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)

	if got := ts.counters["request_decoding_error"].Load(); got != 1 {
		t.Errorf("request_decoding_error = %d, want 1", got)
	}
	if got := ts.counters["request_invalid_type"].Load(); got != 0 {
		t.Errorf("request_invalid_type = %d, want 0", got)
	}
	if got := ts.active.Load(); got != 0 {
		t.Errorf("request_active = %d, want 0", got)
	}
}

func TestHandle_InvalidType(t *testing.T) {
	reg := stats.NewRegistry()
	ts := newThriftStats(reg, "tp")
	f := newTestFilter(ts, oneRoute("ping", "tc"), false, dialResolver(nil, nil, io.EOF))

	down, client := net.Pipe()
	go func() {
		_, _ = client.Write(framedBinaryCall(0x09, "ping", 1)) // msgtype 9 → errInvalidType
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)

	if got := ts.counters["request_invalid_type"].Load(); got != 1 {
		t.Errorf("request_invalid_type = %d, want 1", got)
	}
	if got := ts.counters["request_decoding_error"].Load(); got != 0 {
		t.Errorf("request_decoding_error = %d, want 0", got)
	}
}

func TestHandle_UnknownCluster(t *testing.T) {
	reg := stats.NewRegistry()
	ts := newThriftStats(reg, "tp")
	f := newTestFilter(ts, oneRoute("ping", "tc"), false,
		func(context.Context, string) (network.UpstreamDialFunc, func(), resolveStatus) {
			return nil, nil, resolveUnknownCluster
		})
	down, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		_, _ = client.Write(framedBinaryCall(msgTypeCall, "ping", 1))
		_ = client.Close()
	}()
	go func() { f.Handle(context.Background(), down); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Handle hung on an unknown cluster; want graceful close")
	}
	if got := ts.counters["unknown_cluster"].Load(); got != 1 {
		t.Errorf("unknown_cluster = %d, want 1", got)
	}
	if got := ts.counters["no_healthy_upstream"].Load(); got != 0 {
		t.Errorf("no_healthy_upstream = %d, want 0", got)
	}
}

// twoRoutes builds a route table with two exact-method routes → two distinct
// clusters. First-match ordering means each method resolves to its own cluster.
func twoRoutes(m1, c1, m2, c2 string) *routeTable {
	return &routeTable{entries: []routeEntry{
		{methodName: m1, cluster: c1},
		{methodName: m2, cluster: c2},
	}}
}

// clusterResolver wires a resolveFunc that dispatches the dial closure by cluster
// name to a distinct backend conn, recording a per-cluster dial count. An unknown
// cluster name (no map entry) resolves as resolveUnknownCluster.
func clusterResolver(backends map[string]net.Conn, dials map[string]int) resolveFunc {
	return func(_ context.Context, clusterName string) (network.UpstreamDialFunc, func(), resolveStatus) {
		conn, ok := backends[clusterName]
		if !ok {
			return nil, nil, resolveUnknownCluster
		}
		return func(context.Context) (net.Conn, error) {
			dials[clusterName]++
			return conn, nil
		}, nil, resolveOK
	}
}

// TestHandle_CrossClusterReuse drives two CALLs with different methods routing to
// two different clusters over a SINGLE downstream connection. This exercises the
// defensive cross-cluster re-dial branch in ensureUpstream (upCluster != clusterName):
// the first frame dials c1, the second frame must CLOSE the c1 upstream and DIAL c2.
// It asserts both replies come back from the correct backend (byte-equivalent) and
// that each cluster was dialed exactly once — proving the re-dial fired and the old
// upstream was released (observed via EOF on the c1 backend after re-dial).
func TestHandle_CrossClusterReuse(t *testing.T) {
	reg := stats.NewRegistry()
	ts := newThriftStats(reg, "tp")

	// Backend c1: serve one REPLY tagged "Ping", then observe that the pump closes
	// our upstream conn when the second (cross-cluster) frame triggers a re-dial.
	srv1, cli1 := net.Pipe()
	reply1 := framedBinaryReply("Ping", 11, []byte{0x00})
	c1Closed := make(chan struct{})
	go func() {
		br := bufio.NewReader(srv1)
		_, _ = decodeFrame(br)    // read the CALL("Ping")
		_, _ = srv1.Write(reply1) // void-success REPLY
		_, err := br.ReadByte()   // blocks until the pump closes our side (re-dial)
		if err == io.EOF || err == io.ErrClosedPipe {
			close(c1Closed)
		} else {
			// any non-EOF read means the old upstream was NOT closed on re-dial.
			t.Errorf("c1 backend: expected EOF/closed after re-dial, got byte/err=%v", err)
			close(c1Closed)
		}
	}()

	// Backend c2: serve one REPLY tagged "Pong".
	srv2, cli2 := net.Pipe()
	reply2 := framedBinaryReply("Pong", 22, []byte{0x00})
	go func() {
		br := bufio.NewReader(srv2)
		_, _ = decodeFrame(br) // read the CALL("Pong")
		_, _ = srv2.Write(reply2)
	}()

	dials := map[string]int{}
	resolve := clusterResolver(map[string]net.Conn{"c1": cli1, "c2": cli2}, dials)
	f := newTestFilter(ts, twoRoutes("Ping", "c1", "Pong", "c2"), true, resolve)

	down, client := net.Pipe()
	got1 := make(chan []byte, 1)
	got2 := make(chan []byte, 1)
	go func() {
		// Frame 1: CALL("Ping") → c1.
		_, _ = client.Write(framedBinaryCall(msgTypeCall, "Ping", 11))
		buf1 := make([]byte, len(reply1))
		_, _ = io.ReadFull(client, buf1)
		got1 <- buf1
		// Frame 2: CALL("Pong") → c2 (forces the cross-cluster re-dial).
		_, _ = client.Write(framedBinaryCall(msgTypeCall, "Pong", 22))
		buf2 := make([]byte, len(reply2))
		_, _ = io.ReadFull(client, buf2)
		got2 <- buf2
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)

	if g := <-got1; string(g) != string(reply1) {
		t.Errorf("frame1 reply = %x, want c1 reply %x", g, reply1)
	}
	if g := <-got2; string(g) != string(reply2) {
		t.Errorf("frame2 reply = %x, want c2 reply %x", g, reply2)
	}
	// The defensive re-dial branch must have fired: c1 dialed once, c2 dialed once.
	if dials["c1"] != 1 {
		t.Errorf("c1 dials = %d, want 1", dials["c1"])
	}
	if dials["c2"] != 1 {
		t.Errorf("c2 dials = %d, want 1 (cross-cluster re-dial)", dials["c2"])
	}
	// The old (c1) upstream must have been closed when the c2 frame re-dialed.
	select {
	case <-c1Closed:
	case <-time.After(2 * time.Second):
		t.Fatal("c1 upstream was not closed on cross-cluster re-dial")
	}
	// Two requests, both CALLs, both routed (no miss).
	if got := ts.counters["request"].Load(); got != 2 {
		t.Errorf("request = %d, want 2", got)
	}
	if got := ts.counters["response_success"].Load(); got != 2 {
		t.Errorf("response_success = %d, want 2", got)
	}
	if got := ts.counters["route_missing"].Load(); got != 0 {
		t.Errorf("route_missing = %d, want 0", got)
	}
	if got := ts.active.Load(); got != 0 {
		t.Errorf("request_active = %d after Handle, want 0 (defer-balanced)", got)
	}
}

func TestHandle_NoHealthyUpstream(t *testing.T) {
	reg := stats.NewRegistry()
	ts := newThriftStats(reg, "tp")
	f := newTestFilter(ts, oneRoute("ping", "tc"), false,
		func(context.Context, string) (network.UpstreamDialFunc, func(), resolveStatus) {
			return nil, nil, resolveNoHealthyUpstream
		})
	down, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		_, _ = client.Write(framedBinaryCall(msgTypeCall, "ping", 1))
		_ = client.Close()
	}()
	go func() { f.Handle(context.Background(), down); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Handle hung on no healthy upstream; want graceful close")
	}
	if got := ts.counters["no_healthy_upstream"].Load(); got != 1 {
		t.Errorf("no_healthy_upstream = %d, want 1", got)
	}
}
