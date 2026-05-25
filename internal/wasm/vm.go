// VM lifecycle + options + ABI-callbacks bridge per 25.1 SPEC §3.1.
//
// Per parent §2.7 + this phase §2: wazero runtime mode at 25.1 is the
// `wazero.NewRuntimeConfigInterpreter()` config — compiler-mode is
// deferred to a future benchmark-gated opt-in. Each per-stream `*VM` owns
// its own `wazero.Runtime` (per AMEND-A4 per-stream-VM construction model);
// the runtime gets the env-namespace + wasi_snapshot_preview1-namespace
// host modules registered at construction via `registerHostModules`.
//
// `*VM` is NOT goroutine-safe by contract (per-stream HTTP filter dispatch
// is single-goroutine per stream); the `sync.Mutex` guards the `closed`
// flag + `ctxStore` for safe interleaving across the hostcall goroutine
// boundary (wazero may invoke hostcalls on a different goroutine than the
// per-callback `Call`).
//
// Files in this package owning each part of the surface:
//   - vm.go (THIS FILE) — VM type, options, lifecycle, per-callback methods,
//     panic-wrapper, wasiHost satisfaction.
//   - registration.go — `ABICallbacks` interface, host-module wiring for
//     the 16 active `proxy_*` + 8 active `wasi_*` + 23 deferred-stub
//     hostcalls (47 total per parent §4.2 Option B).
//
// # Lifecycle gate disposition (per SPEC §3.3 + Task instructions)
//
// The 8 lifecycle/HTTP module-getter capability keys (`capProxyOnVmStart`,
// `capProxyOnConfigure`, `capProxyOnContextCreate`, `capProxyOnRequestHeaders`,
// `capProxyOnResponseHeaders`, `capProxyOnDone`, `capProxyOnLog`,
// `capProxyOnDelete`) ARE gated per upstream `wasm.cc:181-206` `_GET_PROXY`
// macro behavior. When `vm.sandbox.IsAllowed(<lifecycle_key>)` returns false,
// the corresponding Run-step / per-callback method SKIPS the call (treats as
// if the guest didn't export it). This matches upstream's "nullptr the
// function pointer" discipline; the dispatch path is a no-op continue for
// the HTTP-phase callbacks and a no-op success for the lifecycle callbacks.
//
// The 5 module-init / allocator keys (`_initialize`, `_start`, `main`,
// `malloc`, `proxy_on_memory_allocate`) are UNGATED per D-P2 closure at
// Task 6. `Run` invokes `_initialize` OR `_start` directly without checking
// `IsAllowed`.
//
// # currentStreamCtxID tracking
//
// At 25.1 the per-VM model is single-stream per the AMEND-A4 per-stream-VM
// construction. The "current" effective context id for a hostcall fires is
// determined by:
//
//	1. The hostcall's explicit context-id argument when one exists.
//	2. The most recent `CallProxyOnX` `streamContextID` argument (stored on
//	   the *VM atomically) for hostcalls without an explicit context arg.
//	3. The most recent `proxy_set_effective_context` invocation (overrides
//	   #2 until the next CallProxyOnX runs).
//
// This is a 25.1 simplification — the full upstream `effective_context_id_`
// stack discipline lands at 25.2 when timer + httpCall callbacks introduce
// real context-switching needs.

