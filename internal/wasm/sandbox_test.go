// Tests for sandbox.go default-deny capability roster per AMEND-A5 + ADR-0204.
//
// Per AMEND-A5 the SandboxConfig zero value is StrictDefaultDeny — an empty
// (or nil) AllowedCapabilities map ⇒ DENY ALL hostcalls. This INVERTS upstream
// proxy-wasm-cpp-host's bare-empty-map-allow-all semantic (see upstream
// `src/wasm.cc:159-206` where `capabilityAllowed("")` defaults to allow-all
// when the allowed-set is empty).
//
// Test matrix (per 25.1 PLAN Task 6 component-table + 25.2 PLAN Task 2
// extension per AMEND-B5):
//   1. EmptyAllowedCapabilities denies every key in the 58-key roster
//      (37 from 25.1 + 21 NEW per 25.2 AMEND-B5).
//   2. AllowedKeys permits only the listed key (proxy_log).
//   3. AllAllow permits every key when the full 58-key roster is populated.
//   4. UnknownKey is always denied regardless of map population.
//   5. SanitizationConfig empty value is accepted as no-op (AMEND-A1 §11.4).
//   6. ModuleInitCallbacks are documented as ungated per D-P2 closure;
//      the constants exist for roster completeness but are NOT consulted
//      by the actual module-init dispatch (Task 7 vm.Run will bypass).
//   7. FullRoster ByteStable — table-driven enumeration of all 58 keys
//      verifies the roster size is EXACTLY 58 (byte-stable count assertion).
//   8. 25.2 NEW-21 keys (14 hostcall + 7 lifecycle per 25.2 SPEC §11.5
//      D-25.2-5): per-key ALLOW (explicit opt-in returns true) + DENY
//      (zero-value sandbox returns false) exhaustive. Gate-at-registration
//      (env-namespace) + gate-at-getFunction (lifecycle) discipline per
//      AMEND-B5 is enforced at registration.go Task 3 — at Task 2 only
//      the IsAllowed semantic is verified here.

package wasm

import (
	"testing"
)

// fullCapabilityRoster25_1 enumerates EVERY capability key materialized at
// phase 25.1. The total is exactly 37 (asserted byte-stable in
// TestSandboxConfig_FullRoster_ByteStable_25_1):
//
//	7 headers-bridge + 1 local-response + 2 property + 2 log + 1 status +
//	1 time + 2 context-lifecycle + 8 WASI (refs from wasi.go) +
//	5 module-init/allocator (ungated per D-P2) + 8 lifecycle/HTTP module-getters
//	= 37 keys
//
// Each entry is one of the package-private `cap*` constants from either
// `sandbox.go` (this task) or `wasi.go` (Task 4). Tests reference both
// sources via the package's internal scope.
func fullCapabilityRoster25_1() []string {
	return []string{
		// Headers-bridge family (7 keys per SPEC §3.3).
		capProxyGetHeaderMapPairs,
		capProxySetHeaderMapPairs,
		capProxyGetHeaderMapValue,
		capProxyAddHeaderMapValue,
		capProxyReplaceHeaderMapValue,
		capProxyRemoveHeaderMapValue,
		capProxyGetHeaderMapSize,

		// Local-response (1 key).
		capProxySendLocalResponse,

		// Property (2 keys).
		capProxyGetProperty,
		capProxySetProperty,

		// Log (2 keys).
		capProxyLog,
		capProxyGetLogLevel,

		// Status (1 key).
		capProxyGetStatus,

		// Time (1 key).
		capProxyGetCurrentTimeNanoseconds,

		// Context-lifecycle (2 keys).
		capProxySetEffectiveContext,
		capProxyDone,

		// WASI (8 keys — defined in wasi.go, NOT re-declared in sandbox.go).
		capWasiFdWrite,
		capWasiClockTimeGet,
		capWasiRandomGet,
		capWasiEnvironSizesGet,
		capWasiEnvironGet,
		capWasiArgsSizesGet,
		capWasiArgsGet,
		capWasiProcExit,

		// Module-init / allocator (5 keys; ungated per D-P2 — these
		// constants exist for documentation completeness; the actual
		// module-init dispatch at Task 7 vm.Run bypasses IsAllowed
		// per upstream `proxy-wasm-cpp-host:wasm.cc:159-178`).
		capModuleInitialize,
		capModuleStart,
		capModuleMain,
		capAllocatorMalloc,
		capAllocatorProxyOnMemoryAllocate,

		// Lifecycle + HTTP callback module-getters (8 keys; gated at
		// getFunction time per upstream `wasm.cc:181-206` `_GET_PROXY`
		// + `_GET_PROXY_ABI` macros).
		capProxyOnContextCreate,
		capProxyOnVmStart,
		capProxyOnConfigure,
		capProxyOnDone,
		capProxyOnDelete,
		capProxyOnLog,
		capProxyOnRequestHeaders,
		capProxyOnResponseHeaders,
	}
}

