package statssink

import (
	"context"
	"sync"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/esalaine/envoy-go/internal/stats"
)

// fakeSink records every Submit batch under a mutex (the -race gate: the ticker
// goroutine calls Submit concurrently with the test reading the count). It also
// satisfies the Sink interface's Close.
type fakeSink struct {
	mu      sync.Mutex
	batches [][]*dto.MetricFamily
}

func (f *fakeSink) Submit(batch []*dto.MetricFamily) {
	f.mu.Lock()
	f.batches = append(f.batches, batch)
	f.mu.Unlock()
}

func (f *fakeSink) Close() error { return nil }

func (f *fakeSink) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches)
}

func (f *fakeSink) last() []*dto.MetricFamily {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.batches) == 0 {
		return nil
	}
	return f.batches[len(f.batches)-1]
}

func TestFlusherFlushOnce(t *testing.T) {
	reg := stats.NewRegistry()
	reg.NewCounter("cluster.backend.upstream_rq_total").Add(5)
	reg.Freeze()

	fake := &fakeSink{}
	f := NewFlusher(reg, time.Hour, []Sink{fake})
	f.nowMs = func() int64 { return 1234 } // deterministic timestamp

	f.flushOnce()

	if got := fake.count(); got != 1 {
		t.Fatalf("fake received %d batches, want exactly 1", got)
	}
	batch := fake.last()
	if len(batch) != 1 {
		t.Fatalf("batch has %d families, want 1", len(batch))
	}
	mf := batch[0]
	if mf.GetType() != dto.MetricType_COUNTER {
		t.Fatalf("family type = %v, want COUNTER", mf.GetType())
	}
	if len(mf.Metric) != 1 {
		t.Fatalf("family has %d metrics, want 1", len(mf.Metric))
	}
	if got := mf.Metric[0].GetCounter().GetValue(); got != 5 {
		t.Fatalf("counter value = %v, want 5", got)
	}
	if got := mf.Metric[0].GetTimestampMs(); got != 1234 {
		t.Fatalf("timestamp = %d, want 1234 (injected nowMs)", got)
	}
}

func TestFlusherStartTicksThenStopsOnCancel(t *testing.T) {
	reg := stats.NewRegistry()
	reg.NewCounter("c.first").Add(1)
	reg.Freeze()

	fake := &fakeSink{}
	f := NewFlusher(reg, 10*time.Millisecond, []Sink{fake})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		f.Start(ctx)
		close(done)
	}()

	// Poll until the ticker has fired at least twice (robust to scheduling
	// jitter — NO sleep-driven exact-count assertion).
	deadline := time.Now().Add(2 * time.Second)
	for fake.count() < 2 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("ticker did not produce >=2 batches within timeout (got %d)", fake.count())
		}
		time.Sleep(time.Millisecond)
	}

	cancel()

	// Start must return promptly after cancel.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Start did not return promptly after cancel")
	}

	// After Start has returned the count must be frozen: read it twice with a
	// gap (several tick intervals) and confirm no growth.
	c1 := fake.count()
	time.Sleep(50 * time.Millisecond)
	c2 := fake.count()
	if c1 != c2 {
		t.Fatalf("batch count grew after cancel: %d -> %d", c1, c2)
	}
}

func TestFlusherMultiSinkFanOut(t *testing.T) {
	reg := stats.NewRegistry()
	reg.NewCounter("c.first").Add(2)
	reg.Freeze()

	a, b := &fakeSink{}, &fakeSink{}
	f := NewFlusher(reg, time.Hour, []Sink{a, b})

	f.flushOnce()
	f.flushOnce()

	if a.count() != 2 || b.count() != 2 {
		t.Fatalf("fan-out counts a=%d b=%d, want both 2", a.count(), b.count())
	}
}
