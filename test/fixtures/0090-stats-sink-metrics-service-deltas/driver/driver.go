// Package driver registers the 0090-stats-sink-metrics-service-deltas fixture
// with the differential runner. It is the behavioral proof of the phase 47.2a
// report_counters_as_deltas knob on the landed 47.1 metrics_service stats sink:
// cross-side EXACT (subject envoy-go vs reference Envoy contrib-v1.37.2 in
// Docker) on a deterministic COUNTER NAME SUBSET, asserting the per-flush DELTA
// model.
//
// Delta-SUM model (the key departure from 0089's last-seen value==K):
// under report_counters_as_deltas:true each COUNTER family carries the per-flush
// DELTA (the increment since the previous flush), NOT the cumulative absolute
// value. A single flush is partial; the LAST idle flush reads ≈0 — so 0089's
// Family(name).value==K (last-seen) is meaningless here. Instead a counter's
// per-flush deltas SUM across flushes to the cumulative total (== K after K 2xx
// requests). 0090 therefore asserts FamilySum(name) == K
// (AMEND-MSD-SUM-IS-THE-INVARIANT). The same 3-counter subset as 0089 is used;
// upstream_cx_total is excluded because it sums to < K under connection reuse
// (AMEND-MSD-CX-NOT-K). Gauges stay ABSOLUTE under the deltas knob and are
// unasserted; message/stream framing is unasserted.
//
// Integration shape (single-listener plaintext H1; driver-owned in-process
// MetricsService gRPC receiver; HTTPFixedBody backend):
//
//  1. TWO driver-owned MetricsService receivers — one per side — on two
//     separately-allocated host ports (refMetricsPort / subjMetricsPort), both
//     bound on 0.0.0.0 BEFORE either proxy starts. This is the key departure
//     from the event-driven OTLP/Zipkin trace fixtures (0087/0088): the
//     metrics_service sink flushes PERIODICALLY (every stats_flush_interval),
//     so the reference proxy keeps streaming into its receiver for the whole
//     test — including during the subject's drive window. A single shared
//     receiver would let the reference's concurrent flushes contaminate the
//     subject snapshot (and silently defeat subject-side deliberate breaks). Two
//     receivers give each side a private, uncontaminated accumulator, so no
//     Reset() between sides is needed and the per-side assertions are strict.
//     ReferenceBootstrap renders envoy.yaml pointing the metrics cluster at
//     host.docker.internal:refMetricsPort (ADR-0010 STRICT_DNS bridge alias);
//     SubjectConfig renders envoy-go.yaml pointing at 127.0.0.1:subjMetricsPort.
//     BOTH bootstraps carry a bootstrap-level stats_sinks[] metrics_service
//     entry (transport_api_version V3, report_counters_as_deltas:true) naming an
//     h2c metrics cluster + a SHORT stats_flush_interval (500ms) for fast
//     deterministic convergence + a node{id,cluster} fixed identically on both
//     sides.
//
//  2. DriveReference / DriveSubject each fire K=7 deterministic GET requests
//     (all 2xx) through the proxy listener, then POLL that side's receiver until
//     the deterministic COUNTER subset's delta-SUM converges to == K AND the
//     identifier node has arrived — a release barrier
//     (reference_concurrency_differential_release_barrier; the proxy flushes the
//     per-flush counter delta every stats_flush_interval; poll not sleep). The
//     per-side family snapshot (the three subset delta-SUMs + the captured
//     identifier node id/cluster) is captured from that side's own receiver
//     before the next side runs.
//
//  3. AssertStats asserts, on BOTH sides: the deterministic COUNTER NAME SUBSET
//     {cluster.c_backend.upstream_rq_total, http.hcm_local.downstream_rq_total,
//     http.hcm_local.downstream_rq_2xx} present with type==COUNTER and
//     delta-SUM==K; the identifier node.id/.cluster == the configured values.
//     PLUS decode-ran (Count() > 0) on each side before asserting.
//
// Decode-ran proof: the poll-to-converge guarantees the subset families arrived
// (delta-SUM == K) on each side's own receiver before asserting — a zero-family
// pass is structurally impossible.
//
// UNasserted (AMEND-MS-HISTOGRAM-PRESENT + reference_streaming_sink_differential_framing):
// the WHOLE family set / family count (the surfaces differ cross-side — envoy-go
// has no histograms, the reference does); metric[].timestamp_ms (value
// non-deterministic; presence-only); help; ABSOLUTE gauges (server.uptime,
// *_active, connection churn — gauges stay absolute under the deltas knob); the
// identifier user_agent_*/extensions[]; message/stream framing + per-message
// family count.
package driver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"text/template"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
	"github.com/esalaine/envoy-go/test/helpers/metricsservice"
)

