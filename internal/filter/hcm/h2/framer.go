package h2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"
)

// defaultPeerMaxFrameSize is the RFC 9113 §6.5.2 default for
// SETTINGS_MAX_FRAME_SIZE, used whenever the peer has not (yet) announced a
// value of its own. RFC 9113 §6.5 does not require a SETTINGS frame to carry
// the parameter at all, so a zero peer value means "unannounced", not "zero".
//
// This is the sibling constant already applied by ServerConn.writeData and
// ClientConn.writeData; phase 88 names it rather than minting a third copy of
// the literal.
const defaultPeerMaxFrameSize = 16384

// tryReadFrameWait bounds how long the burst-drain waits for the reader
// goroutine to produce the next frame before declaring the burst exhausted. It
// replaces the 1 ms SOCKET read deadline the pre-phase-91 tryReadFrame armed.
//
// The value is bounded on both sides, and neither bound is arbitrary:
//
//   - it must be > 1 ms, because the phase-91 shape interposes a goroutine
//     handoff the old inline read did not have. 1 ms was sufficient when the
//     drain performed the read itself; it is not obviously sufficient when a
//     scheduler wake-up must precede it.
//   - it must be < 5 ms, because conn_test.go's tiny-window workload drips
//     WINDOW_UPDATE(1,16) every 5 ms. A wait at or above that interval lets
//     consecutive grants COALESCE into one larger frame, which reddens
//     TestServerConn_WriteData_RespectsPerStreamSendWindow on a legitimate
//     frame rather than on a defect.
//
// Raising it to at most 4 ms is permitted, but only after re-running both the
// h2spec 5.1.2/1 gate and the contended TinyWindowDelivery measurement. Any
// value >= 5 ms is a reversal of ADR-0313, not a constant edit.
const tryReadFrameWait = 2 * time.Millisecond

// aLongTimeAgo is a non-zero time far in the past. Stamping it as a read
// deadline unblocks a reader goroutine parked in a blocking read(2) — the one
// reader state no channel close can reach. Mirrors the x/net/http2 idiom.
var aLongTimeAgo = time.Unix(1, 0)

// errReaderNotStarted is returned by readFrameCtx and tryReadFrame when they
// are called on a framer whose reader plumbing was never allocated — i.e. a
// framer built as a composite literal rather than by newFramer. Returning an
// error converts what would otherwise be a permanent block on a nil channel
// (or, before phase 91, a nil-pointer panic on f.conn) into a diagnosable
// failure.
var errReaderNotStarted = errors.New("h2: framer: reader not started")

// framer is a thin wrapper over *http2.Framer adding context-aware reads.
// Write methods are passthrough via embedding, except writeHeaderBlock, which
// wraps WriteHeaders to honor RFC 9113 §6.10 header-block continuation.
// Phase 05.1 does NOT use http2.Framer.WriteRawFrame (per SPEC §4.1).
//
// Phase 91 (ADR-0313) moved the read side onto a dedicated goroutine. The
// pre-phase-91 shape armed a short read deadline around http2.Framer.ReadFrame
// and retried on timeout, but ReadFrame reads its payload with io.ReadFull, so
// a deadline expiring MID-FRAME discarded bytes already drained off the socket
// and the retry resumed at the wrong byte offset — desynchronizing the frame
// stream permanently. The reader goroutine reads with NO deadline, ever, and
// context awareness is provided by selecting on a channel instead.
//
// ⚠️ PRECONDITION, LOAD-BEARING AND UNENFORCED BY THE TYPE SYSTEM: a framer's
// READ side has exactly ONE consumer goroutine. Server-side that is
// (*ServerConn).Run — processFrameAndMaybeDrain runs on the same goroutine.
// Client-side it is (*ClientConn).readLoop. The `held` field below is
// deliberately unsynchronized and would become a data race under a second
// read-side caller; the package's -race gates are what enforce this.
type framer struct {
	*http2.Framer
	conn net.Conn

	// Reader-goroutine plumbing. All four channels are allocated by newFramer;
	// only the GOROUTINE is lazy, spawned by startReader. A framer built as a
	// composite literal therefore has nil channels, which the read methods
	// detect and reject with errReaderNotStarted rather than blocking forever.
	//
	// frameCh is UNBUFFERED, and that is a correctness requirement rather than
	// a tuning choice: x/net's Framer invalidates the previously returned frame
	// at the ENTRY of the next ReadFrame, so the reader must not re-enter
	// ReadFrame while the consumer still holds frame N. Capacity would let it,
	// and every frame accessor then panics with no recover() in this subtree.
	// framer_reader_test.go pins cap(frameCh) == 0 for exactly this reason.
	frameCh   chan http2.Frame
	releaseCh chan struct{} // capacity 1; carries a TOKEN, never a frame
	stopCh    chan struct{} // closed by closeReader
	doneCh    chan struct{} // closed by the reader goroutine on exit

	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool

	// readErr is written by the reader goroutine BEFORE it closes frameCh, and
	// read by the consumer ONLY after a receive on frameCh has reported the
	// channel closed. That close/receive pair is the happens-before edge.
	readErr error

	// held is touched ONLY by the single consumer goroutine and is deliberately
	// unsynchronized. It is true exactly while the consumer owns a frame the
	// reader has not yet been released from.
	held bool
}

