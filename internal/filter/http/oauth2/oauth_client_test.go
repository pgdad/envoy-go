package oauth2

// oauth_client_test.go — Task 8 refresh-token rotation tests per phase-20
// SPEC §6.8 + §4.6 + §14.2 + ADR-0183 + planner-time D14.
//
// # Test groups
//
//   - handleRefresh flow tests (4 — TestHandleRefresh_* + TestApplyRefreshTokenResponse_*)
//     covering the success + 5xx-failure + 4xx-failure + OnDestroy-guard
//     legs per SPEC §4.6 + AMEND-3 + ADR-0183.
//
//   - TestRefreshTokenRotation_Concurrent_* race group (4 tests) per planner-
//     time D4 + D14 + SPEC §14.2. Validates the no-per-stream-serialization
//     discipline + the counter-one-per-event invariant + the OnDestroy-mid-
//     refresh no-panic guard under `-race`.
//
// # Task 10 dependency note (tokenEndpointPoster injection)
//
// The refresh POST itself requires `oauth_client.go::postTokenEndpoint` which
// lands at Task 10. Task 8 declares the `tokenEndpointPosterFn` function-shaped
// abstraction on compiledConfig + injects a test implementation in these
// tests; Task 10 wires the production implementation via *httpclient.Client.
//
// # Deferred Set-Cookie discipline per ADR-0183
//
// The success leg of the refresh-token rotation stores the new Set-Cookie
// envelope on `filter.pendingSetCookies` then calls `dcb.ContinueDecoding()`
// to resume the parked decode goroutine. The actual encode-side emission
// (writing the envelope to the response) is Task 11 (full filter integration).
// Task 8's tests assert the pendingSetCookies field is populated + the counter
// increments — the encode-side wiring is validated at Task 12 fixture-0024
// scenario (d) `refresh_token_rotation/`.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/httpclient"
	"github.com/pgdad/envoy-go/internal/stats"
)

// ---------------------------------------------------------------------------
// Test harness — fakeTokenPoster + helpers
// ---------------------------------------------------------------------------

// fakeTokenPoster returns a scripted *http.Response with the supplied status
// code. Used to drive the success / 5xx / 4xx legs of the refresh-token
// rotation path without depending on the Task 10 production implementation.
func fakeTokenPoster(status int, body string) tokenEndpointPosterFn {
	return func(_ context.Context, _, _, _, _ string) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
}

// fakeTokenPosterErr returns a poster that returns a transport-level error
// (non-nil err alongside a nil response — mirrors net/http.Client.Do error
// semantics on connection-reset / timeout / context-canceled).
func fakeTokenPosterErr(err error) tokenEndpointPosterFn {
	return func(_ context.Context, _, _, _, _ string) (*http.Response, error) {
		return nil, err
	}
}

// fakeRefreshDCB wraps fakeOAuth2DCB with a mutex for race-clean inspection.
// The resume goroutine fires SendLocalReply / ContinueDecoding from a
// non-main goroutine; the test goroutine inspects from the main goroutine —
// race-clean access requires the mutex.
type fakeRefreshDCB struct {
	mu sync.Mutex
	*fakeOAuth2DCB
}

func newFakeRefreshDCB() *fakeRefreshDCB {
	return &fakeRefreshDCB{fakeOAuth2DCB: newFakeOAuth2DCB()}
}

// ContinueDecoding overrides the embedded method to take the mutex.
func (c *fakeRefreshDCB) ContinueDecoding() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fakeOAuth2DCB.ContinueDecoding()
}

// SendLocalReply overrides the embedded method to take the mutex.
func (c *fakeRefreshDCB) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fakeOAuth2DCB.SendLocalReply(status, body, headers)
}

// snapshot returns the captured state for race-clean assertion access.
func (c *fakeRefreshDCB) snapshot() (continueCount, localReplyCount, lastStatus int, lastBody string, lastHeaders envoyhttp.OrderedHeaders) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.continueCount, c.localReplyCount, c.lastStatus, c.lastBody, c.lastHeaders
}

// newRefreshTestConfig constructs a compiledConfig wired for refresh-token
// rotation tests: stats + cookieNames + cookieAttrs + the supplied poster.
func newRefreshTestConfig(poster tokenEndpointPosterFn) *compiledConfig {
	cc := newTestCompiledConfig()
	cc.authorizationEndpoint = "https://idp.example.com/auth"
	cc.tokenEndpointPoster = poster
	cc.useRefreshToken = true
	cc.clientID = "test-client-id"
	cc.clientSecretFn = func() []byte { return []byte("test-client-secret") }
	reg := stats.NewRegistry()
	cc.stats = newFilterStats(reg, "")
	return cc
}

