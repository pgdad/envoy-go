package adaptive_concurrency

// clock_test.go — test-scope fakeClock implementation + determinism tests per
// phase-21 IMPL Task 3 + planner-time D9 (fakeClock API LOCKED here: Now +
// AfterFunc + Advance; deadline-ascending synchronous fire; documented
// single-Advance-caller discipline). Closes SPEC §12 item B6 (fakeClock
// timer-fire determinism under multi-timer same-tick) per planner-time D15.
//
// The fakeClock lives ONLY in this _test.go file — the production binary
// links defaultClock from clock.go. fakeClock satisfies the Clock interface
// at clock.go via Now + AfterFunc; the test-only Advance method drives the
// step-clock per FAKE-TIME differential strategy per ADR-0186 §Decision +
// SPEC §14.1 Layer A.

import (
	"sort"
	"sync"
	"testing"
	"time"
)

// fakeClock is the test-scope implementation of Clock that step-drives time
// deterministically per planner-time D9. The documented single-Advance-caller
// discipline means tests MUST serialize their Advance calls; concurrent fires
// from goroutines that synchronously register more AfterFunc handlers via
// their callbacks are tolerated via the internal mu (AfterFunc + Now are
// serialized; the callback fires while mu is RELEASED so re-entrant
// AfterFunc registrations from inside a callback succeed without deadlock).
//
// SPEC §12 item B6 closes here: the fakeClock fires timers in deadline-asc
// order; ties broken by insertion order via the sort.SliceStable in Advance.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer // unsorted while AfterFunc accumulates; sorted at Advance
}

// fakeTimer is the test-scope timer registered in fakeClock. The stopped +
// fired bools are guarded by the parent fakeClock.mu when read/written from
// Advance + AfterFunc; the Stop method reads them under mu via the parent.
//
// Per the Stop interface contract (clock.go), Stop returns true if the call
// stopped the timer (the fn will NOT fire), false if the timer had already
// fired or been stopped.
type fakeTimer struct {
	parent   *fakeClock
	deadline time.Time
	fn       func()
	stopped  bool
	fired    bool
	seq      uint64 // insertion-order tiebreaker for same-deadline timers
}

// newFakeClock returns a fakeClock anchored at start. The zero-time start
// (time.Time{}) is permitted; tests that need a more-realistic time use
// time.Unix(0,0) or time.Date(2026, 1, 1, ...) per their preference.
func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

// Now returns the current step-clock time under the parent mu.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// AfterFunc schedules fn to fire at the next Advance call whose new "now"
// covers c.now + d. Returns a *fakeTimer (which satisfies the Stop interface
// at clock.go).
//
// The fn is called SYNCHRONOUSLY from Advance — see Advance's doc-comment for
// the re-entrancy contract.
func (c *fakeClock) AfterFunc(d time.Duration, fn func()) Stop {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{
		parent:   c,
		deadline: c.now.Add(d),
		fn:       fn,
		seq:      uint64(len(c.timers)), // monotone insertion sequence
	}
	c.timers = append(c.timers, t)
	return t
}

