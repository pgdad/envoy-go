// Package clock provides a time-source seam for deterministic fake-time testing
// of time-driven framework components per phase-21 SPEC §3.1 + §6.3 +
// ADR-0186 §Decision + §Consequences (g) EXTRACT-NOW trigger.
//
// # Why this package exists (the EXTRACT-NOW trigger fires at 25.2)
//
// Phase-21 IMPL kept the Clock seam INLINE at
// internal/filter/http/adaptive_concurrency/clock.go per ADR-0186 §Decision
// (consumer count at introduction time = 1; the phase-17 jwt_authn
// EXTRACT-NOW-only-when-trigger-fires lesson). ADR-0186 §Consequences (g)
// pinned the forward-pointer: "when a second timer-driven filter materializes
// (admission_control, global rate limit, or similar §9 row), extract to
// internal/clock/ ...". Phase 25.2 IMPL Task 5 (wasm tick goroutine per Q5 +
// R-25.2-9 + ADR-0205) IS that second consumer — the per-`*RootVM` tick
// dispatcher needs Clock-seam injection at NewRootVM time via
// `WithRootClock(clk)` for fixture fake-time support.
//
// This package RATIFIES the phase-21 extraction discipline at the
// second-or-later co-consumer scope. The existing inline
// adaptive_concurrency/clock.go does NOT migrate at 25.2 — phase-21
// ADR-0186 §Consequences (g)'s "the consumer's Clock-typed field unchanged"
// pattern means the migration can happen at the consumer's leisure (a
// follow-up refactor); the framework-level Clock seam is what 25.2 needs.
//
// # Surface
//
//   - Clock interface (Now + After + AfterFunc) — the superset seam shared by
//     BOTH timer-driven consumers:
//   - After is the CHANNEL-based tick consumer (the 25.2 wasm tick.go
//     pattern). `case <-clk.After(d):` reads idiomatically in a
//     select-based goroutine loop.
//   - AfterFunc is the CALLBACK-based consumer (the phase-21
//     adaptive_concurrency gradientController). It returns a Stop handle
//     mirroring time.Timer.Stop semantics.
//   - Stop interface — the per-timer cancellation handle returned by AfterFunc.
//   - RealClock production wiring (zero-value usable; ~5 LoC of code).
//   - FakeClock test helper exported for use by every co-consumer's test
//     fixtures (e.g., `internal/wasm/tick_test.go`,
//     `internal/filter/http/adaptive_concurrency/*_test.go`). FakeClock fires
//     pending After-channels AND AfterFunc callbacks at Advance time in
//     deadline-ascending order, ties broken by insertion order (deterministic).
//
// # Goroutine-safety
//
// Both RealClock + FakeClock are safe for concurrent use. RealClock is
// stateless (delegates to time.Now + time.After). FakeClock guards its
// internal pending list + now field via sync.Mutex.
package clock

import (
	"sort"
	"sync"
	"time"
)

// Clock is the time-source seam used by time-driven framework components
// (the 25.2 wasm tick goroutine; future co-consumers). The production wiring
// uses RealClock (zero-value usable; delegates to time.Now + time.After);
// tests inject FakeClock for deterministic step-driven time control.
type Clock interface {
	// Now returns the current time per the time-source convention. Wall-clock
	// in production; step-driven in fake-time tests.
	Now() time.Time

	// After returns a channel that fires once with the current time after d.
	// Matches stdlib `time.After(d)` shape so consumers can write idiomatic
	// `case <-clk.After(d): ...` select-loop bodies. The returned channel
	// has buffer 1 in BOTH RealClock + FakeClock — receivers that lose the
	// race against a cancel signal do NOT cause a goroutine leak at the
	// sender (the buffered slot accepts the send + the channel is GC'd
	// when the receiver drops the reference).
	After(d time.Duration) <-chan time.Time

	// AfterFunc schedules fn to fire after d and returns a Stop handle that can
	// cancel a pending fire (mirrors time.AfterFunc + time.Timer.Stop). This is
	// the CALLBACK-based seam consumed by the phase-21 adaptive_concurrency
	// gradientController. Caller-thread-safe in both implementations.
	AfterFunc(d time.Duration, fn func()) Stop
}

// Stop is the per-timer cancellation handle returned by Clock.AfterFunc.
// Mirrors time.Timer.Stop semantics: returns true if the call stopped the
// timer (the fn will NOT fire); false if the timer had already expired or
// been stopped.
type Stop interface {
	Stop() bool
}

// RealClock is the production wiring of Clock — delegates to stdlib time.Now
// + time.After. The zero value is usable (no state).
type RealClock struct{}

// Now returns time.Now().
func (RealClock) Now() time.Time { return time.Now() }

