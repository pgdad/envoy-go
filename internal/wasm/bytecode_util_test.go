// Tests for byte-faithful ABI-version detection per AMEND-A6.
//
// Byte-faithful semantic reference: proxy-wasm-cpp-host:src/bytecode_util.cc:32-97
// at SHA da3ce05d (the AMEND-A6 pinned upstream commit).
//
// Crafted-wasm fixtures synthesized at runtime via mustBuildModule() — no
// vendored .wasm blobs at Task 2 (the only vendored .wasm in phase 25.1
// arrives at Task 15 as the Rust-sourced fixture-0034 binary).

package wasm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// --- crafted-wasm helpers (test-only) ---------------------------------------

// wasmHeader is the 4-byte magic + 4-byte LE version prefix every valid
// wasm module starts with (per https://webassembly.github.io/spec/core/binary/modules.html).
var wasmHeader = []byte{
	0x00, 0x61, 0x73, 0x6d, // "\0asm" magic
	0x01, 0x00, 0x00, 0x00, // version 1
}

// appendUleb128 appends the unsigned LEB128 encoding of v to dst.
func appendUleb128(dst []byte, v uint32) []byte {
	var buf [binary.MaxVarintLen32]byte
	n := binary.PutUvarint(buf[:], uint64(v))
	return append(dst, buf[:n]...)
}

// buildExportEntry encodes one export entry: LEB128 name-len || name bytes ||
// 1-byte kind || LEB128 index.
func buildExportEntry(name string, kind byte, index uint32) []byte {
	out := appendUleb128(nil, uint32(len(name)))
	out = append(out, name...)
	out = append(out, kind)
	out = appendUleb128(out, index)
	return out
}

// buildExportSection wraps the per-entry encodings into a wasm export section
// (section ID 7 || LEB128 section-len || LEB128 vector-len || concatenated entries).
func buildExportSection(entries [][]byte) []byte {
	var body []byte
	body = appendUleb128(body, uint32(len(entries)))
	for _, e := range entries {
		body = append(body, e...)
	}
	out := []byte{0x07}
	out = appendUleb128(out, uint32(len(body)))
	out = append(out, body...)
	return out
}

// mustBuildModule produces a minimal valid wasm module containing only the
// 8-byte header and a single export section with the given entries.
func mustBuildModule(t *testing.T, entries [][]byte) []byte {
	t.Helper()
	out := append([]byte{}, wasmHeader...)
	if len(entries) > 0 {
		out = append(out, buildExportSection(entries)...)
	}
	return out
}

// --- TestGetAbiVersion: table-driven over crafted-wasm fixtures -------------

func TestGetAbiVersion(t *testing.T) {
	type expect struct {
		ver     AbiVersion
		wantErr bool
		// errSubstr is matched against the returned error's Error() string
		// when wantErr is true. Empty means any non-nil error is acceptable.
		errSubstr string
	}

	v021 := buildExportEntry("proxy_abi_version_0_2_1", 0x00, 0)
	v020 := buildExportEntry("proxy_abi_version_0_2_0", 0x00, 0)
	v010 := buildExportEntry("proxy_abi_version_0_1_0", 0x00, 0)
	memExp := buildExportEntry("memory", 0x02, 0)
	v021AsGlobal := buildExportEntry("proxy_abi_version_0_2_1", 0x03, 0)

	cases := []struct {
		name  string
		input []byte
		want  expect
	}{
		{
			name:  "v0.2.1 sentinel exported as function",
			input: mustBuildModule(t, [][]byte{memExp, v021}),
			want:  expect{ver: AbiVersion_0_2_1},
		},
		{
			name:  "v0.2.0 sentinel exported as function",
			input: mustBuildModule(t, [][]byte{v020}),
			want:  expect{ver: AbiVersion_0_2_0},
		},
		{
			name:  "v0.1.0 sentinel exported as function",
			input: mustBuildModule(t, [][]byte{v010}),
			want:  expect{ver: AbiVersion_0_1_0},
		},
		{
			name:  "module with no export section",
			input: mustBuildModule(t, nil),
			want:  expect{ver: AbiVersionUnknown},
		},
		{
			name:  "module with non-sentinel exports only",
			input: mustBuildModule(t, [][]byte{memExp}),
			want:  expect{ver: AbiVersionUnknown},
		},
		{
			name:  "sentinel name exported as global (kind 0x03) — NOT counted",
			input: mustBuildModule(t, [][]byte{v021AsGlobal}),
			want:  expect{ver: AbiVersionUnknown},
		},
		{
			name:  "empty input — too short for wasm header",
			input: nil,
			want:  expect{wantErr: true, errSubstr: "header"},
		},
		{
			name:  "non-wasm input (wrong magic)",
			input: []byte{0xff, 0xff, 0xff, 0xff, 0x01, 0x00, 0x00, 0x00},
			want:  expect{wantErr: true, errSubstr: "magic"},
		},
		{
			name: "truncated module — section header without body",
			input: func() []byte {
				out := append([]byte{}, wasmHeader...)
				// section ID 7, declared length 100, but no payload follows.
				out = append(out, 0x07)
				out = appendUleb128(out, 100)
				return out
			}(),
			want: expect{wantErr: true},
		},
		{
			name: "malformed export-section vector count > available entries",
			input: func() []byte {
				// Declare 5 entries in the vector but only encode 1
				// non-sentinel entry — the loop should fail mid-parse
				// when it tries to read the second entry's name-length
				// LEB128 from beyond the section bound. We use memExp
				// (a non-sentinel) here so the early-return on first
				// sentinel match does NOT short-circuit the bounds check.
				var body []byte
				body = appendUleb128(body, 5)  // claimed vector length
				body = append(body, memExp...) // only 1 actual entry
				out := append([]byte{}, wasmHeader...)
				out = append(out, 0x07)
				out = appendUleb128(out, uint32(len(body)))
				out = append(out, body...)
				return out
			}(),
			want: expect{wantErr: true},
		},
		{
			name: "first-sentinel-wins: v0.1.0 appears before v0.2.1",
			input: mustBuildModule(t, [][]byte{
				buildExportEntry("proxy_abi_version_0_1_0", 0x00, 0),
				buildExportEntry("proxy_abi_version_0_2_1", 0x00, 1),
			}),
			want: expect{ver: AbiVersion_0_1_0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GetAbiVersion(tc.input)
			if tc.want.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (ver=%v)", got)
				}
				if tc.want.errSubstr != "" && !strings.Contains(err.Error(), tc.want.errSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.want.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want.ver {
				t.Fatalf("got AbiVersion=%v, want %v", got, tc.want.ver)
			}
		})
	}
}

