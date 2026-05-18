package oauth2

// callback.go — callback-flow dispatcher (handleCallback) + post-token-POST
// continuation (applyTokenEndpointResponse) + bad-state-401 emitter
// (handleBadState) per phase-20 SPEC §6.8 + §4.1 + ADR-0180.
//
// # Surface
//
// Three filter methods per SPEC §6.8 + §4.1 category (b) + (d):
//
//   - handleCallback(headers): the callback-flow entry. Validates the state
//     cookie + extracts the authorization-code from the query string + parks
//     the decode goroutine on the phase-09 async-resume primitive while the
//     token_endpoint POST is in flight. Task 5 lands the SKELETON; Task 10
//     wires the actual outbound HTTP call.
//
//   - applyTokenEndpointResponse(resp, err): the async-resume continuation
//     fired when the token_endpoint POST completes. Per SPEC §6.8 + §4.7 +
//     AMEND-3:
//
//   - 2xx success → category (b) 302 post-callback (Task 8 + Task 10)
//   - 5xx retry-eligible → category (a) 302 challenge
//   - 4xx terminal → category (d) 401 with constant body
//
//     Task 5 lands the SKELETON shape; Task 8 (refresh leg) + Task 10
//     (token POST) wire the full continuation body.
//
//   - handleBadState(): the category (d) 401-with-constant-body emitter for
//     the bad-state-cookie path AND the POST-callback-method PARSE-REJECT
//     path per SPEC §2.14 + §4.3 + AMEND-3 + planner-time D15.
//
//     Wire shape:
//       - Status: 401
//       - Body:   "OAuth flow failed." (18 bytes; no trailing newline)
//       - Headers: full-envelope clearing via addFlowCookieDeletionHeaders
//
//     NO 500 anywhere per AMEND-3 + §20.P9 REFUTED.
//
// # NO 500 anywhere discipline (AMEND-3 + §20.P9)
//
// The 4-emission-category wire shape is the COMPLETE set of synthesized
// responses oauth2 may emit. NO 500 path exists anywhere in the filter —
// upstream errors (token_endpoint 5xx) map to (a) 302 retry-challenge;
// upstream-terminal errors (token_endpoint 4xx) map to (d) 401 with constant
// body; configuration-rejection (POST callback) maps to (d) 401 with constant
// body. The discipline is enforced by the absence of any 500 emission in
// the codebase — Task 12's fixture-0024 cross-side fixtures assert ALL the
// 4 emission categories byte-exact, and a regression that emits 500 would
// surface at fixture-0024 first.
//
// # POST-callback PARSE-REJECT per SPEC §2.14 + planner-time D15
//
// envoy-go-strict simplification: GET-only callback at MVP. The
// `response_mode=form_post` OAuth-extension POST variant is out of scope
// per SPEC §2.14; the dispatcher (decode_headers.go::DecodeHeaders) routes
// the POST + redirect-path-match case to handleBadState (category (d) 401)
// per planner-time D15. The envoy-go-strict departure is recorded at
// BEHAVIOR_CONTRACT.md per SPEC §13 item C8.
//
// # State-cookie validation (SPEC §6.6 + §4.4 RATIFIED-PENDING-IMPL-TIME)
//
// The callback request's `state` query-parameter MUST equal the value of
// the state cookie set in the prior category (a) 302 challenge. Mismatch
// → category (d) 401 via handleBadState; match → continue with the token
// POST. The byte-exact state-cookie payload shape is RATIFIED-PENDING-IMPL-
// TIME per SPEC §12 item A3 + ADR-0180; Task 5 lands the skeleton that
// compares strings; Task 8 + Task 10 wire the HMAC-protected composition.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// handleCallback is the callback-flow entry per phase-20 SPEC §6.8 +
// ADR-0180.
//
//  1. Extract `code` + `state` query parameters from the request path.
//  2. Validate the state-cookie matches the `state` query-parameter
//     (skeleton — Task 8/10 wire the HMAC-protected validation).
//  3. On state mismatch → handleBadState (category (d) 401).
//  4. On state match → park the decode goroutine on the async-resume
//     primitive + launch the outbound token_endpoint POST goroutine via
//     cc.tokenEndpointPoster (mirrors handleRefresh's mu/done discipline).
//     On completion, the resume goroutine fires applyTokenEndpointResponse.
//
// Returns StopIteration synchronously; the resume goroutine resumes the
// parked decode via dcb.SendLocalReply / dcb.ContinueDecoding from
// applyTokenEndpointResponse.
//
// # Task 12 follow-up wire-up (closes Task 5 SKELETON)
//
// Task 5 shipped the SKELETON shape (state-cookie validation only +
// StopIteration). Task 10 landed `postTokenEndpoint`+ `buildTokenRequestBody`
// + the `tokenEndpointPoster` closure populated at Task 11's
// buildCompiledConfig. This wire-up closes the structural gap so the auth-
// code sign-in flow is end-to-end functional per SPEC §6.8 + §4.5 category
// (b). Mirrors handleRefresh's async-resume goroutine pattern (callback.go
// landed at Task 8) with grantType=authorization_code instead of
// refresh_token.
func (f *filter) handleCallback(headers http.Header) envoyhttp.FilterHeadersStatus {
	cc := f.cc

	// Extract the request path + parse the query string for `code` + `state`.
	path := headers.Get(":path")
	codeParam, stateParam := extractCallbackParams(path)

	// State-cookie validation per SPEC §6.6 + §4.4. Byte-equality compare
	// against the stored state cookie. The HMAC-protected state-cookie
	// payload (per SPEC §12 item A3) is a future tightening; the byte-
	// equality compare suffices for the dispatcher invariant.
	stateCookieValue := f.lookupStateCookie(headers)
	if !validateStateCookie(stateCookieValue, stateParam) {
		// Bad state → category (d) 401 + flow-cookie cleanup per AMEND-3.
		return f.handleBadState()
	}

	// Per-request cancellable context per ADR-0159 D4 precedent. OnDestroy
	// cancels via f.callCancel to abort the in-flight POST promptly.
	callCtx, callCancel := context.WithCancel(context.Background())

	// callbackDone channel: closed by the resume goroutine when
	// applyTokenEndpointResponse returns. Tests use waitCallbackDone() to
	// synchronize without sleeping; production code paths do NOT need to
	// wait (the Envoy chain resumes via dcb.ContinueDecoding /
	// dcb.SendLocalReply from the resume goroutine).
	done := make(chan struct{})

	f.mu.Lock()
	if f.done {
		// OnDestroy fired before we got here — abort without launching the
		// goroutine. Mirrors handleRefresh's guard.
		f.mu.Unlock()
		callCancel()
		close(done)
		return envoyhttp.StopIteration
	}
	f.callCancel = callCancel
	f.callbackDone = done
	// Snapshot the poster + secrets under the lock per ADR-0183 +
	// handleRefresh's defense-in-depth discipline.
	poster := cc.tokenEndpointPoster
	clientID := cc.clientID
	var clientSecret []byte
	if cc.clientSecretFn != nil {
		clientSecret = cc.clientSecretFn()
	}
	f.mu.Unlock()

	if poster == nil {
		// No poster configured — treat as transport-level failure per the
		// AMEND-3 deny-path discipline (mirrors handleRefresh's nil-poster
		// guard at Task 8). Routes through applyTokenEndpointResponse →
		// category (a) 302 challenge.
		go func() {
			defer close(done)
			f.applyTokenEndpointResponse(nil, errNoPosterConfigured)
		}()
		return envoyhttp.StopIteration
	}

	// Launch the async goroutine — mirrors handleRefresh + phase-09 fault +
	// extauthz async-resume pattern. StopIteration returned synchronously;
	// goroutine performs the POST; on completion the goroutine fires
	// applyTokenEndpointResponse which either SendLocalReply+Continue
	// (success → category (b) 302) or SendLocalReply+Continue (failure-leg).
	go func() {
		defer close(done)
		resp, err := poster(callCtx, grantTypeAuthorizationCode, codeParam, clientID, string(clientSecret))
		f.applyTokenEndpointResponse(resp, err)
	}()

	return envoyhttp.StopIteration
}

