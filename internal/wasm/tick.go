// tick.go — per-*RootVM tick goroutine + 10ms envoy-go-strict period floor +
// FIRST co-consumer of phase-21 ADR-0186 Clock seam (via internal/clock) per
// Q5 + R-25.2-9 + ADR-0205.
//
// # Design (per Q5 + R-25.2-9)
//
// Each *RootVM owns AT MOST ONE tick goroutine at any time. The goroutine
// runs:
//
//	for {
//	    select {
//	    case <-rv.clk.After(period):
//	        rv.lockAndDispatchTick(ctx)
//	    case <-stop:
//	        return
//	    }
//	}
//
// `period` is the effective tick period — `max(requestedPeriod, 10ms)` per
// Q5 envoy-go-strict floor (prevents guest-driven CPU spin attacks via
// `proxy_set_tick_period_milliseconds(1)`-style abuse).
//
// SetTickPeriod is called from two sites: (a) the host shim
// abi.SetTickPeriodMillisecondsShim (which is invoked from inside the per-
// RootVM dispatchMu-held frame of the dispatcher running proxy_on_X); (b)
// directly from the per-plugin configure path (a future Task 14 connection;
// at Task 5 only (a) is wired). Re-schedule semantics: SetTickPeriod
// STOPS the current goroutine (close(rv.tickStop) + WaitGroup wait) BEFORE
// spawning a fresh goroutine with the new period. period <= 0 cancels the
// tick (stops the current goroutine + leaves rv.tickStop nil).
//
// # The clock-seam dependency
//
// Phase-21 ADR-0186 §Decision kept the Clock seam INLINE at
// internal/filter/http/adaptive_concurrency/clock.go at consumer count = 1.
// §Consequences (g) pinned the EXTRACT-NOW trigger: when a SECOND timer-
// driven framework consumer materializes, lift to internal/clock/. Phase
// 25.2 IMPL Task 5 IS that second consumer — the per-*RootVM tick
// dispatcher needs Clock-seam injection at NewRootVM time via
// WithRootClock(clk) for fixture fake-time support. The framework-level
// internal/clock/ package (Clock + RealClock + FakeClock) is the lifted
// surface that this file consumes; the existing inline
// adaptive_concurrency/clock.go is unchanged at 25.2 (a follow-up refactor
// may migrate it per ADR-0186 §Consequences (g)'s "the consumer's Clock-
// typed field unchanged" pattern).
//
// # Goroutine-safety
//
//   - tickMu serializes ALL SetTickPeriod invocations (re-schedule discipline).
//   - The tick goroutine itself acquires dispatchMu via lockAndDispatchTick
//     to satisfy the per-RootVM "ONE proxy_on_* / foreign-function in flight
//     at a time" invariant per root_vm.go dispatchMu doc.
//   - At Close, the tick goroutine is signaled via close(rv.tickStop) +
//     joined via rv.tickWG.Wait() BEFORE rv.runtime / rv.instance are
//     cleared. Close-then-tick-fire races are bounded: if the closed flag
//     is set after the tick goroutine has selected its <-rv.clk.After(...)
//     case but before it acquires dispatchMu, the closed check inside
//     lockAndDispatchTick short-circuits the proxy_on_tick invocation.
//
// # Panic-recovery
//
// lockAndDispatchTick wraps the proxy_on_tick invocation with the per-
// RootVM runCallWithPanicWrapper so a guest-side panic does NOT kill the
// tick goroutine. The PanicHandlerFn fires + the tick goroutine continues.
// The envoy_go.failures counter integration is wired by the consumer-side
// stats.go EXTEND at Task 17; at Task 5 only the recover-and-continue path
// is materialized.

package wasm

import (
	"context"
	"time"
)

// TickPeriodFloor is the 10ms envoy-go-strict floor for the
// proxy_set_tick_period_milliseconds hostcall. Requested periods below the
// floor are silently clamped to 10ms host-side. Rationale: prevents guest-
// driven CPU spin attacks (period=0 → hot loop). Compile-time constant per
// 25.2 SPEC §2.16 + §3.1 + Q5. Lands as envoy-go-strict departure record
// #4 at 25.2 BEHAVIOR_CONTRACT.md (consolidated with shared-data + body-
// buffer caps per §9).
const TickPeriodFloor = 10 * time.Millisecond