// writeHeaderBlock writes an encoded header block as exactly one HEADERS frame
// followed by zero or more CONTINUATION frames (RFC 9113 §6.10), each frame's
// payload bounded by peerMaxFrameSize — the PEER's advertised
// SETTINGS_MAX_FRAME_SIZE, since the limit governs what we may SEND.
//
// x/net's Framer.WriteHeaders does not split: it emits whatever fragment it is
// handed as a single frame, rejecting only at the 16 MiB wire ceiling. Before
// phase 88 both header-write sites called it directly with EndHeaders: true,
// so any block larger than the peer's limit went out as an illegal oversized
// frame — masked only because the read side discarded CONTINUATION frames
// before they could be re-emitted (ADR-0310 §Context ¶3).
//
// The caller's p.EndHeaders is IGNORED and recomputed: END_HEADERS belongs on
// the LAST frame of the block and nowhere else. p.EndStream and the PRIORITY
// fields ride the HEADERS frame only — RFC 9113 §6.10 defines END_HEADERS as
// the sole flag a CONTINUATION carries.
//
// An empty block emits exactly one HEADERS frame with an empty fragment and
// END_HEADERS set (never zero frames): the header block must still terminate.
func (f *framer) writeHeaderBlock(p http2.HeadersFrameParam, peerMaxFrameSize int32) error {
	maxFrame := peerMaxFrameSize
	if maxFrame <= 0 {
		maxFrame = defaultPeerMaxFrameSize
	}

	// SETTINGS_MAX_FRAME_SIZE bounds the frame PAYLOAD, not the fragment, and
	// the HEADERS frame's payload also carries the pad-length byte, the padding
	// itself, and the 5-byte PRIORITY block when those are requested. Subtract
	// that overhead from the FIRST frame's fragment budget, or a HEADERS frame
	// carrying PRIORITY would emit a payload of maxFrame+5 — illegal by exactly
	// the rule this method exists to honor. CONTINUATION frames carry no such
	// overhead, so their budget is the full maxFrame.
	//
	// No production call site sets Priority or PadLength today; the guard is
	// here because the roster asserts the emitted PAYLOAD length rather than the
	// fragment length, so a future call site that sets either cannot silently
	// break the bound.
	headMax := maxFrame
	if p.PadLength != 0 {
		headMax -= 1 + int32(p.PadLength)
	}
	if !p.Priority.IsZero() {
		headMax -= 5
	}
	if headMax < 0 {
		headMax = 0
	}

	block := p.BlockFragment
	head := block
	if int32(len(head)) > headMax {
		head = block[:headMax]
	}
	rest := block[len(head):]

	first := p
	first.BlockFragment = head
	first.EndHeaders = len(rest) == 0
	if err := f.WriteHeaders(first); err != nil {
		return err
	}

	for len(rest) > 0 {
		n := int32(len(rest))
		if n > maxFrame {
			n = maxFrame
		}
		chunk := rest[:n]
		rest = rest[n:]
		if err := f.WriteContinuation(p.StreamID, len(rest) == 0, chunk); err != nil {
			return err
		}
	}
	return nil
}

