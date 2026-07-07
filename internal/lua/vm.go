package lua

// vm.go — VM lifecycle + VMOption configurators + State escape-hatch +
// Run / HasGlobalFunc / CallGlobal hook dispatch + Close per 22.1 SPEC
// §3.1 production signatures + §3.4 per-stream lifecycle.
//
// Task 5 IMPL — full *lua.LState construction with the §3.3 sandbox
// roster applied (see sandbox.go applySandbox) + print-redirect to
// BasePrintSink + INTERNAL __envoy_traceback for the panic-wrapper's
// reference + Run loads + executes a precompiled *Chunk via
// NewFunctionFromProto + PCall + HasGlobalFunc surfaces hook-presence
// + CallGlobal dispatches a script-defined hook + panic-wrapper
// converts genuine Go panics into errors (Lua-side runtime errors
// already surface through PCall's error path).
//
// The public surface declared here is BYTE-PINNED at 22.1 SPEC §3.1;
// the vm_test.go compile-time assertions catch any drift.

import (
	"errors"
	"fmt"
	"io"

	lua "github.com/yuin/gopher-lua"
)

// internalTracebackGlobal is the global name under which we expose the
// panic-wrapper's traceback-helper. The `debug` stdlib is DENIED at
// sandbox-strict (per §3.3); the panic-wrapper still wants a structured
// traceback hook, hence the leading-underscore conventionally-private
// name. Scripts CAN see this global (no way to hide it from script
// scope without per-call env juggling), but the leading `__envoy_`
// prefix marks it clearly out-of-band.
const internalTracebackGlobal = "__envoy_traceback"

// VM is a per-stream gopher-lua execution context. NOT goroutine-safe.
// Each per-stream filter dispatch constructs a fresh VM via NewVM;
// OnDestroy releases via Close. The unexported fields hold the
// *lua.LState + the resolved sandbox config + the panic handler + the
// print sink.
type VM struct {
	state     *lua.LState
	sandbox   SandboxConfig
	panicH    PanicHandlerFn
	printSink io.Writer
}

// VMOption configures VM construction. Function-option pattern per
// 22.1 SPEC §3.1 (idiomatic Go; mirrors internal/jwks/Fetcher and
// internal/httpclient/ precedent). Easier to extend at consumer-#2
// phases without API revision than a sealed-interface alternative.
type VMOption func(*VM)

// WithSandboxConfig sets the per-stdlib ALLOW/DENY posture. Zero value =
// StrictUpstreamParity (DENY io/os/debug/package/channel + base-stdlib
// dangerous globals; ALLOW base-core + table + string + math + coroutine
// + os.time/date/clock/difftime via AllowOSTimeHelpers — both flipped
// on by applyZeroValueDefaults to match upstream luaL_openlibs). See
// §3.3 + sandbox.go.
func WithSandboxConfig(sb SandboxConfig) VMOption {
	return func(vm *VM) { vm.sandbox = sb }
}

// WithPanicHandler sets the Go-panic handler invoked after recover() in
// the VM's panic-wrapper. The handler is invoked with the recovered
// value. NOT for catching Lua-side runtime errors (those return via
// *lua.ApiError from Run / CallGlobal); this handler is for genuine
// Go panics from bridge method Go callbacks.
func WithPanicHandler(h PanicHandlerFn) VMOption {
	return func(vm *VM) { vm.panicH = h }
}

// WithBasePrintSink redirects Lua-script print(...) output. Default nil =
// drop (no stdout leak; envoy-go-strict default). NewVM rebinds the
// base-stdlib `print` global to a Go function that writes to this sink
// if non-nil, otherwise drops.
func WithBasePrintSink(w io.Writer) VMOption {
	return func(vm *VM) { vm.printSink = w }
}

