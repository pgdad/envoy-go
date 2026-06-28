package tracing

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// countMetrics returns the number of metrics currently registered in reg.
func countMetrics(reg *stats.Registry) int {
	n := 0
	reg.Walk(func(stats.Metric) { n++ })
	return n
}

// metricNames returns the set of registered metric names in reg.
func metricNames(reg *stats.Registry) map[string]bool {
	names := make(map[string]bool)
	reg.Walk(func(m stats.Metric) { names[m.Name()] = true })
	return names
}

func TestHCMCounters_Registration(t *testing.T) {
	reg := stats.NewRegistry()
	before := countMetrics(reg)

	c, err := RegisterHCMCounters(reg, "ingress_http")
	if err != nil {
		t.Fatalf("RegisterHCMCounters returned error: %v", err)
	}
	if c == nil {
		t.Fatal("RegisterHCMCounters returned nil *HCMCounters")
	}

	after := countMetrics(reg)
	if delta := after - before; delta != 5 {
		t.Fatalf("registry counter delta = %d, want 5", delta)
	}

	names := metricNames(reg)
	want := []string{
		"http.ingress_http.tracing.client_enabled",
		"http.ingress_http.tracing.health_check",
		"http.ingress_http.tracing.not_traceable",
		"http.ingress_http.tracing.random_sampling",
		"http.ingress_http.tracing.service_forced",
	}
	for _, w := range want {
		if !names[w] {
			t.Errorf("missing registered metric %q", w)
		}
	}

	// 5 distinct non-nil counters.
	got := []*stats.Counter{c.clientEnabled, c.healthCheck, c.notTraceable, c.randomSampling, c.serviceForced}
	seen := make(map[*stats.Counter]bool)
	for i, g := range got {
		if g == nil {
			t.Errorf("counter %d is nil", i)
			continue
		}
		if seen[g] {
			t.Errorf("counter %d is not distinct", i)
		}
		seen[g] = true
	}
}

func TestHCMCounters_InvalidPrefix(t *testing.T) {
	reg := stats.NewRegistry()
	c, err := RegisterHCMCounters(reg, "bad name!")
	if err == nil {
		t.Fatal("RegisterHCMCounters(invalid) returned nil error, want error")
	}
	if c != nil {
		t.Fatalf("RegisterHCMCounters(invalid) returned non-nil *HCMCounters: %v", c)
	}
}

func TestHCMCounters_Record(t *testing.T) {
	reg := stats.NewRegistry()
	c, err := RegisterHCMCounters(reg, "ingress_http")
	if err != nil {
		t.Fatalf("RegisterHCMCounters returned error: %v", err)
	}

	c.Record(ClientEnabled)
	c.Record(RandomSampling)
	c.Record(NotTraceable)
	c.Record(HealthCheck)
	c.Record(ServiceForced)
	c.Record(NoClass) // increments none

	if v := c.clientEnabled.Load(); v != 1 {
		t.Errorf("client_enabled = %d, want 1", v)
	}
	if v := c.randomSampling.Load(); v != 1 {
		t.Errorf("random_sampling = %d, want 1", v)
	}
	if v := c.notTraceable.Load(); v != 1 {
		t.Errorf("not_traceable = %d, want 1", v)
	}
	if v := c.healthCheck.Load(); v != 1 {
		t.Errorf("health_check = %d, want 1", v)
	}
	if v := c.serviceForced.Load(); v != 1 {
		t.Errorf("service_forced = %d, want 1", v)
	}

	// NoClass incremented nothing beyond the single bumps above; total == 5.
	total := c.clientEnabled.Load() + c.healthCheck.Load() + c.notTraceable.Load() +
		c.randomSampling.Load() + c.serviceForced.Load()
	if total != 5 {
		t.Errorf("total increments = %d, want 5 (NoClass must increment none)", total)
	}
}