package wasm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// VM is a per-stream wazero execution context. NOT goroutine-safe by
// contract; per-stream HTTP filter dispatch is single-goroutine per stream.
// Constructed via `NewVM(ctx, opts...)`; released via `Close` at OnDestroy
// per the parent §3.5 dispatch shape.
//
// Per AMEND-A4 each per-stream filter dispatch constructs a fresh VM; cross-
// stream state lives at the `*Module` (compiled-module-reuse) and
// `*CompileCache` layers.
type VM struct {
	runtime  wazero.Runtime
	module   wazero.CompiledModule // captured at Run (the vm.runtime-bound re-compile per Task 7 follow-up)
	instance api.Module            // captured at Run after Instantiate
	sandbox  SandboxConfig
	panicH   PanicHandlerFn
	logSink  io.Writer
	cb       ABICallbacks

	// compilationCache, if non-nil, is wired into the VM's wazero.Runtime
	// at NewVM via wazero.NewRuntimeConfigInterpreter().WithCompilationCache(cc).
	// When this cache is also shared with a *CompileCache (via
	// CompileCache.WazeroCompilationCache()), vm.Run's per-stream re-compile
	// of module.Source() hits the shared cache sub-ms. See WithCompilationCache
	// option doc-comment for the production wiring pattern.
	compilationCache wazero.CompilationCache

	// ctxStore retains per-streamContextID contexts for the duration of the
	// stream. Keyed by the streamContextID passed to CallProxyOnContextCreate.
	// Guarded by `mu`. Future 25.2 timer/httpCall callbacks will surface a
	// stored context via proxy_set_effective_context dispatch.
	ctxStore map[uint32]context.Context

	// currentCtxID is the most recently "active" context for hostcall
	// dispatch. Updated by each `CallProxyOnX` entry + by
	// `proxy_set_effective_context` hostcall invocations. Atomic load/store
	// because the hostcall goroutine may be different from the per-callback
	// caller goroutine (wazero implementation detail).
	currentCtxID atomic.Uint32

	mu     sync.Mutex
	closed bool
}

// VMOption configures VM construction. Function-option pattern per the
// internal/lua + internal/jwks + internal/httpclient precedent.
type VMOption func(*VM)

// WithSandboxConfig sets the per-capability ALLOW/DENY posture. Zero value
// (default) = `StrictDefaultDeny` per AMEND-A5 — DENY ALL hostcalls. See
// `SandboxConfig` doc for the full discipline.
func WithSandboxConfig(sb SandboxConfig) VMOption {
	return func(vm *VM) { vm.sandbox = sb }
}

// WithPanicHandler sets the Go-panic handler invoked after recover() in
// the VM's panic-wrapper. The handler is invoked with the recovered value.
// Not for catching wazero-side traps (those return via sys.ExitError or
// wasmruntime.Error from Run/CallProxyOnX); the handler is for genuine Go
// panics from hostcall Go callbacks per AMEND-A8.
func WithPanicHandler(h PanicHandlerFn) VMOption {
	return func(vm *VM) { vm.panicH = h }
}

// WithLogSink redirects `proxy_log` + WASI `fd_write` output for the
// lifetime of this VM. Default nil = drop (no stdout leak; envoy-go-strict
// default). Naming distinct from upstream Envoy's `LogManager` (process-
// wide construct); per-VM sink matches phase-22.1's `WithBasePrintSink`
// precedent.
func WithLogSink(w io.Writer) VMOption {
	return func(vm *VM) { vm.logSink = w }
}

// WithCompilationCache configures the VM's underlying wazero.Runtime with
// a shared wazero.CompilationCache (the wazero-codegen-result cache,
// distinct from envoy-go's *CompileCache Go-level *Module store). When set,
// NewVM constructs the runtime via
// `wazero.NewRuntimeConfigInterpreter().WithCompilationCache(cc)`; when
// unset (default) the runtime is constructed without a cache and each VM
// pays full codegen cost on every Run.
//
// Production wiring pattern (Task 7 follow-up — cross-runtime
// CompiledModule binding fix): pass the cache exposed by the *CompileCache
// that produced the *Module:
//
//	cache := wasm.NewCompileCache(ctx)
//	defer cache.Close()
//	mod, err := wasm.CompileModule(ctx, src, cache)
//	// ...
//	vm := wasm.NewVM(ctx, wasm.WithCompilationCache(cache.WazeroCompilationCache()), opts...)
//	if err := vm.Run(ctx, mod, rootCtxID); err != nil { ... }
//
// This makes `vm.Run`'s internal re-compile of `module.Source()` against
// `vm.runtime` (required because wazero v1.10.1's CompiledModule is bound
// to the engine of the runtime that compiled it — see
// `wazero/cache.go:32-34` doc) hit the shared codegen cache as a sub-ms
// cache lookup rather than a full re-codegen.
func WithCompilationCache(cc wazero.CompilationCache) VMOption {
	return func(vm *VM) { vm.compilationCache = cc }
}

// PanicHandlerFn is invoked after `recover()` in the VM's panic-wrapper.
// `recovered` is the value returned by recover() (typically the panic
// value).
type PanicHandlerFn func(recovered any)

