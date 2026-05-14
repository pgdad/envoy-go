// Package jwt implements the JWS/JWT parser + signature verifier + claim
// validator framework primitive at the NEW top-level package `internal/jwt/`
// per ADR-0151. This package is strategically positioned for cross-phase
// reuse: the `Parse` + `Token.VerifySignature` + `Token.ValidateClaims` +
// `Token.PayloadClaim` API composes against future filters consuming JWT
// semantics — a hypothetical `jwt_claim_router` filter routing on claim
// values, an `oauth2` token-validation step, or extension authn filters all
// reuse the same parse/verify/validate surface. Mirrors phase-16 ADR-0142
// (`internal/matcher/`) + ADR-0144 (DownstreamPrincipal accessor) cross-
// phase-reusable-framework-primitive discipline; co-anchors with ADR-0148
// (jwt_authn package shape) + ADR-0149 (compiledConfig shape) + ADR-0150
// (`internal/jwks/` framework primitive).
//
// API surface (per ADR-0151 §Decision):
//
//   - `Parse(raw string) (*Token, error)` — parses 3-part JWT structure;
//     base64url-decodes header + payload + signature; JSON-decodes header +
//     payload; reads alg + kid from header. Does NOT verify signature or
//     enforce claims at parse time (caller's two-step responsibility).
//   - `(*Token).VerifySignature(key crypto.PublicKey, alg string) error` —
//     verifies signature using the named algorithm against the provided
//     public key; PARSE-REJECT for algorithms outside the 6-entry RS+ES
//     allow-list per §11.P1 (HS family + EdDSA + `none` + PS family →
//     ErrJwtHeaderNotImplementedAlg).
//   - `(*Token).ValidateClaims(opts ValidateOptions) error` — checks exp +
//     nbf + iss + aud per options with clock-skew tolerance applied to both
//     exp + nbf per §1.1 amendment 7; returns canonical errors on rejection.
//   - `(*Token).PayloadClaim(path string) (interface{}, error)` — extracts
//     a claim by dot-notation path; array-claims rejected (ErrArrayClaim)
//     per §11.P10 + §1.1 amendment 10; missing path returns
//     ErrClaimNotFound.
//
// Algorithm allow-list (6 entries) per §11.P1:
//
//   - RS256/384/512 via `crypto/rsa.VerifyPKCS1v15` + SHA-256/384/512.
//   - ES256/384/512 via `crypto/ecdsa.Verify` against P-256/P-384/P-521;
//     signature is `r||s` raw concatenation per RFC 7515 §A — signature byte
//     length is `2 * (BitSize+7)/8` (64 / 96 / 132 bytes respectively).
//   - All other algs (HS family + EdDSA + `none` + PS family) →
//     ErrJwtHeaderNotImplementedAlg. HS family DEFERRED per §8 deferral 5
//     (requires symmetric-secret config plumbing); EdDSA DEFERRED per §8
//     deferral 6; `none` DEFERRED-PERMANENTLY per §8 deferral 7 (security-
//     sensitive); PS family DEFERRED per SPEC §12.6.
//
// Signed-data shape (JWS §5): the bytes signed are the base64url-encoded
// `header.payload` STRING (NO further base64url-decoding). For ECDSA verify
// the raw `r||s` signature is split into the two `*big.Int` halves before
// invoking `ecdsa.Verify`.
//
// Key-type vs alg validation: RS-family + non-RSA key → ErrJwtVerificationFail
// (NOT NotImplementedAlg; NotImplementedAlg is reserved for the alg-not-in-
// allow-list path). Same for ES-family + non-EC key. Signature byte-length
// mismatch (e.g., truncated ES256 sig) → ErrJwtVerificationFail.
//
// Claim validation order per ADR-0151 §Decision + §11.P16 RATIFIED-AND-
// EXTENDED via `authenticator.cc` scrape:
//
//  1. `exp` — if present in payload: `now - ClockSkew <= exp` required;
//     else ErrJwtExpired.
//  2. `nbf` — if present in payload: `nbf <= now + ClockSkew` required;
//     else ErrJwtNotYetValid.
//  3. `iss` — if `opts.Issuer != ""`: payload `iss` must exact-match; else
//     ErrJwtUnknownIssuer.
//  4. `aud` — if `len(opts.Audiences) > 0`: payload `aud` (string or
//     []string) must share at least one entry with `opts.Audiences`; else
//     ErrJwtAudienceNotAllowed.
//
// `iat` (issued-at) is informational only; not enforced for rejection
// (mirrors Envoy semantic).
//
// Silent-ignored ValidateOptions fields per §1.1 amendment 3 (accepted at
// the API but never enforced):
//
//   - `RequireExpiration` — would mandate the `exp` claim; silent-ignored.
//   - `MaxLifetime` — would cap `exp - iat`; silent-ignored.
//   - `Subjects` — would match the `sub` claim against a StringMatcher;
//     silent-ignored. The `StringMatcher` type is exposed for API-shape
//     parity with the proto (v1.37.x JWT-SVID-class extensions) but
//     ValidateClaims never inspects it.
//
// Error sentinels per §11.P1 jwt_verify_lib `status.cc` status codes — the
// `Error()` STRINGS are byte-exact to `getStatusString()` output because
// they become deny-response body content at Task 9 + ADR-0155. The exact
// byte-counts (Jwt is missing = 14B; Jwt is expired = 14B; Jwt verification
// fails = 22B; Jwt issuer is not configured = 28B; Audiences in Jwt are not
// allowed = 32B; Jwt not yet valid = 17B; Jwt header [alg] is not supported
// = 33B) are pinned by Task 9 + ADR-0155 wire-shape acceptance.
//
// Cross-package handshake: this package is a SIBLING of `internal/jwks/`;
// they do NOT import each other. `internal/jwt/` handles JWT parse + verify
// + validate; `internal/jwks/` handles JWK Set fetching + parsing + key
// lookup. The CALLER (`internal/filter/http/jwtauthn/`) composes the two at
// Task 6's evaluateProvider.
//
// Pure-Go stdlib only (NO third-party JWT library) per ADR-0151 §Decision +
// envoy-go zero-third-party-runtime-deps discipline (mirrors phase-06.1
// ADR-0058 + phase-14 ADR-0129 + phase-16 zero-third-party precedent).
package jwt
