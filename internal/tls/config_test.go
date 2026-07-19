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

	xds "github.com/pgdad/envoy-go/internal/xds"

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
			t.Fatal("expected a boot failure, got nil (envoy-go boot-FAILS where the reference init-holds then fails closed per-connection — ADR-0280 family, characterization corrected at ADR-0289)")
		}
		if !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
		}
		if !strings.Contains(err.Error(), "initial fetch timed out") {
			t.Errorf("error = %q, want the provider's classified cause preserved", err.Error())
		}
	})

	t.Run("require_client_certificate=false CONSUMES the SDS validation_context (verify-if-presented)", func(t *testing.T) {
		// Post phase-67 lift: an SDS-delivered validation_context with
		// require_client_certificate=false/absent is HONORED (verify-if-presented),
		// not inert. The fetch fires at ANY require value (un-gated), the returned
		// pool is installed as ClientCAs, and ClientAuth is VerifyClientCertIfGiven.
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(pki.caPEM) {
			t.Fatal("pki.caPEM: no certificates parsed")
		}
		fp := &fakeProvider{pool: caPool}
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
			t.Errorf("NewDownstreamConfig: unexpected err %v — false/absent + SDS anchor must verify-if-presented", err)
		}
		if cfg == nil || cfg.TLSConfig == nil {
			t.Fatal("NewDownstreamConfig returned a nil cfg/TLSConfig")
		}
		if cfg.TLSConfig.ClientCAs == nil {
			t.Error("ClientCAs is nil — require_client_certificate=false must CONSUME the SDS validation_context")
		}
		if cfg.TLSConfig.ClientAuth != stdtls.VerifyClientCertIfGiven {
			t.Errorf("ClientAuth = %v, want VerifyClientCertIfGiven (verify-if-presented)", cfg.TLSConfig.ClientAuth)
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

// ---------------------------------------------------------------------------
// Phase 66: combined_validation_context (CVC).
// ---------------------------------------------------------------------------

// cvcCTC builds a *tlsv3.CommonTlsContext carrying a combined_validation_context
// plus a tls_certificates entry. The tls_certificates entry is REQUIRED: without
// it commonTLSContextToConfig errors `no tls_certificates configured` further
// down and any CVC-arm assertion above would be vacuous. mut lets a caller strip
// either CVC half to exercise the E1/E2 arms.
func cvcCTC(mut ...func(*tlsv3.CommonTlsContext_CombinedCertificateValidationContext)) *tlsv3.CommonTlsContext {
	cvc := &tlsv3.CommonTlsContext_CombinedCertificateValidationContext{
		DefaultValidationContext: &tlsv3.CertificateValidationContext{
			TrustedCa: inlineBytes(pki.caPEM),
		},
		ValidationContextSdsSecretConfig: sdsSecretConfig("validation-secret", "sds_cluster"),
	}
	for _, m := range mut {
		m(cvc)
	}
	return &tlsv3.CommonTlsContext{
		TlsCertificates: []*tlsv3.TlsCertificate{
			{
				CertificateChain: inlineBytes(pki.leafCertPEM),
				PrivateKey:       inlineBytes(pki.leafKeyPEM),
			},
		},
		ValidationContextType: &tlsv3.CommonTlsContext_CombinedValidationContext{
			CombinedValidationContext: cvc,
		},
	}
}

// cvcDownstreamTS wraps cvcCTC in a DownstreamTlsContext + TransportSocket,
// with require_client_certificate set to require (nil => field absent).
func cvcDownstreamTS(t *testing.T, require *wrapperspb.BoolValue) *corev3.TransportSocket {
	t.Helper()
	return makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
		RequireClientCertificate: require,
		CommonTlsContext:         cvcCTC(),
	})
}

// cvcRetainedReject is the BYTE-IDENTICAL phase-03 reject substring the CVC arm
// keeps for its three nil-provider / non-downstream consumers (ADR-0080).
const cvcRetainedReject = "combined_validation_context is not supported in phase 03"

// TestCVC_DownstreamWithProvider_Accepted pins the phase-66 lift: a well-formed
// CVC on the downstream side WITH a live provider is a NO-OP that returns no
// error. It drives commonTLSContextToConfig DIRECTLY (in-package) rather than
// NewDownstreamConfig: at this task NewDownstreamConfig's require block has no
// CVC arm, so require:true would fall to the else and error with
// `require_client_certificate=true requires validation_context.trusted_ca`,
// proving nothing about the CVC arm.
func TestCVC_DownstreamWithProvider_Accepted(t *testing.T) {
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(pki.caPEM) {
		t.Fatal("pki.caPEM: no certificates parsed")
	}
	cfg, err := commonTLSContextToConfig(cvcCTC(), "", "downstream", &fakeProvider{pool: caPool})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Error("expected a non-nil *tls.Config from the accepted CVC arm")
	}
}

// TestCVC_NilProvider_KeepsByteIdenticalReject: the retained guard fires for the
// nil-provider consumers (NewQUICDownstreamConfig, validate.Bootstrap, and
// main.go's ordinary path when boot.NewSDSProvider returns (nil, nil)).
func TestCVC_NilProvider_KeepsByteIdenticalReject(t *testing.T) {
	_, err := commonTLSContextToConfig(cvcCTC(), "", "downstream", nil)
	if err == nil {
		t.Fatal("expected the retained phase-03 reject, got nil")
	}
	if !strings.HasPrefix(err.Error(), "tls: ") {
		t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
	}
	if !strings.Contains(err.Error(), cvcRetainedReject) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), cvcRetainedReject)
	}
}

// TestCVC_Upstream_KeepsByteIdenticalReject pins the RETAINED GUARD on the
// non-downstream half.
//
// This path is DEAD from today's entry points: NewUpstreamConfig rejects earlier
// with its own trusted_ca message, because validation_context_type is a ONEOF —
// selecting the CVC arm makes GetValidationContext() return nil, so the upstream
// pre-check refuses before commonTLSContextToConfig's switch is ever reached
// (verified by execution). This test pins the retained guard, NOT a live path;
// do not mistake it for evidence that the upstream arm is reachable.
func TestCVC_Upstream_KeepsByteIdenticalReject(t *testing.T) {
	_, err := commonTLSContextToConfig(cvcCTC(), "", "upstream", &fakeProvider{})
	if err == nil {
		t.Fatal("expected the retained phase-03 reject, got nil")
	}
	if !strings.Contains(err.Error(), cvcRetainedReject) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), cvcRetainedReject)
	}
}

