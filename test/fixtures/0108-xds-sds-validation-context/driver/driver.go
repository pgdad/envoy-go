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
	fixtureName = "0108-xds-sds-validation-context"

	// In-container reference Envoy listener port. Convention "100NN" for
	// fixture "00NN"; 0103 took 10443 for its single TLS listener, so 0108
	// takes 10444 (RE-DERIVED free: zero hits across test/).
	refListenerPort = 10444

	// secretName is the SDS resource name of the SERVED validation_context —
	// fixed identically in both config templates'
	// validation_context_sds_secret_config.
	secretName = "validation_ca"

	// serverName is the TLS ServerName the driver dials with — must match the
	// generated server leaf's DNS SAN.
	serverName = "l_sds_mtls.fixture.test"

	// probePayload is the good arm's application payload, echoed back verbatim
	// by the TCP echo backend through the tcp_proxy.
	probePayload = "phase65-mtls-probe\n"

	// armDeadline bounds each arm's dial+handshake+write+read. Generous on
	// purpose: a too-tight bound would let a SLOW accept masquerade as a
	// reject on the negative arm. Step-5 break (2) is what proves it does not
	// (signing client_bad with the SERVED CA must flip the arm to ACCEPTED).
	armDeadline = 10 * time.Second
)

// wantObservable is the ONLY byte stream a correct implementation may emit —
// on BOTH sides. structuralCheck enforces it per side.
const wantObservable = "good=ok echo=" + probePayload + "bad=rejected\n"

func init() {
	fixture.RegisterFixture(fixtureName, &sdsVCDriver{})
}

// sdsVCDriver carries the per-driver lifecycle state — the in-memory PKI
// (PLAN-65 D2: five artifacts, nothing on disk) and TWO private SDS receivers,
// one per side (reference_periodic_sink_differential_two_receivers).
type sdsVCDriver struct {
	once sync.Once

	caServedPEM   []byte // served over SDS as the trusted_ca — the anchor under test
	caUnservedPEM []byte // NEVER served; client_bad chains to it
	serverCertPEM []byte // injected inline_string into both yamls
	serverKeyPEM  []byte
	clientGood    stdtls.Certificate // chains to caServed   -> MUST be accepted
	clientBad     stdtls.Certificate // chains to caUnserved -> MUST be rejected
	serverCAPool  *x509.CertPool     // the driver's RootCAs (verifies the proxy's leaf)

	refSDSPort  int
	subjSDSPort int
	refSrv      *sdsserver.Server
	subjSrv     *sdsserver.Server
}

