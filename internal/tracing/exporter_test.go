package tracing

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/pgdad/envoy-go/internal/stats"
)

// fakeTracesClient is a fake otlpTracesClient seam. Export records every
// request (deep-cloned against buf-reuse) and can be made to error per a
// call-indexed queue. Mirrors the otlpsink_test.go fakeOTLPClient discipline.
type fakeTracesClient struct {
	mu         sync.Mutex
	exported   []*coltracepb.ExportTraceServiceRequest
	exportErrs []error
	exportIdx  int
	closeCount int

	// blockCh, when non-nil, blocks each Export on a receive (drop-newest test).
	blockCh chan struct{}
}

func (c *fakeTracesClient) Export(_ context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	if c.blockCh != nil {
		<-c.blockCh
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var err error
	if c.exportIdx < len(c.exportErrs) {
		err = c.exportErrs[c.exportIdx]
	}
	c.exportIdx++
	if err != nil {
		return nil, err
	}
	// Defensive deep copy: the exporter reuses the buf slice (buf = buf[:0]),
	// so without Clone a later batch's append corrupts this recorded request.
	cp := proto.Clone(req).(*coltracepb.ExportTraceServiceRequest)
	c.exported = append(c.exported, cp)
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func (c *fakeTracesClient) Close() error {
	c.mu.Lock()
	c.closeCount++
	c.mu.Unlock()
	return nil
}

func (c *fakeTracesClient) requests() []*coltracepb.ExportTraceServiceRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*coltracepb.ExportTraceServiceRequest, len(c.exported))
	copy(out, c.exported)
	return out
}

func (c *fakeTracesClient) closes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeCount
}

// countSpans sums Spans across all recorded Export requests.
func countSpans(reqs []*coltracepb.ExportTraceServiceRequest) int {
	n := 0
	for _, r := range reqs {
		for _, rs := range r.GetResourceSpans() {
			for _, ss := range rs.GetScopeSpans() {
				n += len(ss.GetSpans())
			}
		}
	}
	return n
}

// waitForSpans polls the fake until it has recorded >= want spans or the
// deadline elapses (never a bare sleep-then-assert).
func waitForSpans(t *testing.T, client *fakeTracesClient, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if countSpans(client.requests()) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d spans; got %d", want, countSpans(client.requests()))
}

// newTracerTestCounters mints a fresh pair of test counters from an isolated
// Registry (Registry panics on duplicate names; each sub-test needs its own).
func newTracerTestCounters(t *testing.T) (*stats.Counter, *stats.Counter) {
	t.Helper()
	reg := stats.NewRegistry()
	return reg.NewCounter("test.spans_sent"), reg.NewCounter("test.spans_dropped")
}

// testSpan returns a minimal valid Span for testing.
func testSpan() *Span {
	return &Span{
		Name:  "ingress",
		Kind:  tracepb.Span_SPAN_KIND_SERVER,
		Start: time.Unix(0, 1),
		End:   time.Unix(0, 100),
	}
}

// spanProtoSize returns the serialized byte size of one testSpan().toProto().
// The timestamps are pinned to small fixed values so the size stays constant.
func spanProtoSize() int {
	return proto.Size(testSpan().toProto())
}

