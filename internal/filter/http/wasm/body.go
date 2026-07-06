package wasm

// body.go — DecodeData + EncodeData dispatchers per 25.2 SPEC §4.3 + Q1 + Q2.
//
// Lands the per-stream body-buffer accumulator + cap-enforcement + 413-on-
// exceed + proxy_on_*_body callback dispatch per the SPEC §4.3 step list:
//
//   1. Append each body chunk to the per-stream accumulator (f.decodeBody on
//      the decode side; f.encodeBody on the encode side). Per Q1: bodySize
//      passed to proxy_on_*_body is the ACCUMULATED total, NOT the just-new-
//      chunk delta. The guest reads via proxy_get_buffer_bytes(HttpRequest
//      Body | HttpResponseBody, start, max_size) which dispatches through
//      abi_callbacks.GetBuffer to the accumulator.
//
//   2. Cap enforcement per Q2: if len(accumulator) > cfg.bodyBufferCapBytes
//      AND the sticky cap-exceeded flag is not yet set:
//      - Set the sticky flag (decodeBodyCapExceeded / encodeBodyCapExceeded)
//        to true.
//      - Increment cfg.stats.bodyBufferCapExceeded (envoy-go-strict counter).
//      - Co-increment cfg.stats.envoyGoFailures per 25.2 §2.25 (generic-
//        failures counter aggregates every envoy-go-strict stream-fail
//        surface).
//      - On the decode side: decoderCb.SendLocalReply(413, "Payload Too
//        Large", ...) per REUSE 5 (parent §3.3).
//      - On the encode side: SendLocalReply is unavailable (EncoderFilter
//        Callbacks does not expose it); log a warning + return DataStop
//        IterationNoBuffer so the chain terminates the response early.
//      Return DataStopIterationNoBuffer.
//
//      Subsequent chunks on the SAME stream short-circuit to DataStop
//      IterationNoBuffer WITHOUT re-invoking SendLocalReply OR re-bumping the
//      counters (sticky flag).
//
//   3. proxy_on_*_body dispatch per Q1: if the guest exports proxy_on_request_
//      body (decode side) OR proxy_on_response_body (encode side), invoke
//      streamCtx.CallProxyOnRequestBody(ctx, uint32(len(accumulator)),
//      endStream) — the gate-at-getFunction discipline per AMEND-B5 means
//      cap-denied / not-exported guests get ProxyActionContinue + nil
//      transparently. ProxyAction handling:
//
//        - Continue → DataContinue
//        - Pause    → DataStopIterationAndBuffer
//        - err      → cfg.stats.envoyGoFailures++ + log + DataContinue
//                     (fail-OPEN per ADR-0072 per-stream posture).
//
// # NO-op disposition (guest not opted in to body callbacks)
//
// If the guest does NOT export proxy_on_*_body, the body chunks STILL
// accumulate (cap enforcement STILL fires + counter STILL increments) but no
// guest callback is dispatched + DataContinue returns. This matches upstream
// proxy-wasm v0.2.1: cap-enforcement is an HOST policy, not a guest policy;
// the guest does not opt out of cap-enforcement by declining body callbacks.
//
// # Cross-references
//
//   - 25.2 SPEC §4.3 (per-stream dispatch shape — body bridge section)
//   - 25.2 SPEC §2.3 (Q2 buffer-cap discipline rationale)
//   - 25.2 SPEC §2.25 (envoy_go.failures co-increment discipline)
//   - 25.2 SPEC §5.3 C14 + C15 (proxy_on_request_body + proxy_on_response_
//     body callback contracts)
//   - 25.2 SPEC §7.1 (9 NEW envoy-go-strict counters — body_buffer_cap_
//     exceeded is counter #5; remaining 8 land at Task 17)
//   - Q1 (BRAINSTORM Q1 — per-chunk-invoke + accumulated body_size +
//     PAUSE-buffer dispatch)
//   - Q2 (BRAINSTORM Q2 — 16 MiB default cap + 413-on-exceed + sticky flag)
//   - AMEND-B5 (gate-at-getFunction for guest-export callbacks)
//   - ADR-0072 (boot-time-fail-fast; per-stream fail-OPEN runtime posture)
//   - ADR-0085 (nil-tolerance discipline)
//   - ADR-0205 (root-VM lifecycle evolution — streamCtx field replaces vm)

