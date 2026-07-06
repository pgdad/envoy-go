package ratelimit

import (
	"testing"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// TestTypeURL pins the byte-exact TypeURL constant per ADR-0143 SN1. Any
// drift surfaces here BEFORE the boot registry "no factory registered for
// type URL" runtime error at Task 7.
func TestTypeURL(t *testing.T) {
	want := "type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}

// TestFilterName pins the byte-exact filterName const consumed by the
// boot-registration HTTPFilter{Name:...} return shape per ADR-0070.
func TestFilterName(t *testing.T) {
	want := "envoy.filters.http.ratelimit"
	if filterName != want {
		t.Fatalf("filterName = %q, want %q", filterName, want)
	}
}

// TestNew_NilTypedConfig pins the typed_config nil-guard per ADR-0072. The
// full `New` body lands at Task 7; the nil-input arm is the first defensive
// guard before buildCompiledConfig (which would otherwise return its own
// `ratelimit: typed_config required` error from the same arm).
func TestNew_NilTypedConfig(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("New(nil): want error, got nil")
	}
}

// TestNew_FullWiring pins the Task-7 positive-path round-trip: a valid config
// + a cluster-manager-bearing FactoryCtx + a stats registry produces a
// non-nil FilterInstanceFactory whose instantiated HTTPFilter has Decoder + Encoder
// pointing at the same *filter (the both-sides discipline per parent SPEC §6.5/§6.6).
func TestNew_FullWiring(t *testing.T) {
	cm := mkRatelimitH2ClusterMgr(t, rlsClusterName_test)
	ctx := ratelimitFactoryCtxWithClusterMgr(cm)
	tc := toAnyRL(t, validRateLimitConfig())

	factory, err := New(tc, ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if factory == nil {
		t.Fatal("New: returned nil factory")
	}
	hf := factory()
	if hf.Name != filterName {
		t.Errorf("HTTPFilter.Name: got %q, want %q", hf.Name, filterName)
	}
	if hf.Decoder == nil {
		t.Error("HTTPFilter.Decoder: nil; want non-nil (decode-side participation per SPEC §6.5)")
	}
	if hf.Encoder == nil {
		t.Error("HTTPFilter.Encoder: nil; want non-nil (encode-side participation per SPEC §6.6 — STUBBED at 24.1 per D-RL7)")
	}
	// Both-sides discipline: the same *filter pointer satisfies both interfaces.
	// Compare via *filter type assertions (the two interface types are
	// distinct, so a direct comparison won't compile).
	dec, dOK := hf.Decoder.(*filter)
	enc, eOK := hf.Encoder.(*filter)
	if !dOK || !eOK {
		t.Fatal("HTTPFilter sides: want *filter on both sides")
	}
	if dec != enc {
		t.Error("HTTPFilter.Decoder pointer != HTTPFilter.Encoder pointer: want same instance (both-sides filter)")
	}
}
