// Package stats is envoy-go's in-tree atomic-counter Registry. Per ADR-0059
// the package owns the canonical observation surface; no third-party
// Prometheus library is consumed at runtime. Per ADR-0060 histograms are
// deferred to a later sub-phase. The LBP-1 invariant ("list before play")
// makes the Walk-under-RLock-plus-atomic-Load read path lock-free against
// hot-path increments — see registry.go's Freeze documentation.
package stats

import (
	"fmt"
	"regexp"
	"sync"
	"sync/atomic"
)

// MetricType enumerates the supported metric primitives at phase 06.1.
// Histogram is reserved per ADR-0060 and not registered.
type MetricType int

const (
	MetricCounter MetricType = iota + 1
	MetricGauge
)

// Metric is the Walk-callback's view of a registered metric. Counter and Gauge
// both satisfy it; the Prometheus writer (prom.go) consumes Type to choose
// "counter" vs "gauge" in the # TYPE line.
type Metric interface {
	Name() string
	Type() MetricType
	// Format returns the metric's current value formatted as a Prometheus
	// metric-line value (the integer or non-negative integer text after the
	// labels block). Negative gauge values are permitted and rendered with a
	// minus sign per the Prometheus exposition spec.
	Format() string
}

// nameRE is the validation regex applied to every NewCounter / NewGauge name.
// Per BRAINSTORM §5.2 the form is ASCII-letter-or-underscore prefix followed
// by ASCII-alphanumerics, underscores, and dots. Dots are permitted because
// the internal hierarchical-dotted-name shape uses them as the segment separator.
// A trailing dot is rejected (dots are segment separators, not terminators —
// "trailing." is a malformed name with an empty trailing segment).
var nameRE = regexp.MustCompile(`^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`)

// Registry holds every metric registered at boot. The list of metrics is
// mutable only during boot; once Freeze is called, NewCounter / NewGauge
// panic. This is the LBP-1 invariant — see ADR-0059 Consequences (a) + (b).
type Registry struct {
	mu      sync.RWMutex
	metrics []Metric
	byName  map[string]Metric
	frozen  atomic.Bool
}

// NewRegistry returns a fresh registry with no metrics registered.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Metric)}
}

// NewCounter registers and returns a counter under the given hierarchical-
// dotted name. Panics if frozen, on invalid name (per nameRE), or on
// duplicate registration. The returned Counter is safe for concurrent Inc.
func (r *Registry) NewCounter(name string) *Counter {
	r.checkRegister(name)
	c := &Counter{name: name}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byName[name]; dup {
		panic(fmt.Sprintf("stats: duplicate metric registration: %q", name))
	}
	r.metrics = append(r.metrics, c)
	r.byName[name] = c
	return c
}

// NewGauge registers and returns a gauge under the given name. Same panic
// discipline as NewCounter.
func (r *Registry) NewGauge(name string) *Gauge {
	r.checkRegister(name)
	g := &Gauge{name: name}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byName[name]; dup {
		panic(fmt.Sprintf("stats: duplicate metric registration: %q", name))
	}
	r.metrics = append(r.metrics, g)
	r.byName[name] = g
	return g
}

// checkRegister panics if the registry is frozen or the name fails validation.
// Called from NewCounter and NewGauge before they take r.mu.Lock.
func (r *Registry) checkRegister(name string) {
	if r.frozen.Load() {
		panic(fmt.Sprintf("stats: registry frozen: cannot register %q post-boot", name))
	}
	if !nameRE.MatchString(name) {
		panic(fmt.Sprintf("stats: invalid metric name: %q (must match %s)", name, nameRE.String()))
	}
}

// Walk invokes fn for each registered metric in registration order. The
// ordering is NOT part of the contract; the Prometheus writer (prom.go)
// sorts post-walk. Walk holds r.mu RLock for the duration of the iteration;
// concurrent Walks are permitted; concurrent NewCounter/NewGauge are NOT
// (Freeze is the discipline that prevents them).
func (r *Registry) Walk(fn func(Metric)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.metrics {
		fn(m)
	}
}

// Freeze locks the metric list. Subsequent NewCounter / NewGauge calls panic.
// Idempotent; safe for concurrent calls.
func (r *Registry) Freeze() { r.frozen.Store(true) }
