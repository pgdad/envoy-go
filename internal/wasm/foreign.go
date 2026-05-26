// foreign.go — ForeignFunctionRegistry + process-global EMPTY default per
// 25.2 SPEC §3.1 + §5.1 #38 + AMEND-A9 + R-25.2-8 + D-25.2-P3 closure
// (mutex-per-RootVM dispatch concurrency model per D-P-PLAN-9).
//
// # Design (per AMEND-A9 + R-25.2-8 + D-P-PLAN-9)
//
// ForeignFunctionFn is the canonical Go-side foreign-function signature:
//
//	func(ctx context.Context, args []byte) (result []byte, status WasmResult)
//
// matching the proxy-wasm v0.2.1 `proxy_call_foreign_function` wire-contract
// (the guest passes `args` bytes; the host returns `result` bytes + a status
// code). The function executes synchronously inside the dispatching
// `*RootVM.CallForeignFunction` call frame (no goroutine offload at 25.2 per
// D-P-PLAN-9 clause b).
//
// # EMPTY default registry per AMEND-A9
//
// envoy-go ships ZERO default foreign functions (vs upstream proxy-wasm-cpp-
// host's 10: verify_signature + sign + compress + uncompress +
// set_envoy_filter_state + clear_route_cache + expr_create + expr_evaluate +
// expr_delete + declare_property per cpp-host `src/exports.cc:147-184`).
// Operators MUST explicitly enable the `proxy_call_foreign_function`
// capability AND register specific foreign functions via
// `wasm.RegisterForeignFunction(name, fn)` at boot. Unregistered names return
// `WasmResult::NotFound` (=1) — byte-faithful to upstream NotFound semantic.
// envoy-go-strict departure record #5 at BEHAVIOR_CONTRACT.md (Task 22).
//
// # Concurrency model per D-P-PLAN-9
//
// 5-clause RATIFIED:
//
//	(a) *ForeignFunctionRegistry.Get holds an mu.RLock() only — read-only
//	    access; registry is mutated only at boot via Register; runtime Get
//	    traffic concurrent.
//	(b) The dispatched ForeignFunctionFn executes synchronously inside
//	    *RootVM.CallForeignFunction (no goroutine offload at 25.2; if compute-
//	    heavy foreign functions emerge, future scope MAY add an opt-in async
//	    dispatch surface per ADR-0207 API-REVISION ALLOWANCE — but at 25.2
//	    dispatch is synchronous).
//	(c) The *RootVM dispatch lock IS HELD during invocation (same lock as
//	    per-stream call frame; the per-*StreamContext call frame holds the
//	    *RootVM dispatchMu for the duration of the proxy_on_* invocation;
//	    foreign-function calls are nested INSIDE that frame so the lock is
//	    already held; CallForeignFunction MUST NOT re-acquire dispatchMu —
//	    sync.Mutex in Go is non-reentrant and would deadlock).
//	(d) Panic-recovery wrapper applies (same wrapper as other host-side
//	    callbacks — Go panic in ForeignFunctionFn → recover() → log +
//	    envoy_go.failures counter increment + return WasmResult::InternalFailure
//	    to guest; lives inside *RootVM.CallForeignFunction).
//	(e) The `foreign_function_denied` envoy-go-strict counter increments on
//	    the NotFound path (unregistered name) per AMEND-A9. Counter wiring
//	    deferred Task 17 (per-plugin stats.go EXTEND).
//
// ALTERNATIVE REJECTED at PLAN session: event-loop-per-RootVM — adds non-
// trivial complexity (Go goroutine pool + select-loop dispatch) for a use
// case that doesn't yet exist (no operator has requested async foreign-
// function dispatch); YAGNI per CLAUDE.md.
//
// ALTERNATIVE REJECTED at PLAN session: caller-goroutine dispatch (foreign-
// function fires on the per-stream goroutine WITHOUT crossing the *RootVM
// lock) — breaks the upstream byte-faithful semantic (cpp-host serializes
// foreign-function dispatch via the Wasm-level lock; envoy-go must mirror to
// keep guest behavior identical); risk of guest-observable concurrency
// divergence is unacceptable.

package wasm

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// ForeignFunctionFn is the canonical Go-side foreign-function signature
// (proxy-wasm v0.2.1 `proxy_call_foreign_function` wire-contract). The guest
// passes `args` bytes; the host returns `result` bytes + a status code.
// `result` MAY be nil for "no result" returns (e.g., a fire-and-forget
// foreign function that only signals via the status). `status` is the
// canonical WasmResult; `WasmResultOk` (=0) signals success.
//
// Concurrency: each invocation executes synchronously inside the per-
// *RootVM dispatchMu-held call frame (D-P-PLAN-9 clause c). The function
// implementation MAY touch shared state (e.g., an in-memory key-value
// store) but MUST NOT block indefinitely + MUST NOT re-acquire dispatchMu
// from within (the lock is non-reentrant). Compute-heavy work is the
// operator's responsibility per D-P-PLAN-9 (b).
//
// Panic-recovery: a Go panic from this function is recovered by
// *RootVM.CallForeignFunction; the recovered value flows through the
// PanicHandlerFn (if configured) + the dispatch returns
// WasmResult::InternalFailure (=10) to the guest per D-P-PLAN-9 (d).
type ForeignFunctionFn func(ctx context.Context, args []byte) (result []byte, status abi.WasmResult)

