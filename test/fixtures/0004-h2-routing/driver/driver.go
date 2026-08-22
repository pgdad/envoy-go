// Package driver registers the 0004-h2-routing fixture with the differential
// runner. Mirrors fixture-0003's driver shape with HTTPS h2 (vs HTTP/1.1
// plaintext) and per-side [3,3,3] distribution assertion per ADR-0024 + the
// 05.2 PLAN "Settled SPEC §10 deferred decisions" #3 (per-Cluster RR scope
// retained). Closes ADR-0035 H/2 leg per ADR-0057 — see ../README.md.
package driver

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

// Phase-88 CONTINUATION-arm constants, MIRRORED from the fixture backend
// (../backends/main.go). See that file for the MEASURED encoded-size table
// that proves 32000 B splits and 1024 B / 16000 B do not.
const (
	contPadLen     = 32000
	contProbeValue = "probe-value"
	contMarker     = "emitted"
)

// contPad mirrors the backend's pad generator byte-for-byte. Both sides must
// agree, because the driver asserts the LENGTH the backend reports back.
func contPad(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[i%len(alphabet)]
	}
	return string(b)
}

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

// drive issues 42 sequential H2 requests against addr (the proxy listener)
// and returns the concatenated 9 /health response bodies, followed by the 2
// phase-87 leading-`//` arm bodies ("edge-ok" x 2), followed by the 8 phase-89
// normalized arm markers ("p89-a1:ok" … "p89-a8:ok"), followed by the 4
// phase-90 authority-normalization lines ("p90-P:…" … "p90-B:…"). The first 27 requests and
// the transcript prefix they produce are unchanged from phase 05.2. Per ADR-0028 +
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

	// Phase-87 leading-`//` origin-form arms (2 requests; total 29/side).
	//
	// A leading `//` in an HTTP/2 `:path` is an ordinary origin-form path, but
	// under the full RFC-3986 URI grammar it reads as a network-path reference
	// whose first segment is an authority. A codec that parses `:path` with
	// url.Parse therefore peels `//edge` into the host component and routes on
	// the REMAINDER — a silent mis-route. Both arms below target the
	// `prefix: "//edge"` direct_response route (200 "edge-ok").
	//
	// BOTH assertions on BOTH arms are load-bearing; neither arm alone catches
	// both failure modes. Do not delete either as "redundant":
	//
	//   - `//edge`: the STATUS assertion is load-bearing. Under the defect the
	//     path degrades to "" — a route MISS (an empty path carries no
	//     `prefix: "/"` either), so the reply is 404 with an EMPTY body, not the
	//     catch-all's "not found\n". A body-only assertion would compare "" to
	//     "" and read GREEN on a regression.
	//   - `//edge/health`: the BODY assertion is load-bearing. Under the defect
	//     the path degrades to "/health", which MATCHES the first route and
	//     replies 200 "OK\n" — the silent mis-route. A status-only assertion
	//     would see 200 and read GREEN on a regression.
	for _, arm := range []string{"//edge", "//edge/health"} {
		status, _, body, err := helpers.H2RoundTrip(ctx, addr, tlsConf, "GET", arm, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", arm, err)
		}
		if status != 200 {
			return nil, fmt.Errorf("%s: status=%d, want 200", arm, status)
		}
		if string(body) != "edge-ok" {
			return nil, fmt.Errorf("%s: body=%q, want %q", arm, string(body), "edge-ok")
		}
		out.Write(body)
	}

	// Phase-88 CONTINUATION arms (2 requests; total 31/side).
	//
	// contPad(contPadLen) HPACK-encodes to 23792 B — past the 16384 B RFC 9113
	// §6.5.2 default SETTINGS_MAX_FRAME_SIZE — so the header block MUST travel
	// as HEADERS + CONTINUATION. Both arms route through `prefix: "/api"` to
	// c_h2_backend, so each exercises BOTH codec legs (downstream server read /
	// upstream client write on the request arm; upstream client read /
	// downstream server write on the response arm).
	//
	// ⚠️ ASSERTION CONSTRAINTS — do not "simplify" these:
	//
	//   1. Both arms assert the LENGTH of the large field, never the small
	//      probe/marker header's PRESENCE. The CONTINUATION-discard defect is
	//      PARTIAL: fields encoded BEFORE the split point survive, so a
	//      presence-only assertion reads GREEN on a broken tip.
	//   2. Neither arm asserts WHICH headers survive — that is x/net encoder
	//      field ordering, not a contract.
	//   3. 32000 B is MEASURED to split at this fixture's entropy. Changing the
	//      pad alphabet or size requires RE-MEASURING the encoded block size;
	//      the byte count does not carry.
	//
	// Both arms are appended AFTER the 9 /api/v1/<n> requests so the per-side
	// [3,3,3] RR distribution over those 9 is untouched, and neither body is
	// concatenated into the differential byte stream (the transcript prefix
	// stays byte-identical).
	{
		reqHdrs := []hpack.HeaderField{
			{Name: "x-cont-probe", Value: contProbeValue},
			{Name: "x-cont-pad", Value: contPad(contPadLen)},
		}
		status, _, body, err := helpers.H2RoundTrip(ctx, addr, tlsConf, "GET", "/api/v1/reflect", reqHdrs, nil)
		if err != nil {
			return nil, fmt.Errorf("cont-request-arm: %w", err)
		}
		if status != 200 {
			return nil, fmt.Errorf("cont-request-arm: status=%d, want 200", status)
		}
		want := fmt.Sprintf("reflect:probe=%s,padlen=%d", contProbeValue, contPadLen)
		if string(body) != want {
			return nil, fmt.Errorf("cont-request-arm: backend saw %q, want %q (CONTINUATION-carried request headers lost)", string(body), want)
		}
	}
	{
		status, respHdrs, _, err := helpers.H2RoundTrip(ctx, addr, tlsConf, "GET", "/api/v1/emit", nil, nil)
		if err != nil {
			return nil, fmt.Errorf("cont-response-arm: %w", err)
		}
		if status != 200 {
			return nil, fmt.Errorf("cont-response-arm: status=%d, want 200", status)
		}
		var marker string
		padLen := 0
		for _, hf := range respHdrs {
			switch strings.ToLower(hf.Name) {
			case "x-cont-marker":
				marker = hf.Value
			case "x-cont-pad":
				padLen = len(hf.Value)
			}
		}
		if marker != contMarker || padLen != contPadLen {
			return nil, fmt.Errorf("cont-response-arm: marker=%q padlen=%d, want %q/%d (CONTINUATION-carried response headers lost)", marker, padLen, contMarker, contPadLen)
		}
	}
	// Phase-89 decode-side filter-mutation arms (8 requests; total 39/side).
	//
	// Each arm targets an exact-path route that carries its own
	// header_mutation `typed_per_filter_config` (A5 excepted — see hmArms),
	// so the decode chain mutates the request header map and the proxy must
	// reconcile that delta onto the ordered upstream H/2 carrier.
	//
	// ⚠️ THE REFLECTED BODY NEVER ENTERS THE DIFFERENTIAL TRANSCRIPT. It
	// carries `x-request-id` (a random UUID) plus reference-only
	// `x-forwarded-proto` / `x-envoy-expected-rq-timeout-ms`, so the two sides
	// can never compare equal byte-for-byte. Each arm parses the block and
	// asserts named headers IN-BAND; only a normalized `p89-<arm>:ok` marker
	// is appended to the cross-side byte stream.
	//
	// ⚠️ NO ARM ASSERTS HEADER ORDER. helpers.H2RoundTrip sets client headers
	// with req.Header.Add onto a Go map, so the relative order of DIFFERENT
	// client-sent names is randomized per request by map iteration; and the
	// backend is net/http + http2.ConfigureServer, whose H/2 server folds the
	// HEADERS block into an http.Header MAP before any handler runs, so wire
	// order is destroyed before it can be observed. Recovering it would need a
	// raw-framer backend (a new BackendKind for one assertion). Wire ORDER is
	// pinned at the UNIT layer instead (internal/filter/hcm reconcile tests);
	// this fixture pins presence / absence / value / count / status.
	//
	// All 8 arms are appended AFTER the pre-existing 31 round-trips, so the
	// transcript prefix and the [3,3,3] RR distribution over the 9 counted
	// /api/v1/<n> requests are both untouched.
	for _, arm := range hmArms() {
		status, _, body, err := helpers.H2RoundTrip(ctx, addr, tlsConf, arm.method, arm.path, arm.send, arm.body)
		if err != nil {
			return nil, fmt.Errorf("p89-%s: %w", arm.name, err)
		}
		if status != 200 {
			return nil, fmt.Errorf("p89-%s: status=%d, want 200 (body=%q)", arm.name, status, string(body))
		}
		got := parseReflectedHeaders(body)
		if err := arm.check(got); err != nil {
			return nil, fmt.Errorf("p89-%s: %w (reflected names=%v)", arm.name, err, sortedNames(got))
		}
		fmt.Fprintf(&out, "p89-%s:ok", arm.name)
	}

	// Phase-90 H/2 `host` vs `:authority` normalization arms (3 requests;
	// total 42/side). Each arm is a HAND-BUILT HPACK field list driven by a raw
	// http2.Framer over its OWN fresh TLS(ALPN h2) connection — the 0119
	// instrument shape (../../0119-grpc-unary-trailers/driver/driver.go).
	//
	// ⚠️ helpers.H2RoundTrip CANNOT express any of these arms, on three
	// independent mechanisms:
	//
	//   1. No req.Host surface — the signature carries no host parameter and
	//      the URL is hard-built as "https://"+addr+path (helpers/h2.go).
	//   2. A client-supplied `host` entry is SILENTLY DROPPED —
	//      x/net/http2/transport.go `continue`s on asciiEqualFold(k, "host"),
	//      so passing {Name:"host"} through the helper is a VACUOUS arm.
	//   3. `:authority` cannot be set, emptied or injected — the transport
	//      derives it from req.Host/req.URL.Host, emits it unconditionally,
	//      and validateHeaders rejects a literal ":authority" header name.
	//
	// ⚠️ ONE FRESH CONNECTION PER ARM, EACH WITH ITS OWN HPACK DECODER. The
	// HPACK dynamic table is CONNECTION-scoped; a shared or per-request decoder
	// decodes only the first request on a pooled connection and then yields
	// "invalid indexed representation index NN" with a truncated field list —
	// which reads exactly like "headers were lost"
	// (reference_hpack_decoder_must_be_per_connection).
	//
	// ⚠️ THIS BLOCK IS DELIBERATELY NOT FAIL-FAST. The arms above assert
	// in-band with `return nil, fmt.Errorf(...)`, so the first failing arm
	// aborts the whole Drive and every later arm is unreachable
	// (reference_failfast_driver_masks_later_red_arms). These arms follow 0119's
	// discipline instead: every failure is recorded IN the transcript, all
	// arms ALWAYS run, and the runner's cross-side byte compare IS the
	// assertion. Exactly ONE `p90-<arm>:auth=… host=…` line is emitted per arm,
	// always, plus an ERR line when the arm did not complete cleanly.
	//
	// ⚠️ Every arm sends a FIXED LITERAL authority, never the dial address:
	// the reference dials a mapped container port and the subject a local one,
	// so an address-derived authority would break cross-side byte equality by
	// construction.
	//
	// ⚠️ NO YAML EDIT. Routes 2-8 are EXACT matches for a1-a4/a6-a8, so a
	// `/api/v1/reflect-headers/p90*` path falls through to
	// `- match: { prefix: "/api" }` -> c_h2_backend -> the backend's
	// `/api/v1/reflect-headers/` SUBTREE handler, exactly as a5 already does by
	// design. And the per-backend counter increment lives ONLY inside the
	// `/api/v1/<n>` loop above (deliberately not spelled here as a literal, so
	// the audit grep for it keeps reading exactly ONE hit), so
	// AssertDistribution's [3,3,3] over those 9 requests is untouched by these
	// three.
	for _, arm := range p90Arms() {
		got, status, failure := p90DriveArm(ctx, addr, tlsConf, arm)
		switch {
		case failure != "":
			fmt.Fprintf(&out, "p90-%s:ERR %s\n", arm.name, failure)
		case status != 200:
			fmt.Fprintf(&out, "p90-%s:ERR status=%d, want 200\n", arm.name, status)
		}
		fmt.Fprintf(&out, "p90-%s:auth=%q host=%v\n", arm.name, p90ObservedValue(got), p90HostPresent(got))
	}
	return []byte(out.String()), nil
}

