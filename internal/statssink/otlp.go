package statssink

// otlp.go — phase 69 (OTLP metrics stats sink, ADR-0291). The FIFTH stats_sinks[]
// consumer over the landed phase-47 flush subsystem. Each Submit is one complete
// full-registry snapshot; the writer goroutine maps it to ONE unary OTLP
// ExportMetricsServiceRequest and sends it (retry-once, fail-open). The
// otlpMetricsClient seam keeps the PRODUCTION statssink package grpcclient-free
// (RD-SEAM) — main.go wires the real *grpcclient.OTLPMetricsClient. The mapping is
// UNARY (the OTLP-logs/traces sink precedent), NOT the streaming MetricsServiceSink
// lifecycle.

import (
	"context"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/pgdad/envoy-go/internal/stats"
)

// otlpMetricsClient is the package-local gRPC seam over
// *grpcclient.OTLPMetricsClient (the metricsClient/alsClient-seam precedent): it
// decouples the PRODUCTION sink from grpcclient (no production import) and keeps the
// package acyclic. otlp_test.go fakes it, and a TEST-ONLY compile-time assertion
// there pins that the real client satisfies it. Export is UNARY (return narrowed to
// error); the sink's writer goroutine bounds ctx and retries once.
type otlpMetricsClient interface {
	Export(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) error
	Close() error
}

// OTLPMetricsSink maps each full-registry snapshot batch to ONE OTLP
// ExportMetricsServiceRequest and sends it over a lazily-driven unary Export. It
// mirrors MetricsServiceSink's bounded-channel + writer-goroutine + idempotent-Close
// shape (ADR-0262) but the run body is UNARY (retry-once client.Export, not a
// stream). On a full channel the newest flush is dropped (drop-newest) and a rate-
// limited diagnostic emitted (drops are LOGGED not counted — +0 stats,
// TestNoNewStat_OTLPRegistrationGuard). On an Export error the writer retries the
// WHOLE request once; a second failure drops the flush (logged, not counted).
//
// Mapping (toExportRequest): ONE ResourceMetrics{Resource: telemetry.sdk.* triple}
// → ONE ScopeMetrics{Scope: EMPTY} → one Metric per family. COUNTER →
// Metric_Sum{IsMonotonic:true, AggregationTemporality: CUMULATIVE (default) or DELTA
// (reportCountersAsDeltas), DataPoints:[{Attributes, StartTimeUnixNano,
// TimeUnixNano: flushNanos, AsDouble}]}. GAUGE → Metric_Gauge{DataPoints:[{Attributes,
// TimeUnixNano: flushNanos, AsDouble}]} — NO StartTime on a gauge. The metric name
// is the tag-extracted residual when useTagExtractedName (else the full dotted
// name), prefix-composed; the extracted tags are emitted as envoy.<tag> KeyValue
// attributes when emitTagsAsAttributes — the two knobs are INDEPENDENT (RD-TAGS).
//
// When reportCountersAsDeltas, the writer rewrites COUNTER families to per-flush
// deltas via a per-sink deltaState just before mapping (ADR-0263); gauges stay
// absolute; the delta Sum RETAINS IsMonotonic and chains StartTime to the previous
// flush's TimeUnixNano. Applying the delta in the writer (not Submit) means an
// enqueue-drop never latches deltaState. Both transforms build the sink's OWN protos
// and never mutate the shared snapshot slice (the ADR-0263/0264 hard constraint).
type OTLPMetricsSink struct {
	ch     chan []*dto.MetricFamily
	client otlpMetricsClient

	reportCountersAsDeltas bool
	useTagExtractedName    bool
	emitTagsAsAttributes   bool
	prefix                 string

	delta          *deltaState          // non-nil ⇒ report_counters_as_deltas (ADR-0263)
	resourceAttrs  []*commonpb.KeyValue // the telemetry.sdk.* triple, built once
	startNanos     uint64               // process-start ns; cumulative StartTime constant
	lastFlushNanos uint64               // delta StartTime window start; chains to each flush's TimeUnixNano

	done        chan struct{}
	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64

	// ctx/cancel bound the lifetime of an in-flight Export; Close cancels ctx to
	// unwedge a stalled unary RPC (bounded network shutdown).
	ctx    context.Context
	cancel context.CancelFunc
}

