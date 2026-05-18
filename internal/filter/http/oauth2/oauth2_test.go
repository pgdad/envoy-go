package oauth2

// oauth2_test.go — Group 8 dispatcher tests + Group 9 compile-time invariant
// tests + per-handler tests + RegisterPerRouteValidator tests per phase-20
// SPEC §14.1 + IMPL Task 5.
//
// # Test group layout
//
//   - Group 8 (dispatcher tests): 9 tests asserting the SPEC §6.3 dispatch-
//     priority order + the 4-emission-category wire shape per SPEC §4.1.
//   - Group 9 (compile-time invariants): blank-identifier interface
//     conformance assertions + TypeURL byte-exact assertion + per-route
//     PARSE-REJECT byte-stable wording assertion.
//   - Per-handler tests: handleUnauthenticated (category (a)) +
//     handlePassThrough (counter increment + Continue) + handleBadState
//     (category (d) 401 + cookie cleanup) + handleValidCookies
//     (Authorization injection).
//   - Per-route validator tests: TestRegisterPerRouteValidator wiring +
//     PARSE-REJECT byte-stable wording per SPEC §5.2 + planner-time D2.
//
// # Test harness
//
// `fakeOAuth2DCB` is a minimal DecoderFilterCallbacks for capturing
// SendLocalReply calls (mirrors the extauthz `fakeExtAuthzDCB` pattern at
// internal/filter/http/extauthz/extauthz_test.go:3559). The harness
// captures the (status, body, headers) triple per SendLocalReply call so
// tests can assert the wire-shape byte-exact.
//
// `newTestCompiledConfig` (declared in compiled_config.go) constructs a
// minimal *compiledConfig with default cookie names + attributes; tests
// populate per-test matchers / secret accessors / behavioral knobs.

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	oauth2v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/oauth2/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// ---------------------------------------------------------------------------
// Test harness — fake DCB + helpers
// ---------------------------------------------------------------------------

// fakeOAuth2DCB is a minimal DecoderFilterCallbacks for capturing
// SendLocalReply calls. Mirrors the extauthz fakeExtAuthzDCB pattern.
type fakeOAuth2DCB struct {
	localReplyCount int
	lastStatus      int
	lastBody        string
	lastHeaders     envoyhttp.OrderedHeaders
	continueCount   int
}

func newFakeOAuth2DCB() *fakeOAuth2DCB { return &fakeOAuth2DCB{} }

func (c *fakeOAuth2DCB) ContinueDecoding()             { c.continueCount++ }
func (c *fakeOAuth2DCB) DownstreamPrincipal() []string { return nil }

// ADR-0165 callback-surface extension stubs.
func (c *fakeOAuth2DCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (c *fakeOAuth2DCB) DownstreamLocalAddr() net.Addr    { return nil }
func (c *fakeOAuth2DCB) DownstreamTLSServerName() string  { return "" }
func (c *fakeOAuth2DCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (c *fakeOAuth2DCB) DownstreamProtocol() string       { return "" }
func (c *fakeOAuth2DCB) ListenerPrincipal() string        { return "" }

func (c *fakeOAuth2DCB) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.localReplyCount++
	c.lastStatus = status
	c.lastBody = body
	c.lastHeaders = headers
}

func (c *fakeOAuth2DCB) RequestRouteConfig() proto.Message { return nil }
func (c *fakeOAuth2DCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *fakeOAuth2DCB) EncodeHeaders(_ http.Header, _ bool) {}
func (c *fakeOAuth2DCB) EncodeData(_ []byte, _ bool)         {}
func (c *fakeOAuth2DCB) EncodeTrailers(_ http.Header)        {}

// countSetCookies returns the number of Set-Cookie headers in `oh`.
func countSetCookies(oh envoyhttp.OrderedHeaders) int {
	n := 0
	for _, hf := range oh {
		if hf.Name == "Set-Cookie" {
			n++
		}
	}
	return n
}

// getHeader returns the first header value for `name` in `oh` (empty if
// absent). Mirrors net/http.Header.Get for OrderedHeaders.
func getHeader(oh envoyhttp.OrderedHeaders, name string) string {
	for _, hf := range oh {
		if hf.Name == name {
			return hf.Value
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Group 9 — compile-time invariants per SPEC §6.12
// ---------------------------------------------------------------------------

// Compile-time blank-identifier assertion: *filter implements the decoder-
// only filter interface per SPEC §6.12. A regression that breaks the
// interface conformance surfaces at build time.
var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)

// TestTypeURL_ByteExact pins the TypeURL constant byte-exact per SPEC §6.1.
// A regression that renames the type URL surfaces at this assertion.
func TestTypeURL_ByteExact(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.http.oauth2.v3.OAuth2"
	if TypeURL != want {
		t.Fatalf("TypeURL byte-exact: got %q, want %q", TypeURL, want)
	}
}

// TestFilterName_ByteExact pins the filterName constant byte-exact per
// SPEC §6.1. The filterName is consumed by RegisterPerRouteValidator + the
// per-route TPFC PARSE-REJECT path per SPEC §5.2.
func TestFilterName_ByteExact(t *testing.T) {
	const want = "envoy.filters.http.oauth2"
	if filterName != want {
		t.Fatalf("filterName byte-exact: got %q, want %q", filterName, want)
	}
}

// TestPerRouteTPFCRejectMsg_ByteExact_PlannerD2 pins the per-route TPFC
// PARSE-REJECT wording byte-exact per planner-time D2 + SPEC §5.2. The
// wording is shared with the framework's HCM-build-time error wrapper
// (BuildPerRouteConfig prepends a location prefix to the wording).
func TestPerRouteTPFCRejectMsg_ByteExact_PlannerD2(t *testing.T) {
	const want = "oauth2: typed_per_filter_config not supported at route or virtualHost level; oauth2 is listener-scoped only"
	if perRouteTPFCRejectMsg != want {
		t.Fatalf("perRouteTPFCRejectMsg byte-exact: got %q, want %q", perRouteTPFCRejectMsg, want)
	}
}

// TestConstUnauthorizedBody_ByteExact_18Bytes pins the (d) 401 constant
// body byte-exact per SPEC §4.3 + §20.P9 + AMEND-3. Sourced from upstream
// `UnauthorizedBodyMessage` (18 ASCII bytes; no trailing newline).
func TestConstUnauthorizedBody_ByteExact_18Bytes(t *testing.T) {
	const want = "OAuth flow failed."
	if constUnauthorizedBody != want {
		t.Fatalf("constUnauthorizedBody byte-exact: got %q, want %q", constUnauthorizedBody, want)
	}
	if len(constUnauthorizedBody) != 18 {
		t.Fatalf("constUnauthorizedBody length: got %d bytes, want 18 bytes (per SPEC §4.3 + §20.P9)", len(constUnauthorizedBody))
	}
}

// ---------------------------------------------------------------------------
// Group 8 — dispatcher tests per SPEC §6.3 + §14.1
// ---------------------------------------------------------------------------

// TestDispatcher_SignoutPath_Highest_Priority verifies signout_path is the
// FIRST priority per SPEC §6.3 step 1. A request to the sign-out path
// routes to handleSignout (category (c) 302) regardless of any other
// matcher hit (the test populates BOTH signoutPath + redirectPathMatcher
// matching the same path; signout wins).
func TestDispatcher_SignoutPath_Highest_Priority(t *testing.T) {
	cc := newTestCompiledConfig()
	cc.signoutPath = func(path string) bool { return path == "/signout" }
	cc.redirectPathMatcher = func(path string) bool { return path == "/signout" } // would also match
	cc.redirectURI = "https://example.com/post-signout"
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	headers.Set(":path", "/signout")
	headers.Set(":method", "GET")

	status := f.DecodeHeaders(headers, true)

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if dcb.localReplyCount != 1 {
		t.Fatalf("localReplyCount: got %d, want 1", dcb.localReplyCount)
	}
	if dcb.lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (category (c) sign-out)", dcb.lastStatus)
	}
	// Sign-out wire shape: 5 Max-Age=0 cookies per SPEC §4.5 category (c).
	if got := countSetCookies(dcb.lastHeaders); got != 5 {
		t.Errorf("Set-Cookie count: got %d, want 5 (full envelope clearing per §4.5 category (c))", got)
	}
}

// TestDispatcher_CallbackPath_GET_HandlesCallback verifies a GET to the
// redirect_path_matcher routes to handleCallback per SPEC §6.3 step 2. The
// callback handler returns StopIteration (parks the decode goroutine on
// the async-resume primitive — Task 5 SKELETON; full body at Task 10).
//
// At Task 5 with a missing state cookie the path falls to handleBadState
// (category (d) 401). To assert dispatch reaches handleCallback (not the
// pass_through or cookie-validate branches), we drive a request with no
// state cookie present + assert the 401 emission.
func TestDispatcher_CallbackPath_GET_HandlesCallback(t *testing.T) {
	cc := newTestCompiledConfig()
	cc.redirectPathMatcher = func(path string) bool {
		return strings.HasPrefix(path, "/oauth2/callback")
	}
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	headers.Set(":path", "/oauth2/callback?code=abc&state=xyz")
	headers.Set(":method", "GET")

	status := f.DecodeHeaders(headers, true)

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (callback dispatch parks decode)", status)
	}
	// State cookie is absent → handleBadState fires (category (d) 401).
	if dcb.localReplyCount != 1 {
		t.Fatalf("localReplyCount: got %d, want 1", dcb.localReplyCount)
	}
	if dcb.lastStatus != 401 {
		t.Errorf("lastStatus: got %d, want 401 (bad-state-cookie → category (d))", dcb.lastStatus)
	}
}

// TestDispatcher_CallbackPath_POST_ParseRejects verifies POST to the
// callback path PARSE-REJECTs with category (d) 401 per SPEC §2.14 +
// planner-time D15. The POST callback form (response_mode=form_post)
// is out of scope at MVP.
//
// Per literal planner-time D15: "NO new counter (the standard
// `oauth_unauthorized_rq` increments per §4.6)" — the standard counter
// SHOULD increment on this PARSE-REJECT path. The post-dispatch counter
// assertion pins the literal-D15 behavior so a future regression that
// re-introduces a no-counter call-site split (cf. Task 8 commit df33e4b,
// later reverted in a Task 8 follow-up) is caught at this test.
func TestDispatcher_CallbackPath_POST_ParseRejects(t *testing.T) {
	cc := newTestCompiledConfig()
	cc.redirectPathMatcher = func(path string) bool {
		return strings.HasPrefix(path, "/oauth2/callback")
	}
	reg := stats.NewRegistry()
	cc.stats = newFilterStats(reg, "")
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	headers.Set(":path", "/oauth2/callback")
	headers.Set(":method", "POST")

	status := f.DecodeHeaders(headers, true)

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if dcb.lastStatus != 401 {
		t.Errorf("POST callback PARSE-REJECT: got status %d, want 401 (category (d) per §2.14 + D15)", dcb.lastStatus)
	}
	if dcb.lastBody != constUnauthorizedBody {
		t.Errorf("body: got %q, want %q (constant body per §4.3)", dcb.lastBody, constUnauthorizedBody)
	}
	// Per literal planner-time D15: "NO new counter (the standard
	// `oauth_unauthorized_rq` increments per §4.6)" — the POST-callback
	// PARSE-REJECT path increments the standard counter alongside the
	// bad-state-cookie path. Pins the D15 literal disposition; catches a
	// future regression that re-introduces the no-counter call-site split.
	if got := cc.stats.oauthUnauthorizedRq.Load(); got != 1 {
		t.Errorf("oauth_unauthorized_rq: got %d, want 1 (literal D15 — counter SHOULD increment on POST-callback PARSE-REJECT)", got)
	}
}

// TestDispatcher_PassThroughMatcher_Hits_Bypasses verifies the
// pass_through_matcher hit returns Continue + increments oauth_passthrough
// per SPEC §6.3 step 3 + §4.6.
func TestDispatcher_PassThroughMatcher_Hits_Bypasses(t *testing.T) {
	cc := newTestCompiledConfig()
	cc.passThroughMatcher = func(h headerLookup) bool {
		return h.Get("X-Bypass-OAuth2") == "true"
	}
	reg := stats.NewRegistry()
	cc.stats = newFilterStats(reg, "")
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	headers.Set(":path", "/api/v1/anything")
	headers.Set(":method", "GET")
	headers.Set("X-Bypass-OAuth2", "true")

	status := f.DecodeHeaders(headers, true)

	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (passthrough bypass)", status)
	}
	if dcb.localReplyCount != 0 {
		t.Errorf("localReplyCount: got %d, want 0 (passthrough does not emit a response)", dcb.localReplyCount)
	}
	if got := cc.stats.oauthPassthrough.Load(); got != 1 {
		t.Errorf("oauth_passthrough: got %d, want 1", got)
	}
}

