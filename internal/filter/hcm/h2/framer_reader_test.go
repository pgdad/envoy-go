package h2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// pollUntil spins until cond() or the deadline, failing the test on timeout.
// Mirrors internal/cluster/connpool_test.go:34 — the shape the goroutine-leak
// precedent (TestH2PoolWatcherEvictRaceNoLeak) itself uses. It is MIRRORED
// rather than imported because that copy is an unexported test helper in
// another package. Two other pollUntil shapes exist in this tree with
// DIFFERENT signatures; this file deliberately does not mint a fourth.
func pollUntil(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pollUntil timed out: %s", msg)
}

// loopbackPair returns a connected pair of REAL TCP sockets on the loopback
// interface. The rest of framer_test.go uses net.Pipe, which is a synchronous
// in-memory pipe with no kernel socket underneath; these tests exercise the
// reader goroutine's interaction with an actual blocking read(2) and with
// SetReadDeadline, so a real socket is what is wanted here.
func loopbackPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	type acc struct {
		c   net.Conn
		err error
	}
	ch := make(chan acc, 1)
	go func() {
		c, err := ln.Accept()
		ch <- acc{c, err}
	}()
	dialed, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	got := <-ch
	if got.err != nil {
		t.Fatalf("accept: %v", got.err)
	}
	t.Cleanup(func() {
		_ = dialed.Close()
		_ = got.c.Close()
	})
	return got.c, dialed
}

// TestFramer_FrameChIsUnbuffered pins the frame channel's capacity at zero.
//
// This is a cheap STRUCTURAL pin and is deliberately not dressed up as a
// behavioral test: it catches a capacity change and nothing else. It cannot
// catch a reordering of the handover/release handshake.
//
// The capacity matters because x/net's Framer invalidates the previously
// returned frame at the ENTRY of the next ReadFrame. A buffered frame channel
// would let the reader re-enter ReadFrame while the consumer still holds frame
// N; every frame accessor then panics via frame.go's ownership check, and
// there is no recover() anywhere in internal/filter/hcm/. The failure mode of
// adding capacity here is a PROCESS CRASH, not slightly wrong data.
func TestFramer_FrameChIsUnbuffered(t *testing.T) {
	srv, _ := loopbackPair(t)
	f := newFramer(srv, 16384)

	if got := cap(f.frameCh); got != 0 {
		t.Fatalf("cap(frameCh) = %d, want 0: a buffered frame channel lets the reader re-enter "+
			"ReadFrame while the consumer still holds frame N, which invalidates that frame and "+
			"makes every accessor panic with no recover() in this subtree", got)
	}
	// releaseCh carries a token, never a frame. Capacity 1 is what makes
	// release() non-blocking; `held` bounds outstanding tokens to exactly one.
	if got := cap(f.releaseCh); got != 1 {
		t.Fatalf("cap(releaseCh) = %d, want 1", got)
	}
}

// TestFramer_ReadErrIsSticky pins the D-91-ERR stickiness contract: the reader
// exits on its FIRST read error, stores it, and closes frameCh, so every
// subsequent consumer read reports that same error IMMEDIATELY and without
// touching the socket.
func TestFramer_ReadErrIsSticky(t *testing.T) {
	srv, cli := loopbackPair(t)
	f := newFramer(srv, 16384)
	f.startReader()
	defer f.closeReader()

	// ⚠️ THE PEER SENDS A MALFORMED FRAME RATHER THAN JUST CLOSING, AND THAT
	// CHOICE IS THE PIN. A SETTINGS frame whose length is not a multiple of 6 is
	// a connection error of type FRAME_SIZE_ERROR (RFC 9113 §6.5.1), so the
	// stored error is DISTINCTIVE. Provoking the error with a bare peer close
	// instead makes it io.EOF -- and io.EOF is exactly what exitErr's fail-closed
	// guard substitutes when readErr is nil, so a regression that DROPPED the
	// stored error would still hand back io.EOF on every read and a
	// close-provoked version of this test would pass while pinning nothing.
	// Measured: with exitErr changed to consume readErr once, the io.EOF form
	// stayed green and this form reddens.
	if _, err := cli.Write([]byte{0, 0, 3, 0x04, 0, 0, 0, 0, 0, 1, 2, 3}); err != nil {
		t.Fatalf("write malformed SETTINGS: %v", err)
	}

	ctx := context.Background()
	_, err1 := f.readFrameCtx(ctx)
	if err1 == nil {
		t.Fatal("first readFrameCtx after a malformed SETTINGS returned nil error; want non-nil")
	}
	var h2err *Error
	if !errors.As(err1, &h2err) || h2err.Code != ErrFrameSizeError {
		t.Fatalf("first readFrameCtx = %v (%T); want an *Error with Code=FRAME_SIZE_ERROR. "+
			"translateFramerErr must be applied to the STORED error too, not only to live reads.", err1, err1)
	}

	// The second and third reads must return the SAME error class, and must
	// return promptly. The elapsed bound is what stops a HANG from passing as
	// stickiness: without it, a read that blocked forever would be
	// indistinguishable from one that returned the stored error.
	start := time.Now()
	_, err2 := f.readFrameCtx(ctx)
	_, err3 := f.readFrameCtx(ctx)
	elapsed := time.Since(start)

	if err2 == nil || err3 == nil {
		t.Fatalf("sticky reads returned nil: err2=%v err3=%v; want the stored read error", err2, err3)
	}
	if err2.Error() != err1.Error() || err3.Error() != err1.Error() {
		t.Fatalf("sticky reads changed error: err1=%v err2=%v err3=%v. The stored read error "+
			"must survive every subsequent read; degrading to io.EOF means readErr was consumed.", err1, err2, err3)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("two sticky reads took %v; want near-immediate — a hang must not pass as stickiness", elapsed)
	}
}

