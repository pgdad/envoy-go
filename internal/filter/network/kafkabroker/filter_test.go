package kafkabroker

import (
	"testing"

	kafka_brokerv3 "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/stats"
)

// mustAny marshals a KafkaBroker message into an Any (the typed_config the factory
// unmarshals). Mirrors mongoproxy/filter_test.go's helper.
func mustAny(t *testing.T, m *kafka_brokerv3.KafkaBroker) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// buildFilter constructs a per-connection *filter via the real NewFactory path
// (boot-parse + eager roster + per-connection instance), returning it together
// with the registry so tests can read counters. Mirrors the mongoproxy precedent.
func buildFilter(t *testing.T, m *kafka_brokerv3.KafkaBroker) (*filter, *stats.Registry) {
	t.Helper()
	reg := stats.NewRegistry()
	instFactory, err := NewFactory(reg)(mustAny(t, m), network.FactoryCtx{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	return instFactory().(*filter), reg
}

// TestFactory_BothDirections proves a factory-built per-connection instance
// satisfies BOTH network.ReadFilter and network.WriteFilter (the both-directions
// sniffer shape — kafka is consumer #3 of the ADR-0221 WriteFilter seam; a filter
// must implement WriteFilter to get the 28.1b post-handoff read seam).
func TestFactory_BothDirections(t *testing.T) {
	f, _ := buildFilter(t, &kafka_brokerv3.KafkaBroker{StatPrefix: "k"})
	var _ network.ReadFilter = f
	var _ network.WriteFilter = f
	if _, ok := any(f).(network.ReadFilter); !ok {
		t.Error("filter must be a ReadFilter")
	}
	if _, ok := any(f).(network.WriteFilter); !ok {
		t.Error("filter must be a WriteFilter")
	}
}

// TestFactory_RejectsBadConfig proves the factory boot-rejects a config missing
// stat_prefix (the PARSE-REJECT arm flows through parseConfig — ADR-0080 byte-stable).
func TestFactory_RejectsBadConfig(t *testing.T) {
	reg := stats.NewRegistry()
	_, err := NewFactory(reg)(mustAny(t, &kafka_brokerv3.KafkaBroker{}), network.FactoryCtx{})
	if err == nil {
		t.Fatal("factory must reject a config missing stat_prefix")
	}
	if err.Error() != errStatPrefixRequired {
		t.Fatalf("err = %q, want %q", err.Error(), errStatPrefixRequired)
	}
}

// TestFactory_NilTypedConfigRejected proves a nil typed_config (empty proto) is
// boot-rejected the same way a missing stat_prefix is (the factory tolerates a nil
// Any then hands the zero-value proto to parseConfig, which rejects it).
func TestFactory_NilTypedConfigRejected(t *testing.T) {
	reg := stats.NewRegistry()
	_, err := NewFactory(reg)(nil, network.FactoryCtx{})
	if err == nil || err.Error() != errStatPrefixRequired {
		t.Fatalf("err = %v, want %q", err, errStatPrefixRequired)
	}
}

// TestOnNewConnection_Continue proves OnNewConnection is a no-op Continue (an
// OnNewConnection StopIteration would set the chain's sticky connHalted flag and
// block all OnData — reference_network_read_filter_onnewconnection_halts).
func TestOnNewConnection_Continue(t *testing.T) {
	f, _ := buildFilter(t, &kafka_brokerv3.KafkaBroker{StatPrefix: "k"})
	if got := f.OnNewConnection(); got != network.Continue {
		t.Errorf("OnNewConnection = %v, want Continue", got)
	}
}

// TestOnData_FeedsDecoder_NeverDrains proves R1 for the request direction: OnData
// feeds the request decoder (a matching per-key counter increments), NEVER drains
// the chain buffer (its length is unchanged), and ALWAYS returns Continue.
func TestOnData_FeedsDecoder_NeverDrains(t *testing.T) {
	f, reg := buildFilter(t, &kafka_brokerv3.KafkaBroker{StatPrefix: "k"})
	var buf network.Buffer
	// ApiVersions(18) v0 non-flexible request, correlation_id 7.
	buf.Append(buildRequest(18, 0, 7, "c", false))
	before := buf.Len()

	if got := f.OnData(&buf, false); got != network.Continue {
		t.Errorf("OnData = %v, want Continue", got)
	}
	if reg.NewCounterIfAbsent("kafka.k.request.api_versions_request").Load() != 1 {
		t.Errorf("OnData did not feed the request decoder (api_versions_request != 1)")
	}
	if buf.Len() != before {
		t.Errorf("OnData drained the chain buffer: %d -> %d (R1: never drain)", before, buf.Len())
	}
}

// TestOnWrite_FeedsResponseDecoder_AlwaysContinue proves R1 for the response
// direction: OnWrite feeds the response decoder (the correlated per-key response
// counter increments), NEVER drains the write chain buffer, and ALWAYS Continues.
func TestOnWrite_FeedsResponseDecoder_AlwaysContinue(t *testing.T) {
	f, reg := buildFilter(t, &kafka_brokerv3.KafkaBroker{StatPrefix: "k"})
	// Request first so the correlation map carries corr 7 -> (18, 0).
	var rbuf network.Buffer
	rbuf.Append(buildRequest(18, 0, 7, "c", false))
	f.OnData(&rbuf, false)

	var wbuf network.Buffer
	wbuf.Append(buildResponse(7)) // correlated response for corr 7
	before := wbuf.Len()

	if got := f.OnWrite(&wbuf, false); got != network.Continue {
		t.Errorf("OnWrite = %v, want Continue", got)
	}
	if reg.NewCounterIfAbsent("kafka.k.response.api_versions_response").Load() != 1 {
		t.Errorf("OnWrite did not feed the response decoder (api_versions_response != 1)")
	}
	if wbuf.Len() != before {
		t.Errorf("OnWrite drained the write chain buffer: %d -> %d (R1: never drain)", before, wbuf.Len())
	}
}

// TestSetCallbacks_NoPanic proves both callback injectors are safe to call (the
// chain runtime injects each exactly once; kafka stores them but never closes).
func TestSetCallbacks_NoPanic(t *testing.T) {
	f, _ := buildFilter(t, &kafka_brokerv3.KafkaBroker{StatPrefix: "k"})
	f.SetReadFilterCallbacks(nil)
	f.SetWriteFilterCallbacks(nil)
}

// TestOnDestroy_NoPanic proves OnDestroy is safe to call (a pure sniffer holds no
// per-connection resource needing release beyond dropping the decoder; it must not
// panic and must be idempotent in the sense that the filter no longer decodes).
func TestOnDestroy_NoPanic(t *testing.T) {
	f, _ := buildFilter(t, &kafka_brokerv3.KafkaBroker{StatPrefix: "k"})
	f.OnDestroy() // must not panic
}
