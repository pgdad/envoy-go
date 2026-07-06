// Package driver registers the 0091-stats-sink-metrics-service-labels fixture
// with the differential runner. It is the behavioral proof of the phase 47.2b
// emit_tags_as_labels knob on the landed 47.1/47.2a metrics_service stats sink:
// cross-side EXACT (subject envoy-go vs reference Envoy contrib-v1.37.2 in
// Docker) on a deterministic COUNTER subset keyed by {residual dotted name,
// sorted envoy.<tag> LabelPairs}, asserting the CUMULATIVE last-seen value.
//
// Labels-split + CUMULATIVE-value model (the key departure from 0090's delta-SUM):
// under emit_tags_as_labels:true each stat name is split per the statsd SN tag
// rules into a RESIDUAL dotted name plus a set of LabelPairs, each keyed by the
// Envoy DOTTED tag-name (envoy.<tag>). The Counter value is the CUMULATIVE
// absolute (== K after K 2xx requests) — the 0089 last-seen value model, NOT a
// per-flush delta. There is NO delta-SUM and NO post-convergence stability
// barrier here (those are 47.2a/0090-only concerns); the cumulative value needs
// only first-reach-K. The driver therefore asserts the last-seen value == K via
// the label-keyed FamilyWithLabels(name, labels) accessor.
//
// The deterministic 3-family subset (residual + sorted labels):
//   - cluster.upstream_rq_total      {envoy.cluster_name=c_backend}
//   - http.downstream_rq_total       {envoy.http_conn_manager_prefix=hcm_local}
//   - http.downstream_rq_xx          {envoy.http_conn_manager_prefix=hcm_local,
//     envoy.response_code_class=2}
//
// The third is the 2xx two-label SN4 split: the response_code_class tag rule
// extracts the "2" digit and rewrites downstream_rq_2xx → the residual
// downstream_rq_xx, leaving a TWO-label set (hcm prefix + response_code_class).
// upstream_cx_total is excluded (connection reuse makes it < K). Gauges are
// labeled too but unasserted (non-deterministic). The label-set ordering is
// NORMALIZED: FamilyWithLabels compares labels in sorted-key order, so the
// LabelPair emission order is not load-bearing.
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
//     entry (transport_api_version V3, emit_tags_as_labels:true) naming an h2c
//     metrics cluster + a SHORT stats_flush_interval (500ms) for fast
//     deterministic convergence + a node{id,cluster} fixed identically on both
//     sides.
//
//  2. DriveReference / DriveSubject each fire K=7 deterministic GET requests
//     (all 2xx) through the proxy listener, then POLL that side's receiver until
//     the deterministic label-keyed COUNTER subset's last-seen VALUE converges
//     to == K AND the identifier node has arrived — a release barrier
//     (reference_concurrency_differential_release_barrier; the proxy flushes the
//     cumulative counter value every stats_flush_interval; poll not sleep). The
//     per-side family snapshot (the three subset values + the captured
//     identifier node id/cluster) is captured from that side's own receiver
//     before the next side runs.
//
//  3. AssertStats asserts, on BOTH sides: the deterministic label-keyed COUNTER
//     subset present with type==COUNTER and cumulative value==K; the identifier
//     node.id/.cluster == the configured values. PLUS decode-ran (a non-empty
//     snapshot) on each side before asserting.
//
// Decode-ran proof: the poll-to-converge guarantees the subset families arrived
// (value == K) on each side's own receiver before asserting — a zero-family
// pass is structurally impossible.
//
// UNasserted (AMEND-MS-HISTOGRAM-PRESENT + reference_streaming_sink_differential_framing):
// the WHOLE family set / family count (the surfaces differ cross-side — envoy-go
// has no histograms, the reference does); metric[].timestamp_ms (value
// non-deterministic; presence-only); help; LABELED gauges (server.uptime,
// *_active, connection churn — non-deterministic); the identifier
// user_agent_*/extensions[]; message/stream framing + per-message family count.
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
	"google.golang.org/protobuf/proto"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"github.com/pgdad/envoy-go/test/helpers/metricsservice"
)

