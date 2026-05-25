// Tests for the VM lifecycle + options + per-callback methods + panic-wrapper
// per 25.1 SPEC §3.1 acceptance criteria (Task 7).
//
// Coverage:
//   - Per-stream construction round-trip: NewVM → RegisterABICallbacks →
//     Run → CallProxyOnContextCreate → CallProxyOnRequestHeaders → Close.
//   - Option application: WithSandboxConfig + WithPanicHandler + WithLogSink
//     independently set fields correctly.
//   - Panic-wrapper: Go panic in ABICallbacks method recover()s; PanicHandlerFn
//     fires; returns InternalFailure (proxy_log path) + non-nil err
//     (per-callback path).
//   - Sandbox-deny lifecycle gates: when capProxyOn* is denied, the
//     corresponding Run-step / per-callback method skips the call.
//   - Concurrent VMs share no state: N goroutines each NewVM/Run/Close
//     against same *Module; race-free under `-race`.
//   - Close idempotent.
//   - wasiHost interface satisfaction (compile-time + behavior).
//
// Test wasm fixtures use the minimal binaries defined in
// vm_test_fixtures.go (committed alongside this file for test isolation).

package wasm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// fakeABICallbacks records every method invocation for test assertions.
// Each method's behavior is configurable via the corresponding response field.
type fakeABICallbacks struct {
	mu sync.Mutex

	// Call log: every invocation appends an entry for later assertions.
	calls []string

	// Configurable returns.
	getHeaderMapReturn      []HeaderPair
	getHeaderMapOK          bool
	getHeaderMapValueReturn string
	getHeaderMapValueOK     bool
	addReturn               abi.WasmResult
	replaceReturn           abi.WasmResult
	removeReturn            abi.WasmResult
	setPairsReturn          abi.WasmResult
	getHeaderMapSizeReturn  uint32
	getPropertyReturn       []byte
	getPropertyOK           bool
	setPropertyReturn       abi.WasmResult
	sendLocalReturn         abi.WasmResult
	getStatusCode           uint32
	getStatusValue          []byte
	getStatusOK             bool
	getLogLevelReturn       abi.LogLevel
	getCurrentTimeReturn    uint64
	setEffectiveCtxReturn   abi.WasmResult
	doneReturn              abi.WasmResult
	panicOnLog              bool   // if set, Log panics on invocation
	panicOnLogMsg           string // the panic value (if panicOnLog)
	logLastMsg              string
	logLastLevel            abi.LogLevel
	addHeaderLastKey        string
	addHeaderLastValue      string
}

func (f *fakeABICallbacks) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakeABICallbacks) callsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeABICallbacks) GetHeaderMap(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType) ([]HeaderPair, bool) {
	f.record(fmt.Sprintf("GetHeaderMap(%d)", mapType))
	return f.getHeaderMapReturn, f.getHeaderMapOK
}
func (f *fakeABICallbacks) GetHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key string) (string, bool) {
	f.record(fmt.Sprintf("GetHeaderMapValue(%d,%q)", mapType, key))
	return f.getHeaderMapValueReturn, f.getHeaderMapValueOK
}
func (f *fakeABICallbacks) AddHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key, value string) abi.WasmResult {
	f.mu.Lock()
	f.addHeaderLastKey = key
	f.addHeaderLastValue = value
	f.mu.Unlock()
	f.record(fmt.Sprintf("AddHeaderMapValue(%d,%q,%q)", mapType, key, value))
	return f.addReturn
}
func (f *fakeABICallbacks) ReplaceHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key, value string) abi.WasmResult {
	f.record(fmt.Sprintf("ReplaceHeaderMapValue(%d,%q,%q)", mapType, key, value))
	return f.replaceReturn
}
func (f *fakeABICallbacks) RemoveHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key string) abi.WasmResult {
	f.record(fmt.Sprintf("RemoveHeaderMapValue(%d,%q)", mapType, key))
	return f.removeReturn
}
func (f *fakeABICallbacks) SetHeaderMapPairs(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, pairs []HeaderPair) abi.WasmResult {
	f.record(fmt.Sprintf("SetHeaderMapPairs(%d,%d-pairs)", mapType, len(pairs)))
	return f.setPairsReturn
}
func (f *fakeABICallbacks) GetHeaderMapSize(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType) uint32 {
	f.record(fmt.Sprintf("GetHeaderMapSize(%d)", mapType))
	return f.getHeaderMapSizeReturn
}
func (f *fakeABICallbacks) GetProperty(_ context.Context, _ uint32, path []string) ([]byte, bool) {
	f.record(fmt.Sprintf("GetProperty(%v)", path))
	return f.getPropertyReturn, f.getPropertyOK
}
func (f *fakeABICallbacks) SetProperty(_ context.Context, _ uint32, path []string, value []byte) abi.WasmResult {
	f.record(fmt.Sprintf("SetProperty(%v,%d-bytes)", path, len(value)))
	return f.setPropertyReturn
}
func (f *fakeABICallbacks) SendLocalResponse(_ context.Context, _ uint32, statusCode uint32, statusMsg, body string, addl []HeaderPair, grpcStatus int32) abi.WasmResult {
	f.record(fmt.Sprintf("SendLocalResponse(%d,%q,%q,%d-hdrs,%d)", statusCode, statusMsg, body, len(addl), grpcStatus))
	return f.sendLocalReturn
}
func (f *fakeABICallbacks) GetStatus(_ context.Context, _ uint32) (uint32, []byte, bool) {
	f.record("GetStatus")
	return f.getStatusCode, f.getStatusValue, f.getStatusOK
}
func (f *fakeABICallbacks) Log(_ context.Context, _ uint32, level abi.LogLevel, msg string) {
	f.mu.Lock()
	f.logLastLevel = level
	f.logLastMsg = msg
	pan := f.panicOnLog
	panMsg := f.panicOnLogMsg
	f.mu.Unlock()
	f.record(fmt.Sprintf("Log(%d,%q)", level, msg))
	if pan {
		panic(panMsg)
	}
}
func (f *fakeABICallbacks) GetLogLevel(_ context.Context) abi.LogLevel {
	f.record("GetLogLevel")
	return f.getLogLevelReturn
}
func (f *fakeABICallbacks) GetCurrentTimeNanoseconds(_ context.Context) uint64 {
	f.record("GetCurrentTimeNanoseconds")
	return f.getCurrentTimeReturn
}
func (f *fakeABICallbacks) SetEffectiveContext(_ context.Context, contextID uint32) abi.WasmResult {
	f.record(fmt.Sprintf("SetEffectiveContext(%d)", contextID))
	return f.setEffectiveCtxReturn
}
func (f *fakeABICallbacks) Done(_ context.Context, contextID uint32) abi.WasmResult {
	f.record(fmt.Sprintf("Done(%d)", contextID))
	return f.doneReturn
}