const (
	fixtureName = "0090-stats-sink-metrics-service-deltas"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0090 takes 10090 for the single plaintext listener.
	refAdminPort    = 9901
	refListenerPort = 10090

	// numReq is the per-side request count K. After K 2xx requests the
	// deterministic COUNTER subset's per-flush deltas SUM to == K on each side.
	numReq = 7

	// The fixed request shape (kept identical cross-side; query-less path).
	probePath = "/probe"
	probeHost = "metrics.example"
	probeUA   = "metrics-probe/1"

	// The HCM stat_prefix + backend cluster name baked IDENTICALLY into both
	// bootstraps, so the mapped dotted stat names match cross-side.
	statPrefix  = "hcm_local"
	backendName = "c_backend"

	// The bootstrap node id/cluster baked identically into both bootstraps →
	// the StreamMetricsMessage.identifier.node (msg #1) — cross-side assertable.
	wantNodeID      = "envoy-go-subject-0090"
	wantNodeCluster = "envoy-go-differential"

	// Converge-poll discipline (reference_concurrency_differential_release_barrier):
	// POLL the subset FamilySum() delta-sums to == K; never sleep-to-wait.
	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)

// subsetNames is the deterministic COUNTER name subset asserted cross-side. All
// three are COUNTERs whose per-flush deltas SUM to exactly numReq after K 2xx
// requests on BOTH the reference and the subject (cross-side name-equality +
// post-K determinism).
var subsetNames = []string{
	"cluster." + backendName + ".upstream_rq_total",
	"http." + statPrefix + ".downstream_rq_total",
	"http." + statPrefix + ".downstream_rq_2xx",
}

func init() {
	fixture.RegisterFixture(fixtureName, &metricsSinkDriver{})
}

// sideSnapshot is the per-side captured state: the three subset family
// (delta-sum, type) readings + the captured identifier node id/cluster.
type sideSnapshot struct {
	fams        map[string]familyReading
	nodeID      string
	nodeCluster string
}

type familyReading struct {
	sum float64
	typ dto.MetricType
	ok  bool
}

// metricsSinkDriver carries the per-driver lifecycle state — TWO private metrics
// receivers (one per side; the reference's periodic flushes must not contaminate
// the subject snapshot) on two separately-allocated host ports, plus the per-side
// snapshots captured during Drive for the AssertStats cross-side assertion.
type metricsSinkDriver struct {
	mu sync.Mutex

	refMetricsPort  int
	subjMetricsPort int
	refSrv          *metricsservice.Server
	subjSrv         *metricsservice.Server

	ref  sideSnapshot
	subj sideSnapshot
}

// ensure allocates the two receiver ports (via Listen+Close) and starts both
// in-process MetricsService receivers bound to 0.0.0.0:<port> (so BOTH the
// reference container via host.docker.internal AND the subject via 127.0.0.1 can
// dial their respective receiver). Idempotent — safe to call from both
// ReferenceBootstrap and SubjectConfig regardless of order; a second call is a
// no-op while the servers run. Both servers are live before either proxy opens
// its StreamMetrics stream.
func (d *metricsSinkDriver) ensure() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.refMetricsPort == 0 {
		d.refMetricsPort = mustAllocatePort()
	}
	if d.subjMetricsPort == 0 {
		d.subjMetricsPort = mustAllocatePort()
	}
	if d.refSrv == nil {
		d.refSrv = mustStartReceiver(d.refMetricsPort)
	}
	if d.subjSrv == nil {
		d.subjSrv = mustStartReceiver(d.subjMetricsPort)
	}
}

