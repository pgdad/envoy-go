package accessloggrpc

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	dataaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// dialTestClient opens a plaintext h2c grpc.ClientConn to the supplied address,
// registers it for teardown via t.Cleanup, and returns the AccessLogServiceClient.
func dialTestClient(t *testing.T, addr string) accesslogv3.AccessLogServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q): %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return accesslogv3.NewAccessLogServiceClient(conn)
}

// mkEntry builds a minimal *HTTPAccessLogEntry whose :path is the supplied
// value (used to distinguish accumulated entries in order assertions).
func mkEntry(path string) *dataaccesslogv3.HTTPAccessLogEntry {
	return &dataaccesslogv3.HTTPAccessLogEntry{
		Request: &dataaccesslogv3.HTTPRequestProperties{
			Path: path,
		},
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

// TestStreamAccessLogs_AccumulatesAcrossMessagesAndStreams drives the receiver
// the way the differential driver does: a client opens StreamAccessLogs, sends
// TWO messages (the first with an Identifier + ONE entry, the second with TWO
// batched entries), CloseAndRecv, then asserts Count()==3 and Entries() returns
// all three in order. A SECOND stream from the same client sends one more entry,
// proving entries accumulate across streams onto the same slice (AMEND-ALS-3).
func TestStreamAccessLogs_AccumulatesAcrossMessagesAndStreams(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First stream: two messages, three entries total.
	stream, err := client.StreamAccessLogs(ctx)
	if err != nil {
		t.Fatalf("StreamAccessLogs: %v", err)
	}
	if err := stream.Send(&accesslogv3.StreamAccessLogsMessage{
		Identifier: &accesslogv3.StreamAccessLogsMessage_Identifier{
			LogName: "test-als",
		},
		LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
			HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
				LogEntry: []*dataaccesslogv3.HTTPAccessLogEntry{mkEntry("/a")},
			},
		},
	}); err != nil {
		t.Fatalf("Send(msg1): %v", err)
	}
	if err := stream.Send(&accesslogv3.StreamAccessLogsMessage{
		LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
			HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
				LogEntry: []*dataaccesslogv3.HTTPAccessLogEntry{mkEntry("/b"), mkEntry("/c")},
			},
		},
	}); err != nil {
		t.Fatalf("Send(msg2): %v", err)
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatalf("CloseAndRecv(stream1): %v", err)
	}

	if got := srv.Count(); got != 3 {
		t.Fatalf("Count after stream1: got %d, want 3", got)
	}
	entries := srv.Entries()
	if len(entries) != 3 {
		t.Fatalf("Entries after stream1: got %d, want 3", len(entries))
	}
	wantPaths := []string{"/a", "/b", "/c"}
	for i, want := range wantPaths {
		if got := entries[i].GetRequest().GetPath(); got != want {
			t.Errorf("entries[%d].path: got %q, want %q (order not preserved)", i, got, want)
		}
	}

	// Second stream from the same client: one more entry accumulates.
	stream2, err := client.StreamAccessLogs(ctx)
	if err != nil {
		t.Fatalf("StreamAccessLogs(2): %v", err)
	}
	if err := stream2.Send(&accesslogv3.StreamAccessLogsMessage{
		LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
			HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
				LogEntry: []*dataaccesslogv3.HTTPAccessLogEntry{mkEntry("/d")},
			},
		},
	}); err != nil {
		t.Fatalf("Send(stream2): %v", err)
	}
	if _, err := stream2.CloseAndRecv(); err != nil {
		t.Fatalf("CloseAndRecv(stream2): %v", err)
	}

	if got := srv.Count(); got != 4 {
		t.Fatalf("Count after stream2: got %d, want 4 (entries must accumulate across streams)", got)
	}
	if got := srv.Entries()[3].GetRequest().GetPath(); got != "/d" {
		t.Errorf("entries[3].path: got %q, want %q", got, "/d")
	}
}

// TestEntries_ReturnsSnapshotCopy verifies Entries() returns a defensive copy:
// mutating the returned slice must not perturb the server's accumulation.
func TestEntries_ReturnsSnapshotCopy(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamAccessLogs(ctx)
	if err != nil {
		t.Fatalf("StreamAccessLogs: %v", err)
	}
	if err := stream.Send(&accesslogv3.StreamAccessLogsMessage{
		LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
			HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
				LogEntry: []*dataaccesslogv3.HTTPAccessLogEntry{mkEntry("/x")},
			},
		},
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatalf("CloseAndRecv: %v", err)
	}

	snap := srv.Entries()
	if len(snap) != 1 {
		t.Fatalf("Entries: got %d, want 1", len(snap))
	}
	snap[0] = nil // mutate the caller copy
	if srv.Entries()[0] == nil {
		t.Error("Entries returned an aliased slice; caller mutation leaked into the server")
	}
}

// TestReset_ClearsAccumulation verifies Reset() drops accumulated entries so a
// differential driver can reuse one server across per-side snapshots.
func TestReset_ClearsAccumulation(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamAccessLogs(ctx)
	if err != nil {
		t.Fatalf("StreamAccessLogs: %v", err)
	}
	if err := stream.Send(&accesslogv3.StreamAccessLogsMessage{
		LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
			HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
				LogEntry: []*dataaccesslogv3.HTTPAccessLogEntry{mkEntry("/x")},
			},
		},
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatalf("CloseAndRecv: %v", err)
	}
	if got := srv.Count(); got != 1 {
		t.Fatalf("Count before Reset: got %d, want 1", got)
	}
	srv.Reset()
	if got := srv.Count(); got != 0 {
		t.Errorf("Count after Reset: got %d, want 0", got)
	}
}

// TestConcurrentStreams_NoRace verifies concurrent StreamAccessLogs clients
// accumulating into the shared slice do not trip the race detector while a
// poller reads Count()/Entries() concurrently.
func TestConcurrentStreams_NoRace(t *testing.T) {
	srv := New(t)

	const concurrency = 16
	var wg sync.WaitGroup
	wg.Add(concurrency)
	errs := make([]error, concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			client := dialTestClient(t, srv.Addr())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stream, err := client.StreamAccessLogs(ctx)
			if err != nil {
				errs[i] = err
				return
			}
			if err := stream.Send(&accesslogv3.StreamAccessLogsMessage{
				LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
					HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
						LogEntry: []*dataaccesslogv3.HTTPAccessLogEntry{mkEntry("/c")},
					},
				},
			}); err != nil {
				errs[i] = err
				return
			}
			if _, err := stream.CloseAndRecv(); err != nil {
				errs[i] = err
			}
		}()
	}

	// Concurrent poller, mimicking the driver's converge loop. Bounded on a
	// deadline so an early stream error (which leaves Count() < concurrency
	// forever) makes the test fail fast with the captured errs instead of
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
			_ = srv.Entries()
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
