package h2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// runFakeServerPeerForClientHandshake reads the client preface + client SETTINGS,
// writes the server's initial SETTINGS, reads the client's SETTINGS_ACK,
// writes its own SETTINGS_ACK, then returns. Returns nil on the happy path.
//
// The bidirectional ACK ordering (read client's ACK before writing our own)
// avoids a synchronous-pipe deadlock where both ends would block on
// simultaneous writes; RFC 9113 §6.5 imposes no ordering between the two
// independent ACKs.
func runFakeServerPeerForClientHandshake(conn net.Conn, _ time.Duration) error {
	prefaceBuf := make([]byte, 24)
	if _, err := io.ReadFull(conn, prefaceBuf); err != nil {
		return fmt.Errorf("preface: %w", err)
	}
	if string(prefaceBuf) != string(clientPrefaceBytes) {
		return fmt.Errorf("bad preface: %q", prefaceBuf)
	}
	fr := http2.NewFramer(conn, conn)
	// Read client SETTINGS.
	frame, err := fr.ReadFrame()
	if err != nil {
		return fmt.Errorf("read client SETTINGS: %w", err)
	}
	if _, ok := frame.(*http2.SettingsFrame); !ok {
		return fmt.Errorf("expected SETTINGS, got %T", frame)
	}
	// Write server SETTINGS.
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16384}); err != nil {
		return fmt.Errorf("write server SETTINGS: %w", err)
	}
	// Read client's SETTINGS_ACK for our SETTINGS BEFORE writing our own
	// SETTINGS_ACK. RFC 9113 §6.5 imposes no ordering between the two ACKs;
	// reading first avoids both peers blocking on synchronous net.Pipe writes
	// at the same time. Real TCP survives the bidirectional simultaneity via
	// socket-buffer slack; net.Pipe has none.
	if _, err := fr.ReadFrame(); err != nil {
		return fmt.Errorf("read client SETTINGS_ACK: %w", err)
	}
	// Write SETTINGS_ACK for client's SETTINGS — the client's readLoop is
	// spawned by now and will consume it, completing the synchronous wait
	// inside NewClientConn.
	if err := fr.WriteSettingsAck(); err != nil {
		return fmt.Errorf("write SETTINGS_ACK: %w", err)
	}
	return nil
}

// TestNewClientConn_PrefaceAndSettingsExchange verifies NewClientConn:
//  1. Writes the 24-byte client preface
//  2. Writes initial SETTINGS
//  3. Reads the server's initial SETTINGS
//  4. Writes SETTINGS_ACK in response
//  5. Reads the server's SETTINGS_ACK for our SETTINGS
//
// Returns a ready-to-RoundTrip *ClientConn.
func TestNewClientConn_PrefaceAndSettingsExchange(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()
	defer func() { _ = serverSide.Close() }()

	done := make(chan error, 1)
	go func() {
		done <- runFakeServerPeerForClientHandshake(serverSide, 0)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cc, err := NewClientConn(ctx, clientSide)
	if err != nil {
		t.Fatalf("NewClientConn: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("fake peer: %v", err)
	}
	// Spawn a drainer on serverSide so Close's GOAWAY write doesn't block on
	// the synchronous net.Pipe (no buffering, no reader = deadlock).
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_, _ = io.Copy(io.Discard, serverSide)
	}()
	if err := cc.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Logf("Close: %v (acceptable on net.Pipe)", err)
	}
	<-drained
}

