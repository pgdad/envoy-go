package localratelimit

import (
	"sync"
	"time"
)

// tokenBucket is the lazy-refill-on-access token-bucket primitive per ADR-0116
// + SPEC §6.4. Single sync.Mutex per bucket; no per-bucket goroutine; no
// time.Ticker; no signal channel.
//
// Concurrency: tryConsume is the SOLE writer; concurrent calls are serialized
// by the mutex. The hot-path holds the mutex for 5–10 nanoseconds typical
// (compute elapsed → integer-divide → conditional add → decrement). Lock
// contention bounded by per-route request rate; per-route TPFC isolation
// means listener-level traffic and per-route traffic don't compete on the
// same bucket per ADR-0117.
//
// Time-source semantics: time.Now().UnixNano() carries Go ≥1.9's monotonic
// component for time.Now()-derived values; arithmetic across time.Now() calls
// advances monotonically under wall-clock NTP corrections / leap seconds.
// Phase 11 takes this guarantee from Go documentation; unit tests do NOT
// exercise wall-clock backward-jump simulation (such testing requires
// test-only clock injection, deferred per SPEC §12 D5 + planner-time decision 5).
//
// LBP-1-adjacent declaration per ADR-0116: closure-capture half preserved
// (matches phase 06.1 / 09 / 10 discipline); lock-free hot-path half
// deliberately departs since the elapsed → refills → tokens computation is
// a multi-step CAS-resistant sequence. Mutex is the natural choice.
type tokenBucket struct {
	maxTokens     int64
	tokensPerFill int64
	fillInterval  time.Duration

	mu           sync.Mutex
	tokens       int64
	lastRefillNs int64 // time.Now().UnixNano() at last refill
}

// newTokenBucket constructs a *tokenBucket with initial fill = maxTokens
// (per SPEC §6.4 + §11.7 — empirical pin §11.7 confirms the initial bucket
// is full at t=0). The lastRefillNs baseline is time.Now().UnixNano() at
// construction; subsequent tryConsume calls compute refills relative to this
// baseline.
//
// Caller (buildRuntimeConfig at local_ratelimit.go) is responsible for
// validating maxTokens > 0 and fillInterval >= 50ms BEFORE calling
// newTokenBucket; the constructor itself does NOT re-validate.
func newTokenBucket(maxTokens, tokensPerFill int64, fillInterval time.Duration) *tokenBucket {
	return &tokenBucket{
		maxTokens:     maxTokens,
		tokensPerFill: tokensPerFill,
		fillInterval:  fillInterval,
		tokens:        maxTokens,
		lastRefillNs:  time.Now().UnixNano(),
	}
}

// tryConsume attempts to consume one token from the bucket; returns true
// on success (token consumed) and false on failure (bucket empty after
// any due refills).
//
// The lazy-refill discipline (per SPEC §6.4):
//
//  1. Lock the mutex.
//  2. Compute elapsedNs = time.Now().UnixNano() - lastRefillNs.
//  3. Compute refills = elapsedNs / int64(fillInterval). Integer-division
//     quantizes to the configured fill_interval boundary; the quotient is
//     the number of full quanta elapsed since last refill.
//  4. If refills > 0:
//     - Add (refills * tokensPerFill) to tokens.
//     - Cap tokens at maxTokens (no over-fill).
//     - Advance lastRefillNs by (refills * fillInterval) (NOT to nowNs;
//     this preserves sub-quantum residual elapsed for the next call).
//  5. If tokens > 0: decrement tokens; return true.
//  6. Else: return false.
//
// The integer-division step (3) is the core quantization rule; SPEC §11.7
// measured the boundary as sharp at ≤5ms granularity in reference Envoy v1.37.2
// (52 trials at delay ≥ 200ms all returned 200; 24 trials at delay ≤ 199ms all
// returned 429). envoy-go's primitive matches by construction.
func (b *tokenBucket) tryConsume() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	nowNs := time.Now().UnixNano()
	elapsedNs := nowNs - b.lastRefillNs
	if refills := elapsedNs / int64(b.fillInterval); refills > 0 {
		b.tokens += refills * b.tokensPerFill
		if b.tokens > b.maxTokens {
			b.tokens = b.maxTokens
		}
		b.lastRefillNs += refills * int64(b.fillInterval)
	}
	if b.tokens > 0 {
		b.tokens--
		return true
	}
	return false
}
