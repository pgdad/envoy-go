package jwt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"
)

// ----------------------------------------------------------------------------
// Canonical error sentinels per §11.P1 jwt_verify_lib status table.
//
// The Error() STRINGS are byte-exact to jwt_verify_lib's getStatusString()
// output because they become deny-response body content at Task 9 + ADR-0155.
// Specific byte-counts called out in PLAN Task 9 are pinned by
// TestErrSentinelsByteExact:
//
//	Jwt is missing                   14B
//	Jwt is expired                   14B
//	Jwt not yet valid                17B
//	Jwt verification fails           22B
//	Jwt issuer is not configured     28B
//	Audiences in Jwt are not allowed 32B
//	Jwt header [alg] is not supported 33B
// ----------------------------------------------------------------------------

var (
	// ErrJwtMissed surfaces when no JWT was extracted from the request (the
	// token-extractor returned empty). 14 bytes. Used by Task 5 + Task 9.
	ErrJwtMissed = errors.New("Jwt is missing")

	// ErrJwtBadFormat surfaces when the raw JWT does not split into 3 dot-
	// separated parts.
	ErrJwtBadFormat = errors.New("Jwt is not in the form of Header.Payload.Signature")

	// ErrJwtHeaderParseErrorBadBase64 surfaces when the header part is not
	// valid base64url.
	ErrJwtHeaderParseErrorBadBase64 = errors.New("Jwt header is an invalid Base64url encoded.")

	// ErrJwtHeaderParseErrorBadJson surfaces when the decoded header is not
	// valid JSON.
	ErrJwtHeaderParseErrorBadJson = errors.New("Jwt header is an invalid JSON.")

	// ErrJwtPayloadParseErrorBadBase64 surfaces when the payload part is not
	// valid base64url.
	ErrJwtPayloadParseErrorBadBase64 = errors.New("Jwt payload is an invalid Base64url encoded.")

	// ErrJwtPayloadParseErrorBadJson surfaces when the decoded payload is not
	// valid JSON.
	ErrJwtPayloadParseErrorBadJson = errors.New("Jwt payload is an invalid JSON.")

	// ErrJwtSignatureParseErrorBadBase64 surfaces when the signature part is
	// not valid base64url.
	ErrJwtSignatureParseErrorBadBase64 = errors.New("Jwt signature is an invalid Base64url encoded.")

	// ErrJwtHeaderBadAlg surfaces when the header `alg` field is missing or
	// not a string.
	ErrJwtHeaderBadAlg = errors.New("Jwt header [alg] field is required.")

	// ErrJwtHeaderNotImplementedAlg surfaces when the algorithm is outside the
	// 6-entry RS+ES allow-list (HS / EdDSA / none / PS rejected). 33 bytes.
	// NOTE: no trailing period — the 33B count called out in PLAN Task 9
	// pins the no-period form (mirrors Envoy jwt_verify_lib status.cc
	// `Jwt header [alg] is not supported` literal).
	ErrJwtHeaderNotImplementedAlg = errors.New("Jwt header [alg] is not supported")

	// ErrJwtVerificationFail surfaces when the signature does not verify (any
	// reason including key-type-vs-alg mismatch and signature byte-length
	// mismatch). 22 bytes.
	ErrJwtVerificationFail = errors.New("Jwt verification fails")

	// ErrJwtExpired surfaces when the `exp` claim is past the acceptance
	// window (now - ClockSkew > exp). 14 bytes.
	ErrJwtExpired = errors.New("Jwt is expired")

	// ErrJwtNotYetValid surfaces when the `nbf` claim is ahead of the
	// acceptance window (now + ClockSkew < nbf). 17 bytes.
	ErrJwtNotYetValid = errors.New("Jwt not yet valid")

	// ErrJwtUnknownIssuer surfaces when the `iss` claim does not match
	// ValidateOptions.Issuer (when set). 28 bytes.
	ErrJwtUnknownIssuer = errors.New("Jwt issuer is not configured")

	// ErrJwtAudienceNotAllowed surfaces when the `aud` claim does not
	// intersect ValidateOptions.Audiences (when set). 32 bytes. NOTE: this
	// status maps to HTTP 403 (NOT 401) at the filter layer per §1.1
	// amendment 8 + ADR-0155.
	ErrJwtAudienceNotAllowed = errors.New("Audiences in Jwt are not allowed")

	// ErrArrayClaim surfaces from PayloadClaim when the resolved value is an
	// array — array-valued claims are not supported per §11.P10 + §1.1
	// amendment 10 + JwtClaimToHeader.claim_name proto comment.
	ErrArrayClaim = errors.New("claim is array-valued; not supported")

	// ErrClaimNotFound surfaces from PayloadClaim when the path does not
	// resolve (missing key OR intermediate non-object node).
	ErrClaimNotFound = errors.New("claim not found")
)

