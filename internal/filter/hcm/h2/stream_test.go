package h2

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2/hpack"
)

// fakeConn records streamConn calls for inspection in tests.
type fakeConn struct {
	headers []struct {
		id  uint32
		h   []hpack.HeaderField
		end bool
	}
	data []struct {
		id  uint32
		b   []byte
		end bool
	}
	rsts []struct {
		id   uint32
		code ErrCode
	}
}

func (f *fakeConn) encodeAndWriteHeaders(id uint32, h []hpack.HeaderField, end bool) error {
	f.headers = append(f.headers, struct {
		id  uint32
		h   []hpack.HeaderField
		end bool
	}{id, h, end})
	return nil
}

func (f *fakeConn) writeData(id uint32, b []byte, end bool) error {
	cp := make([]byte, len(b))
	copy(cp, b)
	f.data = append(f.data, struct {
		id  uint32
		b   []byte
		end bool
	}{id, cp, end})
	return nil
}

func (f *fakeConn) writeRSTStream(id uint32, code ErrCode) error {
	f.rsts = append(f.rsts, struct {
		id   uint32
		code ErrCode
	}{id, code})
	return nil
}

// minHeaders returns a minimal set of pseudo-headers for a GET /.
func minHeaders() []hpack.HeaderField {
	return []hpack.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":path", Value: "/"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.com"},
	}
}

// fakeAction is an Action that writes a fixed 200 response.
type fakeAction struct {
	statusCode string // e.g. "200"
	body       string
}

func (fa *fakeAction) WriteH2(_ context.Context, _ H2Request, sw StreamWriter) error {
	respHeaders := []hpack.HeaderField{
		{Name: ":status", Value: fa.statusCode},
		{Name: "content-length", Value: "5"},
	}
	if err := sw.WriteHeaders(respHeaders, false); err != nil {
		return err
	}
	return sw.WriteData([]byte(fa.body), true)
}

// fakeDispatcher wraps a function as a Dispatcher.
type fakeDispatcher struct {
	fn func(req *http.Request) (Action, bool)
}

func (d *fakeDispatcher) Match(req *http.Request) (Action, bool) {
	return d.fn(req)
}

// dispatchWith is a helper that creates a fakeDispatcher from an Action-returning function.
func dispatchWith(fn func() Action) Dispatcher {
	return &fakeDispatcher{fn: func(_ *http.Request) (Action, bool) { return fn(), true }}
}

// dispatchWithReq creates a fakeDispatcher that passes the request to the function.
func dispatchWithReq(fn func(req *http.Request) Action) Dispatcher {
	return &fakeDispatcher{fn: func(req *http.Request) (Action, bool) { return fn(req), true }}
}

// ---- Tests ----

// TestServerStream_StateTransitions_HeadersOnlyEndStream:
// HEADERS with END_STREAM=true → idle→halfClosedRemote; dispatch writes response → closed.
func TestServerStream_StateTransitions_HeadersOnlyEndStream(t *testing.T) {
	fc := &fakeConn{}
	s := newServerStream(1, fc, 65535, 65535)

	if err := s.recvHeaders(minHeaders(), true); err != nil {
		t.Fatalf("recvHeaders: %v", err)
	}
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()
	if state != streamHalfClosedRemote {
		t.Errorf("state after HEADERS+END_STREAM = %v, want halfClosedRemote", state)
	}

	// dispatch with a direct response that writes 200 + body.
	fa := &fakeAction{statusCode: "200", body: "hello"}
	ctx := context.Background()
	s.dispatch(ctx, dispatchWith(func() Action { return fa }))

	// After dispatch, stream should be closed.
	s.mu.Lock()
	state = s.state
	s.mu.Unlock()
	if state != streamClosed {
		t.Errorf("state after dispatch = %v, want closed", state)
	}

	// fakeConn should have received HEADERS + DATA.
	if len(fc.headers) != 1 {
		t.Fatalf("encodeAndWriteHeaders called %d times, want 1", len(fc.headers))
	}
	if len(fc.data) != 1 {
		t.Fatalf("writeData called %d times, want 1", len(fc.data))
	}
}

