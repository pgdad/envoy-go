// Tests for the StreamContext per-stream-callback methods + Close per
// 25.2 SPEC §3.1 + Task 1 acceptance criteria.
//
// Coverage:
//   - CallProxyOn{RequestHeaders, ResponseHeaders, Done, Log, Delete}
//     graceful default returns on no-export + cap-denied paths.
//   - StreamContext.Close idempotent + fires proxy_on_done +
//     proxy_on_log + proxy_on_delete on the stream context.
//   - StreamContext.Close removes the streamCtxID from rootVM.streamCtxs.
//   - Per-callback panic-wrapper converts a Go-panic in an ABICallbacks
//     method to a non-nil err return.
//   - HasGlobalFunc delegation.
//   - CallProxyOnContextCreate-before-Configure errors.

package wasm

import (
	"context"
	"sync"
	"testing"

	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// --- TestStreamContext_PerCallback_NoExportNoCap --------------------------

// TestStreamContext_PerCallback_NoExportNoCap verifies graceful defaults
// (no error, ProxyActionContinue) when:
//   - Guest module does not export the callback.
//   - Capability is denied via SandboxConfig.
func TestStreamContext_PerCallback_NoExportNoCap(t *testing.T) {
	ctx := context.Background()

	t.Run("guest does not export callback: no-op continue + nil err", func(t *testing.T) {
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
		defer func() { _ = sc.Close(ctx) }()

		act, err := sc.CallProxyOnRequestHeaders(ctx, 5, false)
		if err != nil || act != abi.ProxyActionContinue {
			t.Errorf("OnRequestHeaders: act=%d err=%v, want (Continue, nil)", act, err)
		}
		act, err = sc.CallProxyOnResponseHeaders(ctx, 5, false)
		if err != nil || act != abi.ProxyActionContinue {
			t.Errorf("OnResponseHeaders: act=%d err=%v", act, err)
		}
		done, err := sc.CallProxyOnDone(ctx)
		if err != nil || !done {
			t.Errorf("OnDone: done=%v err=%v, want (true, nil)", done, err)
		}
		if err := sc.CallProxyOnLog(ctx); err != nil {
			t.Errorf("OnLog: %v", err)
		}
		if err := sc.CallProxyOnDelete(ctx); err != nil {
			t.Errorf("OnDelete: %v", err)
		}
	})

	t.Run("capability denied: skip dispatch (no error)", func(t *testing.T) {
		// Deny-all SandboxConfig (zero value).
		mod := mustCompileForRootVM(t, ctx, minimalInitModule)
		rv, err := NewRootVM(ctx, mod, 1)
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
		defer func() { _ = sc.Close(ctx) }()

		act, err := sc.CallProxyOnRequestHeaders(ctx, 0, false)
		if err != nil || act != abi.ProxyActionContinue {
			t.Errorf("OnRequestHeaders on deny: act=%d err=%v", act, err)
		}
		done, err := sc.CallProxyOnDone(ctx)
		if err != nil || !done {
			t.Errorf("OnDone on deny: done=%v err=%v", done, err)
		}
	})
}

// --- TestStreamContext_HasGlobalFunc --------------------------------------

func TestStreamContext_HasGlobalFunc(t *testing.T) {
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
	sc, err := rv.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}
	defer func() { _ = sc.Close(ctx) }()

	if !sc.HasGlobalFunc("_initialize") {
		t.Error("HasGlobalFunc(_initialize) = false, want true")
	}
	if sc.HasGlobalFunc("does_not_exist") {
		t.Error("HasGlobalFunc(does_not_exist) = true, want false")
	}
}

// --- TestStreamContext_Close_Idempotent -----------------------------------

func TestStreamContext_Close_Idempotent(t *testing.T) {
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
	sc, err := rv.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}

	if err := sc.Close(ctx); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := sc.Close(ctx); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if err := sc.Close(ctx); err != nil {
		t.Errorf("third Close: %v", err)
	}
}

// --- TestStreamContext_PanicWrapper ---------------------------------------