// NewVM constructs a per-stream VM. Applies opts (zero-value default-deny
// SandboxConfig if none provided), creates the underlying wazero.Runtime in
// interpreter mode per parent §2.7, and registers the env-namespace +
// wasi_snapshot_preview1-namespace host modules (24 active + 23 deferred =
// 47 hostcalls total per parent §4.2 Option B).
//
// Caller responsibility: release via `Close` at OnDestroy. Returns a
// non-nil *VM. The host-module registration is performed eagerly during
// NewVM so subsequent `Run` instantiations resolve the import section
// against the registered modules.
//
// A registration error is fatal — NewVM panics with the wrapped error.
// At 25.1 this can only happen if wazero's host-module-builder rejects a
// signature (which would be a programmer error in registration.go, caught
// in tests). Panicking here is safe because no resources are exposed to
// the caller before the panic; the runtime is closed in a defer.
func NewVM(ctx context.Context, opts ...VMOption) *VM {
	vm := &VM{
		ctxStore: make(map[uint32]context.Context),
	}
	// Apply opts BEFORE constructing the runtime — WithCompilationCache
	// must be honored at runtime-construction time (wazero's CompilationCache
	// is wired via RuntimeConfig.WithCompilationCache, set before
	// NewRuntimeWithConfig).
	for _, opt := range opts {
		opt(vm)
	}
	runtimeConfig := wazero.NewRuntimeConfigInterpreter()
	if vm.compilationCache != nil {
		runtimeConfig = runtimeConfig.WithCompilationCache(vm.compilationCache)
	}
	vm.runtime = wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	if err := registerHostModules(ctx, vm); err != nil {
		// Defensive: release the runtime to avoid a leak before panicking.
		_ = vm.runtime.Close(context.Background())
		panic(fmt.Errorf("wasm: NewVM: registerHostModules: %w", err))
	}
	return vm
}

// State returns the underlying wazero.Runtime. Escape-hatch for filter
// consumers that need to register additional host modules or query module
// exports beyond the 25.1 surface. Not safe to call after Close.
func (vm *VM) State() wazero.Runtime { return vm.runtime }

// RegisterABICallbacks installs the consumer's per-context callback bundle.
// The framework primitive's host-module wiring invokes these on the
// appropriate hostcall dispatch. Safe to call multiple times (last call
// wins); typically called once per VM before Run.
func (vm *VM) RegisterABICallbacks(cb ABICallbacks) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	vm.cb = cb
}

