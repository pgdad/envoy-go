// Module + CompileCache + ABI-version gating per AMEND-A6 envoy-go-strict-stricter.
//
// The compile.go surface exposes three production types and one error sentinel:
//
//   - `Module` — wraps a `wazero.CompiledModule` + the detected
//     `AbiVersion` + the sha256(src) cache key. Safe for cross-VM reuse
//     (the compiled module is immutable; per-VM instantiation happens at
//     Task 7 vm.go).
//   - `CompileCache` — content-addressed compile cache keyed by sha256(src),
//     owning a shared compile-only `wazero.Runtime`. `sync.RWMutex` discipline
//     scales to N concurrent compile calls per stream (anticipated 25.3
//     multi-plugin VM-sharing). Cache scope is `compiledConfig`-instance per
//     D-P-PLAN-5 (one cache per listener filter-chain mounting a wasm filter);
//     GC-driven eviction (cache lifetime equals compiledConfig lifetime).
//   - `CompileModule` — the single content-addressed compile entry point.
//     Computes the hash, checks the cache (if non-nil), gates by ABI-version
//     (`AbiVersion_0_2_1` only per AMEND-A6), compiles via wazero, caches
//     the result, and returns. `cache == nil` is tolerated per ADR-0085 +
//     AMEND-A9 (nil-tolerance discipline): the function compiles uncached
//     using a transient single-use runtime — the caller is then responsible
//     for the wazero.CompiledModule.Close() lifecycle since no cache owns
//     the runtime.
//   - `ErrUnsupportedAbiVersion` — sentinel returned when the detected ABI
//     version is anything other than `AbiVersion_0_2_1` (AMEND-A6 envoy-go-strict-stricter).
//     The Task 9 `compiled_config.go` PARSE-REJECT arm 16 wraps this with
//     the byte-stable "wasm: config.vm_config.code: unsupported ABI version;
//     only proxy_abi_version_0_2_1 supported (envoy-go-strict)" wording per
//     D-P5; at Task 5 we just return the sentinel with the detected version
//     in the message for caller debugging.
//
// Runtime mode: wazero defaults to compiler-mode on amd64/arm64 + falls back
// to interpreter on other platforms (per `wazero.NewRuntime(ctx)`). The
// per-stream VM runtimes (Task 7 vm.go) can pick a different policy if
// benchmarks demand; the compile-only runtime here uses the default.

package wasm

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
)

// ErrUnsupportedAbiVersion is returned by CompileModule when the source
// module's proxy-wasm ABI version sentinel is anything other than
// `proxy_abi_version_0_2_1` per AMEND-A6 envoy-go-strict-stricter discipline.
// Returned errors WRAP this sentinel via fmt.Errorf("...: %w", ...) so
// callers (Task 9 compiled_config.go's 18-arm PARSE-REJECT roster) can
// compose via errors.Is.
var ErrUnsupportedAbiVersion = errors.New("wasm: unsupported proxy-wasm ABI version")

// Module wraps a compiled `wazero.CompiledModule` + the detected
// proxy-wasm ABI version + the sha256 cache key + the original wasm src
// bytes (immutable; retained for cross-runtime re-compile per the Task 7
// follow-up — see Source doc-comment). Returned by CompileModule + held
// by the *CompileCache. Cross-VM-reusable (the compiled module is
// immutable; per-VM instantiation happens at Task 7 vm.go).
//
// All fields are unexported; accessors `ABIVersion()` / `Compiled()` /
// `Hash()` / `Source()` expose the read-only surface.
//
// `transientRT` is non-nil ONLY on the nil-cache path of CompileModule:
// in that case the *Module owns a single-use wazero.Runtime that must be
// released via `Module.Close(ctx)`. Cache-owned modules have
// `transientRT == nil` — their runtime is owned (and released) by the
// CompileCache; calling Module.Close on a cache-owned module is a
// documented no-op (see Module.Close doc).
type Module struct {
	mu          sync.Mutex
	compiled    wazero.CompiledModule
	abiVer      AbiVersion
	hash        [32]byte
	src         []byte // retained for cross-runtime re-compile via shared wazero.CompilationCache (Task 7 follow-up)
	transientRT wazero.Runtime
	closed      bool
}

// ABIVersion returns the detected proxy-wasm ABI version of the module.
// At 25.1 this is always `AbiVersion_0_2_1` (the other variants
// PARSE-REJECT at CompileModule per AMEND-A6). Exposed for use by
// registration logic + log/diag.
func (m *Module) ABIVersion() AbiVersion { return m.abiVer }