// newFramer constructs a framer over conn for both reading and writing. The
// returned value embeds *http2.Framer so callers can use WriteSettings,
// WriteHeaders, WriteData, WriteRSTStream, WriteWindowUpdate, WritePing,
// WriteGoAway, and ReadFrame directly. maxReadFrameSize sets the read-side
// limit: frames larger than this trigger a FRAME_SIZE_ERROR.
//
// The reader plumbing is ALLOCATED here but the reader goroutine is NOT
// spawned: several call sites read the socket directly before any framer read
// may happen, so the spawn is an explicit startReader() seam. A framer built
// as a composite literal instead of by newFramer therefore has nil channels,
// which the read methods reject with errReaderNotStarted.
func newFramer(conn net.Conn, maxReadFrameSize uint32) *framer {
	fr := http2.NewFramer(conn, conn)
	fr.SetMaxReadFrameSize(maxReadFrameSize)
	return &framer{
		Framer: fr,
		conn:   conn,
		// frameCh is UNBUFFERED and must stay that way — see the field comment.
		// Capacity would break the frame-ownership invariant, not merely change
		// throughput.
		frameCh:   make(chan http2.Frame),
		releaseCh: make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// startReader spawns the frame-reader goroutine. It MUST be called exactly
// once per framer, from the connection's own setup goroutine, AFTER every
// direct socket read (the client preface, the peer's initial SETTINGS) and
// BEFORE the first readFrameCtx or tryReadFrame.
//
// The seam is explicit rather than folded into newFramer because four things
// read the socket directly before any framer read: readClientPreface,
// readClientSettings (which calls the EMBEDDED fr.ReadFrame on both sides),
// the two post-GOAWAY io.Copy drains, and the package's own tests, eleven of
// which hold a *framer and call the embedded ReadFrame directly.
//
// Idempotent. A framer whose reader was never started is still safe to
// closeReader.
func (f *framer) startReader() {
	f.startOnce.Do(func() {
		f.started.Store(true)
		go f.readerLoop()
	})
}

// readerLoop is the single reader goroutine. The name is readerLoop, not
// readLoop: (*ClientConn).readLoop already exists in this package and two
// identically named loops is how a reviewer misreads a defer.
func (f *framer) readerLoop() {
	defer close(f.doneCh)
	for {
		// NO deadline. This is the whole of ADR-0313: a frame read is never
		// abandoned part-way, so io.ReadFull can never discard bytes it has
		// already drained off the socket.
		frame, err := f.Framer.ReadFrame()
		if err != nil {
			// Store BEFORE the close: the close/receive pair is what publishes
			// readErr to the consumer.
			f.readErr = err
			close(f.frameCh)
			return
		}
		select {
		case f.frameCh <- frame:
		case <-f.stopCh:
			return
		}
		// The consumer now OWNS frame. Do not re-enter ReadFrame until it is
		// released: x/net invalidates the previous frame at the entry of the
		// next ReadFrame.
		select {
		case <-f.releaseCh:
		case <-f.stopCh:
			return
		}
	}
}

// release signals the reader that the frame it last handed over is finished
// with. It is called at the START of each consumer read call, which is exact
// parity with x/net's invalidate-at-entry-of-the-next-ReadFrame: the frame
// stays valid for precisely as long as it did before phase 91.
//
// release is a no-op unless a frame is held, which bounds outstanding tokens
// to exactly one — which is in turn why releaseCh can have capacity 1 and why
// this send can never block.
func (f *framer) release() {
	if !f.held {
		return
	}
	f.held = false
	f.releaseCh <- struct{}{}
}

// exitErr reports why the reader stopped. It substitutes io.EOF for a nil
// readErr so a consumer can never receive (nil, nil) from a CLOSED frameCh and
// then dereference a nil Frame. The reader never closes frameCh with a nil
// readErr, so this is a fail-closed guard rather than a live path.
func (f *framer) exitErr() error {
	if f.readErr == nil {
		return io.EOF
	}
	return f.readErr
}

// closeReader stops the reader goroutine and JOINS it. Idempotent, and safe on
// a framer whose reader was never started or whose channels were never
// allocated.
//
// AFTER closeReader RETURNS, THE READER GOROUTINE IS GONE. That is the
// property the two post-GOAWAY drains and framer_leak_test.go both depend on.
//
// It is signal + stamp + join, over the three states the reader can be parked
// in, because no single mechanism reaches all three:
//
//	(i)   blocked in ReadFrame on the socket — reachable ONLY by the deadline
//	      stamp; no channel close can interrupt a blocking read(2).
//	(ii)  blocked SENDING on frameCh       — released by close(stopCh).
//	(iii) blocked WAITING FOR RELEASE      — released by close(stopCh).
//
// The stamp is safe here only because phase 91 deleted every reader-side
// deadline clear from this file; the read deadline now has exactly three
// writers tree-wide — this one, and the two post-GOAWAY drains with their
// paired clears. It is deliberately NOT cleared: both drain sites overwrite it
// with their own bound immediately afterwards, and after the join there is no
// reader left for it to affect. Clearing it would re-open the unbounded
// blocking read this row exists to remove.
func (f *framer) closeReader() {
	if f.stopCh == nil {
		return // composite-literal framer; there is no reader
	}
	f.stopOnce.Do(func() {
		close(f.stopCh)
		if f.conn != nil {
			_ = f.conn.SetReadDeadline(aLongTimeAgo)
		}
	})
	if f.started.Load() {
		<-f.doneCh
	}
}

// translateFramerErr maps errors emitted by *http2.Framer to our h2:-prefixed
// *Error type so callers and fuzz assertions can rely on the h2: prefix
// discipline. Returns nil for nil input. Unrecognized errors pass through
// unchanged.
//
// Recognized classes:
//   - http2.ConnectionError → connection-scoped *Error
//   - http2.StreamError     → stream-scoped *Error (preserving StreamID)
//   - http2.ErrFrameTooLarge (RFC 9113 §4.2 connection error of type
//     FRAME_SIZE_ERROR; not wrapped as http2.ConnectionError by the
//     underlying framer)
//
// Consumed by both readFrameCtx and tryReadFrame, and (post phase 05.2) by
// the client-side codec read loop in client.go.
func translateFramerErr(err error) error {
	if err == nil {
		return nil
	}
	var connErr http2.ConnectionError
	if errors.As(err, &connErr) {
		return connError(ErrCode(connErr), fmt.Sprintf("framer: connection-error code=%d", connErr))
	}
	var streamErr http2.StreamError
	if errors.As(err, &streamErr) {
		return streamError(ErrCode(streamErr.Code), streamErr.StreamID, fmt.Sprintf("framer: stream-error code=%d", streamErr.Code))
	}
	if errors.Is(err, http2.ErrFrameTooLarge) {
		return connError(ErrFrameSizeError, "framer: frame too large")
	}
	return err
}

// readFrameCtx reads one frame, honoring ctx cancellation. Since phase 91 the
// socket read happens on the reader goroutine with NO deadline; this method
// waits for that goroutine to hand a frame over, or for ctx to be done.
//
// On a CLOSED frameCh the reader has already stored its error and returned, so
// the stored error is reported directly without touching the socket. That
// stickiness is new behavior and is deliberate (ADR-0313): it is strictly more
// deterministic than re-entering a failed socket read, and it is what makes
// the reader provably gone by the time any consumer observes an error.
//
// translateFramerErr is applied on EVERY read rather than once at store time:
// connError and streamError allocate a fresh *Error per call, so each caller
// keeps the per-call allocation shape it had before phase 91.
func (f *framer) readFrameCtx(ctx context.Context) (http2.Frame, error) {
	if f.frameCh == nil {
		return nil, errReaderNotStarted
	}
	// The ctx-error early return comes BEFORE the release, and the order is
	// load-bearing. The pre-phase-91 body returned on ctx.Err() without ever
	// reaching ReadFrame, so a ctx-canceled read did NOT invalidate the
	// previously returned frame. Releasing first would invalidate where the old
	// code did not — a behavior change smuggled in as a refactor. It is also
	// what keeps ctx precedence ahead of a stored read error.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.release()
	select {
	case frame, ok := <-f.frameCh:
		if !ok {
			return nil, translateFramerErr(f.exitErr())
		}
		f.held = true
		return frame, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// tryReadFrame attempts to read one frame with a short bounded wait, returning
// (nil, nil) when the burst is exhausted. It is used by the frame loop to
// detect whether more frames arrived in the same TCP burst, so that pending
// dispatch goroutines can be deferred until the batch is drained (see
// ServerConn.Run).
//
// The wait is a TIMER, never a bare default:. A non-blocking select would
// return before the reader goroutine could be scheduled, silently gutting the
// burst drain that the RST_STREAM-before-DATA ordering guarantee — and h2spec
// 5.1.2/1 with it — depends on.
func (f *framer) tryReadFrame() (http2.Frame, error) {
	if f.frameCh == nil {
		return nil, errReaderNotStarted
	}
	f.release()
	t := time.NewTimer(tryReadFrameWait)
	defer t.Stop()
	select {
	case frame, ok := <-f.frameCh:
		if !ok {
			return nil, translateFramerErr(f.exitErr())
		}
		f.held = true
		return frame, nil
	case <-t.C:
		return nil, nil // burst exhausted — the ONLY (nil, nil) return
	}
}
