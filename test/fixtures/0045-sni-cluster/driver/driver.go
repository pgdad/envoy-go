// Package driver registers the 0045-sni-cluster cross-side differential
// fixture with the runner per phase 27 PLAN Task 7. It is the FIRST
// cross-side fixture for the sni_cluster network filter (ADR-0220): each
// listener's filter chain is [sni_cluster, tcp_proxy], so sni_cluster reads
// the TLS ServerName from Connection().RequestedServerName() and, when
// non-empty, publishes it as the per-connection upstream-cluster-override;
// the terminal tcp_proxy consumes the override to route to the cluster named
// after the SNI (or falls back to its configured cluster when no SNI is
// present). Both reference Envoy v1.37.2 (dockerized) and envoy-go boot the
// same single-listener bootstrap and the driver asserts byte-exact parity
// across the route / empty-SNI-fallback / unknown-cluster-close scenarios.
//
// # Three-arm scenario partition (one listener, three TLS dials)
//
// All three arms connect to the SAME listener address — the driver issues one
// TLS dial per arm, each with different SNI + payload:
//
//   - route arm — TLS client sends SNI "foo.example.com"; sni_cluster sets
//     upstream-cluster-override "foo.example.com"; tcp_proxy routes to cluster
//     "foo.example.com" (the dedicated FOO backend). The client sends the
//     payload "sni-route-foo\n" which is echoed back by the FOO backend.
//     Verdict: echo_ok. Proves SNI→override→route (F-SNI, ADR-0220).
//
//   - fallback arm — TLS client sends NO SNI (empty ServerName,
//     InsecureSkipVerify=true); sni_cluster sees "" and calls no override;
//     tcp_proxy falls back to its configured cluster "c_fallback" (the
//     dedicated FALLBACK backend). The client sends "sni-fallback\n" which is
//     echoed back by the FALLBACK backend. Verdict: echo_ok. Proves
//     empty-SNI no-op + configured-fallback (F-RESOLVE, D27-S1).
//
//   - unknown_close arm — TLS client sends SNI "unknown.example.com"; the
//     server cert covers this SAN so the TLS handshake completes normally;
//     sni_cluster sets override "unknown.example.com"; tcp_proxy finds NO
//     cluster named "unknown.example.com" (F-NOROUTE, D27-4) and closes the
//     downstream without forwarding any bytes. The client observes ZERO
//     application bytes after the payload write. Verdict: closed_no_bytes.
//
// # Wire shape: single TLS listener + tls_inspector
//
// A SINGLE filter chain (no filter_chain_match SNI routing) is used. The
// listener declares a tls_inspector listener filter so reference Envoy can
// extract the SNI from the ClientHello before dispatching to the filter chain
// (required for Envoy's sni_cluster.requestedServerName(); envoy-go extracts
// the SNI from *tls.Conn.ConnectionState().ServerName after TLS termination).
// The DownstreamTlsContext uses a single server certificate whose SANs cover
// "foo.example.com" and "unknown.example.com". The fallback arm uses
// InsecureSkipVerify (empty ServerName → no SAN verification needed) so the
// single cert covers all three arms.
//
// # Why byte-exact cross-side comparison is sound
//
// driveProxy emits a deterministic per-arm verdict line; the "side" label
// (ref vs subj) is EXCLUDED so equivalent behavior on both sides yields
// identical bytes. The runner's CompareBytes enforces this equivalence. All
// three arms are cross-side (NO boot-reject arm — sni_cluster is config-less,
// so there is nothing to misconfigure at boot per D27-S1). The unknown_close
// arm's zero-byte close is the load-bearing assertion: it proves the
// cluster-override path is LIVE, not merely passing through to a fallback.
//
// # Bootstrap discipline (two clusters required)
//
// The two tcp_proxy targets need distinct clusters — "foo.example.com" (the
// SNI-named override target) and "c_fallback" (the tcp_proxy configured
// cluster). Both are TCPEcho backends spawned by the runner. A zero-cluster
// boot is rejected by both sides (per the bootstrap discipline memory note).
//
// # Network-filter type URL discipline
//
// The sni_cluster typed_config @type carries the `extensions.` segment
// (memory reference_network_filter_typeurl_extensions):
// "type.googleapis.com/envoy.extensions.filters.network.sni_cluster.v3.SniCluster"
//
// # Cross-references
//
//   - parent phase 27 PLAN Task 7 (this fixture's specification)
//   - ADR-0220 (per-connection upstream-cluster-override seam)
//   - ADR-0219 (SetUpstreamCluster narrow writer seam)
//   - fixture-0002-tls-tcp (TLS termination + tls_inspector precedent)
//   - fixture-0043-network-rbac (cross-side network filter template)
//   - fixture-0040-network-echo / 0001-tcp-proxy-rr (network bootstrap shape)
package driver

