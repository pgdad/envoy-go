package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"

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

// buildBinaryOrSkip compiles cmd/envoy-go into a temporary directory and
// returns the path to the resulting binary. If the build fails the test is
// fatally failed. The binary is placed in t.TempDir() so it is cleaned up
// automatically when the test ends.
func buildBinaryOrSkip(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "envoy-go")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// pkiFixture0002 returns the absolute path to the fixture-0002 pki/ directory,
// resolved relative to this source file so the test works from any working
// directory.
func pkiFixture0002(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate pki directory")
	}
	// thisFile is .../cmd/envoy-go/main_test.go; pki/ is at
	// ../../test/fixtures/0002-tls-tcp/pki relative to this file.
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "test", "fixtures", "0002-tls-tcp", "pki")
}

// TestEnvoyGoBinary_HCMSmoke exercises the phase-04 dataplane: a single HTTP/1.1
// listener serving an HCM direct_response. Asserts the same per-listener
// ready-sentinel format and that a direct_response is properly served with correct
// status, body, and Server header.
func TestEnvoyGoBinary_HCMSmoke(t *testing.T) {
	listenerPort := freeTCPPort(t)
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
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, listenerPort)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		_, _ = cmd.Process.Wait()
	}()

	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	addrs := waitForReadySentinels(t, stdout, []string{"l_http"}, 15*time.Second)

	listenerAddr := addrs["l_http"]
	conn, err := net.DialTimeout("tcp", listenerAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	_, _ = conn.Write([]byte("GET /health HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))
	body, _ := io.ReadAll(conn)
	got := string(body)
	if !strings.Contains(got, "HTTP/1.1 200 OK") {
		t.Errorf("status line missing from response:\n%s", got)
	}
	if !strings.Contains(got, "OK\n") {
		t.Errorf("expected body 'OK\\n' in response:\n%s", got)
	}
	if !strings.Contains(got, "Server: envoy") {
		t.Errorf("expected 'Server: envoy' header in response:\n%s", got)
	}
}

// TestMain_StatsPrometheusEndpointResponds is the phase-06.1 Task 12 boot-wiring
// smoke test: it boots the binary on a minimal HCM bootstrap (the smallest
// existing variant), waits for ready sentinels, then GETs
// /stats/prometheus on the admin port and asserts the body contains
// `# HELP envoy_server_live` (admin's own metric, allocated on whichever
// Registry the admin server holds) AND `# HELP envoy_listener_downstream_cx_total`
// (listener-scope metric, allocated by the listener manager on `bs.Stats` at
// Task 10).
//
// Pre-Task-12 the admin server held a throwaway Registry (not bs.Stats), so
// /stats/prometheus walked the throwaway — server.live is there (admin
// allocates it on whatever it gets) but the listener / cluster / HCM
// metrics are invisible because they live on bs.Stats. The
// envoy_listener_downstream_cx_total assertion is the unification signal:
// post-Task-12 the admin walks bs.Stats and every metric the binary
// allocates is observable (GREEN). Verifies SPEC §5.4 boot wiring +
// §5.7 (server.live exposition).
func TestMain_StatsPrometheusEndpointResponds(t *testing.T) {
	listenerPort := freeTCPPort(t)
	adminPort := freeTCPPort(t)

	bin := buildBinaryOrSkip(t)

	cfgPath := filepath.Join(t.TempDir(), "envoy-go.yaml")
	cfg := fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, listenerPort)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		_, _ = cmd.Process.Wait()
	}()

	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	_ = waitForReadySentinels(t, stdout, []string{"l_http"}, 15*time.Second)

	adminAddr := fmt.Sprintf("127.0.0.1:%d", adminPort)
	resp, err := http.Get("http://" + adminAddr + "/stats/prometheus")
	if err != nil {
		t.Fatalf("GET /stats/prometheus: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("# HELP envoy_server_live")) {
		t.Errorf("body missing # HELP envoy_server_live\n--- body ---\n%s", body)
	}
	// Unification signal: pre-Task-12 the listener metrics live on bs.Stats
	// while admin walks a throwaway Registry. Post-Task-12 admin walks
	// bs.Stats and the listener-scope metric is observable.
	if !bytes.Contains(body, []byte("# HELP envoy_listener_downstream_cx_total")) {
		t.Errorf("body missing # HELP envoy_listener_downstream_cx_total (Registry unification not complete)\n--- body ---\n%s", body)
	}
}

// TestEnvoyGoBinary_TLSInspectorBootWiring is the phase-07.2 (Task 11)
// boot-wiring smoke test: it boots the binary on a bootstrap that declares
// `listener_filters: [{name: envoy.filters.listener.tls_inspector,
// typed_config: TlsInspector}]` and asserts the binary reaches the ready
// sentinel without error. This exercises the full Task-11 boot wiring:
//
//  1. The internal/bootstrap blank-import of tls_inspector v3 (without it,
//     protojson would error "type not registered" at parse time);
//  2. main.go allocating a *listenerfilter.ListenerFilterRegistry,
//     registering tls_inspector.New under tls_inspector.TypeURL, calling
//     Freeze(), and threading it into NewManagerWithBaseDirAndAllowH2C
//     (without it, the per-listener parser would error
//     "listener_filters[]: registry is nil but listener_filters is non-empty"
//     or fail to resolve the type_url);
//  3. The frozen registry's Lookup path resolving tls_inspector at
//     listener-build time (without it, the per-listener parser would error
//     "unknown listener filter type_url").
//
// The bootstrap is plaintext-on-loopback (no TLS handshake exercised — the
// test asserts only that boot succeeds). End-to-end SNI dispatch is covered
// by fixture-0002 and the unit-test integration_test.go (Task 12).
//
// Pre-Task-11, with main.go threading nil for lfRegistry, the listener
// manager's parseListenerFilters would error and main() would log.Fatalf
// before emitting the ready sentinel; waitForReadySentinels would time out.
func TestEnvoyGoBinary_TLSInspectorBootWiring(t *testing.T) {
	listenerPort := freeTCPPort(t)
	adminPort := freeTCPPort(t)

	bin := buildBinaryOrSkip(t)

	cfgPath := filepath.Join(t.TempDir(), "envoy-go.yaml")
	cfg := fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_tls
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      listener_filters:
        - name: envoy.filters.listener.tls_inspector
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tls
                cluster: c_unused
  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, listenerPort)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		_, _ = cmd.Process.Wait()
	}()

	// Capture stderr so a boot failure surfaces in the test log.
	var stderrBuf bytes.Buffer
	go func() { _, _ = io.Copy(&stderrBuf, stderr) }()

	addrs := waitForReadySentinels(t, stdout, []string{"l_tls"}, 15*time.Second)
	if addrs["l_tls"] == "" {
		t.Fatalf("missing l_tls ready sentinel; stderr:\n%s", stderrBuf.String())
	}
}

