package adaptive_concurrency

// filter.go — the per-stream adaptive_concurrency filter struct +
// StreamDecoderFilter + StreamEncoderFilter method declarations per
// phase-21 SPEC §6.3 + §6.4 + AMEND-6.
//
// # Filter struct
//
// The adaptive_concurrency filter is BOTH a decoder (DecodeHeaders →
// forwardingDecision() ⇒ Forward/Block per AMEND-6 wire shape) AND an
// encoder (EncodeHeaders → recordLatencySample + releaseInFlight per
// D11). The same *filter struct implements BOTH interfaces; the Task 9
// factory wires `HTTPFilter{Decoder: f, Encoder: f}` at filter
// construction.
//
// # Per-stream state
//
// The *filter holds two pieces of per-stream state:
//
//   - entryTime time.Time — set on Forward at DecodeHeaders via clock.Now();
//     consumed at Task 6 EncodeHeaders body to compute the RTT sample (rtt
//     = clock.Now() - entryTime).
//   - acquired bool — true between successful forwardingDecision() Forward
//     and the encoder-side release (or OnDestroy fallback per D11 token-
//     release-on-reset-before-encode). Discriminates the encode-side hook
//     between "this stream acquired a token; release it + record RTT" and
//     "this stream never acquired a token; no-op" (the latter case fires
//     when DecodeHeaders short-circuited via Block OR when the disabled
//     pass-through fired).
//
// # Shared *gradientController instance
//
// The *gradientController pointer is captured at the factory level
// (Task 9 wiring) — one controller per HCM filter chain mounting an
// adaptive_concurrency filter. Each per-stream *filter holds the same
// pointer; the controller's atomic numRqOutstanding + concurrencyLimit
// serialize concurrent forwarders per planner-time D17.
//
// # Encode-side body delegation
//
// EncodeHeaders delegates to recordRTTAndRelease at encode_complete.go (Task
// 6 landing): no-op when !f.acquired; otherwise compute rtt = clock.Now() -
// entryTime, call controller.recordLatencySample(rtt) +
// controller.releaseInFlight(), clear f.acquired. OnDestroy implements the
// D11 token-release symmetry inline (Task 6 landing): if f.acquired, release
// the token + clear the flag; otherwise no-op. The f.acquired boolean serves
// as the bidirectional idempotency flag — exactly one of {EncodeHeaders,
// OnDestroy} releases the token, regardless of which fires first.
//
// # Cross-references
//
//   - SPEC §6.3 (filter struct shape)
//   - SPEC §6.4 (DecodeHeaders dispatch + 503 wire shape)
//   - SPEC §6.5 (EncodeHeaders RTT recording — Task 6 landing)
//   - AMEND-6 (503-overflow byte-pinned wire shape — partial cross-side)
//   - ADR-0186 §Decision (Gradient-1 controller — Task 3 landing)
//   - planner-time D11 (OnDestroy token-release — Task 6 landing)
//   - planner-time D17 (atomic.Uint32 hot-path; lock-free CAS)
//   - rbac.go::filter precedent (decoder-only filter; same SetDecoderCallbacks
//     pattern); cors.go::filter precedent (both-sides filter; same Decoder
//     + Encoder method set on the same struct).

