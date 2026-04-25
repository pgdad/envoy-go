package h2

import (
	"context"
	"errors"
	"net"
	"os"
	"time"

	"golang.org/x/net/http2"
)

// framer is a thin wrapper over *http2.Framer adding context-aware reads.
// Write methods are passthrough via embedding. Phase 05.1 does NOT use
// http2.Framer.WriteRawFrame (per SPEC §4.1).
type framer struct {
	*http2.Framer
	conn net.Conn
}

// newFramer constructs a framer over conn for both reading and writing. The
// returned value embeds *http2.Framer so callers can use WriteSettings,
// WriteHeaders, WriteData, WriteRSTStream, WriteWindowUpdate, WritePing,
// WriteGoAway, and ReadFrame directly.
func newFramer(conn net.Conn) *framer {
	return &framer{
		Framer: http2.NewFramer(conn, conn),
		conn:   conn,
	}
}

// readFrameCtx reads one frame, honouring ctx cancellation by setting a read
// deadline on the underlying conn. http2.Framer.ReadFrame is otherwise blocking
// and not ctx-aware; this method bridges the two. On ctx cancel mid-read,
// returns ctx.Err() (context.Canceled or context.DeadlineExceeded).
func (f *framer) readFrameCtx(ctx context.Context) (http2.Frame, error) {
	if dl, ok := ctx.Deadline(); ok {
		_ = f.conn.SetReadDeadline(dl)
	} else {
		// Short-poll: 50ms slices so ctx cancellation is observed within
		// bounded latency. The slice is small enough to be a noisy non-issue
		// in practice.
		_ = f.conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	}
	for {
		frame, err := f.ReadFrame()
		if err == nil {
			_ = f.conn.SetReadDeadline(time.Time{})
			return frame, nil
		}
		// Translate timeout-on-deadline into ctx.Err() when ctx is done.
		var nerr net.Error
		if errors.As(err, &nerr) && nerr.Timeout() {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			// No ctx deadline → re-arm and re-loop.
			if _, hasDL := ctx.Deadline(); !hasDL {
				_ = f.conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
				continue
			}
			return nil, ctxErr(ctx, err)
		}
		// Non-timeout error: pass through. Also pass through os.ErrDeadlineExceeded
		// where ctx has a deadline (the caller imposed it; ctx.Err returns
		// DeadlineExceeded so the wrap is faithful).
		_ = f.conn.SetReadDeadline(time.Time{})
		if errors.Is(err, os.ErrDeadlineExceeded) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
		}
		return nil, err
	}
}

func ctxErr(ctx context.Context, fallback error) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	return fallback
}
