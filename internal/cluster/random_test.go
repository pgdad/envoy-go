package cluster

import (
	"sync"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
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

// TestRandom_HealthAware_PickSequenceUnchanged pins the health-gated pick
// sequence bit-identically across the panicGate/nextAvailable extraction: one
// rng draw per pick, forward wrap-scan from the drawn index to the next
// available host ("b" unhealthy: draw 1 walks to "c").
func TestRandom_HealthAware_PickSequenceUnchanged(t *testing.T) {
	e := eps(3)
	ch := newClusterHealth(e, 50)
	ch.states[e[1].Addr()].healthy.Store(false) // "b" out; 2/3 = 66% > 50% -> no panic
	r := newRandomWithRNG(e, seqRNG(0, 1, 2, 4))
	r.health = ch
	want := []string{"a", "c", "c", "c"} // 0→a; 1→b unhealthy→c; 2→c; 4%3=1→b unhealthy→c
	for i, w := range want {
		ep, _, err := r.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Host != w {
			t.Errorf("pick %d: got %q, want %q (health-gated sequence must be unchanged)", i, ep.Host, w)
		}
	}
}

// TestRandom_PanicMode_BlindDraw pins panic mode: below the threshold the pick
// blindly follows the draw (unhealthy hosts included) and lb_healthy_panic
// increments once per pick.
func TestRandom_PanicMode_BlindDraw(t *testing.T) {
	e := eps(3)
	ch := newClusterHealth(e, 50)
	reg := stats.NewRegistry()
	ch.panicCounter = reg.NewCounter("lb_healthy_panic")
	ch.states[e[0].Addr()].healthy.Store(false)
	ch.states[e[1].Addr()].healthy.Store(false) // 1/3 = 33% < 50% -> panic
	r := newRandomWithRNG(e, seqRNG(1))
	r.health = ch
	ep, _, err := r.Pick(0, false, SubsetMatch{}, false)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if ep.Host != "b" {
		t.Errorf("panic-mode pick = %q, want %q (blind draw, unhealthy included)", ep.Host, "b")
	}
	if got := ch.panicCounter.Load(); got != 1 {
		t.Errorf("lb_healthy_panic = %d, want 1", got)
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
