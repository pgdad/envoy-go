package wasm

// root_abi_callbacks.go — per-RootVM multiplexer that routes guest hostcalls
// to the originating per-stream *abiCallbacks per 25.2 SPEC §4.3 + Task 18
// closure of D-P-PLAN-6.
//
// # Why a multiplexer
//
// `internalwasm.RootVM.RegisterABICallbacks` installs ONE consumer-side
// ABICallbacks bundle per *RootVM. Per 25.2 SPEC §4.3 + ADR-0205, the
// *RootVM is SHARED across all per-stream contexts on a given compiledConfig.
// Per-stream HTTP filter state (decoderCb, encoderCb, requestHeaders,
// responseHeaders, sentLocalResponse, body accumulators, etc.) lives on the
// per-stream *filter — so the host-side ABICallbacks dispatch MUST route to
// the *abiCallbacks bound to the per-stream *filter whose streamCtxID matches
// the currently-dispatching context.
//
// rootABICallbacks is the per-RootVM lookup table. It registers itself once
// at compiledConfig.New() via rv.RegisterABICallbacks(cb); per-stream
// abiCallbacks are inserted at decode_headers.go DecodeHeaders entry
// (after rootVM.NewStreamContext returns the StreamContext) + removed at
// OnDestroy. Every ABICallbacks method on rootABICallbacks looks up the
// per-stream *abiCallbacks via the streamCtxID argument (or via
// CurrentCtxID() for the GetLogLevel / GetCurrentTimeNanoseconds /
// SetEffectiveContext methods that lack a context-id parameter) and
// delegates to the per-stream methods.
//
// # Concurrency
//
// The map is guarded by sync.RWMutex. Per-stream registrations + de-
// registrations happen at filter dispatch entry/exit (low rate, serialized
// per-stream by the framework's per-stream-single-goroutine invariant);
// lookups happen on every hostcall (high rate). RWMutex is the right
// shape: cheap reads on the hot path, occasional writes on the cold path.
// Per ADR-0205: the dispatchMu in *RootVM serializes proxy_on_* invocations
// across ALL streams, so concurrent hostcalls FROM the same guest dispatch
// always serialize through the same in-flight stream context. The map is
// for cross-stream isolation (parallel HTTP streams firing dispatch on
// distinct contexts), not for hostcall-within-dispatch concurrency.
//
// # Nil-tolerance per ADR-0085
//
// Lookup miss returns a no-op fallback ABICallbacks (allCallbacksNoOp). A
// miss can happen if (a) the stream context has been removed (OnDestroy
// ran but a stray hostcall slipped through), or (b) the dispatch is on the
// root context (rootCtxID, used for proxy_on_vm_start / proxy_on_configure
// / proxy_on_tick). The root-context dispatch never reads per-stream state
// (the guest fires these hooks against the root context which has no
// per-stream filter); the no-op fallback returns sensible defaults so the
// host-side dispatch completes without a panic.

import (
	"context"
	"sync"
	"time"

	internalwasm "github.com/pgdad/envoy-go/internal/wasm"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// rootABICallbacks is the per-RootVM ABICallbacks multiplexer. Holds a
// streamCtxID → *abiCallbacks map; per-stream abiCallbacks are registered
// at DecodeHeaders entry + removed at OnDestroy.
//
// Compile-time conformance: *rootABICallbacks satisfies
// internalwasm.ABICallbacks. The static assertion below pins the shape;
// any drift in the upstream interface forces this file to compile-fail
// alongside abi_callbacks.go.
type rootABICallbacks struct {
	rv    *internalwasm.RootVM
	mu    sync.RWMutex
	perCB map[uint32]*abiCallbacks // streamCtxID → per-stream abiCallbacks
}

// Compile-time conformance: *rootABICallbacks satisfies
// internalwasm.ABICallbacks.
var _ internalwasm.ABICallbacks = (*rootABICallbacks)(nil)

// newRootABICallbacks constructs a fresh multiplexer for the given *RootVM.
// The rv pointer is retained for CurrentCtxID() lookups in the
// parameter-less ABICallbacks methods (GetLogLevel /
// GetCurrentTimeNanoseconds / SetEffectiveContext).
func newRootABICallbacks(rv *internalwasm.RootVM) *rootABICallbacks {
	return &rootABICallbacks{
		rv:    rv,
		perCB: make(map[uint32]*abiCallbacks),
	}
}

// register installs the per-stream *abiCallbacks under the given streamCtxID.
// Called by decode_headers.go DecodeHeaders entry AFTER
// rootVM.NewStreamContext returns the StreamContext.
func (r *rootABICallbacks) register(streamCtxID uint32, cb *abiCallbacks) {
	r.mu.Lock()
	r.perCB[streamCtxID] = cb
	r.mu.Unlock()
}

// unregister removes the per-stream *abiCallbacks entry. Called by
// encode_headers.go OnDestroy AFTER streamCtx.Close fires the lifecycle
// teardown callbacks. Idempotent — a second unregister is a no-op against
// a missing key.
func (r *rootABICallbacks) unregister(streamCtxID uint32) {
	r.mu.Lock()
	delete(r.perCB, streamCtxID)
	r.mu.Unlock()
}

// lookup returns the per-stream *abiCallbacks for streamCtxID + a bool
// indicating presence. Used by every per-stream ABICallbacks method below.
func (r *rootABICallbacks) lookup(streamCtxID uint32) (*abiCallbacks, bool) {
	r.mu.RLock()
	cb, ok := r.perCB[streamCtxID]
	r.mu.RUnlock()
	return cb, ok
}

// lookupCurrent returns the per-stream *abiCallbacks for the currently-
// dispatching context (rv.CurrentCtxID()) + a bool indicating presence.
// Used by the 3 ABICallbacks methods that lack a streamCtxID parameter
// (GetLogLevel / GetCurrentTimeNanoseconds / SetEffectiveContext).
func (r *rootABICallbacks) lookupCurrent() (*abiCallbacks, bool) {
	return r.lookup(r.rv.CurrentCtxID())
}

// -----------------------------------------------------------------------------
// 7 header-map methods — route by streamCtxID.
// -----------------------------------------------------------------------------

func (r *rootABICallbacks) GetHeaderMap(ctx context.Context, streamCtxID uint32, mapType abi.WasmHeaderMapType) ([]internalwasm.HeaderPair, bool) {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return nil, false
	}
	return cb.GetHeaderMap(ctx, streamCtxID, mapType)
}