// ----------------------------------------------------------------------------
// Public types.
// ----------------------------------------------------------------------------

// Token represents a parsed JWT. After Parse, Header + Payload carry the
// JSON-decoded `map[string]interface{}` shape; Alg + Kid are extracted from
// the header for convenience. RawHeader / RawPayload / RawSignature carry the
// base64url-encoded form (NOT decoded) — needed by VerifySignature for the
// JWS §5 signed-data shape (the base64url-encoded `header.payload` string is
// what gets signed, NOT the decoded bytes).
type Token struct {
	RawHeader    string // base64url-encoded header part
	RawPayload   string // base64url-encoded payload part
	RawSignature string // base64url-encoded signature part

	Header  map[string]interface{} // JSON-decoded header
	Payload map[string]interface{} // JSON-decoded payload

	Alg string // header `alg` field (extracted; verified non-empty + string by Parse)
	Kid string // header `kid` field (extracted; may be empty)
}

// ValidateOptions controls claim validation. The last 3 fields are SILENT-
// IGNORED per §1.1 amendment 3 — they are accepted at the API for v1.37.x
// JWT-SVID-class proto-shape parity but never enforced by ValidateClaims.
type ValidateOptions struct {
	// Issuer: exact-match against payload `iss` claim. Empty → skip iss check.
	Issuer string

	// Audiences: at least one entry must appear in payload `aud` (which may
	// be string OR []string). Empty → skip aud check.
	Audiences []string

	// ClockSkew: tolerance applied SYMMETRICALLY to exp + nbf. Caller is
	// expected to default this to 60s per §1.1 amendment 7; ValidateClaims
	// itself treats zero as zero tolerance.
	ClockSkew time.Duration

	// Now: injection point for deterministic tests. Zero value uses
	// time.Now() per the standard Go zero-value-is-default discipline.
	Now time.Time

	// RequireExpiration is SILENT-IGNORED per §1.1 amendment 3. The field is
	// retained for API-shape parity with v1.37.x JwtProvider.require_expiration.
	RequireExpiration bool

	// MaxLifetime is SILENT-IGNORED per §1.1 amendment 3. The field is
	// retained for API-shape parity with v1.37.x JwtProvider.max_lifetime.
	MaxLifetime time.Duration

	// Subjects is SILENT-IGNORED per §1.1 amendment 3. The pointer-type
	// matches the v1.37.x JwtProvider.subjects (StringMatcher) shape.
	Subjects *StringMatcher
}

// StringMatcher is a placeholder type per §1.1 amendment 3 — accepted at the
// API but never enforced by ValidateClaims. Mirrors the proto3 StringMatcher
// shape (Exact / Prefix / Suffix) for v1.37.x JwtProvider.subjects parity.
type StringMatcher struct {
	Exact  string
	Prefix string
	Suffix string
}

// ----------------------------------------------------------------------------
// Parse: 3-part header.payload.signature decoding per ADR-0151 §Decision.
// ----------------------------------------------------------------------------