// Advance moves time forward by d and synchronously fires all timers whose
// deadlines fall within [old now, old now + d] in deadline-ascending order
// (ties broken by insertion-sequence per planner-time D9 determinism).
//
// Re-entrancy: timer callbacks may register more timers via AfterFunc. Those
// re-entrant registrations are appended to c.timers; if their deadline falls
// within the same Advance window (e.g., AfterFunc(0, ...) called from inside
// a callback), they are picked up in the SAME Advance call. The loop drains
// until no more ready timers remain (per the upstream Envoy + Go-stdlib
// time.AfterFunc(0, ...) convention).
//
// Callback fire is performed with c.mu RELEASED so re-entrant AfterFunc
// registrations from the callback do not deadlock. The fired/stopped flags
// are mutated UNDER mu before the callback fires; observers of the fakeTimer
// from other goroutines (e.g., a Stop call from the controller's own re-arm
// path) see a consistent fired==true ⇒ Stop returns false.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	deadline := c.now
	c.mu.Unlock()
	// Drain loop — keeps firing while new ready timers appear (re-entrant
	// AfterFunc(0, ...) inside a callback fires in the same Advance pass).
	for {
		c.mu.Lock()
		// Collect all ready timers whose deadline ≤ current "now".
		var ready []*fakeTimer
		var remaining []*fakeTimer
		for _, t := range c.timers {
			if t.stopped || t.fired {
				continue
			}
			if !t.deadline.After(deadline) {
				ready = append(ready, t)
			} else {
				remaining = append(remaining, t)
			}
		}
		c.timers = remaining
		if len(ready) == 0 {
			c.mu.Unlock()
			return
		}
		// Sort ready timers: deadline-asc; insertion-seq-asc on tie. Stable
		// so equal-key entries preserve their original insertion order per
		// planner-time D9.
		sort.SliceStable(ready, func(i, j int) bool {
			if ready[i].deadline.Equal(ready[j].deadline) {
				return ready[i].seq < ready[j].seq
			}
			return ready[i].deadline.Before(ready[j].deadline)
		})
		// Mark fired under mu BEFORE invoking fn so re-entrant Stop calls
		// from inside the callback observe fired==true.
		for _, t := range ready {
			t.fired = true
		}
		c.mu.Unlock()
		// Fire callbacks with mu released; re-entrant AfterFunc registrations
		// inside a callback safely reacquire mu.
		for _, t := range ready {
			if t.fn != nil {
				t.fn()
			}
		}
		// Loop — re-entrant registrations with deadline ≤ "now" picked up
		// in the next iteration.
	}
}

// Stop cancels the timer per the Stop interface contract: returns true if the
// call stopped the timer (the fn will NOT fire); false if the timer had
// already fired or been stopped. The mu is acquired on the parent fakeClock
// to serialize against Advance + AfterFunc.
func (t *fakeTimer) Stop() bool {
	t.parent.mu.Lock()
	defer t.parent.mu.Unlock()
	if t.stopped || t.fired {
		return false
	}
	t.stopped = true
	return true
}

// -----------------------------------------------------------------------------
// fakeClock determinism tests (closes SPEC §12 item B6 per D15).
// -----------------------------------------------------------------------------

// TestFakeClock_Now_ReflectsStart verifies the constructor anchor.
func TestFakeClock_Now_ReflectsStart(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newFakeClock(start)
	if got := c.Now(); !got.Equal(start) {
		t.Errorf("Now() = %v; want %v", got, start)
	}
}

// TestFakeClock_Advance_MovesNow verifies the step-clock advance.
func TestFakeClock_Advance_MovesNow(t *testing.T) {
	c := newFakeClock(time.Unix(0, 0))
	c.Advance(100 * time.Millisecond)
	want := time.Unix(0, int64(100*time.Millisecond))
	if got := c.Now(); !got.Equal(want) {
		t.Errorf("Now() after Advance(100ms) = %v; want %v", got, want)
	}
}

// TestFakeClock_AfterFunc_FiresAtDeadline verifies the basic timer-fire path.
func TestFakeClock_AfterFunc_FiresAtDeadline(t *testing.T) {
	c := newFakeClock(time.Unix(0, 0))
	var fired bool
	c.AfterFunc(50*time.Millisecond, func() { fired = true })
	c.Advance(49 * time.Millisecond)
	if fired {
		t.Errorf("fired prematurely at +49ms")
	}
	c.Advance(1 * time.Millisecond) // total 50ms
	if !fired {
		t.Errorf("did NOT fire at +50ms")
	}
}

// TestFakeClock_AfterFunc_DoesNotFireBefore confirms strict deadline boundary.
func TestFakeClock_AfterFunc_DoesNotFireBefore(t *testing.T) {
	c := newFakeClock(time.Unix(0, 0))
	var fired bool
	c.AfterFunc(100*time.Millisecond, func() { fired = true })
	c.Advance(99 * time.Millisecond)
	if fired {
		t.Errorf("fired at +99ms (deadline was 100ms)")
	}
}

