package tls

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	quicv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/quic/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"

	xds "github.com/pgdad/envoy-go/internal/xds"
)

const (
	downstreamTLSContextTypeURL    = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext"
	upstreamTLSContextTypeURL      = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext"
	quicDownstreamTransportTypeURL = "type.googleapis.com/envoy.extensions.transport_sockets.quic.v3.QuicDownstreamTransport"
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
// provider (phase 60.2, ADR-0280) is consulted when the common_tls_context
// carries a tls_certificate_sds_secret_configs entry — see
// commonTLSContextToConfig. Errors begin with "tls: downstream: ".
func NewDownstreamConfig(ts *corev3.TransportSocket, baseDir string, provider xds.SecretProvider) (*DownstreamConfig, error) {
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
	cfg, err := commonTLSContextToConfig(ctx.GetCommonTlsContext(), baseDir, "downstream", provider)
	if err != nil {
		return nil, err
	}
	// Phase-16 ADR-0147 (unanticipated): when require_client_certificate is
	// true, configure the downstream listener for mandatory mTLS — load the
	// validation_context.trusted_ca into the ClientCAs pool and set
	// ClientAuth=RequireAndVerifyClientCert. This lifts the phase-03 clause-7
	// rejection (ADR-0032 §Decision (7)) to support fixture 0018's scenario 6
	// (ADR-0144 framework primitive end-to-end validation). The lift is
	// SCOPED — only require_client_certificate=true with a validation_context
	// carrying a trusted_ca PEM is accepted; the previously parse-rejected
	// surfaces (SDS-bound secrets, custom_validator_config, match_typed_san,
	// verify_certificate_hash/spki) remain rejected via the unchanged
	// commonTLSContextToConfig pre-checks.
	if ctx.GetRequireClientCertificate().GetValue() {
		common := ctx.GetCommonTlsContext()
		vc := common.GetValidationContext()
		if vc == nil || vc.GetTrustedCa() == nil {
			return nil, fmt.Errorf("tls: downstream: require_client_certificate=true requires validation_context.trusted_ca")
		}
		pool, err := loadTrustedCAPool(vc, baseDir, "downstream")
		if err != nil {
			return nil, err
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = stdtls.RequireAndVerifyClientCert
	}
	return &DownstreamConfig{TLSConfig: cfg}, nil
}

// NewQUICDownstreamConfig parses a *corev3.TransportSocket whose typed_config is
// a QuicDownstreamTransport (envoy.transport_sockets.quic), unwraps the inner
// DownstreamTlsContext, and builds the *stdtls.Config via the SHARED
// commonTLSContextToConfig (so ALPN from alpn_protocols, cert loading, and the
// mandatory-TLS empty-cert error are reused). QUIC carries no SDS in phase 61.1,
// so the provider argument to commonTLSContextToConfig is nil. Errors begin with
// "tls: downstream: ". (Phase 61.1, ADR-0279.)
func NewQUICDownstreamConfig(ts *corev3.TransportSocket, baseDir string) (*DownstreamConfig, error) {
	if ts == nil {
		return nil, fmt.Errorf("tls: downstream: nil transport_socket")
	}
	if ts.GetTypedConfig() == nil || ts.GetTypedConfig().GetTypeUrl() != quicDownstreamTransportTypeURL {
		return nil, fmt.Errorf("tls: downstream: unexpected quic transport_socket type_url %q", ts.GetTypedConfig().GetTypeUrl())
	}
	qt := &quicv3.QuicDownstreamTransport{}
	if err := ts.GetTypedConfig().UnmarshalTo(qt); err != nil {
		return nil, fmt.Errorf("tls: downstream: quic transport_socket unmarshal: %w", err)
	}
	if qt.GetEnableEarlyData().GetValue() {
		return nil, fmt.Errorf("tls: downstream: quic enable_early_data (0-RTT) is not supported in phase 61.1")
	}
	dtc := qt.GetDownstreamTlsContext()
	if dtc == nil {
		return nil, fmt.Errorf("tls: downstream: quic transport_socket has no downstream_tls_context")
	}
	cfg, err := commonTLSContextToConfig(dtc.GetCommonTlsContext(), baseDir, "downstream", nil)
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

	cfg, err := commonTLSContextToConfig(common, baseDir, "upstream", nil)
	if err != nil {
		return nil, err
	}

	// Load CA into RootCAs pool.
	pool, err := loadTrustedCAPool(vc, baseDir, "upstream")
	if err != nil {
		return nil, err
	}
	cfg.RootCAs = pool
	cfg.ServerName = ctx.GetSni()
	return &UpstreamConfig{TLSConfig: cfg, SNI: ctx.GetSni()}, nil
}

// loadTrustedCAPool loads validation_context.trusted_ca into a fresh
// *x509.CertPool. Shared by NewDownstreamConfig (ClientCAs for mandatory
// mTLS) and NewUpstreamConfig (RootCAs). side is "downstream" or "upstream"
// and prefixes every error; the error strings are byte-identical to the
// previously-inlined copies at both call sites.
func loadTrustedCAPool(vc *tlsv3.CertificateValidationContext, baseDir, side string) (*x509.CertPool, error) {
	caPEM, err := loadDataSource(vc.GetTrustedCa(), baseDir)
	if err != nil {
		return nil, fmt.Errorf("tls: %s: validation_context: trusted_ca: %w", side, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("tls: %s: validation_context: trusted_ca: parse failure", side)
	}
	return pool, nil
}

// commonTLSContextToConfig builds a *stdtls.Config carrying Certificates (from
// tls_certificates[] and, downstream-only, an SDS-delivered leaf) and
// NextProtos (from alpn_protocols), plus tls_params-mapped fields. side is
// "downstream" or "upstream" and prefixes every error. provider is the
// phase-60.2 (ADR-0280) SDS seam — consulted only when side=="downstream" and
// the config carries a tls_certificate_sds_secret_configs entry; nil for
// upstream/validate callers, which never fetch SDS.
//
// Phase-03 forbids the following; each errors with a clear message:
//   - tls_certificate_sds_secret_configs set (upstream/validate only — see
//     the ADR-0280 downstream lift below)
//   - validation_context_sds_secret_config set
//   - combined_validation_context set
//   - custom_validator_config set
//   - match_typed_subject_alt_names set
//   - verify_certificate_hash / verify_certificate_spki set
//   - password on key
func commonTLSContextToConfig(c *tlsv3.CommonTlsContext, baseDir, side string, provider xds.SecretProvider) (*stdtls.Config, error) {
	if c == nil {
		return nil, fmt.Errorf("tls: %s: common_tls_context is required", side)
	}

	// SDS-bound downstream server certificate (phase 60.2, ADR-0280). The
	// upstream/validate paths keep the byte-identical reject (arm 6). Downstream
	// with a provider present: validate the SdsSecretConfig (arms 1-4,8,9 live in
	// xds.ParseSDSConfig), then BLOCK on the first Secret (bounded by
	// initial_fetch_timeout inside the provider) and hold the built leaf; it is
	// appended to cfg.Certificates after cfg is created below. On timeout /
	// mgmt-unreachable the provider returns a classified error that PROPAGATES
	// here → the listener build FAILS → boot FAILS (the documented envoy-go
	// DEPARTURE from the reference's serve-cert-less behavior, SPEC §3.4).
	var sdsCert *stdtls.Certificate
	if len(c.GetTlsCertificateSdsSecretConfigs()) > 0 {
		if side != "downstream" {
			return nil, fmt.Errorf("tls: %s: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03", side)
		}
		secretName, _, _, err := xds.ParseSDSConfig(c.GetTlsCertificateSdsSecretConfigs())
		if err != nil {
			return nil, fmt.Errorf("tls: downstream: %w", err)
		}
		if provider == nil {
			return nil, fmt.Errorf("tls: downstream: SDS-delivered certificate requires a live SDS provider (unavailable in this mode)")
		}
		cert, err := provider.FetchInitialCertificate(context.Background(), secretName)
		if err != nil {
			return nil, fmt.Errorf("tls: downstream: SDS secret %q: %w", secretName, err)
		}
		sdsCert = cert
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

	if sdsCert != nil {
		cfg.Certificates = append(cfg.Certificates, *sdsCert)
	}

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