// mustAllocatePort reserves a free TCP port via Listen+Close. Mirrors the 0087
// allocateOTLPPort idiom.
func mustAllocatePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate metrics port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// mustStartReceiver starts an in-process MetricsService receiver bound to
// 0.0.0.0:<port>.
func mustStartReceiver(port int) *metricsservice.Server {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	srv, err := metricsservice.NewAtAddr(addr)
	if err != nil {
		panic(fmt.Sprintf("driver: start metrics receiver on %s: %v", addr, err))
	}
	return srv
}

// --- fixture.Driver (required) ---

func (*metricsSinkDriver) BackendCount() int                { return 1 }
func (*metricsSinkDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }
func (*metricsSinkDriver) SubjectListenerName() string      { return "l_test" }
func (*metricsSinkDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// runner-allocated backend port + the reference-side metrics receiver host:port
// (host=host.docker.internal). It allocates the ports and starts both receivers
// here so they are live before the reference container boots.
func (d *metricsSinkDriver) ReferenceBootstrap(backendPorts []int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    refAdminPort,
		"ListenerPort": refListenerPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
		"MetricsHost":  "host.docker.internal",
		"MetricsPort":  d.refMetricsPort,
		"NodeID":       wantNodeID,
		"NodeCluster":  wantNodeCluster,
		"StatPrefix":   statPrefix,
		"BackendName":  backendName,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
// + backend port (loopback) + the subject-side metrics receiver port
// (host=127.0.0.1).
func (d *metricsSinkDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
		"MetricsPort":  d.subjMetricsPort,
		"NodeID":       wantNodeID,
		"NodeCluster":  wantNodeCluster,
		"StatPrefix":   statPrefix,
		"BackendName":  backendName,
	})
}

// DriveReference fires the workload against the reference proxy and snapshots the
// reference-side subset family readings + identifier node from the reference's
// own private receiver.
func (d *metricsSinkDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	srv := d.refSrv
	d.mu.Unlock()
	out, snap, err := d.driveSide(ctx, addr, srv)
	if err != nil {
		return nil, err
	}
	d.ref = snap
	return out, nil
}

// DriveSubject fires the workload against the subject proxy and snapshots the
// subject-side subset family readings + identifier node from the subject's own
// private receiver. After the subject snapshot BOTH receivers are hard-stopped
// via Close() for deterministic teardown — Close (grpc.Server.Stop), NOT Stop
// (GracefulStop): both proxies hold their long-lived StreamMetrics streams open,
// so GracefulStop would block until the test timeout (the 0087 srv.Close()
// precedent). The readings are already snapshotted so canceling the streams
// loses nothing.
func (d *metricsSinkDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	srv := d.subjSrv
	d.mu.Unlock()
	out, snap, err := d.driveSide(ctx, addr, srv)
	if err != nil {
		return nil, err
	}
	d.subj = snap
	d.closeServers()
	return out, nil
}

// closeServers hard-stops both receivers (idempotent). Called after the subject
// snapshot for deterministic teardown.
func (d *metricsSinkDriver) closeServers() {
	d.mu.Lock()
	ref, subj := d.refSrv, d.subjSrv
	d.refSrv, d.subjSrv = nil, nil
	d.mu.Unlock()
	if ref != nil {
		ref.Close()
	}
	if subj != nil {
		subj.Close()
	}
}

