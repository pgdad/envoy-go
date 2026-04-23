package cluster

import (
	"sync"
	"sync/atomic"
	"testing"
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
		ep, err := rr.Pick()
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
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
	ep, err := rr.Pick()
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
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
				ep, err := rr.Pick()
				if err != nil {
					t.Errorf("pick: %v", err)
					return
				}
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
	_, err := rr.Pick()
	if err == nil {
		t.Fatal("expected error on zero endpoints")
	}
}