// ensure generates the whole PKI in memory, allocates the two receiver ports,
// and starts one SDS receiver per side — each serving the SAME caServedPEM as
// the validation_context under secretName. Idempotent (sync.Once), safe from
// both ReferenceBootstrap and SubjectConfig in either order; the 0103 pattern
// (driver/driver.go:63-87). Both servers are live before either proxy opens
// its StreamSecrets stream.
//
// TWO receivers, not one: a shared receiver would let one side's fetch
// contaminate the other's view (reference_periodic_sink_differential_two_receivers).
func (d *sdsVCDriver) ensure() {
	d.once.Do(func() {
		caServed, caServedKey := mustCA("envoy-go 0108 SERVED CA")
		caUnserved, caUnservedKey := mustCA("envoy-go 0108 UNSERVED CA")

		d.caServedPEM = mustCertPEM(caServed.Raw)
		d.caUnservedPEM = mustCertPEM(caUnserved.Raw)

		// Server leaf: signed by the SERVED CA, ServerAuth + the dialed DNS SAN.
		// The driver's RootCAs trusts the SERVED CA so BOTH arms get past server
		// verification — the only thing under test is the proxy's verdict on OUR
		// client cert.
		serverDER, serverKey := mustLeaf("envoy-go 0108 server", caServed, caServedKey,
			[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, []string{serverName})
		d.serverCertPEM = mustCertPEM(serverDER)
		d.serverKeyPEM = mustKeyPEM(serverKey)

		d.serverCAPool = x509.NewCertPool()
		if !d.serverCAPool.AppendCertsFromPEM(d.caServedPEM) {
			panic("driver: served CA PEM: no certificates parsed")
		}

		// client_good chains to the SERVED CA -> the proxy's SDS-delivered
		// ClientCAs MUST accept it.
		d.clientGood = mustClientCert("envoy-go 0108 client good", caServed, caServedKey)
		// client_bad chains to a CA that is NEVER served -> MUST be rejected.
		// If it is accepted, the trust store is an accept-all and the whole
		// fixture is vacuous — that is exactly what the bad arm catches.
		d.clientBad = mustClientCert("envoy-go 0108 client bad", caUnserved, caUnservedKey)

		d.refSDSPort = mustAllocatePort()
		d.subjSDSPort = mustAllocatePort()

		var err error
		d.refSrv, err = sdsserver.NewAtAddr(fmt.Sprintf("0.0.0.0:%d", d.refSDSPort),
			sdsserver.WithValidationContext(secretName, d.caServedPEM))
		if err != nil {
			panic(fmt.Sprintf("driver: start reference SDS receiver: %v", err))
		}
		d.subjSrv, err = sdsserver.NewAtAddr(fmt.Sprintf("0.0.0.0:%d", d.subjSDSPort),
			sdsserver.WithValidationContext(secretName, d.caServedPEM))
		if err != nil {
			panic(fmt.Sprintf("driver: start subject SDS receiver: %v", err))
		}
	})
}

// --- in-memory PKI (PLAN-65 D2 — the 0018 pki/gen.go crypto shape, no disk) ---

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
	// The chain the proxy verifies is leaf + issuing CA; both proxies build
	// their trust store from the SDS-delivered trusted_ca only.
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

func (*sdsVCDriver) BackendCount() int           { return 1 } // the good arm ECHOES through it
func (*sdsVCDriver) SubjectListenerName() string { return "l_sds_mtls" }
func (*sdsVCDriver) ReferenceListenerPort() int  { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// reference-side SDS receiver port + the runner-allocated backend port, and
// injects the STATIC server cert/key inline (PLAN-65 D2 — no file, no mount).
func (d *sdsVCDriver) ReferenceBootstrap(backendPorts []int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"SDSHost":     "host.docker.internal",
		"SDSPort":     d.refSDSPort,
		"BackendPort": backendPorts[0],
		"ServerCert":  indentPEM(d.serverCertPEM, 24),
		"ServerKey":   indentPEM(d.serverKeyPEM, 24),
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports + the loopback backend port + the subject-side SDS receiver port.
func (d *sdsVCDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"ListenPort":  subjListenerPort,
		"BackendPort": backendPorts[0],
		"SDSPort":     d.subjSDSPort,
		"ServerCert":  indentPEM(d.serverCertPEM, 24),
		"ServerKey":   indentPEM(d.serverKeyPEM, 24),
	})
}

func (d *sdsVCDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, "reference", addr)
}

