// Package driver registers the 0098-stats-sink-statsd-tcp fixture with the
// differential runner. It is the behavioral proof of the phase 55 statsd TCP
// line-protocol stats sink (ADR-0272): the same statsd wire format as the phase
// 48 UDP sink (0092), but carried over a long-lived TCP CONNECTION the proxy
// opens by dialing a named cluster (StatsdSink.statsd_specifier.tcp_cluster_name)
// rather than a connectionless UDP socket.
//
// Cross-side (subject envoy-go vs reference Envoy contrib-v1.37.2 in Docker) it
// asserts the SAME deterministic COUNTER NAME SUBSET delta-SUM model + absolute
// GAUGE subset as 0092. On TOP of that it adds THREE SUBJECT-EXACT transport
// assertions the UDP fixture cannot make, because the TCP receiver observes the
// connection:
//
//   - ConnCount == 1: envoy-go opens exactly ONE long-lived connection (no
//     per-flush redial). Cross-side equality is infeasible BY THE HISTOGRAM
//     BOUNDARY (AMEND-TCP-CONNCOUNT): the reference opens a SECOND, |ms-only
//     worker-timer connection that envoy-go (no histograms, ADR-0060) can never
//     open, so the reference's ConnCount is 2. The reference's value is RECORDED
//     (via log.Printf), never asserted.
//   - UnparsedCount == 0: envoy-go emits only |c and |g lines, every one
//     '\n'-TERMINATED. A non-zero count is the signature of '\n'-SEPARATED (not
//     TERMINATED) framing concatenating two lines across a flush boundary. This
//     is the LIVENESS signal for the controller's later deliberate framing break.
//     The reference legitimately emits ~35 |ms lines, so its unparsed count is
//     recorded, not asserted.
//   - c_statsd.upstream_cx_total == 0: DialSink took the UNACCOUNTED dial path
//     (AMEND-TCP-CXSTATS) — no max_connections permit, no upstream_cx_*
//     accounting. Subject-only: the reference never emits this line at all
//     (AMEND-TCP-USEDONLY — it omits never-incremented counters), while envoy-go
//     emits the whole registry, so the line MUST be present with value 0.
//
// Delta-SUM model + stability barrier (unchanged from 0092): each COUNTER's
// per-flush DELTA is a |c line; the first flush's delta == the cumulative-to-date
// (all requests burst before the flush), so a single-window burst is
// INDISTINGUISHABLE from a broken absolute-emit. awaitFurtherFlushes (>= 2
// further flushes after the delta-SUM converges to K) proves the delta model:
// under correct deltas the idle counters emit 0 each flush so the SUM stays K; an
// absolute sink re-adds the cumulative and the SUM overshoots
// (reference_delta_sink_differential_stability_barrier).
//
// Integration shape (single-listener plaintext H1; driver-owned in-process
// statsdrecv TCP receivers; HTTPFixedBody backend):
//
//  1. TWO driver-owned statsdrecv.Server TCP receivers — one per side — on two
//     separately-allocated host ports, both bound on 0.0.0.0:<port> BEFORE
//     either proxy starts. The statsd sink flushes PERIODICALLY, so the reference
//     keeps sending to its receiver during the subject's drive window; two
//     private receivers give each side an uncontaminated accumulator
//     (reference_periodic_sink_differential_two_receivers). The reference
//     bootstrap's c_statsd cluster endpoint is the LITERAL host-gateway IP (what
//     Docker resolves "host-gateway" to; e.g. 192.168.65.2 on Docker Desktop);
//     the subject's is 127.0.0.1.
//
//  2. DriveReference / DriveSubject each fire K=7 deterministic GET requests
//     (all 2xx) through the proxy listener, POLL that side's private receiver
//     until the COUNTER subset's delta-SUM converges to == K, await >= 2 further
//     flushes (the stability barrier), then snapshot the subset delta-SUMs, the
//     gauge, and the TCP transport observables (ConnCount / UnparsedCount /
//     c_statsd.upstream_cx_total).
//
//  3. closeServers hard-stops both receivers via Close() — for a TCP receiver
//     this hard-closes the listener and every accepted connection. NEVER a
//     graceful stop: the sink holds its connection open for the process lifetime
//     (reference_periodic_sink_differential_two_receivers).
//
// UNasserted (surfaces differ / impl-specific / non-deterministic): the WHOLE
// line set (AMEND-TCP-USEDONLY — the reference emits only USED stats, so the
// sets differ structurally; assert NAMED SUBSETS only, per
// reference_stats_sink_emits_used_only); |ms timer histograms (envoy-go lacks
// them); non-deterministic gauges (server.uptime, *_active, connection churn);
// flush cadence; per-flush write granularity (not observable to a line-parsing
// stream receiver).
package driver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
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
	fixtureName = "0098-stats-sink-statsd-tcp"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0098 takes 10098 for the single plaintext listener.
	refAdminPort    = 9901
	refListenerPort = 10098

	// numReq is the per-side request count K. After K 2xx requests the
	// deterministic COUNTER subset's per-flush deltas SUM to == K on each side.
	numReq = 7

	// The fixed request shape (kept identical cross-side; query-less path).
	probePath = "/probe"
	probeHost = "statsd.example"
	probeUA   = "statsd-probe/1"

	// The HCM stat_prefix + backend cluster name baked IDENTICALLY into both
	// bootstraps, so the mapped dotted stat names match cross-side.
	statPrefix  = "hcm_local"
	backendName = "c_backend"

	// statsdClusterName is the STATIC cluster the statsd TCP sink dials
	// (StatsdSink.statsd_specifier.tcp_cluster_name). Named identically on both
	// sides; its endpoint is the driver-owned statsdrecv TCP receiver.
	statsdClusterName = "c_statsd"

	// node.id + node.cluster, REQUIRED on BOTH sides for a tcp_cluster_name
	// statsd sink (AMEND-TCP-NODE). Values are arbitrary but must be non-empty.
	nodeID      = "sd-node"
	nodeCluster = "sd-cluster"

	// The statsd metric prefix baked identically on both sides. All emitted
	// metric names are <prefix>.<name> (e.g. sdpfx.cluster.c_backend.upstream_rq_total).
	prefix = "sdpfx"

	// Converge-poll discipline (reference_concurrency_differential_release_barrier):
	// POLL the subset DeltaSum() to == K; never sleep-to-wait.
	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)

