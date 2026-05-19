package lua

// vm_test.go — Task 5 full IMPL behavioral tests per 22.1 PLAN Task 5 +
// 22.1 SPEC §3.1 + §3.4. The compile-time signature pins from Task 1
// are preserved at the top of the file; Task 5 adds behavioral coverage
// for NewVM construction + State / RegisterGlobalFunc / Run /
// HasGlobalFunc / CallGlobal / Close + the panic-wrapper Go-panic
// recovery contract.
//
// Race + concurrency tests are DEFERRED to Task 12 per 22.1 PLAN Step
// 12 + 22.1 SPEC §6 Task 12 (`-race` + N=100 goroutines coverage lands
// there).

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// Compile-time assertions pin the public API surface declared in vm.go
// per 22.1 SPEC §3.1. The signatures captured here become a build-break
// the moment any of them drifts.
var (
	_ func(...VMOption) *VM                  = NewVM
	_ func(SandboxConfig) VMOption           = WithSandboxConfig
	_ func(PanicHandlerFn) VMOption          = WithPanicHandler
	_ func(io.Writer) VMOption               = WithBasePrintSink
	_ func(*VM) *lua.LState                  = (*VM).State
	_ func(*VM, string, lua.LGFunction)      = (*VM).RegisterGlobalFunc
	_ func(*VM, *Chunk) error                = (*VM).Run
	_ func(*VM, string) bool                 = (*VM).HasGlobalFunc
	_ func(*VM, string, ...lua.LValue) error = (*VM).CallGlobal
	_ func(*VM)                              = (*VM).Close
	_ PanicHandlerFn                         = func(any) {}
)

// TestNewVM_NotPanic verifies NewVM + Close construct + tear down a
// non-nil VM without panicking. Pinned at Task 1 skeleton; Task 5
// preserves the contract.
func TestNewVM_NotPanic(t *testing.T) {
	vm := NewVM()
	if vm == nil {
		t.Fatal("NewVM returned nil; expected non-nil per SPEC §3.1")
	}
	vm.Close() // idempotent
}

// TestNewVM_ConstructsState_Defaults verifies NewVM produces a usable
// underlying *lua.LState (Task 1 skeleton returned nil from State();
// Task 5 IMPL wires lua.NewState).
func TestNewVM_ConstructsState_Defaults(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	if vm.State() == nil {
		t.Fatal("vm.State() == nil after NewVM; Task 5 IMPL must wire lua.NewState")
	}
}

// TestNewVM_WithSandboxConfig_AllowIO verifies the WithSandboxConfig
// option flips the io stdlib ALLOW arm; post-construction the `io`
// global is a *lua.LTable (lua.OpenIo was called).
func TestNewVM_WithSandboxConfig_AllowIO(t *testing.T) {
	vm := NewVM(WithSandboxConfig(SandboxConfig{AllowIO: true}))
	defer vm.Close()
	ioGlobal := vm.State().GetGlobal("io")
	if ioGlobal.Type() != lua.LTTable {
		t.Fatalf("io global type = %v; want LTTable (lua.OpenIo should have run under AllowIO=true)", ioGlobal.Type())
	}
}

