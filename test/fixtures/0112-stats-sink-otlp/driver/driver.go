// Package driver registers the 0112-stats-sink-otlp fixture with the differential
// runner. It is the behavioral proof of the phase 69 OpenTelemetry (OTLP) metrics
// stats sink (envoy.stat_sinks.open_telemetry) DEFAULT arm: knobs absent ⇒
// CUMULATIVE temporality + tag-extracted RESIDUAL metric names + tags-as-attributes.
// Cross-side (subject envoy-go vs reference Envoy contrib-v1.37.2 in Docker) on a
// deterministic COUNTER residual-name SUBSET aggregated across the periodic unary
// OTLP Export flushes.
//
// Integration shape (single-listener plaintext H1; two driver-owned in-process OTLP
// MetricsService gRPC receivers; HTTPFixedBody backend):
//
//  1. TWO driver-owned otlpmetrics.Server receivers — one per side — on two
//     separately-allocated host ports (refOTLPPort / subjOTLPPort), both bound on
//     0.0.0.0 BEFORE either proxy starts (reference_periodic_sink_differential_two_receivers).
//     The open_telemetry sink flushes PERIODICALLY (every stats_flush_interval), so
//     the reference proxy keeps Exporting into its receiver for the whole test —
//     including during the subject's drive window. A single shared receiver would let
//     the reference's concurrent flushes contaminate the subject snapshot. Two
//     receivers give each side a private accumulator, so no Reset() between sides is
//     needed. ReferenceBootstrap renders envoy.yaml pointing c_otlp at
//     host.docker.internal:refOTLPPort (STRICT_DNS + V4_ONLY, P-16); SubjectConfig
//     renders envoy-go.yaml pointing at 127.0.0.1:subjOTLPPort. BOTH bootstraps carry a
//     bootstrap-level stats_sinks[] open_telemetry entry with NO knob fields (the
//     DEFAULT arm) + a SHORT stats_flush_interval (0.1s) for fast convergence.
//
//  2. DriveReference / DriveSubject each fire K=7 deterministic GET requests (all
//     2xx) through the proxy listener, then POLL that side's receiver until the
//     deterministic listener-scoped COUNTER residual subset converges to == K AND at
//     least one Export has been received (served-this-arm precondition,
//     feedback_probe_fresh_container_per_arm adapted to a driver-owned server), then
//     wait for ≥2 further flushes and re-read the cumulative StartTime (the constant-
//     across-flushes barrier). The per-side snapshot (all datapoints + resource-attr
//     keys + the two StartTime samples) is captured from that side's own receiver
//     before the next side runs. After the subject snapshot BOTH receivers are
//     hard-stopped via Close() (grpc.Server.Stop, NOT GracefulStop — the proxies hold
//     no long-lived stream here since Export is unary, but Close is the deterministic
//     teardown precedent and the readings are already snapshotted).
//
//  3. AssertStats asserts, on BOTH sides (NAMED SUBSETS only,
//     reference_stats_sink_emits_used_only — the reference emits USED-only stats and
//     the set GROWS with use):
//     - the residual counters http.downstream_rq_total, http.downstream_rq_xx, and
//     cluster.upstream_rq_total, each ATTRIBUTE-QUALIFIED (see the COLLISION note),
//     present as monotonic CUMULATIVE Sum with value == K and the expected FULL
//     envoy.<tag> attribute SET (order-insensitive).
//     - the three telemetry.sdk.* resource KEYS present (per-side values, never
//     cross-side value equality).
//     - (subject side) the cumulative StartTimeUnixNano is ns-magnitude and CONSTANT
//     across ≥2 further flushes.
//
// ## The residual-name COLLISION (T5-review critical finding, documented here)
//
// Under the DEFAULT use_tag_extracted_name=true, tag extraction collapses
// cluster.<name>.upstream_rq_total to the RESIDUAL cluster.upstream_rq_total and
// http.<stat_prefix>.downstream_rq_* to http.downstream_rq_*. NONE of these residuals
// is unique in this topology (confirmed empirically, FIXTURE_0112_DUMP):
//   - cluster.upstream_rq_total is emitted by BOTH the application backend cluster
//     (c_backend) AND the OTLP sink's OWN gRPC cluster (c_otlp) — differing only in
//     envoy.cluster_name, and the sink cluster's count GROWS with every flush.
//   - http.downstream_rq_total / _xx are ALSO emitted by the built-in ADMIN listener's
//     own HCM (envoy.http_conn_manager_prefix=admin), not just the test HCM (hcm_local).
//
// So a NAME-ONLY lookup is ambiguous and non-deterministic (latest-write-wins). We
// therefore select each datapoint by a discriminating attribute (envoy.cluster_name=
// c_backend / envoy.http_conn_manager_prefix=hcm_local) via the receiver's
// attribute-qualified Datapoints() snapshot — the T5 receiver was extended by this task
// to expose the full (name, attrs) identity of every accumulated datapoint. The sink
// cluster's non-deterministic c_otlp value is deliberately UNasserted.
//
// The tag-extracted residual spellings were confirmed EMPIRICALLY by logging every
// received datapoint (FIXTURE_0112_DUMP=1) on both sides before finalizing the subset.
//
// UNasserted cross-side (reference_streaming_sink_differential_framing +
// reference_cluster_sink_dial_unaccounted): the WHOLE family set / count (surfaces
// differ — no envoy-go histograms), the reference's StartTime shape (its µs bug), the
// sink cluster's upstream_cx_* (dial-unaccounted + the P-15 feedback loop), OTLP
// message framing + per-Export metric count + flush cadence.
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
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"github.com/pgdad/envoy-go/test/helpers/otlpmetrics"
)

