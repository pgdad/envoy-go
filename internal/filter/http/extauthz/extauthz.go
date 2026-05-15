package extauthz

// extauthz.go — main filter file for envoy.filters.http.ext_authz.
//
// Public surface (per SPEC §6.1):
//   - TypeURL: canonical filter typed_config type URL.
//   - New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error):
//     the HTTPFilterFactory registered at boot per ADR-0072.
//
// See doc.go for the full package overview including the dual-mode service
// envelope, the 18.1-consumed field set, filter shape, per-route canonical,
// stat surface, async-resume discipline, and ADR anchors.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TypeURL is the canonical Envoy type-URL for the ext_authz HTTP filter config.
// Boot wiring at cmd/envoy-go/main.go (Task 10) registers New under this key
// in the HTTPRegistry per ADR-0072 + ADR-0156.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"

// filterName is the canonical http_filters[].name string for ext_authz
// (matches the listener config typed_per_filter_config map keys; underscore
// preserved per the filter's proto canonical name).
const filterName = "envoy.filters.http.ext_authz"

// errHTTPCheckFnStub was removed at Task 3 review-fix — buildHTTPCheckFn is now
// real (see check.go); the sentinel error allocation is no longer needed.
// See commit 21f0ac0 for the original Task 3 landing.

// ---------------------------------------------------------------------------
// Core types per SPEC §6.2 + ADR-0156 + ADR-0157.
// ---------------------------------------------------------------------------

// checkFn is the mode-agnostic outbound auth-check closure. In 18.1 the only
// implementation is the HTTP-mode closure (Task 3). 18.2 adds the gRPC-mode
// closure. The function receives a per-request cancellable context.
type checkFn func(ctx context.Context, req *authRequest) (checkDisposition, error)

// authRequest carries the request-side-filtered inputs for the outbound auth
// check POST (method, path-with-path_prefix, filtered headers, body).
// Full construction lands at Task 4 (AuthorizationRequest builder).
type authRequest struct {
	method  string
	path    string
	headers http.Header
	body    []byte
}

// dispositionClass is the mode-agnostic three-way disposition per SPEC §6.2
// + parent SPEC §5.P10.
type dispositionClass int

const (
	// dispAllow — the auth service allowed the request (HTTP 200 or gRPC OK).
	dispAllow dispositionClass = iota
	// dispDeny — the auth service denied the request (recognized deny status).
	dispDeny
	// dispError — transport/timeout/unrecognized status; failure_mode_allow applies.
	dispError
	// dispInvalid — validate_mutations rejected a header name/value in the
	// allow-path upstream or deny-path client header set. The invalid counter
	// increments; the error posture applies per SPEC §6.3 + ADR-0161.
	dispInvalid
)

// headerKV is a simple name/value header pair used in checkDisposition.
type headerKV struct {
	name  string
	value string
}

// checkDisposition is the mode-agnostic convergence value from a completed
// auth check per SPEC §6.2 + parent SPEC §4.1. Both the HTTP-mode and gRPC-mode
// (18.2) checkFn implementations produce this struct.
type checkDisposition struct {
	class dispositionClass // allow | deny | error

	// Allow-path: headers to inject / append on the upstream request.
	upstreamSet []headerKV // allowed_upstream_headers (overwrite/set)
	upstreamApp []headerKV // allowed_upstream_headers_to_append (append)

	// Deny-path: from the auth service's response.
	denyStatus  uint32     // HTTP status from the auth response
	denyBody    []byte     // verbatim auth response body
	denyHeaders []headerKV // allowed_client_headers-filtered auth response headers
}

// bufferSettings carries the parsed with_request_body / per-route override
// body-buffering config per SPEC §6.2 + ADR-0162.
type bufferSettings struct {
	maxRequestBytes     uint32
	allowPartialMessage bool
	packAsBytes         bool // gRPC-mode-only; parsed but no HTTP-mode effect in 18.1
}

// compiledConfig is the immutable post-parse listener-level config per SPEC §6.2
// + ADR-0157. Mode-agnostic and field-final at 18.1 — 18.2 adds no field (only
// supplies a second checkFn constructor). No transport-specific state. Allocated
// once at New() time; read-only shared after that per ADR-0101.
type compiledConfig struct {
	// checkFn is the resolved transport closure (HTTP-mode in 18.1).
	checkFn checkFn

	// withRequestBody is non-nil when with_request_body is set (validated
	// max_request_bytes > 0).
	withRequestBody *bufferSettings

	// Error-posture fields per SPEC §6.2 + ADR-0157.
	failureModeAllow          bool   // default false; allow through on error
	failureModeAllowHeaderAdd bool   // add x-envoy-auth-failure-mode-allowed header when true
	clearRouteCache           bool   // invoke cb.ClearRouteCache() on allow
	statusOnError             uint32 // default 403; HTTP status on error-path local reply
	validateMutations         bool   // gate header-name/value safety validation on mutations

	// Header-filter matchers (compiled at parse time; nil = all/none).
	// Full compilation at Task 4 via compileStringMatcherList.
	allowedHeaders    *stringMatcherList // top-level request-side allow-list; nil = all allowed
	disallowedHeaders *stringMatcherList // top-level request-side deny-list; nil = none denied

	// deprecatedAllowedHeaders is the pre-compiled AuthorizationRequest.allowed_headers
	// deprecated field (ADR-0160 D6 path). Compiled ONCE at buildCompiledConfig time
	// (Task 5 carried-forward fix — previously compiled per-request in buildAuthRequest).
	// Non-nil only when the deprecated field is present AND cc.allowedHeaders is nil
	// (top-level allowed_headers takes precedence when set). A malformed deprecated field
	// surfaces as a PARSE-REJECT at config-load time (not silent-degrade at runtime).
	deprecatedAllowedHeaders *stringMatcherList

	// stats is SHARED across listener + all per-route configs (SHARED-stats
	// discipline per ADR-0163; mirrors phase-12/13/14/17). nil when
	// ctx.Stats is nil (test path per ADR-0085 nil-tolerance).
	stats *filterStats
}