// TestCVC_MissingDefaultValidationContext_E1: downstream + provider, CVC with no
// default_validation_context.
func TestCVC_MissingDefaultValidationContext_E1(t *testing.T) {
	ctc := cvcCTC(func(c *tlsv3.CommonTlsContext_CombinedCertificateValidationContext) {
		c.DefaultValidationContext = nil
	})
	_, err := commonTLSContextToConfig(ctc, "", "downstream", &fakeProvider{})
	if err == nil {
		t.Fatal("expected E1, got nil")
	}
	const want = "combined_validation_context.default_validation_context is required"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

// TestCVC_MissingSDSSecretConfig_E2: downstream + provider, CVC with no
// validation_context_sds_secret_config.
func TestCVC_MissingSDSSecretConfig_E2(t *testing.T) {
	ctc := cvcCTC(func(c *tlsv3.CommonTlsContext_CombinedCertificateValidationContext) {
		c.ValidationContextSdsSecretConfig = nil
	})
	_, err := commonTLSContextToConfig(ctc, "", "downstream", &fakeProvider{})
	if err == nil {
		t.Fatal("expected E2, got nil")
	}
	const want = "combined_validation_context.validation_context_sds_secret_config is required"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

// TestCVC_E1E2_DoNotPreemptTheRetainedReject: with provider == nil, an E1-shaped
// or E2-shaped CVC must still produce the BYTE-IDENTICAL retained reject — the
// gate precedes the E1/E2 checks (ADR-0080). If E1/E2 leaked here, the three
// nil-provider consumers would see a NEW message.
func TestCVC_E1E2_DoNotPreemptTheRetainedReject(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*tlsv3.CommonTlsContext_CombinedCertificateValidationContext)
	}{
		{"E1-shaped (no default_validation_context)", func(c *tlsv3.CommonTlsContext_CombinedCertificateValidationContext) {
			c.DefaultValidationContext = nil
		}},
		{"E2-shaped (no validation_context_sds_secret_config)", func(c *tlsv3.CommonTlsContext_CombinedCertificateValidationContext) {
			c.ValidationContextSdsSecretConfig = nil
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := commonTLSContextToConfig(cvcCTC(tc.mut), "", "downstream", nil)
			if err == nil {
				t.Fatal("expected the retained phase-03 reject, got nil")
			}
			if !strings.Contains(err.Error(), cvcRetainedReject) {
				t.Errorf("error = %q, want the RETAINED reject %q — the gate must precede E1/E2", err.Error(), cvcRetainedReject)
			}
			if strings.Contains(err.Error(), "is required") {
				t.Errorf("error = %q, want the RETAINED reject, NOT an E1/E2 message", err.Error())
			}
		})
	}
}

// TestNewDownstreamConfig_RequireFalse_CVC_VerifyIfGiven drives
// NewDownstreamConfig: a CVC listener with require_client_certificate false OR
// absent now HONORS the anchor (verify-if-presented) rather than boot-rejecting
// (phase 67 retires E3). The SDS-delivered pool is fetched and installed as
// ClientCAs, and ClientAuth is VerifyClientCertIfGiven.
func TestNewDownstreamConfig_RequireFalse_CVC_VerifyIfGiven(t *testing.T) {
	tests := []struct {
		name    string
		require *wrapperspb.BoolValue
	}{
		{"require_client_certificate: false", &wrapperspb.BoolValue{Value: false}},
		{"require_client_certificate absent (nil BoolValue)", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caPool := x509.NewCertPool()
			if !caPool.AppendCertsFromPEM(pki.caPEM) {
				t.Fatal("pki.caPEM: no certificates parsed")
			}
			cfg, err := NewDownstreamConfig(cvcDownstreamTS(t, tc.require), "", &fakeProvider{pool: caPool})
			if err != nil {
				t.Errorf("NewDownstreamConfig: unexpected err %v — false/absent + anchor must verify-if-presented, not reject", err)
			}
			if cfg == nil || cfg.TLSConfig == nil {
				t.Fatal("NewDownstreamConfig returned a nil cfg/TLSConfig")
			}
			if cfg.TLSConfig.ClientAuth != stdtls.VerifyClientCertIfGiven {
				t.Errorf("ClientAuth = %v, want VerifyClientCertIfGiven", cfg.TLSConfig.ClientAuth)
			}
			if cfg.TLSConfig.ClientCAs == nil {
				t.Error("ClientCAs is nil — the SDS-delivered anchor pool must be installed at false/absent")
			}
		})
	}
}

// TestCVC_RequireFalse_NeverYieldsNoClientCert states the SECURITY PROPERTY
// rather than the mechanism: for a CVC listener with require_client_certificate
// false/absent AND an anchor, NewDownstreamConfig must verify-if-presented and
// must NEVER hand back a cfg whose ClientAuth is NoClientCert — i.e. a silently
// unauthenticated listener. Post phase-67 lift the anchor is HONORED
// (ClientAuth == VerifyClientCertIfGiven), which entails != NoClientCert.
func TestCVC_RequireFalse_NeverYieldsNoClientCert(t *testing.T) {
	tests := []struct {
		name    string
		require *wrapperspb.BoolValue
	}{
		{"require_client_certificate: false", &wrapperspb.BoolValue{Value: false}},
		{"require_client_certificate absent (nil BoolValue)", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caPool := x509.NewCertPool()
			if !caPool.AppendCertsFromPEM(pki.caPEM) {
				t.Fatal("pki.caPEM: no certificates parsed")
			}
			cfg, err := NewDownstreamConfig(cvcDownstreamTS(t, tc.require), "", &fakeProvider{pool: caPool})
			if err != nil {
				t.Errorf("NewDownstreamConfig returned err %v — want nil err (verify-if-presented honors the anchor)", err)
			}
			if cfg == nil || cfg.TLSConfig == nil {
				t.Fatal("NewDownstreamConfig returned a nil cfg/TLSConfig")
			}
			if cfg.TLSConfig.ClientAuth != stdtls.VerifyClientCertIfGiven {
				t.Errorf("ClientAuth = %v, want VerifyClientCertIfGiven (never NoClientCert with an anchor)", cfg.TLSConfig.ClientAuth)
			}
		})
	}
}