// TestFramer_CtxErrTakesPrecedenceOverStickyReadErr pins that a canceled
// context still wins over a stored read error, i.e. that readFrameCtx checks
// ctx.Err() BEFORE consulting the closed frame channel.
//
// This is the assertion that catches someone "simplifying" the ctx-error early
// return away.
//
// What the early return actually buys is stated narrowly, because it is easy
// to oversell: it preserves the pre-phase-91 behavior in which a ctx-canceled
// read does NOT invalidate the previously returned frame (the old body
// returned on ctx.Err() without ever reaching ReadFrame), and it fixes the
// PRECEDENCE of ctx cancellation over a stored read error. It is NOT the only
// thing standing between readLoop and a permanent block: when the reader exits
// via stopCh it returns WITHOUT closing frameCh, and a consumer that re-enters
// readFrameCtx afterwards is rescued by the select's own ctx.Done() arm. The
// early return makes that immediate rather than deferred; it does not make the
// difference between hanging and not hanging.
func TestFramer_CtxErrTakesPrecedenceOverStickyReadErr(t *testing.T) {
	srv, cli := loopbackPair(t)
	f := newFramer(srv, 16384)
	f.startReader()
	defer f.closeReader()

	_ = cli.Close()

	// Drive the framer into the stored-error / closed-channel state first.
	if _, err := f.readFrameCtx(context.Background()); err == nil {
		t.Fatal("setup: expected a read error after peer close")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// ⚠️ ASSERTED 32 TIMES, AND THE REPETITION IS THE WHOLE PIN. A single
	// assertion here is a COIN FLIP, measured: delete the ctx.Err() early return
	// and both arms of readFrameCtx's select are simultaneously ready — the
	// closed frameCh and ctx.Done() — so Go picks one uniformly at random. A
	// one-shot version of this test reddened only 7 times in 20 under exactly
	// that ablation, i.e. it would have passed as a regression pin while failing
	// to pin anything. Thirty-two independent draws take the miss probability to
	// about 2e-10; with the early return in place every draw is deterministic.
	const draws = 32
	for i := 0; i < draws; i++ {
		_, err := f.readFrameCtx(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("readFrameCtx draw %d/%d with a canceled ctx returned %v; want context.Canceled. "+
				"The ctx.Err() early return was removed or reordered behind the stored read error, "+
				"leaving the closed-frameCh arm and the ctx.Done() arm to race.", i+1, draws, err)
		}
	}
}

// TestFramer_NotStartedGuard pins that the composite-literal framer shape
// (the framer_writeheaderblock_test.go:235 shape: Framer set, conn nil, all
// four channels nil) fails LOUDLY rather than hanging or panicking.
//
// Before phase 91 this shape nil-panicked on f.conn.SetReadDeadline. Under the
// reader-goroutine shape a nil channel would instead block forever, which is
// strictly worse to diagnose. The guard converts both into an error, and this
// test is what keeps it that way.
func TestFramer_NotStartedGuard(t *testing.T) {
	var buf bytes.Buffer
	f := &framer{Framer: http2.NewFramer(&buf, nil)}

	if _, err := f.readFrameCtx(context.Background()); !errors.Is(err, errReaderNotStarted) {
		t.Fatalf("readFrameCtx on a composite-literal framer = %v; want errReaderNotStarted", err)
	}
	if _, err := f.tryReadFrame(); !errors.Is(err, errReaderNotStarted) {
		t.Fatalf("tryReadFrame on a composite-literal framer = %v; want errReaderNotStarted", err)
	}
	// closeReader must be a safe no-op on this shape, not a nil-deref.
	f.closeReader()
	f.closeReader()
}

// TestFramer_CloseReaderIdempotentAndJoins pins closeReader's three safety
// properties: safe when the reader was never started, safe when called twice,
// and safe when called while a consumer is parked in readFrameCtx — after
// which THE READER GOROUTINE IS GONE.
//
// The goroutine assertion POLLS rather than sampling once, mirroring
// TestH2PoolWatcherEvictRaceNoLeak: a single sample races the scheduler.
func TestFramer_CloseReaderIdempotentAndJoins(t *testing.T) {
	// (i) never started: must not hang and must not panic.
	srvA, _ := loopbackPair(t)
	fA := newFramer(srvA, 16384)
	fA.closeReader()
	fA.closeReader()

	// (ii)/(iii) started, called twice, concurrently with a blocked consumer.
	base := runtime.NumGoroutine()
	srvB, _ := loopbackPair(t)
	fB := newFramer(srvB, 16384)
	fB.startReader()

	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		// The peer never writes, so this parks until closeReader releases it.
		_, _ = fB.readFrameCtx(context.Background())
	}()

	// Let the consumer actually reach its park before closing, so the test
	// exercises the concurrent case rather than a trivially-ordered one.
	pollUntil(t, func() bool { return runtime.NumGoroutine() > base }, "consumer goroutine should start")

	fB.closeReader()
	fB.closeReader()

	select {
	case <-consumerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("consumer parked in readFrameCtx was not released by closeReader")
	}

	pollUntil(t, func() bool { return runtime.NumGoroutine() <= base },
		"the reader goroutine must be gone after closeReader returns")
}

