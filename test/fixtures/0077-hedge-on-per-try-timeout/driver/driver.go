// Package driver registers the 0077-hedge-on-per-try-timeout cross-side
// differential fixture (phase 42.2b SPEC / PLAN Task 9).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture that proves the
// HTTP retry-loop HEDGING behavior (hedge_on_per_try_timeout) behaves IDENTICALLY
// on the envoy-go (subject) side and the reference-Envoy side. It has ONE cluster +
// ONE route over a SINGLE held backend:
//
//   - c_hedge: lb_policy ROUND_ROBIN (single host), the BlockingHoldResponder
//     (BackendKind 36) — accepts each connection but HOLDS each "GET /<seg>"
//     request open (never responds) until a "GET /__release" control request.
//
//   - /hedge → c_hedge, retry_policy{retry_on:"5xx", num_retries:3,
//     per_try_timeout:0.25s}, hedge_policy{hedge_on_per_try_timeout:true},
//     timeout:0s (AMEND-H7: disable the reference's default route timeout — envoy-go
//     has none — so the held request is not aborted; REQUIRED on BOTH sides).
//
// # The behavior this fixture proves (hedging, not abandon-and-504)
//
// With hedge_on_per_try_timeout over a backend that never answers, each per-try
// deadline T (250ms) LAUNCHES a hedge (a RETRY) and LEAVES the original attempt
// running — it does NOT synthesize a 504, does NOT abandon. So 1 primary + 3 hedges
// (num_retries) all end up in flight (held); after 3 hedges the retry cap is hit
// (upstream_rq_retry_limit_exceeded == 1) and the request BLOCKS awaiting the first
// acceptable result. A GET /__release then makes the held attempts answer 200 → the
// first acceptable 200 returns downstream.
//
// THE LOAD-BEARING PROOF (AMEND-H1): cluster.c_hedge.upstream_rq_per_try_timeout
// stays 0 — a hedged per-try-timeout is a RETRY, not a per_try_timeout. The hedge
// executor uses a hedge-trigger timer, NOT the 42.2a abandon-and-count
// discriminator. Task 10's deliberate-break B makes this non-zero.
//
// # Topology: 1 BlockingHoldResponder (runner-spawned)
//
//   - backend0 → c_hedge endpoint 0 (BlockingHoldResponder; holds GET /hedge until
//     /__release). BackendCount() is 1; the uniform BackendKind() is
//     BlockingHoldResponder (NO PerHostBackendKind, NO new BackendKind authored).
//
//   - Subject (envoy-go): type STATIC, endpoint = 127.0.0.1:<backendPort>
//     (envoy-go's buildCluster ONLY supports STATIC).
//
//   - Reference (Envoy): type STRICT_DNS, endpoint = host.docker.internal:<
//     backendPort> (the 0074/0075/0076 reference shape; the reference MUST be
//     STRICT_DNS over the bridge).
//
// # The driver: goroutine-fire + poll-to-converge + release (sequential per side)
//
// The runner's hooks are DriveReference/DriveSubject (the byte-equiv stream, run
// FIRST) then AssertStats (run LAST, holding BOTH admin addrs). All the measured
// work runs inside AssertStats. The Drive hooks STASH their listener addrs and
// return a fixed "READY\n" for the runner's CompareBytes gate. The config builders
// STASH the backend port so AssertStats can hit the backend's /__release control
// port (127.0.0.1:<backendPort>, loopback — the same machine on both sides).
//
// AssertStats runs SEQUENTIALLY per side (subject FULLY, then reference). UNLIKE
// 0076 (a single blocking GET that exhausts internally), the hedged GET /hedge
// BLOCKS until /__release (all attempts are held), so it is fired in a GOROUTINE (a
// sync.WaitGroup) and the driver POLLS the admin /stats to confirm the steady state
// (3 hedges launched, cap hit) before releasing — the 0074 concurrent-fire +
// poll-to-converge model. There is NO fixed time.Sleep in the assertion path
// (reference_concurrency_differential_release_barrier).
//
// # The H1-loser-cancel asymmetry (ADR-0251 departure, D-S422B-2)
//
// We do NOT assert the UPSTREAM 200-class counter (cluster.c_hedge.upstream_rq_200)
// cross-side. On the SUBJECT (envoy-go H1) side, doH1ClusterAction honors only
// ctx.Deadline(), NOT ctx.Done(), so after /__release ALL 4 held H1 losers complete
// with 200 and each bumps the upstream 200-class counter → the subject over-counts
// (up to 4) AND races the join. The REFERENCE cancels its in-flight losers, so its
// upstream_rq_200 == 1. They are NOT cross-side equal. The DOWNSTREAM
// http.ingress_http.downstream_rq_2xx == 1 IS equal (one client response on both
// sides) — that is the "request recovered" proof we assert.
//
// # Cross-references
//
//   - phase 42.2b SPEC / PLAN Task 9 (the fixture design).
//   - 0076-per-try-timeout (the cross-side CONFIG shape: STATIC subject / STRICT_DNS
//     reference, the BlockingHoldResponder held backend, stashBackendPort +
//     /__release, scrapeStats/assertDelta, the decode-ran guard).
//   - 0074-circuit-breaker-max-requests (the concurrent goroutine-fire +
//     poll-to-converge + /__release release-barrier model).
//   - reference_concurrency_differential_release_barrier (poll-the-gauge, never a
//     sleep).
//   - reference_per_try_timeout_504_reset_classification (AMEND-H1: a hedged
//     per-try-timeout is a retry, NOT a per_try_timeout — counter stays 0).
//   - reference_docker_probe_bridge_network (shared bridge + STRICT_DNS hostname;
//     the upstream_rq_total > 0 "decode ran" guard).
//   - reference_differential_run_selector (target -run
//     'TestDifferential/0077-hedge-on-per-try-timeout').
//   - reference_fixture_workload_constant_desync (constants single-sourced).
//   - reference_differential_asserter_dispatch (cross-side via the StatsAsserter
//     path).
package driver

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0077-hedge-on-per-try-timeout"

	// In-container reference Envoy listener port for l_http. Fixtures run
	// sequentially; a distinct value avoids confusion (0076 took 19165, this takes
	// the next-free 19166).
	refContainerListenerPort = 19166

	refAdminPort = 9901

	// statPrefix is the HCM listener stat_prefix (single-sourced: the bootstraps
	// read this; mirrors the 0074/0075/0076 convention).
	statPrefix = "ingress_http"

	// clusterHedge is the single cluster over the held backend; the stat keys
	// interpolate it.
	clusterHedge = "c_hedge"

	// backendCount is the number of runner-spawned BlockingHoldResponder hosts.
	backendCount = 1

	// numRetries is the /hedge route's num_retries (== numHedges: each per-try
	// deadline launches one hedge while the original keeps running, so the loop
	// launches numRetries hedges before the cap is hit).
	numRetries = 3
	numHedges  = numRetries

	// perTryTimeout is the /hedge route's per_try_timeout (250ms): small enough that
	// each held attempt fires the hedge trigger, large enough not to flake on
	// per-attempt dial+roundtrip jitter. Below connect_timeout (1s).
	perTryTimeout = "0.25s"

	// HTTP client timeout on the GET /hedge: the request BLOCKS until /__release
	// (all attempts held), so a generous bound covering the poll-to-converge phase.
	driveTimeout = 30 * time.Second

	// Convergence poll budget (NO fixed sleep — poll until the predicate holds).
	convergeDeadline = 10 * time.Second
	convergePoll     = 50 * time.Millisecond
)

