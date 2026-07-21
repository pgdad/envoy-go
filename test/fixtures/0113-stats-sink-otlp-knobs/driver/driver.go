// Package driver registers the 0113-stats-sink-otlp-knobs fixture with the
// differential runner. It is the behavioral proof of the phase 69 OpenTelemetry
// (OTLP) metrics stats sink (envoy.stat_sinks.open_telemetry) KNOB arm: the three
// knobs turned ON together in ONE coherent config —
//
//	report_counters_as_deltas: true   ⇒ DELTA temporality; per-flush counter DELTAS
//	                                    that SUM to K across flushes (NOT the
//	                                    cumulative absolute); isMonotonic RETAINED.
//	prefix: <p>                        ⇒ every metric name composed <prefix>.<base>.
//	use_tag_extracted_name: false      ⇒ names are the FULL DOTTED stat names
//	                                    (cluster.<name>.<stat>, http.<prefix>.<stat>),
//	                                    NOT the tag-extracted residual.
//	emit_tags_as_attributes: false     ⇒ NO envoy.<tag> attributes on any datapoint.
//
// Cross-side (subject envoy-go vs reference Envoy contrib-v1.37.2 in Docker) on a
// deterministic COUNTER FULL-DOTTED-name SUBSET, asserting the per-flush DELTA model
// via the receiver's running DeltaSum accessor with a POST-CONVERGENCE STABILITY
// BARRIER.
//
// ## Why the full-dotted names are UNIQUE (no attribute-qualified lookup needed)
//
// With use_tag_extracted_name:false the sink emits the FULL DOTTED name, so the
// application-backend counter is <prefix>.cluster.c_backend.upstream_rq_total —
// DISTINCT from the OTLP sink cluster's <prefix>.cluster.c_otlp.upstream_rq_total and
// from the admin HCM's <prefix>.http.admin.downstream_rq_total. Unlike the 0112
// DEFAULT arm (where tag extraction collapsed every stat to a colliding RESIDUAL and
// forced attribute-qualified selection), here the name alone is unambiguous, so the
// receiver's NAME-keyed DeltaSum(name) running-sum accessor selects the intended
// counter directly (reference_delta_sink_differential_stability_barrier).
//
// ## The delta-SUM model + the STABILITY BARRIER (the load-bearing assertion)
//
// Under report_counters_as_deltas:true each COUNTER family carries the per-flush
// DELTA (the increment since the previous flush), NOT the cumulative absolute value.
// A single flush is partial; once a counter goes idle its per-flush delta is 0. So
// the last-seen value==K test (the 0112 cumulative shape) is meaningless here.
// Instead a counter's per-flush deltas SUM across flushes to the cumulative total
// (== K after K 2xx requests). The receiver's DeltaSum(name) accumulates exactly
// that running sum.
//
// The BARRIER (reference_delta_sink_differential_stability_barrier): after the
// running DeltaSum reaches K, wait for >=2 FURTHER idle flushes and re-read it — it
// must STILL be K (the idle flushes contribute 0 deltas). This distinguishes a true
// DELTA sink from an absolute/cumulative one: an absolute emitter re-sends the whole
// counter value every flush, so its running sum OVERSHOOTS K after the further
// flushes and the barrier FIRES. Without the barrier a first-flush delta is
// indistinguishable from an absolute value (Break O+P prove the barrier is
// load-bearing).
//
// Integration shape (single-listener plaintext H1; two driver-owned in-process OTLP
// MetricsService gRPC receivers; HTTPFixedBody backend) mirrors 0112:
//
//  1. TWO driver-owned otlpmetrics.Server receivers — one per side — on two
//     separately-allocated host ports, both bound on 0.0.0.0 BEFORE either proxy
//     starts (reference_periodic_sink_differential_two_receivers). The reference
//     keeps Exporting into its own receiver for the whole test, so a single shared
//     receiver would contaminate the subject snapshot. ReferenceBootstrap renders
//     envoy.yaml pointing c_otlp at host.docker.internal:refOTLPPort (STRICT_DNS +
//     V4_ONLY, P-16); SubjectConfig renders envoy-go.yaml at 127.0.0.1:subjOTLPPort.
//     BOTH bootstraps carry the open_telemetry sink with the FOUR knob fields (bare
//     scalars) + a SHORT stats_flush_interval (0.1s).
//
//  2. DriveReference / DriveSubject each fire K=7 deterministic GET requests (all
//     2xx) through the proxy listener, then POLL that side's receiver until the
//     full-dotted COUNTER subset's DeltaSum converges to == K AND at least one Export
//     has arrived (served-this-arm precondition), then wait for >=2 further flushes
//     (the stability barrier) and re-read DeltaSum + snapshot the datapoints +
//     resource-attr keys. The per-side snapshot is captured before the next side runs.
//     After the subject snapshot BOTH receivers are hard-stopped via Close().
//
//  3. AssertStats asserts, on BOTH sides (NAMED SUBSETS only,
//     reference_stats_sink_emits_used_only):
//     - the full-dotted counter subset's DeltaSum == K AFTER the barrier (still K);
//     - each such counter's latest datapoint is a DELTA Sum, IsMonotonic RETAINED
//     (true), with NO attributes (both-false);
//     - a GAUGE stays ABSOLUTE (emitted as a Gauge, not converted to a delta Sum);
//     - the three telemetry.sdk.* resource KEYS present (per-side values unasserted).
//
// UNasserted cross-side (reference_streaming_sink_differential_framing +
// reference_cluster_sink_dial_unaccounted): the WHOLE family set / count, the
// reference's StartTime shape (its µs bug), the sink cluster's own counters +
// upstream_cx_* (dial-unaccounted + the P-15 feedback loop), gauge VALUES
// (non-deterministic), OTLP framing + per-Export metric count + flush cadence.
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
	fixtureName = "0113-stats-sink-otlp-knobs"

	// In-container reference Envoy ports. Fixture 0113 → port 10449 per the SPEC.
	refAdminPort    = 9901
	refListenerPort = 10449

	// numReq is the per-side request count K. After K 2xx requests each subset
	// counter's per-flush deltas SUM to == K on each side.
	numReq = 7

	// The fixed request shape (kept identical cross-side; query-less path).
	probePath = "/probe"
	probeHost = "otlp.example"
	probeUA   = "otlp-probe/1"

	// The HCM stat_prefix + backend cluster name baked IDENTICALLY into both
	// bootstraps, so the FULL DOTTED metric names match cross-side.
	statPrefix  = "hcm_local"
	backendName = "c_backend"

	// metricPrefix is the sink's `prefix` knob — composed as "<metricPrefix>.<base>"
	// (a single dot is inserted by the sink; verified empirically via FIXTURE_0113_DUMP).
	metricPrefix = "envoytest"

	// Converge-poll discipline (reference_concurrency_differential_release_barrier):
	// POLL the DeltaSum running sums to == K; never sleep-to-wait.
	pollInterval = 100 * time.Millisecond
	pollDeadline = 30 * time.Second

	// stabilityFlushes is how many FURTHER Exports to wait for after DeltaSum
	// convergence before re-reading it for the stability barrier (an absolute
	// emitter overshoots K across these further flushes).
	stabilityFlushes = 2
)

