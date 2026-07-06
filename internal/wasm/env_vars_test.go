// Tests for env-variable assembly (AssembleEnvVars) + WASI environ encoding
// (encodeWASIEnviron) + the WASI shim round-trip (wasiEnvironSizesGet /
// wasiEnvironGet) per phase-25.3 Task 4 AMEND-C4.
//
// Run order: write tests → FAIL (pre-impl) → implement A,B,C → PASS (-race).
package wasm

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// --- TestAssembleEnvVars ---------------------------------------------------

func TestAssembleEnvVars_CollisionReject(t *testing.T) {
	_, err := AssembleEnvVars([]string{"DUP"}, map[string]string{"DUP": "v"})
	if err == nil {
		t.Fatal("cross-field key collision must reject (upstream parity)")
	}
	if !errors.Is(err, ErrEnvVarsKeyCollision) {
		t.Fatalf("errors.Is(err, ErrEnvVarsKeyCollision) = false; got %v", err)
	}
}

func TestAssembleEnvVars_KeyValuesAndAbsentHostKey(t *testing.T) {
	if err := os.Unsetenv("ENVOY_GO_ABSENT_XYZ"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	env, err := AssembleEnvVars([]string{"ENVOY_GO_ABSENT_XYZ"}, map[string]string{"K": "V"})
	if err != nil {
		t.Fatal(err)
	}
	if env["K"] != "V" {
		t.Fatalf("key_values not applied: %v", env)
	}
	if _, ok := env["ENVOY_GO_ABSENT_XYZ"]; ok {
		t.Fatal("absent host_env_key must be silently skipped")
	}
}

func TestAssembleEnvVars_CapExceeded(t *testing.T) {
	big := map[string]string{}
	for i := 0; i < 65; i++ {
		big["K"+strconv.Itoa(i)] = "x"
	} // 65 entries > 64
	if _, err := AssembleEnvVars(nil, big); err == nil {
		t.Fatal("> 64 entries must reject (envoy-go-strict cap)")
	} else if !errors.Is(err, ErrEnvVarsCapExceeded) {
		t.Fatalf("errors.Is(err, ErrEnvVarsCapExceeded) = false for >64 entries; got %v", err)
	}
	if _, err := AssembleEnvVars(nil, map[string]string{"K": string(make([]byte, 4097))}); err == nil {
		t.Fatal("value > 4 KiB must reject (envoy-go-strict cap)")
	} else if !errors.Is(err, ErrEnvVarsCapExceeded) {
		t.Fatalf("errors.Is(err, ErrEnvVarsCapExceeded) = false for >4096-byte value; got %v", err)
	}
}

func TestAssembleEnvVars_CapBoundaryOK(t *testing.T) {
	// Exactly 64 entries must succeed (cap is strict >, not >=).
	exact64 := map[string]string{}
	for i := 0; i < 64; i++ {
		exact64["K"+strconv.Itoa(i)] = "x"
	}
	got, err := AssembleEnvVars(nil, exact64)
	if err != nil {
		t.Fatalf("exactly 64 entries must not reject; got err: %v", err)
	}
	if len(got) != 64 {
		t.Fatalf("expected 64 entries in result, got %d", len(got))
	}

	// A value of exactly 4096 bytes must succeed (cap is strict >, not >=).
	_, err = AssembleEnvVars(nil, map[string]string{"K": string(make([]byte, 4096))})
	if err != nil {
		t.Fatalf("value of exactly 4096 bytes must not reject; got err: %v", err)
	}
}

func TestAssembleEnvVars_PresentHostKey(t *testing.T) {
	t.Setenv("ENVOY_GO_PRESENT_XYZ", "hostval")
	env, err := AssembleEnvVars([]string{"ENVOY_GO_PRESENT_XYZ"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if env["ENVOY_GO_PRESENT_XYZ"] != "hostval" {
		t.Fatalf("present host key not read: %v", env)
	}
}

func TestEncodeWASIEnviron(t *testing.T) {
	got := encodeWASIEnviron(map[string]string{"B": "2", "A": "1"})
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	// sorted order: A first, B second; each KEY=VALUE\0
	if string(got[0]) != "A=1\x00" {
		t.Fatalf("entry0 = %q want %q", got[0], "A=1\\x00")
	}
	if string(got[1]) != "B=2\x00" {
		t.Fatalf("entry1 = %q want %q", got[1], "B=2\\x00")
	}
}

func TestEncodeWASIEnviron_Empty(t *testing.T) {
	got := encodeWASIEnviron(nil)
	if len(got) != 0 {
		t.Fatalf("nil env must return empty slice, got %d entries", len(got))
	}
	got2 := encodeWASIEnviron(map[string]string{})
	if len(got2) != 0 {
		t.Fatalf("empty env must return empty slice, got %d entries", len(got2))
	}
}

// --- WASI shim round-trip tests -------------------------------------------
//
// These tests exercise wasiEnvironSizesGet + wasiEnvironGet against a real
// wazero memory (via newTestModule from wasi_test.go) with a mockWasiHost
// seeded with a non-empty WASIEnviron() return. This is the full round-trip
// variant — real memory, real shim, asserting pointer-table layout +
// KEY=VALUE\0 bytes in the buffer.

// mockWasiHostWithEnv wraps mockWasiHost and overrides WASIEnviron so the
// test can inject a specific set of entries without routing through a real
// *RootVM. The mockWasiHost embedded type provides IsAllowed + LogProxy.
type mockWasiHostWithEnv struct {
	*mockWasiHost
	entries [][]byte
}

func (m *mockWasiHostWithEnv) WASIEnviron() [][]byte { return m.entries }

// allowAllWithEnv returns a mockWasiHostWithEnv with every WASI capability
// allowed and a WASIEnviron() returning the given encoded entries.
func allowAllWithEnv(entries [][]byte) *mockWasiHostWithEnv {
	return &mockWasiHostWithEnv{
		mockWasiHost: allowAll(),
		entries:      entries,
	}
}

func TestWasiEnvironShimsRoundTrip(t *testing.T) {
	// Assemble env {"A":"1","B":"2"} → sorted entries ["A=1\0","B=2\0"].
	entries := encodeWASIEnviron(map[string]string{"A": "1", "B": "2"})
	if len(entries) != 2 {
		t.Fatalf("precondition: encodeWASIEnviron returned %d entries", len(entries))
	}

	ctx := context.Background()
	mod, cleanup := newTestModule(t)
	defer cleanup()

	host := allowAllWithEnv(entries)

	// Memory layout:
	//   0x100 : num_elements_ptr (written by environ_sizes_get)
	//   0x104 : buffer_size_ptr  (written by environ_sizes_get)
	//   0x200 : environ_ptr      (pointer table; 2 entries × 4 bytes = 8 bytes)
	//   0x300 : environ_buf_ptr  (raw KEY=VALUE\0 bytes)
	const numElemPtr = uint32(0x100)
	const bufSizePtr = uint32(0x104)
	const environPtr = uint32(0x200)
	const bufPtr = uint32(0x300)

	// Step 1: call environ_sizes_get.
	errno := wasiEnvironSizesGet(ctx, mod, host, numElemPtr, bufSizePtr)
	if errno != abi.WasiErrnoSuccess {
		t.Fatalf("environ_sizes_get errno = %d, want success", errno)
	}

	count := readU32(t, mod.Memory(), numElemPtr)
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	// Expected buffer size = len("A=1\0") + len("B=2\0") = 4 + 4 = 8
	expectedBufSize := uint32(0)
	for _, e := range entries {
		expectedBufSize += uint32(len(e)) //nolint:gosec
	}
	gotBufSize := readU32(t, mod.Memory(), bufSizePtr)
	if gotBufSize != expectedBufSize {
		t.Fatalf("buf_size = %d, want %d", gotBufSize, expectedBufSize)
	}

	// Step 2: call environ_get with the pointer table at 0x200 and buffer at 0x300.
	errno = wasiEnvironGet(ctx, mod, host, environPtr, bufPtr)
	if errno != abi.WasiErrnoSuccess {
		t.Fatalf("environ_get errno = %d, want success", errno)
	}

	// Verify: for each entry i, environ_ptr[i*4] must point into the buffer at
	// the correct offset, and the bytes there must equal entries[i].
	offset := bufPtr
	for i, entry := range entries {
		// Read the pointer-table entry.
		ptrTableOffset := environPtr + uint32(i)*4 //nolint:gosec
		gotPtr := readU32(t, mod.Memory(), ptrTableOffset)
		if gotPtr != offset {
			t.Errorf("environ_ptr[%d] = %#x, want %#x", i, gotPtr, offset)
		}
		// Read and compare the bytes at the advertised offset.
		gotBytes := readBytes(t, mod.Memory(), offset, uint32(len(entry))) //nolint:gosec
		if string(gotBytes) != string(entry) {
			t.Errorf("entry[%d] bytes = %q, want %q", i, gotBytes, entry)
		}
		offset += uint32(len(entry)) //nolint:gosec
	}
}

func TestWasiEnvironShimsRoundTrip_EmptyEnv(t *testing.T) {
	// Empty env: both shims must succeed + write 0/0 for sizes.
	ctx := context.Background()
	mod, cleanup := newTestModule(t)
	defer cleanup()

	host := allowAllWithEnv(nil)

	// Write sentinels.
	writeU32(t, mod.Memory(), 0x100, 0xDEADBEEF)
	writeU32(t, mod.Memory(), 0x104, 0xCAFEBABE)

	errno := wasiEnvironSizesGet(ctx, mod, host, 0x100, 0x104)
	if errno != abi.WasiErrnoSuccess {
		t.Fatalf("errno = %d, want success", errno)
	}
	if got := readU32(t, mod.Memory(), 0x100); got != 0 {
		t.Errorf("count = %d, want 0", got)
	}
	if got := readU32(t, mod.Memory(), 0x104); got != 0 {
		t.Errorf("buf_size = %d, want 0", got)
	}

	// environ_get with empty env: should write nothing, return success.
	writeU32(t, mod.Memory(), 0x200, 0x11111111)
	writeU32(t, mod.Memory(), 0x300, 0x22222222)
	errno = wasiEnvironGet(ctx, mod, host, 0x200, 0x300)
	if errno != abi.WasiErrnoSuccess {
		t.Fatalf("environ_get empty errno = %d, want success", errno)
	}
	// Nothing written: sentinels must survive.
	if got := readU32(t, mod.Memory(), 0x200); got != 0x11111111 {
		t.Errorf("environ_ptr clobbered: %#x", got)
	}
	if got := readU32(t, mod.Memory(), 0x300); got != 0x22222222 {
		t.Errorf("environ_buf clobbered: %#x", got)
	}
}
