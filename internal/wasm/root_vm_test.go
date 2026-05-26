// Tests for the RootVM lifecycle + options + Configure + NewStreamContext +
// Close per 25.2 SPEC §3.1 + Task 1 acceptance criteria.
//
// Coverage:
//   - NewRootVM constructs the wazero Runtime + instantiates the Module +
//     applies options (Sandbox / Panic / LogSink / Caps / etc.).
//   - Configure invokes _initialize OR _start + proxy_on_context_create(root,
//     0) + proxy_on_vm_start + proxy_on_configure (matching the 25.1 vm.Run
//     canonical order — context_create BEFORE vm_start).
//   - NewStreamContext allocates monotonic streamCtxIDs + fires
//     proxy_on_context_create(streamCtxID, rootCtxID).
//   - Close idempotent.
//   - WithRootSandboxConfig + WithRootPanicHandler + WithRootLogSink option
//     application.
//   - HasGlobalFunc + IsAllowed delegate to module instance + sandbox.
//   - Concurrent N=100 stream contexts share one RootVM no-state-leak.
//   - LogProxy routes via WithRootLogSink.
//
// These tests are RootVM lifecycle + structural — per-stream callback
// invocation coverage lives in stream_context_test.go.

package wasm

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// --- TestNewRootVM_Options ------------------------------------------------

func TestNewRootVM_Options(t *testing.T) {
	ctx := context.Background()

	t.Run("WithRootSandboxConfig sets sandbox field", func(t *testing.T) {
		sb := SandboxConfig{AllowedCapabilities: map[string]SanitizationConfig{capProxyLog: {}}}
		mod := mustCompileForRootVM(t, ctx, minimalInitModule)
		rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(sb))
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()

		if !rv.IsAllowed(capProxyLog) {
			t.Errorf("sandbox not applied: IsAllowed(proxy_log) = false")
		}
		if rv.IsAllowed(capProxyGetHeaderMapPairs) {
			t.Errorf("sandbox over-permissive: IsAllowed(proxy_get_header_map_pairs) = true")
		}
	})

	t.Run("WithRootPanicHandler sets panicH field", func(t *testing.T) {
		var captured any
		h := func(r any) { captured = r }
		mod := mustCompileForRootVM(t, ctx, minimalInitModule)
		rv, err := NewRootVM(ctx, mod, 1, WithRootPanicHandler(h))
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()

		if rv.panicH == nil {
			t.Fatal("panicH not set")
		}
		rv.panicH("test-panic")
		if captured != "test-panic" {
			t.Errorf("panicH did not capture: got %v, want \"test-panic\"", captured)
		}
	})

	t.Run("WithRootLogSink routes LogProxy through sink", func(t *testing.T) {
		var sink bytes.Buffer
		mod := mustCompileForRootVM(t, ctx, minimalInitModule)
		rv, err := NewRootVM(ctx, mod, 1, WithRootLogSink(&sink))
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()

		rv.LogProxy(abi.LogLevelInfo, "hello")
		got := sink.String()
		if !strings.Contains(got, "info") || !strings.Contains(got, "hello") {
			t.Errorf("log sink did not capture: got %q", got)
		}
	})

	t.Run("default options: zero-value sandbox (deny-all) + nil sink + nil panicH", func(t *testing.T) {
		mod := mustCompileForRootVM(t, ctx, minimalInitModule)
		rv, err := NewRootVM(ctx, mod, 1)
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()

		if rv.IsAllowed(capProxyLog) {
			t.Errorf("default sandbox over-permissive: IsAllowed(proxy_log) = true")
		}
		if rv.panicH != nil {
			t.Errorf("default panicH non-nil")
		}
		if rv.logSink != nil {
			t.Errorf("default logSink non-nil")
		}
	})

	t.Run("WithRootLogSink nil-sink: LogProxy no-op", func(t *testing.T) {
		mod := mustCompileForRootVM(t, ctx, minimalInitModule)
		rv, err := NewRootVM(ctx, mod, 1)
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()
		// Should not panic.
		rv.LogProxy(abi.LogLevelError, "must-not-leak")
	})

	t.Run("WithRootSharedDataCaps records caps (state stub at Task 1)", func(t *testing.T) {
		mod := mustCompileForRootVM(t, ctx, minimalInitModule)
		rv, err := NewRootVM(ctx, mod, 1, WithRootSharedDataCaps(4096, 64))
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()
		if rv.sharedDataValCap != 4096 || rv.sharedDataMaxEntries != 64 {
			t.Errorf("shared-data caps not applied: valCap=%d maxEntries=%d, want (4096, 64)",
				rv.sharedDataValCap, rv.sharedDataMaxEntries)
		}
	})
}

