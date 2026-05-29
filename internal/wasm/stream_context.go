// StreamContext per-stream-callback methods + Close per 25.2 SPEC §3.1 +
// ADR-0205 + Task 1.
//
// *StreamContext is the per-stream filter dispatch context. NOT
// goroutine-safe by contract; access is per-stream-single-goroutine by
// envoy-go's filter dispatch model (per ADR-0071). Each per-stream filter
// dispatch creates a StreamContext via RootVM.NewStreamContext; OnDestroy
// releases via Close.
//
// Cost: microseconds. NO wazero Runtime construction — the runtime is the
// per-RootVM shared one. Construction is just bookkeeping + the
// proxy_on_context_create(streamCtxID, rootCtxID) invocation (which lives
// on the constructor RootVM.NewStreamContext, NOT here — this file's Close
// covers proxy_on_done + proxy_on_log + proxy_on_delete + cancel-at-
// destruction for outstanding httpCalls).
//
// # Cancel-at-destruction (LANDED at Task 8 per AMEND-B3)
//
// Close cancels any outstanding httpCalls dispatched from this stream
// context per AMEND-B3 (cancel-at-destruction). The cancellation walks
// rootVM.httpCalls + filters entries whose originating streamCtxID matches
// sc.streamCtxID + invokes the per-call context.CancelFunc for each.
// Byte-faithful to cpp-host context.cc:1900-1905 destructor pattern.
//
// # Capability gating
//
// Each Call method GATES via the corresponding capProxyOn* key. Cap-denied
// or guest-not-exported step returns the per-callback default (Continue /
// true / nil) — NOT an error. This matches the 25.1 vm.go discipline
// verbatim (per SPEC §3.3).

package wasm

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// StreamContext is the per-stream filter dispatch context. Refer to the
// file header for the lifecycle + concurrency contract.
type StreamContext struct {
	rootVM        *RootVM
	streamCtxID   uint32
	rootContextID uint32

	closeOnce sync.Once
	// closed is set inside closeOnce.Do (per-stream Close) OR by RootVM.Close
	// (cross-goroutine — when the root VM tears down, it flips this on every
	// live child so subsequent per-callback dispatches early-return rather
	// than dereferencing the cleared rv.instance). atomic.Bool removes the
	// data race + mirrors the RootVM.closed precedent at root_vm.go.
	closed atomic.Bool

	// trapped marks the shared instance as POISONED by a guest trap (a wazero
	// RuntimeError surfaced from a proxy_on_* dispatch — e.g. a Rust panic!
	// that aborts to `unreachable`, which leaves the proxy-wasm-rust-sdk
	// dispatcher's RefCell borrowed). Set by any CallProxyOnX method whose
	// underlying guest call returns a non-nil error (BUG-3 fix, phase-25.3 Task
	// 11-fix). Read by Close: a trapped instance MUST NOT be re-entered via the
	// proxy_on_done/log/delete teardown triplet — re-entering a poisoned guest
	// cascades (the second dispatch hits the still-borrowed RefCell →
	// panic_already_borrowed). The shared *RootVM is separately marked Failed
	// (NoteReloadRuntimeError under FAIL_RELOAD) so the next request
	// reinstantiates a fresh instance, clearing the poison for future streams.
	// atomic.Bool because RootVM.Close may observe it cross-goroutine, mirroring
	// the `closed` precedent.
	trapped atomic.Bool
}

// ContextID returns the per-stream context ID allocated by RootVM at
// NewStreamContext time. Exported for caller diagnostics (HTTP filter
// dispatch + logging) — the value is the same one passed to the guest's
// proxy_on_* callbacks as the streamContextID argument.
func (sc *StreamContext) ContextID() uint32 { return sc.streamCtxID }

// RootContextID returns the per-plugin root context ID of the owning
// RootVM. Useful for log lines / propagation; the value is the same one
// the guest received via proxy_on_context_create(streamCtxID, rootCtxID).
func (sc *StreamContext) RootContextID() uint32 { return sc.rootContextID }

