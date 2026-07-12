// Package driver registers the 0101-stats-sink-graphite fixture with the
// differential runner. It is the merged behavioral proof of the phase 57
// graphite_statsd sink (ADR-0275): the tag-in-name grammar
// "<prefix>.<residual>[;envoy.k=v;…]:<value>|<c|g>" (the ONE novel piece of
// phase 57, D-GR-FIXTURE MERGED — this dir folds together what would
// otherwise have been a separate 0093-shape tags fixture and a separate
// 0094-shape batching fixture, per the SPEC-57 decision that the two proofs
// share one workload) PLUS the shared appendBatchLine/flushBatch REAL
// multi-metric datagram batching (D-GR-BATCHSHARE — hoisted VERBATIM from the
// phase-50 dog_statsd batching code; see internal/statssink/udp.go), proving:
//
//  1. the deterministic COUNTER subset's per-flush delta-SUM converges to ==
//     K on each side, with the extracted tag SET (order-independent —
//     AMEND-GR-TAGORDER, the reference's two-tag order is the REVERSE of
//     ours) matching the expected set;
//  2. the absolute GAUGE subset (cluster.membership_total) == 1 with its tag
//     set;
//  3. at least one datagram observed on each side carried MORE THAN ONE line
//     (statsdrecv.Server.MaxLinesInAnyDatagram() > 1) — batching actually
//     occurred;
//  4. the deliberately oversized envoy.cluster_name-tagged
//     "<prefix>.cluster.upstream_rq_total" line was ALWAYS observed ALONE in
//     its own datagram (statsdrecv.Server.LinesInDatagram(name) == (1, true));
//  5. (NEW, phase 57) the SUBJECT-side receiver's UnparsedCount() == 0 — a
//     correct envoy-go subject emits only |c/|g graphite lines, so every
//     ingested line must parse (reference_framing_break_needs_unparsed_counter).
//     The reference legitimately produces unparsed lines (|ms timer
//     histograms), so its count is NOT asserted (SPEC-57 §8.1).
//
// D-GR-CAP (byte math, computed EXACTLY via a throwaway t.Logf(len(...))
// against the REAL production graphiteTagSuffix/stats.ExtractTags code at
// Task 8 authoring time, not a hand-estimate):
//
//	grpfx.cluster.upstream_rq_total;envoy.cluster_name=<171-char backendName>:7|c -> 226 bytes
//	grpfx.cluster.membership_total;envoy.cluster_name=<171-char backendName>:1|g  -> 225 bytes
//	grpfx.http.downstream_rq_total;envoy.http_conn_manager_prefix=hcm_local:7|c   ->  75 bytes
//	grpfx.http.downstream_rq_xx;envoy.response_code_class=2;envoy.http_conn_manager_prefix=hcm_local:7|c -> 100 bytes
//
// Against maxBytesPerDatagram = 200: the two envoy.cluster_name-tagged lines
// (226/225 bytes) EACH ALONE exceed the cap, so they are ALWAYS flushed alone
// (appendBatchLine flushes any PRIOR buffer content before accepting an
// oversized line into a fresh, empty buffer, and the very next line after an
// oversized one trivially exceeds the cap too, forcing another flush before
// it is appended). The two envoy.http_conn_manager_prefix-tagged lines
// (75/100 bytes) are comfortably under the cap — even together with a "\n"
// separator (75+1+100 = 176 <= 200) they FIT in one datagram, and in practice
// co-batch with each other and the many other short envoy-go/reference
// self-stat lines that flush every interval. The PLAN's ~171-char
// backendName estimate needed NO adjustment — the computed 226/225-byte
// lines already clear the 200-byte cap with a comfortable margin, and the
// 75/100-byte HCM lines already sit far enough under it that co-batching is
// not marginal (identical shape to the 0094 precedent's 172-char backendName,
// re-derived here for the graphite tag-in-name overhead which trims 2-3 bytes
// off the dog_statsd "|#" suffix form per line).
//
// Everything else in this driver — the workload shape, the release barrier,
// the post-convergence stability barrier, the DeltaSumTagged admin-collision
// fix, the two-private-receivers + hard Close() discipline, the LOCAL
// hostGatewayIP duplicate (import-cycle avoidance) — is cloned VERBATIM from
// 0094-stats-sink-dogstatsd-batching's driver (itself cloned from
// 0093-stats-sink-dogstatsd); see that package's doc comment for the full
// design rationale (WIRE-name correction, delta-SUM model, admin same-name
// collision, tag-order-unsorted, etc.), which applies here unchanged. This
// file re-keys the constants to phase-57 values (a DISTINCT prefix "grpfx"
// so this fixture's graphite traffic never collides with 0092/0093/0094/0098
// if ever run against a shared receiver), swaps the stats_sinks[] block for
// graphite_statsd, and adds the subject-only UnparsedCount==0 proof.
package driver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"github.com/pgdad/envoy-go/test/helpers/statsdrecv"
)

