package accessloggrpc

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	dataaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	"google.golang.org/grpc"
)

// Server is a minimal in-process AccessLogService gRPC server that accumulates
// streamed *HTTPAccessLogEntry records per D-ALS-RECEIVER. Goroutine-safe: the
// accumulator slice is guarded by an RWMutex so the StreamAccessLogs append
// path and the Count/Entries poll surface are race-free under the -race
// detector (see TestConcurrentStreams_NoRace).
//
// Lifecycle:
//   - New(t) binds 127.0.0.1:0 (ephemeral port) + spawns grpcSrv.Serve(lis) in
//     a goroutine + registers t.Cleanup(Stop). The caller reads Addr() to
//     templatize the ALS grpc_service cluster port into the fixture yaml configs.
//   - NewAtAddr(addr) binds the caller-supplied `host:port` (e.g.
//     "0.0.0.0:<port>" so a Docker reference-Envoy can dial the host) + spawns
//     grpcSrv.Serve(lis) in a goroutine. No t.Cleanup is registered — the
//     caller manages lifecycle via Server.Stop. Used by the fixture-0081
//     differential driver.
//   - StreamAccessLogs drains the client stream, appending every message's HTTP
//     log entries to the accumulator (across BOTH messages and successive
//     streams, AMEND-ALS-3), and SendAndClose on io.EOF.
//   - Count()/Entries() expose the accumulator: Count is the converge poll,
//     Entries is a defensive snapshot copy in arrival order.
//   - Reset() drops the accumulator (per-side snapshot separation).
//   - Stop() GracefulStops the *grpc.Server; idempotent via sync.Once.
//
// Plaintext h2c — no TLS — per D-ALS-RECEIVER: grpc.NewServer() with no Creds()
// option, listener is plain net.Listen("tcp", ...). The fixture's ALS cluster
// uses http2_protocol_options{} without an upstream TLS context.
type Server struct {
	accesslogv3.UnimplementedAccessLogServiceServer

	addr    string
	lis     net.Listener
	grpcSrv *grpc.Server

	mu      sync.RWMutex
	entries []*dataaccesslogv3.HTTPAccessLogEntry

	stopOnce sync.Once
}

// New binds an ephemeral 127.0.0.1 listener and starts an
// accesslogv3.AccessLogServiceServer-registered *grpc.Server in a background
// goroutine. The cleanup is registered via t.Cleanup so test functions do NOT
// need to explicitly call Stop (though they may — Stop is idempotent).
//
// New uses testing.TB so it composes equally with *testing.T, *testing.B, and
// *testing.F (the fuzzer harness). For callers that need a caller-chosen
// listener address (notably the fixture-0081 differential driver — bootstrap
// YAMLs bake in a stable ALS-cluster endpoint port that MUST be allocated
// before the gRPC server starts), see NewAtAddr.
func New(t testing.TB) *Server {
	t.Helper()
	s, err := newServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("accessloggrpc: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

// NewAtAddr binds a listener on the caller-supplied `host:port` (e.g.
// "0.0.0.0:<preAllocatedPort>" so a Docker reference-Envoy can dial the host)
// and starts an accesslogv3.AccessLogServiceServer-registered *grpc.Server in a
// background goroutine. Returns the server + nil on success, or nil + a wrapped
// net.Listen / setup error.
//
// Used by differential fixtures whose envoy.yaml / envoy-go.yaml bootstraps
// templatize a baked-in ALS grpc_service cluster endpoint that MUST match the
// receiver's bound address — the driver allocates a free port via Listen+Close
// BEFORE the bootstraps render, then NewAtAddr re-binds the gRPC server on that
// exact port. Mirrors the extauthzgrpc.NewAtAddr idiom (caller-chosen-port via
// address argument).
//
// Lifecycle management is the CALLER's responsibility — there is no t.Cleanup
// registration (the caller may not be a *testing.T-rooted context; fixture
// drivers stop the server explicitly via Server.Stop). Tests that want
// auto-cleanup should use New(t) instead.
func NewAtAddr(addr string) (*Server, error) {
	return newServer(addr)
}

// newServer is the shared constructor body used by both New(t) and
// NewAtAddr(addr). It binds a TCP listener on `addr` (which may be
// "127.0.0.1:0" for an ephemeral port), constructs the *Server, registers the
// accesslogv3.AccessLogServiceServer impl, and spawns grpcSrv.Serve(lis) in a
// background goroutine. Returns (nil, error) on net.Listen failure.
func newServer(addr string) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	s := &Server{
		addr:    lis.Addr().String(),
		lis:     lis,
		grpcSrv: grpc.NewServer(),
	}
	accesslogv3.RegisterAccessLogServiceServer(s.grpcSrv, s)

	go func() {
		// grpc.Server.Serve returns nil on GracefulStop. Any earlier error
		// (e.g., listener close from a panic path) is discarded — the caller
		// observes lifecycle via Stop's idempotent registration.
		_ = s.grpcSrv.Serve(lis)
	}()

	return s, nil
}

// StreamAccessLogs implements accesslogv3.AccessLogServiceServer. It drains the
// client stream until io.EOF, appending every message's HTTP log entries to the
// accumulator (the Identifier on the first message is ignored — the driver
// asserts the aggregated entries, not the per-stream metadata). On io.EOF it
// SendAndCloses an empty *StreamAccessLogsResponse, the AccessLogService ack.
func (s *Server) StreamAccessLogs(stream accesslogv3.AccessLogService_StreamAccessLogsServer) error {
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&accesslogv3.StreamAccessLogsResponse{})
		}
		if err != nil {
			return err
		}
		if httpLogs := msg.GetHttpLogs(); httpLogs != nil {
			s.mu.Lock()
			s.entries = append(s.entries, httpLogs.GetLogEntry()...)
			s.mu.Unlock()
		}
	}
}