// stringMatcherList is the compiled form of a ListStringMatcher proto.
// The full type definition lives in attributes.go (Task 4), where it is
// defined alongside compileStringMatcherList + matchAny per the rbac/evaluator.go
// precedent of co-locating a compiled type with its constructor + methods.
// The compiledConfig fields reference *stringMatcherList by pointer (Go allows
// forward references within a package).
//
// This comment replaces the Task 2 placeholder type declaration;
// the actual type definition is in attributes.go.

// compiledPerRoute is the cached per-route disposition. Keyed by
// *ExtAuthzPerRoute proto pointer-identity in the factoryState.perRoute
// sync.Map per ADR-0117 + ADR-0125 §(v).
//
// Fields:
//   - cc: the effective listener-level compiledConfig (always the
//     listenerRC; SHARED-stats means no per-route *compiledConfig is
//     allocated — per ADR-0163).
//   - disabled: true when the disabled:true arm is active.
//   - checkSettings: nil for the disabled arm; non-nil for the
//     check_settings arm (narrower per-route override).
type compiledPerRoute struct {
	cc            *compiledConfig
	disabled      bool
	checkSettings *compiledCheckSettings
}

// compiledCheckSettings is the parsed check_settings arm of ExtAuthzPerRoute.
// Per SPEC §6.6 + ADR-0163: carries context_extensions (no HTTP-mode effect
// in 18.1), disable_request_body_buffering XOR with_request_body.
type compiledCheckSettings struct {
	contextExtensions           map[string]string // gRPC-mode-only in 18.1; parsed, no HTTP effect
	disableRequestBodyBuffering bool
	withRequestBody             *bufferSettings // per-route body-buffering override; nil if unset
}

// filterStats is the 6-counter set per parent SPEC §6 amendment 8 + ADR-0156.
// ALL 6 counters allocated unconditionally at New() time (predeclared empty
// counters for scrape stability — operators get a consistent counter surface).
// Namespace shape per SN2-reuse: `http.<HCM_stat_prefix>.ext_authz.<counter>`.
//
//   - ok: request allowed by the auth service.
//   - denied: request denied by the auth service.
//   - errored: transport/timeout/unrecognized status error. Named "errored"
//     to avoid the Go `error` keyword.
//   - disabled: runtime `filter_enabled` gate counter. STRUCTURALLY UNREACHABLE
//     under MVP per parent SPEC §6 amendment 7 — `filter_enabled` is silent-
//     ignored in 18.1; this counter publishes 0 for the listener's lifetime.
//     Registered for scrape-stability.
//   - failureModeAllowed: error disposition + failure_mode_allow:true. Both
//     errored AND failureModeAllowed increment.
//   - invalid: validate_mutations rejection.
type filterStats struct {
	ok                 *stats.Counter
	denied             *stats.Counter
	errored            *stats.Counter // "error" — Go keyword avoidance
	disabled           *stats.Counter // STRUCTURALLY UNREACHABLE under MVP (parent §6 amendment 7)
	failureModeAllowed *stats.Counter
	invalid            *stats.Counter
}

// factoryState is the closure-captured shared state per factory invocation.
// SIMPLIFIED relative to phase-11/15/16 (which carried hcmStatPrefix for
// INDEPENDENT-stats): phase-18.1 SHARED-stats discipline per ADR-0163 —
// no per-route *filterStats allocation.
type factoryState struct {
	listenerRC *compiledConfig
	// perRoute caches parsed *compiledPerRoute values keyed by
	// *ext_authzv3.ExtAuthzPerRoute pointer-identity per ADR-0117 +
	// ADR-0125 §(v). Race-safe via sync.Map.LoadOrStore.
	perRoute sync.Map // map[*ext_authzv3.ExtAuthzPerRoute]*compiledPerRoute
}

