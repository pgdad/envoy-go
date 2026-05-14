package jwt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"
)

// ----------------------------------------------------------------------------
// Test helpers: per-package fresh keys (initialized once per `go test` run).
// ----------------------------------------------------------------------------

var (
	keyOnce sync.Once

	rsaKey   *rsa.PrivateKey
	es256Key *ecdsa.PrivateKey
	es384Key *ecdsa.PrivateKey
	es521Key *ecdsa.PrivateKey
)

func initKeys(t *testing.T) {
	t.Helper()
	keyOnce.Do(func() {
		var err error
		rsaKey, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("rsa.GenerateKey: %v", err)
		}
		es256Key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("ecdsa.GenerateKey P-256: %v", err)
		}
		es384Key, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			t.Fatalf("ecdsa.GenerateKey P-384: %v", err)
		}
		es521Key, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
		if err != nil {
			t.Fatalf("ecdsa.GenerateKey P-521: %v", err)
		}
	})
}

// signRS produces a serialized JWT signed with rsa-pkcs1v15 using the alg-mapped hash.
func signRS(t *testing.T, alg, kid string, claims map[string]interface{}, key *rsa.PrivateKey) string {
	t.Helper()
	header := map[string]interface{}{"alg": alg, "typ": "JWT"}
	if kid != "" {
		header["kid"] = kid
	}
	h, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	p, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	encH := base64.RawURLEncoding.EncodeToString(h)
	encP := base64.RawURLEncoding.EncodeToString(p)
	signed := encH + "." + encP

	var hashed []byte
	var hashAlg crypto.Hash
	switch alg {
	case "RS256":
		sum := sha256.Sum256([]byte(signed))
		hashed = sum[:]
		hashAlg = crypto.SHA256
	case "RS384":
		sum := sha512.Sum384([]byte(signed))
		hashed = sum[:]
		hashAlg = crypto.SHA384
	case "RS512":
		sum := sha512.Sum512([]byte(signed))
		hashed = sum[:]
		hashAlg = crypto.SHA512
	default:
		t.Fatalf("signRS: unknown alg %q", alg)
	}
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, hashAlg, hashed)
	if err != nil {
		t.Fatalf("rsa.SignPKCS1v15: %v", err)
	}
	encS := base64.RawURLEncoding.EncodeToString(sig)
	return signed + "." + encS
}

// signES produces a JWT signed with ECDSA + the alg-mapped hash.
func signES(t *testing.T, alg, kid string, claims map[string]interface{}, key *ecdsa.PrivateKey) string {
	t.Helper()
	header := map[string]interface{}{"alg": alg, "typ": "JWT"}
	if kid != "" {
		header["kid"] = kid
	}
	h, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	p, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	encH := base64.RawURLEncoding.EncodeToString(h)
	encP := base64.RawURLEncoding.EncodeToString(p)
	signed := encH + "." + encP

	var hashed []byte
	switch alg {
	case "ES256":
		sum := sha256.Sum256([]byte(signed))
		hashed = sum[:]
	case "ES384":
		sum := sha512.Sum384([]byte(signed))
		hashed = sum[:]
	case "ES512":
		sum := sha512.Sum512([]byte(signed))
		hashed = sum[:]
	default:
		t.Fatalf("signES: unknown alg %q", alg)
	}
	r, s, err := ecdsa.Sign(rand.Reader, key, hashed)
	if err != nil {
		t.Fatalf("ecdsa.Sign: %v", err)
	}
	// Pad r and s to curve byte size and concatenate per JWS §A.
	byteSize := (key.Curve.Params().BitSize + 7) / 8
	sig := make([]byte, 2*byteSize)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[byteSize-len(rBytes):byteSize], rBytes)
	copy(sig[2*byteSize-len(sBytes):], sBytes)
	encS := base64.RawURLEncoding.EncodeToString(sig)
	return signed + "." + encS
}

// ----------------------------------------------------------------------------
// Parse tests (~8).
// ----------------------------------------------------------------------------

