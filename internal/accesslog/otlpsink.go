package accesslog

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/proto"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"

	"github.com/esalaine/envoy-go/internal/stats"
)

// otlpClient is the minimal sink-facing seam over *grpcclient.OTLPLogsClient
// (test-fakeable). *grpcclient.OTLPLogsClient satisfies it structurally, so the
// sink need not import grpcclient (no import cycle; main wiring passes the
// concrete client).
type otlpClient interface {
	Export(ctx context.Context, req *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error)
	Close() error
}

// OTLPAccessLogSink is the project's THIRD accesslog.Sink: it ships built-in OTLP
// LogRecords to an OpenTelemetry LogsService over a UNARY Export RPC. It mirrors
// GrpcAccessLogSink's bounded-channel + writer-goroutine + size/interval/close
// buffer + idempotent-Close shape (ADR-0258), but is SIMPLER: no stream, no
// identifier, no CloseAndRecv — every flush is a self-contained Export of a
// LogRecord batch. The writer ACCUMULATES records into a buffer and Exports a
// BATCH on the FIRST of three triggers: the accumulated serialized bytes reaching
// bufferSizeBytes (0 ⇒ flush-every-entry), the bufferFlushInterval timer ticking,
// or Close draining the pending buffer. On a full channel the new record is
// dropped (drop-newest), logsDropped Inc'd, and a rate-limited diagnostic emitted
// at most once per second. On an Export error the writer retries the WHOLE batch
// once; a second consecutive failure drops the batch logged-not-counted (memory
// stays bounded under a sustained outage). logsWritten counts RECORDS, not Export
// messages (batch-invariant).
type OTLPAccessLogSink struct {
	ch                   chan any
	client               otlpClient
	logName              string
	node                 *corev3.Node
	disableBuiltinLabels bool
	// body/attrs are the compiled %OPERATOR%-templates substituted per-record at
	// buffer-append (buildLogRecord); resourceAttrs is the literal,
	// substitution-free resource_attributes appended once per Export at flush
	// (buildResource). All three are immutable after construction: body/attrs
	// templates Eval a fresh AnyValue per record, and resourceAttrs are
	// shared-immutable proto pointers read READ-ONLY by the writer goroutine — no
	// lock needed.
	body          *OTLPValueTemplate
	attrs         []OTLPAttrTemplate
	resourceAttrs []*commonpb.KeyValue
	logsWritten   *stats.Counter
	logsDropped   *stats.Counter
	done          chan struct{}
	closeOnce     sync.Once
	closeErr      error
	lastDropLog   atomic.Int64

	bufferSizeBytes     int           // accumulated-serialized-byte flush threshold; 0 ⇒ flush-every-entry
	bufferFlushInterval time.Duration // flush-interval timer period (guaranteed > 0 by the parse layer)

	// ctx/cancel bound the lifetime of the in-flight Export RPC; Close cancels
	// ctx to unwedge a stalled Export (bounded network shutdown).
	ctx    context.Context
	cancel context.CancelFunc
}

// NewOTLPAccessLogSink builds an OTLP access-log sink over client with the bounded
// channel at the default capacity (4096) and starts the writer goroutine. body/attrs
// are the compiled body/attributes templates (nil/empty ⇒ the 45.1 LEAN built-in
// path); resourceAttrs is the literal resource_attributes (appended after the 4
// built-in labels, surviving disableBuiltinLabels — AMEND-OPS-5). The three template
// args are placed AFTER disableBuiltinLabels and BEFORE the counters.
func NewOTLPAccessLogSink(client otlpClient, logName string, node *corev3.Node, disableBuiltinLabels bool, body *OTLPValueTemplate, attrs []OTLPAttrTemplate, resourceAttrs []*commonpb.KeyValue, written, dropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration) *OTLPAccessLogSink {
	return newOTLPSinkWithCapacity(client, logName, node, disableBuiltinLabels, body, attrs, resourceAttrs, written, dropped, bufferSizeBytes, bufferFlushInterval, defaultChannelCapacity)
}

