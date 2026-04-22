package bootstrap

import (
	"bytes"
	"testing"
)

// envoyYAMLSeed is the literal bytes of test/fixtures/0000-tcp-echo/envoy.yaml
// as of phase 01 Task 6. Inlined so the fuzz target has no filesystem
// dependency when run by the CI short-budget job. When the fixture changes
// shape in later phases, update this constant verbatim from the file.
const envoyYAMLSeed = `# Reference upstream Envoy bootstrap for fixture 0000-tcp-echo.
#
# IN-CONTAINER PORT MAP:
#   15000 — TCP listener exposed to the host (the differential runner dials it
#           at the host-mapped port returned by testcontainers).
#   9901  — admin (used by harness wait.ForHTTP("/ready")).
#
# The cluster's endpoint address is "host.docker.internal" so the in-container
# Envoy can reach the host-side echo backend started by the runner. The runner
# substitutes the backend port into this bootstrap with a literal
# ` + "`strings.Replace`" + ` of "port_value: 0" before starting the reference container
# (see test/differential/runner_test.go). Phase 01 replaces the strings.Replace
# with real templating once a config loader exists.
#
# The reference Envoy image must have host-gateway aliasing (modern Docker
# Desktop and recent docker-ce on Linux both honor host.docker.internal via
# extra_hosts; the testcontainers wait.ForHTTP exercises this path).

admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }

static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 0.0.0.0, port_value: 15000 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: host.docker.internal
                      port_value: 0   # placeholder; runner replaces via strings.Replace("port_value: 0", ...) before start
`

func FuzzBootstrapLoad(f *testing.F) {
	// Seed corpus: the known-good sample, the fixture, plus degenerate
	// inputs. The CI short-budget invocation explores mutations of these
	// seeds.
	f.Add([]byte(sampleBootstrap))
	f.Add([]byte(envoyYAMLSeed))
	f.Add([]byte(""))
	f.Add([]byte(" "))
	f.Add([]byte("\x00\x00\x00"))
	f.Add([]byte("admin:"))
	f.Add([]byte("static_resources:\n  listeners: []\n  clusters: []"))
	// Deeply nested YAML.
	nested := bytes.Repeat([]byte("- "), 200)
	f.Add(nested)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Load MUST NOT panic. Any input returns either (*Bootstrap, nil) or
		// (nil, err starting with "bootstrap: ").
		_, _ = Load(bytes.NewReader(data))
	})
}