// new25_2HostcallCapabilityKeys enumerates the 14 NEW env-namespace hostcall
// capability keys materialized at phase 25.2 per 25.2 SPEC §11.5 D-25.2-5
// (hostcall-keys table). Per AMEND-B5: gating happens at `registerCallback`
// time in upstream `wasm.cc:176-189` `_REGISTER_PROXY` macro — denied
// capabilities → NOT registered on the wazero Runtime (envoy-go mirrors at
// `registration.go` Task 3). Order matches the 25.2 SPEC §11.5 table
// (NOT alphabetical — preserves traceability to AMEND-B5).
func new25_2HostcallCapabilityKeys() []string {
	return []string{
		// Body / buffer (3 keys per AMEND-B1 clamp-on-overflow).
		capProxyGetBufferBytes,
		capProxySetBufferBytes,
		capProxyGetBufferStatus,

		// Stream-control (2 keys; ABI-specific per upstream exports.h:154-156).
		capProxyContinueStream,
		capProxyCloseStream,

		// Timer (1 key; paired with proxy_on_tick lifecycle below).
		capProxySetTickPeriodMilliseconds,

		// Metrics (4 keys per AMEND-B2 signed-i64 delta + namespace).
		capProxyDefineMetric,
		capProxyIncrementMetric,
		capProxyRecordMetric,
		capProxyGetMetric,

		// Shared-data (2 keys per Q6 CAS + caps).
		capProxySetSharedData,
		capProxyGetSharedData,

		// Outbound HTTP (1 key per AMEND-B3 cancel-at-destruction).
		capProxyHttpCall,

		// Foreign-function (1 key per AMEND-A9 EMPTY default registry).
		capProxyCallForeignFunction,
	}
}

// new25_2LifecycleCapabilityKeys enumerates the 7 NEW lifecycle (proxy_on_*)
// capability keys materialized at phase 25.2 per 25.2 SPEC §11.5 D-25.2-5
// (lifecycle-keys table). Per AMEND-B5: gating happens at `getFunction`
// time in upstream `wasm.cc:238-247` `_GET_PROXY` macro — denied capabilities
// → function pointer set to nullptr-equivalent + dispatch path skips.
// Order matches the 25.2 SPEC §11.5 table.
func new25_2LifecycleCapabilityKeys() []string {
	return []string{
		capProxyOnRequestBody,
		capProxyOnResponseBody,
		capProxyOnRequestTrailers,
		capProxyOnResponseTrailers,
		capProxyOnTick,
		capProxyOnHttpCallResponse,
		capProxyOnForeignFunction,
	}
}

// fullCapabilityRoster25_2 enumerates EVERY capability key materialized at
// phase 25.2 (the 25.1 37-key roster + 21 NEW keys per AMEND-B5). Total
// = 58 (asserted byte-stable in TestSandboxConfig_FullRoster_ByteStable_25_2):
//
//	37 (25.1 cumulative) + 14 (NEW hostcall) + 7 (NEW lifecycle) = 58 keys
func fullCapabilityRoster25_2() []string {
	roster := fullCapabilityRoster25_1()
	roster = append(roster, new25_2HostcallCapabilityKeys()...)
	roster = append(roster, new25_2LifecycleCapabilityKeys()...)
	return roster
}