// TestExporter_ExportAndClose submits K spans then Close — fake must have
// received exactly K spans (aggregated), spansSent == K.
func TestExporter_ExportAndClose(t *testing.T) {
	client := &fakeTracesClient{}
	sent, dropped := newTracerTestCounters(t)
	e := NewOTLPExporter(client, "my-service", sent, dropped, 0, time.Hour)

	const k = 5
	for i := 0; i < k; i++ {
		e.Export(testSpan())
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := countSpans(client.requests()); got != k {
		t.Errorf("total spans = %d, want %d", got, k)
	}
	if got := sent.Load(); got != k {
		t.Errorf("spansSent = %d, want %d", got, k)
	}
	if got := dropped.Load(); got != 0 {
		t.Errorf("spansDropped = %d, want 0", got)
	}
}

// TestExporter_SizeTrigger uses a tiny bufferSizeBytes so that a batch flushes
// mid-stream (before Close). Asserts the AGGREGATE span count across all
// Export calls (not per-call framing).
func TestExporter_SizeTrigger(t *testing.T) {
	client := &fakeTracesClient{}
	sent, dropped := newTracerTestCounters(t)
	// threshold = 2*spanProtoSize+1 ⇒ flush on the 3rd span of each batch.
	threshold := 2*spanProtoSize() + 1
	e := NewOTLPExporter(client, "my-service", sent, dropped, threshold, time.Hour)

	const n = 6
	for i := 0; i < n; i++ {
		e.Export(testSpan())
	}
	// The size trigger must fire BEFORE Close: two full batches of 3 ⇒ ≥2 Exports.
	waitForSpans(t, client, n, 5*time.Second)

	reqs := client.requests()
	if len(reqs) < 2 {
		t.Fatalf("got %d Export requests, want >= 2 (size trigger fired mid-life)", len(reqs))
	}
	if got := countSpans(reqs); got != n {
		t.Fatalf("total spans = %d, want %d", got, n)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := sent.Load(); got != n {
		t.Errorf("spansSent = %d, want %d (batch-invariant)", got, n)
	}
}

// TestExporter_IntervalTrigger uses a short bufferFlushInterval + huge size
// threshold so only the timer fires. POLL the fake (no sleep-then-assert).
func TestExporter_IntervalTrigger(t *testing.T) {
	client := &fakeTracesClient{}
	sent, dropped := newTracerTestCounters(t)
	e := NewOTLPExporter(client, "my-service", sent, dropped, 1<<30, 25*time.Millisecond)

	e.Export(testSpan())
	waitForSpans(t, client, 1, 5*time.Second)

	if got := countSpans(client.requests()); got != 1 {
		t.Fatalf("total spans = %d, want 1 (timer-flushed before Close)", got)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := sent.Load(); got != 1 {
		t.Errorf("spansSent = %d, want 1", got)
	}
	_ = dropped
}

// TestExporter_CloseDrainFlush proves the close-drain flush: huge size + long
// interval ⇒ neither trigger fires; only Close flushes the pending buffer,
// carrying all 3 spans in ONE Export.
func TestExporter_CloseDrainFlush(t *testing.T) {
	client := &fakeTracesClient{}
	sent, dropped := newTracerTestCounters(t)
	e := NewOTLPExporter(client, "my-service", sent, dropped, 1<<30, time.Hour)

	for i := 0; i < 3; i++ {
		e.Export(testSpan())
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d Export requests, want 1 (single close-drain flush)", len(reqs))
	}
	spans := reqs[0].GetResourceSpans()[0].GetScopeSpans()[0].GetSpans()
	if len(spans) != 3 {
		t.Errorf("batch spans = %d, want 3", len(spans))
	}
	if got := sent.Load(); got != 3 {
		t.Errorf("spansSent = %d, want 3", got)
	}
	_ = dropped
}

// TestExporter_RetryOnce makes the fake error on attempt 1 and succeed on 2.
// The batch must land (spansSent += len(batch)).
func TestExporter_RetryOnce(t *testing.T) {
	client := &fakeTracesClient{exportErrs: []error{errors.New("export boom")}}
	sent, dropped := newTracerTestCounters(t)
	e := NewOTLPExporter(client, "my-service", sent, dropped, 0, time.Hour)

	e.Export(testSpan())
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := countSpans(client.requests()); got != 1 {
		t.Fatalf("total spans = %d, want 1 (landed after retry)", got)
	}
	if got := sent.Load(); got != 1 {
		t.Errorf("spansSent = %d, want 1", got)
	}
	_ = dropped
}

// TestExporter_SecondFailureDropsBatch makes both attempts fail. The batch is
// dropped (logged-not-counted); spansSent stays 0; spansDropped stays 0 (the
// flush-path drop is logged, not channel-overflow-counted).
func TestExporter_SecondFailureDropsBatch(t *testing.T) {
	client := &fakeTracesClient{exportErrs: []error{errors.New("boom1"), errors.New("boom2")}}
	sent, dropped := newTracerTestCounters(t)
	e := NewOTLPExporter(client, "my-service", sent, dropped, 0, time.Hour)

	e.Export(testSpan())
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := countSpans(client.requests()); got != 0 {
		t.Errorf("got %d landed spans, want 0 (both attempts failed)", got)
	}
	if got := sent.Load(); got != 0 {
		t.Errorf("spansSent = %d, want 0 (dropped batch is logged-not-counted)", got)
	}
	if got := dropped.Load(); got != 0 {
		t.Errorf("spansDropped = %d, want 0 (flush-path drops are logged-not-counted)", got)
	}
}

// TestExporter_DropNewest fills the channel past capacity with a blocked fake.
// Overflow spans must increment spansDropped.
func TestExporter_DropNewest(t *testing.T) {
	block := make(chan struct{})
	client := &fakeTracesClient{blockCh: block}
	sent, dropped := newTracerTestCounters(t)
	e := newOTLPExporterWithCapacity(client, "my-service", sent, dropped, 0, time.Hour, 1)

	for i := 0; i < 100; i++ {
		e.Export(testSpan()) // must never block with the writer wedged in Export
	}
	if got := dropped.Load(); got == 0 {
		t.Errorf("expected at least one drop with a wedged writer; spansDropped = 0")
	}

	close(block) // release the writer so Close can drain
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_ = sent
}

// TestExporter_CloseIdempotent: two Close() calls — second is a no-op; client
// Close called exactly once.
func TestExporter_CloseIdempotent(t *testing.T) {
	client := &fakeTracesClient{}
	sent, dropped := newTracerTestCounters(t)
	e := NewOTLPExporter(client, "my-service", sent, dropped, 0, time.Hour)

	e.Export(testSpan())
	if err := e.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if got := client.closes(); got != 1 {
		t.Errorf("client.Close called %d times, want exactly 1", got)
	}
	_, _ = sent, dropped
}

// TestExporter_BuildRequestStructure asserts that a flush produces ONE
// ExportTraceServiceRequest with a ResourceSpans carrying service.name ==
// configured, and all spans under a single ScopeSpans.
func TestExporter_BuildRequestStructure(t *testing.T) {
	const svc = "my-service"
	client := &fakeTracesClient{}
	sent, dropped := newTracerTestCounters(t)
	e := NewOTLPExporter(client, svc, sent, dropped, 0, time.Hour)

	e.Export(testSpan())
	e.Export(testSpan())
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reqs := client.requests()
	// With bufferSizeBytes=0 each span flushes immediately — may be >1 request.
	// Assert the structure of the FIRST request.
	if len(reqs) == 0 {
		t.Fatalf("got 0 Export requests, want >= 1")
	}
	rss := reqs[0].GetResourceSpans()
	if len(rss) != 1 {
		t.Fatalf("ResourceSpans len = %d, want 1", len(rss))
	}
	// service.name attribute
	attrs := rss[0].GetResource().GetAttributes()
	if len(attrs) == 0 {
		t.Fatalf("Resource.Attributes is empty, want service.name")
	}
	gotSvcName := ""
	for _, kv := range attrs {
		if kv.GetKey() == "service.name" {
			gotSvcName = kv.GetValue().GetStringValue()
		}
	}
	if gotSvcName != svc {
		t.Errorf("service.name = %q, want %q", gotSvcName, svc)
	}
	// All spans under a single ScopeSpans
	scopeSpans := rss[0].GetScopeSpans()
	if len(scopeSpans) != 1 {
		t.Fatalf("ScopeSpans len = %d, want 1", len(scopeSpans))
	}
	if len(scopeSpans[0].GetSpans()) == 0 {
		t.Errorf("ScopeSpans[0].Spans is empty, want >= 1")
	}
	_ = dropped
}

// ──────────────────────────── ExporterProvider tests ─────────────────────────

// fakeDialer implements tracesClientDialer for TestExporterProvider.
// clients holds per-cluster fake clients; errs holds per-cluster dial errors.
// Unknown clusters return an errors.New sentinel.
type fakeDialer struct {
	mu      sync.Mutex
	clients map[string]*fakeTracesClient
	errs    map[string]error
}

func newFakeDialer(
	clients map[string]*fakeTracesClient,
	errs map[string]error,
) *fakeDialer {
	d := &fakeDialer{
		clients: make(map[string]*fakeTracesClient),
		errs:    make(map[string]error),
	}
	for k, v := range clients {
		d.clients[k] = v
	}
	for k, e := range errs {
		d.errs[k] = e
	}
	return d
}

func (d *fakeDialer) NewTracesClient(clusterName string) (TracesClient, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err, ok := d.errs[clusterName]; ok {
		return nil, err
	}
	if c, ok := d.clients[clusterName]; ok {
		return c, nil
	}
	return nil, errors.New("unknown cluster: " + clusterName)
}

// countReg counts the number of metrics registered in reg via Walk.
func countReg(reg *stats.Registry) int {
	n := 0
	reg.Walk(func(_ stats.Metric) { n++ })
	return n
}

// otelCfg / zkCfg are TracingConfig builders for the dispatch arms. The Zipkin
// arm reuses the fakeZipkinTransport seam from zipkin_test.go (its hasCluster
// toggle drives the boot-reject gate); Dispatch is not exercised here (no span
// is Export'd in the provider tests).
func otelCfg(cluster, svc string) *TracingConfig {
	return &TracingConfig{Provider: ProviderOTel, ClusterName: cluster, ServiceName: svc}
}

func zkCfg(cluster string) *TracingConfig {
	return &TracingConfig{
		Provider:    ProviderZipkin,
		ClusterName: cluster,
		Zipkin:      &ZipkinSettings{CollectorEndpoint: "/api/v2/spans"},
	}
}

// TestExporterProvider covers: per-cluster memoization (pointer identity), the
// boot-reject gate on a dialer error, lazy TracerCounter registration (sync.Once;
// +2 only on first successful build; none on error-only builds), and CloseAll
// idempotency (OTLPExporter.Close is sync.Once-protected so the underlying client
// Close is never called twice).  Each sub-test uses a FRESH stats.NewRegistry()
// to avoid duplicate-name panics from the static tracer-counter names.
func TestExporterProvider(t *testing.T) {
	t.Run("memoized_per_cluster", func(t *testing.T) {
		reg := stats.NewRegistry()
		fc := &fakeTracesClient{}
		d := newFakeDialer(map[string]*fakeTracesClient{"c": fc}, nil)
		p := NewExporterProvider(d, nil, reg, 0, time.Hour)

		e1, err := p.ExporterFor(otelCfg("c", "svc"))
		if err != nil {
			t.Fatalf("ExporterFor: %v", err)
		}
		if e1 == nil {
			t.Fatal("ExporterFor returned nil exporter")
		}
		e2, err := p.ExporterFor(otelCfg("c", "svc"))
		if err != nil {
			t.Fatalf("ExporterFor second: %v", err)
		}
		// Assert pointer identity: same cluster must return the SAME *OTLPExporter.
		if e1 != e2 {
			t.Error("ExporterFor not memoized: different Exporter returned for same cluster")
		}
		_ = p.CloseAll()
	})

	t.Run("boot_reject_on_dialer_error", func(t *testing.T) {
		reg := stats.NewRegistry()
		d := newFakeDialer(nil, map[string]error{"bad": errors.New("unknown cluster")})
		p := NewExporterProvider(d, nil, reg, 0, time.Hour)

		got, err := p.ExporterFor(otelCfg("bad", "svc"))
		if err == nil {
			t.Fatal("expected error from dial failure, got nil")
		}
		if got != nil {
			t.Fatalf("expected nil exporter on dial error, got %v", got)
		}
	})

	t.Run("lazy_counter_registration", func(t *testing.T) {
		reg := stats.NewRegistry()
		fc := &fakeTracesClient{}
		d := newFakeDialer(
			map[string]*fakeTracesClient{"c": fc},
			map[string]error{"fail": errors.New("dial error")},
		)
		p := NewExporterProvider(d, nil, reg, 0, time.Hour)

		before := countReg(reg) // fresh registry: should be 0

		// Error path: no counters registered (delta stays 0).
		if _, err := p.ExporterFor(otelCfg("fail", "svc")); err == nil {
			t.Fatal("expected error for 'fail' cluster")
		}
		if delta := countReg(reg) - before; delta != 0 {
			t.Errorf("counter delta before any successful build = %d, want 0", delta)
		}

		// First successful build: +2 counters (spans_sent + spans_dropped).
		if _, err := p.ExporterFor(otelCfg("c", "svc")); err != nil {
			t.Fatalf("ExporterFor c: %v", err)
		}
		if delta := countReg(reg) - before; delta != 2 {
			t.Errorf("counter delta after first successful build = %d, want 2", delta)
		}

		// Memoized second call: sync.Once already fired; no new counters.
		if _, err := p.ExporterFor(otelCfg("c", "svc")); err != nil {
			t.Fatalf("ExporterFor c (memoized): %v", err)
		}
		if delta := countReg(reg) - before; delta != 2 {
			t.Errorf("counter delta after memoized call = %d, want still 2", delta)
		}

		_ = p.CloseAll()
	})

	t.Run("close_all_closes_exporters_idempotent", func(t *testing.T) {
		reg := stats.NewRegistry()
		fc1 := &fakeTracesClient{}
		fc2 := &fakeTracesClient{}
		d := newFakeDialer(map[string]*fakeTracesClient{"c1": fc1, "c2": fc2}, nil)
		p := NewExporterProvider(d, nil, reg, 0, time.Hour)

		if _, err := p.ExporterFor(otelCfg("c1", "svc")); err != nil {
			t.Fatalf("ExporterFor c1: %v", err)
		}
		if _, err := p.ExporterFor(otelCfg("c2", "svc")); err != nil {
			t.Fatalf("ExporterFor c2: %v", err)
		}

		// First CloseAll: drains + closes every built exporter.
		if err := p.CloseAll(); err != nil {
			t.Errorf("CloseAll: %v", err)
		}
		if got := fc1.closes(); got != 1 {
			t.Errorf("fc1.closes after first CloseAll = %d, want 1", got)
		}
		if got := fc2.closes(); got != 1 {
			t.Errorf("fc2.closes after first CloseAll = %d, want 1", got)
		}

		// Second CloseAll: idempotent — OTLPExporter.Close is sync.Once-protected;
		// the underlying fakeTracesClient.Close is NOT called again.
		if err := p.CloseAll(); err != nil {
			t.Errorf("second CloseAll: %v", err)
		}
		if got := fc1.closes(); got != 1 {
			t.Errorf("fc1.closes after second CloseAll = %d, want still 1", got)
		}
		if got := fc2.closes(); got != 1 {
			t.Errorf("fc2.closes after second CloseAll = %d, want still 1", got)
		}
	})

	t.Run("same_cluster_different_provider_gets_own_exporter", func(t *testing.T) {
		// The memoize key is (provider, cluster): a Zipkin config naming the SAME
		// cluster as an earlier OTel config must get its own *ZipkinExporter, not
		// the memoized *OTLPExporter (wrong wire format entirely).
		reg := stats.NewRegistry()
		fc := &fakeTracesClient{}
		d := newFakeDialer(map[string]*fakeTracesClient{"shared": fc}, nil)
		zt := &fakeZipkinTransport{hasCluster: true}
		p := NewExporterProvider(d, zt, reg, 0, time.Hour)

		e1, err := p.ExporterFor(otelCfg("shared", ""))
		if err != nil {
			t.Fatalf("ExporterFor otel: %v", err)
		}
		if _, ok := e1.(*OTLPExporter); !ok {
			t.Fatalf("otel arm returned %T, want *OTLPExporter", e1)
		}
		zk := zkCfg("shared")
		e2, err := p.ExporterFor(zk)
		if err != nil {
			t.Fatalf("ExporterFor zipkin (same cluster): %v", err)
		}
		if _, ok := e2.(*ZipkinExporter); !ok {
			t.Fatalf("zipkin arm returned %T, want *ZipkinExporter (got the memoized OTel exporter?)", e2)
		}
		// Each arm stays independently memoized.
		if e3, err := p.ExporterFor(otelCfg("shared", "")); err != nil || e3 != e1 {
			t.Errorf("otel re-request = (%v, %v), want the memoized (%v, nil)", e3, err, e1)
		}
		if e4, err := p.ExporterFor(zk); err != nil || e4 != e2 {
			t.Errorf("zipkin re-request = (%v, %v), want the memoized (%v, nil)", e4, err, e2)
		}
		_ = p.CloseAll()
	})

	t.Run("conflicting_settings_same_key_rejected", func(t *testing.T) {
		reg := stats.NewRegistry()
		fc := &fakeTracesClient{}
		d := newFakeDialer(map[string]*fakeTracesClient{"c": fc}, nil)
		zt := &fakeZipkinTransport{hasCluster: true}
		p := NewExporterProvider(d, zt, reg, 0, time.Hour)

		// OTel: a second config with the same cluster but a different
		// service_name must error, not silently reuse the first exporter.
		if _, err := p.ExporterFor(otelCfg("c", "svc-a")); err != nil {
			t.Fatalf("ExporterFor otel: %v", err)
		}
		if _, err := p.ExporterFor(otelCfg("c", "svc-b")); err == nil {
			t.Error("expected conflict error for same-cluster different service_name, got nil")
		}

		// Zipkin: same cluster, different collector endpoint must error too.
		if _, err := p.ExporterFor(zkCfg("zk")); err != nil {
			t.Fatalf("ExporterFor zipkin: %v", err)
		}
		conflicting := zkCfg("zk")
		conflicting.Zipkin.CollectorEndpoint = "/api/v2/other"
		if _, err := p.ExporterFor(conflicting); err == nil {
			t.Error("expected conflict error for same-cluster different zipkin endpoint, got nil")
		}
		_ = p.CloseAll()
	})

	// ─────────────────── Zipkin dispatch arm ───────────────────

	t.Run("zipkin_memoized_per_cluster", func(t *testing.T) {
		reg := stats.NewRegistry()
		zt := &fakeZipkinTransport{hasCluster: true}
		p := NewExporterProvider(nil, zt, reg, 0, time.Hour)

		e1, err := p.ExporterFor(zkCfg("zk"))
		if err != nil {
			t.Fatalf("ExporterFor zipkin: %v", err)
		}
		if e1 == nil {
			t.Fatal("ExporterFor returned nil zipkin exporter")
		}
		if _, ok := e1.(*ZipkinExporter); !ok {
			t.Fatalf("ExporterFor zipkin returned %T, want *ZipkinExporter", e1)
		}
		e2, err := p.ExporterFor(zkCfg("zk"))
		if err != nil {
			t.Fatalf("ExporterFor zipkin second: %v", err)
		}
		if e1 != e2 {
			t.Error("ExporterFor not memoized: different Exporter for same zipkin cluster")
		}
		_ = p.CloseAll()
	})

	t.Run("zipkin_boot_reject_unknown_cluster", func(t *testing.T) {
		reg := stats.NewRegistry()
		zt := &fakeZipkinTransport{hasCluster: false} // HasCluster == false
		p := NewExporterProvider(nil, zt, reg, 0, time.Hour)

		got, err := p.ExporterFor(zkCfg("zk"))
		if err == nil {
			t.Fatal("expected boot-reject error for unknown zipkin cluster, got nil")
		}
		if got != nil {
			t.Fatalf("expected nil exporter on boot-reject, got %v", got)
		}
		if !strings.Contains(err.Error(), "unknown cluster") {
			t.Errorf("boot-reject error %q, want it to contain %q", err.Error(), "unknown cluster")
		}
		// Boot-reject must NOT register counters or memoize.
		if got := countReg(reg); got != 0 {
			t.Errorf("counters registered on boot-reject = %d, want 0", got)
		}
	})

	t.Run("zipkin_nil_transport_errors", func(t *testing.T) {
		reg := stats.NewRegistry()
		p := NewExporterProvider(nil, nil, reg, 0, time.Hour) // no zipkin transport wired

		got, err := p.ExporterFor(zkCfg("zk"))
		if err == nil {
			t.Fatal("expected error for zipkin provider with no transport, got nil")
		}
		if got != nil {
			t.Fatalf("expected nil exporter with no transport, got %v", got)
		}
	})

	t.Run("zipkin_lazy_counter_registration", func(t *testing.T) {
		reg := stats.NewRegistry()
		zt := &fakeZipkinTransport{hasCluster: true}
		p := NewExporterProvider(nil, zt, reg, 0, time.Hour)

		before := countReg(reg) // fresh registry: 0
		if before != 0 {
			t.Fatalf("fresh registry surface = %d, want 0", before)
		}

		// First successful zipkin build: +2 tracing.zipkin.* counters.
		if _, err := p.ExporterFor(zkCfg("zk")); err != nil {
			t.Fatalf("ExporterFor zk: %v", err)
		}
		if delta := countReg(reg) - before; delta != 2 {
			t.Errorf("zipkin counter delta after first successful build = %d, want 2", delta)
		}
		// Memoized second call: no new counters.
		if _, err := p.ExporterFor(zkCfg("zk")); err != nil {
			t.Fatalf("ExporterFor zk (memoized): %v", err)
		}
		if delta := countReg(reg) - before; delta != 2 {
			t.Errorf("zipkin counter delta after memoized call = %d, want still 2", delta)
		}
		_ = p.CloseAll()
	})

	t.Run("zipkin_never_built_leaves_surface_unmoved", func(t *testing.T) {
		reg := stats.NewRegistry()
		fc := &fakeTracesClient{}
		d := newFakeDialer(map[string]*fakeTracesClient{"c": fc}, nil)
		zt := &fakeZipkinTransport{hasCluster: true}
		p := NewExporterProvider(d, zt, reg, 0, time.Hour)

		// Build only an OTel exporter: tracing.zipkin.* must NOT register.
		if _, err := p.ExporterFor(otelCfg("c", "svc")); err != nil {
			t.Fatalf("ExporterFor otel: %v", err)
		}
		// Surface should be exactly the 2 tracing.opentelemetry.* counters.
		if got := countReg(reg); got != 2 {
			t.Errorf("registry surface = %d, want 2 (otel only; no zipkin)", got)
		}
		_ = p.CloseAll()
	})

	t.Run("close_all_mixed_otel_and_zipkin", func(t *testing.T) {
		reg := stats.NewRegistry()
		fc := &fakeTracesClient{}
		d := newFakeDialer(map[string]*fakeTracesClient{"c": fc}, nil)
		zt := &fakeZipkinTransport{hasCluster: true}
		p := NewExporterProvider(d, zt, reg, 0, time.Hour)

		if _, err := p.ExporterFor(otelCfg("c", "svc")); err != nil {
			t.Fatalf("ExporterFor otel: %v", err)
		}
		if _, err := p.ExporterFor(zkCfg("zk")); err != nil {
			t.Fatalf("ExporterFor zipkin: %v", err)
		}

		// First CloseAll closes both; the OTel client's Close is called once.
		if err := p.CloseAll(); err != nil {
			t.Errorf("CloseAll: %v", err)
		}
		if got := fc.closes(); got != 1 {
			t.Errorf("otel client closes after first CloseAll = %d, want 1", got)
		}
		// Idempotent: second CloseAll is a harmless no-op.
		if err := p.CloseAll(); err != nil {
			t.Errorf("second CloseAll: %v", err)
		}
		if got := fc.closes(); got != 1 {
			t.Errorf("otel client closes after second CloseAll = %d, want still 1", got)
		}
	})
}

// TestExporter_buildExportTraceRequest unit-tests the request builder directly:
// one ResourceSpans, correct service.name, correct ScopeSpans with all provided spans.
func TestExporter_buildExportTraceRequest(t *testing.T) {
	const svc, scope, ver = "svc", "envoy-go", ""
	spans := []*tracepb.Span{testSpan().toProto(), testSpan().toProto()}
	req := buildExportTraceRequest(spans, svc, scope, ver)

	if len(req.GetResourceSpans()) != 1 {
		t.Fatalf("ResourceSpans = %d, want 1", len(req.GetResourceSpans()))
	}
	rs := req.GetResourceSpans()[0]

	// service.name
	found := false
	for _, kv := range rs.GetResource().GetAttributes() {
		if kv.GetKey() == "service.name" && kv.GetValue().GetStringValue() == svc {
			found = true
		}
	}
	if !found {
		t.Errorf("service.name=%q not found in Resource.Attributes %v", svc, rs.GetResource().GetAttributes())
	}

	// ScopeSpans
	if len(rs.GetScopeSpans()) != 1 {
		t.Fatalf("ScopeSpans = %d, want 1", len(rs.GetScopeSpans()))
	}
	ss := rs.GetScopeSpans()[0]
	if ss.GetScope().GetName() != scope {
		t.Errorf("Scope.Name = %q, want %q", ss.GetScope().GetName(), scope)
	}
	if len(ss.GetSpans()) != len(spans) {
		t.Errorf("ScopeSpans[0].Spans = %d, want %d", len(ss.GetSpans()), len(spans))
	}
}