// subsetNames is the deterministic COUNTER name subset asserted cross-side.
// All three are COUNTERs whose per-flush deltas SUM to exactly numReq after K
// 2xx requests on BOTH the reference and the subject. Names are PREFIX-JOINED:
// the statsdrecv receiver keys on the full <prefix>.<name> line, not the bare
// stat name.
var subsetNames = []string{
	prefix + ".cluster." + backendName + ".upstream_rq_total",
	prefix + ".http." + statPrefix + ".downstream_rq_total",
	prefix + ".http." + statPrefix + ".downstream_rq_2xx",
}

// gaugeName is the absolute |g metric asserted cross-side. cluster.<name>.
// membership_total is registered UNCONDITIONALLY for every cluster on BOTH sides
// (== the endpoint count) — value 1 here (one backend endpoint). membership_healthy
// is registered by envoy-go only on clusters WITH health_checks, so a
// no-health-check cluster never emits it; membership_total is the deterministic
// cross-side-equal absolute |g for this topology
// (reference_membership_total_vs_healthy_gauge).
var gaugeName = prefix + ".cluster." + backendName + ".membership_total"

// cxTotalName is the SUBJECT-EXACT transport counter proving DialSink took the
// UNACCOUNTED dial path (AMEND-TCP-CXSTATS): envoy-go registers upstream_cx_total
// unconditionally for every cluster INCLUDING c_statsd, so the line is always
// present, but DialSink increments NOTHING, so its value must be 0.
var cxTotalName = prefix + ".cluster." + statsdClusterName + ".upstream_cx_total"

func init() {
	fixture.RegisterFixture(fixtureName, &statsdTCPDriver{})
}

