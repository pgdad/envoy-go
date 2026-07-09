package statssink

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

// fakeConn is a net.Conn test double that records every Write and can be told to
// accept only n bytes then error (Task 7), or to block until Close (Task 9).
type fakeConn struct {
	mu       sync.Mutex
	writes   [][]byte
	closed   bool
	acceptN  int   // 0 ⇒ accept everything
	errAfter error // non-nil ⇒ return (acceptN, errAfter)
}

func (c *fakeConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, errors.New("fakeConn: write on closed conn")
	}
	if c.errAfter != nil {
		n := c.acceptN
		if n > len(b) {
			n = len(b)
		}
		c.writes = append(c.writes, append([]byte(nil), b[:n]...))
		return n, c.errAfter
	}
	c.writes = append(c.writes, append([]byte(nil), b...))
	return len(b), nil
}

func (c *fakeConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *fakeConn) written() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, len(c.writes))
	copy(out, c.writes)
	return out
}

// isClosed reports whether Close has been called, under the same mutex Write
// and Close use — race-safe.
func (c *fakeConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *fakeConn) Read([]byte) (int, error)         { return 0, errors.New("unused") }
func (c *fakeConn) LocalAddr() net.Addr              { return nil }
func (c *fakeConn) RemoteAddr() net.Addr             { return nil }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

// compile-time seam check
var _ net.Conn = (*fakeConn)(nil)
var _ Sink = (*TCPStatsdSink)(nil)

