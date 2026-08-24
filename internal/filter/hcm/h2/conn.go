package h2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// recvWindowUpdateThreshold is the high-water mark at which the receiver emits
// a WINDOW_UPDATE to replenish its inbound flow-control window. Set to half the
// default initial window (65535/2 ≈ 32768) per ADR-0055: small enough that the
// peer is rarely stalled, large enough that we are not flooding the wire with
// WINDOW_UPDATE frames.
const recvWindowUpdateThreshold = int32(32768)

// safeAddInt32 returns (a+b, true) iff the sum fits in int32 (i.e. is in the
// inclusive range [math.MinInt32, math.MaxInt32]). On overflow returns
// (0, false). Used by RFC 9113 §6.9.1 WINDOW_UPDATE bounds checks.
func safeAddInt32(a, b int32) (int32, bool) {
	sum := int64(a) + int64(b)
	if sum > math.MaxInt32 || sum < math.MinInt32 {
		return 0, false
	}
	return int32(sum), true
}

// maxHeaderBlockSize bounds the total number of header-block fragment bytes a
// single HEADERS + CONTINUATION sequence may accumulate before the connection
// is torn down. RFC 9113 §6.10 places no limit on the number of CONTINUATION
// frames in a block, and x/net's Framer.checkFrameOrder caps each individual
// HOP (SETTINGS_MAX_FRAME_SIZE) but never the running total.
//
// ⚠️ The exposure this bounds is CREATED by reassembly, not repaired by it.
// Before phase 88 the codec DISCARDED every CONTINUATION fragment and
// therefore retained nothing; accumulating the block is what introduces
// retention. envoy-go does not inherit x/net's CVE-2023-45288 CONTINUATION-
// flood mitigation (that lives in readMetaFrame, which this codec never
// enables) and never advertises SETTINGS_MAX_HEADER_LIST_SIZE, so this
// constant is the ONLY cap on a CONTINUATION flood.
//
// The value diverges from the reference deliberately and in two ways — see
// ADR-0310 §Decision: Envoy rejects at max_request_headers_kb (60 KiB by
// default) with a STREAM-scoped RST_STREAM(INTERNAL_ERROR), while this codec
// rejects at 16 MiB with a CONNECTION-scoped GOAWAY(ENHANCE_YOUR_CALM),
// because a byte accumulator that abandons a partially-received block leaves
// the connection-scoped HPACK dynamic table permanently diverged from the
// peer's encoder — the exact defect phase 88 exists to fix.
const maxHeaderBlockSize = 16 << 20 // 16 MiB

// headerBlockAccumulator buffers the fragments of one header block that
// arrived split across a HEADERS frame and one or more CONTINUATION frames
// (RFC 9113 §6.10). The originating HEADERS frame's flags are carried here
// because a CONTINUATION frame defines only END_HEADERS: END_STREAM and the
// PRIORITY fields belong to the HEADERS frame that opened the block.
//
// One accumulator per CONNECTION, not per stream: RFC 9113 §6.10 forbids any
// intervening frame between a HEADERS and its CONTINUATION frames, so at most
// one block can be open on a connection at a time. Both ServerConn (frame
// loop) and ClientConn (read loop) touch their accumulator from a single
// goroutine, so no mutex is taken.
type headerBlockAccumulator struct {
	streamID    uint32
	buf         []byte
	endStream   bool
	hasPriority bool
	priority    http2.PriorityParam
}

// startHeaderBlock opens an accumulator for a HEADERS frame that did NOT carry
// END_HEADERS. The fragment is COPIED: http2.Framer reuses its internal read
// buffer across ReadFrame calls, so the slice does not survive the next read.
func startHeaderBlock(f *http2.HeadersFrame) *headerBlockAccumulator {
	frag := f.HeaderBlockFragment()
	a := &headerBlockAccumulator{
		streamID:    f.Header().StreamID,
		buf:         make([]byte, len(frag)),
		endStream:   f.StreamEnded(),
		hasPriority: f.HasPriority(),
		priority:    f.Priority,
	}
	copy(a.buf, frag)
	return a
}

// appendContinuation appends a CONTINUATION fragment to the open accumulator
// a, which may be nil when no block is open. Shared by ServerConn and
// ClientConn so the bound and the three rejects have exactly one definition.
//
// All three rejects are CONNECTION-scoped: a header block abandoned partway
// through leaves the connection-scoped HPACK decoder unable to interpret any
// later block, which no per-stream reset can repair.
//
// ⚠️ The first two rejects are DEFENSE IN DEPTH, not reachable coverage: with
// frames obtained through Framer.ReadFrame, x/net's checkFrameOrder already
// rejects a CONTINUATION that no HEADERS opened ("unexpected CONTINUATION")
// and one naming a different stream, surfacing as a framer connection error
// before this code sees the frame. The size reject is NOT defense in depth —
// checkFrameOrder bounds the hop, not the total, and a 19.6 MB flood reaches
// here (PLAN-88 §4.1).
//
// ⚠️ The first two messages below deliberately echo x/net's own checkFrameOrder
// wording. That is harmless ONLY because translateFramerErr discards x/net's
// errDetail and reports "framer: connection-error code=%d" instead, so the two
// sources can never be confused in a log or an assertion. If anyone ever plumbs
// errDetail through translateFramerErr, a substring assertion on these two
// messages stops discriminating and must be re-anchored on the h2: prefix.
//
// The size reject needs no errors.Is sentinel: ErrEnhanceYourCalm is emitted at
// exactly one site in internal/ (grep-confirmed at phase 88), so asserting the
// Code alone already discriminates it.
func appendContinuation(a *headerBlockAccumulator, f *http2.ContinuationFrame) error {
	streamID := f.Header().StreamID
	if a == nil {
		return connError(ErrProtocolError, fmt.Sprintf("unexpected CONTINUATION for stream %d", streamID))
	}
	if a.streamID != streamID {
		return connError(ErrProtocolError, fmt.Sprintf("got CONTINUATION for stream %d; expected stream %d", streamID, a.streamID))
	}
	frag := f.HeaderBlockFragment()
	if len(a.buf)+len(frag) > maxHeaderBlockSize {
		return connError(ErrEnhanceYourCalm, "h2 continuation accumulator exceeded maxHeaderBlockSize")
	}
	a.buf = append(a.buf, frag...)
	return nil
}

