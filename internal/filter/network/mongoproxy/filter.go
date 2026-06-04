package mongoproxy

import (
	"fmt"

	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

const (
	// mongoMetadataNamespace is the dynamic-metadata namespace (parent §11.9 /
	// AMEND-B11). mongoMetadataKey is the single-Set wrapper key under which the
	// whole collection→ops StructValue is written (D-S29.2-3 — a unit-test-asserted
	// internal detail; differential-invisible, no cross-side surface).
	mongoMetadataNamespace = "envoy.filters.network.mongo_proxy"
	mongoMetadataKey       = "operations"
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
	f.emitDynamicMetadata()
	return network.Continue
}

// emitDynamicMetadata writes THIS request pass's collection→ops map to the
// per-connection dynamic-metadata Bucket as ONE StructValue (the §3.7 single-Set
// model — the next emitting pass overwrites it, giving per-pass clear for free).
// Gated by emit_dynamic_metadata; a no-op when the flag is off or the pass
// produced no insert/query (SPEC §3.7 "skip the Set if empty"). The Bucket is
// nil-receiver tolerant (ADR-0085).
func (f *filter) emitDynamicMetadata() {
	if !f.cfg.emitDynamicMetadata || len(f.dec.dynMeta) == 0 {
		return
	}
	fields := make(map[string]*structpb.Value, len(f.dec.dynMeta))
	for coll, ops := range f.dec.dynMeta {
		vals := make([]*structpb.Value, len(ops))
		for i, op := range ops {
			vals[i] = structpb.NewStringValue(op)
		}
		fields[coll] = structpb.NewListValue(&structpb.ListValue{Values: vals})
	}
	sv := structpb.NewStructValue(&structpb.Struct{Fields: fields})
	f.cb.DynamicMetadata().Set(mongoMetadataNamespace, mongoMetadataKey, sv)
}

// OnWrite feeds the response decoder the write-direction (upstream→downstream)
// bytes and ALWAYS returns Continue (R3 extended to the write side; upstream
// onWrite parity — never halts). Replaces the 29.1 no-op stub.
func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnWrite(buf.Bytes())
	return network.Continue
}

// SetReadFilterCallbacks / SetWriteFilterCallbacks store both (the both-directions
// dual injection — chain.go injects each exactly once).
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks)   { f.cb = cb }
func (f *filter) SetWriteFilterCallbacks(cb network.WriteFilterCallbacks) { f.wcb = cb }

// OnDestroy drains any residual active-query entries (Dec the gauge per entry so
// it returns to 0 at connection end — §3.4) then drops the per-connection
// decoder. Called exactly once per filter instance (the 28.1a dedupe); it runs
// strictly after both pumps join (the ADR-0221 happens-after edge), so the
// onDestroy lock is uncontended.
func (f *filter) OnDestroy() {
	if f.dec != nil {
		f.dec.onDestroy()
	}
	f.dec = nil
}
