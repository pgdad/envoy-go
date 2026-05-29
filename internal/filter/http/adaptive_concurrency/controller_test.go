package adaptive_concurrency

// controller_test.go — Layer A FAKE-TIME algorithmic-fidelity tests + race
// tests for the Gradient-1 controller per phase-21 SPEC §14.1 Layer A +
// planner-time D3 + D15 + D17 + D18. Closes SPEC §12 items B6 + B7
// RATIFIED-PENDING-IMPL-TIME per D15.
//
// # Test taxonomy (10 families per planner-time D3)
//
//  1. TestController_FAKE_TIME_FirstTickSemantics — per AMEND-2 C4
//  2. TestController_FAKE_TIME_GradientFormula_* — per §4.3 + AMEND-2
//  3. TestController_FAKE_TIME_NewLimitCalculation_* — per §4.4
//  4. TestController_FAKE_TIME_MinRTTRecalcWindow_PercentileNotMin — per AMEND-2 C1
//  5. TestController_FAKE_TIME_JitterApplication_* — per AMEND-2 C2
//  6. TestController_FAKE_TIME_FiveConsecutiveMinForcedRecalc — per AMEND-2 C3
//  7. TestController_ConcurrentForwardingDecision_NConcurrent — race per §12 B7
//  8. TestController_ConcurrentForwardingDecision_NoDeadlockAtN1000 — race
//  9. TestController_FAKE_TIME_TimerOrdering_Deterministic — per D9 (delegated to
//     internal/clock which has the dedicated multi-timer determinism tests)
//  10. TestController_FAKE_TIME_RecordLatencySample_Routing — sample-slice routing
//
// # Deferred tests (per Task 5 + Task 6 split)
//
//   - TestController_503_BodyAndHeaders_ByteExact → Task 5 (decode_headers.go
//     landing; needs the SendLocalReply emission path which lives at the
//     filter layer, not the controller proper).
//   - TestFilter_OnDestroy_ReleasesAcquiredToken_* → Task 6 (encode_complete.go
//     + OnDestroy body landing; needs the per-stream filter struct's
//     acquired-token bookkeeping which lives at Task 5 + Task 6, not the
//     controller).
//
// The deferrals are documented in PROGRESS.md Task 3 entry per the implementer-
// subagent contract.

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/clock"
	"github.com/esalaine/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// testCompiledConfig returns a sensible default *compiledConfig for
// controller tests. Tests that need to vary specific fields call this then
// mutate the returned struct in-place before passing to newGradientController.
func testCompiledConfig() *compiledConfig {
	return &compiledConfig{
		enabled:                        true,
		concurrencyLimitExceededStatus: 503,
		sampleAggregatePercentile:      0.50, // p50
		maxConcurrencyLimit:            1000,
		concurrencyUpdateInterval:      100 * time.Millisecond,
		minRTTCalcInterval:             10 * time.Second,
		minRTTRequestCount:             50,
		minRTTJitterPct:                0.15, // 15%
		minRTTMinConcurrency:           3,
		minRTTBufferPct:                0.25, // 25%
	}
}

// testFilterStats returns a fresh *filterStats backed by a fresh
// *stats.Registry under the test prefix "http.test". Tests rarely inspect
// stat values directly (most assertions are on the controller's internal
// state); when needed, callers Load the relevant Gauge/Counter directly.
func testFilterStats() *filterStats {
	return newFilterStats(stats.NewRegistry(), "http.test")
}

// newTestController is the standard test-scope constructor combining the
// three helpers above with the fakeClock anchored at time.Unix(0, 0).
func newTestController(t *testing.T) (*gradientController, *clock.FakeClock) {
	t.Helper()
	cfg := testCompiledConfig()
	clk := clock.NewFakeClock(time.Unix(0, 0))
	c := newGradientController(cfg, testFilterStats(), clk)
	return c, clk
}

// -----------------------------------------------------------------------------
// 1. TestController_FAKE_TIME_FirstTickSemantics — per AMEND-2 C4
// -----------------------------------------------------------------------------

