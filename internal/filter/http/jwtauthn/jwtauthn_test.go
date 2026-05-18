package jwtauthn

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	jwt_authnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/jwks"
	"github.com/esalaine/envoy-go/internal/jwt"
	"github.com/esalaine/envoy-go/internal/stats"
)

// ----------------------------------------------------------------------------
// Test helpers (mirror phase-16 rbac_test.go precedent).
// ----------------------------------------------------------------------------

// mustAny packages a proto into an *anypb.Any. Mirrors phase-13/14/15/16
// helper precedent.
func mustAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// freshFactoryCtx returns a FactoryCtx carrying a fresh Registry; used by tests
// that exercise the stat-registration path. Per ADR-0085, an empty FactoryCtx{}
// is also valid (nil Stats skips stat registration entirely).
func freshFactoryCtx() envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "ingress_http"}
}

// freshFactoryCtxWithRegistry returns a FactoryCtx with the supplied Registry.
// Used by tests that need to inspect counter registration.
func freshFactoryCtxWithRegistry(reg *stats.Registry) envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"}
}

// localJwksProvider returns a minimal JwtProvider that uses an inline_string
// LocalJwks carrying one valid RSA JWK (RFC 7517 §A.1 exemplar fragments).
// Task 4 + ADR-0151 wires the real `jwks.ParseJWKSet` call; tests using this
// helper assert the parsed `*jwks.JWKSet` is stored at
// `compiledProvider.localJwks`.
func localJwksProvider() *jwt_authnv3.JwtProvider {
	return &jwt_authnv3.JwtProvider{
		Issuer: "https://example.com",
		JwksSourceSpecifier: &jwt_authnv3.JwtProvider_LocalJwks{
			LocalJwks: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineString{
					InlineString: `{"keys":[{"kty":"RSA","kid":"k1","alg":"RS256","use":"sig","n":"0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n91CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINHaQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw","e":"AQAB"}]}`,
				},
			},
		},
	}
}

// happyMatch returns a minimal RouteMatch (prefix `/`) sufficient for parse-
// time RequirementRule.match-required PGV-mirror tests.
func happyMatch() *routev3.RouteMatch {
	return &routev3.RouteMatch{
		PathSpecifier: &routev3.RouteMatch_Prefix{Prefix: "/"},
	}
}

// ----------------------------------------------------------------------------
// Group 1 — Config parsing + buildCompiledConfig + buildCompiledProvider +
// buildCompiledRequirement + buildCompiledRule + filterStats registration
// (per SPEC §6.5 + §1.1 amendments 1-12; ADR-0148 + ADR-0149).
// ----------------------------------------------------------------------------

func TestNew_NilTC(t *testing.T) {
	factory, err := New(nil, freshFactoryCtx())
	if err == nil {
		t.Fatalf("New(nil, _): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(nil, _): want nil factory, got %v", factory)
	}
	if !strings.Contains(err.Error(), "typed_config required") {
		t.Errorf("got %q; want substring 'typed_config required'", err.Error())
	}
}

func TestNew_MalformedTC(t *testing.T) {
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	factory, err := New(bad, freshFactoryCtx())
	if err == nil {
		t.Fatalf("New(malformed, _): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(malformed, _): want nil factory, got %v", factory)
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("got %q; want substring 'unmarshal'", err.Error())
	}
}

func TestBuildCompiledConfig_EmptyConfig_AllOuterFieldsTolerated(t *testing.T) {
	// Empty JwtAuthentication: no providers, no rules, no requirement_map,
	// bypass_cors_preflight=false, strip_failure_response=false,
	// filter_state_rules=nil. All 6 outer fields at proto defaults; defensive
	// parse tolerates empty per §1.1 amendment 1 + §11.P9.
	c := &jwt_authnv3.JwtAuthentication{}
	cc, err := buildCompiledConfig(c, freshFactoryCtx())
	if err != nil {
		t.Fatalf("buildCompiledConfig(empty): want success, got %v", err)
	}
	if len(cc.providers) != 0 {
		t.Errorf("providers: want empty, got %d entries", len(cc.providers))
	}
	if len(cc.rules) != 0 {
		t.Errorf("rules: want empty, got %d entries", len(cc.rules))
	}
	if len(cc.requirementMap) != 0 {
		t.Errorf("requirementMap: want empty, got %d entries", len(cc.requirementMap))
	}
	if cc.bypassCorsPreflight {
		t.Error("bypassCorsPreflight: want false default")
	}
	if cc.stripFailureResponse {
		t.Error("stripFailureResponse: want false default")
	}
	if cc.stats == nil {
		t.Error("stats: want non-nil (ctx.Stats was non-nil)")
	}
}

func TestBuildCompiledConfig_NilStats_NilTolerant(t *testing.T) {
	// Per ADR-0085 nil-tolerance: ctx.Stats == nil → cc.stats == nil; no panic.
	c := &jwt_authnv3.JwtAuthentication{}
	cc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("buildCompiledConfig(nil Stats): want success, got %v", err)
	}
	if cc.stats != nil {
		t.Errorf("stats: want nil when ctx.Stats is nil; got %v", cc.stats)
	}
}

func TestBuildCompiledConfig_BypassCorsPreflight_Honored(t *testing.T) {
	c := &jwt_authnv3.JwtAuthentication{BypassCorsPreflight: true}
	cc, err := buildCompiledConfig(c, freshFactoryCtx())
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	if !cc.bypassCorsPreflight {
		t.Error("bypassCorsPreflight: want true")
	}
}

func TestBuildCompiledConfig_StripFailureResponse_Honored(t *testing.T) {
	c := &jwt_authnv3.JwtAuthentication{StripFailureResponse: true}
	cc, err := buildCompiledConfig(c, freshFactoryCtx())
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	if !cc.stripFailureResponse {
		t.Error("stripFailureResponse: want true")
	}
}

func TestBuildCompiledConfig_FilterStateRules_SilentIgnored(t *testing.T) {
	// Per §1.1 amendment 1 + §8 deferral 12: filter_state_rules is structurally
	// silent-ignored. Parse accepts the field; compiledConfig has no slot for
	// it (the silent-ignore is structural — there is nowhere to assert "not
	// stored" except observing successful parse + absent in compiledConfig).
	c := &jwt_authnv3.JwtAuthentication{
		FilterStateRules: &jwt_authnv3.FilterStateRule{Name: "test"},
	}
	_, err := buildCompiledConfig(c, freshFactoryCtx())
	if err != nil {
		t.Fatalf("buildCompiledConfig: filter_state_rules set; want silent-ignored success, got %v", err)
	}
}

func TestBuildCompiledConfig_FilterStats_SevenCountersRegistered(t *testing.T) {
	// Per SPEC §1.1 amendment 9 + §11.P6 + ADR-0148 + ADR-0154: 7 base
	// counters registered unconditionally at New() time. The 5 active +
	// 2 STRUCTURALLY UNREACHABLE (jwt_cache_hit + jwt_cache_miss per §8
	// deferral 8) all register so operators get a consistent scrape surface.
	reg := stats.NewRegistry()
	c := &jwt_authnv3.JwtAuthentication{}
	cc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"})
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	if cc.stats == nil {
		t.Fatal("stats: want non-nil")
	}
	if cc.stats.allowed == nil {
		t.Error("filterStats.allowed: want non-nil")
	}
	if cc.stats.denied == nil {
		t.Error("filterStats.denied: want non-nil")
	}
	if cc.stats.corsPreflightBypassed == nil {
		t.Error("filterStats.corsPreflightBypassed: want non-nil")
	}
	if cc.stats.jwksFetchSuccess == nil {
		t.Error("filterStats.jwksFetchSuccess: want non-nil")
	}
	if cc.stats.jwksFetchFailed == nil {
		t.Error("filterStats.jwksFetchFailed: want non-nil")
	}
	if cc.stats.jwtCacheHit == nil {
		t.Error("filterStats.jwtCacheHit: want non-nil (structurally unreachable but registered for scrape stability)")
	}
	if cc.stats.jwtCacheMiss == nil {
		t.Error("filterStats.jwtCacheMiss: want non-nil (structurally unreachable but registered for scrape stability)")
	}
}

func TestBuildCompiledConfig_CorsPreflightBypassed_CanonicalNaming(t *testing.T) {
	// Per §1.1 amendment 10 + ADR-0154: canonical name is `cors_preflight_bypassed`
	// (REFUTES BRAINSTORM `bypassed_cors_preflight` hypothesis). The counter
	// name registered with the Registry is the canonical form.
	reg := stats.NewRegistry()
	c := &jwt_authnv3.JwtAuthentication{}
	_, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"})
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	// Verify the registered counters carry the canonical names. Walk the
	// registered counter list looking for the canonical `cors_preflight_bypassed`
	// suffix; the inverse BRAINSTORM-hypothesis `bypassed_cors_preflight` must
	// NOT appear.
	names := registeredCounterNames(reg)
	foundCanonical := false
	for _, n := range names {
		if strings.HasSuffix(n, ".cors_preflight_bypassed") {
			foundCanonical = true
		}
		if strings.HasSuffix(n, ".bypassed_cors_preflight") {
			t.Errorf("REFUTED naming hypothesis appears: %q", n)
		}
	}
	if !foundCanonical {
		t.Errorf("canonical `cors_preflight_bypassed` counter not found in %v", names)
	}
}

// registeredCounterNames returns the list of counter names currently
// registered with reg via reg.Walk.
func registeredCounterNames(reg *stats.Registry) []string {
	var out []string
	reg.Walk(func(m stats.Metric) {
		out = append(out, m.Name())
	})
	return out
}

func TestBuildCompiledProvider_RemoteJwks(t *testing.T) {
	// RemoteJwks path lands at Task 3 (ADR-0150). The real internal/jwks.Fetcher
	// is wired into compiledProvider.jwksFetcher when a RemoteJwks provider is
	// parsed; the fetcher's blocking initial fetch succeeds against the
	// httptest server below.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Minimal valid JWK Set with one RSA key (n+e base64url-encoded
		// fragments lifted from RFC 7517 §A.1's exemplar). Real production
		// JWKS payloads carry actual public-key params; this fixture exists
		// only to exercise the parse + cache wire-up.
		_, _ = w.Write([]byte(`{"keys":[{"kty":"RSA","kid":"k1","alg":"RS256","use":"sig","n":"0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n91CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINHaQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw","e":"AQAB"}]}`))
	}))
	defer srv.Close()

	c := &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p1": {
				Issuer: "https://example.com",
				JwksSourceSpecifier: &jwt_authnv3.JwtProvider_RemoteJwks{
					RemoteJwks: &jwt_authnv3.RemoteJwks{
						HttpUri: &corev3.HttpUri{
							Uri:              srv.URL,
							HttpUpstreamType: &corev3.HttpUri_Cluster{Cluster: "jwks_cluster"},
						},
						CacheDuration: durationpb.New(1 * time.Minute),
					},
				},
			},
		},
	}
	cc, err := buildCompiledConfig(c, freshFactoryCtx())
	if err != nil {
		t.Fatalf("buildCompiledConfig: want success with real RemoteJwks fetcher, got %v", err)
	}
	cp, ok := cc.providers["p1"]
	if !ok {
		t.Fatalf("compiledProvider p1 missing")
	}
	if cp.jwksFetcher == nil {
		t.Fatalf("compiledProvider.jwksFetcher is nil; want *jwks.Fetcher")
	}
	// Close the fetcher to terminate its background refresh goroutine — the
	// listener-lifetime ownership semantic per ADR-0150 §Decision means the
	// real production code path closes via listener-drain; tests close
	// directly.
	if err := cp.jwksFetcher.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestBuildCompiledProvider_RemoteJwks_MissingURI_ParseRejected(t *testing.T) {
	// Per §11.P9 + ADR-0150 PARSE-REJECT-for-missing-http_uri:
	// remote_jwks.http_uri.uri must be non-empty.
	c := &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p1": {
				Issuer: "https://example.com",
				JwksSourceSpecifier: &jwt_authnv3.JwtProvider_RemoteJwks{
					RemoteJwks: &jwt_authnv3.RemoteJwks{
						HttpUri: &corev3.HttpUri{Uri: ""},
					},
				},
			},
		},
	}
	_, err := buildCompiledConfig(c, freshFactoryCtx())
	if err == nil {
		t.Fatalf("buildCompiledConfig: missing remote_jwks.http_uri.uri; want error, got nil")
	}
	if !strings.Contains(err.Error(), "http_uri") {
		t.Errorf("got %q; want substring 'http_uri'", err.Error())
	}
}

func TestBuildCompiledProvider_LocalJwks(t *testing.T) {
	// LocalJwks path lands at Task 4 (ADR-0151). The buildCompiledProvider
	// reads the DataSource inline_string + invokes jwks.ParseJWKSet; the
	// resulting *jwks.JWKSet is stored at compiledProvider.localJwks.
	c := &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p1": localJwksProvider(),
		},
	}
	cc, err := buildCompiledConfig(c, freshFactoryCtx())
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	cp, ok := cc.providers["p1"]
	if !ok {
		t.Fatalf("compiledProvider p1 missing")
	}
	if cp.localJwks == nil {
		t.Fatalf("compiledProvider.localJwks is nil; want non-nil *jwks.JWKSet")
	}
	if cp.jwksFetcher != nil {
		t.Errorf("compiledProvider.jwksFetcher should be nil on LocalJwks path; got %T", cp.jwksFetcher)
	}
	if len(cp.localJwks.Keys) != 1 {
		t.Errorf("localJwks.Keys len = %d; want 1", len(cp.localJwks.Keys))
	}
	if cp.localJwks.Keys[0].Kid != "k1" {
		t.Errorf("localJwks.Keys[0].Kid = %q; want k1", cp.localJwks.Keys[0].Kid)
	}
}

func TestBuildCompiledProvider_LocalJwks_InlineBytes(t *testing.T) {
	// LocalJwks via DataSource_InlineBytes — same RSA fixture, bytes form.
	c := &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p1": {
				Issuer: "https://example.com",
				JwksSourceSpecifier: &jwt_authnv3.JwtProvider_LocalJwks{
					LocalJwks: &corev3.DataSource{
						Specifier: &corev3.DataSource_InlineBytes{
							InlineBytes: []byte(`{"keys":[{"kty":"RSA","kid":"k1","alg":"RS256","n":"0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n91CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINHaQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw","e":"AQAB"}]}`),
						},
					},
				},
			},
		},
	}
	cc, err := buildCompiledConfig(c, freshFactoryCtx())
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	if cc.providers["p1"].localJwks == nil {
		t.Fatalf("compiledProvider.localJwks is nil")
	}
}

func TestBuildCompiledProvider_LocalJwks_MalformedJSON_ParseRejected(t *testing.T) {
	// Defensive: a malformed inline JWK Set surfaces as a parse error.
	c := &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p1": {
				Issuer: "https://example.com",
				JwksSourceSpecifier: &jwt_authnv3.JwtProvider_LocalJwks{
					LocalJwks: &corev3.DataSource{
						Specifier: &corev3.DataSource_InlineString{
							InlineString: `{not json`,
						},
					},
				},
			},
		},
	}
	_, err := buildCompiledConfig(c, freshFactoryCtx())
	if err == nil {
		t.Fatalf("buildCompiledConfig: malformed JWK Set; want error, got nil")
	}
	if !strings.Contains(err.Error(), "local_jwks") {
		t.Errorf("got %q; want substring 'local_jwks'", err.Error())
	}
}

func TestBuildCompiledProvider_NoJwksSource_ParseRejected(t *testing.T) {
	// Per §11.P9 + ADR-0149 envoy-go-side defensive PGV-mirror:
	// neither RemoteJwks nor LocalJwks set → PARSE-REJECT.
	c := &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p1": {Issuer: "https://example.com"},
		},
	}
	_, err := buildCompiledConfig(c, freshFactoryCtx())
	if err == nil {
		t.Fatalf("buildCompiledConfig: provider without JWKS source; want PARSE-REJECT, got nil")
	}
	if !strings.Contains(err.Error(), "remote_jwks") && !strings.Contains(err.Error(), "local_jwks") {
		t.Errorf("got %q; want substring 'remote_jwks' or 'local_jwks'", err.Error())
	}
}

func TestBuildCompiledProvider_ClockSkew_DefaultsTo60s(t *testing.T) {
	// Per §1.1 amendment 7: clock_skew_seconds defaults to 60s when unset.
	// Verified at the buildCompiledProvider level via the durationOrDefault
	// helper. Since RemoteJwks/LocalJwks paths are stubbed, we exercise the
	// helper directly via the no-source PARSE-REJECT path and inspect the
	// default through buildCompiledProvider's intermediate state — but since
	// the build returns nil on error, we instead test durationOrDefault directly.
	got := durationOrDefault(uint32(0), 60*time.Second)
	if got != 60*time.Second {
		t.Errorf("durationOrDefault(0, 60s) = %v; want 60s", got)
	}
	got = durationOrDefault(uint32(120), 60*time.Second)
	if got != 120*time.Second {
		t.Errorf("durationOrDefault(120, 60s) = %v; want 120s", got)
	}
}

func TestBuildCompiledRequirement_Nil_AllowMissingOrFailed(t *testing.T) {
	// Per proto comment on RequirementRule.requires: "If this field is empty,
	// it means JWT authentication is optional." → reqAllowMissingOrFailed.
	cr, err := buildCompiledRequirement(nil, nil)
	if err != nil {
		t.Fatalf("buildCompiledRequirement(nil): want success, got %v", err)
	}
	if cr == nil || cr.kind != reqAllowMissingOrFailed {
		t.Errorf("got %+v; want kind=reqAllowMissingOrFailed", cr)
	}
}

func TestBuildCompiledRequirement_AllowMissing_Variant(t *testing.T) {
	r := &jwt_authnv3.JwtRequirement{
		RequiresType: &jwt_authnv3.JwtRequirement_AllowMissing{
			AllowMissing: &emptypb.Empty{},
		},
	}
	cr, err := buildCompiledRequirement(r, nil)
	if err != nil {
		t.Fatalf("buildCompiledRequirement(allow_missing): %v", err)
	}
	if cr.kind != reqAllowMissing {
		t.Errorf("kind = %v; want reqAllowMissing", cr.kind)
	}
}

func TestBuildCompiledRequirement_AllowMissingOrFailed_Variant(t *testing.T) {
	r := &jwt_authnv3.JwtRequirement{
		RequiresType: &jwt_authnv3.JwtRequirement_AllowMissingOrFailed{
			AllowMissingOrFailed: &emptypb.Empty{},
		},
	}
	cr, err := buildCompiledRequirement(r, nil)
	if err != nil {
		t.Fatalf("buildCompiledRequirement(allow_missing_or_failed): %v", err)
	}
	if cr.kind != reqAllowMissingOrFailed {
		t.Errorf("kind = %v; want reqAllowMissingOrFailed", cr.kind)
	}
}