// TestEnvoyGoBinary_AccessLogSmoke boots the binary with an HCM listener that
// has a file access_log configured, makes one HTTP/1.1 request, and asserts
// that the access-log file is non-empty after the process exits (write-on-close
// flush). The log path is inside t.TempDir() so it is cleaned up automatically.
//
// Verifies SPEC §5.3 boot wiring: the AsyncFileSink is opened by main.go and
// threaded through to the HCM filter via the listener manager.
func TestEnvoyGoBinary_AccessLogSmoke(t *testing.T) {
	listenerPort := freeTCPPort(t)
	adminPort := freeTCPPort(t)

	tmp := t.TempDir()
	bin := buildBinaryOrSkip(t)

	logPath := filepath.Join(tmp, "access.log")

	cfgPath := filepath.Join(tmp, "envoy-go.yaml")
	cfg := fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                access_log:
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: %s
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, listenerPort, logPath)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		_, _ = cmd.Process.Wait()
	}()

	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	addrs := waitForReadySentinels(t, stdout, []string{"l_http"}, 15*time.Second)

	listenerAddr := addrs["l_http"]
	conn, err := net.DialTimeout("tcp", listenerAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	_, _ = conn.Write([]byte("GET /health HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))
	body, _ := io.ReadAll(conn)
	if !strings.Contains(string(body), "HTTP/1.1 200 OK") {
		t.Fatalf("unexpected response: %s", body)
	}

	// Signal shutdown so the defer-Close on the sink flushes buffered entries.
	_ = cmd.Process.Signal(os.Interrupt)
	_, _ = cmd.Process.Wait()

	// Assert the access-log file is non-empty: at least one line was written.
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read access log %q: %v", logPath, err)
	}
	if len(logData) == 0 {
		t.Errorf("access log %q is empty — sink not flushed or not wired", logPath)
	}
}

// TestEnvoyGoBinary_H2Smoke exercises the phase-05.1 dataplane: a single
// HTTP/2-over-TLS listener with ALPN "h2", serving an HCM direct_response.
// Asserts that:
//   - the binary starts and emits the ready sentinel for listener "l_h2"
//   - an http2.Transport GET / receives HTTP 200 with body "OK\n"
//   - resp.ProtoMajor == 2 (confirming H2 framing end-to-end)
//
// PKI: uses fixture-0002 server-alpha cert/key (DNS SAN: alpha.envoy-go.test).
// The client uses InsecureSkipVerify: true so no CA chain validation is needed;
// ServerName is set to alpha.envoy-go.test to satisfy the SNI/cert match.
//
// Smoke-only: production tests must verify CA chain (this test uses
// InsecureSkipVerify: true and is intended only as a binary-level
// smoke check that the H2 dataplane wires up end-to-end).
func TestEnvoyGoBinary_H2Smoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke test in -short mode")
	}

	pki := pkiFixture0002(t)
	certPath := filepath.Join(pki, "server-alpha.pem")
	keyPath := filepath.Join(pki, "server-alpha.key.pem")

	// Read the cert and key bytes so we can embed them inline in the YAML,
	// avoiding any relative-path resolution issues at binary runtime.
	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}

	listenerPort := freeTCPPort(t)
	adminPort := freeTCPPort(t)

	bin := buildBinaryOrSkip(t)

	// Build the inline-string PEM blocks.  Each PEM line must be indented
	// more deeply than the "inline_string:" key (which sits at 22 spaces of
	// indentation) so YAML block-scalar rules are satisfied.
	makeInline := func(pem []byte, keyIndent string) string {
		bodyIndent := keyIndent + "  "
		var sb strings.Builder
		sb.WriteString("|\n")
		for _, line := range strings.Split(strings.TrimRight(string(pem), "\n"), "\n") {
			sb.WriteString(bodyIndent)
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
		return sb.String()
	}
	// "inline_string:" key is at 22 spaces inside the bootstrap YAML below.
	certIS := makeInline(certBytes, "                      ")
	keyIS := makeInline(keyBytes, "                      ")

	cfg := fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_h2
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                tls_certificates:
                  - certificate_chain:
                      inline_string: %s                    private_key:
                      inline_string: %s                alpn_protocols: ["h2"]
          filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP2
                stat_prefix: ingress_h2
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, listenerPort, certIS, keyIS)

	cfgPath := filepath.Join(t.TempDir(), "envoy-go.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		_, _ = cmd.Process.Wait()
	}()

	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	addrs := waitForReadySentinels(t, stdout, []string{"l_h2"}, 15*time.Second)
	listenerAddr := addrs["l_h2"]

	// Issue an HTTP/2 request via http2.Transport.
	// InsecureSkipVerify skips CA validation; ServerName must match the cert's
	// SAN (alpha.envoy-go.test) so the TLS handshake picks the right identity.
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // test-only; no CA chain needed
			ServerName:         "alpha.envoy-go.test",
			NextProtos:         []string{"h2"},
		},
	}
	defer transport.CloseIdleConnections()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+listenerAddr+"/", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK\n" {
		t.Errorf("body = %q, want %q", string(body), "OK\n")
	}
	if resp.ProtoMajor != 2 {
		t.Errorf("ProtoMajor = %d, want 2 (HTTP/2)", resp.ProtoMajor)
	}
}

