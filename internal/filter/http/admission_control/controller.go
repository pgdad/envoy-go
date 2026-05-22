package admission_control

// controller.go — per-HCM-instance sliding-window success-rate controller per
// phase-23 SPEC §4 + §6.2 + ADR-0194 §Decision.
//
// # State-machine shape (per SPEC §6.2)
//
// All window state is held under a sync.Mutex (mu):
//
//   - buckets []bucket — per-second deque (oldest..newest) per AMEND-6
//     (mirrors upstream's std::deque<std::pair<MonotonicTime, RequestData>>
//     at thread_local_controller.h:111; granularity 1s `:14`).
//   - global struct{requests, successes uint64} — running aggregate over
//     all buckets in the window (mirrors global_data_ in upstream).
//
// No hot-path lock-free path is needed: upstream's thread_local_controller
// is thread-local-per-worker (no contention); envoy-go uses a single-instance
// controller per HCM instance, so mu guards all access on the one copy.
//
// # Algorithm (line-cited against upstream source at v1.37.2)
//
//  - recordRequest(success bool): per thread_local_controller.cc:30-54
//      (a) purge stale buckets older than samplingWindow (decrement global)
//      (b) roll a new bucket if newest bucket ts >= 1s older than now
//      (c) increment back bucket + global (requests; successes if success)
//
//  - requestCounts() (n, s uint64): per thread_local_controller.cc:67-70
//      purge stale, return global.requests, global.successes.
//
//  - averageRps() uint32: per thread_local_controller.cc:20-28
//      0 if empty; else global.requests / max(samplingWindow_secs, age_of_oldest_bucket_secs)
//
//  - shouldReject() bool: per admission_control.cc:161-179 (formula per AMEND-1):
//      n, s = requestCounts()
//      inner = (n - s/srThreshold) / (n+1)
//      p = inner                           if aggression == 1.0 (skip math.Pow)
//          math.Pow(inner, 1/aggression)   otherwise
//      p = math.Min(maxRejectionProbability, p)
//      p = math.Max(0, p)
//      r = rand.Uint64()
//      return float64(10000) * p > float64(r % 10000)  // strict >; accuracy=1e4
//
//  - classify(success bool): recordRequest(success) + stats.rqSuccess/rqFailure.Inc()
//
// # Integer-modulo reject decision (AMEND-2 + PD-6)
//
// The LOCKED byte-exact contract is:
//
//	float64(10000) * math.Max(p, 0.0) > float64(r % 10000)
//
// Strict >.  At equality (r%10000 == floor(10000·P)): FALSE → ADMIT.
// P=0 → 0 > (r%10000) is FALSE for every r → NEVER REJECT.
//
// # Cross-references
//
//   - ADR-0194 (this Task's primary ADR landing — §Decision body anchored here)
//   - phase-23 SPEC §4.1 (gating + formula) + §4.2 (window) + §6.2 (struct shape)
//   - phase-23 SPEC §1.1 AMEND-1 (formula refutation) + AMEND-2 (integer-modulo
//     decision refutation) + AMEND-6 (deque-of-buckets window) + AMEND-11 (record
//     discipline)
//   - upstream v1.37.2: admission_control.cc + thread_local_controller.{h,cc}

import (
	"math"
	"sync"
	"time"
)

// controller is the per-HCM-instance sliding-window success-rate controller per
// phase-23 SPEC §6.2. Constructed via newController at filter build time;
// DecodeHeaders calls shouldReject + averageRps; EncodeHeaders/EncodeTrailers
// call classify.
//
// All window fields are guarded by mu. clock.Now() drives bucket rollover +
// expiry. rand.Uint64() drives the dice-roll in shouldReject.
type controller struct {
	cfg   *compiledConfig
	stats *filterStats
	clock Clock
	rand  Rand

	mu      sync.Mutex
	buckets []bucket // per-second deque (oldest..newest) per AMEND-6
	global  struct {
		requests  uint64
		successes uint64
	}
}

// bucket is a per-second sliding-window bucket per SPEC §6.2 + AMEND-6.
// ts is the bucket start time (1s granularity per
// thread_local_controller.h:111 granularity defaultHistoryGranularity{1}).
type bucket struct {
	ts        time.Time
	requests  uint64
	successes uint64
}