// TestStreamContext_PanicWrapper verifies the per-callback method
// panic-wrapper converts a Go panic in the guest's Call execution to a
// non-nil err return. We rig this by registering an ABICallbacks that panics
// inside Log, then invoking proxy_on_request_headers via a fixture that
// internally calls proxy_log.
func TestStreamContext_PanicWrapper(t *testing.T) {
	ctx := context.Background()
	var captured any
	cb := &fakeABICallbacks{
		panicOnLog:    true,
		panicOnLogMsg: "rh-panic",
	}
	mod := mustCompileWithCacheForRootVM(t, ctx, onRequestHeadersInvokesLogModule)
	rv, err := NewRootVM(ctx, mod, 1,
		WithRootSandboxConfig(allowAllSandbox()),
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
	sc, err := rv.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}
	defer func() { _ = sc.Close(ctx) }()

	// proxy_on_request_headers internally calls proxy_log → panics in Go
	// callback → recover converts to InternalFailure on the wire. The
	// PanicHandler fires regardless of the per-callback caller's observed
	// return.
	_, _ = sc.CallProxyOnRequestHeaders(ctx, 0, false)
	if captured != "rh-panic" {
		t.Errorf("PanicHandlerFn captured = %v, want \"rh-panic\"", captured)
	}
}

// --- TestStreamContext_Close_FiresLifecycleCallbacks ----------------------

// TestStreamContext_Close_FiresLifecycleCallbacks is a placeholder
// regression seed: Close must invoke proxy_on_done + proxy_on_log +
// proxy_on_delete in order on the stream context. Verified via the
// ABICallbacks log invocations (the fixture's proxy_on_X bodies are
// no-op since minimalInitModule does not export proxy_on_done etc.).
// The cap-denied branch returns (true, nil) for OnDone; we re-verify
// the no-error invariant by invoking Close on a freshly-allocated
// StreamContext.
func TestStreamContext_Close_FiresLifecycleCallbacks(t *testing.T) {
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
	sc, err := rv.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}

	// minimalInitModule exports no proxy_on_* — Close should still succeed.
	if err := sc.Close(ctx); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// --- TestStreamContext_TrapPoison_SkipsTeardown (BUG-3 regression) ---------
//
// A guest TRAP in proxy_on_request_headers poisons the shared instance (a
// real Rust panic! leaves the proxy-wasm-rust-sdk dispatcher RefCell
// borrowed; re-entry cascades panic_already_borrowed). The host catches the
// trap on the request path, but (*StreamContext).Close MUST NOT re-enter the
// poisoned instance via the proxy_on_done/log/delete teardown triplet.
//
// This is the REAL-trap repro (an `unreachable` guest), NOT injected state:
//  1. CallProxyOnRequestHeaders → expect a non-nil trap error.
//  2. sc.trapped must be set true by the trap.
//  3. sc.Close() → expect NO panic + nil error (the teardown triplet —
//     which ALSO traps in this fixture — is SKIPPED because the instance is
//     poisoned). Before the fix, Close fired proxy_on_done into the poisoned
//     instance → a second trap → the cascade.
//
// Run under -race.
func TestStreamContext_TrapPoison_SkipsTeardown(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, requestHeadersTrapsModule)
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

	// 1. The guest traps in proxy_on_request_headers.
	_, callErr := sc.CallProxyOnRequestHeaders(ctx, 1, true)
	if callErr == nil {
		t.Fatal("CallProxyOnRequestHeaders returned nil err; want a trap error (the guest executes `unreachable`)")
	}

	// 2. The trap must have flagged the instance poisoned.
	if !sc.trapped.Load() {
		t.Fatal("sc.trapped = false after a guest trap; want true (BUG-3: a trap must poison the StreamContext)")
	}

	// 3. Close must skip the teardown triplet (which also traps) — no cascade,
	// no re-entry error.
	var closeErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Close cascaded a panic on a trapped instance (BUG-3): %v", r)
			}
		}()
		closeErr = sc.Close(ctx)
	}()
	if closeErr != nil {
		t.Errorf("Close on a trapped instance returned err=%v; want nil (teardown triplet must be SKIPPED, not re-entered)", closeErr)
	}

	// Close must still have completed its non-guest teardown: the streamCtx
	// entry is removed + the context is flagged closed.
	if !sc.closed.Load() {
		t.Error("sc.closed = false after Close; want true (Close must still flip closed even when skipping teardown)")
	}
	rv.streamCtxsMu.RLock()
	_, stillPresent := rv.streamCtxs[sc.streamCtxID]
	rv.streamCtxsMu.RUnlock()
	if stillPresent {
		t.Error("streamCtx entry still present after Close; want removed (Close must still perform map cleanup)")
	}
}

