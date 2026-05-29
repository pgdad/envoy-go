// RootVM lifecycle + options + per-RootVM state per 25.2 SPEC §3.1 +
// ADR-0205 + Task 1.
//
// The 25.2 root-VM architecture RETIRES the 25.1 per-stream `*VM` model
// (each stream constructing its own wazero.Runtime at 61µs/stream per the
// 25.1 Task 17 R8 benchmark) in favor of ONE long-lived `*RootVM` per
// `*compiledConfig` (upstream-byte-faithful per cpp-host `Wasm`/`Plugin`
// model) + per-stream contexts as CHILDREN sharing the same wazero
// Runtime + Module. Per-stream-context construction cost is microseconds
// (just bookkeeping + a `proxy_on_context_create(streamCtxID, rootCtxID)`
// invocation on the shared Module instance); no wazero Runtime construction.
//
// Per Q3 + ADR-0205 the `*RootVM` owns:
//   - tick goroutine + tick state (stub at this Task; activation in Task 5)
//   - shared-data map (stub; activation in Task 6)
//   - httpCall response routing + call_id allocation (stub; activation in Task 8)
//   - foreign-function registry view (stub; activation in Task 7)
//   - per-plugin dynamic-stats `*Registry` (stub; activation in Task 12)
//   - per-stream-context map (the live children sharing this RootVM)
//
// # File-split boundary at this Task
//
//   - root_vm.go (THIS FILE) — RootVM type, options, lifecycle, Configure,
//     NewStreamContext, Close; the panic-wrapper helpers + setCurrentCtx
//     context-key pattern + logLevelString helper migrated from 25.1 vm.go;
//     wasiHost satisfaction (IsAllowed + LogProxy).
//   - stream_context.go — StreamContext type + per-callback methods +
//     Close (idempotent; fires proxy_on_done + proxy_on_log +
//     proxy_on_delete + STUB cancel-at-destruction for outstanding httpCalls).
//   - registration.go — ABICallbacks interface + host-module wiring; the
//     existing host-call surface is UNCHANGED at this Task. The
//     `registerHostModules` entry-point is re-anchored on `*RootVM` (the
//     replacement for `*VM`).
//
// # Lifecycle gate disposition (per 25.1 SPEC §3.3 — carries forward UNCHANGED)
//
// The 8 lifecycle/HTTP module-getter capability keys + the 5 ungated
// module-init/allocator keys disposition is INHERITED VERBATIM from 25.1
// vm.go. Configure invokes _initialize / _start UNGATED + the lifecycle
// callbacks GATED via SandboxConfig.IsAllowed. NewStreamContext invokes
// proxy_on_context_create GATED via capProxyOnContextCreate; per-stream
// callbacks at stream_context.go GATED via the corresponding capProxyOn*.
//
// # currentCtxID tracking
//
// The per-RootVM "effective" context id for hostcall dispatch (the same
// atomic Uint32 pattern as 25.1). See the doc-comment on the currentCtxID
// field below for the Store/Load contract + the 4-site mutator roster
// (StreamContext.CallProxyOnX + RootVM.Configure + proxy_set_effective_-
// context hostcall + Task 5/8 tick + httpCall dispatch).

package wasm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/esalaine/envoy-go/internal/clock"
	"github.com/esalaine/envoy-go/internal/stats/dynamic"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// RootVM is the per-compiledConfig long-lived VM. ONE per *compiledConfig
