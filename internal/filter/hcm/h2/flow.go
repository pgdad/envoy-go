package h2

import (
	"context"
	"sync"
)

// window models one HTTP/2 flow-control window — either connection-level
// (one per ServerConn) or per-stream (one per serverStream, send and recv
// sides separately). Implementation: a mutex-guarded int32 counter plus a
// signal channel that replenish notifies for blocking reservers.
type window struct {
	mu sync.Mutex
	n  int32
	ch chan struct{}
}

func newWindow(initial int32) *window {
	return &window{n: initial, ch: make(chan struct{}, 1)}
}

// available reports the current window size. Used in tests.
func (w *window) available() int32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.n
}

// replenish increments the window and signals any blocking reserveBlocking.
func (w *window) replenish(delta int32) {
	w.mu.Lock()
	w.n += delta
	w.mu.Unlock()
	select {
	case w.ch <- struct{}{}:
	default:
	}
}

// reserveBlocking atomically waits for window > 0 and decrements up to max
// bytes, returning the actually-taken amount (>= 1 on success). Blocks on the
// internal signal channel until replenish notifies, returning ctx.Err() if
// ctx cancels first.
//
// This collapses the previous waitFor + reserve pair so the wait-then-take
// is a single critical section — no other goroutine can drain the window
// between the wait and the take. (See M-3 of the phase-05.2 plan.)
func (w *window) reserveBlocking(ctx context.Context, max int32) (int32, error) {
	w.mu.Lock()
	for w.n <= 0 {
		w.mu.Unlock()
		select {
		case <-w.ch:
			w.mu.Lock()
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	taken := max
	if taken > w.n {
		taken = w.n
	}
	w.n -= taken
	leftover := w.n > 0
	w.mu.Unlock()
	// Chain the wakeup: the signal channel has capacity 1, so one replenish
	// wakes exactly one waiter. If window capacity remains after this take,
	// re-signal so a second goroutine blocked on the same window (e.g. two
	// streams stalled on the conn-level send window) is woken too instead of
	// stalling until an unrelated future replenish arrives.
	if leftover {
		select {
		case w.ch <- struct{}{}:
		default:
		}
	}
	return taken, nil
}
