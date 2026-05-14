package jwtauthn

// provider.go lands the per-route 8th canonical refinements + runtime-resolve
// helper + the per-provider DataSource + RetryPolicy translation surface per
// SPEC §6.12 + §5 + §1.1 amendment 6 + §11.P12 + ADR-0153.
//
// File-content disposition per PLAN Task 7:
//
//   - Per-route helpers: parsePerRoute + buildCompiledPerRoute +
//     resolvePerRouteConfig — RELOCATED from jwtauthn.go for thematic
//     cohesion. The PLAN File-structure table allows either placement; Task 7
//     consolidates them in provider.go.
//   - Inline helpers: readDataSource (Task 4 inline) + parseRetryPolicy /
//     retryPolicyFromProto (Task 3 inline) + cacheDurationFromProto (Task 3
//     inline) + asyncFetchFromProto (Task 3 inline) — RELOCATED here.
//   - Runtime-resolve helper: (*filter).resolveRequirement — NEW at Task 7.
//     Consumed by Task 9's DecodeHeaders body per SPEC §6.6.
//
// applyResult + applySideEffects + emitDenyResponse + WWW-Authenticate
// construction land at Task 9 per ADR-0155 (still in this file).
//
// Per ADR-0153 §Decision + §Consequences (filled at this task): the 8th
// canonical per-route pattern is `oneof{disabled(bool) | requirement_name
// (string)}` with string-reference-delegation into the listener-level
// requirement_map. Three parse-time cases at parsePerRoute → three runtime
// dispositions at resolveRequirement:
//
//   (a) disabled: true  → compiledPerRoute{disabled: true}  → DecodeHeaders
//       short-circuits to HeaderContinue (no counter increments; passthrough).
//   (b) disabled: false → compiledPerRoute{disabled: false, requirementName:
//       ""} → resolveRequirement falls through to listener-level rules
//       first-match-wins iteration.
//   (c) requirement_name: "<n>" → compiledPerRoute{disabled: false,
//       requirementName: "<n>"} → resolveRequirement looks up
//       activeRC.requirementMap[<n>]; on miss emits 403 + canonical "Failed
//       JWT authentication: Wrong requirement_name: <n>" plain body + denied++
//       per §1.1 amendment 6. NOT 401; NO WWW-Authenticate header (mirrors
//       Envoy filter_config.cc findPerRouteVerifier).
//
// Per ADR-0153 SHARED-stats discipline: per-route does NOT spawn a fresh
// *compiledConfig (string-reference-delegation; the listener-level
// *compiledConfig is the SOLE instance). Mirrors phase-12 csrf + phase-13
// buffer + phase-14 compressor SHARED-stats precedent; DIVERGES from
// phase-11/15/16 INDEPENDENT-stats.

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	jwt_authnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/jwks"
	"github.com/esalaine/envoy-go/internal/jwt"
)

// ---------------------------------------------------------------------------
// Per-route 8th canonical surface (RELOCATED from jwtauthn.go at Task 7 per
// PLAN's File-structure table thematic-cohesion convention).
// ---------------------------------------------------------------------------

