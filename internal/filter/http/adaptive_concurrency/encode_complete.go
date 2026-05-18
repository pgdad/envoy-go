package adaptive_concurrency

// encode_complete.go — the encode-side RTT-recording + token-release helper
// invoked from filter.go's EncodeHeaders method per phase-21 SPEC §6.5 +
// planner-time D11.
//
// # Hook semantics (SPEC §6.5)
//
// The SPEC's OnEncodeComplete() pseudocode maps to envoy-go's EncodeHeaders
// (the first encode-side hook to fire on a per-stream filter; envoy-go has
// no dedicated OnEncodeComplete callback — the response-completion semantic
// equivalent is captured at the first encode-side header observation per the
// Task-3 discovery anchored at controller.go header). The hook body:
//
//  1. If !f.acquired → return (no-op for the Block leg OR the disabled-filter
//     pass-through; the encode-side observes both pre-decode and post-decode
//     state via the acquired discriminator).
//  2. Compute rtt = clock.Now() - entryTime (the per-stream entry timestamp
//     was set at decode_headers.go's Forward leg).
//  3. Call controller.recordLatencySample(rtt) — routes to latencySamples or
//     minRTTSamples per the controller's in-window state per SPEC §4.5.
//  4. Call controller.releaseInFlight() — atomic decrement of
//     numRqOutstanding per planner-time D17 (hot-path lock-free).
//  5. Clear f.acquired = false (prevents D11 OnDestroy double-release per
//     filter.go::OnDestroy's idempotency contract).
//
// # D11 token-release symmetry
//
// The OnDestroy body at filter.go is the symmetric pair for the abort /
// reset paths where EncodeHeaders never fires. Together they pin the
// invariant: exactly one of {EncodeHeaders, OnDestroy} releases the token,
// regardless of which fires first OR whether both fire — the f.acquired
// boolean serves as the per-stream idempotency flag.
//
// # Cross-references
//
//   - SPEC §6.5 (encode-side RTT-recording hook body)
//   - planner-time D11 (OnDestroy token-release symmetry)
//   - planner-time D17 (atomic.Uint32 hot-path; lock-free release)
//   - controller.go::recordLatencySample (in-window-aware sample routing)
//   - controller.go::releaseInFlight (atomic decrement of numRqOutstanding)
//   - filter.go::OnDestroy (D11 symmetric pair; idempotent via f.acquired)

// recordRTTAndRelease is the encode-side hook body per phase-21 SPEC §6.5.
// See file-header comment for the per-step discipline + the D11 idempotency
// contract anchored against filter.go::OnDestroy.
func (f *filter) recordRTTAndRelease() {
	if !f.acquired {
		return
	}
	rtt := f.clock.Now().Sub(f.entryTime)
	f.controller.recordLatencySample(rtt)
	f.controller.releaseInFlight()
	f.acquired = false
}