// NewVM constructs a per-stream VM. Applies sandbox config + print-
// redirect + INTERNAL __envoy_traceback + panic-wrapper setup. Caller
// responsibility: Close at OnDestroy. Returns a non-nil *VM.
//
// Construction sequence (Task 5):
//  1. lua.NewState(lua.Options{SkipOpenLibs: true}) — the catch-all
//     luaL_openlibs is NEVER invoked; libs are opened selectively per
//     the SandboxConfig roster.
//  2. Apply VMOptions (each may mutate vm.sandbox / vm.panicH /
//     vm.printSink).
//  3. Resolve effective sandbox via applyZeroValueDefaults (flips
//     AllowCoroutine + AllowOSTimeHelpers to true at zero-value posture
//     per AMEND-A4); store back into vm.sandbox.
//  4. applySandbox(state, sandbox) — selective per-stdlib OpenXxx +
//     post-walk nil-out per §3.3 (sandbox.go).
//  5. Rebind `print` to a Go closure over vm.printSink (default = drop).
//  6. Expose __envoy_traceback as an INTERNAL global for the panic-
//     wrapper's reference.
func NewVM(opts ...VMOption) *VM {
	vm := &VM{
		state: lua.NewState(lua.Options{SkipOpenLibs: true}),
	}
	for _, opt := range opts {
		opt(vm)
	}
	vm.sandbox = applyZeroValueDefaults(vm.sandbox)
	applySandbox(vm.state, vm.sandbox)
	vm.installPrintRedirect()
	vm.installInternalTraceback()
	return vm
}

// installPrintRedirect rebinds the base-stdlib `print` global to a Go
// closure that writes to vm.printSink if non-nil, otherwise drops the
// output. Mirrors Lua's standard `print` semantics: stringifies each
// argument via tostring (delegated via the *lua.LState's metamethod
// dispatch) + joins with tabs + appends `\n`. When printSink is nil
// the closure returns immediately WITHOUT stringifying the args — any
// `__tostring` metamethod side effects are skipped in the default
// (nil-sink) configuration.
func (vm *VM) installPrintRedirect() {
	sink := vm.printSink
	vm.state.SetGlobal("print", vm.state.NewFunction(func(L *lua.LState) int {
		n := L.GetTop()
		if sink == nil {
			return 0
		}
		for i := 1; i <= n; i++ {
			if i > 1 {
				_, _ = io.WriteString(sink, "\t")
			}
			// L.ToStringMeta honors __tostring metamethods on
			// userdata + tables (Lua's print() semantics);
			// fall back to L.Get(i).String() for plain values.
			s := L.ToStringMeta(L.Get(i)).String()
			_, _ = io.WriteString(sink, s)
		}
		_, _ = io.WriteString(sink, "\n")
		return 0
	}))
}

// installInternalTraceback exposes the panic-wrapper's INTERNAL
// traceback helper under the __envoy_traceback global. The helper
// returns the current Lua-side stack traceback as a string (best-
// effort — uses *LState.Where(1) as a lightweight stand-in for the
// full debug.traceback since the debug stdlib is DENIED in the
// default sandbox).
//
// Scripts CAN see this global (no per-call env juggling at 22.1),
// but the leading `__envoy_` prefix + the underscore convention
// marks it clearly out-of-band — operator-authored scripts that
// inadvertently shadow it bear the consequence.
func (vm *VM) installInternalTraceback() {
	vm.state.SetGlobal(internalTracebackGlobal, vm.state.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(L.Where(1)))
		return 1
	}))
}

// State returns the underlying *lua.LState. ESCAPE-HATCH for filter
// consumers to set up userdata + metatables (request_handle /
// response_handle patterns require direct LState access). Not safe to
// call after Close — returns nil post-Close.
func (vm *VM) State() *lua.LState {
	return vm.state
}

// RegisterGlobalFunc registers a Go function as a Lua-callable global.
// CONVENIENCE for simple globals; userdata + metatables go via State().
func (vm *VM) RegisterGlobalFunc(name string, fn lua.LGFunction) {
	if vm.state == nil {
		return
	}
	vm.state.SetGlobal(name, vm.state.NewFunction(fn))
}

// Run loads the chunk's *FunctionProto onto this VM's *LState (cheap;
// no recompilation per stream — chunks are bytecode-only, no LState-
// specific state) and executes script top-level (which defines any
// globals declared by the script, e.g., envoy_on_request /
// envoy_on_response). Returns a wrapped error on Lua runtime error or
// recovered Go panic.
//
// Lua-side runtime errors surface through PCall's error return as a
// *lua.ApiError with Type == ApiErrorRun / ApiErrorError. Go panics
// inside Lua-invoked Go callbacks are recovered by PCall internally
// and surface as a *lua.ApiError with Type == ApiErrorPanic; the
// dispatchPanic helper routes those to the registered PanicHandlerFn
// (if any) BEFORE returning the wrapped error. A genuine Go panic
// that escapes PCall (e.g., a panic outside the Lua call site) is
// caught by withPanicWrap.
func (vm *VM) Run(chunk *Chunk) error {
	if vm.state == nil {
		return errors.New("lua: vm is closed")
	}
	if chunk == nil || chunk.proto == nil {
		return errors.New("lua: nil chunk")
	}
	return vm.withPanicWrap(func() error {
		fn := vm.state.NewFunctionFromProto(chunk.proto)
		vm.state.Push(fn)
		if err := vm.state.PCall(0, lua.MultRet, nil); err != nil {
			vm.dispatchPanic(err)
			return fmt.Errorf("lua run: %w", err)
		}
		return nil
	})
}

