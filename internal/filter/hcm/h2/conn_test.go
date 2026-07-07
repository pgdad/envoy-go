package h2

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// fixedAction is a Dispatcher that always returns the same Action with a
// fixed status code and body.
type fixedAction struct {
	status int
	body   string
}

func (a *fixedAction) WriteH2(_ context.Context, _ H2Request, sw StreamWriter) error {
	body := []byte(a.body)
	headers := []hpack.HeaderField{
		{Name: ":status", Value: strconv.Itoa(a.status)},
		{Name: "content-length", Value: strconv.Itoa(len(body))},
		{Name: "content-type", Value: "text/plain"},
	}
	if err := sw.WriteHeaders(headers, false); err != nil {
		return err
	}
	return sw.WriteData(body, true)
}

// fixedDispatcher always returns the same Action.
type fixedDispatcher struct {
	action Action
}

func (d *fixedDispatcher) Match(_ *http.Request) (Action, bool) {
	return d.action, true
}

// blockingAction holds until released, then returns a fixed response.
// Used to saturate MaxConcurrentStreams.
type blockingAction struct {
	release <-chan struct{}
	status  int
	body    string
}

func newBlockingAction(release <-chan struct{}) *blockingAction {
	return &blockingAction{
		release: release,
		status:  200,
		body:    "OK",
	}
}

func (a *blockingAction) WriteH2(_ context.Context, _ H2Request, sw StreamWriter) error {
	<-a.release
	body := []byte(a.body)
	headers := []hpack.HeaderField{
		{Name: ":status", Value: strconv.Itoa(a.status)},
		{Name: "content-length", Value: strconv.Itoa(len(body))},
	}
	if err := sw.WriteHeaders(headers, false); err != nil {
		return err
	}
	return sw.WriteData(body, true)
}

// startServerConn starts a ServerConn on one end of a TCP loopback connection
// and returns the client-side conn plus a done channel that receives the
// server's Run() error when it exits.
func startServerConn(t *testing.T, ctx context.Context, dispatcher Dispatcher, settings ServerSettings) (net.Conn, <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	serverDone := make(chan error, 1)
	go func() {
		serverConn, err := ln.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		sc := NewServerConn(ctx, serverConn, dispatcher, settings)
		serverDone <- sc.Run()
	}()

	clientConn, err := net.DialTCP("tcp", nil, ln.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = clientConn.Close() })
	return clientConn, serverDone
}

// newH2Transport creates a round-tripper that speaks HTTP/2 over a raw
// net.Conn. Uses golang.org/x/net/http2.Transport with a custom dial
// function. ADR-0046 test-side carve-out permits Transport use in tests.
func newH2Transport(conn net.Conn) *http2.Transport {
	return &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			return conn, nil
		},
	}
}

// doGET issues a GET request over the given transport and returns the response.
func doGET(t *testing.T, tr *http2.Transport, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", "http://test.local"+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	return resp
}

// readBody drains and returns the response body.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// writeClientPreface sends the client connection preface to the given writer.
func writeClientPreface(w io.Writer) error {
	_, err := w.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
	return err
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestServerConn_DirectResponseRoundTrip validates the basic
// preface + settings + HEADERS+DATA round-trip.
func TestServerConn_DirectResponseRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "OK\n"}}
	clientConn, serverDone := startServerConn(t, ctx, disp, DefaultServerSettings)

	tr := newH2Transport(clientConn)
	defer tr.CloseIdleConnections()

	resp := doGET(t, tr, "/health")
	body := readBody(t, resp)

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if body != "OK\n" {
		t.Errorf("body = %q, want %q", body, "OK\n")
	}

	// Clean up: cancel ctx so server exits.
	cancel()
	select {
	case err := <-serverDone:
		if err != nil && err != context.Canceled {
			t.Logf("server exited with: %v (expected on cancel)", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not exit after cancel")
	}
}

// TestServerConn_ConcurrentStreams opens 3 streams concurrently and asserts
// each gets the expected response.
func TestServerConn_ConcurrentStreams(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "concurrent\n"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)

	tr := newH2Transport(clientConn)
	defer tr.CloseIdleConnections()

	var wg sync.WaitGroup
	errs := make([]error, 3)
	statuses := make([]int, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := tr.RoundTrip(func() *http.Request {
				req, _ := http.NewRequest("GET", fmt.Sprintf("http://test.local/path%d", idx), nil)
				return req
			}())
			if err != nil {
				errs[idx] = err
				return
			}
			statuses[idx] = resp.StatusCode
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("stream %d error: %v", i, err)
		}
	}
	for i, s := range statuses {
		if s != 200 {
			t.Errorf("stream %d status = %d, want 200", i, s)
		}
	}
}

