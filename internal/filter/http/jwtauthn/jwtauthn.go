package jwtauthn

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	jwt_authnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/httpclient"
	"github.com/esalaine/envoy-go/internal/jwks"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TypeURL is the canonical envoy.filters.http.jwt_authn typed_config type URL.
// Boot wiring at cmd/envoy-go/main.go (Task 10) registers New under this key
// in the HTTPRegistry per ADR-0072 + ADR-0148.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication"

// filterName is the canonical http_filters[].name string for jwt_authn
// (matches the listener config typed_per_filter_config map keys).
const filterName = "envoy.filters.http.jwt_authn"

// compiledConfig is the parsed + validated runtime config for one jwt_authn
// filter instance. 6-field shape per SPEC §6.2 + ADR-0149. Closure-capture
// parse-at-New + read-only-shared-after-New per ADR-0101.
//
// Per ADR-0153 SHARED-stats discipline: per-route disposition does NOT spawn
// a fresh *compiledConfig (string-reference-delegation into the listener-level
// requirement_map). The listener-level *compiledConfig is the SOLE config
// instance; per-route entries materialize as lightweight *compiledPerRoute
// values that select among the listener-level config's pre-compiled state.
type compiledConfig struct {
	// providers map<string, JwtProvider> — parsed per §6.5.
	providers map[string]*compiledProvider

	// rules — listener-level RequirementRule entries (first-match dispatch
	// per §6.6 + §11.P12). At Task 2 the matcher is NOT compiled (router
	// extraction is a future surface); the requirement is compiled but the
	// match predicate dispatch lands at Task 6.
	rules []*compiledRule

	// requirementMap — named JwtRequirement entries referenced by listener-
	// level RequirementRule.requirement_name AND per-route TPFC
	// PerRouteConfig.requirement_name per §5.1 + §11.P12.
	requirementMap map[string]*compiledRequirement

	// bypassCorsPreflight + stripFailureResponse — per amendments 10 + 12.
	bypassCorsPreflight  bool
	stripFailureResponse bool

	// stats — nil when ctx.Stats is nil (test path per ADR-0085 nil-tolerance).
	// 7 base counters registered unconditionally at New() time per ADR-0148 +
	// ADR-0154; lazy-allocation discipline NOT applied (mirrors phase-12 csrf
	// + phase-13 buffer SHARED-stats precedent).
	stats *filterStats
}

// compiledProvider carries one parsed + validated JwtProvider entry per §6.2 +
// ADR-0149. 13 consumed proto-faithful fields per amendments 2 + 3; 8
// silent-ignored fields (payload_in_metadata, header_in_metadata,
// failed_status_in_metadata, normalize_payload_in_metadata, jwt_cache_config,
// subjects, require_expiration, max_lifetime) have no slot here (silent-ignore
// is STRUCTURAL).
//
// JWKS source: mutually exclusive — jwksFetcher (RemoteJwks) XOR localJwks
// (LocalJwks). Task 3 wires *jwks.Fetcher (ADR-0150); Task 4 wires
// *jwks.JWKSet (ADR-0151 — Task 4's `internal/jwt/` framework primitive
// consumes a parsed `*jwks.JWKSet` from the `internal/jwks/` sibling at
// Task 6's evaluateProvider; the LocalJwks branch at buildCompiledProvider
// time calls `jwks.ParseJWKSet(raw)` directly).
type compiledProvider struct {
	issuer               string        // "" means skip iss check per JwtProvider.issuer proto comment
	audiences            []string      // empty means skip aud check
	jwksFetcher          *jwks.Fetcher // RemoteJwks path; nil iff LocalJwks
	localJwks            *jwks.JWKSet  // LocalJwks path; nil iff RemoteJwks
	forward              bool          // false (default) → strip JWT from request on success
	fromHeaders          []headerLoc   // parsed JwtHeader entries
	fromParams           []string      // case-sensitive query param names
	fromCookies          []string      // case-sensitive cookie names
	forwardPayloadHeader string        // header name to emit base64url(payload); "" → skip
	padForwardPayloadHdr bool          // preserve = padding when true
	claimToHeaders       []claimToHeader
	clearRouteCache      bool          // invoke cb.ClearRouteCache() on success
	clockSkew            time.Duration // default 60s per amendment 7
}

// headerLoc carries one JwtHeader entry per §6.2.
type headerLoc struct {
	name        string // HTTP header name (case-insensitive HTTP header lookup at request time)
	valuePrefix string // e.g., "Bearer " — substring-searched in the header value
}

// claimToHeader carries one JwtClaimToHeader entry per §6.2.
type claimToHeader struct {
	headerName string // destination request header
	claimName  string // JWT payload claim path (dot-notation for nested per §11.P10)
}

// compiledRule carries one parsed RequirementRule per §6.2 + §11.P12.
//
// matchFn is the compiled route-match predicate. The Task-2 placeholder
// `matcher any` (proto.Message) was replaced at Task 13 with an inline
// compiled predicate covering the two path-specifier arms exercised by
// fixture 0019: PATH (exact) + PREFIX. The remaining 4 arms (SafeRegex,
// PathSeparatedPrefix, ConnectMatcher, plus headers / query_parameters /
// runtime_fraction / dynamic_metadata / tls_context predicates) are
// REJECTED at compile time per §11.P9 + the phase-04 buildMatch precedent
// at internal/filter/hcm/config.go:367. ADR-0149's §11.P9 deferral of
// router.NewRouteMatcher extraction stands — once that extraction lands,
// matchFn can be retired in favor of the shared matcher type.
//
// matchFn nil means "wildcard match" (kept for safety; current compile path
// always populates matchFn). The factory PARSE-REJECTS on missing match.
type compiledRule struct {
	matchFn     func(path string) bool
	requirement *compiledRequirement
}

