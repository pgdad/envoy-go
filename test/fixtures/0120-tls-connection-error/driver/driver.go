// Package driver is the differential fixture driver for
// 0120-tls-connection-error: the listener-scope ssl.connection_error
// differential. It drives FIVE arms against ONE TLS-terminating tcp_proxy
// listener — three connection-level TLS PROTOCOL failures that must increment
// listener.<addr>.ssl.connection_error, one successful mTLS echo, and one
// clean-FIN transport truncation that must NOT increment it.
package driver

import (
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
	fixtureName = "0120-tls-connection-error"

	// In-container reference Envoy ports. The "10<fixture index>" convention
	// yields 10120 for 0120 — but 0028 HOLDS 10120 as part of its 10120-10125
	// run (0028/inputs/driver.go:65-70). 10126 is the minimal index-preserving
	// repair: the first free port above the occupying run. Verified free (zero
	// hits in test/, *.go, *.yaml).
	refAdminPort    = 9901
	refListenerPort = 10126

	// serverName is the TLS ServerName the handshaking arms dial with. It MUST
	// match a DNS SAN on the committed pki/server.pem leaf (DNS:localhost,
	// DNS:l_conn_err.fixture.test) or the positive arm fails verification
	// CLIENT-side and never reaches the server — which would zero the positive
	// control without any server-side fault.
	serverName = "l_conn_err.fixture.test"

	// probePayload is the positive arm's application payload, echoed back
	// verbatim by the TCP echo backend through the tcp_proxy.
	//
	// ⚠️ It is also written by the PLAINTEXT and GARBAGE arms' echo check: those
	// arms must observe NO echo, and comparing against a known payload is what
	// distinguishes "the TLS terminator rejected it" from "the bytes were
	// proxied straight through to the backend".
	probePayload = "phase94-conn-err-probe\n"

	// armDeadline bounds each arm's dial+handshake+write+read. Generous on
	// purpose: a too-tight bound would let a SLOW accept masquerade as a
	// reject on a negative arm.
	armDeadline = 10 * time.Second

	// wantConnectionError and wantHandshake are ABSOLUTE per-side values, not
	// deltas — that is the harness convention (Shape A: scrape ONCE, no baseline
	// subtraction). Nothing pre-moves l_conn_err's ssl.* counters: AssertStats
	// runs at runner step 10, strictly after both Drives and CompareBytes;
	// reference readiness polls admin 9901 (not the TLS port) and the subject
	// parses a stdout sentinel, so driveSide's five arms are the ONLY connections
	// l_conn_err ever sees on either side.
	//
	// ⚠️ THESE ARE ARM ARITHMETIC. Adding a sixth arm INVALIDATES them.
	//
	// MEASURED per-arm on BOTH sides (before/after snapshot per arm, reference =
	// the pinned Envoy image, subject = envoy-go at this tip):
	//
	//   arm                       connection_error   handshake
	//   (v) valid + client cert   +0                 +1
	//   (i) bad version (TLS1.1)  +1                 +0
	//   (ii) plaintext HTTP       +1                 +0
	//   (iii) garbage bytes       +1                 +0
	//   (iv) clean FIN, 0 bytes   +0                 +0
	//
	// Over-firing controls, both sides: 3x clean-FIN -> +0; 3x valid ->
	// handshake +3 only; 2x bad-version -> connection_error +2 only. A positive
	// arm alone cannot catch an OVER-firing counter
	// (reference_positive_arm_cannot_catch_overfiring); those controls can.
	wantConnectionError = 3
	wantHandshake       = 1
)

// connErrDriver is the fixture driver. It is stateless: every arm builds its
// own TLS config from the COMMITTED pki/ directory, so there is no ensure()
// step and no generated-PKI race between the two sides.
type connErrDriver struct{}

func init() { fixture.RegisterFixture(fixtureName, &connErrDriver{}) }

// Compile-time interface assertions. ⚠️ The StatsAsserter one is MANDATORY: the
// runner dispatches the stats step via a SILENT type assertion (runner_test.go
// step 10, no else branch), so a signature typo makes ok == false and the whole
// assertion NEVER RUNS while every tool stays quiet.
var (
	_ fixture.Driver        = (*connErrDriver)(nil)
	_ fixture.StatsAsserter = (*connErrDriver)(nil)
)