// ServerConn is one downstream HTTP/2 server connection. Construct with
// NewServerConn; call Run to drive the connection lifecycle.
//
// Import discipline: this file and all other h2 package files MUST NOT import
// internal/filter/hcm. The one-way import (hcm → h2) is settled per PLAN
// "Settled SPEC §10 deferred decisions" #10. The Dispatcher interface below is
// the seam that keeps the dependency direction correct.
type ServerConn struct {
	ctx           context.Context
	conn          net.Conn
	dispatcher    Dispatcher
	settings      ServerSettings
	fr            *framer
	hpack         *hpackState
	sendW         *window
	recvW         *window
	mu            sync.Mutex
	streams       map[uint32]*serverStream
	closedStreams map[uint32]struct{} // recently-closed stream IDs, for post-close validation
	lastInID      uint32              // highest stream id seen from client (RFC 9113 §5.1.1 monotonic)
	clientS       clientSettings
	goawaySent    bool
	peerGoaway    bool // true after receiving GOAWAY from client; stop accepting new streams
	// doneCh is used by dispatch goroutines to notify the frame loop when a
	// stream has been fully handled.  The frame loop drains doneCh before each
	// MAX_CONCURRENT_STREAMS admission check so that s.streams accurately
	// reflects the number of in-flight streams at decision time.
	doneCh chan uint32
	// pendingDispatch holds goroutine closures that should be launched after the
	// current burst of frames is fully processed.  Deferring dispatch until the
	// burst is drained guarantees that RST_STREAM(REFUSED_STREAM) for an
	// overflow stream is written BEFORE any DATA response from earlier streams,
	// which is required by h2spec 5.1.2/1.
	pendingDispatch []func()
	// recvDebitSinceLastUpdate accumulates inbound DATA bytes consumed against
	// the connection-level recv window since the last conn-level WINDOW_UPDATE
	// was emitted. When it crosses recvWindowUpdateThreshold we emit a
	// WINDOW_UPDATE(0, recvDebitSinceLastUpdate), replenish the local recvW
	// counter by the same amount, and reset to 0. Mutated only from the
	// frame-loop goroutine (in onData), so no separate mutex.
	recvDebitSinceLastUpdate int32
	// hdrAccum holds the in-progress header block when a HEADERS frame arrived
	// without END_HEADERS (RFC 9113 §6.10). nil whenever no block is open.
	// Mutated only from the frame-loop goroutine (onHeaders / onContinuation).
	hdrAccum *headerBlockAccumulator
}

// NewServerConn constructs a ServerConn value. Run owns conn (closes on exit).
// dispatcher is called once per stream to resolve the incoming request to an
// Action. The h2 package defines the Dispatcher interface; the hcm parent
// package provides the production implementation (h2dispatch.go).
func NewServerConn(ctx context.Context, conn net.Conn, dispatcher Dispatcher, settings ServerSettings) *ServerConn {
	return &ServerConn{
		ctx:           ctx,
		conn:          conn,
		dispatcher:    dispatcher,
		settings:      settings,
		fr:            newFramer(conn, settings.MaxFrameSize),
		hpack:         newHPACKState(settings.HeaderTableSize),
		sendW:         newWindow(int32(settings.InitialWindowSize)),
		recvW:         newWindow(int32(settings.InitialWindowSize)),
		streams:       make(map[uint32]*serverStream),
		closedStreams: make(map[uint32]struct{}),
		// Buffer size 256: at most MaxConcurrentStreams (default 100) goroutines
		// can signal concurrently; extra headroom prevents blocking goroutines.
		doneCh: make(chan uint32, 256),
	}
}

