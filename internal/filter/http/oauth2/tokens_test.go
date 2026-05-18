package oauth2

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// -----------------------------------------------------------------------------
// Group 3 — AES-256-CBC encrypt/decrypt vector tests per phase-20 SPEC §14.1 +
// AMEND-1 + AMEND-3 + ADR-0182. The Group 3 anchor is `tokens.go`'s
// encryptToken + decryptToken pair. The decryption-failure fall-back rows
// settle SPEC §12 item B6 (RATIFIED-PENDING-IMPL-TIME per planner-time D17 →
// CLOSED-AT-TASK-7) at unit-test time.
//
// AMEND-1 algorithm: AES-256-CBC; SHA-256(hmacSecret)[:32] KDF; random 16-byte
// IV per encryption (prepended); PKCS#7 padding; Base64URL(IV ‖ CT) envelope.
//
// AMEND-3 decrypt-failure semantics: on ANY decoding/decryption failure
// (malformed base64, truncated envelope, block-size mismatch, bad padding),
// decryptToken returns []byte(envelope) verbatim — the downstream HMAC
// validation step at hmac.go::validateHMAC rejects the fall-back bytes
// naturally. NO error returned. NO `cookie_decrypt_failure` counter increment
// per §20.P11 RATIFIED-AS-ABSENT.
// -----------------------------------------------------------------------------

// hmacSecretForTokens is the shared HMAC-secret used across the Group 3 vector
// tests. The byte sequence is arbitrary but pinned. The 24-byte length is
// deliberately NOT 32 — exercises the SHA-256 KDF (the SHA-256(hmacSecret)[:32]
// derivation is invariant to input length).
var hmacSecretForTokens = []byte("phase-20-test-hmac-key-1")

// TestEncryptToken_RoundTrip_ByteExact exercises the canonical happy path
// across several plaintext sizes: encrypt → decrypt → byte-exact plaintext.
// Covers the foundational correctness invariant for AES-256-CBC with random
// IV prepended + PKCS#7 padding.
func TestEncryptToken_RoundTrip_ByteExact(t *testing.T) {
	plaintexts := [][]byte{
		[]byte("access-token-abc"),
		[]byte("a"),
		[]byte("hello world!"),
		bytes.Repeat([]byte("x"), 100),
		[]byte("eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature"),
	}
	for _, pt := range plaintexts {
		env := encryptToken(pt, hmacSecretForTokens)
		got := decryptToken(env, hmacSecretForTokens)
		if !bytes.Equal(got, pt) {
			t.Fatalf("round-trip mismatch:\n got: %q\nwant: %q", got, pt)
		}
	}
}

// TestEncryptToken_RandomIV_DistinctOutputs asserts that the random 16-byte
// IV is in fact random — encrypting the same plaintext with the same key twice
// yields two distinct envelopes. A regression here (e.g., a future refactor
// that hard-codes the IV or seeds the RNG) would silently degrade the
// chosen-plaintext security posture.
func TestEncryptToken_RandomIV_DistinctOutputs(t *testing.T) {
	pt := []byte("identical-plaintext")
	envA := encryptToken(pt, hmacSecretForTokens)
	envB := encryptToken(pt, hmacSecretForTokens)
	if envA == envB {
		t.Fatalf("expected distinct envelopes for repeated encryption (random IV); both = %q", envA)
	}
	// Both must decrypt back to the same plaintext.
	if got := decryptToken(envA, hmacSecretForTokens); !bytes.Equal(got, pt) {
		t.Fatalf("envA round-trip mismatch: got=%q want=%q", got, pt)
	}
	if got := decryptToken(envB, hmacSecretForTokens); !bytes.Equal(got, pt) {
		t.Fatalf("envB round-trip mismatch: got=%q want=%q", got, pt)
	}
}

