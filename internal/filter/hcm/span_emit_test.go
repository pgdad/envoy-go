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

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	metadatav3 "github.com/envoyproxy/go-control-plane/envoy/type/metadata/v3"
	typetracingv3 "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

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
	f.emitAccessLog(req, 200, 100, cluster.Endpoint{}, start, nil, d, nil, nil)

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
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d, nil, nil)

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
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d, nil, nil)

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
	f.emitAccessLog(req, 0, 0, cluster.Endpoint{}, time.Now(), nil, d, nil, nil)

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
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d, nil, nil)

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
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, nil, nil, nil)
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
	f.emitAccessLogH2(req, 200, 100, cluster.Endpoint{}, start, nil, d, nil, nil)

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
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d, nil, nil)

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
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d, nil, nil)

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
	f.emitAccessLogH2(req, 0, 0, cluster.Endpoint{}, time.Now(), nil, d, nil, nil)

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
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, d, nil, nil)

	if got := len(fe.captured()); got != 1 {
		t.Errorf("expected 1 span even with empty accessLog, got %d", got)
	}
}

// TestSpanEmit_H2_ByteStableNoTracing: H2 mirror — nil exporter + nil decision ⇒ no panic.
func TestSpanEmit_H2_ByteStableNoTracing(t *testing.T) {
	f := &Filter{exporter: nil}
	req := h2.H2Request{Method: "GET", Path: "/", Authority: "example.com"}
	// Must not panic.
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, nil, nil, nil)
}

// spanAttr returns the string value of the named span attribute (or "" if absent).
func spanAttr(s *tracing.Span, key string) string {
	for _, kv := range s.Attrs {
		if kv.Key == key {
			return kv.Str
		}
	}
	return ""
}

// TestSpanEmit_H1_MaxPathTagLengthTruncates: the H1 call site (accesslog_emit.go:40)
// byte-truncates the :path portion of http.url to TracingConfig.MaxPathTagLength (16),
// preserving the scheme://host prefix (SPEC-64 §3.4, mirrors §11 arm 0).
func TestSpanEmit_H1_MaxPathTagLengthTruncates(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 100, MaxPathTagLength: 16}
	f := newTracingFilter(t, fe, cfg)

	req, _ := http.NewRequest("GET", "http://example.com/abcdefghijKLMNOPqrstuvwxyz", nil)
	req.Proto = "HTTP/1.1"
	req.Host = "h.io"

	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, knownDecision(), nil, nil)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "http.url"); got != "http://h.io/abcdefghijKLMNO" {
		t.Errorf("http.url = %q, want http://h.io/abcdefghijKLMNO (:path truncated to 16 bytes)", got)
	}
}

// TestSpanEmit_H2_MaxPathTagLengthTruncates: the H2 call site (accesslog_emit.go:93)
// mirror — truncates req.Path to MaxPathTagLength, scheme://authority preserved.
func TestSpanEmit_H2_MaxPathTagLengthTruncates(t *testing.T) {
	fe := &fakeExporter{}
	cfg := &tracing.TracingConfig{RandomSampling: 100, MaxPathTagLength: 16}
	f := newTracingFilter(t, fe, cfg)

	req := h2.H2Request{Method: "GET", Path: "/abcdefghijKLMNOPqrstuvwxyz", Scheme: "http", Authority: "h.io"}
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, knownDecision(), nil, nil)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "http.url"); got != "http://h.io/abcdefghijKLMNO" {
		t.Errorf("http.url = %q, want http://h.io/abcdefghijKLMNO", got)
	}
}

// ── metadata custom_tag thread (Phase 70 T4) ─────────────────────────────────

// otelTracingProto builds a minimal OTel *hcmv3.HttpConnectionManager_Tracing
// carrying the given custom tags — the shape tracing.NewConfig parses into a
// *TracingConfig with the kindMetadata specs (unexported, so built via the real
// parser rather than a struct literal).
func otelTracingProto(t *testing.T, tags []*typetracingv3.CustomTag) *hcmv3.HttpConnectionManager_Tracing {
	t.Helper()
	otel := &tracev3.OpenTelemetryConfig{
		GrpcService: &corev3.GrpcService{
			TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: "c"},
			},
		},
		ServiceName: "svc",
	}
	any, err := anypb.New(otel)
	if err != nil {
		t.Fatalf("anypb.New(otel): %v", err)
	}
	return &hcmv3.HttpConnectionManager_Tracing{
		Provider: &tracev3.Tracing_Http{
			Name:       "envoy.tracers.opentelemetry",
			ConfigType: &tracev3.Tracing_Http_TypedConfig{TypedConfig: any},
		},
		CustomTags: tags,
	}
}

