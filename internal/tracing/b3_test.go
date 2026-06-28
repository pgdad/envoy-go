package tracing

import (
	"net/http"
	"testing"
)

// --- ExtractB3: single "b3" header -----------------------------------------

func TestB3ExtractSingle64BitSampled(t *testing.T) {
	// 64-bit trace-id occupies the LOW 8 bytes of the [16]byte; high 8 stay zero.
	h := http.Header{}
	h.Set("b3", "0102030405060708-1112131415161718-1")
	ctx, ok := ExtractB3(h)
	if !ok {
		t.Fatalf("ExtractB3 ok = false, want true")
	}
	wantTrace := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	wantParent := [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	if ctx.TraceID != wantTrace {
		t.Fatalf("TraceID = %x, want %x", ctx.TraceID, wantTrace)
	}
	if ctx.ParentID != wantParent {
		t.Fatalf("ParentID = %x, want %x (the span-id field)", ctx.ParentID, wantParent)
	}
	if !ctx.Sampled {
		t.Fatalf("Sampled = false, want true")
	}
	if ctx.TraceState != "" {
		t.Fatalf("TraceState = %q, want empty (B3 leaves it empty)", ctx.TraceState)
	}
}

func TestB3ExtractSingle128BitWithParent(t *testing.T) {
	// 128-bit 4-field form: trace-id is the full 32 hex. Per SPEC §11
	// D-TRACE-ZIPKIN-B3, the incoming SPAN-ID (field[1]) is ALWAYS our server
	// span's parent; the 4th field is the caller's OWN parent (our grandparent),
	// accepted-but-ignored. The 4th field value (0x21..) is deliberately DISTINCT
	// from the span-id (0x11..) so this assertion proves the span-id (not the 4th
	// field) becomes ParentID.
	h := http.Header{}
	h.Set("b3", "0102030405060708090a0b0c0d0e0f10-1112131415161718-1-2122232425262728")
	ctx, ok := ExtractB3(h)
	if !ok {
		t.Fatalf("ExtractB3 ok = false, want true")
	}
	wantTrace := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	wantParent := [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	if ctx.TraceID != wantTrace {
		t.Fatalf("TraceID = %x, want %x", ctx.TraceID, wantTrace)
	}
	if ctx.ParentID != wantParent {
		t.Fatalf("ParentID = %x, want %x (the incoming span-id, NOT the 4th field)", ctx.ParentID, wantParent)
	}
	if !ctx.Sampled {
		t.Fatalf("Sampled = false, want true")
	}
}

func TestB3ExtractSingleSampledZero(t *testing.T) {
	h := http.Header{}
	h.Set("b3", "0102030405060708-1112131415161718-0")
	ctx, ok := ExtractB3(h)
	if !ok {
		t.Fatalf("ExtractB3 ok = false, want true")
	}
	if ctx.Sampled {
		t.Fatalf("Sampled = true, want false (sampled=0)")
	}
}

func TestB3ExtractSingleDebugForcesSampled(t *testing.T) {
	h := http.Header{}
	h.Set("b3", "0102030405060708-1112131415161718-d")
	ctx, ok := ExtractB3(h)
	if !ok {
		t.Fatalf("ExtractB3 ok = false, want true")
	}
	if !ctx.Sampled {
		t.Fatalf("Sampled = false, want true (debug forces sampled)")
	}
}

func TestB3ExtractSingleTwoFieldSamplingDeferred(t *testing.T) {
	// 2-field traceid-spanid form: accepted with Sampled:false (sampling deferred).
	h := http.Header{}
	h.Set("b3", "0102030405060708-1112131415161718")
	ctx, ok := ExtractB3(h)
	if !ok {
		t.Fatalf("ExtractB3 ok = false, want true")
	}
	if ctx.Sampled {
		t.Fatalf("Sampled = true, want false (2-field defers sampling)")
	}
	wantParent := [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	if ctx.ParentID != wantParent {
		t.Fatalf("ParentID = %x, want %x (the span-id field)", ctx.ParentID, wantParent)
	}
}

func TestB3ExtractSingleMalformed(t *testing.T) {
	cases := []string{
		"",        // empty
		"garbage", // junk, 1 field
		"0102030405060708-1112131415161718-1-2122232425262728-extra",           // 5 fields
		"0102030405060708-1112131415161718-x",                                  // bad sampled token
		"010203040506070g-1112131415161718-1",                                  // non-hex trace-id
		"0102030405060708-111213141516171g-1",                                  // non-hex span-id
		"01020304050607-1112131415161718-1",                                    // short trace-id (14 hex)
		"0102030405060708-11121314151617-1",                                    // short span-id (14 hex)
		"0000000000000000-1112131415161718-1",                                  // all-zero trace-id
		"0102030405060708-0000000000000000-1",                                  // all-zero span-id
		"0102030405060708090a0b0c0d0e0f10-1112131415161718-1-0000000000000000", // all-zero parent
	}
	for _, v := range cases {
		h := http.Header{}
		h.Set("b3", v)
		if ctx, ok := ExtractB3(h); ok {
			t.Fatalf("ExtractB3 ok = true for malformed b3 %q (ctx %+v), want false", v, ctx)
		}
	}
}

// --- ExtractB3: multi X-B3-* headers ---------------------------------------

func TestB3ExtractMultiBasic(t *testing.T) {
	h := http.Header{}
	h.Set("X-B3-TraceId", "0102030405060708090a0b0c0d0e0f10")
	h.Set("X-B3-SpanId", "1112131415161718")
	h.Set("X-B3-Sampled", "1")
	ctx, ok := ExtractB3(h)
	if !ok {
		t.Fatalf("ExtractB3 ok = false, want true")
	}
	wantTrace := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	if ctx.TraceID != wantTrace {
		t.Fatalf("TraceID = %x, want %x", ctx.TraceID, wantTrace)
	}
	// No X-B3-ParentSpanId => ParentID falls back to the span-id.
	wantParent := [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	if ctx.ParentID != wantParent {
		t.Fatalf("ParentID = %x, want %x (span-id fallback)", ctx.ParentID, wantParent)
	}
	if !ctx.Sampled {
		t.Fatalf("Sampled = false, want true")
	}
}

func TestB3ExtractMultiWithParent(t *testing.T) {
	// Per SPEC §11 D-TRACE-ZIPKIN-B3 the incoming X-B3-SpanId is ALWAYS our
	// server span's parent; X-B3-ParentSpanId is the caller's OWN parent (our
	// grandparent), accepted-but-ignored. X-B3-ParentSpanId (0x21..) is
	// deliberately DISTINCT from X-B3-SpanId (0x11..) so this proves the span-id
	// (not X-B3-ParentSpanId) becomes ParentID.
	h := http.Header{}
	h.Set("X-B3-TraceId", "0102030405060708")
	h.Set("X-B3-SpanId", "1112131415161718")
	h.Set("X-B3-ParentSpanId", "2122232425262728")
	h.Set("X-B3-Sampled", "1")
	ctx, ok := ExtractB3(h)
	if !ok {
		t.Fatalf("ExtractB3 ok = false, want true")
	}
	wantParent := [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	if ctx.ParentID != wantParent {
		t.Fatalf("ParentID = %x, want %x (the incoming X-B3-SpanId, NOT X-B3-ParentSpanId)", ctx.ParentID, wantParent)
	}
	// 64-bit trace-id in low 8 bytes.
	wantTrace := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	if ctx.TraceID != wantTrace {
		t.Fatalf("TraceID = %x, want %x", ctx.TraceID, wantTrace)
	}
}

func TestB3ExtractMultiFlagsDebugSampled(t *testing.T) {
	// X-B3-Flags:1 (debug) forces Sampled even with X-B3-Sampled absent.
	h := http.Header{}
	h.Set("X-B3-TraceId", "0102030405060708")
	h.Set("X-B3-SpanId", "1112131415161718")
	h.Set("X-B3-Flags", "1")
	ctx, ok := ExtractB3(h)
	if !ok {
		t.Fatalf("ExtractB3 ok = false, want true")
	}
	if !ctx.Sampled {
		t.Fatalf("Sampled = false, want true (X-B3-Flags debug)")
	}
}

func TestB3ExtractMultiSampledZero(t *testing.T) {
	h := http.Header{}
	h.Set("X-B3-TraceId", "0102030405060708")
	h.Set("X-B3-SpanId", "1112131415161718")
	h.Set("X-B3-Sampled", "0")
	ctx, ok := ExtractB3(h)
	if !ok {
		t.Fatalf("ExtractB3 ok = false, want true")
	}
	if ctx.Sampled {
		t.Fatalf("Sampled = true, want false")
	}
}

func TestB3ExtractMultiMissingRequired(t *testing.T) {
	cases := []struct {
		name string
		set  map[string]string
	}{
		{"no trace-id", map[string]string{"X-B3-SpanId": "1112131415161718", "X-B3-Sampled": "1"}},
		{"no span-id", map[string]string{"X-B3-TraceId": "0102030405060708", "X-B3-Sampled": "1"}},
		{"bad hex trace-id", map[string]string{"X-B3-TraceId": "010203040506070g", "X-B3-SpanId": "1112131415161718", "X-B3-Sampled": "1"}},
		{"short span-id", map[string]string{"X-B3-TraceId": "0102030405060708", "X-B3-SpanId": "11121314151617", "X-B3-Sampled": "1"}},
		{"all-zero trace-id", map[string]string{"X-B3-TraceId": "0000000000000000", "X-B3-SpanId": "1112131415161718", "X-B3-Sampled": "1"}},
		{"all-zero span-id", map[string]string{"X-B3-TraceId": "0102030405060708", "X-B3-SpanId": "0000000000000000", "X-B3-Sampled": "1"}},
		{"empty", map[string]string{}},
	}
	for _, tc := range cases {
		h := http.Header{}
		for k, v := range tc.set {
			h.Set(k, v)
		}
		if ctx, ok := ExtractB3(h); ok {
			t.Fatalf("%s: ExtractB3 ok = true (ctx %+v), want false", tc.name, ctx)
		}
	}
}

func TestB3ExtractSinglePrecedenceOverMulti(t *testing.T) {
	// When BOTH b3 and X-B3-* are present, the single b3 header wins.
	h := http.Header{}
	h.Set("b3", "0102030405060708-1112131415161718-1")
	h.Set("X-B3-TraceId", "aaaaaaaaaaaaaaaa")
	h.Set("X-B3-SpanId", "bbbbbbbbbbbbbbbb")
	h.Set("X-B3-Sampled", "0")
	ctx, ok := ExtractB3(h)
	if !ok {
		t.Fatalf("ExtractB3 ok = false, want true")
	}
	wantTrace := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	if ctx.TraceID != wantTrace {
		t.Fatalf("TraceID = %x, want %x (b3 wins)", ctx.TraceID, wantTrace)
	}
	if !ctx.Sampled {
		t.Fatalf("Sampled = false, want true (b3's sampled=1 wins over X-B3-Sampled=0)")
	}
}

// --- InjectB3: multi-header set --------------------------------------------

func TestB3InjectFreshRoot(t *testing.T) {
	h := http.Header{}
	d := Decision{
		TraceID: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:  [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		Sample:  true,
		// ParentSpanID zero => fresh root.
	}
	InjectB3(h, d, false, false)
	if got, want := h.Get("X-B3-TraceId"), "090a0b0c0d0e0f10"; got != want {
		t.Fatalf("X-B3-TraceId = %q, want %q (low-64, 16 chars)", got, want)
	}
	if got, want := h.Get("X-B3-SpanId"), "1112131415161718"; got != want {
		t.Fatalf("X-B3-SpanId = %q, want %q", got, want)
	}
	if got, want := h.Get("X-B3-Sampled"), "1"; got != want {
		t.Fatalf("X-B3-Sampled = %q, want %q", got, want)
	}
	if got := h.Get("X-B3-ParentSpanId"); got != "" {
		t.Fatalf("X-B3-ParentSpanId = %q, want empty (fresh root)", got)
	}
}

func TestB3InjectContinuedNotShared(t *testing.T) {
	h := http.Header{}
	d := Decision{
		TraceID:      [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:       [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		ParentSpanID: [8]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28},
		Sample:       true,
	}
	InjectB3(h, d, false, false)
	if got, want := h.Get("X-B3-SpanId"), "1112131415161718"; got != want {
		t.Fatalf("X-B3-SpanId = %q, want %q (fresh span)", got, want)
	}
	if got, want := h.Get("X-B3-ParentSpanId"), "2122232425262728"; got != want {
		t.Fatalf("X-B3-ParentSpanId = %q, want %q (incoming span-id)", got, want)
	}
}

func TestB3InjectContinuedShared(t *testing.T) {
	h := http.Header{}
	d := Decision{
		TraceID:      [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:       [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		ParentSpanID: [8]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28},
		Sample:       true,
	}
	InjectB3(h, d, false, true)
	if got, want := h.Get("X-B3-SpanId"), "2122232425262728"; got != want {
		t.Fatalf("X-B3-SpanId = %q, want %q (REUSED incoming span-id)", got, want)
	}
	if got := h.Get("X-B3-ParentSpanId"); got != "" {
		t.Fatalf("X-B3-ParentSpanId = %q, want empty (shared)", got)
	}
}

func TestB3Inject128Bit(t *testing.T) {
	h := http.Header{}
	d := Decision{
		TraceID: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:  [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		Sample:  true,
	}
	InjectB3(h, d, true, false)
	if got, want := h.Get("X-B3-TraceId"), "0102030405060708090a0b0c0d0e0f10"; got != want {
		t.Fatalf("X-B3-TraceId = %q, want %q (full 32 hex)", got, want)
	}
}

func TestB3InjectNotSampled(t *testing.T) {
	h := http.Header{}
	d := Decision{
		TraceID: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:  [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		Sample:  false,
	}
	InjectB3(h, d, false, false)
	if got, want := h.Get("X-B3-Sampled"), "0"; got != want {
		t.Fatalf("X-B3-Sampled = %q, want %q", got, want)
	}
}