// TestEncryptToken_KDF_Sha256TruncatedTo32 asserts that the key derivation
// follows SHA-256(hmacSecret)[:32] per ADR-0182. We encrypt with a known
// hmacSecret and verify that decryption succeeds when we feed a DIFFERENT
// byte sequence whose SHA-256[:32] would only match if our KDF differs. We
// also re-derive the key independently and verify the SHA-256 sum size is
// indeed 32 bytes (so [:32] is equivalent to the full sum).
func TestEncryptToken_KDF_Sha256TruncatedTo32(t *testing.T) {
	// Sanity: SHA-256 produces exactly 32 bytes; the [:32] truncation is
	// equivalent to the full sum. A regression that drifted (e.g., [:16] for
	// AES-128) would not just downgrade — it would break decrypt-round-trip.
	sum := sha256.Sum256(hmacSecretForTokens)
	if len(sum) != 32 {
		t.Fatalf("sha256 sum length != 32 (got %d) — invariant broken", len(sum))
	}

	pt := []byte("known-plaintext-for-kdf-test")
	env := encryptToken(pt, hmacSecretForTokens)
	got := decryptToken(env, hmacSecretForTokens)
	if !bytes.Equal(got, pt) {
		t.Fatalf("KDF round-trip mismatch: got=%q want=%q", got, pt)
	}

	// Encrypting with a 32-byte hmacSecret whose value is the pre-computed
	// SHA-256 hash of the original 24-byte hmacSecret SHOULD produce a
	// distinct AES key (because SHA-256(sum) != sum); so decrypt-with-original
	// would fall through AMEND-3 to ciphertext-as-plaintext.
	preHashed := sum[:]
	envPre := encryptToken(pt, preHashed)
	got2 := decryptToken(envPre, hmacSecretForTokens)
	if bytes.Equal(got2, pt) {
		t.Fatalf("decrypting envPre with original hmacSecret yielded original plaintext — KDF is not hashing")
	}
}

// TestEncryptToken_PKCS7Padding_BlockBoundary asserts the PKCS#7 padding
// invariant at the AES block boundary: a 16-byte plaintext (1 full block)
// must be padded with a FULL 16-byte padding block (PKCS#7 requires
// non-empty padding even when the plaintext is a clean block multiple).
// The expected raw byte count pre-base64: 16 IV + 16 CT + 16 padding-block = 48.
func TestEncryptToken_PKCS7Padding_BlockBoundary(t *testing.T) {
	pt := bytes.Repeat([]byte{0x41}, 16) // 16 'A' chars = 1 AES block
	env := encryptToken(pt, hmacSecretForTokens)
	raw, err := base64.RawURLEncoding.DecodeString(env)
	if err != nil {
		t.Fatalf("envelope is not valid base64.RawURLEncoding: %v", err)
	}
	if want := 48; len(raw) != want {
		t.Fatalf("PKCS#7 block-boundary length mismatch: got=%d want=%d", len(raw), want)
	}
	// Round-trip cross-check.
	got := decryptToken(env, hmacSecretForTokens)
	if !bytes.Equal(got, pt) {
		t.Fatalf("block-boundary round-trip mismatch: got=%q want=%q", got, pt)
	}
}

// TestEncryptToken_PKCS7Padding_EmptyPlaintext asserts the edge case: a
// zero-byte plaintext must yield 16 IV + 16 padding-only block = 32 raw bytes.
// The padding-only block is all 0x10 (decimal 16) bytes per PKCS#7.
func TestEncryptToken_PKCS7Padding_EmptyPlaintext(t *testing.T) {
	env := encryptToken([]byte{}, hmacSecretForTokens)
	raw, err := base64.RawURLEncoding.DecodeString(env)
	if err != nil {
		t.Fatalf("envelope is not valid base64.RawURLEncoding: %v", err)
	}
	if want := 32; len(raw) != want {
		t.Fatalf("PKCS#7 empty-plaintext length mismatch: got=%d want=%d", len(raw), want)
	}
	got := decryptToken(env, hmacSecretForTokens)
	if len(got) != 0 {
		t.Fatalf("empty-plaintext round-trip mismatch: got=%q want=(empty)", got)
	}
}

// TestEncryptToken_PKCS7Padding_NonBlockMultiple asserts the typical case: a
// 7-byte plaintext must pad to 16 bytes (one block) with 9 padding bytes,
// each holding the value 0x09. The expected raw byte count pre-base64:
// 16 IV + 16 padded-block = 32.
func TestEncryptToken_PKCS7Padding_NonBlockMultiple(t *testing.T) {
	pt := []byte("seven!!") // 7 bytes
	env := encryptToken(pt, hmacSecretForTokens)
	raw, err := base64.RawURLEncoding.DecodeString(env)
	if err != nil {
		t.Fatalf("envelope is not valid base64.RawURLEncoding: %v", err)
	}
	if want := 32; len(raw) != want {
		t.Fatalf("PKCS#7 non-block-multiple length mismatch: got=%d want=%d", len(raw), want)
	}
	got := decryptToken(env, hmacSecretForTokens)
	if !bytes.Equal(got, pt) {
		t.Fatalf("non-block-multiple round-trip mismatch: got=%q want=%q", got, pt)
	}
}