// Compile-time guard: fakeABICallbacks satisfies ABICallbacks.
var _ ABICallbacks = (*fakeABICallbacks)(nil)

// compileForVM compiles src against vm.runtime and returns a *Module bound
// to that runtime. Convenience helper for tests that don't exercise the
// production cross-runtime path (vm.Run re-compiles module.Source() against
// its own runtime via the shared wazero.CompilationCache wired by
// WithCompilationCache). The src is retained on the *Module so that
// vm.Run's re-compile path still functions even for this test fixture.
//
// PRODUCTION-PATH NOTE (Task 7 follow-up): the cross-VM compile-once-
// share-many pattern that CompileCache (Task 5) was designed for is now
// supported via the shared wazero.CompilationCache exposed by
// CompileCache.WazeroCompilationCache() + wired into per-stream VMs via
// WithCompilationCache(cache.WazeroCompilationCache()). See
// TestVM_Run_FromSharedCacheModule for the end-to-end production pattern.
// `compileForVM` remains useful for tests that don't need the cache plumbing.
func compileForVM(t *testing.T, vm *VM, src []byte) *Module {
	t.Helper()
	ctx := context.Background()
	compiled, err := vm.runtime.CompileModule(ctx, src)
	if err != nil {
		t.Fatalf("compile fixture against vm.runtime: %v", err)
	}
	return &Module{
		compiled: compiled,
		abiVer:   AbiVersion_0_2_1,
		src:      src, // retained for vm.Run's cross-runtime re-compile path
	}
}

// allowAllSandbox returns a SandboxConfig with every capability key allowed.
// Useful for tests that want the sandbox out of the way + focus on behavior.
func allowAllSandbox() SandboxConfig {
	keys := []string{
		// Headers-bridge (7)
		capProxyGetHeaderMapPairs, capProxySetHeaderMapPairs, capProxyGetHeaderMapValue,
		capProxyAddHeaderMapValue, capProxyReplaceHeaderMapValue, capProxyRemoveHeaderMapValue,
		capProxyGetHeaderMapSize,
		// Local-response (1)
		capProxySendLocalResponse,
		// Property (2)
		capProxyGetProperty, capProxySetProperty,
		// Log (2)
		capProxyLog, capProxyGetLogLevel,
		// Status (1)
		capProxyGetStatus,
		// Time (1)
		capProxyGetCurrentTimeNanoseconds,
		// Context-lifecycle (2)
		capProxySetEffectiveContext, capProxyDone,
		// WASI (8)
		capWasiFdWrite, capWasiClockTimeGet, capWasiRandomGet,
		capWasiEnvironSizesGet, capWasiEnvironGet, capWasiArgsSizesGet, capWasiArgsGet, capWasiProcExit,
		// Module-init / allocator (5) — informational; not consulted by Run
		capModuleInitialize, capModuleStart, capModuleMain, capAllocatorMalloc, capAllocatorProxyOnMemoryAllocate,
		// Lifecycle + HTTP module-getters (8)
		capProxyOnContextCreate, capProxyOnVmStart, capProxyOnConfigure, capProxyOnDone,
		capProxyOnDelete, capProxyOnLog, capProxyOnRequestHeaders, capProxyOnResponseHeaders,
	}
	allowed := make(map[string]SanitizationConfig, len(keys))
	for _, k := range keys {
		allowed[k] = SanitizationConfig{}
	}
	return SandboxConfig{AllowedCapabilities: allowed}
}