// requirementKind enumerates the 6 JwtRequirement variants per §11.P16 +
// ADR-0149.
type requirementKind int

const (
	// reqProviderName: validate JWT against named provider.
	reqProviderName requirementKind = iota
	// reqProviderAndAudiences: validate against named provider with per-rule
	// audience override.
	reqProviderAndAudiences
	// reqRequiresAny: OR-semantic; short-circuit on first success.
	reqRequiresAny
	// reqRequiresAll: AND-semantic; short-circuit on first failure.
	reqRequiresAll
	// reqAllowMissing: JWT absent → OK; JWT present-and-invalid → FAIL.
	reqAllowMissing
	// reqAllowMissingOrFailed: any outcome → OK.
	reqAllowMissingOrFailed
)

// compiledRequirement is the parsed + recursively-built JwtRequirement
// evaluator tree per §6.2 + §11.P16 + ADR-0149.
type compiledRequirement struct {
	kind     requirementKind
	provider *compiledProvider      // for reqProviderName + reqProviderAndAudiences
	audOverr []string               // for reqProviderAndAudiences (per-rule audience override)
	children []*compiledRequirement // for reqRequiresAny + reqRequiresAll (recursive)
}

// compiledPerRoute wraps the per-route 8th canonical disposition per §5.1 +
// ADR-0125 §(xiii) + ADR-0153. Three cases:
//
//	(a) disabled=true / requirementName="" — per-route disabled; passthrough.
//	(b) disabled=false / requirementName="" — explicit-not-disabled; falls
//	    through to listener-level rules dispatch.
//	(c) disabled=false / requirementName="<name>" — runtime-resolve at request
//	    time per ADR-0153; dangling reference yields 403 + plain body per
//	    ADR-0155.
type compiledPerRoute struct {
	disabled        bool   // case (a) when true
	requirementName string // case (c) when non-empty
}

// factoryState is the closure-captured shared state per factory invocation per
// SPEC §6.3 + ADR-0117 + ADR-0125 §(xiii) + ADR-0153. SIMPLIFIED relative to
// phase-11/15/16 (which carried per-route lazy-cache `sync.Map` for
// INDEPENDENT-stats): phase-17 SHARED-stats discipline (per ADR-0154) does
// NOT spawn per-route *filterStats. The sync.Map carries lightweight
// *compiledPerRoute entries only.
type factoryState struct {
	listenerRC *compiledConfig
	// perRoute is keyed by *jwt_authnv3.PerRouteConfig pointer-identity per
	// ADR-0117 + ADR-0125 §(xiii). Race-safe via sync.Map's LoadOrStore
	// contract.
	perRoute sync.Map // map[*jwt_authnv3.PerRouteConfig]*compiledPerRoute
}

// filter is the per-stream filter instance allocated by the factory closure.
// Decoder-only per ADR-0148 §Decision; no encode-side state. Mirrors
// phase-12 csrf + phase-13 buffer + phase-16 rbac decoder-only precedent.
type filter struct {
	state *factoryState
	dcb   envoyhttp.DecoderFilterCallbacks

	// Per-stream state cached at DecodeHeaders. Per SPEC §6.6.
	activeRC    *compiledConfig   // always listener-level config (per-route is delegation-only per ADR-0153)
	perRoute    *compiledPerRoute // resolved at DecodeHeaders; nil if no per-route entry
	passthrough bool              // true when per-route disabled=true OR no rules apply
	originalURI string            // captured at DecodeHeaders for WWW-Authenticate realm per amendment 12

	// now is the time-injection point for deterministic tests. Zero value (nil)
	// uses time.Now(). Tests set this to a fixed clock; production leaves it
	// nil. Read by `evaluateProvider` when constructing
	// `jwt.ValidateOptions{Now: ...}` per SPEC §6.8.
	now func() time.Time

	// clearRouteCacheRequested is set to true at applySideEffects step 4 when
	// a successfully-validated provider has clear_route_cache:true. It is a
	// TRACKING flag for test introspection — the actual framework primitive
	// `cb.ClearRouteCache()` is NOT yet exposed on DecoderFilterCallbacks at
	// phase-17 MVP (deferred to a future HCM framework phase per ADR-0155
	// §Decision). Tests assert this flag flips true; production sees a no-op
	// + the requested-side-effect carries forward to the future framework
	// primitive landing. Documented at ADR-0155 §Consequences as a forward-
	// pointer concern.
	clearRouteCacheRequested bool
}

// nowOrDefault returns f.now() if set, else time.Now(). Centralizes the
// time-injection discipline so callers don't repeat the nil-check.
func (f *filter) nowOrDefault() time.Time {
	if f.now != nil {
		return f.now()
	}
	return time.Now()
}

