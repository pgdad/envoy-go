// Package driver is the differential fixture driver for
// 0111-tls-cvc-empty-dynamic-fallback: a CVC-primary, three-arm verdict at
// require_client_certificate=true where the SDS-delivered dynamic
// validation_context is served EMPTY (trusted_ca-absent, S1) and the trust anchor
// therefore FALLS BACK to the inline default_validation_context.trusted_ca (CA_A).
// A FORCED-SEND untrusted arm upper-bounds the fallback pool to CA_A.
package driver

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	stdtls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"github.com/pgdad/envoy-go/test/helpers/sdsserver"
)

const (
	fixtureName = "0111-tls-cvc-empty-dynamic-fallback"

	// In-container reference Envoy listener port. Convention "100NN" for
	// fixture "01NN"; 0109 took 10445 and 0110 took 10446, so 0111 takes 10447
	// (RE-DERIVED free: zero hits across test/).
	refListenerPort = 10447

	// secretName is the SDS resource name of the SERVED validation_context —
	// fixed identically in both config templates'
	// combined_validation_context.validation_context_sds_secret_config. A
	// fixture-distinct name keeps the two SDS receivers plainly separate in logs.
	// Here the server serves this name with an EMPTY validation_context
	// (trusted_ca-absent, S1) via sdsserver.WithEmptyValidationContext — the
	// reference ACKs it, merges away the empty half, and falls back to the inline
	// default_validation_context.trusted_ca (phase 68).
	secretName = "edf_validation_ca"

	// serverName is the TLS ServerName the driver dials with — must match the
	// generated server leaf's DNS SAN. Fixture-distinct from 0110's l_rccf.
	serverName = "l_edf.fixture.test"

	// probePayload is an accepting arm's application payload, echoed back
	// verbatim by the TCP echo backend through the tcp_proxy.
	probePayload = "phase68-edf-probe\n"

	// armDeadline bounds each arm's dial+handshake+write+read. Generous on
	// purpose: a too-tight bound would let a SLOW accept masquerade as a
	// reject on the negative arms.
	armDeadline = 10 * time.Second
)

// clientCertMode selects how the dial helper presents (or withholds) a client
// certificate.
type clientCertMode int

const (
	// sendForced installs GetClientCertificate so the arm's cert is written to
	// the wire REGARDLESS of the server's advertised acceptable-CA list
	// (reference_go_client_cert_withholding). RD3: at require=true the forced-send
	// is NOT the observable's discriminator (a polite dial yields the same
	// `untrusted=rejected`, and a permissive union pool advertises CA_B so the
	// polite client would send client_B too). It is retained so the untrusted arm
	// actually EXERCISES verify-and-reject against the fallback pool rather than
	// collapsing into a no-cert duplicate of the `none` arm, and to keep both sides
	// symmetric. Do NOT claim forced-send flips the observable here.
	sendForced clientCertMode = iota
	// sendNone presents NO client certificate — neither Certificates nor
	// GetClientCertificate is set. At require=true this is REJECTED with TLS alert
	// 116 (certificate_required): the `none` arm proves require=true is ENFORCED
	// against the FALLBACK anchor.
	sendNone
)

// wantObservable is the ONLY byte stream a correct implementation may emit —
// on BOTH sides. structuralCheck enforces it per side. Each arm is its own
// segment/line so the three propositions read unambiguously.
const wantObservable = "trusted=ok echo=" + probePayload + "untrusted=rejected\nnone=rejected\n"

func init() {
	fixture.RegisterFixture(fixtureName, &edfDriver{})
}