// HasGlobalFunc returns true if the named guest export is a callable
// function on the shared instantiated module. Delegated to the parent
// RootVM.
func (sc *StreamContext) HasGlobalFunc(name string) bool {
	return sc.rootVM.HasGlobalFunc(name)
}

// CallProxyOnRequestHeaders invokes `proxy_on_request_headers(streamCtxID,
// numHeaders, endOfStream)` — returns `ProxyAction::CONTINUE` (=0) or
// `::PAUSE` (=1). Gated by capProxyOnRequestHeaders. When denied or when
// the guest does not export the callback, returns ProxyActionContinue +
// nil.
func (sc *StreamContext) CallProxyOnRequestHeaders(ctx context.Context, numHeaders uint32, endOfStream bool) (abi.ProxyAction, error) {
	if sc.closed.Load() {
		return abi.ProxyActionContinue, errors.New("wasm: CallProxyOnRequestHeaders on closed StreamContext")
	}
	rv := sc.rootVM
	if !rv.sandbox.IsAllowed(capProxyOnRequestHeaders) {
		return abi.ProxyActionContinue, nil
	}
	fn := rv.instance.ExportedFunction("proxy_on_request_headers")
	if fn == nil {
		return abi.ProxyActionContinue, nil
	}

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()
	rv.currentCtxID.Store(sc.streamCtxID)

	var endOfStreamU uint64
	if endOfStream {
		endOfStreamU = 1
	}

	var action abi.ProxyAction
	err := rv.runCallWithPanicWrapper(func() error {
		results, err := fn.Call(ctx, uint64(sc.streamCtxID), uint64(numHeaders), endOfStreamU)
		if err != nil {
			return err
		}
		if len(results) > 0 {
			//nolint:gosec // wazero call results are bit-typed by us; truncation is the wire-protocol intent.
			action = abi.ProxyAction(int32(results[0]))
		}
		return nil
	})
	sc.noteTrapOnError(err)
	return action, err
}

// CallProxyOnResponseHeaders invokes `proxy_on_response_headers(streamCtxID,
// numHeaders, endOfStream)`. Cap + export semantics mirror
// CallProxyOnRequestHeaders.
func (sc *StreamContext) CallProxyOnResponseHeaders(ctx context.Context, numHeaders uint32, endOfStream bool) (abi.ProxyAction, error) {
	if sc.closed.Load() {
		return abi.ProxyActionContinue, errors.New("wasm: CallProxyOnResponseHeaders on closed StreamContext")
	}
	rv := sc.rootVM
	if !rv.sandbox.IsAllowed(capProxyOnResponseHeaders) {
		return abi.ProxyActionContinue, nil
	}
	fn := rv.instance.ExportedFunction("proxy_on_response_headers")
	if fn == nil {
		return abi.ProxyActionContinue, nil
	}

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()
	rv.currentCtxID.Store(sc.streamCtxID)

	var endOfStreamU uint64
	if endOfStream {
		endOfStreamU = 1
	}

	var action abi.ProxyAction
	err := rv.runCallWithPanicWrapper(func() error {
		results, err := fn.Call(ctx, uint64(sc.streamCtxID), uint64(numHeaders), endOfStreamU)
		if err != nil {
			return err
		}
		if len(results) > 0 {
			//nolint:gosec // wazero call results are bit-typed by us; truncation is the wire-protocol intent.
			action = abi.ProxyAction(int32(results[0]))
		}
		return nil
	})
	sc.noteTrapOnError(err)
	return action, err
}

