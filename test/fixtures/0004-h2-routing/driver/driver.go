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
	"errors"
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

	// Per-side `content-length` observations, one entry per p92Arms() arm and
	// IN THAT ORDER, recorded during DriveReference / DriveSubject and
	// asserted in AssertDistribution — which the runner calls AFTER the
	// cross-side byte compare. See p92AssertCLFields for why the pins must sit
	// BELOW that gate, and p93CLObs for what each entry carries: the field
	// arity, the THREE-STATE declared value, and the DELIVERED body length.
	refP92CL  []p93CLObs
	subjP92CL []p93CLObs

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

// drive issues 47 sequential H2 requests against addr (the proxy listener)
// and returns the concatenated 9 /health response bodies, followed by the 2
// phase-87 leading-`//` arm bodies ("edge-ok" x 2), followed by the 8 phase-89
// normalized arm markers ("p89-a1:ok" … "p89-a8:ok"), followed by the 4
// phase-90 authority-normalization lines ("p90-P:…" … "p90-B:…"), followed by the
// 5 phase-92 illegal-response-header lines ("p92-keepalive:…" …
// "p92-te-empty:…"). The first 27 requests and
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
//
// Side-effect: *p92CL is appended one entry per phase-92 arm, in p92Arms()
// order and TAGGED WITH THAT ARM'S NAME, carrying the arm's downstream
// `content-length` field arity, its THREE-STATE declared value and the
// DELIVERED body length (see p93CLObs). Those observables are deliberately NOT
// in the cross-side byte stream (see the p92 block below, p92AssertCLFields,
// p93AssertDeclaredDelivered and p93AssertBodyLen); the caller stashes them per
// side and AssertDistribution asserts each side against its OWN measured pins,
// AFTER the runner's cross-side byte compare.
func (d *h2Driver) drive(ctx context.Context, addr string, counts *[3]uint64, p92CL *[]p93CLObs) ([]byte, error) {
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

	// Phase-92 illegal RESPONSE header arms (5 requests; total 47/side).
	//
	// Each arm targets a backend path whose handler emits exactly ONE illegal
	// connection-specific RESPONSE header (see ../backends/main.go), and reads
	// the DOWNSTREAM header block back off the wire with a raw framer. The
	// proxy sits between the two: what the transcript records is what the
	// proxy chose to forward.
	//
	// ⚠️ NO YAML EDIT. Every p92 path sits under /api, so both sides'
	// `- match: { prefix: "/api" }` -> c_h2_backend already routes it. See
	// p92PathBase for why a top-level `/p92-*` path would NOT.
	//
	// ⚠️ THE [3,3,3] DISTRIBUTION IS UNTOUCHED. counts[] is incremented ONLY
	// inside the `/api/v1/<n>` loop far above, which has already completed;
	// these five requests are appended after it exactly as the phase-88/89/90
	// arms are, so AssertDistribution still sees 9 counted requests per side.
	//
	// ⚠️ NOT FAIL-FAST, and NO STATUS ASSERTION. Exactly ONE
	// `p92-<arm>:status=… illegal=…` line is emitted per arm, ALWAYS, plus an
	// ERR line when the arm did not complete — so a red arm cannot make a
	// later arm unreachable. The STATUS IS IN THE LINE rather than asserted
	// in-band on purpose: the two sides are EXPECTED to disagree on it
	// pre-fix, and the cross-side byte compare IS the assertion.
	//
	// ⚠️ THE `content-length` OBSERVABLES ARE DELIBERATELY NOT IN THIS LINE —
	// the field arity, the declared value AND the delivered body length are
	// all carried out through p92CL and pinned PER SIDE instead. Local-reply
	// body composition is an EXCLUDED cross-side axis for this fixture
	// (BEHAVIOR_CONTRACT.md:1993; the 404 catch-all bodies are relaxed for the
	// same reason), so the two sides' 502 bodies differ by construction and
	// putting their length into the byte stream would red the compare for a
	// difference that is ratified. See p92AssertCLFields,
	// p93AssertDeclaredDelivered and p93AssertBodyLen for the pins, and the
	// README's phase-92 departure note for the root cause.
	for _, arm := range p92Arms() {
		fields, status, bodyLen, failure := p92DriveArm(ctx, addr, tlsConf, arm)
		if failure != "" {
			fmt.Fprintf(&out, "p92-%s:ERR %s\n", arm.name, failure)
		}
		fmt.Fprintf(&out, "p92-%s:status=%d illegal=%s\n",
			arm.name, status, p92IllegalRendering(fields))
		if p92CL != nil {
			*p92CL = append(*p92CL, p93Observe(arm.name, fields, bodyLen))
		}
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

// ---------------------------------------------------------------------------
// Phase-92 illegal RESPONSE header arms (raw framer, ONE FRESH CONNECTION EACH)
// ---------------------------------------------------------------------------

const (
	// p92ArmDeadline bounds each phase-92 arm's dial + write + read loop.
	p92ArmDeadline = 10 * time.Second

	// p92StreamID is the single client-initiated stream each arm uses. ONE
	// FRESH connection per arm, so it is always stream 1.
	p92StreamID = 1

	// p92HpackTableSize is the RESPONSE decoder's dynamic-table size. The
	// decoder is created PER CONNECTION, never shared across arms: the HPACK
	// dynamic table is connection-scoped, and a shared decoder yields
	// "invalid indexed representation index NN" plus a truncated field list on
	// every arm after the first — which reads exactly like "headers were lost".
	p92HpackTableSize = 4096

	// p92Authority is the FIXED literal authority every p92 arm sends. Never
	// the dial address: the reference dials a mapped container port and the
	// subject a local one, so an address-derived value would break cross-side
	// byte equality by construction.
	p92Authority = "p92.example"

	// p92PathBase is the backend prefix the phase-92 emitters live under.
	//
	// ⚠️ IT IS UNDER /api DELIBERATELY. Both sides' route tables end with
	// `- match: { prefix: "/api" }` -> c_h2_backend, so these paths need NO
	// YAML edit. A TOP-LEVEL `/p92-*` path does NOT match that prefix — it
	// falls through to `- match: { prefix: "/" }` and is answered by a 404
	// direct_response that never reaches a backend at all, which would make
	// every arm below vacuous on BOTH sides.
	p92PathBase = "/api/v1/p92-"

	// p92NoIllegal is what the transcript records when the SET of illegal
	// response-header names is EMPTY. A literal, so an empty set stays visibly
	// distinct from a truncated or absent field.
	p92NoIllegal = "<none>"

	// p92TETrailers is the ONE `te` value RFC 9113 section 8.2.2 permits on an
	// H/2 message. EVERY other value is illegal, INCLUDING the empty string —
	// which is why the te-empty arm exists as its own shape.
	p92TETrailers = "trailers"
)

// p92ConnectionSpecific is the roster of response-header names that are
// illegal on an H/2 message by NAME ALONE (RFC 9113 section 8.2.2). `te` is
// deliberately NOT in it: `te` is illegal by VALUE, and p92IllegalSet handles
// that separately.
//
// ⚠️ EVERY ARM SCANS THE WHOLE ROSTER, not just the one shape its own backend
// path emits. A shared code path defeats per-arm counts: a fix that suppresses
// one field while laundering another must show up as a CHANGE IN THE SET, so
// the transcript records the SET of illegal names present, NEVER a single name.
var p92ConnectionSpecific = []string{
	"connection",
	"keep-alive",
	"proxy-connection",
	"transfer-encoding",
	"upgrade",
}

// p92Arm is one phase-92 arm: the transcript marker and the backend path whose
// handler emits exactly ONE illegal response header.
type p92Arm struct {
	name string
	path string
}

// p92Arms returns the phase-92 arm roster. ONE ILLEGAL SHAPE PER ARM.
//
// A single path emitting all shapes at once would be BLIND to a fix that
// catches one and launders another: that arm's illegal set would stay
// non-empty either way and no per-shape verdict could be read out of it.
//
// ⚠️ WHY THESE FIVE AND NOT NINE. The fixture backend is net/http +
// http2.ConfigureServer. x/net's H/2 server deletes ONLY `Connection` from a
// handler's response header map (the delete carries a live
// `TODO: remove more Connection-specific header fields here` right beside it),
// so `keep-alive`, `upgrade` and `proxy-connection` reach the upstream wire
// verbatim. FOUR further illegal shapes are STRUCTURALLY UNREACHABLE from such
// a backend and are pinned at the unit layer instead: `connection` (deleted),
// `transfer-encoding` (the H/2 server frames the body itself), an UPPERCASE
// wire name (the HPACK encoder lowercases every name), and a DUPLICATE
// `content-length` (the server synthesizes exactly one).
//
// ⚠️ THE TWO `te` ARMS ARE HERE BECAUSE THEY WERE MEASURED, NOT ASSUMED.
// Whether a `te` field set by a net/http handler survives onto the H/2 wire
// was an open question; a raw-framer probe against the fixture backend alone
// (no proxy in the path) read BOTH `te: gzip` and `te: ""` back off the wire
// verbatim, so both are permanent wire arms rather than unit-only shapes.
func p92Arms() []p92Arm {
	return []p92Arm{
		{"keepalive", p92PathBase + "keepalive"},
		{"upgrade", p92PathBase + "upgrade"},
		{"proxyconn", p92PathBase + "proxyconn"},
		{"te-gzip", p92PathBase + "te-gzip"},
		{"te-empty", p92PathBase + "te-empty"},
	}
}

// p92Fields builds an arm's request field list: the four pseudo-headers in one
// contiguous block and nothing else. The REQUEST is deliberately boring — this
// row's subject is the RESPONSE direction.
func p92Fields(path string) []hpack.HeaderField {
	return []hpack.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":path", Value: path},
		{Name: ":authority", Value: p92Authority},
	}
}

