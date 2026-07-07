// ABICallbacks interface + host-module wiring for the 47 proxy-wasm v0.2.1
// hostcalls per 25.1 SPEC §3.1 + §5 + parent §4.2 Option B.
//
// AT 25.2 (Task 3): the host-module wiring EXTENDED with 14 NEW
// env-namespace hostcalls per §5.1 + the gate-at-registration discipline
// per AMEND-B5 (denied capabilities → NOT registered on wazero Runtime;
// mirrors upstream cpp-host `wasm.cc:176-189` `_REGISTER_PROXY` macro). The
// 25.1 16 active hostcalls KEEP their gate-at-call-site discipline (denied
// returns WasmResult::InternalFailure; preserves byte-stable 25.1 behavior
// per the no-break invariant); the 14 NEW 25.2 hostcalls use the
// gate-at-registration discipline (denied → unknown-import-failure at
// module-instantiation OR wazero-trap on call). The 7 NEW callbacks (per
// §5.3) gate at `HasGlobalFunc` lookup time per `wasm.cc:238-247`
// `_GET_PROXY` macro — denied → reported as missing even if the guest
// exports the function.
//
// The host-module wiring registers two wazero host modules onto the VM's
// runtime:
//
//   - `env` namespace — the 16 active `proxy_*` env-namespace hostcalls
//     (§5.1 25.1) + the 14 NEW gated `proxy_*` env-namespace hostcalls
//     (§5.1 25.2) + the 9 STILL stub-Unimplemented hostcalls (§5.4;
//     shared-queue 4 + gRPC 5). Total registered when ALL 14 NEW
//     capabilities ALLOWED: 39 (16 + 14 + 9). The 23-stub roster at 25.1
//     SHRINKS to 9 at 25.2 — 14 stubs LIFT to gated active per §5.1.
//
//   - `wasi_snapshot_preview1` namespace — the 8 active WASI custom-shim
//     hostcalls (§5.2). Each wraps the corresponding `wasiXxx` free
//     function from `wasi.go` (Task 4); the VM satisfies the `wasiHost`
//     interface so the shims can route logs + check capabilities through
//     it. UNCHANGED at 25.2.
//
// # 25.1 active hostcall body (gate-at-call-site discipline; UNCHANGED)
//
// Each 25.1 active hostcall body:
//
//	1. Checks vm.sandbox.IsAllowed(<cap_key>); if false:
//	   - proxy_* hostcalls: return uint32(abi.WasmResultInternalFailure) (=10)
//	   - wasi_*  hostcalls: return uint32(abi.WasiErrnoNotcapable) (=76)
//	2. Wraps the ABICallbacks invocation in vm.runWithPanicWrapper so a
//	   genuine Go panic in the bridge callback converts to
//	   abi.WasmResultInternalFailure (=10) + invokes vm.panicH.
//	3. Uses mod.Memory().Read / .Write for guest memory access — every
//	   memory read/write that returns ok=false converts to
//	   abi.WasmResultInvalidMemoryAccess (=6).
//
// # 25.2 NEW active hostcall body (gate-at-registration discipline)
//
// Each 25.2 NEW hostcall body:
//
//	1. The IsAllowed check fires at REGISTRATION time (registerProxyHostcalls25_2):
//	   if denied, the host function is NOT registered on the `env` module.
//	   Guest modules importing the denied hostcall fail at module-
//	   instantiation with "unknown import" OR (if a different builder
//	   provided a partial registration) the call traps. NO runtime-deny
//	   sentinel is emitted (in contrast to 25.1's WasmResultInternalFailure).
//	2. If registered, the body delegates to a per-family abi/* dispatch
//	   shim (e.g. `abi.GetBufferBytesShim` for proxy_get_buffer_bytes).
//	   At Task 3 the shims are PLACEHOLDERS that panic — replaced with real
//	   impls at Tasks 4/5/6/7/8/12 as each family lands.
//	3. The shim invocation is wrapped in vm.runWithPanicWrapper so the
//	   Task 3-window panic surfaces as WasmResultInternalFailure (=10) +
//	   invokes vm.panicH — the same recovery discipline as the 25.1 shims.
//
// # Stub hostcall body (UNCHANGED)
//
// Each STILL-stub hostcall body (9 entries at 25.2) just returns
// `uint32(abi.WasmResultUnimplemented)` (=12) without consulting the
// sandbox — the import-section type-signature is what matters for module
// instantiation; the runtime behavior is a stub per parent §4.2 Option B.
//
// # Memory ABI for return-by-reference hostcalls
//
// Several hostcalls return variable-size data (header maps, properties,
// status values) via a guest-allocated buffer. The pattern is:
//
//	1. Host computes the return bytes (e.g. EncodePairs).
//	2. Host invokes the guest's allocator (`malloc` or
//	   `proxy_on_memory_allocate`) with the byte size.
//	3. Allocator returns a guest-memory offset.
//	4. Host writes the bytes at the offset.
//	5. Host writes (offset, size) to the host-provided (ptr_ptr, size_ptr).
//
// The `allocateGuestBuffer` helper encapsulates steps 2-4. If neither
// `malloc` nor `proxy_on_memory_allocate` is exported, the hostcall returns
// `abi.WasmResultInvalidMemoryAccess` (=6) — the guest cannot receive the
// data without an allocator.
//
// # Hostcall roster
//
// At 25.1: 47 total = 16 active proxy_* + 8 active wasi_* + 23 deferred stubs.
// At 25.2: 47 total = 30 active proxy_* (16 25.1 + 14 NEW gated) + 8 active
// wasi_* + 9 STILL stub-Unimplemented (shared-queue 4 + gRPC 5). The
// register-count is INVARIANT (Option B per parent §4.2) — what changes at
// 25.2 is the 14-stub LIFT to gated-active, plus the conditional skip on
// the 14 NEW when their capability is denied (gate-at-registration per
// AMEND-B5). The maximum-allowed registered count is 47; the minimum
// (deny-all 14 NEW) is 33.
//
// The package-private `cap*` constants for the gated hostcalls live in
// sandbox.go (proxy_*) + wasi.go (wasi_*). Capability keys are referenced
// by bare name (the `proxy_` or no prefix) — wazero's host-module name
// (`env` / `wasi_snapshot_preview1`) is the registration-time namespace.