// parsePerRoute parses one PerRouteConfig TPFC entry per §5.1 + §6.12 +
// ADR-0153 8th canonical. Returns the raw proto for the registry's per-route
// map; the actual case (a)/(b)/(c) disposition is determined at
// resolvePerRouteConfig lazy-cache time via buildCompiledPerRoute.
//
// envoy-go-side defensive PGV-mirror per §11.P9 + ADR-0149:
//   - PerRouteConfig.requirement_specifier is REQUIRED (oneof PGV
//     `required = true` per config.pb.validate.go:2472-2481). Neither arm
//     set → PARSE-REJECT.
//   - PerRouteConfig.requirement_name is PGV `min_len=1` per
//     config.pb.validate.go:2460-2462. Empty string → PARSE-REJECT.
//
// The dangling-name runtime-resolve case (listener-level requirement_map
// lookup MISS) lands at this file's resolveRequirement helper per ADR-0153
// (runtime-resolve at request time, NOT parse-reject; see §11.P12
// split-semantic).
func parsePerRoute(tc *anypb.Any) (proto.Message, error) {
	if tc == nil {
		return nil, errors.New("jwt_authn: per-route typed_config required")
	}
	var pr jwt_authnv3.PerRouteConfig
	if err := tc.UnmarshalTo(&pr); err != nil {
		return nil, fmt.Errorf("jwt_authn: per-route unmarshal: %w", err)
	}
	// envoy-go-side defensive PGV-mirror per §11.P9.
	if pr.GetRequirementSpecifier() == nil {
		return nil, errors.New("jwt_authn: per-route: requirement_specifier is required")
	}
	if rn, ok := pr.GetRequirementSpecifier().(*jwt_authnv3.PerRouteConfig_RequirementName); ok {
		if rn.RequirementName == "" {
			return nil, errors.New("jwt_authn: per-route: requirement_name: min_len=1 violation")
		}
	}
	return &pr, nil
}

// buildCompiledPerRoute parses one PerRouteConfig into a *compiledPerRoute
// per §5.1 + §6.12 + ADR-0153. Three cases:
//
//   - case (a) disabled: true → {disabled: true, requirementName: ""}.
//   - case (b) disabled: false → {disabled: false, requirementName: ""}; falls
//     through to listener-level rules dispatch at runtime.
//   - case (c) requirement_name: "<name>" → {disabled: false,
//     requirementName: "<name>"}; runtime-resolved against the listener-level
//     requirement_map per ADR-0153.
//
// Per ADR-0153 envoy-go-side PGV-mirror is applied at parsePerRoute (the entry
// point that the HTTPRegistry consults); buildCompiledPerRoute trusts the
// validated proto here.
func buildCompiledPerRoute(pr *jwt_authnv3.PerRouteConfig) (*compiledPerRoute, error) {
	switch t := pr.GetRequirementSpecifier().(type) {
	case *jwt_authnv3.PerRouteConfig_Disabled:
		return &compiledPerRoute{disabled: t.Disabled, requirementName: ""}, nil
	case *jwt_authnv3.PerRouteConfig_RequirementName:
		return &compiledPerRoute{disabled: false, requirementName: t.RequirementName}, nil
	case nil:
		// Defensive: parsePerRoute already rejects this — buildCompiledPerRoute
		// is internally invoked only after parsePerRoute returns success.
		return nil, errors.New("jwt_authn: per-route: requirement_specifier is required")
	default:
		return nil, fmt.Errorf("jwt_authn: per-route: unknown requirement_specifier %T", t)
	}
}

// resolvePerRouteConfig returns the *compiledPerRoute for the given resolved
// per-route TPFC proto message. Pointer-identity lazy-cache per ADR-0117 +
// ADR-0125 §(xiii) + ADR-0153. Race-safe via sync.Map.LoadOrStore.
//
// SIMPLIFIED relative to phase-11/15/16 (which carried per-route INDEPENDENT-
// stats path): phase-17 SHARED-stats discipline per ADR-0154 — no per-route
// *filterStats allocation; only the lightweight *compiledPerRoute disposition.
//
//   - msg == nil → return nil (no per-route entry on this route; caller falls
//     back to listener-level dispatch).
//   - msg.(*PerRouteConfig) type-assertion failure → return nil (defensive;
//     parsePerRoute should have produced the right type).
//   - cache hit → return cached *compiledPerRoute.
//   - cache miss → buildCompiledPerRoute → sync.Map.LoadOrStore.
func (s *factoryState) resolvePerRouteConfig(msg proto.Message) (*compiledPerRoute, error) {
	if msg == nil {
		return nil, nil
	}
	perRoute, ok := msg.(*jwt_authnv3.PerRouteConfig)
	if !ok {
		return nil, nil
	}
	if loaded, ok := s.perRoute.Load(perRoute); ok {
		pr, ok := loaded.(*compiledPerRoute)
		if !ok {
			return nil, fmt.Errorf("jwt_authn: per-route cache: unexpected type %T", loaded)
		}
		return pr, nil
	}
	fresh, err := buildCompiledPerRoute(perRoute)
	if err != nil {
		// Per-route parse failed at lazy resolve. Mirrors phase-16 ADR-0145
		// log+inherit-listener pattern — the caller (DecodeHeaders, Task 9)
		// treats nil-with-error as "fall back to listener-level dispatch".
		return nil, err
	}
	actual, _ := s.perRoute.LoadOrStore(perRoute, fresh)
	pr, ok := actual.(*compiledPerRoute)
	if !ok {
		return nil, fmt.Errorf("jwt_authn: per-route cache: unexpected type %T", actual)
	}
	return pr, nil
}