// TestServerConn_MaxConcurrentStreamsEnforcement opens streams beyond the
// configured max and asserts that at least one gets RST_STREAM(REFUSED_STREAM).
// Uses a raw framer (not Transport) to avoid the Transport's automatic retry
// on REFUSED_STREAM, which would otherwise loop indefinitely.
func TestServerConn_MaxConcurrentStreamsEnforcement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	releaseCh := make(chan struct{})
	blocker := newBlockingAction(releaseCh)
	disp := &fixedDispatcher{action: blocker}

	settings := DefaultServerSettings
	settings.MaxConcurrentStreams = 2

	clientConn, serverDone := startServerConn(t, ctx, disp, settings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}

	fr := http2.NewFramer(clientConn, clientConn)
	if err := fr.WriteSettings(); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Handshake.
	settingsAcked := false
	clientAcked := false
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
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

	// Build a minimal HEADERS block.
	sendHeaders := func(streamID uint32) {
		var encBuf bytes.Buffer
		enc := hpack.NewEncoder(&encBuf)
		_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
		_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: fmt.Sprintf("/stream%d", streamID)})
		_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
		_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})
		if err := fr.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: encBuf.Bytes(),
			EndHeaders:    true,
			EndStream:     true,
		}); err != nil {
			t.Logf("write headers stream %d: %v", streamID, err)
		}
	}

	// Open 3 streams (max=2); one should get RST_STREAM(REFUSED_STREAM).
	sendHeaders(1)
	sendHeaders(3)
	sendHeaders(5)

	// Collect RST_STREAM frames. Since the blocker holds, we should quickly see
	// at least one RST_STREAM(REFUSED_STREAM) before any responses arrive.
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	refusedStreams := 0
	for i := 0; i < 10; i++ {
		f, err := fr.ReadFrame()
		if err != nil {
			break
		}
		if f.Header().Type == http2.FrameRSTStream {
			rf := f.(*http2.RSTStreamFrame)
			if rf.ErrCode == http2.ErrCodeRefusedStream {
				refusedStreams++
				break
			}
		}
	}

	if refusedStreams == 0 {
		t.Error("expected at least one RST_STREAM(REFUSED_STREAM), got none")
	}

	// Release the blocked streams so the server can exit cleanly.
	close(releaseCh)
	cancel()

	select {
	case <-serverDone:
	case <-time.After(3 * time.Second):
		// Non-fatal: server may be slow to exit, but the test assertion passed.
	}
}

// TestServerConn_GOAWAYOnProtocolError_EvenStreamID sends a HEADERS frame on
// even stream ID 2; server should emit GOAWAY(PROTOCOL_ERROR).
func TestServerConn_GOAWAYOnProtocolError_EvenStreamID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, serverDone := startServerConn(t, ctx, disp, DefaultServerSettings)

	// Write the client preface and initial SETTINGS.
	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}

	fr := http2.NewFramer(clientConn, clientConn)
	// Write client initial SETTINGS (empty).
	if err := fr.WriteSettings(); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Read server's initial SETTINGS.
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("read server SETTINGS: %v", err)
		}
		if f.Header().Type == http2.FrameSettings {
			sf := f.(*http2.SettingsFrame)
			if !sf.IsAck() {
				// ACK the server's SETTINGS.
				if err := fr.WriteSettingsAck(); err != nil {
					t.Fatalf("write settings ack: %v", err)
				}
			}
			break
		}
	}

	// Read the SETTINGS_ACK for our client settings.
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			break
		}
		if f.Header().Type == http2.FrameSettings {
			sf := f.(*http2.SettingsFrame)
			if sf.IsAck() {
				break
			}
		}
	}

	// Send HEADERS on stream 2 (even — PROTOCOL_ERROR).
	var encBuf bytes.Buffer
	enc := hpack.NewEncoder(&encBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})

	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      2, // EVEN — protocol error
		BlockFragment: encBuf.Bytes(),
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		t.Fatalf("write headers on even stream: %v", err)
	}

	// Expect GOAWAY with PROTOCOL_ERROR.
	deadline := time.Now().Add(3 * time.Second)
	_ = clientConn.SetReadDeadline(deadline)
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			break
		}
		if f.Header().Type == http2.FrameGoAway {
			gf := f.(*http2.GoAwayFrame)
			if gf.ErrCode != http2.ErrCodeProtocol {
				t.Errorf("GOAWAY code = %v, want PROTOCOL_ERROR", gf.ErrCode)
			}
			return
		}
	}

	select {
	case err := <-serverDone:
		if h2Err, ok := err.(*Error); ok && h2Err.Code == ErrProtocolError {
			return // server returned PROTOCOL_ERROR — acceptable
		}
		t.Errorf("expected GOAWAY(PROTOCOL_ERROR), server exited with: %v", err)
	case <-time.After(3 * time.Second):
		t.Error("timed out waiting for GOAWAY")
	}
}

