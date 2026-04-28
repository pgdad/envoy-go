package stats

import (
	"strings"
	"sync"
	"testing"
)

func TestRegistry_NewCounter_HappyPath(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("listener.0_0_0_0_10000.downstream_cx_total")
	if c == nil {
		t.Fatal("NewCounter returned nil")
	}
	if c.Name() != "listener.0_0_0_0_10000.downstream_cx_total" {
		t.Errorf("Counter.Name() = %q, want listener.0_0_0_0_10000.downstream_cx_total", c.Name())
	}
	if c.Load() != 0 {
		t.Errorf("freshly-allocated Counter.Load() = %d, want 0", c.Load())
	}
}

func TestRegistry_NewCounter_DuplicateNamePanics(t *testing.T) {
	r := NewRegistry()
	r.NewCounter("foo")
	defer func() {
		got := recover()
		if got == nil {
			t.Fatal("expected panic on duplicate-name registration; got nil")
		}
		s, _ := got.(string)
		if !strings.Contains(s, "duplicate") {
			t.Errorf("panic message = %q, want substring 'duplicate'", s)
		}
	}()
	r.NewCounter("foo")
}

func TestRegistry_NewCounter_InvalidNamePanics(t *testing.T) {
	cases := []string{"", "1leading-digit", "with space", "with-dash", "trailing.", "with$char"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			r := NewRegistry()
			defer func() {
				if recover() == nil {
					t.Errorf("expected panic on invalid name %q", name)
				}
			}()
			r.NewCounter(name)
		})
	}
}

// TestIsValidName covers the read-only IsValidName helper introduced for
// callers (HCM, future cluster-name validation) that derive metric names
// from user-controlled inputs and need to validate at the input boundary
// rather than trip NewCounter's panic-on-invalid-name discipline. Same
// regex as Registry.checkName; happy + sad cases mirror the existing
// TestRegistry_NewCounter_InvalidNamePanics matrix plus full assembled-
// name forms produced by the HCM/cluster/listener naming patterns.
func TestIsValidName(t *testing.T) {
	valid := []string{
		"a", "_", "ab", "a_b", "a.b",
		"http.ingress_http.downstream_rq_total",
		"cluster.c_backend.upstream_rq_2xx",
		"listener.0_0_0_0_10000.downstream_cx_total",
		"server.live",
	}
	for _, n := range valid {
		if !IsValidName(n) {
			t.Errorf("IsValidName(%q) = false, want true", n)
		}
	}
	invalid := []string{
		"",
		"1leading-digit",
		"with space",
		"with-dash",
		"trailing.",
		"with$char",
		"http.0000000000 0.downstream_rq_total", // verbatim assembled name from gate-(d) seed
	}
	for _, n := range invalid {
		if IsValidName(n) {
			t.Errorf("IsValidName(%q) = true, want false", n)
		}
	}
}

func TestRegistry_Walk_RegistrationOrderInvariantNotPromised(t *testing.T) {
	r := NewRegistry()
	a := r.NewCounter("a")
	b := r.NewCounter("b")
	a.Inc()
	b.Inc()
	b.Inc()
	var seen []string
	r.Walk(func(m Metric) {
		seen = append(seen, m.Name())
	})
	if len(seen) != 2 {
		t.Fatalf("Walk visited %d metrics, want 2", len(seen))
	}
}

func TestRegistry_Freeze_PostFreezeRegisterPanics(t *testing.T) {
	r := NewRegistry()
	r.NewCounter("pre.freeze")
	r.Freeze()
	defer func() {
		got := recover()
		if got == nil {
			t.Fatal("expected panic on post-freeze NewCounter; got nil")
		}
		s, _ := got.(string)
		if !strings.Contains(s, "registry frozen: cannot register") {
			t.Errorf("panic message = %q, want substring 'registry frozen: cannot register'", s)
		}
	}()
	r.NewCounter("post.freeze")
}

func TestRegistry_Freeze_PostFreezeNewGaugePanics(t *testing.T) {
	r := NewRegistry()
	r.Freeze()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on post-freeze NewGauge; got nil")
		}
	}()
	r.NewGauge("post.freeze.gauge")
}

func TestRegistry_Freeze_Idempotent(t *testing.T) {
	r := NewRegistry()
	r.Freeze()
	r.Freeze() // must not panic, must not race
}

func TestRegistry_Walk_ConcurrentWithIncs_RaceClean(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("conc.counter")
	r.Freeze()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				c.Inc()
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				r.Walk(func(m Metric) { _ = m.Name() })
			}
		}()
	}
	wg.Wait()
	if got := c.Load(); got != 8000 {
		t.Errorf("Counter.Load() after concurrent Incs = %d, want 8000", got)
	}
}