// (upstream-byte-faithful per cpp-host Wasm/Plugin model). Owns: tick
// goroutine + tick state; shared-data map; httpCall response routing +
// call_id allocation; foreign-function registry view; the per-plugin
// dynamic-stats *Registry; the per-stream-context map.
//
// Goroutine-safety: RootVM is intended for the multi-goroutine setting (one
// goroutine per HTTP stream + tick goroutine + httpCall callback goroutine).
// The dispatchMu serializes proxy_on_* + foreign-function dispatch per
// D-P-PLAN-9. streamCtxsMu guards the streamCtxs map. closed flag + Close
// path guarded by sync.Once.
type RootVM struct {
	runtime  wazero.Runtime        // single per-RootVM
	module   wazero.CompiledModule // re-compiled against runtime
	instance api.Module            // instantiated once at NewRootVM
	src      []byte                // retained for cross-runtime re-compile (from *Module.Source())

	// vmConfigBytes / pluginConfigBytes: the Configure-time argument bytes,
	// retained so a FAIL_RELOAD re-instantiation (reinstantiate) can replay
	// proxy_on_vm_start / proxy_on_configure with the same config. Stored by
	// Configure; read by reinstantiate. Both run under dispatchMu so the
	// fields need no separate guard (phase-25.3 Task 3).
	vmConfigBytes     []byte
	pluginConfigBytes []byte

	sandbox   SandboxConfig
	panicH    PanicHandlerFn
	logSink   io.Writer
	rootCtxID uint32 // typically 1

	cb ABICallbacks // consumer-side callback bundle (set once at RegisterABICallbacks; reads are unlocked per 25.1 vm.go precedent)

	// currentCtxID tracks the "effective" context id for hostcall dispatch.
	// Store-side: mutated only by per-callback methods on *StreamContext
	// (CallProxyOnX paths) + by *RootVM lifecycle dispatch (Configure) +
	// (Tasks 5+8) by the tick goroutine + httpCall response goroutine — all
	// of which serialize via dispatchMu. Load-side: read unlocked by the
	// hostcall bodies in registration.go (and Tasks 4-8+12 hostcall bodies)
	// which always execute synchronously INSIDE the dispatchMu-held caller
	// frame, so the Load is a relaxed load by construction (no acquire/
	// release fence needed). Tasks 5+ tick + Task 8 httpCall response will
	// Store from a goroutine distinct from the per-stream dispatchers; the
	// dispatchMu serialization guarantees the Store-then-Call-then-Load
	// ordering still holds. See "currentCtxID tracking" in the file header
	// for the canonical mutator roster (4 sites).
	currentCtxID atomic.Uint32

	// compilationCache, if non-nil, wires into the RootVM's wazero.Runtime
	// at NewRootVM via wazero.NewRuntimeConfigInterpreter().WithCompilationCache(cc).
	// Production wiring: pass the cache exposed by the *CompileCache that
	// produced the *Module so the RootVM's re-compile of module.Source() hits
	// the shared cache sub-ms.
	compilationCache wazero.CompilationCache

	// shared-data state (activated at Task 6 per Q6 + R-25.2-10; broadened
	// to vm_id scope at Task 2 per AMEND-C2):
	//
	//   - sharedDataValCap / sharedDataMaxEntries: envoy-go-strict caps per
	//     AMEND-B5. Zero ⇒ defaults applied at first use (1 MiB / 1024 per
	//     shared_data.go SharedData{Value,MaxEntries}CapDefault constants).
	//     Operator overrides flow via WithRootSharedDataCaps OR the Task 14
	//     per-plugin Configure-time field-set.
	//
	//   - sharedData: the *sharedDataStore backing this RootVM. By default
	//     (no WithSharedDataStore option) NewRootVM allocates a private store
	//     so each *RootVM is isolated (pre-25.3 behavior). Task 7 wires the
	//     registry-provided vm_id-scoped store here so plugins sharing a
	//     vm_id observe one namespace (AMEND-C2). The entry struct + store
	//     methods + SetSharedData / GetSharedData thin wrappers live at
	//     shared_data.go.
	sharedDataValCap     uint32 // from PluginConfig envoy_go_strict_shared_data_value_cap_bytes; 0 ⇒ default
	sharedDataMaxEntries uint32 // from PluginConfig envoy_go_strict_shared_data_max_entries; 0 ⇒ default
	sharedData           *sharedDataStore

	// tick state (per Q5 + ADR-0186 Clock seam; activated at Task 5):
	//
	//   - clk: the Clock seam used by the tick goroutine. Default RealClock
	//     (production wiring); tests inject clock.NewFakeClock via
	//     WithRootClock. FIRST co-consumer of phase-21 ADR-0186 Clock seam
	//     beyond phase-21 itself per Q5 + R-25.2-9 — RATIFIES the
	//     EXTRACT-NOW trigger fired at §Consequences (g). Set once at
	//     NewRootVM (option application + RealClock default); read by the
	//     tick goroutine unlocked (no mutation after construction).
	//
	//   - tickHandler: optional Go-side per-tick hook fired from inside
	//     dispatchMu BEFORE proxy_on_tick dispatch. Test-observability seam
	//     used by tick_test.go fixtures; production callers leave it nil.
	//     Set once at NewRootVM via WithRootTickHandler.
	//
	//   - tickPeriod / tickStop / tickWG: managed by tick.go SetTickPeriod
	//     / tickRun / stopTickGoroutine. tickMu serializes ALL re-schedule
	//     events. AT MOST ONE tick goroutine alive at a time per *RootVM
	//     (Q5 anti-tick-storm invariant — N concurrent NewStreamContext
	//     calls share the same tick goroutine).
	clk         clock.Clock
	tickHandler func()
	tickPeriod  time.Duration // 0 = no tick scheduled
	tickStop    chan struct{} // nil when no tick goroutine alive
	tickMu      sync.Mutex    // serializes SetTickPeriod re-schedule
	tickWG      sync.WaitGroup

	// reload state (phase-25.3 Task 3 per AMEND-C3 + R-25.3-2/5 + D-25.3-P3):
	//
	//   - reload: the per-*RootVM reload state machine (reload.go). Models the
	//     Running → Failed → (backoff | attempt) → Running transitions for
	//     failure_policy = FAIL_RELOAD. Constructed in NewRootVM from rv.clk +
	//     rv.reloadBaseInterval AFTER the clk default is applied. The
	//     dispatchReload hook (serialized by dispatchMu) drives it; the
	//     reinstantiate primitive is the production reattempt. The END-TO-END
	//     trigger from the filter DecodeHeaders dispatch path lands at Task 9.
	//
	//   - reloadBaseInterval: the operator-configured backoff base interval
	//     (0 ⇒ 1s default; <100ms ⇒ 100ms floor). Set via WithReloadConfig;
	//     read once in NewRootVM to build rv.reload.
	//
	//   - reloadStats: the optional vm_reload counter seam (reload.go
	//     reloadStatsHook). nil-default; set via WithReloadStats so tests +
	//     Task 9 can inject the triplet. dispatchReload guards with a nil-check.
	reload             *reloadState
	reloadBaseInterval time.Duration
	reloadStats        reloadStatsHook

	// per-stream-context map (per Q3 — children sharing root VM):
	streamCtxsMu    sync.RWMutex
	streamCtxs      map[uint32]*StreamContext // streamCtxID → StreamContext
	nextStreamCtxID atomic.Uint32             // monotonic per-RootVM

	// httpCall state (Task 8 per Q4 + R-25.2-3 + AMEND-B3 + ADR-0177 RE-CONSUMER):
	//
	//   - httpDispatcher: the per-*RootVM outbound-HTTP dispatch surface (see
	//     http_call.go HTTPDispatcher interface). RE-CONSUMES phase-20 ADR-0177
	//     internal/httpclient/ at 3rd-or-later co-consumer (phase-22.2 lua
	//     :httpCall() was second) — CLOSES parent SPEC §13-R6 RATIFIED-PENDING-
	//     IMPL anchor. NO API extension on httpclient (phase-22.2 cluster-based
	//     dispatch covers byte-for-byte). Nil-default ⇒ proxy_http_call returns
	//     InternalFailure (no dispatcher wired); production wiring at Task 14
	//     supplies a concrete adapter; tests inject a fake via
	//     WithRootHTTPDispatcher. Set once at NewRootVM; read unlocked from
	//     DispatchHttpCall (no mutation after construction).
	//
	//   - httpCalls: the per-*RootVM map of in-flight proxy_http_call entries
	//     (call_id → pendingHttpCall). Activated at Task 8; lazy-init on first
	//     DispatchHttpCall to keep zero-config RootVMs allocation-free.
	//
	//   - httpCallsMu: sync.Mutex guarding the httpCalls map + nextCallID. A
	//     DIFFERENT mutex from dispatchMu — lock order is dispatchMu →
	//     httpCallsMu (the reverse acquisition would deadlock the response
	//     goroutine vs the DispatchHttpCall caller frame).
	//
	//   - nextCallID: monotonic per-*RootVM call_id allocator. Guarded by
	//     httpCallsMu (allocated under the same lock as the map insert to keep
	//     ID-allocation + map-insert atomic).
	httpDispatcher HTTPDispatcher
	httpCalls      map[uint32]*pendingHttpCall
	httpCallsMu    sync.Mutex
	nextCallID     uint32

	// foreign-function registry (Task 7 per AMEND-A9 + R-25.2-8 + D-P-PLAN-9):
	//
	//   - foreignReg: the registry consulted by CallForeignFunction. Set once
	//     at NewRootVM via WithRootForeignRegistry; if no option is supplied,
	//     defaults to DefaultForeignFunctionRegistry (the process-global
	//     EMPTY-by-default registry per AMEND-A9). Read unlocked from
	//     CallForeignFunction (no mutation after construction).
	//
	// D-P-PLAN-9 RATIFIED dispatch concurrency: synchronous invocation inside
	// the per-stream dispatchMu-held frame; the foreign function executes
	// while dispatchMu is held (the caller is the hostcall body in registration.go
	// which acquires dispatchMu via the per-StreamContext.CallProxyOnX path);
	// panic-recovery wrapper applies (Go panic → recover() → InternalFailure +
	// PanicHandlerFn fires); foreign_function_denied counter increments on
	// NotFound path (counter wiring deferred Task 17). See foreign.go for the
	// canonical 5-clause model + the 2 alternatives REJECTED at PLAN session.
	foreignReg *ForeignFunctionRegistry

	// dynamic-stats wrapper (Task 12 per AMEND-B2 + R-25.2-2 + R-25.2-7 +
	// ADR-0208):
	//
	//   - dynStats: the per-plugin *dynamic.Registry consulted by the four
	//     metric methods (DefineMetric / IncrementMetric / RecordMetric /
	//     GetMetric in dynamic_stats.go) which are in turn the host-side
	//     dispatch targets for the abi/metrics.go shims. Set once at
	//     NewRootVM via WithRootDynamicStats; nil-default means every
	//     metric call returns InternalFailure (envoy-go-strict default-deny
	//     posture — operator opts in by wiring the per-plugin Registry at
	//     compiledConfig construction at Task 14). Read unlocked from the
	//     metric methods (no mutation after construction); the underlying
	//     *dynamic.Registry has its own sync.RWMutex covering Register
	//     mutations + lock-free atomics on the Increment/Record/Get hot
	//     path.
	dynStats *dynamic.Registry

	// env is the assembled environment-variable map fed to the guest's WASI
	// environ_get / environ_sizes_get shims as KEY=VALUE\0 entries per
	// AMEND-C4. Set once at NewRootVM via WithRootEnv (after AssembleEnvVars
	// collision/cap validation at Task 7 from VmConfig.environment_variables).
	// nil/empty ⇒ the guest sees zero env entries (the 25.1/25.2 behavior).
	// Read unlocked from WASIEnviron (called during guest dispatch; no
	// mutation after construction).
	env map[string]string

	// stats is the per-*RootVM RootStatsRecorder counter sink per 25.2 IMPL
	// Task 20 follow-up (Concern 2 — counter-glue wiring gap). Wired ONCE at
	// NewRootVM via WithRootStats; defaults to noopStatsRecorder if no
	// option supplied. ALL hostcall bodies that increment one of the 9 NEW
	// envoy-go-strict counters per §7.1 + AMEND-B3 call rv.stats.XxxInc()
	// unconditionally (the no-op default + the nil-guard at WithRootStats
	// ensure rv.stats is NEVER nil — eliminates per-call nil-checks).
	//
	// Production wiring (compiled_config.New): *filterStats satisfies
	// RootStatsRecorder via thin wrappers; the recorder is passed via
	// WithRootStats(cfg.stats). See internal/wasm/stats_recorder.go for
	// the interface contract + 9-counter roster.
	stats RootStatsRecorder

	// dispatchMu serializes ALL proxy_on_* invocations + foreign-function
	// dispatch per D-P-PLAN-9. Concretely it is held by: Configure-time
	// lifecycle dispatch + per-StreamContext CallProxyOnX + (Task 5) the
	// tick goroutine entry + (Task 7) foreign-function dispatch entry +
	// (Task 8) the httpCall response goroutine entry. At Task 1 only
	// Configure + per-stream callback dispatch acquire it; the Task 5/7/8
	// callers are STUB at this Task and acquire it at their respective
	// activation Tasks.
	//
	// NO RE-ENTRANCY. Hostcall bodies (registration.go + abi/*) and
	// foreign-function callbacks execute INSIDE the held frame and MUST NOT
	// re-acquire dispatchMu — sync.Mutex in Go is non-reentrant and would
	// deadlock. Reentrant ACCESS (reading currentCtxID / cb / streamCtxs)
	// from within a held frame is by-construction safe because the lock-
	// holder is the goroutine doing the work; the prohibition is on a
	// second .Lock() call from within the same held frame.
	dispatchMu sync.Mutex

	closeOnce sync.Once
	closed    atomic.Bool
}