const (
	fixtureName = "0091-stats-sink-metrics-service-labels"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0091 takes 10091 for the single plaintext listener.
	refAdminPort    = 9901
	refListenerPort = 10091

	// numReq is the per-side request count K. After K 2xx requests the
	// deterministic label-keyed COUNTER subset's last-seen VALUE == K on each side.
	numReq = 7

	// The fixed request shape (kept identical cross-side; query-less path).
	probePath = "/probe"
	probeHost = "metrics.example"
	probeUA   = "metrics-probe/1"

	// The HCM stat_prefix + backend cluster name baked IDENTICALLY into both
	// bootstraps, so the extracted label values match cross-side.
	statPrefix  = "hcm_local"
	backendName = "c_backend"

	// The bootstrap node id/cluster baked identically into both bootstraps →
	// the StreamMetricsMessage.identifier.node (msg #1) — cross-side assertable.
	wantNodeID      = "envoy-go-subject-0091"
	wantNodeCluster = "envoy-go-differential"

	// Converge-poll discipline (reference_concurrency_differential_release_barrier):
	// POLL the subset FamilyWithLabels() values to == K; never sleep-to-wait.
	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)

// subsetEntry is one deterministic label-keyed COUNTER family: the RESIDUAL
// dotted name (post tag-split) plus the SORTED envoy.<tag> LabelPairs the
// metrics_service emits under emit_tags_as_labels.
type subsetEntry struct {
	residual string
	labels   []*dto.LabelPair
}

// lp builds a single-label sorted slice; lp2 a two-label sorted slice.
func lp(k, v string) []*dto.LabelPair {
	return []*dto.LabelPair{{Name: proto.String(k), Value: proto.String(v)}}
}

func lp2(k1, v1, k2, v2 string) []*dto.LabelPair {
	return []*dto.LabelPair{
		{Name: proto.String(k1), Value: proto.String(v1)},
		{Name: proto.String(k2), Value: proto.String(v2)},
	}
}

// subset is the deterministic label-keyed COUNTER family subset asserted
// cross-side. The three residuals are DISTINCT, so the snapshot map is keyed by
// residual unambiguously. FamilyWithLabels compares labels order-insensitively
// (sorts internally), so lp2's order is not load-bearing — but the keys are kept
// in sorted order for clarity (envoy.http_conn_manager_prefix <
// envoy.response_code_class).
var subset = []subsetEntry{
	{"cluster.upstream_rq_total", lp("envoy.cluster_name", backendName)},
	{"http.downstream_rq_total", lp("envoy.http_conn_manager_prefix", statPrefix)},
	{"http.downstream_rq_xx", lp2("envoy.http_conn_manager_prefix", statPrefix, "envoy.response_code_class", "2")},
}

func init() {
	fixture.RegisterFixture(fixtureName, &metricsSinkDriver{})
}

// sideSnapshot is the per-side captured state: the three subset family
// (cumulative value, type) readings keyed by residual + the captured identifier
// node id/cluster.
type sideSnapshot struct {
	fams        map[string]familyReading
	nodeID      string
	nodeCluster string
}

