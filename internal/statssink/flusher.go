package statssink

import (
	"context"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

// Flusher snapshots the frozen process-global Registry every interval and
// Submits the batch to each sink. Started AFTER bs.Stats.Freeze() so Walk is
// lock-clean (ADR-0059). One full-registry snapshot per tick (NOT deltas).
type Flusher struct {
	reg      *stats.Registry
	interval time.Duration
	sinks    []Sink
	nowMs    func() int64 // injectable for tests; defaults to wall-clock ms
}

// NewFlusher builds a Flusher over the frozen registry, the stats_flush_interval
// tick period, and the configured sinks, defaulting nowMs to the wall clock.
func NewFlusher(reg *stats.Registry, interval time.Duration, sinks []Sink) *Flusher {
	return &Flusher{
		reg:      reg,
		interval: interval,
		sinks:    sinks,
		nowMs:    func() int64 { return time.Now().UnixMilli() },
	}
}

// Start runs the flush loop until ctx is canceled. No final flush on Done
// (D-MS-FINAL-FLUSH); the sinks' Close drains any in-flight stream.
func (f *Flusher) Start(ctx context.Context) {
	t := time.NewTicker(f.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.flushOnce()
		}
	}
}

func (f *Flusher) flushOnce() {
	batch := snapshot(f.reg, f.nowMs())
	for _, s := range f.sinks {
		s.Submit(batch)
	}
}
