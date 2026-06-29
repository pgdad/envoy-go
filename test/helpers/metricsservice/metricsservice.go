// Package metricsservice provides a minimal in-process MetricsService gRPC
// server for tests and differential fixtures. It is the metrics_service
// counterpart of the otlptrace helper (the OTLP-trace receiver): a driver-owned
// receiver that the proxy DIALS, so per project convention it is a test helper
// rather than a runner BackendKind (D-MS-RECEIVER-WIRING; BackendKind STAYS 38).
package metricsservice

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
)

// familyValue is the accumulated last-seen value + type for a single metric
// family NAME. The metrics_service mapping is cumulative/no-labels, so each
// family carries one Metric whose Counter/Gauge value is the absolute reading.
type familyValue struct {
	value float64
	typ   dto.MetricType
}

// Server is a minimal in-process MetricsService gRPC server that accumulates the
// last-seen value (+ type) of every received MetricFamily keyed by its full
// dotted name across a StreamMetrics client stream, and captures the first
// message's Identifier.Node. Goroutine-safe: the accumulator map and the node
// pointer are guarded by an RWMutex so the StreamMetrics ingest path and the
// Count/Family/Node poll surface are race-free under the -race detector.
//
// Lifecycle (mirrors the otlptrace precedent):
//   - New(t) binds 127.0.0.1:0 (ephemeral port) + spawns grpcSrv.Serve(lis) in
//     a goroutine + registers t.Cleanup(Stop). The caller reads Addr() to
//     templatize the metrics_service grpc_service cluster port into the fixtures.
//   - NewAtAddr(addr) binds the caller-supplied `host:port` (e.g.
//     "0.0.0.0:<port>" so a Docker reference-Envoy can dial the host) + spawns
//     grpcSrv.Serve(lis) in a goroutine. No t.Cleanup is registered — the
//     caller manages lifecycle via Server.Stop. Used by the differential driver.
//   - StreamMetrics accumulates each MetricFamily by name (last-seen wins) and
//     captures the first message's Identifier.Node, acking on io.EOF.
//   - Count() is the number of distinct family NAMES accumulated (the converge
//     poll); Family(name) reads back one family's (value, type, ok).
//   - FamilySum(name) reads back the running SUM of every received value for a
//     family (additive to Family's last-seen) — for a delta sink the per-flush
//     counter deltas sum to the cumulative total (the 0090 convergence invariant).
//   - Node() returns the captured Identifier.Node (nil before any message).
//   - Messages() is the total StreamMetricsMessages received across the stream
//     (the 0090 delta stability barrier: wait for N further flushes/messages
//     after convergence to confirm the idle delta-SUM stays == K).
//   - Reset() drops the accumulator AND the captured node (per-side separation).
//   - Stop() GracefulStops the *grpc.Server; idempotent via sync.Once.
//
// Plaintext h2c — no TLS: grpc.NewServer() with no Creds() option, listener is
// plain net.Listen("tcp", ...).
type Server struct {
	metricsv3.UnimplementedMetricsServiceServer

	addr    string
	lis     net.Listener
	grpcSrv *grpc.Server

	mu       sync.RWMutex
	fams     map[string]familyValue
	sums     map[string]float64
	node     *corev3.Node
	msgCount int

	stopOnce sync.Once
}

// New binds an ephemeral 127.0.0.1 listener and starts a
// metricsv3.MetricsServiceServer-registered *grpc.Server in a background
// goroutine. The cleanup is registered via t.Cleanup so test functions do NOT
// need to explicitly call Stop (though they may — Stop is idempotent).
//
// New uses testing.TB so it composes equally with *testing.T, *testing.B, and
// *testing.F. For callers that need a caller-chosen listener address (notably
// the differential driver — bootstrap YAMLs bake in a stable metrics_service
// cluster endpoint port that MUST be allocated before the server starts), see
// NewAtAddr.
func New(t testing.TB) *Server {
	t.Helper()
	s, err := newServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("metricsservice: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

// NewAtAddr binds a listener on the caller-supplied `host:port` (e.g.
// "0.0.0.0:<preAllocatedPort>" so a Docker reference-Envoy can dial the host)
// and starts a metricsv3.MetricsServiceServer-registered *grpc.Server in a
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
// metricsv3.MetricsServiceServer impl, and spawns grpcSrv.Serve(lis) in a
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
		fams:    make(map[string]familyValue),
		sums:    make(map[string]float64),
	}
	metricsv3.RegisterMetricsServiceServer(s.grpcSrv, s)

	go func() {
		// grpc.Server.Serve returns nil on GracefulStop. Any earlier error is
		// discarded — the caller observes lifecycle via Stop's idempotent
		// registration.
		_ = s.grpcSrv.Serve(lis)
	}()

	return s, nil
}

