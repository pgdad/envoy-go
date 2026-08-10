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
// phase-85 (h2spec-selector-repair) Task 1: SETTINGS validator arms.
//
// These arms are written FIRST, TDD-style, and are EXPECTED TO FAIL (RED) on
// the unfixed tree — onSettings (conn.go) and readClientSettings (settings.go)
// currently apply every SETTINGS value unconditionally with no range
// validation. Task 2 lands the validator that greens the RED arms below.
// A handful of arms are REGRESSION PINS (already green — labeled as such)
// guarding behavior that must not break while Task 2 lands the validator.
// ---------------------------------------------------------------------------

// driveHandshake performs the client side of the connection handshake used
// by every mid-connection arm below: write the client preface, write an
// EMPTY client SETTINGS frame (i.e. no arm-under-test values — those are
// sent as a SECOND, mid-connection SETTINGS frame by the caller), read+ACK
// the server's initial SETTINGS, then drain the SETTINGS ACK for the
// client's own initial frame. Mirrors the pattern at
// conn_test.go:352-393 / conn_test.go:1152-1170
// (TestServerConn_GOAWAYOnProtocolError_EvenStreamID /
// TestServerConn_WriteData_RespectsPerStreamSendWindow).
func driveHandshake(t *testing.T, clientConn net.Conn, fr *http2.Framer) {
	t.Helper()
	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}
	if err := fr.WriteSettings(); err != nil {
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
				if err := fr.WriteSettingsAck(); err != nil {
					t.Fatalf("write settings ack: %v", err)
				}
				settingsAcked = true
			} else {
				clientAcked = true
			}
		}
	}
}

// writeGetHeaders writes a minimal well-formed GET HEADERS frame for
// streamID, optionally ending the stream.
func writeGetHeaders(t *testing.T, fr *http2.Framer, streamID uint32, path string, endStream bool) {
	t.Helper()
	var encBuf bytes.Buffer
	enc := hpack.NewEncoder(&encBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: path})
	_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: encBuf.Bytes(),
		EndHeaders:    true,
		EndStream:     endStream,
	}); err != nil {
		t.Fatalf("write headers: %v", err)
	}
}

// assertMidConnGoaway sends setting as a mid-connection SETTINGS update
// (after driveHandshake has already completed) and asserts the server
// eventually emits GOAWAY with code wantCode. If the server instead ACKs the
// SETTINGS frame, that is the RED reading on the unfixed tree: no validator
// exists yet, so every value (legal or not) is silently accepted.
func assertMidConnGoaway(t *testing.T, clientConn net.Conn, fr *http2.Framer, setting http2.Setting, wantCode http2.ErrCode) {
	t.Helper()
	if err := fr.WriteSettings(setting); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("expected GOAWAY(%v); read error before any decisive frame arrived: %v", wantCode, err)
		}
		if f.Header().Type == http2.FrameGoAway {
			gf := f.(*http2.GoAwayFrame)
			if gf.ErrCode != wantCode {
				t.Errorf("GOAWAY code = %v, want %v", gf.ErrCode, wantCode)
			}
			return
		}
		if f.Header().Type == http2.FrameSettings && f.(*http2.SettingsFrame).IsAck() {
			t.Fatalf("expected GOAWAY(%v); got SETTINGS ACK", wantCode)
		}
	}
}

// assertMidConnAccepted sends setting as a mid-connection SETTINGS update and
// asserts the server ACKs it (no GOAWAY). Used for the MAX_FRAME_SIZE
// boundary CONTROLS, which must stay green both before and after Task 2's
// validator lands.
func assertMidConnAccepted(t *testing.T, clientConn net.Conn, fr *http2.Framer, setting http2.Setting) {
	t.Helper()
	if err := fr.WriteSettings(setting); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("expected SETTINGS ACK for a legal value; read error: %v", err)
		}
		if f.Header().Type == http2.FrameGoAway {
			gf := f.(*http2.GoAwayFrame)
			t.Fatalf("unexpected GOAWAY(%v) for a legal SETTINGS value", gf.ErrCode)
		}
		if f.Header().Type == http2.FrameSettings && f.(*http2.SettingsFrame).IsAck() {
			return
		}
	}
}

// assertHandshakeGoaway sends the client's FIRST (and only) SETTINGS frame
// carrying setting, before any ACK exchange, and asserts GOAWAY with code
// wantCode. Mirrors assertMidConnGoaway's read loop but does not drive a
// prior handshake — the arm-under-test value IS the handshake SETTINGS.
func assertHandshakeGoaway(t *testing.T, clientConn net.Conn, fr *http2.Framer, setting http2.Setting, wantCode http2.ErrCode) {
	t.Helper()
	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}
	if err := fr.WriteSettings(setting); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("expected GOAWAY(%v); read error before any decisive frame arrived: %v", wantCode, err)
		}
		switch tf := f.(type) {
		case *http2.GoAwayFrame:
			if tf.ErrCode != wantCode {
				t.Errorf("GOAWAY code = %v, want %v", tf.ErrCode, wantCode)
			}
			return
		case *http2.SettingsFrame:
			if tf.IsAck() {
				t.Fatalf("expected GOAWAY(%v); got SETTINGS ACK", wantCode)
			}
			// Else: the server's own unconditional initial SETTINGS frame
			// (written before it has read ours) — keep reading.
		}
	}
}