// RootVMOption configures RootVM construction. Function-option pattern per
// 25.1 VMOption precedent (and the broader internal/lua + internal/jwks +
// internal/httpclient option-pattern precedent).
type RootVMOption func(*RootVM)

// WithRootSandboxConfig sets the per-capability ALLOW/DENY posture. Zero
// value (default) = `StrictDefaultDeny` per AMEND-A5 — DENY ALL hostcalls.
// See `SandboxConfig` doc for the full discipline.
func WithRootSandboxConfig(sb SandboxConfig) RootVMOption {
	return func(rv *RootVM) { rv.sandbox = sb }
}

// WithRootPanicHandler sets the Go-panic handler invoked after recover() in
// the RootVM's panic-wrapper. The handler is invoked with the recovered
// value. Not for catching wazero-side traps (those return via sys.ExitError
// or wasmruntime.Error); the handler is for genuine Go panics from hostcall
// Go callbacks per AMEND-A8.
func WithRootPanicHandler(h PanicHandlerFn) RootVMOption {
	return func(rv *RootVM) { rv.panicH = h }
}

// WithRootLogSink redirects `proxy_log` + WASI `fd_write` output for the
// lifetime of this RootVM. Default nil = drop (no stdout leak; envoy-go-strict
// default).
func WithRootLogSink(w io.Writer) RootVMOption {
	return func(rv *RootVM) { rv.logSink = w }
}

// WithRootCompilationCache configures the RootVM's underlying wazero.Runtime
// with a shared wazero.CompilationCache (the wazero-codegen-result cache,
// distinct from envoy-go's *CompileCache Go-level *Module store). When set,
// NewRootVM constructs the runtime via
// `wazero.NewRuntimeConfigInterpreter().WithCompilationCache(cc)`; when
// unset (default) the runtime is constructed without a cache and pays full
// codegen cost on every NewRootVM construction.
//
// Production wiring: pass the cache exposed by the *CompileCache that
// produced the *Module so the RootVM's re-compile of module.Source() hits
// the shared cache sub-ms.
func WithRootCompilationCache(cc wazero.CompilationCache) RootVMOption {
	return func(rv *RootVM) { rv.compilationCache = cc }
}

// WithRootClock injects the time-source seam used by the per-*RootVM tick
// goroutine (tick.go). Default RealClock (clock.RealClock — delegates to
// time.Now + time.After); tests inject clock.NewFakeClock for deterministic
// step-driven time control.
//
// FIRST co-consumer of phase-21 ADR-0186 Clock seam beyond phase-21 itself
// per Q5 + R-25.2-9 — RATIFIES the EXTRACT-NOW trigger fired at
// §Consequences (g). The framework-level internal/clock/ package is the
// lifted surface; the existing inline
// internal/filter/http/adaptive_concurrency/clock.go remains unchanged at
// 25.2 (a follow-up refactor may migrate it).
func WithRootClock(c clock.Clock) RootVMOption {
	return func(rv *RootVM) { rv.clk = c }
}

// WithReloadConfig sets the FAIL_RELOAD backoff base interval per phase-25.3
// Task 3 (AMEND-C3 + R-25.3-2/5). 0 (default) ⇒ 1s; 0 < base < 100ms ⇒ 100ms
// floor; base >= 100ms ⇒ base verbatim. Read once in NewRootVM to build the
// per-*RootVM reload state machine (reload.go). Production wiring lands at
// Task 7/9 from the parsed PluginConfig.
func WithReloadConfig(base time.Duration) RootVMOption {
	return func(rv *RootVM) { rv.reloadBaseInterval = base }
}

// WithReloadStats installs the optional vm_reload counter seam (reload.go
// reloadStatsHook: VmReloadSuccessInc / VmReloadRuntimeFailureInc /
// VmReloadBackoffInc) per phase-25.3 Task 3. nil-default; dispatchReload
// guards with a nil-check. Tests + Task 9 inject the triplet; the LOCAL hook
// interface keeps this Task from forcing RootStatsRecorder to grow before
// Task 8.
func WithReloadStats(h reloadStatsHook) RootVMOption {
	return func(rv *RootVM) { rv.reloadStats = h }
}

// WithRootTickHandler installs an optional Go-side per-tick hook fired from
// inside the dispatchMu frame on every tick fire, BEFORE the proxy_on_tick
// guest dispatch. The handler runs under panic-recovery (a handler panic
// is recovered; PanicHandlerFn fires; the tick goroutine survives).
//
// Test-observability seam used by tick_test.go fixtures (assert tick
// invocation count + cadence under FakeClock). Production callers leave it
// nil; the proxy_on_tick guest dispatch is the production tick effect.
//
// The handler MUST NOT re-acquire dispatchMu — it executes INSIDE the held
// frame (sync.Mutex is non-reentrant in Go).
func WithRootTickHandler(h func()) RootVMOption {
	return func(rv *RootVM) { rv.tickHandler = h }
}