// TestEncryptToken_Base64URLEnvelope asserts the envelope encoding is
// base64.RawURLEncoding (no padding chars; URL-safe alphabet). A regression
// that emitted standard Base64 (with '+/' chars or '=' padding) would break
// cookie wire-compat (the '+' char in a cookie value requires quoting per
// RFC 6265).
func TestEncryptToken_Base64URLEnvelope(t *testing.T) {
	env := encryptToken([]byte("test"), hmacSecretForTokens)
	if strings.ContainsAny(env, "+/=") {
		t.Fatalf("envelope contains non-URL-safe chars or padding: %q", env)
	}
	if _, err := base64.RawURLEncoding.DecodeString(env); err != nil {
		t.Fatalf("envelope is not valid base64.RawURLEncoding: %v", err)
	}
}

// TestEncryptToken_VariousPlaintextSizes exercises the encrypt/decrypt
// round-trip across a wide range of plaintext sizes including all the
// edge-cases around the 16-byte block boundary + larger sizes.
func TestEncryptToken_VariousPlaintextSizes(t *testing.T) {
	sizes := []int{0, 1, 15, 16, 17, 32, 256, 4096}
	for _, n := range sizes {
		pt := bytes.Repeat([]byte{0x7e}, n)
		env := encryptToken(pt, hmacSecretForTokens)
		got := decryptToken(env, hmacSecretForTokens)
		if !bytes.Equal(got, pt) {
			t.Fatalf("size=%d round-trip mismatch: got=%d-bytes want=%d-bytes", n, len(got), len(pt))
		}
		// Raw byte count: 16 (IV) + ceil(n+1, 16)*16 (PKCS#7 always adds at
		// least 1 padding byte; for n%16 == 0 the padding is a full extra block).
		raw, err := base64.RawURLEncoding.DecodeString(env)
		if err != nil {
			t.Fatalf("size=%d envelope is not valid base64.RawURLEncoding: %v", n, err)
		}
		padded := ((n / 16) + 1) * 16
		want := 16 + padded
		if len(raw) != want {
			t.Fatalf("size=%d raw byte count mismatch: got=%d want=%d", n, len(raw), want)
		}
	}
}

// TestDecryptToken_HappyPath asserts the canonical decrypt path: a known
// envelope decrypts to its known plaintext. (This is structurally redundant
// with the round-trip tests but pins the standalone decrypt surface.)
func TestDecryptToken_HappyPath(t *testing.T) {
	pt := []byte("happy-path-plaintext")
	env := encryptToken(pt, hmacSecretForTokens)
	got := decryptToken(env, hmacSecretForTokens)
	if !bytes.Equal(got, pt) {
		t.Fatalf("happy-path decrypt mismatch: got=%q want=%q", got, pt)
	}
}

// TestDecryptToken_MalformedBase64_ReturnsCiphertextAsPlaintext_NoError pins
// AMEND-3: malformed base64 input must return []byte(envelope) verbatim.
// NO error. The downstream HMAC validation step at hmac.go::validateHMAC
// rejects the fall-back bytes naturally.
func TestDecryptToken_MalformedBase64_ReturnsCiphertextAsPlaintext_NoError(t *testing.T) {
	bad := "!!not-base64!!" // contains invalid base64.RawURLEncoding chars
	got := decryptToken(bad, hmacSecretForTokens)
	if string(got) != bad {
		t.Fatalf("AMEND-3 fall-back mismatch: got=%q want=%q (verbatim envelope bytes)", got, bad)
	}
}

