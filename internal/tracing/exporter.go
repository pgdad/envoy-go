package tracing

// exporter.go — OTLPExporter: a bounded-channel + writer-goroutine OTLP trace
// sink (ADR-0260). Mirrors internal/accesslog/otlpsink.go verbatim in
// structure, swapping LogRecord → *Span and ExportLogsServiceRequest →
// ExportTraceServiceRequest. The writer accumulates spans into a buffer and
// ships a batch on the FIRST of three triggers: accumulated serialized bytes
// reaching bufferSizeBytes (0 ⇒ flush-every-entry), the bufferFlushInterval
// timer ticking, or Close draining the pending buffer. On a full channel the
// new span is dropped (drop-newest), spansDropped Inc'd, and a rate-limited
// diagnostic emitted at most once per second. On an Export error the writer
// retries the WHOLE batch once; a second consecutive failure drops the batch
// logged-not-counted (memory stays bounded). spansSent counts SPANS, not
// Export messages (batch-invariant).

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/pgdad/envoy-go/internal/stats"
)

const (
	// exporterDefaultChannelCapacity is the bounded channel capacity for the
	// exporter (mirrors accesslog's defaultChannelCapacity).
	exporterDefaultChannelCapacity = 4096

	// exporterCloseDrainGrace bounds how long Close waits for the writer to drain
	// before canceling the Export context (bounded shutdown).
	exporterCloseDrainGrace = 5 * time.Second

	// exporterDropLogIntervalNanos is the minimum nanoseconds between rate-limited
	// drop-log lines (at most one per second).
	exporterDropLogIntervalNanos = int64(time.Second)

	// defaultScopeName is the InstrumentationScope.Name emitted in every export.
	defaultScopeName = "envoy-go"

	// defaultScopeVersion is the InstrumentationScope.Version (empty — not versioned yet).
	defaultScopeVersion = ""
)

// Exporter is the minimal span-sink interface consumed by the HCM filter; the
// concrete *OTLPExporter satisfies it. The noop exporter (returned when tracing
// is unconfigured) also satisfies it.
type Exporter interface {
	Export(span *Span)
	Close() error
}

// TracesClient is the exported span-export seam over *grpcclient.OTLPTracesClient
// (test-fakeable). *grpcclient.OTLPTracesClient satisfies it structurally, so
// internal/tracing never imports internal/grpcclient (no import cycle; main.go
// wiring passes the concrete client).
type TracesClient interface {
	Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error)
	Close() error
}

// OTLPExporter ships built-in OTLP Spans to an OpenTelemetry TraceService over
// a UNARY Export RPC. It mirrors OTLPAccessLogSink's bounded-channel + writer-
// goroutine + size/interval/close buffer + idempotent-Close shape.
type OTLPExporter struct {
	ch           chan *Span
	client       TracesClient
	serviceName  string
	scopeName    string
	scopeVersion string
	spansSent    *stats.Counter
	spansDropped *stats.Counter
	done         chan struct{}
	closeOnce    sync.Once
	closeErr     error
	lastDropLog  atomic.Int64

	bufferSizeBytes     int           // accumulated-serialized-byte flush threshold; 0 ⇒ flush-every-entry
	bufferFlushInterval time.Duration // flush-interval timer period (must be > 0)

	// ctx/cancel bound the lifetime of in-flight Export RPCs; Close cancels ctx
	// to unwedge a stalled Export (bounded network shutdown).
	ctx    context.Context
	cancel context.CancelFunc
}

// NewOTLPExporter builds an OTLPExporter over client with the default channel
// capacity (4096) and starts the writer goroutine. spansSent and spansDropped
// are the two stats counters held directly (a later task wires them from a
// TracerCounters struct).
func NewOTLPExporter(client TracesClient, serviceName string, spansSent, spansDropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration) *OTLPExporter {
	return newOTLPExporterWithCapacity(client, serviceName, spansSent, spansDropped, bufferSizeBytes, bufferFlushInterval, exporterDefaultChannelCapacity)
}

