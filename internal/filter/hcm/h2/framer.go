package h2

import (
	"context"
	"errors"
	"fmt"
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
// WriteGoAway, and ReadFrame directly. maxReadFrameSize sets the read-side
// limit: frames larger than this trigger a FRAME_SIZE_ERROR.
func newFramer(conn net.Conn, maxReadFrameSize uint32) *framer {
	fr := http2.NewFramer(conn, conn)
	fr.SetMaxReadFrameSize(maxReadFrameSize)
	return &framer{
		Framer: fr,
		conn:   conn,
	}
}

// translateFramerErr maps errors emitted by *http2.Framer to our h2:-prefixed
// *Error type so callers and fuzz assertions can rely on the h2: prefix
// discipline. Returns nil for nil input. Unrecognized errors pass through
// unchanged.
//
// Recognized classes:
//   - http2.ConnectionError → connection-scoped *Error
//   - http2.StreamError     → stream-scoped *Error (preserving StreamID)
//   - http2.ErrFrameTooLarge (RFC 9113 §4.2 connection error of type
//     FRAME_SIZE_ERROR; not wrapped as http2.ConnectionError by the
//     underlying framer)
//
// Consumed by both readFrameCtx and tryReadFrame, and (post phase 05.2) by
// the client-side codec read loop in client.go.
func translateFramerErr(err error) error {
	if err == nil {
		return nil
	}
	var connErr http2.ConnectionError
	if errors.As(err, &connErr) {
		return connError(ErrCode(connErr), fmt.Sprintf("framer: connection-error code=%d", connErr))
	}
	var streamErr http2.StreamError
	if errors.As(err, &streamErr) {
		return streamError(ErrCode(streamErr.Code), streamErr.StreamID, fmt.Sprintf("framer: stream-error code=%d", streamErr.Code))
	}
	if errors.Is(err, http2.ErrFrameTooLarge) {
		return connError(ErrFrameSizeError, "framer: frame too large")
	}
	return err
}

// readFrameCtx reads one frame, honoring ctx cancellation by setting a
// short (50ms) read deadline on the underlying conn and re-checking ctx.Err()
// after each timeout. http2.Framer.ReadFrame is otherwise blocking and not
// ctx-aware; this method bridges the two via polling.
//
// Always uses 50ms slices so that context cancellation (including cancellation
// of a context that also has a deadline) is observed within bounded latency.
// If the context deadline has already passed, ctx.Err() is non-nil on the
// first timeout check and the method returns immediately with ctx.Err().
func (f *framer) readFrameCtx(ctx context.Context) (http2.Frame, error) {
	for {
		// Check ctx before arming a new read deadline.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		_ = f.conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		frame, err := f.ReadFrame()
		if err == nil {
			_ = f.conn.SetReadDeadline(time.Time{})
			return frame, nil
		}
		// On any timeout (os.ErrDeadlineExceeded or net.Error.Timeout), re-check
		// ctx and re-arm. Genuine network errors fall through to the return below.
		var nerr net.Error
		isTimeout := errors.As(err, &nerr) && nerr.Timeout()
		if !isTimeout {
			isTimeout = errors.Is(err, os.ErrDeadlineExceeded)
		}
		if isTimeout {
			if ctxErr := ctx.Err(); ctxErr != nil {
				_ = f.conn.SetReadDeadline(time.Time{})
				return nil, ctxErr
			}
			// ctx still alive — re-loop and re-arm.
			continue
		}
		_ = f.conn.SetReadDeadline(time.Time{})
		return nil, translateFramerErr(err)
	}
}

// tryReadFrame attempts to read one frame without blocking.  It sets a 1ms
// read deadline; if no frame is available it returns (nil, nil).  This is
// used by the frame loop to detect whether more frames are immediately
// available after processing a batch, so that pending dispatch goroutines
// can be deferred until the batch is exhausted (see ServerConn.Run).
func (f *framer) tryReadFrame() (http2.Frame, error) {
	_ = f.conn.SetReadDeadline(time.Now().Add(time.Millisecond))
	frame, err := f.ReadFrame()
	_ = f.conn.SetReadDeadline(time.Time{})
	if err == nil {
		return frame, nil
	}
	// Any timeout means "no data yet" — return (nil, nil).
	var nerr net.Error
	isTimeout := errors.As(err, &nerr) && nerr.Timeout()
	if !isTimeout {
		isTimeout = errors.Is(err, os.ErrDeadlineExceeded)
	}
	if isTimeout {
		return nil, nil
	}
	return nil, translateFramerErr(err)
}
