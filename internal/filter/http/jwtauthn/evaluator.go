package jwtauthn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/esalaine/envoy-go/internal/jwks"
	"github.com/esalaine/envoy-go/internal/jwt"
)

// ---------------------------------------------------------------------------
// Token extraction surface (Task 5 of phase-17 PLAN per ADR-0152).
//
// `extractTokens` walks the 4 extraction sources defined by SPEC §6.7 +
// §11.P14 + §11.P15 in the same iteration order as Envoy's `extractor.cc`:
//
//	(1) configured `from_headers[]` — in declared order; value_prefix is
//	    substring-searched via `strings.Index` per ADR-0152 §Decision
//	    (NOT `strings.HasPrefix` — Envoy uses substring search so a prefix may
//	    appear anywhere inside the header value; `Bearer ` at offset 0 is the
//	    common case, but `JWT=` after a structured tag is also supported).
//	    Post-prefix bytes pass through `stripNonBase64URLChars` to trim
//	    trailing non-base64url-alphabet characters (whitespace, separators)
//	    per Envoy `extractor.cc::extractJWT`.
//	(2) **DEFAULTS** — applied iff `len(fromHeaders) == 0 && len(fromParams) == 0 &&
//	    len(fromCookies) == 0`. The defaults are NOT additive over configured
//	    sources; any explicit source SUPPRESSES them entirely (per Envoy
//	    extractor.cc + §11.P14 RATIFIED). The two defaults are:
//	    - Authorization header with case-sensitive `Bearer ` prefix (RFC 6750 §2.1);
//	    - `access_token` query parameter (case-sensitive name match;
//	      URL-decoded via `url.ParseQuery`).
//	(3) configured `from_params[]` — case-sensitive name match; URL-decoded
//	    via `url.ParseQuery`; first-value-only on multi-value query per
//	    §11.P14 REFINED.
//	(4) configured `from_cookies[]` — case-sensitive name match; cookie value
//	    used VERBATIM (no URL-decode per §11.P15 RATIFIED).
//
// First-success-wins discipline lives in `evaluateProvider` (Task 6); this
// function returns ALL candidate tokens in iteration order. Empty result maps
// to `ErrJwtMissed` failure-reason at the deny-path emit (Task 9 per ADR-0155).
// ---------------------------------------------------------------------------

// extractedToken is one (rawToken, source, name) tuple per extraction match.
// The `name` field carries the header / param / cookie name the token was
// extracted from — used by Task 9's `forward=false` request-mutation pass to
// strip the JWT from the forwarded request on success per SPEC §6.11.
type extractedToken struct {
	raw  string     // the JWT bytes (post-prefix-strip; post-non-base64url-trim for value_prefix extractions)
	src  sourceKind // which extraction source produced this token
	name string     // header / param / cookie name; "authorization" or "access_token" for the defaults
}

// sourceKind enumerates the 3 extraction-source families per ADR-0152.
type sourceKind int

const (
	// sourceHeader: extracted from an HTTP header (default Authorization OR
	// configured from_headers entry).
	sourceHeader sourceKind = iota
	// sourceParam: extracted from a URL query parameter (default access_token
	// OR configured from_params entry).
	sourceParam
	// sourceCookie: extracted from the Cookie header (configured from_cookies
	// entry; there is no default cookie source).
	sourceCookie
)

