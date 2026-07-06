package lua

// crypto_test.go — Task 12 (phase 22.2 IMPL) crypto bridge tests per
// SPEC §3.4 + §6 arm-22 + §11.2 D7 + AMEND-22.2-1 + AMEND-22.2-2 + D8 +
// PLAN Task 12 acceptance.
//
// Coverage:
//
//   - Test_Base64Escape_byte_output_matches_absl_Base64Escape — verifies
//     Go encoding/base64.StdEncoding byte-output matches absl::Base64Escape
//     standard-padding wire shape (upstream-parity per AMEND-22.2-1).
//   - Test_Base64Decode_roundtrip_with_Base64Escape — round-trip property
//     across :base64Escape → :base64Decode (envoy-go-strict per D8).
//   - Test_Sha256_byte_output_vectors — known test vectors (envoy-go-strict
//     per D8; hex-encoded lower-case wire shape).
//   - Test_Sha512_byte_output_vectors — known test vectors (envoy-go-strict
//     per D8; hex-encoded lower-case wire shape).
//   - Test_ImportPublicKey_parses_RSA_PKIX_PEM — RSA PKIX SubjectPublicKeyInfo
//     PEM parses cleanly via crypto/x509.ParsePKIXPublicKey.
//   - Test_ImportPublicKey_parses_ECDSA_P256_PKIX_PEM — ECDSA P-256 PKIX
//     SubjectPublicKeyInfo PEM parses cleanly.
//   - Test_ImportPublicKey_parses_Ed25519_PKIX_PEM — Ed25519 PKIX
//     SubjectPublicKeyInfo PEM parses cleanly.
//   - Test_ImportPublicKey_invalid_PEM_raises_arm22_byte_stable_wording —
//     arm-22 byte-stable runtime-reject wording per SPEC §6 + W2.
//   - Test_PublicKeyWrapper_get_returns_key_bytes — the wrapper userdata
//     :get() method returns the DER-encoded SubjectPublicKeyInfo bytes
//     (MIMICKING upstream wrappers.h:415-427 scope per D8-sub closure).
//   - Test_VerifySignature_happy_RSA_SHA256 — canned RSA-2048 + SHA-256
//     signed payload verifies true.
//   - Test_VerifySignature_invalid_signature_returns_false — tampered
//     signature returns false (no Lua error).
//   - Test_VerifySignature_unsupported_hash_algo_returns_false — unknown
//     hash algo string returns false (no Lua error).
//   - Test_VerifySignature_calling_convention_pinned_4_args — pins the
//     canonical script form `rh:verifySignature(hash, pub, sig, text)`
//     (upstream lua_filter.cc:611 calling convention per D8-sub closure).

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"

	luaprim "github.com/pgdad/envoy-go/internal/lua"
)

// newCryptoBridgeVM constructs a per-test *VM with the bridge metatables
// installed (including the PublicKeyWrapper metatable) + a request_handle
// userdata bound to the Lua global `rh`. Mirrors the newBridgedVM helper
// pattern but adds installPublicKeyWrapperMetatable so :importPublicKey
// returns userdata with a working :get method.
func newCryptoBridgeVM(t *testing.T) *luaprim.VM {
	t.Helper()
	vm := luaprim.NewVM()
	t.Cleanup(vm.Close)
	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installPublicKeyWrapperMetatable(L)
	installPairsShim(L)
	ctx := &requestHandleContext{headers: http.Header{}}
	ud := L.NewUserData()
	ud.Value = ctx
	L.SetMetatable(ud, L.GetTypeMetatable(requestHandleTypeName))
	L.SetGlobal("rh", ud)
	return vm
}

// ---------------------------------------------------------------------
// :base64Escape — upstream-parity per AMEND-22.2-1
// ---------------------------------------------------------------------

