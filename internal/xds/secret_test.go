package xds

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestSecretTypeURL(t *testing.T) {
	got := secretTypeURL()
	want := "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret"
	if got != want {
		t.Fatalf("secretTypeURL() = %q, want %q", got, want)
	}
}

func TestDataSourceBytes_InlineBytes(t *testing.T) {
	ds := &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte("PEM")}}
	got, err := dataSourceBytes(ds, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "PEM" {
		t.Fatalf("dataSourceBytes() = %q, want %q", got, "PEM")
	}
}

func TestDataSourceBytes_InlineString(t *testing.T) {
	ds := &corev3.DataSource{Specifier: &corev3.DataSource_InlineString{InlineString: "PEM"}}
	got, err := dataSourceBytes(ds, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "PEM" {
		t.Fatalf("dataSourceBytes() = %q, want %q", got, "PEM")
	}
}

func TestDataSourceBytes_Filename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secret.pem"), []byte("PEM-FILE"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	ds := &corev3.DataSource{Specifier: &corev3.DataSource_Filename{Filename: "secret.pem"}}
	got, err := dataSourceBytes(ds, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "PEM-FILE" {
		t.Fatalf("dataSourceBytes() = %q, want %q", got, "PEM-FILE")
	}
}

func TestDataSourceBytes_EnvironmentVariable(t *testing.T) {
	ds := &corev3.DataSource{Specifier: &corev3.DataSource_EnvironmentVariable{EnvironmentVariable: "SOME_VAR"}}
	_, err := dataSourceBytes(ds, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "environment_variable") {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), "environment_variable")
	}
}

func TestDataSourceBytes_NoneSet(t *testing.T) {
	ds := &corev3.DataSource{}
	_, err := dataSourceBytes(ds, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "inline_bytes") {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), "inline_bytes")
	}
}

// selfSignedPEM generates a fresh self-signed leaf certificate + key pair (PEM
// encoded) for use as valid tls_certificate DataSource bytes in tests.
func selfSignedPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "secret.envoy-go.test"},
		DNSNames:     []string{"secret.envoy-go.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("x509.MarshalPKCS8PrivateKey: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

// anyOf wraps a proto.Message into an *anypb.Any.
func anyOf(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// inlineDS returns a DataSource with the given bytes inlined.
func inlineDS(b []byte) *corev3.DataSource {
	return &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: b}}
}

func TestParseSecret_Valid(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	sec := &tlsv3.Secret{
		Name: "server_cert",
		Type: &tlsv3.Secret_TlsCertificate{TlsCertificate: &tlsv3.TlsCertificate{
			CertificateChain: inlineDS(certPEM),
			PrivateKey:       inlineDS(keyPEM),
		}},
	}
	cert, err := parseSecret(anyOf(t, sec), "server_cert", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cert == nil {
		t.Fatalf("parseSecret() returned nil cert with nil error")
	}
	if len(cert.Certificate) != 1 {
		t.Errorf("len(cert.Certificate) = %d, want 1", len(cert.Certificate))
	}
}

func TestParseSecret_WrongName(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	sec := &tlsv3.Secret{
		Name: "other",
		Type: &tlsv3.Secret_TlsCertificate{TlsCertificate: &tlsv3.TlsCertificate{
			CertificateChain: inlineDS(certPEM),
			PrivateKey:       inlineDS(keyPEM),
		}},
	}
	_, err := parseSecret(anyOf(t, sec), "server_cert", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), "name")
	}
}

func TestParseSecret_WrongOneof(t *testing.T) {
	sec := &tlsv3.Secret{
		Name: "server_cert",
		Type: &tlsv3.Secret_ValidationContext{ValidationContext: &tlsv3.CertificateValidationContext{}},
	}
	_, err := parseSecret(anyOf(t, sec), "server_cert", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "tls_certificate") {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), "tls_certificate")
	}
}

func TestParseSecret_NonSecretAny(t *testing.T) {
	_, err := parseSecret(anyOf(t, &discoveryv3.DiscoveryRequest{}), "server_cert", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), "type")
	}
}

func TestParseSecret_BadPEM(t *testing.T) {
	sec := &tlsv3.Secret{
		Name: "server_cert",
		Type: &tlsv3.Secret_TlsCertificate{TlsCertificate: &tlsv3.TlsCertificate{
			CertificateChain: inlineDS([]byte("not pem")),
			PrivateKey:       inlineDS([]byte("not pem")),
		}},
	}
	_, err := parseSecret(anyOf(t, sec), "server_cert", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "load") {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), "load")
	}
}
