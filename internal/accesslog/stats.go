package accesslog

import "github.com/esalaine/envoy-go/internal/stats"

// RegisterDroppedCounter allocates the `server.accesslog_dropped` counter on
// reg per ADR-0069. The counter is allocated once per process (not once per
// sink) — multiple sinks share the same counter; per-sink debug visibility is
// through the rate-limited diagnostic log line in writer.go's Submit.
//
// Per 06.1 Rule SN5 (server.<rest> → envoy_server_<rest>, no labels), the
// Prometheus name is envoy_server_accesslog_dropped. Outside the 06.1 17-name
// allow-list — fixture 0005's differential ignores the metric per ADR-0062.
// Operator-visible at /stats/prometheus only.
func RegisterDroppedCounter(reg *stats.Registry) *stats.Counter {
	return reg.NewCounter("server.accesslog_dropped")
}

// RegisterGrpcSinkCounters allocates the two process-global gRPC-ALS sink
// counters (ADR-0255 / AMEND-ALS-1). Registered once per process when ≥1 gRPC
// ALS sink is built. STATIC names (no IsValidName guard — not wire/config
// derived). NOT a reuse of server.accesslog_dropped — the gRPC sink owns its
// own logs_dropped.
func RegisterGrpcSinkCounters(reg *stats.Registry) (written, dropped *stats.Counter) {
	return reg.NewCounter("access_logs.grpc_access_log.logs_written"),
		reg.NewCounter("access_logs.grpc_access_log.logs_dropped")
}

// RegisterOTLPSinkCounters allocates the two process-global OTLP access-log sink
// counters (ADR-0258). Registered once per process when ≥1 OTLP sink is built.
// STATIC names (no IsValidName guard — not wire/config derived; stat_prefix
// honoring is deferred). The OTLP sink owns its own logs_dropped (NOT a reuse of
// server.accesslog_dropped or the gRPC-ALS counter).
func RegisterOTLPSinkCounters(reg *stats.Registry) (written, dropped *stats.Counter) {
	return reg.NewCounter("access_logs.open_telemetry_access_log.logs_written"),
		reg.NewCounter("access_logs.open_telemetry_access_log.logs_dropped")
}