package wasm

import (
	"context"
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// ABICallbacks is the consumer-side interface the framework primitive
// invokes on the appropriate hostcall dispatch. The HTTP-filter package
// (internal/filter/http/wasm/) implements this interface for the per-stream
// HTTP context shape at Task 11; cluster-specifier-wasm + access-logger-wasm
// + the other §9 WASM host family consumers will implement their own
// per-context shape variants per the EXPLICIT API-REVISION ALLOWANCE
// clause anchored at ADR-0202.
//
// At 25.1 the interface carries the 13-method headers-bridge subset
// (per SPEC §3.1 lines 339-426). All methods accept a context.Context that
// the per-stream filter dispatch threads through; the streamContextID
// argument identifies the stream context the hostcall fires within.
type ABICallbacks interface {
	// GetHeaderMap fetches the named header-map for the given context.
	// Returns sorted key/value pairs (for cross-side determinism per
	// parent §4.5 D6 guardrail (b)) + ok=false if the header-map type
	// is not available in this context.
	GetHeaderMap(ctx context.Context, streamContextID uint32, mapType abi.WasmHeaderMapType) (pairs []HeaderPair, ok bool)

	// GetHeaderMapValue fetches a single value from the named header-map.
	GetHeaderMapValue(ctx context.Context, streamContextID uint32, mapType abi.WasmHeaderMapType, key string) (value string, ok bool)

	// AddHeaderMapValue appends a value to the named header-map.
	AddHeaderMapValue(ctx context.Context, streamContextID uint32, mapType abi.WasmHeaderMapType, key, value string) abi.WasmResult

	// ReplaceHeaderMapValue replaces a value in the named header-map.
	ReplaceHeaderMapValue(ctx context.Context, streamContextID uint32, mapType abi.WasmHeaderMapType, key, value string) abi.WasmResult

	// RemoveHeaderMapValue removes a key from the named header-map.
	RemoveHeaderMapValue(ctx context.Context, streamContextID uint32, mapType abi.WasmHeaderMapType, key string) abi.WasmResult

	// SetHeaderMapPairs replaces all pairs in the named header-map.
	SetHeaderMapPairs(ctx context.Context, streamContextID uint32, mapType abi.WasmHeaderMapType, pairs []HeaderPair) abi.WasmResult

	// GetHeaderMapSize returns the number of pairs in the named header-map.
	GetHeaderMapSize(ctx context.Context, streamContextID uint32, mapType abi.WasmHeaderMapType) uint32

	// GetProperty fetches a property by path. Minimal property tree at
	// 25.1 (request.headers.*, response.headers.*, request.path,
	// request.method, request.host); 25.2 extends to full CEL surface.
	GetProperty(ctx context.Context, streamContextID uint32, path []string) (value []byte, ok bool)

	// SetProperty stores a property by path.
	SetProperty(ctx context.Context, streamContextID uint32, path []string, value []byte) abi.WasmResult

	// SendLocalResponse short-circuits to local-reply with the given
	// status + headers + body. Consumed at REUSE point 5 (parent §3.3).
	SendLocalResponse(ctx context.Context, streamContextID uint32, statusCode uint32, statusMsg, body string, additionalHeaders []HeaderPair, grpcStatus int32) abi.WasmResult

	// GetStatus reads the HTTP/gRPC status. Consumed inside
	// proxy_on_response_headers via ADR-0196's
	// EncoderFilterCallbacks.ResponseStatus() accessor (per D-P3 + R7).
	GetStatus(ctx context.Context, streamContextID uint32) (statusCode uint32, value []byte, ok bool)

	// Log emits a log line at the given level. Routes to the VM's
	// WithLogSink output.
	Log(ctx context.Context, streamContextID uint32, level abi.LogLevel, msg string)

	// GetLogLevel returns the active log level (v0.2.1-new hostcall).
	GetLogLevel(ctx context.Context) abi.LogLevel

	// GetCurrentTimeNanoseconds returns the wall-clock time. DEPRECATED
	// in v0.2.1 (guests are encouraged to use
	// wasi_snapshot_preview1.clock_time_get); implemented at 25.1 for
	// upstream byte-faithfulness.
	GetCurrentTimeNanoseconds(ctx context.Context) uint64

	// SetEffectiveContext switches the active context for the calling
	// VM. Used by timer + httpCall callbacks (25.2 territory) but
	// available at 25.1 for completeness.
	SetEffectiveContext(ctx context.Context, contextID uint32) abi.WasmResult

	// Done signals the guest is done with the named context.
	Done(ctx context.Context, contextID uint32) abi.WasmResult

	// --- 25.2 NEW (Task 4) body+buffer + stream-control surface ---------
	//
	// The accessors below carry the body+buffer hostcall dispatch
	// (proxy_get_buffer_bytes + proxy_set_buffer_bytes +
	// proxy_get_buffer_status per §5.1 #25-27) + the stream-control
	// dispatch (proxy_continue_stream + proxy_close_stream per §5.1
	// #28-29) into the consumer-side body accumulation. The
	// implementations land at Tasks 15 + 16 (HTTP-filter consumer);
	// Task 4 lands the interface signatures + the abi/* shim wire-up.

	// GetBuffer returns the current bytes of the buffer identified by
	// (streamContextID, bufferType). Returns a nil slice + nil error for
	// "empty buffer" (e.g., before any body chunk has arrived) — the
	// abi/body_bridge.go clamp logic treats a nil/empty slice as length
	// 0. A non-nil error signals an out-of-range bufferType or other
	// dispatch-layer failure that the host shim converts to
	// abi.WasmResultBadArgument per §11.1 clamp wire-contract.
	GetBuffer(ctx context.Context, streamContextID uint32, bufferType abi.WasmBufferType) ([]byte, error)

	// SetBuffer replaces a slice of the buffer identified by
	// (streamContextID, bufferType) starting at `start` with `data`.
	// Per cpp-host src/exports.cc:set_buffer_bytes semantics, the
	// consumer-side impl is responsible for the splice (extend / truncate
	// when start+len(data) extends past the buffer end). A non-nil error
	// from the consumer converts to abi.WasmResultBadArgument at the host
	// shim.
	SetBuffer(ctx context.Context, streamContextID uint32, bufferType abi.WasmBufferType, start uint32, data []byte) abi.WasmResult

	// GetBufferStatus returns the current size + flag bits for the buffer
	// identified by (streamContextID, bufferType). Flag bits per
	// proxy-wasm v0.2.1 spec README §proxy_get_buffer_status (e.g., end-
	// of-stream, paused). A non-nil error from the consumer converts to
	// abi.WasmResultBadArgument at the host shim.
	GetBufferStatus(ctx context.Context, streamContextID uint32, bufferType abi.WasmBufferType) (size, flags uint32, err error)

	// ContinueStream resumes the named stream (request/response/upstream
	// per the streamType discriminator) after a prior PAUSE return from
	// proxy_on_*_body / proxy_on_*_trailers. The consumer-side impl
	// re-fires the corresponding filter-chain continue (e.g.,
	// FilterCallbacks.ContinueDecoding for request-stream type=0). A
	// non-nil error converts to WasmResultInternalFailure at the host
	// shim (continuation failures are exceptional + non-recoverable; the
	// guest can observe via the result code but no per-error sentinel
	// exists in v0.2.1).
	ContinueStream(ctx context.Context, streamContextID uint32, streamType uint32) abi.WasmResult

	// CloseStream tears down the named stream early. Mirrors the cpp-host
	// proxy_close_stream wire-contract — typically used to half-close the
	// upstream side or short-circuit out of the body-callback dispatch.
	// A non-nil error from the consumer converts to
	// WasmResultInternalFailure at the host shim.
	CloseStream(ctx context.Context, streamContextID uint32, streamType uint32) abi.WasmResult
}

// registerHostModules registers the 16 active proxy_* env-namespace
// hostcalls + the 8 active wasi_*-namespace custom-shim hostcalls + the 23
// deferred-25.2/25.3 stub-Unimplemented hostcalls (47 total per parent
// §4.2 Option B) onto the VM's wazero.Runtime.
//
// Called eagerly by NewVM during construction so subsequent Run-time
// instantiations resolve the import section against the registered modules.
// Returns a wrapped wazero error on registration failure (would indicate
// a programmer error — a malformed function signature).
func registerHostModules(ctx context.Context, vm *RootVM) error {
	envBuilder := vm.runtime.NewHostModuleBuilder("env")
	registerProxyHostcalls(envBuilder, vm)
	registerProxyHostcalls25_2(envBuilder, vm)
	registerDeferredStubs(envBuilder)
	if _, err := envBuilder.Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiate env host module: %w", err)
	}

	wasiBuilder := vm.runtime.NewHostModuleBuilder("wasi_snapshot_preview1")
	registerWasiHostcalls(wasiBuilder, vm)
	if _, err := wasiBuilder.Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiate wasi_snapshot_preview1 host module: %w", err)
	}

	return nil
}

