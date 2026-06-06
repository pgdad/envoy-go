package network

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
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

// fakeDrain is a synthetic DrainState (29.3 Task 9): IsDraining returns the
// configured flag. The network package consumes drain via this narrow structural
// interface so it never imports internal/drain (D-S29.3-3).
type fakeDrain struct{ draining bool }

func (f fakeDrain) IsDraining() bool { return f.draining }

// TestCallbacksDrainingAccessor proves the Draining() accessor reflects the
// SetDrainState-wired DrainState, and is false (no panic) when none is wired
// (the true-nil interface guard — the typed-nil trap is dodged at the call site).
func TestCallbacksDrainingAccessor(t *testing.T) {
	rt := NewChainRuntime(nil, &fakeConn{}, ConnFacts{})
	if rt.rt.cb.Draining() {
		t.Error("no drain state set → Draining() must be false")
	}
	rt.SetDrainState(fakeDrain{draining: true})
	if !rt.rt.cb.Draining() {
		t.Error("Draining() must reflect the set DrainState")
	}
	rt.SetDrainState(fakeDrain{draining: false})
	if rt.rt.cb.Draining() {
		t.Error("Draining() must reflect a non-draining DrainState")
	}
}

// TestCallbacksDrainingNilNoPanic pins the typed-nil-in-interface guard (29.3
// risk register line 1846): a chain with NO drain state wired keeps rt.drain a
// true-nil interface, so Draining() returns false WITHOUT calling IsDraining()
// on a nil receiver (no panic). serveNetworkChain guards SetDrainState(rt.dm)
// with `if rt.dm != nil` to keep this true-nil; this test proves the accessor's
// own `c.rt.drain != nil` short-circuit holds.
func TestCallbacksDrainingNilNoPanic(t *testing.T) {
	rt := NewChainRuntime(nil, &fakeConn{}, ConnFacts{})
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Draining() panicked with no drain state wired: %v", r)
		}
	}()
	if rt.rt.cb.Draining() {
		t.Error("Draining() must be false when no drain state is wired")
	}
}

