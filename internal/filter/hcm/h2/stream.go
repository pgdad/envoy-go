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
//
// Phase 05.2: the interface is widened from the 05.1 single-arg shape to
// (ctx, req, sw) so routerActionH2 can consume the upstream-bound request.
// directResponseAction's adapter ignores ctx + req (synthesizes the reply
// from the action's own state); routerActionH2's adapter passes both
// through to its do method which forwards them to the upstream RoundTrip.
type Action interface {
	WriteH2(ctx context.Context, req H2Request, sw StreamWriter) error
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

	// recvDebitSinceLastUpdate accumulates inbound DATA bytes consumed against
	// this stream's recv window since the last per-stream WINDOW_UPDATE was
	// emitted. Mutated only from the frame-loop goroutine (in ServerConn.onData,
	// before dispatch is launched). No separate mutex needed.
	recvDebitSinceLastUpdate int32

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
// halfClosedLocal. Validates state FIRST per ADR-0055 M-11 (do not grow
// s.reqBody on a closed/half-closed-remote stream); then appends the body
// bytes; on endStream advances state. The body buffer is fully populated
// before the dispatch goroutine starts.
func (s *serverStream) recvData(b []byte, endStream bool) error {
	s.mu.Lock()
	cur := s.state
	// State validity check BEFORE appending (ADR-0055 M-11): a peer sending
	// DATA on a closed or half-closed-remote stream is a stream error and
	// MUST NOT cause server-side memory growth.
	switch cur {
	case streamOpen, streamHalfClosedLocal:
		// valid; fall through to append
	case streamHalfClosedRemote, streamClosed:
		s.mu.Unlock()
		return streamError(ErrStreamClosed, s.id, "DATA on half-closed/closed stream")
	default:
		s.mu.Unlock()
		return streamError(ErrStreamClosed, s.id, fmt.Sprintf("DATA in state %v", cur))
	}
	if len(b) > 0 {
		_, _ = s.reqBody.Write(b) // bytes.Buffer.Write never fails
	}
	s.mu.Unlock()

	switch cur {
	case streamOpen:
		if endStream {
			s.transition(streamHalfClosedRemote)
		}
	case streamHalfClosedLocal:
		if endStream {
			s.transition(streamClosed)
		}
	}
	return nil
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
// RFC 9113 §6.9.1: a WINDOW_UPDATE that would push the window past 2^31-1
// is a stream error of type FLOW_CONTROL_ERROR.
func (s *serverStream) recvWindowUpdate(delta int32) error {
	if delta == 0 {
		return streamError(ErrProtocolError, s.id, "WINDOW_UPDATE with delta 0")
	}
	cur := s.sendW.available()
	if _, ok := safeAddInt32(cur, delta); !ok {
		return streamError(ErrFlowControlError, s.id, "WINDOW_UPDATE stream-level overflow")
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

	// Build the codec-internal H2Request from the decoded request headers + body
	// snapshot. Pseudo-headers are split out; regular headers are kept on
	// .Headers (preserving wire order). The router (routerActionH2) re-prepends
	// pseudo-headers on the upstream encode side per RFC 9113 §8.3.
	h2Req := buildH2Request(s.reqHeaders, bodySnap)

	if writeErr := action.WriteH2(ctx, h2Req, s); writeErr != nil {
		// Stream-scoped errors → RST_STREAM with the carried code.
		code := ErrInternalError
		if hErr, isH2 := writeErr.(*Error); isH2 {
			code = hErr.Code
		}
		_ = s.conn.writeRSTStream(s.id, code)
	}
	s.transition(streamClosed)
}

// buildH2Request constructs the codec-internal H2Request shape from the
// decoded inbound HEADERS pseudo-headers + regular headers + the buffered
// request body. Pseudo-headers are split out into the named fields
// (Method/Path/Scheme/Authority); regular headers are kept on .Headers
// in wire order. The router (routerActionH2) re-prepends the pseudo-headers
// on the upstream encode side per RFC 9113 §8.3.
//
// This is invoked from serverStream.dispatch AFTER buildRequest has already
// validated the pseudo-header set (so duplicate/missing/malformed cases have
// already returned PROTOCOL_ERROR). The function itself is best-effort: it
// pulls whatever is present and ignores duplicates (the validation happens
// up-stack in buildRequest).
func buildH2Request(headers []hpack.HeaderField, body []byte) H2Request {
	out := H2Request{Body: body}
	// D-90-SCOPE. authoritySeen tracks PRESENCE ONLY, because a bare
	// out.Authority != "" cannot tell an ABSENT :authority from a
	// PRESENT-AND-EMPTY one, and only the former promotes. hostSeen plays the
	// same role for the regular host: it latches the FIRST occurrence as the
	// promotion source even when that first value is itself empty.
	var hostField string
	var hostSeen, authoritySeen bool
	for _, h := range headers {
		switch h.Name {
		case ":method":
			out.Method = h.Value
		case ":path":
			out.Path = h.Value
		case ":scheme":
			out.Scheme = h.Value
		case ":authority":
			out.Authority = h.Value
			authoritySeen = true
		case "host":
			// The regular host never reaches the upstream carrier: EVERY
			// occurrence is suppressed here, and the FIRST is latched.
			// Field names are lowercase-only on this leg — buildRequest has
			// already rejected an uppercase name with PROTOCOL_ERROR before
			// this function runs — so the test is an exact match, not a fold.
			if !hostSeen {
				hostField, hostSeen = h.Value, true
			}
		default:
			if len(h.Name) > 0 && h.Name[0] != ':' {
				out.Headers = append(out.Headers, h)
			}
		}
	}
	// Promote on ABSENT, never on EMPTY: a present-and-empty :authority wins
	// and stays empty.
	if !authoritySeen && hostSeen {
		out.Authority = hostField
	}
	return out
}

// teTrailersValue is the ONLY value RFC 9113 §8.2.2 permits for the `te`
// header field on an HTTP/2 message. Any other value is a violation on
// either direction of the wire.
const teTrailersValue = "trailers"

// isConnectionSpecificField reports whether name is one of the RFC 9113
// §8.2.2 connection-specific header fields, which MUST NOT appear in an
// HTTP/2 message.
//
// `te` is deliberately NOT in this set: it is CONDITIONALLY legal (permitted
// with the single value teTrailersValue), so it cannot be answered by a
// name-only predicate. Callers must test `te`'s VALUE separately — see
// buildRequest below and validateResponseTrailers in client.go.
//
// This is the ONE source of truth for the RFC list. It is shared by the
// request-decode path (buildRequest, server side) and the response-trailer
// validation path (validateResponseTrailers, client side) so a member can
// never be dropped from one copy and survive in the other.
func isConnectionSpecificField(name string) bool {
	switch name {
	case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

// IsIllegalH2RequestHeader reports whether a (name, value) pair may NOT appear
// on an HTTP/2 request message per RFC 9113 §8.2.2. name MUST already be
// lowercase (H/2 wire form).
//
// Exported so the OUTBOUND path — package hcm's decode-filter reconciler,
// which re-emits filter-written header names onto the upstream HEADERS block
// — can consult the SAME list buildRequest enforces on the inbound path,
// without duplicating it.
//
// Phase 89 / ADR-0311. This is an EXPORT of the existing predicate pair, not a
// second copy: isConnectionSpecificField above is documented as "the ONE source
// of truth for the RFC list ... so a member can never be dropped from one copy
// and survive in the other", and a duplicate would break that invariant.
func IsIllegalH2RequestHeader(name, value string) bool {
	if isConnectionSpecificField(name) {
		return true
	}
	return name == "te" && value != teTrailersValue
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
	// D-90-SCOPE. authoritySeen records PRESENCE ONLY — deliberately NOT a
	// duplicate guard like its three siblings above: duplicate :authority
	// keeps its existing silent last-wins behavior, and the reject belongs
	// to the deferred authority-validation row (D-90-DUP).
	var hostField string
	var hostSeen, authoritySeen bool
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
				authoritySeen = true
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
		if isConnectionSpecificField(name) {
			return nil, &Error{Code: ErrProtocolError, Msg: "connection-specific header field: " + name}
		}
		if name == "te" && h.Value != teTrailersValue {
			return nil, &Error{Code: ErrProtocolError, Msg: "TE header field value not 'trailers'"}
		}

		// D-90-SCOPE: the regular host is suppressed from the decode map in
		// every shape. It must be dropped BEFORE the Add — http.Header.Add
		// routes through textproto and canonicalizes the key to "Host", so a
		// later delete of "host" would miss it. The FIRST occurrence is
		// latched as the promotion source; hostSeen (not hostField != "")
		// carries that latch so an empty first value still wins.
		if name == "host" {
			if !hostSeen {
				hostField, hostSeen = h.Value, true
			}
			continue
		}

		regular.Add(name, h.Value)
	}

	// Promote on ABSENT, never on EMPTY — resolved here so both u.Host and
	// http.Request.Host below carry the effective authority.
	if !authoritySeen && hostSeen {
		authority = hostField
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

	// RFC 9110 §7.1: an origin-form request-target carries no fragment — the
	// component is never sent on the wire — and ParseRequestURI would keep a
	// '#' as ordinary path bytes rather than splitting it off. Reject it
	// explicitly (ADR-0309).
	if strings.IndexByte(path, '#') >= 0 {
		return nil, &Error{Code: ErrProtocolError, Msg: "fragment in :path"}
	}
	// ParseRequestURI, NOT url.Parse: RFC 9113 §8.3.1 makes :path a
	// request-target, but url.Parse accepts the whole RFC 3986 §4.2
	// URI-reference grammar, in which a leading "//" opens a network-path
	// reference and the next segment is parsed as an AUTHORITY. That peeled
	// "//foo" down to an empty path and silently mis-routed "//foo/bar" to
	// "/bar" (ADR-0309).
	u, err := url.ParseRequestURI(path)
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
