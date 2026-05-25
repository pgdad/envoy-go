// Default-deny capability sandbox per AMEND-A5 + ADR-0204.
//
// envoy-go-strict DEPARTURE from upstream: the SandboxConfig zero value is
// `StrictDefaultDeny` — an empty (or nil) `AllowedCapabilities` map ⇒ DENY
// ALL hostcalls. This INVERTS upstream proxy-wasm-cpp-host's bare-empty-map-
// allow-all semantic at `src/wasm.cc@da3ce05d:181-206` where the
// `_GET_PROXY` / `_GET_PROXY_ABI` macros call `capabilityAllowed("proxy_"
// #_fn)` and that helper defaults to true when no allow-set is configured.
// envoy-go reverses the polarity: NO opt-in ⇒ NO capability.
//
// Defined in this file:
//   - `SandboxConfig` — per-capability ALLOW/DENY posture.
//   - `SanitizationConfig` — empty struct (accept-empty-as-no-op per
//     AMEND-A1 §11.4 + parent §4.3.5; upstream's SanitizationConfig proto
//     is empty and marked "currently unimplemented and ignored").
//   - `IsAllowed(*SandboxConfig, capabilityName string) bool` — the gate
//     check consulted by every hostcall shim at Task 7+ (wasi.go already
//     consumes it; future registration.go will route the other 24 hostcalls
//     through this gate).
//   - 29 package-private capability-key constants for the 25.1 surface
//     (the remaining 8 WASI keys live in wasi.go from Task 4; total 37).
//
// # D-P2 closure (module-init / allocator callbacks)
//
// Per upstream `src/wasm.cc@da3ce05d:159-178` the `WasmBase::getFunctions()`
// method retrieves the 5 module-init / allocator callbacks via the bare
// `_GET` macro:
//
//	void WasmBase::getFunctions() {
//	#define _GET(_fn) wasm_vm_->getFunction(#_fn, &_fn##_);
//	#define _GET_ALIAS(_fn, _alias) wasm_vm_->getFunction(#_alias, &_fn##_);
//	  _GET(_initialize);
//	  if (_initialize_) {
//	    _GET(main);
//	    _GET(__main_void);
//	  } else {
//	    _GET(_start);
//	  }
//
//	  _GET(malloc);
//	  if (!malloc_) {
//	    _GET_ALIAS(malloc, proxy_on_memory_allocate);
//	  }
//	  ...
//	#undef _GET_ALIAS
//	#undef _GET
//
//	  // Try to point the capability to one of the module exports, if the capability has been allowed.
//	#define _GET_PROXY(_fn)                                                                            \
//	  if (capabilityAllowed("proxy_" #_fn)) {                                                          \
//	    wasm_vm_->getFunction("proxy_" #_fn, &_fn##_);                                                 \
//	  } else {                                                                                         \
//	    _fn##_ = nullptr;                                                                              \
//	  }
//
// The `_GET` macro does NOT consult `capabilityAllowed()`. The `_GET_PROXY`
// macro DOES. The 5 module-init / allocator callbacks (`_initialize`,
// `_start`, `main`, `malloc`, `proxy_on_memory_allocate`) are thus
// UNGATED — they MUST be retrievable for instantiation to succeed, and
// gating them would break every module.
//
// Disposition: The 5 module-init capability constants (`capModuleInitialize`,
// `capModuleStart`, `capModuleMain`, `capAllocatorMalloc`,
// `capAllocatorProxyOnMemoryAllocate`) exist in this file for ROSTER
// COMPLETENESS — so a grep for the bare names lands at a single source of
// truth. The actual module-init dispatch at Task 7's `vm.Run` will invoke
// these directly via the wazero `ExportedFunction` lookup WITHOUT calling
// `sb.IsAllowed` first. The `IsAllowed` result for these keys is
// "informational" — defined but not consulted by dispatch.
//
// Cross-references:
//   - Task 7 vm.go — owns the module-init dispatch that bypasses the gate
//     for the 5 ungated keys.
//   - ADR-0204 — Decision body lands at Task 17 and will incorporate this
//     D-P2 disposition.

package wasm

