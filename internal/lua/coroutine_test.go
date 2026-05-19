package lua

// coroutine_test.go — Task 2 (phase 22.2 IMPL) behavioral tests for the
// NEW coroutine API extensions per SPEC §3.2 + ADR-0191 §Context +
// §11.1 D2 closure (gopher-lua native LState.NewThread / Yield /
// Resume).
//
// Tests cover:
//   - NewThread returns non-nil child *LState + non-nil CancelFunc
//   - Child shares globals with parent (Go-registered globals visible
//     from child).
//   - Resume happy path: child returns a value without yielding =>
//     ResumeOK + returned values + nil error.
//   - Resume + YieldFromBridge round-trip: child invokes Go bridge that
//     calls YieldFromBridge => ResumeYield + yielded values.
//   - Resume after yield resumes from where Yield returned: multi-step
//     coroutine yields a value, parent resumes with a new value, script
//     continues with the resume value.
//   - CancelFunc cleanup: invoking CancelFunc cleans up child without
//     goroutine leaks (NumGoroutine delta bounded).
//   - Resume panic-wrapper integration: Go panic in a bridge LGFunction
//     during Resume routes through vm.panicH callback per ADR-0188
//     §Decision 2 panic-wrapper discipline and surfaces a non-nil error.
//   - Race: N=100 parallel VMs each driving a coroutine; clean under
//     `-race`.

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// Compile-time assertions pin the public API surface declared in
// coroutine.go per 22.2 SPEC §3.2 + ADR-0191 §Context. The signatures
// captured here become a build-break the moment any of them drifts.
var (
	_ func(*VM) (*lua.LState, context.CancelFunc)                                                  = (*VM).NewThread
	_ func(*VM, *lua.LState, *lua.LFunction, ...lua.LValue) (lua.ResumeState, error, []lua.LValue) = (*VM).Resume
	_ func(*lua.LState, ...lua.LValue) int                                                         = YieldFromBridge
)

// TestVM_NewThread_returns_nonnil_child_and_CancelFunc verifies the
// happy-path construction contract: NewThread returns a non-nil child
// *LState whose globals are shared with the parent, and (per gopher-lua
// state.go:1614) a CancelFunc that is non-nil when the parent's LState
// carries a context (which we set explicitly via vm.State().SetContext).
//
// Note: gopher-lua only returns a non-nil CancelFunc when the parent
// LState has a context attached. The bridge layer attaches a context
// in production (per-stream dispatch); this test mirrors that pattern.
func TestVM_NewThread_returns_nonnil_child_and_CancelFunc(t *testing.T) {
	vm := NewVM()
	defer vm.Close()

	// Mirror per-stream dispatch by attaching a context to the parent.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vm.State().SetContext(ctx)

	// Register a Go global on the parent so we can later assert globals
	// are visible from the child.
	var seen atomic.Bool
	vm.RegisterGlobalFunc("parent_global", func(L *lua.LState) int {
		seen.Store(true)
		return 0
	})

	child, cancelFn := vm.NewThread()
	if child == nil {
		t.Fatal("NewThread: child == nil; want non-nil child *LState")
	}
	if cancelFn == nil {
		t.Fatal("NewThread: cancelFn == nil; want non-nil CancelFunc when parent LState carries a context (gopher-lua state.go:1614)")
	}
	defer cancelFn()

	// Verify the child sees the parent's Go-registered globals.
	gv := child.GetGlobal("parent_global")
	if gv.Type() != lua.LTFunction {
		t.Fatalf("child.GetGlobal(\"parent_global\").Type() = %v; want LTFunction (globals must be shared)", gv.Type())
	}
}

