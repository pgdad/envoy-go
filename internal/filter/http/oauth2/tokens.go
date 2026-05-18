package oauth2

// tokens.go — filter-local AES-256-CBC token-encryption helper for the
// envoy.filters.http.oauth2 cookie envelope per phase-20 AMEND-1 + §20.P5
// REFUTED + AMEND-3 decrypt-failure fall-back. Anchored at ADR-0182.
//
// # Surface
//
// Two filter-local functions exposed within the `oauth2` package — NOT
// exported (lowercase initial letter; filter-local discipline per ADR-0182):
//
//   - encryptToken(plaintext, hmacSecret []byte) string
//   - decryptToken(envelope string, hmacSecret []byte) []byte
//
// Both functions are stateless + allocation-bounded + panic-free. They do
// NOT consult any operator config or `disable_token_encryption` flag — the
// caller (callback.go / decode_headers.go at Task 11) owns the skip-path
// dispatch per ADR-0182 + SPEC §2.15.
//
// # Algorithm per AMEND-1 + §20.P5 REFUTED
//
// AES-256-CBC. The BRAINSTORM Q4 hypothesis of AES-256-GCM was REFUTED at
// SPEC §11 §20.P5 empirical scrape against reference Envoy v1.37.2 — upstream
// uses CBC, NOT GCM. The algorithm swap is recorded at AMEND-1.
//
//   - Key derivation: SHA-256(hmacSecret)[:32] → 32-byte AES-256 key
//     (SHA-256 produces exactly 32 bytes; the [:32] truncation is equivalent
//     to the full sum but pinned-as-such for spec-readability).
//   - IV: 16 random bytes per encryption, prepended to ciphertext. Read via
//     crypto/rand.Read; the (extremely rare) error path panics — a failure
//     to read from crypto/rand indicates a broken OS RNG which is unrecoverable.
//   - Padding: PKCS#7. Plaintext is padded to a multiple of aes.BlockSize
//     (16 bytes); the padding byte value equals the padding length (1..16).
//     An exact-block-multiple plaintext gets a FULL extra padding block of
//     0x10 bytes.
//   - Wire envelope: base64.RawURLEncoding.EncodeToString(iv ‖ ct) — URL-safe
//     alphabet, no padding chars. Suitable for direct use as a cookie value.
//
// # Decryption-failure fall-back per AMEND-3
//
// decryptToken returns []byte(envelope) verbatim on ANY error path:
//
//   - base64 decode failure (malformed envelope)
//   - decoded length < aes.BlockSize (no IV → no separable CT)
//   - decoded length - aes.BlockSize is not a positive multiple of 16 (CT not
//     a clean block multiple)
//   - PKCS#7 padding validation failure (last byte out of [1, 16], or last
//     padLen bytes not all equal to padLen)
//
// NO error is returned; NO `cookie_decrypt_failure` counter is incremented
// (per §20.P11 RATIFIED-AS-ABSENT). The downstream HMAC validation at
// hmac.go::validateHMAC rejects the fall-back bytes naturally — the recovered
// "plaintext" cannot produce a valid HMAC against the envelope tuple.
//
// This discipline keeps the wire response shape identical between
// "wrong key" and "tampered cookie" — both produce a downstream HMAC
// validation failure, NOT a distinguishable decrypt-error oracle. The
// fall-back IS the padding-oracle hardening — no error-path information leak.
//
// # disable_token_encryption skip-path (consumer-level)
//
// When the operator-config flag `disable_token_encryption=true` (per SPEC
// §2.15), the consumer (the caller at callback.go / decode_headers.go) does
// NOT invoke encryptToken/decryptToken — cookie values are stored + returned
// verbatim. tokens.go has NO awareness of the flag; the skip is exclusively
// at the caller (per ADR-0182 §Decision).
//
// # atomic.Pointer[[32]byte] discipline (consumer-level)
//
// The cooperating consumer (compiledConfig.aesKey at Task 11) holds the AES
// key behind `atomic.Pointer[[32]byte]` for the sdsfile-driven reload path
// per ADR-0182 + ADR-0178. tokens.go's parameter shape (`hmacSecret []byte`)
// accepts any byte slice; the atomic discipline lives at the caller. Race-
// clean under `go test -race` per TestAesKeySwap_Concurrent_* (planner-time
// D4).
//
// # No-panic discipline (one documented exception)
//
// encryptToken + decryptToken NEVER panic on input-driven paths. The single
// exception is encryptToken's `rand.Read` error — a broken OS RNG is
// unrecoverable; we let the panic propagate. crypto/aes.NewCipher cannot
// error for a 32-byte key (the SHA-256 KDF guarantees the size); we discard
// the error return per the documented stdlib contract.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// encryptToken encrypts plaintext under AES-256-CBC with a random 16-byte IV.
// Returns base64.RawURLEncoding.EncodeToString(IV ‖ CT) per phase-20 AMEND-1
// + §20.P5 REFUTED + ADR-0182 wire envelope shape.
//
// Key derivation: SHA-256(hmacSecret)[:32] → 32-byte AES-256 key. Padding:
// PKCS#7. The IV is read via crypto/rand.Read; a failure to read from the
// OS RNG panics (unrecoverable).
//
// hmacSecret length is unconstrained — the SHA-256 KDF accepts any byte
// sequence including nil/empty. Operators who supply an empty hmac_secret
// get a deterministic but PUBLIC AES key; this is operator-error-protected
// at HCM-parse-time by the SDS-secret-empty PARSE-REJECT discipline (not
// here in tokens.go).
func encryptToken(plaintext, hmacSecret []byte) string {
	key := deriveAESKey(hmacSecret)

	// aes.NewCipher never errors for a 32-byte key (the SHA-256 KDF
	// guarantees the size). The error return is discarded per the stdlib
	// contract documented at https://pkg.go.dev/crypto/aes#NewCipher.
	block, _ := aes.NewCipher(key[:])

	// IV: 16 random bytes per encryption. Panic on RNG failure (unrecoverable).
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		// crypto/rand.Read errors only on catastrophic OS RNG failure; the
		// runtime contract is "should never happen in practice". Panic per the
		// no-panic-discipline single documented exception.
		panic("oauth2: crypto/rand.Read failed during AES IV generation: " + err.Error())
	}

	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)

	// Envelope: IV ‖ CT (single alloc; the result is sized exactly).
	envelope := make([]byte, 0, aes.BlockSize+len(ct))
	envelope = append(envelope, iv...)
	envelope = append(envelope, ct...)
	return base64.RawURLEncoding.EncodeToString(envelope)
}

