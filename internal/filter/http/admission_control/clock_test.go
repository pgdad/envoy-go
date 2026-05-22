package admission_control

// clock_test.go — test-scope fakeClock implementation + determinism tests per
// phase-23 IMPL Task 3. Provides the step-driven fake clock consumed by the
// controller's FAKE-TIME window tests (Task 4).
//
// fakeClock is the deterministic test-scope implementation of the Clock interface.
// It exposes Advance(d) to step time forward. Phase-23's fakeClock is simpler
// than phase-21's: no AfterFunc, no timers, no Stop handle — phase-23's Clock
// is Now()-ONLY per SPEC §3.2.
//
// fakeClock lives ONLY in this _test.go file — the production binary links
// defaultClock from clock.go. Task 3 OWNS the fakeClock scaffolding; the
// controller tests (Task 4) consume Advance to drive per-second bucket rollover
// and stale-purge in the FAKE-TIME window tests.

import (
	"sync"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// fakeClock — deterministic test-scope Clock implementation.
// -----------------------------------------------------------------------------

// fakeClock is the test-scope implementation of Clock that step-drives time
// deterministically. The documented single-Advance-caller discipline means tests
// MUST serialize their Advance calls; Now is safe for concurrent reads (mu guards
// the now field).
//
// Unlike phase-21's fakeClock, there are NO timers, NO AfterFunc, NO Stop handle
// — phase-23's Clock interface is Now()-ONLY (per SPEC §3.2). The controller
// calls Now() on every recordRequest + requestCounts call to compare bucket
// timestamps and drive rollover/expiry.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// newFakeClock returns a fakeClock anchored at start. The zero-time start
// (time.Time{}) is permitted; tests may use time.Unix(0,0) or a calendar time
// per their preference.
func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

// Now returns the current step-clock time under mu. Satisfies the Clock interface.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves time forward by d. Safe for single-caller test serialization.
// Controller tests call Advance to simulate the passage of time between
// recordRequest calls and verify bucket rollover + stale-purge behavior.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Compile-time interface satisfaction check.
var _ Clock = (*fakeClock)(nil)

// -----------------------------------------------------------------------------
// fakeClock determinism tests.
// -----------------------------------------------------------------------------

// TestFakeClock_Now_ReflectsStart verifies that the constructor anchors Now()
// to the provided start time.
func TestFakeClock_Now_ReflectsStart(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newFakeClock(start)
	if got := c.Now(); !got.Equal(start) {
		t.Errorf("Now() = %v; want %v", got, start)
	}
}

// TestFakeClock_Advance_MovesNow verifies that Advance moves the clock forward.
func TestFakeClock_Advance_MovesNow(t *testing.T) {
	c := newFakeClock(time.Unix(0, 0))
	c.Advance(1 * time.Second)
	want := time.Unix(1, 0)
	if got := c.Now(); !got.Equal(want) {
		t.Errorf("Now() after Advance(1s) = %v; want %v", got, want)
	}
}

// TestFakeClock_Advance_Cumulative verifies that multiple Advance calls are
// cumulative — each call adds to the current time, not to a base.
func TestFakeClock_Advance_Cumulative(t *testing.T) {
	c := newFakeClock(time.Unix(0, 0))
	c.Advance(1 * time.Second)
	c.Advance(2 * time.Second)
	c.Advance(500 * time.Millisecond)
	want := time.Unix(3, int64(500*time.Millisecond))
	if got := c.Now(); !got.Equal(want) {
		t.Errorf("Now() after cumulative Advance = %v; want %v", got, want)
	}
}

// TestFakeClock_Advance_Zero verifies that Advance(0) is a no-op.
func TestFakeClock_Advance_Zero(t *testing.T) {
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	c := newFakeClock(start)
	c.Advance(0)
	if got := c.Now(); !got.Equal(start) {
		t.Errorf("Now() after Advance(0) = %v; want %v (unchanged)", got, start)
	}
}

// TestFakeClock_Now_Deterministic verifies that successive Now() calls return
// the same value when Advance has not been called between them.
func TestFakeClock_Now_Deterministic(t *testing.T) {
	start := time.Unix(42, 0)
	c := newFakeClock(start)
	for i := 0; i < 5; i++ {
		if got := c.Now(); !got.Equal(start) {
			t.Errorf("call %d: Now() = %v; want %v", i, got, start)
		}
	}
}

// TestFakeClock_Advance_SubSecond verifies sub-second advances work correctly
// (the controller uses 1s granularity but the clock itself must handle any
// duration faithfully).
func TestFakeClock_Advance_SubSecond(t *testing.T) {
	c := newFakeClock(time.Unix(0, 0))
	c.Advance(500 * time.Millisecond)
	c.Advance(500 * time.Millisecond)
	want := time.Unix(1, 0)
	if got := c.Now(); !got.Equal(want) {
		t.Errorf("Now() after two 500ms advances = %v; want %v", got, want)
	}
}

// -----------------------------------------------------------------------------
// defaultClock sanity test — verifies the production wiring compiles and runs.
// -----------------------------------------------------------------------------

// TestDefaultClock_Sanity verifies that defaultClock satisfies the Clock interface
// and that Now() returns a non-zero time, and that two successive calls are
// monotonically non-decreasing. This is a smoke test for the time.Now wiring.
func TestDefaultClock_Sanity(t *testing.T) {
	// Compile-time interface satisfaction.
	var _ Clock = defaultClock{}

	dc := defaultClock{}
	first := dc.Now()
	if first.IsZero() {
		t.Error("defaultClock.Now() returned zero time; wiring suspect")
	}
	second := dc.Now()
	if second.Before(first) {
		t.Errorf("defaultClock.Now() not monotonically non-decreasing: first=%v second=%v", first, second)
	}
}

// TestFakeClock_BucketBoundary verifies the clock's precision at the 1-second
// bucket boundary — the exact tick the controller uses for rollover/expiry.
// Advancing exactly 1s from the bucket start puts Now() at the next bucket
// boundary.
func TestFakeClock_BucketBoundary(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newFakeClock(start)

	// One second before the next boundary — just inside the current bucket.
	c.Advance(999 * time.Millisecond)
	want := start.Add(999 * time.Millisecond)
	if got := c.Now(); !got.Equal(want) {
		t.Errorf("Now() at 999ms = %v; want %v", got, want)
	}

	// Exactly at the 1-second boundary.
	c.Advance(1 * time.Millisecond)
	want = start.Add(1 * time.Second)
	if got := c.Now(); !got.Equal(want) {
		t.Errorf("Now() at 1s boundary = %v; want %v", got, want)
	}
}
