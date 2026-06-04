package mongoproxy

import (
	"testing"

	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

func mustAny(t *testing.T, m *mongo_proxyv3.MongoProxy) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func buildFilter(t *testing.T, m *mongo_proxyv3.MongoProxy) (*filter, *stats.Registry) {
	t.Helper()
	reg := stats.NewRegistry()
	instFactory, err := NewFactory(reg)(mustAny(t, m), network.FactoryCtx{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	return instFactory().(*filter), reg
}

func TestFactory_BootRejectMissingStatPrefix(t *testing.T) {
	reg := stats.NewRegistry()
	_, err := NewFactory(reg)(mustAny(t, &mongo_proxyv3.MongoProxy{}), network.FactoryCtx{})
	if err == nil || err.Error() != errStatPrefixRequired {
		t.Fatalf("err = %v, want %q", err, errStatPrefixRequired)
	}
}

func TestFilter_ImplementsBothDirections(t *testing.T) {
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	if _, ok := any(f).(network.ReadFilter); !ok {
		t.Error("filter must be a ReadFilter")
	}
	if _, ok := any(f).(network.WriteFilter); !ok {
		t.Error("filter must be a WriteFilter")
	}
}

func TestFilter_OnDataFeedsDecoder(t *testing.T) {
	f, reg := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	buf.Append(msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())))
	if f.OnData(&buf, false) != network.Continue {
		t.Error("OnData must return Continue")
	}
	if reg.NewCounterIfAbsent("mongo.p.op_query").Load() != 1 {
		t.Errorf("OnData did not feed the decoder (op_query != 1)")
	}
}

func TestFilter_OnDataNeverDrainsChainBuffer(t *testing.T) {
	// R3: the chain Buffer is observational — never drained/mutated.
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	raw := msg(1, 2004, opQueryBody("db.c", 0, simpleQuery()))
	buf.Append(raw)
	before := buf.Len()
	f.OnData(&buf, false)
	if buf.Len() != before {
		t.Errorf("OnData drained the chain buffer: %d → %d", before, buf.Len())
	}
}

func TestFilter_OnWriteFeedsResponseDecoder(t *testing.T) {
	f, reg := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	buf.Append(respMsg(7, 0, 1, opReplyBody(0, 0))) // an empty OP_REPLY on the write side
	if f.OnWrite(&buf, false) != network.Continue {
		t.Error("OnWrite must return Continue")
	}
	if reg.NewCounterIfAbsent("mongo.p.op_reply").Load() != 1 {
		t.Errorf("OnWrite must feed the response decoder (op_reply != 1)")
	}
}

func TestFilter_OnWriteNeverDrainsChainBuffer(t *testing.T) {
	// R3 extended to the write side: the write chain Buffer is observational.
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	buf.Append(respMsg(7, 0, 1, opReplyBody(0, 0)))
	before := buf.Len()
	f.OnWrite(&buf, false)
	if buf.Len() != before {
		t.Errorf("OnWrite drained the write chain buffer: %d → %d", before, buf.Len())
	}
}

func TestFilter_OnDestroyDrainsGauge(t *testing.T) {
	f, reg := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	buf.Append(msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())))
	f.OnData(&buf, false)
	if reg.NewGaugeIfAbsent("mongo.p.op_query_active").Load() != 1 {
		t.Fatalf("setup: gauge != 1 after one query")
	}
	f.OnDestroy()
	if reg.NewGaugeIfAbsent("mongo.p.op_query_active").Load() != 0 {
		t.Errorf("OnDestroy must drain the gauge to 0")
	}
	if f.dec != nil {
		t.Errorf("OnDestroy must still release the decoder")
	}
}

func TestFilter_OnDestroyReleasesDecoder(t *testing.T) {
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	f.OnDestroy()
	if f.dec != nil {
		t.Error("OnDestroy must release the decoder")
	}
}

type fakeReadCallbacks struct {
	network.ReadFilterCallbacks
	dm *dynamicmetadata.Bucket
}

func (cb *fakeReadCallbacks) DynamicMetadata() *dynamicmetadata.Bucket { return cb.dm }

// driveOnData appends each frame to the connection's ONE shared chain Buffer
// (the per-connection contract: a single Buffer with a monotonic TotalAppended
// drives the decoder's chainConsumed high-water mark — buffer.go §total) and
// runs one OnData pass. Reusing buf across calls models successive reads on the
// same connection (a fresh Buffer per call would reset TotalAppended below
// chainConsumed and feed nothing).
func driveOnData(f *filter, buf *network.Buffer, frames ...[]byte) {
	for _, fr := range frames {
		buf.Append(fr)
	}
	f.OnData(buf, false)
}

func TestEmitDynamicMetadata_CollectionToOps(t *testing.T) {
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p", EmitDynamicMetadata: true})
	cb := &fakeReadCallbacks{dm: dynamicmetadata.NewBucket()}
	f.SetReadFilterCallbacks(cb)
	// One pass: an OP_QUERY on collection1 + an OP_INSERT on collection2.
	driveOnData(f, &network.Buffer{},
		msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())),
		msg(2, 2002, append(append(leI32(0), cstr("db.collection2")...), simpleQuery()...)),
	)
	v, ok := cb.dm.Get("envoy.filters.network.mongo_proxy", "operations")
	if !ok {
		t.Fatalf("dynamic metadata not emitted")
	}
	fields := v.GetStructValue().GetFields()
	if got := fields["collection1"].GetListValue().GetValues(); len(got) != 1 || got[0].GetStringValue() != "query" {
		t.Errorf("collection1 ops = %v, want [query]", got)
	}
	if got := fields["collection2"].GetListValue().GetValues(); len(got) != 1 || got[0].GetStringValue() != "insert" {
		t.Errorf("collection2 ops = %v, want [insert]", got)
	}
}

func TestEmitDynamicMetadata_PerPassOverwriteClear(t *testing.T) {
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p", EmitDynamicMetadata: true})
	cb := &fakeReadCallbacks{dm: dynamicmetadata.NewBucket()}
	f.SetReadFilterCallbacks(cb)
	buf := &network.Buffer{}
	driveOnData(f, buf, msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())))
	driveOnData(f, buf, msg(2, 2004, opQueryBody("db.collection2", 0, simpleQuery())))
	v, _ := cb.dm.Get("envoy.filters.network.mongo_proxy", "operations")
	fields := v.GetStructValue().GetFields()
	if _, present := fields["collection1"]; present {
		t.Errorf("per-pass clear failed: collection1 from pass 1 still present in pass 2")
	}
	if _, present := fields["collection2"]; !present {
		t.Errorf("pass-2 collection2 missing")
	}
}

func TestEmitDynamicMetadata_GatedOff(t *testing.T) {
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"}) // emit flag default false
	cb := &fakeReadCallbacks{dm: dynamicmetadata.NewBucket()}
	f.SetReadFilterCallbacks(cb)
	driveOnData(f, &network.Buffer{}, msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())))
	if _, ok := cb.dm.Get("envoy.filters.network.mongo_proxy", "operations"); ok {
		t.Errorf("no metadata may be emitted when emit_dynamic_metadata is false")
	}
}