// Compiled returns the underlying `wazero.CompiledModule`. ESCAPE-HATCH for
// callers (Task 7 vm.go) that instantiate the module against a per-stream
// wazero runtime. Not safe to use after the owning CompileCache.Close()
// (cache-owned modules) or after `Module.Close(ctx)` (nil-cache transient
// modules — Module.Close releases BOTH the CompiledModule AND its
// transient Runtime per the wazero v1.10.1 contract; see Module.Close
// doc for the rationale).
func (m *Module) Compiled() wazero.CompiledModule { return m.compiled }

// Hash returns the sha256(src) cache key. Useful for debug + log assertions +
// for the cross-VM reuse decision logic at Task 7 vm.go.
func (m *Module) Hash() [32]byte { return m.hash }

// Source returns the original wasm source bytes the module was compiled
// from. Returned bytes are NOT defensively copied — the caller MUST treat
// them as read-only (they're shared with the CompileCache's internal
// retention + with any prior caller that received this *Module from a
// cache hit). Mutating the slice corrupts the cache.
//
// Use case: cross-runtime re-compile. wazero v1.10.1's `CompiledModule`
// is bound to the engine of the runtime that compiled it; a per-stream
// VM's `wazero.Runtime` (Task 7 vm.go) cannot directly instantiate a
// CompiledModule produced by the `CompileCache`'s compile-only runtime.
// `vm.Run` re-compiles the src against `vm.runtime` (sub-ms cache hit
// when a shared `wazero.CompilationCache` is wired via
// `WithCompilationCache(cache.WazeroCompilationCache())` at NewVM).
func (m *Module) Source() []byte { return m.src }

// Close releases the resources owned exclusively by this *Module.
//
// Behavior depends on whether this Module is cache-owned or a nil-cache
// transient (set at CompileModule time):
//
//   - Cache-owned (`transientRT == nil`): Close is a NO-OP. The owning
//     CompileCache is responsible for releasing both the shared
//     wazero.Runtime AND (via the runtime → CompiledModule cascade per
//     wazero v1.10.1 config.go:317) every CompiledModule it compiled.
//     Calling Module.Close on a cache-owned module is safe and idempotent.
//   - Nil-cache transient (`transientRT != nil`): Close releases the
//     wazero.CompiledModule AND the transient wazero.Runtime constructed
//     by the nil-cache path of CompileModule. This is REQUIRED to avoid
//     leaking the runtime, since per wazero v1.10.1's contract
//     `CompiledModule.Close` does NOT cascade to its parent Runtime
//     (only Runtime.Close → cascades to compiled modules).
//
// Idempotent: a second Close returns nil with no side effects (guarded by
// an internal mutex + closed flag).
func (m *Module) Close(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	rt := m.transientRT
	compiled := m.compiled
	m.transientRT = nil
	m.mu.Unlock()

	// Cache-owned module: no-op. The cache owns the runtime + the runtime
	// owns the CompiledModule per wazero's one-way Runtime → CompiledModule
	// cascade — closing the compiled module here would be safe but redundant,
	// and could surface as a confusing double-close if the cache's Close
	// already ran. We just record the closed flag and return.
	if rt == nil {
		return nil
	}

	// Transient nil-cache module: release the CompiledModule first, then
	// the Runtime. Returns the first non-nil error encountered (the
	// subsequent Close still runs to avoid leaking the second resource).
	cErr := compiled.Close(ctx)
	rErr := rt.Close(ctx)
	if cErr != nil {
		return cErr
	}
	return rErr
}

// CompileCache is a content-addressed compile cache, keyed by sha256(src).
// Owned by the *compiledConfig at Task 9 (filter-config-instance scope;
// GC-driven eviction; no manual evict; cache lifetime == compiledConfig
// lifetime per D-P-PLAN-5). Safe for concurrent read/add via internal
// sync.RWMutex.
//
// Holds:
//   - A shared compile-only `wazero.Runtime` (separate from the per-stream
//     runtimes constructed at Task 7 vm.go). Calling Close releases this
//     runtime, which invalidates every *Module previously returned from this
//     cache (the CompiledModule is bound to its runtime).
//   - A shared `wazero.CompilationCache` exposed via WazeroCompilationCache.
//     This is the wazero-codegen-result cache (distinct from this Go-level
//     *Module cache); when shared with per-stream VMs via the Task 7
//     `WithCompilationCache(cache.WazeroCompilationCache())` option, per-stream
//     re-compiles (vm.Run → vm.runtime.CompileModule(module.Source())) hit
//     this cache sub-ms, amortizing the codegen cost across the per-stream
//     VM fleet that re-compiles the same src against its own runtime.
//
// Two-cache model rationale: wazero v1.10.1's `CompiledModule` is bound to
// the engine of the runtime that compiled it (see cache.go:32-34 doc). To
// share compile WORK across the CompileCache's compile-only runtime + every
// per-stream VM's runtime, both must share a `wazero.CompilationCache`. This
// is the Task 7 follow-up fix for the cross-runtime CompiledModule binding
// issue surfaced by the Task 7 implementer.
type CompileCache struct {
	mu       sync.RWMutex
	store    map[[32]byte]*Module
	rt       wazero.Runtime
	wazeroCC wazero.CompilationCache // shared codegen cache (Task 7 follow-up)
	closed   bool
}