// --- Phase 66 task 2: the four sub-field rejects under a combined_validation_context ---
//
// validation_context_type is a ONEOF, so under a CVC GetValidationContext()
// returns nil and the four sub-field rejects in commonTLSContextToConfig were
// BYPASSED. Task 1 lifted the CVC envelope to a NO-OP for downstream+provider;
// without an effective-inline-context selector that lift would SILENTLY ACCEPT
// sub-fields envoy-go cannot honor. Each test below drives a CVC whose
// default_validation_context carries one offending sub-field.

// cvcPKIPool builds the CA pool the live fake provider hands back.
func cvcPKIPool(t *testing.T) *x509.CertPool {
	t.Helper()
	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(pki.caPEM) {
		t.Fatal("pki.caPEM: no certificates parsed")
	}
	return p
}

// cvcWithDefaultVC returns a well-formed CVC ctc whose default_validation_context
// has been mutated by f. The ctc keeps its tls_certificates, or the build would
// error `no tls_certificates configured` and prove nothing.
func cvcWithDefaultVC(f func(*tlsv3.CertificateValidationContext)) *tlsv3.CommonTlsContext {
	return cvcCTC(func(c *tlsv3.CommonTlsContext_CombinedCertificateValidationContext) {
		f(c.DefaultValidationContext)
	})
}

func TestCVC_DefaultVC_CustomValidatorConfig_Rejected(t *testing.T) {
	ctc := cvcWithDefaultVC(func(vc *tlsv3.CertificateValidationContext) {
		vc.CustomValidatorConfig = &corev3.TypedExtensionConfig{Name: "x"}
	})
	_, err := commonTLSContextToConfig(ctc, "", "downstream", &fakeProvider{pool: cvcPKIPool(t)})
	if err == nil {
		t.Fatal("expected the custom_validator_config reject, got nil error — the CVC default_validation_context's sub-field was SILENTLY ACCEPTED")
	}
	if !strings.HasPrefix(err.Error(), "tls: ") {
		t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
	}
	const want = "custom_validator_config is not supported in phase 03"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

func TestCVC_DefaultVC_MatchTypedSAN_Rejected(t *testing.T) {
	ctc := cvcWithDefaultVC(func(vc *tlsv3.CertificateValidationContext) {
		vc.MatchTypedSubjectAltNames = []*tlsv3.SubjectAltNameMatcher{{}}
	})
	_, err := commonTLSContextToConfig(ctc, "", "downstream", &fakeProvider{pool: cvcPKIPool(t)})
	if err == nil {
		t.Fatal("expected the match_typed_subject_alt_names reject, got nil error — the CVC default_validation_context's sub-field was SILENTLY ACCEPTED")
	}
	if !strings.HasPrefix(err.Error(), "tls: ") {
		t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
	}
	const want = "match_typed_subject_alt_names is not supported in phase 03"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

func TestCVC_DefaultVC_VerifyCertHash_Rejected(t *testing.T) {
	ctc := cvcWithDefaultVC(func(vc *tlsv3.CertificateValidationContext) {
		vc.VerifyCertificateHash = []string{"x"}
	})
	_, err := commonTLSContextToConfig(ctc, "", "downstream", &fakeProvider{pool: cvcPKIPool(t)})
	if err == nil {
		t.Fatal("expected the verify_certificate_hash reject, got nil error — the CVC default_validation_context's sub-field was SILENTLY ACCEPTED")
	}
	if !strings.HasPrefix(err.Error(), "tls: ") {
		t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
	}
	const want = "verify_certificate_hash is not supported in phase 03"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

func TestCVC_DefaultVC_VerifyCertSpki_Rejected(t *testing.T) {
	ctc := cvcWithDefaultVC(func(vc *tlsv3.CertificateValidationContext) {
		vc.VerifyCertificateSpki = []string{"x"}
	})
	_, err := commonTLSContextToConfig(ctc, "", "downstream", &fakeProvider{pool: cvcPKIPool(t)})
	if err == nil {
		t.Fatal("expected the verify_certificate_spki reject, got nil error — the CVC default_validation_context's sub-field was SILENTLY ACCEPTED")
	}
	if !strings.HasPrefix(err.Error(), "tls: ") {
		t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
	}
	const want = "verify_certificate_spki is not supported in phase 03"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

// --- Phase 66 task 3: the apply-point — NewDownstreamConfig's CVC arm ---

// altCA generates a SECOND, independent CA (call it CA_Y) plus a leaf signed by
// it. pki's CA is CA_X. Two disjoint trust roots are what makes the equivalence
// theorem's observable DISCRIMINATING: a pool built by the theorem contains X
// and NOT Y, whereas a pool UNION (the rejected Design C) contains BOTH.
func altCA(t *testing.T) (caYPEM []byte, leafY *x509.Certificate) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("altCA: generate CA key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(101),
		Subject:               pkix.Name{CommonName: "envoy-go test CA Y"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("altCA: create CA cert: %v", err)
	}
	caYPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("altCA: generate leaf key: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(102),
		Subject:      pkix.Name{CommonName: "yankee.envoy-go.test"},
		DNSNames:     []string{"yankee.envoy-go.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("altCA: create leaf cert: %v", err)
	}
	leafY, err = x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("altCA: parse leaf: %v", err)
	}
	return caYPEM, leafY
}

// chainsTo reports whether leaf verifies against pool as a trust root. This is
// the DISCRIMINATING check used below instead of pool.Subjects(): Subjects() is
// deprecated (SA1019) and returns only RawSubject bytes, whereas an actual
// Verify exercises the pool the way the TLS stack will. A leaf signed by CA_N
// verifies against pool IFF CA_N is in pool — so running it for BOTH CA_X's leaf
// and CA_Y's leaf reads out set membership for each root independently.
// ExtKeyUsageAny + no DNSName keep the result a pure statement about the trust
// root, not about SAN/EKU policy.
func chainsTo(leaf *x509.Certificate, pool *x509.CertPool) bool {
	if pool == nil {
		return false
	}
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	return err == nil
}

// leafX parses pki's leaf (signed by CA_X) as an *x509.Certificate.
func leafX(t *testing.T) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(pki.leafCertPEM)
	if block == nil {
		t.Fatal("pki.leafCertPEM: PEM decode failed")
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse pki leaf: %v", err)
	}
	return c
}

// cvcDownstreamTSWith wraps a caller-supplied CVC-bearing CommonTlsContext in a
// DownstreamTlsContext + TransportSocket with require_client_certificate: true —
// the only shape that reaches the apply-point (E3 refuses the rest).
func cvcDownstreamTSWith(t *testing.T, ctc *tlsv3.CommonTlsContext) *corev3.TransportSocket {
	t.Helper()
	return makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
		RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
		CommonTlsContext:         ctc,
	})
}

// TestCVC_RequireTrue_InstallsSDSPoolAsClientCAs: the apply-point's happy path —
// the pool the provider serves IS the pool installed as ClientCAs, and mandatory
// mTLS is switched on.
func TestCVC_RequireTrue_InstallsSDSPoolAsClientCAs(t *testing.T) {
	served := cvcPKIPool(t)
	cfg, err := NewDownstreamConfig(cvcDownstreamTS(t, &wrapperspb.BoolValue{Value: true}), "", &fakeProvider{pool: served})
	if err != nil {
		t.Fatalf("NewDownstreamConfig: unexpected err %v", err)
	}
	if cfg.TLSConfig.ClientCAs != served {
		t.Errorf("ClientCAs = %p, want the SDS-served pool %p — the CVC arm did not install the fetched pool", cfg.TLSConfig.ClientCAs, served)
	}
	if cfg.TLSConfig.ClientAuth != stdtls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert (mandatory mTLS)", cfg.TLSConfig.ClientAuth)
	}
}

