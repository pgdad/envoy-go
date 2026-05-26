// Tests for the shared-data hostcall dispatch shims per §5.1 #35-36 + Q6 +
// R-25.2-10. Coverage:
//
//   - SetSharedDataShim round-trip: reads (key, value) from guest memory +
//     forwards to sharedDataHost.SetSharedData; propagates result.
//   - SetSharedDataShim cas semantics: cas=0 unconditional; cas-match;
//     cas-mismatch returns CasMismatch.
//   - GetSharedDataShim round-trip: writes value + cas back to guest
//     memory via allocator-discovered malloc; reads value out of memory
//     for verification.
//   - GetSharedDataShim NotFound: zeroes the return slots + returns NotFound.
//   - Empty key + empty value handled correctly.
//   - Non-sharedDataHost Host25_2 ⇒ InternalFailure (programmer error).
//   - Invalid memory access on key/value read returns InvalidMemoryAccess.
//
// The host-side CAS + cap-boundary semantics are exercised at
// internal/wasm/shared_data_test.go against the *RootVM directly; this
// file exercises the shim wire-shape contract against a fake sharedDataHost.

package abi

import (
	"context"
	"sync"
	"testing"
)

// --- fake host -----------------------------------------------------------

// fakeSharedDataHost is an in-memory K-V store with CAS — a minimal
// reimplementation of the *RootVM.SetSharedData / GetSharedData semantic
// against a sync.Mutex-guarded map. The fake exists ONLY for the abi
// shim-level tests; the load-bearing semantics live at the host
// implementation (internal/wasm/shared_data.go).
type fakeSharedDataHost struct {
	mu      sync.Mutex
	entries map[string]fakeSharedDataEntry
	// Per-key result overrides — used by tests that need to force a
	// specific WasmResult (e.g., InternalFailure cap-exceeded).
	forceResult map[string]WasmResult
}

type fakeSharedDataEntry struct {
	value []byte
	cas   uint32
}

func newFakeSharedDataHost() *fakeSharedDataHost {
	return &fakeSharedDataHost{
		entries:     make(map[string]fakeSharedDataEntry),
		forceResult: make(map[string]WasmResult),
	}
}

func (f *fakeSharedDataHost) SetSharedData(key string, value []byte, cas uint32) WasmResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	if forced, ok := f.forceResult[key]; ok {
		return forced
	}
	if existing, ok := f.entries[key]; ok {
		if cas != 0 && existing.cas != cas {
			return WasmResultCasMismatch
		}
		existing.value = append([]byte(nil), value...)
		existing.cas++
		f.entries[key] = existing
		return WasmResultOk
	}
	f.entries[key] = fakeSharedDataEntry{value: append([]byte(nil), value...), cas: 1}
	return WasmResultOk
}

func (f *fakeSharedDataHost) GetSharedData(key string) ([]byte, uint32, WasmResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if entry, ok := f.entries[key]; ok {
		out := append([]byte(nil), entry.value...)
		return out, entry.cas, WasmResultOk
	}
	return nil, 0, WasmResultNotFound
}

// --- helpers --------------------------------------------------------------

// writeKey + writeValue write bytes into the test wazero module's memory
// at fixed offsets. We use offset 16/64 for key/value to leave the first
// few bytes available for the (ret_ptr_ptr, ret_size_ptr, ret_cas_ptr)
// return-by-reference slots (which writers go to lower addresses).
func writeBytesToMem(t *testing.T, mod interface {
	Memory() interface {
		Write(uint32, []byte) bool
	}
}, offset uint32, data []byte) {
	t.Helper()
	if !mod.Memory().Write(offset, data) {
		t.Fatalf("memory write at offset %d failed", offset)
	}
}

// --- SetSharedDataShim round-trip ----------------------------------------

func TestSharedData_Set_NewEntry(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeSharedDataHost()

	const keyPtr, keySize uint32 = 16, 5
	const valuePtr, valueSize uint32 = 64, 3
	if !mem.Write(keyPtr, []byte("hello")) {
		t.Fatalf("write key failed")
	}
	if !mem.Write(valuePtr, []byte("abc")) {
		t.Fatalf("write value failed")
	}

	res := SetSharedDataShim(ctx, mod, host, keyPtr, keySize, valuePtr, valueSize, 0)
	if res != WasmResultOk {
		t.Errorf("Set new entry: got %v; want Ok", res)
	}

	got, cas, status := host.GetSharedData("hello")
	if status != WasmResultOk {
		t.Errorf("Get after Set: status=%v; want Ok", status)
	}
	if string(got) != "abc" {
		t.Errorf("Get value: got %q; want %q", got, "abc")
	}
	if cas != 1 {
		t.Errorf("Get cas: got %d; want 1", cas)
	}
}

