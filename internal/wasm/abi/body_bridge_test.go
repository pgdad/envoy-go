// internal/wasm/abi/body_bridge_test.go — AMEND-B1 clamp golden table per
// R-25.2-1 + the round-trip-through-callbacks coverage for the body+buffer
// hostcall family (proxy_get_buffer_bytes + proxy_set_buffer_bytes +
// proxy_get_buffer_status) per 25.2 SPEC §5.1 #25-27 + §11.1.
//
// The clamp wire-contract per AMEND-B1 (cpp-host src/exports.cc:get_buffer_-
// bytes byte-faithful):
//
//	if start > buffer.size:                  length = 0       (Ok)
//	elif start + length > buffer.size:       length = clamped (Ok)
//	else:                                    length = length  (Ok)
//
// Only the i32-overflow path `start + max_size < start` (uint32 wraparound)
// returns WasmResult::BadArgument. The spec README text says BAD_ARGUMENT for
// "buffer overflow due to invalid start and/or max_size" but cpp-host
// REFUTES that strict reading — Istio + Envoy production guests rely on the
// clamp. envoy-go MUST mirror cpp-host.
//
// Test fake `fakeBufferHost` is a tiny in-package implementation of the
// private `bufferHost` / `streamHost` interfaces that the shims type-assert
// to. It carries a single canned buffer + records the SetBuffer + Continue/
// Close round-trip args so the test can assert the dispatch reached the
// callback layer correctly.

package abi

import (
	"context"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// --- fake host + memory ---------------------------------------------------

// fakeBufferHost satisfies the unexported bufferHost + streamHost interfaces
// the shims type-assert to. The test seeds (buffer, allocOffset) and the
// shim does the rest.
type fakeBufferHost struct {
	ctxID uint32

	// per-(streamCtxID, bufferType) buffer state.
	buffers map[uint64][]byte
	// per-(streamCtxID, bufferType) flag state for proxy_get_buffer_status.
	flags map[uint64]uint32

	// GetBuffer returns this error if non-nil (lets tests cover the
	// callback-returned-error path → WasmResultBadArgument).
	getErr    error
	setResult WasmResult // zero = Ok
	statusErr error

	// SetBuffer record (the last call's args).
	setLastStreamCtx uint32
	setLastType      WasmBufferType
	setLastStart     uint32
	setLastData      []byte

	// Continue/Close stream records (last call args).
	continueLastStreamCtx uint32
	continueLastType      uint32
	closeLastStreamCtx    uint32
	closeLastType         uint32
	continueResult        WasmResult // zero value = WasmResultOk
	closeResult           WasmResult // zero value = WasmResultOk
}

func newFakeBufferHost() *fakeBufferHost {
	return &fakeBufferHost{
		ctxID:   42,
		buffers: make(map[uint64][]byte),
		flags:   make(map[uint64]uint32),
	}
}

func bufKey(streamCtxID uint32, bt WasmBufferType) uint64 {
	return uint64(streamCtxID)<<32 | uint64(uint32(bt))
}

func (f *fakeBufferHost) seed(bt WasmBufferType, b []byte) {
	f.buffers[bufKey(f.ctxID, bt)] = b
}

func (f *fakeBufferHost) seedFlags(bt WasmBufferType, flags uint32) {
	f.flags[bufKey(f.ctxID, bt)] = flags
}

// bufferHost implementation -------------------------------------------------

func (f *fakeBufferHost) CurrentCtxID() uint32 { return f.ctxID }

func (f *fakeBufferHost) BufferGet(_ context.Context, streamCtxID uint32, bt WasmBufferType) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.buffers[bufKey(streamCtxID, bt)], nil
}

func (f *fakeBufferHost) BufferSet(_ context.Context, streamCtxID uint32, bt WasmBufferType, start uint32, data []byte) WasmResult {
	f.setLastStreamCtx = streamCtxID
	f.setLastType = bt
	f.setLastStart = start
	f.setLastData = append([]byte(nil), data...)
	return f.setResult
}

func (f *fakeBufferHost) BufferStatus(_ context.Context, streamCtxID uint32, bt WasmBufferType) (uint32, uint32, error) {
	if f.statusErr != nil {
		return 0, 0, f.statusErr
	}
	return uint32(len(f.buffers[bufKey(streamCtxID, bt)])), f.flags[bufKey(streamCtxID, bt)], nil
}

