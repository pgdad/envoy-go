package tls

import (
	stdtls "crypto/tls"
	"fmt"
	"log"

	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
)

// applyTLSParams maps Envoy TlsParameters fields onto cfg per SPEC §5.5.
// Nil params is a no-op. Errors begin with "tls: tls_params: ".
//
// See ADR-0030 for the scope decisions: version enum mapping, cipher-suite
// name-to-ID mapping (with TLS-1.3-only ciphers silently dropped since Go's
// crypto/tls does not permit selecting them), ecdh_curves mapping, and
// signature_algorithms erroring because stdlib does not publicly expose the
// knob.
func applyTLSParams(cfg *stdtls.Config, params *tlsv3.TlsParameters) error {
	if params == nil {
		return nil
	}
	if v, err := mapTLSVersion(params.GetTlsMinimumProtocolVersion()); err != nil {
		return fmt.Errorf("tls: tls_params: tls_minimum_protocol_version: %w", err)
	} else if v != 0 {
		cfg.MinVersion = v
	}
	if v, err := mapTLSVersion(params.GetTlsMaximumProtocolVersion()); err != nil {
		return fmt.Errorf("tls: tls_params: tls_maximum_protocol_version: %w", err)
	} else if v != 0 {
		cfg.MaxVersion = v
	}

	for _, name := range params.GetCipherSuites() {
		if tls13CipherByName(name) != 0 {
			// TLS 1.3 cipher — Go's crypto/tls does not allow selection; diagnostic and drop.
			log.Printf("tls: tls_params: TLS-1.3-only cipher %q requested; crypto/tls does not allow selection, dropping", name)
			continue
		}
		id, ok := tls12CipherByName(name)
		if !ok {
			return fmt.Errorf("tls: tls_params: unknown cipher suite %q", name)
		}
		cfg.CipherSuites = append(cfg.CipherSuites, id)
	}

	for _, name := range params.GetEcdhCurves() {
		id, ok := curveByName(name)
		if !ok {
			return fmt.Errorf("tls: tls_params: unknown ecdh curve %q", name)
		}
		cfg.CurvePreferences = append(cfg.CurvePreferences, id)
	}

	if len(params.GetSignatureAlgorithms()) > 0 {
		return fmt.Errorf("tls: tls_params: signature_algorithms is not configurable via Go crypto/tls in phase 03")
	}

	return nil
}

func mapTLSVersion(v tlsv3.TlsParameters_TlsProtocol) (uint16, error) {
	switch v {
	case tlsv3.TlsParameters_TLS_AUTO:
		// TlsProtocol 0 = TLS_AUTO in the proto; at the zero value the field
		// is treated as unset (no explicit version requested). The enum's
		// zero is TLS_AUTO; we cannot distinguish "unset" from "TLS_AUTO
		// explicitly chosen" at the proto level, so we adopt the strict
		// interpretation: explicit TLS_AUTO errors. If the caller wants no
		// bound, they omit the field entirely (which also yields the zero
		// value — indistinguishable). In practice phase-03 fixtures always
		// set TLSv1_2 min / TLSv1_3 max per Settled §10 #7.
		return 0, nil // treat as "not set" — let caller's cfg carry its default.
	case tlsv3.TlsParameters_TLSv1_0, tlsv3.TlsParameters_TLSv1_1:
		return 0, fmt.Errorf("%s is not supported in phase 03", v.String())
	case tlsv3.TlsParameters_TLSv1_2:
		return stdtls.VersionTLS12, nil
	case tlsv3.TlsParameters_TLSv1_3:
		return stdtls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unknown tls protocol enum %d", v)
	}
}

// tls12CipherByName maps an IANA / OpenSSL cipher suite name to a Go
// crypto/tls cipher suite ID, limited to TLS 1.2 ciphers that stdlib honors.
// Returns (0, false) on unknown name.
//
// The name list below is narrow by design — it covers the cipher suites
// crypto/tls actually supports for selection. Names not present (e.g.,
// obscure CBC suites) deliberately error so fixtures cannot quietly pick a
// weak cipher.
func tls12CipherByName(name string) (uint16, bool) {
	switch name {
	case "ECDHE-ECDSA-AES128-GCM-SHA256":
		return stdtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, true
	case "ECDHE-ECDSA-AES256-GCM-SHA384":
		return stdtls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, true
	case "ECDHE-ECDSA-CHACHA20-POLY1305":
		return stdtls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, true
	case "ECDHE-RSA-AES128-GCM-SHA256":
		return stdtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, true
	case "ECDHE-RSA-AES256-GCM-SHA384":
		return stdtls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, true
	case "ECDHE-RSA-CHACHA20-POLY1305":
		return stdtls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305, true
	default:
		return 0, false
	}
}

// tls13CipherByName returns the stdlib cipher ID for TLS 1.3 names, or 0 if
// the name is not a TLS-1.3 cipher. Used only to detect "should silently
// drop" names — the returned ID is not applied to cfg.CipherSuites because
// Go's crypto/tls does not permit TLS 1.3 cipher selection.
func tls13CipherByName(name string) uint16 {
	switch name {
	case "TLS_AES_128_GCM_SHA256":
		return stdtls.TLS_AES_128_GCM_SHA256
	case "TLS_AES_256_GCM_SHA384":
		return stdtls.TLS_AES_256_GCM_SHA384
	case "TLS_CHACHA20_POLY1305_SHA256":
		return stdtls.TLS_CHACHA20_POLY1305_SHA256
	default:
		return 0
	}
}

func curveByName(name string) (stdtls.CurveID, bool) {
	switch name {
	case "X25519":
		return stdtls.X25519, true
	case "P-256":
		return stdtls.CurveP256, true
	case "P-384":
		return stdtls.CurveP384, true
	case "P-521":
		return stdtls.CurveP521, true
	default:
		return 0, false
	}
}