// TestServerStream_StateTransitions_HeadersThenData:
// HEADERS (no end) → open; DATA chunk1 (no end) → open; DATA chunk2 (end) → halfClosedRemote.
func TestServerStream_StateTransitions_HeadersThenData(t *testing.T) {
	fc := &fakeConn{}
	s := newServerStream(1, fc, 65535, 65535)

	if err := s.recvHeaders(minHeaders(), false); err != nil {
		t.Fatalf("recvHeaders: %v", err)
	}
	s.mu.Lock()
	if s.state != streamOpen {
		t.Fatalf("state after HEADERS = %v, want open", s.state)
	}
	s.mu.Unlock()

	// Run dispatch concurrently so the pipe reader unblocks the writes.
	dispatchDone := make(chan string, 1)
	ctx := context.Background()
	go func() {
		var capturedBody string
		s.dispatch(ctx, dispatchWithReq(func(req *http.Request) Action {
			// req is *http.Request; read its Body to capture the request data.
			if req.Body != nil {
				b, _ := io.ReadAll(req.Body)
				capturedBody = string(b)
			}
			return &fakeAction{statusCode: "200", body: "ok"}
		}))
		dispatchDone <- capturedBody
	}()

	if err := s.recvData([]byte("chunk1"), false); err != nil {
		t.Fatalf("recvData chunk1: %v", err)
	}
	s.mu.Lock()
	if s.state != streamOpen {
		t.Fatalf("state after DATA (no end) = %v, want open", s.state)
	}
	s.mu.Unlock()

	if err := s.recvData([]byte("chunk2"), true); err != nil {
		t.Fatalf("recvData chunk2+end: %v", err)
	}
	s.mu.Lock()
	if s.state != streamHalfClosedRemote && s.state != streamClosed {
		t.Fatalf("state after DATA+END = %v, want halfClosedRemote or closed", s.state)
	}
	s.mu.Unlock()

	// Wait for dispatch to finish.
	select {
	case body := <-dispatchDone:
		if body != "chunk1chunk2" {
			t.Errorf("captured body = %q, want %q", body, "chunk1chunk2")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("dispatch did not complete")
	}
}

// TestServerStream_StateTransitions_RSTStream:
// After recvHeaders, recvRSTStream(CANCEL) → closed.
func TestServerStream_StateTransitions_RSTStream(t *testing.T) {
	fc := &fakeConn{}
	s := newServerStream(1, fc, 65535, 65535)

	if err := s.recvHeaders(minHeaders(), false); err != nil {
		t.Fatalf("recvHeaders: %v", err)
	}

	if err := s.recvRSTStream(ErrCancel); err != nil {
		t.Fatalf("recvRSTStream: %v", err)
	}
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()
	if state != streamClosed {
		t.Errorf("state after RST = %v, want closed", state)
	}
}

// TestServerStream_RecvWindowUpdate_ReplenishesSendWindow:
// sendW starts at N; recvWindowUpdate(K) → sendW.available() == N+K.
func TestServerStream_RecvWindowUpdate_ReplenishesSendWindow(t *testing.T) {
	fc := &fakeConn{}
	s := newServerStream(1, fc, 1000, 65535)

	initialAvail := s.sendW.available()
	if initialAvail != 1000 {
		t.Fatalf("initial sendW = %d, want 1000", initialAvail)
	}

	if err := s.recvWindowUpdate(500); err != nil {
		t.Fatalf("recvWindowUpdate: %v", err)
	}

	if got := s.sendW.available(); got != 1500 {
		t.Errorf("sendW after replenish = %d, want 1500", got)
	}
}

// TestServerStream_RecvWindowUpdate_ZeroDeltaIsProtocolError:
// RFC 9113 §6.9: delta == 0 → PROTOCOL_ERROR.
func TestServerStream_RecvWindowUpdate_ZeroDeltaIsProtocolError(t *testing.T) {
	fc := &fakeConn{}
	s := newServerStream(1, fc, 65535, 65535)

	err := s.recvWindowUpdate(0)
	if err == nil {
		t.Fatal("recvWindowUpdate(0) returned nil, want PROTOCOL_ERROR")
	}
	h2err, ok := err.(*Error)
	if !ok {
		t.Fatalf("error is %T, want *Error", err)
	}
	if h2err.Code != ErrProtocolError {
		t.Errorf("error code = %v, want PROTOCOL_ERROR", h2err.Code)
	}
}

// TestServerStream_RecvWindowUpdate_OverflowIsFlowControlError:
// ADR-0055 M-9 / RFC 9113 §6.9.1: a WINDOW_UPDATE that would push the send
// window past 2^31-1 is a stream-scoped FLOW_CONTROL_ERROR.
func TestServerStream_RecvWindowUpdate_OverflowIsFlowControlError(t *testing.T) {
	fc := &fakeConn{}
	// Start the send window very close to MaxInt32 so a small delta overflows.
	s := newServerStream(7, fc, math.MaxInt32-1, 65535)

	// delta=2 would push to MaxInt32+1 → overflow.
	err := s.recvWindowUpdate(2)
	if err == nil {
		t.Fatal("recvWindowUpdate(overflow delta) returned nil, want FLOW_CONTROL_ERROR")
	}
	h2err, ok := err.(*Error)
	if !ok {
		t.Fatalf("error is %T, want *Error", err)
	}
	if h2err.Code != ErrFlowControlError {
		t.Errorf("error code = %v, want FLOW_CONTROL_ERROR", h2err.Code)
	}
	if h2err.Stream != 7 {
		t.Errorf("error stream id = %d, want 7 (stream-scoped)", h2err.Stream)
	}
}

// TestSafeAddInt32 covers the ADR-0055 M-9 helper.
func TestSafeAddInt32(t *testing.T) {
	cases := []struct {
		a, b   int32
		wantOK bool
		want   int32
	}{
		{0, 0, true, 0},
		{1, 2, true, 3},
		{math.MaxInt32, 0, true, math.MaxInt32},
		{math.MaxInt32 - 1, 1, true, math.MaxInt32},
		{math.MaxInt32, 1, false, 0},
		{math.MaxInt32, math.MaxInt32, false, 0},
		{math.MinInt32, -1, false, 0},
		{math.MinInt32, 0, true, math.MinInt32},
	}
	for _, c := range cases {
		got, ok := safeAddInt32(c.a, c.b)
		if ok != c.wantOK {
			t.Errorf("safeAddInt32(%d, %d) ok=%v, want %v", c.a, c.b, ok, c.wantOK)
			continue
		}
		if ok && got != c.want {
			t.Errorf("safeAddInt32(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestServerStream_RecvData_DoesNotGrowReqBodyOnClosedStream covers the
// ADR-0055 M-11 fix: recvData MUST validate the stream state BEFORE appending
// to s.reqBody. Without the reorder, a peer that sends DATA on a closed or
// half-closed-remote stream would still grow s.reqBody — wasted memory plus a
// surprise dispatch-time mismatch if the bytes were observed by a slow reader.
// After the reorder, the state check rejects the frame first and reqBody is
// untouched.
func TestServerStream_RecvData_DoesNotGrowReqBodyOnClosedStream(t *testing.T) {
	fc := &fakeConn{}
	s := newServerStream(1, fc, 65535, 65535)

	// Drive the stream to halfClosedRemote via the legitimate transition path
	// (HEADERS with END_STREAM). DATA frames are invalid in this state per
	// RFC 9113 §5.1.
	if err := s.recvHeaders(minHeaders(), true); err != nil {
		t.Fatalf("recvHeaders: %v", err)
	}
	s.mu.Lock()
	if s.state != streamHalfClosedRemote {
		s.mu.Unlock()
		t.Fatalf("precondition: state = %v, want halfClosedRemote", s.state)
	}
	preLen := s.reqBody.Len()
	s.mu.Unlock()

	err := s.recvData([]byte("late data"), false)
	if err == nil {
		t.Fatal("recvData on halfClosedRemote returned nil, want stream error")
	}
	h2err, ok := err.(*Error)
	if !ok {
		t.Fatalf("error is %T, want *Error", err)
	}
	if h2err.Code != ErrStreamClosed {
		t.Errorf("error code = %v, want STREAM_CLOSED", h2err.Code)
	}

	s.mu.Lock()
	postLen := s.reqBody.Len()
	s.mu.Unlock()
	if postLen != preLen {
		t.Errorf("reqBody.Len() grew from %d to %d on a closed stream; want unchanged", preLen, postLen)
	}
}

// TestServerStream_Dispatch_DirectResponse_WritesHeadersAndData:
// An Action is invoked; observe writes via fakeConn.
func TestServerStream_Dispatch_DirectResponse_WritesHeadersAndData(t *testing.T) {
	fc := &fakeConn{}
	s := newServerStream(3, fc, 65535, 65535)

	// Prime with headers + end_stream so state = halfClosedRemote.
	if err := s.recvHeaders(minHeaders(), true); err != nil {
		t.Fatalf("recvHeaders: %v", err)
	}

	fa := &fakeAction{statusCode: "200", body: "hello"}
	ctx := context.Background()
	s.dispatch(ctx, dispatchWith(func() Action { return fa }))

	if len(fc.headers) != 1 {
		t.Fatalf("headers written = %d, want 1", len(fc.headers))
	}
	if fc.headers[0].id != 3 {
		t.Errorf("headers stream id = %d, want 3", fc.headers[0].id)
	}

	if len(fc.data) != 1 {
		t.Fatalf("data frames written = %d, want 1", len(fc.data))
	}
	if !fc.data[0].end {
		t.Errorf("data end_stream = false, want true")
	}
	if string(fc.data[0].b) != "hello" {
		t.Errorf("data body = %q, want %q", fc.data[0].b, "hello")
	}
}

// TestServerStream_Dispatch_RouterAction_EmitsRSTStreamInternalError:
// An Action whose WriteH2 returns *Error{Code: ErrInternalError} → RST_STREAM(INTERNAL_ERROR).
// This models the h2RouterActionRejection adapter (SPEC §5.2 step 4c).
func TestServerStream_Dispatch_RouterAction_EmitsRSTStreamInternalError(t *testing.T) {
	fc := &fakeConn{}
	s := newServerStream(5, fc, 65535, 65535)

	if err := s.recvHeaders(minHeaders(), true); err != nil {
		t.Fatalf("recvHeaders: %v", err)
	}

	// Simulate the h2RouterActionRejection adapter: WriteH2 returns an INTERNAL_ERROR.
	rejectionAction := &errorAction{err: NewStreamError(ErrInternalError, 5, "router action on h2 listener (SPEC §5.2 step 4c)")}

	ctx := context.Background()
	s.dispatch(ctx, dispatchWith(func() Action { return rejectionAction }))

	if len(fc.rsts) != 1 {
		t.Fatalf("RST_STREAM calls = %d, want 1", len(fc.rsts))
	}
	if fc.rsts[0].id != 5 {
		t.Errorf("RST_STREAM stream id = %d, want 5", fc.rsts[0].id)
	}
	if fc.rsts[0].code != ErrInternalError {
		t.Errorf("RST_STREAM code = %v, want INTERNAL_ERROR", fc.rsts[0].code)
	}
}

// errorAction is a test Action whose WriteH2 always returns a fixed error.
type errorAction struct {
	err error
}

func (a *errorAction) WriteH2(_ context.Context, _ H2Request, _ StreamWriter) error {
	return a.err
}

// TestServerStream_Dispatch_404Adapter_WritesHeadersAndData:
// A 404-synthesizing Action writes HEADERS with :status 404 + DATA body.
// This models the h2DirectResponseAdapter wrapping a 404 directResponseAction.
func TestServerStream_Dispatch_404Adapter_WritesHeadersAndData(t *testing.T) {
	fc := &fakeConn{}
	s := newServerStream(7, fc, 65535, 65535)

	if err := s.recvHeaders(minHeaders(), true); err != nil {
		t.Fatalf("recvHeaders: %v", err)
	}

	// Simulate the 404-synthesizing adapter.
	notFoundAction := &fakeAction{statusCode: "404", body: "not found\n"}

	ctx := context.Background()
	s.dispatch(ctx, dispatchWith(func() Action { return notFoundAction }))

	// Should see HEADERS (with :status 404) + DATA.
	if len(fc.headers) != 1 {
		t.Fatalf("headers written = %d, want 1", len(fc.headers))
	}
	var statusVal string
	for _, hf := range fc.headers[0].h {
		if hf.Name == ":status" {
			statusVal = hf.Value
		}
	}
	if statusVal != "404" {
		t.Errorf(":status = %q, want %q", statusVal, "404")
	}
	if len(fc.data) != 1 {
		t.Fatalf("data frames written = %d, want 1", len(fc.data))
	}
	if !strings.Contains(string(fc.data[0].b), "not found") {
		t.Errorf("404 body = %q, does not contain 'not found'", fc.data[0].b)
	}
}

// TestServerStream_Dispatch_MalformedTrailersError_EmitsRSTStreamInternalError
// is the routed obligation from the phase 84.1 ledger ("downstream
// RST_STREAM(INTERNAL_ERROR) on malformed trailers is REASONED not
// MEASURED", Task 7). It drives dispatch with an Action whose WriteH2
// returns the exact *Error shape doH2ClusterAction's
// errors.Is(err, ErrMalformedTrailers) arm returns unwrapped
// (router_h2.go: `return ActionResponse{Status: 0}, picked, err`) — built
// here via the h2 package's exported *Error fields, since the
// malformedTrailersError constructor that builds it in production is
// unexported outside the package. errors.Is against the exported
// ErrMalformedTrailers sentinel is asserted first so the test fixture
// itself is proven faithful to the production shape before the dispatch
// assertions run.
//
// Asserts: the downstream stream sees RST_STREAM(INTERNAL_ERROR) and NO
// HEADERS/DATA frame — i.e. no 502 local reply. serverStream.dispatch never
// calls sw.WriteHeaders on the error path (see the dispatch source: the
// action.WriteH2 error short-circuits straight to writeRSTStream), so this
// also stands as the "not a 502 HEADERS frame" half of the obligation at
// the one seam that can observe the actual wire RST_STREAM call.
func TestServerStream_Dispatch_MalformedTrailersError_EmitsRSTStreamInternalError(t *testing.T) {
	fc := &fakeConn{}
	s := newServerStream(9, fc, 65535, 65535)

	if err := s.recvHeaders(minHeaders(), true); err != nil {
		t.Fatalf("recvHeaders: %v", err)
	}

	malformedErr := &Error{
		Code:       ErrInternalError,
		Stream:     9,
		Msg:        "trailing HEADERS block without END_STREAM",
		Underlying: ErrMalformedTrailers,
	}
	if !errors.Is(malformedErr, ErrMalformedTrailers) {
		t.Fatalf("errors.Is(malformedErr, ErrMalformedTrailers) = false, want true (test fixture does not match the production shape)")
	}
	rejectionAction := &errorAction{err: malformedErr}

	ctx := context.Background()
	s.dispatch(ctx, dispatchWith(func() Action { return rejectionAction }))

	if len(fc.headers) != 0 {
		t.Errorf("headers frames written = %d, want 0 (malformed-trailers rejection must not emit a HEADERS frame / 502)", len(fc.headers))
	}
	if len(fc.data) != 0 {
		t.Errorf("data frames written = %d, want 0", len(fc.data))
	}
	if len(fc.rsts) != 1 {
		t.Fatalf("RST_STREAM calls = %d, want 1", len(fc.rsts))
	}
	if fc.rsts[0].id != 9 {
		t.Errorf("RST_STREAM stream id = %d, want 9", fc.rsts[0].id)
	}
	if fc.rsts[0].code != ErrInternalError {
		t.Errorf("RST_STREAM code = %v, want INTERNAL_ERROR", fc.rsts[0].code)
	}
}

// TestBuildRequest_ConnectionSpecificFields is the REQUEST-side per-member
// table over buildRequest, the counterpart to Table A
// (TestValidateResponseTrailers_Table in trailers_validate_test.go) on the
// response-trailer side. PLAN-84.1 break C proved that removing a single
// member (e.g. "upgrade") from the shared isConnectionSpecificField set in
// stream.go reddened ONLY the response-trailers table — zero of this
// package's test functions covered the request side per member, so
// buildRequest's use of the shared set (stream.go ~:444-448) was ungated.
//
// ONE CASE PER MEMBER, not one case for the whole set: a single "some
// connection-specific field" case cannot catch a member dropped from the
// shared set, exactly as the response-side comment already explains for
// Table A. Also covers the `te` value rule (RFC 9113 §8.2.2: `te` is
// conditionally legal, permitted only with the value "trailers") and a
// `te: trailers` PASS control so the positive path stays proven too.
func TestBuildRequest_ConnectionSpecificFields(t *testing.T) {
	tests := []struct {
		name       string
		field      hpack.HeaderField
		wantErrMsg string // empty => must PASS (legal field)
	}{
		// --- RFC 9113 §8.2.2 connection-specific set: ONE CASE PER MEMBER ---
		{name: "connection", field: hpack.HeaderField{Name: "connection", Value: "keep-alive"}, wantErrMsg: "connection-specific header field: connection"},
		{name: "keep-alive", field: hpack.HeaderField{Name: "keep-alive", Value: "timeout=5"}, wantErrMsg: "connection-specific header field: keep-alive"},
		{name: "proxy-connection", field: hpack.HeaderField{Name: "proxy-connection", Value: "keep-alive"}, wantErrMsg: "connection-specific header field: proxy-connection"},
		{name: "transfer-encoding", field: hpack.HeaderField{Name: "transfer-encoding", Value: "chunked"}, wantErrMsg: "connection-specific header field: transfer-encoding"},
		{name: "upgrade", field: hpack.HeaderField{Name: "upgrade", Value: "websocket"}, wantErrMsg: "connection-specific header field: upgrade"},

		// --- `te` value rule (must REJECT any value other than "trailers") ---
		{name: "te_gzip_rejected", field: hpack.HeaderField{Name: "te", Value: "gzip"}, wantErrMsg: "TE header field value not 'trailers'"},

		// --- PASS control: te:trailers is the one legal value ---
		{name: "te_trailers_passes", field: hpack.HeaderField{Name: "te", Value: "trailers"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			headers := append(append([]hpack.HeaderField{}, minHeaders()...), tc.field)
			req, err := buildRequest(headers, strings.NewReader(""))

			if tc.wantErrMsg == "" {
				if err != nil {
					t.Errorf("buildRequest error = %v, want nil (legal field)", err)
				}
				if req == nil {
					t.Errorf("buildRequest req = nil, want a built *http.Request")
				}
				return
			}

			if err == nil {
				t.Fatalf("buildRequest error = nil, want a rejection")
			}
			if req != nil {
				t.Errorf("buildRequest req = %v, want nil on rejection", req)
			}
			herr, ok := err.(*Error)
			if !ok {
				t.Fatalf("buildRequest error type = %T, want *Error", err)
			}
			if herr.Code != ErrProtocolError {
				t.Errorf("Code = %v, want PROTOCOL_ERROR", herr.Code)
			}
			// Exact equality (not a substring match): buildRequest's messages
			// are fixed unquoted strings, so an exact-equality assertion is
			// already fully discriminating without needing the quoting trick
			// Table A uses on the response side (whose prefix text happens to
			// contain some member names as substrings; buildRequest's prefix
			// text does not).
			if herr.Msg != tc.wantErrMsg {
				t.Errorf("Msg = %q, want %q", herr.Msg, tc.wantErrMsg)
			}
		})
	}
}

// TestBuildRequest_PathForms is the per-FORM table over the other half of
// buildRequest: the parse of the :path pseudo-header into req.URL. (Cited by
// SYMBOL deliberately — that call's line number has already drifted :440 ->
// :465 -> :478 across this row's own lineage.) The table exists because the
// two grammars that meet at that one call disagree about exactly one byte
// sequence.
//
// In HTTP/2 an origin-form :path is a request-target: RFC 9113 §8.3.1 says it
// carries the path-and-query of the target URI, and RFC 9110 §7.1 defines
// origin-form as absolute-path [ "?" query ] — so a leading "//" is simply
// PATH BYTES (an absolute-path whose first segment is empty). The generic
// RFC 3986 §4.2 URI-reference grammar reads the same bytes differently: a
// relative-reference beginning "//" is a NETWORK-PATH reference, and the
// segment after the slashes is an AUTHORITY, not a path segment. A parser
// that accepts the full URI-reference grammar therefore peels the first
// segment off the path and into u.Host. The routing consequences are two
// distinct failures, which is why the double-slash rows below are split by
// FORM rather than collapsed into one "some double-slash path" row (the same
// reason Table B — TestBuildRequest_ConnectionSpecificFields — takes one case
// per member: a single representative case cannot catch a form that regresses
// on its own):
//
//   - "//foo" peels ENTIRELY into the authority, leaving an EMPTY path, so
//     the request routes as if no path were sent at all (404).
//   - "//foo/bar" peels only "foo", leaving "/bar" — the request is SILENTLY
//     MIS-ROUTED to a different, existing route. No error, wrong backend.
//
// Reject rows: an origin-form :path has no fragment component at all (RFC
// 9110 §7.1: fragments are never sent on the wire), and a rootless or
// query-only target is not an origin-form request-target either. Both
// families must be rejected, and the table keeps them DISTINGUISHABLE by
// asserting Msg exactly plus Underlying for NIL-NESS ONLY: the parse-reject
// family wraps whatever net/url returned (non-nil Underlying), the
// fragment-reject family is a first-party check with nothing to wrap (nil).
// The wrapped stdlib error TEXT is deliberately NOT asserted — its wording is
// net/url's and is hostage to the Go toolchain version.
//
// Control rows are labeled in the table. The five non-"//" accept forms are
// GREEN AT THE PRE-FIX TIP and are REGRESSION CONTROLS, not anchors — a
// green-on-arrival row proves nothing about a fix. Likewise the
// RequestURI-stays-the-literal-:path-bytes assertion already holds at the
// pre-fix tip on every accepted row (buildRequest assigns RequestURI from the
// raw pseudo-header value, never from the parsed URL); it is carried on every
// accept row as a control so that no fix to the parse is allowed to start
// rewriting the literal request-target.
func TestBuildRequest_PathForms(t *testing.T) {
	tests := []struct {
		name string
		path string // the :path pseudo-header value

		// Accept expectations (used when wantErrMsg == "").
		wantPath     string
		wantRawQuery string
		// wantRawPath is asserted only when non-empty. net/url populates
		// RawPath ONLY when the escaped form differs from Path, so "" is the
		// correct value on every row without a percent-escape and asserting
		// it there would pin an implementation detail rather than a property.
		wantRawPath string

		// Reject expectations. wantErrMsg != "" selects the reject arm and is
		// matched by EXACT equality. wantUnderlying records only whether
		// *Error.Underlying must be non-nil — never its text.
		wantErrMsg     string
		wantUnderlying bool
	}{
		// --- REGRESSION CONTROLS (green at the pre-fix tip; NOT anchors) ---
		{name: "root", path: "/", wantPath: "/"},
		{name: "simple", path: "/foo", wantPath: "/foo"},
		{name: "query", path: "/foo?a=b", wantPath: "/foo", wantRawQuery: "a=b"},
		{name: "midpath_double_slash", path: "/a//b", wantPath: "/a//b"},
		{name: "asterisk_form", path: "*", wantPath: "*"},
		// Percent-escape semantics: Path is the DECODED form and RawPath the
		// literal wire bytes, and the two must stay paired. This row is what
		// entitles the contract to say escape semantics are unchanged by the
		// primitive swap — without it that claim is asserted nowhere.
		{name: "percent_escaped_slash", path: "/foo%2Fbar", wantPath: "/foo/bar", wantRawPath: "/foo%2Fbar"},

		// --- RED at the pre-fix tip: the row-87 anchors ---
		// Each of these peels its first segment into u.Host today, so the
		// path arrives empty ("//foo", "//") or truncated ("//foo/bar" ->
		// "/bar", the silent mis-route).
		{name: "leading_double_slash", path: "//foo", wantPath: "//foo"},
		{name: "bare_double_slash", path: "//", wantPath: "//"},
		{name: "leading_double_slash_multi_segment", path: "//foo/bar", wantPath: "//foo/bar"},
		{name: "leading_double_slash_query", path: "//foo?x=1", wantPath: "//foo", wantRawQuery: "x=1"},

		// --- RED at the pre-fix tip: all four are silently ACCEPTED today ---
		// The two families are kept apart by Underlying's nil-ness, not by
		// the wrapped stdlib text.
		{name: "rootless", path: "foo", wantErrMsg: "bad :path", wantUnderlying: true},
		{name: "bare_query", path: "?a=b", wantErrMsg: "bad :path", wantUnderlying: true},
		{name: "fragment", path: "/foo#frag", wantErrMsg: "fragment in :path"},
		{name: "query_and_fragment", path: "/foo?a=b#frag", wantErrMsg: "fragment in :path"},
		// ORDERING PIN, and the only row that has this job. Every other reject
		// row is either fragment-bearing-but-otherwise-valid or
		// invalid-without-a-fragment, so the two families never intersect and
		// NO other row can tell "guard before the parse" from "guard after
		// it". This one is both at once: a rootless target that also carries a
		// fragment. With the guard before the parse it is "fragment in :path"
		// with a nil Underlying; move the guard below the parse and it becomes
		// "bad :path" with a non-nil Underlying, and this row — alone — goes
		// red. Do not delete it as a near-duplicate of "rootless".
		{name: "rootless_fragment", path: "foo#frag", wantErrMsg: "fragment in :path"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			headers := minHeaders()
			for i := range headers {
				if headers[i].Name == ":path" {
					headers[i].Value = tc.path
				}
			}
			req, err := buildRequest(headers, strings.NewReader(""))

			if tc.wantErrMsg == "" {
				if err != nil {
					t.Fatalf("buildRequest(:path=%q) error = %v, want nil (legal origin-form target)", tc.path, err)
				}
				if req == nil {
					t.Fatalf("buildRequest(:path=%q) req = nil, want a built *http.Request", tc.path)
				}
				if req.URL.Path != tc.wantPath {
					t.Errorf("URL.Path = %q, want %q (:path=%q)", req.URL.Path, tc.wantPath, tc.path)
				}
				if req.URL.RawQuery != tc.wantRawQuery {
					t.Errorf("URL.RawQuery = %q, want %q (:path=%q)", req.URL.RawQuery, tc.wantRawQuery, tc.path)
				}
				// Asserted only where the row pins it: net/url leaves RawPath
				// empty whenever the escaped form equals Path, so "" is correct
				// on every unescaped row and pinning it there would assert an
				// implementation detail instead of a property.
				if tc.wantRawPath != "" && req.URL.RawPath != tc.wantRawPath {
					t.Errorf("URL.RawPath = %q, want %q (:path=%q)", req.URL.RawPath, tc.wantRawPath, tc.path)
				}
				// Control: RequestURI must stay the LITERAL :path bytes.
				if req.RequestURI != tc.path {
					t.Errorf("RequestURI = %q, want the literal :path %q", req.RequestURI, tc.path)
				}
				return
			}

			if err == nil {
				t.Fatalf("buildRequest(:path=%q) error = nil, want a rejection", tc.path)
			}
			if req != nil {
				t.Errorf("buildRequest(:path=%q) req = %v, want nil on rejection", tc.path, req)
			}
			herr, ok := err.(*Error)
			if !ok {
				t.Fatalf("buildRequest(:path=%q) error type = %T, want *Error", tc.path, err)
			}
			if herr.Code != ErrProtocolError {
				t.Errorf("Code = %v, want PROTOCOL_ERROR (:path=%q)", herr.Code, tc.path)
			}
			if herr.Msg != tc.wantErrMsg {
				t.Errorf("Msg = %q, want %q (:path=%q)", herr.Msg, tc.wantErrMsg, tc.path)
			}
			// Nil-ness ONLY: the wrapped stdlib wording is not asserted.
			if tc.wantUnderlying && herr.Underlying == nil {
				t.Errorf("Underlying = nil, want a non-nil wrapped parse error (:path=%q)", tc.path)
			}
			if !tc.wantUnderlying && herr.Underlying != nil {
				t.Errorf("Underlying = %v, want nil (first-party reject wraps nothing) (:path=%q)", herr.Underlying, tc.path)
			}
		})
	}
}

// TestBuildH2Request_PseudoHeaderExclusion — phase 89 (ADR-0311)
// CHARACTERIZATION PIN. It PASSES at the unmodified tip; it is not part of the
// phase-89 RED census.
//
// buildH2Request had ZERO test coverage before phase 89 (`git grep -l -w
// buildH2Request -- '*_test.go'` => 0 files), yet its pseudo-header EXCLUSION
// contract is what makes the phase-89 decode-delta reconciler's ':'-skip
// necessary AND sufficient: the ordered carrier the reconciler edits must never
// contain a ':'-prefixed field, because h2/client.go re-prepends the four
// request pseudo-headers itself and a second copy on the wire is a
// PROTOCOL_ERROR. Pin it so a future refactor cannot quietly start leaking
// pseudo-headers onto .Headers.
func TestBuildH2Request_PseudoHeaderExclusion(t *testing.T) {
	tests := []struct {
		name        string
		headers     []hpack.HeaderField
		wantMethod  string
		wantPath    string
		wantScheme  string
		wantAuth    string
		wantHeaders []hpack.HeaderField
	}{
		{
			name:        "minimal_pseudo_only_yields_empty_regular_set",
			headers:     minHeaders(),
			wantMethod:  "GET",
			wantPath:    "/",
			wantScheme:  "https",
			wantAuth:    "example.com",
			wantHeaders: nil,
		},
		{
			name: "regular_headers_kept_in_wire_order",
			headers: append(minHeaders(),
				hpack.HeaderField{Name: "x-b", Value: "2"},
				hpack.HeaderField{Name: "x-a", Value: "1"},
			),
			wantMethod: "GET",
			wantPath:   "/",
			wantScheme: "https",
			wantAuth:   "example.com",
			wantHeaders: []hpack.HeaderField{
				{Name: "x-b", Value: "2"},
				{Name: "x-a", Value: "1"},
			},
		},
		{
			name: "duplicate_regular_names_kept_non_adjacent",
			headers: append(minHeaders(),
				hpack.HeaderField{Name: "x-dup", Value: "one"},
				hpack.HeaderField{Name: "x-mid", Value: "mid"},
				hpack.HeaderField{Name: "x-dup", Value: "three"},
			),
			wantMethod: "GET",
			wantPath:   "/",
			wantScheme: "https",
			wantAuth:   "example.com",
			wantHeaders: []hpack.HeaderField{
				{Name: "x-dup", Value: "one"},
				{Name: "x-mid", Value: "mid"},
				{Name: "x-dup", Value: "three"},
			},
		},
		{
			name: "unknown_pseudo_header_is_excluded_not_forwarded",
			headers: append(minHeaders(),
				hpack.HeaderField{Name: ":protocol", Value: "connect-udp"},
				hpack.HeaderField{Name: "x-a", Value: "1"},
			),
			wantMethod:  "GET",
			wantPath:    "/",
			wantScheme:  "https",
			wantAuth:    "example.com",
			wantHeaders: []hpack.HeaderField{{Name: "x-a", Value: "1"}},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := buildH2Request(tc.headers, []byte("body"))
			if got.Method != tc.wantMethod {
				t.Errorf("Method = %q, want %q", got.Method, tc.wantMethod)
			}
			if got.Path != tc.wantPath {
				t.Errorf("Path = %q, want %q", got.Path, tc.wantPath)
			}
			if got.Scheme != tc.wantScheme {
				t.Errorf("Scheme = %q, want %q", got.Scheme, tc.wantScheme)
			}
			if got.Authority != tc.wantAuth {
				t.Errorf("Authority = %q, want %q", got.Authority, tc.wantAuth)
			}
			if string(got.Body) != "body" {
				t.Errorf("Body = %q, want %q", got.Body, "body")
			}
			for _, h := range got.Headers {
				if len(h.Name) > 0 && h.Name[0] == ':' {
					t.Errorf("Headers carries pseudo-header %q; buildH2Request must exclude every ':'-prefixed field", h.Name)
				}
			}
			if len(got.Headers) != len(tc.wantHeaders) {
				t.Fatalf("Headers = %v, want %v", got.Headers, tc.wantHeaders)
			}
			for i := range tc.wantHeaders {
				if got.Headers[i] != tc.wantHeaders[i] {
					t.Errorf("Headers[%d] = %v, want %v", i, got.Headers[i], tc.wantHeaders[i])
				}
			}
		})
	}
}
