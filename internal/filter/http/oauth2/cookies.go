package oauth2

// cookies.go — 5-cookie envelope parsing + Set-Cookie formatting + per-category
// emission discipline per phase-20 ADR-0181 + SPEC §4.5 + §6.4 + §6.5.
//
// # Surface
//
// This file exposes the per-stream cookie-envelope read + write surface for
// the oauth2 filter:
//
//   - CookieEnvelope: the 5-of-7 MVP-consumed cookie payload (BearerToken /
//     OauthHMAC / OauthExpires / IdToken / RefreshToken) per ADR-0181 §Context.
//     The remaining 2-of-7 names (`oauth_nonce` + `code_verifier`) are
//     PKCE-gated deferrals per SPEC §8 item 5 and are not part of the MVP
//     envelope.
//
//   - CookieNames: the operator-configurable cookie-name overrides per SPEC
//     §6.4. Operators MAY rename any of the 5 envelope cookies via the
//     `cookie_names` proto field; the parser + writer honor the configured
//     names. DefaultCookieNames() returns the upstream-canonical defaults per
//     ADR-0181 (BearerToken / OauthHMAC / OauthExpires / IdToken /
//     RefreshToken — byte-exact upstream).
//
//   - SetCookieAttrs: the per-Set-Cookie attribute carrier per SPEC §4.5 +
//     §12 item A2. MVP defaults (DefaultSetCookieAttrs()): Path=/ + Secure +
//     HttpOnly + SameSite=Lax + Domain="" (host-only per §20.P2 RATIFIED +
//     SPEC §8 item 12) + MaxAge=nil (no Max-Age attribute). The Partitioned
//     attribute (CHIPS-style per AMEND-7) is deferred per SPEC §8 item 15.
//
//   - parseAllCookies(headers, names) reads the Cookie: header values via
//     net/http.Request.Cookies and extracts the 5-cookie envelope by name.
//     Honors multi-Cookie-header semantics per RFC 9113 §8.2.3 (HTTP/2
//     receivers may split the cookie set across multiple Cookie header
//     fields). Duplicate cookie names collapse last-wins per the standard
//     server-side cookie-handling convention.
//
//   - formatSetCookie(name, value, attrs) emits a single Set-Cookie header
//     value byte-exact per SPEC §4.5: `name=value; Path=/; Secure; HttpOnly;
//     SameSite=Lax` with optional `; Max-Age=N` + `; Domain=...` suffixes
//     when the corresponding attrs fields are populated.
//
//   - addFlowCookieDeletionHeaders(headers, names, attrs) emits Max-Age=0
//     Set-Cookie headers for all 5 flow cookies per AMEND-3 + §4.5 category
//     (d). Used on the 401 + 302-challenge deny paths.
//
//   - emitCategoryA/B/C helpers exercise the per-category emission table per
//     SPEC §4.1 + §4.5. Each helper appends the cookies for its category to
//     the supplied response Header.
//
//   - formatExpiresValue(epochSeconds) returns the OauthExpires cookie value
//     per SPEC §12 item A3 — epoch-seconds-as-decimal-string.
//
// # Wire-format invariants per SPEC §4.5
//
// Set-Cookie attribute ordering pinned byte-exact: `Path=/; Secure; HttpOnly;
// SameSite=Lax`. Optional attributes (Max-Age + Domain) append AFTER the
// 4-attr base in fixed order: Max-Age first (when MaxAge!=nil), then Domain
// (when Domain!=""). The ordering closes SPEC §12 item A2 + matches reference
// Envoy v1.37.2 emission order; the IMPL Task 12 fixture-0024 scenario (a)
// cross-side byte-comparison confirms.
//
// # Host-only cookie discipline per §20.P2 + SPEC §8 item 12
//
// When SetCookieAttrs.Domain == "", NO `Domain=` attribute is emitted (host-
// only cookie scope per the browser-default + reference Envoy emission).
// Operators wanting cross-subdomain scope override via the `cookie_domain`
// field on OAuth2Credentials (per AMEND-6 C1; MVP defaults to empty).
//
// # IdToken handling at MVP
//
// IdToken is parsed but neither emitted at category (a) (n/a per §4.5 table)
// nor at category (b) (the callback flow does NOT populate IdToken at MVP per
// SPEC §2.2 — id_token consumption deferred). Category (c) sign-out DOES
// emit Max-Age=0 for IdToken (full envelope clearing per §4.5 — even
// deferred-write fields get the clearing emission). The IdToken cookie name
// is configurable per CookieNames.IdToken for forward-compatibility with the
// future id_token consumption phase.
//
// # No-panic discipline
//
// All read paths (parseAllCookies) tolerate arbitrary header values via
// net/http.Request.Cookies (the stdlib parser is fuzz-hardened). All write
// paths (formatSetCookie + emitCategory*) accept arbitrary string values
// without escaping or validation — operator-supplied cookie values are the
// caller's responsibility to constrain to the RFC 6265 cookie-octet alphabet.
// At fuzz-test time (Task 12 FuzzOAuth2ConfigParse — 26th fuzzer per §7.4),
// the dispatcher consumes parseAllCookies via the cookie-validate path and
// must-never-panic across arbitrary inbound bytes.