// Run drives the connection lifecycle. Returns nil on clean shutdown,
// *Error on protocol violation, ctx.Err() on cancellation.
func (s *ServerConn) Run() error {
	defer func() { _ = s.conn.Close() }()
	// Written AFTER the Close defer above ON PURPOSE: Go defers run LIFO, so a
	// defer written later RUNS EARLIER. The reader must be joined while the
	// conn is still open, never after it has been closed underneath it.
	defer s.fr.closeReader()

	// Step 1: read client preface.
	// RFC 9113 §3.4: mismatch → connection error PROTOCOL_ERROR. We must send
	// GOAWAY before closing the connection so h2spec 3.5/2 sees it.
	if err := readClientPreface(s.conn); err != nil {
		s.emitGoaway(ErrProtocolError)
		return err
	}

	// Step 2: write server initial SETTINGS.
	if err := writeServerInitialSettings(s.fr, s.settings); err != nil {
		return err
	}

	// Step 3: read client initial SETTINGS. Emit GOAWAY with the returned
	// error's OWN code (e.g. a parse-rejected INITIAL_WINDOW_SIZE surfaces
	// FLOW_CONTROL_ERROR, not a hardcoded PROTOCOL_ERROR) — phase 85.
	if err := readClientSettings(s.fr, &s.clientS); err != nil {
		code := ErrProtocolError
		var h2err *Error
		if errors.As(err, &h2err) {
			code = h2err.Code
		}
		s.emitGoaway(code)
		return err
	}

	// Step 4: ACK client SETTINGS.
	if err := s.fr.WriteSettingsAck(); err != nil {
		return err
	}

	// The reader goroutine starts HERE and can start no earlier: Step 1's
	// readClientPreface and Step 3's readClientSettings both read the socket
	// DIRECTLY. It can start no later either -- the dispatch loop below is the
	// first readFrameCtx caller.
	s.fr.startReader()

	// Frame dispatch loop.
	//
	// Design: after processing each frame we try a non-blocking read to detect
	// whether more frames arrived in the same TCP burst.  While the burst is
	// being drained, stream dispatch goroutines are queued in s.pendingDispatch
	// rather than launched immediately.  Once the burst is exhausted (the non-
	// blocking read returns nothing), all pending goroutines are launched.
	//
	// This guarantees that RST_STREAM(REFUSED_STREAM) for an overflow stream is
	// written to the wire BEFORE any DATA responses from the accepted streams,
	// satisfying h2spec 5.1.2/1 which reads the first response frame and checks
	// that it is RST_STREAM rather than DATA.
	for {
		// Check ctx before blocking on a read.
		if err := s.ctx.Err(); err != nil {
			s.emitGoaway(ErrNoError)
			return err
		}

		frame, err := s.fr.readFrameCtx(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				s.emitGoaway(ErrNoError)
				return s.ctx.Err()
			}
			// Framer-level protocol violations (e.g. FRAME_SIZE_ERROR on an
			// invalid PING) arrive here as *Error values.  Emit GOAWAY before
			// closing so h2spec sees the proper connection-error framing.
			// After sending GOAWAY we drain the socket so the peer's TCP stack
			// can flush its send buffer and receive our GOAWAY before we close.
			// Without the drain, closing with unread bytes in the socket buffer
			// triggers a TCP RST which overwrites the GOAWAY in-flight.
			var hErr *Error
			if asErr, ok := err.(*Error); ok {
				hErr = asErr
			}
			if hErr != nil && hErr.Stream == 0 {
				s.emitGoaway(hErr.Code)
				// See the note at the sibling drain below: joined before the drain
				// arms its deadline. A measured no-op at this tip, kept structural.
				s.fr.closeReader()
				_ = s.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				_, _ = io.Copy(io.Discard, s.conn)
				_ = s.conn.SetReadDeadline(time.Time{})
			}
			return err
		}

		if err := s.processFrameAndMaybeDrain(frame); err != nil {
			return err
		}
	}
}

// processFrameAndMaybeDrain processes first, then greedily reads and processes
// all immediately-available subsequent frames before flushing the pending
// dispatch queue.  Frames are processed one at a time (not buffered) to avoid
// the x/net/http2.Framer ownership constraint: each ReadFrame call invalidates
// the previous frame's internal buffers.  It returns the first terminal error.
func (s *ServerConn) processFrameAndMaybeDrain(first http2.Frame) error {
	// Process the first frame.
	if err := s.dispatchOneFrame(first); err != nil {
		return err
	}

	// Non-blocking read: process all frames that are immediately available.
	for {
		next, err := s.fr.tryReadFrame()
		if err != nil {
			// Frame-level error in non-blocking path.
			var hErr *Error
			if asErr, ok := err.(*Error); ok {
				hErr = asErr
			}
			if hErr != nil && hErr.Stream == 0 {
				s.flushPendingDispatch()
				s.emitGoaway(hErr.Code)
				// Join the reader before the drain arms its own deadline: the drain
				// needs the socket exclusively, and its 500 ms bound is only a bound
				// if no other goroutine can clear it. See ADR-0313 -- at this tip the
				// reader is already gone on every path that reaches here, so this is a
				// MEASURED no-op retained as a structural invariant, not a live guard.
				s.fr.closeReader()
				_ = s.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				_, _ = io.Copy(io.Discard, s.conn)
				_ = s.conn.SetReadDeadline(time.Time{})
			}
			return err
		}
		if next == nil {
			break // no more frames immediately available
		}
		if err := s.dispatchOneFrame(next); err != nil {
			return err
		}
	}

	// Burst exhausted — launch pending dispatches.
	s.flushPendingDispatch()
	return nil
}