// CallProxyOnRequestBody invokes `proxy_on_request_body(streamCtxID,
// bodySize, endOfStream)` per 25.2 SPEC §5.3 C14. bodySize is the
// ACCUMULATED total per Q1 — the host calls this on EACH body chunk + the
// guest reads via proxy_get_buffer_bytes(HttpRequestBody, start, max) against
// the accumulated buffer maintained at internal/filter/http/wasm/body.go.
//
// Returns ProxyAction (Continue/Pause). Gated by capProxyOnRequestBody +
// gate-at-getFunction per AMEND-B5: when the cap is denied OR the guest does
// not export the callback, returns (ProxyActionContinue, nil) — the chain
// proceeds as if the guest hadn't opted into body callbacks.
func (sc *StreamContext) CallProxyOnRequestBody(ctx context.Context, bodySize uint32, endOfStream bool) (abi.ProxyAction, error) {
	if sc.closed.Load() {
		return abi.ProxyActionContinue, errors.New("wasm: CallProxyOnRequestBody on closed StreamContext")
	}
	rv := sc.rootVM
	if !rv.sandbox.IsAllowed(capProxyOnRequestBody) {
		return abi.ProxyActionContinue, nil
	}
	fn := rv.instance.ExportedFunction("proxy_on_request_body")
	if fn == nil {
		return abi.ProxyActionContinue, nil
	}

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()
	rv.currentCtxID.Store(sc.streamCtxID)

	var endOfStreamU uint64
	if endOfStream {
		endOfStreamU = 1
	}

	var action abi.ProxyAction
	err := rv.runCallWithPanicWrapper(func() error {
		results, err := fn.Call(ctx, uint64(sc.streamCtxID), uint64(bodySize), endOfStreamU)
		if err != nil {
			return err
		}
		if len(results) > 0 {
			//nolint:gosec // wazero call results are bit-typed by us; truncation is the wire-protocol intent.
			action = abi.ProxyAction(int32(results[0]))
		}
		return nil
	})
	sc.noteTrapOnError(err)
	return action, err
}

// CallProxyOnResponseBody invokes `proxy_on_response_body(streamCtxID,
// bodySize, endOfStream)` per 25.2 SPEC §5.3 C15. Mirror of
// CallProxyOnRequestBody for the encode side.
func (sc *StreamContext) CallProxyOnResponseBody(ctx context.Context, bodySize uint32, endOfStream bool) (abi.ProxyAction, error) {
	if sc.closed.Load() {
		return abi.ProxyActionContinue, errors.New("wasm: CallProxyOnResponseBody on closed StreamContext")
	}
	rv := sc.rootVM
	if !rv.sandbox.IsAllowed(capProxyOnResponseBody) {
		return abi.ProxyActionContinue, nil
	}
	fn := rv.instance.ExportedFunction("proxy_on_response_body")
	if fn == nil {
		return abi.ProxyActionContinue, nil
	}

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()
	rv.currentCtxID.Store(sc.streamCtxID)

	var endOfStreamU uint64
	if endOfStream {
		endOfStreamU = 1
	}

	var action abi.ProxyAction
	err := rv.runCallWithPanicWrapper(func() error {
		results, err := fn.Call(ctx, uint64(sc.streamCtxID), uint64(bodySize), endOfStreamU)
		if err != nil {
			return err
		}
		if len(results) > 0 {
			//nolint:gosec // wazero call results are bit-typed by us; truncation is the wire-protocol intent.
			action = abi.ProxyAction(int32(results[0]))
		}
		return nil
	})
	sc.noteTrapOnError(err)
	return action, err
}