// newOTLPSinkWithCapacity is the test-friendly variant; production callers use
// NewOTLPAccessLogSink (capacity 4096).
func newOTLPSinkWithCapacity(client otlpClient, logName string, node *corev3.Node, disableBuiltinLabels bool, body *OTLPValueTemplate, attrs []OTLPAttrTemplate, resourceAttrs []*commonpb.KeyValue, written, dropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration, capacity int) *OTLPAccessLogSink {
	s := &OTLPAccessLogSink{
		ch:                   make(chan any, capacity),
		client:               client,
		logName:              logName,
		node:                 node,
		disableBuiltinLabels: disableBuiltinLabels,
		body:                 body,
		attrs:                attrs,
		resourceAttrs:        resourceAttrs,
		logsWritten:          written,
		logsDropped:          dropped,
		bufferSizeBytes:      bufferSizeBytes,
		bufferFlushInterval:  bufferFlushInterval,
		done:                 make(chan struct{}),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return s
}

// Submit non-blocking-sends r on the channel. On a full channel the record is
// dropped, the counter Inc'd, and at most one diagnostic emitted per second (the
// drop-newest idiom shared with the file/gRPC sinks).
func (s *OTLPAccessLogSink) Submit(r any) {
	select {
	case s.ch <- r:
	default:
		// logsDropped counts channel-full (overflow) drops only; a batch lost after
		// the retry failure is logged in run(), not counted here.
		s.logsDropped.Inc()
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("accesslog: OTLP channel full, dropping record (log_name=%s)", s.logName)
		}
	}
}

// Close closes the channel, waits up to closeDrainGrace for the writer goroutine
// to drain the pending buffer (and flush it via Export), then cancels the Export
// context to guarantee bounded shutdown even if an Export is wedged on a stalled
// peer, and finally closes the underlying client. Idempotent and threadsafe via
// sync.Once.
//
// Lifecycle contract: callers MUST NOT call Submit once Close has begun — a send
// on the now-closed channel would panic (consistent with the other sinks'
// implicit contract; enforcement is a project-wide concern, not a per-sink guard).
func (s *OTLPAccessLogSink) Close() error {
	s.closeOnce.Do(func() {
		close(s.ch)
		select {
		case <-s.done:
		case <-time.After(closeDrainGrace):
			s.cancel() // abort a wedged Export so the writer can exit
			<-s.done
		}
		s.cancel() // release the context (idempotent; covers the normal path — satisfies vet lostcancel)
		s.closeErr = s.client.Close()
	})
	return s.closeErr
}

// run is the writer goroutine: drain channel-receives into a buffer and Export a
// BATCH on a size-OR-timer trigger, retrying the whole batch once on an Export
// error. Simpler than the gRPC sink: each Export is self-contained (no stream, no
// identifier, no CloseAndRecv). The flush-every-entry behavior is the
// bufferSizeBytes==0 degenerate case (every record crosses sum >= 0 immediately).
func (s *OTLPAccessLogSink) run() {
	defer close(s.done)

	var buf []*logspb.LogRecord
	bufBytes := 0

	// flush Exports the accumulated batch as ONE ExportLogsServiceRequest. Up to
	// two attempts: the initial Export plus one retry of the WHOLE batch. On
	// success logsWritten += len(buf) (batch-invariant); on a second consecutive
	// failure the batch is dropped (logged, not counted) so memory stays bounded
	// under a sustained outage. Empty buf is a no-op (the timer's idle tick).
	//
	// buf-reuse contract: on completion buf is truncated (buf[:0]) and its backing
	// array is reused by the next batch's append. The real unary Export serializes
	// the request synchronously before returning, so the bytes are captured before
	// reuse (zero extra allocation in production). The test fake retains the
	// request pointer, so it takes a defensive deep copy in its Export.
	flush := func() {
		if len(buf) == 0 {
			return
		}
		req := buildExportRequest(buf, s.node, s.logName, s.disableBuiltinLabels, s.resourceAttrs)
		for attempt := 0; attempt < 2; attempt++ {
			if _, err := s.client.Export(s.ctx, req); err == nil {
				s.logsWritten.Add(uint64(len(buf)))
				break
			} else {
				log.Printf("accesslog: OTLP export (log_name=%s, attempt=%d): %v", s.logName, attempt+1, err)
			}
		}
		// Reset the buffer whether the batch was sent or dropped — a dropped batch
		// (second-Export failure) is logged-not-counted, keeping memory bounded.
		buf = buf[:0]
		bufBytes = 0
	}

	ticker := time.NewTicker(s.bufferFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case r, ok := <-s.ch:
			if !ok {
				flush() // drain the pending buffer on close
				return
			}
			rec, ok := r.(*Record)
			if !ok {
				log.Printf("accesslog: OTLP sink got non-*Record %T (log_name=%s); dropping", r, s.logName)
				continue // non-Record ignored
			}
			lr := buildLogRecord(rec, s.body, s.attrs)
			buf = append(buf, lr)
			bufBytes += proto.Size(lr)
			if bufBytes >= s.bufferSizeBytes { // SIZE trigger; 0 ⇒ every record flushes
				flush()
			}
		case <-ticker.C:
			flush() // TIMER trigger (no-op if buf empty)
		}
	}
}