// filterStats is the 7-base-counter set per §1.1 amendment 9 + §11.P6 +
// ADR-0148 + ADR-0154. ALL 7 counters registered unconditionally at New()
// time (NO lazy-allocation; SHARED-stats discipline; mirrors phase-12 csrf +
// phase-13 buffer precedent — predeclared counters for scrape stability per
// Prometheus best practice).
//
// jwtCacheHit + jwtCacheMiss are STRUCTURALLY UNREACHABLE under phase-17 MVP
// per §8 deferral 8 — the jwt_cache_config JwtProvider field is silent-
// ignored, so the cache layer never materializes; the counters publish 0
// for the lifetime of the listener. They are registered for scrape-stability
// — operators get a consistent counter surface regardless of whether the MVP
// scope ever exercises the cache path. When jwt_cache_config support lands in
// a future phase, these counters wire into the cache lookup path.
type filterStats struct {
	allowed               *stats.Counter
	denied                *stats.Counter
	corsPreflightBypassed *stats.Counter
	jwksFetchSuccess      *stats.Counter
	jwksFetchFailed       *stats.Counter
	// jwtCacheHit is STRUCTURALLY UNREACHABLE under MVP per §8 deferral 8.
	jwtCacheHit *stats.Counter
	// jwtCacheMiss is STRUCTURALLY UNREACHABLE under MVP per §8 deferral 8.
	jwtCacheMiss *stats.Counter
}

// Compile-time assertion: *filter satisfies the decoder-only filter interface
// per ADR-0148 §Decision; mirrors phase-12 csrf + phase-13 buffer + phase-16
// rbac precedent.
var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)

// SetDecoderCallbacks stores the per-stream callbacks reference for later
// SendLocalReply + RequestRouteConfig use during DecodeHeaders. Per ADR-0148.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }

// New is the HTTPFilterFactory exposed at boot. Per ADR-0072 + ADR-0148 +
// ADR-0149:
//
//  1. tc must be non-nil (a jwt_authn filter with no typed_config surfaces a
//     config mistake; boot-time-fail-fast per ADR-0072).
//  2. Unmarshal tc to *jwt_authnv3.JwtAuthentication.
//  3. Invoke buildCompiledConfig (parses providers + requirement_map + rules;
//     applies envoy-go-side defensive PGV-mirror per §11.P9).
//  4. Capture *factoryState{listenerRC} for per-route lazy-cache.
//  5. Return FilterInstanceFactory closure that allocates a fresh *filter per
//     stream bound to *factoryState. The HTTPFilter value sets Decoder: f,
//     Encoder: nil — decoder-only per ADR-0148.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("jwt_authn: typed_config required")
	}
	var c jwt_authnv3.JwtAuthentication
	if err := tc.UnmarshalTo(&c); err != nil {
		return nil, fmt.Errorf("jwt_authn: unmarshal: %w", err)
	}
	rc, err := buildCompiledConfig(&c, ctx)
	if err != nil {
		return nil, err
	}
	state := &factoryState{listenerRC: rc}
	return func() envoyhttp.HTTPFilter {
		f := &filter{state: state}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: nil, // decoder-only per ADR-0148 §Decision
		}
	}, nil
}

// buildCompiledConfig parses + validates a JwtAuthentication outer envelope
// per SPEC §6.5 + ADR-0149. The 5-of-6 outer-field consumed shape: providers
// + rules + requirement_map + bypass_cors_preflight + strip_failure_response.
// filter_state_rules is SILENT-IGNORED per amendment 1 + §8 deferral 12.
//
// Order:
//
//  1. Outer flags (bypass_cors_preflight + strip_failure_response).
//  2. Providers map (each entry runs through buildCompiledProvider with
//     envoy-go defensive PGV-mirror per §11.P9; STUB at Task 2 for RemoteJwks
//     + LocalJwks paths).
//  3. requirement_map (each entry runs through buildCompiledRequirement;
//     resolves provider_name + provider_and_audiences references against
//     the providers map).
//  4. rules (each entry runs through buildCompiledRule; match-required PGV-
//     mirror; requirement resolution either inline `requires` OR
//     `requirement_name` lookup against the requirement_map — listener-level
//     dangling-name is PARSE-REJECT per §11.P12).
//  5. Stats — register 7 counters at HCM stat_prefix scope per ADR-0154.
//     Nil-tolerance for ctx.Stats==nil per ADR-0085.
func buildCompiledConfig(c *jwt_authnv3.JwtAuthentication, ctx envoyhttp.FactoryCtx) (*compiledConfig, error) {
	cc := &compiledConfig{
		bypassCorsPreflight:  c.GetBypassCorsPreflight(),
		stripFailureResponse: c.GetStripFailureResponse(),
	}

	// Parse providers map.
	//
	// Per ADR-0150 §Decision AMENDMENT (phase-20 IMPL Task 2b): the shared
	// `*httpclient.Client` (FactoryCtx.HTTPClient) is threaded into
	// buildCompiledProvider for the RemoteJwks path's `jwks.New` constructor.
	// Nil-tolerant: when FactoryCtx.HTTPClient is nil (test code), the jwks
	// Fetcher allocates its own per-Fetcher default `*httpclient.Client`
	// preserving the phase-17-pinned 30-second per-request timeout per
	// ADR-0150 §Decision AMENDMENT + ADR-0085 nil-tolerance.
	providers := make(map[string]*compiledProvider, len(c.GetProviders()))
	for name, p := range c.GetProviders() {
		cp, err := buildCompiledProvider(name, p, ctx.HTTPClient)
		if err != nil {
			return nil, fmt.Errorf("jwt_authn: provider %q: %w", name, err)
		}
		providers[name] = cp
	}
	cc.providers = providers

	// Parse requirement_map.
	reqMap := make(map[string]*compiledRequirement, len(c.GetRequirementMap()))
	for name, r := range c.GetRequirementMap() {
		cr, err := buildCompiledRequirement(r, providers)
		if err != nil {
			return nil, fmt.Errorf("jwt_authn: requirement %q: %w", name, err)
		}
		reqMap[name] = cr
	}
	cc.requirementMap = reqMap

	// Parse rules list (listener-level RequirementRule entries).
	rules := make([]*compiledRule, 0, len(c.GetRules()))
	for i, r := range c.GetRules() {
		rule, err := buildCompiledRule(r, providers, reqMap)
		if err != nil {
			return nil, fmt.Errorf("jwt_authn: rule[%d]: %w", i, err)
		}
		rules = append(rules, rule)
	}
	cc.rules = rules

	// filter_state_rules SILENT-IGNORED per §1.1 amendment 1 + §8 deferral 12.

	// Stats — register 7 counters at HCM stat_prefix scope per ADR-0154.
	// Per ADR-0085 nil-tolerance: ctx.Stats may be nil in test code paths.
	if ctx.Stats != nil {
		cc.stats = newFilterStats(ctx.Stats, ctx.StatPrefix)
		// Per Task-13 empirical reference scrape: reference Envoy increments
		// jwks_fetch_success ONCE per RemoteJwks provider at filter-load time
		// (the initial blocking fetch from jwks.New). Cache hits at request
		// time do NOT increment the counter. Mirror that discipline here by
		// crediting each RemoteJwks provider whose initial fetch succeeded
		// (jwksFetcher non-nil after buildCompiledProvider).
		for _, cp := range providers {
			if cp.jwksFetcher != nil {
				cc.stats.jwksFetchSuccess.Inc()
			}
		}
	}

	return cc, nil
}

