package tls

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	quicv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/quic/v3"
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
		cfg, err := NewDownstreamConfig(ts, "", nil)
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
		cfg, err := NewDownstreamConfig(ts, "", nil)
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
		cfg, err := NewDownstreamConfig(ts, "", nil)
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
		_, err := NewDownstreamConfig(ts, "", nil)
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
		_, err = NewDownstreamConfig(ts, "", nil)
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
		_, err := NewDownstreamConfig(ts, "", nil)
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
		_, err := NewDownstreamConfig(ts, "", nil)
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

	t.Run("SDS-bound secret, malformed SdsSecretConfig", func(t *testing.T) {
		// Phase 60.2 (ADR-0280) lifts the wholesale downstream SDS reject:
		// tls_certificate_sds_secret_configs is now routed through
		// xds.ParseSDSConfig, whose own reject arms (unprefixed "xds: sds:")
		// fire for a malformed SdsSecretConfig — here, a missing sds_config —
		// BEFORE the nil-provider / fetch path is ever reached. See
		// TestNewDownstreamConfig_SDS for the accept/timeout/nil-provider/
		// ads-sourced arms exercised via a well-formed config.
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificateSdsSecretConfigs: []*tlsv3.SdsSecretConfig{
					{Name: "some-secret"},
				},
			},
		})
		_, err := NewDownstreamConfig(ts, "", nil)
		if err == nil {
			t.Fatal("expected error for malformed SDS-bound secret, got nil")
		}
		if !strings.Contains(err.Error(), "sds_config") {
			t.Errorf("error should contain 'sds_config', got: %v", err)
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
		_, err := NewDownstreamConfig(ts, "", nil)
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
		cfg, err := NewDownstreamConfig(ts, "", nil)
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
		_, err := NewDownstreamConfig(ts, "", nil)
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
		_, err := NewDownstreamConfig(ts, "", nil)
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
		_, err := NewDownstreamConfig(ts, "", nil)
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
		_, err := NewDownstreamConfig(ts, "", nil)
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
		_, err := NewDownstreamConfig(ts, "", nil)
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

// fakeProvider is a test-only xds.SecretProvider whose fetches return canned
// values. It mirrors the shape of internal/xds's real Provider without any of
// the stream-opening machinery.
type fakeProvider struct {
	cert *stdtls.Certificate
	err  error

	pool  *x509.CertPool // returned by FetchInitialValidationContext
	vcErr error          // returned by FetchInitialValidationContext
}

func (f *fakeProvider) FetchInitialCertificate(ctx context.Context, secretName string) (*stdtls.Certificate, error) {
	return f.cert, f.err
}

func (f *fakeProvider) FetchInitialValidationContext(ctx context.Context, secretName string) (*x509.CertPool, error) {
	return f.pool, f.vcErr
}

// sdsSecretConfig builds a valid *tlsv3.SdsSecretConfig (api_config_source,
// GRPC, V3, envoy_grpc -> cluster, resource_api_version V3) that mut can
// corrupt to exercise xds.ParseSDSConfig's reject arms from this package's
// call site. Mirrors internal/xds/config_test.go's sdsCfg helper — duplicated
// here (rather than imported) because that helper is unexported test-only.
func sdsSecretConfig(name, cluster string, mut ...func(*corev3.ConfigSource)) *tlsv3.SdsSecretConfig {
	cs := &corev3.ConfigSource{
		ConfigSourceSpecifier: &corev3.ConfigSource_ApiConfigSource{
			ApiConfigSource: &corev3.ApiConfigSource{
				ApiType:             corev3.ApiConfigSource_GRPC,
				TransportApiVersion: corev3.ApiVersion_V3,
				GrpcServices: []*corev3.GrpcService{
					{
						TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
								ClusterName: cluster,
							},
						},
					},
				},
			},
		},
		ResourceApiVersion: corev3.ApiVersion_V3,
	}
	for _, m := range mut {
		m(cs)
	}
	return &tlsv3.SdsSecretConfig{
		Name:      name,
		SdsConfig: cs,
	}
}

