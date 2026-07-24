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

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	tracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	metadatav3 "github.com/envoyproxy/go-control-plane/envoy/type/metadata/v3"
	typetracingv3 "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
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

// ── HOST metadata custom_tag thread (Phase 72 T4) ────────────────────────────

// hostMetadataCustomTag builds a HOST-kind metadata custom_tag reading
// metadata_key.key==ns down the FULL path `segs` (>=1 segment; parse rejects an
// empty path). defaultValue=="" ⇒ HasDefault==false ⇒ the tag is OMITTED
// entirely when the lookup misses; a non-empty default is emitted instead.
func hostMetadataCustomTag(tag, ns, defaultValue string, segs ...string) *typetracingv3.CustomTag {
	path := make([]*metadatav3.MetadataKey_PathSegment, 0, len(segs))
	for _, seg := range segs {
		path = append(path, &metadatav3.MetadataKey_PathSegment{
			Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: seg},
		})
	}
	return &typetracingv3.CustomTag{
		Tag: tag,
		Type: &typetracingv3.CustomTag_Metadata_{Metadata: &typetracingv3.CustomTag_Metadata{
			Kind:         &metadatav3.MetadataKind{Kind: &metadatav3.MetadataKind_Host_{Host: &metadatav3.MetadataKind_Host{}}},
			MetadataKey:  &metadatav3.MetadataKey{Key: ns, Path: path},
			DefaultValue: defaultValue,
		}},
	}
}

// mkHostMetadataEndpoint returns a REAL cluster.Endpoint carrying
// lb_endpoints[].metadata.filter_metadata, built through the PRODUCTION populate
// path: cluster.NewManager → Manager.Get → Cluster.PickEndpoint (already
// exported; no dial). Endpoint.filterMetadata is UNEXPORTED, so a package-hcm
// test cannot build one by struct literal, and minting a test constructor would
// add a third new exported symbol this row's envelope forbids.
//
// The namespace is deliberately NOT "envoy.lb" (proving ANY namespace is
// addressable — the SPEC's P2 probe) and the value is NESTED under
// outer→inner (proving the HOST arm descends the FULL MetaPath, not [1:]).
func mkHostMetadataEndpoint(t *testing.T) cluster.Endpoint {
	t.Helper()
	nsStruct, err := structpb.NewStruct(map[string]interface{}{
		"outer": map[string]interface{}{"inner": "host-resolved-value"},
	})
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 "c_hostmeta",
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: "c_hostmeta",
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							Metadata: &corev3.Metadata{
								FilterMetadata: map[string]*structpb.Struct{"host.ns": nsStruct},
							},
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       "127.0.0.1",
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 1},
									},
								}},
							}},
						}},
					}},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	cl, ok := cm.Get("c_hostmeta")
	if !ok {
		t.Fatalf("Manager.Get(c_hostmeta): not found")
	}
	ep, err := cl.PickEndpoint()
	if err != nil {
		t.Fatalf("PickEndpoint: %v", err)
	}
	if _, found := ep.MetaLookup("host.ns"); !found {
		t.Fatalf("precondition: picked endpoint carries no host.ns filter_metadata")
	}
	return ep
}

// hostMetaFilter builds a *Filter whose tracing config carries ONE HOST-kind
// metadata custom_tag "htag" reading host.ns → outer → inner, with the supplied
// default_value.
func hostMetaFilter(t *testing.T, fe *fakeExporter, defaultValue string) *Filter {
	t.Helper()
	cfg, err := tracing.NewConfig(otelTracingProto(t, []*typetracingv3.CustomTag{
		hostMetadataCustomTag("htag", "host.ns", defaultValue, "outer", "inner"),
	}))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	return newTracingFilter(t, fe, cfg)
}

// spanHasAttr reports whether the span carries an attribute with the given key
// (presence, NOT value — the omit case and an empty-string value both render as
// "" through spanAttr, so the omit assertion needs presence).
func spanHasAttr(s *tracing.Span, key string) bool {
	for _, kv := range s.Attrs {
		if kv.Key == key {
			return true
		}
	}
	return false
}