func TestParse_ValidJWT_Success(t *testing.T) {
	initKeys(t)
	tok := signRS(t, "RS256", "key-1", map[string]interface{}{
		"iss": "https://example.com",
		"sub": "alice",
	}, rsaKey)
	got, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Alg != "RS256" {
		t.Errorf("Alg = %q; want RS256", got.Alg)
	}
	if got.Kid != "key-1" {
		t.Errorf("Kid = %q; want key-1", got.Kid)
	}
	if got.Payload["iss"] != "https://example.com" {
		t.Errorf("payload iss = %v; want https://example.com", got.Payload["iss"])
	}
	if got.Payload["sub"] != "alice" {
		t.Errorf("payload sub = %v; want alice", got.Payload["sub"])
	}
	if got.RawHeader == "" || got.RawPayload == "" || got.RawSignature == "" {
		t.Errorf("Raw fields must be populated; got header=%q payload=%q sig=%q",
			got.RawHeader, got.RawPayload, got.RawSignature)
	}
}

func TestParse_NotThreeParts_ErrJwtBadFormat(t *testing.T) {
	cases := []string{
		"",
		"oneonly",
		"two.parts",
		"a.b.c.d",
		"a.b.c.d.e",
	}
	for _, in := range cases {
		_, err := Parse(in)
		if err == nil {
			t.Errorf("Parse(%q): want error, got nil", in)
			continue
		}
		if !errors.Is(err, ErrJwtBadFormat) {
			t.Errorf("Parse(%q): got %v; want ErrJwtBadFormat", in, err)
		}
	}
}

func TestParse_HeaderInvalidBase64_ErrJwtHeaderParseErrorBadBase64(t *testing.T) {
	// '%' is not a valid base64url character.
	bad := "%%%.eyJzdWIiOiJhIn0.sig"
	_, err := Parse(bad)
	if !errors.Is(err, ErrJwtHeaderParseErrorBadBase64) {
		t.Errorf("got %v; want ErrJwtHeaderParseErrorBadBase64", err)
	}
}

func TestParse_PayloadInvalidBase64_ErrJwtPayloadParseErrorBadBase64(t *testing.T) {
	encH := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	bad := encH + ".%%%.sig"
	_, err := Parse(bad)
	if !errors.Is(err, ErrJwtPayloadParseErrorBadBase64) {
		t.Errorf("got %v; want ErrJwtPayloadParseErrorBadBase64", err)
	}
}

func TestParse_SignatureInvalidBase64_ErrJwtSignatureParseErrorBadBase64(t *testing.T) {
	encH := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	encP := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"a"}`))
	bad := encH + "." + encP + ".%%%"
	_, err := Parse(bad)
	if !errors.Is(err, ErrJwtSignatureParseErrorBadBase64) {
		t.Errorf("got %v; want ErrJwtSignatureParseErrorBadBase64", err)
	}
}

func TestParse_HeaderInvalidJSON_ErrJwtHeaderParseErrorBadJson(t *testing.T) {
	encH := base64.RawURLEncoding.EncodeToString([]byte(`{not json`))
	encP := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"a"}`))
	encS := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	bad := encH + "." + encP + "." + encS
	_, err := Parse(bad)
	if !errors.Is(err, ErrJwtHeaderParseErrorBadJson) {
		t.Errorf("got %v; want ErrJwtHeaderParseErrorBadJson", err)
	}
}

func TestParse_PayloadInvalidJSON_ErrJwtPayloadParseErrorBadJson(t *testing.T) {
	encH := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	encP := base64.RawURLEncoding.EncodeToString([]byte(`{not json`))
	encS := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	bad := encH + "." + encP + "." + encS
	_, err := Parse(bad)
	if !errors.Is(err, ErrJwtPayloadParseErrorBadJson) {
		t.Errorf("got %v; want ErrJwtPayloadParseErrorBadJson", err)
	}
}

