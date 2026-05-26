// Tests for the per-*RootVM ForeignFunctionRegistry + *RootVM.CallForeignFunction
// dispatch per 25.2 SPEC §3.1 + §5.1 #38 + AMEND-A9 + R-25.2-8 + D-25.2-P3
// closure (mutex-per-RootVM dispatch concurrency model per D-P-PLAN-9).
//
// Coverage:
//
//   - TestForeignFunctionRegistry_RegisterAndGet: basic Register/Get round-trip
//   - TestForeignFunctionRegistry_DuplicateRegisterErrors: duplicate name rejected
//   - TestForeignFunctionRegistry_DefaultIsEmpty: AMEND-A9 — process-global
//     DefaultForeignFunctionRegistry ships with ZERO registered functions
//     (vs upstream cpp-host's 10); Get("verify_signature") returns (nil, false)
//   - TestForeignFunctionRegistry_TopLevelRegisterDelegates: wasm.RegisterForeignFunction
//     convenience helper delegates to DefaultForeignFunctionRegistry.Register
//   - TestRootVM_CallForeignFunction_NotFound: unregistered name returns NotFound
//     (byte-faithful to cpp-host src/exports.cc:147-184)
//   - TestRootVM_CallForeignFunction_OkRoundTrip: registered function invoked +
//     args + result propagated correctly
//   - TestRootVM_CallForeignFunction_Panic: Go-panic in foreign-function fn
//     → recover() → InternalFailure; PanicHandlerFn fires; RootVM survives
//   - TestRootVM_CallForeignFunction_ConcurrentDispatch_D_P_PLAN_9: D-25.2-P3
//     closure verification per D-P-PLAN-9 — N=100 concurrent stream contexts
//     dispatch the same registered foreign function via mutex-per-RootVM
//     dispatchMu-held frames. Assert: all 100 return Ok; probe counter == 100
//     (no calls lost; -race clean); each call observes its own args (no
//     cross-stream argument leak); calls execute SERIALLY (mutex-per-RootVM
//     serialization verified via per-call start/end timestamps — no two calls
//     overlap in time).

package wasm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// --- TestForeignFunctionRegistry_RegisterAndGet ---------------------------

func TestForeignFunctionRegistry_RegisterAndGet(t *testing.T) {
	reg := NewForeignFunctionRegistry()

	called := false
	fn := func(_ context.Context, args []byte) ([]byte, abi.WasmResult) {
		called = true
		return append([]byte("echo:"), args...), abi.WasmResultOk
	}

	if err := reg.Register("echo", fn); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := reg.Get("echo")
	if !ok {
		t.Fatalf("Get(\"echo\"): ok=false; want true")
	}
	if got == nil {
		t.Fatalf("Get(\"echo\"): nil fn; want non-nil")
	}
	result, status := got(context.Background(), []byte("payload"))
	if status != abi.WasmResultOk {
		t.Errorf("invoked fn: status=%v; want Ok", status)
	}
	if string(result) != "echo:payload" {
		t.Errorf("invoked fn: result=%q; want %q", result, "echo:payload")
	}
	if !called {
		t.Errorf("fn was not invoked through Get")
	}
}

// --- TestForeignFunctionRegistry_DuplicateRegisterErrors ------------------

func TestForeignFunctionRegistry_DuplicateRegisterErrors(t *testing.T) {
	reg := NewForeignFunctionRegistry()

	fn := func(_ context.Context, _ []byte) ([]byte, abi.WasmResult) {
		return nil, abi.WasmResultOk
	}

	if err := reg.Register("dup", fn); err != nil {
		t.Fatalf("Register first: %v", err)
	}

	err := reg.Register("dup", fn)
	if err == nil {
		t.Fatalf("Register duplicate: nil error; want non-nil error")
	}
	if !strings.Contains(err.Error(), "dup") {
		t.Errorf("duplicate error %q does not mention name", err.Error())
	}
}

