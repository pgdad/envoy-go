package metricsservice

import (
	"context"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

// counterFamily builds a single-Metric COUNTER MetricFamily named `name` with
// the absolute value `v` (the mapping.snapshot shape).
func counterFamily(name string, v float64) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name: proto.String(name),
		Type: dto.MetricType_COUNTER.Enum(),
		Metric: []*dto.Metric{{
			Counter: &dto.Counter{Value: proto.Float64(v)},
		}},
	}
}

// gaugeFamily builds a single-Metric GAUGE MetricFamily named `name` with the
// value `v`.
func gaugeFamily(name string, v float64) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name: proto.String(name),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{{
			Gauge: &dto.Gauge{Value: proto.Float64(v)},
		}},
	}
}

// dialClient opens a bare insecure (h2c) MetricsServiceClient against the
// helper's bound address.
func dialClient(t *testing.T, addr string) metricsv3.MetricsServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return metricsv3.NewMetricsServiceClient(conn)
}

func TestServer_StreamMetrics_AccumulatesByName(t *testing.T) {
	s := New(t)
	client := dialClient(t, s.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamMetrics(ctx)
	if err != nil {
		t.Fatalf("StreamMetrics: %v", err)
	}

	// First message: identifier (Node) + two families.
	if err := stream.Send(&metricsv3.StreamMetricsMessage{
		Identifier: &metricsv3.StreamMetricsMessage_Identifier{
			Node: &corev3.Node{Id: "n", Cluster: "c"},
		},
		EnvoyMetrics: []*dto.MetricFamily{
			counterFamily("cluster.x.upstream_rq_total", 7),
			gaugeFamily("g", 3),
		},
	}); err != nil {
		t.Fatalf("Send #1: %v", err)
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		t.Fatalf("CloseAndRecv: %v", err)
	}
	if resp == nil {
		t.Fatal("CloseAndRecv: nil response")
	}

	if got := s.Count(); got != 2 {
		t.Fatalf("Count() = %d, want 2", got)
	}

	v, typ, ok := s.Family("cluster.x.upstream_rq_total")
	if !ok {
		t.Fatal(`Family("cluster.x.upstream_rq_total"): ok=false, want true`)
	}
	if v != 7.0 {
		t.Errorf("Family value = %v, want 7.0", v)
	}
	if typ != dto.MetricType_COUNTER {
		t.Errorf("Family type = %v, want COUNTER", typ)
	}

	gv, gtyp, ok := s.Family("g")
	if !ok || gv != 3.0 || gtyp != dto.MetricType_GAUGE {
		t.Errorf(`Family("g") = (%v, %v, %v), want (3, GAUGE, true)`, gv, gtyp, ok)
	}

	node := s.Node()
	if node == nil {
		t.Fatal("Node() = nil, want non-nil")
	}
	if node.GetId() != "n" || node.GetCluster() != "c" {
		t.Errorf("Node() = id=%q cluster=%q, want id=n cluster=c", node.GetId(), node.GetCluster())
	}
}

func TestServer_StreamMetrics_LastSeenWins(t *testing.T) {
	s := New(t)
	client := dialClient(t, s.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamMetrics(ctx)
	if err != nil {
		t.Fatalf("StreamMetrics: %v", err)
	}

	if err := stream.Send(&metricsv3.StreamMetricsMessage{
		Identifier: &metricsv3.StreamMetricsMessage_Identifier{
			Node: &corev3.Node{Id: "n", Cluster: "c"},
		},
		EnvoyMetrics: []*dto.MetricFamily{counterFamily("cluster.x.upstream_rq_total", 7)},
	}); err != nil {
		t.Fatalf("Send #1: %v", err)
	}

	// Second message updates the same counter to 9 (no identifier on subsequent
	// messages — the first-message capture must persist).
	if err := stream.Send(&metricsv3.StreamMetricsMessage{
		EnvoyMetrics: []*dto.MetricFamily{counterFamily("cluster.x.upstream_rq_total", 9)},
	}); err != nil {
		t.Fatalf("Send #2: %v", err)
	}

	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatalf("CloseAndRecv: %v", err)
	}

	if got := s.Count(); got != 1 {
		t.Fatalf("Count() = %d, want 1 (same name updated)", got)
	}
	v, _, ok := s.Family("cluster.x.upstream_rq_total")
	if !ok || v != 9.0 {
		t.Fatalf("Family value = (%v, %v), want last-seen 9.0", v, ok)
	}
	// Identifier captured on the first message must persist across messages.
	if n := s.Node(); n == nil || n.GetId() != "n" {
		t.Errorf("Node() = %v, want persisted id=n", n)
	}
}

func TestServer_Reset(t *testing.T) {
	s := New(t)
	client := dialClient(t, s.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamMetrics(ctx)
	if err != nil {
		t.Fatalf("StreamMetrics: %v", err)
	}
	if err := stream.Send(&metricsv3.StreamMetricsMessage{
		Identifier:   &metricsv3.StreamMetricsMessage_Identifier{Node: &corev3.Node{Id: "n", Cluster: "c"}},
		EnvoyMetrics: []*dto.MetricFamily{counterFamily("a", 1)},
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatalf("CloseAndRecv: %v", err)
	}
	if s.Count() != 1 {
		t.Fatalf("Count() = %d, want 1 before Reset", s.Count())
	}

	s.Reset()

	if s.Count() != 0 {
		t.Errorf("Count() = %d, want 0 after Reset", s.Count())
	}
	if _, _, ok := s.Family("a"); ok {
		t.Error("Family(a) ok=true after Reset, want false")
	}
	if s.Node() != nil {
		t.Error("Node() != nil after Reset, want nil")
	}
}

func TestServer_Stop(t *testing.T) {
	s := New(t)
	// Stop is idempotent; explicit + the t.Cleanup-registered call must not panic.
	s.Stop()
	s.Stop()
}