// waitWrites polls until conn has recorded at least n writes.
func waitWrites(t *testing.T, c *fakeConn, n int) [][]byte {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		w := c.written()
		if len(w) >= n {
			return w
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %d writes, want >= %d", len(w), n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// waitStableWriteCount polls conn.written() until its length has stopped
// growing for a short window, i.e. the writer has fully drained whatever the
// channel had buffered and is blocked waiting for the next Submit.
func waitStableWriteCount(t *testing.T, c *fakeConn) int {
	t.Helper()
	const stableWindow = 50 * time.Millisecond
	deadline := time.Now().Add(3 * time.Second)
	last := -1
	stableSince := time.Now()
	for {
		n := len(c.written())
		if n != last {
			last = n
			stableSince = time.Now()
		} else if time.Since(stableSince) >= stableWindow {
			return n
		}
		if time.Now().After(deadline) {
			t.Fatalf("write count never stabilized (stuck at %d)", n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// parseCounterDelta finds the "<name>:<value>|c" line inside one recorded
// write and returns its numeric value.
func parseCounterDelta(write []byte, name string) (float64, bool) {
	prefix := name + ":"
	for _, line := range strings.Split(strings.TrimSuffix(string(write), "\n"), "\n") {
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "|c") {
			continue
		}
		numStr := strings.TrimSuffix(strings.TrimPrefix(line, prefix), "|c")
		v, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			continue
		}
		return v, true
	}
	return 0, false
}

// sumCounterDeltas sums every "<name>:<value>|c" line's value across a set of
// recorded writes.
func sumCounterDeltas(writes [][]byte, name string) float64 {
	var sum float64
	for _, w := range writes {
		if v, ok := parseCounterDelta(w, name); ok {
			sum += v
		}
	}
	return sum
}

func TestTCPStatsdSink_DialsLazilyOnFirstFlush(t *testing.T) {
	dials := 0
	conn := &fakeConn{}
	var mu sync.Mutex
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		dials++
		mu.Unlock()
		return conn, nil
	}, "p")

	mu.Lock()
	d := dials
	mu.Unlock()
	if d != 0 {
		t.Fatalf("dials before first Submit = %d, want 0 (lazy dial)", d)
	}

	s.Submit([]*dto.MetricFamily{counterFam("a.b", 3)})
	waitWrites(t, conn, 1)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if dials != 1 {
		t.Fatalf("dials = %d, want exactly 1 (one long-lived connection)", dials)
	}
}

// TestTCPStatsdSink_SubmitNeverBlocks: the Flusher calls Submit SERIALLY across
// all sinks from ONE goroutine (flusher.go:46-51). A blocking TCP sink would
// starve every sibling sink. Submit must return even with no writer draining.
func TestTCPStatsdSink_SubmitNeverBlocks(t *testing.T) {
	block := make(chan struct{})
	s := NewTCPStatsdSink(func(ctx context.Context) (net.Conn, error) {
		<-block // the writer is parked in dial forever
		return nil, errors.New("unreachable")
	}, "p")
	defer func() { close(block); _ = s.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Far more than defaultChannelCapacity (8): the excess must be DROPPED,
		// not block.
		for i := 0; i < 100; i++ {
			s.Submit([]*dto.MetricFamily{counterFam("a.b", 1)})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Submit blocked; it must drop on a full channel")
	}
}

// TestTCPStatsdSink_EnqueueDropDoesNotLatchDelta pins ADR-0263: delta.apply runs
// in the WRITER, not in Submit, so a batch dropped at the channel never latches
// deltaState — the dropped increments ride the NEXT enqueued flush.
func TestTCPStatsdSink_EnqueueDropDoesNotLatchDelta(t *testing.T) {
	release := make(chan struct{})
	conn := &fakeConn{}
	first := true
	var mu sync.Mutex
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		f := first
		first = false
		mu.Unlock()
		if f {
			<-release // park the writer inside its first dial
		}
		return conn, nil
	}, "p")

	// Fill the channel far past capacity while the writer is parked; all but the
	// buffered few are dropped.
	for i := 0; i < 50; i++ {
		s.Submit([]*dto.MetricFamily{counterFam("a.b", float64(i+1))})
	}
	close(release)
	waitWrites(t, conn, 1)

	// The FIRST line the writer ever emits must carry the absolute value of the
	// first batch it actually dequeued (delta against an empty deltaState:
	// 1-0=1) — never a delta against a batch that was dropped before reaching
	// the writer.
	firstWrite := conn.written()[0]
	if got, ok := parseCounterDelta(firstWrite, "p.a.b"); !ok || got != 1 {
		t.Fatalf("first write %q: a.b delta = %v (ok=%v), want 1", firstWrite, got, ok)
	}

	// Let the writer fully drain whatever the channel buffered (up to
	// defaultChannelCapacity batches, all consecutive integers starting at 1, so
	// every one of their deltas is exactly 1) before injecting a known sentinel.
	drained := waitStableWriteCount(t, conn)
	priorSum := sumCounterDeltas(conn.written()[:drained], "p.a.b")

	const sentinel = 1_000_000.0
	s.Submit([]*dto.MetricFamily{counterFam("a.b", sentinel)})
	final := waitWrites(t, conn, drained+1)
	_ = s.Close()

	// ADR-0263's property, made concrete: the 41+ increments dropped at the full
	// channel (values drained+1..50 — NEVER seen by the writer) must not have
	// latched deltaState. The sentinel's delta must be taken against the LAST
	// absolute value the writer actually DEQUEUED (priorSum, which telescopes to
	// exactly that value: every dequeued batch here contributed a delta of 1) —
	// NOT against 50, the last value merely SUBMITTED. If delta.apply ran in
	// Submit instead of the writer, every one of the 50 Submits — including the
	// ones dropped at the full channel — would already have latched deltaState
	// past this point, and the sentinel's delta would come out wrong (computed
	// against ~50, not against priorSum).
	gotDelta, ok := parseCounterDelta(final[len(final)-1], "p.a.b")
	if !ok {
		t.Fatalf("sentinel write %q: no parseable p.a.b counter line", final[len(final)-1])
	}
	wantDelta := sentinel - priorSum
	if gotDelta != wantDelta {
		t.Fatalf("sentinel delta = %v, want %v (sentinel %v - priorSum %v the writer actually dequeued): a dropped batch latched deltaState", gotDelta, wantDelta, sentinel, priorSum)
	}
}

// TestTCPStatsdSink_LinesAreNewlineTERMINATED pins D-TCP-LINE, adopted verbatim
// from the reference (reference_wire_format_both_sides_see_same_bytes): EVERY
// line, INCLUDING THE LAST OF A FLUSH, ends with '\n'. No write contains "\n\n".
func TestTCPStatsdSink_LinesAreNewlineTERMINATED(t *testing.T) {
	conn := &fakeConn{}
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) { return conn, nil }, "sdpfx")
	s.Submit([]*dto.MetricFamily{counterFam("a.b", 3), gaugeFam("g.h", 9)})
	waitWrites(t, conn, 1)
	_ = s.Close()

	w := conn.written()[0]
	if len(w) == 0 || w[len(w)-1] != '\n' {
		t.Fatalf("write %q must be '\\n'-TERMINATED (not separated)", w)
	}
	if bytes.Contains(w, []byte("\n\n")) {
		t.Fatalf("write %q contains a blank line", w)
	}
	got := strings.Split(strings.TrimSuffix(string(w), "\n"), "\n")
	want := []string{"sdpfx.a.b:3|c", "sdpfx.g.h:9|g"}
	sameSet(t, got, want)
}