// TestSandboxConfig_EmptyAllowedCapabilities_DeniesAll exercises the
// StrictDefaultDeny posture per AMEND-A5. Constructs the zero value
// (nil AllowedCapabilities map) and verifies every capability key in the
// 58-key cumulative 25.2 roster is DENIED. INVERTS upstream's bare-empty-
// map-allow-all.
func TestSandboxConfig_EmptyAllowedCapabilities_DeniesAll(t *testing.T) {
	roster := fullCapabilityRoster25_2()

	// Zero-value sandbox: AllowedCapabilities is nil.
	var sb SandboxConfig
	for _, key := range roster {
		t.Run("nil_map/"+key, func(t *testing.T) {
			if got := sb.IsAllowed(key); got {
				t.Errorf("zero-value SandboxConfig.IsAllowed(%q) = true, want false (StrictDefaultDeny per AMEND-A5)", key)
			}
		})
	}

	// Explicit empty map: same semantic as the nil map.
	sbEmpty := SandboxConfig{AllowedCapabilities: map[string]SanitizationConfig{}}
	for _, key := range roster {
		t.Run("empty_map/"+key, func(t *testing.T) {
			if got := sbEmpty.IsAllowed(key); got {
				t.Errorf("empty-map SandboxConfig.IsAllowed(%q) = true, want false (StrictDefaultDeny per AMEND-A5)", key)
			}
		})
	}
}

// TestSandboxConfig_AllowedKeys_PermitsOnlyListed verifies that an explicit
// opt-in for ONE key permits ONLY that key — the other 57 stay denied.
func TestSandboxConfig_AllowedKeys_PermitsOnlyListed(t *testing.T) {
	sb := SandboxConfig{
		AllowedCapabilities: map[string]SanitizationConfig{
			capProxyLog: {},
		},
	}

	// proxy_log is permitted.
	if !sb.IsAllowed(capProxyLog) {
		t.Fatalf("SandboxConfig.IsAllowed(%q) = false, want true (explicitly allowed)", capProxyLog)
	}

	// Every OTHER capability in the 25.2 cumulative roster is denied.
	for _, key := range fullCapabilityRoster25_2() {
		if key == capProxyLog {
			continue
		}
		t.Run("denied/"+key, func(t *testing.T) {
			if got := sb.IsAllowed(key); got {
				t.Errorf("SandboxConfig.IsAllowed(%q) = true, want false (only proxy_log was opted-in)", key)
			}
		})
	}
}

// TestSandboxConfig_AllAllow_PermitsAll verifies that populating the map
// with every key in the 58-key cumulative 25.2 roster results in every
// IsAllowed returning true.
func TestSandboxConfig_AllAllow_PermitsAll(t *testing.T) {
	roster := fullCapabilityRoster25_2()
	allowed := make(map[string]SanitizationConfig, len(roster))
	for _, key := range roster {
		allowed[key] = SanitizationConfig{}
	}
	sb := SandboxConfig{AllowedCapabilities: allowed}

	for _, key := range roster {
		t.Run(key, func(t *testing.T) {
			if !sb.IsAllowed(key) {
				t.Errorf("SandboxConfig.IsAllowed(%q) = false, want true (all 58 keys opted-in)", key)
			}
		})
	}
}