type familyReading struct {
	value float64
	typ   dto.MetricType
	ok    bool
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
// label-keyed COUNTER subset's last-seen VALUE converges to == numReq AND the
// identifier node has arrived, and snapshots the subset readings + the captured
// identifier node. No Reset() is needed: each side owns a private receiver, so
// its accumulator is exclusively that side's data even though the other proxy
// keeps streaming into ITS receiver.
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

	snap := sideSnapshot{fams: make(map[string]familyReading, len(subset))}
	for _, e := range subset {
		v, typ, ok := srv.FamilyWithLabels(e.residual, e.labels)
		snap.fams[e.residual] = familyReading{value: v, typ: typ, ok: ok}
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

// pollSubset spins until every entry in the deterministic label-keyed COUNTER
// subset has been received with a last-seen VALUE == numReq AND the identifier
// node has arrived (or the context / deadline elapses). The proxy flushes the
// cumulative counter value every stats_flush_interval (500ms), so a fixed sleep
// would be both flaky and slow; the poll converges as soon as the cumulative
// value reaches K. This is the release barrier
// (reference_concurrency_differential_release_barrier).
func pollSubset(ctx context.Context, srv *metricsservice.Server) error {
	deadline := time.Now().Add(pollDeadline)
	for {
		if subsetConverged(srv) && srv.Node() != nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("metrics receiver: timed out waiting for label-keyed COUNTER subset value == %d + identifier node (%s, node=%v)",
				numReq, describeSubset(srv), srv.Node() != nil)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("metrics receiver: context done waiting for label-keyed COUNTER subset value == %d (%s): %w",
				numReq, describeSubset(srv), ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// subsetConverged is true iff every subset entry has been received (label-keyed)
// with a last-seen VALUE of exactly numReq.
func subsetConverged(srv *metricsservice.Server) bool {
	for _, e := range subset {
		v, _, ok := srv.FamilyWithLabels(e.residual, e.labels)
		if !ok || v != float64(numReq) {
			return false
		}
	}
	return true
}

// describeSubset renders the current subset label-keyed values for a timeout
// diagnostic.
func describeSubset(srv *metricsservice.Server) string {
	var b bytes.Buffer
	for _, e := range subset {
		v, _, ok := srv.FamilyWithLabels(e.residual, e.labels)
		fmt.Fprintf(&b, "%s=%v(ok=%v) ", e.residual, v, ok)
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

// AssertStats asserts, on BOTH sides: the deterministic label-keyed COUNTER
// subset present with type==COUNTER and cumulative value==numReq; the identifier
// node.id/.cluster == the configured values. The poll-to-converge already proved
// the subset value arrived at == numReq AND the identifier node arrived on each
// side's private receiver (decode ran — a zero-family pass is structurally
// impossible), but the assertion re-checks the captured snapshot.
func (d *metricsSinkDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	if os.Getenv("FIXTURE_0091_DUMP") != "" {
		fmt.Fprintf(os.Stderr, "=== 0091 ref node{id=%q cluster=%q} subj node{id=%q cluster=%q} ===\n",
			d.ref.nodeID, d.ref.nodeCluster, d.subj.nodeID, d.subj.nodeCluster)
		for _, e := range subset {
			rf := d.ref.fams[e.residual]
			sf := d.subj.fams[e.residual]
			fmt.Fprintf(os.Stderr, "  %s %v: ref{value=%v typ=%v ok=%v} subj{value=%v typ=%v ok=%v}\n",
				e.residual, e.labels, rf.value, rf.typ, rf.ok, sf.value, sf.typ, sf.ok)
		}
	}

	// Decode-ran proof: each side must have captured at least the subset (ok==true
	// on every subset entry below; the converge poll guaranteed it). A zero-family
	// pass is structurally impossible.
	assertSide(t, "reference", d.ref)
	assertSide(t, "subject", d.subj)
}

// assertSide asserts the deterministic label-keyed COUNTER subset (present +
// type==COUNTER + cumulative value==numReq) and the identifier node id/cluster
// for one side's snapshot.
func assertSide(t fixture.TB, side string, snap sideSnapshot) {
	t.Helper()

	if len(snap.fams) == 0 {
		t.Fatalf("%s: no metric families captured (decode did not run)", side)
	}
	for _, e := range subset {
		fr, present := snap.fams[e.residual]
		if !present || !fr.ok {
			t.Fatalf("%s: label-keyed COUNTER subset family %q %v absent (decode did not run for it)", side, e.residual, e.labels)
		}
		if fr.typ != dto.MetricType_COUNTER {
			t.Fatalf("%s: family %q %v type = %v, want COUNTER", side, e.residual, e.labels, fr.typ)
		}
		if fr.value != float64(numReq) {
			t.Fatalf("%s: family %q %v value = %v, want %d (== K)", side, e.residual, e.labels, fr.value, numReq)
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
	// thisFile is .../test/fixtures/0091-stats-sink-metrics-service-labels/driver/driver.go
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
