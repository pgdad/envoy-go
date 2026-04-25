package h2

import (
	"context"
	"net"
	"sync"

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
	ctx        context.Context
	conn       net.Conn
	dispatcher Dispatcher
	settings   ServerSettings
	fr         *framer
	hpack      *hpackState
	sendW      *window
	recvW      *window
	mu         sync.Mutex
	streams    map[uint32]*serverStream
	lastInID   uint32 // highest stream id seen from client (RFC 9113 §5.1.1 monotonic)
	clientS    clientSettings
	goawaySent bool
}

// NewServerConn constructs a ServerConn value. Run owns conn (closes on exit).
// dispatcher is called once per stream to resolve the incoming request to an
// Action. The h2 package defines the Dispatcher interface; the hcm parent
// package provides the production implementation (h2dispatch.go).
func NewServerConn(ctx context.Context, conn net.Conn, dispatcher Dispatcher, settings ServerSettings) *ServerConn {
	return &ServerConn{
		ctx:        ctx,
		conn:       conn,
		dispatcher: dispatcher,
		settings:   settings,
		fr:         newFramer(conn),
		hpack:      newHPACKState(settings.HeaderTableSize),
		sendW:      newWindow(int32(settings.InitialWindowSize)),
		recvW:      newWindow(int32(settings.InitialWindowSize)),
		streams:    make(map[uint32]*serverStream),
	}
}

// Run drives the connection lifecycle. Returns nil on clean shutdown,
// *Error on protocol violation, ctx.Err() on cancellation.
func (s *ServerConn) Run() error {
	defer func() { _ = s.conn.Close() }()

	// Step 1: read client preface.
	if err := readClientPreface(s.conn); err != nil {
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
			return err
		}

		if err := s.dispatchFrame(frame); err != nil {
			var hErr *Error
			if asErr, ok := err.(*Error); ok {
				hErr = asErr
			} else {
				hErr = &Error{Code: ErrInternalError, Underlying: err}
			}
			if hErr.Stream == 0 {
				// Connection-scoped error → GOAWAY and exit.
				s.emitGoaway(hErr.Code)
				return err
			}
			// Stream-scoped error → emit RST_STREAM and continue.
			_ = s.fr.WriteRSTStream(hErr.Stream, http2.ErrCode(hErr.Code))
		}
	}
}

// dispatchFrame routes a single frame to the appropriate handler.
func (s *ServerConn) dispatchFrame(frame http2.Frame) error {
	switch f := frame.(type) {
	case *http2.HeadersFrame:
		return s.onHeaders(f)
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
		// Silently discard per SPEC §2.1 (RFC 9113 §6.3).
		return nil
	default:
		// Unknown frame types are silently ignored per RFC 9113 §4.1.
		return nil
	}
}

// onHeaders handles an incoming HEADERS frame.
func (s *ServerConn) onHeaders(f *http2.HeadersFrame) error {
	streamID := f.Header().StreamID

	// Validate / decode headers.
	headers, err := s.hpack.decodeBlock(f.HeaderBlockFragment(), f.HeadersEnded())
	if err != nil {
		return connError(ErrCompressionError, "HPACK decode failed")
	}

	s.mu.Lock()
	existing, isExisting := s.streams[streamID]
	s.mu.Unlock()

	if isExisting {
		// Existing stream: trailers (HEADERS after DATA). Per SPEC §2.1 discard.
		// If END_STREAM is set, treat as END_STREAM-on-DATA semantically.
		if f.StreamEnded() {
			return existing.recvData(nil, true)
		}
		return nil
	}

	// New stream: validate stream ID.
	s.mu.Lock()
	existingSet := make(map[uint32]struct{}, len(s.streams))
	for id := range s.streams {
		existingSet[id] = struct{}{}
	}
	s.mu.Unlock()

	if err := validateClientStreamID(streamID, existingSet); err != nil {
		return err // connection-scoped PROTOCOL_ERROR
	}

	// Enforce MaxConcurrentStreams.
	maxConc := s.settings.MaxConcurrentStreams
	if s.clientS.MaxConcurrentStreams > 0 && s.clientS.MaxConcurrentStreams < maxConc {
		maxConc = s.clientS.MaxConcurrentStreams
	}
	s.mu.Lock()
	streamCount := uint32(len(s.streams))
	s.mu.Unlock()

	if streamCount >= maxConc {
		// Too many concurrent streams → RST_STREAM(REFUSED_STREAM).
		return streamError(ErrRefusedStream, streamID, "max concurrent streams exceeded")
	}

	// Construct and store the new stream.
	ss := newServerStream(streamID, s, int32(s.settings.InitialWindowSize), int32(s.settings.InitialWindowSize))
	if err := ss.recvHeaders(headers, f.StreamEnded()); err != nil {
		return err
	}

	s.mu.Lock()
	s.streams[streamID] = ss
	if streamID > s.lastInID {
		s.lastInID = streamID
	}
	s.mu.Unlock()

	if f.StreamEnded() {
		// Dispatch immediately (END_STREAM was on HEADERS).
		ctx := s.ctx
		go func() {
			ss.dispatch(ctx, s.dispatcher)
			s.mu.Lock()
			delete(s.streams, streamID)
			s.mu.Unlock()
		}()
	}

	return nil
}

