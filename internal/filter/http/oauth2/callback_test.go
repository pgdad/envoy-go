package oauth2

// callback_test.go — auth-code POST wire-up tests for handleCallback +
// applyTokenEndpointResponse per phase-20 SPEC §6.8 + §4.5 category (b) +
// §4.7 + AMEND-3 + ADR-0180. Task 12 follow-up: closes the structural
// wire-up gap discovered at Task 12 (handleCallback was authored as
// SKELETON at Task 5; Task 10 landed postTokenEndpoint but did NOT wire
// it into handleCallback — the auth-code sign-in flow was structurally
// non-functional end-to-end until this follow-up).
//
// # Test surface
//
// 8 new tests covering the auth-code POST disposition matrix per SPEC §4.5
// + §4.7 + AMEND-3:
//
//   - happy path: 2xx → category (b) 302 + populated 5-cookie envelope +
//     oauth_success++
//   - 5xx retry-eligible → category (a) 302 challenge (no counter)
//   - 4xx terminal → category (d) 401 + addFlowCookieDeletionHeaders +
//     oauth_failure++ + constant body
//   - nil-poster fail-safe → category (a) 302 challenge
//   - OnDestroy mid-POST → no dcb touch + no panic (mirrors Task 8 refresh
//     OnDestroy guard)
//   - malformed JSON body → category (a) 302 challenge (treated as transient)
//   - BearerToken cookie value is AES-CBC ciphertext (NOT plaintext)
//   - HMAC cookie validates against the issued envelope (round-trip via
//     hmac.go::validateHMAC)
//
// # Test harness reuse
//
// Tests reuse `fakeTokenPoster` + `fakeTokenPosterErr` + `fakeRefreshDCB` +
// `newRefreshTestConfig` from oauth_client_test.go (they are
// poster-shape-agnostic). The callback flow shares the
// `tokenEndpointPoster` abstraction with the refresh flow per ADR-0185 +
// compiled_config.go::tokenEndpointPosterFn; no separate harness needed.

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// newCallbackTestConfig constructs a compiledConfig wired for the auth-code
// POST tests: stats + cookieNames + cookieAttrs + redirectURI + hmacSecret
// + the supplied poster.
func newCallbackTestConfig(poster tokenEndpointPosterFn) *compiledConfig {
	cc := newRefreshTestConfig(poster)
	cc.redirectURI = "https://example.com/post-signin"
	cc.hmacSecretFn = func() []byte { return []byte("test-hmac-secret-32-bytes-padding") }
	return cc
}

// callbackHeadersWithState builds an :path-bearing http.Header with a
// matching state-cookie value for the dispatcher's state-cookie validation
// per SPEC §6.6 + §4.4.
func callbackHeadersWithState(state string) http.Header {
	h := http.Header{}
	h.Set(":path", "/oauth2/callback?code=auth-code-from-idp&state="+state)
	h.Set(":method", "GET")
	// State cookie name is "OauthExpires" per newTestCompiledConfig — the
	// state cookie value must byte-equal the `state` query-parameter per
	// validateStateCookie (Task 5 SKELETON byte-equality compare).
	h.Set("Cookie", "OauthExpires="+state)
	return h
}

// ---------------------------------------------------------------------------
// Test 1 — auth-code happy path: 2xx → category (b) 302 + populated envelope
// ---------------------------------------------------------------------------

