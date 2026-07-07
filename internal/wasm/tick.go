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
//	        rv.lockAndDispatchTick(ctx, stop)
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
// SIGNALS the current goroutine to exit (close(rv.tickStop)) then spawns a
// fresh goroutine with the new period WITHOUT waiting for the old one to
// exit — the superseded goroutine is fenced off by the stale-generation
// check in lockAndDispatchTick (its snapshotted stop channel no longer
// matches rv.tickStop) so it can never fire a stale tick. period <= 0
// cancels the tick (signals the current goroutine + leaves rv.tickStop
// nil). Waiting here would deadlock: caller site (a) holds dispatchMu, and
// the old goroutine may itself be the caller's frame (a guest re-arming the
// tick from inside proxy_on_tick) or blocked on the caller-held dispatchMu
// inside lockAndDispatchTick. Only stopTickGoroutine (the Close path, never
// under dispatchMu) joins via tickWG.Wait().
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
// Re-schedule discipline (signal-then-spawn, NO join) is deliberately
// simple: it avoids a re-schedule channel + works correctly when the
// goroutine is currently blocked in <-rv.clk.After(...). The cost is at
// most one extra FakeClock.After registration per re-schedule (the old
// goroutine's pending After-channel is dropped + becomes eligible for GC
// after the next FakeClock.Advance that covers its deadline, or after the
// channel reference is dropped in the production RealClock case via the
// runtime time-heap's natural eviction).
//
// Caller MUST NOT hold tickMu when invoking SetTickPeriod (we acquire it).
// Caller MAY hold dispatchMu (the abi-shim caller path is inside dispatchMu)
// — and the OLD tick goroutine may even BE the caller's own dispatch frame
// (a guest re-arming the tick from inside proxy_on_tick, the standard
// one-shot-timer pattern). That is exactly why SetTickPeriod MUST NOT
// tickWG.Wait(): joining the old goroutine from inside its own dispatch
// frame (or while it is blocked on the caller-held dispatchMu inside
// lockAndDispatchTick) is a guaranteed deadlock. The superseded goroutine
// instead exits on its own (its snapshotted stop channel is closed) and the
// stale-generation check in lockAndDispatchTick keeps it from ever firing a
// stale tick in the interim.
//
// NOTE: SetTickPeriod is safe to call BEFORE NewStreamContext + AFTER
// Configure + AFTER any number of NewStreamContext calls. The tick
// dispatches on the root context (currentCtxID = rootCtxID per §5.3 C18).
func (rv *RootVM) SetTickPeriod(period time.Duration) {
	rv.tickMu.Lock()
	defer rv.tickMu.Unlock()

	// Signal the current goroutine to exit (if any). Close the stop channel
	// + clear the field; deliberately NO tickWG.Wait() here (see the
	// re-schedule discipline doc above — waiting would self-deadlock when
	// the guest re-arms the tick from inside proxy_on_tick, or cross-
	// deadlock when a fired tick is blocked on the caller-held dispatchMu).
	if rv.tickStop != nil {
		close(rv.tickStop)
		rv.tickStop = nil
	}

	// period <= 0 ⇒ cancel (no replacement goroutine).
	if period <= 0 {
		rv.tickPeriod = 0
		return
	}

	// Refuse to spawn on a closed RootVM: a hostcall frame racing Close
	// could otherwise leak a fresh tick goroutine after stopTickGoroutine's
	// sweep + race its tickWG.Add against the Close-side tickWG.Wait.
	if rv.closed.Load() {
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
//	        rv.lockAndDispatchTick(ctx, stop)
//	    case <-stop:
//	        return
//	    }
//	}
//
// The `stop` arg is the goroutine's OWN stop channel (snapshotted under
// tickMu at spawn time) so a concurrent SetTickPeriod re-schedule does NOT
// confuse this goroutine into watching a fresh chan. The same snapshot
// doubles as the goroutine's GENERATION token: lockAndDispatchTick compares
// it against the current rv.tickStop and refuses to dispatch when they
// differ (this goroutine has been superseded by a re-schedule and merely
// hasn't observed its closed stop channel yet — select picks randomly when
// both the After fire and the stop close are ready).
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
			rv.lockAndDispatchTick(ctx, stop)
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
//   - stale-generation check: the caller's snapshotted stop channel must
//     still be the current rv.tickStop (compared under tickMu). A
//     SetTickPeriod re-schedule/cancel supersedes the calling goroutine
//     WITHOUT joining it (see SetTickPeriod doc); a superseded goroutine
//     that already consumed an After fire lands here and MUST NOT dispatch
//     a stale tick. Lock order: dispatchMu → tickMu (same order as the
//     hostcall SetTickPeriod path, which runs inside a dispatchMu-held
//     frame; stopTickGoroutine never holds tickMu across the tickWG join).
//   - proxy_on_tick guest export missing OR capProxyOnTick capability denied
//     ⇒ skip the guest invocation (HasGlobalFunc handles both via the
//     gate-at-getFunction discipline per AMEND-B5).
//
// The runCallWithPanicWrapper around the fn.Call recovers a guest-side
// panic; the panic does NOT propagate to the caller (tickRun) so the tick
// goroutine survives. PanicHandlerFn (if configured) fires from inside the
// wrapper.
func (rv *RootVM) lockAndDispatchTick(ctx context.Context, stop <-chan struct{}) {
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
	// Stale-generation check (see the guard list above): a superseded
	// goroutine's stop snapshot no longer matches rv.tickStop — skip the
	// dispatch entirely (including the test/observability tickHandler).
	rv.tickMu.Lock()
	current := rv.tickStop
	rv.tickMu.Unlock()
	if current != stop {
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
	if rv.tickStop != nil {
		close(rv.tickStop)
		rv.tickStop = nil
	}
	rv.tickPeriod = 0
	rv.tickMu.Unlock()
	// Wait OUTSIDE tickMu — the tick goroutine's lockAndDispatchTick
	// acquires tickMu for the stale-generation check; holding tickMu across
	// the join would deadlock against a goroutine blocked on that acquire.
	rv.tickWG.Wait()
}
