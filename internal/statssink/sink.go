package statssink

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
	dto "github.com/prometheus/client_model/go"
)

// Sink consumes one complete MetricFamily batch per flush. The Flusher Submits a
// full-registry snapshot per stats_flush_interval; Close drains the in-flight
// stream and releases the underlying gRPC client.
type Sink interface {
	Submit(batch []*dto.MetricFamily)
	Close() error
}

// metricsClient is the gRPC seam over *grpcclient.MetricsServiceClient (the
// accesslog alsClient-seam precedent): it decouples the PRODUCTION sink from
// grpcclient (no production import) and keeps the package acyclic. sink_test.go
// fakes it, and a TEST-ONLY compile-time assertion there pins that the real
// client satisfies it.
type metricsClient interface {
	StreamMetrics(ctx context.Context) (metricsv3.MetricsService_StreamMetricsClient, error)
	Close() error
}

const (
	// defaultChannelCapacity bounds the in-flight flush batches. The Flusher is
	// the trigger (each Submit is one complete full-registry snapshot), so a small
	// capacity gives slack for a slow stream without unbounded growth.
	defaultChannelCapacity = 8

	// closeDrainGrace bounds how long Close waits for the in-flight client-
	// streaming RPC to drain before canceling the stream context — guaranteeing
	// bounded shutdown even if a Send/CloseAndRecv is wedged on a stalled peer.
	closeDrainGrace = 2 * time.Second

	// dropLogIntervalNanos rate-limits the channel-full diagnostic to at most once
	// per second (the accesslog lastDropLog idiom). Drops are LOGGED not counted
	// (D-MS-STATS-FINAL: zero metrics_service-scoped self-stats).
	dropLogIntervalNanos = int64(time.Second)
)

// MetricsServiceSink streams full-registry snapshot batches to an Envoy
// MetricsService over a lazily-established, identifier-once, reused client-
// streaming RPC. It mirrors accesslog.GrpcAccessLogSink's bounded-channel +
// writer-goroutine + idempotent-Close shape (ADR-0262), SIMPLIFIED to a 1-deep
// handoff: each Submit is one already-complete batch, so the writer does NO
// size/interval accumulation. On a full channel the flush is dropped (drop-
// newest) and a rate-limited diagnostic emitted. On a Send error the writer
// re-opens the stream once and re-sends the WHOLE batch (re-arming the
// identifier); a stream-open failure or a second-Send failure drops the batch
// (logged, not counted).
//
// When constructed with reportCountersAsDeltas=true, the writer goroutine rewrites
// COUNTER families to per-flush deltas via a per-sink deltaState just before flush
// (ADR-0263); gauges stay absolute. When constructed with emitTagsAsLabels=true,
// the writer then rewrites each family's Name to the tag-residual and emits the
// extracted tags as metric[].label[] LabelPairs via a per-sink labelMapper
// (ADR-0264); applied AFTER the delta so labels ride the (possibly delta-rewritten)
// value. Both transforms build the sink's own batch and never mutate the shared
// snapshot slice. Applying the delta in the writer (not Submit) means an
// enqueue-drop never latches deltaState — the dropped increments ride the next
// successfully-enqueued flush instead of being silently lost.
type MetricsServiceSink struct {
	ch          chan []*dto.MetricFamily
	client      metricsClient
	node        *corev3.Node
	delta       *deltaState  // non-nil ⇒ report_counters_as_deltas (ADR-0263); nil ⇒ absolute
	labels      *labelMapper // non-nil ⇒ emit_tags_as_labels (ADR-0264); nil ⇒ full dotted name, no labels
	done        chan struct{}
	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64

	// ctx/cancel bound the lifetime of the client-streaming RPC; Close cancels
	// ctx to unwedge a stalled Send/CloseAndRecv (bounded network shutdown).
	ctx    context.Context
	cancel context.CancelFunc
}

// NewMetricsServiceSink builds a metrics-service sink over client with the
// bounded channel at the default capacity and starts the writer goroutine.
func NewMetricsServiceSink(client metricsClient, node *corev3.Node, reportCountersAsDeltas, emitTagsAsLabels bool) *MetricsServiceSink {
	return newSinkWithCapacity(client, node, reportCountersAsDeltas, emitTagsAsLabels, defaultChannelCapacity)
}