const (
	fixtureName = "0112-stats-sink-otlp"

	// In-container reference Envoy ports. Convention "104NN" here uses 10448 for
	// the single plaintext listener (fixture 0112 → port 10448 per the SPEC).
	refAdminPort    = 9901
	refListenerPort = 10448

	// numReq is the per-side request count K. After K 2xx requests the deterministic
	// COUNTER residual subset converges to value == K on each side.
	numReq = 7

	// The fixed request shape (kept identical cross-side; query-less path).
	probePath = "/probe"
	probeHost = "otlp.example"
	probeUA   = "otlp-probe/1"

	// The HCM stat_prefix + backend cluster name baked IDENTICALLY into both
	// bootstraps, so the tag-extracted residual names + envoy.<tag> attributes match
	// cross-side.
	statPrefix  = "hcm_local"
	backendName = "c_backend"

	// Converge-poll discipline (reference_concurrency_differential_release_barrier):
	// POLL the residual Sum values to == K; never sleep-to-wait.
	pollInterval = 100 * time.Millisecond
	pollDeadline = 30 * time.Second

	// stabilityFlushes is how many FURTHER Exports to wait for after convergence
	// before re-reading the cumulative StartTime for the constant-across-flushes
	// barrier (subject side).
	stabilityFlushes = 2
)

// Tag-extracted RESIDUAL metric names (confirmed empirically via FIXTURE_0112_DUMP):
//
//	http.<stat_prefix>.downstream_rq_total → http.downstream_rq_total   (SN2)
//	http.<stat_prefix>.downstream_rq_2xx   → http.downstream_rq_xx       (SN2 + SN4 status-class)
//	cluster.<name>.upstream_rq_total       → cluster.upstream_rq_total   (SN1)
const (
	residualHTTPReqTotal = "http.downstream_rq_total"
	residualHTTPReqXX    = "http.downstream_rq_xx"
	residualClusterReq   = "cluster.upstream_rq_total"
)

// The envoy.<tag> attribute names the DEFAULT sink emits (kvFromTags: envoy_<tag>
// label key → envoy.<tag> attribute key).
const (
	attrHTTPPrefix    = "envoy.http_conn_manager_prefix"
	attrClusterName   = "envoy.cluster_name"
	attrResponseClass = "envoy.response_code_class"
)

// The three telemetry.sdk.* resource KEYS the sink always emits (values per-side).
var resourceKeys = []string{
	"telemetry.sdk.name",
	"telemetry.sdk.language",
	"telemetry.sdk.version",
}