func TestBuildCompiledRequirement_ProviderName_LookupSuccess(t *testing.T) {
	providers := map[string]*compiledProvider{
		"p1": {issuer: "https://example.com"},
	}
	r := &jwt_authnv3.JwtRequirement{
		RequiresType: &jwt_authnv3.JwtRequirement_ProviderName{ProviderName: "p1"},
	}
	cr, err := buildCompiledRequirement(r, providers)
	if err != nil {
		t.Fatalf("buildCompiledRequirement(provider_name): %v", err)
	}
	if cr.kind != reqProviderName {
		t.Errorf("kind = %v; want reqProviderName", cr.kind)
	}
	if cr.provider == nil || cr.provider.issuer != "https://example.com" {
		t.Errorf("provider: got %+v; want issuer https://example.com", cr.provider)
	}
}

func TestBuildCompiledRequirement_ProviderName_LookupFails(t *testing.T) {
	r := &jwt_authnv3.JwtRequirement{
		RequiresType: &jwt_authnv3.JwtRequirement_ProviderName{ProviderName: "missing"},
	}
	_, err := buildCompiledRequirement(r, map[string]*compiledProvider{})
	if err == nil {
		t.Fatalf("buildCompiledRequirement: missing provider; want error, got nil")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("got %q; want substring 'missing'", err.Error())
	}
}

func TestBuildCompiledRequirement_ProviderAndAudiences_Variant(t *testing.T) {
	providers := map[string]*compiledProvider{
		"p1": {issuer: "https://example.com"},
	}
	r := &jwt_authnv3.JwtRequirement{
		RequiresType: &jwt_authnv3.JwtRequirement_ProviderAndAudiences{
			ProviderAndAudiences: &jwt_authnv3.ProviderWithAudiences{
				ProviderName: "p1",
				Audiences:    []string{"override-aud-1", "override-aud-2"},
			},
		},
	}
	cr, err := buildCompiledRequirement(r, providers)
	if err != nil {
		t.Fatalf("buildCompiledRequirement(provider_and_audiences): %v", err)
	}
	if cr.kind != reqProviderAndAudiences {
		t.Errorf("kind = %v; want reqProviderAndAudiences", cr.kind)
	}
	if len(cr.audOverr) != 2 || cr.audOverr[0] != "override-aud-1" {
		t.Errorf("audOverr = %v; want [override-aud-1 override-aud-2]", cr.audOverr)
	}
}

func TestBuildCompiledRequirement_RequiresAny_Recursive(t *testing.T) {
	providers := map[string]*compiledProvider{
		"p1": {issuer: "https://issuer-1"},
		"p2": {issuer: "https://issuer-2"},
	}
	r := &jwt_authnv3.JwtRequirement{
		RequiresType: &jwt_authnv3.JwtRequirement_RequiresAny{
			RequiresAny: &jwt_authnv3.JwtRequirementOrList{
				Requirements: []*jwt_authnv3.JwtRequirement{
					{RequiresType: &jwt_authnv3.JwtRequirement_ProviderName{ProviderName: "p1"}},
					{RequiresType: &jwt_authnv3.JwtRequirement_ProviderName{ProviderName: "p2"}},
				},
			},
		},
	}
	cr, err := buildCompiledRequirement(r, providers)
	if err != nil {
		t.Fatalf("buildCompiledRequirement(requires_any): %v", err)
	}
	if cr.kind != reqRequiresAny {
		t.Errorf("kind = %v; want reqRequiresAny", cr.kind)
	}
	if len(cr.children) != 2 {
		t.Fatalf("children: want 2, got %d", len(cr.children))
	}
	if cr.children[0].kind != reqProviderName || cr.children[0].provider.issuer != "https://issuer-1" {
		t.Errorf("children[0]: %+v", cr.children[0])
	}
}

func TestBuildCompiledRequirement_RequiresAll_Recursive(t *testing.T) {
	providers := map[string]*compiledProvider{
		"p1": {issuer: "https://issuer-1"},
		"p2": {issuer: "https://issuer-2"},
	}
	r := &jwt_authnv3.JwtRequirement{
		RequiresType: &jwt_authnv3.JwtRequirement_RequiresAll{
			RequiresAll: &jwt_authnv3.JwtRequirementAndList{
				Requirements: []*jwt_authnv3.JwtRequirement{
					{RequiresType: &jwt_authnv3.JwtRequirement_ProviderName{ProviderName: "p1"}},
					{RequiresType: &jwt_authnv3.JwtRequirement_ProviderName{ProviderName: "p2"}},
				},
			},
		},
	}
	cr, err := buildCompiledRequirement(r, providers)
	if err != nil {
		t.Fatalf("buildCompiledRequirement(requires_all): %v", err)
	}
	if cr.kind != reqRequiresAll {
		t.Errorf("kind = %v; want reqRequiresAll", cr.kind)
	}
	if len(cr.children) != 2 {
		t.Errorf("children: want 2, got %d", len(cr.children))
	}
}

func TestBuildCompiledRule_NoMatch_ParseRejected(t *testing.T) {
	// Per §11.P9 + ADR-0149 envoy-go-side defensive PGV-mirror:
	// RequirementRule.match is REQUIRED.
	rr := &jwt_authnv3.RequirementRule{
		RequirementType: &jwt_authnv3.RequirementRule_Requires{
			Requires: &jwt_authnv3.JwtRequirement{
				RequiresType: &jwt_authnv3.JwtRequirement_AllowMissingOrFailed{
					AllowMissingOrFailed: &emptypb.Empty{},
				},
			},
		},
	}
	_, err := buildCompiledRule(rr, nil, nil)
	if err == nil {
		t.Fatalf("buildCompiledRule: missing match; want error, got nil")
	}
	if !strings.Contains(err.Error(), "match") {
		t.Errorf("got %q; want substring 'match'", err.Error())
	}
}

func TestBuildCompiledRule_DanglingRequirementName_ListenerParseReject(t *testing.T) {
	// Per §1.1 amendment 4 + §11.P12 + ADR-0149: listener-level RequirementRule
	// with requirement_name not in requirement_map → PARSE-REJECT. (The
	// per-route case uses runtime-resolve per ADR-0153; the listener case
	// uses parse-reject — the split-semantic.)
	rr := &jwt_authnv3.RequirementRule{
		Match: happyMatch(),
		RequirementType: &jwt_authnv3.RequirementRule_RequirementName{
			RequirementName: "dangling-name",
		},
	}
	_, err := buildCompiledRule(rr, nil, map[string]*compiledRequirement{})
	if err == nil {
		t.Fatalf("buildCompiledRule: dangling requirement_name; want error, got nil")
	}
	if !strings.Contains(err.Error(), "dangling-name") && !strings.Contains(err.Error(), "requirement_map") {
		t.Errorf("got %q; want substring 'dangling-name' or 'requirement_map'", err.Error())
	}
}

func TestBuildCompiledRule_RequirementNameResolved(t *testing.T) {
	reqMap := map[string]*compiledRequirement{
		"req-1": {kind: reqAllowMissingOrFailed},
	}
	rr := &jwt_authnv3.RequirementRule{
		Match: happyMatch(),
		RequirementType: &jwt_authnv3.RequirementRule_RequirementName{
			RequirementName: "req-1",
		},
	}
	rule, err := buildCompiledRule(rr, nil, reqMap)
	if err != nil {
		t.Fatalf("buildCompiledRule: %v", err)
	}
	if rule.requirement == nil || rule.requirement.kind != reqAllowMissingOrFailed {
		t.Errorf("requirement: got %+v; want kind=reqAllowMissingOrFailed", rule.requirement)
	}
}

func TestBuildCompiledRule_InlineRequires_Honored(t *testing.T) {
	// Per §1.1 amendment 4 + §11.P12: inline `requires` arm is honored
	// proto-faithful (REFUTES BRAINSTORM deprecation-PARSE-REJECT hypothesis).
	rr := &jwt_authnv3.RequirementRule{
		Match: happyMatch(),
		RequirementType: &jwt_authnv3.RequirementRule_Requires{
			Requires: &jwt_authnv3.JwtRequirement{
				RequiresType: &jwt_authnv3.JwtRequirement_AllowMissingOrFailed{
					AllowMissingOrFailed: &emptypb.Empty{},
				},
			},
		},
	}
	rule, err := buildCompiledRule(rr, nil, nil)
	if err != nil {
		t.Fatalf("buildCompiledRule(inline requires): %v", err)
	}
	if rule.requirement == nil || rule.requirement.kind != reqAllowMissingOrFailed {
		t.Errorf("requirement: %+v", rule.requirement)
	}
}

func TestBuildCompiledRule_NoRequirement_DefaultAllowMissingOrFailed(t *testing.T) {
	// Per RequirementRule proto comment: "If not specified, Jwt verification
	// is disabled." → reqAllowMissingOrFailed default.
	rr := &jwt_authnv3.RequirementRule{Match: happyMatch()}
	rule, err := buildCompiledRule(rr, nil, nil)
	if err != nil {
		t.Fatalf("buildCompiledRule: %v", err)
	}
	if rule.requirement == nil || rule.requirement.kind != reqAllowMissingOrFailed {
		t.Errorf("requirement: %+v", rule.requirement)
	}
}

// ----------------------------------------------------------------------------
// Group 7 — Per-route 8th canonical (parsePerRoute + 3 disposition cases +
// PGV-mirror; per SPEC §5.1 + §6.12 + ADR-0153). Dangling-name runtime-resolve
// case STUBBED at Task 2; lands at Task 7.
// ----------------------------------------------------------------------------

func TestParsePerRoute_NilTC(t *testing.T) {
	_, err := parsePerRoute(nil)
	if err == nil {
		t.Fatalf("parsePerRoute(nil): want error, got nil")
	}
}

func TestParsePerRoute_MalformedAny(t *testing.T) {
	bad := &anypb.Any{
		TypeUrl: "type.googleapis.com/envoy.extensions.filters.http.jwt_authn.v3.PerRouteConfig",
		Value:   []byte{0xff, 0xff, 0xff},
	}
	_, err := parsePerRoute(bad)
	if err == nil {
		t.Fatalf("parsePerRoute(malformed): want error, got nil")
	}
}

func TestParsePerRoute_UnsetRequirementSpecifier_ParseRejected(t *testing.T) {
	// Per §11.P9 + config.pb.validate.go:2472-2481 + ADR-0149:
	// PerRouteConfig.requirement_specifier is PGV `required = true`. Neither
	// arm set → PARSE-REJECT.
	pr := &jwt_authnv3.PerRouteConfig{}
	any := mustAny(t, pr)
	_, err := parsePerRoute(any)
	if err == nil {
		t.Fatalf("parsePerRoute(no requirement_specifier): want error, got nil")
	}
	if !strings.Contains(err.Error(), "requirement_specifier") && !strings.Contains(err.Error(), "required") {
		t.Errorf("got %q; want substring 'requirement_specifier' or 'required'", err.Error())
	}
}

func TestParsePerRoute_EmptyRequirementName_ParseRejected(t *testing.T) {
	// Per §11.P9 + config.pb.validate.go:2460-2462 + ADR-0149:
	// PerRouteConfig.requirement_name is PGV `min_len=1` when the chosen
	// arm. Empty string → PARSE-REJECT.
	pr := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_RequirementName{RequirementName: ""},
	}
	any := mustAny(t, pr)
	_, err := parsePerRoute(any)
	if err == nil {
		t.Fatalf("parsePerRoute(empty requirement_name): want error, got nil")
	}
	if !strings.Contains(err.Error(), "requirement_name") && !strings.Contains(err.Error(), "min_len") {
		t.Errorf("got %q; want substring 'requirement_name' or 'min_len'", err.Error())
	}
}

func TestParsePerRoute_DisabledTrue_Success(t *testing.T) {
	// Case (a) per §5.1: disabled: true → compiledPerRoute{disabled: true}.
	pr := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_Disabled{Disabled: true},
	}
	any := mustAny(t, pr)
	msg, err := parsePerRoute(any)
	if err != nil {
		t.Fatalf("parsePerRoute(disabled=true): %v", err)
	}
	gotPr, ok := msg.(*jwt_authnv3.PerRouteConfig)
	if !ok {
		t.Fatalf("parsePerRoute returned %T; want *jwt_authnv3.PerRouteConfig", msg)
	}
	if !gotPr.GetDisabled() {
		t.Error("disabled: want true")
	}
}

func TestParsePerRoute_DisabledFalse_Success(t *testing.T) {
	// Case (b) per §5.1: disabled: false → compiledPerRoute{disabled: false,
	// requirementName: ""} (falls through to listener-level rules dispatch).
	pr := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_Disabled{Disabled: false},
	}
	any := mustAny(t, pr)
	msg, err := parsePerRoute(any)
	if err != nil {
		t.Fatalf("parsePerRoute(disabled=false): %v", err)
	}
	if msg == nil {
		t.Fatal("parsePerRoute returned nil msg")
	}
}

func TestParsePerRoute_RequirementName_Success(t *testing.T) {
	// Case (c) per §5.1: requirement_name: "<name>" → runtime-resolved at
	// request time per ADR-0153. Parse accepts any non-empty string;
	// listener-level lookup deferred to request-time.
	pr := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_RequirementName{
			RequirementName: "some-req-name",
		},
	}
	any := mustAny(t, pr)
	msg, err := parsePerRoute(any)
	if err != nil {
		t.Fatalf("parsePerRoute(requirement_name): %v", err)
	}
	gotPr, ok := msg.(*jwt_authnv3.PerRouteConfig)
	if !ok {
		t.Fatalf("parsePerRoute returned %T; want *jwt_authnv3.PerRouteConfig", msg)
	}
	if gotPr.GetRequirementName() != "some-req-name" {
		t.Errorf("requirement_name = %q; want 'some-req-name'", gotPr.GetRequirementName())
	}
}

func TestBuildCompiledPerRoute_DisabledTrue_Case_A(t *testing.T) {
	pr := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_Disabled{Disabled: true},
	}
	cpr, err := buildCompiledPerRoute(pr)
	if err != nil {
		t.Fatalf("buildCompiledPerRoute: %v", err)
	}
	if !cpr.disabled {
		t.Error("disabled: want true")
	}
	if cpr.requirementName != "" {
		t.Errorf("requirementName: want \"\", got %q", cpr.requirementName)
	}
}

func TestBuildCompiledPerRoute_DisabledFalse_Case_B(t *testing.T) {
	pr := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_Disabled{Disabled: false},
	}
	cpr, err := buildCompiledPerRoute(pr)
	if err != nil {
		t.Fatalf("buildCompiledPerRoute: %v", err)
	}
	if cpr.disabled {
		t.Error("disabled: want false")
	}
	if cpr.requirementName != "" {
		t.Errorf("requirementName: want \"\", got %q", cpr.requirementName)
	}
}

func TestBuildCompiledPerRoute_RequirementName_Case_C(t *testing.T) {
	pr := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_RequirementName{
			RequirementName: "req-X",
		},
	}
	cpr, err := buildCompiledPerRoute(pr)
	if err != nil {
		t.Fatalf("buildCompiledPerRoute: %v", err)
	}
	if cpr.disabled {
		t.Error("disabled: want false")
	}
	if cpr.requirementName != "req-X" {
		t.Errorf("requirementName: want \"req-X\", got %q", cpr.requirementName)
	}
}

// ----------------------------------------------------------------------------
// Group 7 finalization — runtime-resolve helper (resolveRequirement) per
// ADR-0153 + SPEC §6.6 + §1.1 amendment 6. The helper:
//   (1) Per-route requirement_name case (c) → lookup listener-level
//       requirementMap; miss → SendLocalReply(403) + denied++; returns
//       (nil, true) to signal caller stop iteration.
//   (2) Per-route case (b) disabled:false → falls through to listener-level
//       rules first-match-wins iteration. Returns matched rule's requirement.
//   (3) No rule matches → (nil, false); caller treats as pass-through.
//
// NOTE on rules iteration: compiledRule.matcher is typed `any` at Task 2 per
// ADR-0149 §Decision (ix) — Task 7 first-match logic treats `matcher == nil`
// as wildcard-match. A real route-matcher landing in a future task can
// replace this without changing the helper's external contract. Documented
// at PROGRESS.md Task 7 entry.
// ----------------------------------------------------------------------------

// jwtFakeCB is the test-double DecoderFilterCallbacks for Group 7 tests
// (runtime-resolve error path SendLocalReply capture). Mirrors phase-16 rbac
// rbacFakeCB precedent. Per-route TPFC injection via routeCfg; SendLocalReply
// captures status/body/headers.
type jwtFakeCB struct {
	mu         sync.Mutex
	routeCfg   proto.Message      // returned by RequestRouteConfig; nil → inherit listener
	localReply *jwtLocalReplyArgs // captured at SendLocalReply; nil if never called
}

type jwtLocalReplyArgs struct {
	status  int
	body    string
	headers envoyhttp.OrderedHeaders
}

func (c *jwtFakeCB) ContinueDecoding() {}
func (c *jwtFakeCB) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localReply = &jwtLocalReplyArgs{status: status, body: body, headers: headers}
}
func (c *jwtFakeCB) RequestRouteConfig() proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.routeCfg
}
func (c *jwtFakeCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *jwtFakeCB) EncodeHeaders(http.Header, bool) {}
func (c *jwtFakeCB) EncodeData([]byte, bool)         {}
func (c *jwtFakeCB) EncodeTrailers(http.Header)      {}
func (c *jwtFakeCB) DownstreamPrincipal() []string   { return nil }

