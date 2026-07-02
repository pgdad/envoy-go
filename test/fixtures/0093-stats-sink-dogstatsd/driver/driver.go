// Package driver registers the 0093-stats-sink-dogstatsd fixture with the
// differential runner. It is the behavioral proof of the phase 49 dog_statsd UDP
// line-protocol stats sink WITH TAGS: cross-side EXACT (subject envoy-go vs
// reference Envoy contrib-v1.37.2 in Docker) on a deterministic COUNTER WIRE-NAME
// subset (post-stats.ExtractTags, prefix-joined: <prefix>.<residual>), asserting
// BOTH the per-flush DELTA-SUM model AND the extracted tag SET per name. PLUS the
// absolute GAUGE subset (cluster.membership_total == 1, tagged with cluster_name).
//
// Delta-SUM model (dog_statsd |c counters):
// the dog_statsd sink emits each COUNTER's per-flush DELTA as a |c line
// (<prefix>.<residual>:<delta>|c[|#tags]). Because the first flush's delta ==
// the cumulative-to-date (all requests burst before the flush), a single-window
// burst is INDISTINGUISHABLE from a broken absolute-emit. The post-convergence
// stability barrier (awaitFurtherFlushes — waits for >= 2 further flushes after
// the delta-SUM converges to K) proves the delta model: under correct deltas the
// idle counters emit 0 each flush so the SUM stays K; an absolute sink re-adds
// the cumulative and the SUM overshoots K (reference_delta_sink_differential_stability_barrier).
//
// WIRE-name correction (the load-bearing diff vs 0092): dog_statsd applies
// stats.ExtractTags (0092's plain statsd sink does not), so the residual +
// prefix join differs from 0092's dotted name for the "same" underlying stat:
//
//	cluster.<backendName>.upstream_rq_total -> wire "cluster.upstream_rq_total"  (backendName HOISTED to a tag)
//	http.<statPrefix>.downstream_rq_total   -> wire "http.downstream_rq_total"   (statPrefix HOISTED to a tag)
//	http.<statPrefix>.downstream_rq_2xx     -> wire "http.downstream_rq_xx"      (SN4 REWRITES _2xx->_xx + hoists the digit to a tag)
//
// Gauge subset (dog_statsd |g):
// cluster.membership_total is emitted as an ABSOLUTE |g line each flush
// (absolute value, not delta), tagged with cluster_name. The driver asserts the
// captured gauge value == 1 (one backend endpoint) AND its tag set. This is the
// D-DSD-GAUGE-SUBSET.
//
// Integration shape (single-listener plaintext H1; driver-owned in-process
// statsdrecv UDP receivers, extended with a Tags() accessor in Task 6;
// HTTPFixedBody backend):
//
//  1. TWO driver-owned statsdrecv.Server receivers — one per side — on two
//     separately-allocated host ports (refStatsdPort / subjStatsdPort), both
//     bound on 0.0.0.0:<port> BEFORE either proxy starts. The same two-receiver
//     discipline as the 0092 statsd fixture: the dog_statsd sink flushes
//     PERIODICALLY, so the reference keeps sending to its receiver during the
//     subject's drive window. Two receivers give each side a private,
//     uncontaminated accumulator (reference_periodic_sink_differential_two_receivers).
//     The reference bootstrap is templated with the LITERAL host-gateway IP (what
//     Docker resolves "host-gateway" to: the bridge gateway on native Linux Docker,
//     the VM->host gateway e.g. 192.168.65.2 on Docker Desktop) as the dog_statsd
//     address — the dog_statsd sink rejects hostnames, the statsd sink precedent
//     (AMEND-SD-REJECT; reference_docker_probe_bridge_network). The subject
//     bootstrap uses 127.0.0.1:<subjStatsdPort>.
//
//  2. DriveReference / DriveSubject each fire K=7 deterministic GET requests
//     (all 2xx) through the proxy listener, then POLL that side's private
//     receiver until the deterministic COUNTER subset's delta-SUM converges to
//     == K, then await >= 2 further flushes (the stability barrier). The
//     per-side snapshot (the three subset delta-SUMs + their tag sets + the
//     gauge value + its tag set) is captured from that side's own receiver
//     before the next side runs.
//
//  3. AssertStats asserts, on BOTH sides: the deterministic COUNTER WIRE-NAME
//     subset present with delta-SUM == K AND its extracted tag set matching
//     subsetTags; the absolute gauge cluster.membership_total == 1 with its tag
//     set matching gaugeTags. Decode-ran proof: the poll-to-converge guarantees
//     the subset arrived on each side's receiver before asserting — a
//     zero-datagram pass is structurally impossible.
//
// UNasserted (surfaces differ / impl-specific / non-deterministic):
// the WHOLE datagram/line set (the reference emits many more lines including
// |ms timer histograms that envoy-go lacks); non-deterministic gauges
// (server.uptime, *_active, connection churn); per-datagram framing + line
// order; |ms timer lines; the literal tag ORDER on the wire (the assertion is
// order-independent map equality even though production now matches the
// reference's natural order — reference_dogstatsd_tag_order_unsorted); the
// identifier (dog_statsd has none); stream/message framing (UDP is
// connectionless).
//
// The same-name/different-tag admin collision (discovered running the live
// differential, not anticipated by the PLAN): the bootstrap's admin interface
// has its OWN internal http_conn_manager stats (stat_prefix "admin"), which,
// via the SAME SN2 hoist, collapse to the SAME residual wire names as the test
// listener's ("http.downstream_rq_total", "http.downstream_rq_xx") — differing
// only in the envoy.http_conn_manager_prefix tag VALUE. The reference
// container's admin readiness poll (wait.ForHTTP("/ready"), harness.go) is
// itself one such "admin"-tagged 2xx request, so a name-only DeltaSum
// converges to K+1 (never K) on the reference side — the plain-statsd 0092
// fixture never hit this because its RAW dotted names ("...hcm_local..." vs
// "...admin...") never collided. Fixed by statsdrecv.Server.DeltaSumTagged
// (a new, additive accessor — the per-name-per-exact-tag-set running sum),
// used throughout this driver in place of the plain DeltaSum for the counter
// subset.
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

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
	"github.com/esalaine/envoy-go/test/helpers/statsdrecv"
)

