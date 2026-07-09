// Package statssink — the plain-statsd TCP transport (ADR-0272).
package statssink

import (
	"bytes"
	"context"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"
)

// writeTimeout bounds ONE conn.Write so a wedged-but-not-dead peer cannot stall
// the writer goroutine indefinitely. Belt-and-braces with the hard conn.Close()
// that Close performs (D-TCP-CLOSE): the deadline handles a silently stalled
// peer, the Close handles shutdown.
const writeTimeout = 5 * time.Second

// DialFunc obtains one upstream connection for the sink. cmd/envoy-go/main.go
// closes over the cluster manager and re-looks-up the cluster per dial (the
// grpcclient.go:128-141 idiom), so internal/statssink never imports
// internal/cluster. A func rather than a one-method interface: the seam has one
// method and no shared state.
type DialFunc func(ctx context.Context) (net.Conn, error)

// TCPStatsdSink emits the statsd line protocol (identical to StatsdSink's) over
// one long-lived, newline-delimited TCP connection obtained from a named cluster
// (StatsdSink.statsd_specifier.tcp_cluster_name; ADR-0272).
//
// SHAPE (ADR-0262, the MetricsServiceSink precedent): Submit non-blocking-sends
// onto a bounded channel and NEVER blocks — the Flusher calls Submit serially
// across every sink from one goroutine (flusher.go:46-51), so a blocking TCP
// write here would starve every sibling sink. Phase 48's synchronous Submit was
// licensed by a property TCP lacks ("a UDP datagram never blocks on a peer").
//
// A single writer goroutine owns dial, `pending`, and the write. delta.apply runs
// in the WRITER (ADR-0263), so a channel-full enqueue drop never latches
// deltaState: the dropped increments ride the next enqueued flush.
//
// This is the statsd sinks' FIRST background mutator.
type TCPStatsdSink struct {
	ch     chan []*dto.MetricFamily
	dial   DialFunc
	prefix string
	delta  *deltaState // always non-nil — statsd |c is intrinsically a per-flush delta (no knob)

	done        chan struct{}
	closeOnce   sync.Once
	lastDropLog atomic.Int64

	// ctx/cancel bound an in-flight DialContext. They CANNOT interrupt a blocked
	// conn.Write (a raw net.Conn is not context-bound — the one place the
	// MetricsServiceSink precedent stops transferring); connMu + conn.Close() is
	// the unwedge (D-TCP-CLOSE).
	ctx    context.Context
	cancel context.CancelFunc

	// connMu guards the conn FIELD and the closed flag — never the I/O. The
	// writer copies conn out under the lock and does the blocking Write unlocked;
	// Close closes it under the lock, which unwedges that Write with an error.
	connMu sync.Mutex
	conn   net.Conn
	closed bool

	// pending is WRITER-OWNED (no lock): the unwritten bytes, accumulated across
	// flushes, always ending on a line boundary.
	pending []byte
}

// maxPendingBytes bounds the writer's accumulate buffer. The reference is
// effectively UNBOUNDED (SPEC-55 §11.5: >=45 snapshots / ~200 KB retained across
// a 30 s outage, no cap reached), so there is no value to mirror — this is an
// envoy-go choice (D-TCP-PENDING-CAP).
//
// 1 MiB. envoy-go's snapshot() walks the WHOLE frozen registry (~1200 stats x
// ~50 B ~= 60 KB per flush; the reference emits only USED stats, AMEND-TCP-
// USEDONLY), so 1 MiB retains ~16 flushes — the same order as
// MetricsServiceSink's defaultChannelCapacity of 8 in-flight batches — and 1 MiB
// is the tree's existing in-process buffer bound (ADR-0076's retry body buffer).
//
// On overflow the OLDEST WHOLE LINES are dropped and their counter increments are
// lost permanently: a DELIBERATE, documented departure from the reference's
// unbounded buffer. statsd is lossy by design, and an unbounded in-process buffer
// is a memory-growth hazard the tree refuses elsewhere.
const maxPendingBytes = 1 << 20

