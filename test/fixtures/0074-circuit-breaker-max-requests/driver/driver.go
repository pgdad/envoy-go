// Package driver registers the 0074-circuit-breaker-max-requests cross-side
// differential fixture (phase 41 SPEC §8 / PLAN Task 10).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture over ONE
// cluster c_cb (lb_policy ROUND_ROBIN) with a circuit_breakers max_requests
// threshold over a SINGLE endpoint — a BlockingHoldResponder (BackendKind 36)
// that HOLDS each "GET /" request open until the driver releases it via "GET
// /__release". It proves that an over-budget request (the (max_requests+1)th
// concurrent in-flight request) is REJECTED with 503 + increments the
// upstream_rq_pending_overflow counter on BOTH the envoy-go (subject) side AND
// the reference-Envoy side, and that the circuit_breakers.default.rq_open gauge
// tracks the open breaker (1 while the budget is full, 0 after release).
//
// # Topology: 1 BlockingHoldResponder (runner-spawned)
//
//   - backend0 → c_cb endpoint 0 (BlockingHoldResponder; holds GET / until
//     /__release, then 200 "backend-0:")
//
// BackendCount() is 1; the uniform BackendKind() is BlockingHoldResponder (NO
// PerHostBackendKind).
//
// # Cluster shape (both sides)
//
//		c_cb: lb_policy ROUND_ROBIN, 1 endpoint, circuit_breakers: { thresholds:
//		        [ { priority: DEFAULT, max_requests: 4 } ] }
//
//	  - Subject (envoy-go): type STATIC, endpoint = 127.0.0.1:<backendPort>
//	    (envoy-go's buildCluster ONLY supports STATIC).
//	  - Reference (Envoy): type STRICT_DNS, endpoint = host.docker.internal:<
//	    backendPort> (the 0066/0069 reference shape; the reference MUST be
//	    STRICT_DNS).
//
// # The driver: fill-the-budget + probe + release (the determinism)
//
// The runner's hooks are DriveReference/DriveSubject (the byte-equiv stream, run
// FIRST) then AssertStats (run LAST, holding BOTH admin addrs). The budget-fill,
// the over-budget probe, and the release ALL run inside AssertStats (the only
// hook holding both admin addrs). The Drive hooks STASH their listener addrs and
// return a fixed, address-independent byte stream ("READY\n") for the runner's
// CompareBytes gate. The config builders STASH the backend port so AssertStats
// can hit the backend's /__release control port (127.0.0.1:<backendPort>,
// loopback — the same machine on both sides).
//
// AssertStats runs SEQUENTIALLY per side (subject FULLY, then reference; the
// shared in-process backend is idle between sides, so no cross-side release
// interference). For each side with listener listenerAddr + admin adminAddr:
//
//  1. Fire maxRequests (4) CONCURRENT GET / (each BLOCKS at the responder).
//     Capture each result (status + body) in slices via a sync.WaitGroup.
//  2. Poll adminAddr/stats until circuit_breakers.default.rq_open == 1
//     (deadline 10s, poll 50ms; NO fixed sleep) — all 4 slots filled.
//  3. Record the upstream_rq_pending_overflow baseline. Fire the (N+1)th GET /
//     (rejected BEFORE the backend) → assert status 503.
//  4. Re-scrape: rq_open == 1 AND (overflow - baseline) >= 1; upstream_rq_total
//     > 0 (decode-ran guard).
//  5. Release: GET /__release on 127.0.0.1:<backendPort> (the BACKEND control
//     port, NOT the proxy listener).
//  6. wg.Wait() — the 4 held requests now return 200 "backend-0:". Assert all 4
//     got 200. Poll rq_open -> 0 (deadline).
//
// # Deliberate non-assertions (recorded departures)
//
//   - The UO access-log response flag is NOT asserted (D-S41-3): envoy-go has no
//     response-flags plumbing. The 503-status + overflow/rq_open stats pair is
//     the proof.
//   - upstream_rq_5xx is NOT asserted (the overflow 503 is a local reply; the
//     subject does not increment the cluster 5xx counter on the rejected path —
//     a cross-side mismatch the fixture avoids).
//
// # Cross-references
//
//   - phase 41 SPEC §8 / PLAN Task 10 (the fixture design).
//   - 0066-health-check-http / 0069-outlier-detection-consecutive-5xx (the
//     cross-side HTTP shape + poll-to-converge + scrapeStats + Bootstrap/Config
//     builders + backendIdxFromBody host attribution).
//   - reference_docker_probe_bridge_network (shared bridge + STRICT_DNS
//     hostname; the "decode ran" guard verifies the reference forwarded traffic).
//   - reference_differential_run_selector (target -run 'TestDifferential/0074').
//   - reference_fixture_workload_constant_desync (constants single-sourced).
//   - reference_differential_asserter_dispatch (cross-side assertions via the
//     StatsAsserter path — NOT SubjectAsserter, which only runs reference-less).
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
	fixtureName = "0074-circuit-breaker-max-requests"

	// clusterName is the single cluster; the stat keys interpolate it.
	clusterName = "c_cb"

	// In-container reference Envoy listener port for l_http. Fixtures run
	// sequentially; a distinct value avoids confusion (the 0073 family took up
	// to 19162, this takes the next-free 19163).
	refContainerListenerPort = 19163

	refAdminPort = 9901

	// backendCount is the number of runner-spawned BlockingHoldResponder hosts.
	backendCount = 1

	// maxRequests is the circuit_breakers DEFAULT-priority max_requests budget.
	// Filling exactly this many concurrent in-flight requests opens the breaker;
	// the next request overflows (503). Single-sourced — the config builders +
	// AssertStats read it (reference_fixture_workload_constant_desync).
	maxRequests = 4

	// Convergence poll budget (NO fixed sleep — poll until the predicate holds).
	convergeDeadline = 10 * time.Second
	convergePoll     = 50 * time.Millisecond
)

