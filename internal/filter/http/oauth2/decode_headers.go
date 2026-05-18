package oauth2

// decode_headers.go — DecodeHeaders dispatcher + 3-of-4-emission-category
// handlers (handleUnauthenticated for (a), handlePassThrough for the bypass,
// handleValidCookies for the ContinueDecoding path, handleRefreshFailure for
// the (a)-on-refresh-failure leg) per phase-20 SPEC §6.3 + §4.1 + ADR-0180.
//
// The remaining handlers live in sibling files:
//
//   - handleCallback + applyTokenEndpointResponse + handleBadState — callback.go
//   - handleSignout (category (c)) — signout.go (Task 9)
//   - handleRefresh (silent refresh-token POST) — callback.go skeleton +
//     full body at Task 8 per ADR-0183
//
// # Dispatch priority order per SPEC §6.3
//
//	DecodeHeaders(headers, endStream):
//	  1. if signoutPath matches     → handleSignout       — category (c) 302
//	  2. if redirectPathMatcher hit → handleCallback      — async token POST
//	     (POST method PARSE-REJECTs at the callback entry per SPEC §2.14 + D15)
//	  3. if passThroughMatcher hit  → handlePassThrough   — bypass + counter
//	  4. else                       → handleCookieValidate
//	     - valid envelope                                  → ContinueDecoding
//	       (with optional Authorization-header injection per forward_bearer_token)
//	     - expired BearerToken + valid RefreshToken       → handleRefresh
//	     - else                                            → handleUnauthenticated
//
// # 4-emission-category wire shape per SPEC §4.1 + §4.5 + AMEND-3
//
// (a) 302 auth-challenge      — empty body; Location: <auth_endpoint URL>; state cookie SET; envelope cookies CLEARED
// (b) 302 post-callback       — empty body; Location: <redirect_uri>; 5-cookie envelope SET
// (c) 302 sign-out            — empty body; Location: <signout target>; Max-Age=0 for all 5 cookies
// (d) 401 with constant body  — body: "OAuth flow failed." (18 bytes); addFlowCookieDeletionHeaders
//
// NO 500 anywhere per AMEND-3 + §20.P9 REFUTED. Constant 401 body sourced from
// upstream `UnauthorizedBodyMessage` (18 bytes ASCII; no trailing newline).
//
// # POST-callback method PARSE-REJECT per SPEC §2.14 + D15
//
// envoy-go-strict simplification: GET-only callback at MVP. POST callbacks
// (the `response_mode=form_post` OAuth-extension variant) emit category (d)
// 401 with the constant body at the callback-flow entry point in
// DecodeHeaders. The envoy-go-strict departure is recorded at
// BEHAVIOR_CONTRACT.md per phase-20 SPEC §13 item C8.
//
// # No-panic discipline
//
// All dispatch paths tolerate arbitrary header values. The cookie-validate
// path consumes hmac.validateHMAC + cookies.parseAllCookies, both of which
// are stdlib-driven + fuzz-hardened. The decryptToken AMEND-3 fall-back
// returns ciphertext-as-plaintext on any decrypt failure (no panic).

