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
	// Phase 66 (C1/D1): the CVC arm's reject was lifted in
	// commonTLSContextToConfig, which CANNOT see require_client_certificate (it
	// lives on DownstreamTlsContext, not on CommonTlsContext). Without this arm a
	// CVC listener with require_client_certificate false/absent would boot
	// SUCCESSFULLY with ClientCAs nil and ClientAuth == NoClientCert — converting
	// today's loud boot-FAIL into a silently unauthenticated listener.
	// envoy-go-strict (ADR-0080): the reference HONORS the anchor here
	// (verify-if-presented); envoy-go refuses LOUDLY rather than diverging
	// silently. GetValue() on a nil *wrapperspb.BoolValue returns false, so this
	// covers require:false AND require: absent (verified by execution).
	if _, isCVC := ctx.GetCommonTlsContext().GetValidationContextType().(*tlsv3.CommonTlsContext_CombinedValidationContext); isCVC &&
		!ctx.GetRequireClientCertificate().GetValue() {
		return nil, fmt.Errorf("tls: downstream: combined_validation_context requires require_client_certificate: true in phase 03")
	}
	// Phase-16 ADR-0147 (unanticipated): when require_client_certificate is
	// true, configure the downstream listener for mandatory mTLS — load a
	// trusted-CA pool into ClientCAs and set
	// ClientAuth=RequireAndVerifyClientCert. This lifts the phase-03 clause-7
	// rejection (ADR-0032 §Decision (7)) to support fixture 0018's scenario 6
	// (ADR-0144 framework primitive end-to-end validation). The lift is
	// SCOPED to require_client_certificate=true; the trusted-CA pool may be
	// sourced from an inline validation_context.trusted_ca PEM, an SDS-bound
	// validation_context_sds_secret_config (phase 65, ADR-0286 — downstream
	// with a live provider only), or a combined_validation_context (phase 66,
	// this row — downstream with a live provider only; both landed since
	// this comment was first written and are no longer "parse-rejected").
	// custom_validator_config, match_typed_subject_alt_names, and
	// verify_certificate_hash/spki remain rejected regardless of source —
	// including under a combined_validation_context's
	// default_validation_context — via the unchanged
	// commonTLSContextToConfig pre-checks below.
	if ctx.GetRequireClientCertificate().GetValue() {
		common := ctx.GetCommonTlsContext()
		if sdsVC, ok := common.GetValidationContextType().(*tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig); ok {
			// Phase 65 (ADR-0286): the trusted-CA bundle is delivered by an SDS
			// management server over the landed SotW stream, bounded by
			// initial_fetch_timeout. The fetch lives HERE (not in
			// commonTLSContextToConfig) because require_client_certificate is a
			// DownstreamTlsContext field, invisible from the CommonTlsContext — and
			// gating on it means the deferred require==false case performs NO
			// wasteful boot-time fetch (SPEC-65 §3.3b / §3.5).
			if provider == nil {
				// Defense-in-depth: commonTLSContextToConfig's
				// validation_context_sds_secret_config arm already refuses the
				// nil-provider SDS-validation shape above, so this guard is
				// UNREACHABLE from today's entry points. It is kept so a future caller
				// that relaxes that reject cannot nil-deref here.
				return nil, fmt.Errorf("tls: downstream: SDS-delivered validation_context requires a live SDS provider (unavailable in this mode)")
			}
			// ParseSDSConfig takes a LIST (it enforces len==1, internal/xds/config.go:23);
			// validation_context_sds_secret_config is SINGULAR, so wrap it.
			secretName, _, _, err := xds.ParseSDSConfig([]*tlsv3.SdsSecretConfig{sdsVC.ValidationContextSdsSecretConfig})
			if err != nil {
				// xds: -prefixed -> wrap to preserve the `tls: ` invariant
				// (FuzzTLSContextParse).
				return nil, fmt.Errorf("tls: downstream: %w", err)
			}
			pool, err := provider.FetchInitialValidationContext(context.Background(), secretName)
			if err != nil {
				// A timeout / unreachable management server boot-FAILS the listener —
				// the documented envoy-go DEPARTURE from the reference's serve-anyway
				// (ADR-0280, extended to this resource type).
				return nil, fmt.Errorf("tls: downstream: SDS validation secret %q: %w", secretName, err)
			}
			cfg.ClientCAs = pool
		} else if cvc, ok := common.GetValidationContextType().(*tlsv3.CommonTlsContext_CombinedValidationContext); ok {
			// Phase 66 — the EQUIVALENCE THEOREM. envoy-go does NOT implement
			// Message::MergeFrom() for combined_validation_context and MUST NOT:
			// FetchInitialValidationContext returns an *x509.CertPool, so the served
			// CertificateValidationContext message never crosses the internal/xds seam
			// and proto.Merge has no operand. Instead the SDS-delivered pool wins
			// outright and default_validation_context.trusted_ca is NOT read — provably
			// identical to a true merge on envoy-go's honored surface, because:
			//   P1 only trusted_ca is HONORED (four more sub-fields are REJECTED — see
			//      the inlineVC selector in commonTLSContextToConfig; the remaining ten
			//      CertificateValidationContext fields are silently ignored by BOTH
			//      paths, a shared gap, not a divergence);
			//   P2 the DataSource specifier oneof REPLACES rather than appends;
			//   P3 a successful fetch GUARANTEES that specifier is set
			//      (parseValidationSecret errors otherwise), so the default can never
			//      contribute;
			//   P4 the four rejects are RE-POINTED at default_validation_context. This
			//      premise is load-bearing and was nearly omitted: without it a CVC
			//      whose default demands e.g. match_typed_subject_alt_names would boot
			//      with NO SAN check where a true merge boot-FAILS — a silently weaker
			//      listener.
			//
			// P5 — this rests on a COINCIDENCE, not a guarantee: internal/xds's
			// dataSourceBytes and internal/tls's loadDataSource are arm-for-arm
			// identical (differing only in error-string prefix), and main.go passes the
			// SAME filepath.Dir(*cfgPath) as baseDir to BOTH NewSDSProvider and
			// boot.Construct. Nothing structurally enforces either equality. If either
			// ever diverges, this equivalence is SILENTLY falsified and CVC goes wrong
			// with no test to catch it.
			if provider == nil {
				// Defense-in-depth, mirroring the phase-65 SDS-VC arm above: the CVC arm
				// in commonTLSContextToConfig already refuses the nil-provider shape, so
				// this is UNREACHABLE from today's entry points. Kept so a future caller
				// that relaxes that reject cannot nil-deref here.
				return nil, fmt.Errorf("tls: downstream: combined_validation_context requires a live SDS provider (unavailable in this mode)")
			}
			// ParseSDSConfig takes a LIST (it enforces len==1); the CVC's
			// validation_context_sds_secret_config is SINGULAR, so wrap it. This is what
			// validates sds_config/api_config_source/resource_api_version:V3/GRPC/
			// cluster_name — a bare GetName() would silently accept a malformed
			// sds_config that the SDS-VC sibling REJECTS.
			secretName, _, _, err := xds.ParseSDSConfig([]*tlsv3.SdsSecretConfig{cvc.CombinedValidationContext.GetValidationContextSdsSecretConfig()})
			if err != nil {
				// xds: -prefixed -> wrap to preserve the `tls: ` invariant
				// (FuzzTLSContextParse).
				return nil, fmt.Errorf("tls: downstream: %w", err)
			}
			pool, err := provider.FetchInitialValidationContext(context.Background(), secretName)
			if err != nil {
				// A served validation context with no usable trusted_ca, a timeout, or an
				// unreachable management server boot-FAILS the listener. DEPARTURE: the
				// reference ACKs an empty dynamic context, falls back to the DEFAULT CA,
				// and SERVES; envoy-go refuses to boot (ADR-0280 family).
				return nil, fmt.Errorf("tls: downstream: SDS validation secret %q: %w", secretName, err)
			}
			cfg.ClientCAs = pool
		} else {
			vc := common.GetValidationContext()
			if vc == nil || vc.GetTrustedCa() == nil {
				return nil, fmt.Errorf("tls: downstream: require_client_certificate=true requires validation_context.trusted_ca")
			}
			pool, err := loadTrustedCAPool(vc, baseDir, "downstream")
			if err != nil {
				return nil, err
			}
			cfg.ClientCAs = pool
		}
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
//   - validation_context_sds_secret_config set, UNLESS side=="downstream"
//     with a live provider (phase 65, ADR-0286): accepted there as a NO-OP
//     in this function — NewDownstreamConfig's require_client_certificate
//     block performs the actual fetch and installs ClientCAs
//   - combined_validation_context set, UNLESS side=="downstream" with a
//     live provider (phase 66, this row): accepted there as a NO-OP in
//     this function, subject to the default_validation_context /
//     validation_context_sds_secret_config presence checks below — the SDS
//     half is fetched and installed the same way
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
			// Phase 65 (ADR-0286): a downstream SDS-delivered validation_context is
			// HONORED — fetched and installed as ClientCAs by NewDownstreamConfig's
			// require_client_certificate block (the only scope that sees
			// require_client_certificate, which lives on the DownstreamTlsContext,
			// not on this CommonTlsContext). Here it is a NO-OP for the
			// downstream+provider path.
			//
			// The guard below has TWO live production consumers plus TWO exported
			// test-only constructors (verified by walking every caller of
			// NewDownstreamConfig/NewQUICDownstreamConfig/NewManagerWithBaseDirAndAllowH2C
			// repo-wide, including cmd/ and validate/, not just internal/):
			//   - NewQUICDownstreamConfig, which passes side == "downstream" with a
			//     NIL provider (QUIC carries no SDS in phase 61.1) — and whose
			//     transport_socket type_url is invisible to boot.NewSDSProvider's
			//     pre-scan (it matches only the plain DownstreamTlsContext type_url,
			//     never QUIC's wrapper), so this fires independent of any other
			//     listener's SDS shape;
			//   - validate.Bootstrap (validate/validate.go), which threads a LITERAL
			//     nil sdsProvider into boot.Construct unconditionally — no pre-scan
			//     gates it;
			//   - listener.NewManager / NewManagerWithBaseDir (exported, test-only):
			//     both hardcode sdsProvider=nil regardless of bootstrap contents.
			// main.go's ordinary boot path is NOT a consumer of THIS arm — unlike the
			// combined_validation_context arm below (whose pure-inline shape the
			// pre-scan deliberately excludes from its seen count),
			// boot.NewSDSProvider increments seen UNCONDITIONALLY whenever a
			// (non-QUIC) listener selects THIS oneof arm (internal/boot/boot.go: no
			// nil-secret gate on this branch), so any real occurrence forces
			// seen>=1 and main.go can only reach this reject via the
			// NewQUICDownstreamConfig path above, never via a seen==0 nil provider.
			// It keeps the BYTE-IDENTICAL phase-03 reject (ADR-0080 distinct
			// substring). NewUpstreamConfig never reaches this arm: validation_context_type
			// is a ONEOF, so selecting the SDS arm makes its GetValidationContext()
			// return nil and it refuses earlier with its own trusted_ca message. The
			// side != "downstream" half is therefore dead from today's entry points;
			// it is kept so a future upstream caller cannot silently skip validation.
			if side != "downstream" || provider == nil {
				return nil, fmt.Errorf("tls: %s: SDS-bound validation_context_sds_secret_config is not supported in phase 03", side)
			}
		case *tlsv3.CommonTlsContext_CombinedValidationContext:
			// Phase 66: a downstream SDS-delivered combined_validation_context is
			// HONORED — the SDS half is fetched and installed as ClientCAs by
			// NewDownstreamConfig's require_client_certificate block (the only scope
			// that sees require_client_certificate, which lives on the
			// DownstreamTlsContext, not on this CommonTlsContext). Here it is a NO-OP
			// for the downstream+provider path.
			//
			// The guard below is retained BYTE-IDENTICAL (ADR-0080). It has THREE live
			// consumers: NewQUICDownstreamConfig (literal nil provider),
			// validate.Bootstrap (nil), and main.go's ordinary path when
			// boot.NewSDSProvider returns (nil, nil) at seen==0. It MUST precede the
			// E1/E2 checks below, or those consumers would receive E1/E2's message
			// instead of this byte-identical one.
			if side != "downstream" || provider == nil {
				return nil, fmt.Errorf("tls: %s: combined_validation_context is not supported in phase 03", side)
			}
			cvc := c.GetCombinedValidationContext()
			if cvc.GetDefaultValidationContext() == nil {
				return nil, fmt.Errorf("tls: %s: combined_validation_context.default_validation_context is required", side)
			}
			if cvc.GetValidationContextSdsSecretConfig() == nil {
				return nil, fmt.Errorf("tls: %s: combined_validation_context.validation_context_sds_secret_config is required", side)
			}
			// else: NO-OP. NewDownstreamConfig's require_client_certificate block
			// performs the fetch and installs the pool.
		}
	}
	// Phase 66: under a combined_validation_context the validation_context_type
	// oneof makes GetValidationContext() nil, so the four rejects below were
	// BYPASSED — lifting the envelope without this selector would SILENTLY ACCEPT
	// sub-fields envoy-go cannot honor. The four error strings are shared with the
	// inline path and stay BYTE-IDENTICAL (ADR-0080).
	inlineVC := c.GetValidationContext()
	if inlineVC == nil {
		if cvc := c.GetCombinedValidationContext(); cvc != nil {
			inlineVC = cvc.GetDefaultValidationContext()
		}
	}
	if vc := inlineVC; vc != nil {
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
