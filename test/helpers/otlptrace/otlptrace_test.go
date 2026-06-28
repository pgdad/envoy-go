package otlptrace

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// dialTestClient opens a plaintext h2c grpc.ClientConn to the supplied address,
// registers it for teardown via t.Cleanup, and returns the TraceServiceClient.
func dialTestClient(t *testing.T, addr string) coltracepb.TraceServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q): %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return coltracepb.NewTraceServiceClient(conn)
}

// mkSpan builds a minimal *Span whose Name is the supplied value (used to
// distinguish accumulated spans in order assertions).
func mkSpan(name string) *tracepb.Span {
	return &tracepb.Span{Name: name}
}

// mkResourceSpans builds a *ResourceSpans whose Resource carries a single
// service_name=name attribute and one ScopeSpans carrying the supplied spans.
func mkResourceSpans(serviceName string, spans ...*tracepb.Span) *tracepb.ResourceSpans {
	return &tracepb.ResourceSpans{
		Resource: &resourcepb.Resource{
			Attributes: []*commonpb.KeyValue{{
				Key: "service_name",
				Value: &commonpb.AnyValue{
					Value: &commonpb.AnyValue_StringValue{StringValue: serviceName},
				},
			}},
		},
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}},
	}
}

// TestNew_StartsServerOnEphemeralPort verifies that New binds an ephemeral
// 127.0.0.1 port and Addr() returns the bound `host:port` string.
func TestNew_StartsServerOnEphemeralPort(t *testing.T) {
	srv := New(t)
	addr := srv.Addr()
	if addr == "" {
		t.Fatal("Addr: empty after New")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	if host != "127.0.0.1" {
		t.Errorf("host: got %q, want %q", host, "127.0.0.1")
	}
	if port == "0" {
		t.Errorf("port: got %q, want non-zero ephemeral", port)
	}
}

// TestExport_AccumulatesAcrossCalls drives the receiver the way the differential
// driver does: a client Exports TWO requests (the first with a Resource carrying
// service_name=S + ONE span, the second with TWO spans), then asserts Count()==3
// and Spans() returns all three in order. The second Export accumulates onto the
// same slices, proving cross-call accumulation. ResourceAttributes() carries the
// per-ResourceSpans Resource.attributes snapshots including the service_name key.
func TestExport_AccumulatesAcrossCalls(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First Export: one ResourceSpans, one span.
	if _, err := client.Export(ctx, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{mkResourceSpans("S", mkSpan("/a"))},
	}); err != nil {
		t.Fatalf("Export(req1): %v", err)
	}

	// Second Export: one ResourceSpans, two spans.
	if _, err := client.Export(ctx, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{mkResourceSpans("S", mkSpan("/b"), mkSpan("/c"))},
	}); err != nil {
		t.Fatalf("Export(req2): %v", err)
	}

	if got := srv.Count(); got != 3 {
		t.Fatalf("Count: got %d, want 3", got)
	}
	spans := srv.Spans()
	if len(spans) != 3 {
		t.Fatalf("Spans: got %d, want 3", len(spans))
	}
	wantNames := []string{"/a", "/b", "/c"}
	for i, want := range wantNames {
		if got := spans[i].GetName(); got != want {
			t.Errorf("spans[%d].name: got %q, want %q (order not preserved)", i, got, want)
		}
	}

	// ResourceAttributes: one snapshot per ResourceSpans (2 Exports => 2 entries),
	// each carrying the service_name key.
	resAttrs := srv.ResourceAttributes()
	if len(resAttrs) != 2 {
		t.Fatalf("ResourceAttributes: got %d sets, want 2", len(resAttrs))
	}
	for i, set := range resAttrs {
		var foundServiceName bool
		for _, kv := range set {
			if kv.GetKey() == "service_name" {
				foundServiceName = true
				if got := kv.GetValue().GetStringValue(); got != "S" {
					t.Errorf("resAttrs[%d] service_name: got %q, want %q", i, got, "S")
				}
			}
		}
		if !foundServiceName {
			t.Errorf("resAttrs[%d]: missing service_name key", i)
		}
	}
}