// BackendCount stays 1: the failing arms never reach the upstream, but the
// runner rejects 0 (runner_test.go t.Fatalf on BackendCount() < 1). The default
// TCPEcho kind is the minimum viable shape and the positive arm's echo
// round-trip DOES traverse it — see AssertStats for why that round-trip is
// load-bearing rather than decorative. +0 BackendKinds.
func (*connErrDriver) BackendCount() int           { return 1 }
func (*connErrDriver) SubjectListenerName() string { return "l_conn_err" }
func (*connErrDriver) ReferenceListenerPort() int  { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with the fixed in-container ports, the
// runner-allocated backend port, and the three COMMITTED PEMs pre-indented for
// their YAML block scalars.
//
// ⚠️ CERT DELIVERY IS inline_string:, NEVER filename:. pki/ exists on the HOST;
// it does not exist inside the reference CONTAINER, and this fixture implements
// no ReferenceLogMounter bind-mount.
func (*connErrDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":          refAdminPort,
		"ListenerPort":       refListenerPort,
		"BackendPort":        backendPorts[0],
		"ServerCertIndented": indentPEM(mustReadFixtureBytes("pki/server.pem"), 24),
		"ServerKeyIndented":  indentPEM(mustReadFixtureBytes("pki/server.key.pem"), 24),
		"CACertIndented":     indentPEM(mustReadFixtureBytes("pki/ca.pem"), 22),
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports, the loopback backend port, and the same three committed PEMs. The
// PEMs are byte-identical to the reference side's by construction.
func (*connErrDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":          subjAdminPort,
		"ListenerPort":       subjListenerPort,
		"BackendPort":        backendPorts[0],
		"ServerCertIndented": indentPEM(mustReadFixtureBytes("pki/server.pem"), 24),
		"ServerKeyIndented":  indentPEM(mustReadFixtureBytes("pki/server.key.pem"), 24),
		"CACertIndented":     indentPEM(mustReadFixtureBytes("pki/ca.pem"), 22),
	})
}

// DriveReference / DriveSubject: ONE DIRECTORY = ONE RUNNER BRANCH, so there is
// exactly one pair and all five arms sequence INSIDE it (Task 12).
func (d *connErrDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, "reference", addr)
}

func (d *connErrDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, "subject", addr)
}