// TestServerConn_BurstDrainDefersDispatchUntilBurstDrained pins the burst
// deferral that ServerConn.Run's frame loop exists to provide, and with it the
// RST_STREAM-before-DATA ordering guarantee documented there: when more streams
// than MaxConcurrentStreams arrive in a SINGLE TCP burst, every HEADERS frame of
// that burst is admitted-or-refused before any accepted stream's dispatch
// goroutine is launched.
//
// ⚠️ WHY THIS TEST EXISTS, stated plainly because it is an addition beyond the
// phase-91 PLAN's chartered task list. That guarantee was believed to be gated
// by h2spec 5.1.2/1. It is NOT. Measured at the phase-91 IMPL: with tryReadFrame
// ablated to `return nil, nil` — which removes the burst deferral entirely — the
// h2 package runs 204/204 green AND h2spec runs 95 tests / 94 passed / 0 failed,
// 4 times out of 4, with the ablation confirmed present in the compiled subject
// by disassembly. So before this test the guarantee was pinned by ZERO tests at
// ANY layer, unit or conformance, while this row REWRITES the very function that
// provides it.
//
// It differs from TestServerConn_MaxConcurrentStreamsEnforcement in the two ways
// that make that test structurally incapable of catching a regression here: this
// one uses a FAST dispatcher (a blocking one makes the outcome an artifact of the
// fixture, since accepted streams then physically cannot complete or emit DATA
// inside the observation window), and it asserts a whole-burst property rather
// than "at least one stream was refused".
//
// ⚠️ THE TWO ARMS ARE NOT INTERCHANGEABLE, and which one carries the negative
// control was MEASURED, not assumed. Under the ablation, over 20 runs at this
// exact workload:
//
//   - the REFUSAL-COUNT arm reddened 20/20. It is the deterministic one.
//   - the ordering arm (every REFUSED_STREAM reset precedes every DATA frame)
//     reddened 19/20 — the one green sample was a run in which the ablated server
//     refused NOTHING at all, which the count arm caught instead.
//   - the WEAKER ordering formulation the first draft of this test used (merely
//     "the FIRST reset precedes the FIRST DATA") reddened only 11/20 and is
//     therefore NOT used here. On the unablated code the drain out-runs the
//     dispatch goroutines often enough that a first-reset-first sample proves
//     nothing.
//
// Both arms use t.Errorf, never t.Fatalf: a Fatalf in the first arm would make
// the second unreachable and quietly halve the test.
//
// The count arm is a direct statement of the deferral: while the burst is being
// drained NO dispatch goroutine is running, so no accepted stream can COMPLETE
// and hand its MAX_CONCURRENT_STREAMS slot back to a later HEADERS frame of the
// same burst. The admitted set is therefore exactly the first maxConcurrent
// streams, and every later one is refused. Flush dispatch per-frame instead and
// early streams start finishing mid-burst, so later streams find free slots and
// are admitted — which is what the ablated runs above show.
//
// It also gates tryReadFrameWait from below: the drain must outlast the
// reader-goroutine handoff, or it declares the burst exhausted early and flushes
// dispatch mid-burst — the same defect by a different route.
func TestServerConn_BurstDrainDefersDispatchUntilBurstDrained(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// A FAST dispatcher on purpose — see the header. maxConcurrent is small and
	// the burst is large so the refusal count is a wide target: 4 admitted, 20
	// refused. The ablated runs measured above land anywhere in 0..17 refusals.
	const (
		streams       = 24
		maxConcurrent = 4
	)
	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "accepted-stream-body"}}
	settings := DefaultServerSettings
	settings.MaxConcurrentStreams = maxConcurrent

	clientConn, _ := startServerConn(t, ctx, disp, settings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}
	fr := http2.NewFramer(clientConn, clientConn)
	if err := fr.WriteSettings(); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(10 * time.Second))
	settingsAcked, clientAcked := false, false
	for !settingsAcked || !clientAcked {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("handshake: %v", err)
		}
		if f.Header().Type == http2.FrameSettings {
			sf := f.(*http2.SettingsFrame)
			if !sf.IsAck() {
				_ = fr.WriteSettingsAck()
				settingsAcked = true
			} else {
				clientAcked = true
			}
		}
	}

	// Build all HEADERS frames into one buffer and deliver them with ONE Write,
	// so they land in a single TCP burst. Delivering them as separate writes
	// would not exercise the drain at all.
	var burst bytes.Buffer
	bfr := http2.NewFramer(&burst, &burst)
	for i := 0; i < streams; i++ {
		id := uint32(2*i + 1)
		var encBuf bytes.Buffer
		enc := hpack.NewEncoder(&encBuf)
		_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
		_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: fmt.Sprintf("/stream%d", id)})
		_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
		_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})
		if err := bfr.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      id,
			BlockFragment: encBuf.Bytes(),
			EndHeaders:    true,
			EndStream:     true,
		}); err != nil {
			t.Fatalf("buffer headers stream %d: %v", id, err)
		}
	}
	if _, err := clientConn.Write(burst.Bytes()); err != nil {
		t.Fatalf("write burst: %v", err)
	}

	// Read in ARRIVAL ORDER until every stream has reached a terminal frame —
	// either a RST_STREAM refusing it, or the END_STREAM DATA frame completing
	// it. That termination condition holds for BOTH the correct and the ablated
	// server (each stream is refused or served either way), so the read loop
	// cannot itself become the thing that decides the verdict, and neither
	// outcome pays a read-timeout.
	var (
		seq      []string
		resets   int
		firstRST = -1
		lastRST  = -1
		firstDMA = -1
		terminal int
	)
	_ = clientConn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for terminal < streams {
		f, err := fr.ReadFrame()
		if err != nil {
			seq = append(seq, "read-error:"+err.Error())
			break
		}
		switch ff := f.(type) {
		case *http2.RSTStreamFrame:
			seq = append(seq, fmt.Sprintf("RST(%d,%v)", ff.StreamID, ff.ErrCode))
			if ff.ErrCode == http2.ErrCodeRefusedStream {
				if firstRST < 0 {
					firstRST = len(seq) - 1
				}
				lastRST = len(seq) - 1
				resets++
				terminal++
			}
		case *http2.DataFrame:
			seq = append(seq, fmt.Sprintf("DATA(%d,end=%v)", ff.StreamID, ff.StreamEnded()))
			if firstDMA < 0 {
				firstDMA = len(seq) - 1
			}
			if ff.StreamEnded() {
				terminal++
			}
		case *http2.HeadersFrame:
			seq = append(seq, fmt.Sprintf("HEADERS(%d)", ff.StreamID))
		default:
			seq = append(seq, fmt.Sprintf("%v(%d)", f.Header().Type, f.Header().StreamID))
		}
	}

	// Arm 1 — the deferral itself, and the arm that carries the negative control.
	if want := streams - maxConcurrent; resets != want {
		t.Errorf("BURST-DEFERRAL REGRESSION: %d RST_STREAM(REFUSED_STREAM) for %d streams at "+
			"max_concurrent=%d; want exactly %d. A dispatch goroutine was launched while the "+
			"burst was still being drained, so an early stream COMPLETED mid-burst and handed "+
			"its concurrency slot to a later HEADERS frame of the same burst.\nframe order: %v",
			resets, streams, maxConcurrent, want, seq)
	}

	// Arm 2 — the documented ordering guarantee, in its strong form: EVERY
	// refusal precedes EVERY DATA frame, not merely the first.
	if firstRST >= 0 && firstDMA >= 0 && lastRST > firstDMA {
		t.Errorf("ORDERING REGRESSION: a DATA frame arrived at index %d, BEFORE the "+
			"RST_STREAM(REFUSED_STREAM) at index %d. Every refusal of a burst must reach the "+
			"wire before any accepted stream's DATA, or a client sees a response body before "+
			"the refusal that logically precedes it.\nframe order: %v", firstDMA, lastRST, seq)
	}
}
