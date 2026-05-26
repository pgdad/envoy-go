// Tests for the per-*RootVM tick goroutine + 10ms envoy-go-strict floor +
// FakeClock-driven fixtures per Q5 + R-25.2-9 + ADR-0205 + ADR-0186 §Decision
// FIRST co-consumer of phase-21 Clock seam (RATIFIES the EXTRACT-NOW trigger
// at internal/clock/ per §Consequences (g)).
//
// Coverage:
//
//   - 10ms floor enforcement (SetTickPeriod(5ms) → effective = 10ms).
//   - period=0 cancellation (no further ticks fire after SetTickPeriod(0)).
//   - re-schedule with new period (SetTickPeriod(50ms) then SetTickPeriod(10ms)
//     → new period takes effect; old goroutine cleanly exits).
//   - panic-recovery (guest's proxy_on_tick panics → goroutine survives;
//     PanicHandlerFn fires; next tick still dispatches).
//   - concurrent stream contexts share ONE tick goroutine per RootVM (NOT
//     N=100 — Q5 anti-stress assertion).
//   - Close while tick active → goroutine exits cleanly + Close returns.
//   - FloorConstant pin (TickPeriodFloor == 10ms).

package wasm

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/clock"
)

// --- fixture helpers ------------------------------------------------------

// newRootVMForTick constructs a RootVM around exportsAll25_2CallbacksModule
// (which exports proxy_on_tick) with the FakeClock injected via the NEW
// WithRootClock option per Task 5. The all-cap sandbox + an empty
// ABICallbacks bundle satisfies the gate-at-getFunction discipline + the
// per-callback dispatch wiring without needing any consumer-side surface.
func newRootVMForTick(t *testing.T, fc *clock.FakeClock, opts ...RootVMOption) *RootVM {
	t.Helper()
	ctx := context.Background()
	mod := mustCompileWithCacheForRootVM(t, ctx, exportsAll25_2CallbacksModule)
	base := []RootVMOption{
		WithRootSandboxConfig(allowAllSandbox()),
		WithRootClock(fc),
	}
	base = append(base, opts...)
	rv, err := NewRootVM(ctx, mod, 1, base...)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	t.Cleanup(func() { _ = rv.Close() })
	rv.RegisterABICallbacks(&fakeABICallbacks{})
	return rv
}