func (f *fakeBufferHost) AllocateGuestBuffer(ctx context.Context, mod api.Module, data []byte, ptrPtr, sizePtr uint32) WasmResult {
	// Honor the standard return-by-reference shape: empty payload writes
	// (0, 0) without invoking the allocator. Non-empty calls the guest
	// "malloc" + writes the data + writes (offset, size).
	mem := mod.Memory()
	if len(data) == 0 {
		if !mem.WriteUint32Le(ptrPtr, 0) || !mem.WriteUint32Le(sizePtr, 0) {
			return WasmResultInvalidMemoryAccess
		}
		return WasmResultOk
	}
	allocator := mod.ExportedFunction("malloc")
	if allocator == nil {
		return WasmResultInvalidMemoryAccess
	}
	results, err := allocator.Call(ctx, uint64(uint32(len(data))))
	if err != nil || len(results) == 0 {
		return WasmResultInvalidMemoryAccess
	}
	offset := uint32(results[0])
	if offset == 0 {
		return WasmResultInvalidMemoryAccess
	}
	if !mem.Write(offset, data) {
		return WasmResultInvalidMemoryAccess
	}
	if !mem.WriteUint32Le(ptrPtr, offset) {
		return WasmResultInvalidMemoryAccess
	}
	if !mem.WriteUint32Le(sizePtr, uint32(len(data))) {
		return WasmResultInvalidMemoryAccess
	}
	return WasmResultOk
}

// streamHost implementation ------------------------------------------------

func (f *fakeBufferHost) StreamContinue(_ context.Context, streamCtxID, streamType uint32) WasmResult {
	f.continueLastStreamCtx = streamCtxID
	f.continueLastType = streamType
	return f.continueResult
}

func (f *fakeBufferHost) StreamClose(_ context.Context, streamCtxID, streamType uint32) WasmResult {
	f.closeLastStreamCtx = streamCtxID
	f.closeLastType = streamType
	return f.closeResult
}

// --- guest-module fixture for memory exercise -----------------------------

// newHostingModule constructs a wazero module that exports a `malloc(i32) i32`
// returning a fixed offset + a 1-page memory. Used by the body_bridge tests
// that need an actual api.Module to read/write through.
//
// The malloc returns offset 256 on every call (single-allocation tests; tests
// that need multiple distinct allocations would need a bumper). The memory
// section is 1 page (64 KiB) which is ample for the golden-table fixtures.
func newHostingModule(t *testing.T, ctx context.Context) api.Module {
	t.Helper()
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
	t.Cleanup(func() { _ = rt.Close(ctx) })

	wasmBin := buildMallocOnlyModule()
	mod, err := rt.Instantiate(ctx, wasmBin)
	if err != nil {
		t.Fatalf("instantiate hosting module: %v", err)
	}
	return mod
}

// buildMallocOnlyModule hand-crafts a minimal wasm binary that exports
// (a) memory (1 page) and (b) malloc(i32) -> i32 (returns const 256).
func buildMallocOnlyModule() []byte {
	// Module header.
	hdr := []byte{0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00}

	// Type section: 1 type — (i32) -> i32.
	typeSec := section(0x01, []byte{
		0x01,                   // num types
		0x60, 0x01, 0x7F, 0x01, // (i32) -> i32
		0x7F,
	})

	// Function section: 1 function, type 0.
	funcSec := section(0x03, []byte{0x01, 0x00})

	// Memory section: 1 memory, min=1 max not set.
	memSec := section(0x05, []byte{0x01, 0x00, 0x01})

	// Export section: "memory" (memory 0) + "malloc" (func 0).
	exportSec := section(0x07, []byte{
		0x02, // num exports

		0x06, 'm', 'e', 'm', 'o', 'r', 'y',
		0x02, 0x00, // memory idx 0

		0x06, 'm', 'a', 'l', 'l', 'o', 'c',
		0x00, 0x00, // func idx 0
	})

	// Code section payload: 1 function, body = `i32.const 256; end`.
	body := []byte{
		0x00,                   // num locals
		0x41, 0x80, 0x02, 0x0B, // i32.const 256 (LEB128); end
	}
	codePayload := []byte{0x01}                        // num funcs
	codePayload = append(codePayload, byte(len(body))) // body size (1-byte LEB ok < 128)
	codePayload = append(codePayload, body...)
	codeSec := section(0x0A, codePayload)

	out := append(hdr, typeSec...)
	out = append(out, funcSec...)
	out = append(out, memSec...)
	out = append(out, exportSec...)
	out = append(out, codeSec...)
	return out
}