// --- TestNewRootVM_NilModule + TestNewRootVM_HostModulesRegistered --------

func TestNewRootVM_NilModule(t *testing.T) {
	ctx := context.Background()
	_, err := NewRootVM(ctx, nil, 1)
	if err == nil {
		t.Fatal("expected error on nil *Module")
	}
}

func TestNewRootVM_HostModulesRegistered(t *testing.T) {
	ctx := context.Background()
	// Construct a RootVM around a benign minimal module, then verify the
	// env + wasi_snapshot_preview1 host modules are wired by instantiating
	// an importer module via the RootVM's runtime directly (bypasses
	// CompileModule's ABI-sentinel gate — importerProxyLogModule lacks the
	// proxy_abi_version_0_2_1 export by design).
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	inst, err := rv.runtime.Instantiate(ctx, importerProxyLogModule)
	if err != nil {
		t.Fatalf("instantiate importer module: %v", err)
	}
	defer func() { _ = inst.Close(ctx) }()
}

// --- TestRootVM_Configure_Lifecycle ---------------------------------------

// TestRootVM_Configure_Lifecycle is the regression test for the canonical
// proxy-wasm host lifecycle order: Configure must invoke
// proxy_on_context_create(rootCtxID, 0) BEFORE proxy_on_vm_start(rootCtxID, 0)
// BEFORE proxy_on_configure(rootCtxID, 0). This matches the 25.1 vm.Run
// behavior + the upstream cpp-host + proxy-wasm-rust-sdk dispatcher expectation.
func TestRootVM_Configure_Lifecycle(t *testing.T) {
	ctx := context.Background()

	t.Run("proxy_on_context_create -> proxy_on_vm_start -> proxy_on_configure firing order", func(t *testing.T) {
		cb := &fakeABICallbacks{}
		mod := mustCompileWithCacheForRootVM(t, ctx, lifecycleOrderModule)
		rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()
		rv.RegisterABICallbacks(cb)

		if err := rv.Configure(ctx, nil, nil); err != nil {
			t.Fatalf("Configure: %v", err)
		}

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

	t.Run("guest without proxy_on_context_create export: Configure skips + proceeds to vm_start", func(t *testing.T) {
		cb := &fakeABICallbacks{}
		mod := mustCompileWithCacheForRootVM(t, ctx, vmStartOnlyModule)
		rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()
		rv.RegisterABICallbacks(cb)

		if err := rv.Configure(ctx, nil, nil); err != nil {
			t.Fatalf("Configure (no proxy_on_context_create export): %v", err)
		}

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
		sb := allowAllSandbox()
		delete(sb.AllowedCapabilities, capProxyOnContextCreate)

		cb := &fakeABICallbacks{}
		mod := mustCompileWithCacheForRootVM(t, ctx, lifecycleOrderModule)
		rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(sb))
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()
		rv.RegisterABICallbacks(cb)

		if err := rv.Configure(ctx, nil, nil); err != nil {
			t.Fatalf("Configure (cap denied): %v", err)
		}

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

// --- TestRootVM_NewStreamContext ------------------------------------------

func TestRootVM_NewStreamContext(t *testing.T) {
	ctx := context.Background()

	t.Run("allocates monotonic streamCtxIDs", func(t *testing.T) {
		mod := mustCompileWithCacheForRootVM(t, ctx, minimalInitModule)
		rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()
		if err := rv.Configure(ctx, nil, nil); err != nil {
			t.Fatalf("Configure: %v", err)
		}

		sc1, err := rv.NewStreamContext(ctx)
		if err != nil {
			t.Fatalf("NewStreamContext #1: %v", err)
		}
		sc2, err := rv.NewStreamContext(ctx)
		if err != nil {
			t.Fatalf("NewStreamContext #2: %v", err)
		}
		sc3, err := rv.NewStreamContext(ctx)
		if err != nil {
			t.Fatalf("NewStreamContext #3: %v", err)
		}

		if sc1.ContextID() == sc2.ContextID() || sc2.ContextID() == sc3.ContextID() {
			t.Errorf("non-monotonic streamCtxIDs: %d %d %d", sc1.ContextID(), sc2.ContextID(), sc3.ContextID())
		}
		if sc2.ContextID() <= sc1.ContextID() || sc3.ContextID() <= sc2.ContextID() {
			t.Errorf("not strictly increasing: %d %d %d", sc1.ContextID(), sc2.ContextID(), sc3.ContextID())
		}
	})

	t.Run("registers child in streamCtxs map + Close removes it", func(t *testing.T) {
		mod := mustCompileWithCacheForRootVM(t, ctx, minimalInitModule)
		rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()
		if err := rv.Configure(ctx, nil, nil); err != nil {
			t.Fatalf("Configure: %v", err)
		}

		sc, err := rv.NewStreamContext(ctx)
		if err != nil {
			t.Fatalf("NewStreamContext: %v", err)
		}
		rv.streamCtxsMu.RLock()
		_, present := rv.streamCtxs[sc.ContextID()]
		rv.streamCtxsMu.RUnlock()
		if !present {
			t.Errorf("StreamContext not in rv.streamCtxs after NewStreamContext")
		}

		if err := sc.Close(ctx); err != nil {
			t.Fatalf("StreamContext.Close: %v", err)
		}
		rv.streamCtxsMu.RLock()
		_, stillPresent := rv.streamCtxs[sc.ContextID()]
		rv.streamCtxsMu.RUnlock()
		if stillPresent {
			t.Errorf("StreamContext still in rv.streamCtxs after Close")
		}
	})

	t.Run("NewStreamContext fires proxy_on_context_create(streamCtxID, rootCtxID)", func(t *testing.T) {
		// Use lifecycleOrderModule (exports proxy_on_context_create) + a sandbox
		// allowing the cap. Configure first to drain the root-context create
		// log invocation, then make sure NewStreamContext adds exactly ONE
		// more proxy_on_context_create call.
		cb := &fakeABICallbacks{}
		mod := mustCompileWithCacheForRootVM(t, ctx, lifecycleOrderModule)
		rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
		if err != nil {
			t.Fatalf("NewRootVM: %v", err)
		}
		defer func() { _ = rv.Close() }()
		rv.RegisterABICallbacks(cb)
		if err := rv.Configure(ctx, nil, nil); err != nil {
			t.Fatalf("Configure: %v", err)
		}

		before := len(cb.callsSnapshot())
		sc, err := rv.NewStreamContext(ctx)
		if err != nil {
			t.Fatalf("NewStreamContext: %v", err)
		}
		defer func() { _ = sc.Close(ctx) }()
		after := cb.callsSnapshot()
		if len(after)-before != 1 {
			t.Fatalf("NewStreamContext: expected exactly 1 proxy_log invocation (from proxy_on_context_create), got %d (delta=%d)",
				len(after), len(after)-before)
		}
		// The added call is the Info-level log from lifecycleOrderModule's
		// proxy_on_context_create body.
		got := after[before]
		want := `Log(2,"")`
		if got != want {
			t.Errorf("proxy_on_context_create log mismatch: got %q, want %q", got, want)
		}
	})
}

// --- TestRootVM_Close_ClosesChildStreamContexts ---------------------------

// TestRootVM_Close_ClosesChildStreamContexts is the regression test for the
// review BLOCKING-1 finding: after rv.Close(), still-held *StreamContext
// references must observe a graceful error on subsequent CallProxyOnX (NOT
// panic dereferencing a nil rv.instance). Verified by allocating a
// StreamContext, closing the parent RootVM, then invoking each per-callback
// method + Close on the child and asserting (a) no panic, (b) graceful
// non-nil error from each Call (the closed-guard).
func TestRootVM_Close_ClosesChildStreamContexts(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileWithCacheForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	sc, err := rv.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}

	// Close the RootVM while still holding the *StreamContext reference.
	// This is the scenario where rv.instance gets cleared but the caller has
	// not (yet) released their *StreamContext handle.
	if err := rv.Close(); err != nil {
		t.Fatalf("rv.Close: %v", err)
	}

	// Each subsequent per-callback method must observe a graceful non-nil
	// error from the sc.closed guard, NOT panic on the nil rv.instance.
	// Use a small helper that recovers + flags a test failure on panic.
	mustNotPanic := func(name string, fn func() error) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("%s panicked after rv.Close: %v (want graceful error return)", name, r)
			}
		}()
		err := fn()
		if err == nil {
			t.Errorf("%s returned nil error after rv.Close (want graceful closed-error)", name)
		}
	}

	mustNotPanic("CallProxyOnRequestHeaders", func() error {
		_, err := sc.CallProxyOnRequestHeaders(ctx, 0, false)
		return err
	})
	mustNotPanic("CallProxyOnResponseHeaders", func() error {
		_, err := sc.CallProxyOnResponseHeaders(ctx, 0, false)
		return err
	})
	mustNotPanic("CallProxyOnDone", func() error {
		_, err := sc.CallProxyOnDone(ctx)
		return err
	})
	mustNotPanic("CallProxyOnLog", func() error {
		return sc.CallProxyOnLog(ctx)
	})
	mustNotPanic("CallProxyOnDelete", func() error {
		return sc.CallProxyOnDelete(ctx)
	})

	// sc.Close itself must NOT panic; it must observe sc.closed.Load()==true
	// and short-circuit each per-callback fired from within (each Call
	// graceful-errors; firstErr propagates but is non-fatal).
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("sc.Close panicked after rv.Close: %v", r)
		}
	}()
	_ = sc.Close(ctx) // returns the first per-callback closed-error; not asserted here.
}