// filter is the per-stream filter instance allocated by the factory closure.
// Decoder-only per ADR-0156 §Decision; no encode-side state. Mirrors
// phase-12 csrf + phase-13 buffer + phase-16 rbac + phase-17 jwt_authn
// decoder-only precedent.
//
// The async-resume guard fields (mu/done/callCtx/callCancel) are declared
// here per planner-time decision D4; the real async-resume body lands at
// Task 9 per ADR-0159.
type filter struct {
	state *factoryState
	dcb   envoyhttp.DecoderFilterCallbacks

	// Per-stream state cached at DecodeHeaders.
	activeRC     *compiledConfig   // resolved listener-level config (always listenerRC at 18.1)
	perRoute     *compiledPerRoute // resolved per-route config; nil if no per-route TPFC
	awaitingBody bool              // true when body-buffering wait is in progress (ADR-0128)

	// bodySettings caches the effective bufferSettings for this stream (set in
	// DecodeHeaders when awaitingBody=true; read in DecodeData). The effective
	// settings come from the per-route override OR the listener-level
	// withRequestBody, resolved once per stream in DecodeHeaders. Nil when
	// awaitingBody=false (not buffering this stream).
	bodySettings *bufferSettings

	// Per-request body accumulator (ADR-0128 decode-side body-buffering reuse;
	// wired at Task 6). Appended-to per DecodeData chunk; truncated to
	// bodySettings.maxRequestBytes when allow_partial_message=true + over-limit.
	body []byte

	// Async-resume outbound-call leg + per-request cancellable context per
	// planner-time decision D4 + SPEC §6.3. Full wiring at Task 9 (ADR-0159).
	callCtx    context.Context
	callCancel context.CancelFunc

	// mu + done guard the resume-after-OnDestroy race per planner-time decision
	// D4. OnDestroy sets done=true under mu + calls callCancel; the resume
	// goroutine checks done under mu before touching dcb. Full wiring at Task 9.
	mu   sync.Mutex
	done bool
}

// Compile-time assertion: *filter implements the decoder-only filter interface
// per ADR-0156 §Decision; mirrors phase-12 csrf + phase-13 buffer + phase-16
// rbac + phase-17 jwt_authn precedent.
var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)

// SetDecoderCallbacks stores the per-stream callbacks reference for later
// RequestRouteConfig + SendLocalReply + ContinueDecoding use. Per ADR-0156.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }

// ---------------------------------------------------------------------------
// New — HTTPFilterFactory per SPEC §6.1 + ADR-0072 + ADR-0156.
// ---------------------------------------------------------------------------

// New is the HTTPFilterFactory exposed at boot. Per ADR-0072 + ADR-0156:
//
//  1. tc must be non-nil.
//  2. Unmarshal tc → *ext_authzv3.ExtAuthz; return error on malformed Any.
//  3. Invoke buildCompiledConfig (parses + validates ExtAuthz proto).
//  4. Capture *factoryState{listenerRC} for per-route lazy-cache.
//  5. Return FilterInstanceFactory closure that allocates a fresh *filter per
//     stream. HTTPFilter value: Decoder: f, Encoder: nil (decoder-only per
//     ADR-0156).
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("ext_authz: typed_config required")
	}
	var c ext_authzv3.ExtAuthz
	if err := tc.UnmarshalTo(&c); err != nil {
		return nil, fmt.Errorf("ext_authz: unmarshal: %w", err)
	}
	rc, err := buildCompiledConfig(ctx, &c)
	if err != nil {
		return nil, err
	}
	state := &factoryState{listenerRC: rc}
	return func() envoyhttp.HTTPFilter {
		f := &filter{state: state}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: nil, // decoder-only per ADR-0156 §Decision
		}
	}, nil
}

// ---------------------------------------------------------------------------
// buildCompiledConfig — parse + validate ExtAuthz proto per SPEC §6.4 + ADR-0157.
// ---------------------------------------------------------------------------

