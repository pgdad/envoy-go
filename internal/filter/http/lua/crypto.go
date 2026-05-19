package lua

// crypto.go — Task 12 (phase 22.2 IMPL) crypto bridge per SPEC §3.4 +
// §6 arm-22 + §11.2 + AMEND-22.2-1 + AMEND-22.2-2 + D8 closure + PLAN
// Task 12 + ADR-0192 §Decision body anticipation.
//
// # Surface (6 methods on each of request_handle + response_handle)
//
//   - :base64Escape(s)   — upstream-parity per AMEND-22.2-1. Standard
//                          base64 with "=" padding (matches absl::Base64Escape).
//                          Wire output is byte-identical via Go
//                          encoding/base64.StdEncoding.EncodeToString.
//
//   - :base64Decode(s)   — envoy-go-strict per D8. Returns the decoded
//                          bytes on success; on decode error returns
//                          (nil, err_string) per upstream Lua idiom.
//
//   - :sha256(s)         — envoy-go-strict per D8. Returns the lower-case
//                          hex of crypto/sha256.Sum256.
//
//   - :sha512(s)         — envoy-go-strict per D8. Returns the lower-case
//                          hex of crypto/sha512.Sum512.
//
//   - :importPublicKey(pem) — upstream-parity per D8 (MIMICKING upstream
//                             wrappers.h:415-427 PublicKeyWrapper scope).
//                             Parses PEM via pem.Decode +
//                             x509.ParsePKIXPublicKey (PKIX-first; falls
//                             back to PKCS1 RSA for legacy "RSA PUBLIC KEY"
//                             PEMs). Returns a PublicKeyWrapper userdata
//                             whose :get() method returns the DER-encoded
//                             SubjectPublicKeyInfo bytes. On parse error
//                             raises a Lua runtime error with the arm-22
//                             byte-stable wording prefix
//                             `lua: importPublicKey: <inner>` per W2.
//
//   - :verifySignature(hash_algo, pubkey_wrapper, sig, text)
//                        — upstream-parity per D8 (calling convention
//                          pinned to upstream lua_filter.cc:611). hash_algo
//                          dispatches via switch on the string
//                          ("SHA1"/"SHA224"/"SHA256"/"SHA384"/"SHA512").
//                          Returns true on signature-verify success; false
//                          on signature-verify failure OR unsupported
//                          hash. NO Lua-runtime-error for verify failures
//                          (upstream-parity: invalid signatures are NOT a
//                          script error — they're a normal "did not
//                          verify" disposition).
//
// # Classification per D8 PLAN closure (4 envoy-go-strict + 2 upstream-parity)
//
//   - :base64Escape      — upstream-parity (per AMEND-22.2-1).
//   - :base64Decode      — envoy-go-strict (NOT in upstream
//                          StreamHandleWrapper; absent from
//                          PublicKeyWrapper + script-globals per Pin 2
//                          scrape).
//   - :sha256            — envoy-go-strict (same as above).
//   - :sha512            — envoy-go-strict (same as above).
//   - :importPublicKey   — upstream-parity (MIMICKING wrappers.h:415-427
//                          PublicKeyWrapper return scope).
//   - :verifySignature   — upstream-parity (calling convention pinned to
//                          upstream lua_filter.cc:611).
//
// 4 NEW envoy-go-strict BEHAVIOR_CONTRACT.md departure records anticipated
// at Task 19 atomic landing for :base64Decode + :sha256 + :sha512 +
// :fileBytes (the latter at misc.go).
//
// # Cross-references
//
//   - SPEC §3.4 (crypto surface)
//   - SPEC §6 arm-22 + W2 (byte-stable runtime-reject wording)
//   - SPEC §11.2.4 (crypto wire-output parity scrape)
//   - PLAN Task 12 (subagent dispatch outline)
//   - ADR-0192 §Decision body anticipation

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // upstream-parity signature algorithm
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"

	lua "github.com/yuin/gopher-lua"
)

// publicKeyWrapperTypeName is the metatable registry-key for the
// PublicKeyWrapper userdata returned by :importPublicKey per D8-sub
// closure (MIMICKING upstream wrappers.h:415-427 scope). Distinct key
// from other bridge userdata to preserve type-distinct identity for
// script authors who inspect getmetatable(pub).
const publicKeyWrapperTypeName = "envoy_public_key_wrapper"

// cryptoImportPublicKeyErrPrefix is the SPEC §6 arm-22 + W2 byte-stable
// runtime-reject wording prefix raised when :importPublicKey(pem) fails
// to parse. The full wording template is
// `lua: importPublicKey: <inner crypto/x509 error>`. Pinned via a
// package-level const so a regression on the byte-stable contract surfaces
// at the Test_ImportPublicKey_invalid_PEM_raises_arm22_byte_stable_wording
// assertion.
const cryptoImportPublicKeyErrPrefix = "lua: importPublicKey:"