// statKey builds a cluster-scoped stat name "cluster.<clusterName>.<suffix>".
func statKey(suffix string) string { return "cluster." + clusterName + "." + suffix }

// The single-sourced stat keys (built from clusterName).
var (
	statRqOpen          = statKey("circuit_breakers.default.rq_open")
	statPendingOverflow = statKey("upstream_rq_pending_overflow")
	statUpstreamRqTotal = statKey("upstream_rq_total")
)

func init() {
	fixture.RegisterFixture(fixtureName, &cbDriver{})
}

// cbDriver is STATEFUL: the Drive hooks stash the per-side listener addrs and the
// config builders stash the backend port, so AssertStats — the only hook holding
// BOTH admin addrs — can fill the budget, probe the breaker, and release.
type cbDriver struct {
	mu           sync.Mutex
	refListener  string // host:port of the reference l_http listener (from DriveReference)
	subjListener string // 127.0.0.1:<port> of the subject l_http listener (from SubjectConfig)
	backendPort  int    // the single backend's host port (for the /__release control hit)
}

func (*cbDriver) BackendCount() int                { return backendCount }
func (*cbDriver) BackendKind() fixture.BackendKind { return fixture.BlockingHoldResponder }
func (*cbDriver) SubjectListenerName() string      { return "l_http" }
func (*cbDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// stashBackendPort memoizes the single backend's host port. Both ReferenceBootstrap
// and SubjectConfig receive the same backendPorts slice and call this; they must
// agree on the SAME port (the shared in-process backend).
func (d *cbDriver) stashBackendPort(backendPorts []int) {
	d.mu.Lock()
	d.backendPort = backendPorts[0]
	d.mu.Unlock()
}

// circuitBreakersBlock is the shared cluster circuit_breakers YAML (identical on
// both sides — NAT-transparent static config). One DEFAULT-priority threshold,
// max_requests = maxRequests.
var circuitBreakersBlock = fmt.Sprintf(`      circuit_breakers:
        thresholds:
          - priority: DEFAULT
            max_requests: %d`, maxRequests)

// routeTable routes / to c_cb (the data path). Identical on both sides.
const routeTable = `                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_cb }`

func (d *cbDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.stashBackendPort(backendPorts)
	// STRICT_DNS + host.docker.internal (the 0066/0069 reference shape). c_cb over
	// the single BlockingHoldResponder, with the circuit_breakers max_requests cap.
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
    - name: c_cb
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
%s
      load_assignment:
        cluster_name: c_cb
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, refAdminPort, refContainerListenerPort, routeTable, circuitBreakersBlock, backendPorts[0])
}

