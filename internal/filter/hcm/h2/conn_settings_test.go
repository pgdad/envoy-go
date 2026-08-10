package h2

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// ---------------------------------------------------------------------------
// phase-85 (h2spec-selector-repair) Task 1: RFC 9113 §6.9.2 "walk" arms.
//
// §6.9.2: when SETTINGS_INITIAL_WINDOW_SIZE changes, EVERY existing stream's
// send-side flow-control window must be adjusted by the delta
// (new - old) — not just newly-opened streams. onSettings (conn.go) today
// only updates s.clientS.InitialWindowSize; it never walks s.streams, so
// none of the arms below (a/b/c) have any effect on an already-open stream.
// Arm (d) is the one exception that already works (a REGRESSION PIN).
// ---------------------------------------------------------------------------

// startServerConnRef mirrors conn_test.go:86 (startServerConn) but also
// returns the *ServerConn value so the walk arms can inspect internal
// per-stream send-window state directly (white-box — same package). This is
// a NEW helper in a NEW file; conn_test.go itself is not edited per the
// phase-85 IMPL Task 1 brief.
func startServerConnRef(t *testing.T, ctx context.Context, dispatcher Dispatcher, settings ServerSettings) (net.Conn, *ServerConn, <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	scCh := make(chan *ServerConn, 1)
	serverDone := make(chan error, 1)
	go func() {
		serverConn, err := ln.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		sc := NewServerConn(ctx, serverConn, dispatcher, settings)
		scCh <- sc
		serverDone <- sc.Run()
	}()

	clientConn, err := net.DialTCP("tcp", nil, ln.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = clientConn.Close() })
	sc := <-scCh
	return clientConn, sc, serverDone
}

// driveHandshakeWithIWS is like driveHandshake (settings_validate_test.go)
// but announces an explicit SETTINGS_INITIAL_WINDOW_SIZE instead of an empty
// initial SETTINGS frame, so the walk arms below have a known starting
// per-stream send window.
func driveHandshakeWithIWS(t *testing.T, clientConn net.Conn, fr *http2.Framer, iws uint32) {
	t.Helper()
	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingInitialWindowSize, Val: iws}); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	settingsAcked, clientAcked := false, false
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
}

// awaitSettingsAck blocks until a SETTINGS ACK frame is read. Used as an
// ordering barrier: onSettings writes the ACK only after it has finished
// applying (and, once Task 2 lands, walking) the new value, and TCP
// guarantees any frame written by the client BEFORE the triggering SETTINGS
// frame was processed by the single-goroutine frame loop first.
func awaitSettingsAck(t *testing.T, clientConn net.Conn, fr *http2.Framer) {
	t.Helper()
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("waiting for SETTINGS ACK: %v", err)
		}
		if f.Header().Type == http2.FrameSettings && f.(*http2.SettingsFrame).IsAck() {
			return
		}
	}
}

// TestSettingsWalk_IncreaseAdjustsLiveStreamWindow: walk arm (a). RED anchor.
// Client announces IWS=65535 at handshake, opens stream 1 (left open — no
// END_STREAM, so the dispatcher never runs and the stream stays parked in
// s.streams), then sends a mid-connection SETTINGS IWS=70000. Per RFC 9113
// §6.9.2 the live stream's send window must advance by exactly the delta
// (+4465) with no WINDOW_UPDATE involved. onSettings today never walks
// s.streams, so the window is left untouched at 65535.
func TestSettingsWalk_IncreaseAdjustsLiveStreamWindow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, sc, _ := startServerConnRef(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)
	driveHandshakeWithIWS(t, clientConn, fr, 65535)

	writeGetHeaders(t, fr, 1, "/walk-a", false)

	// Mid-connection SETTINGS: INITIAL_WINDOW_SIZE 65535 -> 70000 (+4465).
	// The SETTINGS ACK barrier below guarantees stream 1's HEADERS (written
	// strictly earlier on the same TCP connection) has already been
	// processed and registered in sc.streams.
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingInitialWindowSize, Val: 70000}); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	awaitSettingsAck(t, clientConn, fr)

	sc.mu.Lock()
	ss, ok := sc.streams[1]
	sc.mu.Unlock()
	if !ok {
		t.Fatalf("stream 1 was never registered by the server")
	}
	if got := ss.sendW.available(); got != 70000 {
		t.Errorf("live stream send window = %d, want 70000", got)
	}
}

