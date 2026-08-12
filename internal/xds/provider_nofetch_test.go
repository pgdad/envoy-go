package xds

import (
	"context"
	"strings"
	"testing"
)

func TestNoFetchProvider_Discriminator(t *testing.T) {
	if !IsNoFetch(NoFetchProvider()) {
		t.Error("IsNoFetch(NoFetchProvider()) = false, want true")
	}
	if IsNoFetch(nil) {
		t.Error("IsNoFetch(nil) = true, want false — nil must NEVER classify as the sentinel")
	}
	var other SecretProvider = &Provider{} // any non-sentinel implementation
	if IsNoFetch(other) {
		t.Error("IsNoFetch(non-sentinel) = true, want false")
	}
}

func TestNoFetchProvider_FetchMethodsError(t *testing.T) {
	p := NoFetchProvider()
	if _, err := p.FetchInitialCertificate(context.Background(), "s"); err == nil ||
		!strings.Contains(err.Error(), "no-fetch (validate-mode) provider asked to fetch") {
		t.Errorf("FetchInitialCertificate: want the defense-in-depth substring, got %v", err)
	}
	if _, err := p.FetchInitialValidationContext(context.Background(), "s"); err == nil ||
		!strings.Contains(err.Error(), "no-fetch (validate-mode) provider asked to fetch") {
		t.Errorf("FetchInitialValidationContext: want the defense-in-depth substring, got %v", err)
	}
}