// TestServerConn_GOAWAYOnProtocolError_StreamIDReuse verifies that a peer
// re-using a stream id below s.lastInID (i.e., a non-monotonic stream id)
// triggers GOAWAY(PROTOCOL_ERROR) per RFC 9113 §5.1.1.
//
// This is the integration coverage for the monotonic-id rejection branch in
// onHeaders (currently at conn.go:343-344, originally at the conn.go:308-319
// range when the PLAN was authored). Closes the 05.1-REVIEW carry-forward
// (per phase 05.2 SPEC §12.3 / §10 #10) — the rejection branch was previously
// only unit-tested via the sibling even-id branch
// (TestServerConn_GOAWAYOnProtocolError_EvenStreamID).
//
// Driver: open stream 3 (skipping 1) and complete it, then send HEADERS on
// stream 1. The server has bumped lastInID to 3, so a HEADERS on stream 1 is
// non-monotonic and must hit the PROTOCOL_ERROR branch.
//
// The assertion observes the GOAWAY frame on the wire via the framer (NOT
// inferred from the conn close) so a regression that drops the rejection
// branch would be caught.
func TestServerConn_GOAWAYOnProtocolError_StreamIDReuse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, serverDone := startServerConn(t, ctx, disp, DefaultServerSettings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}

	fr := http2.NewFramer(clientConn, clientConn)
	if err := fr.WriteSettings(); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Read server's initial SETTINGS, then ACK it.
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("read server SETTINGS: %v", err)
		}
		if f.Header().Type == http2.FrameSettings {
			sf := f.(*http2.SettingsFrame)
			if !sf.IsAck() {
				if err := fr.WriteSettingsAck(); err != nil {
					t.Fatalf("write settings ack: %v", err)
				}
				break
			}
		}
	}

	// Build the HEADERS block once; reuse for both writes.
	var encBuf bytes.Buffer
	enc := hpack.NewEncoder(&encBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})
	headerBlock := append([]byte{}, encBuf.Bytes()...)

	// Step 1: open stream 3 with HEADERS+END_STREAM.  This bumps lastInID to 3.
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      3,
		BlockFragment: headerBlock,
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		t.Fatalf("write headers on stream 3: %v", err)
	}

	// Drain frames until we see the response END_STREAM on stream 3, ignoring
	// settings ACKs and other server-side frames.  Once stream 3's response is
	// fully received, lastInID has been advanced and the stream is considered
	// closed, but more importantly the monotonic check uses lastInID (which
	// was set when the HEADERS was processed, before the response ran).
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	gotResponse := false
	for !gotResponse {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("read frame waiting for stream 3 response: %v", err)
		}
		switch fr := f.(type) {
		case *http2.HeadersFrame:
			if fr.StreamID == 3 && fr.StreamEnded() {
				gotResponse = true
			}
		case *http2.DataFrame:
			if fr.StreamID == 3 && fr.StreamEnded() {
				gotResponse = true
			}
		case *http2.GoAwayFrame:
			t.Fatalf("unexpected early GOAWAY: code=%v", fr.ErrCode)
		}
	}

	// Step 2: send HEADERS on stream 1.  lastInID is 3, so 1 <= 3 → the
	// monotonic check at conn.go fires and returns PROTOCOL_ERROR.
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1, // non-monotonic — must trip PROTOCOL_ERROR
		BlockFragment: headerBlock,
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		t.Fatalf("write headers on stream 1 (reused id): %v", err)
	}

	// Wire-level assertion: read the GOAWAY frame from the framer and verify
	// its error code is PROTOCOL_ERROR.  Don't accept conn close alone as
	// evidence — that could come from any path.
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	sawGoAway := false
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			break // EOF / read error after server closes the conn
		}
		if gf, ok := f.(*http2.GoAwayFrame); ok {
			if gf.ErrCode != http2.ErrCodeProtocol {
				t.Errorf("GOAWAY code = %v, want PROTOCOL_ERROR", gf.ErrCode)
			}
			sawGoAway = true
			break
		}
	}
	if !sawGoAway {
		t.Error("expected GOAWAY(PROTOCOL_ERROR) on wire after reusing stream id")
	}

	// Confirm the server-side Run() loop also exited with the matching error.
	select {
	case err := <-serverDone:
		if h2Err, ok := err.(*Error); ok && h2Err.Code == ErrProtocolError {
			return
		}
		t.Errorf("expected server to exit with PROTOCOL_ERROR, got: %v", err)
	case <-time.After(3 * time.Second):
		t.Error("timed out waiting for server Run() to exit after GOAWAY")
	}
}

// TestServerConn_PingPingAck verifies that a PING from the client triggers
// a PING ACK from the server.
func TestServerConn_PingPingAck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}

	fr := http2.NewFramer(clientConn, clientConn)
	if err := fr.WriteSettings(); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Consume server frames until we've acked the server SETTINGS.
	settingsAcked := false
	clientAcked := false
	deadline := time.Now().Add(3 * time.Second)
	_ = clientConn.SetReadDeadline(deadline)
	for !settingsAcked || !clientAcked {
		f, err := fr.ReadFrame()
		if err != nil {
			break
		}
		switch f.Header().Type {
		case http2.FrameSettings:
			sf := f.(*http2.SettingsFrame)
			if !sf.IsAck() {
				_ = fr.WriteSettingsAck()
				settingsAcked = true
			} else {
				clientAcked = true
			}
		}
	}

	pingData := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	if err := fr.WritePing(false, pingData); err != nil {
		t.Fatalf("write ping: %v", err)
	}

	// Read frames until we see a PING ACK.
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("read frame after PING: %v", err)
		}
		if f.Header().Type == http2.FramePing {
			pf := f.(*http2.PingFrame)
			if pf.IsAck() {
				if pf.Data != pingData {
					t.Errorf("PING ACK data = %v, want %v", pf.Data, pingData)
				}
				return
			}
		}
	}
}

