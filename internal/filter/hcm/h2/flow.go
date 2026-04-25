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

// reserve atomically decrements up to n bytes, returning the actually-taken
// amount (which may be less than n if the window has fewer bytes available,
// or 0 if empty). Non-blocking. Callers that need >= n bytes call waitFor first.
func (w *window) reserve(n int32) (int32, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.n <= 0 {
		return 0, nil
	}
	taken := n
	if w.n < n {
		taken = w.n
	}
	w.n -= taken
	return taken, nil
}

// replenish increments the window and signals any blocking waitFor.
func (w *window) replenish(delta int32) {
	w.mu.Lock()
	w.n += delta
	w.mu.Unlock()
	select {
	case w.ch <- struct{}{}:
	default:
	}
}

// waitFor blocks until the window has at least n bytes available or ctx
// cancels. Returns nil on success, ctx.Err() on cancel.
func (w *window) waitFor(ctx context.Context, n int32) error {
	for {
		w.mu.Lock()
		if w.n >= n {
			w.mu.Unlock()
			return nil
		}
		w.mu.Unlock()
		select {
		case <-w.ch:
			// loop and re-check
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
