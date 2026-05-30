package network

import (
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
)

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

func TestChainOnDestroyCallsAllFilters(t *testing.T) {
	a, b := &destroyFilter{}, &destroyFilter{}
	rt := newChainRuntime([]ReadFilter{a, b}, &fakeConn{}, connFacts{})
	rt.onDestroy()
	if !a.destroyed || !b.destroyed {
		t.Errorf("OnDestroy not called on all filters: a=%v b=%v", a.destroyed, b.destroyed)
	}
}

type destroyFilter struct{ destroyed bool }

func (f *destroyFilter) OnNewConnection() Status                    { return Continue }
func (f *destroyFilter) OnData(*Buffer, bool) Status                { return Continue }
func (f *destroyFilter) SetReadFilterCallbacks(ReadFilterCallbacks) {}
func (f *destroyFilter) OnDestroy()                                 { f.destroyed = true }
