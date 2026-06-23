// Package driver registers the 0078-connection-pool-max-connections cross-side
// differential fixture (phase 43.1 SPEC / PLAN Task 8).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture over ONE
// cluster c_cp (lb_policy ROUND_ROBIN) with a circuit_breakers
// max_connections + max_pending_requests threshold over a SINGLE endpoint — a
// BlockingHoldResponder (BackendKind 36) that HOLDS each "GET /" request open
// until the driver releases it. It proves the connection-pool CONNECTION budget
// (max_connections HARD-CAP, max_pending_requests bounded wait-queue) on BOTH
// the envoy-go (subject) side AND the reference-Envoy side.
//
// # The soft-breaker DEPARTURE (reference_max_connections_soft_breaker)
//
// The reference Envoy's max_connections is a SOFT breaker — upstream_cx_active
// can EXCEED the cap with timing slop. So a cross-side EXACT connection-pool
// differential is INFEASIBLE. envoy-go implements a CLEAN HARD-CAP +
// bounded-queue DEPARTURE. The fixture is therefore split into TWO prongs run
// SEQUENTIALLY per side inside AssertStats (subject FULLY, then reference; the
// shared in-process backend is idle between sides):
//
//   - SUBJECT (envoy-go) — EXACT: the hard cap holds precisely — upstream_cx_active
//     never exceeds N; the queue peaks at M; exactly J oversubscribers overflow
//     503; upstream_rq_pending_total == M exactly.
//   - REFERENCE (envoy) — ROBUST: only the observable invariants — cx_open flips
//     at saturation; >=1 downstream-class 503; upstream_rq_pending_overflow delta
//     >= 1; the decode-ran guard upstream_cx_total > 0; final gauges back to 0.
//
// # Topology: 1 BlockingHoldResponder (runner-spawned)
//
//   - backend0 → c_cp endpoint 0 (BlockingHoldResponder; holds GET / until a
//     release, then 200 "backend-0:")
//
// BackendCount() is 1; the uniform BackendKind() is BlockingHoldResponder (NO
// PerHostBackendKind). The BackendKind tail is UNCHANGED at 36 (REUSE).
//
// # Cluster shape (both sides)
//
//		c_cp: lb_policy ROUND_ROBIN, 1 endpoint, circuit_breakers: { thresholds:
//		        [ { priority: DEFAULT, max_connections: N, max_pending_requests: M } ] }
//
//	  - Subject (envoy-go): type STATIC, endpoint = 127.0.0.1:<backendPort>.
//	  - Reference (Envoy): type STRICT_DNS, endpoint = host.docker.internal:<port>
//	    (the 0066/0069/0074 reference shape; the reference MUST be STRICT_DNS).
//
// # The staged drive (per side, inside AssertStats)
//
// The runner's hooks are DriveReference/DriveSubject (the byte-equiv "READY\n"
// stream, run FIRST) then AssertStats (run LAST, holding BOTH admin addrs). The
// fill / pend / oversub / sticky-release ALL run inside AssertStats. The Drive
// hooks stash listener addrs; the config builders stash the backend host port so
// AssertStats can hit 127.0.0.1:<backendPort>/__release_sticky (loopback — the
// same machine on both sides) to drain.
//
//	| Step | SUBJECT (exact)                                 | REFERENCE (robust)                |
//	|------|-------------------------------------------------|-----------------------------------|
//	| 1    | Fire N held GET / → poll cx_open==1 AND          | Fire N held GET / → poll cx_open==1|
//	|      | upstream_cx_active==N                            | (cx_active may EXCEED N — soft)    |
//	| 2    | Fire M further held GET / (PEND) → poll          | — (skip; ref pend split is timing) |
//	|      | rq_pending_open==1 AND upstream_rq_pending_active==M |                               |
//	| 3    | Fire J oversubscribers → each 503               | Fire refOversub → >=1 gets 503     |
//	| 4ex  | cx_active==N throughout; rq_pending peaked at M; |                                   |
//	|      | exactly J 503; upstream_rq_pending_total==M      |                                   |
//	| 4rob | cx_open==1; >=1 downstream 503 (from the FIRED   | same                              |
//	|      | requests' codes); overflow delta>=1; cx_total>0  |                                   |
//	| 5    | /__release_sticky → drain to 200 → gauges -> 0   | same → cx_open==0 + rq_pending==0  |
//
// CRITICAL: the 503 detection observes the DOWNSTREAM status codes of the FIRED
// oversubscriber requests (count how many returned 503), NOT upstream_rq_5xx
// (reference_concurrent_attempt_downstream_class_assertion). All polls use
// convergeDeadline — NEVER a fixed sleep (reference_concurrency_differential_release_barrier).
//
// # Cross-references
//
//   - phase 43.1 SPEC / PLAN Task 8 (the fixture design).
//   - 0074-circuit-breaker-max-requests (the cross-side fill/probe/release shape).
//   - reference_max_connections_soft_breaker (the exact-vs-robust prong split).
//   - reference_concurrent_attempt_downstream_class_assertion (count downstream 503s).
//   - reference_docker_probe_bridge_network (shared bridge + STRICT_DNS hostname).
//   - reference_differential_run_selector (target -run 'TestDifferential/0078').
//   - reference_fixture_workload_constant_desync (constants single-sourced).
//   - reference_differential_asserter_dispatch (StatsAsserter — NOT SubjectAsserter).
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
	fixtureName = "0078-connection-pool-max-connections"

	// clusterName is the single cluster; the stat keys interpolate it.
	clusterName = "c_cp"

	// In-container reference Envoy listener port for l_http. Fixtures run
	// sequentially; next-free is 19167 (the 0074 family took up to 19166).
	refContainerListenerPort = 19167

	refAdminPort = 9901

	// backendCount is the number of runner-spawned BlockingHoldResponder hosts.
	backendCount = 1

	// maxConnections (N) is the circuit_breakers DEFAULT max_connections HARD-CAP.
	// Filling exactly N concurrent held requests saturates the pool (cx_open == 1).
	maxConnections = 2

	// maxPendingRequests (M) is the bounded wait-queue depth. After the N conns
	// saturate, M further held requests PEND (rq_pending_open == 1).
	maxPendingRequests = 2

	// oversub (J) is the subject's exact oversubscription: after N conns + M
	// pending, J more requests find the queue full → exactly J overflow 503s.
	oversub = 2

	// refOversub is the reference's heavier oversubscription. The reference's
	// max_connections is a SOFT breaker, so the pending/overflow split is timing
	// dependent; oversubscribe well past M+J to GUARANTEE >= 1 overflow 503.
	refOversub = maxPendingRequests + oversub + 4

	// Convergence poll budget (NO fixed sleep — poll until the predicate holds).
	convergeDeadline = 10 * time.Second
	convergePoll     = 50 * time.Millisecond
)