import (
	"context"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// proxyOnRequestBody is the byte-stable guest-export function name probed via
// streamCtx.HasGlobalFunc to determine whether the guest opted into the
// request-body callback per Q1 + AMEND-B5. Mirrors the upstream proxy-wasm
// v0.2.1 export name verbatim.
const proxyOnRequestBody = "proxy_on_request_body"

// proxyOnResponseBody is the byte-stable guest-export function name for the
// encode-side body callback.
const proxyOnResponseBody = "proxy_on_response_body"

// localReply413Body is the byte-stable response body for the 413 Payload Too
// Large local reply emitted when the body-buffer cap is exceeded on the
// decode side per Q2 + 25.2 SPEC §4.3.
const localReply413Body = "Payload Too Large"

// localReply413Status is the HTTP status code for the cap-exceeded local
// reply per Q2 (RFC 9110 §15.5.14 — 413 Content Too Large; envoy-go uses
// the legacy "Payload Too Large" body text for upstream-faithful wire match).
const localReply413Status = 413

// DecodeData implements envoyhttp.StreamDecoderFilter per 25.2 SPEC §4.3 +
// Q1 + Q2. See the file-header for the full step list + error-handling
// disposition.
//
// REPLACES the 25.1 no-op DecodeData in decode_headers.go (the 25.1 stub
// returned DataContinue unconditionally; this Task 16 body lands the real
// accumulator + cap-enforcement + dispatch).
func (f *filter) DecodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	// Defensive nil-cfg pass-through. Should-not-happen in production
	// (New factory always allocates cfg); preserved for test-double paths
	// that construct *filter{} directly.
	if f.cfg == nil {
		return envoyhttp.DataContinue
	}

	// Sticky cap-exceeded short-circuit per Q2. Once the cap is exceeded on
	// the decode side, subsequent chunks pass through StopAllIteration
	// without re-invoking SendLocalReply or re-bumping the counters. The
	// chain.go SendLocalReply path has already short-circuited iteration via
	// f.sentLocalResponse capture; this branch covers the case where the
	// framework still calls DecodeData with more chunks after the local
	// reply (defensive against chain reentry).
	if f.decodeBodyCapExceeded {
		return envoyhttp.DataStopIterationNoBuffer
	}

	// Step 1: append to per-stream accumulator. Per Q1: the accumulator
	// grows monotonically; the guest reads via proxy_get_buffer_bytes(
	// HttpRequestBody, ...) which dispatches through abi_callbacks.GetBuffer
	// to f.decodeBody.
	f.decodeBody = append(f.decodeBody, data...)

	// Step 2: Q2 cap enforcement. The cap is a uint32 ceiling on accumulator
	// length; if the accumulator grew past the cap on this chunk, fire the
	// sticky flag + counters + 413 SendLocalReply (decode-side only — encode
	// side does not have a SendLocalReply surface).
	if uint32(len(f.decodeBody)) > f.cfg.bodyBufferCapBytes {
		f.decodeBodyCapExceeded = true

		if f.cfg.stats != nil {
			if f.cfg.stats.bodyBufferCapExceeded != nil {
				f.cfg.stats.bodyBufferCapExceeded.Inc()
			}
			// 25.2 §2.25 co-increment: every envoy-go-strict surface that
			// fails a stream MUST also count against the generic failures
			// counter so operators see one number reflecting all wasm-driven
			// stream failures.
			if f.cfg.stats.envoyGoFailures != nil {
				f.cfg.stats.envoyGoFailures.Inc()
			}
		}

		// REUSE 5 (parent §3.3): SendLocalReply on the decode side. The
		// captured-local-response path used by SendLocalResponse hostcall
		// dispatch (Task 11) goes through f.sentLocalResponse + decode_
		// headers.go consume; here we invoke decoderCb.SendLocalReply
		// directly since the framework calls THIS file's DecodeData (not
		// the abi_callbacks consume path).
		if f.decoderCb != nil {
			f.decoderCb.SendLocalReply(localReply413Status, localReply413Body, nil)
		} else {
			logf("WARN wasm: body cap exceeded (cap=%d, accumulated=%d) but decoderCb is nil — cannot emit 413; stopping iteration",
				f.cfg.bodyBufferCapBytes, len(f.decodeBody))
		}
		return envoyhttp.DataStopIterationNoBuffer
	}

	// Step 3: proxy_on_request_body dispatch per Q1. Gate-at-getFunction per
	// AMEND-B5 — guests that did not opt into the body callback get a
	// no-op pass-through. The streamCtx may be nil in pre-Task-18 wiring
	// (test-double paths construct *filter without calling decode_headers.go
	// initVM); nil-streamCtx defensive short-circuits to DataContinue.
	if f.streamCtx == nil || !f.streamCtx.HasGlobalFunc(proxyOnRequestBody) {
		return envoyhttp.DataContinue
	}

	ctx := context.Background()
	// Per Q1: body_size is the ACCUMULATED total available, NOT the just-new-
	// chunk delta. The guest reads via proxy_get_buffer_bytes(HttpRequestBody,
	// start, max_size) against the accumulator.
	//
	//nolint:gosec // f.cfg.bodyBufferCapBytes is uint32 + we already enforced
	//             // len(f.decodeBody) <= cap above; uint32 conversion is safe.
	action, err := f.streamCtx.CallProxyOnRequestBody(ctx, uint32(len(f.decodeBody)), endStream)
	if err != nil {
		// Fail-OPEN per ADR-0072 per-stream runtime posture — the request
		// still serves, just without wasm body processing on this chunk.
		if f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnRequestBody(stream=%d, bodySize=%d, endStream=%v): %v",
			f.streamContextID, len(f.decodeBody), endStream, err)
		return envoyhttp.DataContinue
	}

	// REUSE 5 (parent §3.3): captured-local-response short-circuit. The guest
	// may have called proxy_send_local_response inside proxy_on_request_body;
	// the SendLocalResponse hostcall captured the payload on
	// f.sentLocalResponse. Consume + dispatch via decoderCb.SendLocalReply +
	// DataStopIterationNoBuffer (the chain stops iteration; the local reply
	// flows through the encode chain).
	if f.sentLocalResponse != nil {
		captured := f.sentLocalResponse
		f.sentLocalResponse = nil // consume; idempotent against double-dispatch
		if f.decoderCb != nil {
			addl := convertHeaderPairsToOrderedHeaders(captured.additionalHeaders)
			f.decoderCb.SendLocalReply(int(captured.statusCode), captured.body, addl)
		}
		return envoyhttp.DataStopIterationNoBuffer
	}

	// ProxyAction handling per Q1 + 25.2 SPEC §4.3:
	//   - Continue → DataContinue (chain proceeds; guest will receive the
	//                next chunk on the next DecodeData call)
	//   - Pause    → DataStopIterationAndBuffer (chain accumulates further
	//                chunks; guest's subsequent body-callback invocations
	//                see the additional bytes)
	switch action {
	case abi.ProxyActionPause:
		return envoyhttp.DataStopIterationAndBuffer
	case abi.ProxyActionContinue:
		fallthrough
	default:
		return envoyhttp.DataContinue
	}
}

