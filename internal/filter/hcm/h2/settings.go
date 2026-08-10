package h2

import (
	"golang.org/x/net/http2"
)

// ServerSettings is the value-typed bundle of phase-05.1 server-side SETTINGS
// values. Per ADR-0047 the values are hardcoded; the struct exists so future
// phases can vary per-listener if a use case demands.
type ServerSettings struct {
	MaxConcurrentStreams uint32
	InitialWindowSize    uint32
	MaxFrameSize         uint32
	EnablePush           uint32 // 0 = disabled (phase 05.1 always)
	NoRFC7540Priorities  uint32 // 1 = announce client we discard PRIORITY
	HeaderTableSize      uint32
}

// DefaultServerSettings is the phase-05.1 hardcoded set per ADR-0047.
var DefaultServerSettings = ServerSettings{
	MaxConcurrentStreams: 100,
	InitialWindowSize:    65535,
	MaxFrameSize:         16384,
	EnablePush:           0,
	NoRFC7540Priorities:  1,
	HeaderTableSize:      4096,
}

// clientSettings holds the values the client announces in its initial SETTINGS
// frame (or in subsequent SETTINGS updates). Only the fields phase 05.1 reads
// are present.
type clientSettings struct {
	MaxConcurrentStreams uint32
	InitialWindowSize    uint32
	MaxFrameSize         uint32
	HeaderTableSize      uint32
	EnablePush           uint32
	// InitialWindowSizeAnnounced distinguishes "the client explicitly
	// announced SETTINGS_INITIAL_WINDOW_SIZE=0" (a legal RFC 9113 §6.9.2
	// value that must hold all response DATA) from "the client has never
	// sent the setting" (the zero Go value, which must default to 65535).
	// Set at both assignment sites: readClientSettings (handshake path) and
	// ServerConn.onSettings (mid-connection path). Phase 85; ADR-0307.
	InitialWindowSizeAnnounced bool
}

// writeServerInitialSettings writes a single SETTINGS frame (no ACK flag) to
// the peer carrying the six configured values. The peer is expected to send
// SETTINGS_ACK in response (the ServerConn frame loop reads it).
func writeServerInitialSettings(fr *framer, s ServerSettings) error {
	return fr.WriteSettings(
		http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: s.MaxConcurrentStreams},
		http2.Setting{ID: http2.SettingInitialWindowSize, Val: s.InitialWindowSize},
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: s.MaxFrameSize},
		http2.Setting{ID: http2.SettingEnablePush, Val: s.EnablePush},
		http2.Setting{ID: http2.SettingHeaderTableSize, Val: s.HeaderTableSize},
		// SETTINGS_NO_RFC7540_PRIORITIES is RFC 9218; x/net/http2 may not have
		// a constant for it. Use the numeric ID 0x9 directly.
		http2.Setting{ID: http2.SettingID(0x9), Val: s.NoRFC7540Priorities},
	)
}

// writeClientInitialSettings writes the client's initial SETTINGS frame.
// In phase 05.2 the client and server settings are byte-identical (the same
// DefaultServerSettings constants per ADR-0047 apply on both sides;
// SETTINGS_ENABLE_PUSH=0 is correct for the client per RFC 9113 §6.5.2 because
// clients can't accept PUSH — advertising it disabled is symmetric correctness).
// A separate helper exists from writeServerInitialSettings for future divergence
// (when client-only settings tuning lands in a future phase).
func writeClientInitialSettings(fr *framer, s ServerSettings) error {
	return fr.WriteSettings(
		http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: s.MaxConcurrentStreams},
		http2.Setting{ID: http2.SettingInitialWindowSize, Val: s.InitialWindowSize},
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: s.MaxFrameSize},
		http2.Setting{ID: http2.SettingEnablePush, Val: s.EnablePush},
		http2.Setting{ID: http2.SettingHeaderTableSize, Val: s.HeaderTableSize},
		// SETTINGS_NO_RFC7540_PRIORITIES is RFC 9218; not in stdlib's enum.
		http2.Setting{ID: http2.SettingID(0x9), Val: s.NoRFC7540Priorities},
	)
}