// registerProxyHostcalls registers the 16 active proxy_* env-namespace
// hostcalls per §5.1. Each body checks sandbox.IsAllowed + invokes the
// ABICallbacks under the panic-wrapper.
func registerProxyHostcalls(b wazero.HostModuleBuilder, vm *RootVM) {
	// 1. proxy_log(level, msg_ptr, msg_size) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, level, msgPtr, msgSize uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxyLog) {
				vm.LogProxy(abi.LogLevelError, "hostcall denied: proxy_log")
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				msg, ok := readString(mod, msgPtr, msgSize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				//nolint:gosec // wire-protocol enum decode.
				vm.cb.Log(ctx, vm.currentCtxID.Load(), abi.LogLevel(int32(level)), msg)
				return abi.WasmResultOk
			}))
		}).
		Export("proxy_log")

	// 2. proxy_get_log_level(result_level_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, resultPtr uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxyGetLogLevel) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				lvl := vm.cb.GetLogLevel(ctx)
				if !mod.Memory().WriteUint32Le(resultPtr, uint32(lvl)) {
					return abi.WasmResultInvalidMemoryAccess
				}
				return abi.WasmResultOk
			}))
		}).
		Export("proxy_get_log_level")

	// 3. proxy_send_local_response(status_code, status_msg_ptr, status_msg_size,
	//    body_ptr, body_size, additional_headers_ptr, additional_headers_size,
	//    grpc_status) -> i32  (8 args)
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, statusCode, statusMsgPtr, statusMsgSize, bodyPtr, bodySize, addlHdrsPtr, addlHdrsSize, grpcStatus uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxySendLocalResponse) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				statusMsg, ok := readString(mod, statusMsgPtr, statusMsgSize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				body, ok := readString(mod, bodyPtr, bodySize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				var addlHdrs []HeaderPair
				if addlHdrsSize > 0 {
					raw, ok := mod.Memory().Read(addlHdrsPtr, addlHdrsSize)
					if !ok {
						return abi.WasmResultInvalidMemoryAccess
					}
					decoded, err := DecodePairs(raw)
					if err != nil {
						return abi.WasmResultBadArgument
					}
					addlHdrs = decoded
				}
				//nolint:gosec // grpc_status is wire-defined as signed int32; conversion is intentional.
				return vm.cb.SendLocalResponse(ctx, vm.currentCtxID.Load(), statusCode, statusMsg, body, addlHdrs, int32(grpcStatus))
			}))
		}).
		Export("proxy_send_local_response")

	// 4. proxy_get_header_map_pairs(type, ptr_ptr, size_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, mapType, ptrPtr, sizePtr uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxyGetHeaderMapPairs) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				//nolint:gosec // wire-protocol enum decode.
				pairs, ok := vm.cb.GetHeaderMap(ctx, vm.currentCtxID.Load(), abi.WasmHeaderMapType(int32(mapType)))
				if !ok {
					return abi.WasmResultNotFound
				}
				encoded := EncodePairs(pairs)
				return writeReturnBuffer(ctx, mod, encoded, ptrPtr, sizePtr)
			}))
		}).
		Export("proxy_get_header_map_pairs")

	// 5. proxy_set_header_map_pairs(type, ptr, size) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, mapType, ptr, size uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxySetHeaderMapPairs) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				raw, ok := mod.Memory().Read(ptr, size)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				pairs, err := DecodePairs(raw)
				if err != nil {
					return abi.WasmResultBadArgument
				}
				//nolint:gosec // wire-protocol enum decode.
				return vm.cb.SetHeaderMapPairs(ctx, vm.currentCtxID.Load(), abi.WasmHeaderMapType(int32(mapType)), pairs)
			}))
		}).
		Export("proxy_set_header_map_pairs")

	// 6. proxy_get_header_map_value(type, key_ptr, key_size, value_ptr_ptr, value_size_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, mapType, keyPtr, keySize, valuePtrPtr, valueSizePtr uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxyGetHeaderMapValue) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				key, ok := readString(mod, keyPtr, keySize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				//nolint:gosec // wire-protocol enum decode.
				val, ok := vm.cb.GetHeaderMapValue(ctx, vm.currentCtxID.Load(), abi.WasmHeaderMapType(int32(mapType)), key)
				if !ok {
					return abi.WasmResultNotFound
				}
				return writeReturnBuffer(ctx, mod, []byte(val), valuePtrPtr, valueSizePtr)
			}))
		}).
		Export("proxy_get_header_map_value")

	// 7. proxy_add_header_map_value(type, key_ptr, key_size, value_ptr, value_size) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, mapType, keyPtr, keySize, valuePtr, valueSize uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxyAddHeaderMapValue) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				key, ok := readString(mod, keyPtr, keySize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				val, ok := readString(mod, valuePtr, valueSize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				//nolint:gosec // wire-protocol enum decode.
				return vm.cb.AddHeaderMapValue(ctx, vm.currentCtxID.Load(), abi.WasmHeaderMapType(int32(mapType)), key, val)
			}))
		}).
		Export("proxy_add_header_map_value")

	// 8. proxy_replace_header_map_value(type, key_ptr, key_size, value_ptr, value_size) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, mapType, keyPtr, keySize, valuePtr, valueSize uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxyReplaceHeaderMapValue) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				key, ok := readString(mod, keyPtr, keySize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				val, ok := readString(mod, valuePtr, valueSize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				//nolint:gosec // wire-protocol enum decode.
				return vm.cb.ReplaceHeaderMapValue(ctx, vm.currentCtxID.Load(), abi.WasmHeaderMapType(int32(mapType)), key, val)
			}))
		}).
		Export("proxy_replace_header_map_value")

	// 9. proxy_remove_header_map_value(type, key_ptr, key_size) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, mapType, keyPtr, keySize uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxyRemoveHeaderMapValue) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				key, ok := readString(mod, keyPtr, keySize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				//nolint:gosec // wire-protocol enum decode.
				return vm.cb.RemoveHeaderMapValue(ctx, vm.currentCtxID.Load(), abi.WasmHeaderMapType(int32(mapType)), key)
			}))
		}).
		Export("proxy_remove_header_map_value")

	// 10. proxy_get_header_map_size(type, result_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, mapType, resultPtr uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxyGetHeaderMapSize) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				//nolint:gosec // wire-protocol enum decode.
				size := vm.cb.GetHeaderMapSize(ctx, vm.currentCtxID.Load(), abi.WasmHeaderMapType(int32(mapType)))
				if !mod.Memory().WriteUint32Le(resultPtr, size) {
					return abi.WasmResultInvalidMemoryAccess
				}
				return abi.WasmResultOk
			}))
		}).
		Export("proxy_get_header_map_size")

	// 11. proxy_get_property(path_ptr, path_size, value_ptr_ptr, value_size_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, pathPtr, pathSize, valuePtrPtr, valueSizePtr uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxyGetProperty) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				pathBytes, ok := mod.Memory().Read(pathPtr, pathSize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				path := splitPath(pathBytes)
				val, ok := vm.cb.GetProperty(ctx, vm.currentCtxID.Load(), path)
				if !ok {
					return abi.WasmResultNotFound
				}
				return writeReturnBuffer(ctx, mod, val, valuePtrPtr, valueSizePtr)
			}))
		}).
		Export("proxy_get_property")

	// 12. proxy_set_property(key_ptr, key_size, value_ptr, value_size) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, keyPtr, keySize, valuePtr, valueSize uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxySetProperty) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				pathBytes, ok := mod.Memory().Read(keyPtr, keySize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				path := splitPath(pathBytes)
				val, ok := mod.Memory().Read(valuePtr, valueSize)
				if !ok {
					return abi.WasmResultInvalidMemoryAccess
				}
				// Defensive copy: wazero returns a view into the live memory.
				valCopy := make([]byte, len(val))
				copy(valCopy, val)
				return vm.cb.SetProperty(ctx, vm.currentCtxID.Load(), path, valCopy)
			}))
		}).
		Export("proxy_set_property")

	// 13. proxy_get_status(code_ptr, value_ptr_ptr, value_size_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, codePtr, valuePtrPtr, valueSizePtr uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxyGetStatus) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				code, val, ok := vm.cb.GetStatus(ctx, vm.currentCtxID.Load())
				if !ok {
					return abi.WasmResultNotFound
				}
				if !mod.Memory().WriteUint32Le(codePtr, code) {
					return abi.WasmResultInvalidMemoryAccess
				}
				return writeReturnBuffer(ctx, mod, val, valuePtrPtr, valueSizePtr)
			}))
		}).
		Export("proxy_get_status")

	// 14. proxy_set_effective_context(context_id) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, _ api.Module, contextID uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxySetEffectiveContext) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				result := vm.cb.SetEffectiveContext(ctx, contextID)
				if result == abi.WasmResultOk {
					vm.currentCtxID.Store(contextID)
				}
				return result
			}))
		}).
		Export("proxy_set_effective_context")

	// 15. proxy_done() -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, _ api.Module) uint32 {
			if !vm.sandbox.IsAllowed(capProxyDone) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				return vm.cb.Done(ctx, vm.currentCtxID.Load())
			}))
		}).
		Export("proxy_done")

	// 16. proxy_get_current_time_nanoseconds(result_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, resultPtr uint32) uint32 {
			if !vm.sandbox.IsAllowed(capProxyGetCurrentTimeNanoseconds) {
				return uint32(abi.WasmResultInternalFailure)
			}
			return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
				ns := vm.cb.GetCurrentTimeNanoseconds(ctx)
				if !mod.Memory().WriteUint64Le(resultPtr, ns) {
					return abi.WasmResultInvalidMemoryAccess
				}
				return abi.WasmResultOk
			}))
		}).
		Export("proxy_get_current_time_nanoseconds")
}