// NewOTLPMetricsSink builds an OTLP metrics sink over client with the bounded
// channel at the default capacity and starts the writer goroutine. version is the
// telemetry.sdk.version resource value (main.go threads admin.BuildVersionString()).
func NewOTLPMetricsSink(client otlpMetricsClient, version string, reportCountersAsDeltas, useTagExtractedName, emitTagsAsAttributes bool, prefix string) *OTLPMetricsSink {
	return newOTLPSinkWithCapacity(client, version, reportCountersAsDeltas, useTagExtractedName, emitTagsAsAttributes, prefix, defaultChannelCapacity)
}

func newOTLPSinkWithCapacity(client otlpMetricsClient, version string, reportCountersAsDeltas, useTagExtractedName, emitTagsAsAttributes bool, prefix string, capacity int) *OTLPMetricsSink {
	now := uint64(time.Now().UnixNano())
	s := &OTLPMetricsSink{
		ch:                     make(chan []*dto.MetricFamily, capacity),
		client:                 client,
		reportCountersAsDeltas: reportCountersAsDeltas,
		useTagExtractedName:    useTagExtractedName,
		emitTagsAsAttributes:   emitTagsAsAttributes,
		prefix:                 prefix,
		resourceAttrs: []*commonpb.KeyValue{
			otlpKV("telemetry.sdk.name", "envoy-go"),
			otlpKV("telemetry.sdk.language", "go"),
			otlpKV("telemetry.sdk.version", version),
		},
		startNanos:     now,
		lastFlushNanos: now,
		done:           make(chan struct{}),
	}
	if reportCountersAsDeltas {
		s.delta = newDeltaState()
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return s
}

// Submit non-blocking-sends batch on the channel. On a full channel the batch is
// dropped and at most one diagnostic emitted per second (the drop-newest idiom);
// drops are LOGGED, not counted. The delta transform runs in the writer goroutine
// (just before mapping), so an enqueue-drop never latches deltaState.
//
// Lifecycle contract: callers MUST NOT call Submit once Close has begun (the Flusher
// stops before Close) — a send on the now-closed channel would panic.
func (s *OTLPMetricsSink) Submit(batch []*dto.MetricFamily) {
	select {
	case s.ch <- batch:
	default:
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("statssink: otlp metrics channel full, dropping flush batch")
		}
	}
}

// run is the writer goroutine: dequeue each batch, apply the delta transform (if
// delta-mode) HERE (single goroutine — the no-lock deltaState contract holds; an
// enqueue-drop never latched it), map to ONE ExportMetricsServiceRequest, and send
// it unary with a single retry. A second failure drops the flush (logged, not
// counted).
func (s *OTLPMetricsSink) run() {
	defer close(s.done)
	for batch := range s.ch {
		flushNanos := uint64(time.Now().UnixNano())
		if s.delta != nil {
			batch = s.delta.apply(batch) // build the sink's OWN delta batch; never mutate the shared slice
		}
		req := s.toExportRequest(batch, flushNanos)
		s.flush(req)
	}
}

// flush sends req over the unary Export, retrying the WHOLE request once on error.
// A second failure drops the flush (logged, not counted — fail-open, +0 stats).
func (s *OTLPMetricsSink) flush(req *colmetricspb.ExportMetricsServiceRequest) {
	for attempt := 0; attempt < 2; attempt++ {
		if err := s.client.Export(s.ctx, req); err != nil {
			if attempt == 0 {
				continue // retry the whole request once
			}
			log.Printf("statssink: otlp metrics export dropped after retry: %v", err)
			return
		}
		return
	}
}

