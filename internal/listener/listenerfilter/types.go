package listenerfilter

import (
	"context"
	"net"

	"google.golang.org/protobuf/types/known/anypb"
)

// ListenerFilterStatus is the result of a ListenerFilter.Inspect call.
//
//nolint:revive // ADR-0079 reserves the ListenerFilterStatus name for the dispatch-protocol surface.
type ListenerFilterStatus int

const (
	// Continue allows the pipeline to advance to the next filter (or finish
	// if this is the last filter).
	Continue ListenerFilterStatus = 0
	// StopIteration halts the pipeline; remaining filters are skipped. The
	// chain-match algorithm runs on whatever inputs were populated so far.
	StopIteration ListenerFilterStatus = 1
)

// ChainMatchInputs holds the eight chain-match dimensions that
// chainmatch.SelectChain consults. Some fields are populated from
// connection-level facts at Pipeline construction (LocalAddr/RemoteAddr);
// others are populated by listener filters (e.g., tls_inspector contributes
// ServerName, TransportProtocol, ApplicationProtocols).
//
// The struct is mutated in place by listener filters during pipeline
// dispatch. Callers that need an immutable snapshot must copy.
type ChainMatchInputs struct {
	// DestinationIP is the listener's bound IP address (conn.LocalAddr().IP).
	DestinationIP net.IP
	// DestinationPort is the listener's bound port (conn.LocalAddr().Port).
	DestinationPort uint32
	// SourceIP is the accepted connection's peer IP (conn.RemoteAddr().IP).
	SourceIP net.IP
	// SourcePort is the accepted connection's peer port (conn.RemoteAddr().Port).
	SourcePort uint32
	// ServerName is the SNI extracted from the ClientHello by tls_inspector.
	// Empty when no TLS inspection happened or the ClientHello had no SNI.
	ServerName string
	// TransportProtocol is "tls" if a TLS ClientHello was detected,
	// "raw_buffer" if the byte preamble was non-TLS, or "" if no listener
	// filter inspected the connection.
	TransportProtocol string
	// ApplicationProtocols is the ALPN offer list extracted from the
	// ClientHello by tls_inspector. Empty when no TLS inspection happened
	// or the ClientHello had no ALPN extension.
	ApplicationProtocols []string
}

// IsLoopbackSource reports whether SourceIP is in the IPv4 127.0.0.0/8 range
// or is the IPv6 ::1 address. Used by the source_type: LOCAL match rule.
func (c *ChainMatchInputs) IsLoopbackSource() bool {
	return c.SourceIP != nil && c.SourceIP.IsLoopback()
}

// Peeker is the peek-without-consume interface listener filters call to
// inspect the connection's byte preamble. Returns up to n bytes without
// advancing the read position. Subsequent Read calls drain the same bytes.
type Peeker interface {
	Peek(n int) ([]byte, error)
}

// ListenerFilter is the per-connection filter interface. Implementations
// inspect the peeker (read without consuming) and write to inputs (mutate
// chain-match inputs in place). Return Continue to allow the pipeline to
// advance, StopIteration to halt the pipeline (whatever inputs were
// populated stand). On non-nil error the pipeline aborts with the error.
type ListenerFilter interface {
	Inspect(ctx context.Context, peeker Peeker, inputs *ChainMatchInputs) (ListenerFilterStatus, error)
	// OnDestroy is called when the per-connection pipeline ends (either
	// after dispatch completes or on connection close before dispatch).
	// Implementations release any per-connection resources here.
	OnDestroy()
}

// InitialReadBufferSizer is an OPTIONAL interface a ListenerFilter
// implements to hint how many connection-preamble bytes it needs to Peek
// (tls_inspector reports its parsed initial_read_buffer_size). The listener
// manager probes one sample instance per filter at build time and sizes the
// per-connection peekerConn to the LARGEST hint (via NewPeekerConnSize), so
// a configured size > the 4096 default is actually honored. Filters without
// the interface fall back to the default-size peek buffer.
type InitialReadBufferSizer interface {
	InitialReadBufferSize() int
}

// FactoryCtx carries the parsed-config context a ListenerFilterFactory
// needs to resolve the typed_config Any. Currently empty; reserved for
// future extensions (e.g., a Registry pointer for filters that compose).
type FactoryCtx struct{}

// ListenerFilterFactory parses + validates a listener filter's typed_config
// Any once at NewManager-build time and returns a per-connection
// FilterInstanceFactory closure. Mirrors 07.1's HTTPFilterFactory pattern
// (ADR-0072).
//
//nolint:revive // ADR-0079 reserves the ListenerFilterFactory name for the boot-time factory surface.
type ListenerFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)

// FilterInstanceFactory allocates a fresh ListenerFilter instance once per
// accepted connection. Per-config validation cost is paid once at boot;
// per-connection cost is one allocation.
type FilterInstanceFactory func() ListenerFilter