// Parse parses a 3-part header.payload.signature JWT.
//
// Returns ErrJwtBadFormat if not exactly 3 parts.
// Returns ErrJwtHeaderParseErrorBadBase64 / ErrJwtHeaderParseErrorBadJson on
// header decode failures.
// Returns ErrJwtPayloadParseErrorBadBase64 / ErrJwtPayloadParseErrorBadJson on
// payload decode failures.
// Returns ErrJwtSignatureParseErrorBadBase64 on signature decode failure (the
// raw signature bytes are stored back-encoded for VerifySignature use).
// Returns ErrJwtHeaderBadAlg if the alg field is missing OR not a string.
//
// Parse does NOT verify the signature and does NOT enforce claims — those are
// the caller's two-step responsibility (VerifySignature + ValidateClaims).
func Parse(raw string) (*Token, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, ErrJwtBadFormat
	}

	rawHeader, rawPayload, rawSig := parts[0], parts[1], parts[2]

	// Decode header.
	headerBytes, err := base64.RawURLEncoding.DecodeString(rawHeader)
	if err != nil {
		return nil, ErrJwtHeaderParseErrorBadBase64
	}
	var header map[string]interface{}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrJwtHeaderParseErrorBadJson
	}

	// Decode payload.
	payloadBytes, err := base64.RawURLEncoding.DecodeString(rawPayload)
	if err != nil {
		return nil, ErrJwtPayloadParseErrorBadBase64
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, ErrJwtPayloadParseErrorBadJson
	}

	// Verify the signature part is valid base64url (we don't need the decoded
	// bytes here — VerifySignature re-decodes — but a malformed signature is
	// a Parse-time error per ADR-0151 §Decision).
	if _, err := base64.RawURLEncoding.DecodeString(rawSig); err != nil {
		return nil, ErrJwtSignatureParseErrorBadBase64
	}

	// Extract alg + kid. alg is REQUIRED + must be a string.
	algRaw, hasAlg := header["alg"]
	if !hasAlg {
		return nil, ErrJwtHeaderBadAlg
	}
	alg, ok := algRaw.(string)
	if !ok || alg == "" {
		return nil, ErrJwtHeaderBadAlg
	}
	var kid string
	if kidRaw, hasKid := header["kid"]; hasKid {
		// kid is optional; tolerate non-string as "no kid" rather than error
		// (mirrors Envoy semantic — Jwks::pickKeyAlgWithKid handles empty
		// kid).
		if s, ok := kidRaw.(string); ok {
			kid = s
		}
	}

	return &Token{
		RawHeader:    rawHeader,
		RawPayload:   rawPayload,
		RawSignature: rawSig,
		Header:       header,
		Payload:      payload,
		Alg:          alg,
		Kid:          kid,
	}, nil
}

// ----------------------------------------------------------------------------
// VerifySignature: 6-alg dispatch + RS+ES verification.
// ----------------------------------------------------------------------------

// VerifySignature verifies the token signature with the given public key + alg.
//
// alg MUST be in the 6-entry RS+ES allow-list per §11.P1 (RS256, RS384, RS512,
// ES256, ES384, ES512). Other algs (HS family, EdDSA, none, PS family) →
// ErrJwtHeaderNotImplementedAlg.
//
// On any verification failure (signature mismatch, key-type-vs-alg mismatch,
// signature byte-length mismatch for ECDSA) → ErrJwtVerificationFail.
//
// The signed-data shape per JWS §5 is the base64url-encoded
// `header.payload` STRING (NO further base64url-decoding before hashing).
func (t *Token) VerifySignature(key crypto.PublicKey, alg string) error {
	if t == nil {
		return ErrJwtVerificationFail
	}
	signed := []byte(t.RawHeader + "." + t.RawPayload)

	sig, err := base64.RawURLEncoding.DecodeString(t.RawSignature)
	if err != nil {
		// The signature part was Parse-validated to be base64url; defensively
		// surface a verification failure rather than a panic if a caller
		// mutated RawSignature post-Parse.
		return ErrJwtVerificationFail
	}

	switch alg {
	case "RS256", "RS384", "RS512":
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return ErrJwtVerificationFail
		}
		hashed, hashAlg := rsaHash(alg, signed)
		if err := rsa.VerifyPKCS1v15(rsaKey, hashAlg, hashed, sig); err != nil {
			return ErrJwtVerificationFail
		}
		return nil

	case "ES256", "ES384", "ES512":
		ecKey, ok := key.(*ecdsa.PublicKey)
		if !ok {
			return ErrJwtVerificationFail
		}
		if ecKey.Curve == nil {
			return ErrJwtVerificationFail
		}
		// Expected signature length is `2 * (BitSize+7)/8` per RFC 7515 §A.
		byteSize := (ecKey.Curve.Params().BitSize + 7) / 8
		expected := 2 * byteSize
		if len(sig) != expected {
			return ErrJwtVerificationFail
		}
		r := new(big.Int).SetBytes(sig[:byteSize])
		s := new(big.Int).SetBytes(sig[byteSize:])
		hashed := ecHash(alg, signed)
		if !ecdsa.Verify(ecKey, hashed, r, s) {
			return ErrJwtVerificationFail
		}
		return nil

	default:
		// HS family, EdDSA, none, PS family — all rejected.
		return ErrJwtHeaderNotImplementedAlg
	}
}