// validRefreshEnvelope returns a CookieEnvelope shaped for the refresh-token
// rotation path: encrypted BearerToken + HMAC + expired epoch + RefreshToken
// non-empty.
func validRefreshEnvelope() CookieEnvelope {
	return CookieEnvelope{
		BearerToken:  "expired-bearer-token",
		OauthHMAC:    "hmac-value",
		OauthExpires: "1",
		RefreshToken: "valid-refresh-token",
	}
}

// ---------------------------------------------------------------------------
// handleRefresh + applyRefreshTokenResponse tests
// ---------------------------------------------------------------------------

// TestHandleRefresh_SuccessfulPost_ContinuesDecodingWithDeferredSetCookie
// verifies the happy path: refresh POST 2xx → ContinueDecoding +
// pendingSetCookies populated with the new envelope per ADR-0183 + §4.6 +
// `oauth_refreshtoken_success++`.
func TestHandleRefresh_SuccessfulPost_ContinuesDecodingWithDeferredSetCookie(t *testing.T) {
	cc := newRefreshTestConfig(fakeTokenPoster(200, `{"access_token":"new-at","refresh_token":"new-rt","expires_in":3600}`))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	// Drive handleRefresh + wait for the async goroutine to complete.
	status := f.handleRefresh(validRefreshEnvelope())
	if status != envoyhttp.StopIteration {
		t.Fatalf("handleRefresh: got %v, want StopIteration (parks decode)", status)
	}
	f.waitRefreshDone()

	contCount, lrCount, _, _, _ := dcb.snapshot()
	if contCount != 1 {
		t.Errorf("ContinueDecoding count: got %d, want 1 (success-leg resumes the parked goroutine)", contCount)
	}
	if lrCount != 0 {
		t.Errorf("SendLocalReply count: got %d, want 0 (success-leg does NOT emit)", lrCount)
	}
	if got := cc.stats.oauthRefreshtokenSuccess.Load(); got != 1 {
		t.Errorf("oauth_refreshtoken_success: got %d, want 1", got)
	}
	if got := cc.stats.oauthRefreshtokenFailure.Load(); got != 0 {
		t.Errorf("oauth_refreshtoken_failure: got %d, want 0 (success-leg does NOT increment failure)", got)
	}
	if got := cc.stats.oauthFailure.Load(); got != 0 {
		t.Errorf("oauth_failure: got %d, want 0 (success-leg does NOT increment terminal-failure)", got)
	}

	// Deferred Set-Cookie envelope: 4-cookie envelope (BearerToken + OauthHMAC
	// + OauthExpires + RefreshToken) per ADR-0181 / SPEC §4.5 category (b).
	pending := f.snapshotPendingSetCookies()
	if got := countSetCookies(pending); got < 3 {
		t.Errorf("pendingSetCookies Set-Cookie count: got %d, want >= 3 (BearerToken + OauthHMAC + OauthExpires; RefreshToken optional per cookie envelope discipline)", got)
	}
}

// TestHandleRefresh_FailedPost_5xx_Emits302ChallengeWithRefreshtokenFailureCounter
// verifies a 5xx refresh POST → category (a) 302 challenge +
// `oauth_refreshtoken_failure++` (NOT also `oauth_failure` per AMEND-3 +
// §4.6).
func TestHandleRefresh_FailedPost_5xx_Emits302ChallengeWithRefreshtokenFailureCounter(t *testing.T) {
	cc := newRefreshTestConfig(fakeTokenPoster(500, "internal server error"))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handleRefresh(validRefreshEnvelope())
	if status != envoyhttp.StopIteration {
		t.Fatalf("handleRefresh: got %v, want StopIteration", status)
	}
	f.waitRefreshDone()

	contCount, lrCount, lastStatus, _, _ := dcb.snapshot()
	if lrCount != 1 {
		t.Fatalf("SendLocalReply count: got %d, want 1 (5xx → category (a) 302 challenge)", lrCount)
	}
	if lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (category (a) auth-challenge per §4.7)", lastStatus)
	}
	if contCount != 1 {
		t.Errorf("ContinueDecoding count: got %d, want 1 (SendLocalReply pattern requires Continue to unblock the parked goroutine per extauthz precedent)", contCount)
	}
	if got := cc.stats.oauthRefreshtokenFailure.Load(); got != 1 {
		t.Errorf("oauth_refreshtoken_failure: got %d, want 1", got)
	}
	if got := cc.stats.oauthFailure.Load(); got != 0 {
		t.Errorf("oauth_failure: got %d, want 0 (refresh-failure MUST NOT also bump oauth_failure per AMEND-3 + §4.6)", got)
	}
}