// dispatchOneFrame is a helper that calls dispatchFrame and handles errors.
func (s *ServerConn) dispatchOneFrame(f http2.Frame) error {
	if err := s.dispatchFrame(f); err != nil {
		var hErr *Error
		if asErr, ok := err.(*Error); ok {
			hErr = asErr
		} else {
			hErr = &Error{Code: ErrInternalError, Underlying: err}
		}
		if hErr.Stream == 0 {
			// Connection-scoped error → flush pending, GOAWAY, and exit.
			s.flushPendingDispatch()
			s.emitGoaway(hErr.Code)
			return err
		}
		// Stream-scoped error → emit RST_STREAM and continue.
		_ = s.writeRSTStream(hErr.Stream, hErr.Code)
	}
	return nil
}

// flushPendingDispatch launches all queued dispatch goroutines and clears the
// pending queue.
func (s *ServerConn) flushPendingDispatch() {
	for _, fn := range s.pendingDispatch {
		go fn()
	}
	s.pendingDispatch = s.pendingDispatch[:0]
}

// dispatchFrame routes a single frame to the appropriate handler.
func (s *ServerConn) dispatchFrame(frame http2.Frame) error {
	switch f := frame.(type) {
	case *http2.HeadersFrame:
		return s.onHeaders(f)
	case *http2.ContinuationFrame:
		return s.onContinuation(f)
	case *http2.DataFrame:
		return s.onData(f)
	case *http2.SettingsFrame:
		return s.onSettings(f)
	case *http2.PingFrame:
		return s.onPing(f)
	case *http2.WindowUpdateFrame:
		return s.onWindowUpdate(f)
	case *http2.RSTStreamFrame:
		return s.onRSTStream(f)
	case *http2.GoAwayFrame:
		return s.onGoaway(f)
	case *http2.PushPromiseFrame:
		return connError(ErrProtocolError, "client cannot send PUSH_PROMISE")
	case *http2.PriorityFrame:
		return s.onPriority(f)
	default:
		// Unknown frame types are silently ignored per RFC 9113 §4.1.
		return nil
	}
}

// onHeaders handles an incoming HEADERS frame.
//
// A frame WITHOUT END_HEADERS opens a header-block accumulator and is neither
// decoded nor dispatched here: the rest of the block arrives as CONTINUATION
// frames (RFC 9113 §6.10) and onContinuation completes it. Before phase 88
// this method decoded the partial fragment and dispatched the partial field
// set, leaving the connection-scoped HPACK dynamic table diverged from the
// peer's encoder for the remaining life of the connection.
func (s *ServerConn) onHeaders(f *http2.HeadersFrame) error {
	if !f.HeadersEnded() {
		s.hdrAccum = startHeaderBlock(f)
		return nil
	}
	return s.onHeaderBlock(f.Header().StreamID, f.HeaderBlockFragment(), f.StreamEnded(), f.HasPriority(), f.Priority)
}

// onContinuation accumulates one CONTINUATION fragment (RFC 9113 §6.10),
// completing the block once END_HEADERS arrives. A rejected fragment clears
// the accumulator; the reject is connection-scoped, so the connection is going
// away regardless.
func (s *ServerConn) onContinuation(f *http2.ContinuationFrame) error {
	if err := appendContinuation(s.hdrAccum, f); err != nil {
		s.hdrAccum = nil
		return err
	}
	if !f.HeadersEnded() {
		return nil
	}
	a := s.hdrAccum
	s.hdrAccum = nil
	return s.onHeaderBlock(a.streamID, a.buf, a.endStream, a.hasPriority, a.priority)
}

