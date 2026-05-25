package wasm

// encode_headers.go — EncodeHeaders dispatcher per 25.1 SPEC §4.3 +
// OnDestroy lifecycle per parent SPEC §3.5.
//
// Mirrors DecodeHeaders for the encode side; RE-USES the per-stream
// *wasm.VM allocated at DecodeHeaders (the *filter holds vm; encode just
// dispatches proxy_on_response_headers + handles the captured-local-
// response / ProxyAction return shapes identically).
//
// At 25.1 there is NO encode-side VM construction path: if DecodeHeaders
// bailed at the nil-cfg pass-through OR at the initVM error fail-OPEN,
// f.vm is nil and EncodeHeaders short-circuits to Continue. Matches
// upstream wasm's encode-side null-vm parity.
//
// # OnDestroy lifecycle (per 25.1 SPEC §4.3 step list + parent §3.5)
//
// OnDestroy is the per-stream finalize callback per ADR-0071. The shape is:
//
//   1. CallProxyOnDone(streamContextID): guest's chance to signal it is
//      done (returning false would defer finalize per SPEC §3.1; at 25.1
//      we don't honor the defer — proceed to teardown unconditionally
//      because the framework's per-stream OnDestroy IS the teardown).
//   2. CallProxyOnLog(streamContextID): guest's last per-stream log point.
//   3. CallProxyOnDelete(streamContextID): context teardown on the guest.
//   4. vm.Close(): releases the wazero.Runtime.
//   5. Decrement the Group-B active gauge.
//
// All errors during teardown are logged + counted (envoy_go.failures.Inc())
// but do not abort the teardown — the runtime MUST be released even if a
// guest callback errors. Idempotent — second OnDestroy against a nil vm
// is a no-op.

import (
	"context"
	gohttp "net/http"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// EncodeHeaders implements envoyhttp.StreamEncoderFilter per 25.1 SPEC §4.3.
// Symmetric to DecodeHeaders for the encode side; reuses the per-stream
// *wasm.VM constructed at DecodeHeaders entry.
func (f *filter) EncodeHeaders(headers gohttp.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Defensive nil-cfg / nil-vm short-circuit. nil-vm fires when
	// DecodeHeaders bailed at the nil-cfg pass-through OR at the initVM
	// error fail-OPEN; in either case the encode side has no per-stream
	// VM to dispatch + must pass through. Matches upstream wasm's
	// encode-side null-vm parity.
	if f.cfg == nil || f.vm == nil {
		return envoyhttp.Continue
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
	action, err := f.vm.CallProxyOnResponseHeaders(ctx, f.streamContextID, numHeaders, endStream)
	if err != nil {
		if f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnResponseHeaders(stream=%d): %v",
			f.streamContextID, err)
		return envoyhttp.Continue
	}

	// REUSE 5 (parent §3.3) — captured-local-response on the encode side.
	// EncoderFilterCallbacks does NOT expose SendLocalReply (only the
	// decode side does, per the upstream contract: local replies entering
	// from the encode side would loop back through the encode chain).
	// At 25.1 we consume the capture + log a warning; the StopIteration
	// return prevents the response from continuing through the chain so
	// the guest's intent (do not emit this response) is honored even if
	// the SendLocalReply itself is not.
	if f.sentLocalResponse != nil {
		captured := f.sentLocalResponse
		f.sentLocalResponse = nil // consume; idempotent against double-dispatch
		logf("WARN wasm: proxy_send_local_response from proxy_on_response_headers (stream=%d, status=%d) — EncoderFilterCallbacks does not expose SendLocalReply at 25.1; stopping iteration",
			f.streamContextID, captured.statusCode)
		return envoyhttp.StopIteration
	}

	// ProxyAction handling — identical to the decode side.
	switch action {
	case abi.ProxyActionPause:
		logf("INFO wasm: proxy_on_response_headers returned PAUSE without captured local response — stream-control deferred to 25.2 (parent §1 architectural primitive 6); continuing")
		return envoyhttp.Continue
	case abi.ProxyActionContinue:
		fallthrough
	default:
		return envoyhttp.Continue
	}
}

// EncodeData is a no-op pass-through at 25.1 per parent SPEC §3.5 +
// 25.1 SPEC §4.3 (headers-bridge subset only). Body bridge lands at 25.2.
func (f *filter) EncodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// EncodeTrailers is a no-op pass-through at 25.1 per parent SPEC §3.5 +
// 25.1 SPEC §4.3 (headers-bridge subset only). Trailers bridge lands at 25.2.
func (f *filter) EncodeTrailers(_ gohttp.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// SetEncoderCallbacks stores the framework-supplied per-stream callback
// reference for the encode-side per ADR-0196 D-P3 first co-consumer — the
// *abiCallbacks.GetStatus reads f.encoderCb.ResponseStatus() to satisfy
// the guest's proxy_get_status hostcall (Task 11).
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.encoderCb = cb }

// OnDestroy is the per-stream finalize callback per ADR-0071 + 25.1 SPEC §4.3.
// Idempotent — second OnDestroy is a no-op against a nil vm. See the file-
// header comment for the full step list + error-handling disposition.
func (f *filter) OnDestroy() {
	if f.vm == nil {
		return
	}

	ctx := context.Background()

	// Step 1: CallProxyOnDone — guest signals done. Defer-finalize is
	// not honored at 25.1 (proceed to teardown regardless).
	if _, err := f.vm.CallProxyOnDone(ctx, f.streamContextID); err != nil {
		if f.cfg != nil && f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnDone(stream=%d): %v", f.streamContextID, err)
		// Continue teardown — the runtime MUST be released.
	}

	// Step 2: CallProxyOnLog — guest's last per-stream log point.
	if err := f.vm.CallProxyOnLog(ctx, f.streamContextID); err != nil {
		if f.cfg != nil && f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnLog(stream=%d): %v", f.streamContextID, err)
	}

	// Step 3: CallProxyOnDelete — context teardown on the guest.
	if err := f.vm.CallProxyOnDelete(ctx, f.streamContextID); err != nil {
		if f.cfg != nil && f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnDelete(stream=%d): %v", f.streamContextID, err)
	}

	// Step 4: vm.Close — releases the wazero.Runtime. Idempotent at the
	// vm.Close level (second Close returns nil with no side effects).
	if err := f.vm.Close(); err != nil {
		if f.cfg != nil && f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: vm.Close(stream=%d): %v", f.streamContextID, err)
	}
	f.vm = nil

	// Step 5: Group-B active gauge decrement per AMEND-A2.
	if f.cfg != nil && f.cfg.stats != nil && f.cfg.stats.active != nil {
		f.cfg.stats.active.Dec()
	}
}
