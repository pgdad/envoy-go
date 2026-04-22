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

func (echoDriver) ReferenceListenerPort() int { return refContainerListenerPort }

func (echoDriver) ReferenceBootstrap() string {
	// host.docker.internal resolves to the host gateway from inside the
	// Envoy container. We bake the bootstrap at registration with a literal
	// `port_value: 0` placeholder; the runner replaces it with the actual
	// backend port via strings.Replace before starting the container
	// (test/differential/runner_test.go). Phase 01 replaces this with
	// proper templating once a config loader exists.
	return refBootstrap
}

func (echoDriver) SubjectConfig(refListenerPort, subjListenerPort, backendPort, subjAdminPort int) string {
	_ = refListenerPort // phase 01 does not wire the reference listener port into the subject bootstrap
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
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: %d }
`, subjAdminPort, subjListenerPort, backendPort)
}

func (echoDriver) Drive(ctx context.Context, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error) {
	uid := randHex(6)
	var payload []byte
	for n := 0; n < 10; n++ {
		payload = append(payload, []byte(fmt.Sprintf("ping-%d-%s\n", n, uid))...)
	}
	refBytes, err = helpers.TCPRoundTrip(ctx, refAddr, payload, time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("ref drive: %w", err)
	}
	subjBytes, err = helpers.TCPRoundTrip(ctx, subjAddr, payload, time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("subj drive: %w", err)
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

// refBootstrap is the reference Envoy bootstrap with a placeholder
// `port_value: 0` that the runner replaces with the backend port at test
// time. The string-replacement is intentional and trivial; phase 01 replaces
// this with proper templating once a config loader exists.
const refBootstrap = `admin:
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
                      port_value: 0
`

// The runner (test/differential/runner_test.go) performs a strings.Replace
// on `port_value: 0` before passing the bootstrap to StartReferenceProxy.
// The placeholder is the per-fixture contract; the substitution is trivial
// today and will be replaced by proper templating in phase 01.