// DriveSubject drives the subject and then hard-stops both receivers — both
// proxies hold their long-lived StreamSecrets stream open until this point, so
// GracefulStop would block (the 0103 teardown).
func (d *sdsVCDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	out, err := d.driveSide(ctx, "subject", addr)
	d.closeServers()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// closeServers hard-stops both receivers (idempotent via sdsserver.Server's own
// sync.Once).
func (d *sdsVCDriver) closeServers() {
	if d.refSrv != nil {
		d.refSrv.Close()
	}
	if d.subjSrv != nil {
		d.subjSrv.Close()
	}
}

// driveSide returns the NORMALIZED two-arm verdict. Both sides must emit
// byte-identical output.
//
// The good/bad CONTRAST is the whole proof: a client cert chaining to the
// SDS-SERVED CA is ACCEPTED; one chaining to an UN-SERVED CA is REJECTED. That
// is what makes this fixture non-vacuous — an accept-all trust store emits
// "bad=ACCEPTED", and a trust store anchored on some OTHER CA emits
// "good=REJECTED".
//
// The failure TEXT is deliberately NOT part of the observable (PLAN-65 C3): the
// reference (BoringSSL) sends the alert `unknown ca` while envoy-go (Go
// crypto/tls) sends `bad certificate`. Asserting it would fail cross-side 100%
// of the time. Only the normalized verdict is recorded (the 0045 closeOK idiom).
//
// ⚠️ The structural check at the end is LOAD-BEARING, not decoration. CompareBytes
// alone proves only that the two sides AGREE. A break that changes the served CA
// (or re-signs client_bad with it) changes BOTH sides identically, so the streams
// still compare EQUAL and a pure-CompareBytes fixture would PASS
// (reference_vacuous_break_receiver_normalizes). structuralCheck asserts each
// side's OWN bytes against the one correct shape, so such a break fails loudly
// via the runner's `t.Fatalf("<side> drive: ...")`.
func (d *sdsVCDriver) driveSide(ctx context.Context, side, addr string) ([]byte, error) {
	var out bytes.Buffer

	// Arm 1 (positive): client_good chains to the SERVED CA -> handshake OK -> echo.
	echo, err := d.mtlsEcho(ctx, addr, d.clientGood, []byte(probePayload))
	if err != nil {
		fmt.Fprintf(&out, "good=REJECTED err=%s\n", normalizeTLSErr(err))
	} else {
		fmt.Fprintf(&out, "good=ok echo=%s", echo)
	}

	// Arm 2 (negative): client_bad chains to an UN-SERVED CA -> the round trip
	// MUST fail (at the handshake under TLS 1.2, or at the first read under TLS
	// 1.3, where the server's verdict on the client cert arrives after the
	// client's Handshake() has already returned — which is why mtlsEcho drives
	// the FULL round trip rather than stopping at Handshake).
	if _, err := d.mtlsEcho(ctx, addr, d.clientBad, []byte(probePayload)); err != nil {
		out.WriteString("bad=rejected\n")
	} else {
		// The trust store accepted a cert chaining to a CA it was never served
		// -> the served CA is NOT the real anchor.
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
		probs = append(probs, "positive arm: client_good chains to the SDS-SERVED CA and MUST be accepted "+
			"(the served validation_context is not the trust anchor, or the echo path is broken)")
	}
	if !strings.HasSuffix(s, "bad=rejected\n") {
		probs = append(probs, "negative arm: client_bad chains to an UN-SERVED CA and MUST be rejected "+
			"(an accept-all trust store would pass a pure cross-side comparison — this is the vacuity guard)")
	}
	if len(probs) == 0 {
		probs = append(probs, "stream does not match the expected shape")
	}
	return fmt.Errorf("%s: SDS validation_context structural check FAILED:\n  %s\n  want: %q\n  got:  %q",
		side, strings.Join(probs, "\n  "), wantObservable, s)
}

// mtlsEcho dials addr, presents clientCert, and drives the FULL round trip:
// handshake -> write payload -> read len(payload) echoed bytes. Any stage
// failing is a REJECT, which is the version-independent normalization: under
// TLS 1.2 the server's rejection surfaces at the handshake, under TLS 1.3 it
// surfaces at the first read.
//
// The dial is inlined deliberately: neither helpers.TLSServedLeaf
// (test/helpers/tls.go:63-80) nor helpers.TLSRoundTrip can present a CLIENT
// cert — neither has a Certificates field nor a parameter to supply one. This
// keeps test/helpers untouched (the 0045 tlsDial shape + the 0018 scenario-6
// tls.Config shape).
func (d *sdsVCDriver) mtlsEcho(ctx context.Context, addr string, clientCert stdtls.Certificate, payload []byte) ([]byte, error) {
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
func (*sdsVCDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- file / template helpers (the 0103 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0108-xds-sds-validation-context/driver/driver.go
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
var _ fixture.Driver = (*sdsVCDriver)(nil)