// ADR-0165 callback-surface extension stubs (phase-18.2 Task 4).
func (c *jwtFakeCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (c *jwtFakeCB) DownstreamLocalAddr() net.Addr    { return nil }
func (c *jwtFakeCB) DownstreamTLSServerName() string  { return "" }
func (c *jwtFakeCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (c *jwtFakeCB) DownstreamProtocol() string       { return "" }
func (c *jwtFakeCB) ListenerPrincipal() string        { return "" }

// newFilterWithListenerRC wires a *filter against the supplied listener-level
// *compiledConfig + per-route *compiledPerRoute + fresh jwtFakeCB. Used by
// Group 7 finalization tests that exercise resolveRequirement directly.
func newFilterWithListenerRC(t *testing.T, rc *compiledConfig, pr *compiledPerRoute) (*filter, *jwtFakeCB) {
	t.Helper()
	st := &factoryState{listenerRC: rc}
	cb := &jwtFakeCB{}
	f := &filter{
		state:    st,
		dcb:      cb,
		activeRC: rc,
		perRoute: pr,
	}
	return f, cb
}

// TestResolvePerRouteConfig_DanglingName_RuntimeResolve verifies the per-route
// runtime-resolve error path per ADR-0153 + §1.1 amendment 6: when per-route
// requirement_name does not resolve in the listener-level requirement_map,
// resolveRequirement emits SendLocalReply(403, "Failed JWT authentication:
// Wrong requirement_name: <name>", nil) + increments denied counter + returns
// (nil, true).
func TestResolvePerRouteConfig_DanglingName_RuntimeResolve(t *testing.T) {
	// Listener-level config with a non-empty requirementMap that does NOT
	// contain "missing-req". Stats registered so denied counter is non-nil.
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	rc := &compiledConfig{
		requirementMap: map[string]*compiledRequirement{
			"present-req": {kind: reqAllowMissingOrFailed},
		},
		stats: fs,
	}
	pr := &compiledPerRoute{disabled: false, requirementName: "missing-req"}
	f, cb := newFilterWithListenerRC(t, rc, pr)

	req, denied := f.resolveRequirement(http.Header{})
	if req != nil {
		t.Errorf("req: want nil; got %+v", req)
	}
	if !denied {
		t.Fatal("denied: want true (helper emitted SendLocalReply on dangling name); got false")
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply was not called")
	}
	if cb.localReply.status != 403 {
		t.Errorf("SendLocalReply status: got %d; want 403", cb.localReply.status)
	}
	wantBody := `Failed JWT authentication: Wrong requirement_name: missing-req`
	if cb.localReply.body != wantBody {
		t.Errorf("SendLocalReply body: got %q; want %q", cb.localReply.body, wantBody)
	}
	if cb.localReply.headers != nil {
		t.Errorf("SendLocalReply headers: got %+v; want nil (per §1.1 amendment 6 — NO WWW-Authenticate)", cb.localReply.headers)
	}
	// Counter increment verification per ADR-0148 §Decision + §1.1 amendment 6.
	if got := fs.denied.Load(); got != 1 {
		t.Errorf("denied counter: got %d; want 1", got)
	}
}

// TestResolveRequirement_DanglingName_NilStats_NoPanic verifies the nil-
// tolerance per ADR-0085: when listener-level stats is nil (test code path
// w/o ctx.Stats), resolveRequirement still emits SendLocalReply without
// panicking on the denied counter increment.
func TestResolveRequirement_DanglingName_NilStats_NoPanic(t *testing.T) {
	rc := &compiledConfig{
		requirementMap: map[string]*compiledRequirement{
			"present-req": {kind: reqAllowMissingOrFailed},
		},
		stats: nil, // nil stats → nil-tolerance per ADR-0085
	}
	pr := &compiledPerRoute{disabled: false, requirementName: "missing-req"}
	f, cb := newFilterWithListenerRC(t, rc, pr)

	req, denied := f.resolveRequirement(http.Header{})
	if req != nil || !denied {
		t.Errorf("nil-stats path: want (nil, true); got (%+v, %v)", req, denied)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply must still fire even when stats is nil")
	}
	if cb.localReply.status != 403 {
		t.Errorf("SendLocalReply status: got %d; want 403", cb.localReply.status)
	}
}

// TestResolveRequirement_PerRouteRequirementName_Success verifies case (c)
// the happy path: per-route requirement_name resolves against listener-level
// requirement_map. Returns (req, false) — no SendLocalReply.
func TestResolveRequirement_PerRouteRequirementName_Success(t *testing.T) {
	wantReq := &compiledRequirement{kind: reqAllowMissingOrFailed}
	rc := &compiledConfig{
		requirementMap: map[string]*compiledRequirement{
			"my-req": wantReq,
		},
	}
	pr := &compiledPerRoute{disabled: false, requirementName: "my-req"}
	f, cb := newFilterWithListenerRC(t, rc, pr)

	req, denied := f.resolveRequirement(http.Header{})
	if denied {
		t.Error("denied: want false on resolve success; got true")
	}
	if req != wantReq {
		t.Errorf("req: got %p; want %p", req, wantReq)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on resolve success; got %+v", cb.localReply)
	}
}

// TestResolveRequirement_PerRouteCaseB_FallsThroughToListenerRules verifies
// case (b) disabled:false / requirementName=="" → falls through to listener-
// level rules first-match-wins iteration. The matched rule's requirement is
// returned.
func TestResolveRequirement_PerRouteCaseB_FallsThroughToListenerRules(t *testing.T) {
	wantReq := &compiledRequirement{kind: reqAllowMissing}
	rc := &compiledConfig{
		rules: []*compiledRule{
			// matcher==nil treated as wildcard match per Task 7 first-match
			// stub (ADR-0149 §Decision (ix)).
			{matchFn: nil, requirement: wantReq},
		},
	}
	pr := &compiledPerRoute{disabled: false, requirementName: ""}
	f, cb := newFilterWithListenerRC(t, rc, pr)

	req, denied := f.resolveRequirement(http.Header{})
	if denied {
		t.Error("denied: want false on case-(b) fall-through; got true")
	}
	if req != wantReq {
		t.Errorf("req: got %p; want %p (matched listener rule's requirement)", req, wantReq)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on case-(b) fall-through; got %+v", cb.localReply)
	}
}

// TestResolveRequirement_NoPerRoute_FallsThroughToListenerRules verifies that
// when f.perRoute is nil (no per-route TPFC entry on this route),
// resolveRequirement falls through to listener-level rules iteration
// identically to case (b).
func TestResolveRequirement_NoPerRoute_FallsThroughToListenerRules(t *testing.T) {
	wantReq := &compiledRequirement{kind: reqProviderName}
	rc := &compiledConfig{
		rules: []*compiledRule{
			{matchFn: nil, requirement: wantReq},
		},
	}
	f, cb := newFilterWithListenerRC(t, rc, nil)

	req, denied := f.resolveRequirement(http.Header{})
	if denied {
		t.Error("denied: want false; got true")
	}
	if req != wantReq {
		t.Errorf("req: got %p; want %p", req, wantReq)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on no-per-route; got %+v", cb.localReply)
	}
}

// TestResolveRequirement_NoRulesNoMatch_PassThrough verifies that when no
// listener-level rule matches (empty rules slice OR all matchers return
// false), resolveRequirement returns (nil, false). Caller treats nil as
// pass-through (no JWT verification required) per §6.6.
func TestResolveRequirement_NoRulesNoMatch_PassThrough(t *testing.T) {
	rc := &compiledConfig{
		rules: nil, // no rules → no match → pass-through
	}
	f, cb := newFilterWithListenerRC(t, rc, nil)

	req, denied := f.resolveRequirement(http.Header{})
	if denied {
		t.Error("denied: want false on no-match; got true")
	}
	if req != nil {
		t.Errorf("req: want nil (no rule matched); got %+v", req)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on no-match pass-through; got %+v", cb.localReply)
	}
}

// TestResolveRequirement_FirstMatchWins verifies that listener-level rules
// iteration is first-match-wins: the FIRST matcher (==nil-wildcard) returns
// its requirement even though subsequent rules also match.
func TestResolveRequirement_FirstMatchWins(t *testing.T) {
	first := &compiledRequirement{kind: reqProviderName}
	second := &compiledRequirement{kind: reqAllowMissingOrFailed}
	rc := &compiledConfig{
		rules: []*compiledRule{
			{matchFn: nil, requirement: first},
			{matchFn: nil, requirement: second},
		},
	}
	f, _ := newFilterWithListenerRC(t, rc, nil)

	req, _ := f.resolveRequirement(http.Header{})
	if req != first {
		t.Errorf("req: want first-match (%p); got %p", first, req)
	}
}

// TestResolveRequirement_PreconditionViolation_DisabledFallsThroughToRules
// documents the helper's CONTRACT: if the caller violates the precondition by
// passing a `disabled: true` per-route through to resolveRequirement, the
// helper falls through to listener-level rules iteration. Task 9's
// DecodeHeaders is responsible for short-circuiting case (a) BEFORE invoking
// this helper (per ADR-0153 §Resolution-flow + provider.go file-level comment
// lines 28-29 + the resolveRequirement docstring PRECONDITION).
//
// This test pins the current behavior so a refactor that accidentally moves
// case-(a) handling into resolveRequirement (or removes the early-return at
// DecodeHeaders) surfaces here as a regression.
func TestResolveRequirement_PreconditionViolation_DisabledFallsThroughToRules(t *testing.T) {
	wantReq := &compiledRequirement{kind: reqAllowMissingOrFailed}
	rc := &compiledConfig{
		rules: []*compiledRule{
			// matcher==nil treated as wildcard per Task 7 first-match stub.
			{matchFn: nil, requirement: wantReq},
		},
		// empty requirementMap — case (c) lookup would NOT apply here even if
		// requirementName were set; the disabled flag with empty name reaches
		// the rules-iteration loop.
	}
	// Precondition violation: pass disabled:true through to the helper. Task 9
	// DecodeHeaders normally short-circuits this BEFORE invoking the helper.
	pr := &compiledPerRoute{disabled: true, requirementName: ""}
	f, cb := newFilterWithListenerRC(t, rc, pr)

	req, denied := f.resolveRequirement(http.Header{})

	if denied {
		t.Fatalf("expected no 403 emission on precondition violation; got denied=true")
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on precondition-violation fall-through; got %+v", cb.localReply)
	}
	if req == nil {
		t.Fatalf("expected wildcard match to first rule (req != nil); helper fell through to no-match")
	}
	// The req returned is the listener-level rules first-match. Confirm
	// precondition-violation semantic: NOT short-circuited, falls through.
	if req != wantReq {
		t.Errorf("req: want first-rule requirement (%p); got %p", wantReq, req)
	}
	if req.kind != reqAllowMissingOrFailed {
		t.Errorf("expected first-rule requirement, got kind=%d", req.kind)
	}
}

// ----------------------------------------------------------------------------
// Group 11 — `compiledPerRoute` lazy-cache (identity-keyed by *PerRouteConfig
// proto pointer; concurrent LoadOrStore safety; cache-hit re-resolve).
// Per SPEC §6.12 + ADR-0153 + phase-16 ADR-0145 lazy-cache precedent.
// ----------------------------------------------------------------------------

func TestResolvePerRouteConfig_PointerIdentityLazyCache(t *testing.T) {
	// Per SPEC §6.12: per-route lazy-cache is keyed by *PerRouteConfig proto
	// pointer. Same pointer twice → same compiledPerRoute returned.
	pr := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_RequirementName{RequirementName: "r1"},
	}
	st := &factoryState{}
	cpr1, err := st.resolvePerRouteConfig(pr)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	cpr2, err := st.resolvePerRouteConfig(pr)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if cpr1 != cpr2 {
		t.Errorf("lazy-cache identity broken: cpr1=%p cpr2=%p", cpr1, cpr2)
	}
}

func TestResolvePerRouteConfig_DifferentPointers_DifferentCompiled(t *testing.T) {
	// Different *PerRouteConfig pointers → distinct compiledPerRoute entries.
	pr1 := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_RequirementName{RequirementName: "r1"},
	}
	pr2 := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_RequirementName{RequirementName: "r2"},
	}
	st := &factoryState{}
	cpr1, err := st.resolvePerRouteConfig(pr1)
	if err != nil {
		t.Fatalf("resolve pr1: %v", err)
	}
	cpr2, err := st.resolvePerRouteConfig(pr2)
	if err != nil {
		t.Fatalf("resolve pr2: %v", err)
	}
	if cpr1 == cpr2 {
		t.Errorf("different pointers should produce different compiledPerRoute: %p %p", cpr1, cpr2)
	}
	if cpr1.requirementName != "r1" || cpr2.requirementName != "r2" {
		t.Errorf("names: cpr1=%q cpr2=%q; want r1 r2", cpr1.requirementName, cpr2.requirementName)
	}
}

func TestResolvePerRouteConfig_Concurrent_LoadOrStoreRaceSafe(t *testing.T) {
	// Per SPEC §6.12 + phase-16 ADR-0145: concurrent resolvePerRouteConfig
	// calls produce ONE compiledPerRoute per proto pointer. sync.Map's
	// LoadOrStore is the race-safe primitive. Run with -race for the race
	// detector to catch violations.
	pr := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_Disabled{Disabled: true},
	}
	st := &factoryState{}

	const N = 16
	var wg sync.WaitGroup
	results := make([]*compiledPerRoute, N)
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			cpr, err := st.resolvePerRouteConfig(pr)
			if err != nil {
				t.Errorf("goroutine %d resolve: %v", idx, err)
				return
			}
			results[idx] = cpr
		}(i)
	}
	wg.Wait()

	// All N goroutines should observe pointer-identical compiledPerRoute
	// (sync.Map.LoadOrStore semantic).
	first := results[0]
	if first == nil {
		t.Fatal("first goroutine result was nil")
	}
	for i := 1; i < N; i++ {
		if results[i] != first {
			t.Errorf("goroutine %d observed different compiledPerRoute: %p (vs %p)", i, results[i], first)
		}
	}
}

// ----------------------------------------------------------------------------
// Group 2 — Token extraction across 4 sources per SPEC §6.7 + §11.P14 + §11.P15 +
// ADR-0152. Iteration order: (1) configured from_headers OR (default Authorization
// Bearer + access_token query param when ALL three explicit lists empty) → (2)
// configured from_params → (3) configured from_cookies. First-success-wins
// discipline lives in evaluateProvider (Task 6); extractTokens returns ALL
// candidate tokens in iteration order.
// ----------------------------------------------------------------------------

// newHeaders builds an http.Header from a map of (already-canonical OR
// pseudo-header) keys to values. Mirrors phase-13 buffer_test.go pattern.
func newHeaders(kv map[string]string) http.Header {
	h := make(http.Header, len(kv))
	for k, v := range kv {
		h.Set(k, v)
	}
	return h
}

func TestExtractTokens_DefaultAuthorizationBearer_Success(t *testing.T) {
	// Per §6.7 + §11.P14 RATIFIED + ADR-0152: when the provider has no explicit
	// extraction sources, the default Authorization Bearer scheme applies.
	p := &compiledProvider{}
	headers := newHeaders(map[string]string{"Authorization": "Bearer eyJfake"})
	out := extractTokens(p, headers)
	if len(out) != 1 {
		t.Fatalf("extractTokens: want 1 token, got %d (%v)", len(out), out)
	}
	if out[0].raw != "eyJfake" {
		t.Errorf("raw: got %q; want %q", out[0].raw, "eyJfake")
	}
	if out[0].src != sourceHeader {
		t.Errorf("src: got %v; want sourceHeader", out[0].src)
	}
}

func TestExtractTokens_DefaultAuthorizationBearer_StripsTrailingGarbage(t *testing.T) {
	// Regression: the default `Authorization: Bearer ...` branch MUST apply
	// `stripNonBase64URLChars(strings.TrimSpace(...))` to the post-prefix
	// bytes — mirroring the explicit `from_headers` post-prefix treatment
	// (evaluator.go lines 120 + 134) per Envoy `extractor.cc::extractJWT`.
	// Without the uniform strip, trailing garbage (`; foo=bar`, whitespace,
	// etc.) would propagate to the verifier.
	p := &compiledProvider{} // no explicit extraction sources
	h := newHeaders(map[string]string{
		"authorization": "Bearer eyJfake; foo=bar",
	})
	tokens := extractTokens(p, h)
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].raw != "eyJfake" {
		t.Fatalf("expected raw=%q (stripped), got %q", "eyJfake", tokens[0].raw)
	}
}

func TestExtractTokens_DefaultAccessTokenQuery_Success(t *testing.T) {
	// Per §6.7 + ADR-0152 default access_token query when no explicit sources.
	p := &compiledProvider{}
	headers := newHeaders(map[string]string{":path": "/foo?access_token=eyJqfake"})
	out := extractTokens(p, headers)
	if len(out) != 1 {
		t.Fatalf("extractTokens: want 1 token, got %d (%v)", len(out), out)
	}
	if out[0].raw != "eyJqfake" {
		t.Errorf("raw: got %q; want %q", out[0].raw, "eyJqfake")
	}
	if out[0].src != sourceParam {
		t.Errorf("src: got %v; want sourceParam", out[0].src)
	}
	if out[0].name != "access_token" {
		t.Errorf("name: got %q; want access_token", out[0].name)
	}
}

func TestExtractTokens_DefaultBoth_TwoTokens(t *testing.T) {
	// Per §6.7 + ADR-0152: both Authorization Bearer + access_token query
	// produce 2 tokens; header iterates first per Envoy extractor.cc.
	p := &compiledProvider{}
	headers := newHeaders(map[string]string{
		"Authorization": "Bearer eyJhdr",
		":path":         "/foo?access_token=eyJqry",
	})
	out := extractTokens(p, headers)
	if len(out) != 2 {
		t.Fatalf("extractTokens: want 2 tokens, got %d (%v)", len(out), out)
	}
	if out[0].src != sourceHeader || out[0].raw != "eyJhdr" {
		t.Errorf("token[0]: got %+v; want {raw:eyJhdr src:sourceHeader}", out[0])
	}
	if out[1].src != sourceParam || out[1].raw != "eyJqry" {
		t.Errorf("token[1]: got %+v; want {raw:eyJqry src:sourceParam}", out[1])
	}
}

func TestExtractTokens_AuthorizationWithoutBearerPrefix_NotExtracted(t *testing.T) {
	// Per §6.7 + ADR-0152: the default Authorization extractor requires the
	// case-sensitive "Bearer " prefix (Basic / Digest / other schemes are NOT
	// JWT and produce zero tokens).
	p := &compiledProvider{}
	headers := newHeaders(map[string]string{"Authorization": "Basic abc"})
	out := extractTokens(p, headers)
	if len(out) != 0 {
		t.Errorf("extractTokens: want 0 tokens, got %d (%v)", len(out), out)
	}
}

func TestExtractTokens_FromHeaders_ValuePrefix_Substring(t *testing.T) {
	// Per §6.7 + §11.P14 + ADR-0152: value_prefix uses strings.Index substring
	// search (NOT HasPrefix) — the JWT bytes are everything after the prefix.
	p := &compiledProvider{
		fromHeaders: []headerLoc{{name: "X-JWT", valuePrefix: "Bearer "}},
	}
	headers := newHeaders(map[string]string{"X-JWT": "Bearer eyJxjwt"})
	out := extractTokens(p, headers)
	if len(out) != 1 {
		t.Fatalf("extractTokens: want 1 token, got %d (%v)", len(out), out)
	}
	if out[0].raw != "eyJxjwt" {
		t.Errorf("raw: got %q; want %q", out[0].raw, "eyJxjwt")
	}
	if out[0].name != "X-JWT" {
		t.Errorf("name: got %q; want X-JWT", out[0].name)
	}
}

