// Package driver is the differential fixture driver for
// 0121-listener-default-chain-tls: the listener-scope ssl.* differential on a
// listener whose ONLY TLS context is its default_filter_chain.
//
// The listener under test (l_dfc) carries NO filter_chains[] key at all — it is
// the first such listener in this tree. It terminates TLS from its
// default_filter_chain and answers every request with a direct_response 200.
//
// THE PROPOSITION: both sides register the five listener-scope ssl.* counters
// on that shape, and both book ssl.handshake AND ssl.no_certificate once per
// COMPLETED handshake. The driver drives THREE arms; each completes a TLS
// handshake AND a full HTTP round trip, and asserts HTTP 200 BEFORE any counter
// is believed.
package driver

import (
	"bufio"
	"bytes"
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0121-listener-default-chain-tls"

	// In-container reference Envoy ports.
	//
	// ⚠️ THE LISTENER PORT IS 10127, NOT 10121. The "10<fixture index>"
	// convention would yield 10121, but 0028 HOLDS 10120-10125 as one
	// contiguous six-listener run (0028/inputs/driver.go:65-70) and 0120 holds
	// 10126. 10127 is the first free port above both. Censused at this tip:
	// `git grep -n 10127 -- test/ internal/ cmd/` reads ZERO hits, while the
	// negative control `git grep -n 10126` reads four occupied files. The
	// reasoning is recorded here and in README.md so the next fixture does not
	// re-derive it.
	refAdminPort    = 9901
	refListenerPort = 10127

	// serverName is the TLS ServerName every arm dials with. It MUST match a
	// DNS SAN on the committed pki/server.pem leaf (DNS:l_dfc.fixture.test,
	// DNS:localhost, DNS:host.docker.internal) or every arm fails verification
	// CLIENT-side, never reaches the server, and the five ssl.* counters read
	// zero with NO server-side fault at all — a vacuous red that looks like a
	// real one.
	serverName = "l_dfc.fixture.test"

	// wantBody is the direct_response payload configured in BOTH bootstraps. It
	// is asserted per arm, per side, BEFORE any counter is read: a counter map
	// scraped from a listener that did not actually serve is not evidence.
	wantBody = "phase96-default-chain-ok\n"

	// armDeadline bounds each arm's dial+handshake+write+read. Generous on
	// purpose: a too-tight bound would let a SLOW accept masquerade as a
	// handshake failure.
	armDeadline = 10 * time.Second

	// armCount is the DRIVE COUNT, N. It is 3 and not 1 by decision, not by
	// habit: at N=1 the value 1 is consistent BOTH with a per-connection
	// counter and with a fire-once one, so N=1 cannot discriminate them. N=3
	// can. Every want* below is arm arithmetic derived from it — ⚠️ changing
	// armCount INVALIDATES the four nonzero pins.
	armCount = 3

	// The MEASURED expectation map. Measured on BOTH sides at N=1 and N=3 on
	// exactly this fixture's shape, with the reference in a container launched
	// by the pinned digest and the subject carrying the one-line predicate
	// repair; all five values identical cross-side at both drive counts.
	//
	//   stat                     ref N=1  subj N=1  ref N=3  subj N=3
	//   ssl.handshake                  1         1        3         3
	//   ssl.no_certificate             1         1        3         3
	//   ssl.fail_verify_error          0         0        0         0
	//   ssl.fail_verify_no_cert        0         0        0         0
	//   ssl.connection_error           0         0        0         0
	//
	// ⚠️ wantNoCertificate IS armCount, NOT 0. The listener carries no
	// require_client_certificate and no validation_context, so it sends no
	// CertificateRequest — and a handshake that completes without a client
	// certificate books BOTH ssl.handshake and ssl.no_certificate. A
	// {handshake: N, everything else: 0} map would fail against CORRECT code,
	// on BOTH sides, at both drive counts.
	wantHandshake       = armCount
	wantNoCertificate   = armCount
	wantFailVerifyError = 0
	wantFailVerifyNoCer = 0
	wantConnectionError = 0

	// wantDownstreamCxTotal is the LIVENESS co-assertion. It stands where a
	// no_filter_chain_match pin would otherwise have gone — see AssertStats for
	// why that name must not be asserted at all. downstream_cx_total is emitted
	// by BOTH sides and reads 3 at N=3.
	wantDownstreamCxTotal = armCount
)