// TestSandboxConfig_UnknownKey_AlwaysDenied verifies that capability keys
// outside the materialized 25.2 cumulative roster are ALWAYS denied
// regardless of map population (no implicit allow-by-absence semantic).
func TestSandboxConfig_UnknownKey_AlwaysDenied(t *testing.T) {
	const fakeKey = "not_a_real_capability"

	// Against the zero-value sandbox.
	var sbZero SandboxConfig
	if sbZero.IsAllowed(fakeKey) {
		t.Errorf("zero-value SandboxConfig.IsAllowed(%q) = true, want false", fakeKey)
	}

	// Against a sandbox that has ALL real keys allowed (the fake key is
	// still not in the map).
	roster := fullCapabilityRoster25_2()
	allowed := make(map[string]SanitizationConfig, len(roster))
	for _, key := range roster {
		allowed[key] = SanitizationConfig{}
	}
	sbAll := SandboxConfig{AllowedCapabilities: allowed}
	if sbAll.IsAllowed(fakeKey) {
		t.Errorf("all-allow SandboxConfig.IsAllowed(%q) = true, want false (unknown key not in map)", fakeKey)
	}

	// Against a sandbox that has ONLY the fake key — to prove the lookup
	// is by exact string match (no normalization / no prefix-strip).
	sbFake := SandboxConfig{
		AllowedCapabilities: map[string]SanitizationConfig{
			fakeKey: {},
		},
	}
	if !sbFake.IsAllowed(fakeKey) {
		t.Errorf("SandboxConfig.IsAllowed(%q) = false, want true (exact-match lookup)", fakeKey)
	}
	// And the real keys are denied.
	for _, key := range roster {
		if sbFake.IsAllowed(key) {
			t.Errorf("SandboxConfig (only %q allowed) .IsAllowed(%q) = true, want false", fakeKey, key)
		}
	}
}

// TestSandboxConfig_SanitizationConfigEmpty_AcceptedAsNoOp verifies that the
// zero-value `SanitizationConfig{}` is accepted as a valid map value per
// AMEND-A1 §11.4 + parent §4.3.5 (upstream's SanitizationConfig proto is
// EMPTY and marked "currently unimplemented and ignored, and so should be
// left empty"). The empty value is the SOLE valid value at 25.1.
func TestSandboxConfig_SanitizationConfigEmpty_AcceptedAsNoOp(t *testing.T) {
	// SanitizationConfig is a struct{} with zero fields — verify by
	// construction that it can be the value in the map.
	cfg := SanitizationConfig{}
	sb := SandboxConfig{
		AllowedCapabilities: map[string]SanitizationConfig{
			capProxyLog:                       cfg,
			capProxyGetCurrentTimeNanoseconds: {},
		},
	}

	if !sb.IsAllowed(capProxyLog) {
		t.Errorf("SandboxConfig.IsAllowed(%q) = false, want true (empty SanitizationConfig value accepted)", capProxyLog)
	}
	if !sb.IsAllowed(capProxyGetCurrentTimeNanoseconds) {
		t.Errorf("SandboxConfig.IsAllowed(%q) = false, want true (empty SanitizationConfig value accepted)", capProxyGetCurrentTimeNanoseconds)
	}
}

// TestSandboxConfig_ModuleInitCallbacks_UngatedBehaviorDocumented is a
// DOCUMENTATION test for D-P2 closure. The 5 module-init / allocator
// capability constants (`_initialize`, `_start`, `main`, `malloc`,
// `proxy_on_memory_allocate`) exist in sandbox.go for roster completeness,
// but per upstream `proxy-wasm-cpp-host@da3ce05d:src/wasm.cc:159-178` the
// `WasmBase::getFunctions()` method retrieves these via the bare `_GET`
// macro — which does NOT consult `capabilityAllowed()`. By contrast, the
// proxy_on_* callbacks at lines 181-206 use `_GET_PROXY` / `_GET_PROXY_ABI`
// macros that DO consult `capabilityAllowed()`.
//
// Disposition (D-P2 closure): **UNGATED**. The 5 module-init/allocator
// callbacks bypass the capability gate. Task 7 vm.Run will invoke
// `_initialize` / `_start` / `main` directly via the wazero ExportedFunction
// lookup WITHOUT calling sb.IsAllowed first. The constants exist here so
// that downstream readers grepping for "_initialize" / "malloc" find a
// single source of truth for the capability key strings.
//
// This test verifies:
//   - The 5 constants exist as package-private `cap*` symbols (compile-time
//     check via reference in the test body).
//   - The constant values are byte-stable (verified by direct string
//     comparison to the upstream-faithful names).
//   - sb.IsAllowed returns false for these keys at the zero-value posture
//     (consistent with the default-deny semantic), but per the D-P2
//     disposition the host MUST still be able to call them — that
//     verification lives at Task 7 vm.go (this test merely documents the
//     contract).
func TestSandboxConfig_ModuleInitCallbacks_UngatedBehaviorDocumented(t *testing.T) {
	// The 5 module-init / allocator capability keys (byte-stable per upstream).
	moduleInitKeys := map[string]string{
		"_initialize":              capModuleInitialize,
		"_start":                   capModuleStart,
		"main":                     capModuleMain,
		"malloc":                   capAllocatorMalloc,
		"proxy_on_memory_allocate": capAllocatorProxyOnMemoryAllocate,
	}

	// Verify each constant matches the upstream-faithful bare name.
	for want, got := range moduleInitKeys {
		if got != want {
			t.Errorf("module-init capability key mismatch: got %q, want %q (byte-stable per upstream wasm.cc:159-178)", got, want)
		}
	}

	// Verify sb.IsAllowed returns false for these keys at the zero-value
	// posture. This is consistent with the default-deny semantic; the
	// D-P2 disposition is that Task 7 vm.Run BYPASSES this gate for the
	// 5 module-init keys.
	var sb SandboxConfig
	for _, key := range moduleInitKeys {
		if sb.IsAllowed(key) {
			t.Errorf("zero-value SandboxConfig.IsAllowed(%q) = true, want false", key)
		}
	}
}

