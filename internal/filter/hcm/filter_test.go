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
	if _, err := NewFilter(any, cm); err == nil || !strings.HasPrefix(err.Error(), "hcm:") {
		t.Errorf("expected hcm:-prefixed error, got: %v", err)
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