// tokenEndpointResponse is the parsed JSON body returned by the OAuth2
// token_endpoint per RFC 6749 §4.1.4 + §5.1. Only the 4 fields needed by
// the auth-code success leg are decoded; extra fields (token_type / scope)
// are tolerated + ignored per the JSON unmarshal contract.
type tokenEndpointResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// applyTokenEndpointResponse is the post-token-POST continuation per phase-20
// SPEC §6.8 + §4.5 category (b) + §4.7 + ADR-0180 + AMEND-3. Called from
// the resume goroutine launched by handleCallback.
//
// Per SPEC §4.5 + §4.7 + AMEND-3 the disposition is:
//
//   - 2xx success         → category (b) 302 post-callback-success
//     (Set-Cookie envelope + Location: redirect_uri) +
//     oauth_success++
//   - 2xx malformed body  → category (a) 302 challenge (treated as transient
//     failure; the recovered envelope cannot be HMAC-
//     validated downstream so a 302 challenge is the
//     deny-path classification per AMEND-3)
//   - 5xx retry-eligible  → category (a) 302 challenge (re-authenticate
//     from scratch) — NO counter increment on this
//     leg per planner-time D5 (oauth_unauthorized_rq
//     already implicitly via the (a) emission semantic;
//     applyTokenEndpointResponse does NOT bump
//     oauth_unauthorized_rq from here per AMEND-3 +
//     §4.6 — only handleBadState does)
//   - 4xx terminal        → category (d) 401 with constant body +
//     oauth_failure++
//   - transport-level err → category (a) 302 challenge (mirrors 5xx)
//
// Per ADR-0159 D4 + OnDestroy guard: acquires f.mu before touching any
// callback or stream state; short-circuits if f.done is set.
func (f *filter) applyTokenEndpointResponse(resp *http.Response, err error) {
	// Drain + close the response body in all paths to avoid connection leak
	// per the net/http documentation. We need to read the body to parse the
	// JSON success-leg AND to drain the failure-leg body fully.
	var bodyBytes []byte
	if resp != nil && resp.Body != nil {
		bodyBytes, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		// OnDestroy fired before the continuation — abort without touching
		// dcb. Counter increments + state mutations also abort (the request
		// is gone; the upstream that would have observed the deferred
		// Set-Cookie never gets to encode-side).
		return
	}

	cc := f.cc

	// Classify the disposition per SPEC §4.5 + §4.7 + AMEND-3.
	switch {
	case err != nil:
		// Transport-level error → category (a) 302 challenge per AMEND-3.
		f.emitCategoryA_AuthChallengeLocked()
	case resp == nil:
		// Defensive: nil response without err is anomalous; route to (a).
		f.emitCategoryA_AuthChallengeLocked()
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// 2xx — parse JSON body + emit (b) post-callback-success.
		var tr tokenEndpointResponse
		if jerr := json.Unmarshal(bodyBytes, &tr); jerr != nil || tr.AccessToken == "" {
			// Malformed body OR missing access_token → treat as transient
			// failure (category (a) 302 challenge).
			f.emitCategoryA_AuthChallengeLocked()
			break
		}
		f.emitCategoryB_PostCallbackLocked(tr)
		if cc.stats != nil && cc.stats.oauthSuccess != nil {
			cc.stats.oauthSuccess.Inc()
		}
	case resp.StatusCode >= 500 && resp.StatusCode < 600:
		// 5xx retry-eligible → category (a) 302 challenge per AMEND-3.
		f.emitCategoryA_AuthChallengeLocked()
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		// 4xx terminal → category (d) 401 + oauth_failure++.
		if cc.stats != nil && cc.stats.oauthFailure != nil {
			cc.stats.oauthFailure.Inc()
		}
		_ = f.emitCategoryD401()
	default:
		// Any other non-2xx (e.g. 3xx) → treat as retry-eligible per
		// AMEND-3 deny-path simplification.
		f.emitCategoryA_AuthChallengeLocked()
	}

	// Resume the parked decode goroutine per the extauthz SendLocalReply +
	// ContinueDecoding pattern. SendLocalReply alone sets c.localReplyDone
	// but does NOT unblock the park.
	if f.dcb != nil {
		f.dcb.ContinueDecoding()
	}
}

