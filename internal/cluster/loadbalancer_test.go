package cluster

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestRoundRobin_DistributionExact(t *testing.T) {
	rr := &roundRobin{
		endpoints: []Endpoint{
			{Host: "10.0.0.1", Port: 1001},
			{Host: "10.0.0.2", Port: 1002},
			{Host: "10.0.0.3", Port: 1003},
		},
	}
	counts := map[string]int{}
	const N = 30
	for i := 0; i < N; i++ {
		ep, release, err := rr.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		_ = release
		counts[ep.Addr()]++
	}
	for _, ep := range rr.endpoints {
		if got := counts[ep.Addr()]; got != N/3 {
			t.Errorf("endpoint %s: got %d picks, want %d", ep.Addr(), got, N/3)
		}
	}
}

func TestRoundRobin_FirstPickIsEndpoint0(t *testing.T) {
	rr := &roundRobin{
		endpoints: []Endpoint{
			{Host: "10.0.0.1", Port: 1001},
			{Host: "10.0.0.2", Port: 1002},
		},
	}
	ep, release, err := rr.Pick(0, false, SubsetMatch{}, false)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	_ = release
	if ep.Host != "10.0.0.1" {
		t.Errorf("first pick: got %s, want 10.0.0.1 (sequence-starts-at-0 invariant)", ep.Host)
	}
}

func TestRoundRobin_ConcurrentDistributionExact(t *testing.T) {
	rr := &roundRobin{
		endpoints: []Endpoint{
			{Host: "10.0.0.1", Port: 1001},
			{Host: "10.0.0.2", Port: 1002},
			{Host: "10.0.0.3", Port: 1003},
		},
	}
	const goroutines = 100
	const perGoroutine = 30
	const total = goroutines * perGoroutine // 3000, divisible by 3
	var counts [3]atomic.Uint64
	addrToIdx := map[string]int{
		rr.endpoints[0].Addr(): 0,
		rr.endpoints[1].Addr(): 1,
		rr.endpoints[2].Addr(): 2,
	}
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				ep, release, err := rr.Pick(0, false, SubsetMatch{}, false)
				if err != nil {
					t.Errorf("pick: %v", err)
					return
				}
				_ = release
				counts[addrToIdx[ep.Addr()]].Add(1)
			}
		}()
	}
	wg.Wait()
	for i := 0; i < 3; i++ {
		if got := counts[i].Load(); got != total/3 {
			t.Errorf("endpoint[%d]: got %d picks, want %d (atomic.Add(1) gives unique i; mod 3 balances exactly when 3 | total)", i, got, total/3)
		}
	}
}

func TestRoundRobin_ZeroEndpoints(t *testing.T) {
	rr := &roundRobin{endpoints: nil}
	_, release, err := rr.Pick(0, false, SubsetMatch{}, false)
	if err == nil {
		t.Fatal("expected error on zero endpoints")
	}
	if release == nil {
		t.Fatal("release must be non-nil even on the error path (interface contract)")
	}
	release() // must not panic
}

func TestRoundRobin_ReleaseIsNonNilNoop(t *testing.T) {
	rr := &roundRobin{endpoints: []Endpoint{{Host: "10.0.0.1", Port: 1}}}
	ep, release, err := rr.Pick(0, false, SubsetMatch{}, false)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if ep.Host != "10.0.0.1" {
		t.Errorf("ep.Host = %q, want 10.0.0.1", ep.Host)
	}
	if release == nil {
		t.Fatal("release must be non-nil (interface contract)")
	}
	release() // must not panic and must be safe to call twice
	release()
}

// TestRoundRobin_HealthAware proves the ADR-0243 health-aware pick: while the
// healthy fraction is at/above the panic threshold the unhealthy host is never
// returned; once it drops below the threshold panic mode returns ALL hosts
// (incl. unhealthy) and increments lb_healthy_panic.
func TestRoundRobin_HealthAware(t *testing.T) {
	e := eps(3) // addrs "a:1000", "b:1001", "c:1002"
	ch := newClusterHealth(e, 0.5)
	rr := &roundRobin{endpoints: e, health: ch}

	// Mark one unhealthy: 2/3 = 66% > 50% -> NOT in panic; that host is skipped.
	unhealthyAddr := e[2].Addr()
	ch.states[unhealthyAddr].healthy.Store(false)
	for i := 0; i < 30; i++ {
		ep, _, err := rr.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Addr() == unhealthyAddr {
			t.Fatalf("pick %d returned the unhealthy host %q (should be skipped while healthy fraction > threshold)", i, unhealthyAddr)
		}
	}

	// Mark a second unhealthy: 1/3 = 33% < 50% -> panic mode returns all hosts.
	reg := stats.NewRegistry()
	ch.panicCounter = reg.NewCounter("lb_healthy_panic")
	ch.states[e[1].Addr()].healthy.Store(false)

	sawUnhealthy := false
	for i := 0; i < 30; i++ {
		ep, _, err := rr.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatalf("panic-mode pick %d: %v", i, err)
		}
		if ep.Addr() == e[1].Addr() || ep.Addr() == e[2].Addr() {
			sawUnhealthy = true
		}
	}
	if !sawUnhealthy {
		t.Fatal("panic mode never returned an unhealthy host (expected all hosts eligible)")
	}
	if got := ch.panicCounter.Load(); got <= 0 {
		t.Fatalf("lb_healthy_panic = %d, want > 0", got)
	}
}

func TestRoundRobin_PickSequenceUnchanged(t *testing.T) {
	// Behavior-neutrality of the reshape: first pick is endpoints[0], then mod-index.
	rr := &roundRobin{endpoints: []Endpoint{{Host: "a"}, {Host: "b"}, {Host: "c"}}}
	want := []string{"a", "b", "c", "a", "b", "c"}
	for i, w := range want {
		ep, _, err := rr.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Host != w {
			t.Errorf("pick %d: got %q, want %q", i, ep.Host, w)
		}
	}
}
