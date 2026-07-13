package hcm

import (
	"context"
	stdtls "crypto/tls"
	"log"
	"net"

	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/drain"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/httpclient"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/tracing"
)

// Compile-time assertion that *Filter satisfies network.TerminalFilter (26.2
// Task 6; R-T). The sealed isNetworkFilter() comes from the network.Marker
// embed on Filter (config.go); Handle below is byte-identical to
// TerminalFilter.Handle — only the marker embed was added to seal it.
var _ network.TerminalFilter = (*Filter)(nil)

// NewNetworkFactory returns a network.NetworkFilterFactory that builds the HCM
// terminal filter once per chain at boot, capturing the boot singletons (cm,
// registry, accessLogSinks, httpRegistry, dm, httpClient) and bridging the
// per-chain network.FactoryCtx into hcm.ListenerCtx. It yields the SHARED
// *Filter per accepted connection (HCM is conn-stateless across its Handle
// serve loop). This is the bridge the retired manager.go filterRegistry HCM
// closure performed (manager.go:112-131), moved into the hcm package; Task 10
// deletes that closure as a true no-op.
func NewNetworkFactory(
	cm *cluster.Manager,
	registry *stats.Registry,
	accessLogSinks []accesslog.Sink,
	httpRegistry *filter_http.HTTPRegistry,
	dm *drain.Manager,
	httpClient *httpclient.Client,
	tracingExporters *tracing.ExporterProvider,
) network.NetworkFilterFactory {
	return func(tc *anypb.Any, ctx network.FactoryCtx) (network.FilterInstanceFactory, error) {
		f, err := NewFilterWithCtxAndSinksAndRegistry(
			tc, cm,
			ListenerCtx{
				HasTLS:             ctx.HasTLS,
				AllowH2C:           ctx.AllowH2C,
				IsQUIC:             ctx.IsQUIC,
				ListenerPrincipal:  ctx.ListenerPrincipal,
				HTTPClient:         httpClient,
				NodeServiceCluster: ctx.NodeServiceCluster,
			},
			registry, accessLogSinks, httpRegistry, dm, tracingExporters,
		)
		if err != nil {
			return nil, err
		}
		return func() network.NetworkFilter { return f }, nil
	}
}

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
	dm *drain.Manager,
	tracingExporters *tracing.ExporterProvider,
) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, lc, registry, accessLogSinks, httpRegistry, dm, tracingExporters)
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
//
// Phase 16 Task 6 (ADR-0144): the per-connection h2Dispatcher captures the
// downstream TLS principal-name candidates extracted once at connection
// build time. All H2 streams on this conn share the same priority-ordered
// candidate slice — symmetric to H1's per-request extraction (the H1 conn
// is also pinned to a single TLS state for the connection lifetime, so
// per-conn caching mirrors that semantic). The dispatcher seeds the
// per-stream chain via chain.SetTLSPrincipals before RunDecodeHeaders
// dispatch in chainDispatchAction.WriteH2.
func (f *Filter) runH2(ctx context.Context, downstream net.Conn) {
	disp := newH2Dispatcher(f)
	disp.tlsPrincipals = downstreamTLSPrincipals(downstream)
	// Phase 22.2 Task 6 (ADR-0192): capture the FULL *tls.ConnectionState ONCE
	// at H2 connection build time — symmetric to tlsPrincipals above and to
	// the H1 path's per-request capture in dispatchRequest. The snapshot
	// pointer threads into every per-stream chain via chainDispatchAction →
	// chain.SetTLSConnectionState before RunDecodeHeaders dispatch.
	// downstreamTLSConnectionState returns nil for plaintext / non-*tls.Conn /
	// pre-handshake conns; the dispatcher field stays nil and the per-stream
	// SetTLSConnectionState(nil) call is the documented nil-passthrough.
	// Per SPEC §11.5.3 + ADR-0192 §Decision body anticipation (chain-side
	// extension lives INSIDE ADR-0192 per Q13 WEAK HOLD).
	disp.tlsConnectionState = downstreamTLSConnectionState(downstream)
	// Phase 18.2 Task 4 (ADR-0165): capture the 4 per-connection callback-
	// surface-extension fields ONCE at H2 connection build time — symmetric
	// to tlsPrincipals + the H1 path's per-request capture in dispatchRequest.
	// The captured values thread into every per-stream chain via
	// chainDispatchAction → chain.SetX. Protocol/listenerPrincipal are NOT
	// captured here: protocol is invariant ("HTTP/2") at the dispatch site;
	// listenerPrincipal is the per-Filter listener value.
	//
	// Nil-downstream guard mirrors the H1 path discipline (tests that pass
	// nil exercise the dispatcher without a real listener-conn).
	if downstream != nil {
		disp.downstreamRemoteAddr = downstream.RemoteAddr()
		disp.downstreamLocalAddr = downstream.LocalAddr()
	}
	if tlsConn, ok := downstream.(*stdtls.Conn); ok {
		state := tlsConn.ConnectionState()
		disp.downstreamTLSServerName = state.ServerName
		if len(state.PeerCertificates) > 0 {
			disp.downstreamTLSPeerCertDER = state.PeerCertificates[0].Raw
		}
	}
	sc := h2.NewServerConn(ctx, downstream, disp, h2.DefaultServerSettings)
	if err := sc.Run(); err != nil {
		log.Printf("hcm: h2: %v", err)
	}
}