// TestClientConn_Close_EmitsGracefulGoaway verifies Close emits a GOAWAY
// with NO_ERROR before closing the underlying conn.
func TestClientConn_Close_EmitsGracefulGoaway(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer func() { _ = serverSide.Close() }()

	handshakeDone := make(chan error, 1)
	go func() {
		handshakeDone <- runFakeServerPeerForClientHandshake(serverSide, 0)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cc, err := NewClientConn(ctx, clientSide)
	if err != nil {
		t.Fatalf("NewClientConn: %v", err)
	}
	if err := <-handshakeDone; err != nil {
		t.Fatalf("fake peer: %v", err)
	}

	// Reader goroutine drains post-handshake frames; the only one we expect
	// is the GOAWAY emitted by Close().
	gotGoaway := make(chan *http2.GoAwayFrame, 1)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		fr := http2.NewFramer(serverSide, serverSide)
		for {
			f, err := fr.ReadFrame()
			if err != nil {
				return
			}
			if g, ok := f.(*http2.GoAwayFrame); ok {
				gotGoaway <- g
				return
			}
		}
	}()

	// Trigger Close: should emit GOAWAY then close the underlying conn.
	if err := cc.Close(); err != nil {
		t.Logf("Close: %v (acceptable on net.Pipe)", err)
	}

	select {
	case g := <-gotGoaway:
		if http2.ErrCode(g.ErrCode) != http2.ErrCodeNo {
			t.Fatalf("GOAWAY ErrCode = %v, want NO_ERROR", g.ErrCode)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not observe GOAWAY frame on peer side within 3s")
	}
	<-readDone
}

// TestNewClientConn_SettingsHandshakeFailureBubblesUp verifies that a peer
// sending malformed SETTINGS (ACK bit set on first frame, forbidden per
// RFC 9113 §6.5) causes NewClientConn to return a *Error.
func TestNewClientConn_SettingsHandshakeFailureBubblesUp(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()
	defer func() { _ = serverSide.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Read client preface.
		prefaceBuf := make([]byte, 24)
		if _, err := io.ReadFull(serverSide, prefaceBuf); err != nil {
			return
		}
		fr := http2.NewFramer(serverSide, serverSide)
		// Read client's initial SETTINGS (so client doesn't block on Write).
		if _, err := fr.ReadFrame(); err != nil {
			return
		}
		// Send SETTINGS_ACK as our first frame — RFC 9113 §6.5 violation.
		_ = fr.WriteSettingsAck()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cc, err := NewClientConn(ctx, clientSide)
	if err == nil {
		_ = cc.Close()
		t.Fatal("NewClientConn returned nil; want *Error")
	}
	var herr *Error
	if !errors.As(err, &herr) {
		t.Fatalf("err = %v (%T); want *Error", err, err)
	}
	if herr.Code != ErrProtocolError {
		t.Fatalf("err code = %v, want PROTOCOL_ERROR", herr.Code)
	}
	<-done
}

// fakeH2ServerPeer wraps an http2.Framer-driven peer for the RoundTrip test
// suite. It performs the handshake on Start, then exposes hooks for reading
// the client's request and writing arbitrary response framing.
type fakeH2ServerPeer struct {
	conn      net.Conn
	fr        *http2.Framer
	hpackEnc  *hpack.Encoder
	encBuf    bytes.Buffer
	dec       *hpack.Decoder
	decFields []hpack.HeaderField
}

// newFakeH2ServerPeer wraps the server side of a net.Pipe. Caller is
// responsible for closing the underlying conn.
func newFakeH2ServerPeer(t *testing.T, conn net.Conn) *fakeH2ServerPeer {
	t.Helper()
	p := &fakeH2ServerPeer{conn: conn}
	p.hpackEnc = hpack.NewEncoder(&p.encBuf)
	p.dec = hpack.NewDecoder(4096, func(f hpack.HeaderField) {
		p.decFields = append(p.decFields, f)
	})
	return p
}

// runHandshake reads preface + client SETTINGS, writes server SETTINGS,
// reads client SETTINGS_ACK, writes server SETTINGS_ACK. After this returns
// p.fr is initialized and ready for further frame I/O.
func (p *fakeH2ServerPeer) runHandshake() error {
	prefaceBuf := make([]byte, 24)
	if _, err := io.ReadFull(p.conn, prefaceBuf); err != nil {
		return fmt.Errorf("preface: %w", err)
	}
	if string(prefaceBuf) != string(clientPrefaceBytes) {
		return fmt.Errorf("bad preface: %q", prefaceBuf)
	}
	p.fr = http2.NewFramer(p.conn, p.conn)
	frame, err := p.fr.ReadFrame()
	if err != nil {
		return fmt.Errorf("read client SETTINGS: %w", err)
	}
	if _, ok := frame.(*http2.SettingsFrame); !ok {
		return fmt.Errorf("expected SETTINGS, got %T", frame)
	}
	if err := p.fr.WriteSettings(http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16384}); err != nil {
		return fmt.Errorf("write server SETTINGS: %w", err)
	}
	if _, err := p.fr.ReadFrame(); err != nil {
		return fmt.Errorf("read client SETTINGS_ACK: %w", err)
	}
	if err := p.fr.WriteSettingsAck(); err != nil {
		return fmt.Errorf("write SETTINGS_ACK: %w", err)
	}
	return nil
}

// readRequestHeaders reads frames from the client until a HEADERS frame is
// returned (intermediate WINDOW_UPDATE/PING/SETTINGS-ACK frames are
// tolerated). Returns the HEADERS frame and the decoded fields.
func (p *fakeH2ServerPeer) readRequestHeaders() (*http2.HeadersFrame, []hpack.HeaderField, error) {
	for {
		f, err := p.fr.ReadFrame()
		if err != nil {
			return nil, nil, err
		}
		switch fr := f.(type) {
		case *http2.HeadersFrame:
			fields, derr := p.decodeBlock(fr.HeaderBlockFragment())
			if derr != nil {
				return nil, nil, derr
			}
			return fr, fields, nil
		case *http2.WindowUpdateFrame, *http2.SettingsFrame, *http2.PingFrame:
			// keep reading
		default:
			// keep reading; tests may handle intermediate frames externally
		}
	}
}

// readDataFrame reads frames from the client until a DATA frame is returned.
func (p *fakeH2ServerPeer) readDataFrame() (*http2.DataFrame, error) {
	for {
		f, err := p.fr.ReadFrame()
		if err != nil {
			return nil, err
		}
		switch fr := f.(type) {
		case *http2.DataFrame:
			return fr, nil
		case *http2.WindowUpdateFrame, *http2.SettingsFrame, *http2.PingFrame:
			// keep reading
		default:
			// keep reading
		}
	}
}

// readNextFrame returns the next frame ignoring nothing.
func (p *fakeH2ServerPeer) readNextFrame() (http2.Frame, error) {
	return p.fr.ReadFrame()
}

func (p *fakeH2ServerPeer) decodeBlock(b []byte) ([]hpack.HeaderField, error) {
	p.decFields = p.decFields[:0]
	if _, err := p.dec.Write(b); err != nil {
		return nil, err
	}
	if err := p.dec.Close(); err != nil {
		return nil, err
	}
	out := make([]hpack.HeaderField, len(p.decFields))
	copy(out, p.decFields)
	return out, nil
}

func (p *fakeH2ServerPeer) encodeHeaders(headers []hpack.HeaderField) []byte {
	p.encBuf.Reset()
	for _, h := range headers {
		_ = p.hpackEnc.WriteField(h)
	}
	out := make([]byte, p.encBuf.Len())
	copy(out, p.encBuf.Bytes())
	return out
}

// writeResponse writes a HEADERS frame with :status + supplied headers,
// optionally followed by a DATA frame; END_STREAM is set on the last frame.
func (p *fakeH2ServerPeer) writeResponse(streamID uint32, status int, extra []hpack.HeaderField, body []byte) error {
	headers := []hpack.HeaderField{{Name: ":status", Value: strconv.Itoa(status)}}
	headers = append(headers, extra...)
	block := p.encodeHeaders(headers)
	endStream := len(body) == 0
	if err := p.fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block,
		EndStream:     endStream,
		EndHeaders:    true,
	}); err != nil {
		return err
	}
	if !endStream {
		if err := p.fr.WriteData(streamID, true, body); err != nil {
			return err
		}
	}
	return nil
}

