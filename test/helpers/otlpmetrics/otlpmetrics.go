// Package otlpmetrics provides a minimal in-process OTLP MetricsService gRPC
// server for tests and differential fixtures. It is the OTLP-metrics
// counterpart of the otlptrace helper (the OTLP trace receiver): a
// driver-owned receiver that the proxy DIALS, so per project convention it is
// a test helper rather than a runner BackendKind
// (reference_differential_grpc_receiver_driver_owned; BackendKind STAYS 38).
package otlpmetrics

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"testing"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
)

// datapoint is the latest recorded state for one (metric-name, attribute-set)
// key. The Sum-only fields (temporality, isMonotonic, startTime) are
// zero-valued when isSum is false (a Gauge datapoint). name and attrs are
// retained so the attribute-qualified accessor surface (Datapoints) can return
// the FULL identity of every accumulated datapoint — the 0112/0113 differential
// fixtures need this to (a) disambiguate the residual-name COLLISION where the
// application backend cluster AND the OTLP sink's OWN gRPC cluster both emit the
// tag-extracted residual `cluster.upstream_rq_total` (differing only in the
// envoy.cluster_name attribute) and (b) assert the emitted envoy.<tag> attribute
// SET on a datapoint (order-insensitive).
type datapoint struct {
	name        string
	attrs       []*commonpb.KeyValue
	value       float64
	isSum       bool
	temporality metricspb.AggregationTemporality
	isMonotonic bool
	startTime   uint64
}

// DatapointView is an exported read-only snapshot of one accumulated datapoint's
// full identity (name + attribute set + value + Sum metadata). Returned by
// Datapoints so a differential driver can select a datapoint by an attribute
// (collision disambiguation) and assert its attribute set (order-insensitive) —
// beyond what the name-only latest-write-wins accessors (SumValue, …) expose.
type DatapointView struct {
	Name        string
	Attrs       []*commonpb.KeyValue
	Value       float64
	IsSum       bool
	Temporality metricspb.AggregationTemporality
	IsMonotonic bool
	StartTime   uint64
}

// Server is a minimal in-process OTLP MetricsService gRPC server that
// accumulates datapoints across unary Export calls (flattening the
// ResourceMetrics/ScopeMetrics/Metric nesting) AND records each
// ResourceMetrics.Resource's attribute set in arrival order. Goroutine-safe:
// the accumulator maps/slices are guarded by an RWMutex so the Export append
// path and the accessor poll surface are race-free under the -race detector.
//
// Keying (the order-insensitive property the 0112/0113 differential fixtures'
// non-break control depends on): each datapoint is recorded under a key of
// (metric-name, SORTED attribute k=v pairs) — sorting makes the key
// insensitive to the wire-order the reference/subject happen to emit
// attributes in (see Break K in otlpmetrics_test.go, which un-sorts the key
// and proves the reversed-attrs self-test assertion actually fires).
//
// Accessor surface (name-keyed, latest-write-wins via byName): SumValue,
// GaugeValue, Temporality, IsMonotonic, StartTime all resolve the MOST
// RECENTLY WRITTEN datapoint for a given metric name — the differential
// fixtures emit at most one attribute-set per name, so "latest write for this
// name" and "latest write for this (name,attrs) key" coincide in practice.
// DeltaSum is a SEPARATE accumulator, summing every Sum datapoint value ever
// recorded for a name across ALL Exports (the 0113 delta-mode stability
// barrier: after the running sum reaches K, further idle flushes must not
// move it further).
//
// Lifecycle (mirrors the otlptrace precedent):
//   - New(t) binds 127.0.0.1:0 (ephemeral port) + spawns grpcSrv.Serve(lis) in
//     a goroutine + registers t.Cleanup(Stop).
//   - NewAtAddr(addr) binds the caller-supplied `host:port` for a differential
//     driver that pre-allocates a stable port. No t.Cleanup is registered.
//   - Export accumulates every datapoint + records the Resource.attributes
//     set, returning an empty ack. Tolerates a present-but-EMPTY Scope (the T2
//     sink's shape: `Scope: &InstrumentationScope{}`) — flattening never reads
//     Scope fields, so a nil OR empty Scope is handled identically.
//   - Reset() drops ALL accumulators (per-side snapshot separation).
//   - Stop() GracefulStops; Close() hard-Stops. Both idempotent via one
//     shared sync.Once.
//
// Plaintext h2c — no TLS: grpc.NewServer() with no Creds() option, listener is
// plain net.Listen("tcp", ...).
type Server struct {
	colmetricspb.UnimplementedMetricsServiceServer

	addr    string
	lis     net.Listener
	grpcSrv *grpc.Server

	mu          sync.RWMutex
	latest      map[string]datapoint   // key: datapointKey(name, attrs) -> latest recorded datapoint
	byName      map[string]string      // metric name -> the most-recently-written key above
	deltaSum    map[string]float64     // metric name -> running sum of every Sum datapoint value ever recorded
	resAttrs    [][]*commonpb.KeyValue // per-ResourceMetrics Resource.attributes, arrival order
	exportCount int                    // number of Export unary calls received (served-this-arm precondition)

	stopOnce sync.Once
}

