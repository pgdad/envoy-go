package hcm

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"

	"github.com/pgdad/envoy-go/internal/drain"
	"github.com/pgdad/envoy-go/internal/stats"
)

func TestNewFilter_HappyPath(t *testing.T) {
	cm := mkClusterManager(t)
	f, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil), cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil)
	if err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}
	if f.statPrefix != "ingress_http" {
		t.Errorf("statPrefix: got %q, want %q", f.statPrefix, "ingress_http")
	}
	if len(f.table.routes) != 1 {
		t.Errorf("table.routes: got %d, want 1", len(f.table.routes))
	}
}

func TestNewFilter_PreservesParseErrorPrefix(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP2 })
	// Zero-value ListenerCtx (no TLS, no allowH2C); HTTP2 must be rejected.
	if _, err := NewFilterWithCtxAndSinksAndRegistry(any, cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil); err == nil || !strings.HasPrefix(err.Error(), "hcm:") {
		t.Errorf("expected hcm:-prefixed error, got: %v", err)
	}
}

// TestNewFilter_Allocates5HCMMetrics — Phase 06.1 Task 11: the HCM constructor
// must register the 5 HCM-scope per-instance counters per SPEC §6
// ("HCM — 5 names") on the supplied Registry, keyed by stat_prefix from the
// HCM config.
func TestNewFilter_Allocates5HCMMetrics(t *testing.T) {
	cm := mkClusterManager(t)
	r := stats.NewRegistry()
	if _, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil), cm, ListenerCtx{}, r, nil, testHTTPRegistry(), nil, nil); err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}
	want := map[string]bool{
		"http.ingress_http.downstream_rq_total": false,
		"http.ingress_http.downstream_rq_2xx":   false,
		"http.ingress_http.downstream_rq_3xx":   false,
		"http.ingress_http.downstream_rq_4xx":   false,
		"http.ingress_http.downstream_rq_5xx":   false,
	}
	r.Walk(func(m stats.Metric) {
		if _, ok := want[m.Name()]; ok {
			want[m.Name()] = true
		}
	})
	for name, seen := range want {
		if !seen {
			t.Errorf("missing HCM-scope metric %q in Registry after filter build", name)
		}
	}
}

// TestFilter_RequestEntry_IncsDownstreamRqTotal — Phase 06.1 Task 11 hot path:
// driving a single HTTP/1.1 GET / through a Filter Inc's downstream_rq_total
// by exactly 1. Per SPEC §12 #1 site (a): "first byte of request line/headers
// in connection.go's read loop" — once-per-request, before route-match.
func TestFilter_RequestEntry_IncsDownstreamRqTotal(t *testing.T) {
	cm := mkClusterManager(t)
	r := stats.NewRegistry()
	f, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil), cm, ListenerCtx{}, r, nil, testHTTPRegistry(), nil, nil)
	if err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}
	if got := f.downstreamRqTotal.Load(); got != 0 {
		t.Fatalf("downstream_rq_total pre-request = %d, want 0", got)
	}

	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go f.Handle(context.Background(), server)

	writeRequest(t, client, "GET", "/health", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Errorf("status: got %d, want 200", got)
	}
	if got := f.downstreamRqTotal.Load(); got != 1 {
		t.Errorf("downstream_rq_total post-request = %d, want 1", got)
	}
}

// TestFilter_ResponseFinalization_IncsStatusClassCounter — Phase 06.1 Task 11
// hot path: a 200 response Inc's downstream_rq_2xx; 404 Inc's downstream_rq_4xx.
// Per SPEC §5.5 + §6: "switch on response status class → downstream_rq_<Nxx>.Inc()
// once per response. Lives where the response status code is finalized, before
// bytes hit the wire." 200 comes from the /health direct_response; 404 comes
// from the no-route-matched catch-all (mkHCM defines only /health).
func TestFilter_ResponseFinalization_IncsStatusClassCounter(t *testing.T) {
	cm := mkClusterManager(t)
	r := stats.NewRegistry()
	f, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil), cm, ListenerCtx{}, r, nil, testHTTPRegistry(), nil, nil)
	if err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}

	// 200 — /health direct_response.
	client200, server200 := connPair(t)
	defer func() { _ = client200.Close() }()
	go f.Handle(context.Background(), server200)
	writeRequest(t, client200, "GET", "/health", "Connection: close")
	if got := readResponseStatus(t, client200); got != 200 {
		t.Fatalf("/health status: got %d, want 200", got)
	}
	if got := f.downstreamRq2xx.Load(); got != 1 {
		t.Errorf("downstream_rq_2xx after 200 = %d, want 1", got)
	}

	// 404 — no route match for /missing (mkHCM defines only /health).
	client404, server404 := connPair(t)
	defer func() { _ = client404.Close() }()
	go f.Handle(context.Background(), server404)
	writeRequest(t, client404, "GET", "/missing", "Connection: close")
	if got := readResponseStatus(t, client404); got != 404 {
		t.Fatalf("/missing status: got %d, want 404", got)
	}
	if got := f.downstreamRq4xx.Load(); got != 1 {
		t.Errorf("downstream_rq_4xx after 404 = %d, want 1", got)
	}

	// 3xx and 5xx counters remain at 0 — no path produced those classes.
	if got := f.downstreamRq3xx.Load(); got != 0 {
		t.Errorf("downstream_rq_3xx unexpected = %d, want 0", got)
	}
	if got := f.downstreamRq5xx.Load(); got != 0 {
		t.Errorf("downstream_rq_5xx unexpected = %d, want 0", got)
	}
}

