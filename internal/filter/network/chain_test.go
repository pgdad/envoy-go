package network

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
)

// scriptedConn returns a net.Conn whose first Read yields live then io.EOF,
// capturing writes. net.Pipe is deliberately avoided: it lacks CloseWrite and
// would not exercise the live-tail read after the buffered prefix.
func scriptedConn(live []byte) net.Conn { return &scriptConn{live: live} }

type scriptConn struct {
	live   []byte
	read   bool
	writes []byte
}

func (c *scriptConn) Read(b []byte) (int, error) {
	if c.read {
		return 0, io.EOF
	}
	c.read = true
	n := copy(b, c.live)
	return n, nil
}
func (c *scriptConn) Write(b []byte) (int, error) {
	c.writes = append(c.writes, b...)
	return len(b), nil
}
func (c *scriptConn) Close() error                       { return nil }
func (c *scriptConn) LocalAddr() net.Addr                { return nil }
func (c *scriptConn) RemoteAddr() net.Addr               { return nil }
func (c *scriptConn) SetDeadline(_ time.Time) error      { return nil }
func (c *scriptConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *scriptConn) SetWriteDeadline(_ time.Time) error { return nil }

// recordTerminal captures the bytes Handle reads off the handed-over conn.
type recordTerminal struct {
	Marker
	got []byte
}

func (rt *recordTerminal) Handle(_ context.Context, c net.Conn) {
	rt.got, _ = io.ReadAll(c)
}

// alwaysContinue drains NOTHING and Continues — the synthetic read filter that
// hands the buffered prefix to the terminal (R-M).
type alwaysContinue struct {
	Marker
	cb ReadFilterCallbacks
}

func (f *alwaysContinue) OnNewConnection() Status                       { return Continue }
func (f *alwaysContinue) OnData(_ *Buffer, _ bool) Status               { return Continue }
func (f *alwaysContinue) SetReadFilterCallbacks(cb ReadFilterCallbacks) { f.cb = cb }
func (f *alwaysContinue) OnDestroy()                                    {}

func TestPureTerminalImmediateHandoff(t *testing.T) {
	term := &recordTerminal{}
	conn := scriptedConn([]byte("RAW"))
	rt := NewChainRuntime([]NetworkFilter{term}, conn, ConnFacts{})
	if !rt.TerminalReady() {
		t.Fatal("pure-terminal chain not TerminalReady at construction")
	}
	rt.HandleTerminal(context.Background())
	if string(term.got) != "RAW" {
		t.Fatalf("pure-terminal handoff: terminal saw %q, want RAW", term.got)
	}
}

func TestMixedChainBufferedPrefixHandoff(t *testing.T) { // R-M
	term := &recordTerminal{}
	rf := &alwaysContinue{}
	conn := scriptedConn([]byte("LIVE"))
	rt := NewChainRuntime([]NetworkFilter{rf, term}, conn, ConnFacts{})
	rt.OnNewConnection()
	rt.OnData([]byte("PREFIX"), false) // rf Continues without draining → prefix retained
	if !rt.TerminalReady() {
		t.Fatal("mixed chain not TerminalReady after read filter Continued")
	}
	rt.HandleTerminal(context.Background())
	if string(term.got) != "PREFIXLIVE" {
		t.Fatalf("buffered-prefix handoff: terminal saw %q, want PREFIXLIVE", term.got)
	}
}

// stopThenContinue Continues OnNewConnection (so OnData flows — an
// OnNewConnection StopIteration is a sticky connHalted that would block OnData
// entirely), then stays in the chain via StopIteration on the FIRST OnData
// (buffering, no ContinueReading), and Continues to the terminal on the SECOND
// OnData. This is the mid-loop TerminalReady transition the post-OnData
// serveNetworkChain handoff branch relies on (rbac_network, 26.3: decide
// allow/deny after inspecting buffered bytes). It drains nothing, so the
// buffered prefix is retained for the terminal handoff.
type stopThenContinue struct {
	Marker
	cb        ReadFilterCallbacks
	continued bool
}

func (f *stopThenContinue) OnNewConnection() Status { return Continue }
func (f *stopThenContinue) OnData(_ *Buffer, _ bool) Status {
	if !f.continued {
		f.continued = true
		return StopIteration // first OnData: keep buffering, stay in the chain
	}
	return Continue // second OnData: release to the terminal
}
func (f *stopThenContinue) SetReadFilterCallbacks(cb ReadFilterCallbacks) { f.cb = cb }
func (f *stopThenContinue) OnDestroy()                                    {}

