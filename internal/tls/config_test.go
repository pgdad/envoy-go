package tls

import (
	stdtls "crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	// crypto imports used by pki init
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"time"
)

// testPKI holds PEM-encoded test certificate material generated at init time.
type testPKI struct {
	caPEM       []byte
	leafCertPEM []byte
	leafKeyPEM  []byte
}

// pki is the package-level test PKI, generated once per test run.
var pki = func() *testPKI {
	// CA
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "envoy-go test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		panic(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	// Leaf
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "alpha.envoy-go.test"},
		DNSNames:     []string{"alpha.envoy-go.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	if err != nil {
		panic(err)
	}
	leafCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	leafKeyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		panic(err)
	}
	leafKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyDER})

	return &testPKI{caPEM: caPEM, leafCertPEM: leafCertPEM, leafKeyPEM: leafKeyPEM}
}()

// makeTransportSocket wraps a proto message into a TransportSocket with the
// canonical tls/v3 type_url.
func makeTransportSocket(t *testing.T, inner proto.Message) *corev3.TransportSocket {
	t.Helper()
	anyMsg, err := anypb.New(inner)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.tls",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: anyMsg},
	}
}

// inlineBytes returns a DataSource with the given bytes inlined.
func inlineBytes(b []byte) *corev3.DataSource {
	return &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: b}}
}

func TestNewDownstreamConfig_Happy(t *testing.T) {
	t.Run("inline PEMs", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
			},
		})
		cfg, err := NewDownstreamConfig(ts, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg == nil || cfg.TLSConfig == nil {
			t.Fatal("expected non-nil DownstreamConfig with non-nil TLSConfig")
		}
		if len(cfg.TLSConfig.Certificates) != 1 {
			t.Errorf("got %d certificates, want 1", len(cfg.TLSConfig.Certificates))
		}
	})

	t.Run("tls_params pulled through", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				TlsParams: &tlsv3.TlsParameters{
					TlsMinimumProtocolVersion: tlsv3.TlsParameters_TLSv1_2,
					TlsMaximumProtocolVersion: tlsv3.TlsParameters_TLSv1_3,
				},
			},
		})
		cfg, err := NewDownstreamConfig(ts, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.TLSConfig.MinVersion != stdtls.VersionTLS12 {
			t.Errorf("MinVersion = %d, want %d", cfg.TLSConfig.MinVersion, stdtls.VersionTLS12)
		}
		if cfg.TLSConfig.MaxVersion != stdtls.VersionTLS13 {
			t.Errorf("MaxVersion = %d, want %d", cfg.TLSConfig.MaxVersion, stdtls.VersionTLS13)
		}
	})

	t.Run("alpn_protocols populated", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				AlpnProtocols: []string{"h2", "http/1.1"},
			},
		})
		cfg, err := NewDownstreamConfig(ts, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"h2", "http/1.1"}
		if len(cfg.TLSConfig.NextProtos) != len(want) {
			t.Fatalf("got NextProtos %v, want %v", cfg.TLSConfig.NextProtos, want)
		}
		for i, p := range want {
			if cfg.TLSConfig.NextProtos[i] != p {
				t.Errorf("NextProtos[%d] = %q, want %q", i, cfg.TLSConfig.NextProtos[i], p)
			}
		}
	})
}

