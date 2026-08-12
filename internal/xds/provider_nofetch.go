package xds

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"fmt"
)

// noFetchProvider is the validate-mode sentinel SecretProvider (phase 86,
// ADR-0308). It exists so --mode validate can run the ENTIRE boot pre-scan
// (boot.NewValidateSDSProvider) and then thread a NON-NIL provider whose only
// job is to be recognized by internal/tls's IsNoFetch fetch-site skips. The
// discriminator is this TYPE, never provider == nil — the nil-reject's other
// consumers (QUIC, the exported test-only constructors, main.go's seen==0
// case) keep rejecting byte-identically.
type noFetchProvider struct{}

// NoFetchProvider returns the validate-mode sentinel. Value type: the zero
// value IS the sentinel, and IsNoFetch works on any copy.
func NoFetchProvider() SecretProvider { return noFetchProvider{} }

// IsNoFetch reports whether p is the validate-mode sentinel. A nil p is NOT
// the sentinel (the type assertion on a nil interface is false).
func IsNoFetch(p SecretProvider) bool { _, ok := p.(noFetchProvider); return ok }

func (noFetchProvider) FetchInitialCertificate(context.Context, string) (*stdtls.Certificate, error) {
	// Never called: every fetch site checks IsNoFetch FIRST. Kept as
	// defense-in-depth (ADR-0080-distinct if ever seen).
	return nil, fmt.Errorf("xds: sds: internal: no-fetch (validate-mode) provider asked to fetch a certificate")
}

func (noFetchProvider) FetchInitialValidationContext(context.Context, string) (*x509.CertPool, error) {
	return nil, fmt.Errorf("xds: sds: internal: no-fetch (validate-mode) provider asked to fetch a validation context")
}