func TestParse_AlgFieldMissing_ErrJwtHeaderBadAlg(t *testing.T) {
	// No alg in header.
	encH := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
	encP := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"a"}`))
	encS := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	bad := encH + "." + encP + "." + encS
	_, err := Parse(bad)
	if !errors.Is(err, ErrJwtHeaderBadAlg) {
		t.Errorf("missing alg: got %v; want ErrJwtHeaderBadAlg", err)
	}

	// Non-string alg.
	encH2 := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":42,"typ":"JWT"}`))
	bad2 := encH2 + "." + encP + "." + encS
	_, err = Parse(bad2)
	if !errors.Is(err, ErrJwtHeaderBadAlg) {
		t.Errorf("non-string alg: got %v; want ErrJwtHeaderBadAlg", err)
	}
}

// ----------------------------------------------------------------------------
// VerifySignature tests (~10).
// ----------------------------------------------------------------------------

func TestVerifySignature_RS256_Success(t *testing.T) {
	initKeys(t)
	tok := signRS(t, "RS256", "", map[string]interface{}{"sub": "alice"}, rsaKey)
	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := parsed.VerifySignature(&rsaKey.PublicKey, "RS256"); err != nil {
		t.Errorf("VerifySignature: %v", err)
	}
}

func TestVerifySignature_RS384_Success(t *testing.T) {
	initKeys(t)
	tok := signRS(t, "RS384", "", map[string]interface{}{"sub": "alice"}, rsaKey)
	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := parsed.VerifySignature(&rsaKey.PublicKey, "RS384"); err != nil {
		t.Errorf("VerifySignature: %v", err)
	}
}

func TestVerifySignature_RS512_Success(t *testing.T) {
	initKeys(t)
	tok := signRS(t, "RS512", "", map[string]interface{}{"sub": "alice"}, rsaKey)
	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := parsed.VerifySignature(&rsaKey.PublicKey, "RS512"); err != nil {
		t.Errorf("VerifySignature: %v", err)
	}
}

func TestVerifySignature_ES256_Success(t *testing.T) {
	initKeys(t)
	tok := signES(t, "ES256", "", map[string]interface{}{"sub": "alice"}, es256Key)
	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := parsed.VerifySignature(&es256Key.PublicKey, "ES256"); err != nil {
		t.Errorf("VerifySignature: %v", err)
	}
}

func TestVerifySignature_ES384_Success(t *testing.T) {
	initKeys(t)
	tok := signES(t, "ES384", "", map[string]interface{}{"sub": "alice"}, es384Key)
	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := parsed.VerifySignature(&es384Key.PublicKey, "ES384"); err != nil {
		t.Errorf("VerifySignature: %v", err)
	}
}

func TestVerifySignature_ES512_Success(t *testing.T) {
	initKeys(t)
	tok := signES(t, "ES512", "", map[string]interface{}{"sub": "alice"}, es521Key)
	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := parsed.VerifySignature(&es521Key.PublicKey, "ES512"); err != nil {
		t.Errorf("VerifySignature: %v", err)
	}
}

func TestVerifySignature_TamperedSignature_ErrJwtVerificationFail(t *testing.T) {
	initKeys(t)
	tok := signRS(t, "RS256", "", map[string]interface{}{"sub": "alice"}, rsaKey)
	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Flip a byte in the signature.
	sig, err := base64.RawURLEncoding.DecodeString(parsed.RawSignature)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	sig[0] ^= 0xff
	parsed.RawSignature = base64.RawURLEncoding.EncodeToString(sig)
	if err := parsed.VerifySignature(&rsaKey.PublicKey, "RS256"); !errors.Is(err, ErrJwtVerificationFail) {
		t.Errorf("got %v; want ErrJwtVerificationFail", err)
	}
}