// TestCallbacksCloseDirectionDefault proves the CloseDirection() accessor reads
// the runtime's closeDir field, defaulting to Unset (the read-side lands at Task
// 9; the setter + tcp_proxy recording is Task 10).
func TestCallbacksCloseDirectionDefault(t *testing.T) {
	rt := NewChainRuntime(nil, &fakeConn{}, ConnFacts{})
	if got := rt.rt.cb.CloseDirection(); got != CloseDirectionUnset {
		t.Errorf("CloseDirection() = %v, want CloseDirectionUnset (default)", got)
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

// ≥1 write filter: the terminal receives a writeChainConn; composition is
// writeChainConn(prefixConn(readChainConn(conn))) when a buffered prefix exists —
// prefixConn over the readChainConn so reads still replay the prefix.
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
	pc, ok := wrap.Conn.(*prefixConn)
	if !ok {
		t.Fatalf("writeChainConn wraps %T, want *prefixConn (prefix INNER)", wrap.Conn)
	}
	// 28.1b: prefixConn now wraps a readChainConn (was the raw conn at 28.1a).
	if _, ok := pc.Conn.(*readChainConn); !ok {
		t.Fatalf("prefixConn wraps %T, want *readChainConn (innermost — 28.1b read seam)", pc.Conn)
	}
	// Reads promote through writeChainConn to the prefix replay:
	got := make([]byte, 6)
	n, _ := wrap.Read(got)
	if string(got[:n]) != "prefix" {
		t.Fatalf("Read through writeChainConn = %q, want prefix replay", got[:n])
	}
}

// --- Task 5 (28.1b): handleTerminal read-wrap insertion ---

// R1 (BOTH wraps): zero-write-filter chains get NEITHER conn-wrap — the
// terminal receives the raw conn (or prefixConn over the raw conn), never a
// readChainConn and never a writeChainConn. This is the structural back-compat
// guarantee for every existing production chain (SPEC §3.4 item 1).
func TestHandleTerminalZeroWriteFiltersNeitherWrap(t *testing.T) {
	rec := &recordingTerminal{}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.handleTerminal(context.Background())
	if _, isWrap := rec.gotConn.(*writeChainConn); isWrap {
		t.Fatal("zero-write-filter chain must NOT get a writeChainConn (R1)")
	}
	if _, isWrap := rec.gotConn.(*readChainConn); isWrap {
		t.Fatal("zero-write-filter chain must NOT get a readChainConn (R1 — the SHARED predicate)")
	}
}

// Zero write filters + buffered prefix: prefixConn wraps the RAW conn directly
// (byte-identical to the 26.2/27/28.1a composition — no readChainConn in between).
func TestHandleTerminalZeroWriteFiltersPrefixOverRawConn(t *testing.T) {
	rec := &recordingTerminal{}
	raw := &fakeConn{}
	rt := newChainRuntime(nil, raw, connFacts{})
	rt.terminal = rec
	rt.buf.Append([]byte("prefix"))
	rt.handleTerminal(context.Background())
	pc, ok := rec.gotConn.(*prefixConn)
	if !ok {
		t.Fatalf("terminal conn = %T, want *prefixConn", rec.gotConn)
	}
	if pc.Conn != net.Conn(raw) {
		t.Fatalf("prefixConn wraps %T, want the raw conn (no intermediate wrap for R1 chains)", pc.Conn)
	}
}

// ≥1 write filter + buffered prefix: the FULL composition, innermost → outermost:
// readChainConn(raw) ← prefixConn ← writeChainConn (SPEC §3.1).
func TestHandleTerminalFullCompositionOrder(t *testing.T) {
	rec := &recordingTerminal{}
	raw := &fakeConn{}
	wf := &fakeWriteFilter{name: "w", status: Continue}
	rt := newChainRuntime(nil, raw, connFacts{})
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{wf}
	rt.buf.Append([]byte("prefix"))
	rt.handleTerminal(context.Background())

	wc, ok := rec.gotConn.(*writeChainConn)
	if !ok {
		t.Fatalf("outermost = %T, want *writeChainConn", rec.gotConn)
	}
	pc, ok := wc.Conn.(*prefixConn)
	if !ok {
		t.Fatalf("middle = %T, want *prefixConn", wc.Conn)
	}
	rc, ok := pc.Conn.(*readChainConn)
	if !ok {
		t.Fatalf("inner = %T, want *readChainConn (INNERMOST — prefix bytes bypass the replay)", pc.Conn)
	}
	if rc.Conn != net.Conn(raw) {
		t.Fatalf("readChainConn wraps %T, want the raw conn", rc.Conn)
	}
}

// ≥1 write filter, NO prefix: writeChainConn(readChainConn(raw)).
func TestHandleTerminalCompositionNoPrefix(t *testing.T) {
	rec := &recordingTerminal{}
	raw := &fakeConn{}
	wf := &fakeWriteFilter{name: "w", status: Continue}
	rt := newChainRuntime(nil, raw, connFacts{})
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{wf}
	rt.handleTerminal(context.Background())

	wc, ok := rec.gotConn.(*writeChainConn)
	if !ok {
		t.Fatalf("outermost = %T, want *writeChainConn", rec.gotConn)
	}
	rc, ok := wc.Conn.(*readChainConn)
	if !ok {
		t.Fatalf("inner = %T, want *readChainConn", wc.Conn)
	}
	if rc.Conn != net.Conn(raw) {
		t.Fatalf("readChainConn wraps %T, want the raw conn", rc.Conn)
	}
}

// The prefixConn's buffered prefix is NOT re-fed through the replay: the read
// filters already saw those bytes pre-handoff. Only LIVE post-handoff socket
// reads replay (SPEC §3.1 composition item 1 — the innermost-is-load-bearing test).
func TestHandleTerminalPrefixNotReFedThroughReplay(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	wf := &fakeWriteFilter{name: "w", status: Continue}
	raw := &multiReadConn{payloads: [][]byte{[]byte("LIVE")}} // the Task-4 multi-read double
	rt := newChainRuntime([]ReadFilter{f}, raw, connFacts{})
	rec := &recordingTerminal{}
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{wf}

	// Pre-handoff: the filter sees "pre" via the normal onData path; the bytes
	// stay undrained (passthrough filter) → become the handoff prefix.
	rt.onData([]byte("pre"), false)
	if string(f.seen) != "pre" {
		t.Fatalf("pre-handoff: filter saw %q, want pre", f.seen)
	}

	rt.handleTerminal(context.Background())

	// The terminal reads through the full stack: first the prefix, then live bytes.
	buf := make([]byte, 16)
	n, _ := rec.gotConn.Read(buf)
	if string(buf[:n]) != "pre" {
		t.Fatalf("first terminal read = %q, want the prefix replay", buf[:n])
	}
	// The prefix read must NOT have re-fed the filter (it saw "pre" exactly once).
	if string(f.seen) != "pre" {
		t.Fatalf("after prefix read: filter saw %q, want pre (prefix NOT re-fed)", f.seen)
	}
	n, _ = rec.gotConn.Read(buf)
	if string(buf[:n]) != "LIVE" {
		t.Fatalf("second terminal read = %q, want LIVE", buf[:n])
	}
	// The live read DID replay: the filter has now seen pre + LIVE, each exactly once.
	if string(f.seen) != "preLIVE" {
		t.Fatalf("after live read: filter saw %q, want preLIVE (live bytes replayed exactly once)", f.seen)
	}
}

// The §3.3 soundness invariant, end-to-end: a tracking filter (the TotalAppended
// high-water-mark discipline — exactly the zookeeperproxy decoder's discipline)
// sees EVERY appended byte EXACTLY ONCE across the pre-handoff regime, the
// handoff drain, and the post-handoff replay regime.
func TestChainSoundnessInvariantEveryByteSeenExactlyOnce(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	wf := &fakeWriteFilter{name: "w", status: Continue}
	raw := &multiReadConn{payloads: [][]byte{[]byte("ghi"), []byte("jkl")}} // the Task-4 multi-read double
	rt := newChainRuntime([]ReadFilter{f}, raw, connFacts{})
	rec := &recordingTerminal{}
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{wf}

	// Pre-handoff: one socket read via the normal onData path. (The runtime
	// hands off the moment every read filter has Continued — serveNetworkChain's
	// read loop returns at the first TerminalReady, internal/listener/manager.go
	// :1066-1068 — so a passthrough chain sees exactly ONE pre-handoff read; the
	// rest are post-handoff replays. Feeding two pre-handoff onData calls would
	// NOT model the runtime: the second would append undrained bytes the filter
	// never saw, violating the high-water-mark discipline this very test asserts.)
	rt.onData([]byte("abcdef"), false)
	if string(f.seen) != "abcdef" {
		t.Fatalf("pre-handoff: filter saw %q, want abcdef", f.seen)
	}
	// Handoff (drains the buffer into the prefix).
	rt.handleTerminal(context.Background())
	// Post-handoff: the terminal drains the prefix, then two live reads (replayed).
	buf := make([]byte, 16)
	for i := 0; i < 3; i++ { // prefix ("abcdef"), "ghi", "jkl"
		_, _ = rec.gotConn.Read(buf)
	}

	if string(f.seen) != "abcdefghijkl" {
		t.Fatalf("tracking filter saw %q, want abcdefghijkl (every appended byte exactly once — no drop, no double-feed)", f.seen)
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

// --- Task 3: chainRuntime.replayRead ---

// recordingReadFilter records every OnData delivery: the bytes it saw (novel
// tail, tracked via the TotalAppended discipline — the same discipline the
// zookeeperproxy decoder uses) + the endStream flags + a call order tag.
// status controls the per-call return (Status is IGNORED on the replay path —
// the StopIteration variant proves it).
type recordingReadFilter struct {
	Marker
	name      string
	status    Status
	seen      []byte // novel bytes, accumulated via the high-water-mark discipline
	mark      int64  // high-water mark against buf.TotalAppended()
	ends      []bool // endStream flag per OnData call
	order     *[]string
	destroyed int
}

func (f *recordingReadFilter) OnNewConnection() Status { return Continue }
func (f *recordingReadFilter) OnData(buf *Buffer, endStream bool) Status {
	if newCount := buf.TotalAppended() - f.mark; newCount > 0 {
		bs := buf.Bytes()
		f.seen = append(f.seen, bs[int64(len(bs))-newCount:]...)
		f.mark = buf.TotalAppended()
	}
	f.ends = append(f.ends, endStream)
	if f.order != nil {
		*f.order = append(*f.order, f.name)
	}
	return f.status
}
func (f *recordingReadFilter) SetReadFilterCallbacks(ReadFilterCallbacks) {}
func (f *recordingReadFilter) OnDestroy()                                 { f.destroyed++ }

var _ ReadFilter = (*recordingReadFilter)(nil)

// replayRead delivers to ALL read filters in CHAIN order (SPEC §3.2 item 1 /
// D-28.1b-5: upstream re-iteration parity).
func TestReplayReadDeliversToAllFiltersInChainOrder(t *testing.T) {
	var order []string
	a := &recordingReadFilter{name: "A", status: Continue, order: &order}
	b := &recordingReadFilter{name: "B", status: Continue, order: &order}
	rt := newChainRuntime([]ReadFilter{a, b}, &fakeConn{}, connFacts{})
	rt.replayRead([]byte("xyz"), false)
	if string(a.seen) != "xyz" || string(b.seen) != "xyz" {
		t.Fatalf("replay delivery: A saw %q, B saw %q, want xyz/xyz", a.seen, b.seen)
	}
	if len(order) != 2 || order[0] != "A" || order[1] != "B" {
		t.Fatalf("replay order = %v, want [A B] (chain order)", order)
	}
}

// Status is IGNORED on the replay path (SPEC §3.5 row 1 — observational): a
// mid-chain StopIteration does NOT halt delivery to later filters. This is the
// LIVE divergence-from-pre-handoff assertion (§11.1).
func TestReplayReadStatusIgnoredMidChainStop(t *testing.T) {
	a := &recordingReadFilter{name: "A", status: StopIteration}
	b := &recordingReadFilter{name: "B", status: Continue}
	rt := newChainRuntime([]ReadFilter{a, b}, &fakeConn{}, connFacts{})
	rt.replayRead([]byte("xyz"), false)
	if string(b.seen) != "xyz" {
		t.Fatalf("filter B saw %q, want xyz (A's StopIteration must be IGNORED on the replay path)", b.seen)
	}
	// And the chain's pre-handoff park state is untouched (no resumeIdx/connHalted side effects).
	if rt.connHalted {
		t.Fatal("replayRead must not set connHalted")
	}
}

// The runtime drains the chain buffer fully after each replay pass (SPEC §3.2
// item 3 — bounded memory).
func TestReplayReadDrainsAfterPass(t *testing.T) {
	a := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{a}, &fakeConn{}, connFacts{})
	rt.replayRead([]byte("hello"), false)
	if rt.buf.Len() != 0 {
		t.Fatalf("chain buffer Len = %d after replay pass, want 0 (drain-after-pass)", rt.buf.Len())
	}
	// TotalAppended is monotonic across the drain (the §3.2 item 2 continuity).
	if rt.buf.TotalAppended() != 5 {
		t.Fatalf("TotalAppended = %d, want 5", rt.buf.TotalAppended())
	}
	rt.replayRead([]byte("-more"), false)
	if string(a.seen) != "hello-more" {
		t.Fatalf("filter saw %q across two replay passes, want hello-more (every byte exactly once)", a.seen)
	}
}

// endStream is delivered through the replay (SPEC §3.2 item 4 — the EOF replay).
func TestReplayReadEndStream(t *testing.T) {
	a := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{a}, &fakeConn{}, connFacts{})
	rt.replayRead([]byte("x"), false)
	rt.replayRead(nil, true) // the EOF endStream replay (zero bytes)
	if len(a.ends) != 2 || a.ends[0] != false || a.ends[1] != true {
		t.Fatalf("endStream flags = %v, want [false true]", a.ends)
	}
}

// --- Task 6: §3.6 concurrent-pumps race test (D-S28.1b-2) ---

// pumpingTerminal mirrors tcp_proxy.Handle's goroutine topology (filter.go:134-138):
// two concurrent io.Copy pumps + wg.Wait. It is the §3.6 race-surface double.
type pumpingTerminal struct {
	Marker
	upstream net.Conn
}

func (p *pumpingTerminal) Handle(_ context.Context, downstream net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// Close BOTH sides when pump A exits, mirroring tcp_proxy.Handle's
		// deferred conn.Close() discipline:
		//  - downstream.Close() unblocks any stuck testEnd.Write (client writer)
		//    and causes pump B's downstream.Write to fail → pump B exits.
		//  - upstream.Close() unblocks pump B if it is blocked in upstream.Read
		//    waiting for data that will never arrive (backend sender already done).
		defer func() { _ = downstream.Close(); _ = p.upstream.Close() }()
		_, _ = io.Copy(p.upstream, downstream) // goroutine A: reads → replay
	}()
	go func() { defer wg.Done(); _, _ = io.Copy(downstream, p.upstream) }() // goroutine B: writes → write chain
	wg.Wait()
}

