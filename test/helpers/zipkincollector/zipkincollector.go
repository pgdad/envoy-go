// Package zipkincollector provides a minimal in-process HTTP/JSON Zipkin v2
// collector for tests and differential fixtures. It is the HTTP analog of the
// otlptrace helper (the OTLP-trace gRPC receiver): a driver-owned receiver that
// the proxy DIALS, so per project convention it is a test helper rather than a
// runner BackendKind (D-TRACE-ZIPKIN-RECEIVER-WIRING; BackendKind STAYS 38).
//
// It accumulates every Zipkin v2 JSON span POSTed to <Addr()>/api/v2/spans
// across POSTs and exposes the same Count()/Spans()/Reset()/Addr()/Stop()
// surface as otlptrace, swapping the gRPC unary Export for a chunked HTTP POST
// of a JSON span array (the stdlib de-chunks the body transparently).
package zipkincollector

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
)

// ReceivedSpan mirrors the PUBLIC fields of internal/tracing.zipkinSpan (the
// Zipkin v2 JSON wire shape). It is defined here — a separate package — with
// matching json tags so a POSTed span array decodes field-for-field.
// Timestamp/Duration are microseconds. localEndpoint/annotations are not
// modeled (UNasserted by the differential).
type ReceivedSpan struct {
	TraceID   string            `json:"traceId"`
	ID        string            `json:"id"`
	ParentID  string            `json:"parentId,omitempty"`
	Name      string            `json:"name"`
	Kind      string            `json:"kind"`
	Timestamp int64             `json:"timestamp"`
	Duration  int64             `json:"duration"`
	Shared    bool              `json:"shared,omitempty"`
	Tags      map[string]string `json:"tags"`
}

// Collector is an in-process net/http server that accumulates every Zipkin v2
// JSON span POSTed to /api/v2/spans across POSTs. Goroutine-safe: the spans
// accumulator is guarded by a Mutex so the handler append path and the
// Count/Spans/Reset poll surface are race-free under -race
// (see TestConcurrentPOSTs_NoRace).
//
// Lifecycle (mirrors the otlptrace precedent):
//   - New() binds an ephemeral 127.0.0.1 listener BEFORE spawning the serve
//     goroutine, so Addr() is immediately usable. The caller reads Addr() to
//     templatize the Zipkin collector cluster endpoint into the fixtures.
//   - the handler io.ReadAlls the body (stdlib de-chunks transparently),
//     json.Unmarshals into []ReceivedSpan, appends under the mutex, and replies
//     202 Accepted (the reference collector contract).
//   - Count()/Spans() expose the accumulator: Count is the converge poll, Spans
//     is a defensive snapshot copy in arrival order.
//   - Reset() truncates the slice (per-side snapshot separation).
//   - Stop() closes the server; idempotent via sync.Once.
type Collector struct {
	srv *http.Server
	ln  net.Listener

	mu    sync.Mutex
	spans []ReceivedSpan

	stopOnce sync.Once
}

// New binds an ephemeral 127.0.0.1 listener and starts the serve goroutine,
// returning only once the listener is bound (so Addr() is immediately usable).
// Returns the collector + nil on success, or nil + a wrapped net.Listen error.
//
// Lifecycle management is the caller's responsibility: call Stop when done
// (e.g. via defer). Stop is idempotent.
func New() (*Collector, error) {
	return newCollector("127.0.0.1:0")
}

// NewAtAddr binds a listener on the caller-supplied `host:port` (e.g.
// "0.0.0.0:<preAllocatedPort>" so a Docker reference-Envoy can dial the host)
// and starts the serve goroutine. Returns the collector + nil on success, or
// nil + a wrapped net.Listen error.
//
// Lifecycle management is the caller's responsibility (no t.Cleanup) — the
// differential driver allocates a stable port BEFORE the reference container
// starts and binds 0.0.0.0:<port> so BOTH the reference (host.docker.internal)
// AND the subject (127.0.0.1) reach it. Mirrors otlptrace.NewAtAddr.
func NewAtAddr(addr string) (*Collector, error) {
	return newCollector(addr)
}

// newCollector is the shared constructor body used by both New() and
// NewAtAddr(addr). It binds a TCP listener on `addr`, constructs the Collector,
// registers the /api/v2/spans handler, and spawns srv.Serve(ln) in a background
// goroutine. Returns (nil, error) on net.Listen failure.
func newCollector(addr string) (*Collector, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	c := &Collector{ln: ln}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/spans", c.handleSpans)
	c.srv = &http.Server{Handler: mux}

	go func() {
		// http.Server.Serve returns ErrServerClosed on Close. Any earlier error
		// is discarded — the caller observes lifecycle via Stop.
		_ = c.srv.Serve(ln)
	}()

	return c, nil
}

// handleSpans is the /api/v2/spans POST handler: it io.ReadAlls the (possibly
// chunked) body, json.Unmarshals it into a []ReceivedSpan, appends under the
// mutex, and replies 202 Accepted. A non-POST method or a malformed body is
// rejected with a 4xx and skipped (graceful — must not panic).
func (c *Collector) handleSpans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	var spans []ReceivedSpan
	if err := json.Unmarshal(body, &spans); err != nil {
		http.Error(w, "decode spans: "+err.Error(), http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	c.spans = append(c.spans, spans...)
	c.mu.Unlock()

	w.WriteHeader(http.StatusAccepted)
}

// Spans returns a defensive snapshot copy of the accumulated spans in arrival
// order. The slice is freshly allocated so caller mutation cannot perturb the
// collector's accumulation.
func (c *Collector) Spans() []ReceivedSpan {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ReceivedSpan, len(c.spans))
	copy(out, c.spans)
	return out
}

// Count returns the number of accumulated spans. Load-bearing as the
// differential driver's converge poll — it spins on Count() reaching the
// expected total before reading Spans() to assert.
func (c *Collector) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.spans)
}

// Reset truncates the accumulated spans. Used when a single collector instance
// is reused across per-side snapshots.
//
// Call only when no POST is in flight (between per-side snapshots; collector
// quiescent); a concurrent in-flight append would land after the nil and
// survive the reset. It is mutex-guarded so memory-safe, but the quiescence
// contract is the caller's to uphold.
func (c *Collector) Reset() {
	c.mu.Lock()
	c.spans = nil
	c.mu.Unlock()
}

// Addr returns the listener's bound host:port string. Load-bearing — the
// fixture driver reads this back to templatize the Zipkin collector cluster
// endpoint into the bootstraps.
func (c *Collector) Addr() string {
	return c.ln.Addr().String()
}

// Stop closes the *http.Server (and the underlying listener). Idempotent via
// sync.Once: multiple calls are no-ops after the first.
func (c *Collector) Stop() {
	c.stopOnce.Do(func() {
		_ = c.srv.Close()
	})
}