// TestHandleCallback_SuccessfulPost_EmitsCategoryB302_With_PopulatedEnvelope
// verifies the full auth-code sign-in success leg per SPEC §6.8 + §4.5
// category (b):
//
//  1. State cookie validates → dispatch to outbound POST (NOT bad-state).
//  2. Poster returns 2xx with a 4-field JSON token-endpoint response.
//  3. applyTokenEndpointResponse parses the body, encrypts the access_token
//     + refresh_token, computes the HMAC, emits a 302 with:
//     - Location: <redirect_uri>
//     - Set-Cookie: BearerToken=<ciphertext>; ...
//     - Set-Cookie: OauthHMAC=<base64url>; ...
//     - Set-Cookie: OauthExpires=<epoch>; ...
//     - Set-Cookie: RefreshToken=<ciphertext>; ...
//  4. oauth_success counter increments per SPEC §4.6 + ADR-0181.
func TestHandleCallback_SuccessfulPost_EmitsCategoryB302_With_PopulatedEnvelope(t *testing.T) {
	body := `{"access_token":"at-from-idp","refresh_token":"rt-from-idp","id_token":"","expires_in":3600}`
	cc := newCallbackTestConfig(fakeTokenPoster(200, body))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handleCallback(callbackHeadersWithState("state-xyz"))
	if status != envoyhttp.StopIteration {
		t.Fatalf("handleCallback: got %v, want StopIteration", status)
	}
	f.waitCallbackDone()

	contCount, lrCount, lastStatus, lastBody, lastHeaders := dcb.snapshot()
	if lrCount != 1 {
		t.Fatalf("SendLocalReply count: got %d, want 1 (success-leg emits category (b) 302)", lrCount)
	}
	if lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (category (b) per §4.5)", lastStatus)
	}
	if lastBody != "" {
		t.Errorf("lastBody: got %q, want \"\" (category (b) has empty body per §4.5)", lastBody)
	}
	if contCount != 1 {
		t.Errorf("ContinueDecoding count: got %d, want 1 (SendLocalReply pattern requires Continue to unblock parked goroutine)", contCount)
	}
	// Location: <redirect_uri>
	if got := getHeader(lastHeaders, "Location"); got != cc.redirectURI {
		t.Errorf("Location: got %q, want %q", got, cc.redirectURI)
	}
	// Set-Cookie envelope: 4 cookies (BearerToken / OauthHMAC / OauthExpires
	// / RefreshToken) per §4.5 category (b) with RefreshToken present.
	if got := countSetCookies(lastHeaders); got != 4 {
		t.Errorf("Set-Cookie count: got %d, want 4 (BearerToken + OauthHMAC + OauthExpires + RefreshToken per §4.5 category (b))", got)
	}
	// oauth_success counter increments per SPEC §4.6.
	if got := cc.stats.oauthSuccess.Load(); got != 1 {
		t.Errorf("oauth_success: got %d, want 1", got)
	}
	if got := cc.stats.oauthFailure.Load(); got != 0 {
		t.Errorf("oauth_failure: got %d, want 0 (success-leg does NOT bump failure)", got)
	}
}

// ---------------------------------------------------------------------------
// Test 2 — 5xx retry-eligible → category (a) 302 challenge
// ---------------------------------------------------------------------------

// TestHandleCallback_TokenEndpointFailure_5xx_EmitsCategoryA302 verifies a
// 5xx response from the token_endpoint routes through
// applyTokenEndpointResponse to category (a) 302 challenge per SPEC §4.7 +
// AMEND-3 (retry-eligible: re-authenticate from scratch). Per AMEND-3 +
// §4.6 NO `oauth_failure` increment on this leg (terminal-failure 4xx is
// distinct from retry-eligible 5xx).
func TestHandleCallback_TokenEndpointFailure_5xx_EmitsCategoryA302(t *testing.T) {
	cc := newCallbackTestConfig(fakeTokenPoster(500, "internal server error"))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handleCallback(callbackHeadersWithState("state-xyz"))
	if status != envoyhttp.StopIteration {
		t.Fatalf("handleCallback: got %v, want StopIteration", status)
	}
	f.waitCallbackDone()

	_, lrCount, lastStatus, _, lastHeaders := dcb.snapshot()
	if lrCount != 1 {
		t.Fatalf("SendLocalReply count: got %d, want 1 (5xx → category (a))", lrCount)
	}
	if lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (category (a) per §4.7)", lastStatus)
	}
	if got := getHeader(lastHeaders, "Location"); got != cc.authorizationEndpoint {
		t.Errorf("Location: got %q, want %q (category (a) redirects to authorization_endpoint)", got, cc.authorizationEndpoint)
	}
	if got := cc.stats.oauthFailure.Load(); got != 0 {
		t.Errorf("oauth_failure: got %d, want 0 (5xx is retry-eligible per AMEND-3; oauth_failure is reserved for 4xx terminal)", got)
	}
	if got := cc.stats.oauthSuccess.Load(); got != 0 {
		t.Errorf("oauth_success: got %d, want 0 (5xx is NOT success)", got)
	}
}

// ---------------------------------------------------------------------------
// Test 3 — 4xx terminal → category (d) 401 + constant body
// ---------------------------------------------------------------------------