// TestFakeClock_Stop_PreventsFire verifies cancel semantics.
func TestFakeClock_Stop_PreventsFire(t *testing.T) {
	c := newFakeClock(time.Unix(0, 0))
	var fired bool
	s := c.AfterFunc(50*time.Millisecond, func() { fired = true })
	if !s.Stop() {
		t.Errorf("Stop() returned false on first call (should return true)")
	}
	c.Advance(100 * time.Millisecond)
	if fired {
		t.Errorf("fired after Stop()")
	}
}

// TestFakeClock_Stop_AfterFireReturnsFalse verifies the post-fire Stop branch.
func TestFakeClock_Stop_AfterFireReturnsFalse(t *testing.T) {
	c := newFakeClock(time.Unix(0, 0))
	s := c.AfterFunc(10*time.Millisecond, func() {})
	c.Advance(10 * time.Millisecond)
	if s.Stop() {
		t.Errorf("Stop() after fire returned true; want false")
	}
}

// TestFakeClock_MultiTimer_DeterministicOrder closes SPEC §12 item B6: 3
// timers registered at distinct intervals; verify they fire in deadline-asc
// order when Advance covers all 3.
func TestFakeClock_MultiTimer_DeterministicOrder(t *testing.T) {
	c := newFakeClock(time.Unix(0, 0))
	var order []int
	c.AfterFunc(30*time.Millisecond, func() { order = append(order, 30) })
	c.AfterFunc(10*time.Millisecond, func() { order = append(order, 10) })
	c.AfterFunc(20*time.Millisecond, func() { order = append(order, 20) })
	c.Advance(100 * time.Millisecond)
	want := []int{10, 20, 30}
	if len(order) != 3 {
		t.Fatalf("len(order) = %d; want 3", len(order))
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %d; want %d (full order=%v)", i, order[i], want[i], order)
		}
	}
}

// TestFakeClock_MultiTimer_SameDeadlineInsertionOrder verifies tie-break
// determinism per planner-time D9: same-deadline timers fire in registration
// order.
func TestFakeClock_MultiTimer_SameDeadlineInsertionOrder(t *testing.T) {
	c := newFakeClock(time.Unix(0, 0))
	var order []string
	c.AfterFunc(10*time.Millisecond, func() { order = append(order, "first") })
	c.AfterFunc(10*time.Millisecond, func() { order = append(order, "second") })
	c.AfterFunc(10*time.Millisecond, func() { order = append(order, "third") })
	c.Advance(10 * time.Millisecond)
	if len(order) != 3 {
		t.Fatalf("len(order) = %d; want 3", len(order))
	}
	want := []string{"first", "second", "third"}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %q; want %q (full order=%v)", i, order[i], want[i], order)
		}
	}
}

// TestFakeClock_ReentrantAfterFunc verifies that re-entrant AfterFunc calls
// from inside a callback are safe (no deadlock) and pick up at the next
// Advance pass — or in the SAME pass if the new deadline is ≤ current "now"
// (the AfterFunc(0, ...) self-rearm pattern used by the 5-consecutive-min
// forced-recalc trigger per AMEND-2 C3).
func TestFakeClock_ReentrantAfterFunc(t *testing.T) {
	c := newFakeClock(time.Unix(0, 0))
	var fires int
	var s Stop
	s = c.AfterFunc(10*time.Millisecond, func() {
		fires++
		// Re-arm at 0 (immediate) — matches the AMEND-2 C3 force-arm pattern.
		// The new timer must fire in the SAME Advance pass.
		if fires < 3 {
			s = c.AfterFunc(0, func() {
				fires++
				if fires < 3 {
					s = c.AfterFunc(0, func() { fires++ })
				}
			})
		}
	})
	_ = s // silence unused
	c.Advance(10 * time.Millisecond)
	if fires != 3 {
		t.Errorf("re-entrant AfterFunc chain fired %d times; want 3", fires)
	}
}
