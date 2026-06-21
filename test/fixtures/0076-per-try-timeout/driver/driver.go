// Package driver registers the 0076-per-try-timeout cross-side differential
// fixture (phase 42.2a SPEC / PLAN Task 8).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture that proves the
// HTTP retry-loop per_try_timeout (a per-ATTEMPT deadline) behaves IDENTICALLY on
// the envoy-go (subject) side and the reference-Envoy side. It has ONE cluster +
// ONE route over a SINGLE held backend:
//
//   - c_ptt: lb_policy ROUND_ROBIN (single host), the BlockingHoldResponder
//     (BackendKind 36) — accepts each connection but HOLDS each "GET /<seg>"
//     request open (never responds) until a "GET /__release" control request.
//
//   - /ptt → c_ptt, retry_policy{retry_on:"5xx", num_retries:3, per_try_timeout:0.25s}
//
// With a small per_try_timeout T (250ms) over a backend that never answers, EVERY
// attempt blocks past T, so the per-try-timeout fires on each attempt → a
// synthesized 504. 504 is a 5xx, so under retry_on:"5xx" each timed-out attempt is
// retriable; the retry loop runs all num_retries+1 (4) attempts, exhausts the cap,
// and returns the final 504 to the client. SINGLE host ⇒ deterministic,
// offset-irrelevant, cross-side-EXACT:
//
//	cluster.c_ptt.upstream_rq_per_try_timeout    (delta) == numRetries+1 (4)
//	cluster.c_ptt.upstream_rq_retry              (delta) == numRetries   (3)
//	cluster.c_ptt.upstream_rq_retry_limit_exceeded (delta) == 1
//	cluster.c_ptt.upstream_rq_retry_success      (delta) == 0 (nothing recovered)
//	cluster.c_ptt.upstream_rq_total              (delta) == numRetries+1 (4)
//	cluster.c_ptt.upstream_rq_total > 0 (decode-ran guard, reference side)
//
// # Topology: 1 BlockingHoldResponder (runner-spawned)
//
//   - backend0 → c_ptt endpoint 0 (BlockingHoldResponder; holds GET /ptt until
//     /__release). BackendCount() is 1; the uniform BackendKind() is
//     BlockingHoldResponder (NO PerHostBackendKind, NO new BackendKind authored).
//
//   - Subject (envoy-go): type STATIC, endpoint = 127.0.0.1:<backendPort>
//     (envoy-go's buildCluster ONLY supports STATIC).
//
//   - Reference (Envoy): type STRICT_DNS, endpoint = host.docker.internal:<
//     backendPort> (the 0074/0075 reference shape; the reference MUST be STRICT_DNS
//     over the bridge).
//
// # The driver: drive ONE /ptt + delta the per-try-timeout stats (sequential per side)
//
// The runner's hooks are DriveReference/DriveSubject (the byte-equiv stream, run
// FIRST) then AssertStats (run LAST, holding BOTH admin addrs). All the measured
// work runs inside AssertStats. The Drive hooks STASH their listener addrs and
// return a fixed "READY\n" for the runner's CompareBytes gate. The config builders
// STASH the backend port so AssertStats can hit the backend's /__release control
// port (127.0.0.1:<backendPort>, loopback — the same machine on both sides).
//
// AssertStats runs SEQUENTIALLY per side (subject FULLY, then reference): one GET
// /ptt drives the entire 4-attempt retry loop INSIDE the proxy (~1.2s; the
// per_try_timeout is the FEATURE's own timing — NOT a time.Sleep). The held
// attempts park inside the backend; after the assertions a GET /__release on the
// backend control port drains them, re-arming the gate for the next side. There is
// NO concurrency in the driver (unlike 0074): the retry loop exhausts within the
// single sequential request.
//
// # Cross-references
//
//   - phase 42.2a SPEC / PLAN Task 8 (the fixture design).
//   - 0074-circuit-breaker-max-requests (the BlockingHoldResponder topology +
//     stashBackendPort + /__release control hit + the STATIC vs STRICT_DNS shapes).
//   - 0075-retry-loop (the retry-cluster + cross-side StatsAsserter pattern +
//     scrapeStats/assertDelta + the retry_policy YAML + the stat_prefix
//     single-sourcing).
//   - reference_differential_asserter_dispatch (cross-side assertions via the
//     StatsAsserter path — NOT SubjectAsserter, which only runs reference-less).
//   - reference_fixture_workload_constant_desync (constants single-sourced).
//   - reference_differential_run_selector (target -run 'TestDifferential/0076').
//   - reference_docker_probe_bridge_network (shared bridge + STRICT_DNS hostname;
//     the upstream_rq_total > 0 "decode ran" guard).
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
	fixtureName = "0076-per-try-timeout"

	// In-container reference Envoy listener port for l_http. Fixtures run
	// sequentially; a distinct value avoids confusion (0075 took 19164, this
	// takes the next-free 19165).
	refContainerListenerPort = 19165

	refAdminPort = 9901

	// statPrefix is the HCM listener stat_prefix (single-sourced: the bootstraps
	// read this; mirrors the 0074/0075 convention).
	statPrefix = "ingress_http"

	// clusterPtt is the single cluster over the held backend; the stat keys
	// interpolate it.
	clusterPtt = "c_ptt"

	// backendCount is the number of runner-spawned BlockingHoldResponder hosts.
	backendCount = 1

	// numRetries is the /ptt route's num_retries (the static retry cap). EVERY
	// attempt per-try-times-out against the held backend, so the loop runs
	// numRetries+1 (4) attempts, exhausts the cap, and returns the final 504.
	numRetries = 3

	// perTryTimeout is the /ptt route's per_try_timeout (250ms): small enough that
	// every attempt fires it while the backend holds, large enough not to flake on
	// per-attempt dial+roundtrip jitter. Below connect_timeout (1s) so it is the
	// per_try deadline — not the connect timeout — that fires.
	perTryTimeout = "0.25s"

	// HTTP client timeout on the GET /ptt: the whole 4-attempt retry loop runs
	// inside the proxy (~1.2s = 4 * 0.25s + backoff), so a generous bound.
	driveTimeout = 30 * time.Second
)

