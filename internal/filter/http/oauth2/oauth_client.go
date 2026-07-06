package oauth2

// oauth_client.go — token_endpoint POST helper + body templates + custom
// urlEncode percent-encoder per phase-20 SPEC §6.7 + AMEND-5 + §20.P10
// RATIFIED + ADR-0185.
//
// # Surface
//
// Three package-internal functions consumed by the callback-flow auth-code
// leg (Task 11 wire-up) + the refresh-token leg (Task 8 wire-up via the
// `tokenEndpointPosterFn` abstraction declared in compiled_config.go):
//
//   - urlEncode(value string) string — percent-encodes the input per the
//     §20.P10 RATIFIED charset (see "urlEncode discipline" below).
//
//   - buildTokenRequestBody(grantType string, params map[string]string) []byte
//     — returns the byte-exact `application/x-www-form-urlencoded` body for
//     the token_endpoint POST. Two templates per AMEND-5:
//       * "authorization_code" → 4-field MVP template (PKCE-gated 5th field
//         absent per §2.1 + S3);
//       * "refresh_token"       → 3-field template.
//     The field ORDER inside each template is fixed (matches upstream Envoy
//     v1.37.2 oauth_client.cc); missing param values emit as empty strings.
//
//   - postTokenEndpoint(ctx, *httpclient.Client, endpoint, body) →
//     (*http.Response, error) — wraps the stdlib http.NewRequestWithContext +
//     the ADR-0177 *httpclient.Client.Do contract. Content-Type is fixed at
//     `application/x-www-form-urlencoded` per RFC 6749 §4.1.3 + §6.
//
// # urlEncode discipline (§20.P10 RATIFIED + AMEND-5 + ADR-0185)
//
// The custom encoder percent-encodes every byte OUTSIDE the RFC 3986
// "unreserved" set: `A-Z a-z 0-9 - . _ ~`. Consequences:
//
//   - The 5 reserved characters explicitly called out by §20.P10 — `:`, `/`,
//     `=`, `&`, `?` — are encoded as `%3A`, `%2F`, `%3D`, `%26`, `%3F`. This
//     is the byte-exact divergence from stdlib `url.PathEscape` (which leaves
//     `:` + `/` un-encoded under the path-segment-encoding discipline) +
//     `url.QueryEscape` (which encodes spaces as `+` rather than `%20`).
//
//   - Spaces encode as `%20` (NOT `+`) — matches upstream Envoy + RFC 3986
//     percent-encoding discipline.
//
//   - Non-ASCII bytes encode as `%XX` per their literal UTF-8 byte sequence
//     (this is the stdlib UTF-8 escaping fall-out; vector-tested per §12
//     item A5 RATIFIED-AT-IMPL + D16 A5).
//
// # tokenEndpointPosterFn production binding (Task 11 wire-up note)
//
// The `tokenEndpointPosterFn` abstraction in compiled_config.go is populated
// at Task 11 by wrapping `postTokenEndpoint` + `buildTokenRequestBody` —
// the wrapper closure builds the body via buildTokenRequestBody (mapping
// the (grantType, codeOrRefreshToken, clientID, clientSecret) tuple to the
// fixed-template params map) then dispatches to postTokenEndpoint with the
// compiledConfig's *httpclient.Client + token_endpoint URL. Task 10 lands
// the three functions in this file; Task 11 lands the wrapper.
//
// # POST callback's inbound URL-decode counterpart (Item C)
//
// The OUTBOUND `urlEncode` discipline has an INBOUND symmetric counterpart
// at `callback.go::extractCallbackParams`: the `code` + `state` query
// parameters on the callback URL are URL-DECODED before consumption per the
// AMEND-5 + ADR-0185 §Decision (the auth-server percent-encoded these on
// the redirect-back; the filter undoes the encoding on read). The decode
// uses `url.QueryUnescape` per RFC 3986 §2.4 — see the extractCallbackParams
// docstring + tests in oauth_client_test.go for the vector coverage.

import (
	"bytes"
	"context"
	"errors"
	"net/http"

	"github.com/pgdad/envoy-go/internal/httpclient"
)

// grantTypeAuthorizationCode is the OAuth 2.0 `grant_type` value for the
// authorization-code POST template per RFC 6749 §4.1.3 + AMEND-5 + ADR-0185.
// Paired with `grantTypeRefreshToken` (declared in callback.go for symmetry
// with handleRefresh — the two constants together form the AMEND-5 template-
// selector keyspace).
const grantTypeAuthorizationCode = "authorization_code"