// validateSetting enforces the RFC 9113 §6.5.2 value constraints for one
// SETTINGS parameter. nil means acceptable. The returned error carries the
// RFC-mandated connection-error code: PROTOCOL_ERROR for ENABLE_PUSH and
// MAX_FRAME_SIZE violations, FLOW_CONTROL_ERROR for an INITIAL_WINDOW_SIZE
// above 2^31-1. Called from BOTH the handshake path (readClientSettings) and
// the mid-connection path (ServerConn.onSettings) — phase 85; ADR-0307.
func validateSetting(st http2.Setting) *Error {
	switch st.ID {
	case http2.SettingEnablePush:
		if st.Val > 1 {
			return &Error{Code: ErrProtocolError, Msg: "SETTINGS_ENABLE_PUSH must be 0 or 1"}
		}
	case http2.SettingMaxFrameSize:
		if st.Val < 16384 || st.Val > 16777215 {
			return &Error{Code: ErrProtocolError, Msg: "SETTINGS_MAX_FRAME_SIZE outside [2^14, 2^24-1]"}
		}
	case http2.SettingInitialWindowSize:
		// Values > 2^31-1 are rejected at frame-PARSE time by the vendored
		// x/net framer (parseSettingsFrame returns
		// ConnectionError(ErrCodeFlowControl) before this code runs), so this
		// arm is defense-in-depth against an x/net behavior change, not a
		// live rejection path. Measured cost: 3 code lines (PLAN §2.1).
		if st.Val > 2147483647 {
			return &Error{Code: ErrFlowControlError, Msg: "SETTINGS_INITIAL_WINDOW_SIZE exceeds 2^31-1"}
		}
	}
	return nil
}

// readClientSettings reads one SETTINGS frame from fr and applies its values
// to applyTo. Returns *Error{Code: PROTOCOL_ERROR} if the first frame is a
// SETTINGS_ACK (RFC 9113 §6.5: server must read client's initial SETTINGS
// before reading the ACK to its own).
//
// Validation runs strictly before application: a first ForeachSetting pass
// runs validateSetting over every parameter (first error wins, captured
// outside the closure); only if that pass finds nothing wrong does a second
// pass apply the values, so an invalid frame applies none of its values.
func readClientSettings(fr *framer, applyTo *clientSettings) error {
	frame, err := fr.ReadFrame()
	if err != nil {
		// Preserve the framer's own ConnectionError/StreamError code (e.g. an
		// x/net parse-time reject of INITIAL_WINDOW_SIZE > 2^31-1 surfaces
		// FLOW_CONTROL_ERROR) instead of blanket-wrapping every read failure
		// as PROTOCOL_ERROR.
		return translateFramerErr(err)
	}
	sf, ok := frame.(*http2.SettingsFrame)
	if !ok {
		return &Error{Code: ErrProtocolError, Msg: "expected SETTINGS frame, got " + frame.Header().Type.String()}
	}
	if sf.IsAck() {
		return &Error{Code: ErrProtocolError, Msg: "ACK on first client SETTINGS"}
	}

	var firstErr *Error
	_ = sf.ForeachSetting(func(s http2.Setting) error {
		if firstErr == nil {
			firstErr = validateSetting(s)
		}
		return nil
	})
	if firstErr != nil {
		return firstErr
	}

	_ = sf.ForeachSetting(func(s http2.Setting) error {
		switch s.ID {
		case http2.SettingMaxConcurrentStreams:
			applyTo.MaxConcurrentStreams = s.Val
		case http2.SettingInitialWindowSize:
			applyTo.InitialWindowSize = s.Val
			applyTo.InitialWindowSizeAnnounced = true
		case http2.SettingMaxFrameSize:
			applyTo.MaxFrameSize = s.Val
		case http2.SettingHeaderTableSize:
			applyTo.HeaderTableSize = s.Val
		case http2.SettingEnablePush:
			applyTo.EnablePush = s.Val
		}
		// Unknown settings are silently ignored per RFC 9113 §6.5.2.
		return nil
	})
	return nil
}