// Stat keys (single-sourced — all interpolate clusterHedge; TestStatKeys pins the
// resulting strings against the YAML's cluster name).
var (
	statPerTryTimeout = "cluster." + clusterHedge + ".upstream_rq_per_try_timeout"
	statRetry         = "cluster." + clusterHedge + ".upstream_rq_retry"
	statRetryLimitExc = "cluster." + clusterHedge + ".upstream_rq_retry_limit_exceeded"
	statRqTotal       = "cluster." + clusterHedge + ".upstream_rq_total"

	// The DOWNSTREAM 2xx-class counter — the cross-side-equal "request recovered"
	// proof (one client response on both sides). NOT the upstream 200-class (the
	// H1-loser-cancel asymmetry — see the package doc + README).
	statDownstream2xx = "http." + statPrefix + ".downstream_rq_2xx"
)

func init() {
	fixture.RegisterFixture(fixtureName, &hedgeDriver{})
}

// hedgeDriver is STATEFUL: the Drive hooks stash the per-side listener addrs and the
// config builders stash the backend port, so AssertStats — the only hook holding
// BOTH admin addrs — can drive /hedge, poll the steady state, delta-assert, and
// release the parked held attempts.
type hedgeDriver struct {
	mu           sync.Mutex
	refListener  string // host:port of the reference l_http listener (from DriveReference)
	subjListener string // 127.0.0.1:<port> of the subject l_http listener (from SubjectConfig)
	backendPort  int    // the single backend's host port (for the /__release control hit)
}