// Run re-compiles the module's source against this VM's wazero.Runtime
// (sub-ms cache hit when a shared `wazero.CompilationCache` is configured
// via `WithCompilationCache(cache.WazeroCompilationCache())`), instantiates
// the resulting CompiledModule, and executes the module-init lifecycle for
// the root context:
//
//	(a) Re-compile module.Source() against vm.runtime to produce a
//	    runtime-bound CompiledModule. REQUIRED because wazero v1.10.1's
//	    CompiledModule is bound to the engine of the runtime that compiled
//	    it (`wazero/cache.go:32-34`); a *Module produced by a *CompileCache's
//	    compile-only runtime cannot be directly instantiated by a per-stream
//	    VM's runtime. The shared wazero.CompilationCache amortizes the
//	    codegen cost.
//	(b) Instantiate the (vm.runtime-bound) CompiledModule onto the runtime.
//	(c) Call _initialize OR _start (mutually exclusive per the proxy-wasm
//	    v0.2.1 spec). UNGATED per D-P2.
//	(c.5) Call proxy_on_context_create(rootContextID, 0) — seeds the root
//	    context per the canonical proxy-wasm host lifecycle (matches upstream
//	    proxy-wasm-cpp-host@da3ce05d:src/wasm.cc + the proxy-wasm-rust-sdk
//	    v0.2.4 dispatcher expectation; proxy_on_vm_start consults the
//	    dispatcher's roots map + panics with "invalid context_id" if the root
//	    was not pre-created). parentContextID == 0 signals ROOT-context creation
//	    per the proxy-wasm v0.2.1 spec. Gated by capProxyOnContextCreate.
//	(d) Call proxy_on_vm_start(rootContextID, 0). Gated by capProxyOnVmStart
//	    per SPEC §3.3 — when denied the call is skipped (no-op success).
//	(e) Call proxy_on_configure(rootContextID, 0). Gated by capProxyOnConfigure
//	    per SPEC §3.3.
//
// Returns the wrapped wazero re-compile / instantiation / call error on
// failure. The 5th arg of `proxy_on_vm_start` / `proxy_on_configure`
// (vm_configuration_size / plugin_configuration_size) is zero at 25.1 —
// the VmConfig / PluginConfig data-source resolution lands at Task 10; the
// 25.1 surface invokes the callbacks with size=0 (matching upstream's "no
// config" wire shape).
//
// Lifetime note: the re-compiled CompiledModule is owned by vm.runtime and
// is released by vm.runtime.Close() per wazero's one-way cascade (no
// separate Close on the CompiledModule needed).
func (vm *VM) Run(ctx context.Context, module *Module, rootContextID uint32) error {
	if module == nil {
		return errors.New("wasm: Run: nil *Module")
	}

	vm.mu.Lock()
	if vm.closed {
		vm.mu.Unlock()
		return errors.New("wasm: Run on closed VM")
	}
	vm.mu.Unlock()

	// (a) Re-compile module.Source() against vm.runtime. Sub-ms cache hit
	// when WithCompilationCache(cache.WazeroCompilationCache()) was wired
	// at NewVM time; otherwise a full codegen (and the production wiring
	// should always pass the shared cache — see WithCompilationCache doc).
	src := module.Source()
	if src == nil {
		// Defensive: a *Module without retained src cannot be re-compiled.
		// All production *Modules (CompileModule, cache-owned or nil-cache)
		// retain src; this branch guards test fakes only.
		return errors.New("wasm: Run: *Module has no retained Source() bytes")
	}
	compiled, err := vm.runtime.CompileModule(ctx, src)
	if err != nil {
		return fmt.Errorf("wasm: re-compile for per-stream VM: %w", err)
	}

	// (b) Instantiate. Use an empty ModuleConfig — the WASI shims handle
	// stdout/stderr fanout (no .WithStdout / .WithStderr); the proxy-wasm
	// guest gets its environment via `proxy_get_property` (Task 11 territory).
	// .WithName("") avoids the wazero default-name collision when running
	// the same VM type concurrently in tests.
	instance, err := vm.runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return fmt.Errorf("wasm: instantiate: %w", err)
	}

	vm.mu.Lock()
	vm.instance = instance
	vm.module = compiled
	vm.mu.Unlock()

	// (c) Call _initialize OR _start. Mutually exclusive — per proxy-wasm
	// v0.2.1 + upstream wasm.cc:159-178: a module exports exactly one of
	// these as its initialization hook (WASI guests typically export _start;
	// proxy-wasm guests typically export _initialize). UNGATED per D-P2.
	if initFn := instance.ExportedFunction("_initialize"); initFn != nil {
		if _, err := initFn.Call(ctx); err != nil {
			return fmt.Errorf("wasm: _initialize: %w", err)
		}
	} else if startFn := instance.ExportedFunction("_start"); startFn != nil {
		if _, err := startFn.Call(ctx); err != nil {
			return fmt.Errorf("wasm: _start: %w", err)
		}
	}
	// No init export is acceptable — minimal test modules may omit both.

	// (c.5) Seed the root context per the canonical proxy-wasm host lifecycle.
	// Matches upstream proxy-wasm-cpp-host@da3ce05d:src/wasm.cc + the
	// proxy-wasm-rust-sdk v0.2.4 dispatcher expectation: proxy_on_vm_start
	// consults the dispatcher's roots map + panics with "invalid context_id"
	// if the root was not pre-created. parentContextID == 0 signals
	// ROOT-context creation per proxy-wasm v0.2.1 spec. Gated by
	// capProxyOnContextCreate; skipped if guest does not export
	// proxy_on_context_create (some hand-crafted minimal guests omit it).
	if vm.sandbox.IsAllowed(capProxyOnContextCreate) {
		if fn := instance.ExportedFunction("proxy_on_context_create"); fn != nil {
			if _, err := fn.Call(ctx, uint64(rootContextID), 0); err != nil {
				return fmt.Errorf("wasm: proxy_on_context_create(root): %w", err)
			}
		}
	}

	// (d) Call proxy_on_vm_start(rootContextID, 0). Gated by capProxyOnVmStart.
	if vm.sandbox.IsAllowed(capProxyOnVmStart) {
		if fn := instance.ExportedFunction("proxy_on_vm_start"); fn != nil {
			if _, err := fn.Call(ctx, uint64(rootContextID), 0); err != nil {
				return fmt.Errorf("wasm: proxy_on_vm_start: %w", err)
			}
		}
	}

	// (e) Call proxy_on_configure(rootContextID, 0). Gated by capProxyOnConfigure.
	if vm.sandbox.IsAllowed(capProxyOnConfigure) {
		if fn := instance.ExportedFunction("proxy_on_configure"); fn != nil {
			if _, err := fn.Call(ctx, uint64(rootContextID), 0); err != nil {
				return fmt.Errorf("wasm: proxy_on_configure: %w", err)
			}
		}
	}

	return nil
}

