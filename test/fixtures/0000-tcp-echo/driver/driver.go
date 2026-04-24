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

// echoPayload is the deterministic payload sent by both DriveReference and
// DriveSubject. A static form (line-numbered pings) gives the same debugging
// value as a per-call random uid without any per-call divergence.
func echoPayload() []byte {
	var p []byte
	for n := 0; n < 10; n++ {
		p = append(p, []byte(fmt.Sprintf("ping-%d\n", n))...)
	}
	return p
}

// DriveReference runs the fixture's driver logic against the reference
// proxy's listener address. Returns all received bytes.
func (echoDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	b, err := helpers.TCPRoundTrip(ctx, addr, echoPayload(), time.Second)
	if err != nil {
		return nil, fmt.Errorf("ref drive: %w", err)
	}
	return b, nil
}

// DriveSubject runs the fixture's driver logic against the subject
// proxy's listener address. Returns all received bytes.
func (echoDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	b, err := helpers.TCPRoundTrip(ctx, addr, echoPayload(), time.Second)
	if err != nil {
		return nil, fmt.Errorf("subj drive: %w", err)
	}
	return b, nil
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