// TestMixedChainPostOnDataHandoff covers the THIRD terminal-handoff site: a read
// filter that does NOT release in OnNewConnection but Continues to the terminal
// during a LATER OnData, so TerminalReady flips true AFTER an OnData (inside the
// read loop), not before. The post-OnData serveNetworkChain branch depends on
// this transition (rbac_network 26.3). It also proves the buffered prefix
// accumulated across the multi-OnData stop/continue reaches the terminal ahead
// of the live tail.
func TestMixedChainPostOnDataHandoff(t *testing.T) {
	term := &recordTerminal{}
	rf := &stopThenContinue{}
	conn := scriptedConn([]byte("LIVE"))
	rt := NewChainRuntime([]NetworkFilter{rf, term}, conn, ConnFacts{})
	rt.OnNewConnection()
	if rt.TerminalReady() {
		t.Fatal("terminal ready before read filter Continued (should still be buffering)")
	}
	rt.OnData([]byte("PRE"), false) // first OnData: filter StopIteration → not ready
	if rt.TerminalReady() {
		t.Fatal("terminal ready after first OnData but filter has not Continued")
	}
	rt.OnData([]byte("FIX"), false) // second OnData: filter Continues → ready now (post-OnData transition)
	if !rt.TerminalReady() {
		t.Fatal("terminal not ready after read filter Continued mid-OnData")
	}
	rt.HandleTerminal(context.Background())
	// The filter drained nothing, so both undrained reads ("PRE"+"FIX") remain
	// in the connection buffer and are replayed before the live conn tail.
	if string(term.got) != "PREFIXLIVE" {
		t.Fatalf("post-OnData handoff: terminal saw %q, want PREFIXLIVE (buffered prefix then live tail)", term.got)
	}
}

func TestPureReadNeverTerminalReady(t *testing.T) {
	conn := scriptedConn(nil)
	rt := NewChainRuntime([]NetworkFilter{&filterB{}}, conn, ConnFacts{})
	rt.OnNewConnection()
	if rt.TerminalReady() {
		t.Fatal("pure-read chain reported TerminalReady")
	}
}

// fakeConn implements net.Conn capturing writes + close.
type fakeConn struct {
	writes []byte
	closed bool
	addr   net.Addr
}

func (c *fakeConn) Read(_ []byte) (int, error) { return 0, io.EOF }
func (c *fakeConn) Write(b []byte) (int, error) {
	c.writes = append(c.writes, b...)
	return len(b), nil
}
func (c *fakeConn) Close() error                       { c.closed = true; return nil }
func (c *fakeConn) LocalAddr() net.Addr                { return c.addr }
func (c *fakeConn) RemoteAddr() net.Addr               { return c.addr }
func (c *fakeConn) SetDeadline(_ time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(_ time.Time) error { return nil }

// filterA stops on its first OnData and, in the same call, requests resume at
// the next filter via ContinueReading (the StopIteration-then-resume contract,
// SPEC §3.3). filterB then sees the connection-level buffered bytes.
type filterA struct {
	Marker
	cb          ReadFilterCallbacks
	stoppedOnce bool
	onDataCalls int
}

func (f *filterA) OnNewConnection() Status { return Continue }
func (f *filterA) OnData(_ *Buffer, _ bool) Status {
	f.onDataCalls++
	if !f.stoppedOnce {
		f.stoppedOnce = true
		f.cb.ContinueReading() // resume at the NEXT filter with the live buffer
		return StopIteration
	}
	return Continue
}
func (f *filterA) SetReadFilterCallbacks(cb ReadFilterCallbacks) { f.cb = cb }
func (f *filterA) OnDestroy()                                    {}

type filterB struct {
	Marker
	saw           string
	newConnCalled bool
}

func (f *filterB) OnNewConnection() Status                    { f.newConnCalled = true; return Continue }
func (f *filterB) OnData(b *Buffer, _ bool) Status            { f.saw = string(b.Bytes()); return Continue }
func (f *filterB) SetReadFilterCallbacks(ReadFilterCallbacks) {}
func (f *filterB) OnDestroy()                                 {}

func TestChainContinueReadingResumesAtNextFilter(t *testing.T) {
	fc := &fakeConn{addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9}}
	a, b := &filterA{}, &filterB{}
	rt := newChainRuntime([]ReadFilter{a, b}, fc, connFacts{})
	rt.onNewConnection()
	rt.onData([]byte("payload"), false)
	// Resume path must be exercised LIVE: filterA must have taken its
	// StopIteration branch. A plain-advance impl (OnData just returns Continue)
	// never sets stoppedOnce, so this fails — proving the assertion discriminates.
	if !a.stoppedOnce {
		t.Error("filterA never halted — StopIteration→ContinueReading resume path not exercised")
	}
	// filterA.OnData must run exactly once for the halting path: the single
	// StopIteration call. ContinueReading resumes at the NEXT filter (not back at
	// filterA), so filterA is never re-entered.
	if a.onDataCalls != 1 {
		t.Errorf("filterA.OnData ran %d times, want 1 (StopIteration call only; resume advances to next filter)", a.onDataCalls)
	}
	if !b.newConnCalled {
		t.Errorf("filterB.OnNewConnection not called on resume")
	}
	if b.saw != "payload" {
		t.Errorf("filterB saw %q, want buffered bytes", b.saw)
	}
}

