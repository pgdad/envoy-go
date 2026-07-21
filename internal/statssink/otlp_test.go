package statssink

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/pgdad/envoy-go/internal/grpcclient"
)

// Compile-time seam assertion (TEST-ONLY): the real grpcclient.OTLPMetricsClient
// satisfies the package-local otlpMetricsClient seam. This import lives in the
// test file ONLY so the PRODUCTION statssink package stays grpcclient-free
// (RD-SEAM; the cycle guard runs on production deps).
var _ otlpMetricsClient = (*grpcclient.OTLPMetricsClient)(nil)

// fakeOTLPMetricsClient captures the last ExportMetricsServiceRequest and drives
// the writer-goroutine tests. failNext leading Export calls return an error
// (retry test); gate (when non-nil) blocks Export so the bounded channel fills
// (drop test). Each successful Export signals exported.
type fakeOTLPMetricsClient struct {
	mu       sync.Mutex
	last     *colmetricspb.ExportMetricsServiceRequest
	calls    int
	failNext int
	closed   int
	gate     chan struct{}
	exported chan struct{}
}

func newFakeOTLP() *fakeOTLPMetricsClient {
	return &fakeOTLPMetricsClient{exported: make(chan struct{}, 64)}
}

func (f *fakeOTLPMetricsClient) Export(_ context.Context, req *colmetricspb.ExportMetricsServiceRequest) error {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	f.calls++
	if f.failNext > 0 {
		f.failNext--
		f.mu.Unlock()
		return errors.New("fake export failure")
	}
	f.last = req
	f.mu.Unlock()
	f.exported <- struct{}{}
	return nil
}

func (f *fakeOTLPMetricsClient) Close() error {
	f.mu.Lock()
	f.closed++
	f.mu.Unlock()
	return nil
}

func (f *fakeOTLPMetricsClient) received() *colmetricspb.ExportMetricsServiceRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

func (f *fakeOTLPMetricsClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeOTLPMetricsClient) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// counterFam / gaugeFam (delta_test.go) build a one-metric dto family with a
// dotted, tag-carrying name in the snapshot() shape (mapping.go:26-41).

// driveOnce Submits batch and blocks until the writer goroutine's Export lands,
// returning the captured request.
func driveOnce(t *testing.T, s *OTLPMetricsSink, f *fakeOTLPMetricsClient, batch []*dto.MetricFamily) *colmetricspb.ExportMetricsServiceRequest {
	t.Helper()
	s.Submit(batch)
	select {
	case <-f.exported:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Export")
	}
	return f.received()
}

func metricsOf(req *colmetricspb.ExportMetricsServiceRequest) []*metricspb.Metric {
	if req == nil || len(req.GetResourceMetrics()) == 0 {
		return nil
	}
	rm := req.GetResourceMetrics()[0]
	if len(rm.GetScopeMetrics()) == 0 {
		return nil
	}
	return rm.GetScopeMetrics()[0].GetMetrics()
}

func findMetric(ms []*metricspb.Metric, name string) *metricspb.Metric {
	for _, m := range ms {
		if m.GetName() == name {
			return m
		}
	}
	return nil
}

func attrValue(attrs []*commonpb.KeyValue, key string) (string, bool) {
	for _, a := range attrs {
		if a.GetKey() == key {
			return a.GetValue().GetStringValue(), true
		}
	}
	return "", false
}