// TestSpanEmit_H1_HostMetadataCustomTag_ThreadLive proves the H1 call site
// (accesslog_emit.go:57) threads picked.MetaLookup as the 5th
// ResolveCustomTags argument: a HOST-kind metadata custom_tag resolves its value
// from the SELECTED UPSTREAM ENDPOINT's static filter_metadata onto the ingress
// span. A nil 5th argument would leave the tag OMITTED (no default) — the
// resolved-value assertion discriminates (Break L / Break M).
func TestSpanEmit_H1_HostMetadataCustomTag_ThreadLive(t *testing.T) {
	fe := &fakeExporter{}
	f := hostMetaFilter(t, fe, "")
	picked := mkHostMetadataEndpoint(t)

	req, _ := http.NewRequest("GET", "http://example.com/health", nil)
	req.Proto = "HTTP/1.1"
	req.Host = "example.com"

	f.emitAccessLog(req, 200, 100, picked, time.Now(), nil, knownDecision(), nil, nil)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "htag"); got != "host-resolved-value" {
		t.Errorf("H1 span attr htag = %q, want host-resolved-value (picked.MetaLookup thread not live at accesslog_emit.go:57)", got)
	}
}

// TestSpanEmit_H2_HostMetadataCustomTag_ThreadLive: the H2 mirror, proving the
// SECOND call site (accesslog_emit.go:118) independently. A single shared helper
// would leave two of the three sites unproven.
func TestSpanEmit_H2_HostMetadataCustomTag_ThreadLive(t *testing.T) {
	fe := &fakeExporter{}
	f := hostMetaFilter(t, fe, "")
	picked := mkHostMetadataEndpoint(t)

	req := h2.H2Request{Method: "GET", Path: "/health", Scheme: "https", Authority: "example.com"}

	f.emitAccessLogH2(req, 200, 100, picked, time.Now(), nil, knownDecision(), nil, nil)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "htag"); got != "host-resolved-value" {
		t.Errorf("H2 span attr htag = %q, want host-resolved-value (picked.MetaLookup thread not live at accesslog_emit.go:118)", got)
	}
}

// TestSpanEmit_H3_HostMetadataCustomTag_ThreadLive: the H3 mirror, proving the
// THIRD call site (accesslog_emit.go:179) independently.
func TestSpanEmit_H3_HostMetadataCustomTag_ThreadLive(t *testing.T) {
	fe := &fakeExporter{}
	f := hostMetaFilter(t, fe, "")
	picked := mkHostMetadataEndpoint(t)

	req, _ := http.NewRequest("GET", "http://example.com/health", nil)
	req.Proto = "HTTP/3.0"
	req.Host = "example.com"

	f.emitAccessLogH3(req, 200, 100, picked, time.Now(), nil, knownDecision(), nil, nil)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "htag"); got != "host-resolved-value" {
		t.Errorf("H3 span attr htag = %q, want host-resolved-value (picked.MetaLookup thread not live at accesslog_emit.go:179)", got)
	}
}

