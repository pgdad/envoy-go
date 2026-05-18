package adaptive_concurrency

// clock.go — in-package Clock interface seam per phase-21 SPEC §3.1 + §6.3 +
// ADR-0186 §Decision (inline NOT framework primitive). Production wiring uses
// defaultClock (time.Now + time.AfterFunc). Tests use the test-scope-only
// fakeClock at clock_test.go (step-driven; deterministic per planner-time D9).
//
// # Why inline (NOT a framework primitive)
//
// Per ADR-0186 §Decision + phase-21 BRAINSTORM Q3 + the phase-17 jwt_authn
// EXTRACT-NOW-only-when-trigger-fires lesson: consumer count at introduction
// time is **1** (the in-package gradientController only). The seam stays inline
// at internal/filter/http/adaptive_concurrency/clock.go until a second timer-
// driven filter materializes (admission_control, global rate limit, or similar
// §9 row). The forward-pointer EXTRACT-NOW trigger is anchored at ADR-0186
// §Consequences (g) — when the second consumer lands, this file lifts to
// internal/clock/ (or equivalent framework path) with a `defaultClock` move +
// the consumer's Clock-typed field unchanged.
//
// # Surface
//
//   - Clock interface (Now + AfterFunc) with the Stop handle returned by
//     AfterFunc mirroring time.Timer.Stop semantics.
//   - defaultClock + timerStop production wiring (~10 LoC of code; the rest
//     is doc-comment per ADR-0186 §Decision).
//
// The fakeClock test-scope implementation lives at clock_test.go in the same
// package; it is NOT compiled into the production binary (test-file-only).

import "time"

// Clock is the time-source seam used by gradientController for deterministic
// FAKE-TIME testing per phase-21 SPEC §3.1 + ADR-0186 §Decision. The
// production wiring uses defaultClock (time.Now + time.AfterFunc); tests use
// fakeClock at clock_test.go (step-driven; deterministic per planner-time D9).
//
// Inline NOT framework primitive per ADR-0186 §Decision (consumer count 1 at
// introduction time; phase-17 jwt_authn EXTRACT-NOW-only-when-trigger-fires
// lesson). When a second timer-driven filter materializes (admission_control,
// global rate limit, or similar §9 row), extract to internal/clock/ per the
// forward-pointer at ADR-0186 §Consequences (g).
type Clock interface {
	// Now returns the current time per the time-source convention. Wall-clock
	// in production; step-driven in tests.
	Now() time.Time
	// AfterFunc schedules fn to fire after d. Returns a Stop handle that can
	// cancel a pending fire. Caller-thread-safe in both implementations.
	AfterFunc(d time.Duration, fn func()) Stop
}

// Stop is the per-timer cancellation handle returned by Clock.AfterFunc.
// Mirrors time.Timer.Stop semantics: returns true if the call stopped the
// timer (the fn will NOT fire); false if the timer had already expired or
// been stopped.
type Stop interface {
	Stop() bool
}

// defaultClock wraps time.Now + time.AfterFunc for production wiring. The
// zero-value is usable (no state). Consumed at Task 9 boot-registration where
// the filter factory wires defaultClock{} as the Clock arg to
// newGradientController in the production HCM-build path
// (adaptive_concurrency.go::New).
type defaultClock struct{}

func (defaultClock) Now() time.Time { return time.Now() }

func (defaultClock) AfterFunc(d time.Duration, fn func()) Stop {
	return timerStop{t: time.AfterFunc(d, fn)}
}

// timerStop adapts *time.Timer to the Stop interface. Instantiated
// transitively from defaultClock.AfterFunc; consumed via the controller's
// Stop-typed timer fields per controller.go.
type timerStop struct {
	t *time.Timer
}

func (s timerStop) Stop() bool { return s.t.Stop() }
