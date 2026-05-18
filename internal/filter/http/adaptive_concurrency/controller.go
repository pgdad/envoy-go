package adaptive_concurrency

// controller.go — the per-HCM-instance Gradient-1 adaptive-concurrency
// controller state machine per phase-21 SPEC §4 + §6.2 + ADR-0186 §Decision.
//
// # State-machine shape (per planner-time D17 + SPEC §6.2)
//
// Hot-path (lock-free) per D17:
//
//   - concurrencyLimit atomic.Uint32 — current limit, init = cfg.minRTTMinConcurrency per §4.6
//   - numRqOutstanding atomic.Uint32 — current in-flight count, init = 0
//
// Cold-path (under mu) per D17:
//
//   - latencySamples []time.Duration — appended on encodeComplete when NOT in minRTT window
//   - minRTTSamples  []time.Duration — appended on encodeComplete when IN minRTT window
//   - minRTT         time.Duration   — last computed minRTT
//   - deferredLimitValue uint32      — saved limit during minRTT window; 0 == NOT in minRTT window
//   - consecutiveMinConcurrencySet uint32 — 5-consecutive-min counter per AMEND-2 C3
//   - sampleResetTimer Stop          — periodic concurrency-update tick handle
//   - minRTTCalcTimer  Stop          — periodic minRTT-recalc trigger handle
//   - rng *rand.Rand                 — per-controller jitter source per planner-time D18
//
// # Algorithmic-invariant lemmata (line-cited against gradient_controller.cc)
//
//   - forwardingDecision: CAS loop over num_rq_outstanding < limit per cc:209-233
//   - gradient: clamp(0.5, min_rtt × (1 + buffer) / sample_rtt, 2.0) per cc:190-192
//   - new-limit: limit + sqrt(limit) double-clamped to [min_concurrency, max_concurrency_limit] per cc:198-206
//   - minRTT recalc: sample_aggregate_percentile-quantile NOT MIN per AMEND-2 C1 + cc:176-182
//   - jitter: additive-to-next-interval-delay per AMEND-2 C2 + cc:152-160
//   - 5-consecutive-min forced-recalc trigger per AMEND-2 C3 + cc:281-283
//   - first-tick-immediate-window-entry semantics per AMEND-2 C4 + cc:55-92
//
// # Stat-surface emission
//
// All 7 filterStats fields are consumed here:
//
//   - rqBlocked (counter): Inc on Block return from forwardingDecision
//   - concurrencyLimit (gauge): Set on every limit change (construction +
//     minRTT-window-entry + minRTT-window-close + every concurrency-update tick)
//   - gradient (gauge): Set per concurrency-update tick; int64 ×1000 per
//     ADR-0059 §Decision AMENDMENT
//   - burstQueueSize (gauge): Set per concurrency-update tick; int64 sqrt(limit*gradient)
//   - sampleRTTMsecs (gauge): Set per concurrency-update tick; int64 ns per
//     AMEND-3 C3 envoy-go-strict departure (NAME byte-exact upstream)
//   - minRTTMsecs (gauge): Set on minRTT recalc; same int64 ns encoding
//   - minRTTCalculationActive (gauge): Set 1 on window-entry, 0 on window-close;
//     via stats.BoolToInt(active)
//
// # Cross-references
//
//   - ADR-0186 (this Task's primary ADR landing; §Decision + §Consequences anchored here)
//   - ADR-0059 §Decision AMENDMENT (Task 4 landing; float-valued-gauge int64 encoding)
//   - ADR-0187 (Task 2 landing; enabled.RuntimeFeatureFlag PARSE-REJECT — cc.enabled gating)
//   - phase-21 SPEC §1.1 AMEND-1 + AMEND-2 + AMEND-3 + AMEND-6
//   - phase-21 SPEC §4.1-§4.6 + §6.2
//   - upstream Envoy v1.37.2 source: gradient_controller.cc (lemmata anchored above)

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