// extractTokens implements the 4-source extraction-iteration surface per
// SPEC §6.7 + §11.P14 + §11.P15 + ADR-0152. Returns the list of (rawToken,
// sourceKind, name) tuples in iteration order; empty when no extraction
// matched.
//
// Iteration discipline:
//
//   - If the provider declares ANY explicit extraction source (from_headers,
//     from_params, or from_cookies), the defaults (Authorization Bearer +
//     access_token query) are SUPPRESSED entirely (per Envoy extractor.cc).
//   - Otherwise the defaults apply: Authorization with case-sensitive
//     `Bearer ` prefix, followed by `access_token` query param.
//
// The caller (Task 6 evaluateProvider) iterates the returned slice
// first-success-wins: each token is attempted against the provider's
// JWKS; the first successful Parse + VerifySignature + ValidateClaims wins.
func extractTokens(p *compiledProvider, headers http.Header) []extractedToken {
	hasExplicit := len(p.fromHeaders) > 0 || len(p.fromParams) > 0 || len(p.fromCookies) > 0

	if !hasExplicit {
		// DEFAULTS path: Authorization Bearer + access_token query.
		var out []extractedToken
		if v := headers.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
			// Mirror the explicit from_headers post-prefix treatment: trim
			// leading whitespace then strip trailing non-base64url-alphabet
			// chars per Envoy `extractor.cc::extractJWT`. Real-world headers
			// like `Bearer eyJ...; foo=bar` MUST NOT propagate trailing
			// garbage to the verifier.
			raw := stripNonBase64URLChars(strings.TrimSpace(v[len("Bearer "):]))
			if raw != "" {
				out = append(out, extractedToken{
					raw:  raw,
					src:  sourceHeader,
					name: "authorization",
				})
			}
		}
		path := headers.Get(":path")
		if vals, ok := parseQueryParam(path, "access_token"); ok && len(vals) > 0 && vals[0] != "" {
			out = append(out, extractedToken{
				raw:  vals[0],
				src:  sourceParam,
				name: "access_token",
			})
		}
		return out
	}

	// EXPLICIT path: from_headers → from_params → from_cookies. Defaults
	// suppressed.
	var out []extractedToken

	// (1) Configured from_headers, in declared order. value_prefix is
	// substring-searched per ADR-0152.
	for _, hl := range p.fromHeaders {
		v := headers.Get(hl.name)
		if v == "" {
			continue
		}
		if hl.valuePrefix == "" {
			// Verbatim header-value as JWT; trim leading whitespace then
			// strip trailing non-base64url chars defensively.
			raw := stripNonBase64URLChars(strings.TrimSpace(v))
			if raw == "" {
				continue
			}
			out = append(out, extractedToken{raw: raw, src: sourceHeader, name: hl.name})
			continue
		}
		// Substring search via strings.Index. The JWT bytes are everything
		// AFTER the prefix; trailing non-base64url chars are stripped.
		idx := strings.Index(v, hl.valuePrefix)
		if idx < 0 {
			continue
		}
		candidate := v[idx+len(hl.valuePrefix):]
		raw := stripNonBase64URLChars(candidate)
		if raw == "" {
			continue
		}
		out = append(out, extractedToken{raw: raw, src: sourceHeader, name: hl.name})
	}

	// (2) Configured from_params — case-sensitive; URL-decoded;
	// first-value-only per §11.P14 REFINED.
	if len(p.fromParams) > 0 {
		path := headers.Get(":path")
		for _, paramName := range p.fromParams {
			vals, ok := parseQueryParam(path, paramName)
			if !ok || len(vals) == 0 || vals[0] == "" {
				continue
			}
			out = append(out, extractedToken{
				raw:  vals[0],
				src:  sourceParam,
				name: paramName,
			})
		}
	}

	// (3) Configured from_cookies — case-sensitive; verbatim value (no
	// URL-decode) per §11.P15 RATIFIED.
	if len(p.fromCookies) > 0 {
		cookies := parseCookies(headers.Get("Cookie"))
		for _, cookieName := range p.fromCookies {
			v, ok := cookies[cookieName]
			if !ok || v == "" {
				continue
			}
			out = append(out, extractedToken{
				raw:  v,
				src:  sourceCookie,
				name: cookieName,
			})
		}
	}

	return out
}