// WithRootForeignRegistry installs a per-*RootVM ForeignFunctionRegistry,
// overriding the process-global DefaultForeignFunctionRegistry. Used by
// tests (which inject a fresh empty registry per test for isolation) +
// multi-tenant setups (where two RootVMs need different foreign-function
// rosters). If no option is supplied at NewRootVM, the *RootVM consults
// DefaultForeignFunctionRegistry (the EMPTY-by-default global per AMEND-A9).
//
// Per D-P-PLAN-9: the registry is consulted via RLock-on-Get from
// CallForeignFunction; concurrent Get from multiple goroutines proceeds.
// Mutation (Register) is typically boot-time only; mid-flight Register is
// permitted but blocks Get for the Lock-held duration.
func WithRootForeignRegistry(reg *ForeignFunctionRegistry) RootVMOption {
	return func(rv *RootVM) { rv.foreignReg = reg }
}

// WithRootDynamicStats installs the per-plugin *dynamic.Registry consulted
// by the four metric methods on *RootVM (DefineMetric / IncrementMetric /
// RecordMetric / GetMetric in dynamic_stats.go) which are the host-side
// dispatch targets for the abi/metrics.go shims (proxy_define_metric /
// proxy_increment_metric / proxy_record_metric / proxy_get_metric per
// 25.2 SPEC §5.1 #31-34 + AMEND-B2 + R-25.2-2).
//
// Per ADR-0208 + R-25.2-7 the per-plugin *dynamic.Registry is constructed
// at compiledConfig.New time (Task 14) from the shared *stats.Registry +
// the per-plugin scope prefix (typically "wasm.<plugin_name>") + the
// envoy-go-strict cap (default 1024 per AMEND-B2 + Q9). Tests inject a
// fresh Registry per test for isolation.
//
// Nil-default semantic: a *RootVM constructed WITHOUT this option has
// dynStats == nil; every metric method returns InternalFailure
// (envoy-go-strict default-deny — operator opts in by wiring the Registry).
// Set once at NewRootVM; read unlocked from the metric methods.
func WithRootDynamicStats(reg *dynamic.Registry) RootVMOption {
	return func(rv *RootVM) { rv.dynStats = reg }
}

// WithRootSharedDataCaps configures the per-value + per-map entry caps for
// the shared-data store. Per AMEND-B5 envoy-go-strict envelope: per-value
// 1 MiB cap (configurable via `envoy_go_strict_shared_data_value_cap_bytes`;
// default 1048576); 1024-entry cap (configurable via
// `envoy_go_strict_shared_data_max_entries`; default 1024).
//
// STATE: field-set only at this Task; activation at Task 6 shared_data.go.
func WithRootSharedDataCaps(valCap, maxEntries uint32) RootVMOption {
	return func(rv *RootVM) {
		rv.sharedDataValCap = valCap
		rv.sharedDataMaxEntries = maxEntries
	}
}

// WithSharedDataStore injects a shared (registry-owned, vm_id-scoped)
// shared-data store. When unset, NewRootVM allocates a private store so
// each *RootVM is isolated (the pre-25.3 behavior). Task 7 wires the
// registry-provided store here so plugins under one vm_id share a namespace.
func WithSharedDataStore(store *sharedDataStore) RootVMOption {
	return func(rv *RootVM) { rv.sharedData = store }
}

// WithRootEnv sets the assembled environment-variable map fed to the guest's
// WASI environ_get/environ_sizes_get shims as KEY=VALUE\0 entries per AMEND-C4.
// Wired by compiledConfig at Task 7 from VmConfig.environment_variables (after
// AssembleEnvVars collision/cap validation). Unset ⇒ the guest sees zero env
// entries (the 25.1/25.2 behavior).
func WithRootEnv(env map[string]string) RootVMOption { return func(rv *RootVM) { rv.env = env } }

// PanicHandlerFn is invoked after `recover()` in the RootVM's panic-wrapper.
// `recovered` is the value returned by recover() (typically the panic value).
type PanicHandlerFn func(recovered any)

// NewRootVM constructs the per-compiledConfig long-lived VM. Applies sandbox
// config + creates the underlying wazero.Runtime in interpreter mode per
// parent §2.7 + registers the env-namespace + wasi_snapshot_preview1-
// namespace host modules + re-compiles module.Source() against the new
// runtime + instantiates the module once + records the rootContextID.
//
// Caller responsibility: release via Close at compiledConfig shutdown.
// Returns a wrapped error on (a) registration failure, (b) re-compile
// failure, (c) instantiation failure. NewRootVM does NOT invoke
// _initialize / _start / proxy_on_vm_start — those fire at Configure.
//
// At 25.2 the rootCtxID is taken as an explicit argument (typically 1 per
// upstream convention) — the caller (compiledConfig.New at Task 14)
// allocates it via its own monotonic root-context-id counter, preserving
// the 25.1 separation between per-plugin root + per-stream context id
// namespaces.
func NewRootVM(ctx context.Context, module *Module, rootCtxID uint32, opts ...RootVMOption) (*RootVM, error) {
	if module == nil {
		return nil, errors.New("wasm: NewRootVM: nil *Module")
	}
	src := module.Source()
	if src == nil {
		return nil, errors.New("wasm: NewRootVM: *Module has no retained Source() bytes")
	}

	rv := &RootVM{
		streamCtxs: make(map[uint32]*StreamContext),
		rootCtxID:  rootCtxID,
	}
	for _, opt := range opts {
		opt(rv)
	}
	// Default the Clock seam to RealClock if no WithRootClock option was
	// applied. The tick goroutine (tick.go) reads rv.clk unlocked after
	// construction; we set it here so test fixtures + production wiring
	// share the same code path. FIRST co-consumer of phase-21 ADR-0186
	// Clock seam per Q5 + R-25.2-9.
	if rv.clk == nil {
		rv.clk = clock.RealClock{}
	}
	// Construct the reload state machine AFTER the clk default is applied so
	// it shares the (possibly fake) clock. Per phase-25.3 Task 3 + D-25.3-P3.
	if rv.reload == nil {
		rv.reload = newReloadState(rv.clk, rv.reloadBaseInterval)
	}
	// Default the foreign-function registry to the process-global EMPTY
	// registry if no WithRootForeignRegistry option was applied. Per AMEND-A9
	// + D-P-PLAN-9: envoy-go ships ZERO default foreign functions; operators
	// register via wasm.RegisterForeignFunction(name, fn) at boot. The
	// per-RootVM override exists for test isolation + multi-tenant setups.
	if rv.foreignReg == nil {
		rv.foreignReg = DefaultForeignFunctionRegistry
	}
	// Default the RootStatsRecorder to noopStatsRecorder if no WithRootStats
	// option was applied. Per 25.2 IMPL Task 20 follow-up (Concern 2) the
	// no-op default ensures rv.stats is NEVER nil — hostcall bodies call
	// rv.stats.XxxInc() unconditionally without nil-check guards. Production
	// wiring at compiled_config.New passes WithRootStats(cfg.stats) where
	// *filterStats satisfies the interface via thin wrappers.
	if rv.stats == nil {
		rv.stats = noopStatsRecorder{}
	}
	// Default the shared-data store to a fresh private store if no
	// WithSharedDataStore option was applied. This preserves the pre-25.3
	// per-RootVM isolation so all existing tests stay green. Task 7 wires
	// the registry-provided vm_id-scoped store here (AMEND-C2).
	if rv.sharedData == nil {
		rv.sharedData = newSharedDataStore()
	}

	runtimeConfig := wazero.NewRuntimeConfigInterpreter()
	if rv.compilationCache != nil {
		runtimeConfig = runtimeConfig.WithCompilationCache(rv.compilationCache)
	}
	rv.runtime = wazero.NewRuntimeWithConfig(ctx, runtimeConfig)

	if err := registerHostModules(ctx, rv); err != nil {
		// Defensive: release the runtime to avoid a leak before returning.
		_ = rv.runtime.Close(context.Background())
		return nil, fmt.Errorf("wasm: NewRootVM: registerHostModules: %w", err)
	}

	// Re-compile module.Source() against this RootVM's runtime. Sub-ms cache
	// hit when WithRootCompilationCache(cache.WazeroCompilationCache()) was
	// wired at NewRootVM time; otherwise full codegen.
	compiled, err := rv.runtime.CompileModule(ctx, src)
	if err != nil {
		_ = rv.runtime.Close(context.Background())
		return nil, fmt.Errorf("wasm: NewRootVM: re-compile for root VM: %w", err)
	}
	rv.module = compiled
	rv.src = src

	instance, err := rv.runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		_ = rv.runtime.Close(context.Background())
		return nil, fmt.Errorf("wasm: NewRootVM: instantiate: %w", err)
	}
	rv.instance = instance

	// Pre-seed the currentCtxID at root for Configure-time hostcall dispatch.
	rv.currentCtxID.Store(rootCtxID)

	return rv, nil
}