// TestDecryptToken_BadPadding_ReturnsCiphertextAsPlaintext_NoError pins
// AMEND-3 at the PKCS#7-padding-error path: a valid-base64 input with a
// valid-looking 16-byte IV but a CT whose decrypted padding byte is invalid
// (e.g., > 16 or 0) must return []byte(envelope) verbatim. NO error.
func TestDecryptToken_BadPadding_ReturnsCiphertextAsPlaintext_NoError(t *testing.T) {
	// Craft a base64 envelope of length 32 raw bytes (16 IV + 16 CT). The CT
	// is all-zero — when decrypted under our test key, the last byte of the
	// recovered plaintext is some essentially-random value that will almost
	// never be a valid PKCS#7 padding byte. The fall-back path returns the
	// envelope verbatim.
	raw := make([]byte, 32) // all zeros
	env := base64.RawURLEncoding.EncodeToString(raw)
	got := decryptToken(env, hmacSecretForTokens)
	// Either AMEND-3 fall-back fires (length unchanged → bytes-equal) OR the
	// padding happens to validate by chance (1-in-256). We assert the
	// fall-back path: the returned bytes match the envelope verbatim.
	if string(got) != env {
		// Allow the 1/256 false-positive padding case to be acknowledged:
		// log it, but still fail since the test is deterministic relative to
		// the hmacSecret + the zero CT (the AES decrypt output is determined
		// by the key, not random). If this test ever fires the failure
		// branch, retune the constants until the padding-error path is
		// exercised reliably.
		t.Fatalf("AMEND-3 fall-back mismatch:\n got: %q\nwant: %q (verbatim envelope bytes)", got, env)
	}
}

// TestDecryptToken_TruncatedEnvelope_ReturnsCiphertextAsPlaintext_NoError pins
// AMEND-3 at the truncated-envelope path: an envelope whose base64-decoded
// length is < 16 (i.e., not enough bytes for even an IV) must return
// []byte(envelope) verbatim. NO error.
func TestDecryptToken_TruncatedEnvelope_ReturnsCiphertextAsPlaintext_NoError(t *testing.T) {
	// 8 zero bytes → base64.RawURLEncoding 11 chars.
	short := base64.RawURLEncoding.EncodeToString(make([]byte, 8))
	got := decryptToken(short, hmacSecretForTokens)
	if string(got) != short {
		t.Fatalf("AMEND-3 truncated-envelope fall-back mismatch: got=%q want=%q", got, short)
	}
	// Edge case: empty string → AMEND-3 fall-back returns []byte("").
	got2 := decryptToken("", hmacSecretForTokens)
	if len(got2) != 0 {
		t.Fatalf("AMEND-3 empty-envelope fall-back: got=%q want=(empty)", got2)
	}
}

// TestDecryptToken_WrongHmacSecret_GarbageOutputsLikely_NoError asserts that
// decryption with a wrong hmacSecret either produces garbage that fails the
// PKCS#7 padding check (→ AMEND-3 fall-back returns envelope verbatim) OR by
// chance produces a valid-padded garbage plaintext (extremely rare; ~1/256
// per try). Either way: NO error is returned.
func TestDecryptToken_WrongHmacSecret_GarbageOutputsLikely_NoError(t *testing.T) {
	pt := []byte("the-real-plaintext")
	env := encryptToken(pt, hmacSecretForTokens)
	wrong := []byte("phase-20-test-hmac-key-2") // different from hmacSecretForTokens
	got := decryptToken(env, wrong)
	// The function must NEVER panic and must NEVER return nil; the contract
	// is a non-nil []byte. The downstream HMAC validation rejects naturally
	// (the recovered garbage will not produce a valid HMAC).
	if got == nil {
		t.Fatalf("decryptToken returned nil for wrong-secret case; expected non-nil bytes per AMEND-3")
	}
	// We do NOT assert byte-equality with `pt` — the wrong key produces
	// either garbage (AMEND-3 fall-back to envelope) or by chance valid-padded
	// garbage. The downstream HMAC step is the rejecter.
	if bytes.Equal(got, pt) {
		t.Fatalf("decryptToken returned original plaintext with WRONG secret — KDF or AES broken")
	}
}