// Headers-bridge capability keys per SPEC §3.3 (7 keys). The header-map
// hostcalls covered by the 25.1 first-co-consumer headers-bridge surface
// (request + response phases; trailers + body land at 25.2 + 25.3).
//
// Bare names — the `proxy_` prefix is the byte-faithful identifier used by
// upstream proxy-wasm-cpp-host at the wazero host-module registration
// boundary (the `proxy_` family lives under a `env`-named module by
// upstream convention; per AMEND-A2 the host-module name is fixed at
// Task 7 registration.go).
const (
	capProxyGetHeaderMapPairs     = "proxy_get_header_map_pairs"
	capProxySetHeaderMapPairs     = "proxy_set_header_map_pairs"
	capProxyGetHeaderMapValue     = "proxy_get_header_map_value"
	capProxyAddHeaderMapValue     = "proxy_add_header_map_value"
	capProxyReplaceHeaderMapValue = "proxy_replace_header_map_value"
	capProxyRemoveHeaderMapValue  = "proxy_remove_header_map_value"
	capProxyGetHeaderMapSize      = "proxy_get_header_map_size"
)

// Local-response capability key (1 key). Powers the 18-arm
// proxy_send_local_response hostcall consumed by the 25.1 dispatch shape
// at Task 12 encode_headers.go.
const capProxySendLocalResponse = "proxy_send_local_response"

// Property capability keys (2 keys). 25.1 surface only — full envoy-property
// schema lands progressively across 25.2 + 25.3 as additional hostcall
// shims register against the same capability keys.
const (
	capProxyGetProperty = "proxy_get_property"
	capProxySetProperty = "proxy_set_property"
)

// Log capability keys (2 keys). `proxy_log` is the load-bearing structured
// log sink; `proxy_get_log_level` returns the host's current verbosity.
const (
	capProxyLog         = "proxy_log"
	capProxyGetLogLevel = "proxy_get_log_level"
)

// Status capability key (1 key). Per proxy-wasm v0.2.1 wire shape, returns
// the upstream HTTP response status code on the response-phase hostcall.
const capProxyGetStatus = "proxy_get_status"

// Time capability key (1 key). Returns the wall clock in nanoseconds-since-
// epoch; the bare name is upstream-faithful (no `_nanoseconds_since_epoch`
// suffix).
const capProxyGetCurrentTimeNanoseconds = "proxy_get_current_time_nanoseconds"

// Context-lifecycle capability keys (2 keys). `proxy_set_effective_context`
// scopes subsequent hostcalls to the named context-id; `proxy_done` signals
// the guest is finished with the current context (paired with the
// `proxy_on_done` callback at the OnLog/OnDelete flow).
const (
	capProxySetEffectiveContext = "proxy_set_effective_context"
	capProxyDone                = "proxy_done"
)

// Module-init / allocator capability keys (5 keys). Per the D-P2 closure
// at the file header doc-comment: these constants exist for roster
// completeness; the actual module-init dispatch at Task 7 vm.Run BYPASSES
// the capability gate (matching upstream `wasm.cc:159-178` `_GET` macro
// behavior). `IsAllowed` for these keys is informational; the dispatch
// path does NOT call it.
//
//	`_initialize`              — proxy-wasm v0.2.1 standard init export.
//	`_start`                   — WASI standard init export (alternative
//	                             to `_initialize` for WASI-only modules).
//	`main`                     — alternative init export found on some
//	                             WASI-libc compilations.
//	`malloc`                   — guest-side memory allocator export
//	                             (REQUIRED for host→guest data passing).
//	`proxy_on_memory_allocate` — alternative allocator export name
//	                             accepted by upstream as a malloc fallback
//	                             via `_GET_ALIAS(malloc, proxy_on_memory_allocate)`.
const (
	capModuleInitialize               = "_initialize"
	capModuleStart                    = "_start"
	capModuleMain                     = "main"
	capAllocatorMalloc                = "malloc"
	capAllocatorProxyOnMemoryAllocate = "proxy_on_memory_allocate"
)

