package hcm

// span_emit_test.go — Phase 46.1b Task 8: unit tests for the span-end wiring
// that carries a *tracing.Decision from the dispatch seam to emitAccessLog /
// emitAccessLogH2 and builds+exports the per-request ingress span there.
//
// Test cases (H1 + H2 mirrors):
//   - sampled-exports:    one span reaches the fake exporter; Name=="ingress",
//     Kind==SERVER, start<end, TraceID/SpanID match the decision.
//   - not-sampled-no-export: decision.Sample==false => no span exported.
//   - continued:          incoming traceparent ⇒ span.ParentSpanID==incoming parent.
//   - cancel-no-span:     statusCode==0 (ctx-cancel sentinel) ⇒ no span.
//   - export-without-access-log: span fires even when f.accessLog is empty (the
//     CRITICAL AMEND-TRACE-SPANEND-SEAM ordering requirement).
//   - byte-stable/no-tracing: nil traceDecision + nil exporter ⇒ no span, no panic.

import (
	"net/http"
	"sync"
	"testing"
	"time"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/tracing"
)

// ── fake exporter ─────────────────────────────────────────────────────────────

type fakeExporter struct {
	mu    sync.Mutex
	spans []*tracing.Span
}

func (fe *fakeExporter) Export(s *tracing.Span) {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.spans = append(fe.spans, s)
}

func (fe *fakeExporter) Close() error { return nil }

func (fe *fakeExporter) captured() []*tracing.Span {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	out := make([]*tracing.Span, len(fe.spans))
	copy(out, fe.spans)
	return out
}

// ── helpers ────────────────────────────────────────────────────────────────────

// newTracingFilter builds a minimal *Filter with the given exporter + config.
// No access-log sinks are installed by default so the export-without-access-log
// test verifies the AMEND-TRACE-SPANEND-SEAM ordering directly.
func newTracingFilter(t *testing.T, exp tracing.Exporter, cfg *tracing.TracingConfig) *Filter {
	t.Helper()
	r := stats.NewRegistry()
	prefix := "http.test_span."
	var counters *tracing.HCMCounters
	if cfg != nil {
		var err error
		counters, err = tracing.RegisterHCMCounters(r, "test_span")
		if err != nil {
			t.Fatalf("RegisterHCMCounters: %v", err)
		}
	}
	return &Filter{
		statPrefix:        "test_span",
		downstreamRqTotal: r.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   r.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   r.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   r.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   r.NewCounter(prefix + "downstream_rq_5xx"),
		tracingConfig:     cfg,
		tracingCounters:   counters,
		exporter:          exp,
	}
}

// knownDecision returns a *tracing.Decision with fixed, recognizable IDs and
// Sample==true.  Used to verify the span carries the SAME ids as the decision.
func knownDecision() *tracing.Decision {
	return &tracing.Decision{
		Sample:    true,
		TraceID:   [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:    [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
		RequestID: "test-req-id",
	}
}

// ── H1 tests ──────────────────────────────────────────────────────────────────

// TestSpanEmit_H1_SampledExports: sampled decision ⇒ ONE span with Name==ingress,
// Kind==SERVER, start<end, TraceID+SpanID matching the decision.
func TestSpanEmit_H1_SampledExports(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 100, OverallSampling: 100, ClientSampling: 100}
	f := newTracingFilter(t, fe, cfg)

	req, _ := http.NewRequest("GET", "http://example.com/health", nil)
	req.Proto = "HTTP/1.1"
	req.Host = "example.com"

	d := knownDecision()
	start := time.Now().Add(-5 * time.Millisecond)
	f.emitAccessLog(req, 200, 100, cluster.Endpoint{}, start, nil, d)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name != "ingress" {
		t.Errorf("Name = %q, want ingress", s.Name)
	}
	if s.Kind != tracepb.Span_SPAN_KIND_SERVER {
		t.Errorf("Kind = %v, want SERVER", s.Kind)
	}
	if !s.Start.Before(s.End) {
		t.Errorf("start=%v must be before end=%v", s.Start, s.End)
	}
	if s.TraceID != d.TraceID {
		t.Errorf("TraceID mismatch: got %x, want %x", s.TraceID, d.TraceID)
	}
	if s.SpanID != d.SpanID {
		t.Errorf("SpanID mismatch: got %x, want %x", s.SpanID, d.SpanID)
	}
}

// TestSpanEmit_H1_NotSampledNoExport: Sample==false ⇒ no span.
func TestSpanEmit_H1_NotSampledNoExport(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 0}
	f := newTracingFilter(t, fe, cfg)

	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	req.Proto = "HTTP/1.1"

	d := &tracing.Decision{Sample: false, TraceID: [16]byte{1}, SpanID: [8]byte{1}}
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d)

	if got := len(fe.captured()); got != 0 {
		t.Errorf("expected 0 spans for not-sampled, got %d", got)
	}
}

