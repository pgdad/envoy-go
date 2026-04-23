package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const fixtureName = "0000-tcp-echo"
const refContainerListenerPort = 15000

func init() {
	fixture.RegisterFixture(fixtureName, &echoDriver{})
}

type echoDriver struct{}

func (echoDriver) BackendCount() int           { return 1 }
func (echoDriver) SubjectListenerName() string { return "l_tcp" }
func (echoDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

func (echoDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("0000-tcp-echo: expected 1 backend port, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(`admin:
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
                      port_value: %d
`, backendPorts[0])
}

func (echoDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("0000-tcp-echo: expected 1 backend port, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0000, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: %d }
`, subjAdminPort, subjListenerPort, backendPorts[0])
}

func (echoDriver) Drive(ctx context.Context, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error) {
	// Deterministic payload: both sides receive the same bytes so byte-exact
	// echo comparison holds under the post-Task-7 runner pattern that calls
	// Drive separately per side. An earlier variant used a per-call randHex
	// uid, which diverged between the two calls; the static form has the same
	// debugging value (line-numbered pings) with no run-time state.
	var payload []byte
	for n := 0; n < 10; n++ {
		payload = append(payload, []byte(fmt.Sprintf("ping-%d\n", n))...)
	}
	if refAddr != "" {
		refBytes, err = helpers.TCPRoundTrip(ctx, refAddr, payload, time.Second)
		if err != nil {
			return nil, nil, fmt.Errorf("ref drive: %w", err)
		}
	}
	if subjAddr != "" {
		subjBytes, err = helpers.TCPRoundTrip(ctx, subjAddr, payload, time.Second)
		if err != nil {
			return nil, nil, fmt.Errorf("subj drive: %w", err)
		}
	}
	return refBytes, subjBytes, nil
}

func (echoDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref probe: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj probe: %w", err)
	}
	return refBytes, subjBytes, nil
}
