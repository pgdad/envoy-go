// Package abi — foreign.go: proxy_call_foreign_function hostcall dispatch
// shim per 25.2 SPEC §5.1 #38 + AMEND-A9 + R-25.2-8 + D-25.2-P3 closure
// (mutex-per-RootVM dispatch concurrency model per D-P-PLAN-9).
//
// # The single host shim landed at Task 7
//
//   - CallForeignFunctionShim — proxy_call_foreign_function(
//     name_data, name_size, args_data, args_size,
//     ret_results_data_ptr, ret_results_size_ptr) -> WasmResult. Reads
//     (name, args) from guest memory + delegates to the per-*RootVM foreign-
//     function dispatcher via the foreignHost interface; on Ok writes the
//     result bytes back to guest memory via allocator-discovered malloc.
//
// # Wire-shape (cpp-host include/proxy-wasm/exports.h)
//
//	Word call_foreign_function(Word function_name_ptr, Word function_name_size,
//	                            Word arguments_ptr, Word arguments_size,
//	                            Word results_ptr_ptr, Word results_size_ptr)
//
// # EMPTY default registry per AMEND-A9
//
// envoy-go ships ZERO default foreign functions (vs upstream proxy-wasm-cpp-
// host's 10: verify_signature + sign + compress + uncompress +
// set_envoy_filter_state + clear_route_cache + expr_create + expr_evaluate +
// expr_delete + declare_property). Operators MUST explicitly register at boot
// via `wasm.RegisterForeignFunction(name, fn)`. Unregistered names return
// WasmResult::NotFound (=1) byte-faithful to cpp-host src/exports.cc:147-184.
// The host shim writes (0, 0) to the return slots on NotFound (no allocator
// invocation; matches the "no data" pattern from writeReturnBuffer in
// internal/wasm/registration.go).
//
// # Concurrency contract per D-P-PLAN-9 (LOAD-BEARING)
//
// The shim is invoked from inside the per-*RootVM dispatchMu-held hostcall
// body frame (the registration.go envelope acquires dispatchMu via the per-
// StreamContext.CallProxyOnX path). The shim delegates to
// `foreignHost.CallForeignFunction` which executes the foreign function
// synchronously inside the held frame. The host MUST NOT re-acquire
// dispatchMu — sync.Mutex is non-reentrant in Go and would deadlock. The
// host-side panic-recovery wrapper applies (Go panic in the foreign
// function → recover() → InternalFailure + PanicHandlerFn fires).
//
// # Decoupling from internal/wasm
//
// abi/ MUST NOT import internal/wasm (would induce a cycle — wasm imports
// abi). The shim accepts a `Host25_2 any` parameter (declared in
// stubs_25_2.go) carrying an opaque value the registration.go envelope
// passes through unchanged (typically `*RootVM`). The shim type-asserts to
// a package-private `foreignHost` interface; if the value does not satisfy
// the interface, the shim returns WasmResultInternalFailure (programmer
// error — wrong host wired in).
package abi

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// foreignHost is the package-private interface the foreign-function shim
// dispatches through. Satisfied by `*wasm.RootVM` (via the
// CallForeignFunction method on the RootVM type — see internal/wasm/
// root_vm.go) so the registration.go envelope can forward its `vm`
// argument unchanged. The shim type-asserts the opaque Host25_2 argument
// to this interface; non-satisfaction returns WasmResultInternalFailure.
//
// D-P-PLAN-9 mutex-per-RootVM concurrency contract: the CallForeignFunction
// implementation executes the ForeignFunctionFn synchronously inside the
// per-stream dispatchMu-held call frame; the shim MUST NOT re-acquire
// dispatchMu (sync.Mutex is non-reentrant; would deadlock); panic-recovery
// converts Go panics to InternalFailure.
type foreignHost interface {
	// CallForeignFunction dispatches the named foreign function with `args`.
	// Returns:
	//   - (result, WasmResultOk) on success;
	//   - (nil, WasmResultNotFound) for unregistered names (byte-faithful to
	//     cpp-host src/exports.cc:147-184);
	//   - (nil, WasmResultInternalFailure) on Go-side panic recovery;
	//   - (result, status) for any other status the foreign function returns.
	CallForeignFunction(ctx context.Context, name string, args []byte) ([]byte, WasmResult)
}

