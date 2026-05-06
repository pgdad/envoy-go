package localratelimit

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenBucket_NewInitialFillEqualsMax(t *testing.T) {
	b := newTokenBucket(10, 1, 1*time.Second)
	if b.tokens != 10 {
		t.Errorf("initial tokens: got %d, want 10", b.tokens)
	}
	if b.maxTokens != 10 || b.tokensPerFill != 1 || b.fillInterval != 1*time.Second {
		t.Errorf("config: got max=%d perFill=%d interval=%v, want 10/1/1s", b.maxTokens, b.tokensPerFill, b.fillInterval)
	}
}

func TestTokenBucket_TryConsume_DepletesUntilZero(t *testing.T) {
	b := newTokenBucket(3, 1, 1*time.Hour) // huge interval to defeat refill during test
	for i := 0; i < 3; i++ {
		if !b.tryConsume() {
			t.Fatalf("tryConsume call %d of 3: got false, want true", i+1)
		}
	}
	if b.tokens != 0 {
		t.Errorf("after 3 consumes: tokens %d, want 0", b.tokens)
	}
	if b.tryConsume() {
		t.Error("4th tryConsume should fail (bucket empty)")
	}
}

func TestTokenBucket_TryConsume_ReturnsFalseWhenEmpty(t *testing.T) {
	b := newTokenBucket(1, 1, 1*time.Hour)
	_ = b.tryConsume() // drain
	for i := 0; i < 100; i++ {
		if b.tryConsume() {
			t.Errorf("tryConsume %d: got true, want false (bucket should stay empty)", i)
			return
		}
	}
}

func TestTokenBucket_LazyRefill_NoRefillBelowFillInterval(t *testing.T) {
	b := newTokenBucket(1, 1, 200*time.Millisecond)
	_ = b.tryConsume() // drain
	// Backdate lastRefillNs to simulate a 100ms elapsed window (less than 200ms).
	b.lastRefillNs = time.Now().UnixNano() - int64(100*time.Millisecond)
	if b.tryConsume() {
		t.Error("tryConsume after 100ms (< fill_interval=200ms): got true, want false (no refill expected)")
	}
}

func TestTokenBucket_LazyRefill_SingleQuantumRefill(t *testing.T) {
	b := newTokenBucket(1, 1, 200*time.Millisecond)
	_ = b.tryConsume() // drain to 0
	// Backdate lastRefillNs to simulate a 250ms elapsed window (>= 200ms).
	b.lastRefillNs = time.Now().UnixNano() - int64(250*time.Millisecond)
	if !b.tryConsume() {
		t.Error("tryConsume after 250ms (>= fill_interval=200ms): got false, want true (single-quantum refill)")
	}
	if b.tokens != 0 {
		t.Errorf("after refill+consume: tokens %d, want 0", b.tokens)
	}
}

func TestTokenBucket_LazyRefill_MultiQuantumRefill_CapAtMax(t *testing.T) {
	b := newTokenBucket(5, 2, 100*time.Millisecond)
	// Drain.
	for i := 0; i < 5; i++ {
		b.tryConsume()
	}
	if b.tokens != 0 {
		t.Fatalf("setup: drain left tokens %d, want 0", b.tokens)
	}
	// Backdate to simulate 5*100ms = 500ms elapsed → 5 refill quanta × 2 tokensPerFill = 10 tokens
	// → capped at maxTokens=5.
	b.lastRefillNs = time.Now().UnixNano() - int64(500*time.Millisecond)
	if !b.tryConsume() {
		t.Fatal("tryConsume after 500ms multi-quantum refill: got false, want true")
	}
	if b.tokens != 4 {
		// After multi-quantum refill: tokens=5 (capped from 10); consumed 1 → tokens=4.
		t.Errorf("after multi-quantum refill+consume: tokens %d, want 4 (5-cap then -1)", b.tokens)
	}
}

func TestTokenBucket_LazyRefill_LastRefillNsAdvancesByFullQuanta(t *testing.T) {
	b := newTokenBucket(10, 1, 200*time.Millisecond)
	_ = b.tryConsume() // consume 1; tokens=9; lastRefillNs unchanged (no refill since elapsed < interval)
	// Snapshot the baseline.
	baseline := b.lastRefillNs
	// Backdate to simulate 350ms elapsed → 1 quantum × 200ms; the residual 150ms must NOT be lost.
	b.lastRefillNs = baseline - int64(350*time.Millisecond)
	_ = b.tryConsume() // refill of 1; consume 1; net tokens unchanged at 9
	// Verify lastRefillNs advanced by exactly 1*200ms = 200ms from the backdated value
	// (NOT to nowNs which would lose the 150ms residual).
	expectedAdvance := int64(200 * time.Millisecond)
	actualAdvance := b.lastRefillNs - (baseline - int64(350*time.Millisecond))
	if actualAdvance != expectedAdvance {
		t.Errorf("lastRefillNs advance: got %dns, want %dns (must be quantum-aligned)", actualAdvance, expectedAdvance)
	}
}

// TestTokenBucket_ConcurrentTryConsume per planner-time decision 7.
// Fires tryConsume concurrently across 64 goroutines × 100 iterations with
// shared *tokenBucket; verifies no race; verifies total-allowed-count is
// bounded by initial-tokens + at-most-one-or-two refill-quanta during the
// sub-second test window.
//
// Run with `go test -race`; the race detector validates the mutex discipline
// mechanically. The sub-second runtime keeps refill quanta to ≤ 1 (fillInterval
// = 1*time.Hour; no refill expected during the test).
func TestTokenBucket_ConcurrentTryConsume(t *testing.T) {
	const goroutines = 64
	const iterations = 100
	const initialTokens = int64(1000) // > goroutines*iterations / 6.4 to ensure both true + false outcomes
	b := newTokenBucket(initialTokens, 1, 1*time.Hour)

	var allowed atomic.Int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if b.tryConsume() {
					allowed.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	total := allowed.Load()
	// Bound: with fillInterval=1h and a sub-second test window, zero refill
	// quanta can occur, so total must equal initialTokens exactly. The +1/-1
	// band is kept as a robustness margin against unanticipated scheduler skew.
	if total < 0 {
		t.Errorf("total allowed: %d, want >= 0", total)
	}
	if total > initialTokens+1 {
		t.Errorf("total allowed: %d, want <= %d (no refill quanta possible during sub-second test)", total, initialTokens+1)
	}
	if total < initialTokens-1 {
		// The 64*100=6400 attempts comfortably exceed initialTokens=1000, so
		// total should saturate at initialTokens.
		t.Errorf("total allowed: %d, want >= %d (saturation)", total, initialTokens-1)
	}
}