// gradientController is the per-HCM-instance Gradient-1 adaptive-concurrency
// controller per phase-21 SPEC §6.2. Constructed via newGradientController at
// filter build time (Task 9 wiring); the Task 5 + Task 6 filter glue calls
// forwardingDecision / recordLatencySample / releaseInFlight per the hot
// path + sample-recording + token-release lifecycle.
//
// Concurrency contract per planner-time D17:
//
//   - forwardingDecision is hot-path lock-free (atomic CAS only on
//     numRqOutstanding; atomic Load on concurrencyLimit).
//   - releaseInFlight is hot-path lock-free (atomic Add on numRqOutstanding).
//   - recordLatencySample acquires mu for sample-slice mutation + minRTT
//     state-machine transitions.
//   - Timer callbacks (concurrencyUpdateTick, updateMinRTTTick) acquire mu
//     for state mutation; their re-arm AfterFunc calls happen with mu
//     released (per the fakeClock re-entrancy contract at clock_test.go).
type gradientController struct {
	cfg   *compiledConfig
	stats *filterStats
	clock Clock

	// Hot-path atomics (lock-free) per planner-time D17.
	concurrencyLimit atomic.Uint32 // current limit; init = cfg.minRTTMinConcurrency per §4.6
	numRqOutstanding atomic.Uint32 // current in-flight count; init = 0

	// Cold-path state (under mu) per planner-time D17.
	mu                           sync.Mutex
	latencySamples               []time.Duration // appended when NOT in minRTT window
	minRTTSamples                []time.Duration // appended when IN minRTT window
	minRTT                       time.Duration   // last computed minRTT
	deferredLimitValue           uint32          // saved limit during minRTT window; 0 == NOT in window
	consecutiveMinConcurrencySet uint32          // 5-consecutive-min counter per AMEND-2 C3
	sampleResetTimer             Stop            // periodic concurrency-update tick
	minRTTCalcTimer              Stop            // periodic minRTT-recalc trigger
	rng                          *rand.Rand      // jitter source per planner-time D18; mu-protected
}

// newGradientController constructs the controller per AMEND-2 C4 first-tick
// semantics. Sets initial concurrencyLimit = cfg.minRTTMinConcurrency per
// §4.6; immediately enters the minRTT sampling window; arms both timers.
//
// Per AMEND-2 C4: at phase-21 MVP isMinRTTSamplingEnabled is always TRUE
// (fixed_value PARSE-REJECTed per ADR-0186 §Consequences (d) + §5.3), so
// the constructor unconditionally calls enterMinRTTSamplingWindowLocked.
func newGradientController(cfg *compiledConfig, st *filterStats, clock Clock) *gradientController {
	c := &gradientController{
		cfg:   cfg,
		stats: st,
		clock: clock,
		//nolint:gosec // jitter is non-crypto; per-controller seed via clock.Now per D18
		rng: rand.New(rand.NewSource(clock.Now().UnixNano())),
	}
	// Initial state per §4.6: concurrencyLimit = minConcurrency; numRqOutstanding = 0.
	c.concurrencyLimit.Store(cfg.minRTTMinConcurrency)
	c.stats.concurrencyLimit.Set(int64(cfg.minRTTMinConcurrency))

	// First-tick: immediately enter minRTT sampling window per AMEND-2 C4
	// (mirrors gradient_controller.cc:55-92, 66-80, 149). The
	// sampleResetTimer's callback short-circuits while in-window per §4.5;
	// updateMinRTT re-arms it when the window closes.
	c.mu.Lock()
	c.enterMinRTTSamplingWindowLocked()
	c.mu.Unlock()

	// Arm both timers. sampleResetTimer's callback short-circuits during the
	// initial minRTT window (deferredLimitValue != 0). minRTTCalcTimer's
	// callback would normally enter a window, but the controller is already
	// in-window, so it short-circuits too — re-arm happens at window close.
	c.sampleResetTimer = clock.AfterFunc(cfg.concurrencyUpdateInterval, c.concurrencyUpdateTick)
	c.minRTTCalcTimer = clock.AfterFunc(cfg.minRTTCalcInterval, c.updateMinRTTTick)

	return c
}

