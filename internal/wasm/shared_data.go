// shared_data.go — per-*RootVM CAS-protected shared-data K-V map per Q6 +
// R-25.2-10 + 25.2 SPEC §3.1 + §5.1 #35-36.
//
// # Design (per Q6 + R-25.2-10 + cpp-host byte-faithful)
//
// Each *RootVM owns ONE shared-data store — a `map[string]sharedDataEntry`
// guarded by sync.RWMutex. The entry carries the value bytes + a monotonic
// CAS counter. The CAS semantic is byte-exact from upstream proxy-wasm-cpp-
// host (src/exports.cc:set_shared_data):
//
//	cas == 0           ⇒ UNCONDITIONAL write (returns Ok, increments CAS)
//	cas > 0 + match    ⇒ CONDITIONAL write succeeds (returns Ok, increments CAS)
//	cas > 0 + mismatch ⇒ FAILS with WasmResult::CasMismatch (=8); entry unchanged
//
// The cas counter starts at 1 for newly-created entries (matches cpp-host)
// and increments on every successful write — so the next read returns the
// freshly-bumped CAS value and a guest that does the canonical
// "Get → mutate → Set(cas=returned)" round-trip succeeds iff no concurrent
// writer raced ahead.
//
// # envoy-go-strict cap discipline (per Q6 + AMEND-B5)
//
// Two operator-configurable caps (defaults at envoy-go-strict envelope):
//
//	per-value: 1 MiB (1048576 bytes) — configurable via
//	           envoy_go_strict_shared_data_value_cap_bytes
//	entry-count: 1024 — configurable via
//	             envoy_go_strict_shared_data_max_entries
//
// Both caps are LOAD-bearing: a value-cap-exceeded write returns
// WasmResult::InternalFailure (=10) WITHOUT touching the map; an
// entry-cap-exceeded NEW write returns WasmResult::InternalFailure WITHOUT
// adding the entry. IN-PLACE updates of EXISTING entries are NOT subject to
// the entry-cap (no new slot) but ARE still subject to the value-cap (the
// new value bytes must fit).
//
// The cap-defaults (1 MiB + 1024) materialize at FIRST USE — if neither
// WithRootSharedDataCaps nor the per-plugin Configure path has set the cap
// fields, SetSharedData applies the defaults transparently. This keeps
// NewRootVM's option-list small (no required cap-init) and matches the
// "envoy-go-strict envelope as the implicit default" pattern.
//
// # Counter integration (deferred to Task 17)
//
// On a cap-exceeded path the SPEC mandates a `wasm.<plugin>.shared_data_
// cap_exceeded` envoy-go-strict counter increment + an `envoy_go.failures`
// co-increment per §2.25. The per-plugin stats wiring lands at Task 17
// (stats.go EXTEND); at Task 6 the cap-exceeded path returns the
// load-bearing WasmResult::InternalFailure but does NOT yet invoke a
// counter (the per-RootVM stats reference is not yet wired). The
// integration touchpoint is marked with a `TODO Task 17` reference in the
// cap-exceeded branches below.
//
// # Goroutine-safety
//
// The shared-data store is accessed by ANY goroutine that runs an in-flight
// hostcall on a `*RootVM`: the per-stream filter-dispatch goroutines (via
// proxy_set_shared_data / proxy_get_shared_data shim invocations from
// inside the dispatchMu-held frame), the tick goroutine (if a future
// guest's proxy_on_tick calls shared-data hostcalls), and the httpCall
// response goroutine (likewise). The dispatchMu serialization that the
// abi-shim envelopes inherit means the shared-data store is touched by AT
// MOST ONE goroutine at a time in practice — but the SetSharedData /
// GetSharedData methods are nevertheless guarded by sync.RWMutex so they
// are SAFE to call from any goroutine independently of dispatchMu (e.g.,
// the Task 8 httpCall pre-dispatch path or the Task 7 foreign-function
// pre-dispatch path may consult the store outside the dispatchMu frame).
//
// Lock discipline: Set acquires the write lock; Get acquires the read lock.
// Map access is the only critical section; the cap-check is performed
// under the same lock as the write so a concurrent racing Set cannot
// overflow the entry-cap by interleaving.

package wasm

