package statssink

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"
)

func counterFam(name string, v float64) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name:   proto.String(name),
		Type:   dto.MetricType_COUNTER.Enum(),
		Metric: []*dto.Metric{{Counter: &dto.Counter{Value: proto.Float64(v)}, TimestampMs: proto.Int64(1)}},
	}
}
func gaugeFam(name string, v float64) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name:   proto.String(name),
		Type:   dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{{Gauge: &dto.Gauge{Value: proto.Float64(v)}, TimestampMs: proto.Int64(1)}},
	}
}
func counterVal(f *dto.MetricFamily) float64 { return f.GetMetric()[0].GetCounter().GetValue() }

func TestDeltaState_Apply(t *testing.T) {
	d := newDeltaState()

	// Flush 1: absolute (last empty -> current-0).
	out := d.apply([]*dto.MetricFamily{counterFam("c.rq", 7), gaugeFam("c.live", 1)})
	if got := counterVal(out[0]); got != 7 {
		t.Errorf("flush1 counter delta = %v, want 7 (absolute)", got)
	}
	if out[1].GetType() != dto.MetricType_GAUGE || out[1].GetMetric()[0].GetGauge().GetValue() != 1 {
		t.Errorf("flush1 gauge not passed through absolute: %v", out[1])
	}

	// Flush 2: increment since flush 1 -> delta=3; latched.
	out = d.apply([]*dto.MetricFamily{counterFam("c.rq", 10), gaugeFam("c.live", 1)})
	if got := counterVal(out[0]); got != 3 {
		t.Errorf("flush2 counter delta = %v, want 3", got)
	}

	// Flush 3: idle -> delta=0 (still emitted).
	out = d.apply([]*dto.MetricFamily{counterFam("c.rq", 10), gaugeFam("c.live", 1)})
	if got := counterVal(out[0]); got != 0 {
		t.Errorf("flush3 idle delta = %v, want 0", got)
	}
}

func TestDeltaState_Apply_DoesNotMutateInput(t *testing.T) {
	d := newDeltaState()
	in := []*dto.MetricFamily{counterFam("c.rq", 7)}
	_ = d.apply(in)
	_ = d.apply(in) // re-applying the SAME input must see 7 again, not a mutated value
	if got := counterVal(in[0]); got != 7 {
		t.Fatalf("input counter mutated: got %v, want 7", got)
	}
}

func TestDeltaState_Apply_GaugeSharedNotTransformed(t *testing.T) {
	d := newDeltaState()
	g := gaugeFam("c.live", 5)
	out := d.apply([]*dto.MetricFamily{g})
	if out[0] != g { // gauge family shared by pointer (untransformed)
		t.Errorf("gauge family should be shared by pointer, got a copy")
	}
}
