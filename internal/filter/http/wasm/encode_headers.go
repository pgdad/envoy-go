package wasm

// encode_headers.go — EncodeHeaders dispatcher per 25.2 SPEC §4.3 +
// OnDestroy lifecycle per parent SPEC §3.5 (REVISED at Task 18 for root-VM
// model).
//
// Mirrors DecodeHeaders for the encode side; RE-USES the per-stream
// *wasm.StreamContext allocated at DecodeHeaders (the *filter holds streamCtx;
// encode just dispatches proxy_on_response_headers + handles the captured-
// local-response / ProxyAction return shapes identically).
//
// At 25.2 there is NO encode-side StreamContext construction path: if
// DecodeHeaders bailed at the nil-cfg pass-through OR at the initStreamContext
// error fail-OPEN, f.streamCtx is nil and EncodeHeaders short-circuits to
// Continue. Matches upstream wasm's encode-side null-vm parity.
//
// # OnDestroy lifecycle (per 25.2 SPEC §4.3 step list + parent §3.5 +
// # AMEND-B3 cancel-at-destruction)
//
// OnDestroy is the per-stream finalize callback per ADR-0071. The shape is:
//
//   1. streamCtx.Close(ctx) — fires the 3 lifecycle teardown callbacks
//      (proxy_on_done → proxy_on_log → proxy_on_delete) + cancels any
//      outstanding httpCalls dispatched from this stream context
//      (cancel-at-destruction per AMEND-B3 + R-25.2-3; the cancel walk
//      lives at internal/wasm/stream_context.go Close).
//   2. cfg.rootCB.unregister(streamContextID) — removes the per-stream
//      *abiCallbacks from the per-RootVM multiplexer so stray late
//      hostcalls route to the no-op fallback rather than the freed
//      per-stream state.
//   3. Decrement the Group-B active gauge.
//
// All errors during teardown are logged + counted (envoy_go.failures.Inc())
// but do not abort the teardown — the per-stream context MUST be released
// even if a guest callback errors. Idempotent — second OnDestroy against a
// nil streamCtx is a no-op (streamCtx.Close is itself idempotent via
// sync.Once).

import (
	"context"
	gohttp "net/http"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// EncodeHeaders implements envoyhttp.StreamEncoderFilter per 25.2 SPEC §4.3.
// Symmetric to DecodeHeaders for the encode side; reuses the per-stream
// *wasm.StreamContext constructed at DecodeHeaders entry.
func (f *filter) EncodeHeaders(headers gohttp.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Defensive nil-cfg / nil-streamCtx short-circuit. nil-streamCtx fires
	// when DecodeHeaders bailed at the nil-cfg pass-through OR at the
	// initStreamContext error fail-OPEN; in either case the encode side has
	// no per-stream context to dispatch + must pass through. Matches
	// upstream wasm's encode-side null-vm parity.
	if f.cfg == nil || f.streamCtx == nil {
		return envoyhttp.Continue
	}

	// Use the SAME effective config resolved at DecodeHeaders (per-route
	// override > listener default) per phase-25.3 Task 9 — the per-stream
	// StreamContext belongs to eff.rootVM. Fall back to f.cfg on the encode-
	// only edge (DecodeHeaders never ran; f.eff nil) — though that edge is
	// already short-circuited by the f.streamCtx==nil guard above.
	eff := f.eff
	if eff == nil {
		eff = f.cfg
	}

	// Capture response headers for the *abiCallbacks back-pointer per
	// ADR-0071 single-goroutine-per-stream invariant.
	f.responseHeaders = headers

	ctx := context.Background()

	// Per AMEND-A2 + parent SPEC §7 line 738 + §5.1 hostcall 1 commentary:
	// `wasm.<plugin>.executions` is allocated as the per-
	// `proxy_on_request_headers`-invocation counter ONLY — the encode-side
	// `proxy_on_response_headers` dispatch does NOT increment it (the
	// DecodeHeaders body holds the lone Inc site per the SPEC's lifecycle
	// step list line 787). This pin is exercised cross-side by
	// fixture-0034 scenario (e) StatsAsserter: one Drive per side issues
	// ONE GET per scenario; the executions counter post-Drive must equal
	// exactly 1 on the per-listener plugin (e.g. `wasm.plugin_e.executions`
	// post-l_test_e probe).

	numHeaders := numHeaderValues(headers)
	action, err := f.streamCtx.CallProxyOnResponseHeaders(ctx, numHeaders, endStream)
	if err != nil {
		if eff.stats != nil && eff.stats.envoyGoFailures != nil {
			eff.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnResponseHeaders(stream=%d): %v",
			f.streamContextID, err)
		return envoyhttp.Continue
	}

	// REUSE 5 (parent §3.3) — captured-local-response on the encode side.
	// EncoderFilterCallbacks does NOT expose SendLocalReply (only the
	// decode side does, per the upstream contract: local replies entering
	// from the encode side would loop back through the encode chain).
	// At 25.2 we consume the capture + log a warning; the StopIteration
	// return prevents the response from continuing through the chain so
	// the guest's intent (do not emit this response) is honored even if
	// the SendLocalReply itself is not.
	if f.sentLocalResponse != nil {
		captured := f.sentLocalResponse
		f.sentLocalResponse = nil // consume; idempotent against double-dispatch
		logf("WARN wasm: proxy_send_local_response from proxy_on_response_headers (stream=%d, status=%d) — EncoderFilterCallbacks does not expose SendLocalReply at 25.2; stopping iteration",
			f.streamContextID, captured.statusCode)
		return envoyhttp.StopIteration
	}

	// ProxyAction handling — identical to the decode side.
	switch action {
	case abi.ProxyActionPause:
		logf("INFO wasm: proxy_on_response_headers returned PAUSE without captured local response — stream-control deferred (parent §1 architectural primitive 6); continuing")
		return envoyhttp.Continue
	case abi.ProxyActionContinue:
		fallthrough
	default:
		return envoyhttp.Continue
	}
}

// EncodeData + EncodeTrailers landed at 25.2 IMPL Task 16 — see body.go +
// trailers.go for the per-stream body-buffer accumulator + cap-enforcement
// + trailer-dispatch shape per 25.2 SPEC §4.3 + Q1 + Q2.

// SetEncoderCallbacks stores the framework-supplied per-stream callback
// reference for the encode-side per ADR-0196 D-P3 first co-consumer — the
// *abiCallbacks.GetStatus reads f.encoderCb.ResponseStatus() to satisfy
// the guest's proxy_get_status hostcall (Task 11).
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.encoderCb = cb }

