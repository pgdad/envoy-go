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
