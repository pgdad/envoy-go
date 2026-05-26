// host_bridge_25_2.go — adapter methods on *RootVM that satisfy the
// package-private dispatch interfaces in internal/wasm/abi/ (bufferHost +
// streamHost as of Task 4; timerHost as of Task 5; sharedDataHost as of
// Task 6; foreignHost as of Task 7; httpCallHost as of Task 8; metricsHost
// as of Task 12).
//
// # Design intent
//
// The 14 NEW 25.2 hostcall dispatch shims live in internal/wasm/abi/ (per
// the Task 3 + Task 4..12 file-split). Each shim accepts an opaque
// `Host25_2 any` parameter that the registration.go envelope passes its
// `*RootVM` through unchanged. The shim type-asserts to a package-private
// interface (e.g., `bufferHost` for the body+buffer family) and dispatches.
//
// abi/ MUST NOT import internal/wasm (would induce a cycle — wasm imports
// abi). To make `*RootVM` satisfy abi/'s package-private interfaces
// structurally, this file defines adapter methods on `*RootVM` with the
// matching names + signatures. Each adapter is a thin wrapper around
// `rv.cb.*` (the ABICallbacks) + optional state on the RootVM itself
// (currentCtxID for the per-stream dispatch).
//
// # Goroutine-safety
//
// The adapter methods are invoked from the hostcall body INSIDE the held
// dispatchMu frame (per root_vm.go currentCtxID-tracking doc). Reads of
// rv.cb + rv.currentCtxID are race-free under the dispatchMu serialization
// discipline (the hostcall envelope in registration.go runs sync.Mutex
// while-held; the abi/ shim → adapter → ABICallbacks chain inherits the
// frame).

package wasm

