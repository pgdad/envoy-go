package differential

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseEnvoyTarget_PullsTagAndDigest(t *testing.T) {
	src := `# envoy-go Reference Envoy Pin

**Tag:** ` + "`envoyproxy/envoy:v1.34.0`" + `
**SHA256:** ` + "`sha256:abc123def456`" + `
`
	pin, err := parseEnvoyTarget(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parseEnvoyTarget: %v", err)
	}
	if pin.Tag != "envoyproxy/envoy:v1.34.0" {
		t.Errorf("Tag: got %q", pin.Tag)
	}
	if pin.SHA256 != "sha256:abc123def456" {
		t.Errorf("SHA256: got %q", pin.SHA256)
	}
}

func TestParseEnvoyTarget_RejectsMissingTag(t *testing.T) {
	src := "no tag here\n**SHA256:** `sha256:abc`\n"
	if _, err := parseEnvoyTarget(strings.NewReader(src)); err == nil {
		t.Fatalf("parseEnvoyTarget accepted input without Tag")
	}
}

func TestReferenceProxy_Starts(t *testing.T) {
	if testing.Short() {
		t.Skip("differential test; skipped under -short")
	}
	ensureDocker(t)

	pin := loadPinFromRepo(t)
	const cfg = `
admin: { address: { socket_address: { address: 0.0.0.0, port_value: 9901 } } }
`
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ref, err := StartReferenceProxy(ctx, pin, cfg)
	if err != nil {
		t.Fatalf("StartReferenceProxy: %v", err)
	}
	defer func() { _ = ref.Stop(context.Background()) }()
	if ref.AdminAddr() == "" {
		t.Errorf("AdminAddr empty")
	}
}

func ensureDocker(t *testing.T) {
	t.Helper()
	// Try canonical path first; Docker Desktop on Linux bind-mounts it or
	// exposes it at $HOME/.docker/desktop/docker.sock.
	paths := []string{
		"/var/run/docker.sock",
		os.Getenv("HOME") + "/.docker/desktop/docker.sock",
	}
	for _, p := range paths {
		if conn, err := net.Dial("unix", p); err == nil {
			_ = conn.Close()
			return
		}
	}
	t.Fatalf("docker unavailable: no reachable socket at %v", paths)
}

func loadPinFromRepo(t *testing.T) *EnvoyPin {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	f, err := os.Open(filepath.Join(repoRoot, "docs", "envoy-go", "ENVOY_TARGET.md"))
	if err != nil {
		t.Fatalf("open pin: %v", err)
	}
	defer func() { _ = f.Close() }()
	pin, err := parseEnvoyTarget(f)
	if err != nil {
		t.Fatalf("parse pin: %v", err)
	}
	return pin
}

func TestSubjectProxy_StartsAndReports(t *testing.T) {
	if testing.Short() {
		t.Skip("subject subprocess test; skipped under -short")
	}
	port := freeTCPPort(t) // helper from cmd/envoy-go/main_test.go-style
	adminPort := freeTCPPort(t)
	cfg := fmt.Sprintf(`
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
                    socket_address: { address: 127.0.0.1, port_value: 65535 }
`, adminPort, port)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	subj, err := StartSubjectProxy(ctx, repoRoot(t), cfg, fmt.Sprintf("127.0.0.1:%d", adminPort))
	if err != nil {
		t.Fatalf("StartSubjectProxy: %v", err)
	}
	defer func() { _ = subj.Stop() }()

	if got, want := subj.ListenerAddr("l_tcp"), fmt.Sprintf("127.0.0.1:%d", port); got != want {
		t.Errorf("ListenerAddr: got %q, want %q", got, want)
	}
	if got, want := subj.AdminAddr(), fmt.Sprintf("127.0.0.1:%d", adminPort); got != want {
		t.Errorf("AdminAddr: got %q, want %q", got, want)
	}
}

// repoRoot returns the absolute path to the repository root. Used by both the
// subject-proxy starter (build.Dir) and the runner (loadPinFromRepo path
// resolution).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	abs, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if err != nil {
		t.Fatalf("repoRoot: %v", err)
	}
	return abs
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
