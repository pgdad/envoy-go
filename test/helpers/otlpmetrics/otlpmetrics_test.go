package otlpmetrics

import (
	"context"
	"testing"
	"time"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// dialTestClient opens a plaintext h2c grpc.ClientConn to the supplied address,
// registers it for teardown via t.Cleanup, and returns the MetricsServiceClient.
func dialTestClient(t *testing.T, addr string) colmetricspb.MetricsServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q): %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return colmetricspb.NewMetricsServiceClient(conn)
}

// strKV builds a StringValue KeyValue (the T2 sink's otlpKV shape).
func strKV(k, v string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
}

// mkSumMetric builds a Metric_Sum with a single NumberDataPoint carrying attrs.
func mkSumMetric(name string, temporality metricspb.AggregationTemporality, isMonotonic bool, startTime, timeUnixNano uint64, value float64, attrs ...*commonpb.KeyValue) *metricspb.Metric {
	return &metricspb.Metric{
		Name: name,
		Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
			DataPoints: []*metricspb.NumberDataPoint{{
				Attributes:        attrs,
				StartTimeUnixNano: startTime,
				TimeUnixNano:      timeUnixNano,
				Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: value},
			}},
			AggregationTemporality: temporality,
			IsMonotonic:            isMonotonic,
		}},
	}
}

// mkGaugeMetric builds a Metric_Gauge with a single NumberDataPoint (no
// StartTime — matches the T2 sink's gauge shape).
func mkGaugeMetric(name string, timeUnixNano uint64, value float64, attrs ...*commonpb.KeyValue) *metricspb.Metric {
	return &metricspb.Metric{
		Name: name,
		Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
			DataPoints: []*metricspb.NumberDataPoint{{
				Attributes:   attrs,
				TimeUnixNano: timeUnixNano,
				Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: value},
			}},
		}},
	}
}

// mkResourceMetrics builds a *ResourceMetrics with the supplied Resource
// attributes and ONE ScopeMetrics carrying an EMPTY Scope (the T2 sink's
// shape: Scope present but with zero fields set) wrapping metrics.
func mkResourceMetrics(resAttrs []*commonpb.KeyValue, metrics ...*metricspb.Metric) *metricspb.ResourceMetrics {
	return &metricspb.ResourceMetrics{
		Resource: &resourcepb.Resource{Attributes: resAttrs},
		ScopeMetrics: []*metricspb.ScopeMetrics{{
			Scope:   &commonpb.InstrumentationScope{}, // present but empty
			Metrics: metrics,
		}},
	}
}

// datapointCount is a white-box test helper (lock-guarded, race-safe) that
// exposes the number of distinct (name, sorted-attrs) keys currently
// accumulated. Used to prove the order-insensitive keying property: two
// datapoints for the same name with the SAME attribute set in different
// orders must collapse onto ONE key, not split into two.
func (s *Server) datapointCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.latest)
}

// TestNew_StartsServerOnEphemeralPort verifies that New binds an ephemeral
// 127.0.0.1 port and Addr() returns the bound `host:port` string.
func TestNew_StartsServerOnEphemeralPort(t *testing.T) {
	srv := New(t)
	addr := srv.Addr()
	if addr == "" {
		t.Fatal("Addr: empty after New")
	}
}

