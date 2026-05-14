package inputs

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/esalaine/envoy-go/test/helpers/jwksbackend"
)

// signTokenRS256 produces a serialized JWT signed with RSA-PKCS1v15 + SHA-256
// (the RS256 alg). The header carries the supplied kid so the validator can
// select the matching JWK from the configured JWK Set. Mirrors the test
// helper at internal/jwt/jwt_test.go's signRS pattern; this fixture-local
// copy exists so the driver does not depend on internal/jwt test code.
func signTokenRS256(claims map[string]interface{}, key *rsa.PrivateKey, kid string) (string, error) {
	header := map[string]interface{}{"alg": "RS256", "typ": "JWT"}
	if kid != "" {
		header["kid"] = kid
	}
	h, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal header: %w", err)
	}
	p, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	encH := base64.RawURLEncoding.EncodeToString(h)
	encP := base64.RawURLEncoding.EncodeToString(p)
	signed := encH + "." + encP

	sum := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("rsa.SignPKCS1v15: %w", err)
	}
	encS := base64.RawURLEncoding.EncodeToString(sig)
	return signed + "." + encS, nil
}

// signTokenES256 produces a serialized JWT signed with ECDSA over the P-256
// curve + SHA-256 (the ES256 alg). The signature is the raw r||s pair padded
// to the curve byte size (32 bytes each → 64 bytes total) per JWS §A. The
// header carries the supplied kid for JWK Set selection. Mirrors the test
// helper at internal/jwt/jwt_test.go's signES pattern.
func signTokenES256(claims map[string]interface{}, key *ecdsa.PrivateKey, kid string) (string, error) {
	header := map[string]interface{}{"alg": "ES256", "typ": "JWT"}
	if kid != "" {
		header["kid"] = kid
	}
	h, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal header: %w", err)
	}
	p, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	encH := base64.RawURLEncoding.EncodeToString(h)
	encP := base64.RawURLEncoding.EncodeToString(p)
	signed := encH + "." + encP

	sum := sha256.Sum256([]byte(signed))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return "", fmt.Errorf("ecdsa.Sign: %w", err)
	}
	// Pad r and s to curve byte size and concatenate per JWS §A.
	byteSize := (key.Curve.Params().BitSize + 7) / 8
	sig := make([]byte, 2*byteSize)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[byteSize-len(rBytes):byteSize], rBytes)
	copy(sig[2*byteSize-len(sBytes):], sBytes)
	encS := base64.RawURLEncoding.EncodeToString(sig)
	return signed + "." + encS, nil
}

// jwksbackendNewImpl wires the driver-side indirection to the shared
// jwksbackend test helper. Returns the helper's *Server as the minimal
// Stop()-returning interface the driver expects.
func jwksbackendNewImpl(ctx context.Context, addr string, routes map[string]string) (jwksbackendStarter, error) {
	return jwksbackend.New(ctx, addr, routes)
}