// section wraps a section payload with (sectionID, byte-len-LEB128, payload).
// Lengths < 128 fit in a single byte (true for the small fixtures here).
func section(id byte, payload []byte) []byte {
	if len(payload) >= 128 {
		panic(fmt.Sprintf("section payload too large for 1-byte LEB128: %d", len(payload)))
	}
	out := []byte{id, byte(len(payload))}
	out = append(out, payload...)
	return out
}

// --- TestBodyBridge_GetBufferBytes_GoldenTable ----------------------------

// TestBodyBridge_GetBufferBytes_GoldenTable exhaustively asserts the 6
// AMEND-B1 clamp golden-table rows per R-25.2-1.
func TestBodyBridge_GetBufferBytes_GoldenTable(t *testing.T) {
	ctx := context.Background()

	const memLen = 20
	buf := make([]byte, memLen)
	for i := range buf {
		buf[i] = byte(i)
	}

	type tc struct {
		name       string
		bufferType uint32
		start      uint32
		maxSize    uint32
		// retDataPtr + retSizePtr are derived per test (we use 64/128).
		wantResult WasmResult
		wantLen    uint32 // ignored when wantResult != Ok
	}
	cases := []tc{
		// (a) start in bounds, max in bounds → Ok + length=10
		{name: "a/in-bounds", bufferType: uint32(WasmBufferTypeHttpRequestBody), start: 0, maxSize: 10, wantResult: WasmResultOk, wantLen: 10},
		// (b) start in bounds, max overflows buffer end → Ok + length=clamp (5)
		{name: "b/clamp-on-overflow", bufferType: uint32(WasmBufferTypeHttpRequestBody), start: 15, maxSize: 10, wantResult: WasmResultOk, wantLen: 5},
		// (c) start at end → Ok + length=0
		{name: "c/start-at-end", bufferType: uint32(WasmBufferTypeHttpRequestBody), start: 20, maxSize: 5, wantResult: WasmResultOk, wantLen: 0},
		// (d) start beyond end → Ok + length=0
		{name: "d/start-beyond-end", bufferType: uint32(WasmBufferTypeHttpRequestBody), start: 30, maxSize: 5, wantResult: WasmResultOk, wantLen: 0},
		// (e) i32 overflow on start+maxSize → BadArgument
		{name: "e/i32-overflow", bufferType: uint32(WasmBufferTypeHttpRequestBody), start: 0xFFFFFFFE, maxSize: 5, wantResult: WasmResultBadArgument},
		// (f) unrecognized buffer-type (99) → BadArgument
		{name: "f/bad-buffer-type", bufferType: 99, start: 0, maxSize: 5, wantResult: WasmResultBadArgument},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host := newFakeBufferHost()
			host.seed(WasmBufferTypeHttpRequestBody, buf)

			mod := newHostingModule(t, ctx)

			const retDataPtr uint32 = 64
			const retSizePtr uint32 = 128

			got := GetBufferBytesShim(ctx, mod, host, c.bufferType, c.start, c.maxSize, retDataPtr, retSizePtr)
			if got != c.wantResult {
				t.Fatalf("GetBufferBytesShim = %v, want %v", got, c.wantResult)
			}
			if c.wantResult != WasmResultOk {
				return
			}
			// On Ok, retSizePtr must reflect the clamped length.
			gotLen, ok := mod.Memory().ReadUint32Le(retSizePtr)
			if !ok {
				t.Fatalf("ReadUint32Le(retSizePtr) failed")
			}
			if gotLen != c.wantLen {
				t.Fatalf("clamped length = %d, want %d", gotLen, c.wantLen)
			}
			// On non-zero length, the byte payload must equal buf[start:start+wantLen].
			if c.wantLen > 0 {
				offset, ok := mod.Memory().ReadUint32Le(retDataPtr)
				if !ok {
					t.Fatalf("ReadUint32Le(retDataPtr) failed")
				}
				if offset == 0 {
					t.Fatalf("offset = 0 (allocator failure or empty path)")
				}
				gotBytes, ok := mod.Memory().Read(offset, gotLen)
				if !ok {
					t.Fatalf("Read(offset=%d, len=%d) failed", offset, gotLen)
				}
				wantBytes := buf[c.start : c.start+c.wantLen]
				if string(gotBytes) != string(wantBytes) {
					t.Fatalf("payload bytes = % x, want % x", gotBytes, wantBytes)
				}
			} else {
				// length==0 must write (0, 0) without invoking the allocator.
				offset, ok := mod.Memory().ReadUint32Le(retDataPtr)
				if !ok {
					t.Fatalf("ReadUint32Le(retDataPtr) failed")
				}
				if offset != 0 {
					t.Fatalf("length=0 path wrote offset=%d, want 0", offset)
				}
			}
		})
	}
}