// registerWasiHostcalls registers the 8 wasi_snapshot_preview1.* shims per
// §5.2. Each wraps the corresponding `wasiXxx` free function from wasi.go
// (Task 4) with vm-as-wasiHost adapter; the shims own the IsAllowed check.
func registerWasiHostcalls(b wazero.HostModuleBuilder, vm *RootVM) {
	// 17. fd_write(fd, iovec, iovec_size, nwritten_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, fd, iovsPtr, iovsLen, nwrittenPtr uint32) uint32 {
			//nolint:gosec // WasiErrno is int32; uint32 cast is the wire-protocol intent.
			return uint32(wasiFdWrite(ctx, mod, vm, fd, iovsPtr, iovsLen, nwrittenPtr))
		}).
		Export("fd_write")

	// 18. clock_time_get(clock_id, precision, time_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, clockID uint32, precision uint64, timePtr uint32) uint32 {
			//nolint:gosec // WasiErrno is int32; uint32 cast is the wire-protocol intent.
			return uint32(wasiClockTimeGet(ctx, mod, vm, clockID, precision, timePtr))
		}).
		Export("clock_time_get")

	// 19. random_get(buffer, buffer_size) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, bufferPtr, bufferSize uint32) uint32 {
			//nolint:gosec // WasiErrno is int32; uint32 cast is the wire-protocol intent.
			return uint32(wasiRandomGet(ctx, mod, vm, bufferPtr, bufferSize))
		}).
		Export("random_get")

	// 20. environ_sizes_get(num_elements_ptr, buffer_size_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, numElementsPtr, bufferSizePtr uint32) uint32 {
			//nolint:gosec // WasiErrno is int32; uint32 cast is the wire-protocol intent.
			return uint32(wasiEnvironSizesGet(ctx, mod, vm, numElementsPtr, bufferSizePtr))
		}).
		Export("environ_sizes_get")

	// 21. environ_get(argv_ptr, buffer_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, argvPtr, bufferPtr uint32) uint32 {
			//nolint:gosec // WasiErrno is int32; uint32 cast is the wire-protocol intent.
			return uint32(wasiEnvironGet(ctx, mod, vm, argvPtr, bufferPtr))
		}).
		Export("environ_get")

	// 22. args_sizes_get(argc_ptr, argv_buf_size_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, argcPtr, argvBufSizePtr uint32) uint32 {
			//nolint:gosec // WasiErrno is int32; uint32 cast is the wire-protocol intent.
			return uint32(wasiArgsSizesGet(ctx, mod, vm, argcPtr, argvBufSizePtr))
		}).
		Export("args_sizes_get")

	// 23. args_get(argv_ptr, argv_buf_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, argvPtr, argvBufPtr uint32) uint32 {
			//nolint:gosec // WasiErrno is int32; uint32 cast is the wire-protocol intent.
			return uint32(wasiArgsGet(ctx, mod, vm, argvPtr, argvBufPtr))
		}).
		Export("args_get")

	// 24. proc_exit(exit_code) — never returns to guest; ExitError trap.
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, exitCode uint32) {
			// The Go signature has no return type — wazero will treat the
			// returned error from wasiProcExit as a trap. We can't easily
			// return an error from a WithFunc void-return; the workaround is
			// to use the panic mechanism wazero translates to a trap, OR
			// switch to WithGoModuleFunction. For 25.1 simplicity: panic
			// with a *sys.ExitError-equivalent (wazero recognizes
			// sys.ExitError via panic in some paths). Simpler approach used
			// here: re-implement the body inline to avoid the void-return
			// limitation.
			//
			// Since wasiProcExit returns an error of type *sys.ExitError,
			// and we want wazero to treat it as a trap, we panic with it —
			// wazero's panic-recovery wraps it as a trap.
			err := wasiProcExit(ctx, mod, vm, exitCode)
			if err != nil {
				panic(err)
			}
		}).
		Export("proc_exit")
}