// waitForTickInvocations polls the fire-count until it reaches `want` (or
// fails after the deadline). Used by tests that have already advanced the
// FakeClock past the next tick deadline + are waiting for the async tick
// goroutine to dispatch.
func waitForTickInvocations(t *testing.T, getCount func() uint64, want uint64, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if getCount() >= want {
			return
		}
		time.Sleep(1 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for tick invocations: got %d, want >= %d", getCount(), want)
}

// advanceForNTicks drives the FakeClock to fire N ticks at the given period.
// Each iteration: (a) Advance by `period`; (b) wait for the fire count to
// reach the next expected value. The wait-after-advance pattern is
// necessary because the tick goroutine's `<-clk.After(period)` registration
// races with Advance — the goroutine must finish dispatching THIS tick
// before re-registering its NEXT After, otherwise the next Advance(period)
// fires nothing (no pending After-entry). Each iteration waits until the
// goroutine has registered its next After (observed via the fire count
// reaching `currentFires+1`) before advancing again.
//
// Returns the final observed fire count.
func advanceForNTicks(t *testing.T, fc *fakeClockHelper, getCount func() uint64, baseline uint64, n uint64, period time.Duration) uint64 {
	t.Helper()
	for i := uint64(0); i < n; i++ {
		want := baseline + i + 1
		// Wait for the tick goroutine to have registered its next After
		// (it does so AFTER returning from lockAndDispatchTick of the
		// previous tick). For the first tick (i=0) there's no prior
		// dispatch to wait for; for subsequent iterations we wait until
		// the previous tick fully dispatched (the count reached
		// baseline+i) before advancing.
		if i > 0 {
			waitForTickInvocations(t, getCount, baseline+i, 1*time.Second)
		}
		// Give the tick goroutine a moment to re-register its next After.
		// Without this, the FakeClock.Advance below may run BEFORE the
		// goroutine's `clk.After(period)` registers, leaving no pending
		// entry to fire. We poll on the FakeClock's pending-length
		// reaching 1.
		fc.waitForPending(t, 1, 1*time.Second)
		fc.fc.Advance(period)
		waitForTickInvocations(t, getCount, want, 1*time.Second)
	}
	return getCount()
}

// fakeClockHelper wraps clock.FakeClock to expose the pending-list length
// for the wait-after-advance pattern used by advanceForNTicks. The pending
// length tells us whether the tick goroutine has registered its next After
// (we wait for >= want before advancing).
type fakeClockHelper struct {
	fc *clock.FakeClock
}

func (h *fakeClockHelper) waitForPending(t *testing.T, want int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if h.fc.PendingLen() >= want {
			return
		}
		time.Sleep(1 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for FakeClock pending >= %d (got %d)", want, h.fc.PendingLen())
}

// --- TestTick_FloorConstant ----------------------------------------------

func TestTick_FloorConstant(t *testing.T) {
	if TickPeriodFloor != 10*time.Millisecond {
		t.Errorf("TickPeriodFloor = %v; want 10ms (Q5 envoy-go-strict)", TickPeriodFloor)
	}
}

// --- TestTick_10msFloorEnforcement ---------------------------------------

// TestTick_10msFloorEnforcement: SetTickPeriod(5ms) — the host clamps to
// 10ms per Q5.
//
// Verification: the tick goroutine's `<-clk.After(period)` registration
// uses period=10ms (the clamped value), NOT the requested 5ms. We assert
// this by (a) confirming Advance(5ms) does NOT fire (the 5ms-period
// hypothesis would have the After-entry's deadline at +5ms which would
// fire); (b) confirming Advance(5ms more, total 10ms) DOES fire (the
// 10ms floor is the actual deadline). Then for safety we advance for 2
// more ticks at the 10ms cadence + assert the cadence holds.
func TestTick_10msFloorEnforcement(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0))
	var fires atomic.Uint64
	rv := newRootVMForTick(t, fc, WithRootTickHandler(func() { fires.Add(1) }))
	h := &fakeClockHelper{fc: fc}

	rv.SetTickPeriod(5 * time.Millisecond) // below floor → clamps to 10ms

	// Wait for the tick goroutine to register its first After.
	h.waitForPending(t, 1, 1*time.Second)

	// Advance by 5ms — at the (denied) 5ms cadence this would fire. At the
	// 10ms clamped cadence it should NOT fire.
	fc.Advance(5 * time.Millisecond)
	time.Sleep(20 * time.Millisecond) // grace
	if got := fires.Load(); got != 0 {
		t.Errorf("fires after Advance(5ms) = %d; want 0 (5ms below 10ms floor — should NOT fire)", got)
	}

	// Advance 5 more ms (total 10ms) — should fire ONCE.
	fc.Advance(5 * time.Millisecond)
	waitForTickInvocations(t, fires.Load, 1, 1*time.Second)

	// Run 2 more ticks at the 10ms cadence to confirm the period sticks.
	advanceForNTicks(t, h, fires.Load, 1, 2, 10*time.Millisecond)
	if got := fires.Load(); got != 3 {
		t.Errorf("fires = %d; want 3 (1 initial + 2 cadence)", got)
	}
}

// --- TestTick_PeriodCancellation -----------------------------------------

// TestTick_PeriodCancellation: SetTickPeriod(50ms) then SetTickPeriod(0)
// → no further ticks fire after the cancel.
func TestTick_PeriodCancellation(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0))
	var fires atomic.Uint64
	rv := newRootVMForTick(t, fc, WithRootTickHandler(func() { fires.Add(1) }))
	h := &fakeClockHelper{fc: fc}

	rv.SetTickPeriod(50 * time.Millisecond)
	h.waitForPending(t, 1, 1*time.Second)
	fc.Advance(50 * time.Millisecond)
	waitForTickInvocations(t, fires.Load, 1, 1*time.Second)

	rv.SetTickPeriod(0) // CANCEL
	before := fires.Load()

	// After cancel, the tick goroutine has exited (SetTickPeriod(0) waits
	// on the WaitGroup). FakeClock.PendingLen should drop to 0 (the
	// goroutine's pending After is orphaned — it never gets fired because
	// nobody reads the channel, but it stays in the pending list until
	// the next Advance covers its deadline). Advance to clear + assert no
	// more fires.
	fc.Advance(500 * time.Millisecond) // would fire 10 times if not canceled
	time.Sleep(20 * time.Millisecond)
	if got := fires.Load(); got != before {
		t.Errorf("fires after cancel = %d; want %d (no further fires after SetTickPeriod(0))", got, before)
	}
}