// sdsDownstreamTS builds a DownstreamTlsContext whose common_tls_context has a
// single valid tls_certificate_sds_secret_configs entry (api_config_source
// GRPC/V3), wrapped in a TransportSocket. mut is forwarded to sdsSecretConfig
// so callers can corrupt the ConfigSource to probe xds.ParseSDSConfig's reject
// arms via NewDownstreamConfig.
func sdsDownstreamTS(t *testing.T, secret, cluster string, mut ...func(*corev3.ConfigSource)) *corev3.TransportSocket {
	t.Helper()
	return makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
		CommonTlsContext: &tlsv3.CommonTlsContext{
			TlsCertificateSdsSecretConfigs: []*tlsv3.SdsSecretConfig{sdsSecretConfig(secret, cluster, mut...)},
		},
	})
}

// sdsUpstreamTS builds an UpstreamTlsContext whose common_tls_context has a
// single valid tls_certificate_sds_secret_configs entry plus a trusted_ca (so
// NewUpstreamConfig's pre-commonTLSContextToConfig trusted_ca check passes and
// the SDS reject inside commonTLSContextToConfig — arm 6 — is what fires).
func sdsUpstreamTS(t *testing.T, secret, cluster string) *corev3.TransportSocket {
	t.Helper()
	return makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
		CommonTlsContext: &tlsv3.CommonTlsContext{
			TlsCertificateSdsSecretConfigs: []*tlsv3.SdsSecretConfig{sdsSecretConfig(secret, cluster)},
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
				ValidationContext: &tlsv3.CertificateValidationContext{
					TrustedCa: inlineBytes(pki.caPEM),
				},
			},
		},
	})
}