// New binds an ephemeral 127.0.0.1 listener and starts a
// colmetricspb.MetricsServiceServer-registered *grpc.Server in a background
// goroutine. The cleanup is registered via t.Cleanup so test functions do NOT
// need to explicitly call Stop (though they may — Stop is idempotent).
//
// New uses testing.TB so it composes equally with *testing.T, *testing.B, and
// *testing.F. For callers that need a caller-chosen listener address (the
// differential driver bakes a stable OTLP-cluster endpoint port into the
// bootstrap YAMLs before the gRPC server starts), see NewAtAddr.
func New(t testing.TB) *Server {
	t.Helper()
	s, err := newServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("otlpmetrics: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

// NewAtAddr binds a listener on the caller-supplied `host:port` (e.g.
// "0.0.0.0:<preAllocatedPort>" so a Docker reference-Envoy can dial the host)
// and starts a colmetricspb.MetricsServiceServer-registered *grpc.Server in a
// background goroutine. Returns the server + nil on success, or nil + a
// wrapped net.Listen / setup error.
//
// Lifecycle management is the CALLER's responsibility — there is no t.Cleanup
// registration. Tests that want auto-cleanup should use New(t) instead.
func NewAtAddr(addr string) (*Server, error) {
	return newServer(addr)
}

// newServer is the shared constructor body used by both New(t) and
// NewAtAddr(addr).
func newServer(addr string) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	s := &Server{
		addr:     lis.Addr().String(),
		lis:      lis,
		grpcSrv:  grpc.NewServer(),
		latest:   make(map[string]datapoint),
		byName:   make(map[string]string),
		deltaSum: make(map[string]float64),
	}
	colmetricspb.RegisterMetricsServiceServer(s.grpcSrv, s)

	go func() {
		// grpc.Server.Serve returns nil on GracefulStop. Any earlier error is
		// discarded — the caller observes lifecycle via Stop's idempotent
		// registration.
		_ = s.grpcSrv.Serve(lis)
	}()

	return s, nil
}

// Export implements colmetricspb.MetricsServiceServer. It flattens every
// received ResourceMetrics/ScopeMetrics/Metric nesting, recording each Sum or
// Gauge datapoint under its (name, sorted-attrs) key AND recording each
// ResourceMetrics.Resource's attribute set in arrival order, then returns an
// empty *ExportMetricsServiceResponse ack.
//
// A present-but-empty Scope (ScopeMetrics.Scope == &InstrumentationScope{},
// the T2 sink's shape) is handled identically to a nil Scope — flattening
// never reads Scope fields at all, so there is nothing to tolerate beyond not
// dereferencing it.
//
// Histogram/ExponentialHistogram/Summary metrics are skipped defensively
// (never produced by the T2 sink — SN7 only emits COUNTER/GAUGE families).
func (s *Server) Export(_ context.Context, req *colmetricspb.ExportMetricsServiceRequest) (*colmetricspb.ExportMetricsServiceResponse, error) {
	s.mu.Lock()
	s.exportCount++
	for _, rm := range req.GetResourceMetrics() {
		s.resAttrs = append(s.resAttrs, rm.GetResource().GetAttributes())
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				name := m.GetName()
				switch {
				case m.GetSum() != nil:
					sum := m.GetSum()
					for _, dp := range sum.GetDataPoints() {
						attrs := dp.GetAttributes()
						key := datapointKey(name, attrs)
						v := numberValue(dp)
						s.latest[key] = datapoint{
							name:        name,
							attrs:       attrs,
							value:       v,
							isSum:       true,
							temporality: sum.GetAggregationTemporality(),
							isMonotonic: sum.GetIsMonotonic(),
							startTime:   dp.GetStartTimeUnixNano(),
						}
						s.byName[name] = key
						s.deltaSum[name] += v
					}
				case m.GetGauge() != nil:
					for _, dp := range m.GetGauge().GetDataPoints() {
						attrs := dp.GetAttributes()
						key := datapointKey(name, attrs)
						s.latest[key] = datapoint{name: name, attrs: attrs, value: numberValue(dp)}
						s.byName[name] = key
					}
				default:
					// Histogram/ExponentialHistogram/Summary/no-data: skip
					// defensively rather than record a meaningless entry.
				}
			}
		}
	}
	s.mu.Unlock()
	return &colmetricspb.ExportMetricsServiceResponse{}, nil
}