// TestHandleCallback_TokenEndpointFailure_4xx_EmitsCategoryD401 verifies a
// 4xx response from the token_endpoint routes to category (d) 401 with
// constant body + addFlowCookieDeletionHeaders per SPEC §4.5 category (d)
// + AMEND-3. Per SPEC §4.6 `oauth_failure` increments on this leg.
func TestHandleCallback_TokenEndpointFailure_4xx_EmitsCategoryD401(t *testing.T) {
	cc := newCallbackTestConfig(fakeTokenPoster(400, `{"error":"invalid_grant"}`))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handleCallback(callbackHeadersWithState("state-xyz"))
	if status != envoyhttp.StopIteration {
		t.Fatalf("handleCallback: got %v, want StopIteration", status)
	}
	f.waitCallbackDone()

	_, lrCount, lastStatus, lastBody, lastHeaders := dcb.snapshot()
	if lrCount != 1 {
		t.Fatalf("SendLocalReply count: got %d, want 1 (4xx → category (d))", lrCount)
	}
	if lastStatus != 401 {
		t.Errorf("lastStatus: got %d, want 401 (category (d) per §4.5)", lastStatus)
	}
	if lastBody != constUnauthorizedBody {
		t.Errorf("lastBody: got %q, want %q (category (d) constant body per §4.3)", lastBody, constUnauthorizedBody)
	}
	// 5 envelope-clearing Set-Cookie headers per addFlowCookieDeletionHeaders.
	if got := countSetCookies(lastHeaders); got != 5 {
		t.Errorf("Set-Cookie count: got %d, want 5 (5 envelope cookies cleared per addFlowCookieDeletionHeaders)", got)
	}
	if got := cc.stats.oauthFailure.Load(); got != 1 {
		t.Errorf("oauth_failure: got %d, want 1 (4xx terminal increments oauth_failure per §4.6)", got)
	}
	if got := cc.stats.oauthSuccess.Load(); got != 0 {
		t.Errorf("oauth_success: got %d, want 0 (4xx is NOT success)", got)
	}
}

// ---------------------------------------------------------------------------
// Test 4 — nil-poster fail-safe → category (a) 302 challenge
// ---------------------------------------------------------------------------

// TestHandleCallback_PosterNil_GracefulFailure verifies handleCallback
// routes through the nil-poster guard to category (a) 302 challenge per
// AMEND-3 deny-path discipline (mirrors handleRefresh's nil-poster guard
// landed at Task 8). The errNoPosterConfigured surfaces through
// applyTokenEndpointResponse's err-non-nil branch.
func TestHandleCallback_PosterNil_GracefulFailure(t *testing.T) {
	cc := newCallbackTestConfig(nil) // nil poster — exercises the guard
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleCallback(nil poster) panicked: %v", r)
		}
	}()
	status := f.handleCallback(callbackHeadersWithState("state-xyz"))
	if status != envoyhttp.StopIteration {
		t.Fatalf("handleCallback: got %v, want StopIteration", status)
	}
	f.waitCallbackDone()

	_, lrCount, lastStatus, _, _ := dcb.snapshot()
	if lrCount != 1 {
		t.Fatalf("SendLocalReply count: got %d, want 1 (nil-poster → category (a))", lrCount)
	}
	if lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (nil-poster fail-safe is category (a) per AMEND-3)", lastStatus)
	}
}

// ---------------------------------------------------------------------------
// Test 5 — OnDestroy guard: in-flight POST + OnDestroy → no dcb touch
// ---------------------------------------------------------------------------

// TestApplyTokenEndpointResponse_OnDestroyGuard verifies the done-guard
// per ADR-0159 D4 + extauthz precedent. After OnDestroy fires,
// applyTokenEndpointResponse MUST NOT touch the dcb (no SendLocalReply, no
// ContinueDecoding, no counter increment) + MUST NOT panic. Mirrors Task
// 8's TestApplyRefreshTokenResponse_OnDestroyGuard.
func TestApplyTokenEndpointResponse_OnDestroyGuard(t *testing.T) {
	cc := newCallbackTestConfig(fakeTokenPoster(200, `{"access_token":"at"}`))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	f.OnDestroy() // sets done=true

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("applyTokenEndpointResponse panicked under OnDestroy: %v", r)
		}
	}()
	// Direct-invoke applyTokenEndpointResponse with a synthesized 2xx —
	// the done-guard MUST short-circuit before touching dcb or counters.
	f.applyTokenEndpointResponse(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil)

	contCount, lrCount, _, _, _ := dcb.snapshot()
	if contCount != 0 {
		t.Errorf("ContinueDecoding count: got %d, want 0 (done-guard must short-circuit)", contCount)
	}
	if lrCount != 0 {
		t.Errorf("SendLocalReply count: got %d, want 0 (done-guard must short-circuit)", lrCount)
	}
	if got := cc.stats.oauthSuccess.Load(); got != 0 {
		t.Errorf("oauth_success: got %d, want 0 (done-guard must short-circuit before counter bump)", got)
	}
}

