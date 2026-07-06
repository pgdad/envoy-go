package zookeeperproxy

import (
	"fmt"

	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/stats"
)

// TypeURL is the canonical Any type URL for zookeeper_proxy's typed_config.
// Derived via proto.MessageName (NEVER a hand-typed docs string —
// reference_network_filter_typeurl_extensions; the rbac.go:38 precedent).
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&zookeeper_proxyv3.ZooKeeperProxy{}))

// NewFactory returns the zookeeperproxy NetworkFilterFactory with the stats
// Registry closure-captured (the rbac NewFactory(reg) / D-26.3-3 precedent —
// network.FactoryCtx carries no stats registry). The factory parses + validates
// ONCE at boot (ADR-0079 two-step factory) and EAGERLY creates the 201-counter
// roster per distinct stat_prefix (D-P5 creation parity). The returned
// FilterInstanceFactory allocates a fresh per-connection *filter; all instances
// share the boot-parsed *compiledConfig (counter increments are atomic).
func NewFactory(reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		var msg zookeeper_proxyv3.ZooKeeperProxy
		if tc != nil && len(tc.GetValue()) > 0 {
			if err := tc.UnmarshalTo(&msg); err != nil {
				return nil, fmt.Errorf("zookeeper_proxy: invalid typed_config: %w", err)
			}
		}
		cfg, err := parseConfig(&msg)
		if err != nil {
			return nil, err
		}
		cfg.stats = newRosterStats(reg, cfg.statPrefix)
		return func() network.NetworkFilter {
			return &filter{cfg: cfg, decoder: newDecoder(cfg, cfg.stats)}
		}, nil
	}
}

// filter is the per-connection zookeeper_proxy filter. It implements BOTH
// network.ReadFilter and network.WriteFilter (one instance, both directions —
// the first consumer of the ADR-0221 seam; upstream addFilter parity).
type filter struct {
	network.Marker
	cfg     *compiledConfig // shared, boot-parsed (incl. the roster counters)
	decoder *decoder        // per-connection (reassembly bufs + correlation structures + mu)
	cb      network.ReadFilterCallbacks
	wcb     network.WriteFilterCallbacks
}

// OnNewConnection is a no-op Continue: an OnNewConnection StopIteration would
// set the chain's sticky connHalted flag and block all OnData
// (reference_network_read_filter_onnewconnection_halts).
func (f *filter) OnNewConnection() network.Status { return network.Continue }

// OnData feeds the decoder with the chain-buffer contents + the monotonic
// TotalAppended counter (the 28.1b §3.3 re-based feed — correct under runtime
// drains) and ALWAYS returns Continue. It NEVER drains the chain buffer, never
// closes, never halts (AMEND-A8 unconditional passthrough; R3).
func (f *filter) OnData(buf *network.Buffer, _ bool) network.Status {
	f.decoder.decodeOnData(buf.Bytes(), buf.TotalAppended())
	return network.Continue
}

// OnWrite feeds the decoder's write-side reassembly buffer with the
// upstream→downstream bytes and ALWAYS returns Continue (AMEND-A8
// unconditional passthrough; R3 — the filter never mutates the chain Buffer,
// never halts the write, never closes). Each OnWrite call sees a FRESH
// per-Write *Buffer (writeChainConn.Write allocates one per call), so the
// bytes are appended directly — no TotalAppended high-water mark is needed on
// the write side (every byte arrives exactly once by construction; SPEC §3.2
// item 1 / ADR-0223). Runs on goroutine B (the upstream→downstream pump);
// the §3.6 decoder mutex makes the correlation-map accesses race-free against
// goroutine A's request decode.
func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
	f.decoder.decodeOnWrite(buf.Bytes())
	return network.Continue
}

// SetReadFilterCallbacks stores the per-connection read callbacks.
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks) { f.cb = cb }

// SetWriteFilterCallbacks stores the per-connection write callbacks
// (the both-directions dual injection — D-P2/§3.3).
func (f *filter) SetWriteFilterCallbacks(cb network.WriteFilterCallbacks) { f.wcb = cb }

// OnDestroy drops the per-connection decoder (the correlation structures +
// reassembly buffer die with the connection). The chain runtime calls this
// exactly once per filter instance (the §3.3 dedupe).
func (f *filter) OnDestroy() { f.decoder = nil }