// contentTypeFormURLEncoded is the fixed Content-Type request header for
// the token_endpoint POST per RFC 6749 §4.1.3 + §6 + upstream Envoy v1.37.2
// oauth_client.cc.
const contentTypeFormURLEncoded = "application/x-www-form-urlencoded"

// errNilHTTPClient is the sentinel returned by postTokenEndpoint when the
// supplied *httpclient.Client is nil. Defense-in-depth — the production
// wiring at Task 11 always supplies a non-nil client; a misconfigured boot
// path surfaces this error rather than panicking inside the resume
// goroutine.
var errNilHTTPClient = errors.New("oauth2: postTokenEndpoint called with nil *httpclient.Client")

// urlEncode percent-encodes the input string per the §20.P10 RATIFIED +
// AMEND-5 + ADR-0185 charset:
//
//   - Bytes in the RFC 3986 §2.3 "unreserved" set (`A-Z a-z 0-9 - . _ ~`)
//     pass through verbatim.
//   - All other bytes — including the explicitly-called-out reserved set
//     `:/=&?`, spaces, and non-ASCII UTF-8 bytes — encode as `%XX` with
//     UPPERCASE hex digits.
//
// The function is byte-exact-divergent from stdlib `url.PathEscape` (which
// leaves `:` + `/` un-encoded) and from stdlib `url.QueryEscape` (which
// encodes spaces as `+`). The custom helper exists per ADR-0185 §Decision
// because neither stdlib helper matches upstream Envoy v1.37.2's
// PercentEncoding byte-exact behavior.
//
// Per §12 item A5 RATIFIED-AT-IMPL: non-ASCII bytes encode per their literal
// UTF-8 byte sequence (e.g. `ä` → `%C3%A4`); this is the stdlib UTF-8
// escaping fall-out, vector-tested via TestUrlEncode_NonAsciiBytes_*.
//
// Implementation: byte-loop with a single-byte unreserved-set classifier +
// `bytes.Buffer` accumulator. The accumulator pre-allocates `len(value)` bytes
// (worst case is 3x len for all-encoded input; the common ASCII-mostly case
// is close to the input size; the pre-allocate avoids the first 1-2 growths).
func urlEncode(value string) string {
	// Fast-path: empty input.
	if len(value) == 0 {
		return ""
	}

	// Fast-path: walk once to check whether ANY byte needs encoding. If not,
	// return the input verbatim without allocating.
	allUnreserved := true
	for i := 0; i < len(value); i++ {
		if !isUnreservedByte(value[i]) {
			allUnreserved = false
			break
		}
	}
	if allUnreserved {
		return value
	}

	// Slow-path: emit a percent-encoded form.
	var buf bytes.Buffer
	buf.Grow(len(value))
	for i := 0; i < len(value); i++ {
		b := value[i]
		if isUnreservedByte(b) {
			buf.WriteByte(b)
			continue
		}
		buf.WriteByte('%')
		buf.WriteByte(hexUpper[b>>4])
		buf.WriteByte(hexUpper[b&0x0F])
	}
	return buf.String()
}

// isUnreservedByte reports whether b is in the RFC 3986 §2.3 "unreserved"
// set: `A-Z a-z 0-9 - . _ ~`. Bytes outside this set are percent-encoded by
// urlEncode per §20.P10 + AMEND-5.
func isUnreservedByte(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '-' || b == '.' || b == '_' || b == '~':
		return true
	default:
		return false
	}
}

// hexUpper is the uppercase hex alphabet for the urlEncode percent-encoder.
// Matches RFC 3986 §2.1 + upstream Envoy PercentEncoding (which emits
// uppercase hex digits in `%XX` sequences).
const hexUpper = "0123456789ABCDEF"

