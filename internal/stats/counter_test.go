package stats

import (
	"sync"
	"testing"
)

func TestCounter_Inc_Sequential(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("c.seq")
	if got := c.Load(); got != 0 {
		t.Fatalf("initial Load = %d, want 0", got)
	}
	c.Inc()
	c.Inc()
	c.Inc()
	if got := c.Load(); got != 3 {
		t.Errorf("Load after 3 Incs = %d, want 3", got)
	}
}

func TestCounter_Add_Sequential(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("c.add")
	c.Add(7)
	c.Add(13)
	if got := c.Load(); got != 20 {
		t.Errorf("Load after Add(7)+Add(13) = %d, want 20", got)
	}
}

func TestCounter_Inc_RaceClean(t *testing.T) {
	const N = 8
	const M = 10000
	r := NewRegistry()
	c := r.NewCounter("c.race")
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
	if got := c.Load(); got != N*M {
		t.Errorf("Load after %d×%d concurrent Incs = %d, want %d", N, M, got, N*M)
	}
}
