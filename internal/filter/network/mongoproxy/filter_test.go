package mongoproxy

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/accesslog"
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

// newTestFilter wires a *filter directly over a supplied *compiledConfig + a
// fresh roster (29.3 Task 6). It bypasses NewFactory so a test can set the
// fault-delay fields on the compiledConfig literal. The roster is attached to
// cfg.stats (armDelay increments delays_injected through it).
func newTestFilter(t *testing.T, cfg *compiledConfig) *filter {
	t.Helper()
	f, _ := newTestFilterWithStats(t, cfg)
	return f
}

func newTestFilterWithStats(t *testing.T, cfg *compiledConfig) (*filter, *mongoStats) {
	t.Helper()
	reg := stats.NewRegistry()
	ms := newMongoStats(reg, cfg.statPrefix)
	cfg.stats = ms
	return &filter{cfg: cfg, dec: newDecoder(cfg, ms)}, ms
}

// fakeReadCBContinue is a ReadFilterCallbacks fake that records ContinueReading
// by closing/sending on the continued channel (29.3 Task 6 — the async resume).
// It implements the FULL ReadFilterCallbacks surface (Connection, ContinueReading,
// DynamicMetadata, SetUpstreamCluster, plus Draining/CloseDirection — 29.3 Task 9).
// draining/closeDir drive the two new accessors; closed/closeType record the
// drain-close that cx_drain_close triggers (via the stub Connection below).
type fakeReadCBContinue struct {
	continued chan struct{}
	dm        *dynamicmetadata.Bucket
	draining  bool
	closeDir  network.CloseDirection
	closed    bool
	closeType network.CloseType
}

func (cb *fakeReadCBContinue) Connection() network.Connection {
	return &fakeRecordingConn{cb: cb}
}
func (cb *fakeReadCBContinue) ContinueReading()                         { close(cb.continued) }
func (cb *fakeReadCBContinue) DynamicMetadata() *dynamicmetadata.Bucket { return cb.dm }
func (cb *fakeReadCBContinue) SetUpstreamCluster(string)                {}
func (cb *fakeReadCBContinue) Draining() bool                           { return cb.draining }
func (cb *fakeReadCBContinue) CloseDirection() network.CloseDirection   { return cb.closeDir }

// fakeRecordingConn is the stub network.Connection the fake callbacks return; it
// records Close(closeType) on the owning callbacks so the drain-close test can
// assert cx_drain_close → Connection().Close(FlushWrite) (29.3 Task 9).
type fakeRecordingConn struct {
	cb *fakeReadCBContinue
}

func (c *fakeRecordingConn) Write([]byte, bool)             {}
func (c *fakeRecordingConn) Close(ct network.CloseType)     { c.cb.closed = true; c.cb.closeType = ct }
func (c *fakeRecordingConn) LocalAddr() net.Addr            { return nil }
func (c *fakeRecordingConn) RemoteAddr() net.Addr           { return nil }
func (c *fakeRecordingConn) RequestedServerName() string    { return "" }
func (c *fakeRecordingConn) DownstreamPrincipals() []string { return nil }

// newTestFilterWithCB wires a *filter over cfg with a fake ReadFilterCallbacks
// that records ContinueReading (the timer's async resume — 29.3 Task 6).
func newTestFilterWithCB(t *testing.T, cfg *compiledConfig) (*filter, *mongoStats, *fakeReadCBContinue) {
	t.Helper()
	f, ms := newTestFilterWithStats(t, cfg)
	cb := &fakeReadCBContinue{continued: make(chan struct{}), dm: dynamicmetadata.NewBucket()}
	f.SetReadFilterCallbacks(cb)
	return f, ms, cb
}