// TestController_FAKE_TIME_FirstTickSemantics verifies that the constructor
// per AMEND-2 C4 first-tick semantics: concurrencyLimit pinned to
// minRTTMinConcurrency; deferredLimitValue != 0 (in minRTT window);
// minRTTCalculationActive gauge = 1.
func TestController_FAKE_TIME_FirstTickSemantics(t *testing.T) {
	c, _ := newTestController(t)
	// (a) concurrencyLimit pinned to minConcurrency.
	if got, want := c.concurrencyLimit.Load(), uint32(3); got != want {
		t.Errorf("concurrencyLimit at construction = %d; want %d (minRTTMinConcurrency)", got, want)
	}
	// (b) numRqOutstanding starts at 0.
	if got := c.numRqOutstanding.Load(); got != 0 {
		t.Errorf("numRqOutstanding at construction = %d; want 0", got)
	}
	// (c) deferredLimitValue != 0 (in minRTT window).
	c.mu.Lock()
	deferred := c.deferredLimitValue
	c.mu.Unlock()
	if deferred == 0 {
		t.Errorf("deferredLimitValue at construction = 0; want != 0 (controller should be in minRTT window per AMEND-2 C4)")
	}
	// (d) minRTTCalculationActive gauge = 1.
	if got := c.stats.minRTTCalculationActive.Load(); got != 1 {
		t.Errorf("minRTTCalculationActive at construction = %d; want 1", got)
	}
	// (e) concurrencyLimit gauge mirrors atomic value.
	if got, want := c.stats.concurrencyLimit.Load(), int64(3); got != want {
		t.Errorf("concurrencyLimit gauge at construction = %d; want %d", got, want)
	}
}

// TestController_FAKE_TIME_ConcurrencyUpdateTickShortCircuitsInWindow verifies
// that the periodic concurrency-update tick callback short-circuits while
// the controller is in the minRTT sampling window (no gradient computed; no
// limit change). Per AMEND-2 C4 + §4.5 (no re-arm here; updateMinRTT re-arms
// at window close).
func TestController_FAKE_TIME_ConcurrencyUpdateTickShortCircuitsInWindow(t *testing.T) {
	c, clk := newTestController(t)
	// Record limit before advance; advance past the concurrency-update
	// interval; verify limit unchanged.
	limitBefore := c.concurrencyLimit.Load()
	clk.Advance(c.cfg.concurrencyUpdateInterval + 1*time.Millisecond)
	if got := c.concurrencyLimit.Load(); got != limitBefore {
		t.Errorf("concurrencyLimit changed during minRTT window: before=%d after=%d", limitBefore, got)
	}
	// Verify minRTTCalculationActive stayed at 1.
	if got := c.stats.minRTTCalculationActive.Load(); got != 1 {
		t.Errorf("minRTTCalculationActive after tick = %d; want 1 (still in window)", got)
	}
}

// -----------------------------------------------------------------------------
// 2. TestController_FAKE_TIME_GradientFormula_* — per §4.3 + AMEND-2
// -----------------------------------------------------------------------------

func TestController_FAKE_TIME_GradientFormula_NoBuffer(t *testing.T) {
	got := computeGradient(100*time.Millisecond, 100*time.Millisecond, 0.0)
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("computeGradient(100ms, 100ms, 0.0) = %v; want 1.0", got)
	}
}

func TestController_FAKE_TIME_GradientFormula_25PctBuffer(t *testing.T) {
	got := computeGradient(100*time.Millisecond, 100*time.Millisecond, 0.25)
	if math.Abs(got-1.25) > 1e-9 {
		t.Errorf("computeGradient(100ms, 100ms, 0.25) = %v; want 1.25", got)
	}
}