import (
	"net/http"
	"strconv"
	"time"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// constUnauthorizedBody is the (d) 401 constant body per phase-20 AMEND-3 +
// SPEC §4.3 + §20.P9. Byte-exact 18 ASCII bytes; no trailing newline. Sourced
// from upstream `UnauthorizedBodyMessage` constant.
const constUnauthorizedBody = "OAuth flow failed."

// DecodeHeaders is the dispatcher entry point per phase-20 SPEC §6.3 +
// ADR-0180. Routes the inbound request to one of 4 emission-category
// handlers based on the dispatch priority order.
//
// Returns StopIteration when the dispatch path synthesizes a deny-emission
// (categories (a) / (c) / (d)) OR when it parks the decode goroutine on an
// async token_endpoint POST (callback flow + refresh-token flow). Returns
// Continue ONLY on the cookie-validate happy path (a valid envelope passes
// through to the upstream).
//
// Per SPEC §6.3 + planner-time D15 the POST-callback method PARSE-REJECT
// happens INSIDE this method (when the path matches redirectPathMatcher
// AND the method is POST, emit category (d) 401).
//
// Per ADR-0085 nil-tolerance — if f.cc is nil (test code path that did not
// supply a compiledConfig), returns Continue to avoid a nil-deref. Production
// code paths always carry a non-nil cc per ADR-0072 boot-time discipline.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	cc := f.cc
	if cc == nil {
		return envoyhttp.Continue
	}

	// Extract pseudo-headers via the http.Header.Get convention (mirrors
	// extauthz.go:999 + jwtauthn.go:759 + rbac.go:951). Per the framework's
	// incoming-header normalization, the `:path` + `:method` pseudo-headers
	// are stored verbatim (NOT canonicalized to capital-first per
	// http.CanonicalHeaderKey).
	path := getHeaderValue(headers, ":path")
	method := getHeaderValue(headers, ":method")

	// (1) signout_path match → handleSignout (category (c) 302)
	// Per ADR-0184 the headers carrier is threaded through so the
	// denyRedirectMatcher (a headerMatcher returning the (url, ok) tuple)
	// can read request headers to pick the post-sign-out Location.
	if cc.signoutPath != nil && cc.signoutPath(path) {
		return f.handleSignout(headers)
	}

	// (2) redirectPathMatcher match → handleCallback
	//     (POST method PARSE-REJECTs per SPEC §2.14 + D15 — category (d) 401)
	if cc.redirectPathMatcher != nil && cc.redirectPathMatcher(path) {
		if method == "POST" {
			// POST callback PARSE-REJECT per SPEC §2.14 + AMEND-3 envoy-go-
			// strict departure. Per literal planner-time D15: "NO new counter
			// (the standard `oauth_unauthorized_rq` increments per §4.6)" —
			// the standard counter SHOULD increment on BOTH the bad-state-
			// cookie path AND the POST-callback PARSE-REJECT path. Route to
			// handleBadState (which increments oauth_unauthorized_rq) per
			// literal D15. The Task 8 commit df33e4b introduced a transient
			// handleBadStateNoCounter call-site split which was reverted in a
			// Task 8 follow-up; both PARSE-REJECT triggers now flow through
			// handleBadState per literal D15.
			return f.handleBadState()
		}
		return f.handleCallback(headers)
	}

	// (3) passThroughMatcher hit → handlePassThrough (bypass + counter)
	if cc.passThroughMatcher != nil && cc.passThroughMatcher(headerView(headers)) {
		return f.handlePassThrough()
	}

	// (4) else handleCookieValidate → ContinueDecoding OR handleRefresh OR handleUnauthenticated
	return f.handleCookieValidate(headers)
}

// handleSignout has moved to signout.go per phase-20 IMPL Task 9 + ADR-0184.
// The Task 5 SKELETON's no-headers entry shape (`f.handleSignout()`) was
// replaced by the Task 9 headers-threading signature
// (`f.handleSignout(headers)`) so the denyRedirectMatcher closure can read
// the inbound request headers to pick the post-sign-out Location URL per
// SPEC §4.4 category (c) three-tier cascade.
//
// handleUnauthenticated emits the category (a) 302 auth-challenge per
// phase-20 SPEC §4.1 + §6.6 + ADR-0180. Wire shape:
//
//   - Status: 302
//   - Body:   empty
//   - Headers:
//   - Location: <authorization_endpoint URL>     — SPEC §4.4
//   - Set-Cookie: <state cookie SET to HMAC(state)> — SPEC §4.5
//   - Set-Cookie: <BearerToken=; Max-Age=0; ...> (×4 envelope cookies cleared)
//
// Task 5 lands the SKELETON wire-shape emission with placeholder values for
// the state-cookie payload + the authorization-endpoint URL. Task 8 + Task
// 10 wire the full RFC 6749 §4.1.1 URL composition (response_type + client_id
// + redirect_uri + state + scope + resource) + the byte-exact state-cookie
// payload per SPEC §12 item A3 RATIFIED-PENDING-IMPL-TIME.
func (f *filter) handleUnauthenticated() envoyhttp.FilterHeadersStatus {
	cc := f.cc

	// Compose a placeholder state-cookie value. Task 8/10 finalize per
	// SPEC §12 item A3 + §4.4. Task 5 ships a non-empty placeholder so the
	// dispatcher test can assert the state cookie is SET on the wire.
	stateValue := f.composeStateCookieValueSkeleton()

	// Compose the authorization-endpoint URL placeholder. Task 10 wires the
	// full RFC 6749 §4.1.1 composition. Task 5 ships the raw endpoint URL
	// so the dispatcher test can assert the Location header is set.
	authorizationURL := cc.authorizationEndpoint

	hdrs := envoyhttp.OrderedHeaders{
		{Name: "Location", Value: authorizationURL},
	}

	// Emit the (a) auth-challenge cookie payload per SPEC §4.5: 4 envelope
	// cookies cleared (BearerToken / OauthHMAC / OauthExpires / RefreshToken)
	// + state cookie SET.
	cookieHeaders := make(http.Header)
	emitCategoryA_AuthChallenge(
		cookieHeaders,
		cc.cookieNames,
		cc.cookieAttrs,
		cc.stateCookieName,
		stateValue,
	)
	for _, v := range cookieHeaders["Set-Cookie"] {
		hdrs = append(hdrs, envoyhttp.HeaderField{Name: "Set-Cookie", Value: v})
	}

	if f.dcb != nil {
		f.dcb.SendLocalReply(302, "", hdrs)
	}
	return envoyhttp.StopIteration
}

