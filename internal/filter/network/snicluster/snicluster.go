// internal/filter/network/snicluster/snicluster.go — config-less L4 read filter
// that publishes the TLS SNI as the per-connection upstream-cluster-override
// (ADR-0220). Mirrors echo's config-less parse shape.

// Package snicluster implements envoy.filters.network.sni_cluster — a config-less
// L4 read filter that publishes the TLS SNI verbatim as the per-connection
// upstream-cluster-override (ADR-0220). It reads Connection().RequestedServerName()
// in OnNewConnection and, when non-empty, calls ReadFilterCallbacks.SetUpstreamCluster
// (the narrow override seam, ADR-0219); the terminal tcp_proxy consumes it to
// override its configured cluster. Mirrors echo's config-less parse shape.
package snicluster

import (
	"fmt"

	sni_clusterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/sni_cluster/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/filter/network"
)

// TypeURL is the canonical Any type URL for sni_cluster's typed_config.
// Derived from proto.MessageName so the extensions. segment is guaranteed exact
// (verified at Task 1; memory reference_network_filter_typeurl_extensions).
// Declared as const to mirror echo/directresponse/tcpproxy; byte-stable form
// pinned by TestTypeURLHasExtensionsSegment so it cannot drift from the proto.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.network.sni_cluster.v3.SniCluster"

// New is the NetworkFilterFactory for sni_cluster, registered at boot under
// TypeURL. The proto (sni_clusterv3.SniCluster) has no fields; an empty/absent
// typed_config is accepted (echo shape). A structurally-malformed Any body
// surfaces "sni_cluster: invalid typed_config: %w".
func New(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
	if tc != nil && len(tc.GetValue()) > 0 {
		if err := tc.UnmarshalTo(&sni_clusterv3.SniCluster{}); err != nil {
			return nil, fmt.Errorf("sni_cluster: invalid typed_config: %w", err)
		}
	}
	return func() network.NetworkFilter { return &filter{} }, nil
}

// filter publishes the SNI as the per-connection upstream-cluster-override.
type filter struct {
	network.Marker
	cb network.ReadFilterCallbacks
}

// OnNewConnection reads the SNI and, when non-empty, publishes it verbatim as
// the per-connection upstream-cluster-override (F-SNI). MUST return Continue:
// an OnNewConnection StopIteration would set the chain's sticky connHalted flag
// and block all OnData (the chain constraint; memory
// reference_network_read_filter_onnewconnection_halts). Envoy's sni_cluster
// also returns Continue (D27-6), so constraint and parity coincide.
func (f *filter) OnNewConnection() network.Status {
	if sni := f.cb.Connection().RequestedServerName(); sni != "" {
		f.cb.SetUpstreamCluster(sni) // verbatim — no transform (F-SNI, ADR-0220)
	}
	return network.Continue
}

// OnData is a pass-through Continue — sni_cluster does not inspect payload; it
// passes bytes through so the chain reaches the terminal (unlike echo, which
// halts/drains). Makes [sni_cluster, tcp_proxy] a mixed read→terminal chain.
func (f *filter) OnData(_ *network.Buffer, _ bool) network.Status { return network.Continue }

// SetReadFilterCallbacks stores the per-connection callbacks handle.
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks) { f.cb = cb }

// OnDestroy is a no-op; sni_cluster holds no per-connection resources.
func (f *filter) OnDestroy() {}
