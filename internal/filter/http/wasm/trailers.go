package wasm

// trailers.go — DecodeTrailers + EncodeTrailers dispatchers per 25.2 SPEC
// §4.3.
//
// Lands the per-stream trailer-dispatch glue per the SPEC §4.3 step list:
//
//   1. Capture trailers on the per-stream *filter so the *abiCallbacks
//      back-pointer can route guest header-map hostcalls to the trailer
//      maps. (WasmHeaderMapType values 1 = HttpRequestTrailers + 3 =
//      HttpResponseTrailers are ACTIVATED at Task 16; Task 15 abi_callbacks
//      returns NotFound/Unimplemented for these types pending the
//      activation; the activation is implicit in the trailer capture below
//      + a follow-up edit to abi_callbacks.headerMapForType lands at Task
//      16 alongside the trailer capture.)
//
//   2. proxy_on_*_trailers dispatch per §5.3 C16 + C17: if the guest exports
//      proxy_on_request_trailers (decode side) OR proxy_on_response_trailers
//      (encode side), invoke streamCtx.CallProxyOnRequestTrailers(ctx,
//      uint32(numTrailers)) — the gate-at-getFunction discipline per
//      AMEND-B5 means cap-denied / not-exported guests get a no-op
//      pass-through. ProxyAction handling:
//
//        - Continue → TrailersContinue
//        - Pause    → filter.beginDecodePause/beginEncodePause +
//                     TrailersStopIteration. ⚠️ Phase-83 S2: this status
//                     PARKS the chain. Before S2 this arm set no flag and
//                     armed no watchdog, so the park was PERMANENTLY
//                     UNRESUMABLE — the guest's own proxy_continue_stream lost
//                     resumeDecode's CAS and fired nothing while still
//                     returning WasmResult::Ok. The PLAIN begin form, not the
//                     `Once` form: a trailers callback fires at most once per
//                     stream. See the arms themselves and pause.go.
//        - err      → cfg.stats.envoyGoFailures++ + log + TrailersContinue
//                     (fail-OPEN per ADR-0072 per-stream posture).
//
// # numTrailers semantic
//
// Per §5.3 C16 + C17: the wire arg `num_trailers` is the TOTAL value count
// (multi-value trailers expand to one entry per value, matching the §5.3
// pair-emission shape used by GetHeaderMap for the trailer side). This
// matches numHeaderValues for the headers side (decode_headers.go line 281).
//
// # Cross-references
//
//   - 25.2 SPEC §4.3 (per-stream dispatch shape — trailer-bridge section)
//   - 25.2 SPEC §5.3 C16 + C17 (proxy_on_request_trailers + proxy_on_
//     response_trailers callback contracts)
//   - AMEND-B5 (gate-at-getFunction for guest-export callbacks)
//   - ADR-0072 (per-stream fail-OPEN runtime posture)
//   - ADR-0205 (root-VM lifecycle evolution — streamCtx field)