// publicKeyWrapper is the Go-side state backing the PublicKeyWrapper
// userdata returned by :importPublicKey. Holds the DER-encoded
// SubjectPublicKeyInfo bytes (returned by :get()) + the parsed
// crypto.PublicKey interface (consumed by :verifySignature).
//
// Per D8-sub closure: MIMICKING upstream wrappers.h:415-427 scope —
// PublicKeyWrapper exposes the key bytes via :get() so script authors can
// log/audit the imported key. The parsed key is opaque to Lua (held
// Go-side only for signature-verify dispatch).
type publicKeyWrapper struct {
	derBytes []byte           // PKIX SubjectPublicKeyInfo DER (returned by :get)
	key      crypto.PublicKey // parsed key (RSA / ECDSA / Ed25519)
}

// publicKeyWrapperMethods is the method-name → LGFunction dispatch table
// for the PublicKeyWrapper userdata's __index metafield. Single method
// (:get) per D8-sub closure + upstream wrappers.h:415-427 scope.
var publicKeyWrapperMethods = map[string]lua.LGFunction{
	"get": requestHandlePublicKeyWrapperGet,
}

// installPublicKeyWrapperMetatable registers the PublicKeyWrapper
// metatable under publicKeyWrapperTypeName per Task 12 (phase 22.2 IMPL)
// + SPEC §3.4 + D8-sub closure. Idempotent — gopher-lua's
// NewTypeMetatable returns the existing metatable on second call.
//
// Consumed at filter VM construction (decode_headers.go +
// encode_headers.go) alongside the other bridge metatable installs.
func installPublicKeyWrapperMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(publicKeyWrapperTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), publicKeyWrapperMethods))
	return mt
}

// ---------------------------------------------------------------------
// :base64Escape — upstream-parity per AMEND-22.2-1
// ---------------------------------------------------------------------

// requestHandleBase64Escape implements request_handle:base64Escape(s) per
// SPEC §3.4 + AMEND-22.2-1. Returns the standard-base64 encoding of s
// (matches absl::Base64Escape standard-padding wire shape).
func requestHandleBase64Escape(L *lua.LState) int {
	_ = L.CheckUserData(1) // type-check the receiver (request_handle/response_handle)
	s := L.CheckString(2)
	L.Push(lua.LString(base64.StdEncoding.EncodeToString([]byte(s))))
	return 1
}

// ---------------------------------------------------------------------
// :base64Decode — envoy-go-strict per D8
// ---------------------------------------------------------------------

// requestHandleBase64Decode implements request_handle:base64Decode(s) per
// SPEC §3.4 + D8 (envoy-go-strict). On success returns the decoded bytes
// as a Lua string. On decode error returns (nil, err_string) per upstream
// Lua idiom (scripts may check `local b, err = rh:base64Decode(...); if
// b == nil then ... end`).
func requestHandleBase64Decode(L *lua.LState) int {
	_ = L.CheckUserData(1)
	s := L.CheckString(2)
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(b))
	return 1
}

// ---------------------------------------------------------------------
// :sha256 — envoy-go-strict per D8
// ---------------------------------------------------------------------

// requestHandleSha256 implements request_handle:sha256(s) per SPEC §3.4
// + D8 (envoy-go-strict). Returns the lower-case hex of
// crypto/sha256.Sum256.
func requestHandleSha256(L *lua.LState) int {
	_ = L.CheckUserData(1)
	s := L.CheckString(2)
	sum := sha256.Sum256([]byte(s))
	L.Push(lua.LString(hex.EncodeToString(sum[:])))
	return 1
}

// ---------------------------------------------------------------------
// :sha512 — envoy-go-strict per D8
// ---------------------------------------------------------------------

// requestHandleSha512 implements request_handle:sha512(s) per SPEC §3.4
// + D8 (envoy-go-strict). Returns the lower-case hex of
// crypto/sha512.Sum512.
func requestHandleSha512(L *lua.LState) int {
	_ = L.CheckUserData(1)
	s := L.CheckString(2)
	sum := sha512.Sum512([]byte(s))
	L.Push(lua.LString(hex.EncodeToString(sum[:])))
	return 1
}

// ---------------------------------------------------------------------
// :importPublicKey — upstream-parity per D8 (PublicKeyWrapper return)
// ---------------------------------------------------------------------