// newController constructs the controller per SPEC §6.2. The controller starts
// with an empty window (zero global + empty buckets slice). All window state
// is built up lazily by the first recordRequest call.
func newController(cfg *compiledConfig, st *filterStats, clock Clock, rnd Rand) *controller {
	return &controller{
		cfg:   cfg,
		stats: st,
		clock: clock,
		rand:  rnd,
	}
}

// recordRequest records a single request outcome into the sliding-window per
// SPEC §4.2 + AMEND-6 + thread_local_controller.cc:30-54.
//
// Steps (mirrors upstream maybeUpdateHistoricalData + record):
//  1. Acquire mu.
//  2. Purge stale buckets older than samplingWindow (decrement global).
//  3. Roll a new bucket if newest bucket ts is >= 1s older than now.
//  4. Increment back bucket + global.
func (c *controller) recordRequest(success bool) {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeStaleLocked(now)
	c.maybeRolloverLocked(now)
	// Increment the back bucket.
	if len(c.buckets) == 0 {
		// No buckets yet (empty window after purge or first call).
		c.buckets = append(c.buckets, bucket{ts: now})
	}
	back := len(c.buckets) - 1
	c.buckets[back].requests++
	c.global.requests++
	if success {
		c.buckets[back].successes++
		c.global.successes++
	}
}

// requestCounts purges stale buckets then returns the (requests, successes)
// aggregate per SPEC §4.2 + thread_local_controller.cc:67-70.
func (c *controller) requestCounts() (n, s uint64) {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeStaleLocked(now)
	return c.global.requests, c.global.successes
}

// averageRps returns the windowed average requests-per-second per SPEC §4.2
// + thread_local_controller.cc:20-28.
//
//   - Returns 0 when the window is empty (no buckets).
//   - Denominator = max(samplingWindow_secs, age_of_oldest_sample_secs).
//     The age is measured from the oldest bucket's ts to now (in whole seconds).
//
// This mirrors upstream:
//
//	auto window_length = std::chrono::duration_cast<std::chrono::seconds>(
//	    std::max(sampling_window_, time_source_.monotonicTime() - buckets_.front().first));
//	return global_data_.requests / window_length.count();
func (c *controller) averageRps() uint32 {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeStaleLocked(now)
	if len(c.buckets) == 0 {
		return 0
	}
	// Age of oldest bucket in whole seconds (truncation toward zero).
	ageSecs := int64(now.Sub(c.buckets[0].ts).Seconds())
	if ageSecs < 0 {
		ageSecs = 0
	}
	windowSecs := int64(c.cfg.samplingWindow.Seconds())
	denomSecs := windowSecs
	// I-2: post-purge, ageSecs never exceeds windowSecs for integer-second windows
	// (every surviving bucket has age ≤ samplingWindow), so this branch is effectively
	// unreachable. Retained to mirror upstream's std::max(sampling_window_,
	// age_of_oldest_sample) semantics and guard against any future sub-second-window
	// change where truncation could produce ageSecs > windowSecs.
	if ageSecs > denomSecs {
		denomSecs = ageSecs
	}
	if denomSecs == 0 {
		// Sub-second window — avoid divide-by-zero; upstream truncates to seconds,
		// so a window of 0s returns 0 (consistent with empty-window behavior).
		return 0
	}
	//nolint:gosec // truncation to uint32 is intentional; RPS values are small
	return uint32(c.global.requests / uint64(denomSecs))
}