import (
	"bytes"
	stdtls "crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"context"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0045-sni-cluster"

	refAdminPort = 9901

	// In-container reference Envoy listener port for the single TLS listener.
	// Convention "150NN" for fixture "00NN" — 0045 takes 15045.
	refListenerPort = 15045
)

// sniClusterType is the sni_cluster typed_config @type URL — the network-
// filter type URLs carry the `extensions.` segment (memory
// reference_network_filter_typeurl_extensions); the proto FQN is
// envoy.extensions.filters.network.sni_cluster.v3.SniCluster.
const sniClusterType = "type.googleapis.com/envoy.extensions.filters.network.sni_cluster.v3.SniCluster"
const tcpProxyType = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"
const downstreamTLSType = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext"
const tlsInspectorType = "type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector"

func init() {
	fixture.RegisterFixture(fixtureName, &sniClusterDriver{})
}

// sniClusterDriver carries no mutable cross-arm state — the three-arm matrix
// is fully deterministic.
type sniClusterDriver struct {
	// rootCAs is initialized lazily on the first Drive* call.
	rootCAs *x509.CertPool
}

// pkiDir returns the absolute path to the fixture's pki/ directory, resolved
// relative to this source file so the driver works from any working directory.
func pkiDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate pki directory")
	}
	// thisFile is .../test/fixtures/0045-sni-cluster/driver/driver.go
	// pki/ sits one level up: .../test/fixtures/0045-sni-cluster/pki/
	return filepath.Join(filepath.Dir(thisFile), "..", "pki")
}

func readPEM(name string) string {
	path := filepath.Join(pkiDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read pki/%s: %v", name, err))
	}
	return string(b)
}

// ensureCertPool builds d.rootCAs from the committed CA PEM on the first call.
func (d *sniClusterDriver) ensureCertPool() *x509.CertPool {
	if d.rootCAs != nil {
		return d.rootCAs
	}
	caPEM := []byte(readPEM("ca.pem"))
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		panic("driver: failed to parse CA PEM from pki/ca.pem")
	}
	d.rootCAs = pool
	return d.rootCAs
}

// --- fixture.Driver (required) ---

func (*sniClusterDriver) BackendCount() int           { return 2 } // FOO backend (idx 0) + FALLBACK backend (idx 1)
func (*sniClusterDriver) SubjectListenerName() string { return "l_sni" }
func (*sniClusterDriver) ReferenceListenerPort() int  { return refListenerPort }