const (
	fixtureName = "0093-stats-sink-dogstatsd"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0093 takes 10093 for the single plaintext listener.
	refAdminPort    = 9901
	refListenerPort = 10093

	// numReq is the per-side request count K. After K 2xx requests the
	// deterministic COUNTER subset's per-flush deltas SUM to == K on each side.
	numReq = 7

	// The fixed request shape (kept identical cross-side; query-less path).
	probePath = "/probe"
	probeHost = "dogstatsd.example"
	probeUA   = "dogstatsd-probe/1"

	// The HCM stat_prefix + backend cluster name baked IDENTICALLY into both
	// bootstraps, so the extracted tag values match cross-side.
	statPrefix  = "hcm_local"
	backendName = "c_backend"

	// The dog_statsd metric prefix baked identically on both sides. DISTINCT
	// from the 0092 statsd fixture's "sdpfx" (a different sink, coexistence not
	// tested here, but the prefixes must not collide if both were ever combined).
	prefix = "dsdpfx"

	// Converge-poll discipline (reference_concurrency_differential_release_barrier):
	// POLL the subset DeltaSum() to == K; never sleep-to-wait.
	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)

// subsetNames is the deterministic COUNTER WIRE (post-stats.ExtractTags) name
// subset — NOT the raw pre-extraction dotted names 0092 uses. dog_statsd applies
// ExtractTags (0092's plain statsd sink does not), so the residual + prefix join
// differs from 0092's for the "same" underlying stat:
//
//	cluster.<backendName>.upstream_rq_total -> wire "cluster.upstream_rq_total"  (backendName HOISTED to a tag)
//	http.<statPrefix>.downstream_rq_total   -> wire "http.downstream_rq_total"   (statPrefix HOISTED to a tag)
//	http.<statPrefix>.downstream_rq_2xx     -> wire "http.downstream_rq_xx"      (SN4 REWRITES _2xx->_xx + hoists the digit)
//
// CONFIRMED at the SPEC-49 §11 live probe: "probepfx.http.downstream_rq_xx:...|c|#envoy.response_code_class:2,...".
var subsetNames = []string{
	prefix + ".cluster.upstream_rq_total",
	prefix + ".http.downstream_rq_total",
	prefix + ".http.downstream_rq_xx",
}

// subsetTags is the expected extracted tag SET (order-independent map equality
// — the differential does NOT depend on the wire's tag ORDER even though the
// production formatter now matches the reference's natural order) per subset name.
var subsetTags = map[string]map[string]string{
	prefix + ".cluster.upstream_rq_total": {"envoy.cluster_name": backendName},
	prefix + ".http.downstream_rq_total":  {"envoy.http_conn_manager_prefix": statPrefix},
	prefix + ".http.downstream_rq_xx":     {"envoy.response_code_class": "2", "envoy.http_conn_manager_prefix": statPrefix},
}

// gaugeName + gaugeTags: the absolute |g subset (D-DSD-GAUGE-SUBSET), mirroring
// 0092's membership_total-not-membership_healthy precedent
// (reference_membership_total_vs_healthy_gauge — the 0093 cluster, cloned from
// 0092, carries no health_checks).
var gaugeName = prefix + ".cluster.membership_total"
var gaugeTags = map[string]string{"envoy.cluster_name": backendName}

func init() {
	fixture.RegisterFixture(fixtureName, &statsdDriver{})
}

// sideSnapshot is the per-side captured state: the three subset delta-SUMs +
// their tag sets + the gauge value + its tag set.
type sideSnapshot struct {
	sums      map[string]float64
	tags      map[string]map[string]string
	gaugeVal  float64
	gaugeOK   bool
	gaugeTags map[string]string
}

