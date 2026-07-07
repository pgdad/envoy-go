package statssink

import (
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"
)

// deltaState holds the per-sink last-flushed COUNTER values keyed by full dotted
// family name, for report_counters_as_deltas=true (ADR-0263). A delta-mode sink
// owns one deltaState and applies it once per batch: each COUNTER family's value
// is rewritten to current-last (the per-flush delta) and last is latched to
// current; GAUGE (and any non-COUNTER) families pass through absolute,
// untransformed. Each owner touches the map from exactly ONE goroutine — the
// MetricsServiceSink writer goroutine (run, just before flush, so an enqueue-drop
// never latches), or the statsd/dog_statsd Submit which the single Flusher
// goroutine calls serially per flush (the sink contract forbids Submit after
// Close) — so no lock is needed.
type deltaState struct {
	last map[string]uint64
}

func newDeltaState() *deltaState { return &deltaState{last: make(map[string]uint64)} }

// apply returns a NEW batch in which every COUNTER family carries the per-flush
// delta (current - last[name]) instead of the absolute value, latching
// last[name]=current. It MUST NOT mutate the input batch: the Flusher fans the
// SAME absolute slice to every sink (flusher.go:47-49), so an in-place rewrite
// would corrupt a sibling absolute sink and be non-idempotent across the fan-out.
// COUNTER families are rebuilt (new MetricFamily/Metric/Counter); GAUGE (and any
// non-COUNTER) families are shared by pointer (not transformed — the proto field
// applies to counters only, D-MS-DELTA-GAUGE). An absent key reads 0, so the
// first flush / a counter's first appearance emits current-0 = the absolute value
// (AMEND-MSD-FIRST-FLUSH-ABSOLUTE — no special first-flush branch). Counters are
// monotone u64 (current >= last), so the delta is non-negative; no underflow
// guard. envoy-go's mapping emits exactly one Metric per family (no labels), so
// keying last by family name is exact. The absolute value originated as
// float64(Counter.Load()) (u64) so uint64(value) round-trips exactly for the
// realistic counter range (< 2^53).
func (d *deltaState) apply(abs []*dto.MetricFamily) []*dto.MetricFamily {
	out := make([]*dto.MetricFamily, 0, len(abs))
	for _, fam := range abs {
		if fam.GetType() != dto.MetricType_COUNTER {
			out = append(out, fam) // GAUGE/other: shared as-is, untransformed
			continue
		}
		name := fam.GetName()
		ms := make([]*dto.Metric, 0, len(fam.GetMetric()))
		for _, m := range fam.GetMetric() {
			cur := uint64(m.GetCounter().GetValue())
			delta := cur - d.last[name]
			d.last[name] = cur
			ms = append(ms, &dto.Metric{
				Counter:     &dto.Counter{Value: proto.Float64(float64(delta))},
				TimestampMs: m.TimestampMs, // read-only share of the *int64
			})
		}
		out = append(out, &dto.MetricFamily{Name: fam.Name, Type: fam.Type, Metric: ms})
	}
	return out
}
