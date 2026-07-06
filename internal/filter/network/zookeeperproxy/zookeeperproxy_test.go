package zookeeperproxy

import (
	"strings"
	"testing"

	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/stats"
)

// mustAny marshals a proto message into *anypb.Any (mirrors rbac_test.go:41 shape).
func mustAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// newTestFilter builds a *filter via NewFactory + a good typed_config.
func newTestFilter(t *testing.T) *filter {
	t.Helper()
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	good := mustAny(t, &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk"})
	instFactory, err := factory(good, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	f, ok := instFactory().(*filter)
	if !ok {
		t.Fatal("instFactory() did not return *filter")
	}
	return f
}

// TestTypeURLViaProtoMessageName pins the TypeURL — derived via proto.MessageName
// (never hand-typed; reference_network_filter_typeurl_extensions; rbac.go:38 precedent).
func TestTypeURLViaProtoMessageName(t *testing.T) {
	want := "type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}

// TestNewFactoryParseAndReject: missing stat_prefix → reject; valid config →
// 201 counters created eagerly; two instances share cfg but have independent decoders.
func TestNewFactoryParseAndReject(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)

	// Reject: missing stat_prefix.
	bad := mustAny(t, &zookeeper_proxyv3.ZooKeeperProxy{})
	if _, err := factory(bad, network.FactoryCtx{}); err == nil || !strings.Contains(err.Error(), errStatPrefixRequired) {
		t.Fatalf("factory(no stat_prefix) err = %v, want %q", err, errStatPrefixRequired)
	}

	// Accept: the 201 counters exist at 0 right after parse (eager creation — D-P5).
	good := mustAny(t, &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk"})
	instFactory, err := factory(good, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if got := reg.NewCounterIfAbsent("zk.zookeeper.getdata_resp_fast").Load(); got != 0 {
		t.Fatalf("response-side counter not pre-created at 0 (eager roster)")
	}

	// Two instances share config but have independent decoders.
	f1 := instFactory().(*filter)
	f2 := instFactory().(*filter)
	if f1.cfg != f2.cfg {
		t.Fatal("instances must share the boot-parsed compiledConfig")
	}
	if f1.decoder == f2.decoder {
		t.Fatal("instances must have per-connection decoders")
	}
}

// TestNewFactoryMalformedAny: malformed value bytes → "zookeeper_proxy: invalid typed_config: …".
func TestNewFactoryMalformedAny(t *testing.T) {
	factory := NewFactory(stats.NewRegistry())
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff}}
	if _, err := factory(bad, network.FactoryCtx{}); err == nil ||
		!strings.HasPrefix(err.Error(), "zookeeper_proxy: invalid typed_config: ") {
		t.Fatalf("factory(malformed) err = %v, want invalid-typed_config prefix", err)
	}
}

// Compile-time interface assertions: the filter implements BOTH directions
// (the first both-directions production filter — ADR-0221).
var (
	_ network.ReadFilter  = (*filter)(nil)
	_ network.WriteFilter = (*filter)(nil)
)

// TestFilterOnDataPassthroughNeverDrains: OnData ALWAYS returns Continue and NEVER
// drains the chain buffer (R3 unconditional passthrough; AMEND-A8).
func TestFilterOnDataPassthroughNeverDrains(t *testing.T) {
	f := newTestFilter(t)
	buf := &network.Buffer{}
	buf.Append(dataFrame(1, opGetData, padTo(opGetData)))
	before := buf.Len()
	if got := f.OnData(buf, false); got != network.Continue {
		t.Fatalf("OnData = %v, want Continue (R3 unconditional passthrough)", got)
	}
	if buf.Len() != before {
		t.Fatalf("OnData drained the chain buffer (len %d → %d) — FORBIDDEN (R3)", before, buf.Len())
	}
}

// TestFilterMultiReadNoDoubleCount: undrained chain-buffer accumulation does not
// double-count (the decoder feeds buf.Bytes() — the FULL buffer — each call;
// the high-water mark skips already-consumed bytes; D-S28.1-3).
func TestFilterMultiReadNoDoubleCount(t *testing.T) {
	f := newTestFilter(t)
	buf := &network.Buffer{}
	buf.Append(dataFrame(1, opGetData, padTo(opGetData)))
	f.OnData(buf, false)
	buf.Append(dataFrame(2, opExists, padTo(opExists))) // chain buffer accumulates
	f.OnData(buf, false)
	rs := f.cfg.stats
	if rs.counters["getdata_rq"].Load() != 1 || rs.counters["exists_rq"].Load() != 1 {
		t.Fatalf("counters getdata=%d exists=%d, want 1/1 (no double-count across reads)",
			rs.counters["getdata_rq"].Load(), rs.counters["exists_rq"].Load())
	}
}

// TestFilterOnWriteFeedsDecoder: OnWrite feeds the decoder's write side (the 28.2
// response decoder — ADR-0223) and ALWAYS returns Continue (R3 extended to the
// write side — SPEC §3.2 item 5).
func TestFilterOnWriteFeedsDecoder(t *testing.T) {
	f := newTestFilter(t)
	// Pre-load a pending request so the response correlates.
	reqBuf := &network.Buffer{}
	reqBuf.Append(dataFrame(1, opGetData, padTo(opGetData)))
	f.OnData(reqBuf, false)

	respBuf := &network.Buffer{}
	resp := stdRespFrame(1, 100, 0)
	respBuf.Append(resp)
	before := respBuf.Len()
	if got := f.OnWrite(respBuf, false); got != network.Continue {
		t.Fatalf("OnWrite = %v, want Continue (always — R3)", got)
	}
	if respBuf.Len() != before {
		t.Fatalf("OnWrite drained/mutated the chain buffer (len %d -> %d) — FORBIDDEN (R3)", before, respBuf.Len())
	}
	rs := f.cfg.stats
	if got := rs.counters["getdata_resp"].Load(); got != 1 {
		t.Fatalf("getdata_resp = %d, want 1 (OnWrite must feed the response decoder)", got)
	}
	if got := rs.counters["response_bytes"].Load(); got != uint64(len(resp)) {
		t.Fatalf("response_bytes = %d, want %d", got, len(resp))
	}
}

// TestFilterOnWritePartialFramesAcrossCalls: response bytes split across multiple
// OnWrite calls (each a FRESH per-Write Buffer — writeconn.go:35) reassemble in
// the decoder's writeBuf (SPEC §3.2 item 1: no write-side TotalAppended; each
// OnWrite call's bytes are appended directly).
func TestFilterOnWritePartialFramesAcrossCalls(t *testing.T) {
	f := newTestFilter(t)
	reqBuf := &network.Buffer{}
	reqBuf.Append(dataFrame(1, opGetData, padTo(opGetData)))
	f.OnData(reqBuf, false)

	resp := stdRespFrame(1, 100, 0)
	cut := len(resp) / 2
	for _, half := range [][]byte{resp[:cut], resp[cut:]} {
		b := &network.Buffer{} // fresh per-Write Buffer, exactly as writeChainConn.Write does
		b.Append(half)
		if got := f.OnWrite(b, false); got != network.Continue {
			t.Fatalf("OnWrite = %v, want Continue", got)
		}
	}
	if got := f.cfg.stats.counters["getdata_resp"].Load(); got != 1 {
		t.Fatalf("getdata_resp = %d, want 1 (reassembled across OnWrite calls)", got)
	}
}

// TestFilterOnNewConnectionContinue: OnNewConnection is a no-op Continue (the sticky-halt
// constraint — reference_network_read_filter_onnewconnection_halts).
func TestFilterOnNewConnectionContinue(t *testing.T) {
	f := newTestFilter(t)
	if got := f.OnNewConnection(); got != network.Continue {
		t.Fatalf("OnNewConnection = %v, want Continue (sticky-halt constraint)", got)
	}
}

// TestFilterCallbacksAndDestroy: both callback injections are stored verbatim;
// OnDestroy drops the per-connection decoder.
func TestFilterCallbacksAndDestroy(t *testing.T) {
	f := newTestFilter(t)
	f.SetReadFilterCallbacks(nil) // stored verbatim; nil ok in unit test
	f.SetWriteFilterCallbacks(nil)
	f.OnDestroy()
	if f.decoder != nil {
		t.Fatal("OnDestroy must drop the per-connection decoder")
	}
}