// driveSide sequences all five arms against ONE side, in a FIXED order, and
// returns a NON-NIL EMPTY slice so CompareBytes has a defined result on both
// sides.
//
// ⚠️ It deliberately returns []byte{} rather than what it read: arm (iii)
// DIVERGES ON THE WIRE. MEASURED live against both sides at this fixture's
// build: the reference answers the garbage bytes with a fatal TLS alert record
// (the first six bytes read back are 15 03 01 00 02 02 — a 7-byte
// alert(21)/TLS1.0/len-2/fatal(2)/handshake_failure(40) record), while envoy-go
// answers with NOTHING and the read returns n=0 err=EOF. Returning read bytes
// would fail CompareBytes for a reason that is not this row's subject
// (reference_wire_format_both_sides_see_same_bytes). BOTH answers are valid
// rejections; the arm therefore inspects the read result ONLY for the one
// forbidden outcome, the payload coming back verbatim.
//
// ⚠️ ALL DISCRIMINATION LIVES IN AssertStats, NOT HERE. normalizeTLSErr collapses
// every failure to one constant (the 0110 shape) because BoringSSL and Go
// crypto/tls emit different client-visible alerts, so the drive bytes can only
// distinguish FAILED from SUCCEEDED. WHICH failure occurred is a STATS question.
//
// Every arm is evaluated independently and ALL violations are reported in ONE
// error (reference_fatalf_makes_assertions_unreachable): a t.Fatalf-shaped early
// return would make every later arm dead code.
func (d *connErrDriver) driveSide(ctx context.Context, side, addr string) ([]byte, error) {
	var probs []string
	record := func(arm string, err error) {
		log.Printf("0120 %s arm=%s err=%v", side, arm, normalizeTLSErr(err))
		if err != nil {
			probs = append(probs, fmt.Sprintf("%s: %v", arm, err))
		}
	}

	// (v) POSITIVE CONTROL, run FIRST so a broken upstream is caught before any
	// negative arm can be misread. ⚠️ It asserts the ECHO ROUND-TRIP, not just
	// the handshake: MEASURED, a cluster that cannot reach its backend lets the
	// client report HANDSHAKE=OK while the reference's ssl.handshake reads 0,
	// because tcp_proxy tears the downstream connection down before the
	// handshake is booked. A handshake-only pin would go RED with the config
	// looking fine and the client reporting success.
	if echo, err := d.mtlsEcho(ctx, side, addr, []byte(probePayload)); err != nil {
		record("valid", fmt.Errorf("handshake/roundtrip FAILED: %w", err))
	} else if !bytes.Equal(echo, []byte(probePayload)) {
		record("valid", fmt.Errorf("echo mismatch: got %q want %q", echo, probePayload))
	} else {
		record("valid", nil)
	}

	// (i) bad version — CLIENT-side only. MaxVersion TLS 1.1 against the config's
	// TLS 1.2 floor. ⚠️ NEVER lower the floor in YAML: the two pre-1.2
	// protocol-version enum values in a tls_params block BOOT-REJECT envoy-go
	// (internal/tls/params.go), so a config-side expression of this arm would
	// take the subject down at boot instead of producing a handshake failure.
	record("bad_version", d.expectHandshakeFailure(ctx, side, addr))
	// (ii) plaintext HTTP to the TLS port.
	record("plaintext", d.expectRawFailure(ctx, side, addr, []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")))
	// (iii) garbage bytes.
	record("garbage", d.expectRawFailure(ctx, side, addr, []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11}))
	// (iv) ⚠️ THE DISCRIMINATING NEGATIVE CONTROL. Connect and FIN with zero
	// bytes. The server sees a bare io.EOF, classifyHandshakeErr returns
	// outcomeOther, and the predicate's io.EOF term must SUPPRESS the Inc.
	// The reference books NOTHING here (MEASURED: connection_error +0). Without
	// this arm the fixture proves only that the counter CAN move, never that the
	// predicate DISCRIMINATES — and this arm is what exercises the io.EOF term.
	record("clean_fin", d.expectCleanFIN(ctx, side, addr))

	if len(probs) > 0 {
		return nil, fmt.Errorf("%s: %s", side, strings.Join(probs, "; "))
	}
	return []byte{}, nil
}

// --- arm helpers ---

// clientKeyPair loads the COMMITTED client leaf + key. It is a fresh load per
// call rather than cached state: the driver is stateless and both sides read
// the same two files, so the chain is byte-identical cross-side by construction.
func clientKeyPair() (stdtls.Certificate, error) {
	return stdtls.X509KeyPair(mustReadFixtureBytes("pki/client.pem"), mustReadFixtureBytes("pki/client.key.pem"))
}

// serverCAPool is the committed fixture CA, used to VERIFY the server leaf. The
// positive arm runs with InsecureSkipVerify: false against this pool — an arm
// that skipped verification could not tell a correct server cert from a wrong
// one.
func serverCAPool() (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(mustReadFixtureBytes("pki/ca.pem")) {
		return nil, errors.New("pki/ca.pem: no certificate appended")
	}
	return pool, nil
}

// baseTLSConfig builds the shared client config with the client chain
// FORCE-SENT via GetClientCertificate.
//
// ⚠️ THE FORCED SEND IS MANDATORY, NOT STYLE (reference_go_client_cert_withholding).
// Go's "polite" client (Certificates:) runs SupportsCertificate filtering against
// the server's advertised acceptable-CA list and SILENTLY sends an EMPTY chain
// when it does not match — which would turn a server-side measurement into a
// CLIENT-side one and make the arm vacuous while every byte comparison stays
// green. GetClientCertificate bypasses that filtering entirely.
//
// It also LOGS the certificate count actually handed to the stack, per arm and
// per side, so a silently-empty chain is visible in the run log rather than
// inferred. ⚠️ fixture.TB has no Logf — log.Printf is the recording channel.
func baseTLSConfig(side, arm string) (*stdtls.Config, error) {
	pool, err := serverCAPool()
	if err != nil {
		return nil, err
	}
	cert, err := clientKeyPair()
	if err != nil {
		return nil, fmt.Errorf("client keypair: %w", err)
	}
	cfg := &stdtls.Config{
		RootCAs:            pool,
		ServerName:         serverName,
		MinVersion:         stdtls.VersionTLS12,
		MaxVersion:         stdtls.VersionTLS13,
		InsecureSkipVerify: false, //nolint:gosec // explicit: this arm MUST verify
		GetClientCertificate: func(*stdtls.CertificateRequestInfo) (*stdtls.Certificate, error) {
			log.Printf("0120 %s arm=%s client_cert_chain_len=%d (FORCED SEND)", side, arm, len(cert.Certificate))
			return &cert, nil
		},
	}
	return cfg, nil
}

// mtlsEcho is arm (v): a full mTLS handshake followed by an application-level
// echo round-trip through the tcp_proxy to the TCP echo backend.
func (d *connErrDriver) mtlsEcho(ctx context.Context, side, addr string, payload []byte) ([]byte, error) {
	cfg, err := baseTLSConfig(side, "valid")
	if err != nil {
		return nil, err
	}
	raw, err := dialWithDeadline(ctx, addr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = raw.Close() }()

	conn := stdtls.Client(raw, cfg)
	defer func() { _ = conn.Close() }()
	if err := conn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("handshake: %w", err)
	}
	if _, err := conn.Write(payload); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	echo := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, echo); err != nil {
		return nil, fmt.Errorf("read echo: %w", err)
	}
	return echo, nil
}