// buildCompiledConfig parses + validates an ext_authzv3.ExtAuthz envelope per
// SPEC §6.4 + ADR-0157. Parse order:
//
//  1. services oneof presence check: nil → PARSE-REJECT; grpc_service → PARSE-REJECT.
//  2. transport_api_version: non-V3 → PARSE-REJECT per ADR-0008.
//  3. with_request_body: if set, validate max_request_bytes > 0 (PGV-mirror);
//     build *bufferSettings.
//  4. error-posture fields (failure_mode_allow/failure_mode_allow_header_add/
//     clear_route_cache/validate_mutations) — parsed before http_service build
//     so validate_mutations can be threaded into buildHTTPCheckFn.
//  5. http_service arm → buildHTTPCheckFn (real impl Task 3; authorization_response
//     matchers + validateMutations closure-capture added at Task 5).
//     Pre-compile deprecated AuthorizationRequest.allowed_headers at parse time
//     (Task 5 carried-forward fix from Task 4 code review).
//  6. allowed_headers / disallowed_headers: compile ListStringMatcher (real
//     impl in attributes.go as of Task 4); null out deprecatedAllowedHeaders if
//     top-level allowedHeaders is set (top-level wins per D6 + ADR-0160).
//  7. allocate filterStats (guarded `if ctx.Stats != nil`) per ADR-0085.
//
// Note on ordering: transport_api_version + with_request_body validation
// precede the services-specific buildHTTPCheckFn call so that these rejections
// surface before the http-client construction. This gives operators a deterministic
// parse-error ordering.
func buildCompiledConfig(ctx envoyhttp.FactoryCtx, raw *ext_authzv3.ExtAuthz) (*compiledConfig, error) {
	cc := &compiledConfig{}

	// 1a. services oneof: nil → PARSE-REJECT; grpc_service → PARSE-REJECT.
	switch svc := raw.GetServices().(type) {
	case nil:
		return nil, errors.New("ext_authz: services oneof must be set")
	case *ext_authzv3.ExtAuthz_GrpcService:
		_ = svc
		return nil, errors.New("ext_authz: grpc_service mode not yet supported (lands in phase 18.2)")
	case *ext_authzv3.ExtAuthz_HttpService:
		// validated below after transport_api_version + with_request_body checks
	default:
		return nil, fmt.Errorf("ext_authz: unknown services oneof type %T", svc)
	}

	// 2. transport_api_version: V3-only PARSE-REJECT per ADR-0008.
	if raw.GetTransportApiVersion() != corev3.ApiVersion_V3 {
		return nil, fmt.Errorf("ext_authz: transport_api_version must be V3 (got %v); non-V3 rejected per ADR-0008",
			raw.GetTransportApiVersion())
	}

	// 3. with_request_body: validate max_request_bytes > 0 (PGV-mirror).
	if wb := raw.GetWithRequestBody(); wb != nil {
		if wb.GetMaxRequestBytes() == 0 {
			return nil, errors.New("ext_authz: with_request_body.max_request_bytes must be > 0")
		}
		cc.withRequestBody = &bufferSettings{
			maxRequestBytes:     wb.GetMaxRequestBytes(),
			allowPartialMessage: wb.GetAllowPartialMessage(),
			packAsBytes:         wb.GetPackAsBytes(),
		}
	}

	// 4. error-posture fields (parsed before http_service build so validateMutations
	//    can be threaded into buildHTTPCheckFn for closure capture).
	cc.failureModeAllow = raw.GetFailureModeAllow()
	cc.failureModeAllowHeaderAdd = raw.GetFailureModeAllowHeaderAdd()
	cc.clearRouteCache = raw.GetClearRouteCache()
	cc.validateMutations = raw.GetValidateMutations()

	// 5. http_service arm → buildHTTPCheckFn (real impl Task 3; extended Task 5
	//    to compile authorization_response matchers + accept validateMutations).
	httpSvc := raw.GetHttpService()
	fn, err := buildHTTPCheckFn(httpSvc, cc.validateMutations)
	if err != nil {
		return nil, err
	}
	cc.checkFn = fn

	// Task 5 carried-forward fix: pre-compile the deprecated
	// AuthorizationRequest.allowed_headers field at config-load time (was
	// per-request in buildAuthRequest Task 4). Only stored when the top-level
	// allowed_headers is nil (cc.allowedHeaders is parsed at step 6 below but
	// we need to compile the deprecated field regardless so malformed configs
	// surface at parse time). We resolve the cc.allowedHeaders precondition after
	// step 6 by nulling out deprecatedAllowedHeaders when allowedHeaders is set.
	//
	// Note: this step must run after the http_service validation above (so we have
	// a valid httpSvc) but before step 6 (cc.allowedHeaders is set there).
	if httpSvc != nil {
		if ar := httpSvc.GetAuthorizationRequest(); ar != nil {
			if deprecated := ar.GetAllowedHeaders(); deprecated != nil {
				compiled, compErr := compileStringMatcherList(deprecated)
				if compErr != nil {
					return nil, fmt.Errorf("ext_authz: authorization_request.allowed_headers (deprecated): %w", compErr)
				}
				cc.deprecatedAllowedHeaders = compiled
			}
		}
	}

	// status_on_error default 403 per SPEC §6.4.
	if soe := raw.GetStatusOnError(); soe != nil {
		cc.statusOnError = uint32(soe.GetCode())
	} else {
		cc.statusOnError = 403
	}

	// 6. allowed_headers / disallowed_headers: compile ListStringMatcher.
	// Real compilation via compileStringMatcherList (attributes.go, Task 4).
	// nil lsm → nil sml (no filtering); PARSE-REJECT on invalid pattern.
	ah, err := compileStringMatcherList(raw.GetAllowedHeaders())
	if err != nil {
		return nil, fmt.Errorf("ext_authz: allowed_headers: %w", err)
	}
	cc.allowedHeaders = ah

	dh, err := compileStringMatcherList(raw.GetDisallowedHeaders())
	if err != nil {
		return nil, fmt.Errorf("ext_authz: disallowed_headers: %w", err)
	}
	cc.disallowedHeaders = dh

	// Task 5 carried-forward fix (continued): if the top-level allowed_headers
	// is set (cc.allowedHeaders != nil), the deprecated field is superseded —
	// null it out so buildAuthRequest uses the top-level field exclusively.
	// If cc.allowedHeaders is nil, cc.deprecatedAllowedHeaders remains as the
	// effective allow-list (honored-if-present per D6 + ADR-0160).
	if cc.allowedHeaders != nil {
		cc.deprecatedAllowedHeaders = nil
	}

	// 7. filterStats — guarded `if ctx.Stats != nil` per ADR-0085 nil-tolerance.
	if ctx.Stats != nil {
		cc.stats = newFilterStats(ctx.Stats, ctx.StatPrefix)
	}

	return cc, nil
}