// newOTLPExporterWithCapacity is the test-friendly variant; production callers
// use NewOTLPExporter (capacity 4096).
func newOTLPExporterWithCapacity(client TracesClient, serviceName string, spansSent, spansDropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration, capacity int) *OTLPExporter {
	e := &OTLPExporter{
		ch:                  make(chan *Span, capacity),
		client:              client,
		serviceName:         serviceName,
		scopeName:           defaultScopeName,
		scopeVersion:        defaultScopeVersion,
		spansSent:           spansSent,
		spansDropped:        spansDropped,
		bufferSizeBytes:     bufferSizeBytes,
		bufferFlushInterval: bufferFlushInterval,
		done:                make(chan struct{}),
	}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	go e.run()
	return e
}

// Export non-blocking-sends span on the channel. On a full channel the span is
// dropped, spansDropped Inc'd, and at most one diagnostic emitted per second
// (the drop-newest idiom shared with the accesslog sinks).
func (e *OTLPExporter) Export(span *Span) {
	select {
	case e.ch <- span:
	default:
		// spansDropped counts channel-full (overflow) drops only; a batch lost
		// after the retry failure is logged in run(), not counted here.
		e.spansDropped.Inc()
		now := time.Now().UnixNano()
		last := e.lastDropLog.Load()
		if now-last >= exporterDropLogIntervalNanos && e.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("tracing: OTLP channel full, dropping span (service=%s)", e.serviceName)
		}
	}
}

// Close closes the channel, waits up to exporterCloseDrainGrace for the writer
// goroutine to drain the pending buffer (and flush it via Export), then cancels
// the Export context to guarantee bounded shutdown even if an Export is wedged
// on a stalled peer, and finally closes the underlying client. Idempotent and
// threadsafe via sync.Once.
func (e *OTLPExporter) Close() error {
	e.closeOnce.Do(func() {
		close(e.ch)
		select {
		case <-e.done:
		case <-time.After(exporterCloseDrainGrace):
			e.cancel() // abort a wedged Export so the writer can exit
			<-e.done
		}
		e.cancel() // release the context (idempotent; covers the normal path — satisfies vet lostcancel)
		e.closeErr = e.client.Close()
	})
	return e.closeErr
}

// run is the writer goroutine: drain channel-receives into a buffer and Export
// a BATCH on a size-OR-timer trigger, retrying the whole batch once on an
// Export error. Each Export is self-contained (no stream). The
// flush-every-entry behavior is the bufferSizeBytes==0 degenerate case (every
// span crosses sum >= 0 immediately).
func (e *OTLPExporter) run() {
	defer close(e.done)

	var buf []*tracepb.Span
	bufBytes := 0

	// flush Exports the accumulated batch as ONE ExportTraceServiceRequest. Up
	// to two attempts: the initial Export plus one retry of the WHOLE batch. On
	// success spansSent += len(buf) (batch-invariant); on a second consecutive
	// failure the batch is dropped (logged, not counted) so memory stays bounded
	// under a sustained outage. Empty buf is a no-op (the timer's idle tick).
	//
	// buf-reuse contract: on completion buf is truncated (buf[:0]) and its
	// backing array is reused by the next batch's append. The real unary Export
	// serializes the request synchronously before returning, so the bytes are
	// captured before reuse. The test fake retains the request pointer, so it
	// takes a defensive deep copy in its Export.
	flush := func() {
		if len(buf) == 0 {
			return
		}
		req := buildExportTraceRequest(buf, e.serviceName, e.scopeName, e.scopeVersion)
		for attempt := 0; attempt < 2; attempt++ {
			if _, err := e.client.Export(e.ctx, req); err == nil {
				e.spansSent.Add(uint64(len(buf)))
				break
			} else {
				log.Printf("tracing: OTLP export (service=%s, attempt=%d): %v", e.serviceName, attempt+1, err)
			}
		}
		// Reset the buffer whether the batch was sent or dropped — a dropped
		// batch (second-Export failure) is logged-not-counted, keeping memory
		// bounded.
		buf = buf[:0]
		bufBytes = 0
	}

	ticker := time.NewTicker(e.bufferFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case span, ok := <-e.ch:
			if !ok {
				flush() // drain the pending buffer on close
				return
			}
			sp := span.toProto()
			buf = append(buf, sp)
			if e.bufferSizeBytes > 0 { // skip the proto.Size walk when the accumulated size is never consulted
				bufBytes += proto.Size(sp)
			}
			if bufBytes >= e.bufferSizeBytes { // SIZE trigger; 0 ⇒ every span flushes
				flush()
			}
		case <-ticker.C:
			flush() // TIMER trigger (no-op if buf empty)
		}
	}
}

