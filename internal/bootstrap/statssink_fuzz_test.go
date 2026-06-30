package bootstrap

import (
	"bytes"
	"testing"
)

// FuzzStatsSinkConfigParse exercises the top-level stats_sinks[]/stats_flush_interval
// parse arm (phase 47.1 Task 6) end-to-end through Load for arbitrary bootstrap
// document bytes. Load MUST NOT panic on any input; a returned error is fine
// (D-MS-FUZZER). The untrusted boundary is the bootstrap config carrying a
// metrics_service stats_sinks[] entry (the FuzzParseOpenTelemetryAccessLogConfig
// parse-fuzzer precedent).
//
// The seeds exercise: a valid metrics_service accept, each strict-reject arm
// (report_counters_as_deltas / emit_tags_as_labels / histogram_emit_mode / V2 /
// google_grpc / empty cluster / sibling StatsdSink / stats_flush_on_admin), the
// inert stats_flush_interval, plus degenerate/garbage documents.
func FuzzStatsSinkConfigParse(f *testing.F) {
	const head = `node: { id: mc-node, cluster: mc-cluster }
admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters: []
`
	const msType = "type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig"
	const statsdType = "type.googleapis.com/envoy.config.metrics.v3.StatsdSink"

	// valid accept (default knobs)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + msType + `
      transport_api_version: V3
      grpc_service:
        envoy_grpc:
          cluster_name: mc
`))
	// explicit flush interval
	f.Add([]byte(head + `stats_flush_interval: 2s
stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + msType + `
      grpc_service:
        envoy_grpc:
          cluster_name: mc
`))
	// report_counters_as_deltas:true (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + msType + `
      report_counters_as_deltas: true
      grpc_service:
        envoy_grpc:
          cluster_name: mc
`))
	// emit_tags_as_labels:true (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + msType + `
      emit_tags_as_labels: true
      grpc_service:
        envoy_grpc:
          cluster_name: mc
`))
	// histogram_emit_mode non-default (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + msType + `
      histogram_emit_mode: SUMMARY
      grpc_service:
        envoy_grpc:
          cluster_name: mc
`))
	// transport_api_version V2 (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + msType + `
      transport_api_version: V2
      grpc_service:
        envoy_grpc:
          cluster_name: mc
`))
	// google_grpc (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + msType + `
      grpc_service:
        google_grpc:
          target_uri: 127.0.0.1:50051
          stat_prefix: mc
`))
	// empty cluster_name (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + msType + `
      grpc_service:
        envoy_grpc:
          cluster_name: ""
`))
	// statsd tcp_cluster_name (UDP-only reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      tcp_cluster_name: statsd
`))
	// stats_flush_on_admin (reject)
	f.Add([]byte(head + `stats_flush_on_admin: true
stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + msType + `
      grpc_service:
        envoy_grpc:
          cluster_name: mc
`))
	// inert stats_flush_interval (no sinks)
	f.Add([]byte(head + `stats_flush_interval: 3s
`))
	// degenerate / garbage
	f.Add([]byte{})
	f.Add([]byte("\x00\x00\x00"))
	f.Add([]byte("stats_sinks: [{}]\n"))
	f.Add([]byte(head + "stats_sinks: [{typed_config: {\"@type\": garbage}}]\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Load MUST NOT panic regardless of data content; an error return is fine.
		_, _ = Load(bytes.NewReader(data))
	})
}
