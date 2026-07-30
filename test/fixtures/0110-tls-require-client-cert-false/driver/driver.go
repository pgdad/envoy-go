// Package driver is the differential fixture driver for
// 0110-tls-require-client-cert-false: a CVC-primary, three-arm verify-if-presented
// verdict at require_client_certificate=false, with a FORCED-SEND untrusted arm.
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
	"log"
	"math"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"github.com/pgdad/envoy-go/test/helpers/sdsserver"
)

const (
	fixtureName = "0110-tls-require-client-cert-false"

	// In-container reference Envoy listener port. Convention "100NN" for
	// fixture "00NN"; 0108 took 10444 and 0109 took 10445, so 0110 takes 10446
	// (RE-DERIVED free: zero hits across test/).
	refListenerPort = 10446

	// secretName is the SDS resource name of the SERVED validation_context —
	// fixed identically in both config templates'
	// combined_validation_context.validation_context_sds_secret_config. A
	// fixture-distinct name (0109 uses "validation_ca") to keep the two SDS
	// receivers plainly separate in logs. The served wire is byte-identical to
	// 0109's; the management server cannot tell a CVC client from a plain-SDS
	// client (SPEC §8).
	secretName = "rccf_validation_ca"

	// serverName is the TLS ServerName the driver dials with — must match the
	// generated server leaf's DNS SAN. Fixture-distinct from 0109's l_sds_cvc.
	serverName = "l_rccf.fixture.test"

	// probePayload is an accepting arm's application payload, echoed back
	// verbatim by the TCP echo backend through the tcp_proxy.
	probePayload = "phase67-rccf-probe\n"

	// armDeadline bounds each arm's dial+handshake+write+read. Generous on
	// purpose: a too-tight bound would let a SLOW accept masquerade as a
	// reject on the negative arm.
	armDeadline = 10 * time.Second
)

// --- the sds.* label-hoisting roster (phase 80) ---

// sdsSecretLabel is the prometheus label BOTH sides hoist the SDS secret name
// into. MEASURED against the live pinned reference and a real subject boot:
//
//	envoy_sds_update_success{envoy_xds_resource_name="rccf_validation_ca"} 1
const sdsSecretLabel = "envoy_xds_resource_name"

// sdsProjectedNames is the five-name subset envoy-go registers per SDS secret
// and (phase 80) projects onto /stats/prometheus with the secret name hoisted
// into sdsSecretLabel. Every entry is asserted for PRESENCE plus the hoisted
// LABEL on both sides — never for a value, and never cross-side by value.
//
// ⚠️ This is a strict SUBSET of the reference's own sds.* prometheus roster.
// MEASURED live on this fixture: the reference exposes FOURTEEN distinct
// envoy_sds_* metric names (twelve families — the update_duration histogram
// contributes _sum, _bucket and _count), the subject exposes exactly these five.
// The extras are OUT OF SCOPE: this leg must NEVER set-equality assert, and must
// never assert a name outside this list.
var sdsProjectedNames = []string{
	"envoy_sds_update_attempt",
	"envoy_sds_update_success",
	"envoy_sds_update_failure",
	"envoy_sds_update_rejected",
	"envoy_sds_init_fetch_timeout",
}

// sdsMovedNames is the sub-subset that actually MOVES on both sides, and the
// only one carrying a `>= 1` value floor. MEASURED, stable across runs:
// reference attempt=3 success=1 failure=1, subject attempt=1 success=1
// failure=0 — so update_failure, update_rejected and init_fetch_timeout are
// ZERO on the SUBJECT and a blanket per-side `>= 1` over all five would be RED
// ON ARRIVAL. The roster is SPLIT for exactly that reason; it is derived from
// measurement, never from the registered-suffix list.
//
// ⚠️ AND THE FLOOR SET IS PER-FIXTURE — do not copy this one. 0111 runs the same
// leg against an EMPTY validation_context: there the reference ACKs and books
// update_success while envoy-go REJECTS and books update_rejected, so its subject
// reads update_success=0 and only update_attempt can carry a floor. Each fixture
// measures its own.
var sdsMovedNames = []string{
	"envoy_sds_update_attempt",
	"envoy_sds_update_success",
}

// sdsHasValueFloor reports whether name carries the `>= 1` floor.
func sdsHasValueFloor(name string) bool {
	for _, n := range sdsMovedNames {
		if n == name {
			return true
		}
	}
	return false
}

// promSample is ONE prometheus exposition line: its label set and its value.
type promSample struct {
	labels map[string]string
	value  float64
}