// dropOldestLines returns the longest suffix of p that (a) is at most capBytes
// long and (b) begins at a LINE boundary. Whole lines, never a partial line:
// `pending` carries no snapshot delimiters, so lines are the only unit its
// structure supports, and writing a partial line to the wire would corrupt a
// receiver's stream parse. Returns a subslice of p; the caller compacts.
//
// off > 0 always holds when len(p) > capBytes >= 0, so p[off-1] is in range.
func dropOldestLines(p []byte, capBytes int) []byte {
	if len(p) <= capBytes {
		return p
	}
	off := len(p) - capBytes // the first byte index we are allowed to keep
	if p[off-1] == '\n' {
		return p[off:] // off already begins a line; drop no extra line
	}
	idx := bytes.IndexByte(p[off:], '\n')
	if idx < 0 {
		return p[:0] // no boundary at or after off: the tail is one huge partial line
	}
	return p[off+idx+1:]
}

// newTCPStatsdSinkNoRun builds the sink WITHOUT starting the writer goroutine, so
// unit tests can drive flush() synchronously on the writer-owned `pending`.
func newTCPStatsdSinkNoRun(dial DialFunc, prefix string) *TCPStatsdSink {
	s := &TCPStatsdSink{
		ch:     make(chan []*dto.MetricFamily, defaultChannelCapacity),
		dial:   dial,
		prefix: prefix,
		delta:  newDeltaState(),
		done:   make(chan struct{}),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s
}

// NewTCPStatsdSink starts the writer goroutine. The dial is LAZY: no connection
// is opened until the first flush.
func NewTCPStatsdSink(dial DialFunc, prefix string) *TCPStatsdSink {
	s := newTCPStatsdSinkNoRun(dial, prefix)
	go s.run()
	return s
}

// Submit enqueues one full-registry snapshot. Non-blocking: on a full channel the
// batch is DROPPED with a rate-limited diagnostic (the sink.go:123-133 idiom).
func (s *TCPStatsdSink) Submit(batch []*dto.MetricFamily) {
	select {
	case s.ch <- batch:
	default:
		s.logDrop("channel full, dropping flush batch", nil)
	}
}

func (s *TCPStatsdSink) run() {
	defer close(s.done)
	defer s.markClosedAndCloseConn()
	for batch := range s.ch {
		s.flush(batch)
	}
}

// flush applies the delta, serializes into pending, and writes with
// line-aligned resume on a partial write (AMEND-TCP-RESUME).
func (s *TCPStatsdSink) flush(batch []*dto.MetricFamily) {
	// delta.apply runs HERE, in the writer (ADR-0263) — never in Submit — so a
	// channel-full enqueue drop cannot latch deltaState. It returns a NEW slice;
	// the Flusher's absolute batch, fanned to every sink, is never mutated.
	emitStatsdLines(s.delta.apply(batch), func(fam *dto.MetricFamily) (string, string) {
		return s.prefix + "." + fam.GetName(), ""
	}, func(line string) {
		// '\n'-TERMINATED, including the LAST line of the flush. Adopted verbatim
		// from the reference: every captured write ends 0x0A, no write contains
		// "\n\n" (SPEC-55 §11.1).
		s.pending = append(append(s.pending, line...), '\n')
	})
	if len(s.pending) > maxPendingBytes {
		kept := dropOldestLines(s.pending, maxPendingBytes)
		s.pending = append(s.pending[:0], kept...)
		s.logDrop("pending buffer over maxPendingBytes, dropped oldest lines", nil)
	}
	if len(s.pending) == 0 {
		return
	}
	if s.currentConn() == nil && !s.redial() {
		return // dial failed: pending RETAINED, retried next flush
	}
	conn := s.currentConn()
	if conn == nil {
		return
	}
	// Bound ONE write so a wedged-but-alive peer cannot stall the writer.
	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	n, err := conn.Write(s.pending)
	if err == nil {
		// Go's blocking net.Conn.Write loops internally: err == nil ⇒ n == len.
		s.pending = s.pending[:0]
		return
	}
	s.dropConn()
	// LINE-ALIGNED RESUME (AMEND-TCP-RESUME). Bytes [0,n) may have reached the
	// peer, ending mid-line. Retain from the start of the STRADDLING line:
	//   - complete lines within [0,n) landed exactly once and are NOT re-sent
	//     ⇒ no duplication;
	//   - the straddling line was DISCARDED by the dead conn's stream parser at
	//     EOF and is re-sent WHOLE on the next conn ⇒ no loss.
	// LastIndexByte(...)+1 == n when n lands on a boundary (retain nothing) and
	// == 0 when n == 0 (retain everything).
	keep := bytes.LastIndexByte(s.pending[:n], '\n') + 1
	// Compact to the front: `copy` (inside append) handles the overlapping
	// forward move correctly (destination index < source index), and the
	// buffer's backing array is REUSED instead of growing without bound. Do
	// NOT "optimize" this to `s.pending = s.pending[keep:]` — that reslice
	// keeps the whole head-of-array alive and lets the backing array grow
	// unboundedly across reconnects.
	s.pending = append(s.pending[:0], s.pending[keep:]...)
	s.logDrop("write", err)
}

// currentConn returns the live conn, or nil. Never call I/O while holding connMu.
func (s *TCPStatsdSink) currentConn() net.Conn {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	return s.conn
}

// dropConn closes and clears the live conn after a write error.
func (s *TCPStatsdSink) dropConn() {
	s.connMu.Lock()
	c := s.conn
	s.conn = nil
	s.connMu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

// redial opens a fresh connection. Returns false on failure (pending is RETAINED
// and retried on the next flush) or if Close has already latched.
func (s *TCPStatsdSink) redial() bool {
	c, err := s.dial(s.ctx)
	if err != nil {
		s.logDrop("dial", err)
		return false
	}
	s.connMu.Lock()
	if s.closed {
		s.connMu.Unlock()
		_ = c.Close() // Close raced us; do not leak the fresh conn
		return false
	}
	s.conn = c
	s.connMu.Unlock()
	return true
}

// markClosedAndCloseConn latches closed and hard-closes the live conn, which
// unwedges a blocked Write. Idempotent; safe from any goroutine.
func (s *TCPStatsdSink) markClosedAndCloseConn() {
	s.connMu.Lock()
	s.closed = true
	c := s.conn
	s.conn = nil
	s.connMu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

// Close stops the writer goroutine and releases the connection. Idempotent and
// threadsafe via sync.Once.
//
// Mirrors MetricsServiceSink.Close (sink.go:140-153) — close(ch), wait for the
// drain up to closeDrainGrace, force, wait again — with ONE substantive
// difference. cancel() aborts a context-bound gRPC stream; it CANNOT interrupt a
// blocked net.Conn.Write. This is precisely where that precedent stops
// transferring (D-TCP-CLOSE). So the force step is TWO actions:
//
//   - cancel()                 aborts an in-flight DialContext;
//   - markClosedAndCloseConn() hard-closes the live conn, which unwedges a
//     blocked Write with an error, AND latches `closed` so the writer's next
//     redial gives up instead of leaking a fresh conn.
//
// The writer never holds connMu across the write, so this can never deadlock.
// Proven under -race by TestTCPStatsdSink_CloseUnwedgesBlockedWrite.
func (s *TCPStatsdSink) Close() error {
	s.closeOnce.Do(func() {
		close(s.ch)
		select {
		case <-s.done:
		case <-time.After(closeDrainGrace):
			s.cancel()
			s.markClosedAndCloseConn()
			<-s.done
		}
		s.cancel()                 // release the context (idempotent; satisfies vet lostcancel)
		s.markClosedAndCloseConn() // idempotent; the writer's own defer usually got here first
	})
	return nil
}

// logDrop rate-limits a diagnostic to at most once per second (the sink.go
// lastDropLog idiom). Drops are LOGGED not counted — zero statsd-sink self-stats
// (SPEC-55 §7 / D-TCP-STATS).
func (s *TCPStatsdSink) logDrop(what string, err error) {
	now := time.Now().UnixNano()
	last := s.lastDropLog.Load()
	if now-last < dropLogIntervalNanos || !s.lastDropLog.CompareAndSwap(last, now) {
		return
	}
	if err != nil {
		log.Printf("statssink: statsd tcp: %s: %v", what, err)
		return
	}
	log.Printf("statssink: statsd tcp: %s", what)
}