// After returns time.After(d).
//
// NOTE: stdlib `time.After` allocates a new *time.Timer per call; the timer
// is NOT eligible for GC until it fires (Go runtime keeps the underlying
// timer heap entry live). For tight tick loops with large periods + frequent
// cancellation this can accumulate. Consumers MAY switch to a more careful
// time.NewTimer + Stop pattern if measurements show pressure; for the 25.2
// wasm tick goroutine (one timer in-flight per RootVM at a time) the
// time.After cost is amortized.
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// AfterFunc wraps time.AfterFunc(d, fn) and returns a Stop adapter over the
// resulting *time.Timer.
func (RealClock) AfterFunc(d time.Duration, fn func()) Stop {
	return realTimerStop{t: time.AfterFunc(d, fn)}
}

// realTimerStop adapts *time.Timer to the Stop interface.
type realTimerStop struct {
	t *time.Timer
}

func (s realTimerStop) Stop() bool { return s.t.Stop() }

// FakeClock is the test-scope implementation of Clock that step-drives time
// deterministically. Tests construct a FakeClock via NewFakeClock(start) +
// drive time via Advance(d). Both After-channels AND AfterFunc callbacks
// registered before Advance fire in deadline-ascending order (ties broken by
// insertion order — per the phase-21 planner-time D9 determinism contract).
// After + AfterFunc share the SAME nextSeq insertion-order counter.
//
// # Single-Advance-caller discipline
//
// FakeClock tolerates concurrent After()/AfterFunc() registrations from
// multiple goroutines (the mu serializes them), but Advance() is intended to
// be called from a single test-driver goroutine. Tests that need
// barrier-style synchronization between Advance + the goroutine consuming
// the After-channel must implement their own sync (e.g., a done-channel
// the consumer closes after handling the fire).
type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	pending []*fakeAfter
	timers  []*fakeTimer
	nextSeq uint64
}

// fakeAfter is the pending After registration. ch is the buffered output
// channel (cap=1) returned to the After() caller; deadline is the absolute
// fire time; seq is the insertion-order tiebreaker for same-deadline fires.
type fakeAfter struct {
	ch       chan time.Time
	deadline time.Time
	seq      uint64
}

// fakeTimer is the pending AfterFunc registration (the CALLBACK seam). The
// stopped + fired bools are guarded by the parent FakeClock.mu when
// read/written from Advance + AfterFunc; the Stop method reads them under the
// parent's mu. Per the Stop interface contract, Stop returns true iff it
// stopped a not-yet-fired, not-already-stopped timer.
type fakeTimer struct {
	parent   *FakeClock
	deadline time.Time
	fn       func()
	seq      uint64
	stopped  bool
	fired    bool
}

// Stop cancels the timer per the Stop interface contract: returns true if the
// call stopped the timer (the fn will NOT fire); false if the timer had
// already fired or been stopped. mu is acquired on the parent FakeClock to
// serialize against Advance + AfterFunc.
func (t *fakeTimer) Stop() bool {
	t.parent.mu.Lock()
	defer t.parent.mu.Unlock()
	if t.stopped || t.fired {
		return false
	}
	t.stopped = true
	return true
}

// NewFakeClock returns a FakeClock anchored at start. The zero-time start
// (time.Time{}) is permitted; tests that need a more-realistic time use
// time.Unix(0,0) or time.Date(2026, 1, 1, ...) per their preference.
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{now: start}
}

// Now returns the current step-clock time under mu.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// PendingLen returns the number of pending After-registrations awaiting
// Advance. Test-observability seam used by consumers that need to
// synchronize an Advance against an in-flight goroutine's re-registration
// of a follow-up After (e.g., the 25.2 wasm tick goroutine's per-tick
// re-arm — see internal/wasm/tick_test.go's advanceForNTicks helper).
//
// Returns 0 when no After is currently pending; non-zero otherwise. The
// caller is responsible for the wait-loop discipline (poll PendingLen
// until it reaches the expected count, then Advance).
func (c *FakeClock) PendingLen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

// After returns a buffered (cap=1) channel that fires at the next Advance
// call whose new "now" covers c.now + d. Matches stdlib time.After shape.
//
// The d <= 0 case fires synchronously inside After (the channel is buffered
// so the send does NOT block); matches stdlib semantics where time.After(0)
// fires "immediately" (the receiver may still need to drain the channel to
// observe the fire).
func (c *FakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		// Immediate fire — d=0 or negative duration matches stdlib.
		ch <- c.now
		return ch
	}
	c.pending = append(c.pending, &fakeAfter{
		ch:       ch,
		deadline: c.now.Add(d),
		seq:      c.nextSeq,
	})
	c.nextSeq++
	return ch
}