// driveSide fires numReq deterministic GET requests against the proxy listener
// at addr (all 2xx), polls that side's private receiver until the deterministic
// COUNTER subset's delta-SUM converges to == numReq AND the identifier node has
// arrived, and snapshots the subset readings + the captured identifier node. No
// Reset() is needed: each side owns a private receiver, so its accumulator is
// exclusively that side's data even though the other proxy keeps streaming into
// ITS receiver.
func (d *metricsSinkDriver) driveSide(ctx context.Context, addr string, srv *metricsservice.Server) ([]byte, sideSnapshot, error) {
	if srv == nil {
		return nil, sideSnapshot{}, fmt.Errorf("driver: metrics receiver not running")
	}

	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	var b bytes.Buffer
	for i := 0; i < numReq; i++ {
		code, err := d.fireProbe(ctx, client, addr)
		if err != nil {
			return nil, sideSnapshot{}, fmt.Errorf("request %d: %w", i, err)
		}
		if code < 200 || code >= 300 {
			return nil, sideSnapshot{}, fmt.Errorf("request %d: non-2xx status %d", i, code)
		}
		fmt.Fprintf(&b, "status=%d\n", code)
	}

	if err := pollSubset(ctx, srv); err != nil {
		return nil, sideSnapshot{}, err
	}

	// Post-convergence stability barrier — the delta invariant that distinguishes
	// report_counters_as_deltas from absolute: once each subset counter's per-flush
	// delta-SUM reaches numReq, observe >=2 further flushes and snapshot only then.
	// Under deltas the now-idle counters emit a 0 delta each flush, so the SUM stays
	// numReq; an absolute (or un-latched) sink re-adds the cumulative every flush, so
	// the SUM overshoots numReq and the assertSide check fails. This is a release
	// barrier on the flush ticker (receiver message arrival), NOT a sleep — without
	// it a single-window request burst makes the first flush's delta == the absolute
	// value, rendering the deliberate emit-absolute / skip-the-latch breaks invisible.
	if err := awaitFurtherFlushes(ctx, srv, 2); err != nil {
		return nil, sideSnapshot{}, err
	}

	snap := sideSnapshot{fams: make(map[string]familyReading, len(subsetNames))}
	for _, name := range subsetNames {
		sum, ok := srv.FamilySum(name)
		_, typ, _ := srv.Family(name) // type is last-seen COUNTER under deltas
		snap.fams[name] = familyReading{sum: sum, typ: typ, ok: ok}
	}
	if n := srv.Node(); n != nil {
		snap.nodeID = n.GetId()
		snap.nodeCluster = n.GetCluster()
	}
	return b.Bytes(), snap, nil
}