// raceBothFilter is the minimal both-directions double for the race test: the
// read path counts bytes ATOMICALLY (it runs on pump goroutine A); OnWrite is
// a no-op Continue (it runs on pump goroutine B). The two paths share no other
// state — exactly the 28.1b production posture (§3.6 item 2).
type raceBothFilter struct {
	Marker
	readBytes atomic.Int64
	mark      int64 // touched ONLY on goroutine A (the §3.6 pin); no lock needed
}

func (f *raceBothFilter) OnNewConnection() Status { return Continue }
func (f *raceBothFilter) OnData(buf *Buffer, _ bool) Status {
	if newCount := buf.TotalAppended() - f.mark; newCount > 0 {
		f.readBytes.Add(newCount)
		f.mark = buf.TotalAppended()
	}
	return Continue
}
func (f *raceBothFilter) SetReadFilterCallbacks(ReadFilterCallbacks)   {}
func (f *raceBothFilter) OnWrite(*Buffer, bool) Status                 { return Continue }
func (f *raceBothFilter) SetWriteFilterCallbacks(WriteFilterCallbacks) {}
func (f *raceBothFilter) OnDestroy()                                   {}

// Compile-time assertion: raceBothFilter must satisfy both ReadFilter and WriteFilter.
var _ ReadFilter = (*raceBothFilter)(nil)
var _ WriteFilter = (*raceBothFilter)(nil)