// handlePassThrough bypasses the OAuth2 dispatch + increments the
// `oauth_passthrough` counter per phase-20 SPEC §4.6 + §6.3 step 3.
// Returns Continue — the request proceeds through the rest of the filter
// chain unchanged.
func (f *filter) handlePassThrough() envoyhttp.FilterHeadersStatus {
	if f.cc.stats != nil && f.cc.stats.oauthPassthrough != nil {
		f.cc.stats.oauthPassthrough.Inc()
	}
	return envoyhttp.Continue
}

// handleCookieValidate is the post-route-priority cookie-envelope validator
// per phase-20 SPEC §6.3 step 4. Routes to one of three terminal handlers:
//
//   - Valid envelope (HMAC validates + BearerToken not expired) →
//     ContinueDecoding via handleValidCookies (optional Authorization
//     header injection per forward_bearer_token=true)
//   - Expired BearerToken + valid RefreshToken (use_refresh_token=true) →
//     handleRefresh (silent refresh-token POST; lives in callback.go skeleton
//     at Task 5; full body at Task 8 per ADR-0183)
//   - else                                                                 →
//     handleUnauthenticated (category (a) 302 challenge)
//
// Per SPEC §6.4 the HMAC validation tuple is (domain, expires, token,
// id_token, refresh_token). domain == the request `Host` header (the
// authority for the request); expires + token + id_token + refresh_token
// come from the parsed cookie envelope.
func (f *filter) handleCookieValidate(headers http.Header) envoyhttp.FilterHeadersStatus {
	cc := f.cc

	env := parseAllCookies(headers, cc.cookieNames)
	// The HMAC validation domain is the request authority. HTTP/2 uses
	// `:authority`; HTTP/1.1 uses `Host`. Prefer `:authority` to match the
	// h2 convention; fall back to `Host`.
	domain := getHeaderValue(headers, ":authority")
	if domain == "" {
		domain = headers.Get("Host")
	}

	// HMAC validation per SPEC §6.4 + AMEND-2 5-input composition. The
	// hmacSecretFn closure returns the current SDS-loaded secret bytes; nil
	// closure means "no HMAC secret loaded" → fail validation.
	var secret []byte
	if cc.hmacSecretFn != nil {
		secret = cc.hmacSecretFn()
	}
	if !validateHMAC(domain, env.OauthExpires, env.BearerToken, env.IdToken, env.RefreshToken, secret, env.OauthHMAC) {
		// HMAC mismatch (or no envelope at all) → unauthenticated.
		return f.handleUnauthenticated()
	}

	// Envelope is HMAC-valid. Check BearerToken expiry.
	if isExpired(env.OauthExpires) {
		if cc.useRefreshToken && env.RefreshToken != "" {
			// Silent refresh leg — async token POST. Full body at Task 8.
			return f.handleRefresh(env)
		}
		// Expired + no refresh-token available → category (a) 302 challenge.
		return f.handleUnauthenticated()
	}

	// Valid envelope; not expired → ContinueDecoding (optional Authorization
	// injection per forward_bearer_token).
	return f.handleValidCookies(headers, env)
}

