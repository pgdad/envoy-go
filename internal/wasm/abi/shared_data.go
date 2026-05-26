// Package abi — shared_data.go: shared-data hostcall dispatch shims per
// 25.2 SPEC §5.1 #35-36 + Q6 + R-25.2-10.
//
// # Two host shims landed at Task 6
//
//   - SetSharedDataShim — proxy_set_shared_data(key_ptr, key_size, value_ptr,
//     value_size, cas) -> WasmResult. Reads (key, value) from guest memory +
//     delegates to the per-*RootVM CAS-protected K-V map via the
//     sharedDataHost interface. The host-side cap-check + CAS semantic
//     materialize in shared_data.go (internal/wasm/); this shim is the
//     thin wire-decode + dispatch wrapper.
//
//   - GetSharedDataShim — proxy_get_shared_data(key_ptr, key_size,
//     ret_value_ptr_ptr, ret_value_size_ptr, ret_cas_ptr) -> WasmResult.
//     Reads `key` from guest memory + delegates to the per-*RootVM store +
//     writes (value, cas) back to guest memory via the standard return-by-
//     reference pattern (allocator-discovered guest-side malloc).
//
// # CAS semantic byte-faithful (cpp-host src/exports.cc:set_shared_data)
//
//   - cas == 0         ⇒ UNCONDITIONAL write (returns Ok)
//   - cas > 0 + match  ⇒ CONDITIONAL write succeeds (returns Ok)
//   - cas > 0 + miss   ⇒ FAILS with WasmResult::CasMismatch (=8)
//
// New entries start at CAS=1 (matches cpp-host); every successful write
// bumps the CAS counter by 1.
//
// # envoy-go-strict cap discipline (per Q6)
//
//   - per-value 1 MiB cap (configurable via
//     envoy_go_strict_shared_data_value_cap_bytes)
//   - 1024-entry cap (configurable via
//     envoy_go_strict_shared_data_max_entries)
//
// Both caps enforced host-side in shared_data.go SetSharedData; cap-exceeded
// returns WasmResult::InternalFailure (=10). The counter-increment
// integration is wired by Task 17 (stats.go EXTEND).
//
// # Decoupling from internal/wasm
//
// abi/ MUST NOT import internal/wasm (would induce a cycle — wasm imports
// abi). Each shim accepts a `Host25_2 any` parameter (declared in
// stubs_25_2.go) carrying an opaque value the registration.go envelope
// passes through unchanged (typically `*RootVM`). The shim type-asserts to
// a package-private `sharedDataHost` interface; if the value does not
// satisfy the interface, the shim returns WasmResultInternalFailure
// (programmer error — wrong host wired in).
package abi

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// sharedDataHost is the package-private interface the shared-data shims
// dispatch through. Satisfied by `*wasm.RootVM` (via the SetSharedData +
// GetSharedData methods on the RootVM type — see internal/wasm/
// shared_data.go) so the registration.go envelope can forward its `vm`
// argument unchanged. The shim type-asserts the opaque Host25_2 argument
// to this interface; non-satisfaction returns WasmResultInternalFailure.
type sharedDataHost interface {
	// SetSharedData writes (or CAS-updates) the entry at `key` to `value`.
	// Returns Ok on success, CasMismatch on conflict, InternalFailure on
	// cap-exceeded. See internal/wasm/shared_data.go for the full semantic.
	SetSharedData(key string, value []byte, cas uint32) WasmResult

	// GetSharedData returns the (value, cas, status) tuple for the entry
	// at `key`. status is Ok when the entry exists; NotFound otherwise.
	GetSharedData(key string) (value []byte, cas uint32, status WasmResult)
}

// SetSharedDataShim — proxy_set_shared_data dispatch per §5.1 #35.
//
// Wire-shape (cpp-host include/proxy-wasm/exports.h):
//
//	Word set_shared_data(Word key_ptr, Word key_size,
//	                      Word value_ptr, Word value_size, Word cas)
//
// Reads (key, value) from guest memory; delegates to the per-*RootVM
// sharedDataHost.SetSharedData; propagates the result unchanged. The host
// applies the CAS semantic + the envoy-go-strict caps.
//
// Memory-read failure on key OR value returns WasmResultInvalidMemoryAccess.
// An empty key (keySize == 0) is read as an empty string — cpp-host accepts
// empty keys; envoy-go does too (the host-side store treats "" as a valid
// distinct key).
//
// An empty value (valueSize == 0) is a valid empty-value write — matches
// cpp-host. The empty value still occupies one entry slot and counts
// against the entry-cap.
func SetSharedDataShim(_ context.Context, mod api.Module, host Host25_2, keyPtr, keySize, valuePtr, valueSize, cas uint32) WasmResult {
	sh, ok := host.(sharedDataHost)
	if !ok {
		return WasmResultInternalFailure
	}

	mem := mod.Memory()

	// Read key bytes from guest memory. keySize == 0 is valid (empty key).
	var keyBytes []byte
	if keySize > 0 {
		raw, ok := mem.Read(keyPtr, keySize)
		if !ok {
			return WasmResultInvalidMemoryAccess
		}
		// Defensive copy: the wazero Read returns a view into live guest
		// memory; we need to convert to a stable string. Building a
		// fresh-byte string copy here ensures the host-side map key is
		// independent of subsequent guest mutation.
		keyBytes = make([]byte, len(raw))
		copy(keyBytes, raw)
	}

	// Read value bytes. valueSize == 0 is valid (empty value).
	var valueBytes []byte
	if valueSize > 0 {
		raw, ok := mem.Read(valuePtr, valueSize)
		if !ok {
			return WasmResultInvalidMemoryAccess
		}
		// Defensive copy so the host-side store does not retain a view
		// into mutable guest memory.
		valueBytes = make([]byte, len(raw))
		copy(valueBytes, raw)
	}

	return sh.SetSharedData(string(keyBytes), valueBytes, cas)
}

