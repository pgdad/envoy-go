// Package pki provides RSA + ECDSA keypair generation for the
// fixture-0019-http-jwt-authn differential fixture.
//
// Per planner-time decision 11 + SPEC §7.3: the keys are generated FRESH at
// fixture-load time (NOT pre-baked / committed to the repo) — the keypairs
// + the derived JWK Set JSON live only for the duration of the test process.
//
// Generated artifacts (per SPEC §7.3 + the driver's fixturePKI shape at
// inputs/driver.go):
//
//   - 3 RSA-2048 keypairs:
//   - RS256Key1 / RS256Kid1: provider-rs256 (RemoteJwks via jwksbackend at
//     /.well-known/jwks-rs.json). Scenarios 1, 4 sign with this key. Scenario
//     5 signs with TamperedKey but claims this kid in the JWS header so the
//     validator selects the matching JWK by kid lookup; signature
//     verification then fails because the signing key does not match the
//     public modulus.
//   - RS256Key2 / RS256Kid2: spare keypair reserved by SPEC §7.3 (a 3rd RSA-
//     2048 keypair); currently not consumed by any scenario but added to the
//     provider-rs256 JWK Set so the multi-key lookup path is exercised at
//     runtime.
//   - RS256Key3 / AltKid: provider-alt (RemoteJwks via jwksbackend at
//     /.well-known/jwks-alt.json). Scenario 7 (per-route 8th-canonical
//     requirement_name delegation) signs with this key.
//   - 1 ECDSA-P256 keypair:
//   - ES256Key / ES256Kid: provider-es256 (LocalJwks inline_string). Scenario
//     2 signs with this key. No JWKS fetch involved (LocalJwks path).
//   - 1 RSA-2048 "tampered" keypair:
//   - TamperedKey: used by scenario 5 to sign a JWT whose JWS header kid =
//     RS256Kid1 (so the validator selects the matching JWK by kid lookup)
//     but whose private key does NOT match the public modulus in the JWK Set →
//     signature verification fails → 401 + `Jwt verification fails`.
//
// JWK Set serialization (RFC 7517 §5 — minimal RSA + EC member set):
//   - RS256JWKSet contains RS256Key1 + RS256Key2 public keys (provider-rs256's
//     JWK Set body served at /.well-known/jwks-rs.json).
//   - AltProviderJWKSet contains RS256Key3 public key (provider-alt's JWK Set
//     body served at /.well-known/jwks-alt.json).
//   - ES256JWKSet contains ES256Key public key (provider-es256's LocalJwks
//     inline_string content; substituted into the envoy.yaml + envoy-go.yaml
//     templates at driver bootstrap-render time).
//
// All RSA modulus + ECDSA coordinate bytes are base64url-encoded (RFC 4648 §5
// "without padding" form per RFC 7518 §6.3.1.1 + §6.2.1.2). ECDSA P-256 x + y
// coordinates are padded with leading zeros to the curve byte size (32 bytes)
// per JWS §A so the JWK serialization is byte-stable across keypairs.
//
// Stdlib-only: crypto/rsa + crypto/ecdsa + crypto/elliptic + crypto/rand +
// encoding/base64 + encoding/json + math/big (no third-party JWT/JWK libraries
// per ADR-0151 framework-primitive discipline + phase-16 pki/gen.go precedent).
package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
)

// FixturePKI carries all generated keypairs + serialized JWK Set JSON for
// fixture 0019. The field shape matches the driver's `fixturePKI` struct at
// inputs/driver.go — the driver constructs its `fixturePKI` from a *FixturePKI
// via field-by-field copy at loadFixturePKI().
type FixturePKI struct {
	// Private keys for token signing.
	RS256Key1   *rsa.PrivateKey   // provider-rs256 (RemoteJwks)
	RS256Key2   *rsa.PrivateKey   // spare RSA-2048 keypair reserved by SPEC §7.3
	RS256Key3   *rsa.PrivateKey   // provider-alt (RemoteJwks; per-route scenario 7)
	ES256Key    *ecdsa.PrivateKey // provider-es256 (LocalJwks)
	TamperedKey *rsa.PrivateKey   // scenario 5 bad-signature

	// kids for the JWS header `kid` field. Matches the JWK Set entry kids so
	// the validator's kid-lookup path selects the right JWK at verification
	// time.
	RS256Kid1 string
	RS256Kid2 string
	AltKid    string // kid for RS256Key3 (provider-alt's JWK Set entry)
	ES256Kid  string

	// JWK Set JSON serializations.
	RS256JWKSet       string // provider-rs256 — served at /.well-known/jwks-rs.json
	AltProviderJWKSet string // provider-alt   — served at /.well-known/jwks-alt.json
	ES256JWKSet       string // provider-es256 — LocalJwks inline_string content
}

