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
	fixtureName = "0109-xds-sds-combined-validation-context"

	// In-container reference Envoy listener port. Convention "100NN" for
	// fixture "00NN"; 0103 took 10443 and 0108 took 10444, so 0109 takes 10445
	// (RE-DERIVED free: zero hits across test/).
	refListenerPort = 10445

	// secretName is the SDS resource name of the SERVED validation_context —
	// fixed identically in both config templates'
	// combined_validation_context.validation_context_sds_secret_config. The
	// served wire is byte-identical to 0108's: the management server cannot tell
	// a CVC client from a plain-SDS client (SPEC §8).
	secretName = "validation_ca"

	// serverName is the TLS ServerName the driver dials with — must match the
	// generated server leaf's DNS SAN.
	serverName = "l_sds_cvc.fixture.test"

	// probePayload is the good arm's application payload, echoed back verbatim
	// by the TCP echo backend through the tcp_proxy.
	probePayload = "phase66-cvc-probe\n"

	// armDeadline bounds each arm's dial+handshake+write+read. Generous on
	// purpose: a too-tight bound would let a SLOW accept masquerade as a
	// reject on the negative arm.
	armDeadline = 10 * time.Second
)

// wantObservable is the ONLY byte stream a correct implementation may emit —
// on BOTH sides. structuralCheck enforces it per side.
const wantObservable = "good=ok echo=" + probePayload + "bad=rejected\n"

func init() {
	fixture.RegisterFixture(fixtureName, &sdsCVCDriver{})
}

// sdsCVCDriver carries the per-driver lifecycle state — the in-memory PKI and
// TWO private SDS receivers, one per side
// (reference_periodic_sink_differential_two_receivers).
//
// The CA re-labeling (PLAN-66 D4) is the semantic core of this fixture vs 0108:
//   - CA_X is served over SDS as the validation_context — the anchor that MUST WIN.
//   - CA_Y is the inline combined_validation_context.default_validation_context.
//     trusted_ca — a CONFIGURED competitor that MUST LOSE (this is what proves
//     REPLACE, not union: 0108's CA_unserved was never delivered at all).
type sdsCVCDriver struct {
	once sync.Once

	caXServedPEM  []byte // CA_X — served over SDS as trusted_ca; the anchor that MUST win
	caYInlinePEM  []byte // CA_Y — inline default_validation_context.trusted_ca; MUST lose
	serverCertPEM []byte // injected inline_string into both yamls (signed by CA_X)
	serverKeyPEM  []byte
	clientX       stdtls.Certificate // chains to CA_X (served)  -> MUST be accepted
	clientY       stdtls.Certificate // chains to CA_Y (inline)  -> MUST be rejected (proves REPLACE)
	serverCAPool  *x509.CertPool     // the driver's RootCAs (verifies the proxy's leaf)

	refSDSPort  int
	subjSDSPort int
	refSrv      *sdsserver.Server
	subjSrv     *sdsserver.Server
}