// handleValidCookies is the ContinueDecoding path per phase-20 SPEC §6.3
// step 4 (valid envelope leg). Optionally injects `Authorization: Bearer
// <decrypted>` per cc.forwardBearerToken + cc.preserveAuthorizationHeader
// per SPEC §2.15 + AMEND-6 C3.
//
// Per SPEC + AMEND-3 decryption-failure fall-back — decryptToken returns
// ciphertext-as-plaintext on any failure; the downstream upstream sees the
// fall-back bytes verbatim (the HMAC validation already passed at this
// point, so the fall-back path is unreachable in practice — the BearerToken
// cookie value MUST be a valid AES-CBC envelope for the HMAC to have
// validated against the upstream-decrypted plaintext).
func (f *filter) handleValidCookies(headers http.Header, env CookieEnvelope) envoyhttp.FilterHeadersStatus {
	cc := f.cc
	if !cc.forwardBearerToken {
		return envoyhttp.Continue
	}

	// Skip Authorization injection if an existing Authorization header is
	// present AND preserveAuthorizationHeader=true. Otherwise overwrite.
	if cc.preserveAuthorizationHeader && headers.Get("Authorization") != "" {
		return envoyhttp.Continue
	}

	// Decrypt the BearerToken cookie unless encryption is disabled.
	var plaintext []byte
	if cc.disableTokenEncryption {
		plaintext = []byte(env.BearerToken)
	} else {
		var secret []byte
		if cc.hmacSecretFn != nil {
			secret = cc.hmacSecretFn()
		}
		plaintext = decryptToken(env.BearerToken, secret)
	}

	// Set Authorization: Bearer <decrypted> on the request headers. Use
	// http.Header.Set so the canonical form ("Authorization") is the wire
	// key per the net/http convention.
	authValue := "Bearer " + string(plaintext)
	headers.Set("Authorization", authValue)

	return envoyhttp.Continue
}

// handleRefreshFailure emits the category (a) 302 challenge after a refresh-
// token POST non-2xx response per phase-20 SPEC §4.7 + §6.3 + ADR-0183.
// Increments `oauth_refreshtoken_failure` per SPEC §4.6.
//
// Distinct from handleUnauthenticated only in the counter increment — the
// wire-shape emission is identical (category (a) 302 challenge per §4.1).
// Task 5 lands the counter+emission pair; Task 8 wires the call-site from
// the refresh-token rotation continuation.
func (f *filter) handleRefreshFailure() envoyhttp.FilterHeadersStatus {
	if f.cc.stats != nil && f.cc.stats.oauthRefreshtokenFailure != nil {
		f.cc.stats.oauthRefreshtokenFailure.Inc()
	}
	return f.handleUnauthenticated()
}

// composeStateCookieValueSkeleton returns a placeholder state-cookie value
// for the (a) 302 auth-challenge per phase-20 SPEC §4.4 + §12 item A3
// RATIFIED-PENDING-IMPL-TIME. Task 5 SKELETON; Task 8 + Task 10 wire the
// full HMAC-protected payload composition.
//
// Skeleton shape per SPEC §12 item A3 anticipation: epoch-seconds prefix +
// HMAC. The skeleton emits ONLY the epoch-seconds prefix (no HMAC) — Task
// 8/10 append the HMAC; the dispatcher test asserts the cookie is SET on
// the wire (non-empty value), NOT byte-exact equality against the final-
// state payload.
func (f *filter) composeStateCookieValueSkeleton() string {
	// Task 5 skeleton: epoch-seconds-decimal-string per SPEC §12 item A3.
	// Task 8/10 wire the HMAC append + state-payload composition.
	return strconv.FormatInt(time.Now().Unix(), 10)
}

// isExpired returns true iff the OauthExpires cookie value (epoch-seconds-
// decimal-string per SPEC §12 item A3) is in the past. An empty / malformed
// expires value is treated as EXPIRED (the dispatcher falls back to the
// unauthenticated leg).
func isExpired(expiresValue string) bool {
	if expiresValue == "" {
		return true
	}
	epoch, err := strconv.ParseInt(expiresValue, 10, 64)
	if err != nil {
		return true
	}
	return time.Now().Unix() >= epoch
}

// getHeaderValue returns the first value for `name` from a raw
// http.Header header carrier. Mirrors net/http.Header.Get but
// without canonicalizing the lookup key (the framework's incoming headers
// arrive with lowercase pseudo-headers; this helper accepts whatever the
// caller supplies).
func getHeaderValue(h http.Header, name string) string {
	if vs, ok := h[name]; ok && len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// headerView adapts a raw http.Header to the headerLookup interface the
// compiled header-matcher predicate consumes per SPEC §6.2 +
// compiled_config.go. The adapter delegates to http.Header.Get which
// canonicalizes the lookup key via http.CanonicalHeaderKey — mirrors the
// stdlib convention so matchers written against net/http canonical names
// work uniformly.
type headerViewAdapter http.Header

// Get returns the first value for `name` per the headerLookup contract.
// Delegates to net/http canonicalization (X-Foo-Bar / x-foo-bar / X-FOO-BAR
// all resolve to the canonical X-Foo-Bar key).
func (h headerViewAdapter) Get(name string) string {
	return http.Header(h).Get(name)
}

// headerView constructs the headerLookup adapter for the matcher closure.
func headerView(h http.Header) headerLookup {
	return headerViewAdapter(h)
}