// emitCategoryA_AuthChallengeLocked emits the category (a) 302 wire shape
// inline. Caller MUST hold f.mu. Mirrors handleUnauthenticated's wire
// emission shape (decode_headers.go) — the only difference is the call-site
// (this one fires from the resume goroutine, NOT the dispatcher entry).
func (f *filter) emitCategoryA_AuthChallengeLocked() {
	cc := f.cc

	stateValue := f.composeStateCookieValueSkeleton()
	authorizationURL := cc.authorizationEndpoint

	hdrs := envoyhttp.OrderedHeaders{
		{Name: "Location", Value: authorizationURL},
	}
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
}

// emitCategoryB_PostCallbackLocked emits the category (b) 302 post-callback-
// success wire shape per SPEC §4.5 category (b). Caller MUST hold f.mu.
//
// Wire shape per §4.5:
//
//   - Status: 302
//   - Body:   empty
//   - Headers:
//     Location: <redirect_uri>
//     Set-Cookie: BearerToken=<AES-CBC(access_token)>; ...        (always)
//     Set-Cookie: OauthHMAC=<HMAC over envelope>; ...             (always)
//     Set-Cookie: OauthExpires=<epoch-seconds>; ...               (always)
//     Set-Cookie: RefreshToken=<AES-CBC(refresh_token)>; ...      (when present)
//
// Per AMEND-1 + ADR-0182 BearerToken + RefreshToken values are AES-256-CBC-
// encrypted before emission UNLESS disable_token_encryption=true (proto
// default false; MVP always encrypts per compiled_config.go).
//
// The HMAC tuple is (domain, expires, encAccessToken, idToken,
// encRefreshToken) per SPEC §6.4 + AMEND-2 5-input composition. `domain`
// is the empty string at this call-site — the HMAC validation site
// (handleCookieValidate) uses the request authority; on the callback's
// upstream-bound redirect we have no authority context to anchor against,
// so the issued HMAC is consistent with a host-only cookie's
// implicit-domain semantic (the browser sets the cookie on the redirect-
// target host; subsequent requests to that host produce the same domain
// input via the request authority).
func (f *filter) emitCategoryB_PostCallbackLocked(tr tokenEndpointResponse) {
	cc := f.cc

	// Resolve the HMAC secret + encrypt tokens per AMEND-1 + ADR-0182.
	var secret []byte
	if cc.hmacSecretFn != nil {
		secret = cc.hmacSecretFn()
	}

	var encAccessToken, encRefreshToken string
	if cc.disableTokenEncryption {
		encAccessToken = tr.AccessToken
		encRefreshToken = tr.RefreshToken
	} else {
		encAccessToken = encryptToken([]byte(tr.AccessToken), secret)
		if tr.RefreshToken != "" {
			encRefreshToken = encryptToken([]byte(tr.RefreshToken), secret)
		}
	}

	// Compute the expires epoch per SPEC §12 item A3 — epoch-seconds-decimal-
	// string. Prefer the upstream-supplied `expires_in`; fall back to the
	// operator-configured `default_expires_in` per RFC 6749 §5.1 (the
	// `expires_in` is RECOMMENDED but NOT required).
	expiresIn := tr.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int64(cc.defaultExpiresIn / time.Second)
	}
	expiresEpoch := time.Now().Add(time.Duration(expiresIn) * time.Second).Unix()
	expiresStr := formatExpiresValue(expiresEpoch)

	// Compute the 5-input HMAC over the envelope per SPEC §6.4 + AMEND-2 +
	// ADR-0179. Domain at this call-site is the empty string per the
	// docstring rationale above (no authority context on the callback's
	// upstream-bound redirect).
	hmacValue := computeHMAC("", expiresStr, encAccessToken, tr.IDToken, encRefreshToken, secret)

	// Emit the (b) post-callback Set-Cookie envelope per §4.5 category (b).
	cookieHeaders := make(http.Header)
	emitCategoryB_PostCallback(
		cookieHeaders,
		cc.cookieNames,
		cc.cookieAttrs,
		encAccessToken,
		hmacValue,
		expiresEpoch,
		encRefreshToken,
	)

	// Compose the final header set: Location + Set-Cookie envelope.
	hdrs := envoyhttp.OrderedHeaders{
		{Name: "Location", Value: cc.redirectURI},
	}
	for _, v := range cookieHeaders["Set-Cookie"] {
		hdrs = append(hdrs, envoyhttp.HeaderField{Name: "Set-Cookie", Value: v})
	}

	if f.dcb != nil {
		f.dcb.SendLocalReply(302, "", hdrs)
	}
}

