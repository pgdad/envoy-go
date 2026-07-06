package statssink

import (
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/pgdad/envoy-go/internal/stats"
)

// TestSnapshot covers the cumulative/no-labels Counter/Gauge -> MetricFamily
// mapping (ADR-0262): COUNTER/GAUGE families, the full dotted Name(), the
// absolute Load() value, a flush-time TimestampMs, ZERO LabelPairs, empty Help,
// registration order preserved (the Walk order), an empty registry -> empty
// slice, and a signed (negative) gauge.
func TestSnapshot(t *testing.T) {
	const nowMs = int64(1782734137801)

	t.Run("counter_and_gauge_families", func(t *testing.T) {
		reg := stats.NewRegistry()
		c := reg.NewCounter("cluster.backend.upstream_rq_total")
		c.Add(7)
		g := reg.NewGauge("cluster.backend.membership_healthy")
		g.Set(3)

		fams := snapshot(reg, nowMs)
		if len(fams) != 2 {
			t.Fatalf("snapshot len = %d, want 2", len(fams))
		}

		// Family[0] is the COUNTER (registered first -> Walk order).
		cf := fams[0]
		if got := cf.GetName(); got != "cluster.backend.upstream_rq_total" {
			t.Errorf("counter family name = %q, want cluster.backend.upstream_rq_total", got)
		}
		if got := cf.GetType(); got != dto.MetricType_COUNTER {
			t.Errorf("counter family type = %v, want COUNTER", got)
		}
		if got := cf.GetHelp(); got != "" {
			t.Errorf("counter family help = %q, want empty", got)
		}
		if got := len(cf.GetMetric()); got != 1 {
			t.Fatalf("counter family metric count = %d, want 1", got)
		}
		cm := cf.GetMetric()[0]
		if got := cm.GetCounter().GetValue(); got != 7.0 {
			t.Errorf("counter value = %v, want 7.0", got)
		}
		if got := cm.GetTimestampMs(); got != nowMs {
			t.Errorf("counter timestamp_ms = %d, want %d", got, nowMs)
		}
		if got := len(cm.GetLabel()); got != 0 {
			t.Errorf("counter label count = %d, want 0", got)
		}

		// Family[1] is the GAUGE.
		gf := fams[1]
		if got := gf.GetName(); got != "cluster.backend.membership_healthy" {
			t.Errorf("gauge family name = %q, want cluster.backend.membership_healthy", got)
		}
		if got := gf.GetType(); got != dto.MetricType_GAUGE {
			t.Errorf("gauge family type = %v, want GAUGE", got)
		}
		if got := len(gf.GetMetric()); got != 1 {
			t.Fatalf("gauge family metric count = %d, want 1", got)
		}
		gm := gf.GetMetric()[0]
		if got := gm.GetGauge().GetValue(); got != 3.0 {
			t.Errorf("gauge value = %v, want 3.0", got)
		}
		if got := gm.GetTimestampMs(); got != nowMs {
			t.Errorf("gauge timestamp_ms = %d, want %d", got, nowMs)
		}
		if got := len(gm.GetLabel()); got != 0 {
			t.Errorf("gauge label count = %d, want 0", got)
		}
	})

	t.Run("registration_order_preserved", func(t *testing.T) {
		reg := stats.NewRegistry()
		reg.NewCounter("a.first")
		reg.NewGauge("b.second")
		reg.NewCounter("c.third")

		fams := snapshot(reg, nowMs)
		want := []string{"a.first", "b.second", "c.third"}
		if len(fams) != len(want) {
			t.Fatalf("snapshot len = %d, want %d", len(fams), len(want))
		}
		for i, name := range want {
			if got := fams[i].GetName(); got != name {
				t.Errorf("family[%d] name = %q, want %q", i, got, name)
			}
		}
	})

	t.Run("empty_registry", func(t *testing.T) {
		reg := stats.NewRegistry()
		fams := snapshot(reg, nowMs)
		if len(fams) != 0 {
			t.Fatalf("snapshot of empty registry len = %d, want 0", len(fams))
		}
	})

	t.Run("negative_gauge", func(t *testing.T) {
		reg := stats.NewRegistry()
		g := reg.NewGauge("cluster.backend.some_signed_gauge")
		g.Set(-5)

		fams := snapshot(reg, nowMs)
		if len(fams) != 1 {
			t.Fatalf("snapshot len = %d, want 1", len(fams))
		}
		if got := fams[0].GetMetric()[0].GetGauge().GetValue(); got != -5.0 {
			t.Errorf("negative gauge value = %v, want -5.0", got)
		}
	})
}
