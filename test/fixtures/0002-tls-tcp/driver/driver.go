// Package driver implements the 0002-tls-tcp fixture's driver: 9 TLS
// round-trips per SNI (18 total) against a 2-SNI, 6-endpoint cluster pair,
// with per-cluster distribution asserted to be exactly [3,3,3]/[3,3,3].
//
// The fixture exercises downstream TLS termination and SNI-indexed filter-chain
// dispatch. Upstream clusters use plaintext TCP (the harness's echo backends are
// plain TCP listeners; upstream TLS origination would require TLS-capable
// backends, which the generic harness does not provide).
package driver

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const fixtureName = "0002-tls-tcp"
const refContainerListenerPort = 15002
const requestsPerSNI = 9

func init() {
	fixture.RegisterFixture(fixtureName, &tlsDriver{})
}

// pkiDir returns the absolute path to the fixture's pki/ directory, resolved
// relative to this source file so the driver works from any working directory.
func pkiDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate pki directory")
	}
	// thisFile is .../test/fixtures/0002-tls-tcp/driver/driver.go
	// pki/ sits one level up: .../test/fixtures/0002-tls-tcp/pki/
	return filepath.Join(filepath.Dir(thisFile), "..", "pki")
}

// readPEM reads a PEM file from the pki directory.
func readPEM(name string) string {
	path := filepath.Join(pkiDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read pki/%s: %v", name, err))
	}
	return string(b)
}

// tlsDriver implements fixture.Driver and fixture.DistributionAsserter for
// fixture 0002-tls-tcp.
type tlsDriver struct {
	// rootCAs is initialized lazily on the first Drive* call and reused for
	// all subsequent calls on the same driver instance.
	//
	// Concurrency note: the runner calls DriveReference first, then
	// DriveSubject sequentially (never concurrently). The rootCAs field is
	// therefore written once (on the DriveReference call) and read once (on
	// the DriveSubject call). No mutex is needed under this assumption.
	rootCAs *x509.CertPool
}