// TestNewDownstreamConfig_SDS exercises the phase-60.2 (ADR-0280) downstream
// SDS lift: a well-formed tls_certificate_sds_secret_configs entry is parsed
// via xds.ParseSDSConfig, then (given a live provider) blocks on
// FetchInitialCertificate and appends the resulting leaf to cfg.Certificates.
func TestNewDownstreamConfig_SDS(t *testing.T) {
	t.Run("accept via fake provider", func(t *testing.T) {
		leaf, err := stdtls.X509KeyPair(pki.leafCertPEM, pki.leafKeyPEM)
		if err != nil {
			t.Fatalf("X509KeyPair: %v", err)
		}
		cfg, err := NewDownstreamConfig(sdsDownstreamTS(t, "server_cert", "sds_cluster"), "", &fakeProvider{cert: &leaf})
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

	t.Run("timeout propagation", func(t *testing.T) {
		providerErr := fmt.Errorf("xds: sds: secret %q: initial fetch timed out after 15s", "server_cert")
		_, err := NewDownstreamConfig(sdsDownstreamTS(t, "server_cert", "sds_cluster"), "", &fakeProvider{err: providerErr})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "initial fetch timed out") {
			t.Errorf("error should contain 'initial fetch timed out', got: %v", err)
		}
	})

	t.Run("nil provider with valid SDS config", func(t *testing.T) {
		_, err := NewDownstreamConfig(sdsDownstreamTS(t, "server_cert", "sds_cluster"), "", nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "requires a live SDS provider") {
			t.Errorf("error should contain 'requires a live SDS provider', got: %v", err)
		}
	})

	t.Run("ads-sourced ConfigSource rejected before fetch", func(t *testing.T) {
		leaf, err := stdtls.X509KeyPair(pki.leafCertPEM, pki.leafKeyPEM)
		if err != nil {
			t.Fatalf("X509KeyPair: %v", err)
		}
		ts := sdsDownstreamTS(t, "server_cert", "sds_cluster", func(cs *corev3.ConfigSource) {
			cs.ConfigSourceSpecifier = &corev3.ConfigSource_Ads{Ads: &corev3.AggregatedConfigSource{}}
		})
		// A fake provider that would ACCEPT is deliberately present: if
		// ParseSDSConfig's validation didn't run before the fetch, this
		// config would wrongly succeed instead of rejecting on "ads-sourced".
		_, err = NewDownstreamConfig(ts, "", &fakeProvider{cert: &leaf})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "ads-sourced") {
			t.Errorf("error should contain 'ads-sourced', got: %v", err)
		}
	})

	t.Run("validation_context_sds_secret_config fetches and installs ClientCAs (arm 5, phase 65)", func(t *testing.T) {
		// NOTE (phase 65): the pre-65 version of this subtest passed an
		// SdsSecretConfig with NO sds_config, which — now that the path routes
		// through xds.ParseSDSConfig — would fire the `sds_config is required`
		// envelope reject and never reach the provider, making the ACCEPT
		// assertion VACUOUS. A FULL sds_config is required here.
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(pki.caPEM) {
			t.Fatal("pki.caPEM: no certificates parsed")
		}
		fp := &fakeProvider{pool: caPool}
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
					ValidationContextSdsSecretConfig: sdsSecretConfig("validation-secret", "sds_cluster"),
				},
			},
		})
		cfg, err := NewDownstreamConfig(ts, "", fp)
		if err != nil {
			t.Fatalf("NewDownstreamConfig: unexpected err %v", err)
		}
		if cfg.TLSConfig.ClientCAs == nil {
			t.Error("ClientCAs is nil — the SDS-delivered validation_context was not installed")
		}
		if cfg.TLSConfig.ClientAuth != stdtls.RequireAndVerifyClientCert {
			t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert (mandatory mTLS)", cfg.TLSConfig.ClientAuth)
		}
	})

	t.Run("validation_context_sds fetch failure boot-FAILS (the ADR-0280 departure, extended)", func(t *testing.T) {
		fp := &fakeProvider{vcErr: errors.New("xds: sds: secret \"validation-secret\": initial fetch timed out after 15s: context deadline exceeded")}
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
					ValidationContextSdsSecretConfig: sdsSecretConfig("validation-secret", "sds_cluster"),
				},
			},
		})
		_, err := NewDownstreamConfig(ts, "", fp)
		if err == nil {
			t.Fatal("expected a boot failure, got nil (envoy-go boot-FAILS where the reference serves anyway — ADR-0280)")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
		}
		if !strings.Contains(err.Error(), "initial fetch timed out") {
			t.Errorf("error = %q, want the provider's classified cause preserved", err.Error())
		}
	})

	t.Run("require_client_certificate=false leaves the SDS validation_context INERT", func(t *testing.T) {
		// Mirrors the landed inline behavior: an inline validation_context with
		// require_client_certificate=false is ALSO inert (only the require==true
		// block loads ClientCAs). Phase 65 introduces no NEW inconsistency, and
		// crucially performs NO boot-time SDS fetch for this shape (SPEC-65 §3.5) —
		// the vcErr below would surface as a boot failure if a fetch happened.
		fp := &fakeProvider{vcErr: errors.New("FETCH MUST NOT HAPPEN")}
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			// RequireClientCertificate deliberately absent (false).
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{
					{
						CertificateChain: inlineBytes(pki.leafCertPEM),
						PrivateKey:       inlineBytes(pki.leafKeyPEM),
					},
				},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
					ValidationContextSdsSecretConfig: sdsSecretConfig("validation-secret", "sds_cluster"),
				},
			},
		})
		cfg, err := NewDownstreamConfig(ts, "", fp)
		if err != nil {
			t.Fatalf("NewDownstreamConfig: unexpected err %v (the fetch must be SKIPPED, not attempted)", err)
		}
		if cfg.TLSConfig.ClientCAs != nil {
			t.Error("ClientCAs is non-nil — require_client_certificate=false must leave the SDS validation_context inert")
		}
		if cfg.TLSConfig.ClientAuth != stdtls.NoClientCert {
			t.Errorf("ClientAuth = %v, want NoClientCert (inert)", cfg.TLSConfig.ClientAuth)
		}
	})
}