// State returns the underlying wazero.Runtime. Escape-hatch for callers
// that need to register additional host modules or query module exports
// beyond the 25.2 surface. Not safe to call after Close.
func (rv *RootVM) State() wazero.Runtime { return rv.runtime }

// RegisterABICallbacks installs the consumer's per-RootVM callback bundle.
// The framework primitive's host-module wiring invokes these on the
// appropriate hostcall dispatch. Safe to call multiple times (last call
// wins); typically called once per RootVM before Configure.
//
// Concurrency note: registration is typically done at compiledConfig.New
// time (single-goroutine); the cb field is then read unlocked from the
// hostcall shims, mirroring 25.1 vm.go's precedent. Mid-flight
// re-registration is not supported.
func (rv *RootVM) RegisterABICallbacks(cb ABICallbacks) {
	rv.cb = cb
}

// RegisteredABICallbacks returns the ABICallbacks previously registered via
// RegisterABICallbacks (nil if none). Consumed by the filter package's
// multi-plugin VM-sharing path (compiled_config.go): when two compiledConfigs
// share one *RootVM via the registry, the second (registry-hit) compiledConfig
// retrieves the SAME multiplexer the first registered, rather than building +
// registering a fresh one that would clobber the shared per-stream routing
// table. Read unlocked, mirroring the RegisterABICallbacks concurrency note.
func (rv *RootVM) RegisteredABICallbacks() ABICallbacks { return rv.cb }

// Configure invokes the canonical proxy-wasm host-side root-context
// lifecycle on the instantiated module:
//
//	(a) _initialize OR _start (mutually exclusive per proxy-wasm v0.2.1).
//	    UNGATED per D-P2.
//	(b) proxy_on_context_create(rootCtxID, 0) — seeds the root context.
//	    Gated by capProxyOnContextCreate. parentContextID == 0 signals
//	    ROOT-context creation per proxy-wasm v0.2.1.
//	(c) proxy_on_vm_start(rootCtxID, vm_config_size). Gated by
//	    capProxyOnVmStart.
//	(d) proxy_on_configure(rootCtxID, plugin_config_size). Gated by
//	    capProxyOnConfigure.
//
// vmConfigBytes + pluginConfigBytes wire-up at Task 14 (compiled_config.go
// EXTEND); at Task 1 the bytes are accepted but the byte-size passed to the
// callbacks is 0 (matching 25.1 vm.Run's "no config" semantic). The bytes
// arguments are kept on the signature to match 25.2 SPEC §3.1's contract.
//
// Lifecycle gate disposition (matches 25.1 vm.Run verbatim): cap-denied
// step is a no-op skip (NOT an error); guest-not-exported step is also a
// no-op skip; only a non-nil Call error from a fired callback surfaces as
// a wrapped error return.
func (rv *RootVM) Configure(ctx context.Context, vmConfigBytes, pluginConfigBytes []byte) error {
	if rv.closed.Load() {
		return errors.New("wasm: Configure on closed RootVM")
	}

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()

	// Retain the config bytes so a FAIL_RELOAD re-instantiation (reinstantiate)
	// can replay proxy_on_vm_start / proxy_on_configure with the same config
	// per phase-25.3 Task 3. Stored under dispatchMu (read by reinstantiate
	// under the same lock).
	rv.vmConfigBytes = vmConfigBytes
	rv.pluginConfigBytes = pluginConfigBytes

	return rv.runRootLifecycle(ctx)
}

// runRootLifecycle invokes steps (a)-(d) of the canonical proxy-wasm root-
// context lifecycle on the CURRENT rv.instance. Shared by Configure (first
// configuration) + reinstantiate (FAIL_RELOAD replay) per phase-25.3 Task 3
// so the gating/no-op-skip discipline stays byte-identical across both paths.
//
//	(a) _initialize OR _start (mutually exclusive). UNGATED.
//	(b) proxy_on_context_create(rootCtxID, 0). Gated by capProxyOnContextCreate.
//	(c) proxy_on_vm_start(rootCtxID, vm_config_size). Gated by capProxyOnVmStart.
//	(d) proxy_on_configure(rootCtxID, plugin_config_size). Gated by capProxyOnConfigure.
//
// MUST be called with dispatchMu held. Sets currentCtxID = rootCtxID for the
// duration. Cap-denied + guest-not-exported steps are no-op skips (NOT
// errors); only a non-nil Call error surfaces as a wrapped error.
//
// At this Task the byte-size passed to (c)/(d) is 0 even when the retained
// bytes are non-nil — the VmConfig/PluginConfig data-source resolution +
// memory-copy lands at Task 14. The bytes are nonetheless retained for the
// replay-with-same-config contract.
func (rv *RootVM) runRootLifecycle(ctx context.Context) error {
	// Restore + set currentCtxID for the duration. The pre-seed at NewRootVM
	// already points at rootCtxID, but we re-set defensively for the case
	// where the caller has issued earlier per-stream callbacks.
	rv.currentCtxID.Store(rv.rootCtxID)

	// (a) _initialize OR _start. Mutually exclusive — per proxy-wasm v0.2.1
	// + upstream wasm.cc:159-178. UNGATED per D-P2.
	if initFn := rv.instance.ExportedFunction("_initialize"); initFn != nil {
		if _, err := initFn.Call(ctx); err != nil {
			return fmt.Errorf("wasm: _initialize: %w", err)
		}
	} else if startFn := rv.instance.ExportedFunction("_start"); startFn != nil {
		if _, err := startFn.Call(ctx); err != nil {
			return fmt.Errorf("wasm: _start: %w", err)
		}
	}
	// No init export is acceptable — minimal test modules may omit both.

	// (b) Seed the root context. Matches upstream proxy-wasm-cpp-host@da3ce05d.
	if rv.sandbox.IsAllowed(capProxyOnContextCreate) {
		if fn := rv.instance.ExportedFunction("proxy_on_context_create"); fn != nil {
			if _, err := fn.Call(ctx, uint64(rv.rootCtxID), 0); err != nil {
				return fmt.Errorf("wasm: proxy_on_context_create(root): %w", err)
			}
		}
	}

	// (c) proxy_on_vm_start(rootCtxID, vm_config_size).
	//
	// At this Task we pass byte-size 0 even when vmConfigBytes is non-nil — the
	// VmConfig data-source resolution + memory-copy lands at Task 14.
	if rv.sandbox.IsAllowed(capProxyOnVmStart) {
		if fn := rv.instance.ExportedFunction("proxy_on_vm_start"); fn != nil {
			if _, err := fn.Call(ctx, uint64(rv.rootCtxID), 0); err != nil {
				return fmt.Errorf("wasm: proxy_on_vm_start: %w", err)
			}
		}
	}

	// (d) proxy_on_configure(rootCtxID, plugin_config_size).
	if rv.sandbox.IsAllowed(capProxyOnConfigure) {
		if fn := rv.instance.ExportedFunction("proxy_on_configure"); fn != nil {
			if _, err := fn.Call(ctx, uint64(rv.rootCtxID), 0); err != nil {
				return fmt.Errorf("wasm: proxy_on_configure: %w", err)
			}
		}
	}

	return nil
}