// datapointKey builds the order-insensitive accumulation key for one
// datapoint: the metric name plus its attribute set rendered as SORTED
// "k=v" pairs. Sorting is what makes the key insensitive to wire-order (Break
// K un-sorts this and proves the self-test's reversed-attrs assertion fires).
func datapointKey(name string, attrs []*commonpb.KeyValue) string {
	return name + "\x00" + attrKey(attrs)
}

// attrKey renders attrs as a sorted, comma-joined "k=v" string. Empty/nil
// attrs render as the empty string, so a no-attribute datapoint keys purely
// on name.
func attrKey(attrs []*commonpb.KeyValue) string {
	if len(attrs) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(attrs))
	for _, kv := range attrs {
		pairs = append(pairs, kv.GetKey()+"="+anyValueString(kv.GetValue()))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

// anyValueString renders an AnyValue as a string for key-building purposes.
// The T2 sink emits StringValue exclusively (otlpKV); the fallback below is
// purely defensive (never exercised by the T2 sink today).
func anyValueString(v *commonpb.AnyValue) string {
	if v == nil {
		return ""
	}
	if s, ok := v.GetValue().(*commonpb.AnyValue_StringValue); ok {
		return s.StringValue
	}
	return fmt.Sprintf("%v", v.GetValue())
}

// numberValue reads a NumberDataPoint's oneof value regardless of whether it
// was set via AsDouble or AsInt (the T2 sink always uses AsDouble, but this
// stays correct for any conforming producer).
func numberValue(dp *metricspb.NumberDataPoint) float64 {
	switch v := dp.GetValue().(type) {
	case *metricspb.NumberDataPoint_AsDouble:
		return v.AsDouble
	case *metricspb.NumberDataPoint_AsInt:
		return float64(v.AsInt)
	default:
		return 0
	}
}

// lookup returns the latest datapoint recorded for name (resolved via
// byName) and whether one was found. Caller must hold s.mu (R or W).
func (s *Server) lookup(name string) (datapoint, bool) {
	key, ok := s.byName[name]
	if !ok {
		return datapoint{}, false
	}
	dp, ok := s.latest[key]
	return dp, ok
}

// SumValue returns the latest recorded value for the Sum-typed metric name,
// and whether one was found (false if name is unknown OR its latest entry is
// Gauge-typed).
func (s *Server) SumValue(name string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dp, ok := s.lookup(name)
	if !ok || !dp.isSum {
		return 0, false
	}
	return dp.value, true
}

// GaugeValue returns the latest recorded value for the Gauge-typed metric
// name, and whether one was found (false if name is unknown OR its latest
// entry is Sum-typed).
func (s *Server) GaugeValue(name string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dp, ok := s.lookup(name)
	if !ok || dp.isSum {
		return 0, false
	}
	return dp.value, true
}

// Temporality returns the latest recorded Sum's AggregationTemporality for
// name, and whether one was found (false if name is unknown OR Gauge-typed).
func (s *Server) Temporality(name string) (metricspb.AggregationTemporality, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dp, ok := s.lookup(name)
	if !ok || !dp.isSum {
		return 0, false
	}
	return dp.temporality, true
}

// IsMonotonic returns the latest recorded Sum's IsMonotonic flag for name,
// and whether one was found (false if name is unknown OR Gauge-typed).
func (s *Server) IsMonotonic(name string) (bool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dp, ok := s.lookup(name)
	if !ok || !dp.isSum {
		return false, false
	}
	return dp.isMonotonic, true
}

// StartTime returns the latest recorded Sum datapoint's StartTimeUnixNano for
// name, and whether one was found (false if name is unknown OR Gauge-typed —
// gauges carry no StartTime per the T2 sink's shape).
func (s *Server) StartTime(name string) (uint64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dp, ok := s.lookup(name)
	if !ok || !dp.isSum {
		return 0, false
	}
	return dp.startTime, true
}

// DeltaSum returns the running sum of every Sum datapoint value ever recorded
// for name across ALL Export calls (NOT just the latest), and whether any
// were recorded. This is the accessor the 0113 delta-mode differential
// fixture's post-convergence stability barrier reads: once the running sum
// reaches the expected total K, further idle flushes (report_counters_as_deltas
// emitting zero-valued deltas) must leave DeltaSum unchanged.
func (s *Server) DeltaSum(name string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sum, ok := s.deltaSum[name]
	return sum, ok
}

// ResourceAttributes returns a defensive snapshot copy of the per-ResourceMetrics
// Resource.attributes sets in arrival order (one entry per received
// ResourceMetrics). The outer slice is freshly allocated; the inner
// []*KeyValue and the KeyValue pointers themselves are shared (the driver only
// reads them).
func (s *Server) ResourceAttributes() [][]*commonpb.KeyValue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([][]*commonpb.KeyValue, len(s.resAttrs))
	copy(out, s.resAttrs)
	return out
}