// buildHTTPCheckFn is defined in check.go (Task 3 real implementation).
// At Task 2 this was a stub in this file; at Task 3 the real implementation
// lives in check.go and this comment serves as the cross-reference.
// Signature: buildHTTPCheckFn(hs *ext_authzv3.HttpService, validateMutations bool) (checkFn, error)

// compileStringMatcherList is defined in attributes.go (Task 4 real implementation).
// It compiles a *matcherv3.ListStringMatcher into a *stringMatcherList with error
// return. Signature: compileStringMatcherList(lsm *matcherv3.ListStringMatcher) (*stringMatcherList, error).
// This comment serves as the cross-reference; the function body lives in attributes.go.

// ---------------------------------------------------------------------------
// Per-route helpers per SPEC §6.6 + ADR-0163.
// ---------------------------------------------------------------------------

// parsePerRoute parses one ExtAuthzPerRoute TPFC entry per SPEC §6.6 + §5 +
// ADR-0163. Envoy-go-side defensive PGV-mirror:
//   - override oneof is PGV-required → PARSE-REJECT when not set.
//   - disabled arm: const:true PGV constraint → PARSE-REJECT when disabled=false.
//   - check_settings arm: compile context_extensions + validate
//     disable_request_body_buffering XOR with_request_body.
//
// Returns the raw proto pointer for pointer-identity keying in resolvePerRouteConfig;
// also returns the *compiledPerRoute since we need it for the sync.Map.
func parsePerRoute(raw *ext_authzv3.ExtAuthzPerRoute) (*compiledPerRoute, error) {
	if raw == nil {
		return nil, errors.New("ext_authz: per-route: ExtAuthzPerRoute is nil")
	}

	// override oneof is PGV-required.
	if raw.GetOverride() == nil {
		return nil, errors.New("ext_authz: per-route: override oneof is required")
	}

	switch arm := raw.GetOverride().(type) {
	case *ext_authzv3.ExtAuthzPerRoute_Disabled:
		// PGV const:true — disabled must be true.
		if !arm.Disabled {
			return nil, errors.New("ext_authz: per-route: disabled must be true (PGV const:true violation; disabled:false is not meaningful)")
		}
		return &compiledPerRoute{
			cc:            nil, // filled at resolvePerRouteConfig time
			disabled:      true,
			checkSettings: nil,
		}, nil

	case *ext_authzv3.ExtAuthzPerRoute_CheckSettings:
		cs := arm.CheckSettings
		if cs == nil {
			return nil, errors.New("ext_authz: per-route: check_settings is required when arm is set")
		}
		// XOR: disable_request_body_buffering and with_request_body are mutually exclusive.
		if cs.GetDisableRequestBodyBuffering() && cs.GetWithRequestBody() != nil {
			return nil, errors.New("ext_authz: per-route: check_settings: disable_request_body_buffering and with_request_body are mutually exclusive")
		}
		ccs := &compiledCheckSettings{
			contextExtensions:           cs.GetContextExtensions(),
			disableRequestBodyBuffering: cs.GetDisableRequestBodyBuffering(),
		}
		if wb := cs.GetWithRequestBody(); wb != nil {
			ccs.withRequestBody = &bufferSettings{
				maxRequestBytes:     wb.GetMaxRequestBytes(),
				allowPartialMessage: wb.GetAllowPartialMessage(),
				packAsBytes:         wb.GetPackAsBytes(),
			}
		}
		return &compiledPerRoute{
			cc:            nil, // filled at resolvePerRouteConfig time
			disabled:      false,
			checkSettings: ccs,
		}, nil

	default:
		return nil, fmt.Errorf("ext_authz: per-route: unknown override type %T", arm)
	}
}

