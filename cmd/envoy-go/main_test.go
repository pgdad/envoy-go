package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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

	"github.com/pgdad/envoy-go/test/helpers"
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

// TestEnvoyGoBinary_ModeValidate is the phase-51 (Task 5) CLI-subprocess test
// for the new --mode validate flag. It asserts three exit-code contracts:
// (a) a valid config exits 0 with "configuration OK" on stdout, (b) an
// invalid config exits 1 with a recognizable error naming the failure on
// stderr, and (c) an unrecognized --mode value exits 2 (usage error, the
// same class as the pre-existing missing-`-c` case). The bad-config fixture
// (a TLS upstream cluster referencing a nonexistent cert file) exercises the
// same cluster.NewManagerWithBaseDir failure path validate.Bootstrap wraps,
// without needing any live socket.
func TestEnvoyGoBinary_ModeValidate(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "envoy-go")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	goodCfg := filepath.Join(t.TempDir(), "good.yaml")
	goodYAML := `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
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
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`
	if err := os.WriteFile(goodCfg, []byte(goodYAML), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	badCfg := filepath.Join(t.TempDir(), "bad.yaml")
	badYAML := `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners: []
  clusters:
    - name: c_tls_upstream
      type: STATIC
      connect_timeout: 1s
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
          common_tls_context:
            validation_context:
              trusted_ca:
                inline_string: "unused-placeholder"
            tls_certificates:
              - certificate_chain:
                  filename: does-not-exist-cert.pem
                private_key:
                  filename: does-not-exist-key.pem
      load_assignment:
        cluster_name: c_tls_upstream
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`
	if err := os.WriteFile(badCfg, []byte(badYAML), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// (a) Good config: exit 0, stdout contains "configuration OK".
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-c", goodCfg, "--mode", "validate")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("good config: --mode validate failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "configuration OK") {
		t.Errorf("good config: stdout = %q, want it to contain %q", out, "configuration OK")
	}

	// (b) Bad config: exit 1, stderr contains a recognizable substring.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	cmd2 := exec.CommandContext(ctx2, bin, "-c", badCfg, "--mode", "validate")
	out2, err2 := cmd2.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err2, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("bad config: got err=%v, want *exec.ExitError with exit code 1 (out=%s)", err2, out2)
	}
	if !strings.Contains(string(out2), "does-not-exist-cert.pem") {
		t.Errorf("bad config: output = %q, want it to name the missing file", out2)
	}

	// (c) Unknown --mode value: exit 2 (usage error).
	ctx3, cancel3 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel3()
	cmd3 := exec.CommandContext(ctx3, bin, "-c", goodCfg, "--mode", "bogus")
	out3, err3 := cmd3.CombinedOutput()
	var exitErr3 *exec.ExitError
	if !errors.As(err3, &exitErr3) || exitErr3.ExitCode() != 2 {
		t.Fatalf("unknown --mode: got err=%v, want *exec.ExitError with exit code 2 (out=%s)", err3, out3)
	}
}

// bootPanicVisibleDeadline bounds the trigger process. MEASURED: on a tree
// carrying the phase-78 fix the binary panics and dies in 0.009-0.011 s wall
// (5 consecutive runs), so this is ~2700x headroom and exists only to bound a
// REGRESSION, never a healthy run. On the pre-fix tree the same process runs
// until the deadline kills it, having printed ZERO bytes.
const bootPanicVisibleDeadline = 30 * time.Second

// bootPanicTriggerYAML renders the ONLY config-reachable in-window boot panic:
// two HTTP connection managers on DISTINCT listener addresses sharing one
// stat_prefix, so the second chain's `http.<prefix>.downstream_rq_total`
// counter collides in the stats registry.
//
// Every element is load-bearing and a probe's INPUT is a claim:
//   - DISTINCT addresses: two listeners on the SAME address die at
//     `bind: address already in use` (exit 1) BEFORE registration runs, because
//     registerListenerMetrics runs post-bind.
//   - >=1 cluster: a `clusters: []` bootstrap dies at
//     `cluster: zero clusters in bootstrap`, which is a BROKEN ARM, not a result.
//   - a stats_sinks[] entry: it makes statsFlusher non-nil so the flusher
//     goroutine and the `flusherDone` wait actually exist. The statsd sink is UDP
//     and needs no live receiver.
//   - an http_filters[] router: the HCM rejects a chain with zero http filters
//     before it ever registers a counter.
func bootPanicTriggerYAML(adminPort, listenerAPort, listenerBPort, statsdPort, backendPort int, statPrefixA, statPrefixB string) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": type.googleapis.com/envoy.config.metrics.v3.StatsdSink
      address:
        socket_address: { protocol: UDP, address: 127.0.0.1, port_value: %d }
      prefix: p78
