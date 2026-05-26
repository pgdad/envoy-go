// Tests for the host-module registration + ABICallbacks invocation
// round-trip per 25.1 SPEC §5 + Task 7 acceptance criteria.
//
// MIGRATED at 25.2 Task 1 per D-P-PLAN-6: the test now constructs a
// *RootVM via NewRootVM(ctx, module, rootCtxID, opts...) + Configure
// instead of the retired 25.1 per-stream *VM. The hostcall surface +
// ABICallbacks interface + gate discipline are UNCHANGED at this Task.
//
// Coverage:
//   - Host-module wiring smoke test: env + wasi_snapshot_preview1 modules
//     registered correctly; a guest module importing any hostcall instantiates
//     cleanly. The hostcall count check (24 active + 23 deferred = 47 total)
//     is enforced via the importer-table fixture covering every registered
//     hostcall name.
//   - ABICallbacks invocation round-trip: proxy_log fires the cb.Log Go method;
//     proxy_get_log_level writes the cb return value to guest memory; etc.
//   - Sandbox-deny: when the capability is denied via SandboxConfig, the
//     hostcall returns WasmResultInternalFailure (=10) for proxy_*; the
//     wasi-deny path returns WasiErrnoNotcapable (=76) via the wasi.go shim
//     (already covered in wasi_test.go but spot-checked here for the
//     registration-time wiring).
//   - Deferred stubs: each of the 23 deferred hostcalls returns
//     WasmResultUnimplemented (=12) when invoked.
//   - Helper functions: readString + splitPath + writeReturnBuffer behavior.

package wasm

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// --- TestRegistration_FullRoster_ImportableWithoutError -------------------

// TestRegistration_FullRoster_ImportableWithoutError builds a wasm module
// that imports every one of the 47 registered hostcalls + verifies the
// module instantiates cleanly against a fresh RootVM.
//
// At 25.2 (Task 3): the test now constructs the RootVM with allowAllSandbox
// so the 14 NEW 25.2 gated hostcalls per AMEND-B5 are registered. Under
// default deny-all sandbox the 14 NEW would NOT be registered + the
// importer would fail at module-instantiation with "unknown import" —
// covered by TestGateAtRegistration_NewHostcall_NotRegistered_When_Denied
// below.
func TestRegistration_FullRoster_ImportableWithoutError(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	// The fixture imports every active + deferred hostcall.
	inst, err := rv.runtime.Instantiate(ctx, fullRosterImporterModule)
	if err != nil {
		t.Fatalf("instantiate full-roster importer: %v", err)
	}
	defer func() { _ = inst.Close(ctx) }()
}

// --- TestRegistration_ProxyLog_RoundTrip ----------------------------------

