// Package driver registers the 0004-h2-routing fixture with the differential
// runner. Mirrors fixture-0003's driver shape with HTTPS h2 (vs HTTP/1.1
// plaintext) and per-side [3,3,3] distribution assertion per ADR-0024 + the
// 05.2 PLAN "Settled SPEC §10 deferred decisions" #3 (per-Cluster RR scope
// retained). Closes ADR-0035 H/2 leg per ADR-0057 — see ../README.md.
package driver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const fixtureName = "0004-h2-routing"
const refContainerListenerPort = 15004

func init() {
	fixture.RegisterFixture(fixtureName, &h2Driver{})
}

// h2Driver implements fixture.Driver, fixture.DistributionAsserter,
// fixture.HTTPExpectations (intentionally NOT — fixture asserts in-band per
// SPEC §4.3 recommendation), and fixture.BackendKindAware.
//
// The runner's in-process accept counter is NOT incremented for HTTPSH2
// subprocess backends, so AssertDistribution is fed by per-side counts that
// the driver derived from response bodies during DriveReference / DriveSubject.
// Those counts are stashed on the driver instance (refCounts / subjCounts) and
// surfaced via lastCountsMu-guarded fields the runner consumes through
// AssertDistribution(refCounts, subjCounts) — except the runner passes IN the
// runner-collected counts (zeros for HTTPSH2). The driver therefore IGNORES
// the incoming refCounts/subjCounts and asserts the body-derived counts it
// recorded itself.
type h2Driver struct {
	mu          sync.Mutex
	refBodyCnt  [3]uint64
	subjBodyCnt [3]uint64

	rootCAs *x509.CertPool
}

func (*h2Driver) BackendCount() int                { return 3 }
func (*h2Driver) BackendKind() fixture.BackendKind { return fixture.HTTPSH2 }
func (*h2Driver) SubjectListenerName() string      { return "l_h2" }
func (*h2Driver) ReferenceListenerPort() int       { return refContainerListenerPort }

// fixtureDir resolves the absolute path to the fixture directory (the parent
// of this driver/ package), regardless of the test's working directory.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0004-h2-routing/driver/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

// readPEM reads a PEM file from the fixture's pki/ directory.
func readPEM(name string) string {
	path := filepath.Join(fixtureDir(), "pki", name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read pki/%s: %v", name, err))
	}
	return string(b)
}

// readYAML reads a YAML template from the fixture root directory and strips
// the leading comment header (lines starting with `#` or blank, up to the
// first non-comment line). The committed templates (envoy.yaml /
// envoy-go.yaml) document the placeholder names in their header comments
// (e.g. "{{LISTENER_CERT}}" appears in prose), which would collide with the
// driver's first-occurrence string substitution. Stripping the header before
// substitution makes the YAML usage of each placeholder the first match.
func readYAML(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	lines := strings.Split(string(b), "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		return strings.Join(lines[i:], "\n")
	}
	panic(fmt.Sprintf("driver: %s contains only comments/blanks", name))
}

// substitutePEM finds a line of the form `<indent>{{NAME}}` in yaml and
// replaces that single line with the PEM body, indenting every PEM line by
// `<indent>` so the result is a valid YAML block-scalar continuation. The
// caller-supplied placeholder indent in the template determines body indent
// (the templates use different indents for listener PEMs vs the CA PEM).
//
// Only the FIRST occurrence is replaced. The header-comment block has been
// stripped by readYAML so the placeholder mention in prose does not collide
// with the YAML usage.
func substitutePEM(yaml, placeholder, pem string) string {
	target := "{{" + placeholder + "}}"
	idx := strings.Index(yaml, target)
	if idx < 0 {
		panic(fmt.Sprintf("driver: placeholder %s not found", target))
	}
	// Walk back to the start of the line to capture the indent.
	lineStart := strings.LastIndexByte(yaml[:idx], '\n') + 1 // 0 if no prior newline
	indent := yaml[lineStart:idx]
	// Validate: the prefix must be all whitespace. (If anything else slipped in
	// front of the placeholder, the substitution would corrupt the YAML; fail
	// loudly rather than silently emit broken YAML.)
	for _, ch := range indent {
		if ch != ' ' && ch != '\t' {
			panic(fmt.Sprintf("driver: %s placeholder line has non-whitespace prefix %q", placeholder, indent))
		}
	}
	// End of placeholder. The trailing newline (if any) is preserved as the
	// separator between the substituted PEM body and the next YAML line.
	lineEnd := idx + len(target)
	// Build body: first PEM line takes the existing indent (in place of the
	// placeholder); subsequent lines get `\n<indent>` prepended.
	var body strings.Builder
	for i, line := range strings.Split(strings.TrimRight(pem, "\n"), "\n") {
		if i == 0 {
			body.WriteString(line)
			continue
		}
		body.WriteByte('\n')
		body.WriteString(indent)
		body.WriteString(line)
	}
	return yaml[:idx] + body.String() + yaml[lineEnd:]
}