// TestHandleRefresh_4xxFailure_Emits302ChallengeWithRefreshtokenFailureCounter
// verifies a 4xx refresh POST → category (a) 302 + `oauth_refreshtoken_failure++`.
// Per ADR-0183 + §4.6 the refresh leg's failure handling does NOT distinguish
// 4xx from 5xx (both classify as refresh-failure and route to handleRefreshFailure).
func TestHandleRefresh_4xxFailure_Emits302ChallengeWithRefreshtokenFailureCounter(t *testing.T) {
	cc := newRefreshTestConfig(fakeTokenPoster(401, "invalid_grant"))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handleRefresh(validRefreshEnvelope())
	if status != envoyhttp.StopIteration {
		t.Fatalf("handleRefresh: got %v, want StopIteration", status)
	}
	f.waitRefreshDone()

	_, lrCount, lastStatus, _, _ := dcb.snapshot()
	if lrCount != 1 {
		t.Fatalf("SendLocalReply count: got %d, want 1", lrCount)
	}
	if lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (category (a) per §4.7)", lastStatus)
	}
	if got := cc.stats.oauthRefreshtokenFailure.Load(); got != 1 {
		t.Errorf("oauth_refreshtoken_failure: got %d, want 1", got)
	}
	if got := cc.stats.oauthFailure.Load(); got != 0 {
		t.Errorf("oauth_failure: got %d, want 0 (refresh-failure MUST NOT also bump oauth_failure per AMEND-3 + §4.6)", got)
	}
}

// TestHandleRefresh_TransportError_TreatedAsFailure verifies a transport-level
// error (non-nil err from the poster) routes to handleRefreshFailure per
// SPEC §4.7 + AMEND-3 (treated as a non-2xx for counter / wire-shape purposes).
func TestHandleRefresh_TransportError_TreatedAsFailure(t *testing.T) {
	cc := newRefreshTestConfig(fakeTokenPosterErr(context.DeadlineExceeded))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	status := f.handleRefresh(validRefreshEnvelope())
	if status != envoyhttp.StopIteration {
		t.Fatalf("handleRefresh: got %v, want StopIteration", status)
	}
	f.waitRefreshDone()

	_, lrCount, lastStatus, _, _ := dcb.snapshot()
	if lrCount != 1 {
		t.Fatalf("SendLocalReply count: got %d, want 1", lrCount)
	}
	if lastStatus != 302 {
		t.Errorf("lastStatus: got %d, want 302 (transport-error routes to category (a))", lastStatus)
	}
	if got := cc.stats.oauthRefreshtokenFailure.Load(); got != 1 {
		t.Errorf("oauth_refreshtoken_failure: got %d, want 1", got)
	}
}

// TestApplyRefreshTokenResponse_OnDestroyGuard verifies that
// applyRefreshTokenResponse respects the done-guard per ADR-0159 D4 +
// extauthz precedent. After OnDestroy fires, applyRefreshTokenResponse must
// NOT touch the dcb (no SendLocalReply, no ContinueDecoding, no counter
// increment) + must NOT panic.
func TestApplyRefreshTokenResponse_OnDestroyGuard(t *testing.T) {
	cc := newRefreshTestConfig(fakeTokenPoster(200, `{"access_token":"new-at"}`))
	dcb := newFakeRefreshDCB()
	f := &filter{cc: cc, dcb: dcb}

	f.OnDestroy() // sets done=true

	// Direct-invoke applyRefreshTokenResponse with a synthesized 2xx response
	// — the done-guard MUST short-circuit before touching dcb or counters.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("applyRefreshTokenResponse panicked under OnDestroy: %v", r)
		}
	}()
	f.applyRefreshTokenResponse(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil)

	contCount, lrCount, _, _, _ := dcb.snapshot()
	if contCount != 0 {
		t.Errorf("ContinueDecoding count: got %d, want 0 (done-guard must short-circuit)", contCount)
	}
	if lrCount != 0 {
		t.Errorf("SendLocalReply count: got %d, want 0 (done-guard must short-circuit)", lrCount)
	}
	if got := cc.stats.oauthRefreshtokenSuccess.Load(); got != 0 {
		t.Errorf("oauth_refreshtoken_success: got %d, want 0 (done-guard must short-circuit before counter bump)", got)
	}
}

// ---------------------------------------------------------------------------
// TestRefreshTokenRotation_Concurrent_* race group per D4 + D14 + ADR-0183
// ---------------------------------------------------------------------------