// TestWrappedChainConcurrentPumpsRace drives a wrapped chain (a both-directions
// synthetic filter, the zookeeperproxy shape) under live concurrent pumps over
// net.Pipe, with traffic flowing BOTH directions simultaneously. The assertion
// is `go test -race` itself: the 28.1b race surface must be EMPTY (SPEC §3.6
// item 2 — goroutine A touches rt.buf + the read path; goroutine B's OnWrite
// path shares NO mutable state with it). The test also asserts the filter saw
// every downstream byte (the replay is live under concurrency).
//
// Goroutine topology (four driver goroutines + one wg.Wait/close bookkeeping goroutine
// + the two pump goroutines inside pumpingTerminal.Handle = seven goroutines total):
//
//	client writer  → testEnd.Write  → chainEnd.Read  (pump A) → replayRead (filter sees bytes)
//	pump A         → upstreamEnd.Write → backendEnd.Read (backend reader, discarded)
//	backend sender → backendEnd.Write → upstreamEnd.Read (pump B) → chainEnd.Write
//	pump B         → chainEnd.Write → testEnd.Read   (client reader, discarded)
//
// Every net.Pipe write has exactly one reader. When the client closes testEnd,
// pump A (chainEnd.Read → error) and pump B (chainEnd.Write → error) both exit,
// which unblocks Handle's wg.Wait() → HandleTerminal returns. After HandleTerminal
// returns, upstreamEnd is closed explicitly, unblocking the backend sender and the
// backend reader so all four driver goroutines can exit cleanly.
func TestWrappedChainConcurrentPumpsRace(t *testing.T) {
	// Both-directions filter: records read-path bytes via the TotalAppended
	// discipline; OnWrite is a pure no-op Continue (the 28.1 §4.7 pin).
	f := &raceBothFilter{}
	term := &pumpingTerminal{}

	// downstream pipe: testEnd <-> chainEnd (rt.conn); upstream pipe: upstreamEnd <-> backendEnd.
	testEnd, chainEnd := net.Pipe()
	upstreamEnd, backendEnd := net.Pipe()
	term.upstream = upstreamEnd

	crt := NewChainRuntime([]NetworkFilter{f, term}, chainEnd, ConnFacts{})
	crt.OnNewConnection()
	// Drive the chain past the read filter: deliver an endStream-only OnData (zero
	// bytes, endStream=true). raceBothFilter.OnData sees TotalAppended=0 → adds
	// nothing to readBytes; it returns Continue → resumeIdx advances past the last
	// read filter → terminalReady() becomes true. No prefix bytes are buffered.
	crt.OnData(nil, true)
	if !crt.TerminalReady() {
		t.Fatal("chain must be terminal-ready after the endStream pass (both filter Continues)")
	}

	// Timeout guard: the entire test must complete within this window; a
	// deadlock-induced hang becomes a test failure rather than a hanging process.
	// 60s is generous even under -race; a real deadlock hangs indefinitely.
	deadline := 60 * time.Second
	if d, ok := t.Deadline(); ok {
		if rem := time.Until(d) - 2*time.Second; rem > 0 && rem < deadline {
			deadline = rem
		}
	}
	timer := time.NewTimer(deadline)
	defer timer.Stop()

	done := make(chan struct{})
	go func() { crt.HandleTerminal(context.Background()); close(done) }()

	const writes = 50
	const payload = "downstream-frame-"
	const upPayload = "upstream-frame-"

	var wg sync.WaitGroup
	wg.Add(4)
	// Client writer: writes 50 downstream payloads, then closes testEnd.
	// Closing testEnd causes chainEnd.Read to error → pump A exits.
	// chainEnd.Write also errors → pump B exits.
	// Both pump exits → Handle's wg.Wait returns → HandleTerminal returns → done closes.
	go func() {
		defer wg.Done()
		for i := 0; i < writes; i++ {
			if _, err := testEnd.Write([]byte(payload)); err != nil {
				return
			}
		}
		_ = testEnd.Close()
	}()
	// Backend sender: writes 50 upstream payloads (pump B's source). Exits on
	// write error (upstreamEnd closed after HandleTerminal). Does NOT close
	// backendEnd itself — closing backendEnd would cause pump A's upstreamEnd.Write
	// to fail mid-stream, leaving the client writer blocked on testEnd.Write with
	// no reader. Instead, upstreamEnd.Close() (called below after HandleTerminal
	// returns) propagates the close to backendEnd and unblocks any pending write.
	go func() {
		defer wg.Done()
		for i := 0; i < writes; i++ {
			if _, err := backendEnd.Write([]byte(upPayload)); err != nil {
				return
			}
		}
		// No backendEnd.Close() here — upstreamEnd.Close() below terminates this.
	}()
	// Client reader: drains pump B's downstream writes (chainEnd→testEnd direction).
	// Exits when testEnd is closed by the client writer.
	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.Discard, testEnd)
	}()
	// Backend reader: drains pump A's upstream forwards (upstreamEnd→backendEnd direction).
	// REQUIRED: without this goroutine, pump A's upstreamEnd.Write has no reader and
	// blocks indefinitely — net.Pipe is unbuffered — deadlocking the test; see
	// PROGRESS.md Task 6 for the full analysis.
	// Exits when backendEnd is closed by the backend sender, or when upstreamEnd is
	// closed below (after HandleTerminal returns).
	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.Discard, backendEnd)
	}()

	// Phase 1: wait for HandleTerminal to return (both pumps have exited).
	// After HandleTerminal returns, close upstreamEnd so the backend sender and
	// backend reader (which may be blocked on backendEnd.Write / backendEnd.Read
	// respectively, waiting for a reader/writer on the upstreamEnd side) unblock
	// and exit cleanly.
	select {
	case <-done:
	case <-timer.C:
		t.Fatal("HandleTerminal did not return within the deadline (possible deadlock)")
	}
	_ = upstreamEnd.Close()

	// Phase 2: wait for all four driver goroutines to exit.
	trafficDone := make(chan struct{})
	go func() { wg.Wait(); close(trafficDone) }()
	select {
	case <-trafficDone:
	case <-timer.C:
		t.Fatal("traffic goroutines did not complete within the deadline after upstreamEnd closed")
	}

	// All downstream bytes were replayed to the filter exactly once.
	// Each testEnd.Write is SYNCHRONOUS (net.Pipe): it blocks until pump A's
	// chainEnd.Read happens. readChainConn.Read calls replayRead BEFORE returning
	// to io.Copy (§5.1 replay-before-return). So all 50 replays have completed
	// before testEnd.Close() is called — the count is deterministic.
	want := int64(writes * len(payload))
	if got := f.readBytes.Load(); got != want {
		t.Fatalf("filter saw %d downstream bytes via the replay, want %d", got, want)
	}
}

