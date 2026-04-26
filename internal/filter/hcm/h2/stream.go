package h2

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// StreamWriter is the interface an Action calls to emit response frames back
// to the peer. The concrete implementation is *serverStream itself (accessible
// to the action via the Action interface seam in stream.go).
type StreamWriter interface {
	WriteHeaders(headers []hpack.HeaderField, endStream bool) error
	WriteData(b []byte, endStream bool) error
}

// Action is the interface that any route action satisfies in order to be
// invoked on an H2 stream. The hcm parent package provides implementations
// via h2dispatch.go; tests provide fakes.
type Action interface {
	WriteH2(sw StreamWriter) error
}

// Dispatcher is the interface ServerConn uses to resolve an incoming request
// to an Action. The hcm parent package provides h2Dispatcher; tests provide
// fakes. Match returns (action, true) on a successful lookup (even if the
// action is a 404-synthesizing adapter — that is still ok=true). ok=false
// signals a genuine match-engine failure → INTERNAL_ERROR RST_STREAM.
type Dispatcher interface {
	Match(req *http.Request) (Action, bool)
}

// streamConn is the minimum surface serverStream needs from ServerConn.
type streamConn interface {
	encodeAndWriteHeaders(streamID uint32, headers []hpack.HeaderField, endStream bool) error
	writeData(streamID uint32, b []byte, endStream bool) error
	writeRSTStream(streamID uint32, code ErrCode) error
}

// serverStream is one HTTP/2 server-side stream. recvX methods are called
// from the ServerConn's frame loop. dispatch runs in its own goroutine.
// The mu mutex guards state and body accumulation.
type serverStream struct {
	id    uint32
	mu    sync.Mutex
	state streamState

	sendW *window
	recvW *window

	reqHeaders []hpack.HeaderField
	reqBody    bytes.Buffer // accumulates body DATA frames; fully populated before dispatch starts

	conn streamConn
}

// newServerStream constructs a serverStream in the idle state.
func newServerStream(id uint32, conn streamConn, initialSendWindow, initialRecvWindow int32) *serverStream {
	return &serverStream{
		id:    id,
		state: streamIdle,
		sendW: newWindow(initialSendWindow),
		recvW: newWindow(initialRecvWindow),
		conn:  conn,
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
		} else {
			s.transition(streamOpen)
		}
		return nil
	case streamOpen:
		// Trailers — per SPEC §2.1 they are observed + discarded in phase 05.1.
		if endStream {
			s.transition(streamHalfClosedRemote)
		}
		return nil
	default:
		return streamError(ErrStreamClosed, s.id, fmt.Sprintf("HEADERS in state %v", cur))
	}
}

// recvTrailingHeaders handles a HEADERS frame on an existing stream.
// In phase 05.1, trailing HEADERS (trailers) are discarded per SPEC §2.1.
// HEADERS on a half-closed-remote or closed stream is a stream error.
// Trailing HEADERS MUST NOT contain pseudo-headers (RFC 9113 §8.1.2.1).
func (s *serverStream) recvTrailingHeaders(headers []hpack.HeaderField, endStream bool) error {
	s.mu.Lock()
	cur := s.state
	s.mu.Unlock()

	switch cur {
	case streamOpen:
		// RFC 9113 §8.1: a second HEADERS frame without END_STREAM is only
		// valid as trailers, which MUST have END_STREAM set. A second HEADERS
		// without END_STREAM is a stream error PROTOCOL_ERROR.
		if !endStream {
			return streamError(ErrProtocolError, s.id, "second HEADERS without END_STREAM")
		}
		// Validate: trailers must not contain pseudo-headers.
		for _, h := range headers {
			if len(h.Name) > 0 && h.Name[0] == ':' {
				return streamError(ErrProtocolError, s.id, "pseudo-header in trailers: "+h.Name)
			}
		}
		// Trailing HEADERS with END_STREAM: advance state.
		s.transition(streamHalfClosedRemote)
		return nil
	case streamHalfClosedRemote, streamClosed:
		// RFC 9113 §5.1: HEADERS on half-closed-remote or closed is STREAM_CLOSED.
		return streamError(ErrStreamClosed, s.id, fmt.Sprintf("HEADERS in state %v", cur))
	default:
		return streamError(ErrStreamClosed, s.id, fmt.Sprintf("trailing HEADERS in unexpected state %v", cur))
	}
}

// recvData processes an incoming DATA frame. Valid from open or
// halfClosedLocal. Appends data to the body buffer; on endStream advances state.
// The body buffer is fully populated before the dispatch goroutine starts.
func (s *serverStream) recvData(b []byte, endStream bool) error {
	s.mu.Lock()
	cur := s.state
	if len(b) > 0 {
		_, _ = s.reqBody.Write(b) // bytes.Buffer.Write never fails
	}
	s.mu.Unlock()

	switch cur {
	case streamOpen:
		if endStream {
			s.transition(streamHalfClosedRemote)
		}
		return nil
	case streamHalfClosedLocal:
		if endStream {
			s.transition(streamClosed)
		}
		return nil
	case streamHalfClosedRemote, streamClosed:
		return streamError(ErrStreamClosed, s.id, "DATA on half-closed/closed stream")
	default:
		return streamError(ErrStreamClosed, s.id, fmt.Sprintf("DATA in state %v", cur))
	}
}