// buildCompiledProvider parses one JwtProvider with envoy-go-side defensive
// PGV-mirror per SPEC §11.P9 + ADR-0149. Consumes 13 fields; silent-ignores
// 8 fields. JWKS source oneof dispatch — at Task 2 both RemoteJwks (lands
// Task 3 per ADR-0150) and LocalJwks (lands Task 4 per ADR-0151) paths return
// sentinel errors; the neither-set case PARSE-REJECTS per §11.P9 defensive
// PGV-mirror.
//
// Per ADR-0150 §Decision AMENDMENT (phase-20 IMPL Task 2b): the `httpClient`
// parameter is the shared `*httpclient.Client` framework primitive
// (ADR-0177) threaded from FactoryCtx.HTTPClient. The RemoteJwks branch
// passes it to `jwks.New` per ADR-0150 §Decision AMENDMENT. Nil-tolerant:
// nil httpClient lets the jwks Fetcher allocate its own per-Fetcher default
// preserving the phase-17-pinned 30-second per-request timeout.
func buildCompiledProvider(name string, p *jwt_authnv3.JwtProvider, httpClient *httpclient.Client) (*compiledProvider, error) {
	_ = name // currently unused in the Task 2 stub; reserved for richer error wording at Task 3+4.
	cp := &compiledProvider{
		issuer:               p.GetIssuer(),
		audiences:            p.GetAudiences(),
		forward:              p.GetForward(),
		forwardPayloadHeader: p.GetForwardPayloadHeader(),
		padForwardPayloadHdr: p.GetPadForwardPayloadHeader(),
		clearRouteCache:      p.GetClearRouteCache(),
		clockSkew:            durationOrDefault(p.GetClockSkewSeconds(), 60*time.Second),
	}

	// Parse from_headers, from_params, from_cookies.
	for _, h := range p.GetFromHeaders() {
		cp.fromHeaders = append(cp.fromHeaders, headerLoc{
			name:        h.GetName(),
			valuePrefix: h.GetValuePrefix(),
		})
	}
	cp.fromParams = p.GetFromParams()
	cp.fromCookies = p.GetFromCookies()

	// Parse claim_to_headers.
	for _, ch := range p.GetClaimToHeaders() {
		cp.claimToHeaders = append(cp.claimToHeaders, claimToHeader{
			headerName: ch.GetHeaderName(),
			claimName:  ch.GetClaimName(),
		})
	}

	// Silent-ignored fields not parsed: payload_in_metadata, header_in_metadata,
	// failed_status_in_metadata, normalize_payload_in_metadata, jwt_cache_config,
	// subjects, require_expiration, max_lifetime.

	// Parse JWKS source oneof. Task 3 lands RemoteJwks via internal/jwks per
	// ADR-0150; Task 4 lands LocalJwks via internal/jwt per ADR-0151.
	switch src := p.GetJwksSourceSpecifier().(type) {
	case *jwt_authnv3.JwtProvider_RemoteJwks:
		if src.RemoteJwks.GetHttpUri() == nil {
			return nil, errors.New("remote_jwks.http_uri is required")
		}
		if src.RemoteJwks.GetHttpUri().GetUri() == "" {
			return nil, errors.New("remote_jwks.http_uri.uri is required")
		}
		// Translate proto → jwks types. The task 7 provider.go refactor MAY
		// migrate these helpers; at Task 3 they live inline here.
		//
		// Per ADR-0150 §Decision AMENDMENT (phase-20 IMPL Task 2b): the
		// shared `*httpclient.Client` framework primitive is threaded as
		// the 5th constructor argument. Nil-tolerant per the AMENDMENT —
		// nil lets the Fetcher allocate its own per-Fetcher default
		// preserving the phase-17-pinned 30-second per-request timeout.
		fetcher, err := jwks.New(
			src.RemoteJwks.GetHttpUri().GetUri(),
			cacheDurationFromProto(src.RemoteJwks.GetCacheDuration()),
			asyncFetchFromProto(src.RemoteJwks.GetAsyncFetch()),
			retryPolicyFromProto(src.RemoteJwks.GetRetryPolicy()),
			httpClient,
		)
		if err != nil {
			return nil, fmt.Errorf("remote_jwks fetcher init: %w", err)
		}
		cp.jwksFetcher = fetcher
	case *jwt_authnv3.JwtProvider_LocalJwks:
		// LocalJwks reads inline JWK Set JSON via the envoy.config.core.v3
		// DataSource oneof. Per ADR-0151 §Decision (vii) the JWK Set parser
		// lives at `internal/jwks/ParseJWKSet` (sibling to `internal/jwt/`;
		// the PLAN's File-structure table hedged on package placement —
		// final landing is `internal/jwks/` per Task 3 + ADR-0150 §Decision
		// (vii)). Task 7's provider.go MAY relocate readDataSource to a
		// dedicated helper module.
		if src.LocalJwks == nil {
			return nil, errors.New("local_jwks is required")
		}
		raw, err := readDataSource(src.LocalJwks)
		if err != nil {
			return nil, fmt.Errorf("local_jwks: %w", err)
		}
		set, err := jwks.ParseJWKSet(raw)
		if err != nil {
			return nil, fmt.Errorf("local_jwks parse: %w", err)
		}
		cp.localJwks = set
	default:
		// Neither set: provider has no JWKS source — PARSE-REJECT per §11.P9
		// envoy-go-side defensive PGV-mirror + JwksSourceSpecifier
		// structurally-required (the oneof carries no PGV `required` tag in
		// the .proto, but operational correctness demands a JWKS source).
		return nil, errors.New("either remote_jwks or local_jwks must be set")
	}
	return cp, nil
}