import (
	"context"

	"github.com/tetratelabs/wazero/api"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// CurrentCtxID returns the in-flight stream context id for the hostcall
// dispatch (the same atomic Uint32 the 25.1 hostcalls read inline as
// `vm.currentCtxID.Load()`). Adapter for the abi/ bufferHost +
// streamHost interfaces.
func (rv *RootVM) CurrentCtxID() uint32 {
	return rv.currentCtxID.Load()
}

// --- bufferHost (abi/body_bridge.go) -------------------------------------

// BufferGet forwards to the consumer-side ABICallbacks.GetBuffer for the
// body+buffer hostcall (proxy_get_buffer_bytes per §5.1 #25). A non-nil
// error from the consumer surfaces at the shim as
// abi.WasmResultBadArgument per AMEND-B1.
func (rv *RootVM) BufferGet(ctx context.Context, streamCtxID uint32, bt abi.WasmBufferType) ([]byte, error) {
	return rv.cb.GetBuffer(ctx, streamCtxID, bt)
}

// BufferSet forwards to ABICallbacks.SetBuffer for the body-mutation
// hostcall (proxy_set_buffer_bytes per §5.1 #26).
func (rv *RootVM) BufferSet(ctx context.Context, streamCtxID uint32, bt abi.WasmBufferType, start uint32, data []byte) abi.WasmResult {
	return rv.cb.SetBuffer(ctx, streamCtxID, bt, start, data)
}

// BufferStatus forwards to ABICallbacks.GetBufferStatus for the
// buffer-state hostcall (proxy_get_buffer_status per §5.1 #27).
func (rv *RootVM) BufferStatus(ctx context.Context, streamCtxID uint32, bt abi.WasmBufferType) (uint32, uint32, error) {
	return rv.cb.GetBufferStatus(ctx, streamCtxID, bt)
}

// AllocateGuestBuffer adapts the return-by-reference allocation helper
// (registration.go writeReturnBuffer) to the abi/ bufferHost interface
// shape. Reuses the existing implementation byte-for-byte so the
// proxy_get_buffer_bytes shim follows the same allocator-discovery +
// (offset, size) write pattern as the 25.1 proxy_get_header_map_pairs +
// proxy_get_property hostcalls.
func (rv *RootVM) AllocateGuestBuffer(ctx context.Context, mod api.Module, data []byte, ptrPtr, sizePtr uint32) abi.WasmResult {
	return writeReturnBuffer(ctx, mod, data, ptrPtr, sizePtr)
}

// --- streamHost (abi/stream_control.go) ----------------------------------

// StreamContinue forwards to ABICallbacks.ContinueStream for the
// stream-resume hostcall (proxy_continue_stream per §5.1 #28).
func (rv *RootVM) StreamContinue(ctx context.Context, streamCtxID uint32, streamType uint32) abi.WasmResult {
	return rv.cb.ContinueStream(ctx, streamCtxID, streamType)
}

// StreamClose forwards to ABICallbacks.CloseStream for the
// stream-teardown hostcall (proxy_close_stream per §5.1 #29).
func (rv *RootVM) StreamClose(ctx context.Context, streamCtxID uint32, streamType uint32) abi.WasmResult {
	return rv.cb.CloseStream(ctx, streamCtxID, streamType)
}

// --- foreignHost (abi/foreign.go) ----------------------------------------
//
// *RootVM structurally satisfies the abi foreignHost interface via the
// CallForeignFunction method defined at root_vm.go (the per-*RootVM
// foreign-function dispatch entry point per AMEND-A9 + R-25.2-8 +
// D-P-PLAN-9). No adapter wrapper is needed — the method signature already
// matches the interface contract verbatim; the abi/foreign.go shim
// type-asserts host → foreignHost at dispatch time.
//
// D-P-PLAN-9 contract (LOAD-BEARING — see foreign.go for the 5-clause
// model): CallForeignFunction is invoked from inside the dispatchMu-held
// hostcall body frame; it MUST NOT re-acquire dispatchMu (sync.Mutex is
// non-reentrant; would deadlock). The foreign function executes
// synchronously inside the held frame; panic-recovery converts Go panics
// to InternalFailure + invokes PanicHandlerFn.

// --- httpCallHost (abi/http_call.go) -------------------------------------
//
// *RootVM structurally satisfies the abi httpCallHost interface via the
// DispatchHttpCall25_2 adapter method below + the CurrentCtxID method above.
// The adapter exists because abi/http_call.go declares its OWN HeaderPair25_2
// struct (the abi package MUST NOT import internal/wasm — would induce a
// cycle); the adapter converts between the two structurally-identical
// header-pair shapes via a per-pair iteration.
//
// D-P-PLAN-9 mutex-per-RootVM concurrency contract: the adapter inherits
// the per-stream dispatchMu-held call frame from the shim caller; the
// underlying DispatchHttpCall acquires a SEPARATE mutex (httpCallsMu) for
// the map insert; lock order is dispatchMu → httpCallsMu. The dispatch
// goroutine's handleHttpCallResponse re-acquires dispatchMu on its own
// goroutine for the proxy_on_http_call_response invocation.

// DispatchHttpCall25_2 is the abi/httpCallHost adapter that translates
// between abi.HeaderPair25_2 (abi-package-local) + wasm.HeaderPair (wasm-
// package-local). The two structs are structurally identical (Key + Value
// string fields); we cannot direct-cast because Go strict-typing treats
// them as distinct named types. Per-pair iteration is the canonical Go
// conversion pattern for this scenario; the slice allocation is a single
// O(N) cost paid per dispatch.
//
// The method name is DispatchHttpCall25_2 (vs the plain DispatchHttpCall
// at http_call.go) to keep the two surfaces distinct + avoid signature
// shadowing — the http_call.go DispatchHttpCall takes []HeaderPair (the
// internal/wasm type), the adapter takes []abi.HeaderPair25_2 (the
// internal/wasm/abi type).
func (rv *RootVM) DispatchHttpCall25_2(ctx context.Context, streamCtxID uint32, cluster string, headers []abi.HeaderPair25_2, body []byte, trailers []abi.HeaderPair25_2, timeoutMs uint32) (uint32, abi.WasmResult) {
	wasmHeaders := abiHeadersToWasm(headers)
	wasmTrailers := abiHeadersToWasm(trailers)
	return rv.DispatchHttpCall(ctx, streamCtxID, cluster, wasmHeaders, body, wasmTrailers, timeoutMs)
}

// abiHeadersToWasm converts a []abi.HeaderPair25_2 slice to a []HeaderPair
// slice. Nil-safe (nil input → nil output). Defensive-allocates the result
// slice to len(in) up front; the per-pair conversion is a literal value
// copy (both structs have identical Key + Value string fields).
func abiHeadersToWasm(in []abi.HeaderPair25_2) []HeaderPair {
	if len(in) == 0 {
		return nil
	}
	out := make([]HeaderPair, len(in))
	for i, p := range in {
		out[i] = HeaderPair{Key: p.Key, Value: p.Value}
	}
	return out
}

// --- metricsHost (abi/metrics.go) ----------------------------------------
//
// *RootVM structurally satisfies the abi metricsHost interface via the
// DefineMetric + IncrementMetric + RecordMetric + GetMetric methods defined
// at dynamic_stats.go (the per-*RootVM dynamic-stats wrapper around
// *dynamic.Registry per AMEND-B2 + R-25.2-2 + R-25.2-7 + ADR-0208). No
// adapter wrapper is needed — the methods already match the interface
// signature exactly (DefineMetric(uint32, string) → (uint32, WasmResult);
// IncrementMetric(uint32, int64) → WasmResult; RecordMetric(uint32, uint64)
// → WasmResult; GetMetric(uint32) → (uint64, WasmResult)). The abi/metrics.go
// shim type-asserts host → metricsHost at dispatch time; non-satisfaction
// returns WasmResultInternalFailure.
//
// MetricType byte-pin per AMEND-B2: the metricType uint32 wire arg maps
// directly to dynamic.MetricType (Counter=0, Gauge=1, Histogram=2). Signed-
// int64 delta + unsigned-uint64 value per AMEND-B2.

// --- sharedDataHost (abi/shared_data.go) ---------------------------------
//
// *RootVM structurally satisfies the abi sharedDataHost interface via the
// SetSharedData + GetSharedData methods defined at shared_data.go (the
// per-*RootVM CAS-protected K-V map with envoy-go-strict caps per Q6 +
// R-25.2-10). No adapter wrapper is needed — the methods already match the
// interface signature; the abi/shared_data.go shim type-asserts host →
// sharedDataHost at dispatch time. The compile-time guard at
// registration.go (`var _ wasiHost = (*RootVM)(nil)`-style) does NOT cover
// abi-package interfaces (they are package-private); the structural-match
// is verified at first dispatch.
