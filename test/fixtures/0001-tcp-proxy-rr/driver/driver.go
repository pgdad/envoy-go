// Package driver implements the 0001-tcp-proxy-rr fixture's driver: 9 TCP
// round-trips against a 3-endpoint cluster, with per-proxy distribution
// asserted to be exactly [3, 3, 3].
package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const fixtureName = "0001-tcp-proxy-rr"
const refContainerListenerPort = 15001
const requestsPerSide = 9

func init() {
	fixture.RegisterFixture(fixtureName, &rrDriver{})
}

type rrDriver struct{}

func (rrDriver) BackendCount() int           { return 3 }
func (rrDriver) SubjectListenerName() string { return "l_tcp" }
func (rrDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

func (rrDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("0001: expected 3 backend ports, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 0.0.0.0, port_value: 15001 }
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
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (rrDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("0001: expected 3 backend ports, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0001, cluster: envoy-go-differential }
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
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

// Drive sends 9 TCP round-trips against whichever side(s) are non-"".
func (rrDriver) Drive(ctx context.Context, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error) {
	uid := randHex(6)
	var payloads [][]byte
	for n := 0; n < requestsPerSide; n++ {
		payloads = append(payloads, []byte(fmt.Sprintf("rr-%d-%s\n", n, uid)))
	}
	if refAddr != "" {
		var sb strings.Builder
		for i, p := range payloads {
			b, err := helpers.TCPRoundTrip(ctx, refAddr, p, time.Second)
			if err != nil {
				return nil, nil, fmt.Errorf("ref drive[%d]: %w", i, err)
			}
			sb.Write(b)
		}
		refBytes = []byte(sb.String())
	}
	if subjAddr != "" {
		var sb strings.Builder
		for i, p := range payloads {
			b, err := helpers.TCPRoundTrip(ctx, subjAddr, p, time.Second)
			if err != nil {
				return nil, nil, fmt.Errorf("subj drive[%d]: %w", i, err)
			}
			sb.Write(b)
		}
		subjBytes = []byte(sb.String())
	}
	return refBytes, subjBytes, nil
}

// AssertDistribution: each proxy's per-backend counts must be exactly [3,3,3]
// (9 requests / 3 endpoints, deterministic mod-index distribution).
func (rrDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	want := [3]uint64{3, 3, 3}
	for side, counts := range map[string][]uint64{"reference": refCounts, "subject": subjCounts} {
		if len(counts) != 3 {
			return fmt.Errorf("%s: expected 3 backend counts, got %d", side, len(counts))
		}
		var got [3]uint64
		copy(got[:], counts)
		if got != want {
			return fmt.Errorf("%s: distribution %v != %v", side, got, want)
		}
	}
	return nil
}

// ProbeAdmin reuses the phase-01 raw-socket /ready probe shape (mirrors
// fixture 0000's implementation; not extracted to a shared helper because the
// per-fixture admin-probe customization lands in later phases).
func (rrDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Compile-time check the driver implements both required and optional ifaces.
var (
	_ fixture.Driver               = (*rrDriver)(nil)
	_ fixture.DistributionAsserter = (*rrDriver)(nil)
)