// TestSpanEmit_ZeroPickedHostMetadata_FallsToDefault pins the ZERO-Endpoint arm:
// the 5 SPAN-CAPABLE local-reply sites (connection.go:597/:699,
// h2dispatch.go:530, h3dispatch.go:280/:341 — all post-Decode local replies)
// pass cluster.Endpoint{}, whose nil filterMetadata map makes MetaLookup return
// (nil,false) WITHOUT panicking. A HOST tag there falls to default_value, and
// with an empty/omitted default is OMITTED entirely (the request_header default
// rule, RD-DEFAULT). This is the named departure B2 — NOT "11 of 18": the other
// 6 zero-Endpoint sites are PRE-Decide and can never emit a span at all.
func TestSpanEmit_ZeroPickedHostMetadata_FallsToDefault(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com/health", nil)
	req.Proto = "HTTP/1.1"
	req.Host = "example.com"

	// (a) non-empty default_value ⇒ the tag is emitted with the DEFAULT.
	feDefault := &fakeExporter{}
	fDefault := hostMetaFilter(t, feDefault, "fallback")
	fDefault.emitAccessLog(req, 200, 100, cluster.Endpoint{}, time.Now(), nil, knownDecision(), nil, nil)
	spans := feDefault.captured()
	if len(spans) != 1 {
		t.Fatalf("default arm: expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "htag"); got != "fallback" {
		t.Errorf("zero picked, default_value=fallback: htag = %q, want fallback", got)
	}

	// (b) empty default_value ⇒ the tag is OMITTED entirely (presence check).
	feOmit := &fakeExporter{}
	fOmit := hostMetaFilter(t, feOmit, "")
	fOmit.emitAccessLog(req, 200, 100, cluster.Endpoint{}, time.Now(), nil, knownDecision(), nil, nil)
	spans = feOmit.captured()
	if len(spans) != 1 {
		t.Fatalf("omit arm: expected 1 span, got %d", len(spans))
	}
	if spanHasAttr(spans[0], "htag") {
		t.Errorf("zero picked, empty default_value: htag present (%q), want OMITTED", spanAttr(spans[0], "htag"))
	}
}

// ── CLUSTER metadata custom_tag thread (Phase 73 T4) ─────────────────────────

// clusterMetadataCustomTag builds a CLUSTER-kind metadata custom_tag reading
// metadata_key.key==ns down the FULL path `segs` (>=1 segment; parse rejects an
// empty path). defaultValue=="" ⇒ HasDefault==false ⇒ the tag is OMITTED
// entirely when the lookup misses; a non-empty default is emitted instead (the
// request_header default rule — a present-but-EMPTY value still emits "").
func clusterMetadataCustomTag(tag, ns, defaultValue string, segs ...string) *typetracingv3.CustomTag {
	path := make([]*metadatav3.MetadataKey_PathSegment, 0, len(segs))
	for _, seg := range segs {
		path = append(path, &metadatav3.MetadataKey_PathSegment{
			Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: seg},
		})
	}
	return &typetracingv3.CustomTag{
		Tag: tag,
		Type: &typetracingv3.CustomTag_Metadata_{Metadata: &typetracingv3.CustomTag_Metadata{
			Kind:         &metadatav3.MetadataKind{Kind: &metadatav3.MetadataKind_Cluster_{Cluster: &metadatav3.MetadataKind_Cluster{}}},
			MetadataKey:  &metadatav3.MetadataKey{Key: ns, Path: path},
			DefaultValue: defaultValue,
		}},
	}
}

// mkClusterMetadataEndpoint returns a REAL cluster.Endpoint carrying the OWNING
// CLUSTER's clusters[].metadata.filter_metadata, built through the PRODUCTION
// populate path: cluster.NewManager → Manager.Get → Cluster.PickEndpoint
// (already exported; no dial). Endpoint.clusterFilterMetadata is UNEXPORTED, so
// a package-hcm test cannot build one by struct literal, and minting a test
// constructor would add a SECOND new exported symbol this row's envelope forbids
// (exactly ONE: cluster.Endpoint.ClusterMetaLookup).
//
// ⚠️ The metadata block sits on the CLUSTER (a sibling of name:/type:), NOT on
// lb_endpoints[] — that is the phase-72 HOST source and putting it there is the
// source-relocation failure mode. The namespace is deliberately NOT "envoy.lb"
// (ANY filter_metadata namespace is addressable) and the value is NESTED under
// outer→inner (the CLUSTER arm descends the FULL MetaPath, not [1:]).
func mkClusterMetadataEndpoint(t *testing.T) cluster.Endpoint {
	t.Helper()
	nsStruct, err := structpb.NewStruct(map[string]interface{}{
		"outer": map[string]interface{}{"inner": "cluster-resolved-value"},
	})
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 "c_clustermeta",
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				Metadata: &corev3.Metadata{
					FilterMetadata: map[string]*structpb.Struct{"cluster.ns": nsStruct},
				},
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: "c_clustermeta",
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       "127.0.0.1",
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 1},
									},
								}},
							}},
						}},
					}},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	cl, ok := cm.Get("c_clustermeta")
	if !ok {
		t.Fatalf("Manager.Get(c_clustermeta): not found")
	}
	ep, err := cl.PickEndpoint()
	if err != nil {
		t.Fatalf("PickEndpoint: %v", err)
	}
	if _, found := ep.ClusterMetaLookup("cluster.ns"); !found {
		t.Fatalf("precondition: picked endpoint carries no cluster.ns owning-cluster filter_metadata")
	}
	return ep
}