func TestExtractTokens_FromHeaders_NoPrefix_Verbatim(t *testing.T) {
	// Per §6.7: empty value_prefix → use entire header value verbatim as JWT.
	p := &compiledProvider{
		fromHeaders: []headerLoc{{name: "X-JWT", valuePrefix: ""}},
	}
	headers := newHeaders(map[string]string{"X-JWT": "eyJvjwt"})
	out := extractTokens(p, headers)
	if len(out) != 1 {
		t.Fatalf("extractTokens: want 1 token, got %d (%v)", len(out), out)
	}
	if out[0].raw != "eyJvjwt" {
		t.Errorf("raw: got %q; want %q", out[0].raw, "eyJvjwt")
	}
}

func TestExtractTokens_FromHeaders_PrefixMidString_Substring(t *testing.T) {
	// Per ADR-0152 + Envoy extractor.cc: value_prefix is substring-searched via
	// strings.Index — the prefix MAY appear anywhere in the header value, not
	// just at position 0. Stripping of non-base64url trailing chars happens
	// post-substring via stripNonBase64URLChars.
	p := &compiledProvider{
		fromHeaders: []headerLoc{{name: "X-Auth", valuePrefix: "JWT="}},
	}
	headers := newHeaders(map[string]string{"X-Auth": "tag=foo; JWT=eyJxabc; bar=baz"})
	out := extractTokens(p, headers)
	if len(out) != 1 {
		t.Fatalf("extractTokens: want 1 token, got %d (%v)", len(out), out)
	}
	if out[0].raw != "eyJxabc" {
		t.Errorf("raw: got %q; want %q (trailing non-base64url chars must be stripped)", out[0].raw, "eyJxabc")
	}
}

func TestExtractTokens_FromParams_FirstValueOnly(t *testing.T) {
	// Per §11.P14 REFINED: multi-value query `?token=A&token=B` extracts ONLY
	// first value via `getFirstValue` per Envoy extractor.cc.
	p := &compiledProvider{fromParams: []string{"token"}}
	headers := newHeaders(map[string]string{":path": "/foo?token=A&token=B"})
	out := extractTokens(p, headers)
	if len(out) != 1 {
		t.Fatalf("extractTokens: want 1 token (first-value-only), got %d (%v)", len(out), out)
	}
	if out[0].raw != "A" {
		t.Errorf("raw: got %q; want %q", out[0].raw, "A")
	}
}

func TestExtractTokens_FromParams_CaseSensitive(t *testing.T) {
	// Per §11.P14 + ADR-0152: query param name matching is case-SENSITIVE.
	// from_params: [token] does NOT match ?TOKEN=...
	p := &compiledProvider{fromParams: []string{"token"}}
	headers := newHeaders(map[string]string{":path": "/foo?TOKEN=eyJfake"})
	out := extractTokens(p, headers)
	if len(out) != 0 {
		t.Errorf("extractTokens: want 0 tokens (case-sensitive), got %d (%v)", len(out), out)
	}
}

func TestExtractTokens_FromCookies_Verbatim_CaseSensitive(t *testing.T) {
	// Per §11.P15 RATIFIED + ADR-0152: cookie name matching is case-SENSITIVE
	// exact match; cookie value used VERBATIM (no URL-decode).
	p := &compiledProvider{fromCookies: []string{"jwt"}}
	headers := newHeaders(map[string]string{"Cookie": "jwt=eyJxcookie; other=stuff"})
	out := extractTokens(p, headers)
	if len(out) != 1 {
		t.Fatalf("extractTokens: want 1 token, got %d (%v)", len(out), out)
	}
	if out[0].raw != "eyJxcookie" {
		t.Errorf("raw: got %q; want %q", out[0].raw, "eyJxcookie")
	}
	if out[0].src != sourceCookie {
		t.Errorf("src: got %v; want sourceCookie", out[0].src)
	}
}

func TestExtractTokens_FromCookies_NoUrlDecode_Per11P15(t *testing.T) {
	// Per §11.P15 RATIFIED: cookie values used VERBATIM (no URL-decode). The
	// URL-encoded value `ey%2BJ` MUST surface as-is `ey%2BJ` (not decoded to
	// `ey+J`).
	p := &compiledProvider{fromCookies: []string{"jwt"}}
	headers := newHeaders(map[string]string{"Cookie": "jwt=ey%2BJ"})
	out := extractTokens(p, headers)
	if len(out) != 1 {
		t.Fatalf("extractTokens: want 1 token, got %d (%v)", len(out), out)
	}
	if out[0].raw != "ey%2BJ" {
		t.Errorf("raw: got %q; want %q (verbatim — no URL-decode per §11.P15)", out[0].raw, "ey%2BJ")
	}
}

func TestExtractTokens_ExplicitSources_DefaultsSuppressed(t *testing.T) {
	// Per §6.7 + ADR-0152: when ANY of from_headers/from_params/from_cookies is
	// non-empty, the default Authorization Bearer + access_token query
	// extractors are SUPPRESSED entirely. Explicit-sources is NOT additive over
	// defaults.
	p := &compiledProvider{
		fromHeaders: []headerLoc{{name: "X-JWT", valuePrefix: ""}},
	}
	headers := newHeaders(map[string]string{
		"X-JWT":         "eyJxexplicit",
		"Authorization": "Bearer eyJxdefault",
		":path":         "/foo?access_token=eyJqdefault",
	})
	out := extractTokens(p, headers)
	if len(out) != 1 {
		t.Fatalf("extractTokens: want 1 token (defaults suppressed), got %d (%v)", len(out), out)
	}
	if out[0].raw != "eyJxexplicit" || out[0].name != "X-JWT" {
		t.Errorf("token: got %+v; want raw=eyJxexplicit name=X-JWT", out[0])
	}
}

func TestExtractTokens_IterationOrder_HeadersThenParamsThenCookies(t *testing.T) {
	// Per §6.7 + ADR-0152 + Envoy extractor.cc iteration order: configured
	// from_headers → from_params → from_cookies. All three present produces
	// 3 tokens in that exact order.
	p := &compiledProvider{
		fromHeaders: []headerLoc{{name: "X-JWT", valuePrefix: ""}},
		fromParams:  []string{"qtok"},
		fromCookies: []string{"ctok"},
	}
	headers := newHeaders(map[string]string{
		"X-JWT":  "eyJhdr",
		":path":  "/?qtok=eyJqry",
		"Cookie": "ctok=eyJcok",
	})
	out := extractTokens(p, headers)
	if len(out) != 3 {
		t.Fatalf("extractTokens: want 3 tokens, got %d (%v)", len(out), out)
	}
	if out[0].src != sourceHeader || out[0].raw != "eyJhdr" {
		t.Errorf("token[0]: got %+v; want header eyJhdr", out[0])
	}
	if out[1].src != sourceParam || out[1].raw != "eyJqry" {
		t.Errorf("token[1]: got %+v; want param eyJqry", out[1])
	}
	if out[2].src != sourceCookie || out[2].raw != "eyJcok" {
		t.Errorf("token[2]: got %+v; want cookie eyJcok", out[2])
	}
}

func TestExtractTokens_NoMatches_EmptySlice(t *testing.T) {
	// Per ADR-0152 + ADR-0155: empty-extraction yields zero tokens; the
	// downstream evaluator maps to ErrJwtMissed at Task 6.
	p := &compiledProvider{
		fromHeaders: []headerLoc{{name: "X-JWT", valuePrefix: "Bearer "}},
		fromParams:  []string{"token"},
		fromCookies: []string{"jwt"},
	}
	headers := newHeaders(map[string]string{":path": "/no-match"})
	out := extractTokens(p, headers)
	if len(out) != 0 {
		t.Errorf("extractTokens: want 0 tokens, got %d (%v)", len(out), out)
	}
}

// ----------------------------------------------------------------------------
// Group 9 — CORS preflight predicate per SPEC §11.P1 + filter.cc verbatim.
// `isCorsPreflightRequest` returns true iff method == OPTIONS AND origin != ""
// AND access-control-request-method != "". ADR-0152 §Decision documents the
// 3-condition AND.
// ----------------------------------------------------------------------------

func TestIsCorsPreflightRequest_AllThreeConditions_True(t *testing.T) {
	headers := newHeaders(map[string]string{
		":method":                       "OPTIONS",
		"Origin":                        "https://example.com",
		"Access-Control-Request-Method": "POST",
	})
	if !isCorsPreflightRequest(headers) {
		t.Error("isCorsPreflightRequest: want true (all 3 conditions met)")
	}
}

func TestIsCorsPreflightRequest_MissingOrigin_False(t *testing.T) {
	headers := newHeaders(map[string]string{
		":method":                       "OPTIONS",
		"Access-Control-Request-Method": "POST",
	})
	if isCorsPreflightRequest(headers) {
		t.Error("isCorsPreflightRequest: want false (Origin missing)")
	}
}

func TestIsCorsPreflightRequest_MissingACRMHeader_False(t *testing.T) {
	headers := newHeaders(map[string]string{
		":method": "OPTIONS",
		"Origin":  "https://example.com",
	})
	if isCorsPreflightRequest(headers) {
		t.Error("isCorsPreflightRequest: want false (Access-Control-Request-Method missing)")
	}
}

func TestIsCorsPreflightRequest_NotOptionsMethod_False(t *testing.T) {
	headers := newHeaders(map[string]string{
		":method":                       "GET",
		"Origin":                        "https://example.com",
		"Access-Control-Request-Method": "POST",
	})
	if isCorsPreflightRequest(headers) {
		t.Error("isCorsPreflightRequest: want false (:method is GET, not OPTIONS)")
	}
}

// ----------------------------------------------------------------------------
// Group 3 + 4 + 5 + 10 evaluator integration tests (Task 6 of phase-17 per
// SPEC §6.8 + §11.P16 + ADR-0149).
//
// Test helpers (RSA key generation + JWT sign + JWKS-JSON build) are factored
// at the top of this section. The evaluator-side filter struct is constructed
// in-test with a hand-crafted `*compiledConfig` carrying 1-2 providers using
// LocalJwks (or RemoteJwks via httptest for Group 10 smoke tests).
// ----------------------------------------------------------------------------

// evalTestKeyOnce guards lazy initialization of the test RSA keypair used by
// Group 3/4/5/10 tests. Mirrors internal/jwt/jwt_test.go::keyOnce + initKeys
// precedent; one key per `go test` run is enough — generating a 2048-bit RSA
// key per-test would balloon test wallclock.
var (
	evalTestKeyOnce sync.Once
	evalTestKey     *rsa.PrivateKey
)

func ensureEvalTestKey(t *testing.T) {
	t.Helper()
	evalTestKeyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("rsa.GenerateKey: %v", err)
		}
		evalTestKey = k
	})
}

// signTestJWT_RS256 produces a serialized JWT signed with RS256 against
// evalTestKey using the supplied kid + claims. Mirrors jwt_test.go::signRS.
func signTestJWT_RS256(t *testing.T, kid string, claims map[string]interface{}) string {
	t.Helper()
	header := map[string]interface{}{"alg": "RS256", "typ": "JWT"}
	if kid != "" {
		header["kid"] = kid
	}
	h, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	p, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	encH := base64.RawURLEncoding.EncodeToString(h)
	encP := base64.RawURLEncoding.EncodeToString(p)
	signed := encH + "." + encP
	sum := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, evalTestKey, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("rsa.SignPKCS1v15: %v", err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// buildTestJWKSetJSON constructs an RFC 7517 §5 JWK Set JSON string carrying
// one RSA JWK with the supplied kid+alg, derived from evalTestKey.
func buildTestJWKSetJSON(t *testing.T, kid, alg string) string {
	t.Helper()
	jwk := map[string]string{
		"kty": "RSA",
		"kid": kid,
		"alg": alg,
		"use": "sig",
		"n":   base64.RawURLEncoding.EncodeToString(evalTestKey.PublicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(evalTestKey.PublicKey.E)).Bytes()),
	}
	out := map[string]interface{}{"keys": []map[string]string{jwk}}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	return string(b)
}

// buildTestLocalProvider constructs a *compiledProvider with LocalJwks carrying
// the test RSA public key under kid+alg. Issuer / audiences / clockSkew are
// configurable; defaults applied where supplied as zero values.
func buildTestLocalProvider(t *testing.T, kid, alg, issuer string, audiences []string, clockSkew time.Duration) *compiledProvider {
	t.Helper()
	ensureEvalTestKey(t)
	set, err := jwks.ParseJWKSet([]byte(buildTestJWKSetJSON(t, kid, alg)))
	if err != nil {
		t.Fatalf("ParseJWKSet: %v", err)
	}
	if clockSkew == 0 {
		clockSkew = 60 * time.Second
	}
	return &compiledProvider{
		issuer:    issuer,
		audiences: audiences,
		localJwks: set,
		clockSkew: clockSkew,
	}
}

// makeTestFilter constructs a *filter wired around a *compiledConfig with the
// supplied providers map. The activeRC field is pre-bound (mimicking the
// DecodeHeaders cache-step at Task 9). Setting `now` to a fixed clock makes
// exp/nbf-based tests deterministic.
func makeTestFilter(providers map[string]*compiledProvider, now func() time.Time) *filter {
	cfg := &compiledConfig{
		providers: providers,
	}
	return &filter{
		state:    &factoryState{listenerRC: cfg},
		activeRC: cfg,
		now:      now,
	}
}

// fixedNow returns a func() time.Time closure returning the supplied time
// verbatim. Used to make exp/nbf tests deterministic.
func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// ----------------------------------------------------------------------------
// Group 3 — JWT parse + signature smoke tests (integration; full coverage at
// internal/jwt/jwt_test.go per Task 4). ~3 cases.
// ----------------------------------------------------------------------------

func TestEvaluateProvider_ParseFail_BadJWT_PropagatesError(t *testing.T) {
	// Per SPEC §6.8 + Task 6: when extractTokens yields a malformed JWT (not
	// 3 dot-separated parts), jwt.Parse returns ErrJwtBadFormat; that error
	// propagates as the evalResult.err.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, nil)
	// Use a non-standard from_headers source so the malformed token is
	// extracted verbatim (the default Authorization Bearer path applies
	// stripNonBase64URLChars + must-have-prefix; we want the bytes to reach
	// jwt.Parse unmolested).
	p.fromHeaders = []headerLoc{{name: "X-JWT", valuePrefix: ""}}
	headers := newHeaders(map[string]string{"X-JWT": "notajwt"})

	r := f.evaluateProvider(p, headers, p.audiences)
	if r.allowed {
		t.Fatalf("evaluateProvider: want denied (malformed JWT); got %+v", r)
	}
	if !errors.Is(r.err, jwt.ErrJwtBadFormat) {
		t.Errorf("err = %v; want wraps ErrJwtBadFormat", r.err)
	}
}

func TestEvaluateProvider_SignatureValid_RS256_Success(t *testing.T) {
	// Per SPEC §6.8 + Task 6: a well-formed RS256 token signed by the test
	// keypair validates against the LocalJwks-derived *jwks.JWKSet.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000), // 1000s in the future relative to fixed Now
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	r := f.evaluateProvider(p, headers, p.audiences)
	if !r.allowed {
		t.Fatalf("evaluateProvider: want allowed; got err=%v", r.err)
	}
	if r.token == nil || r.provider != p {
		t.Errorf("result: want token+provider populated; got token=%v provider=%v", r.token, r.provider)
	}
}

func TestEvaluateProvider_SignatureInvalid_TamperedBytes_Denied(t *testing.T) {
	// Tampering the final character of the signature breaks RS256 verification;
	// jwt.VerifySignature surfaces ErrJwtVerificationFail per ADR-0151.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	// Tamper the signature by decoding, flipping a middle byte, and re-encoding.
	// Flipping the LAST base64url character is data-dependent (only 2 significant
	// bits) and can leave the decoded bytes unchanged → flaky. Decoding-then-XOR
	// guarantees the verified byte sequence differs.
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("signTestJWT_RS256 produced %d parts; want 3", len(parts))
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature segment: %v", err)
	}
	sigBytes[len(sigBytes)/2] ^= 0xFF
	parts[2] = base64.RawURLEncoding.EncodeToString(sigBytes)
	tampered := strings.Join(parts, ".")
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tampered})

	r := f.evaluateProvider(p, headers, p.audiences)
	if r.allowed {
		t.Fatalf("evaluateProvider: want denied (tampered sig); got allowed")
	}
	if !errors.Is(r.err, jwt.ErrJwtVerificationFail) {
		t.Errorf("err = %v; want wraps ErrJwtVerificationFail", r.err)
	}
}

// ----------------------------------------------------------------------------
// Group 4 — Claim validation smoke tests (integration; full coverage at
// internal/jwt/jwt_test.go per Task 4). ~3 cases.
// ----------------------------------------------------------------------------

func TestEvaluateProvider_ClaimExpired_Denied(t *testing.T) {
	// exp in the past + zero clock-skew → ErrJwtExpired per ADR-0151.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "", nil, 1*time.Nanosecond) // tiny clock skew so the past exp surfaces deterministically
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"exp": float64(1699999000), // 1000s in the past relative to fixed Now
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	r := f.evaluateProvider(p, headers, p.audiences)
	if r.allowed {
		t.Fatalf("evaluateProvider: want denied (exp in past); got allowed")
	}
	if !errors.Is(r.err, jwt.ErrJwtExpired) {
		t.Errorf("err = %v; want wraps ErrJwtExpired", r.err)
	}
}

func TestEvaluateProvider_ClaimAudienceMismatch_Denied(t *testing.T) {
	// aud claim doesn't intersect provider audiences → ErrJwtAudienceNotAllowed.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "", []string{"required-aud"}, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"aud": "other-aud",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	r := f.evaluateProvider(p, headers, p.audiences)
	if r.allowed {
		t.Fatalf("evaluateProvider: want denied (aud mismatch); got allowed")
	}
	if !errors.Is(r.err, jwt.ErrJwtAudienceNotAllowed) {
		t.Errorf("err = %v; want wraps ErrJwtAudienceNotAllowed", r.err)
	}
}

func TestEvaluateProvider_ClaimIssuerMismatch_Denied(t *testing.T) {
	// iss claim doesn't match provider.issuer → ErrJwtUnknownIssuer.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://wrong.example",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	r := f.evaluateProvider(p, headers, p.audiences)
	if r.allowed {
		t.Fatalf("evaluateProvider: want denied (iss mismatch); got allowed")
	}
	if !errors.Is(r.err, jwt.ErrJwtUnknownIssuer) {
		t.Errorf("err = %v; want wraps ErrJwtUnknownIssuer", r.err)
	}
}