// CallProxyOnRequestTrailers invokes `proxy_on_request_trailers(streamCtxID,
// numTrailers)` per 25.2 SPEC §5.3 C16. numTrailers is the total trailer
// count (multi-value trailers expand to one entry per value, matching the
// proxy-wasm pair-emission shape).
//
// Returns ProxyAction (Continue/Pause). Gated by capProxyOnRequestTrailers +
// gate-at-getFunction per AMEND-B5.
func (sc *StreamContext) CallProxyOnRequestTrailers(ctx context.Context, numTrailers uint32) (abi.ProxyAction, error) {
	if sc.closed.Load() {
		return abi.ProxyActionContinue, errors.New("wasm: CallProxyOnRequestTrailers on closed StreamContext")
	}
	rv := sc.rootVM
	if !rv.sandbox.IsAllowed(capProxyOnRequestTrailers) {
		return abi.ProxyActionContinue, nil
	}
	fn := rv.instance.ExportedFunction("proxy_on_request_trailers")
	if fn == nil {
		return abi.ProxyActionContinue, nil
	}

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()
	rv.currentCtxID.Store(sc.streamCtxID)

	var action abi.ProxyAction
	err := rv.runCallWithPanicWrapper(func() error {
		results, err := fn.Call(ctx, uint64(sc.streamCtxID), uint64(numTrailers))
		if err != nil {
			return err
		}
		if len(results) > 0 {
			//nolint:gosec // wazero call results are bit-typed by us; truncation is the wire-protocol intent.
			action = abi.ProxyAction(int32(results[0]))
		}
		return nil
	})
	sc.noteTrapOnError(err)
	return action, err
}

// CallProxyOnResponseTrailers invokes `proxy_on_response_trailers(streamCtxID,
// numTrailers)` per 25.2 SPEC §5.3 C17. Mirror of CallProxyOnRequestTrailers
// for the encode side.
func (sc *StreamContext) CallProxyOnResponseTrailers(ctx context.Context, numTrailers uint32) (abi.ProxyAction, error) {
	if sc.closed.Load() {
		return abi.ProxyActionContinue, errors.New("wasm: CallProxyOnResponseTrailers on closed StreamContext")
	}
	rv := sc.rootVM
	if !rv.sandbox.IsAllowed(capProxyOnResponseTrailers) {
		return abi.ProxyActionContinue, nil
	}
	fn := rv.instance.ExportedFunction("proxy_on_response_trailers")
	if fn == nil {
		return abi.ProxyActionContinue, nil
	}

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()
	rv.currentCtxID.Store(sc.streamCtxID)

	var action abi.ProxyAction
	err := rv.runCallWithPanicWrapper(func() error {
		results, err := fn.Call(ctx, uint64(sc.streamCtxID), uint64(numTrailers))
		if err != nil {
			return err
		}
		if len(results) > 0 {
			//nolint:gosec // wazero call results are bit-typed by us; truncation is the wire-protocol intent.
			action = abi.ProxyAction(int32(results[0]))
		}
		return nil
	})
	sc.noteTrapOnError(err)
	return action, err
}

// CallProxyOnHttpCallResponse invokes `proxy_on_http_call_response(plugin_
// context_id, callID, numHeaders, bodySize, numTrailers)` per 25.2 SPEC §5.3
// C19 — invoked by the AsyncClient response goroutine on the originating
// stream context AFTER the per-call token lookup succeeds (or the
// http_call_response_after_close counter increments if the lookup misses per
// AMEND-B3).
//
// At 25.2 the existing dispatchHttpCallResponse in http_call.go consumes the
// proxy_on_http_call_response dispatch directly via the RootVM's instance
// (the per-stream path goes through the RootVM, not through this method).
// This *StreamContext method is provided for symmetry + for any future
// per-stream-direct invocation path (e.g. test fixtures); production traffic
// goes through http_call.go's dispatchHttpCallResponse path which holds
// dispatchMu itself.
//
// Returns nil err on cap-denied / not-exported (the dispatch is best-effort).
// The 5-tuple matches §5.3 C19 wire shape exactly.
func (sc *StreamContext) CallProxyOnHttpCallResponse(ctx context.Context, callID, numHeaders, bodySize, numTrailers uint32) error {
	if sc.closed.Load() {
		return errors.New("wasm: CallProxyOnHttpCallResponse on closed StreamContext")
	}
	rv := sc.rootVM
	if !rv.sandbox.IsAllowed(capProxyOnHttpCallResponse) {
		return nil
	}
	fn := rv.instance.ExportedFunction("proxy_on_http_call_response")
	if fn == nil {
		return nil
	}

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()
	rv.currentCtxID.Store(sc.streamCtxID)

	err := rv.runCallWithPanicWrapper(func() error {
		_, callErr := fn.Call(ctx,
			uint64(sc.streamCtxID),
			uint64(callID),
			uint64(numHeaders),
			uint64(bodySize),
			uint64(numTrailers),
		)
		return callErr
	})
	sc.noteTrapOnError(err)
	return err
}