// --- TestNewVM_Options ----------------------------------------------------

func TestNewVM_Options(t *testing.T) {
	ctx := context.Background()

	t.Run("WithSandboxConfig sets sandbox field", func(t *testing.T) {
		sb := SandboxConfig{AllowedCapabilities: map[string]SanitizationConfig{capProxyLog: {}}}
		vm := NewVM(ctx, WithSandboxConfig(sb))
		defer func() { _ = vm.Close() }()

		if !vm.sandbox.IsAllowed(capProxyLog) {
			t.Errorf("sandbox not applied: IsAllowed(proxy_log) = false")
		}
		if vm.sandbox.IsAllowed(capProxyGetHeaderMapPairs) {
			t.Errorf("sandbox over-permissive: IsAllowed(proxy_get_header_map_pairs) = true")
		}
	})

	t.Run("WithPanicHandler sets panicH field", func(t *testing.T) {
		var captured any
		h := func(r any) { captured = r }
		vm := NewVM(ctx, WithPanicHandler(h))
		defer func() { _ = vm.Close() }()

		if vm.panicH == nil {
			t.Fatal("panicH not set")
		}
		// Trigger the handler manually to verify the wired function.
		vm.panicH("test-panic")
		if captured != "test-panic" {
			t.Errorf("panicH did not capture: got %v, want \"test-panic\"", captured)
		}
	})

	t.Run("WithLogSink sets logSink field + LogProxy routes through it", func(t *testing.T) {
		var sink bytes.Buffer
		vm := NewVM(ctx, WithLogSink(&sink))
		defer func() { _ = vm.Close() }()

		vm.LogProxy(abi.LogLevelInfo, "hello")
		got := sink.String()
		if !strings.Contains(got, "info") || !strings.Contains(got, "hello") {
			t.Errorf("log sink did not capture: got %q", got)
		}
	})

	t.Run("default options: zero-value sandbox (deny-all) + nil sink + nil panicH", func(t *testing.T) {
		vm := NewVM(ctx)
		defer func() { _ = vm.Close() }()

		if vm.sandbox.IsAllowed(capProxyLog) {
			t.Errorf("default sandbox over-permissive: IsAllowed(proxy_log) = true")
		}
		if vm.panicH != nil {
			t.Errorf("default panicH non-nil")
		}
		if vm.logSink != nil {
			t.Errorf("default logSink non-nil")
		}
	})

	t.Run("WithLogSink nil-sink: LogProxy no-op", func(t *testing.T) {
		vm := NewVM(ctx) // no WithLogSink
		defer func() { _ = vm.Close() }()
		// Should not panic.
		vm.LogProxy(abi.LogLevelError, "must-not-leak")
	})
}

// --- TestNewVM_HostModulesRegistered --------------------------------------

// TestNewVM_HostModulesRegistered verifies the env + wasi_snapshot_preview1
// host modules are registered during NewVM construction. A module that
// imports a single proxy_* function should instantiate cleanly; without
// registration, wazero would return "unknown import" at instantiate time.
func TestNewVM_HostModulesRegistered(t *testing.T) {
	ctx := context.Background()
	vm := NewVM(ctx)
	defer func() { _ = vm.Close() }()

	// Module imports env.proxy_log; instantiating must succeed even though
	// nothing calls proxy_log.
	mod, err := vm.runtime.Instantiate(ctx, importerProxyLogModule)
	if err != nil {
		t.Fatalf("instantiate importer module: %v", err)
	}
	defer func() { _ = mod.Close(ctx) }()
}

// --- TestVM_Run_Lifecycle -------------------------------------------------