// onHeaderBlock processes one COMPLETE request header block — the whole of it,
// whether it arrived in a single HEADERS frame or was reassembled from a
// HEADERS plus CONTINUATION frames. endStream, hasPriority and priority come
// from the HEADERS frame that opened the block.
func (s *ServerConn) onHeaderBlock(streamID uint32, block []byte, endStream, hasPriority bool, priority http2.PriorityParam) error {
	// Drain any goroutine completions first so that s.streams and s.closedStreams
	// are up to date before we inspect stream state.  In particular, a stream
	// that was dispatched and completed before this HEADERS frame arrived will be
	// moved from s.streams to s.closedStreams by drainDone, allowing the closed-
	// stream error check below (h2spec 5.1/12) and the concurrency check to work
	// correctly.
	s.drainDone()

	// Validate / decode headers. The block is complete by construction, so the
	// decoder is always closed at the block boundary.
	headers, err := s.hpack.decodeBlock(block, true)
	if err != nil {
		return connError(ErrCompressionError, "HPACK decode failed")
	}

	s.mu.Lock()
	existing, isExisting := s.streams[streamID]
	_, isClosed := s.closedStreams[streamID]
	s.mu.Unlock()

	if isExisting {
		// Existing stream: check stream state first.
		// RFC 9113 §5.1: HEADERS on a half-closed-remote stream is a stream error.
		// Trailers (HEADERS after DATA in open state) are discarded per SPEC §2.1.
		// We still validate: trailing HEADERS MUST NOT contain pseudo-headers
		// (RFC 9113 §8.1.2.1 — pseudo-headers only valid in first HEADERS block).
		if err := existing.recvTrailingHeaders(headers, endStream); err != nil {
			return err
		}
		if endStream {
			// Trailing HEADERS carried END_STREAM: the request is now complete,
			// so queue the dispatch exactly as the HEADERS/DATA END_STREAM paths
			// do (the trailer fields themselves were observed-and-discarded
			// above per SPEC §2.1). Without this the stream would never be
			// dispatched: no response would be written and the stream would
			// occupy its MAX_CONCURRENT_STREAMS slot forever.
			ctx := s.ctx
			s.pendingDispatch = append(s.pendingDispatch, func() {
				existing.dispatch(ctx, s.dispatcher)
				s.doneCh <- streamID
			})
		}
		return nil
	}

	if isClosed {
		// RFC 9113 §5.1: HEADERS on a closed stream that the server closed
		// (i.e., we fully handled it) is a connection error STREAM_CLOSED.
		// Using streamError here is wrong per the RFC and causes h2spec 5.1/12
		// to fail (it expects a GOAWAY, not RST_STREAM).
		return connError(ErrStreamClosed, "HEADERS on closed stream")
	}

	// Reject new streams if the peer has sent GOAWAY (they are going away and
	// should not be opening new streams).
	s.mu.Lock()
	peerGone := s.peerGoaway
	s.mu.Unlock()
	if peerGone {
		return streamError(ErrRefusedStream, streamID, "peer sent GOAWAY; rejecting new stream")
	}

	// New stream: validate stream ID (even-numbered or reuse is PROTOCOL_ERROR).
	if streamID%2 == 0 {
		return connError(ErrProtocolError, "even client stream id")
	}

	// RFC 9113 §5.1.1: stream IDs must be monotonically increasing.
	s.mu.Lock()
	lastID := s.lastInID
	s.mu.Unlock()
	if streamID <= lastID {
		return connError(ErrProtocolError, "stream id not monotonically increasing")
	}

	// RFC 9113 §5.3.1: a stream cannot depend on itself. Check PRIORITY flag.
	if hasPriority && priority.StreamDep == streamID {
		return streamError(ErrProtocolError, streamID, "HEADERS self-dependency")
	}

	// Construct the new stream before taking the lock.
	//
	// Per-stream send-window initial size is the PEER-announced
	// SETTINGS_INITIAL_WINDOW_SIZE (RFC 9113 §6.9.2: this is the limit on data
	// WE can send into a stream). Default to 65535 if the peer hasn't sent
	// the setting. The recv window initial size is OUR own announced value
	// (governs how much the peer can send us before WINDOW_UPDATE).
	peerInitWindow := int32(s.clientS.InitialWindowSize)
	if !s.clientS.InitialWindowSizeAnnounced {
		// The client has never sent SETTINGS_INITIAL_WINDOW_SIZE — default
		// per RFC 9113 §6.5.2. An explicitly announced 0 is a deliberate
		// "hold all response DATA" instruction and must NOT be conflated
		// with this default (phase 85 quirk fix; must share this exact
		// computation with onSettings's walk effective-old, or h2spec
		// 6.9.2/1 reds).
		peerInitWindow = 65535
	}
	ss := newServerStream(streamID, s, peerInitWindow, int32(s.settings.InitialWindowSize))
	if err := ss.recvHeaders(headers, endStream); err != nil {
		return err
	}

	// Enforce MaxConcurrentStreams.  We check len(s.streams) under the mutex so
	// the check and the insertion are atomic with respect to the drain above.
	// We use only s.settings.MaxConcurrentStreams (what WE advertised);
	// s.clientS.MaxConcurrentStreams is the limit on server-push streams
	// (RFC 9113 §6.5.2) and is not relevant here.
	maxConc := s.settings.MaxConcurrentStreams
	s.mu.Lock()
	if uint32(len(s.streams)) >= maxConc {
		s.mu.Unlock()
		// Too many concurrent streams → RST_STREAM(REFUSED_STREAM).
		return streamError(ErrRefusedStream, streamID, "max concurrent streams exceeded")
	}
	s.streams[streamID] = ss
	if streamID > s.lastInID {
		s.lastInID = streamID
	}
	s.mu.Unlock()

	if endStream {
		// Queue the dispatch for deferred launch.  The frame loop will launch
		// all pending dispatches once the current TCP burst is exhausted (no
		// more frames immediately available), ensuring that RST_STREAM for an
		// overflow stream is written before any DATA response is emitted.
		ctx := s.ctx
		s.pendingDispatch = append(s.pendingDispatch, func() {
			ss.dispatch(ctx, s.dispatcher)
			// Signal the frame loop that this stream is done.
			s.doneCh <- streamID
		})
	}

	return nil
}