// CallProxyOnDone invokes `proxy_on_done(streamCtxID)` → bool. Returning
// false defers finalize (host returns CONTINUE on the wire per SPEC §3.1).
// Gated by capProxyOnDone. When denied or when the guest does not export
// the callback, returns (true, nil) — "done" is the default.
func (sc *StreamContext) CallProxyOnDone(ctx context.Context) (bool, error) {
	if sc.closed.Load() {
		return true, errors.New("wasm: CallProxyOnDone on closed StreamContext")
	}
	rv := sc.rootVM
	if !rv.sandbox.IsAllowed(capProxyOnDone) {
		return true, nil
	}
	fn := rv.instance.ExportedFunction("proxy_on_done")
	if fn == nil {
		return true, nil
	}

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()
	rv.currentCtxID.Store(sc.streamCtxID)

	var done bool
	err := rv.runCallWithPanicWrapper(func() error {
		results, err := fn.Call(ctx, uint64(sc.streamCtxID))
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
	sc.noteTrapOnError(err)
	return done, err
}

// CallProxyOnLog invokes `proxy_on_log(streamCtxID)`. Gated by
// capProxyOnLog. When denied or when the guest does not export the
// callback, returns nil (no-op success).
func (sc *StreamContext) CallProxyOnLog(ctx context.Context) error {
	if sc.closed.Load() {
		return errors.New("wasm: CallProxyOnLog on closed StreamContext")
	}
	rv := sc.rootVM
	if !rv.sandbox.IsAllowed(capProxyOnLog) {
		return nil
	}
	fn := rv.instance.ExportedFunction("proxy_on_log")
	if fn == nil {
		return nil
	}

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()
	rv.currentCtxID.Store(sc.streamCtxID)

	err := rv.runCallWithPanicWrapper(func() error {
		_, callErr := fn.Call(ctx, uint64(sc.streamCtxID))
		return callErr
	})
	sc.noteTrapOnError(err)
	return err
}

// CallProxyOnDelete invokes `proxy_on_delete(streamCtxID)`. Gated by
// capProxyOnDelete. When denied or when the guest does not export the
// callback, returns nil (no-op success).
func (sc *StreamContext) CallProxyOnDelete(ctx context.Context) error {
	if sc.closed.Load() {
		return errors.New("wasm: CallProxyOnDelete on closed StreamContext")
	}
	rv := sc.rootVM
	if !rv.sandbox.IsAllowed(capProxyOnDelete) {
		return nil
	}
	fn := rv.instance.ExportedFunction("proxy_on_delete")
	if fn == nil {
		return nil
	}

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()
	rv.currentCtxID.Store(sc.streamCtxID)

	err := rv.runCallWithPanicWrapper(func() error {
		_, callErr := fn.Call(ctx, uint64(sc.streamCtxID))
		return callErr
	})
	sc.noteTrapOnError(err)
	return err
}

// noteTrapOnError records a guest dispatch trap on the StreamContext per the
// BUG-3 fix (phase-25.3 Task 11-fix). Any non-nil error from a CallProxyOnX
// guest dispatch (a wazero RuntimeError trap, or a panic-wrapped Go panic from
// a bridge callback) marks the shared instance POISONED so Close skips the
// teardown triplet rather than re-entering a poisoned guest (which cascades —
// see the `trapped` field doc). Setting the flag on ANY dispatch error is the
// robust, conservative choice: a spurious skip of teardown on a non-trap error
// is harmless (the *RootVM is torn down or reinstantiated regardless), whereas
// re-entering a genuinely poisoned guest is the cascade we must prevent.
func (sc *StreamContext) noteTrapOnError(err error) {
	if err != nil {
		sc.trapped.Store(true)
	}
}

// Close fires proxy_on_done + proxy_on_log + proxy_on_delete on the stream
// context + cancels any outstanding httpCalls dispatched from this stream
// context (cancel-at-destruction per AMEND-B3 — LANDED at Task 8).
// Idempotent — second Close returns nil with no side effects.
//
// The streamCtxID entry is removed from rootVM.streamCtxs at Close. Errors
// from the per-callback invocations are joined (first non-nil wins); the
// cleanup still proceeds on error so the stream context teardown completes.
//
// # Trap-poison guard (BUG-3 fix, phase-25.3 Task 11-fix)
//
// If a prior guest dispatch on this stream TRAPPED (sc.trapped is set — a
// wazero RuntimeError, e.g. a Rust panic! that left the proxy-wasm-rust-sdk
// dispatcher's RefCell borrowed), the shared instance is POISONED. Close MUST
// NOT re-enter it via the proxy_on_done/log/delete teardown triplet: a second
// dispatch into the poisoned guest cascades (panic_already_borrowed in the
// Rust SDK; a redundant re-trap + spurious failures-counter bump in our host).
// On a trapped instance Close SKIPS the three teardown callbacks but still
// performs the rest of teardown (flip closed, remove from the streamCtxs map,
// cancel outstanding httpCalls). The shared *RootVM is separately marked Failed
// (NoteReloadRuntimeError under FAIL_RELOAD) so the next request reinstantiates
// a FRESH instance — clearing the poison for future streams. Under non-
// FAIL_RELOAD policies (FAIL_CLOSED / FAIL_OPEN) the *RootVM is NOT
// reinstantiated, so the poisoned shared instance persists for the lifetime of
// the *RootVM (a known limitation — upstream Envoy under non-reload policies
// likewise does not recover a trapped VM); the Close-skip nonetheless prevents
// the IMMEDIATE per-stream cascade on this stream's teardown.
func (sc *StreamContext) Close(ctx context.Context) error {
	var firstErr error
	sc.closeOnce.Do(func() {
		// Fire the three lifecycle teardown callbacks — UNLESS the instance is
		// poisoned by a prior trap (sc.trapped), in which case re-entering it
		// would cascade. Each is gated + guest-export-tolerant per the per-
		// callback method's contract; we invoke through the public methods to
		// retain the dispatch-lock + panic-wrapper discipline.
		if !sc.trapped.Load() {
			if _, err := sc.CallProxyOnDone(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
			if err := sc.CallProxyOnLog(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
			if err := sc.CallProxyOnDelete(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}

		// Flip closed AFTER firing the three callbacks (they early-return
		// on sc.closed.Load()). atomic.Bool removes the need to hold
		// dispatchMu for the transition; concurrent CallProxyOnX racers
		// observe the Store via acquire/release ordering on atomic.Bool.
		sc.closed.Store(true)

		// Remove from the per-RootVM streamCtxs map. Idempotent if the
		// map was already cleared by rootVM.Close.
		sc.rootVM.streamCtxsMu.Lock()
		if sc.rootVM.streamCtxs != nil {
			delete(sc.rootVM.streamCtxs, sc.streamCtxID)
		}
		sc.rootVM.streamCtxsMu.Unlock()

		// Cancel-at-destruction at Task 8 per AMEND-B3 + cpp-host
		// context.cc:1900-1905. Walks rootVM.httpCalls + filters entries
		// whose originating streamCtxID matches sc.streamCtxID + invokes
		// the per-call context.CancelFunc for each. The dispatch goroutines
		// observe ctx-cancellation + their handleHttpCallResponse path
		// early-returns via the token-miss guard (the entry has already
		// been removed). cancelOutstandingHttpCalls is defined at
		// http_call.go.
		sc.rootVM.cancelOutstandingHttpCalls(sc.streamCtxID)
	})
	return firstErr
}
