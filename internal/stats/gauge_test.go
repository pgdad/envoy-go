package stats

import (
	"sync"
	"testing"
)

func TestGauge_IncDecSet_Sequential(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("g.seq")
	if got := g.Load(); got != 0 {
		t.Fatalf("initial Load = %d, want 0", got)
	}
	g.Inc()
	g.Inc()
	g.Inc()
	if got := g.Load(); got != 3 {
		t.Errorf("Load after 3 Incs = %d, want 3", got)
	}
	g.Dec()
	if got := g.Load(); got != 2 {
		t.Errorf("Load after Dec = %d, want 2", got)
	}
	g.Set(100)
	if got := g.Load(); got != 100 {
		t.Errorf("Load after Set(100) = %d, want 100", got)
	}
}

func TestGauge_NegativeValueAllowed(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("g.neg")
	g.Dec()
	g.Dec()
	g.Dec()
	if got := g.Load(); got != -3 {
		t.Errorf("Load after 3 Decs (no Incs) = %d, want -3", got)
	}
	g.Set(-42)
	if got := g.Load(); got != -42 {
		t.Errorf("Load after Set(-42) = %d, want -42", got)
	}
}

func TestGauge_Add_PositiveAndNegative(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("g.add")
	g.Add(10)
	g.Add(-3)
	if got := g.Load(); got != 7 {
		t.Errorf("Load after Add(10)+Add(-3) = %d, want 7", got)
	}
}

func TestGauge_RaceClean_ConcurrentIncDecSet(t *testing.T) {
	const N = 8
	const M = 1000
	r := NewRegistry()
	g := r.NewGauge("g.race")
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				g.Inc()
				g.Dec()
			}
		}()
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				g.Set(int64(j))
			}
		}()
	}
	wg.Wait()
	// final value is non-deterministic (the Set goroutines race against each
	// other) but the race detector must report no data race.
	_ = g.Load()
}

func TestGauge_Format_NegativeRendered(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("g.fmt")
	g.Set(-5)
	if got := g.Format(); got != "-5" {
		t.Errorf("Format() = %q, want -5", got)
	}
	g.Set(42)
	if got := g.Format(); got != "42" {
		t.Errorf("Format() = %q, want 42", got)
	}
}