// TestSandboxConfig_FullRoster_ByteStable_25_1 is a byte-stable integrity
// check that the 25.1 capability roster has EXACTLY 37 keys (no accidental
// addition/removal). Per the SPEC §3.3 breakdown:
//
//	7 headers-bridge + 1 local-response + 2 property + 2 log + 1 status +
//	1 time + 2 context-lifecycle + 8 WASI + 5 module-init/allocator +
//	8 lifecycle/HTTP module-getters = 37
//
// Additionally verifies each key is unique (no duplicates).
func TestSandboxConfig_FullRoster_ByteStable_25_1(t *testing.T) {
	roster := fullCapabilityRoster25_1()

	const want = 37
	if got := len(roster); got != want {
		t.Errorf("fullCapabilityRoster25_1() returned %d keys, want %d", got, want)
	}

	seen := make(map[string]int, len(roster))
	for i, key := range roster {
		if prev, dup := seen[key]; dup {
			t.Errorf("duplicate capability key %q at indices %d and %d", key, prev, i)
		}
		seen[key] = i
	}

	// Drive a table-driven IsAllowed check across the full roster against
	// an all-allow sandbox; every entry MUST be allowed.
	allowed := make(map[string]SanitizationConfig, len(roster))
	for _, key := range roster {
		allowed[key] = SanitizationConfig{}
	}
	sb := SandboxConfig{AllowedCapabilities: allowed}
	for _, key := range roster {
		if !sb.IsAllowed(key) {
			t.Errorf("all-allow SandboxConfig.IsAllowed(%q) = false, want true", key)
		}
	}
}

