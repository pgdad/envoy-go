// Package pki generates the fixture-0018-http-rbac mTLS PKI materials
// (fixture-CA + server cert/key + client cert/key) at fixture-load time per
// planner-time decision 11 (FRESH-cert generation; NOT pre-baked).
//
// # Why fresh-cert, not committed
//
// Phase-03 fixture 0002-tls-tcp uses committed PEMs with deterministic
// regeneration via `go run ./pki/gen`. Phase-16 fixture 0018 follows the
// alternative planner-time-decision-11 disposition: regenerate the PKI at
// fixture-load time (the package's `init()` runs when the driver imports
// this package). Rationale:
//
//   - The PKI is exercised by exactly one scenario (#6, mTLS allow-by-TLS-
//     principal); committed PEMs would leak as visible "secrets" in the
//     repo. Fresh-gen keeps the test self-contained.
//   - 24-hour validity is sufficient for the differential run; cert rotation
//     is automatic on every test invocation.
//   - The init() approach lets the driver consume the same file-path
//     contracts established at Task 12 (the five pki*Path() accessors).
//
// # PKI hierarchy
//
//   - Fixture CA: ecdsa.P256 self-signed; CN "fixture-0018-rbac-ca"; CA=true.
//   - Server cert: ecdsa.P256 signed by CA; DNSNames=[l_test_a_tls.fixture.test];
//     CN "fixture-0018-rbac-server"; ExtKeyUsage=ServerAuth. The DNS SAN
//     entry exactly matches the SNI presented by the Task-12 driver
//     (`runTLSScenario6`'s `tls.Config{ServerName: "l_test_a_tls.fixture.test"}`).
//   - Client cert: ecdsa.P256 signed by CA; URIs=[spiffe://example.com/admin];
//     CN "client.fixture.test"; ExtKeyUsage=ClientAuth. The URI SAN exactly
//     matches the Task-13 YAML's `authenticated_admin` Principal_Authenticated
//     policy (`principal_name.exact: spiffe://example.com/admin`).
//
// # File layout
//
// Files are written to `<fixtureDir>/pki/` with the .pem / .key.pem naming
// established at Task 12 (the driver's five package-private pki*Path()
// accessors):
//
//   - ca.pem
//   - server.pem        + server.key.pem
//   - client.pem        + client.key.pem
//
// # PKI orchestration choice (a/b/c from Task 13 spec)
//
// Option (b): this package's `init()` auto-generates on import. The driver
// (`test/fixtures/0018-http-rbac/inputs/driver.go`) carries a blank-import
// `_ "github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/pki"` that
// triggers the init() chain BEFORE `ReferenceBootstrap` / `SubjectConfig` /
// `runTLSScenario6` reference the cert paths. The fixture-load ordering is
// guaranteed by Go's package-init topology (pki's init runs strictly before
// the inputs package's init).
//
// Test-side validation lives in `pki_test.go` (parses each emitted PEM and
// asserts x509-cert validity); per PLAN line 634 acceptance.
package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// init triggers Generate against the fixture's `pki/` directory. Failure
// panics — fixture-load-time PKI gen MUST succeed for scenario 6 to load
// certs. The driver's Task-12 contract reads from the same directory via the
// five pki*Path() accessors.
func init() {
	dir := defaultOutputDir()
	if err := Generate(dir); err != nil {
		panic(fmt.Sprintf("pki: init-time Generate(%s): %v", dir, err))
	}
}

// defaultOutputDir resolves the fixture's `pki/` directory from this source
// file's location via runtime.Caller — works regardless of the caller's cwd
// (mirrors driver.go's `fixtureDir()` helper).
func defaultOutputDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("pki: runtime.Caller failed — cannot locate output directory")
	}
	// thisFile is .../test/fixtures/0018-http-rbac/pki/gen.go; the same
	// directory holds the emitted PEMs.
	return filepath.Dir(thisFile)
}

// Generate writes a fresh fixture-CA cert + server cert/key + client cert/key
// to dir. Idempotent across init() invocations within a single process (the
// pem.Encode + os.WriteFile pair atomically replace any existing files).
//
// Validity: 24 hours. Long enough for a single test run; short enough that
// committed PEMs (if any leaked into git) would expire quickly.
func Generate(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	now := time.Now()
	notBefore := now.Add(-time.Hour)
	notAfter := now.Add(24 * time.Hour)

	// 1. Fixture CA — self-signed.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate CA key: %w", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "fixture-0018-rbac-ca"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create CA cert: %w", err)
	}
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return fmt.Errorf("parse CA cert: %w", err)
	}

	// 2. Server cert — signed by CA; DNS SAN matches Task-12 driver's SNI.
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate server key: %w", err)
	}
	serverTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "fixture-0018-rbac-server"},
		DNSNames:     []string{"l_test_a_tls.fixture.test"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTmpl, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create server cert: %w", err)
	}

	// 3. Client cert — signed by CA; URI SAN spiffe://example.com/admin
	// matches the listener-level RBAC policy `authenticated_admin` in
	// envoy.yaml + envoy-go.yaml.
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate client key: %w", err)
	}
	spiffeURI, err := url.Parse("spiffe://example.com/admin")
	if err != nil {
		return fmt.Errorf("parse SPIFFE URI: %w", err)
	}
	clientTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "client.fixture.test"},
		URIs:         []*url.URL{spiffeURI},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTmpl, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create client cert: %w", err)
	}

	// 4. PEM-encode + write. File names match the Task-12 driver's
	// pki*Path() accessors: ca.pem / server.pem / server.key.pem /
	// client.pem / client.key.pem.
	if err := writeCertPEM(filepath.Join(dir, "ca.pem"), caCertDER); err != nil {
		return err
	}
	if err := writeCertPEM(filepath.Join(dir, "server.pem"), serverCertDER); err != nil {
		return err
	}
	if err := writeKeyPEM(filepath.Join(dir, "server.key.pem"), serverKey); err != nil {
		return err
	}
	if err := writeCertPEM(filepath.Join(dir, "client.pem"), clientCertDER); err != nil {
		return err
	}
	if err := writeKeyPEM(filepath.Join(dir, "client.key.pem"), clientKey); err != nil {
		return err
	}

	return nil
}

// writeCertPEM PEM-encodes a CERTIFICATE block (DER bytes) to path with
// 0o644 perms.
func writeCertPEM(path string, der []byte) error {
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// writeKeyPEM PEM-encodes a PRIVATE KEY block (PKCS#8) to path with 0o600
// perms (mode restricted to read+write for owner only; matches the
// phase-03 precedent + the standard mTLS key-perms convention).
func writeKeyPEM(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal PKCS8 key for %s: %w", path, err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