// AfterFunc schedules fn to fire at the next Advance call whose new "now"
// covers c.now + d. Returns a *fakeTimer (which satisfies the Stop interface).
// The fn is called SYNCHRONOUSLY from Advance — see Advance's doc-comment for
// the re-entrancy contract.
//
// A d <= 0 registers the timer at deadline == c.now; it fires at the next
// Advance (and an AfterFunc(0, ...) registered from inside a firing callback
// fires in the SAME Advance pass via the callback drain loop — the phase-21
// AMEND-2 C3 force-arm pattern).
//
// NOTE: unlike After(d<=0), which fires synchronously before returning (the
// buffered channel is sent to immediately under mu), AfterFunc(d<=0) cannot
// fire synchronously — invoking fn() under mu would deadlock any re-entrant
// AfterFunc/After call the callback makes. The timer is therefore registered
// at deadline==now and fires on the NEXT Advance call.
func (c *FakeClock) AfterFunc(d time.Duration, fn func()) Stop {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{
		parent:   c,
		deadline: c.now.Add(d),
		fn:       fn,
		seq:      c.nextSeq,
	}
	c.timers = append(c.timers, t)
	c.nextSeq++
	return t
}

// Advance moves time forward by d and fires both seams whose deadlines fall
// within [old now, old now + d]:
//
//  1. CHANNEL block — pending After-channels, in deadline-ascending order
//     (ties broken by insertion seq). Fires are non-blocking sends on cap=1
//     buffered channels — if a receiver has already dropped the channel
//     reference the send still succeeds (slot accepts; channel GC'd when
//     receiver drops). Snapshot-once: a callback/goroutine that registers a
//     NEW After() during this Advance does NOT fire in the same Advance call
//     (matches the monotonic-time semantic; tests needing re-fire chain
//     Advance calls). This block is UNCHANGED from the 25.2 tick contract.
//
//  2. CALLBACK block — pending AfterFunc timers, in deadline-ascending order
//     (ties broken by insertion seq). Each fn() is called SYNCHRONOUSLY with
//     mu RELEASED so re-entrant AfterFunc/After registrations from inside a
//     callback do not deadlock. fired=true is marked UNDER mu BEFORE the
//     callback runs, so a Stop() racing a fire observes fired==true ⇒ returns
//     false. This block uses a DRAIN LOOP: an AfterFunc(0, ...) self-rearm
//     registered from inside a firing callback (deadline ≤ new now) fires in
//     the SAME Advance pass (the phase-21 AMEND-2 C3 force-arm pattern).
//     Drain-loop termination relies on callbacks NOT unconditionally re-arming
//     at deadline ≤ now on every fire; a callback that always re-arms itself
//     at d=0 regardless of state would spin the drain loop forever (a
//     usage-contract violation, not a FakeClock defect).
//
// The two blocks are kept separate; the channel block runs first and is not
// affected by the callback drain loop.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	deadline := c.now
	// CHANNEL block: snapshot fires under mu; release before sending.
	var fires []*fakeAfter
	var keep []*fakeAfter
	for _, p := range c.pending {
		if !p.deadline.After(deadline) {
			fires = append(fires, p)
		} else {
			keep = append(keep, p)
		}
	}
	c.pending = keep
	c.mu.Unlock()

	// Sort fires: deadline-asc; insertion-seq-asc on tie. Stable so equal-key
	// entries preserve their original insertion order.
	sort.SliceStable(fires, func(i, j int) bool {
		if fires[i].deadline.Equal(fires[j].deadline) {
			return fires[i].seq < fires[j].seq
		}
		return fires[i].deadline.Before(fires[j].deadline)
	})

	for _, p := range fires {
		// Buffered cap=1 channel; non-blocking send. If the receiver has
		// dropped the reference (e.g., goroutine selected the stop signal
		// instead), the send still succeeds (slot accepts; channel GC'd
		// when the receiver releases).
		p.ch <- p.deadline
	}

	// CALLBACK block: drain loop — keeps firing while new ready timers appear
	// (re-entrant AfterFunc(0, ...) inside a callback fires in the same pass).
	for {
		c.mu.Lock()
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
		// Sort ready timers: deadline-asc; insertion-seq-asc on tie.
		sort.SliceStable(ready, func(i, j int) bool {
			if ready[i].deadline.Equal(ready[j].deadline) {
				return ready[i].seq < ready[j].seq
			}
			return ready[i].deadline.Before(ready[j].deadline)
		})
		// Mark fired UNDER mu BEFORE invoking fn so re-entrant Stop calls from
		// inside the callback observe fired==true.
		for _, t := range ready {
			t.fired = true
		}
		c.mu.Unlock()
		// Fire callbacks with mu released.
		for _, t := range ready {
			if t.fn != nil {
				t.fn()
			}
		}
	}
}