func TestNewDownstreamConfig_Errors(t *testing.T) {
	t.Run("wrong type_url", func(t *testing.T) {
		// Use a non-DownstreamTlsContext proto to get a wrong type_url.
		ts := makeTransportSocket(t, wrapperspb.String("nope"))
		_, err := NewDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for wrong type_url, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "unexpected type_url") {
			t.Errorf("error should contain 'unexpected type_url', got: %v", err)
		}
	})

	t.Run("unmarshal failure", func(t *testing.T) {
		// Build a valid DownstreamTlsContext Any, then corrupt its Value bytes.
		anyMsg, err := anypb.New(&tlsv3.DownstreamTlsContext{})
		if err != nil {
			t.Fatalf("anypb.New: %v", err)
		}
		anyMsg.Value = []byte{0xff, 0xff, 0xff}
		ts := &corev3.TransportSocket{
			Name:       "envoy.transport_sockets.tls",
			ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: anyMsg},
		}
		_, err = NewDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for corrupted Any bytes, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "unmarshal") {
			t.Errorf("error should contain 'unmarshal', got: %v", err)
		}
	})

	t.Run("missing tls_certificates", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{},
		})
		_, err := NewDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for missing tls_certificates, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "no tls_certificates") {
			t.Errorf("error should contain 'no tls_certificates', got: %v", err)
		}
	})

	t.Run("malformed PEM in certificate_chain", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes([]byte("not a pem")),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
			},
		})
		_, err := NewDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for malformed PEM, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "load cert") {
			t.Errorf("error should contain 'load cert', got: %v", err)
		}
	})

	t.Run("SDS-bound secret", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificateSdsSecretConfigs: []*tlsv3.SdsSecretConfig{
					{Name: "some-secret"},
				},
			},
		})
		_, err := NewDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for SDS-bound secret, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "SDS") {
			t.Errorf("error should contain 'SDS', got: %v", err)
		}
	})

	t.Run("require_client_certificate_without_trusted_ca", func(t *testing.T) {
		// Per ADR-0147 (unanticipated phase-16 amendment): require_client_certificate=true
		// without validation_context.trusted_ca must error — the listener cannot
		// verify presented client certs without a CA pool. ADR-0147 lifts the
		// phase-03 blanket rejection (ADR-0032 §Decision (7)) only for the
		// well-formed mTLS configuration (trusted_ca PEM provided).
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
			},
		})
		_, err := NewDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for require_client_certificate without trusted_ca, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "require_client_certificate") {
			t.Errorf("error should contain 'require_client_certificate', got: %v", err)
		}
		if !strings.Contains(err.Error(), "trusted_ca") {
			t.Errorf("error should contain 'trusted_ca', got: %v", err)
		}
	})

	t.Run("require_client_certificate_with_trusted_ca", func(t *testing.T) {
		// ADR-0147: require_client_certificate=true with validation_context.trusted_ca
		// configures the listener for mandatory mTLS — ClientCAs pool populated +
		// ClientAuth=RequireAndVerifyClientCert. Lifts the phase-03 ADR-0032
		// clause-7 rejection scoped to well-formed mTLS configs.
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa: inlineBytes(pki.caPEM),
					},
				},
			},
		})
		cfg, err := NewDownstreamConfig(ts, "")
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if cfg.TLSConfig.ClientAuth != stdtls.RequireAndVerifyClientCert {
			t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", cfg.TLSConfig.ClientAuth)
		}
		if cfg.TLSConfig.ClientCAs == nil {
			t.Errorf("ClientCAs nil; want populated pool")
		}
	})

	t.Run("custom_validator_config", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						CustomValidatorConfig: &corev3.TypedExtensionConfig{Name: "x"},
					},
				},
			},
		})
		_, err := NewDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for custom_validator_config, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "custom_validator_config") {
			t.Errorf("error should contain 'custom_validator_config', got: %v", err)
		}
	})

	t.Run("match_typed_subject_alt_names", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						MatchTypedSubjectAltNames: []*tlsv3.SubjectAltNameMatcher{{}},
					},
				},
			},
		})
		_, err := NewDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for match_typed_subject_alt_names, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "match_typed_subject_alt_names") {
			t.Errorf("error should contain 'match_typed_subject_alt_names', got: %v", err)
		}
	})

	t.Run("verify_certificate_hash", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						VerifyCertificateHash: []string{"x"},
					},
				},
			},
		})
		_, err := NewDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for verify_certificate_hash, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "verify_certificate_hash") {
			t.Errorf("error should contain 'verify_certificate_hash', got: %v", err)
		}
	})

	t.Run("password on key", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
						Password: &corev3.DataSource{
							Specifier: &corev3.DataSource_InlineString{InlineString: "secret"},
						},
					},
				},
			},
		})
		_, err := NewDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for password-protected key, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "password-protected keys") {
			t.Errorf("error should contain 'password-protected keys', got: %v", err)
		}
	})

	t.Run("invalid tls_params TLSv1_0", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				TlsParams: &tlsv3.TlsParameters{
					TlsMinimumProtocolVersion: tlsv3.TlsParameters_TLSv1_0,
				},
			},
		})
		_, err := NewDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for TLSv1_0, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "TLSv1_0") {
			t.Errorf("error should contain 'TLSv1_0', got: %v", err)
		}
	})
}

