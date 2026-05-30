// internal/filter/network/terminal.go (NEW)

package network

import (
	"context"
	"net"
)

// NetworkFilter is the sealed common interface every chain filter satisfies —
// either a ReadFilter (OnData inspection model) or a TerminalFilter
// (connection-takeover model). The chain builder classifies each; the dispatch
// type-switches. Sealed via the unexported isNetworkFilter() method, granted
// ONLY by embedding the exported Marker — so a value cannot satisfy
// NetworkFilter without being a deliberate network filter.
//
//nolint:revive // ADR-0215 reserves the network.NetworkFilter name.
type NetworkFilter interface {
	isNetworkFilter()
}

// Marker is the exported embeddable that grants the sealed isNetworkFilter()
// method. Out-of-package filters (echo, direct_response, tcp_proxy, HCM) embed
// network.Marker to satisfy ReadFilter / TerminalFilter (which embed
// NetworkFilter). Zero-size; no state.
type Marker struct{}

func (Marker) isNetworkFilter() {}

// TerminalFilter is a network filter that takes over the downstream connection
// at the END of the chain (tcp_proxy: L4 bidirectional pump to an upstream
// cluster member; HCM: the HTTP/1 driver or HTTP/2 codec). Unlike a ReadFilter
// (which inspects buffered bytes via OnData and writes through
// ReadFilterCallbacks.Connection), a TerminalFilter owns the raw net.Conn and
// runs a blocking serve loop to connection close. It mirrors upstream's
// terminal read filters (tcp_proxy/HCM), which return StopIteration and drive
// the connection directly. Handle's signature is byte-identical to the
// phase-02 manager.go filterHandler interface this seam retires, so the
// existing *tcpproxy.Filter + *hcm.Filter satisfy it with no method change.
//
//nolint:revive // ADR-0215 reserves the network.TerminalFilter name.
type TerminalFilter interface {
	NetworkFilter
	// Handle takes ownership of the downstream connection and runs to
	// completion. It owns the conn-close lifecycle (the unified dispatch does
	// NOT close the conn for a terminal filter; Handle's own defer conn.Close()
	// runs — byte-identical to the retired terminal path).
	Handle(ctx context.Context, downstream net.Conn)
}