// TestCVC_ServedPoolWins_DefaultTrustedCaNotRead is THE EQUIVALENCE THEOREM'S
// OBSERVABLE, and it is a REFUTATION, not a happy path: the SDS-served pool
// (CA_X) wins outright and default_validation_context.trusted_ca (CA_Y) is NOT
// read. Design C — union the default's trusted_ca into the served pool — would
// leave CA_Y in the pool and is REFUTED by the second assertion below.
func TestCVC_ServedPoolWins_DefaultTrustedCaNotRead(t *testing.T) {
	caYPEM, certY := altCA(t)
	// The CVC's default_validation_context.trusted_ca is CA_Y; the provider serves
	// a pool holding ONLY CA_X.
	ctc := cvcWithDefaultVC(func(vc *tlsv3.CertificateValidationContext) {
		vc.TrustedCa = inlineBytes(caYPEM)
	})
	served := cvcPKIPool(t) // CA_X only
	cfg, err := NewDownstreamConfig(cvcDownstreamTSWith(t, ctc), "", &fakeProvider{pool: served})
	if err != nil {
		t.Fatalf("NewDownstreamConfig: unexpected err %v", err)
	}
	if !chainsTo(leafX(t), cfg.TLSConfig.ClientCAs) {
		t.Error("a leaf signed by CA_X does NOT verify against ClientCAs — the SDS-served pool was not installed")
	}
	if chainsTo(certY, cfg.TLSConfig.ClientCAs) {
		t.Error("a leaf signed by CA_Y VERIFIES against ClientCAs — default_validation_context.trusted_ca was read into the pool (this is Design C, the rejected pool-UNION; the equivalence theorem requires the served pool to win outright)")
	}
}

// TestCVC_MalformedSDSConfig_Rejected: the CVC's validation_context_sds_secret_config
// routes through xds.ParseSDSConfig, so a malformed sds_config (here: an
// envoy_grpc with no cluster_name) is REFUSED — a bare GetName() would silently
// accept it, since the NAME is perfectly well-formed. Two independent properties,
// two Errorf's.
func TestCVC_MalformedSDSConfig_Rejected(t *testing.T) {
	ctc := cvcCTC(func(c *tlsv3.CommonTlsContext_CombinedCertificateValidationContext) {
		c.ValidationContextSdsSecretConfig = sdsSecretConfig("validation-secret", "" /* no cluster_name */)
	})
	_, err := NewDownstreamConfig(cvcDownstreamTSWith(t, ctc), "", &fakeProvider{pool: cvcPKIPool(t)})
	// (i) it REJECTS at all.
	if err == nil {
		t.Error("NewDownstreamConfig returned nil — a malformed sds_config (no cluster_name) was SILENTLY ACCEPTED; the CVC arm is not routing through xds.ParseSDSConfig")
	}
	// (ii) the reject keeps the `tls: ` prefix FuzzTLSContextParse enforces: an
	// UNWRAPPED xds:-prefixed error would violate the invariant. Evaluated
	// independently of (i) — with no error at all there is no `tls: `-prefixed
	// reject either, so this property fails on its own terms.
	got := ""
	if err != nil {
		got = err.Error()
	}
	if !strings.HasPrefix(got, "tls: ") {
		t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", got)
	}
}

// TestCVC_GateRunsBeforeRequireBlock pins the ORDERING the apply-point's
// dereferences depend on: commonTLSContextToConfig runs FIRST and its error
// propagates BEFORE the require_client_certificate block, so a nil provider can
// never reach the CVC arm. The RETAINED phase-03 substring proves it was the gate
// that fired, not the arm's own defense-in-depth guard.
func TestCVC_GateRunsBeforeRequireBlock(t *testing.T) {
	_, err := NewDownstreamConfig(cvcDownstreamTS(t, &wrapperspb.BoolValue{Value: true}), "", nil)
	if err == nil {
		t.Fatal("expected the retained phase-03 reject, got nil")
	}
	if !strings.Contains(err.Error(), cvcRetainedReject) {
		t.Errorf("error = %q, want it to contain the RETAINED gate reject %q — the gate must run BEFORE the require_client_certificate block", err.Error(), cvcRetainedReject)
	}
}

// TestCVC_EmptyDynamicVC_BootFails: a provider whose fetch ERRORS — which is what
// parseValidationSecret does for a served validation context carrying no usable
// trusted_ca — boot-FAILS the listener.
//
// DEPARTURE (ADR-0280 family): the reference ACKs the empty dynamic context,
// falls back to the DEFAULT CA, and SERVES. envoy-go boot-FAILS instead. This is
// NOT "envoy-go rejects where the reference rejects" — the reference ACCEPTS this
// shape. envoy-go refuses LOUDLY rather than serving a listener whose trust roots
// are not the ones the operator's SDS server was asked for.
func TestCVC_EmptyDynamicVC_BootFails(t *testing.T) {
	fetchErr := errors.New("xds: sds: secret \"validation-secret\": validation context has no trusted_ca")
	_, err := NewDownstreamConfig(cvcDownstreamTS(t, &wrapperspb.BoolValue{Value: true}), "", &fakeProvider{vcErr: fetchErr})
	if err == nil {
		t.Fatal("expected a boot failure, got nil (the reference falls back to the default CA and SERVES; envoy-go boot-FAILS — ADR-0280 family)")
	}
	if !strings.HasPrefix(err.Error(), "tls: ") {
		t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
	}
	if !errors.Is(err, fetchErr) {
		t.Errorf("error = %q, want the provider's classified cause WRAPPED (errors.Is)", err.Error())
	}
	if !strings.Contains(err.Error(), "validation-secret") {
		t.Errorf("error = %q, want the secret name preserved for operator diagnosis", err.Error())
	}
}