import (
	"net/http"
	"time"

	"github.com/pgdad/envoy-go/internal/clock"
	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// filter is the per-stream adaptive_concurrency filter instance per SPEC
// §6.3. Owns the per-request entryTime / acquired bookkeeping; shares the
// *gradientController pointer (one per HCM filter chain mounting an
// adaptive_concurrency filter) captured at the New() factory level (Task 9
// landing).
//
// Both-sides filter: implements StreamDecoderFilter (DecodeHeaders /
// DecodeData / DecodeTrailers / SetDecoderCallbacks / OnDestroy) AND
// StreamEncoderFilter (EncodeHeaders / EncodeData / EncodeTrailers /
// SetEncoderCallbacks / OnDestroy). Task 9 wires
// `HTTPFilter{Decoder: f, Encoder: f}` at the factory return; the chain
// dispatches per-side as appropriate.
type filter struct {
	cc         *compiledConfig
	controller *gradientController
	clock      clock.Clock
	dcb        envoyhttp.DecoderFilterCallbacks

	// entryTime is set on Forward at DecodeHeaders via clock.Now(); consumed
	// at Task 6 EncodeHeaders body to compute rtt = clock.Now() - entryTime.
	// Zero-valued (time.Time{}) when no token was acquired (disabled
	// pass-through OR Block leg).
	entryTime time.Time

	// acquired is true between successful forwardingDecision() Forward and
	// the encoder-side release (or OnDestroy fallback per D11). False on
	// disabled pass-through OR Block leg. Discriminates the encode-side
	// hook (Task 6) between release-and-record + no-op.
	acquired bool
}

// Static interface conformance assertions. The *filter implements BOTH
// the decoder and encoder side per SPEC §6.3 + Task 9's
// `HTTPFilter{Decoder: f, Encoder: f}` wiring.
var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

// SetDecoderCallbacks stores the callbacks for later SendLocalReply use at
// the DecodeHeaders Block path. Mirrors rbac.go::SetDecoderCallbacks +
// oauth2.go::SetDecoderCallbacks precedent.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }

// SetEncoderCallbacks is a no-op at MVP. The encoder side of
// adaptive_concurrency does not consume the callbacks surface: Task 6's
// EncodeHeaders body records RTT + releases the in-flight token directly
// via the controller pointer, without callback indirection (no
// SendLocalReply on the encode side, no OverwriteBody, no
// BufferEncodedBody). The interface method MUST exist for the
// StreamEncoderFilter assertion to compile; the body intentionally
// discards the supplied callbacks.
func (f *filter) SetEncoderCallbacks(_ envoyhttp.EncoderFilterCallbacks) {}

// DecodeData is pass-through. adaptive_concurrency is a header-time gate;
// no per-stream body interaction.
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers is pass-through.
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// EncodeHeaders is the encode-side RTT-recording hook per SPEC §6.5. The
// envoy-go encode-side first-hook is the OnEncodeComplete semantic
// equivalent (envoy-go has no dedicated OnEncodeComplete callback per the
// Task-3 discovery anchored at controller.go header).
//
// Delegates to recordRTTAndRelease (encode_complete.go) which:
//   - is a no-op when !f.acquired (Block leg OR disabled pass-through);
//   - computes rtt = clock.Now() - entryTime, calls
//     controller.recordLatencySample(rtt) + controller.releaseInFlight(),
//     and clears f.acquired (preventing the D11 OnDestroy double-release).
func (f *filter) EncodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	f.recordRTTAndRelease()
	return envoyhttp.Continue
}

// EncodeData is pass-through. The RTT-recording happens at EncodeHeaders
// (the first encode-side hook).
func (f *filter) EncodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// EncodeTrailers is pass-through.
func (f *filter) EncodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// OnDestroy is the per-stream destruction hook + the D11 token-release-on-
// reset-before-encode safety net per planner-time D11.
//
// Guards three lifecycle paths:
//
//   - Happy path (DecodeHeaders Forward → EncodeHeaders fires → releases) —
//     OnDestroy observes f.acquired == false (cleared by the encode-side
//     release) and is a no-op. Per-stream idempotency invariant.
//   - Stream-reset / client-disconnect path (DecodeHeaders Forward → stream
//     terminated mid-decode → EncodeHeaders never fires) — OnDestroy
//     releases the token to prevent a permanent slot leak in
//     numRqOutstanding. Without this fallback, a single mid-decode reset
//     would consume one slot forever; sustained-reset workloads would
//     eventually pin numRqOutstanding == concurrencyLimit, sending 100% of
//     subsequent requests to the Block leg.
//   - Disabled / Block path (DecodeHeaders short-circuit; f.acquired stays
//     false) — OnDestroy observes f.acquired == false and is a no-op. Must
//     NOT decrement (the disabled-filter pass-through is the dominant case
//     at rollout — see decode_headers.go header).
//
// Idempotency is critical: numRqOutstanding is a uint32; a double-decrement
// wraps around to MAX_UINT32 — a far worse failure mode than the slot leak
// the D11 fallback was added to prevent. The f.acquired boolean serves as
// the per-stream idempotency flag.
func (f *filter) OnDestroy() {
	if f.acquired {
		f.controller.releaseInFlight()
		f.acquired = false
	}
}