func (d *cbDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.stashBackendPort(backendPorts)
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	// STATIC + 127.0.0.1 (the 0066/0069 subject shape). c_cb over the single
	// BlockingHoldResponder, with the circuit_breakers max_requests cap.
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0074, cluster: envoy-go-differential }
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
    - name: c_cb
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s
      load_assignment:
        cluster_name: c_cb
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, routeTable, circuitBreakersBlock, backendPorts[0])
}

// DriveReference stashes the reference listener addr and returns the fixed
// byte-equiv stream. The real work runs in AssertStats.
func (d *cbDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

// DriveSubject returns the fixed byte-equiv stream (the subject listener addr was
// already stashed in SubjectConfig). The real work runs in AssertStats.
func (d *cbDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0066/0069
// raw /ready probe, verbatim).
func (*cbDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// pollStat scrapes adminAddr/stats every convergePoll until st[key] == want or the
// deadline trips, returning a clear error (with the last value) on timeout.
func pollStat(side, adminAddr, key string, want uint64) error {
	deadline := time.Now().Add(convergeDeadline)
	var last int64 = -1
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			if v, ok := st[key]; ok {
				last = int64(v)
				if v == want {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: %s did not converge to %d within %s (last seen %d)",
				side, key, want, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

// heldResult captures one held GET / outcome (status + body) for the post-release
// 200 assertion.
type heldResult struct {
	status int
	body   []byte
	err    error
}

// assertSide runs the full fill-the-budget + probe + release flow for ONE side.
// listenerAddr is the proxy's l_http listener; adminAddr its admin; backendPort
// the shared backend's host port (for the /__release control hit, always over
// 127.0.0.1 loopback — the same machine on both sides).
func (d *cbDriver) assertSide(t fixture.TB, side, listenerAddr, adminAddr string, backendPort int) {
	t.Helper()
	ctx := context.Background()

	// 1. Fire maxRequests CONCURRENT GET / — each BLOCKS at the responder until
	// the /__release below. Capture each outcome for the post-release assertion.
	results := make([]heldResult, maxRequests)
	var wg sync.WaitGroup
	wg.Add(maxRequests)
	for i := 0; i < maxRequests; i++ {
		go func(i int) {
			defer wg.Done()
			resp, body, err := helpers.HTTPRoundTrip(ctx, listenerAddr, "GET", "/", nil, nil)
			r := heldResult{err: err, body: body}
			if resp != nil {
				r.status = resp.StatusCode
			}
			results[i] = r
		}(i)
	}

	// 2. Poll rq_open -> 1: all maxRequests slots filled (the breaker is open).
	if err := pollStat(side, adminAddr, statRqOpen, 1); err != nil {
		t.Fatalf("%s: fill-the-budget: %v (the %d held requests did not occupy all max_requests slots — is the breaker enforcing? is the backend holding?)", side, err, maxRequests)
	}

	// 3. Baseline the overflow counter, then fire the (N+1)th GET / — rejected
	// BEFORE the backend (the breaker is full) → must be 503.
	base, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape overflow baseline: %v", side, err)
	}
	baseOverflow := base[statPendingOverflow]

	resp, _, err := helpers.HTTPRoundTrip(ctx, listenerAddr, "GET", "/", nil, nil)
	if err != nil {
		t.Fatalf("%s: over-budget GET /: transport error: %v (the (N+1)th request should get a 503 local reply, not a transport failure)", side, err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("%s: over-budget GET /: status %d, want 503 (the breaker did not reject the (max_requests+1)th request)", side, resp.StatusCode)
	}

	// 4. Re-scrape: breaker still open + the overflow counter incremented + the
	// "decode ran" guard (the reference container actually forwarded the fills).
	after, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape after over-budget probe: %v", side, err)
	}
	if after[statRqOpen] != 1 {
		t.Errorf("%s: %s = %d after the over-budget probe, want 1 (breaker should still be open)", side, statRqOpen, after[statRqOpen])
	}
	if delta := after[statPendingOverflow] - baseOverflow; delta < 1 {
		t.Errorf("%s: %s delta = %d, want >= 1 (the overflow counter did not increment on the rejected request; after %d, base %d)",
			side, statPendingOverflow, delta, after[statPendingOverflow], baseOverflow)
	}
	if after[statUpstreamRqTotal] == 0 {
		t.Fatalf("%s: did NOT decode: %s == 0 (could not reach the backend — bridge network / host.docker.internal?)", side, statUpstreamRqTotal)
	}

	// 5. Release: hit the BACKEND control port (NOT the proxy listener). Always
	// loopback (127.0.0.1) — the backend is in-process on this machine for both
	// sides. This frees the maxRequests held requests.
	releaseAddr := "127.0.0.1:" + strconv.Itoa(backendPort)
	relResp, _, err := helpers.HTTPRoundTrip(ctx, releaseAddr, "GET", "/__release", nil, nil)
	if err != nil {
		t.Fatalf("%s: /__release: transport error to backend %s: %v", side, releaseAddr, err)
	}
	if relResp.StatusCode != http.StatusOK {
		t.Fatalf("%s: /__release: status %d, want 200 (the backend control port did not release)", side, relResp.StatusCode)
	}

	// 6. The held requests now return 200 "backend-0:". Assert all maxRequests got
	// 200 with a backend-0 body.
	wg.Wait()
	for i, r := range results {
		if r.err != nil {
			t.Errorf("%s: held GET /[%d]: transport error: %v", side, i, r.err)
			continue
		}
		if r.status != http.StatusOK {
			t.Errorf("%s: held GET /[%d]: status %d, want 200 (the in-budget held request was not served after release)", side, i, r.status)
			continue
		}
		idx, perr := backendIdxFromBody(r.body)
		if perr != nil {
			t.Errorf("%s: held GET /[%d]: %v", side, i, perr)
			continue
		}
		if idx != 0 {
			t.Errorf("%s: held GET /[%d]: backend idx %d, want 0 (the single host)", side, i, idx)
		}
	}

	// Poll rq_open -> 0: after the held requests drained, the breaker closes.
	if err := pollStat(side, adminAddr, statRqOpen, 0); err != nil {
		t.Fatalf("%s: breaker did not close after release: %v (rq_open should return to 0 once the held requests drained)", side, err)
	}
}

// AssertStats runs the fill-the-budget + probe + release flow SEQUENTIALLY per
// side (subject FULLY, then reference). The shared in-process backend is idle
// between sides (subject's held requests are all released before reference's fire),
// so there is no cross-side release interference.
func (d *cbDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
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
	// clean between sides).
	d.assertSide(t, "subject", subjListener, subjAdminAddr, backendPort)
	d.assertSide(t, "reference", refListener, refAdminAddr, backendPort)
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0066/0069 driver scrapeStats,
// verbatim.)
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
	_ fixture.Driver           = (*cbDriver)(nil)
	_ fixture.StatsAsserter    = (*cbDriver)(nil)
	_ fixture.BackendKindAware = (*cbDriver)(nil)
)
