package tracing

import (
	"testing"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// freshDecision returns a sampled, non-continued Decision with known IDs.
func freshDecision() Decision {
	var traceID [16]byte
	var spanID [8]byte
	for i := range traceID {
		traceID[i] = byte(i + 1)
	}
	for i := range spanID {
		spanID[i] = byte(i + 0x11)
	}
	return Decision{
		Sample:       true,
		Reason:       Sampled,
		Class:        RandomSampling,
		Continued:    false,
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: [8]byte{}, // all-zero => root
		TraceState:   "",
		RequestID:    "abc-9def",
	}
}

func freshInputs() SpanInputs {
	return SpanInputs{
		Method:            "GET",
		URL:               "http://h/p",
		Protocol:          "HTTP/1.1",
		StatusCode:        200,
		UserAgent:         "ua",
		RequestSize:       0,
		ResponseSize:      11,
		UpstreamCluster:   "c",
		DownstreamCluster: "-",
		ResponseFlags:     "-",
		NodeID:            "",
		Zone:              "",
		PeerAddress:       "1.2.3.4",
		ClientTraceID:     "",
	}
}

// TestSpanBuildFresh verifies BuildServerSpan with a fresh (root) Decision.
func TestSpanBuildFresh(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	start := time.Now()
	end := start.Add(10 * time.Millisecond)

	s := BuildServerSpan(d, in, start, end)

	if s.Name != "ingress" {
		t.Errorf("Name = %q, want %q", s.Name, "ingress")
	}
	if s.Kind != tracepb.Span_SPAN_KIND_SERVER {
		t.Errorf("Kind = %v, want SPAN_KIND_SERVER", s.Kind)
	}
	if !s.Start.Equal(start) {
		t.Errorf("Start = %v, want %v", s.Start, start)
	}
	if !s.End.Equal(end) {
		t.Errorf("End = %v, want %v", s.End, end)
	}
	if !s.Start.Before(s.End) {
		t.Errorf("Start %v is not before End %v", s.Start, s.End)
	}
	if s.TraceID != d.TraceID {
		t.Errorf("TraceID mismatch")
	}
	if s.SpanID != d.SpanID {
		t.Errorf("SpanID mismatch")
	}
	if s.ParentSpanID != ([8]byte{}) {
		t.Errorf("ParentSpanID = %v, want zero (root)", s.ParentSpanID)
	}

	// guid:x-request-id must be present with value == RequestID
	if v := attrStr(s.Attrs, "guid:x-request-id"); v != d.RequestID {
		t.Errorf("guid:x-request-id = %q, want %q", v, d.RequestID)
	}
	// guid:x-client-trace-id must be absent (empty ClientTraceID input)
	if hasAttr(s.Attrs, "guid:x-client-trace-id") {
		t.Error("guid:x-client-trace-id present, want absent for empty ClientTraceID")
	}

	// Spot-check a few attribute values
	if v := attrStr(s.Attrs, "http.method"); v != "GET" {
		t.Errorf("http.method = %q, want GET", v)
	}
	if v := attrStr(s.Attrs, "http.url"); v != "http://h/p" {
		t.Errorf("http.url = %q, want http://h/p", v)
	}
	if v := attrStr(s.Attrs, "http.protocol"); v != "HTTP/1.1" {
		t.Errorf("http.protocol = %q, want HTTP/1.1", v)
	}
	if v := attrStr(s.Attrs, "component"); v != "proxy" {
		t.Errorf("component = %q, want proxy", v)
	}
	if v := attrStr(s.Attrs, "upstream_cluster"); v != "c" {
		t.Errorf("upstream_cluster = %q, want c", v)
	}
	if v := attrStr(s.Attrs, "upstream_cluster.name"); v != "c" {
		t.Errorf("upstream_cluster.name = %q, want c", v)
	}
	if v := attrStr(s.Attrs, "downstream_cluster"); v != "-" {
		t.Errorf("downstream_cluster = %q, want -", v)
	}
	if v := attrStr(s.Attrs, "response_flags"); v != "-" {
		t.Errorf("response_flags = %q, want -", v)
	}
	if v := attrStr(s.Attrs, "user_agent"); v != "ua" {
		t.Errorf("user_agent = %q, want ua", v)
	}
	if v := attrStr(s.Attrs, "peer.address"); v != "1.2.3.4" {
		t.Errorf("peer.address = %q, want 1.2.3.4", v)
	}

	// STRING attributes (previously INT; changed to STRING to match the reference
	// Envoy cpp OTel tracer wire format — observed via the 0087 differential DUMP).
	if v := attrStr(s.Attrs, "http.status_code"); v != "200" {
		t.Errorf("http.status_code = %q, want \"200\"", v)
	}
	if v := attrStr(s.Attrs, "request_size"); v != "0" {
		t.Errorf("request_size = %q, want \"0\"", v)
	}
	if v := attrStr(s.Attrs, "response_size"); v != "11" {
		t.Errorf("response_size = %q, want \"11\"", v)
	}
}

// TestSpanBuildAuthority verifies the provider-neutral Authority field is carried
// on the Span alongside the unchanged Name="ingress", and that toProto's Name stays
// "ingress" (the OTLP wire is unaffected by Authority — D-TRACE-ZIPKIN-SPAN-NAME).
func TestSpanBuildAuthority(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	in.Authority = "127.0.0.1:10000"
	start := time.Now()
	end := start.Add(5 * time.Millisecond)

	s := BuildServerSpan(d, in, start, end)

	if s.Authority != "127.0.0.1:10000" {
		t.Errorf("Authority = %q, want %q", s.Authority, "127.0.0.1:10000")
	}
	if s.Name != "ingress" {
		t.Errorf("Name = %q, want %q", s.Name, "ingress")
	}
	if pb := s.toProto(); pb.Name != "ingress" {
		t.Errorf("toProto().Name = %q, want %q (OTLP unchanged)", pb.Name, "ingress")
	}
}

// TestSpanBuildContinued verifies that a continued Decision propagates ParentSpanID.
func TestSpanBuildContinued(t *testing.T) {
	d := freshDecision()
	var parent [8]byte
	for i := range parent {
		parent[i] = byte(0x55 + i)
	}
	d.Continued = true
	d.ParentSpanID = parent

	in := freshInputs()
	start := time.Now()
	end := start.Add(5 * time.Millisecond)

	s := BuildServerSpan(d, in, start, end)

	if s.ParentSpanID != parent {
		t.Errorf("ParentSpanID = %v, want %v", s.ParentSpanID, parent)
	}
}

// TestSpanBuildClientTraceID verifies that a non-empty ClientTraceID produces the attr.
func TestSpanBuildClientTraceID(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	in.ClientTraceID = "abc"
	start := time.Now()
	end := start.Add(5 * time.Millisecond)

	s := BuildServerSpan(d, in, start, end)

	if v := attrStr(s.Attrs, "guid:x-client-trace-id"); v != "abc" {
		t.Errorf("guid:x-client-trace-id = %q, want %q", v, "abc")
	}
}

// TestSpanToProtoFresh verifies toProto for a root span (all-zero ParentSpanID).
func TestSpanToProtoFresh(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	start := time.Unix(0, 1_000_000_000)
	end := time.Unix(0, 1_010_000_000)

	s := BuildServerSpan(d, in, start, end)
	pb := s.toProto()

	// TraceId / SpanId
	if len(pb.TraceId) != 16 {
		t.Fatalf("TraceId len = %d, want 16", len(pb.TraceId))
	}
	for i, b := range pb.TraceId {
		if b != d.TraceID[i] {
			t.Errorf("TraceId[%d] = %02x, want %02x", i, b, d.TraceID[i])
		}
	}
	if len(pb.SpanId) != 8 {
		t.Fatalf("SpanId len = %d, want 8", len(pb.SpanId))
	}
	for i, b := range pb.SpanId {
		if b != d.SpanID[i] {
			t.Errorf("SpanId[%d] = %02x, want %02x", i, b, d.SpanID[i])
		}
	}

	// Root: ParentSpanId must be nil or empty
	if len(pb.ParentSpanId) != 0 {
		t.Errorf("ParentSpanId = %v, want nil/empty for root span", pb.ParentSpanId)
	}

	if pb.Name != "ingress" {
		t.Errorf("Name = %q, want ingress", pb.Name)
	}
	if pb.Kind != tracepb.Span_SPAN_KIND_SERVER {
		t.Errorf("Kind = %v, want SPAN_KIND_SERVER", pb.Kind)
	}
	if pb.StartTimeUnixNano != uint64(start.UnixNano()) {
		t.Errorf("StartTimeUnixNano = %d, want %d", pb.StartTimeUnixNano, uint64(start.UnixNano()))
	}
	if pb.EndTimeUnixNano != uint64(end.UnixNano()) {
		t.Errorf("EndTimeUnixNano = %d, want %d", pb.EndTimeUnixNano, uint64(end.UnixNano()))
	}

	// Verify ALL asserted attrs are STRING (http.status_code, request_size,
	// response_size changed from INT to STRING in this phase to match the reference
	// Envoy cpp OTel tracer wire format — observed via the 0087 differential DUMP).
	kvMap := protoKVMap(pb.Attributes)
	strKeys := []string{
		"http.method", "http.url", "http.protocol",
		"http.status_code", "request_size", "response_size",
		"component", "upstream_cluster", "upstream_cluster.name",
		"downstream_cluster", "response_flags", "user_agent",
		"guid:x-request-id", "peer.address",
	}
	for _, k := range strKeys {
		v, ok := kvMap[k]
		if !ok {
			t.Errorf("attribute %q missing from proto", k)
			continue
		}
		if _, ok := v.Value.(*commonpb.AnyValue_StringValue); !ok {
			t.Errorf("attribute %q Value type = %T, want StringValue", k, v.Value)
		}
	}
}

// TestSpanToProtoContinued verifies that a continued span has an 8-byte ParentSpanId.
func TestSpanToProtoContinued(t *testing.T) {
	d := freshDecision()
	var parent [8]byte
	for i := range parent {
		parent[i] = byte(0xAA + i)
	}
	d.Continued = true
	d.ParentSpanID = parent

	in := freshInputs()
	start := time.Now()
	end := start.Add(5 * time.Millisecond)

	s := BuildServerSpan(d, in, start, end)
	pb := s.toProto()

	if len(pb.ParentSpanId) != 8 {
		t.Fatalf("ParentSpanId len = %d, want 8 for continued span", len(pb.ParentSpanId))
	}
	for i, b := range pb.ParentSpanId {
		if b != parent[i] {
			t.Errorf("ParentSpanId[%d] = %02x, want %02x", i, b, parent[i])
		}
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func attrStr(kvs []KV, key string) string {
	for _, kv := range kvs {
		if kv.Key == key && !kv.IsInt {
			return kv.Str
		}
	}
	return ""
}

func hasAttr(kvs []KV, key string) bool {
	for _, kv := range kvs {
		if kv.Key == key {
			return true
		}
	}
	return false
}

func protoKVMap(attrs []*commonpb.KeyValue) map[string]*commonpb.AnyValue {
	m := make(map[string]*commonpb.AnyValue, len(attrs))
	for _, kv := range attrs {
		m[kv.Key] = kv.Value
	}
	return m
}