// renderBootstrap fills the YAML template's PEM placeholders and the numbered
// `port_value: 0` lines.
//
// Replacement strategy:
//   - `{{LISTENER_CERT}}` / `{{LISTENER_KEY}}` / `{{CA_CERT}}` placeholders
//     each occupy a single indented line within an `inline_string: |` block
//     scalar; substitutePEM replaces that line with the PEM body indented at
//     the placeholder's existing indent. The listener PEM placeholders sit
//     at 24 spaces (downstream filter-chain depth); the CA placeholder sits
//     at 18 spaces (cluster trusted_ca depth) — substitutePEM derives the
//     correct indent per placeholder from the source line.
//   - `port_value: 0` lines appear in deterministic order. For the subject
//     template the order is admin, listener, backend-0, backend-1,
//     backend-2. For the reference template only the three backend ports
//     appear (admin and listener are fixed at 9901 / 15004).
func renderBootstrap(yaml string, ports []string) string {
	yaml = substitutePEM(yaml, "LISTENER_CERT", readPEM("listener.pem"))
	yaml = substitutePEM(yaml, "LISTENER_KEY", readPEM("listener.key.pem"))
	yaml = substitutePEM(yaml, "CA_CERT", readPEM("ca.pem"))

	for _, p := range ports {
		yaml = strings.Replace(yaml, "port_value: 0", "port_value: "+p, 1)
	}
	return yaml
}

// ReferenceBootstrap returns the upstream Envoy bootstrap YAML (admin :9901,
// listener :15004 fixed; only backend ports vary).
func (*h2Driver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("0004: expected 3 backend ports, got %d", len(backendPorts)))
	}
	yaml := readYAML("envoy.yaml")
	ports := []string{
		fmt.Sprintf("%d", backendPorts[0]),
		fmt.Sprintf("%d", backendPorts[1]),
		fmt.Sprintf("%d", backendPorts[2]),
	}
	return renderBootstrap(yaml, ports)
}

// SubjectConfig returns the envoy-go bootstrap YAML.
func (*h2Driver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("0004: expected 3 backend ports, got %d", len(backendPorts)))
	}
	yaml := readYAML("envoy-go.yaml")
	ports := []string{
		fmt.Sprintf("%d", subjAdminPort),
		fmt.Sprintf("%d", subjListenerPort),
		fmt.Sprintf("%d", backendPorts[0]),
		fmt.Sprintf("%d", backendPorts[1]),
		fmt.Sprintf("%d", backendPorts[2]),
	}
	return renderBootstrap(yaml, ports)
}

// ensureCertPool builds d.rootCAs from the committed CA PEM on the first call.
// Subsequent calls return the cached pool. Guarded by d.mu.
func (d *h2Driver) ensureCertPool() *x509.CertPool {
	d.mu.Lock()
	defer d.mu.Unlock()
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

// loadTLSConfig builds a TLS config that trusts the fixture-local CA and
// advertises only NextProtos=["h2"] so the listener (offering ["h2","http/1.1"])
// negotiates h2 via ALPN.
//
// ServerName: the listener and backend leaves carry SANs `localhost`,
// `host.docker.internal`, and IP `127.0.0.1`. The driver dials the proxy
// listener at either 127.0.0.1:<port> (subject) or 127.0.0.1:<mappedPort>
// (reference, mapped from the container by testcontainers). Both reach the
// listener cert which has `127.0.0.1` in its IP SAN; ServerName="localhost"
// matches the DNS SAN (Go's tls.Dial uses ServerName for SNI + verification;
// using IP-as-ServerName would require the IP-SAN path).
func (d *h2Driver) loadTLSConfig() *tls.Config {
	pool := d.ensureCertPool()
	return &tls.Config{
		RootCAs:    pool,
		NextProtos: []string{"h2"},
		ServerName: "localhost",
		MinVersion: tls.VersionTLS12,
	}
}

// drive issues 27 sequential H2 requests against addr (the proxy listener)
// and returns the concatenated 9 /health response bodies. Per ADR-0028 +
// ADR-0056: each request opens a fresh *http2.ClientConn (the helper's
// fresh-Transport-per-call discipline). The 9 /api response bodies are NOT
// concatenated into the diff stream (per-side RR offset may differ; routing
// correctness is covered by AssertDistribution). The 9 /missing bodies are
// NOT concatenated (404 body is intentionally relaxed — Envoy emits HTML/JSON
// while envoy-go emits "not found\n").
//
// Side-effect: per-call /api response-body parsing populates counts[idx]
// (where idx is parsed from the "backend-<idx>:" prefix) so the driver can
// surface per-side distribution to AssertDistribution (subprocess backends
// don't increment the runner's accept counter).
func (d *h2Driver) drive(ctx context.Context, addr string, counts *[3]uint64) ([]byte, error) {
	tlsConf := d.loadTLSConfig()

	var out strings.Builder
	for n := 0; n < 9; n++ {
		status, _, body, err := helpers.H2RoundTrip(ctx, addr, tlsConf, "GET", "/health", nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/health[%d]: %w", n, err)
		}
		if status != 200 {
			return nil, fmt.Errorf("/health[%d]: status=%d, want 200", n, status)
		}
		out.Write(body)
	}
	for n := 0; n < 9; n++ {
		status, _, body, err := helpers.H2RoundTrip(ctx, addr, tlsConf, "GET", fmt.Sprintf("/api/v1/%d", n), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/api/v1/%d: %w", n, err)
		}
		if status != 200 {
			return nil, fmt.Errorf("/api/v1/%d: status=%d, want 200", n, status)
		}
		idx, err := parseBackendIdx(body)
		if err != nil {
			return nil, fmt.Errorf("/api/v1/%d: parse backend idx: %w (body=%q)", n, err, string(body))
		}
		if idx < 0 || idx >= 3 {
			return nil, fmt.Errorf("/api/v1/%d: backend idx %d out of range [0,3)", n, idx)
		}
		counts[idx]++
	}
	for n := 0; n < 9; n++ {
		status, _, _, err := helpers.H2RoundTrip(ctx, addr, tlsConf, "GET", fmt.Sprintf("/missing/%d", n), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/missing/%d: %w", n, err)
		}
		if status != 404 {
			return nil, fmt.Errorf("/missing/%d: status=%d, want 404", n, status)
		}
	}
	return []byte(out.String()), nil
}

// parseBackendIdx extracts the numeric idx from a body that starts with
// "backend-<idx>:..." (per fixture-0004 backend's /api/v1 handler).
func parseBackendIdx(body []byte) (int, error) {
	const prefix = "backend-"
	s := string(body)
	if !strings.HasPrefix(s, prefix) {
		return -1, fmt.Errorf("missing %q prefix", prefix)
	}
	rest := s[len(prefix):]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return -1, fmt.Errorf("missing ':' separator after backend idx")
	}
	var idx int
	if _, err := fmt.Sscanf(rest[:colon], "%d", &idx); err != nil {
		return -1, fmt.Errorf("parse %q: %w", rest[:colon], err)
	}
	return idx, nil
}