// TestExport_SumGaugeAndOrderInsensitiveKeying drives the receiver with two
// unary Export calls: the first carries a Sum "test_counter" datapoint
// (attrs k1=v1,k2=v2) + a Gauge "test_gauge" datapoint; the second carries a
// SECOND "test_counter" datapoint with the SAME attributes REVERSED
// (k2=v2,k1=v1). It asserts:
//   - SumValue/GaugeValue read back the LATEST value for each name
//   - Temporality/IsMonotonic/StartTime reflect the latest Sum datapoint
//   - DeltaSum accumulates the per-flush values across BOTH Exports (5+7=12)
//   - the reversed-attrs datapoint resolves to the SAME internal key as the
//     first (datapointCount stays at 2: one for test_counter, one for
//     test_gauge — NOT 3), proving order-insensitive keying
//   - an absent name returns ok=false
//   - ResourceAttributes records one snapshot per ResourceMetrics (2 Exports)
func TestExport_SumGaugeAndOrderInsensitiveKeying(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resAttrs := []*commonpb.KeyValue{strKV("telemetry.sdk.name", "envoy-go")}

	req1 := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{mkResourceMetrics(resAttrs,
			mkSumMetric("test_counter", metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, true, 100, 200, 5,
				strKV("k1", "v1"), strKV("k2", "v2")),
			mkGaugeMetric("test_gauge", 200, 42),
		)},
	}
	if _, err := client.Export(ctx, req1); err != nil {
		t.Fatalf("Export(req1): %v", err)
	}

	// Second flush: SAME test_counter attribute set, REVERSED order, a new
	// value, and StartTime chained to the previous flush's TimeUnixNano (the
	// delta-mode chaining shape) — proves order-insensitive keying AND
	// DeltaSum accumulation.
	req2 := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{mkResourceMetrics(resAttrs,
			mkSumMetric("test_counter", metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, true, 200, 300, 7,
				strKV("k2", "v2"), strKV("k1", "v1")),
		)},
	}
	if _, err := client.Export(ctx, req2); err != nil {
		t.Fatalf("Export(req2): %v", err)
	}

	if got, ok := srv.SumValue("test_counter"); !ok || got != 7 {
		t.Errorf("SumValue(test_counter): got (%v,%v), want (7,true)", got, ok)
	}
	if got, ok := srv.GaugeValue("test_gauge"); !ok || got != 42 {
		t.Errorf("GaugeValue(test_gauge): got (%v,%v), want (42,true)", got, ok)
	}
	if got, ok := srv.Temporality("test_counter"); !ok || got != metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA {
		t.Errorf("Temporality(test_counter): got (%v,%v), want (DELTA,true)", got, ok)
	}
	if got, ok := srv.IsMonotonic("test_counter"); !ok || !got {
		t.Errorf("IsMonotonic(test_counter): got (%v,%v), want (true,true)", got, ok)
	}
	if got, ok := srv.StartTime("test_counter"); !ok || got != 200 {
		t.Errorf("StartTime(test_counter): got (%v,%v), want (200,true)", got, ok)
	}
	if got, ok := srv.DeltaSum("test_counter"); !ok || got != 12 {
		t.Errorf("DeltaSum(test_counter): got (%v,%v), want (12,true)", got, ok)
	}

	if got := srv.datapointCount(); got != 2 {
		t.Fatalf("datapointCount: got %d, want 2 (the reversed-attrs datapoint must collapse onto the SAME key as the first, not split into a third)", got)
	}

	if _, ok := srv.SumValue("does_not_exist"); ok {
		t.Error("SumValue(does_not_exist): got ok=true, want false")
	}
	if _, ok := srv.GaugeValue("test_counter"); ok {
		t.Error("GaugeValue(test_counter): got ok=true for a Sum-typed name, want false")
	}

	if got := srv.ResourceAttributes(); len(got) != 2 {
		t.Fatalf("ResourceAttributes: got %d sets, want 2", len(got))
	}
}

// TestReset_ClearsAccumulation verifies Reset() drops accumulated datapoints
// AND resource attributes AND the delta-sum accumulator.
func TestReset_ClearsAccumulation(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Export(ctx, &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{mkResourceMetrics(nil,
			mkSumMetric("c", metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true, 0, 100, 3),
		)},
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if _, ok := srv.SumValue("c"); !ok {
		t.Fatal("SumValue(c) before Reset: got ok=false, want true")
	}

	srv.Reset()

	if _, ok := srv.SumValue("c"); ok {
		t.Error("SumValue(c) after Reset: got ok=true, want false")
	}
	if _, ok := srv.DeltaSum("c"); ok {
		t.Error("DeltaSum(c) after Reset: got ok=true, want false")
	}
	if got := srv.ResourceAttributes(); len(got) != 0 {
		t.Errorf("ResourceAttributes after Reset: got %v, want empty", got)
	}
}

// TestClose_Idempotent verifies Close (the immediate hard-stop variant) is
// idempotent and mutually exclusive with Stop via the shared sync.Once.
func TestClose_Idempotent(t *testing.T) {
	srv, err := NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	srv.Close()
	srv.Close() // second Close: no-op via stopOnce.
	srv.Stop()  // Stop after Close: no-op (shared once).
}