const (
	fixtureName = "0101-stats-sink-graphite"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0101 → 10101.
	refAdminPort    = 9901
	refListenerPort = 10101

	// numReq is the per-side request count K. After K 2xx requests the
	// deterministic COUNTER subset's per-flush deltas SUM to == K on each side.
	numReq = 7

	// The fixed request shape (kept identical cross-side; query-less path).
	probePath = "/probe"
	probeHost = "graphite.example"
	probeUA   = "graphite-probe/1"

	// statPrefix stays SHORT so the envoy.http_conn_manager_prefix-tagged lines
	// stay small enough to naturally co-batch under the cap.
	statPrefix = "hcm_local"

	// backendName is DELIBERATELY LONG (171 chars, confirmed at Task 8
	// authoring time via a throwaway t.Logf(len(...)) against the REAL
	// production graphiteTagSuffix/stats.ExtractTags code — see the package
	// doc's byte-math table) so the envoy.cluster_name-tagged lines
	// (cluster.upstream_rq_total, cluster.membership_total) exceed
	// maxBytesPerDatagram ALONE (226/225 bytes > 200), while the
	// envoy.http_conn_manager_prefix-tagged lines (tagged with the SHORT
	// statPrefix, 75/100 bytes) stay small and naturally co-batch with each
	// other and the many other short envoy-go/reference self-stat lines.
	// COUPLED with numReq and maxBytesPerDatagram — never change one alone
	// (reference_fixture_workload_constant_desync).
	backendName = "very_long_backend_cluster_name_deliberately_chosen_to_force_the_envoy_cluster_name_tagged_graphite_lines_past_the_configured_max_bytes_per_datagram_cap_for_this_fixture_gr"

	// The graphite_statsd metric prefix baked identically on both sides.
	// DISTINCT from 0092/0093/0094/0098's prefixes — a different fixture; must
	// not collide if ever run against a shared receiver.
	prefix = "grpfx"

	// maxBytesPerDatagram: the configured cap (D-GR-CAP). Large enough to
	// batch the short HCM-tagged lines together (comfortably, 75/100 bytes);
	// comfortably smaller than the long-cluster-name-tagged lines (226/225
	// bytes), which are therefore ALWAYS sent alone.
	maxBytesPerDatagram = 200

	// Converge-poll discipline (reference_concurrency_differential_release_barrier):
	// POLL the subset DeltaSum() to == K; never sleep-to-wait.
	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)

// subsetNames is the deterministic COUNTER WIRE (post-stats.ExtractTags,
// post-tag-strip) name subset. The receiver strips the ";k=v" tag suffix back
// out of the graphite name before keying (Task 7), so lookup keys are the
// SAME STRIPPED "<prefix>.<residual>" names the dog_statsd fixtures use —
// IDENTICAL IN SHAPE to 0093/0094's, re-keyed to this fixture's prefix/
// statPrefix/backendName.
var subsetNames = []string{
	prefix + ".cluster.upstream_rq_total",
	prefix + ".http.downstream_rq_total",
	prefix + ".http.downstream_rq_xx",
}

// subsetTags is the expected extracted tag SET (order-independent map
// equality) per subset name — IDENTICAL IN SHAPE to 0093/0094's. NO `_NNN`
// exact-code names (AMEND-GR-EXACTCODE-SUBSET); `_xx` class names only.
var subsetTags = map[string]map[string]string{
	prefix + ".cluster.upstream_rq_total": {"envoy.cluster_name": backendName},
	prefix + ".http.downstream_rq_total":  {"envoy.http_conn_manager_prefix": statPrefix},
	prefix + ".http.downstream_rq_xx":     {"envoy.response_code_class": "2", "envoy.http_conn_manager_prefix": statPrefix},
}

