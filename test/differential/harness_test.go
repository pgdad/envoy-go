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

// backendPortBand{Base,Span} bound the port range freeTCPPort draws from:
// 11000..14999. Chosen to sit BELOW the kernel ephemeral range
// (net.ipv4.ip_local_port_range, 32768+ by default) for the same reason
// freeTCPPortBlock's 20000..31007 band does — see its comment — and clear of
// every other claimed range: the static fixture ports (10000..10447,
// 15000..15011, 18001..18007) and the subject blocks (20000..31007).
const (
	backendPortBandBase = 11000
	backendPortBandSpan = 4000
)

// backendPortCursor strides freeTCPPort's candidates so a port released by the
// previous fixture is not immediately re-offered to the next one.
var backendPortCursor atomic.Uint32

// freeTCPPort returns a port observed bindable ON THE WILDCARD ADDRESS, drawn
// from a band outside the kernel's ephemeral range.
//
// Both properties are load-bearing, and this helper had NEITHER until the
// 0084-otlp-access-log failure of 2026-07-30 (CI run 30505421651):
//
//	listen: listen tcp 0.0.0.0:36243: bind: address already in use
//	runner_test.go:343: backend[0] not ready: waitTCPDial: ... within 5s
//
// (1) WILDCARD, not loopback. Every subprocess backend this feeds binds
// `0.0.0.0:<port>`, whose conflict set is strictly larger than
// `127.0.0.1:<port>`'s — a port held on any single interface address makes the
// wildcard bind fail while a loopback probe still reports it free. Measured
// directly: with a squatter on 192.168.1.76:46081, `127.0.0.1:46081` binds FREE
// and `0.0.0.0:46081` fails EADDRINUSE. The old probe address therefore did not
// answer the question its callers ask.
//
// (2) OUTSIDE the ephemeral range. 36243 is inside 32768..60999, which the
// kernel hands out concurrently to the suite's own traffic (Docker, the
// drivers' client connections). freeTCPPortBlock already moved the SUBJECT off
// that range for exactly this reason and recorded ~8% of it busy under
// full-suite load; the backend path never got the same treatment.
//
// A window remains between the probe's Close and the child's bind (the child
// is `go run`, so the gap spans a build). Banding shrinks that window's
// exposure to fixture-allocated ports only — the kernel no longer assigns from
// here — but does not close it. A start-retry loop, as the subject-start path
// has, would be the second line of defense if this ever recurs.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	for tries := 0; tries < 256; tries++ {
		slot := backendPortCursor.Add(1)
		port := backendPortBandBase + int(slot%backendPortBandSpan)
		ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port
	}
	// Band exhausted: fall back to the kernel allocator rather than fail the
	// fixture. Still wildcard-probed, so property (1) survives the fallback.
	t.Logf("freeTCPPort: no free port in %d..%d after 256 tries; falling back to the ephemeral range",
		backendPortBandBase, backendPortBandBase+backendPortBandSpan-1)
	ln, err := net.Listen("tcp", "0.0.0.0:0")
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

// TestFreeTCPPort_BandedAndWildcardBindable pins the two properties that the
// 0084-otlp-access-log CI failure of 2026-07-30 showed freeTCPPort lacked: the
// port must be OUTSIDE the kernel ephemeral range, and it must be bindable on
// the WILDCARD address (which is what every subprocess backend binds), not
// merely on loopback.
func TestFreeTCPPort_BandedAndWildcardBindable(t *testing.T) {
	// Static invariant: the backend band must not overlap the subject band.
	// A future edit to either pair of constants trips this without needing a
	// suite run to discover the collision.
	backendHi := backendPortBandBase + backendPortBandSpan - 1
	// freeTCPPortBlock's bases are 20000 + (slot*span % 11008). span divides
	// 11008 evenly (11008/16 = 688), so the residues are exactly the multiples
	// of span in [0, 11008-span] and the highest base is 20000+11008-span; the
	// block it hands out then extends span-1 further. Top = 20000+11008-1.
	const subjectModulus = 11008
	subjectLo := 20000
	subjectHi := 20000 + subjectModulus - 1
	if backendPortBandBase <= subjectHi && subjectLo <= backendHi {
		t.Errorf("backend band %d..%d overlaps subject band %d..%d",
			backendPortBandBase, backendHi, subjectLo, subjectHi)
	}

	seen := make(map[int]bool)
	for i := 0; i < 32; i++ {
		port := freeTCPPort(t)

		if port < backendPortBandBase || port > backendHi {
			// Not fatal on its own: the documented fallback path may legitimately
			// return an ephemeral port if the band is exhausted. Say so loudly
			// rather than silently accepting an ephemeral port as normal.
			t.Errorf("freeTCPPort returned %d, outside the band %d..%d "+
				"(ephemeral fallback taken? the band should not be exhausted on an idle host)",
				port, backendPortBandBase, backendHi)
		}

		// The property the old loopback probe did not establish.
		ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			t.Errorf("freeTCPPort returned %d but it is not wildcard-bindable: %v", port, err)
		} else {
			_ = ln.Close()
		}

		if seen[port] {
			t.Errorf("freeTCPPort returned %d twice in %d calls; the cursor should stride", port, i+1)
		}
		seen[port] = true
	}
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