// parseQueryParam extracts the values for `name` from the query portion of
// `path` per SPEC §11.P14. Uses Go stdlib `url.ParseQuery` for URL-decoding
// (matches Envoy's `QueryParamsMulti::parseAndDecodeQueryString` semantic).
// Returns (values, true) on match; (nil, false) on absence or parse failure.
//
// Case-sensitivity: `url.ParseQuery` preserves case in the value map keys, so
// case-sensitive name matching is a verbatim map lookup.
//
// Multi-value handling: returns the full slice; the caller (extractTokens)
// uses only `vals[0]` per the first-value-only discipline. Returning the slice
// (rather than just the first value) keeps the helper compositional for
// future callers that may want all values.
func parseQueryParam(path, name string) ([]string, bool) {
	if path == "" {
		return nil, false
	}
	// Isolate the query string between `?` and (optionally) `#`.
	q := ""
	if i := strings.Index(path, "?"); i >= 0 {
		q = path[i+1:]
	} else {
		return nil, false
	}
	if j := strings.Index(q, "#"); j >= 0 {
		q = q[:j]
	}
	if q == "" {
		return nil, false
	}
	vals, err := url.ParseQuery(q)
	if err != nil {
		return nil, false
	}
	if v, ok := vals[name]; ok {
		return v, true
	}
	return nil, false
}

// parseCookies parses an HTTP Cookie header value into a map of name → value
// entries per RFC 6265 §4.2.1 + SPEC §11.P15 RATIFIED. Cookie names are
// case-sensitive (preserved verbatim); cookie values are VERBATIM (no
// URL-decode per §11.P15). Surrounding double-quotes are stripped per RFC 6265
// quoted-value syntax (`v` → `"v"` → `v`).
//
// Multiple cookies are separated by `; ` (semicolon + optional whitespace);
// each pair is `name=value`. Pairs without `=` or with `=` at position 0
// (empty name) are silently skipped. Empty cookie header → empty map.
func parseCookies(cookieHeader string) map[string]string {
	out := make(map[string]string)
	if cookieHeader == "" {
		return out
	}
	for _, kv := range strings.Split(cookieHeader, ";") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		i := strings.Index(kv, "=")
		if i <= 0 {
			// No `=` separator OR empty cookie name → skip.
			continue
		}
		name := kv[:i]
		value := kv[i+1:]
		// RFC 6265 §5.2 allows quoted cookie values; strip outer DQUOTE.
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		out[name] = value
	}
	return out
}

// isCorsPreflightRequest implements the §11.P1 + Envoy `filter.cc` verbatim
// 3-condition AND: `:method == OPTIONS && origin != "" && access-control-
// request-method != ""`. Used by Task 9 DecodeHeaders to short-circuit JWT
// validation for preflight requests when `bypass_cors_preflight: true`.
//
// Header lookups are case-INSENSITIVE per HTTP/1.1 RFC 7230 (Go stdlib
// `http.Header.Get` canonicalizes); `:method` comparison is case-SENSITIVE on
// the value (`OPTIONS` only, NOT `Options`) per Envoy + HTTP method-name
// case-sensitive convention.
func isCorsPreflightRequest(headers http.Header) bool {
	if headers.Get(":method") != "OPTIONS" {
		return false
	}
	if headers.Get("Origin") == "" {
		return false
	}
	if headers.Get("Access-Control-Request-Method") == "" {
		return false
	}
	return true
}

// stripNonBase64URLChars returns the longest leading run of characters that
// belong to the JWT-valid alphabet (RFC 4648 §5 base64url: `A-Z`, `a-z`,
// `0-9`, `-`, `_`) plus the JWT segment-separator `.` and the optional
// base64 padding `=`. The first character that falls outside this alphabet
// truncates the result. Per Envoy `extractor.cc::extractJWT`: when
// extracting from a value_prefix-substring inside a structured header value
// (e.g., `tag=foo; JWT=eyJ...; bar=baz` with prefix `JWT=`), the
// post-prefix bytes are `eyJ...; bar=baz`; the JWT span is the leading
// `eyJ...` run before `;` which is the first non-alphabet character. Whole-
// span truncation is consistent with: (a) JWT bytes must be entirely
// base64url + `.` + `=` per RFC 7519 §3; (b) any non-alphabet character
// surfaces a malformed-JWT — better to truncate than to feed a structurally
// invalid byte stream into the verifier.
//
// Empty input → empty output. Input starting with a non-alphabet character
// → empty output.
func stripNonBase64URLChars(s string) string {
	for i := 0; i < len(s); i++ {
		if !isBase64URLOrJWTChar(s[i]) {
			return s[:i]
		}
	}
	return s
}