// registerProxyHostcalls25_2 registers the 14 NEW 25.2 env-namespace proxy_*
// hostcalls per §5.1 + AMEND-B1..B5. EACH hostcall is GATED at registration
// time per AMEND-B5: if `vm.sandbox.IsAllowed(<cap>)` returns false the
// host function is NOT registered on the wazero Runtime; guests importing
// the denied hostcall fail at module-instantiation OR (if a different
// path provides a partial registration) the call traps. Mirrors upstream
// cpp-host `wasm.cc:176-189` `_REGISTER_PROXY` macro discipline.
//
// Each function body delegates to a forward-decl shim in `internal/wasm/abi/
// stubs_25_2.go`. At Task 3 the shims PANIC with "Task N not yet landed"
// — the real impls land at Tasks 4/5/6/7/8/12 (each Task DELETES its
// placeholder line from stubs_25_2.go + adds the real impl).
//
// Wire signatures pinned per 25.2 SPEC §11.1 (body+buffer+trailers) +
// §11.2 (metrics) + §11.3 (proxy_http_call) + §11.5 (capability gating).
//
// obscure the gate-at-registration discipline (each entry is the
// SAME 5-line structure: cap-check, NewFunctionBuilder, WithFunc shim,
// Export). Per-family file extraction belongs at Tasks 4-12 when each
// abi/* family file lands its real impl.
//
//nolint:funlen // 14 hostcall envelopes; splitting per-family would
func registerProxyHostcalls25_2(b wazero.HostModuleBuilder, vm *RootVM) {
	// 25. proxy_get_buffer_bytes(buffer_type, start, max_size, ret_data_ptr,
	//     ret_size_ptr) -> i32 — Task 4 lands real impl (AMEND-B1 clamp).
	if vm.sandbox.IsAllowed(capProxyGetBufferBytes) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, bufferType, start, maxSize, retDataPtr, retSizePtr uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.GetBufferBytesShim(ctx, mod, vm, bufferType, start, maxSize, retDataPtr, retSizePtr)
				}))
			}).
			Export("proxy_get_buffer_bytes")
	}

	// 26. proxy_set_buffer_bytes(buffer_type, start, size, data_ptr,
	//     data_size) -> i32 — Task 4.
	if vm.sandbox.IsAllowed(capProxySetBufferBytes) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, bufferType, start, size, dataPtr, dataSize uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.SetBufferBytesShim(ctx, mod, vm, bufferType, start, size, dataPtr, dataSize)
				}))
			}).
			Export("proxy_set_buffer_bytes")
	}

	// 27. proxy_get_buffer_status(buffer_type, ret_size_ptr, ret_flags_ptr)
	//     -> i32 — Task 4.
	if vm.sandbox.IsAllowed(capProxyGetBufferStatus) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, bufferType, retSizePtr, retFlagsPtr uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.GetBufferStatusShim(ctx, mod, vm, bufferType, retSizePtr, retFlagsPtr)
				}))
			}).
			Export("proxy_get_buffer_status")
	}

	// 28. proxy_continue_stream(stream_type) -> i32 — Task 4 (paired with
	//     PAUSE-buffer dispatch on body callbacks).
	if vm.sandbox.IsAllowed(capProxyContinueStream) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, streamType uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.ContinueStreamShim(ctx, mod, vm, streamType)
				}))
			}).
			Export("proxy_continue_stream")
	}

	// 29. proxy_close_stream(stream_type) -> i32 — Task 4.
	if vm.sandbox.IsAllowed(capProxyCloseStream) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, streamType uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.CloseStreamShim(ctx, mod, vm, streamType)
				}))
			}).
			Export("proxy_close_stream")
	}

	// 30. proxy_set_tick_period_milliseconds(period_ms) -> i32 — Task 5
	//     (Q5 10ms envoy-go-strict floor; Clock seam FIRST co-consumer).
	if vm.sandbox.IsAllowed(capProxySetTickPeriodMilliseconds) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, periodMs uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.SetTickPeriodMillisecondsShim(ctx, mod, vm, periodMs)
				}))
			}).
			Export("proxy_set_tick_period_milliseconds")
	}

	// 31. proxy_define_metric(metric_type, name_data, name_size,
	//     ret_metric_id_ptr) -> i32 — Task 12 (AMEND-B2: Counter=0/Gauge=1/
	//     Histogram=2; namespace wasmcustom.<name>).
	if vm.sandbox.IsAllowed(capProxyDefineMetric) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, metricType, nameData, nameSize, retMetricIDPtr uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.DefineMetricShim(ctx, mod, vm, metricType, nameData, nameSize, retMetricIDPtr)
				}))
			}).
			Export("proxy_define_metric")
	}

	// 32. proxy_increment_metric(metric_id, delta:int64 SIGNED) -> i32
	//     — Task 12 (AMEND-B2 signed-i64 delta).
	if vm.sandbox.IsAllowed(capProxyIncrementMetric) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, metricID uint32, delta int64) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.IncrementMetricShim(ctx, mod, vm, metricID, delta)
				}))
			}).
			Export("proxy_increment_metric")
	}

	// 33. proxy_record_metric(metric_id, value:uint64 UNSIGNED) -> i32
	//     — Task 12 (AMEND-B2 unsigned-u64 value).
	if vm.sandbox.IsAllowed(capProxyRecordMetric) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, metricID uint32, value uint64) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.RecordMetricShim(ctx, mod, vm, metricID, value)
				}))
			}).
			Export("proxy_record_metric")
	}

	// 34. proxy_get_metric(metric_id, ret_value_ptr) -> i32 — Task 12.
	if vm.sandbox.IsAllowed(capProxyGetMetric) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, metricID, retValuePtr uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.GetMetricShim(ctx, mod, vm, metricID, retValuePtr)
				}))
			}).
			Export("proxy_get_metric")
	}

	// 35. proxy_set_shared_data(key_ptr, key_size, value_ptr, value_size,
	//     cas) -> i32 — Task 6 (Q6 CAS + 1 MiB value cap + 1024-entry cap).
	if vm.sandbox.IsAllowed(capProxySetSharedData) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, keyPtr, keySize, valuePtr, valueSize, cas uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.SetSharedDataShim(ctx, mod, vm, keyPtr, keySize, valuePtr, valueSize, cas)
				}))
			}).
			Export("proxy_set_shared_data")
	}

	// 36. proxy_get_shared_data(key_ptr, key_size, ret_value_ptr_ptr,
	//     ret_value_size_ptr, ret_cas_ptr) -> i32 — Task 6.
	if vm.sandbox.IsAllowed(capProxyGetSharedData) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, keyPtr, keySize, retValuePtrPtr, retValueSizePtr, retCASPtr uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.GetSharedDataShim(ctx, mod, vm, keyPtr, keySize, retValuePtrPtr, retValueSizePtr, retCASPtr)
				}))
			}).
			Export("proxy_get_shared_data")
	}

	// 37. proxy_http_call(cluster_data, cluster_size, headers_data,
	//     headers_size, body_data, body_size, trailers_data, trailers_size,
	//     timeout_ms, ret_call_id_ptr) -> i32 (10 args; timeout_ms = uint32)
	//     — Task 8 (Q4 + AMEND-B3: BadArgument-on-unknown-cluster;
	//     cancel-at-destruction; http_call_response_after_close counter).
	if vm.sandbox.IsAllowed(capProxyHttpCall) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, clusterData, clusterSize, headersData, headersSize, bodyData, bodySize, trailersData, trailersSize, timeoutMs, retCallIDPtr uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.HttpCallShim(ctx, mod, vm, clusterData, clusterSize, headersData, headersSize, bodyData, bodySize, trailersData, trailersSize, timeoutMs, retCallIDPtr)
				}))
			}).
			Export("proxy_http_call")
	}

	// 38. proxy_call_foreign_function(name_data, name_size, args_data,
	//     args_size, ret_results_data_ptr, ret_results_size_ptr) -> i32
	//     — Task 7 (AMEND-A9: NotFound for unregistered; EMPTY default
	//     registry; envoy-go-strict departure record #4).
	if vm.sandbox.IsAllowed(capProxyCallForeignFunction) {
		b.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, nameData, nameSize, argsData, argsSize, retResultsDataPtr, retResultsSizePtr uint32) uint32 {
				return uint32(vm.runWithPanicWrapper(func() abi.WasmResult {
					return abi.CallForeignFunctionShim(ctx, mod, vm, nameData, nameSize, argsData, argsSize, retResultsDataPtr, retResultsSizePtr)
				}))
			}).
			Export("proxy_call_foreign_function")
	}
}