// TestDispatcher_ValidCookieEnvelope_ContinuesDecoding verifies a valid
// envelope returns Continue per SPEC §6.3 step 4 + the cookie-validate
// happy path. The test composes a valid 5-cookie envelope with a valid
// HMAC + a non-expired OauthExpires.
func TestDispatcher_ValidCookieEnvelope_ContinuesDecoding(t *testing.T) {
	cc := newTestCompiledConfig()
	hmacSecret := []byte("test-hmac-secret-32-bytes-padding")
	cc.hmacSecretFn = func() []byte { return hmacSecret }
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	// Compose a valid envelope.
	const domain = "example.com"
	const expiresFuture = "9999999999" // year 2286 — future per any reasonable test run
	const bearer = "encrypted-bearer-token"
	const idToken = ""
	const refreshToken = "encrypted-refresh-token"
	hmacValue := computeHMAC(domain, expiresFuture, bearer, idToken, refreshToken, hmacSecret)

	headers := http.Header{}
	headers.Set(":path", "/api/v1/anything")
	headers.Set(":method", "GET")
	headers.Set("Host", domain)
	cookies := []string{
		cc.cookieNames.BearerToken + "=" + bearer,
		cc.cookieNames.OauthHMAC + "=" + hmacValue,
		cc.cookieNames.OauthExpires + "=" + expiresFuture,
		cc.cookieNames.RefreshToken + "=" + refreshToken,
	}
	headers.Set("Cookie", strings.Join(cookies, "; "))

	status := f.DecodeHeaders(headers, true)

	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (valid envelope ContinueDecoding)", status)
	}
	if dcb.localReplyCount != 0 {
		t.Errorf("localReplyCount: got %d, want 0 (valid envelope path does not emit a response)", dcb.localReplyCount)
	}
}

// TestDispatcher_ValidCookies_ForwardBearerToken_InjectsAuthorization
// verifies the forward_bearer_token=true Authorization-header injection
// per SPEC §2.15 + AMEND-6 C3 + handleValidCookies. The dispatcher
// decrypts the BearerToken and sets `Authorization: Bearer <decrypted>`
// on the request headers before returning Continue.
func TestDispatcher_ValidCookies_ForwardBearerToken_InjectsAuthorization(t *testing.T) {
	cc := newTestCompiledConfig()
	hmacSecret := []byte("test-hmac-secret-32-bytes-padding")
	cc.hmacSecretFn = func() []byte { return hmacSecret }
	cc.forwardBearerToken = true
	cc.disableTokenEncryption = true // simplest path — no AES round-trip
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	const domain = "example.com"
	const expiresFuture = "9999999999"
	const bearer = "plain-access-token"
	const idToken = ""
	const refreshToken = ""
	hmacValue := computeHMAC(domain, expiresFuture, bearer, idToken, refreshToken, hmacSecret)

	headers := http.Header{}
	headers.Set(":path", "/api/v1/anything")
	headers.Set(":method", "GET")
	headers.Set("Host", domain)
	cookies := []string{
		cc.cookieNames.BearerToken + "=" + bearer,
		cc.cookieNames.OauthHMAC + "=" + hmacValue,
		cc.cookieNames.OauthExpires + "=" + expiresFuture,
	}
	headers.Set("Cookie", strings.Join(cookies, "; "))

	status := f.DecodeHeaders(headers, true)

	if status != envoyhttp.Continue {
		t.Fatalf("status: got %v, want Continue", status)
	}
	got := headers.Get("Authorization")
	want := "Bearer " + bearer
	if got != want {
		t.Errorf("Authorization header injection: got %q, want %q", got, want)
	}
}

// TestDispatcher_ExpiredBearerToken_ValidRefreshToken_DispatchesRefresh
// verifies the expired-BearerToken + valid-RefreshToken leg dispatches to
// handleRefresh per SPEC §6.3 step 4 + ADR-0183. Task 8 wires the full
// async refresh POST — the dispatcher returns StopIteration synchronously
// + the async goroutine fires applyRefreshTokenResponse on POST completion.
//
// The Task 5 SKELETON assertion that "handleRefresh emits nothing" was
// re-targeted at Task 8: the synchronous StopIteration return + the async
// goroutine's deferred dispatch (success → ContinueDecoding; failure →
// SendLocalReply + ContinueDecoding) are the load-bearing invariants. The
// test waits on waitRefreshDone() (Task 8 helper) to synchronize with the
// async goroutine before inspecting the dcb state — this is the race-clean
// pattern per planner-time D14 + ADR-0183.
func TestDispatcher_ExpiredBearerToken_ValidRefreshToken_DispatchesRefresh(t *testing.T) {
	cc := newTestCompiledConfig()
	hmacSecret := []byte("test-hmac-secret-32-bytes-padding")
	cc.hmacSecretFn = func() []byte { return hmacSecret }
	cc.useRefreshToken = true
	// Task 8: inject a no-op success poster so the dispatcher reaches
	// handleRefresh + the async goroutine completes deterministically.
	// Without the poster, the goroutine would fire the no-poster-configured
	// failure leg (functionally equivalent for the dispatcher test, but
	// less explicit about the dispatch path's intent).
	cc.authorizationEndpoint = "https://idp.example.com/auth"
	reg := stats.NewRegistry()
	cc.stats = newFilterStats(reg, "")
	cc.tokenEndpointPoster = func(_ context.Context, _, _, _, _ string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	const domain = "example.com"
	const expiresPast = "1" // year 1970 — expired
	const bearer = "expired-bearer-token"
	const idToken = ""
	const refreshToken = "valid-refresh-token"
	hmacValue := computeHMAC(domain, expiresPast, bearer, idToken, refreshToken, hmacSecret)

	headers := http.Header{}
	headers.Set(":path", "/api/v1/anything")
	headers.Set(":method", "GET")
	headers.Set("Host", domain)
	cookies := []string{
		cc.cookieNames.BearerToken + "=" + bearer,
		cc.cookieNames.OauthHMAC + "=" + hmacValue,
		cc.cookieNames.OauthExpires + "=" + expiresPast,
		cc.cookieNames.RefreshToken + "=" + refreshToken,
	}
	headers.Set("Cookie", strings.Join(cookies, "; "))

	status := f.DecodeHeaders(headers, true)

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (refresh leg parks decode)", status)
	}

	// Task 8: wait on the async goroutine before inspecting dcb. The
	// goroutine fires applyRefreshTokenResponse on the success leg →
	// ContinueDecoding is called (no SendLocalReply).
	f.waitRefreshDone()

	if dcb.localReplyCount != 0 {
		t.Errorf("localReplyCount: got %d, want 0 (success-leg does not emit; ContinueDecoding instead)", dcb.localReplyCount)
	}
	if dcb.continueCount != 1 {
		t.Errorf("continueCount: got %d, want 1 (success-leg resumes the parked goroutine via ContinueDecoding per ADR-0183)", dcb.continueCount)
	}
	if got := cc.stats.oauthRefreshtokenSuccess.Load(); got != 1 {
		t.Errorf("oauth_refreshtoken_success: got %d, want 1 (success-leg counter per §4.6)", got)
	}
}