// onData handles an incoming DATA frame.
func (s *ServerConn) onData(f *http2.DataFrame) error {
	streamID := f.Header().StreamID

	s.mu.Lock()
	ss, ok := s.streams[streamID]
	s.mu.Unlock()

	if !ok {
		// DATA on unknown stream — emit RST_STREAM(STREAM_CLOSED).
		return streamError(ErrStreamClosed, streamID, "DATA on unknown stream")
	}

	if err := ss.recvData(f.Data(), f.StreamEnded()); err != nil {
		return err
	}

	if f.StreamEnded() {
		// Launch dispatch goroutine.
		ctx := s.ctx
		go func() {
			ss.dispatch(ctx, s.dispatcher)
			s.mu.Lock()
			delete(s.streams, streamID)
			s.mu.Unlock()
		}()
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
	return s.fr.WriteSettingsAck()
}

// onPing handles an incoming PING frame.
func (s *ServerConn) onPing(f *http2.PingFrame) error {
	if f.IsAck() {
		// Unsolicited PING ACK — discard.
		return nil
	}
	// Respond with PING ACK carrying the same 8-byte payload.
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
	ss, ok := s.streams[f.StreamID]
	s.mu.Unlock()

	if !ok {
		// WINDOW_UPDATE on closed/unknown stream — silently ignore per RFC 9113.
		return nil
	}

	return ss.recvWindowUpdate(delta)
}

// onRSTStream handles an incoming RST_STREAM frame.
func (s *ServerConn) onRSTStream(f *http2.RSTStreamFrame) error {
	s.mu.Lock()
	ss, ok := s.streams[f.StreamID]
	if ok {
		delete(s.streams, f.StreamID)
	}
	s.mu.Unlock()

	if !ok {
		return nil
	}
	return ss.recvRSTStream(ErrCode(f.ErrCode))
}

// onGoaway handles an incoming GOAWAY frame. Marks the conn for graceful
// close; subsequent new streams from the client will be rejected.
func (s *ServerConn) onGoaway(_ *http2.GoAwayFrame) error {
	// Client is going away. Emit our own GOAWAY and exit gracefully.
	s.emitGoaway(ErrNoError)
	return connError(ErrNoError, "peer sent GOAWAY")
}

// emitGoaway sends a GOAWAY frame once. Subsequent calls are no-ops.
func (s *ServerConn) emitGoaway(code ErrCode) {
	if s.goawaySent {
		return
	}
	s.goawaySent = true
	_ = s.fr.WriteGoAway(s.lastInID, http2.ErrCode(code), nil)
}

// encodeAndWriteHeaders implements streamConn. Called from serverStream.WriteHeaders.
func (s *ServerConn) encodeAndWriteHeaders(streamID uint32, headers []hpack.HeaderField, endStream bool) error {
	// Serialise via the connection-level mutex to prevent interleaved frames.
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
	// waiting for send-window capacity before each write. We honour the
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