// toExportRequest maps a family batch to ONE ExportMetricsServiceRequest. It builds
// the sink's OWN protos and NEVER mutates the input families (the shared-snapshot
// constraint). In delta-mode it advances lastFlushNanos AFTER reading it (safe: run
// is the single writer goroutine).
func (s *OTLPMetricsSink) toExportRequest(batch []*dto.MetricFamily, flushNanos uint64) *colmetricspb.ExportMetricsServiceRequest {
	startNanos := s.startNanos // cumulative: fixed process-start constant
	temporality := metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE
	if s.delta != nil {
		startNanos = s.lastFlushNanos // delta: chain to the previous flush's TimeUnixNano
		temporality = metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
	}

	metrics := make([]*metricspb.Metric, 0, len(batch))
	for _, fam := range batch {
		residual, labels, err := stats.ExtractTags(fam.GetName())

		// Name: tag-extracted residual when useTagExtractedName (fall back to the
		// full dotted name on an ExtractTags error — the labelMapper posture), then
		// prefix-composed.
		base := fam.GetName()
		if s.useTagExtractedName && err == nil {
			base = residual
		}
		name := base
		if s.prefix != "" {
			name = s.prefix + "." + base
		}

		// Attributes: the extracted tags as envoy.<tag> KeyValues when
		// emitTagsAsAttributes (independent of the name knob — RD-TAGS).
		var attrs []*commonpb.KeyValue
		if s.emitTagsAsAttributes && err == nil {
			attrs = kvFromTags(labels)
		}

		switch fam.GetType() {
		case dto.MetricType_COUNTER:
			dps := make([]*metricspb.NumberDataPoint, 0, len(fam.GetMetric()))
			for _, m := range fam.GetMetric() {
				dps = append(dps, &metricspb.NumberDataPoint{
					Attributes:        attrs,
					StartTimeUnixNano: startNanos,
					TimeUnixNano:      flushNanos,
					Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: m.GetCounter().GetValue()},
				})
			}
			metrics = append(metrics, &metricspb.Metric{
				Name: name,
				Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
					DataPoints:             dps,
					AggregationTemporality: temporality,
					IsMonotonic:            true,
				}},
			})
		case dto.MetricType_GAUGE:
			dps := make([]*metricspb.NumberDataPoint, 0, len(fam.GetMetric()))
			for _, m := range fam.GetMetric() {
				dps = append(dps, &metricspb.NumberDataPoint{
					Attributes:   attrs,
					TimeUnixNano: flushNanos, // NO StartTime on a gauge
					Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: m.GetGauge().GetValue()},
				})
			}
			metrics = append(metrics, &metricspb.Metric{
				Name: name,
				Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: dps}},
			})
		default:
			// Non-counter/gauge families are not produced by snapshot() (SN7:
			// histograms not emitted). Skip defensively rather than emit an
			// invalid Metric with no Data oneof.
			continue
		}
	}

	if s.delta != nil {
		s.lastFlushNanos = flushNanos // advance the delta window (single writer goroutine)
	}

	return &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource: &resourcepb.Resource{Attributes: s.resourceAttrs},
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Scope:   &commonpb.InstrumentationScope{}, // EMPTY scope
				Metrics: metrics,
			}},
		}},
	}
}

// kvFromTags maps ExtractTags labels to OTLP KeyValue attributes keyed by the Envoy
// DOTTED tag-name (envoy.<tag>, from the envoy_ key via "envoy."+TrimPrefix) — the
// labelMapper key idiom (label.go:46), values as StringValue.
func kvFromTags(labels []stats.Label) []*commonpb.KeyValue {
	if len(labels) == 0 {
		return nil
	}
	out := make([]*commonpb.KeyValue, 0, len(labels))
	for _, l := range labels {
		out = append(out, otlpKV("envoy."+strings.TrimPrefix(l.Key, "envoy_"), l.Value))
	}
	return out
}

// otlpKV builds a StringValue KeyValue (the accesslog otlpmapping idiom).
func otlpKV(k, v string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
}

// Close closes the channel, waits up to closeDrainGrace for the writer goroutine to
// drain, then cancels the in-flight Export context to guarantee bounded shutdown, and
// finally closes the underlying client. Idempotent and threadsafe via sync.Once.
func (s *OTLPMetricsSink) Close() error {
	s.closeOnce.Do(func() {
		close(s.ch)
		select {
		case <-s.done:
		case <-time.After(closeDrainGrace):
			s.cancel() // abort a wedged Export so the writer can exit
			<-s.done
		}
		s.cancel() // release the context (idempotent; satisfies vet lostcancel)
		s.closeErr = s.client.Close()
	})
	return s.closeErr
}
