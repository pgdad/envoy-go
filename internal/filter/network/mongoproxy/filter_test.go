package mongoproxy

import (
	"testing"

	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

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

func TestFilter_OnWriteIsNoOp(t *testing.T) {
	f, reg := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	buf.Append(msg(1, 1, []byte{1, 2, 3})) // an OP_REPLY on the write side
	if f.OnWrite(&buf, false) != network.Continue {
		t.Error("OnWrite must return Continue")
	}
	// No write-side decode at 29.1: op_reply stays 0.
	if reg.NewCounterIfAbsent("mongo.p.op_reply").Load() != 0 {
		t.Errorf("OnWrite must be a pure no-op at 29.1 (op_reply must stay 0)")
	}
}

func TestFilter_OnDestroyReleasesDecoder(t *testing.T) {
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	f.OnDestroy()
	if f.dec != nil {
		t.Error("OnDestroy must release the decoder")
	}
}