// edfDriver carries the per-driver lifecycle state — the in-memory PKI and
// TWO private SDS receivers, one per side
// (reference_periodic_sink_differential_two_receivers).
//
// The CA roles INVERT vs 0110 (where the SDS-served CA won and the inline default
// lost). Here the SDS half is served EMPTY (S1), so the inline default WINS via
// the phase-68 fallback:
//   - CA_A is the inline combined_validation_context.default_validation_context.
//     trusted_ca — the FALLBACK anchor that MUST WIN.
//   - CA_B is a FOREIGN CA (templated into no yaml); client_B chains to it and
//     MUST be rejected — this upper-bounds the fallback pool to CA_A only.
//
// The three arms prove, at require=true against the fallback anchor: (1) the
// fallback is LIVE and CA_A (trusted client_A accepted+echo), (2) the fallback
// pool is CA_A specifically (untrusted client_B rejected — a CA_A∪CA_B union
// would accept it), (3) require=true is ENFORCED against the fallback anchor
// (no-cert rejected, alert 116).
type edfDriver struct {
	once sync.Once

	caAInlinePEM  []byte // CA_A — inline default_validation_context.trusted_ca; the FALLBACK anchor that MUST win
	serverCertPEM []byte // injected inline_string into both yamls (signed by CA_A)
	serverKeyPEM  []byte
	clientA       stdtls.Certificate // chains to CA_A (inline default) -> MUST be accepted (via the fallback)
	clientB       stdtls.Certificate // chains to CA_B (foreign)        -> MUST be rejected (upper-bounds the pool to CA_A)
	serverCAPool  *x509.CertPool     // the driver's RootCAs (verifies the proxy's leaf, signed by CA_A)

	refSDSPort  int
	subjSDSPort int
	refSrv      *sdsserver.Server
	subjSrv     *sdsserver.Server
}

// ensure generates the whole PKI in memory, allocates the two receiver ports,
// and starts one SDS receiver per side — each serving an EMPTY validation_context
// (trusted_ca-absent, S1) under secretName. Idempotent (sync.Once).
//
// TWO receivers, not one: a shared receiver would let one side's fetch
// contaminate the other's view (reference_periodic_sink_differential_two_receivers).
func (d *edfDriver) ensure() {
	d.once.Do(func() {
		caA, caAKey := mustCA("envoy-go 0111 CA_A (inline default, fallback anchor)")
		caB, caBKey := mustCA("envoy-go 0111 CA_B (foreign)")

		d.caAInlinePEM = mustCertPEM(caA.Raw)

		// Server leaf: signed by CA_A (the inline default / fallback CA), ServerAuth
		// + the dialed DNS SAN. The driver's RootCAs trusts CA_A so ALL arms get past
		// server verification — the only thing under test is the proxy's verdict on
		// OUR client cert (or its absence).
		serverDER, serverKey := mustLeaf("envoy-go 0111 server", caA, caAKey,
			[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, []string{serverName})
		d.serverCertPEM = mustCertPEM(serverDER)
		d.serverKeyPEM = mustKeyPEM(serverKey)

		d.serverCAPool = x509.NewCertPool()
		if !d.serverCAPool.AppendCertsFromPEM(d.caAInlinePEM) {
			panic("driver: CA_A PEM: no certificates parsed")
		}

		// client_A chains to CA_A (the INLINE default) -> the proxy's FALLBACK pool
		// MUST accept it (the empty served dynamic VC merged away, so the inline
		// default is the live trust anchor).
		d.clientA = mustClientCert("envoy-go 0111 client_A", caA, caAKey)
		// client_B chains to CA_B (a FOREIGN CA, in no yaml) -> MUST be rejected. If
		// it is accepted, the fallback pool is broader than CA_A (a union / accept-all
		// pool) and the fixture's proposition is refuted.
		d.clientB = mustClientCert("envoy-go 0111 client_B", caB, caBKey)

		d.refSDSPort = mustAllocatePort()
		d.subjSDSPort = mustAllocatePort()

		var err error
		d.refSrv, err = sdsserver.NewAtAddr(fmt.Sprintf("0.0.0.0:%d", d.refSDSPort),
			sdsserver.WithEmptyValidationContext(secretName))
		if err != nil {
			panic(fmt.Sprintf("driver: start reference SDS receiver: %v", err))
		}
		d.subjSrv, err = sdsserver.NewAtAddr(fmt.Sprintf("0.0.0.0:%d", d.subjSDSPort),
			sdsserver.WithEmptyValidationContext(secretName))
		if err != nil {
			panic(fmt.Sprintf("driver: start subject SDS receiver: %v", err))
		}
	})
}

// --- in-memory PKI (the 0018 pki/gen.go crypto shape, no disk) ---

func pkiValidity() (notBefore, notAfter time.Time) {
	now := time.Now()
	return now.Add(-time.Hour), now.Add(24 * time.Hour)
}

// mustCA generates a self-signed ECDSA P-256 CA.
func mustCA(cn string) (*x509.Certificate, *ecdsa.PrivateKey) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("driver: generate CA key (%s): %v", cn, err))
	}
	notBefore, notAfter := pkiValidity()
	tmpl := &x509.Certificate{
		SerialNumber:          mustSerial(),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(fmt.Sprintf("driver: create CA cert (%s): %v", cn, err))
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic(fmt.Sprintf("driver: parse CA cert (%s): %v", cn, err))
	}
	return cert, key
}