func TestNewUpstreamConfig_Happy(t *testing.T) {
	t.Run("inline CA + SNI + tls_params", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			Sni: "alpha.envoy-go.test",
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsParams: &tlsv3.TlsParameters{
					TlsMinimumProtocolVersion: tlsv3.TlsParameters_TLSv1_2,
					TlsMaximumProtocolVersion: tlsv3.TlsParameters_TLSv1_3,
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa: inlineBytes(pki.caPEM),
					},
				},
			},
		})
		cfg, err := NewUpstreamConfig(ts, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg == nil || cfg.TLSConfig == nil {
			t.Fatal("expected non-nil UpstreamConfig with non-nil TLSConfig")
		}
		if cfg.TLSConfig.ServerName != "alpha.envoy-go.test" {
			t.Errorf("ServerName = %q, want %q", cfg.TLSConfig.ServerName, "alpha.envoy-go.test")
		}
		if cfg.SNI != "alpha.envoy-go.test" {
			t.Errorf("SNI = %q, want %q", cfg.SNI, "alpha.envoy-go.test")
		}
		if cfg.TLSConfig.RootCAs == nil {
			t.Error("RootCAs should be non-nil")
		}
		if cfg.TLSConfig.MinVersion != stdtls.VersionTLS12 {
			t.Errorf("MinVersion = %d, want %d", cfg.TLSConfig.MinVersion, stdtls.VersionTLS12)
		}
		if cfg.TLSConfig.MaxVersion != stdtls.VersionTLS13 {
			t.Errorf("MaxVersion = %d, want %d", cfg.TLSConfig.MaxVersion, stdtls.VersionTLS13)
		}
	})

	t.Run("alpn_protocols populated", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			Sni: "alpha.envoy-go.test",
			CommonTlsContext: &tlsv3.CommonTlsContext{
				AlpnProtocols: []string{"h2", "http/1.1"},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa: inlineBytes(pki.caPEM),
					},
				},
			},
		})
		cfg, err := NewUpstreamConfig(ts, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"h2", "http/1.1"}
		if len(cfg.TLSConfig.NextProtos) != len(want) {
			t.Fatalf("got NextProtos %v, want %v", cfg.TLSConfig.NextProtos, want)
		}
		for i, p := range want {
			if cfg.TLSConfig.NextProtos[i] != p {
				t.Errorf("NextProtos[%d] = %q, want %q", i, cfg.TLSConfig.NextProtos[i], p)
			}
		}
	})

	t.Run("allow_renegotiation false default no error", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			AllowRenegotiation: false,
			Sni:                "alpha.envoy-go.test",
			CommonTlsContext: &tlsv3.CommonTlsContext{
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa: inlineBytes(pki.caPEM),
					},
				},
			},
		})
		_, err := NewUpstreamConfig(ts, "")
		if err != nil {
			t.Fatalf("allow_renegotiation=false should not error, got: %v", err)
		}
	})
}

