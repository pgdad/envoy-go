package wasm

// perroute.go — per-route wholesale Wasm TPFC override per phase-25.3
// Task 6 + AMEND-C1 + ADR-0210.
//
// # AMEND-C1: wholesale Wasm message replacement
//
// The per-route override for envoy.filters.http.wasm is a WHOLESALE `Wasm`
// message replacement — there is NO `WasmPerRoute` type. When a route carries
// a per-route typed_per_filter_config, the ENTIRE config (including its VM)
// swaps per-route. This is the 5th-canonical REUSE-by-absence shape per
// ADR-0210 (EXPLICIT-NO-NEW-CANONICAL): we reuse the EXISTING compiledConfig
// + buildCompiledConfig machinery without introducing a new canonical type.
//
// # parsePerRouteWasm — type-assert + anypb round-trip → buildCompiledConfig
//
// The ADR-0110 single-chokepoint validator shape (proto.Message in, typed
// config + error out) mirrors parsePerRouteLua in internal/filter/http/lua/
// perroute.go. The function type-asserts the proto.Message to *wasmv3.Wasm,
// wraps it back into an *anypb.Any (required by buildCompiledConfig's
// signature), and delegates to buildCompiledConfig with a nil factoryCtx.
//
// Nil factoryCtx: per the comment near compiled_config.go line ~974, a nil
// factoryCtx is tolerated on the test-double / per-route-compile path. The
// dispatcher + stats wiring for the per-route VM lands at Task 9; for now the
// per-route compiledConfig validates shape + builds the RootVM with no stats
// surface.
//
// # resolvePerRoute — 2-tier most-specific-wins
//
// Returns the effective *compiledConfig for a stream: the per-route override
// when present (non-nil), else the listener default. The framework's
// PerRouteConfig.ResolveAllTiers collapses Route>VirtualHost>RouteConfiguration
// into a single routeCfg upstream of this call (wired at Task 9).
// nil routeCfg ⇒ listener default.
//
// # Single-source-of-truth for error wording
//
// An invalid per-route Wasm config returns the SAME byte-stable PARSE-REJECT
// wording as the listener path (single source of truth = buildCompiledConfig).
// No new error strings are introduced here.

import (
	"context"
	"fmt"

	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// parsePerRouteWasm compiles a per-route wholesale Wasm TPFC override into a
// *compiledConfig per AMEND-C1 (NO WasmPerRoute type — the per-route override
// is a full Wasm message; the entire compiledConfig, hence its *RootVM, swaps
// per-route) + ADR-0210 (EXPLICIT-NO-NEW-CANONICAL; reuses buildCompiledConfig).
//
// It type-asserts m to *wasmv3.Wasm and delegates to buildCompiledConfig via
// an anypb round-trip. An invalid per-route Wasm returns the SAME byte-stable
// PARSE-REJECT wording as the listener path (single source of truth =
// buildCompiledConfig). Mirrors lua's parsePerRouteLua shape.
//
// Wired into production at Task 9 (per-stream dispatch resolution) via
// (*compiledConfig).resolveEffective. The factoryCtx param carries the live
// listener FactoryCtx (retained on the listener compiledConfig) so the per-
// route *RootVM gets the SAME live ClusterManager / HTTPClient dispatcher as
// the listener VM (Task-6 review carried-concern #1: a zero-value FactoryCtx
// would leave proxy_http_call without a dispatcher).
func parsePerRouteWasm(m proto.Message, factoryCtx envoyhttp.FactoryCtx) (*compiledConfig, error) {
	// Group 1: type-assert to *wasmv3.Wasm. A misconfigured registry or test
	// passing the wrong proto type gets a clean, actionable type-assert error
	// rather than a nil-pointer panic — mirrors parsePerRouteLua's posture per
	// ADR-0018 never-panic.
	w, ok := m.(*wasmv3.Wasm)
	if !ok {
		return nil, fmt.Errorf("wasm: per-route: expected *wasmv3.Wasm, got %T", m)
	}

	// Group 2: anypb round-trip. buildCompiledConfig's signature requires an
	// *anypb.Any (to support the full UnmarshalTo + arm-2 unmarshal-fail path).
	// We round-trip the already-typed *wasmv3.Wasm back through anypb.New so
	// all 18 PARSE-REJECT arms in buildCompiledConfig fire correctly.
	any, err := anypb.New(w)
	if err != nil {
		return nil, err
	}

	// Group 3: delegate to buildCompiledConfig with the SUPPLIED factoryCtx.
	// At Task 9 dispatch resolution the caller (resolveEffective) passes the
	// listener's retained live FactoryCtx so the per-route *RootVM gets the
	// live ClusterManager / HTTPClient dispatcher + the per-plugin stats
	// surface (carried-concern #1). Per ADR-0085 nil-tolerance a zero-value
	// FactoryCtx is still safe (stats nil → noop; no dispatcher → proxy_http_
	// call returns InternalFailure) — that path is what the Task-6 perroute_
	// test.go callers exercise.
	return buildCompiledConfig(context.Background(), any, factoryCtx)
}

// resolvePerRoute returns the effective *compiledConfig for a stream: the
// per-route override when present, else the listener default (2-tier
// most-specific-wins). The framework's PerRouteConfig.ResolveAllTiers collapses
// Route>VirtualHost>RouteConfiguration into routeCfg upstream of this call
// (wired at Task 9 via resolveEffective). nil routeCfg ⇒ listener default.
func resolvePerRoute(routeCfg, listenerCfg *compiledConfig) *compiledConfig {
	if routeCfg != nil {
		return routeCfg
	}
	return listenerCfg
}
