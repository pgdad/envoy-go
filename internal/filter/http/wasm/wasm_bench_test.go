// wasm_bench_test.go — Task 17 R8 benchmark gate per the 25.1 SPEC §13-R8
// + parent §13-R8 + D-P-PLAN-10 + D-P4, REVISED at 25.2 Task 18 for the
// root-VM model.
//
// BenchmarkPerStreamContext_Construction_Headers measures the per-stream
// StreamContext construction + proxy_on_context_create dispatch cost
// the production DecodeHeaders incurs at the 25.2 headers-only bridge
// surface. The reported ns/op gates ADR-0205 firing at this Task 17 atomic
// landing per the R8 signaling protocol:
//
//   - ns/op <= 1_000_000 (= 1ms) → WEAK-default per-stream VM construction
//     STANDS; ADR-0205 NOT consumed; the WEAK-HOLD 1-slot escape-valve
//     buffer is RELEASED + carries forward to 25.2 BRAINSTORM as the
//     family-level escape valve.
//   - ns/op  > 1_000_000 (= 1ms) → escape-valve FIRES; the per-module
//     wazero Runtime pool with pre-instantiated entries per ADR-0205
//     §Decision lands.
//
// At 25.2 the per-stream construction cost is MICROSECONDS (just bookkeeping
// + a proxy_on_context_create invocation on the shared RootVM instance);
// the ADR-0205 root-VM lifecycle evolution has already been consumed (the
// 25.1 per-stream wasm.NewVM construction at ~61µs/stream is RETIRED at
// Task 1 + 18 per D-P-PLAN-6).
//
// Run via:
//
//	go test -count=1 -benchmem \
//	        -bench=BenchmarkPerStreamContext_Construction_Headers \
//	        -run=^$ ./internal/filter/http/wasm/

package wasm

import (
	"context"
	"testing"

	internalwasm "github.com/pgdad/envoy-go/internal/wasm"
)