// --- Phase 68 T2: the empty-dynamic fallback + empty-both routing ---

// cvcSentinelErr models the real dual-wrapped chain on the ONE bit config.go
// reads — errors.Is(err, xds.ErrEmptyValidationContext) — for a served
// validation_context whose trusted_ca is absent entirely (S1). The real chain
// (internal/xds's parseValidationSecret + applyValidationResponse, T1) also
// carries the unexported errValidation dual-%w'd alongside the sentinel; that
// second bit is not observable from this package and is not modeled here.
var cvcSentinelErr = fmt.Errorf("xds: sds: validation secret %q: %w", "x", xds.ErrEmptyValidationContext)

// TestNewDownstreamConfig_CVC_EmptyDynamic_FallsBackToDefault pins the T2
// fallback: an ACKed-but-empty dynamic fetch (the sentinel) with a CVC whose
// default_validation_context carries a valid trusted_ca falls back to that
// default and SERVES, at both require=true and require=false/absent.
func TestNewDownstreamConfig_CVC_EmptyDynamic_FallsBackToDefault(t *testing.T) {
	t.Run("require_client_certificate: true", func(t *testing.T) {
		ts := cvcDownstreamTS(t, &wrapperspb.BoolValue{Value: true})
		cfg, err := NewDownstreamConfig(ts, "", &fakeProvider{vcErr: cvcSentinelErr})
		if err != nil {
			t.Fatalf("NewDownstreamConfig: unexpected err %v — the sentinel-shaped empty-dynamic fetch must fall back to the default anchor and serve", err)
		}
		if cfg == nil || cfg.TLSConfig == nil {
			t.Fatal("NewDownstreamConfig returned a nil cfg/TLSConfig")
		}
		if cfg.TLSConfig.ClientCAs == nil {
			t.Error("ClientCAs is nil — the default_validation_context.trusted_ca fallback pool was not installed")
		}
		if cfg.TLSConfig.ClientAuth != stdtls.RequireAndVerifyClientCert {
			t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", cfg.TLSConfig.ClientAuth)
		}
	})

	for _, tc := range requireFalseCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := cvcDownstreamTS(t, tc.require)
			cfg, err := NewDownstreamConfig(ts, "", &fakeProvider{vcErr: cvcSentinelErr})
			if err != nil {
				t.Fatalf("NewDownstreamConfig: unexpected err %v — the sentinel-shaped empty-dynamic fetch must fall back to the default anchor and serve", err)
			}
			if cfg == nil || cfg.TLSConfig == nil {
				t.Fatal("NewDownstreamConfig returned a nil cfg/TLSConfig")
			}
			if cfg.TLSConfig.ClientCAs == nil {
				t.Error("ClientCAs is nil — the default_validation_context.trusted_ca fallback pool was not installed")
			}
			if cfg.TLSConfig.ClientAuth != stdtls.VerifyClientCertIfGiven {
				t.Errorf("ClientAuth = %v, want VerifyClientCertIfGiven", cfg.TLSConfig.ClientAuth)
			}
		})
	}
}

// TestNewDownstreamConfig_CVC_EmptyBoth_NoAnchor pins the empty-both routing:
// the sentinel-shaped empty-dynamic fetch WITH a CVC whose
// default_validation_context ALSO carries no trusted_ca routes through the
// phase-67 no-anchor logic exactly like the inline default arm — a require:true
// listener boot-rejects with the byte-identical :203 message; require:false/
// absent yields NoClientCert with no pool installed (no new reject).
func TestNewDownstreamConfig_CVC_EmptyBoth_NoAnchor(t *testing.T) {
	ctc := cvcCTC(func(c *tlsv3.CommonTlsContext_CombinedCertificateValidationContext) {
		c.DefaultValidationContext = &tlsv3.CertificateValidationContext{}
	})

	t.Run("require_client_certificate: true", func(t *testing.T) {
		ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext:         ctc,
		})
		cfg, err := NewDownstreamConfig(ts, "", &fakeProvider{vcErr: cvcSentinelErr})
		if err == nil {
			t.Fatal("expected the require+no-anchor reject, got nil")
		}
		const want = "require_client_certificate=true requires validation_context.trusted_ca"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain the byte-identical :203 message %q", err.Error(), want)
		}
		if cfg != nil {
			t.Errorf("cfg = %+v, want nil on the empty-both require:true reject", cfg)
		}
	})

	for _, tc := range requireFalseCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
				RequireClientCertificate: tc.require,
				CommonTlsContext:         ctc,
			})
			cfg, err := NewDownstreamConfig(ts, "", &fakeProvider{vcErr: cvcSentinelErr})
			if err != nil {
				t.Fatalf("NewDownstreamConfig: unexpected err %v — empty-both at false/absent must yield NoClientCert, not an error", err)
			}
			if cfg == nil || cfg.TLSConfig == nil {
				t.Fatal("NewDownstreamConfig returned a nil cfg/TLSConfig")
			}
			if cfg.TLSConfig.ClientAuth != stdtls.NoClientCert {
				t.Errorf("ClientAuth = %v, want NoClientCert (zero value)", cfg.TLSConfig.ClientAuth)
			}
			if cfg.TLSConfig.ClientCAs != nil {
				t.Error("ClientCAs is non-nil, want nil — empty-both must not install a pool")
			}
		})
	}
}