// TestDispatcher_Unauthenticated_EmitsCategory_A_302_Challenge verifies
// the default leg (no valid envelope) emits category (a) 302 + state
// cookie + cleared envelope cookies per SPEC §4.1 + §4.5.
func TestDispatcher_Unauthenticated_EmitsCategory_A_302_Challenge(t *testing.T) {
	cc := newTestCompiledConfig()
	cc.authorizationEndpoint = "https://idp.example.com/auth"
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	headers.Set(":path", "/api/v1/anything")
	headers.Set(":method", "GET")

	status := f.DecodeHeaders(headers, true)

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if dcb.localReplyCount != 1 {
		t.Fatalf("localReplyCount: got %d, want 1", dcb.localReplyCount)
	}
	if dcb.lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (category (a) auth-challenge)", dcb.lastStatus)
	}
	if got := getHeader(dcb.lastHeaders, "Location"); got != cc.authorizationEndpoint {
		t.Errorf("Location: got %q, want %q", got, cc.authorizationEndpoint)
	}
	// Category (a): 4 cleared cookies + 1 state cookie SET = 5 Set-Cookie headers.
	if got := countSetCookies(dcb.lastHeaders); got != 5 {
		t.Errorf("Set-Cookie count: got %d, want 5 (4 cleared envelope + 1 state SET per §4.5 category (a))", got)
	}
	// State cookie is SET to a non-empty value (Task 5 skeleton; Task 8/10
	// wire the HMAC-protected payload).
	stateCookiePresent := false
	for _, hf := range dcb.lastHeaders {
		if hf.Name != "Set-Cookie" {
			continue
		}
		if strings.HasPrefix(hf.Value, cc.stateCookieName+"=") && !strings.Contains(hf.Value, "Max-Age=0") {
			stateCookiePresent = true
		}
	}
	if !stateCookiePresent {
		t.Errorf("state cookie %q SET (non-clearing): expected present", cc.stateCookieName)
	}
}

// TestHandleBadState_EmitsCategory_D_401_With_Constant_Body verifies
// handleBadState emits the wire-exact category (d) 401 per SPEC §4.1 +
// §4.3 + AMEND-3. Includes the addFlowCookieDeletionHeaders cleanup.
func TestHandleBadState_EmitsCategory_D_401_With_Constant_Body(t *testing.T) {
	cc := newTestCompiledConfig()
	reg := stats.NewRegistry()
	cc.stats = newFilterStats(reg, "")
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handleBadState()

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if dcb.localReplyCount != 1 {
		t.Fatalf("localReplyCount: got %d, want 1", dcb.localReplyCount)
	}
	if dcb.lastStatus != 401 {
		t.Errorf("lastStatus: got %d, want 401 (category (d))", dcb.lastStatus)
	}
	if dcb.lastBody != constUnauthorizedBody {
		t.Errorf("body: got %q, want %q (constant body per §4.3)", dcb.lastBody, constUnauthorizedBody)
	}
	// addFlowCookieDeletionHeaders emits 5 Max-Age=0 Set-Cookie headers per
	// SPEC §4.5 category (d).
	if got := countSetCookies(dcb.lastHeaders); got != 5 {
		t.Errorf("Set-Cookie count: got %d, want 5 (flow-cookie deletion per §4.5 category (d))", got)
	}
	// Each Set-Cookie must contain Max-Age=0.
	for _, hf := range dcb.lastHeaders {
		if hf.Name != "Set-Cookie" {
			continue
		}
		if !strings.Contains(hf.Value, "Max-Age=0") {
			t.Errorf("Set-Cookie missing Max-Age=0: %q", hf.Value)
		}
	}
	// oauth_unauthorized_rq counter incremented per SPEC §4.6.
	if got := cc.stats.oauthUnauthorizedRq.Load(); got != 1 {
		t.Errorf("oauth_unauthorized_rq: got %d, want 1", got)
	}
}

// TestHandlePassThrough_NoOauth2Emission_IncrementsCounter verifies
// handlePassThrough returns Continue (no oauth2 emission) + increments
// the oauth_passthrough counter per SPEC §4.6.
func TestHandlePassThrough_NoOauth2Emission_IncrementsCounter(t *testing.T) {
	cc := newTestCompiledConfig()
	reg := stats.NewRegistry()
	cc.stats = newFilterStats(reg, "")
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handlePassThrough()

	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if dcb.localReplyCount != 0 {
		t.Errorf("localReplyCount: got %d, want 0 (passthrough does not emit)", dcb.localReplyCount)
	}
	if got := cc.stats.oauthPassthrough.Load(); got != 1 {
		t.Errorf("oauth_passthrough: got %d, want 1", got)
	}
}

// TestHandleUnauthenticated_EmitsCategory_A verifies handleUnauthenticated
// emits the category (a) wire shape per SPEC §4.1.
func TestHandleUnauthenticated_EmitsCategory_A(t *testing.T) {
	cc := newTestCompiledConfig()
	cc.authorizationEndpoint = "https://idp.example.com/auth"
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handleUnauthenticated()

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if dcb.lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (category (a))", dcb.lastStatus)
	}
	if dcb.lastBody != "" {
		t.Errorf("body: got %q, want empty (category (a))", dcb.lastBody)
	}
	if got := getHeader(dcb.lastHeaders, "Location"); got != cc.authorizationEndpoint {
		t.Errorf("Location: got %q, want %q", got, cc.authorizationEndpoint)
	}
}

// TestHandleRefreshFailure_EmitsCategory_A_IncrementsCounter verifies
// handleRefreshFailure emits the category (a) wire shape per SPEC §4.7 +
// increments oauth_refreshtoken_failure per SPEC §4.6.
func TestHandleRefreshFailure_EmitsCategory_A_IncrementsCounter(t *testing.T) {
	cc := newTestCompiledConfig()
	cc.authorizationEndpoint = "https://idp.example.com/auth"
	reg := stats.NewRegistry()
	cc.stats = newFilterStats(reg, "")
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handleRefreshFailure()

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if dcb.lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (category (a) per §4.7)", dcb.lastStatus)
	}
	if got := cc.stats.oauthRefreshtokenFailure.Load(); got != 1 {
		t.Errorf("oauth_refreshtoken_failure: got %d, want 1", got)
	}
}

// TestHandleValidCookies_NoForwardBearerToken_LeavesAuthorizationUntouched
// verifies that when forward_bearer_token=false (default), the dispatcher
// does NOT inject Authorization.
func TestHandleValidCookies_NoForwardBearerToken_LeavesAuthorizationUntouched(t *testing.T) {
	cc := newTestCompiledConfig()
	cc.forwardBearerToken = false
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer pre-existing")
	env := CookieEnvelope{BearerToken: "encrypted"}

	status := f.handleValidCookies(headers, env)

	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if got := headers.Get("Authorization"); got != "Bearer pre-existing" {
		t.Errorf("Authorization: got %q, want %q (untouched)", got, "Bearer pre-existing")
	}
}

// TestHandleValidCookies_PreserveAuthorizationHeader_NoOverwrite verifies
// preserveAuthorizationHeader=true skips Authorization injection when an
// existing Authorization header is present.
func TestHandleValidCookies_PreserveAuthorizationHeader_NoOverwrite(t *testing.T) {
	cc := newTestCompiledConfig()
	cc.forwardBearerToken = true
	cc.preserveAuthorizationHeader = true
	cc.disableTokenEncryption = true
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	headers.Set("Authorization", "Basic existing-creds")
	env := CookieEnvelope{BearerToken: "plain-token"}

	f.handleValidCookies(headers, env)

	if got := headers.Get("Authorization"); got != "Basic existing-creds" {
		t.Errorf("Authorization: got %q, want %q (preserved per preserveAuthorizationHeader)", got, "Basic existing-creds")
	}
}

// ---------------------------------------------------------------------------
// Per-route validator tests per SPEC §5.2 + planner-time D2
// ---------------------------------------------------------------------------

// fakeReg captures the RegisterPerRouteValidator invocation for the
// TestRegisterPerRouteValidator_Wiring assertion.
type fakeReg struct {
	registeredName      string
	registeredValidator func(proto.Message) error
}