// DriveReference runs 27 H2 round-trips against the reference proxy listener
// and records per-backend body counts on d.refBodyCnt for AssertDistribution.
func (d *h2Driver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	var counts [3]uint64
	b, err := d.drive(ctx, addr, &counts)
	if err != nil {
		return nil, fmt.Errorf("ref drive: %w", err)
	}
	d.mu.Lock()
	d.refBodyCnt = counts
	d.mu.Unlock()
	return b, nil
}

// DriveSubject runs 27 H2 round-trips against the subject proxy listener and
// records per-backend body counts on d.subjBodyCnt for AssertDistribution.
func (d *h2Driver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	var counts [3]uint64
	b, err := d.drive(ctx, addr, &counts)
	if err != nil {
		return nil, fmt.Errorf("subj drive: %w", err)
	}
	d.mu.Lock()
	d.subjBodyCnt = counts
	d.mu.Unlock()
	return b, nil
}

// AssertDistribution asserts both ref and subj per-cluster RR distributions
// are exactly [3,3,3] over the 9 router-action /api requests.
//
// The runner-supplied refCounts / subjCounts are zero for HTTPSH2 backends
// (subprocess backends don't increment the in-process counter). The driver
// therefore consults the body-derived counts it recorded during DriveReference
// / DriveSubject. The incoming counters are accepted but only used for a
// length sanity check; their values are deliberately ignored.
func (d *h2Driver) AssertDistribution(refCounts, subjCounts []uint64) error {
	if len(refCounts) != 3 {
		return fmt.Errorf("ref backend count: got %d, want 3", len(refCounts))
	}
	if len(subjCounts) != 3 {
		return fmt.Errorf("subj backend count: got %d, want 3", len(subjCounts))
	}
	d.mu.Lock()
	ref := d.refBodyCnt
	subj := d.subjBodyCnt
	d.mu.Unlock()

	want := [3]uint64{3, 3, 3}
	if subj != want {
		return fmt.Errorf("subj distribution %v != %v (RR [3,3,3] expected)", subj, want)
	}
	if ref != want {
		return fmt.Errorf("ref distribution %v != %v (RR [3,3,3] expected)", ref, want)
	}
	return nil
}

// HTTPExpectations is intentionally NOT implemented: fixture 0004 asserts
// in-band via the Drive pass (status / body / per-backend distribution all
// witnessed during the H2 round-trips). Per SPEC §4.3 recommendation, no
// post-Drive HTTPRoundTrip pass is needed; the helper used in Drive is
// helpers.H2RoundTrip per Task 12. The runner's HTTPExpectations branch uses
// helpers.HTTPRoundTrip (HTTP/1.1) which would not work over an h2-only
// listener.

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// the raw response bytes (status line + headers + body) for the differential
// diff. Phase 01 contract.
func (*h2Driver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref admin: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj admin: %w", err)
	}
	return refBytes, subjBytes, nil
}

// Compile-time checks: driver implements all required and optional interfaces.
var (
	_ fixture.Driver               = (*h2Driver)(nil)
	_ fixture.DistributionAsserter = (*h2Driver)(nil)
	_ fixture.BackendKindAware     = (*h2Driver)(nil)
)