// clusterMetaFilter builds a *Filter whose tracing config carries ONE
// CLUSTER-kind metadata custom_tag "ctag" reading cluster.ns → outer → inner,
// with the supplied default_value.
func clusterMetaFilter(t *testing.T, fe *fakeExporter, defaultValue string) *Filter {
	t.Helper()
	cfg, err := tracing.NewConfig(otelTracingProto(t, []*typetracingv3.CustomTag{
		clusterMetadataCustomTag("ctag", "cluster.ns", defaultValue, "outer", "inner"),
	}))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	return newTracingFilter(t, fe, cfg)
}

// TestSpanEmit_H1_ClusterMetadataCustomTag_ThreadLive proves the H1 call site
// (accesslog_emit.go:57) threads picked.ClusterMetaLookup as the 6th
// ResolveCustomTags argument: a CLUSTER-kind metadata custom_tag resolves its
// value from the PICKED host's OWNING CLUSTER's static filter_metadata onto the
// ingress span. A nil 6th argument would leave the tag OMITTED (no default) —
// the resolved-value assertion discriminates (Break L / Break M).
func TestSpanEmit_H1_ClusterMetadataCustomTag_ThreadLive(t *testing.T) {
	fe := &fakeExporter{}
	f := clusterMetaFilter(t, fe, "")
	picked := mkClusterMetadataEndpoint(t)

	req, _ := http.NewRequest("GET", "http://example.com/health", nil)
	req.Proto = "HTTP/1.1"
	req.Host = "example.com"

	f.emitAccessLog(req, 200, 100, picked, time.Now(), nil, knownDecision(), nil, nil)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "ctag"); got != "cluster-resolved-value" {
		t.Errorf("H1 span attr ctag = %q, want cluster-resolved-value (picked.ClusterMetaLookup thread not live at accesslog_emit.go:57)", got)
	}
}

// TestSpanEmit_H2_ClusterMetadataCustomTag_ThreadLive: the H2 mirror, proving
// the SECOND call site (accesslog_emit.go:118) INDEPENDENTLY. A single shared
// helper test would leave two of the three sites unproven — Break L (drop the
// 6th arg at :57 only) is what checks that this independence is real.
func TestSpanEmit_H2_ClusterMetadataCustomTag_ThreadLive(t *testing.T) {
	fe := &fakeExporter{}
	f := clusterMetaFilter(t, fe, "")
	picked := mkClusterMetadataEndpoint(t)

	req := h2.H2Request{Method: "GET", Path: "/health", Scheme: "https", Authority: "example.com"}

	f.emitAccessLogH2(req, 200, 100, picked, time.Now(), nil, knownDecision(), nil, nil)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "ctag"); got != "cluster-resolved-value" {
		t.Errorf("H2 span attr ctag = %q, want cluster-resolved-value (picked.ClusterMetaLookup thread not live at accesslog_emit.go:118)", got)
	}
}

// TestSpanEmit_H3_ClusterMetadataCustomTag_ThreadLive: the H3 mirror, proving
// the THIRD call site (accesslog_emit.go:179) INDEPENDENTLY. ⚠️ :57 and :179 are
// BYTE-IDENTICAL source lines (both use reqHeaderLookupH1(r)), so only a
// per-site test distinguishes them.
func TestSpanEmit_H3_ClusterMetadataCustomTag_ThreadLive(t *testing.T) {
	fe := &fakeExporter{}
	f := clusterMetaFilter(t, fe, "")
	picked := mkClusterMetadataEndpoint(t)

	req, _ := http.NewRequest("GET", "http://example.com/health", nil)
	req.Proto = "HTTP/3.0"
	req.Host = "example.com"

	f.emitAccessLogH3(req, 200, 100, picked, time.Now(), nil, knownDecision(), nil, nil)

	spans := fe.captured()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "ctag"); got != "cluster-resolved-value" {
		t.Errorf("H3 span attr ctag = %q, want cluster-resolved-value (picked.ClusterMetaLookup thread not live at accesslog_emit.go:179)", got)
	}
}