// expectHandshakeFailure is arm (i): a TLS ClientHello whose MAXIMUM offered
// version is TLS 1.1, below the listener's TLS 1.2 floor.
//
// ⚠️ MinVersion is lowered TOO. Go's client refuses to build a config whose
// MaxVersion is below its MinVersion, and since Go 1.22 the DEFAULT client
// minimum is TLS 1.2 — leaving MinVersion unset would make the dial fail
// LOCALLY, sending nothing, and the server would observe a clean FIN instead of
// a version failure. That would silently DEGRADE arm (i) into a second copy of
// arm (iv) and book connection_error +0 (reference_vacuous_break_modes).
//
// The arm SUCCEEDS (returns nil) when the handshake FAILS. A handshake that
// SUCCEEDS is a violation and is reported.
func (d *connErrDriver) expectHandshakeFailure(ctx context.Context, side, addr string) error {
	cfg, err := baseTLSConfig(side, "bad_version")
	if err != nil {
		return err
	}
	cfg.MinVersion = stdtls.VersionTLS10
	cfg.MaxVersion = stdtls.VersionTLS11
	raw, err := dialWithDeadline(ctx, addr)
	if err != nil {
		return err
	}
	defer func() { _ = raw.Close() }()

	conn := stdtls.Client(raw, cfg)
	defer func() { _ = conn.Close() }()
	if err := conn.HandshakeContext(ctx); err != nil {
		return nil // expected
	}
	return errors.New("handshake SUCCEEDED at max version TLS 1.1 against a TLS 1.2 floor")
}

// expectRawFailure is arms (ii) and (iii): open a PLAIN TCP connection to the
// TLS port and write bytes that are not a valid TLS 1.2+ ClientHello.
//
// The arm SUCCEEDS (returns nil) when the connection produces no echo of the
// payload — i.e. the TLS terminator rejected it. An echo would mean the bytes
// were proxied straight through to the backend, which is a violation.
func (d *connErrDriver) expectRawFailure(ctx context.Context, side, addr string, payload []byte) error {
	log.Printf("0120 %s arm=raw client_cert_chain_len=0 (plain TCP, no TLS client)", side)
	raw, err := dialWithDeadline(ctx, addr)
	if err != nil {
		return err
	}
	defer func() { _ = raw.Close() }()

	if _, err := raw.Write(payload); err != nil {
		return nil // the server already reset/closed — the rejection we want
	}
	// Read until EOF/error or until the payload could have been echoed. The
	// reference answers with a 7-byte fatal TLS alert here and envoy-go answers
	// with nothing; BOTH are acceptable rejections, so the read result is only
	// inspected for the ONE forbidden outcome: our own payload coming back.
	got := make([]byte, len(payload))
	n, _ := io.ReadFull(raw, got)
	if n == len(payload) && bytes.Equal(got, payload) {
		return fmt.Errorf("payload was ECHOED back verbatim (%d bytes) — the port did not terminate TLS", n)
	}
	return nil
}

// expectCleanFIN is arm (iv), THE DISCRIMINATING NEGATIVE CONTROL: connect and
// close, writing ZERO bytes. The server's Accept succeeds, its handshake read
// returns a bare io.EOF, classifyHandshakeErr has no EOF branch so it returns
// outcomeOther, and the predicate's io.EOF term is what SUPPRESSES the Inc.
//
// This is the only arm that can tell a DISCRIMINATING predicate from a counter
// that merely CAN move.
func (d *connErrDriver) expectCleanFIN(ctx context.Context, side, addr string) error {
	log.Printf("0120 %s arm=clean_fin client_cert_chain_len=0 (plain TCP, zero bytes written)", side)
	raw, err := dialWithDeadline(ctx, addr)
	if err != nil {
		return err
	}
	// A CLEAN FIN, not a reset: Close() on a TCP conn with no unread data and no
	// SO_LINGER sends FIN. A reset would surface as ECONNRESET on the server and
	// exercise a DIFFERENT term of the predicate.
	if err := raw.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

// dialWithDeadline dials addr and installs the arm deadline, clamped by the
// context deadline when the runner's is tighter.
func dialWithDeadline(ctx context.Context, addr string) (net.Conn, error) {
	dialer := &net.Dialer{}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	deadline := time.Now().Add(armDeadline)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := raw.SetDeadline(deadline); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("set deadline: %w", err)
	}
	return raw, nil
}