// TestNewVM_WithBasePrintSink_Captures verifies print(...) calls in
// Lua scripts route to the configured sink (default-nil = drop;
// non-nil = write the script's print output to the sink).
func TestNewVM_WithBasePrintSink_Captures(t *testing.T) {
	var buf bytes.Buffer
	vm := NewVM(WithBasePrintSink(&buf))
	defer vm.Close()
	chunk, err := CompileScript([]byte(`print("hello")`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}
	got := buf.String()
	if !strings.Contains(got, "hello") {
		t.Fatalf("BasePrintSink captured = %q; want contains %q", got, "hello")
	}
}

// TestNewVM_DefaultPrintSink_Drops verifies the default nil-sink behavior
// drops Lua print(...) output silently (no panic + no stdout leak).
func TestNewVM_DefaultPrintSink_Drops(t *testing.T) {
	vm := NewVM() // no WithBasePrintSink
	defer vm.Close()
	chunk, err := CompileScript([]byte(`print("dropped")`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	// Must not panic; must not error.
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil under default nil-sink", err)
	}
}

// TestVM_RegisterGlobalFunc verifies a registered Go callback is
// invocable from Lua via the named global. Task 1 skeleton was a no-op;
// Task 5 IMPL must wire vm.state.SetGlobal(name, vm.state.NewFunction(fn)).
func TestVM_RegisterGlobalFunc(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	called := false
	vm.RegisterGlobalFunc("go_callback", func(L *lua.LState) int {
		called = true
		return 0
	})
	chunk, err := CompileScript([]byte(`go_callback()`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}
	if !called {
		t.Fatal("registered Go callback was NOT invoked from Lua; want invoked")
	}
}

// TestVM_Run_ValidScript verifies Run succeeds for a syntactically-
// valid script with no runtime errors.
func TestVM_Run_ValidScript(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	chunk, err := CompileScript([]byte(`x = 42`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil for valid script", err)
	}
}

// TestVM_Run_LuaRuntimeError verifies a Lua-side runtime `error("...")`
// at top-level surfaces as a non-nil error containing the error message.
func TestVM_Run_LuaRuntimeError(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	chunk, err := CompileScript([]byte(`error("boom")`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	runErr := vm.Run(chunk)
	if runErr == nil {
		t.Fatal("vm.Run err = nil; want non-nil for script that calls error('boom')")
	}
	if !strings.Contains(runErr.Error(), "boom") {
		t.Fatalf("vm.Run err = %q; want substring %q", runErr.Error(), "boom")
	}
}

// TestVM_HasGlobalFunc_Defined verifies HasGlobalFunc returns true for
// a global defined by the script as a function value.
func TestVM_HasGlobalFunc_Defined(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	chunk, err := CompileScript([]byte(`function foo() end`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}
	if !vm.HasGlobalFunc("foo") {
		t.Fatal("HasGlobalFunc(\"foo\") = false; want true after `function foo() end`")
	}
}

// TestVM_HasGlobalFunc_Undefined verifies HasGlobalFunc returns false
// for a name that was never defined.
func TestVM_HasGlobalFunc_Undefined(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	if vm.HasGlobalFunc("nonexistent") {
		t.Fatal("HasGlobalFunc(\"nonexistent\") = true; want false")
	}
}

// TestVM_HasGlobalFunc_NotFunction verifies HasGlobalFunc returns false
// for a global that exists but is NOT a function (e.g., a number).
func TestVM_HasGlobalFunc_NotFunction(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	chunk, err := CompileScript([]byte(`x = 42`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}
	if vm.HasGlobalFunc("x") {
		t.Fatal("HasGlobalFunc(\"x\") = true; want false (x is a number, not a function)")
	}
}

// TestVM_CallGlobal_Happy verifies CallGlobal invokes a previously-
// defined global function. Side-effect (assignment to a sentinel global)
// is observable post-call.
func TestVM_CallGlobal_Happy(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	chunk, err := CompileScript([]byte(`called = false; function foo() called = true end`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}
	if err := vm.CallGlobal("foo"); err != nil {
		t.Fatalf("vm.CallGlobal err = %v; want nil", err)
	}
	v := vm.State().GetGlobal("called")
	if v != lua.LBool(true) {
		t.Fatalf("called global = %v; want true after CallGlobal(\"foo\")", v)
	}
}

// TestVM_CallGlobal_LuaError verifies a Lua-side error inside the
// called function surfaces as a non-nil error containing the message.
func TestVM_CallGlobal_LuaError(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	chunk, err := CompileScript([]byte(`function foo() error("bar") end`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}
	callErr := vm.CallGlobal("foo")
	if callErr == nil {
		t.Fatal("vm.CallGlobal err = nil; want non-nil for function that calls error('bar')")
	}
	if !strings.Contains(callErr.Error(), "bar") {
		t.Fatalf("vm.CallGlobal err = %q; want substring %q", callErr.Error(), "bar")
	}
}

// TestVM_CallGlobal_NotFunction verifies attempting to CallGlobal a
// non-function global returns an error (NOT a Go panic).
func TestVM_CallGlobal_NotFunction(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	chunk, err := CompileScript([]byte(`x = 42`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}
	callErr := vm.CallGlobal("x")
	if callErr == nil {
		t.Fatal("vm.CallGlobal(\"x\") err = nil; want non-nil (x is not a function)")
	}
	if !strings.Contains(callErr.Error(), "not a function") {
		t.Fatalf("vm.CallGlobal err = %q; want substring %q", callErr.Error(), "not a function")
	}
}

// TestVM_CallGlobal_Undefined verifies CallGlobal on an undefined
// global returns an error citing "not a function" (nil is not a
// function from the script's perspective).
func TestVM_CallGlobal_Undefined(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	callErr := vm.CallGlobal("nonexistent")
	if callErr == nil {
		t.Fatal("vm.CallGlobal(\"nonexistent\") err = nil; want non-nil")
	}
}

// TestVM_PanicWrapper_GoFromCallback verifies a Go panic in a registered
// Go callback is recovered by the VM's panic-wrapper: CallGlobal
// returns a non-nil error (NOT propagating the panic) AND the registered
// PanicHandlerFn is invoked with the recovered value.
func TestVM_PanicWrapper_GoFromCallback(t *testing.T) {
	var recovered any
	vm := NewVM(WithPanicHandler(func(r any) { recovered = r }))
	defer vm.Close()
	vm.RegisterGlobalFunc("panicker", func(L *lua.LState) int {
		panic("from-go-callback")
	})
	chunk, err := CompileScript([]byte(`panicker()`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	runErr := vm.Run(chunk)
	if runErr == nil {
		t.Fatal("vm.Run err = nil; want non-nil after Go callback panics (panic-wrapper must convert to error)")
	}
	if recovered == nil {
		t.Fatal("PanicHandlerFn was NOT invoked; want invoked with recovered value")
	}
}

// TestVM_PanicWrapper_NoHandler verifies a Go panic is still recovered
// (converted to error) even when no PanicHandlerFn is registered.
func TestVM_PanicWrapper_NoHandler(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	vm.RegisterGlobalFunc("panicker", func(L *lua.LState) int {
		panic("from-go-callback-no-handler")
	})
	chunk, err := CompileScript([]byte(`panicker()`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	runErr := vm.Run(chunk)
	if runErr == nil {
		t.Fatal("vm.Run err = nil; want non-nil after Go callback panics (panic-wrapper must convert to error even without handler)")
	}
}

// TestVM_Close_Idempotent verifies double-Close is a no-op (no panic
// on the second invocation).
func TestVM_Close_Idempotent(t *testing.T) {
	vm := NewVM()
	vm.Close()
	// Second Close must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("second vm.Close() panicked: %v; want idempotent no-op", r)
		}
	}()
	vm.Close()
}

// TestVM_State_NilAfterClose verifies State() returns nil post-Close
// (the underlying *lua.LState is released + the field zeroed).
func TestVM_State_NilAfterClose(t *testing.T) {
	vm := NewVM()
	vm.Close()
	if vm.State() != nil {
		t.Fatal("vm.State() != nil after Close; want nil")
	}
}

// TestVM_CallGlobal_WithArgs verifies args are pushed in order +
// the Go-callback sees them through *lua.LState.Get(n).
func TestVM_CallGlobal_WithArgs(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	chunk, err := CompileScript([]byte(`got1, got2 = nil, nil; function foo(a, b) got1 = a; got2 = b end`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}
	if err := vm.CallGlobal("foo", lua.LString("hello"), lua.LNumber(7)); err != nil {
		t.Fatalf("vm.CallGlobal err = %v; want nil", err)
	}
	got1 := vm.State().GetGlobal("got1")
	got2 := vm.State().GetGlobal("got2")
	if got1 != lua.LString("hello") {
		t.Fatalf("got1 = %v; want \"hello\"", got1)
	}
	if got2 != lua.LNumber(7) {
		t.Fatalf("got2 = %v; want 7", got2)
	}
}

// ----------------------------------------------------------------------
// Task 12 — concurrent VM construction tests per 22.1 PLAN Task 12 + 22.1
// SPEC §6 Task 12 + 22.1 SPEC §2.19 + §13-R6 + parent §13-R6.
//
// Validates the cross-VM `*Chunk` reuse contract: a single *FunctionProto
// authored by CompileScript is safe to load onto N concurrent *lua.LStates
// concurrently. Each per-stream VM gets its OWN *lua.LState — no global
// mutable state is shared across VMs (the *FunctionProto is bytecode-only
// + read-only at Run time per gopher-lua's API contract).
//
// All tests in this section are intended to be run under -race. The
// acceptance criterion is no data race AND no cross-VM state leak.
// Run via:
//
//   go test -race -count=10 ./internal/lua/...
//
// ----------------------------------------------------------------------

// TestVM_ConcurrentNewVM_RunCallGlobalClose verifies that N=100 goroutines
// each constructing a fresh VM via NewVM, running the SAME *Chunk, calling
// the SAME named hook, and Close()-ing the VM, do not race. Each VM
// observes its OWN state (no cross-VM leak via the shared *Chunk's
// *FunctionProto). The hook also writes a per-VM sentinel global to
// assert per-VM isolation — the sentinel value must equal the goroutine's
// input index, never another goroutine's index.
func TestVM_ConcurrentNewVM_RunCallGlobalClose(t *testing.T) {
	// Use a single shared chunk to verify the cross-VM *FunctionProto
	// reuse contract. The hook reads its single arg and writes it to a
	// per-VM global `seen`.
	chunk, err := CompileScript([]byte(`
		function envoy_on_request(idx)
			seen = idx
		end
	`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}

	const N = 100
	var (
		wg      sync.WaitGroup
		failCnt atomic.Int64
	)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			vm := NewVM()
			defer vm.Close()
			if err := vm.Run(chunk); err != nil {
				t.Errorf("goroutine %d: vm.Run err = %v; want nil", idx, err)
				failCnt.Add(1)
				return
			}
			if !vm.HasGlobalFunc("envoy_on_request") {
				t.Errorf("goroutine %d: envoy_on_request hook NOT defined", idx)
				failCnt.Add(1)
				return
			}
			if err := vm.CallGlobal("envoy_on_request", lua.LNumber(idx)); err != nil {
				t.Errorf("goroutine %d: CallGlobal err = %v; want nil", idx, err)
				failCnt.Add(1)
				return
			}
			// Per-VM isolation: this VM's `seen` must equal THIS
			// goroutine's idx (no cross-VM leak via the shared
			// *FunctionProto). If the shared *FunctionProto leaked
			// mutable state across VMs, two goroutines could observe
			// each other's idx.
			seen := vm.State().GetGlobal("seen")
			if got, want := seen, lua.LNumber(idx); got != want {
				t.Errorf("goroutine %d: seen = %v; want %v (cross-VM state leak via shared *Chunk)", idx, got, want)
				failCnt.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if n := failCnt.Load(); n > 0 {
		t.Fatalf("%d / %d goroutines failed concurrency assertions", n, N)
	}
}

// TestVM_ConcurrentNewVM_SharedChunkAndCache verifies that N goroutines
// sharing both the *Chunk AND the *CompileCache that produced it do not
// race. The cache is read-only on this path (no goroutine adds new
// entries), but the *Chunk lookup goes through the cache's RWMutex
// fast-path — the test exercises the (RLock, lookup hit, RUnlock) cycle
// concurrently with the per-stream VM construction.
func TestVM_ConcurrentNewVM_SharedChunkAndCache(t *testing.T) {
	cache := NewCompileCache()
	const src = `function envoy_on_request() end`
	chunk, err := CompileScript([]byte(src), cache)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	_ = chunk // primed in the cache

	const N = 100
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine reads the chunk from the cache + uses it.
			c, err := CompileScript([]byte(src), cache)
			if err != nil {
				t.Errorf("CompileScript (cache hit) err = %v; want nil", err)
				return
			}
			vm := NewVM()
			defer vm.Close()
			if err := vm.Run(c); err != nil {
				t.Errorf("vm.Run err = %v; want nil", err)
				return
			}
			if err := vm.CallGlobal("envoy_on_request"); err != nil {
				t.Errorf("vm.CallGlobal err = %v; want nil", err)
			}
		}()
	}
	wg.Wait()
}
