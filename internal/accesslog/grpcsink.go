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
	"google.golang.org/protobuf/proto"

	"github.com/pgdad/envoy-go/internal/stats"
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
// (ADR-0255). The writer ACCUMULATES entries into an in-memory buffer and Sends
// a BATCH as one StreamAccessLogsMessage on the FIRST of three triggers: the
// accumulated serialized bytes reaching bufferSizeBytes (AMEND-BUF-1; 0 ⇒
// flush-every-entry), the bufferFlushInterval timer ticking (AMEND-BUF-2), or
// Close draining the pending buffer (AMEND-BUF-5). On a full channel the new
// record is dropped (drop-newest), logsDropped Inc'd, and a rate-limited
// diagnostic emitted at most once per second. On a Send error the writer
// re-opens the stream once and resends the WHOLE batch; a stream-open failure or
// a second-Send failure drops the batch logged-not-counted (memory stays bounded
// under a sustained outage). logsWritten counts ENTRIES, not messages
// (batch-invariant — AMEND-BUF-4).
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

	bufferSizeBytes     int           // accumulated-serialized-byte flush threshold (AMEND-BUF-1); 0 ⇒ flush-every-entry
	bufferFlushInterval time.Duration // flush-interval timer period (AMEND-BUF-2; guaranteed > 0 by the parse layer)

	// additionalRequestHeaders/additionalResponseHeaders are THIS sink's configured
	// header names (lowercased at parse; AMEND-HDR-1). buildHTTPAccessLogEntry
	// filters the emit-hook capture UNION (Record.RequestHeaders/ResponseHeaders)
	// down to these names per-sink, and the Filter reads them via the
	// headerCaptureSink interface (Capture{Request,Response}HeaderNames) to build
	// that union (D-HDR-SINK-FILTER).
	additionalRequestHeaders  []string
	additionalResponseHeaders []string

	// ctx/cancel bound the lifetime of the client-streaming RPC; Close cancels
	// ctx to unwedge a stalled Send/CloseAndRecv (bounded network shutdown).
	ctx    context.Context
	cancel context.CancelFunc
}

// NewGrpcAccessLogSink builds a gRPC ALS sink over client with the bounded
// channel at the default capacity (4096) and starts the writer goroutine.
func NewGrpcAccessLogSink(client alsClient, logName string, node *corev3.Node, written, dropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration, additionalRequestHeaders, additionalResponseHeaders []string) *GrpcAccessLogSink {
	return newGrpcSinkWithCapacity(client, logName, node, written, dropped, bufferSizeBytes, bufferFlushInterval, additionalRequestHeaders, additionalResponseHeaders, defaultChannelCapacity)
}