// wantDP describes one asserted datapoint, selected by its FULL expected envoy.<tag>
// attribute SET (order-insensitive). EVERY residual in this subset is
// attribute-qualified — the empirical dump (FIXTURE_0112_DUMP) confirmed NONE is
// unique:
//   - the built-in ADMIN listener runs its OWN HCM (stat_prefix=admin), so
//     http.downstream_rq_total / _xx also appear with envoy.http_conn_manager_prefix=admin;
//   - cluster.upstream_rq_total is emitted by BOTH c_backend and the c_otlp sink cluster;
//   - crucially, the SUBJECT emits http.downstream_rq_xx for response classes 2,3,4,5
//     (used-but-zero 3xx/4xx/5xx), all sharing envoy.http_conn_manager_prefix=hcm_local
//     — so a single-key selection is NOT deterministic. Selecting by the full attribute
//     SET (matching ALL wantAttrs pairs) picks the one datapoint we mean; the equal-
//     length check in assertSumDatapoint then proves there is no EXTRA attribute beyond
//     wantAttrs. The reference emits only the class-2 datapoint (used-only), and the
//     class 3/4/5 zero datapoints are a cross-side surface difference deliberately
//     outside the asserted named subset.
type wantDP struct {
	name      string
	wantAttrs map[string]string // the FULL expected attribute set (also the selector)
}

var wantDPs = []wantDP{
	{residualHTTPReqTotal, map[string]string{attrHTTPPrefix: statPrefix}},
	{residualHTTPReqXX, map[string]string{attrHTTPPrefix: statPrefix, attrResponseClass: "2"}},
	{residualClusterReq, map[string]string{attrClusterName: backendName}},
}

func init() {
	fixture.RegisterFixture(fixtureName, &otlpSinkDriver{})
}

// sideSnapshot is the per-side captured state: every accumulated datapoint (for the
// named-subset + attribute-set + collision-resolved assertions), the union of
// resource-attribute keys, and the two cumulative-StartTime samples (converge-time and
// post-stability-barrier) for the constant-across-flushes assertion.
type sideSnapshot struct {
	dps        []otlpmetrics.DatapointView
	resKeys    map[string]bool
	startTime1 uint64 // http.downstream_rq_total StartTime at convergence
	startTime2 uint64 // ... after ≥stabilityFlushes further Exports
}

// otlpSinkDriver carries the per-driver lifecycle state — TWO private OTLP receivers
// (one per side) on two separately-allocated host ports, plus the per-side snapshots
// captured during Drive for the AssertStats cross-side assertion.
type otlpSinkDriver struct {
	mu sync.Mutex

	refOTLPPort  int
	subjOTLPPort int
	refSrv       *otlpmetrics.Server
	subjSrv      *otlpmetrics.Server

	ref  sideSnapshot
	subj sideSnapshot
}

// ensure allocates the two receiver ports (via Listen+Close) and starts both
// in-process OTLP receivers bound to 0.0.0.0:<port>. Idempotent.
func (d *otlpSinkDriver) ensure() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.refOTLPPort == 0 {
		d.refOTLPPort = mustAllocatePort()
	}
	if d.subjOTLPPort == 0 {
		d.subjOTLPPort = mustAllocatePort()
	}
	if d.refSrv == nil {
		d.refSrv = mustStartReceiver(d.refOTLPPort)
	}
	if d.subjSrv == nil {
		d.subjSrv = mustStartReceiver(d.subjOTLPPort)
	}
}

// mustAllocatePort reserves a free TCP port via Listen+Close.
func mustAllocatePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate otlp port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// mustStartReceiver starts an in-process OTLP receiver bound to 0.0.0.0:<port>.
func mustStartReceiver(port int) *otlpmetrics.Server {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	srv, err := otlpmetrics.NewAtAddr(addr)
	if err != nil {
		panic(fmt.Sprintf("driver: start otlp receiver on %s: %v", addr, err))
	}
	return srv
}

// --- fixture.Driver (required) ---

func (*otlpSinkDriver) BackendCount() int                { return 1 }
func (*otlpSinkDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }
func (*otlpSinkDriver) SubjectListenerName() string      { return "l_test" }
func (*otlpSinkDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the runner-
// allocated backend port + the reference-side OTLP receiver host:port. It allocates the
// ports and starts both receivers here so they are live before the reference boots.
func (d *otlpSinkDriver) ReferenceBootstrap(backendPorts []int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    refAdminPort,
		"ListenerPort": refListenerPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
		"OTLPHost":     "host.docker.internal",
		"OTLPPort":     d.refOTLPPort,
		"StatPrefix":   statPrefix,
		"BackendName":  backendName,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports +
// backend port (loopback) + the subject-side OTLP receiver port (host=127.0.0.1).
func (d *otlpSinkDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
		"OTLPPort":     d.subjOTLPPort,
		"StatPrefix":   statPrefix,
		"BackendName":  backendName,
	})
}