func TestSharedData_Set_CasMatch(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeSharedDataHost()

	// Seed an entry directly.
	host.SetSharedData("k", []byte("old"), 0)

	const keyPtr, keySize uint32 = 16, 1
	const valuePtr, valueSize uint32 = 64, 3
	if !mem.Write(keyPtr, []byte("k")) {
		t.Fatalf("write key failed")
	}
	if !mem.Write(valuePtr, []byte("new")) {
		t.Fatalf("write value failed")
	}
	// cas=1 matches the seeded entry.
	res := SetSharedDataShim(ctx, mod, host, keyPtr, keySize, valuePtr, valueSize, 1)
	if res != WasmResultOk {
		t.Errorf("Set cas-match: got %v; want Ok", res)
	}
	got, cas, _ := host.GetSharedData("k")
	if string(got) != "new" || cas != 2 {
		t.Errorf("after cas-match: got (%q, %d); want (%q, 2)", got, cas, "new")
	}
}

func TestSharedData_Set_CasMismatch(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeSharedDataHost()
	host.SetSharedData("k", []byte("old"), 0)

	const keyPtr, keySize uint32 = 16, 1
	const valuePtr, valueSize uint32 = 64, 3
	if !mem.Write(keyPtr, []byte("k")) {
		t.Fatalf("write key failed")
	}
	if !mem.Write(valuePtr, []byte("new")) {
		t.Fatalf("write value failed")
	}
	// cas=99 mismatches the seeded entry's cas=1.
	res := SetSharedDataShim(ctx, mod, host, keyPtr, keySize, valuePtr, valueSize, 99)
	if res != WasmResultCasMismatch {
		t.Errorf("Set cas-mismatch: got %v; want CasMismatch", res)
	}
	got, cas, _ := host.GetSharedData("k")
	if string(got) != "old" || cas != 1 {
		t.Errorf("after cas-mismatch: got (%q, %d); want (%q, 1) unchanged", got, cas, "old")
	}
}

func TestSharedData_Set_InternalFailureForwarded(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeSharedDataHost()
	host.forceResult["k"] = WasmResultInternalFailure

	const keyPtr, keySize uint32 = 16, 1
	const valuePtr, valueSize uint32 = 64, 3
	if !mem.Write(keyPtr, []byte("k")) {
		t.Fatalf("write key failed")
	}
	if !mem.Write(valuePtr, []byte("new")) {
		t.Fatalf("write value failed")
	}
	res := SetSharedDataShim(ctx, mod, host, keyPtr, keySize, valuePtr, valueSize, 0)
	if res != WasmResultInternalFailure {
		t.Errorf("Set forced-cap-exceeded: got %v; want InternalFailure", res)
	}
}

func TestSharedData_Set_EmptyKeyAndValue(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	host := newFakeSharedDataHost()

	// keySize=0 + valueSize=0 → empty-key empty-value entry.
	res := SetSharedDataShim(ctx, mod, host, 0, 0, 0, 0, 0)
	if res != WasmResultOk {
		t.Errorf("Set empty: got %v; want Ok", res)
	}
	got, cas, status := host.GetSharedData("")
	if status != WasmResultOk {
		t.Errorf("Get empty key: status=%v; want Ok", status)
	}
	if len(got) != 0 {
		t.Errorf("Get empty key value: len=%d; want 0", len(got))
	}
	if cas != 1 {
		t.Errorf("Get empty key cas: %d; want 1", cas)
	}
}

func TestSharedData_Set_InvalidMemoryOnKey(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	host := newFakeSharedDataHost()
	// Key pointer way past 1 page (64 KiB) — memory.Read should fail.
	res := SetSharedDataShim(ctx, mod, host, 1_000_000, 10, 0, 0, 0)
	if res != WasmResultInvalidMemoryAccess {
		t.Errorf("Set bad key ptr: got %v; want InvalidMemoryAccess", res)
	}
}

func TestSharedData_Set_InvalidMemoryOnValue(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeSharedDataHost()
	if !mem.Write(16, []byte("k")) {
		t.Fatalf("write key failed")
	}
	// Value pointer way past 1 page — memory.Read should fail.
	res := SetSharedDataShim(ctx, mod, host, 16, 1, 1_000_000, 10, 0)
	if res != WasmResultInvalidMemoryAccess {
		t.Errorf("Set bad value ptr: got %v; want InvalidMemoryAccess", res)
	}
}

func TestSharedData_Set_NonHostValue(t *testing.T) {
	res := SetSharedDataShim(context.Background(), nil, "not a host", 0, 0, 0, 0, 0)
	if res != WasmResultInternalFailure {
		t.Errorf("Set non-host: got %v; want InternalFailure", res)
	}
}

func TestSharedData_Set_NilHost(t *testing.T) {
	res := SetSharedDataShim(context.Background(), nil, nil, 0, 0, 0, 0, 0)
	if res != WasmResultInternalFailure {
		t.Errorf("Set nil host: got %v; want InternalFailure", res)
	}
}

// --- GetSharedDataShim round-trip ----------------------------------------