// haltingFilter is a synthetic MayHalt read filter for the seam tests: its OnData
// returns StopIteration exactly once per "armed" hold; an external goroutine calls
// cb.ContinueReading() to release. It models mongo's fault-delay shape WITHOUT the
// mongo dependency (the seam is independently testable — SPEC §12 / R-HALT).
type haltingFilter struct {
	Marker
	cb       ReadFilterCallbacks
	mayHalt  bool
	holdOnce bool // arm: StopIteration on the first OnData with bytes
	sawData  int
	released chan struct{}
}

func (f *haltingFilter) OnNewConnection() Status { return Continue }
func (f *haltingFilter) OnData(_ *Buffer, _ bool) Status {
	f.sawData++
	if f.holdOnce {
		f.holdOnce = false
		// Resume asynchronously from a separate goroutine (the time.AfterFunc shape),
		// but only on a haltable chain — a non-haltable StopIteration is a per-pass
		// stop that never holds, so spawning a resume goroutine there would leak
		// (nothing ever closes f.released on that path).
		if f.mayHalt {
			go func() { <-f.released; f.cb.ContinueReading() }()
		}
		return StopIteration
	}
	return Continue
}
func (f *haltingFilter) SetReadFilterCallbacks(cb ReadFilterCallbacks) { f.cb = cb }
func (f *haltingFilter) OnDestroy()                                    {}
func (f *haltingFilter) MayHalt() bool                                 { return f.mayHalt }