// defaultSink: cumulative, both knobs TRUE (the parse nil→TRUE resolution), no prefix.
func defaultSink(t *testing.T, f *fakeOTLPMetricsClient) *OTLPMetricsSink {
	t.Helper()
	s := NewOTLPMetricsSink(f, "1.2.3", false, true, true, "")
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// Test 1 — structure: ONE ResourceMetrics / ONE ScopeMetrics / empty Scope; one Metric per family.
func TestOTLP_Structure(t *testing.T) {
	f := newFakeOTLP()
	s := defaultSink(t, f)
	req := driveOnce(t, s, f, []*dto.MetricFamily{
		counterFam("cluster.svc.upstream_rq_total", 5),
		gaugeFam("cluster.svc.membership_total", 3),
	})

	if got := len(req.GetResourceMetrics()); got != 1 {
		t.Errorf("ResourceMetrics count = %d, want 1", got)
	}
	rm := req.GetResourceMetrics()[0]
	if got := len(rm.GetScopeMetrics()); got != 1 {
		t.Errorf("ScopeMetrics count = %d, want 1", got)
	}
	sm := rm.GetScopeMetrics()[0]
	if name := sm.GetScope().GetName(); name != "" {
		t.Errorf("Scope.Name = %q, want empty", name)
	}
	if got := len(sm.GetMetrics()); got != 2 {
		t.Errorf("Metrics count = %d, want 2 (one per family)", got)
	}
}

// Test 2 — type/temporality: Counter → monotonic CUMULATIVE Sum; Gauge → Gauge.
func TestOTLP_TypeAndTemporality(t *testing.T) {
	f := newFakeOTLP()
	s := defaultSink(t, f)
	req := driveOnce(t, s, f, []*dto.MetricFamily{
		counterFam("cluster.svc.upstream_rq_total", 5),
		gaugeFam("cluster.svc.membership_total", 3),
	})
	ms := metricsOf(req)

	c := findMetric(ms, "cluster.upstream_rq_total")
	if c == nil || c.GetSum() == nil {
		t.Fatalf("counter metric: want a Sum, got %+v", c)
	}
	if !c.GetSum().GetIsMonotonic() {
		t.Errorf("counter Sum.IsMonotonic = false, want true")
	}
	if got := c.GetSum().GetAggregationTemporality(); got != metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE {
		t.Errorf("counter temporality = %v, want CUMULATIVE", got)
	}
	g := findMetric(ms, "cluster.membership_total")
	if g == nil || g.GetGauge() == nil {
		t.Errorf("gauge metric: want a Gauge, got %+v", g)
	}
	if g != nil && g.GetSum() != nil {
		t.Errorf("gauge metric must not carry a Sum")
	}
}

// Test 3 — TRUE-default tags: tag-extracted residual name AND envoy.<tag> attribute.
func TestOTLP_TrueDefaultTags(t *testing.T) {
	f := newFakeOTLP()
	s := defaultSink(t, f)
	req := driveOnce(t, s, f, []*dto.MetricFamily{counterFam("cluster.svc.upstream_rq_total", 5)})
	ms := metricsOf(req)

	c := findMetric(ms, "cluster.upstream_rq_total")
	if c == nil {
		t.Fatalf("want a metric named the residual %q, got names %v", "cluster.upstream_rq_total", metricNames(ms))
	}
	dps := c.GetSum().GetDataPoints()
	if len(dps) != 1 {
		t.Fatalf("datapoints = %d, want 1", len(dps))
	}
	v, ok := attrValue(dps[0].GetAttributes(), "envoy.cluster_name")
	if !ok {
		t.Fatalf("attribute envoy.cluster_name absent; attrs=%v", dps[0].GetAttributes())
	}
	if v != "svc" {
		t.Errorf("envoy.cluster_name = %q, want svc", v)
	}
}

func metricNames(ms []*metricspb.Metric) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.GetName())
	}
	return out
}

// Test 4 — resolved-bool knob matrix (nil→TRUE inversion lives at PARSE, not here).
func TestOTLP_KnobMatrix(t *testing.T) {
	full := "cluster.svc.upstream_rq_total"
	residual := "cluster.upstream_rq_total"

	t.Run("useTagExtractedName=false → full dotted name", func(t *testing.T) {
		f := newFakeOTLP()
		s := NewOTLPMetricsSink(f, "v", false, false, true, "")
		t.Cleanup(func() { _ = s.Close() })
		req := driveOnce(t, s, f, []*dto.MetricFamily{counterFam(full, 1)})
		if findMetric(metricsOf(req), full) == nil {
			t.Errorf("want full name %q; got %v", full, metricNames(metricsOf(req)))
		}
	})

	t.Run("emitTagsAsAttributes=false → no attributes", func(t *testing.T) {
		f := newFakeOTLP()
		s := NewOTLPMetricsSink(f, "v", false, true, false, "")
		t.Cleanup(func() { _ = s.Close() })
		req := driveOnce(t, s, f, []*dto.MetricFamily{counterFam(full, 1)})
		c := findMetric(metricsOf(req), residual)
		if c == nil {
			t.Fatalf("want residual %q; got %v", residual, metricNames(metricsOf(req)))
		}
		if n := len(c.GetSum().GetDataPoints()[0].GetAttributes()); n != 0 {
			t.Errorf("attributes = %d, want 0", n)
		}
	})

	t.Run("both-false → full name + no attrs (0113 shape)", func(t *testing.T) {
		f := newFakeOTLP()
		s := NewOTLPMetricsSink(f, "v", false, false, false, "")
		t.Cleanup(func() { _ = s.Close() })
		req := driveOnce(t, s, f, []*dto.MetricFamily{counterFam(full, 1)})
		c := findMetric(metricsOf(req), full)
		if c == nil {
			t.Fatalf("want full name %q; got %v", full, metricNames(metricsOf(req)))
		}
		if n := len(c.GetSum().GetDataPoints()[0].GetAttributes()); n != 0 {
			t.Errorf("attributes = %d, want 0", n)
		}
	})
}

