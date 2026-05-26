// Package abi — body_bridge.go: body+buffer hostcall dispatch shims per
// 25.2 SPEC §5.1 #25-27 + AMEND-B1 buffer-clamp wire-contract per R-25.2-1.
//
// # Three host shims landed at Task 4
//
//   - GetBufferBytesShim — proxy_get_buffer_bytes(buffer_type, start,
//     max_size, ret_data_ptr, ret_size_ptr) -> WasmResult
//   - SetBufferBytesShim — proxy_set_buffer_bytes(buffer_type, start, size,
//     data_ptr, data_size) -> WasmResult
//   - GetBufferStatusShim — proxy_get_buffer_status(buffer_type,
//     ret_size_ptr, ret_flags_ptr) -> WasmResult
//
// # AMEND-B1 buffer-clamp wire-contract (per R-25.2-1)
//
// The proxy-wasm v0.2.1 spec README text states proxy_get_buffer_bytes
// returns BadArgument "in case of buffer overflow due to invalid `start`
// and/or `max_size` values". The proxy-wasm-cpp-host REFUTES this strict
// reading: src/exports.cc:get_buffer_bytes clamps silently:
//
//	if start > buffer.size:        length = 0       (returns Ok)
//	elif start + length > buffer.size:  length = clamp   (returns Ok)
//	else:                          length unchanged (returns Ok)
//
// Only the i32-overflow path (start + max_size < start under uint32) returns
// BadArgument. envoy-go MUST mirror cpp-host because Istio + Envoy
// production guests rely on the clamp. The cpp-host source comparison cited
// at SPEC §11.1 D-25.2-1 is the authoritative reference; the golden table
// at body_bridge_test.go pins the 6 row contract.
//
// # WasmBufferType values activated at 25.2 (per §11.1)
//
//   - 0 = HttpRequestBody
//   - 1 = HttpResponseBody
//   - 4 = HttpCallResponseBody
//
// Values 2/3 (Downstream/Upstream data), 5 (GrpcReceive), 6 (VmConfiguration),
// 7 (PluginConfiguration), 8 (ForeignFunctionArguments) are RECOGNIZED in
// the WasmBufferType enum (per AMEND-A7) but NOT routed to a body backing
// at 25.2 — those values return BadArgument from the shims. Activations
// arrive at 25.3+ when the corresponding host families land.
//
// # Decoupling from internal/wasm
//
// abi/ MUST NOT import internal/wasm (would induce a cycle — wasm imports
// abi). Each shim accepts a `Host25_2 any` parameter (declared in
// stubs_25_2.go) carrying an opaque value the registration.go envelope
// passes through unchanged (typically `*RootVM`). The shim type-asserts to
// a package-private `bufferHost` interface; if the value does not satisfy
// the interface, the shim returns WasmResultInternalFailure. This pattern
// mirrors the cpp-host approach (host-callable bound to ContextBase at
// runtime via virtual dispatch); the Go equivalent is interface
// satisfaction at call time.
package abi

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// bufferHost is the package-private interface the body+buffer shims dispatch
// through. Satisfied by `*wasm.RootVM` (via methods added at Task 4 — see
// internal/wasm/host_bridge_25_2.go) so the registration.go envelope can
// forward its `vm` argument unchanged. The shim type-asserts the opaque
// Host25_2 argument to this interface; non-satisfaction returns
// WasmResultInternalFailure (programmer error — wrong host wired in).
type bufferHost interface {
	// CurrentCtxID returns the currently-active stream context id for the
	// in-flight hostcall dispatch.
	CurrentCtxID() uint32

	// BufferGet returns the current bytes of the buffer identified by
	// (streamContextID, bufferType). A non-nil error → BadArgument at the
	// shim per §11.1 ("unknown buffer_id ⇒ BadArgument"). A nil slice +
	// nil error is the empty-buffer case (length 0; valid clamp-to-zero
	// per AMEND-B1).
	BufferGet(ctx context.Context, streamCtxID uint32, bufferType WasmBufferType) ([]byte, error)

	// BufferSet splices `data` into the buffer at offset `start`. The
	// consumer-side impl handles extend/truncate per cpp-host
	// src/exports.cc:set_buffer_bytes. Returns the wire-result directly.
	BufferSet(ctx context.Context, streamCtxID uint32, bufferType WasmBufferType, start uint32, data []byte) WasmResult

	// BufferStatus returns size + flag bits for the buffer. flag bits per
	// proxy-wasm v0.2.1 spec README §proxy_get_buffer_status (e.g.,
	// end-of-stream, paused; landing at Task 16). Non-nil error →
	// BadArgument at the shim.
	BufferStatus(ctx context.Context, streamCtxID uint32, bufferType WasmBufferType) (size, flags uint32, err error)

	// AllocateGuestBuffer implements the standard return-by-reference
	// pattern: call the guest's allocator (malloc / proxy_on_memory_-
	// allocate), copy `data` to the returned offset, write (offset, size)
	// to (ptrPtr, sizePtr). Mirrors registration.go writeReturnBuffer
	// shape. Empty data writes (0, 0) without invoking the allocator.
	AllocateGuestBuffer(ctx context.Context, mod api.Module, data []byte, ptrPtr, sizePtr uint32) WasmResult
}