// reinstantiate is the production FAIL_RELOAD re-instantiation primitive per
// phase-25.3 Task 3 (AMEND-C3 + R-25.3-2/5 + D-25.3-P3). It re-instantiates
// the already-compiled rv.module into a fresh api.Module + replays the root-
// context lifecycle (steps (a)-(d) via runRootLifecycle) using the retained
// vm/plugin config bytes.
//
// MUST be called with dispatchMu held (dispatchReload holds it across the
// whole decide+attempt sequence, so exactly one goroutine ever reaches here
// for a given VM). On a closed RootVM it returns an error without touching
// the cleared runtime/module. The old rv.instance is closed before the new
// one is installed; on a re-instantiation or replay error rv.instance is left
// pointing at whatever was last successfully installed (the caller marks the
// reload failed + the next attempt re-tries past the backoff window).
func (rv *RootVM) reinstantiate(ctx context.Context) error {
	if rv.closed.Load() || rv.runtime == nil || rv.module == nil {
		return errors.New("wasm: reinstantiate on closed RootVM")
	}

	// Close the old instance first so the fresh InstantiateModule starts from
	// clean module state (the wazero runtime keeps the compiled module; only
	// the live instance is replaced).
	if rv.instance != nil {
		_ = rv.instance.Close(ctx)
		rv.instance = nil
	}

	instance, err := rv.runtime.InstantiateModule(ctx, rv.module, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return fmt.Errorf("wasm: reinstantiate: %w", err)
	}
	rv.instance = instance

	// Replay the lifecycle on the fresh instance with the retained config
	// bytes. runRootLifecycle sets currentCtxID = rootCtxID as its first step.
	if err := rv.runRootLifecycle(ctx); err != nil {
		return fmt.Errorf("wasm: reinstantiate replay: %w", err)
	}
	return nil
}

// dispatchReload is the dispatch-entry hook for the FAIL_RELOAD state machine
// per phase-25.3 Task 3 + D-25.3-P3. It acquires dispatchMu (NOT reentrant —
// callers MUST NOT already hold it) so the whole decide+attempt sequence is
// serialized: exactly one goroutine ever reloads a given VM; concurrent
// goroutines block, then observe the post-reload Running state + get Serve.
//
//   - reloadDecisionAttempt: invoke reattempt(ctx). On nil err →
//     markReloaded() + VmReloadSuccessInc; on error → markReloadFailed() +
//     VmReloadRuntimeFailureInc.
//   - reloadDecisionBackoff: VmReloadBackoffInc (no reattempt).
//   - reloadDecisionServe: no-op.
//
// Returns the decision so the caller (Task 9 filter dispatch) can branch the
// stream disposition. The injectable reattempt param lets tests count
// attempts; production callers pass rv.reinstantiate.
//
// Lock order: dispatchMu → reload.mu (reload.decide / mark* acquire their own
// mu under the held dispatchMu — safe + non-reentrant).
func (rv *RootVM) dispatchReload(ctx context.Context, reattempt func(context.Context) error) reloadDecision {
	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()

	decision := rv.reload.decide()
	switch decision {
	case reloadDecisionAttempt:
		if err := reattempt(ctx); err != nil {
			rv.reload.markReloadFailed()
			if rv.reloadStats != nil {
				rv.reloadStats.VmReloadRuntimeFailureInc()
			}
		} else {
			rv.reload.markReloaded()
			if rv.reloadStats != nil {
				rv.reloadStats.VmReloadSuccessInc()
			}
		}
	case reloadDecisionBackoff:
		if rv.reloadStats != nil {
			rv.reloadStats.VmReloadBackoffInc()
		}
	case reloadDecisionServe:
		// no-op — VM is Running.
	}
	return decision
}

// NewStreamContext allocates a fresh per-stream context. Monotonic
// streamCtxID via nextStreamCtxID counter (starting at rootCtxID+1 to keep
// the namespaces disjoint per upstream convention); invokes
// proxy_on_context_create(streamCtxID, rootCtxID) on the shared Module
// instance (gated by capProxyOnContextCreate); records the new
// StreamContext in rv.streamCtxs; returns the per-stream handle.
//
// Per-stream-context construction cost is microseconds — just bookkeeping +
// the proxy_on_context_create invocation. NO wazero Runtime construction
// (the runtime is the per-RootVM shared one).
func (rv *RootVM) NewStreamContext(ctx context.Context) (*StreamContext, error) {
	if rv.closed.Load() {
		return nil, errors.New("wasm: NewStreamContext on closed RootVM")
	}

	// Allocate a fresh monotonic streamCtxID. The counter is per-RootVM; the
	// first allocation returns 1 (we keep rootCtxID + streamCtxID namespaces
	// disjoint by convention — the caller's rootCtxID is typically the
	// per-plugin counter at compiledConfig scope while the streamCtxID is
	// per-RootVM here).
	id := rv.nextStreamCtxID.Add(1)

	sc := &StreamContext{
		rootVM:        rv,
		streamCtxID:   id,
		rootContextID: rv.rootCtxID,
	}

	// Record the child BEFORE firing proxy_on_context_create so the
	// hostcall dispatch (which reads rv.currentCtxID = id) sees a registered
	// child if it consults streamCtxs.
	rv.streamCtxsMu.Lock()
	rv.streamCtxs[id] = sc
	rv.streamCtxsMu.Unlock()

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()

	rv.currentCtxID.Store(id)

	if rv.sandbox.IsAllowed(capProxyOnContextCreate) {
		if fn := rv.instance.ExportedFunction("proxy_on_context_create"); fn != nil {
			if err := rv.runCallWithPanicWrapper(func() error {
				_, callErr := fn.Call(ctx, uint64(id), uint64(rv.rootCtxID))
				return callErr
			}); err != nil {
				// Rollback the streamCtxs entry on dispatch failure.
				rv.streamCtxsMu.Lock()
				delete(rv.streamCtxs, id)
				rv.streamCtxsMu.Unlock()
				return nil, fmt.Errorf("wasm: NewStreamContext: proxy_on_context_create(stream=%d, root=%d): %w",
					id, rv.rootCtxID, err)
			}
		}
	}

	return sc, nil
}

