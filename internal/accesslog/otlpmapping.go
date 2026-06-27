package accesslog

// otlpmapping.go — phase 45.1 (OTLP access-log core, ADR-0258). Pure mapping of
// an access-log Record into the LEAN built-in OTLP LogRecord + the 4-label
// Resource + the ExportLogsServiceRequest envelope. No gRPC, no goroutine; the
// SPEC §11 live probe (contrib-v1.37.2) pins the built-in shape.

import (
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

// buildLogRecord maps a Record into an OTLP LogRecord: always time_unix_nano; when
// body != nil sets LogRecord.body = body.Eval(rec); when attrs is non-empty sets
// LogRecord.attributes (each value operator-substituted per record). The 45.1 LEAN
// built-in path is the body==nil && len(attrs)==0 case (ONLY time_unix_nano — no
// observed_time, severity, body, or attributes; byte-identical to 45.1). The time
// VALUE is non-deterministic; only PRESENCE is asserted cross-side.
func buildLogRecord(rec *Record, body *OTLPValueTemplate, attrs []OTLPAttrTemplate) *logspb.LogRecord {
	lr := &logspb.LogRecord{TimeUnixNano: uint64(rec.StartTime.UnixNano())}
	if body != nil {
		lr.Body = body.Eval(rec)
	}
	if len(attrs) > 0 {
		kvs := make([]*commonpb.KeyValue, len(attrs))
		for i, a := range attrs {
			kvs[i] = &commonpb.KeyValue{Key: a.Key, Value: a.Value.Eval(rec)}
		}
		lr.Attributes = kvs
	}
	return lr
}

// buildResource emits the 4 built-in Resource labels (always all 4 keys, in this
// order; dropped wholesale by disableBuiltinLabels), then ALWAYS (unconditionally)
// appends the literal resourceAttrs — which SURVIVE disableBuiltinLabels
// (AMEND-OPS-5). resourceAttrs are
// shared-immutable proto pointers reused for every export: treated as READ-ONLY
// (appended into a fresh slice, never mutated). An empty Resource results only when
// disableBuiltinLabels && len(resourceAttrs)==0 (append(nil, …) stays nil — the 45.1
// byte-identical empty path).
func buildResource(node *corev3.Node, logName string, disableBuiltinLabels bool, resourceAttrs []*commonpb.KeyValue) *resourcepb.Resource {
	var attrs []*commonpb.KeyValue
	if !disableBuiltinLabels {
		kv := func(k, v string) *commonpb.KeyValue {
			return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
		}
		attrs = []*commonpb.KeyValue{
			kv("log_name", logName),
			kv("zone_name", node.GetLocality().GetZone()),
			kv("cluster_name", node.GetCluster()),
			kv("node_name", node.GetId()),
		}
	}
	attrs = append(attrs, resourceAttrs...)
	return &resourcepb.Resource{Attributes: attrs}
}

// buildExportRequest wraps an ALREADY-BUILT batch of LogRecords into one
// ExportLogsServiceRequest: one ResourceLogs (Resource = the built-in labels THEN the
// literal resourceAttrs) → one ScopeLogs (Scope ABSENT) → the batch. The per-record
// body/attribute substitution happens in buildLogRecord at buffer-append time, NOT
// here; this wraps an already-built batch and only threads the resource-scoped
// resourceAttrs through (built once per Export).
func buildExportRequest(batch []*logspb.LogRecord, node *corev3.Node, logName string, disableBuiltinLabels bool, resourceAttrs []*commonpb.KeyValue) *collogspb.ExportLogsServiceRequest {
	return &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource:  buildResource(node, logName, disableBuiltinLabels, resourceAttrs),
			ScopeLogs: []*logspb.ScopeLogs{{LogRecords: batch}},
		}},
	}
}
