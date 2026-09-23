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
//   - timeoutMs > 0, ctx done: a Peek blocked on a deadline-capable peeker is
//     cut (the deadline is cleared before return); returns wrapped ctx.Err().
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
		// ONE clock: the socket read is interrupted only AFTER ctx is done, so
		// an interrupted Peek always observes ctx.Err() != nil below.
		if ds, ok := peeker.(interface{ SetReadDeadline(time.Time) error }); ok {
			fired := make(chan struct{})
			stop := context.AfterFunc(ctx, func() {
				_ = ds.SetReadDeadline(time.Unix(1, 0))
				close(fired)
			})
			defer func() {
				if !stop() {
					<-fired
				}
				_ = ds.SetReadDeadline(time.Time{})
			}()
		}
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