// resolvePerRouteConfig returns the effective *compiledPerRoute for the given
// TPFC message (which may be an *ext_authzv3.ExtAuthzPerRoute returned by
// dcb.RequestRouteConfig()). Per ADR-0117 + ADR-0125 §(v): keyed by proto
// pointer-identity via sync.Map lazy-cache.
//
//   - msg == nil: inherit listener-level config (no per-route TPFC).
//   - type assertion failure: inherit listener-level config (defensive).
//   - cache hit: return cached *compiledPerRoute.
//   - cache miss: parsePerRoute → LoadOrStore.
//     On parse error: log + return listener-level fallback (DO NOT cache;
//     mirrors phase-16 rbac resolvePerRouteConfig pattern).
//
// The returned *compiledPerRoute always has .cc set to the listener-level
// compiledConfig (SHARED-stats per ADR-0163 — no per-route cc instantiation).
func (s *factoryState) resolvePerRouteConfig(msg proto.Message) *compiledPerRoute {
	if msg == nil {
		return &compiledPerRoute{cc: s.listenerRC, disabled: false, checkSettings: nil}
	}

	// Type-assert to *ExtAuthzPerRoute; fall back to listener on mismatch.
	raw, ok := msg.(*ext_authzv3.ExtAuthzPerRoute)
	if !ok {
		return &compiledPerRoute{cc: s.listenerRC, disabled: false, checkSettings: nil}
	}

	// sync.Map lookup by pointer identity.
	if cached, ok := s.perRoute.Load(raw); ok {
		return cached.(*compiledPerRoute)
	}

	// Cache miss: parse and store.
	fresh, err := parsePerRoute(raw)
	if err != nil {
		// Per-route parse error: log + fall back to listener-level.
		// DO NOT cache the error sentinel (same pattern as phase-16 rbac).
		log.Printf("ext_authz: per-route resolve failed (inherit-listener): %v", err)
		return &compiledPerRoute{cc: s.listenerRC, disabled: false, checkSettings: nil}
	}
	// Wire the listener cc (SHARED-stats: no per-route cc).
	fresh.cc = s.listenerRC

	actual, _ := s.perRoute.LoadOrStore(raw, fresh)
	return actual.(*compiledPerRoute)
}

// ---------------------------------------------------------------------------
// newFilterStats — 6-counter registration per SPEC §6.2 + ADR-0156.
// ---------------------------------------------------------------------------

// newFilterStats registers the 6 base counters per ADR-0156 + parent SPEC §6
// amendment 8. All 6 registered unconditionally (predeclared empty counters
// for scrape stability per Prometheus best practice; mirrors phase-17 jwt_authn
// unconditional-allocation discipline).
//
// Internal stat path per SN2-reuse (RATIFIED-PENDING-IMPL-TIME at Task 8 via
// reference Envoy v1.37.2 empirical scrape per §18.P6):
//
//	http.<HCM_stat_prefix>.ext_authz.<counter>
//
// Empty HCM stat_prefix (test code paths) folds to the non-HCM-rooted shape
// `ext_authz.<counter>` to satisfy the Registry's nameRE which forbids leading
// dots / double dots.
//
// Uses NewCounterIfAbsent (rather than NewCounter) to avoid the multi-listener-
// same-prefix panic footgun per the rbac Task 8 lesson (CF-Task2-I3) —
// idempotent registration is safe and consistent with the existing discipline.
//
// **Caller MUST guard against nil registry**: this helper unconditionally
// dereferences reg. The nil-tolerance contract lives at buildCompiledConfig's
// `if ctx.Stats != nil` gate per ADR-0085.
func newFilterStats(reg *stats.Registry, hcmStatPrefix string) *filterStats {
	prefix := baseStatPrefix(hcmStatPrefix)
	return &filterStats{
		ok:                 reg.NewCounterIfAbsent(prefix + "ok"),
		denied:             reg.NewCounterIfAbsent(prefix + "denied"),
		errored:            reg.NewCounterIfAbsent(prefix + "error"), // "error" on wire; "errored" in Go
		disabled:           reg.NewCounterIfAbsent(prefix + "disabled"),
		failureModeAllowed: reg.NewCounterIfAbsent(prefix + "failure_mode_allowed"),
		invalid:            reg.NewCounterIfAbsent(prefix + "invalid"),
	}
}

// baseStatPrefix returns the canonical base-prefix segment for the 6 base
// counters per ADR-0156 + §18.P7 SN2-reuse hypothesis. Shape:
//
//	http.<HCM_stat_prefix>.ext_authz.
//
// Empty HCM stat_prefix (test code paths) folds to the bare `ext_authz.` form
// to satisfy the Registry's nameRE (forbids leading dots / double dots).
// RATIFIED-PENDING-IMPL-TIME closure at Task 8 via the reference Envoy v1.37.2
// empirical scrape.
func baseStatPrefix(hcmStatPrefix string) string {
	if hcmStatPrefix == "" {
		return "ext_authz."
	}
	return "http." + hcmStatPrefix + ".ext_authz."
}

// ---------------------------------------------------------------------------
// applyUpstreamMutations — allow-path upstream header injection helper.
// Per SPEC §6.3 + ADR-0161. Called from the Task 9 DecodeHeaders dispatch
// on the allow path after the async outbound check resolves.
// ---------------------------------------------------------------------------