// SetTickPeriod (re-)schedules the per-*RootVM tick goroutine. Called from
// the host-side abi.SetTickPeriodMillisecondsShim dispatch (invoked from
// inside the dispatchMu-held frame of a proxy_on_X handler).
//
// Semantics (per Q5 + R-25.2-9):
//
//   - period <= 0 ⇒ CANCEL. Stop the current goroutine (if any); leave no
//     replacement.
//   - 0 < period < TickPeriodFloor ⇒ CLAMP to TickPeriodFloor (10ms floor).
//     Stop the current goroutine + spawn a fresh one running the clamped
//     period.
//   - period >= TickPeriodFloor ⇒ USE AS-IS. Stop the current goroutine +
//     spawn a fresh one running the requested period.
//
// Re-schedule discipline (stop-then-spawn) is deliberately simple: it avoids
// a re-schedule channel + works correctly when the goroutine is currently
// blocked in <-rv.clk.After(...). The cost is at most one extra
// FakeClock.After registration per re-schedule (the old goroutine's pending
// After-channel is dropped + becomes eligible for GC after the next
// FakeClock.Advance that covers its deadline, or after the channel
// reference is dropped in the production RealClock case via the runtime
// time-heap's natural eviction).
//
// Caller MUST NOT hold tickMu when invoking SetTickPeriod (we acquire it).
// Caller MAY hold dispatchMu (the abi-shim caller path is inside dispatchMu;
// the WaitGroup-wait on the old tick goroutine does NOT re-acquire
// dispatchMu — it only waits for the goroutine's own exit, which races with
// the goroutine's own dispatchMu-acquire — but the goroutine acquires
// dispatchMu only INSIDE lockAndDispatchTick which short-circuits via the
// closed-or-stopped check; the stop signal pre-empts the next After loop
// iteration BEFORE re-acquiring dispatchMu).
//
// NOTE: SetTickPeriod is safe to call BEFORE NewStreamContext + AFTER
// Configure + AFTER any number of NewStreamContext calls. The tick
// dispatches on the root context (currentCtxID = rootCtxID per §5.3 C18).
func (rv *RootVM) SetTickPeriod(period time.Duration) {
	rv.tickMu.Lock()
	defer rv.tickMu.Unlock()

	// Stop the current goroutine (if any). Close the stop channel, wait
	// for the WaitGroup to drain, then clear the field.
	if rv.tickStop != nil {
		close(rv.tickStop)
		rv.tickStop = nil
		rv.tickWG.Wait()
	}

	// period <= 0 ⇒ cancel (no replacement goroutine).
	if period <= 0 {
		rv.tickPeriod = 0
		return
	}

	// Floor clamp per Q5.
	effective := period
	if effective < TickPeriodFloor {
		effective = TickPeriodFloor
	}
	rv.tickPeriod = effective

	// Spawn the new goroutine. We use a background context here — the
	// tick dispatch lifetime is bounded by Close (rv.tickStop closure),
	// not by any per-request context.
	rv.tickStop = make(chan struct{})
	rv.tickWG.Add(1)
	go rv.tickRun(context.Background(), effective, rv.tickStop)
}

// tickRun is the per-*RootVM tick goroutine. Runs the canonical proxy-wasm
// tick dispatch loop:
//
//	for {
//	    select {
//	    case <-rv.clk.After(period):
//	        rv.lockAndDispatchTick(ctx)
//	    case <-stop:
//	        return
//	    }
//	}
//
// The `stop` arg is the goroutine's OWN stop channel (snapshotted under
// tickMu at spawn time) so a concurrent SetTickPeriod re-schedule does NOT
// confuse this goroutine into watching a fresh chan.
func (rv *RootVM) tickRun(ctx context.Context, period time.Duration, stop <-chan struct{}) {
	defer rv.tickWG.Done()
	for {
		// Re-arm the After channel on every loop iteration. The "drop the
		// channel reference on stop" race is benign because the channel is
		// cap=1 buffered in both RealClock + FakeClock (see internal/clock
		// doc) — a lost-race fire writes into the buffer slot + the channel
		// is GC'd when this goroutine drops the reference at return.
		select {
		case <-rv.clk.After(period):
			rv.lockAndDispatchTick(ctx)
		case <-stop:
			return
		}
	}
}