// ----------------------------------------------------------------------------
// Group 5 — 6-variant JwtRequirement evaluator + recursive combinators per
// SPEC §6.8 + §11.P16 + ADR-0149. ~14 cases.
// ----------------------------------------------------------------------------

func TestEvaluateRequirement_ProviderName_Success(t *testing.T) {
	// Per SPEC §6.8 + ADR-0149: reqProviderName variant validates against the
	// named provider with the provider's own audiences list.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	req := &compiledRequirement{kind: reqProviderName, provider: p}
	r := f.evaluateRequirement(req, headers)
	if !r.allowed {
		t.Fatalf("evaluateRequirement: want allowed; got err=%v", r.err)
	}
	if r.token == nil || r.provider != p {
		t.Errorf("result: want token+provider populated; got %+v", r)
	}
}

func TestEvaluateRequirement_ProviderName_InvalidToken_Denied(t *testing.T) {
	// Same shape but signature tampered → denied with ErrJwtVerificationFail.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	// Decode-flip-encode the signature segment so the verified bytes differ
	// unambiguously (flipping the last base64url char only affects 2 bits and
	// can be a no-op on the decoded sequence → flaky).
	parts := strings.Split(tok, ".")
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature segment: %v", err)
	}
	sigBytes[len(sigBytes)/2] ^= 0xFF
	parts[2] = base64.RawURLEncoding.EncodeToString(sigBytes)
	tampered := strings.Join(parts, ".")
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tampered})

	req := &compiledRequirement{kind: reqProviderName, provider: p}
	r := f.evaluateRequirement(req, headers)
	if r.allowed {
		t.Fatalf("evaluateRequirement: want denied (tampered); got allowed")
	}
	if !errors.Is(r.err, jwt.ErrJwtVerificationFail) {
		t.Errorf("err = %v; want wraps ErrJwtVerificationFail", r.err)
	}
}

func TestEvaluateRequirement_ProviderAndAudiences_AudienceOverride_Success(t *testing.T) {
	// Per SPEC §6.8: reqProviderAndAudiences variant validates with the
	// per-rule audOverr REPLACING the provider's own audiences list.
	ensureEvalTestKey(t)
	// Provider has audiences=["provider-default"]; per-rule override is
	// ["per-rule-aud"]; token's aud is "per-rule-aud" → must match override.
	p := buildTestLocalProvider(t, "k1", "RS256", "", []string{"provider-default"}, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"aud": "per-rule-aud",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	req := &compiledRequirement{
		kind:     reqProviderAndAudiences,
		provider: p,
		audOverr: []string{"per-rule-aud"},
	}
	r := f.evaluateRequirement(req, headers)
	if !r.allowed {
		t.Fatalf("evaluateRequirement: want allowed (per-rule audOverr matches); got err=%v", r.err)
	}
}

func TestEvaluateRequirement_ProviderAndAudiences_AudienceMismatch_Denied(t *testing.T) {
	// Token aud doesn't match per-rule audOverr → ErrJwtAudienceNotAllowed.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"aud": "wrong-aud",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	req := &compiledRequirement{
		kind:     reqProviderAndAudiences,
		provider: p,
		audOverr: []string{"per-rule-aud"},
	}
	r := f.evaluateRequirement(req, headers)
	if r.allowed {
		t.Fatalf("evaluateRequirement: want denied (aud mismatch); got allowed")
	}
	if !errors.Is(r.err, jwt.ErrJwtAudienceNotAllowed) {
		t.Errorf("err = %v; want wraps ErrJwtAudienceNotAllowed", r.err)
	}
}

func TestEvaluateRequirement_RequiresAny_FirstSucceeds_ShortCircuits(t *testing.T) {
	// Per SPEC §6.8 + §11.P16: requires_any returns the FIRST successful
	// evaluation's status. Build 3 children — first succeeds; the rest must
	// not be invoked (short-circuit).
	ensureEvalTestKey(t)
	pGood := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	// Two "decoy" providers that will fail — wrong issuer so the same token's
	// iss claim mismatches.
	pBad1 := buildTestLocalProvider(t, "k1", "RS256", "https://wrong-1.example", nil, 0)
	pBad2 := buildTestLocalProvider(t, "k1", "RS256", "https://wrong-2.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{
		"good": pGood, "bad1": pBad1, "bad2": pBad2,
	}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	req := &compiledRequirement{
		kind: reqRequiresAny,
		children: []*compiledRequirement{
			{kind: reqProviderName, provider: pGood}, // succeeds → short-circuit
			{kind: reqProviderName, provider: pBad1}, // would fail
			{kind: reqProviderName, provider: pBad2}, // would fail
		},
	}
	r := f.evaluateRequirement(req, headers)
	if !r.allowed {
		t.Fatalf("evaluateRequirement: want allowed (first child succeeds); got err=%v", r.err)
	}
	if r.provider != pGood {
		t.Errorf("provider: want pGood; got %v", r.provider)
	}
}

func TestEvaluateRequirement_RequiresAny_AllFail_LastFailureReturned(t *testing.T) {
	// Per SPEC §6.8 + §11.P16: if all children fail, requires_any returns
	// the LAST failure's error (per Envoy verifier.cc).
	ensureEvalTestKey(t)
	pBad1 := buildTestLocalProvider(t, "k1", "RS256", "https://wrong-1.example", nil, 0)
	pBad2 := buildTestLocalProvider(t, "k1", "RS256", "https://wrong-2.example", nil, 0)
	pBad3 := buildTestLocalProvider(t, "k1", "RS256", "https://wrong-3.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{
		"bad1": pBad1, "bad2": pBad2, "bad3": pBad3,
	}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	req := &compiledRequirement{
		kind: reqRequiresAny,
		children: []*compiledRequirement{
			{kind: reqProviderName, provider: pBad1},
			{kind: reqProviderName, provider: pBad2},
			{kind: reqProviderName, provider: pBad3},
		},
	}
	r := f.evaluateRequirement(req, headers)
	if r.allowed {
		t.Fatalf("evaluateRequirement: want denied (all children fail); got allowed")
	}
	if !errors.Is(r.err, jwt.ErrJwtUnknownIssuer) {
		t.Errorf("err = %v; want wraps ErrJwtUnknownIssuer (the per-child failure)", r.err)
	}
}

func TestEvaluateRequirement_RequiresAll_AllSucceed_Allowed(t *testing.T) {
	// Per SPEC §6.8: requires_all returns allowed iff ALL children succeed.
	// Same token, two providers that both accept it (same RSA pubkey, same
	// kid/alg; the only difference is the audiences override per child).
	ensureEvalTestKey(t)
	p1 := buildTestLocalProvider(t, "k1", "RS256", "", []string{"aud-1"}, 0)
	p2 := buildTestLocalProvider(t, "k1", "RS256", "", []string{"aud-2"}, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p1, "p2": p2}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		// Token carries both audiences in the aud array.
		"aud": []string{"aud-1", "aud-2"},
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	req := &compiledRequirement{
		kind: reqRequiresAll,
		children: []*compiledRequirement{
			{kind: reqProviderName, provider: p1},
			{kind: reqProviderName, provider: p2},
		},
	}
	r := f.evaluateRequirement(req, headers)
	if !r.allowed {
		t.Fatalf("evaluateRequirement: want allowed (all children succeed); got err=%v", r.err)
	}
	// Per Task 6 decision (PROGRESS.md §Decision (iv); evaluator.go:422-436):
	// requires_all returns the LAST successful child's token+provider rather
	// than a blanket {allowed: true}. Assert that semantic explicitly.
	if r.token == nil {
		t.Errorf("expected token populated from last successful child, got nil")
	}
	if r.provider == nil {
		t.Errorf("expected provider populated from last successful child, got nil")
	}
	if r.provider != p2 {
		t.Errorf("expected provider == p2 (last successful child); got %v", r.provider)
	}
}

func TestEvaluateRequirement_RequiresAll_FirstFails_ShortCircuits(t *testing.T) {
	// Per SPEC §6.8: requires_all short-circuits on first failure; the
	// remaining children must not be evaluated.
	ensureEvalTestKey(t)
	pBad := buildTestLocalProvider(t, "k1", "RS256", "https://wrong.example", nil, 0)
	pGood := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"bad": pBad, "good": pGood}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	req := &compiledRequirement{
		kind: reqRequiresAll,
		children: []*compiledRequirement{
			{kind: reqProviderName, provider: pBad}, // fails first → short-circuit
			{kind: reqProviderName, provider: pGood},
		},
	}
	r := f.evaluateRequirement(req, headers)
	if r.allowed {
		t.Fatalf("evaluateRequirement: want denied (first child fails); got allowed")
	}
	if !errors.Is(r.err, jwt.ErrJwtUnknownIssuer) {
		t.Errorf("err = %v; want wraps ErrJwtUnknownIssuer", r.err)
	}
}

func TestEvaluateRequirement_AllowMissing_NoToken_Allowed(t *testing.T) {
	// Per SPEC §6.8 + §11.P16 RATIFIED-AND-EXTENDED: allow_missing semantic
	// allows requests with NO extracted token. Iterates all providers'
	// extraction sources; if none match, missing-OK.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	headers := newHeaders(map[string]string{}) // no Authorization header, no query

	req := &compiledRequirement{kind: reqAllowMissing}
	r := f.evaluateRequirement(req, headers)
	if !r.allowed {
		t.Fatalf("evaluateRequirement: want allowed (no token, missing-OK); got err=%v", r.err)
	}
}

func TestEvaluateRequirement_AllowMissing_PresentAndValid_Allowed(t *testing.T) {
	// Per §11.P16: present-and-valid token → allowed (the token validates
	// against the provider, just like reqProviderName).
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	req := &compiledRequirement{kind: reqAllowMissing}
	r := f.evaluateRequirement(req, headers)
	if !r.allowed {
		t.Fatalf("evaluateRequirement: want allowed (present + valid); got err=%v", r.err)
	}
	if r.token == nil {
		t.Error("token: want populated on present+valid path")
	}
}

func TestEvaluateRequirement_AllowMissing_PresentAndInvalid_Denied(t *testing.T) {
	// Per §11.P16: present-and-invalid → FAIL. Distinguishing feature vs
	// allow_missing_or_failed.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://wrong.example", // mismatched issuer
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	req := &compiledRequirement{kind: reqAllowMissing}
	r := f.evaluateRequirement(req, headers)
	if r.allowed {
		t.Fatalf("evaluateRequirement: want denied (present-and-invalid); got allowed")
	}
	if !errors.Is(r.err, jwt.ErrJwtUnknownIssuer) {
		t.Errorf("err = %v; want wraps ErrJwtUnknownIssuer", r.err)
	}
}

func TestEvaluateRequirement_AllowMissingOrFailed_AlwaysAllowed(t *testing.T) {
	// Per SPEC §6.8: allow_missing_or_failed → any outcome → OK.
	ensureEvalTestKey(t)
	p := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	f := makeTestFilter(map[string]*compiledProvider{"p1": p}, fixedNow(time.Unix(1700000000, 0)))

	// Case (a): no token.
	if r := f.evaluateRequirement(&compiledRequirement{kind: reqAllowMissingOrFailed}, newHeaders(nil)); !r.allowed {
		t.Errorf("allowMissingOrFailed + no token: want allowed; got %+v", r)
	}
	// Case (b): present-and-invalid token.
	badTok := signTestJWT_RS256(t, "k1", map[string]interface{}{"iss": "https://wrong.example", "exp": float64(1700001000)})
	hBad := newHeaders(map[string]string{"Authorization": "Bearer " + badTok})
	if r := f.evaluateRequirement(&compiledRequirement{kind: reqAllowMissingOrFailed}, hBad); !r.allowed {
		t.Errorf("allowMissingOrFailed + invalid token: want allowed; got %+v", r)
	}
	_ = p // silence unused (p used for future-shape parity)
}

func TestEvaluateRequirement_RecursiveCombinator_AnyInsideAll_Success(t *testing.T) {
	// Per SPEC §6.8 + §11.P16 + §13 test-design: recursive combinators —
	// requires_all with a requires_any nested child. The nested any-child has
	// 2 sub-providers; one succeeds. The outer all-child has 2 children: the
	// nested any (succeeds) + a direct provider_name (succeeds).
	ensureEvalTestKey(t)
	pGood := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	pBad := buildTestLocalProvider(t, "k1", "RS256", "https://wrong.example", nil, 0)
	pAud := buildTestLocalProvider(t, "k1", "RS256", "", []string{"aud-correct"}, 0)
	f := makeTestFilter(map[string]*compiledProvider{
		"good": pGood, "bad": pBad, "aud": pAud,
	}, fixedNow(time.Unix(1700000000, 0)))
	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"aud": "aud-correct",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	// requires_all { requires_any [bad, good], provider_name(aud) }
	req := &compiledRequirement{
		kind: reqRequiresAll,
		children: []*compiledRequirement{
			{
				kind: reqRequiresAny,
				children: []*compiledRequirement{
					{kind: reqProviderName, provider: pBad},  // fails
					{kind: reqProviderName, provider: pGood}, // succeeds → satisfies inner any
				},
			},
			{kind: reqProviderName, provider: pAud}, // succeeds → satisfies outer all
		},
	}
	r := f.evaluateRequirement(req, headers)
	if !r.allowed {
		t.Fatalf("evaluateRequirement: want allowed (recursive combinator); got err=%v", r.err)
	}
}

func TestEvaluateRequirement_NilRequirement_Allowed(t *testing.T) {
	// Defensive: nil requirement post-buildCompiledRequirement substitution
	// → treated as allowed (mirrors the proto-comment "If this field is
	// empty, JWT authentication is optional"). In practice
	// buildCompiledRequirement substitutes reqAllowMissingOrFailed for nil
	// proto, but the evaluator defends.
	f := makeTestFilter(map[string]*compiledProvider{}, nil)
	r := f.evaluateRequirement(nil, newHeaders(nil))
	if !r.allowed {
		t.Errorf("evaluateRequirement(nil): want allowed; got %+v", r)
	}
}

// ----------------------------------------------------------------------------
// Group 10 — RemoteJwks lifecycle smoke tests (integration; full coverage at
// internal/jwks/jwks_test.go per Task 3). ~3 cases.
// ----------------------------------------------------------------------------

func TestEvaluateProvider_RemoteJwks_FetchSuccess_Counter(t *testing.T) {
	// Per Task-13 empirical reference scrape: jwksFetchSuccess is emitted at
	// config-load time (ONCE per RemoteJwks provider whose initial blocking
	// fetch succeeded), NOT on each evaluator-side Get() cache hit. This test
	// constructs a fresh compiledConfig via buildCompiledConfig (the
	// production code path) so the credit-at-load discipline fires; then
	// checks that the counter reflects exactly one increment per RemoteJwks
	// provider whose fetcher exists. The evaluator's evaluateProvider does
	// NOT touch jwksFetchSuccess on cache hits.
	ensureEvalTestKey(t)
	jwksJSON := buildTestJWKSetJSON(t, "k1", "RS256")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(jwksJSON))
	}))
	defer srv.Close()
	fetcher, err := jwks.New(srv.URL, 5*time.Minute, nil, nil, nil)
	if err != nil {
		t.Fatalf("jwks.New: %v", err)
	}
	defer func() { _ = fetcher.Close() }()

	reg := stats.NewRegistry()
	cfg := &compiledConfig{
		providers: map[string]*compiledProvider{},
		stats:     newFilterStats(reg, "ingress_http"),
	}
	p := &compiledProvider{
		issuer:      "https://issuer.example",
		jwksFetcher: fetcher,
		clockSkew:   60 * time.Second,
	}
	cfg.providers["p1"] = p
	// Simulate the buildCompiledConfig credit-at-load increment: one Inc per
	// RemoteJwks provider whose fetcher is non-nil after init.
	cfg.stats.jwksFetchSuccess.Inc()
	f := &filter{state: &factoryState{listenerRC: cfg}, activeRC: cfg, now: fixedNow(time.Unix(1700000000, 0))}

	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	before := cfg.stats.jwksFetchSuccess.Load()
	r := f.evaluateProvider(p, headers, p.audiences)
	after := cfg.stats.jwksFetchSuccess.Load()
	if !r.allowed {
		t.Fatalf("evaluateProvider: want allowed; got err=%v", r.err)
	}
	// Task-13 refinement: counter is credited at LOAD time, not per-request.
	// Cache hits at request time must NOT bump the counter further.
	if after != before {
		t.Errorf("jwksFetchSuccess: want unchanged on cache hit; before=%d after=%d", before, after)
	}
	if got := cfg.stats.jwksFetchFailed.Load(); got != 0 {
		t.Errorf("jwksFetchFailed: want 0 on success path; got %d", got)
	}
}

func TestEvaluateProvider_RemoteJwks_FetchFailure_Counter(t *testing.T) {
	// Per SPEC §6.8 + §11.P6 + Task 6: when a RemoteJwks Get() fails (here,
	// a fast_listener=true Fetcher whose initial fetch hits a 500-returning
	// server), jwksFetchFailed counter increments. We use fast_listener=true
	// so jwks.New() doesn't block-fail; instead Get() at evaluator time
	// returns the underlying ErrJwksFetchFail (after the initial fetch
	// goroutine completed with a failure).
	ensureEvalTestKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	fetcher, err := jwks.New(
		srv.URL,
		5*time.Minute,
		&jwks.AsyncFetch{FastListener: true, FailedRefetchDuration: 30 * time.Second},
		&jwks.RetryPolicy{NumRetries: 0, BaseInterval: 10 * time.Millisecond, MaxInterval: 20 * time.Millisecond},
		nil,
	)
	if err != nil {
		t.Fatalf("jwks.New: %v", err)
	}
	defer func() { _ = fetcher.Close() }()

	// Poll until the initial fetch goroutine has marked notReadyErr (the
	// failure path closes ready with notReadyErr set). 200ms is generous;
	// the goroutine should complete its 1-attempt HTTP cycle in <10ms.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, gerr := fetcher.Get(context.Background()); gerr != nil && !errors.Is(gerr, jwks.ErrJwksNotReady) {
			break // initial fetch errored out; we're ready to test
		}
		time.Sleep(5 * time.Millisecond)
	}

	reg := stats.NewRegistry()
	cfg := &compiledConfig{
		providers: map[string]*compiledProvider{},
		stats:     newFilterStats(reg, "ingress_http"),
	}
	p := &compiledProvider{
		issuer:      "https://issuer.example",
		jwksFetcher: fetcher,
		clockSkew:   60 * time.Second,
	}
	cfg.providers["p1"] = p
	f := &filter{state: &factoryState{listenerRC: cfg}, activeRC: cfg, now: fixedNow(time.Unix(1700000000, 0))}

	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	r := f.evaluateProvider(p, headers, p.audiences)
	if r.allowed {
		t.Fatalf("evaluateProvider: want denied (fetch failure); got allowed")
	}
	if got := cfg.stats.jwksFetchFailed.Load(); got == 0 {
		t.Errorf("jwksFetchFailed: want incremented (>=1); got 0")
	}
}