// Test 5 — prefix composes with a dot; empty prefix has no leading dot.
func TestOTLP_Prefix(t *testing.T) {
	t.Run("prefix p → p.<residual>", func(t *testing.T) {
		f := newFakeOTLP()
		s := NewOTLPMetricsSink(f, "v", false, true, true, "p")
		t.Cleanup(func() { _ = s.Close() })
		req := driveOnce(t, s, f, []*dto.MetricFamily{counterFam("cluster.svc.upstream_rq_total", 1)})
		if findMetric(metricsOf(req), "p.cluster.upstream_rq_total") == nil {
			t.Errorf("want p.cluster.upstream_rq_total; got %v", metricNames(metricsOf(req)))
		}
	})

	t.Run("empty prefix → no leading dot", func(t *testing.T) {
		f := newFakeOTLP()
		s := defaultSink(t, f)
		req := driveOnce(t, s, f, []*dto.MetricFamily{counterFam("cluster.svc.upstream_rq_total", 1)})
		names := metricNames(metricsOf(req))
		for _, n := range names {
			if n == ".cluster.upstream_rq_total" || (len(n) > 0 && n[0] == '.') {
				t.Errorf("name %q has a leading dot", n)
			}
		}
		if findMetric(metricsOf(req), "cluster.upstream_rq_total") == nil {
			t.Errorf("want cluster.upstream_rq_total; got %v", names)
		}
	})
}

// Test 6 — report_counters_as_deltas: DELTA temporality; the SECOND flush of an
// unchanged cumulative counter is the per-window delta (0); gauge unaffected.
func TestOTLP_Delta(t *testing.T) {
	f := newFakeOTLP()
	s := NewOTLPMetricsSink(f, "v", true, true, true, "")
	t.Cleanup(func() { _ = s.Close() })

	batch := []*dto.MetricFamily{
		counterFam("cluster.svc.upstream_rq_total", 7),
		gaugeFam("cluster.svc.membership_total", 3),
	}
	// First flush: delta of first appearance == absolute (7).
	_ = driveOnce(t, s, f, batch)
	// Second flush: same cumulative value → per-window delta 0.
	req := driveOnce(t, s, f, []*dto.MetricFamily{
		counterFam("cluster.svc.upstream_rq_total", 7),
		gaugeFam("cluster.svc.membership_total", 3),
	})
	ms := metricsOf(req)

	c := findMetric(ms, "cluster.upstream_rq_total")
	if c == nil || c.GetSum() == nil {
		t.Fatalf("counter: want a Sum, got %+v", c)
	}
	if got := c.GetSum().GetAggregationTemporality(); got != metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA {
		t.Errorf("counter temporality = %v, want DELTA", got)
	}
	if !c.GetSum().GetIsMonotonic() {
		t.Errorf("delta Sum.IsMonotonic = false, want true (retained)")
	}
	if v := c.GetSum().GetDataPoints()[0].GetAsDouble(); v != 0 {
		t.Errorf("second-flush delta value = %v, want 0", v)
	}
	g := findMetric(ms, "cluster.membership_total")
	if g == nil || g.GetGauge() == nil {
		t.Errorf("gauge must be unaffected by delta mode, got %+v", g)
	}
}

// Test 7 — startTime: cumulative StartTime constant across flushes AND ns-magnitude;
// gauge datapoints carry NO startTime.
func TestOTLP_StartTime(t *testing.T) {
	f := newFakeOTLP()
	s := defaultSink(t, f)

	req1 := driveOnce(t, s, f, []*dto.MetricFamily{
		counterFam("cluster.svc.upstream_rq_total", 1),
		gaugeFam("cluster.svc.membership_total", 3),
	})
	req2 := driveOnce(t, s, f, []*dto.MetricFamily{counterFam("cluster.svc.upstream_rq_total", 2)})

	c1 := findMetric(metricsOf(req1), "cluster.upstream_rq_total")
	c2 := findMetric(metricsOf(req2), "cluster.upstream_rq_total")
	st1 := c1.GetSum().GetDataPoints()[0].GetStartTimeUnixNano()
	st2 := c2.GetSum().GetDataPoints()[0].GetStartTimeUnixNano()
	if st1 == 0 {
		t.Errorf("cumulative StartTimeUnixNano = 0, want a captured process-start ns")
	}
	if st1 != st2 {
		t.Errorf("cumulative StartTime not constant across flushes: %d vs %d", st1, st2)
	}
	// ns magnitude: same digit band as TimeUnixNano (> 1e18 for a 2020+ epoch).
	tt := c1.GetSum().GetDataPoints()[0].GetTimeUnixNano()
	if st1 < 1_000_000_000_000_000_000 || tt < 1_000_000_000_000_000_000 {
		t.Errorf("expected ns-magnitude times: start=%d time=%d", st1, tt)
	}
	g := findMetric(metricsOf(req1), "cluster.membership_total")
	if st := g.GetGauge().GetDataPoints()[0].GetStartTimeUnixNano(); st != 0 {
		t.Errorf("gauge StartTimeUnixNano = %d, want 0 (no start time on gauge)", st)
	}
}