import (
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// SharedDataValueCapDefault is the envoy-go-strict envelope default for the
// per-value byte cap per Q6 + AMEND-B5 (1 MiB = 1048576 bytes). Operators
// override via WithRootSharedDataCaps OR per-plugin
// envoy_go_strict_shared_data_value_cap_bytes config field.
const SharedDataValueCapDefault uint32 = 1024 * 1024 // 1 MiB

// SharedDataMaxEntriesDefault is the envoy-go-strict envelope default for
// the entry-count cap per Q6 + AMEND-B5 (1024 entries). Operators override
// via WithRootSharedDataCaps OR per-plugin
// envoy_go_strict_shared_data_max_entries config field.
const SharedDataMaxEntriesDefault uint32 = 1024

// sharedDataEntry is one slot in the per-*RootVM shared-data store. The
// CAS counter is monotonic per entry — starts at 1 for newly-created
// entries (matches cpp-host) and increments on every successful write.
type sharedDataEntry struct {
	value []byte
	cas   uint32
}

// effectiveSharedDataValCap returns the value-cap to apply for this *RootVM:
// the configured value if non-zero, the envoy-go-strict default (1 MiB)
// otherwise. Centralized so the cap-default semantic stays consistent
// across Set + any future callers (e.g., the Task 14 PARSE-REJECT
// validation of envoy_go_strict_shared_data_value_cap_bytes uses this
// constant to surface the default when reporting "zero is rejected").
func (rv *RootVM) effectiveSharedDataValCap() uint32 {
	if rv.sharedDataValCap == 0 {
		return SharedDataValueCapDefault
	}
	return rv.sharedDataValCap
}

// effectiveSharedDataMaxEntries returns the entry-count cap to apply for
// this *RootVM: the configured value if non-zero, the envoy-go-strict
// default (1024) otherwise. See effectiveSharedDataValCap for the rationale.
func (rv *RootVM) effectiveSharedDataMaxEntries() uint32 {
	if rv.sharedDataMaxEntries == 0 {
		return SharedDataMaxEntriesDefault
	}
	return rv.sharedDataMaxEntries
}

// SetSharedData writes (or CAS-updates) the entry at `key` to `value`.
// Returns:
//
//   - WasmResultOk on a successful write (NEW entry OR CAS-matched update
//     OR cas==0 unconditional update). The entry's CAS counter is
//     incremented (new entries start at 1; existing entries' CAS bumps by 1).
//   - WasmResultCasMismatch on `cas > 0 && existing.cas != cas`. The entry
//     is UNCHANGED.
//   - WasmResultInternalFailure on a cap-exceeded write (value-cap OR
//     entry-cap). The store is UNCHANGED. Per §2.25 this ALSO increments
//     `envoy_go.failures` + the `shared_data_cap_exceeded` envoy-go-strict
//     counter — wiring deferred to Task 17 (per-plugin stats.go EXTEND).
//
// `value == nil` (or len == 0) is a valid empty-value write (matches
// cpp-host). The empty value still occupies one entry slot and counts
// against the entry-cap.
//
// Concurrency: acquires sharedDataMu Lock. The cap-check + the map
// mutation are performed under the same lock so concurrent racing Sets
// cannot overflow the entry-cap by interleaving.
func (rv *RootVM) SetSharedData(key string, value []byte, cas uint32) abi.WasmResult {
	rv.sharedDataMu.Lock()
	defer rv.sharedDataMu.Unlock()

	valCap := rv.effectiveSharedDataValCap()
	maxEntries := rv.effectiveSharedDataMaxEntries()

	// Value-cap check (applies to BOTH new + in-place updates).
	//nolint:gosec // len(value) is bounded by the guest's actual payload; the cap-check IS the bound.
	if uint32(len(value)) > valCap {
		// envoy-go-strict counter increment per Q6 (counter 12 of 14 in
		// §7.1) + envoy_go.failures co-increment per §2.25. Wired via
		// RootStatsRecorder per 25.2 IMPL Task 20 follow-up (Concern 2).
		rv.stats.SharedDataCapExceededInc()
		rv.stats.EnvoyGoFailuresInc()
		return abi.WasmResultInternalFailure
	}

	// Lazy-init the map on first write. Avoids a NewRootVM-time allocation
	// for plugins that never touch shared-data.
	if rv.sharedData == nil {
		rv.sharedData = make(map[string]sharedDataEntry)
	}

	if existing, ok := rv.sharedData[key]; ok {
		// CAS check (cpp-host byte-faithful: cas=0 unconditionally writes;
		// cas>0 must match the existing entry's CAS).
		if cas != 0 && existing.cas != cas {
			return abi.WasmResultCasMismatch
		}
		// Defensive copy of `value` so a subsequent guest-side mutation of
		// the source bytes does NOT silently mutate the stored entry (the
		// shim's caller already copies the wasm-memory slice, but call
		// sites from Go-side code (Task 17 stats, future direct callers)
		// MAY pass a non-copied slice).
		newVal := append([]byte(nil), value...)
		existing.value = newVal
		existing.cas++
		rv.sharedData[key] = existing
		return abi.WasmResultOk
	}

	// New entry. Entry-cap check.
	//nolint:gosec // len(sharedData) bounded by past-Set successes; well within uint32.
	if uint32(len(rv.sharedData)) >= maxEntries {
		// envoy-go-strict counter increment per Q6 (counter 12 of 14 in
		// §7.1) + envoy_go.failures co-increment per §2.25. Wired via
		// RootStatsRecorder per 25.2 IMPL Task 20 follow-up (Concern 2).
		rv.stats.SharedDataCapExceededInc()
		rv.stats.EnvoyGoFailuresInc()
		return abi.WasmResultInternalFailure
	}
	newVal := append([]byte(nil), value...)
	rv.sharedData[key] = sharedDataEntry{value: newVal, cas: 1}
	return abi.WasmResultOk
}

// GetSharedData returns the value + current CAS counter for the entry at
// `key`. Returns:
//
//   - (value, cas, WasmResultOk) when the entry exists. `value` is a fresh
//     copy of the stored bytes — callers MAY mutate without affecting the
//     store.
//   - (nil, 0, WasmResultNotFound) when the entry does not exist.
//
// Concurrency: acquires sharedDataMu RLock. Multiple readers can proceed
// concurrently; a concurrent Set blocks until they all complete.
func (rv *RootVM) GetSharedData(key string) ([]byte, uint32, abi.WasmResult) {
	rv.sharedDataMu.RLock()
	defer rv.sharedDataMu.RUnlock()

	if rv.sharedData == nil {
		return nil, 0, abi.WasmResultNotFound
	}
	entry, ok := rv.sharedData[key]
	if !ok {
		return nil, 0, abi.WasmResultNotFound
	}
	// Defensive copy: do NOT hand the caller a reference to our internal
	// slice (a Get-then-mutate pattern would otherwise corrupt the store
	// without going through the CAS check).
	out := append([]byte(nil), entry.value...)
	return out, entry.cas, abi.WasmResultOk
}