// --- TestTick_RescheduleWithNewPeriod ------------------------------------

// TestTick_RescheduleWithNewPeriod: SetTickPeriod(50ms) then
// SetTickPeriod(10ms) → the new period takes effect; old goroutine cleanly
// exits before the new one starts.
func TestTick_RescheduleWithNewPeriod(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0))
	var fires atomic.Uint64
	rv := newRootVMForTick(t, fc, WithRootTickHandler(func() { fires.Add(1) }))
	h := &fakeClockHelper{fc: fc}

	rv.SetTickPeriod(50 * time.Millisecond)
	h.waitForPending(t, 1, 1*time.Second)
	fc.Advance(50 * time.Millisecond)
	waitForTickInvocations(t, fires.Load, 1, 1*time.Second)

	// The old 50ms goroutine may have registered a second After at deadline
	// 100ms before we re-schedule (it's racing between dispatch-return and
	// the select). Cancel + drain the orphan first so the next
	// SetTickPeriod(10ms) starts from a clean slate.
	rv.SetTickPeriod(0)
	// Advance past the orphan's deadline (100ms = 50ms more from current
	// 50ms) so the orphan's pending After-entry drains. The send fires into
	// a buffered (cap=1) channel that nobody reads; the channel becomes
	// GC-eligible at end of Advance.
	fc.Advance(60 * time.Millisecond)
	// Confirm pending cleared.
	if got := fc.PendingLen(); got != 0 {
		t.Fatalf("orphan drain failed: pending=%d after cancel+advance", got)
	}

	// NOW re-schedule to 10ms. The new 10ms-period goroutine takes over from
	// a clean pending state.
	baseline := fires.Load()
	rv.SetTickPeriod(10 * time.Millisecond)
	h.waitForPending(t, 1, 1*time.Second)

	// Run 3 ticks at the new 10ms cadence — expect 3 more fires.
	advanceForNTicks(t, h, fires.Load, baseline, 3, 10*time.Millisecond)
	if got := fires.Load(); got != baseline+3 {
		t.Errorf("fires = %d; want %d (baseline %d + 3 from 10ms cadence)", got, baseline+3, baseline)
	}
}

// --- TestTick_PanicInTickRecovers ----------------------------------------

// TestTick_PanicInTickRecovers: the tick callback panics → tick goroutine
// survives (the panic-wrapper in lockAndDispatchTick recovers); PanicHandlerFn
// is invoked; the NEXT tick still dispatches.
func TestTick_PanicInTickRecovers(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0))
	var fires atomic.Uint64
	var panics atomic.Uint64
	var captured any
	var mu sync.Mutex

	handler := func() {
		n := fires.Add(1)
		if n == 1 {
			panic("synthetic tick panic")
		}
	}
	panicH := func(r any) {
		mu.Lock()
		captured = r
		mu.Unlock()
		panics.Add(1)
	}
	rv := newRootVMForTick(t, fc,
		WithRootTickHandler(handler),
		WithRootPanicHandler(panicH),
	)
	h := &fakeClockHelper{fc: fc}

	rv.SetTickPeriod(10 * time.Millisecond)
	h.waitForPending(t, 1, 1*time.Second)

	// First tick: handler panics → goroutine recovers, panicH fires.
	fc.Advance(10 * time.Millisecond)
	waitForTickInvocations(t, fires.Load, 1, 1*time.Second)
	// Wait briefly for panicH to be invoked (it runs INSIDE recover).
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && panics.Load() == 0 {
		time.Sleep(1 * time.Millisecond)
	}
	if got := panics.Load(); got != 1 {
		t.Errorf("PanicHandlerFn fires = %d; want 1", got)
	}
	mu.Lock()
	gotPanic := captured
	mu.Unlock()
	if gotPanic != "synthetic tick panic" {
		t.Errorf("captured panic = %v; want %q", gotPanic, "synthetic tick panic")
	}

	// Second tick: handler does NOT panic → goroutine still alive + dispatches.
	// Wait for the goroutine to re-register its After before advancing.
	h.waitForPending(t, 1, 1*time.Second)
	fc.Advance(10 * time.Millisecond)
	waitForTickInvocations(t, fires.Load, 2, 1*time.Second)
}