// activatedBufferType reports whether bt is one of the 3 WasmBufferType
// values activated at 25.2 (HttpRequestBody=0, HttpResponseBody=1,
// HttpCallResponseBody=4 per §11.1). Inactivated values + out-of-range
// integers both return false.
func activatedBufferType(bt WasmBufferType) bool {
	switch bt {
	case WasmBufferTypeHttpRequestBody, WasmBufferTypeHttpResponseBody, WasmBufferTypeHttpCallResponseBody:
		return true
	default:
		return false
	}
}

// GetBufferBytesShim — proxy_get_buffer_bytes dispatch per §5.1 #25 +
// AMEND-B1 buffer-clamp wire-contract per R-25.2-1.
//
// Wire-shape (cpp-host include/proxy-wasm/exports.h:107):
//
//	Word get_buffer_bytes(Word type, Word start, Word length,
//	                       Word ptr_ptr, Word size_ptr)
//
// AMEND-B1 clamp logic (byte-faithful to cpp-host src/exports.cc:get_buffer_-
// bytes):
//
//   - start + maxSize underflows uint32 (i32-overflow) ⇒ BadArgument
//   - start > len(buf)                                 ⇒ length = 0 + Ok
//   - start + maxSize > len(buf)                       ⇒ length = clamp + Ok
//   - else                                              ⇒ length = maxSize + Ok
//
// Per AMEND-B1 ONLY the i32-overflow path returns BadArgument. The
// start-beyond-end and clamp-on-overflow paths both return Ok with the
// truncated payload (length=0 in the start-beyond-end case; length=
// len(buf)-start in the clamp case).
//
// Unrecognized WasmBufferType ⇒ BadArgument (consistent with cpp-host
// "unknown buffer_id" disposition).
func GetBufferBytesShim(ctx context.Context, mod api.Module, host Host25_2, bufferType, start, maxSize, retDataPtr, retSizePtr uint32) WasmResult {
	bh, ok := host.(bufferHost)
	if !ok {
		return WasmResultInternalFailure
	}

	//nolint:gosec // wire-protocol enum decode; bufferType is an i32 from the guest.
	bt := WasmBufferType(int32(bufferType))
	if !activatedBufferType(bt) {
		return WasmResultBadArgument
	}

	// AMEND-B1: i32-overflow guard MUST come BEFORE fetching the buffer.
	// Per cpp-host the overflow check uses unsigned wrap detection:
	// `start + length < start` indicates wraparound under uint32.
	if start+maxSize < start {
		return WasmResultBadArgument
	}

	buf, err := bh.BufferGet(ctx, bh.CurrentCtxID(), bt)
	if err != nil {
		return WasmResultBadArgument
	}
	bufLen := uint32(len(buf))

	// AMEND-B1 clamp logic (cpp-host src/exports.cc:get_buffer_bytes
	// byte-faithful):
	var length uint32
	switch {
	case start >= bufLen:
		length = 0
	case start+maxSize > bufLen:
		length = bufLen - start
	default:
		length = maxSize
	}

	var slice []byte
	if length > 0 {
		slice = buf[start : start+length]
	}
	return bh.AllocateGuestBuffer(ctx, mod, slice, retDataPtr, retSizePtr)
}