func TestController_FAKE_TIME_GradientFormula_ClampLow(t *testing.T) {
	// min_rtt=10ms, sample_rtt=1000ms, no buffer → raw = 10/1000 = 0.01 → clamped to 0.5
	got := computeGradient(10*time.Millisecond, 1000*time.Millisecond, 0.0)
	if math.Abs(got-0.5) > 1e-9 {
		t.Errorf("computeGradient(10ms, 1000ms, 0.0) = %v; want 0.5 (clamped low)", got)
	}
}

func TestController_FAKE_TIME_GradientFormula_ClampHigh(t *testing.T) {
	// min_rtt=1000ms, sample_rtt=10ms, no buffer → raw = 100 → clamped to 2.0
	got := computeGradient(1000*time.Millisecond, 10*time.Millisecond, 0.0)
	if math.Abs(got-2.0) > 1e-9 {
		t.Errorf("computeGradient(1000ms, 10ms, 0.0) = %v; want 2.0 (clamped high)", got)
	}
}

func TestController_FAKE_TIME_GradientFormula_ZeroSampleRTT(t *testing.T) {
	// Defensive: sample_rtt=0 returns 1.0 (no-op gradient).
	got := computeGradient(100*time.Millisecond, 0, 0.25)
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("computeGradient(100ms, 0, 0.25) = %v; want 1.0 (defensive zero-sample-rtt)", got)
	}
}

// -----------------------------------------------------------------------------
// 3. TestController_FAKE_TIME_NewLimitCalculation_* — per §4.4
// -----------------------------------------------------------------------------

func TestController_FAKE_TIME_NewLimitCalculation_NoChange(t *testing.T) {
	c, _ := newTestController(t)
	c.cfg.minRTTMinConcurrency = 3
	c.cfg.maxConcurrencyLimit = 1000
	c.concurrencyLimit.Store(100)
	c.mu.Lock()
	got := c.calculateNewLimitLocked(1.0)
	c.mu.Unlock()
	// limit = 100*1.0 = 100; burst_headroom = sqrt(100) = 10; new = 110.
	if got != 110 {
		t.Errorf("calculateNewLimit(100, gradient=1.0) = %d; want 110", got)
	}
}

func TestController_FAKE_TIME_NewLimitCalculation_ClampMin(t *testing.T) {
	c, _ := newTestController(t)
	c.cfg.minRTTMinConcurrency = 5
	c.cfg.maxConcurrencyLimit = 1000
	c.concurrencyLimit.Store(10)
	c.mu.Lock()
	got := c.calculateNewLimitLocked(0.001)
	c.mu.Unlock()
	// limit = 0.01; sqrt = 0.1; sum = 0.11 → uint32 0 → clamped to 5.
	if got != 5 {
		t.Errorf("calculateNewLimit(10, gradient=0.001) = %d; want 5 (clamped to minConcurrency)", got)
	}
}

func TestController_FAKE_TIME_NewLimitCalculation_ClampMax(t *testing.T) {
	c, _ := newTestController(t)
	c.cfg.minRTTMinConcurrency = 3
	c.cfg.maxConcurrencyLimit = 1000
	c.concurrencyLimit.Store(900)
	c.mu.Lock()
	got := c.calculateNewLimitLocked(2.0)
	c.mu.Unlock()
	// limit = 1800; sqrt ≈ 42.43; sum ≈ 1842 → clamped to 1000.
	if got != 1000 {
		t.Errorf("calculateNewLimit(900, gradient=2.0) = %d; want 1000 (clamped to maxConcurrencyLimit)", got)
	}
}

// -----------------------------------------------------------------------------
// 4. TestController_FAKE_TIME_MinRTTRecalcWindow_PercentileNotMin — per AMEND-2 C1
// -----------------------------------------------------------------------------