// fireProbe issues one query-less GET probePath with the fixed Host + User-Agent
// and returns the response status code (the body is drained and discarded).
func (d *metricsSinkDriver) fireProbe(ctx context.Context, client *http.Client, addr string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+probePath, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Host = probeHost
	req.Header.Set("User-Agent", probeUA)
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// pollSubset spins until every name in the deterministic COUNTER subset has been
// received with a delta-SUM == numReq AND the identifier node has arrived (or the
// context / deadline elapses). The proxy flushes the per-flush counter delta every
// stats_flush_interval (500ms), so a fixed sleep would be both flaky and slow;
// the poll converges as soon as the deltas have summed to K. This is the
// release barrier (reference_concurrency_differential_release_barrier).
func pollSubset(ctx context.Context, srv *metricsservice.Server) error {
	deadline := time.Now().Add(pollDeadline)
	for {
		if subsetConverged(srv) && srv.Node() != nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("metrics receiver: timed out waiting for COUNTER subset delta-SUM == %d + identifier node (%s, node=%v)",
				numReq, describeSubset(srv), srv.Node() != nil)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("metrics receiver: context done waiting for COUNTER subset delta-SUM == %d (%s): %w",
				numReq, describeSubset(srv), ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// awaitFurtherFlushes blocks until the receiver has accepted `extra` more
// StreamMetricsMessages than it had on entry (or ctx/deadline elapses). The proxy
// flushes every stats_flush_interval, so each additional message is one more flush;
// this is the release barrier the delta stability check rides on.
func awaitFurtherFlushes(ctx context.Context, srv *metricsservice.Server, extra int) error {
	base := srv.Messages()
	deadline := time.Now().Add(pollDeadline)
	for srv.Messages() < base+extra {
		if time.Now().After(deadline) {
			return fmt.Errorf("metrics receiver: timed out waiting for %d further flushes (messages=%d base=%d)", extra, srv.Messages(), base)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("metrics receiver: context done waiting for %d further flushes: %w", extra, ctx.Err())
		case <-time.After(pollInterval):
		}
	}
	return nil
}

// subsetConverged is true iff every subset name has been received with a
// delta-SUM of exactly numReq.
func subsetConverged(srv *metricsservice.Server) bool {
	for _, name := range subsetNames {
		v, ok := srv.FamilySum(name)
		if !ok || v != float64(numReq) {
			return false
		}
	}
	return true
}

// describeSubset renders the current subset delta-SUMs for a timeout diagnostic.
func describeSubset(srv *metricsservice.Server) string {
	var b bytes.Buffer
	for _, name := range subsetNames {
		v, ok := srv.FamilySum(name)
		fmt.Fprintf(&b, "%s=%v(ok=%v) ", name, v, ok)
	}
	return b.String()
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// the raw response bytes for the standard admin-diff at the runner's probe step.
func (*metricsSinkDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- fixture.StatsAsserter ---

// AssertStats asserts, on BOTH sides: the deterministic COUNTER NAME SUBSET
// present with type==COUNTER and delta-SUM==numReq; the identifier node.id/.cluster
// == the configured values. The poll-to-converge already proved the subset
// delta-SUM arrived at == numReq AND the identifier node arrived on each side's
// private receiver (decode ran — a zero-family pass is structurally impossible),
// but the assertion re-checks the captured snapshot.
func (d *metricsSinkDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	if os.Getenv("FIXTURE_0090_DUMP") != "" {
		fmt.Fprintf(os.Stderr, "=== 0090 ref node{id=%q cluster=%q} subj node{id=%q cluster=%q} ===\n",
			d.ref.nodeID, d.ref.nodeCluster, d.subj.nodeID, d.subj.nodeCluster)
		for _, name := range subsetNames {
			rf := d.ref.fams[name]
			sf := d.subj.fams[name]
			fmt.Fprintf(os.Stderr, "  %s: ref{sum=%v typ=%v ok=%v} subj{sum=%v typ=%v ok=%v}\n",
				name, rf.sum, rf.typ, rf.ok, sf.sum, sf.typ, sf.ok)
		}
	}

	// Decode-ran proof: each side must have captured at least the subset (Count()
	// > 0 is implied by ok==true on every subset name below; the converge poll
	// guaranteed it). A zero-family pass is structurally impossible.
	assertSide(t, "reference", d.ref)
	assertSide(t, "subject", d.subj)
}

// assertSide asserts the deterministic COUNTER subset (present + type==COUNTER +
// delta-SUM==numReq) and the identifier node id/cluster for one side's snapshot.
func assertSide(t fixture.TB, side string, snap sideSnapshot) {
	t.Helper()

	if len(snap.fams) == 0 {
		t.Fatalf("%s: no metric families captured (decode did not run)", side)
	}
	for _, name := range subsetNames {
		fr, present := snap.fams[name]
		if !present || !fr.ok {
			t.Fatalf("%s: COUNTER subset family %q absent (decode did not run for it)", side, name)
		}
		if fr.typ != dto.MetricType_COUNTER {
			t.Fatalf("%s: family %q type = %v, want COUNTER", side, name, fr.typ)
		}
		if fr.sum != float64(numReq) {
			t.Fatalf("%s: family %q delta-sum = %v, want %d (== K)", side, name, fr.sum, numReq)
		}
	}

	// The identifier node (msg #1) — cross-side EXACT on id + cluster.
	if snap.nodeID != wantNodeID {
		t.Fatalf("%s: identifier node.id = %q, want %q", side, snap.nodeID, wantNodeID)
	}
	if snap.nodeCluster != wantNodeCluster {
		t.Fatalf("%s: identifier node.cluster = %q, want %q", side, snap.nodeCluster, wantNodeCluster)
	}
}

// --- file / template helpers (the 0087 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0090-stats-sink-metrics-service-deltas/driver/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

func mustReadFixtureFile(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return string(b)
}

func mustRender(tpl string, data map[string]any) string {
	t, err := template.New("bootstrap").Parse(tpl)
	if err != nil {
		panic(fmt.Sprintf("driver: template parse: %v", err))
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("driver: template execute: %v", err))
	}
	return buf.String()
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*metricsSinkDriver)(nil)
	_ fixture.BackendKindAware = (*metricsSinkDriver)(nil)
	_ fixture.StatsAsserter    = (*metricsSinkDriver)(nil)
)
