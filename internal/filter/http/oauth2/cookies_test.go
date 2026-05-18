package oauth2

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// Group 4 — cookie envelope round-trip + per-category emission tests.
//
// Surface under test per phase-20 SPEC §6.4 + §6.5 + §4.1 + §4.5 + ADR-0181:
//
//   - parseAllCookies(headers, names) extracts the 5-cookie envelope from the
//     request Cookie: header(s). Operator-configurable cookie names per the
//     CookieNames carrier (defaults: BearerToken / OauthHMAC / OauthExpires /
//     IdToken / RefreshToken per ADR-0181 §Context).
//   - formatSetCookie(name, value, attrs) emits the Set-Cookie header value
//     byte-exact per §4.5 + §12 item A2 (Path=/ + Secure + HttpOnly +
//     SameSite=Lax; optional Max-Age + Domain).
//   - addFlowCookieDeletionHeaders(headers, names, attrs) emits Max-Age=0
//     Set-Cookie deletions for all flow cookies per AMEND-3 + §4.5 category (d).
//   - emitCategoryX helpers exercise the per-category emission table per §4.1.
//
// All vector outputs are pinned BYTE-EXACT to lock the wire-format discipline.
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// parseAllCookies tests
// -----------------------------------------------------------------------------

// TestParseAllCookies_FullEnvelope exercises the maximal 5-cookie case — all
// 5 cookies present in a single `Cookie:` header per RFC 6265 §5.4 semicolon-
// separated cookie-pairs. Verifies the canonical defaults extract all 5
// envelope fields without dropping any.
func TestParseAllCookies_FullEnvelope(t *testing.T) {
	h := http.Header{}
	h.Set("Cookie",
		"BearerToken=enc-access-tok; OauthHMAC=abc123; OauthExpires=1700000000; IdToken=id-tok; RefreshToken=enc-refresh-tok")
	env := parseAllCookies(h, DefaultCookieNames())
	if env.BearerToken != "enc-access-tok" {
		t.Errorf("BearerToken: got %q, want %q", env.BearerToken, "enc-access-tok")
	}
	if env.OauthHMAC != "abc123" {
		t.Errorf("OauthHMAC: got %q, want %q", env.OauthHMAC, "abc123")
	}
	if env.OauthExpires != "1700000000" {
		t.Errorf("OauthExpires: got %q, want %q", env.OauthExpires, "1700000000")
	}
	if env.IdToken != "id-tok" {
		t.Errorf("IdToken: got %q, want %q", env.IdToken, "id-tok")
	}
	if env.RefreshToken != "enc-refresh-tok" {
		t.Errorf("RefreshToken: got %q, want %q", env.RefreshToken, "enc-refresh-tok")
	}
}

// TestParseAllCookies_MissingIdToken exercises the MVP-typical case (id_token
// deferred per SPEC §2.2). The IdToken cookie is absent; all other 4 cookies
// extract; env.IdToken is the zero value ("").
func TestParseAllCookies_MissingIdToken(t *testing.T) {
	h := http.Header{}
	h.Set("Cookie",
		"BearerToken=enc-access-tok; OauthHMAC=abc123; OauthExpires=1700000000; RefreshToken=enc-refresh-tok")
	env := parseAllCookies(h, DefaultCookieNames())
	if env.IdToken != "" {
		t.Errorf("IdToken absent: got %q, want empty string", env.IdToken)
	}
	if env.BearerToken == "" || env.OauthHMAC == "" || env.OauthExpires == "" || env.RefreshToken == "" {
		t.Errorf("non-IdToken fields must populate: env=%+v", env)
	}
}

// TestParseAllCookies_MissingRefreshToken exercises the no-refresh case
// (use_refresh_token=false OR post-rotation transient). RefreshToken absent;
// env.RefreshToken is the zero value.
func TestParseAllCookies_MissingRefreshToken(t *testing.T) {
	h := http.Header{}
	h.Set("Cookie", "BearerToken=enc-access-tok; OauthHMAC=abc123; OauthExpires=1700000000")
	env := parseAllCookies(h, DefaultCookieNames())
	if env.RefreshToken != "" {
		t.Errorf("RefreshToken absent: got %q, want empty string", env.RefreshToken)
	}
	if env.BearerToken == "" || env.OauthHMAC == "" || env.OauthExpires == "" {
		t.Errorf("non-RefreshToken fields must populate: env=%+v", env)
	}
}