// runHandshakeAsync starts the handshake on a goroutine; the returned
// channel emits any error encountered. Tests should range/select on the
// channel as part of their teardown.
func (p *fakeH2ServerPeer) runHandshakeAsync() <-chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- p.runHandshake()
	}()
	return ch
}

// dialClientConn brings up a net.Pipe pair, drives the handshake from the
// peer, and returns a ready *ClientConn + the peer for response scripting.
// Caller defers cleanup() to close everything.
func dialClientConn(t *testing.T) (*ClientConn, *fakeH2ServerPeer, func()) {
	t.Helper()
	clientSide, serverSide := net.Pipe()
	peer := newFakeH2ServerPeer(t, serverSide)
	hsErr := peer.runHandshakeAsync()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	cc, err := NewClientConn(ctx, clientSide)
	if err != nil {
		cancel()
		_ = clientSide.Close()
		_ = serverSide.Close()
		t.Fatalf("NewClientConn: %v", err)
	}
	if err := <-hsErr; err != nil {
		cancel()
		_ = cc.Close()
		_ = serverSide.Close()
		t.Fatalf("peer handshake: %v", err)
	}
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			cancel()
			// Drain peer side so any GOAWAY/RST_STREAM writes from Close
			// don't deadlock on the synchronous net.Pipe.
			drained := make(chan struct{})
			go func() {
				defer close(drained)
				_, _ = io.Copy(io.Discard, serverSide)
			}()
			_ = cc.Close()
			_ = serverSide.Close()
			<-drained
		})
	}
	return cc, peer, cleanup
}