// HasGlobalFunc returns true if the named guest export is a callable
// function on the instantiated module. Delegated to the per-RootVM
// instance.
//
// At 25.2 (Task 3): if `name` is one of the 7 NEW lifecycle-callback names
// (proxy_on_request_body / proxy_on_response_body / proxy_on_request_trailers
// / proxy_on_response_trailers / proxy_on_tick / proxy_on_http_call_response
// / proxy_on_foreign_function) AND the corresponding capability key is
// DENIED via `rv.sandbox.IsAllowed`, this method returns false EVEN IF the
// guest exports the function. Mirrors upstream cpp-host `wasm.cc:238-247`
// `_GET_PROXY` macro discipline (denied capability → function-pointer left
// null + the host treats the missing function as if the guest hadn't
// exported it). The 25.1 13 callback names + non-callback exports remain
// gated at their per-callback caller (or are looked up bare) — this method
// only short-circuits the 7 NEW lifecycle callbacks per AMEND-B5.
func (rv *RootVM) HasGlobalFunc(name string) bool {
	if rv.instance == nil {
		return false
	}
	if cap, isNew25_2Callback := newCallbackCapability25_2[name]; isNew25_2Callback {
		if !rv.sandbox.IsAllowed(cap) {
			return false
		}
	}
	return rv.instance.ExportedFunction(name) != nil
}

// newCallbackCapability25_2 maps the 7 NEW 25.2 lifecycle-callback guest-
// export names to their corresponding capability key per §5.3 + AMEND-B5.
// The map is consulted by HasGlobalFunc above to implement the
// gate-at-getFunction discipline mirroring cpp-host `wasm.cc:238-247`
// `_GET_PROXY` macro. The 13 25.1 callback names are NOT in this map —
// their gate stays at the per-callback caller (CallProxyOnX) per 25.1
// SPEC §3.3 + the no-break invariant.
var newCallbackCapability25_2 = map[string]string{
	"proxy_on_request_body":       capProxyOnRequestBody,
	"proxy_on_response_body":      capProxyOnResponseBody,
	"proxy_on_request_trailers":   capProxyOnRequestTrailers,
	"proxy_on_response_trailers":  capProxyOnResponseTrailers,
	"proxy_on_tick":               capProxyOnTick,
	"proxy_on_http_call_response": capProxyOnHttpCallResponse,
	"proxy_on_foreign_function":   capProxyOnForeignFunction,
}

// IsAllowed forwards to rv.sandbox.IsAllowed; satisfies the wasiHost
// interface (wasi.go) so the RootVM can pass itself to the WASI shim
// wrappers at registration time.
func (rv *RootVM) IsAllowed(capabilityName string) bool {
	return rv.sandbox.IsAllowed(capabilityName)
}

// WASIEnviron returns the guest-visible environment as KEY=VALUE\0 entries
// (sorted-key order for determinism) per AMEND-C4. Satisfies wasiHost.
// Delegates to encodeWASIEnviron(rv.env) — nil/empty env ⇒ nil (zero
// entries advertised; the 25.1/25.2 behavior).
func (rv *RootVM) WASIEnviron() [][]byte { return encodeWASIEnviron(rv.env) }

// LogProxy routes a log line to rv.logSink (if set); satisfies the wasiHost
// interface (wasi.go). Default nil sink ⇒ drop, matching the
// envoy-go-strict "no stdout leak" discipline.
func (rv *RootVM) LogProxy(level abi.LogLevel, msg string) {
	if rv.logSink == nil {
		return
	}
	// Sink write errors are intentionally discarded — the log sink is a
	// best-effort observability channel; failing to write a log line MUST
	// NOT propagate as an error to the wasm guest or the filter chain.
	_, _ = fmt.Fprintf(rv.logSink, "[wasm %s] %s\n", logLevelString(level), msg)
}

// Close releases the wazero.Runtime + (Task 5) stops the tick goroutine +
// (Task 8) cancels outstanding httpCalls + (Task 12) closes the
// dynamic-stats Registry. Idempotent — second Close returns nil with no
// side effects.
//
// Close first marks every still-live child StreamContext as closed (via
// sc.closed.Store(true)) BEFORE clearing rv.instance + rv.streamCtxs, so
// any caller still holding a *StreamContext reference observes a graceful
// early-return on the next CallProxyOnX rather than a nil-pointer panic
// dereferencing the cleared instance. We do NOT call sc.Close per child:
// that would re-enter dispatchMu via CallProxyOnDone/Log/Delete after
// rv.instance is already cleared. Marking closed is sufficient + race-free
// because rv.streamCtxs is the registry of LIVE children only (Close
// removes the entry).
//
// At Task 5 the tick goroutine integration is LIVE — Close stops the tick
// goroutine (close(rv.tickStop) + tickWG.Wait()) BEFORE clearing rv.instance
// so the tick goroutine's lockAndDispatchTick observes a live module while
// it short-circuits via the closed flag. At Task 8 + Task 12 the httpCalls
// + dynamic-stats activations land their corresponding cleanup hooks.
func (rv *RootVM) Close() error {
	var closeErr error
	rv.closeOnce.Do(func() {
		rv.closed.Store(true)

		// Stop the tick goroutine (if any) BEFORE clearing rv.instance per
		// Task 5 + Q5 + R-25.2-9. stopTickGoroutine: (a) acquires tickMu;
		// (b) close(rv.tickStop) — signals the goroutine to exit; (c)
		// rv.tickWG.Wait() — joins the goroutine before returning. The
		// tick goroutine's lockAndDispatchTick has a closed-check guard
		// + an rv.instance == nil guard so races between Close-flag-flip
		// and tick-fire are handled gracefully (short-circuit before
		// touching the cleared runtime).
		rv.stopTickGoroutine()

		// Snapshot the live children under streamCtxsMu, then flip each
		// sc.closed OUTSIDE the map lock — the atomic.Bool Store is
		// race-free + we avoid holding streamCtxsMu while touching N
		// independent StreamContext structs. Snapshot-then-flip mirrors
		// the standard concurrent-map cleanup pattern.
		rv.streamCtxsMu.Lock()
		children := make([]*StreamContext, 0, len(rv.streamCtxs))
		for _, sc := range rv.streamCtxs {
			children = append(children, sc)
		}
		rv.streamCtxs = nil
		rv.streamCtxsMu.Unlock()
		for _, sc := range children {
			sc.closed.Store(true)
		}

		// Cancel any in-flight httpCalls (Task 8 per AMEND-B3). Snapshot under
		// httpCallsMu + cancel OUTSIDE the lock to keep lock-held duration
		// bounded. The dispatch goroutines observe the canceled ctx + their
		// handleHttpCallResponse path early-returns via the token-miss guard
		// (the entry is already removed from the map).
		rv.httpCallsMu.Lock()
		var cancels []context.CancelFunc
		for _, e := range rv.httpCalls {
			if e.cancel != nil {
				cancels = append(cancels, e.cancel)
			}
		}
		rv.httpCalls = nil
		rv.httpCallsMu.Unlock()
		for _, c := range cancels {
			c()
		}

		// Acquire dispatchMu before clearing runtime/instance/module so we
		// serialize with any in-flight per-stream callback dispatch +
		// response goroutine (handleHttpCallResponse at http_call.go re-
		// acquires dispatchMu before touching rv.instance). This races
		// would otherwise produce a write-vs-read on rv.instance under
		// `-race` per the standard Go memory model — the closeOnce.Do
		// flips closed.Load() FIRST so subsequent dispatchMu acquirers
		// observe the closed flag + early-return; this acquisition drains
		// any frame already inside dispatchMu before we wipe the fields.
		rv.dispatchMu.Lock()
		rt := rv.runtime
		rv.runtime = nil
		rv.instance = nil
		rv.module = nil
		rv.dispatchMu.Unlock()

		if rt != nil {
			closeErr = rt.Close(context.Background())
		}
	})
	return closeErr
}