// TestRefreshTokenRotation_Concurrent_2RequestsSameCookies_BothPost verifies
// the no-per-stream-serialization discipline per planner-time D14 + ADR-0183:
// 2 concurrent in-flight requests with the same expired BearerToken + valid
// RefreshToken each POST refresh independently → both succeed → both record
// their pending Set-Cookie envelope. The "latest Set-Cookie wins" semantic
// is browser-side (later Set-Cookie absorbs earlier) — at the filter level,
// each request emits its own envelope without coordination.
func TestRefreshTokenRotation_Concurrent_2RequestsSameCookies_BothPost(t *testing.T) {
	var postCount int32
	poster := func(_ context.Context, _, _, _, _ string) (*http.Response, error) {
		atomic.AddInt32(&postCount, 1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	cc := newRefreshTestConfig(poster)

	const N = 2
	filters := make([]*filter, N)
	dcbs := make([]*fakeRefreshDCB, N)
	for i := 0; i < N; i++ {
		dcbs[i] = newFakeRefreshDCB()
		filters[i] = &filter{cc: cc, dcb: dcbs[i]}
	}

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = filters[idx].handleRefresh(validRefreshEnvelope())
		}(i)
	}
	wg.Wait()
	for i := 0; i < N; i++ {
		filters[i].waitRefreshDone()
	}

	// Per D14: each in-flight request POSTs independently — no dedup.
	if got := atomic.LoadInt32(&postCount); got != N {
		t.Errorf("postCount: got %d, want %d (no per-stream serialization per D14 — each in-flight request POSTs independently)", got, N)
	}
	for i := 0; i < N; i++ {
		cont, lr, _, _, _ := dcbs[i].snapshot()
		if cont != 1 {
			t.Errorf("filter[%d] ContinueDecoding: got %d, want 1", i, cont)
		}
		if lr != 0 {
			t.Errorf("filter[%d] SendLocalReply: got %d, want 0 (success-leg does not emit)", i, lr)
		}
	}
	if got := cc.stats.oauthRefreshtokenSuccess.Load(); got != uint64(N) {
		t.Errorf("oauth_refreshtoken_success: got %d, want %d (one-per-event per ADR-0183)", got, N)
	}
}

// TestRefreshTokenRotation_Concurrent_CounterIncrementOnePerEvent verifies
// N concurrent rotations → `oauth_refreshtoken_success` counter == N (one-
// per-event invariant per ADR-0183 + §4.6).
func TestRefreshTokenRotation_Concurrent_CounterIncrementOnePerEvent(t *testing.T) {
	cc := newRefreshTestConfig(fakeTokenPoster(200, ""))

	const N = 20
	filters := make([]*filter, N)
	for i := 0; i < N; i++ {
		filters[i] = &filter{cc: cc, dcb: newFakeRefreshDCB()}
	}

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = filters[idx].handleRefresh(validRefreshEnvelope())
		}(i)
	}
	wg.Wait()
	for i := 0; i < N; i++ {
		filters[i].waitRefreshDone()
	}

	if got := cc.stats.oauthRefreshtokenSuccess.Load(); got != uint64(N) {
		t.Errorf("oauth_refreshtoken_success: got %d, want %d (one-per-event per ADR-0183)", got, N)
	}
	if got := cc.stats.oauthRefreshtokenFailure.Load(); got != 0 {
		t.Errorf("oauth_refreshtoken_failure: got %d, want 0 (all rotations succeed)", got)
	}
}

// TestRefreshTokenRotation_Concurrent_MixedSuccessAndFailure verifies a mix
// of 2xx + 5xx concurrent rotations increment the respective counters
// correctly per ADR-0183 + §4.6.
func TestRefreshTokenRotation_Concurrent_MixedSuccessAndFailure(t *testing.T) {
	// Half-success / half-failure poster — alternates based on the goroutine
	// call ordinal (atomic counter).
	var ordinal int32
	poster := func(_ context.Context, _, _, _, _ string) (*http.Response, error) {
		n := atomic.AddInt32(&ordinal, 1)
		status := 200
		if n%2 == 0 {
			status = 500
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	cc := newRefreshTestConfig(poster)

	const N = 20 // 10 successes + 10 failures
	filters := make([]*filter, N)
	for i := 0; i < N; i++ {
		filters[i] = &filter{cc: cc, dcb: newFakeRefreshDCB()}
	}

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = filters[idx].handleRefresh(validRefreshEnvelope())
		}(i)
	}
	wg.Wait()
	for i := 0; i < N; i++ {
		filters[i].waitRefreshDone()
	}

	succ := cc.stats.oauthRefreshtokenSuccess.Load()
	fail := cc.stats.oauthRefreshtokenFailure.Load()
	if succ+fail != uint64(N) {
		t.Errorf("succ+fail: got %d, want %d (total counter increments = total rotations)", succ+fail, N)
	}
	if succ != uint64(N/2) {
		t.Errorf("oauth_refreshtoken_success: got %d, want %d (half-success poster)", succ, N/2)
	}
	if fail != uint64(N/2) {
		t.Errorf("oauth_refreshtoken_failure: got %d, want %d (half-failure poster)", fail, N/2)
	}
	if got := cc.stats.oauthFailure.Load(); got != 0 {
		t.Errorf("oauth_failure: got %d, want 0 (refresh-failure MUST NOT also bump oauth_failure per AMEND-3 + §4.6)", got)
	}
}