// TestTCPStatsdSink_CounterDeltaGaugeAbsolute pins D-TCP-DELTA.
func TestTCPStatsdSink_CounterDeltaGaugeAbsolute(t *testing.T) {
	conn := &fakeConn{}
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) { return conn, nil }, "sdpfx")

	s.Submit([]*dto.MetricFamily{counterFam("a.b", 2), gaugeFam("g.h", 5)})
	waitWrites(t, conn, 1)
	s.Submit([]*dto.MetricFamily{counterFam("a.b", 7), gaugeFam("g.h", 5)})
	waitWrites(t, conn, 2)
	s.Submit([]*dto.MetricFamily{counterFam("a.b", 7), gaugeFam("g.h", 5)})
	waitWrites(t, conn, 3)
	_ = s.Close()

	w := conn.written()
	// COUNTER: per-flush DELTA. 2 → +5 → +0. A ZERO-delta counter is STILL emitted.
	assertContains(t, w[0], "sdpfx.a.b:2|c")
	assertContains(t, w[1], "sdpfx.a.b:5|c")
	assertContains(t, w[2], "sdpfx.a.b:0|c")
	// GAUGE: ABSOLUTE, constant.
	for i := range w {
		assertContains(t, w[i], "sdpfx.g.h:5|g")
	}
}

func assertContains(t *testing.T, haystack []byte, needle string) {
	t.Helper()
	if !bytes.Contains(haystack, []byte(needle+"\n")) {
		t.Errorf("write %q missing terminated line %q", haystack, needle)
	}
}

// TestTCPStatsdSink_LineAlignedResume is the AMEND-TCP-RESUME proof. Conn #1
// accepts a byte count that lands MID-LINE and then errors; conn #2 receives the
// remainder. The delivered line MULTISET across both conns must equal the emitted
// multiset EXACTLY — no loss, no duplication.
func TestTCPStatsdSink_LineAlignedResume(t *testing.T) {
	// Three lines, deterministic order via a single-family batch per name is not
	// possible (emitStatsdLines walks the batch slice in order), so we control
	// order by the batch slice order.
	batch := []*dto.MetricFamily{
		counterFam("aaa", 1), // "sdpfx.aaa:1|c\n"  → 14 bytes
		counterFam("bbb", 2), // "sdpfx.bbb:2|c\n"  → 14 bytes
		counterFam("ccc", 3), // "sdpfx.ccc:3|c\n"  → 14 bytes
	}
	const line0 = "sdpfx.aaa:1|c\n"
	const line1 = "sdpfx.bbb:2|c\n"
	const line2 = "sdpfx.ccc:3|c\n"
	// Accept line0 in full plus 5 bytes of line1 ("sdpfx"), then error.
	acceptN := len(line0) + 5

	c1 := &fakeConn{acceptN: acceptN, errAfter: errors.New("peer reset")}
	c2 := &fakeConn{}
	var mu sync.Mutex
	dialN := 0
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		dialN++
		if dialN == 1 {
			return c1, nil
		}
		return c2, nil
	}, "sdpfx")

	s.Submit(batch)
	waitWrites(t, c1, 1)
	// Second flush: the writer redials and sends the retained suffix + the new
	// (zero-delta) lines.
	s.Submit(batch)
	waitWrites(t, c2, 1)
	_ = s.Close()

	// (i) conn #1 received line0 in full and a 5-byte fragment of line1. This one
	// stays Fatalf: if the harness didn't even set up conn#1's bytes as expected,
	// every assertion below is meaningless and continuing would just produce
	// noise. (ii)-(v) each pin a DISTINCT property of the resume behavior (whole
	// re-send, no duplication, exactly-once delivery, no loss), and the
	// deliberate-break protocol requires each to be independently observable —
	// so they use Errorf, not Fatalf: a failure in one must not hide whether the
	// others also failed or passed.
	got1 := string(c1.written()[0])
	if got1 != line0+"sdpfx" {
		t.Fatalf("conn#1 got %q, want %q", got1, line0+"sdpfx")
	}
	// (ii) conn #2's FIRST bytes are line1 re-sent WHOLE — not its 9-byte tail.
	got2 := string(c2.written()[0])
	if !strings.HasPrefix(got2, line1) {
		t.Errorf("(ii) whole-resend: conn#2 first bytes = %q; the straddling line %q must be re-sent WHOLE", got2, line1)
	}
	// (iii) line0 is NOT re-sent: a complete line that landed is never duplicated.
	if strings.Contains(got2, line0) {
		t.Errorf("(iii) no-duplication: conn#2 %q re-sent the already-delivered line %q", got2, line0)
	}
	// (iv) the straddling line is present exactly once across BOTH conns' COMPLETE
	//      lines: conn#1's trailing fragment is discarded by the receiver at EOF.
	if strings.Count(completeLines(got1)+completeLines(got2), line1) != 1 {
		t.Errorf("(iv) exactly-once: line1 must appear exactly once among complete lines, got1=%q got2=%q", got1, got2)
	}
	// (v) line2 (never written to conn#1) survives.
	if !strings.Contains(got2, line2) {
		t.Errorf("(v) no-loss: conn#2 %q lost line2 %q", got2, line2)
	}
}