// Per-provider data-source + retry-policy + cache-duration + async-fetch
// translation helpers — RELOCATED to provider.go at Task 7 per PLAN's
// File-structure table thematic-cohesion convention. Callers in this file
// (buildCompiledProvider's RemoteJwks + LocalJwks branches) reach across the
// in-package boundary.

// buildCompiledRequirement recursively parses a JwtRequirement evaluator tree
// per §6.5 + §11.P16 + ADR-0149. 6-variant switch on RequiresType:
//
//   - *JwtRequirement_ProviderName → lookup provider; missing → error.
//   - *JwtRequirement_ProviderAndAudiences → lookup provider + capture
//     per-rule Audiences override.
//   - *JwtRequirement_RequiresAny → recurse on each child; assemble children.
//   - *JwtRequirement_RequiresAll → same shape; kind = reqRequiresAll.
//   - *JwtRequirement_AllowMissing → terminal; kind = reqAllowMissing.
//   - *JwtRequirement_AllowMissingOrFailed → terminal; kind =
//     reqAllowMissingOrFailed.
//
// nil requirement (proto unset) → reqAllowMissingOrFailed per the
// RequirementRule.requires proto comment ("If this field is empty, it means
// JWT authentication is optional"). Mirrors Envoy filter_config.cc.
func buildCompiledRequirement(r *jwt_authnv3.JwtRequirement, providers map[string]*compiledProvider) (*compiledRequirement, error) {
	if r == nil || r.GetRequiresType() == nil {
		return &compiledRequirement{kind: reqAllowMissingOrFailed}, nil
	}
	switch t := r.GetRequiresType().(type) {
	case *jwt_authnv3.JwtRequirement_ProviderName:
		p, ok := providers[t.ProviderName]
		if !ok {
			return nil, fmt.Errorf("provider_name %q not in providers map", t.ProviderName)
		}
		return &compiledRequirement{kind: reqProviderName, provider: p}, nil
	case *jwt_authnv3.JwtRequirement_ProviderAndAudiences:
		p, ok := providers[t.ProviderAndAudiences.GetProviderName()]
		if !ok {
			return nil, fmt.Errorf("provider_and_audiences.provider_name %q not in providers map", t.ProviderAndAudiences.GetProviderName())
		}
		return &compiledRequirement{
			kind:     reqProviderAndAudiences,
			provider: p,
			audOverr: t.ProviderAndAudiences.GetAudiences(),
		}, nil
	case *jwt_authnv3.JwtRequirement_RequiresAny:
		cr := &compiledRequirement{kind: reqRequiresAny}
		for i, sub := range t.RequiresAny.GetRequirements() {
			sc, err := buildCompiledRequirement(sub, providers)
			if err != nil {
				return nil, fmt.Errorf("requires_any[%d]: %w", i, err)
			}
			cr.children = append(cr.children, sc)
		}
		return cr, nil
	case *jwt_authnv3.JwtRequirement_RequiresAll:
		cr := &compiledRequirement{kind: reqRequiresAll}
		for i, sub := range t.RequiresAll.GetRequirements() {
			sc, err := buildCompiledRequirement(sub, providers)
			if err != nil {
				return nil, fmt.Errorf("requires_all[%d]: %w", i, err)
			}
			cr.children = append(cr.children, sc)
		}
		return cr, nil
	case *jwt_authnv3.JwtRequirement_AllowMissing:
		return &compiledRequirement{kind: reqAllowMissing}, nil
	case *jwt_authnv3.JwtRequirement_AllowMissingOrFailed:
		return &compiledRequirement{kind: reqAllowMissingOrFailed}, nil
	default:
		return nil, fmt.Errorf("unknown requires_type %T", t)
	}
}

