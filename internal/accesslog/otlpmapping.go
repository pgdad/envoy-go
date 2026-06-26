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

// buildLogRecord maps a Record into the LEAN built-in OTLP LogRecord: ONLY
// time_unix_nano is set — no observed_time, severity, body, or LogRecord.attributes
// (those are 45.2 operator templating). The time VALUE is non-deterministic; only
// PRESENCE is asserted cross-side.
func buildLogRecord(rec *Record) *logspb.LogRecord {
	return &logspb.LogRecord{
		TimeUnixNano: uint64(rec.StartTime.UnixNano()),
	}
}

// buildResource emits the 4 built-in Resource labels (always all 4 keys, in this
// order). disableBuiltinLabels drops them all wholesale (an empty Resource).
func buildResource(node *corev3.Node, logName string, disableBuiltinLabels bool) *resourcepb.Resource {
	if disableBuiltinLabels {
		return &resourcepb.Resource{}
	}
	kv := func(k, v string) *commonpb.KeyValue {
		return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
	}
	return &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
		kv("log_name", logName),
		kv("zone_name", node.GetLocality().GetZone()),
		kv("cluster_name", node.GetCluster()),
		kv("node_name", node.GetId()),
	}}
}

// buildExportRequest wraps a batch of LogRecords into one ExportLogsServiceRequest:
// one ResourceLogs (Resource = the built-in labels) → one ScopeLogs (Scope ABSENT)
// → the batch.
func buildExportRequest(batch []*logspb.LogRecord, node *corev3.Node, logName string, disableBuiltinLabels bool) *collogspb.ExportLogsServiceRequest {
	return &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource:  buildResource(node, logName, disableBuiltinLabels),
			ScopeLogs: []*logspb.ScopeLogs{{LogRecords: batch}},
		}},
	}
}