// mustLeaf generates an ECDSA P-256 leaf signed by parent.
func mustLeaf(cn string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey,
	eku []x509.ExtKeyUsage, dnsNames []string,
) ([]byte, *ecdsa.PrivateKey) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("driver: generate leaf key (%s): %v", cn, err))
	}
	notBefore, notAfter := pkiValidity()
	tmpl := &x509.Certificate{
		SerialNumber: mustSerial(),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  eku,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		panic(fmt.Sprintf("driver: create leaf cert (%s): %v", cn, err))
	}
	return der, key
}

// mustClientCert builds a ready-to-present client keypair (ExtKeyUsage
// ClientAuth) chaining to the supplied CA.
func mustClientCert(cn string, ca *x509.Certificate, caKey *ecdsa.PrivateKey) stdtls.Certificate {
	der, key := mustLeaf(cn, ca, caKey, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, nil)
	// The chain the proxy verifies is leaf + issuing CA; the fallback trust store is
	// the inline default CA_A (the served dynamic VC is EMPTY and merges away).
	cert, err := stdtls.X509KeyPair(
		append(mustCertPEM(der), mustCertPEM(ca.Raw)...),
		mustKeyPEM(key),
	)
	if err != nil {
		panic(fmt.Sprintf("driver: build client keypair (%s): %v", cn, err))
	}
	return cert
}

func mustSerial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(fmt.Sprintf("driver: generate serial: %v", err))
	}
	return n
}