// TestDecryptToken_AmbiguousFallback_PlaintextLooksLikeEnvelope_StillFallsBack
// pins the edge case where the "envelope" happens to be a string that itself
// looks like valid base64. The AMEND-3 fall-back must still fire — i.e., the
// returned bytes are the envelope verbatim, NOT some accidentally-decoded
// alternative shape.
func TestDecryptToken_AmbiguousFallback_PlaintextLooksLikeEnvelope_StillFallsBack(t *testing.T) {
	// "QUFBQUFBQUFBQUFBQUFBQQ" is valid base64.RawURLEncoding (decodes to
	// 16 'A' bytes). But that's NOT a valid AES-CBC envelope (because the
	// 16 'A' bytes have no separable IV + CT — the format requires at
	// least 16 IV + 16 CT = 32 bytes raw).
	env := "QUFBQUFBQUFBQUFBQUFBQQ"
	raw, err := base64.RawURLEncoding.DecodeString(env)
	if err != nil {
		t.Fatalf("test setup: envelope is not valid base64.RawURLEncoding: %v", err)
	}
	if got, want := len(raw), 16; got != want {
		t.Fatalf("test setup: expected 16 raw bytes, got %d", got)
	}
	got := decryptToken(env, hmacSecretForTokens)
	// Per AMEND-3: returns envelope bytes verbatim. The envelope happens to
	// be valid base64 — but decryption fails because len(raw)==16 means
	// there's only an IV, no CT.
	if string(got) != env {
		t.Fatalf("AMEND-3 ambiguous-fallback mismatch: got=%q want=%q (verbatim envelope)", got, env)
	}
}

// TestDecryptToken_BlockSizeNotMultiple_ReturnsCiphertextAsPlaintext_NoError
// pins AMEND-3 at the CT-not-block-multiple path: an envelope whose base64-
// decoded length is >= 16 but (len-16) is NOT a positive multiple of 16
// (i.e., the CT after IV-stripping is not a clean block multiple) must
// return []byte(envelope) verbatim.
func TestDecryptToken_BlockSizeNotMultiple_ReturnsCiphertextAsPlaintext_NoError(t *testing.T) {
	// 16 IV + 7 CT (NOT a block multiple) = 23 raw bytes.
	raw := make([]byte, 23)
	env := base64.RawURLEncoding.EncodeToString(raw)
	got := decryptToken(env, hmacSecretForTokens)
	if string(got) != env {
		t.Fatalf("AMEND-3 non-block-multiple fall-back: got=%q want=%q", got, env)
	}
}

// TestEncryptToken_NilHmacSecret_DerivesFromEmptyKey pins the corner case:
// nil/empty hmacSecret is acceptable input; the KDF is SHA-256([]byte{}) =
// a well-known 32-byte constant. NO panic; the round-trip succeeds because
// the same nil hmacSecret derives the same key on both ends. Documents the
// implementation choice — operators who supply an empty hmac_secret get a
// deterministic but PUBLIC AES key (this is operator-error-protected by the
// upstream HCM-parse-time PARSE-REJECT for empty SDS secrets, NOT by
// tokens.go).
func TestEncryptToken_NilHmacSecret_DerivesFromEmptyKey(t *testing.T) {
	pt := []byte("nil-secret-test")
	env := encryptToken(pt, nil)
	got := decryptToken(env, nil)
	if !bytes.Equal(got, pt) {
		t.Fatalf("nil-hmac round-trip mismatch: got=%q want=%q", got, pt)
	}
	// Empty-slice variant — equivalent to nil per SHA-256 semantics.
	envE := encryptToken(pt, []byte{})
	gotE := decryptToken(envE, []byte{})
	if !bytes.Equal(gotE, pt) {
		t.Fatalf("empty-hmac round-trip mismatch: got=%q want=%q", gotE, pt)
	}
	// Cross-check: nil-encrypted envelope decrypts under empty-slice secret
	// (both derive to SHA-256([])[:32]).
	if got2 := decryptToken(env, []byte{}); !bytes.Equal(got2, pt) {
		t.Fatalf("nil-vs-empty-hmac cross-decrypt mismatch: got=%q want=%q", got2, pt)
	}
}

// TestDecryptToken_DisableEncryption_SkipPath_DocumentsConsumerBehavior
// documents (rather than exercises) the consumer-level skip-path per ADR-0182
// §Context + SPEC §2.15 + §3.3: when `compiledConfig.disableTokenEncryption`
// is true, the consumer (the caller in callback.go / decode_headers.go) does
// NOT invoke encryptToken/decryptToken — cookie values are stored + read
// verbatim. This test verifies that aspect by exercising the contract: the
// tokens.go functions themselves have NO disable_token_encryption awareness
// — they always encrypt/decrypt. The skip-path is exclusively at the caller.
//
// The test is a no-op assertion on the contract — present for audit-trail
// completeness and as a regression-canary against any future "convenience"
// refactor that moved the skip into tokens.go.
func TestDecryptToken_DisableEncryption_SkipPath_DocumentsConsumerBehavior(t *testing.T) {
	// The bare functions accept any plaintext / envelope and always
	// encrypt/decrypt. They do NOT consult any disable-flag.
	pt := []byte("plaintext-when-disabled-the-caller-would-not-call-this")
	env := encryptToken(pt, hmacSecretForTokens)
	// The tokens.go contract: always encrypts. If skip were ever moved here,
	// env would equal string(pt). Confirm it does NOT.
	if env == string(pt) {
		t.Fatalf("encryptToken acted as pass-through; expected encrypted envelope. "+
			"The disable_token_encryption skip-path must live at the CALLER (per ADR-0182 + §2.15), "+
			"NOT in tokens.go. got=%q", env)
	}
	// Round-trip succeeds (canonical encrypt path is exercised).
	if got := decryptToken(env, hmacSecretForTokens); !bytes.Equal(got, pt) {
		t.Fatalf("contract-check round-trip mismatch: got=%q want=%q", got, pt)
	}
}

