package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

func (echoDriver) ReferenceListenerPort() int { return refContainerListenerPort }

func (echoDriver) ReferenceBootstrap() string {
	// host.docker.internal resolves to the host gateway from inside the
	// Envoy container; the harness sets the backend port via Cmd override
	// indirection. We bake the bootstrap at registration; the runner
	// substitutes the placeholder.
	return refBootstrap
}

func (echoDriver) SubjectConfig(refListenerPort, subjListenerPort, backendPort int) string {
	return fmt.Sprintf(`
listener:
  address: 127.0.0.1
  port: %d
upstream:
  address: 127.0.0.1
  port: %d
`, subjListenerPort, backendPort)
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