// Lifecycle + HTTP callback module-getters (8 keys). Gated at getFunction
// time per upstream `wasm.cc:181-206` `_GET_PROXY` + `_GET_PROXY_ABI` macros
// — when `capabilityAllowed("proxy_on_*")` is false, the upstream sets the
// function pointer to nullptr (the callback is skipped at dispatch). At
// 25.1 envoy-go matches this discipline: an absent capability ⇒ the
// callback is NOT registered against the guest module + the dispatch path
// at Task 7 skips it gracefully.
//
//	`proxy_on_context_create`   — context lifecycle: NEW context-id
//	                              allocated for a stream.
//	`proxy_on_vm_start`         — VM-scope init callback.
//	`proxy_on_configure`        — plugin-scope init callback.
//	`proxy_on_done`             — context lifecycle: stream is finishing.
//	`proxy_on_delete`           — context lifecycle: context-id is being
//	                              released.
//	`proxy_on_log`              — access-log phase callback.
//	`proxy_on_request_headers`  — HTTP request-headers phase callback
//	                              (ABI v0.2.1 invokes the `_abi_02` variant
//	                              per upstream `wasm.cc:201`).
//	`proxy_on_response_headers` — HTTP response-headers phase callback
//	                              (ABI v0.2.1 invokes the `_abi_02` variant).
const (
	capProxyOnContextCreate   = "proxy_on_context_create"
	capProxyOnVmStart         = "proxy_on_vm_start"
	capProxyOnConfigure       = "proxy_on_configure"
	capProxyOnDone            = "proxy_on_done"
	capProxyOnDelete          = "proxy_on_delete"
	capProxyOnLog             = "proxy_on_log"
	capProxyOnRequestHeaders  = "proxy_on_request_headers"
	capProxyOnResponseHeaders = "proxy_on_response_headers"
)

// SanitizationConfig is the per-capability host-side sanitization policy.
//
// Per AMEND-A1 §11.4 + parent §4.3.5: upstream's `SanitizationConfig` proto
// is EMPTY and marked "currently unimplemented and ignored, and so should
// be left empty". envoy-go matches byte-faithfully — the type carries NO
// fields; the empty value `SanitizationConfig{}` is the SOLE valid value
// at phase 25.1.
//
// Reserved for future per-capability sanitization rules if upstream lands
// them; the type is kept as a struct (not a sentinel `bool`) so that field
// additions stay backwards-compatible (consumers using positional struct
// literals would break under a sentinel-bool retrofit).
type SanitizationConfig struct {
	// No fields. Reserved for future per-capability sanitization rules
	// if upstream lands them.
}

// SandboxConfig governs which proxy-wasm hostcalls + WASI shims + module-
// callbacks are permitted at the capability gate.
//
// Zero value = `StrictDefaultDeny` per AMEND-A5 + ADR-0204: a `SandboxConfig{}`
// with nil `AllowedCapabilities` DENIES every capability. This INVERTS
// upstream proxy-wasm-cpp-host's bare-empty-map-allow-all semantic at
// `src/wasm.cc@da3ce05d:181-206`. Explicit opt-in is required for each
// capability via population of `AllowedCapabilities`.
//
// The per-capability value is `SanitizationConfig{}` (currently empty per
// AMEND-A1; reserved for future per-capability sanitization knobs).
type SandboxConfig struct {
	// AllowedCapabilities is the set of capability keys the guest is
	// permitted to invoke. The KEY is the bare upstream-faithful name
	// (e.g. `proxy_log`, `fd_write`, `proxy_on_request_headers`) — see
	// the per-family `cap*` constants at the head of this file (and the
	// `capWasi*` constants in wasi.go).
	//
	// Empty (or nil) ⇒ DENY ALL hostcalls per AMEND-A5 — INVERTS
	// upstream's empty-map-allow-all semantic. See SPEC §3.3 for the
	// full ~80-key roster (37 keys materialized at phase 25.1).
	AllowedCapabilities map[string]SanitizationConfig
}

// IsAllowed reports whether the named capability is permitted by the
// sandbox. Pointer receiver per the SPEC; callers commonly hold a
// *SandboxConfig threaded through the per-stream VM context.
//
// Semantic: returns true iff `capabilityName` is present as a key in
// `AllowedCapabilities`. The value (a `SanitizationConfig`) is unused at
// 25.1 — its presence in the map IS the allow-signal.
//
// Nil-receiver tolerance: a nil `*SandboxConfig` is treated as the
// zero value (StrictDefaultDeny) and returns false for every key. This
// matches the discipline of nil-pointer-as-zero-value semantics used
// throughout envoy-go (e.g. `(*CompileCache).Close`).
//
// Empty-or-nil-map ⇒ deny-all falls out naturally from Go's map lookup
// semantic: a lookup against a nil or empty map returns (zero-value,
// ok=false) — this implements the StrictDefaultDeny INVERSION of
// upstream's allow-all without any explicit branch.
func (sb *SandboxConfig) IsAllowed(capabilityName string) bool {
	if sb == nil {
		return false
	}
	_, ok := sb.AllowedCapabilities[capabilityName]
	return ok
}