// CallForeignFunction dispatches a registered foreign function synchronously
// per 25.2 SPEC §3.1 + §5.1 #38 + AMEND-A9 + R-25.2-8 + D-P-PLAN-9
// (mutex-per-RootVM concurrency model).
//
// Returns:
//
//   - (result, WasmResultOk) on a successful invocation. `result` is the
//     bytes returned by the ForeignFunctionFn (MAY be nil for empty-result
//     returns).
//   - (nil, WasmResultNotFound) when no foreign function is registered under
//     `name`. Byte-faithful to upstream cpp-host `src/exports.cc:147-184`.
//     The envoy-go-strict `foreign_function_denied` counter increments on
//     this path (wiring deferred Task 17).
//   - (nil, WasmResultInternalFailure) when the ForeignFunctionFn Go-panics.
//     The recovered value flows through PanicHandlerFn (if configured); the
//     envoy_go.failures counter increments (wiring deferred Task 17). The
//     *RootVM survives + can dispatch a subsequent call.
//   - (result, status) for any other status the ForeignFunctionFn explicitly
//     returns (e.g., BadArgument for malformed args; CasMismatch if the
//     function wraps a CAS-based primitive).
//
// # Concurrency contract per D-P-PLAN-9 (LOAD-BEARING — DO NOT VIOLATE)
//
// CallForeignFunction MUST be called from inside a dispatchMu-held caller
// frame (e.g., a hostcall body invoked during proxy_on_*). The hostcall body
// at registration.go acquires dispatchMu via the per-StreamContext.CallProxyOnX
// path; the foreign-function dispatch is a nested call inside that frame and
// inherits the held lock. CallForeignFunction MUST NOT re-acquire dispatchMu
// — sync.Mutex in Go is non-reentrant and would deadlock.
//
// This contract is the mutex-per-RootVM serialization that closes D-25.2-P3:
// upstream cpp-host serializes foreign-function dispatch via the Wasm-level
// lock; envoy-go mirrors via dispatchMu so guest behavior is byte-faithful.
// ALTERNATIVE REJECTED at PLAN: caller-goroutine dispatch (fires WITHOUT
// crossing the *RootVM lock) — breaks upstream byte-faithful semantic.
// ALTERNATIVE REJECTED at PLAN: event-loop-per-RootVM — YAGNI.
//
// # Panic-recovery wrapper
//
// The ForeignFunctionFn invocation is wrapped in a defer/recover frame
// (inlined here rather than using runWithPanicWrapper because the WasmResult
// + result-bytes signature does not fit the existing
// `func() abi.WasmResult` wrapper shape). A Go panic from the function:
//
//	(1) is caught by recover();
//	(2) invokes rv.panicH (the configured PanicHandlerFn) if non-nil;
//	(3) clears `result` to nil;
//	(4) sets `status` to WasmResultInternalFailure.
//
// The *RootVM is NOT poisoned — a subsequent CallForeignFunction (or any
// other host-side dispatch) proceeds normally.
func (rv *RootVM) CallForeignFunction(ctx context.Context, name string, args []byte) (result []byte, status abi.WasmResult) {
	fn, ok := rv.foreignReg.Get(name)
	if !ok {
		// Increment the `foreign_function_denied` envoy-go-strict counter per
		// AMEND-A9 + R-25.2-8 (wire name `wasm.<plugin>.foreign_function_
		// denied`). Wired via the RootStatsRecorder interface per 25.2 IMPL
		// Task 20 follow-up (Concern 2 — counter-glue wiring gap).
		rv.stats.ForeignFunctionDeniedInc()
		return nil, abi.WasmResultNotFound
	}

	// Panic-recovery wrapper per D-P-PLAN-9 (d). Inlined rather than using
	// rv.runWithPanicWrapper because the WasmResult+result-bytes signature
	// doesn't fit that helper's `func() abi.WasmResult` shape; the recovery
	// discipline is byte-identical (recover + PanicHandlerFn + return
	// InternalFailure).
	defer func() {
		if r := recover(); r != nil {
			if rv.panicH != nil {
				rv.panicH(r)
			}
			// envoy_go.failures co-increment per §2.25 (host-side panic-
			// recovery is a stream-failure event; surfaces on the generic
			// envoy-go-strict failures counter). Wired via the
			// RootStatsRecorder interface per 25.2 IMPL Task 20 follow-up
			// (Concern 2).
			rv.stats.EnvoyGoFailuresInc()
			result = nil
			status = abi.WasmResultInternalFailure
		}
	}()

	return fn(ctx, args)
}

// --- panic-wrapper helpers (migrated from 25.1 vm.go) --------------------

// runWithPanicWrapper runs fn under a panic-wrapper that converts a
// recovered panic into the named WasmResult sentinel + invokes the
// configured PanicHandlerFn (if any). Used by the hostcall bodies in
// registration.go to convert Go-side panics in ABICallbacks methods into
// `abi.WasmResultInternalFailure` (=10) returns.
//
// Per AMEND-A8: the panic-wrapper is for genuine Go panics from bridge
// callbacks; wazero-side traps return via the Call error path and are
// NOT recovered here (they propagate to the per-callback caller).
func (rv *RootVM) runWithPanicWrapper(fn func() abi.WasmResult) (result abi.WasmResult) {
	defer func() {
		if r := recover(); r != nil {
			if rv.panicH != nil {
				rv.panicH(r)
			}
			result = abi.WasmResultInternalFailure
		}
	}()
	return fn()
}

// runCallWithPanicWrapper is the error-returning variant, used by the
// per-callback CallProxyOnX methods on *StreamContext. A recovered Go panic
// converts to a wrapped error so the per-callback caller observes a
// non-nil err.
func (rv *RootVM) runCallWithPanicWrapper(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if rv.panicH != nil {
				rv.panicH(r)
			}
			err = fmt.Errorf("wasm: recovered panic in guest call: %v", r)
		}
	}()
	return fn()
}

// logLevelString returns the upstream proxy-wasm level-name string for the
// LogProxy + proxy_log integration sink formatting. Migrated from 25.1 vm.go.
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