// completeLines drops an unterminated trailing line, exactly as the receiver's
// stream parser does at EOF.
func completeLines(s string) string {
	i := strings.LastIndexByte(s, '\n')
	if i < 0 {
		return ""
	}
	return s[:i+1]
}

// TestTCPStatsdSink_WriteErrorAtBoundaryRetainsNothing: n lands exactly on a '\n'.
func TestTCPStatsdSink_WriteErrorAtBoundaryRetainsNothing(t *testing.T) {
	c1 := &fakeConn{acceptN: len("sdpfx.aaa:1|c\n"), errAfter: errors.New("reset")}
	c2 := &fakeConn{}
	var mu sync.Mutex
	n := 0
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return c1, nil
		}
		return c2, nil
	}, "sdpfx")
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 1), counterFam("bbb", 2)})
	waitWrites(t, c1, 1)
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 1), counterFam("bbb", 2)})
	waitWrites(t, c2, 1)
	_ = s.Close()

	if got := string(c2.written()[0]); strings.Contains(got, "sdpfx.aaa:1|c") {
		t.Fatalf("conn#2 %q re-sent a line that landed exactly at the boundary", got)
	}
}

// TestTCPStatsdSink_ZeroBytesWrittenRetainsEverything: n == 0 ⇒ nothing landed.
func TestTCPStatsdSink_ZeroBytesWrittenRetainsEverything(t *testing.T) {
	c1 := &fakeConn{acceptN: 0, errAfter: errors.New("reset")}
	c2 := &fakeConn{}
	var mu sync.Mutex
	n := 0
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return c1, nil
		}
		return c2, nil
	}, "sdpfx")
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 1)})
	waitWrites(t, c1, 1)
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 1)})
	waitWrites(t, c2, 1)
	_ = s.Close()

	if got := string(c2.written()[0]); !strings.Contains(got, "sdpfx.aaa:1|c\n") {
		t.Fatalf("conn#2 %q lost the line that was never written (n==0 must retain everything)", got)
	}
}

// TestTCPStatsdSink_DialFailureRetainsPending: a failed dial must not lose bytes.
func TestTCPStatsdSink_DialFailureRetainsPending(t *testing.T) {
	c := &fakeConn{}
	var mu sync.Mutex
	n := 0
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return nil, errors.New("connection refused")
		}
		return c, nil
	}, "sdpfx")
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 4)}) // dial fails; pending retained
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 4)}) // dial succeeds; delta 0
	waitWrites(t, c, 1)
	_ = s.Close()

	got := string(c.written()[0])
	if !strings.Contains(got, "sdpfx.aaa:4|c\n") {
		t.Fatalf("the first flush's line was lost across a dial failure: %q", got)
	}
}