// Prometheus metric names, all anchored on the envoy_listener_ssl prefix.
//
// ⚠️ ANCHORING IS LOAD-BEARING. An unanchored "ssl" match over a proxy's stats
// reads FOUR hits on the reference (http.<prefix>.downstream_cx_ssl_active /
// _total pairs) and ONE on the subject (server.acce**ssl**og_dropped) — both
// witnesses appear simultaneously, one per side, on the same config pair. The
// dotted-stats equivalent of this prefix is ^listener\.[^:]*\.ssl\.
const (
	mSSLHandshake       = "envoy_listener_ssl_handshake"
	mSSLNoCertificate   = "envoy_listener_ssl_no_certificate"
	mSSLFailVerifyError = "envoy_listener_ssl_fail_verify_error"
	mSSLFailVerifyNoCer = "envoy_listener_ssl_fail_verify_no_cert"
	mSSLConnectionError = "envoy_listener_ssl_connection_error"
	mDownstreamCxTotal  = "envoy_listener_downstream_cx_total"
)

// defaultChainTLSDriver is the fixture driver. It is stateless: every arm builds
// its own TLS config from the COMMITTED pki/ directory, so there is no ensure()
// step and no generated-PKI race between the two sides.
type defaultChainTLSDriver struct{}

func init() { fixture.RegisterFixture(fixtureName, &defaultChainTLSDriver{}) }

// Compile-time interface assertions.
//
// ⚠️ THE StatsAsserter ONE IS MANDATORY, NOT DECORATIVE. The runner dispatches
// the stats step via a SILENT type assertion with NO else branch, so a
// signature typo makes ok == false and the ENTIRE stats leg — which is where
// all of this fixture's discrimination lives — never runs, while gofmt, vet,
// the linter and the suite all stay quiet and green.
var (
	_ fixture.Driver        = (*defaultChainTLSDriver)(nil)
	_ fixture.StatsAsserter = (*defaultChainTLSDriver)(nil)
)

// BackendCount is 1 even though direct_response NEVER reaches the backend: the
// runner t.Fatalf's on BackendCount() < 1, and an omitted clusters: key
// BOOT-REJECTS envoy-go. The allocated backend port is templated into the
// placeholder cluster and nothing ever dials it.
func (*defaultChainTLSDriver) BackendCount() int           { return 1 }
func (*defaultChainTLSDriver) SubjectListenerName() string { return "l_dfc" }
func (*defaultChainTLSDriver) ReferenceListenerPort() int  { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with the fixed in-container ports, the
// runner-allocated placeholder backend port, and the two COMMITTED PEMs
// pre-indented for their YAML block scalars.
//
// ⚠️ CERT DELIVERY IS inline_string:, NEVER filename:. pki/ exists on the HOST;
// it does not exist inside the reference CONTAINER, and this fixture implements
// no ReferenceLogMounter bind-mount.
func (*defaultChainTLSDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":          refAdminPort,
		"ListenerPort":       refListenerPort,
		"BackendPort":        backendPorts[0],
		"ServerCertIndented": indentPEM(mustReadFixtureBytes("pki/server.pem"), 22),
		"ServerKeyIndented":  indentPEM(mustReadFixtureBytes("pki/server.key.pem"), 22),
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports, the loopback placeholder backend port, and the same two committed
// PEMs. The PEMs are byte-identical to the reference side's by construction.
func (*defaultChainTLSDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":          subjAdminPort,
		"ListenerPort":       subjListenerPort,
		"BackendPort":        backendPorts[0],
		"ServerCertIndented": indentPEM(mustReadFixtureBytes("pki/server.pem"), 22),
		"ServerKeyIndented":  indentPEM(mustReadFixtureBytes("pki/server.key.pem"), 22),
	})
}

// DriveReference / DriveSubject: ONE DIRECTORY = ONE RUNNER BRANCH, so both
// delegate to a single driveSide and all three arms sequence INSIDE it.
func (d *defaultChainTLSDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, "reference", addr)
}

func (d *defaultChainTLSDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, "subject", addr)
}