// gaugeName + gaugeTags: the absolute |g subset (D-DSD-GAUGE-SUBSET, 0093/0094
// precedent) — membership_total (not membership_healthy), this cluster
// carries no health_checks (reference_membership_total_vs_healthy_gauge).
var gaugeName = prefix + ".cluster.membership_total"
var gaugeTags = map[string]string{"envoy.cluster_name": backendName}

func init() {
	fixture.RegisterFixture(fixtureName, &graphiteDriver{})
}

// clusterTaggedName is the deliberately oversized line's wire name — the
// SAME name as subsetNames[0], named separately here for clarity at each
// batching-proof call site.
const clusterTaggedName = prefix + ".cluster.upstream_rq_total"

// sideSnapshot is the per-side captured state: the three subset delta-SUMs +
// their tag sets + the gauge value + its tag set, the two batching-specific
// proofs (largest line-count ever observed in a single datagram, and how many
// lines were in the datagram that most recently carried the
// deliberately-oversized cluster-tagged line), PLUS (phase 57, Task 8) the
// receiver's UnparsedCount() — asserted SUBJECT-ONLY.
type sideSnapshot struct {
	sums      map[string]float64
	tags      map[string]map[string]string
	gaugeVal  float64
	gaugeOK   bool
	gaugeTags map[string]string

	maxLinesInDatagram int
	linesInDatagram    map[string]int

	// unparsed (phase 57, Task 8): the count of lines the receiver could not
	// account for. Asserted == 0 SUBJECT-ONLY (reference_framing_break_needs_unparsed_counter).
	unparsed int
}

// graphiteDriver carries the per-driver lifecycle state — TWO private
// statsdrecv UDP receivers (one per side; the reference's periodic flushes
// must not contaminate the subject snapshot) on two separately-allocated host
// ports, plus the per-side snapshots captured during Drive for the
// AssertStats cross-side assertion.
type graphiteDriver struct {
	mu sync.Mutex

	refStatsdPort  int
	subjStatsdPort int
	refSrv         *statsdrecv.Server
	subjSrv        *statsdrecv.Server

	ref  sideSnapshot
	subj sideSnapshot
}

// ensure allocates the two receiver ports (via Listen+Close on UDP) and starts
// both in-process statsdrecv UDP receivers bound to 0.0.0.0:<port> (so BOTH
// the reference container and the subject can send datagrams to their respective
// receiver). Idempotent — safe to call from both ReferenceBootstrap and
// SubjectConfig regardless of order; a second call is a no-op while the servers
// run. Both servers are live before either proxy boots.
func (d *graphiteDriver) ensure() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.refStatsdPort == 0 {
		d.refStatsdPort = mustAllocateUDPPort()
	}
	if d.subjStatsdPort == 0 {
		d.subjStatsdPort = mustAllocateUDPPort()
	}
	if d.refSrv == nil {
		d.refSrv = mustStartReceiver(d.refStatsdPort)
	}
	if d.subjSrv == nil {
		d.subjSrv = mustStartReceiver(d.subjStatsdPort)
	}
}

// mustAllocateUDPPort reserves a free UDP port via ListenUDP+Close.
func mustAllocateUDPPort() int {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: resolve udp addr: %v", err))
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		panic(fmt.Sprintf("driver: allocate udp port: %v", err))
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	_ = conn.Close()
	return port
}

// mustStartReceiver starts an in-process statsdrecv UDP receiver bound to
// 0.0.0.0:<port>.
func mustStartReceiver(port int) *statsdrecv.Server {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	srv, err := statsdrecv.NewAtAddr(addr)
	if err != nil {
		panic(fmt.Sprintf("driver: start statsd receiver on %s: %v", addr, err))
	}
	return srv
}

// --- fixture.Driver (required) ---