// ─────────────────────────── ExporterProvider ────────────────────────────────

// tracesClientDialer is the per-cluster dial seam. An implementation may check
// whether the named cluster exists and is H2-capable, returning an error for
// unknown or non-H2 clusters — this is the boot-reject gate
// (AMEND-TRACE-NO-BOOT-REJECT departure: the fatal is a returned error so the
// provider can surface it at ExporterFor time). In production, main.go supplies a
// concrete dialer backed by the cluster manager.
type tracesClientDialer interface {
	NewTracesClient(clusterName string) (TracesClient, error)
}

// ExporterProvider is the memoizing factory for tracing Exporter instances
// (D-TRACE-EXPORTER-WIRING / D-TRACE-ZIPKIN-TRANSPORT-WIRING). It dispatches on
// the parsed TracingConfig.Provider: ProviderOTel builds an *OTLPExporter over
// the dialer; ProviderZipkin builds a *ZipkinExporter over the injected
// ZipkinTransport behind a HasCluster boot-reject gate. One Exporter is built per
// unique (provider, cluster name) pair; repeated ExporterFor calls with the same
// config return the same pointer (pointer identity), while a same-key config with
// CONFLICTING settings (service_name / Zipkin knobs) is rejected with an error
// rather than silently handed an exporter built for a different config. The two
// counter rosters register lazily on the FIRST successful build of their kind
// (separate sync.Once guards) so a provider that never builds an exporter of a
// kind leaves that kind's registry surface unchanged.
type ExporterProvider struct {
	dialer   tracesClientDialer
	zt       ZipkinTransport
	reg      *stats.Registry
	once     sync.Once
	counters *TracerCounters
	bufBytes int
	bufFlush time.Duration

	zipkinOnce     sync.Once
	zipkinCounters *ZipkinCounters

	mu    sync.Mutex
	byKey map[exporterKey]exporterEntry
}

// exporterKey is the memoization key: the same cluster may legitimately back
// both an OTel and a Zipkin provider, and keying by cluster alone would silently
// hand the second config the first config's exporter — including an OTLP
// exporter for a Zipkin config (wrong wire format entirely).
type exporterKey struct {
	provider ProviderKind
	cluster  string
}

// exporterEntry pairs a memoized Exporter with the distinguishing settings of
// the config it was built from, so a later same-key config with conflicting
// settings is rejected instead of silently receiving a mismatched exporter.
type exporterEntry struct {
	exp      Exporter
	settings exporterSettings
}

// exporterSettings are the TracingConfig fields baked into a built Exporter
// beyond the memoize key (everything ExporterFor forwards to the constructors).
// Comparable so a conflicting re-request is a plain != check.
type exporterSettings struct {
	serviceName string
	zipkin      ZipkinSettings // zero value on the OTel arm
}

// settingsOf extracts the comparable exporterSettings from cfg.
func settingsOf(cfg *TracingConfig) exporterSettings {
	s := exporterSettings{serviceName: cfg.ServiceName}
	if cfg.Zipkin != nil {
		s.zipkin = *cfg.Zipkin
	}
	return s
}

// NewExporterProvider builds an ExporterProvider backed by dialer (the OTel arm)
// and zt (the Zipkin arm; nil-able — only the ProviderZipkin dispatch consults it,
// so a deployment with no Zipkin config boots cleanly with a nil transport).
// bufBytes and bufFlush are forwarded to each Exporter created by ExporterFor. reg
// receives the 2 TracerCounters (tracing.opentelemetry.*) lazily on the first
// successful OTel build and the 2 ZipkinCounters (tracing.zipkin.*) lazily on the
// first successful Zipkin build.
func NewExporterProvider(d tracesClientDialer, zt ZipkinTransport, reg *stats.Registry, bufBytes int, bufFlush time.Duration) *ExporterProvider {
	return &ExporterProvider{
		dialer:   d,
		zt:       zt,
		reg:      reg,
		bufBytes: bufBytes,
		bufFlush: bufFlush,
		byKey:    make(map[exporterKey]exporterEntry),
	}
}