// applyUpstreamMutations applies the allow-path header mutations from a
// checkDisposition to the upstream request headers:
//
//  1. upstreamSet: for each headerKV, headers.Set(name, value) — overwrites any
//     existing header with the same canonical key (allowed_upstream_headers).
//  2. upstreamApp: for each headerKV, headers.Add(name, value) — appends to any
//     existing values for the same canonical key (allowed_upstream_headers_to_append).
//
// Set is applied before Append so that Append can stack on top of the overwritten
// value. headers is the incoming client request header map (from DecodeHeaders)
// which is the map forwarded upstream by the HCM chain after the filter returns.
//
// Only the allow path calls this helper (disp.class == dispAllow); the deny and
// error / invalid paths do NOT mutate the upstream headers.
func applyUpstreamMutations(headers http.Header, disp checkDisposition) {
	// 1. upstreamSet: overwrite (Set) per allowed_upstream_headers.
	for _, kv := range disp.upstreamSet {
		headers.Set(kv.name, kv.value)
	}
	// 2. upstreamApp: append (Add) per allowed_upstream_headers_to_append.
	for _, kv := range disp.upstreamApp {
		headers.Add(kv.name, kv.value)
	}
}

// ---------------------------------------------------------------------------
// DecodeHeaders / DecodeData / DecodeTrailers / OnDestroy.
// Per SPEC §6.3 + ADR-0156 + ADR-0162.
// Full async-dispatch body (outbound check + resume) lands at Task 9.
// Task 6 lands: body-buffering branch of DecodeHeaders + DecodeData
// accumulation + over-limit 413 edge case.
// ---------------------------------------------------------------------------

// effectiveWithRequestBody returns the effective *bufferSettings for this stream
// per SPEC §6.3 step 3 + SPEC §8 + ADR-0162:
//
//  1. If per-route check_settings.disable_request_body_buffering=true → nil (OFF).
//  2. If per-route check_settings.with_request_body is set → use per-route override.
//  3. Otherwise → use listener-level cc.withRequestBody (may be nil = OFF).
//
// Called once per stream from DecodeHeaders; result cached on f.bodySettings.
func (f *filter) effectiveWithRequestBody(pr *compiledPerRoute) *bufferSettings {
	if pr != nil && pr.checkSettings != nil {
		if pr.checkSettings.disableRequestBodyBuffering {
			return nil // per-route: body buffering OFF
		}
		if pr.checkSettings.withRequestBody != nil {
			return pr.checkSettings.withRequestBody // per-route: body buffering override
		}
	}
	// Listener-level fallback.
	if f.activeRC != nil {
		return f.activeRC.withRequestBody
	}
	return nil
}

// DecodeHeaders is the request-gate entry per ADR-0156 + ADR-0162.
//
// Task 6 lands the body-buffering branch of SPEC §6.3 steps 3–4:
//   - Resolves the effective withRequestBody (per-route override OR listener-level).
//   - If withRequestBody is set AND endStream=false: sets f.awaitingBody=true +
//     caches f.bodySettings for DecodeData to read; returns Continue.
//     IMPORTANT: Continue (NOT StopIteration) per ADR-0128 synchronous-HCM
//     dispatch constraint — returning StopIteration from DecodeHeaders would
//     deadlock (the body-read loop is the ContinueDecoding path; both run in
//     the same HCM dispatch goroutine per ADR-0076 + ADR-0128).
//
// Task 9 wires the full dispatch body: per-route resolve → disabled short-circuit
// → async outbound check → disposition application per SPEC §6.3.
//
// Task 9 buildAuthRequest call-site (Task 4 review-fix forward-pointer):
// before invoking f.activeRC.checkFn, Task 9 MUST call buildAuthRequest(f, hs,
// dcb.RequestHeaders(), f.body, path) to construct the request-side-filtered
// *authRequest, then pass that *authRequest into the checkFn closure. The
// closure (check.go) does NOT do the request-side header filtering itself —
// buildAuthRequest needs the per-stream f (f.activeRC) + the real request
// headers, which only exist here at DecodeHeaders, not at config-load time.
// See the PROGRESS.md Task 4 "Deviation from PLAN Step 4" note + ADR-0160
// §Decision (v)/(vii) + the call-site-boundary comment in check.go.
func (f *filter) DecodeHeaders(_ http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Task 9 wires the per-route resolve (step 1) + disabled short-circuit (step 2)
	// + async outbound check dispatch (steps 5–7). The body-buffering branch
	// (steps 3–4) lands here at Task 6.

	// Task 6: compute effective withRequestBody (SPEC §6.3 step 3).
	// At Task 9, the per-route resolve is done first (f.perRoute = resolvePerRouteConfig(...));
	// for now, resolve it directly from dcb.RequestRouteConfig() when dcb is set.
	var pr *compiledPerRoute
	if f.state != nil && f.dcb != nil {
		pr = f.state.resolvePerRouteConfig(f.dcb.RequestRouteConfig())
	}

	effectiveWRB := f.effectiveWithRequestBody(pr)

	// Task 6: body-buffering branch (SPEC §6.3 step 4).
	// If withRequestBody is set AND !endStream: set awaitingBody + cache bodySettings.
	// Return Continue (see ADR-0128 note above — NOT StopIteration).
	if effectiveWRB != nil && !endStream {
		f.awaitingBody = true
		f.bodySettings = effectiveWRB
	}

	// Task 9 wires the real dispatch body per SPEC §6.3 (incl. the
	// buildAuthRequest call documented above, and the StopIteration for the
	// async outbound check, and the resume-goroutine dispatch).
	return envoyhttp.Continue
}