// Datapoints returns a defensive snapshot of EVERY distinct accumulated
// (name, sorted-attrs) datapoint — not just the latest per name — so a caller
// can select a datapoint by an attribute value (residual-name collision
// disambiguation) and assert its emitted attribute set (order-insensitive). The
// returned slice + views are freshly allocated; the KeyValue pointers inside
// Attrs are shared (the caller only reads them). Iteration order is
// unspecified (map order).
func (s *Server) Datapoints() []DatapointView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DatapointView, 0, len(s.latest))
	for _, dp := range s.latest {
		out = append(out, DatapointView{
			Name:        dp.name,
			Attrs:       dp.attrs,
			Value:       dp.value,
			IsSum:       dp.isSum,
			Temporality: dp.temporality,
			IsMonotonic: dp.isMonotonic,
			StartTime:   dp.startTime,
		})
	}
	return out
}

// ExportCount returns the number of unary Export calls received since the last
// Reset. A differential driver polls this for the served-this-arm precondition
// (a zero-Export pass is a false green) and for a stability barrier that waits
// for ≥N further flushes (e.g. asserting a cumulative StartTime stays constant
// across flushes).
func (s *Server) ExportCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.exportCount
}

// Reset drops ALL accumulated state: datapoints, the name index, the
// delta-sum accumulator, resource attributes, and the Export counter. Used
// when a single server instance is reused across per-side snapshots.
//
// Call only when no Export is in flight (between per-side snapshots; server
// quiescent); a concurrent in-flight append would land after the reset and
// survive it. It is mutex-guarded so memory-safe, but the quiescence contract
// is the caller's to uphold.
func (s *Server) Reset() {
	s.mu.Lock()
	s.latest = make(map[string]datapoint)
	s.byName = make(map[string]string)
	s.deltaSum = make(map[string]float64)
	s.resAttrs = nil
	s.exportCount = 0
	s.mu.Unlock()
}

// Addr returns the listener's bound `host:port` string. Load-bearing when New
// allocated an ephemeral port — the fixture driver reads this back to
// templatize the OTLP grpc_service cluster endpoint into the bootstraps.
func (s *Server) Addr() string {
	return s.addr
}

// Stop GracefulStops the *grpc.Server. Idempotent via sync.Once: multiple
// calls are no-ops after the first. Registered as t.Cleanup at New time, so
// callers typically do NOT need to invoke Stop explicitly.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		s.grpcSrv.GracefulStop()
	})
}

// Close is the immediate hard-stop variant of Stop for callers (e.g. the
// differential driver) that have already snapshotted datapoints and want
// deterministic teardown without waiting on still-open proxy streams. Export
// is unary here (no long-lived streams), so Close and Stop differ only in
// whether in-flight calls are canceled (Close) or allowed to finish
// (Stop) — both share the same sync.Once so they are idempotent and mutually
// exclusive.
func (s *Server) Close() {
	s.stopOnce.Do(s.grpcSrv.Stop)
}
