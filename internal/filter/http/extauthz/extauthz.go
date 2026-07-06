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
	"net"
	"net/http"
	"sync"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
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

// authRequest carries the request-side inputs for the outbound auth check.
//
// HTTP-mode (18.1): the four base fields (method, path, headers, body) are the
// request-side-filtered inputs for the POST.
//
// gRPC-mode (18.2): the base fields PLUS the 10 extension fields (per ADR-0160
// gRPC-mode portion + ADR-0165) are the inputs for the *authv3.AttributeContext
// constructed by buildAttributeContext. The 10 extension fields are populated
// at DecodeHeaders time from the new DecoderFilterCallbacks accessors per
// ADR-0165 (Task 8 wires the seeding via dispatchOutboundCheck — Task 5 only
// declares the fields and authors the consumer buildAttributeContext).
//
// The 10 extension fields are appended unconditionally regardless of mode —
// HTTP-mode buildAuthRequest leaves them at zero values; gRPC-mode
// buildAttributeContext reads them. This keeps the struct mode-agnostic per
// the ADR-0157 §Decision invariant.
type authRequest struct {
	// HTTP-mode + gRPC-mode shared (18.1 fields).
	method  string
	path    string
	headers http.Header
	body    []byte

	// gRPC-mode extension fields (18.2 — D3 + ADR-0165 + ADR-0160 gRPC-mode portion).
	// Populated at DecodeHeaders time from the per-stream DecoderFilterCallbacks
	// accessors. Read by buildAttributeContext to populate the AttributeContext.
	//
	// remoteAddr is the downstream peer's *net.TCPAddr (or equivalent). Source of
	// AttributeContext.source.address.socket_address per parent §5.P3 + ADR-0144.
	remoteAddr net.Addr
	// localAddr is the downstream-facing listener's *net.TCPAddr. Source of
	// AttributeContext.destination.address.socket_address per §11.P4.
	localAddr net.Addr
	// tlsServerName is the downstream TLS ConnectionState.ServerName (SNI).
	// Source of AttributeContext.tls_session.sni when include_tls_session=true
	// AND non-empty (the gate per §11.P4 RATIFICATION).
	tlsServerName string
	// peerCertDER is the DER-encoded leaf cert of the downstream's CLIENT cert
	// (NOT the listener cert — that is listenerPrincipal). Source of
	// AttributeContext.source.certificate when include_peer_certificate=true
	// AND non-empty (the gate per parent §5.P3).
	peerCertDER []byte
	// listenerPrincipal is the listener server-cert's URI SAN[0] / DNS SAN[0] /
	// Subject CN, extracted at listener-build time per ADR-0165 §Decision +
	// extractListenerPrincipal. Source of AttributeContext.destination.principal
	// — populated AUTOMATICALLY per §11.P4 (NOT gated by include_peer_certificate).
	listenerPrincipal string
	// protocol is the downstream-connection protocol string ("HTTP/1.1" / "HTTP/2").
	// Source of AttributeContext.request.http.protocol per §11.P4.
	protocol string
	// requestID is the value of the x-request-id header (HCM-injected per §11.P4).
	// Source of AttributeContext.request.http.id. When absent, the IMPL leaves
	// the field empty (no generation — HCM is the authoritative source).
	requestID string
	// streamStartTime is the time the stream was received. Source of
	// AttributeContext.request.time. When zero, buildAttributeContext substitutes
	// time.Now() per SPEC §6.6 step 4.
	streamStartTime time.Time
	// perRouteContextExtensions is the merged per-route CheckSettings.context_extensions
	// map (resolved at DecodeHeaders time). Source of AttributeContext.context_extensions.
	// Empty for MVP-no-per-route per SPEC §5; Task 7 wires the merge.
	perRouteContextExtensions map[string]string
	// downstreamPrincipal is the ADR-0144 DownstreamPrincipal() result (joined-string
	// of URI SAN | DNS SAN | CN per ADR-0144). Source of AttributeContext.source.principal
	// — buildAttributeContext takes the first element (or "" if empty) per SPEC §6.6 step 1.
	downstreamPrincipal []string
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

// appendDispatch discriminates the four gRPC-mode `HeaderValueOption.append_action`
// enum arms per ADR-0161 gRPC-mode portion (D5). Phase-18.1 HTTP-mode predates
// this discriminator: all 18.1 `headerKV` values carry the zero value
// (`appendDispatchDefault`) and the slice they live in (`upstreamSet` vs
// `upstreamApp`) already encodes Set-vs-Add discipline. Phase-18.2 gRPC-mode
// maps the 4-arm `append_action` enum onto:
//
//   - `APPEND_IF_EXISTS_OR_ADD`   → `upstreamApp` with `appendDispatchDefault`
//     (Add semantics via `headers.Add`)
//   - `OVERWRITE_IF_EXISTS_OR_ADD` → `upstreamSet` with `appendDispatchDefault`
//     (Set semantics via `headers.Set` — 18.1 HTTP-mode default)
//   - `OVERWRITE_IF_EXISTS`        → `upstreamSet` with `appendDispatchOverwriteOnly`
//     (Set only when the header is already present; does NOT add)
//   - `ADD_IF_ABSENT`              → `upstreamSet` with `appendDispatchAddIfAbsent`
//     (Set only when the header is absent; does NOT overwrite)
//
// The discriminator lives on `headerKV` so `applyUpstreamMutations` can branch
// per-entry without splitting the slice into 4 separate fields. This keeps the
// 18.1 HTTP-mode call sites byte-identical (they emit `headerKV{name, value}`
// literals with the zero-value action — Set/Add discipline preserved by slice).
type appendDispatch int8

// appendDispatch enum values per ADR-0161 gRPC-mode portion (D5).
const (
	// appendDispatchDefault is the zero value — the slice container
	// (`upstreamSet` Set-discipline vs `upstreamApp` Add-discipline) determines
	// behavior. All 18.1 HTTP-mode `headerKV` values use this default. gRPC-mode
	// `APPEND_IF_EXISTS_OR_ADD` (→ `upstreamApp`) and `OVERWRITE_IF_EXISTS_OR_ADD`
	// (→ `upstreamSet`) also use this default.
	appendDispatchDefault appendDispatch = iota
	// appendDispatchOverwriteOnly maps gRPC-mode `OVERWRITE_IF_EXISTS` — Set only
	// when the header is already present in the upstream request map; does NOT
	// add when absent. Lives in `upstreamSet`.
	appendDispatchOverwriteOnly
	// appendDispatchAddIfAbsent maps gRPC-mode `ADD_IF_ABSENT` — Set only when
	// the header is absent from the upstream request map; does NOT overwrite
	// when present. Lives in `upstreamSet`.
	appendDispatchAddIfAbsent
)

// headerKV is a simple name/value header pair used in checkDisposition.
//
// `action` discriminates the gRPC-mode `HeaderValueOption.append_action`
// 4-arm dispatch per ADR-0161 gRPC-mode portion (D5). Default zero value
// (`appendDispatchDefault`) is honored by 18.1 HTTP-mode call sites — the
// slice container (`upstreamSet` vs `upstreamApp`) encodes Set-vs-Add. The
// non-default values (`appendDispatchOverwriteOnly` / `appendDispatchAddIfAbsent`)
// apply only when the entry lives in `upstreamSet` and signal Set-IF-PRESENT
// / Set-IF-ABSENT semantics respectively.
type headerKV struct {
	name   string
	value  string
	action appendDispatch // D5 dispatch per ADR-0161 gRPC-mode portion; zero value = default
}

// checkDisposition is the mode-agnostic convergence value from a completed
// auth check per SPEC §6.2 + parent SPEC §4.1. Both the HTTP-mode and gRPC-mode
// (18.2) checkFn implementations produce this struct.
type checkDisposition struct {
	class dispositionClass // allow | deny | error

	// Allow-path: headers to inject / append on the upstream request.
	upstreamSet []headerKV // allowed_upstream_headers (overwrite/set) + gRPC OkHttpResponse.headers (Set-discipline arms; D5)
	upstreamApp []headerKV // allowed_upstream_headers_to_append (append) + gRPC OkHttpResponse.headers (APPEND_IF_EXISTS_OR_ADD arm)
	// upstreamDel is the gRPC-mode `OkHttpResponse.headers_to_remove` list per
	// ADR-0161 gRPC-mode portion. Each entry is a (lowercased) header name to
	// be deleted from the upstream request headers via `headers.Del`. UNUSED
	// in 18.1 HTTP-mode (the HTTP `AuthorizationResponse` proto has no
	// equivalent field). Applied AFTER upstreamSet + upstreamApp in
	// `applyUpstreamMutations` so the delete is the final step (Set+Add then
	// Del = "remove unconditionally even if just set/added" — matches reference
	// Envoy's verbatim semantics).
	upstreamDel []string

	// Deny-path: from the auth service's response.
	denyStatus  uint32     // HTTP status from the auth response
	denyBody    []byte     // verbatim auth response body
	denyHeaders []headerKV // allowed_client_headers-filtered auth response headers (HTTP-mode); verbatim DeniedHttpResponse.headers (gRPC-mode per ADR-0161 §5.P11 — NO allowed_client_headers filter)
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

	// headersToAdd is the pre-compiled AuthorizationRequest.headers_to_add static
	// header list. Compiled ONCE at buildCompiledConfig time (Task 9 review fix —
	// previously read from hs at request time, requiring a non-nil hs argument to
	// buildAuthRequest; passing nil silently dropped these headers). Pre-compilation
	// makes buildAuthRequest hs-independent: no nil/empty argument needed.
	headersToAdd []headerKV

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

	// cachedHeaders caches the DecodeHeaders headers argument so the
	// body-complete path in DecodeData can pass the same header map reference
	// to dispatchOutboundCheck for allow-path upstream injection. Set at
	// DecodeHeaders time when body buffering is active (awaitingBody=true).
	cachedHeaders http.Header

	// Async-resume outbound-call leg + per-request cancellable context per
	// planner-time decision D4 + SPEC §6.3. Full wiring at Task 9 (ADR-0159).
	callCtx    context.Context
	callCancel context.CancelFunc

	// mu + done guard the resume-after-OnDestroy race per planner-time decision
	// D4. OnDestroy sets done=true under mu + calls callCancel; the resume
	// goroutine checks done under mu before touching dcb. Full wiring at Task 9.
	mu   sync.Mutex
	done bool

	// clearRouteCacheRequested tracks whether the allow path requested a
	// clear_route_cache() invocation. The actual DecoderFilterCallbacks
	// primitive cb.ClearRouteCache() is NOT yet exposed on the interface at
	// phase-18.1 MVP (deferred per ADR-0155 §Consequences, same as phase-17
	// jwt_authn). This flag is the test-introspection anchor; production sees a
	// no-op + forward-pointer to the future framework phase.
	clearRouteCacheRequested bool

	// streamStartTime is the wall-clock instant at which the stream's
	// DecodeHeaders entry fired — the canonical anchor for
	// AttributeContext.request.time per SPEC §11.P4 (request-arrival time, NOT
	// the auth-call time). Captured at DecodeHeaders entry per PLAN Task 8
	// §Step 3 (the recommended capture site — DecoderHeaders entry is the
	// closest envoy-go-observable proxy for the request-arrival instant; the
	// alternative dispatchOutboundCheck-time capture would systematically
	// understate request.time when body-buffering delays the dispatch). Seeded
	// onto the *authRequest in dispatchOutboundCheck so buildAttributeContext
	// (Task 5 — Group 12) marshals it into AttributeContext.request.time.
	//
	// gRPC-mode-only consumer at 18.2 MVP; HTTP-mode 18.1 ignores the field.
	streamStartTime time.Time
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

	// 1a. services oneof: nil → PARSE-REJECT; grpc_service → DISPATCH to
	// buildGRPCCheckFn (Task 3 WIRE-UP per ADR-0157 §Decision AMENDMENT —
	// activates the gRPC arm; buildGRPCCheckFn body is STUBBED at Task 3 and
	// lands at Task 5).
	switch svc := raw.GetServices().(type) {
	case nil:
		return nil, errors.New("ext_authz: services oneof must be set")
	case *ext_authzv3.ExtAuthz_GrpcService:
		// gRPC-mode arm — handled AFTER the structural checks below.
		_ = svc
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

	// 5. services oneof DISPATCH:
	//    - *ExtAuthz_HttpService → buildHTTPCheckFn (18.1 HTTP-mode closure).
	//    - *ExtAuthz_GrpcService → buildGRPCCheckFn (18.2 gRPC-mode closure;
	//      ADR-0157 §Decision AMENDMENT — Task 3 WIRE-UP with a STUB body that
	//      returns the "TODO (Task 5)" sentinel; Task 5/6 land the real body).
	//
	// Note: the services-oneof presence check fired at step 1a; here we
	// re-dispatch to pick the per-arm constructor. The 18.1 PARSE-REJECT for
	// the gRPC arm has been replaced by a real (stubbed) constructor call per
	// the ADR-0157 §Decision AMENDMENT.
	httpSvc := raw.GetHttpService()
	switch s := raw.GetServices().(type) {
	case *ext_authzv3.ExtAuthz_HttpService:
		fn, err := buildHTTPCheckFn(httpSvc, cc.validateMutations)
		if err != nil {
			return nil, err
		}
		cc.checkFn = fn
	case *ext_authzv3.ExtAuthz_GrpcService:
		fn, err := buildGRPCCheckFn(
			s.GrpcService,
			ctx,
			cc.validateMutations,
			raw.GetIncludePeerCertificate(),
			raw.GetIncludeTlsSession(),
			raw.GetEncodeRawHeaders(),
			packAsBytesFromWRB(cc.withRequestBody),
		)
		if err != nil {
			return nil, err
		}
		cc.checkFn = fn
	}

	// Pre-compile AuthorizationRequest fields from http_service at config-load time
	// so that buildAuthRequest does not need the *HttpService proto at request time.
	//
	// Task 5 carried-forward fix: pre-compile the deprecated allowed_headers field.
	// Only stored when the top-level allowed_headers is nil (cc.allowedHeaders is
	// parsed at step 6 below but we need to compile the deprecated field regardless
	// so malformed configs surface at parse time). We resolve the cc.allowedHeaders
	// precondition after step 6 by nulling out deprecatedAllowedHeaders when set.
	//
	// Task 9 review fix: pre-compile headers_to_add into cc.headersToAdd. The
	// previous approach passed hs to buildAuthRequest and guarded with hs != nil,
	// which silently dropped all headers_to_add when dispatchOutboundCheck passed
	// nil. Pre-compilation makes buildAuthRequest hs-independent.
	//
	// Note: this step must run after the http_service validation above (so we have
	// a valid httpSvc) but before step 6 (cc.allowedHeaders is set there).
	if httpSvc != nil {
		if ar := httpSvc.GetAuthorizationRequest(); ar != nil {
			if deprecated := ar.GetAllowedHeaders(); deprecated != nil { //nolint:staticcheck // intentional: deprecated field honored for backward-compat per ADR-0160
				compiled, compErr := compileStringMatcherList(deprecated)
				if compErr != nil {
					return nil, fmt.Errorf("ext_authz: authorization_request.allowed_headers (deprecated): %w", compErr)
				}
				cc.deprecatedAllowedHeaders = compiled
			}
			for _, hv := range ar.GetHeadersToAdd() {
				if hv.GetKey() == "" {
					continue
				}
				cc.headersToAdd = append(cc.headersToAdd, headerKV{
					name:  http.CanonicalHeaderKey(hv.GetKey()),
					value: hv.GetValue(),
				})
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
//  1. upstreamSet: per-entry dispatch on `headerKV.action` per ADR-0161
//     gRPC-mode portion (D5):
//     - appendDispatchDefault         → headers.Set(name, value) unconditionally
//     (overwrite-or-add; 18.1 HTTP-mode default + gRPC OVERWRITE_IF_EXISTS_OR_ADD).
//     - appendDispatchOverwriteOnly   → headers.Set(name, value) ONLY when the
//     header is already present (gRPC OVERWRITE_IF_EXISTS).
//     - appendDispatchAddIfAbsent     → headers.Set(name, value) ONLY when the
//     header is absent (gRPC ADD_IF_ABSENT).
//  2. upstreamApp: for each headerKV, headers.Add(name, value) — appends to any
//     existing values for the same canonical key (allowed_upstream_headers_to_append
//     + gRPC APPEND_IF_EXISTS_OR_ADD).
//  3. upstreamDel: for each name, headers.Del(name) — removes any existing
//     values (gRPC `OkHttpResponse.headers_to_remove` per ADR-0161 gRPC-mode
//     portion). Applied LAST so that a header named in both `upstreamSet`/
//     `upstreamApp` AND `upstreamDel` is deleted (matches reference Envoy's
//     verbatim "set-then-remove" semantics — the auth service's intent is to
//     remove the header regardless of what the client supplied).
//
// Set is applied before Append so that Append can stack on top of the overwritten
// value. headers is the incoming client request header map (from DecodeHeaders)
// which is the map forwarded upstream by the HCM chain after the filter returns.
//
// Only the allow path calls this helper (disp.class == dispAllow); the deny and
// error / invalid paths do NOT mutate the upstream headers.
func applyUpstreamMutations(headers http.Header, disp checkDisposition) {
	// 1. upstreamSet: Set-discipline with per-entry D5 dispatch.
	for _, kv := range disp.upstreamSet {
		switch kv.action {
		case appendDispatchOverwriteOnly:
			// OVERWRITE_IF_EXISTS — Set only when the header is already present.
			// We use http.Header.Values (the lowercased read accessor) to detect
			// presence regardless of the canonical-case form used in the map key.
			if len(headers.Values(kv.name)) > 0 {
				headers.Set(kv.name, kv.value)
			}
		case appendDispatchAddIfAbsent:
			// ADD_IF_ABSENT — Set only when the header is absent.
			if len(headers.Values(kv.name)) == 0 {
				headers.Set(kv.name, kv.value)
			}
		default:
			// appendDispatchDefault — unconditional Set (18.1 HTTP-mode default
			// + gRPC OVERWRITE_IF_EXISTS_OR_ADD).
			headers.Set(kv.name, kv.value)
		}
	}
	// 2. upstreamApp: append (Add) per allowed_upstream_headers_to_append /
	// gRPC APPEND_IF_EXISTS_OR_ADD.
	for _, kv := range disp.upstreamApp {
		headers.Add(kv.name, kv.value)
	}
	// 3. upstreamDel: delete (Del) per gRPC OkHttpResponse.headers_to_remove.
	// UNUSED in 18.1 HTTP-mode (HTTP `AuthorizationResponse` has no equivalent).
	for _, name := range disp.upstreamDel {
		headers.Del(name)
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
//
// Precondition: caller has already short-circuited on pr.disabled=true
// (per SPEC §6.3 step 2 — Task 9 wires this). When pr.disabled=true and
// pr.checkSettings=nil, this helper would fall through to the listener-level
// withRequestBody — but a disabled per-route entry must skip body buffering
// entirely. Task 9's DecodeHeaders short-circuit fires before this helper is
// called, so the implicit contract is: callers MUST NOT invoke this helper
// when pr.disabled=true.
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

// perRouteContextExtensionsFor returns the resolved per-route
// `CheckSettings.context_extensions` map for the current stream's per-route
// config. Returns nil when no per-route override applies OR when the per-route
// is in a non-CheckSettings arm (e.g., disabled:true). The proto3 default
// treats nil and empty maps equivalently on the wire — `AttributeContext.
// context_extensions` is an empty map in both cases per SPEC §6.6 step 8.
//
// MVP per ADR-0163's 5th-canonical-REUSE: `ExtAuthz` carries no top-level
// `context_extensions` field, and `core.GrpcService.initial_metadata` is
// DEFERRED per SPEC §2.6 + §8 item 2. The per-route map is therefore the
// ONLY source of context_extensions at 18.2 MVP. Per SPEC §5, per-route wins
// on key collisions (none at MVP since there is no listener-level baseline).
//
// SPEC §8 item 8 (the 18.1 forward-pointer that parsed context_extensions but
// NO-OPed in HTTP-mode) is CLOSED by this task: the parsed
// `compiledCheckSettings.contextExtensions` map now flows into the
// `*authRequest.perRouteContextExtensions` field at dispatchOutboundCheck
// seeding time, which `buildAttributeContext` reads into
// `AttributeContext.context_extensions` at gRPC-mode CheckRequest build time.
//
// Called from `dispatchOutboundCheck`; the return value seeds
// `req.perRouteContextExtensions` BEFORE the checkFn closure invocation.
func perRouteContextExtensionsFor(pr *compiledPerRoute) map[string]string {
	if pr == nil || pr.checkSettings == nil {
		return nil
	}
	return pr.checkSettings.contextExtensions
}

// dispatchOutboundCheck is the ONE source of truth for "build authRequest +
// launch goroutine + apply disposition under mu/done guard" per SPEC §6.3
// steps 5–11 + planner-time decision D4. Called from both DecodeHeaders (no-
// body path or body-complete via endStream=true) and DecodeData (body-complete
// path). Returns the correct FilterHeadersStatus/FilterDataStatus park signal
// to the HCM chain (always HeaderStopIteration / DataStopIterationAndBuffer).
//
// Design choice (Option B per SPEC §6.3 KEY DESIGN QUESTION): a shared helper
// so there is ONE source of truth for authRequest construction + goroutine +
// disposition logic. DecodeHeaders and DecodeData call it identically.
//
// Per Task 4 review-fix forward-pointer: buildAuthRequest(f, hs, headers, body,
// path) constructs the request-side-filtered authRequest; the checkFn closure
// faithfully transmits whatever it receives. The headers argument is the
// incoming client request header map (the same map DecodeHeaders receives, so
// allow-path mutations via applyUpstreamMutations land on the upstream headers
// the HCM chain forwards after the filter returns Continue).
//
// The per-request cancellable context is created here (D4): callCtx +
// callCancel are stored on f so OnDestroy can cancel them.
//
// ADR-0044 escape-valve NOT fired: the mu/done + cancellable-context guard
// sufficed (see OnDestroy + resume goroutine). ADR-0165 not authored.
func (f *filter) dispatchOutboundCheck(headers http.Header) {
	cc := f.activeRC

	// Guard: if checkFn is nil (test construction without a real factory), skip
	// the dispatch. This is a defensive nil-guard — production code always has
	// a non-nil checkFn (buildCompiledConfig wires it; New() guarantees it).
	// In test code that constructs a *compiledConfig directly without a checkFn,
	// the dispatch is a no-op (no outbound call, no resume).
	if cc.checkFn == nil {
		return
	}

	// Build the authRequest with the request-side-filtered headers + buffered body.
	// f.body is nil when with_request_body is not set (no buffering configured);
	// non-nil when body buffering has assembled the body via DecodeData.
	//
	// path is extracted from the :path pseudo-header (mirrors jwtauthn.go:759 /
	// rbac.go:951 pattern). An empty :path results in an empty path string, which
	// check.go's closure maps to baseURL + pathPrefix — the same behavior as no
	// path override.
	//
	// All config sourced from hs (headers_to_add, deprecated allowed_headers) is
	// pre-compiled into cc at buildCompiledConfig time (cc.headersToAdd,
	// cc.deprecatedAllowedHeaders). buildAuthRequest is fully hs-independent.
	path := headers.Get(":path")
	authReq := buildAuthRequest(f, headers, f.body, path)

	// Task 7: seed req.perRouteContextExtensions from the resolved per-route
	// CheckSettings.context_extensions map BEFORE the closure call. Mode-
	// agnostic field — HTTP-mode 18.1 leaves it nil (no consumer); gRPC-mode
	// 18.2 reads it via buildAttributeContext per SPEC §6.6 step 8 +
	// ADR-0163's 5th-canonical-REUSE. Per ADR-0163: no listener-level
	// baseline (ExtAuthz has no top-level context_extensions field), so the
	// per-route map IS the effective map.
	authReq.perRouteContextExtensions = perRouteContextExtensionsFor(f.perRoute)

	// Task 8: seed the remaining 9 extended *authRequest fields per SPEC §6.5
	// + the Task 4 callback-surface extension (ADR-0165). Sources:
	//   - 6 socket / TLS / protocol accessors from f.dcb (DecoderFilterCallbacks);
	//   - 1 listener-side principal from f.dcb.ListenerPrincipal();
	//   - the canonical x-request-id from the incoming client headers;
	//   - the request-arrival instant captured at DecodeHeaders entry.
	//
	// Mode-agnostic seeding: HTTP-mode 18.1 leaves the 10 extension fields at
	// zero values; gRPC-mode 18.2 reads them via buildAttributeContext per
	// SPEC §6.6 + §11.P4. The f.dcb nil-guard tolerates test setups that
	// construct a *filter without a callbacks reference (the body-only test
	// path) — in that case the 9 fields land at the zero values (effectively
	// equivalent to a plaintext / synthetic stream).
	if f.dcb != nil {
		authReq.remoteAddr = f.dcb.DownstreamRemoteAddr()
		authReq.localAddr = f.dcb.DownstreamLocalAddr()
		authReq.tlsServerName = f.dcb.DownstreamTLSServerName()
		authReq.peerCertDER = f.dcb.DownstreamTLSPeerCertDER()
		authReq.listenerPrincipal = f.dcb.ListenerPrincipal()
		authReq.downstreamPrincipal = f.dcb.DownstreamPrincipal() // ADR-0144 reuse
		authReq.protocol = f.dcb.DownstreamProtocol()
	}
	authReq.requestID = headers.Get("x-request-id")
	authReq.streamStartTime = f.streamStartTime

	// Create the per-request cancellable context (D4 + Task-2-M5 forward-pointer).
	callCtx, callCancel := context.WithCancel(context.Background())
	f.callCtx = callCtx
	f.callCancel = callCancel

	// Launch the async goroutine — mirrors phase-09 fault async-resume pattern:
	// StopIteration returned synchronously; goroutine performs the call;
	// cb.ContinueDecoding() (or SendLocalReply for deny) on completion.
	go func() {
		disp, err := cc.checkFn(callCtx, authReq)

		// Acquire mu before touching any callback or stream state (D4 guard).
		f.mu.Lock()
		defer f.mu.Unlock()

		// If OnDestroy fired before us, the stream is gone — do NOT touch dcb.
		if f.done {
			return
		}

		f.applyDisposition(headers, cc, disp, err)
	}()
}

// applyDisposition applies the checkDisposition to the stream per SPEC §6.3
// steps 7–11. Called from within the resume goroutine under f.mu.
// headers is the incoming client request header map (allow-path mutations land here).
//
// Precondition: f.mu is held by the caller; f.done is false.
func (f *filter) applyDisposition(headers http.Header, cc *compiledConfig, disp checkDisposition, err error) {
	fs := cc.stats

	// Normalize: a non-nil err from checkFn that is NOT already an error class
	// becomes dispError (transport / timeout / context-canceled).
	effectiveClass := disp.class
	if err != nil && effectiveClass == dispAllow {
		// checkFn returned an error AND dispAllow — treat as dispError.
		effectiveClass = dispError
	}

	switch effectiveClass {
	case dispAllow:
		// Allow path: inject upstream headers; optional clear_route_cache;
		// increment ok; resume the chain.
		if fs != nil && fs.ok != nil {
			fs.ok.Inc()
		}
		applyUpstreamMutations(headers, disp)
		if cc.clearRouteCache {
			// cb.ClearRouteCache() is NOT yet on the DecoderFilterCallbacks
			// interface at phase-18.1 MVP (same deferral as phase-17 jwt_authn
			// per ADR-0155 §Consequences). Record the request via the tracking
			// flag; a future framework phase exposes the primitive.
			f.clearRouteCacheRequested = true
		}
		f.dcb.ContinueDecoding()

	case dispDeny:
		// Deny path: emit auth service's response via SendLocalReply;
		// increment denied; then unblock the parked dispatch goroutine.
		//
		// ContinueDecoding() is required even after SendLocalReply: when the
		// async goroutine calls SendLocalReply from outside the dispatch
		// goroutine, the dispatch goroutine is still parked in parkDecode
		// (waiting on decodeResumeCh). SendLocalReply alone sets
		// c.localReplyDone but does NOT unblock the park. ContinueDecoding()
		// wakes the parked goroutine so it can observe localReplyDone=true and
		// return. This mirrors the phase-09 fault filter's
		// SendLocalReply+ContinueDecoding pattern (fault.go:321-324).
		if fs != nil && fs.denied != nil {
			fs.denied.Inc()
		}
		// Convert []headerKV → OrderedHeaders for SendLocalReply.
		oh := headerKVToOrderedHeaders(disp.denyHeaders)
		f.dcb.SendLocalReply(int(disp.denyStatus), string(disp.denyBody), oh)
		f.dcb.ContinueDecoding() // wake the parked dispatch goroutine

	case dispInvalid:
		// Invalid path: validate_mutations rejection; increment invalid;
		// then apply the error posture.
		if fs != nil && fs.invalid != nil {
			fs.invalid.Inc()
		}
		// Fall through to error-posture logic via a synthetic error disposition.
		f.applyErrorPosture(headers, cc, fs)

	default: // dispError (or any unrecognized class + non-nil err)
		f.applyErrorPosture(headers, cc, fs)
	}
}

// applyErrorPosture applies the failure_mode_allow / status_on_error posture
// per SPEC §6.3 + §4. Called from applyDisposition for dispError + dispInvalid.
// Precondition: f.mu is held; f.done is false.
func (f *filter) applyErrorPosture(headers http.Header, cc *compiledConfig, fs *filterStats) {
	if fs != nil && fs.errored != nil {
		fs.errored.Inc()
	}
	if cc.failureModeAllow {
		// failure_mode_allow:true → allow through; increment failureModeAllowed;
		// optionally add x-envoy-auth-failure-mode-allowed header.
		if fs != nil && fs.failureModeAllowed != nil {
			fs.failureModeAllowed.Inc()
		}
		if cc.failureModeAllowHeaderAdd {
			headers.Set("X-Envoy-Auth-Failure-Mode-Allowed", "true")
		}
		f.dcb.ContinueDecoding()
	} else {
		// failure_mode_allow:false → deny with statusOnError + empty body.
		// ContinueDecoding() is required after SendLocalReply: the async goroutine
		// calls SendLocalReply from outside the dispatch goroutine, which is still
		// parked in parkDecode (waiting on decodeResumeCh). SendLocalReply alone
		// sets c.localReplyDone but does NOT unblock the park. ContinueDecoding()
		// wakes the parked goroutine so it can observe localReplyDone=true and
		// return. Mirrors the phase-09 fault filter SendLocalReply+ContinueDecoding
		// pattern (fault.go:321-324).
		f.dcb.SendLocalReply(int(cc.statusOnError), "", nil)
		f.dcb.ContinueDecoding() // wake the parked dispatch goroutine
	}
}

// headerKVToOrderedHeaders converts a []headerKV to envoyhttp.OrderedHeaders
// for the SendLocalReply call-site. Preserves insertion order (decision
// headers first, housekeeping last — per SPEC §4 + parent §5.P11).
func headerKVToOrderedHeaders(kvs []headerKV) envoyhttp.OrderedHeaders {
	if len(kvs) == 0 {
		return nil
	}
	oh := make(envoyhttp.OrderedHeaders, 0, len(kvs))
	for _, kv := range kvs {
		oh = append(oh, envoyhttp.HeaderField{Name: kv.name, Value: kv.value})
	}
	return oh
}

// DecodeHeaders is the request-gate entry per ADR-0156 + ADR-0162.
//
// Full dispatch body per SPEC §6.3 (Task 9):
//  1. Resolve per-route → cache *compiledPerRoute on filter state.
//  2. If perRoute.disabled: return Continue (NO auth call, NO counter increments).
//  3. Compute effective withRequestBody (per-route override OR listener-level).
//  4. If withRequestBody != nil AND !endStream: set awaitingBody + cache
//     bodySettings; return Continue per ADR-0128 synchronous-HCM dispatch
//     constraint (NOT StopIteration — would deadlock the body-read loop).
//  5. Else (no body to buffer, OR endStream with body buffering): fire the
//     async outbound check via dispatchOutboundCheck; return HeaderStopIteration.
//
// The body-buffering branch (step 4) was wired at Task 6. Task 9 adds the
// full per-route resolve + disabled short-circuit + async dispatch + resume.
//
// buildAuthRequest call-site per Task 4 review-fix + Task 9 review fix:
// dispatchOutboundCheck calls buildAuthRequest(f, headers, f.body, path) —
// path extracted from the :path pseudo-header — to construct the
// request-side-filtered *authRequest; see dispatchOutboundCheck for details.
func (f *filter) DecodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Capture the stream's request-arrival instant per PLAN Task 8 §Step 3.
	// This is the canonical anchor for AttributeContext.request.time per
	// SPEC §11.P4 (request-arrival time, NOT the auth-call time). Captured
	// BEFORE any per-route resolution so the body-buffering branch sees the
	// already-set anchor when DecodeData later fires dispatchOutboundCheck.
	// gRPC-mode-only consumer; HTTP-mode 18.1 ignores f.streamStartTime.
	f.streamStartTime = time.Now()

	// Step 1: resolve per-route config + cache on filter state.
	var pr *compiledPerRoute
	if f.state != nil && f.dcb != nil {
		pr = f.state.resolvePerRouteConfig(f.dcb.RequestRouteConfig())
	}
	f.perRoute = pr

	// Step 2: per-route disabled short-circuit — no auth call, no counters.
	// Per SPEC §6.3 step 2 + parent §6 amendment 7.
	if pr != nil && pr.disabled {
		return envoyhttp.Continue
	}

	// Step 3: cache the effective compiledConfig (always listenerRC at 18.1
	// since SHARED-stats; pr.cc was wired to listenerRC at resolve time).
	if pr != nil && pr.cc != nil {
		f.activeRC = pr.cc
	} else if f.activeRC == nil && f.state != nil {
		f.activeRC = f.state.listenerRC
	}

	// Step 4: compute effective withRequestBody.
	effectiveWRB := f.effectiveWithRequestBody(pr)

	// Step 4b: body-buffering branch — if withRequestBody is set AND !endStream,
	// set awaitingBody + cache bodySettings and return Continue (ADR-0128
	// synchronous-HCM constraint: NOT StopIteration here).
	if effectiveWRB != nil && !endStream {
		f.awaitingBody = true
		f.bodySettings = effectiveWRB
		// Cache headers so the body-complete path in DecodeData can pass them
		// to dispatchOutboundCheck for allow-path upstream injection.
		f.cachedHeaders = headers
		return envoyhttp.Continue
	}

	// Step 5: no body to buffer (withRequestBody nil OR endStream=true) — fire
	// the async outbound check now. Return HeaderStopIteration so the chain
	// parks; the resume goroutine calls cb.ContinueDecoding() or SendLocalReply.
	f.dispatchOutboundCheck(headers)
	return envoyhttp.StopIteration
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
	// Fire the async outbound check via the shared dispatchOutboundCheck helper
	// (Option B per SPEC §6.3 KEY DESIGN QUESTION: one source of truth for
	// authRequest construction + goroutine + disposition). Park via
	// DataStopIterationAndBuffer — the resume goroutine calls ContinueDecoding().
	if endStream {
		// Body complete: dispatch the outbound auth check using the cached headers
		// reference stored by DecodeHeaders (f.cachedHeaders). Caching the headers
		// at DecodeHeaders time lets the allow-path upstream mutations (injected by
		// applyUpstreamMutations under mu in the resume goroutine) land on the same
		// header map that the HCM chain already holds — so the injected headers
		// are visible to subsequent filters and the upstream request. If cachedHeaders
		// is nil (degenerate body-only test setup without a DecodeHeaders call),
		// fall back to an empty header map.
		hdrs := f.cachedHeaders
		if hdrs == nil {
			hdrs = make(http.Header)
		}
		f.dispatchOutboundCheck(hdrs)
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

// OnDestroy implements the per-stream teardown per planner-time decision D4
// + SPEC §6.3 + ADR-0159 async-resume discipline:
//
//  1. Acquire f.mu and set f.done = true under the lock. This is the
//     resume-after-OnDestroy guard: any resume goroutine that acquires f.mu
//     after OnDestroy will observe done=true and abort the callback touch
//     without panicking or touching the already-destroyed stream.
//  2. Call f.callCancel() (if non-nil) to cancel the in-flight checkFn's
//     context.Context, causing the in-flight client.Do to return promptly
//     (per the net/http cancellation contract).
//
// ADR-0044 escape-valve verdict: the mu/done + cancellable-context guard
// sufficed. No framework primitive needed. ADR-0165 NOT authored.
func (f *filter) OnDestroy() {
	f.mu.Lock()
	f.done = true
	cancel := f.callCancel
	f.mu.Unlock()

	// Cancel the in-flight outbound call's context OUTSIDE the lock to avoid
	// holding mu during the cancel (which may trigger net/http cleanup).
	if cancel != nil {
		cancel()
	}
}
