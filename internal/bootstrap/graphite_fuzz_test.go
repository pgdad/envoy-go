package bootstrap

import (
	"bytes"
	"testing"
)

// FuzzGraphiteStatsdSinkConfigParse exercises the graphite_statsd
// stats_sinks[] parse arm (phase 57 Task 2) end-to-end through Load for
// arbitrary bootstrap document bytes. Load MUST NOT panic on any input; a
// returned error is fine (mirrors FuzzDogStatsdSinkConfigParse's D-DSD-FUZZER
// posture for the graphite_statsd sink).
func FuzzGraphiteStatsdSinkConfigParse(f *testing.F) {
	const head = `node: { id: mc-node, cluster: mc-cluster }
admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters:
    - name: mc
      connect_timeout: 1s
      type: STATIC
      load_assignment:
        cluster_name: mc
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: {address: 127.0.0.1, port_value: 9999}
`
	const graphiteType = "type.googleapis.com/envoy.extensions.stat_sinks.graphite_statsd.v3.GraphiteStatsdSink"
	const dogStatsdType = "type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink"
	const statsdType = "type.googleapis.com/envoy.config.metrics.v3.StatsdSink"
	const msType = "type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig"

	// valid accept: address + prefix + max_bytes_per_datagram: 512
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.graphite_statsd
    typed_config:
      "@type": ` + graphiteType + `
      address:
        socket_address: {address: 127.0.0.1, port_value: 8125}
      prefix: gpfx
      max_bytes_per_datagram: 512
`))
	// default prefix (no prefix)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.graphite_statsd
    typed_config:
      "@type": ` + graphiteType + `
      address:
        socket_address: {address: 127.0.0.1, port_value: 8125}
`))
	// protocol: TCP accepted-and-ignored
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.graphite_statsd
    typed_config:
      "@type": ` + graphiteType + `
      address:
        socket_address: {address: 127.0.0.1, port_value: 8125, protocol: TCP}
`))
	// explicit max_bytes_per_datagram: 0 (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.graphite_statsd
    typed_config:
      "@type": ` + graphiteType + `
      address:
        socket_address: {address: 127.0.0.1, port_value: 8125}
      max_bytes_per_datagram: 0
`))
	// missing statsd_specifier (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.graphite_statsd
    typed_config:
      "@type": ` + graphiteType + `
      prefix: x
`))
	// hostname address (accept — the documented departure)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.graphite_statsd
    typed_config:
      "@type": ` + graphiteType + `
      address:
        socket_address: {address: localhost, port_value: 8125}
`))
	// coexisting FOUR sinks: metrics_service + statsd + dog_statsd + graphite_statsd
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + msType + `
      transport_api_version: V3
      grpc_service:
        envoy_grpc:
          cluster_name: mc
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8126 } }
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
  - name: envoy.stat_sinks.graphite_statsd
    typed_config:
      "@type": ` + graphiteType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8127 } }
`))
	// degenerate / garbage
	f.Add([]byte{})
	f.Add([]byte("\x00\x00\x00"))
	f.Add([]byte("stats_sinks: [{}]\n"))
	f.Add([]byte(head + "stats_sinks: [{typed_config: {\"@type\": " + graphiteType + "}}]\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Load MUST NOT panic regardless of data content; an error return is fine.
		_, _ = Load(bytes.NewReader(data))
	})
}