// buildTokenRequestBody returns the byte-exact `application/x-www-form-
// urlencoded` body for the token_endpoint POST per AMEND-5 + §20.P10
// RATIFIED + ADR-0185.
//
// Two templates per grantType (field ORDER inside each template is FIXED
// to match upstream Envoy v1.37.2 oauth_client.cc):
//
//   - "authorization_code" → 4-field MVP template:
//     `grant_type=authorization_code&code={code}&client_id={client_id}&
//     client_secret={client_secret}&redirect_uri={redirect_uri}`
//     The PKCE 5th field `&code_verifier={code_verifier}` is GATED per
//     §2.1 + S3 — absent at MVP; a future PKCE-enabling phase toggles the
//     5th field on per ADR-0185 §Decision.
//
//   - "refresh_token" → 3-field template:
//     `grant_type=refresh_token&refresh_token={refresh_token}&
//     client_id={client_id}&client_secret={client_secret}`
//
// Each field's value is urlEncode-percent-encoded per §20.P10. Missing
// params (absent key in the map) emit as empty string values — the field
// is ALWAYS present in the template; only the value varies.
//
// Unknown grantType returns an empty body — defense-in-depth; the caller
// (the Task 11 wrapper around postTokenEndpoint) always supplies one of
// the two recognized constants.
func buildTokenRequestBody(grantType string, params map[string]string) []byte {
	switch grantType {
	case grantTypeAuthorizationCode:
		// 4-field MVP template. PKCE 5th field is GATED per §2.1 + S3 —
		// any "code_verifier" key in params is IGNORED at MVP.
		var buf bytes.Buffer
		buf.WriteString("grant_type=authorization_code")
		buf.WriteString("&code=")
		buf.WriteString(urlEncode(params["code"]))
		buf.WriteString("&client_id=")
		buf.WriteString(urlEncode(params["client_id"]))
		buf.WriteString("&client_secret=")
		buf.WriteString(urlEncode(params["client_secret"]))
		buf.WriteString("&redirect_uri=")
		buf.WriteString(urlEncode(params["redirect_uri"]))
		return buf.Bytes()
	case grantTypeRefreshToken:
		// 3-field template per RFC 6749 §6.
		var buf bytes.Buffer
		buf.WriteString("grant_type=refresh_token")
		buf.WriteString("&refresh_token=")
		buf.WriteString(urlEncode(params["refresh_token"]))
		buf.WriteString("&client_id=")
		buf.WriteString(urlEncode(params["client_id"]))
		buf.WriteString("&client_secret=")
		buf.WriteString(urlEncode(params["client_secret"]))
		return buf.Bytes()
	default:
		// Unknown grant type — empty body. Caller responsibility to pass
		// one of the two recognized constants; this is defense-in-depth.
		return nil
	}
}

// postTokenEndpoint dispatches the token_endpoint POST via the supplied
// *httpclient.Client per phase-20 SPEC §6.7 + ADR-0185 + ADR-0177 (the
// framework outbound-HTTP primitive).
//
// Per AMEND-5 + §20.P10:
//   - HTTP method: POST
//   - Content-Type request header: application/x-www-form-urlencoded
//   - Body: the byte-exact wire-encoded form produced by buildTokenRequestBody
//
// Returns the *http.Response from httpclient.Client.Do verbatim — caller
// (applyTokenEndpointResponse / applyRefreshTokenResponse) classifies the
// disposition by resp.StatusCode per SPEC §4.7 + AMEND-3 (2xx → success;
// 5xx → retry-eligible 302 challenge; 4xx → terminal 401).
//
// On transport-level error (connection refused / context canceled / TLS
// handshake failure) returns (nil, err) — caller routes through the
// refresh-failure leg per AMEND-3.
//
// Per ADR-0159 D4 + extauthz precedent: the supplied ctx is wired into the
// outbound request via http.NewRequestWithContext — OnDestroy cancels the
// ctx via the per-stream callCancel, which surfaces here as ctx.Err()
// propagating out of httpclient.Client.Do.
func postTokenEndpoint(ctx context.Context, httpClient *httpclient.Client, endpoint string, body []byte) (*http.Response, error) {
	// Defense-in-depth: nil-client guard per the misconfigured-boot-path
	// rationale at compiled_config.go::tokenEndpointPosterFn docstring.
	if httpClient == nil {
		return nil, errNilHTTPClient
	}

	// bytes.Reader is the canonical body shape for the *httpclient.Client.Do
	// retry loop per ADR-0177 — implements io.ReadSeeker so the retry path
	// could in principle rewind (the oauth2 consumer uses Attempts=0 per
	// §20.P1 zero-retry default; rewind never fires, but the body shape is
	// retry-compatible per the consumer-side body-rewind responsibility
	// documented in ADR-0177 §Decision).
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentTypeFormURLEncoded)

	return httpClient.Do(req)
}