// TestParseAllCookies_MissingHMAC_ReturnsIncomplete exercises the partial-
// envelope case where the OauthHMAC cookie is absent. The parser does NOT
// reject — it returns a partial envelope; the downstream `hmacValidate`
// + dispatcher classify the partial envelope as unauthenticated → 302
// challenge per §6.3 step 4.
func TestParseAllCookies_MissingHMAC_ReturnsIncomplete(t *testing.T) {
	h := http.Header{}
	h.Set("Cookie", "BearerToken=enc-access-tok; OauthExpires=1700000000")
	env := parseAllCookies(h, DefaultCookieNames())
	if env.OauthHMAC != "" {
		t.Errorf("OauthHMAC absent: got %q, want empty", env.OauthHMAC)
	}
	if env.BearerToken == "" || env.OauthExpires == "" {
		t.Errorf("other fields must populate: env=%+v", env)
	}
}

// TestParseAllCookies_MultipleHeaders exercises the case where the request
// carries multiple `Cookie:` headers (each a comma-separated list of one or
// more cookie-pairs is NOT RFC-compliant; HTTP/2 RFC 9113 §8.2.3 explicitly
// permits MULTIPLE Cookie header fields concatenated by the receiver). The
// parser must iterate ALL Cookie header values, not just the first.
func TestParseAllCookies_MultipleHeaders(t *testing.T) {
	h := http.Header{}
	h.Add("Cookie", "BearerToken=enc-access-tok; OauthHMAC=abc123")
	h.Add("Cookie", "OauthExpires=1700000000; RefreshToken=enc-refresh-tok")
	env := parseAllCookies(h, DefaultCookieNames())
	if env.BearerToken != "enc-access-tok" || env.OauthHMAC != "abc123" ||
		env.OauthExpires != "1700000000" || env.RefreshToken != "enc-refresh-tok" {
		t.Errorf("multi-header parse failed: env=%+v", env)
	}
}

// TestParseAllCookies_DuplicateCookies_LastWins exercises the case where the
// same cookie name appears multiple times within the Cookie: header. Per
// net/http.Request.Cookies semantics, ALL values are returned in order; the
// parser collapses to the LAST occurrence (HTTP cookie-handling convention —
// server-side, when a duplicate name arrives, the LAST value is the most
// recently mutated). The behavior is asserted here so a future refactor
// surfaces the regression.
func TestParseAllCookies_DuplicateCookies_LastWins(t *testing.T) {
	h := http.Header{}
	h.Set("Cookie", "BearerToken=old; BearerToken=new; OauthHMAC=abc; OauthExpires=1700000000")
	env := parseAllCookies(h, DefaultCookieNames())
	if env.BearerToken != "new" {
		t.Errorf("duplicate BearerToken: got %q, want %q (last-wins)", env.BearerToken, "new")
	}
}

// TestParseAllCookies_CustomCookieNames_Honored exercises the operator-
// configurable cookie names per SPEC §6.4 (CookieNames proto field). When the
// operator overrides the default name set, the parser MUST use the configured
// names — the defaults MUST NOT bleed through.
func TestParseAllCookies_CustomCookieNames_Honored(t *testing.T) {
	custom := CookieNames{
		BearerToken:  "My_Token",
		OauthHMAC:    "My_HMAC",
		OauthExpires: "My_Exp",
		IdToken:      "My_Id",
		RefreshToken: "My_Refresh",
	}
	h := http.Header{}
	h.Set("Cookie", "My_Token=tok; My_HMAC=h; My_Exp=1700000000; My_Refresh=r; BearerToken=should-be-ignored")
	env := parseAllCookies(h, custom)
	if env.BearerToken != "tok" {
		t.Errorf("custom BearerToken name: got %q, want %q", env.BearerToken, "tok")
	}
	if env.OauthHMAC != "h" {
		t.Errorf("custom OauthHMAC name: got %q, want %q", env.OauthHMAC, "h")
	}
	if env.OauthExpires != "1700000000" {
		t.Errorf("custom OauthExpires name: got %q, want %q", env.OauthExpires, "1700000000")
	}
	if env.RefreshToken != "r" {
		t.Errorf("custom RefreshToken name: got %q, want %q", env.RefreshToken, "r")
	}
	// Default-name `BearerToken=should-be-ignored` MUST NOT leak — the parser
	// honored the operator override; the default name is non-load-bearing here.
	if env.BearerToken == "should-be-ignored" {
		t.Errorf("default cookie name leaked despite override: env.BearerToken=%q", env.BearerToken)
	}
}