func (r *fakeReg) RegisterPerRouteValidator(name string, v func(proto.Message) error) {
	r.registeredName = name
	r.registeredValidator = v
}

// TestRegisterPerRouteValidator_PARSE_REJECTS_RouteLevel_Placement
// verifies the per-route validator registers under the canonical filter
// name + rejects ANY non-nil message with the byte-stable D2 wording per
// SPEC §5.2.
func TestRegisterPerRouteValidator_PARSE_REJECTS_RouteLevel_Placement(t *testing.T) {
	reg := &fakeReg{}
	RegisterPerRouteValidator(reg)

	if reg.registeredName != filterName {
		t.Errorf("registered filterName: got %q, want %q", reg.registeredName, filterName)
	}
	if reg.registeredValidator == nil {
		t.Fatal("registered validator: got nil, want non-nil")
	}

	// The validator REJECTS any non-nil proto.Message at any tier per SPEC §5
	// — the v1.37.x proto has NO OAuth2PerRoute message at all. Drive with a
	// stand-in proto.Message (the validator does not type-assert; rejects
	// unconditionally).
	stand := &anypb.Any{TypeUrl: "type.googleapis.com/some.synthetic.Message"}
	err := reg.registeredValidator(stand)
	if err == nil {
		t.Fatal("validator returned nil error: want PARSE-REJECT")
	}
	if err.Error() != perRouteTPFCRejectMsg {
		t.Errorf("validator error wording: got %q, want %q (byte-stable per planner-time D2)", err.Error(), perRouteTPFCRejectMsg)
	}
}

// TestRegisterPerRouteValidator_ViaHTTPRegistry_Roundtrip verifies the
// validator wires correctly into the framework's *HTTPRegistry. Mirrors
// the header_mutation per-route validator's roundtrip pattern.
func TestRegisterPerRouteValidator_ViaHTTPRegistry_Roundtrip(t *testing.T) {
	r := envoyhttp.NewHTTPRegistry()
	RegisterPerRouteValidator(r)

	v := r.PerRouteValidator(filterName)
	if v == nil {
		t.Fatal("PerRouteValidator(filterName): got nil, want non-nil")
	}
	stand := &anypb.Any{TypeUrl: "type.googleapis.com/some.synthetic.Message"}
	err := v(stand)
	if err == nil {
		t.Fatal("validator returned nil error: want PARSE-REJECT")
	}
	if err.Error() != perRouteTPFCRejectMsg {
		t.Errorf("validator error: got %q, want %q", err.Error(), perRouteTPFCRejectMsg)
	}
}

// TestValidatePerRouteOAuth2_DirectInvocation verifies the unexported
// validator returns the byte-stable D2 wording on direct invocation.
func TestValidatePerRouteOAuth2_DirectInvocation(t *testing.T) {
	stand := &anypb.Any{TypeUrl: "type.googleapis.com/some.synthetic.Message"}
	err := validatePerRouteOAuth2(stand)
	if err == nil {
		t.Fatal("validatePerRouteOAuth2: got nil error, want non-nil")
	}
	if err.Error() != perRouteTPFCRejectMsg {
		t.Errorf("error wording: got %q, want %q", err.Error(), perRouteTPFCRejectMsg)
	}
}

// ---------------------------------------------------------------------------
// Factory + OnDestroy + DecodeData/DecodeTrailers skeleton tests
// ---------------------------------------------------------------------------

// TestNew_NilTypedConfig_ReturnsError verifies the Task 5 STUB New factory
// returns a fail-fast error on a nil typed_config per ADR-0072.
func TestNew_NilTypedConfig_ReturnsError(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("New(nil): got nil error, want non-nil")
	}
}

// TestNew_NonNilTypedConfig_WrongType_ReturnsUnmarshalError verifies the
// Task 11 New factory returns a clear unmarshal-error for any non-OAuth2
// typed_config. The synthetic *anypb.Any with a non-OAuth2 TypeUrl cannot
// be UnmarshalTo'd into *OAuth2 — the framework's proto runtime surfaces
// the mismatched-message-type error.
func TestNew_NonNilTypedConfig_WrongType_ReturnsUnmarshalError(t *testing.T) {
	stand := &anypb.Any{TypeUrl: "type.googleapis.com/some.synthetic.Message"}
	_, err := New(stand, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("New(wrong-type): got nil error, want unmarshal error")
	}
	if !strings.Contains(err.Error(), "oauth2: unmarshal") {
		t.Errorf("error wording: got %q, want prefix %q", err.Error(), "oauth2: unmarshal")
	}
	_ = errors.New // preserve import use; errors is still referenced elsewhere in this file
}

// TestDecodeData_PassThrough verifies DecodeData returns DataContinue
// unconditionally per SPEC §6.12 decoder-only discipline.
func TestDecodeData_PassThrough(t *testing.T) {
	f := &filter{}
	if got := f.DecodeData([]byte("body"), true); got != envoyhttp.DataContinue {
		t.Errorf("DecodeData: got %v, want DataContinue", got)
	}
}

// TestDecodeTrailers_PassThrough verifies DecodeTrailers returns
// TrailersContinue per SPEC §6.12 decoder-only discipline.
func TestDecodeTrailers_PassThrough(t *testing.T) {
	f := &filter{}
	if got := f.DecodeTrailers(http.Header{}); got != envoyhttp.TrailersContinue {
		t.Errorf("DecodeTrailers: got %v, want TrailersContinue", got)
	}
}

// TestOnDestroy_NilCallCancel_NoPanic verifies OnDestroy is a safe no-op
// when no token_endpoint POST was fired (the common 4-emission-category
// synchronous-deny case).
func TestOnDestroy_NilCallCancel_NoPanic(t *testing.T) {
	f := &filter{}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("OnDestroy panicked: %v", r)
		}
	}()
	f.OnDestroy()
	if !f.done {
		t.Error("done: got false, want true (OnDestroy must set done)")
	}
}

// TestOnDestroy_WithCallCancel_FiresCancellation verifies OnDestroy fires
// the callCancel closure to cancel the in-flight token_endpoint POST per
// SPEC §6.8.
func TestOnDestroy_WithCallCancel_FiresCancellation(t *testing.T) {
	var canceled atomic.Bool
	f := &filter{}
	f.callCancel = func() { canceled.Store(true) }
	f.OnDestroy()
	if !canceled.Load() {
		t.Error("callCancel: was not invoked")
	}
	if !f.done {
		t.Error("done: got false, want true")
	}
}

// TestSetDecoderCallbacks_Stores verifies SetDecoderCallbacks records the
// per-stream callbacks reference.
func TestSetDecoderCallbacks_Stores(t *testing.T) {
	f := &filter{}
	dcb := newFakeOAuth2DCB()
	f.SetDecoderCallbacks(dcb)
	if f.dcb == nil {
		t.Error("dcb: got nil, want stored value")
	}
}

// TestDecodeHeaders_NilCompiledConfig_ReturnsContinue verifies the
// ADR-0085 nil-tolerance guard in DecodeHeaders.
func TestDecodeHeaders_NilCompiledConfig_ReturnsContinue(t *testing.T) {
	f := &filter{} // cc is nil
	headers := http.Header{}
	if got := f.DecodeHeaders(headers, true); got != envoyhttp.Continue {
		t.Errorf("DecodeHeaders nil cc: got %v, want Continue (nil-tolerance per ADR-0085)", got)
	}
}

// ---------------------------------------------------------------------------
// Callback-helper unit tests
// ---------------------------------------------------------------------------

// TestExtractCallbackParams_HappyPath verifies the (code, state)
// extraction per SPEC §6.8 + RFC 6749 §4.1.2.
func TestExtractCallbackParams_HappyPath(t *testing.T) {
	code, state := extractCallbackParams("/oauth2/callback?code=ABC&state=XYZ")
	if code != "ABC" {
		t.Errorf("code: got %q, want %q", code, "ABC")
	}
	if state != "XYZ" {
		t.Errorf("state: got %q, want %q", state, "XYZ")
	}
}

// TestExtractCallbackParams_NoQueryString verifies the no-query case
// returns empty strings.
func TestExtractCallbackParams_NoQueryString(t *testing.T) {
	code, state := extractCallbackParams("/oauth2/callback")
	if code != "" || state != "" {
		t.Errorf("got (%q, %q), want both empty", code, state)
	}
}

// TestValidateStateCookie_Matches verifies byte-equality compare per Task
// 5 SKELETON (Task 8/10 wire the HMAC-protected composition).
func TestValidateStateCookie_Matches(t *testing.T) {
	if !validateStateCookie("xyz", "xyz") {
		t.Error("matching state should validate")
	}
}

// TestValidateStateCookie_Mismatches verifies a state-cookie mismatch
// fails validation.
func TestValidateStateCookie_Mismatches(t *testing.T) {
	if validateStateCookie("xyz", "abc") {
		t.Error("mismatched state should NOT validate")
	}
}

// TestValidateStateCookie_EmptyValuesFail verifies empty values on either
// side fail validation per AMEND-3 deny-path.
func TestValidateStateCookie_EmptyValuesFail(t *testing.T) {
	if validateStateCookie("", "xyz") {
		t.Error("empty cookie value should fail")
	}
	if validateStateCookie("xyz", "") {
		t.Error("empty query value should fail")
	}
}