// subsetCounters is the deterministic FULL-DOTTED COUNTER name subset asserted
// cross-side, prefix-composed. All three are counters whose per-flush deltas SUM to
// exactly numReq after K 2xx requests on BOTH sides. Each full-dotted name is UNIQUE
// (no residual collision — see the package doc), so the NAME-keyed DeltaSum accessor
// selects each unambiguously.
//
// The prefixed spellings were confirmed EMPIRICALLY (FIXTURE_0113_DUMP=1) on both
// sides before finalizing.
var subsetCounters = []string{
	metricPrefix + ".cluster." + backendName + ".upstream_rq_total",
	metricPrefix + ".http." + statPrefix + ".downstream_rq_total",
	metricPrefix + ".http." + statPrefix + ".downstream_rq_2xx",
}

// gaugeName is a deterministically-present GAUGE (the backend cluster's configured
// endpoint membership, value 1 both sides). Under report_counters_as_deltas it stays
// ABSOLUTE — emitted as a Gauge, NOT converted to a delta Sum (D-MS-DELTA-GAUGE). Its
// VALUE is unasserted (cross-side membership shape differs); only its Gauge TYPE is.
const gaugeName = metricPrefix + ".cluster." + backendName + ".membership_total"

// The three telemetry.sdk.* resource KEYS the sink always emits (values per-side).
var resourceKeys = []string{
	"telemetry.sdk.name",
	"telemetry.sdk.language",
	"telemetry.sdk.version",
}

