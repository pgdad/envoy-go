package admission_control

// decode_headers.go — the DecodeHeaders 3-gate dispatch body per phase-23
// SPEC §6.4 + AMEND-7 + AMEND-11 + PD-2.503 + PD-3.
//
// # 3-gate dispatch shape (per SPEC §6.4)
//
//  1. Disabled pass-through: !f.cc.enabled ⇒ f.record=false; return Continue.
//     The healthCheck() arm is NOT-MODELED per PD-3: envoy-go's
//     DecoderFilterCallbacks has NO StreamInfo()/HealthCheck()/IsHealthCheck()
//     accessor (verified against internal/filter/http/callbacks.go). Adding such
//     an accessor would be a NEW framework primitive — violating the
//     ZERO-new-primitive constraint at phase-23. Documented in PROGRESS Task 5
//     entry with forward-pointer to the Task 11 BEHAVIOR_CONTRACT note.
//
//  2. RPS suppression: averageRps() < rpsThreshold ⇒ return Continue WITHOUT
//     consulting shouldReject. f.record stays true (the request is admitted
//     to encode-side classification). NOT a reject. Mirrors upstream
//     admission_control.cc:87-91.
//
//  3. Probabilistic reject: shouldReject() ⇒
//     f.record=false; f.stats.rqRejected.Inc();
//     f.cb.SendLocalReply(503, "", nil); return StopIteration.
//     Wire shape per AMEND-7 + PD-2.503: status 503, EMPTY body "", nil
//     headers. The "denied_by_admission_control" rc-details is NOT
//     surfaceable through the 3-arg SendLocalReply API (ABSENT-by-API per
//     PD-2.503 — NOT pinned in this file).
//
//  4. Admit pass-through: return Continue. f.record stays true.
//
// # Counter discipline
//
// rqRejected is incremented HERE (filter level) per AMEND-11, NOT inside the
// controller. shouldReject() does NOT touch rqRejected — the filter owns that
// increment. (Contrast with adaptive_concurrency where the controller's
// forwardingDecision increments rqBlocked internally; admission_control's
// controller ONLY handles the window + formula.)
//
// # Cross-references
//
//   - SPEC §6.4 (DecodeHeaders dispatch — 3-gate order)
//   - AMEND-7 (reject wire shape: 503 + empty body + nil headers)
//   - AMEND-11 (record discipline: reject + disabled ⇒ f.record=false)
//   - PD-2.503 (SendLocalReply(503, "", nil) — 3-arg API; rc-details ABSENT-by-API)
//   - PD-3 (healthCheck() arm NOT-MODELED at MVP)
//   - adaptive_concurrency/decode_headers.go (nearest precedent; differs in gate order
//     + reject wire shape + no acquired/entryTime per-stream state)

import (
	"net/http"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// DecodeHeaders is the request-gate entry per SPEC §6.4. Three-gate order:
// disabled pass-through → RPS suppression → probabilistic reject → admit.
// See file-header comment for the per-gate discipline + AMEND-7 wire-shape pin.
func (f *filter) DecodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Gate 1: disabled pass-through.
	// The healthCheck() arm is NOT-MODELED per PD-3 (no StreamInfo() accessor
	// on DecoderFilterCallbacks; adding one would be a new framework primitive).
	// Clear record per AMEND-11: a disabled-filter request MUST NOT be classified
	// at encode time.
	if !f.cc.enabled {
		f.record = false
		return envoyhttp.Continue
	}

	// Gate 2: RPS suppression — admit WITHOUT consulting shouldReject.
	// averageRps() < rpsThreshold ⇒ traffic volume below the RPS floor; the
	// filter would produce noisy decisions on a near-empty window. Pass through
	// with f.record=true so the encode side classifies the request normally.
	// NOT a reject; do NOT clear record. Mirrors admission_control.cc:87-91.
	if f.controller.averageRps() < f.cc.rpsThreshold {
		return envoyhttp.Continue
	}

	// Gate 3: probabilistic reject.
	// shouldReject() consults the window via requestCounts() + the formula.
	// On reject: clear record per AMEND-11; increment rqRejected (filter owns
	// this counter per the controller comment above); emit the AMEND-7 wire shape.
	if f.controller.shouldReject() {
		f.record = false
		f.stats.rqRejected.Inc()
		// AMEND-7 + PD-2.503 reject wire shape: status 503, EMPTY body "",
		// nil headers. The "denied_by_admission_control" rc-details is NOT
		// surfaceable through the 3-arg SendLocalReply (ABSENT-by-API — do NOT
		// attempt to pin it here).
		f.cb.SendLocalReply(503, "", nil)
		return envoyhttp.StopIteration
	}

	// Gate 4: admit pass-through. f.record stays true; encode side classifies.
	return envoyhttp.Continue
}

// DecodeData is pass-through. admission_control is a header-time gate; no
// per-stream body interaction.
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers is pass-through.
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// SetDecoderCallbacks stores the callbacks for later SendLocalReply use at the
// reject path. Mirrors adaptive_concurrency/filter.go::SetDecoderCallbacks +
// rbac.go::SetDecoderCallbacks precedent.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.cb = cb }