// TestNewUpstreamConfig_SDS asserts arm 6: the upstream (and validate) side
// keeps the byte-identical pre-60.2 wholesale reject for
// tls_certificate_sds_secret_configs — SDS delivery is downstream-only.
func TestNewUpstreamConfig_SDS(t *testing.T) {
	t.Run("SDS-bound tls_certificate_sds_secret_configs rejected (arm 6)", func(t *testing.T) {
		_, err := NewUpstreamConfig(sdsUpstreamTS(t, "c", "cl"), "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		want := "tls: upstream: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03"
		if err.Error() != want {
			t.Errorf("error = %q, want %q (byte-identical to the pre-60.2 reject)", err.Error(), want)
		}
	})
}

// mkQUICDownstreamTS wraps a QuicDownstreamTransport (envoy.transport_sockets.quic)
// carrying an inner DownstreamTlsContext into a *corev3.TransportSocket. When
// certPEM/keyPEM are nil, the inner CommonTlsContext carries no
// tls_certificates (used to exercise the mandatory-TLS empty-cert error).
func mkQUICDownstreamTS(t *testing.T, certPEM, keyPEM []byte, alpn []string) *corev3.TransportSocket {
	t.Helper()
	common := &tlsv3.CommonTlsContext{AlpnProtocols: alpn}
	if certPEM != nil || keyPEM != nil {
		common.TlsCertificates = []*tlsv3.TlsCertificate{
			{
				CertificateChain: inlineBytes(certPEM),
				PrivateKey:       inlineBytes(keyPEM),
			},
		}
	}
	inner := &quicv3.QuicDownstreamTransport{
		DownstreamTlsContext: &tlsv3.DownstreamTlsContext{CommonTlsContext: common},
	}
	anyMsg, err := anypb.New(inner)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.quic",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: anyMsg},
	}
}

// mkQUICDownstreamTSEarlyData is mkQUICDownstreamTS plus an explicit
// enable_early_data (0-RTT) value on the QuicDownstreamTransport, used to
// exercise the phase-61.1 0-RTT strict-reject (ADR-0080).
func mkQUICDownstreamTSEarlyData(t *testing.T, certPEM, keyPEM []byte, alpn []string, enableEarlyData bool) *corev3.TransportSocket {
	t.Helper()
	common := &tlsv3.CommonTlsContext{AlpnProtocols: alpn}
	if certPEM != nil || keyPEM != nil {
		common.TlsCertificates = []*tlsv3.TlsCertificate{
			{
				CertificateChain: inlineBytes(certPEM),
				PrivateKey:       inlineBytes(keyPEM),
			},
		}
	}
	inner := &quicv3.QuicDownstreamTransport{
		DownstreamTlsContext: &tlsv3.DownstreamTlsContext{CommonTlsContext: common},
		EnableEarlyData:      wrapperspb.Bool(enableEarlyData),
	}
	anyMsg, err := anypb.New(inner)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.quic",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: anyMsg},
	}
}

// TestNewQUICDownstreamConfig_Rejects0RTT verifies the phase-61.1 strict
// reject of quic_downstream_transport.enable_early_data (0-RTT), which the
// reference supports but the minimal slice does not (ADR-0080).
func TestNewQUICDownstreamConfig_Rejects0RTT(t *testing.T) {
	ts := mkQUICDownstreamTSEarlyData(t, pki.leafCertPEM, pki.leafKeyPEM, []string{"h3"}, true)
	_, err := NewQUICDownstreamConfig(ts, "")
	if err == nil || !strings.Contains(err.Error(), "enable_early_data") {
		t.Fatalf("expected enable_early_data reject, got %v", err)
	}
}

// containsStr reports whether ss contains s.
func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// TestNewQUICDownstreamConfig_ALPNh3 verifies the QUIC transport socket unwrap
// reuses commonTLSContextToConfig: the inner DownstreamTlsContext's cert loads
// and alpn_protocols:["h3"] lands in NextProtos.
func TestNewQUICDownstreamConfig_ALPNh3(t *testing.T) {
	ts := mkQUICDownstreamTS(t, pki.leafCertPEM, pki.leafKeyPEM, []string{"h3"})
	dc, err := NewQUICDownstreamConfig(ts, "")
	if err != nil {
		t.Fatalf("NewQUICDownstreamConfig: %v", err)
	}
	if len(dc.TLSConfig.Certificates) == 0 {
		t.Errorf("no certificates loaded from the inner DownstreamTlsContext")
	}
	if !containsStr(dc.TLSConfig.NextProtos, "h3") {
		t.Errorf("NextProtos = %v, want to contain \"h3\"", dc.TLSConfig.NextProtos)
	}
}

// TestNewQUICDownstreamConfig_MandatoryTLS verifies a QUIC transport socket with
// no cert (empty inner DownstreamTlsContext) errors — mandatory TLS.
func TestNewQUICDownstreamConfig_MandatoryTLS(t *testing.T) {
	ts := mkQUICDownstreamTS(t, nil, nil, []string{"h3"})
	_, err := NewQUICDownstreamConfig(ts, "")
	if err == nil {
		t.Fatal("expected error for a QUIC transport socket with no cert, got nil")
	}
	if !strings.Contains(err.Error(), "no tls_certificates configured") {
		t.Errorf("error %q is not the mandatory-TLS empty-cert error", err.Error())
	}
}

