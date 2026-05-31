// internal/filter/network/callbacks.go — per-connection callbacks + connection accessor

package network

import (
	"net"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
)

// ReadFilterCallbacks is the per-connection callback surface handed to each
// ReadFilter via SetReadFilterCallbacks. Mirrors upstream
// Network::ReadFilterCallbacks. The DynamicMetadata accessor is shaped at 26.1
// (parent AMEND-A5) so the 26.3 rbac_network shadow-metadata sink drops in
// without a callbacks revision.
type ReadFilterCallbacks interface {
	// Connection returns the per-connection accessor (write/close/addrs/SNI/
	// principals). The accessor surface supplies the L4 inputs the 26.3 rbac
	// matcher subset needs (parent §11.3 D3).
	Connection() Connection
	// ContinueReading resumes a chain halted by StopIteration, restarting at
	// the NEXT filter with the currently-available buffered bytes (parent
	// §11.5 D5). No-op if the chain is not halted.
	ContinueReading()
	// DynamicMetadata returns the per-connection dynamic-metadata bucket
	// (owned by the per-connection runtime context; REUSE of
	// internal/dynamicmetadata/ at connection scope per parent AMEND-A5).
	// Unused by echo/direct_response at 26.1; consumed by rbac_network at 26.3.
	DynamicMetadata() *dynamicmetadata.Bucket
}

// CloseType selects the connection-close semantics. FlushWrite (flush pending
// write then close) is used by direct_response (parent §11.7 D7); NoFlush
// (close immediately, dropping any pending downstream write) is used by 26.3
// rbac enforced-deny (parent AMEND-A7). As of 26.3 (F3, D-26.3-2) the two are
// RECORDED + honored distinctly by the framework: connection.Close records the
// requested type and serveNetworkChain honors it at the pure-read close site.
// Upstream's FlushWriteAndDelay/Abort/AbortReset are a documented forward-
// extension under the ADR-0213 API-revision allowance.
type CloseType int

const (
	// FlushWrite flushes any pending write buffer to the downstream socket,
	// then closes the connection.
	FlushWrite CloseType = iota
	// NoFlush closes immediately, discarding any pending downstream write
	// (RST semantics via SO_LINGER 0).
	NoFlush
)

// Connection is the per-connection accessor a ReadFilter uses to write to /
// close the downstream connection and to read L4 connection facts. Mirrors the
// subset of upstream Network::Connection the phase-26 filters consume.
type Connection interface {
	// Write writes data to the downstream connection. endStream=true signals
	// no further writes follow (direct_response sets it; echo propagates the
	// downstream end_stream).
	Write(data []byte, endStream bool)
	// Close closes the downstream connection with the given semantics. The
	// CloseType (FlushWrite vs NoFlush) is recorded and honored by the framework
	// as of 26.3 (F3, D-26.3-2): NoFlush closes immediately, discarding any
	// pending downstream write (RST semantics via SO_LINGER 0); FlushWrite
	// drains then closes.
	Close(ct CloseType)
	// LocalAddr / RemoteAddr return the connection's bound / peer address.
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	// RequestedServerName returns the SNI extracted by tls_inspector (the
	// listener-filter ChainMatchInputs.ServerName), or "" if none.
	RequestedServerName() string
	// DownstreamPrincipals returns the mTLS peer-cert URI/DNS-SAN principals
	// from the TLS handshake, or nil if the connection is not mTLS. Shaped for
	// 26.3 rbac `authenticated` principal matching.
	DownstreamPrincipals() []string
}