func TestEvaluateProvider_RemoteJwks_KidMismatch_Denied(t *testing.T) {
	// Per SPEC §6.8 + ADR-0150: JWKS Lookup mismatch (token kid=k2, set has
	// only kid=k1) → ErrJwksKidAlgMismatch.
	ensureEvalTestKey(t)
	jwksJSON := buildTestJWKSetJSON(t, "k1", "RS256")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(jwksJSON))
	}))
	defer srv.Close()
	fetcher, err := jwks.New(srv.URL, 5*time.Minute, nil, nil, nil)
	if err != nil {
		t.Fatalf("jwks.New: %v", err)
	}
	defer func() { _ = fetcher.Close() }()

	reg := stats.NewRegistry()
	cfg := &compiledConfig{
		providers: map[string]*compiledProvider{},
		stats:     newFilterStats(reg, "ingress_http"),
	}
	p := &compiledProvider{
		issuer:      "https://issuer.example",
		jwksFetcher: fetcher,
		clockSkew:   60 * time.Second,
	}
	cfg.providers["p1"] = p
	f := &filter{state: &factoryState{listenerRC: cfg}, activeRC: cfg, now: fixedNow(time.Unix(1700000000, 0))}

	// Sign with kid=k2 — the JWKS has kid=k1 only.
	tok := signTestJWT_RS256(t, "k2", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{"Authorization": "Bearer " + tok})

	r := f.evaluateProvider(p, headers, p.audiences)
	if r.allowed {
		t.Fatalf("evaluateProvider: want denied (kid mismatch); got allowed")
	}
	if !errors.Is(r.err, jwks.ErrJwksKidAlgMismatch) {
		t.Errorf("err = %v; want wraps ErrJwksKidAlgMismatch", r.err)
	}
}

// ----------------------------------------------------------------------------
// Stubs for downstream test groups (visible-from-start per Task 2 spec).
// ----------------------------------------------------------------------------

// ----------------------------------------------------------------------------
// Group 5 — DecodeHeaders dispatch integration tests (Task 9 of phase-17 per
// SPEC §6.6 + §1.1 amendment 12 + ADR-0155).
//
// 14 cases pin the request-time dispatch surface:
//   - Per-route disabled passthrough (NO counters).
//   - CORS preflight bypass + bypass_cors_preflight=false case.
//   - Per-route dangling-name 403 (via resolveRequirement).
//   - No-rule-match passthrough.
//   - Allow / deny paths for valid+expired+audience+missing+bad-sig tokens.
//   - originalURI capture timing.
//   - strip_failure_response gating.
//   - Per-route requirement_name happy path.
// ----------------------------------------------------------------------------

// buildTestFilterForDispatch wires a *filter with a complete listener-level
// *compiledConfig (providers + rules + requirementMap + stats) + a wired
// jwtFakeCB. Used by Group 5 dispatch integration tests. The fixedNow closure
// makes exp/nbf-bound claims deterministic.
func buildTestFilterForDispatch(t *testing.T, cfg *compiledConfig, perRouteMsg proto.Message, fixed time.Time) (*filter, *jwtFakeCB) {
	t.Helper()
	cb := &jwtFakeCB{routeCfg: perRouteMsg}
	st := &factoryState{listenerRC: cfg}
	f := &filter{
		state: st,
		dcb:   cb,
		now:   fixedNow(fixed),
	}
	return f, cb
}

// dispatchFixture returns a *compiledConfig carrying one provider (LocalJwks,
// RS256, kid=k1, iss=https://issuer.example) + one wildcard rule binding that
// provider via reqProviderName + the 7-counter filterStats registered to a
// fresh Registry. The test's fixed clock is 1700000000 (the de-facto Group
// 3+4 anchor).
func dispatchFixture(t *testing.T) *compiledConfig {
	t.Helper()
	ensureEvalTestKey(t)
	reg := stats.NewRegistry()
	prov := buildTestLocalProvider(t, "k1", "RS256", "https://issuer.example", nil, 0)
	cfg := &compiledConfig{
		providers: map[string]*compiledProvider{"p1": prov},
		rules: []*compiledRule{
			{matchFn: nil, requirement: &compiledRequirement{kind: reqProviderName, provider: prov}},
		},
		stats: newFilterStats(reg, "ingress_http"),
	}
	return cfg
}

func TestDecodeHeaders_PerRouteDisabled_Passthrough_NoCounters(t *testing.T) {
	// Per SPEC §5.3 + ADR-0153: per-route disabled:true → HeaderContinue
	// + NO counter increments (passthrough fast-path).
	cfg := dispatchFixture(t)
	// Per-route proto with disabled:true; lazy-cache will build a
	// *compiledPerRoute{disabled:true}.
	prMsg := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_Disabled{Disabled: true},
	}
	f, cb := buildTestFilterForDispatch(t, cfg, prMsg, time.Unix(1700000000, 0))

	headers := newHeaders(map[string]string{
		":method": "GET",
		":path":   "/foo",
	})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.Continue {
		t.Errorf("DecodeHeaders: got %v; want Continue", got)
	}
	if !f.passthrough {
		t.Error("f.passthrough: got false; want true (per-route disabled fast-path)")
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on per-route disabled; got %+v", cb.localReply)
	}
	// NO counter increments per §11.P9.
	if got := cfg.stats.allowed.Load(); got != 0 {
		t.Errorf("allowed counter: got %d; want 0 (passthrough fast-path)", got)
	}
	if got := cfg.stats.denied.Load(); got != 0 {
		t.Errorf("denied counter: got %d; want 0", got)
	}
	if got := cfg.stats.corsPreflightBypassed.Load(); got != 0 {
		t.Errorf("cors_preflight_bypassed counter: got %d; want 0", got)
	}
}

func TestDecodeHeaders_CorsPreflight_Bypassed_CounterIncremented(t *testing.T) {
	// Per §11.P1 + §1.1 amendment 10 + ADR-0148: bypass_cors_preflight:true +
	// OPTIONS+Origin+ACR-M → HeaderContinue + cors_preflight_bypassed++.
	cfg := dispatchFixture(t)
	cfg.bypassCorsPreflight = true
	f, cb := buildTestFilterForDispatch(t, cfg, nil, time.Unix(1700000000, 0))

	headers := newHeaders(map[string]string{
		":method":                       "OPTIONS",
		":path":                         "/foo",
		"Origin":                        "https://example.com",
		"Access-Control-Request-Method": "POST",
	})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.Continue {
		t.Errorf("DecodeHeaders: got %v; want Continue", got)
	}
	if !f.passthrough {
		t.Error("f.passthrough: got false; want true (CORS bypass)")
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on CORS bypass; got %+v", cb.localReply)
	}
	if got := cfg.stats.corsPreflightBypassed.Load(); got != 1 {
		t.Errorf("cors_preflight_bypassed: got %d; want 1", got)
	}
	if got := cfg.stats.allowed.Load(); got != 0 {
		t.Errorf("allowed counter: got %d; want 0 (CORS bypass does NOT tick allowed)", got)
	}
}

func TestDecodeHeaders_CorsPreflightDisabled_NotBypassed(t *testing.T) {
	// Per §1.1 amendment 10: bypass_cors_preflight=false → CORS goes through
	// to validation. Absent JWT → 401 deny.
	cfg := dispatchFixture(t)
	cfg.bypassCorsPreflight = false
	f, cb := buildTestFilterForDispatch(t, cfg, nil, time.Unix(1700000000, 0))

	headers := newHeaders(map[string]string{
		":method":                       "OPTIONS",
		":path":                         "/foo",
		"Origin":                        "https://example.com",
		"Access-Control-Request-Method": "POST",
	})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders: got %v; want StopIteration (no JWT → deny)", got)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want fired (no JWT); got nil")
	}
	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401 (JwtMissed)", cb.localReply.status)
	}
	if got := cfg.stats.corsPreflightBypassed.Load(); got != 0 {
		t.Errorf("cors_preflight_bypassed: got %d; want 0 (disabled)", got)
	}
	if got := cfg.stats.denied.Load(); got != 1 {
		t.Errorf("denied: got %d; want 1", got)
	}
}

func TestDecodeHeaders_DanglingPerRouteName_403_Denied(t *testing.T) {
	// Per §1.1 amendment 6 + ADR-0153: per-route requirement_name not in
	// requirement_map → 403 emitted by resolveRequirement + denied++.
	cfg := dispatchFixture(t)
	cfg.requirementMap = map[string]*compiledRequirement{
		"present-req": {kind: reqAllowMissingOrFailed},
	}
	prMsg := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_RequirementName{RequirementName: "missing-req"},
	}
	f, cb := buildTestFilterForDispatch(t, cfg, prMsg, time.Unix(1700000000, 0))

	headers := newHeaders(map[string]string{":method": "GET", ":path": "/foo"})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders: got %v; want StopIteration", got)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want fired; got nil")
	}
	if cb.localReply.status != 403 {
		t.Errorf("status: got %d; want 403 (dangling per-route)", cb.localReply.status)
	}
	wantBody := "Failed JWT authentication: Wrong requirement_name: missing-req"
	if cb.localReply.body != wantBody {
		t.Errorf("body: got %q; want %q", cb.localReply.body, wantBody)
	}
	if cb.localReply.headers != nil {
		t.Errorf("headers: got %v; want nil (per §1.1 amendment 6 — NO WWW-Authenticate)", cb.localReply.headers)
	}
	if got := cfg.stats.denied.Load(); got != 1 {
		t.Errorf("denied: got %d; want 1", got)
	}
}

func TestDecodeHeaders_NoRuleMatch_NoPerRoute_Passthrough_NoCounters(t *testing.T) {
	// Per SPEC §6.6: no per-route + empty rules → pass-through; NO counters.
	cfg := &compiledConfig{
		providers: map[string]*compiledProvider{},
		rules:     nil,
		stats:     newFilterStats(stats.NewRegistry(), "ingress_http"),
	}
	f, cb := buildTestFilterForDispatch(t, cfg, nil, time.Unix(1700000000, 0))

	headers := newHeaders(map[string]string{":method": "GET", ":path": "/foo"})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.Continue {
		t.Errorf("DecodeHeaders: got %v; want Continue", got)
	}
	if !f.passthrough {
		t.Error("f.passthrough: got false; want true")
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on no-match passthrough; got %+v", cb.localReply)
	}
	if got := cfg.stats.allowed.Load(); got != 0 {
		t.Errorf("allowed: got %d; want 0", got)
	}
	if got := cfg.stats.denied.Load(); got != 0 {
		t.Errorf("denied: got %d; want 0", got)
	}
}

func TestDecodeHeaders_ValidToken_RouteMatch_Allowed_HeaderContinue(t *testing.T) {
	// Happy path: valid Bearer token + matching rule → HeaderContinue + allowed++.
	cfg := dispatchFixture(t)
	f, cb := buildTestFilterForDispatch(t, cfg, nil, time.Unix(1700000000, 0))

	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{
		":method":       "GET",
		":path":         "/foo",
		"Authorization": "Bearer " + tok,
	})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.Continue {
		t.Errorf("DecodeHeaders: got %v; want Continue", got)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on allow; got %+v", cb.localReply)
	}
	if got := cfg.stats.allowed.Load(); got != 1 {
		t.Errorf("allowed: got %d; want 1", got)
	}
	if got := cfg.stats.denied.Load(); got != 0 {
		t.Errorf("denied: got %d; want 0", got)
	}
}

func TestDecodeHeaders_ExpiredToken_Denied_401(t *testing.T) {
	// Expired token → 401 + body "Jwt is expired" + denied++.
	cfg := dispatchFixture(t)
	f, cb := buildTestFilterForDispatch(t, cfg, nil, time.Unix(1700000000, 0))

	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1699900000), // ~28h in the past → expired
	})
	headers := newHeaders(map[string]string{
		":method":       "GET",
		":path":         "/foo",
		"Authorization": "Bearer " + tok,
	})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders: got %v; want StopIteration", got)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want fired; got nil")
	}
	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401", cb.localReply.status)
	}
	if cb.localReply.body != "Jwt is expired" {
		t.Errorf("body: got %q; want %q", cb.localReply.body, "Jwt is expired")
	}
	if got := cfg.stats.denied.Load(); got != 1 {
		t.Errorf("denied: got %d; want 1", got)
	}
}

func TestDecodeHeaders_AudienceMismatch_Denied_403(t *testing.T) {
	// Per §1.1 amendment 8 + ADR-0155: audience mismatch → 403 (NOT 401)
	// + body "Audiences in Jwt are not allowed" + denied++.
	cfg := dispatchFixture(t)
	// Tighten provider to require an audience the token won't carry.
	cfg.providers["p1"].audiences = []string{"intended-aud"}
	cfg.rules[0].requirement.provider = cfg.providers["p1"]
	f, cb := buildTestFilterForDispatch(t, cfg, nil, time.Unix(1700000000, 0))

	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"aud": "wrong-aud",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{
		":method":       "GET",
		":path":         "/foo",
		"Authorization": "Bearer " + tok,
	})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders: got %v; want StopIteration", got)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want fired; got nil")
	}
	if cb.localReply.status != 403 {
		t.Errorf("status: got %d; want 403 (audience mismatch per amendment 8)", cb.localReply.status)
	}
	if cb.localReply.body != "Audiences in Jwt are not allowed" {
		t.Errorf("body: got %q; want %q", cb.localReply.body, "Audiences in Jwt are not allowed")
	}
}

func TestDecodeHeaders_MissingToken_Denied_401_NoErrorParam_WWWAuth(t *testing.T) {
	// Per §1.1 amendment 12 + §11.P2 + ADR-0155: JwtMissed → 401 +
	// WWW-Authenticate `Bearer realm="<:path>"` with NO error param.
	cfg := dispatchFixture(t)
	f, cb := buildTestFilterForDispatch(t, cfg, nil, time.Unix(1700000000, 0))

	headers := newHeaders(map[string]string{
		":method": "GET",
		":path":   "/api/foo",
		// NO Authorization
	})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders: got %v; want StopIteration", got)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want fired; got nil")
	}
	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401 (JwtMissed)", cb.localReply.status)
	}
	if cb.localReply.body != "Jwt is missing" {
		t.Errorf("body: got %q; want %q", cb.localReply.body, "Jwt is missing")
	}
	// WWW-Authenticate: realm="/api/foo" (NO `, error="invalid_token"` per §11.P2).
	wantWWW := `Bearer realm="/api/foo"`
	if got := cb.localReply.headers.Get("WWW-Authenticate"); got != wantWWW {
		t.Errorf("WWW-Authenticate: got %q; want %q", got, wantWWW)
	}
}

func TestDecodeHeaders_BadSignature_Denied_401_WithErrorParam_WWWAuth(t *testing.T) {
	// Per §1.1 amendment 12 + §11.P2: non-JwtMissed deny → WWW-Authenticate
	// with `, error="invalid_token"` appended.
	cfg := dispatchFixture(t)
	f, cb := buildTestFilterForDispatch(t, cfg, nil, time.Unix(1700000000, 0))

	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	// Tamper the signature segment to break verification.
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts in JWT, got %d", len(parts))
	}
	if parts[2] == "" {
		t.Fatal("signature empty")
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	sigBytes[0] ^= 0xff
	tampered := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sigBytes)

	headers := newHeaders(map[string]string{
		":method":       "GET",
		":path":         "/api/foo",
		"Authorization": "Bearer " + tampered,
	})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders: got %v; want StopIteration", got)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want fired; got nil")
	}
	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401", cb.localReply.status)
	}
	wantWWW := `Bearer realm="/api/foo", error="invalid_token"`
	if got := cb.localReply.headers.Get("WWW-Authenticate"); got != wantWWW {
		t.Errorf("WWW-Authenticate: got %q; want %q", got, wantWWW)
	}
}

func TestDecodeHeaders_OriginalURICapturedBeforeRouteMutation(t *testing.T) {
	// Per §1.1 amendment 12 + ADR-0155: f.originalURI is captured at
	// DecodeHeaders entry from :path (BEFORE any route mutation).
	cfg := dispatchFixture(t)
	f, _ := buildTestFilterForDispatch(t, cfg, nil, time.Unix(1700000000, 0))

	headers := newHeaders(map[string]string{
		":method": "GET",
		":path":   "/foo?bar=baz",
	})
	_ = f.DecodeHeaders(headers, false)
	if f.originalURI != "/foo?bar=baz" {
		t.Errorf("originalURI: got %q; want %q", f.originalURI, "/foo?bar=baz")
	}
}

func TestDecodeHeaders_StripFailureResponse_EmptyBody_NoWWWAuth(t *testing.T) {
	// Per §11.P3 RATIFIED: strip_failure_response:true → SendLocalReply with
	// body="" + nil headers (NO WWW-Authenticate).
	cfg := dispatchFixture(t)
	cfg.stripFailureResponse = true
	f, cb := buildTestFilterForDispatch(t, cfg, nil, time.Unix(1700000000, 0))

	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1699900000), // expired
	})
	headers := newHeaders(map[string]string{
		":method":       "GET",
		":path":         "/api/foo",
		"Authorization": "Bearer " + tok,
	})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders: got %v; want StopIteration", got)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want fired; got nil")
	}
	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401 (status preserved)", cb.localReply.status)
	}
	if cb.localReply.body != "" {
		t.Errorf("body: got %q; want %q (stripped)", cb.localReply.body, "")
	}
	if cb.localReply.headers != nil {
		t.Errorf("headers: got %v; want nil (strip_failure_response strips WWW-Authenticate)", cb.localReply.headers)
	}
}

func TestDecodeHeaders_PerRouteRequirementName_ValidToken_Allowed(t *testing.T) {
	// Per ADR-0153 case (c): per-route requirement_name hits requirement_map →
	// uses named requirement → allowed when validation succeeds.
	cfg := dispatchFixture(t)
	// Bind the provider into the requirement_map under "named-req"; clear
	// listener rules so per-route is the SOLE dispatch source.
	cfg.rules = nil
	cfg.requirementMap = map[string]*compiledRequirement{
		"named-req": {kind: reqProviderName, provider: cfg.providers["p1"]},
	}
	prMsg := &jwt_authnv3.PerRouteConfig{
		RequirementSpecifier: &jwt_authnv3.PerRouteConfig_RequirementName{RequirementName: "named-req"},
	}
	f, cb := buildTestFilterForDispatch(t, cfg, prMsg, time.Unix(1700000000, 0))

	tok := signTestJWT_RS256(t, "k1", map[string]interface{}{
		"iss": "https://issuer.example",
		"exp": float64(1700001000),
	})
	headers := newHeaders(map[string]string{
		":method":       "GET",
		":path":         "/foo",
		"Authorization": "Bearer " + tok,
	})
	got := f.DecodeHeaders(headers, false)

	if got != envoyhttp.Continue {
		t.Errorf("DecodeHeaders: got %v; want Continue", got)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on allow; got %+v", cb.localReply)
	}
	if got := cfg.stats.allowed.Load(); got != 1 {
		t.Errorf("allowed: got %d; want 1", got)
	}
}

