package oauth2

// hmac.go — 5-input HMAC-SHA256 cookie envelope composition + dual-encoding
// read for the envoy.filters.http.oauth2 cookie envelope per phase-20
// AMEND-2 + §20.P4 REFUTED + S4. Anchored at ADR-0179.
//
// # Composition (per AMEND-2 + §20.P4 REFUTED)
//
// The HMAC input is a 5-input newline-joined message:
//
//	HMAC-SHA256(hmac_secret, StrJoin({domain, expires, token, id_token, refresh_token}, "\n"))
//
// The BRAINSTORM Q9 3-input hypothesis was REFUTED at the SPEC §11 §20.P4
// empirical scrape against reference Envoy v1.37.2 — upstream's composition
// is unambiguously 5-input.
//
// # Empty-string-when-absent semantics
//
// When the request envelope lacks IdToken (MVP — id_token deferred per SPEC
// §2.2) OR RefreshToken (MVP — when `use_refresh_token=false` OR after a
// refresh-rotation completes), the corresponding input is the empty string.
// The StrJoin produces `domain + "\n" + expires + "\n" + token + "\n" + "\n" + ""`
// for the (no-IdToken, no-RefreshToken) case. The "\n\n" double-separator
// IS DELIBERATE — it's the upstream byte-exact composition.
//
// # Emit-side encoding (per SPEC §6.5)
//
// `computeHMAC` returns Base64URL-raw (unpadded). 32 raw HMAC-SHA256 bytes
// always encode to exactly 43 Base64URL chars.
//
// # Read-side dual encoding (per S4)
//
// `validateHMAC` accepts BOTH Base64 encodings AND the nested HexBase64
// encoding (Base64URL-of-hex-string-of-raw-bytes) on read. The S4 settlement
// rationale: some upstream Envoy deployments emit HexBase64 due to
// historical configuration; the MVP widens compatibility by accepting BOTH
// at validation time. The decode cascade tries:
//
//  1. Base64URL-raw (unpadded; the canonical emit form)
//  2. Base64URL-std (padded; operator-tolerant fall-back — some operators
//     emit padded encodings due to lib choice)
//  3. Nested Base64URL → hex.DecodeString (the HexBase64 variant per S4)
//
// The first decoder that yields exactly 32 bytes (the HMAC-SHA256 sum length)
// is accepted. We DO NOT short-circuit on partial decodes — a 32-byte output
// is the cryptographic precondition for the subsequent hmac.Equal compare.
//
// # Constant-time-compare invariant
//
// The decoded candidate is compared against the computed HMAC via
// `crypto/hmac.Equal` (constant-time). The dual-encoding decode cascade does
// NOT compromise constant-time semantics: each decoder branch is independent
// of secret bytes; only the final compare touches the computed HMAC, and
// that compare is constant-time relative to the candidate's bytes.
//
// # No nil-tolerance shortcuts
//
// validateHMAC returns false on every error path — empty cookie value,
// malformed Base64URL, wrong-length decoded output, HMAC mismatch. NO panic
// path; the fuzzer (Task 12) asserts must-never-panic across this surface
// per SPEC §14.3.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// hmacOutputLen is the byte-length of an HMAC-SHA256 sum. Pinned as a
// constant for the decode-cascade length-check.
const hmacOutputLen = sha256.Size // 32

// computeHMAC returns the Base64URL-raw-encoded HMAC-SHA256 of the 5-input
// newline-joined message per phase-20 AMEND-2 + §20.P4 REFUTED + ADR-0179.
//
// All 5 inputs participate as-is — id_token + refresh_token contribute as
// empty strings when absent (NOT skipped). The output is always exactly 43
// chars (43 = ceil(32 * 8 / 6) for unpadded Base64URL).
//
// hmacSecret length is unconstrained per RFC 2104; callers typically supply
// the bytes from the filesystem-SDS-loaded `hmac_secret` Secret per ADR-0178.
func computeHMAC(domain, expires, token, idToken, refreshToken string, hmacSecret []byte) string {
	// Delegate the message composition + MAC computation to rawHMAC (the
	// single home of the 5-input composition); only the Base64URL-raw
	// encoding differs between the emit side and the validate side.
	return base64.RawURLEncoding.EncodeToString(rawHMAC(domain, expires, token, idToken, refreshToken, hmacSecret))
}

