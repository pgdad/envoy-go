package h2

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"golang.org/x/net/http2/hpack"
)

// streamState is the RFC 9113 §5.1 per-stream state machine value.
type streamState int

const (
	streamIdle             streamState = iota
	streamOpen             streamState = iota
	streamHalfClosedRemote streamState = iota
	streamHalfClosedLocal  streamState = iota
	streamClosed           streamState = iota
)

// StreamWriter is the interface a DirectResponseDispatcher calls to emit
// response frames back to the peer. The concrete implementation is
// *serverStream itself (accessible to the action via the DirectResponseDispatcher
// interface seam in stream.go).
type StreamWriter interface {
	WriteHeaders(headers []hpack.HeaderField, endStream bool) error
	WriteData(b []byte, endStream bool) error
}

// DirectResponseDispatcher is the interface that a direct-response action
// satisfies in order to be invoked on an H2 stream. Task 10 will implement
// this in the hcm parent package; for now the tests provide a fake.
type DirectResponseDispatcher interface {
	WriteH2(sw StreamWriter) error
}

// hcmAction is the opaque type returned from the lookup function passed to
// dispatch. Using interface{} keeps stream.go free of any import of the
// parent hcm package.
type hcmAction interface{}

// streamConn is the minimum surface serverStream needs from ServerConn.
type streamConn interface {
	encodeAndWriteHeaders(streamID uint32, headers []hpack.HeaderField, endStream bool) error
	writeData(streamID uint32, b []byte, endStream bool) error
	writeRSTStream(streamID uint32, code ErrCode) error
}

// serverStream is one HTTP/2 server-side stream. recvX methods are called
// from the ServerConn's frame loop. dispatch runs in its own goroutine.
// The mu mutex guards state only; the pipe naturally serialises body access.
type serverStream struct {
	id    uint32
	mu    sync.Mutex
	state streamState

	sendW *window
	recvW *window

	reqHeaders []hpack.HeaderField
	reqBodyR   *io.PipeReader
	reqBodyW   *io.PipeWriter

	conn streamConn
}

// newServerStream constructs a serverStream in the idle state.
func newServerStream(id uint32, conn streamConn, initialSendWindow, initialRecvWindow int32) *serverStream {
	pr, pw := io.Pipe()
	return &serverStream{
		id:       id,
		state:    streamIdle,
		sendW:    newWindow(initialSendWindow),
		recvW:    newWindow(initialRecvWindow),
		reqBodyR: pr,
		reqBodyW: pw,
		conn:     conn,
	}
}

// transition sets the stream state under the mutex.
func (s *serverStream) transition(to streamState) {
	s.mu.Lock()
	s.state = to
	s.mu.Unlock()
}

// recvHeaders processes an incoming HEADERS frame. Valid from idle (→ open or
// halfClosedRemote if endStream) or open (→ halfClosedRemote if endStream, as
// trailing HEADERS). Returns a stream-scoped error on state violations.
func (s *serverStream) recvHeaders(headers []hpack.HeaderField, endStream bool) error {
	s.mu.Lock()
	cur := s.state
	s.mu.Unlock()

	switch cur {
	case streamIdle:
		s.reqHeaders = headers
		if endStream {
			s.transition(streamHalfClosedRemote)
			// Close the write side of the body pipe immediately; there's no DATA.
			_ = s.reqBodyW.Close()
		} else {
			s.transition(streamOpen)
		}
		return nil
	case streamOpen:
		// Trailers — per SPEC §2.1 they are observed + discarded in phase 05.1.
		if endStream {
			s.transition(streamHalfClosedRemote)
			_ = s.reqBodyW.Close()
		}
		return nil
	default:
		return streamError(ErrStreamClosed, s.id, fmt.Sprintf("HEADERS in state %v", cur))
	}
}

// recvData processes an incoming DATA frame. Valid from open or
// halfClosedLocal. Writes data into the body pipe; on endStream closes the
// pipe and advances state.
func (s *serverStream) recvData(b []byte, endStream bool) error {
	s.mu.Lock()
	cur := s.state
	s.mu.Unlock()

	switch cur {
	case streamOpen:
		if _, err := s.reqBodyW.Write(b); err != nil {
			return streamError(ErrStreamClosed, s.id, "body write error: "+err.Error())
		}
		if endStream {
			_ = s.reqBodyW.Close()
			s.transition(streamHalfClosedRemote)
		}
		return nil
	case streamHalfClosedLocal:
		if _, err := s.reqBodyW.Write(b); err != nil {
			return streamError(ErrStreamClosed, s.id, "body write error: "+err.Error())
		}
		if endStream {
			_ = s.reqBodyW.Close()
			s.transition(streamClosed)
		}
		return nil
	case streamHalfClosedRemote, streamClosed:
		return streamError(ErrStreamClosed, s.id, "DATA on half-closed/closed stream")
	default:
		return streamError(ErrStreamClosed, s.id, fmt.Sprintf("DATA in state %v", cur))
	}
}

// recvRSTStream handles a peer RST_STREAM. Closes the body pipe with an error
// so the dispatch goroutine (if any) unblocks with an error. Transitions to
// closed.
func (s *serverStream) recvRSTStream(code ErrCode) error {
	s.mu.Lock()
	s.state = streamClosed
	s.mu.Unlock()

	_ = s.reqBodyW.CloseWithError(&Error{
		Code:   code,
		Stream: s.id,
		Msg:    "peer RST_STREAM",
	})
	return nil
}

