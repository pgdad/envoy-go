package adaptive_concurrency

// encode_complete_test.go — Group 6 encode-side hook + OnDestroy D11
// token-release tests for the adaptive_concurrency filter per phase-21 SPEC
// §6.5 + planner-time D11.
//
// Covers five scenarios spanning the encode-side + OnDestroy lifecycle:
//
//  1. Happy path (EncodeHeaders RTT-record + release): Forward at
//     DecodeHeaders (sets f.acquired = true + f.entryTime); advance the fake
//     clock by some delta D; call EncodeHeaders; verify
//     controller.numRqOutstanding decremented + RTT sample observable in the
//     controller's in-window minRTTSamples slice + f.acquired flipped to
//     false (preventing the D11 OnDestroy double-release).
//
//  2. Disabled / Block pass-through (EncodeHeaders no-op): f.acquired = false
//     from the start (e.g., the DecodeHeaders disabled leg or Block leg);
//     EncodeHeaders returns Continue without touching the controller
//     (numRqOutstanding unchanged; no sample appended).
//
//  3. D11 token-release on reset-before-encode: Forward at DecodeHeaders;
//     never call EncodeHeaders (simulates client disconnect / stream reset
//     mid-decode); call OnDestroy; verify numRqOutstanding decremented +
//     f.acquired flipped to false. This is the D11 safety-net path that
//     guards against permanent slot-leak on the abort lifecycle.
//
//  4. D11 idempotency (OnDestroy after EncodeHeaders already released): the
//     symmetric pair test — happy-path EncodeHeaders already released the
//     token; OnDestroy must observe f.acquired == false + no-op. Without the
//     idempotency guard, OnDestroy would double-decrement
//     numRqOutstanding (uint32 wraparound to MAX_UINT32 — a far worse
//     failure mode than the slot-leak it prevents).
//
//  5. OnDestroy on never-acquired path: DecodeHeaders disabled leg (or never
//     called); f.acquired = false from the start; OnDestroy is a no-op
//     (numRqOutstanding unchanged).
//
// All five tests use the newTestFilter helper from decode_headers_test.go
// (Task 5 landing) — the same *compiledConfig + *fakeClock + *filterStats +
// *gradientController wiring, no additional helpers required.

import (
	"net/http"
	"testing"
	"time"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// -----------------------------------------------------------------------------
// Test 1: EncodeHeaders happy path — RTT recorded + token released
// -----------------------------------------------------------------------------

// TestFilter_EncodeHeaders_RecordsRTTAndReleases verifies the SPEC §6.5
// encode-side hook body: after a Forward at DecodeHeaders (sets f.acquired =
// true + records f.entryTime), advancing the clock by delta D and invoking
// EncodeHeaders must:
//
//   - Decrement numRqOutstanding (controller.releaseInFlight() — atomic Add(-1))
//   - Append RTT = D to the controller's sample slice (here minRTTSamples
//     since the controller starts in-window per AMEND-2 C4)
//   - Clear f.acquired (prevents D11 OnDestroy double-release)
//   - Return Continue (does NOT stop iteration; the encode-side proceeds
//     normally to the downstream)
func TestFilter_EncodeHeaders_RecordsRTTAndReleases(t *testing.T) {
	f, clock, _ := newTestFilter(t)

	// Forward at DecodeHeaders: numRqOutstanding == 1 + f.acquired == true +
	// f.entryTime == clock.Now().
	if status := f.DecodeHeaders(http.Header{}, false); status != envoyhttp.Continue {
		t.Fatalf("setup: DecodeHeaders returned %v; want Continue (Forward leg)", status)
	}
	if !f.acquired {
		t.Fatalf("setup: f.acquired = false after Forward; want true")
	}
	if got := f.controller.numRqOutstanding.Load(); got != 1 {
		t.Fatalf("setup: numRqOutstanding = %d after Forward; want 1", got)
	}

	// Advance the clock by a known delta — the RTT sample appended by
	// EncodeHeaders must equal this delta exactly (deterministic fakeClock).
	const rttDelta = 42 * time.Millisecond
	clock.Advance(rttDelta)

	status := f.EncodeHeaders(http.Header{}, false)

	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (encode-side hook does not stop iteration)", status)
	}
	// Token released: numRqOutstanding decremented back to 0.
	if got := f.controller.numRqOutstanding.Load(); got != 0 {
		t.Errorf("numRqOutstanding = %d after EncodeHeaders; want 0 (releaseInFlight should have decremented)", got)
	}
	// f.acquired cleared (prevents OnDestroy double-release per D11).
	if f.acquired {
		t.Errorf("f.acquired = true after EncodeHeaders; want false (must be cleared post-release)")
	}
	// RTT sample appended. The controller starts in the minRTT sampling window
	// per AMEND-2 C4 — recordLatencySample routes to minRTTSamples until the
	// window closes. Inspect the slice directly under mu.
	f.controller.mu.Lock()
	defer f.controller.mu.Unlock()
	if len(f.controller.minRTTSamples) != 1 {
		t.Fatalf("minRTTSamples len = %d after EncodeHeaders; want 1 (single RTT sample appended in-window)", len(f.controller.minRTTSamples))
	}
	if got := f.controller.minRTTSamples[0]; got != rttDelta {
		t.Errorf("minRTTSamples[0] = %v; want %v (RTT = clock.Now() - f.entryTime)", got, rttDelta)
	}
}