// SetBufferBytesShim — proxy_set_buffer_bytes dispatch per §5.1 #26.
//
// Wire-shape (cpp-host include/proxy-wasm/exports.h:109):
//
//	Word set_buffer_bytes(Word type, Word start, Word length,
//	                       Word data_ptr, Word data_size)
//
// NOTE: the `length` field in the wire-shape is semantically the "old
// length being replaced" (a hint cpp-host accepts but ignores in the
// reference impl — the splice is bounded by data_size). envoy-go follows
// the same pattern: we ignore `size` and use `dataSize` as the bytes to
// splice. The consumer-side BufferSet does the extend/truncate.
//
// Unrecognized WasmBufferType ⇒ BadArgument. Memory-read failure on the
// guest data ⇒ InvalidMemoryAccess. Consumer BufferSet result propagated
// directly.
//
// dataSize==0 is a valid empty-splice (no Memory().Read invoked).
func SetBufferBytesShim(ctx context.Context, mod api.Module, host Host25_2, bufferType, start, size, dataPtr, dataSize uint32) WasmResult {
	_ = size // wire `length` arg ignored per cpp-host reference; dataSize is authoritative.

	bh, ok := host.(bufferHost)
	if !ok {
		return WasmResultInternalFailure
	}
	//nolint:gosec // wire-protocol enum decode.
	bt := WasmBufferType(int32(bufferType))
	if !activatedBufferType(bt) {
		return WasmResultBadArgument
	}

	var data []byte
	if dataSize > 0 {
		raw, ok := mod.Memory().Read(dataPtr, dataSize)
		if !ok {
			return WasmResultInvalidMemoryAccess
		}
		// Defensive copy: wazero returns a view into live memory; the
		// consumer-side splice may retain a reference past this call.
		data = make([]byte, len(raw))
		copy(data, raw)
	}

	return bh.BufferSet(ctx, bh.CurrentCtxID(), bt, start, data)
}

// GetBufferStatusShim — proxy_get_buffer_status dispatch per §5.1 #27.
//
// Wire-shape (cpp-host include/proxy-wasm/exports.h:108):
//
//	Word get_buffer_status(Word type, Word length_ptr, Word flags_ptr)
//
// Returns the current buffer size + flag bits at (retSizePtr, retFlagsPtr).
// flag bits per proxy-wasm v0.2.1 spec README §proxy_get_buffer_status
// (end-of-stream, paused, etc.) — the consumer-side impl at Task 16
// populates them.
//
// Unrecognized WasmBufferType ⇒ BadArgument. Memory-write failure ⇒
// InvalidMemoryAccess. Consumer-side error ⇒ BadArgument.
func GetBufferStatusShim(ctx context.Context, mod api.Module, host Host25_2, bufferType, retSizePtr, retFlagsPtr uint32) WasmResult {
	bh, ok := host.(bufferHost)
	if !ok {
		return WasmResultInternalFailure
	}
	//nolint:gosec // wire-protocol enum decode.
	bt := WasmBufferType(int32(bufferType))
	if !activatedBufferType(bt) {
		return WasmResultBadArgument
	}

	size, flags, err := bh.BufferStatus(ctx, bh.CurrentCtxID(), bt)
	if err != nil {
		return WasmResultBadArgument
	}
	mem := mod.Memory()
	if !mem.WriteUint32Le(retSizePtr, size) {
		return WasmResultInvalidMemoryAccess
	}
	if !mem.WriteUint32Le(retFlagsPtr, flags) {
		return WasmResultInvalidMemoryAccess
	}
	return WasmResultOk
}
