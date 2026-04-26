package h2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/net/http2"
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