// statsdDriver carries the per-driver lifecycle state — TWO private statsdrecv
// UDP receivers (one per side; the reference's periodic flushes must not
// contaminate the subject snapshot) on two separately-allocated host ports, plus
// the per-side snapshots captured during Drive for the AssertStats cross-side
// assertion.
type statsdDriver struct {
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
func (d *statsdDriver) ensure() {
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

func (*statsdDriver) BackendCount() int                { return 1 }
func (*statsdDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }
func (*statsdDriver) SubjectListenerName() string      { return "l_test" }
func (*statsdDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// runner-allocated backend port + the reference-side statsdrecv receiver
// address (LITERAL HostGatewayIP:refStatsdPort). It allocates the ports and
// starts both receivers here so they are live before the reference container
// boots.
func (d *statsdDriver) ReferenceBootstrap(backendPorts []int) string {
	d.ensure()
	gwIP, err := hostGatewayIP(context.Background())
	if err != nil {
		panic(fmt.Sprintf("driver: hostGatewayIP: %v", err))
	}
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":     refAdminPort,
		"ListenerPort":  refListenerPort,
		"BackendHost":   "host.docker.internal",
		"BackendPort":   backendPorts[0],
		"DogStatsdHost": gwIP,
		"DogStatsdPort": d.refStatsdPort,
		"Prefix":        prefix,
		"StatPrefix":    statPrefix,
		"BackendName":   backendName,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
// + backend port (loopback) + the subject-side statsdrecv receiver address
// (127.0.0.1:subjStatsdPort).
func (d *statsdDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":     subjAdminPort,
		"ListenerPort":  subjListenerPort,
		"BackendPort":   backendPorts[0],
		"DogStatsdHost": "127.0.0.1",
		"DogStatsdPort": d.subjStatsdPort,
		"Prefix":        prefix,
		"StatPrefix":    statPrefix,
		"BackendName":   backendName,
	})
}

// DriveReference fires the workload against the reference proxy and snapshots
// the reference-side subset readings from the reference's own private receiver.
func (d *statsdDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
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
func (d *statsdDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
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
func (d *statsdDriver) closeServers() {
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
// gauge + its tag set. No Reset() is needed: each side owns a private receiver.
func (d *statsdDriver) driveSide(ctx context.Context, addr string, srv *statsdrecv.Server) ([]byte, sideSnapshot, error) {
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
		// subsetTags[name] contribute. This was discovered empirically running
		// the live differential (a bare DeltaSum(name) converged to K+1, never
		// K) — see the package doc on statsdrecv.Server.DeltaSumTagged.
		sum, ok := srv.DeltaSumTagged(name, subsetTags[name])
		snap.sums[name] = sum
		if ok {
			snap.tags[name] = subsetTags[name]
		}
	}
	snap.gaugeVal, snap.gaugeOK = srv.Gauge(gaugeName)
	snap.gaugeTags, _ = srv.Tags(gaugeName)
	return b.Bytes(), snap, nil
}

// fireProbe issues one query-less GET probePath with the fixed Host + User-Agent
// and returns the response status code (the body is drained and discarded).
func (d *statsdDriver) fireProbe(ctx context.Context, client *http.Client, addr string) (int, error) {
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
func (*statsdDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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
// matching gaugeTags. The poll-to-converge already proved the subset delta-SUM
// arrived == numReq on each side's private receiver (decode ran — a
// zero-datagram pass is structurally impossible), but the assertion re-checks
// the captured snapshot.
func (d *statsdDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	if os.Getenv("FIXTURE_0093_DUMP") != "" {
		fmt.Fprintf(os.Stderr, "=== 0093 ref gauge{%s=%v ok=%v tags=%v} subj gauge{%s=%v ok=%v tags=%v} ===\n",
			gaugeName, d.ref.gaugeVal, d.ref.gaugeOK, d.ref.gaugeTags,
			gaugeName, d.subj.gaugeVal, d.subj.gaugeOK, d.subj.gaugeTags)
		for _, name := range subsetNames {
			fmt.Fprintf(os.Stderr, "  %s: ref=%v(tags=%v) subj=%v(tags=%v)\n",
				name, d.ref.sums[name], d.ref.tags[name], d.subj.sums[name], d.subj.tags[name])
		}
	}

	assertSide(t, "reference", d.ref)
	assertSide(t, "subject", d.subj)
}

// assertSide asserts the deterministic COUNTER subset (present + delta-SUM ==
// numReq + tag set == subsetTags) and the absolute gauge (== 1 + tag set ==
// gaugeTags) for one side's snapshot.
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
// IP — the dog_statsd UDP sink rejects hostnames, the statsd sink precedent
// (AMEND-SD-REJECT; reference_docker_probe_bridge_network). Mirrors
// differential.HostGatewayIP but inlined here to avoid an import cycle
// (runner_test.go blank-imports this driver from within the `differential`
// package, so this driver cannot import it).
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
	// thisFile is .../test/fixtures/0093-stats-sink-dogstatsd/driver/driver.go
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
	_ fixture.Driver           = (*statsdDriver)(nil)
	_ fixture.BackendKindAware = (*statsdDriver)(nil)
	_ fixture.StatsAsserter    = (*statsdDriver)(nil)
)