stats_flush_interval: 0.5s
static_resources:
  listeners:
    - name: l_a
      address: { socket_address: { address: 127.0.0.1, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: %s
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
                route_config:
                  name: r_a
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response: { status: 200, body: { inline_string: "a" } }
    - name: l_b
      address: { socket_address: { address: 127.0.0.1, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: %s
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
                route_config:
                  name: r_b
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response: { status: 200, body: { inline_string: "b" } }
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
                    socket_address: { address: 127.0.0.1, port_value: %d }
`, adminPort, statsdPort, listenerAPort, statPrefixA, listenerBPort, statPrefixB, backendPort)
}

// TestMain_BootPanicIsVisible is the phase-78 black-box guard (ADR-0300): a
// panic unwinding through main() during boot must be VISIBLE — it must kill the
// process, print the panic and print a goroutine dump. Before phase 78 the
// shutdown defer that waits on `<-flusherDone` was registered at the TOP of a
// 68-line boot window while the channel was only closed at the BOTTOM, so any
// in-window panic unwound into a permanent block and the process HUNG with ZERO
// bytes on both streams.
//
// THE ASSERTION IS A CONJUNCTION, and each half has an EXECUTED counter-example
// (SPEC 78 R5, PLAN 78 SS1.2) — do not weaken any of it:
//   - exit status alone is BLIND: through this exact harness shape a HEALTHY
//     boot and a HUNG boot are byte-identical on every status observable —
//     ctx.Err() is DeadlineExceeded, the run error is `signal: killed` and
//     ExitCode() is -1 on BOTH. They differ only in OUTPUT.
//   - output alone is satisfied by a PRINT-THEN-HANG build: recover(), print the
//     exact panic text, then block forever. Every string assertion passes while
//     the process never dies.
//   - output VOLUME is satisfied by NOISE: on the broken tree the still-running
//     flush ticker writes 500-1300 bytes of `statsd udp write failed` lines to
//     stderr while hanging. Assert the panic TEXT, never a byte count.
//
// So the hang is detected from the DEADLINE STATE (ctx.Err() ==
// context.DeadlineExceeded), NOT from an exit code. exec.CommandContext's default
// cancel action is Process.Kill (SIGKILL) and MUST NOT be changed to SIGTERM: a
// SIGTERM cancels the server ctx, which releases the very wait that is hanging,
// so a SIGTERM-based harness cannot falsify this contract at all — it reads a
// hang as "printed, just slow" (measured: exit 124 at 8.006 s WITH the panic
// text present).
//
// TRIGGER DEPENDENCY — this test's trigger is the ONLY config-reachable
// in-window panic in the tree, and there is NO fallback. It reaches the panic at
// internal/stats/registry.go:107 ("stats: duplicate metric registration")
// through the HCM per-filter counter registration in internal/filter/hcm/config.go
// (prefix derivation `prefix := "http." + statPrefix + "."` at :352; the five
// `registry.NewCounter(prefix + ...)` calls at :358-362, of which
// `downstream_rq_total` at :358 is the one that collides).
// If a future row makes duplicate registration a get-or-create (reference
// parity) or a clean config reject, this test goes RED — that is intended.
// RE-POINT THE TRIGGER; DO NOT RELAX THE ASSERTION. A guard that stops
// triggering must fail loudly, never pass vacuously.
func TestMain_BootPanicIsVisible(t *testing.T) {
	bin := buildBinaryOrSkip(t)

	cfgPath := filepath.Join(t.TempDir(), "boot-panic-trigger.yaml")
	// The SAME stat_prefix on both chains is the trigger. Distinct addresses.
	cfg := bootPanicTriggerYAML(
		freeTCPPort(t), freeTCPPort(t), freeTCPPort(t), freeTCPPort(t), freeTCPPort(t),
		"dup_prefix", "dup_prefix",
	)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), bootPanicVisibleDeadline)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	// LEG 1 — THE PROCESS DID NOT HANG. Sole t.Fatalf in this test: after a hang
	// there is no output left to assert on, and every later leg would report a
	// second, derived failure for one defect.
	if ctx.Err() == context.DeadlineExceeded {
		// stdout is the discriminator between the two ways this leg goes red:
		// ZERO bytes means the phase-78 defect is back (a panic swallowed by a
		// blocking defer in main()'s boot window — see the <-flusherDone wait in
		// cmd/envoy-go/main.go); the `envoy-go ... ready` sentinels mean the
		// process booted HEALTHILY, i.e. the trigger stopped triggering and must
		// be RE-POINTED (see this test's doc comment).
		t.Fatalf("BOOT DID NOT TERMINATE within %v: stdout=%d bytes %q, stderr=%d bytes, run error=%v; "+
			"zero stdout => a boot-window panic is being swallowed by a blocking defer; "+
			"a ready sentinel on stdout => the trigger no longer panics and must be re-pointed",
			bootPanicVisibleDeadline, stdout.Len(), firstLineOf(stdout.String()), stderr.Len(), runErr)
	}

	// LEG 2 — Go's unrecovered-panic exit status is exactly 2. t.Errorf, not
	// Fatalf, so legs 3 and 4 are not dead code.
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		t.Errorf("exit status: got 0 (process exited cleanly), want 2 (unrecovered panic); stderr=%q", stderr.String())
	case errors.As(runErr, &exitErr):
		if got := exitErr.ExitCode(); got != 2 {
			t.Errorf("exit status: got %d, want 2 (unrecovered panic); run error=%v; stderr=%q", got, runErr, stderr.String())
		}
	default:
		t.Errorf("exit status: run failed without an *exec.ExitError: %v", runErr)
	}

	// LEG 3 — stderr NAMES the panic. This is what pins the guard to a real
	// defect rather than to "something went wrong", and it is what a byte-count
	// assertion cannot do: the hanging tree emits hundreds of bytes of unrelated
	// statsd write-failure noise.
	const wantPanic = `stats: duplicate metric registration`
	if !strings.Contains(stderr.String(), wantPanic) {
		t.Errorf("stderr does not name the panic: want a line containing %q; got %d bytes:\n%s",
			wantPanic, stderr.Len(), stderr.String())
	}

	// LEG 4 — stderr carries Go's panic dump: the `panic:` header AND at least
	// one goroutine stack. A recover-and-log build satisfies leg 3 but not this.
	if !strings.Contains(stderr.String(), "panic: ") {
		t.Errorf("stderr lacks the %q header (a recovered-and-reprinted panic is not a panic dump); got %d bytes:\n%s",
			"panic: ", stderr.Len(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "goroutine ") {
		t.Errorf("stderr lacks a goroutine dump (want a %q frame header); got %d bytes:\n%s",
			"goroutine ", stderr.Len(), stderr.String())
	}
}

// firstLineOf returns s up to (not including) the first newline. Used to keep
// the boot-hang failure message short while still showing whether the subject
// printed a ready sentinel.
func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ---------------------------------------------------------------------------
// Phase 78 (ADR-0300) — the two STRUCTURAL arms of the boot-panic-visibility
// guard. They are structural because there is no config-reachable POST-anchor
// panic to drive a behavioral one: the only config-reachable boot-window panic
// trigger (duplicate stat_prefix -> internal/stats/registry.go:107, registered
// at internal/filter/hcm/config.go:352-362) fires inside boot.Construct, i.e.
// PRE-anchor, and a pre-anchor panic is fixed by the relocation alone. The
// behavioral guard therefore PASSES on a tree that still hangs post-anchor
// (EXECUTED, PLAN 78 T3); these two arms are what closes that gap.
// ---------------------------------------------------------------------------

// bootPanicVisibilityMainGo parses cmd/envoy-go/main.go and returns its AST plus
// the FileSet needed to turn token.Pos into line numbers. The path is resolved
// from THIS source file via runtime.Caller (the same technique as pkiFixture0002)
// rather than from the process working directory, so the arms are correct no
// matter how the test binary is invoked.
func bootPanicVisibilityMainGo(t *testing.T) (*token.FileSet, *ast.File, string) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate main.go")
	}
	path := filepath.Join(filepath.Dir(thisFile), "main.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return fset, f, path
}

// isFlusherDoneReceive reports whether n is the expression `<-flusherDone`.
func isFlusherDoneReceive(n ast.Node) bool {
	u, ok := n.(*ast.UnaryExpr)
	if !ok || u.Op != token.ARROW {
		return false
	}
	id, ok := u.X.(*ast.Ident)
	return ok && id.Name == "flusherDone"
}

// isFlusherDoneClose reports whether n is the call `close(flusherDone)`.
func isFlusherDoneClose(n ast.Node) bool {
	c, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	fn, ok := c.Fun.(*ast.Ident)
	if !ok || fn.Name != "close" || len(c.Args) != 1 {
		return false
	}
	arg, ok := c.Args[0].(*ast.Ident)
	return ok && arg.Name == "flusherDone"
}

// TestBootPanicVisibility_FlusherDoneWaitIsAfterEveryClose is phase-78 structural
// arm 2: every `<-flusherDone` receive in main.go must sit at a line strictly
// GREATER than every `close(flusherDone)` call.
//
// Why it is red-on-regression: the phase-78 defect is a `defer func(){ <-flusherDone
// ... }()` registered ~70 lines BEFORE anything closes flusherDone. Every panic
// unwinding through main() in that window blocks forever on a channel that nothing
// has closed, producing a zero-byte silent hang. Moving the receive after both
// close sites is exactly what makes the boot window panic-visible, and that is a
// pure source-ORDER property, which is why this arm is structural.
//
// ⚠️ This arm is NOT sufficient on its own — it is GREEN on the naively-relocated
// tree, which still hangs. See the companion arm
// TestBootPanicVisibility_FlusherDoneWaitIsPrecededByCancel.
//
// Anti-vacuity: a structural test that finds nothing and says nothing is GREEN for
// the wrong reason. Both counts are asserted non-zero BEFORE the ordering leg, and
// the two zero cases are reported separately so a rename of flusherDone can never
// masquerade as a pass.
func TestBootPanicVisibility_FlusherDoneWaitIsAfterEveryClose(t *testing.T) {
	fset, f, path := bootPanicVisibilityMainGo(t)

	var receiveLines, closeLines []int
	ast.Inspect(f, func(n ast.Node) bool {
		switch {
		case isFlusherDoneReceive(n):
			receiveLines = append(receiveLines, fset.Position(n.Pos()).Line)
		case isFlusherDoneClose(n):
			closeLines = append(closeLines, fset.Position(n.Pos()).Line)
		}
		return true
	})

	// Anti-vacuity legs — never a silent pass.
	if len(receiveLines) == 0 {
		t.Fatalf("%s: found ZERO `<-flusherDone` receive expressions (closes found: %v) — "+
			"the phase-78 boot-panic-visibility guard is VACUOUS; if flusherDone was "+
			"renamed, re-point this arm at the new name, do NOT delete it", path, closeLines)
	}
	if len(closeLines) == 0 {
		t.Fatalf("%s: found ZERO `close(flusherDone)` calls (receives found: %v) — "+
			"the phase-78 boot-panic-visibility guard is VACUOUS; if flusherDone was "+
			"renamed, re-point this arm at the new name, do NOT delete it", path, receiveLines)
	}

	// Ordering leg.
	maxClose := closeLines[0]
	for _, l := range closeLines {
		if l > maxClose {
			maxClose = l
		}
	}
	for _, r := range receiveLines {
		if r <= maxClose {
			t.Errorf("%s:%d: `<-flusherDone` occurs at or before the last "+
				"`close(flusherDone)` (line %d; all closes: %v). A defer that waits on "+
				"flusherDone before anything can close it turns EVERY panic in the boot "+
				"window into a zero-byte silent hang (phase 78 / ADR-0300). Move the "+
				"waiting defer below the close(flusherDone) if/else.",
				path, r, maxClose, closeLines)
		}
	}
	t.Logf("%s: `<-flusherDone` receives at %v; `close(flusherDone)` calls at %v", path, receiveLines, closeLines)
}

// TestBootPanicVisibility_FlusherDoneWaitIsPrecededByCancel is phase-78 structural
// arm 3 (D-BPV-GUARD-COVERAGE): inside the deferred function literal that waits on
// flusherDone, a call to cancel() must appear BEFORE the receive.
//
// Why arm 2 is not enough. Relocating the waiting defer past the close sites makes
// it the LAST-registered defer in main(), hence the FIRST to run in LIFO — ahead of
// `defer cancel()`. Its wait is released only when statsFlusher.Start(ctx) returns,
// which requires ctx to be CANCELED. So on the naively-relocated tree a panic in
// the new post-anchor window still hangs (EXECUTED: exit 137 under a SIGKILL
// deadline, zero panic bytes) while arm 2 is GREEN, because the receive line is
// genuinely below every close line. The cancel() first in the body is what makes
// the wait unable to outlive an unwinding main(); context.CancelFunc is idempotent,
// so the normal signal path is byte-identical (measured: 12/12 arms, 152 datagrams
// / 5888 bytes on a live receiver, identical across trees).
//
// THIS ARM PINS ONE SHAPE, DELIBERATELY. A bounded `select { case <-flusherDone:
// case <-time.After(d): }` and a trailing sibling `defer cancel()` both also work
// behaviorally, and both go RED here. That is intended: accepting a second shape
// would make this arm reason about registration order across sibling statements,
// which is exactly the reasoning that failed and produced the moved hang. If a
// later row deliberately switches, it must EDIT this arm — a visible, reviewable
// act — rather than have it silently accept a second shape.
//
// Anti-vacuity: not finding the deferred wait at all is a t.Fatalf, never a pass.
func TestBootPanicVisibility_FlusherDoneWaitIsPrecededByCancel(t *testing.T) {
	fset, f, path := bootPanicVisibilityMainGo(t)

	deferredWaits := 0
	guarded := 0
	ast.Inspect(f, func(n ast.Node) bool {
		d, ok := n.(*ast.DeferStmt)
		if !ok {
			return true
		}
		lit, ok := d.Call.Fun.(*ast.FuncLit)
		if !ok || lit.Body == nil {
			return true
		}
		// The receive must be in THIS literal's own body — not in a nested
		// literal, which would be a different frame with different unwind rules.
		var recvPos token.Pos = token.NoPos
		for _, stmt := range lit.Body.List {
			ast.Inspect(stmt, func(m ast.Node) bool {
				if recvPos == token.NoPos && isFlusherDoneReceive(m) {
					recvPos = m.Pos()
				}
				return recvPos == token.NoPos
			})
			if recvPos != token.NoPos {
				break
			}
		}
		if recvPos == token.NoPos {
			return true
		}
		deferredWaits++

		// Two positions are tracked, not one, so the "no cancel() at all" and the
		// "cancel() present but AFTER the receive" regressions fire DISTINCT
		// messages. Both are red, but they are different mistakes and a shared
		// message would let one masquerade as the other in a break test.
		beforePos, afterPos := token.NoPos, token.NoPos
		ast.Inspect(lit.Body, func(m ast.Node) bool {
			c, ok := m.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := c.Fun.(*ast.Ident)
			if !ok || id.Name != "cancel" || len(c.Args) != 0 {
				return true
			}
			if c.Pos() < recvPos {
				if beforePos == token.NoPos {
					beforePos = c.Pos()
				}
			} else if afterPos == token.NoPos {
				afterPos = c.Pos()
			}
			return true
		})
		if beforePos == token.NoPos {
			where := "there is NO cancel() call in this literal at all"
			if afterPos != token.NoPos {
				where = fmt.Sprintf("the only cancel() call is at line %d, AFTER the receive, "+
					"where it is unreachable during the hang", fset.Position(afterPos).Line)
			}
			t.Errorf("%s:%d: the deferred `<-flusherDone` wait is NOT preceded by a "+
				"`cancel()` call inside the same function literal — %s. Relocating this "+
				"defer below the close(flusherDone) sites makes it LAST-registered hence "+
				"FIRST in LIFO — ahead of `defer cancel()` — so on a panic path nothing "+
				"ever fires ctx.Done(), statsFlusher.Start(ctx) never returns, flusherDone "+
				"is never closed, and the panic is swallowed into a zero-byte hang exactly "+
				"as before (phase 78 / ADR-0300). Put cancel() FIRST in this body.",
				path, fset.Position(recvPos).Line, where)
			return true
		}
		guarded++
		t.Logf("%s: deferred `<-flusherDone` at line %d is preceded by `cancel()` at line %d",
			path, fset.Position(recvPos).Line, fset.Position(beforePos).Line)
		return true
	})

	// Anti-vacuity leg — never a silent pass.
	if deferredWaits == 0 {
		t.Fatalf("%s: found ZERO deferred function literals containing a `<-flusherDone` "+
			"receive — the phase-78 coverage arm is VACUOUS. If the wait moved or "+
			"flusherDone was renamed, re-point this arm; do NOT delete it.", path)
	}
	if guarded != deferredWaits {
		t.Errorf("%s: %d of %d deferred flusherDone waits are cancel()-guarded", path, guarded, deferredWaits)
	}
}