// forwardingDecision is the hot-path CAS loop per SPEC §4.1 +
// gradient_controller.cc:209-233. Returns true on Forward (CAS succeeded);
// false on Block (current_outstanding >= limit). On Block, increments the
// rq_blocked counter; the caller (Task 5 decode_headers.go) is responsible
// for emitting the 503-overflow wire shape per AMEND-6.
//
// Lock-free per planner-time D17 — no mutex acquisition on the request path.
func (c *gradientController) forwardingDecision() bool {
	for {
		limit := c.concurrencyLimit.Load()
		current := c.numRqOutstanding.Load()
		if current >= limit {
			c.stats.rqBlocked.Inc()
			return false
		}
		if c.numRqOutstanding.CompareAndSwap(current, current+1) {
			return true
		}
		// CAS failed (concurrent forwarder claimed the slot); retry the
		// load+check sequence with the latest values.
	}
}

// recordLatencySample appends rtt to the per-tick or per-recalc-window sample
// slice depending on minRTT-window state per SPEC §4.2 + §4.5. Acquires mu.
// Called by the encoder-side filter hook (Task 6) after rtt = clock.Now() -
// entryTime.
//
// When in-window, if the sample brings minRTTSamples to >= cfg.minRTTRequestCount
// the recalc fires inline (updateMinRTTLocked) — the recalc closes the window
// + re-arms both timers.
func (c *gradientController) recordLatencySample(rtt time.Duration) {
	c.mu.Lock()
	if c.deferredLimitValue != 0 {
		// In minRTT sampling window — append to recalc samples.
		c.minRTTSamples = append(c.minRTTSamples, rtt)
		if uint32(len(c.minRTTSamples)) >= c.cfg.minRTTRequestCount {
			c.updateMinRTTLocked()
		}
	} else {
		c.latencySamples = append(c.latencySamples, rtt)
	}
	c.mu.Unlock()
}

// releaseInFlight decrements numRqOutstanding atomically. Hot-path lock-free
// per planner-time D17. Called by the encoder-side hook (Task 6) AND by
// OnDestroy for the D11 token-release-on-reset-before-encode path (Task 6
// landing; deferred from Task 3 per the task split).
//
// Uses Add(^uint32(0)) which is unsigned-arithmetic -1 (the standard Go idiom
// for atomic decrement on Uint32).
func (c *gradientController) releaseInFlight() {
	c.numRqOutstanding.Add(^uint32(0))
}

// concurrencyUpdateTick is the sample_reset_timer callback per SPEC §4.2 +
// gradient_controller.cc:66-80. Aggregates the per-tick sample-RTT slice;
// computes gradient + new-limit; re-arms itself for the next tick.
//
// Short-circuits (without re-arming itself) when in minRTT window —
// updateMinRTT re-arms once the window closes per §4.5.
func (c *gradientController) concurrencyUpdateTick() {
	c.mu.Lock()
	if c.deferredLimitValue != 0 {
		// In minRTT sampling window — bail without re-arming. updateMinRTT
		// re-arms this timer when the window closes per §4.5 (j).
		c.mu.Unlock()
		return
	}
	if len(c.latencySamples) == 0 {
		// No samples accumulated this tick. Re-arm + return per cc:66-80.
		c.mu.Unlock()
		c.sampleResetTimer = c.clock.AfterFunc(c.cfg.concurrencyUpdateInterval, c.concurrencyUpdateTick)
		return
	}
	// Aggregate sample-RTT via sample_aggregate_percentile-quantile per
	// cc:176-182 + AMEND-2 C1 (sorted-slice percentile per §3.3 +
	// BRAINSTORM §8 item 4 carve-out — ≤ 1 bin-width divergence acceptable
	// per ADR-0186 §Decision).
	sampleRTT := Quantile(c.latencySamples, c.cfg.sampleAggregatePercentile)
	c.latencySamples = c.latencySamples[:0] // clear, preserving capacity
	c.stats.sampleRTTMsecs.Set(sampleRTT.Nanoseconds())
	// Compute gradient + new-limit per §4.3 + §4.4.
	gradient := computeGradient(c.minRTT, sampleRTT, c.cfg.minRTTBufferPct)
	c.stats.gradient.Set(int64(gradient * 1000)) // ×1000 per ADR-0059 AMENDMENT
	newLimit := c.calculateNewLimitLocked(gradient)
	c.updateConcurrencyLimitLocked(newLimit)
	c.mu.Unlock()
	// Re-arm for the next tick.
	c.sampleResetTimer = c.clock.AfterFunc(c.cfg.concurrencyUpdateInterval, c.concurrencyUpdateTick)
}

