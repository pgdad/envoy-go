package h2

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestWindow_ReserveAndReplenish(t *testing.T) {
	w := newWindow(1000)
	got, err := w.reserve(100)
	if err != nil || got != 100 {
		t.Fatalf("reserve(100) = (%d, %v), want (100, nil)", got, err)
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
		_ = w.waitFor(ctx, 50)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	w.replenish(100)
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("waitFor did not return after replenish")
	}
}

func TestWindow_CtxCancelDuringWait(t *testing.T) {
	w := newWindow(0)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := w.waitFor(ctx, 50)
	if err == nil {
		t.Fatal("waitFor returned nil; want ctx.Err()")
	}
	if ctx.Err() == nil {
		t.Errorf("ctx.Err() = nil; want non-nil")
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
			if err := w.waitFor(ctx, 1); err != nil {
				t.Errorf("waitFor at i=%d: %v", i, err)
				close(done)
				return
			}
			_, _ = w.reserve(1)
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
