package hcm

// tracing_zipkin_dispatch_test.go — Phase 46.2 Task 9: the HCM dispatch seam is
// provider-aware. When tracingConfig.Provider == ProviderZipkin the dispatch
// extracts/injects B3 (x-b3-*) instead of the W3C traceparent, and the per-request
// span carries the request :authority (Span.Authority). The OTel path is unchanged
// (traceparent regression), and the no-tracing path stays byte-stable.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"net/http"
	"testing"

	"golang.org/x/net/http2/hpack"

	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/tracing"
)

// mkZipkinTracingFilter is the Zipkin analog of mkTracingFilter: a full-sampling
// TracingConfig with Provider==ProviderZipkin + Zipkin settings, a fresh counter
// set, the supplied deterministic rng, and a fake exporter for span capture.
func mkZipkinTracingFilter(t *testing.T, tt *routeTable, rng tracing.RandSource, exp tracing.Exporter, id128, shared bool) (*Filter, *stats.Registry) {
	t.Helper()
	f := mkFilterForTable(t, tt)
	reg := stats.NewRegistry()
	counters, err := tracing.RegisterHCMCounters(reg, "ingress_http")
	if err != nil {
		t.Fatalf("RegisterHCMCounters: %v", err)
	}
	f.tracingConfig = &tracing.TracingConfig{
		ClientSampling:  100,
		RandomSampling:  100,
		OverallSampling: 100,
		Provider:        tracing.ProviderZipkin,
		Zipkin:          &tracing.ZipkinSettings{TraceID128Bit: id128, SharedSpanContext: shared},
	}
	f.tracingCounters = counters
	f.rng = rng
	f.exporter = exp
	return f, reg
}

// --- H1 (connection.go dispatchRequest) -------------------------------------

// TestDispatchRequest_TracingZipkin_SampledInjectsB3: a Zipkin-provider HCM with
// NO incoming B3 ⇒ the dispatched upstream request carries X-B3-TraceId/SpanId/
// Sampled (NOT traceparent); the recorded span's Authority == r.Host.
func TestDispatchRequest_TracingZipkin_SampledInjectsB3(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	fe := &fakeExporter{}
	f, reg := mkZipkinTracingFilter(t, tt, fakeTraceRand{f: 0, b: 0xab}, fe, true, true)

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	req.Host = "zipkin.example.com"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := f.dispatchRequest(context.Background(), nil, req, bw); err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}

	if got := req.Header.Get("X-B3-Traceid"); len(got) != 32 {
		t.Errorf("X-B3-TraceId = %q, want a 32-hex id (id128)", got)
	}
	if got := req.Header.Get("X-B3-Spanid"); len(got) != 16 {
		t.Errorf("X-B3-SpanId = %q, want a 16-hex id", got)
	}
	if got := req.Header.Get("X-B3-Sampled"); got != "1" {
		t.Errorf("X-B3-Sampled = %q, want 1", got)
	}
	if got := req.Header.Get("Traceparent"); got != "" {
		t.Errorf("Traceparent = %q, want empty on the Zipkin path", got)
	}
	rid := req.Header.Get("X-Request-Id")
	if len(rid) != 36 {
		t.Errorf("X-Request-Id = %q, want a 36-char id", rid)
	}

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Authority != "zipkin.example.com" {
		t.Errorf("span Authority = %q, want zipkin.example.com", spans[0].Authority)
	}
	if got := tracingCounterValue(t, reg, "random_sampling"); got != 1 {
		t.Errorf("random_sampling = %d, want 1", got)
	}
}

// TestDispatchRequest_TracingZipkin_Continued: an incoming single b3
// "<trace>-<span>-1" ⇒ the recorded span continues that trace, and the upstream
// X-B3-TraceId == the incoming trace.
func TestDispatchRequest_TracingZipkin_Continued(t *testing.T) {
	const fixedTrace = "0af7651916cd43dd8448eb211c80319c" // 32-hex
	const fixedSpan = "b7ad6b7169203331"                  // 16-hex
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	fe := &fakeExporter{}
	f, _ := mkZipkinTracingFilter(t, tt, fakeTraceRand{f: 0, b: 0x11}, fe, true, true)

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	req.Host = "zipkin.example.com"
	req.Header.Set("B3", fixedTrace+"-"+fixedSpan+"-1")
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := f.dispatchRequest(context.Background(), nil, req, bw); err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}

	if got := req.Header.Get("X-B3-Traceid"); got != fixedTrace {
		t.Errorf("forwarded X-B3-TraceId = %q, want continued %s", got, fixedTrace)
	}

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := hex.EncodeToString(spans[0].TraceID[:]); got != fixedTrace {
		t.Errorf("span TraceID = %q, want continued %s", got, fixedTrace)
	}
}