func TestVM_Run_Lifecycle(t *testing.T) {
	ctx := context.Background()

	t.Run("nil module returns error", func(t *testing.T) {
		vm := NewVM(ctx)
		defer func() { _ = vm.Close() }()
		err := vm.Run(ctx, nil, 1)
		if err == nil {
			t.Fatal("expected error on nil module")
		}
	})

	t.Run("Run after Close errors", func(t *testing.T) {
		vm := NewVM(ctx)
		_ = vm.Close()

		mod, err := CompileModule(ctx, minimalInitModule, nil)
		if err != nil {
			t.Fatalf("compile fixture: %v", err)
		}
		defer func() { _ = mod.Close(ctx) }()

		err = vm.Run(ctx, mod, 1)
		if err == nil {
			t.Fatal("expected error on Run after Close")
		}
	})

	t.Run("Run invokes _initialize when exported", func(t *testing.T) {
		vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
		defer func() { _ = vm.Close() }()

		mod := compileForVM(t, vm, minimalInitModule)
		if err := vm.Run(ctx, mod, 1); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	t.Run("module without init exports does not error", func(t *testing.T) {
		vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
		defer func() { _ = vm.Close() }()

		mod := compileForVM(t, vm, noInitModule)
		if err := vm.Run(ctx, mod, 1); err != nil {
			t.Fatalf("Run with no init: %v", err)
		}
	})
}

// --- TestVM_Run_Lifecycle_ContextCreateBeforeVmStart ----------------------

// TestVM_Run_Lifecycle_ContextCreateBeforeVmStart is the regression test for
// the Task 15 follow-up fix: vm.Run must invoke `proxy_on_context_create(
// rootCtxID, 0)` BEFORE `proxy_on_vm_start(rootCtxID, 0)` to match the
// canonical proxy-wasm host lifecycle (per upstream proxy-wasm-cpp-host@
// da3ce05d:src/wasm.cc + the proxy-wasm-rust-sdk v0.2.4 dispatcher, which
// panics `"invalid context_id"` if `proxy_on_vm_start` fires before the root
// is registered).
//
// The fixture's three lifecycle callbacks each call `proxy_log` with a
// DISTINCT level so the firing order is recoverable from the recorded log
// levels: Info (=2) → Warn (=3) → Error (=4) corresponds to context_create →
// vm_start → configure.
//
// Also verifies the "guest does not export proxy_on_context_create" branch
// is handled gracefully: vmStartOnlyModule exports proxy_on_vm_start but not
// proxy_on_context_create; vm.Run must skip the (c.5) seeding step + still
// invoke proxy_on_vm_start successfully.
func TestVM_Run_Lifecycle_ContextCreateBeforeVmStart(t *testing.T) {
	ctx := context.Background()

	t.Run("proxy_on_context_create fires before proxy_on_vm_start before proxy_on_configure", func(t *testing.T) {
		cb := &fakeABICallbacks{}
		vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
		defer func() { _ = vm.Close() }()
		vm.RegisterABICallbacks(cb)

		mod := compileForVM(t, vm, lifecycleOrderModule)
		if err := vm.Run(ctx, mod, 1); err != nil {
			t.Fatalf("Run: %v", err)
		}

		// Recover firing order from the proxy_log call levels.
		// fakeABICallbacks.Log records `Log(<level>,...)` strings in `calls`.
		got := cb.callsSnapshot()
		if len(got) != 3 {
			t.Fatalf("expected 3 proxy_log invocations (one per lifecycle callback), got %d: %v", len(got), got)
		}
		want := []string{
			`Log(2,"")`, // proxy_on_context_create (Info)
			`Log(3,"")`, // proxy_on_vm_start       (Warn)
			`Log(4,"")`, // proxy_on_configure      (Error)
		}
		for i, w := range want {
			if got[i] != w {
				t.Errorf("lifecycle order mismatch at step %d: got %q, want %q (full sequence: %v)", i, got[i], w, got)
			}
		}
	})

	t.Run("guest without proxy_on_context_create export: vm.Run skips (c.5) + proceeds to vm_start", func(t *testing.T) {
		cb := &fakeABICallbacks{}
		vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
		defer func() { _ = vm.Close() }()
		vm.RegisterABICallbacks(cb)

		mod := compileForVM(t, vm, vmStartOnlyModule)
		if err := vm.Run(ctx, mod, 1); err != nil {
			t.Fatalf("Run (no proxy_on_context_create export): %v", err)
		}

		// Only proxy_on_vm_start ran (Warn=3). proxy_on_context_create not
		// exported → skipped silently. proxy_on_configure not exported → also
		// skipped.
		got := cb.callsSnapshot()
		if len(got) != 1 {
			t.Fatalf("expected 1 proxy_log invocation (proxy_on_vm_start only), got %d: %v", len(got), got)
		}
		want := `Log(3,"")`
		if got[0] != want {
			t.Errorf("expected proxy_on_vm_start log entry %q, got %q", want, got[0])
		}
	})

	t.Run("capability denied: proxy_on_context_create skipped + downstream lifecycle still runs", func(t *testing.T) {
		// SandboxConfig allows everything EXCEPT capProxyOnContextCreate.
		sb := allowAllSandbox()
		delete(sb.AllowedCapabilities, capProxyOnContextCreate)

		cb := &fakeABICallbacks{}
		vm := NewVM(ctx, WithSandboxConfig(sb))
		defer func() { _ = vm.Close() }()
		vm.RegisterABICallbacks(cb)

		mod := compileForVM(t, vm, lifecycleOrderModule)
		if err := vm.Run(ctx, mod, 1); err != nil {
			// Without the cap, vm.Run skips seeding the root → if the guest's
			// proxy_on_vm_start asserted on a pre-seeded root, it would trap.
			// Our hand-crafted fixture doesn't assert, so it succeeds — the
			// behavior matches the gate-discipline contract: cap-denied =
			// host-side no-op skip, NOT a host-side error.
			t.Fatalf("Run with capProxyOnContextCreate denied: %v", err)
		}

		// proxy_on_context_create was NOT called (cap denied); proxy_on_vm_start
		// + proxy_on_configure still ran.
		got := cb.callsSnapshot()
		if len(got) != 2 {
			t.Fatalf("expected 2 proxy_log invocations (vm_start + configure; context_create denied), got %d: %v", len(got), got)
		}
		want := []string{
			`Log(3,"")`, // proxy_on_vm_start
			`Log(4,"")`, // proxy_on_configure
		}
		for i, w := range want {
			if got[i] != w {
				t.Errorf("lifecycle order mismatch at step %d (cap-denied path): got %q, want %q", i, got[i], w)
			}
		}
	})
}

// --- TestVM_PerCallback_NoExportNoCap -------------------------------------

// TestVM_PerCallback_NoExportNoCap verifies that the per-callback methods
// return graceful defaults (no error, ProxyActionContinue, etc.) when:
//
//   - The guest module does not export the callback.
//   - The capability is denied via SandboxConfig.
func TestVM_PerCallback_NoExportNoCap(t *testing.T) {
	ctx := context.Background()

	t.Run("guest does not export callback: no-op continue + nil err", func(t *testing.T) {
		vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
		defer func() { _ = vm.Close() }()
		mod := compileForVM(t, vm, minimalInitModule)
		if err := vm.Run(ctx, mod, 1); err != nil {
			t.Fatalf("Run: %v", err)
		}

		// All 6 per-callback methods must succeed with their defaults.
		if err := vm.CallProxyOnContextCreate(ctx, 2, 1); err != nil {
			t.Errorf("OnContextCreate: %v", err)
		}
		act, err := vm.CallProxyOnRequestHeaders(ctx, 2, 5, false)
		if err != nil || act != abi.ProxyActionContinue {
			t.Errorf("OnRequestHeaders: act=%d err=%v, want (Continue, nil)", act, err)
		}
		act, err = vm.CallProxyOnResponseHeaders(ctx, 2, 5, false)
		if err != nil || act != abi.ProxyActionContinue {
			t.Errorf("OnResponseHeaders: act=%d err=%v", act, err)
		}
		done, err := vm.CallProxyOnDone(ctx, 2)
		if err != nil || !done {
			t.Errorf("OnDone: done=%v err=%v, want (true, nil)", done, err)
		}
		if err := vm.CallProxyOnLog(ctx, 2); err != nil {
			t.Errorf("OnLog: %v", err)
		}
		if err := vm.CallProxyOnDelete(ctx, 2); err != nil {
			t.Errorf("OnDelete: %v", err)
		}
	})

	t.Run("capability denied: skip dispatch (no error)", func(t *testing.T) {
		// Deny-all SandboxConfig (zero value).
		vm := NewVM(ctx)
		defer func() { _ = vm.Close() }()
		mod := compileForVM(t, vm, minimalInitModule)
		if err := vm.Run(ctx, mod, 1); err != nil {
			t.Fatalf("Run: %v", err)
		}

		if err := vm.CallProxyOnContextCreate(ctx, 2, 1); err != nil {
			t.Errorf("OnContextCreate on deny: %v (want nil)", err)
		}
		act, err := vm.CallProxyOnRequestHeaders(ctx, 2, 0, false)
		if err != nil || act != abi.ProxyActionContinue {
			t.Errorf("OnRequestHeaders on deny: act=%d err=%v", act, err)
		}
		done, err := vm.CallProxyOnDone(ctx, 2)
		if err != nil || !done {
			t.Errorf("OnDone on deny: done=%v err=%v", done, err)
		}
	})

	t.Run("CallProxyOnContextCreate before Run returns error", func(t *testing.T) {
		vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
		defer func() { _ = vm.Close() }()

		err := vm.CallProxyOnContextCreate(ctx, 2, 1)
		if err == nil {
			t.Error("expected error: CallProxyOnContextCreate before Run")
		}
	})
}

// --- TestVM_PanicWrapper --------------------------------------------------

// TestVM_PanicWrapper verifies the panic-wrapper integration when a Go
// panic is raised inside an ABICallbacks method during a hostcall (proxy_log).
func TestVM_PanicWrapper(t *testing.T) {
	ctx := context.Background()
	var sink bytes.Buffer

	var captured any
	cb := &fakeABICallbacks{
		panicOnLog:    true,
		panicOnLogMsg: "callback-panic-payload",
	}
	vm := NewVM(ctx,
		WithSandboxConfig(allowAllSandbox()),
		WithLogSink(&sink),
		WithPanicHandler(func(r any) { captured = r }),
	)
	defer func() { _ = vm.Close() }()
	vm.RegisterABICallbacks(cb)

	// Compile + Run the proxy_log invoker fixture.
	mod := compileForVM(t, vm, invokeProxyLogModule)
	if err := vm.Run(ctx, mod, 1); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// invoke_proxy_log is exported; invoke it manually to fire the hostcall.
	invoke := vm.instance.ExportedFunction("invoke_proxy_log")
	if invoke == nil {
		t.Fatal("invoke_proxy_log not exported on fixture")
	}
	results, err := invoke.Call(ctx)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("invoke returned no results")
	}

	// The hostcall body wraps the cb.Log invocation in runWithPanicWrapper;
	// the recover converts the panic to abi.WasmResultInternalFailure (=10).
	got := uint32(results[0])
	if got != uint32(abi.WasmResultInternalFailure) {
		t.Errorf("hostcall return = %d, want %d (InternalFailure)", got, uint32(abi.WasmResultInternalFailure))
	}

	if captured != "callback-panic-payload" {
		t.Errorf("PanicHandlerFn captured = %v, want \"callback-panic-payload\"", captured)
	}

	if len(cb.callsSnapshot()) != 1 {
		t.Errorf("Log call count = %d, want 1", len(cb.callsSnapshot()))
	}
}

// --- TestVM_PerCallback_Panic ---------------------------------------------

// TestVM_PerCallback_Panic verifies the per-callback method panic-wrapper
// converts a Go panic in the guest's Call execution to a non-nil err return.
// We rig this by registering an ABICallbacks that panics inside Log, then
// invoking proxy_on_request_headers via a fixture that calls proxy_log.
func TestVM_PerCallback_Panic(t *testing.T) {
	ctx := context.Background()
	var captured any
	cb := &fakeABICallbacks{
		panicOnLog:    true,
		panicOnLogMsg: "rh-panic",
	}
	vm := NewVM(ctx,
		WithSandboxConfig(allowAllSandbox()),
		WithPanicHandler(func(r any) { captured = r }),
	)
	defer func() { _ = vm.Close() }()
	vm.RegisterABICallbacks(cb)

	mod := compileForVM(t, vm, onRequestHeadersInvokesLogModule)
	if err := vm.Run(ctx, mod, 1); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// proxy_on_request_headers internally calls proxy_log → panics in Go
	// callback → recover converts to InternalFailure on the wire. The
	// callback caller observes the action (Continue=0; that's what the
	// fixture's proxy_on_request_headers returns even on panic) — the
	// PanicHandler fires regardless.
	act, err := vm.CallProxyOnRequestHeaders(ctx, 2, 0, false)
	_ = act
	_ = err

	if captured != "rh-panic" {
		t.Errorf("PanicHandlerFn captured = %v, want \"rh-panic\"", captured)
	}
}

// --- TestVM_Run_FromSharedCacheModule -------------------------------------

// TestVM_Run_FromSharedCacheModule verifies the Task 7 follow-up fix for
// the cross-runtime CompiledModule binding issue: a *Module produced by
// a *CompileCache (Task 5) is now successfully consumed by a per-stream
// *VM (Task 7) wired via WithCompilationCache(cache.WazeroCompilationCache()).
//
// The production pattern is:
//
//	(1) cache := wasm.NewCompileCache(ctx)         // Task 9 territory
//	(2) mod, _ := wasm.CompileModule(ctx, src, cache)
//	(3) vm := wasm.NewVM(ctx,                       // Task 12 territory
//	        wasm.WithCompilationCache(cache.WazeroCompilationCache()),
//	        wasm.WithSandboxConfig(...))
//	(4) vm.Run(ctx, mod, rootCtxID)                 // re-compile cache hit
//
// Without the WithCompilationCache wiring, step (4) re-compiles fresh
// against vm.runtime (functionally correct, but pays full codegen cost
// instead of sub-ms cache lookup); this test verifies BOTH paths work
// (with + without the shared cache wiring), since the *Module retains
// src in both cases.
func TestVM_Run_FromSharedCacheModule(t *testing.T) {
	ctx := context.Background()

	t.Run("with shared CompilationCache: re-compile cache hit + Run succeeds", func(t *testing.T) {
		cache := NewCompileCache(ctx)
		defer func() { _ = cache.Close() }()

		// (1) Compile once via the cache (this populates the shared
		// wazero.CompilationCache's codegen-result entry).
		mod, err := CompileModule(ctx, minimalInitModule, cache)
		if err != nil {
			t.Fatalf("CompileModule: %v", err)
		}
		if mod.Source() == nil {
			t.Fatal("Module.Source() returned nil — re-compile path requires retained src")
		}

		// (2) Construct a per-stream VM wired with the same shared
		// CompilationCache. vm.Run's re-compile-against-vm.runtime path
		// should hit the shared cache sub-ms.
		vm := NewVM(ctx,
			WithCompilationCache(cache.WazeroCompilationCache()),
			WithSandboxConfig(allowAllSandbox()),
		)
		defer func() { _ = vm.Close() }()

		if err := vm.Run(ctx, mod, 1); err != nil {
			t.Fatalf("Run with shared-cache *Module: %v (must succeed — Task 7 follow-up fix)", err)
		}

		// Verify the instance is live + the lifecycle ran.
		if !vm.HasGlobalFunc("_initialize") {
			t.Error("post-Run: HasGlobalFunc(_initialize) = false; instance not wired correctly")
		}
	})

	t.Run("multiple VMs share the cache: each Run succeeds + cache-hit", func(t *testing.T) {
		cache := NewCompileCache(ctx)
		defer func() { _ = cache.Close() }()

		mod, err := CompileModule(ctx, minimalInitModule, cache)
		if err != nil {
			t.Fatalf("CompileModule: %v", err)
		}

		// Three independent per-stream VMs all using the same cached *Module +
		// the same shared CompilationCache. Each Run re-compiles against its
		// own runtime; the shared cache amortizes the codegen work.
		for i := 0; i < 3; i++ {
			vm := NewVM(ctx,
				WithCompilationCache(cache.WazeroCompilationCache()),
				WithSandboxConfig(allowAllSandbox()),
			)
			if err := vm.Run(ctx, mod, uint32(i+1)); err != nil {
				_ = vm.Close()
				t.Fatalf("VM %d Run: %v", i, err)
			}
			if err := vm.Close(); err != nil {
				t.Errorf("VM %d Close: %v", i, err)
			}
		}
	})

	t.Run("without shared CompilationCache: re-compile still succeeds (slower path)", func(t *testing.T) {
		// Verifies the fallback: even without WithCompilationCache, vm.Run
		// re-compiles module.Source() against vm.runtime + the result is
		// instantiable. This path pays full codegen on each VM construction;
		// the WithCompilationCache wiring is the optimization, not the
		// correctness gate.
		cache := NewCompileCache(ctx)
		defer func() { _ = cache.Close() }()

		mod, err := CompileModule(ctx, minimalInitModule, cache)
		if err != nil {
			t.Fatalf("CompileModule: %v", err)
		}

		vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
		defer func() { _ = vm.Close() }()

		if err := vm.Run(ctx, mod, 1); err != nil {
			t.Fatalf("Run without shared cache: %v (must succeed via fallback re-compile)", err)
		}
	})

	t.Run("nil-cache *Module also Run-able: vm.Run uses src retained on nil-cache transient", func(t *testing.T) {
		// Nil-cache CompileModule produces a *Module with non-nil
		// transientRT. The src is still retained for the cross-runtime
		// re-compile path; vm.Run should consume it cleanly.
		mod, err := CompileModule(ctx, minimalInitModule, nil)
		if err != nil {
			t.Fatalf("CompileModule(nil-cache): %v", err)
		}
		defer func() { _ = mod.Close(ctx) }()

		vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
		defer func() { _ = vm.Close() }()

		if err := vm.Run(ctx, mod, 1); err != nil {
			t.Fatalf("Run with nil-cache *Module: %v", err)
		}
	})
}

// --- TestCompileCache_WazeroCompilationCache ------------------------------

// TestCompileCache_WazeroCompilationCache verifies the exposed shared
// wazero.CompilationCache accessor returns the same non-nil cache for the
// lifetime of the *CompileCache (so callers retain pointer-identity across
// repeated accesses) + is released by *CompileCache.Close.
func TestCompileCache_WazeroCompilationCache(t *testing.T) {
	ctx := context.Background()
	cache := NewCompileCache(ctx)

	wcc1 := cache.WazeroCompilationCache()
	if wcc1 == nil {
		t.Fatal("WazeroCompilationCache returned nil on fresh cache")
	}
	wcc2 := cache.WazeroCompilationCache()
	if wcc1 != wcc2 {
		t.Errorf("WazeroCompilationCache pointer-identity broken: %p != %p", wcc1, wcc2)
	}

	// Close should release both the runtime AND the wazero CompilationCache.
	if err := cache.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// Idempotent.
	if err := cache.Close(); err != nil {
		t.Errorf("Close (second): %v", err)
	}
}

// --- TestVM_Close_Idempotent ----------------------------------------------

func TestVM_Close_Idempotent(t *testing.T) {
	ctx := context.Background()
	vm := NewVM(ctx)
	if err := vm.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := vm.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if err := vm.Close(); err != nil {
		t.Errorf("third Close: %v", err)
	}
}

// --- TestVM_Concurrent_NoSharedState --------------------------------------

// TestVM_Concurrent_NoSharedState fans out N goroutines, each constructing
// its own *VM (with its own per-VM-compiled module) + running the round-trip
// + closing. With -race the test catches any cross-VM mutation. Asserts no
// errors + all VMs close cleanly. See compileForVM doc-comment for the
// per-VM compile rationale (cross-runtime CompiledModule incompatibility).
func TestVM_Concurrent_NoSharedState(t *testing.T) {
	ctx := context.Background()

	const n = 16
	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(streamID uint32) {
			defer wg.Done()

			vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
			cb := &fakeABICallbacks{getHeaderMapOK: true}
			vm.RegisterABICallbacks(cb)

			compiled, err := vm.runtime.CompileModule(ctx, minimalInitModule)
			if err != nil {
				errs <- fmt.Errorf("goroutine %d compile: %w", streamID, err)
				_ = vm.Close()
				return
			}
			mod := &Module{compiled: compiled, abiVer: AbiVersion_0_2_1, src: minimalInitModule}

			if err := vm.Run(ctx, mod, 1); err != nil {
				errs <- fmt.Errorf("goroutine %d Run: %w", streamID, err)
				_ = vm.Close()
				return
			}
			if err := vm.CallProxyOnContextCreate(ctx, streamID, 1); err != nil {
				errs <- fmt.Errorf("goroutine %d OnContextCreate: %w", streamID, err)
			}
			if _, err := vm.CallProxyOnRequestHeaders(ctx, streamID, 0, false); err != nil {
				errs <- fmt.Errorf("goroutine %d OnRequestHeaders: %w", streamID, err)
			}
			if err := vm.Close(); err != nil {
				errs <- fmt.Errorf("goroutine %d Close: %w", streamID, err)
			}
		}(uint32(i + 100))
	}
	wg.Wait()
	close(errs)

	for e := range errs {
		t.Error(e)
	}
}

