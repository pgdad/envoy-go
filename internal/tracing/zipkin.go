package tracing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

// zipkinEndpoint is the Zipkin v2 endpoint object. Only serviceName is modeled;
// the host/port fields are env-specific and omitted (UNasserted). When serviceName
// is empty the whole localEndpoint is omitted by the parent's omitempty pointer.
type zipkinEndpoint struct {
	ServiceName string `json:"serviceName,omitempty"`
}

// zipkinSpan is the Zipkin v2 JSON span model (the wire analog of the OTLP
// tracepb.Span). Timestamp/Duration are microseconds. Annotations are intentionally
// omitted (D-TRACE-ZIPKIN-ANNOTATIONS).
type zipkinSpan struct {
	TraceID       string            `json:"traceId"`
	ID            string            `json:"id"`
	ParentID      string            `json:"parentId,omitempty"`
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	Timestamp     int64             `json:"timestamp"`
	Duration      int64             `json:"duration"`
	Shared        bool              `json:"shared,omitempty"`
	LocalEndpoint *zipkinEndpoint   `json:"localEndpoint,omitempty"`
	Tags          map[string]string `json:"tags"`
}

// identityInput carries the three trace-identity fields shared by a Decision and a
// Span so that zipkinIdentity can derive the outbound trace/span/parent ids for both
// the B3 injector and the Zipkin span encoder from a single code path.
type identityInput struct {
	TraceID      [16]byte
	SpanID       [8]byte
	ParentSpanID [8]byte
}

// zipkinIdentity derives the outbound (traceID, id, parentID) hex strings and the
// emitShared flag for a span/decision under the id128 and shared switches:
//   - fresh root (ParentSpanID zero): id = hex(SpanID), no parent, not shared.
//   - continued + shared: the server REUSES the incoming span-id as its own id
//     (id = hex(ParentSpanID)), with no parentId and shared=true.
//   - continued + not shared: a fresh id = hex(SpanID) with parentId = hex(ParentSpanID).
//
// traceID is the full 32-hex 128-bit id when id128, else the low-64 16-hex id. Hex is
// lowercase (D-TRACE-ZIPKIN-IDS/SHARED).
func zipkinIdentity(d identityInput, id128, shared bool) (traceID, id, parentID string, emitShared bool) {
	traceID = traceIDHex(d.TraceID, id128)
	switch {
	case d.ParentSpanID == ([8]byte{}):
		id = spanIDHex(d.SpanID) // fresh root, no parent
	case shared:
		id = spanIDHex(d.ParentSpanID) // reuse the incoming span-id, no parent
		emitShared = true
	default:
		id = spanIDHex(d.SpanID)
		parentID = spanIDHex(d.ParentSpanID)
	}
	return traceID, id, parentID, emitShared
}

// encodeZipkinSpans maps the spans to the Zipkin v2 JSON model and marshals them as a
// JSON array. The span name is the request :authority (Span.Authority), kind is
// SERVER, timing is in microseconds, the trace identity is derived via zipkinIdentity,
// and the tags are the span attributes with node_id/zone dropped (the 16-attr roster
// minus the two Zipkin omits — D-TRACE-ZIPKIN-WIRE).
func encodeZipkinSpans(spans []*Span, id128, shared bool) ([]byte, error) {
	out := make([]zipkinSpan, 0, len(spans))
	for _, s := range spans {
		traceID, id, parentID, emitShared := zipkinIdentity(identityInput{
			TraceID:      s.TraceID,
			SpanID:       s.SpanID,
			ParentSpanID: s.ParentSpanID,
		}, id128, shared)

		tags := make(map[string]string, len(s.Attrs))
		for _, kv := range s.Attrs {
			if kv.Key == "node_id" || kv.Key == "zone" {
				continue // Zipkin drops these two (14 tags from the 16-attr roster)
			}
			tags[kv.Key] = kv.Str
		}

		zs := zipkinSpan{
			TraceID:   traceID,
			ID:        id,
			ParentID:  parentID,
			Name:      s.Authority,
			Kind:      "SERVER",
			Timestamp: s.Start.UnixMicro(),
			Duration:  s.End.Sub(s.Start).Microseconds(),
			Tags:      tags,
		}
		if emitShared {
			zs.Shared = true
		}
		out = append(out, zs)
	}
	return json.Marshal(out)
}