// stopConnFilter halts the chain at OnNewConnection (returns StopIteration);
// its own OnData must never run (ContinueReading resumes at the NEXT filter).
type stopConnFilter struct {
	Marker
	newConnCalled bool
	dataCalled    bool
}

func (f *stopConnFilter) OnNewConnection() Status                    { f.newConnCalled = true; return StopIteration }
func (f *stopConnFilter) OnData(*Buffer, bool) Status                { f.dataCalled = true; return Continue }
func (f *stopConnFilter) SetReadFilterCallbacks(ReadFilterCallbacks) {}
func (f *stopConnFilter) OnDestroy()                                 {}

// lazyConnFilter has OnNewConnection called LAZILY (only when the chain reaches
// it after a ContinueReading jump), proving the §3.3 lazy-OnNewConnection path.
type lazyConnFilter struct {
	Marker
	newConnCalled bool
	dataSaw       string
}

func (f *lazyConnFilter) OnNewConnection() Status { f.newConnCalled = true; return Continue }
func (f *lazyConnFilter) OnData(b *Buffer, _ bool) Status {
	f.dataSaw = string(b.Bytes())
	return Continue
}
func (f *lazyConnFilter) SetReadFilterCallbacks(ReadFilterCallbacks) {}
func (f *lazyConnFilter) OnDestroy()                                 {}

func TestChainOnNewConnectionStopHaltsThenResumesLazily(t *testing.T) {
	sf := &stopConnFilter{}
	lf := &lazyConnFilter{}
	rt := newChainRuntime([]ReadFilter{sf, lf}, &fakeConn{}, connFacts{})
	rt.onNewConnection()
	if !sf.newConnCalled {
		t.Fatalf("filter0 OnNewConnection not called eagerly")
	}
	if lf.newConnCalled {
		t.Fatalf("filter1 OnNewConnection called eagerly past the StopIteration halt")
	}
	if !rt.connHalted {
		t.Fatalf("chain did not halt on OnNewConnection StopIteration")
	}
	// A socket read arrives while the chain is parked on the OnNewConnection
	// halt: this halt IS sticky across reads (the filter has not consented to
	// data flowing past it), so bytes buffer but are not delivered until
	// ContinueReading resumes the chain. (This is the one legitimate
	// data-withheld path; an OnData StopIteration, by contrast, does NOT
	// withhold later reads — see TestChainEchoStyleMultipleReads.)
	rt.onData([]byte("late"), false)
	if sf.dataCalled || lf.dataSaw != "" {
		t.Fatalf("data delivered while halted: filter0.dataCalled=%v filter1.saw=%q", sf.dataCalled, lf.dataSaw)
	}
	rt.callbacks().ContinueReading()
	if sf.dataCalled {
		t.Errorf("filter0 OnData ran after ContinueReading (resume must skip the stopped filter)")
	}
	if !lf.newConnCalled {
		t.Errorf("filter1 OnNewConnection not called lazily on resume")
	}
	if lf.dataSaw != "late" {
		t.Errorf("after resume filter1 OnData saw %q, want buffered bytes", lf.dataSaw)
	}
}