func TestVerifySignature_AlgOutsideAllowList_ErrJwtHeaderNotImplementedAlg(t *testing.T) {
	initKeys(t)
	// Construct a token with alg=HS256 (outside RS+ES allow-list).
	encH := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	encP := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"a"}`))
	encS := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	tok := encH + "." + encP + "." + encS

	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, alg := range []string{"HS256", "EdDSA", "none", "PS256", "PS384", "PS512", "RS128"} {
		parsed.Alg = alg
		err := parsed.VerifySignature(&rsaKey.PublicKey, alg)
		if !errors.Is(err, ErrJwtHeaderNotImplementedAlg) {
			t.Errorf("alg=%q: got %v; want ErrJwtHeaderNotImplementedAlg", alg, err)
		}
	}
}

func TestVerifySignature_WrongKeyTypeForAlg_ErrJwtVerificationFail(t *testing.T) {
	initKeys(t)
	// RSA alg + ECDSA key.
	tok := signRS(t, "RS256", "", map[string]interface{}{"sub": "alice"}, rsaKey)
	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := parsed.VerifySignature(&es256Key.PublicKey, "RS256"); !errors.Is(err, ErrJwtVerificationFail) {
		t.Errorf("RS256 with EC key: got %v; want ErrJwtVerificationFail", err)
	}

	// ECDSA alg + RSA key.
	tok2 := signES(t, "ES256", "", map[string]interface{}{"sub": "alice"}, es256Key)
	parsed2, err := Parse(tok2)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := parsed2.VerifySignature(&rsaKey.PublicKey, "ES256"); !errors.Is(err, ErrJwtVerificationFail) {
		t.Errorf("ES256 with RSA key: got %v; want ErrJwtVerificationFail", err)
	}
}

func TestVerifySignature_ES256_WrongSignatureLength_ErrJwtVerificationFail(t *testing.T) {
	initKeys(t)
	tok := signES(t, "ES256", "", map[string]interface{}{"sub": "alice"}, es256Key)
	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Truncate signature.
	sig, err := base64.RawURLEncoding.DecodeString(parsed.RawSignature)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	parsed.RawSignature = base64.RawURLEncoding.EncodeToString(sig[:len(sig)-4])
	if err := parsed.VerifySignature(&es256Key.PublicKey, "ES256"); !errors.Is(err, ErrJwtVerificationFail) {
		t.Errorf("got %v; want ErrJwtVerificationFail", err)
	}
}

// ----------------------------------------------------------------------------
// ValidateClaims tests (~12-13).
// ----------------------------------------------------------------------------

func parseClaimsToken(t *testing.T, claims map[string]interface{}) *Token {
	t.Helper()
	initKeys(t)
	tok := signRS(t, "RS256", "", claims, rsaKey)
	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return parsed
}

func TestValidateClaims_ExpInFuture_Success(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := parseClaimsToken(t, map[string]interface{}{
		"exp": now.Add(time.Hour).Unix(),
	})
	err := tok.ValidateClaims(ValidateOptions{Now: now, ClockSkew: 60 * time.Second})
	if err != nil {
		t.Errorf("got %v; want nil", err)
	}
}

func TestValidateClaims_ExpInPast_ErrJwtExpired(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := parseClaimsToken(t, map[string]interface{}{
		"exp": now.Add(-time.Hour).Unix(),
	})
	err := tok.ValidateClaims(ValidateOptions{Now: now, ClockSkew: 60 * time.Second})
	if !errors.Is(err, ErrJwtExpired) {
		t.Errorf("got %v; want ErrJwtExpired", err)
	}
}

func TestValidateClaims_ExpWithinClockSkew_Success(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// exp 30s in the past, ClockSkew=60s → acceptance window covers it.
	tok := parseClaimsToken(t, map[string]interface{}{
		"exp": now.Add(-30 * time.Second).Unix(),
	})
	err := tok.ValidateClaims(ValidateOptions{Now: now, ClockSkew: 60 * time.Second})
	if err != nil {
		t.Errorf("got %v; want nil", err)
	}
}

func TestValidateClaims_NbfInPast_Success(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := parseClaimsToken(t, map[string]interface{}{
		"nbf": now.Add(-time.Hour).Unix(),
	})
	err := tok.ValidateClaims(ValidateOptions{Now: now, ClockSkew: 60 * time.Second})
	if err != nil {
		t.Errorf("got %v; want nil", err)
	}
}

func TestValidateClaims_NbfInFuture_ErrJwtNotYetValid(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := parseClaimsToken(t, map[string]interface{}{
		"nbf": now.Add(time.Hour).Unix(),
	})
	err := tok.ValidateClaims(ValidateOptions{Now: now, ClockSkew: 60 * time.Second})
	if !errors.Is(err, ErrJwtNotYetValid) {
		t.Errorf("got %v; want ErrJwtNotYetValid", err)
	}
}

func TestValidateClaims_NbfWithinClockSkew_Success(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// nbf 30s in the future, ClockSkew=60s → acceptance window covers it.
	tok := parseClaimsToken(t, map[string]interface{}{
		"nbf": now.Add(30 * time.Second).Unix(),
	})
	err := tok.ValidateClaims(ValidateOptions{Now: now, ClockSkew: 60 * time.Second})
	if err != nil {
		t.Errorf("got %v; want nil", err)
	}
}

func TestValidateClaims_IssuerExact_Success(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := parseClaimsToken(t, map[string]interface{}{
		"iss": "https://example.com",
	})
	err := tok.ValidateClaims(ValidateOptions{Now: now, Issuer: "https://example.com"})
	if err != nil {
		t.Errorf("got %v; want nil", err)
	}
}

func TestValidateClaims_IssuerEmpty_SkipsCheck(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Token has no iss; opts.Issuer="" → skip.
	tok := parseClaimsToken(t, map[string]interface{}{})
	err := tok.ValidateClaims(ValidateOptions{Now: now, Issuer: ""})
	if err != nil {
		t.Errorf("got %v; want nil", err)
	}
}

func TestValidateClaims_IssuerMismatch_ErrJwtUnknownIssuer(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := parseClaimsToken(t, map[string]interface{}{
		"iss": "https://other.example.com",
	})
	err := tok.ValidateClaims(ValidateOptions{Now: now, Issuer: "https://example.com"})
	if !errors.Is(err, ErrJwtUnknownIssuer) {
		t.Errorf("got %v; want ErrJwtUnknownIssuer", err)
	}
}

func TestValidateClaims_AudienceIntersection_String_Success(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := parseClaimsToken(t, map[string]interface{}{
		"aud": "service-x",
	})
	err := tok.ValidateClaims(ValidateOptions{Now: now, Audiences: []string{"service-x", "service-y"}})
	if err != nil {
		t.Errorf("got %v; want nil", err)
	}
}

func TestValidateClaims_AudienceIntersection_Array_Success(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := parseClaimsToken(t, map[string]interface{}{
		"aud": []interface{}{"service-x", "service-z"},
	})
	err := tok.ValidateClaims(ValidateOptions{Now: now, Audiences: []string{"service-y", "service-z"}})
	if err != nil {
		t.Errorf("got %v; want nil", err)
	}
}

func TestValidateClaims_AudienceEmpty_SkipsCheck(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := parseClaimsToken(t, map[string]interface{}{})
	// No Audiences option → no aud-check; should succeed.
	err := tok.ValidateClaims(ValidateOptions{Now: now})
	if err != nil {
		t.Errorf("got %v; want nil", err)
	}
}

func TestValidateClaims_AudienceMismatch_ErrJwtAudienceNotAllowed(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := parseClaimsToken(t, map[string]interface{}{
		"aud": []interface{}{"service-a", "service-b"},
	})
	err := tok.ValidateClaims(ValidateOptions{Now: now, Audiences: []string{"service-x"}})
	if !errors.Is(err, ErrJwtAudienceNotAllowed) {
		t.Errorf("got %v; want ErrJwtAudienceNotAllowed", err)
	}
}

// ----------------------------------------------------------------------------
// PayloadClaim tests (~6).
// ----------------------------------------------------------------------------

func TestPayloadClaim_TopLevelString_Success(t *testing.T) {
	tok := parseClaimsToken(t, map[string]interface{}{
		"sub": "alice",
	})
	got, err := tok.PayloadClaim("sub")
	if err != nil {
		t.Fatalf("PayloadClaim: %v", err)
	}
	if got != "alice" {
		t.Errorf("got %v; want alice", got)
	}
}

func TestPayloadClaim_NestedDotNotation_Success(t *testing.T) {
	tok := parseClaimsToken(t, map[string]interface{}{
		"user": map[string]interface{}{
			"email": "alice@example.com",
		},
	})
	got, err := tok.PayloadClaim("user.email")
	if err != nil {
		t.Fatalf("PayloadClaim: %v", err)
	}
	if got != "alice@example.com" {
		t.Errorf("got %v; want alice@example.com", got)
	}
}

func TestPayloadClaim_ScalarTypes_AllSupported(t *testing.T) {
	tok := parseClaimsToken(t, map[string]interface{}{
		"s": "string",
		"i": 42,
		"b": true,
		"n": nil,
	})
	type expect struct {
		path string
		want interface{}
	}
	cases := []expect{
		{"s", "string"},
		{"i", float64(42)}, // JSON unmarshals numbers to float64
		{"b", true},
		{"n", nil},
	}
	for _, c := range cases {
		got, err := tok.PayloadClaim(c.path)
		if err != nil {
			t.Errorf("%s: %v", c.path, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %v (%T); want %v (%T)", c.path, got, got, c.want, c.want)
		}
	}
}

func TestPayloadClaim_ArrayClaim_ErrArrayClaim(t *testing.T) {
	tok := parseClaimsToken(t, map[string]interface{}{
		"groups": []interface{}{"admin", "ops"},
	})
	_, err := tok.PayloadClaim("groups")
	if !errors.Is(err, ErrArrayClaim) {
		t.Errorf("got %v; want ErrArrayClaim", err)
	}
}

func TestPayloadClaim_MissingPath_ErrClaimNotFound(t *testing.T) {
	tok := parseClaimsToken(t, map[string]interface{}{
		"sub": "alice",
	})
	_, err := tok.PayloadClaim("nope")
	if !errors.Is(err, ErrClaimNotFound) {
		t.Errorf("got %v; want ErrClaimNotFound", err)
	}
	_, err = tok.PayloadClaim("a.b.c")
	if !errors.Is(err, ErrClaimNotFound) {
		t.Errorf("nested miss: got %v; want ErrClaimNotFound", err)
	}
}

func TestPayloadClaim_NestedNonObject_ErrClaimNotFound(t *testing.T) {
	tok := parseClaimsToken(t, map[string]interface{}{
		"user": "alice",
	})
	// "user.email" but user is a string, not an object.
	_, err := tok.PayloadClaim("user.email")
	if !errors.Is(err, ErrClaimNotFound) {
		t.Errorf("got %v; want ErrClaimNotFound", err)
	}
}

// ----------------------------------------------------------------------------
// Silent-ignore verification (~3) — fields accepted at the API but never
// enforced per §1.1 amendment 3.
// ----------------------------------------------------------------------------

func TestValidateOptions_RequireExpiration_SilentIgnored(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Token has NO exp claim; opts.RequireExpiration=true should be silent-ignored.
	tok := parseClaimsToken(t, map[string]interface{}{})
	err := tok.ValidateClaims(ValidateOptions{
		Now:               now,
		RequireExpiration: true,
	})
	if err != nil {
		t.Errorf("RequireExpiration silent-ignored: got %v; want nil", err)
	}
}

func TestValidateOptions_MaxLifetime_SilentIgnored(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Token with exp far in the future; MaxLifetime=1s should be silent-ignored.
	tok := parseClaimsToken(t, map[string]interface{}{
		"iat": now.Unix(),
		"exp": now.Add(100 * 365 * 24 * time.Hour).Unix(),
	})
	err := tok.ValidateClaims(ValidateOptions{
		Now:         now,
		MaxLifetime: 1 * time.Second,
	})
	if err != nil {
		t.Errorf("MaxLifetime silent-ignored: got %v; want nil", err)
	}
}

func TestValidateOptions_Subjects_SilentIgnored(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := parseClaimsToken(t, map[string]interface{}{
		"sub": "alice",
	})
	// Subjects matcher requires "bob" exact; should be silent-ignored.
	err := tok.ValidateClaims(ValidateOptions{
		Now:      now,
		Subjects: &StringMatcher{Exact: "bob"},
	})
	if err != nil {
		t.Errorf("Subjects silent-ignored: got %v; want nil", err)
	}
}

// ----------------------------------------------------------------------------
// Error-string byte-exact verification per §11.P1 (deny-response body content
// at Task 9 — error strings MUST match jwt_verify_lib getStatusString output).
// ----------------------------------------------------------------------------

func TestErrSentinelsByteExact(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ErrJwtMissed, "Jwt is missing"},
		{ErrJwtBadFormat, "Jwt is not in the form of Header.Payload.Signature"},
		{ErrJwtHeaderParseErrorBadBase64, "Jwt header is an invalid Base64url encoded."},
		{ErrJwtHeaderParseErrorBadJson, "Jwt header is an invalid JSON."},
		{ErrJwtPayloadParseErrorBadBase64, "Jwt payload is an invalid Base64url encoded."},
		{ErrJwtPayloadParseErrorBadJson, "Jwt payload is an invalid JSON."},
		{ErrJwtSignatureParseErrorBadBase64, "Jwt signature is an invalid Base64url encoded."},
		{ErrJwtHeaderBadAlg, "Jwt header [alg] field is required."},
		{ErrJwtHeaderNotImplementedAlg, "Jwt header [alg] is not supported"},
		{ErrJwtVerificationFail, "Jwt verification fails"},
		{ErrJwtExpired, "Jwt is expired"},
		{ErrJwtNotYetValid, "Jwt not yet valid"},
		{ErrJwtUnknownIssuer, "Jwt issuer is not configured"},
		{ErrJwtAudienceNotAllowed, "Audiences in Jwt are not allowed"},
	}
	for _, c := range cases {
		if c.err.Error() != c.want {
			t.Errorf("%T.Error() = %q; want %q", c.err, c.err.Error(), c.want)
		}
	}
	// Spot-check the specific byte-counts called out in PLAN Task 9.
	exactLens := map[error]int{
		ErrJwtMissed:                  14,
		ErrJwtExpired:                 14,
		ErrJwtVerificationFail:        22,
		ErrJwtUnknownIssuer:           28,
		ErrJwtAudienceNotAllowed:      32,
		ErrJwtNotYetValid:             17,
		ErrJwtHeaderNotImplementedAlg: 33,
	}
	for err, n := range exactLens {
		if got := len(err.Error()); got != n {
			t.Errorf("%v: byte-count %d; want %d", err, got, n)
		}
	}
}

// ----------------------------------------------------------------------------
// Smoke: cross-reference that the small helper compiles + sane.
// ----------------------------------------------------------------------------

func TestSignedDataIsBase64UrlNotRaw(t *testing.T) {
	// Quick sanity that we sign the base64url-encoded header.payload, not the
	// raw decoded bytes — the test catches an implementation slip on
	// signature semantics.
	initKeys(t)
	tok := signRS(t, "RS256", "", map[string]interface{}{"sub": "alice"}, rsaKey)
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("split: %d parts", len(parts))
	}
	parsed, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// VerifySignature must accept the base64url-encoded signed-data form.
	if err := parsed.VerifySignature(&rsaKey.PublicKey, "RS256"); err != nil {
		t.Errorf("VerifySignature: %v", err)
	}
}

// Compile-time sanity that the math/big import is used (in ECDSA verify path).
var _ = (*big.Int)(nil)