func init() {
	fixture.RegisterFixture(fixtureName, &otlpKnobsDriver{})
}

// sideSnapshot is the per-side captured state: the subset DeltaSum running sums
// captured at convergence (sum1) AND after >=stabilityFlushes further flushes (sum2,
// the barrier), plus every accumulated datapoint (for the DELTA/monotonic/no-attrs +
// gauge-absolute assertions) and the union of resource-attribute keys.
type sideSnapshot struct {
	deltaSum1 map[string]float64 // subset name -> DeltaSum at convergence
	deltaSum2 map[string]float64 // subset name -> DeltaSum after >=stabilityFlushes further flushes
	dps       []otlpmetrics.DatapointView
	resKeys   map[string]bool
}

// otlpKnobsDriver carries the per-driver lifecycle state — TWO private OTLP receivers
// (one per side) on two separately-allocated host ports, plus the per-side snapshots.
type otlpKnobsDriver struct {
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
func (d *otlpKnobsDriver) ensure() {
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

func (*otlpKnobsDriver) BackendCount() int                { return 1 }
func (*otlpKnobsDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }
func (*otlpKnobsDriver) SubjectListenerName() string      { return "l_test" }
func (*otlpKnobsDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the runner-
// allocated backend port + the reference-side OTLP receiver host:port. It allocates
// the ports and starts both receivers here so they are live before the reference boots.
func (d *otlpKnobsDriver) ReferenceBootstrap(backendPorts []int) string {
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
		"MetricPrefix": metricPrefix,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports +
// backend port (loopback) + the subject-side OTLP receiver port (host=127.0.0.1).
func (d *otlpKnobsDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
		"OTLPPort":     d.subjOTLPPort,
		"StatPrefix":   statPrefix,
		"BackendName":  backendName,
		"MetricPrefix": metricPrefix,
	})
}

// DriveReference fires the workload against the reference proxy and snapshots the
// reference-side readings from the reference's own private receiver.
func (d *otlpKnobsDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
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
// subject-side readings, then hard-stops both receivers for deterministic teardown.
func (d *otlpKnobsDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
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
func (d *otlpKnobsDriver) closeServers() {
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

// driveSide fires numReq deterministic GET requests against the proxy listener at
// addr (all 2xx), polls that side's private receiver until the full-dotted COUNTER
// subset's DeltaSum converges to == numReq AND at least one Export has arrived, then
// waits for stabilityFlushes further Exports (the stability barrier) and snapshots the
// convergence-time + post-barrier DeltaSums, all datapoints, and the resource-attr keys.
func (d *otlpKnobsDriver) driveSide(ctx context.Context, addr string, srv *otlpmetrics.Server, side string) ([]byte, sideSnapshot, error) {
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

	// DeltaSum sample #1 at convergence (each subset counter == numReq).
	sum1 := deltaSums(srv)

	// The STABILITY BARRIER: wait for >=stabilityFlushes further Exports, then re-read
	// DeltaSum. Under DELTA the now-idle counters emit a 0 delta each flush, so the sum
	// stays numReq; an absolute emitter re-adds the cumulative every flush, so the sum
	// OVERSHOOTS numReq and the post-barrier assertion fires. This is a release barrier
	// on the flush ticker (Export arrival), NOT a sleep.
	base := srv.ExportCount()
	if err := pollExportCount(ctx, srv, base+stabilityFlushes); err != nil {
		dumpDatapoints(side, srv)
		return nil, sideSnapshot{}, err
	}
	sum2 := deltaSums(srv)

	snap := sideSnapshot{
		deltaSum1: sum1,
		deltaSum2: sum2,
		dps:       srv.Datapoints(),
		resKeys:   resourceKeySet(srv),
	}
	dumpDatapoints(side, srv)
	return b.Bytes(), snap, nil
}

// deltaSums reads the running DeltaSum for each subset counter into a fresh map.
func deltaSums(srv *otlpmetrics.Server) map[string]float64 {
	out := make(map[string]float64, len(subsetCounters))
	for _, name := range subsetCounters {
		v, _ := srv.DeltaSum(name)
		out[name] = v
	}
	return out
}

// fireProbe issues one query-less GET probePath with the fixed Host + User-Agent and
// returns the response status code (the body is drained and discarded).
func (d *otlpKnobsDriver) fireProbe(ctx context.Context, client *http.Client, addr string) (int, error) {
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

// pollSubset spins until every full-dotted subset counter has a running DeltaSum ==
// numReq AND at least one Export has arrived (the served-this-arm precondition — a
// zero-Export pass is a false green). This is the release barrier
// (reference_concurrency_differential_release_barrier).
func pollSubset(ctx context.Context, srv *otlpmetrics.Server) error {
	deadline := time.Now().Add(pollDeadline)
	for {
		if srv.ExportCount() > 0 && subsetConverged(srv) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("otlp receiver: timed out waiting for DeltaSum subset == %d + an Export (exports=%d, %s)",
				numReq, srv.ExportCount(), describeSubset(srv))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("otlp receiver: context done waiting for DeltaSum subset == %d (%s): %w",
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

// subsetConverged is true iff every full-dotted subset counter's running DeltaSum is
// exactly numReq.
func subsetConverged(srv *otlpmetrics.Server) bool {
	for _, name := range subsetCounters {
		v, ok := srv.DeltaSum(name)
		if !ok || v != float64(numReq) {
			return false
		}
	}
	return true
}

// describeSubset renders the current subset DeltaSums for a diagnostic.
func describeSubset(srv *otlpmetrics.Server) string {
	var b bytes.Buffer
	for _, name := range subsetCounters {
		v, ok := srv.DeltaSum(name)
		fmt.Fprintf(&b, "%s=%v(ok=%v) ", name, v, ok)
	}
	return b.String()
}

// findByName returns the (first) accumulated datapoint named name (with no
// attribute-qualification — under emit_tags_as_attributes:false there is exactly one
// datapoint per name). ok is false if absent.
func findByName(dps []otlpmetrics.DatapointView, name string) (otlpmetrics.DatapointView, bool) {
	for _, dp := range dps {
		if dp.Name == name {
			return dp, true
		}
	}
	return otlpmetrics.DatapointView{}, false
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

// dumpDatapoints logs every accumulated datapoint (name + sorted attrs + value + type
// + DeltaSum) when FIXTURE_0113_DUMP is set — the empirical prefixed-name confirmation
// surface.
func dumpDatapoints(side string, srv *otlpmetrics.Server) {
	if os.Getenv("FIXTURE_0113_DUMP") == "" {
		return
	}
	dps := srv.Datapoints()
	sort.Slice(dps, func(i, j int) bool { return dps[i].Name < dps[j].Name })
	fmt.Fprintf(os.Stderr, "=== 0113 %s datapoints (exports=%d) ===\n", side, srv.ExportCount())
	for _, dp := range dps {
		kind := "GAUGE"
		if dp.IsSum {
			kind = fmt.Sprintf("SUM(temp=%v,mono=%v,start=%d)", dp.Temporality, dp.IsMonotonic, dp.StartTime)
		}
		ds, _ := srv.DeltaSum(dp.Name)
		fmt.Fprintf(os.Stderr, "  %-52s %-10s v=%v deltaSum=%v attrs=%s\n", dp.Name, kind, dp.Value, ds, attrsString(dp.Attrs))
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
func (*otlpKnobsDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertStats asserts the KNOB-arm proposition on BOTH sides (named subsets only).
func (d *otlpKnobsDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	assertSide(t, "reference", d.ref)
	assertSide(t, "subject", d.subj)
}

// assertSide asserts one side's snapshot: the full-dotted counter subset's DeltaSum
// == K at convergence AND after the barrier (stays K); each counter's latest
// datapoint is a DELTA monotonic Sum with NO attributes; a gauge stays absolute; the
// telemetry.sdk.* resource keys present.
func assertSide(t fixture.TB, side string, snap sideSnapshot) {
	t.Helper()

	if len(snap.dps) == 0 {
		t.Fatalf("%s: no datapoints captured (Export did not run)", side)
	}

	// (1) The full-dotted counter subset: running DeltaSum == K at convergence AND
	// STILL == K after >=stabilityFlushes further flushes (the stability barrier —
	// an absolute emitter overshoots K here). Then the latest datapoint for each is a
	// DELTA monotonic Sum with NO attributes (both-false).
	for _, name := range subsetCounters {
		if got := snap.deltaSum1[name]; got != float64(numReq) {
			t.Errorf("%s: %q DeltaSum at convergence = %v, want %d (== K)", side, name, got, numReq)
		}
		if got := snap.deltaSum2[name]; got != float64(numReq) {
			t.Errorf("%s: %q DeltaSum after >=%d further flushes = %v, want %d (barrier: DELTA, not absolute)",
				side, name, stabilityFlushes, got, numReq)
		}
		assertDeltaCounter(t, side, snap.dps, name)
	}

	// (2) A GAUGE stays ABSOLUTE — emitted as a Gauge, NOT a delta Sum (the knob
	// applies to counters only). VALUE unasserted.
	if dp, ok := findByName(snap.dps, gaugeName); !ok {
		t.Errorf("%s: gauge %q absent", side, gaugeName)
	} else if dp.IsSum {
		t.Errorf("%s: gauge %q emitted as a Sum (temp=%v) — under report_counters_as_deltas a gauge must stay an absolute Gauge", side, gaugeName, dp.Temporality)
	}

	// (3) The three telemetry.sdk.* resource KEYS present (per-side values UNasserted).
	for _, k := range resourceKeys {
		if !snap.resKeys[k] {
			t.Errorf("%s: resource attribute key %q absent (have %v)", side, k, sortedKeys(snap.resKeys))
		}
	}
}

// assertDeltaCounter selects the datapoint named name and asserts it is a DELTA Sum
// with IsMonotonic RETAINED (true) and NO attributes (emit_tags_as_attributes:false).
func assertDeltaCounter(t fixture.TB, side string, dps []otlpmetrics.DatapointView, name string) {
	t.Helper()
	dp, ok := findByName(dps, name)
	if !ok {
		t.Errorf("%s: counter %q datapoint absent", side, name)
		return
	}
	if !dp.IsSum {
		t.Errorf("%s: %q is not a Sum (Gauge) — want a DELTA monotonic Sum", side, name)
		return
	}
	if dp.Temporality != metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA {
		t.Errorf("%s: %q Sum temporality = %v, want DELTA", side, name, dp.Temporality)
	}
	if !dp.IsMonotonic {
		t.Errorf("%s: %q Sum IsMonotonic = false, want true (RETAINED under DELTA)", side, name)
	}
	if len(dp.Attrs) != 0 {
		t.Errorf("%s: %q has %d attributes %s, want none (emit_tags_as_attributes:false)", side, name, len(dp.Attrs), attrsString(dp.Attrs))
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// --- file / template helpers ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0113-stats-sink-otlp-knobs/driver/driver.go
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
	_ fixture.Driver           = (*otlpKnobsDriver)(nil)
	_ fixture.BackendKindAware = (*otlpKnobsDriver)(nil)
	_ fixture.StatsAsserter    = (*otlpKnobsDriver)(nil)
)
