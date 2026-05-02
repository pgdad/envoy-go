package hcm

import (
	"context"
	stdtls "crypto/tls"
	"log"
	"net"

	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// NewFilterWithCtxAndSinksAndRegistry parses the typed_config Any into a
// *Filter. Errors begin with "hcm: "; the listener manager wraps them with
// "listener: %q: filter_chains[%d]: ".
//
// Phase 07.1 Task 14 (per ADR-0072 / Decision §3.4): this is the SOLE HCM
// constructor. The four legacy variants (NewFilter, NewFilterWithCtx,
// NewFilterWithCtxAndSinks, plus the Task-13 transitional
// defaultRouterOnlyHTTPRegistry helper) were deleted; every caller now passes
// the boot-populated, frozen *filter_http.HTTPRegistry explicitly per ADR-0072's
// freeze-after-boot contract.
//
// Parameters:
//   - tc: the typed_config Any from a filter_chain[].filters[].typed_config slot
//   - clusters: the resolved cluster.Manager
//   - lc: phase-05.1 ListenerCtx (HasTLS, AllowH2C; per ADR-0049 + ADR-0050)
//   - registry: the *stats.Registry the 5 HCM-scope per-instance metrics are
//     allocated on (per SPEC §6 + Phase 06.1 Task 11). Must be non-nil and
//     non-Frozen at call time per SPEC §5.4 boot ordering.
//   - accessLogSinks: opened AsyncFileSinks from main.go (Phase 06.2 Task 14;
//     nil/empty means no access logging on this listener).
//   - httpRegistry: the *filter_http.HTTPRegistry the parser uses to resolve
//     each http_filters[] entry's typed_config.type_url to a per-instance
//     factory closure (per ADR-0072). Must be non-nil and Frozen at call
//     time. The four chain-shape rules per SPEC §1 #6 + ADR-0071 are applied
//     via filter_http.ValidateChainShape.
func NewFilterWithCtxAndSinksAndRegistry(
	tc *anypb.Any,
	clusters *cluster.Manager,
	lc ListenerCtx,
	registry *stats.Registry,
	accessLogSinks []accesslog.Sink,
	httpRegistry *filter_http.HTTPRegistry,
) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, lc, registry, accessLogSinks, httpRegistry)
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