// ForeignFunctionRegistry is a name-keyed registry of ForeignFunctionFn
// implementations. Per D-P-PLAN-9 + AMEND-A9 + R-25.2-8:
//
//   - Register acquires Lock + checks for duplicate name (returns error if
//     already registered) + adds to map. Typically called at boot (process
//     init or per-test setup).
//   - Get acquires RLock + looks up + returns (fn, ok). Called from
//     *RootVM.CallForeignFunction on every dispatch. RLock allows
//     concurrent Get from multiple goroutines (D-P-PLAN-9 clause a).
//
// The registry is process-global at DefaultForeignFunctionRegistry; per-
// RootVM overrides are supported via WithRootForeignRegistry for testing
// + multi-tenant isolation.
type ForeignFunctionRegistry struct {
	mu  sync.RWMutex
	fns map[string]ForeignFunctionFn
}

// NewForeignFunctionRegistry constructs a fresh EMPTY ForeignFunctionRegistry.
// Per AMEND-A9 + R-25.2-8: envoy-go's default registry ships with ZERO
// pre-registered functions (vs upstream proxy-wasm-cpp-host's 10). Operators
// register at boot via Register or the top-level RegisterForeignFunction
// helper.
func NewForeignFunctionRegistry() *ForeignFunctionRegistry {
	return &ForeignFunctionRegistry{
		fns: make(map[string]ForeignFunctionFn),
	}
}

// Register adds a named foreign function to the registry. Returns an error
// if (a) name is empty, (b) fn is nil, or (c) a function is already
// registered under name (duplicate registration). Typically called at boot
// (process init or compiledConfig.New time).
//
// Concurrency: acquires the registry's write Lock. Safe to call from any
// goroutine; concurrent racing Registers serialize.
func (r *ForeignFunctionRegistry) Register(name string, fn ForeignFunctionFn) error {
	if name == "" {
		return errors.New("wasm: ForeignFunctionRegistry.Register: empty name")
	}
	if fn == nil {
		return fmt.Errorf("wasm: ForeignFunctionRegistry.Register: nil ForeignFunctionFn for name %q", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.fns[name]; ok {
		return fmt.Errorf("wasm: ForeignFunctionRegistry.Register: duplicate name %q already registered", name)
	}
	r.fns[name] = fn
	return nil
}

// Get returns the foreign function registered under name. Returns (nil,
// false) if no function is registered under name. The unregistered-name
// case is the canonical NotFound path that *RootVM.CallForeignFunction
// translates to WasmResult::NotFound (=1) per AMEND-A9 + R-25.2-8 (byte-
// faithful to upstream cpp-host `src/exports.cc:147-184`).
//
// Concurrency: acquires the registry's read RLock. Multiple goroutines can
// Get concurrently without blocking (D-P-PLAN-9 clause a). A concurrent
// Register blocks Get for the duration of the Lock-held section.
func (r *ForeignFunctionRegistry) Get(name string) (ForeignFunctionFn, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.fns[name]
	return fn, ok
}

// DefaultForeignFunctionRegistry is the process-global ForeignFunctionRegistry
// consumed by RootVM construction when no per-RootVM override is supplied via
// WithRootForeignRegistry. Per AMEND-A9 + R-25.2-8 it ships EMPTY — operators
// MUST explicitly register foreign functions at boot via
// wasm.RegisterForeignFunction(name, fn). The empty default + the
// gate-at-registration capProxyCallForeignFunction discipline (default-deny per
// AMEND-A5) form the two-layer defense-in-depth envoy-go-strict envelope.
//
//nolint:gochecknoglobals // process-global by design per AMEND-A9 + R-25.2-8.
var DefaultForeignFunctionRegistry = NewForeignFunctionRegistry()

// RegisterForeignFunction is the top-level convenience helper that
// delegates to DefaultForeignFunctionRegistry.Register. Operators call this
// at boot to register implementation-specific foreign functions per
// AMEND-A9 (envoy-go ships ZERO defaults; the operator is responsible for
// registering whatever foreign functions the guest needs).
//
// Example boot wiring:
//
//	func init() {
//	    if err := wasm.RegisterForeignFunction("expr_evaluate", myExprEvaluator); err != nil {
//	        log.Fatalf("wasm: register expr_evaluate: %v", err)
//	    }
//	}
//
// Returns the error from the underlying Register call (duplicate name; nil
// fn; empty name).
func RegisterForeignFunction(name string, fn ForeignFunctionFn) error {
	return DefaultForeignFunctionRegistry.Register(name, fn)
}