func mustCertPEM(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func mustKeyPEM(key *ecdsa.PrivateKey) []byte {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		panic(fmt.Sprintf("driver: marshal PKCS8 key: %v", err))
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// mustAllocatePort reserves a free TCP port via Listen+Close (the 0103/0089 idiom).
func mustAllocatePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate SDS port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// --- fixture.Driver (required) ---

func (*edfDriver) BackendCount() int           { return 1 } // the accepting arm ECHOES through it
func (*edfDriver) SubjectListenerName() string { return "l_edf" }
func (*edfDriver) ReferenceListenerPort() int  { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// reference-side SDS receiver port + the runner-allocated backend port, and
// injects the STATIC server cert/key AND the inline CA_A default (no file, no
// mount).
func (d *edfDriver) ReferenceBootstrap(backendPorts []int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"SDSHost":     "host.docker.internal",
		"SDSPort":     d.refSDSPort,
		"BackendPort": backendPorts[0],
		"ServerCert":  indentPEM(d.serverCertPEM, 24),
		"ServerKey":   indentPEM(d.serverKeyPEM, 24),
		"InlineCA":    indentPEM(d.caAInlinePEM, 26),
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports + the loopback backend port + the subject-side SDS receiver port + the
// inline CA_A default.
func (d *edfDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"ListenPort":  subjListenerPort,
		"BackendPort": backendPorts[0],
		"SDSPort":     d.subjSDSPort,
		"ServerCert":  indentPEM(d.serverCertPEM, 24),
		"ServerKey":   indentPEM(d.serverKeyPEM, 24),
		"InlineCA":    indentPEM(d.caAInlinePEM, 26),
	})
}

func (d *edfDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, "reference", addr)
}

// DriveSubject drives the subject and then hard-stops both receivers — both
// proxies hold their long-lived StreamSecrets stream open until this point, so
// GracefulStop would block (the 0103/0108/0109/0110 teardown).
func (d *edfDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	out, err := d.driveSide(ctx, "subject", addr)
	d.closeServers()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// closeServers hard-stops both receivers (idempotent via sdsserver.Server's own
// sync.Once).
func (d *edfDriver) closeServers() {
	if d.refSrv != nil {
		d.refSrv.Close()
	}
	if d.subjSrv != nil {
		d.subjSrv.Close()
	}
}

// serverForSide returns the SDS receiver that THIS side's proxy dials.
func (d *edfDriver) serverForSide(side string) *sdsserver.Server {
	if side == "subject" {
		return d.subjSrv
	}
	return d.refSrv
}

// driveSide returns the NORMALIZED three-arm verdict. Both sides must emit
// byte-identical output.
//
// The three-arm CONTRAST is the whole proof AT require=true against the FALLBACK
// anchor (the served dynamic VC is EMPTY, so the inline default CA_A is live):
//   - trusted:   client_A chains to the inline default CA_A (fallback) -> ACCEPTED (+echo)
//   - untrusted: client_B chains to a FOREIGN CA_B                     -> REJECTED
//   - none:      NO client cert is presented                          -> REJECTED (alert 116)
//
// The untrusted arm UPPER-BOUNDS the fallback pool to CA_A: a CA_A∪CA_B union or
// an accept-all pool would ACCEPT client_B. The none arm proves require=true is
// ENFORCED against the fallback anchor (a no-cert connection draws alert 116, not
// an accept).
//
// The failure TEXT is deliberately NOT part of the observable (inherits PLAN-65
// C3): the reference (BoringSSL) sends the alert `unknown ca` / `certificate
// required` while envoy-go (Go crypto/tls) sends `bad certificate` /
// `certificate required`. Asserting it would fail cross-side
// (reference_differential_reference_parses_full_message). Only the normalized
// verdict is recorded.
//
// ⚠️ The structural check at the end is LOAD-BEARING, not decoration. CompareBytes
// alone proves only that the two sides AGREE. A break that changes the fallback CA
// (or re-signs client_B with it) changes BOTH sides identically, so the streams
// still compare EQUAL and a pure-CompareBytes fixture would PASS
// (reference_vacuous_break_receiver_normalizes). structuralCheck asserts each
// side's OWN bytes against the one correct shape, so such a break fails loudly.
func (d *edfDriver) driveSide(ctx context.Context, side, addr string) ([]byte, error) {
	// Served-this-arm precondition (SPEC §8 stale-server trap). THIS run's
	// receiver for THIS side must have recorded at least one StreamSecrets request
	// from the proxy's boot-time initial fetch. Zero requests means the CVC's inner
	// secret was NOT fetched from this server — the verdict below would be
	// meaningless (a stale/previous-arm server may have served it).
	if n := len(d.serverForSide(side).Requests()); n == 0 {
		return nil, fmt.Errorf("%s: served-this-arm precondition FAILED: this run's SDS receiver recorded ZERO "+
			"StreamSecrets requests — the proxy never fetched the CVC's inner secret from THIS server "+
			"(a stale/previous-arm server may have served it; SPEC §8)", side)
	}

	var out bytes.Buffer

	// Arm 1 (trusted, positive): client_A chains to the inline default CA_A (the
	// live FALLBACK anchor), FORCED-SEND -> handshake OK -> echo.
	echo, err := d.mtlsEcho(ctx, addr, sendForced, d.clientA, []byte(probePayload))
	if err != nil {
		fmt.Fprintf(&out, "trusted=REJECTED err=%s\n", normalizeTLSErr(err))
	} else {
		fmt.Fprintf(&out, "trusted=ok echo=%s", echo)
	}

	// Arm 2 (untrusted, negative): client_B chains to a FOREIGN CA_B, FORCED-SEND
	// (RD3 — retained so the arm EXERCISES verify-and-reject and stays symmetric
	// cross-side; NOT because it flips the require=true observable). The server
	// verifies against the fallback pool {CA_A} and REJECTS. This UPPER-BOUNDS the
	// fallback pool: a CA_A∪CA_B union would ACCEPT client_B.
	if _, err := d.mtlsEcho(ctx, addr, sendForced, d.clientB, []byte(probePayload)); err != nil {
		out.WriteString("untrusted=rejected\n")
	} else {
		// The fallback trust store accepted a cert chaining to the FOREIGN CA_B -> the
		// fallback pool is broader than CA_A (a union / accept-all pool).
		out.WriteString("untrusted=ACCEPTED\n")
	}

	// Arm 3 (none): present NO client cert. At require=true this is REJECTED with
	// TLS alert 116 (certificate_required) — proving require=true is ENFORCED
	// against the FALLBACK anchor.
	if _, err := d.mtlsEcho(ctx, addr, sendNone, stdtls.Certificate{}, []byte(probePayload)); err != nil {
		out.WriteString("none=rejected\n")
	} else {
		// A no-cert connection was ACCEPTED -> require=true was NOT enforced against
		// the fallback anchor (the fallback silently degraded to verify-if-presented
		// or no-client-cert).
		out.WriteString("none=ACCEPTED\n")
	}

	if err := structuralCheck(side, out.Bytes()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// structuralCheck asserts THIS side's own bytes against the one correct shape.
// It is what makes a SYMMETRIC break fail: the runner turns the returned error
// into t.Fatalf("<side> drive: ..."), naming the side and the violated arm.
//
// All three arms are evaluated independently and ALL violations are reported in
// one error (reference_fatalf_makes_assertions_unreachable — the first violation
// must not hide the others).
func structuralCheck(side string, out []byte) error {
	if bytes.Equal(out, []byte(wantObservable)) {
		return nil
	}
	s := string(out)
	var probs []string
	if !strings.HasPrefix(s, "trusted=ok echo="+probePayload) {
		probs = append(probs, "trusted arm: client_A chains to the inline default CA_A (the fallback anchor) and MUST be "+
			"accepted with an echo (the empty served dynamic VC did not fall back to the inline default, or the echo path is broken)")
	}
	if !strings.Contains(s, "\nuntrusted=rejected\n") {
		probs = append(probs, "untrusted arm: client_B (FORCED-SEND) chains to a FOREIGN CA_B and MUST be rejected "+
			"(the fallback pool is CA_A ONLY; if client_B is accepted the pool is broader than CA_A — a union / accept-all — "+
			"and the proposition is refuted)")
	}
	if !strings.HasSuffix(s, "none=rejected\n") {
		probs = append(probs, "none arm: NO client cert MUST be REJECTED at require=true (alert 116, certificate_required); "+
			"an accept here means require=true was NOT enforced against the fallback anchor")
	}
	if len(probs) == 0 {
		probs = append(probs, "stream does not match the expected shape")
	}
	return fmt.Errorf("%s: require=true empty-dynamic-fallback structural check FAILED:\n  %s\n  want: %q\n  got:  %q",
		side, strings.Join(probs, "\n  "), wantObservable, s)
}

// mtlsEcho dials addr, presents a client cert per mode, and drives the FULL
// round trip: handshake -> write payload -> read len(payload) echoed bytes. Any
// stage failing is a REJECT, which is the version-independent normalization:
// under TLS 1.2 the server's rejection surfaces at the handshake, under TLS 1.3
// it surfaces at the first read.
//
// clientCertMode: sendForced installs GetClientCertificate so the cert transmits
// regardless of the server's advertised acceptable-CA list
// (reference_go_client_cert_withholding); sendNone presents nothing (the require=true
// no-cert arm, rejected with alert 116).
//
// The dial is inlined deliberately: neither helpers.TLSServedLeaf nor
// helpers.TLSRoundTrip can present a CLIENT cert. This keeps test/helpers
// untouched (the 0045 tlsDial shape + the 0018 scenario-6 tls.Config shape).
func (d *edfDriver) mtlsEcho(ctx context.Context, addr string, mode clientCertMode, armCert stdtls.Certificate, payload []byte) ([]byte, error) {
	dialer := &net.Dialer{}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = raw.Close() }()

	deadline := time.Now().Add(armDeadline)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := raw.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}

	cfg := &stdtls.Config{
		RootCAs:    d.serverCAPool,
		ServerName: serverName,
		MinVersion: stdtls.VersionTLS12,
		MaxVersion: stdtls.VersionTLS13,
	}
	switch mode {
	case sendForced:
		// forced-send: transmits the cert BYPASSING SupportsCertificate filtering.
		cert := armCert
		cfg.GetClientCertificate = func(*stdtls.CertificateRequestInfo) (*stdtls.Certificate, error) {
			return &cert, nil
		}
	case sendNone:
		// neither Certificates nor GetClientCertificate set -> no cert on the wire.
	}

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

// normalizeTLSErr collapses a handshake/round-trip error to a stable token. It
// exists ONLY for the diagnostic path on the ACCEPTING arm (a failure that must
// never happen); the negative arms never record error text, because the reference
// (BoringSSL) and envoy-go (Go crypto/tls) send different alerts and cross-side
// text equality is impossible (PLAN-65 C3).
func normalizeTLSErr(err error) string {
	if err == nil {
		return "none"
	}
	return "handshake-or-roundtrip-failed"
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint for the
// runner's standard admin-diff probe step.
func (*edfDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- file / template helpers (the 0103/0108/0109/0110 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0111-tls-cvc-empty-dynamic-fallback/driver/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

func mustReadFixtureFile(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return string(b)
}

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

// Compile-time interface assertion.
var _ fixture.Driver = (*edfDriver)(nil)
