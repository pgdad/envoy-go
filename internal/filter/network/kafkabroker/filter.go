package kafkabroker

import (
	"fmt"

	kafka_brokerv3 "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/stats"
)

// NewFactory returns the kafka_broker NetworkFilterFactory with the stats Registry
// closure-captured (the mongoproxy/zookeeperproxy precedent — network.FactoryCtx
// carries no stats registry). The factory parses + validates ONCE at boot
// (ADR-0079) and EAGERLY creates the 176-counter roster per distinct stat_prefix at
// parse (AMEND-K3, present-at-0 from boot). The returned FilterInstanceFactory
// allocates a fresh *filter per connection, all sharing the boot-parsed
// *compiledConfig (incl. the roster); each gets its OWN per-connection *decoder.
func NewFactory(reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		var msg kafka_brokerv3.KafkaBroker
		if tc != nil && len(tc.GetValue()) > 0 {
			if err := tc.UnmarshalTo(&msg); err != nil {
				return nil, fmt.Errorf("kafka_broker: invalid typed_config: %w", err)
			}
		}
		cfg, err := parseConfig(&msg)
		if err != nil {
			return nil, err
		}
		cfg.stats = newKafkaStats(reg, cfg.statPrefix) // EAGER roster (present-at-0 from boot)
		return func() network.NetworkFilter {
			return &filter{cfg: cfg, dec: newDecoder(cfg.stats)}
		}, nil
	}
}

// filter is the per-connection kafka_broker filter. It implements BOTH
// network.ReadFilter and network.WriteFilter (one instance, both directions —
// consumer #3 of the ADR-0221 WriteFilter seam; the mongoproxy/zookeeperproxy
// both-directions shape). It is a PURE COPYING SNIFFER (R1): it reads the chain
// buffers, feeds the decoders, and ALWAYS returns Continue — it NEVER drains or
// mutates a chain buffer and NEVER closes the connection. It has no halt path, so
// it deliberately does NOT implement HaltingReadFilter (MayHalt) — the chain takes
// the byte-identical non-haltable path. It implements WriteFilter even though
// OnWrite never mutates the write buffer, because a read-side sniffer must qualify
// the chain for the 28.1b post-handoff read seam: read filters on UNWRAPPED chains
// only observe pre-handoff bytes, and the seam restores full-lifetime read
// observation ONLY for chains with ≥1 write filter
// (reference_network_chain_terminal_handoff_ends_ondata).
type filter struct {
	network.Marker
	cfg *compiledConfig // shared, boot-parsed (incl. the eager roster)
	dec *decoder        // per-connection (private readBuf/writeBuf + chainConsumed + correlation map)
	cb  network.ReadFilterCallbacks
	wcb network.WriteFilterCallbacks
}

// OnNewConnection is a no-op Continue: an OnNewConnection StopIteration would set
// the chain's sticky connHalted flag and block all OnData
// (reference_network_read_filter_onnewconnection_halts).
func (f *filter) OnNewConnection() network.Status { return network.Continue }

// OnData feeds the request decoder the chain-buffer's NEW bytes (the chainConsumed
// high-water mark against TotalAppended — the mongoproxy mechanism) and ALWAYS
// returns Continue. It NEVER drains the chain buffer, never closes, never halts (R1).
func (f *filter) OnData(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnData(buf.Bytes(), buf.TotalAppended())
	return network.Continue
}

// OnWrite feeds the response decoder the write-direction (upstream→downstream)
// bytes and ALWAYS returns Continue (R1 extended to the write side; never halts,
// never mutates). The write seam is incremental (writeconn.go allocates a fresh
// *Buffer per Write), so the decoder reassembles internally without a high-water
// mark — see decodeOnWrite (ADR-0225).
func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnWrite(buf.Bytes())
	return network.Continue
}

// SetReadFilterCallbacks / SetWriteFilterCallbacks store both (the both-directions
// dual injection — chain.go injects each exactly once). The pure sniffer never
// uses them (it never closes/continues/reads dynamic metadata), but storing them
// mirrors the mongoproxy precedent and keeps the surface uniform.
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks)   { f.cb = cb }
func (f *filter) SetWriteFilterCallbacks(cb network.WriteFilterCallbacks) { f.wcb = cb }

// OnDestroy drops the per-connection decoder. Called exactly once per filter
// instance (the 28.1a dedupe); it runs strictly after both pumps join (the
// ADR-0221 happens-after edge). A pure sniffer holds no gauge/timer to drain — it
// simply releases the decoder.
func (f *filter) OnDestroy() { f.dec = nil }
