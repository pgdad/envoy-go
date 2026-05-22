package admission_control

// clock.go — in-package Clock interface seam per phase-23 SPEC §3.2 + ADR-0194
// §Decision (inline NOT framework primitive). Production wiring uses defaultClock
// (time.Now). Tests use the test-scope-only fakeClock at clock_test.go
// (step-driven; deterministic).
//
// # Why inline (NOT a framework primitive)
//
// Per SPEC §3.2 + SPEC §2.5: Clock-shaped consumer count is now 2 (phase-21
// adaptive_concurrency + phase-23 admission_control). The documented EXTRACT-NOW
// trigger threshold is met in count, BUT the two seams have materially different
// shapes — phase-21 needs Now() + AfterFunc (timer cascade); phase-23 needs
// Now() ONLY (monotonic wall-clock reads for bucket rollover). A shared
// internal/clock/ extraction would over-fit to the divergent shapes.
// Phase-23 records the convergence question as a forward-pointer (SPEC §8
// item 8); NOT an extraction obligation. The phase-17 jwt_authn
// EXTRACT-NOW-only-when-trigger-genuinely-fires lesson governs.
//
// # Shape difference from phase-21 (Now-ONLY)
//
// Phase-23's Clock is Now()-ONLY. No AfterFunc, no timers, no Stop handle.
// The sliding success-rate window expires per-second buckets by monotonic
// wall-clock reads on every recordRequest + requestCounts call — no background
// timer cascade. This is a strictly simpler shape than phase-21's Clock.
//
// # Surface
//
//   - Clock interface (Now() time.Time ONLY).
//   - defaultClock production wiring wrapping time.Now.
//
// The fakeClock test-scope implementation lives at clock_test.go in the same
// package; it is NOT compiled into the production binary (test-file-only).

import "time"

// Clock is the controller's seam over the monotonic clock used to expire
// per-second window buckets (mirrors upstream's TimeSource::monotonicTime()).
// The in-package interface lets the unit tests inject a step-driven fake clock
// to drive window-bucket rollover + expiry deterministically without relying on
// real wall-clock time.
//
// Inline NOT framework primitive per SPEC §3.2 + §2.5 (shapes differ:
// phase-21 = Now()+AfterFunc; phase-23 = Now()-ONLY; forward-pointer at
// SPEC §8 item 8 to the convergence question). When a third convergent
// consumer materializes, re-evaluate extraction to internal/clock/.
type Clock interface {
	// Now returns the current time per the time-source convention. Wall-clock
	// in production; step-driven in tests.
	Now() time.Time
}

// defaultClock wraps time.Now (which carries a monotonic reading) for
// production wiring. The zero-value is usable (no state). Consumed at filter
// construction where the factory wires defaultClock{} as the Clock arg to
// the controller in the production HCM-build path.
type defaultClock struct{}

func (defaultClock) Now() time.Time { return time.Now() }