func TestChainHaltablePreHandoffHoldThenResume(t *testing.T) {
	hf := &haltingFilter{mayHalt: true, holdOnce: true, released: make(chan struct{})}
	rt := newChainRuntime([]ReadFilter{hf}, &fakeConn{}, connFacts{})
	if !rt.haltable {
		t.Fatal("chain with a MayHalt filter must be haltable")
	}
	rt.onNewConnection()
	// Drive OnData on a goroutine: it will HOLD (block in holdUntilResume) until released.
	done := make(chan struct{})
	go func() { rt.onData([]byte("PING"), false); close(done) }()
	// onData must NOT have returned yet (the hold blocks the dispatcher).
	select {
	case <-done:
		t.Fatal("onData returned before resume — the haltable hold did not block")
	case <-time.After(50 * time.Millisecond):
	}
	close(hf.released) // let the async ContinueReading fire
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("onData did not return after resume — the hold never released")
	}
	if rt.resumeIdx != 1 {
		t.Errorf("resumeIdx = %d, want 1 (advanced PAST the halting filter)", rt.resumeIdx)
	}
	if hf.sawData != 1 {
		t.Errorf("sawData = %d, want 1 (the halting filter is entered exactly once; resume advances PAST it, not re-delivering)", hf.sawData)
	}
}

// TestChainAsyncResumeReachesTerminalReady proves the async-active third context
// of ContinueReading (29.3 SPEC §3.1 extension 1): on a HALTABLE chain, an
// off-dispatch ContinueReading (from the haltingFilter's resume goroutine, the
// time.AfterFunc shape) releases the blocked dispatcher, which then advances
// resumeIdx PAST the last read filter — making the trailing terminal reachable.
// The production routing (the haltable releaseResume branch) already exists, so
// this test PASSES on first run; it is a live assertion that the pre-handoff
// async resume drives the chain to TerminalReady (R-HALT).
func TestChainAsyncResumeReachesTerminalReady(t *testing.T) {
	hf := &haltingFilter{mayHalt: true, holdOnce: true, released: make(chan struct{})}
	term := &recordingTerminal{} // the existing TerminalFilter double in upstreamcluster_test.go
	rt := NewChainRuntime([]NetworkFilter{hf, term}, &fakeConn{}, ConnFacts{})
	rt.OnNewConnection()
	if rt.TerminalReady() {
		t.Fatal("not ready before the read filter Continues past")
	}
	done := make(chan struct{})
	go func() { rt.OnData([]byte("Q1"), false); close(done) }()
	time.Sleep(30 * time.Millisecond) // let it reach the hold
	close(hf.released)                // async ContinueReading fires
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnData did not return after async resume — the hold never released")
	}
	if !rt.TerminalReady() {
		t.Fatal("after async resume the chain must be TerminalReady (resumeIdx past the last read filter)")
	}
}