// NewCompileCache constructs an empty cache + initializes:
//
//   - A shared `wazero.CompilationCache` (`wazero.NewCompilationCache()`).
//     Returned via WazeroCompilationCache for use by per-stream VMs (Task 7
//     `WithCompilationCache` option).
//   - The compile-only `wazero.Runtime` wired with the shared CompilationCache
//     via `wazero.NewRuntimeConfig().WithCompilationCache(wazeroCC)`. Runtime
//     mode uses wazero's default (compiler-mode on amd64/arm64; interpreter on
//     platforms without compiler support). Per parent §1.2 hypothesis + Task
//     17 benchmark expectations, compiler-mode init is comparable to
//     interpreter for the headers-bridge workload.
func NewCompileCache(ctx context.Context) *CompileCache {
	wazeroCC := wazero.NewCompilationCache()
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCompilationCache(wazeroCC))
	return &CompileCache{
		store:    make(map[[32]byte]*Module),
		rt:       rt,
		wazeroCC: wazeroCC,
	}
}

// WazeroCompilationCache returns the shared `wazero.CompilationCache` owned
// by this *CompileCache. Pass it to per-stream VMs via the Task 7
// `WithCompilationCache(cc)` option so the per-stream `vm.runtime` shares
// the codegen cache with the cache's compile-only runtime. The result is
// that `vm.Run`'s internal re-compile-against-vm.runtime (via
// `module.Source()`) hits the shared cache sub-ms instead of re-doing the
// codegen work.
//
// Lifetime: the returned cache is released by *CompileCache.Close — callers
// MUST NOT retain it past the *CompileCache's lifetime.
func (cc *CompileCache) WazeroCompilationCache() wazero.CompilationCache {
	return cc.wazeroCC
}