// blockingConn parks in Write until Close is called. This is the D-TCP-CLOSE
// hazard made deterministic: a real socket only blocks once its send buffer
// fills, which is timing-dependent and flaky.
type blockingConn struct {
	entered   chan struct{} // closed on the first Write
	unblock   chan struct{} // closed by Close
	closeOnce sync.Once
	enterOnce sync.Once
}

func newBlockingConn() *blockingConn {
	return &blockingConn{entered: make(chan struct{}), unblock: make(chan struct{})}
}

func (c *blockingConn) Write(b []byte) (int, error) {
	c.enterOnce.Do(func() { close(c.entered) })
	<-c.unblock // parks until Close
	return 0, errors.New("blockingConn: closed while writing")
}

func (c *blockingConn) Close() error {
	c.closeOnce.Do(func() { close(c.unblock) })
	return nil
}

func (c *blockingConn) Read([]byte) (int, error)         { return 0, errors.New("unused") }
func (c *blockingConn) LocalAddr() net.Addr              { return nil }
func (c *blockingConn) RemoteAddr() net.Addr             { return nil }
func (c *blockingConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingConn) SetWriteDeadline(time.Time) error { return nil }

var _ net.Conn = (*blockingConn)(nil)

// TestTCPStatsdSink_CloseUnwedgesBlockedWrite is the D-TCP-CLOSE PROOF. It must
// be run under -race: the writer goroutine holds `conn` and is parked inside
// Write; Close reaches that conn from the caller's goroutine and hard-closes it.
func TestTCPStatsdSink_CloseUnwedgesBlockedWrite(t *testing.T) {
	bc := newBlockingConn()
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) { return bc, nil }, "sdpfx")

	s.Submit([]*dto.MetricFamily{counterFam("a.b", 1)})
	select {
	case <-bc.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the writer never entered Write")
	}

	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(closeDrainGrace + 3*time.Second):
		t.Fatal("Close did not return: the blocked Write was never unwedged " +
			"(cancel() cannot interrupt a raw net.Conn — conn.Close() must)")
	}
	if elapsed := time.Since(start); elapsed < closeDrainGrace {
		t.Fatalf("Close returned in %v, before the %v drain grace; it must WAIT for the drain first", elapsed, closeDrainGrace)
	}
}

// TestTCPStatsdSink_CloseIsIdempotent asserts a second Close is a harmless no-op.
func TestTCPStatsdSink_CloseIsIdempotent(t *testing.T) {
	conn := &fakeConn{}
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) { return conn, nil }, "sdpfx")
	s.Submit([]*dto.MetricFamily{counterFam("a.b", 1)})
	waitWrites(t, conn, 1)
	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// waitClosedLatched polls (via the sink's own connMu — the same lock the