// echoStyleFilter models the upstream echo read filter: on each OnData it
// records the bytes it saw, drains the buffer, and returns StopIteration WITHOUT
// ever calling ContinueReading (faithful to upstream echo.cc). StopIteration
// stops only the CURRENT pass's downstream propagation; it must NOT permanently
// halt the connection. Every subsequent socket read must re-deliver the
// accumulated bytes to OnData (upstream FilterManagerImpl::onRead re-iterates the
// chain on every read).
type echoStyleFilter struct {
	Marker
	seen []string
}

func (f *echoStyleFilter) OnNewConnection() Status { return Continue }
func (f *echoStyleFilter) OnData(b *Buffer, _ bool) Status {
	f.seen = append(f.seen, string(b.Bytes()))
	b.Drain(b.Len())
	return StopIteration
}
func (f *echoStyleFilter) SetReadFilterCallbacks(ReadFilterCallbacks) {}
func (f *echoStyleFilter) OnDestroy()                                 {}

// TestChainEchoStyleMultipleReads pins the corrected per-socket-read
// re-iteration semantics: a single echo-style filter (drains + StopIteration,
// no ContinueReading) must run its OnData on EVERY socket read. A prior
// StopIteration must not swallow a later fresh read. The second read is the
// regression (the old sticky-halt impl early-returned in onData once halted).
func TestChainEchoStyleMultipleReads(t *testing.T) {
	e := &echoStyleFilter{}
	rt := newChainRuntime([]ReadFilter{e}, &fakeConn{}, connFacts{})
	rt.onNewConnection()
	rt.onData([]byte("first"), false)
	rt.onData([]byte("second"), false)
	if len(e.seen) != 2 {
		t.Fatalf("echo-style filter OnData ran %d times, want 2 (once per socket read); seen=%v", len(e.seen), e.seen)
	}
	if e.seen[0] != "first" {
		t.Errorf("first read delivered %q, want %q", e.seen[0], "first")
	}
	if e.seen[1] != "second" {
		t.Errorf("second read delivered %q, want %q (regression: StopIteration must not permanently halt)", e.seen[1], "second")
	}
}

func TestChainDynamicMetadataRoundTrip(t *testing.T) {
	fc := &fakeConn{}
	rt := newChainRuntime([]ReadFilter{&filterB{}}, fc, connFacts{})
	bucket := rt.callbacks().DynamicMetadata()
	bucket.Set("f", "k", structpb.NewStringValue("v"))
	got, ok := rt.callbacks().DynamicMetadata().Get("f", "k")
	if !ok || got.GetStringValue() != "v" {
		t.Fatalf("metadata round-trip failed: %v %v", got, ok)
	}
}

func TestConnectionAccessorSurface(t *testing.T) { // R2 readiness — prove each accessor is live
	fc := &fakeConn{addr: &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 443}}
	rt := newChainRuntime(nil, fc, connFacts{serverName: "sni.example", principals: []string{"spiffe://x"}})
	c := rt.callbacks().Connection()
	c.Write([]byte("hi"), true)
	if string(fc.writes) != "hi" {
		t.Errorf("Write not forwarded: %q", fc.writes)
	}
	if c.RequestedServerName() != "sni.example" {
		t.Errorf("SNI accessor dead")
	}
	if len(c.DownstreamPrincipals()) != 1 {
		t.Errorf("principals accessor dead")
	}
	if c.RemoteAddr().String() != "10.0.0.1:443" {
		t.Errorf("RemoteAddr dead")
	}
	c.Close(FlushWrite)
	if !rt.closeRequested() {
		t.Errorf("Close did not set closeRequested (D-P26.1-5a)")
	}
}

// closeFilter closes the connection with a configured CloseType from
// OnNewConnection, then halts. Used to prove connection.Close records the
// requested CloseType distinctly (FlushWrite vs NoFlush; F3).
type closeFilter struct {
	Marker
	cb ReadFilterCallbacks
	ct CloseType
}

func (f *closeFilter) OnNewConnection() Status {
	f.cb.Connection().Close(f.ct)
	return StopIteration
}
func (f *closeFilter) OnData(*Buffer, bool) Status                   { return StopIteration }
func (f *closeFilter) SetReadFilterCallbacks(cb ReadFilterCallbacks) { f.cb = cb }
func (f *closeFilter) OnDestroy()                                    {}