// validateHMAC returns true iff cookieHMAC validates against the recomputed
// HMAC for the (domain, expires, token, idToken, refreshToken) tuple under
// hmacSecret. Dual-encoding read per S4 — accepts Base64URL-raw,
// Base64URL-std (padded), and the nested HexBase64 (Base64URL-of-hex-string)
// variants. Constant-time compare via crypto/hmac.Equal.
//
// Returns false on every error path: empty cookieHMAC, malformed envelope,
// wrong-length decoded output, HMAC mismatch. NO panic.
func validateHMAC(domain, expires, token, idToken, refreshToken string, hmacSecret []byte, cookieHMAC string) bool {
	if cookieHMAC == "" {
		return false
	}

	// Recompute the expected HMAC once; reuse across the decode-cascade
	// candidates. Computing it ONCE preserves constant-time semantics —
	// every decode branch ends in the same hmac.Equal call against the same
	// computed sum.
	expected := rawHMAC(domain, expires, token, idToken, refreshToken, hmacSecret)

	// Decode cascade per S4 dual-encoding read. Each decoder branch is
	// independent of secret bytes; only the final compare touches `expected`.
	candidates := decodeDualEncoding(cookieHMAC)
	for _, candidate := range candidates {
		if len(candidate) != hmacOutputLen {
			continue
		}
		if hmac.Equal(candidate, expected) {
			return true
		}
	}
	return false
}

// rawHMAC computes the raw 32-byte HMAC-SHA256 sum without Base64 encoding —
// the single home of the 5-input message composition. computeHMAC wraps it
// with the Base64URL-raw emit encoding; validateHMAC consumes the raw sum
// for the constant-time compare against the candidate decoded bytes.
func rawHMAC(domain, expires, token, idToken, refreshToken string, hmacSecret []byte) []byte {
	// Compose the 5-input newline-joined message in a single allocation by
	// pre-computing the final length. This is a hot path on every request
	// that carries a cookie envelope, so the allocation discipline matters.
	totalLen := len(domain) + len(expires) + len(token) +
		len(idToken) + len(refreshToken) + 4 // 4 newlines between 5 inputs
	msg := make([]byte, 0, totalLen)
	msg = append(msg, domain...)
	msg = append(msg, '\n')
	msg = append(msg, expires...)
	msg = append(msg, '\n')
	msg = append(msg, token...)
	msg = append(msg, '\n')
	msg = append(msg, idToken...)
	msg = append(msg, '\n')
	msg = append(msg, refreshToken...)

	mac := hmac.New(sha256.New, hmacSecret)
	mac.Write(msg)
	return mac.Sum(nil)
}

// decodeDualEncoding returns the candidate raw-byte decodings of the
// cookieHMAC envelope per S4 dual-encoding read. Returns 1-3 candidates
// depending on which decoder branches yield bytes; the caller filters by
// length + constant-time-compares.
//
// Branches:
//
//  1. Base64URL-raw (the canonical emit form per SPEC §6.5)
//  2. Base64URL-std (padded; operator-tolerant fall-back)
//  3. Base64URL → hex.DecodeString (the HexBase64 nested variant per S4)
//
// The function is allocation-bounded: at most 3 candidate slices. Each
// branch independently attempts a decode; failures contribute no candidate
// (zero-length slice not appended).
func decodeDualEncoding(cookieHMAC string) [][]byte {
	candidates := make([][]byte, 0, 3)

	// Branch 1: Base64URL-raw (unpadded). The canonical emit form.
	if raw, err := base64.RawURLEncoding.DecodeString(cookieHMAC); err == nil {
		candidates = append(candidates, raw)
	}

	// Branch 2: Base64URL-std (with '=' padding). Operator-tolerant
	// fall-back — some operators emit padded encodings due to lib choice.
	if std, err := base64.URLEncoding.DecodeString(cookieHMAC); err == nil {
		candidates = append(candidates, std)
	}

	// Branch 3: Nested HexBase64 per S4 — Base64URL-decode to get a hex
	// string, then hex-decode to get the raw bytes. Some upstream Envoy
	// deployments emit this encoding due to historical configuration; the
	// MVP widens compatibility by accepting it at validation time.
	//
	// The nesting tries BOTH outer base64 forms (raw + std) before
	// hex-decoding. We use the FIRST outer-decode that succeeded above
	// (if any) AND independently re-attempt with the other form. To keep
	// the cascade simple + cheap, we just iterate over the outer candidates
	// already collected and hex-decode each — any candidate that happens to
	// be valid ASCII hex contributes a nested-decode candidate.
	for _, outer := range candidates {
		if len(outer) == 0 {
			continue
		}
		// Hex decoding requires even-length ASCII hex input.
		if len(outer)%2 != 0 {
			continue
		}
		if inner, err := hex.DecodeString(string(outer)); err == nil {
			candidates = append(candidates, inner)
		}
	}

	return candidates
}
