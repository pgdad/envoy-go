package accesslog

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	dataaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"

	"github.com/esalaine/envoy-go/internal/stats"
)

// alsClient is the minimal sink-facing seam over *grpcclient.ALSClient
// (test-fakeable). *grpcclient.ALSClient satisfies it structurally, so the sink
// need not import grpcclient (no import cycle; main wiring at Task 8 passes the
// concrete client).
type alsClient interface {
	StreamAccessLogs(ctx context.Context) (accesslogv3.AccessLogService_StreamAccessLogsClient, error)
	Close() error
}

// closeDrainGrace bounds how long Close waits for the in-flight client-streaming
// RPC to drain before canceling the stream context. Because this is a NETWORK
// sink, a wedged Send/CloseAndRecv (peer stall) would otherwise hang process
// shutdown forever; the grace + cancel guarantees bounded shutdown.
const closeDrainGrace = 5 * time.Second

// GrpcAccessLogSink is the project's second accesslog.Sink: it streams
// structured HTTPAccessLogEntry protos to an Envoy AccessLogService over a
// lazily-established, identifier-once, reused client-streaming RPC. It mirrors
// AsyncFileSink's bounded-channel + writer-goroutine + idempotent-Close shape
// (ADR-0255). On a full channel the new record is dropped (drop-newest) and the
// dropped counter Inc'd; a rate-limited diagnostic is emitted at most once per
// second. On a Send error the writer re-opens the stream once (re-sending the
// identifier on the same record's resend) before dropping the entry.
type GrpcAccessLogSink struct {
	ch          chan any
	client      alsClient
	logName     string
	node        *corev3.Node
	logsWritten *stats.Counter
	logsDropped *stats.Counter
	done        chan struct{}
	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64

	// ctx/cancel bound the lifetime of the client-streaming RPC; Close cancels
	// ctx to unwedge a stalled Send/CloseAndRecv (bounded network shutdown).
	ctx    context.Context
	cancel context.CancelFunc
}

// NewGrpcAccessLogSink builds a gRPC ALS sink over client with the bounded
// channel at the default capacity (4096) and starts the writer goroutine.
func NewGrpcAccessLogSink(client alsClient, logName string, node *corev3.Node, written, dropped *stats.Counter) *GrpcAccessLogSink {
	return newGrpcSinkWithCapacity(client, logName, node, written, dropped, defaultChannelCapacity)
}

// newGrpcSinkWithCapacity is the test-friendly variant; production callers use
// NewGrpcAccessLogSink (capacity 4096).
func newGrpcSinkWithCapacity(client alsClient, logName string, node *corev3.Node, written, dropped *stats.Counter, capacity int) *GrpcAccessLogSink {
	s := &GrpcAccessLogSink{
		ch:          make(chan any, capacity),
		client:      client,
		logName:     logName,
		node:        node,
		logsWritten: written,
		logsDropped: dropped,
		done:        make(chan struct{}),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return s
}

// Submit non-blocking-sends r on the channel. On a full channel the record is
// dropped, the counter Inc'd, and at most one diagnostic emitted per second
// (the AsyncFileSink drop-newest idiom).
func (s *GrpcAccessLogSink) Submit(r any) {
	select {
	case s.ch <- r:
	default:
		// logsDropped counts channel-full (overflow) drops only; an entry lost
		// after the one-shot reconnect failure is logged in run(), not counted here.
		s.logsDropped.Inc()
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("accesslog: gRPC ALS channel full, dropping record (log_name=%s)", s.logName)
		}
	}
}

// Close closes the channel, waits up to closeDrainGrace for the writer goroutine
// to drain the in-flight stream (and CloseAndRecv it), then cancels the stream
// context to guarantee bounded shutdown even if a Send/CloseAndRecv is wedged on
// a stalled peer, and finally closes the underlying client. Idempotent and
// threadsafe via sync.Once.
//
// Lifecycle contract: callers MUST NOT call Submit once Close has begun — a send
// on the now-closed channel would panic (consistent with AsyncFileSink's
// implicit contract; enforcement is a project-wide concern, not a per-sink guard).
func (s *GrpcAccessLogSink) Close() error {
	s.closeOnce.Do(func() {
		close(s.ch)
		select {
		case <-s.done:
		case <-time.After(closeDrainGrace):
			s.cancel() // abort a wedged Send/CloseAndRecv so the writer can exit
			<-s.done
		}
		s.cancel() // release the context (idempotent; covers the normal path — satisfies vet lostcancel)
		s.closeErr = s.client.Close()
	})
	return s.closeErr
}

// run is the writer goroutine: drain channel-receives into per-record
// client-streaming Sends over a single reused stream, re-establishing it on a
// Send error. The identifier is attached to the first successful Send of the
// sink's life (and re-armed across a reconnect for the same record).
func (s *GrpcAccessLogSink) run() {
	defer close(s.done)
	var stream accesslogv3.AccessLogService_StreamAccessLogsClient
	sentIdentifier := false

	for r := range s.ch {
		rec, ok := r.(*Record)
		if !ok {
			log.Printf("accesslog: gRPC ALS sink got non-*Record %T (log_name=%s); dropping", r, s.logName)
			continue // non-Record ignored
		}
		// Up to two attempts: the initial Send plus one reconnect-and-resend.
		for attempt := 0; attempt < 2; attempt++ {
			if stream == nil {
				st, err := s.client.StreamAccessLogs(s.ctx)
				if err != nil {
					log.Printf("accesslog: gRPC ALS open stream (log_name=%s): %v", s.logName, err)
					break // leave stream nil; the next record retries
				}
				stream = st
				sentIdentifier = false
			}
			msg := &accesslogv3.StreamAccessLogsMessage{
				LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
					HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
						LogEntry: []*dataaccesslogv3.HTTPAccessLogEntry{buildHTTPAccessLogEntry(rec)},
					},
				},
			}
			if !sentIdentifier {
				msg.Identifier = &accesslogv3.StreamAccessLogsMessage_Identifier{
					Node:    s.node,
					LogName: s.logName,
				}
			}
			if err := stream.Send(msg); err != nil {
				log.Printf("accesslog: gRPC ALS send (log_name=%s): %v", s.logName, err)
				stream = nil
				sentIdentifier = false
				continue // reconnect-and-resend this record once
			}
			sentIdentifier = true
			s.logsWritten.Inc()
			break
		}
	}

	if stream != nil {
		if _, err := stream.CloseAndRecv(); err != nil {
			log.Printf("accesslog: gRPC ALS close-and-recv (log_name=%s): %v", s.logName, err)
		}
	}
}