// driveSide sequences armCount identical arms against ONE side, in a FIXED
// order, and returns the concatenated response BODIES so the runner's
// CompareBytes has something real to compare. Bodies — not headers, not the
// raw response — because the body is the direct_response inline_string, which
// is byte-identical in both bootstraps by construction, while status lines and
// header sets carry per-implementation divergences that are not this row's
// subject (reference_wire_format_both_sides_see_same_bytes).
//
// Every arm is evaluated independently and ALL violations are reported in ONE
// error: an early return would make every later arm dead code
// (reference_fatalf_makes_assertions_unreachable, at the fixture layer).
//
// ⚠️ EACH ARM ASSERTS HTTP 200 AND THE BODY BEFORE ANY COUNTER IS BELIEVED. A
// handshake that completes and then serves nothing still books ssl.handshake on
// some shapes; only the round trip proves the listener actually served.
// ⚠️ fixture.TB has no Logf — log.Printf is the recording channel.
func (d *defaultChainTLSDriver) driveSide(ctx context.Context, side, addr string) ([]byte, error) {
	var probs []string
	var out bytes.Buffer

	for i := 1; i <= armCount; i++ {
		arm := fmt.Sprintf("tls_http_%d", i)
		body, err := d.tlsRoundTrip(ctx, side, arm, addr)
		switch {
		case err != nil:
			probs = append(probs, fmt.Sprintf("%s: %v", arm, err))
			log.Printf("0121 %s arm=%s FAILED err=%v", side, arm, err)
		case body != wantBody:
			probs = append(probs, fmt.Sprintf("%s: body = %q, want %q", arm, body, wantBody))
			log.Printf("0121 %s arm=%s BODY-MISMATCH got=%q", side, arm, body)
		default:
			out.WriteString(body)
			log.Printf("0121 %s arm=%s status=200 body=OK", side, arm)
		}
	}

	if len(probs) > 0 {
		return nil, fmt.Errorf("%s: %s", side, strings.Join(probs, "; "))
	}
	return out.Bytes(), nil
}

// tlsRoundTrip is ONE arm: one fresh TCP connection, one TLS handshake against
// the committed fixture CA, one HTTP/1.1 request, one response. It returns the
// response body and asserts the status BEFORE returning it.
//
// One connection per arm is what makes the counter arithmetic legible: a pooled
// or reused connection would book ONE handshake for three requests, and the
// pins would read 1 against a want of 3 with nothing wrong on either side.
// Connection: close plus a per-arm dial guarantees armCount handshakes.
//
// ⚠️ NO CLIENT CERTIFICATE IS PRESENTED, AND NONE IS CONFIGURED. That is the
// point: the listener sends no CertificateRequest, so ssl.no_certificate moves
// with ssl.handshake. There is deliberately no client leaf in pki/.
func (d *defaultChainTLSDriver) tlsRoundTrip(ctx context.Context, side, arm, addr string) (string, error) {
	pool, err := serverCAPool()
	if err != nil {
		return "", err
	}
	dialer := &net.Dialer{Timeout: armDeadline}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = raw.Close() }()
	if err := raw.SetDeadline(time.Now().Add(armDeadline)); err != nil {
		return "", fmt.Errorf("set deadline: %w", err)
	}

	// InsecureSkipVerify stays FALSE: an arm that skipped verification could
	// not tell a correct server leaf from a wrong one, and the whole point of
	// the DNS SAN on pki/server.pem is that this check runs.
	conn := stdtls.Client(raw, &stdtls.Config{
		RootCAs:            pool,
		ServerName:         serverName,
		MinVersion:         stdtls.VersionTLS12,
		NextProtos:         []string{"http/1.1"},
		InsecureSkipVerify: false, //nolint:gosec // explicit: this arm MUST verify
	})
	if err := conn.HandshakeContext(ctx); err != nil {
		return "", fmt.Errorf("tls handshake: %w", err)
	}
	log.Printf("0121 %s arm=%s handshake OK version=%#04x alpn=%q",
		side, arm, conn.ConnectionState().Version, conn.ConnectionState().NegotiatedProtocol)

	req := "GET / HTTP/1.1\r\nHost: " + serverName + "\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return "", fmt.Errorf("write request: %w", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	// ⚠️ THE STATUS CHECK COMES BEFORE THE BODY IS RETURNED AND BEFORE ANY
	// COUNTER IS READ. A non-200 means the direct_response route did not match
	// and the counter map, whatever it says, is not evidence about this shape.
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status = %d, want 200 (direct_response route did not answer)", resp.StatusCode)
	}
	return string(body), nil
}

