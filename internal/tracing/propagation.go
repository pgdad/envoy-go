package tracing

import (
	"encoding/hex"
	"net/http"
	"strings"
)

// TraceContext is an extracted W3C trace-context: the inbound trace identity plus
// the sampling decision and opaque vendor state. ParentID holds the upstream
// parent span-id (the traceparent's parent-id field).
type TraceContext struct {
	TraceID    [16]byte
	ParentID   [8]byte
	Sampled    bool
	TraceState string
}

// ExtractTraceparent parses a W3C "traceparent" header from h and returns the
// decoded TraceContext. The wire format is
// "00-<32 hex trace-id>-<16 hex parent-id>-<2 hex flags>" (version "00"; flags
// bit 0 = sampled). An all-zero trace-id or all-zero parent-id is invalid and any
// framing/hex/version/length failure yields (TraceContext{}, false). The opaque
// "tracestate" header (if any) is carried through verbatim in TraceState.
func ExtractTraceparent(h http.Header) (TraceContext, bool) {
	v := h.Get("Traceparent")
	if v == "" {
		return TraceContext{}, false
	}
	parts := strings.Split(v, "-")
	if len(parts) != 4 {
		return TraceContext{}, false
	}
	if parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return TraceContext{}, false
	}

	var ctx TraceContext
	if _, err := hex.Decode(ctx.TraceID[:], []byte(parts[1])); err != nil {
		return TraceContext{}, false
	}
	if _, err := hex.Decode(ctx.ParentID[:], []byte(parts[2])); err != nil {
		return TraceContext{}, false
	}
	flags, err := hex.DecodeString(parts[3])
	if err != nil {
		return TraceContext{}, false
	}
	if ctx.TraceID == ([16]byte{}) || ctx.ParentID == ([8]byte{}) {
		return TraceContext{}, false
	}

	ctx.Sampled = flags[0]&0x01 == 1
	ctx.TraceState = h.Get("Tracestate")
	return ctx, true
}

// InjectTraceparent writes a W3C "traceparent" header onto h encoding traceID,
// spanID (as the outbound parent-id) and the sampled flag (bit 0). Hex is
// lowercase. A non-empty tracestate is written verbatim to the "tracestate"
// header; an empty tracestate writes no header.
func InjectTraceparent(h http.Header, traceID [16]byte, spanID [8]byte, sampled bool, tracestate string) {
	flags := "00"
	if sampled {
		flags = "01"
	}
	h.Set("Traceparent", "00-"+hex.EncodeToString(traceID[:])+"-"+hex.EncodeToString(spanID[:])+"-"+flags)
	if tracestate != "" {
		h.Set("Tracestate", tracestate)
	}
}
