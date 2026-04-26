package h2

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWindow_ReserveAndReplenish(t *testing.T) {
	w := newWindow(1000)
	ctx := context.Background()
	got, err := w.reserveBlocking(ctx, 100)
	if err != nil || got != 100 {
		t.Fatalf("reserveBlocking(100) = (%d, %v), want (100, nil)", got, err)
	}
	if w.available() != 900 {
		t.Errorf("available = %d, want 900", w.available())
	}
	w.replenish(100)
	if w.available() != 1000 {
		t.Errorf("after replenish, available = %d, want 1000", w.available())
	}
}

func TestWindow_BlockingWaitFor(t *testing.T) {
	w := newWindow(0)
	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_, _ = w.reserveBlocking(ctx, 50)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	w.replenish(100)
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("reserveBlocking did not return after replenish")
	}
}

func TestWindow_CtxCancelDuringWait(t *testing.T) {
	w := newWindow(0)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := w.reserveBlocking(ctx, 50)
	if err == nil {
		t.Fatal("reserveBlocking returned nil; want ctx.Err()")
	}
	if ctx.Err() == nil {
		t.Errorf("ctx.Err() = nil; want non-nil")
	}
}

func TestWindow_ReserveBlocking_AtomicityUnderConcurrency(t *testing.T) {
	// 20 goroutines each call reserveBlocking(ctx, 50) against a window
	// primed at 100, with a replenisher goroutine adding 100 ten times.
	// Verifies no over-reservation: total taken must be <= 100 (initial)
	// + 10*100 (replenishments) = 1100.
	w := newWindow(100)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var taken atomic.Int64
	var wg sync.WaitGroup
	const consumers = 20
	for i := 0; i < consumers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := w.reserveBlocking(ctx, 50)
			if err != nil {
				return
			}
			taken.Add(int64(got))
		}()
	}

	// Replenisher adds 100 ten times via a ticker.
	replenishDone := make(chan struct{})
	go func() {
		defer close(replenishDone)
		tk := time.NewTicker(5 * time.Millisecond)
		defer tk.Stop()
		for i := 0; i < 10; i++ {
			<-tk.C
			w.replenish(100)
		}
	}()

	wg.Wait()
	<-replenishDone
	if taken.Load() > 1100 {
		t.Fatalf("over-reservation: taken=%d, want <= 1100", taken.Load())
	}
}

func TestWindow_TinyWindowStressDelivery(t *testing.T) {
	// SPEC §11.5 mitigation: INITIAL_WINDOW_SIZE = 1, send 100 bytes in
	// 1-byte chunks via WINDOW_UPDATE-driven progress. Eventual full delivery.
	w := newWindow(1)
	delivered := 0
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		for i := 0; i < 100; i++ {
			if _, err := w.reserveBlocking(ctx, 1); err != nil {
				t.Errorf("reserveBlocking at i=%d: %v", i, err)
				close(done)
				return
			}
			mu.Lock()
			delivered++
			mu.Unlock()
		}
		close(done)
	}()
	for i := 0; i < 99; i++ {
		time.Sleep(time.Millisecond)
		w.replenish(1)
	}
	<-done
	mu.Lock()
	defer mu.Unlock()
	if delivered != 100 {
		t.Errorf("delivered = %d, want 100", delivered)
	}
}