// -----------------------------------------------------------------------------
// Test 2: EncodeHeaders no-op when not acquired
// -----------------------------------------------------------------------------

// TestFilter_EncodeHeaders_NotAcquired_NoOp verifies that when f.acquired ==
// false at EncodeHeaders entry (e.g., the disabled-filter pass-through or
// the DecodeHeaders Block leg), EncodeHeaders returns Continue WITHOUT
// touching the controller (no sample appended; numRqOutstanding unchanged).
// This is the SPEC §6.5 no-op branch.
func TestFilter_EncodeHeaders_NotAcquired_NoOp(t *testing.T) {
	f, clock, _ := newTestFilter(t)
	// Simulate the disabled-filter pass-through: never call DecodeHeaders, so
	// f.acquired stays false + f.entryTime stays zero.
	outstandingBefore := f.controller.numRqOutstanding.Load()
	clock.Advance(99 * time.Millisecond) // advance to confirm time is irrelevant

	status := f.EncodeHeaders(http.Header{}, false)

	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (no-op pass-through)", status)
	}
	if got := f.controller.numRqOutstanding.Load(); got != outstandingBefore {
		t.Errorf("numRqOutstanding = %d after EncodeHeaders no-op; want %d (controller must not be touched)", got, outstandingBefore)
	}
	if f.acquired {
		t.Errorf("f.acquired = true after no-op EncodeHeaders; want false (state unchanged)")
	}
	// No sample appended in either slice.
	f.controller.mu.Lock()
	defer f.controller.mu.Unlock()
	if len(f.controller.minRTTSamples) != 0 {
		t.Errorf("minRTTSamples len = %d after no-op EncodeHeaders; want 0 (no sample on no-op branch)", len(f.controller.minRTTSamples))
	}
	if len(f.controller.latencySamples) != 0 {
		t.Errorf("latencySamples len = %d after no-op EncodeHeaders; want 0 (no sample on no-op branch)", len(f.controller.latencySamples))
	}
}

// -----------------------------------------------------------------------------
// Test 3: OnDestroy D11 — releases token on reset-before-encode
// -----------------------------------------------------------------------------

// TestFilter_OnDestroy_ReleasesAcquiredToken_AcquiredButNotEncoded covers the
// D11 stream-reset / client-disconnect / HCM-side abort path: Forward at
// DecodeHeaders (acquires a token); EncodeHeaders NEVER fires (stream
// terminated mid-decode); OnDestroy MUST release the token to prevent a
// permanent slot leak in numRqOutstanding.
//
// Without this safety net, a single mid-decode reset would permanently
// consume one slot from the concurrency limit; sustained-reset workloads
// would eventually pin numRqOutstanding == concurrencyLimit forever, sending
// 100% of subsequent requests to the Block leg.
func TestFilter_OnDestroy_ReleasesAcquiredToken_AcquiredButNotEncoded(t *testing.T) {
	f, _, _ := newTestFilter(t)

	// Forward at DecodeHeaders — acquires the token.
	if status := f.DecodeHeaders(http.Header{}, false); status != envoyhttp.Continue {
		t.Fatalf("setup: DecodeHeaders returned %v; want Continue (Forward leg)", status)
	}
	if !f.acquired {
		t.Fatalf("setup: f.acquired = false after Forward; want true")
	}
	if got := f.controller.numRqOutstanding.Load(); got != 1 {
		t.Fatalf("setup: numRqOutstanding = %d after Forward; want 1", got)
	}

	// Stream reset mid-decode: EncodeHeaders never fires; OnDestroy is the
	// only post-decode hook to execute. D11 must release the token.
	f.OnDestroy()

	if got := f.controller.numRqOutstanding.Load(); got != 0 {
		t.Errorf("numRqOutstanding = %d after OnDestroy on acquired stream; want 0 (D11 must release the token)", got)
	}
	if f.acquired {
		t.Errorf("f.acquired = true after OnDestroy; want false (D11 must clear the acquired flag)")
	}
}

