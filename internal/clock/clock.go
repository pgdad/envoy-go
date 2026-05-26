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
//   - Clock interface (Now + After) — the After channel-based API matches
//     stdlib `time.After`; `case <-clk.After(d):` reads idiomatically in a
//     select-based goroutine loop (the 25.2 tick.go pattern).
//   - RealClock production wiring (zero-value usable; ~5 LoC of code).
//   - FakeClock test helper exported for use by every co-consumer's test
//     fixtures (e.g., `internal/wasm/tick_test.go`). FakeClock fires
//     pending After-channels at Advance time in deadline-ascending order,
//     ties broken by insertion order (deterministic).
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

// FakeClock is the test-scope implementation of Clock that step-drives time
// deterministically. Tests construct a FakeClock via NewFakeClock(start) +
// drive time via Advance(d). After-channels registered before Advance fire
// in deadline-ascending order (ties broken by insertion order — per the
// phase-21 planner-time D9 determinism contract).
//
// # Single-Advance-caller discipline
//
// FakeClock tolerates concurrent After() registrations from multiple
// goroutines (the mu serializes them), but Advance() is intended to be
// called from a single test-driver goroutine. Tests that need
// barrier-style synchronization between Advance + the goroutine consuming
// the After-channel must implement their own sync (e.g., a done-channel
// the consumer closes after handling the fire).
type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	pending []*fakeAfter
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

// Advance moves time forward by d and fires all pending After-channels
// whose deadlines fall within [old now, old now + d] in deadline-ascending
// order (ties broken by insertion order). The fires are non-blocking sends
// on cap=1 buffered channels — if a receiver has already dropped the
// channel reference the send still succeeds (slot accepts; channel is GC'd
// when receiver drops). After firing, the entries are removed from pending.
//
// Re-entrancy: a fire callback that synchronously registers a NEW After()
// is supported — the new entry appears in pending AFTER the Advance
// snapshot, so it does NOT fire in the same Advance call (matches the
// monotonic-time semantic; tests that need re-fire-in-same-Advance must
// chain Advance calls).
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	deadline := c.now
	// Snapshot fires under mu; release before sending to avoid sender-side
	// blocking if a future consumer holds mu (defensive — the current
	// channels are cap=1 buffered so sends won't block, but the
	// snapshot-then-fire-outside-mu pattern is robust against future
	// changes).
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
}
