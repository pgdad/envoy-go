package stats

import (
	"strconv"
	"sync/atomic"
)

// Counter is a monotonic non-negative integer metric. The hot-path Inc and
// Add are lock-free atomics; the Walk-callback's Format reads atomically.
type Counter struct {
	name string
	v    atomic.Uint64
}

// Name returns the registered hierarchical-dotted name.
func (c *Counter) Name() string { return c.name }

// Type returns MetricCounter.
func (c *Counter) Type() MetricType { return MetricCounter }

// Inc atomically increments by 1.
func (c *Counter) Inc() { c.v.Add(1) }

// Add atomically adds delta. Caller is responsible for delta >= 0
// (the type signature uses uint64 to encode the non-negativity at compile
// time; underflow is impossible).
func (c *Counter) Add(delta uint64) { c.v.Add(delta) }

// Load returns the current value.
func (c *Counter) Load() uint64 { return c.v.Load() }

// Format implements Metric: the Prometheus value text.
func (c *Counter) Format() string { return strconv.FormatUint(c.v.Load(), 10) }
