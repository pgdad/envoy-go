//go:build !race

package wasm

// pause_gen_test.go — phase-83 S3/S9: the pause-watchdog GENERATION harness.
//
// # What this file gates
//
// A wasm guest that pauses, resumes, and pauses again inside one stream arms a
// fresh watchdog each time. Every SUPERSEDED watchdog is still a live
// time.AfterFunc closure holding the chain's *decoderCb. When one of those
// closures runs it wins beginDecodePause's CompareAndSwap against a LATER
// pause's flag and calls ContinueDecoding for a pause it does not own — a
// SPURIOUS resume that un-parks the chain early. The fix is two-part:
//
//	(1) a monotone generation counter captured at arm time and re-checked
//	    INSIDE the closure, ahead of the CAS, and
//	(2) Stop() on the timer handle the new pause displaces.
//
//	  ⚠️ The generation check must precede the CAS. CAS-FIRST is worse than
//	  the bug: a superseded closure consumes the CURRENT pause's flag and only
//	  then bails on the generation check, so the current pause has no resumer
//	  at all and the chain parks forever.
//
// # ⚠️ THE PACER AND THE WATCHDOG ARE BOTH PINNED, AND THE RATIO IS THE GATE
//
// A fire-count A/B is VACUOUS unless the pacer implementation, the watchdog
// magnitude and their RATIO are all pinned. Measured at the phase-83 PLAN
// stage, two independent agents read OPPOSITE results for the same cell of
// this same A/B because one paced with time.Sleep (which has a ~1 ms floor on
// this host) and one busy-waited; a third configuration read 291 vs 298 —
// green on BOTH arms.
//
// This harness pins:
//
//	pacer     time.Sleep
//	pace      p83Pace     = 1 ms
//	watchdog  p83Watchdog = 100 ms
//	n         p83Gens     = 300
//
// and the discriminating regime is
//
//	pace < watchdog < n*pace
//
// pace must be SHORTER than the watchdog so later pauses supersede earlier
// timers, and n*pace must be LONGER than the watchdog so superseded fires
// INTERLEAVE with fresh pauses. Both boundaries are load-bearing: at
// watchdog=100 ms / pace=0 the loop finishes long before the first deadline,
// the flag is set once, the first fire consumes it, and EVERY arm reads 1 —
// measured, a fully vacuous cell.
//
// ⚠️ THE PINNED PACE IS 0.01x THE WATCHDOG, NOT EQUAL TO IT, AND THAT IS A
// MEASURED CHOICE. The 2x2 was run at both. pace == watchdog does separate
// (299 vs 1) but its FIXED-arm 1 is a scheduler-latency outcome, not a
// structural one: the superseded closure's deadline lands 50-100 us BEFORE the
// next pause bumps the generation, so the fixed arm reads 1 only because the
// AfterFunc goroutine has not been scheduled yet when the bump lands. One of
// five repeats at watchdog=pace=20 ms already read 2. At pace << watchdog the
// fixed arm is structurally 1: once pause k+1 has run, generation k can NEVER
// again be current, so timer k cannot fire however late it is scheduled.
// A gate whose GREEN arm depends on winning a 60 us race fails a correct
// implementation on a faster host; this one cannot.
//
// ⚠️ A NOMINAL pace IS NOT THE REAL pace. time.Sleep has a ~1.05 ms floor on
// this host: 300 nominal 50 us sleeps measured 315.6 ms, i.e. 1.052 ms each,
// so a configuration LABELED pace == watchdog == 50 us really runs at 21x the
// watchdog. The measured loop wall time is therefore recorded in the harness
// log line and in every failure message — never the nominal value alone.
//
// # ⚠️ NOT `-race`-SAFE — the build tag is deliberate, and the test is NOT flaky
//
// Under `-race` the instrumented atomics widen the arm-to-fire window so a
// superseded closure legitimately reaches its guard BEFORE the superseding
// pause bumps the generation. Fixed code then reads in the SAME band as broken
// code: measured with the CORRECT fix loaded, 5 of 7 fire-count assertions
// FAIL (133/300, 2, 150/300, 207/400, 101/200) with ZERO DATA RACE reported.
// That is a property of the measurement, not a defect in the code under test
// and NOT nondeterminism in this file: the fire count is a race-window
// observable, and `-race` changes the window. The blindness is recorded, not
// papered over — routing the watchdog through internal/clock's Clock seam
// would remove it, but clock.Clock exposes no AfterFunc, so that is a named
// deferred candidate rather than this row's work.
//
// # ⚠️ NO t.Parallel() ANYWHERE IN THIS FILE
//
// The same gate was measured FAIL 3/3 under full-package load with
// t.Parallel() and PASS 3/3 under `-run` isolation. Every test here is
// timing-shaped; a parallel sibling stealing the P is indistinguishable from a
// superseded fire.