// normalizeTLSErr collapses a handshake/round-trip error to a stable token for
// the LOG. It exists because BoringSSL and Go crypto/tls send DIFFERENT alerts
// for the same rejection, so no cross-side equality can ever be built on the
// error text. The token is diagnostic only; the returned error keeps its full
// text for the failure path.
func normalizeTLSErr(err error) string {
	if err == nil {
		return "none"
	}
	return "arm-failed"
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint for the
// runner's standard admin-diff probe step (the 0110/0118 shape).
func (*connErrDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertStats is the runner's step-10 stats leg (ADR-0062) and is where ALL of
// this fixture's discrimination lives. The drive bytes can only say FAILED vs
// SUCCEEDED (normalizeTLSErr collapses every failure to one token, because
// BoringSSL and Go crypto/tls send different alerts and no cross-side text
// equality can be built on them). WHICH failure occurred is a STATS question,
// and this is where it is asked.
//
// ⚠️ KEYED ON THE METRIC NAME, IGNORING THE LABEL VALUE. MEASURED live: the
// reference renders envoy_listener_ssl_connection_error{envoy_listener_address=
// "0.0.0.0_10126"} while the subject renders the same metric with
// envoy_listener_address="___12127" (envoy-go binds [::] and the subject's
// listener port is runner-allocated). Keying on the name resolves all three
// cross-side scope divergences at once — dotted address form, IPv6 bracket
// normalization, and stat_prefix — because the Prometheus NAME carries none of
// them. scrapeProm therefore strips the label set entirely.
//
// ⚠️ Every violation is reported with Errorf, never Fatalf: a Fatalf on the
// first side would make the second side's assertions DEAD CODE
// (reference_fatalf_makes_assertions_unreachable). The only Fatalf is the
// scrape itself, where there is nothing left to assert.
func (d *connErrDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	for _, side := range []struct{ name, addr string }{
		{"reference", refAdminAddr},
		{"subject", subjAdminAddr},
	} {
		got, err := scrapeProm(side.addr)
		if err != nil {
			t.Fatalf("%s: scrape: %v", side.name, err)
		}
		// POSITIVE HALF: the three protocol-error arms moved the counter, and
		// ONLY those three. An EXACT equality, not a floor: a floor could not
		// tell a discriminating predicate from one that also fires on the
		// clean-FIN arm (that would read 4).
		if v := got["envoy_listener_ssl_connection_error"]; v != wantConnectionError {
			t.Errorf("%s: ssl.connection_error = %d, want %d "+
				"(arms bad_version + plaintext + garbage each Inc; the clean-FIN "+
				"transport arm MUST NOT — that arm is what exercises the predicate's "+
				"io.EOF term)", side.name, v, wantConnectionError)
		}
		// POSITIVE HALF: exactly one successful handshake — the valid arm. This
		// pin is what catches a listener whose upstream is broken: a
		// handshake-only client check reports OK while this reads 0.
		if v := got["envoy_listener_ssl_handshake"]; v != wantHandshake {
			t.Errorf("%s: ssl.handshake = %d, want %d (only arm (v) completes a handshake)",
				side.name, v, wantHandshake)
		}
		// ⚠️ NEGATIVE HALF — assert WHICH DID NOT FIRE. A pin proving
		// connection_error moved says nothing about whether a CERTIFICATE counter
		// also moved. No arm presents a bad or missing client certificate (the
		// valid arm force-sends a trusted one and every other arm is either
		// pre-certificate or plain TCP), so both cert counters MUST stay 0 on
		// BOTH sides. Without this half, an implementation that booked every
		// failure under all three names would still pass the positive half.
		for _, n := range []string{
			"envoy_listener_ssl_fail_verify_error",
			"envoy_listener_ssl_fail_verify_no_cert",
		} {
			if v := got[n]; v != 0 {
				t.Errorf("%s: %s = %d, want 0 — no arm drives a certificate failure", side.name, n, v)
			}
		}
	}
}

// scrapeProm fetches /stats/prometheus and returns a map keyed by metric NAME
// with the label set stripped ENTIRELY. Stripping is REQUIRED here, not a
// convenience: the two sides render envoy_listener_address differently
// (MEASURED "0.0.0.0_10126" vs "___12127"), so any label-preserving key would
// be cross-side incomparable by construction.
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

// --- file / template helpers (the 0103/0108/0109/0110 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0120-tls-connection-error/driver/driver.go
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