// TestNewQUICDownstreamConfig_WrongTypeURL verifies a non-QUIC transport socket
// type URL is rejected.
func TestNewQUICDownstreamConfig_WrongTypeURL(t *testing.T) {
	ts := &corev3.TransportSocket{
		Name:       "bogus",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: &anypb.Any{TypeUrl: "type.googleapis.com/bogus.Bogus"}},
	}
	_, err := NewQUICDownstreamConfig(ts, "")
	if err == nil || !strings.Contains(err.Error(), "unexpected quic transport_socket type_url") {
		t.Fatalf("expected wrong-type-URL error, got %v", err)
	}
}

// TestValidationContextSDS_SiblingRejectsStay is a REGRESSION FENCE for phase 65
// (ADR-0286), which lifts the downstream validation_context_sds_secret_config
// reject (config.go:227) to a no-op ONLY for the live-provider path. Every OTHER
// arm must keep refusing, and the reject substring stays BYTE-IDENTICAL (ADR-0080
// distinct-substring rule). Errorf per arm so one failure does not mask the rest.
//
// The oneof matters here: CommonTlsContext.ValidationContextType holds EXACTLY
// one of validation_context / validation_context_sds_secret_config /
// combined_validation_context, so the arms below are mutually exclusive by
// construction.
func TestValidationContextSDS_SiblingRejectsStay(t *testing.T) {
	const wantSub = "SDS-bound validation_context_sds_secret_config is not supported in phase 03"

	vcSDS := func() *tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig {
		return &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
			ValidationContextSdsSecretConfig: sdsSecretConfig("validation_ca", "sds_cluster"),
		}
	}

	// Upstream: NewUpstreamConfig's trusted_ca gate (config.go:141) reads
	// common.GetValidationContext(), which returns nil when the oneof holds the SDS
	// arm — so upstream refuses BEFORE commonTLSContextToConfig is ever reached and
	// the config.go:227 upstream arm is UNREACHABLE from this entry point. The fence
	// pins the REFUSAL (what callers depend on), not a substring that never fires.
	t.Run("upstream still rejects", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.UpstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{ValidationContextType: vcSDS()},
		})
		_, err := NewUpstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "validation_context.trusted_ca is required") {
			t.Errorf("upstream error = %q, want it to contain %q", err.Error(), "validation_context.trusted_ca is required")
		}
	})

	// QUIC: NewQUICDownstreamConfig (config.go:90-113) calls
	// commonTLSContextToConfig(..., "downstream", nil) at config.go:108 — side ==
	// "downstream" WITH a nil provider. This is the arm that proves the guard needs
	// its `|| provider == nil` clause: without it, a QUIC listener carrying an SDS
	// validation_context would fall through and skip validation entirely. QUIC
	// carries no SDS.
	t.Run("quic downstream (nil provider) still rejects", func(t *testing.T) {
		ts := makeTransportSocket(t, &quicv3.QuicDownstreamTransport{
			DownstreamTlsContext: &tlsv3.DownstreamTlsContext{
				CommonTlsContext: &tlsv3.CommonTlsContext{ValidationContextType: vcSDS()},
			},
		})
		_, err := NewQUICDownstreamConfig(ts, "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), wantSub) {
			t.Errorf("quic error = %q, want it to contain %q", err.Error(), wantSub)
		}
	})

	// Downstream with a nil provider (the non-QUIC no-SDS-provider mode): must still
	// refuse. Either this reject or T5's nil-provider reject is acceptable; both are
	// `tls: `-prefixed and both refuse. Pin the REFUSAL + the prefix invariant that
	// FuzzTLSContextParse depends on.
	t.Run("downstream with NIL provider still rejects", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{ValidationContextType: vcSDS()},
		})
		_, err := NewDownstreamConfig(ts, "", nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
		}
		if !strings.Contains(err.Error(), wantSub) {
			t.Errorf("downstream/nil-provider error = %q, want it to contain %q", err.Error(), wantSub)
		}
	})
}
