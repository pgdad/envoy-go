package listenerfilter

import (
	"context"
	"fmt"
	"time"
)

// Pipeline drives the per-connection listener-filter sequential dispatch.
// Allocated by the listener manager's accept-loop on each accepted
// connection. Owns nothing per-instance — Run is a pure function over
// (filters, peeker, inputs, timeoutMs); the struct exists for future
// per-pipeline state (e.g., metrics counters) without breaking the API.
type Pipeline struct{}

// Run iterates filters sequentially, calling each filter's Inspect with the
// shared (ctx, peeker, inputs) trio. Behavior:
//   - 0 filters: returns nil immediately.
//   - timeoutMs == 0: no per-pipeline deadline established (the caller's
//     ctx is passed through as-is).
//   - timeoutMs > 0: a single context.WithTimeout(ctx, timeoutMs *
//     time.Millisecond) wraps the loop; the per-filter Inspect calls share
//     the deadline (ADR-0082 + Decision N — per-pipeline NOT per-filter).
//   - On Continue: advances to the next filter (or finishes if last).
//   - On StopIteration: halts the loop; remaining filters are skipped.
//   - On non-nil error: aborts; the error is wrapped with the filter index.
//   - On context-deadline-exceeded after a filter's Inspect returns: the
//     pipeline returns a wrapped timeout error.
//   - OnDestroy is called on every filter (in declaration order) after the
//     loop ends, regardless of how the loop exited (Continue/StopIteration/
//     error/timeout).
func (p *Pipeline) Run(ctx context.Context, filters []ListenerFilter, peeker Peeker, inputs *ChainMatchInputs, timeoutMs uint32) (retErr error) {
	defer func() {
		for _, f := range filters {
			f.OnDestroy()
		}
	}()
	if len(filters) == 0 {
		return nil
	}
	if timeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}
	for i, f := range filters {
		status, err := f.Inspect(ctx, peeker, inputs)
		if err != nil {
			return fmt.Errorf("listener-filter[%d]: %w", i, err)
		}
		if ctx.Err() != nil {
			return fmt.Errorf("listener-filter[%d]: pipeline timeout: %w", i, ctx.Err())
		}
		if status == StopIteration {
			return nil
		}
	}
	return nil
}