// recvRSTStream handles a peer RST_STREAM. Transitions the stream to closed.
func (s *serverStream) recvRSTStream(code ErrCode) error {
	s.mu.Lock()
	s.state = streamClosed
	s.mu.Unlock()
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
// Called from the dispatch goroutine (via an Action).
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
// The dispatch contract is uniform: always call action.WriteH2(s), then handle
// any stream-scoped error. The Dispatcher adapter (h2dispatch.go in hcm
// package) is responsible for synthesizing 404 adapters on no-match and
// INTERNAL_ERROR sentinels on router-action-on-H2 (SPEC §5.2 step 4c).
func (s *serverStream) dispatch(ctx context.Context, dispatcher Dispatcher) {
	// Snapshot the body buffer (dispatch happens after END_STREAM so the
	// buffer is fully populated).
	s.mu.Lock()
	bodyBytes := s.reqBody.Bytes()
	bodySnap := make([]byte, len(bodyBytes))
	copy(bodySnap, bodyBytes)
	s.mu.Unlock()

	// RFC 9113 §8.1.2.6: if content-length is present, it MUST equal the
	// actual body length. Mismatch → stream error PROTOCOL_ERROR.
	for _, cl := range s.reqHeaders {
		if strings.EqualFold(cl.Name, "content-length") {
			declared, parseErr := strconv.ParseInt(cl.Value, 10, 64)
			if parseErr != nil || declared < 0 || int(declared) != len(bodySnap) {
				_ = s.conn.writeRSTStream(s.id, ErrProtocolError)
				s.transition(streamClosed)
				return
			}
			break
		}
	}

	req, err := buildRequest(s.reqHeaders, bytes.NewReader(bodySnap))
	if err != nil {
		_ = s.conn.writeRSTStream(s.id, ErrProtocolError)
		s.transition(streamClosed)
		return
	}

	action, ok := dispatcher.Match(req)
	if !ok {
		// ok=false is a genuine match-engine failure → INTERNAL_ERROR.
		_ = s.conn.writeRSTStream(s.id, ErrInternalError)
		s.transition(streamClosed)
		return
	}

	if writeErr := action.WriteH2(s); writeErr != nil {
		// Stream-scoped errors → RST_STREAM with the carried code.
		code := ErrInternalError
		if hErr, isH2 := writeErr.(*Error); isH2 {
			code = hErr.Code
		}
		_ = s.conn.writeRSTStream(s.id, code)
	}
	s.transition(streamClosed)
}

// buildRequest constructs an *http.Request from decoded pseudo-headers,
// regular headers, and the body pipe reader. Per SPEC §10 #3: reuse stdlib
// *http.Request so the route-table machinery stays single-shape.
// Performs RFC 9113 §8.3 header field validation:
//   - pseudo-headers must appear before regular headers
//   - only request pseudo-headers (:method, :path, :scheme, :authority) are allowed
//   - :method, :scheme, :path are required; each must not be duplicated
//   - uppercase header field names are a PROTOCOL_ERROR (RFC 9113 §8.2)
//   - connection-specific headers (connection, keep-alive, etc.) are rejected
//   - TE header value must be "trailers" if present
func buildRequest(headers []hpack.HeaderField, body io.Reader) (*http.Request, error) {
	var method, path, scheme, authority string
	var methodSeen, pathSeen, schemeSeen bool
	seenRegular := false
	regular := http.Header{}

	for _, h := range headers {
		name := h.Name

		// Pseudo-headers start with ':'
		if len(name) > 0 && name[0] == ':' {
			// RFC 9113 §8.3: pseudo-headers MUST NOT appear after regular headers.
			if seenRegular {
				return nil, &Error{Code: ErrProtocolError, Msg: "pseudo-header after regular header"}
			}
			switch name {
			case ":method":
				if methodSeen {
					return nil, &Error{Code: ErrProtocolError, Msg: "duplicate :method"}
				}
				method = h.Value
				methodSeen = true
			case ":path":
				if pathSeen {
					return nil, &Error{Code: ErrProtocolError, Msg: "duplicate :path"}
				}
				if h.Value == "" {
					return nil, &Error{Code: ErrProtocolError, Msg: "empty :path"}
				}
				path = h.Value
				pathSeen = true
			case ":scheme":
				if schemeSeen {
					return nil, &Error{Code: ErrProtocolError, Msg: "duplicate :scheme"}
				}
				scheme = h.Value
				schemeSeen = true
			case ":authority":
				authority = h.Value
			default:
				// Unknown or response pseudo-header in request → PROTOCOL_ERROR.
				return nil, &Error{Code: ErrProtocolError, Msg: "unknown/invalid pseudo-header: " + name}
			}
			continue
		}

		seenRegular = true

		// RFC 9113 §8.2: uppercase characters in header field names are PROTOCOL_ERROR.
		for _, c := range name {
			if c >= 'A' && c <= 'Z' {
				return nil, &Error{Code: ErrProtocolError, Msg: "uppercase character in header name: " + name}
			}
		}

		// RFC 9113 §8.2.2: connection-specific header fields are PROTOCOL_ERROR.
		switch name {
		case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade":
			return nil, &Error{Code: ErrProtocolError, Msg: "connection-specific header field: " + name}
		case "te":
			if h.Value != "trailers" {
				return nil, &Error{Code: ErrProtocolError, Msg: "TE header field value not 'trailers'"}
			}
		}

		regular.Add(name, h.Value)
	}

	if method == "" {
		return nil, &Error{Code: ErrProtocolError, Msg: "missing :method"}
	}
	if scheme == "" {
		return nil, &Error{Code: ErrProtocolError, Msg: "missing :scheme"}
	}
	if path == "" {
		return nil, &Error{Code: ErrProtocolError, Msg: "missing :path"}
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