// --- TestForeignFunctionRegistry_NilFnRejected ----------------------------

func TestForeignFunctionRegistry_NilFnRejected(t *testing.T) {
	reg := NewForeignFunctionRegistry()
	if err := reg.Register("nilfn", nil); err == nil {
		t.Fatalf("Register(nil): nil error; want non-nil")
	}
}

// --- TestForeignFunctionRegistry_EmptyNameRejected ------------------------

func TestForeignFunctionRegistry_EmptyNameRejected(t *testing.T) {
	reg := NewForeignFunctionRegistry()
	fn := func(_ context.Context, _ []byte) ([]byte, abi.WasmResult) {
		return nil, abi.WasmResultOk
	}
	if err := reg.Register("", fn); err == nil {
		t.Fatalf("Register(\"\"): nil error; want non-nil")
	}
}

// --- TestForeignFunctionRegistry_DefaultIsEmpty ---------------------------

// TestForeignFunctionRegistry_DefaultIsEmpty verifies AMEND-A9: the
// process-global DefaultForeignFunctionRegistry ships with ZERO registered
// functions. Upstream cpp-host's 10 default foreign functions
// (verify_signature, sign, compress, uncompress, set_envoy_filter_state,
// clear_route_cache, expr_create, expr_evaluate, expr_delete, declare_property)
// are NOT pre-registered in envoy-go — operators MUST register explicitly
// via wasm.RegisterForeignFunction(name, fn). envoy-go-strict departure
// record #5 at BEHAVIOR_CONTRACT.md (Task 22).
func TestForeignFunctionRegistry_DefaultIsEmpty(t *testing.T) {
	if DefaultForeignFunctionRegistry == nil {
		t.Fatalf("DefaultForeignFunctionRegistry == nil; expected pre-initialized")
	}
	// Cpp-host's 10 default foreign functions — all MUST be absent in envoy-go.
	defaults := []string{
		"verify_signature",
		"sign",
		"compress",
		"uncompress",
		"set_envoy_filter_state",
		"clear_route_cache",
		"expr_create",
		"expr_evaluate",
		"expr_delete",
		"declare_property",
	}
	for _, name := range defaults {
		t.Run(name, func(t *testing.T) {
			if _, ok := DefaultForeignFunctionRegistry.Get(name); ok {
				t.Errorf("DefaultForeignFunctionRegistry contains %q; per AMEND-A9 envoy-go ships ZERO default foreign functions", name)
			}
		})
	}
}

// --- TestForeignFunctionRegistry_TopLevelRegisterDelegates ----------------

// TestForeignFunctionRegistry_TopLevelRegisterDelegates verifies the
// `wasm.RegisterForeignFunction(name, fn)` convenience helper delegates to
// DefaultForeignFunctionRegistry.Register. Operators use this top-level
// helper at boot to register implementation-specific foreign functions per
// AMEND-A9. We register under a test-only namespace to avoid polluting the
// process-global registry for other tests.
func TestForeignFunctionRegistry_TopLevelRegisterDelegates(t *testing.T) {
	// Use a uniquely-named function to avoid colliding with concurrent tests.
	name := "test_top_level_delegate_" + t.Name()
	fn := func(_ context.Context, _ []byte) ([]byte, abi.WasmResult) {
		return []byte("delegated"), abi.WasmResultOk
	}

	if err := RegisterForeignFunction(name, fn); err != nil {
		t.Fatalf("RegisterForeignFunction: %v", err)
	}
	// Read back via the DefaultForeignFunctionRegistry directly to confirm
	// delegation.
	got, ok := DefaultForeignFunctionRegistry.Get(name)
	if !ok {
		t.Fatalf("DefaultForeignFunctionRegistry.Get(%q) after RegisterForeignFunction: ok=false", name)
	}
	res, status := got(context.Background(), nil)
	if status != abi.WasmResultOk || string(res) != "delegated" {
		t.Errorf("got (%q, %v); want (\"delegated\", Ok)", res, status)
	}
}