// HasGlobalFunc returns true if the named guest export is a callable
// function on the instantiated module. Used by Task 9 to check hook-presence
// after Run (supports module-shape PARSE-REJECT if needed — provisional at
// 25.1; 25.2 may extend). Returns false if Run has not yet been called.
func (vm *VM) HasGlobalFunc(name string) bool {
	vm.mu.Lock()
	inst := vm.instance
	vm.mu.Unlock()
	if inst == nil {
		return false
	}
	return inst.ExportedFunction(name) != nil
}

// CallProxyOnContextCreate invokes `proxy_on_context_create(streamContextID,
// rootContextID)` — creates a new per-stream context under the root.
//
// Gated by capProxyOnContextCreate. When denied or when the guest does not
// export proxy_on_context_create, returns nil (no-op success).
func (vm *VM) CallProxyOnContextCreate(ctx context.Context, streamContextID, rootContextID uint32) error {
	vm.setCurrentCtx(streamContextID, ctx)

	if !vm.sandbox.IsAllowed(capProxyOnContextCreate) {
		return nil
	}
	vm.mu.Lock()
	inst := vm.instance
	vm.mu.Unlock()
	if inst == nil {
		return errors.New("wasm: CallProxyOnContextCreate before Run")
	}
	fn := inst.ExportedFunction("proxy_on_context_create")
	if fn == nil {
		return nil
	}

	return vm.runCallWithPanicWrapper(func() error {
		_, err := fn.Call(ctx, uint64(streamContextID), uint64(rootContextID))
		return err
	})
}

// CallProxyOnRequestHeaders invokes `proxy_on_request_headers(streamContextID,
// numHeaders, endOfStream)` — returns `ProxyAction::CONTINUE` (=0) or
// `::PAUSE` (=1). Gated by capProxyOnRequestHeaders. When denied or when the
// guest does not export the callback, returns ProxyActionContinue + nil.
func (vm *VM) CallProxyOnRequestHeaders(ctx context.Context, streamContextID, numHeaders uint32, endOfStream bool) (abi.ProxyAction, error) {
	vm.setCurrentCtx(streamContextID, ctx)

	if !vm.sandbox.IsAllowed(capProxyOnRequestHeaders) {
		return abi.ProxyActionContinue, nil
	}
	vm.mu.Lock()
	inst := vm.instance
	vm.mu.Unlock()
	if inst == nil {
		return abi.ProxyActionContinue, errors.New("wasm: CallProxyOnRequestHeaders before Run")
	}
	fn := inst.ExportedFunction("proxy_on_request_headers")
	if fn == nil {
		return abi.ProxyActionContinue, nil
	}

	var endOfStreamU uint64
	if endOfStream {
		endOfStreamU = 1
	}

	var action abi.ProxyAction
	err := vm.runCallWithPanicWrapper(func() error {
		results, err := fn.Call(ctx, uint64(streamContextID), uint64(numHeaders), endOfStreamU)
		if err != nil {
			return err
		}
		if len(results) > 0 {
			//nolint:gosec // wazero call results are bit-typed by us; truncation is the wire-protocol intent.
			action = abi.ProxyAction(int32(results[0]))
		}
		return nil
	})
	return action, err
}