// Generate produces a fresh FixturePKI. Idempotent at the caller (the driver
// memoizes via sync.Once so the same instance is reused across
// ReferenceBootstrap + SubjectConfig + every scenario invocation).
//
// Wallclock cost: ~250-500ms typical (4 RSA-2048 keygens + 1 ECDSA-P256). The
// driver's sync.Once memoization means this fires once per `go test` process.
func Generate() (*FixturePKI, error) {
	rsa1, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RS256Key1: %w", err)
	}
	rsa2, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RS256Key2: %w", err)
	}
	rsa3, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RS256Key3: %w", err)
	}
	tampered, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate TamperedKey: %w", err)
	}
	es256, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ES256Key: %w", err)
	}

	p := &FixturePKI{
		RS256Key1: rsa1, RS256Kid1: "rs256-1",
		RS256Key2: rsa2, RS256Kid2: "rs256-2",
		RS256Key3: rsa3, AltKid: "alt-1",
		ES256Key: es256, ES256Kid: "es256-1",
		TamperedKey: tampered,
	}

	rsJWKS, err := marshalRSAJWKSet([]rsaKey{
		{key: rsa1, kid: p.RS256Kid1},
		{key: rsa2, kid: p.RS256Kid2},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal RS256JWKSet: %w", err)
	}
	p.RS256JWKSet = rsJWKS

	altJWKS, err := marshalRSAJWKSet([]rsaKey{
		{key: rsa3, kid: p.AltKid},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal AltProviderJWKSet: %w", err)
	}
	p.AltProviderJWKSet = altJWKS

	esJWKS, err := marshalECDSAJWKSet([]ecdsaKey{
		{key: es256, kid: p.ES256Kid},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal ES256JWKSet: %w", err)
	}
	p.ES256JWKSet = esJWKS

	return p, nil
}

type rsaKey struct {
	key *rsa.PrivateKey
	kid string
}

type ecdsaKey struct {
	key *ecdsa.PrivateKey
	kid string
}

// jwkRSA mirrors the RFC 7517 §4 + RFC 7518 §6.3.1 JSON representation for an
// RSA signature-public-key. Field order in JSON output is the struct order
// here (encoding/json honors declared order).
type jwkRSA struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// jwkEC mirrors the RFC 7517 §4 + RFC 7518 §6.2.1 JSON representation for an
// EC signature-public-key.
type jwkEC struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type jwkSet struct {
	Keys []any `json:"keys"`
}

// marshalRSAJWKSet serializes RSA public keys as RFC 7517 §5 JWK Set JSON.
func marshalRSAJWKSet(keys []rsaKey) (string, error) {
	set := jwkSet{Keys: make([]any, 0, len(keys))}
	for _, k := range keys {
		nBytes := k.key.N.Bytes()
		eBytes := big.NewInt(int64(k.key.E)).Bytes()
		set.Keys = append(set.Keys, jwkRSA{
			Kty: "RSA",
			Kid: k.kid,
			Alg: "RS256",
			Use: "sig",
			N:   base64.RawURLEncoding.EncodeToString(nBytes),
			E:   base64.RawURLEncoding.EncodeToString(eBytes),
		})
	}
	raw, err := json.Marshal(set)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// marshalECDSAJWKSet serializes EC public keys as RFC 7517 §5 JWK Set JSON.
// P-256 coordinates are 32 bytes each (curve byte size); leading zero bytes
// are preserved via padLeft so the base64url-encoded form is byte-stable
// across keypairs.
func marshalECDSAJWKSet(keys []ecdsaKey) (string, error) {
	set := jwkSet{Keys: make([]any, 0, len(keys))}
	for _, k := range keys {
		byteSize := (k.key.Curve.Params().BitSize + 7) / 8
		x := padLeft(k.key.X.Bytes(), byteSize)
		y := padLeft(k.key.Y.Bytes(), byteSize)
		crv := k.key.Curve.Params().Name // "P-256" for elliptic.P256()
		set.Keys = append(set.Keys, jwkEC{
			Kty: "EC",
			Kid: k.kid,
			Alg: "ES256",
			Use: "sig",
			Crv: crv,
			X:   base64.RawURLEncoding.EncodeToString(x),
			Y:   base64.RawURLEncoding.EncodeToString(y),
		})
	}
	raw, err := json.Marshal(set)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// padLeft returns b padded with leading zero bytes up to size. If len(b) >=
// size, returns b unchanged. Used to normalize ECDSA coordinate widths to the
// curve byte size per JWS §A.
func padLeft(b []byte, size int) []byte {
	if len(b) >= size {
		return b
	}
	padded := make([]byte, size)
	copy(padded[size-len(b):], b)
	return padded
}