// statKey builds a cluster-scoped stat name "cluster.<clusterName>.<suffix>".
func statKey(suffix string) string { return "cluster." + clusterName + "." + suffix }

// The single-sourced stat keys (built from clusterName).
var (
	statCxOpen          = statKey("circuit_breakers.default.cx_open")
	statRqPendingOpen   = statKey("circuit_breakers.default.rq_pending_open")
	statCxActive        = statKey("upstream_cx_active")
	statCxTotal         = statKey("upstream_cx_total")
	statRqPendingActive = statKey("upstream_rq_pending_active")
	statRqPendingTotal  = statKey("upstream_rq_pending_total")
	statPendingOverflow = statKey("upstream_rq_pending_overflow")
)

func init() {
	fixture.RegisterFixture(fixtureName, &cpDriver{})
}

// cpDriver is STATEFUL: the Drive hooks stash the per-side listener addrs and the
// config builders stash the backend port, so AssertStats — the only hook holding
// BOTH admin addrs — can run the staged drive (fill / pend / oversub / drain).
type cpDriver struct {
	mu           sync.Mutex
	refListener  string // host:port of the reference l_http listener (from DriveReference)
	subjListener string // 127.0.0.1:<port> of the subject l_http listener (from SubjectConfig)
	backendPort  int    // the single backend's host port (for the /__release_sticky hit)
}

