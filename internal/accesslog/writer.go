package accesslog

import (
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pgdad/envoy-go/internal/stats"
)

const defaultChannelCapacity = 4096
const dropLogIntervalNanos = int64(time.Second)

// Formatter renders an access-log record into a line of bytes. The default
// formatter (when AsyncFileSink.formatter is nil) is the HTTP Default over *Record
// (HCM callers byte-identical — the 29.3 D-P7 regression gate). Non-HTTP sinks
// (mongo_proxy, 29.3) supply their own formatter over their own record type.
type Formatter func(rec any) []byte

// AsyncFileSink writes Default-formatted records to a file in append mode via a
// bounded-channel writer goroutine. On full channel the new record is dropped
// and the dropped counter (per ADR-0069 — server.accesslog_dropped) is Inc'd;
// a rate-limited diagnostic is emitted at most once per second.
type AsyncFileSink struct {
	ch          chan any // was chan *Record — now carries any record type
	f           *os.File
	done        chan struct{}
	dropped     *stats.Counter
	lastDropLog atomic.Int64
	closeOnce   sync.Once
	path        string
	formatter   Formatter // nil → the Default(*Record) adapter (byte-identical HCM path)
}

// NewAsyncFileSink opens path with O_APPEND|O_CREAT|O_WRONLY mode 0644 and
// starts a writer goroutine. Per ADR-0066 the channel is bounded at 4096
// records; on full channel the new record is dropped (drop-newest discipline).
func NewAsyncFileSink(path string, dropped *stats.Counter) (*AsyncFileSink, error) {
	return newAsyncFileSinkWithCapacity(path, dropped, defaultChannelCapacity, nil)
}

// NewAsyncFileSinkWithFormatter is the 29.3 variant: a sink with a custom record
// formatter (mongo_proxy). The default NewAsyncFileSink keeps formatter nil → the
// Default(*Record) adapter, so every existing HCM caller is byte-identical.
func NewAsyncFileSinkWithFormatter(path string, dropped *stats.Counter, f Formatter) (*AsyncFileSink, error) {
	return newAsyncFileSinkWithCapacity(path, dropped, defaultChannelCapacity, f)
}

// newAsyncFileSinkWithCapacity is the test-friendly variant; production callers
// use NewAsyncFileSink (capacity 4096). The formatter (nil → the Default(*Record)
// adapter) is threaded in before the writer goroutine starts (no post-start race).
func newAsyncFileSinkWithCapacity(path string, dropped *stats.Counter, cap int, formatter Formatter) (*AsyncFileSink, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	s := &AsyncFileSink{
		ch:        make(chan any, cap),
		f:         f,
		done:      make(chan struct{}),
		dropped:   dropped,
		path:      path,
		formatter: formatter,
	}
	go s.run()
	return s, nil
}

// Submit non-blocking-sends r on the channel. On full-channel the record is
// dropped, the counter Inc'd, and at most one diagnostic emitted per second.
func (s *AsyncFileSink) Submit(r any) {
	select {
	case s.ch <- r:
	default:
		s.dropped.Inc()
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("accesslog: channel full, dropping record (path=%s)", s.path)
		}
	}
}

// Close closes the channel, waits for the writer goroutine to drain, then closes
// the file descriptor. Idempotent and threadsafe via sync.Once.
func (s *AsyncFileSink) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.ch)
		<-s.done
		err = s.f.Close()
	})
	return err
}

// run is the writer goroutine: drain channel-receives into per-record file
// writes. Per `man 2 write` on Linux, O_APPEND writes are atomic for sub-PAGE
// (default 4 KiB) writes.
func (s *AsyncFileSink) run() {
	defer close(s.done)
	for r := range s.ch {
		var line []byte
		if s.formatter != nil {
			line = s.formatter(r)
		} else if rec, ok := r.(*Record); ok {
			line = Default(rec) // the byte-identical HCM path
		} else {
			log.Printf("accesslog: default sink got non-*Record %T (path=%s); dropping", r, s.path)
			continue
		}
		if _, err := s.f.Write(line); err != nil {
			log.Printf("accesslog: file write error (path=%s): %v", s.path, err)
		}
	}
}