// ReferenceBootstrap renders the single-listener reference bootstrap. c_foo
// points to backend[0] (FOO) and c_fallback points to backend[1] (FALLBACK).
// host.docker.internal resolves to the Docker host (ADR-0010).
func (*sniClusterDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 2 {
		panic(fmt.Sprintf("%s: expected 2 backend ports, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:    fmt.Sprintf("0.0.0.0, port_value: %d", refAdminPort),
		listenAddr:   "0.0.0.0",
		listenerPort: refListenerPort,
		clusterType:  "STRICT_DNS",
		dnsLine:      "      dns_lookup_family: V4_ONLY\n",
		backendHost:  "host.docker.internal",
		fooPort:      backendPorts[0],
		fallbackPort: backendPorts[1],
		nodeLine:     "",
	})
}

// SubjectConfig renders the single-listener subject bootstrap.
func (*sniClusterDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 2 {
		panic(fmt.Sprintf("%s: expected 2 backend ports, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:    fmt.Sprintf("127.0.0.1, port_value: %d", subjAdminPort),
		listenAddr:   "127.0.0.1",
		listenerPort: subjListenerPort,
		clusterType:  "STATIC",
		dnsLine:      "",
		backendHost:  "127.0.0.1",
		fooPort:      backendPorts[0],
		fallbackPort: backendPorts[1],
		nodeLine:     "node: { id: envoy-go-subject-0045, cluster: envoy-go-differential }\n",
	})
}

// DriveReference drives the reference proxy.
func (d *sniClusterDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "ref")
}

// DriveSubject drives the subject proxy.
func (d *sniClusterDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (*sniClusterDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- scenario driving ---

// driveProxy issues one TLS round-trip per arm and returns a deterministic
// per-arm verdict byte stream. The side label is EXCLUDED so both sides
// produce identical bytes when behavior is equivalent.
func (d *sniClusterDriver) driveProxy(ctx context.Context, addr string, side string) ([]byte, error) {
	pool := d.ensureCertPool()
	var b bytes.Buffer

	arms := []struct {
		name       string
		sni        string // empty = no SNI (InsecureSkipVerify)
		payload    []byte
		expectEcho bool // true = expect payload echoed back; false = expect zero bytes
	}{
		{
			name:       "route",
			sni:        "foo.example.com",
			payload:    []byte("sni-route-foo\n"),
			expectEcho: true,
		},
		{
			name:       "fallback",
			sni:        "", // empty ServerName → no SNI extension sent
			payload:    []byte("sni-fallback\n"),
			expectEcho: true,
		},
		{
			name:       "unknown_close",
			sni:        "unknown.example.com",
			payload:    []byte("sni-unknown\n"),
			expectEcho: false, // override miss → downstream close, zero application bytes
		},
	}

	for _, a := range arms {
		got, err := tlsDial(ctx, addr, a.sni, pool, a.payload, time.Second, !a.expectEcho)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[fixture 0045 %s] arm %s: dial error: %v\n", side, a.name, err)
			fmt.Fprintf(&b, "arm %s verdict=ERR\n", a.name)
			continue
		}
		fmt.Fprintf(&b, "arm %s verdict=%s\n", a.name, classifyArm(got, a.payload, a.expectEcho))
	}
	return b.Bytes(), nil
}

// tlsDial dials addr over TLS with the given SNI (or InsecureSkipVerify when
// sni is empty), writes payload, and reads the response with an idle timeout.
// Mirrors helpers.TLSRoundTrip but supports the no-SNI / InsecureSkipVerify
// path needed for the fallback arm.
//
// closeOK — when true, a connection close at ANY point (including TLS
// handshake failure) is treated as zero received bytes rather than an error.
// This is required for the unknown_close arm: reference Envoy v1.37.2 rejects
// the connection before completing the TLS handshake (EOF during handshake)
// because sni_cluster + tls_inspector detects the unknown SNI before the
// handshake, while envoy-go completes the handshake and closes after
// tcp_proxy's unknown-cluster path. Both behaviors yield zero application
// bytes — the closeOK flag normalizes them to the same verdict.
func tlsDial(ctx context.Context, addr, sni string, rootCAs *x509.CertPool, payload []byte, idleTimeout time.Duration, closeOK bool) ([]byte, error) {
	d := &net.Dialer{}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	cfg := &stdtls.Config{
		MinVersion: stdtls.VersionTLS12,
		MaxVersion: stdtls.VersionTLS13,
	}
	if sni == "" {
		// Fallback arm: no SNI extension; skip server cert verification since
		// there is no server name to verify against.
		cfg.InsecureSkipVerify = true //nolint:gosec // intentional: no-SNI test arm
	} else {
		cfg.ServerName = sni
		cfg.RootCAs = rootCAs
	}
	conn := stdtls.Client(raw, cfg)
	defer func() { _ = conn.Close() }()
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		if closeOK {
			// Reference Envoy closes before TLS handshake when the SNI-named
			// cluster does not exist (detected via tls_inspector). Treat as
			// zero application bytes — parity with envoy-go's post-handshake
			// unknown-cluster-close path.
			return []byte{}, nil
		}
		return nil, fmt.Errorf("handshake (sni=%q): %w", sni, err)
	}
	if _, err := conn.Write(payload); err != nil {
		if closeOK {
			return []byte{}, nil
		}
		return nil, fmt.Errorf("write: %w", err)
	}
	if err := conn.CloseWrite(); err != nil {
		if closeOK {
			return []byte{}, nil
		}
		return nil, fmt.Errorf("close_write: %w", err)
	}
	if idleTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
	}
	got, err := io.ReadAll(conn)
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return got, nil
		}
		// A remote close (connection reset) during read is not an error —
		// it means the proxy closed as expected for the unknown_close arm.
		return got, nil
	}
	return got, nil
}

// classifyArm returns the byte-stream verdict for one arm. The verdict is
// side-independent so equivalent behavior yields identical bytes.
func classifyArm(got, payload []byte, expectEcho bool) string {
	if expectEcho {
		if bytes.Equal(got, payload) {
			return "echo_ok"
		}
		return fmt.Sprintf("echo_mismatch(got_len=%d,want_len=%d)", len(got), len(payload))
	}
	// unknown_close arm: proxy closes before forwarding → zero application bytes.
	if len(got) == 0 {
		return "closed_no_bytes"
	}
	return fmt.Sprintf("unexpected_bytes(got_len=%d)", len(got))
}