// BenchmarkPerStreamContext_Construction_Headers measures the per-stream
// StreamContext construction + proxy_on_context_create dispatch cost the
// production filter dispatch incurs per per-stream per Q3 + ADR-0205
// root-VM lifecycle evolution. Mirrors phase-22.1's
// BenchmarkPerStreamLState_Construction_Headers shape with the 25.2
// root-VM-model adjustments.
//
// Build the *RootVM + run Configure ONCE outside the timed loop (these
// are per-compiledConfig operations; the per-stream cost is just
// NewStreamContext + Close). The benchmark exercises:
//
//	(a) rootVM.NewStreamContext(ctx) — bookkeeping + proxy_on_context_
//	    create(streamCtxID, rootCtxID) dispatch.
//	(b) streamCtx.Close(ctx) — fires proxy_on_done + proxy_on_log +
//	    proxy_on_delete + cancels outstanding httpCalls per AMEND-B3.
//
// This is the "headers-only bridge surface" — no proxy_on_request_headers /
// proxy_on_response_headers dispatch (those are the per-callback hot-path
// overhead measured separately).
//
// The minimal proxy-wasm guest is buildMinimalProxyWasm (defined in
// wasm_fixtures_test.go) — it exports `_initialize` (no-op) +
// `proxy_abi_version_0_2_1` + a 1-page memory; it does NOT export
// proxy_on_vm_start / proxy_on_configure / proxy_on_context_create, so
// the lifecycle dispatch path is exercised but each callback is a
// no-op-on-missing-export (matches upstream's "nullptr the function
// pointer" discipline per ADR-0204 + §3.3).
func BenchmarkPerStreamContext_Construction_Headers(b *testing.B) {
	ctx := context.Background()
	cache := internalwasm.NewCompileCache(ctx)
	defer func() { _ = cache.Close() }()

	src := buildMinimalProxyWasm()
	mod, err := internalwasm.CompileModule(ctx, src, cache)
	if err != nil {
		b.Fatalf("CompileModule err = %v; want nil", err)
	}

	rv, err := internalwasm.NewRootVM(ctx, mod, 1,
		internalwasm.WithRootCompilationCache(cache.WazeroCompilationCache()),
	)
	if err != nil {
		b.Fatalf("NewRootVM err = %v; want nil", err)
	}
	defer func() { _ = rv.Close() }()

	if err := rv.Configure(ctx, nil, nil); err != nil {
		b.Fatalf("rv.Configure err = %v; want nil", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sc, err := rv.NewStreamContext(ctx)
		if err != nil {
			b.Fatalf("NewStreamContext err = %v; want nil", err)
		}
		if err := sc.Close(ctx); err != nil {
			b.Fatalf("sc.Close err = %v; want nil", err)
		}
	}
}

// BenchmarkPerStreamModule_Instantiation is the Task 22 R8-gate benchmark per
// D-P-PLAN-11 + D-25.2-P2 + 25.2 SPEC §15 item 41. Measures the per-stream
// Module-instantiation cost under the 25.2 root-VM model.
//
// Under ADR-0205 the WEAK-default at 25.2 is fresh-per-stream module
// construction: ONE long-lived *RootVM per *compiledConfig (shared
// wazero.Runtime + compiled *Module bytes), with per-stream contexts as
// CHILDREN that share that Runtime + Module + foreign-function registry +
// shared-data + httpCall routing state. The per-stream cost is bookkeeping +
// proxy_on_context_create dispatch on the shared instance — NOT a fresh
// wazero.Runtime.InstantiateModule call.
//
// R8 threshold gate (per 25.2 SPEC §15 item 41 + D-P-PLAN-11):
//
//   - ns/op <= 1_000_000 (1ms) → WEAK-default fresh-per-stream STANDS;
//     ADR-0209 escape-valve STAYS UNCONSUMED + carries to 25.3.
//   - ns/op  > 1_000_000 (1ms) → ADR-0209 escape-valve FIRES at this same
//     Task 22 atomic landing per ADR-0044 — §Context + §Decision +
//     §Consequences body all land at the same commit anchoring the
//     pooled-Module vs shared-Module-with-mutex-serialization decision
//     based on the empirical evidence.
//
// Anticipated outcome per D-P-PLAN-11 + 25.2 SPEC §2.16 (root-VM model
// retires the 25.1 per-stream Runtime construction at ~61µs/stream → 25.2
// per-stream bookkeeping at microseconds): WELL UNDER 1ms; ADR-0209 STAYS
// UNCONSUMED.
//
// Naming note: this is the canonically-named R8 benchmark per the PLAN
// Task 22 commit-message template; the existing
// BenchmarkPerStreamContext_Construction_Headers above is the sibling
// measurement from 25.1 Task 17 that surveys the same hot path under the
// "headers-only bridge surface" framing.
//
// Run via:
//
//	go test -count=1 -benchmem -run=^$ \
//	        -bench=BenchmarkPerStreamModule_Instantiation \
//	        ./internal/filter/http/wasm/
func BenchmarkPerStreamModule_Instantiation(b *testing.B) {
	ctx := context.Background()
	cache := internalwasm.NewCompileCache(ctx)
	defer func() { _ = cache.Close() }()

	src := buildMinimalProxyWasm()
	mod, err := internalwasm.CompileModule(ctx, src, cache)
	if err != nil {
		b.Fatalf("CompileModule err = %v; want nil", err)
	}

	rv, err := internalwasm.NewRootVM(ctx, mod, 1,
		internalwasm.WithRootCompilationCache(cache.WazeroCompilationCache()),
	)
	if err != nil {
		b.Fatalf("NewRootVM err = %v; want nil", err)
	}
	defer func() { _ = rv.Close() }()

	if err := rv.Configure(ctx, nil, nil); err != nil {
		b.Fatalf("rv.Configure err = %v; want nil", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sc, err := rv.NewStreamContext(ctx)
		if err != nil {
			b.Fatalf("NewStreamContext err = %v; want nil", err)
		}
		if err := sc.Close(ctx); err != nil {
			b.Fatalf("sc.Close err = %v; want nil", err)
		}
	}
}

// BenchmarkPerStreamPluginContextLookup is a NEW 25.3 R8-supplementary
// benchmark per BRAINSTORM Q6 + 25.3 SPEC D-25.3-5 (SAME 1ms per-stream-cost
// threshold). It measures the per-stream context-creation cost on a *RootVM
// that was acquired through the process-global registry (ADR-0211 multi-plugin
// VM-sharing) — the path a multi-plugin chain takes where several PluginConfigs
// share one registry-keyed *RootVM and each request creates a fresh per-stream
// child context against the shared instance. The registry AcquireFor lookup
// (composite Sha256(vm_id‖vm_configuration‖code) key + refcount) runs ONCE
// outside the timed loop (it is a per-compiledConfig operation, not per-stream);
// the timed loop is the per-stream NewStreamContext + Close on the shared VM.
//
// R8 threshold gate (SAME 1ms per D-25.3-5):
//
//   - ns/op <= 1_000_000 (1ms) → WEAK-default STANDS; ADR-0209 + ADR-0213
//     escape-valve reserves STAY UNCONSUMED.
//   - ns/op  > 1_000_000 (1ms) → escape-valve FIRES (ADR-0209/0213).
//
// Anticipated: WELL UNDER 1ms (the registry only changes WHERE the shared VM
// comes from; the per-stream cost is identical bookkeeping + dispatch).
//
// Run via:
//
//	go test -count=1 -benchmem -run=^$ \
//	        -bench=BenchmarkPerStreamPluginContextLookup \
//	        ./internal/filter/http/wasm/
func BenchmarkPerStreamPluginContextLookup(b *testing.B) {
	ctx := context.Background()
	cache := internalwasm.NewCompileCache(ctx)
	defer func() { _ = cache.Close() }()

	src := buildMinimalProxyWasm()
	mod, err := internalwasm.CompileModule(ctx, src, cache)
	if err != nil {
		b.Fatalf("CompileModule err = %v; want nil", err)
	}

	reg := internalwasm.NewRegistry()
	rv, key, err := reg.AcquireFor("bench-vm", nil, src, func() (*internalwasm.RootVM, error) {
		return internalwasm.NewRootVM(ctx, mod, 1,
			internalwasm.WithRootCompilationCache(cache.WazeroCompilationCache()),
		)
	})
	if err != nil {
		b.Fatalf("AcquireFor err = %v; want nil", err)
	}
	defer func() { _ = reg.Release(key) }()

	if err := rv.Configure(ctx, nil, nil); err != nil {
		b.Fatalf("rv.Configure err = %v; want nil", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sc, err := rv.NewStreamContext(ctx)
		if err != nil {
			b.Fatalf("NewStreamContext err = %v; want nil", err)
		}
		if err := sc.Close(ctx); err != nil {
			b.Fatalf("sc.Close err = %v; want nil", err)
		}
	}
}

// BenchmarkPerRouteResolve is a NEW 25.3 R8-supplementary benchmark per
// BRAINSTORM Q6 + 25.3 SPEC D-25.3-5. It measures the per-request per-route
// resolution selection cost (ADR-0210 per-route wholesale Wasm TPFC override)
// — the 3-tier resolver (per-route override → listener-level → no-op) that
// runs at DecodeHeaders for every request. resolvePerRoute itself is a pure
// pointer-select over the pre-built per-route and listener-level
// *compiledConfig; the benchmark confirms the per-request resolution cost is
// negligible (the EXPENSIVE per-route compiledConfig build is amortized
// out-of-band, not per-request, per the BUG-1 fix discipline).
//
// SAME 1ms per-stream-cost threshold per D-25.3-5. Anticipated: a handful of
// nanoseconds (pointer comparison + return) — orders of magnitude under 1ms.
//
// Run via:
//
//	go test -count=1 -benchmem -run=^$ \
//	        -bench=BenchmarkPerRouteResolve \
//	        ./internal/filter/http/wasm/
func BenchmarkPerRouteResolve(b *testing.B) {
	routeCfg := &compiledConfig{}
	listenerCfg := &compiledConfig{}

	b.ResetTimer()
	b.ReportAllocs()
	var sink *compiledConfig
	for i := 0; i < b.N; i++ {
		// Alternate the per-route presence so neither branch of the 3-tier
		// resolver is dead-code-eliminated by the compiler.
		if i&1 == 0 {
			sink = resolvePerRoute(routeCfg, listenerCfg)
		} else {
			sink = resolvePerRoute(nil, listenerCfg)
		}
	}
	if sink == nil {
		b.Fatal("resolvePerRoute returned nil; want non-nil")
	}
}