// newSinkWithCapacity is the test-friendly variant; production callers use
// NewMetricsServiceSink (default capacity).
func newSinkWithCapacity(client metricsClient, node *corev3.Node, reportCountersAsDeltas, emitTagsAsLabels bool, capacity int) *MetricsServiceSink {
	s := &MetricsServiceSink{
		ch:     make(chan []*dto.MetricFamily, capacity),
		client: client,
		node:   node,
		done:   make(chan struct{}),
	}
	if reportCountersAsDeltas {
		s.delta = newDeltaState()
	}
	if emitTagsAsLabels {
		s.labels = newLabelMapper()
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return s
}

// Submit non-blocking-sends batch on the channel. On a full channel the batch is
// dropped and at most one diagnostic emitted per second (the accesslog drop-
// newest idiom). Drops are logged, not counted (D-MS-STATS-FINAL). The delta/
// labels transforms run in the writer goroutine (just before flush), so an
// enqueue-drop never latches deltaState — the dropped increments are carried by
// the next successfully-enqueued flush's delta instead of being lost.
//
// Lifecycle contract: callers MUST NOT call Submit once Close has begun — a send
// on the now-closed channel would panic (the Flusher stops before Close).
func (s *MetricsServiceSink) Submit(batch []*dto.MetricFamily) {
	select {
	case s.ch <- batch:
	default:
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("statssink: metrics_service channel full, dropping flush batch")
		}
	}
}

// Close closes the channel, waits up to closeDrainGrace for the writer goroutine
// to drain the in-flight stream (and CloseAndRecv it), then cancels the stream
// context to guarantee bounded shutdown even if a Send/CloseAndRecv is wedged on
// a stalled peer, and finally closes the underlying client. Idempotent and
// threadsafe via sync.Once.
func (s *MetricsServiceSink) Close() error {
	s.closeOnce.Do(func() {
		close(s.ch)
		select {
		case <-s.done:
		case <-time.After(closeDrainGrace):
			s.cancel() // abort a wedged Send/CloseAndRecv so the writer can exit
			<-s.done
		}
		s.cancel() // release the context (idempotent; satisfies vet lostcancel)
		s.closeErr = s.client.Close()
	})
	return s.closeErr
}

// run is the writer goroutine: lazily open the stream, send the identifier once
// per stream, send each batch as one StreamMetricsMessage, reopen-and-resend once
// on a Send error, drain + CloseAndRecv on channel close.
func (s *MetricsServiceSink) run() {
	defer close(s.done)

	var stream metricsv3.MetricsService_StreamMetricsClient
	var sentIdentifier bool

	// flush sends batch on the live stream, opening it (and re-sending the
	// identifier) up to twice (initial + one reconnect). On a second failure the
	// batch is dropped (logged, not counted — D-MS-STATS-FINAL).
	flush := func(batch []*dto.MetricFamily) {
		for attempt := 0; attempt < 2; attempt++ {
			if stream == nil {
				st, err := s.client.StreamMetrics(s.ctx)
				if err != nil {
					log.Printf("statssink: metrics_service stream open: %v", err)
					return
				}
				stream, sentIdentifier = st, false
			}
			msg := &metricsv3.StreamMetricsMessage{EnvoyMetrics: batch}
			if !sentIdentifier {
				msg.Identifier = &metricsv3.StreamMetricsMessage_Identifier{Node: s.node}
			}
			if err := stream.Send(msg); err != nil {
				_, _ = stream.CloseAndRecv()
				stream = nil // reopen on the next attempt
				continue
			}
			sentIdentifier = true
			return
		}
		log.Printf("statssink: metrics_service flush dropped after reconnect")
	}

	for batch := range s.ch {
		// Apply the delta/labels transforms HERE (single writer goroutine — the
		// no-lock deltaState contract holds) so that a batch dropped at enqueue
		// never latched deltaState: the delta of the next flushed batch is
		// relative to the last batch that REACHED this point, and wire bytes for
		// every successfully-sent batch are unchanged.
		if s.delta != nil {
			batch = s.delta.apply(batch) // build the sink's OWN delta batch; never mutate the shared slice
		}
		if s.labels != nil {
			batch = s.labels.apply(batch) // rewrite Name+Label; orthogonal to delta's VALUE rewrite (delta-then-labels)
		}
		flush(batch)
	}
	if stream != nil {
		_, _ = stream.CloseAndRecv()
	}
}