// ---------------------------------------------------------------------------
// Runtime-resolve helper (NEW at Task 7; consumed by Task 9's DecodeHeaders).
// ---------------------------------------------------------------------------

// resolveRequirement returns the listener-level *compiledRequirement to
// evaluate against this request, per ADR-0153 + SPEC §6.6 + §1.1 amendment 6.
//
// PRECONDITION: the caller MUST short-circuit on `f.perRoute != nil &&
// f.perRoute.disabled == true` BEFORE invoking this helper. Case (a)
// `disabled: true` per SPEC §5.1 + ADR-0153 §Resolution-flow is handled in
// DecodeHeaders (Task 9) by early-returning HeaderContinue with NO counter
// increments. If this precondition is violated, the helper will (incorrectly)
// fall through to listener-level rules iteration since `requirementName == ""`
// for the disabled case — silently re-enabling JWT verification on a route
// that explicitly requested it be disabled.
//
// Three-way return contract:
//
//   - (req, false): a requirement applies; caller evaluates it.
//   - (nil, false): NO rule matches; caller treats this as PASS-THROUGH (no
//     JWT verification required for this request).
//   - (nil, true):  per-route requirement_name dangling-reference miss. The
//     helper has ALREADY emitted SendLocalReply(403, "Failed JWT
//     authentication: Wrong requirement_name: <name>", nil) + incremented
//     denied counter. Caller MUST stop iteration (HeaderStopIteration).
//
// Resolution flow per SPEC §5.3 (cases (b) + (c) only — case (a) is the
// caller's responsibility per the PRECONDITION above):
//
//  1. Per-route case (c) — f.perRoute.requirementName != "" — performs the
//     listener-level requirement_map lookup. Hit → return the named
//     *compiledRequirement. Miss → emit 403 + denied++ + return (nil, true).
//     Status is 403 (NOT 401; per Envoy filter.cc Http::Code::Forbidden);
//     headers is nil per §1.1 amendment 6 (NO WWW-Authenticate header for
//     this per-route error case — distinct from Task 9's evaluator-deny
//     paths).
//  2. Otherwise (case (b) per-route OR no per-route entry) — iterate
//     f.activeRC.rules[] first-match-wins. The matched rule's requirement
//     applies.
//  3. No rule matched → (nil, false). Pass-through.
//
// rules-iteration NOTE: compiledRule.matchFn is the compiled route-match
// predicate landed at Task 13 (`compileRouteMatch` in jwtauthn.go covers the
// PATH-exact + PREFIX path_specifier arms; the remaining arms PARSE-REJECT).
// First-match-wins evaluates `matchFn(:path)`; a nil matchFn is treated as
// WILDCARD-MATCH (defensive — the current compile path always populates
// matchFn). ADR-0149 §Decision (ix)'s deferral of the shared
// router.NewRouteMatcher extraction still stands; matchFn is the interim
// shape. Documented at PROGRESS.md Task 13 entry.
//
// Nil-tolerance per ADR-0085: if f.activeRC.stats == nil, the denied++ guard
// skips. SendLocalReply still fires.
func (f *filter) resolveRequirement(headers http.Header) (req *compiledRequirement, denied bool) {
	// Case (c): per-route requirement_name → runtime-resolve.
	if f.perRoute != nil && f.perRoute.requirementName != "" {
		name := f.perRoute.requirementName
		if r, ok := f.activeRC.requirementMap[name]; ok {
			return r, false
		}
		// Dangling reference: emit 403 + denied++ + signal stop iteration.
		// Per §1.1 amendment 6: NO WWW-Authenticate header (nil headers
		// argument); plain canonical body verbatim with Envoy filter_config.cc.
		if f.activeRC.stats != nil {
			f.activeRC.stats.denied.Inc()
		}
		if f.dcb != nil {
			f.dcb.SendLocalReply(403, "Failed JWT authentication: Wrong requirement_name: "+name, nil)
		}
		return nil, true
	}

	// Cases (b) and no-per-route: fall through to listener-level rules
	// first-match-wins iteration. matchFn nil is treated as wildcard
	// (defensive — current compile path always populates matchFn).
	path := headers.Get(":path")
	for _, rule := range f.activeRC.rules {
		if rule.matchFn == nil || rule.matchFn(path) {
			return rule.requirement, false
		}
	}

	// No rule matched → pass-through (no JWT verification required).
	return nil, false
}