// TestSandboxConfig_FullRoster_ByteStable_25_2 is the byte-stable integrity
// check for the 25.2 cumulative roster — EXACTLY 58 keys (37 from 25.1 +
// 21 NEW at 25.2 per AMEND-B5 + 25.2 SPEC §11.5 D-25.2-5):
//
//	37 (25.1 cumulative) +
//	14 NEW hostcall (body/buffer 3 + stream-ctl 2 + timer 1 + metrics 4
//	  + shared-data 2 + outbound-HTTP 1 + foreign-fn 1) +
//	7 NEW lifecycle (request_body + response_body + request_trailers
//	  + response_trailers + tick + http_call_response + foreign_function)
//	= 58
//
// Additionally verifies each key is unique across the cumulative roster
// (catches accidental cross-phase key collisions).
func TestSandboxConfig_FullRoster_ByteStable_25_2(t *testing.T) {
	roster := fullCapabilityRoster25_2()

	const want = 58
	if got := len(roster); got != want {
		t.Errorf("fullCapabilityRoster25_2() returned %d keys, want %d (37 from 25.1 + 21 NEW per AMEND-B5)", got, want)
	}

	// 25.1 + 25.2-NEW partition counts.
	if got := len(fullCapabilityRoster25_1()); got != 37 {
		t.Errorf("fullCapabilityRoster25_1() returned %d keys, want 37 (25.1 baseline; must not drift)", got)
	}
	if got := len(new25_2HostcallCapabilityKeys()); got != 14 {
		t.Errorf("new25_2HostcallCapabilityKeys() returned %d keys, want 14 (per 25.2 SPEC §11.5 hostcall-keys table)", got)
	}
	if got := len(new25_2LifecycleCapabilityKeys()); got != 7 {
		t.Errorf("new25_2LifecycleCapabilityKeys() returned %d keys, want 7 (per 25.2 SPEC §11.5 lifecycle-keys table)", got)
	}

	seen := make(map[string]int, len(roster))
	for i, key := range roster {
		if prev, dup := seen[key]; dup {
			t.Errorf("duplicate capability key %q at indices %d and %d (cross-phase 25.1+25.2 collision)", key, prev, i)
		}
		seen[key] = i
	}

	// Drive a table-driven IsAllowed check across the full 58-key roster
	// against an all-allow sandbox; every entry MUST be allowed.
	allowed := make(map[string]SanitizationConfig, len(roster))
	for _, key := range roster {
		allowed[key] = SanitizationConfig{}
	}
	sb := SandboxConfig{AllowedCapabilities: allowed}
	for _, key := range roster {
		if !sb.IsAllowed(key) {
			t.Errorf("all-allow SandboxConfig.IsAllowed(%q) = false, want true", key)
		}
	}
}

// TestSandboxConfig_25_2_NewHostcallKeys_PerKeyAllowDeny exercises each of
// the 14 NEW env-namespace hostcall capability keys materialized at 25.2
// per AMEND-B5 + 25.2 SPEC §11.5 D-25.2-5 (hostcall-keys table). For each
// key: ALLOW (explicit opt-in via single-key map returns true) + DENY
// (zero-value sandbox returns false). Verifies the key-string is byte-
// stable to the upstream cpp-host `proxy_<base>` format (NOT alphabetized
// — matches the SPEC table order for traceability).
//
// NOTE: This test ONLY verifies the IsAllowed semantic at the constant
// level. The gate-at-registration discipline per AMEND-B5 (denied keys
// → NOT registered on wazero Runtime) is asserted at Task 3
// registration_test.go.
func TestSandboxConfig_25_2_NewHostcallKeys_PerKeyAllowDeny(t *testing.T) {
	// Map of constant-symbol → upstream-faithful key string (byte-stable
	// per 25.2 SPEC §11.5 hostcall-keys table). The order of slice entries
	// matches the SPEC table (1..14) for traceability to AMEND-B5.
	cases := []struct {
		name string // human label for t.Run sub-test
		key  string // upstream-faithful capability key (proxy_<base>)
		cap  string // package-private cap* constant value (same as key)
	}{
		{"01_proxy_get_buffer_bytes", "proxy_get_buffer_bytes", capProxyGetBufferBytes},
		{"02_proxy_set_buffer_bytes", "proxy_set_buffer_bytes", capProxySetBufferBytes},
		{"03_proxy_get_buffer_status", "proxy_get_buffer_status", capProxyGetBufferStatus},
		{"04_proxy_continue_stream", "proxy_continue_stream", capProxyContinueStream},
		{"05_proxy_close_stream", "proxy_close_stream", capProxyCloseStream},
		{"06_proxy_set_tick_period_milliseconds", "proxy_set_tick_period_milliseconds", capProxySetTickPeriodMilliseconds},
		{"07_proxy_define_metric", "proxy_define_metric", capProxyDefineMetric},
		{"08_proxy_increment_metric", "proxy_increment_metric", capProxyIncrementMetric},
		{"09_proxy_record_metric", "proxy_record_metric", capProxyRecordMetric},
		{"10_proxy_get_metric", "proxy_get_metric", capProxyGetMetric},
		{"11_proxy_set_shared_data", "proxy_set_shared_data", capProxySetSharedData},
		{"12_proxy_get_shared_data", "proxy_get_shared_data", capProxyGetSharedData},
		{"13_proxy_http_call", "proxy_http_call", capProxyHttpCall},
		{"14_proxy_call_foreign_function", "proxy_call_foreign_function", capProxyCallForeignFunction},
	}

	if got := len(cases); got != 14 {
		t.Fatalf("test case table has %d entries, want 14 (NEW hostcall keys per AMEND-B5)", got)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Byte-stable: constant value MUST equal upstream-faithful key.
			if tc.cap != tc.key {
				t.Errorf("capability key constant mismatch: got %q, want %q (byte-stable per upstream proxy_<base> format)", tc.cap, tc.key)
			}

			// DENY: zero-value sandbox denies the key.
			var sbZero SandboxConfig
			if sbZero.IsAllowed(tc.key) {
				t.Errorf("zero-value SandboxConfig.IsAllowed(%q) = true, want false (StrictDefaultDeny per AMEND-A5)", tc.key)
			}

			// ALLOW: explicit opt-in via single-key map permits the key.
			sbAllow := SandboxConfig{
				AllowedCapabilities: map[string]SanitizationConfig{
					tc.key: {},
				},
			}
			if !sbAllow.IsAllowed(tc.key) {
				t.Errorf("single-key-allow SandboxConfig.IsAllowed(%q) = false, want true (explicit opt-in)", tc.key)
			}
		})
	}
}