// TestController_FAKE_TIME_MinRTTRecalcWindow_PercentileNotMin closes the
// load-bearing AMEND-2 C1 lemma: minRTT recalc takes the
// sample_aggregate_percentile-quantile (default p50) of recalc-window
// samples, NOT the MIN as BRAINSTORM hypothesized.
//
// Configure cfg.minRTTRequestCount = 6 + cfg.sampleAggregatePercentile = 0.5.
// Feed samples [50, 60, 70, 80, 90, 100]ms. Quantile (p50) of 6 sorted
// samples: idx = int(0.5 * 5) = 2 → sorted[2] = 70ms. NOT the MIN 50ms.
func TestController_FAKE_TIME_MinRTTRecalcWindow_PercentileNotMin(t *testing.T) {
	c, _ := newTestController(t)
	c.cfg.minRTTRequestCount = 6
	c.cfg.sampleAggregatePercentile = 0.5
	// Controller is already in minRTT window from construction (AMEND-2 C4).
	// Feed 6 samples — the 6th triggers updateMinRTTLocked inline.
	samples := []time.Duration{
		50 * time.Millisecond,
		60 * time.Millisecond,
		70 * time.Millisecond,
		80 * time.Millisecond,
		90 * time.Millisecond,
		100 * time.Millisecond,
	}
	for _, s := range samples {
		c.recordLatencySample(s)
	}
	// After the 6th sample, the window closes. Verify:
	// (a) c.minRTT == 70ms (p50 NOT MIN per AMEND-2 C1).
	c.mu.Lock()
	gotMinRTT := c.minRTT
	deferred := c.deferredLimitValue
	c.mu.Unlock()
	if gotMinRTT != 70*time.Millisecond {
		t.Errorf("minRTT after recalc = %v; want 70ms (p50 of [50..100]); NOT 50ms (MIN). AMEND-2 C1 lemma broken.", gotMinRTT)
	}
	// (b) deferredLimitValue restored to 0 (window closed).
	if deferred != 0 {
		t.Errorf("deferredLimitValue after recalc = %d; want 0 (window should be closed)", deferred)
	}
	// (c) minRTTCalculationActive gauge = 0.
	if got := c.stats.minRTTCalculationActive.Load(); got != 0 {
		t.Errorf("minRTTCalculationActive after recalc = %d; want 0", got)
	}
	// (d) minRTTMsecs gauge stores 70ms in nanoseconds.
	if got, want := c.stats.minRTTMsecs.Load(), int64((70 * time.Millisecond).Nanoseconds()); got != want {
		t.Errorf("minRTTMsecs gauge = %d ns; want %d ns (70ms)", got, want)
	}
}

// -----------------------------------------------------------------------------
// 5. TestController_FAKE_TIME_JitterApplication_* — per AMEND-2 C2
// -----------------------------------------------------------------------------

// TestController_FAKE_TIME_JitterApplication_InRange verifies the jitter
// output is in [interval, interval + interval*jitter_pct) per AMEND-2 C2 +
// cc:152-160. Runs 100 iterations to sample the distribution.
func TestController_FAKE_TIME_JitterApplication_InRange(t *testing.T) {
	c, _ := newTestController(t)
	interval := 1000 * time.Millisecond
	jitterPct := 0.15 // 15%
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := 0; i < 100; i++ {
		got := c.applyJitterLocked(interval, jitterPct)
		if got < interval {
			t.Errorf("iter %d: applyJitter = %v; want >= %v", i, got, interval)
		}
		maxExclusive := interval + time.Duration(float64(interval)*jitterPct)
		if got >= maxExclusive {
			t.Errorf("iter %d: applyJitter = %v; want < %v", i, got, maxExclusive)
		}
	}
}

// TestController_FAKE_TIME_JitterApplication_ZeroPct verifies the no-jitter
// short-circuit path returns interval unchanged.
func TestController_FAKE_TIME_JitterApplication_ZeroPct(t *testing.T) {
	c, _ := newTestController(t)
	c.mu.Lock()
	defer c.mu.Unlock()
	got := c.applyJitterLocked(1000*time.Millisecond, 0.0)
	if got != 1000*time.Millisecond {
		t.Errorf("applyJitter(1s, 0.0) = %v; want 1s (no-jitter short-circuit)", got)
	}
}