// Test 8 — resource: the three telemetry.sdk.* keys with the constructor values.
func TestOTLP_Resource(t *testing.T) {
	f := newFakeOTLP()
	s := NewOTLPMetricsSink(f, "9.9.9", false, true, true, "")
	t.Cleanup(func() { _ = s.Close() })
	req := driveOnce(t, s, f, []*dto.MetricFamily{counterFam("cluster.svc.upstream_rq_total", 1)})

	attrs := req.GetResourceMetrics()[0].GetResource().GetAttributes()
	want := map[string]string{
		"telemetry.sdk.name":     "envoy-go",
		"telemetry.sdk.language": "go",
		"telemetry.sdk.version":  "9.9.9",
	}
	for k, wv := range want {
		v, ok := attrValue(attrs, k)
		if !ok {
			t.Errorf("resource attribute %q absent", k)
			continue
		}
		if v != wv {
			t.Errorf("resource %q = %q, want %q", k, v, wv)
		}
	}
}

// Test 9 — no-mutate: the shared input batch is byte-unchanged after Submit.
func TestOTLP_NoMutateInput(t *testing.T) {
	f := newFakeOTLP()
	s := defaultSink(t, f)

	fam := counterFam("cluster.svc.upstream_rq_total", 5)
	batch := []*dto.MetricFamily{fam}
	before := proto.Clone(fam).(*dto.MetricFamily)

	_ = driveOnce(t, s, f, batch)

	if fam.GetName() != before.GetName() {
		t.Errorf("input family Name mutated: %q → %q", before.GetName(), fam.GetName())
	}
	if !proto.Equal(fam, before) {
		t.Errorf("input family mutated by Submit:\n before=%v\n after =%v", before, fam)
	}
}

// ---- Step 5: writer robustness ----

// Drop: a full channel drops the newest flush (Submit does not block); the drop is
// LOGGED not counted (proven by the +0 registration guard).
func TestOTLP_ChannelFullDrops(t *testing.T) {
	f := newFakeOTLP()
	f.gate = make(chan struct{}) // block Export so the channel backs up
	s := NewOTLPMetricsSink(f, "v", false, true, true, "")
	t.Cleanup(func() {
		close(f.gate) // release the wedged writer so Close can drain
		_ = s.Close()
	})

	// Fill: one batch is taken by the writer (blocked in Export on the gate), then
	// defaultChannelCapacity more fill the buffered channel. Further Submits drop.
	done := make(chan struct{})
	go func() {
		for i := 0; i < defaultChannelCapacity+50; i++ {
			s.Submit([]*dto.MetricFamily{counterFam("cluster.svc.upstream_rq_total", float64(i))})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Submit blocked on a full channel; drop-newest not honored")
	}
}

// Retry-once: a single Export failure is retried and the batch is delivered.
func TestOTLP_RetryOnce(t *testing.T) {
	f := newFakeOTLP()
	f.failNext = 1
	s := NewOTLPMetricsSink(f, "v", false, true, true, "")
	t.Cleanup(func() { _ = s.Close() })

	req := driveOnce(t, s, f, []*dto.MetricFamily{counterFam("cluster.svc.upstream_rq_total", 1)})
	if req == nil {
		t.Fatal("retry did not deliver the request")
	}
	if f.callCount() != 2 {
		t.Errorf("Export calls = %d, want 2 (fail once + retry)", f.callCount())
	}
}

// Close idempotence: Close is safe to call twice and closes the client once.
func TestOTLP_CloseIdempotent(t *testing.T) {
	f := newFakeOTLP()
	s := NewOTLPMetricsSink(f, "v", false, true, true, "")
	if err := s.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if f.closeCount() != 1 {
		t.Errorf("client Close called %d times, want 1", f.closeCount())
	}
}
