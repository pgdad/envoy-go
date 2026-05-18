package adaptive_concurrency

// decode_headers.go — the DecodeHeaders dispatch body per phase-21 SPEC §6.4
// + AMEND-6 byte-pinned 503-overflow wire shape.
//
// # Dispatch shape (per SPEC §6.4)
//
// Three legs:
//
//  1. Disabled pass-through: cc.enabled == false ⇒ Continue (no controller
//     consultation, no state mutation). The default-application step at
//     buildCompiledConfig (Task 2) sets enabled = false when the proto field
//     is absent OR default_enabled == false per AMEND-4; the disabled leg
//     is the dominant case at filter rollout (operators flip enabled = true
//     only after canarying).
//  2. Forward (capacity available): forwardingDecision() returns true ⇒
//     record f.entryTime = clock.Now(); set f.acquired = true; Continue.
//     The encoder-side hook (Task 6) consumes f.entryTime to compute the
//     RTT sample + f.acquired to discriminate the release-or-no-op path.
//  3. Block (at capacity): forwardingDecision() returns false ⇒
//     SendLocalReply(concurrencyLimitExceededStatus, "reached concurrency
//     limit", {content-type: text/plain}) per AMEND-6; StopIteration.
//
// # AMEND-6 byte-pinned 503 wire shape
//
// Per SPEC §11 §21.P1 RATIFIED + AMEND-6:
//
//   - status: cc.concurrencyLimitExceededStatus (default 503 per Task 2
//     default-application; configurable via the
//     concurrency_limit_exceeded_status proto field — operators may set 429
//     Too Many Requests OR any other valid HTTP status code).
//   - body: "reached concurrency limit" (25 bytes verbatim; no trailing
//     newline; no JSON wrapping). Byte-pinned constant rqBlockedBody below.
//   - headers: content-type: text/plain (lowercase per SPEC line 440
//     fixture convention). The HCM framework auto-injects content-length:
//     25 based on the body length per the OAUTH2/cookies precedent (no
//     per-filter content-length emission required).
//
// The response_code_details "reached_concurrency_limit" field (upstream
// Envoy at adaptive_concurrency_filter.cc:50-54) is NOT byte-pinned at
// envoy-go MVP per SPEC §12 item A3 — envoy-go has no access-log surface;
// the response_code_details field is ABSENT-by-config (NOT byte-pinned).
//
// # Counter discipline (rqBlocked single-increment)
//
// Per Task 3's controller.go::forwardingDecision: the controller's Block
// leg increments rqBlocked exactly once before returning false. The filter
// MUST NOT double-increment — this body emits the SendLocalReply +
// StopIteration but does NOT touch f.controller.stats.rqBlocked. The
// TestFilter_DecodeHeaders_Block_503_BodyAndHeaders_ByteExact test pins
// this single-increment invariant.
//
// # Cross-references
//
//   - SPEC §6.4 (DecodeHeaders dispatch — three legs)
//   - SPEC §11 §21.P1 (503-overflow byte-pinned wire shape)
//   - AMEND-6 (cross-side byte-exact promotion driver)
//   - ADR-0186 §Decision (controller forwardingDecision hot-path)
//   - rbac.go::DecodeHeaders (precedent for SendLocalReply + StopIteration
//     pattern on a denial leg)

import (
	"net/http"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// rqBlockedBody is the byte-pinned 25-byte response body per AMEND-6 +
// SPEC §11 §21.P1 RATIFIED. Verbatim; no trailing newline; no JSON
// wrapping. Length asserted at decode_headers_test.go::
// TestFilter_DecodeHeaders_Block_503_BodyAndHeaders_ByteExact.
const rqBlockedBody = "reached concurrency limit"

// DecodeHeaders is the request-gate entry per SPEC §6.4. Three legs:
// disabled pass-through, Forward (capacity available), Block (at
// capacity). See file-header comment for the per-leg discipline + the
// AMEND-6 byte-pinned wire-shape pin.
func (f *filter) DecodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Leg 1: disabled pass-through. The controller is NOT consulted +
	// the rqBlocked counter does NOT increment + acquired stays false.
	if !f.cc.enabled {
		return envoyhttp.Continue
	}

	// Legs 2 + 3: consult the controller. forwardingDecision is hot-path
	// lock-free per planner-time D17 (atomic CAS on numRqOutstanding +
	// atomic Load on concurrencyLimit). On Block, the controller already
	// incremented rqBlocked — DO NOT double-increment here.
	if !f.controller.forwardingDecision() {
		// Leg 3: Block. Emit the AMEND-6 byte-pinned 503-overflow wire
		// shape. SPEC line 440 fixture convention pins the content-type
		// header value to lowercase "text/plain"; the HCM framework
		// auto-injects content-length: 25 downstream based on the body
		// length (no per-filter content-length emission required).
		f.dcb.SendLocalReply(
			int(f.cc.concurrencyLimitExceededStatus),
			rqBlockedBody,
			envoyhttp.OrderedHeaders{
				{Name: "content-type", Value: "text/plain"},
			},
		)
		return envoyhttp.StopIteration
	}

	// Leg 2: Forward. Record the entry timestamp for the Task-6 encoder-
	// side RTT computation; flip acquired so the encoder-side hook
	// dispatches the release-and-record path (vs the no-op path that
	// fires when acquired is false).
	f.entryTime = f.clock.Now()
	f.acquired = true
	return envoyhttp.Continue
}
