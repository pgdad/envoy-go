package xds

import (
	stdtls "crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// secretTypeURL returns the wire type URL for an SDS Secret resource, derived at
// runtime from the proto descriptor (never hardcoded — reference_network_filter_typeurl_extensions).
func secretTypeURL() string {
	return "type.googleapis.com/" + string(proto.MessageName(&tlsv3.Secret{}))
}

// dataSourceBytes resolves a core.DataSource into raw bytes. It MIRRORS
// internal/tls.loadDataSource's phase-03 grammar (inline_bytes / inline_string /
// filename honored; environment_variable + zero-value error) but is duplicated
// here deliberately: internal/xds must NOT import internal/tls (the 60.2 cycle
// guard — see doc.go / ADR-0278). A non-absolute filename resolves relative to
// baseDir.
func dataSourceBytes(ds *corev3.DataSource, baseDir string) ([]byte, error) {
	switch s := ds.GetSpecifier().(type) {
	case *corev3.DataSource_InlineBytes:
		return s.InlineBytes, nil
	case *corev3.DataSource_InlineString:
		return []byte(s.InlineString), nil
	case *corev3.DataSource_Filename:
		p := s.Filename
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("xds: sds: data source: read %s: %w", p, err)
		}
		return b, nil
	case *corev3.DataSource_EnvironmentVariable:
		return nil, fmt.Errorf("xds: sds: data source: environment_variable is not supported")
	default:
		return nil, fmt.Errorf("xds: sds: data source: none of inline_bytes, inline_string, filename set")
	}
}

// parseSecret validates one DiscoveryResponse resource and builds the served leaf.
// It requires: the Any resolves to a tls.v3.Secret; Secret.name == wantName; the
// secret's oneof is tls_certificate; and the certificate_chain/private_key
// DataSources yield a valid X509 key pair. Returns a classified error (wrapping
// errValidation, Task 4) so the caller can NACK + increment update_rejected.
func parseSecret(resource *anypb.Any, wantName, baseDir string) (*stdtls.Certificate, error) {
	var sec tlsv3.Secret
	if err := resource.UnmarshalTo(&sec); err != nil {
		return nil, fmt.Errorf("xds: sds: resource is not a %s: %w", secretTypeURL(), err)
	}
	if sec.GetName() != wantName {
		return nil, fmt.Errorf("xds: sds: response secret name %q != requested %q", sec.GetName(), wantName)
	}
	tc := sec.GetTlsCertificate()
	if tc == nil {
		return nil, fmt.Errorf("xds: sds: secret %q is not a tls_certificate (unsupported oneof arm)", wantName)
	}
	certPEM, err := dataSourceBytes(tc.GetCertificateChain(), baseDir)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: secret %q: certificate_chain: %w", wantName, err)
	}
	keyPEM, err := dataSourceBytes(tc.GetPrivateKey(), baseDir)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: secret %q: private_key: %w", wantName, err)
	}
	pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: secret %q: load cert: %w", wantName, err)
	}
	return &pair, nil
}

// parseValidationSecret unmarshals an SDS-delivered resource into a
// tls.v3.Secret, verifies it carries the validation_context oneof arm under the
// requested name, holds the served CertificateValidationContext to the SAME
// support surface as the inline path (internal/tls/config.go:234-245 — lifting
// the SDS envelope is not license to silently accept CVC sub-fields envoy-go
// cannot honor, reference_strict_reject_sibling_typeurl_gap), and loads
// trusted_ca into an *x509.CertPool.
//
// It is the validation_context sibling of parseSecret (which stays
// tls_certificate-only). The two are deliberately DISJOINT: each rejects the
// other's oneof arm, so a mis-served secret fails loudly rather than silently
// yielding a zero-value trust anchor.
//
// The CertPool build DUPLICATES internal/tls.loadTrustedCAPool rather than
// calling it: internal/tls imports internal/xds (config.go:13), so the reverse
// edge would cycle (ADR-0278 / reference_xds_config_seam_transitive_cycle_guard).
// This mirrors dataSourceBytes's deliberate duplication of
// internal/tls.loadDataSource. Keep internal/xds's dep set at internal/stats only.
//
// crl is NOT rejected — the inline path does not check it either
// (config.go:233-246), so rejecting here would be a NEW asymmetry. A documented
// SHARED gap, deferred to the CVC-feature follow-on (SPEC-65 §6).
func parseValidationSecret(resource *anypb.Any, wantName, baseDir string) (*x509.CertPool, error) {
	var sec tlsv3.Secret
	if err := resource.UnmarshalTo(&sec); err != nil {
		return nil, fmt.Errorf("xds: sds: resource is not a %s: %w", secretTypeURL(), err)
	}
	if sec.GetName() != wantName {
		return nil, fmt.Errorf("xds: sds: response secret name %q != requested %q", sec.GetName(), wantName)
	}
	vc := sec.GetValidationContext()
	if vc == nil {
		return nil, fmt.Errorf("xds: sds: secret %q is not a validation_context (unsupported oneof arm)", wantName)
	}
	if vc.GetCustomValidatorConfig() != nil {
		return nil, fmt.Errorf("xds: sds: validation secret %q: custom_validator_config is not supported", wantName)
	}
	if len(vc.GetMatchTypedSubjectAltNames()) > 0 {
		return nil, fmt.Errorf("xds: sds: validation secret %q: match_typed_subject_alt_names is not supported", wantName)
	}
	if len(vc.GetVerifyCertificateHash()) > 0 {
		return nil, fmt.Errorf("xds: sds: validation secret %q: verify_certificate_hash is not supported", wantName)
	}
	if len(vc.GetVerifyCertificateSpki()) > 0 {
		return nil, fmt.Errorf("xds: sds: validation secret %q: verify_certificate_spki is not supported", wantName)
	}
	if vc.GetTrustedCa() == nil {
		// S1: the validation_context ACKs with trusted_ca ABSENT ENTIRELY (an
		// entirely empty CertificateValidationContext — a PGV-valid message). The
		// reference ACKs this and merges the empty context away, so the default's
		// trusted_ca survives and the listener serves; classify it so internal/tls
		// can fall back (phase 68, ADR-0290).
		//
		// This gate is NARROW by design. A present-but-specifier-unset
		// trusted_ca:{} (a DataSource with the specifier oneof unset) is NOT a
		// fallback trigger: the reference PGV-NACKs that served secret
		// (CertificateValidationContext.TrustedCa … field: "specifier", reason:
		// is required — SPEC-66 §3.9(b)/ADR-0287 live probe). It therefore falls
		// through to the dataSourceBytes no-specifier reject below and stays a
		// plain errValidation boot-FAIL, exactly like a set-but-empty
		// inline_bytes:"" (S2) or a corrupt PEM (S3).
		return nil, fmt.Errorf("xds: sds: validation secret %q: %w", wantName, ErrEmptyValidationContext)
	}
	caPEM, err := dataSourceBytes(vc.GetTrustedCa(), baseDir)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: validation secret %q: trusted_ca: %w", wantName, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("xds: sds: validation secret %q: trusted_ca: parse failure", wantName)
	}
	return pool, nil
}