import (
	"context"
	gohttp "net/http"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// proxyOnRequestTrailers is the byte-stable guest-export function name probed
// via streamCtx.HasGlobalFunc to determine whether the guest opted into the
// request-trailers callback per AMEND-B5.
const proxyOnRequestTrailers = "proxy_on_request_trailers"

// proxyOnResponseTrailers is the byte-stable guest-export function name for
// the encode-side trailers callback.
const proxyOnResponseTrailers = "proxy_on_response_trailers"

// requestTrailers + responseTrailers per-stream capture fields used by the
// trailer-side header-map hostcall dispatch. Captured at the entry to
// DecodeTrailers / EncodeTrailers below; the abi_callbacks.headerMapForType
// routes WasmHeaderMapType=1 (HttpRequestTrailers) + =3 (HttpResponseTrailers)
// to these fields. The fields are added directly on *filter (rather than
// using the existing requestHeaders / responseHeaders) so the trailer side
// has its own independent map — mutations by the guest on the trailers map
// MUST NOT bleed into the headers map (different proxy-wasm WasmHeaderMapType
// values; different envoy filter callback semantics).
//
// Per ADR-0071 single-goroutine-per-stream invariant: no synchronization
// needed; the same goroutine writes here in DecodeTrailers / EncodeTrailers
// and reads via the abi_callbacks back-pointer during the CallProxyOn*
// Trailers dispatch.

// DecodeTrailers implements envoyhttp.StreamDecoderFilter per 25.2 SPEC §4.3
// + §5.3 C16. See the file-header for the full step list + error-handling
// disposition.
//
// REPLACES the 25.1 no-op DecodeTrailers in decode_headers.go (the 25.1 stub
// returned TrailersContinue unconditionally; this Task 16 body lands the
// real dispatch).
func (f *filter) DecodeTrailers(trailers gohttp.Header) envoyhttp.FilterTrailersStatus {
	if f.cfg == nil {
		return envoyhttp.TrailersContinue
	}

	// Capture trailers on the per-stream *filter. The capture surfaces the
	// trailer-side header-map dispatch at WasmHeaderMapType=1 (Http Request
	// Trailers) which is read via the abi_callbacks back-pointer; at Task
	// 15 the abi_callbacks.headerMapForType returns (nil, false) for type 1
	// — the activation lands alongside this trailer capture at Task 16 +
	// the dispatch surface goes live.
	//
	// Note: the trailer maps live on a separate set of fields from the
	// headers maps (added at Task 18 alongside the per-stream context
	// construction); at Task 16 we capture into requestHeaders to support
	// the 25.2-active trailer dispatch — the abi_callbacks routing
	// adjustment for trailer maps lands at Task 18.
	_ = trailers // captured into f.requestHeaders by the framework; trailer-side header-map dispatch via abi_callbacks lands at Task 18

	if f.streamCtx == nil || !f.streamCtx.HasGlobalFunc(proxyOnRequestTrailers) {
		return envoyhttp.TrailersContinue
	}

	// Per-route effective config (override > listener default) — the
	// failure counter belongs to the per-route plugin's stat scope when an
	// override is active. Mirrors encode_headers.go.
	eff := f.eff
	if eff == nil {
		eff = f.cfg
	}

	ctx := context.Background()
	numTrailers := numHeaderValues(trailers)
	action, err := f.streamCtx.CallProxyOnRequestTrailers(ctx, numTrailers)
	if err != nil {
		if eff.stats != nil && eff.stats.envoyGoFailures != nil {
			eff.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnRequestTrailers(stream=%d, numTrailers=%d): %v",
			f.streamContextID, numTrailers, err)
		return envoyhttp.TrailersContinue
	}

	// REUSE 5 (parent §3.3): captured-local-response short-circuit. The guest
	// may have called proxy_send_local_response inside proxy_on_request_
	// trailers; consume + dispatch via decoderCb.SendLocalReply +
	// TrailersStopIteration.
	if f.sentLocalResponse != nil {
		captured := f.sentLocalResponse
		f.sentLocalResponse = nil
		// Phase-83 S8, same asymmetry as body.go's decode arms: with a
		// decoderCb, beginLocalReply sets localReplyDone synchronously and
		// RunDecodeTrailers re-checks it BEFORE the status switch, so
		// TrailersStopIteration is never read. With a nil one nothing sets the
		// flag and the status parks with no resumer, so that branch terminates.
		if f.decoderCb == nil {
			logf("WARN wasm: captured local response from proxy_on_request_trailers (stream=%d, status=%d) but decoderCb is nil — terminating the stream",
				f.streamContextID, captured.statusCode)
			return envoyhttp.TrailersTerminateStream
		}
		addl := convertHeaderPairsToOrderedHeaders(captured.additionalHeaders)
		f.decoderCb.SendLocalReply(int(captured.statusCode), captured.body, addl)
		return envoyhttp.TrailersStopIteration
	}

	// Phase-83 S2. TrailersStopIteration PARKS: FilterChain.RunDecodeTrailers
	// blocks in parkDecode on this status. Before this commit the arm set no
	// flag and armed no watchdog, which made the park PERMANENTLY UNRESUMABLE
	// — not merely long:
	//
	//   - the guest's own proxy_continue_stream could not release it. With
	//     decodePaused false, resumeDecode's CompareAndSwap FAILS and fires no
	//     ContinueDecoding at all, while ContinueStream still returns
	//     WasmResult::Ok — so the guest cannot even observe that its resume
	//     was dropped.
	//   - nothing outside the filter bounded it either. There is NO stream
	//     idle timeout anywhere in this tree (listener/manager.go records
	//     idle_timeout as an explicit deferral), and the connection ctx handed
	//     to parkDecode is never canceled on downstream disconnect.
	//
	// MEASURED, not read: with no bookkeeping on this arm, RunDecodeTrailers
	// did not return in 10 s (trailers_test.go's decode anchor), and the
	// encode mirror behaved identically through parkEncode.
	//
	// The PLAIN begin form, not beginDecodePauseOnce: a trailers callback
	// fires at most ONCE per stream, so there is no per-chunk re-arming
	// hazard, and the plain form is the SAFER of the two here — if a prior
	// body chunk left decodePaused set with no timer armed (the decoderCb ==
	// nil shape), Once would skip arming and leave this park unbounded.
	//
	// ⚠️ THIS MAKES THE PARK BOUNDED, NOT USEFUL. A guest paused from a
	// trailers callback is BLIND: abi_callbacks.headerMapForType routes
	// WasmHeaderMapType 1/3 to its `default: return nil, false, false` arm,
	// and this function never captures the trailers at all (see `_ = trailers`
	// above). Both are DEFERRED BY NAME in the phase-83 PLAN §11 — a future
	// row must price TWO seams, and fixing the routing alone would still hand
	// the guest an empty map. Bounding the leak is worth landing regardless.
	switch action {
	case abi.ProxyActionPause:
		f.beginDecodePause(proxyOnRequestTrailers)
		return envoyhttp.TrailersStopIteration
	case abi.ProxyActionContinue:
		fallthrough
	default:
		return envoyhttp.TrailersContinue
	}
}

// EncodeTrailers implements envoyhttp.StreamEncoderFilter per 25.2 SPEC §4.3
// + §5.3 C17. Mirror of DecodeTrailers for the encode side.
//
// SendLocalReply unavailable on the encode side. If the guest invokes
// proxy_send_local_response from proxy_on_response_trailers the capture is
// consumed + a warning logs + envoyhttp.TrailersTerminateStream returns.
//
// ⚠️ THIS SENTENCE WAS FALSE THREE WAYS AND ALL THREE ARE FIXED IN PLACE BY
// THE COMMITS THAT FALSIFIED THEM. It said TrailersStopIteration "returns so
// the chain terminates the response early": (1) the status is now
// TrailersTerminateStream, changed by phase-83 S8 at the arm below; (2) the
// old status did not terminate anything — RunEncodeTrailers PARKS on it and
// carries zero localReplyDone re-checks, so the "early termination" was a
// measured indefinite park, which is exactly why S8 changed it; and (3) it
// cited ADR-0071 for a claim ADR-0071 does not make — a bounded extraction of
// ADR-0071 ("HTTP filter iteration protocol shape", a 48-line span) contains
// ZERO mentions of SendLocalReply, and the ADR that DOES own the subject,
// ADR-0075 ("sendLocalReply encode-chain semantics"), says the opposite in its
// §Context: a filter may call it "from any callback (decode or encode side)".
// The real reason is not doctrinal but structural: this tree's
// http.EncoderFilterCallbacks does not declare the method (it is declared on
// http.DecoderFilterCallbacks only, internal/filter/http/callbacks.go).
//
// REPLACES the 25.1 no-op EncodeTrailers in encode_headers.go.
func (f *filter) EncodeTrailers(trailers gohttp.Header) envoyhttp.FilterTrailersStatus {
	if f.cfg == nil {
		return envoyhttp.TrailersContinue
	}

	_ = trailers // see DecodeTrailers note re: trailer-side header-map dispatch (lands at Task 18)

	if f.streamCtx == nil || !f.streamCtx.HasGlobalFunc(proxyOnResponseTrailers) {
		return envoyhttp.TrailersContinue
	}

	// Per-route effective config (override > listener default) — see the
	// DecodeTrailers mirror above.
	eff := f.eff
	if eff == nil {
		eff = f.cfg
	}

	ctx := context.Background()
	numTrailers := numHeaderValues(trailers)
	action, err := f.streamCtx.CallProxyOnResponseTrailers(ctx, numTrailers)
	if err != nil {
		if eff.stats != nil && eff.stats.envoyGoFailures != nil {
			eff.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnResponseTrailers(stream=%d, numTrailers=%d): %v",
			f.streamContextID, numTrailers, err)
		return envoyhttp.TrailersContinue
	}

	// ⚠️ PHASE-83 S8, AND A PARK THE ROW'S PLAN DOES NOT NAME — found at Task 4,
	// confirmed by execution at Task 6. RunEncodeTrailers (chain.go) parks on
	// TrailersStopIteration and carries ZERO localReplyDone re-checks, this arm
	// armed no watchdog, and EncoderFilterCallbacks exposes no SendLocalReply
	// for the flag to be set through — so the park had no resumer of any kind.
	// MEASURED (encodecap_park_test.go, cell
	// TestFilterChain_EncodeLocalResponse_TerminatesStream/trailers):
	// RunEncodeTrailers did not return inside the probe budget and only the
	// probe's ctx-cancel reaped the dispatch goroutine.
	//
	// LATENT, not live: Run*Trailers is dead production code at this tip —
	// every non-test hit of RunDecodeTrailers / RunEncodeTrailers in the tree
	// is a comment or the definition. Closed anyway.
	if f.sentLocalResponse != nil {
		captured := f.sentLocalResponse
		f.sentLocalResponse = nil
		logf("WARN wasm: proxy_send_local_response from proxy_on_response_trailers (stream=%d, status=%d) — EncoderFilterCallbacks does not expose SendLocalReply at 25.2; terminating the stream",
			f.streamContextID, captured.statusCode)
		return envoyhttp.TrailersTerminateStream
	}

	// Phase-83 S2, encode mirror. See the DecodeTrailers arm above for the
	// full rationale.
	//
	// The encode side is the harsher of the two: all SIX of chain.go's
	// in-iterator c.localReplyDone.Load() re-checks sit in RunDecodeHeaders /
	// RunDecodeData / RunDecodeTrailers (the seventh occurrence is the
	// exported LocalReplyDone accessor). RunEncodeTrailers carries ZERO, so an
	// encode-side trailers park has no secondary escape of any kind — the
	// watchdog is the only bound that exists.
	switch action {
	case abi.ProxyActionPause:
		f.beginEncodePause(proxyOnResponseTrailers)
		return envoyhttp.TrailersStopIteration
	case abi.ProxyActionContinue:
		fallthrough
	default:
		return envoyhttp.TrailersContinue
	}
}