// -----------------------------------------------------------------------------
// 6. TestController_FAKE_TIME_FiveConsecutiveMinForcedRecalc — per AMEND-2 C3
// -----------------------------------------------------------------------------

// TestController_FAKE_TIME_FiveConsecutiveMinForcedRecalc closes AMEND-2 C3:
// 5 consecutive concurrency-update ticks with newLimit == oldLimit ==
// minConcurrency (outside a minRTT window) force-arm minRTTCalcTimer at 0ms.
//
// Strategy: close the initial minRTT window first by recording enough
// samples. Then drive 5 calls to updateConcurrencyLimitLocked with
// newLimit == minConcurrency outside the window; verify the 5th call
// force-arms a 0ms timer which fires updateMinRTTTick → re-enters window.
func TestController_FAKE_TIME_FiveConsecutiveMinForcedRecalc(t *testing.T) {
	c, clk := newTestController(t)
	c.cfg.minRTTRequestCount = 3
	c.cfg.sampleAggregatePercentile = 0.5
	// Close the initial window by feeding enough samples.
	c.recordLatencySample(10 * time.Millisecond)
	c.recordLatencySample(20 * time.Millisecond)
	c.recordLatencySample(30 * time.Millisecond)
	c.mu.Lock()
	if c.deferredLimitValue != 0 {
		c.mu.Unlock()
		t.Fatalf("setup: initial minRTT window did not close; deferredLimitValue = %d", c.deferredLimitValue)
	}
	c.mu.Unlock()
	// Now drive 5 consecutive limit-stays-at-min updates.
	for i := 0; i < 5; i++ {
		c.mu.Lock()
		c.updateConcurrencyLimitLocked(c.cfg.minRTTMinConcurrency)
		c.mu.Unlock()
	}
	// The 5th call should have force-armed minRTTCalcTimer at 0ms. Advance
	// the clock by 0 — the re-entrant AfterFunc(0, ...) fires in the same
	// Advance pass per internal/clock::TestFakeClock_ReentrantAfterFunc.
	clk.Advance(1 * time.Microsecond) // tiny step to trigger pending 0ms timer
	// Verify the force-armed minRTTCalcTimer fired updateMinRTTTick which
	// entered a fresh minRTT window.
	c.mu.Lock()
	deferred := c.deferredLimitValue
	c.mu.Unlock()
	if deferred == 0 {
		t.Errorf("after 5-consecutive-min: deferredLimitValue = 0; want != 0 (forced recalc should have entered window)")
	}
	if got := c.stats.minRTTCalculationActive.Load(); got != 1 {
		t.Errorf("after 5-consecutive-min: minRTTCalculationActive = %d; want 1 (in window)", got)
	}
}

// -----------------------------------------------------------------------------
// 7+8. TestController_ConcurrentForwardingDecision_* — race tests per §12 B7 + D17
// -----------------------------------------------------------------------------

