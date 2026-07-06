// Package driver registers the 0040-network-echo cross-side differential
// fixture with the runner per phase 26.1 SPEC §8.1. It exercises the network
// read-filter echo dispatch: both reference Envoy v1.37.2 and envoy-go boot a
// single filter chain whose only network filter is
// envoy.filters.network.echo, and the driver asserts byte-exact reflection of
// a deterministic multi-line payload on both sides.
package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0040-network-echo"
const refContainerListenerPort = 15000

func init() {
	fixture.RegisterFixture(fixtureName, &echoDriver{})
}

type echoDriver struct{}

func (echoDriver) BackendCount() int           { return 1 } // runner fatals on n<1; spare TCP-echo backend is unused by the echo network filter
func (echoDriver) SubjectListenerName() string { return "l_echo" }
func (echoDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

func (echoDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("0040-network-echo: expected 1 backend port, got %d", len(backendPorts)))
	}
	// The echo network filter reflects bytes directly and does NOT route to a
	// cluster, but envoy-go's cluster manager boots BEFORE the listener manager
	// and rejects a zero-cluster bootstrap; the c_echo cluster (mirroring
	// 0000-tcp-echo) satisfies boot on both sides and keeps the two configs
	// shape-identical. The echo @type is the CORRECTED extensions.* URL
	// (envoy.extensions.filters.network.echo.v3.Echo), verified via
	// proto.MessageName + boot smoke this session.
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_echo
      address:
        socket_address: { address: 0.0.0.0, port_value: 15000 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.echo
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.echo.v3.Echo
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
		panic(fmt.Sprintf("0040-network-echo: expected 1 backend port, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0040, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_echo
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.echo
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.echo.v3.Echo
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

// echoPayload is the deterministic multi-line payload sent by both
// DriveReference and DriveSubject. The newline-separated lines exercise the
// OnData read loop across buffer reads; the echo filter reflects the bytes
// verbatim, so the returned stream must equal this payload byte-for-byte on
// both sides.
func echoPayload() []byte {
	var p []byte
	for n := 0; n < 10; n++ {
		p = append(p, []byte(fmt.Sprintf("ping-%d\n", n))...)
	}
	return p
}

// DriveReference runs the driver logic against the reference proxy's listener
// address. Returns all received bytes.
func (echoDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	// No half-close: a downstream FIN co-arriving with the payload makes
	// reference Envoy v1.37.2's echo filter tear the connection down before
	// flushing the echo (characterized in this fixture's README); draining on
	// the idle timeout is byte-exact on both sides. See
	// helpers.TCPRoundTripNoHalfClose.
	b, err := helpers.TCPRoundTripNoHalfClose(ctx, addr, echoPayload(), time.Second)
	if err != nil {
		return nil, fmt.Errorf("ref drive: %w", err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("0040-network-echo: ref drive returned 0 bytes (echo produced no output)")
	}
	return b, nil
}

// DriveSubject runs the driver logic against the subject proxy's listener
// address. Returns all received bytes.
func (echoDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	b, err := helpers.TCPRoundTripNoHalfClose(ctx, addr, echoPayload(), time.Second)
	if err != nil {
		return nil, fmt.Errorf("subj drive: %w", err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("0040-network-echo: subj drive returned 0 bytes (echo produced no output)")
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