func (*graphiteDriver) BackendCount() int                { return 1 }
func (*graphiteDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }
func (*graphiteDriver) SubjectListenerName() string      { return "l_test" }
func (*graphiteDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// runner-allocated backend port + the reference-side statsdrecv receiver
// address (LITERAL HostGatewayIP:refStatsdPort) + the configured
// max_bytes_per_datagram cap. It allocates the ports and starts both
// receivers here so they are live before the reference container boots.
func (d *graphiteDriver) ReferenceBootstrap(backendPorts []int) string {
	d.ensure()
	gwIP, err := hostGatewayIP(context.Background())
	if err != nil {
		panic(fmt.Sprintf("driver: hostGatewayIP: %v", err))
	}
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":           refAdminPort,
		"ListenerPort":        refListenerPort,
		"BackendHost":         "host.docker.internal",
		"BackendPort":         backendPorts[0],
		"GraphiteHost":        gwIP,
		"GraphitePort":        d.refStatsdPort,
		"Prefix":              prefix,
		"StatPrefix":          statPrefix,
		"BackendName":         backendName,
		"MaxBytesPerDatagram": maxBytesPerDatagram,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
// + backend port (loopback) + the subject-side statsdrecv receiver address
// (127.0.0.1:subjStatsdPort) + the configured max_bytes_per_datagram cap.
func (d *graphiteDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":           subjAdminPort,
		"ListenerPort":        subjListenerPort,
		"BackendPort":         backendPorts[0],
		"GraphiteHost":        "127.0.0.1",
		"GraphitePort":        d.subjStatsdPort,
		"Prefix":              prefix,
		"StatPrefix":          statPrefix,
		"BackendName":         backendName,
		"MaxBytesPerDatagram": maxBytesPerDatagram,
	})
}