// DriveReference fires the workload against the reference proxy and snapshots the
// reference-side datapoints from the reference's own private receiver.
func (d *otlpSinkDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	srv := d.refSrv
	d.mu.Unlock()
	out, snap, err := d.driveSide(ctx, addr, srv, "reference")
	if err != nil {
		return nil, err
	}
	d.ref = snap
	return out, nil
}

// DriveSubject fires the workload against the subject proxy and snapshots the
// subject-side datapoints from the subject's own private receiver, then hard-stops both
// receivers for deterministic teardown.
func (d *otlpSinkDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	srv := d.subjSrv
	d.mu.Unlock()
	out, snap, err := d.driveSide(ctx, addr, srv, "subject")
	if err != nil {
		return nil, err
	}
	d.subj = snap
	d.closeServers()
	return out, nil
}

// closeServers hard-stops both receivers (idempotent).
func (d *otlpSinkDriver) closeServers() {
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

// driveSide fires numReq deterministic GET requests against the proxy listener at addr
// (all 2xx), polls that side's private receiver until the listener-scoped residual
// subset converges to == numReq AND at least one Export has arrived, waits for
// stabilityFlushes further Exports (the cumulative-StartTime constant barrier), and
// snapshots all datapoints + resource-attr keys + the two StartTime samples.
func (d *otlpSinkDriver) driveSide(ctx context.Context, addr string, srv *otlpmetrics.Server, side string) ([]byte, sideSnapshot, error) {
	if srv == nil {
		return nil, sideSnapshot{}, fmt.Errorf("driver: otlp receiver not running")
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
		dumpDatapoints(side, srv)
		return nil, sideSnapshot{}, err
	}

	// StartTime sample #1 at convergence (the test-driven HCM's http.downstream_rq_total).
	start1, _ := sumStartTime(srv.Datapoints(), residualHTTPReqTotal, map[string]string{attrHTTPPrefix: statPrefix})

	// Wait for ≥stabilityFlushes further Exports so the cumulative StartTime can be
	// re-read for the constant-across-flushes barrier.
	base := srv.ExportCount()
	if err := pollExportCount(ctx, srv, base+stabilityFlushes); err != nil {
		dumpDatapoints(side, srv)
		return nil, sideSnapshot{}, err
	}

	dps := srv.Datapoints()
	start2, _ := sumStartTime(dps, residualHTTPReqTotal, map[string]string{attrHTTPPrefix: statPrefix})

	snap := sideSnapshot{
		dps:        dps,
		resKeys:    resourceKeySet(srv),
		startTime1: start1,
		startTime2: start2,
	}
	dumpDatapoints(side, srv)
	return b.Bytes(), snap, nil
}

// fireProbe issues one query-less GET probePath with the fixed Host + User-Agent and
// returns the response status code (the body is drained and discarded).
func (d *otlpSinkDriver) fireProbe(ctx context.Context, client *http.Client, addr string) (int, error) {
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

// pollSubset spins until every attribute-qualified residual in wantDPs has been
// received as a Sum with value == numReq AND at least one Export has arrived (the
// served-this-arm precondition — a zero-Export pass is a false green). This is the
// release barrier (reference_concurrency_differential_release_barrier).
func pollSubset(ctx context.Context, srv *otlpmetrics.Server) error {
	deadline := time.Now().Add(pollDeadline)
	for {
		if srv.ExportCount() > 0 && subsetConverged(srv) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("otlp receiver: timed out waiting for residual Sum subset == %d + an Export (exports=%d, %s)",
				numReq, srv.ExportCount(), describeSubset(srv))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("otlp receiver: context done waiting for residual subset == %d (%s): %w",
				numReq, describeSubset(srv), ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// pollExportCount spins until the receiver has recorded at least want Exports.
func pollExportCount(ctx context.Context, srv *otlpmetrics.Server, want int) error {
	deadline := time.Now().Add(pollDeadline)
	for {
		if srv.ExportCount() >= want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("otlp receiver: timed out waiting for %d Exports (have %d)", want, srv.ExportCount())
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("otlp receiver: context done waiting for %d Exports (have %d): %w", want, srv.ExportCount(), ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// subsetConverged is true iff every wantDP residual has an attribute-set-qualified Sum
// datapoint with value exactly numReq.
func subsetConverged(srv *otlpmetrics.Server) bool {
	dps := srv.Datapoints()
	for _, w := range wantDPs {
		dp, ok := findByAttrs(dps, w.name, w.wantAttrs)
		if !ok || !dp.IsSum || dp.Value != float64(numReq) {
			return false
		}
	}
	return true
}

// describeSubset renders the current wantDP Sum readings for a diagnostic.
func describeSubset(srv *otlpmetrics.Server) string {
	dps := srv.Datapoints()
	var b bytes.Buffer
	for _, w := range wantDPs {
		dp, ok := findByAttrs(dps, w.name, w.wantAttrs)
		fmt.Fprintf(&b, "%s%v=%v(ok=%v) ", w.name, w.wantAttrs, dp.Value, ok)
	}
	return b.String()
}

// findByAttrs returns the (first) datapoint named name whose attribute set CONTAINS
// every key=val pair in want (order-insensitive). Because the callers pass the FULL
// expected set, this deterministically selects the intended datapoint even when the
// same residual name carries several attribute sets (e.g. per response-code class).
func findByAttrs(dps []otlpmetrics.DatapointView, name string, want map[string]string) (otlpmetrics.DatapointView, bool) {
	for _, dp := range dps {
		if dp.Name != name {
			continue
		}
		got := attrMap(dp.Attrs)
		match := true
		for k, v := range want {
			if got[k] != v {
				match = false
				break
			}
		}
		if match {
			return dp, true
		}
	}
	return otlpmetrics.DatapointView{}, false
}

// sumStartTime returns the StartTimeUnixNano of the (first) Sum datapoint named name
// matching want.
func sumStartTime(dps []otlpmetrics.DatapointView, name string, want map[string]string) (uint64, bool) {
	dp, ok := findByAttrs(dps, name, want)
	if !ok || !dp.IsSum {
		return 0, false
	}
	return dp.StartTime, true
}

// resourceKeySet returns the union of all resource-attribute keys seen across every
// received ResourceMetrics on this side's receiver.
func resourceKeySet(srv *otlpmetrics.Server) map[string]bool {
	keys := map[string]bool{}
	for _, set := range srv.ResourceAttributes() {
		for _, kv := range set {
			keys[kv.GetKey()] = true
		}
	}
	return keys
}

// dumpDatapoints logs every accumulated datapoint (name + sorted attrs + value + type)
// when FIXTURE_0112_DUMP is set — the empirical residual-name confirmation surface.
func dumpDatapoints(side string, srv *otlpmetrics.Server) {
	if os.Getenv("FIXTURE_0112_DUMP") == "" {
		return
	}
	dps := srv.Datapoints()
	sort.Slice(dps, func(i, j int) bool { return dps[i].Name < dps[j].Name })
	fmt.Fprintf(os.Stderr, "=== 0112 %s datapoints (exports=%d) ===\n", side, srv.ExportCount())
	for _, dp := range dps {
		kind := "GAUGE"
		if dp.IsSum {
			kind = fmt.Sprintf("SUM(temp=%v,mono=%v,start=%d)", dp.Temporality, dp.IsMonotonic, dp.StartTime)
		}
		fmt.Fprintf(os.Stderr, "  %-40s %-10s v=%v attrs=%s\n", dp.Name, kind, dp.Value, attrsString(dp.Attrs))
	}
	for k := range resourceKeySet(srv) {
		fmt.Fprintf(os.Stderr, "  resource-key: %s\n", k)
	}
}

func attrsString(attrs []*commonpb.KeyValue) string {
	pairs := make([]string, 0, len(attrs))
	for _, kv := range attrs {
		pairs = append(pairs, kv.GetKey()+"="+kv.GetValue().GetStringValue())
	}
	sort.Strings(pairs)
	return "{" + strings.Join(pairs, ",") + "}"
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (*otlpSinkDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertStats asserts the DEFAULT-arm proposition on BOTH sides (named subsets only).
func (d *otlpSinkDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	assertSide(t, "reference", d.ref, false)
	assertSide(t, "subject", d.subj, true)
}

// assertSide asserts one side's snapshot: the listener-scoped residual Sums (value==K,
// cumulative, monotonic, expected attribute set), the collision-resolved backend-cluster
// Sum, the telemetry.sdk.* resource keys, and (subject) the constant cumulative
// StartTime.
func assertSide(t fixture.TB, side string, snap sideSnapshot, checkStartConstant bool) {
	t.Helper()

	if len(snap.dps) == 0 {
		t.Fatalf("%s: no datapoints captured (Export did not run)", side)
	}

	// (1) The attribute-qualified residual Sum subset: each present as monotonic
	// CUMULATIVE Sum with value == K and the FULL expected envoy.<tag> attribute set.
	// Every residual is attribute-qualified because none is unique — the built-in ADMIN
	// HCM shares http.downstream_rq_total / _xx, and the c_otlp sink cluster shares
	// cluster.upstream_rq_total (the sink cluster's non-deterministic value is
	// deliberately UNasserted). See wantDPs.
	for _, w := range wantDPs {
		assertCumulativeSumByAttr(t, side, snap.dps, w)
	}

	// (2) The three telemetry.sdk.* resource KEYS present (per-side values UNasserted).
	for _, k := range resourceKeys {
		if !snap.resKeys[k] {
			t.Errorf("%s: resource attribute key %q absent (have %v)", side, k, sortedKeys(snap.resKeys))
		}
	}

	// (3) Subject side: the cumulative StartTimeUnixNano is ns-magnitude and CONSTANT
	// across ≥stabilityFlushes further flushes.
	if checkStartConstant {
		if snap.startTime1 == 0 || snap.startTime2 == 0 {
			t.Errorf("%s: cumulative StartTime unset (sample1=%d sample2=%d)", side, snap.startTime1, snap.startTime2)
		}
		if !nsMagnitude(snap.startTime2) {
			t.Errorf("%s: cumulative StartTime %d is not ns-magnitude", side, snap.startTime2)
		}
		if snap.startTime1 != snap.startTime2 {
			t.Errorf("%s: cumulative StartTime not constant across flushes: %d != %d", side, snap.startTime1, snap.startTime2)
		}
	}
}

// assertCumulativeSumByAttr selects the datapoint named w.name matching w.wantAttrs
// (collision + response-class disambiguation) and asserts it is a monotonic CUMULATIVE
// Sum with value == numReq whose FULL attribute set equals w.wantAttrs (order-insensitive).
func assertCumulativeSumByAttr(t fixture.TB, side string, dps []otlpmetrics.DatapointView, w wantDP) {
	t.Helper()
	dp, ok := findByAttrs(dps, w.name, w.wantAttrs)
	if !ok {
		t.Errorf("%s: residual Sum %q with attrs %v absent", side, w.name, w.wantAttrs)
		return
	}
	assertSumDatapoint(t, side, dp, w.name, w.wantAttrs)
}

// assertSumDatapoint asserts a datapoint is a monotonic CUMULATIVE Sum with value ==
// numReq and a FULL attribute set equal to wantAttrs (order-insensitive map comparison).
func assertSumDatapoint(t fixture.TB, side string, dp otlpmetrics.DatapointView, name string, wantAttrs map[string]string) {
	t.Helper()
	if !dp.IsSum {
		t.Errorf("%s: %q is not a Sum (Gauge) — want monotonic cumulative Sum", side, name)
		return
	}
	if !dp.IsMonotonic {
		t.Errorf("%s: %q Sum IsMonotonic=false, want true", side, name)
	}
	if dp.Temporality != metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE {
		t.Errorf("%s: %q Sum temporality=%v, want CUMULATIVE", side, name, dp.Temporality)
	}
	if dp.Value != float64(numReq) {
		t.Errorf("%s: %q Sum value=%v, want %d (== K)", side, name, dp.Value, numReq)
	}
	got := attrMap(dp.Attrs)
	if len(got) != len(wantAttrs) {
		t.Errorf("%s: %q attribute set = %v, want %v", side, name, got, wantAttrs)
		return
	}
	for k, v := range wantAttrs {
		if got[k] != v {
			t.Errorf("%s: %q attribute %q = %q, want %q (full set got=%v)", side, name, k, got[k], v, got)
		}
	}
}

// attrMap renders a datapoint's attributes as a key→string-value map (order-insensitive).
func attrMap(attrs []*commonpb.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		m[kv.GetKey()] = kv.GetValue().GetStringValue()
	}
	return m
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// nsMagnitude is true if v looks like a UnixNano timestamp (> ~2001-09 in ns).
func nsMagnitude(v uint64) bool {
	return v > 1_000_000_000_000_000_000
}

// --- file / template helpers ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0112-stats-sink-otlp/driver/driver.go
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
	_ fixture.Driver           = (*otlpSinkDriver)(nil)
	_ fixture.BackendKindAware = (*otlpSinkDriver)(nil)
	_ fixture.StatsAsserter    = (*otlpSinkDriver)(nil)
)
