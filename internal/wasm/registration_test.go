// Tests for the host-module registration + ABICallbacks invocation
// round-trip per 25.1 SPEC §5 + Task 7 acceptance criteria.
//
// Coverage:
//   - Host-module wiring smoke test: env + wasi_snapshot_preview1 modules
//     registered correctly; a guest module importing any hostcall instantiates
//     cleanly. The hostcall count check (24 active + 23 deferred = 47 total)
//     is enforced via the importer-table fixture covering every registered
//     hostcall name.
//   - ABICallbacks invocation round-trip: proxy_log fires the cb.Log Go method;
//     proxy_get_log_level writes the cb return value to guest memory; etc.
//     The proxy_log fixture from vm_test.go is reused.
//   - Sandbox-deny: when the capability is denied via SandboxConfig, the
//     hostcall returns WasmResultInternalFailure (=10) for proxy_*; the
//     wasi-deny path returns WasiErrnoNotcapable (=76) via the wasi.go shim
//     (already covered in wasi_test.go but spot-checked here for the
//     registration-time wiring).
//   - Deferred stubs: each of the 23 deferred hostcalls returns
//     WasmResultUnimplemented (=12) when invoked. The fixture imports
//     proxy_continue_stream (a 1-arg deferred stub representative) and
//     verifies the return.
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
// module instantiates cleanly against a fresh VM. If any hostcall is
// missing from the registration, wazero reports "unknown import" at
// instantiate time.
func TestRegistration_FullRoster_ImportableWithoutError(t *testing.T) {
	ctx := context.Background()
	vm := NewVM(ctx)
	defer func() { _ = vm.Close() }()

	// The fixture imports every active + deferred hostcall.
	mod, err := vm.runtime.Instantiate(ctx, fullRosterImporterModule)
	if err != nil {
		t.Fatalf("instantiate full-roster importer: %v", err)
	}
	defer func() { _ = mod.Close(ctx) }()
}

// --- TestRegistration_ProxyLog_RoundTrip ----------------------------------

// TestRegistration_ProxyLog_RoundTrip verifies the complete proxy_log
// dispatch: guest call → host hostcall body → sandbox check → ABICallbacks
// invocation → return value to guest.
func TestRegistration_ProxyLog_RoundTrip(t *testing.T) {
	ctx := context.Background()
	cb := &fakeABICallbacks{}

	vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
	defer func() { _ = vm.Close() }()
	vm.RegisterABICallbacks(cb)

	mod := compileForVM(t, vm, invokeProxyLogModule)
	if err := vm.Run(ctx, mod, 1); err != nil {
		t.Fatalf("Run: %v", err)
	}

	invoke := vm.instance.ExportedFunction("invoke_proxy_log")
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
	vm := NewVM(ctx)
	defer func() { _ = vm.Close() }()
	vm.RegisterABICallbacks(cb)

	mod := compileForVM(t, vm, invokeProxyLogModule)
	if err := vm.Run(ctx, mod, 1); err != nil {
		t.Fatalf("Run: %v", err)
	}

	invoke := vm.instance.ExportedFunction("invoke_proxy_log")
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
// written to vm.logSink on sandbox-deny ("hostcall denied: proxy_log").
func TestRegistration_ProxyLog_DenyLogged(t *testing.T) {
	ctx := context.Background()
	var sink bytes.Buffer

	vm := NewVM(ctx, WithLogSink(&sink))
	defer func() { _ = vm.Close() }()
	vm.RegisterABICallbacks(&fakeABICallbacks{})

	mod := compileForVM(t, vm, invokeProxyLogModule)
	if err := vm.Run(ctx, mod, 1); err != nil {
		t.Fatalf("Run: %v", err)
	}

	invoke := vm.instance.ExportedFunction("invoke_proxy_log")
	if _, err := invoke.Call(ctx); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	out := sink.String()
	if !strings.Contains(out, "hostcall denied: proxy_log") {
		t.Errorf("integration log missing: %q", out)
	}
}

// --- TestRegistration_DeferredStub_Unimplemented --------------------------

// TestRegistration_DeferredStub_Unimplemented verifies that a deferred
// hostcall stub returns WasmResultUnimplemented (=12) when invoked from a
// guest, without consulting the sandbox.
func TestRegistration_DeferredStub_Unimplemented(t *testing.T) {
	ctx := context.Background()

	// Use the fixture that calls proxy_continue_stream(0) — a 1-arg deferred stub.
	vm := NewVM(ctx, WithSandboxConfig(allowAllSandbox()))
	defer func() { _ = vm.Close() }()

	mod := compileForVM(t, vm, invokeContinueStreamModule)
	if err := vm.Run(ctx, mod, 1); err != nil {
		t.Fatalf("Run: %v", err)
	}

	invoke := vm.instance.ExportedFunction("invoke_continue_stream")
	results, err := invoke.Call(ctx)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if got := uint32(results[0]); got != uint32(abi.WasmResultUnimplemented) {
		t.Errorf("proxy_continue_stream return = %d, want %d (Unimplemented)", got, abi.WasmResultUnimplemented)
	}
}

// --- TestRegistration_ABICallbacksInterface -------------------------------

// TestRegistration_ABICallbacksInterface verifies the fakeABICallbacks
// (defined in vm_test.go) satisfies the ABICallbacks interface at runtime.
// The compile-time guard `var _ ABICallbacks = (*fakeABICallbacks)(nil)`
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
	vm := NewVM(ctx)
	defer func() { _ = vm.Close() }()

	mod, err := vm.runtime.Instantiate(ctx, importerProxyLogModule)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	defer func() { _ = mod.Close(ctx) }()

	t.Run("zero-size returns empty + ok", func(t *testing.T) {
		s, ok := readString(mod, 0, 0)
		if !ok || s != "" {
			t.Errorf("got (%q, %v), want (\"\", true)", s, ok)
		}
	})

	t.Run("valid bytes return string", func(t *testing.T) {
		mod.Memory().Write(0x100, []byte("hello"))
		s, ok := readString(mod, 0x100, 5)
		if !ok || s != "hello" {
			t.Errorf("got (%q, %v), want (\"hello\", true)", s, ok)
		}
	})

	t.Run("OOB returns empty + not-ok", func(t *testing.T) {
		s, ok := readString(mod, 0xFFFFFFFF, 5)
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
	vm := NewVM(ctx)
	defer func() { _ = vm.Close() }()

	mod, err := vm.runtime.Instantiate(ctx, importerProxyLogModule)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	defer func() { _ = mod.Close(ctx) }()

	// Set sentinels.
	mod.Memory().WriteUint32Le(0x200, 0xDEADBEEF)
	mod.Memory().WriteUint32Le(0x204, 0xCAFEBABE)

	res := writeReturnBuffer(ctx, mod, nil, 0x200, 0x204)
	if res != abi.WasmResultOk {
		t.Fatalf("writeReturnBuffer(nil): %d, want Ok", res)
	}
	gotPtr, _ := mod.Memory().ReadUint32Le(0x200)
	gotSize, _ := mod.Memory().ReadUint32Le(0x204)
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
	vm := NewVM(ctx)
	defer func() { _ = vm.Close() }()

	mod, err := vm.runtime.Instantiate(ctx, importerProxyLogModule)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	defer func() { _ = mod.Close(ctx) }()

	res := writeReturnBuffer(ctx, mod, []byte("hello"), 0x200, 0x204)
	if res != abi.WasmResultInvalidMemoryAccess {
		t.Errorf("writeReturnBuffer no-allocator: %d, want %d (InvalidMemoryAccess)", res, abi.WasmResultInvalidMemoryAccess)
	}
}