// clientCertMode selects how the dial helper presents (or withholds) a client
// certificate.
type clientCertMode int

const (
	// sendForced installs GetClientCertificate so the arm's cert is written to
	// the wire REGARDLESS of the server's advertised acceptable-CA list. This is
	// SPEC §3.7 MANDATORY for the untrusted arm (reference_go_client_cert_withholding):
	// at require=false the server sets VerifyClientCertIfGiven with ClientCAs=CA_X
	// and advertises CA_X as the acceptable CA. Go's POLITE client (Certificates:)
	// would silently WITHHOLD client_Y (issuer CA_Y is not advertised via
	// SupportsCertificate filtering), collapsing the untrusted arm into a second
	// no-cert arm — a vacuous green. GetClientCertificate bypasses SupportsCertificate
	// and forces the cert onto the wire so the server actually verifies-and-rejects it.
	sendForced clientCertMode = iota
	// sendNone presents NO client certificate — neither Certificates nor
	// GetClientCertificate is set. This is the require=false DISCRIMINATOR arm:
	// verify-if-presented accepts a no-cert connection; at require=true it would
	// be rejected with TLS alert 116 (certificate_required).
	sendNone
)

// wantObservable is the ONLY byte stream a correct implementation may emit —
// on BOTH sides. structuralCheck enforces it per side. Each arm is its own
// segment/line so the three propositions read unambiguously.
const wantObservable = "trusted=ok echo=" + probePayload + "untrusted=rejected\nnone=ok echo=" + probePayload

func init() {
	fixture.RegisterFixture(fixtureName, &rccfDriver{})
}