// TestNewDownstreamConfig_CVC_SetButEmpty_And_Corrupt_BootFail pins the NARROW
// gate at the consumer: a fetch error that does NOT carry
// xds.ErrEmptyValidationContext (a set-but-empty trusted_ca specifier, a
// corrupt served PEM, a timeout, or an unreachable management server) does NOT
// fall back — it boot-FAILS, even though the CVC's default_validation_context
// carries a VALID trusted_ca. Building the CVC with a valid default (rather
// than an empty one) is deliberate (M3): it is what makes this test
// DISCRIMINATE Break C (widening errors.Is to fire on any error), because an
// empty default would boot-FAIL via the empty-both branch regardless of the
// gate. This test is expected ALREADY GREEN before the fallback branch exists
// (the pre-T2 code boot-FAILs on every fetch error) — a regression pin, no red
// owed.
func TestNewDownstreamConfig_CVC_SetButEmpty_And_Corrupt_BootFail(t *testing.T) {
	nonSentinelErr := errors.New("xds: sds: validation secret \"x\": trusted_ca: parse failure")
	ts := cvcDownstreamTS(t, &wrapperspb.BoolValue{Value: true})
	cfg, err := NewDownstreamConfig(ts, "", &fakeProvider{vcErr: nonSentinelErr})
	if err == nil {
		t.Fatal("expected a boot failure for a non-sentinel fetch error, got nil — the fallback fired where it must not")
	}
	if cfg != nil {
		t.Errorf("cfg = %+v, want nil on a boot failure", cfg)
	}
}

// TestNewDownstreamConfig_CVC_EmptyDynamic_RequireEnforcedAgainstFallback pins
// the require x fallback cross-product at the unit level: require=true +
// sentinel + a valid default yields RequireAndVerifyClientCert with a non-nil
// ClientCAs pool. The wire-level CA_B-reject / no-cert-reject behavior against
// this fallback anchor is the fixture's job (T5), not this unit test's.
func TestNewDownstreamConfig_CVC_EmptyDynamic_RequireEnforcedAgainstFallback(t *testing.T) {
	ts := cvcDownstreamTS(t, &wrapperspb.BoolValue{Value: true})
	cfg, err := NewDownstreamConfig(ts, "", &fakeProvider{vcErr: cvcSentinelErr})
	if err != nil {
		t.Fatalf("NewDownstreamConfig: unexpected err %v", err)
	}
	if cfg == nil || cfg.TLSConfig == nil {
		t.Fatal("NewDownstreamConfig returned a nil cfg/TLSConfig")
	}
	if cfg.TLSConfig.ClientAuth != stdtls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", cfg.TLSConfig.ClientAuth)
	}
	if cfg.TLSConfig.ClientCAs == nil {
		t.Error("ClientCAs is nil, want the installed fallback pool")
	}
}

// TestSDSVC_And_Inline_Paths_Unchanged: the 3-way branch must not regress either
// landed arm. The SDS-VC arm (phase 65) still fetches and installs; the inline
// arm (phase 16) still loads trusted_ca and still refuses a require:true listener
// with no trusted_ca, with its BYTE-IDENTICAL message.
func TestSDSVC_And_Inline_Paths_Unchanged(t *testing.T) {
	t.Run("SDS-VC arm still installs the served pool", func(t *testing.T) {
		served := cvcPKIPool(t)
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
		cfg, err := NewDownstreamConfig(ts, "", &fakeProvider{pool: served})
		if err != nil {
			t.Fatalf("NewDownstreamConfig: unexpected err %v", err)
		}
		if cfg.TLSConfig.ClientCAs != served {
			t.Errorf("ClientCAs = %p, want the SDS-served pool %p", cfg.TLSConfig.ClientCAs, served)
		}
		if cfg.TLSConfig.ClientAuth != stdtls.RequireAndVerifyClientCert {
			t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", cfg.TLSConfig.ClientAuth)
		}
	})

	t.Run("inline arm still loads trusted_ca", func(t *testing.T) {
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
		// A nil provider is deliberate: the inline arm must not have acquired an SDS
		// dependency from the 3-way split.
		cfg, err := NewDownstreamConfig(ts, "", nil)
		if err != nil {
			t.Fatalf("NewDownstreamConfig: unexpected err %v", err)
		}
		if !chainsTo(leafX(t), cfg.TLSConfig.ClientCAs) {
			t.Error("a leaf signed by CA_X does not verify against ClientCAs — the inline trusted_ca was not loaded")
		}
		if cfg.TLSConfig.ClientAuth != stdtls.RequireAndVerifyClientCert {
			t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", cfg.TLSConfig.ClientAuth)
		}
	})

	t.Run("inline arm still refuses require:true with no trusted_ca (byte-identical)", func(t *testing.T) {
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
			t.Fatal("expected the trusted_ca reject, got nil")
		}
		const want = "tls: downstream: require_client_certificate=true requires validation_context.trusted_ca"
		if err.Error() != want {
			t.Errorf("error = %q, want %q (byte-identical)", err.Error(), want)
		}
	})
}

