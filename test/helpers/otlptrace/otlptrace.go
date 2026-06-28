// Package otlptrace provides a minimal in-process OTLP TraceService gRPC server
// for tests and differential fixtures. It is the OTLP-trace counterpart of the
// otlplogs helper (the OTLP logs receiver): a driver-owned receiver that the
// proxy DIALS, so per project convention it is a test helper rather than a
// runner BackendKind (D-OTLP-RECEIVER-WIRING; BackendKind STAYS 38).
package otlptrace

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
)

// Server is a minimal in-process OTLP TraceService gRPC server that accumulates
// every *Span across unary Export calls (flattening the
// ResourceSpans/ScopeSpans nesting) AND records each ResourceSpans.Resource's
// attribute set in arrival order. Goroutine-safe: both accumulator slices are
// guarded by an RWMutex so the Export append path and the
// Count/Spans/ResourceAttributes poll surface are race-free under the -race
// detector (see TestConcurrentExports_NoRace).
//
// Lifecycle (mirrors the otlplogs precedent):
//   - New(t) binds 127.0.0.1:0 (ephemeral port) + spawns grpcSrv.Serve(lis) in
//     a goroutine + registers t.Cleanup(Stop). The caller reads Addr() to
//     templatize the OTLP grpc_service cluster port into the fixture configs.
//   - NewAtAddr(addr) binds the caller-supplied `host:port` (e.g.
//     "0.0.0.0:<port>" so a Docker reference-Envoy can dial the host) + spawns
//     grpcSrv.Serve(lis) in a goroutine. No t.Cleanup is registered — the
//     caller manages lifecycle via Server.Stop. Used by the differential driver.
//   - Export accumulates every Span across the ResourceSpans/ScopeSpans
//     nesting AND records each Resource.attributes set, returning an empty ack.
//   - Count()/Spans() expose the flat span accumulator: Count is the
//     converge poll, Spans is a defensive snapshot copy in arrival order.
//   - ResourceAttributes() exposes the per-ResourceSpans Resource.attributes
//     snapshots (one entry per received ResourceSpans, in arrival order).
//   - Reset() drops BOTH accumulators (per-side snapshot separation).
//   - Stop() GracefulStops the *grpc.Server; idempotent via sync.Once.
//
// Plaintext h2c — no TLS: grpc.NewServer() with no Creds() option, listener is
// plain net.Listen("tcp", ...).
type Server struct {
	coltracepb.UnimplementedTraceServiceServer

	addr    string
	lis     net.Listener
	grpcSrv *grpc.Server

	mu       sync.RWMutex
	spans    []*tracepb.Span
	resAttrs [][]*commonpb.KeyValue // per-ResourceSpans Resource.attributes, arrival order

	stopOnce sync.Once
}

// New binds an ephemeral 127.0.0.1 listener and starts a
// coltracepb.TraceServiceServer-registered *grpc.Server in a background goroutine.
// The cleanup is registered via t.Cleanup so test functions do NOT need to
// explicitly call Stop (though they may — Stop is idempotent).
//
// New uses testing.TB so it composes equally with *testing.T, *testing.B, and
// *testing.F (the fuzzer harness). For callers that need a caller-chosen
// listener address (notably the differential driver — bootstrap YAMLs bake in a
// stable OTLP-cluster endpoint port that MUST be allocated before the gRPC
// server starts), see NewAtAddr.
func New(t testing.TB) *Server {
	t.Helper()
	s, err := newServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("otlptrace: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

// NewAtAddr binds a listener on the caller-supplied `host:port` (e.g.
// "0.0.0.0:<preAllocatedPort>" so a Docker reference-Envoy can dial the host)
// and starts a coltracepb.TraceServiceServer-registered *grpc.Server in a
// background goroutine. Returns the server + nil on success, or nil + a wrapped
// net.Listen / setup error.
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
// coltracepb.TraceServiceServer impl, and spawns grpcSrv.Serve(lis) in a
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
	coltracepb.RegisterTraceServiceServer(s.grpcSrv, s)

	go func() {
		// grpc.Server.Serve returns nil on GracefulStop. Any earlier error is
		// discarded — the caller observes lifecycle via Stop's idempotent
		// registration.
		_ = s.grpcSrv.Serve(lis)
	}()

	return s, nil
}

// Export implements coltracepb.TraceServiceServer. It flattens every received
// ResourceSpans/ScopeSpans nesting, appending each Span to the accumulator
// AND recording each ResourceSpans.Resource's attribute set in arrival order,
// then returns an empty *ExportTraceServiceResponse ack.
func (s *Server) Export(_ context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	s.mu.Lock()
	for _, rs := range req.GetResourceSpans() {
		s.resAttrs = append(s.resAttrs, rs.GetResource().GetAttributes())
		for _, ss := range rs.GetScopeSpans() {
			s.spans = append(s.spans, ss.GetSpans()...)
		}
	}
	s.mu.Unlock()
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

// Spans returns a defensive snapshot copy of the accumulated *Span values in
// arrival order. The slice is freshly allocated so caller mutation cannot
// perturb the server's accumulation; the span pointers themselves are shared
// (the driver only reads them).
func (s *Server) Spans() []*tracepb.Span {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*tracepb.Span, len(s.spans))
	copy(out, s.spans)
	return out
}

// Count returns the number of accumulated spans. Load-bearing as the
// differential driver's converge poll — it spins on Count() reaching the
// expected span total before reading Spans() to assert.
func (s *Server) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.spans)
}

// ResourceAttributes returns a defensive snapshot copy of the per-ResourceSpans
// Resource.attributes sets in arrival order (one entry per received
// ResourceSpans). The outer slice is freshly allocated; the inner []*KeyValue and
// the KeyValue pointers themselves are shared (the driver only reads them).
func (s *Server) ResourceAttributes() [][]*commonpb.KeyValue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([][]*commonpb.KeyValue, len(s.resAttrs))
	copy(out, s.resAttrs)
	return out
}

// Reset drops all accumulated spans AND resource attributes. Used when a
// single server instance is reused across per-side snapshots (the driver may
// prefer a fresh server per side; Reset is the lighter-weight alternative).
//
// Call only when no Export is in flight (between per-side snapshots; server
// quiescent); a concurrent in-flight append would land after the nil and survive
// the reset. It is mutex-guarded so memory-safe, but the quiescence contract is
// the caller's to uphold.
func (s *Server) Reset() {
	s.mu.Lock()
	s.spans = nil
	s.resAttrs = nil
	s.mu.Unlock()
}

// Addr returns the listener's bound `host:port` string. Load-bearing when New
// allocated an ephemeral port — the fixture driver reads this back to
// templatize the OTLP grpc_service cluster endpoint into the bootstraps.
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
// (e.g. the differential driver) that have already snapshotted spans and want
// deterministic teardown without waiting on still-open proxy streams. It calls
// grpc.Server.Stop (cancels in-flight calls, returns at once) and shares the
// same sync.Once as Stop so the two are idempotent and mutually exclusive.
func (s *Server) Close() {
	s.stopOnce.Do(s.grpcSrv.Stop)
}