// rccfDriver carries the per-driver lifecycle state — the in-memory PKI and
// TWO private SDS receivers, one per side
// (reference_periodic_sink_differential_two_receivers).
//
// The CA roles mirror 0109 (CVC REPLACE, not union):
//   - CA_X is served over SDS as the validation_context — the anchor that MUST WIN.
//   - CA_Y is the inline combined_validation_context.default_validation_context.
//     trusted_ca — a CONFIGURED competitor that MUST LOSE.
//
// THE DELTA vs 0109: require_client_certificate is FALSE here (verify-if-presented,
// not mandatory mTLS). The three arms prove: (1) the anchor is LIVE at require=false
// (untrusted rejected), (2) the served pool REPLACED the inline default (client_Y
// rejected — a union would accept it), (3) verify-if-presented — a NO-CERT
// connection is ACCEPTED (the flag's discriminator; this arm FLIPS vs require=true,
// where it would be rejected).
type rccfDriver struct {
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
func (d *rccfDriver) ensure() {
	d.once.Do(func() {
		caX, caXKey := mustCA("envoy-go 0110 CA_X (served over SDS)")
		caY, caYKey := mustCA("envoy-go 0110 CA_Y (inline default)")

		d.caXServedPEM = mustCertPEM(caX.Raw)
		d.caYInlinePEM = mustCertPEM(caY.Raw)

		// Server leaf: signed by CA_X (the SERVED CA), ServerAuth + the dialed DNS
		// SAN. The driver's RootCAs trusts CA_X so ALL arms get past server
		// verification — the only thing under test is the proxy's verdict on OUR
		// client cert (or its absence).
		serverDER, serverKey := mustLeaf("envoy-go 0110 server", caX, caXKey,
			[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, []string{serverName})
		d.serverCertPEM = mustCertPEM(serverDER)
		d.serverKeyPEM = mustKeyPEM(serverKey)

		d.serverCAPool = x509.NewCertPool()
		if !d.serverCAPool.AppendCertsFromPEM(d.caXServedPEM) {
			panic("driver: CA_X PEM: no certificates parsed")
		}

		// client_X chains to CA_X (SERVED) -> the proxy's SDS-delivered ClientCAs
		// MUST accept it.
		d.clientX = mustClientCert("envoy-go 0110 client_X", caX, caXKey)
		// client_Y chains to CA_Y (the INLINE default_validation_context.trusted_ca)
		// -> MUST be rejected. This is the REPLACE arm: if it is accepted, the
		// inline default UNIONED with the served pool instead of being REPLACED by
		// it, and the fixture's proposition is refuted.
		d.clientY = mustClientCert("envoy-go 0110 client_Y", caY, caYKey)

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

func (*rccfDriver) BackendCount() int           { return 1 } // the accepting arms ECHO through it
func (*rccfDriver) SubjectListenerName() string { return "l_rccf" }
func (*rccfDriver) ReferenceListenerPort() int  { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// reference-side SDS receiver port + the runner-allocated backend port, and
// injects the STATIC server cert/key AND the inline CA_Y default (no file, no
// mount).
func (d *rccfDriver) ReferenceBootstrap(backendPorts []int) string {
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
func (d *rccfDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
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

func (d *rccfDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, "reference", addr)
}

// DriveSubject drives the subject and then hard-stops both receivers — both
// proxies hold their long-lived StreamSecrets stream open until this point, so
// GracefulStop would block (the 0103/0108/0109 teardown).
func (d *rccfDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	out, err := d.driveSide(ctx, "subject", addr)
	d.closeServers()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// closeServers hard-stops both receivers (idempotent via sdsserver.Server's own
// sync.Once).
func (d *rccfDriver) closeServers() {
	if d.refSrv != nil {
		d.refSrv.Close()
	}
	if d.subjSrv != nil {
		d.subjSrv.Close()
	}
}

// serverForSide returns the SDS receiver that THIS side's proxy dials.
func (d *rccfDriver) serverForSide(side string) *sdsserver.Server {
	if side == "subject" {
		return d.subjSrv
	}
	return d.refSrv
}

// driveSide returns the NORMALIZED three-arm verdict. Both sides must emit
// byte-identical output.
//
// The three-arm CONTRAST is the whole proof AT require=false (verify-if-presented):
//   - trusted:   client_X chains to the SDS-SERVED CA (CA_X)     -> ACCEPTED (+echo)
//   - untrusted: client_Y chains to the INLINE CA (CA_Y)         -> REJECTED
//   - none:      NO client cert is presented                     -> ACCEPTED (+echo)
//
// The untrusted arm proves the served pool REPLACED the inline default (a union
// would trust CA_Y and accept client_Y). The none arm is the FLAG's DISCRIMINATOR:
// at require=false a no-cert connection is honored-if-absent and ACCEPTED; at
// require=true (0109) it would be REJECTED with TLS alert 116. That single arm is
// what separates verify-if-presented from mandatory mTLS.
//
// The failure TEXT is deliberately NOT part of the observable (inherits PLAN-65
// C3): the reference (BoringSSL) sends the alert `unknown ca` while envoy-go
// (Go crypto/tls) sends `bad certificate`. Asserting it would fail cross-side
// 100% of the time (reference_differential_reference_parses_full_message). Only
// the normalized verdict is recorded.
//
// ⚠️ The structural check at the end is LOAD-BEARING, not decoration. CompareBytes
// alone proves only that the two sides AGREE. A break that changes the served CA
// (or re-signs client_Y with it) changes BOTH sides identically, so the streams
// still compare EQUAL and a pure-CompareBytes fixture would PASS
// (reference_vacuous_break_receiver_normalizes). structuralCheck asserts each
// side's OWN bytes against the one correct shape, so such a break fails loudly.
func (d *rccfDriver) driveSide(ctx context.Context, side, addr string) ([]byte, error) {
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

	// Arm 1 (trusted, positive): client_X chains to CA_X (SERVED), FORCED-SEND ->
	// handshake OK -> echo.
	echo, err := d.mtlsEcho(ctx, addr, sendForced, d.clientX, []byte(probePayload))
	if err != nil {
		fmt.Fprintf(&out, "trusted=REJECTED err=%s\n", normalizeTLSErr(err))
	} else {
		fmt.Fprintf(&out, "trusted=ok echo=%s", echo)
	}

	// Arm 2 (untrusted, negative): client_Y chains to CA_Y (the INLINE default),
	// FORCED-SEND (SPEC §3.7 — MANDATORY). The server verify-if-presents against
	// ClientCAs=CA_X and REJECTS. Forcing the cert onto the wire is load-bearing:
	// a polite client would WITHHOLD client_Y (issuer CA_Y not advertised) and the
	// handshake would SUCCEED, collapsing this into a second no-cert arm — a
	// vacuous green (reference_go_client_cert_withholding).
	if _, err := d.mtlsEcho(ctx, addr, sendForced, d.clientY, []byte(probePayload)); err != nil {
		out.WriteString("untrusted=rejected\n")
	} else {
		// The trust store accepted a cert chaining to the INLINE default CA -> the
		// served pool UNIONED with the inline default rather than REPLACING it.
		out.WriteString("untrusted=ACCEPTED\n")
	}

	// Arm 3 (none, the discriminator): present NO client cert. At require=false
	// (verify-if-presented) this is ACCEPTED and echoes; at require=true it would
	// be rejected with alert 116. This arm FLIPS vs 0109.
	echo, err = d.mtlsEcho(ctx, addr, sendNone, stdtls.Certificate{}, []byte(probePayload))
	if err != nil {
		// A rejection here means require=false did NOT relax to verify-if-presented
		// (the flag was ignored, or the code still requires a client cert).
		fmt.Fprintf(&out, "none=REJECTED err=%s\n", normalizeTLSErr(err))
	} else {
		fmt.Fprintf(&out, "none=ok echo=%s", echo)
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
		probs = append(probs, "trusted arm: client_X chains to the SDS-SERVED CA (CA_X) and MUST be accepted with an echo "+
			"(the served validation_context is not the trust anchor, or the echo path is broken)")
	}
	if !strings.Contains(s, "\nuntrusted=rejected\n") {
		probs = append(probs, "untrusted arm: client_Y (FORCED-SEND) chains to the INLINE default CA (CA_Y) and MUST be rejected "+
			"(the SDS-served pool REPLACES the inline default; if client_Y is accepted the pools UNIONED and the proposition "+
			"is refuted — and if it WITHHELD, the forced-send regressed to a polite dial and the arm went vacuous)")
	}
	if !strings.HasSuffix(s, "none=ok echo="+probePayload) {
		probs = append(probs, "none arm (the discriminator): NO client cert MUST be ACCEPTED at require=false (verify-if-presented); "+
			"a rejection here means the flag was ignored and a client cert is still mandatory (at require=true this arm is rejected "+
			"with alert 116)")
	}
	if len(probs) == 0 {
		probs = append(probs, "stream does not match the expected shape")
	}
	return fmt.Errorf("%s: require=false verify-if-presented structural check FAILED:\n  %s\n  want: %q\n  got:  %q",
		side, strings.Join(probs, "\n  "), wantObservable, s)
}

// mtlsEcho dials addr, presents a client cert per mode, and drives the FULL
// round trip: handshake -> write payload -> read len(payload) echoed bytes. Any
// stage failing is a REJECT, which is the version-independent normalization:
// under TLS 1.2 the server's rejection surfaces at the handshake, under TLS 1.3
// it surfaces at the first read.
//
// clientCertMode is the load-bearing delta vs 0109. sendForced installs
// GetClientCertificate so the cert transmits regardless of the server's
// advertised acceptable-CA list (SPEC §3.7; reference_go_client_cert_withholding);
// sendNone presents nothing (the require=false discriminator arm).
//
// The dial is inlined deliberately: neither helpers.TLSServedLeaf nor
// helpers.TLSRoundTrip can present a CLIENT cert. This keeps test/helpers
// untouched (the 0045 tlsDial shape + the 0018 scenario-6 tls.Config shape).
func (d *rccfDriver) mtlsEcho(ctx context.Context, addr string, mode clientCertMode, armCert stdtls.Certificate, payload []byte) ([]byte, error) {
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
// exists ONLY for the diagnostic paths on the ACCEPTING arms (a failure that must
// never happen); the untrusted arm never records error text, because the reference
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
func (*rccfDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- file / template helpers (the 0103/0108/0109 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0110-tls-require-client-cert-false/driver/driver.go
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

// --- fixture.StatsAsserter (phase 75): the CROSS-SIDE ssl.* counter leg ---

// AssertStats is the runner's step-10 stats leg (ADR-0062). It scrapes
// /stats/prometheus on BOTH sides and pins the FOUR listener-scope ssl.* counters
// to their exact per-side values, then cross-side: the three phase-74
// handshake-outcome counters plus the phase-75 SUCCESS-PATH annotation
// ssl.no_certificate — the FIRST cross-side assertion of that name.
//
// ⚠️ The phase-75 name is what makes this fixture's counter layer DISCRIMINATING
// where the accept/reject contrast is not. Arms 1 and 3 BOTH accept, so the byte
// observable cannot tell them apart; ssl.handshake=2 against ssl.no_certificate=1
// can, because only arm 3 completed its handshake without presenting a cert.
//
// Shape A (scrape ONCE, ABSOLUTE counts, no baseline delta): nothing pre-moves
// l_rccf's ssl.* counters. AssertStats runs at step 10, strictly after both Drives
// and CompareBytes; reference readiness polls admin 9901 (not the TLS port),
// subject readiness parses a stdout sentinel, and startSubjectWithRetry restarts
// with a FRESH process (stats reset). The three arms of driveSide are therefore
// the ONLY connections l_rccf ever sees ⇒ deterministically 3 accepts / 2 handshake
// successes / 1 rejection per side.
//
// ⚠️ THE sds.* BOUNDARY IS NOW SPLIT BY WHAT IS ASSERTED, NOT BY SCOPE (phase 80,
// reversing this comment's own earlier prohibition). The old text forbade reaching
// into sds.* at all, on the ground that DriveSubject hard-stops both SDS receivers
// before step 10, so those scopes are reconnecting against a closed port while this
// runs. The MECHANISM is real and confirmed; the CONSEQUENCE was misattributed. The
// hard-stopped receiver is why the reference reads update_attempt=3 / update_failure=1
// instead of 1/0 — but those readings were STABLE across repeated runs, and the
// instability lands on VALUES, never on names or labels. So:
//
//   - NAMES and LABELS are stable and ARE asserted cross-side (assertSDSProjection).
//   - VALUES are NOT stable and are NEVER asserted cross-side. A value pin on the
//     REFERENCE side is a flake by construction: its retry count after receiver close
//     depends on timing, so a slower machine can read a different number. Only the two
//     names in sdsMovedNames carry a floor, and only per side.
//
// cluster.sds_cluster.* stays entirely UNasserted — it is not projected by this row
// and the same value instability applies to it without a name+label leg to rescue it.
//
// Keying: the two sides normalize the listener address DIFFERENTLY in the flat
// stat name, so a flat /stats comparison is impossible by name. The reference
// binds 0.0.0.0 and yields `listener.0.0.0.0_10446.ssl.handshake`; envoy-go binds
// the IPv6 WILDCARD, and normalizeAddr strips the brackets and maps BOTH ':' and
// '.' to '_', so `[::]:<runner-allocated-port>` becomes `___<port>` — NOT
// `0_0_0_0_<port>`. BOTH sides hoist the address into the `envoy_listener_address`
// LABEL and leave the metric NAME address-free, so the NAME is cross-side
// IDENTICAL and only the label differs — key on the name and IGNORE the label
// (the 0005/driver.go:537 precedent for envoy_listener_downstream_cx_total).
// Measured at the phase-75 PLAN:
//
//	reference | envoy_listener_ssl_no_certificate{envoy_listener_address="0.0.0.0_10446"} 1
//	subject   | envoy_listener_ssl_no_certificate{envoy_listener_address="___20016"} 1
func (d *rccfDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	// The two scrapes are PRECONDITIONS, not properties -> Fatalf
	// (reference_fatalf_makes_assertions_unreachable).
	ref, err := scrapeProm(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats/prometheus: %v", err)
	}
	subj, err := scrapeProm(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats/prometheus: %v", err)
	}

	// PRECONDITION 1 — the accept path ran (the 0097/0111 shape). If a side never
	// ACCEPTED a connection on l_rccf, every assertion below is vacuous — a broken
	// PRECONDITION, so Fatalf.
	if ref["envoy_listener_downstream_cx_total"] == 0 {
		t.Fatalf("reference did NOT accept on l_rccf: envoy_listener_downstream_cx_total == 0")
	}
	if subj["envoy_listener_downstream_cx_total"] == 0 {
		t.Fatalf("subject did NOT accept on l_rccf: envoy_listener_downstream_cx_total == 0")
	}

	// PRECONDITION 2 — the TLS path itself ran. ⚠️ downstream_cx_total > 0 is NOT
	// a sufficient decode-ran guard for ssl.* ON THE REFERENCE. Envoy C++'s
	// tcp_proxy dials the upstream at ACCEPT time, before any TLS byte is read, so
	// an instantly-refused upstream tears the downstream connection down
	// MID-handshake: downstream_cx_total still increments while the ENTIRE ssl.*
	// family stays silent. EXECUTED at the phase-75 PLAN: with the reference
	// cluster pointed at 127.0.0.1:1 the reference yields four honest zeros with
	// downstream_cx_total == 3.
	//
	// ⚠️ envoy-go does NOT share this hazard: serveConnection completes the
	// handshake at step (6) and only dispatches to the network chain at step (7),
	// so ssl.* is accounted STRICTLY BEFORE the upstream dial — the subject's
	// numbers were BYTE-IDENTICAL under a refused upstream. The guard is kept
	// per-side anyway, because a cross-side fixture is only as strong as its
	// weaker side.
	//
	// ⚠️ Its UNIQUE contribution is narrower than it looks: the three non-zero
	// rows would fail as value mismatches anyway. What only this guard does is
	// stop the want: 0 row from passing vacuously, and turn three cryptic
	// mismatches into ONE named diagnosis.
	if ref["envoy_listener_ssl_handshake"] == 0 {
		t.Fatalf("ref: TLS decode did NOT run — envoy_listener_ssl_handshake == 0 while "+
			"envoy_listener_downstream_cx_total == %d", ref["envoy_listener_downstream_cx_total"])
	}
	if subj["envoy_listener_ssl_handshake"] == 0 {
		t.Fatalf("subj: TLS decode did NOT run — envoy_listener_ssl_handshake == 0 while "+
			"envoy_listener_downstream_cx_total == %d", subj["envoy_listener_downstream_cx_total"])
	}

	// fixture.TB has EXACTLY Errorf/Fatalf/Helper — no Logf, no Cleanup
	// (reference_fixture_tb_has_no_logf). Diagnostics go through log.Printf.
	for _, s := range []struct {
		side string
		m    map[string]uint64
	}{{"reference", ref}, {"subject", subj}} {
		log.Printf("0110 AssertStats: %s ssl.handshake=%d ssl.no_certificate=%d ssl.fail_verify_error=%d ssl.fail_verify_no_cert=%d (downstream_cx_total=%d)",
			s.side, s.m["envoy_listener_ssl_handshake"], s.m["envoy_listener_ssl_no_certificate"],
			s.m["envoy_listener_ssl_fail_verify_error"], s.m["envoy_listener_ssl_fail_verify_no_cert"],
			s.m["envoy_listener_downstream_cx_total"])
	}

	names := []string{
		"envoy_listener_ssl_handshake",
		"envoy_listener_ssl_no_certificate",
		"envoy_listener_ssl_fail_verify_error",
		"envoy_listener_ssl_fail_verify_no_cert",
	}
	want := map[string]uint64{
		"envoy_listener_ssl_handshake":           2, // arms 1 AND 3 both complete a handshake
		"envoy_listener_ssl_no_certificate":      1, // arm 3 ONLY — the discriminator vs handshake=2
		"envoy_listener_ssl_fail_verify_error":   1, // arm 2, the FORCED-SEND untrusted cert
		"envoy_listener_ssl_fail_verify_no_cert": 0, // never: at require=false no-cert is HONORED
	}
	for _, n := range names {
		refVal, refOK := ref[n]
		subjVal, subjOK := subj[n]
		// ⚠️ THE ABSENT CHECK IS SEPARATE FROM THE VALUE CHECK, and it `continue`s.
		// A counter that fails to REGISTER reads as 0 == 0 through a single-value
		// lookup and would pass VACUOUSLY. (The 0055/driver.go:655-669 map-plus-
		// continue shape. Note 0005's struct-snapshot parser defaults ABSENT names
		// to zero — exactly the vacuity being guarded against here.)
		//
		// ⚠️ The counter this genuinely protects on THIS fixture is
		// ssl.fail_verify_no_cert. It is registered here but Inc'd on NONE of the
		// three arms (arm 2 books fail_verify_error), so deleting its registration
		// is SILENT and a single-value lookup reads 0 == 0 and passes. It is NOT
		// ssl.no_certificate: that one IS Inc'd, on arm 3, so deleting ITS
		// registration nil-pointer-SIGSEGVs the subject subprocess and the run dies
		// at structuralCheck before AssertStats ever executes (EXECUTED, phase-75
		// PLAN F4 — the absent branch never fires for an Inc'd counter).
		if !refOK {
			t.Errorf("ref: %s ABSENT from /stats/prometheus", n)
			continue
		}
		if !subjOK {
			t.Errorf("subj: %s ABSENT from /stats/prometheus", n)
			continue
		}
		if refVal != want[n] {
			t.Errorf("ref %s = %d, want %d", n, refVal, want[n])
		}
		if subjVal != want[n] {
			t.Errorf("subj %s = %d, want %d", n, subjVal, want[n])
		}
		// ⚠️ ENTAILED, and kept deliberately as a labeled REDUNDANT tripwire: given
		// ABSOLUTE want values this cannot fire unless one of the two checks above
		// already did. It is three lines, it survives a future refactor to per-side
		// want values, and it makes the cross-side claim legible at the call site —
		// but it is NOT an independently-firing property.
		if refVal != subjVal {
			t.Errorf("cross-side mismatch %s: ref=%d subj=%d", n, refVal, subjVal)
		}
	}

	d.assertSDSProjection(t, refAdminAddr, subjAdminAddr)
}

// assertSDSProjection is the phase-80 leg: the FIVE sds.* counters this fixture's
// served secret registers must appear on /stats/prometheus on BOTH sides with the
// secret name hoisted into sdsSecretLabel.
//
// ⚠️ NAME + LABEL cross-side; NEVER a value cross-side. envoy-go is
// initial-fetch-only and does not hold the SDS stream open, while the reference
// maintains a long-lived subscription that RE-ATTEMPTS after DriveSubject hard-stops
// both receivers. The two sides therefore disagree on update_attempt (and on
// update_failure, which the reference books for the post-close reconnect and the
// subject never books at all) while agreeing exactly on every NAME and on the LABEL
// VALUE. That is the whole proposition: the hoisting is cross-side identical.
//
// ⚠️ THE ROSTER IS SPLIT and the split is load-bearing. All five are asserted for
// presence + label; only sdsMovedNames carries a `>= 1` floor. A blanket per-side
// `>= 1` over all five REDS ON ARRIVAL on three names, because update_failure,
// update_rejected and init_fetch_timeout are all ZERO on the subject.
//
// ⚠️ The five are a strict SUBSET of the reference's sds.* roster. No set-equality
// assertion, and none of the other families is asserted.
//
// This is the first in-tree fixture to assert a HOISTED LABEL VALUE. 0111's
// cross-side prom assertion is precedent for the metric NAME surviving hoisting,
// not for the label value.
func (*rccfDriver) assertSDSProjection(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	// The two scrapes are PRECONDITIONS, not properties -> Fatalf.
	refL, err := scrapePromLabeled(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats/prometheus (labeled): %v", err)
	}
	subjL, err := scrapePromLabeled(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats/prometheus (labeled): %v", err)
	}

	// ⚠️ THE ONLY Fatalf IN THIS LEG, and it is deliberately scoped to the
	// SCRAPER-BROKEN diagnosis. A label-aware scrape returning ZERO families means
	// the parser stopped working; that must be separable from "the sds.* projection
	// did not land", which is what the per-name Errorf below reports. Every property
	// assertion is Errorf so the first violation cannot hide the rest
	// (reference_fatalf_makes_assertions_unreachable).
	if len(refL) == 0 || len(subjL) == 0 {
		t.Fatalf("label-aware scrape returned ZERO metric families (ref=%d subj=%d) — the SCRAPER is broken; "+
			"this is NOT the same diagnosis as the sds.* projection failing to land", len(refL), len(subjL))
	}

	sides := []struct {
		side string
		m    map[string][]promSample
	}{{"ref", refL}, {"subj", subjL}}

	// fixture.TB has EXACTLY Errorf/Fatalf/Helper — no Logf
	// (reference_fixture_tb_has_no_logf). Diagnostics go through log.Printf. The
	// observed family roster is RECORDED with its denominator so that an empty
	// result can never read as a zero result; it is never asserted.
	for _, s := range sides {
		fams := sdsFamilies(s.m)
		log.Printf("0110 assertSDSProjection: %s families=%d sds_families=%d %v",
			s.side, len(s.m), len(fams), fams)
		for _, n := range sdsProjectedNames {
			for _, sm := range s.m[n] {
				log.Printf("0110 assertSDSProjection: %s %s{%s=%q} = %g",
					s.side, n, sdsSecretLabel, sm.labels[sdsSecretLabel], sm.value)
			}
		}
	}

	for _, s := range sides {
		var missing []string
		for _, n := range sdsProjectedNames {
			samples, ok := s.m[n]
			if !ok {
				missing = append(missing, n)
				t.Errorf("%s: %s ABSENT from /stats/prometheus (the sds.* projection did not land)", s.side, n)
				continue
			}
			hit, found := promSample{}, false
			for _, sm := range samples {
				if sm.labels[sdsSecretLabel] == secretName {
					hit, found = sm, true
					break
				}
			}
			if !found {
				t.Errorf("%s: %s carries NO sample with %s=%q — the secret name was not hoisted into the label "+
					"(%d sample(s) seen, label values %v)", s.side, n, sdsSecretLabel, secretName,
					len(samples), labelValuesOf(samples, sdsSecretLabel))
				continue
			}
			if sdsHasValueFloor(n) && hit.value < 1 {
				t.Errorf("%s: %s{%s=%q} = %g, want >= 1 (this name is in sdsMovedNames and MUST have moved)",
					s.side, n, sdsSecretLabel, secretName, hit.value)
			}
		}
		// The SET, reported once and separately from the per-name failures
		// (reference_stat_count_guard_blind_to_rename). Only MISSING is a defect:
		// the roster is a strict subset, so "extra" sds families are expected and
		// are recorded by the log above rather than asserted.
		if len(missing) > 0 {
			t.Errorf("%s: %d of %d projected sds names MISSING as a SET: %v",
				s.side, len(missing), len(sdsProjectedNames), missing)
		}
	}
}

// labelValuesOf lists the value each sample carries for key — diagnostic text for
// the label-missing failure, so the message names what WAS seen instead.
func labelValuesOf(samples []promSample, key string) []string {
	out := make([]string, 0, len(samples))
	for _, s := range samples {
		out = append(out, s.labels[key])
	}
	return out
}

// scrapeProm issues GET http://<addr>/stats/prometheus and returns a map keyed by
// the metric NAME with the `{...}` label set STRIPPED ENTIRELY — the
// `envoy_listener_address` label is DELIBERATELY ignored, because the two sides
// normalize the listener address differently and the subject's port is
// runner-allocated per run. Values collide-SUM: there is only one listener here,
// so no collision occurs, but summing makes the address-agnostic keying EXPLICIT
// rather than accidental. Handles the labeled, bare and trailing-timestamp line
// variants (the 0055 scrapeProm + 0005 parseMetricLine shapes).
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
		// ParseFloat, not ParseUint: the exposition format permits float values,
		// and histogram lines can carry nan/inf. Non-finite and negative values are
		// skipped rather than converted (uint64(NaN) is undefined).
		v, err := strconv.ParseFloat(rest, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			continue
		}
		out[name] += uint64(v)
	}
	return out, nil
}

// scrapePromLabeled is scrapeProm's LABEL-PRESERVING SIBLING. It is a sibling
// and not an edit: scrapeProm keys by metric NAME with the label set stripped
// ENTIRELY, which is exactly right for the ssl.* leg (the two sides normalize
// the listener address differently, so that leg must ignore the label) and
// STRUCTURALLY UNABLE to assert a hoisted label value. scrapeProm is therefore
// retained unchanged and this one is added beside it.
//
// The return is keyed by metric NAME and carries EVERY line under that name as
// its own promSample, so a family with one series per SDS secret stays
// separable. Values are NOT collide-summed here (scrapeProm sums; that would
// destroy the per-secret split this leg exists to read).
//
// Handles the labeled, bare and trailing-timestamp line variants (the 0055
// scrapeProm + 0005 parseMetricLine shapes). Non-finite values are skipped
// rather than converted. Negative values are RETAINED (gauges may go negative);
// only the uint64 conversion in scrapeProm needed to drop them.
func scrapePromLabeled(adminAddr string) (map[string][]promSample, error) {
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

	out := map[string][]promSample{}
	for _, line := range strings.Split(body.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var name, rest string
		labels := map[string]string{}
		if open := strings.IndexByte(line, '{'); open >= 0 {
			closeIdx := strings.LastIndexByte(line, '}')
			if closeIdx < open {
				continue // malformed: no closing brace
			}
			name = line[:open]
			labels = parseLabelSet(line[open+1 : closeIdx])
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
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		out[name] = append(out[name], promSample{labels: labels, value: v})
	}
	return out, nil
}

// parseLabelSet parses a `key="value",key2="value2"` label string into a map
// (the 0005 parseLabels shape, which is unexported in a different package and
// therefore not reusable from here).
//
// The comma split is the in-tree precedent and is sound for every label this
// fixture reads: the hoisted secret name is charset-guarded before registration
// and the listener address cannot contain a comma either. A label value that
// DID contain a comma would split wrongly — recorded, not guarded.
func parseLabelSet(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			continue
		}
		k := part[:eq]
		v := strings.TrimSuffix(strings.TrimPrefix(part[eq+1:], `"`), `"`)
		out[k] = v
	}
	return out
}

// sdsFamilies returns the observed envoy_sds_* metric NAMES. DIAGNOSTIC ONLY —
// it is logged, never asserted. The projected five are a strict SUBSET of the
// reference's roster, so a set-equality assertion here would be wrong by
// construction; recording the denominator keeps an empty result from reading
// as a zero result.
func sdsFamilies(m map[string][]promSample) []string {
	var names []string
	for n := range m {
		if strings.HasPrefix(n, "envoy_sds_") {
			names = append(names, n)
		}
	}
	return names
}

// Compile-time interface assertions. ⚠️ The StatsAsserter one is MANDATORY:
// runner_test.go:1347 dispatches via a SILENT type assertion — no else branch, no
// log, no skip message — so a signature typo (*testing.T instead of fixture.TB, a
// returned error, swapped parameter order, a misspelled method name) makes
// ok == false and the whole assertion NEVER RUNS, while the compiler, go vet AND
// golangci-lint all stay quiet — a renamed method leaves the fixture GREEN with
// the whole stats leg silently gone.
var _ fixture.Driver = (*rccfDriver)(nil)
var _ fixture.StatsAsserter = (*rccfDriver)(nil)