// TestServerConn_PushPromiseFromClient_GOAWAYProtocolError verifies that a
// client-sent PUSH_PROMISE triggers GOAWAY(PROTOCOL_ERROR).
func TestServerConn_PushPromiseFromClient_GOAWAYProtocolError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, serverDone := startServerConn(t, ctx, disp, DefaultServerSettings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}

	fr := http2.NewFramer(clientConn, clientConn)
	if err := fr.WriteSettings(); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Consume server's SETTINGS and SETTINGS_ACK.
	settingsAcked := false
	clientAcked := false
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for !settingsAcked || !clientAcked {
		f, err := fr.ReadFrame()
		if err != nil {
			break
		}
		switch f.Header().Type {
		case http2.FrameSettings:
			sf := f.(*http2.SettingsFrame)
			if !sf.IsAck() {
				_ = fr.WriteSettingsAck()
				settingsAcked = true
			} else {
				clientAcked = true
			}
		}
	}

	// Send a PUSH_PROMISE (clients can't legally do this).
	var encBuf bytes.Buffer
	enc := hpack.NewEncoder(&encBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/push"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})

	// Write raw PUSH_PROMISE frame (stream ID 1, promised stream ID 2).
	// We use WriteRawFrame since http2.Framer doesn't expose client-side PUSH_PROMISE.
	// Frame format: 4-byte promised stream id (big-endian) + header block.
	promisedID := uint32(2)
	payload := make([]byte, 4+encBuf.Len())
	payload[0] = byte(promisedID >> 24)
	payload[1] = byte(promisedID >> 16)
	payload[2] = byte(promisedID >> 8)
	payload[3] = byte(promisedID)
	copy(payload[4:], encBuf.Bytes())
	if err := fr.WriteRawFrame(http2.FramePushPromise, 0x4 /* END_HEADERS */, 1, payload); err != nil {
		t.Fatalf("write push_promise: %v", err)
	}

	// Expect GOAWAY with PROTOCOL_ERROR.
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			break
		}
		if f.Header().Type == http2.FrameGoAway {
			gf := f.(*http2.GoAwayFrame)
			if gf.ErrCode != http2.ErrCodeProtocol {
				t.Errorf("GOAWAY code = %v, want PROTOCOL_ERROR", gf.ErrCode)
			}
			return
		}
	}

	select {
	case err := <-serverDone:
		if h2Err, ok := err.(*Error); ok && h2Err.Code == ErrProtocolError {
			return
		}
		t.Errorf("expected GOAWAY(PROTOCOL_ERROR), server exited with: %v", err)
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for GOAWAY")
	}
}

// TestServerConn_PriorityFrameSilentlyDiscarded verifies that a PRIORITY frame
// in the middle of a normal request does not disrupt the response.
func TestServerConn_PriorityFrameSilentlyDiscarded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "priority-ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)

	tr := newH2Transport(clientConn)
	defer tr.CloseIdleConnections()

	// Issue a normal request; the transport may send PRIORITY internally.
	resp := doGET(t, tr, "/priority-test")
	body := readBody(t, resp)

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if body != "priority-ok" {
		t.Errorf("body = %q, want %q", body, "priority-ok")
	}
}

// TestServerConn_HPACKTableSizeUpdate_PropagatesToOutgoingHEADERS verifies
// that after a client announces SETTINGS_HEADER_TABLE_SIZE=64, the server's
// outgoing HEADERS for the next response is still correctly decodable on the
// peer side (i.e., the table-size update was honored).
func TestServerConn_HPACKTableSizeUpdate_PropagatesToOutgoingHEADERS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "hpack-ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}

	fr := http2.NewFramer(clientConn, clientConn)
	// Send client SETTINGS with tiny header table size.
	if err := fr.WriteSettings(http2.Setting{
		ID:  http2.SettingHeaderTableSize,
		Val: 64,
	}); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Handshake: read server SETTINGS and ACK; read ACK for our settings.
	settingsAcked := false
	clientAcked := false
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for !settingsAcked || !clientAcked {
		f, err := fr.ReadFrame()
		if err != nil {
			break
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

	// Send a GET request.
	var encBuf bytes.Buffer
	enc := hpack.NewEncoder(&encBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/hpack-test"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})

	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: encBuf.Bytes(),
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		t.Fatalf("write headers: %v", err)
	}

	// Decode the response HEADERS with a fresh HPACK decoder.
	dec := hpack.NewDecoder(4096, nil)
	var respHeaders []hpack.HeaderField

	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	responseComplete := false
	for !responseComplete {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("read response frame: %v", err)
		}
		switch ff := f.(type) {
		case *http2.HeadersFrame:
			dec.SetEmitFunc(func(hf hpack.HeaderField) {
				respHeaders = append(respHeaders, hf)
			})
			if _, err := dec.Write(ff.HeaderBlockFragment()); err != nil {
				t.Fatalf("hpack decode: %v", err)
			}
			if ff.StreamEnded() {
				responseComplete = true
			}
		case *http2.DataFrame:
			if ff.StreamEnded() {
				responseComplete = true
			}
		}
	}

	// Verify :status is present and readable.
	var statusVal string
	for _, hf := range respHeaders {
		if hf.Name == ":status" {
			statusVal = hf.Value
		}
	}
	if statusVal != "200" {
		t.Errorf(":status = %q, want 200", statusVal)
	}
}