// TestFilter_Handle_HTTP2_PlaintextH2C verifies that a Filter with
// codec_type=HTTP2 (built with AllowH2C=true) dispatches to the H2 driver.
// The H2 driver reads the client preface; since we close the conn immediately
// without sending it, the driver returns a preface error. We assert that
// Handle returns (not blocking forever) and that the conn is closed.
func TestFilter_Handle_HTTP2_PlaintextH2C(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP2 })
	f, err := NewFilterWithCtxAndSinksAndRegistry(any, cm, ListenerCtx{AllowH2C: true}, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil)
	if err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}

	client, server := connPair(t)
	// Close client immediately — H2 driver will get EOF reading preface.
	_ = client.Close()

	done := make(chan struct{})
	go func() { defer close(done); f.Handle(context.Background(), server) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("Handle did not return after H2 preface read failed (conn closed)")
	}
}

// TestFilter_Handle_AUTO_Plaintext_DispatchesToH1 verifies that a Filter with
// codec_type=AUTO dispatches to the H1 driver when the downstream is a plain
// net.Conn (not a *tls.Conn). We send a basic HTTP/1.1 request and verify a
// response is returned.
func TestFilter_Handle_AUTO_Plaintext_DispatchesToH1(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_AUTO })
	f, err := NewFilterWithCtxAndSinksAndRegistry(any, cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil)
	if err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}

	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go f.Handle(context.Background(), server)

	writeRequest(t, client, "GET", "/health", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Errorf("AUTO plaintext dispatch: status got %d, want 200", got)
	}
}

func TestFilter_Handle_OneRequestThenEOF(t *testing.T) {
	cm := mkClusterManager(t)
	f, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil), cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go f.Handle(context.Background(), server)

	writeRequest(t, client, "GET", "/health", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Errorf("status: got %d, want 200", got)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil {
		t.Error("expected EOF/read-error after Connection: close, got bytes")
	}
}

func TestFilter_Handle_CtxAlreadyCancelledShortCircuits(t *testing.T) {
	cm := mkClusterManager(t)
	f, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil), cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() { defer close(done); f.Handle(ctx, server) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Handle did not return promptly on canceled ctx")
	}
}

// TestHCM_DrainInflightBalance — Phase 08.2 Task 9: a request that completes
// normally must Dec the inflight counter (so Done() fires after Drain()).
func TestHCM_DrainInflightBalance(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	cm := mkClusterManager(t)
	f, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil), cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), dm, nil)
	if err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}
	srv, client := net.Pipe()
	defer func() { _ = srv.Close(); _ = client.Close() }()
	go f.Handle(context.Background(), srv)
	_, _ = client.Write([]byte("GET /health HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"))
	buf := make([]byte, 1024)
	_ = client.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _ = client.Read(buf)
	dm.Drain()
	select {
	case <-dm.Done():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("dm.Done() did not fire — inflight not balanced")
	}
}

// TestHCM_DrainInflightBalance_SendLocalReply — Phase 08.2 Task 9: a request
// that hits the no-route 404 sendLocalReply path must still Dec via defer so
// Done() fires after Drain().
func TestHCM_DrainInflightBalance_SendLocalReply(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	cm := mkClusterManager(t)
	f, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil), cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), dm, nil)
	if err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}
	srv, client := net.Pipe()
	defer func() { _ = srv.Close(); _ = client.Close() }()
	go f.Handle(context.Background(), srv)
	_, _ = client.Write([]byte("GET /no-route-match HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"))
	buf := make([]byte, 1024)
	_ = client.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _ = client.Read(buf) // expect 404
	dm.Drain()
	select {
	case <-dm.Done():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("dm.Done() did not fire after sendLocalReply path — markedInflight unbalanced")
	}
}

// TestHCM_DrainInflightBalance_NilDrainManager — Phase 08.2 Task 9: a nil dm
// must not panic; the nil-tolerant gate skips Inc/Dec silently.
func TestHCM_DrainInflightBalance_NilDrainManager(t *testing.T) {
	cm := mkClusterManager(t)
	f, err := NewFilterWithCtxAndSinksAndRegistry(mkHCM(nil), cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil)
	if err != nil {
		t.Fatalf("NewFilterWithCtxAndSinksAndRegistry: %v", err)
	}
	srv, client := net.Pipe()
	defer func() { _ = srv.Close(); _ = client.Close() }()
	go f.Handle(context.Background(), srv)
	_, _ = client.Write([]byte("GET /health HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"))
	buf := make([]byte, 1024)
	_ = client.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _ = client.Read(buf)
	// Test passes if no panic.
}

// Compile-time check that Filter implements the listener.filterHandler shape.
var _ filterHandlerShape = (*Filter)(nil)

type filterHandlerShape interface {
	Handle(ctx context.Context, downstream net.Conn)
}
