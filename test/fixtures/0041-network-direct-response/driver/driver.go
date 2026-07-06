// Package driver registers the 0041-network-direct-response cross-side
// differential fixture with the runner per phase 26.1 SPEC §8.2. It exercises
// the direct_response network filter: both reference Envoy v1.37.2 and envoy-go
// boot a single filter chain whose only network filter is
// envoy.filters.network.direct_response with an inline_string response; the
// filter writes the static body "envoy-go-direct-response\n" in
// OnNewConnection then closes (FlushWrite), and the driver asserts the returned
// bytes equal that body byte-for-byte on both sides.
package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0041-network-direct-response"
const refContainerListenerPort = 15000

func init() {
	fixture.RegisterFixture(fixtureName, &directResponseDriver{})
}

type directResponseDriver struct{}

func (directResponseDriver) BackendCount() int           { return 1 } // runner fatals on n<1; spare TCP-echo backend is unused by the direct_response network filter
func (directResponseDriver) SubjectListenerName() string { return "l_dr" }
func (directResponseDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

func (directResponseDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("0041-network-direct-response: expected 1 backend port, got %d", len(backendPorts)))
	}
	// The direct_response network filter writes a static body and does NOT
	// route to a cluster, but envoy-go's cluster manager boots BEFORE the
	// listener manager and rejects a zero-cluster bootstrap; the c_echo cluster
	// (mirroring 0000-tcp-echo) satisfies boot on both sides and keeps the two
	// configs shape-identical. The direct_response @type is the
	// envoy.extensions.filters.network.direct_response.v3.Config URL (note
	// Config, not DirectResponse).
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_dr
      address:
        socket_address: { address: 0.0.0.0, port_value: 15000 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.direct_response
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config
                response: { inline_string: "envoy-go-direct-response\n" }
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

func (directResponseDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("0041-network-direct-response: expected 1 backend port, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0041, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_dr
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.direct_response
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config
                response: { inline_string: "envoy-go-direct-response\n" }
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

// DriveReference runs the driver logic against the reference proxy's listener
// address. Returns all received bytes (the static direct_response body).
func (directResponseDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	b, err := helpers.TCPRoundTrip(ctx, addr, nil, time.Second)
	if err != nil {
		return nil, fmt.Errorf("ref drive: %w", err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("0041-network-direct-response: ref drive returned 0 bytes (direct_response produced no output)")
	}
	return b, nil
}

// DriveSubject runs the driver logic against the subject proxy's listener
// address. Returns all received bytes (the static direct_response body).
func (directResponseDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	b, err := helpers.TCPRoundTrip(ctx, addr, nil, time.Second)
	if err != nil {
		return nil, fmt.Errorf("subj drive: %w", err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("0041-network-direct-response: subj drive returned 0 bytes (direct_response produced no output)")
	}
	return b, nil
}

func (directResponseDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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