// TestDispatchRequest_TracingOTel_StillInjectsTraceparent: regression — an OTel
// provider config still injects the W3C traceparent (and NO x-b3-*).
func TestDispatchRequest_TracingOTel_StillInjectsTraceparent(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f, _ := mkTracingFilter(t, tt, fakeTraceRand{f: 0, b: 0xcd})
	f.tracingConfig.Provider = tracing.ProviderOTel // explicit OTel

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := f.dispatchRequest(context.Background(), nil, req, bw); err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}

	if tp := req.Header.Get("Traceparent"); !traceparentMatches(tp, true) {
		t.Errorf("Traceparent = %q, want 00-<32hex>-<16hex>-01 on the OTel path", tp)
	}
	if got := req.Header.Get("X-B3-Traceid"); got != "" {
		t.Errorf("X-B3-TraceId = %q, want empty on the OTel path", got)
	}
}

// --- H2 (h2dispatch.go WriteH2) ---------------------------------------------

// TestWriteH2_TracingZipkin_SampledInjectsB3: the H2 analog — x-b3-* write-back
// via upsertH2Header (NOT traceparent); the recorded span's Authority ==
// req.Authority.
func TestWriteH2_TracingZipkin_SampledInjectsB3(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	fe := &fakeExporter{}
	f, reg := mkZipkinTracingFilter(t, tt, fakeTraceRand{f: 0, b: 0x33}, fe, true, true)

	var captured h2.H2Request
	hreq, _ := http.NewRequest("GET", "/health", nil)
	hreq.Proto = "HTTP/2.0"
	c := &chainDispatchAction{f: f, action: captureH2Action(&captured), req: hreq, routeIdx: 0}

	h2req := h2.H2Request{Method: "GET", Path: "/health", Scheme: "https", Authority: "zipkin.h2.example.com"}
	if err := c.WriteH2(context.Background(), h2req, &captureH2Writer{}); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	if got := h2HeaderValue(captured, "x-b3-traceid"); len(got) != 32 {
		t.Errorf("forwarded x-b3-traceid = %q, want a 32-hex id (id128)", got)
	}
	if got := h2HeaderValue(captured, "x-b3-spanid"); len(got) != 16 {
		t.Errorf("forwarded x-b3-spanid = %q, want a 16-hex id", got)
	}
	if got := h2HeaderValue(captured, "x-b3-sampled"); got != "1" {
		t.Errorf("forwarded x-b3-sampled = %q, want 1", got)
	}
	if got := h2HeaderValue(captured, "traceparent"); got != "" {
		t.Errorf("forwarded traceparent = %q, want empty on the Zipkin path", got)
	}
	if got := h2HeaderValue(captured, "x-request-id"); len(got) != 36 {
		t.Errorf("forwarded x-request-id = %q, want a 36-char id", got)
	}

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Authority != "zipkin.h2.example.com" {
		t.Errorf("span Authority = %q, want zipkin.h2.example.com", spans[0].Authority)
	}
	if got := tracingCounterValue(t, reg, "random_sampling"); got != 1 {
		t.Errorf("random_sampling = %d, want 1", got)
	}
}

// TestWriteH2_TracingZipkin_Continued: the H2 analog of the continuation — an
// incoming single b3 header continues the trace onto the upstream X-B3-TraceId.
func TestWriteH2_TracingZipkin_Continued(t *testing.T) {
	const fixedTrace = "0af7651916cd43dd8448eb211c80319c"
	const fixedSpan = "b7ad6b7169203331"
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	fe := &fakeExporter{}
	f, _ := mkZipkinTracingFilter(t, tt, fakeTraceRand{f: 0, b: 0x44}, fe, true, true)

	var captured h2.H2Request
	hreq, _ := http.NewRequest("GET", "/health", nil)
	hreq.Proto = "HTTP/2.0"
	c := &chainDispatchAction{f: f, action: captureH2Action(&captured), req: hreq, routeIdx: 0}

	h2req := h2.H2Request{
		Method:    "GET",
		Path:      "/health",
		Scheme:    "https",
		Authority: "zipkin.h2.example.com",
		Headers:   []hpack.HeaderField{{Name: "b3", Value: fixedTrace + "-" + fixedSpan + "-1"}},
	}
	if err := c.WriteH2(context.Background(), h2req, &captureH2Writer{}); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	if got := h2HeaderValue(captured, "x-b3-traceid"); got != fixedTrace {
		t.Errorf("forwarded x-b3-traceid = %q, want continued %s", got, fixedTrace)
	}
	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := hex.EncodeToString(spans[0].TraceID[:]); got != fixedTrace {
		t.Errorf("span TraceID = %q, want continued %s", got, fixedTrace)
	}
}