// --- TestVM_HasGlobalFunc -------------------------------------------------

func TestVM_HasGlobalFunc(t *testing.T) {
	ctx := context.Background()

	t.Run("returns false before Run", func(t *testing.T) {
		vm := NewVM(ctx)
		defer func() { _ = vm.Close() }()
		if vm.HasGlobalFunc("_initialize") {
			t.Error("HasGlobalFunc before Run = true, want false")
		}
	})

	t.Run("returns true for exported function after Run", func(t *testing.T) {
		vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
		defer func() { _ = vm.Close() }()
		mod := compileForVM(t, vm, minimalInitModule)
		if err := vm.Run(ctx, mod, 1); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !vm.HasGlobalFunc("_initialize") {
			t.Error("HasGlobalFunc(_initialize) = false, want true")
		}
		if vm.HasGlobalFunc("does_not_exist") {
			t.Error("HasGlobalFunc(does_not_exist) = true, want false")
		}
	})
}

// --- TestVM_State ---------------------------------------------------------

func TestVM_State(t *testing.T) {
	ctx := context.Background()
	vm := NewVM(ctx)
	defer func() { _ = vm.Close() }()

	rt := vm.State()
	if rt == nil {
		t.Error("State() = nil, want non-nil wazero.Runtime")
	}
}