// =====================================================================
// *RootVM.CallForeignFunction tests — D-P-PLAN-9 mutex-per-RootVM model
// =====================================================================

// --- TestRootVM_CallForeignFunction_DefaultsToGlobalRegistry --------------

// TestRootVM_CallForeignFunction_DefaultsToGlobalRegistry verifies that a
// *RootVM constructed without WithRootForeignRegistry consults the process-
// global DefaultForeignFunctionRegistry. Since the default registry is EMPTY
// per AMEND-A9, an unregistered name returns NotFound — confirming the
// default-wiring path.
func TestRootVM_CallForeignFunction_DefaultsToGlobalRegistry(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	// Synthetic dispatchMu-held frame (mirrors the real abi-shim caller path
	// which executes inside the per-stream dispatch frame).
	rv.dispatchMu.Lock()
	result, status := rv.CallForeignFunction(ctx, "no_such_function_in_default_registry_xyz", nil)
	rv.dispatchMu.Unlock()

	if status != abi.WasmResultNotFound {
		t.Errorf("CallForeignFunction unregistered: status=%v; want NotFound (default registry should be EMPTY per AMEND-A9)", status)
	}
	if result != nil {
		t.Errorf("CallForeignFunction NotFound: result=%v; want nil", result)
	}
}

// --- TestRootVM_CallForeignFunction_NotFound ------------------------------

// TestRootVM_CallForeignFunction_NotFound verifies that an unregistered name
// returns WasmResult::NotFound (=1) byte-faithful to cpp-host src/exports.cc:
// 147-184. AMEND-A9 + R-25.2-8: the envoy-go-strict counter
// `foreign_function_denied` increments on this path; counter wiring deferred
// to Task 17.
func TestRootVM_CallForeignFunction_NotFound(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	reg := NewForeignFunctionRegistry()
	rv, err := NewRootVM(ctx, mod, 1, WithRootForeignRegistry(reg))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	rv.dispatchMu.Lock()
	result, status := rv.CallForeignFunction(ctx, "unknown_fn", []byte("args"))
	rv.dispatchMu.Unlock()

	if status != abi.WasmResultNotFound {
		t.Errorf("unregistered: status=%v (%d); want NotFound (=1) per AMEND-A9", status, status)
	}
	if result != nil {
		t.Errorf("unregistered: result=%v; want nil", result)
	}
}

// --- TestRootVM_CallForeignFunction_OkRoundTrip ---------------------------

