package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/test/helpers"
)

// TestEnvoyGoBinary_TwoListenerCutover exercises the phase-02 dataplane: two
// listeners both proxying to the same single-endpoint cluster. Asserts the
// new per-listener ready-sentinel format (one `envoy-go listener <name> ready
// on <addr>` per listener, then a terminal `envoy-go ready`) and that each
// listener echoes byte-exact through the shared backend.
func TestEnvoyGoBinary_TwoListenerCutover(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer func() { _ = backend.Close() }()
	go acceptEcho(backend)
	backendPort := backend.Addr().(*net.TCPAddr).Port

	listenerPortA := freeTCPPort(t)
	listenerPortB := freeTCPPort(t)
	adminPort := freeTCPPort(t)

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "envoy-go")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cfgPath := filepath.Join(tmp, "envoy-go.yaml")
	cfg := fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_tcp_a
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp_a
                cluster: c_echo
    - name: l_tcp_b
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp_b
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
`, adminPort, listenerPortA, listenerPortB, backendPort)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	addrs := waitForReadySentinels(t, stdout, []string{"l_tcp_a", "l_tcp_b"}, 5*time.Second)

	for name, addr := range addrs {
		resp, err := helpers.TCPRoundTrip(ctx, addr, []byte("ping-7-cutover\n"), 500*time.Millisecond)
		if err != nil {
			t.Fatalf("%s round-trip: %v", name, err)
		}
		if string(resp) != "ping-7-cutover\n" {
			t.Errorf("%s: got %q, want %q", name, resp, "ping-7-cutover\n")
		}
	}
}

// waitForReadySentinels reads stdout line-by-line until every listener in
// `names` has a `envoy-go listener <name> ready on <addr>` line followed by
// the terminal `envoy-go ready` line. Returns the name → addr map.
func waitForReadySentinels(t *testing.T, r io.Reader, names []string, timeout time.Duration) map[string]string {
	t.Helper()
	want := map[string]struct{}{}
	for _, n := range names {
		want[n] = struct{}{}
	}
	got := map[string]string{}
	re := regexp.MustCompile(`^envoy-go listener (\S+) ready on (\S+)$`)
	deadline := time.Now().Add(timeout)
	br := bufio.NewReader(r)
	for time.Now().Before(deadline) {
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			t.Fatalf("ready: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "envoy-go ready" {
			if len(got) == len(names) {
				return got
			}
			t.Fatalf("terminal sentinel before all listeners (%d/%d)", len(got), len(names))
		}
		if m := re.FindStringSubmatch(line); m != nil {
			if _, expected := want[m[1]]; !expected {
				t.Fatalf("unexpected listener name in sentinel: %q", m[1])
			}
			if _, dup := got[m[1]]; dup {
				t.Fatalf("duplicate sentinel for listener %q", m[1])
			}
			got[m[1]] = m[2]
		}
	}
	t.Fatalf("ready sentinels not seen within %s; got=%v", timeout, got)
	return nil
}

// echoConn / acceptEcho / freeTCPPort are kept as in phase 01; deletion of the
// old TestEnvoyGoBinary_EchoesThroughUpstream removes the old waitForReady.
type echoConn struct{ net.Conn }

func acceptEcho(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) { defer func() { _ = c.Close() }(); _, _ = io.Copy(echoConn{c}, echoConn{c}) }(c)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}