// registerDeferredStubs registers the 9 STILL-deferred hostcalls per §5.4
// + parent §4.2 Option B. At 25.1 this was 23 stubs; 14 of those LIFTED at
// 25.2 (registered via registerProxyHostcalls25_2 above) leaving 9 stubs:
// shared-queue family (4) + gRPC family (5). Each returns
// uint32(abi.WasmResultUnimplemented) (=12) without consulting the sandbox.
// The import-section type-signature is what matters for module instantiation;
// the runtime behavior is a stub. The signatures match upstream proxy-wasm-
// cpp-host's include/proxy-wasm/exports.h at SHA da3ce05d.
func registerDeferredStubs(b wazero.HostModuleBuilder) {
	stub := func() uint32 { return uint32(abi.WasmResultUnimplemented) }

	// shared-queue (4)
	// proxy_register_shared_queue(queue_name_ptr, queue_name_size, token_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(_, _, _ uint32) uint32 { return stub() }).
		Export("proxy_register_shared_queue")
	// proxy_resolve_shared_queue(vm_id_ptr, vm_id_size, queue_name_ptr, queue_name_size, token_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(_, _, _, _, _ uint32) uint32 { return stub() }).
		Export("proxy_resolve_shared_queue")
	// proxy_dequeue_shared_queue(token, data_ptr_ptr, data_size_ptr) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(_, _, _ uint32) uint32 { return stub() }).
		Export("proxy_dequeue_shared_queue")
	// proxy_enqueue_shared_queue(token, data_ptr, data_size) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(_, _, _ uint32) uint32 { return stub() }).
		Export("proxy_enqueue_shared_queue")

	// outbound gRPC (5)
	// proxy_grpc_call(service_ptr, service_size, service_name_ptr, service_name_size,
	//                 method_name_ptr, method_name_size, initial_metadata_ptr, initial_metadata_size,
	//                 request_ptr, request_size, timeout_milliseconds, token_ptr) -> i32  (12 args)
	b.NewFunctionBuilder().
		WithFunc(func(_, _, _, _, _, _, _, _, _, _, _, _ uint32) uint32 { return stub() }).
		Export("proxy_grpc_call")
	// proxy_grpc_stream(service_ptr, service_size, service_name_ptr, service_name_size,
	//                   method_name_ptr, method_name_size, initial_metadata_ptr, initial_metadata_size,
	//                   token_ptr) -> i32  (9 args)
	b.NewFunctionBuilder().
		WithFunc(func(_, _, _, _, _, _, _, _, _ uint32) uint32 { return stub() }).
		Export("proxy_grpc_stream")
	// proxy_grpc_send(token, message_ptr, message_size, end_stream) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(_, _, _, _ uint32) uint32 { return stub() }).
		Export("proxy_grpc_send")
	// proxy_grpc_cancel(token) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(_ uint32) uint32 { return stub() }).
		Export("proxy_grpc_cancel")
	// proxy_grpc_close(token) -> i32
	b.NewFunctionBuilder().
		WithFunc(func(_ uint32) uint32 { return stub() }).
		Export("proxy_grpc_close")
}