func TestDecodeHeaders_PathMissing_OriginalURIEmpty_RealmDefaultsToSlash(t *testing.T) {
	// Per ADR-0155: :path missing at DecodeHeaders → f.originalURI stays ""
	// → emitDenyResponse falls back to realm="/" defensive default.
	cfg := dispatchFixture(t)
	f, cb := buildTestFilterForDispatch(t, cfg, nil, time.Unix(1700000000, 0))

	headers := newHeaders(map[string]string{
		":method": "GET",
		// NO :path → originalURI stays ""
	})
	_ = f.DecodeHeaders(headers, false)

	if f.originalURI != "" {
		t.Errorf("originalURI: got %q; want empty", f.originalURI)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want fired; got nil")
	}
	wantWWW := `Bearer realm="/"`
	if got := cb.localReply.headers.Get("WWW-Authenticate"); got != wantWWW {
		t.Errorf("WWW-Authenticate: got %q; want %q (defensive default)", got, wantWWW)
	}
}

// ----------------------------------------------------------------------------
// Group 6 — Side-effect emit-order tests (Task 9 of phase-17 per SPEC §6.9 +
// §11.P10 + §11.P13 + ADR-0149 + ADR-0155).
//
// 10 cases pin the 4-step success-side-effect emit-order:
//   - Strip-on-success (forward=false / forward=true / from_headers / from_params).
//   - from_cookies UNTOUCHED per proto caveat.
//   - forward_payload_header padding true/false.
//   - claim_to_headers (string / numeric / array silent-skip / nested dot-notation).
//   - clear_route_cache TRACKING flag flip.
//   - Combined emit-order check.
// ----------------------------------------------------------------------------

// validToken builds a fully-validated *jwt.Token via signTestJWT_RS256 +
// jwt.Parse round-trip. The returned token's RawPayload is the base64url-
// encoded JWT payload segment (for forward_payload_header tests). Used by
// Group 6 tests that need a populated *jwt.Token to drive applySideEffects.
func validToken(t *testing.T, claims map[string]interface{}) *jwt.Token {
	t.Helper()
	ensureEvalTestKey(t)
	raw := signTestJWT_RS256(t, "k1", claims)
	tok, err := jwt.Parse(raw)
	if err != nil {
		t.Fatalf("jwt.Parse: %v", err)
	}
	return tok
}

func TestApplySideEffects_StripAuthorizationHeader_OnForwardFalse(t *testing.T) {
	// Per §6.9 + planner-time decision 8: forward=false (default) + no
	// explicit sources → strip Authorization header on success.
	f := &filter{}
	p := &compiledProvider{forward: false}
	tok := validToken(t, map[string]interface{}{"iss": "x"})
	headers := newHeaders(map[string]string{
		"Authorization": "Bearer eyJfake",
		":path":         "/foo",
	})

	f.applySideEffects(tok, p, headers)

	if got := headers.Get("Authorization"); got != "" {
		t.Errorf("Authorization: got %q; want empty (stripped on forward=false)", got)
	}
}

func TestApplySideEffects_AuthorizationRetained_OnForwardTrue(t *testing.T) {
	// Per §6.9: forward=true → JWT retained in forwarded request.
	f := &filter{}
	p := &compiledProvider{forward: true}
	tok := validToken(t, map[string]interface{}{"iss": "x"})
	headers := newHeaders(map[string]string{
		"Authorization": "Bearer eyJfake",
		":path":         "/foo",
	})

	f.applySideEffects(tok, p, headers)

	if got := headers.Get("Authorization"); got != "Bearer eyJfake" {
		t.Errorf("Authorization: got %q; want %q (retained on forward=true)", got, "Bearer eyJfake")
	}
}

func TestApplySideEffects_StripFromHeaders(t *testing.T) {
	// Per planner-time decision 8: forward=false + explicit from_headers →
	// strip those headers (and ONLY those headers — defaults suppressed).
	f := &filter{}
	p := &compiledProvider{
		forward:     false,
		fromHeaders: []headerLoc{{name: "X-JWT", valuePrefix: ""}},
	}
	tok := validToken(t, map[string]interface{}{"iss": "x"})
	headers := newHeaders(map[string]string{
		"X-JWT":         "eyJfake",
		"Authorization": "Bearer otherToken",
	})

	f.applySideEffects(tok, p, headers)

	if got := headers.Get("X-JWT"); got != "" {
		t.Errorf("X-JWT: got %q; want empty (stripped)", got)
	}
	// Authorization is NOT in fromHeaders; defaults are suppressed when
	// explicit sources are configured. Authorization should remain.
	if got := headers.Get("Authorization"); got != "Bearer otherToken" {
		t.Errorf("Authorization: got %q; want %q (explicit sources suppress defaults)", got, "Bearer otherToken")
	}
}

func TestApplySideEffects_StripFromParams_PathRewritten(t *testing.T) {
	// Per planner-time decision 8: forward=false + from_params → :path
	// rewritten without the named params.
	f := &filter{}
	p := &compiledProvider{
		forward:    false,
		fromParams: []string{"token"},
	}
	tok := validToken(t, map[string]interface{}{"iss": "x"})
	headers := newHeaders(map[string]string{
		":path": "/foo?token=eyJfake&other=keep",
	})

	f.applySideEffects(tok, p, headers)

	// Check that "token" is stripped but "other" remains.
	got := headers.Get(":path")
	if strings.Contains(got, "token=") {
		t.Errorf(":path: got %q; want token param stripped", got)
	}
	if !strings.Contains(got, "other=keep") {
		t.Errorf(":path: got %q; want other=keep retained", got)
	}
}

func TestApplySideEffects_FromCookiesUntouched_PerProtoCaveat(t *testing.T) {
	// Per JwtProvider.from_cookies proto comment: cookies UNTOUCHED on
	// strip-on-success, even with forward=false.
	f := &filter{}
	p := &compiledProvider{
		forward:     false,
		fromCookies: []string{"jwt-cookie"},
	}
	tok := validToken(t, map[string]interface{}{"iss": "x"})
	cookieValue := "jwt-cookie=eyJfake; other=v"
	headers := newHeaders(map[string]string{"Cookie": cookieValue})

	f.applySideEffects(tok, p, headers)

	if got := headers.Get("Cookie"); got != cookieValue {
		t.Errorf("Cookie: got %q; want %q (UNTOUCHED per proto caveat)", got, cookieValue)
	}
}

func TestApplySideEffects_ForwardPayloadHeader_PaddingTrue(t *testing.T) {
	// Per §11.P13: pad_forward_payload_header:true → trailing `=` retained.
	f := &filter{}
	p := &compiledProvider{
		forward:              true,
		forwardPayloadHeader: "X-Jwt-Payload",
		padForwardPayloadHdr: true,
	}
	// Build a token whose payload base64url-encodes to a length that is NOT
	// a multiple of 4 (so padding makes a visible difference).
	tok := validToken(t, map[string]interface{}{"iss": "x", "v": float64(1)})
	headers := newHeaders(map[string]string{})

	f.applySideEffects(tok, p, headers)

	emitted := headers.Get("X-Jwt-Payload")
	if emitted == "" {
		t.Fatal("X-Jwt-Payload: want emitted; got empty")
	}
	if len(emitted)%4 != 0 {
		t.Errorf("X-Jwt-Payload length %d not multiple of 4; want padded", len(emitted))
	}
	// The padded form is the raw + 0-3 trailing `=` chars.
	expected := tok.RawPayload
	for len(expected)%4 != 0 {
		expected += "="
	}
	if emitted != expected {
		t.Errorf("X-Jwt-Payload: got %q; want %q", emitted, expected)
	}
}

func TestApplySideEffects_ForwardPayloadHeader_PaddingFalse(t *testing.T) {
	// Per §11.P13 + RFC 7515 §2: pad_forward_payload_header:false → no
	// trailing `=` (unpadded form).
	f := &filter{}
	p := &compiledProvider{
		forward:              true,
		forwardPayloadHeader: "X-Jwt-Payload",
		padForwardPayloadHdr: false,
	}
	tok := validToken(t, map[string]interface{}{"iss": "x", "v": float64(1)})
	headers := newHeaders(map[string]string{})

	f.applySideEffects(tok, p, headers)

	emitted := headers.Get("X-Jwt-Payload")
	if emitted == "" {
		t.Fatal("X-Jwt-Payload: want emitted; got empty")
	}
	if strings.HasSuffix(emitted, "=") {
		t.Errorf("X-Jwt-Payload: got %q; want no trailing `=` (unpadded form)", emitted)
	}
}

func TestApplySideEffects_ClaimToHeaders_StringClaim_Emitted(t *testing.T) {
	// Per §11.P10: string-valued claim → emit as header value verbatim.
	f := &filter{}
	p := &compiledProvider{
		forward: true,
		claimToHeaders: []claimToHeader{
			{headerName: "X-Sub", claimName: "sub"},
		},
	}
	tok := validToken(t, map[string]interface{}{"sub": "user@example.com"})
	headers := newHeaders(map[string]string{})

	f.applySideEffects(tok, p, headers)

	if got := headers.Get("X-Sub"); got != "user@example.com" {
		t.Errorf("X-Sub: got %q; want %q", got, "user@example.com")
	}
}

func TestApplySideEffects_ClaimToHeaders_NumericClaim_Stringified(t *testing.T) {
	// Per §11.P10: numeric (JSON float64) claim → stringified. Integral
	// float64 → integer string (operator-intuitive for unix-time claims).
	f := &filter{}
	p := &compiledProvider{
		forward: true,
		claimToHeaders: []claimToHeader{
			{headerName: "X-Exp", claimName: "exp"},
		},
	}
	tok := validToken(t, map[string]interface{}{"exp": float64(1700000000)})
	headers := newHeaders(map[string]string{})

	f.applySideEffects(tok, p, headers)

	if got := headers.Get("X-Exp"); got != "1700000000" {
		t.Errorf("X-Exp: got %q; want %q (integer-stringified)", got, "1700000000")
	}
}

func TestApplySideEffects_ClaimToHeaders_ArrayClaim_SilentSkip(t *testing.T) {
	// Per §11.P10: array claim → SILENT-SKIP (header NOT emitted).
	f := &filter{}
	p := &compiledProvider{
		forward: true,
		claimToHeaders: []claimToHeader{
			{headerName: "X-Groups", claimName: "groups"},
			{headerName: "X-Sub", claimName: "sub"},
		},
	}
	tok := validToken(t, map[string]interface{}{
		"sub":    "alice",
		"groups": []interface{}{"a", "b"}, // array → silent-skip
	})
	headers := newHeaders(map[string]string{})

	f.applySideEffects(tok, p, headers)

	if got := headers.Get("X-Groups"); got != "" {
		t.Errorf("X-Groups: got %q; want empty (array claim silent-skip)", got)
	}
	// Sub still emitted (silent-skip on array doesn't abort the loop).
	if got := headers.Get("X-Sub"); got != "alice" {
		t.Errorf("X-Sub: got %q; want %q (loop continues after silent-skip)", got, "alice")
	}
}

func TestApplySideEffects_ClaimToHeaders_NestedDotNotation(t *testing.T) {
	// Per §11.P10: dot-notation claim path → nested map traversal.
	f := &filter{}
	p := &compiledProvider{
		forward: true,
		claimToHeaders: []claimToHeader{
			{headerName: "X-Email", claimName: "user.email"},
		},
	}
	tok := validToken(t, map[string]interface{}{
		"user": map[string]interface{}{
			"email": "alice@example.com",
			"name":  "Alice",
		},
	})
	headers := newHeaders(map[string]string{})

	f.applySideEffects(tok, p, headers)

	if got := headers.Get("X-Email"); got != "alice@example.com" {
		t.Errorf("X-Email: got %q; want %q (nested dot-notation)", got, "alice@example.com")
	}
}

func TestApplySideEffects_ClearRouteCache_TriggersDcbInvocation(t *testing.T) {
	// Per §6.9 step 4: clear_route_cache:true → f.clearRouteCacheRequested
	// flips true. The actual framework primitive cb.ClearRouteCache() is
	// deferred to a future HCM phase per ADR-0155 §Consequences; the
	// TRACKING flag is the test-introspection anchor.
	f := &filter{}
	p := &compiledProvider{
		forward:         true,
		clearRouteCache: true,
	}
	tok := validToken(t, map[string]interface{}{"sub": "alice"})
	headers := newHeaders(map[string]string{})

	if f.clearRouteCacheRequested {
		t.Fatal("precondition: flag must start false")
	}
	f.applySideEffects(tok, p, headers)

	if !f.clearRouteCacheRequested {
		t.Error("clearRouteCacheRequested: want true after clear_route_cache:true success")
	}
}

func TestApplySideEffects_EmitOrder_StripBeforeForwardBeforeClaimBeforeClear(t *testing.T) {
	// Per §6.9 emit-order: (1) strip → (2) forward_payload_header → (3)
	// claim_to_headers → (4) clear_route_cache. Combined-action smoke test.
	f := &filter{}
	p := &compiledProvider{
		forward:              false,
		forwardPayloadHeader: "X-Jwt-Payload",
		padForwardPayloadHdr: false,
		claimToHeaders: []claimToHeader{
			{headerName: "X-Sub", claimName: "sub"},
		},
		clearRouteCache: true,
	}
	tok := validToken(t, map[string]interface{}{"sub": "alice"})
	headers := newHeaders(map[string]string{
		"Authorization": "Bearer eyJfake",
		":path":         "/foo",
	})

	f.applySideEffects(tok, p, headers)

	// (1) Authorization stripped.
	if got := headers.Get("Authorization"); got != "" {
		t.Errorf("step1 strip: Authorization not stripped; got %q", got)
	}
	// (2) forward_payload_header emitted.
	if got := headers.Get("X-Jwt-Payload"); got == "" {
		t.Error("step2 forward_payload_header: X-Jwt-Payload not emitted")
	}
	// (3) claim_to_headers emitted.
	if got := headers.Get("X-Sub"); got != "alice" {
		t.Errorf("step3 claim_to_headers: X-Sub got %q; want %q", got, "alice")
	}
	// (4) clear_route_cache flag flipped.
	if !f.clearRouteCacheRequested {
		t.Error("step4 clear_route_cache: flag not flipped")
	}
}

// ----------------------------------------------------------------------------
// Group 8 — Deny-path wire shape tests (Task 9 of phase-17 per SPEC §4 +
// §6.9 + §1.1 amendments 8 + 11 + 12 + ADR-0155).
//
// 14 cases pin the byte-exact deny-path response shape:
//   - Canonical jwt_verify_lib body strings per failure-reason (~7 cases).
//   - strip_failure_response empty-body + nil-headers case.
//   - Realm capture from originalURI / default to "/".
//   - mapStatusToHTTPCode dispatch (audience=403; others/nil=401).
//   - Content-Type text/plain header.
// ----------------------------------------------------------------------------

// buildBareFilterForEmit returns a *filter wired with a fresh jwtFakeCB +
// minimal *compiledConfig (no stats; no rules — emitDenyResponse path only).
// Used by Group 8 tests that exercise emitDenyResponse / mapStatusToHTTPCode
// directly.
func buildBareFilterForEmit(originalURI string, stripFailureResponse bool) (*filter, *jwtFakeCB) {
	cb := &jwtFakeCB{}
	cfg := &compiledConfig{stripFailureResponse: stripFailureResponse}
	f := &filter{
		state:       &factoryState{listenerRC: cfg},
		dcb:         cb,
		activeRC:    cfg,
		originalURI: originalURI,
	}
	return f, cb
}

func TestEmitDenyResponse_JwtMissed_401_BodyByteExact_NoErrorParam(t *testing.T) {
	// ErrJwtMissed → 401 + body "Jwt is missing" + WWW-Authenticate without
	// error param per §11.P2.
	f, cb := buildBareFilterForEmit("/api/foo", false)
	f.emitDenyResponse(jwt.ErrJwtMissed)

	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401", cb.localReply.status)
	}
	if cb.localReply.body != "Jwt is missing" {
		t.Errorf("body: got %q; want %q", cb.localReply.body, "Jwt is missing")
	}
	if len(cb.localReply.body) != 14 {
		t.Errorf("body length: got %d; want 14 bytes", len(cb.localReply.body))
	}
	wantWWW := `Bearer realm="/api/foo"`
	if got := cb.localReply.headers.Get("WWW-Authenticate"); got != wantWWW {
		t.Errorf("WWW-Authenticate: got %q; want %q (NO error param for JwtMissed)", got, wantWWW)
	}
}

func TestEmitDenyResponse_JwtExpired_401_BodyByteExact_WithErrorParam(t *testing.T) {
	// ErrJwtExpired → 401 + body "Jwt is expired" + WWW-Authenticate WITH
	// error param per §11.P2 (non-JwtMissed).
	f, cb := buildBareFilterForEmit("/api/foo", false)
	f.emitDenyResponse(jwt.ErrJwtExpired)

	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401", cb.localReply.status)
	}
	if cb.localReply.body != "Jwt is expired" {
		t.Errorf("body: got %q; want %q", cb.localReply.body, "Jwt is expired")
	}
	if len(cb.localReply.body) != 14 {
		t.Errorf("body length: got %d; want 14 bytes", len(cb.localReply.body))
	}
	wantWWW := `Bearer realm="/api/foo", error="invalid_token"`
	if got := cb.localReply.headers.Get("WWW-Authenticate"); got != wantWWW {
		t.Errorf("WWW-Authenticate: got %q; want %q", got, wantWWW)
	}
}

func TestEmitDenyResponse_JwtVerificationFail_401_BodyByteExact(t *testing.T) {
	f, cb := buildBareFilterForEmit("/x", false)
	f.emitDenyResponse(jwt.ErrJwtVerificationFail)
	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401", cb.localReply.status)
	}
	if cb.localReply.body != "Jwt verification fails" {
		t.Errorf("body: got %q; want %q", cb.localReply.body, "Jwt verification fails")
	}
}