// newGrpcSinkWithCapacity is the test-friendly variant; production callers use
// NewGrpcAccessLogSink (capacity 4096).
func newGrpcSinkWithCapacity(client alsClient, logName string, node *corev3.Node, written, dropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration, additionalRequestHeaders, additionalResponseHeaders []string, capacity int) *GrpcAccessLogSink {
	s := &GrpcAccessLogSink{
		ch:                        make(chan any, capacity),
		client:                    client,
		logName:                   logName,
		node:                      node,
		logsWritten:               written,
		logsDropped:               dropped,
		bufferSizeBytes:           bufferSizeBytes,
		bufferFlushInterval:       bufferFlushInterval,
		additionalRequestHeaders:  additionalRequestHeaders,
		additionalResponseHeaders: additionalResponseHeaders,
		done:                      make(chan struct{}),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return s
}

// CaptureRequestHeaderNames / CaptureResponseHeaderNames implement the
// hcm.headerCaptureSink interface: the HCM Filter reads these to build the
// emit-hook capture UNION across all ALS sinks (D-HDR-SINK-FILTER). Returning
// the configured names (nil when none) keeps capture inert under the no-config
// path (an empty union ⇒ the emit hooks skip CaptureHeaders entirely).
func (s *GrpcAccessLogSink) CaptureRequestHeaderNames() []string { return s.additionalRequestHeaders }

// CaptureResponseHeaderNames returns this sink's configured response-header names
// (see CaptureRequestHeaderNames).
func (s *GrpcAccessLogSink) CaptureResponseHeaderNames() []string {
	return s.additionalResponseHeaders
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

// run is the writer goroutine: drain channel-receives into a buffer and Send a
// BATCH on a size-OR-timer trigger over a single reused stream, re-establishing
// it on a Send error. The identifier is attached to the first successful flush
// of the sink's life (and re-armed across a reconnect). The 44.1 one-entry-per-
// message behavior is the bufferSizeBytes==0 degenerate case (every entry
// crosses sum >= 0 immediately).
func (s *GrpcAccessLogSink) run() {
	defer close(s.done)

	var stream accesslogv3.AccessLogService_StreamAccessLogsClient
	sentIdentifier := false
	var buf []*dataaccesslogv3.HTTPAccessLogEntry
	bufBytes := 0

	// flush Sends the accumulated batch as ONE StreamAccessLogsMessage, with the
	// identifier on the first successful flush of the stream's life (re-armed
	// across a reconnect). Up to two attempts: the initial Send plus one
	// reconnect-and-resend-the-WHOLE-batch. On success logsWritten += len(buf)
	// (batch-invariant — AMEND-BUF-4); on a stream-open failure OR a second-Send
	// failure the batch is dropped (logged, not counted) so memory stays bounded
	// under a sustained outage — matching 44.1's open-failure-drops-the-record
	// policy. logs_dropped stays channel-full-overflow-only (AMEND-ALS-1), so a
	// flush-path drop is logged but NOT counted there. Empty buf is a no-op (the
	// timer's idle tick).
	//
	// buf-reuse contract: on completion buf is truncated (buf[:0]) and its backing
	// array is reused by the next batch's append. The real gRPC Send serializes
	// the message synchronously before returning, so the bytes are captured before
	// reuse (zero extra allocation in production). The test fake records the
	// message pointer, so it takes a defensive copy of GetLogEntry() in its Send.
	flush := func() {
		if len(buf) == 0 {
			return
		}
		for attempt := 0; attempt < 2; attempt++ {
			if stream == nil {
				st, err := s.client.StreamAccessLogs(s.ctx)
				if err != nil {
					log.Printf("accesslog: gRPC ALS open stream (log_name=%s): %v", s.logName, err)
					break // drop the batch (reset below) — bounds memory under a sustained outage
				}
				stream = st
				sentIdentifier = false
			}
			msg := &accesslogv3.StreamAccessLogsMessage{
				LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
					HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
						LogEntry: buf,
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
				continue // reconnect-and-resend the whole batch once
			}
			sentIdentifier = true
			s.logsWritten.Add(uint64(len(buf)))
			break
		}
		// Reset the buffer whether the batch was sent or dropped — a dropped batch
		// (open failure OR second-Send failure) is logged-not-counted, matching the
		// 44.1 drop policy and keeping memory bounded under a sustained outage.
		buf = buf[:0]
		bufBytes = 0
	}

	ticker := time.NewTicker(s.bufferFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case r, ok := <-s.ch:
			if !ok {
				flush() // AMEND-BUF-5: drain the pending buffer before CloseAndRecv
				if stream != nil {
					if _, err := stream.CloseAndRecv(); err != nil {
						log.Printf("accesslog: gRPC ALS close-and-recv (log_name=%s): %v", s.logName, err)
					}
				}
				return
			}
			rec, ok := r.(*Record)
			if !ok {
				log.Printf("accesslog: gRPC ALS sink got non-*Record %T (log_name=%s); dropping", r, s.logName)
				continue // non-Record ignored
			}
			entry := buildHTTPAccessLogEntry(rec, s.additionalRequestHeaders, s.additionalResponseHeaders)
			buf = append(buf, entry)
			if s.bufferSizeBytes > 0 { // skip the proto.Size walk when the accumulated size is never consulted
				bufBytes += proto.Size(entry)
			}
			if bufBytes >= s.bufferSizeBytes { // SIZE trigger (AMEND-BUF-1); 0 ⇒ every entry flushes
				flush()
			}
		case <-ticker.C:
			flush() // TIMER trigger (AMEND-BUF-2; no-op if buf empty)
		}
	}
}