// updateMinRTTTick is the min_rtt_calc_timer callback per SPEC §4.5 trigger
// paths (i) timer-scheduled periodic + (ii) forced (5-consecutive-min path
// via AfterFunc(0, ...) per AMEND-2 C3).
//
// Per AMEND-2 C4 first-tick semantics: at construction, the controller is
// already in the minRTT window — if this fires while in-window (concurrent
// fire or race), short-circuit to avoid double-window-entry.
func (c *gradientController) updateMinRTTTick() {
	c.mu.Lock()
	if c.deferredLimitValue != 0 {
		// Already in window (e.g., the construction-time enter-window has
		// not yet closed via a sample-driven updateMinRTT). No-op.
		c.mu.Unlock()
		return
	}
	c.enterMinRTTSamplingWindowLocked()
	c.mu.Unlock()
	// sampleResetTimer's callback now short-circuits per the new in-window
	// state; updateMinRTT (called from recordLatencySample once the window
	// fills) re-arms both timers per §4.5 (j).
}

// enterMinRTTSamplingWindowLocked transitions the controller into the minRTT
// sampling window per SPEC §4.5 (a)-(e). Caller MUST hold mu.
//
// Steps (a)-(e) from §4.5:
//
//	(a) Save current limit to deferredLimitValue
//	(b) Clamp concurrencyLimit to cfg.minRTTMinConcurrency
//	(c) Clear minRTTSamples slice
//	(d) Set min_rtt_calculation_active gauge = 1
//	(e) (implicit) subsequent recordLatencySample calls append to minRTTSamples
func (c *gradientController) enterMinRTTSamplingWindowLocked() {
	c.deferredLimitValue = c.concurrencyLimit.Load()
	if c.deferredLimitValue == 0 {
		// Defensive: the saved limit must be non-zero so the
		// in-window discriminator (deferredLimitValue != 0) works. The
		// concurrencyLimit is always >= cfg.minRTTMinConcurrency which is
		// PARSE-REJECTed at >0 per Arm 6, so this should never happen in
		// practice — but pin defensively.
		c.deferredLimitValue = c.cfg.minRTTMinConcurrency
	}
	c.concurrencyLimit.Store(c.cfg.minRTTMinConcurrency)
	c.stats.concurrencyLimit.Set(int64(c.cfg.minRTTMinConcurrency))
	c.minRTTSamples = c.minRTTSamples[:0]
	c.stats.minRTTCalculationActive.Set(stats.BoolToInt(true))
}