func TestChainNonHaltableByteIdentity(t *testing.T) {
	// A MayHalt filter whose MayHalt()==false (mongo-no-delay) takes the byte-identical
	// pre-29.3 path: an OnData StopIteration is a PER-PASS stop (next read re-delivers),
	// NOT a hold; the halt mutex is never engaged.
	hf := &haltingFilter{mayHalt: false, holdOnce: true, released: make(chan struct{})}
	rt := newChainRuntime([]ReadFilter{hf}, &fakeConn{}, connFacts{})
	if rt.haltable {
		t.Fatal("MayHalt()==false must NOT make the chain haltable")
	}
	rt.onNewConnection()
	rt.onData([]byte("PING"), false) // must return immediately (per-pass stop, no block)
	if rt.resumeIdx != 0 {
		t.Errorf("resumeIdx = %d, want 0 (per-pass stop leaves resumeIdx at the filter)", rt.resumeIdx)
	}
}

func TestReplayReadPostHandoffHoldThenResume(t *testing.T) {
	hf := &haltingFilter{mayHalt: true, holdOnce: true, released: make(chan struct{})}
	rt := newChainRuntime([]ReadFilter{hf}, &fakeConn{}, connFacts{})
	// Simulate post-handoff: a replay parks at the hold; the pump must withhold.
	parked := rt.replayRead([]byte("Q2"), false)
	if !parked {
		t.Fatal("replayRead must report parked=true when a haltable filter holds")
	}
	done := make(chan struct{})
	go func() { rt.holdUntilResume(); close(done) }()
	select {
	case <-done:
		t.Fatal("holdUntilResume returned before resume")
	case <-time.After(40 * time.Millisecond):
	}
	close(hf.released)
	<-done // resume releases
}

func TestReplayReadNonHaltableObservational(t *testing.T) {
	// Non-haltable replay is byte-identical to pre-29.3: all filters observe, buffer
	// fully drained, parked=false always.
	b := &filterB{}
	rt := newChainRuntime([]ReadFilter{b}, &fakeConn{}, connFacts{})
	parked := rt.replayRead([]byte("hello"), false)
	if parked {
		t.Fatal("non-haltable replay never parks")
	}
	if rt.buf.Len() != 0 {
		t.Errorf("buffer not drained after observational replay: %d", rt.buf.Len())
	}
	if b.saw != "hello" {
		t.Errorf("filter did not observe the replayed bytes: %q", b.saw)
	}
}

// rearmingHaltingFilter is a MayHalt read filter that HOLDS on EVERY OnData (not
// just the first), each hold spawning its own async ContinueReading goroutine —
// the multi-hold shape TestSeamRaceReplayPublication needs to drive consecutive
// parked replay passes through the real pump cycle. It models a fault-delay that
// re-arms per request frame (the seam, not mongo specifically).
type rearmingHaltingFilter struct {
	Marker
	cb      ReadFilterCallbacks
	sawData int32
}

func (f *rearmingHaltingFilter) OnNewConnection() Status { return Continue }
func (f *rearmingHaltingFilter) OnData(_ *Buffer, endStream bool) Status {
	atomic.AddInt32(&f.sawData, 1)
	if endStream {
		// The final endStream replay (readChainConn.Read's EOF symmetry) is NOT a
		// request frame — observe and Continue, mirroring a real fault-delay that
		// only holds actual frames (the pump does not holdUntilResume on the EOF path).
		return Continue
	}
	// Hold on every byte-bearing pass; resume asynchronously (the time.AfterFunc
	// shape). Each hold is matched 1:1 by exactly one ContinueReading (the seam
	// contract).
	go f.cb.ContinueReading()
	return StopIteration
}
func (f *rearmingHaltingFilter) SetReadFilterCallbacks(cb ReadFilterCallbacks) { f.cb = cb }
func (f *rearmingHaltingFilter) OnDestroy()                                    {}
func (f *rearmingHaltingFilter) MayHalt() bool                                 { return true }

// nReadConn yields the SAME live payload n times from Read, then io.EOF. It feeds
// the post-handoff pump (readChainConn.Read) a real multi-read stream so each read
// drives one parked replayRead pass.
type nReadConn struct {
	fakeConn
	payload []byte
	n       int
	count   int
}

func (c *nReadConn) Read(b []byte) (int, error) {
	if c.count >= c.n {
		return 0, io.EOF
	}
	c.count++
	return copy(b, c.payload), nil
}