func (r *rootABICallbacks) SetHeaderMapPairs(ctx context.Context, streamCtxID uint32, mapType abi.WasmHeaderMapType, pairs []internalwasm.HeaderPair) abi.WasmResult {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return abi.WasmResultUnimplemented
	}
	return cb.SetHeaderMapPairs(ctx, streamCtxID, mapType, pairs)
}

func (r *rootABICallbacks) GetHeaderMapValue(ctx context.Context, streamCtxID uint32, mapType abi.WasmHeaderMapType, key string) (string, bool) {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return "", false
	}
	return cb.GetHeaderMapValue(ctx, streamCtxID, mapType, key)
}

func (r *rootABICallbacks) AddHeaderMapValue(ctx context.Context, streamCtxID uint32, mapType abi.WasmHeaderMapType, key, value string) abi.WasmResult {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return abi.WasmResultUnimplemented
	}
	return cb.AddHeaderMapValue(ctx, streamCtxID, mapType, key, value)
}

func (r *rootABICallbacks) ReplaceHeaderMapValue(ctx context.Context, streamCtxID uint32, mapType abi.WasmHeaderMapType, key, value string) abi.WasmResult {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return abi.WasmResultUnimplemented
	}
	return cb.ReplaceHeaderMapValue(ctx, streamCtxID, mapType, key, value)
}

func (r *rootABICallbacks) RemoveHeaderMapValue(ctx context.Context, streamCtxID uint32, mapType abi.WasmHeaderMapType, key string) abi.WasmResult {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return abi.WasmResultUnimplemented
	}
	return cb.RemoveHeaderMapValue(ctx, streamCtxID, mapType, key)
}

func (r *rootABICallbacks) GetHeaderMapSize(ctx context.Context, streamCtxID uint32, mapType abi.WasmHeaderMapType) uint32 {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return 0
	}
	return cb.GetHeaderMapSize(ctx, streamCtxID, mapType)
}

// -----------------------------------------------------------------------------
// GetProperty + SetProperty — route by streamCtxID.
// -----------------------------------------------------------------------------

func (r *rootABICallbacks) GetProperty(ctx context.Context, streamCtxID uint32, path []string) ([]byte, bool) {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return nil, false
	}
	return cb.GetProperty(ctx, streamCtxID, path)
}

func (r *rootABICallbacks) SetProperty(ctx context.Context, streamCtxID uint32, path []string, value []byte) abi.WasmResult {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return abi.WasmResultOk // no-op per 25.1 SPEC: property tree read-only
	}
	return cb.SetProperty(ctx, streamCtxID, path, value)
}

// -----------------------------------------------------------------------------
// SendLocalResponse — route by streamCtxID.
// -----------------------------------------------------------------------------

func (r *rootABICallbacks) SendLocalResponse(ctx context.Context, streamCtxID uint32, statusCode uint32, statusMsg string, body string, additionalHeaders []internalwasm.HeaderPair, grpcStatus int32) abi.WasmResult {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return abi.WasmResultInternalFailure
	}
	return cb.SendLocalResponse(ctx, streamCtxID, statusCode, statusMsg, body, additionalHeaders, grpcStatus)
}

// -----------------------------------------------------------------------------
// GetStatus — route by streamCtxID.
// -----------------------------------------------------------------------------