// decryptToken decrypts a base64.RawURLEncoding(IV ‖ CT) envelope under
// SHA-256(hmacSecret)[:32]. On ANY decoding/decryption failure (malformed
// base64, truncated envelope, block-size mismatch, bad padding) returns
// []byte(envelope) verbatim per phase-20 AMEND-3 + ADR-0182 §Decision —
// the downstream HMAC validation step at hmac.go::validateHMAC rejects the
// fall-back bytes naturally.
//
// NO error is returned. NO `cookie_decrypt_failure` counter is incremented
// (per §20.P11 RATIFIED-AS-ABSENT). The function NEVER panics; arbitrary
// inbound envelope bytes are tolerated (the fuzzer at Task 12 asserts the
// no-panic discipline across this surface per SPEC §14.3).
func decryptToken(envelope string, hmacSecret []byte) []byte {
	fallback := []byte(envelope) // AMEND-3: ciphertext-as-plaintext on any failure

	raw, err := base64.RawURLEncoding.DecodeString(envelope)
	if err != nil {
		return fallback
	}
	if len(raw) < aes.BlockSize {
		// Need at least an IV worth of bytes; with 0 CT bytes there's
		// nothing to decrypt.
		return fallback
	}
	ctLen := len(raw) - aes.BlockSize
	if ctLen == 0 || ctLen%aes.BlockSize != 0 {
		// CT must be a positive multiple of the AES block size.
		return fallback
	}

	iv := raw[:aes.BlockSize]
	ct := raw[aes.BlockSize:]

	key := deriveAESKey(hmacSecret)
	block, _ := aes.NewCipher(key[:]) // never errors for 32-byte key

	padded := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(padded, ct)

	pt, ok := pkcs7Unpad(padded, aes.BlockSize)
	if !ok {
		return fallback
	}
	return pt
}

// deriveAESKey returns the 32-byte AES-256 key derived from hmacSecret per
// phase-20 ADR-0182 + AMEND-1: SHA-256(hmacSecret)[:32]. Since SHA-256
// produces exactly 32 bytes, the [:32] truncation is equivalent to the full
// sum but pinned-as-such for spec-readability.
//
// Returns a value type ([32]byte) — callers slice with [:] for the
// crypto/aes.NewCipher call. The value-type return ensures the key bytes are
// stack-allocated where possible.
func deriveAESKey(hmacSecret []byte) [32]byte {
	return sha256.Sum256(hmacSecret)
}

// pkcs7Pad returns plaintext padded to a multiple of blockSize per PKCS#7
// (RFC 5652 §6.3). The padding byte value equals the padding length
// (1..blockSize). An exact-block-multiple plaintext receives a FULL extra
// padding block of `blockSize` bytes (each holding the value `blockSize`).
//
// blockSize must be in [1, 255]; for AES blockSize == 16. The function never
// panics on input — the empty plaintext yields a single full padding block.
func pkcs7Pad(plaintext []byte, blockSize int) []byte {
	padLen := blockSize - (len(plaintext) % blockSize)
	out := make([]byte, len(plaintext)+padLen)
	copy(out, plaintext)
	for i := len(plaintext); i < len(out); i++ {
		out[i] = byte(padLen)
	}
	return out
}

// pkcs7Unpad strips PKCS#7 padding from padded per RFC 5652 §6.3, returning
// the unpadded plaintext and true on success. On any validation failure
// (length not a positive multiple of blockSize, last byte not in [1, blockSize],
// or the last padLen bytes not all equal to padLen), returns (nil, false).
//
// The padding-check is constant-time relative to the padding length to avoid
// padding-oracle timing side-channels; the AMEND-3 fall-back at decryptToken
// is the load-bearing oracle-hardening (returning ciphertext-as-plaintext on
// failure eliminates the distinguishable error-path), but a constant-time
// strip is the defense-in-depth.
func pkcs7Unpad(padded []byte, blockSize int) ([]byte, bool) {
	n := len(padded)
	if n == 0 || n%blockSize != 0 {
		return nil, false
	}
	padLen := int(padded[n-1])
	if padLen == 0 || padLen > blockSize {
		return nil, false
	}
	// Constant-time-relative-to-padLen padding-bytes-equal check: iterate
	// the entire last block, accumulate diffs in a single int, then check
	// once at the end. This avoids early-exit on first-bad-padding-byte.
	var diff byte
	for i := n - padLen; i < n; i++ {
		diff |= padded[i] ^ byte(padLen)
	}
	if diff != 0 {
		return nil, false
	}
	return padded[:n-padLen], true
}