// TestController_ConcurrentForwardingDecision_NConcurrent runs N goroutines
// each calling forwardingDecision() against a configured limit K. Verifies:
//
//   - exactly K return true (Forward)
//   - exactly N-K return false (Block)
//   - numRqOutstanding == K post-test (no leaks)
//   - rqBlocked counter == N-K
//
// Closes SPEC §12 item B7 (CAS-vs-mutex contention behavior at scale).
func TestController_ConcurrentForwardingDecision_NConcurrent(t *testing.T) {
	const N = 1000
	const K = 100
	c, _ := newTestController(t)
	// Close the initial minRTT window so the limit is at K, not minConcurrency.
	c.concurrencyLimit.Store(K)
	c.mu.Lock()
	c.deferredLimitValue = 0
	c.mu.Unlock()

	var forwardCount, blockCount atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if c.forwardingDecision() {
				forwardCount.Add(1)
			} else {
				blockCount.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := forwardCount.Load(); got != K {
		t.Errorf("forwardCount = %d; want %d", got, K)
	}
	if got := blockCount.Load(); got != N-K {
		t.Errorf("blockCount = %d; want %d", got, N-K)
	}
	if got := c.numRqOutstanding.Load(); got != K {
		t.Errorf("numRqOutstanding post-test = %d; want %d (K forwarders never released)", got, K)
	}
	if got, want := c.stats.rqBlocked.Load(), uint64(N-K); got != want {
		t.Errorf("rqBlocked counter = %d; want %d", got, want)
	}
}

// TestController_ConcurrentForwardingDecision_NoDeadlockAtN1000 verifies no
// deadlock under N=1000 with K=1 (the heavily-contended worst-case CAS
// scenario). Uses a watchdog timer to fail-fast on deadlock.
func TestController_ConcurrentForwardingDecision_NoDeadlockAtN1000(t *testing.T) {
	const N = 1000
	const K = 1
	c, _ := newTestController(t)
	c.concurrencyLimit.Store(K)
	c.mu.Lock()
	c.deferredLimitValue = 0
	c.mu.Unlock()

	var wg sync.WaitGroup
	done := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.forwardingDecision()
		}()
	}
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// Successful completion.
	case <-time.After(5 * time.Second):
		t.Fatal("forwardingDecision deadlocked under N=1000 + K=1")
	}
	// Sanity: exactly K acquired.
	if got := c.numRqOutstanding.Load(); got != K {
		t.Errorf("numRqOutstanding = %d; want %d", got, K)
	}
}

// TestController_ReleaseInFlight_Decrements verifies the hot-path release
// path decrements numRqOutstanding atomically. Covers the D17 contract.
func TestController_ReleaseInFlight_Decrements(t *testing.T) {
	c, _ := newTestController(t)
	c.concurrencyLimit.Store(10)
	c.mu.Lock()
	c.deferredLimitValue = 0
	c.mu.Unlock()
	// Acquire 3 tokens.
	for i := 0; i < 3; i++ {
		if !c.forwardingDecision() {
			t.Fatalf("iter %d: forwardingDecision returned false", i)
		}
	}
	if got := c.numRqOutstanding.Load(); got != 3 {
		t.Fatalf("post-3-acquire: numRqOutstanding = %d; want 3", got)
	}
	// Release 2.
	c.releaseInFlight()
	c.releaseInFlight()
	if got := c.numRqOutstanding.Load(); got != 1 {
		t.Errorf("post-2-release: numRqOutstanding = %d; want 1", got)
	}
}

// -----------------------------------------------------------------------------
// 10. TestController_FAKE_TIME_RecordLatencySample_Routing
// -----------------------------------------------------------------------------

// TestController_FAKE_TIME_RecordLatencySample_RoutesToMinRTTSamplesInWindow
// verifies that recordLatencySample routes to minRTTSamples while in window
// (controller starts in-window per AMEND-2 C4).
func TestController_FAKE_TIME_RecordLatencySample_RoutesToMinRTTSamplesInWindow(t *testing.T) {
	c, _ := newTestController(t)
	c.cfg.minRTTRequestCount = 100 // high so the window doesn't auto-close on us
	c.recordLatencySample(50 * time.Millisecond)
	c.recordLatencySample(60 * time.Millisecond)
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.minRTTSamples) != 2 {
		t.Errorf("minRTTSamples len = %d; want 2 (controller starts in-window)", len(c.minRTTSamples))
	}
	if len(c.latencySamples) != 0 {
		t.Errorf("latencySamples len = %d; want 0 (should not route to per-tick slice in-window)", len(c.latencySamples))
	}
}