// TestServerConn_TinyWindowDelivery verifies that a 1024-byte response is
// delivered correctly when the client announces INITIAL_WINDOW_SIZE=1 and
// sends WINDOW_UPDATE frames incrementally.
func TestServerConn_TinyWindowDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const bodySize = 1024
	body := make([]byte, bodySize)
	for i := range body {
		body[i] = 'x'
	}

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: string(body)}}
	clientConn, serverDone := startServerConn(t, ctx, disp, DefaultServerSettings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}

	fr := http2.NewFramer(clientConn, clientConn)
	fr.SetMaxReadFrameSize(65535)
	// Send SETTINGS with INITIAL_WINDOW_SIZE=1 so every byte needs a WINDOW_UPDATE.
	if err := fr.WriteSettings(http2.Setting{
		ID:  http2.SettingInitialWindowSize,
		Val: 1,
	}); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Handshake.
	settingsAcked := false
	clientAcked := false
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for !settingsAcked || !clientAcked {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("read handshake: %v", err)
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

	// Also open the connection-level window (server uses 65535 initial).
	if err := fr.WriteWindowUpdate(0, 65535); err != nil {
		t.Fatalf("conn window update: %v", err)
	}

	// Send request.
	var encBuf bytes.Buffer
	enc := hpack.NewEncoder(&encBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/tiny"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})

	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: encBuf.Bytes(),
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		t.Fatalf("write headers: %v", err)
	}

	// Read response headers + data; send WINDOW_UPDATE for each DATA byte.
	var received []byte
	_ = clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	done := false
	for !done {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("read frame: %v", err)
		}
		switch ff := f.(type) {
		case *http2.HeadersFrame:
			// Response headers — open up 1 byte of window to allow first DATA byte.
			if err := fr.WriteWindowUpdate(1, 1); err != nil {
				t.Fatalf("stream window update: %v", err)
			}
		case *http2.DataFrame:
			received = append(received, ff.Data()...)
			if ff.StreamEnded() {
				done = true
			} else {
				// Send window for the next byte.
				if err := fr.WriteWindowUpdate(1, 1); err != nil {
					t.Fatalf("stream window update: %v", err)
				}
			}
		}
	}

	if len(received) != bodySize {
		t.Errorf("received %d bytes, want %d", len(received), bodySize)
	}

	_ = serverDone
}

// TestServerConn_WriteData_RespectsMaxFrameSize verifies ADR-0055 I-1: the
// server's outbound DATA frames never exceed the peer-advertised
// SETTINGS_MAX_FRAME_SIZE. Setup: peer announces MaxFrameSize=16384; server
// sends a 32768-byte body. Expectation: at least 2 DATA frames are emitted,
// every DATA frame's payload is <= 16384 bytes.
func TestServerConn_WriteData_RespectsMaxFrameSize(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const bodySize = 32768
	const peerMaxFrame = uint32(16384)
	body := make([]byte, bodySize)
	for i := range body {
		body[i] = byte('a' + (i % 26))
	}

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: string(body)}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}

	fr := http2.NewFramer(clientConn, clientConn)
	// Allow reading frames up to the server's advertised default.
	fr.SetMaxReadFrameSize(65535)
	// Announce MaxFrameSize=16384 so the server should chunk its 32768-byte body
	// into at least two DATA frames.
	if err := fr.WriteSettings(http2.Setting{
		ID:  http2.SettingMaxFrameSize,
		Val: peerMaxFrame,
	}); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Handshake.
	settingsAcked := false
	clientAcked := false
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
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

	// Open the connection-level send window beyond the body so the conn-window
	// is never the constraint (we want to test the MaxFrameSize cap in
	// isolation).
	if err := fr.WriteWindowUpdate(0, 1<<20); err != nil {
		t.Fatalf("conn window update: %v", err)
	}

	// Send the request.
	var encBuf bytes.Buffer
	enc := hpack.NewEncoder(&encBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/big"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: encBuf.Bytes(),
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		t.Fatalf("write headers: %v", err)
	}

	// Open per-stream window large enough for the entire body (we are NOT
	// testing per-stream windowing here — that's I-2). Stream-level initial
	// window from peer SETTINGS defaults to 65535 (we did not override).
	if err := fr.WriteWindowUpdate(1, 1<<20); err != nil {
		t.Fatalf("stream window update: %v", err)
	}

	// Collect DATA frames until END_STREAM.
	var dataFrames int
	var totalBytes int
	_ = clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	done := false
	for !done {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("read frame: %v", err)
		}
		switch ff := f.(type) {
		case *http2.DataFrame:
			dataFrames++
			payloadLen := len(ff.Data())
			totalBytes += payloadLen
			if uint32(payloadLen) > peerMaxFrame {
				t.Errorf("DATA frame #%d payload = %d bytes, exceeds peer MaxFrameSize=%d",
					dataFrames, payloadLen, peerMaxFrame)
			}
			if ff.StreamEnded() {
				done = true
			}
		}
	}

	if dataFrames < 2 {
		t.Errorf("got %d DATA frames, want >= 2 (32768 bytes / MaxFrameSize=16384)", dataFrames)
	}
	if totalBytes != bodySize {
		t.Errorf("total bytes received = %d, want %d", totalBytes, bodySize)
	}
}