// TestClientConn_RoundTrip_HappyPath_BodylessGET — bodyless GET; peer
// responds HEADERS+DATA+END_STREAM; assert the assembled H2Response.
func TestClientConn_RoundTrip_HappyPath_BodylessGET(t *testing.T) {
	cc, peer, cleanup := dialClientConn(t)
	defer cleanup()

	peerDone := make(chan error, 1)
	go func() {
		hf, _, err := peer.readRequestHeaders()
		if err != nil {
			peerDone <- fmt.Errorf("readRequestHeaders: %w", err)
			return
		}
		if !hf.StreamEnded() {
			peerDone <- fmt.Errorf("expected END_STREAM on bodyless GET HEADERS")
			return
		}
		peerDone <- peer.writeResponse(hf.StreamID, 200,
			[]hpack.HeaderField{{Name: "content-type", Value: "text/plain"}},
			[]byte("hello"),
		)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := cc.RoundTrip(ctx, H2Request{
		Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
	})
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if err := <-peerDone; err != nil {
		t.Fatalf("peer: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("Status = %d, want 200", resp.Status)
	}
	if string(resp.Body) != "hello" {
		t.Errorf("Body = %q, want %q", resp.Body, "hello")
	}
	var sawStatus, sawCT bool
	for _, hf := range resp.Headers {
		if hf.Name == ":status" && hf.Value == "200" {
			sawStatus = true
		}
		if hf.Name == "content-type" && hf.Value == "text/plain" {
			sawCT = true
		}
	}
	if !sawStatus || !sawCT {
		t.Errorf("Headers missing :status or content-type: %+v", resp.Headers)
	}
}

// TestClientConn_RoundTrip_HappyPath_WithBody — POST with body; peer reads
// HEADERS+DATA, responds HEADERS+DATA+END_STREAM.
func TestClientConn_RoundTrip_HappyPath_WithBody(t *testing.T) {
	cc, peer, cleanup := dialClientConn(t)
	defer cleanup()

	peerDone := make(chan error, 1)
	go func() {
		hf, _, err := peer.readRequestHeaders()
		if err != nil {
			peerDone <- fmt.Errorf("readRequestHeaders: %w", err)
			return
		}
		if hf.StreamEnded() {
			peerDone <- fmt.Errorf("HEADERS unexpectedly END_STREAM on POST with body")
			return
		}
		df, err := peer.readDataFrame()
		if err != nil {
			peerDone <- fmt.Errorf("readDataFrame: %w", err)
			return
		}
		if !df.StreamEnded() {
			peerDone <- fmt.Errorf("expected END_STREAM on body DATA frame")
			return
		}
		if string(df.Data()) != "request-body" {
			peerDone <- fmt.Errorf("body = %q, want %q", df.Data(), "request-body")
			return
		}
		peerDone <- peer.writeResponse(hf.StreamID, 201, nil, []byte("created"))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := cc.RoundTrip(ctx, H2Request{
		Method: "POST", Path: "/x", Scheme: "https", Authority: "example.test",
		Body: []byte("request-body"),
	})
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if err := <-peerDone; err != nil {
		t.Fatalf("peer: %v", err)
	}
	if resp.Status != 201 {
		t.Errorf("Status = %d, want 201", resp.Status)
	}
	if string(resp.Body) != "created" {
		t.Errorf("Body = %q, want %q", resp.Body, "created")
	}
}

// TestClientConn_RoundTrip_CtxCancelDuringWrite — cancel ctx after the peer
// has read HEADERS but before it responds; assert RoundTrip returns
// ctx.Err() AND that a RST_STREAM(CANCEL) appears on the wire.
func TestClientConn_RoundTrip_CtxCancelDuringWrite(t *testing.T) {
	cc, peer, cleanup := dialClientConn(t)
	defer cleanup()

	gotRST := make(chan *http2.RSTStreamFrame, 1)
	peerDone := make(chan error, 1)
	cancelTrigger := make(chan struct{})
	go func() {
		hf, _, err := peer.readRequestHeaders()
		if err != nil {
			peerDone <- err
			return
		}
		// Signal to the test that HEADERS was read, then keep reading until
		// we see RST_STREAM. We deliberately do NOT respond.
		_ = hf
		close(cancelTrigger)
		for {
			f, err := peer.readNextFrame()
			if err != nil {
				peerDone <- nil
				return
			}
			if rst, ok := f.(*http2.RSTStreamFrame); ok {
				gotRST <- rst
				peerDone <- nil
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-cancelTrigger
		cancel()
	}()
	_, err := cc.RoundTrip(ctx, H2Request{
		Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
	})
	if err == nil {
		t.Fatal("RoundTrip returned nil; want ctx.Err()")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RoundTrip err = %v, want context.Canceled", err)
	}

	select {
	case rst := <-gotRST:
		if rst.ErrCode != http2.ErrCodeCancel {
			t.Errorf("RST_STREAM code = %v, want CANCEL", rst.ErrCode)
		}
	case <-time.After(2 * time.Second):
		t.Error("did not observe RST_STREAM on the wire within 2s")
	}
	<-peerDone
}

// TestClientConn_RoundTrip_CtxCancelDuringRead — peer sends HEADERS but no
// DATA; ctx cancels mid-read; assert RoundTrip returns ctx.Err() and emits
// RST_STREAM(CANCEL).
func TestClientConn_RoundTrip_CtxCancelDuringRead(t *testing.T) {
	cc, peer, cleanup := dialClientConn(t)
	defer cleanup()

	gotRST := make(chan *http2.RSTStreamFrame, 1)
	peerDone := make(chan error, 1)
	headersWritten := make(chan struct{})
	go func() {
		hf, _, err := peer.readRequestHeaders()
		if err != nil {
			peerDone <- err
			return
		}
		// Write only the HEADERS (no END_STREAM, no DATA) so RoundTrip is
		// blocked waiting for body bytes.
		headers := []hpack.HeaderField{{Name: ":status", Value: "200"}}
		block := peer.encodeHeaders(headers)
		if err := peer.fr.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      hf.StreamID,
			BlockFragment: block,
			EndStream:     false,
			EndHeaders:    true,
		}); err != nil {
			peerDone <- err
			return
		}
		close(headersWritten)
		for {
			f, err := peer.readNextFrame()
			if err != nil {
				peerDone <- nil
				return
			}
			if rst, ok := f.(*http2.RSTStreamFrame); ok {
				gotRST <- rst
				peerDone <- nil
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-headersWritten
		// Brief pause so the client's readLoop processes HEADERS before we
		// cancel; we want to be in the "waiting for DATA" state.
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := cc.RoundTrip(ctx, H2Request{
		Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
	})
	if err == nil {
		t.Fatal("RoundTrip returned nil; want ctx.Err()")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RoundTrip err = %v, want context.Canceled", err)
	}
	select {
	case rst := <-gotRST:
		if rst.ErrCode != http2.ErrCodeCancel {
			t.Errorf("RST_STREAM code = %v, want CANCEL", rst.ErrCode)
		}
	case <-time.After(2 * time.Second):
		t.Error("did not observe RST_STREAM on the wire within 2s")
	}
	<-peerDone
}

// TestClientConn_RoundTrip_PeerRSTStream — peer sends RST_STREAM(CANCEL)
// instead of a normal response; assert *Error{Code: CANCEL}.
func TestClientConn_RoundTrip_PeerRSTStream(t *testing.T) {
	cc, peer, cleanup := dialClientConn(t)
	defer cleanup()

	peerDone := make(chan error, 1)
	go func() {
		hf, _, err := peer.readRequestHeaders()
		if err != nil {
			peerDone <- err
			return
		}
		peerDone <- peer.fr.WriteRSTStream(hf.StreamID, http2.ErrCodeCancel)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := cc.RoundTrip(ctx, H2Request{
		Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
	})
	if err == nil {
		t.Fatal("RoundTrip returned nil; want *Error{Code: CANCEL}")
	}
	var herr *Error
	if !errors.As(err, &herr) {
		t.Fatalf("err = %v (%T); want *Error", err, err)
	}
	if herr.Code != ErrCancel {
		t.Errorf("err code = %v, want CANCEL", herr.Code)
	}
	if err := <-peerDone; err != nil {
		t.Fatalf("peer: %v", err)
	}
}

// TestClientConn_RoundTrip_PeerGoaway — peer sends GOAWAY(NO_ERROR) with
// last-stream-id = 0 just after reading the request HEADERS; our stream id
// (1) > 0, so the stream is finished with the GOAWAY error code.
func TestClientConn_RoundTrip_PeerGoaway(t *testing.T) {
	cc, peer, cleanup := dialClientConn(t)
	defer cleanup()

	peerDone := make(chan error, 1)
	go func() {
		_, _, err := peer.readRequestHeaders()
		if err != nil {
			peerDone <- err
			return
		}
		peerDone <- peer.fr.WriteGoAway(0, http2.ErrCodeNo, []byte("graceful"))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := cc.RoundTrip(ctx, H2Request{
		Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
	})
	if err == nil {
		t.Fatal("RoundTrip returned nil; want *Error from peer GOAWAY")
	}
	var herr *Error
	if !errors.As(err, &herr) {
		// On peer GOAWAY with LastStreamID=0 the stream is finished with a
		// stream-scoped error. Tolerate either *Error wrap or context-canceled
		// fallback if cc.ctx happens to die first.
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v (%T); want *Error or context.Canceled", err, err)
		}
	}
	if err := <-peerDone; err != nil {
		t.Fatalf("peer: %v", err)
	}
}

// TestClientConn_RoundTrip_PeerDataAfterEndStream — peer sends an extra DATA
// frame AFTER it already sent DATA+END_STREAM. The client's RoundTrip has
// already returned with the (legitimate) response; the second DATA frame
// must invalidate the connection (cc.ctx is canceled by the readLoop).
func TestClientConn_RoundTrip_PeerDataAfterEndStream(t *testing.T) {
	cc, peer, cleanup := dialClientConn(t)
	defer cleanup()

	peerDone := make(chan error, 1)
	go func() {
		hf, _, err := peer.readRequestHeaders()
		if err != nil {
			peerDone <- err
			return
		}
		// Write HEADERS (no END_STREAM) + DATA(END_STREAM) + extra DATA.
		headers := []hpack.HeaderField{{Name: ":status", Value: "200"}}
		block := peer.encodeHeaders(headers)
		if err := peer.fr.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      hf.StreamID,
			BlockFragment: block,
			EndStream:     false,
			EndHeaders:    true,
		}); err != nil {
			peerDone <- err
			return
		}
		if err := peer.fr.WriteData(hf.StreamID, true, []byte("ok")); err != nil {
			peerDone <- err
			return
		}
		// Sleep briefly so the client's RoundTrip returns and the stream is
		// removed from cc.streams before the rogue frame arrives.
		time.Sleep(100 * time.Millisecond)
		_ = peer.fr.WriteData(hf.StreamID, false, []byte("rogue"))
		peerDone <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := cc.RoundTrip(ctx, H2Request{
		Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
	})
	if err != nil {
		t.Fatalf("first RoundTrip: %v", err)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("first response Body = %q, want %q", resp.Body, "ok")
	}
	if err := <-peerDone; err != nil {
		t.Fatalf("peer: %v", err)
	}
	// After the rogue DATA frame the readLoop should cancel cc.ctx. Wait up
	// to 2s for the cancellation to propagate.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cc.ctx.Err() != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if cc.ctx.Err() == nil {
		t.Fatal("expected cc.ctx to be canceled after rogue DATA frame")
	}
}

// TestClientConn_RoundTrip_AfterClose — Close, then RoundTrip; assert error.
func TestClientConn_RoundTrip_AfterClose(t *testing.T) {
	cc, _, cleanup := dialClientConn(t)
	defer cleanup()

	// cleanup() drains the peer side and Closes; calling RoundTrip after
	// that returns the conn-closed error per the cc.ctx.Err() upfront check.
	// We invoke cleanup eagerly here so that subsequent RoundTrip is on a
	// closed conn, then make sure the deferred cleanup is idempotent.
	cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := cc.RoundTrip(ctx, H2Request{
		Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
	})
	if err == nil {
		t.Fatal("RoundTrip after Close returned nil; want error")
	}
}

// TestClientConn_RoundTrip_StreamIDMonotonicity — three sequential
// RoundTrips on the same conn; observe stream ids 1, 3, 5 server-side.
// Run sequentially so the wire-order matches the allocator order; the
// allocator itself is the unit-under-test, not the concurrency model.
func TestClientConn_RoundTrip_StreamIDMonotonicity(t *testing.T) {
	cc, peer, cleanup := dialClientConn(t)
	defer cleanup()

	const n = 3
	idsCh := make(chan uint32, n)
	peerDone := make(chan error, 1)
	go func() {
		var firstErr error
		for i := 0; i < n; i++ {
			hf, _, err := peer.readRequestHeaders()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				break
			}
			idsCh <- hf.StreamID
			if err := peer.writeResponse(hf.StreamID, 200, nil, []byte("ok")); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				break
			}
		}
		peerDone <- firstErr
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i := 0; i < n; i++ {
		_, err := cc.RoundTrip(ctx, H2Request{
			Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
		})
		if err != nil {
			t.Fatalf("RoundTrip %d: %v", i, err)
		}
	}
	if err := <-peerDone; err != nil {
		t.Fatalf("peer: %v", err)
	}
	close(idsCh)
	var got []uint32
	for id := range idsCh {
		got = append(got, id)
	}
	want := []uint32{1, 3, 5}
	if len(got) != len(want) {
		t.Fatalf("got %d ids, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stream id[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}