// EncodeData implements envoyhttp.StreamEncoderFilter per 25.2 SPEC §4.3 +
// Q1 + Q2. Mirror of DecodeData for the encode side; reads + writes
// f.encodeBody + f.encodeBodyCapExceeded.
//
// Encode-side SendLocalReply unavailability per ADR-0071: EncoderFilter
// Callbacks does NOT expose SendLocalReply (the local reply would loop back
// through the encode chain). On cap-exceeded on the encode side we log a
// warning + return DataStopIterationNoBuffer so the chain terminates the
// response early; the operator-visible signal is the body_buffer_cap_
// exceeded counter + envoy_go.failures counter.
//
// REPLACES the 25.1 no-op EncodeData in encode_headers.go.
func (f *filter) EncodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	if f.cfg == nil {
		return envoyhttp.DataContinue
	}

	if f.encodeBodyCapExceeded {
		return envoyhttp.DataStopIterationNoBuffer
	}

	f.encodeBody = append(f.encodeBody, data...)

	if uint32(len(f.encodeBody)) > f.cfg.bodyBufferCapBytes {
		f.encodeBodyCapExceeded = true

		if f.cfg.stats != nil {
			if f.cfg.stats.bodyBufferCapExceeded != nil {
				f.cfg.stats.bodyBufferCapExceeded.Inc()
			}
			if f.cfg.stats.envoyGoFailures != nil {
				f.cfg.stats.envoyGoFailures.Inc()
			}
		}

		// SendLocalReply unavailable on the encode side per ADR-0071. Log
		// the cap-exceeded event so the operator sees a per-stream trace +
		// return StopAllIteration so the chain terminates the response early.
		logf("WARN wasm: encode-side body cap exceeded (cap=%d, accumulated=%d) — EncoderFilterCallbacks does not expose SendLocalReply; stopping iteration",
			f.cfg.bodyBufferCapBytes, len(f.encodeBody))
		return envoyhttp.DataStopIterationNoBuffer
	}

	if f.streamCtx == nil || !f.streamCtx.HasGlobalFunc(proxyOnResponseBody) {
		return envoyhttp.DataContinue
	}

	ctx := context.Background()
	//nolint:gosec // len(f.encodeBody) bounded by f.cfg.bodyBufferCapBytes uint32 ceiling above.
	action, err := f.streamCtx.CallProxyOnResponseBody(ctx, uint32(len(f.encodeBody)), endStream)
	if err != nil {
		if f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnResponseBody(stream=%d, bodySize=%d, endStream=%v): %v",
			f.streamContextID, len(f.encodeBody), endStream, err)
		return envoyhttp.DataContinue
	}

	// captured-local-response on the encode side: SendLocalReply unavailable;
	// consume + log + StopIteration matches the encode_headers.go REUSE-5
	// disposition for proxy_on_response_headers.
	if f.sentLocalResponse != nil {
		captured := f.sentLocalResponse
		f.sentLocalResponse = nil
		logf("WARN wasm: proxy_send_local_response from proxy_on_response_body (stream=%d, status=%d) — EncoderFilterCallbacks does not expose SendLocalReply at 25.2; stopping iteration",
			f.streamContextID, captured.statusCode)
		return envoyhttp.DataStopIterationNoBuffer
	}

	switch action {
	case abi.ProxyActionPause:
		return envoyhttp.DataStopIterationAndBuffer
	case abi.ProxyActionContinue:
		fallthrough
	default:
		return envoyhttp.DataContinue
	}
}