// handleRefresh is the silent refresh-token POST dispatcher per phase-20
// SPEC §6.3 step 4 + §6.8 + ADR-0183. Task 8 implementation.
//
// Per SPEC + ADR-0183: the refresh leg parks the decode goroutine on the
// phase-09 async-resume primitive, POSTs to the token_endpoint with the
// 3-field refresh-token template per AMEND-5, then on 2xx CONTINUES the
// request with the deferred Set-Cookie envelope (the upstream response
// gets the new envelope cookies set on its way back to the client). On
// non-2xx (incl. transport-level error) falls through to handleRefreshFailure
// per SPEC §4.7 + AMEND-3.
//
// Per planner-time D14 + ADR-0183: NO per-stream serialization. Concurrent
// in-flight requests with the same expired BearerToken + valid RefreshToken
// each POST refresh INDEPENDENTLY; the latest Set-Cookie wins via the
// browser's standard cookie-overwrite semantic.
//
// The deferred Set-Cookie envelope is recorded on f.pendingSetCookies under
// f.mu; Task 11 (full filter integration) wires the encode-side emission
// onto the upstream response. Task 8 returns StopIteration synchronously +
// the resume goroutine fires applyRefreshTokenResponse on POST completion.
func (f *filter) handleRefresh(env CookieEnvelope) envoyhttp.FilterHeadersStatus {
	cc := f.cc

	// Per-request cancellable context per ADR-0159 D4 precedent. OnDestroy
	// cancels via f.callCancel to abort the in-flight POST promptly.
	callCtx, callCancel := context.WithCancel(context.Background())

	// refreshDone channel: closed by the resume goroutine when
	// applyRefreshTokenResponse returns. Tests use waitRefreshDone() to
	// synchronize without sleeping; production code paths do NOT need to
	// wait (the Envoy chain resumes via dcb.ContinueDecoding /
	// dcb.SendLocalReply called from the resume goroutine).
	done := make(chan struct{})

	f.mu.Lock()
	if f.done {
		// OnDestroy fired before we got here — abort without launching the
		// goroutine. The dispatcher's downstream filter chain would have
		// reaped the stream regardless; the dispatch path is safe-to-
		// short-circuit.
		f.mu.Unlock()
		callCancel()
		close(done)
		return envoyhttp.StopIteration
	}
	f.callCancel = callCancel
	f.refreshDone = done
	// Snapshot the poster + tokens under the lock to avoid races with a
	// hypothetical Task 11 hot-reload of compiledConfig (defense-in-depth;
	// MVP has no such reload, but the snapshot discipline is cheap).
	poster := cc.tokenEndpointPoster
	clientID := cc.clientID
	var clientSecret []byte
	if cc.clientSecretFn != nil {
		clientSecret = cc.clientSecretFn()
	}
	refreshToken := env.RefreshToken
	f.mu.Unlock()

	if poster == nil {
		// No poster configured — treat as transport-level failure per the
		// AMEND-3 deny-path discipline. The dispatcher tests at Task 5
		// don't reach this path (they short-circuit at the dispatcher);
		// only the Task 8 tests reach here, and they always inject a
		// poster. Belt-and-braces guard against a misconfigured Task 11
		// boot path.
		go func() {
			defer close(done)
			f.applyRefreshTokenResponse(nil, errNoPosterConfigured)
		}()
		return envoyhttp.StopIteration
	}

	// Launch the async goroutine — mirrors phase-09 fault + extauthz
	// async-resume pattern: StopIteration returned synchronously;
	// goroutine performs the call; on completion the goroutine fires
	// applyRefreshTokenResponse which either ContinueDecoding (success-
	// leg) or SendLocalReply + ContinueDecoding (failure-leg).
	go func() {
		defer close(done)
		resp, err := poster(callCtx, grantTypeRefreshToken, refreshToken, clientID, string(clientSecret))
		f.applyRefreshTokenResponse(resp, err)
	}()

	return envoyhttp.StopIteration
}