// buildCompiledRule parses one RequirementRule per §6.5 + §11.P12 + ADR-0149.
// Defensive PGV-mirror: match REQUIRED. Requirement-type oneof:
//
//   - *RequirementRule_Requires → inline requirement; recursive
//     buildCompiledRequirement.
//   - *RequirementRule_RequirementName → reqMap lookup; LISTENER-LEVEL
//     dangling name → PARSE-REJECT per §11.P12 split-semantic ("listener-
//     level is parse-reject; per-route is runtime-resolve").
//   - nil → reqAllowMissingOrFailed per proto comment ("If not specified,
//     Jwt verification is disabled").
//
// At Task 2 the matcher field is a placeholder (the existing router package
// does not yet expose a NewRouteMatcher constructor; the match-required
// PGV-mirror is enforced but the compilation is Task 6's deliverable). The
// stored proto-level RouteMatch carries enough state for the eventual
// matcher compilation.
func buildCompiledRule(r *jwt_authnv3.RequirementRule, providers map[string]*compiledProvider, reqMap map[string]*compiledRequirement) (*compiledRule, error) {
	if r.GetMatch() == nil {
		return nil, errors.New("match is required")
	}
	var req *compiledRequirement
	var err error
	switch t := r.GetRequirementType().(type) {
	case *jwt_authnv3.RequirementRule_Requires:
		req, err = buildCompiledRequirement(t.Requires, providers)
		if err != nil {
			return nil, fmt.Errorf("requires: %w", err)
		}
	case *jwt_authnv3.RequirementRule_RequirementName:
		r2, ok := reqMap[t.RequirementName]
		if !ok {
			// Per §11.P12 split-semantic: listener-level dangling name
			// PARSE-REJECTS. (Per-route case uses runtime-resolve per
			// ADR-0153; landed at Task 7.)
			return nil, fmt.Errorf("requirement_name %q not in requirement_map", t.RequirementName)
		}
		req = r2
	case nil:
		// No requirement set — proto comment: "If not specified, Jwt
		// verification is disabled."
		req = &compiledRequirement{kind: reqAllowMissingOrFailed}
	default:
		return nil, fmt.Errorf("unknown requirement_type %T", t)
	}
	matchFn, err := compileRouteMatch(r.GetMatch())
	if err != nil {
		return nil, fmt.Errorf("match: %w", err)
	}
	return &compiledRule{matchFn: matchFn, requirement: req}, nil
}

// compileRouteMatch translates the Envoy v3 RouteMatch proto's path_specifier
// oneof into a path-predicate closure. Path-specifier coverage mirrors
// phase-04's internal/filter/hcm/config.go buildMatch: PATH (exact) + PREFIX.
// SafeRegex / PathSeparatedPrefix / ConnectMatcher remain DEFERRED per §11.P9
// + ADR-0149's matcher-extraction deferral; headers / query_parameters /
// runtime_fraction / dynamic_metadata / tls_context predicates are NOT
// supported (rejected with a descriptive error so fixture-driven configs that
// rely on these surfaces fail loud rather than silently bypass the filter).
func compileRouteMatch(m *routev3.RouteMatch) (func(string) bool, error) {
	if m == nil {
		return nil, errors.New("match is missing")
	}
	if m.GetCaseSensitive() != nil && !m.GetCaseSensitive().GetValue() {
		return nil, errors.New("match.case_sensitive=false is not supported (phase 17 jwt_authn)")
	}
	if len(m.GetHeaders()) > 0 {
		return nil, errors.New("match.headers is not supported (phase 17 jwt_authn)")
	}
	if len(m.GetQueryParameters()) > 0 {
		return nil, errors.New("match.query_parameters is not supported (phase 17 jwt_authn)")
	}
	if m.GetRuntimeFraction() != nil {
		return nil, errors.New("match.runtime_fraction is not supported (phase 17 jwt_authn)")
	}
	if len(m.GetDynamicMetadata()) > 0 {
		return nil, errors.New("match.dynamic_metadata is not supported (phase 17 jwt_authn)")
	}
	if m.GetTlsContext() != nil {
		return nil, errors.New("match.tls_context is not supported (phase 17 jwt_authn)")
	}
	switch ps := m.GetPathSpecifier().(type) {
	case *routev3.RouteMatch_Path:
		if ps.Path == "" {
			return nil, errors.New("match.path is empty")
		}
		want := ps.Path
		return func(p string) bool { return p == want }, nil
	case *routev3.RouteMatch_Prefix:
		if ps.Prefix == "" {
			return nil, errors.New("match.prefix is empty")
		}
		want := ps.Prefix
		return func(p string) bool { return strings.HasPrefix(p, want) }, nil
	case *routev3.RouteMatch_SafeRegex:
		return nil, errors.New("match.safe_regex is not supported (phase 17 jwt_authn)")
	case *routev3.RouteMatch_PathSeparatedPrefix:
		return nil, errors.New("match.path_separated_prefix is not supported (phase 17 jwt_authn)")
	case *routev3.RouteMatch_ConnectMatcher_:
		return nil, errors.New("match.connect_matcher is not supported (phase 17 jwt_authn)")
	default:
		return nil, fmt.Errorf("match.path_specifier is missing or of unsupported type %T", ps)
	}
}

