// internal/filter/network/types.go — iteration protocol + factory types

package network

import "google.golang.org/protobuf/types/known/anypb"

// Status is the result of a ReadFilter OnNewConnection / OnData call.
// Two values only (parent SPEC §11.5 D5): L4 buffering is connection-level
// (the chain owns one read Buffer for all filters), so there is no HTTP-style
// per-filter StopIterationAndBuffer variant.
//
//nolint:revive // ADR-0213 reserves the network.Status name for the read-filter dispatch surface.
type Status int

const (
	// Continue advances the read-filter chain to the next filter (or finishes
	// the iteration if this is the last filter).
	Continue Status = iota
	// StopIteration halts the chain at the current filter; undrained bytes
	// remain in the connection read buffer. Resume via cb.ContinueReading()
	// (which restarts at the NEXT filter with the live read buffer).
	StopIteration
)

// ReadFilter is the per-connection read-filter interface. Mirrors upstream
// Network::ReadFilter (onNewConnection + onData + initializeReadFilterCallbacks)
// + the envoy-go OnDestroy convention. A fresh instance is allocated per
// accepted connection by FilterInstanceFactory.
type ReadFilter interface {
	NetworkFilter
	// OnNewConnection is called eagerly per filter at connection accept,
	// before any downstream data, in chain order (parent §11.5 D5). Return
	// Continue to advance; StopIteration to halt (resume via ContinueReading).
	OnNewConnection() Status
	// OnData is called with the connection read buffer when bytes are
	// available (or on end-of-stream). The filter consumes bytes by draining
	// buf (e.g. via Connection().Write of buf.Bytes() + buf.Drain). Return
	// Continue to advance to the next filter; StopIteration to halt with
	// undrained bytes retained.
	OnData(buf *Buffer, endStream bool) Status
	// SetReadFilterCallbacks injects the per-connection callbacks. Called
	// once by the chain runner before OnNewConnection (envoy-go naming for
	// upstream initializeReadFilterCallbacks; mirrors http.SetDecoderCallbacks).
	SetReadFilterCallbacks(cb ReadFilterCallbacks)
	// OnDestroy releases per-connection resources. Called when the connection
	// dispatch ends, regardless of how iteration exited.
	OnDestroy()
}

// WriteFilter is the per-connection write-filter interface — the write-direction
// (upstream→downstream) half of the chain. Mirrors upstream Network::WriteFilter
// (onWrite + initializeWriteFilterCallbacks) + the envoy-go OnDestroy convention.
// A filter may implement ReadFilter, WriteFilter, or BOTH (one instance seeing
// both directions — upstream Network::Filter / addFilter parity, AMEND-A11).
//
//nolint:revive // ADR-0221 reserves the network.WriteFilter name for the write-direction dispatch surface.
type WriteFilter interface {
	NetworkFilter
	// OnWrite is called with the bytes a terminal filter writes downstream,
	// BEFORE they reach the downstream socket, in REVERSE chain order
	// (AMEND-A11 LIFO parity). Return Continue to forward; StopIteration to
	// stop the write (the bytes are NOT forwarded — upstream no-forward parity;
	// documented-unsupported-by-consumers at 28.x).
	OnWrite(buf *Buffer, endStream bool) Status
	// SetWriteFilterCallbacks injects the per-connection write callbacks. Called
	// once by the chain runtime at construction, before any OnWrite.
	SetWriteFilterCallbacks(cb WriteFilterCallbacks)
	// OnDestroy releases per-connection resources. For a filter implementing
	// BOTH ReadFilter and WriteFilter this is the SAME method (one instance);
	// the chain runtime calls OnDestroy exactly ONCE per filter instance.
	OnDestroy()
}

// WriteFilterCallbacks is the per-connection callback surface handed to each
// WriteFilter via SetWriteFilterCallbacks. Minimal at 28.1 (D-P2): zookeeper
// needs only the connection accessor. Upstream's WriteFilterCallbacks carries
// injectWriteDataToFilterChain + disableClose — both deferred under the
// ADR-0213/0221 API-revision allowance until a consumer needs them.
//
//nolint:revive // ADR-0221 reserves the network.WriteFilterCallbacks name.
type WriteFilterCallbacks interface {
	// Connection returns the per-connection accessor (the same concrete impl
	// the ReadFilterCallbacks Connection() returns).
	Connection() Connection
}

// NetworkFilterFactory parses + validates a network filter's typed_config Any
// once at boot (NewManager-build time) and returns a per-connection
// FilterInstanceFactory closure. Mirrors listenerfilter.ListenerFilterFactory
// + http.HTTPFilterFactory (ADR-0079 two-step factory).
//
//nolint:revive // ADR-0213 reserves the NetworkFilterFactory name for the boot-time factory surface.
type NetworkFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)

// FilterInstanceFactory allocates a fresh ReadFilter — or returns the shared
// TerminalFilter — per accepted connection (any NetworkFilter; the chain
// builder classifies). Per-config validation cost is paid once at boot.
type FilterInstanceFactory func() NetworkFilter

// FactoryCtx carries the PER-CHAIN build context a NetworkFilterFactory needs.
// Primitives + BaseDir only — the heavy boot singletons (cluster manager, stats
// registry, access-log sinks, HTTP-filter registry, drain manager, http client)
// are captured in the registration closures (internal/filter/network/builtins,
// §3.4), keeping this package free of cluster/stats/hcm imports.
type FactoryCtx struct {
	// BaseDir is the bootstrap config directory (for direct_response DataSource
	// Filename resolution relative to the config file; D-P26.1-2). echo ignores it.
	BaseDir string
	// Per-chain terminal-filter build context (26.2; consumed by the HCM
	// adapter — mirrors the retired manager.go listenerCtx). echo/direct_response
	// ignore these; tcp_proxy ignores all but is handed them uniformly.
	HasTLS             bool   // chain has a *stdtls.Config (hcm.ListenerCtx.HasTLS)
	AllowH2C           bool   // --allow-h2c (hcm.ListenerCtx.AllowH2C)
	ListenerPrincipal  string // per-chain leaf-cert principal (hcm.ListenerCtx.ListenerPrincipal)
	NodeServiceCluster string // bootstrap node.cluster (hcm.ListenerCtx.NodeServiceCluster)
}