// p92DriveArm opens a FRESH TLS(ALPN h2) connection, sends the arm's request
// with a raw framer, and returns the RESPONSE header fields EXACTLY AS THEY
// ARRIVED ON THE WIRE, the observed :status, the TOTAL DATA-frame payload
// bytes delivered on the stream, and a NON-EMPTY failure string when the arm
// did not complete. It NEVER returns an error, so no arm can make a later arm
// unreachable.
//
// ⚠️ THE BODY LENGTH IS A RETURNED OBSERVATION, THE BODY TEXT IS NOT. See the
// DATA-frame case below for why the two are treated differently.
//
// ⚠️ A RAW FRAMER IS REQUIRED, not helpers.H2RoundTrip. That helper hands the
// response to net/http and then rebuilds the field list from `resp.Header` — a
// MAP. That canonicalizes every name (`keep-alive` -> `Keep-Alive`), collapses
// arity, and iterates in randomized order, so the field list it returns is
// neither the wire truth nor byte-stable across runs. The illegal shapes this
// row exists to observe are precisely the ones a header map destroys.
//
// ⚠️ Failure strings are scrubbed with p90ScrubAddr — the SAME helper the
// phase-90 arms use, deliberately reused rather than duplicated so both
// families scrub byte-identically. Reference and subject dial different
// addresses, so an unscrubbed error text can never compare equal.
func p92DriveArm(ctx context.Context, addr string, tlsConf *tls.Config, a p92Arm) ([]hpack.HeaderField, int, int, string) {
	fail := func(stage string, err error) ([]hpack.HeaderField, int, int, string) {
		return nil, 0, 0, stage + ": " + p90ScrubAddr(err.Error(), addr)
	}

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: p92ArmDeadline},
		Config:    tlsConf,
	}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fail("dial", err)
	}
	conn, ok := raw.(*tls.Conn)
	if !ok {
		_ = raw.Close()
		return nil, 0, 0, "dial: not a TLS connection"
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(p92ArmDeadline))

	// 0004 is TLS+ALPN-h2, NOT h2c: assert the negotiated protocol BEFORE the
	// preface is written.
	if proto := conn.ConnectionState().NegotiatedProtocol; proto != "h2" {
		return nil, 0, 0, fmt.Sprintf("alpn: negotiated %q, want h2", proto)
	}

	if _, err := io.WriteString(conn, http2.ClientPreface); err != nil {
		return fail("preface", err)
	}
	fr := http2.NewFramer(conn, conn)
	fr.ReadMetaHeaders = hpack.NewDecoder(p92HpackTableSize, nil)
	if err := fr.WriteSettings(); err != nil {
		return fail("settings", err)
	}

	block, err := p90EncodeHeaderBlock(p92Fields(a.path))
	if err != nil {
		return fail("hpack-encode", err)
	}
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      p92StreamID,
		BlockFragment: block,
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		return fail("write-headers", err)
	}

	status := 0
	bodyLen := 0
	var fields []hpack.HeaderField
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			return fields, status, bodyLen, "read-frame: " + p90ScrubAddr(err.Error(), addr)
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
			if f.StreamID != p92StreamID {
				continue
			}
			// ⚠️ FIRST header block ONLY. A TRAILING block would otherwise
			// overwrite the response header fields this arm exists to observe.
			if fields == nil {
				fields = f.Fields
				for _, hf := range f.Fields {
					if hf.Name == ":status" {
						if n, convErr := strconv.Atoi(hf.Value); convErr == nil {
							status = n
						}
					}
				}
			}
			if f.StreamEnded() {
				return fields, status, bodyLen, ""
			}
		case *http2.DataFrame:
			if f.StreamID != p92StreamID {
				continue
			}
			// ⚠️ THE BODY TEXT IS STILL NOT RECORDED — BUT ITS LENGTH NOW IS.
			// A forwarded 200 "p92-ok" and a locally generated 502 carry
			// different, side-specific TEXT, so the BYTES are not a cross-side
			// contract and never enter the diff stream. The LENGTH is a
			// different observable and the one an instrument needs: it is what
			// makes a `content-length` that lies about its own body visible
			// (RFC 9110 §8.6), and it is asserted PER SIDE in
			// AssertDistribution, never cross-side.
			bodyLen += len(f.Data())
			if f.StreamEnded() {
				return fields, status, bodyLen, ""
			}
		case *http2.RSTStreamFrame:
			if f.StreamID != p92StreamID {
				continue
			}
			return fields, status, bodyLen, fmt.Sprintf("rst-stream code=%v", f.ErrCode)
		case *http2.GoAwayFrame:
			return fields, status, bodyLen, fmt.Sprintf("goaway code=%v", f.ErrCode)
		default:
			// WINDOW_UPDATE / PRIORITY / unknown: not recorded.
		}
	}
}