// CallProxyOnResponseHeaders invokes `proxy_on_response_headers(streamContextID,
// numHeaders, endOfStream)` — returns `ProxyAction::CONTINUE` (=0) or
// `::PAUSE` (=1). Gated by capProxyOnResponseHeaders. When denied or when the
// guest does not export the callback, returns ProxyActionContinue + nil.
func (vm *VM) CallProxyOnResponseHeaders(ctx context.Context, streamContextID, numHeaders uint32, endOfStream bool) (abi.ProxyAction, error) {
	vm.setCurrentCtx(streamContextID, ctx)

	if !vm.sandbox.IsAllowed(capProxyOnResponseHeaders) {
		return abi.ProxyActionContinue, nil
	}
	vm.mu.Lock()
	inst := vm.instance
	vm.mu.Unlock()
	if inst == nil {
		return abi.ProxyActionContinue, errors.New("wasm: CallProxyOnResponseHeaders before Run")
	}
	fn := inst.ExportedFunction("proxy_on_response_headers")
	if fn == nil {
		return abi.ProxyActionContinue, nil
	}

	var endOfStreamU uint64
	if endOfStream {
		endOfStreamU = 1
	}

	var action abi.ProxyAction
	err := vm.runCallWithPanicWrapper(func() error {
		results, err := fn.Call(ctx, uint64(streamContextID), uint64(numHeaders), endOfStreamU)
		if err != nil {
			return err
		}
		if len(results) > 0 {
			//nolint:gosec // wazero call results are bit-typed by us; truncation is the wire-protocol intent.
			action = abi.ProxyAction(int32(results[0]))
		}
		return nil
	})
	return action, err
}

// CallProxyOnDone invokes `proxy_on_done(streamContextID)` → bool.
// Returning false defers finalize (host returns CONTINUE on the wire per
// SPEC §3.1). Gated by capProxyOnDone. When denied or when the guest does
// not export the callback, returns (true, nil) — "done" is the default.
func (vm *VM) CallProxyOnDone(ctx context.Context, streamContextID uint32) (bool, error) {
	vm.setCurrentCtx(streamContextID, ctx)

	if !vm.sandbox.IsAllowed(capProxyOnDone) {
		return true, nil
	}
	vm.mu.Lock()
	inst := vm.instance
	vm.mu.Unlock()
	if inst == nil {
		return true, errors.New("wasm: CallProxyOnDone before Run")
	}
	fn := inst.ExportedFunction("proxy_on_done")
	if fn == nil {
		return true, nil
	}

	var done bool
	err := vm.runCallWithPanicWrapper(func() error {
		results, err := fn.Call(ctx, uint64(streamContextID))
		if err != nil {
			return err
		}
		if len(results) > 0 {
			done = results[0] != 0
		} else {
			done = true
		}
		return nil
	})
	return done, err
}

// CallProxyOnLog invokes `proxy_on_log(streamContextID)`. Gated by
// capProxyOnLog. When denied or when the guest does not export the callback,
// returns nil (no-op success).
func (vm *VM) CallProxyOnLog(ctx context.Context, streamContextID uint32) error {
	vm.setCurrentCtx(streamContextID, ctx)

	if !vm.sandbox.IsAllowed(capProxyOnLog) {
		return nil
	}
	vm.mu.Lock()
	inst := vm.instance
	vm.mu.Unlock()
	if inst == nil {
		return errors.New("wasm: CallProxyOnLog before Run")
	}
	fn := inst.ExportedFunction("proxy_on_log")
	if fn == nil {
		return nil
	}

	return vm.runCallWithPanicWrapper(func() error {
		_, err := fn.Call(ctx, uint64(streamContextID))
		return err
	})
}

// CallProxyOnDelete invokes `proxy_on_delete(streamContextID)`. Gated by
// capProxyOnDelete. When denied or when the guest does not export the
// callback, returns nil (no-op success). After this call returns, the
// streamContextID is removed from vm.ctxStore.
func (vm *VM) CallProxyOnDelete(ctx context.Context, streamContextID uint32) error {
	vm.setCurrentCtx(streamContextID, ctx)

	defer func() {
		vm.mu.Lock()
		delete(vm.ctxStore, streamContextID)
		vm.mu.Unlock()
	}()

	if !vm.sandbox.IsAllowed(capProxyOnDelete) {
		return nil
	}
	vm.mu.Lock()
	inst := vm.instance
	vm.mu.Unlock()
	if inst == nil {
		return errors.New("wasm: CallProxyOnDelete before Run")
	}
	fn := inst.ExportedFunction("proxy_on_delete")
	if fn == nil {
		return nil
	}

	return vm.runCallWithPanicWrapper(func() error {
		_, err := fn.Call(ctx, uint64(streamContextID))
		return err
	})
}