// grantTypeRefreshToken is the OAuth 2.0 `grant_type` value for the refresh-
// token POST template per RFC 6749 §6 + AMEND-5 + ADR-0185.
const grantTypeRefreshToken = "refresh_token"

// errNoPosterConfigured is the synthetic transport-level error surfaced when
// compiledConfig.tokenEndpointPoster is nil. The dispatcher routes the
// failure through handleRefreshFailure per SPEC §4.7 + AMEND-3 — operators
// see a category (a) 302 challenge (re-authenticate from scratch) rather
// than a hard failure.
var errNoPosterConfigured = &posterConfigError{msg: "oauth2: tokenEndpointPoster not configured"}

// posterConfigError is a sentinel error type for the no-poster-configured
// fail-fast path. Implements the `error` interface.
type posterConfigError struct{ msg string }

func (e *posterConfigError) Error() string { return e.msg }

// applyRefreshTokenResponse is the post-refresh-POST continuation per phase-
// 20 SPEC §6.8 + §4.6 + ADR-0183 + AMEND-3 + planner-time D14. Called from
// the resume goroutine launched by handleRefresh.
//
// Per SPEC §4.6 counter increment matrix:
//   - 2xx success → ContinueDecoding + deferred Set-Cookie envelope on
//     f.pendingSetCookies + `oauth_refreshtoken_success++`
//   - non-2xx + transport-level error → category (a) 302 challenge via
//     handleRefreshFailure + `oauth_refreshtoken_failure++`
//
// Per AMEND-3 + §4.6: NO `oauth_failure` increment on refresh-failure
// (one-counter-per-event invariant; refresh-failure is distinct from the
// callback-leg's terminal-failure path).
//
// Per ADR-0159 D4 + Task 5 OnDestroy guard: acquires f.mu before touching
// any callback or stream state; short-circuits if f.done is set.
func (f *filter) applyRefreshTokenResponse(resp *http.Response, err error) {
	// Drain + close the response body in all paths to avoid connection leak
	// per the net/http documentation. Safe-to-call on nil body via the
	// stdlib's nopcloser shape.
	defer func() {
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}()

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		// OnDestroy fired before the continuation — abort without touching
		// dcb. Counter increments + Set-Cookie envelope mutations also
		// abort (the request is gone; the upstream that would have
		// observed the deferred Set-Cookie never gets to encode-side).
		return
	}

	cc := f.cc

	// Classify the disposition per SPEC §4.7 + AMEND-3:
	//   - non-2xx OR transport-level error → refresh-failure leg
	//   - 2xx → refresh-success leg
	failed := err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300

	if failed {
		// Refresh-failure: category (a) 302 challenge via handleRefreshFailure
		// (lives in decode_headers.go; handles the counter increment + the
		// 302 emission). Mirrors extauthz's deny-path SendLocalReply +
		// ContinueDecoding pattern to unblock the parked decode goroutine.
		_ = f.handleRefreshFailureLocked()
		// Resume the parked decode goroutine per the extauthz
		// SendLocalReply + ContinueDecoding pattern (fault.go:321-324).
		// SendLocalReply alone sets c.localReplyDone but does NOT unblock
		// the park.
		if f.dcb != nil {
			f.dcb.ContinueDecoding()
		}
		return
	}

	// Refresh-success: emit the deferred Set-Cookie envelope per ADR-0183 +
	// SPEC §6.8 + §4.5 category (b). Task 8 records the pending envelope
	// on f.pendingSetCookies; Task 11 (full filter integration) wires the
	// encode-side emission. Task 12 fixture-0024 scenario (d)
	// `refresh_token_rotation/` validates end-to-end.
	//
	// Task 8 builds the envelope with placeholder values for the new
	// access_token + refresh_token (the actual JSON parse + AES-CBC
	// re-encryption lands at Task 10/11 via the oauth_client.go body
	// + the cookie-write call-site). The Task 8 placeholder is "" for
	// each cookie value — the envelope SHAPE (3-or-4 Set-Cookie headers
	// per §4.5 category (b)) is the load-bearing invariant; the byte-
	// exact cookie values land at Task 10/11.
	pending := make(http.Header)
	// At Task 8 we lack a Task-10 JSON parser for the token_endpoint
	// response body — wire the envelope with empty values for now per
	// the SKELETON discipline (Task 10 fills the placeholders with the
	// parsed access_token + refresh_token + expires_in). The pending
	// envelope shape is correct per §4.5 category (b); the byte-exact
	// values are Task 10's concern.
	expiresEpoch := time.Now().Add(cc.defaultExpiresIn).Unix()
	emitCategoryB_PostCallback(
		pending,
		cc.cookieNames,
		cc.cookieAttrs,
		"", // encAccessToken — Task 10 fills from token_endpoint JSON
		"", // hmacValue       — Task 10 computes via hmac.go::computeHMAC
		expiresEpoch,
		"", // encRefreshToken — Task 10 fills (empty when use_refresh_token=false)
	)

	oh := envoyhttp.OrderedHeaders{}
	for _, v := range pending["Set-Cookie"] {
		oh = append(oh, envoyhttp.HeaderField{Name: "Set-Cookie", Value: v})
	}
	f.pendingSetCookies = oh

	// Counter bump: oauth_refreshtoken_success per SPEC §4.6 + ADR-0183.
	// One-per-event invariant: NO other counter increments on this leg
	// (oauth_failure / oauth_refreshtoken_failure / oauth_success all
	// stay at zero per AMEND-3 + §4.6).
	if cc.stats != nil && cc.stats.oauthRefreshtokenSuccess != nil {
		cc.stats.oauthRefreshtokenSuccess.Inc()
	}

	// Resume the parked decode goroutine — the request flows through to
	// the upstream with the (Task 11) Authorization-header injection per
	// `forward_bearer_token=true`; the deferred Set-Cookie envelope lands
	// on the response via the (Task 11) encode-side wiring.
	if f.dcb != nil {
		f.dcb.ContinueDecoding()
	}
}