// ─────────────────────────── ZipkinExporter ──────────────────────────────────

// ZipkinTransport is the test-fakeable cluster-dispatch seam. *httpclient.Client
// satisfies HasCluster/Dispatch structurally (the Dispatch adapter binds the
// cluster manager), so internal/tracing never imports internal/httpclient (no
// import cycle); main.go wiring passes the concrete adapter.
type ZipkinTransport interface {
	HasCluster(name string) bool
	Dispatch(ctx context.Context, clusterName string, req *http.Request) (*http.Response, error)
}

// ZipkinExporter ships built-in Spans to a Zipkin v2 collector as a JSON array
// POSTed over an httpclient.ClusterDispatch transport. It is the second Exporter
// impl behind the tracing.Exporter seam, mirroring OTLPExporter's bounded-channel
// + writer-goroutine + size/interval/close-buffer + retry-once + drop-newest +
// idempotent-Close shape (exporter.go), swapping the gRPC unary Export for a
// v2-JSON HTTP POST over the ZipkinTransport seam
// (D-TRACE-ZIPKIN-WIRE / D-TRACE-ZIPKIN-TRANSPORT-WIRING).
type ZipkinExporter struct {
	ch           chan *Span
	transport    ZipkinTransport
	clusterName  string
	endpoint     string
	hostname     string
	id128        bool
	shared       bool
	spansSent    *stats.Counter
	spansDropped *stats.Counter
	done         chan struct{}
	closeOnce    sync.Once
	closeErr     error
	lastDropLog  atomic.Int64

	bufferSizeBytes     int           // accumulated-estimated-byte flush threshold; 0 ⇒ flush-every-entry
	bufferFlushInterval time.Duration // flush-interval timer period (must be > 0)

	// ctx/cancel bound the lifetime of in-flight Dispatch POSTs; Close cancels
	// ctx to unwedge a stalled POST (bounded network shutdown).
	ctx    context.Context
	cancel context.CancelFunc
}

// NewZipkinExporter builds a ZipkinExporter over transport with the default
// channel capacity (4096) and starts the writer goroutine. endpoint is the
// collector path (e.g. /api/v2/spans); hostname is the Host-header value (cluster
// name when empty). id128/shared are the Zipkin encoder switches. spansSent and
// spansDropped are the two tracing.zipkin.* counters held directly.
func NewZipkinExporter(transport ZipkinTransport, clusterName, endpoint, hostname string, id128, shared bool, spansSent, spansDropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration) *ZipkinExporter {
	return newZipkinExporterWithCapacity(transport, clusterName, endpoint, hostname, id128, shared, spansSent, spansDropped, bufferSizeBytes, bufferFlushInterval, exporterDefaultChannelCapacity)
}