// production code uses to guard `closed`, so this is race-safe, not an
// unguarded peek) until s.closed is true. Deadline is closeDrainGrace plus
// slack: with the writer parked inside dial, Close cannot reach s.done and so
// MUST take its closeDrainGrace timeout branch — the only path that can set
// closed=true before the dial returns.
func waitClosedLatched(t *testing.T, s *TCPStatsdSink) {
	t.Helper()
	deadline := time.Now().Add(closeDrainGrace + 3*time.Second)
	for {
		s.connMu.Lock()
		c := s.closed
		s.connMu.Unlock()
		if c {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("s.closed never latched (Close's closeDrainGrace timeout branch never fired)")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestTCPStatsdSink_CloseNeitherWritesToNorLeaksARacingRedial is the proof for
// the plan's third deliberate break: drop `if s.closed { ...; return false }`
// from redial. An earlier version of this test asserted only "the racing conn
// ends up closed" and PASSED 5/5 under that break — vacuous, because run's
// unconditional `defer s.markClosedAndCloseConn` closes whatever conn is
// sitting in s.conn when the writer goroutine exits, REGARDLESS of whether the
// guard fired. That assertion cannot tell a guarded redial from an unguarded
// one.
//
// The guard's real, observable effect is upstream of Close: it decides whether
// a connection DIALED AFTER Close already latched `closed` gets WRITTEN TO.
// Without the guard, redial stashes the fresh conn into s.conn and returns
// true, and flush's very next statement is a real conn.Write(s.pending) on it.
// With the guard, redial closes that fresh conn itself and returns false, and
// flush returns before ever calling Write. So the discriminating assertion is
// (1) len(dialedConn.written()) == 0.
//
// For (1) to mean anything, `closed` must already be true at the instant
// redial's dial call returns — otherwise `if s.closed` reads false under
// EITHER the guarded or unguarded code, and a write happens regardless of the
// guard. Since the writer is parked inside dial, Close's only route to
// closed=true before the dial returns is its closeDrainGrace timeout branch,
// so the test waits (waitClosedLatched, no sleep-as-synchronization) for that
// to fire before releasing the dial.
//
// Assertion (2), that dialedConn ends up closed, is kept and is a real
// property (a conn must never be leaked un-closed) — but it is NOT proof of
// the guard, for the reason above: run's deferred markClosedAndCloseConn (or,
// in the guarded path, redial's own close-and-discard) closes it either way.
func TestTCPStatsdSink_CloseNeitherWritesToNorLeaksARacingRedial(t *testing.T) {
	gate := make(chan struct{})
	dialing := make(chan struct{})
	var dialingOnce sync.Once
	dialedConn := &fakeConn{}
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		dialingOnce.Do(func() { close(dialing) })
		<-gate // hold the writer inside dial until `closed` has latched
		return dialedConn, nil
	}, "sdpfx")

	s.Submit([]*dto.MetricFamily{counterFam("a.b", 1)})
	select {
	case <-dialing:
	case <-time.After(2 * time.Second):
		t.Fatal("the writer never entered dial")
	}

	closeDone := make(chan struct{})
	go func() { defer close(closeDone); _ = s.Close() }()

	// Deterministically wait for closed=true BEFORE releasing the dial: see the
	// doc comment above for why this is required for assertion (1) to
	// discriminate at all.
	waitClosedLatched(t, s)

	close(gate) // dial now returns a conn dialed AFTER closed was already true
	<-closeDone

	// (1) THE discriminating assertion: a conn dialed after Close latched must
	// never be written to.
	if got := dialedConn.written(); len(got) != 0 {
		t.Fatalf("a conn dialed after Close latched was WRITTEN TO (%d writes: %q): "+
			"the closed guard in redial did not prevent the write", len(got), got)
	}
	// (2) A real property (no leaked conn), but NOT proof of the guard: see the
	// doc comment above — run's deferred markClosedAndCloseConn closes whatever
	// ends up in s.conn regardless of whether the guard fired.
	if !dialedConn.isClosed() {
		t.Fatal("a conn dialed during/after Close was not closed: leaked")
	}
}

// TestTCPStatsdSink_CloseDrainsQueuedBatches: a clean Close (no blocked write)
// must flush what is already queued before returning.
func TestTCPStatsdSink_CloseDrainsQueuedBatches(t *testing.T) {
	conn := &fakeConn{}
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) { return conn, nil }, "sdpfx")
	for i := 0; i < 3; i++ {
		s.Submit([]*dto.MetricFamily{counterFam("a.b", float64(i+1))})
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if n := len(conn.written()); n < 1 {
		t.Fatalf("Close returned with %d writes; queued batches must drain", n)
	}
}