// onData handles an incoming DATA frame.
func (s *ServerConn) onData(f *http2.DataFrame) error {
	streamID := f.Header().StreamID

	// Drain goroutine completions so that closedStreams is up to date.
	s.drainDone()

	s.mu.Lock()
	ss, okActive := s.streams[streamID]
	_, okClosed := s.closedStreams[streamID]
	lastID := s.lastInID
	s.mu.Unlock()

	if !okActive {
		// RFC 9113 §5.1: DATA on an idle stream (never seen before AND below
		// lastInID) is a connection error PROTOCOL_ERROR.
		// DATA on a closed stream is also a stream error STREAM_CLOSED.
		if streamID > lastID {
			// Never-seen stream ID — idle state → connection error PROTOCOL_ERROR.
			return connError(ErrProtocolError, "DATA on idle stream")
		}
		if okClosed {
			// Recently-closed stream — stream error STREAM_CLOSED.
			return streamError(ErrStreamClosed, streamID, "DATA on closed stream")
		}
		// Otherwise (stream we don't know about) → STREAM_CLOSED stream error.
		return streamError(ErrStreamClosed, streamID, "DATA on unknown stream")
	}

	dataLen := int32(len(f.Data()))

	if err := ss.recvData(f.Data(), f.StreamEnded()); err != nil {
		return err
	}

	// ADR-0055 I-3: receive-side flow control. Debit both the conn-level and
	// per-stream recv windows by the DATA payload length. When the running
	// debit counter crosses the half-window high-water mark, emit a
	// WINDOW_UPDATE so the peer can keep sending. Without this, a peer that
	// streams a body larger than 65535 bytes will stall once the initial
	// window is exhausted.
	if dataLen > 0 {
		// Conn-level debit + WINDOW_UPDATE(0, delta). Mutates s.recvDebitSinceLastUpdate
		// (the *ServerConn field).
		s.recvW.replenish(-dataLen)
		s.recvDebitSinceLastUpdate += dataLen
		if s.recvDebitSinceLastUpdate >= recvWindowUpdateThreshold {
			delta := s.recvDebitSinceLastUpdate
			s.recvDebitSinceLastUpdate = 0
			s.recvW.replenish(delta)
			s.mu.Lock()
			werr := s.fr.WriteWindowUpdate(0, uint32(delta))
			s.mu.Unlock()
			if werr != nil {
				return translateFramerErr(werr)
			}
		}

		// Stream-level debit + WINDOW_UPDATE(streamID, delta). Independent
		// from the conn-level block above; mutates ss.recvDebitSinceLastUpdate
		// (the *serverStream field — different field, same spelling).
		ss.recvW.replenish(-dataLen)
		ss.recvDebitSinceLastUpdate += dataLen
		// Only emit a per-stream WINDOW_UPDATE while the stream is still
		// half-open on the remote side (peer can still send DATA on this
		// stream). Once we've seen END_STREAM, no further DATA can arrive on
		// this stream and a WINDOW_UPDATE would be wasted.
		if !f.StreamEnded() && ss.recvDebitSinceLastUpdate >= recvWindowUpdateThreshold {
			delta := ss.recvDebitSinceLastUpdate
			ss.recvDebitSinceLastUpdate = 0
			ss.recvW.replenish(delta)
			s.mu.Lock()
			werr := s.fr.WriteWindowUpdate(streamID, uint32(delta))
			s.mu.Unlock()
			if werr != nil {
				return translateFramerErr(werr)
			}
		}
	}

	if f.StreamEnded() {
		// Queue the dispatch for deferred launch (see onHeaders for rationale).
		ctx := s.ctx
		s.pendingDispatch = append(s.pendingDispatch, func() {
			ss.dispatch(ctx, s.dispatcher)
			s.doneCh <- streamID
		})
	}

	return nil
}