func TestSharedData_Get_OkRoundTrip(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeSharedDataHost()
	host.SetSharedData("k", []byte("hello"), 0)

	const keyPtr, keySize uint32 = 16, 1
	const retValuePtrPtr, retValueSizePtr, retCASPtr uint32 = 0, 4, 8
	if !mem.Write(keyPtr, []byte("k")) {
		t.Fatalf("write key failed")
	}
	res := GetSharedDataShim(ctx, mod, host, keyPtr, keySize, retValuePtrPtr, retValueSizePtr, retCASPtr)
	if res != WasmResultOk {
		t.Fatalf("Get round-trip: got %v; want Ok", res)
	}

	gotOffset, ok := mem.ReadUint32Le(retValuePtrPtr)
	if !ok {
		t.Fatalf("read value ptr failed")
	}
	gotSize, ok := mem.ReadUint32Le(retValueSizePtr)
	if !ok {
		t.Fatalf("read value size failed")
	}
	gotCAS, ok := mem.ReadUint32Le(retCASPtr)
	if !ok {
		t.Fatalf("read cas failed")
	}
	if gotSize != 5 {
		t.Errorf("Get size: got %d; want 5", gotSize)
	}
	if gotCAS != 1 {
		t.Errorf("Get cas: got %d; want 1", gotCAS)
	}
	gotBytes, ok := mem.Read(gotOffset, gotSize)
	if !ok {
		t.Fatalf("read returned-value bytes failed")
	}
	if string(gotBytes) != "hello" {
		t.Errorf("Get value: got %q; want %q", gotBytes, "hello")
	}
}

func TestSharedData_Get_NotFoundZeroesSlots(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeSharedDataHost()

	const keyPtr, keySize uint32 = 16, 4
	const retValuePtrPtr, retValueSizePtr, retCASPtr uint32 = 0, 4, 8
	// Seed return slots with non-zero so we can verify the shim zeros them.
	mem.WriteUint32Le(retValuePtrPtr, 0xDEADBEEF)
	mem.WriteUint32Le(retValueSizePtr, 0xCAFEBABE)
	mem.WriteUint32Le(retCASPtr, 0x12345678)
	if !mem.Write(keyPtr, []byte("nope")) {
		t.Fatalf("write key failed")
	}

	res := GetSharedDataShim(ctx, mod, host, keyPtr, keySize, retValuePtrPtr, retValueSizePtr, retCASPtr)
	if res != WasmResultNotFound {
		t.Errorf("Get nonexistent: got %v; want NotFound", res)
	}
	gotOffset, _ := mem.ReadUint32Le(retValuePtrPtr)
	gotSize, _ := mem.ReadUint32Le(retValueSizePtr)
	gotCAS, _ := mem.ReadUint32Le(retCASPtr)
	if gotOffset != 0 || gotSize != 0 || gotCAS != 0 {
		t.Errorf("Get nonexistent slots: (offset=%d, size=%d, cas=%d); want all zero",
			gotOffset, gotSize, gotCAS)
	}
}

func TestSharedData_Get_EmptyValueOkWritesZeroPtrSize(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeSharedDataHost()
	host.SetSharedData("k", nil, 0) // empty value, cas=1

	const keyPtr, keySize uint32 = 16, 1
	const retValuePtrPtr, retValueSizePtr, retCASPtr uint32 = 0, 4, 8
	if !mem.Write(keyPtr, []byte("k")) {
		t.Fatalf("write key failed")
	}
	res := GetSharedDataShim(ctx, mod, host, keyPtr, keySize, retValuePtrPtr, retValueSizePtr, retCASPtr)
	if res != WasmResultOk {
		t.Errorf("Get empty value: got %v; want Ok", res)
	}
	gotOffset, _ := mem.ReadUint32Le(retValuePtrPtr)
	gotSize, _ := mem.ReadUint32Le(retValueSizePtr)
	gotCAS, _ := mem.ReadUint32Le(retCASPtr)
	if gotOffset != 0 || gotSize != 0 {
		t.Errorf("Get empty value slots: (offset=%d, size=%d); want (0, 0)", gotOffset, gotSize)
	}
	if gotCAS != 1 {
		t.Errorf("Get empty value cas: %d; want 1", gotCAS)
	}
}

func TestSharedData_Get_InvalidMemoryOnKey(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	host := newFakeSharedDataHost()
	// Key pointer way past 1 page (64 KiB).
	res := GetSharedDataShim(ctx, mod, host, 1_000_000, 10, 0, 4, 8)
	if res != WasmResultInvalidMemoryAccess {
		t.Errorf("Get bad key ptr: got %v; want InvalidMemoryAccess", res)
	}
}

func TestSharedData_Get_NonHostValue(t *testing.T) {
	res := GetSharedDataShim(context.Background(), nil, "not a host", 0, 0, 0, 0, 0)
	if res != WasmResultInternalFailure {
		t.Errorf("Get non-host: got %v; want InternalFailure", res)
	}
}

func TestSharedData_Get_NilHost(t *testing.T) {
	res := GetSharedDataShim(context.Background(), nil, nil, 0, 0, 0, 0, 0)
	if res != WasmResultInternalFailure {
		t.Errorf("Get nil host: got %v; want InternalFailure", res)
	}
}

// --- silence unused helper -----------------------------------------------

var _ = writeBytesToMem // helper retained for future Task 6 ext tests