func TestConnectionCloseRecordsCloseType(t *testing.T) {
	// FlushWrite (default, zero value) path.
	rtF := NewChainRuntime([]NetworkFilter{&closeFilter{ct: FlushWrite}}, scriptedConn(nil), ConnFacts{})
	rtF.OnNewConnection()
	if !rtF.CloseRequested() {
		t.Fatal("FlushWrite close not requested")
	}
	if rtF.CloseType() != FlushWrite {
		t.Fatalf("CloseType = %v, want FlushWrite", rtF.CloseType())
	}
	// NoFlush path.
	rtN := NewChainRuntime([]NetworkFilter{&closeFilter{ct: NoFlush}}, scriptedConn(nil), ConnFacts{})
	rtN.OnNewConnection()
	if !rtN.CloseRequested() {
		t.Fatal("NoFlush close not requested")
	}
	if rtN.CloseType() != NoFlush {
		t.Fatalf("CloseType = %v, want NoFlush", rtN.CloseType())
	}
}

// TestCloseTypeDefaultsToFlushWrite pins that an un-closed runtime reports the
// zero value FlushWrite, so the serveNetworkChain default close path is
// unchanged from the pre-26.3 behavior (R: back-compat).
func TestCloseTypeDefaultsToFlushWrite(t *testing.T) {
	rt := NewChainRuntime(nil, scriptedConn(nil), ConnFacts{})
	if rt.CloseType() != FlushWrite {
		t.Fatalf("default CloseType = %v, want FlushWrite (zero value)", rt.CloseType())
	}
}

func TestResponseCodeDetailsSink(t *testing.T) { // D-P26.1-5b — prove the sink is live before direct_response uses it
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	setter, ok := rt.callbacks().(interface{ SetResponseCodeDetails(string) })
	if !ok {
		t.Fatalf("callbacks does not expose SetResponseCodeDetails — direct_response (Task 8) sink would be dead")
	}
	setter.SetResponseCodeDetails("DirectResponse")
	if rt.responseCodeDetails() != "DirectResponse" {
		t.Errorf("RCD not stored: %q", rt.responseCodeDetails())
	}
}

func TestSetUpstreamClusterStoresOverride(t *testing.T) {
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.cb.SetUpstreamCluster("foo.example.com")
	if got := rt.upstreamClusterOverride; got != "foo.example.com" {
		t.Fatalf("upstreamClusterOverride = %q, want %q", got, "foo.example.com")
	}
}

// fakeWriteFilter is a synthetic WriteFilter recording OnWrite calls + injections.
// status controls the per-call return; calls records the buffer contents seen.
// order is an optional shared recorder: when non-nil, OnWrite appends f.name to
// *order (used by TestWriteChainConnDispatchOrder to assert strict front-to-back
// ordering over the dispatch slice).
// Reused by Tasks 3–5.
type fakeWriteFilter struct {
	Marker
	name      string
	status    Status
	calls     []string
	order     *[]string
	wcb       WriteFilterCallbacks
	wcbCalls  int
	destroyed int
}

func (f *fakeWriteFilter) OnWrite(buf *Buffer, _ bool) Status {
	f.calls = append(f.calls, f.name+":"+string(buf.Bytes()))
	if f.order != nil {
		*f.order = append(*f.order, f.name)
	}
	return f.status
}
func (f *fakeWriteFilter) SetWriteFilterCallbacks(cb WriteFilterCallbacks) { f.wcb = cb; f.wcbCalls++ }
func (f *fakeWriteFilter) OnDestroy()                                      { f.destroyed++ }

// Compile-time assertion: fakeWriteFilter must satisfy WriteFilter (Tasks 3–5 use it).
var _ WriteFilter = (*fakeWriteFilter)(nil)

// TestWriteCallbacksConnectionAccessor proves the concrete writeCallbacks
// Connection() returns the SAME per-connection accessor the read callbacks
// return (SPEC §3.1 — one connection, two views).
func TestWriteCallbacksConnectionAccessor(t *testing.T) {
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	wcb := &writeCallbacks{rt: rt}
	if wcb.Connection() != Connection(rt.cxn) {
		t.Fatal("writeCallbacks.Connection() != rt.cxn (must be the same accessor as read callbacks)")
	}
}

func TestChainOnDestroyCallsAllFilters(t *testing.T) {
	a, b := &destroyFilter{}, &destroyFilter{}
	rt := newChainRuntime([]ReadFilter{a, b}, &fakeConn{}, connFacts{})
	rt.onDestroy()
	if !a.destroyed || !b.destroyed {
		t.Errorf("OnDestroy not called on all filters: a=%v b=%v", a.destroyed, b.destroyed)
	}
}