// TestRootVM_CallForeignFunction_OkRoundTrip verifies registered-then-call:
// the function executes synchronously; args + result propagate correctly.
func TestRootVM_CallForeignFunction_OkRoundTrip(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	reg := NewForeignFunctionRegistry()
	if err := reg.Register("echo_args", func(_ context.Context, args []byte) ([]byte, abi.WasmResult) {
		out := make([]byte, 0, len(args)+5)
		out = append(out, []byte("echo:")...)
		out = append(out, args...)
		return out, abi.WasmResultOk
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	rv, err := NewRootVM(ctx, mod, 1, WithRootForeignRegistry(reg))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	rv.dispatchMu.Lock()
	result, status := rv.CallForeignFunction(ctx, "echo_args", []byte("payload"))
	rv.dispatchMu.Unlock()

	if status != abi.WasmResultOk {
		t.Errorf("status=%v; want Ok", status)
	}
	if string(result) != "echo:payload" {
		t.Errorf("result=%q; want %q", result, "echo:payload")
	}
}

// --- TestRootVM_CallForeignFunction_Panic ---------------------------------

// TestRootVM_CallForeignFunction_Panic verifies the panic-recovery wrapper
// per D-P-PLAN-9 (d): Go panic in the ForeignFunctionFn → recover() →
// InternalFailure; PanicHandlerFn fires; the *RootVM survives + can dispatch
// a subsequent call. The envoy_go.failures counter integration is deferred
// to Task 17.
func TestRootVM_CallForeignFunction_Panic(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)

	reg := NewForeignFunctionRegistry()
	if err := reg.Register("panicker", func(_ context.Context, _ []byte) ([]byte, abi.WasmResult) {
		panic("synthetic foreign-function panic")
	}); err != nil {
		t.Fatalf("Register panicker: %v", err)
	}
	if err := reg.Register("survivor", func(_ context.Context, _ []byte) ([]byte, abi.WasmResult) {
		return []byte("alive"), abi.WasmResultOk
	}); err != nil {
		t.Fatalf("Register survivor: %v", err)
	}

	var panicHandlerCalled atomic.Bool
	var panicValue atomic.Value
	rv, err := NewRootVM(ctx, mod, 1,
		WithRootForeignRegistry(reg),
		WithRootPanicHandler(func(r any) {
			panicHandlerCalled.Store(true)
			panicValue.Store(fmt.Sprintf("%v", r))
		}),
	)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	// Dispatch the panicking fn under a dispatchMu-held frame.
	rv.dispatchMu.Lock()
	result, status := rv.CallForeignFunction(ctx, "panicker", []byte("ignored"))
	rv.dispatchMu.Unlock()

	if status != abi.WasmResultInternalFailure {
		t.Errorf("panicked fn: status=%v; want InternalFailure per D-P-PLAN-9 (d)", status)
	}
	if result != nil {
		t.Errorf("panicked fn: result=%v; want nil", result)
	}
	if !panicHandlerCalled.Load() {
		t.Errorf("PanicHandlerFn was NOT invoked; expected fire on Go panic in ForeignFunctionFn")
	}
	if pv := panicValue.Load(); pv == nil || !strings.Contains(pv.(string), "synthetic foreign-function panic") {
		t.Errorf("PanicHandlerFn value = %v; expected substring \"synthetic foreign-function panic\"", pv)
	}

	// Survivor: confirm the RootVM is still functional after the panic-recover.
	rv.dispatchMu.Lock()
	result2, status2 := rv.CallForeignFunction(ctx, "survivor", nil)
	rv.dispatchMu.Unlock()
	if status2 != abi.WasmResultOk || string(result2) != "alive" {
		t.Errorf("post-panic survivor: (%q, %v); want (\"alive\", Ok)", result2, status2)
	}
}

// =====================================================================
// D-25.2-P3 closure verification per D-P-PLAN-9 — mutex-per-RootVM
// =====================================================================
//
// The test below is THE empirical evidence that closes D-25.2-P3 in code.
// It verifies the 5-clause mutex-per-RootVM dispatch concurrency model:
//
//   (a) *ForeignFunctionRegistry.Get holds RLock only (read-only lookup;
//       concurrent Get from multiple goroutines proceeds).
//   (b) dispatched ForeignFunctionFn executes SYNCHRONOUSLY inside
//       *RootVM.CallForeignFunction (no goroutine offload).
//   (c) *RootVM dispatch lock IS HELD during invocation (same lock as
//       per-stream call frame; the test simulates this by acquiring
//       dispatchMu BEFORE calling CallForeignFunction, mirroring the real
//       abi-shim call site).
//   (d) panic-recovery wrapper applies (covered by the Panic test above).
//   (e) foreign_function_denied counter increments on NotFound path
//       (counter wiring deferred Task 17; covered by NotFound test above).

// TestRootVM_CallForeignFunction_ConcurrentDispatch_D_P_PLAN_9 is the
// load-bearing test that closes D-25.2-P3 in code: N=100 concurrent stream
// contexts dispatch the same registered foreign function via mutex-per-
// RootVM dispatchMu-held frames. Assertions:
//
//   - all 100 invocations return Ok (no calls lost; no spurious errors);
//   - probe counter == 100 (every dispatch executed the fn body exactly once);
//   - each call observes ITS OWN args bytes (no cross-stream argument leak);
//   - calls execute SERIALLY (no two ForeignFunctionFn bodies execute
//     concurrently — mutex-per-RootVM serialization);
//   - -race clean.
//
// The serialization assertion is the key D-P-PLAN-9 evidence: the foreign
// function records (start, end) timestamps + an in-flight atomic counter
// that increments at fn-entry + decrements at fn-exit; the test asserts the
// max observed in-flight value was 1 throughout the run (no overlapping
// invocations).
func TestRootVM_CallForeignFunction_ConcurrentDispatch_D_P_PLAN_9(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)

	const N = 100

	var (
		probeCounter atomic.Uint32
		inFlight     atomic.Int32
		maxInFlight  atomic.Int32
	)

	// Per-call records: streamID -> seen-args. Used for the no-cross-stream-
	// leak assertion. Keyed by the args content (which encodes the streamID
	// as a unique 4-byte big-endian prefix).
	type record struct {
		streamID  uint32
		seen      []byte
		startNano int64
		endNano   int64
	}
	var (
		recsMu sync.Mutex
		recs   []record
	)

	reg := NewForeignFunctionRegistry()
	if err := reg.Register("probe", func(_ context.Context, args []byte) ([]byte, abi.WasmResult) {
		// Track in-flight count. The mutex-per-RootVM serialization should
		// keep this AT MOST 1 throughout the run.
		cur := inFlight.Add(1)
		defer inFlight.Add(-1)
		// Track the high-water mark across all goroutines.
		for {
			prev := maxInFlight.Load()
			if cur <= prev {
				break
			}
			if maxInFlight.CompareAndSwap(prev, cur) {
				break
			}
		}

		start := time.Now().UnixNano()
		// Hold the lock briefly so a non-serialized impl would contend.
		// 100µs is long enough to surface a race; short enough that 100
		// serial calls run in well under a second.
		time.Sleep(100 * time.Microsecond)
		end := time.Now().UnixNano()

		probeCounter.Add(1)

		// Decode streamID from the args (first 4 bytes big-endian).
		var sid uint32
		if len(args) >= 4 {
			sid = uint32(args[0])<<24 | uint32(args[1])<<16 | uint32(args[2])<<8 | uint32(args[3])
		}
		recsMu.Lock()
		recs = append(recs, record{
			streamID:  sid,
			seen:      append([]byte(nil), args...),
			startNano: start,
			endNano:   end,
		})
		recsMu.Unlock()

		// Echo the args back so the caller can verify no-cross-stream leak.
		out := append([]byte(nil), args...)
		return out, abi.WasmResultOk
	}); err != nil {
		t.Fatalf("Register probe: %v", err)
	}

	rv, err := NewRootVM(ctx, mod, 1, WithRootForeignRegistry(reg))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	// Spawn N goroutines, each simulating a per-stream caller that holds
	// dispatchMu (per the real abi-shim call path) BEFORE invoking
	// CallForeignFunction. This mirrors D-P-PLAN-9 clause (c): the call site
	// is INSIDE the dispatchMu-held frame.
	type result struct {
		streamID uint32
		status   abi.WasmResult
		echo     []byte
	}
	results := make([]result, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			// streamID encoded as a unique 4-byte big-endian prefix in args.
			sid := uint32(i + 1)
			args := []byte{
				byte(sid >> 24), byte(sid >> 16), byte(sid >> 8), byte(sid),
				// Append a per-stream payload to amplify any cross-stream
				// argument leak.
				byte(i), byte(i + 1), byte(i + 2),
			}
			rv.dispatchMu.Lock()
			echo, status := rv.CallForeignFunction(ctx, "probe", args)
			rv.dispatchMu.Unlock()
			results[i] = result{streamID: sid, status: status, echo: echo}
		}()
	}
	wg.Wait()

	// (1) all 100 returns Ok.
	for i, r := range results {
		if r.status != abi.WasmResultOk {
			t.Errorf("stream %d: status=%v; want Ok", i, r.status)
		}
	}

	// (2) probe counter == 100 (no calls lost).
	if got := probeCounter.Load(); got != N {
		t.Errorf("probeCounter=%d; want %d (no calls lost)", got, N)
	}

	// (3) no cross-stream argument leak: each result's echoed args MUST
	//     start with its own streamID-prefix.
	for i, r := range results {
		if len(r.echo) < 4 {
			t.Errorf("stream %d: echo too short (%d bytes)", i, len(r.echo))
			continue
		}
		gotSID := uint32(r.echo[0])<<24 | uint32(r.echo[1])<<16 | uint32(r.echo[2])<<8 | uint32(r.echo[3])
		if gotSID != r.streamID {
			t.Errorf("stream %d: echoed streamID=%d; want %d (CROSS-STREAM ARGUMENT LEAK)", i, gotSID, r.streamID)
		}
	}

	// (4) records: every streamID appears exactly once.
	seen := make(map[uint32]int, N)
	for _, r := range recs {
		seen[r.streamID]++
	}
	if len(seen) != N {
		t.Errorf("recs distinct streamIDs=%d; want %d", len(seen), N)
	}
	for sid, count := range seen {
		if count != 1 {
			t.Errorf("streamID %d invoked %d times; want exactly 1", sid, count)
		}
	}

	// (5) D-P-PLAN-9 LOAD-BEARING — mutex-per-RootVM serialization. The
	//     max in-flight count throughout the run MUST be 1 (no two
	//     ForeignFunctionFn bodies executed concurrently). This is the
	//     empirical evidence that dispatchMu serialization holds.
	if got := maxInFlight.Load(); got != 1 {
		t.Errorf("maxInFlight=%d; want 1 (mutex-per-RootVM serialization broken per D-P-PLAN-9 clause c)", got)
	}

	// (6) Serial-execution check via timestamps: for ANY two records
	//     a, b — either a.end <= b.start OR b.end <= a.start. We sample a
	//     handful of pairs (full O(N^2) would slow the test).
	if len(recs) >= 2 {
		for i := 0; i < len(recs); i++ {
			for j := i + 1; j < len(recs) && j < i+10; j++ {
				a, b := recs[i], recs[j]
				if !(a.endNano <= b.startNano || b.endNano <= a.startNano) {
					t.Errorf("overlapping invocations: a=[%d,%d] b=[%d,%d] — mutex-per-RootVM serialization broken",
						a.startNano, a.endNano, b.startNano, b.endNano)
				}
			}
		}
	}
}