// TestSpanEmit_ZeroPickedClusterMetadata_FallsToDefault pins the ZERO-Endpoint
// arm for the CLUSTER kind: the 5 SPAN-CAPABLE local-reply sites
// (connection.go:597/:699, h2dispatch.go:530, h3dispatch.go:280/:341 — all
// post-Decide local replies) pass cluster.Endpoint{}, whose nil
// clusterFilterMetadata map makes ClusterMetaLookup return (nil,false) WITHOUT
// panicking. A CLUSTER tag there falls to default_value, and with an
// empty/omitted default is OMITTED entirely (the request_header default rule).
//
// ⚠️ NOT "10 of 18" or "11 of 18": the other zero-Endpoint sites are PRE-Decide
// (connection.go:330/:464, h2dispatch.go:313/:396, h3dispatch.go:130/:210) and
// can never emit a span at all, so only 5 of the 12 span-capable sites are in
// this arm.
//
// ⚠️ Asserted by PRESENCE, not `== ""`: the request_header default rule makes a
// present-but-empty value emit "", so `== ""` cannot discriminate OMITTED from
// emitted-as-empty.
func TestSpanEmit_ZeroPickedClusterMetadata_FallsToDefault(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com/health", nil)
	req.Proto = "HTTP/1.1"
	req.Host = "example.com"

	// (a) non-empty default_value ⇒ the tag is emitted with the DEFAULT.
	feDefault := &fakeExporter{}
	fDefault := clusterMetaFilter(t, feDefault, "cfallback")
	fDefault.emitAccessLog(req, 200, 100, cluster.Endpoint{}, time.Now(), nil, knownDecision(), nil, nil)
	spans := feDefault.captured()
	if len(spans) != 1 {
		t.Fatalf("default arm: expected 1 span, got %d", len(spans))
	}
	if got := spanAttr(spans[0], "ctag"); got != "cfallback" {
		t.Errorf("zero picked, default_value=cfallback: ctag = %q, want cfallback", got)
	}

	// (b) empty default_value ⇒ the tag is OMITTED entirely (presence check).
	feOmit := &fakeExporter{}
	fOmit := clusterMetaFilter(t, feOmit, "")
	fOmit.emitAccessLog(req, 200, 100, cluster.Endpoint{}, time.Now(), nil, knownDecision(), nil, nil)
	spans = feOmit.captured()
	if len(spans) != 1 {
		t.Fatalf("omit arm: expected 1 span, got %d", len(spans))
	}
	if spanHasAttr(spans[0], "ctag") {
		t.Errorf("zero picked, empty default_value: ctag present (%q), want OMITTED", spanAttr(spans[0], "ctag"))
	}
}