type destroyFilter struct {
	Marker
	destroyed bool
}

func (f *destroyFilter) OnNewConnection() Status                    { return Continue }
func (f *destroyFilter) OnData(*Buffer, bool) Status                { return Continue }
func (f *destroyFilter) SetReadFilterCallbacks(ReadFilterCallbacks) {}
func (f *destroyFilter) OnDestroy()                                 { f.destroyed = true }

// fakeBothFilter implements BOTH ReadFilter and WriteFilter (the zookeeperproxy
// shape — one instance, both directions; upstream addFilter parity).
type fakeBothFilter struct {
	Marker
	rcb       ReadFilterCallbacks
	wcb       WriteFilterCallbacks
	rcbCalls  int
	wcbCalls  int
	destroyed int
}

func (f *fakeBothFilter) OnNewConnection() Status                         { return Continue }
func (f *fakeBothFilter) OnData(*Buffer, bool) Status                     { return Continue }
func (f *fakeBothFilter) SetReadFilterCallbacks(cb ReadFilterCallbacks)   { f.rcb = cb; f.rcbCalls++ }
func (f *fakeBothFilter) OnWrite(*Buffer, bool) Status                    { return Continue }
func (f *fakeBothFilter) SetWriteFilterCallbacks(cb WriteFilterCallbacks) { f.wcb = cb; f.wcbCalls++ }
func (f *fakeBothFilter) OnDestroy()                                      { f.destroyed++ }

// Compile-time assertion: fakeBothFilter must satisfy both ReadFilter and WriteFilter.
var _ ReadFilter = (*fakeBothFilter)(nil)
var _ WriteFilter = (*fakeBothFilter)(nil)

// A both-directions filter lands in BOTH the read and write sets — SAME instance.
func TestClassificationBothDirectionsFilter(t *testing.T) {
	both := &fakeBothFilter{}
	term := &recordTerminal{}
	crt := NewChainRuntime([]NetworkFilter{both, term}, &fakeConn{}, ConnFacts{})
	rt := crt.rt
	if len(rt.filters) != 1 || rt.filters[0].(*fakeBothFilter) != both {
		t.Fatalf("read set = %v, want [both]", rt.filters)
	}
	if len(rt.writeFilters) != 1 || rt.writeFilters[0].(*fakeBothFilter) != both {
		t.Fatalf("write set = %v, want [both]", rt.writeFilters)
	}
}

// A write-only filter lands ONLY in the write set (framework-level; boot still
// rejects it — manager.go untouched, SPEC §3.6).
func TestClassificationWriteOnlyFilter(t *testing.T) {
	wf := &fakeWriteFilter{name: "w", status: Continue}
	crt := NewChainRuntime([]NetworkFilter{wf}, &fakeConn{}, ConnFacts{})
	rt := crt.rt
	if len(rt.filters) != 0 {
		t.Fatalf("read set = %v, want empty", rt.filters)
	}
	if len(rt.writeFilters) != 1 {
		t.Fatalf("write set len = %d, want 1", len(rt.writeFilters))
	}
}

// Both-directions filter receives BOTH callback injections, each exactly once (D-P2).
func TestBothFilterDualCallbackInjection(t *testing.T) {
	both := &fakeBothFilter{}
	NewChainRuntime([]NetworkFilter{both}, &fakeConn{}, ConnFacts{})
	if both.rcbCalls != 1 || both.wcbCalls != 1 {
		t.Fatalf("injections (read=%d, write=%d), want (1, 1)", both.rcbCalls, both.wcbCalls)
	}
	if both.rcb == nil || both.wcb == nil {
		t.Fatal("callbacks not stored")
	}
	if both.wcb.Connection() != both.rcb.Connection() {
		t.Fatal("write and read callbacks must expose the SAME Connection accessor")
	}
}

// countingReadFilter is a minimal read-only double that counts OnDestroy calls.
type countingReadFilter struct {
	Marker
	destroyed int
}

func (f *countingReadFilter) OnNewConnection() Status                    { return Continue }
func (f *countingReadFilter) OnData(*Buffer, bool) Status                { return Continue }
func (f *countingReadFilter) SetReadFilterCallbacks(ReadFilterCallbacks) {}
func (f *countingReadFilter) OnDestroy()                                 { f.destroyed++ }