// TestMain_FourNewAdminEndpointsRespond200 is the phase-08.1 (Task 10)
// boot-wiring smoke test for the four new read-only operator-introspection
// endpoints landed in 08.1 per SPEC §1: /config_dump, /clusters, /listeners,
// /server_info (ADR-0085 records the constructor-widening; ADRs 0086-0088
// record the per-endpoint shapes). Boots the binary on a representative
// HCM-with-router bootstrap, waits for the ready sentinel, then GETs each of
// the four endpoints from the admin port and asserts:
//
//   - status code 200 (200 OK contract per SPEC §5.4)
//   - body is non-empty (admin renders a real payload, not a stub)
//   - /config_dump body parses as JSON (SPEC §5.4.1: emits an
//     envoy.admin.v3.ConfigDump protojson document, NOT a YAML round-trip)
//   - /server_info body parses as JSON (SPEC §5.4.4: ServerInfo protojson)
//
// The /clusters and /listeners endpoints emit text/plain by 08.1 contract
// (the admin formats are operator-friendly text — JSON variants land later);
// they're asserted only on status + non-empty body. End-to-end shape
// assertions live in differential fixture 0009 (Task 14) and the in-package
// admin tests (Task 11).
//
// Pre-Task-10 main.go's admin.New(addr, bs.Stats) call had only 2 args while
// the constructor (Task 5) widened to 5: this test does NOT compile against
// the broken main.go (the binary build at the top of the test fails with
// "not enough arguments in call to admin.New"). Post-Task-10 wiring it
// passes. Verifies SPEC §4.2 boot wiring + SPEC §1 endpoint set.
func TestMain_FourNewAdminEndpointsRespond200(t *testing.T) {
	listenerPort := freeTCPPort(t)
	adminPort := freeTCPPort(t)

	bin := buildBinaryOrSkip(t)

	cfgPath := filepath.Join(t.TempDir(), "envoy-go.yaml")
	cfg := fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, listenerPort)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		_, _ = cmd.Process.Wait()
	}()

	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	_ = waitForReadySentinels(t, stdout, []string{"l_http"}, 15*time.Second)

	adminAddr := fmt.Sprintf("127.0.0.1:%d", adminPort)
	endpoints := []struct {
		path     string
		wantJSON bool
	}{
		{"/config_dump", true},
		{"/clusters", false},
		{"/listeners", false},
		{"/server_info", true},
	}
	for _, ep := range endpoints {
		ep := ep
		t.Run(ep.path, func(t *testing.T) {
			resp, err := http.Get("http://" + adminAddr + ep.path)
			if err != nil {
				t.Fatalf("GET %s: %v", ep.path, err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200", resp.StatusCode)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if len(body) == 0 {
				t.Errorf("empty body for %s", ep.path)
			}
			if ep.wantJSON {
				var generic map[string]interface{}
				if err := json.Unmarshal(body, &generic); err != nil {
					t.Errorf("body for %s is not valid JSON: %v\n--- body ---\n%s", ep.path, err, body)
				}
			}
		})
	}
}
