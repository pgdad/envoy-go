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

// TestBatchSizes_TracksPerMessageEntryCount verifies the 44.2
// D-BUF-RECEIVER-BATCH-API additions: BatchSizes, MaxBatchSize, and
// MessageCount track per-message entry counts, and Reset clears them both
// alongside the flat entries accumulator.
//
// The test sends THREE messages with entry counts {1, 3, 2} (the second and
// third simulate a buffered flush carrying multiple entries), asserts the new
// accessors, then validates Reset zeroes all state.
func TestBatchSizes_TracksPerMessageEntryCount(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamAccessLogs(ctx)
	if err != nil {
		t.Fatalf("StreamAccessLogs: %v", err)
	}

	// Message 1: 1 entry.
	if err := stream.Send(&accesslogv3.StreamAccessLogsMessage{
		LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
			HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
				LogEntry: []*dataaccesslogv3.HTTPAccessLogEntry{mkEntry("/m1a")},
			},
		},
	}); err != nil {
		t.Fatalf("Send(msg1): %v", err)
	}
	// Message 2: 3 entries (simulates a buffered batch).
	if err := stream.Send(&accesslogv3.StreamAccessLogsMessage{
		LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
			HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
				LogEntry: []*dataaccesslogv3.HTTPAccessLogEntry{
					mkEntry("/m2a"), mkEntry("/m2b"), mkEntry("/m2c"),
				},
			},
		},
	}); err != nil {
		t.Fatalf("Send(msg2): %v", err)
	}
	// Message 3: 2 entries.
	if err := stream.Send(&accesslogv3.StreamAccessLogsMessage{
		LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
			HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
				LogEntry: []*dataaccesslogv3.HTTPAccessLogEntry{
					mkEntry("/m3a"), mkEntry("/m3b"),
				},
			},
		},
	}); err != nil {
		t.Fatalf("Send(msg3): %v", err)
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatalf("CloseAndRecv: %v", err)
	}

	// Flat entry total is unchanged: 1+3+2 == 6.
	if got := srv.Count(); got != 6 {
		t.Errorf("Count: got %d, want 6", got)
	}
	// MessageCount: one int per received message.
	if got := srv.MessageCount(); got != 3 {
		t.Errorf("MessageCount: got %d, want 3", got)
	}
	// BatchSizes: per-message entry counts in arrival order.
	gotSizes := srv.BatchSizes()
	wantSizes := []int{1, 3, 2}
	if len(gotSizes) != len(wantSizes) {
		t.Fatalf("BatchSizes len: got %d, want %d", len(gotSizes), len(wantSizes))
	}
	for i, want := range wantSizes {
		if gotSizes[i] != want {
			t.Errorf("BatchSizes[%d]: got %d, want %d", i, gotSizes[i], want)
		}
	}
	// MaxBatchSize: the largest per-message count.
	if got := srv.MaxBatchSize(); got != 3 {
		t.Errorf("MaxBatchSize: got %d, want 3", got)
	}

	// After Reset: both entries AND batchSizes clear.
	srv.Reset()
	if got := srv.Count(); got != 0 {
		t.Errorf("Count after Reset: got %d, want 0", got)
	}
	if got := srv.MessageCount(); got != 0 {
		t.Errorf("MessageCount after Reset: got %d, want 0", got)
	}
	if got := srv.MaxBatchSize(); got != 0 {
		t.Errorf("MaxBatchSize after Reset: got %d, want 0", got)
	}
	if got := srv.BatchSizes(); len(got) != 0 {
		t.Errorf("BatchSizes after Reset: got %v, want empty", got)
	}
}

// TestMaxBatchSize_ZeroOnFreshServer verifies MaxBatchSize returns 0 (no panic)
// when called on a server that has not yet received any messages.
func TestMaxBatchSize_ZeroOnFreshServer(t *testing.T) {
	srv := New(t)
	if got := srv.MaxBatchSize(); got != 0 {
		t.Errorf("MaxBatchSize on fresh server: got %d, want 0", got)
	}
	if got := srv.MessageCount(); got != 0 {
		t.Errorf("MessageCount on fresh server: got %d, want 0", got)
	}
	if got := srv.BatchSizes(); len(got) != 0 {
		t.Errorf("BatchSizes on fresh server: got %v, want empty", got)
	}
}

// TestConcurrentStreams_NoRace_BatchSizes extends the existing concurrency test
// to cover the new batchSizes field under the -race detector: concurrent
// StreamAccessLogs goroutines write batchSizes, while a poller reads
// BatchSizes()/MessageCount()/MaxBatchSize() concurrently.
func TestConcurrentStreams_NoRace_BatchSizes(t *testing.T) {
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
			stream, err := client.StreamAccessLogs(ctx)
			if err != nil {
				errs[i] = err
				return
			}
			if err := stream.Send(&accesslogv3.StreamAccessLogsMessage{
				LogEntries: &accesslogv3.StreamAccessLogsMessage_HttpLogs{
					HttpLogs: &accesslogv3.StreamAccessLogsMessage_HTTPAccessLogEntries{
						LogEntry: []*dataaccesslogv3.HTTPAccessLogEntry{mkEntry("/r"), mkEntry("/r2")},
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

	// Concurrent poller exercising all three new accessors under -race.
	done := make(chan struct{})
	deadline := time.After(10 * time.Second)
	go func() {
		defer close(done)
		for {
			if srv.Count() >= concurrency*2 {
				return
			}
			select {
			case <-deadline:
				return
			default:
			}
			_ = srv.BatchSizes()
			_ = srv.MessageCount()
			_ = srv.MaxBatchSize()
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
	if got := srv.Count(); got != concurrency*2 {
		t.Errorf("Count: got %d, want %d", got, concurrency*2)
	}
	if got := srv.MessageCount(); got != concurrency {
		t.Errorf("MessageCount: got %d, want %d", got, concurrency)
	}
	if got := srv.MaxBatchSize(); got != 2 {
		t.Errorf("MaxBatchSize: got %d, want 2", got)
	}
}