// isBase64URLOrJWTChar reports whether b is one of the characters that can
// legitimately appear in a JWT (base64url alphabet + `.` segment-separator +
// `=` optional padding).
func isBase64URLOrJWTChar(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '-' || b == '_' || b == '.' || b == '=':
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// 6-variant JwtRequirement evaluator + per-token evaluateProvider (Task 6 of
// phase-17 PLAN per SPEC §6.8 + §11.P16 + ADR-0149).
//
// `evaluateRequirement` recurses on `reqRequiresAny` / `reqRequiresAll`
// children. `evaluateProvider` iterates extracted tokens (first-success-wins),
// resolves the key via `compiledProvider.localJwks` OR
// `compiledProvider.jwksFetcher.Get()` (RemoteJwks path; with
// jwks_fetch_success / jwks_fetch_failed counter increments per §11.P6), and
// runs the 3-step validation pipeline `jwt.Parse → keyset.Lookup → t.VerifySignature → t.ValidateClaims`.
//
// Counter wiring (jwksFetchSuccess / jwksFetchFailed) follows the SPEC §6.8
// literal pattern — counters increment on each `jwksFetcher.Get()` outcome.
// Since `jwks.Fetcher.Get()` returns the cached `*JWKSet` without
// distinguishing fresh-fetch-vs-cache-hit, the counters semantically reflect
// "evaluator-side JWKS-resolution outcomes" rather than Envoy's "actual HTTP
// fetch attempts". The discrepancy is documented for Task 8 closure — if the
// empirical scrape reveals Envoy's counter semantics differ, Task 8 can refine
// (e.g., add an Observer callback to `jwks.Fetcher` that fires only on actual
// HTTP outcomes).
// ---------------------------------------------------------------------------

// evalResult is the per-evaluation outcome carried back from
// `evaluateRequirement` / `evaluateProvider`. On success: `allowed=true`,
// `err=nil`, and `token`+`provider` carry the validated token + the provider
// that validated it (used by Task 9's `applySideEffects` 4-step emit-order
// per SPEC §6.9). On failure: `allowed=false`, `err` carries the canonical
// failure-reason per §11.P1.
type evalResult struct {
	allowed  bool
	err      error             // canonical failure-reason per §11.P1; nil on success
	token    *jwt.Token        // non-nil on successful validation (for side-effects)
	provider *compiledProvider // non-nil on successful validation (for side-effects)
}

// evaluateRequirement is the 6-variant JwtRequirement evaluator per SPEC §6.8
// + §11.P16 + ADR-0149. Top-level entry called by Task 9's `DecodeHeaders`
// body once the active requirement has been resolved (listener-level rule
// match OR per-route `requirement_name` lookup).
//
// Variant semantics:
//
//   - reqProviderName            → validate against named provider with the
//     provider's OWN audiences list.
//   - reqProviderAndAudiences    → validate against named provider with the
//     per-rule audience override (`audOverr`).
//   - reqRequiresAny             → OR-semantic; short-circuit on first allowed;
//     if all fail, return the LAST failure error (per Envoy verifier.cc).
//   - reqRequiresAll             → AND-semantic; short-circuit on first
//     failure; on all-success, return the LAST success result (which carries
//     the last validated token+provider — Envoy's verifier.cc behavior; the
//     first-success token-set is fine for side-effects since `requires_all`
//     typically references a single provider's strict requirement).
//   - reqAllowMissing            → JWT absent → OK; JWT present-and-invalid
//     → FAIL (per §11.P16 RATIFIED-AND-EXTENDED + planner-time decision 4).
//   - reqAllowMissingOrFailed    → always OK regardless of token state.
//
// Defensive disposition: nil requirement (proto unset surfaced post-
// compilation) treated as `reqAllowMissingOrFailed` per SPEC §6.5 +
// `buildCompiledRequirement` default. Unknown kind → deny with descriptive
// error (defensive — `requirementKind` constants are package-internal).
func (f *filter) evaluateRequirement(req *compiledRequirement, headers http.Header) evalResult {
	if req == nil {
		// Defensive: nil should not surface post-buildCompiledRequirement
		// (which substitutes reqAllowMissingOrFailed for nil proto). Treat as
		// allowed to mirror the proto-comment "JWT verification is optional".
		return evalResult{allowed: true}
	}
	switch req.kind {
	case reqProviderName:
		// Provider's OWN audiences list applies per SPEC §6.8 (the
		// requirement does NOT override). compiledProvider.audiences may be
		// empty → ValidateClaims skips aud-check per §11.P16.
		return f.evaluateProvider(req.provider, headers, req.provider.audiences)
	case reqProviderAndAudiences:
		// Per-rule audOverr REPLACES (not merges) the provider's audiences
		// per Envoy filter_config.cc + §11.P16 (also see SPEC §6.8 evalProvider
		// signature `effectiveAudiences`).
		return f.evaluateProvider(req.provider, headers, req.audOverr)
	case reqRequiresAny:
		// OR-semantic: short-circuit on first allowed; if all fail, return the
		// MOST-INFORMATIVE failure error per Envoy verifier.cc behavior
		// (Task-13 empirical refinement). Priority order:
		//
		//   1. JWT-specific errors from a provider that matched + verified +
		//      claim-failed (e.g., ErrJwtExpired, ErrJwtVerificationFail,
		//      ErrJwtAudienceNotAllowed). These carry the most user-actionable
		//      reason.
		//   2. JWKS lookup misses (ErrJwksKidAlgMismatch) — the provider has
		//      no key matching the token's kid/alg.
		//   3. ErrJwtMissed — the provider's extraction sources produced no
		//      token (least informative since other providers might have
		//      extracted but failed differently).
		//
		// Selection rule: keep the FIRST error of the highest priority class
		// seen across children. Without this, a later kid-mismatch from a
		// non-matching provider would overwrite an earlier expired/verify-fail
		// from the matching provider — producing a misleading body string
		// like "jwks: no key matched kid+alg" instead of "Jwt is expired".
		var bestErr error
		for _, sub := range req.children {
			r := f.evaluateRequirement(sub, headers)
			if r.allowed {
				return r
			}
			bestErr = mergeRequiresAnyErr(bestErr, r.err)
		}
		return evalResult{allowed: false, err: bestErr}
	case reqRequiresAll:
		// AND-semantic: short-circuit on first failure. On all-success,
		// return the LAST success result (which carries the last validated
		// token+provider — Envoy's verifier.cc behavior; side-effects at
		// Task 9 use the surviving token+provider). Empty children (vacuous
		// AND) → allowed with no token/provider.
		last := evalResult{allowed: true}
		for _, sub := range req.children {
			r := f.evaluateRequirement(sub, headers)
			if !r.allowed {
				return r
			}
			last = r
		}
		return last
	case reqAllowMissing:
		// Per §11.P16 RATIFIED-AND-EXTENDED + planner-time decision 4:
		// iterate ALL providers' extraction sources. If ANY provider's
		// extraction produced a token, that token MUST validate (against
		// THAT provider) — present-and-invalid → FAIL. If NO provider
		// extracted a token, missing-OK.
		//
		// MVP simplification per SPEC §6.8 + §12.4: requires_any-style
		// first-success-wins across providers; the LAST failure propagates
		// if any provider extracted-but-failed and no other provider
		// succeeded.
		return f.evaluateAllowMissing(headers)
	case reqAllowMissingOrFailed:
		// Any outcome → OK. Useful for validate-and-forward-for-downstream
		// patterns where downstream consumers make the auth decision.
		return evalResult{allowed: true}
	}
	return evalResult{allowed: false, err: fmt.Errorf("jwt_authn: unknown requirement kind %d", req.kind)}
}

// evaluateAllowMissing implements the `reqAllowMissing` discipline per SPEC
// §6.8 + §11.P16 + §12.4 + planner-time decision 4. Iterates ALL providers in
// the listener-level `compiledConfig.providers` map; for each, runs
// `evaluateProvider` with the provider's OWN audiences. Three outcomes:
//
//  1. NO provider's extraction matched any token → missing-OK → allowed=true.
//  2. Some provider's extraction matched + that provider validated → first-
//     success-wins; returns that success.
//  3. Some provider's extraction matched + ALL providers failed validation →
//     last failure propagates (present-and-invalid → FAIL).
//
// `ErrJwtMissed` failures from individual providers are NOT propagated as
// failures here — they mean "this provider did not extract a token", which
// is OK in the allow_missing semantic. Only NON-`ErrJwtMissed` failures
// indicate "token extracted but failed validation", which is the FAIL case.
//
// Map iteration order over `f.activeRC.providers` is non-deterministic per
// Go's map semantic — the "first-success-wins" + "last-failure-wins" rules
// must hold regardless of iteration order, which they do (success is
// definitive; failure is order-independent because all failures are equally
// "last-or-only").
func (f *filter) evaluateAllowMissing(headers http.Header) evalResult {
	var lastNonMissedErr error
	anyTokenExtracted := false
	for _, p := range f.activeRC.providers {
		if len(extractTokens(p, headers)) == 0 {
			continue
		}
		anyTokenExtracted = true
		r := f.evaluateProvider(p, headers, p.audiences)
		if r.allowed {
			return r
		}
		// Failure: only treat as a propagation-worthy error if it's NOT
		// ErrJwtMissed (which would only surface here if the extraction
		// returned a token but evaluateProvider's internal recheck found it
		// empty — defensive; in practice ErrJwtMissed only fires when no
		// tokens extract).
		if r.err != nil && !errors.Is(r.err, jwt.ErrJwtMissed) {
			lastNonMissedErr = r.err
		}
	}
	if !anyTokenExtracted {
		// Missing-OK per §11.P16: JWT absent → allowed.
		return evalResult{allowed: true}
	}
	if lastNonMissedErr != nil {
		return evalResult{allowed: false, err: lastNonMissedErr}
	}
	// Defensive: tokens extracted but no non-missed error surfaced; treat as
	// allowed (this should be unreachable in practice).
	return evalResult{allowed: true}
}

// evaluateProvider runs the per-provider validation pipeline per SPEC §6.8:
//
//  1. Extract tokens via `extractTokens(p, headers)`. Empty → `ErrJwtMissed`.
//  2. For each extracted token (first-success-wins):
//     a. `jwt.Parse(et.raw)` — 3-part header.payload.signature decode.
//     b. Resolve the JWKS:
//     - LocalJwks: direct `p.localJwks`.
//     - RemoteJwks: `p.jwksFetcher.Get(ctx)`; on failure +1 `jwksFetchFailed`.
//     c. `keyset.Lookup(t.Kid, t.Alg)` — kid+alg pickKey per Envoy.
//     d. `t.VerifySignature(key, t.Alg)` — RS+ES 6-alg dispatch.
//     e. `t.ValidateClaims({Issuer, Audiences: effectiveAudiences, ClockSkew, Now})`.
//  3. On any step failure within a single token, capture the error and
//     continue to the next extracted token. After all tokens exhausted,
//     return the last failure error.
//
// `effectiveAudiences` is either `p.audiences` (provider_name variant) OR the
// per-rule `audOverr` (provider_and_audiences variant) per the caller.
//
// JWKS fetch counter semantics (per §11.P6 + ADR-0154 §Decision (vii), Task-13
// amendment): `jwksFetchSuccess` is credited ONCE per RemoteJwks provider at
// filter-load time in `buildCompiledConfig` (mirroring reference Envoy's
// load-time-credit semantic), NOT on each request-time `Get()`. This function
// therefore touches ONLY `jwksFetchFailed` — incremented on a `Get()` error.
func (f *filter) evaluateProvider(p *compiledProvider, headers http.Header, effectiveAudiences []string) evalResult {
	tokens := extractTokens(p, headers)
	if len(tokens) == 0 {
		return evalResult{allowed: false, err: jwt.ErrJwtMissed}
	}

	var lastErr error = jwt.ErrJwtMissed
	for _, et := range tokens {
		t, err := jwt.Parse(et.raw)
		if err != nil {
			lastErr = err
			continue
		}

		// Resolve the JWKS. LocalJwks XOR RemoteJwks per
		// buildCompiledProvider's PARSE-REJECT on neither-set.
		var keyset *jwks.JWKSet
		switch {
		case p.localJwks != nil:
			keyset = p.localJwks
		case p.jwksFetcher != nil:
			ks, gerr := p.jwksFetcher.Get(context.Background())
			if gerr != nil {
				// Per SPEC §6.8 + §11.P6 + Task-13 empirical reference scrape:
				// counter increment on Get failure. jwks_fetch_success is
				// emitted at config-load time per buildCompiledConfig (NOT on
				// each Get cache hit) — see Task-13 stat-surface refinement at
				// jwtauthn.go.
				if f.activeRC != nil && f.activeRC.stats != nil {
					f.activeRC.stats.jwksFetchFailed.Inc()
				}
				lastErr = gerr
				continue
			}
			keyset = ks
		default:
			// Defensive: buildCompiledProvider PARSE-REJECTs the neither-set
			// case. Surface verification-fail rather than panic if some path
			// produced an invalid *compiledProvider (test-helper foot-gun).
			lastErr = jwt.ErrJwtVerificationFail
			continue
		}

		key, kerr := keyset.Lookup(t.Kid, t.Alg)
		if kerr != nil {
			lastErr = kerr
			continue
		}

		if verr := t.VerifySignature(key, t.Alg); verr != nil {
			lastErr = verr
			continue
		}

		if cerr := t.ValidateClaims(jwt.ValidateOptions{
			Issuer:    p.issuer,
			Audiences: effectiveAudiences,
			ClockSkew: p.clockSkew,
			Now:       f.nowOrDefault(),
		}); cerr != nil {
			lastErr = cerr
			continue
		}

		// Success: first-success-wins per SPEC §6.8.
		return evalResult{allowed: true, token: t, provider: p}
	}
	return evalResult{allowed: false, err: lastErr}
}

// requiresAnyErrPriority classifies a child-error from requires_any iteration
// into a priority class for the most-informative-error selection used by
// mergeRequiresAnyErr. Higher value = more informative.
//
//   - 3: JWT-semantic errors (the provider extracted + verified the token
//     against its JWKS but failed claim validation — expired, verification
//     fails, audience not allowed, unknown issuer, not-yet-valid, etc.).
//     These carry the user-actionable reason for rejection.
//   - 2: JWKS lookup miss (the provider has no key matching the token's
//     kid/alg) — informative but less so than JWT-semantic errors.
//   - 1: JWT-missing (the provider's extraction sources produced no token).
//   - 0: nil / unknown error.
func requiresAnyErrPriority(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, jwt.ErrJwtMissed) {
		return 1
	}
	if errors.Is(err, jwks.ErrJwksKidAlgMismatch) {
		return 2
	}
	// All other JWT-semantic errors (expired, verification-fail, audience,
	// issuer, not-yet-valid, format, etc.) — most informative.
	return 3
}

// mergeRequiresAnyErr selects the more-informative error between two children
// of a requires_any. On tie, keeps the EARLIER error (first-of-priority-class
// wins) — mirrors Envoy verifier.cc's first-encountered preserve discipline.
func mergeRequiresAnyErr(cur, next error) error {
	if cur == nil {
		return next
	}
	if next == nil {
		return cur
	}
	if requiresAnyErrPriority(next) > requiresAnyErrPriority(cur) {
		return next
	}
	return cur
}
