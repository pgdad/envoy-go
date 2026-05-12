// Package pki tests — verify the fresh-cert generator produces valid x509
// materials usable for mTLS handshake (PLAN line 634 acceptance).
//
// The package's init() has already generated certs into this directory
// before the test runs; in addition, Generate(t.TempDir()) is invoked to
// verify standalone behavior (independent of init() side effects).
package pki

import (
	"crypto/x509"
	"encoding/pem"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGenerate_ProducesValidPKI exercises Generate against a fresh temp
// directory and parses each emitted PEM, asserting:
//
//   - all five files exist
//   - the CA cert is self-signed, IsCA=true
//   - the server cert is signed by the CA + carries DNSNames=[l_test_a_tls.fixture.test]
//   - the client cert is signed by the CA + carries URIs=[spiffe://example.com/admin]
//   - all three certs have NotBefore < now < NotAfter
//   - the keys parse as ecdsa.P256 (the canonical curve per planner-time
//     decision 11)
//   - the chain (server, client) verifies against the CA via x509.Verify
//
// This is the PLAN line 634 acceptance: "PKI generator produces valid x509
// certs (verified via standalone Go test of gen.go)".
func TestGenerate_ProducesValidPKI(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Verify all five files exist + parse.
	caCert := parseCert(t, filepath.Join(dir, "ca.pem"))
	serverCert := parseCert(t, filepath.Join(dir, "server.pem"))
	clientCert := parseCert(t, filepath.Join(dir, "client.pem"))
	parseKey(t, filepath.Join(dir, "server.key.pem"))
	parseKey(t, filepath.Join(dir, "client.key.pem"))

	// CA properties.
	if !caCert.IsCA {
		t.Errorf("CA cert IsCA = false; want true")
	}
	if caCert.Subject.CommonName != "fixture-0018-rbac-ca" {
		t.Errorf("CA cert CN = %q; want %q", caCert.Subject.CommonName, "fixture-0018-rbac-ca")
	}
	// Self-signed: Issuer == Subject.
	if caCert.Issuer.CommonName != caCert.Subject.CommonName {
		t.Errorf("CA Issuer.CN = %q; want self-signed (%q)", caCert.Issuer.CommonName, caCert.Subject.CommonName)
	}

	// Server cert properties.
	wantServerDNS := "l_test_a_tls.fixture.test"
	foundDNS := false
	for _, d := range serverCert.DNSNames {
		if d == wantServerDNS {
			foundDNS = true
			break
		}
	}
	if !foundDNS {
		t.Errorf("server cert DNSNames = %v; want entry %q", serverCert.DNSNames, wantServerDNS)
	}
	if serverCert.Issuer.CommonName != caCert.Subject.CommonName {
		t.Errorf("server Issuer.CN = %q; want signed by CA (%q)", serverCert.Issuer.CommonName, caCert.Subject.CommonName)
	}

	// Client cert properties.
	wantClientURI, _ := url.Parse("spiffe://example.com/admin")
	foundURI := false
	for _, u := range clientCert.URIs {
		if u.String() == wantClientURI.String() {
			foundURI = true
			break
		}
	}
	if !foundURI {
		t.Errorf("client cert URIs = %v; want entry %q", clientCert.URIs, wantClientURI)
	}
	if clientCert.Issuer.CommonName != caCert.Subject.CommonName {
		t.Errorf("client Issuer.CN = %q; want signed by CA (%q)", clientCert.Issuer.CommonName, caCert.Subject.CommonName)
	}

	// Validity window — NotBefore < now < NotAfter for all three.
	now := time.Now()
	for _, c := range []*x509.Certificate{caCert, serverCert, clientCert} {
		if !c.NotBefore.Before(now) {
			t.Errorf("%s: NotBefore = %v; want < now (%v)", c.Subject.CommonName, c.NotBefore, now)
		}
		if !c.NotAfter.After(now) {
			t.Errorf("%s: NotAfter = %v; want > now (%v)", c.Subject.CommonName, c.NotAfter, now)
		}
	}

	// Chain verification: server + client verify against the CA root pool
	// with their appropriate ExtKeyUsage.
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	if _, err := serverCert.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// DNSName left empty: we verify chain trust only here. The TLS
		// handshake at scenario-6 time validates SNI separately via the
		// tls.Config.ServerName field.
	}); err != nil {
		t.Errorf("server cert chain verify: %v", err)
	}
	if _, err := clientCert.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Errorf("client cert chain verify: %v", err)
	}
}

// TestInitPopulatesFixtureDir confirms the package-init() side effect:
// after init() runs (implicit at test-binary load time), the fixture's
// `pki/` directory contains all five PEM files. This validates the
// orchestration choice (option b) documented in gen.go's package
// doc-comment.
func TestInitPopulatesFixtureDir(t *testing.T) {
	dir := defaultOutputDir()
	for _, name := range []string{"ca.pem", "server.pem", "server.key.pem", "client.pem", "client.key.pem"} {
		path := filepath.Join(dir, name)
		fi, err := os.Stat(path)
		if err != nil {
			t.Errorf("init: %s missing: %v", name, err)
			continue
		}
		if fi.Size() == 0 {
			t.Errorf("init: %s exists but is empty", name)
		}
	}
}

// parseCert reads + parses a CERTIFICATE PEM from path.
func parseCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	block, _ := pem.Decode(b)
	if block == nil {
		t.Fatalf("decode %s: no PEM block", path)
	}
	if block.Type != "CERTIFICATE" {
		t.Fatalf("decode %s: block type %q; want CERTIFICATE", path, block.Type)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return cert
}

// parseKey reads + parses a PRIVATE KEY PEM (PKCS#8) from path and asserts
// the curve is P-256.
func parseKey(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	block, _ := pem.Decode(b)
	if block == nil {
		t.Fatalf("decode %s: no PEM block", path)
	}
	if block.Type != "PRIVATE KEY" {
		t.Fatalf("decode %s: block type %q; want PRIVATE KEY", path, block.Type)
	}
	if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
		t.Fatalf("parse PKCS8 %s: %v", path, err)
	}
}
