package lua

// coroutine.go — Task 2 (phase 22.2 IMPL) NEW Go-side coroutine API
// extensions per 22.2 SPEC §3.2 + ADR-0191 §Context + §11.1 D2 closure
// (gopher-lua native LState.NewThread / Yield / Resume — Option B
// Go-side channel-scheduler wrapper REJECTED per §11.1 D2).
//
// Three additions to the package public surface:
//
//   - (vm *VM) NewThread() (*lua.LState, context.CancelFunc) wraps
//     gopher-lua's *LState.NewThread() (state.go:1614). Mints a child
//     *LState sharing the parent's globals (`G`+`Env`). The returned
//     context.CancelFunc cancels the child's ctx-derived loop; bridge
//     authors MUST call it at stream destroy to avoid leaking ctx-
//     attached child loops (only non-nil when the parent LState carries
//     a context — see gopher-lua state.go:1618).
//
//   - (vm *VM) Resume(child *lua.LState, fn *lua.LFunction, args ...)
//     wraps the parent's *LState.Resume (state.go:2157). Returns
//     (ResumeOK, nil, values) on script return; (ResumeYield, nil,
//     yielded-values) on coroutine yield; (ResumeError, *ApiError, nil)
//     on Lua runtime or bridge-side panic. Wrapped under withPanicWrap
//     per ADR-0188 §Decision 2; in addition, any ResumeError result is
//     forwarded to vm.panicH per the ADR-0191 panic-wrapper extension
//     for coroutines — gopher-lua's threadRun (vm.go:272) installs its
//     own recover() that converts Go panics in bridge LGFunctions into
//     an *ApiError(ApiErrorRun) routed back to the parent's Resume call
//     site (state.go:2210). Since coroutine bridge panics universally
//     surface as ApiErrorRun rather than ApiErrorPanic, the dispatch
//     here MUST cover the broader case to honor the panic-wrapper
//     discipline at the coroutine surface. (See coroutineDispatchPanic
//     for the broadened dispatcher used by Resume; the dispatchPanic
//     used by Run / CallGlobal stays scoped to ApiErrorPanic and is
//     UNCHANGED.)
//
//   - YieldFromBridge(L *lua.LState, args ...) is a Go-side helper for
//     bridge LGFunction implementations. The LGFunction body MUST
//     `return YieldFromBridge(L, ...)` directly to gopher-lua so
//     callGFunction (vm.go:200-210) sees gfnret < 0 and calls
//     switchToParentThread per §11.1 D2 closure. Package-level
//     (NOT a method on *VM) so bridge LGFunctions don't have to
//     close over the parent VM to yield.
//
// Per-stream child-LState lifecycle: 1 parent *LState per stream
// (compiled-bytecode owner; lives for filter lifetime) + 1 child per
// phase invocation (the coroutine; cheap to construct — shares
// `G`+`Env`). Both released on stream destroy; child's
// context.CancelFunc invoked at stream destroy per ADR-0191 §Context.
//
// Lineage-separation note (per BRAINSTORM Q10 strict scope + ADR-0188
// §Decision 5 API-REVISION ALLOWANCE clause): this file is NEW (NOT
// an in-place APPEND to vm.go); the 22.1 VM surface (`NewVM`, `State`,
// `RegisterGlobalFunc`, `Run`, `HasGlobalFunc`, `CallGlobal`, `Close`,
// `PanicHandlerFn`, `VMOption`, `WithSandboxConfig`,
// `WithPanicHandler`, `WithBasePrintSink`) is UNCHANGED. The consumer-#1
// scope-expansion lands under NEW ADR-0191 (not in-place AMEND on
// ADR-0188); ADR-0188's API-REVISION ALLOWANCE STAYS scoped to
// consumer-#2 (future cluster-specifier / access-logger / string-matcher
// Lua phases).
//
// The script-side `coroutine` library (gopher-lua's coroutinelib.go
// surface — `coroutine.create` / `resume` / `yield` / `running` /
// `status` / `wrap`) stays ENABLED via the sandbox roster's
// `AllowCoroutine` zero-value default (per 22.1 AMEND-A4). The Go-side
// API here is ORTHOGONAL to the script-side surface — bridge authors
// use this Go-side API to suspend the running script from a Go bridge
// LGFunction (e.g., `:body()` when the request body is not yet
// buffered).
//
// See:
//   - 22.2 SPEC §3.2 (production signatures)
//   - 22.2 SPEC §11.1 D2 closure (gopher-lua native evidence)
//   - DECISIONS.md ADR-0191 §Context (lineage-separation rationale)
//   - DECISIONS.md ADR-0188 §Decision 2 (panic-wrapper discipline)
//   - DECISIONS.md ADR-0188 §Decision 5 (API-REVISION ALLOWANCE clause —
//     STAYS scoped to consumer-#2; consumer-#1 scope-expansion lands
//     under NEW ADR-0191 not in-place AMEND on ADR-0188)

import (
	"context"
	"errors"

	lua "github.com/yuin/gopher-lua"
)

// NewThread constructs a child *LState as a coroutine state, sharing
// globals (`G`+`Env`) with the parent. Per §11.1 D2 closure: the
// gopher-lua native LState.NewThread() (state.go:1614) is the
// underlying mechanism — the returned *LState IS the coroutine state.
// Callers Resume it via the parent VM's Resume method.
//
// The returned context.CancelFunc is non-nil only when the parent
// LState carries a context (see gopher-lua state.go:1618 — only
// constructs a child ctx when ls.ctx != nil). Bridge authors that
// attach a per-stream context to the parent (via vm.State().SetContext)
// MUST invoke this CancelFunc at stream destroy to cancel the child's
// ctx-derived loop and avoid leaking ctx-attached child loops.
//
// On a closed VM (post-Close), returns (nil, nil) rather than panicking
// per the package's nil-tolerance ergonomic discipline (mirrors Run /
// HasGlobalFunc / CallGlobal post-Close behavior).
func (vm *VM) NewThread() (*lua.LState, context.CancelFunc) {
	if vm.state == nil {
		return nil, nil
	}
	return vm.state.NewThread()
}