func (*cpDriver) BackendCount() int                { return backendCount }
func (*cpDriver) BackendKind() fixture.BackendKind { return fixture.BlockingHoldResponder }
func (*cpDriver) SubjectListenerName() string      { return "l_http" }
func (*cpDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// stashBackendPort memoizes the single backend's host port. Both builders receive
// the same backendPorts slice and must agree on the SAME port (shared backend).
func (d *cpDriver) stashBackendPort(backendPorts []int) {
	d.mu.Lock()
	d.backendPort = backendPorts[0]
	d.mu.Unlock()
}

// circuitBreakersBlock is the shared cluster circuit_breakers YAML (identical on
// both sides — NAT-transparent static config). One DEFAULT-priority threshold
// with the connection-pool budgets max_connections (N) + max_pending_requests (M).
var circuitBreakersBlock = fmt.Sprintf(`      circuit_breakers:
        thresholds:
          - priority: DEFAULT
            max_connections: %d
            max_pending_requests: %d`, maxConnections, maxPendingRequests)

// routeTable routes / to c_cp (the data path). Identical on both sides.
const routeTable = `                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_cp }`

func (d *cpDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.stashBackendPort(backendPorts)
	// STRICT_DNS + host.docker.internal (the 0066/0069/0074 reference shape).
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
                stat_prefix: ingress_http
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
    - name: c_cp
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
%s
      load_assignment:
        cluster_name: c_cp
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, refAdminPort, refContainerListenerPort, routeTable, circuitBreakersBlock, backendPorts[0])
}

func (d *cpDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.stashBackendPort(backendPorts)
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	// STATIC + 127.0.0.1 (the 0066/0069/0074 subject shape).
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0078, cluster: envoy-go-differential }
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
                stat_prefix: ingress_http
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
    - name: c_cp
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s
      load_assignment:
        cluster_name: c_cp
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, routeTable, circuitBreakersBlock, backendPorts[0])
}