// TestRegistration_ProxyLog_RoundTrip verifies the complete proxy_log
// dispatch: guest call → host hostcall body → sandbox check → ABICallbacks
// invocation → return value to guest.
func TestRegistration_ProxyLog_RoundTrip(t *testing.T) {
	ctx := context.Background()
	cb := &fakeABICallbacks{}

	mod := mustCompileWithCacheForRootVM(t, ctx, invokeProxyLogModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	rv.RegisterABICallbacks(cb)
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	invoke := rv.instance.ExportedFunction("invoke_proxy_log")
	results, err := invoke.Call(ctx)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	// The fixture invokes proxy_log(2, 0, 0) → returns WasmResultOk (=0).
	if got := uint32(results[0]); got != uint32(abi.WasmResultOk) {
		t.Errorf("proxy_log return = %d, want %d (Ok)", got, abi.WasmResultOk)
	}
	if cb.logLastLevel != abi.LogLevelInfo {
		t.Errorf("cb.Log level = %d, want %d (Info)", cb.logLastLevel, abi.LogLevelInfo)
	}
	if cb.logLastMsg != "" {
		t.Errorf("cb.Log msg = %q, want \"\"", cb.logLastMsg)
	}
}

// --- TestRegistration_ProxyLog_SandboxDeny --------------------------------

// TestRegistration_ProxyLog_SandboxDeny verifies that when the proxy_log
// capability is denied, the hostcall returns WasmResultInternalFailure
// without invoking the ABICallbacks Go method.
func TestRegistration_ProxyLog_SandboxDeny(t *testing.T) {
	ctx := context.Background()
	cb := &fakeABICallbacks{}

	// Deny-all sandbox (no capability allowed).
	mod := mustCompileWithCacheForRootVM(t, ctx, invokeProxyLogModule)
	rv, err := NewRootVM(ctx, mod, 1)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	rv.RegisterABICallbacks(cb)
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	invoke := rv.instance.ExportedFunction("invoke_proxy_log")
	results, err := invoke.Call(ctx)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if got := uint32(results[0]); got != uint32(abi.WasmResultInternalFailure) {
		t.Errorf("proxy_log return on deny = %d, want %d (InternalFailure)", got, abi.WasmResultInternalFailure)
	}
	if cb.logLastMsg != "" {
		t.Errorf("cb.Log invoked on sandbox-deny: %q (must be empty)", cb.logLastMsg)
	}
}

// --- TestRegistration_ProxyLog_DenyLogged ---------------------------------

// TestRegistration_ProxyLog_DenyLogged verifies the integration-log line
// written to rv.logSink on sandbox-deny ("hostcall denied: proxy_log").
func TestRegistration_ProxyLog_DenyLogged(t *testing.T) {
	ctx := context.Background()
	var sink bytes.Buffer
	mod := mustCompileWithCacheForRootVM(t, ctx, invokeProxyLogModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootLogSink(&sink))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	rv.RegisterABICallbacks(&fakeABICallbacks{})
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	invoke := rv.instance.ExportedFunction("invoke_proxy_log")
	if _, err := invoke.Call(ctx); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	out := sink.String()
	if !strings.Contains(out, "hostcall denied: proxy_log") {
		t.Errorf("integration log missing: %q", out)
	}
}

// --- TestRegistration_DeferredStub_Unimplemented --------------------------

// TestRegistration_DeferredStub_Unimplemented verifies that a still-deferred
// hostcall stub returns WasmResultUnimplemented (=12) when invoked from a
// guest, without consulting the sandbox.
//
// At 25.2 Task 3: the 23-stub roster at 25.1 SHRUNK to 9 (shared-queue 4 +
// gRPC 5) — 14 stubs LIFTED to gated active per §5.1. This test now uses
// proxy_grpc_cancel(0) — a 1-arg STILL-deferred gRPC-family stub — to
// preserve the original Unimplemented-on-invoke assertion. The 25.1-era
// proxy_continue_stream LIFTED to gated active (covered by new tests below).
func TestRegistration_DeferredStub_Unimplemented(t *testing.T) {
	ctx := context.Background()

	// Use the fixture that calls proxy_grpc_cancel(0) — a 1-arg STILL-deferred stub.
	mod := mustCompileWithCacheForRootVM(t, ctx, invokeGrpcCancelModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	invoke := rv.instance.ExportedFunction("invoke_grpc_cancel")
	results, err := invoke.Call(ctx)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if got := uint32(results[0]); got != uint32(abi.WasmResultUnimplemented) {
		t.Errorf("proxy_grpc_cancel return = %d, want %d (Unimplemented)", got, abi.WasmResultUnimplemented)
	}
}

// --- TestRegistration_ABICallbacksInterface -------------------------------

// TestRegistration_ABICallbacksInterface verifies the fakeABICallbacks
// (defined in testhelpers_test.go) satisfies the ABICallbacks interface at
// runtime. The compile-time guard `var _ ABICallbacks = (*fakeABICallbacks)(nil)`
// already enforces this; this test exercises representative methods.
func TestRegistration_ABICallbacksInterface(t *testing.T) {
	cb := &fakeABICallbacks{
		getHeaderMapOK:         true,
		getHeaderMapReturn:     []HeaderPair{{Key: "k1", Value: "v1"}},
		getHeaderMapSizeReturn: 5,
		addReturn:              abi.WasmResultOk,
		getLogLevelReturn:      abi.LogLevelWarn,
		getCurrentTimeReturn:   42,
	}
	ctx := context.Background()

	pairs, ok := cb.GetHeaderMap(ctx, 1, abi.WasmHeaderMapTypeHttpRequestHeaders)
	if !ok || len(pairs) != 1 || pairs[0].Key != "k1" {
		t.Errorf("GetHeaderMap: pairs=%v ok=%v", pairs, ok)
	}
	if got := cb.GetHeaderMapSize(ctx, 1, abi.WasmHeaderMapTypeHttpRequestHeaders); got != 5 {
		t.Errorf("GetHeaderMapSize: got %d, want 5", got)
	}
	if got := cb.AddHeaderMapValue(ctx, 1, abi.WasmHeaderMapTypeHttpRequestHeaders, "k", "v"); got != abi.WasmResultOk {
		t.Errorf("AddHeaderMapValue: got %d, want Ok", got)
	}
	if got := cb.GetLogLevel(ctx); got != abi.LogLevelWarn {
		t.Errorf("GetLogLevel: got %d, want %d", got, abi.LogLevelWarn)
	}
	if got := cb.GetCurrentTimeNanoseconds(ctx); got != 42 {
		t.Errorf("GetCurrentTimeNanoseconds: got %d, want 42", got)
	}
	cb.Log(ctx, 1, abi.LogLevelDebug, "msg")
	calls := cb.callsSnapshot()
	if len(calls) == 0 {
		t.Error("no calls recorded")
	}
}

// --- TestReadString -------------------------------------------------------

func TestReadString(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	inst, err := rv.runtime.Instantiate(ctx, importerProxyLogModule)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	defer func() { _ = inst.Close(ctx) }()

	t.Run("zero-size returns empty + ok", func(t *testing.T) {
		s, ok := readString(inst, 0, 0)
		if !ok || s != "" {
			t.Errorf("got (%q, %v), want (\"\", true)", s, ok)
		}
	})

	t.Run("valid bytes return string", func(t *testing.T) {
		inst.Memory().Write(0x100, []byte("hello"))
		s, ok := readString(inst, 0x100, 5)
		if !ok || s != "hello" {
			t.Errorf("got (%q, %v), want (\"hello\", true)", s, ok)
		}
	})

	t.Run("OOB returns empty + not-ok", func(t *testing.T) {
		s, ok := readString(inst, 0xFFFFFFFF, 5)
		if ok || s != "" {
			t.Errorf("got (%q, %v), want (\"\", false)", s, ok)
		}
	})
}

// --- TestSplitPath --------------------------------------------------------

func TestSplitPath(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want []string
	}{
		{"empty", []byte{}, nil},
		{"NUL-separated", []byte("a\x00b\x00c"), []string{"a", "b", "c"}},
		{"NUL-terminated", []byte("a\x00b\x00c\x00"), []string{"a", "b", "c"}},
		{"dot-separated fallback", []byte("request.headers.x-foo"), []string{"request", "headers", "x-foo"}},
		{"single segment dot-only", []byte("foo"), []string{"foo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitPath(tc.in)
			if !equalStringSlice(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- TestWriteReturnBuffer_EmptyPayload -----------------------------------

// TestWriteReturnBuffer_EmptyPayload verifies the empty-payload special-case
// of writeReturnBuffer: writes (0, 0) to (ptrPtr, sizePtr) without invoking
// the allocator.
func TestWriteReturnBuffer_EmptyPayload(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	inst, err := rv.runtime.Instantiate(ctx, importerProxyLogModule)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	defer func() { _ = inst.Close(ctx) }()

	// Set sentinels.
	inst.Memory().WriteUint32Le(0x200, 0xDEADBEEF)
	inst.Memory().WriteUint32Le(0x204, 0xCAFEBABE)

	res := writeReturnBuffer(ctx, inst, nil, 0x200, 0x204)
	if res != abi.WasmResultOk {
		t.Fatalf("writeReturnBuffer(nil): %d, want Ok", res)
	}
	gotPtr, _ := inst.Memory().ReadUint32Le(0x200)
	gotSize, _ := inst.Memory().ReadUint32Le(0x204)
	if gotPtr != 0 || gotSize != 0 {
		t.Errorf("empty payload: (ptr=%#x, size=%#x), want (0, 0)", gotPtr, gotSize)
	}
}

// --- TestWriteReturnBuffer_NoAllocator ------------------------------------

// TestWriteReturnBuffer_NoAllocator verifies that when the guest module
// exports neither `malloc` nor `proxy_on_memory_allocate`, a non-empty
// return-buffer write returns WasmResultInvalidMemoryAccess.
func TestWriteReturnBuffer_NoAllocator(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	inst, err := rv.runtime.Instantiate(ctx, importerProxyLogModule)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	defer func() { _ = inst.Close(ctx) }()

	res := writeReturnBuffer(ctx, inst, []byte("hello"), 0x200, 0x204)
	if res != abi.WasmResultInvalidMemoryAccess {
		t.Errorf("writeReturnBuffer no-allocator: %d, want %d (InvalidMemoryAccess)", res, abi.WasmResultInvalidMemoryAccess)
	}
}

// =====================================================================
// 25.2 Task 3 tests — 14 NEW gated hostcalls + 7 NEW gated callbacks +
// gate-at-registration discipline per AMEND-B5 + R-25.2-5
// =====================================================================
//
// The 25.2 NEW gate-at-registration discipline (per AMEND-B5 + cpp-host
// `wasm.cc:176-189` `_REGISTER_PROXY` macro): for each of the 14 NEW
// env-namespace hostcalls, if the corresponding capability is DENIED via
// `vm.sandbox.IsAllowed(<cap>)` at NewRootVM time, the host function is
// NOT registered on the wazero Runtime. Guests importing the denied
// hostcall fail at module-instantiation OR (via the wazero env-module
// hierarchy) at the call site. The 7 NEW lifecycle callbacks gate at
// HasGlobalFunc lookup time per `wasm.cc:238-247` `_GET_PROXY` (denied →
// HasGlobalFunc returns false EVEN IF the guest exports the function).

// new25_2HostcallSignatures pins the 14 NEW 25.2 hostcall name + capability
// + ULEB128 type-index in the fullRosterImporterModule type table (see
// fixtures_test.go::rosterImports + ::types). Test-only registry; the
// production registration roster is in registration.go::registerProxyHost-
// calls25_2.
var new25_2HostcallSignatures = []struct {
	name string
	cap  string
}{
	{"proxy_get_buffer_bytes", capProxyGetBufferBytes},
	{"proxy_set_buffer_bytes", capProxySetBufferBytes},
	{"proxy_get_buffer_status", capProxyGetBufferStatus},
	{"proxy_continue_stream", capProxyContinueStream},
	{"proxy_close_stream", capProxyCloseStream},
	{"proxy_set_tick_period_milliseconds", capProxySetTickPeriodMilliseconds},
	{"proxy_define_metric", capProxyDefineMetric},
	{"proxy_increment_metric", capProxyIncrementMetric},
	{"proxy_record_metric", capProxyRecordMetric},
	{"proxy_get_metric", capProxyGetMetric},
	{"proxy_set_shared_data", capProxySetSharedData},
	{"proxy_get_shared_data", capProxyGetSharedData},
	{"proxy_http_call", capProxyHttpCall},
	{"proxy_call_foreign_function", capProxyCallForeignFunction},
}

// new25_2CallbackSignatures pins the 7 NEW 25.2 lifecycle callback name +
// capability per §5.3 + AMEND-B5.
var new25_2CallbackSignatures = []struct {
	name string
	cap  string
}{
	{"proxy_on_request_body", capProxyOnRequestBody},
	{"proxy_on_response_body", capProxyOnResponseBody},
	{"proxy_on_request_trailers", capProxyOnRequestTrailers},
	{"proxy_on_response_trailers", capProxyOnResponseTrailers},
	{"proxy_on_tick", capProxyOnTick},
	{"proxy_on_http_call_response", capProxyOnHttpCallResponse},
	{"proxy_on_foreign_function", capProxyOnForeignFunction},
}

// sandboxAllowing returns a SandboxConfig that allows exactly the named
// capability keys (everything else DENIED). Used by per-key ALLOW assertion
// tests.
func sandboxAllowing(keys ...string) SandboxConfig {
	allowed := make(map[string]SanitizationConfig, len(keys))
	for _, k := range keys {
		allowed[k] = SanitizationConfig{}
	}
	return SandboxConfig{AllowedCapabilities: allowed}
}

// --- TestRegistration_NewHostcall_Registered_25_2 -------------------------

// TestRegistration_NewHostcall_Registered_25_2 verifies that EACH of the 14
// NEW 25.2 hostcalls is registered on the env host module when its
// capability is ALLOWED. The assertion uses `rv.runtime.Module("env")`'s
// `ExportedFunctionDefinitions()` map to check the function name is
// present.
func TestRegistration_NewHostcall_Registered_25_2(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	envModule := rv.runtime.Module("env")
	if envModule == nil {
		t.Fatalf("rv.runtime.Module(\"env\") == nil; expected env host module registered")
	}
	exports := envModule.ExportedFunctionDefinitions()
	for _, hc := range new25_2HostcallSignatures {
		t.Run(hc.name, func(t *testing.T) {
			if _, ok := exports[hc.name]; !ok {
				t.Errorf("env host module missing %q under allow-all sandbox (gate-at-registration ALLOW path broken)", hc.name)
			}
		})
	}
}

// --- TestRegistration_NewHostcall_NotRegistered_When_Denied_25_2 ----------

// TestRegistration_NewHostcall_NotRegistered_When_Denied_25_2 verifies the
// gate-at-registration deny-path per AMEND-B5: for EACH of the 14 NEW 25.2
// hostcalls, when the capability is DENIED (default deny-all sandbox), the
// hostcall is NOT registered on the env host module. Per-hostcall subtests
// construct a sandbox allowing EVERY 25.2 NEW key EXCEPT the one under
// test; assert the env module's exports exclude the denied name AND include
// the other 13 (to confirm the gate is per-capability, not all-or-nothing).
func TestRegistration_NewHostcall_NotRegistered_When_Denied_25_2(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)

	for _, denied := range new25_2HostcallSignatures {
		t.Run(denied.name, func(t *testing.T) {
			// Build a sandbox allowing every 25.2 NEW key EXCEPT denied.cap.
			allowList := make([]string, 0, len(new25_2HostcallSignatures)-1)
			for _, hc := range new25_2HostcallSignatures {
				if hc.cap == denied.cap {
					continue
				}
				allowList = append(allowList, hc.cap)
			}
			rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(sandboxAllowing(allowList...)))
			if err != nil {
				t.Fatalf("NewRootVM: %v", err)
			}
			defer func() { _ = rv.Close() }()

			envModule := rv.runtime.Module("env")
			if envModule == nil {
				t.Fatalf("rv.runtime.Module(\"env\") == nil")
			}
			exports := envModule.ExportedFunctionDefinitions()

			// Denied hostcall MUST NOT be registered (per AMEND-B5
			// gate-at-registration discipline).
			if _, ok := exports[denied.name]; ok {
				t.Errorf("env host module exports %q under DENY (capability %q); gate-at-registration discipline violated", denied.name, denied.cap)
			}

			// The other 13 NEW hostcalls MUST be registered (capability ALLOW
			// → registered).
			for _, hc := range new25_2HostcallSignatures {
				if hc.cap == denied.cap {
					continue
				}
				if _, ok := exports[hc.name]; !ok {
					t.Errorf("env host module missing %q under ALLOW (capability %q); per-capability gate broken", hc.name, hc.cap)
				}
			}
		})
	}
}

// --- TestRegistration_NewHostcall_DenyRejectsGuestInstantiation_25_2 ------

// TestRegistration_NewHostcall_DenyRejectsGuestInstantiation_25_2 verifies
// the end-to-end gate-at-registration outcome from the guest's perspective:
// when a hostcall capability is DENIED, a guest importing that hostcall
// FAILS at module-instantiation with an "unknown import" error (the gate's
// observable behavior + the test driving the AMEND-B5 R-25.2-5 path).
// Uses the proxy_set_tick_period_milliseconds case per the PLAN-pinned
// example.
func TestRegistration_NewHostcall_DenyRejectsGuestInstantiation_25_2(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)

	// Build a sandbox with everything allowed EXCEPT proxy_set_tick_period_milliseconds.
	allowList := make([]string, 0, len(new25_2HostcallSignatures)-1)
	for _, hc := range new25_2HostcallSignatures {
		if hc.cap == capProxySetTickPeriodMilliseconds {
			continue
		}
		allowList = append(allowList, hc.cap)
	}
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(sandboxAllowing(allowList...)))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	// A fixture that imports proxy_set_tick_period_milliseconds(i32) -> i32
	// MUST fail at module-instantiation because the host did NOT register
	// the function under the deny-this-capability sandbox.
	_, err = rv.runtime.Instantiate(ctx, importerSetTickPeriodModule)
	if err == nil {
		t.Fatalf("instantiate succeeded; expected failure due to denied proxy_set_tick_period_milliseconds")
	}
	if !strings.Contains(err.Error(), "proxy_set_tick_period_milliseconds") {
		t.Errorf("error %q does not mention the denied hostcall name", err.Error())
	}
}

// --- panic-discipline test history (RETIRED at Task 12) -------------------
//
// Tasks 4-8 carried a TestRegistration_NewHostcall_AllowDispatchPanicsTo
// InternalFailure_25_2 test that verified the placeholder-panic discipline:
// when a capability was ALLOWED but the corresponding abi/stubs_25_2.go
// shim was still a "Task N not yet landed" panic placeholder, the
// vm.runWithPanicWrapper envelope converted the panic to
// WasmResultInternalFailure (=10). The test was re-targeted across each
// shim-lift Task:
//
//	Task 4 → proxy_continue_stream (lifted Task 4)
//	Task 4 → proxy_set_tick_period_milliseconds (lifted Task 5)
//	Task 5 → proxy_get_shared_data (lifted Task 6)
//	Task 6 → proxy_call_foreign_function (lifted Task 7)
//	Task 7 → proxy_http_call (lifted Task 8)
//	Task 8 → proxy_define_metric (lifted Task 12)
//
// At Task 12 the last metric placeholder disappears (all 14 Task-3 forward-
// decls activated against real impls across Tasks 4-8 + 12); abi/stubs_25_2.go
// has 0 placeholders + the panic-wrapper coverage is fully exercised by the
// per-family unit tests under -race. The panic-discipline test is RETIRED
// here per the Task 12 PLAN sketch "REMOVED at Task 12 when the last
// placeholder disappears". The general panic-wrapper invariant (Go-panic in
// hostcall body → InternalFailure) remains covered by stream_context_test.go
// TestStreamContext_PanicRecoveryInProxyOnXX patterns + the per-family
// abi/*_test.go non-Host-value tests (e.g., abi.TestMetrics_NonHostHostValue
// at metrics_test.go).

// --- TestGateAtRegistration_NewCallback_HasGlobalFunc_Allow ---------------

// TestGateAtRegistration_NewCallback_HasGlobalFunc_Allow verifies that
// HasGlobalFunc returns true for each of the 7 NEW lifecycle callbacks
// when (a) the guest exports the function AND (b) the capability is
// ALLOWED. Mirrors the cpp-host `_GET_PROXY` macro success-path.
func TestGateAtRegistration_NewCallback_HasGlobalFunc_Allow(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, exportsAll25_2CallbacksModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	for _, cb := range new25_2CallbackSignatures {
		t.Run(cb.name, func(t *testing.T) {
			if !rv.HasGlobalFunc(cb.name) {
				t.Errorf("HasGlobalFunc(%q) = false under allow-all + guest-exports; want true", cb.name)
			}
		})
	}
}

// --- TestGateAtRegistration_NewCallback_HasGlobalFunc_Deny ----------------

// TestGateAtRegistration_NewCallback_HasGlobalFunc_Deny verifies that
// HasGlobalFunc returns false for each of the 7 NEW lifecycle callbacks
// when the capability is DENIED — EVEN IF the guest exports the function.
// Mirrors the cpp-host `_GET_PROXY` macro deny-path (function pointer is
// left null → host treats the missing function as if the guest hadn't
// exported it).
func TestGateAtRegistration_NewCallback_HasGlobalFunc_Deny(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, exportsAll25_2CallbacksModule)

	for _, denied := range new25_2CallbackSignatures {
		t.Run(denied.name, func(t *testing.T) {
			// Build a sandbox allowing every 25.2 NEW callback key EXCEPT
			// denied.cap. ALSO allow all the 14 NEW hostcall keys so the
			// host module registers cleanly + the RootVM constructs.
			allowList := make([]string, 0, len(new25_2HostcallSignatures)+len(new25_2CallbackSignatures)-1)
			for _, hc := range new25_2HostcallSignatures {
				allowList = append(allowList, hc.cap)
			}
			for _, cb := range new25_2CallbackSignatures {
				if cb.cap == denied.cap {
					continue
				}
				allowList = append(allowList, cb.cap)
			}
			rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(sandboxAllowing(allowList...)))
			if err != nil {
				t.Fatalf("NewRootVM: %v", err)
			}
			defer func() { _ = rv.Close() }()

			// Denied callback MUST report missing even though the guest
			// exports it.
			if rv.HasGlobalFunc(denied.name) {
				t.Errorf("HasGlobalFunc(%q) = true under DENY (capability %q); gate-at-getFunction violated", denied.name, denied.cap)
			}

			// The other 6 NEW callbacks MUST report present (per-capability
			// gate, not all-or-nothing).
			for _, cb := range new25_2CallbackSignatures {
				if cb.cap == denied.cap {
					continue
				}
				if !rv.HasGlobalFunc(cb.name) {
					t.Errorf("HasGlobalFunc(%q) = false under ALLOW (capability %q); per-capability gate broken", cb.name, cb.cap)
				}
			}
		})
	}
}