func Test_Base64Escape_byte_output_matches_absl_Base64Escape(t *testing.T) {
	// absl::Base64Escape uses standard padding (the "+/" alphabet + "="
	// padding). Go encoding/base64.StdEncoding produces byte-identical
	// output. Verify across N inputs spanning byte-aligned + non-aligned
	// + binary + NUL-bearing cases.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"hello", "hello", "aGVsbG8="},
		{"hi", "hi", "aGk="},
		// "Hello, World!" → standard padding output (absl-parity).
		{"hello_world_punc", "Hello, World!", "SGVsbG8sIFdvcmxkIQ=="},
		// Binary bytes including NUL.
		{"binary", "\x00\x01\x02\xff", "AAEC/w=="},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			vm := newCryptoBridgeVM(t)
			runScript(t, vm, `result = rh:base64Escape("`+tc.in+`")`)
			// Cross-check vs the StdEncoding ground-truth for sanity.
			if expect := base64.StdEncoding.EncodeToString([]byte(tc.in)); expect != tc.want {
				t.Fatalf("StdEncoding ground-truth = %q; want %q", expect, tc.want)
			}
			if got := getGlobalString(t, vm, "result"); got != tc.want {
				t.Fatalf(":base64Escape(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}

	// Direct-byte form for binary input (Lua-side escape would otherwise
	// mangle the bytes). Use NewState API to push the literal string.
	t.Run("binary_via_setglobal", func(t *testing.T) {
		vm := newCryptoBridgeVM(t)
		L := vm.State()
		// Construct binary input directly to avoid Lua-side escape pitfalls.
		input := []byte{0x00, 0x01, 0x02, 0xff}
		L.SetGlobal("input", lua.LString(input))
		runScript(t, vm, `result = rh:base64Escape(input)`)
		want := base64.StdEncoding.EncodeToString(input)
		if got := getGlobalString(t, vm, "result"); got != want {
			t.Fatalf(":base64Escape(binary) = %q; want %q", got, want)
		}
	})
}

// ---------------------------------------------------------------------
// :base64Decode — envoy-go-strict per D8
// ---------------------------------------------------------------------

func Test_Base64Decode_roundtrip_with_Base64Escape(t *testing.T) {
	// Round-trip property: for any input s, :base64Decode(:base64Escape(s))
	// returns s. Covers byte-aligned + non-aligned + NUL-bearing inputs.
	cases := []string{
		"",
		"hi",
		"hello",
		"Hello, World!",
		"\x00\x01\x02\xff",
	}
	for _, in := range cases {
		in := in
		t.Run(hex.EncodeToString([]byte(in)), func(t *testing.T) {
			vm := newCryptoBridgeVM(t)
			L := vm.State()
			L.SetGlobal("input", lua.LString(in))
			runScript(t, vm, `result = rh:base64Decode(rh:base64Escape(input))`)
			if got := getGlobalString(t, vm, "result"); got != in {
				t.Fatalf("roundtrip(%q) = %q; want %q", in, got, in)
			}
		})
	}
}

func Test_Base64Decode_invalid_input_returns_nil_plus_error(t *testing.T) {
	// Per upstream Lua pattern + idiomatic Lua disposition: invalid input
	// returns (nil, err_string) so scripts can check `local b, err =
	// rh:base64Decode("not valid"); if b == nil then ... end`.
	vm := newCryptoBridgeVM(t)
	runScript(t, vm, `b, err = rh:base64Decode("@@@@not-valid-base64@@@@")`)
	if !isGlobalNil(vm, "b") {
		t.Fatalf("b = %v; want nil", vm.State().GetGlobal("b"))
	}
	errGlobal := vm.State().GetGlobal("err")
	if errGlobal == lua.LNil {
		t.Fatalf("err = nil; want a non-empty string")
	}
	if _, ok := errGlobal.(lua.LString); !ok {
		t.Fatalf("err type = %s; want string", errGlobal.Type())
	}
}

// ---------------------------------------------------------------------
// :sha256 — envoy-go-strict per D8
// ---------------------------------------------------------------------

func Test_Sha256_byte_output_vectors(t *testing.T) {
	// Known SHA-256 test vectors (NIST FIPS 180-4). The Lua-side output
	// is lower-case hex (matches `fmt.Sprintf("%x", sum)` Go convention).
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", "abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			vm := newCryptoBridgeVM(t)
			runScript(t, vm, `result = rh:sha256("`+tc.in+`")`)
			// Cross-check vs Go ground-truth.
			sum := sha256.Sum256([]byte(tc.in))
			expect := hex.EncodeToString(sum[:])
			if expect != tc.want {
				t.Fatalf("ground-truth = %q; want %q", expect, tc.want)
			}
			if got := getGlobalString(t, vm, "result"); got != tc.want {
				t.Fatalf(":sha256(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// :sha512 — envoy-go-strict per D8
// ---------------------------------------------------------------------

func Test_Sha512_byte_output_vectors(t *testing.T) {
	// Known SHA-512 test vectors (NIST FIPS 180-4).
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty",
			in:   "",
			want: "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
		},
		{
			name: "abc",
			in:   "abc",
			want: "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			vm := newCryptoBridgeVM(t)
			runScript(t, vm, `result = rh:sha512("`+tc.in+`")`)
			sum := sha512.Sum512([]byte(tc.in))
			expect := hex.EncodeToString(sum[:])
			if expect != tc.want {
				t.Fatalf("ground-truth = %q; want %q", expect, tc.want)
			}
			if got := getGlobalString(t, vm, "result"); got != tc.want {
				t.Fatalf(":sha512(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// :importPublicKey — upstream-parity per D8 (PublicKeyWrapper return)
// ---------------------------------------------------------------------

// genRSAKeyPEM produces an RSA-2048 PKIX SubjectPublicKeyInfo PEM block
// suitable for :importPublicKey testing. Returns (pem, *rsa.PrivateKey)
// so the signature-verify test can sign canned text.
func genRSAKeyPEM(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	derBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: derBytes})
	return string(pemBytes), priv
}

// genECDSAP256KeyPEM produces an ECDSA P-256 PKIX SubjectPublicKeyInfo
// PEM block.
func genECDSAP256KeyPEM(t *testing.T) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	derBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: derBytes})
	return string(pemBytes)
}

// genEd25519KeyPEM produces an Ed25519 PKIX SubjectPublicKeyInfo PEM
// block.
func genEd25519KeyPEM(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	derBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: derBytes})
	return string(pemBytes)
}

func Test_ImportPublicKey_parses_RSA_PKIX_PEM(t *testing.T) {
	pemStr, _ := genRSAKeyPEM(t)
	vm := newCryptoBridgeVM(t)
	L := vm.State()
	L.SetGlobal("pem", lua.LString(pemStr))
	runScript(t, vm, `pub = rh:importPublicKey(pem)`)
	if isGlobalNil(vm, "pub") {
		t.Fatalf("pub = nil; want PublicKeyWrapper userdata")
	}
	if _, ok := L.GetGlobal("pub").(*lua.LUserData); !ok {
		t.Fatalf("pub type = %s; want LUserData", L.GetGlobal("pub").Type())
	}
}

func Test_ImportPublicKey_parses_ECDSA_P256_PKIX_PEM(t *testing.T) {
	pemStr := genECDSAP256KeyPEM(t)
	vm := newCryptoBridgeVM(t)
	L := vm.State()
	L.SetGlobal("pem", lua.LString(pemStr))
	runScript(t, vm, `pub = rh:importPublicKey(pem)`)
	if isGlobalNil(vm, "pub") {
		t.Fatalf("pub = nil; want PublicKeyWrapper userdata")
	}
}

func Test_ImportPublicKey_parses_Ed25519_PKIX_PEM(t *testing.T) {
	pemStr := genEd25519KeyPEM(t)
	vm := newCryptoBridgeVM(t)
	L := vm.State()
	L.SetGlobal("pem", lua.LString(pemStr))
	runScript(t, vm, `pub = rh:importPublicKey(pem)`)
	if isGlobalNil(vm, "pub") {
		t.Fatalf("pub = nil; want PublicKeyWrapper userdata")
	}
}

func Test_ImportPublicKey_invalid_PEM_raises_arm22_byte_stable_wording(t *testing.T) {
	// Arm-22 byte-stable runtime-reject wording per SPEC §6 row 22 + W2.
	// The wording template is `lua: importPublicKey: <inner error>` —
	// pin the byte-stable prefix.
	vm := newCryptoBridgeVM(t)
	L := vm.State()
	L.SetGlobal("pem", lua.LString("not a valid PEM"))
	chunk, err := luaprim.CompileScript([]byte(`rh:importPublicKey(pem)`), nil)
	if err != nil {
		t.Fatalf("CompileScript: %v", err)
	}
	runErr := vm.Run(chunk)
	if runErr == nil {
		t.Fatalf("vm.Run = nil; want runtime error")
	}
	// Byte-stable check: must contain the canonical prefix.
	if !strings.Contains(runErr.Error(), cryptoImportPublicKeyErrPrefix) {
		t.Fatalf("vm.Run error = %q; want substring %q",
			runErr.Error(), cryptoImportPublicKeyErrPrefix)
	}
}

// ---------------------------------------------------------------------
// PublicKeyWrapper :get — MIMICKING upstream wrappers.h:415-427 scope
// ---------------------------------------------------------------------

func Test_PublicKeyWrapper_get_returns_key_bytes(t *testing.T) {
	// The wrapper's :get() method returns the DER-encoded
	// SubjectPublicKeyInfo bytes (MIMICKING upstream wrappers.h:415-427
	// PublicKeyWrapper scope per D8-sub closure).
	pemStr, priv := genRSAKeyPEM(t)
	vm := newCryptoBridgeVM(t)
	L := vm.State()
	L.SetGlobal("pem", lua.LString(pemStr))
	runScript(t, vm, `
local pub = rh:importPublicKey(pem)
result = pub:get()
`)
	got := getGlobalString(t, vm, "result")
	wantDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	if got != string(wantDER) {
		t.Fatalf("pub:get() len = %d; want %d (DER bytes)", len(got), len(wantDER))
	}
}

// ---------------------------------------------------------------------
// :verifySignature — calling convention pinned to upstream
// ---------------------------------------------------------------------

// signRSASHA256 produces an RSA-PKCS1v15 + SHA-256 signature over text
// using the supplied private key.
func signRSASHA256(t *testing.T, priv *rsa.PrivateKey, text string) []byte {
	t.Helper()
	hashed := sha256.Sum256([]byte(text))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, hashed[:])
	if err != nil {
		t.Fatalf("SignPKCS1v15: %v", err)
	}
	return sig
}