// ExporterFor returns the Exporter for cfg, building it on the first call and
// dispatching on cfg.Provider. A second call with the same (provider, cluster)
// key and the same settings returns the SAME pointer (memoized); a same-key call
// whose remaining settings (service_name / Zipkin knobs) CONFLICT with the
// memoized build returns an error instead of a mismatched exporter.
//
// ProviderOTel: the dialer builds a TracesClient for the cluster (an error — unknown
// or non-H2 cluster — is the boot-reject, returned directly); the 2 tracing.opentelemetry.*
// counters register lazily on the first successful build.
//
// ProviderZipkin: a nil ZipkinTransport is a wiring error; an absent cluster
// (HasCluster == false) is the boot-reject; otherwise the 2 tracing.zipkin.* counters
// register lazily and a *ZipkinExporter is built over the transport. The boot-reject
// and nil-transport paths return (nil, error) WITHOUT registering counters or memoizing.
func (p *ExporterProvider) ExporterFor(cfg *TracingConfig) (Exporter, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := exporterKey{provider: cfg.Provider, cluster: cfg.ClusterName}
	settings := settingsOf(cfg)
	if ent, ok := p.byKey[key]; ok {
		if ent.settings != settings {
			return nil, fmt.Errorf("tracing: cluster %q already has an exporter built from a conflicting tracing config", cfg.ClusterName)
		}
		return ent.exp, nil
	}

	var e Exporter
	switch cfg.Provider {
	case ProviderOTel:
		client, err := p.dialer.NewTracesClient(cfg.ClusterName)
		if err != nil {
			return nil, err
		}
		p.once.Do(func() { p.counters = RegisterTracerCounters(p.reg) })
		e = NewOTLPExporter(client, cfg.ServiceName, p.counters.spansSent, p.counters.spansDropped, p.bufBytes, p.bufFlush)
	case ProviderZipkin:
		if p.zt == nil {
			return nil, fmt.Errorf("tracing: zipkin provider but no transport wired")
		}
		if !p.zt.HasCluster(cfg.ClusterName) {
			return nil, fmt.Errorf("tracing: zipkin: unknown cluster %q", cfg.ClusterName)
		}
		p.zipkinOnce.Do(func() { p.zipkinCounters = RegisterZipkinCounters(p.reg) })
		e = NewZipkinExporter(p.zt, cfg.ClusterName, cfg.Zipkin.CollectorEndpoint, cfg.Zipkin.CollectorHostname, cfg.Zipkin.TraceID128Bit, cfg.Zipkin.SharedSpanContext, p.zipkinCounters.spansSent, p.zipkinCounters.spansDropped, p.bufBytes, p.bufFlush)
	default:
		return nil, fmt.Errorf("tracing: unknown provider %v", cfg.Provider)
	}

	p.byKey[key] = exporterEntry{exp: e, settings: settings}
	return e, nil
}

// CloseAll closes every Exporter built by this provider and returns the first
// non-nil Close error. Idempotent: each Exporter.Close is sync.Once-protected, so
// a second CloseAll is a harmless no-op on each exporter (the underlying client
// Close is not called again).
func (p *ExporterProvider) CloseAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var firstErr error
	for _, ent := range p.byKey {
		if err := ent.exp.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// buildExportTraceRequest wraps an ALREADY-BUILT batch of proto Spans into one
// ExportTraceServiceRequest: one ResourceSpans (Resource = service.name attr)
// → one ScopeSpans (Scope = InstrumentationScope{Name, Version}) → the batch.
func buildExportTraceRequest(spans []*tracepb.Span, serviceName, scopeName, scopeVersion string) *coltracepb.ExportTraceServiceRequest {
	return &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					{
						Key:   "service.name",
						Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: serviceName}},
					},
				},
			},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{
					Name:    scopeName,
					Version: scopeVersion,
				},
				Spans: spans,
			}},
		}},
	}
}