// parsePerRoute + buildCompiledPerRoute + resolvePerRouteConfig — RELOCATED
// to provider.go at Task 7 per PLAN's File-structure table thematic-cohesion
// convention + ADR-0153. The 8th canonical per-route surface lives wholly in
// provider.go.

// newFilterStats registers the 7 base counters at the HCM stat_prefix scope
// per ADR-0154. ALL 7 counters registered unconditionally (NO lazy-allocation
// per ADR-0148 + ADR-0154 SHARED-stats discipline; predeclared empty counters
// for scrape stability per Prometheus best practice).
//
// Internal stat path per §11.P7 (RATIFIED-PENDING-IMPL-TIME-EMPIRICAL-SCRAPE
// at Task 8 per ADR-0154 + planner-time decision 10):
//
//	http.<HCM_stat_prefix>.jwt_authn.<counter>
//
// Empty HCM stat_prefix (test code paths) folds to the non-HCM-rooted shape
// `jwt_authn.<counter>` to satisfy the Registry's name regex which forbids
// leading dots / double dots. The empirical scrape closure at Task 8 may
// REFINE this shape (e.g., if the reference Envoy scrape reveals a different
// segment between `http.<HCM>.` and `.<counter>`).
//
// **Caller MUST guard against nil registry**: this helper unconditionally
// dereferences reg via reg.NewCounter. The nil-tolerance contract lives at
// the SINGLE caller (buildCompiledConfig's `if ctx.Stats != nil` gate) per
// ADR-0085.
//
// NewCounter (NOT NewCounterIfAbsent) is used at Task 2 since SHARED-stats
// discipline means each listener-level New() invocation creates its own
// fresh Registry-scoped counter set. If two listeners share an HCM
// stat_prefix (operational edge case), the Registry's NewCounter would panic
// on duplicate name; Task 8 may switch to NewCounterIfAbsent if the
// empirical-scrape closure reveals this is operator-reachable. Mirrors phase-
// 12 csrf newFilterStats precedent.
func newFilterStats(reg *stats.Registry, hcmStatPrefix string) *filterStats {
	prefix := baseStatPrefix(hcmStatPrefix)
	return &filterStats{
		allowed:               reg.NewCounter(prefix + "allowed"),
		denied:                reg.NewCounter(prefix + "denied"),
		corsPreflightBypassed: reg.NewCounter(prefix + "cors_preflight_bypassed"),
		jwksFetchSuccess:      reg.NewCounter(prefix + "jwks_fetch_success"),
		jwksFetchFailed:       reg.NewCounter(prefix + "jwks_fetch_failed"),
		jwtCacheHit:           reg.NewCounter(prefix + "jwt_cache_hit"),
		jwtCacheMiss:          reg.NewCounter(prefix + "jwt_cache_miss"),
	}
}

// baseStatPrefix returns the canonical base-prefix segment for the 7 base
// counters per ADR-0154 + §11.P7. The shape `http.<HCM>.jwt_authn.` is the
// HCM-rooted SN2-reuse form; empty HCM prefix folds to the bare `jwt_authn.`
// form (test code paths; production HCM always provides non-empty stat_prefix
// at FactoryCtx.StatPrefix wiring per ADR-0061).
//
// The Registry's nameRE forbids leading dots + double dots; the empty-HCM
// fallback is structural (NOT just a regex workaround — operators that don't
// configure HCM-rooting expect a flatter namespace anyway). Mirrors phase-16
// rbac/baseStatPrefix precedent.
//
// RATIFIED-PENDING closure at Task 8 per ADR-0154 — if the reference Envoy
// scrape reveals a different segment shape, this helper amends in place.
func baseStatPrefix(hcmStatPrefix string) string {
	if hcmStatPrefix == "" {
		return "jwt_authn."
	}
	return "http." + hcmStatPrefix + ".jwt_authn."
}

// durationOrDefault returns def when secs == 0, else secs as a time.Duration.
// Used for the clock_skew_seconds 60s default per amendment 7. `secs` is a
// uint32 matching the JwtProvider.clock_skew_seconds proto type
// (envoy.extensions.filters.http.jwt_authn.v3.JwtProvider field 10).
func durationOrDefault(secs uint32, def time.Duration) time.Duration {
	if secs == 0 {
		return def
	}
	return time.Duration(secs) * time.Second
}