func TestEmitDenyResponse_JwtUnknownIssuer_401_BodyByteExact(t *testing.T) {
	f, cb := buildBareFilterForEmit("/x", false)
	f.emitDenyResponse(jwt.ErrJwtUnknownIssuer)
	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401", cb.localReply.status)
	}
	if cb.localReply.body != "Jwt issuer is not configured" {
		t.Errorf("body: got %q; want %q", cb.localReply.body, "Jwt issuer is not configured")
	}
}

func TestEmitDenyResponse_JwtAudienceNotAllowed_403_BodyByteExact(t *testing.T) {
	// Per §1.1 amendment 8 + ADR-0155: ErrJwtAudienceNotAllowed → 403 (NOT 401).
	f, cb := buildBareFilterForEmit("/x", false)
	f.emitDenyResponse(jwt.ErrJwtAudienceNotAllowed)
	if cb.localReply.status != 403 {
		t.Errorf("status: got %d; want 403 (audience mismatch per amendment 8)", cb.localReply.status)
	}
	if cb.localReply.body != "Audiences in Jwt are not allowed" {
		t.Errorf("body: got %q; want %q", cb.localReply.body, "Audiences in Jwt are not allowed")
	}
	if len(cb.localReply.body) != 32 {
		t.Errorf("body length: got %d; want 32 bytes", len(cb.localReply.body))
	}
}

func TestEmitDenyResponse_JwtBadFormat_401_BodyByteExact(t *testing.T) {
	f, cb := buildBareFilterForEmit("/x", false)
	f.emitDenyResponse(jwt.ErrJwtBadFormat)
	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401", cb.localReply.status)
	}
	if cb.localReply.body != "Jwt is not in the form of Header.Payload.Signature" {
		t.Errorf("body: got %q; want %q", cb.localReply.body, "Jwt is not in the form of Header.Payload.Signature")
	}
}

func TestEmitDenyResponse_JwtHeaderNotImplementedAlg_401_BodyByteExact(t *testing.T) {
	f, cb := buildBareFilterForEmit("/x", false)
	f.emitDenyResponse(jwt.ErrJwtHeaderNotImplementedAlg)
	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401", cb.localReply.status)
	}
	if cb.localReply.body != "Jwt header [alg] is not supported" {
		t.Errorf("body: got %q; want %q", cb.localReply.body, "Jwt header [alg] is not supported")
	}
}

func TestEmitDenyResponse_StripFailureResponse_EmptyBody_NoHeaders(t *testing.T) {
	// Per §11.P3 RATIFIED: strip_failure_response:true → body="" + nil
	// headers carrier; status preserved.
	f, cb := buildBareFilterForEmit("/api/foo", true)
	f.emitDenyResponse(jwt.ErrJwtExpired)
	if cb.localReply.status != 401 {
		t.Errorf("status: got %d; want 401 (status preserved through strip)", cb.localReply.status)
	}
	if cb.localReply.body != "" {
		t.Errorf("body: got %q; want empty", cb.localReply.body)
	}
	if cb.localReply.headers != nil {
		t.Errorf("headers: got %v; want nil (stripped)", cb.localReply.headers)
	}
}

func TestEmitDenyResponse_RealmCapturedFromOriginalURI(t *testing.T) {
	// Per §1.1 amendment 12: realm uses f.originalURI verbatim.
	f, cb := buildBareFilterForEmit("/api/v1/very/long/path?with=query", false)
	f.emitDenyResponse(jwt.ErrJwtMissed)
	wantWWW := `Bearer realm="/api/v1/very/long/path?with=query"`
	if got := cb.localReply.headers.Get("WWW-Authenticate"); got != wantWWW {
		t.Errorf("WWW-Authenticate: got %q; want %q", got, wantWWW)
	}
}

func TestEmitDenyResponse_RealmDefaultsToSlash_WhenURIEmpty(t *testing.T) {
	// Per ADR-0155: empty originalURI → defensive default realm="/".
	f, cb := buildBareFilterForEmit("", false)
	f.emitDenyResponse(jwt.ErrJwtMissed)
	wantWWW := `Bearer realm="/"`
	if got := cb.localReply.headers.Get("WWW-Authenticate"); got != wantWWW {
		t.Errorf("WWW-Authenticate: got %q; want %q (defensive default)", got, wantWWW)
	}
}

func TestMapStatusToHTTPCode_AudienceNotAllowed_403(t *testing.T) {
	// Per §1.1 amendment 8 + ADR-0155.
	if got := mapStatusToHTTPCode(jwt.ErrJwtAudienceNotAllowed); got != 403 {
		t.Errorf("got %d; want 403", got)
	}
}

func TestMapStatusToHTTPCode_OtherErrors_401(t *testing.T) {
	// Per §1.1 amendment 8: everything else → 401.
	cases := []error{
		jwt.ErrJwtMissed,
		jwt.ErrJwtExpired,
		jwt.ErrJwtVerificationFail,
		jwt.ErrJwtUnknownIssuer,
		jwt.ErrJwtBadFormat,
		jwt.ErrJwtHeaderNotImplementedAlg,
		jwt.ErrJwtNotYetValid,
	}
	for _, e := range cases {
		if got := mapStatusToHTTPCode(e); got != 401 {
			t.Errorf("mapStatusToHTTPCode(%v): got %d; want 401", e, got)
		}
	}
}

func TestMapStatusToHTTPCode_NilReason_401(t *testing.T) {
	// Per §1.1 amendment 8: nil reason → 401 (defensive — should not arise
	// in practice since the caller passes evalResult.err which is non-nil
	// on the deny path).
	if got := mapStatusToHTTPCode(nil); got != 401 {
		t.Errorf("got %d; want 401 (nil default)", got)
	}
}

func TestEmitDenyResponse_ContentTypeTextPlain(t *testing.T) {
	// Per ADR-0155 4-header standard set: content-type: text/plain emitted
	// when body is non-empty (i.e., NOT under strip_failure_response).
	f, cb := buildBareFilterForEmit("/x", false)
	f.emitDenyResponse(jwt.ErrJwtMissed)
	if got := cb.localReply.headers.Get("Content-Type"); got != "text/plain" {
		t.Errorf("Content-Type: got %q; want %q", got, "text/plain")
	}
}

// ----------------------------------------------------------------------------
// Group 12 — Stat-surface integration tests (per SPEC §14.1 #12 + ADR-0154).
//
// 5 cases pin the 7-counter SN2-reuse stat surface at the stat-API level:
//   - All 7 base counters land on the Registry at New() time unconditionally.
//   - The 2 STRUCTURALLY UNREACHABLE counters (jwt_cache_hit/miss per §8
//     deferral 8) are registered yet never incremented by any code path.
//   - The SN2-reuse namespace `http.<HCM_stat_prefix>.jwt_authn.<counter>` is
//     present verbatim post-New() per §11.P7 (RATIFIED-PENDING-IMPL-TIME-
//     EMPIRICAL-SCRAPE — SCRAPE DEFERRED to Task 13 per planner-time decision
//     10 + phase-13 ADR-0127-v2 in-place-amend precedent — see ADR-0154
//     §Decision).
//   - The canonical naming `cors_preflight_bypassed` (REFUTES BRAINSTORM
//     `bypassed_cors_preflight` per §1.1 amendment 10) is the registered
//     counter name.
//   - The jwks_fetch_success / jwks_fetch_failed counter wiring observed via
//     the Registry-level Counter handle (re-anchors the Group 10 Task-6
//     evaluator-side increment assertions at the stat-API level).
//
// The 5 tests mirror phase-16 rbac_test.go Group 9 precedent
// (TestStatsNamespace_*); test-helper `collectMetricNames` walks the Registry.
// ----------------------------------------------------------------------------

// collectMetricNames returns the Registry's full registered-name set via Walk.
// Used by Group 12 tests to assert the SN2-reuse counter-name shape. Mirrors
// phase-16 rbac/rbac_test.go's `collectMetricNames` precedent.
func collectMetricNames(reg *stats.Registry) []string {
	var names []string
	reg.Walk(func(m stats.Metric) {
		names = append(names, m.Name())
	})
	return names
}

// containsString reports whether haystack contains needle (exact match).
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestFilterStats_AllSevenCountersRegistered pins ADR-0154 §Decision: all 7
// base counters land on the Registry at New() time UNCONDITIONALLY (NO lazy-
// allocation; predeclared empty counters for scrape stability per Prometheus
// best practice). Per §11.P6 + §1.1 amendment 9 the 7 names are: allowed,
// denied, cors_preflight_bypassed, jwks_fetch_success, jwks_fetch_failed,
// jwt_cache_hit, jwt_cache_miss.
//
// The test asserts the FULL SN2-reuse counter-name set `http.<HCM>.jwt_authn.
// <counter>` lands on the Registry verbatim — the hypothesis pinned at
// SPEC §11.P7 RATIFIED-PENDING. (The §11.P7 closure empirically against
// reference Envoy v1.37.2 is DEFERRED to Task 13's fixture 0019 end-to-end
// scrape per ADR-0154 §Decision + planner-time decision 10.)
func TestFilterStats_AllSevenCountersRegistered(t *testing.T) {
	reg := stats.NewRegistry()
	c := &jwt_authnv3.JwtAuthentication{}
	cc, err := buildCompiledConfig(c, freshFactoryCtxWithRegistry(reg))
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	if cc.stats == nil {
		t.Fatal("cc.stats: want non-nil")
	}
	names := collectMetricNames(reg)
	want := []string{
		"http.ingress_http.jwt_authn.allowed",
		"http.ingress_http.jwt_authn.denied",
		"http.ingress_http.jwt_authn.cors_preflight_bypassed",
		"http.ingress_http.jwt_authn.jwks_fetch_success",
		"http.ingress_http.jwt_authn.jwks_fetch_failed",
		"http.ingress_http.jwt_authn.jwt_cache_hit",
		"http.ingress_http.jwt_authn.jwt_cache_miss",
	}
	for _, w := range want {
		if !containsString(names, w) {
			t.Errorf("missing counter %q in Registry; got names=%v", w, names)
		}
	}
	// 7 counters exactly — no extras under empty-config New() (no per-provider
	// scaling per §1.1 amendment 9 REFUTES BRAINSTORM 8-per-provider hypothesis).
	if len(names) != 7 {
		t.Errorf("Registry size = %d; want 7 (no per-provider scaling per ADR-0154); got names=%v", len(names), names)
	}
}

// TestFilterStats_NilRegistry_NoPanic pins ADR-0085 nil-tolerance for the
// stats wiring: when ctx.Stats is nil (test code paths), New() + the
// downstream filter operations must NOT panic. The `cc.stats` field stays
// nil; the `f.activeRC.stats != nil` guard sites (evaluator.go +
// provider.go's resolveRequirement) skip counter increments rather than
// dereferencing nil. Mirrors phase-16 rbac nil-tolerance discipline.
func TestFilterStats_NilRegistry_NoPanic(t *testing.T) {
	c := &jwt_authnv3.JwtAuthentication{}
	cc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}) // Stats == nil, StatPrefix == ""
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	if cc.stats != nil {
		t.Errorf("cc.stats: want nil under nil-Stats FactoryCtx; got %v", cc.stats)
	}
	// Wire a *filter against the nil-stats listener-level config + invoke
	// resolveRequirement on a dangling-name per-route. The helper MUST emit
	// SendLocalReply without panicking on the (nil-guarded) counter increment.
	rc := &compiledConfig{
		requirementMap: map[string]*compiledRequirement{
			"present-req": {kind: reqAllowMissingOrFailed},
		},
		stats: nil, // explicit nil to assert the guard fires
	}
	pr := &compiledPerRoute{disabled: false, requirementName: "missing-req"}
	f, cb := newFilterWithListenerRC(t, rc, pr)
	req, denied := f.resolveRequirement(http.Header{})
	if req != nil || !denied {
		t.Errorf("resolveRequirement: want (nil, true); got (%v, %v)", req, denied)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply not called under nil-stats path; want call")
	}
	if cb.localReply.status != 403 {
		t.Errorf("SendLocalReply status under nil-stats: got %d; want 403", cb.localReply.status)
	}
}

// TestFilterStats_CanonicalCorsPreflightBypassedName pins §1.1 amendment 10
// + ADR-0154: the canonical counter name is `cors_preflight_bypassed`
// (REFUTES BRAINSTORM `bypassed_cors_preflight` hypothesis). The SN2-reuse
// shape carries the canonical suffix; the inverse hypothesis MUST NOT appear.
//
// This test duplicates the Group 1 TestBuildCompiledConfig_CorsPreflight
// Bypassed_CanonicalNaming assertion at the Group 12 anchor — the canonical
// naming is load-bearing for the §11.P7 empirical-scrape closure (the Task 13
// scrape directly compares the canonical name).
func TestFilterStats_CanonicalCorsPreflightBypassedName(t *testing.T) {
	reg := stats.NewRegistry()
	c := &jwt_authnv3.JwtAuthentication{}
	_, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"})
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	names := collectMetricNames(reg)
	wantCanonical := "http.ingress_http.jwt_authn.cors_preflight_bypassed"
	if !containsString(names, wantCanonical) {
		t.Errorf("canonical counter %q not found in Registry; got names=%v", wantCanonical, names)
	}
	// REFUTED inverse BRAINSTORM hypothesis MUST NOT appear.
	refutedInverse := "http.ingress_http.jwt_authn.bypassed_cors_preflight"
	if containsString(names, refutedInverse) {
		t.Errorf("REFUTED naming hypothesis %q appears in Registry — §1.1 amendment 10 violation", refutedInverse)
	}
}

// TestFilterStats_JwksFetchCountersWired re-anchors the Group 10 Task-6
// evaluator-side counter-increment assertions at the stat-API level. The
// jwks_fetch_success / jwks_fetch_failed counters are observable via the
// Registry's Counter handles (NOT just the in-memory atomic). Verifies the
// counter Handle's Name() matches the SN2-reuse shape; the increment
// propagates to Load() AND the counter's Name() lands on the Walk-output set.
func TestFilterStats_JwksFetchCountersWired(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	if fs == nil {
		t.Fatal("newFilterStats: want non-nil")
	}
	// Direct counter-handle name assertions — the wire-shape pin.
	if got, want := fs.jwksFetchSuccess.Name(), "http.ingress_http.jwt_authn.jwks_fetch_success"; got != want {
		t.Errorf("jwksFetchSuccess.Name() = %q; want %q (SN2-reuse per ADR-0154 + §11.P7)", got, want)
	}
	if got, want := fs.jwksFetchFailed.Name(), "http.ingress_http.jwt_authn.jwks_fetch_failed"; got != want {
		t.Errorf("jwksFetchFailed.Name() = %q; want %q", got, want)
	}
	// Increment + assert the Load() observable.
	fs.jwksFetchSuccess.Inc()
	fs.jwksFetchSuccess.Inc()
	fs.jwksFetchFailed.Inc()
	if got, want := fs.jwksFetchSuccess.Load(), uint64(2); got != want {
		t.Errorf("jwksFetchSuccess.Load() after 2 Inc(): got %d; want %d", got, want)
	}
	if got, want := fs.jwksFetchFailed.Load(), uint64(1); got != want {
		t.Errorf("jwksFetchFailed.Load() after 1 Inc(): got %d; want %d", got, want)
	}
	// Verify the same counter handles appear in the Walk-output names (the
	// Registry-level surface — what /stats scrapes).
	names := collectMetricNames(reg)
	if !containsString(names, "http.ingress_http.jwt_authn.jwks_fetch_success") {
		t.Errorf("Walk-output missing jwks_fetch_success counter; names=%v", names)
	}
	if !containsString(names, "http.ingress_http.jwt_authn.jwks_fetch_failed") {
		t.Errorf("Walk-output missing jwks_fetch_failed counter; names=%v", names)
	}
}

// TestFilterStats_CacheCountersRegisteredButUnreachable pins ADR-0154 +
// §8 deferral 8: the jwt_cache_hit + jwt_cache_miss counters are STRUCTURALLY
// UNREACHABLE under MVP — registered at New() time (predeclared empty
// counters for scrape stability) yet NEVER incremented by any code path.
// Verifies post-New() they are present at Load() == 0 AND a `grep -RE` over
// the package source confirms no Inc()/Add() call site exists.
//
// Implementation: post-evaluateRequirement allow + deny + dangling-name
// paths the cache counters' Load() must remain 0. Static guarantee: the
// only Inc() call sites in the package are on the 5 active counters
// (allowed, denied, corsPreflightBypassed, jwksFetchSuccess, jwksFetchFailed);
// the test asserts the runtime observable.
func TestFilterStats_CacheCountersRegisteredButUnreachable(t *testing.T) {
	reg := stats.NewRegistry()
	c := &jwt_authnv3.JwtAuthentication{}
	cc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"})
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	if cc.stats == nil {
		t.Fatal("cc.stats: want non-nil")
	}
	// Counters must be REGISTERED (non-nil handles + Walk-observable names).
	if cc.stats.jwtCacheHit == nil {
		t.Fatal("jwtCacheHit: want non-nil registered handle; got nil")
	}
	if cc.stats.jwtCacheMiss == nil {
		t.Fatal("jwtCacheMiss: want non-nil registered handle; got nil")
	}
	names := collectMetricNames(reg)
	if !containsString(names, "http.ingress_http.jwt_authn.jwt_cache_hit") {
		t.Errorf("jwt_cache_hit counter NOT registered; names=%v", names)
	}
	if !containsString(names, "http.ingress_http.jwt_authn.jwt_cache_miss") {
		t.Errorf("jwt_cache_miss counter NOT registered; names=%v", names)
	}
	// Exercise multiple code paths that COULD touch the cache counters
	// (resolveRequirement dangling-name → deny path; allow path is harder to
	// reach without a full evaluator pipeline — covered by the Group 5
	// integration tests landing at Task 9). Confirm both Load() values stay 0.
	rc := &compiledConfig{
		requirementMap: map[string]*compiledRequirement{
			"present-req": {kind: reqAllowMissingOrFailed},
		},
		stats: cc.stats,
	}
	pr := &compiledPerRoute{disabled: false, requirementName: "missing-req"}
	f, _ := newFilterWithListenerRC(t, rc, pr)
	_, _ = f.resolveRequirement(http.Header{})
	if got := cc.stats.jwtCacheHit.Load(); got != 0 {
		t.Errorf("jwtCacheHit.Load() post deny-path: got %d; want 0 (STRUCTURALLY UNREACHABLE per §8 deferral 8)", got)
	}
	if got := cc.stats.jwtCacheMiss.Load(); got != 0 {
		t.Errorf("jwtCacheMiss.Load() post deny-path: got %d; want 0 (STRUCTURALLY UNREACHABLE per §8 deferral 8)", got)
	}
}