// TestController_FAKE_TIME_RecordLatencySample_RoutesToLatencySamplesOutOfWindow
// verifies that recordLatencySample routes to latencySamples when not in
// window. Closes the window first via enough samples.
func TestController_FAKE_TIME_RecordLatencySample_RoutesToLatencySamplesOutOfWindow(t *testing.T) {
	c, _ := newTestController(t)
	c.cfg.minRTTRequestCount = 3
	c.cfg.sampleAggregatePercentile = 0.5
	// Close initial window.
	c.recordLatencySample(10 * time.Millisecond)
	c.recordLatencySample(20 * time.Millisecond)
	c.recordLatencySample(30 * time.Millisecond)
	c.mu.Lock()
	if c.deferredLimitValue != 0 {
		c.mu.Unlock()
		t.Fatalf("setup: window did not close; deferredLimitValue = %d", c.deferredLimitValue)
	}
	c.mu.Unlock()
	// Now record samples that should land in latencySamples.
	c.recordLatencySample(40 * time.Millisecond)
	c.recordLatencySample(50 * time.Millisecond)
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.latencySamples) != 2 {
		t.Errorf("latencySamples len = %d; want 2", len(c.latencySamples))
	}
	if len(c.minRTTSamples) != 0 {
		t.Errorf("minRTTSamples len = %d; want 0 (window closed; should route to per-tick slice)", len(c.minRTTSamples))
	}
}

// -----------------------------------------------------------------------------
// End-to-end tick driver — exercises the periodic concurrency-update tick
// path post-initial-window-close. Validates the gauge-emission discipline.
// -----------------------------------------------------------------------------

// TestController_FAKE_TIME_ConcurrencyUpdateTick_EmitsGaugesAfterWindowClose
// drives a full lifecycle: initial window opens; window closes via samples;
// post-window concurrency-update tick fires + emits gradient + burst_queue_size
// + sample_rtt_msecs gauges.
func TestController_FAKE_TIME_ConcurrencyUpdateTick_EmitsGaugesAfterWindowClose(t *testing.T) {
	c, clk := newTestController(t)
	c.cfg.minRTTRequestCount = 3
	c.cfg.sampleAggregatePercentile = 0.5
	c.cfg.concurrencyUpdateInterval = 100 * time.Millisecond
	// Close initial window with 3 samples (minRTT becomes 20ms = p50 of [10,20,30]).
	c.recordLatencySample(10 * time.Millisecond)
	c.recordLatencySample(20 * time.Millisecond)
	c.recordLatencySample(30 * time.Millisecond)
	c.mu.Lock()
	if c.minRTT != 20*time.Millisecond {
		c.mu.Unlock()
		t.Fatalf("setup: minRTT = %v; want 20ms (p50 of [10,20,30])", c.minRTT)
	}
	c.mu.Unlock()
	// Now record per-tick samples (sample_rtt path).
	c.recordLatencySample(40 * time.Millisecond)
	c.recordLatencySample(60 * time.Millisecond)
	c.recordLatencySample(80 * time.Millisecond)
	// Advance past concurrency-update interval — the tick fires.
	clk.Advance(101 * time.Millisecond)
	// Verify: sample_rtt_msecs gauge updated to 60ms (p50 of [40,60,80] = idx 1).
	if got, want := c.stats.sampleRTTMsecs.Load(), int64((60 * time.Millisecond).Nanoseconds()); got != want {
		t.Errorf("sampleRTTMsecs gauge = %d ns; want %d ns (60ms p50 of [40,60,80])", got, want)
	}
	// Verify gradient gauge updated. Gradient = clamp(0.5, 20*(1+0.25)/60, 2.0)
	//                                          = clamp(0.5, 25/60, 2.0)
	//                                          = clamp(0.5, 0.4167, 2.0)
	//                                          = 0.5 (clamped low)
	// gradient × 1000 = 500.
	if got, want := c.stats.gradient.Load(), int64(500); got != want {
		t.Errorf("gradient gauge = %d (× 1000); want %d (0.5 × 1000)", got, want)
	}
	// Verify burst_queue_size gauge updated. limit_pre = 3 (minConcurrency).
	// new limit calc: 3 × 0.5 = 1.5; sqrt(1.5) ≈ 1.224 → stored as int64 1.
	if got := c.stats.burstQueueSize.Load(); got != 1 {
		t.Errorf("burstQueueSize gauge = %d; want 1 (sqrt(3*0.5) ≈ 1.22 → int64 1)", got)
	}
}
