package tls

import (
	stdtls "crypto/tls"
	"crypto/x509"
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
)

const (
	downstreamTLSContextTypeURL = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext"
	upstreamTLSContextTypeURL   = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext"
)

// DownstreamConfig is the phase-03 output of parsing a DownstreamTlsContext.
// Callers embed cfg.TLSConfig in the chain's per-chain *stdtls.Config used by
// the listener's GetConfigForClient callback.
type DownstreamConfig struct {
	TLSConfig *stdtls.Config
}

// UpstreamConfig is the phase-03 output of parsing an UpstreamTlsContext.
// Callers use cfg.TLSConfig with stdtls.Client for each upstream dial.
type UpstreamConfig struct {
	TLSConfig *stdtls.Config
	SNI       string
}

// NewDownstreamConfig parses a *corev3.TransportSocket whose typed_config is a
// DownstreamTlsContext. baseDir is used to resolve filename-based DataSources.
// Errors begin with "tls: downstream: ".
func NewDownstreamConfig(ts *corev3.TransportSocket, baseDir string) (*DownstreamConfig, error) {
	if ts == nil {
		return nil, fmt.Errorf("tls: downstream: nil transport_socket")
	}
	if ts.GetTypedConfig() == nil || ts.GetTypedConfig().GetTypeUrl() != downstreamTLSContextTypeURL {
		return nil, fmt.Errorf("tls: downstream: unexpected type_url %q", ts.GetTypedConfig().GetTypeUrl())
	}
	ctx := &tlsv3.DownstreamTlsContext{}
	if err := ts.GetTypedConfig().UnmarshalTo(ctx); err != nil {
		return nil, fmt.Errorf("tls: downstream: unmarshal: %w", err)
	}
	if ctx.GetRequireClientCertificate().GetValue() {
		return nil, fmt.Errorf("tls: downstream: require_client_certificate is not supported in phase 03")
	}
	cfg, err := commonTLSContextToConfig(ctx.GetCommonTlsContext(), baseDir, "downstream")
	if err != nil {
		return nil, err
	}
	return &DownstreamConfig{TLSConfig: cfg}, nil
}

