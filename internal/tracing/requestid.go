package tracing

import "encoding/hex"

// TraceReason is the shared trace-decision vocabulary (D-TRACE-REQUESTID). It is
// encoded into the x-request-id UUID at canonical string index 14 (the version
// nibble) so the decision survives downstream re-propagation: NoTrace, Sampled
// (random/overall sampling), Client (client-forced). Consumed here for the
// request-id stamp and by the sampling decision engine (Task 6).
type TraceReason int

const (
	// NoTrace marks an id that was NOT selected for tracing (nibble '4').
	NoTrace TraceReason = iota
	// Sampled marks an id selected by random/overall sampling (nibble '9').
	Sampled
	// Client marks an id force-traced by the downstream client (nibble 'b').
	Client
)

// reasonNibble maps a TraceReason to its canonical UUID version-nibble hex char.
// An unknown/default reason maps to '4' (NoTrace) defensively.
func reasonNibble(r TraceReason) byte {
	switch r {
	case Sampled:
		return '9'
	case Client:
		return 'b'
	case NoTrace:
		return '4'
	default:
		return '4'
	}
}

// GenerateRequestID produces a fresh x-request-id: a canonical lowercase UUIDv4
// string with the trace-decision REASON stamped into string index 14 (the version
// nibble). It reads 16 bytes from rng, sets the v4 version (byte-6 high nibble = 4)
// and RFC-4122 variant (byte-8 high bits = 10), formats as 8-4-4-4-12 hex, then
// overwrites index 14 with reasonNibble(reason).
func GenerateRequestID(reason TraceReason, rng RandSource) string {
	var b [16]byte
	_, _ = rng.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC-4122 variant (10xx)

	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])

	buf[14] = reasonNibble(reason)
	return string(buf[:])
}

// StampRequestID overwrites the trace-decision nibble (string index 14) of an
// EXISTING x-request-id, preserving every other byte verbatim. The reference does
// NOT re-validate the inbound id; if it is too short to hold index 14 (len < 15)
// the input is returned unchanged (defensive, never panics).
func StampRequestID(existing string, reason TraceReason) string {
	if len(existing) < 15 {
		return existing
	}
	b := []byte(existing)
	b[14] = reasonNibble(reason)
	return string(b)
}