// ---------------------------------------------------------------------------
// readDataSource + retry-policy + cache-duration + async-fetch helpers
// (RELOCATED from jwtauthn.go at Task 7).
// ---------------------------------------------------------------------------

// readDataSource resolves an envoy.config.core.v3.DataSource oneof into raw
// bytes. Supports inline_string + inline_bytes + filename + environment_variable
// per the proto comment. Returned error wraps a descriptive reason — no
// sentinel package on this helper (mirrors phase-16 lookup-helper precedent).
//
// File reads use os.ReadFile (bounded by Go's default file-size policy);
// environment-variable reads use os.LookupEnv (missing env → error).
//
// Consumed by jwtauthn.go's buildCompiledProvider LocalJwks branch.
func readDataSource(ds *envoycorev3.DataSource) ([]byte, error) {
	if ds == nil {
		return nil, errors.New("data_source is nil")
	}
	switch s := ds.GetSpecifier().(type) {
	case *envoycorev3.DataSource_InlineString:
		return []byte(s.InlineString), nil
	case *envoycorev3.DataSource_InlineBytes:
		return s.InlineBytes, nil
	case *envoycorev3.DataSource_Filename:
		if s.Filename == "" {
			return nil, errors.New("data_source filename empty")
		}
		raw, err := os.ReadFile(s.Filename)
		if err != nil {
			return nil, fmt.Errorf("data_source read file %q: %w", s.Filename, err)
		}
		return raw, nil
	case *envoycorev3.DataSource_EnvironmentVariable:
		if s.EnvironmentVariable == "" {
			return nil, errors.New("data_source environment_variable empty")
		}
		v, ok := os.LookupEnv(s.EnvironmentVariable)
		if !ok {
			return nil, fmt.Errorf("data_source env %q not set", s.EnvironmentVariable)
		}
		return []byte(v), nil
	case nil:
		return nil, errors.New("data_source specifier is required")
	default:
		return nil, fmt.Errorf("data_source unsupported specifier %T", s)
	}
}

// cacheDurationFromProto translates the optional durationpb.Duration carrying
// JwtProvider.remote_jwks.cache_duration into a time.Duration. Returns 0 when
// the proto is nil (the jwks.New constructor applies its 10-minute default).
func cacheDurationFromProto(d *durationpb.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return d.AsDuration()
}

// asyncFetchFromProto translates *jwt_authnv3.JwksAsyncFetch → *jwks.AsyncFetch.
func asyncFetchFromProto(af *jwt_authnv3.JwksAsyncFetch) *jwks.AsyncFetch {
	if af == nil {
		return nil
	}
	out := &jwks.AsyncFetch{FastListener: af.GetFastListener()}
	if d := af.GetFailedRefetchDuration(); d != nil {
		out.FailedRefetchDuration = d.AsDuration()
	}
	return out
}