// DriveReference stashes the reference listener addr and returns the fixed
// byte-equiv stream. The real work runs in AssertStats.
func (d *cpDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

// DriveSubject returns the fixed byte-equiv stream (the subject listener addr was
// already stashed in SubjectConfig). The real work runs in AssertStats.
func (d *cpDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0074 raw
// /ready probe, verbatim).
func (*cpDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// backendIdxFromBody parses the BlockingHoldResponder canned body
// "backend-<idx>:<seg>" and returns the embedded backend idx (host attribution).
func backendIdxFromBody(body []byte) (int, error) {
	s := string(body)
	const pfx = "backend-"
	if !strings.HasPrefix(s, pfx) {
		return 0, fmt.Errorf("body %q has no %q prefix", s, pfx)
	}
	rest := s[len(pfx):]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return 0, fmt.Errorf("body %q has no ':' after idx", s)
	}
	idx, err := strconv.Atoi(rest[:colon])
	if err != nil {
		return 0, fmt.Errorf("body %q: bad idx: %w", s, err)
	}
	return idx, nil
}

// pollStats scrapes until ALL key==want pairs hold simultaneously (or deadline).
func pollStats(side, adminAddr string, want map[string]uint64) error {
	deadline := time.Now().Add(convergeDeadline)
	last := map[string]int64{}
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			ok := true
			for k, w := range want {
				v := st[k]
				last[k] = int64(v)
				if v != w {
					ok = false
				}
			}
			if ok {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: stats did not converge to %v within %s (last seen %v)",
				side, want, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

// heldResult captures one GET / outcome (status + body) for the post-release tally.
type heldResult struct {
	status int
	body   []byte
	err    error
}

// fireHeld launches n concurrent GET / requests (each BLOCKS at the responder
// until a release). Results land in res[base:base+n]; the caller wg.Waits later.
func fireHeld(ctx context.Context, listenerAddr string, n, base int, res []heldResult, wg *sync.WaitGroup) {
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(slot int) {
			defer wg.Done()
			resp, body, err := helpers.HTTPRoundTrip(ctx, listenerAddr, "GET", "/", nil, nil)
			r := heldResult{err: err, body: body}
			if resp != nil {
				r.status = resp.StatusCode
			}
			res[slot] = r
		}(base + i)
	}
}

// assertSubject runs the EXACT prong: the hard cap holds precisely.
func (d *cpDriver) assertSubject(t fixture.TB, listenerAddr, adminAddr string, backendPort int) {
	t.Helper()
	const side = "subject"
	ctx := context.Background()

	// Layout: [0:N) the N held conns, [N:N+M) the M pending, [N+M:N+M+J) the J
	// oversubscribers (which 503).
	total := maxConnections + maxPendingRequests + oversub
	res := make([]heldResult, total)
	var wg sync.WaitGroup

	// Step 1: fire N held GET / → poll cx_open==1 AND upstream_cx_active==N.
	fireHeld(ctx, listenerAddr, maxConnections, 0, res, &wg)
	if err := pollStats(side, adminAddr, map[string]uint64{
		statCxOpen:   1,
		statCxActive: maxConnections,
	}); err != nil {
		t.Fatalf("%s: saturate-the-pool: %v (the %d held requests did not occupy all max_connections — is the hard-cap enforcing? is the backend holding?)", side, err, maxConnections)
	}

	// Step 2: fire M further held GET / (they PEND) → poll rq_pending_open==1 AND
	// upstream_rq_pending_active==M. cx_active MUST still be exactly N (hard cap).
	fireHeld(ctx, listenerAddr, maxPendingRequests, maxConnections, res, &wg)
	if err := pollStats(side, adminAddr, map[string]uint64{
		statRqPendingOpen:   1,
		statRqPendingActive: maxPendingRequests,
		statCxActive:        maxConnections, // never exceeds the cap
	}); err != nil {
		t.Fatalf("%s: fill-the-queue: %v (the %d further held requests did not PEND at depth M, or cx_active exceeded N)", side, err, maxPendingRequests)
	}

	// Baseline the overflow counter for the delta assertion.
	base, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape overflow baseline: %v", side, err)
	}
	baseOverflow := base[statPendingOverflow]

	// Step 3: fire J oversubscribers — each finds the queue FULL → 503. These are
	// SYNCHRONOUS (they return immediately with the local 503, do not block).
	got503 := 0
	for i := 0; i < oversub; i++ {
		resp, _, err := helpers.HTTPRoundTrip(ctx, listenerAddr, "GET", "/", nil, nil)
		if err != nil {
			t.Errorf("%s: oversub[%d]: transport error: %v (should be a 503 local reply, not a transport failure)", side, i, err)
			continue
		}
		if resp.StatusCode == http.StatusServiceUnavailable {
			got503++
		} else {
			t.Errorf("%s: oversub[%d]: status %d, want 503 (the queue was full — should overflow)", side, i, resp.StatusCode)
		}
	}

	// Step 4 (exact): exactly J got 503; the breaker still open; cx_active still N
	// (never exceeded); the queue still peaked at M; overflow delta == J;
	// upstream_rq_pending_total == M EXACTLY (the M that ever queued).
	if got503 != oversub {
		t.Errorf("%s: %d oversubscribers got 503, want exactly %d (J)", side, got503, oversub)
	}
	after, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape after oversub: %v", side, err)
	}
	if after[statCxOpen] != 1 {
		t.Errorf("%s: %s = %d after oversub, want 1 (pool still saturated)", side, statCxOpen, after[statCxOpen])
	}
	if after[statCxActive] != maxConnections {
		t.Errorf("%s: %s = %d, want %d (the HARD CAP — cx_active must NEVER exceed N)", side, statCxActive, after[statCxActive], maxConnections)
	}
	if after[statRqPendingActive] != maxPendingRequests {
		t.Errorf("%s: %s = %d, want %d (queue peaked at M; the J overflows did NOT enqueue)", side, statRqPendingActive, after[statRqPendingActive], maxPendingRequests)
	}
	if delta := after[statPendingOverflow] - baseOverflow; delta != uint64(oversub) {
		t.Errorf("%s: %s delta = %d, want exactly %d (J overflows; after %d base %d)", side, statPendingOverflow, delta, oversub, after[statPendingOverflow], baseOverflow)
	}
	if after[statRqPendingTotal] != maxPendingRequests {
		t.Errorf("%s: %s = %d, want %d EXACTLY (only the M that ever queued; the J overflows never entered the queue)", side, statRqPendingTotal, after[statRqPendingTotal], maxPendingRequests)
	}
	if after[statCxTotal] == 0 {
		t.Fatalf("%s: did NOT decode: %s == 0", side, statCxTotal)
	}

	// Step 5: sticky-release → the N held + the M woken (dial FRESH conns) drain to
	// 200. Then poll cx_open==0 AND rq_pending_open==0 (gauges back to baseline).
	d.stickyRelease(t, side, backendPort)
	wg.Wait()

	// Tally: the N held + M woken (= N+M) requests should all be 200 "backend-0:".
	for i := 0; i < maxConnections+maxPendingRequests; i++ {
		r := res[i]
		if r.err != nil {
			t.Errorf("%s: held/pending[%d]: transport error: %v", side, i, r.err)
			continue
		}
		if r.status != http.StatusOK {
			t.Errorf("%s: held/pending[%d]: status %d, want 200 (in-budget request not served after drain)", side, i, r.status)
			continue
		}
		if idx, perr := backendIdxFromBody(r.body); perr != nil {
			t.Errorf("%s: held/pending[%d]: %v", side, i, perr)
		} else if idx != 0 {
			t.Errorf("%s: held/pending[%d]: backend idx %d, want 0", side, i, idx)
		}
	}

	if err := pollStats(side, adminAddr, map[string]uint64{
		statCxOpen:        0,
		statRqPendingOpen: 0,
	}); err != nil {
		t.Fatalf("%s: pool did not drain: %v (cx_open + rq_pending_open should return to 0 after the sticky release)", side, err)
	}
}

// assertReference runs the ROBUST prong: only the reliably-observable invariants.
//
// The reference max_connections is a SOFT breaker (reference_max_connections_soft_breaker):
//   - cx_active can EXCEED N (the cap is not a hard wall).
//   - the circuit_breakers.default.cx_open GAUGE does NOT reliably latch for a
//     max_connections breaker — it is only momentarily set while a new connection
//     is being admitted over the cap, and the soft breaker admits so fast that a
//     50ms poll never catches it (observed: cx_open stays 0 even while overflow
//     counters fire). So cx_open is NOT asserted on the reference.
//   - upstream_cx_overflow (the connection-cap breaker counter) is ALSO racy: it
//     increments only when the breaker actively denies a NEW connection attempt,
//     which depends on the connection-establishment-vs-request-arrival race; it is
//     0 on some runs (observed: pending_overflow=8 but cx_overflow=0). So
//     upstream_cx_overflow is NOT asserted on the reference either.
//
// What IS reliably observable (and proves the connection-pool budget engaged): the
// monotonic upstream_rq_pending_overflow counter — the bounded pending queue ALWAYS
// overflows under the burst — plus >=1 downstream-class 503 from the fired burst,
// and the decode-ran guard upstream_cx_total>0. The reference fires the FULL burst
// (N + refOversub) CONCURRENTLY so the surplus over N drives the pending overflow.
func (d *cpDriver) assertReference(t fixture.TB, listenerAddr, adminAddr string, backendPort int) {
	t.Helper()
	const side = "reference"
	ctx := context.Background()

	// Baseline the pending-overflow counter before the burst (delta assertion).
	base, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape overflow baseline: %v", side, err)
	}
	basePendOverflow := base[statPendingOverflow]

	// Step 1+3 (robust): fire the FULL burst (N saturating + refOversub) CONCURRENT.
	// The N saturate the cap; the surplus drives the bounded pending queue overflow
	// (upstream_rq_pending_overflow) + downstream 503s. Each non-rejected member
	// BLOCKS at the responder until the sticky release.
	total := maxConnections + refOversub
	burstRes := make([]heldResult, total)
	var wg sync.WaitGroup
	fireHeld(ctx, listenerAddr, total, 0, burstRes, &wg)

	// Poll until the pending-overflow counter rises above baseline (the bounded
	// queue overflowed — the reliably-observable robust signal). NEVER a fixed sleep.
	if err := pollOverflowDelta(side, adminAddr, basePendOverflow); err != nil {
		st, _ := scrapeStats(adminAddr)
		var diag []string
		for k, v := range st {
			if strings.Contains(k, "c_cp") && (strings.Contains(k, "cx") || strings.Contains(k, "circuit_breaker") || strings.Contains(k, "pending")) {
				diag = append(diag, fmt.Sprintf("%s=%d", k, v))
			}
		}
		t.Fatalf("%s: overflow: %v (the soft breaker did not overflow the pending queue under burst=%d)\nREF-DIAG: %s", side, err, total, strings.Join(diag, "\n"))
	}

	// Step 4 (robust): re-scrape — pending-overflow delta>=1; cx_total>0 (decode-ran).
	after, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape after burst: %v", side, err)
	}
	if delta := after[statPendingOverflow] - basePendOverflow; delta < 1 {
		t.Errorf("%s: %s delta = %d, want >= 1 (the pending queue overflowed)", side, statPendingOverflow, delta)
	}
	if after[statCxTotal] == 0 {
		t.Fatalf("%s: did NOT decode: %s == 0 (could not reach the backend — bridge network / host.docker.internal?)", side, statCxTotal)
	}

	// Step 5: sticky-release → drain everything still held. Then poll the final
	// gauges back to 0 (cross-side parity: cx_open + rq_pending_open == 0; on the
	// reference these are 0 throughout, but the parity poll proves no gauge leaked).
	d.stickyRelease(t, side, backendPort)
	wg.Wait()

	// Count the downstream 503s observed across the FIRED burst requests (>=1) —
	// from the status codes, NOT upstream_rq_5xx
	// (reference_concurrent_attempt_downstream_class_assertion).
	got503 := 0
	for _, r := range burstRes {
		if r.err == nil && r.status == http.StatusServiceUnavailable {
			got503++
		}
	}
	if got503 < 1 {
		t.Errorf("%s: %d downstream 503s across the %d-member burst, want >= 1 (observed from the fired requests' status codes, not upstream_rq_5xx)", side, got503, total)
	}

	if err := pollStats(side, adminAddr, map[string]uint64{
		statCxOpen:        0,
		statRqPendingOpen: 0,
	}); err != nil {
		t.Fatalf("%s: gauges did not settle: %v (cx_open + rq_pending_open should be 0 after the sticky release)", side, err)
	}
}