// Stat keys (single-sourced — all interpolate clusterPtt; TestStatKeys pins the
// resulting strings against the YAML's cluster name).
var (
	statPerTryTimeout = "cluster." + clusterPtt + ".upstream_rq_per_try_timeout"
	statRetry         = "cluster." + clusterPtt + ".upstream_rq_retry"
	statRetryLimitExc = "cluster." + clusterPtt + ".upstream_rq_retry_limit_exceeded"
	statRetrySuccess  = "cluster." + clusterPtt + ".upstream_rq_retry_success"
	statRqTotal       = "cluster." + clusterPtt + ".upstream_rq_total"
)

func init() {
	fixture.RegisterFixture(fixtureName, &pttDriver{})
}

// pttDriver is STATEFUL: the Drive hooks stash the per-side listener addrs and the
// config builders stash the backend port, so AssertStats — the only hook holding
// BOTH admin addrs — can drive /ptt, delta-assert the per-try-timeout stats, and
// release the parked held attempts.
type pttDriver struct {
	mu           sync.Mutex
	refListener  string // host:port of the reference l_http listener (from DriveReference)
	subjListener string // 127.0.0.1:<port> of the subject l_http listener (from SubjectConfig)
	backendPort  int    // the single backend's host port (for the /__release control hit)
}

func (*pttDriver) BackendCount() int                { return backendCount }
func (*pttDriver) BackendKind() fixture.BackendKind { return fixture.BlockingHoldResponder }
func (*pttDriver) SubjectListenerName() string      { return "l_http" }
func (*pttDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// stashBackendPort memoizes the single backend's host port. Both ReferenceBootstrap
// and SubjectConfig receive the same backendPorts slice and call this; they must
// agree on the SAME port (the shared in-process backend).
func (d *pttDriver) stashBackendPort(backendPorts []int) {
	d.mu.Lock()
	d.backendPort = backendPorts[0]
	d.mu.Unlock()
}

// routeTable routes /ptt → c_ptt with the per_try_timeout retry_policy. Identical
// on both sides (the retry_policy is static config). retry_on:"5xx" matches the
// synthesized 504 a per-try-timeout produces, so every timed-out attempt retries.
var routeTable = fmt.Sprintf(`                      routes:
                        - match: { prefix: "/ptt" }
                          route:
                            cluster: %s
                            retry_policy:
                              retry_on: "5xx"
                              num_retries: %d
                              per_try_timeout: %s`,
	clusterPtt, numRetries, perTryTimeout)

func (d *pttDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.stashBackendPort(backendPorts)
	// STRICT_DNS + host.docker.internal (the 0074/0075 reference shape). c_ptt over
	// the single BlockingHoldResponder, with the per_try_timeout retry route.
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
`, refAdminPort, refContainerListenerPort, statPrefix, routeTable, clusterPtt, clusterPtt, backendPorts[0])
}

func (d *pttDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.stashBackendPort(backendPorts)
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	// STATIC + 127.0.0.1 (the 0074/0075 subject shape). Same cluster/route topology.
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0076, cluster: envoy-go-differential }
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
`, subjAdminPort, subjListenerPort, statPrefix, routeTable, clusterPtt, clusterPtt, backendPorts[0])
}