// TestRefreshTokenRotation_Concurrent_OnDestroy_Mid_Refresh_NoPanic verifies
// that OnDestroy firing concurrently with an in-flight refresh POST does NOT
// panic + the resume goroutine's done-guard short-circuits before touching
// the destroyed stream. Mirrors the extauthz async-resume + OnDestroy
// invariant per ADR-0159 D4.
func TestRefreshTokenRotation_Concurrent_OnDestroy_Mid_Refresh_NoPanic(t *testing.T) {
	// Slow poster — blocks on the context cancellation. The expected sequence:
	// (1) handleRefresh fires the goroutine + arms f.callCancel.
	// (2) OnDestroy fires concurrently → sets done=true + invokes
	//     f.callCancel which cancels the poster's context.
	// (3) Poster returns ctx.Err() → applyRefreshTokenResponse runs under
	//     f.mu, observes done=true, and short-circuits without touching dcb.
	poster := func(ctx context.Context, _, _, _, _ string) (*http.Response, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	cc := newRefreshTestConfig(poster)

	const N = 20
	filters := make([]*filter, N)
	for i := 0; i < N; i++ {
		filters[i] = &filter{cc: cc, dcb: newFakeRefreshDCB()}
	}

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = filters[idx].handleRefresh(validRefreshEnvelope())
		}(i)
	}

	// Fire OnDestroy on all filters from a concurrent goroutine — racing with
	// the in-flight refresh POSTs. The done-guard MUST short-circuit before
	// any dcb touch.
	for i := 0; i < N; i++ {
		go filters[i].OnDestroy()
	}

	wg.Wait()
	for i := 0; i < N; i++ {
		filters[i].waitRefreshDone()
	}

	// No panic + all filters' done flag is set (the OnDestroy guard fired).
	for i := 0; i < N; i++ {
		filters[i].mu.Lock()
		done := filters[i].done
		filters[i].mu.Unlock()
		if !done {
			t.Errorf("filter[%d].done: got false, want true (OnDestroy must set done)", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Group 5 — buildTokenRequestBody template byte-exact tests per ADR-0185 +
// AMEND-5 + §20.P10 RATIFIED + SPEC §14.1 group 5
// ---------------------------------------------------------------------------

// TestBuildTokenRequestBody_AuthCode_4FieldByteExact asserts the 4-field
// authorization_code template emits the byte-exact wire shape per AMEND-5 +
// §20.P10 + upstream Envoy v1.37.2 source/extensions/filters/http/oauth2/
// oauth_client.cc. The MVP template is `grant_type=authorization_code&code=
// {0}&client_id={1}&client_secret={2}&redirect_uri={3}` with each {N} value
// urlEncode-percent-encoded per §6.7 + ADR-0185.
//
// PKCE-gated 5th field (`&code_verifier={4}`) is ABSENT at MVP per §2.1 + S3.
func TestBuildTokenRequestBody_AuthCode_4FieldByteExact(t *testing.T) {
	params := map[string]string{
		"code":          "abc123",
		"client_id":     "my-client",
		"client_secret": "s3cr3t",
		"redirect_uri":  "https://example.com/callback",
	}
	got := string(buildTokenRequestBody(grantTypeAuthorizationCode, params))
	// Each value is urlEncode-encoded. `:/` in redirect_uri → `%3A%2F`.
	want := "grant_type=authorization_code" +
		"&code=abc123" +
		"&client_id=my-client" +
		"&client_secret=s3cr3t" +
		"&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback"
	if got != want {
		t.Errorf("buildTokenRequestBody(authorization_code):\n got: %q\nwant: %q", got, want)
	}
}

// TestBuildTokenRequestBody_RefreshToken_3FieldByteExact asserts the 3-field
// refresh_token template emits the byte-exact wire shape per AMEND-5 +
// §20.P10 + upstream oauth_client.cc.
func TestBuildTokenRequestBody_RefreshToken_3FieldByteExact(t *testing.T) {
	params := map[string]string{
		"refresh_token": "rt-xyz-789",
		"client_id":     "my-client",
		"client_secret": "s3cr3t",
	}
	got := string(buildTokenRequestBody(grantTypeRefreshToken, params))
	want := "grant_type=refresh_token" +
		"&refresh_token=rt-xyz-789" +
		"&client_id=my-client" +
		"&client_secret=s3cr3t"
	if got != want {
		t.Errorf("buildTokenRequestBody(refresh_token):\n got: %q\nwant: %q", got, want)
	}
}

// TestBuildTokenRequestBody_AuthCode_PKCEAbsent_NoCodeVerifier asserts the
// 4-field auth-code template does NOT carry a `code_verifier` field at MVP
// per §2.1 + S3 (PKCE-gated 5th field deferred).
func TestBuildTokenRequestBody_AuthCode_PKCEAbsent_NoCodeVerifier(t *testing.T) {
	params := map[string]string{
		"code":          "abc",
		"client_id":     "x",
		"client_secret": "y",
		"redirect_uri":  "https://z",
		// A stray "code_verifier" key in params must NOT bleed into the body
		// — the template emits only the 4 named fields per AMEND-5.
		"code_verifier": "should-not-appear",
	}
	got := string(buildTokenRequestBody(grantTypeAuthorizationCode, params))
	if strings.Contains(got, "code_verifier") {
		t.Errorf("buildTokenRequestBody(authorization_code) leaked code_verifier into body: %q (PKCE 5th field is gated per §2.1 + S3)", got)
	}
}

// TestBuildTokenRequestBody_AuthCode_ValuesPercentEncoded asserts that special
// characters in field values are percent-encoded per §20.P10 charset.
func TestBuildTokenRequestBody_AuthCode_ValuesPercentEncoded(t *testing.T) {
	params := map[string]string{
		"code":          "a b&c?",
		"client_id":     "id:1",
		"client_secret": "p=q",
		"redirect_uri":  "https://example.com/cb?x=1",
	}
	got := string(buildTokenRequestBody(grantTypeAuthorizationCode, params))
	// `:` → `%3A`, `/` → `%2F`, `&` → `%26`, `?` → `%3F`, `=` → `%3D`,
	// ` ` → `%20`.
	want := "grant_type=authorization_code" +
		"&code=a%20b%26c%3F" +
		"&client_id=id%3A1" +
		"&client_secret=p%3Dq" +
		"&redirect_uri=https%3A%2F%2Fexample.com%2Fcb%3Fx%3D1"
	if got != want {
		t.Errorf("buildTokenRequestBody value-encoding:\n got: %q\nwant: %q", got, want)
	}
}

// TestBuildTokenRequestBody_RefreshToken_MissingFieldsEmitsEmpty asserts that
// when a field is absent from params, the template emits the field name with
// an empty value (matches upstream behavior — fields are always present in
// the template; missing values become empty-string field values).
func TestBuildTokenRequestBody_RefreshToken_MissingFieldsEmitsEmpty(t *testing.T) {
	params := map[string]string{
		"refresh_token": "rt",
		// client_id + client_secret intentionally absent.
	}
	got := string(buildTokenRequestBody(grantTypeRefreshToken, params))
	want := "grant_type=refresh_token&refresh_token=rt&client_id=&client_secret="
	if got != want {
		t.Errorf("buildTokenRequestBody refresh-token-missing-fields:\n got: %q\nwant: %q", got, want)
	}
}

// TestBuildTokenRequestBody_UnknownGrantType_EmptyBody asserts that an unknown
// grant_type returns an empty body (defensive — the caller is responsible
// for passing a recognized grant type; the helper does not panic).
func TestBuildTokenRequestBody_UnknownGrantType_EmptyBody(t *testing.T) {
	got := buildTokenRequestBody("unknown_grant", map[string]string{"foo": "bar"})
	if len(got) != 0 {
		t.Errorf("buildTokenRequestBody(unknown_grant): got %q, want empty body", got)
	}
}

// ---------------------------------------------------------------------------
// urlEncode vector tests per §12 item A5 RATIFIED-AT-IMPL via these tests +
// fixture-0024 scenario (a) token_endpoint POST body capture (Task 12)
// ---------------------------------------------------------------------------

// TestUrlEncode_PercentEncodes_ColonSlashEqualsAmpersandQuestion asserts the
// 5 named reserved characters from §20.P10 RATIFIED + AMEND-5 are percent-
// encoded by urlEncode (in contrast to stdlib `url.PathEscape` which leaves
// `:` and `/` un-encoded).
func TestUrlEncode_PercentEncodes_ColonSlashEqualsAmpersandQuestion(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{":", "%3A"},
		{"/", "%2F"},
		{"=", "%3D"},
		{"&", "%26"},
		{"?", "%3F"},
		{":/=&?", "%3A%2F%3D%26%3F"},
	}
	for _, c := range cases {
		if got := urlEncode(c.in); got != c.want {
			t.Errorf("urlEncode(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}

// TestUrlEncode_SpacesAsPercent20 asserts spaces are encoded as `%20` (NOT
// `+`) per AMEND-5 + RFC 3986 percent-encoding discipline. `+` would be the
// `application/x-www-form-urlencoded` legacy form-encoding choice, which
// Envoy upstream does NOT use for the token_endpoint POST body.
func TestUrlEncode_SpacesAsPercent20(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{" ", "%20"},
		{"a b", "a%20b"},
		{"   ", "%20%20%20"},
	}
	for _, c := range cases {
		if got := urlEncode(c.in); got != c.want {
			t.Errorf("urlEncode(%q): got %q, want %q (spaces must encode as %%20, not +)", c.in, got, c.want)
		}
	}
}

// TestUrlEncode_StdlibPathEscapeDivergence asserts urlEncode emits a byte-
// exact-different output from `url.PathEscape` for an input that includes
// the `:/=&?` charset. This pins the wire-divergence rationale anchored in
// ADR-0185 §Decision (the custom helper exists BECAUSE stdlib differs).
func TestUrlEncode_StdlibPathEscapeDivergence(t *testing.T) {
	in := "https://example.com/cb?x=1&y=2"
	custom := urlEncode(in)
	stdlib := url.PathEscape(in)
	if custom == stdlib {
		t.Errorf("urlEncode(%q) == url.PathEscape(%q) == %q — expected byte-exact divergence (the custom helper exists per ADR-0185 because stdlib PathEscape leaves :/=&? un-encoded)", in, in, custom)
	}
	// Sanity-check the custom output is the expected fully-encoded form.
	want := "https%3A%2F%2Fexample.com%2Fcb%3Fx%3D1%26y%3D2"
	if custom != want {
		t.Errorf("urlEncode(%q): got %q, want %q", in, custom, want)
	}
}

// TestUrlEncode_NonAsciiBytes_UTF8Escaped asserts non-ASCII inputs are
// percent-encoded per their UTF-8 byte sequence per §12 item A5 + D16 A5.
// "ä" is U+00E4, UTF-8 bytes 0xC3 0xA4 → `%C3%A4`.
// "€" is U+20AC, UTF-8 bytes 0xE2 0x82 0xAC → `%E2%82%AC`.
func TestUrlEncode_NonAsciiBytes_UTF8Escaped(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"ä", "%C3%A4"},
		{"€", "%E2%82%AC"},
		{"ä€", "%C3%A4%E2%82%AC"},
		// Mixed ASCII + non-ASCII.
		{"aäb", "a%C3%A4b"},
	}
	for _, c := range cases {
		if got := urlEncode(c.in); got != c.want {
			t.Errorf("urlEncode(%q): got %q, want %q (non-ASCII bytes encoded per UTF-8 byte sequence per §12 item A5 + D16 A5)", c.in, got, c.want)
		}
	}
}

// TestUrlEncode_UnreservedCharsPassThrough asserts the unreserved set per
// RFC 3986 §2.3 (`A-Z a-z 0-9 - . _ ~`) passes through verbatim — these are
// the only characters the custom encoder leaves alone.
func TestUrlEncode_UnreservedCharsPassThrough(t *testing.T) {
	in := "ABCabc012-._~"
	got := urlEncode(in)
	if got != in {
		t.Errorf("urlEncode(%q): got %q, want %q (RFC 3986 unreserved set passes through verbatim)", in, got, in)
	}
}

// TestUrlEncode_EmptyInput asserts urlEncode of an empty string returns an
// empty string (no panic, no allocation surprises).
func TestUrlEncode_EmptyInput(t *testing.T) {
	if got := urlEncode(""); got != "" {
		t.Errorf("urlEncode(\"\"): got %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// postTokenEndpoint tests per §6.7 + ADR-0185
// ---------------------------------------------------------------------------

// TestPostTokenEndpoint_SuccessfulPost_2xx asserts the happy path:
// postTokenEndpoint builds a POST request with the supplied body + Content-
// Type `application/x-www-form-urlencoded` + invokes httpclient.Client.Do +
// returns the response unchanged.
func TestPostTokenEndpoint_SuccessfulPost_2xx(t *testing.T) {
	var gotMethod, gotContentType, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		buf, _ := io.ReadAll(r.Body)
		gotBody = string(buf)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"at","expires_in":3600}`))
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{})
	body := []byte("grant_type=refresh_token&refresh_token=rt&client_id=c&client_secret=s")
	resp, err := postTokenEndpoint(context.Background(), client, srv.URL, body)
	if err != nil {
		t.Fatalf("postTokenEndpoint: unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("postTokenEndpoint: got status %d, want 200", resp.StatusCode)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("request method: got %q, want POST", gotMethod)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type: got %q, want application/x-www-form-urlencoded", gotContentType)
	}
	if gotBody != string(body) {
		t.Errorf("body: got %q, want %q", gotBody, string(body))
	}
}

// TestPostTokenEndpoint_HttpClientNil_ReturnsError asserts a nil
// *httpclient.Client argument surfaces an error rather than panicking.
// Defense-in-depth — the production wiring at Task 11 always supplies a
// non-nil client, but a misconfigured boot path should not crash the
// goroutine.
func TestPostTokenEndpoint_HttpClientNil_ReturnsError(t *testing.T) {
	resp, err := postTokenEndpoint(context.Background(), nil, "https://example.com/token", []byte("x=1"))
	if err == nil {
		t.Fatalf("postTokenEndpoint(nil client): expected error, got resp=%v", resp)
	}
	if resp != nil {
		t.Errorf("postTokenEndpoint(nil client): expected nil response, got %v", resp)
	}
}

// TestPostTokenEndpoint_ContextCanceled_PropagatesError asserts that a
// canceled context surfaces the cancel-error per the stdlib net/http
// contract (the OnDestroy guard relies on this — the resume goroutine sees
// ctx.Err() and routes to handleRefreshFailure per ADR-0159 D4).
func TestPostTokenEndpoint_ContextCanceled_PropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel — Do should return ctx.Err()
	resp, err := postTokenEndpoint(ctx, client, srv.URL, []byte("x=1"))
	if err == nil {
		t.Fatalf("postTokenEndpoint(canceled ctx): expected error, got resp=%v", resp)
	}
}

// TestPostTokenEndpoint_ContentTypeIsFormUrlEncoded asserts the Content-Type
// request header is exactly `application/x-www-form-urlencoded` per RFC 6749
// §4.1.3 + §6 + upstream Envoy oauth_client.cc.
func TestPostTokenEndpoint_ContentTypeIsFormUrlEncoded(t *testing.T) {
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{})
	resp, err := postTokenEndpoint(context.Background(), client, srv.URL, []byte(""))
	if err != nil {
		t.Fatalf("postTokenEndpoint: unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type: got %q, want application/x-www-form-urlencoded (RFC 6749 §4.1.3 + §6)", gotContentType)
	}
}

// ---------------------------------------------------------------------------
// Item C — extractCallbackParams URL-decode (paired inbound counterpart to
// outbound urlEncode per ADR-0185 §Decision)
// ---------------------------------------------------------------------------

// TestExtractCallbackParams_URLDecoded asserts that the state + code params
// extracted from a callback URL query string are URL-decoded per RFC 3986 +
// the AMEND-5 outbound urlEncode counterpart discipline. A `%20` in the
// query string surfaces as a literal space in the returned value.
func TestExtractCallbackParams_URLDecoded(t *testing.T) {
	code, state := extractCallbackParams("/cb?state=foo%20bar&code=hello%20world")
	if code != "hello world" {
		t.Errorf("code: got %q, want %q (URL-decode per Item C)", code, "hello world")
	}
	if state != "foo bar" {
		t.Errorf("state: got %q, want %q (URL-decode per Item C)", state, "foo bar")
	}
}

// TestExtractCallbackParams_URLDecoded_ReservedChars asserts that reserved
// characters (`:`, `/`, `=`, `&`, `?`) percent-encoded in the query string
// are URL-decoded back to their literal bytes — symmetric to the outbound
// urlEncode discipline per ADR-0185 §Decision.
func TestExtractCallbackParams_URLDecoded_ReservedChars(t *testing.T) {
	// state value contains a literal `:` that the auth-server percent-encoded
	// as `%3A` on the redirect-back.
	code, state := extractCallbackParams("/cb?code=abc%3D&state=tenant%3Acustomer")
	if code != "abc=" {
		t.Errorf("code: got %q, want %q", code, "abc=")
	}
	if state != "tenant:customer" {
		t.Errorf("state: got %q, want %q", state, "tenant:customer")
	}
}

// TestExtractCallbackParams_MalformedURLEncoding_GracefulFailure asserts that
// invalid percent-encoding in the query string does NOT panic. The function
// returns empty strings for unparseable values per the must-never-panic
// discipline + the AMEND-3 deny-path classification (malformed callback →
// downstream bad-state path).
func TestExtractCallbackParams_MalformedURLEncoding_GracefulFailure(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("extractCallbackParams panicked on malformed input: %v", r)
		}
	}()
	// `%ZZ` is invalid percent-encoding.
	code, state := extractCallbackParams("/cb?code=abc%ZZ&state=xyz")
	// Malformed `code` → empty; valid `state` → returned decoded.
	if code != "" {
		t.Errorf("code (malformed): got %q, want %q (graceful failure)", code, "")
	}
	if state != "xyz" {
		t.Errorf("state (valid): got %q, want %q (sibling parse continues)", state, "xyz")
	}
}

// TestExtractCallbackParams_NoQuery_ReturnsEmpty asserts that a path without
// a query string returns empty params (no panic, no allocation surprises).
func TestExtractCallbackParams_NoQuery_ReturnsEmpty(t *testing.T) {
	code, state := extractCallbackParams("/cb")
	if code != "" || state != "" {
		t.Errorf("extractCallbackParams(no-query): got (%q, %q), want (\"\", \"\")", code, state)
	}
}