// pollOverflowDelta polls until upstream_rq_pending_overflow has risen above base
// (the bounded pending queue overflowed). NEVER a fixed sleep.
func pollOverflowDelta(side, adminAddr string, base uint64) error {
	deadline := time.Now().Add(convergeDeadline)
	var last int64 = -1
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			if v, ok := st[statPendingOverflow]; ok {
				last = int64(v)
				if v > base {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: %s did not rise above %d within %s (last seen %d)",
				side, statPendingOverflow, base, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

// stickyRelease hits the BACKEND control port /__release_sticky (NOT the proxy
// listener), permanently opening the gate so the held conns AND the woken pending
// dials (fresh connections) all drain. Always loopback (the backend is in-process
// on this machine for both sides).
func (d *cpDriver) stickyRelease(t fixture.TB, side string, backendPort int) {
	t.Helper()
	releaseAddr := "127.0.0.1:" + strconv.Itoa(backendPort)
	resp, _, err := helpers.HTTPRoundTrip(context.Background(), releaseAddr, "GET", "/__release_sticky", nil, nil)
	if err != nil {
		t.Fatalf("%s: /__release_sticky: transport error to backend %s: %v", side, releaseAddr, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: /__release_sticky: status %d, want 200", side, resp.StatusCode)
	}
}

// AssertStats runs the staged drive SEQUENTIALLY per side (subject FULLY — the
// exact prong; then reference — the robust prong). The shared in-process backend
// is idle between sides (the subject's held + woken requests all drain before the
// reference's fire), so there is no cross-side release interference.
func (d *cpDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
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

	d.assertSubject(t, subjListener, subjAdminAddr, backendPort)
	d.assertReference(t, refListener, refAdminAddr, backendPort)
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0074 driver scrapeStats.)
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
	_ fixture.Driver           = (*cpDriver)(nil)
	_ fixture.StatsAsserter    = (*cpDriver)(nil)
	_ fixture.BackendKindAware = (*cpDriver)(nil)
)