func TestNewUpstreamConfig_Errors(t *testing.T) {
	t.Run("wrong type_url", func(t *testing.T) {
		ts := makeTransportSocket(t, wrapperspb.String("nope"))
		_, err := NewUpstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for wrong type_url, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "unexpected type_url") {
			t.Errorf("error should contain 'unexpected type_url', got: %v", err)
		}
	})

	t.Run("missing trusted_ca", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			Sni: "alpha.envoy-go.test",
			CommonTlsContext: &tlsv3.CommonTlsContext{
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{},
				},
			},
		})
		_, err := NewUpstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for missing trusted_ca, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "trusted_ca is required") {
			t.Errorf("error should contain 'trusted_ca is required', got: %v", err)
		}
	})

	t.Run("malformed CA PEM", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			Sni: "alpha.envoy-go.test",
			CommonTlsContext: &tlsv3.CommonTlsContext{
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa: inlineBytes([]byte("not a pem")),
					},
				},
			},
		})
		_, err := NewUpstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for malformed CA PEM, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "parse failure") {
			t.Errorf("error should contain 'parse failure', got: %v", err)
		}
	})

	t.Run("SDS-bound secret", func(t *testing.T) {
		// Provide a valid trusted_ca so the trusted_ca-required check passes; the
		// SDS check in commonTLSContextToConfig then fires on TlsCertificateSdsSecretConfigs.
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificateSdsSecretConfigs: []*tlsv3.SdsSecretConfig{
					{Name: "some-secret"},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa: inlineBytes(pki.caPEM),
					},
				},
			},
		})
		_, err := NewUpstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for SDS-bound secret, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "SDS") {
			t.Errorf("error should contain 'SDS', got: %v", err)
		}
	})

	t.Run("allow_renegotiation", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			AllowRenegotiation: true,
			CommonTlsContext: &tlsv3.CommonTlsContext{
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa: inlineBytes(pki.caPEM),
					},
				},
			},
		})
		_, err := NewUpstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for allow_renegotiation=true, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "allow_renegotiation") {
			t.Errorf("error should contain 'allow_renegotiation', got: %v", err)
		}
	})

	t.Run("custom_validator_config", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			Sni: "alpha.envoy-go.test",
			CommonTlsContext: &tlsv3.CommonTlsContext{
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa:             inlineBytes(pki.caPEM),
						CustomValidatorConfig: &corev3.TypedExtensionConfig{Name: "x"},
					},
				},
			},
		})
		_, err := NewUpstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for custom_validator_config, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "custom_validator_config") {
			t.Errorf("error should contain 'custom_validator_config', got: %v", err)
		}
	})

	t.Run("match_typed_subject_alt_names", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			Sni: "alpha.envoy-go.test",
			CommonTlsContext: &tlsv3.CommonTlsContext{
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa:                 inlineBytes(pki.caPEM),
						MatchTypedSubjectAltNames: []*tlsv3.SubjectAltNameMatcher{{}},
					},
				},
			},
		})
		_, err := NewUpstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for match_typed_subject_alt_names, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "match_typed_subject_alt_names") {
			t.Errorf("error should contain 'match_typed_subject_alt_names', got: %v", err)
		}
	})

	t.Run("password on client-cert key", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			Sni: "alpha.envoy-go.test",
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
						Password: &corev3.DataSource{
							Specifier: &corev3.DataSource_InlineString{InlineString: "secret"},
						},
					},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa: inlineBytes(pki.caPEM),
					},
				},
			},
		})
		_, err := NewUpstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error for password on client-cert key, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error should begin with 'tls: ', got: %v", err)
		}
		if !strings.Contains(err.Error(), "password-protected keys") {
			t.Errorf("error should contain 'password-protected keys', got: %v", err)
		}
	})
}

// TestPKISanity verifies the inline test PKI is self-consistent:
// loading the leaf cert with the leaf key should round-trip through
// stdtls.X509KeyPair without error, and the CA should verify the leaf.
func TestPKISanity(t *testing.T) {
	// Suppress unused-import lint if the asn1 package is only used indirectly
	// via init (crypto routines use it internally).
	_ = asn1.ObjectIdentifier(nil)

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(pki.caPEM) {
		t.Fatal("CA PEM did not append")
	}
	block, _ := pem.Decode(pki.leafCertPEM)
	if block == nil {
		t.Fatal("leaf PEM decode failed")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: caPool, DNSName: "alpha.envoy-go.test"}); err != nil {
		t.Fatalf("leaf verify: %v", err)
	}
	// Also verify X509KeyPair round-trip.
	if _, err := stdtls.X509KeyPair(pki.leafCertPEM, pki.leafKeyPEM); err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}

	// Verify strings is used via the other tests to avoid unused import.
	_ = strings.TrimSpace
}
