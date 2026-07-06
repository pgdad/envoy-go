package wasm

// tick_clock.go — Clock seam injection plumbing per 25.2 SPEC §3.1 + Q5 +
// R-25.2-9. The Clock seam (phase-21 ADR-0186) is FIRST co-consumed at the
// internal/wasm/ framework primitive layer via wasm.WithRootClock (per Task
// 5 internal/wasm/tick.go); this file lands the consumer-side test seam at
// the internal/filter/http/wasm/ filter package — fixture-0036 tick-fires-
// counter scenario injects a clock.FakeClock here to make per-tick guest-
// dispatch deterministic.
//
// # Production posture
//
// Production callers do NOT supply a clock — the buildCompiledConfig path
// (compiled_config.go) constructs the *RootVM via wasm.NewRootVM WITHOUT a
// WithRootClock option; wasm.NewRootVM defaults rv.clk = clock.RealClock{}
// per the Task 1 root_vm.go default-clock branch. The tick goroutine
// (internal/wasm/tick.go) reads rv.clk unlocked after construction; production
// callers get the wall-clock semantic verbatim.
//
// # Test posture
//
// Test callers — particularly fixture-0036 mixed-mode (Task 20) which
// includes a tick-fires-counter scenario — inject a clock.FakeClock via the
// test-only ClockOption helper here. The helper applies BEFORE buildCompiled
// Config's wasm.NewRootVM call so the WithRootClock option threads through
// the rootOpts slice at compiled_config.go line 927-937.
//
// # Why this file exists
//
// The Clock seam at internal/wasm/ is a parameter of NewRootVM (per Q5 + Task
// 5); buildCompiledConfig (compiled_config.go) is the constructor that
// allocates the *RootVM via NewRootVM. To make Clock injectable at the
// FILTER layer (where fixtures live), we need a seam that buildCompiled
// Config consults — that's the package-level testClock variable below + the
// withTestClock helper.
//
// The mechanism is intentionally test-only: production callers never invoke
// withTestClock; the default zero-value testClock is nil → no WithRootClock
// option appended → wasm.NewRootVM defaults to RealClock per the framework-
// layer plumbing.
//
// # Cross-references
//
//   - Q5 (BRAINSTORM Q5 — tick goroutine + 10ms floor + Clock seam FIRST
//     co-consumer)
//   - R-25.2-9 (RATIFICATION — phase-21 ADR-0186 Clock seam extraction)
//   - 25.2 SPEC §3.1 (internal/wasm/ tick.go + Clock seam)
//   - ADR-0186 (Clock seam — phase-21 extraction; envoy-go test discipline)
//   - ADR-0205 (root-VM lifecycle evolution; rootVM constructed once at New)
//   - phase-25.2 IMPL Task 5 (internal/wasm/tick.go + WithRootClock + default
//     RealClock branch)
//   - phase-25.2 IMPL Task 14 (compiled_config.go rootVM construction)

import (
	"github.com/pgdad/envoy-go/internal/clock"
)

// testClock is the test-only Clock seam injection point. nil under production
// callers (the default) means buildCompiledConfig does NOT pass a WithRootClock
// option to wasm.NewRootVM + the framework defaults to clock.RealClock per
// Task 5 root_vm.go.
//
// Test callers invoke withTestClock(fc) at test setup to inject a
// clock.FakeClock; the supplied clock is consulted by buildCompiledConfig
// when constructing the *RootVM. Concurrent test execution requires test
// isolation (the variable is package-level state); fixture-0036 +
// body_test.go OWN this variable and reset to nil at test teardown via
// t.Cleanup.
//
// PRODUCTION NEVER touches this variable. The package-level state is a
// deliberate test-only seam per the comment block above.
var testClock clock.Clock

// withTestClock installs the supplied clock as the test-only Clock seam for
// subsequent buildCompiledConfig calls. Returns a cleanup function that
// restores testClock to its previous value (typically nil). Callers MUST
// defer the cleanup OR invoke it at t.Cleanup to avoid cross-test state
// bleed.
//
// Test-only by contract. Production callers never invoke this function;
// the default zero-value testClock is nil + production sees RealClock via
// the framework-layer default at wasm.NewRootVM.
//
// Usage:
//
//	func TestTickClock_Whatever(t *testing.T) {
//	    fc := clock.NewFakeClock(time.Unix(0, 0))
//	    restore := withTestClock(fc)
//	    t.Cleanup(restore)
//	    // ... construct compiledConfig + dispatch ticks via fc.Step ...
//	}
//
// Per ADR-0186 + R-25.2-9: the Clock seam is consumer-injectable at the
// FRAMEWORK boundary (NewRootVM); this helper bridges from the filter-layer
// test harness to that framework boundary via the testClock package var
// consulted by buildCompiledConfig.
func withTestClock(c clock.Clock) (restore func()) {
	prev := testClock
	testClock = c
	return func() { testClock = prev }
}

// resolveClock returns the Clock that compiled_config.go's buildCompiledConfig
// passes to wasm.NewRootVM via WithRootClock. Production callers (with nil
// testClock) get nil back; buildCompiledConfig SKIPS appending the
// WithRootClock option in that case — the framework-layer default at
// wasm.NewRootVM fires + rv.clk = clock.RealClock{}.
//
// Test callers see the injected FakeClock returned verbatim; buildCompiled
// Config appends WithRootClock(c) and the *RootVM's tick goroutine runs
// under the FakeClock semantic (deterministic time control via fc.Step).
//
// The indirection through this accessor (rather than a direct testClock
// read at compiled_config.go) keeps the test-only seam SCOPED to this file
// + makes the production-callers-never-touch contract auditable.
func resolveClock() clock.Clock {
	return testClock
}