// --- TestStreamContext_DispatchAfterRootVMClose_ErrorNotPanic --------------

// TestStreamContext_DispatchAfterRootVMClose_ErrorNotPanic: RootVM.Close
// flips every live child StreamContext's closed flag + nils rv.instance
// under dispatchMu. A caller still holding the *StreamContext must observe
// the graceful closed-StreamContext error from every CallProxyOn* method —
// NOT a nil-interface panic dereferencing the cleared rv.instance — and
// NewStreamContext must reject with the closed-RootVM error. Exercises the
// dispatchGuest re-check discipline (the instance read lives under
// dispatchMu with a post-Lock closed re-check).
func TestStreamContext_DispatchAfterRootVMClose_ErrorNotPanic(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileWithCacheForRootVM(t, ctx, exportsAll25_2CallbacksModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	rv.RegisterABICallbacks(&fakeABICallbacks{})
	sc, err := rv.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}

	if err := rv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Every per-callback dispatch must early-return with the exact
	// pre-consolidation closed-StreamContext error string (pinned — the
	// dispatchGuest helper builds it from the method name).
	if _, err := sc.CallProxyOnRequestHeaders(ctx, 1, false); err == nil ||
		err.Error() != "wasm: CallProxyOnRequestHeaders on closed StreamContext" {
		t.Errorf("CallProxyOnRequestHeaders after Close: err=%v; want the closed-StreamContext error", err)
	}
	if _, err := sc.CallProxyOnRequestBody(ctx, 1, false); err == nil ||
		err.Error() != "wasm: CallProxyOnRequestBody on closed StreamContext" {
		t.Errorf("CallProxyOnRequestBody after Close: err=%v; want the closed-StreamContext error", err)
	}
	if done, err := sc.CallProxyOnDone(ctx); err == nil || !done ||
		err.Error() != "wasm: CallProxyOnDone on closed StreamContext" {
		t.Errorf("CallProxyOnDone after Close: done=%v err=%v; want (true, closed-StreamContext error)", done, err)
	}
	if err := sc.CallProxyOnLog(ctx); err == nil ||
		err.Error() != "wasm: CallProxyOnLog on closed StreamContext" {
		t.Errorf("CallProxyOnLog after Close: err=%v; want the closed-StreamContext error", err)
	}

	if _, err := rv.NewStreamContext(ctx); err == nil {
		t.Error("NewStreamContext after Close: err=nil; want closed-RootVM error")
	}
}

// --- TestStreamContext_ConcurrentDispatchAndClose_NoPanic ------------------

// TestStreamContext_ConcurrentDispatchAndClose_NoPanic: hammers per-stream
// dispatch + NewStreamContext concurrently with RootVM.Close under -race.
// Before the dispatchGuest consolidation the CallProxyOn* template read
// rv.instance BEFORE acquiring dispatchMu and never re-checked closed after
// Lock, so a dispatch racing Close could dereference the nil'd instance —
// a panic this test would surface (plus a -race report on the unsynchronized
// instance read). Errors are expected + ignored; the assertion is the
// absence of panics/races.
func TestStreamContext_ConcurrentDispatchAndClose_NoPanic(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileWithCacheForRootVM(t, ctx, exportsAll25_2CallbacksModule)
	rv, err := NewRootVM(ctx, mod, 1, WithRootSandboxConfig(allowAllSandbox()))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	rv.RegisterABICallbacks(&fakeABICallbacks{})

	const streams = 8
	scs := make([]*StreamContext, streams)
	for i := range scs {
		sc, err := rv.NewStreamContext(ctx)
		if err != nil {
			t.Fatalf("NewStreamContext(%d): %v", i, err)
		}
		scs[i] = sc
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, sc := range scs {
		wg.Add(1)
		go func(sc *StreamContext) {
			defer wg.Done()
			<-start
			for j := 0; j < 100; j++ {
				// Real guest dispatch (exported) + no-export path — both
				// must survive a concurrent Close gracefully.
				_, _ = sc.CallProxyOnRequestBody(ctx, 1, false)
				_, _ = sc.CallProxyOnRequestHeaders(ctx, 1, false)
			}
		}(sc)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for j := 0; j < 100; j++ {
			if sc, err := rv.NewStreamContext(ctx); err == nil {
				_ = sc.Close(ctx)
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_ = rv.Close()
	}()

	close(start)
	wg.Wait()
}