// StreamMetrics implements metricsv3.MetricsServiceServer. It drains the client
// stream: on the first message it captures Identifier.Node, and for every
// message it stores each EnvoyMetrics MetricFamily by name (last-seen value +
// type wins) under the mutex. On io.EOF it acks with an empty
// StreamMetricsResponse; any other Recv error is returned verbatim.
func (s *Server) StreamMetrics(stream metricsv3.MetricsService_StreamMetricsServer) error {
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&metricsv3.StreamMetricsResponse{})
		}
		if err != nil {
			return err
		}

		s.mu.Lock()
		s.msgCount++
		if n := msg.GetIdentifier().GetNode(); n != nil && s.node == nil {
			s.node = n
		}
		for _, f := range msg.GetEnvoyMetrics() {
			var value float64
			if ms := f.GetMetric(); len(ms) > 0 {
				switch f.GetType() {
				case dto.MetricType_GAUGE:
					value = ms[0].GetGauge().GetValue()
				default:
					value = ms[0].GetCounter().GetValue()
				}
			}
			s.fams[f.GetName()] = familyValue{value: value, typ: f.GetType()}
			s.sums[f.GetName()] += value
		}
		s.mu.Unlock()
	}
}

// Family returns the last-seen (value, type, ok) for the accumulated MetricFamily
// named `name`. ok is false if no family with that name has been received. The
// type is the dto.MetricType so the differential can assert COUNTER vs GAUGE.
func (s *Server) Family(name string) (value float64, typ dto.MetricType, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fv, ok := s.fams[name]
	return fv.value, fv.typ, ok
}

// FamilySum returns the running SUM of every received value for the family named
// `name` across all messages, and ok=false if none was received. For a delta sink
// (report_counters_as_deltas=true) the per-flush counter deltas sum to the
// cumulative total (== K after K requests) — the 0090 convergence invariant
// (AMEND-MSD-SUM-IS-THE-INVARIANT). Additive to Family() (last-seen), which 0089
// keeps.
func (s *Server) FamilySum(name string) (sum float64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sum, ok = s.sums[name]
	return
}

// Count returns the number of distinct family NAMES accumulated. Load-bearing as
// the differential driver's converge poll — a second message updating an
// existing name keeps Count unchanged.
func (s *Server) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.fams)
}

// Messages returns the total number of StreamMetricsMessages received across the
// stream. Load-bearing for the 0090 delta stability barrier: a differential driver
// can wait for N further flushes (messages) to arrive after convergence to confirm
// the delta-SUM is stable (idle flushes add 0 under report_counters_as_deltas).
func (s *Server) Messages() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.msgCount
}

// Node returns the Identifier.Node captured from the first message that carried
// one. nil before any identifier-bearing message has been received. The pointer
// is shared (the driver only reads it).
func (s *Server) Node() *corev3.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.node
}

// Reset drops all accumulated families AND the captured node. Used when a single
// server instance is reused across per-side snapshots (the driver may prefer a
// fresh server per side; Reset is the lighter-weight alternative).
//
// Call only when no StreamMetrics ingest is in flight (between per-side
// snapshots; server quiescent); a concurrent in-flight store would land after
// the clear and survive the reset. It is mutex-guarded so memory-safe, but the
// quiescence contract is the caller's to uphold.
func (s *Server) Reset() {
	s.mu.Lock()
	s.fams = make(map[string]familyValue)
	s.sums = make(map[string]float64)
	s.node = nil
	s.msgCount = 0
	s.mu.Unlock()
}

// Addr returns the listener's bound `host:port` string. Load-bearing when New
// allocated an ephemeral port — the fixture driver reads this back to
// templatize the metrics_service grpc_service cluster endpoint into the
// bootstraps.
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
// (e.g. the differential driver) that have already snapshotted the accumulated
// families and want deterministic teardown without waiting on still-open proxy
// StreamMetrics streams. Unlike OTLP Export (unary) or Zipkin (HTTP), the
// metrics_service StreamMetrics RPC is long-lived — both proxies keep it open
// and flush every stats_flush_interval — so GracefulStop would block
// indefinitely waiting for those streams to drain. Close calls grpc.Server.Stop
// (cancels in-flight calls, returns at once) and shares the same sync.Once as
// Stop so the two are idempotent and mutually exclusive.
func (s *Server) Close() {
	s.stopOnce.Do(s.grpcSrv.Stop)
}