// windowWalkAction writes a first DATA chunk, blocks on release, then writes
// a second DATA chunk. Used by walk arm (b) to control exactly when the
// "pending write" is attempted relative to the mid-connection SETTINGS
// change under test.
type windowWalkAction struct {
	first, second []byte
	release       <-chan struct{}
}

func (a *windowWalkAction) WriteH2(_ context.Context, _ H2Request, sw StreamWriter) error {
	headers := []hpack.HeaderField{{Name: ":status", Value: "200"}}
	if err := sw.WriteHeaders(headers, false); err != nil {
		return err
	}
	if err := sw.WriteData(a.first, false); err != nil {
		return err
	}
	<-a.release
	return sw.WriteData(a.second, true)
}

// TestSettingsWalk_DecreaseDrivesWindowNegative_WindowUpdateUnblocks: walk
// arm (b). RED anchor. Client announces IWS=1000 at handshake; the
// dispatcher writes a 900-byte first chunk (window: 1000 -> 100), then
// blocks. The test sends a mid-connection SETTINGS IWS=0 (delta -1000). Per
// RFC 9113 §6.9.2 the live stream's window must become 100 + (-1000) = -900
// (negative — sending must BLOCK, not error). The dispatcher is then
// released to attempt its second (10-byte) write, which must stay blocked
// until a stream-level WINDOW_UPDATE brings the window positive again.
//
// onSettings today never walks s.streams, so the window is left untouched at
// 100 — the second write proceeds immediately instead of blocking.
func TestSettingsWalk_DecreaseDrivesWindowNegative_WindowUpdateUnblocks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first := bytes.Repeat([]byte{'A'}, 900) // leaves window at 100 (1000-900)
	second := []byte("0123456789")          // 10 bytes — the "pending write"
	release := make(chan struct{})

	action := &windowWalkAction{first: first, second: second, release: release}
	disp := &fixedDispatcher{action: action}
	clientConn, sc, _ := startServerConnRef(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)
	driveHandshakeWithIWS(t, clientConn, fr, 1000)

	writeGetHeaders(t, fr, 1, "/walk-b", true)

	// Drain frames until the first DATA chunk (900 bytes) has fully arrived —
	// confirms the stream "consumed" the handshake window down to 100.
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	firstReceived := 0
	for firstReceived < len(first) {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("waiting for first DATA chunk: %v", err)
		}
		if df, ok := f.(*http2.DataFrame); ok {
			firstReceived += len(df.Data())
		}
	}

	// Mid-connection SETTINGS: INITIAL_WINDOW_SIZE 1000 -> 0 (delta -1000).
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingInitialWindowSize, Val: 0}); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	awaitSettingsAck(t, clientConn, fr)

	// Release the dispatcher's second write now that the SETTINGS change has
	// been fully applied (the ACK barrier above guarantees ordering).
	close(release)

	// Bounded wait: on the unfixed tree the window was never touched by the
	// SETTINGS change and remains 100 (>= 10), so the second write proceeds
	// immediately — that is the RED reading.
	prematureData := false
	_ = clientConn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
readLoop:
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			break // timeout — correctly blocked (fixed-tree behavior)
		}
		if df, ok := f.(*http2.DataFrame); ok {
			window := int32(-1)
			sc.mu.Lock()
			if ss, ok := sc.streams[1]; ok {
				window = ss.sendW.available()
			}
			sc.mu.Unlock()
			t.Errorf("DATA (%d bytes) arrived while window should be negative (window=%d)", len(df.Data()), window)
			prematureData = true
			break readLoop
		}
	}
	if prematureData {
		return // RED already recorded — nothing further to verify on this tree.
	}

	// Correctly blocked (fixed-tree behavior): unblock with a stream-level
	// WINDOW_UPDATE and confirm the pending write completes.
	if err := fr.WriteWindowUpdate(1, 1000); err != nil {
		t.Fatalf("write window update: %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	secondReceived := 0
	for secondReceived < len(second) {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("waiting for unblocked DATA: %v", err)
		}
		if df, ok := f.(*http2.DataFrame); ok {
			secondReceived += len(df.Data())
		}
	}
}