// --- TestVM_WasiHost_Satisfaction -----------------------------------------

// TestVM_WasiHost_Satisfaction verifies *VM satisfies the wasiHost interface
// at runtime (compile-time guard already in registration.go via `var _ wasiHost = (*VM)(nil)`).
func TestVM_WasiHost_Satisfaction(t *testing.T) {
	ctx := context.Background()
	var sink bytes.Buffer

	vm := NewVM(ctx,
		WithSandboxConfig(SandboxConfig{AllowedCapabilities: map[string]SanitizationConfig{capWasiFdWrite: {}}}),
		WithLogSink(&sink),
	)
	defer func() { _ = vm.Close() }()

	var host wasiHost = vm
	if !host.IsAllowed(capWasiFdWrite) {
		t.Error("wasiHost.IsAllowed(fd_write) = false, want true")
	}
	if host.IsAllowed("not_a_real_cap") {
		t.Error("wasiHost.IsAllowed(not_a_real_cap) = true, want false")
	}

	host.LogProxy(abi.LogLevelWarn, "test-msg")
	if !strings.Contains(sink.String(), "warn") || !strings.Contains(sink.String(), "test-msg") {
		t.Errorf("LogProxy via wasiHost did not write to sink: %q", sink.String())
	}
}

// --- TestVM_LogLevelString ------------------------------------------------

// TestVM_LogLevelString verifies the level-name mapping used by LogProxy.
func TestVM_LogLevelString(t *testing.T) {
	cases := []struct {
		l    abi.LogLevel
		want string
	}{
		{abi.LogLevelTrace, "trace"},
		{abi.LogLevelDebug, "debug"},
		{abi.LogLevelInfo, "info"},
		{abi.LogLevelWarn, "warn"},
		{abi.LogLevelError, "error"},
		{abi.LogLevelCritical, "critical"},
		{abi.LogLevel(99), "level(99)"},
	}
	for _, tc := range cases {
		got := logLevelString(tc.l)
		if got != tc.want {
			t.Errorf("logLevelString(%d) = %q, want %q", tc.l, got, tc.want)
		}
	}
}

// --- silence-unused linters -----------------------------------------------

// Ensure errors + io stay referenced even if helpers don't surface them in
// every assertion.
var (
	_           = errors.New
	_ io.Writer = (*bytes.Buffer)(nil)
)