import (
	"net/http"
	"strconv"
	"time"
)

// CookieEnvelope is the 5-of-7 MVP-consumed cookie payload per phase-20
// ADR-0181 §Context + SPEC §6.4. All fields are the empty string when the
// corresponding cookie is absent from the request envelope.
type CookieEnvelope struct {
	// BearerToken carries the AES-256-CBC-encrypted access_token bytes per
	// ADR-0182 + AMEND-1.
	BearerToken string

	// OauthHMAC carries the 5-input HMAC over the envelope per ADR-0179 +
	// AMEND-2 (Base64URL-raw emit; dual-encoding read per S4).
	OauthHMAC string

	// OauthExpires carries the epoch-seconds-as-decimal-string per SPEC §12
	// item A3 (e.g. "1747522800"). NOT an RFC 3339 timestamp; NOT
	// milliseconds.
	OauthExpires string

	// IdToken carries the id_token value. Deferred per SPEC §2.2 — parsed
	// at MVP for forward-compatibility but not consumed by the dispatcher.
	IdToken string

	// RefreshToken carries the AES-256-CBC-encrypted refresh_token bytes per
	// ADR-0182. Empty when `use_refresh_token=false` or post-rotation
	// transient.
	RefreshToken string
}

// CookieNames carries the operator-configurable cookie-name overrides per
// phase-20 SPEC §6.4 (`cookie_names` proto field). Operators MAY rename any
// of the 5 envelope cookies; DefaultCookieNames() returns the upstream-
// canonical defaults per ADR-0181 §Context.
type CookieNames struct {
	BearerToken  string // default "BearerToken"
	OauthHMAC    string // default "OauthHMAC"
	OauthExpires string // default "OauthExpires"
	IdToken      string // default "IdToken"
	RefreshToken string // default "RefreshToken"
}

// DefaultCookieNames returns the upstream-canonical default cookie names per
// phase-20 ADR-0181 §Context. Byte-exact reference Envoy v1.37.2 names.
func DefaultCookieNames() CookieNames {
	return CookieNames{
		BearerToken:  "BearerToken",
		OauthHMAC:    "OauthHMAC",
		OauthExpires: "OauthExpires",
		IdToken:      "IdToken",
		RefreshToken: "RefreshToken",
	}
}

// SetCookieAttrs carries the per-Set-Cookie attribute payload per phase-20
// SPEC §4.5 + §12 item A2.
//
// The 4-attr base (Path + Secure + HttpOnly + SameSite) is emitted
// unconditionally with the field values verbatim. The 2 optional attributes
// (MaxAge + Domain) append AFTER the base in fixed order when populated.
type SetCookieAttrs struct {
	// Path is the Path= attribute value. MVP default "/".
	Path string

	// Secure indicates the Secure attribute is emitted. MVP default true.
	Secure bool

	// HttpOnly indicates the HttpOnly attribute is emitted. MVP default true.
	HttpOnly bool

	// SameSite is the SameSite= attribute value (typically "Lax" or "Strict").
	// MVP default "Lax" per §4.5.
	SameSite string

	// MaxAge controls the Max-Age= attribute emission per RFC 6265 §5.2.2.
	// nil → NO Max-Age attribute (session cookie; user-agent default lifetime).
	// non-nil → emit `Max-Age=N` where N is the duration in whole seconds.
	// A zero-Duration value emits `Max-Age=0` (sign-out clearing per §4.5
	// categories (c) + (d)).
	MaxAge *time.Duration

	// Domain controls the Domain= attribute emission. Empty → NO Domain
	// attribute (host-only cookie per §20.P2 RATIFIED + SPEC §8 item 12).
	// Non-empty → emit `Domain=<value>`.
	Domain string
}

// DefaultSetCookieAttrs returns the MVP-default Set-Cookie attribute set per
// phase-20 SPEC §4.5 + §12 item A2: Path=/ + Secure + HttpOnly + SameSite=Lax
// + host-only (no Domain) + no Max-Age (session-default lifetime).
func DefaultSetCookieAttrs() SetCookieAttrs {
	return SetCookieAttrs{
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: "Lax",
		// MaxAge: nil (session cookie)
		// Domain: "" (host-only)
	}
}