func (*hedgeDriver) BackendCount() int                { return backendCount }
func (*hedgeDriver) BackendKind() fixture.BackendKind { return fixture.BlockingHoldResponder }
func (*hedgeDriver) SubjectListenerName() string      { return "l_http" }
func (*hedgeDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// stashBackendPort memoizes the single backend's host port. Both ReferenceBootstrap
// and SubjectConfig receive the same backendPorts slice and call this; they must
// agree on the SAME port (the shared in-process backend).
func (d *hedgeDriver) stashBackendPort(backendPorts []int) {
	d.mu.Lock()
	d.backendPort = backendPorts[0]
	d.mu.Unlock()
}

// routeTable routes /hedge → c_hedge with the per_try_timeout retry_policy + the
// hedge_policy + the route timeout:0s (AMEND-H7). Identical on both sides (static
// config). retry_on:"5xx" + hedge_on_per_try_timeout: each per-try deadline launches
// a hedge (a retry) while the original keeps running; the loop launches num_retries
// hedges before the cap is hit. timeout:0s disables the reference's default route
// timeout so the held request is not aborted (envoy-go has no global route timeout).
var routeTable = fmt.Sprintf(`                      routes:
                        - match: { prefix: "/hedge" }
                          route:
                            cluster: %s
                            timeout: 0s
                            retry_policy:
                              retry_on: "5xx"
                              num_retries: %d
                              per_try_timeout: %s
                            hedge_policy:
                              hedge_on_per_try_timeout: true`,
	clusterHedge, numRetries, perTryTimeout)

func (d *hedgeDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.stashBackendPort(backendPorts)
	// STRICT_DNS + host.docker.internal (the 0074/0075/0076 reference shape).
	// c_hedge over the single BlockingHoldResponder, with the hedging retry route.
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 0.0.0.0, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: %s
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
%s
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: %s
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: %s
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, refAdminPort, refContainerListenerPort, statPrefix, routeTable, clusterHedge, clusterHedge, backendPorts[0])
}

func (d *hedgeDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.stashBackendPort(backendPorts)
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	// STATIC + 127.0.0.1 (the 0074/0075/0076 subject shape). Same cluster/route
	// topology.
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0077, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: %s
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
%s
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: %s
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: %s
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, statPrefix, routeTable, clusterHedge, clusterHedge, backendPorts[0])
}

// DriveReference stashes the reference listener addr and returns the fixed
// byte-equiv stream. The real work runs in AssertStats.
func (d *hedgeDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

// DriveSubject returns the fixed byte-equiv stream (the subject listener addr was
// already stashed in SubjectConfig). The real work runs in AssertStats.
func (d *hedgeDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the
// 0074/0075/0076 raw /ready probe, verbatim).
func (*hedgeDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref probe: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj probe: %w", err)
	}
	return refBytes, subjBytes, nil
}

// pollDelta scrapes adminAddr/stats every convergePoll until (st[key]-base[key]) ==
// want or the deadline trips, returning a clear error (with the last delta seen) on
// timeout. base is the pre-fire baseline; want is the target DELTA (the absolute
// target is base[key]+want).
func pollDelta(side, adminAddr, key string, base map[string]uint64, want uint64) error {
	deadline := time.Now().Add(convergeDeadline)
	var lastDelta int64 = -1
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			delta := int64(st[key]) - int64(base[key])
			lastDelta = delta
			if delta == int64(want) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: %s delta did not converge to %d within %s (last delta %d)",
				side, key, want, convergeDeadline, lastDelta)
		}
		time.Sleep(convergePoll)
	}
}

