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

// ----------------------------------------------------------------------------
// 25.2 capability extensions per AMEND-B5 + 25.2 SPEC §11.5 D-25.2-5
// ----------------------------------------------------------------------------
//
// Phase 25.2 EXTENDS the capability roster from 37 keys (25.1 cumulative) to
// 58 keys: 14 NEW env-namespace hostcall keys + 7 NEW lifecycle (proxy_on_*)
// keys. The 25.1 default-deny posture per AMEND-A5 + ADR-0204 INHERITS
// unchanged — empty `AllowedCapabilities` map → DENY ALL (including all 21
// NEW keys).
//
// Two distinct gate-sites per upstream `proxy-wasm-cpp-host@da3ce05d` (see
// 25.2 SPEC §11.5 D-25.2-5 evidence):
//
//   - **Env-namespace hostcalls** (14 NEW keys below) — gated at
//     `registerCallback` TIME via `src/wasm.cc:176-189` `_REGISTER_PROXY`
//     macro. The macro expands to `if (capabilityAllowed("proxy_" #_fn))
//     wasm_vm_->registerCallback(...)`. envoy-go mirrors at Task 3
//     `registration.go`: for a denied capability, the host function is NOT
//     registered on the wazero Runtime; the guest's import resolution fails
//     at module-instantiation OR the runtime trap fires on call (depending
//     on whether the guest declared the import). This AMPLIFIES the default-
//     deny posture — a denied hostcall is INVISIBLE to the guest from
//     instantiation, not merely rejected at call time.
//
//   - **Lifecycle (proxy_on_*) keys** (7 NEW keys below) — gated at
//     `getFunction` TIME via `src/wasm.cc:238-247` `_GET_PROXY` macro. The
//     macro expands to `if (capabilityAllowed("proxy_" #_fn))
//     wasm_vm_->getFunction(#_fn, &_fn##_);` — denied capability ⇒ function
//     pointer left null; dispatch skips. envoy-go mirrors at Task 3:
//     `*StreamContext`/`*RootVM` callback dispatchers consult `IsAllowed`
//     before lookup + Call.
//
// Both gate-sites consult THIS file's `IsAllowed` semantic — the
// capability-key strings below are the SOLE source of truth for both
// `registration.go` (Task 3) + every consumer that constructs a
// `SandboxConfig` allowlist.

// 25.2 body / buffer capability keys (3 keys per AMEND-B1 + §11.5 D-25.2-5
// hostcall-keys table entries 1..3). Body/buffer hostcalls implement the
// clamp-on-overflow wire-contract per AMEND-B1: when the requested
// `start+length` exceeds the buffer extent, the hostcall returns the
// truncated slice (NOT WasmResult::BadArgument). The 16-MiB envoy-go-strict
// body-buffer cap per ADR-0208 is enforced at the consumer (`internal/
// filter/http/wasm/body.go` Task 16), not at the hostcall shim.
const (
	capProxyGetBufferBytes  = "proxy_get_buffer_bytes"
	capProxySetBufferBytes  = "proxy_set_buffer_bytes"
	capProxyGetBufferStatus = "proxy_get_buffer_status"
)

// 25.2 stream-control capability keys (2 keys per §11.5 D-25.2-5 hostcall-
// keys table entries 4..5). ABI-specific per upstream `include/proxy-wasm/
// exports.h:154-156`. `proxy_continue_stream` un-pauses a previously-paused
// phase (driven by guest returning `Pause`/`StopIteration*` from a callback);
// `proxy_close_stream` closes the stream side (request or response) per
// the WasmStreamType arg.
const (
	capProxyContinueStream = "proxy_continue_stream"
	capProxyCloseStream    = "proxy_close_stream"
)

// 25.2 timer capability key (1 key per §11.5 D-25.2-5 hostcall-keys table
// entry 6). Paired with the `proxy_on_tick` lifecycle key below. Per
// envoy-go-strict Q5 + R-25.2-9 a 10ms tick-period floor is enforced at
// the shim (Task 5 `internal/wasm/tick.go`); the upstream cpp-host has no
// floor (any positive ms accepted).
const capProxySetTickPeriodMilliseconds = "proxy_set_tick_period_milliseconds"