// NewUpstreamConfig parses a *corev3.TransportSocket whose typed_config is an
// UpstreamTlsContext. baseDir is used to resolve filename-based DataSources.
// Errors begin with "tls: upstream: ".
func NewUpstreamConfig(ts *corev3.TransportSocket, baseDir string) (*UpstreamConfig, error) {
	if ts == nil {
		return nil, fmt.Errorf("tls: upstream: nil transport_socket")
	}
	if ts.GetTypedConfig() == nil || ts.GetTypedConfig().GetTypeUrl() != upstreamTLSContextTypeURL {
		return nil, fmt.Errorf("tls: upstream: unexpected type_url %q", ts.GetTypedConfig().GetTypeUrl())
	}
	ctx := &tlsv3.UpstreamTlsContext{}
	if err := ts.GetTypedConfig().UnmarshalTo(ctx); err != nil {
		return nil, fmt.Errorf("tls: upstream: unmarshal: %w", err)
	}
	if ctx.GetAllowRenegotiation() {
		return nil, fmt.Errorf("tls: upstream: allow_renegotiation is not supported (crypto/tls does not support TLS 1.2 renegotiation as a client)")
	}
	common := ctx.GetCommonTlsContext()
	if common == nil {
		return nil, fmt.Errorf("tls: upstream: common_tls_context is required")
	}

	// Enforce §5.4 tightening: trusted_ca required on every upstream TLS cluster.
	vc := common.GetValidationContext()
	if vc == nil || vc.GetTrustedCa() == nil {
		return nil, fmt.Errorf("tls: upstream: validation_context.trusted_ca is required (phase 03 does not permit unvalidated upstream TLS)")
	}

	cfg, err := commonTLSContextToConfig(common, baseDir, "upstream")
	if err != nil {
		return nil, err
	}

	// Load CA into RootCAs pool.
	caPEM, err := loadDataSource(vc.GetTrustedCa(), baseDir)
	if err != nil {
		return nil, fmt.Errorf("tls: upstream: validation_context: trusted_ca: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("tls: upstream: validation_context: trusted_ca: parse failure")
	}
	cfg.RootCAs = pool
	cfg.ServerName = ctx.GetSni()
	return &UpstreamConfig{TLSConfig: cfg, SNI: ctx.GetSni()}, nil
}

// commonTLSContextToConfig builds a *stdtls.Config carrying Certificates (from
// tls_certificates[]) and NextProtos (from alpn_protocols), plus
// tls_params-mapped fields. side is "downstream" or "upstream" and prefixes
// every error.
//
// Phase-03 forbids the following; each errors with a clear message:
//   - tls_certificate_sds_secret_configs set
//   - validation_context_sds_secret_config set
//   - combined_validation_context set
//   - custom_validator_config set
//   - match_typed_subject_alt_names set
//   - verify_certificate_hash / verify_certificate_spki set
//   - password on key
func commonTLSContextToConfig(c *tlsv3.CommonTlsContext, baseDir, side string) (*stdtls.Config, error) {
	if c == nil {
		return nil, fmt.Errorf("tls: %s: common_tls_context is required", side)
	}
	if len(c.GetTlsCertificateSdsSecretConfigs()) > 0 {
		return nil, fmt.Errorf("tls: %s: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03", side)
	}
	if c.GetValidationContextType() != nil {
		switch c.GetValidationContextType().(type) {
		case *tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig:
			return nil, fmt.Errorf("tls: %s: SDS-bound validation_context_sds_secret_config is not supported in phase 03", side)
		case *tlsv3.CommonTlsContext_CombinedValidationContext:
			return nil, fmt.Errorf("tls: %s: combined_validation_context is not supported in phase 03", side)
		}
	}
	if vc := c.GetValidationContext(); vc != nil {
		if vc.GetCustomValidatorConfig() != nil {
			return nil, fmt.Errorf("tls: %s: custom_validator_config is not supported in phase 03", side)
		}
		if len(vc.GetMatchTypedSubjectAltNames()) > 0 {
			return nil, fmt.Errorf("tls: %s: match_typed_subject_alt_names is not supported in phase 03", side)
		}
		if len(vc.GetVerifyCertificateHash()) > 0 {
			return nil, fmt.Errorf("tls: %s: verify_certificate_hash is not supported in phase 03", side)
		}
		if len(vc.GetVerifyCertificateSpki()) > 0 {
			return nil, fmt.Errorf("tls: %s: verify_certificate_spki is not supported in phase 03", side)
		}
	}

	cfg := &stdtls.Config{}

	for i, tc := range c.GetTlsCertificates() {
		if tc.GetPassword() != nil {
			return nil, fmt.Errorf("tls: %s: tls_certificates[%d]: password-protected keys are not supported in phase 03", side, i)
		}
		certPEM, err := loadDataSource(tc.GetCertificateChain(), baseDir)
		if err != nil {
			return nil, fmt.Errorf("tls: %s: tls_certificates[%d]: certificate_chain: %w", side, i, err)
		}
		keyPEM, err := loadDataSource(tc.GetPrivateKey(), baseDir)
		if err != nil {
			return nil, fmt.Errorf("tls: %s: tls_certificates[%d]: private_key: %w", side, i, err)
		}
		pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("tls: %s: tls_certificates[%d]: load cert: %w", side, i, err)
		}
		cfg.Certificates = append(cfg.Certificates, pair)
	}

	if side == "downstream" && len(cfg.Certificates) == 0 {
		return nil, fmt.Errorf("tls: downstream: no tls_certificates configured")
	}

	if err := applyTLSParams(cfg, c.GetTlsParams()); err != nil {
		return nil, fmt.Errorf("tls: %s: %w", side, err)
	}

	cfg.NextProtos = append(cfg.NextProtos, c.GetAlpnProtocols()...)

	return cfg, nil
}
