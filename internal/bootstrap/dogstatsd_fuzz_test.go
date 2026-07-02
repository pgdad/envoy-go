package bootstrap

import (
	"bytes"
	"testing"
)

// FuzzDogStatsdSinkConfigParse exercises the dog_statsd stats_sinks[] parse arm
// (phase 49 Task 2) end-to-end through Load for arbitrary bootstrap document
// bytes. Load MUST NOT panic on any input; a returned error is fine (D-DSD-FUZZER).
func FuzzDogStatsdSinkConfigParse(f *testing.F) {
	const head = `node: { id: dsd-node, cluster: dsd-cluster }
admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters: []
`
	const dogStatsdType = "type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink"
	const statsdType = "type.googleapis.com/envoy.config.metrics.v3.StatsdSink"
	const msType = "type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig"

	// valid accept (socket_address + prefix)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
      prefix: myprefix
`))
	// default prefix (no prefix)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
`))
	// protocol: TCP accepted-and-ignored
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125, protocol: TCP } }
`))
	// max_bytes_per_datagram set (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
      max_bytes_per_datagram: 512
`))
	// missing dog_statsd_specifier (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      prefix: x
`))
	// coexisting metrics_service + statsd + dog_statsd
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
`))
	// degenerate / garbage
	f.Add([]byte{})
	f.Add([]byte("\x00\x00\x00"))
	f.Add([]byte("stats_sinks: [{}]\n"))
	f.Add([]byte(head + "stats_sinks: [{typed_config: {\"@type\": " + dogStatsdType + "}}]\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Load MUST NOT panic regardless of data content; an error return is fine.
		_, _ = Load(bytes.NewReader(data))
	})
}