// retryPolicyFromProto translates *envoycorev3.RetryPolicy → *jwks.RetryPolicy.
// Per §11.P4 + ADR-0150: NumRetries from the wrapperspb.UInt32Value; base +
// max from the BackoffStrategy sub-message.
func retryPolicyFromProto(rp *envoycorev3.RetryPolicy) *jwks.RetryPolicy {
	if rp == nil {
		return nil
	}
	out := &jwks.RetryPolicy{}
	if v := rp.GetNumRetries(); v != nil {
		out.NumRetries = v.GetValue()
	}
	if bs := rp.GetRetryBackOff(); bs != nil {
		if d := bs.GetBaseInterval(); d != nil {
			out.BaseInterval = d.AsDuration()
		}
		if d := bs.GetMaxInterval(); d != nil {
			out.MaxInterval = d.AsDuration()
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// applyResult / applySideEffects / emitDenyResponse — Task 9 per ADR-0155.
//
//   - applyResult: ALLOWED → allowed++ + applySideEffects + HeaderContinue;
//                  DENIED → denied++ + emitDenyResponse + HeaderStopIteration.
//   - applySideEffects: 4-step emit-order per §6.9 + §11.P10 + §11.P13:
//                       (1) strip-on-success (forward=false) →
//                       (2) forward_payload_header base64url encoding →
//                       (3) claim_to_headers dot-notation extract+stringify →
//                       (4) clear_route_cache TRACKING flag flip.
//   - emitDenyResponse: 401-or-403 dispatch (403 only for
//                       ErrJwtAudienceNotAllowed); canonical jwt_verify_lib
//                       body; WWW-Authenticate Bearer realm + conditional
//                       error="invalid_token"; strip_failure_response gate.
// ---------------------------------------------------------------------------

// applyResult applies the evalResult disposition + counter + side-effects
// per SPEC §6.9 + ADR-0155. Three outcomes:
//
//   - DENIED → denied++ + emitDenyResponse(r.err) + HeaderStopIteration.
//   - ALLOWED with token+provider populated → allowed++ + applySideEffects
//     (strip + forward_payload_header + claim_to_headers + clear_route_cache).
//   - ALLOWED with NO token+provider (e.g., reqAllowMissing with no token
//     extracted, or reqAllowMissingOrFailed) → allowed++ + HeaderContinue
//     (no side-effects since no token survived to mutate against).
//
// Nil-tolerance per ADR-0085: f.activeRC.stats may be nil (test path); the
// counter increment guards.
func (f *filter) applyResult(r evalResult, headers http.Header) envoyhttp.FilterHeadersStatus {
	if !r.allowed {
		if f.activeRC != nil && f.activeRC.stats != nil {
			f.activeRC.stats.denied.Inc()
		}
		f.emitDenyResponse(r.err)
		return envoyhttp.StopIteration
	}

	if f.activeRC != nil && f.activeRC.stats != nil {
		f.activeRC.stats.allowed.Inc()
	}

	if r.token != nil && r.provider != nil {
		f.applySideEffects(r.token, r.provider, headers)
	}

	return envoyhttp.Continue
}

// applySideEffects emits the 4-step success-side-effect sequence per SPEC §6.9
// + §11.P10 + §11.P13 + ADR-0149 + ADR-0155. Order is structurally pinned:
//
//  1. strip-on-success: when !p.forward, strip the extraction sources from the
//     forwarded request per planner-time decision 8. from_cookies UNTOUCHED
//     per proto caveat.
//  2. forward_payload_header: base64url-encode the payload + Set the header.
//     padForwardPayloadHdr controls trailing `=` retention.
//  3. claim_to_headers: dot-notation extract via Token.PayloadClaim;
//     stringify scalar values; SILENT-SKIP on err (array claim per §11.P10
//     OR missing claim) OR on non-stringifiable values.
//  4. clear_route_cache: when p.clearRouteCache is true, flip the filter's
//     clearRouteCacheRequested TRACKING flag. The framework primitive
//     cb.ClearRouteCache() is NOT yet exposed on DecoderFilterCallbacks
//     (deferred to a future HCM framework phase per ADR-0155 §Consequences).
//
// Headers mutations use the supplied http.Header (NOT a fresh copy) — the
// in-place mutation is what subsequent filters in the decode chain consume.
func (f *filter) applySideEffects(t *jwt.Token, p *compiledProvider, headers http.Header) {
	// (1) Strip-on-success per planner-time decision 8 + §11.P10.
	if !p.forward {
		stripExtractionSources(headers, p)
	}

	// (2) forward_payload_header per §11.P13.
	if p.forwardPayloadHeader != "" {
		encoded := encodeBase64URL(t.RawPayload, p.padForwardPayloadHdr)
		headers.Set(p.forwardPayloadHeader, encoded)
	}

	// (3) claim_to_headers per §11.P10 + §1.1 amendment 10.
	for _, ch := range p.claimToHeaders {
		val, err := t.PayloadClaim(ch.claimName)
		if err != nil {
			// SILENT-SKIP per §11.P10: array claim OR missing claim. Do not
			// surface either as an error to the caller — the proto comment
			// at JwtClaimToHeader.claim_name says "Array type claims are not
			// supported" + Envoy filter_config.cc silently skips missing
			// claims; envoy-go MIRRORS.
			continue
		}
		s, ok := stringify(val)
		if !ok {
			// Non-stringifiable (map / array surviving past PayloadClaim's
			// own guard — defensive). SILENT-SKIP per §11.P10.
			continue
		}
		headers.Set(ch.headerName, s)
	}

	// (4) clear_route_cache — TRACKING flag flip per ADR-0155 §Consequences.
	if p.clearRouteCache {
		f.clearRouteCacheRequested = true
		// TODO(future framework phase): when cb.ClearRouteCache() lands on
		// DecoderFilterCallbacks, invoke `f.dcb.ClearRouteCache()` here. The
		// TRACKING flag remains as the test-introspection anchor + serves as
		// an empirical-divergence pin (operators depending on clear-route-
		// cache see envoy-go-Envoy divergence at the BEHAVIOR_CONTRACT entry
		// per ADR-0155 §Consequences).
	}
}

// emitDenyResponse synthesizes the 401-or-403 deny response per SPEC §4 + §6.9
// + §1.1 amendments 8 + 11 + 12 + ADR-0155 + RFC 6750. Behavior:
//
//   - strip_failure_response: true → SendLocalReply(code, "", nil) per §11.P3.
//     Body empty + nil headers carrier (NO WWW-Authenticate).
//   - else → SendLocalReply(code, reason.Error(), {WWW-Authenticate,
//     Content-Type}). WWW-Authenticate per RFC 6750 §3:
//     `Bearer realm="<originalURI>"` + conditional `, error="invalid_token"`
//     for non-JwtMissed.
//
// Nil-tolerance per ADR-0085: f.dcb may be nil in test code paths that exercise
// emitDenyResponse without a wired callback. The function returns silently in
// that case (the caller observes nil-effect; production never sees nil-f.dcb
// since SetDecoderCallbacks is always invoked by the framework first per
// ADR-0071).
func (f *filter) emitDenyResponse(reason error) {
	if f.dcb == nil {
		return
	}
	code := mapStatusToHTTPCode(reason)

	if f.activeRC != nil && f.activeRC.stripFailureResponse {
		// Per §11.P3 RATIFIED: body empty + nil headers carrier (NO
		// WWW-Authenticate). The 4-header standard set (content-length:0 +
		// date + server: envoy) is still injected by the framework's
		// SendLocalReply path per ADR-0085 + §11.2 OrderedHeaders discipline.
		f.dcb.SendLocalReply(code, "", nil)
		return
	}

	body := ""
	if reason != nil {
		body = reason.Error()
	}

	// WWW-Authenticate Bearer realm per RFC 6750 §3 + §1.1 amendment 12 +
	// §11.P2. Realm uses the original :path captured at DecodeHeaders entry
	// (NOT the JWT provider's issuer — REFINES BRAINSTORM §4 hypothesis;
	// see ADR-0155 §Context).
	realm := f.originalURI
	if realm == "" {
		// Defensive default per ADR-0155: realm parameter is REQUIRED by
		// RFC 6750 §3 syntax; fall back to "/" when :path was absent (rare
		// in practice but possible for non-HTTP-protocol shim paths).
		realm = "/"
	}
	wwwAuth := `Bearer realm="` + realm + `"`

	// Conditional error parameter for non-JwtMissed per RFC 6750 §3 +
	// §1.1 amendment 12 + §11.P2. JwtMissed = "no credential supplied" →
	// no rejection-of-credential to signal.
	if !errors.Is(reason, jwt.ErrJwtMissed) {
		wwwAuth += `, error="invalid_token"`
	}

	headers := envoyhttp.OrderedHeaders{
		{Name: "www-authenticate", Value: wwwAuth},
		{Name: "content-type", Value: "text/plain"},
	}
	f.dcb.SendLocalReply(code, body, headers)
}

// mapStatusToHTTPCode dispatches the deny-path status code per §1.1 amendment
// 8 + §11.P1 + ADR-0155. Envoy filter.cc verbatim:
//
//	auto code = (status == Status::JwtAudienceNotAllowed)
//	    ? Http::Code::Forbidden : Http::Code::Unauthorized;
//
// All non-audience failure-reasons (including nil) → 401. JwtAudienceNotAllowed
// → 403. Per RFC 6750 §3 semantic: 401 = unauthenticated/credential-invalid;
// 403 = authenticated-but-not-authorized-for-this-resource (audience mismatch
// is the canonical "authenticated but not for this audience" case).
func mapStatusToHTTPCode(reason error) int {
	if errors.Is(reason, jwt.ErrJwtAudienceNotAllowed) {
		return 403
	}
	return 401
}

// encodeBase64URL emits the base64url-encoded form of the payload per
// §11.P13 + JwtProvider.pad_forward_payload_header proto comment + ADR-0155.
//
// jwt.Token.RawPayload is ALREADY base64url-encoded (the unpadded form per
// RFC 7515 §2's base64url-encoded payload definition). This helper applies
// optional `=` padding based on the provider's pad_forward_payload_header
// flag:
//
//   - pad=false (default per proto bool zero-value) → STRIP trailing `=`
//     (no-op since RawPayload is already unpadded; the strip is defensive).
//   - pad=true → APPEND `=` characters until length % 4 == 0 (canonical
//     base64 padding per RFC 4648 §5).
//
// The raw payload's contents (URL-safe alphabet `A-Z`, `a-z`, `0-9`, `-`,
// `_`) is preserved verbatim — only the `=` padding suffix varies.
func encodeBase64URL(raw string, pad bool) string {
	if pad {
		// Append `=` until length is a multiple of 4 per RFC 4648 §5.
		out := raw
		for len(out)%4 != 0 {
			out += "="
		}
		return out
	}
	// Strip trailing `=` for unpadded form (RFC 7515 §2). No-op when raw is
	// already unpadded (the standard case for jwt.Token.RawPayload).
	return strings.TrimRight(raw, "=")
}

// stripExtractionSources removes the JWT-extraction surface from the forwarded
// request per planner-time decision 8 + §6.9 + ADR-0155. Invoked from
// applySideEffects step 1 when provider.forward is false (the default; per
// JwtProvider.forward proto comment "if false, JWT is removed in the request
// after a success verification").
//
// Behavior:
//
//   - NO explicit extraction sources configured (defaults path) → strip the
//     default Authorization header + the access_token query parameter.
//   - Explicit sources → strip the configured from_headers entries + the
//     configured from_params entries (URL rewrite of :path). from_cookies is
//     UNTOUCHED per the JwtProvider.from_cookies proto comment caveat (the
//     proto explicitly notes JWT in cookies is NOT stripped on success).
//
// Header mutations use http.Header.Del (case-insensitive HTTP header semantic).
// Query-param rewrites use url.ParseQuery + reassemble via Values.Encode +
// preserve the path prefix verbatim.
//
// SIMPLIFICATION per task prompt: strip ALL configured extraction sources
// unconditionally (rather than only the source that yielded the validated
// token). This matches Envoy's strip-on-success semantic — the proto doesn't
// distinguish per-source token disposition; a successfully-validated JWT means
// ALL configured sources for that provider are stripped.
func stripExtractionSources(headers http.Header, p *compiledProvider) {
	hasExplicit := len(p.fromHeaders) > 0 || len(p.fromParams) > 0 || len(p.fromCookies) > 0

	if !hasExplicit {
		// DEFAULTS path: strip Authorization header entirely + access_token
		// query param.
		headers.Del("Authorization")
		if path := headers.Get(":path"); path != "" {
			newPath := stripQueryParam(path, "access_token")
			if newPath != path {
				headers.Set(":path", newPath)
			}
		}
		return
	}

	// EXPLICIT path: strip from_headers + from_params (rewrite :path).
	// from_cookies UNTOUCHED per proto caveat.
	for _, h := range p.fromHeaders {
		headers.Del(h.name)
	}

	if len(p.fromParams) > 0 {
		path := headers.Get(":path")
		newPath := path
		for _, name := range p.fromParams {
			newPath = stripQueryParam(newPath, name)
		}
		if newPath != path {
			headers.Set(":path", newPath)
		}
	}

	// from_cookies UNTOUCHED per JwtProvider.from_cookies proto comment.
}

// stripQueryParam removes a query parameter from a URL path. Used by
// stripExtractionSources to rewrite :path after stripping from_params (or the
// default access_token query param).
//
// Behavior:
//
//   - No `?` separator → return path verbatim (no query string to strip).
//   - Parse-failure → return path verbatim (defensive; malformed query
//     surface preserved rather than silently rewritten).
//   - All params removed → return base path (no trailing `?`).
//   - Some params remain → return base + `?` + re-encoded query.
//
// URL fragment (`#`-suffix) is preserved if present after the query (the
// rare case where :path carries a fragment — HTTP/2 :path generally does
// not, but defensive).
func stripQueryParam(path, name string) string {
	qi := strings.Index(path, "?")
	if qi < 0 {
		return path
	}
	base := path[:qi]
	qs := path[qi+1:]
	// Isolate fragment if present (rare in :path).
	frag := ""
	if fi := strings.Index(qs, "#"); fi >= 0 {
		frag = qs[fi:] // includes the leading '#'
		qs = qs[:fi]
	}
	vals, err := url.ParseQuery(qs)
	if err != nil {
		return path
	}
	if _, ok := vals[name]; !ok {
		// Param not present → no-op rewrite. Return verbatim to avoid
		// re-encoding spurious changes.
		return path
	}
	delete(vals, name)
	if len(vals) == 0 {
		return base + frag
	}
	return base + "?" + vals.Encode() + frag
}

// stringify coerces a JWT-claim value to its string form per §11.P10 + §1.1
// amendment 10 + ADR-0155. Supports scalar JSON types (string, bool, number,
// null); arrays and objects return ok=false for SILENT-SKIP per Envoy
// filter_config.cc's claim_to_headers silent-skip discipline.
//
// JSON-decoded number values are always float64 in Go's encoding/json default;
// stringify detects integral float64 values and emits them as integer strings
// (e.g., `1700000000.0` → `"1700000000"` for a typical exp claim) — operator-
// intuitive for the common integer-valued numeric claims.
func stringify(val interface{}) (string, bool) {
	switch v := val.(type) {
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	case float64:
		// Integral float64 → int64 string. Common for unix-time claims (exp,
		// nbf, iat) which JSON-decode as float64 but operators expect as
		// integer strings.
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), true
		}
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case nil:
		return "", true
	default:
		// Array, map, or other unhandled type → SILENT-SKIP per §11.P10.
		return "", false
	}
}