// TestSandboxConfig_25_2_NewLifecycleKeys_PerKeyAllowDeny exercises each of
// the 7 NEW lifecycle (proxy_on_*) capability keys materialized at 25.2
// per AMEND-B5 + 25.2 SPEC §11.5 D-25.2-5 (lifecycle-keys table). For each
// key: ALLOW (explicit opt-in returns true) + DENY (zero-value returns
// false). Order matches the SPEC table (15..21) for traceability.
//
// NOTE: Gate-at-getFunction discipline per AMEND-B5 (denied keys → function
// pointer set to nullptr-equivalent; dispatch path skips) is asserted at
// Task 3 registration_test.go.
func TestSandboxConfig_25_2_NewLifecycleKeys_PerKeyAllowDeny(t *testing.T) {
	cases := []struct {
		name string
		key  string
		cap  string
	}{
		{"15_proxy_on_request_body", "proxy_on_request_body", capProxyOnRequestBody},
		{"16_proxy_on_response_body", "proxy_on_response_body", capProxyOnResponseBody},
		{"17_proxy_on_request_trailers", "proxy_on_request_trailers", capProxyOnRequestTrailers},
		{"18_proxy_on_response_trailers", "proxy_on_response_trailers", capProxyOnResponseTrailers},
		{"19_proxy_on_tick", "proxy_on_tick", capProxyOnTick},
		{"20_proxy_on_http_call_response", "proxy_on_http_call_response", capProxyOnHttpCallResponse},
		{"21_proxy_on_foreign_function", "proxy_on_foreign_function", capProxyOnForeignFunction},
	}

	if got := len(cases); got != 7 {
		t.Fatalf("test case table has %d entries, want 7 (NEW lifecycle keys per AMEND-B5)", got)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.cap != tc.key {
				t.Errorf("capability key constant mismatch: got %q, want %q (byte-stable per upstream proxy_on_<event> format)", tc.cap, tc.key)
			}

			var sbZero SandboxConfig
			if sbZero.IsAllowed(tc.key) {
				t.Errorf("zero-value SandboxConfig.IsAllowed(%q) = true, want false (StrictDefaultDeny per AMEND-A5)", tc.key)
			}

			sbAllow := SandboxConfig{
				AllowedCapabilities: map[string]SanitizationConfig{
					tc.key: {},
				},
			}
			if !sbAllow.IsAllowed(tc.key) {
				t.Errorf("single-key-allow SandboxConfig.IsAllowed(%q) = false, want true (explicit opt-in)", tc.key)
			}
		})
	}
}