// TestInlineVC_FourRejects_Unchanged: the PLAIN validation_context path still
// rejects all four sub-fields — the effective-inline-context selector must not
// regress the inline half it shares its error strings with.
func TestInlineVC_FourRejects_Unchanged(t *testing.T) {
	inlineCTC := func(f func(*tlsv3.CertificateValidationContext)) *tlsv3.CommonTlsContext {
		vc := &tlsv3.CertificateValidationContext{TrustedCa: inlineBytes(pki.caPEM)}
		f(vc)
		return &tlsv3.CommonTlsContext{
			TlsCertificates: []*tlsv3.TlsCertificate{
				{
					CertificateChain: inlineBytes(pki.leafCertPEM),
					PrivateKey:       inlineBytes(pki.leafKeyPEM),
				},
			},
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{ValidationContext: vc},
		}
	}
	tests := []struct {
		name string
		mut  func(*tlsv3.CertificateValidationContext)
		want string
	}{
		{
			name: "custom_validator_config",
			mut: func(vc *tlsv3.CertificateValidationContext) {
				vc.CustomValidatorConfig = &corev3.TypedExtensionConfig{Name: "x"}
			},
			want: "custom_validator_config is not supported in phase 03",
		},
		{
			name: "match_typed_subject_alt_names",
			mut: func(vc *tlsv3.CertificateValidationContext) {
				vc.MatchTypedSubjectAltNames = []*tlsv3.SubjectAltNameMatcher{{}}
			},
			want: "match_typed_subject_alt_names is not supported in phase 03",
		},
		{
			name: "verify_certificate_hash",
			mut:  func(vc *tlsv3.CertificateValidationContext) { vc.VerifyCertificateHash = []string{"x"} },
			want: "verify_certificate_hash is not supported in phase 03",
		},
		{
			name: "verify_certificate_spki",
			mut:  func(vc *tlsv3.CertificateValidationContext) { vc.VerifyCertificateSpki = []string{"x"} },
			want: "verify_certificate_spki is not supported in phase 03",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := commonTLSContextToConfig(inlineCTC(tc.mut), "", "downstream", nil)
			if err == nil {
				t.Fatalf("expected the %s reject on the inline validation_context path, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Phase 67 (SPEC §10): the require_client_certificate false/absent mapping
// cross-product — inline / SDS-VC / CVC (T1) shapes, each at {false, absent} —
// plus the anchorless regression pin, the corrupt-CA boot-error pin, and the
// §3.6 nil-pool unconstructibility property.
// ---------------------------------------------------------------------------

// requireFalseCases is the {false, absent} pair shared by every phase-67
// mapping test: GetValue() on a nil *wrapperspb.BoolValue returns false, so
// require:false and require:absent coincide (no tri-state).
var requireFalseCases = []struct {
	name    string
	require *wrapperspb.BoolValue
}{
	{"require_client_certificate: false", &wrapperspb.BoolValue{Value: false}},
	{"require_client_certificate absent (nil BoolValue)", nil},
}

// TestNewDownstreamConfig_RequireFalse_Inline_VerifyIfGiven pins the phase-67
// hoist for the PLAIN inline validation_context.trusted_ca arm:
// require_client_certificate false/absent with an inline anchor now HONORS
// the anchor (verify-if-presented) instead of leaving it inert.
func TestNewDownstreamConfig_RequireFalse_Inline_VerifyIfGiven(t *testing.T) {
	for _, tc := range requireFalseCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
				RequireClientCertificate: tc.require,
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
				t.Errorf("NewDownstreamConfig: unexpected err %v — false/absent + inline anchor must verify-if-presented", err)
			}
			if cfg == nil || cfg.TLSConfig == nil {
				t.Fatal("NewDownstreamConfig returned a nil cfg/TLSConfig")
			}
			if cfg.TLSConfig.ClientAuth != stdtls.VerifyClientCertIfGiven {
				t.Errorf("ClientAuth = %v, want VerifyClientCertIfGiven", cfg.TLSConfig.ClientAuth)
			}
			if cfg.TLSConfig.ClientCAs == nil {
				t.Error("ClientCAs is nil — the inline trusted_ca anchor must be installed at false/absent")
			}
		})
	}
}

// sdsVCDownstreamTS builds a DownstreamTlsContext carrying a single leaf
// tls_certificate plus an SDS-delivered validation_context (phase 65,
// ADR-0286), with require_client_certificate set to require (nil => absent).
func sdsVCDownstreamTS(t *testing.T, require *wrapperspb.BoolValue) *corev3.TransportSocket {
	t.Helper()
	return makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
		RequireClientCertificate: require,
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
}

// TestNewDownstreamConfig_RequireFalse_SDSVC_VerifyIfGiven pins the phase-67
// hoist for the SDS-delivered validation_context arm (phase 65, ADR-0286):
// require_client_certificate false/absent now HONORS the SDS-fetched anchor
// (verify-if-presented) rather than leaving it inert. It ALSO pins the §3.5
// departure directly: a require=false SDS validation_context fetch FAILURE
// now boot-FAILS the listener (it is no longer inert just because
// require_client_certificate is false) — the fetch is UN-GATED (T1). No
// deliberate break is owed for the fetch-failure arm; it is a green-stable
// pin whose integration twin is TestSDSEndToEnd_FetchFailure_BootFailsClosed.
func TestNewDownstreamConfig_RequireFalse_SDSVC_VerifyIfGiven(t *testing.T) {
	for _, tc := range requireFalseCases {
		t.Run(tc.name, func(t *testing.T) {
			caPool := x509.NewCertPool()
			if !caPool.AppendCertsFromPEM(pki.caPEM) {
				t.Fatal("pki.caPEM: no certificates parsed")
			}
			cfg, err := NewDownstreamConfig(sdsVCDownstreamTS(t, tc.require), "", &fakeProvider{pool: caPool})
			if err != nil {
				t.Errorf("NewDownstreamConfig: unexpected err %v — false/absent + SDS anchor must verify-if-presented", err)
			}
			if cfg == nil || cfg.TLSConfig == nil {
				t.Fatal("NewDownstreamConfig returned a nil cfg/TLSConfig")
			}
			if cfg.TLSConfig.ClientCAs == nil {
				t.Error("ClientCAs is nil — the SDS fetch did not fire / the pool was not installed")
			}
			if cfg.TLSConfig.ClientAuth != stdtls.VerifyClientCertIfGiven {
				t.Errorf("ClientAuth = %v, want VerifyClientCertIfGiven", cfg.TLSConfig.ClientAuth)
			}
		})
	}

	t.Run("fetch failure propagates at require=false (§3.5 departure)", func(t *testing.T) {
		for _, tc := range requireFalseCases {
			t.Run(tc.name, func(t *testing.T) {
				fetchErr := errors.New("xds: sds: secret \"validation-secret\": initial fetch timed out after 15s: context deadline exceeded")
				cfg, err := NewDownstreamConfig(sdsVCDownstreamTS(t, tc.require), "", &fakeProvider{vcErr: fetchErr})
				if err == nil {
					t.Fatal("expected a boot failure, got nil — require=false must not mask an SDS fetch failure (§3.5)")
				}
				if !strings.HasPrefix(err.Error(), "tls: ") {
					t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
				}
				if !strings.Contains(err.Error(), "initial fetch timed out") {
					t.Errorf("error = %q, want the provider's classified cause preserved", err.Error())
				}
				if cfg != nil {
					t.Errorf("cfg = %+v, want nil on a boot failure", cfg)
				}
			})
		}
	})
}

// TestNewDownstreamConfig_RequireFalse_Anchorless_NoClientCert is a
// green-stable REGRESSION PIN: this cell (no anchor + require false/absent)
// is UNCHANGED by the phase-67 hoist — no break is owed. Two anchorless
// shapes are covered: no validation config at all, and a present-but-empty
// validation_context carrying no trusted_ca.
func TestNewDownstreamConfig_RequireFalse_Anchorless_NoClientCert(t *testing.T) {
	shapes := []struct {
		name    string
		withVCT func(*tlsv3.CommonTlsContext)
	}{
		{"no validation config at all", func(c *tlsv3.CommonTlsContext) {}},
		{"anchorless validation_context (empty, no trusted_ca)", func(c *tlsv3.CommonTlsContext) {
			c.ValidationContextType = &tlsv3.CommonTlsContext_ValidationContext{
				ValidationContext: &tlsv3.CertificateValidationContext{},
			}
		}},
	}
	for _, shape := range shapes {
		for _, tc := range requireFalseCases {
			t.Run(shape.name+"/"+tc.name, func(t *testing.T) {
				common := &tlsv3.CommonTlsContext{
					TlsCertificates: []*tlsv3.TlsCertificate{
						{
							CertificateChain: inlineBytes(pki.leafCertPEM),
							PrivateKey:       inlineBytes(pki.leafKeyPEM),
						},
					},
				}
				shape.withVCT(common)
				ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
					RequireClientCertificate: tc.require,
					CommonTlsContext:         common,
				})
				cfg, err := NewDownstreamConfig(ts, "", nil)
				if err != nil {
					t.Errorf("NewDownstreamConfig: unexpected err %v", err)
				}
				if cfg == nil || cfg.TLSConfig == nil {
					t.Fatal("NewDownstreamConfig returned a nil cfg/TLSConfig")
				}
				if cfg.TLSConfig.ClientAuth != stdtls.NoClientCert {
					t.Errorf("ClientAuth = %v, want NoClientCert (zero value)", cfg.TLSConfig.ClientAuth)
				}
				if cfg.TLSConfig.ClientCAs != nil {
					t.Error("ClientCAs is non-nil; want nil (no anchor was configured)")
				}
			})
		}
	}
}