// assertSide drives ONE hedged GET /hedge for a side (in a goroutine, since it
// BLOCKS until /__release), polls the steady state (numHedges launched + cap hit),
// releases the parked held attempts, joins on the downstream 200, then delta-asserts
// the hedging stats. adminAddr is the proxy admin; listenerAddr the l_http listener;
// backendPort the shared backend's host port (for the /__release control hit, always
// over 127.0.0.1 loopback).
func (d *hedgeDriver) assertSide(t fixture.TB, side, listenerAddr, adminAddr string, backendPort int) {
	t.Helper()
	ctx := context.Background()

	base, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape baseline /stats: %v", side, err)
	}

	// 1. Fire ONE GET /hedge in a GOROUTINE — it BLOCKS (all held attempts never
	// complete pre-release). Capture its final status + transport error.
	var (
		downStatus int
		downErr    error
	)
	client := &http.Client{Timeout: driveTimeout}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+listenerAddr+"/hedge", nil)
		if rerr != nil {
			downErr = rerr
			return
		}
		resp, rerr := client.Do(req)
		if rerr != nil {
			downErr = rerr
			return
		}
		downStatus = resp.StatusCode
		_ = resp.Body.Close()
	}()

	// 2. Poll the steady state (NO fixed sleep): the original + numHedges launched
	// (upstream_rq_retry delta == numHedges) AND the retry cap hit
	// (upstream_rq_retry_limit_exceeded delta == 1). At that point all 1+numHedges
	// attempts are in flight (held) and the request BLOCKS awaiting the first
	// acceptable result.
	if err := pollDelta(side, adminAddr, statRetry, base, numHedges); err != nil {
		t.Fatalf("%s: hedge launch: %v (the held attempts did not launch %d hedges — is hedge_on_per_try_timeout enforcing? is the backend holding?)", side, err, numHedges)
	}
	if err := pollDelta(side, adminAddr, statRetryLimitExc, base, 1); err != nil {
		t.Fatalf("%s: retry cap: %v (the retry limit was not hit after %d hedges)", side, err, numHedges)
	}

	// 3. Release: hit the BACKEND control port (NOT the proxy listener), always over
	// 127.0.0.1 loopback (the backend is in-process on this machine for both sides).
	// The held attempts answer 200 → the first acceptable 200 returns downstream.
	releaseAddr := "127.0.0.1:" + strconv.Itoa(backendPort)
	relResp, _, err := helpers.HTTPRoundTrip(ctx, releaseAddr, "GET", "/__release", nil, nil)
	if err != nil {
		t.Fatalf("%s: /__release: transport error to backend %s: %v", side, releaseAddr, err)
	}
	if relResp.StatusCode != http.StatusOK {
		t.Fatalf("%s: /__release: status %d, want 200 (the backend control port did not release)", side, relResp.StatusCode)
	}

	// 4. Join: the blocked GET /hedge now returns. Assert the captured downstream
	// status == 200 (the "request recovered" proof).
	wg.Wait()
	if downErr != nil {
		t.Fatalf("%s: GET /hedge: transport error: %v (the hedged request should return a 200, not a transport failure)", side, downErr)
	}
	if downStatus != http.StatusOK {
		t.Errorf("%s: GET /hedge status %d, want 200 (the first acceptable hedge result should return downstream after release)", side, downStatus)
	}

	// 5. Final scrape + delta-assert the hedging stats.
	fin, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape final /stats: %v", side, err)
	}

	// "decode ran" guard (reference_docker_probe_bridge_network): the ref container
	// must have reached the backend over the bridge (no hedges can launch on an
	// attempt that never connected).
	if side == "reference" && fin[statRqTotal]-base[statRqTotal] == 0 {
		t.Fatalf("reference did NOT decode: %s delta == 0 (container could not reach the held backend — bridge network / host.docker.internal?)", statRqTotal)
	}

	// Cross-side EXACT deltas (assert on BOTH sides):
	//   THE LOAD-BEARING assertion (AMEND-H1): a hedged per-try-timeout is a retry,
	//   not a per_try_timeout, so this stays 0. Task 10's deliberate-break B makes it
	//   non-zero.
	assertDelta(t, side, fin, base, statPerTryTimeout, 0)     // 0 — hedged, NOT a per_try_timeout
	assertDelta(t, side, fin, base, statRetry, numHedges)     // 3 hedges launched
	assertDelta(t, side, fin, base, statRetryLimitExc, 1)     // cap hit once
	assertDelta(t, side, fin, base, statRqTotal, numHedges+1) // 4 = 1 primary + 3 hedges (counted at attempt entry)
	assertDelta(t, side, fin, base, statDownstream2xx, 1)     // the SINGLE downstream 200 (cross-side equal)

	// NOTE: we deliberately do NOT assert cluster.c_hedge.upstream_rq_200 /
	// upstream_2xx cross-side — the H1-loser-cancel asymmetry (ADR-0251 departure):
	// the subject's H1 losers all complete 200 (doH1ClusterAction honors only
	// ctx.Deadline(), not ctx.Done()) so the subject over-counts the upstream
	// 200-class, while the reference cancels its losers (== 1). The DOWNSTREAM
	// downstream_rq_2xx == 1 IS equal and is the proof asserted above.
}