// CallForeignFunctionShim — proxy_call_foreign_function dispatch per §5.1
// #38 + AMEND-A9 + R-25.2-8 + D-P-PLAN-9 mutex-per-RootVM concurrency model.
//
// Wire-shape (cpp-host include/proxy-wasm/exports.h):
//
//	Word call_foreign_function(Word function_name_ptr, Word function_name_size,
//	                            Word arguments_ptr, Word arguments_size,
//	                            Word results_ptr_ptr, Word results_size_ptr)
//
// Reads (name, args) from guest memory; delegates to foreignHost.CallForeignFunction;
// on Ok writes the result bytes back to guest memory via allocator-discovered
// malloc (the standard return-by-reference pattern; mirrors GetSharedDataShim
// + the writeReturnBuffer helper at internal/wasm/registration.go).
//
// Semantic per AMEND-A9 + R-25.2-8 + cpp-host byte-faithful:
//
//   - name read failure → WasmResult::InvalidMemoryAccess (=6). args read
//     failure → InvalidMemoryAccess.
//   - name unregistered (host returns NotFound) → write (0, 0) to the
//     return slots + return WasmResult::NotFound (=1). The `foreign_function_denied`
//     envoy-go-strict counter increments on this path (host-side wiring
//     deferred Task 17).
//   - panic in the foreign-function Go code → host-side panic-wrapper
//     converts to WasmResult::InternalFailure (=10); the shim observes this
//     status as the host's return value and propagates unchanged.
//   - any non-Ok status from the host → propagate unchanged (e.g.,
//     BadArgument if the function rejects malformed args).
//   - Ok with non-empty result → allocate guest-side buffer via `malloc`
//     (or `proxy_on_memory_allocate` fallback) + write result bytes there +
//     write (offset, size) to (ret_results_data_ptr, ret_results_size_ptr).
//     Allocator-missing OR allocator-returns-0 OR memory-write-failure →
//     WasmResult::InvalidMemoryAccess.
//   - Ok with empty result (host returned nil OR len==0) → write (0, 0) to
//     the return slots WITHOUT invoking the allocator (matches the
//     writeReturnBuffer empty-data discipline).
//
// nameSize == 0 is treated as the empty string. cpp-host treats an empty
// name as just-another-NotFound (no special-case); envoy-go matches.
//
// argsSize == 0 is treated as nil args. The foreign function MAY interpret
// nil-args as "no args supplied" + behave accordingly.
//
// Concurrency: the shim is invoked from inside the per-*RootVM dispatchMu-
// held hostcall body frame; foreignHost.CallForeignFunction executes the
// foreign function synchronously inside the held frame per D-P-PLAN-9.
func CallForeignFunctionShim(ctx context.Context, mod api.Module, host Host25_2, nameData, nameSize, argsData, argsSize, retResultsDataPtr, retResultsSizePtr uint32) WasmResult {
	fh, ok := host.(foreignHost)
	if !ok {
		return WasmResultInternalFailure
	}

	mem := mod.Memory()

	// Read name bytes. nameSize == 0 → empty string (valid; cpp-host treats
	// as NotFound; envoy-go matches).
	var nameBytes []byte
	if nameSize > 0 {
		raw, ok := mem.Read(nameData, nameSize)
		if !ok {
			return WasmResultInvalidMemoryAccess
		}
		// Defensive copy: mem.Read returns a view into live guest memory;
		// convert to a stable backing for the host-side string key.
		nameBytes = make([]byte, len(raw))
		copy(nameBytes, raw)
	}

	// Read args bytes. argsSize == 0 → nil args (valid).
	var argsBytes []byte
	if argsSize > 0 {
		raw, ok := mem.Read(argsData, argsSize)
		if !ok {
			return WasmResultInvalidMemoryAccess
		}
		// Defensive copy: the host-side foreign function MAY retain the
		// args slice past its return; isolate from subsequent guest
		// mutation of the source memory.
		argsBytes = make([]byte, len(raw))
		copy(argsBytes, raw)
	}

	result, status := fh.CallForeignFunction(ctx, string(nameBytes), argsBytes)

	// On NotFound: zero out the return slots + propagate NotFound (matches
	// cpp-host + the GetSharedDataShim NotFound pattern).
	if status == WasmResultNotFound {
		if !mem.WriteUint32Le(retResultsDataPtr, 0) {
			return WasmResultInvalidMemoryAccess
		}
		if !mem.WriteUint32Le(retResultsSizePtr, 0) {
			return WasmResultInvalidMemoryAccess
		}
		return WasmResultNotFound
	}

	// On any other non-Ok status (InternalFailure from panic-recovery;
	// BadArgument from the foreign function; CasMismatch if the function
	// wraps a CAS primitive; etc.) — propagate unchanged WITHOUT touching
	// the return slots. The guest's contract per proxy-wasm v0.2.1 is to
	// inspect the return status FIRST and only consume the result slots on
	// Ok.
	if status != WasmResultOk {
		return status
	}

	// Ok path: write the result bytes back to guest memory.
	if len(result) == 0 {
		// Empty result: write (0, 0) without invoking the allocator —
		// matches the writeReturnBuffer empty-data discipline.
		if !mem.WriteUint32Le(retResultsDataPtr, 0) {
			return WasmResultInvalidMemoryAccess
		}
		if !mem.WriteUint32Le(retResultsSizePtr, 0) {
			return WasmResultInvalidMemoryAccess
		}
		return WasmResultOk
	}

	allocator := mod.ExportedFunction("malloc")
	if allocator == nil {
		allocator = mod.ExportedFunction("proxy_on_memory_allocate")
	}
	if allocator == nil {
		return WasmResultInvalidMemoryAccess
	}
	//nolint:gosec // len(result) bound is the foreign function's responsibility; oversize ⇒ alloc-fail.
	results, err := allocator.Call(ctx, uint64(uint32(len(result))))
	if err != nil || len(results) == 0 {
		return WasmResultInvalidMemoryAccess
	}
	//nolint:gosec // wazero stack values are u64; the guest's malloc returns a u32 offset.
	offset := uint32(results[0])
	if offset == 0 {
		return WasmResultInvalidMemoryAccess
	}
	if !mem.Write(offset, result) {
		return WasmResultInvalidMemoryAccess
	}
	if !mem.WriteUint32Le(retResultsDataPtr, offset) {
		return WasmResultInvalidMemoryAccess
	}
	//nolint:gosec // len(result) bounded by the allocator call we just made successfully.
	if !mem.WriteUint32Le(retResultsSizePtr, uint32(len(result))) {
		return WasmResultInvalidMemoryAccess
	}
	return WasmResultOk
}