// sideSnapshot is the per-side captured state: the three subset delta-SUMs, the
// gauge value, and the TCP transport observables (ConnCount / UnparsedCount /
// c_statsd.upstream_cx_total).
type sideSnapshot struct {
	sums      map[string]float64
	gaugeVal  float64
	gaugeOK   bool
	connCount int
	unparsed  int
	cxTotal   float64
	cxTotalOK bool
}

// statsdTCPDriver carries the per-driver lifecycle state — TWO private statsdrecv
// TCP receivers (one per side; the reference's periodic flushes must not
// contaminate the subject snapshot) on two separately-allocated host ports, plus
// the per-side snapshots captured during Drive for the AssertStats assertions.
type statsdTCPDriver struct {
	mu sync.Mutex

	refStatsdPort  int
	subjStatsdPort int
	refSrv         *statsdrecv.Server
	subjSrv        *statsdrecv.Server

	ref  sideSnapshot
	subj sideSnapshot
}

// ensure allocates the two receiver ports (via Listen+Close on TCP) and starts
// both in-process statsdrecv TCP receivers bound to 0.0.0.0:<port> (so BOTH the
// reference container and the subject can connect to their respective receiver).
// Idempotent — safe to call from both ReferenceBootstrap and SubjectConfig
// regardless of order; a second call is a no-op while the servers run. Both
// servers are live before either proxy boots.
func (d *statsdTCPDriver) ensure() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.refStatsdPort == 0 {
		d.refStatsdPort = mustAllocateTCPPort()
	}
	if d.subjStatsdPort == 0 {
		d.subjStatsdPort = mustAllocateTCPPort()
	}
	if d.refSrv == nil {
		d.refSrv = mustStartReceiver(d.refStatsdPort)
	}
	if d.subjSrv == nil {
		d.subjSrv = mustStartReceiver(d.subjStatsdPort)
	}
}

