package network

import (
	"errors"
	"net"
	"testing"
)

// recordingConn is a net.Conn double recording Write payloads; failErr forces
// Write to fail (error-propagation test).
type recordingConn struct {
	net.Conn // nil; only Write is exercised
	writes   [][]byte
	failErr  error
}

func (r *recordingConn) Write(p []byte) (int, error) {
	if r.failErr != nil {
		return 0, r.failErr
	}
	cp := make([]byte, len(p))
	copy(cp, p)
	r.writes = append(r.writes, cp)
	return len(p), nil
}

// mutatingWriteFilter appends "XYZ" to the buffer in OnWrite, returns Continue.
type mutatingWriteFilter struct {
	Marker
}

func (m *mutatingWriteFilter) OnWrite(buf *Buffer, _ bool) Status {
	buf.Append([]byte("XYZ"))
	return Continue
}
func (m *mutatingWriteFilter) SetWriteFilterCallbacks(_ WriteFilterCallbacks) {}
func (m *mutatingWriteFilter) OnDestroy()                                     {}

// Compile-time assertion: mutatingWriteFilter must satisfy WriteFilter.
var _ WriteFilter = (*mutatingWriteFilter)(nil)

// endStreamRecorder records the endStream bool argument into *sink, returns Continue.
type endStreamRecorder struct {
	Marker
	sink *[]bool
}

func (e *endStreamRecorder) OnWrite(_ *Buffer, endStream bool) Status {
	*e.sink = append(*e.sink, endStream)
	return Continue
}
func (e *endStreamRecorder) SetWriteFilterCallbacks(_ WriteFilterCallbacks) {}
func (e *endStreamRecorder) OnDestroy()                                     {}

// Compile-time assertion: endStreamRecorder must satisfy WriteFilter.
var _ WriteFilter = (*endStreamRecorder)(nil)

// All-Continue chain: bytes forward to the inner conn; Write reports (len(p), nil).
func TestWriteChainConnForwards(t *testing.T) {
	inner := &recordingConn{}
	a := &fakeWriteFilter{name: "a", status: Continue}
	w := newWriteChainConn(inner, []WriteFilter{a})
	n, err := w.Write([]byte("hello"))
	if n != 5 || err != nil {
		t.Fatalf("Write = (%d, %v), want (5, nil)", n, err)
	}
	if len(inner.writes) != 1 || string(inner.writes[0]) != "hello" {
		t.Fatalf("inner writes = %q, want [hello]", inner.writes)
	}
	if len(a.calls) != 1 {
		t.Fatalf("OnWrite calls = %d, want 1", len(a.calls))
	}
}

// StopIteration: nothing reaches the inner conn; Write STILL reports (len(p), nil) — D-P7.
func TestWriteChainConnStopIterationNoForward(t *testing.T) {
	inner := &recordingConn{}
	stop := &fakeWriteFilter{name: "stop", status: StopIteration}
	after := &fakeWriteFilter{name: "after", status: Continue}
	w := newWriteChainConn(inner, []WriteFilter{stop, after})
	n, err := w.Write([]byte("dropme"))
	if n != 6 || err != nil {
		t.Fatalf("Write = (%d, %v), want (6, nil) — D-P7 chain-stop reports success", n, err)
	}
	if len(inner.writes) != 0 {
		t.Fatalf("inner received %q, want nothing (no-forward parity)", inner.writes)
	}
	if len(after.calls) != 0 {
		t.Fatal("filters after the stopping filter must NOT be invoked")
	}
}

// Dispatch order: writeChainConn iterates its slice front-to-back (the slice IS
// dispatch order — handleTerminal reverses chain order before constructing).
// Use a SHARED order recorder so strict ordering (a before b) is asserted.
func TestWriteChainConnDispatchOrder(t *testing.T) {
	var order []string
	mk := func(name string) *fakeWriteFilter {
		return &fakeWriteFilter{name: name, status: Continue, order: &order}
	}
	a, b := mk("a"), mk("b")
	w := newWriteChainConn(&recordingConn{}, []WriteFilter{a, b})
	_, _ = w.Write([]byte("x"))
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("dispatch order = %v, want [a b] (front-to-back over the dispatch slice)", order)
	}
}

// Post-chain bytes: a mutating write filter's output is what reaches the inner
// conn (upstream parity: the transport sees the filtered buffer); the Write
// return value still reports len(p) of the ORIGINAL payload.
func TestWriteChainConnForwardsPostChainBytes(t *testing.T) {
	inner := &recordingConn{}
	mut := &mutatingWriteFilter{}
	w := newWriteChainConn(inner, []WriteFilter{mut})
	n, err := w.Write([]byte("abc"))
	if n != 3 || err != nil {
		t.Fatalf("Write = (%d, %v), want (3, nil)", n, err)
	}
	if len(inner.writes) != 1 || string(inner.writes[0]) != "abcXYZ" {
		t.Fatalf("inner writes = %q, want [abcXYZ] (post-chain buffer)", inner.writes)
	}
}

// Underlying-write errors propagate as (0, err).
func TestWriteChainConnUnderlyingErrorPropagates(t *testing.T) {
	wantErr := errors.New("boom")
	inner := &recordingConn{failErr: wantErr}
	w := newWriteChainConn(inner, []WriteFilter{&fakeWriteFilter{name: "a", status: Continue}})
	n, err := w.Write([]byte("x"))
	if n != 0 || !errors.Is(err, wantErr) {
		t.Fatalf("Write = (%d, %v), want (0, boom)", n, err)
	}
}

// endStream is always false at 28.1 (net.Conn.Write carries no half-close signal).
func TestWriteChainConnEndStreamAlwaysFalse(t *testing.T) {
	got := make([]bool, 0, 1)
	rec := &endStreamRecorder{sink: &got}
	w := newWriteChainConn(&recordingConn{}, []WriteFilter{rec})
	_, _ = w.Write([]byte("x"))
	if len(got) != 1 || got[0] != false {
		t.Fatalf("endStream values = %v, want [false]", got)
	}
}