// --- helpers --------------------------------------------------------------

// readString reads a string from guest memory. Returns ("", false) on OOB.
// size==0 returns ("", true) as a valid empty-string read.
func readString(mod api.Module, ptr, size uint32) (string, bool) {
	if size == 0 {
		return "", true
	}
	buf, ok := mod.Memory().Read(ptr, size)
	if !ok {
		return "", false
	}
	return string(buf), true
}

// splitPath splits a proxy_get_property / proxy_set_property path bytes
// into segments. Upstream proxy-wasm uses NUL (`\0`) as the path separator;
// some guests use `.` instead. We accept BOTH: NUL first (upstream); if the
// path contains no NULs, fall back to `.`. Empty path bytes ⇒ empty
// segment slice.
func splitPath(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	// Prefer NUL-separator (upstream proxy-wasm wire format).
	for _, c := range b {
		if c == 0 {
			parts := strings.Split(string(b), "\x00")
			// Filter empty trailing segment if the path is NUL-terminated.
			if len(parts) > 0 && parts[len(parts)-1] == "" {
				parts = parts[:len(parts)-1]
			}
			return parts
		}
	}
	return strings.Split(string(b), ".")
}

// writeReturnBuffer implements the standard proxy-wasm return-by-reference
// pattern: allocates a guest-memory buffer via the guest's allocator
// (`malloc` or `proxy_on_memory_allocate`), writes the bytes there, then
// writes the (offset, size) tuple to the host-provided (ptrPtr, sizePtr).
//
// Returns:
//   - WasmResultOk on success
//   - WasmResultInvalidMemoryAccess if (a) no allocator is exported,
//     (b) the allocator returns 0 (allocation failed), (c) the bytes write
//     fails, OR (d) the (offset, size) write fails.
//
// An empty payload (len(data)==0) writes (0, 0) to (ptrPtr, sizePtr)
// without invoking the allocator — matches upstream behavior for the
// "no data" case.
func writeReturnBuffer(ctx context.Context, mod api.Module, data []byte, ptrPtr, sizePtr uint32) abi.WasmResult {
	mem := mod.Memory()
	if len(data) == 0 {
		if !mem.WriteUint32Le(ptrPtr, 0) {
			return abi.WasmResultInvalidMemoryAccess
		}
		if !mem.WriteUint32Le(sizePtr, 0) {
			return abi.WasmResultInvalidMemoryAccess
		}
		return abi.WasmResultOk
	}

	allocator := mod.ExportedFunction("malloc")
	if allocator == nil {
		allocator = mod.ExportedFunction("proxy_on_memory_allocate")
	}
	if allocator == nil {
		return abi.WasmResultInvalidMemoryAccess
	}

	//nolint:gosec // len(data) bound is the guest's responsibility; oversize ⇒ alloc-fail.
	results, err := allocator.Call(ctx, uint64(uint32(len(data))))
	if err != nil || len(results) == 0 {
		return abi.WasmResultInvalidMemoryAccess
	}
	//nolint:gosec // wazero stack values are u64 wide; the guest's malloc returns a u32 pointer.
	offset := uint32(results[0])
	if offset == 0 {
		return abi.WasmResultInvalidMemoryAccess
	}
	if !mem.Write(offset, data) {
		return abi.WasmResultInvalidMemoryAccess
	}
	if !mem.WriteUint32Le(ptrPtr, offset) {
		return abi.WasmResultInvalidMemoryAccess
	}
	//nolint:gosec // len(data) is bounded by the allocator call we just made successfully.
	if !mem.WriteUint32Le(sizePtr, uint32(len(data))) {
		return abi.WasmResultInvalidMemoryAccess
	}
	return abi.WasmResultOk
}

// Compile-time guard: ensure *RootVM satisfies the wasiHost interface
// required by the wasi.go shims (IsAllowed + LogProxy methods). The 25.1
// *VM equivalent guard moved here as part of D-P-PLAN-6.
var _ wasiHost = (*RootVM)(nil)