func Test_VerifySignature_happy_RSA_SHA256(t *testing.T) {
	// Canonical 4-arg calling convention per upstream lua_filter.cc:611:
	//   request_handle:verifySignature(hash, pubkey_wrapper, sig, text)
	pemStr, priv := genRSAKeyPEM(t)
	text := "hello world"
	sig := signRSASHA256(t, priv, text)

	vm := newCryptoBridgeVM(t)
	L := vm.State()
	L.SetGlobal("pem", lua.LString(pemStr))
	L.SetGlobal("sig", lua.LString(sig))
	L.SetGlobal("text", lua.LString(text))
	runScript(t, vm, `
local pub = rh:importPublicKey(pem)
result = rh:verifySignature("SHA256", pub, sig, text)
`)
	v := L.GetGlobal("result")
	b, ok := v.(lua.LBool)
	if !ok {
		t.Fatalf("result type = %s; want bool", v.Type())
	}
	if !bool(b) {
		t.Fatalf("verifySignature(happy) = false; want true")
	}
}

func Test_VerifySignature_invalid_signature_returns_false(t *testing.T) {
	pemStr, priv := genRSAKeyPEM(t)
	text := "hello world"
	sig := signRSASHA256(t, priv, text)
	// Tamper: flip the first byte.
	sig[0] ^= 0xff

	vm := newCryptoBridgeVM(t)
	L := vm.State()
	L.SetGlobal("pem", lua.LString(pemStr))
	L.SetGlobal("sig", lua.LString(sig))
	L.SetGlobal("text", lua.LString(text))
	runScript(t, vm, `
local pub = rh:importPublicKey(pem)
result = rh:verifySignature("SHA256", pub, sig, text)
`)
	v := L.GetGlobal("result")
	b, ok := v.(lua.LBool)
	if !ok {
		t.Fatalf("result type = %s; want bool", v.Type())
	}
	if bool(b) {
		t.Fatalf("verifySignature(invalid) = true; want false")
	}
}