// ---------------------------------------------------------------------------
// DecodeHeaders body per SPEC §6.6 + §1.1 amendment 12 + ADR-0155.
//
// Order:
//
//  1. Capture `:path` → f.originalURI for WWW-Authenticate realm (BEFORE any
//     route mutation; per §1.1 amendment 12).
//  2. Resolve per-route TPFC via f.dcb.RequestRouteConfig() → factoryState.
//     resolvePerRouteConfig. Case (a) disabled:true → passthrough +
//     HeaderContinue + NO counter increments per §5.3 + ADR-0153.
//  3. CORS preflight bypass per §11.P1: bypass_cors_preflight=true AND
//     isCorsPreflightRequest → cors_preflight_bypassed++ + HeaderContinue.
//  4. Resolve the requirement via f.resolveRequirement(headers). Three
//     outcomes per the helper's contract:
//     - (nil, true) per-route dangling-name miss → SendLocalReply(403) +
//     denied++ already fired; return HeaderStopIteration.
//     - (nil, false) no rule matched + no per-route requirement_name →
//     pass-through (no counter increments).
//     - (req, false) requirement applies → evaluate.
//  5. Evaluate the requirement via Task 6's evaluateRequirement.
//  6. Apply the result via applyResult (allowed++ + side-effects on
//     allow OR denied++ + emitDenyResponse on deny per SPEC §6.9).
//
// ---------------------------------------------------------------------------

// DecodeHeaders is the request-gate entry per ADR-0148 §Decision + SPEC §6.6.
// Composes the dispatch surface across Tasks 5/6/7's helpers and Task 9's
// applyResult emit-path.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Bind the listener-level config as the active config. Per ADR-0153
	// SHARED-stats discipline + ADR-0154, per-route does NOT spawn a fresh
	// *compiledConfig — listener-level is the SOLE config instance.
	f.activeRC = f.state.listenerRC

	// (1) Capture the full request URI → f.originalURI for WWW-Authenticate
	// realm per §1.1 amendment 12 + RFC 6750 §3 + Task-13 empirical reference
	// scrape. Reference Envoy emits the FULL request URL form
	// `<scheme>://<:authority><:path>` (scheme inferred from x-forwarded-proto
	// when present, default "http"). Done BEFORE any route mutation so the
	// realm reflects the ORIGINAL request URI (operator-subtle: header-
	// mutation filters upstream of jwt_authn rewriting :path would make the
	// realm reflect the mutated path; documented at ADR-0155 §Context).
	if path := headers.Get(":path"); path != "" {
		authority := headers.Get(":authority")
		if authority == "" {
			authority = headers.Get("host")
		}
		scheme := headers.Get("x-forwarded-proto")
		if scheme == "" {
			scheme = headers.Get(":scheme")
		}
		if scheme == "" {
			scheme = "http"
		}
		if authority != "" {
			f.originalURI = scheme + "://" + authority + path
		} else {
			// Fallback when :authority + Host are both absent (rare in
			// practice; defensive). Bare-path realm matches the older
			// ADR-0155 behavior + the jwksbackend-less unit tests.
			f.originalURI = path
		}
	}

	// (2) Resolve per-route TPFC. Nil-tolerant on f.dcb per ADR-0085.
	if f.dcb != nil {
		if cfgMsg := f.dcb.RequestRouteConfig(); cfgMsg != nil {
			// resolvePerRouteConfig returns (nil, nil) when the proto type
			// is unexpected; (nil, err) on per-route parse failure (log +
			// inherit-listener semantic per ADR-0153 + ADR-0145 precedent).
			if pr, err := f.state.resolvePerRouteConfig(cfgMsg); err == nil && pr != nil {
				f.perRoute = pr
			}
		}
	}

	// Case (a) per-route disabled:true → wholesale passthrough per §5.3 +
	// ADR-0153. NO counter increments per §11.P9.
	if f.perRoute != nil && f.perRoute.disabled {
		f.passthrough = true
		return envoyhttp.Continue
	}

	// (3) CORS preflight bypass per §11.P1 + §1.1 amendment 10.
	if f.activeRC != nil && f.activeRC.bypassCorsPreflight && isCorsPreflightRequest(headers) {
		if f.activeRC.stats != nil {
			f.activeRC.stats.corsPreflightBypassed.Inc()
		}
		f.passthrough = true
		return envoyhttp.Continue
	}

	// (4) Resolve the requirement.
	req, denied := f.resolveRequirement(headers)
	if denied {
		// resolveRequirement already emitted SendLocalReply(403) + denied++
		// for the per-route dangling-name case per ADR-0153 + §1.1 amendment 6.
		return envoyhttp.StopIteration
	}
	if req == nil {
		// No rule matched + no per-route requirement_name → pass-through.
		// NO counter increments (no requirement applies; no allow/deny event).
		f.passthrough = true
		return envoyhttp.Continue
	}

	// (5) Evaluate the requirement.
	r := f.evaluateRequirement(req, headers)

	// (6) Apply the result.
	return f.applyResult(r, headers)
}

// DecodeData is pass-through per ADR-0148 §Decision — jwt_authn evaluates at
// DecodeHeaders before body bytes flow. No per-stream body interaction.
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers is pass-through per ADR-0148 §Decision.
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// OnDestroy is a no-op per ADR-0148 §Decision — jwt_authn has no per-stream
// async resources to release. The listener-level *jwks.Fetcher (per ADR-0150
// + planner-time decision D9) is owned by compiledProvider; lifetime is
// listener-lifetime; not closed at filter OnDestroy.
func (f *filter) OnDestroy() {}
