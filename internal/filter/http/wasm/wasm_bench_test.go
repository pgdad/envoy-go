// wasm_bench_test.go — Task 17 R8 benchmark gate per the 25.1 SPEC §13-R8
// + parent §13-R8 + D-P-PLAN-10 + D-P4.
//
// BenchmarkPerStreamVM_Construction_Headers measures the per-stream
// wazero-runtime construction + module instantiation + lifecycle dispatch
// cost the production DecodeHeaders incurs at the 25.1 headers-only
// bridge surface. The reported ns/op gates ADR-0205 firing at this
// Task 17 atomic landing per the R8 signaling protocol:
//
//   - ns/op <= 1_000_000 (= 1ms) → WEAK-default per-stream VM construction
//     STANDS; ADR-0205 NOT consumed; the WEAK-HOLD 1-slot escape-valve
//     buffer is RELEASED + carries forward to 25.2 BRAINSTORM as the
//     family-level escape valve.
//   - ns/op  > 1_000_000 (= 1ms) → escape-valve FIRES; this Task 17 lands
//     the per-module wazero Runtime pool with pre-instantiated entries
//     per ADR-0205 §Decision.
//
// Run via:
//
//	go test -count=1 -benchmem \
//	        -bench=BenchmarkPerStreamVM_Construction_Headers \
//	        -run=^$ ./internal/filter/http/wasm/
//
// The reported ns/op is quoted verbatim in PROGRESS.md Task 17 + REVIEW.md
// per the D-P4 + D-P-PLAN-10 closure protocol. Anticipated under threshold
// per parent §1.2 + phase-22.1 70µs analogous-benchmark precedent.

package wasm

import (
	"context"
	"testing"

	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
)

// BenchmarkPerStreamVM_Construction_Headers measures the per-stream
// wazero.Runtime construction + CompiledModule re-compile (sub-ms cache
// hit via shared wazero.CompilationCache) + module instantiation +
// _initialize + proxy_on_context_create(root,0) + proxy_on_vm_start +
// proxy_on_configure cost the production filter dispatch incurs per
// per-stream per AMEND-A4 per-stream-VM construction model. Mirrors
// phase-22.1's BenchmarkPerStreamLState_Construction_Headers shape.
//
// Build the *Module once outside the timed loop (the
// content-addressed compile cache means subsequent per-stream
// CompileModule calls return the SAME *Module pointer — a hot cache
// hit). The benchmark exercises:
//
//	(a) NewVM(ctx, WithCompilationCache(...)) — wazero.Runtime
//	    construction + 47-hostcall host-module registration (24 active
//	    proxy_* + 8 active wasi_* + 15 stub-Unimplemented per §5).
//	(b) vm.Run(ctx, mod, 1) — re-compile mod.Source() against
//	    vm.runtime (sub-ms shared codegen cache hit) + Instantiate +
//	    _initialize + proxy_on_context_create(root, 0) +
//	    proxy_on_vm_start(root, 0) + proxy_on_configure(root, 0).
//	(c) vm.Close() — runtime teardown.
//
// This is the "headers-only bridge surface" — no proxy_on_request_headers
// / proxy_on_response_headers dispatch (those are the per-callback
// hot-path overhead measured separately).
//
// The minimal proxy-wasm guest is buildMinimalProxyWasm (defined in
// wasm_fixtures_test.go) — it exports `_initialize` (no-op) +
// `proxy_abi_version_0_2_1` + a 1-page memory; it does NOT export
// proxy_on_vm_start / proxy_on_configure / proxy_on_context_create, so
// the lifecycle dispatch path is exercised but each callback is a
// no-op-on-missing-export (matches upstream's "nullptr the function
// pointer" discipline per ADR-0204 + §3.3).
func BenchmarkPerStreamVM_Construction_Headers(b *testing.B) {
	ctx := context.Background()
	cache := internalwasm.NewCompileCache(ctx)
	defer func() { _ = cache.Close() }()

	// Compile the minimal valid wasm fixture ONCE outside the timed
	// loop. The content-addressed cache means subsequent CompileModule
	// calls with the same src return the same *Module pointer (hot
	// cache hit; not measured here). This matches the production
	// per-listener config wiring: cc.module is compiled once at filter
	// factory construction (Task 8), then re-used across every
	// per-stream Run on the shared cache.
	src := buildMinimalProxyWasm()
	mod, err := internalwasm.CompileModule(ctx, src, cache)
	if err != nil {
		b.Fatalf("CompileModule err = %v; want nil", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Mirrors production: per-stream NewVM + WithCompilationCache
		// (shared wazero.CompilationCache from the *CompileCache) so
		// vm.Run's re-compile of mod.Source() against vm.runtime hits
		// the shared codegen cache sub-ms instead of paying the full
		// wazero codegen cost on every stream.
		vm := internalwasm.NewVM(ctx,
			internalwasm.WithCompilationCache(cache.WazeroCompilationCache()),
		)

		// rootContextID=1 matches the production root-context seeding
		// at filter factory construction. Lifecycle dispatch (steps
		// c..e of vm.Run) is gated by the default-deny sandbox; the
		// zero-value SandboxConfig denies all 8 lifecycle callbacks per
		// §3.3 — so the per-callback ExportedFunction lookup + IsAllowed
		// check costs are exercised but no guest code beyond
		// _initialize actually runs. This matches the headers-only
		// bridge benchmark scope per D-P4.
		if err := vm.Run(ctx, mod, 1); err != nil {
			b.Fatalf("vm.Run err = %v; want nil", err)
		}

		// Mirrors filter.OnDestroy per encode_headers.go's per-stream
		// VM release at end-of-stream.
		if err := vm.Close(); err != nil {
			b.Fatalf("vm.Close err = %v; want nil", err)
		}
	}
}