import (
	"sync"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// The pinned configuration. Changing any of these three invalidates the 2x2
// discrimination proof recorded in the Task-1 commit message; re-run it.
// -----------------------------------------------------------------------------

const (
	// p83Watchdog is the per-stream watchdog override handed to the filter.
	p83Watchdog = 100 * time.Millisecond
	// p83Pace is the nominal time.Sleep between two consecutive pauses.
	p83Pace = 1 * time.Millisecond
	// p83Gens is the number of superseded generations driven per run.
	p83Gens = 300
	// p83Trials is the number of independent disarm-authority trials.
	p83Trials = 400
)

// p83CountingCb counts ContinueDecoding calls made by watchdog closures.
// It deliberately does NOT embed countingDecoderCb: this harness must be
// readable on its own, and the embedded type's atomic.Int32 saturates well
// below the counts a stacked break produces on a wider n.
type p83CountingCb struct {
	fakeDecoderCb
	mu    sync.Mutex
	count int
}

func (c *p83CountingCb) ContinueDecoding() {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
}

func (c *p83CountingCb) load() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// p83CountingEncCb is the encode-side twin. It exists because beginEncodePause
// is a hand-written MIRROR of beginDecodePause, not a shared helper: nothing in
// the compiler couples the two, so a guard dropped from one half is invisible
// to a gate that only drives the other.
type p83CountingEncCb struct {
	fakeEncoderCb
	mu    sync.Mutex
	count int
}

func (c *p83CountingEncCb) ContinueEncoding() {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
}

func (c *p83CountingEncCb) load() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// p83HarnessCbDecode / p83HarnessCbEncode are the guest-export names this
// harness attributes its synthetic pauses to. beginDecodePause / beginEncodePause
// take the name as a parameter as of phase-83 S1 — it is printed verbatim in
// the watchdog's operator-facing warning, and hard-coding the headers callback
// there misattributed every BODY and TRAILERS pause.
const (
	p83HarnessCbDecode = "proxy_on_request_headers"
	p83HarnessCbEncode = "proxy_on_response_headers"
)

// p83Opts parameterizes the fire-count harness. Every field is recorded in the
// failure messages of its callers — an unpinned pacer is the whole failure
// mode this file exists to avoid.
type p83Opts struct {
	// Gens is the number of beginDecodePause calls driven back to back.
	Gens int
	// Watchdog is the per-stream watchdog override (f.pauseWatchdog).
	Watchdog time.Duration
	// Pace is the nominal time.Sleep between two consecutive pauses.
	Pace time.Duration
	// Encode drives beginEncodePause instead of beginDecodePause.
	Encode bool
}

// p83Side renders an Opts side for a failure message.
func (o p83Opts) p83Side() string {
	if o.Encode {
		return "encode"
	}
	return "decode"
}

// p83FireCount drives Gens superseded decode-side pauses at the pinned pacing
// and returns how many ContinueDecoding calls the watchdog closures made
// (fires) and how many of those arrived when this filter owed the chain NO
// resume (spurious).
//
// SPURIOUS is the load-bearing number: a fire that lands while decodePaused is
// already false is a resume this filter does not owe, and the chain's resume
// channel is buffered-1, so it LATCHES a token that un-parks a DIFFERENT
// filter later in the same stream. Because every fire path is gated on
// CompareAndSwap(true,false), a fire can only be observed as spurious by
// counting fires against the number of pauses that were still outstanding;
// this harness reports it as "fires beyond the one the final generation owes".
func p83FireCount(t testing.TB, o p83Opts) (fires, spurious int) {
	t.Helper()

	f := &filter{}
	f.pauseWatchdog = o.Watchdog

	var begin func()
	var read func() int
	if o.Encode {
		cb := &p83CountingEncCb{}
		f.SetEncoderCallbacks(cb)
		begin, read = func() { f.beginEncodePause(p83HarnessCbEncode) }, cb.load
	} else {
		cb := &p83CountingCb{}
		f.SetDecoderCallbacks(cb)
		begin, read = func() { f.beginDecodePause(p83HarnessCbDecode) }, cb.load
	}
	t.Cleanup(f.stopPauseWatchdogs)

	start := time.Now()
	for i := 0; i < o.Gens; i++ {
		begin()
		if o.Pace > 0 {
			time.Sleep(o.Pace)
		}
	}
	loop := time.Since(start)

	// Settle: every armed watchdog must have had its full window to run, or a
	// low fire count would only mean "we did not wait", which reads as a pass.
	time.Sleep(o.Watchdog + 200*time.Millisecond)

	fires = read()
	spurious = fires - 1
	if spurious < 0 {
		spurious = 0
	}
	t.Logf("p83FireCount: side=%s gens=%d watchdog=%v pace=%v (pacer=time.Sleep) loop=%v fires=%d spurious=%d",
		o.p83Side(), o.Gens, o.Watchdog, o.Pace, loop, fires, spurious)
	return fires, spurious
}

// -----------------------------------------------------------------------------
// The gate.
// -----------------------------------------------------------------------------

// TestP83_PauseGeneration_SupersededWatchdogsDoNotFire is the S3 anchor.
//
// ⚠️ STACKED CONTROL. The bound below is an INTERVAL, not a floor. A
// lower-bound-only assertion cannot catch an OVER-firing counter, and an
// upper-bound-only assertion passes vacuously when nothing fires at all — the
// single-pause NC below supplies the liveness half by pinning a lone pause at
// EXACTLY 1.
//
// ⚠️ The guarded arm reads 1 OR 2, not exactly 1: the final generation's own
// watchdog always fires, and a closure that entered just before the last
// supersession can legitimately add one. The band is [1,10]; the broken arm
// measures two orders of magnitude above it.
// ⚠️ BOTH SIDES ARE DRIVEN. beginEncodePause is a hand-written MIRROR of
// beginDecodePause with its own generation field, its own closure and its own
// Stop(); no compiler coupling ties the two. A decode-only gate reads as
// coverage of the pair and is not.
func TestP83_PauseGeneration_SupersededWatchdogsDoNotFire(t *testing.T) {
	// NO t.Parallel — see the file header.
	for _, encode := range []bool{false, true} {
		o := p83Opts{Gens: p83Gens, Watchdog: p83Watchdog, Pace: p83Pace, Encode: encode}
		fires, spurious := p83FireCount(t, o)
		if fires < 1 || fires > 10 {
			t.Errorf("S3 fire-count [%s side]: the watchdog resumed the chain %d times over %d superseded generations (spurious=%d); want 1 <= fires <= 10.\n"+
				"PINNED CONFIGURATION: pacer=time.Sleep, pace=%v, watchdog=%v, ratio pace/watchdog=%.3f, n=%d.\n"+
				"A count far above the band means superseded watchdog closures are still resuming the chain: either the generation guard is missing, or it sits AFTER the CAS, or the displaced timer handle is not Stop()ped. A count of 0 means no watchdog ran at all — check the settle window, not the guard.",
				o.p83Side(), fires, p83Gens, spurious,
				p83Pace, p83Watchdog, float64(p83Pace)/float64(p83Watchdog), p83Gens)
		}
	}
}

// TestP83_PauseGeneration_SinglePauseNC is the liveness half of the stacked
// control: ONE pause, nothing to supersede it, so EXACTLY ONE watchdog fire.
//
// It is pinned at exactly 1 in both directions on purpose. Without it the
// `fires <= 10` bound above passes on a harness that observes nothing — a
// broken counting double, a watchdog that never arms, a settle window shorter
// than the watchdog — and reads as coverage.
func TestP83_PauseGeneration_SinglePauseNC(t *testing.T) {
	// NO t.Parallel — see the file header.
	for _, encode := range []bool{false, true} {
		o := p83Opts{Gens: 1, Watchdog: p83Watchdog, Pace: p83Pace, Encode: encode}
		fires, _ := p83FireCount(t, o)
		if fires != 1 {
			t.Errorf("single-pause NC [%s side]: the watchdog resumed the chain %d times for ONE unsuperseded pause; want exactly 1.\n"+
				"PINNED CONFIGURATION: pacer=time.Sleep, pace=%v, watchdog=%v, n=1.\n"+
				"0 means the harness cannot observe a fire at all and every fire-count bound in this file is vacuous; >1 means the counting double is double-counting.",
				o.p83Side(), fires, p83Pace, p83Watchdog)
		}
	}
}

// TestP83_PauseGeneration_GuardPrecedesTheCAS gates the ORDERING constraint,
// which the fire-count arm above CANNOT see.
//
// THE HAZARD, and why it is worse than the bug the guard fixes. If the CAS runs
// FIRST, a superseded closure consumes the CURRENT pause's flag and only THEN
// bails on the generation check. The current pause is left with no resumer at
// all: the guest's proxy_continue_stream loses its own CAS, the watchdog that
// would have force-resumed has already returned, and the chain parks FOREVER.
// A spurious resume becomes an UNBOUNDED park — the exact failure this whole
// row exists to close.
//
// ⚠️ EVERY OTHER ARM IN THIS FILE IS STRUCTURALLY BLIND TO IT — MEASURED, NOT
// ASSUMED. With the check moved after the CAS and this test absent, the full
// package read 448 PASS / 0 FAIL: superseded fires 1/1 both sides, disarm
// authority 0/400 both sides, single-pause NC 1/1 both sides. Nothing caught
// it. The reason is Stop(): at pace << watchdog every superseded timer is canceled
// ~1 ms after it is armed, ~99 ms before its deadline, so no superseded closure
// ever RUNS and there is nothing to mis-order. The damage is confined to a
// closure that has already ENTERED, so the probe has to CONSTRUCT that overlap
// rather than wait for it.
//
// THE CONSTRUCTION. Arm generation 1 with a SHORT watchdog. Then hold pauseMu
// and launch a superseding pause: beginDecodePause bumps the generation and
// sets the flag BEFORE it takes the lock, so generation 2 is live and owed a
// resume while generation 1's timer is still armed (it is only Stop()ped after
// the lock, which we hold). Generation 2's own watchdog is given a 10 s window
// so it cannot confound the reading. Then let generation 1's deadline elapse.
//
//	guard before CAS (correct): flag still true, 0 fires   — gen 2 keeps its resumer
//	CAS first                 : flag STOLEN                — gen 2 parks forever
//	no guard at all           : flag stolen AND a spurious ContinueDecoding
func TestP83_PauseGeneration_GuardPrecedesTheCAS(t *testing.T) {
	// NO t.Parallel — see the file header.
	const (
		shortWatchdog = 2 * time.Millisecond
		longWatchdog  = 10 * time.Second
		settle        = 500 * time.Microsecond
	)

	for _, encode := range []bool{false, true} {
		side := p83Opts{Encode: encode}.p83Side()

		f := &filter{}
		var read func() int
		var begin func()
		var paused func() bool
		if encode {
			cb := &p83CountingEncCb{}
			f.SetEncoderCallbacks(cb)
			read, paused = cb.load, f.encodePaused.Load
			begin = func() { f.beginEncodePause(p83HarnessCbEncode) }
		} else {
			cb := &p83CountingCb{}
			f.SetDecoderCallbacks(cb)
			read, paused = cb.load, f.decodePaused.Load
			begin = func() { f.beginDecodePause(p83HarnessCbDecode) }
		}

		// Generation 1, short window — this is the closure that must NOT touch
		// generation 2's flag.
		f.pauseWatchdog = shortWatchdog
		begin()

		// Generation 2 gets a window long enough that its own watchdog cannot
		// confound the reading. The write happens-before the `go` below.
		f.pauseWatchdog = longWatchdog

		f.pauseMu.Lock()
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			begin() // bumps the generation and sets the flag, THEN blocks on pauseMu
		}()
		time.Sleep(settle) // let it get past the bump and onto the lock

		// Generation 1's deadline elapses with generation 2 live and unresumed.
		time.Sleep(shortWatchdog + settle)
		stillPaused := paused()
		fires := read()
		f.pauseMu.Unlock()
		wg.Wait()
		f.stopPauseWatchdogs()

		// Logged UNCONDITIONALLY: a silent pass cannot distinguish a held flag
		// from a probe that never ran.
		t.Logf("guard-before-CAS [%s side]: stillPaused=%v fires=%d (gen-1 watchdog=%v, gen-2 watchdog=%v)",
			side, stillPaused, fires, shortWatchdog, longWatchdog)

		if !stillPaused {
			t.Errorf("guard-before-CAS [%s side]: a SUPERSEDED watchdog closure consumed the CURRENT pause's flag; want the flag still held.\n"+
				"The generation check must run BEFORE the CompareAndSwap. With the CAS first, the superseded closure takes the flag and only then bails, so the live pause has NO resumer: the guest's proxy_continue_stream loses its own CAS and the chain parks forever. This converts a spurious resume into an UNBOUNDED park.\n"+
				"PINNED CONFIGURATION: gen-1 watchdog=%v, gen-2 watchdog=%v, overlap forced via pauseMu.",
				side, shortWatchdog, longWatchdog)
		}
		if fires != 0 {
			t.Errorf("guard-before-CAS [%s side]: a superseded watchdog closure resumed the chain %d times; want 0. The generation guard is not rejecting it at all.",
				side, fires)
		}
	}
}