// recvWindowUpdate replenishes the stream's send-side flow-control window.
// RFC 9113 §6.9: delta == 0 is a PROTOCOL_ERROR.
func (s *serverStream) recvWindowUpdate(delta int32) error {
	if delta == 0 {
		return streamError(ErrProtocolError, s.id, "WINDOW_UPDATE with delta 0")
	}
	s.sendW.replenish(delta)
	return nil
}

// WriteHeaders writes an HEADERS frame to the peer via the parent conn.
// Called from the dispatch goroutine (via a DirectResponseDispatcher).
func (s *serverStream) WriteHeaders(headers []hpack.HeaderField, endStream bool) error {
	if err := s.conn.encodeAndWriteHeaders(s.id, headers, endStream); err != nil {
		return err
	}
	if endStream {
		s.transition(streamClosed)
	}
	return nil
}

// WriteData writes a DATA frame to the peer via the parent conn.
// Called from the dispatch goroutine.
func (s *serverStream) WriteData(b []byte, endStream bool) error {
	if err := s.conn.writeData(s.id, b, endStream); err != nil {
		return err
	}
	if endStream {
		s.transition(streamClosed)
	}
	return nil
}

// dispatch runs the route-table lookup and action invocation for this stream.
// It should be called in a dedicated goroutine after END_STREAM has been
// received (either on HEADERS or DATA), per SPEC §10 #1
// (wait-for-END_STREAM before dispatching).
//
// lookup receives the built *http.Request and returns an hcmAction. dispatch
// distinguishes three cases:
//  1. DirectResponseDispatcher: invoke WriteH2(s).
//  2. Non-nil, non-DirectResponseDispatcher: emit RST_STREAM(INTERNAL_ERROR).
//  3. nil: synthesise a 404 DirectResponseDispatcher and invoke it.
func (s *serverStream) dispatch(ctx context.Context, lookup func(interface{}) hcmAction) {
	req, err := buildRequest(s.reqHeaders, s.reqBodyR)
	if err != nil {
		_ = s.conn.writeRSTStream(s.id, ErrProtocolError)
		s.transition(streamClosed)
		return
	}

	action := lookup(req)

	switch a := action.(type) {
	case DirectResponseDispatcher:
		if writeErr := a.WriteH2(s); writeErr != nil {
			_ = s.conn.writeRSTStream(s.id, ErrInternalError)
		}
		s.transition(streamClosed)
	case nil:
		notFoundAction := notFound404{}
		if writeErr := notFoundAction.WriteH2(s); writeErr != nil {
			_ = s.conn.writeRSTStream(s.id, ErrInternalError)
		}
		s.transition(streamClosed)
	default:
		// Non-nil but not a DirectResponseDispatcher (e.g. routerAction):
		// per SPEC §5.2 step 4c emit RST_STREAM(INTERNAL_ERROR).
		_ = s.conn.writeRSTStream(s.id, ErrInternalError)
		s.transition(streamClosed)
	}
}

// notFound404 is an unexported DirectResponseDispatcher that returns a
// synthetic 404 Not Found response.
type notFound404 struct{}

func (notFound404) WriteH2(sw StreamWriter) error {
	body := []byte("404 Not Found")
	headers := []hpack.HeaderField{
		{Name: ":status", Value: "404"},
		{Name: "content-type", Value: "text/plain"},
		{Name: "content-length", Value: strconv.Itoa(len(body))},
	}
	if err := sw.WriteHeaders(headers, false); err != nil {
		return err
	}
	return sw.WriteData(body, true)
}

// buildRequest constructs an *http.Request from decoded pseudo-headers,
// regular headers, and the body pipe reader. Per SPEC §10 #3: reuse stdlib
// *http.Request so the route-table machinery stays single-shape.
func buildRequest(headers []hpack.HeaderField, body io.Reader) (*http.Request, error) {
	var method, path, scheme, authority string
	regular := http.Header{}
	for _, h := range headers {
		switch h.Name {
		case ":method":
			method = h.Value
		case ":path":
			path = h.Value
		case ":scheme":
			scheme = h.Value
		case ":authority":
			authority = h.Value
		default:
			if len(h.Name) > 0 && h.Name[0] == ':' {
				return nil, &Error{Code: ErrProtocolError, Msg: "unknown pseudo-header: " + h.Name}
			}
			regular.Add(h.Name, h.Value)
		}
	}
	if method == "" || path == "" {
		return nil, &Error{Code: ErrProtocolError, Msg: "missing :method or :path"}
	}
	u, err := url.Parse(path)
	if err != nil {
		return nil, &Error{Code: ErrProtocolError, Msg: "bad :path", Underlying: err}
	}
	u.Scheme = scheme
	u.Host = authority
	req := &http.Request{
		Method:     method,
		URL:        u,
		Host:       authority,
		Proto:      "HTTP/2.0",
		ProtoMajor: 2,
		ProtoMinor: 0,
		Header:     regular,
		Body:       io.NopCloser(body),
		RequestURI: path,
	}
	return req, nil
}

// validateClientStreamID validates a client-initiated stream ID against the
// H2 rules:
//   - Even-numbered IDs are reserved for server-initiated streams; a client
//     sending an even ID is a PROTOCOL_ERROR (RFC 9113 §5.1.1).
//   - Reusing an existing stream ID is a PROTOCOL_ERROR.
//
// existing is the set of stream IDs already known to the ServerConn.
func validateClientStreamID(id uint32, existing map[uint32]struct{}) error {
	if id%2 == 0 {
		return connError(ErrProtocolError, fmt.Sprintf("even client stream id %d", id))
	}
	if _, seen := existing[id]; seen {
		return connError(ErrProtocolError, fmt.Sprintf("stream id %d reuse", id))
	}
	return nil
}