// AssertStats drives ONE hedged /hedge per side SEQUENTIALLY (subject FULLY, then
// reference) and delta-asserts the hedging stats (the only hook holding both admin
// addrs). The shared in-process backend is drained (via /__release) between sides,
// so there is no cross-side interference.
func (d *hedgeDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	d.mu.Lock()
	refListener := d.refListener
	subjListener := d.subjListener
	backendPort := d.backendPort
	d.mu.Unlock()
	if refListener == "" || subjListener == "" {
		t.Fatalf("listener addrs not stashed (ref=%q subj=%q) — Drive hooks did not run?", refListener, subjListener)
	}
	if backendPort == 0 {
		t.Fatalf("backend port not stashed — config builders did not run?")
	}

	// Subject FULLY first, then reference (sequential — the shared backend is
	// drained between sides).
	d.assertSide(t, "subject", subjListener, subjAdminAddr, backendPort)
	d.assertSide(t, "reference", refListener, refAdminAddr, backendPort)
}

// assertDelta asserts (final[key] - base[key]) == want — the change in a counter
// over the measured phase. Absent keys read as 0 (reference Envoy lazily allocates
// per-response-class counters), so a 0-want passes when the class was never touched.
func assertDelta(t fixture.TB, side string, st, base map[string]uint64, key string, want uint64) {
	t.Helper()
	got := st[key] - base[key]
	if got != want {
		t.Errorf("%s: %s delta = %d, want %d (final %d, base %d)", side, key, got, want, st[key], base[key])
	}
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0074/0075/0076 driver
// scrapeStats, verbatim.)
func scrapeStats(adminAddr string) (map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	out := make(map[string]uint64)
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 32*1024)
	for {
		nn, rerr := resp.Body.Read(tmp)
		if nn > 0 {
			buf = append(buf, tmp[:nn]...)
		}
		if rerr != nil {
			break
		}
	}
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, ": ")
		if idx < 0 {
			continue
		}
		name := line[:idx]
		valStr := strings.TrimSpace(line[idx+2:])
		v, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue // skip non-numeric (histograms, special formats)
		}
		out[name] = v
	}
	return out, nil
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*hedgeDriver)(nil)
	_ fixture.StatsAsserter    = (*hedgeDriver)(nil)
	_ fixture.BackendKindAware = (*hedgeDriver)(nil)
)