// TestIsExpired_PastExpires_ReturnsTrue verifies the expiry-check.
func TestIsExpired_PastExpires_ReturnsTrue(t *testing.T) {
	if !isExpired("1") {
		t.Error("past epoch should be expired")
	}
}

// TestIsExpired_FutureExpires_ReturnsFalse verifies a future epoch is not
// expired.
func TestIsExpired_FutureExpires_ReturnsFalse(t *testing.T) {
	if isExpired("9999999999") {
		t.Error("future epoch should NOT be expired")
	}
}

// TestIsExpired_EmptyOrMalformed_ReturnsTrue verifies malformed values
// fall to the expired branch per the dispatcher's deny-path discipline.
func TestIsExpired_EmptyOrMalformed_ReturnsTrue(t *testing.T) {
	if !isExpired("") {
		t.Error("empty should be expired")
	}
	if !isExpired("not-a-number") {
		t.Error("malformed should be expired")
	}
}

// TestCompiledConfig_Fields_ForwardStable asserts the Task 5 STUB
// compiledConfig struct exposes the fields Task 8 + Task 9 + Task 10 +
// Task 11 will fill. Each field is assigned a non-zero value to anchor
// the forward-stability contract — a regression that renames or removes
// any of these fields surfaces at this assignment-site.
//
// Per planner-time D11 + Task 11 deferral discipline: Task 5 ships the
// struct shape; Task 11 wires `buildCompiledConfig` to populate from
// proto. This test acts as the inter-task forward-stability anchor.
func TestCompiledConfig_Fields_ForwardStable(t *testing.T) {
	cc := newTestCompiledConfig()
	// Task 11 deferred fields — each referenced here to anchor the
	// struct-shape contract. The actual proto-driven population lands at
	// Task 11; this test only checks the field exists + accepts an
	// assignment.
	// Task 9 settled the denyRedirectMatcher field shape to
	// denyRedirectMatcherFn (returns (redirectURL, matched)) per ADR-0184 +
	// SPEC §4.4 category (c). The shape was a pathMatcherFn at Task 5 +
	// the consumer was unspecified; Task 9 (signout.go) settles the consumer
	// + the (url, ok) return tuple needed to populate the (c) 302 Location
	// header.
	cc.denyRedirectMatcher = func(headerLookup) (string, bool) { return "", false }
	cc.clientID = "test-client-id"
	cc.clientSecretFn = func() []byte { return []byte("test-secret") }
	cc.defaultExpiresIn = 3600
	cc.csrfTokenExpiresIn = 600
	var keyBytes [32]byte
	cc.aesKey.Store(&keyBytes)

	if cc.denyRedirectMatcher == nil {
		t.Error("denyRedirectMatcher: assignment did not stick")
	}
	if cc.clientID != "test-client-id" {
		t.Errorf("clientID: got %q, want %q", cc.clientID, "test-client-id")
	}
	if cc.clientSecretFn == nil {
		t.Error("clientSecretFn: assignment did not stick")
	}
	if cc.defaultExpiresIn != 3600 {
		t.Errorf("defaultExpiresIn: got %v, want 3600", cc.defaultExpiresIn)
	}
	if cc.csrfTokenExpiresIn != 600 {
		t.Errorf("csrfTokenExpiresIn: got %v, want 600", cc.csrfTokenExpiresIn)
	}
	if cc.aesKey.Load() == nil {
		t.Error("aesKey: atomic.Pointer.Store did not publish")
	}
}

// TestApplyTokenEndpointResponse_OnDestroyGuard_Skeleton verifies the
// applyTokenEndpointResponse done-guard short-circuits the (nil, nil)
// no-op path under OnDestroy per ADR-0159 D4. The full-body variant
// (TestApplyTokenEndpointResponse_OnDestroyGuard in callback_test.go)
// asserts the same invariant against a non-nil 2xx response.
func TestApplyTokenEndpointResponse_OnDestroyGuard_Skeleton(t *testing.T) {
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: newTestCompiledConfig(), dcb: dcb}
	f.OnDestroy() // sets done=true
	f.applyTokenEndpointResponse(nil, nil)
	if dcb.localReplyCount != 0 {
		t.Errorf("localReplyCount: got %d, want 0 (done-guard must prevent dcb touch)", dcb.localReplyCount)
	}
}

// ---------------------------------------------------------------------------
// Task 9 — handleSignout tests per SPEC §4.1 category (c) + §4.5 + §6.9 +
// ADR-0184. Sign-out flow: full envelope clearing (Max-Age=0 for all 5
// cookies) + deny_redirect_matcher integration for the Location header.
// NO separate signout_completed counter per AMEND-4 + S5 (the 302 emission
// IS the sign-out completion event).
// ---------------------------------------------------------------------------

// signoutHeaders constructs the inbound-request header set the sign-out
// dispatch path consumes. The :path is matched against signoutPath; the
// :method is GET in the common case (sign-out links / form GETs).
func signoutHeaders() http.Header {
	h := http.Header{}
	h.Set(":path", "/signout")
	h.Set(":method", "GET")
	return h
}

// newSignoutFilter constructs a minimal *filter wired for the handleSignout
// call-site: cc with signoutPath matcher + dcb + an optional stats Registry
// for the no-counter assertion. The caller populates redirectURI +
// denyRedirectMatcher per-test.
func newSignoutFilter(t *testing.T) (*filter, *compiledConfig, *fakeOAuth2DCB) {
	t.Helper()
	cc := newTestCompiledConfig()
	cc.signoutPath = func(path string) bool { return path == "/signout" }
	reg := stats.NewRegistry()
	cc.stats = newFilterStats(reg, "")
	dcb := newFakeOAuth2DCB()
	f := &filter{cc: cc, dcb: dcb}
	return f, cc, dcb
}

// TestHandleSignout_EmitsCategory_C_302_With_MaxAge0_AllCookies pins the
// wire shape per SPEC §4.5 category (c): 302 status + 5 Set-Cookie headers
// each carrying Max-Age=0 for the full envelope clearing (BearerToken /
// OauthHMAC / OauthExpires / IdToken / RefreshToken).
func TestHandleSignout_EmitsCategory_C_302_With_MaxAge0_AllCookies(t *testing.T) {
	f, cc, dcb := newSignoutFilter(t)
	cc.redirectURI = "https://example.com/post-signout"

	status := f.handleSignout(signoutHeaders())

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if dcb.localReplyCount != 1 {
		t.Fatalf("localReplyCount: got %d, want 1", dcb.localReplyCount)
	}
	if dcb.lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (category (c) sign-out)", dcb.lastStatus)
	}
	if dcb.lastBody != "" {
		t.Errorf("body: got %q, want empty (category (c) sign-out per §4.1)", dcb.lastBody)
	}
	// Full envelope clearing: 5 Max-Age=0 Set-Cookie headers per §4.5
	// category (c) — BearerToken / OauthHMAC / OauthExpires / IdToken /
	// RefreshToken.
	if got := countSetCookies(dcb.lastHeaders); got != 5 {
		t.Errorf("Set-Cookie count: got %d, want 5 (full envelope clearing per §4.5 category (c))", got)
	}
	for _, hf := range dcb.lastHeaders {
		if hf.Name != "Set-Cookie" {
			continue
		}
		if !strings.Contains(hf.Value, "Max-Age=0") {
			t.Errorf("Set-Cookie missing Max-Age=0: %q", hf.Value)
		}
	}
}

// TestHandleSignout_DenyRedirectMatcher_Honored_When_Match pins the Location
// header per ADR-0184 + SPEC §4.4 category (c): when the
// denyRedirectMatcher returns (url, true), the 302 Location is the matcher-
// supplied URL (NOT cc.redirectURI). The matcher takes precedence.
func TestHandleSignout_DenyRedirectMatcher_Honored_When_Match(t *testing.T) {
	f, cc, dcb := newSignoutFilter(t)
	cc.redirectURI = "https://example.com/default-fallback"
	const matcherURL = "https://example.com/post-signout-landing"
	cc.denyRedirectMatcher = func(h headerLookup) (string, bool) {
		// Match on a sentinel header to keep the test deterministic.
		if h.Get("X-Signout-Target") == "landing" {
			return matcherURL, true
		}
		return "", false
	}

	// Drive through DecodeHeaders so the matcher closure receives a real
	// header carrier. signoutHeaders() supplies :path=/signout +
	// :method=GET; we set the sentinel header.
	headers := signoutHeaders()
	headers.Set("X-Signout-Target", "landing")

	status := f.DecodeHeaders(headers, true)

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if got := getHeader(dcb.lastHeaders, "Location"); got != matcherURL {
		t.Errorf("Location: got %q, want %q (denyRedirectMatcher precedence per ADR-0184)", got, matcherURL)
	}
}

// TestHandleSignout_NoMatch_FallsBackToRedirectURI pins the default Location
// per ADR-0184 fall-back: when the denyRedirectMatcher does NOT match (or
// is nil), the 302 Location is cc.redirectURI verbatim.
func TestHandleSignout_NoMatch_FallsBackToRedirectURI(t *testing.T) {
	f, cc, dcb := newSignoutFilter(t)
	const fallback = "https://example.com/post-signout"
	cc.redirectURI = fallback
	// denyRedirectMatcher left nil → guaranteed miss.

	status := f.DecodeHeaders(signoutHeaders(), true)

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if got := getHeader(dcb.lastHeaders, "Location"); got != fallback {
		t.Errorf("Location: got %q, want %q (cc.redirectURI fall-back)", got, fallback)
	}
}