// TestSpans_ReturnsSnapshotCopy verifies Spans() returns a defensive copy:
// mutating the returned slice must not perturb the server's accumulation.
func TestSpans_ReturnsSnapshotCopy(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Export(ctx, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{mkResourceSpans("S", mkSpan("/x"))},
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	snap := srv.Spans()
	if len(snap) != 1 {
		t.Fatalf("Spans: got %d, want 1", len(snap))
	}
	snap[0] = nil // mutate the caller copy
	if srv.Spans()[0] == nil {
		t.Error("Spans returned an aliased slice; caller mutation leaked into the server")
	}
}

// TestResourceAttributes_ReturnsSnapshotCopy verifies ResourceAttributes()
// returns a defensive outer-slice copy.
func TestResourceAttributes_ReturnsSnapshotCopy(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Export(ctx, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{mkResourceSpans("S", mkSpan("/x"))},
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	snap := srv.ResourceAttributes()
	if len(snap) != 1 {
		t.Fatalf("ResourceAttributes: got %d, want 1", len(snap))
	}
	snap[0] = nil // mutate the caller copy
	if srv.ResourceAttributes()[0] == nil {
		t.Error("ResourceAttributes returned an aliased slice; caller mutation leaked into the server")
	}
}

// TestReset_ClearsAccumulation verifies Reset() drops accumulated spans AND
// resource attributes so a differential driver can reuse one server across
// per-side snapshots.
func TestReset_ClearsAccumulation(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Export(ctx, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{mkResourceSpans("S", mkSpan("/x"))},
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got := srv.Count(); got != 1 {
		t.Fatalf("Count before Reset: got %d, want 1", got)
	}
	srv.Reset()
	if got := srv.Count(); got != 0 {
		t.Errorf("Count after Reset: got %d, want 0", got)
	}
	if got := srv.ResourceAttributes(); len(got) != 0 {
		t.Errorf("ResourceAttributes after Reset: got %v, want empty", got)
	}
}

// TestConcurrentExports_NoRace verifies concurrent Export clients accumulating
// into the shared slices do not trip the race detector while a poller reads
// Count()/Spans()/ResourceAttributes() concurrently.
func TestConcurrentExports_NoRace(t *testing.T) {
	srv := New(t)

	const concurrency = 16
	var wg sync.WaitGroup
	wg.Add(concurrency)
	errs := make([]error, concurrency)
	for i := 0; i < concurrency; i++ {
		i := i
		go func() {
			defer wg.Done()
			client := dialTestClient(t, srv.Addr())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := client.Export(ctx, &coltracepb.ExportTraceServiceRequest{
				ResourceSpans: []*tracepb.ResourceSpans{mkResourceSpans("S", mkSpan("/c"))},
			}); err != nil {
				errs[i] = err
			}
		}()
	}

	// Concurrent poller, mimicking the driver's converge loop. Bounded on a
	// deadline so an early Export error makes the test fail fast instead of
	// hanging into a whole-suite timeout.
	done := make(chan struct{})
	deadline := time.After(10 * time.Second)
	go func() {
		defer close(done)
		for {
			if srv.Count() >= concurrency {
				return
			}
			select {
			case <-deadline:
				return
			default:
			}
			_ = srv.Spans()
			_ = srv.ResourceAttributes()
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
	<-done
	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if got := srv.Count(); got != concurrency {
		t.Errorf("Count: got %d, want %d", got, concurrency)
	}
}

// TestClose_Idempotent verifies Close (the immediate hard-stop variant used by
// the differential driver) is idempotent and mutually exclusive with Stop via
// the shared sync.Once — repeated Close()/Stop() calls after the first are
// no-ops and must not panic.
func TestClose_Idempotent(t *testing.T) {
	srv, err := NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	srv.Close()
	srv.Close() // second Close: no-op via stopOnce.
	srv.Stop()  // Stop after Close: no-op (shared once).
}