// Resume invokes parent.Resume(child, fn, args...) per gopher-lua
// semantics (state.go:2157). The first call seeds the coroutine with
// `fn` + `args`; subsequent calls (on resume-after-yield) pass `fn=nil`
// + the resume-values forwarded as Yield's return value into the
// suspended script.
//
// Return shapes per §11.1 D2 closure + gopher-lua state.go:2210-2214:
//   - ResumeOK: script returned; `values` are the returned values;
//     err is nil.
//   - ResumeYield: script yielded; `values` are the yielded values
//     (from L.Yield in the bridge LGFunction); err is nil.
//   - ResumeError: Lua runtime error OR gopher-lua-internal-recovered
//     Go panic; `values` is nil; err is the *lua.ApiError carrying
//     the error object.
//
// Wrapped under withPanicWrap per ADR-0188 §Decision 2: a Go panic
// that escapes gopher-lua's own recover paths (state.go:2040;
// vm.go:272 threadRun) is caught here and routed through vm.panicH if
// registered; the surfaced error includes the recovered value. When
// gopher-lua's own recover paths catch the panic and surface it as a
// *lua.ApiError of type ApiErrorPanic, dispatchPanic forwards the
// recovered value to vm.panicH for symmetric handling with Run /
// CallGlobal.
//
// On a closed VM, returns (ResumeError, error, nil) without panicking
// per the package's nil-tolerance ergonomic discipline.
func (vm *VM) Resume(child *lua.LState, fn *lua.LFunction, args ...lua.LValue) (lua.ResumeState, error, []lua.LValue) {
	if vm.state == nil {
		return lua.ResumeError, errors.New("lua: vm is closed"), nil
	}
	if child == nil {
		return lua.ResumeError, errors.New("lua: nil child *LState"), nil
	}

	var (
		state  lua.ResumeState
		rerr   error
		values []lua.LValue
	)
	wrapErr := vm.withPanicWrap(func() error {
		state, rerr, values = vm.state.Resume(child, fn, args...)
		if rerr != nil {
			// Panic-wrapper discipline at the coroutine surface (ADR-0191
			// extension of ADR-0188 §Decision 2). gopher-lua's threadRun
			// (vm.go:272) installs its own recover() that converts Go
			// panics in bridge LGFunctions into an *ApiError(ApiErrorRun)
			// — i.e., the Go-panic origin is opaque at this seam, the
			// surface error type does NOT carry a Go-vs-Lua distinction.
			// To honor the panic-wrapper discipline at the coroutine
			// seam, dispatch ANY ResumeError to vm.panicH (broader than
			// dispatchPanic's ApiErrorPanic-only scope used by Run /
			// CallGlobal). Consumers that only want Go-panic observability
			// can filter inside their panicH using *ApiError type +
			// Object string-pattern inspection; the default empty
			// handler is no-op.
			coroutineDispatchPanic(vm, rerr)
		}
		return nil
	})
	// If withPanicWrap caught a panic that escaped gopher-lua's own
	// recover paths, prefer its error (it carries the recovered value
	// via the standard "lua: panic recovered: ..." formatter) and
	// surface ResumeError.
	if wrapErr != nil {
		return lua.ResumeError, wrapErr, nil
	}
	return state, rerr, values
}

// coroutineDispatchPanic forwards a Resume-surfaced *ApiError (any
// type) to vm.panicH per the ADR-0191 panic-wrapper extension for
// coroutines. Unlike VM.dispatchPanic (Run / CallGlobal — scoped to
// ApiErrorPanic only), the coroutine path uses this broader dispatcher
// because gopher-lua's threadRun-internal recover converts Go panics
// in bridge LGFunctions into ApiErrorRun-typed *ApiErrors, erasing the
// Go-vs-Lua origin distinction. See coroutine.go file-doc for the
// discipline rationale.
func coroutineDispatchPanic(vm *VM, err error) {
	if vm == nil || vm.panicH == nil || err == nil {
		return
	}
	var apiErr *lua.ApiError
	if errors.As(err, &apiErr) {
		vm.panicH(apiErr.Object.String())
		return
	}
	vm.panicH(err.Error())
}

// YieldFromBridge is a Go-side helper for bridge LGFunction
// implementations. It pushes `args` onto the bridge LState (via
// L.Yield) and returns the gopher-lua yield sentinel (-1).
//
// Discipline (per §11.1 D2 closure):
//
//	func myBridgeMethod(L *lua.LState) int {
//	    // ... resolve filter pointer, stash *L in pending-map ...
//	    return YieldFromBridge(L, lua.LNil)
//	}
//
// The bridge LGFunction MUST return YieldFromBridge's result directly
// to gopher-lua — callGFunction (gopher-lua vm.go:200-210) sees
// gfnret < 0 and calls switchToParentThread, unwinding to the parent
// LState's Resume call site. The parent then handles the yielded
// values and (eventually) calls Resume(child, nil, resume-values) to
// resume the suspended script.
//
// On a nil L, returns 0 (no-op) per the package's nil-tolerance
// ergonomic discipline — a nil bridge LState is not a panic; the
// caller's bridge dispatcher is responsible for non-nil L invariants.
func YieldFromBridge(L *lua.LState, args ...lua.LValue) int {
	if L == nil {
		return 0
	}
	return L.Yield(args...)
}