// TestFilter_DrainCloseOnEmptyListWhenDraining proves the cx_drain_close reply-
// completion path (29.3 Task 9, SPEC §3.4): a correlated reply empties the active-
// query list while the callbacks report Draining()==true → cx_drain_close +1 and
// Connection().Close(FlushWrite) (the reply is flushed first; D-S29.3-7).
func TestFilter_DrainCloseOnEmptyListWhenDraining(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", commands: map[string]bool{}}
	f, ms, cb := newTestFilterWithCB(t, cfg)
	cb.draining = true // the callbacks report Draining()==true
	// Send a query (appends to the active-query list), then a correlated reply
	// (responseTo=1 matches the request's requestID=1 → empties the list).
	rbuf := &network.Buffer{}
	rbuf.Append(msg(1, 2004, opQueryBody("db.c1", 0, simpleQuery())))
	_ = f.OnData(rbuf, false)
	wbuf := &network.Buffer{}
	wbuf.Append(respMsg(99 /*reqID*/, 1 /*responseTo*/, 1 /*OP_REPLY*/, opReplyBody(0, 0)))
	_ = f.OnWrite(wbuf, false)
	if v := ms.counters["cx_drain_close"].Load(); v != 1 {
		t.Errorf("cx_drain_close = %d, want 1 (list emptied while draining)", v)
	}
	if cb.closeType != network.FlushWrite || !cb.closed {
		t.Errorf("expected Connection().Close(FlushWrite), got closed=%v type=%v", cb.closed, cb.closeType)
	}
}

// TestFilter_NoDrainCloseWhenNotDraining proves cx_drain_close is gated on
// Draining(): an identical correlated-reply-empties-the-list flow with the
// callbacks NOT draining must neither increment nor close (29.3 Task 9).
func TestFilter_NoDrainCloseWhenNotDraining(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", commands: map[string]bool{}}
	f, ms, cb := newTestFilterWithCB(t, cfg) // cb.draining == false
	rbuf := &network.Buffer{}
	rbuf.Append(msg(1, 2004, opQueryBody("db.c1", 0, simpleQuery())))
	_ = f.OnData(rbuf, false)
	wbuf := &network.Buffer{}
	wbuf.Append(respMsg(99, 1, 1, opReplyBody(0, 0)))
	_ = f.OnWrite(wbuf, false)
	if v := ms.counters["cx_drain_close"].Load(); v != 0 {
		t.Errorf("cx_drain_close = %d, want 0 (not draining)", v)
	}
	if cb.closed {
		t.Error("must not close when not draining")
	}
}

// TestFilter_OnDestroyCloseDirectionKeyed proves the D-P4 close-direction-keyed
// cx_destroy_* increment (29.3 Task 10, SPEC §3.5): a non-empty active-query list
// at OnDestroy increments cx_destroy_local_with_active_rq or
// cx_destroy_remote_with_active_rq per the chain's recorded CloseDirection; an
// all-answered list (residual 0) increments NEITHER regardless of direction.
func TestFilter_OnDestroyCloseDirectionKeyed(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		dir                   network.CloseDirection
		answered              bool
		wantLocal, wantRemote uint64
	}{
		{"local+active", network.CloseDirectionLocal, false, 1, 0},
		{"remote+active", network.CloseDirectionRemote, false, 0, 1},
		{"all-answered", network.CloseDirectionLocal, true, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &compiledConfig{statPrefix: "m", commands: map[string]bool{}}
			f, ms, cb := newTestFilterWithCB(t, cfg)
			cb.closeDir = tc.dir
			rbuf := &network.Buffer{}
			rbuf.Append(msg(1, 2004, opQueryBody("db.c1", 0, simpleQuery())))
			_ = f.OnData(rbuf, false)
			if tc.answered {
				wbuf := &network.Buffer{}
				wbuf.Append(respMsg(99, 1, 1, opReplyBody(0, 0)))
				_ = f.OnWrite(wbuf, false)
			}
			f.OnDestroy()
			if got := ms.counters["cx_destroy_local_with_active_rq"].Load(); got != tc.wantLocal {
				t.Errorf("local = %d, want %d", got, tc.wantLocal)
			}
			if got := ms.counters["cx_destroy_remote_with_active_rq"].Load(); got != tc.wantRemote {
				t.Errorf("remote = %d, want %d", got, tc.wantRemote)
			}
		})
	}
}

func TestFilter_MayHaltReflectsDelayConfigured(t *testing.T) {
	fNo := newTestFilter(t, &compiledConfig{statPrefix: "m", commands: map[string]bool{}})
	if fNo.MayHalt() {
		t.Error("no delay configured → MayHalt() must be false (chain stays non-haltable)")
	}
	fYes := newTestFilter(t, &compiledConfig{statPrefix: "m", delayConfigured: true,
		fixedDelay: 10 * time.Millisecond, delayPercentNum: 100, commands: map[string]bool{}})
	if !fYes.MayHalt() {
		t.Error("delay configured → MayHalt() must be true")
	}
}

