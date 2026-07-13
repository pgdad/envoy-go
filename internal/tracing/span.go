package tracing

import (
	"strconv"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// KV is a single span attribute: either a string or an int64 value.
type KV struct {
	Key   string
	Str   string
	Int   int64
	IsInt bool
}

// SpanInputs carries the per-request fields used to populate the 16 built-in
// span attributes (§11 D-TRACE-SPAN).
type SpanInputs struct {
	Method            string
	URL               string
	Protocol          string
	StatusCode        int
	UserAgent         string
	RequestSize       int64
	ResponseSize      int64
	UpstreamCluster   string
	DownstreamCluster string
	ResponseFlags     string
	NodeID            string
	Zone              string
	PeerAddress       string
	ClientTraceID     string
	// Authority is the request :authority (host:port) — the provider-neutral span
	// name carried for the Zipkin encoder (D-TRACE-ZIPKIN-SPAN-NAME). The OTLP wire
	// keeps Name="ingress"; Authority is a separate field, NOT a span attribute/tag.
	Authority string
}

// Span is the project's internal representation of a single ingress trace span.
// It is assembled by BuildServerSpan and converted to the wire proto by toProto.
type Span struct {
	TraceID      [16]byte
	SpanID       [8]byte
	ParentSpanID [8]byte
	Name         string
	Kind         tracepb.Span_SpanKind
	Start        time.Time
	End          time.Time
	TraceState   string
	ServiceName  string
	Attrs        []KV
	// Authority is the request :authority used as the Zipkin span name; the OTLP
	// Name stays "ingress" (D-TRACE-ZIPKIN-SPAN-NAME).
	Authority string
}

// BuildServerSpan assembles the single "ingress" SERVER span for a request using
// the tracing Decision d, the per-request SpanInputs in, and the wall-clock
// start/end times. The 16 built-in attributes are always emitted; the optional
// guid:x-client-trace-id attribute is emitted only when in.ClientTraceID != "".
func BuildServerSpan(d Decision, in SpanInputs, customTags []KV, start, end time.Time) *Span {
	attrs := make([]KV, 0, 17+len(customTags)) // 16 always-present + 1 optional + custom

	// 16 built-in attributes (§11 D-TRACE-SPAN)
	attrs = append(attrs,
		KV{Key: "http.method", Str: in.Method},
		KV{Key: "http.url", Str: in.URL},
		KV{Key: "http.protocol", Str: in.Protocol},
		// http.status_code, request_size, response_size: emitted as STRING to match
		// the reference Envoy cpp OTel tracer wire format (observed via the 0087
		// differential DUMP; reference emits STRING, not INT — D-TRACE-SPAN wire-parity).
		KV{Key: "http.status_code", Str: strconv.Itoa(in.StatusCode)},
		KV{Key: "component", Str: "proxy"},
		KV{Key: "upstream_cluster", Str: in.UpstreamCluster},
		KV{Key: "upstream_cluster.name", Str: in.UpstreamCluster},
		KV{Key: "downstream_cluster", Str: in.DownstreamCluster},
		KV{Key: "response_flags", Str: in.ResponseFlags},
		KV{Key: "request_size", Str: strconv.FormatInt(in.RequestSize, 10)},
		KV{Key: "response_size", Str: strconv.FormatInt(in.ResponseSize, 10)},
		KV{Key: "user_agent", Str: in.UserAgent},
		KV{Key: "guid:x-request-id", Str: d.RequestID},
		KV{Key: "node_id", Str: in.NodeID},
		KV{Key: "zone", Str: in.Zone},
		KV{Key: "peer.address", Str: in.PeerAddress},
	)

	// Optional: guid:x-client-trace-id (only when the client sent one)
	if in.ClientTraceID != "" {
		attrs = append(attrs, KV{Key: "guid:x-client-trace-id", Str: in.ClientTraceID})
	}

	// Custom tags (literal). Upsert-by-key: a tag whose key collides with a built-in
	// OVERRIDES it (last-write-wins, matching the reference OTel tracer — SPEC-59 §11
	// arm precedence-otlp), NOT append-duplicate. The common non-colliding case is a
	// pure append. Cost is O(builtins) per tag — negligible (≤17 built-ins, few tags).
	for _, ct := range customTags {
		upsertAttr(&attrs, ct)
	}

	return &Span{
		TraceID:      d.TraceID,
		SpanID:       d.SpanID,
		ParentSpanID: d.ParentSpanID,
		Name:         "ingress",
		Kind:         tracepb.Span_SPAN_KIND_SERVER,
		Start:        start,
		End:          end,
		TraceState:   d.TraceState,
		Attrs:        attrs,
		Authority:    in.Authority,
	}
}

// upsertAttr sets ct by key: replaces an existing attribute with the same key
// (last-write-wins), else appends. Keeps envoy-go's OTLP toProto (which emits every
// KV as a distinct wire attribute) consistent with the reference's single overridden
// attribute AND with the Zipkin encoder's tags map (which already last-write-wins).
func upsertAttr(attrs *[]KV, ct KV) {
	for i := range *attrs {
		if (*attrs)[i].Key == ct.Key {
			(*attrs)[i] = ct
			return
		}
	}
	*attrs = append(*attrs, ct)
}

// toProto converts the internal Span to its OTLP wire representation.
// ParentSpanId is nil/empty when s.ParentSpanID is all-zero (root span) and
// an 8-byte slice otherwise (continued span).
func (s *Span) toProto() *tracepb.Span {
	traceID := make([]byte, 16)
	copy(traceID, s.TraceID[:])

	spanID := make([]byte, 8)
	copy(spanID, s.SpanID[:])

	var parentSpanID []byte
	if s.ParentSpanID != ([8]byte{}) {
		parentSpanID = make([]byte, 8)
		copy(parentSpanID, s.ParentSpanID[:])
	}

	attrs := make([]*commonpb.KeyValue, 0, len(s.Attrs))
	for _, kv := range s.Attrs {
		var val *commonpb.AnyValue
		if kv.IsInt {
			val = &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: kv.Int}}
		} else {
			val = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: kv.Str}}
		}
		attrs = append(attrs, &commonpb.KeyValue{Key: kv.Key, Value: val})
	}

	return &tracepb.Span{
		TraceId:           traceID,
		SpanId:            spanID,
		TraceState:        s.TraceState,
		ParentSpanId:      parentSpanID,
		Name:              s.Name,
		Kind:              s.Kind,
		StartTimeUnixNano: uint64(s.Start.UnixNano()),
		EndTimeUnixNano:   uint64(s.End.UnixNano()),
		Attributes:        attrs,
	}
}