// updateMinRTTLocked closes the minRTT sampling window per SPEC §4.5 (f)-(j).
// Caller MUST hold mu. Called from recordLatencySample once
// len(minRTTSamples) >= cfg.minRTTRequestCount.
//
// Steps (f)-(j) from §4.5:
//
//	(f) Compute new minRTT as sample_aggregate_percentile-quantile of
//	    minRTTSamples per AMEND-2 C1 (NOT MIN as BRAINSTORM hypothesized)
//	(g) Set min_rtt_msecs gauge to minRTT.Nanoseconds (envoy-go-strict per AMEND-3 C3)
//	(h) Set min_rtt_calculation_active gauge to 0
//	(i) Restore deferredLimitValue to concurrencyLimit
//	(j) Re-arm both timers (minRTTCalcTimer per applyJitter; sampleResetTimer
//	    per concurrencyUpdateInterval)
func (c *gradientController) updateMinRTTLocked() {
	if uint32(len(c.minRTTSamples)) < c.cfg.minRTTRequestCount {
		// Defensive guard — caller already checked, but pin invariant here.
		return
	}
	// Step (f) — percentile aggregation NOT MIN per AMEND-2 C1 + cc:176-182.
	minRTT := Quantile(c.minRTTSamples, c.cfg.sampleAggregatePercentile)
	c.minRTT = minRTT
	c.stats.minRTTMsecs.Set(minRTT.Nanoseconds()) // ns per AMEND-3 C3
	c.minRTTSamples = c.minRTTSamples[:0]
	// Step (h) — active flag 0.
	c.stats.minRTTCalculationActive.Set(stats.BoolToInt(false))
	// Step (i) — restore deferred limit.
	c.concurrencyLimit.Store(c.deferredLimitValue)
	c.stats.concurrencyLimit.Set(int64(c.deferredLimitValue))
	c.deferredLimitValue = 0
	// 5-consecutive-min counter resets at window close (a fresh measurement
	// cycle begins; the counter only increments per concurrency-update tick
	// inside updateConcurrencyLimitLocked).
	c.consecutiveMinConcurrencySet = 0
	// Step (j) — re-arm both timers. The minRTTCalcTimer interval is
	// jittered per AMEND-2 C2. The sampleResetTimer interval is the raw
	// concurrencyUpdateInterval (no jitter on this timer per cc:66-80).
	jittered := c.applyJitterLocked(c.cfg.minRTTCalcInterval, c.cfg.minRTTJitterPct)
	c.minRTTCalcTimer = c.clock.AfterFunc(jittered, c.updateMinRTTTick)
	c.sampleResetTimer = c.clock.AfterFunc(c.cfg.concurrencyUpdateInterval, c.concurrencyUpdateTick)
}

// calculateNewLimitLocked computes the new concurrency limit per SPEC §4.4 +
// gradient_controller.cc:198-206. Caller MUST hold mu (for burstQueueSize.Set;
// concurrencyLimit read is atomic).
//
// Formula:
//
//	limit         = currentLimit × gradient
//	burst_headroom = sqrt(limit)
//	new_limit     = uint32(limit + burst_headroom)  // truncation toward zero
//	new_limit     = clamp(new_limit, minConcurrency, maxConcurrencyLimit)
//
// The burst_queue_size gauge stores burst_headroom per §4.4 semantics
// correction (the gauge stores the per-tick burst-headroom value, NOT actual
// in-flight queued requests above the limit).
func (c *gradientController) calculateNewLimitLocked(gradient float64) uint32 {
	limit := float64(c.concurrencyLimit.Load()) * gradient
	burstHeadroom := math.Sqrt(limit)
	c.stats.burstQueueSize.Set(int64(burstHeadroom))
	newLimit := uint32(limit + burstHeadroom) //nolint:gosec // double-clamped below
	if newLimit < c.cfg.minRTTMinConcurrency {
		newLimit = c.cfg.minRTTMinConcurrency
	}
	if newLimit > c.cfg.maxConcurrencyLimit {
		newLimit = c.cfg.maxConcurrencyLimit
	}
	return newLimit
}

