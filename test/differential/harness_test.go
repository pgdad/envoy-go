package differential

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

// TestReferenceProxy_UDPExposure verifies that a listener port designated UDP
// is exposed via Docker's /udp form and mapped into ReferenceProxy.udpAddrs,
// surfaced through ListenerUDPAddr as a host:port string (phase 61.3 — the
// harness's first non-TCP transport; see startReferenceProxy's udpPorts
// param). Docker publishes an exposed port regardless of whether the
// container process binds it, so a minimal admin-only bootstrap suffices to
// prove the exposure->mapping plumbing; the true end-to-end HTTP/3 proof is a
// separate task.
func TestReferenceProxy_UDPExposure(t *testing.T) {
	if testing.Short() {
		t.Skip("differential test; skipped under -short")
	}
	ensureDocker(t)

	pin := loadPinFromRepo(t)
	const cfg = `
admin: { address: { socket_address: { address: 0.0.0.0, port_value: 9901 } } }
`
	const udpPort = 8443
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ref, err := startReferenceProxy(ctx, pin, cfg, nil, nil, []int{udpPort})
	if err != nil {
		t.Fatalf("startReferenceProxy: %v", err)
	}
	defer func() { _ = ref.Stop(context.Background()) }()

	addr := ref.ListenerUDPAddr(udpPort)
	t.Logf("ListenerUDPAddr(%d) = %q", udpPort, addr)
	if addr == "" {
		t.Fatalf("ListenerUDPAddr(%d) empty; /udp exposure did not map", udpPort)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("ListenerUDPAddr(%d) = %q: not a valid host:port: %v", udpPort, addr, err)
	}
	if host == "" || port == "" {
		t.Errorf("ListenerUDPAddr(%d) = %q: empty host or port component", udpPort, addr)
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

// subjectPortBlockSpan is the number of consecutive ports probed by
// freeTCPPortBlock. Multi-listener fixtures derive listener ports as
// subjPort+1..+N without reserving them (0036 derives 12); 16 covers every
// current fixture with headroom.
const subjectPortBlockSpan = 16

// subjectPortBlockCursor strides freeTCPPortBlock's candidate bases so
// concurrent subtests never probe the same block within a wrap
// (11008/16 = 688 blocks per wrap).
var subjectPortBlockCursor atomic.Uint32

// freeTCPPortBlock returns a base port whose full derived block
// [base, base+subjectPortBlockSpan) was observed bindable (on the wildcard
// address, matching the subject's own 0.0.0.0 listener binds) at allocation
// time. Bases come from 20000..31007 — BELOW the kernel ephemeral range
// (net.ipv4.ip_local_port_range, 32768+ by default) that freeTCPPort draws
// from — so the subject's derived, never-reserved listener ports don't race
// the ephemeral source ports of concurrent suite traffic (the documented
// bind-collision flake at the subject-start retry loop; ~8% of the ephemeral
// range was observed busy under Docker Desktop + full-suite load). Ports are
// probed then closed, so a small race window remains — the subject-start
// retry loop is the second line of defense.
func freeTCPPortBlock(t *testing.T) int {
	t.Helper()
	for tries := 0; tries < 256; tries++ {
		slot := subjectPortBlockCursor.Add(1)
		base := 20000 + int(slot*subjectPortBlockSpan%11008)
		ok := true
		var lns []net.Listener
		for p := base; p < base+subjectPortBlockSpan; p++ {
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
			if err != nil {
				ok = false
				break
			}
			lns = append(lns, ln)
		}
		for _, ln := range lns {
			_ = ln.Close()
		}
		if ok {
			return base
		}
	}
	t.Logf("freeTCPPortBlock: no fully-free %d-port block after 256 tries; falling back to an ephemeral base", subjectPortBlockSpan)
	return freeTCPPort(t)
}

// TestHostGatewayIP verifies that HostGatewayIP returns a non-empty literal IP
// string parseable by net.ParseIP. Skipped if Docker is unavailable (mirrors
// the ensureDocker socket-probe pattern at :64-79).
func TestHostGatewayIP(t *testing.T) {
	// Skip — not fail — if Docker is unavailable; the controller re-runs on a
	// live-Docker host.
	paths := []string{
		"/var/run/docker.sock",
		os.Getenv("HOME") + "/.docker/desktop/docker.sock",
	}
	dockerAvailable := false
	for _, p := range paths {
		if conn, err := net.Dial("unix", p); err == nil {
			_ = conn.Close()
			dockerAvailable = true
			break
		}
	}
	if !dockerAvailable {
		t.Skip("docker unavailable: no reachable socket — skipping TestHostGatewayIP")
	}

	ip, err := HostGatewayIP(context.Background())
	if err != nil {
		t.Fatalf("HostGatewayIP: %v", err)
	}
	if ip == "" {
		t.Fatalf("HostGatewayIP returned empty string")
	}
	if net.ParseIP(ip) == nil {
		t.Errorf("HostGatewayIP returned %q which is not a valid IP address", ip)
	}
}

// TestAcceptHTTP503Counting verifies the in-process always-503 responder
// (phase 40.1 / fixture 0069) returns 503 Service Unavailable with a
// "backend-<idx>:<seg>" attribution body over a loopback listener.
func TestAcceptHTTP503Counting(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go acceptHTTP503Counting(ln, 7)

	url := fmt.Sprintf("http://%s/x/req-42", ln.Addr().String())
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d %q; want 503", resp.StatusCode, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got, want := string(b), "backend-7:req-42"; got != want {
		t.Errorf("body = %q; want %q", got, want)
	}
}