// TestInlineCorruptTrustedCA_RequireFalse_BootError is a DECISION on
// envoy-go's strict posture (SPEC §3.12(1)), NOT a parity claim — the
// reference's corrupt-CA config-validate posture was NOT probed here. A
// corrupt inline trusted_ca PEM must boot-FAIL even at
// require_client_certificate=false/absent — the un-gated (T1) load must not
// be silently swallowed just because require is false.
func TestInlineCorruptTrustedCA_RequireFalse_BootError(t *testing.T) {
	for _, tc := range requireFalseCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := makeTransportSocket(t, &tlsv3.DownstreamTlsContext{
				RequireClientCertificate: tc.require,
				CommonTlsContext: &tlsv3.CommonTlsContext{
					TlsCertificates: []*tlsv3.TlsCertificate{
						{
							CertificateChain: inlineBytes(pki.leafCertPEM),
							PrivateKey:       inlineBytes(pki.leafKeyPEM),
						},
					},
					ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
						ValidationContext: &tlsv3.CertificateValidationContext{
							TrustedCa: inlineBytes([]byte("not a pem")),
						},
					},
				},
			})
			cfg, err := NewDownstreamConfig(ts, "", nil)
			if err == nil {
				t.Fatal("expected a boot error for a corrupt trusted_ca, got nil")
			}
			if !strings.HasPrefix(err.Error(), "tls: ") {
				t.Errorf("error = %q, want the `tls: ` prefix (the FuzzTLSContextParse invariant)", err.Error())
			}
			if cfg != nil {
				t.Errorf("cfg = %+v, want nil on a boot error", cfg)
			}
		})
	}
}

// compile-time interface pin (§3.6): fakeProvider must satisfy xds.SecretProvider
// so TestVerifyIfGiven_NilPool_Unconstructible's (nil,nil) arm is a legitimate
// value of the SAME interface production fetchers implement — not a shape
// only a test double could return.
var _ xds.SecretProvider = &fakeProvider{}

// TestVerifyIfGiven_NilPool_Unconstructible pins §3.6: NO config ×
// provider-behavior combination may yield the forbidden state
// ClientAuth==VerifyClientCertIfGiven && ClientCAs==nil.
// VerifyClientCertIfGiven + a nil ClientCAs pool makes Go's crypto/tls fall
// back to the SYSTEM root pool — rejecting the legitimate anchor-signed
// client while ADMITTING anonymous ones (reference_go_client_cert_withholding):
// the worst-direction failure mode, silently weaker than either NoClientCert
// or a boot failure. installPool's nil-pool guard (config.go) is the only
// thing standing between a (nil, nil) FetchInitialValidationContext return
// and that forbidden state.
//
// This test's liveness is NOT provable by a red-first run: no live
// production fetcher (SDS server, CVC) can itself return (nil, nil) without
// an error, so the guard is unreachable from today's entry points under
// normal input. Its ONLY liveness proof is a deliberate break (T2 Break D)
// that deletes the installPool nil-pool guard — see the T2 report for the
// break's execution.
func TestVerifyIfGiven_NilPool_Unconstructible(t *testing.T) {
	validPool := x509.NewCertPool()
	if !validPool.AppendCertsFromPEM(pki.caPEM) {
		t.Fatal("pki.caPEM: no certificates parsed")
	}

	providers := []struct {
		name string
		fp   *fakeProvider
	}{
		{"success (non-nil pool, nil error)", &fakeProvider{pool: validPool}},
		{"nil pool, nil error (the hazard arm)", &fakeProvider{}},
		{"fetch error (non-nil err)", &fakeProvider{vcErr: errors.New("boom")}},
	}

	shapes := []struct {
		name string
		ts   func(t *testing.T, require *wrapperspb.BoolValue) *corev3.TransportSocket
	}{
		{"SDS-VC", func(t *testing.T, require *wrapperspb.BoolValue) *corev3.TransportSocket {
			return sdsVCDownstreamTS(t, require)
		}},
		{"CVC", func(t *testing.T, require *wrapperspb.BoolValue) *corev3.TransportSocket {
			return cvcDownstreamTS(t, require)
		}},
	}

	for _, shape := range shapes {
		for _, tc := range requireFalseCases {
			for _, p := range providers {
				t.Run(shape.name+"/"+tc.name+"/"+p.name, func(t *testing.T) {
					cfg, err := NewDownstreamConfig(shape.ts(t, tc.require), "", p.fp)

					// The property, independent of which arm produced it: NEVER
					// ClientAuth==VerifyClientCertIfGiven with ClientCAs==nil.
					if cfg != nil && cfg.TLSConfig != nil &&
						cfg.TLSConfig.ClientAuth == stdtls.VerifyClientCertIfGiven &&
						cfg.TLSConfig.ClientCAs == nil {
						t.Errorf("forbidden state reached: ClientAuth=VerifyClientCertIfGiven, ClientCAs=nil (shape=%s, provider=%s)", shape.name, p.name)
					}

					// The (nil, nil) hazard arm specifically: the installPool
					// nil-pool guard must turn this into an error, never a cfg.
					if p.fp.pool == nil && p.fp.vcErr == nil {
						if err == nil {
							t.Errorf("(nil,nil) fetch: expected an error from the installPool nil-pool guard, got nil (cfg=%+v)", cfg)
						}
					}
				})
			}
		}
	}
}