// ---------------------------------------------------------------------------
// Step 1: mid-connection validation arms.
// ---------------------------------------------------------------------------

// TestSettingsValidate_MidConn_EnablePushInvalid_GOAWAYProtocolError:
// RFC 9113 §6.5.2 — SETTINGS_ENABLE_PUSH MUST be 0 or 1; any other value is
// a connection error PROTOCOL_ERROR. RED anchor: onSettings applies every
// value unconditionally today, so the server ACKs instead of GOAWAY-ing.
func TestSettingsValidate_MidConn_EnablePushInvalid_GOAWAYProtocolError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)
	driveHandshake(t, clientConn, fr)

	assertMidConnGoaway(t, clientConn, fr,
		http2.Setting{ID: http2.SettingEnablePush, Val: 2},
		http2.ErrCodeProtocol)
}

// TestSettingsValidate_MidConn_MaxFrameSizeTooSmall_GOAWAYProtocolError:
// RFC 9113 §6.5.2 — SETTINGS_MAX_FRAME_SIZE MUST be within
// [16384, 16777215]; below the minimum is a connection error PROTOCOL_ERROR.
// RED anchor (same mechanism as the ENABLE_PUSH arm above).
func TestSettingsValidate_MidConn_MaxFrameSizeTooSmall_GOAWAYProtocolError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)
	driveHandshake(t, clientConn, fr)

	assertMidConnGoaway(t, clientConn, fr,
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16383},
		http2.ErrCodeProtocol)
}

// TestSettingsValidate_MidConn_MaxFrameSizeTooLarge_GOAWAYProtocolError:
// above the RFC 9113 §6.5.2 maximum (16777215 = 2^24-1) is also a connection
// error PROTOCOL_ERROR. RED anchor.
func TestSettingsValidate_MidConn_MaxFrameSizeTooLarge_GOAWAYProtocolError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)
	driveHandshake(t, clientConn, fr)

	assertMidConnGoaway(t, clientConn, fr,
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16777216},
		http2.ErrCodeProtocol)
}

// TestSettingsValidate_MidConn_InitialWindowSizeOverflow_GOAWAYFlowControlError:
// REGRESSION PIN — NOT a RED anchor; green on arrival. x/net v0.34.0's
// parseSettingsFrame already rejects SETTINGS_INITIAL_WINDOW_SIZE > 2^31-1 at
// frame-PARSE time (golang.org/x/net/http2/frame.go:747), returning a
// ConnectionError(ErrCodeFlowControl) from ReadFrame BEFORE the frame ever
// reaches onSettings/dispatchFrame. translateFramerErr already maps that
// through correctly, so this is a delegated guard the subject inherits for
// free. Kept as a pin so a future refactor of the framer wrapper cannot
// silently regress it.
func TestSettingsValidate_MidConn_InitialWindowSizeOverflow_GOAWAYFlowControlError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)
	driveHandshake(t, clientConn, fr)

	assertMidConnGoaway(t, clientConn, fr,
		http2.Setting{ID: http2.SettingInitialWindowSize, Val: 2147483648}, // 2^31
		http2.ErrCodeFlowControl)
}

// TestSettingsValidate_MidConn_MaxFrameSizeBoundaryMin_Accepted: CONTROL,
// green before AND after Task 2. 16384 is the RFC 9113 §6.5.2 legal minimum.
func TestSettingsValidate_MidConn_MaxFrameSizeBoundaryMin_Accepted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)
	driveHandshake(t, clientConn, fr)

	assertMidConnAccepted(t, clientConn, fr,
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16384})
}

// TestSettingsValidate_MidConn_MaxFrameSizeBoundaryMax_Accepted: CONTROL,
// green before AND after Task 2. 16777215 (2^24-1) is the RFC 9113 §6.5.2
// legal maximum.
func TestSettingsValidate_MidConn_MaxFrameSizeBoundaryMax_Accepted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)
	driveHandshake(t, clientConn, fr)

	assertMidConnAccepted(t, clientConn, fr,
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16777215})
}

// ---------------------------------------------------------------------------
// Step 2: handshake-path arms — the same invalid values sent as the client's
// FIRST SETTINGS frame (before any ACK exchange).
// ---------------------------------------------------------------------------