// requestHandleImportPublicKey implements
// request_handle:importPublicKey(pem) per SPEC §3.4 + D8-sub closure.
// Parses the supplied PEM-encoded SubjectPublicKeyInfo via pem.Decode +
// crypto/x509.ParsePKIXPublicKey (PKIX-first; falls back to PKCS1 RSA
// for legacy "RSA PUBLIC KEY" PEM blocks).
//
// On success returns a PublicKeyWrapper userdata whose :get() method
// returns the DER-encoded key bytes (MIMICKING upstream wrappers.h:
// 415-427 scope per D8-sub closure).
//
// On parse error raises a Lua runtime error with the arm-22 byte-stable
// wording prefix `lua: importPublicKey: <inner>` per W2 + SPEC §6 row 22.
func requestHandleImportPublicKey(L *lua.LState) int {
	_ = L.CheckUserData(1)
	pemStr := L.CheckString(2)
	wrapper, err := parsePublicKeyPEM([]byte(pemStr))
	if err != nil {
		L.RaiseError("%s %s", cryptoImportPublicKeyErrPrefix, err.Error())
		return 0
	}
	ud := L.NewUserData()
	ud.Value = wrapper
	L.SetMetatable(ud, L.GetTypeMetatable(publicKeyWrapperTypeName))
	L.Push(ud)
	return 1
}

// parsePublicKeyPEM decodes the supplied PEM bytes and tries PKIX first
// (the universal SubjectPublicKeyInfo encoding) then falls back to PKCS1
// RSA for legacy "RSA PUBLIC KEY" PEMs. Returns the wrapped key + DER
// bytes ready to embed in the userdata.
func parsePublicKeyPEM(pemBytes []byte) (*publicKeyWrapper, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}

	// PKIX-first per upstream parity (the universal encoding).
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		derBytes := make([]byte, len(block.Bytes))
		copy(derBytes, block.Bytes)
		return &publicKeyWrapper{derBytes: derBytes, key: key}, nil
	}

	// Fallback: PKCS1 RSA (legacy "RSA PUBLIC KEY" PEMs).
	if rsaKey, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		// Re-encode as PKIX for the :get() return so the wire shape
		// is universally consumable.
		derBytes, marshalErr := x509.MarshalPKIXPublicKey(rsaKey)
		if marshalErr != nil {
			return nil, marshalErr
		}
		return &publicKeyWrapper{derBytes: derBytes, key: rsaKey}, nil
	}

	// Both PKIX + PKCS1 RSA parse failed — surface the PKIX error
	// (the dominant case for operator-supplied PEMs).
	_, pkixErr := x509.ParsePKIXPublicKey(block.Bytes)
	if pkixErr == nil {
		pkixErr = errors.New("unable to parse public key")
	}
	return nil, pkixErr
}