// TestP83_StopPauseTimer_IsAuthoritativeAgainstAnEnteredClosure is the S9
// anchor: the DISARM-AUTHORITY probe.
//
// THE HAZARD. Timer.Stop() cannot cancel a closure that has already entered.
// stopPauseTimer therefore has to be authoritative by GENERATION, not by
// handle: it must bump the counter so an in-flight closure fails its guard.
// Without the bump the closure passes the guard, wins the CAS and calls
// ContinueDecoding on a stream that is being torn down — an unmatched resume
// latched into the chain's buffered-1 resume channel.
//
// THE CONSTRUCTION, and why it is deterministic rather than a race hunt.
// stopPauseTimer takes f.pauseMu. The probe holds pauseMu itself, launches the
// disarm (which blocks on the lock, having ALREADY bumped the generation if
// the fix is present — the bump is ahead of the Lock for exactly this reason),
// and only then lets the watchdog window elapse. The closure and the disarm
// are thereby overlapped ON EVERY TRIAL, with no reliance on hitting a
// microsecond window:
//
//	fix absent : 400/400 late fires
//	fix present:   0/400
func TestP83_StopPauseTimer_IsAuthoritativeAgainstAnEnteredClosure(t *testing.T) {
	// NO t.Parallel — see the file header.
	const (
		watchdog = 2 * time.Millisecond
		settle   = 500 * time.Microsecond
	)

	for _, encode := range []bool{false, true} {
		side := p83Opts{Encode: encode}.p83Side()
		late := 0
		for i := 0; i < p83Trials; i++ {
			f := &filter{}
			f.pauseWatchdog = watchdog
			var read func() int
			if encode {
				cb := &p83CountingEncCb{}
				f.SetEncoderCallbacks(cb)
				read = cb.load
				f.beginEncodePause(p83HarnessCbEncode)
			} else {
				cb := &p83CountingCb{}
				f.SetDecoderCallbacks(cb)
				read = cb.load
				f.beginDecodePause(p83HarnessCbDecode)
			}

			// Hold the mutex so the disarm cannot complete, then start it. The
			// generation bump (if present) lands here; the handle release does not.
			f.pauseMu.Lock()
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				f.stopPauseTimer(!encode)
			}()
			time.Sleep(settle) // let the disarm reach the lock

			// Now let the watchdog window elapse with the disarm in flight.
			time.Sleep(watchdog + settle)
			fired := read()
			f.pauseMu.Unlock()
			wg.Wait()

			if fired > 0 {
				late++
			}
		}

		// Logged UNCONDITIONALLY: a silent pass cannot distinguish "0 late
		// fires over 400 trials" from "the loop never ran".
		t.Logf("disarm authority [%s side]: late=%d/%d trials, watchdog=%v, overlap forced via pauseMu",
			side, late, p83Trials, watchdog)

		if late != 0 {
			t.Errorf("disarm authority [%s side]: %d/%d trials saw a watchdog closure resume the chain AFTER stopPauseTimer was already in flight; want 0/%d.\n"+
				"PINNED CONFIGURATION: watchdog=%v, disarm-to-deadline overlap forced via pauseMu, trials=%d.\n"+
				"Timer.Stop() cannot cancel an entered closure, so stopPauseTimer must bump the generation counter BEFORE it takes pauseMu; without that bump this reads %d/%d.",
				side, late, p83Trials, p83Trials, watchdog, p83Trials, p83Trials, p83Trials)
		}
	}
}