// Entries returns a defensive snapshot copy of the accumulated
// *HTTPAccessLogEntry records in arrival order. The slice is freshly allocated
// so caller mutation cannot perturb the server's accumulation; the entry
// pointers themselves are shared (the driver only reads them).
func (s *Server) Entries() []*dataaccesslogv3.HTTPAccessLogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*dataaccesslogv3.HTTPAccessLogEntry, len(s.entries))
	copy(out, s.entries)
	return out
}

// Count returns the number of accumulated entries. Load-bearing as the
// differential driver's converge poll — it spins on Count() reaching the
// expected entry total before reading Entries() to assert.
func (s *Server) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// Reset drops all accumulated entries. Used when a single server instance is
// reused across per-side snapshots (the driver may prefer a fresh server per
// side; Reset is the lighter-weight alternative).
//
// Call only when no stream is in flight (between per-side snapshots; server
// quiescent); a concurrent in-flight append would land after the nil and
// survive the reset. It is mutex-guarded so memory-safe, but the quiescence
// contract is the caller's to uphold.
func (s *Server) Reset() {
	s.mu.Lock()
	s.entries = nil
	s.mu.Unlock()
}

// Addr returns the listener's bound `host:port` string. Load-bearing when New
// allocated an ephemeral port — the fixture driver reads this back to
// templatize the ALS grpc_service cluster endpoint into envoy.yaml +
// envoy-go.yaml.
func (s *Server) Addr() string {
	return s.addr
}

// Stop GracefulStops the *grpc.Server. Idempotent via sync.Once: multiple calls
// are no-ops after the first. Registered as t.Cleanup at New time, so callers
// typically do NOT need to invoke Stop explicitly — the test runtime drives it
// at test end.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		s.grpcSrv.GracefulStop()
	})
}

// Close is the immediate hard-stop variant of Stop (GracefulStop) for callers
// (e.g. the differential driver) that have already snapshotted entries and want
// deterministic teardown without waiting on still-open proxy streams. It calls
// grpc.Server.Stop (cancels in-flight streams, returns at once) and shares the
// same sync.Once as Stop so the two are idempotent and mutually exclusive.
func (s *Server) Close() {
	s.stopOnce.Do(s.grpcSrv.Stop)
}