// metadataCustomTag builds a REQUEST-kind metadata custom_tag reading
// metadata_key.key==ns with a single path segment `seg`.
func metadataCustomTag(tag, ns, seg string) *typetracingv3.CustomTag {
	return &typetracingv3.CustomTag{
		Tag: tag,
		Type: &typetracingv3.CustomTag_Metadata_{Metadata: &typetracingv3.CustomTag_Metadata{
			Kind: &metadatav3.MetadataKind{Kind: &metadatav3.MetadataKind_Request_{Request: &metadatav3.MetadataKind_Request{}}},
			MetadataKey: &metadatav3.MetadataKey{
				Key:  ns,
				Path: []*metadatav3.MetadataKey_PathSegment{{Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: seg}}},
			},
		}},
	}
}

// TestSpanEmit_H1_MetadataCustomTag_ThreadLive proves the metaLookup thread is
// LIVE: a REQUEST-kind metadata custom_tag resolves its value from the supplied
// metaLookup onto the ingress span (the Phase 70 T4 seam). A byte-stable no-op
// metaLookup would leave the tag at default/omit — the resolved-value assertion
// discriminates.
func TestSpanEmit_H1_MetadataCustomTag_ThreadLive(t *testing.T) {
	fe := &fakeExporter{}
	cfg, err := tracing.NewConfig(otelTracingProto(t, []*typetracingv3.CustomTag{
		metadataCustomTag("mtag", "my.ns", "field"),
	}))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	f := newTracingFilter(t, fe, cfg)

	req, _ := http.NewRequest("GET", "http://example.com/health", nil)
	req.Proto = "HTTP/1.1"
	req.Host = "example.com"

	// metaLookup resolves (my.ns, field) -> a structpb string value.
	metaLookup := func(ns, key string) (*structpb.Value, bool) {
		if ns == "my.ns" && key == "field" {
			return structpb.NewStringValue("resolved-value"), true
		}
		return nil, false
	}

	f.emitAccessLog(req, 200, 100, cluster.Endpoint{}, time.Now(), nil, knownDecision(), metaLookup, nil)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "mtag"); got != "resolved-value" {
		t.Errorf("span attr mtag = %q, want resolved-value (metaLookup thread not live)", got)
	}
}

// routeMetadataCustomTag builds a ROUTE-kind metadata custom_tag reading
// metadata_key.key==ns with a single path segment `seg`.
func routeMetadataCustomTag(tag, ns, seg string) *typetracingv3.CustomTag {
	return &typetracingv3.CustomTag{
		Tag: tag,
		Type: &typetracingv3.CustomTag_Metadata_{Metadata: &typetracingv3.CustomTag_Metadata{
			Kind: &metadatav3.MetadataKind{Kind: &metadatav3.MetadataKind_Route_{Route: &metadatav3.MetadataKind_Route{}}},
			MetadataKey: &metadatav3.MetadataKey{
				Key:  ns,
				Path: []*metadatav3.MetadataKey_PathSegment{{Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: seg}}},
			},
		}},
	}
}

// TestSpanEmit_H1_RouteMetadataCustomTag_ThreadLive proves the routeMetaLookup
// thread is LIVE (phase 71 T4): a ROUTE-kind metadata custom_tag resolves its
// value from the supplied routeMetaLookup onto the ingress span. A nil (or
// always-miss) routeMetaLookup would leave the tag at default/omit — the
// resolved-value assertion discriminates (Break H / Break I).
func TestSpanEmit_H1_RouteMetadataCustomTag_ThreadLive(t *testing.T) {
	fe := &fakeExporter{}
	cfg, err := tracing.NewConfig(otelTracingProto(t, []*typetracingv3.CustomTag{
		routeMetadataCustomTag("rtag", "route.ns", "field"),
	}))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	f := newTracingFilter(t, fe, cfg)

	req, _ := http.NewRequest("GET", "http://example.com/health", nil)
	req.Proto = "HTTP/1.1"
	req.Host = "example.com"

	// routeMetaLookup resolves ns "route.ns" -> a structpb struct value with
	// a "field" key, mirroring FilterChain.RouteMetaLookup's shape (the
	// namespace's filter_metadata struct, wrapped via structpb.NewStructValue).
	routeMetaLookup := func(ns string) (*structpb.Value, bool) {
		if ns == "route.ns" {
			s, _ := structpb.NewStruct(map[string]interface{}{"field": "route-resolved-value"})
			return structpb.NewStructValue(s), true
		}
		return nil, false
	}

	f.emitAccessLog(req, 200, 100, cluster.Endpoint{}, time.Now(), nil, knownDecision(), nil, routeMetaLookup)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "rtag"); got != "route-resolved-value" {
		t.Errorf("span attr rtag = %q, want route-resolved-value (routeMetaLookup thread not live)", got)
	}
}