// mkDualMetadataEndpoint returns a REAL cluster.Endpoint whose OWNING CLUSTER
// and whose OWN lb_endpoint both carry the SAME namespace+path
// (dual.ns → outer → inner) with DIFFERENT values. Built through the same
// production populate path (NewManager → Get → PickEndpoint).
func mkDualMetadataEndpoint(t *testing.T) cluster.Endpoint {
	t.Helper()
	mk := func(v string) *structpb.Struct {
		st, err := structpb.NewStruct(map[string]interface{}{
			"outer": map[string]interface{}{"inner": v},
		})
		if err != nil {
			t.Fatalf("structpb.NewStruct(%q): %v", v, err)
		}
		return st
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 "c_dualmeta",
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				Metadata: &corev3.Metadata{
					FilterMetadata: map[string]*structpb.Struct{"dual.ns": mk("from-cluster")},
				},
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: "c_dualmeta",
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							Metadata: &corev3.Metadata{
								FilterMetadata: map[string]*structpb.Struct{"dual.ns": mk("from-host")},
							},
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       "127.0.0.1",
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 1},
									},
								}},
							}},
						}},
					}},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	cl, ok := cm.Get("c_dualmeta")
	if !ok {
		t.Fatalf("Manager.Get(c_dualmeta): not found")
	}
	ep, err := cl.PickEndpoint()
	if err != nil {
		t.Fatalf("PickEndpoint: %v", err)
	}
	if _, found := ep.MetaLookup("dual.ns"); !found {
		t.Fatalf("precondition: picked endpoint carries no dual.ns HOST filter_metadata")
	}
	if _, found := ep.ClusterMetaLookup("dual.ns"); !found {
		t.Fatalf("precondition: picked endpoint carries no dual.ns CLUSTER filter_metadata")
	}
	return ep
}

// TestSpanEmit_ClusterVsHostMetadata_SourceDistinct pins the two ADJACENT,
// IDENTICALLY-TYPED method values (picked.MetaLookup 5th, picked.ClusterMetaLookup
// 6th) against being CROSSED at the emit call sites — a defect the COMPILER
// CANNOT catch, since both are func(string) (*structpb.Value, bool).
//
// One config, one namespace+path (dual.ns → outer → inner), TWO tags: a HOST
// tag and a CLUSTER tag. The endpoint's OWN metadata says "from-host"; its
// OWNING CLUSTER's metadata says "from-cluster". Each span attribute must carry
// its OWN source's value. Swapping the 5th and 6th arguments compiles clean,
// passes vet and passes lint — this test is the ONLY guard (Break M2).
//
// Run at ALL THREE call sites, since a crossing could be introduced at one only.
func TestSpanEmit_ClusterVsHostMetadata_SourceDistinct(t *testing.T) {
	cfg, err := tracing.NewConfig(otelTracingProto(t, []*typetracingv3.CustomTag{
		hostMetadataCustomTag("dual_host", "dual.ns", "", "outer", "inner"),
		clusterMetadataCustomTag("dual_cluster", "dual.ns", "", "outer", "inner"),
	}))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	picked := mkDualMetadataEndpoint(t)

	r, _ := http.NewRequest("GET", "http://example.com/health", nil)
	r.Proto = "HTTP/1.1"
	r.Host = "example.com"
	h2req := h2.H2Request{Method: "GET", Path: "/health", Scheme: "https", Authority: "example.com"}

	for _, tc := range []struct {
		name string
		site string
		emit func(f *Filter)
	}{
		{"H1", "accesslog_emit.go:57", func(f *Filter) {
			f.emitAccessLog(r, 200, 100, picked, time.Now(), nil, knownDecision(), nil, nil)
		}},
		{"H2", "accesslog_emit.go:118", func(f *Filter) {
			f.emitAccessLogH2(h2req, 200, 100, picked, time.Now(), nil, knownDecision(), nil, nil)
		}},
		{"H3", "accesslog_emit.go:179", func(f *Filter) {
			f.emitAccessLogH3(r, 200, 100, picked, time.Now(), nil, knownDecision(), nil, nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fe := &fakeExporter{}
			tc.emit(newTracingFilter(t, fe, cfg))
			spans := fe.captured()
			if len(spans) != 1 {
				t.Fatalf("expected 1 span, got %d", len(spans))
			}
			if got := spanAttr(spans[0], "dual_host"); got != "from-host" {
				t.Errorf("%s span attr dual_host = %q, want from-host (HOST/CLUSTER lookups crossed at %s?)", tc.name, got, tc.site)
			}
			if got := spanAttr(spans[0], "dual_cluster"); got != "from-cluster" {
				t.Errorf("%s span attr dual_cluster = %q, want from-cluster (HOST/CLUSTER lookups crossed at %s?)", tc.name, got, tc.site)
			}
		})
	}
}
