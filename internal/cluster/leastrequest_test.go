package cluster

import (
	"sync"
	"testing"
)

// seqRNG returns a deterministic rng closure yielding the given values in order,
// then repeating — the upstream mock-RNG posture (AMEND-L6).
func seqRNG(vals ...uint64) func() uint64 {
	i := 0
	return func() uint64 {
		v := vals[i%len(vals)]
		i++
		return v
	}
}

func eps(n int) []Endpoint {
	out := make([]Endpoint, n)
	for i := range out {
		out[i] = Endpoint{Host: string(rune('a' + i)), Port: uint32(1000 + i)}
	}
	return out
}

func TestLeastRequest_FirstDrawnWinsTies(t *testing.T) {
	// All counters 0; choiceCount 2; draws {0, 1}. Strict < keeps the first-drawn
	// (index 0) on a tie. winner = endpoints[0].
	lr := newLeastRequestWithRNG(eps(3), 2, seqRNG(0, 1))
	ep, _, err := lr.Pick()
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if ep.Host != "a" {
		t.Errorf("tie: got %q, want a (first-drawn wins)", ep.Host)
	}
}

func TestLeastRequest_PicksFewestActive(t *testing.T) {
	lr := newLeastRequestWithRNG(eps(3), 2, seqRNG(0, 2))
	lr.active[0].Store(5) // endpoint a is loaded
	ep, _, err := lr.Pick()
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if ep.Host != "c" { // draws {0(active 5), 2(active 0)} → strict < → c
		t.Errorf("got %q, want c (fewest active)", ep.Host)
	}
}

func TestLeastRequest_WithReplacementNoClamp(t *testing.T) {
	// choiceCount 5 > n 2: draws sample WITH replacement, no clamp, no panic.
	lr := newLeastRequestWithRNG(eps(2), 5, seqRNG(0, 0, 1, 1, 0))
	if _, _, err := lr.Pick(); err != nil {
		t.Fatalf("Pick with choiceCount>n: %v", err)
	}
}

func TestLeastRequest_IncDecBalance(t *testing.T) {
	lr := newLeastRequestWithRNG(eps(3), 2, seqRNG(0, 0))
	_, release, _ := lr.Pick()
	if got := lr.active[0].Load(); got != 1 {
		t.Fatalf("after Pick: active[0] = %d, want 1", got)
	}
	release()
	if got := lr.active[0].Load(); got != 0 {
		t.Fatalf("after release: active[0] = %d, want 0", got)
	}
}

func TestLeastRequest_DoubleReleaseGuard(t *testing.T) {
	lr := newLeastRequestWithRNG(eps(3), 2, seqRNG(0, 0))
	_, release, _ := lr.Pick()
	release()
	release() // sync.Once: second call is a no-op
	if got := lr.active[0].Load(); got != 0 {
		t.Errorf("after double-release: active[0] = %d, want 0 (no underflow)", got)
	}
}

func TestLeastRequest_NoEndpoints(t *testing.T) {
	lr := newLeastRequestWithRNG(nil, 2, seqRNG(0))
	_, release, err := lr.Pick()
	if err != errNoEndpoints {
		t.Errorf("err = %v, want errNoEndpoints", err)
	}
	if release == nil {
		t.Error("release must be non-nil even on error (interface contract)")
	}
}

// TestLeastRequest_SkewAvoidsLoadedEndpoint is the unit-level proof that
// least_request actually shifts load away from busy endpoints: held picks (never
// released) raise an endpoint's active count, and the subsequent P2C sample
// avoids the heaviest endpoint. It is the in-process analog of the 0059
// differential band (the wire-level band is Task 6/7).
//
// Draw sequence: seqRNG(0,1) cycles, so EVERY Pick draws indices {0,1}
// (choiceCount 2). Holding the first 3 picks WITHOUT releasing accumulates load:
//
//	pick 1: {0:0, 1:0} tie → a (first-drawn, strict <);  active=[1,0,0]
//	pick 2: {0:1, 1:0}     → b (strict <);                active=[1,1,0]
//	pick 3: {0:1, 1:1} tie → a (first-drawn, strict <);   active=[2,1,0]
//
// Now index 0 (a) is the heaviest (active 2). The 4th (assertion) Pick draws
// {0:2, 1:1}: strict < selects index 1 (host "b") ≠ "a", so the test PASSES.
// Liveness (Step 3): if the comparison is inverted to `>`, the 4th Pick keeps
// index 0 (2 > 1 is false; best stays 0) → host "a" → the test FAILS. Thus the
// test is non-vacuous: it discriminates the strict-< skew from its inversion.
func TestLeastRequest_SkewAvoidsLoadedEndpoint(t *testing.T) {
	lr := newLeastRequestWithRNG(eps(3), 2, seqRNG(0, 1))
	held := make([]func(), 0, 3)
	for i := 0; i < 3; i++ {
		_, rel, _ := lr.Pick()
		held = append(held, rel) // hold (never release) to keep the load elevated
	}
	if got := lr.active[0].Load(); got != 2 {
		// Pin the exact precondition the trace establishes: a is strictly heaviest
		// (active 2 vs b's 1). A looser >=1 guard would also pass on a 1-1 tie,
		// silently changing what the final assertion proves.
		t.Fatalf("endpoint a should carry 2 held picks, active[0]=%d", got)
	}
	// a (index 0, active 2) is heaviest. The next sample {0:2, 1:1} → strict <
	// selects b. The load-bearing assertion: the heaviest endpoint is NOT re-picked.
	ep, rel, _ := lr.Pick()
	rel()
	if ep.Host == "a" {
		t.Errorf("loaded endpoint a was re-picked over lighter b; skew not working")
	}
	for _, r := range held {
		r()
	}
}

func TestNewLeastRequest_ProductionRNGSeeds(t *testing.T) {
	// Smoke: the crypto-seeded production constructor succeeds and Pick works.
	lr, err := newLeastRequest(eps(3), 2)
	if err != nil {
		t.Fatalf("newLeastRequest: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ { // concurrency smoke (mutex-guarded rng)
		wg.Add(1)
		go func() { defer wg.Done(); _, rel, _ := lr.Pick(); rel() }()
	}
	wg.Wait()
	for i := range lr.active {
		if got := lr.active[i].Load(); got != 0 {
			t.Errorf("active[%d] = %d, want 0 after balanced pick/release", i, got)
		}
	}
}
