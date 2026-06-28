package tracing

import (
	"net/http"
	"testing"
)

func TestPropagationExtractValidSampled(t *testing.T) {
	h := http.Header{"Traceparent": []string{"00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01"}}
	ctx, ok := ExtractTraceparent(h)
	if !ok {
		t.Fatalf("ExtractTraceparent ok = false, want true")
	}
	wantTrace := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	wantParent := [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	if ctx.TraceID != wantTrace {
		t.Fatalf("TraceID = %x, want %x", ctx.TraceID, wantTrace)
	}
	if ctx.ParentID != wantParent {
		t.Fatalf("ParentID = %x, want %x", ctx.ParentID, wantParent)
	}
	if !ctx.Sampled {
		t.Fatalf("Sampled = false, want true")
	}
}

func TestPropagationExtractFlagsNotSampled(t *testing.T) {
	h := http.Header{"Traceparent": []string{"00-0102030405060708090a0b0c0d0e0f10-1112131415161718-00"}}
	ctx, ok := ExtractTraceparent(h)
	if !ok {
		t.Fatalf("ExtractTraceparent ok = false, want true")
	}
	if ctx.Sampled {
		t.Fatalf("Sampled = true, want false")
	}
}

func TestPropagationExtractAllZeroTraceIDInvalid(t *testing.T) {
	h := http.Header{"Traceparent": []string{"00-00000000000000000000000000000000-1112131415161718-01"}}
	if _, ok := ExtractTraceparent(h); ok {
		t.Fatalf("ExtractTraceparent ok = true for all-zero trace-id, want false")
	}
}

func TestPropagationExtractAllZeroParentIDInvalid(t *testing.T) {
	h := http.Header{"Traceparent": []string{"00-0102030405060708090a0b0c0d0e0f10-0000000000000000-01"}}
	if _, ok := ExtractTraceparent(h); ok {
		t.Fatalf("ExtractTraceparent ok = true for all-zero parent-id, want false")
	}
}

func TestPropagationExtractMalformed(t *testing.T) {
	cases := []string{
		"00-0102030405060708090a0b0c0d0e0f10-1112131415161718",          // wrong field count (3)
		"00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01-extra", // wrong field count (5)
		"99-0102030405060708090a0b0c0d0e0f10-1112131415161718-01",       // wrong version
		"00-0102030405060708090a0b0c0d0e0f1g-1112131415161718-01",       // non-hex trace-id
		"00-0102030405060708090a0b0c0d0e0f-1112131415161718-01",         // short trace-id
		"00-0102030405060708090a0b0c0d0e0f10-11121314151617-01",         // short parent-id
		"00-0102030405060708090a0b0c0d0e0f10-1112131415161718-0",        // short flags
		"00-0102030405060708090a0b0c0d0e0f10-1112131415161718-zz",       // non-hex flags
		"",        // empty
		"garbage", // junk
	}
	for _, v := range cases {
		h := http.Header{"Traceparent": []string{v}}
		if _, ok := ExtractTraceparent(h); ok {
			t.Fatalf("ExtractTraceparent ok = true for malformed %q, want false", v)
		}
	}
}

func TestPropagationExtractMissingHeader(t *testing.T) {
	h := http.Header{}
	if _, ok := ExtractTraceparent(h); ok {
		t.Fatalf("ExtractTraceparent ok = true for missing header, want false")
	}
}

func TestPropagationExtractCaseInsensitiveKey(t *testing.T) {
	// Lowercase key; http.Header.Get canonicalizes to "Traceparent".
	h := http.Header{}
	h.Set("traceparent", "00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01")
	if _, ok := ExtractTraceparent(h); !ok {
		t.Fatalf("ExtractTraceparent ok = false for lowercase key, want true")
	}
}

func TestPropagationExtractTraceState(t *testing.T) {
	h := http.Header{}
	h.Set("Traceparent", "00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01")
	h.Set("Tracestate", "vendor=abc,foo=bar")
	ctx, ok := ExtractTraceparent(h)
	if !ok {
		t.Fatalf("ExtractTraceparent ok = false, want true")
	}
	if ctx.TraceState != "vendor=abc,foo=bar" {
		t.Fatalf("TraceState = %q, want %q", ctx.TraceState, "vendor=abc,foo=bar")
	}
}

func TestPropagationInjectSampled(t *testing.T) {
	h := http.Header{}
	traceID := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	spanID := [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	InjectTraceparent(h, traceID, spanID, true, "")
	want := "00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01"
	if got := h.Get("Traceparent"); got != want {
		t.Fatalf("Traceparent = %q, want %q", got, want)
	}
	if h.Get("Tracestate") != "" {
		t.Fatalf("Tracestate = %q, want empty", h.Get("Tracestate"))
	}
}

func TestPropagationInjectNotSampled(t *testing.T) {
	h := http.Header{}
	traceID := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	spanID := [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	InjectTraceparent(h, traceID, spanID, false, "")
	want := "00-0102030405060708090a0b0c0d0e0f10-1112131415161718-00"
	if got := h.Get("Traceparent"); got != want {
		t.Fatalf("Traceparent = %q, want %q", got, want)
	}
}

func TestPropagationInjectTraceState(t *testing.T) {
	h := http.Header{}
	traceID := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	spanID := [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	InjectTraceparent(h, traceID, spanID, true, "vendor=abc")
	if got := h.Get("Tracestate"); got != "vendor=abc" {
		t.Fatalf("Tracestate = %q, want %q", got, "vendor=abc")
	}
}

func TestPropagationRoundTrip(t *testing.T) {
	h := http.Header{}
	traceID := [16]byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c}
	spanID := [8]byte{0xca, 0xfe, 0xba, 0xbe, 0x11, 0x22, 0x33, 0x44}
	InjectTraceparent(h, traceID, spanID, true, "")
	ctx, ok := ExtractTraceparent(h)
	if !ok {
		t.Fatalf("round-trip ExtractTraceparent ok = false, want true")
	}
	if ctx.TraceID != traceID {
		t.Fatalf("round-trip TraceID = %x, want %x", ctx.TraceID, traceID)
	}
	if ctx.ParentID != spanID {
		t.Fatalf("round-trip ParentID = %x, want spanID %x", ctx.ParentID, spanID)
	}
	if !ctx.Sampled {
		t.Fatalf("round-trip Sampled = false, want true")
	}
}

func FuzzExtractTraceparent(f *testing.F) {
	f.Add("00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01")
	f.Add("")
	f.Add("garbage-not-a-traceparent")
	f.Fuzz(func(t *testing.T, v string) {
		h := http.Header{}
		h.Set("Traceparent", v)
		ctx, ok := ExtractTraceparent(h) // must not panic
		if ok {
			if ctx.TraceID == ([16]byte{}) || ctx.ParentID == ([8]byte{}) {
				t.Fatalf("ok parse with zero id: %q", v)
			}
		}
	})
}