// parseAllCookies reads the Cookie: header(s) from `h` and extracts the
// 5-cookie envelope per `names`. Per RFC 9113 §8.2.3 the receiver MAY
// concatenate multiple Cookie header fields; this implementation iterates
// `h.Values("Cookie")` to cover both single- and multi-header request
// shapes. Duplicate cookie names collapse last-wins (server-side standard).
//
// Cookies absent from the request envelope appear as the empty string in the
// returned CookieEnvelope. The function NEVER panics on malformed Cookie
// header values — the stdlib net/http cookie parser silently drops malformed
// cookie-pairs.
func parseAllCookies(h http.Header, names CookieNames) CookieEnvelope {
	// net/http exposes cookie parsing only via *http.Request.Cookies; we
	// synthesize a minimal request bound to the supplied header.
	r := &http.Request{Header: h}
	all := r.Cookies()

	var env CookieEnvelope
	// Iterate in arrival order; later occurrences overwrite earlier ones
	// (last-wins per server-side cookie-handling convention).
	for _, c := range all {
		switch c.Name {
		case names.BearerToken:
			env.BearerToken = c.Value
		case names.OauthHMAC:
			env.OauthHMAC = c.Value
		case names.OauthExpires:
			env.OauthExpires = c.Value
		case names.IdToken:
			env.IdToken = c.Value
		case names.RefreshToken:
			env.RefreshToken = c.Value
		}
	}
	return env
}

// formatSetCookie emits a single Set-Cookie header value byte-exact per SPEC
// §4.5. The 4-attr base is unconditional; Max-Age + Domain append in fixed
// order when populated.
//
// Pre-allocation: the function uses a sized strings.Builder to avoid
// allocator churn on the hot per-emission path. Capacity is approximated as
// 96 bytes per typical-shape cookie value, which covers the common case
// without resizing.
func formatSetCookie(name, value string, attrs SetCookieAttrs) string {
	// Approximate the post-format length to right-size the builder.
	// Cookie pair: name=value
	// Base attrs: "; Path=" + path + "; Secure; HttpOnly; SameSite=" + sameSite
	// Optional: "; Max-Age=" + decimal-seconds; "; Domain=" + domain
	approxLen := len(name) + 1 + len(value) +
		8 + len(attrs.Path) +
		17 + // "; Secure; HttpOnly"
		11 + len(attrs.SameSite) // "; SameSite="
	if attrs.MaxAge != nil {
		approxLen += 11 + 11 // "; Max-Age=" + up to 11 digits for int32 seconds
	}
	if attrs.Domain != "" {
		approxLen += 9 + len(attrs.Domain) // "; Domain=" + domain
	}

	buf := make([]byte, 0, approxLen)
	buf = append(buf, name...)
	buf = append(buf, '=')
	buf = append(buf, value...)

	// Path — emitted unconditionally per the 4-attr base shape per §4.5.
	buf = append(buf, "; Path="...)
	buf = append(buf, attrs.Path...)

	// Secure — emitted when set (MVP default true).
	if attrs.Secure {
		buf = append(buf, "; Secure"...)
	}

	// HttpOnly — emitted when set (MVP default true).
	if attrs.HttpOnly {
		buf = append(buf, "; HttpOnly"...)
	}

	// SameSite — emitted unconditionally per the 4-attr base shape per §4.5.
	buf = append(buf, "; SameSite="...)
	buf = append(buf, attrs.SameSite...)

	// Max-Age — optional, per RFC 6265 §5.2.2. Whole-seconds decimal.
	if attrs.MaxAge != nil {
		seconds := int64(*attrs.MaxAge / time.Second)
		buf = append(buf, "; Max-Age="...)
		buf = strconv.AppendInt(buf, seconds, 10)
	}

	// Domain — optional, per SPEC §20.P2 host-only-when-empty discipline.
	if attrs.Domain != "" {
		buf = append(buf, "; Domain="...)
		buf = append(buf, attrs.Domain...)
	}

	return string(buf)
}

// formatExpiresValue formats `epochSeconds` as the OauthExpires cookie value
// per SPEC §12 item A3 — epoch-seconds-as-decimal-string.
func formatExpiresValue(epochSeconds int64) string {
	return strconv.FormatInt(epochSeconds, 10)
}

// withMaxAgeZero returns a copy of `attrs` with MaxAge set to the zero
// Duration (emits `Max-Age=0` for the sign-out / cleanup clearing semantic
// per §4.5 categories (c) + (d)).
func withMaxAgeZero(attrs SetCookieAttrs) SetCookieAttrs {
	zero := time.Duration(0)
	attrs.MaxAge = &zero
	return attrs
}