// TestBodyBridge_GetBufferBytes_CallbackError covers the path where the
// consumer-side BufferGet returns an error — the shim must convert that to
// WasmResultBadArgument (no leaking the Go error).
func TestBodyBridge_GetBufferBytes_CallbackError(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	host.getErr = fmt.Errorf("synthetic callback failure")
	mod := newHostingModule(t, ctx)

	got := GetBufferBytesShim(ctx, mod, host, uint32(WasmBufferTypeHttpRequestBody), 0, 10, 64, 128)
	if got != WasmResultBadArgument {
		t.Fatalf("callback-error path = %v, want WasmResultBadArgument", got)
	}
}

// TestBodyBridge_GetBufferBytes_WasmBufferTypes verifies each of the 3
// activated WasmBufferType values (0/1/4) dispatches correctly per §11.1.
func TestBodyBridge_GetBufferBytes_WasmBufferTypes(t *testing.T) {
	ctx := context.Background()
	type tc struct {
		name string
		bt   WasmBufferType
	}
	cases := []tc{
		{"HttpRequestBody=0", WasmBufferTypeHttpRequestBody},
		{"HttpResponseBody=1", WasmBufferTypeHttpResponseBody},
		{"HttpCallResponseBody=4", WasmBufferTypeHttpCallResponseBody},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host := newFakeBufferHost()
			canned := []byte(fmt.Sprintf("type=%d", c.bt))
			host.seed(c.bt, canned)

			mod := newHostingModule(t, ctx)
			got := GetBufferBytesShim(ctx, mod, host, uint32(c.bt), 0, uint32(len(canned)), 64, 128)
			if got != WasmResultOk {
				t.Fatalf("dispatch result for bt=%d: %v, want Ok", c.bt, got)
			}
			gotLen, _ := mod.Memory().ReadUint32Le(128)
			if gotLen != uint32(len(canned)) {
				t.Fatalf("len for bt=%d = %d, want %d", c.bt, gotLen, len(canned))
			}
		})
	}
}

// TestBodyBridge_GetBufferBytes_UnactivatedBufferType verifies that
// WasmBufferType values in the spec roster but NOT activated at 25.2 (2, 3,
// 5, 6, 7, 8) also return BadArgument per §11.1 ("Values 2/3/5-8 remain
// inactive in 25.2").
func TestBodyBridge_GetBufferBytes_UnactivatedBufferType(t *testing.T) {
	ctx := context.Background()
	for _, bt := range []uint32{2, 3, 5, 6, 7, 8} {
		t.Run(fmt.Sprintf("type=%d", bt), func(t *testing.T) {
			host := newFakeBufferHost()
			mod := newHostingModule(t, ctx)
			got := GetBufferBytesShim(ctx, mod, host, bt, 0, 5, 64, 128)
			if got != WasmResultBadArgument {
				t.Fatalf("unactivated type %d = %v, want BadArgument", bt, got)
			}
		})
	}
}

// TestBodyBridge_GetBufferBytes_NonHostHostValue verifies the dispatch
// guard: if Host25_2 is not satisfied by a `bufferHost` impl, the shim
// returns WasmResultInternalFailure.
func TestBodyBridge_GetBufferBytes_NonHostHostValue(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	got := GetBufferBytesShim(ctx, mod, "not-a-host", uint32(WasmBufferTypeHttpRequestBody), 0, 5, 64, 128)
	if got != WasmResultInternalFailure {
		t.Fatalf("non-host host value = %v, want WasmResultInternalFailure", got)
	}
}

// --- TestBodyBridge_SetBufferBytes ---------------------------------------