// p92IllegalSet returns the SORTED SET of illegal header names present in a
// response field list: every p92ConnectionSpecific name, plus `te` whenever its
// value is anything other than p92TETrailers (the empty value included).
//
// ⚠️ A SET, NEVER A SINGLE NAME. See p92ConnectionSpecific.
func p92IllegalSet(fields []hpack.HeaderField) []string {
	seen := make(map[string]bool, len(p92ConnectionSpecific)+1)
	for _, hf := range fields {
		n := strings.ToLower(hf.Name)
		for _, bad := range p92ConnectionSpecific {
			if n == bad {
				seen[n] = true
			}
		}
		if n == "te" && hf.Value != p92TETrailers {
			seen[n] = true
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// p92IllegalRendering renders p92IllegalSet for the transcript. SORTED and
// comma-joined, so the rendering is stable regardless of wire order, and
// p92NoIllegal when the set is empty.
func p92IllegalRendering(fields []hpack.HeaderField) string {
	names := p92IllegalSet(fields)
	if len(names) == 0 {
		return p92NoIllegal
	}
	return strings.Join(names, ",")
}

// p92ContentLengthFields counts the `content-length` FIELDS in a response.
//
// ⚠️ THIS HELPER IS THE ARITY AND ONLY THE ARITY. Arity is the observable a
// duplicate-`content-length` regression moves, and it is what lets p93Observe
// tell an ABSENT header from a DUPLICATED one. The VALUE is read separately by
// p93ContentLength and compared only PER SIDE — never cross-side, because the
// two sides' 502 bodies still differ by construction (a forwarded 200 "p92-ok"
// against a locally generated `bad gateway\n`).
func p92ContentLengthFields(fields []hpack.HeaderField) int {
	n := 0
	for _, hf := range fields {
		if strings.ToLower(hf.Name) == "content-length" {
			n++
		}
	}
	return n
}

// p93CLObs is ONE phase-92 arm's `content-length` observation: what the
// response DECLARED and what it actually DELIVERED.
//
// ⚠️ `declared` IS THREE-STATE, AND `declaredOK` IS THE ONLY LEGAL WAY TO READ
// IT. A response's `content-length` can be ABSENT, DUPLICATED (malformed per
// RFC 9113 §8.1.1), or exactly one parsable integer — and ONLY the third state
// sets declaredOK. Pre-fix this very fixture measured `arity=0
// declared=<absent>` on all five SUBJECT arms: ABSENT, not zero. Modeling
// `declared` as a bare int would have reported a `content-length: 0` that was
// never on the wire and let an absent header pass against an empty body.
//
// The arm NAME is carried in the observation so p93AssertRoster can check
// per-index identity against the LIVE p92Arms() roster.
type p93CLObs struct {
	arm        string // p92Arms() arm name this observation came from
	arity      int    // number of `content-length` FIELDS seen on the wire
	declared   int    // the parsed value — MEANINGLESS unless declaredOK
	declaredOK bool   // true iff EXACTLY ONE field carried a parsable value
	bodyLen    int    // total DATA-frame payload bytes delivered
}

// p93Observe builds one arm's observation from the wire header fields and the
// delivered body length p92DriveArm summed off the DATA frames.
func p93Observe(arm string, fields []hpack.HeaderField, bodyLen int) p93CLObs {
	declared, ok := p93ContentLength(fields)
	return p93CLObs{
		arm:        arm,
		arity:      p92ContentLengthFields(fields),
		declared:   declared,
		declaredOK: ok,
		bodyLen:    bodyLen,
	}
}

// p93ContentLength reads a response's `content-length` VALUE as a three-state
// observation: (n, true) only when EXACTLY ONE field carries a non-negative
// decimal, and (0, false) in every other case.
//
// ⚠️ A DUPLICATED `content-length` YIELDS declaredOK=false, NEVER "the first
// value". A duplicate is malformed per RFC 9113 §8.1.1 and its value is
// meaningless; returning the first would launder a real defect into a
// plausible number that p93AssertDeclaredDelivered might then accept.
//
// ⚠️ THE ZERO RETURNED ON FAILURE IS NOT AN OBSERVATION. Callers MUST branch on
// the bool: an absent header and a `content-length: 0` are different facts.
func p93ContentLength(fields []hpack.HeaderField) (int, bool) {
	var val string
	n := 0
	for _, hf := range fields {
		if strings.ToLower(hf.Name) == "content-length" {
			val = hf.Value
			n++
		}
	}
	if n != 1 {
		return 0, false
	}
	v, err := strconv.Atoi(val)
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

// declaredString renders the three-state declared value for an error message,
// naming WHICH non-observation state an arm is in rather than printing a `0`
// that was never on the wire.
func (o p93CLObs) declaredString() string {
	switch {
	case o.declaredOK:
		return strconv.Itoa(o.declared)
	case o.arity == 0:
		return "absent"
	case o.arity > 1:
		return fmt.Sprintf("duplicated(x%d)", o.arity)
	default:
		return "unparsable"
	}
}

// PER-SIDE `content-length` pins: field arity, and the delivered body length.
//
// ⚠️ THESE ARE A DOCUMENTED DEPARTURE, PINNED IN BOTH DIRECTIONS — not an
// exemption. On every phase-92 arm both sides answer 502 from a LOCAL REPLY.
// The two local replies still differ in their body TEXT, but they no longer
// differ in whether they declare its length:
//
//   - the REFERENCE emits a `content-length` on its 502  -> arity 1
//   - the SUBJECT now emits one too                      -> arity 1
//
// ⚠️ PHASE 92 PREDICTED THIS EXACT REDNESS, AND THE PREDICTION CAME TRUE — THE
// PIN WORKED AS DESIGNED. The comment this block replaces read: "if envoy-go
// later gains a Content-Length on H/2 local replies, or the reference stops
// emitting one, THESE PINS REDDEN and a human must consciously re-derive
// them", and it recorded closing the gap as BANKED — "its own behavior-contract
// row". THIS row is that row. Phase 93 gave `h2LocalReplyHeaders()`
// (internal/filter/http/router/router_h2.go) the bodyLen its H/1 sibling
// `localReplyHeaders(bodyLen int)` (router.go) always had, so it now always
// emits a Content-Length across the 502/503/504 H/2 local-reply sites. The
// subject-side pin then went red with precisely that message and was
// CONSCIOUSLY re-derived 0 -> 1, with its negative control inverted alongside
// it so the row still fails when the observation disagrees. The pin is
// re-baselined here, not weakened, and this note is the record of it firing.
//
// Pinning per side rather than cross-side keeps the REMAINING departure
// visible: if either side stops emitting a Content-Length, or either side's
// local-reply body length moves, these pins redden again and a human must
// consciously re-derive them again. Deleting the observable would have hidden
// it.
//
// ⚠️ ARITY AND LENGTH, NEVER BODY BYTES — the old "ARITY, NEVER VALUE" rule is
// NARROWED, not dropped. The two 502 bodies still differ by construction (the
// reference's own local-reply text against envoy-go's `bad gateway\n`), and
// local-reply body bytes are an EXCLUDED cross-side axis for this fixture per
// BEHAVIOR_CONTRACT.md:1993 — so the BYTES are compared on neither side and
// never enter the diff stream. What IS now pinned is numeric and per-side:
// each side's delivered body LENGTH against its own measured constant, plus
// the one genuinely side-INDEPENDENT invariant — a declared `content-length`
// must equal the body actually delivered (RFC 9110 §8.6). That invariant is
// NOT a departure and is relaxed on NEITHER side; it is the only pin that can
// still see a `content-length` lying about its own body now that arity reads 1
// everywhere.
//
// ⚠️ THE BODY-LENGTH VALUES ARE MEASURED, NOT DERIVED. 12 is
// len("bad gateway\n") on the subject side; 87 is the length of the
// reference's own 502 local-reply text, read off the wire by the differential
// run that baselined it. A change to either side's local-reply text moves
// them, and that is the point.
const (
	p92WantRefCLFields  = 1
	p92WantSubjCLFields = 1

	p93WantRefBodyLen  = 87
	p93WantSubjBodyLen = 12
)

// p92AssertCLFields checks one side's per-arm `content-length` field arity
// against that side's measured pin. side is "ref" or "subj" and appears in the
// error so a red run names the side without the caller re-wrapping.
//
// The observation count is asserted against the LIVE p92Arms() roster first:
// an empty or short observation slice would otherwise satisfy the per-arm loop
// vacuously, and this pin has to fail when the arms did not run at all.
//
// ⚠️ NOT FAIL-FAST ACROSS ARMS. Every mismatching arm is named in the returned
// error, not just the first. Returning on the first would let one arm MASK its
// four siblings, exactly as the runner's first-divergence byte comparator does
// — and a break that reddens only the first arm is not evidence the other four
// are pinned at all.
func p92AssertCLFields(side string, want int, got []p93CLObs) error {
	arms := p92Arms()
	if len(got) != len(arms) {
		return fmt.Errorf("p92 %s content-length arity: got %d observations, want %d (one per arm)",
			side, len(got), len(arms))
	}
	var bad []string
	for i, o := range got {
		if o.arity != want {
			bad = append(bad, fmt.Sprintf("%s=%d", arms[i].name, o.arity))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("p92 %s content-length fields: want %d on every arm, got %s (%d of %d arms)",
			side, want, strings.Join(bad, ","), len(bad), len(arms))
	}
	return nil
}

// p93AssertRoster is the NON-VACUITY BARRIER for the per-arm phase-93 pins.
//
// ⚠️ WITHOUT IT, ZERO OBSERVATIONS SATISFIES EVERY PER-ARM PIN. Each pin below
// is a range loop over the observations, so an EMPTY slice makes every one of
// them return nil. MEASURED without a barrier: a side that recorded nothing at
// all passed the declared==delivered pin and BOTH body-length pins silently.
// The barrier is therefore the GATE for those pins — AssertDistribution runs
// them only when it holds — so a run with no observations names the missing
// roster ONCE instead of letting the gated pins report the same absence in
// three different vocabularies, or worse, not report it at all.
//
// ⚠️ THE ROSTER IS READ FROM THE LIVE p92Arms(), NEVER A LITERAL, and identity
// is checked PER INDEX. A count-only check would accept a five-entry slice
// holding the WRONG five arms, and a literal 5 would go stale the moment an
// arm is added, removed or renamed.
func p93AssertRoster(side string, got []p93CLObs) error {
	arms := p92Arms()
	if len(got) != len(arms) {
		return fmt.Errorf("p93 %s observation roster: got %d observations, want %d (one per p92Arms() arm)",
			side, len(got), len(arms))
	}
	var bad []string
	for i, o := range got {
		if o.arm != arms[i].name {
			bad = append(bad, fmt.Sprintf("[%d]=%q want %q", i, o.arm, arms[i].name))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("p93 %s observation roster: arm identity mismatch: %s (%d of %d arms)",
			side, strings.Join(bad, ","), len(bad), len(arms))
	}
	return nil
}

// p93AssertDeclaredDelivered checks, for ONE side, that every arm's DECLARED
// `content-length` equals the body length actually DELIVERED on that stream.
//
// ⚠️ THIS IS THE ONE PROPERTY HERE THAT IS NOT A DEPARTURE. RFC 9110 §8.6
// requires a `content-length` to state the real length of the body it
// accompanies. It must hold on BOTH sides independently, it is relaxed on
// neither, and it is asserted as plain equality. It is also the ONLY pin that
// can still see a header lying about its own body now that arity reads 1 on
// both sides: the arity pin counts fields and cannot, and the per-side length
// pins only see the delivered half.
//
// ⚠️ AN ARM WITH declaredOK=false FAILS THIS PIN. Absent and duplicated both
// mean "there is no declared length to compare"; treating either as 0 would
// let a missing or malformed header pass against an empty body.
//
// ⚠️ NOT FAIL-FAST ACROSS ARMS — every violating arm is named, never just the
// first. See p92AssertCLFields for why.
func p93AssertDeclaredDelivered(side string, got []p93CLObs) error {
	var bad []string
	for _, o := range got {
		if !o.declaredOK || o.declared != o.bodyLen {
			bad = append(bad, fmt.Sprintf("%s=declared %s/delivered %d", o.arm, o.declaredString(), o.bodyLen))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("p93 %s declared != delivered: %s (%d of %d arms)",
			side, strings.Join(bad, ","), len(bad), len(got))
	}
	return nil
}

// p93AssertBodyLen checks one side's per-arm DELIVERED body length against that
// side's OWN measured pin.
//
// ⚠️ PER SIDE, NEVER CROSS-SIDE. The two 502 local-reply bodies differ by
// construction and that difference is ratified (BEHAVIOR_CONTRACT.md:1993), so
// this is a departure recorded in both directions — not a cross-side equality.
// It is what makes the declared==delivered invariant non-vacuous: without it,
// a side that delivered 0 bytes and declared 0 would satisfy the invariant.
//
// ⚠️ NOT FAIL-FAST ACROSS ARMS — every violating arm is named.
func p93AssertBodyLen(side string, want int, got []p93CLObs) error {
	var bad []string
	for _, o := range got {
		if o.bodyLen != want {
			bad = append(bad, fmt.Sprintf("%s=%d", o.arm, o.bodyLen))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("p93 %s body length: want %d on every arm, got %s (%d of %d arms)",
			side, want, strings.Join(bad, ","), len(bad), len(got))
	}
	return nil
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

// DriveReference runs 47 H2 round-trips against the reference proxy listener
// and records per-backend body counts on d.refBodyCnt for AssertDistribution.
func (d *h2Driver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	var counts [3]uint64
	var p92CL []p93CLObs
	b, err := d.drive(ctx, addr, &counts, &p92CL)
	if err != nil {
		return nil, fmt.Errorf("ref drive: %w", err)
	}
	d.mu.Lock()
	d.refBodyCnt = counts
	d.refP92CL = p92CL
	d.mu.Unlock()
	return b, nil
}

// DriveSubject runs 47 H2 round-trips against the subject proxy listener and
// records per-backend body counts on d.subjBodyCnt for AssertDistribution.
func (d *h2Driver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	var counts [3]uint64
	var p92CL []p93CLObs
	b, err := d.drive(ctx, addr, &counts, &p92CL)
	if err != nil {
		return nil, fmt.Errorf("subj drive: %w", err)
	}
	d.mu.Lock()
	d.subjBodyCnt = counts
	d.subjP92CL = p92CL
	d.mu.Unlock()
	return b, nil
}

// AssertDistribution asserts both ref and subj per-cluster RR distributions
// are exactly [3,3,3] over the 9 router-action /api requests, AND the per-side
// `content-length` pins: field arity, declared==delivered, and the delivered
// body length.
//
// The runner-supplied refCounts / subjCounts are zero for HTTPSH2 backends
// (subprocess backends don't increment the in-process counter). The driver
// therefore consults the body-derived counts it recorded during DriveReference
// / DriveSubject. The incoming counters are accepted but only used for a
// length sanity check; their values are deliberately ignored.
//
// ⚠️ THIS IS THE FIXTURE'S POST-DIFF IN-BAND ASSERTION HOOK, and that is WHY
// the phase-92 arity pins live here rather than in DriveReference /
// DriveSubject. The runner calls AssertDistribution at step 8 — AFTER the
// cross-side CompareBytes at step 7 — and surfaces its error with t.Errorf,
// not t.Fatalf. A Drive-level return would have been t.Fatalf'd BEFORE the byte
// compare ever ran. MEASURED: with the production guard reverted, a Drive-level
// arity pin reported ONLY "p92 subj content-length fields: …" and the row's own
// status/illegal divergence was never reached — the out-of-charter pin MASKED
// the charter regression. Here both are reported by the same run.
//
// ⚠️ EVERY failing property is reported, never just the first: the distribution
// rule, the two arity pins and the phase-93 declared/delivered pins are
// independent, and returning on the first would let any of them mask the
// others. The ONE exception is deliberate — the per-arm phase-93 pins are
// GATED on p93AssertRoster, because when a side recorded no observations at
// all those pins pass VACUOUSLY and the roster is the only truthful report.
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
	refCL := d.refP92CL
	subjCL := d.subjP92CL
	d.mu.Unlock()

	var errs []error
	want := [3]uint64{3, 3, 3}
	if subj != want {
		errs = append(errs, fmt.Errorf("subj distribution %v != %v (RR [3,3,3] expected)", subj, want))
	}
	if ref != want {
		errs = append(errs, fmt.Errorf("ref distribution %v != %v (RR [3,3,3] expected)", ref, want))
	}
	if err := p92AssertCLFields("ref", p92WantRefCLFields, refCL); err != nil {
		errs = append(errs, err)
	}
	if err := p92AssertCLFields("subj", p92WantSubjCLFields, subjCL); err != nil {
		errs = append(errs, err)
	}
	// The phase-93 declared/delivered pins, BELOW the byte compare like every
	// other pin here, and GATED on the roster barrier: each is a range loop
	// over the per-arm observations, so an empty slice would satisfy all of
	// them vacuously. p93AssertRoster is what turns "the arms never ran" into a
	// failure; when it fires the pins it gates are skipped, so the roster is
	// reported once rather than three times in three vocabularies.
	for _, side := range []struct {
		name        string
		obs         []p93CLObs
		wantBodyLen int
	}{
		{"ref", refCL, p93WantRefBodyLen},
		{"subj", subjCL, p93WantSubjBodyLen},
	} {
		if err := p93AssertRoster(side.name, side.obs); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := p93AssertDeclaredDelivered(side.name, side.obs); err != nil {
			errs = append(errs, err)
		}
		if err := p93AssertBodyLen(side.name, side.wantBodyLen, side.obs); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
