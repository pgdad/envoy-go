// Package wasm — http_dispatcher_adapter.go: production wasm.HTTPDispatcher
// adapter wrapping the per-listener `*cluster.Manager` + `*httpclient.Client`
// for the `*RootVM` outbound HTTP dispatch surface per 25.2 SPEC §3.1 + §5.1
// #37 + §11.3 D-25.2-3 + AMEND-B3 + R-25.2-3 + ADR-0177 RE-CONSUMER (3rd or
// later co-consumer after phase-22.2 lua `:httpCall()` 2nd).
//
// # Task 20 fix-up #2 closure (HTTPDispatcher production wiring)
//
// Task 8's `internal/wasm/http_call.go` introduced the `wasm.HTTPDispatcher`
// orchestration seam consumed by `*RootVM.DispatchHttpCall`. Task 14's
// `compiled_config.go::New` had a wiring gap: `WithRootHTTPDispatcher` was
// NOT applied, so `rv.httpDispatcher == nil` → every `proxy_http_call`
// hostcall returned `WasmResultInternalFailure` BEFORE any counter could
// fire. That gap blocked fixture-0036 arms (l) httpCall-success + (m)
// httpCall-unknown-cluster.
//
// THIS FILE closes the gap by:
//
//   - Authoring `wasmHTTPDispatcher` — a thin adapter struct satisfying the
//     `wasm.HTTPDispatcher` interface by delegating `HasCluster` to
//     `*cluster.Manager.Get` + `Dispatch` to
//     `*httpclient.Client.ClusterDispatch` (the SAME phase-20 framework
//     primitive consumed by phase-22.2 lua's `:httpCall()` per ADR-0177's
//     2nd co-consumer landing).
//
//   - Providing `newWasmHTTPDispatcher(clusterMgr, client)` for construction
//     at `compiledConfig.New` time. compiled_config.go appends
//     `wasm.WithRootHTTPDispatcher(adapter)` to the per-RootVM options
//     when BOTH `factoryCtx.ClusterManager` + `factoryCtx.HTTPClient` are
//     non-nil per ADR-0085 nil-tolerance (test-double paths that bypass
//     the full FactoryCtx wiring continue to construct a RootVM with a
//     nil-default httpDispatcher → proxy_http_call returns
//     InternalFailure exactly as documented).
//
// # Phase-22.2 lua :httpCall() precedent
//
// The adapter pattern mirrors phase-22.2's lua filter at
// `internal/filter/http/lua/httpcall.go::runHTTPCall` which consumes BOTH
// `*httpclient.Client` + `*cluster.Manager` references at the filter scope
// + invokes `httpclient.Client.ClusterDispatch(ctx, cluster, req,
// clusterMgr)` directly. The 22.2 surface is synchronous + lives at the
// per-stream filter package; the 25.2 wasm surface is asynchronous + lives
// at the per-RootVM package (the wasm filter's hostcall body returns
// immediately + the dispatch goroutine routes the response back through
// `proxy_on_http_call_response`). The orchestration seam (HTTPDispatcher
// interface) absorbs that async dispatch lifecycle in the wasm package —
// the adapter here is the production wiring of the seam.
//
// NO API extension on httpclient per parent SPEC §13-R6 invariant — the
// 22.2 `ClusterDispatch + cluster.Manager.Get` surface covers 25.2's
// `proxy_http_call` byte-for-byte. This adapter is the wasm-package-private
// glue that bridges the orchestration seam to the framework primitive; the
// httpclient public surface is UNCHANGED.
//
// # Cancel-at-destruction (AMEND-B3 + R-25.2-3)
//
// The adapter's Dispatch method honors the supplied `ctx` verbatim — when
// `*RootVM.cancelOutstandingHttpCalls` cancels the per-call context at
// `StreamContext.Close` time, the in-flight `*http.Client.Do` returns with
// the ctx-cancellation error + the dispatch goroutine's
// `handleHttpCallResponse` early-returns via the token-miss guard (the
// pendingHttpCall entry was already removed from the map). The adapter
// itself adds NO additional cancellation state — the framework primitive
// `httpclient.Client.ClusterDispatch` already threads ctx through stdlib
// `*http.Client.Do` per the standard ctx-cancellation discipline.
//
// # Cross-references
//
//   - 25.2 SPEC §3.1 (HTTPDispatcher orchestration seam)
//   - 25.2 SPEC §5.1 #37 + §11.3 D-25.2-3 (proxy_http_call wire shape)
//   - AMEND-B3 (cancel-at-destruction + http_call_response_after_close
//     defensive observability counter)
//   - R-25.2-3 (3rd or later co-consumer of phase-20 ADR-0177)
//   - parent SPEC §13-R6 RATIFIED-PENDING-IMPL anchor closure at Task 8
//   - ADR-0177 §Decision (httpclient framework primitive surface
//     stability)
//   - phase-22.2 lua httpcall.go (2nd co-consumer precedent)
//   - Task 8 internal/wasm/http_call.go (HTTPDispatcher interface
//     declaration)
//   - Task 14 internal/filter/http/wasm/compiled_config.go (production
//     wiring at compiledConfig.New)
package wasm

import (
	"context"
	"net/http"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/httpclient"
	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
)

