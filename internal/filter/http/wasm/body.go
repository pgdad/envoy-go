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
//        Large", ...) per REUSE 5 (parent §3.3) + return DataStopIteration
//        NoBuffer, whose value the chain never reads — beginLocalReply sets
//        localReplyDone synchronously and RunDecodeData re-checks it BEFORE
//        the status switch. When decoderCb is nil no reply is sent and
//        nothing sets the flag, so that branch returns DataTerminateStream.
//      - On the encode side: SendLocalReply is unavailable (EncoderFilter
//        Callbacks does not expose it); log a warning + return
//        DataTerminateStream, which aborts RunEncodeData with
//        errStreamTerminatedByFilter and closes the connection. Phase-83 S8:
//        the pre-83 DataStopIterationNoBuffer here PARKED the encode chain
//        forever, because no RunEncode* iterator carries a localReplyDone
//        re-check and no watchdog bounds a host-initiated park.
//
//      Subsequent chunks on the SAME stream short-circuit to
//      DataTerminateStream WITHOUT re-invoking SendLocalReply OR re-bumping
//      the counters (sticky flag).
//
//   3. proxy_on_*_body dispatch per Q1: if the guest exports proxy_on_request_
//      body (decode side) OR proxy_on_response_body (encode side), invoke
//      streamCtx.CallProxyOnRequestBody(ctx, uint32(len(accumulator)),
//      endStream) — the gate-at-getFunction discipline per AMEND-B5 means
//      cap-denied / not-exported guests get ProxyActionContinue + nil
//      transparently. ProxyAction handling:
//
//        - Continue → filter.disarmDecodePause/disarmEncodePause (this arm
//                     RELEASES the chain, so any pause an earlier chunk armed
//                     must be retired here) + DataContinue
//        - Pause    → filter.beginDecodePauseOnce/beginEncodePauseOnce +
//                     DataStopIterationAndBuffer. ⚠️ Phase-83 S1: this status
//                     PARKS the chain — it does NOT "buffer further chunks for
//                     a later invocation", because the goroutine it parks is
//                     the one that would deliver them. See the arm itself and
//                     pause.go for the measured leak the missing bookkeeping
//                     produced, and for why the `Once` form is required on a
//                     per-CHUNK callback.
//        - err      → cfg.stats.envoyGoFailures++ + log + disarm +
//                     DataContinue (fail-OPEN per ADR-0072 per-stream
//                     posture; the fail-OPEN arm releases the chain too).
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

	// Use the SAME effective config resolved at DecodeHeaders (per-route
	// override > listener default) per phase-25.3 Task 9 — the body cap +
	// the per-plugin stat scope belong to the per-route plugin when an
	// override is active. Fall back to f.cfg when DecodeHeaders never
	// resolved (test-double paths). Mirrors encode_headers.go.
	eff := f.eff
	if eff == nil {
		eff = f.cfg
	}

	// Sticky cap-exceeded short-circuit per Q2. Once the cap is exceeded on the
	// decode side, subsequent chunks short-circuit here without re-invoking
	// SendLocalReply or re-bumping the counters.
	//
	// Phase-83 S8: DataTerminateStream, not DataStopIterationNoBuffer. The
	// stream is already unservable and this arm has NO resume to offer, so a
	// parking status parks with no resumer: it sends no local reply, so
	// RunDecodeData's pre-switch localReplyDone re-check cannot rescue it
	// either. MEASURED, not read: with DataStopIterationNoBuffer here,
	// RunDecodeData did not return in the probe budget (see
	// encodecap_park_test.go, cell decode_sticky) and only the probe's own
	// ctx-cancel reaped the goroutine.
	//
	// Reachable only by chain reentry today (the normal decode path stops at
	// the localReplyDone re-check on the chunk that fired the cap), which makes
	// this LATENT rather than live. It is closed anyway — "unreachable at this
	// tip" is not a property to hang a held connection on.
	if f.decodeBodyCapExceeded {
		return envoyhttp.DataTerminateStream
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
	if uint32(len(f.decodeBody)) > eff.bodyBufferCapBytes {
		f.decodeBodyCapExceeded = true

		if eff.stats != nil {
			if eff.stats.bodyBufferCapExceeded != nil {
				eff.stats.bodyBufferCapExceeded.Inc()
			}
			// 25.2 §2.25 co-increment: every envoy-go-strict surface that
			// fails a stream MUST also count against the generic failures
			// counter so operators see one number reflecting all wasm-driven
			// stream failures.
			if eff.stats.envoyGoFailures != nil {
				eff.stats.envoyGoFailures.Inc()
			}
		}

		// REUSE 5 (parent §3.3): SendLocalReply on the decode side. The
		// captured-local-response path used by SendLocalResponse hostcall
		// dispatch (Task 11) goes through f.sentLocalResponse + decode_
		// headers.go consume; here we invoke decoderCb.SendLocalReply
		// directly since the framework calls THIS file's DecodeData (not
		// the abi_callbacks consume path).
		//
		// Phase-83 S8: the two branches return DIFFERENT statuses, and the
		// asymmetry is the point. With a decoderCb, beginLocalReply sets
		// localReplyDone SYNCHRONOUSLY and RunDecodeData re-checks it BEFORE
		// the status switch, so the returned status is never examined and the
		// production decode path is genuinely rescued. With a nil decoderCb no
		// reply is sent, nothing sets the flag, and DataStopIterationNoBuffer
		// parked forever — MEASURED (encodecap_park_test.go, cell
		// decode_cap_nil_cb). NewFilterChain always wires a real callback, so
		// the nil shape is test-double-reachable only; it is closed regardless.
		if f.decoderCb == nil {
			logf("WARN wasm: body cap exceeded (cap=%d, accumulated=%d) but decoderCb is nil — cannot emit 413; terminating the stream",
				eff.bodyBufferCapBytes, len(f.decodeBody))
			return envoyhttp.DataTerminateStream
		}
		f.decoderCb.SendLocalReply(localReply413Status, localReply413Body, nil)
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
	//nolint:gosec // eff.bodyBufferCapBytes is uint32 + we already enforced
	//             // len(f.decodeBody) <= cap above; uint32 conversion is safe.
	action, err := f.streamCtx.CallProxyOnRequestBody(ctx, uint32(len(f.decodeBody)), endStream)
	if err != nil {
		// Fail-OPEN per ADR-0072 per-stream runtime posture — the request
		// still serves, just without wasm body processing on this chunk.
		if eff.stats != nil && eff.stats.envoyGoFailures != nil {
			eff.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnRequestBody(stream=%d, bodySize=%d, endStream=%v): %v",
			f.streamContextID, len(f.decodeBody), endStream, err)
		// DataContinue RELEASES the chain, so this arm owes the same disarm
		// the Continue arm does — a pause left armed by an earlier chunk
		// would fire an unmatched ContinueDecoding into an unparked stream.
		f.disarmDecodePause()
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
		// Same asymmetry as the cap arm above: with a decoderCb the local reply
		// sets localReplyDone and the chain's pre-switch re-check ends decode
		// iteration before the status is read; with a nil one, nothing does.
		if f.decoderCb == nil {
			logf("WARN wasm: captured local response from proxy_on_request_body (stream=%d, status=%d) but decoderCb is nil — terminating the stream",
				f.streamContextID, captured.statusCode)
			return envoyhttp.DataTerminateStream
		}
		addl := convertHeaderPairsToOrderedHeaders(captured.additionalHeaders)
		f.decoderCb.SendLocalReply(int(captured.statusCode), captured.body, addl)
		return envoyhttp.DataStopIterationNoBuffer
	}

	// ProxyAction handling per Q1 + 25.2 SPEC §4.3:
	//
	//   - Continue → DataContinue. The chain advances to the next filter and
	//     this filter sees the next chunk on the next DecodeData call.
	//
	//   - Pause → DataStopIterationAndBuffer, WHICH PARKS. Phase-83 S1.
	//
	// ⚠️ THE PRE-83 COMMENT HERE WAS FALSE, AND ITS FALSEHOOD IS THE WHOLE
	// BUG. It read "chain accumulates further chunks; guest's subsequent
	// body-callback invocations see the additional bytes." What
	// FilterChain.RunDecodeData actually does with this status is append the
	// CURRENT chunk to c.decodeBuf and then BLOCK in parkDecode — and the
	// goroutine it blocks is the very one that would have delivered the next
	// chunk. There are no "further chunks" and no "subsequent invocations";
	// there is one parked dispatch goroutine and one held connection.
	// MEASURED, not read: with no bookkeeping on this arm, RunDecodeData did
	// not return in 10 s (pause_body_test.go's decode anchor) and the encode
	// mirror behaved identically through parkEncode.
	//
	// beginDecodePauseOnce records that THIS filter owes the chain exactly one
	// resume — without the flag the guest's own proxy_continue_stream loses
	// resumeDecode's CompareAndSwap and fires NOTHING while still returning
	// WasmResult::Ok — and arms the S9 watchdog that bounds the park. `Once`
	// rather than the plain form because a body callback runs per CHUNK: see
	// pause.go for why re-arming per chunk removes the bound entirely.
	switch action {
	case abi.ProxyActionPause:
		f.beginDecodePauseOnce(proxyOnRequestBody)
		return envoyhttp.DataStopIterationAndBuffer
	case abi.ProxyActionContinue:
		fallthrough
	default:
		f.disarmDecodePause()
		return envoyhttp.DataContinue
	}
}

// EncodeData implements envoyhttp.StreamEncoderFilter per 25.2 SPEC §4.3 +
// Q1 + Q2. Mirror of DecodeData for the encode side; reads + writes
// f.encodeBody + f.encodeBodyCapExceeded.
//
// Encode-side SendLocalReply unavailability: http.EncoderFilterCallbacks does
// NOT declare SendLocalReply (it is declared on http.DecoderFilterCallbacks
// only, internal/filter/http/callbacks.go). ⚠️ NOT "per ADR-0071" — that ADR
// mentions SendLocalReply zero times in its 48 lines, and ADR-0075 §Context
// says the opposite; see the full three-way correction at the cap arm below.
//
// On cap-exceeded on the encode side we log a warning + return
// envoyhttp.DataTerminateStream — ⚠️ NOT DataStopIterationNoBuffer "so the
// chain terminates the response early", which is what this comment said until
// phase-83 S8 and which was false in both halves: that status PARKED
// RunEncodeData rather than terminating anything, measurably forever. The
// operator-visible signal is the body_buffer_cap_exceeded counter +
// envoy_go.failures counter.
//
// REPLACES the 25.1 no-op EncodeData in encode_headers.go.
func (f *filter) EncodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	if f.cfg == nil {
		return envoyhttp.DataContinue
	}

	// Per-route effective config (override > listener default) — see the
	// DecodeData mirror above.
	eff := f.eff
	if eff == nil {
		eff = f.cfg
	}

	// Phase-83 S8, encode mirror of the decode sticky arm — and the LIVE one.
	// The encode side has no localReplyDone re-check anywhere, so a parking
	// status here parked forever. MEASURED: encodecap_park_test.go, cell
	// encode_sticky.
	if f.encodeBodyCapExceeded {
		return envoyhttp.DataTerminateStream
	}

	f.encodeBody = append(f.encodeBody, data...)

	if uint32(len(f.encodeBody)) > eff.bodyBufferCapBytes {
		f.encodeBodyCapExceeded = true

		if eff.stats != nil {
			if eff.stats.bodyBufferCapExceeded != nil {
				eff.stats.bodyBufferCapExceeded.Inc()
			}
			if eff.stats.envoyGoFailures != nil {
				eff.stats.envoyGoFailures.Inc()
			}
		}

		// ⚠️ PHASE-83 S8 — THE ARM THAT MOTIVATED DataTerminateStream. The
		// comment that stood here was false three ways and each falsehood is
		// corrected, not softened:
		//
		//  1. It named `StopAllIteration`, an identifier DECLARED NOWHERE in
		//     this repository. The real roster is DataContinue /
		//     DataStopIterationAndBuffer / DataStopIterationNoBuffer, plus
		//     DataTerminateStream as of this commit.
		//  2. It attributed "SendLocalReply is unavailable on the encode side"
		//     to ADR-0071. Bounded extraction of ADR-0071 (heading
		//     DECISIONS.md:2586, next `^## ADR-` at :2634, 48-line
		//     denominator) returns ZERO hits for SendLocalReply. The ADR that
		//     actually speaks to it is ADR-0075 §Context (:2733), and it says
		//     the OPPOSITE — SendLocalReply is called "from any callback
		//     (decode OR ENCODE side)". The real constraint is narrower and
		//     lives in this tree, not in an ADR: EncoderFilterCallbacks simply
		//     does not expose the method.
		//  3. It claimed the return "terminates the response early". It did
		//     not: DataStopIterationNoBuffer PARKS RunEncodeData in
		//     parkEncode, forever. MEASURED (encodecap_park_test.go, cell
		//     cap): the iterator did not return, no watchdog was armed, and
		//     only the probe's ctx-cancel reaped the goroutine.
		//
		// DataContinue is NOT the alternative and this too is measured, not
		// reasoned: flipping this arm to DataContinue yields terminated=true,
		// err=nil and no encode-body override, so HCM writes resp.Body IN FULL
		// and the cap is silently disabled.
		//
		// THE REFERENCE, MEASURED against pinned envoyproxy/envoy:
		// contrib-v1.37.2 (four arms, fresh container each, with an NC): an
		// encode-side buffer-limit breach against a paused encoder filter
		// delivers 200 + headers and then RESETS THE STREAM with ZERO body
		// bytes in 1.007 s, emitting http.<prefix>.rs_too_large: 1 and
		// downstream_rq_tx_reset: 1. The NC (same limit, guest Continue)
		// served 65536 bytes in 0.003 s with rs_too_large: 0. Upstream never
		// stalls on a cap breach; it also bounds any indefinite encode pause
		// separately, with stream_idle_timeout (arm 3b terminated at 4.005 s
		// with downstream_rq_idle_timeout: 1). Two mechanisms for two
		// failures; envoy-go had neither.
		//
		// ⚠️ DEPARTURE, RECORDED NOT PAPERED OVER: upstream flushes the
		// response HEADERS before resetting. envoy-go runs the whole encode
		// chain before writeH1Reply, so on this sentinel the client gets
		// NOTHING — connection close, no headers. Both are "no usable body +
		// a torn-down connection"; the byte shape differs.
		logf("WARN wasm: encode-side body cap exceeded (cap=%d, accumulated=%d) — EncoderFilterCallbacks does not expose SendLocalReply; terminating the stream",
			eff.bodyBufferCapBytes, len(f.encodeBody))
		return envoyhttp.DataTerminateStream
	}

	if f.streamCtx == nil || !f.streamCtx.HasGlobalFunc(proxyOnResponseBody) {
		return envoyhttp.DataContinue
	}

	ctx := context.Background()
	//nolint:gosec // len(f.encodeBody) bounded by eff.bodyBufferCapBytes uint32 ceiling above.
	action, err := f.streamCtx.CallProxyOnResponseBody(ctx, uint32(len(f.encodeBody)), endStream)
	if err != nil {
		if eff.stats != nil && eff.stats.envoyGoFailures != nil {
			eff.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnResponseBody(stream=%d, bodySize=%d, endStream=%v): %v",
			f.streamContextID, len(f.encodeBody), endStream, err)
		// Fail-OPEN releases the chain — same disarm obligation as Continue.
		f.disarmEncodePause()
		return envoyhttp.DataContinue
	}

	// Captured-local-response on the encode side: EncoderFilterCallbacks does
	// not expose SendLocalReply, so the guest's reply cannot be delivered and
	// the response it wanted suppressed must not go out either.
	//
	// Phase-83 S8: DataTerminateStream. The host has decided the stream is
	// unservable and owes no resume; the previous DataStopIterationNoBuffer
	// parked RunEncodeData forever — MEASURED (encodecap_park_test.go, cell
	// TestFilterChain_EncodeLocalResponse_TerminatesStream/body).
	if f.sentLocalResponse != nil {
		captured := f.sentLocalResponse
		f.sentLocalResponse = nil
		logf("WARN wasm: proxy_send_local_response from proxy_on_response_body (stream=%d, status=%d) — EncoderFilterCallbacks does not expose SendLocalReply at 25.2; terminating the stream",
			f.streamContextID, captured.statusCode)
		return envoyhttp.DataTerminateStream
	}

	// Phase-83 S1, encode mirror. See the DecodeData arm above for the full
	// rationale and for the false comment this replaces.
	//
	// The encode side is the harsher of the two: all SIX of chain.go's
	// in-iterator c.localReplyDone.Load() re-checks sit in RunDecodeHeaders /
	// RunDecodeData / RunDecodeTrailers (the seventh occurrence is the
	// exported LocalReplyDone accessor). The three RunEncode* iterators carry
	// ZERO, so an encode-side park has no secondary escape at all — the
	// watchdog is the only bound that exists.
	switch action {
	case abi.ProxyActionPause:
		f.beginEncodePauseOnce(proxyOnResponseBody)
		return envoyhttp.DataStopIterationAndBuffer
	case abi.ProxyActionContinue:
		fallthrough
	default:
		f.disarmEncodePause()
		return envoyhttp.DataContinue
	}
}