// addFlowCookieDeletionHeaders appends Max-Age=0 Set-Cookie headers for ALL
// 5 envelope cookies per AMEND-3 + SPEC §4.5 category (d). Used on the deny
// path (401 + 302-challenge) to clear stale flow state. The cookie value
// emitted is the empty string per the standard cookie-deletion idiom.
func addFlowCookieDeletionHeaders(h http.Header, names CookieNames, attrs SetCookieAttrs) {
	clearing := withMaxAgeZero(attrs)
	h.Add("Set-Cookie", formatSetCookie(names.BearerToken, "", clearing))
	h.Add("Set-Cookie", formatSetCookie(names.OauthHMAC, "", clearing))
	h.Add("Set-Cookie", formatSetCookie(names.OauthExpires, "", clearing))
	h.Add("Set-Cookie", formatSetCookie(names.IdToken, "", clearing))
	h.Add("Set-Cookie", formatSetCookie(names.RefreshToken, "", clearing))
}

// emitCategoryA_AuthChallenge emits the (a) 302 auth-challenge cookie payload
// per SPEC §4.5 category (a). Emits:
//   - Max-Age=0 clearing for the 4 wire-emitted envelope cookies (BearerToken
//     / OauthHMAC / OauthExpires / RefreshToken). IdToken is `(n/a)` per the
//     §4.5 table — NOT emitted on category (a).
//   - One state cookie SET to `stateValue` (typically HMAC(state)) with the
//     base 4-attr (no Max-Age).
//
// The state cookie name is supplied explicitly (`stateCookieName`) because it
// is configured outside the 5-cookie envelope (per phase-09 async-resume +
// the state-cookie discipline at §4.4 + ADR-0180). The value layout (epoch-
// seconds prefix + HMAC) is determined upstream of this helper.
func emitCategoryA_AuthChallenge(
	h http.Header,
	names CookieNames,
	attrs SetCookieAttrs,
	stateCookieName, stateValue string,
) {
	clearing := withMaxAgeZero(attrs)
	// 4 wire-emitted envelope cookies cleared per §4.5 category (a).
	h.Add("Set-Cookie", formatSetCookie(names.BearerToken, "", clearing))
	h.Add("Set-Cookie", formatSetCookie(names.OauthHMAC, "", clearing))
	h.Add("Set-Cookie", formatSetCookie(names.OauthExpires, "", clearing))
	h.Add("Set-Cookie", formatSetCookie(names.RefreshToken, "", clearing))
	// State cookie SET with the base 4-attr shape (no Max-Age).
	h.Add("Set-Cookie", formatSetCookie(stateCookieName, stateValue, attrs))
}

// emitCategoryB_PostCallback emits the (b) 302 post-callback-success cookie
// payload per SPEC §4.5 category (b). Emits:
//   - BearerToken SET to `encAccessToken` (the AES-256-CBC-encrypted bytes
//     per ADR-0182 / AMEND-1).
//   - OauthHMAC SET to `hmacValue` (the 5-input HMAC per ADR-0179 / AMEND-2).
//   - OauthExpires SET to the epoch-seconds-decimal-string per §12 A3.
//   - RefreshToken SET to `encRefreshToken` when non-empty;
//     when empty (use_refresh_token=false), the RefreshToken cookie is
//     OMITTED entirely.
//
// IdToken is `(n/a)` per the §4.5 table — NOT emitted on category (b).
func emitCategoryB_PostCallback(
	h http.Header,
	names CookieNames,
	attrs SetCookieAttrs,
	encAccessToken string,
	hmacValue string,
	expiresEpoch int64,
	encRefreshToken string,
) {
	h.Add("Set-Cookie", formatSetCookie(names.BearerToken, encAccessToken, attrs))
	h.Add("Set-Cookie", formatSetCookie(names.OauthHMAC, hmacValue, attrs))
	h.Add("Set-Cookie", formatSetCookie(names.OauthExpires, formatExpiresValue(expiresEpoch), attrs))
	if encRefreshToken != "" {
		h.Add("Set-Cookie", formatSetCookie(names.RefreshToken, encRefreshToken, attrs))
	}
}

// emitCategoryC_Signout emits the (c) 302 sign-out cookie payload per SPEC
// §4.5 category (c). Emits Max-Age=0 for ALL 5 envelope cookies (full
// envelope clearing — even deferred-write IdToken gets the clearing
// emission). Alias for addFlowCookieDeletionHeaders — both paths use the
// same full-envelope-clear shape per §4.5.
func emitCategoryC_Signout(h http.Header, names CookieNames, attrs SetCookieAttrs) {
	addFlowCookieDeletionHeaders(h, names, attrs)
}
