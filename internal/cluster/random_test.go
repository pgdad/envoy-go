package cluster

import (
	"sync"
	"testing"
)

func TestRandom_FollowsDrawExactly(t *testing.T) {
	// pick i == endpoints[draw % n] for each draw — the load-bearing correctness
	// test (the pin-to-endpoints[0] deliberate break fails this). n=3, draws map:
	//   0%3=0→a, 4%3=1→b, 8%3=2→c, 2%3=2→c, 3%3=0→a.
	r := newRandomWithRNG(eps(3), seqRNG(0, 4, 8, 2, 3))
	want := []string{"a", "b", "c", "c", "a"}
	for i, w := range want {
		ep, release, err := r.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Host != w {
			t.Errorf("pick %d: got %q, want %q (rng()%%n)", i, ep.Host, w)
		}
		if release == nil {
			t.Fatalf("pick %d: release must be non-nil (interface contract)", i)
		}
		release()
		release() // noopRelease: safe to call twice
	}
}

func TestRandom_ReleaseIsSharedNoop(t *testing.T) {
	// random holds NO per-pick state → it returns the shared noopRelease.
	r := newRandomWithRNG(eps(2), seqRNG(0))
	_, release, err := r.Pick(0, false, SubsetMatch{}, false)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if release == nil {
		t.Fatal("release must be non-nil")
	}
	release() // must not panic
}

func TestRandom_NoEndpoints(t *testing.T) {
	r := newRandomWithRNG(nil, seqRNG(0))
	_, release, err := r.Pick(0, false, SubsetMatch{}, false)
	if err != errNoEndpoints {
		t.Errorf("err = %v, want errNoEndpoints", err)
	}
	if release == nil {
		t.Error("release must be non-nil even on error (interface contract)")
	}
}

// TestRandom_DoesNotAvoidHeldEndpoint is the anti-skew integration test
// (D-S35-4): the contrapositive of least_request's
// TestLeastRequest_SkewAvoidsLoadedEndpoint. randomLB holds NO active counters,
// so repeatedly picking AND holding an endpoint does NOT make a later pick avoid
// it — the draw alone decides. With a deterministic RNG drawing index 0 every
// time (seqRNG(0) → 0%3=0 always), all three picks land on endpoint "a" even
// though the first two are still held. (least_request would have routed picks
// 2/3 AWAY from the loaded "a".)
func TestRandom_DoesNotAvoidHeldEndpoint(t *testing.T) {
	r := newRandomWithRNG(eps(3), seqRNG(0))
	held := make([]func(), 0, 3)
	for i := 0; i < 3; i++ {
		ep, rel, err := r.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Host != "a" {
			t.Errorf("pick %d: got %q, want a (RANDOM follows the draw, ignores held load)", i, ep.Host)
		}
		held = append(held, rel) // hold (noopRelease — nothing to elevate; there is no counter)
	}
	for _, rel := range held {
		rel()
	}
}

func TestNewRandom_ProductionRNGSeeds(t *testing.T) {
	// Smoke: the crypto-seeded production constructor (REUSED newPCGRNG) succeeds
	// and Pick is concurrency-safe (the mutex-guarded rng).
	r, err := newRandom(eps(3))
	if err != nil {
		t.Fatalf("newRandom: %v", err)
	}
	var wg sync.WaitGroup
	seen := make([]int, 3)
	var mu sync.Mutex
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ep, rel, perr := r.Pick(0, false, SubsetMatch{}, false)
			if perr != nil {
				return
			}
			rel()
			mu.Lock()
			seen[int(ep.Port)-1000]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	for i, n := range seen {
		if n == 0 {
			t.Errorf("endpoint %d never picked over 60 draws (rng stuck?)", i)
		}
	}
}
