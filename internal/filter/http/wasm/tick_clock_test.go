package wasm

// tick_clock_test.go — Task 16 unit tests for tick_clock.go Clock seam
// injection plumbing per 25.2 SPEC §3.1 + Q5 + R-25.2-9.
//
// Test surface:
//
//   1. TestTickClock_DefaultIsNil — production default: no testClock
//      injected → resolveClock returns nil; compiled_config.go skips
//      WithRootClock + the framework defaults to RealClock.
//
//   2. TestTickClock_WithTestClock_Installs — withTestClock(fc) installs
//      the supplied clock + resolveClock returns it.
//
//   3. TestTickClock_WithTestClock_RestoreCleansUp — restore func returned
//      by withTestClock reverts testClock to its previous value (typically
//      nil); subsequent resolveClock returns nil.
//
//   4. TestTickClock_WithTestClock_StackedRestores — nested withTestClock
//      calls + restores LIFO-cleanup correctly (testClock is package-level
//      mutable state; concurrent test isolation is the caller's
//      responsibility per the test-only contract).

import (
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/clock"
)

// TestTickClock_DefaultIsNil asserts the production default: no testClock
// injection → resolveClock returns nil; the buildCompiledConfig path skips
// the WithRootClock option + the framework defaults to RealClock.
func TestTickClock_DefaultIsNil(t *testing.T) {
	// Cannot t.Parallel() — testClock is package-level mutable state +
	// other tests in this file mutate it.
	if testClock != nil {
		// Test isolation guard: prior test failure may have leaked state.
		// Reset to nil; the test body re-asserts.
		testClock = nil
	}
	if got := resolveClock(); got != nil {
		t.Errorf("resolveClock (default) = %v; want nil", got)
	}
}

// TestTickClock_WithTestClock_Installs asserts that withTestClock(fc)
// installs the supplied clock + resolveClock returns it.
func TestTickClock_WithTestClock_Installs(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0))
	restore := withTestClock(fc)
	t.Cleanup(restore)

	got := resolveClock()
	if got == nil {
		t.Fatalf("resolveClock (post-withTestClock) = nil; want injected FakeClock")
	}
	if got != clock.Clock(fc) {
		t.Errorf("resolveClock returned a different Clock than the one installed")
	}
}

// TestTickClock_WithTestClock_RestoreCleansUp asserts that the restore func
// returned by withTestClock reverts testClock to its previous value.
func TestTickClock_WithTestClock_RestoreCleansUp(t *testing.T) {
	// Ensure clean baseline.
	testClock = nil

	fc := clock.NewFakeClock(time.Unix(0, 0))
	restore := withTestClock(fc)

	if got := resolveClock(); got == nil {
		t.Fatalf("pre-restore: resolveClock = nil; want injected FakeClock")
	}

	restore()

	if got := resolveClock(); got != nil {
		t.Errorf("post-restore: resolveClock = %v; want nil (restored to baseline)", got)
	}
}

// TestTickClock_WithTestClock_StackedRestores asserts LIFO-cleanup for
// nested withTestClock invocations.
func TestTickClock_WithTestClock_StackedRestores(t *testing.T) {
	testClock = nil // baseline

	fc1 := clock.NewFakeClock(time.Unix(0, 0))
	restore1 := withTestClock(fc1)
	if got := resolveClock(); got != clock.Clock(fc1) {
		t.Fatalf("after restore1 install: resolveClock != fc1")
	}

	fc2 := clock.NewFakeClock(time.Unix(100, 0))
	restore2 := withTestClock(fc2)
	if got := resolveClock(); got != clock.Clock(fc2) {
		t.Fatalf("after restore2 install: resolveClock != fc2")
	}

	// LIFO unwind.
	restore2()
	if got := resolveClock(); got != clock.Clock(fc1) {
		t.Errorf("after restore2(): resolveClock = %v; want fc1 (LIFO)", got)
	}
	restore1()
	if got := resolveClock(); got != nil {
		t.Errorf("after restore1(): resolveClock = %v; want nil (baseline)", got)
	}
}