// CompileModule compiles src via wazero's parser + the byte-faithful
// GetAbiVersion detection (per AMEND-A6). If cache is non-nil, caches
// by sha256(src) (content-addressed; same bytes → same Module pointer
// across cache-hit calls). If cache is nil, compiles uncached via a
// transient single-use runtime (ADR-0085 + AMEND-A9 nil-tolerance
// discipline) — the returned *Module owns BOTH the CompiledModule AND
// the underlying wazero.Runtime in this case, and the caller MUST
// release both via `Module.Close(ctx)`. Closing only the CompiledModule
// (via `Module.Compiled().Close(ctx)`) is INSUFFICIENT because per
// wazero v1.10.1's contract (config.go:317) only `Runtime.Close`
// cascades to its CompiledModules; the reverse is NOT true.
//
// Returns:
//   - wrapped wazero compile error on parse/compile failure
//   - ErrUnsupportedAbiVersion (via %w) when the module's ABI sentinel
//     is not `proxy_abi_version_0_2_1` per AMEND-A6
//   - nil error + non-nil *Module on success (always AbiVersion_0_2_1)
//
// The cache-vs-ABI-gate ordering: we check the cache FIRST (by hash) so
// that already-validated modules short-circuit before paying the
// GetAbiVersion + wazero.CompileModule cost on every call.
func CompileModule(ctx context.Context, src []byte, cache *CompileCache) (*Module, error) {
	hash := sha256.Sum256(src)

	// Fast path: cache hit.
	if cache != nil {
		cache.mu.RLock()
		if cache.closed {
			cache.mu.RUnlock()
			return nil, fmt.Errorf("wasm: CompileModule on closed CompileCache")
		}
		if existing, ok := cache.store[hash]; ok {
			cache.mu.RUnlock()
			return existing, nil
		}
		cache.mu.RUnlock()
	}

	// ABI-version gate per AMEND-A6 — run BEFORE the wazero compile so we
	// pay the parser cost only once when the ABI is wrong (GetAbiVersion
	// is cheap; wazero.CompileModule is not).
	ver, err := GetAbiVersion(src)
	if err != nil {
		return nil, fmt.Errorf("wasm: ABI-version detection: %w", err)
	}
	if ver != AbiVersion_0_2_1 {
		return nil, fmt.Errorf("wasm: detected ABI %v: %w", ver, ErrUnsupportedAbiVersion)
	}

	// Resolve the wazero runtime to compile against. For the cache path,
	// use the cache's shared compile-only runtime; for the nil-cache path,
	// construct a transient single-use runtime owned by the returned
	// *Module (released via Module.Close per the ADR-0085 nil-tolerance
	// contract — see the Module.Close doc for the wazero v1.10.1 contract
	// rationale).
	var (
		rt          wazero.Runtime
		transientRT wazero.Runtime // captured on the *Module only when nil-cache
	)
	if cache != nil {
		cache.mu.RLock()
		if cache.closed {
			cache.mu.RUnlock()
			return nil, fmt.Errorf("wasm: CompileModule on closed CompileCache")
		}
		rt = cache.rt
		cache.mu.RUnlock()
	} else {
		rt = wazero.NewRuntime(ctx)
		transientRT = rt
	}

	compiled, err := rt.CompileModule(ctx, src)
	if err != nil {
		// On the nil-cache path, the transient runtime must be released
		// even on compile failure — otherwise we'd leak it. On the cache
		// path the cache owns the runtime, so we don't touch it here.
		if transientRT != nil {
			_ = transientRT.Close(context.Background())
		}
		return nil, fmt.Errorf("wasm: wazero compile: %w", err)
	}

	mod := &Module{
		compiled:    compiled,
		abiVer:      AbiVersion_0_2_1,
		hash:        hash,
		src:         src,         // retained for cross-runtime re-compile via shared CompilationCache
		transientRT: transientRT, // nil for cache-owned; non-nil for nil-cache transient
	}

	// Slow path: cache miss → add. We re-check under the write-lock to
	// resolve the rare race where two goroutines both see "miss" under
	// RLock and both compile. The first to acquire the write-lock wins;
	// the second discards its freshly-compiled module + returns the
	// already-cached one. This wastes one compile in the race, but keeps
	// the *Module pointer-identity invariant (cache-hit always returns
	// the SAME pointer for the same content).
	if cache != nil {
		cache.mu.Lock()
		if cache.closed {
			cache.mu.Unlock()
			// Release the wazero.CompiledModule we just compiled — the
			// owning runtime is gone, so the compiled module is invalid.
			_ = compiled.Close(ctx)
			return nil, fmt.Errorf("wasm: CompileModule on closed CompileCache")
		}
		if existing, ok := cache.store[hash]; ok {
			cache.mu.Unlock()
			_ = compiled.Close(ctx) // discard the racer's compile
			return existing, nil
		}
		cache.store[hash] = mod
		cache.mu.Unlock()
	}
	return mod, nil
}

// Close releases the compile-only wazero.Runtime AND the shared
// wazero.CompilationCache owned by the cache. Idempotent: the second
// Close returns nil with no side effects. After Close, any *Module
// returned from this cache's CompileModule should be considered invalid
// (the CompiledModule is bound to the now-closed runtime); any per-stream
// VM still configured with the shared CompilationCache will no longer
// share compile-work with this cache (the cache's engines are torn down).
//
// Future CompileModule calls against the closed cache return an error
// rather than silently constructing a fresh runtime — the API contract is
// that Close ends the cache's life.
//
// Errors from runtime.Close + wazeroCC.Close are joined via errors.Join;
// both releases run unconditionally to avoid leaking the second resource
// if the first errors.
func (cc *CompileCache) Close() error {
	cc.mu.Lock()
	if cc.closed {
		cc.mu.Unlock()
		return nil
	}
	cc.closed = true
	rt := cc.rt
	wcc := cc.wazeroCC
	cc.rt = nil
	cc.wazeroCC = nil
	cc.store = nil
	cc.mu.Unlock()

	// Use context.Background here — Close is called from cleanup paths
	// where the original ctx may already be done; we must still release
	// the runtime + cache resources.
	var errs []error
	if rt != nil {
		if err := rt.Close(context.Background()); err != nil {
			errs = append(errs, err)
		}
	}
	if wcc != nil {
		if err := wcc.Close(context.Background()); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