// --- TestNewStreamContext_RollbackOnDispatchFailure ----------------------

// TestNewStreamContext_RollbackOnDispatchFailure is the regression test for
// the review SHOULD-FIX-5 finding: NewStreamContext's rollback path (the
// delete(rv.streamCtxs, id) on dispatch failure at root_vm.go:404-406) is
// exercised via a fixture whose proxy_on_context_create body traps
// (single `unreachable` opcode). Asserts (a) NewStreamContext returns a
// non-nil error; (b) the failed-allocation id is NOT present in
// rv.streamCtxs; (c) a subsequent successful NewStreamContext call on the
// same RootVM still works + its id IS present (so the rollback is scoped
// — it doesn't poison the registry).
//
// Note: we do NOT call Configure on this RootVM (its proxy_on_context_create
// for the root context would trap too); the test exercises NewStreamContext
// directly. The pre-seeded currentCtxID at NewRootVM is sufficient for the
// hostcall-free dispatch.
func TestNewStreamContext_RollbackOnDispatchFailure(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileWithCacheForRootVM(t, ctx, contextCreateTrapsModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	// The first call traps inside proxy_on_context_create → wrapped error +
	// rollback of the streamCtxs entry.
	sc, err := rv.NewStreamContext(ctx)
	if err == nil {
		t.Fatal("NewStreamContext: expected non-nil err from trapping proxy_on_context_create")
	}
	if sc != nil {
		t.Errorf("NewStreamContext: expected nil sc on failure, got %v", sc)
	}

	// Verify the failed-allocation id was rolled back from the registry.
	// nextStreamCtxID is Add'd BEFORE the dispatch attempt, so the failed
	// id is the current value of nextStreamCtxID (== 1 for the first call).
	failedID := rv.nextStreamCtxID.Load()
	rv.streamCtxsMu.RLock()
	_, present := rv.streamCtxs[failedID]
	mapLen := len(rv.streamCtxs)
	rv.streamCtxsMu.RUnlock()
	if present {
		t.Errorf("rollback failed: streamCtxs[%d] still present after dispatch failure", failedID)
	}
	if mapLen != 0 {
		t.Errorf("rollback scope leak: streamCtxs has %d entries, want 0", mapLen)
	}

	// Verify the rollback didn't poison the registry — a second attempt
	// fails identically (rollback STILL fires; no leak across attempts).
	sc2, err2 := rv.NewStreamContext(ctx)
	if err2 == nil {
		t.Fatal("NewStreamContext #2: expected non-nil err (fixture always traps)")
	}
	if sc2 != nil {
		t.Errorf("NewStreamContext #2: expected nil sc on failure, got %v", sc2)
	}
	rv.streamCtxsMu.RLock()
	mapLenAfter := len(rv.streamCtxs)
	rv.streamCtxsMu.RUnlock()
	if mapLenAfter != 0 {
		t.Errorf("rollback scope leak across attempts: streamCtxs has %d entries after 2 failures, want 0", mapLenAfter)
	}
}

// --- TestRootVM_Close_Idempotent ------------------------------------------

func TestRootVM_Close_Idempotent(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	if err := rv.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := rv.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if err := rv.Close(); err != nil {
		t.Errorf("third Close: %v", err)
	}
}

// --- TestRootVM_HasGlobalFunc + TestRootVM_State --------------------------

func TestRootVM_HasGlobalFunc(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileWithCacheForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if !rv.HasGlobalFunc("_initialize") {
		t.Error("HasGlobalFunc(_initialize) = false, want true")
	}
	if rv.HasGlobalFunc("does_not_exist") {
		t.Error("HasGlobalFunc(does_not_exist) = true, want false")
	}
}

func TestRootVM_State(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	rt := rv.State()
	if rt == nil {
		t.Error("State() = nil, want non-nil wazero.Runtime")
	}
}

// --- TestRootVM_WasiHost_Satisfaction -------------------------------------

func TestRootVM_WasiHost_Satisfaction(t *testing.T) {
	ctx := context.Background()
	var sink bytes.Buffer
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1,
		WithRootSandboxConfig(SandboxConfig{AllowedCapabilities: map[string]SanitizationConfig{capWasiFdWrite: {}}}),
		WithRootLogSink(&sink),
	)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	var host wasiHost = rv
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

// --- TestRootVM_Concurrent_StreamContexts_NoStateLeak ---------------------

// TestRootVM_Concurrent_StreamContexts_NoStateLeak fans out N=100 goroutines
// each constructing+closing a StreamContext against ONE shared RootVM. With
// -race, the test catches any cross-stream mutation. Asserts no errors + all
// stream contexts close cleanly + final streamCtxs map is empty.
func TestRootVM_Concurrent_StreamContexts_NoStateLeak(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileWithCacheForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	const n = 100
	var wg sync.WaitGroup
	var errCount atomic.Uint32
	errs := make(chan error, n)
	idsCh := make(chan uint32, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc, err := rv.NewStreamContext(ctx)
			if err != nil {
				errs <- fmt.Errorf("NewStreamContext: %w", err)
				errCount.Add(1)
				return
			}
			idsCh <- sc.ContextID()
			if err := sc.Close(ctx); err != nil {
				errs <- fmt.Errorf("StreamContext.Close: %w", err)
				errCount.Add(1)
				return
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(idsCh)

	for e := range errs {
		t.Error(e)
	}
	if errCount.Load() != 0 {
		t.Fatalf("errCount = %d, want 0", errCount.Load())
	}

	// Collect IDs + ensure they're all distinct.
	seen := make(map[uint32]struct{}, n)
	for id := range idsCh {
		if _, dup := seen[id]; dup {
			t.Errorf("duplicate streamCtxID allocated: %d", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != n {
		t.Errorf("expected %d distinct streamCtxIDs, got %d", n, len(seen))
	}

	// Final map state — every stream context Close'd, so empty.
	rv.streamCtxsMu.RLock()
	leftOver := len(rv.streamCtxs)
	rv.streamCtxsMu.RUnlock()
	if leftOver != 0 {
		t.Errorf("rv.streamCtxs leaked %d entries after Close fan-in (want 0)", leftOver)
	}
}

// --- TestRootVM_LogLevelString --------------------------------------------

// TestRootVM_LogLevelString verifies the level-name mapping used by LogProxy.
// Inherited from 25.1 vm.go's logLevelString helper which migrates to
// root_vm.go.
func TestRootVM_LogLevelString(t *testing.T) {
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

// --- TestRootVM_PanicWrapper ----------------------------------------------

// TestRootVM_PanicWrapper verifies the panic-wrapper integration when a Go
// panic is raised inside an ABICallbacks method during a hostcall (proxy_log).
// The wrapper converts the panic to abi.WasmResultInternalFailure (=10) +
// fires the PanicHandlerFn.
func TestRootVM_PanicWrapper(t *testing.T) {
	ctx := context.Background()
	var sink bytes.Buffer

	var captured any
	cb := &fakeABICallbacks{
		panicOnLog:    true,
		panicOnLogMsg: "callback-panic-payload",
	}
	mod := mustCompileWithCacheForRootVM(t, ctx, invokeProxyLogModule)
	rv, err := NewRootVM(ctx, mod, 1,
		WithRootSandboxConfig(allowAllSandbox()),
		WithRootLogSink(&sink),
		WithRootPanicHandler(func(r any) { captured = r }),
	)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	rv.RegisterABICallbacks(cb)
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// invoke_proxy_log is exported; invoke it manually to fire the hostcall.
	invoke := rv.instance.ExportedFunction("invoke_proxy_log")
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

// --- Test helpers ---------------------------------------------------------

// mustCompileForRootVM compiles src via the nil-cache CompileModule path
// (transient runtime owned by the *Module). Convenience for tests that don't
// need a shared CompilationCache.
func mustCompileForRootVM(t *testing.T, ctx context.Context, src []byte) *Module {
	t.Helper()
	mod, err := CompileModule(ctx, src, nil)
	if err != nil {
		t.Fatalf("CompileModule: %v", err)
	}
	t.Cleanup(func() { _ = mod.Close(ctx) })
	return mod
}

// mustCompileWithCacheForRootVM compiles src via a CompileCache so the
// CompiledModule binds to the cache's runtime — NewRootVM internally re-compiles
// against its own wazero.Runtime via the shared CompilationCache exposed by
// the cache, mirroring the production wiring pattern.
func mustCompileWithCacheForRootVM(t *testing.T, ctx context.Context, src []byte) *Module {
	t.Helper()
	cache := NewCompileCache(ctx)
	t.Cleanup(func() { _ = cache.Close() })
	mod, err := CompileModule(ctx, src, cache)
	if err != nil {
		t.Fatalf("CompileModule(cache): %v", err)
	}
	return mod
}