// TestSeamRaceReplayPublication is the R-HALT publication-edge proof. It drives the
// REAL post-handoff pump cycle — readChainConn.Read → replayRead (UNLOCKED writes
// to replayIdx/replayHeld/buf on the pump goroutine) → holdUntilResume → the async
// ContinueReading (haltMu-GUARDED reads/mutations of those SAME fields on the timer
// goroutine via finishParkedReplayLocked). Consecutive holds overlap: while a prior
// resume's ContinueReading may still be inside its haltMu section finishing the
// parked replay, the pump's next Read has already begun the next replayRead. The
// assertion is `go test -race` itself: the replay-state publication between the
// unlocked pump writes and the guarded resume reads must be fenced by haltMu.
//
// Regression coverage: removing the haltMu fence around the replay-state work in
// ContinueReading (finishParkedReplayLocked) is intended to be observable under
// -race here. (See the liveness note in PROGRESS.md for the empirical result of
// the deliberate-break check.)
func TestSeamRaceReplayPublication(t *testing.T) {
	const reads = 30
	for iter := 0; iter < 25; iter++ {
		hf := &rearmingHaltingFilter{}
		conn := &nReadConn{payload: []byte("FRAME"), n: reads}
		rt := newChainRuntime([]ReadFilter{hf}, conn, connFacts{})
		// readChainConn is the production post-handoff read wrap; its Read runs the
		// exact pump cycle (replayRead + holdUntilResume on park).
		rc := newReadChainConn(conn, rt)

		// Pump goroutine: loop Read until EOF, exactly as the terminal's downstream
		// pump does. Each non-EOF Read parks (rearming filter holds every pass) and
		// blocks in holdUntilResume until that pass's async ContinueReading releases.
		done := make(chan struct{})
		go func() {
			buf := make([]byte, 64)
			for {
				_, err := rc.Read(buf)
				if err != nil {
					close(done)
					return
				}
			}
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("iter %d: pump did not drain — a hold never released (possible deadlock)", iter)
		}
		// reads byte-bearing passes (each held + resumed) + 1 EOF-endStream pass.
		if got := atomic.LoadInt32(&hf.sawData); int(got) != reads+1 {
			t.Fatalf("iter %d: filter saw %d frames, want %d (reads held passes + 1 EOF pass)", iter, got, reads+1)
		}
	}
}

// TestChainCloseDirectionRecording exercises the setCloseDirection first-wins CAS
// through readChainConn.SetCloseDirection on a BARE readChainConn (29.3 Task 10,
// §3.5). The first recorded direction wins; a later opposite direction is ignored.
func TestChainCloseDirectionRecording(t *testing.T) {
	rt := newChainRuntime([]ReadFilter{&filterB{}}, &fakeConn{}, connFacts{})
	rc := newReadChainConn(&fakeConn{}, rt)
	rc.SetCloseDirection(CloseDirectionLocal)
	if got := rt.cb.CloseDirection(); got != CloseDirectionLocal {
		t.Errorf("CloseDirection = %v, want Local", got)
	}
	rc.SetCloseDirection(CloseDirectionRemote) // first-wins: stays Local
	if got := rt.cb.CloseDirection(); got != CloseDirectionLocal {
		t.Errorf("first-wins violated: %v", got)
	}
}

// TestCloseDirectionThroughWrapStack proves the setter reaches the chainRuntime
// through the ACTUAL handoff composition writeChainConn(prefixConn(readChainConn))
// — NOT just a bare readChainConn. The embedded net.Conn is an INTERFACE, so the
// custom SetCloseDirection does not auto-promote; the forwarding methods on
// prefixConn + writeChainConn (Task 10 Step 3) are what make this pass. WITHOUT
// them this test fails (the type-assert misses) — guarding against the
// silently-dead bug (risk register line 1845).
func TestCloseDirectionThroughWrapStack(t *testing.T) {
	rt := newChainRuntime([]ReadFilter{&filterB{}}, &fakeConn{}, connFacts{})
	inner := newReadChainConn(&fakeConn{}, rt)
	mid := newPrefixConn(inner, []byte("x"))         // prefix present
	outer := newWriteChainConn(mid, []WriteFilter{}) // the conn tcp_proxy gets
	sd, ok := net.Conn(outer).(interface{ SetCloseDirection(CloseDirection) })
	if !ok {
		t.Fatal("the handed-off conn must expose SetCloseDirection through the wrap stack")
	}
	sd.SetCloseDirection(CloseDirectionRemote)
	if got := rt.cb.CloseDirection(); got != CloseDirectionRemote {
		t.Errorf("CloseDirection through wrap stack = %v, want Remote", got)
	}
}

// TestCloseDirectionThroughWrapStackNoPrefix is the no-prefix composition variant:
// handleTerminal omits the prefixConn when the buffer is empty, so tcp_proxy gets
// writeChainConn(readChainConn) directly. The forwarding on writeChainConn must
// still relay SetCloseDirection inward to the readChainConn (29.3 Task 10).
func TestCloseDirectionThroughWrapStackNoPrefix(t *testing.T) {
	rt := newChainRuntime([]ReadFilter{&filterB{}}, &fakeConn{}, connFacts{})
	inner := newReadChainConn(&fakeConn{}, rt)
	outer := newWriteChainConn(inner, []WriteFilter{}) // no prefixConn (empty buffer)
	sd, ok := net.Conn(outer).(interface{ SetCloseDirection(CloseDirection) })
	if !ok {
		t.Fatal("the handed-off conn must expose SetCloseDirection (no-prefix stack)")
	}
	sd.SetCloseDirection(CloseDirectionLocal)
	if got := rt.cb.CloseDirection(); got != CloseDirectionLocal {
		t.Errorf("CloseDirection through no-prefix wrap stack = %v, want Local", got)
	}
}