// hmArm is one phase-89 decode-mutation arm: a request to send and an
// assertion over the header block the BACKEND reports having received.
type hmArm struct {
	name   string              // marker written to the cross-side transcript
	path   string              // exact route path (route config keys off it)
	method string              // "GET" except the body-carrying arm
	body   []byte              // request body (nil except the body-carrying arm)
	send   []hpack.HeaderField // client-sent seed headers
	check  func(got map[string][]string) error
}

// hmReflectBase is the backend's reflect-headers subtree. Every arm path is
// this prefix plus the arm's id.
const hmReflectBase = "/api/v1/reflect-headers/"

// hmArms returns the phase-89 arm table. See ../README.md for the honest
// scope statement on each arm.
func hmArms() []hmArm {
	return []hmArm{
		// A1 — net-new append (APPEND_IF_EXISTS_OR_ADD, no client seed). The
		// COUNT is asserted, not just presence: a reconcile that appended the
		// added value once per pass would read GREEN on a presence-only check.
		{
			name: "a1", path: hmReflectBase + "a1", method: "GET",
			check: func(got map[string][]string) error {
				return hmWantValues(got, "x-p89-added", "a1")
			},
		},
		// A2 — per-route `remove` of a header the CLIENT sent. Under the
		// pre-phase-89 defect the removal was ignored and the seed reached the
		// upstream, so this arm is the removal half of the two-container bug.
		{
			name: "a2", path: hmReflectBase + "a2", method: "GET",
			send:  []hpack.HeaderField{{Name: "x-p89-removed", Value: "seed"}},
			check: func(got map[string][]string) error { return hmWantAbsent(got, "x-p89-removed") },
		},
		// A3 — OVERWRITE_IF_EXISTS_OR_ADD over a client-sent value. Asserting
		// EXACTLY ONE value is what discriminates overwrite from append: a
		// reconcile that appended instead of replacing would deliver
		// ["old","new"] and a Get-style check would still see "new".
		{
			name: "a3", path: hmReflectBase + "a3", method: "GET",
			send: []hpack.HeaderField{{Name: "x-p89-changed", Value: "old"}},
			check: func(got map[string][]string) error {
				return hmWantValues(got, "x-p89-changed", "new")
			},
		},
		// A4 — the cross-side pin for the APPEND rule. Reference Envoy's
		// APPEND_IF_EXISTS_OR_ADD leaves the existing field where it is and
		// appends only the new value; envoy-go's reconcile classifies this as
		// `extra` for the same reason. BOTH values must survive, in wire order
		// [c0 v1] — duplicates of one name are always adjacent (the client
		// sets them through a single http.Header slot), so this ordering is a
		// pin and not a flake.
		{
			name: "a4", path: hmReflectBase + "a4", method: "GET",
			send: []hpack.HeaderField{{Name: "x-p89-dup", Value: "c0"}},
			check: func(got map[string][]string) error {
				return hmWantValues(got, "x-p89-dup", "c0", "v1")
			},
		},
		// A5 — NO per-route config. `/api/v1/reflect-headers/a5` has no exact
		// route, so it falls through to the `prefix: "/api"` route: the
		// no-mutation path. The arm's real content is the status-200 +
		// round-trip-completes assertion, i.e. that the reconcile is a no-op
		// when the delta is empty.
		//
		// ⚠️ THE PSEUDO-HEADER CHECK BELOW IS A FREE VACUOUS INVARIANT, NOT
		// COVERAGE. A `:`-prefixed name in the regular-header region is an RFC
		// 9113 §8.3 protocol error that the backend's codec rejects before any
		// handler runs, so this loop can never fire on a reachable input. It is
		// kept because it costs nothing and documents the intent; it must NOT
		// be counted as pinning h2ReconcileSkipKey's pseudo-header clause
		// (that clause is pinned at the unit layer).
		{
			name: "a5", path: hmReflectBase + "a5", method: "GET",
			check: func(got map[string][]string) error {
				for n := range got {
					if strings.HasPrefix(n, ":") {
						return fmt.Errorf("regular-header block carries pseudo-header %q", n)
					}
				}
				return nil
			},
		},
		// A6 — the config key is written in canonical-MIME form
		// (`X-P89-Case`), which both proxies canonicalize on ingest.
		//
		// ⚠️ HONEST SCOPE: this arm does NOT prove the wire name was
		// lowercased. The backend is net/http, which canonicalizes every
		// received name into `X-P89-Case` regardless of the bytes on the wire,
		// so the reflected block reads the same either way. The REAL
		// discriminator is that an uppercase name on an H/2 request is a
		// protocol error — a proxy emitting `X-P89-Case` verbatim would fail
		// the round-trip outright and this arm would go red on the STATUS
		// check, not on the header check.
		{
			name: "a6", path: hmReflectBase + "a6", method: "GET",
			check: func(got map[string][]string) error {
				return hmWantValues(got, "x-p89-case", "c1")
			},
		},
		// A7a — a benign custom append alongside `te: trailers`, the ONE
		// conditionally-legal RFC 9113 §8.2.2 value that must SURVIVE the
		// reconcile's IsIllegalH2RequestHeader guard.
		//
		// ⚠️ `te` is appended AT MOST ONCE. Reference Envoy comma-coalesces
		// repeated `te` into a single field, and the conformant x/net backend
		// rejects anything but exactly "trailers".
		//
		// ⚠️ THE A7b COUNTERPART IS DELIBERATELY ABSENT — see ../README.md
		// "Phase-89 INTENTIONAL DEPARTURE".
		{
			name: "a7", path: hmReflectBase + "a7", method: "GET",
			check: func(got map[string][]string) error {
				if err := hmWantValues(got, "x-p89-benign", "kept"); err != nil {
					return err
				}
				return hmWantValues(got, "te", "trailers")
			},
		},
		// A8 — the A1 mutation replayed on a POST with a NON-EMPTY body, so
		// the request takes WriteH2's `hasBody` branch and RunDecodeData runs
		// before the reconcile.
		//
		// ⚠️ HONEST SCOPE: this arm exercises that branch but does NOT
		// discriminate the reconcile's PLACEMENT relative to RunDecodeData.
		// Every mutation here originates in DecodeHeaders, so moving the
		// reconcile above the hasBody block would leave this arm green. That
		// placement is pinned at the unit layer by
		// TestWriteH2_Reconcile_DecodeDataMutationIsApplied.
		{
			name: "a8", path: hmReflectBase + "a8", method: "POST",
			body: []byte("p89-a8-body"),
			check: func(got map[string][]string) error {
				return hmWantValues(got, "x-p89-added", "a1")
			},
		},
	}
}