// handleRefreshFailureLocked is the f.mu-held variant of
// handleRefreshFailure (decode_headers.go) — used by
// applyRefreshTokenResponse from inside the resume goroutine where f.mu is
// already held. The handler's body must NOT re-acquire f.mu.
//
// Increments oauth_refreshtoken_failure + emits the (a) 302 challenge wire
// shape per SPEC §4.1 + §4.7 + ADR-0183. The wire shape is identical to
// handleUnauthenticated — the only difference is the counter increment site.
func (f *filter) handleRefreshFailureLocked() envoyhttp.FilterHeadersStatus {
	cc := f.cc
	if cc.stats != nil && cc.stats.oauthRefreshtokenFailure != nil {
		cc.stats.oauthRefreshtokenFailure.Inc()
	}

	// Emit the category (a) 302 challenge wire shape inline (the
	// handleUnauthenticated body acquires no locks of its own; the only
	// state it touches is cc.* (read-only) + f.dcb (write-only via
	// SendLocalReply). Safe to call with f.mu held.
	stateValue := f.composeStateCookieValueSkeleton()
	authorizationURL := cc.authorizationEndpoint

	hdrs := envoyhttp.OrderedHeaders{
		{Name: "Location", Value: authorizationURL},
	}
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

// handleBadState emits the category (d) 401-with-constant-body wire shape
// per phase-20 SPEC §4.1 + §4.3 + AMEND-3 + ADR-0180 + planner-time D15.
// Increments `oauth_unauthorized_rq` per SPEC §4.6.
//
// Per literal planner-time D15: "NO new counter (the standard
// `oauth_unauthorized_rq` increments per §4.6)" — the standard counter
// SHOULD increment on BOTH the bad-state-cookie path AND the POST-callback
// PARSE-REJECT path. Both PARSE-REJECT triggers flow through this single
// handler per literal D15.
//
// Per AMEND-3 cleanup is part of the deny-path: addFlowCookieDeletionHeaders
// runs to clear the 5 flow cookies.
//
// Per SPEC §4.3 the 401 body is the byte-exact 18-byte constant
// "OAuth flow failed." (sourced from upstream `UnauthorizedBodyMessage`;
// no trailing newline). Content-Type defers to the HCM SendLocalReply
// default for non-grpc downstream per SPEC §12 item A1 + A4 RATIFIED-
// PENDING-IMPL-TIME (most-likely text/plain).
//
// History: Task 8 commit df33e4b introduced a transient call-site split
// (handleBadStateNoCounter for the POST-callback path) which contradicted
// the literal D15 text. The split was reverted in a Task 8 follow-up; the
// single-handler shape below is the post-revert state per literal D15.
func (f *filter) handleBadState() envoyhttp.FilterHeadersStatus {
	cc := f.cc

	// Increment oauth_unauthorized_rq per SPEC §4.6 + literal planner-time
	// D15 — both the bad-state-cookie branch AND the POST-callback PARSE-
	// REJECT branch increment the standard counter (D15: "NO new counter
	// (the standard `oauth_unauthorized_rq` increments per §4.6)").
	if cc.stats != nil && cc.stats.oauthUnauthorizedRq != nil {
		cc.stats.oauthUnauthorizedRq.Inc()
	}

	return f.emitCategoryD401()
}

// emitCategoryD401 is the (d) 401 wire-shape emission helper.
// Called from handleBadState (which adds the counter bump per literal D15).
// Per SPEC §4.5 category (d) + AMEND-3 emits constUnauthorizedBody +
// addFlowCookieDeletionHeaders.
func (f *filter) emitCategoryD401() envoyhttp.FilterHeadersStatus {
	cc := f.cc

	// Compose the wire-shape header set:
	//   - Status: 401
	//   - Body:   "OAuth flow failed." (18 bytes)
	//   - Headers: addFlowCookieDeletionHeaders (5 cleared cookies)
	cookieHeaders := make(http.Header)
	addFlowCookieDeletionHeaders(cookieHeaders, cc.cookieNames, cc.cookieAttrs)

	hdrs := envoyhttp.OrderedHeaders{}
	for _, v := range cookieHeaders["Set-Cookie"] {
		hdrs = append(hdrs, envoyhttp.HeaderField{Name: "Set-Cookie", Value: v})
	}

	if f.dcb != nil {
		f.dcb.SendLocalReply(401, constUnauthorizedBody, hdrs)
	}
	return envoyhttp.StopIteration
}

// extractCallbackParams parses the `code` + `state` query parameters from a
// callback URL path. Per SPEC §6.8 the callback request carries the OAuth2
// `code` + `state` parameters in the query string per RFC 6749 §4.1.2.
//
// Returns the parsed (code, state) values URL-DECODED per RFC 3986 §2.4;
// empty strings on absence / malformed query. The function NEVER panics on
// arbitrary input (the fuzzer at Task 12 asserts the must-never-panic
// discipline across this surface).
//
// # Item C URL-decode (Task 10 + ADR-0185 §Decision)
//
// The auth-server percent-encoded the `code` + `state` values on the
// redirect-back per RFC 6749 §4.1.2 + RFC 3986; the filter undoes the
// encoding on read so downstream consumers see the literal bytes the
// auth-server originally allocated. This is the symmetric counterpart to
// the outbound `urlEncode` discipline at oauth_client.go (the two flow
// directions share the same charset per AMEND-5 + §20.P10 RATIFIED). A
// malformed percent-encoding (e.g. `%ZZ`) in either value surfaces as an
// EMPTY string for that field — graceful-failure per the must-never-panic
// discipline + the AMEND-3 deny-path classification (the downstream state-
// cookie validator routes empty-state to handleBadState).
func extractCallbackParams(path string) (code, state string) {
	qIdx := strings.IndexByte(path, '?')
	if qIdx == -1 {
		return "", ""
	}
	query := path[qIdx+1:]
	for _, pair := range strings.Split(query, "&") {
		eqIdx := strings.IndexByte(pair, '=')
		if eqIdx == -1 {
			continue
		}
		key := pair[:eqIdx]
		rawValue := pair[eqIdx+1:]
		// URL-decode per RFC 3986 §2.4 + ADR-0185 §Decision Item C. Malformed
		// percent-encoding → empty string for that field (graceful failure;
		// must-never-panic per SPEC §7.4 fuzzer discipline).
		decoded, err := url.QueryUnescape(rawValue)
		if err != nil {
			decoded = ""
		}
		switch key {
		case "code":
			code = decoded
		case "state":
			state = decoded
		}
	}
	return code, state
}

// lookupStateCookie returns the state-cookie value from the request
// headers per phase-20 SPEC §6.6 + §4.4. The state cookie name is
// configured on the compiledConfig (cc.stateCookieName) per SPEC §4.5
// category (a) + §4.4.
//
// Returns the empty string when the cookie is absent.
func (f *filter) lookupStateCookie(headers http.Header) string {
	r := &http.Request{Header: headers}
	for _, c := range r.Cookies() {
		if c.Name == f.cc.stateCookieName {
			return c.Value
		}
	}
	return ""
}

// validateStateCookie returns true iff the state-cookie value matches the
// `state` query-parameter per phase-20 SPEC §6.6 + §4.4. Task 5 SKELETON:
// byte-equality compare. Task 8/10 wire the HMAC-protected composition +
// verification per SPEC §12 item A3 RATIFIED-PENDING-IMPL-TIME.
//
// Empty values (either side) → false. A missing state-cookie OR missing
// `state` query-param both fail validation per the AMEND-3 deny-path
// discipline.
func validateStateCookie(cookieValue, queryValue string) bool {
	if cookieValue == "" || queryValue == "" {
		return false
	}
	return cookieValue == queryValue
}