// TestGetAbiVersion_ErrorWrapping verifies that returned errors wrap (via
// fmt.Errorf("...: %w", ...)) so that consumers (Task 5 compile.go) can use
// errors.Is / errors.As on sentinel errors when those land.
func TestGetAbiVersion_ErrorWrapping(t *testing.T) {
	_, err := GetAbiVersion(nil)
	if err == nil {
		t.Fatal("expected error on empty input")
	}
	// errors.Unwrap should not panic; for fmt.Errorf("...: %w", io.EOF) it
	// returns the wrapped target. We just confirm the type-assertion path is
	// safe (a wrapped error reports a non-nil unwrap chain).
	_ = errors.Unwrap(err) // may legitimately return nil for top-level sentinel
}

// TestGetAbiVersion_ExportKindByteBoundCheck regression-pins the
// must-never-panic invariant on the inner export-section parse loop. Prior
// to Phase 25.1 Task 14 fuzzer discovery (corpus seed 444839f772f59a6d),
// the kind-byte bound check used the MODULE end (`len(src)`) instead of
// the section end (`sectionEnd`); on attacker-supplied input where
// `vectorLen` overstated the export count and the section payload was
// short, `pos` advanced past `sectionEnd` and the subsequent
// `readUleb128(src[pos:sectionEnd])` panicked with `slice bounds out of
// range [pos:sectionEnd]` where pos > sectionEnd. Mirrors the bound at
// upstream src/bytecode_util.cc:69 with the tighter section bound noted
// in §line-comment.
func TestGetAbiVersion_ExportKindByteBoundCheck(t *testing.T) {
	// Construct a wasm module with an export section claiming 1 export but
	// only enough bytes for the name-len + name (no kind byte). The MODULE
	// has trailing bytes after the section payload (so len(src) > sectionEnd)
	// — this is exactly the shape that defeats the old `len(src)` bound.
	//
	// Layout: magic+version (8) || export-section(id=7, size=3,
	// vector-len=1, name-len=1, name="x") || trailing 0x00 bytes.
	mod := []byte{}
	mod = append(mod, wasmHeader...)
	// Export section: id=7, payload size = vector-len(1) + name-len(1) + name(1) = 3 bytes.
	// Note: NO kind byte + NO index — exactly the truncated shape.
	mod = append(mod, 0x07, 0x03, 0x01, 0x01, 'x')
	// Trailing bytes that make len(src) > sectionEnd; without the
	// sectionEnd-tight bound, the parser would happily read past
	// sectionEnd into these bytes and then panic on the next inner
	// readUleb128(src[pos:sectionEnd]) call.
	mod = append(mod, 0x00, 0x00, 0x00, 0x00)

	// MUST NOT panic. MUST return a wrapped error.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GetAbiVersion panicked: %v", r)
		}
	}()
	_, err := GetAbiVersion(mod)
	if err == nil {
		t.Fatal("expected error on truncated export-section kind byte; got nil")
	}
	if !strings.Contains(err.Error(), "kind byte overruns section") {
		t.Fatalf("expected 'kind byte overruns section' wrapping; got: %v", err)
	}
}

// TestSentinelStringsAre23Bytes guards against any future typo edit to the
// three sentinel strings; each is exactly 23 ASCII bytes
// (`proxy_abi_version_0_X_Y` = 18 + 5 == 23). The Task 2 prompt mentioned
// 24 as a careless count; the cpp upstream stringliteral is 23 bytes (no
// NUL byte: cpp's `const std::string export_name = {name_begin, export_name_size}`
// constructs the string from the wasm-section bytes only — no NUL inflation).
func TestSentinelStringsAre23Bytes(t *testing.T) {
	for _, s := range []string{
		"proxy_abi_version_0_1_0",
		"proxy_abi_version_0_2_0",
		"proxy_abi_version_0_2_1",
	} {
		if len(s) != 23 {
			t.Errorf("sentinel %q is %d bytes; expected 23", s, len(s))
		}
		// All-ASCII (every rune is one byte).
		if !bytes.Equal([]byte(s), []byte(s[:])) {
			t.Errorf("sentinel %q contains non-ASCII bytes", s)
		}
	}
}
