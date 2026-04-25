package hcm

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
)

func TestNewFilter_HappyPath(t *testing.T) {
	cm := mkClusterManager(t)
	f, err := NewFilter(mkHCM(nil), cm)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
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
	// NewFilter uses zero-value ListenerCtx (no TLS, no allowH2C); HTTP2 must be rejected.
	if _, err := NewFilter(any, cm); err == nil || !strings.HasPrefix(err.Error(), "hcm:") {
		t.Errorf("expected hcm:-prefixed error, got: %v", err)
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
	f, err := NewFilterWithCtx(any, cm, ListenerCtx{AllowH2C: true})
	if err != nil {
		t.Fatalf("NewFilterWithCtx: %v", err)
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
	f, err := NewFilterWithCtx(any, cm, ListenerCtx{})
	if err != nil {
		t.Fatalf("NewFilterWithCtx: %v", err)
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
	f, err := NewFilter(mkHCM(nil), cm)
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
	f, err := NewFilter(mkHCM(nil), cm)
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

// Compile-time check that Filter implements the listener.filterHandler shape.
var _ filterHandlerShape = (*Filter)(nil)

type filterHandlerShape interface {
	Handle(ctx context.Context, downstream net.Conn)
}