// 25.2 metrics capability keys (4 keys per AMEND-B2 + §11.5 D-25.2-5
// hostcall-keys table entries 7..10). Per AMEND-B2: `proxy_increment_metric`
// takes a SIGNED i64 delta (upstream cpp-host signature; per-metric
// type-based clamping at the consumer per Task 12). Metric names land
// under the `wasmcustom.<custom_name>` namespace per AMEND-B2 via NEW
// `internal/stats/dynamic/` infrastructure (Task 11). The 1024-entry cap
// is enforced at the `*dynamic.Registry` per ADR-0208.
const (
	capProxyDefineMetric    = "proxy_define_metric"
	capProxyIncrementMetric = "proxy_increment_metric"
	capProxyRecordMetric    = "proxy_record_metric"
	capProxyGetMetric       = "proxy_get_metric"
)

// 25.2 shared-data capability keys (2 keys per Q6 + §11.5 D-25.2-5 hostcall-
// keys table entries 11..12). Per-`*RootVM` CAS-protected K-V map per Q6 +
// R-25.2-10 (Task 6 `internal/wasm/shared_data.go`). envoy-go-strict caps:
// 1 MiB per value + 1024 entries per `*RootVM`. CAS semantics byte-faithful
// to upstream cpp-host (`proxy_set_shared_data` with `cas=0` writes
// unconditionally; non-zero `cas` requires match-or-CasMismatch).
const (
	capProxySetSharedData = "proxy_set_shared_data"
	capProxyGetSharedData = "proxy_get_shared_data"
)

// 25.2 outbound-HTTP capability key (1 key per AMEND-B3 + §11.5 D-25.2-5
// hostcall-keys table entry 13). Paired with the `proxy_on_http_call_
// response` lifecycle key below. Per AMEND-B3: at-destruction cancel
// discipline — pending httpCalls are CANCELED (NOT dropped silently)
// when the originating `*StreamContext` closes; the late-arrival case
// increments the envoy-go-strict counter `http_call_response_after_close`
// (counter 14 per ADR-0208).
const capProxyHttpCall = "proxy_http_call"

// 25.2 foreign-function capability key (1 key per AMEND-A9 + §11.5 D-25.2-5
// hostcall-keys table entry 14). Per AMEND-A9: EMPTY default registry —
// `proxy_call_foreign_function` returns `WasmResult::NotFound (=1)` for any
// unregistered name (byte-faithful to upstream cpp-host `src/exports.cc:
// 147-184`). Per D-P-PLAN-9: mutex-per-RootVM dispatch concurrency (sync.
// RWMutex on registry; synchronous dispatch inside per-stream call frame;
// panic-recovery wrapper). envoy-go-strict departure record #4 (0-vs-10
// default registrations vs upstream's 10 demo entries).
const capProxyCallForeignFunction = "proxy_call_foreign_function"

// 25.2 lifecycle (proxy_on_*) capability keys (7 keys per §11.5 D-25.2-5
// lifecycle-keys table entries 15..21). Gated at `getFunction` time per
// upstream `src/wasm.cc:238-247` `_GET_PROXY` macro (denied capability ⇒
// function pointer left null; dispatch path skips).
//
//	`proxy_on_request_body`        — HTTP request-body phase callback
//	                                  (per AMEND-B1; envoy-go-strict
//	                                  body-buffer cap enforced at consumer).
//	`proxy_on_response_body`       — HTTP response-body phase callback.
//	`proxy_on_request_trailers`    — HTTP request-trailers phase callback
//	                                  (reuses 25.1 header-map family with
//	                                  WasmHeaderMapType=1 activated).
//	`proxy_on_response_trailers`   — HTTP response-trailers phase callback
//	                                  (WasmHeaderMapType=3 activated).
//	`proxy_on_tick`                — root-context tick callback (per Q5
//	                                  10ms floor + Clock seam first
//	                                  co-consumer per phase-21 ADR-0186).
//	`proxy_on_http_call_response`  — outbound-HTTP response callback
//	                                  (per AMEND-B3 cancel-at-destruction).
//	`proxy_on_foreign_function`    — foreign-function callback (gated
//	                                  separately via `_GET_PROXY` per
//	                                  ABI 0.2.0/0.2.1 branch in upstream
//	                                  `wasm.cc`).
const (
	capProxyOnRequestBody      = "proxy_on_request_body"
	capProxyOnResponseBody     = "proxy_on_response_body"
	capProxyOnRequestTrailers  = "proxy_on_request_trailers"
	capProxyOnResponseTrailers = "proxy_on_response_trailers"
	capProxyOnTick             = "proxy_on_tick"
	capProxyOnHttpCallResponse = "proxy_on_http_call_response"
	capProxyOnForeignFunction  = "proxy_on_foreign_function"
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
