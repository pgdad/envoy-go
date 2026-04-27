package stats

// Gauge is registered here so the Registry's NewGauge method compiles.
// The full Gauge body — Inc/Dec/Set/Format/Type/Name with sync/atomic.Int64
// backing — lands at Task 3. This Task 2 stub provides Name/Type/Format
// (the three methods required by the Metric interface so that the registry's
// `metrics []Metric` slice can hold *Gauge); Inc/Dec/Set/Load arrive at
// Task 3 alongside the atomic.Int64 backing field.
//
// Do not export new methods on this type without checking Task 3's plan.
type Gauge struct {
	name string
}

// Name returns the registered name. Task 3 will add Inc, Dec, Set, and Load
// alongside an `atomic.Int64` backing field.
func (g *Gauge) Name() string { return g.name }

// Type returns MetricGauge.
func (g *Gauge) Type() MetricType { return MetricGauge }

// Format returns the gauge's current value as a Prometheus-line value.
// Task 3 wires up the atomic.Int64 backing; this Task 2 stub returns "0".
func (g *Gauge) Format() string { return "0" }