// newZipkinExporterWithCapacity is the test-friendly variant; production callers
// use NewZipkinExporter (capacity 4096).
func newZipkinExporterWithCapacity(transport ZipkinTransport, clusterName, endpoint, hostname string, id128, shared bool, spansSent, spansDropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration, capacity int) *ZipkinExporter {
	e := &ZipkinExporter{
		ch:                  make(chan *Span, capacity),
		transport:           transport,
		clusterName:         clusterName,
		endpoint:            endpoint,
		hostname:            hostname,
		id128:               id128,
		shared:              shared,
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
// dropped, spansDropped Inc'd, and at most one diagnostic emitted per second (the
// drop-newest idiom shared with OTLPExporter / the accesslog sinks).
func (e *ZipkinExporter) Export(span *Span) {
	select {
	case e.ch <- span:
	default:
		e.spansDropped.Inc()
		now := time.Now().UnixNano()
		last := e.lastDropLog.Load()
		if now-last >= exporterDropLogIntervalNanos && e.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("tracing: Zipkin channel full, dropping span (cluster=%s)", e.clusterName)
		}
	}
}

// Close closes the channel, waits up to exporterCloseDrainGrace for the writer to
// drain the pending buffer (and flush it), then cancels the dispatch context to
// guarantee bounded shutdown even if a POST is wedged on a stalled collector. The
// ZipkinTransport is shared/stateless so there is no underlying client to close.
// Idempotent and threadsafe via sync.Once.
func (e *ZipkinExporter) Close() error {
	e.closeOnce.Do(func() {
		close(e.ch)
		select {
		case <-e.done:
		case <-time.After(exporterCloseDrainGrace):
			e.cancel() // abort a wedged POST so the writer can exit
			<-e.done
		}
		e.cancel()       // release the context (idempotent; covers the normal path)
		e.closeErr = nil // the transport is shared/stateless — nothing to close
	})
	return e.closeErr
}

// run is the writer goroutine: drain channel-receives into a buffer of *Span and
// POST a BATCH on a size-OR-timer trigger, retrying the whole batch once on a
// non-2xx/error. The flush-every-entry behavior is the bufferSizeBytes==0
// degenerate case (every span crosses sum >= 0 immediately).
func (e *ZipkinExporter) run() {
	defer close(e.done)

	var buf []*Span
	bufBytes := 0

	// flush encodes the accumulated batch as ONE v2-JSON array and POSTs it. Up
	// to two attempts: the initial POST plus one retry of the WHOLE batch. The
	// request is rebuilt INSIDE the attempt loop so the retry re-sends the bytes
	// (a consumed bytes.Reader would re-POST an empty body). On success spansSent
	// += len(buf) (batch-invariant); on a second consecutive failure the batch is
	// dropped (logged, not counted) so memory stays bounded. Empty buf is a no-op.
	flush := func() {
		if len(buf) == 0 {
			return
		}
		body, err := encodeZipkinSpans(buf, e.id128, e.shared)
		if err != nil {
			log.Printf("tracing: Zipkin encode (cluster=%s): %v", e.clusterName, err)
			buf = buf[:0]
			bufBytes = 0
			return
		}
		host := e.hostname
		if host == "" {
			host = e.clusterName
		}
		for attempt := 0; attempt < 2; attempt++ {
			// Rebuild the request per attempt: bytes.NewReader is consumed after
			// the first read, so the retry needs a fresh body reader.
			req, rerr := http.NewRequestWithContext(e.ctx, http.MethodPost, "http://"+host+e.endpoint, bytes.NewReader(body))
			if rerr != nil {
				log.Printf("tracing: Zipkin build-request (cluster=%s): %v", e.clusterName, rerr)
				break
			}
			req.Host = host // the placeholder host carries the Host header; ClusterDispatch rewrites URL.Host
			req.Header.Set("Content-Type", "application/json")

			resp, derr := e.transport.Dispatch(e.ctx, e.clusterName, req)
			if derr == nil && resp != nil && resp.StatusCode/100 == 2 {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				e.spansSent.Add(uint64(len(buf)))
				break
			}
			// Failure: drain/close any non-nil response body and retry.
			status := 0
			if resp != nil {
				status = resp.StatusCode
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
			log.Printf("tracing: Zipkin POST (cluster=%s, attempt=%d, status=%d): %v", e.clusterName, attempt+1, status, derr)
		}
		// Reset the buffer whether sent or dropped — a dropped batch is
		// logged-not-counted, keeping memory bounded under a sustained outage.
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
			buf = append(buf, span)
			// Cheap per-span size estimate (do NOT encode per-span — the whole
			// batch is encoded ONCE at flush).
			bufBytes += len(span.Authority) + len(span.Attrs)*32
			if bufBytes >= e.bufferSizeBytes { // SIZE trigger; 0 ⇒ every span flushes
				flush()
			}
		case <-ticker.C:
			flush() // TIMER trigger (no-op if buf empty)
		}
	}
}