// updateConcurrencyLimitLocked atomically transitions concurrencyLimit to
// newLimit + updates the gauge + maintains the 5-consecutive-min forced-recalc
// counter per AMEND-2 C3 + gradient_controller.cc:281-283. Caller MUST hold mu.
//
// 5-consecutive-min forced-recalc trigger: when updateConcurrencyLimit is
// called with newLimit == oldLimit == minConcurrency for 5 consecutive ticks
// (outside a minRTT window), force-arm minRTTCalcTimer at 0ms. This guards
// against a stale-high minRTT pinning the system at min_concurrency forever.
//
// The counter resets to 0 whenever newLimit != minConcurrency OR the trigger
// fires (so we don't fire repeatedly on each subsequent tick).
func (c *gradientController) updateConcurrencyLimitLocked(newLimit uint32) {
	oldLimit := c.concurrencyLimit.Load()
	c.concurrencyLimit.Store(newLimit)
	c.stats.concurrencyLimit.Set(int64(newLimit))
	// 5-consecutive-min bookkeeping per AMEND-2 C3 + cc:281-283.
	if newLimit == c.cfg.minRTTMinConcurrency && oldLimit == c.cfg.minRTTMinConcurrency {
		c.consecutiveMinConcurrencySet++
		if c.consecutiveMinConcurrencySet >= 5 && c.deferredLimitValue == 0 {
			// Force-arm minRTTCalcTimer at 0ms — fires updateMinRTTTick
			// in the SAME Advance pass (fakeClock supports this re-entrancy
			// per clock_test.go::TestFakeClock_ReentrantAfterFunc; production
			// time.AfterFunc(0, ...) fires "immediately" per stdlib).
			if c.minRTTCalcTimer != nil {
				c.minRTTCalcTimer.Stop()
			}
			c.minRTTCalcTimer = c.clock.AfterFunc(0, c.updateMinRTTTick)
			c.consecutiveMinConcurrencySet = 0 // reset to avoid double-fire on next tick
		}
	} else {
		c.consecutiveMinConcurrencySet = 0
	}
}

// computeGradient computes the gradient per SPEC §4.3 +
// gradient_controller.cc:190-192. Pure function.
//
// Formula:
//
//	buffered_min_rtt = min_rtt × (1 + buffer_pct)
//	raw_gradient     = buffered_min_rtt / sample_rtt
//	gradient         = clamp(0.5, raw_gradient, 2.0)
//
// Clamp constants 0.5 and 2.0 are hard-coded (NOT configurable per upstream).
// The defensive sample_rtt == 0 path returns 1.0 (a no-op gradient — the
// concurrency-update tick caller path ensures sample_rtt > 0 by virtue of
// the latencySamples slice being non-empty + Quantile returning a sample
// from the slice; this path is for the unit-test edge case + future-proofing
// against divide-by-zero).
func computeGradient(minRTT, sampleRTT time.Duration, bufferPct float64) float64 {
	if sampleRTT == 0 {
		return 1.0
	}
	bufferedMinRTT := float64(minRTT) * (1.0 + bufferPct)
	raw := bufferedMinRTT / float64(sampleRTT)
	return math.Max(0.5, math.Min(2.0, raw))
}

// applyJitterLocked applies additive jitter to the next-interval delay per
// AMEND-2 C2 + gradient_controller.cc:152-160. Returns interval + uniform
// random [0, interval × jitter_pct). Caller MUST hold mu (uses c.rng).
//
// When jitter_pct == 0 (or interval × jitter_pct rounds to 0), returns
// interval unchanged. Default jitter_pct is 15% (= 0.15 fraction) per SPEC
// §6.1 default.
func (c *gradientController) applyJitterLocked(interval time.Duration, jitterPct float64) time.Duration {
	if jitterPct == 0 {
		return interval
	}
	jitterRange := time.Duration(float64(interval) * jitterPct)
	if jitterRange <= 0 {
		return interval
	}
	return interval + time.Duration(c.rng.Int63n(int64(jitterRange)))
}