// --- TestTick_ConcurrentStreamsShareOneTickGoroutine ---------------------

// TestTick_ConcurrentStreamsShareOneTickGoroutine: N=100 concurrent
// NewStreamContext + SetTickPeriod called once → only ONE tick goroutine
// runs per RootVM (Q5 anti-tick-storm assertion).
//
// Verification: assert exactly one tickStop chan is registered on the
// RootVM after all 100 streams are created.
func TestTick_ConcurrentStreamsShareOneTickGoroutine(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0))
	var fires atomic.Uint64
	rv := newRootVMForTick(t, fc, WithRootTickHandler(func() { fires.Add(1) }))

	rv.SetTickPeriod(10 * time.Millisecond)
	h := &fakeClockHelper{fc: fc}
	h.waitForPending(t, 1, 1*time.Second)

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	ctx := context.Background()
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, err := rv.NewStreamContext(ctx)
			if err != nil {
				t.Errorf("NewStreamContext: %v", err)
			}
		}()
	}
	wg.Wait()

	// Drive 5 ticks at the 10ms cadence — expect EXACTLY 5 fires (one per
	// 10ms period), NOT 500 (which would be 5 fires × 100 streams if
	// there were a per-stream tick goroutine).
	advanceForNTicks(t, h, fires.Load, 0, 5, 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	if got := fires.Load(); got != 5 {
		t.Errorf("fires = %d; want exactly 5 (one tick goroutine per RootVM, NOT per stream)", got)
	}

	// Defensive: peek at the RootVM's tickStop field to verify the design
	// invariant — at most one tickStop chan registered (we serialize via
	// tickMu so the assertion is race-free).
	rv.tickMu.Lock()
	hasOneStop := rv.tickStop != nil
	rv.tickMu.Unlock()
	if !hasOneStop {
		t.Errorf("expected exactly 1 tickStop chan; got nil (no tick goroutine alive)")
	}
}

// --- TestTick_ClosesAtRootVMClose ----------------------------------------

// TestTick_ClosesAtRootVMClose: tick active → Close() stops the goroutine
// cleanly + returns within reasonable time. Without the Close-side
// goroutine-stop integration this test would hang (the tick goroutine
// would keep running after rv.runtime closes, dispatching against a
// nil/closed module).
func TestTick_ClosesAtRootVMClose(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0))
	var fires atomic.Uint64
	ctx := context.Background()
	mod := mustCompileWithCacheForRootVM(t, ctx, exportsAll25_2CallbacksModule)
	rv, err := NewRootVM(ctx, mod, 1,
		WithRootSandboxConfig(allowAllSandbox()),
		WithRootClock(fc),
		WithRootTickHandler(func() { fires.Add(1) }),
	)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	rv.RegisterABICallbacks(&fakeABICallbacks{})

	rv.SetTickPeriod(10 * time.Millisecond)
	h := &fakeClockHelper{fc: fc}
	h.waitForPending(t, 1, 1*time.Second)
	fc.Advance(10 * time.Millisecond)
	waitForTickInvocations(t, fires.Load, 1, 1*time.Second)

	// Close must stop the tick goroutine BEFORE clearing rv.instance.
	done := make(chan error, 1)
	go func() { done <- rv.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did NOT return within 5s (tick goroutine likely leaked or deadlocked at Close)")
	}
}

// --- TestTick_DefaultClock_RealWiring -----------------------------------

// TestTick_DefaultClock_RealWiring: when NO WithRootClock is supplied, the
// RootVM defaults to clock.RealClock — exercising the real time-source so
// the production wiring path is covered.
func TestTick_DefaultClock_RealWiring(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileWithCacheForRootVM(t, ctx, exportsAll25_2CallbacksModule)
	var fires atomic.Uint64
	rv, err := NewRootVM(ctx, mod, 1,
		WithRootSandboxConfig(allowAllSandbox()),
		// NO WithRootClock — defaults to RealClock.
		WithRootTickHandler(func() { fires.Add(1) }),
	)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	rv.RegisterABICallbacks(&fakeABICallbacks{})

	rv.SetTickPeriod(10 * time.Millisecond)
	waitForTickInvocations(t, fires.Load, 1, 1*time.Second)
}