// rsaHash returns the alg-mapped hash digest + crypto.Hash identifier for RS
// dispatch.
func rsaHash(alg string, signed []byte) ([]byte, crypto.Hash) {
	switch alg {
	case "RS256":
		sum := sha256.Sum256(signed)
		return sum[:], crypto.SHA256
	case "RS384":
		sum := sha512.Sum384(signed)
		return sum[:], crypto.SHA384
	case "RS512":
		sum := sha512.Sum512(signed)
		return sum[:], crypto.SHA512
	default:
		// Unreachable — caller already switched on alg.
		return nil, 0
	}
}

// ecHash returns the alg-mapped hash digest for ES dispatch.
func ecHash(alg string, signed []byte) []byte {
	switch alg {
	case "ES256":
		sum := sha256.Sum256(signed)
		return sum[:]
	case "ES384":
		sum := sha512.Sum384(signed)
		return sum[:]
	case "ES512":
		sum := sha512.Sum512(signed)
		return sum[:]
	default:
		// Unreachable — caller already switched on alg.
		return nil
	}
}

// ----------------------------------------------------------------------------
// ValidateClaims: exp + nbf + iss + aud per §6.8 + §11.P16.
// ----------------------------------------------------------------------------

// ValidateClaims checks exp + nbf + iss + aud in the order:
//
//  1. `exp` — if present in payload: `now - ClockSkew <= exp` required;
//     else ErrJwtExpired. Clock skew widens the acceptance window.
//  2. `nbf` — if present in payload: `nbf <= now + ClockSkew` required;
//     else ErrJwtNotYetValid. Clock skew widens the acceptance window.
//  3. `iss` — if `opts.Issuer != ""`: payload `iss` (as string) must equal
//     opts.Issuer; else ErrJwtUnknownIssuer.
//  4. `aud` — if `len(opts.Audiences) > 0`: payload `aud` (either a string
//     or a JSON array of strings) must share at least one entry with
//     opts.Audiences; else ErrJwtAudienceNotAllowed. OR-semantic intersection
//     per §6.8.
//
// Silent-ignored fields per §1.1 amendment 3: RequireExpiration, MaxLifetime,
// Subjects — these are accepted at the API but never inspected here.
//
// Zero opts.Now uses time.Now() per the standard Go zero-value-is-default
// discipline.
func (t *Token) ValidateClaims(opts ValidateOptions) error {
	if t == nil || t.Payload == nil {
		// Defensive: a nil token / payload should not Validate.
		return ErrJwtVerificationFail
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	// (1) exp.
	if expRaw, ok := t.Payload["exp"]; ok {
		expUnix, ok := numericToInt64(expRaw)
		if !ok {
			// Malformed exp — treat as expired (mirrors Envoy semantic of
			// rejecting a present-but-unparseable timestamp).
			return ErrJwtExpired
		}
		exp := time.Unix(expUnix, 0)
		// exp + ClockSkew must be >= now → equivalently now <= exp + ClockSkew.
		if now.Add(-opts.ClockSkew).After(exp) {
			// now - ClockSkew > exp → expired
			return ErrJwtExpired
		}
	}

	// (2) nbf.
	if nbfRaw, ok := t.Payload["nbf"]; ok {
		nbfUnix, ok := numericToInt64(nbfRaw)
		if !ok {
			return ErrJwtNotYetValid
		}
		nbf := time.Unix(nbfUnix, 0)
		// nbf <= now + ClockSkew required.
		if nbf.After(now.Add(opts.ClockSkew)) {
			return ErrJwtNotYetValid
		}
	}

	// (3) iss.
	if opts.Issuer != "" {
		issRaw, ok := t.Payload["iss"]
		if !ok {
			return ErrJwtUnknownIssuer
		}
		iss, ok := issRaw.(string)
		if !ok || iss != opts.Issuer {
			return ErrJwtUnknownIssuer
		}
	}

	// (4) aud — OR-semantic intersection per §6.8.
	if len(opts.Audiences) > 0 {
		if !audIntersects(t.Payload["aud"], opts.Audiences) {
			return ErrJwtAudienceNotAllowed
		}
	}

	// Silent-ignored: RequireExpiration, MaxLifetime, Subjects.
	_ = opts.RequireExpiration
	_ = opts.MaxLifetime
	_ = opts.Subjects

	return nil
}

// numericToInt64 converts a JSON-decoded number to int64. JSON numeric values
// arrive as float64 from encoding/json; we tolerate int / int64 inputs too in
// case callers materialize claims manually.
func numericToInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

// audIntersects returns true iff the JWT aud claim (string OR []string) shares
// at least one entry with opts. Empty / missing aud claim returns false.
func audIntersects(audRaw interface{}, want []string) bool {
	if audRaw == nil {
		return false
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, w := range want {
		wantSet[w] = struct{}{}
	}
	switch a := audRaw.(type) {
	case string:
		_, ok := wantSet[a]
		return ok
	case []interface{}:
		for _, entry := range a {
			s, ok := entry.(string)
			if !ok {
				continue
			}
			if _, hit := wantSet[s]; hit {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// ----------------------------------------------------------------------------
// PayloadClaim: dot-notation extractor per §11.P10 + §1.1 amendment 10.
// ----------------------------------------------------------------------------

// PayloadClaim extracts a claim via dot-notation path (e.g., "user.email"
// traverses payload["user"]["email"]).
//
// Returns ErrClaimNotFound when:
//   - Any path segment is missing.
//   - Any intermediate segment resolves to a non-map (the path cannot
//     continue traversal).
//
// Returns ErrArrayClaim when the final value (or any intermediate that the
// path would index into) is a `[]interface{}` — array-valued claims are NOT
// supported per the JwtClaimToHeader.claim_name proto comment.
//
// Scalar leaf values (string, float64, bool, nil per JSON decoding) are
// returned verbatim as `interface{}`. Map leaf values are also rejected as
// ErrArrayClaim (the caller wants a scalar; returning a map would break the
// header-emission contract at Task 9).
func (t *Token) PayloadClaim(path string) (interface{}, error) {
	if t == nil || t.Payload == nil {
		return nil, ErrClaimNotFound
	}
	if path == "" {
		return nil, ErrClaimNotFound
	}
	segments := strings.Split(path, ".")
	var cur interface{} = t.Payload
	for i, seg := range segments {
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return nil, ErrClaimNotFound
		}
		next, ok := obj[seg]
		if !ok {
			return nil, ErrClaimNotFound
		}
		// On the last segment we may return a scalar (or a non-scalar leaf
		// which falls into the post-loop check below).
		_ = i
		cur = next
	}
	switch cur.(type) {
	case []interface{}:
		return nil, ErrArrayClaim
	case map[string]interface{}:
		// A map at the leaf is treated as "not a scalar" — same disposition
		// as array claims per the proto-comment "Array type claims are not
		// supported" (extended here to maps; the caller wants a scalar for
		// header emission).
		return nil, ErrArrayClaim
	default:
		return cur, nil
	}
}