// -----------------------------------------------------------------------------
// Race-test group — TestAesKeySwap_Concurrent_* per phase-20 planner-time D4.
//
// These tests validate the atomic.Pointer[[32]byte] discipline that the
// compiledConfig.aesKey (Task 11) will use for the sdsfile-driven reload
// path: a key swap during in-flight encrypt/decrypt MUST NOT race against
// readers. The key is loaded via atomic.Pointer.Load() inside each goroutine;
// the swap goroutine calls atomic.Pointer.Store() concurrently. All
// invariants must hold under `go test -race`.
//
// Note: tokens.go's encryptToken + decryptToken take `hmacSecret []byte` as a
// parameter (NOT an atomic.Pointer), so the atomic-pointer discipline lives at
// the CALLER level. These tests simulate the caller pattern.
// -----------------------------------------------------------------------------

// TestAesKeySwap_Concurrent_DuringEncrypt exercises N encryptor goroutines +
// 1 swapper goroutine. Each encryptor reads the current key via
// atomic.Pointer.Load(), encrypts a plaintext, and verifies that decryption
// with the SAME loaded key recovers the plaintext byte-exact. The swapper
// rotates the atomic pointer between two keys. Under `-race`, atomic.Pointer
// must serialize cleanly — no torn pointer reads, no data races.
func TestAesKeySwap_Concurrent_DuringEncrypt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping race-stress test in -short mode")
	}
	var key atomic.Pointer[[]byte]
	keyA := []byte("key-A-phase-20-aes-swap-test--")
	keyB := []byte("key-B-phase-20-aes-swap-test--")
	key.Store(&keyA)

	// Use TWO WaitGroups: one for the encryptors, one for the swapper.
	// This lets the test wait for encryptors to complete, then stop the
	// swapper independently.
	var encWG sync.WaitGroup
	var swapWG sync.WaitGroup
	const encryptors = 4
	const iterationsPerGoroutine = 200
	stop := make(chan struct{})

	for i := 0; i < encryptors; i++ {
		encWG.Add(1)
		go func() {
			defer encWG.Done()
			pt := []byte("concurrent-encrypt-plaintext-payload")
			for j := 0; j < iterationsPerGoroutine; j++ {
				k := key.Load() // atomic read
				env := encryptToken(pt, *k)
				// Decrypt with the SAME loaded key — this MUST round-trip.
				if got := decryptToken(env, *k); !bytes.Equal(got, pt) {
					t.Errorf("round-trip mismatch under concurrent swap: got=%q want=%q", got, pt)
					return
				}
			}
		}()
	}

	// Swapper goroutine rotates the key between A and B until told to stop.
	swapWG.Add(1)
	go func() {
		defer swapWG.Done()
		current := &keyA
		for {
			select {
			case <-stop:
				return
			default:
			}
			if current == &keyA {
				key.Store(&keyB)
				current = &keyB
			} else {
				key.Store(&keyA)
				current = &keyA
			}
		}
	}()

	// Wait for encryptors to finish all iterations BEFORE telling the
	// swapper to stop — this guarantees the swapper races against in-flight
	// encrypt+decrypt for the full iteration window. The race detector's
	// promise is that ANY race during this window surfaces.
	encWG.Wait()
	close(stop)
	swapWG.Wait()
}