// onSettings handles an incoming SETTINGS frame.
//
// Validation runs strictly before application (phase 85): a first
// ForeachSetting pass runs validateSetting over every parameter (first error
// wins, captured outside the closure); only if that pass finds nothing wrong
// does a second pass apply the values, so an invalid frame applies none of
// its values. The apply pass's INITIAL_WINDOW_SIZE case additionally "walks"
// every live stream's send window by the delta (RFC 9113 §6.9.2) — a change
// discovered only during application (it depends on the streams open at
// apply time), so it is reported via walkErr rather than the pre-pass.
func (s *ServerConn) onSettings(f *http2.SettingsFrame) error {
	if f.IsAck() {
		// ACK for our server-initial SETTINGS — discard.
		return nil
	}

	var validateErr *Error
	_ = f.ForeachSetting(func(setting http2.Setting) error {
		if validateErr == nil {
			validateErr = validateSetting(setting)
		}
		return nil
	})
	if validateErr != nil {
		return validateErr
	}

	// Apply new settings.
	var walkErr error
	_ = f.ForeachSetting(func(setting http2.Setting) error {
		switch setting.ID {
		case http2.SettingMaxConcurrentStreams:
			s.clientS.MaxConcurrentStreams = setting.Val
		case http2.SettingInitialWindowSize:
			// Effective previous value MUST be computed the same way stream
			// seeding computes it (the announced-flag form in onHeaders) or
			// never-announced connections walk with a wrong delta.
			old := int32(s.clientS.InitialWindowSize)
			if !s.clientS.InitialWindowSizeAnnounced {
				old = 65535
			}
			s.clientS.InitialWindowSize = setting.Val
			s.clientS.InitialWindowSizeAnnounced = true
			if delta := int32(setting.Val) - old; delta != 0 {
				s.mu.Lock()
				for id, ss := range s.streams {
					if !ss.sendW.adjust(delta) {
						s.mu.Unlock()
						walkErr = connError(ErrFlowControlError,
							fmt.Sprintf("SETTINGS_INITIAL_WINDOW_SIZE adjustment overflows stream %d send window", id))
						return nil
					}
				}
				s.mu.Unlock()
			}
		case http2.SettingMaxFrameSize:
			s.clientS.MaxFrameSize = setting.Val
		case http2.SettingHeaderTableSize:
			s.clientS.HeaderTableSize = setting.Val
			// Propagate the peer's SETTINGS_HEADER_TABLE_SIZE to our encoder so
			// outgoing HEADERS blocks respect the new limit.
			s.hpack.updateMaxTableSize(setting.Val)
		case http2.SettingEnablePush:
			s.clientS.EnablePush = setting.Val
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	// ACK the peer's SETTINGS.
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fr.WriteSettingsAck()
}

// onPing handles an incoming PING frame.
func (s *ServerConn) onPing(f *http2.PingFrame) error {
	if f.IsAck() {
		// Unsolicited PING ACK — discard.
		return nil
	}
	// Respond with PING ACK carrying the same 8-byte payload.
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fr.WritePing(true, f.Data)
}

// onWindowUpdate handles an incoming WINDOW_UPDATE frame.
//
// RFC 9113 §6.9.1: a WINDOW_UPDATE that would push the receiver's flow-control
// window past 2^31-1 is treated as a flow-control error — connection-scoped
// (GOAWAY) for stream id 0, stream-scoped (RST_STREAM) for a non-zero stream.
// RFC 9113 §6.9 also forbids delta == 0: connection-level → connection error,
// stream-level → stream error, both PROTOCOL_ERROR.
func (s *ServerConn) onWindowUpdate(f *http2.WindowUpdateFrame) error {
	delta := int32(f.Increment)
	if f.StreamID == 0 {
		// Connection-level WINDOW_UPDATE.
		if delta == 0 {
			return connError(ErrProtocolError, "WINDOW_UPDATE delta 0 on connection")
		}
		// Bounds check: the resulting window must fit in int32 per RFC 9113 §6.9.1.
		cur := s.sendW.available()
		if _, ok := safeAddInt32(cur, delta); !ok {
			return connError(ErrFlowControlError, "WINDOW_UPDATE conn-level overflow")
		}
		s.sendW.replenish(delta)
		return nil
	}

	// Stream-level WINDOW_UPDATE.
	s.mu.Lock()
	ss, okActive := s.streams[f.StreamID]
	lastID := s.lastInID
	s.mu.Unlock()

	if !okActive {
		// RFC 9113 §5.1: WINDOW_UPDATE on an idle stream is a connection error
		// PROTOCOL_ERROR. On a closed stream it's silently ignored.
		if f.StreamID > lastID {
			return connError(ErrProtocolError, "WINDOW_UPDATE on idle stream")
		}
		// Closed/unknown stream — silently ignore.
		return nil
	}

	return ss.recvWindowUpdate(delta)
}

// onRSTStream handles an incoming RST_STREAM frame.
func (s *ServerConn) onRSTStream(f *http2.RSTStreamFrame) error {
	s.mu.Lock()
	ss, okActive := s.streams[f.StreamID]
	lastID := s.lastInID
	if okActive {
		delete(s.streams, f.StreamID)
		s.closedStreams[f.StreamID] = struct{}{}
	}
	s.mu.Unlock()

	if !okActive {
		// RST_STREAM on idle stream → connection error PROTOCOL_ERROR.
		// RST_STREAM on closed stream → silently ignore.
		if f.StreamID > lastID {
			return connError(ErrProtocolError, "RST_STREAM on idle stream")
		}
		return nil
	}
	return ss.recvRSTStream(ErrCode(f.ErrCode))
}

// onGoaway handles an incoming GOAWAY frame from the client.
// RFC 9113 §6.8: MUST NOT trigger special behavior for unknown error codes.
// We mark the connection so new streams are no longer accepted, but continue
// processing existing frames (including PING ACKs) until the peer closes.
// The frame loop will exit naturally on the subsequent EOF.
func (s *ServerConn) onGoaway(_ *http2.GoAwayFrame) error {
	s.mu.Lock()
	s.peerGoaway = true
	s.mu.Unlock()
	return nil // continue the frame loop; exit will happen on peer EOF
}

// onPriority handles an incoming PRIORITY frame. RFC 9113 deprecated PRIORITY
// frames; phase 05.1 silently discards them except for the self-dependency
// rule (RFC 9113 §5.3.1): a stream that depends on itself MUST be treated as a
// stream error of type PROTOCOL_ERROR.
func (s *ServerConn) onPriority(f *http2.PriorityFrame) error {
	dep := f.PriorityParam.StreamDep
	if dep == f.Header().StreamID {
		return streamError(ErrProtocolError, f.Header().StreamID, "PRIORITY self-dependency")
	}
	// All other PRIORITY frames are silently discarded per SPEC §2.1.
	return nil
}

// drainDone processes any pending goroutine-completion signals from doneCh.
// It removes the signaled stream IDs from s.streams and adds them to
// s.closedStreams.  Must be called from the frame-loop goroutine only.
//
// Rationale: dispatch goroutines send to doneCh instead of directly
// mutating s.streams, ensuring that the frame loop (which drives the
// MAX_CONCURRENT_STREAMS admission check) decides when a stream is retired
// from the active count.  Draining immediately before the admission check
// guarantees that any goroutines that finished before the current HEADERS
// frame arrived are reflected in len(s.streams).
func (s *ServerConn) drainDone() {
	for {
		select {
		case id := <-s.doneCh:
			s.mu.Lock()
			delete(s.streams, id)
			s.closedStreams[id] = struct{}{}
			s.mu.Unlock()
		default:
			return
		}
	}
}

// emitGoaway sends a GOAWAY frame once. Subsequent calls are no-ops.
// The mutex is held across the framer write so that the write is
// serialized against encodeAndWriteHeaders / writeData.
func (s *ServerConn) emitGoaway(code ErrCode) {
	s.mu.Lock()
	if s.goawaySent {
		s.mu.Unlock()
		return
	}
	s.goawaySent = true
	lastID := s.lastInID
	_ = s.fr.WriteGoAway(lastID, http2.ErrCode(code), nil)
	s.mu.Unlock()
}

// encodeAndWriteHeaders implements streamConn. Called from serverStream.WriteHeaders.
func (s *ServerConn) encodeAndWriteHeaders(streamID uint32, headers []hpack.HeaderField, endStream bool) error {
	// Serialize via the connection-level mutex to prevent interleaved frames.
	s.mu.Lock()
	defer s.mu.Unlock()
	encoded := s.hpack.encodeHeaders(headers)
	// Make a copy of the encoded bytes because encBuf is reused.
	block := make([]byte, len(encoded))
	copy(block, encoded)
	// Split into HEADERS + CONTINUATION frames bounded by the peer's advertised
	// SETTINGS_MAX_FRAME_SIZE (RFC 9113 §6.10). Emitting the whole block as one
	// frame — what this call site did before phase 88 — is illegal as soon as
	// the encoded block exceeds the peer's limit, and a conforming peer answers
	// with a FRAME_SIZE_ERROR instead of the response.
	return s.fr.writeHeaderBlock(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block,
		EndStream:     endStream,
	}, int32(s.clientS.MaxFrameSize))
}

// writeData implements streamConn. Called from serverStream.WriteData.
// ADR-0055 chunking: each DATA frame is bounded by min(connSendWindow,
// streamSendWindow, peer.MaxFrameSize). The per-stream window is reserved
// FIRST (smaller bound first reduces head-of-line blocking on the conn-level
// window); the conn-level window is reserved second for the streamTaken
// amount; on conn-level under-reservation we replenish the stream-level
// over-reservation so per-stream window state stays consistent.
func (s *ServerConn) writeData(streamID uint32, b []byte, endStream bool) error {
	ctx := s.ctx

	// Resolve the per-stream send-window. RFC 9113 §5.1: a stream that has
	// closed during dispatch may already be removed from s.streams; we treat a
	// missing stream as a no-op since the caller's response is no longer
	// deliverable. (This shouldn't happen in normal flow — dispatch holds the
	// stream live — but the lookup keeps the function defensive.)
	s.mu.Lock()
	ss, ok := s.streams[streamID]
	s.mu.Unlock()
	if !ok {
		return nil
	}

	// Peer SETTINGS_MAX_FRAME_SIZE cap. RFC 9113 §6.5.2 default is 16384 if
	// the peer has not yet sent SETTINGS (s.clientS.MaxFrameSize == 0).
	maxFrame := int32(s.clientS.MaxFrameSize)
	if maxFrame <= 0 {
		maxFrame = defaultPeerMaxFrameSize
	}

	remaining := b
	for len(remaining) > 0 {
		want := int32(len(remaining))
		if want > maxFrame {
			want = maxFrame
		}

		// Reserve per-stream first (smaller bound reduces head-of-line
		// blocking on the conn-level window).
		streamTaken, err := ss.sendW.reserveBlocking(ctx, want)
		if err != nil {
			return err
		}

		// Reserve conn-level for the streamTaken amount.
		connTaken, err := s.sendW.reserveBlocking(ctx, streamTaken)
		if err != nil {
			// ctx canceled (or similar) — replenish the per-stream over-
			// reservation so the window state stays accurate. Do NOT replenish
			// more than streamTaken.
			ss.sendW.replenish(streamTaken)
			return err
		}
		// If conn-level returned less than streamTaken, replenish the
		// difference back to the per-stream window so we only consume what we
		// will actually write.
		if connTaken < streamTaken {
			ss.sendW.replenish(streamTaken - connTaken)
		}

		chunk := remaining[:connTaken]
		remaining = remaining[connTaken:]
		isLast := len(remaining) == 0 && endStream

		s.mu.Lock()
		werr := s.fr.WriteData(streamID, isLast, chunk)
		s.mu.Unlock()
		if werr != nil {
			return translateFramerErr(werr)
		}
	}

	// Empty body with endStream: emit a zero-length DATA frame with END_STREAM.
	// The chunking loop above is skipped when len(b) == 0; the per-stream and
	// conn-level send windows do not constrain a zero-length frame.
	if len(b) == 0 && endStream {
		s.mu.Lock()
		err := s.fr.WriteData(streamID, true, nil)
		s.mu.Unlock()
		return translateFramerErr(err)
	}
	return nil
}

// writeRSTStream implements streamConn. Called from serverStream.dispatch on error.
func (s *ServerConn) writeRSTStream(streamID uint32, code ErrCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fr.WriteRSTStream(streamID, http2.ErrCode(code))
}