// requestHandlePublicKeyWrapperGet implements PublicKeyWrapper:get() per
// SPEC §3.4 + D8-sub closure (MIMICKING upstream wrappers.h:415-427 scope).
// Returns the DER-encoded SubjectPublicKeyInfo bytes as a Lua string.
func requestHandlePublicKeyWrapperGet(L *lua.LState) int {
	ud := L.CheckUserData(1)
	wrapper, ok := ud.Value.(*publicKeyWrapper)
	if !ok {
		L.ArgError(1, "expected PublicKeyWrapper")
		return 0
	}
	if wrapper == nil || wrapper.derBytes == nil {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(lua.LString(wrapper.derBytes))
	return 1
}

// ---------------------------------------------------------------------
// :verifySignature — upstream-parity per D8 (4-arg calling convention)
// ---------------------------------------------------------------------

// requestHandleVerifySignature implements
// request_handle:verifySignature(hash_algo, pubkey_wrapper, sig, text)
// per SPEC §3.4 + D8 closure + upstream lua_filter.cc:611 calling
// convention. Dispatches via switch on hash_algo string
// ("SHA1"/"SHA224"/"SHA256"/"SHA384"/"SHA512"). Returns true on
// signature-verify success; false on signature-verify failure OR
// unsupported hash algo (NO Lua runtime error — upstream-parity:
// invalid signatures are a normal "did not verify" disposition).
//
// Arguments (per upstream calling convention):
//
//  1. receiver (request_handle/response_handle userdata)
//  2. hash_algo — string (e.g. "SHA256")
//  3. pubkey_wrapper — PublicKeyWrapper userdata (from :importPublicKey)
//  4. signature — string (raw bytes; e.g. RSA-PKCS1v15 / ECDSA-DER /
//     Ed25519 raw 64-byte signature)
//  5. text — string (the plaintext that was signed)
func requestHandleVerifySignature(L *lua.LState) int {
	_ = L.CheckUserData(1)
	hashAlgo := L.CheckString(2)
	ud := L.CheckUserData(3)
	sig := L.CheckString(4)
	text := L.CheckString(5)

	wrapper, ok := ud.Value.(*publicKeyWrapper)
	if !ok || wrapper == nil || wrapper.key == nil {
		L.Push(lua.LBool(false))
		return 1
	}

	hashFunc, ok := resolveHashAlgo(hashAlgo)
	if !ok {
		L.Push(lua.LBool(false))
		return 1
	}

	if err := verifySignature(wrapper.key, hashFunc, []byte(sig), []byte(text)); err != nil {
		L.Push(lua.LBool(false))
		return 1
	}
	L.Push(lua.LBool(true))
	return 1
}

// resolveHashAlgo maps a hash-algo string (matching upstream Envoy Lua
// filter's supported set: SHA1/SHA224/SHA256/SHA384/SHA512) to a
// crypto.Hash enum + hash.Hash factory. Returns (0, false) for
// unsupported algos.
func resolveHashAlgo(name string) (crypto.Hash, bool) {
	switch name {
	case "SHA1":
		return crypto.SHA1, true
	case "SHA224":
		return crypto.SHA224, true
	case "SHA256":
		return crypto.SHA256, true
	case "SHA384":
		return crypto.SHA384, true
	case "SHA512":
		return crypto.SHA512, true
	}
	return 0, false
}

// newHash returns a fresh hash.Hash instance for the given crypto.Hash
// enum. The Go-side helper exists because crypto.Hash.New() will panic
// if the hash algorithm hasn't been linked into the binary — our switch
// in resolveHashAlgo guarantees the standard SHA family is registered.
func newHash(h crypto.Hash) hash.Hash {
	switch h {
	case crypto.SHA1:
		return sha1.New() //nolint:gosec // upstream-parity signature algorithm
	case crypto.SHA224:
		return sha256.New224()
	case crypto.SHA256:
		return sha256.New()
	case crypto.SHA384:
		return sha512.New384()
	case crypto.SHA512:
		return sha512.New()
	}
	return nil
}

// verifySignature dispatches signature verification to the per-key-type
// verifier (RSA-PKCS1v15 / ECDSA-DER / Ed25519). Returns nil on success;
// non-nil error on signature mismatch or key-type mismatch (caught by
// the caller as a "verify-false" disposition).
func verifySignature(key crypto.PublicKey, hashFunc crypto.Hash, sig, text []byte) error {
	switch k := key.(type) {
	case *rsa.PublicKey:
		hasher := newHash(hashFunc)
		if hasher == nil {
			return errors.New("unsupported hash for RSA verify")
		}
		hasher.Write(text)
		hashed := hasher.Sum(nil)
		return rsa.VerifyPKCS1v15(k, hashFunc, hashed, sig)
	case *ecdsa.PublicKey:
		hasher := newHash(hashFunc)
		if hasher == nil {
			return errors.New("unsupported hash for ECDSA verify")
		}
		hasher.Write(text)
		hashed := hasher.Sum(nil)
		if ecdsa.VerifyASN1(k, hashed, sig) {
			return nil
		}
		return errors.New("ECDSA signature verification failed")
	case ed25519.PublicKey:
		// Ed25519 ignores the hash algo (it always uses its own internal
		// hashing). Upstream Envoy parity: pass-through to ed25519.Verify.
		if ed25519.Verify(k, text, sig) {
			return nil
		}
		return errors.New("Ed25519 signature verification failed")
	}
	return fmt.Errorf("unsupported public key type %T", key)
}

// ---------------------------------------------------------------------
// Response-handle parity stubs — symmetric to request-handle for the
// encode side. Scripts writing envoy_on_response may equivalently call
// :base64Escape / :base64Decode / :sha256 / :sha512 / :importPublicKey /
// :verifySignature on the response_handle. All 6 methods reuse the
// shared LGFunctions (the receiver type-check at index 1 covers both
// requestHandleContext and responseHandleContext — neither is consulted
// inside the crypto LGFunction bodies; the userdata is only validated as
// non-nil + a userdata).
// ---------------------------------------------------------------------

// responseHandleBase64Escape mirrors requestHandleBase64Escape for the
// encode side. Same implementation; receiver type is decoupled (the
// CheckUserData(1) is only a sanity-check — the crypto methods are
// stateless w.r.t. the handle context).
func responseHandleBase64Escape(L *lua.LState) int    { return requestHandleBase64Escape(L) }
func responseHandleBase64Decode(L *lua.LState) int    { return requestHandleBase64Decode(L) }
func responseHandleSha256(L *lua.LState) int          { return requestHandleSha256(L) }
func responseHandleSha512(L *lua.LState) int          { return requestHandleSha512(L) }
func responseHandleImportPublicKey(L *lua.LState) int { return requestHandleImportPublicKey(L) }
func responseHandleVerifySignature(L *lua.LState) int { return requestHandleVerifySignature(L) }