// TestParseAllCookies_NoCookieHeader exercises the zero-value case: no Cookie
// header at all. All envelope fields are the zero value; no panic.
func TestParseAllCookies_NoCookieHeader(t *testing.T) {
	h := http.Header{}
	env := parseAllCookies(h, DefaultCookieNames())
	zero := CookieEnvelope{}
	if env != zero {
		t.Errorf("no Cookie header: got %+v, want zero-value envelope", env)
	}
}

// TestDefaultCookieNames_UpstreamCanonical verifies the byte-exact upstream
// names per ADR-0181 §Context. A regression here would break wire-compat with
// reference Envoy v1.37.2.
func TestDefaultCookieNames_UpstreamCanonical(t *testing.T) {
	d := DefaultCookieNames()
	cases := []struct {
		got, want string
		field     string
	}{
		{d.BearerToken, "BearerToken", "BearerToken"},
		{d.OauthHMAC, "OauthHMAC", "OauthHMAC"},
		{d.OauthExpires, "OauthExpires", "OauthExpires"},
		{d.IdToken, "IdToken", "IdToken"},
		{d.RefreshToken, "RefreshToken", "RefreshToken"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s default: got %q, want %q (upstream-canonical)", tc.field, tc.got, tc.want)
		}
	}
}

// -----------------------------------------------------------------------------
// formatSetCookie tests
// -----------------------------------------------------------------------------