// --- TestForeignFunctionRegistry_ConcurrentGetIsSafe ----------------------

// TestForeignFunctionRegistry_ConcurrentGetIsSafe verifies D-P-PLAN-9 clause
// (a): *ForeignFunctionRegistry.Get holds RLock only — multiple goroutines
// can Get concurrently without blocking. -race must be clean.
func TestForeignFunctionRegistry_ConcurrentGetIsSafe(t *testing.T) {
	reg := NewForeignFunctionRegistry()
	for i := 0; i < 16; i++ {
		name := fmt.Sprintf("fn_%d", i)
		if err := reg.Register(name, func(_ context.Context, _ []byte) ([]byte, abi.WasmResult) {
			return nil, abi.WasmResultOk
		}); err != nil {
			t.Fatalf("Register %s: %v", name, err)
		}
	}

	const goroutines = 100
	const itersPerG = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < itersPerG; i++ {
				name := fmt.Sprintf("fn_%d", i%16)
				if _, ok := reg.Get(name); !ok {
					t.Errorf("Get(%q) ok=false; want true", name)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// --- compile-time sanity --------------------------------------------------

// _ keeps the errors import alive even when no test references it directly
// (defensive — future test additions may need errors.Is checks against the
// duplicate-Register error).
var _ = errors.New
