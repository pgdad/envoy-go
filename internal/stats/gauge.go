package stats

import (
	"strconv"
	"sync/atomic"
)

// Gauge is a signed-integer metric that may rise and fall. Negative values
// are permitted (a Dec not paired with an Inc is defensive — gauge reflects
// reality per BRAINSTORM §5.2). The hot-path Inc/Dec/Set/Add are lock-free
// atomics on int64.
type Gauge struct {
	name string
	v    atomic.Int64
}

// Name returns the registered hierarchical-dotted name.
func (g *Gauge) Name() string { return g.name }

// Type returns MetricGauge.
func (g *Gauge) Type() MetricType { return MetricGauge }

// Inc atomically adds 1.
func (g *Gauge) Inc() { g.v.Add(1) }

// Dec atomically subtracts 1.
func (g *Gauge) Dec() { g.v.Add(-1) }

// Add atomically adds delta. Negative delta is permitted.
func (g *Gauge) Add(delta int64) { g.v.Add(delta) }

// Set atomically replaces the current value.
func (g *Gauge) Set(v int64) { g.v.Store(v) }

// Load returns the current value.
func (g *Gauge) Load() int64 { return g.v.Load() }

// Format implements Metric: the Prometheus value text. Negative values are
// rendered with a minus sign per the Prometheus exposition spec.
func (g *Gauge) Format() string { return strconv.FormatInt(g.v.Load(), 10) }