func TestDropOldestLines(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		capBytes int
		want     string
	}{
		{"under cap: untouched", "aa\nbb\n", 100, "aa\nbb\n"},
		{"exactly at cap: untouched", "aa\nbb\n", 6, "aa\nbb\n"},
		{"drops the oldest whole line", "aa\nbb\n", 5, "bb\n"},
		{"rounds UP to the next boundary, never splits a line", "aaaa\nbb\n", 6, "bb\n"},
		{"off lands exactly on a boundary: drop no extra line", "aa\nbb\n", 3, "bb\n"},
		{"drops several lines", "a\nb\nc\nd\n", 3, "d\n"},
		{"no boundary at/after off: drop all", "aaaaaaaa\n", 3, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(dropOldestLines([]byte(tc.in), tc.capBytes))
			if got != tc.want {
				t.Errorf("dropOldestLines(%q, %d) = %q, want %q", tc.in, tc.capBytes, got, tc.want)
			}
			if len(got) > tc.capBytes {
				t.Errorf("dropOldestLines(%q, %d) = %q, len %d exceeds capBytes %d", tc.in, tc.capBytes, got, len(got), tc.capBytes)
			}
			// START boundary, independent of `want`: got must be a suffix of the
			// original input, and — when got is a non-empty, proper suffix — the
			// byte immediately preceding it in `in` must be '\n'. That is exactly
			// "the result begins at a line boundary of the original buffer", the
			// property a wrong front-cut (off-by-one, wrong scan direction,
			// splitting a line) violates. It fires even if `want` itself were
			// miscomputed, unlike the dead "ends in '\n'" check this replaces
			// (dropOldestLines only ever cuts the FRONT, so the tail is untouched
			// by construction and that check could never fail).
			if !strings.HasSuffix(tc.in, got) {
				t.Errorf("dropOldestLines(%q, %d) = %q is not a suffix of the input", tc.in, tc.capBytes, got)
			} else if got != "" && len(got) < len(tc.in) {
				precedingIdx := len(tc.in) - len(got) - 1
				if tc.in[precedingIdx] != '\n' {
					t.Errorf("dropOldestLines(%q, %d) = %q does not begin at a '\\n' boundary of the input (preceding byte %q)", tc.in, tc.capBytes, got, tc.in[precedingIdx])
				}
			}
		})
	}
}

// TestTCPStatsdSink_PendingIsBounded: with a dead dial, pending must never
// exceed maxPendingBytes, and every line it retains after a drop must be a
// complete, well-formed statsd line — never a fragment left by a front-cut
// that landed mid-line.
func TestTCPStatsdSink_PendingIsBounded(t *testing.T) {
	s := newTCPStatsdSinkNoRun(func(context.Context) (net.Conn, error) {
		return nil, errors.New("always refused")
	}, "sdpfx")

	// Drive the writer directly: Submit would drop past the cap-8 channel.
	big := make([]*dto.MetricFamily, 0, 200)
	for i := 0; i < 200; i++ {
		big = append(big, counterFam(fmt.Sprintf("fam.%04d", i), float64(i)))
	}
	for i := 0; i < 2000; i++ {
		s.flush(big) // same goroutine: `pending` is writer-owned, so this is legal in-test
		if len(s.pending) > maxPendingBytes {
			t.Fatalf("pending grew to %d bytes, exceeding maxPendingBytes=%d", len(s.pending), maxPendingBytes)
		}
		assertPendingWellFormed(t, s.pending)
	}
	if len(s.pending) == 0 {
		t.Fatal("pending is empty; the test never reached the cap and proves nothing")
	}
}

// assertPendingWellFormed asserts every line in pending (after trimming the
// single trailing '\n') is a complete "name:value|type" statsd line — it
// contains a ':' followed later by a '|'. This is the property that actually
// protects the wire: a front-cut landing mid-line produces a malformed first
// line (e.g. "am.0007:7|c" missing its leading bytes, or just "7|c" missing
// its name and ':' entirely), which this catches. Replaces the old "pending
// ends on a line boundary" check, which was dead — dropOldestLines only ever
// cuts the FRONT, so the tail is untouched by construction and that check
// could never fail, even under a completely wrong front-cut.
func assertPendingWellFormed(t *testing.T, pending []byte) {
	t.Helper()
	if len(pending) == 0 {
		return
	}
	trimmed := strings.TrimSuffix(string(pending), "\n")
	for _, line := range strings.Split(trimmed, "\n") {
		if line == "" {
			t.Fatalf("pending %q contains a blank line", pending)
		}
		colon := strings.IndexByte(line, ':')
		bar := strings.IndexByte(line, '|')
		if colon < 0 || bar < 0 || bar < colon {
			t.Fatalf("pending line %q is not a well-formed name:value|type statsd line (in pending %q)", line, pending)
		}
	}
}