// TestHandleSignout_EmptyRedirectURI_DefaultLocation pins the
// denyRedirectMatcher-miss + cc.redirectURI-empty edge case. Per ADR-0184
// §Context: the Location is "empty for browser-default" when neither the
// matcher matches nor cc.redirectURI is configured. The 302 emission
// proceeds with an empty Location string (the browser typically renders
// the current page / no-op).
func TestHandleSignout_EmptyRedirectURI_DefaultLocation(t *testing.T) {
	f, _, dcb := newSignoutFilter(t)
	// cc.redirectURI left empty; denyRedirectMatcher left nil.

	status := f.handleSignout(signoutHeaders())

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if dcb.lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (category (c) sign-out — emission proceeds even with empty Location)", dcb.lastStatus)
	}
	if got := getHeader(dcb.lastHeaders, "Location"); got != "" {
		t.Errorf("Location: got %q, want empty (browser-default per ADR-0184)", got)
	}
	// Envelope clearing still happens regardless of Location.
	if got := countSetCookies(dcb.lastHeaders); got != 5 {
		t.Errorf("Set-Cookie count: got %d, want 5 (full envelope clearing per §4.5)", got)
	}
}

// TestHandleSignout_NoSeparateCounter_For_Signout pins AMEND-4 + S5 + §4.6:
// the sign-out emission does NOT increment ANY of the 6 oauth counters.
// The 302 emission IS the sign-out completion event (operator observability
// via downstream access-logs); no per-filter counter fires.
func TestHandleSignout_NoSeparateCounter_For_Signout(t *testing.T) {
	f, cc, _ := newSignoutFilter(t)
	cc.redirectURI = "https://example.com/post-signout"

	_ = f.handleSignout(signoutHeaders())

	// Assert all 6 counters at zero per AMEND-4 + S5.
	if got := cc.stats.oauthUnauthorizedRq.Load(); got != 0 {
		t.Errorf("oauth_unauthorized_rq: got %d, want 0 (no counter on sign-out per AMEND-4)", got)
	}
	if got := cc.stats.oauthFailure.Load(); got != 0 {
		t.Errorf("oauth_failure: got %d, want 0", got)
	}
	if got := cc.stats.oauthPassthrough.Load(); got != 0 {
		t.Errorf("oauth_passthrough: got %d, want 0", got)
	}
	if got := cc.stats.oauthSuccess.Load(); got != 0 {
		t.Errorf("oauth_success: got %d, want 0", got)
	}
	if got := cc.stats.oauthRefreshtokenSuccess.Load(); got != 0 {
		t.Errorf("oauth_refreshtoken_success: got %d, want 0", got)
	}
	if got := cc.stats.oauthRefreshtokenFailure.Load(); got != 0 {
		t.Errorf("oauth_refreshtoken_failure: got %d, want 0", got)
	}
}

// TestHandleSignout_OnDestroyGuard_NoPanic pins the OnDestroy guard per
// ADR-0159 D4 precedent. If sign-out is dispatched after the stream's
// OnDestroy fired (no realistic race given handleSignout is synchronous,
// but the guard mirrors other handlers' no-touch-dcb-after-OnDestroy
// discipline), no dcb touch happens.
func TestHandleSignout_OnDestroyGuard_NoPanic(t *testing.T) {
	f, cc, dcb := newSignoutFilter(t)
	cc.redirectURI = "https://example.com/post-signout"
	f.OnDestroy() // sets done=true

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleSignout panicked after OnDestroy: %v", r)
		}
	}()
	status := f.handleSignout(signoutHeaders())
	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (guard still returns terminal status)", status)
	}
	if dcb.localReplyCount != 0 {
		t.Errorf("localReplyCount: got %d, want 0 (done-guard must prevent dcb touch)", dcb.localReplyCount)
	}
}

// TestHandleSignout_DenyRedirectMatcher_NilOK_DoesNotPanic pins the
// nil-matcher tolerance per ADR-0085 + the dispatcher's nil-matcher-is-miss
// convention. handleSignout MUST tolerate a nil denyRedirectMatcher
// without panic; the fall-back path uses cc.redirectURI.
func TestHandleSignout_DenyRedirectMatcher_NilOK_DoesNotPanic(t *testing.T) {
	f, cc, dcb := newSignoutFilter(t)
	cc.redirectURI = "https://example.com/post-signout"
	cc.denyRedirectMatcher = nil

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleSignout panicked on nil denyRedirectMatcher: %v", r)
		}
	}()
	status := f.handleSignout(signoutHeaders())
	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if got := getHeader(dcb.lastHeaders, "Location"); got != cc.redirectURI {
		t.Errorf("Location: got %q, want %q (cc.redirectURI fall-back on nil matcher)", got, cc.redirectURI)
	}
}

// ---------------------------------------------------------------------------
// Group 1 — PARSE-REJECT byte-stable wording tests per planner-time D2
// (PLAN lines 119-133). Each row asserts (err != nil) AND (err.Error() ==
// expected D2 reference string). Per ADR-0080 envoy-go-strict discipline +
// the SPEC §12 byte-stable wording acceptance gate.
//
// The tests use the in-process oauth2.New factory (which wraps
// buildCompiledConfig). Each row constructs a *oauth2v3.OAuth2 proto with
// the field-under-test populated to trigger the PARSE-REJECT path; the
// remaining fields use the minimum-viable-shape established by
// validOAuth2Config (a 4-field skeleton: token_endpoint + authorization_
// endpoint + redirect_uri + credentials.client_id + credentials.hmac_secret
// pointing at a per-test temp file).
// ---------------------------------------------------------------------------

// validOAuth2Config returns a minimum-viable OAuth2 proto skeleton for
// PARSE-REJECT tests. Callers mutate field(s) per-test to trigger the
// PARSE-REJECT path. The skeleton uses a temp hmac_secret file so the
// success-path is intact when no mutation triggers a reject.
func validOAuth2Config(t *testing.T) *oauth2v3.OAuth2 {
	t.Helper()
	dir := t.TempDir()
	hmacPath := writeTempSecret(t, dir, "hmac.json", "test-hmac-secret-bytes-32-byte-pad")
	clientPath := writeTempSecret(t, dir, "client_secret.json", "test-client-secret")
	return &oauth2v3.OAuth2{
		Config: &oauth2v3.OAuth2Config{
			TokenEndpoint: &corev3.HttpUri{
				Uri: "https://idp.example.com/token",
				HttpUpstreamType: &corev3.HttpUri_Cluster{
					Cluster: "idp-cluster",
				},
			},
			AuthorizationEndpoint: "https://idp.example.com/auth",
			RedirectUri:           "https://app.example.com/oauth2/callback",
			Credentials: &oauth2v3.OAuth2Credentials{
				ClientId: "test-client-id",
				TokenFormation: &oauth2v3.OAuth2Credentials_HmacSecret{
					HmacSecret: pathSdsSecret("hmac", hmacPath),
				},
				TokenSecret: pathSdsSecret("client_secret", clientPath),
			},
		},
	}
}

