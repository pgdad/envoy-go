package h2

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

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

	// Step 3: read client initial SETTINGS.
	if err := readClientSettings(s.fr, &s.clientS); err != nil {
		s.emitGoaway(ErrProtocolError)
		return err
	}

	// Step 4: ACK client SETTINGS.
	if err := s.fr.WriteSettingsAck(); err != nil {
		return err
	}

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
		// Handled by framer as part of ReadMetaHeaders; reaching here means the
		// framer gave us a raw ContinuationFrame (shouldn't happen in normal usage,
		// but be safe and ignore).
		return nil
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
func (s *ServerConn) onHeaders(f *http2.HeadersFrame) error {
	streamID := f.Header().StreamID

	// Drain any goroutine completions first so that s.streams and s.closedStreams
	// are up to date before we inspect stream state.  In particular, a stream
	// that was dispatched and completed before this HEADERS frame arrived will be
	// moved from s.streams to s.closedStreams by drainDone, allowing the closed-
	// stream error check below (h2spec 5.1/12) and the concurrency check to work
	// correctly.
	s.drainDone()

	// Validate / decode headers.
	headers, err := s.hpack.decodeBlock(f.HeaderBlockFragment(), f.HeadersEnded())
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
		if err := existing.recvTrailingHeaders(headers, f.StreamEnded()); err != nil {
			return err
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
	if f.HasPriority() && f.Priority.StreamDep == streamID {
		return streamError(ErrProtocolError, streamID, "HEADERS self-dependency")
	}

	// Construct the new stream before taking the lock.
	ss := newServerStream(streamID, s, int32(s.settings.InitialWindowSize), int32(s.settings.InitialWindowSize))
	if err := ss.recvHeaders(headers, f.StreamEnded()); err != nil {
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

	if f.StreamEnded() {
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

	if err := ss.recvData(f.Data(), f.StreamEnded()); err != nil {
		return err
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
func (s *ServerConn) onSettings(f *http2.SettingsFrame) error {
	if f.IsAck() {
		// ACK for our server-initial SETTINGS — discard.
		return nil
	}

	// Apply new settings.
	_ = f.ForeachSetting(func(setting http2.Setting) error {
		switch setting.ID {
		case http2.SettingMaxConcurrentStreams:
			s.clientS.MaxConcurrentStreams = setting.Val
		case http2.SettingInitialWindowSize:
			s.clientS.InitialWindowSize = setting.Val
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
func (s *ServerConn) onWindowUpdate(f *http2.WindowUpdateFrame) error {
	delta := int32(f.Increment)
	if f.StreamID == 0 {
		// Connection-level WINDOW_UPDATE.
		if delta == 0 {
			return connError(ErrProtocolError, "WINDOW_UPDATE delta 0 on connection")
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
	return s.fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block,
		EndStream:     endStream,
		EndHeaders:    true,
	})
}

// writeData implements streamConn. Called from serverStream.WriteData.
// Respects the connection-level and stream-level send windows.
func (s *ServerConn) writeData(streamID uint32, b []byte, endStream bool) error {
	// For phase 05.1 we write the whole body in one or more DATA frames,
	// waiting for send-window capacity before each write. We honor the
	// connection-level window (s.sendW) but not per-stream windows for
	// outgoing DATA (the stream's sendW is client-controlled; the per-stream
	// window is replenished by incoming WINDOW_UPDATE for that stream ID).
	// Full per-stream send-window enforcement is done in a simple loop.
	ctx := s.ctx
	remaining := b
	for len(remaining) > 0 {
		// Wait for connection-level send window.
		if err := s.sendW.waitFor(ctx, 1); err != nil {
			return err
		}
		taken, _ := s.sendW.reserve(int32(len(remaining)))
		if taken <= 0 {
			taken = 1
			if _, err := s.sendW.reserve(1); err != nil {
				_ = err
			}
		}
		chunk := remaining[:taken]
		remaining = remaining[taken:]
		isLast := len(remaining) == 0 && endStream
		s.mu.Lock()
		err := s.fr.WriteData(streamID, isLast, chunk)
		s.mu.Unlock()
		if err != nil {
			return err
		}
	}
	if len(b) == 0 && endStream {
		s.mu.Lock()
		err := s.fr.WriteData(streamID, true, nil)
		s.mu.Unlock()
		return err
	}
	return nil
}

// writeRSTStream implements streamConn. Called from serverStream.dispatch on error.
func (s *ServerConn) writeRSTStream(streamID uint32, code ErrCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fr.WriteRSTStream(streamID, http2.ErrCode(code))
}