// TestSpanEmit_H1_Continued: continued trace ⇒ span.ParentSpanID == inbound parent.
func TestSpanEmit_H1_Continued(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 100}
	f := newTracingFilter(t, fe, cfg)

	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	req.Proto = "HTTP/1.1"

	parentSpanID := [8]byte{10, 20, 30, 40, 50, 60, 70, 80}
	traceID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	d := &tracing.Decision{
		Sample:       true,
		Continued:    true,
		TraceID:      traceID,
		SpanID:       [8]byte{11, 22, 33, 44, 55, 66, 77, 88},
		ParentSpanID: parentSpanID,
	}
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.TraceID != traceID {
		t.Errorf("TraceID mismatch: got %x, want %x", s.TraceID, traceID)
	}
	if s.ParentSpanID != parentSpanID {
		t.Errorf("ParentSpanID mismatch: got %x, want %x", s.ParentSpanID, parentSpanID)
	}
}

// TestSpanEmit_H1_CancelNoSpan: statusCode==0 (ctx-cancel sentinel) ⇒ no span.
func TestSpanEmit_H1_CancelNoSpan(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 100}
	f := newTracingFilter(t, fe, cfg)

	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	req.Proto = "HTTP/1.1"

	d := &tracing.Decision{Sample: true}
	f.emitAccessLog(req, 0, 0, cluster.Endpoint{}, time.Now(), nil, d)

	if got := len(fe.captured()); got != 0 {
		t.Errorf("expected 0 spans for statusCode=0 (ctx-cancel), got %d", got)
	}
}

// TestSpanEmit_H1_ExportEvenWithoutAccessLog: AMEND-TRACE-SPANEND-SEAM.  The
// span block runs BEFORE the len(f.accessLog)==0 early-return, so a tracing
// HCM with no access_log block still exports spans.
func TestSpanEmit_H1_ExportEvenWithoutAccessLog(t *testing.T) {
	fe := &fakeExporter{}
	// No accessLog slice — the len(f.accessLog)==0 guard would skip emission.
	f := &Filter{
		exporter:      fe,
		tracingConfig: &tracing.TracingConfig{RandomSampling: 100},
	}

	req, _ := http.NewRequest("GET", "http://example.com/health", nil)
	req.Proto = "HTTP/1.1"

	d := knownDecision()
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d)

	if got := len(fe.captured()); got != 1 {
		t.Errorf("expected 1 span even with empty accessLog, got %d", got)
	}
}

// TestSpanEmit_H1_ByteStableNoTracing: nil exporter + nil traceDecision ⇒
// no span, no panic.  Verifies the no-tracing path is zero-behavioral-change.
func TestSpanEmit_H1_ByteStableNoTracing(t *testing.T) {
	f := &Filter{exporter: nil, tracingConfig: nil}
	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	req.Proto = "HTTP/1.1"
	// Must not panic.
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, nil)
}

// ── H2 tests ──────────────────────────────────────────────────────────────────