// wasmHTTPDispatcher adapts the per-listener `*cluster.Manager` +
// `*httpclient.Client` framework primitives to satisfy
// `internalwasm.HTTPDispatcher`. Constructed at `compiledConfig.New` per
// the phase-22.2 lua `:httpCall()` precedent + wired via
// `internalwasm.WithRootHTTPDispatcher` on the per-RootVM option chain.
//
// The adapter holds NO additional state beyond the two framework-primitive
// references — both are read-only at runtime per their respective ADR-0010
// + ADR-0177 disciplines (cluster.Manager is build-time-then-frozen;
// httpclient.Client is goroutine-safe by design). Multiple concurrent
// dispatch goroutines spawned from the same `*RootVM` may invoke Dispatch
// concurrently; the adapter does NO synchronization of its own.
type wasmHTTPDispatcher struct {
	// clusterMgr is the per-listener cluster manager threaded from
	// FactoryCtx.ClusterManager. Resolves cluster names to *cluster.Cluster
	// for the HasCluster check + the ClusterDispatch endpoint-resolution.
	// Must be non-nil at construction time per newWasmHTTPDispatcher's
	// caller-side nil guard at compiled_config.New.
	clusterMgr *cluster.Manager

	// client is the shared *httpclient.Client framework primitive threaded
	// from FactoryCtx.HTTPClient. Performs the actual outbound HTTP request
	// via ClusterDispatch per ADR-0177. Must be non-nil at construction
	// time per the caller-side nil guard.
	client *httpclient.Client
}

// newWasmHTTPDispatcher constructs the production HTTPDispatcher adapter
// per Task 20 fix-up #2. Caller (compiled_config.go::New) MUST ensure both
// arguments are non-nil — the constructor does NOT defensively check
// (returning nil would silently revert the RootVM to the no-dispatcher
// path; failing loud at the call site is the preferred discipline per
// ADR-0072 boot-time-fail-fast).
func newWasmHTTPDispatcher(clusterMgr *cluster.Manager, client *httpclient.Client) *wasmHTTPDispatcher {
	return &wasmHTTPDispatcher{
		clusterMgr: clusterMgr,
		client:     client,
	}
}

// HasCluster reports whether the named cluster is registered in the per-
// listener cluster manager. Used by `(*RootVM).DispatchHttpCall` for the
// up-front BadArgument-on-unknown-cluster check per Q4 + AMEND-B3 + cpp-
// host context.cc:1547-1550 byte-faithfulness.
//
// Returns true only when cluster.Manager.Get succeeds. The cluster lookup
// is read-only per ADR-0010 build-time-then-frozen — concurrent HasCluster
// calls are safe.
func (d *wasmHTTPDispatcher) HasCluster(name string) bool {
	if d.clusterMgr == nil {
		return false
	}
	_, ok := d.clusterMgr.Get(name)
	return ok
}

// Dispatch performs the outbound HTTP request synchronously via the
// underlying `*httpclient.Client.ClusterDispatch`. The caller controls
// request lifetime via the supplied `ctx` — at cancel-at-destruction the
// `*RootVM` cancels the per-call ctx + Dispatch returns with the ctx-
// cancellation error per AMEND-B3.
//
// Returns:
//
//   - (resp, nil) on successful dispatch (any HTTP status — the
//     `(*RootVM).handleHttpCallResponse` path materializes the response
//     headers + body for the guest's `proxy_on_http_call_response`
//     callback regardless of status code, mirroring proxy-wasm's
//     status-agnostic response routing per §5.3 C19).
//
//   - (nil, err) on cluster-lookup miss (errClusterNotFound from
//     ClusterDispatch when the cluster was registered at HasCluster time
//     but drained between the gate + the dispatch — defensive; HasCluster
//     already gates the common case), transport error, or ctx
//     cancellation. The `(*RootVM).dispatchHttpCallGoroutine` routes the
//     error through `handleHttpCallResponse` which invokes
//     `proxy_on_http_call_response` with degenerate (num_headers=0,
//     body_size=0, num_trailers=0) per §5.3 C19.
//
// The response body is consumed by the wasm-side
// `(*RootVM).handleHttpCallResponse` via `io.ReadAll(resp.Body)` + closed
// — Dispatch itself does NOT drain or close the body (connection-pool
// reuse discipline is the consumer's responsibility, mirroring stdlib
// `*http.Client.Do` contract).
func (d *wasmHTTPDispatcher) Dispatch(ctx context.Context, clusterName string, req *http.Request) (*http.Response, error) {
	// Thread the context onto the request before delegating — the request
	// arrived from buildHttpCallRequest with the per-call timeout context
	// already attached, but the dispatch ctx may be a SUPERSET (the
	// dispatch goroutine spawned from DispatchHttpCall passes req.Context()
	// verbatim per http_call.go::dispatchHttpCallGoroutine — so the two
	// ctxs are typically the same instance). Defensive re-thread via
	// WithContext keeps the contract crisp for future callers that may
	// supply a distinct ctx.
	if req != nil && req.Context() != ctx {
		req = req.WithContext(ctx)
	}
	return d.client.ClusterDispatch(ctx, clusterName, req, d.clusterMgr)
}

// Compile-time guard: wasmHTTPDispatcher satisfies internalwasm.HTTPDispatcher.
var _ internalwasm.HTTPDispatcher = (*wasmHTTPDispatcher)(nil)