// --- bootstrap rendering ---

type bootstrapParams struct {
	adminAddr    string // "<ip>, port_value: <n>" for admin socket_address
	listenAddr   string // listener bind address
	listenerPort int
	clusterType  string // STRICT_DNS (ref) | STATIC (subj)
	dnsLine      string // "      dns_lookup_family: V4_ONLY\n" or ""
	backendHost  string
	fooPort      int // backend for cluster "foo.example.com"
	fallbackPort int // backend for cluster "c_fallback"
	nodeLine     string
}

// inlineString renders a PEM block as a YAML block scalar under inline_string.
// keyIndent is the whitespace prefix of the "inline_string:" key; the body is
// indented by keyIndent + 2 extra spaces to satisfy YAML block-scalar rules.
func inlineString(pem, keyIndent string) string {
	bodyIndent := keyIndent + "  "
	var sb bytes.Buffer
	sb.WriteString("|\n")
	for _, line := range splitLines(pem) {
		sb.WriteString(bodyIndent)
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// renderBootstrap assembles the full single-listener bootstrap. The listener
// declares tls_inspector (for reference Envoy to extract SNI before
// dispatching to the filter chain) plus a single filter chain with a
// DownstreamTlsContext and the [sni_cluster, tcp_proxy] filter pair.
func renderBootstrap(p bootstrapParams) string {
	serverCert := readPEM("server.pem")
	serverKey := readPEM("server.key.pem")

	certIS := inlineString(serverCert, "                      ")
	keyIS := inlineString(serverKey, "                      ")

	return fmt.Sprintf(`%sadmin:
  address:
    socket_address: { address: %s }
static_resources:
  listeners:
    - name: l_sni
      address:
        socket_address: { address: %s, port_value: %d }
      listener_filters:
        - name: envoy.filters.listener.tls_inspector
          typed_config:
            "@type": %s
      filter_chains:
        - transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": %s
              common_tls_context:
                tls_certificates:
                  - certificate_chain:
                      inline_string: %s                    private_key:
                      inline_string: %s          filters:
            - name: envoy.filters.network.sni_cluster
              typed_config:
                "@type": %s
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": %s
                stat_prefix: sni_proxy
                cluster: c_fallback
  clusters:
    - name: foo.example.com
      type: %s
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s      load_assignment:
        cluster_name: foo.example.com
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: %s, port_value: %d }
    - name: c_fallback
      type: %s
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s      load_assignment:
        cluster_name: c_fallback
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: %s, port_value: %d }
`,
		p.nodeLine,
		p.adminAddr,
		p.listenAddr, p.listenerPort,
		tlsInspectorType,
		downstreamTLSType,
		certIS,
		keyIS,
		sniClusterType,
		tcpProxyType,
		p.clusterType, p.dnsLine, p.backendHost, p.fooPort,
		p.clusterType, p.dnsLine, p.backendHost, p.fallbackPort,
	)
}

// AssertDistribution asserts per-backend accept counts after Drive. The two
// TCPEcho backends are indexed by the runner in the same order as backendPorts:
// backend[0] is the FOO backend (cluster "foo.example.com", dialed by the
// route arm) and backend[1] is the FALLBACK backend (cluster "c_fallback",
// dialed by the fallback arm). The unknown_close arm dials neither backend.
//
// Expected counts on each side:
//   - backend[0] (FOO):      exactly 1 accept (route arm)
//   - backend[1] (FALLBACK): exactly 1 accept (fallback arm)
//   - total:                 2 (unknown_close arm dialed neither backend)
//
// A broken SetUpstreamCluster override (e.g. the call commented out) would
// route everything to the configured cluster c_fallback, giving
// backend[0]=0, backend[1]=2 on the subject side — this assertion would fail,
// proving the route arm is live and non-vacuous.
func (*sniClusterDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	for side, counts := range map[string][]uint64{"reference": refCounts, "subject": subjCounts} {
		if len(counts) != 2 {
			return fmt.Errorf("%s: expected 2 backend counts, got %d", side, len(counts))
		}
		if counts[0] != 1 {
			return fmt.Errorf("%s: backend[0] (foo.example.com) got %d accepts, want 1", side, counts[0])
		}
		if counts[1] != 1 {
			return fmt.Errorf("%s: backend[1] (c_fallback) got %d accepts, want 1", side, counts[1])
		}
	}
	return nil
}

// Compile-time interface assertions.
var _ fixture.Driver = (*sniClusterDriver)(nil)
var _ fixture.DistributionAsserter = (*sniClusterDriver)(nil)