// --- TestGateAtRegistration_NewCallback_NotExported_NotPresent ------------

// TestGateAtRegistration_NewCallback_NotExported_NotPresent verifies the
// guest-not-exported case for the 7 NEW callbacks: HasGlobalFunc returns
// false even with the capability ALLOWED. Confirms the cap-gate is
// short-circuit-on-deny, not a fabricate-on-allow.
func TestGateAtRegistration_NewCallback_NotExported_NotPresent(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule) // no callbacks exported
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	for _, cb := range new25_2CallbackSignatures {
		t.Run(cb.name, func(t *testing.T) {
			if rv.HasGlobalFunc(cb.name) {
				t.Errorf("HasGlobalFunc(%q) = true on a module that does NOT export it; want false", cb.name)
			}
		})
	}
}

// --- TestRegistration_HostModuleTotalCount_25_2 ---------------------------

// TestRegistration_HostModuleTotalCount_25_2 verifies the cumulative
// registered count on the env host module per the 25.2 SPEC §5.5 totals:
//
//   - Under deny-all sandbox: 16 active (25.1) + 0 NEW (25.2) + 9 STILL stub
//     = 25 env-namespace exports.
//   - Under allow-all sandbox: 16 active (25.1) + 14 NEW (25.2) + 9 STILL
//     stub = 39 env-namespace exports.
//
// Plus 8 WASI shims on wasi_snapshot_preview1; the wazero host modules are
// separate so the env count is 25/39 + the WASI count is 8.
func TestRegistration_HostModuleTotalCount_25_2(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)

	t.Run("allow-all 39 env + 8 wasi", func(t *testing.T) {
		rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()

		envExports := rv.runtime.Module("env").ExportedFunctionDefinitions()
		if got, want := len(envExports), 39; got != want {
			t.Errorf("env exports under allow-all = %d, want %d (16 25.1 active + 14 NEW 25.2 + 9 STILL stub)", got, want)
		}

		wasiExports := rv.runtime.Module("wasi_snapshot_preview1").ExportedFunctionDefinitions()
		if got, want := len(wasiExports), 8; got != want {
			t.Errorf("wasi_snapshot_preview1 exports = %d, want %d (8 WASI shims; UNCHANGED at 25.2)", got, want)
		}
	})

	t.Run("deny-all 25 env + 8 wasi", func(t *testing.T) {
		rv, err := NewRootVM(ctx, mod, 1) // default deny-all
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()

		envExports := rv.runtime.Module("env").ExportedFunctionDefinitions()
		if got, want := len(envExports), 25; got != want {
			t.Errorf("env exports under deny-all = %d, want %d (16 25.1 active + 0 NEW 25.2 + 9 STILL stub)", got, want)
		}

		wasiExports := rv.runtime.Module("wasi_snapshot_preview1").ExportedFunctionDefinitions()
		if got, want := len(wasiExports), 8; got != want {
			t.Errorf("wasi_snapshot_preview1 exports = %d, want %d", got, want)
		}
	})
}