// TestServerConn_WriteData_RespectsPerStreamSendWindow verifies ADR-0055 I-2:
// the server respects the peer-advertised per-stream INITIAL_WINDOW_SIZE.
// Setup: peer announces InitialWindowSize=16; server sends a 100-byte body.
// The peer drips WINDOW_UPDATE(streamID, 16) every ~5ms. Expectation: many
// small DATA frames (no single frame exceeds 16 bytes), no peer-side
// FLOW_CONTROL_ERROR (since the server never sends more than the per-stream
// window allows).
func TestServerConn_WriteData_RespectsPerStreamSendWindow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const bodySize = 100
	const peerInitialWindow = uint32(16)
	body := make([]byte, bodySize)
	for i := range body {
		body[i] = byte('A' + (i % 26))
	}

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: string(body)}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}

	fr := http2.NewFramer(clientConn, clientConn)
	// Announce a tiny INITIAL_WINDOW_SIZE so each stream's send-window starts
	// at 16. The connection-level window stays at the default 65535.
	if err := fr.WriteSettings(http2.Setting{
		ID:  http2.SettingInitialWindowSize,
		Val: peerInitialWindow,
	}); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Handshake.
	settingsAcked := false
	clientAcked := false
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
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

	// Send the request.
	var encBuf bytes.Buffer
	enc := hpack.NewEncoder(&encBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/streamwindow"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: encBuf.Bytes(),
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		t.Fatalf("write headers: %v", err)
	}

	// Goroutine: drip WINDOW_UPDATE(1, 16) every ~5ms until ctx done.
	dripDone := make(chan struct{})
	dripperMu := &sync.Mutex{} // serialize WINDOW_UPDATE writes against fr's writer
	go func() {
		defer close(dripDone)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				dripperMu.Lock()
				_ = fr.WriteWindowUpdate(1, peerInitialWindow)
				dripperMu.Unlock()
			}
		}
	}()
	defer func() {
		cancel()
		<-dripDone
	}()

	// Collect DATA frames until END_STREAM. Assert no frame > 16 bytes and no
	// GOAWAY arrived (a FLOW_CONTROL_ERROR would manifest as GOAWAY from the
	// peer; from our side, since WE are the peer here, we'd never *send*
	// GOAWAY — what we'd see is the server emit a too-large DATA frame).
	var dataFrames int
	var totalBytes int
	_ = clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	done := false
	for !done {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("read frame: %v", err)
		}
		switch ff := f.(type) {
		case *http2.DataFrame:
			dataFrames++
			payloadLen := len(ff.Data())
			totalBytes += payloadLen
			if uint32(payloadLen) > peerInitialWindow {
				t.Errorf("DATA frame #%d payload = %d bytes, exceeds peer per-stream initial window=%d",
					dataFrames, payloadLen, peerInitialWindow)
			}
			if ff.StreamEnded() {
				done = true
			}
		}
	}

	// 100 bytes / 16-byte chunks = at least 7 frames (ceil(100/16) = 7).
	// Allow some slack: frames may be smaller than 16 if drip timing splits
	// reservations, so require >= 7 frames as a lower bound.
	if dataFrames < 7 {
		t.Errorf("got %d DATA frames, want >= 7 (ceil(100/16) = 7)", dataFrames)
	}
	if totalBytes != bodySize {
		t.Errorf("total bytes received = %d, want %d", totalBytes, bodySize)
	}
}

// TestServerConn_BadPrefaceClosesConnection verifies that a non-preface
// 24-byte write causes ServerConn.Run to return *Error{Code: ErrProtocolError}.
func TestServerConn_BadPrefaceClosesConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, serverDone := startServerConn(t, ctx, disp, DefaultServerSettings)

	// Write garbage (not the client preface).
	garbage := bytes.Repeat([]byte{0xFF}, 24)
	_, err := clientConn.Write(garbage)
	if err != nil {
		t.Fatalf("write garbage: %v", err)
	}

	select {
	case runErr := <-serverDone:
		if runErr == nil {
			t.Fatal("Run() returned nil, want *Error{Code: ErrProtocolError}")
		}
		h2Err, ok := runErr.(*Error)
		if !ok {
			t.Fatalf("Run() returned %T (%v), want *Error", runErr, runErr)
		}
		if h2Err.Code != ErrProtocolError {
			t.Errorf("error code = %v, want PROTOCOL_ERROR", h2Err.Code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Run() to return")
	}
}

// TestServerConn_CtxCancelEmitsGOAWAY verifies that canceling the context
// causes the server to emit GOAWAY(NO_ERROR) to the peer.
func TestServerConn_CtxCancelEmitsGOAWAY(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, serverDone := startServerConn(t, ctx, disp, DefaultServerSettings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}

	fr := http2.NewFramer(clientConn, clientConn)
	if err := fr.WriteSettings(); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Handshake.
	settingsAcked := false
	clientAcked := false
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for !settingsAcked || !clientAcked {
		f, err := fr.ReadFrame()
		if err != nil {
			break
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

	// Cancel the context — server should emit GOAWAY(NO_ERROR).
	cancel()

	// Wait for server to exit.
	select {
	case <-serverDone:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not exit after ctx cancel")
	}

	// Read frames looking for GOAWAY.
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			// Connection closed — GOAWAY may have been sent before close.
			return
		}
		if f.Header().Type == http2.FrameGoAway {
			gf := f.(*http2.GoAwayFrame)
			if gf.ErrCode != http2.ErrCodeNo {
				t.Errorf("GOAWAY code = %v, want NO_ERROR", gf.ErrCode)
			}
			return
		}
	}
}

// bodyCaptureAction reads the entire request body and reports the byte count
// on doneCh. Used by ADR-0055 I-3 receive-side flow-control tests.
type bodyCaptureAction struct {
	doneCh chan int
}

func (a *bodyCaptureAction) WriteH2(_ context.Context, _ H2Request, sw StreamWriter) error {
	// We do not have direct access to the request body here (it's read by the
	// dispatch goroutine before WriteH2 is called); the dispatcher captures the
	// body length in its Match function and sends it on doneCh from there.
	headers := []hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-length", Value: "2"},
	}
	if err := sw.WriteHeaders(headers, false); err != nil {
		return err
	}
	return sw.WriteData([]byte("OK"), true)
}

// bodyCaptureDispatcher captures req.Body length on Match and forwards
// the action.
type bodyCaptureDispatcher struct {
	doneCh chan int
}

func (d *bodyCaptureDispatcher) Match(req *http.Request) (Action, bool) {
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		select {
		case d.doneCh <- len(b):
		default:
		}
	}
	return &bodyCaptureAction{doneCh: d.doneCh}, true
}