// Compile-time assertion: countingReadFilter must satisfy ReadFilter.
var _ ReadFilter = (*countingReadFilter)(nil)

// --- Task 5: handleTerminal writeChainConn wrap ---

// Zero-write-filter chains get NO writeChainConn wrap (R1 back-compat): the
// terminal receives the raw conn (or prefixConn) — never a writeChainConn.
func TestHandleTerminalZeroWriteFiltersUnwrapped(t *testing.T) {
	rec := &recordingTerminal{} // upstreamcluster_test.go double (records ctx + conn)
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.handleTerminal(context.Background())
	if _, isWrap := rec.gotConn.(*writeChainConn); isWrap {
		t.Fatal("zero-write-filter chain must NOT wrap the terminal conn (R1 back-compat)")
	}
}

// ≥1 write filter: the terminal receives a writeChainConn; composition is
// writeChainConn(prefixConn(conn)) when a buffered prefix exists — prefixConn
// INNER so reads still replay the prefix.
func TestHandleTerminalWrapComposition(t *testing.T) {
	rec := &recordingTerminal{}
	wf := &fakeWriteFilter{name: "w", status: Continue}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{wf}
	rt.buf.Append([]byte("prefix")) // simulate undrained buffered prefix
	rt.handleTerminal(context.Background())
	wrap, ok := rec.gotConn.(*writeChainConn)
	if !ok {
		t.Fatalf("terminal conn = %T, want *writeChainConn", rec.gotConn)
	}
	if _, ok := wrap.Conn.(*prefixConn); !ok {
		t.Fatalf("writeChainConn wraps %T, want *prefixConn (prefix INNER)", wrap.Conn)
	}
	// Reads promote through writeChainConn to the prefix replay:
	got := make([]byte, 6)
	n, _ := wrap.Read(got)
	if string(got[:n]) != "prefix" {
		t.Fatalf("Read through writeChainConn = %q, want prefix replay", got[:n])
	}
}

// Write dispatch through handleTerminal is REVERSE chain order (AMEND-A11):
// chain [A, B] ⇒ terminal write dispatch B → A.
func TestHandleTerminalReverseWriteDispatch(t *testing.T) {
	var order []string
	mk := func(name string) *fakeWriteFilter {
		return &fakeWriteFilter{name: name, status: Continue, order: &order}
	}
	a, b := mk("A"), mk("B")
	rec := &recordingTerminal{}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{a, b} // CHAIN order
	rt.handleTerminal(context.Background())
	_, _ = rec.gotConn.Write([]byte("x"))
	if len(order) != 2 || order[0] != "B" || order[1] != "A" {
		t.Fatalf("write dispatch order = %v, want [B A] (reverse chain order)", order)
	}
}

// The chain-order slice on the runtime is NOT mutated by the reversal (the
// dispatch slice is a copy).
func TestHandleTerminalDoesNotMutateChainOrder(t *testing.T) {
	a := &fakeWriteFilter{name: "A", status: Continue}
	b := &fakeWriteFilter{name: "B", status: Continue}
	rec := &recordingTerminal{}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{a, b}
	rt.handleTerminal(context.Background())
	if rt.writeFilters[0].(*fakeWriteFilter).name != "A" {
		t.Fatal("handleTerminal mutated chainRuntime.writeFilters (must reverse a COPY)")
	}
}

// OnDestroy runs exactly ONCE per instance for a both-directions filter (D-P2 dedupe);
// read-only and write-only filters each get exactly one call too.
func TestOnDestroyOncePerInstance(t *testing.T) {
	both := &fakeBothFilter{}
	ro := &countingReadFilter{}
	wo := &fakeWriteFilter{name: "w", status: Continue}
	crt := NewChainRuntime([]NetworkFilter{ro, both, wo}, &fakeConn{}, ConnFacts{})
	crt.rt.onDestroy()
	if both.destroyed != 1 {
		t.Fatalf("both-directions filter destroyed %d times, want exactly 1", both.destroyed)
	}
	if ro.destroyed != 1 {
		t.Fatalf("read-only filter destroyed %d times, want 1", ro.destroyed)
	}
	if wo.destroyed != 1 {
		t.Fatalf("write-only filter destroyed %d times, want 1", wo.destroyed)
	}
}