// DriveReference fires the workload against the reference proxy and snapshots
// the reference-side subset readings from the reference's own private receiver.
func (d *graphiteDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
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
// subject-side subset readings from the subject's own private receiver. After
// the subject snapshot BOTH receivers are hard-stopped via Close() for
// deterministic teardown — UDP is connectionless, so Close is unambiguous (no
// GracefulStop-vs-hard-stop distinction; cf. the metricsservice gRPC precedent
// in 0090).
func (d *graphiteDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
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
func (d *graphiteDriver) closeServers() {
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
// COUNTER subset's delta-SUM converges to == numReq, awaits >= 2 further flushes
// (the stability barrier), and snapshots the subset delta-SUMs + tag sets + the
// gauge + its tag set, the batching-specific MaxLinesInAnyDatagram() +
// LinesInDatagram(clusterTaggedName) readings, PLUS (phase 57, Task 8) the
// receiver's UnparsedCount(). The batching proofs are read AFTER the stability
// barrier, same as the value/tag-correctness readings — by then several
// flushes have occurred on each side, giving MaxLinesInAnyDatagram() every
// opportunity to have observed a multi-line datagram, and
// LinesInDatagram(clusterTaggedName) reflects the LATEST flush's
// oversized-line-alone behavior. No Reset() is needed: each side owns a
// private receiver.
func (d *graphiteDriver) driveSide(ctx context.Context, addr string, srv *statsdrecv.Server) ([]byte, sideSnapshot, error) {
	if srv == nil {
		return nil, sideSnapshot{}, fmt.Errorf("driver: statsd receiver not running")
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
	// per-flush delta from absolute: once each subset counter's per-flush
	// delta-SUM reaches numReq, observe >= 2 further flushes (via SeenCount) and
	// snapshot only then. Under correct deltas the now-idle counters emit 0 each
	// flush so the SUM stays numReq; an absolute (or un-latched) sink re-adds
	// the cumulative every flush, so the SUM overshoots numReq and the assertSide
	// check fails. This is the release barrier (reference_delta_sink_differential_stability_barrier).
	if err := awaitFurtherFlushes(ctx, srv, subsetNames[0], 2); err != nil {
		return nil, sideSnapshot{}, err
	}

	snap := sideSnapshot{
		sums: make(map[string]float64, len(subsetNames)),
		tags: make(map[string]map[string]string, len(subsetNames)),
	}
	for _, name := range subsetNames {
		// DeltaSumTagged, NOT the plain per-name DeltaSum: the bootstrap's admin
		// interface has its OWN internal http_conn_manager stats (stat_prefix
		// "admin") which, via stats.ExtractTags's SN2 hoist, collapse to the SAME
		// residual wire names as the test listener's ("http.downstream_rq_total",
		// "http.downstream_rq_xx") — differing only in the
		// envoy.http_conn_manager_prefix tag VALUE ("admin" vs "hcm_local"). A
		// name-only sum would be inflated by the admin interface's own
		// readiness-probe traffic (the wait.ForHTTP("/ready") startup poll);
		// DeltaSumTagged requires an EXACT tag-set match, so only lines carrying
		// subsetTags[name] contribute.
		sum, ok := srv.DeltaSumTagged(name, subsetTags[name])
		snap.sums[name] = sum
		if ok {
			snap.tags[name] = subsetTags[name]
		}
	}
	snap.gaugeVal, snap.gaugeOK = srv.Gauge(gaugeName)
	snap.gaugeTags, _ = srv.Tags(gaugeName)

	snap.maxLinesInDatagram = srv.MaxLinesInAnyDatagram()
	snap.linesInDatagram = make(map[string]int, 1)
	if n, ok := srv.LinesInDatagram(clusterTaggedName); ok {
		snap.linesInDatagram[clusterTaggedName] = n
	}

	// (phase 57, Task 8) SUBJECT-EXACT: recorded on BOTH sides here for
	// symmetry, but only the subject's value is asserted in AssertStats — see
	// sideSnapshot.unparsed's doc.
	snap.unparsed = srv.UnparsedCount()

	return b.Bytes(), snap, nil
}

// fireProbe issues one query-less GET probePath with the fixed Host + User-Agent
// and returns the response status code (the body is drained and discarded).
func (d *graphiteDriver) fireProbe(ctx context.Context, client *http.Client, addr string) (int, error) {
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

// pollSubset spins until every name in the deterministic COUNTER subset has
// delta-SUM == numReq (or the context / deadline elapses). The proxy flushes
// every stats_flush_interval (500ms), so a fixed sleep would be both flaky and
// slow; the poll converges as soon as the deltas have summed to K. This is the
// release barrier (reference_concurrency_differential_release_barrier).
func pollSubset(ctx context.Context, srv *statsdrecv.Server) error {
	deadline := time.Now().Add(pollDeadline)
	for {
		if subsetConverged(srv) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("statsd receiver: timed out waiting for COUNTER subset delta-SUM == %d (%s)",
				numReq, describeSubset(srv))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("statsd receiver: context done waiting for COUNTER subset delta-SUM == %d (%s): %w",
				numReq, describeSubset(srv), ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// awaitFurtherFlushes blocks until the receiver's SeenCount(marker) has advanced
// by at least `extra` from its value on entry (each flush increments SeenCount
// for active names — including a 0-delta |c line on idle counters — so SeenCount
// is a reliable flush-count signal). This is the stability barrier.
func awaitFurtherFlushes(ctx context.Context, srv *statsdrecv.Server, marker string, extra int) error {
	base := srv.SeenCount(marker)
	deadline := time.Now().Add(pollDeadline)
	for srv.SeenCount(marker) < base+extra {
		if time.Now().After(deadline) {
			return fmt.Errorf("statsd receiver: timed out waiting for %d further flushes (seen=%d base=%d)", extra, srv.SeenCount(marker), base)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("statsd receiver: context done waiting for %d further flushes: %w", extra, ctx.Err())
		case <-time.After(pollInterval):
		}
	}
	return nil
}

// subsetConverged is true iff every subset name has a TAG-MATCHED delta-SUM
// (DeltaSumTagged against subsetTags[name]) of exactly numReq. Tag-matched, not
// the plain per-name DeltaSum, to avoid cross-contamination from the admin
// interface's own same-named HCM counters (see the DeltaSumTagged doc).
func subsetConverged(srv *statsdrecv.Server) bool {
	for _, name := range subsetNames {
		v, ok := srv.DeltaSumTagged(name, subsetTags[name])
		if !ok || v != float64(numReq) {
			return false
		}
	}
	return true
}

// describeSubset renders the current subset tag-matched delta-SUMs for a
// timeout diagnostic.
func describeSubset(srv *statsdrecv.Server) string {
	var b bytes.Buffer
	for _, name := range subsetNames {
		v, ok := srv.DeltaSumTagged(name, subsetTags[name])
		fmt.Fprintf(&b, "%s=%v(ok=%v) ", name, v, ok)
	}
	return b.String()
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// the raw response bytes for the standard admin-diff at the runner's probe step.
func (*graphiteDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertStats asserts, on BOTH sides: the deterministic COUNTER WIRE-NAME subset
// present with delta-SUM == numReq AND its extracted tag set matching
// subsetTags; the absolute gauge cluster.membership_total == 1 with its tag set
// matching gaugeTags; the two batching-specific proofs; PLUS (phase 57, Task 8)
// the SUBJECT-ONLY UnparsedCount() == 0 proof. The poll-to-converge already
// proved the subset delta-SUM arrived == numReq on each side's private
// receiver (decode ran — a zero-datagram pass is structurally impossible), but
// the assertion re-checks the captured snapshot.
func (d *graphiteDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	if os.Getenv("FIXTURE_0101_DUMP") != "" {
		fmt.Fprintf(os.Stderr, "=== 0101 ref gauge{%s=%v ok=%v tags=%v} subj gauge{%s=%v ok=%v tags=%v} ===\n",
			gaugeName, d.ref.gaugeVal, d.ref.gaugeOK, d.ref.gaugeTags,
			gaugeName, d.subj.gaugeVal, d.subj.gaugeOK, d.subj.gaugeTags)
		for _, name := range subsetNames {
			fmt.Fprintf(os.Stderr, "  %s: ref=%v(tags=%v) subj=%v(tags=%v)\n",
				name, d.ref.sums[name], d.ref.tags[name], d.subj.sums[name], d.subj.tags[name])
		}
		fmt.Fprintf(os.Stderr, "  batching: ref maxLines=%d %s=%v subj maxLines=%d %s=%v\n",
			d.ref.maxLinesInDatagram, clusterTaggedName, d.ref.linesInDatagram[clusterTaggedName],
			d.subj.maxLinesInDatagram, clusterTaggedName, d.subj.linesInDatagram[clusterTaggedName])
		fmt.Fprintf(os.Stderr, "  unparsed: ref=%d subj=%d\n", d.ref.unparsed, d.subj.unparsed)
	}

	assertSide(t, "reference", d.ref)
	assertSide(t, "subject", d.subj)

	// SUBJECT-EXACT (never cross-side): envoy-go emits only |c/|g graphite
	// lines, so a correct subject leaves the receiver with ZERO unaccountable
	// lines. The reference legitimately produces unparsed lines (|ms timers) —
	// its count is NOT asserted (reference_framing_break_needs_unparsed_counter;
	// SPEC-57 §8.1).
	if d.subj.unparsed != 0 {
		t.Errorf("subject: UnparsedCount() = %d, want 0 (every subject graphite line must parse)", d.subj.unparsed)
	}
}

// assertSide asserts the deterministic COUNTER subset (present + delta-SUM ==
// numReq + tag set == subsetTags) and the absolute gauge (== 1 + tag set ==
// gaugeTags) for one side's snapshot, PLUS the two batching-specific proofs:
// at least one datagram carried more than one line, and the
// deliberately-oversized cluster-tagged line stayed alone in its own
// datagram.
func assertSide(t fixture.TB, side string, snap sideSnapshot) {
	t.Helper()

	if len(snap.sums) == 0 {
		t.Fatalf("%s: no metric counters captured (decode did not run)", side)
	}
	for _, name := range subsetNames {
		sum, present := snap.sums[name]
		if !present {
			t.Fatalf("%s: COUNTER subset %q absent (decode did not run for it)", side, name)
		}
		if sum != float64(numReq) {
			t.Fatalf("%s: counter %q delta-sum = %v, want %d (== K)", side, name, sum, numReq)
		}
		wantTags := subsetTags[name]
		gotTags := snap.tags[name]
		if !maps.Equal(gotTags, wantTags) {
			t.Fatalf("%s: counter %q tags = %v, want %v", side, name, gotTags, wantTags)
		}
	}

	// Absolute gauge subset (D-DSD-GAUGE-SUBSET): cluster.membership_total == 1,
	// tagged with cluster_name.
	if !snap.gaugeOK {
		t.Fatalf("%s: gauge %q absent (expected membership_total|g)", side, gaugeName)
	}
	if snap.gaugeVal != 1 {
		t.Fatalf("%s: gauge %q = %v, want 1", side, gaugeName, snap.gaugeVal)
	}
	if !maps.Equal(snap.gaugeTags, gaugeTags) {
		t.Fatalf("%s: gauge %q tags = %v, want %v", side, gaugeName, snap.gaugeTags, gaugeTags)
	}

	// Batching-specific proofs (ADR-0267 semantics, hoisted D-GR-BATCHSHARE).
	if snap.maxLinesInDatagram <= 1 {
		t.Fatalf("%s: MaxLinesInAnyDatagram() = %d, want > 1 (no batching observed)", side, snap.maxLinesInDatagram)
	}
	if n, ok := snap.linesInDatagram[clusterTaggedName]; !ok || n != 1 {
		t.Fatalf("%s: LinesInDatagram(%q) = (%d, %v), want (1, true) (the long-cluster-name-tagged line must stay alone)", side, clusterTaggedName, n, ok)
	}
}

// hostGatewayIP returns the LITERAL IP a reference Envoy container reaches the
// host at — the value Docker resolves "host-gateway" to (the same endpoint the
// harness's ExtraHosts "host.docker.internal:host-gateway" produces). On native
// Linux Docker that is the bridge gateway (e.g. 172.17.0.1); on Docker Desktop it
// is the VM->host gateway (e.g. 192.168.65.2) — and the bridge IPAM gateway is
// NOT a usable substitute there (it is the VM-internal bridge interface, not the
// host where the in-process statsdrecv receiver binds). So we resolve it the only
// portable way: run a throwaway container with the same host-gateway ExtraHosts
// mapping and read what host.docker.internal resolves to. The result is a literal
// IP — the graphite_statsd UDP sink rejects hostnames, the dog_statsd sink
// precedent (AMEND-SD-REJECT; reference_docker_probe_bridge_network). Mirrors
// differential.HostGatewayIP but inlined here to avoid an import cycle
// (runner_test.go blank-imports this driver from within the `differential`
// package, so this driver cannot import it) — copied VERBATIM from the 0094
// driver.
func hostGatewayIP(ctx context.Context) (string, error) {
	cli, err := testcontainers.NewDockerClientWithOpts(ctx)
	if err != nil {
		return "", fmt.Errorf("host gateway ip: docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	const image = "alpine:3"
	cfg := &container.Config{
		Image:      image,
		Entrypoint: []string{"getent", "hosts", "host.docker.internal"},
		Tty:        true, // raw stdout — no stdcopy demux needed
	}
	hostCfg := &container.HostConfig{
		ExtraHosts: []string{"host.docker.internal:host-gateway"},
	}
	created, err := cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, "")
	if err != nil {
		rc, perr := cli.ImagePull(ctx, image, dockertypes.ImagePullOptions{})
		if perr != nil {
			return "", fmt.Errorf("host gateway ip: pull %q: %w (create: %v)", image, perr, err)
		}
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
		created, err = cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, "")
		if err != nil {
			return "", fmt.Errorf("host gateway ip: create resolver container (image %q): %w", image, err)
		}
	}
	defer func() {
		_ = cli.ContainerRemove(context.Background(), created.ID, dockertypes.ContainerRemoveOptions{Force: true})
	}()

	if err := cli.ContainerStart(ctx, created.ID, dockertypes.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("host gateway ip: start resolver container: %w", err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, created.ID, container.WaitConditionNotRunning)
	select {
	case werr := <-errCh:
		if werr != nil {
			return "", fmt.Errorf("host gateway ip: wait resolver container: %w", werr)
		}
	case <-statusCh:
	case <-ctx.Done():
		return "", fmt.Errorf("host gateway ip: context done waiting for resolver: %w", ctx.Err())
	}

	logs, err := cli.ContainerLogs(ctx, created.ID, dockertypes.ContainerLogsOptions{ShowStdout: true})
	if err != nil {
		return "", fmt.Errorf("host gateway ip: resolver logs: %w", err)
	}
	defer func() { _ = logs.Close() }()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, logs); err != nil {
		return "", fmt.Errorf("host gateway ip: read resolver logs: %w", err)
	}

	out := strings.TrimSpace(buf.String())
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", fmt.Errorf("host gateway ip: resolver produced no output (host.docker.internal unresolved)")
	}
	ip := fields[0]
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("host gateway ip: resolver output %q is not a valid IP (full output: %q)", ip, out)
	}
	return ip, nil
}

// --- file / template helpers (the 0087 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0101-stats-sink-graphite/driver/driver.go
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
	_ fixture.Driver           = (*graphiteDriver)(nil)
	_ fixture.BackendKindAware = (*graphiteDriver)(nil)
	_ fixture.StatsAsserter    = (*graphiteDriver)(nil)
)