// TestFormatSetCookie_DefaultAttributes exercises the MVP-default Set-Cookie
// attribute shape per SPEC §4.5 + §12 item A2: `name=value; Path=/; Secure;
// HttpOnly; SameSite=Lax`. Byte-exact attribute ordering pinned — a regression
// here would break the cross-side byte-comparison at IMPL Task 12 fixture-0024
// scenario (a).
func TestFormatSetCookie_DefaultAttributes(t *testing.T) {
	got := formatSetCookie("BearerToken", "enc-tok", DefaultSetCookieAttrs())
	want := "BearerToken=enc-tok; Path=/; Secure; HttpOnly; SameSite=Lax"
	if got != want {
		t.Errorf("Set-Cookie default attrs mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestFormatSetCookie_MaxAgeZero exercises the sign-out / cleanup clearing
// shape per §4.5 categories (c) + (d): `name=; ...; Max-Age=0`. The empty
// value + the Max-Age=0 attribute together direct the user-agent to delete
// the cookie immediately.
func TestFormatSetCookie_MaxAgeZero(t *testing.T) {
	attrs := DefaultSetCookieAttrs()
	zero := time.Duration(0)
	attrs.MaxAge = &zero
	got := formatSetCookie("BearerToken", "", attrs)
	want := "BearerToken=; Path=/; Secure; HttpOnly; SameSite=Lax; Max-Age=0"
	if got != want {
		t.Errorf("Set-Cookie Max-Age=0 mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestFormatSetCookie_MaxAgePositive exercises the post-callback (b) shape
// where the cookie's Max-Age caps the validity to the operator-configured
// `default_expires_in`. Output uses seconds-as-decimal-integer per RFC 6265
// §5.2.2.
func TestFormatSetCookie_MaxAgePositive(t *testing.T) {
	attrs := DefaultSetCookieAttrs()
	d := 3600 * time.Second
	attrs.MaxAge = &d
	got := formatSetCookie("BearerToken", "enc-tok", attrs)
	want := "BearerToken=enc-tok; Path=/; Secure; HttpOnly; SameSite=Lax; Max-Age=3600"
	if got != want {
		t.Errorf("Set-Cookie Max-Age=3600 mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestFormatSetCookie_DomainSet exercises the `Domain=` attribute when the
// operator configures `cookie_domain` (per AMEND-6 C1 + SPEC §20.P2). MVP
// default is host-only (Domain unset); this test exercises the non-default
// override path.
func TestFormatSetCookie_DomainSet(t *testing.T) {
	attrs := DefaultSetCookieAttrs()
	attrs.Domain = "example.com"
	got := formatSetCookie("BearerToken", "enc-tok", attrs)
	want := "BearerToken=enc-tok; Path=/; Secure; HttpOnly; SameSite=Lax; Domain=example.com"
	if got != want {
		t.Errorf("Set-Cookie Domain=example.com mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestFormatSetCookie_DomainEmpty_HostOnly verifies the MVP default per §20.P2
// RATIFIED + SPEC §8 item 12: when Domain is empty, NO `Domain=` attribute is
// emitted (host-only cookie). A regression here would silently broaden the
// cookie scope.
func TestFormatSetCookie_DomainEmpty_HostOnly(t *testing.T) {
	got := formatSetCookie("BearerToken", "enc-tok", DefaultSetCookieAttrs())
	if strings.Contains(got, "Domain=") {
		t.Errorf("Set-Cookie with empty Domain MUST be host-only (no Domain= attr); got %q", got)
	}
}

// -----------------------------------------------------------------------------
// Round-trip test
// -----------------------------------------------------------------------------

// TestRoundTrip_5CookieEnvelope exercises the emit → parse round-trip. Build
// 5 Set-Cookie header values via formatSetCookie; re-parse the (cookie-pair-
// only) portion via parseAllCookies; the resulting envelope MUST equal the
// original (cookie-value byte-exact). The Set-Cookie attribute suffix is
// stripped before re-parse because Set-Cookie is server→user-agent (response
// side) and Cookie is user-agent→server (request side); the cookie value
// shape itself is symmetric across both directions per RFC 6265 §4.1 + §4.2.
func TestRoundTrip_5CookieEnvelope(t *testing.T) {
	original := CookieEnvelope{
		BearerToken:  "enc-access-tok",
		OauthHMAC:    "abc123-hmac-value",
		OauthExpires: "1700000000",
		IdToken:      "id-tok",
		RefreshToken: "enc-refresh-tok",
	}
	// Emit Set-Cookie headers, then synthesize the user-agent's Cookie request
	// header by extracting the cookie-pair (name=value) portion of each
	// Set-Cookie value (everything before the first "; ").
	names := DefaultCookieNames()
	cookieValues := []string{
		cookiePairOf(formatSetCookie(names.BearerToken, original.BearerToken, DefaultSetCookieAttrs())),
		cookiePairOf(formatSetCookie(names.OauthHMAC, original.OauthHMAC, DefaultSetCookieAttrs())),
		cookiePairOf(formatSetCookie(names.OauthExpires, original.OauthExpires, DefaultSetCookieAttrs())),
		cookiePairOf(formatSetCookie(names.IdToken, original.IdToken, DefaultSetCookieAttrs())),
		cookiePairOf(formatSetCookie(names.RefreshToken, original.RefreshToken, DefaultSetCookieAttrs())),
	}
	h := http.Header{}
	h.Set("Cookie", strings.Join(cookieValues, "; "))
	got := parseAllCookies(h, names)
	if got != original {
		t.Errorf("round-trip mismatch:\n got: %+v\nwant: %+v", got, original)
	}
}

// cookiePairOf extracts the cookie-pair (name=value) portion of a Set-Cookie
// value (everything before the first "; "). Test-only helper.
func cookiePairOf(setCookieValue string) string {
	if i := strings.Index(setCookieValue, "; "); i >= 0 {
		return setCookieValue[:i]
	}
	return setCookieValue
}

// -----------------------------------------------------------------------------
// Per-category emission tests per SPEC §4.1 + §4.5
// -----------------------------------------------------------------------------

// TestPerCategory_Emission_Table_A_AuthChallenge exercises the (a) 302 auth-
// challenge emission per §4.5 category (a): cleared envelope cookies for the
// 4 wire-emitted fields (BearerToken / OauthHMAC / OauthExpires / RefreshToken)
// + state cookie SET to HMAC(state). IdToken is `(n/a)` per the §4.5 table.
//
// Test asserts:
//   - 4 Set-Cookie headers for the envelope-clearing (Max-Age=0 each)
//   - 1 Set-Cookie header SETting the state cookie (no Max-Age)
//   - 0 IdToken Set-Cookie header (n/a per §4.5)
func TestPerCategory_Emission_Table_A_AuthChallenge(t *testing.T) {
	h := http.Header{}
	emitCategoryA_AuthChallenge(h, DefaultCookieNames(), DefaultSetCookieAttrs(),
		"oauth_state_cookie", "hmac-of-state-value")

	setCookies := h.Values("Set-Cookie")
	// Expect 4 clearings + 1 state-set = 5 Set-Cookie headers.
	if len(setCookies) != 5 {
		t.Fatalf("category (a): got %d Set-Cookie headers; want 5\nactual: %v", len(setCookies), setCookies)
	}
	// 4 of the 5 are Max-Age=0 clearings; 1 is the state cookie SET (with the
	// hmac value as cookie value; no Max-Age=0).
	maxAgeZero := 0
	stateSet := 0
	for _, sc := range setCookies {
		if strings.Contains(sc, "Max-Age=0") {
			maxAgeZero++
		}
		if strings.HasPrefix(sc, "oauth_state_cookie=hmac-of-state-value;") {
			stateSet++
		}
	}
	if maxAgeZero != 4 {
		t.Errorf("category (a): got %d Max-Age=0 clearings; want 4 (envelope-clearing)", maxAgeZero)
	}
	if stateSet != 1 {
		t.Errorf("category (a): got %d state-cookie SETs; want 1", stateSet)
	}
}

// TestPerCategory_Emission_Table_B_PostCallback exercises the (b) 302 post-
// callback-success emission per §4.5 category (b): BearerToken=encrypted
// access_token + OauthHMAC + OauthExpires=epoch-seconds-decimal-string +
// (optional) IdToken + (optional) RefreshToken. The test exercises the
// 4-cookie shape (IdToken `(n/a)` per §4.5; RefreshToken SET).
func TestPerCategory_Emission_Table_B_PostCallback(t *testing.T) {
	h := http.Header{}
	expiresEpoch := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC).Unix()
	emitCategoryB_PostCallback(h, DefaultCookieNames(), DefaultSetCookieAttrs(),
		"enc-access-tok", "abc123-hmac", expiresEpoch, "enc-refresh-tok")

	setCookies := h.Values("Set-Cookie")
	// 4 SETs: BearerToken + OauthHMAC + OauthExpires + RefreshToken.
	if len(setCookies) != 4 {
		t.Fatalf("category (b): got %d Set-Cookie headers; want 4\nactual: %v", len(setCookies), setCookies)
	}
	// OauthExpires value MUST be the epoch-seconds-decimal-string per §12 A3.
	expectedExpires := "OauthExpires=" + strconv.FormatInt(expiresEpoch, 10) + ";"
	found := false
	for _, sc := range setCookies {
		if strings.HasPrefix(sc, expectedExpires) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("category (b): OauthExpires cookie missing or wrong format; want prefix %q; got:\n%v",
			expectedExpires, setCookies)
	}
}

// TestPerCategory_Emission_Table_B_PostCallback_NoRefreshToken exercises the
// `use_refresh_token=false` case where RefreshToken is `(n/a)` per the
// callback flow. Only 3 cookies emit (BearerToken + OauthHMAC + OauthExpires).
func TestPerCategory_Emission_Table_B_PostCallback_NoRefreshToken(t *testing.T) {
	h := http.Header{}
	expiresEpoch := int64(1747522800)
	emitCategoryB_PostCallback(h, DefaultCookieNames(), DefaultSetCookieAttrs(),
		"enc-access-tok", "abc123-hmac", expiresEpoch, "" /* no refresh token */)

	setCookies := h.Values("Set-Cookie")
	if len(setCookies) != 3 {
		t.Fatalf("category (b) no-refresh: got %d Set-Cookie headers; want 3\nactual: %v",
			len(setCookies), setCookies)
	}
	for _, sc := range setCookies {
		if strings.HasPrefix(sc, "RefreshToken=") {
			t.Errorf("category (b) no-refresh: unexpected RefreshToken Set-Cookie: %q", sc)
		}
	}
}

// TestPerCategory_Emission_Table_C_Signout exercises the (c) 302 sign-out
// emission per §4.5 category (c): Max-Age=0 for ALL 5 cookies (full envelope
// clearing).
func TestPerCategory_Emission_Table_C_Signout(t *testing.T) {
	h := http.Header{}
	emitCategoryC_Signout(h, DefaultCookieNames(), DefaultSetCookieAttrs())

	setCookies := h.Values("Set-Cookie")
	// 5 SETs: all envelope cookies cleared.
	if len(setCookies) != 5 {
		t.Fatalf("category (c): got %d Set-Cookie headers; want 5\nactual: %v", len(setCookies), setCookies)
	}
	for _, sc := range setCookies {
		if !strings.Contains(sc, "Max-Age=0") {
			t.Errorf("category (c): Set-Cookie missing Max-Age=0: %q", sc)
		}
	}
	// Verify each of the 5 envelope cookies is present by name prefix.
	wantPrefixes := []string{"BearerToken=", "OauthHMAC=", "OauthExpires=", "IdToken=", "RefreshToken="}
	for _, p := range wantPrefixes {
		found := false
		for _, sc := range setCookies {
			if strings.HasPrefix(sc, p) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("category (c): missing Set-Cookie for prefix %q; got: %v", p, setCookies)
		}
	}
}

// TestPerCategory_Emission_Table_D_401 exercises the (d) 401 emission per
// §4.5 category (d) + AMEND-3: cleared envelope via
// addFlowCookieDeletionHeaders. All 5 envelope cookies emit Max-Age=0; the
// 401-body emission is owned by callback.go SendLocalReply (out of scope for
// this Group 4 cookie-emission test).
func TestPerCategory_Emission_Table_D_401(t *testing.T) {
	h := http.Header{}
	addFlowCookieDeletionHeaders(h, DefaultCookieNames(), DefaultSetCookieAttrs())

	setCookies := h.Values("Set-Cookie")
	if len(setCookies) != 5 {
		t.Fatalf("category (d): got %d Set-Cookie headers; want 5\nactual: %v", len(setCookies), setCookies)
	}
	for _, sc := range setCookies {
		if !strings.Contains(sc, "Max-Age=0") {
			t.Errorf("category (d): Set-Cookie missing Max-Age=0: %q", sc)
		}
	}
}

// TestStateCookiePayloadShape_EpochSecondsDecimalString verifies the
// OauthExpires format per §12 item A3 + planner-time D16 A3: epoch-seconds-
// as-decimal-string (e.g. `"1747522800"`). NOT an RFC 3339 timestamp; NOT
// milliseconds. The format MUST round-trip through strconv.FormatInt /
// strconv.ParseInt.
func TestStateCookiePayloadShape_EpochSecondsDecimalString(t *testing.T) {
	// Pin a representative timestamp: 2025-05-17T19:00:00Z = 1747522800.
	epoch := int64(1747522800)
	got := formatExpiresValue(epoch)
	want := "1747522800"
	if got != want {
		t.Errorf("OauthExpires format: got %q, want %q (epoch-seconds-decimal-string per §12 A3)", got, want)
	}
	// Verify round-trip via strconv.
	parsed, err := strconv.ParseInt(got, 10, 64)
	if err != nil {
		t.Errorf("OauthExpires must round-trip via ParseInt; got err=%v", err)
	}
	if parsed != epoch {
		t.Errorf("OauthExpires round-trip: got %d, want %d", parsed, epoch)
	}
}
