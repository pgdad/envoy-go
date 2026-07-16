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
//
// It takes a testing.TB (not a *testing.T) so the fuzz seed helpers in
// fuzz_test.go, which hold a *testing.F, can reuse it.
func selfSignedPEM(t testing.TB) (certPEM, keyPEM []byte) {
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

// vcSecret builds a Secret{name, validation_context{trusted_ca: inline caPEM}},
// the phase-65 happy-path shape (SPEC-65 §11 config_dump).
func vcSecret(t *testing.T, name string, caPEM []byte) *anypb.Any {
	t.Helper()
	return anyOf(t, &tlsv3.Secret{
		Name: name,
		Type: &tlsv3.Secret_ValidationContext{ValidationContext: &tlsv3.CertificateValidationContext{
			TrustedCa: inlineDS(caPEM),
		}},
	})
}

func TestParseValidationSecret_Valid(t *testing.T) {
	caPEM, _ := selfSignedPEM(t)
	pool, err := parseValidationSecret(vcSecret(t, "validation_ca", caPEM), "validation_ca", "")
	if err != nil {
		t.Fatalf("parseValidationSecret: unexpected err %v", err)
	}
	if pool == nil {
		t.Fatal("pool is nil")
	}
	// The pool must actually carry the CA — an empty pool would silently
	// accept nothing (or, as ClientCAs, reject every client).
	if got := len(pool.Subjects()); got != 1 { //nolint:staticcheck // Subjects() is fine for a test-only count
		t.Errorf("pool holds %d subjects, want 1", got)
	}
}

func TestParseValidationSecret_WrongName(t *testing.T) {
	caPEM, _ := selfSignedPEM(t)
	_, err := parseValidationSecret(vcSecret(t, "other", caPEM), "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "!= requested") {
		t.Errorf("error = %q, want it to mention the name mismatch", err.Error())
	}
}

// TestParseValidationSecret_WrongOneof is the MIRROR of TestParseSecret_WrongOneof
// (secret_test.go:175): parseSecret rejects a validation_context, and
// parseValidationSecret rejects a tls_certificate. The two appliers stay disjoint.
func TestParseValidationSecret_WrongOneof(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	sec := anyOf(t, &tlsv3.Secret{
		Name: "validation_ca",
		Type: &tlsv3.Secret_TlsCertificate{TlsCertificate: &tlsv3.TlsCertificate{
			CertificateChain: inlineDS(certPEM),
			PrivateKey:       inlineDS(keyPEM),
		}},
	})
	_, err := parseValidationSecret(sec, "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "is not a validation_context") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "is not a validation_context")
	}
}

// TestParseValidationSecret_CVCRejects: lifting the SDS envelope is NOT license to
// silently accept CertificateValidationContext sub-fields envoy-go cannot honor
// (reference_strict_reject_sibling_typeurl_gap). Each mirrors an inline reject
// (internal/tls/config.go:234-245) with an `xds: sds:`-prefixed DISTINCT substring
// (ADR-0080). Errorf per row so one failure does not mask the rest.
func TestParseValidationSecret_CVCRejects(t *testing.T) {
	caPEM, _ := selfSignedPEM(t)
	cases := []struct {
		name    string
		mut     func(*tlsv3.CertificateValidationContext)
		wantSub string
	}{
		{"custom_validator_config", func(v *tlsv3.CertificateValidationContext) {
			v.CustomValidatorConfig = &corev3.TypedExtensionConfig{Name: "x"}
		}, "custom_validator_config is not supported"},
		{"match_typed_subject_alt_names", func(v *tlsv3.CertificateValidationContext) {
			v.MatchTypedSubjectAltNames = []*tlsv3.SubjectAltNameMatcher{{}}
		}, "match_typed_subject_alt_names is not supported"},
		{"verify_certificate_hash", func(v *tlsv3.CertificateValidationContext) {
			v.VerifyCertificateHash = []string{"deadbeef"}
		}, "verify_certificate_hash is not supported"},
		{"verify_certificate_spki", func(v *tlsv3.CertificateValidationContext) {
			v.VerifyCertificateSpki = []string{"c3BraQ=="}
		}, "verify_certificate_spki is not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vc := &tlsv3.CertificateValidationContext{TrustedCa: inlineDS(caPEM)}
			tc.mut(vc)
			sec := anyOf(t, &tlsv3.Secret{
				Name: "validation_ca",
				Type: &tlsv3.Secret_ValidationContext{ValidationContext: vc},
			})
			_, err := parseValidationSecret(sec, "validation_ca", "")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantSub)
			}
			if !strings.HasPrefix(err.Error(), "xds: sds: ") {
				t.Errorf("error = %q, want the `xds: sds: ` prefix", err.Error())
			}
		})
	}
}

func TestParseValidationSecret_NoTrustedCa(t *testing.T) {
	sec := anyOf(t, &tlsv3.Secret{
		Name: "validation_ca",
		Type: &tlsv3.Secret_ValidationContext{ValidationContext: &tlsv3.CertificateValidationContext{}},
	})
	_, err := parseValidationSecret(sec, "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "trusted_ca") {
		t.Errorf("error = %q, want it to mention trusted_ca", err.Error())
	}
}

func TestParseValidationSecret_BadPEM(t *testing.T) {
	_, err := parseValidationSecret(vcSecret(t, "validation_ca", []byte("not a pem")), "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parse failure") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "parse failure")
	}
}