func Test_VerifySignature_unsupported_hash_algo_returns_false(t *testing.T) {
	pemStr, priv := genRSAKeyPEM(t)
	text := "hello world"
	sig := signRSASHA256(t, priv, text)

	vm := newCryptoBridgeVM(t)
	L := vm.State()
	L.SetGlobal("pem", lua.LString(pemStr))
	L.SetGlobal("sig", lua.LString(sig))
	L.SetGlobal("text", lua.LString(text))
	// "MD5" is in the upstream supported list but envoy-go disallows
	// (NOT in our switch); upstream would reject too via EVP. Use a
	// definitely-unsupported algo string to pin the behavior.
	runScript(t, vm, `
local pub = rh:importPublicKey(pem)
result = rh:verifySignature("NOT_A_REAL_ALGO", pub, sig, text)
`)
	v := L.GetGlobal("result")
	b, ok := v.(lua.LBool)
	if !ok {
		t.Fatalf("result type = %s; want bool", v.Type())
	}
	if bool(b) {
		t.Fatalf("verifySignature(unsupported_algo) = true; want false")
	}
}

func Test_VerifySignature_calling_convention_pinned_4_args(t *testing.T) {
	// Pin the canonical 4-arg shape `rh:verifySignature(hash, pub, sig,
	// text)` from upstream lua_filter.cc:611. The bridge contract is:
	//   arg 1 (receiver) = request_handle userdata
	//   arg 2 = hash algo string ("SHA1"/"SHA224"/"SHA256"/"SHA384"/"SHA512")
	//   arg 3 = PublicKeyWrapper userdata (from :importPublicKey)
	//   arg 4 = signature string (raw bytes)
	//   arg 5 = text string (the plaintext that was signed)
	pemStr, priv := genRSAKeyPEM(t)
	text := "pin the calling convention"
	sig := signRSASHA256(t, priv, text)

	vm := newCryptoBridgeVM(t)
	L := vm.State()
	L.SetGlobal("pem", lua.LString(pemStr))
	L.SetGlobal("sig", lua.LString(sig))
	L.SetGlobal("text", lua.LString(text))

	// The script form is exactly the upstream convention; any deviation
	// (e.g. reordering args) would cause this test to fail.
	runScript(t, vm, `
local pub = rh:importPublicKey(pem)
result = rh:verifySignature("SHA256", pub, sig, text)
`)
	v := L.GetGlobal("result")
	b, ok := v.(lua.LBool)
	if !ok || !bool(b) {
		t.Fatalf("calling convention pin failed: result = %v; want true", v)
	}
}