// shouldReject computes the rejection probability P_reject over the current
// window and returns true if the draw rejects per SPEC §4.1 + AMEND-1 +
// AMEND-2 + PD-6.
//
// Formula (AMEND-1, line-cited against admission_control.cc:161-179):
//
//	inner    = (n - s/srThreshold) / (n + 1)          // :167-168
//	p        = inner                                    // aggression==1.0 path
//	p        = math.Pow(inner, 1/aggression)            // aggression!=1.0 path (:170-171)
//	p        = math.Min(maxRejectionProbability, p)     // :173
//	(p floored at 0 implicitly via math.Max in decision)
//
// Decision (AMEND-2, line-cited against admission_control.cc:175-178):
//
//	r        = rand.Uint64()
//	return float64(10000) * math.Max(p, 0.0) > float64(r % 10000)
//
// STRICT >. P=0 → 0 > (r%10000) is FALSE for every r → NEVER REJECT.
func (c *controller) shouldReject() bool {
	n, s := c.requestCounts()

	totalRequests := float64(n)
	successfulRequests := float64(s)

	// Formula :167-168: numerator = total - successful/srThreshold; denominator = total+1.
	inner := (totalRequests - successfulRequests/c.cfg.srThreshold) / (totalRequests + 1)

	// Formula :170-171: apply aggression exponent; skip math.Pow when aggression==1.0
	// (avoids the needless Pow; matches upstream guard `if (aggression != 1.0)`).
	var p float64
	if c.cfg.aggression == 1.0 {
		p = inner
	} else {
		p = math.Pow(inner, 1.0/c.cfg.aggression)
	}

	// Formula :173: clamp to maxRejectionProbability.
	p = math.Min(c.cfg.maxRejectionProbability, p)

	// Decision :175-178: integer-modulo comparison with accuracy=1e4.
	// math.Max(p, 0.0) floors at 0 (the max(0,·) from the formula).
	// Strict > per upstream: r%10000==floor(10000·P) ADMITS (false at equality).
	r := c.rand.Uint64()
	return float64(10000)*math.Max(p, 0.0) > float64(r%10000)
}

// classify records a request outcome into the window AND increments the
// appropriate stats counter per SPEC §4.4 + AMEND-11.
//
// classify is called by the encode-side filter hook (EncodeHeaders /
// EncodeTrailers) for admitted, non-health-check, filter-enabled requests only
// (per AMEND-11: rejected, health-check, and disabled-filter requests are
// deliberately NOT recorded).
func (c *controller) classify(success bool) {
	c.recordRequest(success)
	if success {
		c.stats.rqSuccess.Inc()
	} else {
		c.stats.rqFailure.Inc()
	}
}

// purgeStaleLocked removes buckets from the front of the deque that are older
// than samplingWindow, decrementing global by their amounts.
// Caller MUST hold mu. Mirrors upstream's purge loop in
// thread_local_controller.cc:30-44.
//
// I-1 fix: after counting k stale front buckets, compact with copy(dst, src)
// so the backing array does not accumulate dropped bucket structs over the
// lifetime of a long-running controller (the old c.buckets[1:] idiom advanced
// the slice header without releasing the front of the array).
func (c *controller) purgeStaleLocked(now time.Time) {
	k := 0
	for k < len(c.buckets) {
		age := now.Sub(c.buckets[k].ts)
		if age <= c.cfg.samplingWindow {
			break
		}
		// Bucket is stale — decrement global.
		old := c.buckets[k]
		c.global.requests -= old.requests
		c.global.successes -= old.successes
		k++
	}
	if k > 0 {
		n := copy(c.buckets, c.buckets[k:])
		// Clear the tail so dropped bucket structs are GC-able (they contain
		// time.Time; no pointers, but keep tidy and avoid stale references).
		clear(c.buckets[n:])
		c.buckets = c.buckets[:n]
	}
}

// maybeRolloverLocked creates a new bucket if the newest bucket's ts is >= 1s
// older than now (the 1s-granularity per AMEND-6 + thread_local_controller.cc
// defaultHistoryGranularity{1}). Caller MUST hold mu.
//
// Only creates a new bucket when there is at least one existing bucket; the
// first bucket creation happens in recordRequest after the purgeStaleLocked +
// maybeRolloverLocked calls (when buckets is empty post-construction or post-
// full-purge, a new bucket is appended directly).
func (c *controller) maybeRolloverLocked(now time.Time) {
	if len(c.buckets) == 0 {
		return
	}
	newest := c.buckets[len(c.buckets)-1]
	if now.Sub(newest.ts) >= 1*time.Second {
		c.buckets = append(c.buckets, bucket{ts: now})
	}
}