// ---------------------------------------------------------------------------
// Test 6 — malformed JSON body → category (a) 302 challenge
// ---------------------------------------------------------------------------

// TestApplyTokenEndpointResponse_MalformedJSON_EmitsCategoryA verifies a
// 2xx response with a malformed JSON body routes to category (a) 302
// challenge per AMEND-3 deny-path discipline (treat-as-transient-failure).
// The recovered envelope (with empty access_token) cannot be HMAC-validated
// downstream, so the 302 challenge is the appropriate retry-eligible
// classification.
func TestApplyTokenEndpointResponse_MalformedJSON_EmitsCategoryA(t *testing.T) {
	cc := newCallbackTestConfig(fakeTokenPoster(200, `{malformed-json`))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handleCallback(callbackHeadersWithState("state-xyz"))
	if status != envoyhttp.StopIteration {
		t.Fatalf("handleCallback: got %v, want StopIteration", status)
	}
	f.waitCallbackDone()

	_, lrCount, lastStatus, _, lastHeaders := dcb.snapshot()
	if lrCount != 1 {
		t.Fatalf("SendLocalReply count: got %d, want 1", lrCount)
	}
	if lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (malformed body → transient-failure → category (a))", lastStatus)
	}
	if got := getHeader(lastHeaders, "Location"); got != cc.authorizationEndpoint {
		t.Errorf("Location: got %q, want %q (category (a))", got, cc.authorizationEndpoint)
	}
	if got := cc.stats.oauthSuccess.Load(); got != 0 {
		t.Errorf("oauth_success: got %d, want 0 (malformed body is NOT success)", got)
	}
	if got := cc.stats.oauthFailure.Load(); got != 0 {
		t.Errorf("oauth_failure: got %d, want 0 (malformed body is retry-eligible; oauth_failure reserved for 4xx terminal)", got)
	}
}

// ---------------------------------------------------------------------------
// Test 7 — BearerToken cookie value is AES-CBC ciphertext (NOT plaintext)
// ---------------------------------------------------------------------------

// TestHandleCallback_AccessTokenEncrypted_InEnvelope verifies the
// BearerToken cookie value emitted on the (b) 302 success leg is the AES-
// 256-CBC ciphertext per AMEND-1 + ADR-0182 (NOT the plaintext
// access_token). Round-trips via decryptToken to recover the original
// plaintext.
func TestHandleCallback_AccessTokenEncrypted_InEnvelope(t *testing.T) {
	const accessTokenPlaintext = "secret-access-token-from-idp"
	body := `{"access_token":"` + accessTokenPlaintext + `","expires_in":3600}`
	cc := newCallbackTestConfig(fakeTokenPoster(200, body))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	_ = f.handleCallback(callbackHeadersWithState("state-xyz"))
	f.waitCallbackDone()

	_, _, _, _, lastHeaders := dcb.snapshot()
	// Find the BearerToken Set-Cookie + extract its value.
	bearerCookieValue := extractCookieValue(lastHeaders, cc.cookieNames.BearerToken)
	if bearerCookieValue == "" {
		t.Fatalf("BearerToken cookie missing from emitted headers: %v", lastHeaders)
	}
	if bearerCookieValue == accessTokenPlaintext {
		t.Fatalf("BearerToken cookie value is plaintext (%q); expected AES-CBC ciphertext per AMEND-1", bearerCookieValue)
	}
	// AES-CBC ciphertext envelope is Base64URL-raw-encoded (IV‖CT). Smoke-
	// check the value decodes as Base64URL-raw + has length >= 16+16 = 32
	// raw bytes (IV + at least one CT block).
	raw, err := base64.RawURLEncoding.DecodeString(bearerCookieValue)
	if err != nil {
		t.Fatalf("BearerToken value is not Base64URL-raw: %q (err=%v)", bearerCookieValue, err)
	}
	if len(raw) < 32 {
		t.Errorf("BearerToken decoded length: got %d, want >= 32 (IV=16 + >=1 CT block of 16)", len(raw))
	}
	// Round-trip via decryptToken: ciphertext → plaintext.
	plaintext := decryptToken(bearerCookieValue, cc.hmacSecretFn())
	if string(plaintext) != accessTokenPlaintext {
		t.Errorf("decryptToken(BearerToken): got %q, want %q (round-trip per AMEND-1)", plaintext, accessTokenPlaintext)
	}
}