// DriveReference stashes the reference listener addr and returns the fixed
// byte-equiv stream. The real work runs in AssertStats.
func (d *pttDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

// DriveSubject returns the fixed byte-equiv stream (the subject listener addr was
// already stashed in SubjectConfig). The real work runs in AssertStats.
func (d *pttDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0074/0075
// raw /ready probe, verbatim).
func (*pttDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// assertSide drives ONE GET /ptt for a side, asserts the final 504, delta-asserts
// the per-try-timeout/retry stats, then releases the parked held attempts (which
// re-arms the backend gate for the next side). adminAddr is the proxy admin;
// listenerAddr the l_http listener; backendPort the shared backend's host port
// (for the /__release control hit, always over 127.0.0.1 loopback).
func (d *pttDriver) assertSide(t fixture.TB, side, listenerAddr, adminAddr string, backendPort int) {
	t.Helper()
	ctx := context.Background()

	base, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape baseline /stats: %v", side, err)
	}

	// One GET /ptt: the proxy runs the entire 4-attempt retry loop internally
	// (~1.2s, each attempt per-try-times-out against the held backend) and returns
	// the final 504. A generous client timeout covers the loop's wall time.
	client := &http.Client{Timeout: driveTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+listenerAddr+"/ptt", nil)
	if err != nil {
		t.Fatalf("%s: build GET /ptt: %v", side, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s: GET /ptt: transport error: %v (the retry loop should return a 504 local reply, not a transport failure)", side, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Errorf("%s: GET /ptt status %d, want 504 (every attempt per-try-times-out against the held backend → the retry loop exhausts to a final 504)", side, resp.StatusCode)
	}

	fin, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape final /stats: %v", side, err)
	}

	// "decode ran" guard (reference_docker_probe_bridge_network): the ref container
	// must have reached the backend over the bridge (per_try_timeout cannot fire on
	// an attempt that never connected).
	if side == "reference" && fin[statRqTotal]-base[statRqTotal] == 0 {
		t.Fatalf("reference did NOT decode: %s delta == 0 (container could not reach the held backend — bridge network / host.docker.internal?)", statRqTotal)
	}

	// Cross-side EXACT (single host, no round-robin offset): every attempt
	// per-try-timed-out, so the loop ran numRetries+1 attempts and exhausted.
	assertDelta(t, side, fin, base, statPerTryTimeout, numRetries+1) // 4 — every attempt timed out
	assertDelta(t, side, fin, base, statRetry, numRetries)           // 3 retries
	assertDelta(t, side, fin, base, statRetryLimitExc, 1)            // exhausted once
	assertDelta(t, side, fin, base, statRetrySuccess, 0)             // nothing recovered
	assertDelta(t, side, fin, base, statRqTotal, numRetries+1)       // 4 attempts

	// Release: hit the BACKEND control port (NOT the proxy listener), always over
	// 127.0.0.1 loopback (the backend is in-process on this machine for both sides).
	// This drains the parked held attempts and re-arms the gate for the next side.
	releaseAddr := "127.0.0.1:" + strconv.Itoa(backendPort)
	relResp, _, err := helpers.HTTPRoundTrip(ctx, releaseAddr, "GET", "/__release", nil, nil)
	if err != nil {
		t.Fatalf("%s: /__release: transport error to backend %s: %v", side, releaseAddr, err)
	}
	if relResp.StatusCode != http.StatusOK {
		t.Fatalf("%s: /__release: status %d, want 200 (the backend control port did not release)", side, relResp.StatusCode)
	}
}

// AssertStats drives ONE /ptt per side SEQUENTIALLY (subject FULLY, then
// reference) and delta-asserts the per-try-timeout stats (the only hook holding
// both admin addrs; the 0075 cross-side pattern). The shared in-process backend is
// drained (via /__release) between sides, so there is no cross-side interference.
func (d *pttDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
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
// "name: value" lines into a map[name]uint64. (The 0074/0075 driver scrapeStats,
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
	_ fixture.Driver           = (*pttDriver)(nil)
	_ fixture.StatsAsserter    = (*pttDriver)(nil)
	_ fixture.BackendKindAware = (*pttDriver)(nil)
)
