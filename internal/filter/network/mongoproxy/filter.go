package mongoproxy

import (
	"fmt"

	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

// NewFactory returns the mongoproxy NetworkFilterFactory with the stats Registry
// closure-captured (the zookeeperproxy.go:26 precedent — network.FactoryCtx
// carries no stats registry). The factory parses + validates ONCE at boot
// (ADR-0079) and EAGERLY creates the 23-stat roster per distinct stat_prefix at
// parse (D-P1). The returned FilterInstanceFactory allocates a fresh *filter per
// connection, all sharing the boot-parsed *compiledConfig (incl. the roster).
func NewFactory(reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		var msg mongo_proxyv3.MongoProxy
		if tc != nil && len(tc.GetValue()) > 0 {
			if err := tc.UnmarshalTo(&msg); err != nil {
				return nil, fmt.Errorf("mongo_proxy: invalid typed_config: %w", err)
			}
		}
		cfg, err := parseConfig(&msg)
		if err != nil {
			return nil, err
		}
		cfg.stats = newMongoStats(reg, cfg.statPrefix)
		return func() network.NetworkFilter {
			return &filter{cfg: cfg, dec: newDecoder(cfg, cfg.stats)}
		}, nil
	}
}

// filter is the per-connection mongo_proxy filter. It implements BOTH
// network.ReadFilter and network.WriteFilter (one instance, both directions —
// consumer #2 of the ADR-0221 seam; the zookeeperproxy both-directions shape).
type filter struct {
	network.Marker
	cfg *compiledConfig // shared, boot-parsed (incl. the roster)
	dec *decoder        // per-connection (private readBuf + sniffing + chainConsumed + active-query list)
	cb  network.ReadFilterCallbacks
	wcb network.WriteFilterCallbacks
}

// OnNewConnection is a no-op Continue: an OnNewConnection StopIteration would set
// the chain's sticky connHalted flag and block all OnData
// (reference_network_read_filter_onnewconnection_halts).
func (f *filter) OnNewConnection() network.Status { return network.Continue }

// OnData feeds the decoder the chain-buffer's NEW bytes (the chainConsumed
// high-water mark against TotalAppended — D-S29.1-4) and ALWAYS returns Continue.
// It NEVER drains the chain buffer, never closes, never halts (R3; at 29.1
// mongoproxy has no halt path — fault delay is 29.3).
func (f *filter) OnData(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnData(buf.Bytes(), buf.TotalAppended())
	return network.Continue
}

// OnWrite is a PURE NO-OP at 29.1 (the 28.1 zookeeper OnWrite-stub pin verbatim):
// it does NOT buffer write-direction bytes (no response decoder to drain them →
// unbounded growth). The write-side private buffer is created WITH the response
// decoder at 29.2. The stub exists so the filter satisfies WriteFilter end-to-end
// (the 0049 traffic DOES flow through writeChainConn → OnWrite).
func (f *filter) OnWrite(_ *network.Buffer, _ bool) network.Status { return network.Continue }

// SetReadFilterCallbacks / SetWriteFilterCallbacks store both (the both-directions
// dual injection — chain.go injects each exactly once).
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks)   { f.cb = cb }
func (f *filter) SetWriteFilterCallbacks(cb network.WriteFilterCallbacks) { f.wcb = cb }

// OnDestroy drops the per-connection decoder + its active-query list (they die
// with the connection). Called exactly once per filter instance (the 28.1a dedupe).
func (f *filter) OnDestroy() { f.dec = nil }