// DecodeData implements the ADR-0128 decode-side body-buffering reuse per
// SPEC §6.3 + ADR-0162:
//
//  1. Pass-through when awaitingBody=false (DataContinue; ext_authz evaluates
//     at DecodeHeaders before body bytes flow in the non-body-buffering path).
//  2. Accumulate: f.body = append(f.body, data...).
//  3. Over-limit check (strict > per buffer filter ADR-0128 §11.2 precedent):
//     accumulated > maxRequestBytes:
//     - allow_partial_message=false → SendLocalReply(413, "Payload Too Large",
//     {Connection: close}) + DataStopIterationNoBuffer. Auth NOT called; NO
//     ext_authz counter increments (parent SPEC §6 amendment 6 + ADR-0162).
//     - allow_partial_message=true → truncate f.body to maxRequestBytes prefix;
//     fall through to endStream handling (the truncated prefix is used in the
//     auth request; Task 9 adds x-envoy-auth-partial-body: true to auth req).
//  4. endStream=true: body complete (within limit OR allow_partial prefix).
//     Park via DataStopIterationAndBuffer — the Task 9 seam:
//     // Task 9: build authRequest (buildAuthRequest(f, hs, headers, f.body,
//     //          path)), fire async outbound check via a goroutine, resume with
//     //          cb.ContinueDecoding() on check completion (per ADR-0159 async-
//     //          resume discipline + bandwidthlimit Task 4 precedent).
//  5. Non-terminal chunk within limit: DataContinue per ADR-0128 (envoy-go HCM
//     accumulates all body bytes in connection.go bodyBuf; DataStopIterationAndBuffer
//     is not needed mid-stream — see buffer filter §Decision (v) note).
//
// envoy-go HCM dispatch reads body chunks sequentially in the same goroutine that
// runs the chain (ADR-0076 + ADR-0128). DataStopIterationAndBuffer on endStream
// parks the chain so Task 9's ContinueDecoding-from-goroutine can resume it.
// DataContinue on mid-stream chunks is safe (framework bodyBuf holds bytes).
func (f *filter) DecodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	// Step 1: pass-through when not buffering.
	if !f.awaitingBody {
		return envoyhttp.DataContinue
	}

	// Step 2: accumulate.
	f.body = append(f.body, data...)

	// Step 3: over-limit check (strict > per ADR-0128 buffer filter §11.2 precedent).
	if f.bodySettings != nil && uint32(len(f.body)) > f.bodySettings.maxRequestBytes {
		if !f.bodySettings.allowPartialMessage {
			// allow_partial_message=false: emit 413 + discard.
			// Auth NOT called; NO ext_authz counter increments per parent SPEC
			// §6 amendment 6 + ADR-0162. The stream ends here.
			f.dcb.SendLocalReply(413, "Payload Too Large", envoyhttp.OrderedHeaders{
				{Name: "Connection", Value: "close"},
			})
			return envoyhttp.DataStopIterationNoBuffer
		}
		// allow_partial_message=true: truncate to max_request_bytes prefix.
		// The truncated prefix goes to the auth service; Task 9 injects
		// x-envoy-auth-partial-body: true into the auth request headers.
		f.body = f.body[:f.bodySettings.maxRequestBytes]
		// Fall through to step 4/5: endStream handling with truncated body.
	}

	// Step 4: terminal chunk (body complete OR truncated prefix assembled).
	// Park via DataStopIterationAndBuffer — the Task 9 seam for async check.
	if endStream {
		// Task 9: build the authRequest and fire the async outbound check:
		//   authReq := buildAuthRequest(f, hs, headers, f.body, path)
		//   go func() { disp, err := f.activeRC.checkFn(ctx, authReq); ... f.dcb.ContinueDecoding() }
		// Per ADR-0159 async-resume + bandwidthlimit Task 4 DataStopIterationAndBuffer precedent.
		return envoyhttp.DataStopIterationAndBuffer
	}

	// Step 5: non-terminal chunk within limit — DataContinue.
	// (framework bodyBuf holds the bytes; parking mid-stream is not needed.)
	return envoyhttp.DataContinue
}

// DecodeTrailers is pass-through per ADR-0156 §Decision.
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// OnDestroy is a Task 2 skeleton. Task 9 wires the full cancellation logic:
// sets done=true under mu, calls callCancel() to cancel the in-flight
// outbound call's context.Context, per planner-time decision D4 + SPEC §6.3.
func (f *filter) OnDestroy() {
	// Task 9 wires: mu.Lock(); f.done = true; mu.Unlock(); if f.callCancel != nil { f.callCancel() }
}