// TestSpanEmit_H2_SampledExports: H2 mirror of TestSpanEmit_H1_SampledExports.
func TestSpanEmit_H2_SampledExports(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 100, OverallSampling: 100}
	f := newTracingFilter(t, fe, cfg)

	req := h2.H2Request{
		Method:    "GET",
		Path:      "/health",
		Scheme:    "https",
		Authority: "example.com",
	}

	d := knownDecision()
	start := time.Now().Add(-5 * time.Millisecond)
	f.emitAccessLogH2(req, 200, 100, cluster.Endpoint{}, start, nil, d)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name != "ingress" {
		t.Errorf("Name = %q, want ingress", s.Name)
	}
	if s.Kind != tracepb.Span_SPAN_KIND_SERVER {
		t.Errorf("Kind = %v, want SERVER", s.Kind)
	}
	if !s.Start.Before(s.End) {
		t.Errorf("start=%v must be before end=%v", s.Start, s.End)
	}
	if s.TraceID != d.TraceID {
		t.Errorf("TraceID mismatch: got %x, want %x", s.TraceID, d.TraceID)
	}
	if s.SpanID != d.SpanID {
		t.Errorf("SpanID mismatch: got %x, want %x", s.SpanID, d.SpanID)
	}
}

// TestSpanEmit_H2_NotSampledNoExport: H2 mirror — Sample==false ⇒ no span.
func TestSpanEmit_H2_NotSampledNoExport(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 0}
	f := newTracingFilter(t, fe, cfg)

	req := h2.H2Request{Method: "GET", Path: "/", Authority: "example.com"}
	d := &tracing.Decision{Sample: false}
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d)

	if got := len(fe.captured()); got != 0 {
		t.Errorf("expected 0 spans for not-sampled, got %d", got)
	}
}

// TestSpanEmit_H2_Continued: H2 mirror — ParentSpanID propagated from decision.
func TestSpanEmit_H2_Continued(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 100}
	f := newTracingFilter(t, fe, cfg)

	req := h2.H2Request{Method: "GET", Path: "/", Scheme: "https", Authority: "example.com"}
	parentSpanID := [8]byte{10, 20, 30, 40, 50, 60, 70, 80}
	traceID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	d := &tracing.Decision{
		Sample:       true,
		Continued:    true,
		TraceID:      traceID,
		SpanID:       [8]byte{11, 22, 33, 44, 55, 66, 77, 88},
		ParentSpanID: parentSpanID,
	}
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.TraceID != traceID {
		t.Errorf("TraceID mismatch: got %x, want %x", s.TraceID, traceID)
	}
	if s.ParentSpanID != parentSpanID {
		t.Errorf("ParentSpanID mismatch: got %x, want %x", s.ParentSpanID, parentSpanID)
	}
}

// TestSpanEmit_H2_CancelNoSpan: H2 mirror — statusCode==0 ⇒ no span.
func TestSpanEmit_H2_CancelNoSpan(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 100}
	f := newTracingFilter(t, fe, cfg)

	req := h2.H2Request{Method: "GET", Path: "/", Authority: "example.com"}
	d := &tracing.Decision{Sample: true}
	f.emitAccessLogH2(req, 0, 0, cluster.Endpoint{}, time.Now(), nil, d)

	if got := len(fe.captured()); got != 0 {
		t.Errorf("expected 0 spans for statusCode=0, got %d", got)
	}
}

// TestSpanEmit_H2_ExportEvenWithoutAccessLog: H2 AMEND-TRACE-SPANEND-SEAM mirror.
func TestSpanEmit_H2_ExportEvenWithoutAccessLog(t *testing.T) {
	fe := &fakeExporter{}
	f := &Filter{
		exporter:      fe,
		tracingConfig: &tracing.TracingConfig{RandomSampling: 100},
	}

	req := h2.H2Request{Method: "GET", Path: "/health", Scheme: "https", Authority: "example.com"}
	d := knownDecision()
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d)

	if got := len(fe.captured()); got != 1 {
		t.Errorf("expected 1 span even with empty accessLog, got %d", got)
	}
}

// TestSpanEmit_H2_ByteStableNoTracing: H2 mirror — nil exporter + nil decision ⇒ no panic.
func TestSpanEmit_H2_ByteStableNoTracing(t *testing.T) {
	f := &Filter{exporter: nil}
	req := h2.H2Request{Method: "GET", Path: "/", Authority: "example.com"}
	// Must not panic.
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, nil)
}
