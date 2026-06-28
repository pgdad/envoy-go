package tracing

import (
	"encoding/hex"
	"net/http"
	"strings"
)

// ExtractB3 parses Zipkin B3 trace-context from h, returning the decoded
// TraceContext (the B3 path leaves TraceState empty). It is the Zipkin analog of
// ExtractTraceparent over an UNTRUSTED-input boundary.
//
// The single "b3" header takes precedence when present:
// "<traceid>-<spanid>[-<sampled>[-<parentid>]]" where traceid is 16 hex (64-bit,
// occupying the LOW 8 bytes of the [16]byte) or 32 hex (128-bit), sampled is
// "1"/"d" (sampled, "d" = debug) or "0" (not sampled). The incoming span-id
// (field[1]) is ALWAYS our server span's parent — the caller's current span is our
// parent. The optional 4th field is the caller's OWN parent (our grandparent): it
// is validated for framing but accepted-and-ignored; the reference continues on the
// incoming span-id (SPEC §11 D-TRACE-ZIPKIN-B3).
//
// Absent the "b3" header, the multi-header X-B3-* set is read:
// X-B3-TraceId + X-B3-SpanId are required; the incoming X-B3-SpanId is ALWAYS the
// parent. X-B3-ParentSpanId is optional (the caller's own parent / our grandparent):
// validated for framing but accepted-and-ignored. Sampled = (X-B3-Sampled == "1")
// OR (X-B3-Flags == "1").
//
// An all-zero trace-id/span-id/parent-id and any framing/hex/length failure yields
// (TraceContext{}, false).
func ExtractB3(h http.Header) (TraceContext, bool) {
	if v := h.Get("B3"); v != "" {
		return extractB3Single(v)
	}
	return extractB3Multi(h)
}

func extractB3Single(v string) (TraceContext, bool) {
	parts := strings.Split(v, "-")
	if len(parts) < 2 || len(parts) > 4 {
		return TraceContext{}, false
	}

	var ctx TraceContext
	if !decodeB3TraceID(parts[0], &ctx.TraceID) {
		return TraceContext{}, false
	}

	span, ok := decodeB3SpanID(parts[1])
	if !ok {
		return TraceContext{}, false
	}

	sampled := false
	if len(parts) >= 3 {
		switch parts[2] {
		case "1", "d":
			sampled = true
		case "0":
			sampled = false
		default:
			return TraceContext{}, false
		}
	}

	// The incoming span-id is ALWAYS our server span's parent (SPEC §11
	// D-TRACE-ZIPKIN-B3). The optional 4th field is the caller's OWN parent (our
	// grandparent): validate its framing but accept-and-ignore it.
	parent := span
	if len(parts) == 4 {
		if _, ok := decodeB3SpanID(parts[3]); !ok {
			return TraceContext{}, false
		}
	}

	ctx.ParentID = parent
	ctx.Sampled = sampled
	return ctx, true
}

func extractB3Multi(h http.Header) (TraceContext, bool) {
	tid := h.Get("X-B3-TraceId")
	sid := h.Get("X-B3-SpanId")
	if tid == "" || sid == "" {
		return TraceContext{}, false
	}

	var ctx TraceContext
	if !decodeB3TraceID(tid, &ctx.TraceID) {
		return TraceContext{}, false
	}

	span, ok := decodeB3SpanID(sid)
	if !ok {
		return TraceContext{}, false
	}

	// The incoming X-B3-SpanId is ALWAYS our server span's parent (SPEC §11
	// D-TRACE-ZIPKIN-B3). X-B3-ParentSpanId is the caller's OWN parent (our
	// grandparent): validate its framing but accept-and-ignore it.
	parent := span
	if pid := h.Get("X-B3-ParentSpanId"); pid != "" {
		if _, ok := decodeB3SpanID(pid); !ok {
			return TraceContext{}, false
		}
	}

	ctx.ParentID = parent
	ctx.Sampled = h.Get("X-B3-Sampled") == "1" || h.Get("X-B3-Flags") == "1"
	return ctx, true
}

// decodeB3TraceID decodes a 16-hex (64-bit, low 8 bytes) or 32-hex (128-bit) B3
// trace-id into dst, rejecting bad hex/length and the all-zero trace-id.
func decodeB3TraceID(s string, dst *[16]byte) bool {
	var t [16]byte
	switch len(s) {
	case 16:
		if _, err := hex.Decode(t[8:], []byte(s)); err != nil {
			return false
		}
	case 32:
		if _, err := hex.Decode(t[:], []byte(s)); err != nil {
			return false
		}
	default:
		return false
	}
	if t == ([16]byte{}) {
		return false
	}
	*dst = t
	return true
}

// decodeB3SpanID decodes a 16-hex B3 span-id, rejecting bad hex/length and the
// all-zero span-id.
func decodeB3SpanID(s string) ([8]byte, bool) {
	var b [8]byte
	if len(s) != 16 {
		return b, false
	}
	if _, err := hex.Decode(b[:], []byte(s)); err != nil {
		return b, false
	}
	if b == ([8]byte{}) {
		return b, false
	}
	return b, true
}

// InjectB3 writes the multi-header X-B3-* set onto h for the decision d. The
// outbound identity (trace-id hex width per id128; the fresh-root / shared-reuse /
// continued-with-parent span-id derivation) is delegated to the shared zipkinIdentity
// helper so the B3 injector and the Zipkin span encoder agree. X-B3-Sampled is "1"/"0"
// per d.Sample. Hex is lowercase.
func InjectB3(h http.Header, d Decision, id128, shared bool) {
	traceID, spanID, parentID, _ := zipkinIdentity(identityInput{
		TraceID:      d.TraceID,
		SpanID:       d.SpanID,
		ParentSpanID: d.ParentSpanID,
	}, id128, shared)

	h.Set("X-B3-TraceId", traceID)
	h.Set("X-B3-SpanId", spanID)
	if d.Sample {
		h.Set("X-B3-Sampled", "1")
	} else {
		h.Set("X-B3-Sampled", "0")
	}
	if parentID != "" {
		h.Set("X-B3-ParentSpanId", parentID)
	}
}

// traceIDHex renders a B3 trace-id: the full 32-hex 128-bit id, or the low-64
// 16-hex id when id128 is false.
func traceIDHex(t [16]byte, id128 bool) string {
	if id128 {
		return hex.EncodeToString(t[:])
	}
	return hex.EncodeToString(t[8:])
}

// spanIDHex renders a B3 span-id as 16 lowercase hex chars.
func spanIDHex(s [8]byte) string {
	return hex.EncodeToString(s[:])
}