func (r *rootABICallbacks) GetStatus(ctx context.Context, streamCtxID uint32) (uint32, []byte, bool) {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return 0, nil, false
	}
	return cb.GetStatus(ctx, streamCtxID)
}

// -----------------------------------------------------------------------------
// Log — route by streamCtxID (best-effort; degrades to no-op on miss).
// -----------------------------------------------------------------------------

func (r *rootABICallbacks) Log(ctx context.Context, streamCtxID uint32, level abi.LogLevel, msg string) {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		// Root-context log (proxy_on_vm_start / proxy_on_configure / tick).
		// Best-effort drop — no per-stream filter to log to. The
		// internal/wasm WASI shim already routes wasi_snapshot_preview1
		// fd_write to rv.logSink; proxy_log is the guest-explicit log
		// surface + drops are acceptable.
		return
	}
	cb.Log(ctx, streamCtxID, level, msg)
}

// -----------------------------------------------------------------------------
// GetLogLevel — no streamCtxID parameter; uses CurrentCtxID().
// -----------------------------------------------------------------------------

func (r *rootABICallbacks) GetLogLevel(ctx context.Context) abi.LogLevel {
	cb, ok := r.lookupCurrent()
	if !ok {
		return abi.LogLevelInfo // 25.1 default
	}
	return cb.GetLogLevel(ctx)
}

// -----------------------------------------------------------------------------
// GetCurrentTimeNanoseconds — no streamCtxID; uses CurrentCtxID() for context.
// -----------------------------------------------------------------------------

func (r *rootABICallbacks) GetCurrentTimeNanoseconds(ctx context.Context) uint64 {
	cb, ok := r.lookupCurrent()
	if !ok {
		// Root-context dispatch: just return time.Now nanoseconds (mirrors
		// the per-stream default at abi_callbacks.go).
		//nolint:gosec // time.Now().UnixNano() returns int64; uint64 conversion is wire-protocol intent.
		return uint64(time.Now().UnixNano())
	}
	return cb.GetCurrentTimeNanoseconds(ctx)
}

// -----------------------------------------------------------------------------
// SetEffectiveContext — no streamCtxID; uses CurrentCtxID() to find the
// CURRENT context's *abiCallbacks. The contextID argument is the TARGET
// context; the RootVM dispatch in registration.go already updates
// rv.currentCtxID before invoking this method, so we can route via the
// target streamCtxID directly.
// -----------------------------------------------------------------------------

func (r *rootABICallbacks) SetEffectiveContext(ctx context.Context, contextID uint32) abi.WasmResult {
	cb, ok := r.lookup(contextID)
	if !ok {
		// Root-context or stale context: accept the switch as a no-op success
		// per 25.1 + Q3 (the guest may switch to root context for the tick
		// dispatch; per-stream filter state is not consulted).
		return abi.WasmResultOk
	}
	return cb.SetEffectiveContext(ctx, contextID)
}

// -----------------------------------------------------------------------------
// Done — route by streamCtxID.
// -----------------------------------------------------------------------------

func (r *rootABICallbacks) Done(ctx context.Context, streamCtxID uint32) abi.WasmResult {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return abi.WasmResultOk
	}
	return cb.Done(ctx, streamCtxID)
}

// -----------------------------------------------------------------------------
// 5 buffer-related methods (per 25.2 Task 4 ABICallbacks interface extension).
// -----------------------------------------------------------------------------

func (r *rootABICallbacks) GetBuffer(ctx context.Context, streamCtxID uint32, bufferType abi.WasmBufferType) ([]byte, error) {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return nil, nil
	}
	return cb.GetBuffer(ctx, streamCtxID, bufferType)
}

func (r *rootABICallbacks) SetBuffer(ctx context.Context, streamCtxID uint32, bufferType abi.WasmBufferType, start uint32, data []byte) abi.WasmResult {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return abi.WasmResultInternalFailure
	}
	return cb.SetBuffer(ctx, streamCtxID, bufferType, start, data)
}

func (r *rootABICallbacks) GetBufferStatus(ctx context.Context, streamCtxID uint32, bufferType abi.WasmBufferType) (uint32, uint32, error) {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return 0, 0, nil
	}
	return cb.GetBufferStatus(ctx, streamCtxID, bufferType)
}

func (r *rootABICallbacks) ContinueStream(ctx context.Context, streamCtxID uint32, streamType uint32) abi.WasmResult {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return abi.WasmResultInternalFailure
	}
	return cb.ContinueStream(ctx, streamCtxID, streamType)
}

func (r *rootABICallbacks) CloseStream(ctx context.Context, streamCtxID uint32, streamType uint32) abi.WasmResult {
	cb, ok := r.lookup(streamCtxID)
	if !ok {
		return abi.WasmResultInternalFailure
	}
	return cb.CloseStream(ctx, streamCtxID, streamType)
}