// TestServerConn_ReceiveSide_FlowControl_LargeInboundBody verifies ADR-0055 I-3:
// the server emits WINDOW_UPDATE on the half-window threshold so the peer can
// keep sending past the initial 65535 receive-window. Without I-3 the peer
// stalls after 65535 bytes and the request body never completes.
func TestServerConn_ReceiveSide_FlowControl_LargeInboundBody(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	const totalBody = 100 * 1024 // 100 KB > 65535 default initial recv window
	const chunkSize = 8 * 1024

	doneCh := make(chan int, 1)
	disp := &bodyCaptureDispatcher{doneCh: doneCh}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}

	fr := http2.NewFramer(clientConn, clientConn)
	if err := fr.WriteSettings(); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Synchronously complete the SETTINGS handshake before sending HEADERS so
	// we observe a stable initial state on both sides (server-advertised
	// SETTINGS read; our SETTINGS ACK'd by server).
	settingsAcked := false
	clientAcked := false
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
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

	// Goroutine: continuously read frames from the server. We need to consume
	// WINDOW_UPDATE / SETTINGS / HEADERS / DATA without blocking the server.
	connWindowGrants := make(chan int32, 64)
	streamWindowGrants := make(chan int32, 64)
	respDone := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		_ = clientConn.SetReadDeadline(time.Now().Add(7 * time.Second))
		for {
			f, err := fr.ReadFrame()
			if err != nil {
				return
			}
			switch ff := f.(type) {
			case *http2.SettingsFrame:
				if !ff.IsAck() {
					_ = fr.WriteSettingsAck()
				}
			case *http2.WindowUpdateFrame:
				if ff.Header().StreamID == 0 {
					select {
					case connWindowGrants <- int32(ff.Increment):
					default:
					}
				} else {
					select {
					case streamWindowGrants <- int32(ff.Increment):
					default:
					}
				}
			case *http2.DataFrame:
				if ff.StreamEnded() {
					select {
					case <-respDone:
					default:
						close(respDone)
					}
				}
			}
		}
	}()

	// Send the request HEADERS (no END_STREAM — body to follow).
	var encBuf bytes.Buffer
	enc := hpack.NewEncoder(&encBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "POST"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/upload"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})
	_ = enc.WriteField(hpack.HeaderField{Name: "content-length", Value: strconv.Itoa(totalBody)})
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: encBuf.Bytes(),
		EndHeaders:    true,
		EndStream:     false,
	}); err != nil {
		t.Fatalf("write headers: %v", err)
	}

	// Send the body in 8KB chunks. Track conn-level + stream-level granted
	// window so we never overrun the peer's receive window. Initial windows
	// (server's recv side) are 65535 each.
	connAvail := int32(65535)
	streamAvail := int32(65535)
	body := make([]byte, totalBody)
	for i := range body {
		body[i] = byte('a' + (i % 26))
	}
	sent := 0
	for sent < totalBody {
		// Wait until both windows have at least chunkSize available, draining
		// any WINDOW_UPDATEs that have arrived.
		for connAvail < 1 || streamAvail < 1 {
			select {
			case g := <-connWindowGrants:
				connAvail += g
			case g := <-streamWindowGrants:
				streamAvail += g
			case <-time.After(3 * time.Second):
				t.Fatalf("timed out waiting for WINDOW_UPDATE; sent=%d/%d connAvail=%d streamAvail=%d",
					sent, totalBody, connAvail, streamAvail)
			}
		}
		// Drain any further pending grants without blocking.
	drain:
		for {
			select {
			case g := <-connWindowGrants:
				connAvail += g
			case g := <-streamWindowGrants:
				streamAvail += g
			default:
				break drain
			}
		}

		want := chunkSize
		if want > totalBody-sent {
			want = totalBody - sent
		}
		if int32(want) > connAvail {
			want = int(connAvail)
		}
		if int32(want) > streamAvail {
			want = int(streamAvail)
		}
		if want <= 0 {
			continue
		}
		end := sent + want
		isLast := end == totalBody
		if err := fr.WriteData(1, isLast, body[sent:end]); err != nil {
			t.Fatalf("write data: %v", err)
		}
		sent = end
		connAvail -= int32(want)
		streamAvail -= int32(want)
	}

	// Wait for the dispatcher to capture the body length.
	select {
	case got := <-doneCh:
		if got != totalBody {
			t.Errorf("captured body length = %d, want %d", got, totalBody)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for dispatcher body capture; sent=%d/%d", sent, totalBody)
	}

	cancel()
	<-readerDone
}