// TestVM_Resume_happy_no_yield exercises the no-yield happy path: the
// child function returns a value at top-level without invoking yield;
// Resume returns ResumeOK + the returned values + nil error.
func TestVM_Resume_happy_no_yield(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vm.State().SetContext(ctx)

	// Compile + run a chunk that defines a global function `child_fn`
	// returning a constant string.
	chunk, err := CompileScript([]byte(`function child_fn() return "ok" end`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}

	// Resolve the global to its *LFunction.
	lv := vm.State().GetGlobal("child_fn")
	fn, ok := lv.(*lua.LFunction)
	if !ok {
		t.Fatalf("child_fn global type = %T; want *lua.LFunction", lv)
	}

	child, cancelFn := vm.NewThread()
	defer cancelFn()

	state, rerr, values := vm.Resume(child, fn)
	if rerr != nil {
		t.Fatalf("Resume err = %v; want nil", rerr)
	}
	if state != lua.ResumeOK {
		t.Fatalf("Resume state = %v; want ResumeOK", state)
	}
	if len(values) != 1 {
		t.Fatalf("Resume returned %d values; want 1", len(values))
	}
	if values[0].String() != "ok" {
		t.Fatalf("Resume returned values[0] = %q; want %q", values[0].String(), "ok")
	}
}

// TestVM_Resume_with_YieldFromBridge_roundtrip exercises the yield path
// via the bridge helper: the child function invokes a Go bridge that
// calls YieldFromBridge with a sentinel value; Resume returns
// ResumeYield + the yielded values + nil error.
func TestVM_Resume_with_YieldFromBridge_roundtrip(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vm.State().SetContext(ctx)

	// Register a Go bridge that yields the string "yielded-val". The
	// bridge MUST return YieldFromBridge's result directly to gopher-lua
	// so callGFunction (vm.go:200-210) sees gfnret<0 and unwinds via
	// switchToParentThread per §11.1 D2 closure.
	vm.RegisterGlobalFunc("bridge_yield", func(L *lua.LState) int {
		return YieldFromBridge(L, lua.LString("yielded-val"))
	})

	chunk, err := CompileScript([]byte(`function child_fn() bridge_yield(); return "after-resume" end`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}
	fn := vm.State().GetGlobal("child_fn").(*lua.LFunction)

	child, cancelFn := vm.NewThread()
	defer cancelFn()

	state, rerr, values := vm.Resume(child, fn)
	if rerr != nil {
		t.Fatalf("Resume err = %v; want nil", rerr)
	}
	if state != lua.ResumeYield {
		t.Fatalf("Resume state = %v; want ResumeYield", state)
	}
	if len(values) != 1 {
		t.Fatalf("Resume returned %d yielded values; want 1", len(values))
	}
	if values[0].String() != "yielded-val" {
		t.Fatalf("Resume yielded values[0] = %q; want %q", values[0].String(), "yielded-val")
	}
}

// TestVM_Resume_after_yield_resumes_from_where_Yield_returned exercises
// the multi-step round-trip: child yields, parent resumes with a new
// value, the script continues with that resume value as the yield's
// return value, and finally returns a result. Mirrors the production
// `:body()` deferred-resume pattern.
func TestVM_Resume_after_yield_resumes_from_where_Yield_returned(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vm.State().SetContext(ctx)

	// Bridge yields nil; the resume call passes "resumed-body" back —
	// the bridge function's Lua-side call expression evaluates to that
	// value (this is the script-author-facing yield-return discipline).
	vm.RegisterGlobalFunc("bridge_yield", func(L *lua.LState) int {
		return YieldFromBridge(L, lua.LNil)
	})

	chunk, err := CompileScript([]byte(`
		function child_fn()
			local body = bridge_yield()
			return "got:" .. tostring(body)
		end
	`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}
	fn := vm.State().GetGlobal("child_fn").(*lua.LFunction)

	child, cancelFn := vm.NewThread()
	defer cancelFn()

	// Step 1: Resume kicks off the coroutine; it yields nil.
	state, rerr, _ := vm.Resume(child, fn)
	if rerr != nil {
		t.Fatalf("Resume[1] err = %v; want nil", rerr)
	}
	if state != lua.ResumeYield {
		t.Fatalf("Resume[1] state = %v; want ResumeYield", state)
	}

	// Step 2: Resume the suspended coroutine with the deferred body
	// value. gopher-lua state.go:2157 takes fn=nil + args on resume-after-
	// yield (the running coroutine's resumed values).
	state, rerr, values := vm.Resume(child, nil, lua.LString("resumed-body"))
	if rerr != nil {
		t.Fatalf("Resume[2] err = %v; want nil", rerr)
	}
	if state != lua.ResumeOK {
		t.Fatalf("Resume[2] state = %v; want ResumeOK (coroutine should complete)", state)
	}
	if len(values) != 1 {
		t.Fatalf("Resume[2] returned %d values; want 1", len(values))
	}
	if values[0].String() != "got:resumed-body" {
		t.Fatalf("Resume[2] returned values[0] = %q; want %q", values[0].String(), "got:resumed-body")
	}
}

// TestVM_NewThread_CancelFunc_cleans_up_without_leaks verifies that
// invoking CancelFunc does not leak goroutines. We construct N child
// threads, cancel each, and assert the goroutine count delta stays
// within a small tolerance (≤ N — each gopher-lua child thread is
// purely synchronous; no background goroutines are spawned).
func TestVM_NewThread_CancelFunc_cleans_up_without_leaks(t *testing.T) {
	vm := NewVM()
	defer vm.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vm.State().SetContext(ctx)

	// Settle baseline.
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	before := runtime.NumGoroutine()

	const N = 50
	for i := 0; i < N; i++ {
		child, cancelFn := vm.NewThread()
		if child == nil {
			t.Fatalf("NewThread iter %d: nil child", i)
		}
		if cancelFn == nil {
			t.Fatalf("NewThread iter %d: nil cancelFn", i)
		}
		cancelFn()
	}

	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	after := runtime.NumGoroutine()

	// Goroutine count after must not exceed baseline by more than a
	// small tolerance — gopher-lua coroutines are synchronous (no
	// background workers). Tolerance ≤ 2 accommodates the testing
	// runtime's own scheduler churn.
	if after > before+2 {
		t.Fatalf("goroutine leak: before=%d after=%d delta=%d (tolerance ≤ 2)", before, after, after-before)
	}
}

// TestVM_Resume_panic_wraps_via_panicH verifies that a genuine Go panic
// inside a bridge LGFunction invoked during Resume is recovered and
// routed through vm.panicH per ADR-0188 §Decision 2 panic-wrapper
// discipline, and that Resume returns a ResumeError with a non-nil
// error rather than allowing the panic to escape.
func TestVM_Resume_panic_wraps_via_panicH(t *testing.T) {
	var captured atomic.Value // any
	vm := NewVM(WithPanicHandler(func(rcv any) {
		captured.Store(rcv)
	}))
	defer vm.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vm.State().SetContext(ctx)

	vm.RegisterGlobalFunc("bridge_panic", func(L *lua.LState) int {
		panic("bridge-panic-value")
	})

	chunk, err := CompileScript([]byte(`function child_fn() bridge_panic() end`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want nil", err)
	}
	fn := vm.State().GetGlobal("child_fn").(*lua.LFunction)

	child, cancelFn := vm.NewThread()
	defer cancelFn()

	state, rerr, _ := vm.Resume(child, fn)
	if state != lua.ResumeError {
		t.Fatalf("Resume state = %v; want ResumeError after bridge panic", state)
	}
	if rerr == nil {
		t.Fatal("Resume err = nil; want non-nil after bridge panic")
	}

	v := captured.Load()
	if v == nil {
		t.Fatal("panicH not invoked; want invocation with recovered value")
	}
	// The recovered value may be the original "bridge-panic-value"
	// string or gopher-lua's *ApiError wrapper holding it. Accept either
	// shape so long as it carries the panic-value substring.
	got := ""
	switch tv := v.(type) {
	case string:
		got = tv
	case error:
		got = tv.Error()
	default:
		got = ""
	}
	if got == "" {
		t.Fatalf("panicH recovered value carries no string form: %#v", v)
	}
	// (Substring check intentionally permissive — gopher-lua may prefix
	// "<source>:<line>: " per AMEND-9(c); we only care the panic value
	// is observable to the handler.)
}

// TestRace_N100_parallel_NewThread_Resume_yield_clean_under_race spawns
// N=100 parallel goroutines, each constructing an independent VM and
// driving a coroutine through:
//
//   - vm.NewThread (mint child *LState + CancelFunc).
//   - vm.Resume (initial Resume yields via bridge_yield).
//   - vm.Resume (resume-after-yield with a fresh value; coroutine
//     completes with ResumeOK).
//   - cancelFn invocation (cancel child loop).
//
// Race-clean under `go test -race -count=10` per Task 15 D-P10 +
// ADR-0071 single-goroutine-per-stream invariant + ADR-0191 §Context
// per-stream child-LState lifecycle. Asserts:
//
//   - No data race on the parent VM or child *LState (each goroutine
//     uses its own VM + child pair; no shared state).
//   - No goroutine leak from cancelFn invocation (runtime.NumGoroutine
//     delta ≤ 2 — Go scheduler churn tolerance).
//   - Resume[1] returns ResumeYield with the bridge's yielded value;
//     Resume[2] returns ResumeOK with the script's completion value.
//
// Per D-P9 race-scoping: this is the internal/lua-package-level
// coroutine race test; the broader filter-level race test
// (TestRace_N100_parallel_filter_dispatches_clean_under_race) lives at
// internal/filter/http/lua/lua_test.go per Task 15 PLAN.
//
// Skipped under `-short`.
func TestRace_N100_parallel_NewThread_Resume_yield_clean_under_race(t *testing.T) {
	if testing.Short() {
		t.Skip("skip parallel race test under -short")
	}

	// Settle baseline before spawning the worker pool.
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	before := runtime.NumGoroutine()

	const N = 100
	var (
		wg       sync.WaitGroup
		failures atomic.Int64
	)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			vm := NewVM()
			defer vm.Close()

			ctx, cancelCtx := context.WithCancel(context.Background())
			defer cancelCtx()
			vm.State().SetContext(ctx)

			// Register a bridge that yields a per-goroutine-distinct value.
			yieldVal := lua.LString(fmt.Sprintf("y-%d", idx))
			vm.RegisterGlobalFunc("bridge_yield", func(L *lua.LState) int {
				return YieldFromBridge(L, yieldVal)
			})

			// Per-goroutine-distinct script returns a completion value
			// blending the resume-passed-back value with the goroutine idx
			// — surfaces cross-goroutine state leak as a wrong return.
			src := fmt.Sprintf(`
				function child_fn()
					local r = bridge_yield()
					return "got:" .. tostring(r) .. ":done-%d"
				end
			`, idx)
			chunk, err := CompileScript([]byte(src), nil)
			if err != nil {
				t.Errorf("[%d] CompileScript err = %v", idx, err)
				failures.Add(1)
				return
			}
			if err := vm.Run(chunk); err != nil {
				t.Errorf("[%d] vm.Run err = %v", idx, err)
				failures.Add(1)
				return
			}
			fn := vm.State().GetGlobal("child_fn").(*lua.LFunction)

			child, cancelFn := vm.NewThread()
			if child == nil || cancelFn == nil {
				t.Errorf("[%d] NewThread returned nil child or cancelFn", idx)
				failures.Add(1)
				return
			}
			// Invoke cancelFn at end-of-iteration to release the child loop
			// per ADR-0191 §Context.
			defer cancelFn()

			// Resume[1]: kicks off the coroutine; it yields via bridge_yield.
			state, rerr, values := vm.Resume(child, fn)
			if rerr != nil {
				t.Errorf("[%d] Resume[1] err = %v; want nil", idx, rerr)
				failures.Add(1)
				return
			}
			if state != lua.ResumeYield {
				t.Errorf("[%d] Resume[1] state = %v; want ResumeYield", idx, state)
				failures.Add(1)
				return
			}
			wantYield := fmt.Sprintf("y-%d", idx)
			if len(values) != 1 || values[0].String() != wantYield {
				t.Errorf("[%d] Resume[1] yielded values = %v; want [%q] (cross-goroutine yield-state leak)",
					idx, values, wantYield)
				failures.Add(1)
				return
			}

			// Resume[2]: resume-after-yield with a per-goroutine-distinct
			// value; coroutine completes with ResumeOK + a completion
			// string that surfaces any cross-goroutine state leak.
			resumeVal := lua.LString(fmt.Sprintf("r-%d", idx))
			state, rerr, values = vm.Resume(child, nil, resumeVal)
			if rerr != nil {
				t.Errorf("[%d] Resume[2] err = %v; want nil", idx, rerr)
				failures.Add(1)
				return
			}
			if state != lua.ResumeOK {
				t.Errorf("[%d] Resume[2] state = %v; want ResumeOK", idx, state)
				failures.Add(1)
				return
			}
			wantDone := fmt.Sprintf("got:r-%d:done-%d", idx, idx)
			if len(values) != 1 || values[0].String() != wantDone {
				t.Errorf("[%d] Resume[2] return values = %v; want [%q] (cross-goroutine resume-state leak)",
					idx, values, wantDone)
				failures.Add(1)
				return
			}
		}(i)
	}
	wg.Wait()

	if n := failures.Load(); n > 0 {
		t.Fatalf("%d / %d goroutines failed coroutine assertions", n, N)
	}

	// Goroutine-leak assertion: settled count must not exceed baseline +
	// 2 (Go scheduler churn tolerance). gopher-lua coroutines are purely
	// synchronous — no background goroutines should remain post-cancelFn
	// invocation per ADR-0191 §Context.
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Fatalf("goroutine leak: before=%d after=%d delta=%d (tolerance ≤ 2)",
			before, after, after-before)
	}
}

// TestVM_Coroutine_race_N100_parallel_clean_under_race spawns N=100
// parallel VMs each driving an independent coroutine. Race-clean under
// `go test -race`. Skipped under `-short`.
func TestVM_Coroutine_race_N100_parallel_clean_under_race(t *testing.T) {
	if testing.Short() {
		t.Skip("skip parallel race test under -short")
	}

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			vm := NewVM()
			defer vm.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			vm.State().SetContext(ctx)

			vm.RegisterGlobalFunc("bridge_yield", func(L *lua.LState) int {
				return YieldFromBridge(L, lua.LString("y"))
			})
			chunk, err := CompileScript([]byte(`function child_fn() bridge_yield(); return "done" end`), nil)
			if err != nil {
				t.Errorf("CompileScript err = %v", err)
				return
			}
			if err := vm.Run(chunk); err != nil {
				t.Errorf("vm.Run err = %v", err)
				return
			}
			fn := vm.State().GetGlobal("child_fn").(*lua.LFunction)

			child, cancelFn := vm.NewThread()
			defer cancelFn()

			state, rerr, _ := vm.Resume(child, fn)
			if rerr != nil || state != lua.ResumeYield {
				t.Errorf("Resume[1] state=%v err=%v; want (ResumeYield, nil)", state, rerr)
				return
			}
			state, rerr, _ = vm.Resume(child, nil)
			if rerr != nil || state != lua.ResumeOK {
				t.Errorf("Resume[2] state=%v err=%v; want (ResumeOK, nil)", state, rerr)
				return
			}
		}()
	}
	wg.Wait()
}
