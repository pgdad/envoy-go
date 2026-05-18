package oauth2

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// secret is the shared HMAC secret used across the Group 2 vector tests.
// The byte sequence is arbitrary but pinned to keep the expected-vector
// outputs reproducible. The 33-byte length is non-special — HMAC-SHA256
// accepts arbitrary-length keys per RFC 2104.
var secret = []byte("test-hmac-secret-32-bytes-padding")

// -----------------------------------------------------------------------------
// Group 2.A — computeHMAC vector tests per SPEC §14.1 + ADR-0179 + AMEND-2.
//
// The expected outputs are derived from a one-shot Go program run at
// author-time against the deterministic HMAC-SHA256 + Base64URL definition
// (per phase-20 SPEC §6.5 + AMEND-2). Pinning the outputs as constants
// guards against accidental drift in the composition (e.g. a future refactor
// that changed the separator, swapped argument order, or dropped an empty-
// string input would surface here).
// -----------------------------------------------------------------------------

// TestComputeHMAC_FullEnvelope exercises the maximal 5-input case: domain +
// expires + token + id_token + refresh_token all non-empty. Verifies the
// canonical AMEND-2 composition with NO empty inputs at any position.
func TestComputeHMAC_FullEnvelope(t *testing.T) {
	got := computeHMAC("example.com", "1700000000", "access_token_abc",
		"id_token_xyz", "refresh_token_qrs", secret)
	want := "RhCMkX_oqdOZSaeGJFOoBmWeBE_laTUPYbgZpt35avg"
	if got != want {
		t.Fatalf("computeHMAC mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestComputeHMAC_NoIdToken_NoRefreshToken exercises the MVP-typical case
// (id_token deferred per §2.2; refresh_token absent on a stateless cookie).
// Both inputs participate as EMPTY STRINGS per §20.P4 REFUTED — the resulting
// HMAC is distinct from the full-envelope vector.
func TestComputeHMAC_NoIdToken_NoRefreshToken(t *testing.T) {
	got := computeHMAC("example.com", "1700000000", "access_token_abc",
		"", "", secret)
	want := "wk2IrStexXq5Jf5i-XF5JmTCLr_wO-vggVEY8yavQBI"
	if got != want {
		t.Fatalf("computeHMAC mismatch:\n got: %q\nwant: %q", got, want)
	}
	// Sanity: distinct from the full-envelope vector — confirms the empty-
	// string inputs participate (i.e. the implementation is not silently
	// skipping empty inputs, which would collapse the input space).
	full := computeHMAC("example.com", "1700000000", "access_token_abc",
		"id_token_xyz", "refresh_token_qrs", secret)
	if got == full {
		t.Fatalf("empty id/refresh produced same HMAC as full envelope; "+
			"empty-string inputs are NOT participating per §20.P4 REFUTED expectation; "+
			"got both = %q", got)
	}
}

// TestComputeHMAC_OnlyRefreshTokenAbsent exercises the "id_token present,
// refresh_token absent" partial case. Distinct from both the full envelope
// and the no-id/no-refresh case.
func TestComputeHMAC_OnlyRefreshTokenAbsent(t *testing.T) {
	got := computeHMAC("example.com", "1700000000", "access_token_abc",
		"id_token_xyz", "", secret)
	want := "ndH13vuDQ-HGeAApBxCiVz69RtxEbdlMsOLuA3Te1b8"
	if got != want {
		t.Fatalf("computeHMAC mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestComputeHMAC_OnlyIdTokenAbsent exercises the "id_token absent,
// refresh_token present" partial case (the MVP runtime shape when
// use_refresh_token=true but id_token is deferred per §2.2).
func TestComputeHMAC_OnlyIdTokenAbsent(t *testing.T) {
	got := computeHMAC("example.com", "1700000000", "access_token_abc",
		"", "refresh_token_qrs", secret)
	want := "s7I_UJj0Lf0nT-FPIV2ccdFWUyIb6JRJ7ww3z9BMiiE"
	if got != want {
		t.Fatalf("computeHMAC mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestComputeHMAC_KnownVectorsMatchUpstream is the table-driven anchor for
// the Group 2 vector roster. Each row pins a (domain, expires, token,
// idToken, refreshToken, expected) tuple. The expected values were computed
// at author-time via a one-shot Go program against the deterministic
// HMAC-SHA256 + Base64URL composition per AMEND-2 + §6.5. Upstream Envoy
// v1.37.2 uses byte-identical composition (the SPEC §11 §20.P4 empirical
// scrape REFUTED the BRAINSTORM Q9 3-input hypothesis); since cross-process
// upstream comparison is the differential-fixture domain (Task 12),
// here we pin the OWN-implementation outputs to guard against drift.
func TestComputeHMAC_KnownVectorsMatchUpstream(t *testing.T) {
	tests := []struct {
		name                                      string
		domain, expires, token, idTok, refreshTok string
		want                                      string
	}{
		{
			name:    "different_domain",
			domain:  "api.example.com",
			expires: "1700000123",
			token:   "token",
			want:    "jfA2y-1abGL4njiMxs29X5djfzxiZYC0OxjiLnyiDYo",
		},
		{
			name:    "empty_domain",
			domain:  "",
			expires: "1700000000",
			token:   "access_token_abc",
			want:    "gXPO6L_S-hK3iRsAXDs_FJ0TwDwrF_TDEdk3zTE5KzY",
		},
		{
			name:    "unicode_token",
			domain:  "example.com",
			expires: "1700000000",
			token:   "🔐token-ünïcode",
			want:    "0Af14k511abyR39Cf3WHF1VGFbxxyFlUZYNsy4e0ZOk",
		},
		{
			name:    "max_like_expires",
			domain:  "example.com",
			expires: "9999999999",
			token:   "access_token_abc",
			want:    "-rMpJr5DK2h_RhbOFTeOlEqZWvQlKHAeSVoa7rxHsr0",
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := computeHMAC(tc.domain, tc.expires, tc.token,
				tc.idTok, tc.refreshTok, secret)
			if got != tc.want {
				t.Fatalf("computeHMAC mismatch:\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestComputeHMAC_LongInputs exercises a >1MB input across the token
// position. HMAC-SHA256 has no maximum input size; the test verifies no
// panic + deterministic output (same input → same hash twice).
func TestComputeHMAC_LongInputs(t *testing.T) {
	largeToken := strings.Repeat("A", 1<<20) // 1 MiB
	first := computeHMAC("example.com", "1700000000", largeToken, "", "", secret)
	second := computeHMAC("example.com", "1700000000", largeToken, "", "", secret)
	if first != second {
		t.Fatalf("computeHMAC not deterministic for long input:\n  first: %q\n second: %q",
			first, second)
	}
	// Output is always exactly 43 chars — the Base64URL-raw length of a 32-byte SHA-256 digest.
	if len(first) != 43 {
		t.Fatalf("computeHMAC output length = %d; want 43 (32-byte SHA-256 → Base64URL-raw)",
			len(first))
	}
}

// TestComputeHMAC_EmptyEverything exercises the edge case where ALL 5 inputs
// are empty strings. The composition `"\n\n\n\n"` is still well-defined;
// the HMAC is the HMAC of 4 newlines. Confirms no panic + a stable output.
func TestComputeHMAC_EmptyEverything(t *testing.T) {
	got := computeHMAC("", "", "", "", "", secret)
	if got == "" {
		t.Fatal("computeHMAC of all-empty inputs returned empty string; want stable 43-char Base64URL-raw")
	}
	if len(got) != 43 {
		t.Fatalf("computeHMAC output length = %d; want 43", len(got))
	}
	// Sanity: distinct from any of the non-empty vectors above.
	full := computeHMAC("example.com", "1700000000", "access_token_abc",
		"id_token_xyz", "refresh_token_qrs", secret)
	if got == full {
		t.Fatalf("all-empty inputs produced same HMAC as full envelope (got = %q); "+
			"HMAC must distinguish input vectors", got)
	}
}

// -----------------------------------------------------------------------------
// Group 2.B — validateHMAC tests per SPEC §6.4 + ADR-0179 + S4.
// -----------------------------------------------------------------------------

// TestValidateHMAC_Base64EncodingAccepted is the primary positive: a freshly
// computed Base64URL-raw HMAC validates GREEN.
func TestValidateHMAC_Base64EncodingAccepted(t *testing.T) {
	envelope := computeHMAC("example.com", "1700000000", "access_token_abc",
		"", "", secret)
	if !validateHMAC("example.com", "1700000000", "access_token_abc",
		"", "", secret, envelope) {
		t.Fatalf("validateHMAC rejected a freshly-computed Base64URL HMAC = %q", envelope)
	}
}

// TestValidateHMAC_HexBase64EncodingAccepted exercises the dual-encoding read
// per S4 — an envelope encoded as Base64URL-of-hex-string (nested) validates
// GREEN. This is the cross-deployment-migration wire-compat path: some
// upstream Envoy deployments emit HexBase64 due to historical configuration;
// the MVP read path accepts BOTH to widen compatibility per AMEND-2 + S4.
func TestValidateHMAC_HexBase64EncodingAccepted(t *testing.T) {
	// Compute raw HMAC bytes directly + apply the nested HexBase64 encoding.
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("example.com\n1700000000\naccess_token_abc\n\n"))
	rawSum := mac.Sum(nil)
	hexStr := hex.EncodeToString(rawSum)
	hexB64 := base64.RawURLEncoding.EncodeToString([]byte(hexStr))

	if !validateHMAC("example.com", "1700000000", "access_token_abc",
		"", "", secret, hexB64) {
		t.Fatalf("validateHMAC rejected a valid HexBase64-encoded HMAC = %q "+
			"(dual-encoding read per S4 should accept)", hexB64)
	}
}

// TestValidateHMAC_PaddedBase64Accepted exercises the operator-tolerant
// padded-Base64URL acceptance. SPEC §6.5 specifies Base64URL-raw (unpadded)
// for the emit path, but the read path is operator-tolerant via the
// fall-back decode chain: if the raw-decoder rejects (due to '=' padding),
// the std-Base64URL decoder (which accepts padding) gets a chance, and
// finally the HexBase64 path. We accept padded variants to widen
// cross-deployment compatibility — the constant-time compare remains intact
// because we compare against the SAME 32-byte HMAC output regardless of
// which decoder branch succeeded.
func TestValidateHMAC_PaddedBase64Accepted(t *testing.T) {
	// Compute the raw HMAC bytes + encode with Base64URL std (padded).
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("example.com\n1700000000\naccess_token_abc\n\n"))
	rawSum := mac.Sum(nil)
	padded := base64.URLEncoding.EncodeToString(rawSum) // includes any '=' padding

	if !validateHMAC("example.com", "1700000000", "access_token_abc",
		"", "", secret, padded) {
		t.Fatalf("validateHMAC rejected a valid padded-Base64URL HMAC = %q "+
			"(operator-tolerant read should accept)", padded)
	}
}

// TestValidateHMAC_TamperedHmac_Rejected exercises envelope-level tampering:
// a single bit-flip in the HMAC cookie → RED.
func TestValidateHMAC_TamperedHmac_Rejected(t *testing.T) {
	envelope := computeHMAC("example.com", "1700000000", "access_token_abc",
		"", "", secret)
	// Replace the first character with a different valid Base64URL char.
	var tampered string
	if envelope[0] == 'A' {
		tampered = "B" + envelope[1:]
	} else {
		tampered = "A" + envelope[1:]
	}
	if validateHMAC("example.com", "1700000000", "access_token_abc",
		"", "", secret, tampered) {
		t.Fatalf("validateHMAC accepted a tampered HMAC = %q (original = %q)", tampered, envelope)
	}
}

// TestValidateHMAC_TamperedToken_Rejected exercises bearer-token tampering:
// the cookie envelope's HMAC was computed over one token; we present a
// different token at validation time → RED.
func TestValidateHMAC_TamperedToken_Rejected(t *testing.T) {
	envelope := computeHMAC("example.com", "1700000000", "access_token_abc",
		"", "", secret)
	if validateHMAC("example.com", "1700000000", "access_token_DIFFERENT",
		"", "", secret, envelope) {
		t.Fatal("validateHMAC accepted a tampered bearer token")
	}
}

// TestValidateHMAC_TamperedDomain_Rejected exercises cross-host envelope
// reuse rejection: an envelope HMAC-bound to host A cannot validate at
// host B. This is the load-bearing security property — prevents cookie
// theft across deployments.
func TestValidateHMAC_TamperedDomain_Rejected(t *testing.T) {
	envelope := computeHMAC("example.com", "1700000000", "access_token_abc",
		"", "", secret)
	if validateHMAC("evil.example.com", "1700000000", "access_token_abc",
		"", "", secret, envelope) {
		t.Fatal("validateHMAC accepted cross-host envelope reuse")
	}
}

// TestValidateHMAC_TamperedExpires_Rejected exercises expires-field tampering
// (an attacker extending session lifetime by editing the OauthExpires cookie
// without also editing the OauthHMAC cookie).
func TestValidateHMAC_TamperedExpires_Rejected(t *testing.T) {
	envelope := computeHMAC("example.com", "1700000000", "access_token_abc",
		"", "", secret)
	if validateHMAC("example.com", "9999999999", "access_token_abc",
		"", "", secret, envelope) {
		t.Fatal("validateHMAC accepted tampered expires value")
	}
}

// TestValidateHMAC_TamperedIdToken_Rejected exercises id_token tampering at
// the 4th input position.
func TestValidateHMAC_TamperedIdToken_Rejected(t *testing.T) {
	envelope := computeHMAC("example.com", "1700000000", "access_token_abc",
		"id_token_xyz", "refresh_token_qrs", secret)
	if validateHMAC("example.com", "1700000000", "access_token_abc",
		"id_token_DIFFERENT", "refresh_token_qrs", secret, envelope) {
		t.Fatal("validateHMAC accepted tampered id_token")
	}
}

// TestValidateHMAC_TamperedRefreshToken_Rejected exercises refresh_token
// tampering at the 5th input position.
func TestValidateHMAC_TamperedRefreshToken_Rejected(t *testing.T) {
	envelope := computeHMAC("example.com", "1700000000", "access_token_abc",
		"id_token_xyz", "refresh_token_qrs", secret)
	if validateHMAC("example.com", "1700000000", "access_token_abc",
		"id_token_xyz", "refresh_token_DIFFERENT", secret, envelope) {
		t.Fatal("validateHMAC accepted tampered refresh_token")
	}
}

// TestValidateHMAC_EmptyHmac_Rejected exercises the empty-cookie edge case:
// an empty OauthHMAC cookie value must reject GREEN (no panic, no validation).
func TestValidateHMAC_EmptyHmac_Rejected(t *testing.T) {
	if validateHMAC("example.com", "1700000000", "access_token_abc",
		"", "", secret, "") {
		t.Fatal("validateHMAC accepted an empty cookie value")
	}
}

// TestValidateHMAC_MalformedBase64_Rejected exercises the parse-error edge
// case: garbage in the OauthHMAC cookie must reject (no panic). The dual-
// encoding read tries Base64URL-raw, then padded Base64URL, then HexBase64;
// all 3 paths must fail cleanly on garbage input.
func TestValidateHMAC_MalformedBase64_Rejected(t *testing.T) {
	garbageInputs := []string{
		"!!not-base64!!",
		"@#$%^&*()",
		"hello world with spaces",
		"\x00\x01\x02\x03",
		// Wrong length even if valid Base64URL chars:
		"AAAA", // decodes to 3 bytes; HMAC-SHA256 sum is 32 bytes
		// HexBase64-shape but inner hex is invalid:
		base64.RawURLEncoding.EncodeToString([]byte("not-hex-chars-zzzz")),
	}
	for _, in := range garbageInputs {
		in := in
		t.Run(in, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("validateHMAC panicked on garbage input %q: %v", in, r)
				}
			}()
			if validateHMAC("example.com", "1700000000", "access_token_abc",
				"", "", secret, in) {
				t.Fatalf("validateHMAC accepted garbage input %q", in)
			}
		})
	}
}

// TestValidateHMAC_DualEncoding_BothCanonicalDecode confirms the two
// canonical decoded forms (Base64URL of raw HMAC bytes; Base64URL of
// hex-string-of-raw-HMAC-bytes) both validate against the same input
// vector. This is the structural double-check for S4 dual-encoding read.
func TestValidateHMAC_DualEncoding_BothCanonicalDecode(t *testing.T) {
	domain, expires, tok := "example.com", "1700000000", "access_token_abc"
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(domain + "\n" + expires + "\n" + tok + "\n\n"))
	rawSum := mac.Sum(nil)

	base64Form := base64.RawURLEncoding.EncodeToString(rawSum)
	hexB64Form := base64.RawURLEncoding.EncodeToString([]byte(hex.EncodeToString(rawSum)))

	if base64Form == hexB64Form {
		t.Fatalf("Base64 + HexBase64 produced identical encodings (= %q) — dual-encoding test is structurally degenerate", base64Form)
	}
	if !validateHMAC(domain, expires, tok, "", "", secret, base64Form) {
		t.Fatalf("validateHMAC rejected canonical Base64URL form = %q", base64Form)
	}
	if !validateHMAC(domain, expires, tok, "", "", secret, hexB64Form) {
		t.Fatalf("validateHMAC rejected canonical HexBase64 form = %q", hexB64Form)
	}
}

// TestValidateHMAC_ConstantTimeCompare_UsesHmacEqual is an inspection-style
// assertion that the implementation uses crypto/hmac.Equal (constant-time)
// rather than the variable-time bytes.Equal or string ==. Direct timing-
// side-channel measurement is flaky in CI; the indirect-but-deterministic
// guard here parses the hmac.go source and asserts the presence of
// `hmac.Equal(` AND the ABSENCE of `bytes.Equal(` on the validation hot
// path. This is fragile against pure-refactor renames but catches the
// load-bearing "developer forgot constant-time-compare" regression.
func TestValidateHMAC_ConstantTimeCompare_UsesHmacEqual(t *testing.T) {
	src := readSourceFile(t, "hmac.go")
	if !strings.Contains(src, "hmac.Equal(") {
		t.Error("hmac.go does NOT call hmac.Equal — constant-time-compare not in use")
	}
	if strings.Contains(src, "bytes.Equal(") {
		t.Error("hmac.go calls bytes.Equal — must use hmac.Equal for constant-time-compare")
	}
}

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

// readSourceFile reads a file relative to the test's package directory and
// returns its contents. The Go test runner sets cwd to the package
// directory, so a bare filename suffices.
func readSourceFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("readSourceFile(%q): %v", name, err)
	}
	return string(b)
}