// TestSettingsWalk_OverflowIsConnectionError: walk arm (c). RED anchor.
// Client announces IWS=65535, opens stream 1 (left open), then pushes the
// stream's send window up to exactly 2^31-1 (the legal maximum) via a
// stream-level WINDOW_UPDATE — onWindowUpdate's own bounds check (RFC 9113
// §6.9.1, conn.go:559+) allows this; it is a STREAM error on overflow, not a
// connection error. The test then sends a mid-connection SETTINGS
// IWS=65536 (delta +1): per RFC 9113 §6.9.2, a SETTINGS-driven adjustment
// that pushes a live stream's window past 2^31-1 MUST be a CONNECTION error
// FLOW_CONTROL_ERROR — a different error scope than the WINDOW_UPDATE case,
// which is the distinction this arm exercises.
//
// onSettings today never walks s.streams (no overflow detection at all), so
// the server just ACKs.
func TestSettingsWalk_OverflowIsConnectionError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _, _ := startServerConnRef(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)
	driveHandshakeWithIWS(t, clientConn, fr, 65535)

	writeGetHeaders(t, fr, 1, "/walk-c", false)

	const maxWindow = int32(2147483647) // 2^31 - 1
	const startWindow = int32(65535)
	if err := fr.WriteWindowUpdate(1, uint32(maxWindow-startWindow)); err != nil {
		t.Fatalf("write window update: %v", err)
	}

	// Mid-connection SETTINGS: INITIAL_WINDOW_SIZE 65535 -> 65536 (+1). The
	// live stream's window (now at the maximum) would be pushed to 2^31, one
	// past the legal maximum.
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingInitialWindowSize, Val: 65536}); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("expected GOAWAY(FLOW_CONTROL_ERROR); read error before any decisive frame arrived: %v", err)
		}
		if f.Header().Type == http2.FrameGoAway {
			gf := f.(*http2.GoAwayFrame)
			if gf.ErrCode != http2.ErrCodeFlowControl {
				t.Errorf("GOAWAY code = %v, want FLOW_CONTROL_ERROR", gf.ErrCode)
			}
			return
		}
		if f.Header().Type == http2.FrameSettings && f.(*http2.SettingsFrame).IsAck() {
			t.Fatalf("expected GOAWAY(FLOW_CONTROL_ERROR); got SETTINGS ACK")
		}
	}
}

// TestSettingsWalk_NewStreamSeedsAtLatestAnnouncedIWS: walk arm (d).
// REGRESSION PIN — NOT a RED anchor; green on arrival. onHeaders (conn.go)
// already reads s.clientS.InitialWindowSize (kept current by onSettings) at
// stream-CREATION time, so a stream opened AFTER a mid-connection SETTINGS
// change already seeds at the latest announced value. This guards against a
// regression in Task 2's edits to onSettings/onHeaders when the walk logic
// for existing streams is added alongside it.
func TestSettingsWalk_NewStreamSeedsAtLatestAnnouncedIWS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, sc, _ := startServerConnRef(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)
	driveHandshake(t, clientConn, fr) // default handshake IWS (65535)

	// Mid-connection SETTINGS: INITIAL_WINDOW_SIZE -> 70000.
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingInitialWindowSize, Val: 70000}); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	awaitSettingsAck(t, clientConn, fr)

	// Open a NEW stream (left open — no END_STREAM) after the SETTINGS
	// change; it must seed its send window at the latest value (70000), not
	// the handshake default.
	writeGetHeaders(t, fr, 1, "/walk-d", false)

	// Barrier: a PING round trip guarantees the HEADERS frame (written
	// strictly before the PING on the same connection) has already been
	// processed by the single-goroutine frame loop.
	var pingData [8]byte
	if err := fr.WritePing(false, pingData); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("waiting for PING ACK: %v", err)
		}
		if pf, ok := f.(*http2.PingFrame); ok && pf.IsAck() {
			break
		}
	}

	sc.mu.Lock()
	ss, ok := sc.streams[1]
	sc.mu.Unlock()
	if !ok {
		t.Fatalf("stream 1 was never registered by the server")
	}
	if got := ss.sendW.available(); got != 70000 {
		t.Errorf("new stream send window = %d, want 70000", got)
	}
}