// OnDestroy is the per-stream finalize callback per ADR-0071 + 25.2 SPEC §4.3.
// Idempotent — second OnDestroy is a no-op against a nil streamCtx. See the
// file-header comment for the full step list + error-handling disposition.
//
// REVISED at Task 18 for the root-VM model:
//
//   - Replaces the 25.1 manual CallProxyOnDone + CallProxyOnLog +
//     CallProxyOnDelete + vm.Close sequence with a single streamCtx.Close
//     call which encapsulates all 4 lifecycle steps + cancel-at-destruction
//     per AMEND-B3.
//   - Unregisters the per-stream *abiCallbacks from the per-RootVM
//     multiplexer so stray hostcalls route to the no-op fallback.
//   - Group-B active gauge decrements at the same site.
func (f *filter) OnDestroy() {
	if f.streamCtx == nil {
		return
	}

	// Use the SAME effective config resolved at DecodeHeaders (the per-stream
	// StreamContext + its multiplexer registration belong to eff.rootVM /
	// eff.rootCB) per phase-25.3 Task 9. Fall back to f.cfg if DecodeHeaders
	// never ran (defensive; f.streamCtx would be nil on that edge anyway).
	eff := f.eff
	if eff == nil {
		eff = f.cfg
	}

	ctx := context.Background()

	// streamCtx.Close fires (in order, idempotent via sync.Once):
	//   - CallProxyOnDone (guest's chance to signal done; defer-finalize
	//     not honored at 25.2)
	//   - CallProxyOnLog (guest's last per-stream log point)
	//   - CallProxyOnDelete (context teardown on the guest)
	//   - cancelOutstandingHttpCalls(streamCtxID) per AMEND-B3 + R-25.2-3
	//   - removes the streamCtxID entry from rv.streamCtxs
	//
	// Errors are logged + counted but do not abort the teardown — the
	// per-stream context MUST be released even if a guest callback errors.
	if err := f.streamCtx.Close(ctx); err != nil {
		if eff != nil && eff.stats != nil && eff.stats.envoyGoFailures != nil {
			eff.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: streamCtx.Close(stream=%d): %v", f.streamContextID, err)
	}

	// Unregister from the per-RootVM rootABICallbacks multiplexer. After
	// this point, any stray hostcalls fired against streamContextID route
	// to the multiplexer's no-op fallback rather than the freed per-stream
	// *abiCallbacks / *filter state. Per AMEND-B3: the cancel-at-
	// destruction walk inside streamCtx.Close above cancels outstanding
	// httpCalls; any response that slipped through (the cancel race) is
	// counted via the http_call_response_after_close counter at the
	// http_call.go response-dispatch path.
	if eff != nil && eff.rootCB != nil {
		eff.rootCB.unregister(f.streamContextID)
	}

	f.streamCtx = nil

	// Group-B active gauge decrement per AMEND-A2.
	if eff != nil && eff.stats != nil && eff.stats.active != nil {
		eff.stats.active.Dec()
	}
}