// ensure generates the whole PKI in memory, allocates the two receiver ports,
// and starts one SDS receiver per side — each serving the SAME caXServedPEM as
// the validation_context under secretName. Idempotent (sync.Once).
//
// TWO receivers, not one: a shared receiver would let one side's fetch
// contaminate the other's view (reference_periodic_sink_differential_two_receivers).
func (d *sdsCVCDriver) ensure() {
	d.once.Do(func() {
		caX, caXKey := mustCA("envoy-go 0109 CA_X (served over SDS)")
		caY, caYKey := mustCA("envoy-go 0109 CA_Y (inline default)")

		d.caXServedPEM = mustCertPEM(caX.Raw)
		d.caYInlinePEM = mustCertPEM(caY.Raw)

		// Server leaf: signed by CA_X (the SERVED CA), ServerAuth + the dialed DNS
		// SAN. The driver's RootCAs trusts CA_X so BOTH arms get past server
		// verification — the only thing under test is the proxy's verdict on OUR
		// client cert.
		serverDER, serverKey := mustLeaf("envoy-go 0109 server", caX, caXKey,
			[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, []string{serverName})
		d.serverCertPEM = mustCertPEM(serverDER)
		d.serverKeyPEM = mustKeyPEM(serverKey)

		d.serverCAPool = x509.NewCertPool()
		if !d.serverCAPool.AppendCertsFromPEM(d.caXServedPEM) {
			panic("driver: CA_X PEM: no certificates parsed")
		}

		// client_X chains to CA_X (SERVED) -> the proxy's SDS-delivered ClientCAs
		// MUST accept it.
		d.clientX = mustClientCert("envoy-go 0109 client_X", caX, caXKey)
		// client_Y chains to CA_Y (the INLINE default_validation_context.trusted_ca)
		// -> MUST be rejected. This is the load-bearing arm: if it is accepted, the
		// inline default UNIONED with the served pool (Design C) instead of being
		// REPLACED by it, and the fixture's headline proposition is refuted.
		d.clientY = mustClientCert("envoy-go 0109 client_Y", caY, caYKey)

		d.refSDSPort = mustAllocatePort()
		d.subjSDSPort = mustAllocatePort()

		var err error
		d.refSrv, err = sdsserver.NewAtAddr(fmt.Sprintf("0.0.0.0:%d", d.refSDSPort),
			sdsserver.WithValidationContext(secretName, d.caXServedPEM))
		if err != nil {
			panic(fmt.Sprintf("driver: start reference SDS receiver: %v", err))
		}
		d.subjSrv, err = sdsserver.NewAtAddr(fmt.Sprintf("0.0.0.0:%d", d.subjSDSPort),
			sdsserver.WithValidationContext(secretName, d.caXServedPEM))
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
	// The chain the proxy verifies is leaf + issuing CA; both proxies build their
	// trust store from the SDS-delivered trusted_ca only (never the inline default).
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

func (*sdsCVCDriver) BackendCount() int           { return 1 } // the good arm ECHOES through it
func (*sdsCVCDriver) SubjectListenerName() string { return "l_sds_cvc" }
func (*sdsCVCDriver) ReferenceListenerPort() int  { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// reference-side SDS receiver port + the runner-allocated backend port, and
// injects the STATIC server cert/key AND the inline CA_Y default (PLAN-66 D2 —
// no file, no mount).
func (d *sdsCVCDriver) ReferenceBootstrap(backendPorts []int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"SDSHost":     "host.docker.internal",
		"SDSPort":     d.refSDSPort,
		"BackendPort": backendPorts[0],
		"ServerCert":  indentPEM(d.serverCertPEM, 24),
		"ServerKey":   indentPEM(d.serverKeyPEM, 24),
		"InlineCA":    indentPEM(d.caYInlinePEM, 26),
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports + the loopback backend port + the subject-side SDS receiver port + the
// inline CA_Y default.
func (d *sdsCVCDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"ListenPort":  subjListenerPort,
		"BackendPort": backendPorts[0],
		"SDSPort":     d.subjSDSPort,
		"ServerCert":  indentPEM(d.serverCertPEM, 24),
		"ServerKey":   indentPEM(d.serverKeyPEM, 24),
		"InlineCA":    indentPEM(d.caYInlinePEM, 26),
	})
}

func (d *sdsCVCDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, "reference", addr)
}

// DriveSubject drives the subject and then hard-stops both receivers — both
// proxies hold their long-lived StreamSecrets stream open until this point, so
// GracefulStop would block (the 0103/0108 teardown).
func (d *sdsCVCDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	out, err := d.driveSide(ctx, "subject", addr)
	d.closeServers()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// closeServers hard-stops both receivers (idempotent via sdsserver.Server's own
// sync.Once).
func (d *sdsCVCDriver) closeServers() {
	if d.refSrv != nil {
		d.refSrv.Close()
	}
	if d.subjSrv != nil {
		d.subjSrv.Close()
	}
}

// serverForSide returns the SDS receiver that THIS side's proxy dials.
func (d *sdsCVCDriver) serverForSide(side string) *sdsserver.Server {
	if side == "subject" {
		return d.subjSrv
	}
	return d.refSrv
}

// driveSide returns the NORMALIZED two-arm verdict. Both sides must emit
// byte-identical output.
//
// The good/bad CONTRAST is the whole proof: a client cert chaining to the
// SDS-SERVED CA (CA_X) is ACCEPTED; one chaining to the INLINE-CONFIGURED CA
// (CA_Y, the combined_validation_context.default_validation_context.trusted_ca)
// is REJECTED. That is what makes this fixture non-vacuous AND load-bearing for
// the design: an accept-all trust store emits "bad=ACCEPTED", a UNION of the two
// pools (Design C) ALSO emits "bad=ACCEPTED" (client_Y chains to CA_Y, which the
// union trusts), and only a REPLACE — the served pool winning outright — emits
// "bad=rejected".
//
// The failure TEXT is deliberately NOT part of the observable (PLAN-66, inherits
// PLAN-65 C3): the reference (BoringSSL) sends the alert `unknown ca` while
// envoy-go (Go crypto/tls) sends `bad certificate`. Asserting it would fail
// cross-side 100% of the time. Only the normalized verdict is recorded.
//
// ⚠️ The structural check at the end is LOAD-BEARING, not decoration. CompareBytes
// alone proves only that the two sides AGREE. A break that changes the served CA
// (or re-signs client_Y with it) changes BOTH sides identically, so the streams
// still compare EQUAL and a pure-CompareBytes fixture would PASS
// (reference_vacuous_break_receiver_normalizes). structuralCheck asserts each
// side's OWN bytes against the one correct shape, so such a break fails loudly.
func (d *sdsCVCDriver) driveSide(ctx context.Context, side, addr string) ([]byte, error) {
	// Served-this-arm precondition (SPEC §8 stale-server trap). A stale SDS server
	// once silently served the PREVIOUS arm's config and nearly produced a false
	// divergence. Per-side servers on separately-allocated ports (above) make that
	// structurally impossible here, but we assert it directly rather than trust the
	// structure: THIS run's receiver for THIS side must have recorded at least one
	// StreamSecrets request from the proxy's boot-time initial fetch. Zero requests
	// means the CVC's inner secret was NOT fetched from this server — the verdict
	// below would be meaningless. 0108 lacks this assert; it is ADDED here (0108
	// itself stays untouched).
	if n := len(d.serverForSide(side).Requests()); n == 0 {
		return nil, fmt.Errorf("%s: served-this-arm precondition FAILED: this run's SDS receiver recorded ZERO "+
			"StreamSecrets requests — the proxy never fetched the CVC's inner secret from THIS server "+
			"(a stale/previous-arm server may have served it; SPEC §8)", side)
	}

	var out bytes.Buffer

	// Arm 1 (positive): client_X chains to CA_X (SERVED) -> handshake OK -> echo.
	echo, err := d.mtlsEcho(ctx, addr, d.clientX, []byte(probePayload))
	if err != nil {
		fmt.Fprintf(&out, "good=REJECTED err=%s\n", normalizeTLSErr(err))
	} else {
		fmt.Fprintf(&out, "good=ok echo=%s", echo)
	}

	// Arm 2 (negative): client_Y chains to CA_Y (the INLINE default) -> the round
	// trip MUST fail (at the handshake under TLS 1.2, or at the first read under
	// TLS 1.3). If it succeeds, the served pool did NOT replace the inline default.
	if _, err := d.mtlsEcho(ctx, addr, d.clientY, []byte(probePayload)); err != nil {
		out.WriteString("bad=rejected\n")
	} else {
		// The trust store accepted a cert chaining to the INLINE default CA -> the
		// served pool UNIONED with the inline default rather than REPLACING it.
		out.WriteString("bad=ACCEPTED\n")
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
// Both arms are evaluated independently and ALL violations are reported in one
// error (reference_fatalf_makes_assertions_unreachable — the first violation
// must not hide the second).
func structuralCheck(side string, out []byte) error {
	if bytes.Equal(out, []byte(wantObservable)) {
		return nil
	}
	s := string(out)
	var probs []string
	if !strings.HasPrefix(s, "good=ok echo="+probePayload) {
		probs = append(probs, "positive arm: client_X chains to the SDS-SERVED CA (CA_X) and MUST be accepted "+
			"(the served validation_context is not the trust anchor, or the echo path is broken)")
	}
	if !strings.HasSuffix(s, "bad=rejected\n") {
		probs = append(probs, "negative arm: client_Y chains to the INLINE default CA (CA_Y) and MUST be rejected "+
			"(the SDS-served pool REPLACES the inline default; if client_Y is accepted the pools UNIONED — Design C — "+
			"and the headline proposition is refuted)")
	}
	if len(probs) == 0 {
		probs = append(probs, "stream does not match the expected shape")
	}
	return fmt.Errorf("%s: SDS combined_validation_context structural check FAILED:\n  %s\n  want: %q\n  got:  %q",
		side, strings.Join(probs, "\n  "), wantObservable, s)
}

// mtlsEcho dials addr, presents clientCert, and drives the FULL round trip:
// handshake -> write payload -> read len(payload) echoed bytes. Any stage
// failing is a REJECT, which is the version-independent normalization: under
// TLS 1.2 the server's rejection surfaces at the handshake, under TLS 1.3 it
// surfaces at the first read.
//
// The dial is inlined deliberately: neither helpers.TLSServedLeaf nor
// helpers.TLSRoundTrip can present a CLIENT cert. This keeps test/helpers
// untouched (the 0045 tlsDial shape + the 0018 scenario-6 tls.Config shape).
func (d *sdsCVCDriver) mtlsEcho(ctx context.Context, addr string, clientCert stdtls.Certificate, payload []byte) ([]byte, error) {
	dialer := &net.Dialer{}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = raw.Close() }()

	deadline := time.Now().Add(armDeadline)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := raw.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}

	conn := stdtls.Client(raw, &stdtls.Config{
		Certificates: []stdtls.Certificate{clientCert},
		RootCAs:      d.serverCAPool,
		ServerName:   serverName,
		MinVersion:   stdtls.VersionTLS12,
		MaxVersion:   stdtls.VersionTLS13,
	})
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
// exists ONLY for the good=REJECTED diagnostic path (a failure that must never
// happen); the negative arm never records error text, because the reference
// (BoringSSL, `unknown ca`) and envoy-go (Go crypto/tls, `bad certificate`) send
// different alerts and cross-side text equality is impossible (PLAN-65 C3).
func normalizeTLSErr(err error) string {
	if err == nil {
		return "none"
	}
	return "handshake-or-roundtrip-failed"
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint for the
// runner's standard admin-diff probe step.
func (*sdsCVCDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- file / template helpers (the 0103/0108 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0109-xds-sds-combined-validation-context/driver/driver.go
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
var _ fixture.Driver = (*sdsCVCDriver)(nil)