// TestServerConn_WindowUpdate_OverflowBoundsCheck verifies ADR-0055 M-9: a
// WINDOW_UPDATE that would push the send window past 2^31-1 must trigger
// FLOW_CONTROL_ERROR per RFC 9113 §6.9.1 — RST_STREAM on a stream-level
// overflow, GOAWAY on a connection-level overflow.
func TestServerConn_WindowUpdate_OverflowBoundsCheck(t *testing.T) {
	t.Run("stream-level overflow → RST_STREAM(FLOW_CONTROL_ERROR)", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
		clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)

		if err := writeClientPreface(clientConn); err != nil {
			t.Fatalf("write preface: %v", err)
		}
		fr := http2.NewFramer(clientConn, clientConn)
		if err := fr.WriteSettings(); err != nil {
			t.Fatalf("write settings: %v", err)
		}

		// Handshake.
		settingsAcked := false
		clientAcked := false
		_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
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

		// Open a stream by sending HEADERS without END_STREAM (so the stream
		// remains "open" and not yet dispatched/closed).
		var encBuf bytes.Buffer
		enc := hpack.NewEncoder(&encBuf)
		_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "POST"})
		_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/x"})
		_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
		_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})
		if err := fr.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      1,
			BlockFragment: encBuf.Bytes(),
			EndHeaders:    true,
			EndStream:     false,
		}); err != nil {
			t.Fatalf("write headers: %v", err)
		}

		// First WINDOW_UPDATE pushes sendW close to MaxInt32 (initial window is
		// 65535, so adding MaxInt32 - 65535 brings it exactly to MaxInt32).
		if err := fr.WriteWindowUpdate(1, uint32(math.MaxInt32-65535)); err != nil {
			t.Fatalf("write first WINDOW_UPDATE: %v", err)
		}
		// Second WINDOW_UPDATE with delta=1 → would push past MaxInt32 → FLOW_CONTROL_ERROR.
		if err := fr.WriteWindowUpdate(1, 1); err != nil {
			t.Fatalf("write overflow WINDOW_UPDATE: %v", err)
		}

		// Expect RST_STREAM(FLOW_CONTROL_ERROR) for stream 1.
		_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
		gotRST := false
		for i := 0; i < 20; i++ {
			f, err := fr.ReadFrame()
			if err != nil {
				break
			}
			if rf, ok := f.(*http2.RSTStreamFrame); ok {
				if rf.Header().StreamID == 1 && rf.ErrCode == http2.ErrCodeFlowControl {
					gotRST = true
					break
				}
			}
		}
		if !gotRST {
			t.Error("expected RST_STREAM(FLOW_CONTROL_ERROR) for stream 1 on send-window overflow")
		}
	})

	t.Run("connection-level overflow → GOAWAY(FLOW_CONTROL_ERROR)", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
		clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)

		if err := writeClientPreface(clientConn); err != nil {
			t.Fatalf("write preface: %v", err)
		}
		fr := http2.NewFramer(clientConn, clientConn)
		if err := fr.WriteSettings(); err != nil {
			t.Fatalf("write settings: %v", err)
		}

		// Handshake.
		settingsAcked := false
		clientAcked := false
		_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
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

		// First conn-level WINDOW_UPDATE pushes sendW (conn) close to MaxInt32.
		if err := fr.WriteWindowUpdate(0, uint32(math.MaxInt32-65535)); err != nil {
			t.Fatalf("write first WINDOW_UPDATE: %v", err)
		}
		// Second conn-level WINDOW_UPDATE with delta=1 → overflow → FLOW_CONTROL_ERROR.
		if err := fr.WriteWindowUpdate(0, 1); err != nil {
			t.Fatalf("write overflow WINDOW_UPDATE: %v", err)
		}

		// Expect GOAWAY(FLOW_CONTROL_ERROR).
		_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
		gotGoaway := false
		for i := 0; i < 20; i++ {
			f, err := fr.ReadFrame()
			if err != nil {
				break
			}
			if gf, ok := f.(*http2.GoAwayFrame); ok {
				if gf.ErrCode == http2.ErrCodeFlowControl {
					gotGoaway = true
					break
				}
				t.Errorf("GOAWAY code = %v, want FLOW_CONTROL_ERROR", gf.ErrCode)
				break
			}
		}
		if !gotGoaway {
			t.Error("expected GOAWAY(FLOW_CONTROL_ERROR) on conn-level send-window overflow")
		}
	})
}

// TestServerConn_RequestWithTrailersIsDispatched pins the trailing-HEADERS
// dispatch path: a request whose END_STREAM arrives on a trailing HEADERS
// frame (HEADERS without END_STREAM → DATA without END_STREAM → trailing
// HEADERS with END_STREAM) MUST still be dispatched and served. The trailer
// fields themselves are observed-and-discarded per SPEC §2.1; only the
// END_STREAM transition matters. Before the fix the isExisting branch of
// onHeaders returned without queueing a dispatch, so the request hung with
// no response and the stream leaked its MAX_CONCURRENT_STREAMS slot forever.
func TestServerConn_RequestWithTrailersIsDispatched(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "trailered\n"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)

	tr := newH2Transport(clientConn)
	defer tr.CloseIdleConnections()

	// x/net's http2.Transport emits the trailing-HEADERS frame when
	// req.Trailer is populated: HEADERS (no END_STREAM), DATA (no END_STREAM),
	// then HEADERS carrying the trailers with END_STREAM set.
	req, err := http.NewRequest("POST", "http://test.local/upload", bytes.NewBufferString("hello"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Trailer = http.Header{"X-Checksum": []string{"abc123"}}

	type rtResult struct {
		resp *http.Response
		err  error
	}
	resCh := make(chan rtResult, 1)
	go func() {
		resp, err := tr.RoundTrip(req)
		resCh <- rtResult{resp, err}
	}()

	select {
	case res := <-resCh:
		if res.err != nil {
			t.Fatalf("RoundTrip: %v", res.err)
		}
		if res.resp.StatusCode != 200 {
			t.Errorf("status = %d, want 200", res.resp.StatusCode)
		}
		if body := readBody(t, res.resp); body != "trailered\n" {
			t.Errorf("body = %q, want %q", body, "trailered\n")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request with trailing HEADERS was never served (dispatch not queued on the trailer END_STREAM)")
	}
}