func TestBodyBridge_SetBufferBytes_RoundTrip(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	mod := newHostingModule(t, ctx)

	// Write the source payload into linear memory at offset 256 so the
	// shim's Memory().Read fetches it.
	src := []byte("hello-world-payload")
	if !mod.Memory().Write(256, src) {
		t.Fatalf("seed memory failed")
	}

	got := SetBufferBytesShim(ctx, mod, host, uint32(WasmBufferTypeHttpRequestBody), 0, uint32(len(src)), 256, uint32(len(src)))
	if got != WasmResultOk {
		t.Fatalf("SetBufferBytesShim = %v, want Ok", got)
	}
	if host.setLastStreamCtx != host.ctxID {
		t.Errorf("BufferSet stream-ctx = %d, want %d", host.setLastStreamCtx, host.ctxID)
	}
	if host.setLastType != WasmBufferTypeHttpRequestBody {
		t.Errorf("BufferSet type = %d, want %d", host.setLastType, WasmBufferTypeHttpRequestBody)
	}
	if host.setLastStart != 0 {
		t.Errorf("BufferSet start = %d, want 0", host.setLastStart)
	}
	if string(host.setLastData) != string(src) {
		t.Errorf("BufferSet data = %q, want %q", host.setLastData, src)
	}
}

func TestBodyBridge_SetBufferBytes_BadBufferType(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	mod := newHostingModule(t, ctx)
	got := SetBufferBytesShim(ctx, mod, host, 99, 0, 0, 0, 0)
	if got != WasmResultBadArgument {
		t.Fatalf("SetBufferBytesShim(type=99) = %v, want BadArgument", got)
	}
}

func TestBodyBridge_SetBufferBytes_EmptyDataIsOk(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	mod := newHostingModule(t, ctx)
	got := SetBufferBytesShim(ctx, mod, host, uint32(WasmBufferTypeHttpRequestBody), 0, 0, 0, 0)
	if got != WasmResultOk {
		t.Fatalf("SetBufferBytesShim(empty) = %v, want Ok", got)
	}
}

func TestBodyBridge_SetBufferBytes_CallbackErrorPropagates(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	host.setResult = WasmResultBadArgument
	mod := newHostingModule(t, ctx)
	got := SetBufferBytesShim(ctx, mod, host, uint32(WasmBufferTypeHttpRequestBody), 0, 0, 0, 0)
	if got != WasmResultBadArgument {
		t.Fatalf("SetBufferBytesShim(cb-err) = %v, want BadArgument (propagated)", got)
	}
}

// --- TestBodyBridge_GetBufferStatus --------------------------------------

func TestBodyBridge_GetBufferStatus_RoundTrip(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	host.seed(WasmBufferTypeHttpResponseBody, []byte("0123456789"))
	host.seedFlags(WasmBufferTypeHttpResponseBody, 0x7)
	mod := newHostingModule(t, ctx)

	const sizePtr uint32 = 64
	const flagsPtr uint32 = 128

	got := GetBufferStatusShim(ctx, mod, host, uint32(WasmBufferTypeHttpResponseBody), sizePtr, flagsPtr)
	if got != WasmResultOk {
		t.Fatalf("GetBufferStatusShim = %v, want Ok", got)
	}
	size, _ := mod.Memory().ReadUint32Le(sizePtr)
	flags, _ := mod.Memory().ReadUint32Le(flagsPtr)
	if size != 10 {
		t.Errorf("size = %d, want 10", size)
	}
	if flags != 0x7 {
		t.Errorf("flags = %#x, want 0x7", flags)
	}
}

func TestBodyBridge_GetBufferStatus_BadBufferType(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	mod := newHostingModule(t, ctx)
	got := GetBufferStatusShim(ctx, mod, host, 99, 64, 128)
	if got != WasmResultBadArgument {
		t.Fatalf("GetBufferStatusShim(type=99) = %v, want BadArgument", got)
	}
}

func TestBodyBridge_GetBufferStatus_CallbackError(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	host.statusErr = fmt.Errorf("synthetic status failure")
	mod := newHostingModule(t, ctx)
	got := GetBufferStatusShim(ctx, mod, host, uint32(WasmBufferTypeHttpRequestBody), 64, 128)
	if got != WasmResultBadArgument {
		t.Fatalf("GetBufferStatusShim(cb-err) = %v, want BadArgument", got)
	}
}