func TestFilter_OnDataArmsDelayAndStops(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", delayConfigured: true, fixedDelay: 20 * time.Millisecond,
		delayPercentNum: 100, commands: map[string]bool{}}
	f, ms, cb := newTestFilterWithCB(t, cfg) // a fake ReadFilterCallbacks recording ContinueReading
	buf := &network.Buffer{}
	q := msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	buf.Append(q)
	if got := f.OnData(buf, false); got != network.StopIteration {
		t.Fatalf("OnData = %v, want StopIteration (delay armed)", got)
	}
	if v := ms.counters["delays_injected"].Load(); v != 1 {
		t.Errorf("delays_injected = %d, want 1 (at ARM)", v)
	}
	// The timer fires after ~20ms → ContinueReading on the callbacks.
	select {
	case <-cb.continued:
	case <-time.After(2 * time.Second):
		t.Fatal("timer did not fire ContinueReading")
	}
	if f.dec.delayPending.Load() {
		t.Error("delayPending must be cleared after the timer fires")
	}
}

func TestFilter_OnDestroyCancelsPendingTimer(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", delayConfigured: true, fixedDelay: 10 * time.Second, // long
		delayPercentNum: 100, commands: map[string]bool{}}
	f, _, _ := newTestFilterWithCB(t, cfg)
	buf := &network.Buffer{}
	q := msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	buf.Append(q)
	_ = f.OnData(buf, false) // arms a 10s timer
	f.OnDestroy()            // must Stop it (no panic, no leak) — race-clean w/ a firing timer
}

// TestFilter_AccessLogGatedOffNoEmit asserts that with access_log unset the sink
// is nil and emitting is a no-op (no panic). The decoder still decodes; emit just
// does nothing. The cb's Connection() is nil here, exercising the nil-host guard.
func TestFilter_AccessLogGatedOffNoEmit(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", accessLog: "", commands: map[string]bool{}} // disabled
	f, _, _ := newTestFilterWithCB(t, cfg)
	if f.alSink != nil {
		t.Fatalf("access_log unset → alSink must be nil; got %v", f.alSink)
	}
	buf := &network.Buffer{}
	buf.Append(msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())))
	_ = f.OnData(buf, false) // must not panic; no sink → no emit
	// A direct emit call with non-empty lines is also a no-op when alSink is nil.
	f.emitAccessLog([]string{"OP_QUERY id=1 collection1"})
}

// TestFilter_AccessLogEmitsPerMessageBothDirections feeds a request then a reply
// through a real temp-file sink (constructed via NewAsyncFileSinkWithFormatter)
// and asserts at least one JSON line is written per direction after Close.
func TestFilter_AccessLogEmitsPerMessageBothDirections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mongo.log")
	reg := stats.NewRegistry()
	sink, err := accesslog.NewAsyncFileSinkWithFormatter(path, accessLogDroppedCounter(reg), accesslog.MongoFormat)
	if err != nil {
		t.Fatalf("sink: %v", err)
	}
	cfg := &compiledConfig{statPrefix: "m", accessLog: path, commands: map[string]bool{}}
	f, _, _ := newTestFilterWithCB(t, cfg)
	f.alSink = sink // inject the real sink (the factory does this for production)

	// Request direction: one OP_QUERY.
	rbuf := &network.Buffer{}
	rbuf.Append(msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())))
	_ = f.OnData(rbuf, false)

	// Response direction: one OP_REPLY correlating responseTo=1.
	wbuf := &network.Buffer{}
	wbuf.Append(respMsg(99, 1, 1, opReplyBody(0, 0)))
	_ = f.OnWrite(wbuf, false)

	if err := sink.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("want >=2 access-log lines (one per direction), got %d: %q", len(lines), string(data))
	}
	var sawQuery, sawReply bool
	for _, ln := range lines {
		if strings.Contains(ln, "OP_QUERY") {
			sawQuery = true
		}
		if strings.Contains(ln, "OP_REPLY") {
			sawReply = true
		}
		if !strings.Contains(ln, `"upstream_host"`) {
			t.Errorf("line missing upstream_host: %q", ln)
		}
	}
	if !sawQuery {
		t.Errorf("no request-direction (OP_QUERY) access-log line in %q", string(data))
	}
	if !sawReply {
		t.Errorf("no response-direction (OP_REPLY) access-log line in %q", string(data))
	}
}
