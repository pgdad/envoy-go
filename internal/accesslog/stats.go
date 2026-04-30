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
