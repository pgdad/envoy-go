package otlplogs

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// dialTestClient opens a plaintext h2c grpc.ClientConn to the supplied address,
// registers it for teardown via t.Cleanup, and returns the LogsServiceClient.
func dialTestClient(t *testing.T, addr string) collogspb.LogsServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q): %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return collogspb.NewLogsServiceClient(conn)
}

// mkRecord builds a minimal *LogRecord whose SeverityText is the supplied value
// (used to distinguish accumulated records in order assertions).
func mkRecord(sev string) *logspb.LogRecord {
	return &logspb.LogRecord{SeverityText: sev}
}

// mkResourceLogs builds a *ResourceLogs whose Resource carries a single
// log_name=name attribute and one ScopeLogs carrying the supplied records.
func mkResourceLogs(logName string, recs ...*logspb.LogRecord) *logspb.ResourceLogs {
	return &logspb.ResourceLogs{
		Resource: &resourcepb.Resource{
			Attributes: []*commonpb.KeyValue{{
				Key: "log_name",
				Value: &commonpb.AnyValue{
					Value: &commonpb.AnyValue_StringValue{StringValue: logName},
				},
			}},
		},
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: recs}},
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
// log_name=L + ONE record, the second with TWO records), then asserts Count()==3
// and Records() returns all three in order. The second Export accumulates onto
// the same slices, proving cross-call accumulation. ResourceAttributes() carries
// the per-ResourceLogs Resource.attributes snapshots including the log_name key.
func TestExport_AccumulatesAcrossCalls(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First Export: one ResourceLogs, one record.
	if _, err := client.Export(ctx, &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{mkResourceLogs("L", mkRecord("/a"))},
	}); err != nil {
		t.Fatalf("Export(req1): %v", err)
	}

	// Second Export: one ResourceLogs, two records.
	if _, err := client.Export(ctx, &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{mkResourceLogs("L", mkRecord("/b"), mkRecord("/c"))},
	}); err != nil {
		t.Fatalf("Export(req2): %v", err)
	}

	if got := srv.Count(); got != 3 {
		t.Fatalf("Count: got %d, want 3", got)
	}
	records := srv.Records()
	if len(records) != 3 {
		t.Fatalf("Records: got %d, want 3", len(records))
	}
	wantSev := []string{"/a", "/b", "/c"}
	for i, want := range wantSev {
		if got := records[i].GetSeverityText(); got != want {
			t.Errorf("records[%d].severityText: got %q, want %q (order not preserved)", i, got, want)
		}
	}

	// ResourceAttributes: one snapshot per ResourceLogs (2 Exports => 2 entries),
	// each carrying the log_name key.
	resAttrs := srv.ResourceAttributes()
	if len(resAttrs) != 2 {
		t.Fatalf("ResourceAttributes: got %d sets, want 2", len(resAttrs))
	}
	for i, set := range resAttrs {
		var foundLogName bool
		for _, kv := range set {
			if kv.GetKey() == "log_name" {
				foundLogName = true
				if got := kv.GetValue().GetStringValue(); got != "L" {
					t.Errorf("resAttrs[%d] log_name: got %q, want %q", i, got, "L")
				}
			}
		}
		if !foundLogName {
			t.Errorf("resAttrs[%d]: missing log_name key", i)
		}
	}
}

// TestRecords_ReturnsSnapshotCopy verifies Records() returns a defensive copy:
// mutating the returned slice must not perturb the server's accumulation.
func TestRecords_ReturnsSnapshotCopy(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Export(ctx, &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{mkResourceLogs("L", mkRecord("/x"))},
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	snap := srv.Records()
	if len(snap) != 1 {
		t.Fatalf("Records: got %d, want 1", len(snap))
	}
	snap[0] = nil // mutate the caller copy
	if srv.Records()[0] == nil {
		t.Error("Records returned an aliased slice; caller mutation leaked into the server")
	}
}

// TestResourceAttributes_ReturnsSnapshotCopy verifies ResourceAttributes()
// returns a defensive outer-slice copy.
func TestResourceAttributes_ReturnsSnapshotCopy(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Export(ctx, &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{mkResourceLogs("L", mkRecord("/x"))},
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

// TestReset_ClearsAccumulation verifies Reset() drops accumulated records AND
// resource attributes so a differential driver can reuse one server across
// per-side snapshots.
func TestReset_ClearsAccumulation(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Export(ctx, &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{mkResourceLogs("L", mkRecord("/x"))},
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
// Count()/Records()/ResourceAttributes() concurrently.
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
			if _, err := client.Export(ctx, &collogspb.ExportLogsServiceRequest{
				ResourceLogs: []*logspb.ResourceLogs{mkResourceLogs("L", mkRecord("/c"))},
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
			_ = srv.Records()
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