// serverCAPool is the committed fixture CA, used to VERIFY the server leaf.
func serverCAPool() (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(mustReadFixtureBytes("pki/ca.pem")) {
		return nil, errors.New("pki/ca.pem: no certificate appended")
	}
	return pool, nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint for the
// runner's standard admin-diff probe step (the 0110/0118/0120 shape).
func (*defaultChainTLSDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertStats is the runner's step-10 stats leg (ADR-0062) and is where this
// fixture's discrimination lives. It runs strictly AFTER both Drives.
//
// ⚠️ KEYED ON THE METRIC NAME, WITH THE LABEL SET STRIPPED ENTIRELY. Stripping
// is REQUIRED, not a convenience: the reference renders
// envoy_listener_ssl_handshake{envoy_listener_address="0.0.0.0_10127"} while the
// subject renders the same metric with its IPv6-wildcard form and a
// runner-allocated port, so a label-preserving key is cross-side incomparable
// BY CONSTRUCTION.
//
// ⚠️ EXACT EQUALITY, NEVER A FLOOR. A floor cannot tell a per-connection
// counter from an over-firing one, which is the whole reason N is 3.
//
// ⚠️ NEVER A NAME-SET EQUALITY. Post-drive the reference emits SEVENTEEN
// listener-scope ssl.* names and the subject five (a strict subset). A set
// equality would be red against correct code on both sides. Nor may any
// presence set be taken BEFORE the drive: the reference registers FOURTEEN of
// those names at boot and seventeen only after the first handshake — the three
// latecomers are the dynamic families ssl.ciphers.<suite>, ssl.curves.<curve>
// and ssl.versions.<version>. This assertion is a NAMED SUBSET, taken
// post-drive, and must stay one.
//
// ⚠️ Every violation is reported with Errorf, never Fatalf: a Fatalf on the
// reference side would make every subject-side assertion DEAD CODE
// (reference_fatalf_makes_assertions_unreachable). The only Fatalf is the
// scrape itself, where there is nothing left to assert.
func (d *defaultChainTLSDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	for _, side := range []struct{ name, addr string }{
		{"reference", refAdminAddr},
		{"subject", subjAdminAddr},
	} {
		got, err := scrapeProm(side.addr)
		if err != nil {
			t.Fatalf("%s: scrape: %v", side.name, err)
		}

		// POSITIVE HALF. Both movers, at the drive count. ssl.no_certificate is
		// asserted at armCount and NOT at 0: the listener sends no
		// CertificateRequest, so a completing handshake books both names. This
		// is the pin that a forecast map would have gotten wrong.
		if v := got[mSSLHandshake]; v != wantHandshake {
			t.Errorf("%s: %s = %d, want %d — %d arms each completed a TLS handshake AND a 200 "+
				"round trip on a listener whose only TLS is its default_filter_chain",
				side.name, mSSLHandshake, v, wantHandshake, armCount)
		}
		if v := got[mSSLNoCertificate]; v != wantNoCertificate {
			t.Errorf("%s: %s = %d, want %d — the listener carries NO require_client_certificate "+
				"and NO validation_context, so it sends no CertificateRequest and every completed "+
				"handshake books this name alongside ssl.handshake (it is NOT 0 here)",
				side.name, mSSLNoCertificate, v, wantNoCertificate)
		}

		// ⚠️ NEGATIVE HALF — assert WHICH DID NOT FIRE. A pin proving the two
		// movers moved says nothing about whether a FAILURE classifier also
		// fired. No arm drives a certificate failure or a protocol error (every
		// arm verifies the server leaf against the committed CA and completes),
		// so all three must stay 0 on BOTH sides. Without this half, an
		// implementation that booked every handshake under all five names would
		// still pass the positive half.
		for _, p := range []struct {
			name string
			want uint64
		}{
			{mSSLFailVerifyError, wantFailVerifyError},
			{mSSLFailVerifyNoCer, wantFailVerifyNoCer},
			{mSSLConnectionError, wantConnectionError},
		} {
			if v := got[p.name]; v != p.want {
				t.Errorf("%s: %s = %d, want %d — no arm drives a certificate failure or a "+
					"connection-level TLS error", side.name, p.name, v, p.want)
			}
		}

		// LIVENESS CO-ASSERTION.
		//
		// 🔴 IT IS downstream_cx_total AND NOT no_filter_chain_match, ON
		// PURPOSE. The divergence on that name is NAME-level, not value-level:
		// the reference emits listener.<addr>.no_filter_chain_match at 0 and the
		// subject does not emit the NAME AT ALL (the string has no production
		// site in this repository). Because scrapeProm returns the zero value
		// for a missing key, a `no_filter_chain_match == 0` pin would be
		// SILENTLY VACUOUS on the subject rather than red — it would pass for
		// the wrong reason, forever. downstream_cx_total is emitted by both
		// sides and reads armCount here.
		if v := got[mDownstreamCxTotal]; v != wantDownstreamCxTotal {
			t.Errorf("%s: %s = %d, want %d — liveness: the listener must have accepted exactly "+
				"%d downstream connections", side.name, mDownstreamCxTotal, v, wantDownstreamCxTotal, armCount)
		}
	}
}

// scrapeProm fetches /stats/prometheus and returns a map keyed by metric NAME
// with the label set stripped ENTIRELY. Stripping is REQUIRED here, not a
// convenience: the two sides render envoy_listener_address differently
// ("0.0.0.0_10127" against the subject's IPv6-wildcard form), so any
// label-preserving key would be cross-side incomparable by construction.
//
// Handles the labeled, bare and trailing-timestamp line variants. ParseFloat,
// NOT ParseUint: the exposition format permits float values and histogram lines
// can carry nan/inf. Non-finite and negative values are SKIPPED rather than
// converted — uint64(NaN) is undefined.
func scrapeProm(adminAddr string) (map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	var body bytes.Buffer
	if _, err := body.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read %s body: %w", url, err)
	}

	out := map[string]uint64{}
	for _, line := range strings.Split(body.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var name, rest string
		if open := strings.IndexByte(line, '{'); open >= 0 {
			closeIdx := strings.LastIndexByte(line, '}')
			if closeIdx < open {
				continue // malformed: no closing brace
			}
			name = line[:open]
			rest = strings.TrimSpace(line[closeIdx+1:])
		} else {
			sp := strings.IndexByte(line, ' ')
			if sp < 0 {
				continue
			}
			name = line[:sp]
			rest = strings.TrimSpace(line[sp+1:])
		}
		// Strip an optional trailing timestamp ("<value> <timestamp>").
		if sp := strings.IndexByte(rest, ' '); sp >= 0 {
			rest = rest[:sp]
		}
		v, err := strconv.ParseFloat(rest, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			continue
		}
		out[name] += uint64(v)
	}
	return out, nil
}

// --- file / template helpers (the 0103/0108/0109/0110/0120 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0121-listener-default-chain-tls/driver/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

func mustReadFixtureBytes(name string) []byte {
	path := filepath.Join(fixtureDir(), filepath.FromSlash(name))
	b, err := os.ReadFile(path) //nolint:gosec // fixture-relative path, test-only
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return b
}

func mustReadFixtureFile(name string) string { return string(mustReadFixtureBytes(name)) }

// indentPEM prefixes every line of a multi-line PEM with `spaces` spaces so it
// can be injected under a YAML block scalar (`inline_string: |`). A raw
// fmt.Sprintf of the PEM produces INVALID YAML — every continuation line would
// land at column 0. The trailing newline is trimmed first so the block scalar
// does not gain an empty, space-only final line.
func indentPEM(pemBytes []byte, spaces int) string {
	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimRight(string(pemBytes), "\n"), "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

func mustRender(tpl string, data map[string]any) string {
	t, err := template.New("bootstrap").Parse(tpl)
	if err != nil {
		panic(fmt.Sprintf("driver: template parse: %v", err))
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("driver: template execute: %v", err))
	}
	return buf.String()
}