// mustAllocateTCPPort reserves an ephemeral TCP port and releases it, so the
// receiver can bind it on 0.0.0.0 (reachable from the reference container via
// the host gateway). Racy in principle, matching 0092's UDP precedent.
func mustAllocateTCPPort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("%s: allocate tcp port: %v", fixtureName, err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// mustStartReceiver starts an in-process statsdrecv TCP receiver bound to
// 0.0.0.0:<port>.
func mustStartReceiver(port int) *statsdrecv.Server {
	srv, err := statsdrecv.NewTCPAtAddr(fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		panic(fmt.Sprintf("%s: start tcp statsd receiver on %d: %v", fixtureName, port, err))
	}
	return srv
}

// --- fixture.Driver (required) ---

func (*statsdTCPDriver) BackendCount() int                { return 1 }
func (*statsdTCPDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }
func (*statsdTCPDriver) SubjectListenerName() string      { return "l_test" }
func (*statsdTCPDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// runner-allocated backend port + the reference-side statsdrecv receiver
// address (LITERAL host-gateway IP:refStatsdPort, reached by the c_statsd
// cluster) + node.id/node.cluster (REQUIRED for a tcp_cluster_name sink). It
// allocates the ports and starts both receivers here so they are live before
// the reference container boots.
func (d *statsdTCPDriver) ReferenceBootstrap(backendPorts []int) string {
	d.ensure()
	gwIP, err := hostGatewayIP(context.Background())
	if err != nil {
		panic(fmt.Sprintf("%s: hostGatewayIP: %v", fixtureName, err))
	}
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":         refAdminPort,
		"ListenerPort":      refListenerPort,
		"BackendHost":       "host.docker.internal",
		"BackendPort":       backendPorts[0],
		"StatsdHost":        gwIP,
		"StatsdPort":        d.refStatsdPort,
		"StatsdClusterName": statsdClusterName,
		"Prefix":            prefix,
		"StatPrefix":        statPrefix,
		"BackendName":       backendName,
		"NodeID":            nodeID,
		"NodeCluster":       nodeCluster,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
// + backend port (loopback) + the subject-side statsdrecv receiver address
// (127.0.0.1:subjStatsdPort, reached by the c_statsd cluster) +
// node.id/node.cluster (REQUIRED for a tcp_cluster_name sink).
func (d *statsdTCPDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":         subjAdminPort,
		"ListenerPort":      subjListenerPort,
		"BackendPort":       backendPorts[0],
		"StatsdHost":        "127.0.0.1",
		"StatsdPort":        d.subjStatsdPort,
		"StatsdClusterName": statsdClusterName,
		"Prefix":            prefix,
		"StatPrefix":        statPrefix,
		"BackendName":       backendName,
		"NodeID":            nodeID,
		"NodeCluster":       nodeCluster,
	})
}

// DriveReference fires the workload against the reference proxy and snapshots
// the reference-side readings from the reference's own private receiver.
func (d *statsdTCPDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
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
// subject-side readings from the subject's own private receiver. After the
// subject snapshot BOTH receivers are hard-stopped via Close() (which for a TCP
// receiver hard-closes the listener and every accepted connection) for
// deterministic teardown — NEVER a graceful stop, because the sink holds its
// connection open for the process lifetime
// (reference_periodic_sink_differential_two_receivers).
func (d *statsdTCPDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
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
func (d *statsdTCPDriver) closeServers() {
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
// (the stability barrier), and snapshots the subset delta-SUMs + the gauge + the
// TCP transport observables. No Reset() is needed: each side owns a private
// receiver.
func (d *statsdTCPDriver) driveSide(ctx context.Context, addr string, srv *statsdrecv.Server) ([]byte, sideSnapshot, error) {
	if srv == nil {
		return nil, sideSnapshot{}, fmt.Errorf("%s: statsd receiver not running", fixtureName)
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
	// STABILITY BARRIER (reference_delta_sink_differential_stability_barrier):
	// a burst-all-requests delta sink cannot tell a DELTA from an ABSOLUTE — the
	// first flush's delta EQUALS the absolute value. Require >= 2 further flushes
	// and re-read: under correct per-flush deltas the idle counters emit 0 and the
	// SUM stays numReq; under absolute values it grows by numReq each flush.
	if err := awaitFurtherFlushes(ctx, srv, subsetNames[0], 2); err != nil {
		return nil, sideSnapshot{}, err
	}

	snap := sideSnapshot{sums: make(map[string]float64, len(subsetNames))}
	for _, name := range subsetNames {
		if v, ok := srv.DeltaSum(name); ok {
			snap.sums[name] = v
		}
	}
	snap.gaugeVal, snap.gaugeOK = srv.Gauge(gaugeName)
	snap.connCount = srv.ConnCount()
	snap.unparsed = srv.UnparsedCount()
	snap.cxTotal, snap.cxTotalOK = srv.DeltaSum(cxTotalName)
	return b.Bytes(), snap, nil
}

// fireProbe issues one query-less GET probePath with the fixed Host + User-Agent
// and returns the response status code (the body is drained and discarded).
func (d *statsdTCPDriver) fireProbe(ctx context.Context, client *http.Client, addr string) (int, error) {
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

// subsetConverged is true iff every subset name has delta-SUM of exactly numReq.
func subsetConverged(srv *statsdrecv.Server) bool {
	for _, name := range subsetNames {
		v, ok := srv.DeltaSum(name)
		if !ok || v != float64(numReq) {
			return false
		}
	}
	return true
}

// describeSubset renders the current subset delta-SUMs for a timeout diagnostic.
func describeSubset(srv *statsdrecv.Server) string {
	var b bytes.Buffer
	for _, name := range subsetNames {
		v, ok := srv.DeltaSum(name)
		fmt.Fprintf(&b, "%s=%v(ok=%v) ", name, v, ok)
	}
	return b.String()
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// the raw response bytes for the standard admin-diff at the runner's probe step.
func (*statsdTCPDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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
// present with delta-SUM == numReq; the absolute gauge
// cluster.<backend>.membership_total == 1 (assertPayloadParity). It then adds
// the THREE SUBJECT-EXACT transport assertions the TCP receiver makes possible
// (ConnCount / UnparsedCount / c_statsd.upstream_cx_total). The reference's
// transport values are RECORDED, never asserted — cross-side equality is
// infeasible by the histogram boundary.
func (d *statsdTCPDriver) AssertStats(t fixture.TB, _, _ string) {
	t.Helper()
	d.mu.Lock()
	ref, subj := d.ref, d.subj
	d.mu.Unlock()

	assertPayloadParity(t, "reference", ref)
	assertPayloadParity(t, "subject", subj)

	// ---- SUBJECT-EXACT (the reference's values are RECORDED, not asserted) ----
	//
	// fixture.TB exposes ONLY Errorf/Fatalf/Helper — it deliberately does not
	// import "testing", so there is NO t.Logf. Record the reference's values with
	// the stdlib logger.
	log.Printf("%s reference (RECORDED, not asserted): connCount=%d unparsed=%d "+
		"(2 conns: main + the |ms-only worker-timer sink; ~35 |ms lines are legitimately unparsed)",
		fixtureName, ref.connCount, ref.unparsed)

	// ONE long-lived connection. A per-flush redial yields one conn per flush.
	if subj.connCount != 1 {
		t.Errorf("subject ConnCount = %d, want exactly 1 (one long-lived connection; "+
			"a per-flush redial inflates this)", subj.connCount)
	}
	// envoy-go emits only |c and |g. A non-zero count means the receiver saw a line
	// it could not account for — the signature of \n-SEPARATED rather than
	// \n-TERMINATED framing concatenating two lines across a flush boundary.
	if subj.unparsed != 0 {
		t.Errorf("subject UnparsedCount = %d, want 0 (envoy-go emits only |c and |g; "+
			"a non-zero count means the flush framing is not '\\n'-TERMINATED)", subj.unparsed)
	}
	// DialSink took the UNACCOUNTED path. Subject-only: the reference never emits
	// this line at all (AMEND-TCP-USEDONLY — it omits never-incremented counters).
	// envoy-go registers upstream_cx_total unconditionally for c_statsd, so the
	// line MUST be present; absence is a real failure, not a tolerable one.
	if !subj.cxTotalOK {
		t.Errorf("subject: %s absent; envoy-go emits every registered stat, so it must be present", cxTotalName)
	} else if subj.cxTotal != 0 {
		t.Errorf("subject %s = %v, want 0 (DialSink must take NO upstream_cx_* accounting; "+
			"a value of 1 means Cluster.Dial was used instead)", cxTotalName, subj.cxTotal)
	}
}

// assertPayloadParity asserts the deterministic COUNTER subset (present +
// delta-SUM == numReq, STILL numReq after >= 2 further flushes) and the absolute
// gauge (== 1) for one side's snapshot. This is the cross-side parity both sides
// must satisfy.
func assertPayloadParity(t fixture.TB, side string, snap sideSnapshot) {
	t.Helper()
	if len(snap.sums) == 0 {
		t.Fatalf("%s: no statsd lines received at all", side)
	}
	for _, name := range subsetNames {
		v, ok := snap.sums[name]
		if !ok {
			t.Errorf("%s: counter %q absent from the emitted lines", side, name)
			continue
		}
		if v != float64(numReq) {
			t.Errorf("%s: %q |c delta-SUM = %v, want %d (still %d after >=2 further flushes)",
				side, name, v, numReq, numReq)
		}
	}
	if !snap.gaugeOK {
		t.Errorf("%s: gauge %q absent", side, gaugeName)
	} else if snap.gaugeVal != 1 {
		t.Errorf("%s: gauge %q = %v, want 1", side, gaugeName, snap.gaugeVal)
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
// mapping and read what host.docker.internal resolves to. Mirrors
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
	// thisFile is .../test/fixtures/0098-stats-sink-statsd-tcp/driver/driver.go
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
	_ fixture.Driver           = (*statsdTCPDriver)(nil)
	_ fixture.BackendKindAware = (*statsdTCPDriver)(nil)
	_ fixture.StatsAsserter    = (*statsdTCPDriver)(nil)
)