// -----------------------------------------------------------------------------
// Test 4: OnDestroy D11 idempotency — no double-release after EncodeHeaders
// -----------------------------------------------------------------------------

// TestFilter_OnDestroy_AlreadyReleased_NoOp pins the D11 idempotency
// invariant: when EncodeHeaders has already run (and thereby released the
// token + cleared f.acquired), a subsequent OnDestroy MUST observe f.acquired
// == false and skip the release. Without this guard, the
// happy-path-followed-by-OnDestroy sequence would double-decrement
// numRqOutstanding — and because the counter is a uint32, the second
// decrement would wrap around to MAX_UINT32, a far worse failure mode
// (every subsequent request would block forever) than the slot-leak that
// D11 was added to prevent.
//
// The symmetry test for Test 3 — together they pin the bidirectional
// invariant: exactly one of {EncodeHeaders, OnDestroy} releases the token,
// regardless of which fires first or whether both fire.
func TestFilter_OnDestroy_AlreadyReleased_NoOp(t *testing.T) {
	f, _, _ := newTestFilter(t)

	// Forward at DecodeHeaders — acquires the token.
	if status := f.DecodeHeaders(http.Header{}, false); status != envoyhttp.Continue {
		t.Fatalf("setup: DecodeHeaders returned %v; want Continue (Forward leg)", status)
	}
	// Happy path: EncodeHeaders releases the token + clears f.acquired.
	if status := f.EncodeHeaders(http.Header{}, false); status != envoyhttp.Continue {
		t.Fatalf("setup: EncodeHeaders returned %v; want Continue", status)
	}
	if f.acquired {
		t.Fatalf("setup: f.acquired = true after EncodeHeaders; want false (release cleared the flag)")
	}
	if got := f.controller.numRqOutstanding.Load(); got != 0 {
		t.Fatalf("setup: numRqOutstanding = %d after EncodeHeaders; want 0", got)
	}

	// Now OnDestroy fires (HCM-side cleanup). Per D11 idempotency, MUST be a
	// no-op — no double-release; no uint32 wraparound.
	f.OnDestroy()

	if got := f.controller.numRqOutstanding.Load(); got != 0 {
		t.Errorf("numRqOutstanding = %d after OnDestroy following EncodeHeaders; want 0 (D11 idempotency: must NOT double-release)", got)
	}
	if f.acquired {
		t.Errorf("f.acquired = true after OnDestroy; want false (state unchanged)")
	}
}

// -----------------------------------------------------------------------------
// Test 5: OnDestroy on never-acquired path — no-op
// -----------------------------------------------------------------------------

// TestFilter_OnDestroy_NotAcquired_NoOp covers the disabled-filter +
// DecodeHeaders-Block paths: f.acquired = false from the start; OnDestroy
// MUST NOT touch the controller (no decrement; no spurious state mutation).
// Without this guard, a disabled-filter pass-through (the dominant case at
// filter rollout — see decode_headers.go header) would corrupt
// numRqOutstanding via uint32 wraparound on every stream.
func TestFilter_OnDestroy_NotAcquired_NoOp(t *testing.T) {
	f, _, _ := newTestFilter(t)
	// Never call DecodeHeaders, so f.acquired stays false (simulates the
	// disabled pass-through OR a hypothetical pre-DecodeHeaders abort).
	outstandingBefore := f.controller.numRqOutstanding.Load()

	f.OnDestroy()

	if got := f.controller.numRqOutstanding.Load(); got != outstandingBefore {
		t.Errorf("numRqOutstanding = %d after OnDestroy on never-acquired stream; want %d (controller must not be touched)", got, outstandingBefore)
	}
	if f.acquired {
		t.Errorf("f.acquired = true after OnDestroy; want false (state unchanged)")
	}
}