// Close releases the VM's wazero.Runtime. Idempotent — second Close
// returns nil with no side effects.
func (vm *VM) Close() error {
	vm.mu.Lock()
	if vm.closed {
		vm.mu.Unlock()
		return nil
	}
	vm.closed = true
	rt := vm.runtime
	vm.runtime = nil
	vm.instance = nil
	vm.module = nil
	vm.ctxStore = nil
	vm.mu.Unlock()

	if rt == nil {
		return nil
	}
	return rt.Close(context.Background())
}

// IsAllowed forwards to vm.sandbox.IsAllowed; satisfies the wasiHost
// interface (Task 4 wasi.go) so the VM can pass itself to the WASI shim
// wrappers at registration time.
func (vm *VM) IsAllowed(capabilityName string) bool {
	return vm.sandbox.IsAllowed(capabilityName)
}

// LogProxy routes a log line to vm.logSink (if set); satisfies the wasiHost
// interface (Task 4 wasi.go). Default nil sink ⇒ drop, matching the
// envoy-go-strict "no stdout leak" discipline.
func (vm *VM) LogProxy(level abi.LogLevel, msg string) {
	if vm.logSink == nil {
		return
	}
	// Sink write errors are intentionally discarded — the log sink is a
	// best-effort observability channel; failing to write a log line MUST
	// NOT propagate as an error to the wasm guest or the filter chain.
	_, _ = fmt.Fprintf(vm.logSink, "[wasm %s] %s\n", logLevelString(level), msg)
}

// logLevelString returns the upstream proxy-wasm level-name string for the
// LogProxy + proxy_log integration sink formatting.
func logLevelString(l abi.LogLevel) string {
	switch l {
	case abi.LogLevelTrace:
		return "trace"
	case abi.LogLevelDebug:
		return "debug"
	case abi.LogLevelInfo:
		return "info"
	case abi.LogLevelWarn:
		return "warn"
	case abi.LogLevelError:
		return "error"
	case abi.LogLevelCritical:
		return "critical"
	default:
		return fmt.Sprintf("level(%d)", l)
	}
}

// setCurrentCtx records the streamContextID as the active context for
// subsequent hostcall dispatch + stores the Go context in ctxStore. Called
// at entry of every CallProxyOnX method.
func (vm *VM) setCurrentCtx(streamContextID uint32, ctx context.Context) {
	vm.currentCtxID.Store(streamContextID)
	vm.mu.Lock()
	if vm.ctxStore != nil {
		vm.ctxStore[streamContextID] = ctx
	}
	vm.mu.Unlock()
}

// runWithPanicWrapper runs fn under a panic-wrapper that converts a
// recovered panic into the named WasmResult sentinel + invokes the
// configured PanicHandlerFn (if any). Used by the hostcall bodies in
// registration.go to convert Go-side panics in ABICallbacks methods into
// `abi.WasmResultInternalFailure` (=10) returns.
//
// Per AMEND-A8: the panic-wrapper is for genuine Go panics from bridge
// callbacks; wazero-side traps return via the Call error path and are
// NOT recovered here (they propagate to the per-callback caller).
func (vm *VM) runWithPanicWrapper(fn func() abi.WasmResult) (result abi.WasmResult) {
	defer func() {
		if r := recover(); r != nil {
			if vm.panicH != nil {
				vm.panicH(r)
			}
			result = abi.WasmResultInternalFailure
		}
	}()
	return fn()
}

// runCallWithPanicWrapper is the error-returning variant of
// runWithPanicWrapper, used by the per-callback CallProxyOnX methods. A
// recovered Go panic converts to a wrapped error so the per-callback caller
// observes a non-nil err.
func (vm *VM) runCallWithPanicWrapper(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if vm.panicH != nil {
				vm.panicH(r)
			}
			err = fmt.Errorf("wasm: recovered panic in guest call: %v", r)
		}
	}()
	return fn()
}
