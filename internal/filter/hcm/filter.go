package hcm

import (
	"context"
	stdtls "crypto/tls"
	"log"
	"net"

	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
	"github.com/esalaine/envoy-go/internal/stats"
)

// NewFilter parses the typed_config Any into a *Filter. Errors begin with
// "hcm: "; the listener manager wraps them with "listener: %q: filter_chains[%d]: ".
//
// Phase 06.1 Task 11 widened the signature with a trailing *stats.Registry —
// see NewFilterWithCtx (config.go) for the contract. The 5 HCM-scope metrics
// per SPEC §6 are allocated at filter-build time, pre-Freeze.
//
// Task 13 transitional: this legacy constructor builds a default router-only
// HTTPRegistry so the http_filters[] chain validates clean. Task 14 sweeps
// all callers to a registry-aware constructor and DELETES this function.
func NewFilter(tc *anypb.Any, clusters *cluster.Manager, registry *stats.Registry) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, ListenerCtx{}, registry, nil, defaultRouterOnlyHTTPRegistry())
}

// Handle drives one downstream connection from acceptance to close. ALPN
// dispatch (phase 05.1, ADR-0050): on a TLS conn with ALPN==h2, dispatch to
// the h2 codec; otherwise dispatch to the phase-04 H1 driver.
//
// Filter implements internal/listener.filterHandler:
//
//	Handle(ctx context.Context, downstream net.Conn)
//
// (No error return — matches the phase-02 tcpproxy.Filter.Handle precedent.)
func (f *Filter) Handle(ctx context.Context, downstream net.Conn) {
	if err := ctx.Err(); err != nil {
		_ = downstream.Close()
		return
	}
	defer func() { _ = downstream.Close() }()

	switch f.codecType {
	case hcmv3.HttpConnectionManager_HTTP1:
		runConnection(ctx, downstream, f)
		return
	case hcmv3.HttpConnectionManager_HTTP2:
		f.runH2(ctx, downstream)
		return
	case hcmv3.HttpConnectionManager_AUTO:
		if tlsConn, ok := downstream.(*stdtls.Conn); ok {
			// Defensive: ensure handshake is complete so NegotiatedProtocol is
			// authoritative. Idempotent for already-handshaken conns.
			// SPEC §11.6 mitigation.
			_ = tlsConn.HandshakeContext(ctx)
			if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
				f.runH2(ctx, downstream)
				return
			}
		}
		runConnection(ctx, downstream, f)
		return
	}
}

// runH2 constructs an h2.ServerConn for the downstream conn and runs it to
// completion. Connection-level errors are logged with "hcm: h2: " prefix.
func (f *Filter) runH2(ctx context.Context, downstream net.Conn) {
	disp := newH2Dispatcher(f)
	sc := h2.NewServerConn(ctx, downstream, disp, h2.DefaultServerSettings)
	if err := sc.Run(); err != nil {
		log.Printf("hcm: h2: %v", err)
	}
}