// lockAndDispatchTick acquires the per-*RootVM dispatchMu + invokes
// proxy_on_tick on the ROOT context (currentCtxID = rootCtxID per §5.3 C18)
// with panic-recovery. Also invokes the optional per-RootVM tick-handler hook
// (rv.tickHandler) for test observability — fires INSIDE the same dispatchMu
// frame so observations are race-free relative to other dispatch.
//
// Short-circuit guards:
//   - closed flag set ⇒ return immediately (Close-in-flight; do NOT acquire
//     dispatchMu; do NOT touch rv.instance).
//   - re-check closed flag AFTER acquiring dispatchMu (Close races with the
//     tick fire — the lock-acquire may complete after Close.Lock release).
//   - proxy_on_tick guest export missing OR capProxyOnTick capability denied
//     ⇒ skip the guest invocation (HasGlobalFunc handles both via the
//     gate-at-getFunction discipline per AMEND-B5).
//
// The runCallWithPanicWrapper around the fn.Call recovers a guest-side
// panic; the panic does NOT propagate to the caller (tickRun) so the tick
// goroutine survives. PanicHandlerFn (if configured) fires from inside the
// wrapper.
func (rv *RootVM) lockAndDispatchTick(ctx context.Context) {
	if rv.closed.Load() {
		return
	}
	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()
	// Re-check closed AFTER lock: Close races with the tick-fire goroutine;
	// Close may have acquired + released dispatchMu (e.g., during a final
	// drain) between our outer-closed-check and our Lock call.
	if rv.closed.Load() || rv.instance == nil {
		return
	}

	// Test/observability hook: fire the tick handler if configured. The
	// handler runs INSIDE the dispatchMu frame so any observations the
	// handler makes (counters, callbacks) inherit the serialization
	// discipline. The handler MUST NOT re-acquire dispatchMu (sync.Mutex
	// is non-reentrant in Go).
	if rv.tickHandler != nil {
		// Wrap with the err-returning panic-wrapper so a handler panic
		// recovers + PanicHandlerFn fires + the loop survives.
		_ = rv.runCallWithPanicWrapper(func() error {
			rv.tickHandler()
			return nil
		})
	}

	// Set currentCtxID to root for the proxy_on_tick dispatch (per §5.3 C18:
	// tick callback dispatches on the ROOT context, NOT a per-stream
	// context).
	rv.currentCtxID.Store(rv.rootCtxID)

	// gate-at-getFunction (AMEND-B5): HasGlobalFunc short-circuits on the
	// denied-capability path AND on the no-such-export path.
	if !rv.HasGlobalFunc("proxy_on_tick") {
		return
	}
	fn := rv.instance.ExportedFunction("proxy_on_tick")
	if fn == nil {
		// HasGlobalFunc said it exists but ExportedFunction returned nil —
		// defensive (theoretically unreachable; concurrent module-close
		// would have set closed=true).
		return
	}
	_ = rv.runCallWithPanicWrapper(func() error {
		_, err := fn.Call(ctx, uint64(rv.rootCtxID))
		return err
	})

	// envoy-go-strict counter increment per Q5 (counter 6 of 14 in §7.1).
	// Wire name: `wasm.<plugin>.tick_invocations`. Incremented after the
	// proxy_on_tick dispatch returns (panic-wrapper swallowed any guest
	// panic; the tick still fired from the host perspective). Wired via
	// RootStatsRecorder per 25.2 IMPL Task 20 follow-up (Concern 2).
	rv.stats.TickInvocationsInc()
}

// stopTickGoroutine is the Close-time path: stops the tick goroutine (if
// any) + waits for its exit, without re-spawning. Called from Close BEFORE
// rv.runtime / rv.instance are cleared so the tick goroutine's
// lockAndDispatchTick observes a live module when it short-circuits via
// the closed flag.
func (rv *RootVM) stopTickGoroutine() {
	rv.tickMu.Lock()
	defer rv.tickMu.Unlock()
	if rv.tickStop != nil {
		close(rv.tickStop)
		rv.tickStop = nil
	}
	rv.tickPeriod = 0
	// Release tickMu before Wait — the tick goroutine does NOT touch
	// tickMu, so this is purely defensive against a future change that
	// might.
	rv.tickWG.Wait()
}