func (*tlsDriver) BackendCount() int           { return 6 }
func (*tlsDriver) SubjectListenerName() string { return "l_tls" }
func (*tlsDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

// inlineString renders a YAML block scalar value for inline_string.
// keyIndent is the whitespace that prefixes the "inline_string:" key.
// The body (PEM lines) is indented by keyIndent + 2 extra spaces, satisfying
// the YAML block-scalar indentation requirement that the body be more indented
// than its key.
func inlineString(pem, keyIndent string) string {
	bodyIndent := keyIndent + "  "
	var sb strings.Builder
	sb.WriteString("|\n")
	for _, line := range strings.Split(strings.TrimRight(pem, "\n"), "\n") {
		sb.WriteString(bodyIndent)
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// downstreamChain builds a single downstream TLS filter chain YAML block.
//
// YAML depth under filter_chains (6-space key):
//
//	filter_chains item bullet:   8 spaces
//	chain mapping keys:         10 spaces  (filter_chain_match, transport_socket, filters)
//	transport_socket children:  12 spaces
//	typed_config children:      14 spaces
//	common_tls_context:         14 spaces
//	tls_certificates:           16 spaces
//	list-item bullet:           16 spaces  → content at 18
//	certificate_chain:          18 spaces
//	inline_string:              22 spaces  → body at 24 spaces ✓
func downstreamChain(sni, serverCert, serverKey, clusterName string) string {
	statPrefix := "ingress_tls_" + func() string {
		if strings.HasPrefix(sni, "alpha") {
			return "alpha"
		}
		return "beta"
	}()

	// inline_string: key is at 22 spaces under the list item.
	certIS := inlineString(serverCert, "                      ")
	keyIS := inlineString(serverKey, "                      ")

	return fmt.Sprintf(`        - filter_chain_match:
            server_names: ["%s"]
          transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                tls_certificates:
                  - certificate_chain:
                      inline_string: %s                    private_key:
                      inline_string: %s          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: %s
                cluster: %s
`, sni, certIS, keyIS, statPrefix, clusterName)
}

// upstreamCluster builds a single cluster YAML block (plaintext TCP to the
// harness echo backends).
//
// Upstream TLS origination is intentionally omitted: the harness creates plain
// TCP echo backends; adding UpstreamTlsContext would cause TLS handshake
// failures against those backends. The upstream-*.pem PKI materials are
// committed for future fixtures that supply TLS-capable backends.
func upstreamCluster(
	name, clusterType string,
	dnsLookupFamily bool,
	endpointHost string,
	ports [3]int,
) string {
	dnsLine := ""
	if dnsLookupFamily {
		dnsLine = "      dns_lookup_family: V4_ONLY\n"
	}
	return fmt.Sprintf(`    - name: %s
      type: %s
      connect_timeout: 1s
%s      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: %s
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }
`,
		name, clusterType, dnsLine,
		name,
		endpointHost, ports[0],
		endpointHost, ports[1],
		endpointHost, ports[2],
	)
}

// ReferenceBootstrap returns the upstream Envoy bootstrap YAML with:
//   - tls_inspector listener filter (required for SNI-based filter_chain_match)
//   - 2 SNI-indexed downstream TLS filter chains on port 15002
//   - 2 STRICT_DNS clusters (c_alpha, c_beta) reaching backends via
//     host.docker.internal (ADR-0010), plaintext upstream
func (*tlsDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 6 {
		panic(fmt.Sprintf("0002: expected 6 backend ports, got %d", len(backendPorts)))
	}
	return buildBootstrap(
		"", // no node stanza for reference Envoy
		"0.0.0.0", 9901,
		"0.0.0.0", 15002,
		"STRICT_DNS", true, "host.docker.internal",
		backendPorts,
	)
}

// SubjectConfig returns the envoy-go bootstrap YAML with:
//   - 2 SNI-indexed downstream TLS filter chains
//   - 2 STATIC clusters (c_alpha, c_beta) reaching backends via 127.0.0.1
//     (ADR-0027), plaintext upstream
//
// No tls_inspector listener filter is needed: envoy-go's SNI dispatch uses
// Go's crypto/tls GetConfigForClient callback which reads the SNI directly from
// the ClientHello.
func (*tlsDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 6 {
		panic(fmt.Sprintf("0002: expected 6 backend ports, got %d", len(backendPorts)))
	}
	return buildBootstrap(
		"node: { id: envoy-go-subject-0002, cluster: envoy-go-differential }\n",
		"127.0.0.1", subjAdminPort,
		"127.0.0.1", subjListenerPort,
		"STATIC", false, "127.0.0.1",
		backendPorts,
	)
}

// buildBootstrap assembles the full YAML bootstrap string. The reference config
// includes a tls_inspector listener_filter (Envoy needs it to read SNI before
// filter_chain_match); the subject config omits it (envoy-go reads SNI via
// crypto/tls directly). The isReference flag selects which set of sections
// to include — here the node stanza being empty distinguishes the two.
func buildBootstrap(
	nodeStanza string,
	adminAddr string, adminPort int,
	listenerAddr string, listenerPort int,
	clusterType string, dnsLookupFamily bool, endpointHost string,
	backendPorts []int,
) string {
	serverAlphaCert := readPEM("server-alpha.pem")
	serverAlphaKey := readPEM("server-alpha.key.pem")
	serverBetaCert := readPEM("server-beta.pem")
	serverBetaKey := readPEM("server-beta.key.pem")

	chainAlpha := downstreamChain("alpha.envoy-go.test", serverAlphaCert, serverAlphaKey, "c_alpha")
	chainBeta := downstreamChain("beta.envoy-go.test", serverBetaCert, serverBetaKey, "c_beta")

	clusterAlpha := upstreamCluster(
		"c_alpha", clusterType, dnsLookupFamily, endpointHost,
		[3]int{backendPorts[0], backendPorts[1], backendPorts[2]},
	)
	clusterBeta := upstreamCluster(
		"c_beta", clusterType, dnsLookupFamily, endpointHost,
		[3]int{backendPorts[3], backendPorts[4], backendPorts[5]},
	)

	// Reference Envoy requires tls_inspector to detect SNI before filter_chain
	// selection. The subject (envoy-go) reads SNI natively via crypto/tls and
	// does not parse listener_filters at all — omitting it is safe and avoids a
	// "unsupported listener_filter" parse error.
	listenerFiltersSection := ""
	if nodeStanza == "" {
		// Reference path: Envoy needs tls_inspector.
		listenerFiltersSection = `      listener_filters:
        - name: envoy.filters.listener.tls_inspector
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector
`
	}

	return fmt.Sprintf(`%vadmin:
  address:
    socket_address: { address: %s, port_value: %d }
static_resources:
  listeners:
    - name: l_tls
      address:
        socket_address: { address: %s, port_value: %d }
%v      filter_chains:
%v%v  clusters:
%v%v`,
		nodeStanza,
		adminAddr, adminPort,
		listenerAddr, listenerPort,
		listenerFiltersSection,
		chainAlpha, chainBeta,
		clusterAlpha, clusterBeta,
	)
}

// ensureCertPool builds d.rootCAs from the committed CA PEM on the first call.
// Subsequent calls are no-ops.
//
// Concurrency note: the runner always calls DriveReference before DriveSubject
// (sequentially, never concurrently). ensureCertPool is therefore called at
// most twice in sequence, so the pool is written once and read once — no mutex
// needed. If the calling order ever changes this function must be guarded with
// sync.Once.
func (d *tlsDriver) ensureCertPool() *x509.CertPool {
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

// tlsPayloads returns the deterministic per-request payloads for one SNI
// (prefix is "alpha" or "beta"). A static sequence gives the same debugging
// value as a per-call random identifier without diverging between
// DriveReference and DriveSubject calls.
func tlsPayloads(prefix string) [][]byte {
	p := make([][]byte, requestsPerSNI)
	for i := 0; i < requestsPerSNI; i++ {
		p[i] = []byte(fmt.Sprintf("rr-%s-%d\n", prefix, i))
	}
	return p
}

// driveSide runs 9 TLS round-trips for alpha SNI followed by 9 for beta SNI
// against addr, returning the concatenated response bytes.
func (d *tlsDriver) driveSide(ctx context.Context, addr string) ([]byte, error) {
	pool := d.ensureCertPool()
	var sb strings.Builder

	for _, sni := range []string{"alpha.envoy-go.test", "beta.envoy-go.test"} {
		prefix := "alpha"
		if sni == "beta.envoy-go.test" {
			prefix = "beta"
		}
		for i, p := range tlsPayloads(prefix) {
			b, err := helpers.TLSRoundTrip(ctx, addr, sni, pool, p, time.Second)
			if err != nil {
				return nil, fmt.Errorf("drive[sni=%s,i=%d]: %w", sni, i, err)
			}
			sb.Write(b)
		}
	}
	return []byte(sb.String()), nil
}

// DriveReference runs 18 TLS round-trips (9 per SNI) against the reference
// proxy's listener address and returns the concatenated response bytes.
func (d *tlsDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	b, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("ref drive: %w", err)
	}
	return b, nil
}

// DriveSubject runs 18 TLS round-trips (9 per SNI) against the subject proxy's
// listener address and returns the concatenated response bytes.
func (d *tlsDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	b, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("subj drive: %w", err)
	}
	return b, nil
}

// AssertDistribution: per-proxy per-cluster counts must be exactly [3,3,3] for
// c_alpha (indices 0–2) and [3,3,3] for c_beta (indices 3–5). Both the
// reference and subject proxies must satisfy this invariant.
func (*tlsDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	want := [3]uint64{3, 3, 3}
	for side, counts := range map[string][]uint64{"reference": refCounts, "subject": subjCounts} {
		if len(counts) != 6 {
			return fmt.Errorf("%s: expected 6 backend counts, got %d", side, len(counts))
		}
		var gotAlpha, gotBeta [3]uint64
		copy(gotAlpha[:], counts[0:3])
		copy(gotBeta[:], counts[3:6])
		if gotAlpha != want {
			return fmt.Errorf("%s: c_alpha distribution %v != %v", side, gotAlpha, want)
		}
		if gotBeta != want {
			return fmt.Errorf("%s: c_beta distribution %v != %v", side, gotBeta, want)
		}
	}
	return nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// the raw response bytes for the differential diff.
func (*tlsDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref probe: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj probe: %w", err)
	}
	return refBytes, subjBytes, nil
}

// Compile-time check the driver implements both required and optional interfaces.
var (
	_ fixture.Driver               = (*tlsDriver)(nil)
	_ fixture.DistributionAsserter = (*tlsDriver)(nil)
)
