package h2

import (
	"context"
	"io"
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

func (fa *fakeAction) WriteH2(sw StreamWriter) error {
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
// After recvHeaders, recvRSTStream(CANCEL) → closed; reading from reqBodyR returns error.
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

	// Reading from reqBodyR should return an error (the RST error was injected).
	buf := make([]byte, 4)
	_, err := s.reqBodyR.Read(buf)
	if err == nil {
		t.Fatal("reqBodyR.Read returned nil, want error after RST_STREAM")
	}
	if !strings.Contains(err.Error(), "h2:") {
		t.Errorf("error does not start with h2: prefix, got: %v", err)
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

func (a *errorAction) WriteH2(_ StreamWriter) error {
	return a.err
}

// TestServerStream_Dispatch_404Adapter_WritesHeadersAndData:
// A 404-synthesising Action writes HEADERS with :status 404 + DATA body.
// This models the h2DirectResponseAdapter wrapping a 404 directResponseAction.
func TestServerStream_Dispatch_404Adapter_WritesHeadersAndData(t *testing.T) {
	fc := &fakeConn{}
	s := newServerStream(7, fc, 65535, 65535)

	if err := s.recvHeaders(minHeaders(), true); err != nil {
		t.Fatalf("recvHeaders: %v", err)
	}

	// Simulate the 404-synthesising adapter.
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

// TestServerStream_RejectsEvenClientStreamID:
// validateClientStreamID(2, empty) → *Error{Code: ErrProtocolError}.
func TestServerStream_RejectsEvenClientStreamID(t *testing.T) {
	err := validateClientStreamID(2, map[uint32]struct{}{})
	if err == nil {
		t.Fatal("expected error for even stream id, got nil")
	}
	h2err, ok := err.(*Error)
	if !ok {
		t.Fatalf("error is %T, want *Error", err)
	}
	if h2err.Code != ErrProtocolError {
		t.Errorf("error code = %v, want PROTOCOL_ERROR", h2err.Code)
	}
}

// TestServerStream_RejectsStreamIDReuse:
// validateClientStreamID(3, {3:{}}) → *Error{Code: ErrProtocolError}.
func TestServerStream_RejectsStreamIDReuse(t *testing.T) {
	err := validateClientStreamID(3, map[uint32]struct{}{3: {}})
	if err == nil {
		t.Fatal("expected error for stream id reuse, got nil")
	}
	h2err, ok := err.(*Error)
	if !ok {
		t.Fatalf("error is %T, want *Error", err)
	}
	if h2err.Code != ErrProtocolError {
		t.Errorf("error code = %v, want PROTOCOL_ERROR", h2err.Code)
	}
}