// writeTempSecret writes the supplied inline string as a JSON Secret-proto
// envelope at dir/name + returns the absolute path. The format is
// `{"name":"...","generic_secret":{"secret":{"inline_string":"..."}}}` per
// the makeSecretAccessor JSON-parse convention.
func writeTempSecret(t *testing.T, dir, name, inline string) string {
	t.Helper()
	path := dir + "/" + name
	body := `{"name":"` + name + `","generic_secret":{"secret":{"inline_string":"` + inline + `"}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp secret %s: %v", path, err)
	}
	return path
}

// pathSdsSecret returns a *SdsSecretConfig wired with the supplied filesystem
// path via the PathConfigSource oneof arm (the canonical filesystem-SDS
// arm per SPEC §20.P6 RATIFIED).
func pathSdsSecret(name, path string) *tlsv3.SdsSecretConfig {
	return &tlsv3.SdsSecretConfig{
		Name: name,
		SdsConfig: &corev3.ConfigSource{
			ConfigSourceSpecifier: &corev3.ConfigSource_PathConfigSource{
				PathConfigSource: &corev3.PathConfigSource{Path: path},
			},
		},
	}
}

// newOAuth2Any wraps a *oauth2v3.OAuth2 proto into an *anypb.Any envelope
// for the New() factory call.
func newOAuth2Any(t *testing.T, msg *oauth2v3.OAuth2) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// parseRejectCase is one row in the Group 1 PARSE-REJECT table.
type parseRejectCase struct {
	name    string
	mutate  func(*oauth2v3.OAuth2)
	wantErr string
}

// runParseRejectRow drives one PARSE-REJECT row: build skeleton + apply
// mutate + call New + assert err byte-exact.
func runParseRejectRow(t *testing.T, tc parseRejectCase) {
	t.Helper()
	msg := validOAuth2Config(t)
	tc.mutate(msg)
	any := newOAuth2Any(t, msg)
	_, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatalf("%s: got nil error, want %q", tc.name, tc.wantErr)
	}
	if err.Error() != tc.wantErr {
		t.Errorf("%s: error wording: got %q, want %q", tc.name, err.Error(), tc.wantErr)
	}
}

// TestParseConfig_PARSE_REJECT_AuthorizationEndpoint_Empty pins the byte-
// stable D2 wording for the empty authorization_endpoint case per SPEC §6.2.
func TestParseConfig_PARSE_REJECT_AuthorizationEndpoint_Empty(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name:    "authorization_endpoint empty",
		mutate:  func(m *oauth2v3.OAuth2) { m.Config.AuthorizationEndpoint = "" },
		wantErr: "oauth2: authorization_endpoint empty",
	})
}

// TestParseConfig_PARSE_REJECT_RedirectURI_Empty pins the byte-stable D2
// wording for the empty redirect_uri case per SPEC §6.2.
func TestParseConfig_PARSE_REJECT_RedirectURI_Empty(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name:    "redirect_uri empty",
		mutate:  func(m *oauth2v3.OAuth2) { m.Config.RedirectUri = "" },
		wantErr: "oauth2: redirect_uri empty",
	})
}

// TestParseConfig_PARSE_REJECT_ClientID_Empty pins the byte-stable D2
// wording for the empty client_id case per SPEC §6.2.
func TestParseConfig_PARSE_REJECT_ClientID_Empty(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name:    "client_id empty",
		mutate:  func(m *oauth2v3.OAuth2) { m.Config.Credentials.ClientId = "" },
		wantErr: "oauth2: client_id empty",
	})
}

// TestParseConfig_PARSE_REJECT_BasicAuth pins the byte-stable D2 wording for
// the OAuth2Credentials.basic_auth (auth_type=BASIC_AUTH) case per AMEND-5 +
// §2.3.
func TestParseConfig_PARSE_REJECT_BasicAuth(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name:    "BASIC_AUTH",
		mutate:  func(m *oauth2v3.OAuth2) { m.Config.AuthType = oauth2v3.OAuth2Config_BASIC_AUTH },
		wantErr: "oauth2: OAuth2Credentials.basic_auth not supported; use client_secret_post (token_endpoint POST body)",
	})
}

// TestParseConfig_PARSE_REJECT_PKCE_OauthNonce pins the byte-stable D2
// wording for the PKCE oauth_nonce field per §2.1.
func TestParseConfig_PARSE_REJECT_PKCE_OauthNonce(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name: "PKCE oauth_nonce set",
		mutate: func(m *oauth2v3.OAuth2) {
			m.Config.Credentials.CookieNames = &oauth2v3.OAuth2Credentials_CookieNames{
				OauthNonce: "Nonce",
			}
		},
		wantErr: "oauth2: use_pkce + PKCE-related fields not supported in MVP",
	})
}

// TestParseConfig_PARSE_REJECT_SDS_ApiConfigSource pins the byte-stable D2
// wording for the ApiConfigSource arm per SPEC §2.11 + §20.P6.
func TestParseConfig_PARSE_REJECT_SDS_ApiConfigSource(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name: "SDS ApiConfigSource arm",
		mutate: func(m *oauth2v3.OAuth2) {
			m.Config.Credentials.TokenFormation = &oauth2v3.OAuth2Credentials_HmacSecret{
				HmacSecret: &tlsv3.SdsSecretConfig{
					Name: "hmac",
					SdsConfig: &corev3.ConfigSource{
						ConfigSourceSpecifier: &corev3.ConfigSource_ApiConfigSource{
							ApiConfigSource: &corev3.ApiConfigSource{},
						},
					},
				},
			}
		},
		wantErr: "oauth2: ApiConfigSource ConfigSource arm not supported; only filesystem PathConfigSource is supported",
	})
}

// TestParseConfig_PARSE_REJECT_SDS_Ads pins the byte-stable D2 wording for
// the Ads ConfigSource arm per SPEC §2.11.
func TestParseConfig_PARSE_REJECT_SDS_Ads(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name: "SDS Ads arm",
		mutate: func(m *oauth2v3.OAuth2) {
			m.Config.Credentials.TokenFormation = &oauth2v3.OAuth2Credentials_HmacSecret{
				HmacSecret: &tlsv3.SdsSecretConfig{
					Name: "hmac",
					SdsConfig: &corev3.ConfigSource{
						ConfigSourceSpecifier: &corev3.ConfigSource_Ads{
							Ads: &corev3.AggregatedConfigSource{},
						},
					},
				},
			}
		},
		wantErr: "oauth2: Ads ConfigSource arm not supported; only filesystem PathConfigSource is supported",
	})
}

// TestParseConfig_PARSE_REJECT_SDS_DeprecatedPath pins the byte-stable D2
// wording for the deprecated ConfigSource.path field 1 case per §2.11 +
// §20.P6. Uses the Path arm directly (without going through PathConfigSource).
func TestParseConfig_PARSE_REJECT_SDS_DeprecatedPath(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name: "SDS deprecated Path field",
		mutate: func(m *oauth2v3.OAuth2) {
			m.Config.Credentials.TokenFormation = &oauth2v3.OAuth2Credentials_HmacSecret{
				HmacSecret: &tlsv3.SdsSecretConfig{
					Name: "hmac",
					SdsConfig: &corev3.ConfigSource{
						ConfigSourceSpecifier: &corev3.ConfigSource_Path{
							Path: "/some/file/path",
						},
					},
				},
			}
		},
		wantErr: "oauth2: deprecated ConfigSource.path field 1 not supported; use PathConfigSource (oneof arm field 8)",
	})
}

// TestParseConfig_PARSE_REJECT_SDS_DeprecatedPath_NilPathConfigSource pins
// the byte-stable D2 wording for the empty-PathConfigSource case (nil-inner-
// path-config-source maps to the same deprecated-path rejection per the
// validation discipline).
func TestParseConfig_PARSE_REJECT_SDS_DeprecatedPath_NilPathConfigSource(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name: "SDS nil PathConfigSource",
		mutate: func(m *oauth2v3.OAuth2) {
			m.Config.Credentials.TokenFormation = &oauth2v3.OAuth2Credentials_HmacSecret{
				HmacSecret: &tlsv3.SdsSecretConfig{
					Name: "hmac",
					SdsConfig: &corev3.ConfigSource{
						ConfigSourceSpecifier: &corev3.ConfigSource_PathConfigSource{
							PathConfigSource: &corev3.PathConfigSource{Path: ""},
						},
					},
				},
			}
		},
		wantErr: "oauth2: deprecated ConfigSource.path field 1 not supported; use PathConfigSource (oneof arm field 8)",
	})
}

// TestParseConfig_PARSE_REJECT_AuthorizationEndpoint_TakesPrecedenceOverRedirect
// pins the dispatch order — authorization_endpoint validation fires BEFORE
// redirect_uri validation per buildCompiledConfig step order. With BOTH
// empty, the auth-endpoint wording wins.
func TestParseConfig_PARSE_REJECT_AuthorizationEndpoint_TakesPrecedenceOverRedirect(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name: "auth-endpoint precedence vs redirect_uri",
		mutate: func(m *oauth2v3.OAuth2) {
			m.Config.AuthorizationEndpoint = ""
			m.Config.RedirectUri = ""
		},
		wantErr: "oauth2: authorization_endpoint empty",
	})
}

// TestParseConfig_PARSE_REJECT_TokenEndpoint_EmptyURI pins the
// token_endpoint URL invalid wording for the empty-URI case per SPEC §6.2.
func TestParseConfig_PARSE_REJECT_TokenEndpoint_EmptyURI(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name: "token_endpoint empty URI",
		mutate: func(m *oauth2v3.OAuth2) {
			m.Config.TokenEndpoint = &corev3.HttpUri{
				Uri:              "",
				HttpUpstreamType: &corev3.HttpUri_Cluster{Cluster: "x"},
			}
		},
		wantErr: "oauth2: token_endpoint URL invalid: empty URI",
	})
}

// TestParseConfig_PARSE_REJECT_TokenEndpoint_Missing pins the token_endpoint
// missing wording for the nil-token_endpoint case per SPEC §6.2.
func TestParseConfig_PARSE_REJECT_TokenEndpoint_Missing(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name: "token_endpoint missing",
		mutate: func(m *oauth2v3.OAuth2) {
			m.Config.TokenEndpoint = nil
		},
		wantErr: "oauth2: token_endpoint URL invalid: token_endpoint missing",
	})
}

// TestParseConfig_PARSE_REJECT_TokenEndpoint_MalformedURI pins the
// token_endpoint URL invalid wording for the malformed-URI case. The stdlib
// url.Parse error tail is included in the wording per D2.
func TestParseConfig_PARSE_REJECT_TokenEndpoint_MalformedURI(t *testing.T) {
	msg := validOAuth2Config(t)
	// url.Parse rejects strings with `\x00` per the C0 control-char filter
	// (per net/url.Parse documentation + Go runtime invariants).
	msg.Config.TokenEndpoint = &corev3.HttpUri{
		Uri:              "ht\x00tp://invalid\x00.example.com/",
		HttpUpstreamType: &corev3.HttpUri_Cluster{Cluster: "x"},
	}
	any := newOAuth2Any(t, msg)
	_, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("token_endpoint malformed URI: got nil error, want non-nil")
	}
	if !strings.HasPrefix(err.Error(), "oauth2: token_endpoint URL invalid: ") {
		t.Errorf("token_endpoint malformed URI: got %q, want prefix %q", err.Error(), "oauth2: token_endpoint URL invalid: ")
	}
}

// TestParseConfig_PARSE_REJECT_DisableEncryption_Default_NoHmacSecret pins
// the byte-stable D2 wording for the disable_token_encryption=false (the
// proto default in v1.32.4 since the field is absent) + missing hmac_secret
// case per SPEC §6.2. Triggered by setting hmac_secret to nil.
func TestParseConfig_PARSE_REJECT_DisableEncryption_Default_NoHmacSecret(t *testing.T) {
	runParseRejectRow(t, parseRejectCase{
		name: "encryption ON + no hmac_secret",
		mutate: func(m *oauth2v3.OAuth2) {
			m.Config.Credentials.TokenFormation = nil
		},
		wantErr: "oauth2: disable_token_encryption=false requires non-empty hmac_secret",
	})
}

// TestParseConfig_NilTypedConfig pins the nil-tc fail-fast per ADR-0072.
func TestParseConfig_NilTypedConfig(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("New(nil): got nil error, want non-nil")
	}
	if err.Error() != "oauth2: typed_config required" {
		t.Errorf("error wording: got %q, want %q", err.Error(), "oauth2: typed_config required")
	}
}

// TestParseConfig_HappyPath_ValidConfig_NoError pins the success path —
// the minimum-viable skeleton parses without error.
func TestParseConfig_HappyPath_ValidConfig_NoError(t *testing.T) {
	msg := validOAuth2Config(t)
	any := newOAuth2Any(t, msg)
	factory, err := New(any, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("New(valid): got err %v, want nil", err)
	}
	if factory == nil {
		t.Fatal("factory: got nil, want non-nil")
	}
	// Invoke the factory once to verify the closure surface.
	hf := factory()
	if hf.Name != filterName {
		t.Errorf("HTTPFilter.Name: got %q, want %q", hf.Name, filterName)
	}
	if hf.Decoder == nil {
		t.Error("HTTPFilter.Decoder: got nil, want non-nil (decoder-only)")
	}
	if hf.Encoder != nil {
		t.Error("HTTPFilter.Encoder: got non-nil, want nil (decoder-only per SPEC §6.12)")
	}
}

// TestParseConfig_HappyPath_BehavioralKnobs_Captured verifies the
// buildCompiledConfig captures the forward_bearer_token + preserve_
// authorization_header + use_refresh_token + auth_scopes + resources + the
// default_expires_in fields from the proto into the compiledConfig.
func TestParseConfig_HappyPath_BehavioralKnobs_Captured(t *testing.T) {
	msg := validOAuth2Config(t)
	msg.Config.ForwardBearerToken = true
	msg.Config.PreserveAuthorizationHeader = false
	msg.Config.UseRefreshToken = &wrapperspb.BoolValue{Value: true}
	msg.Config.AuthScopes = []string{"openid", "profile"}
	msg.Config.Resources = []string{"resource-a"}
	msg.Config.DefaultExpiresIn = durationpb.New(3600 * time.Second)

	any := newOAuth2Any(t, msg)
	factory, err := New(any, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hf := factory()
	df, ok := hf.Decoder.(*filter)
	if !ok {
		t.Fatalf("Decoder type: got %T, want *filter", hf.Decoder)
	}
	cc := df.cc
	if !cc.forwardBearerToken {
		t.Error("forwardBearerToken: got false, want true")
	}
	if cc.preserveAuthorizationHeader {
		t.Error("preserveAuthorizationHeader: got true, want false")
	}
	if !cc.useRefreshToken {
		t.Error("useRefreshToken: got false, want true")
	}
	if cc.defaultExpiresIn != 3600*time.Second {
		t.Errorf("defaultExpiresIn: got %v, want %v", cc.defaultExpiresIn, 3600*time.Second)
	}
	if len(cc.authScopes) != 2 || cc.authScopes[0] != "openid" {
		t.Errorf("authScopes: got %v, want [openid profile]", cc.authScopes)
	}
	if len(cc.resources) != 1 || cc.resources[0] != "resource-a" {
		t.Errorf("resources: got %v, want [resource-a]", cc.resources)
	}
	// AES key derived from hmac_secret per ADR-0182 — pointer non-nil.
	if cc.aesKey.Load() == nil {
		t.Error("aesKey: got nil pointer, want SHA-256-derived key")
	}
	// httpClient is nil because FactoryCtx.HTTPClient was not threaded.
	if cc.httpClient != nil {
		t.Error("httpClient: got non-nil, want nil (test FactoryCtx)")
	}
	// tokenEndpointPoster wired by Task 11 — non-nil closure.
	if cc.tokenEndpointPoster == nil {
		t.Error("tokenEndpointPoster: got nil, want non-nil (Task 11 closure)")
	}
}

// TestParseConfig_HappyPath_CookieNames_OperatorOverride verifies operator-
// supplied CookieNames overrides are honored (empty fields fall back to
// upstream-canonical defaults).
func TestParseConfig_HappyPath_CookieNames_OperatorOverride(t *testing.T) {
	msg := validOAuth2Config(t)
	msg.Config.Credentials.CookieNames = &oauth2v3.OAuth2Credentials_CookieNames{
		BearerToken: "MyBearer",
		// IdToken left empty — falls back to "IdToken" default
		RefreshToken: "MyRefresh",
	}
	any := newOAuth2Any(t, msg)
	factory, err := New(any, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hf := factory()
	df := hf.Decoder.(*filter)
	if df.cc.cookieNames.BearerToken != "MyBearer" {
		t.Errorf("BearerToken: got %q, want %q", df.cc.cookieNames.BearerToken, "MyBearer")
	}
	if df.cc.cookieNames.IdToken != "IdToken" {
		t.Errorf("IdToken (fall-back): got %q, want %q", df.cc.cookieNames.IdToken, "IdToken")
	}
	if df.cc.cookieNames.RefreshToken != "MyRefresh" {
		t.Errorf("RefreshToken: got %q, want %q", df.cc.cookieNames.RefreshToken, "MyRefresh")
	}
}

// TestParseConfig_HappyPath_RedirectPathMatcher_Compiles verifies the
// redirect_path_matcher proto compiles into a pathMatcherFn closure.
func TestParseConfig_HappyPath_RedirectPathMatcher_Compiles(t *testing.T) {
	msg := validOAuth2Config(t)
	msg.Config.RedirectPathMatcher = &matcherv3.PathMatcher{
		Rule: &matcherv3.PathMatcher_Path{
			Path: &matcherv3.StringMatcher{
				MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "/oauth2/callback"},
			},
		},
	}
	any := newOAuth2Any(t, msg)
	factory, err := New(any, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hf := factory()
	df := hf.Decoder.(*filter)
	if df.cc.redirectPathMatcher == nil {
		t.Fatal("redirectPathMatcher: got nil, want compiled closure")
	}
	if !df.cc.redirectPathMatcher("/oauth2/callback?code=abc") {
		t.Error("matcher should match /oauth2/callback?code=abc (Prefix)")
	}
	if df.cc.redirectPathMatcher("/different/path") {
		t.Error("matcher should NOT match /different/path")
	}
}

// TestParseConfig_HappyPath_PassThroughMatcher_Compiles verifies the
// pass_through_matcher proto compiles into a headerMatcherFn closure.
func TestParseConfig_HappyPath_PassThroughMatcher_Compiles(t *testing.T) {
	msg := validOAuth2Config(t)
	msg.Config.PassThroughMatcher = []*routev3.HeaderMatcher{
		{
			Name: "X-Bypass-OAuth2",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{
				ExactMatch: "true",
			},
		},
	}
	any := newOAuth2Any(t, msg)
	factory, err := New(any, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hf := factory()
	df := hf.Decoder.(*filter)
	if df.cc.passThroughMatcher == nil {
		t.Fatal("passThroughMatcher: got nil, want compiled closure")
	}
	hv := headerViewAdapter(http.Header{"X-Bypass-Oauth2": []string{"true"}})
	if !df.cc.passThroughMatcher(hv) {
		t.Error("matcher should match X-Bypass-OAuth2=true")
	}
}

// TestParseConfig_HappyPath_StatsAllocated verifies the 6-counter filterStats
// surface is allocated when ctx.Stats is non-nil.
func TestParseConfig_HappyPath_StatsAllocated(t *testing.T) {
	msg := validOAuth2Config(t)
	any := newOAuth2Any(t, msg)
	reg := stats.NewRegistry()
	factory, err := New(any, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hf := factory()
	df := hf.Decoder.(*filter)
	if df.cc.stats == nil {
		t.Fatal("stats: got nil, want allocated *filterStats")
	}
	if df.cc.stats.oauthUnauthorizedRq == nil {
		t.Error("oauthUnauthorizedRq counter: got nil, want allocated")
	}
	if df.cc.stats.oauthRefreshtokenSuccess == nil {
		t.Error("oauthRefreshtokenSuccess counter: got nil, want allocated")
	}
}

// TestParseConfig_HappyPath_NilStats_GracefulNilFilterStats verifies the
// ADR-0085 nil-tolerance — a nil ctx.Stats produces a nil filterStats
// without panic.
func TestParseConfig_HappyPath_NilStats_GracefulNilFilterStats(t *testing.T) {
	msg := validOAuth2Config(t)
	any := newOAuth2Any(t, msg)
	factory, err := New(any, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hf := factory()
	df := hf.Decoder.(*filter)
	if df.cc.stats != nil {
		t.Error("stats: got non-nil, want nil (ADR-0085 nil-tolerance with nil Stats)")
	}
}
