package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
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
	uid := randHex(6)
	var payload []byte
	for n := 0; n < 10; n++ {
		payload = append(payload, []byte(fmt.Sprintf("ping-%d-%s\n", n, uid))...)
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
	refBytes, err = probeReady(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref probe: %w", err)
	}
	subjBytes, err = probeReady(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj probe: %w", err)
	}
	return refBytes, subjBytes, nil
}

// probeReady issues a raw-socket GET /ready and reads the full wire response.
// Not using net/http.Client because the diff needs the status line and
// headers as on-the-wire bytes (net/http's response object discards some wire
// detail like header ordering that the diff's set-equal allow-list tolerates
// but the body/status exact-match rule does not).
func probeReady(ctx context.Context, addr string) ([]byte, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}
	req := "GET /ready HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return nil, err
	}
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, rerr := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if rerr != nil {
			break
		}
	}
	return buf, nil
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