// parseReflectedHeaders parses the backend's sorted `name: value` block into a
// lowercase-keyed multimap. Values are taken verbatim after the FIRST ": "
// separator, so a value containing ": " survives intact.
func parseReflectedHeaders(body []byte) map[string][]string {
	out := make(map[string][]string)
	for _, line := range strings.Split(strings.TrimRight(string(body), "\n"), "\n") {
		if line == "" {
			continue
		}
		i := strings.Index(line, ": ")
		if i < 0 {
			continue
		}
		out[strings.ToLower(line[:i])] = append(out[strings.ToLower(line[:i])], line[i+2:])
	}
	return out
}

// sortedNames returns the reflected block's names in sorted order, for error
// messages. Never used in an assertion.
func sortedNames(got map[string][]string) []string {
	names := make([]string, 0, len(got))
	for n := range got {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// hmWantValues asserts that name is present with EXACTLY the given value list,
// in order. The length check is the load-bearing half: a duplicate-emitting
// reconcile is invisible to a presence-only or first-value-only check.
func hmWantValues(got map[string][]string, name string, want ...string) error {
	have := got[name]
	if len(have) != len(want) {
		return fmt.Errorf("header %q: got %d value(s) %v, want %d %v", name, len(have), have, len(want), want)
	}
	for i := range want {
		if have[i] != want[i] {
			return fmt.Errorf("header %q: got %v, want %v", name, have, want)
		}
	}
	return nil
}

// hmWantAbsent asserts that name does not appear in the reflected block at all.
func hmWantAbsent(got map[string][]string, name string) error {
	if v, ok := got[name]; ok {
		return fmt.Errorf("header %q: present with %v, want ABSENT (per-route `remove` did not reach the upstream H/2 carrier)", name, v)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Phase-90 authority-normalization arms (raw framer, ONE FRESH CONNECTION EACH)
// ---------------------------------------------------------------------------

const (
	// p90ArmDeadline bounds each phase-90 arm's dial + write + read loop.
	p90ArmDeadline = 10 * time.Second

	// p90StreamID is the single client-initiated stream each arm uses. One
	// FRESH connection per arm, so it is always stream 1.
	p90StreamID = 1

	// p90HpackTableSize is the RESPONSE decoder's dynamic-table size. The
	// decoder itself is created per connection, never shared across arms.
	p90HpackTableSize = 4096

	// The wire literals. FIXED, never derived from the dial address — see the
	// block comment in drive().
	p90Authority = "p90.example"
	p90HostValue = "p90host.example"

	// p90ObservedAuthority is the header the fixture backend emits AFTER its
	// sorted reflected block, carrying r.Host — i.e. the authority the proxy
	// actually forwarded (../backends/main.go).
	p90ObservedAuthority = "x-observed-authority"

	// p90AbsentAuthority is what the transcript records when the reflected
	// block carries NO p90ObservedAuthority key at all.
	//
	// ⚠️ ABSENT and PRESENT-AND-EMPTY are DIFFERENT observations and this
	// fixture must not conflate them. parseReflectedHeaders splits on the FIRST
	// ": ", so an EMPTY r.Host emits `x-observed-authority: ` — separator
	// intact — and parses as the key PRESENT with value "" (rendered `""`).
	// A key missing entirely means the request never reached the reflect
	// handler at all, which is a different finding and renders as this literal.
	// No arm sends a header whose value could collide with it.
	p90AbsentAuthority = "<absent>"
)

// p90Arm is one phase-90 arm: a hand-built HPACK field list in WIRE ORDER, and
// the path it targets. `fields` is hpack-encoded verbatim, duplicates and all,
// which is precisely what helpers.H2RoundTrip cannot express.
type p90Arm struct {
	name   string
	fields []hpack.HeaderField
}

// p90Fields builds a request field list: the three mandatory pseudo-headers
// followed by rest, in order. Any `:authority` belongs in rest's FIRST slot so
// the pseudo-header block stays contiguous, as RFC 9113 §8.3 requires.
func p90Fields(path string, rest ...hpack.HeaderField) []hpack.HeaderField {
	fields := []hpack.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":path", Value: hmReflectBase + path},
	}
	return append(fields, rest...)
}

// p90Arms returns the phase-90 arm roster.
//
// ⚠️ ARM C (`:authority` PRESENT-AND-EMPTY) IS DELIBERATELY NOT HERE. It is a
// deferred follow-on: `:authority` absent and `:authority` present-and-empty
// both satisfy `rp.authority == ""` in x/net's H/2 server and both take the
// fallback to the Host header, landing byte-identical in r.Host AND r.Header —
// so NO backend edit can recover the distinction, and recovering it needs a
// raw-framer BACKEND (a new BackendKind) that this row does not buy.
//
// ⚠️ ARM E (FIRST-OCCURRENCE-WINS: two regular `host` fields, no `:authority`)
// WAS HERE AND WAS REMOVED. It is NOT DIFFERENTIABLE IN PRINCIPLE — do not
// re-add it:
//
//  1. THE REFERENCE REFUSES THE SHAPE. MEASURED against the pinned image
//     (envoyproxy/envoy:contrib-v1.37.2, docs/envoy-go/ENVOY_TARGET.md): a
//     SECOND regular `host` field on the H/2 downstream leg is rejected at the
//     codec layer — "Invalid HTTP header field was received: frame type: 1,
//     stream: 1, name: [host]", response details `http2.invalid.header.field`,
//     reset reason "connection termination". The client is sent NO GOAWAY and
//     NO RST_STREAM; the connection is closed and the arm reads a bare EOF, so
//     the reference transcript line is `p90-E:ERR read-frame: …` while a
//     correct subject serves a 200. The rejection is by ARITY, not by value —
//     two IDENTICAL `host` values are refused the same way — and holds with
//     `:authority` also present. Stats move `http2.rx_messaging_error`,
//     `downstream_cx_protocol_error` and `downstream_rq_rx_reset`.
//     Testing first-occurrence-wins REQUIRES two `host` fields, so no
//     subject-side change can ever make the two sides' bytes agree.
//  2. THE AXIS IS ALREADY PINNED, AT THE UNIT LAYER.
//     TestAuthorityNormalization/E_dup_host_first_wins
//     (internal/filter/hcm/h2/authority_norm_test.go) is the SOLE first-wins
//     discriminator in the tree: inverting both latches to last-wins reddens
//     that arm and only that arm. Do not delete it.
//  3. MATCHING THE REFERENCE HERE IS OUT OF CHARTER. Row 90 is
//     promote/suppress. A duplicate-`host` REJECT is reference-side admission
//     control — the same family as the deferred arm-C validity reject and the
//     same class as D-90-DUP (the duplicate-`:authority` reject this row
//     deliberately declined to add). Implementing it would be an unpriced
//     behavior change.
func p90Arms() []p90Arm {
	return []p90Arm{
		// P90-P — POSITIVE CONTROL: `:authority` only, no `host` field.
		// Asserts x-observed-authority == p90.example AND `host` ABSENT.
		// ⚠️ This arm must PASS on BOTH sides even PRE-fix; if it ever
		// diverges the roster is vacuously red and the rest proves nothing.
		{"P", p90Fields("p90p",
			hpack.HeaderField{Name: ":authority", Value: p90Authority},
		)},
		// P90-A — BOTH authorities. `:authority` wins and the regular `host`
		// field must not survive onto the upstream carrier. Pre-fix the
		// SUBJECT emits `host: p90host.example` and the reference does not.
		{"A", p90Fields("p90a",
			hpack.HeaderField{Name: ":authority", Value: p90Authority},
			hpack.HeaderField{Name: "host", Value: p90HostValue},
		)},
		// P90-B — `host` ONLY, no `:authority`. The `host` field must be
		// PROMOTED to the authority and then suppressed as a regular field.
		// Pre-fix the subject reads an EMPTY authority — which is why
		// p90AbsentAuthority exists as a distinct rendering (see above).
		{"B", p90Fields("p90b",
			hpack.HeaderField{Name: "host", Value: p90HostValue},
		)},
	}
}

// p90EncodeHeaderBlock hpack-encodes fields into a single header block
// fragment, in the given order. DUPLICATE NAMES ARE PRESERVED (arm E sends two
// `host` fields) and pseudo-headers are written verbatim; neither is
// expressible through helpers.H2RoundTrip.
func p90EncodeHeaderBlock(fields []hpack.HeaderField) ([]byte, error) {
	var buf bytes.Buffer
	enc := hpack.NewEncoder(&buf)
	for _, hf := range fields {
		if err := enc.WriteField(hf); err != nil {
			return nil, fmt.Errorf("hpack encode %s: %w", hf.Name, err)
		}
	}
	return buf.Bytes(), nil
}

// p90ScrubAddr replaces the side-specific dial address with a fixed
// placeholder so failure lines stay cross-side comparable (the reference dials
// a mapped container port, the subject a local one).
func p90ScrubAddr(msg, addr string) string {
	return strings.ReplaceAll(msg, addr, "<addr>")
}

// p90DriveArm opens a FRESH TLS(ALPN h2) connection, writes the arm's
// hand-built header block with a raw framer, and reads the response stream to
// END_STREAM. It returns the parsed reflected block, the observed :status, and
// a NON-EMPTY failure string when the arm did not complete — it NEVER returns
// an error, so no arm can make a later arm unreachable.
func p90DriveArm(ctx context.Context, addr string, tlsConf *tls.Config, a p90Arm) (map[string][]string, int, string) {
	fail := func(stage string, err error) (map[string][]string, int, string) {
		return nil, 0, stage + ": " + p90ScrubAddr(err.Error(), addr)
	}

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: p90ArmDeadline},
		Config:    tlsConf,
	}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fail("dial", err)
	}
	conn, ok := raw.(*tls.Conn)
	if !ok {
		_ = raw.Close()
		return nil, 0, "dial: not a TLS connection"
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(p90ArmDeadline))

	// ⚠️ 0004 is TLS+ALPN-h2, NOT h2c: the negotiated protocol is asserted
	// BEFORE the preface is written, exactly as 0119's driveArm does.
	if proto := conn.ConnectionState().NegotiatedProtocol; proto != "h2" {
		return nil, 0, fmt.Sprintf("alpn: negotiated %q, want h2", proto)
	}

	// h2 client preface + SETTINGS.
	if _, err := io.WriteString(conn, http2.ClientPreface); err != nil {
		return fail("preface", err)
	}
	fr := http2.NewFramer(conn, conn)
	// ⚠️ PER-CONNECTION decoder. See drive()'s block comment.
	fr.ReadMetaHeaders = hpack.NewDecoder(p90HpackTableSize, nil)
	if err := fr.WriteSettings(); err != nil {
		return fail("settings", err)
	}

	block, err := p90EncodeHeaderBlock(a.fields)
	if err != nil {
		return fail("hpack-encode", err)
	}
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      p90StreamID,
		BlockFragment: block,
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		return fail("write-headers", err)
	}

	// Read loop: accumulate the response body until END_STREAM (or a terminal
	// condition, returned as a failure string).
	status := 0
	var body []byte
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			return nil, status, "read-frame: " + p90ScrubAddr(err.Error(), addr)
		}
		switch f := f.(type) {
		case *http2.SettingsFrame:
			if !f.IsAck() {
				if err := fr.WriteSettingsAck(); err != nil {
					return fail("settings-ack", err)
				}
			}
		case *http2.PingFrame:
			if !f.IsAck() {
				_ = fr.WritePing(true, f.Data)
			}
		case *http2.MetaHeadersFrame:
			if f.StreamID != p90StreamID {
				continue
			}
			for _, hf := range f.Fields {
				if hf.Name == ":status" {
					if n, convErr := strconv.Atoi(hf.Value); convErr == nil {
						status = n
					}
				}
			}
			if f.StreamEnded() {
				return parseReflectedHeaders(body), status, ""
			}
		case *http2.DataFrame:
			if f.StreamID != p90StreamID {
				continue
			}
			body = append(body, f.Data()...)
			if f.StreamEnded() {
				return parseReflectedHeaders(body), status, ""
			}
		case *http2.RSTStreamFrame:
			if f.StreamID != p90StreamID {
				continue
			}
			return nil, status, fmt.Sprintf("rst-stream code=%v", f.ErrCode)
		case *http2.GoAwayFrame:
			return nil, status, fmt.Sprintf("goaway code=%v", f.ErrCode)
		default:
			// WINDOW_UPDATE / PRIORITY / unknown: not recorded.
		}
	}
}

// p90ObservedValue renders the reflected p90ObservedAuthority for the
// transcript, keeping ABSENT distinguishable from PRESENT-AND-EMPTY (see
// p90AbsentAuthority).
func p90ObservedValue(got map[string][]string) string {
	v, ok := got[p90ObservedAuthority]
	switch {
	case !ok:
		return p90AbsentAuthority
	case len(v) == 1:
		return v[0]
	default:
		// The backend emits the header exactly once, so any other count is
		// itself a finding: record it verbatim rather than taking v[0].
		return strings.Join(v, "|")
	}
}

// p90HostPresent reports whether a regular `host` field survived onto the
// upstream H/2 carrier. PRESENCE, not value, is the contract here: the fix
// suppresses the field outright, so there is no surviving value to compare.
func p90HostPresent(got map[string][]string) bool {
	_, ok := got["host"]
	return ok
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

// DriveReference runs 42 H2 round-trips against the reference proxy listener
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

// DriveSubject runs 42 H2 round-trips against the subject proxy listener and
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