// TestHandshakeSettings_EnablePushInvalid_GOAWAYProtocolError: RED anchor,
// same signature as the mid-connection ENABLE_PUSH arm. readClientSettings
// (settings.go) applies every value unconditionally with no validation, so
// Run() unconditionally ACKs instead of GOAWAY-ing.
func TestHandshakeSettings_EnablePushInvalid_GOAWAYProtocolError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)

	assertHandshakeGoaway(t, clientConn, fr,
		http2.Setting{ID: http2.SettingEnablePush, Val: 2},
		http2.ErrCodeProtocol)
}

// TestHandshakeSettings_MaxFrameSizeTooSmall_GOAWAYProtocolError: RED anchor,
// same signature.
func TestHandshakeSettings_MaxFrameSizeTooSmall_GOAWAYProtocolError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)

	assertHandshakeGoaway(t, clientConn, fr,
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16383},
		http2.ErrCodeProtocol)
}

// TestHandshakeSettings_MaxFrameSizeTooLarge_GOAWAYProtocolError: RED anchor,
// same signature.
func TestHandshakeSettings_MaxFrameSizeTooLarge_GOAWAYProtocolError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)

	assertHandshakeGoaway(t, clientConn, fr,
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16777216},
		http2.ErrCodeProtocol)
}

// TestHandshakeSettings_InitialWindowSizeOverflow_WrongCode: RED anchor for
// the WRONG-CODE reason (not the missing-validator reason the other
// handshake arms hit). x/net's parseSettingsFrame already rejects
// IWS > 2^31-1 at parse time (see the MidConn IWS pin above), so
// readClientSettings's plain fr.ReadFrame() call DOES return an error — but
// readClientSettings (settings.go:79-90) blanket-wraps EVERY ReadFrame error
// as &Error{Code: ErrProtocolError}, discarding the framer's own
// FLOW_CONTROL_ERROR code, and Run() (conn.go) further hardcodes
// s.emitGoaway(ErrProtocolError) at the call site. The wire therefore DOES
// carry a GOAWAY (not a stray ACK) — just with the wrong code. This forces
// Task 2's readClientSettings + Run() plumbing edit to propagate the
// framer's real error code instead of a hardcoded PROTOCOL_ERROR.
func TestHandshakeSettings_InitialWindowSizeOverflow_WrongCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "ok"}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)

	assertHandshakeGoaway(t, clientConn, fr,
		http2.Setting{ID: http2.SettingInitialWindowSize, Val: 2147483648}, // 2^31
		http2.ErrCodeFlowControl)
}

// TestHandshakeSettings_IWSZero_HoldsResponseData: RED anchor for the §2.2
// IWS=0 seeding quirk. RFC 9113 §6.9.2: SETTINGS_INITIAL_WINDOW_SIZE=0 is a
// legal, deliberate announcement — every new stream's send window must start
// at exactly 0, holding all response DATA until a WINDOW_UPDATE arrives.
//
// The RED mechanism: onHeaders (conn.go) computes
//
//	peerInitWindow := int32(s.clientS.InitialWindowSize)
//	if peerInitWindow <= 0 {
//	    peerInitWindow = 65535
//	}
//
// which conflates the zero VALUE (a legitimate announcement) with the
// zero-value DEFAULT (client hasn't sent SETTINGS_INITIAL_WINDOW_SIZE at
// all). An explicit IWS=0 is silently replaced with 65535, so a small
// response body sails straight through instead of being held.
func TestHandshakeSettings_IWSZero_HoldsResponseData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const body = "OK" // 2 bytes
	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: body}}
	clientConn, _ := startServerConn(t, ctx, disp, DefaultServerSettings)

	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("write preface: %v", err)
	}
	fr := http2.NewFramer(clientConn, clientConn)
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingInitialWindowSize, Val: 0}); err != nil {
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

	writeGetHeaders(t, fr, 1, "/iws-zero", true)

	// Bounded wait: DATA must NOT arrive while the announced window is 0.
	prematureData := false
	_ = clientConn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
readLoop:
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			break // timeout — correctly held (fixed-tree behavior)
		}
		if df, ok := f.(*http2.DataFrame); ok {
			t.Errorf("DATA (%d bytes) arrived despite announced SETTINGS_INITIAL_WINDOW_SIZE=0", len(df.Data()))
			prematureData = true
			break readLoop
		}
	}
	if prematureData {
		return // RED already recorded — nothing further to verify on this tree.
	}

	// Correctly held (fixed-tree behavior): unblock via a stream-level
	// WINDOW_UPDATE and confirm the response completes.
	if err := fr.WriteWindowUpdate(1, uint32(len(body))); err != nil {
		t.Fatalf("write window update: %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var got bytes.Buffer
	for got.Len() < len(body) {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("waiting for unblocked DATA: %v", err)
		}
		if df, ok := f.(*http2.DataFrame); ok {
			got.Write(df.Data())
		}
	}
	if got.String() != body {
		t.Errorf("response body = %q, want %q", got.String(), body)
	}
}