// GetSharedDataShim — proxy_get_shared_data dispatch per §5.1 #36.
//
// Wire-shape (cpp-host include/proxy-wasm/exports.h):
//
//	Word get_shared_data(Word key_ptr, Word key_size,
//	                      Word value_ptr_ptr, Word value_size_ptr,
//	                      Word cas_ptr)
//
// Reads `key` from guest memory; delegates to sharedDataHost.GetSharedData;
// on Ok writes the value + cas back to guest memory via the standard
// return-by-reference pattern (allocator-discovered guest-side malloc).
//
// NotFound result: writes (value=nil, cas=0) — value-ptr-ptr + size set to
// 0 (no allocator invocation); cas-ptr set to 0. Returns the NotFound
// status to the guest so it can distinguish from an empty-value Ok.
//
// Memory-read failure on key returns WasmResultInvalidMemoryAccess.
// Memory-write failure on (value, cas) writes back returns
// WasmResultInvalidMemoryAccess. Allocator-missing on a non-empty value
// returns WasmResultInvalidMemoryAccess.
func GetSharedDataShim(ctx context.Context, mod api.Module, host Host25_2, keyPtr, keySize, retValuePtrPtr, retValueSizePtr, retCASPtr uint32) WasmResult {
	sh, ok := host.(sharedDataHost)
	if !ok {
		return WasmResultInternalFailure
	}

	mem := mod.Memory()

	// Read key bytes from guest memory.
	var keyBytes []byte
	if keySize > 0 {
		raw, ok := mem.Read(keyPtr, keySize)
		if !ok {
			return WasmResultInvalidMemoryAccess
		}
		keyBytes = make([]byte, len(raw))
		copy(keyBytes, raw)
	}

	value, cas, status := sh.GetSharedData(string(keyBytes))

	// On NotFound, write (0, 0, 0) per cpp-host convention: zero out the
	// value-ptr + value-size + cas slots so the guest sees a clean "no
	// data" state before checking the status. Then return the NotFound
	// status code.
	if status == WasmResultNotFound {
		if !mem.WriteUint32Le(retValuePtrPtr, 0) {
			return WasmResultInvalidMemoryAccess
		}
		if !mem.WriteUint32Le(retValueSizePtr, 0) {
			return WasmResultInvalidMemoryAccess
		}
		if !mem.WriteUint32Le(retCASPtr, 0) {
			return WasmResultInvalidMemoryAccess
		}
		return WasmResultNotFound
	}

	// Any other non-Ok status (defensive — current GetSharedData only
	// returns Ok or NotFound) — propagate unchanged.
	if status != WasmResultOk {
		return status
	}

	// Ok path: allocate guest-side buffer + write value + write
	// (offset, size) + write cas. We inline the return-by-reference pattern
	// here (rather than depending on bufferHost.AllocateGuestBuffer) so the
	// shim has no cross-shim coupling.
	if len(value) == 0 {
		// Empty value: write (0, 0) without invoking the allocator —
		// matches the writeReturnBuffer empty-data discipline.
		if !mem.WriteUint32Le(retValuePtrPtr, 0) {
			return WasmResultInvalidMemoryAccess
		}
		if !mem.WriteUint32Le(retValueSizePtr, 0) {
			return WasmResultInvalidMemoryAccess
		}
	} else {
		allocator := mod.ExportedFunction("malloc")
		if allocator == nil {
			allocator = mod.ExportedFunction("proxy_on_memory_allocate")
		}
		if allocator == nil {
			return WasmResultInvalidMemoryAccess
		}
		//nolint:gosec // len(value) bounded by the host-side value-cap (default 1 MiB; max uint32).
		results, err := allocator.Call(ctx, uint64(uint32(len(value))))
		if err != nil || len(results) == 0 {
			return WasmResultInvalidMemoryAccess
		}
		//nolint:gosec // wazero stack values are u64; guest malloc returns a u32 offset.
		offset := uint32(results[0])
		if offset == 0 {
			return WasmResultInvalidMemoryAccess
		}
		if !mem.Write(offset, value) {
			return WasmResultInvalidMemoryAccess
		}
		if !mem.WriteUint32Le(retValuePtrPtr, offset) {
			return WasmResultInvalidMemoryAccess
		}
		//nolint:gosec // len(value) bounded by the allocator call we just made successfully.
		if !mem.WriteUint32Le(retValueSizePtr, uint32(len(value))) {
			return WasmResultInvalidMemoryAccess
		}
	}

	if !mem.WriteUint32Le(retCASPtr, cas) {
		return WasmResultInvalidMemoryAccess
	}
	return WasmResultOk
}