// HasGlobalFunc returns true if the named global is a callable
// function. Used by the filter to check hook-presence after Run
// (supports D1 script-missing-required-hooks PARSE-REJECT —
// anticipated; closes at IMPL Task 2 against upstream Envoy).
func (vm *VM) HasGlobalFunc(name string) bool {
	if vm.state == nil {
		return false
	}
	return vm.state.GetGlobal(name).Type() == lua.LTFunction
}

// CallGlobal invokes the named global function with args. Returns a
// wrapped error on Lua runtime error or recovered Go panic. The
// filter consumer pushes userdata (e.g., request_handle) via the args
// slice using lua.LValue (typically the LUserData wrapping a Go
// pointer).
//
// If the named global is not a function (nil, number, table, etc.),
// the call returns an error citing "not a function" rather than
// attempting an unsupported invoke.
func (vm *VM) CallGlobal(name string, args ...lua.LValue) error {
	if vm.state == nil {
		return errors.New("lua: vm is closed")
	}
	lv := vm.state.GetGlobal(name)
	if lv.Type() != lua.LTFunction {
		return fmt.Errorf("lua call: global %q is not a function (got %s)", name, lv.Type())
	}
	return vm.withPanicWrap(func() error {
		vm.state.Push(lv)
		for _, a := range args {
			vm.state.Push(a)
		}
		if err := vm.state.PCall(len(args), 0, nil); err != nil {
			vm.dispatchPanic(err)
			return fmt.Errorf("lua call %q: %w", name, err)
		}
		return nil
	})
}

// dispatchPanic invokes the registered PanicHandlerFn (if any) when
// the supplied error is a *lua.ApiError of type ApiErrorPanic (i.e.,
// gopher-lua's PCall recovered a Go panic inside a Lua-invoked Go
// callback). For non-panic errors (ApiErrorRun / ApiErrorError /
// ApiErrorSyntax / ApiErrorFile — pure Lua-side runtime/parse errors),
// the handler is NOT invoked: the panic handler is specifically for
// Go-panic-derived errors. The recovered value is reconstructed from
// the *ApiError.Object (which holds the panic value as a Lua string
// via fmt.Sprint(rcv) at the gopher-lua state.go PCall recover site).
func (vm *VM) dispatchPanic(err error) {
	if vm.panicH == nil {
		return
	}
	var apiErr *lua.ApiError
	if !errors.As(err, &apiErr) {
		return
	}
	if apiErr.Type != lua.ApiErrorPanic {
		return
	}
	// The Object field holds the original panic value (stringified via
	// fmt.Sprint in gopher-lua's PCall recover at state.go:2045). Pass
	// the Lua string back as the recovered value; downstream handlers
	// commonly want it as an `any` for logging.
	vm.panicH(apiErr.Object.String())
}

// Close releases the VM's *lua.LState. Idempotent — a second Close
// after the first is a no-op.
func (vm *VM) Close() {
	if vm.state != nil {
		vm.state.Close()
		vm.state = nil
	}
}

// withPanicWrap runs fn under a deferred recover() that converts a Go
// panic to an error return + invokes the registered PanicHandlerFn (if
// any) with the recovered value. The wrapper is the bridge between
// "Go panics inside Lua-invoked Go callbacks" and "Run / CallGlobal
// error returns" — without it a panic in a registered Go function
// would propagate up the gopher-lua call stack and crash the
// per-stream goroutine.
//
// Lua-side runtime errors do NOT trigger this wrapper; they surface
// through *lua.LState.PCall's error return.
func (vm *VM) withPanicWrap(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if vm.panicH != nil {
				vm.panicH(r)
			}
			err = fmt.Errorf("lua: panic recovered: %v", r)
		}
	}()
	return fn()
}

// PanicHandlerFn is invoked after recover() in the VM's panic-wrapper.
// recovered is the value returned by recover() (typically the panic
// value).
type PanicHandlerFn func(recovered any)