// TestAesKeySwap_Concurrent_DuringDecrypt exercises N decryptor goroutines +
// 1 swapper goroutine. Each decryptor reads the current key via atomic.Pointer.Load(),
// then encrypts and decrypts within the SAME atomic snapshot — verifying that
// the key snapshot is consistent across the encrypt/decrypt pair even as
// concurrent swaps happen. Under `-race`, atomic.Pointer must serialize cleanly.
func TestAesKeySwap_Concurrent_DuringDecrypt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping race-stress test in -short mode")
	}
	var key atomic.Pointer[[]byte]
	keyA := []byte("key-A-phase-20-aes-decrypt----")
	keyB := []byte("key-B-phase-20-aes-decrypt----")
	key.Store(&keyA)

	var decWG sync.WaitGroup
	var swapWG sync.WaitGroup
	const decryptors = 4
	const iterationsPerGoroutine = 200
	stop := make(chan struct{})

	// Pre-compute envelopes under both keys for the decryptor goroutines to
	// chase. The decryptor reads the current key snapshot, then decrypts the
	// matching pre-computed envelope.
	pt := []byte("concurrent-decrypt-payload")
	envA := encryptToken(pt, keyA)
	envB := encryptToken(pt, keyB)

	for i := 0; i < decryptors; i++ {
		decWG.Add(1)
		go func() {
			defer decWG.Done()
			for j := 0; j < iterationsPerGoroutine; j++ {
				k := key.Load() // atomic read
				var env string
				if bytes.Equal(*k, keyA) {
					env = envA
				} else {
					env = envB
				}
				if got := decryptToken(env, *k); !bytes.Equal(got, pt) {
					t.Errorf("decrypt mismatch under concurrent swap: got=%q want=%q", got, pt)
					return
				}
			}
		}()
	}

	swapWG.Add(1)
	go func() {
		defer swapWG.Done()
		current := &keyA
		for {
			select {
			case <-stop:
				return
			default:
			}
			if current == &keyA {
				key.Store(&keyB)
				current = &keyB
			} else {
				key.Store(&keyA)
				current = &keyA
			}
		}
	}()

	// Wait for decryptors to finish; then stop the swapper.
	decWG.Wait()
	close(stop)
	swapWG.Wait()
}

// TestAesKeySwap_Concurrent_ReadAfterSwapObservesNewKey asserts the atomic-
// pointer publishing invariant: after Store(newKey) returns, all subsequent
// Load() calls observe newKey (or a strictly-later swap). Combined with the
// AMEND-3 fall-back: an envelope encrypted under keyA + decrypted under keyB
// returns the envelope verbatim (decrypt-failure → ciphertext-as-plaintext).
// This verifies the swap is OBSERVABLE, not that the application does
// anything in particular with the fall-back bytes.
func TestAesKeySwap_Concurrent_ReadAfterSwapObservesNewKey(t *testing.T) {
	var key atomic.Pointer[[]byte]
	keyA := []byte("key-A-readafterswap-test------")
	keyB := []byte("key-B-readafterswap-test------")
	key.Store(&keyA)

	pt := []byte("readafterswap-plaintext")
	envA := encryptToken(pt, keyA)

	// Initial state: load returns keyA; decrypt round-trips.
	{
		k := key.Load()
		if !bytes.Equal(*k, keyA) {
			t.Fatalf("initial load did not return keyA")
		}
		if got := decryptToken(envA, *k); !bytes.Equal(got, pt) {
			t.Fatalf("initial decrypt round-trip mismatch: got=%q want=%q", got, pt)
		}
	}

	// Swap to keyB.
	key.Store(&keyB)

	// Post-swap: load returns keyB.
	k2 := key.Load()
	if !bytes.Equal(*k2, keyB) {
		t.Fatalf("post-swap load did not return keyB")
	}

	// Decrypt envA (encrypted under keyA) with keyB → either garbage (AMEND-3
	// fall-back returns envelope) or by chance valid-padded garbage. Either
	// way, NOT the original plaintext.
	got := decryptToken(envA, *k2)
	if bytes.Equal(got, pt) {
		t.Fatalf("decrypt with WRONG key returned original plaintext — atomic swap or KDF broken")
	}
	// Per AMEND-3 the fall-back returns envelope bytes verbatim (or
	// 1-in-256-chance random plaintext). We do NOT assert which — we assert
	// only that the swap is observable + the wrong-key decrypt is not the
	// original plaintext.
	if got == nil {
		t.Fatalf("decryptToken returned nil; expected non-nil bytes per AMEND-3")
	}
}
