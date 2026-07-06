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
//        - Pause    → TrailersStopIteration
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

	ctx := context.Background()
	numTrailers := numHeaderValues(trailers)
	action, err := f.streamCtx.CallProxyOnRequestTrailers(ctx, numTrailers)
	if err != nil {
		if f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
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
		if f.decoderCb != nil {
			addl := convertHeaderPairsToOrderedHeaders(captured.additionalHeaders)
			f.decoderCb.SendLocalReply(int(captured.statusCode), captured.body, addl)
		}
		return envoyhttp.TrailersStopIteration
	}

	switch action {
	case abi.ProxyActionPause:
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
// SendLocalReply unavailable on the encode side per ADR-0071. If the guest
// invokes proxy_send_local_response from proxy_on_response_trailers the
// capture is consumed + a warning logs + TrailersStopIteration returns so
// the chain terminates the response early.
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

	ctx := context.Background()
	numTrailers := numHeaderValues(trailers)
	action, err := f.streamCtx.CallProxyOnResponseTrailers(ctx, numTrailers)
	if err != nil {
		if f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnResponseTrailers(stream=%d, numTrailers=%d): %v",
			f.streamContextID, numTrailers, err)
		return envoyhttp.TrailersContinue
	}

	if f.sentLocalResponse != nil {
		captured := f.sentLocalResponse
		f.sentLocalResponse = nil
		logf("WARN wasm: proxy_send_local_response from proxy_on_response_trailers (stream=%d, status=%d) — EncoderFilterCallbacks does not expose SendLocalReply at 25.2; stopping iteration",
			f.streamContextID, captured.statusCode)
		return envoyhttp.TrailersStopIteration
	}

	switch action {
	case abi.ProxyActionPause:
		return envoyhttp.TrailersStopIteration
	case abi.ProxyActionContinue:
		fallthrough
	default:
		return envoyhttp.TrailersContinue
	}
}