// ---------------------------------------------------------------------------
// Test 8 — HMAC over envelope validates round-trip
// ---------------------------------------------------------------------------

// TestHandleCallback_HMACOverEnvelope_Computed_Correctly verifies the
// OauthHMAC cookie value emitted on the (b) 302 success leg validates
// against the issued envelope via hmac.go::validateHMAC. The HMAC is
// computed over (domain="", expires, encAccessToken, idToken, encRefreshToken)
// per the callback-leg's empty-domain anchoring per AMEND-2 + ADR-0179.
func TestHandleCallback_HMACOverEnvelope_Computed_Correctly(t *testing.T) {
	body := `{"access_token":"at","refresh_token":"rt","expires_in":3600}`
	cc := newCallbackTestConfig(fakeTokenPoster(200, body))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	_ = f.handleCallback(callbackHeadersWithState("state-xyz"))
	f.waitCallbackDone()

	_, _, _, _, lastHeaders := dcb.snapshot()
	encAccessToken := extractCookieValue(lastHeaders, cc.cookieNames.BearerToken)
	hmacValue := extractCookieValue(lastHeaders, cc.cookieNames.OauthHMAC)
	expires := extractCookieValue(lastHeaders, cc.cookieNames.OauthExpires)
	encRefreshToken := extractCookieValue(lastHeaders, cc.cookieNames.RefreshToken)

	if hmacValue == "" {
		t.Fatalf("OauthHMAC cookie missing from emitted headers: %v", lastHeaders)
	}

	// Round-trip: validateHMAC over the SAME tuple the emitter used should
	// return true. The callback emitter anchors domain="" (no request
	// authority context on the upstream-bound redirect — see the docstring
	// rationale at emitCategoryB_PostCallbackLocked).
	ok := validateHMAC("", expires, encAccessToken, "", encRefreshToken, cc.hmacSecretFn(), hmacValue)
	if !ok {
		t.Errorf("validateHMAC round-trip: got false, want true (the emitted HMAC must validate against the emitted envelope)")
	}
}

// ---------------------------------------------------------------------------
// Test 9 — transport-level error → category (a) 302 challenge
// ---------------------------------------------------------------------------

// TestHandleCallback_TransportError_EmitsCategoryA verifies a transport-
// level error (non-nil err from the poster) routes to category (a) 302
// challenge per AMEND-3 (treated as 5xx-equivalent for counter / wire-
// shape purposes). Mirrors the refresh-leg's transport-error behavior at
// TestHandleRefresh_TransportError_TreatedAsFailure.
func TestHandleCallback_TransportError_EmitsCategoryA(t *testing.T) {
	cc := newCallbackTestConfig(fakeTokenPosterErr(context.DeadlineExceeded))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handleCallback(callbackHeadersWithState("state-xyz"))
	if status != envoyhttp.StopIteration {
		t.Fatalf("handleCallback: got %v, want StopIteration", status)
	}
	f.waitCallbackDone()

	_, lrCount, lastStatus, _, _ := dcb.snapshot()
	if lrCount != 1 {
		t.Fatalf("SendLocalReply count: got %d, want 1", lrCount)
	}
	if lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (transport-error → category (a) per AMEND-3)", lastStatus)
	}
	if got := cc.stats.oauthFailure.Load(); got != 0 {
		t.Errorf("oauth_failure: got %d, want 0 (transport-error is retry-eligible)", got)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractCookieValue returns the cookie value for `name` from the first
// Set-Cookie header that begins with `name=` in `oh`. Returns "" when
// the cookie is absent. The cookie value is the substring between
// `name=` and the first `;` (or end-of-string).
func extractCookieValue(oh envoyhttp.OrderedHeaders, name string) string {
	prefix := name + "="
	for _, hf := range oh {
		if hf.Name != "Set-Cookie" {
			continue
		}
		if !strings.HasPrefix(hf.Value, prefix) {
			continue
		}
		rest := hf.Value[len(prefix):]
		if semi := strings.IndexByte(rest, ';'); semi >= 0 {
			return rest[:semi]
		}
		return rest
	}
	return ""
}
