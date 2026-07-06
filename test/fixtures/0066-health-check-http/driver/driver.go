// Package driver registers the 0066-health-check-http cross-side differential
// fixture (phase 39.1 SPEC §8.1 / PLAN Task 12).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture over ONE
// cluster c_hc (lb_policy ROUND_ROBIN) with active HTTP health checking over
// THREE endpoints — 2 LIVE HTTPEcho backends + 1 DEAD host (a host:port with no
// listener → connect refused → the probe fails). It proves that an unhealthy
// (dead) upstream host is DETECTED by active health checking and REMOVED from LB
// rotation on BOTH the envoy-go (subject) side and the reference-Envoy side.
//
// The healthy fraction after convergence is 2/3 ≈ 66% > the 50% panic threshold,
// so the cluster FILTERS the dead host (it does NOT enter panic mode and spray
// across all hosts) — the load lands EXCLUSIVELY on the 2 live backends.
//
// # Topology: 2 LIVE backends (runner-spawned) + 1 DEAD host (unbound port)
//
//   - backend0 → c_hc endpoint 0 (LIVE; HTTPEcho 200s every path incl. /health)
//   - backend1 → c_hc endpoint 1 (LIVE; HTTPEcho 200s every path incl. /health)
//   - deadPort → c_hc endpoint 2 (DEAD; no listener → connect refused → probe fails)
//
// The DEAD host is NOT a runner backend (the runner spawns BackendCount()==2 live
// HTTPEcho backends). The driver allocates a host port, binds it to learn the
// number, then CLOSES the listener so the port stays unbound for the run — both
// sides reference that same port number (reference via host.docker.internal:<dead>,
// subject via 127.0.0.1:<dead>), and a probe to it is refused on both sides.
//
// # Cluster shape (both sides)
//
//		c_hc: lb_policy ROUND_ROBIN, 3 endpoints, health_checks: [{
//		        interval: 0.5s, timeout: 0.5s,
//		        unhealthy_threshold: 1, healthy_threshold: 1,
//		        http_health_check: { path: /health } }]
//
//	  - Subject (envoy-go): type STATIC, endpoints = 127.0.0.1:<live0,live1,dead>
//	    (envoy-go's buildCluster ONLY supports STATIC).
//	  - Reference (Envoy): type STRICT_DNS, endpoints = host.docker.internal:<live0,
//	    live1,dead> (the 0065 reference shape; the reference MUST be STRICT_DNS).
//
// # The driver: poll-to-converge (the NEW determinism mechanism)
//
// The runner's hooks are: DriveReference/DriveSubject (the byte-equiv stream, run
// FIRST) then AssertStats (run LAST, holding BOTH admin addrs). Because the load
// MUST NOT begin until the dead host has been detected + filtered (an early
// request could be round-robined to the dead host → 5xx), the convergence poll +
// the load + the assertions ALL run inside AssertStats (the only hook holding both
// admin addrs). The Drive hooks STASH their listener addrs and return a fixed,
// address-independent byte stream ("READY\n") for the runner's CompareBytes gate.
//
//	AssertStats:
//	 1. Poll /stats on BOTH sides until cluster.c_hc.membership_healthy == 2
//	    (deadline 30s, every 200ms; NO fixed sleep). Fail clearly on timeout.
//	 2. Load: send n=100 GET / to each side's listener.
//	 3. Assert 100% served by the 2 LIVE backends (response body "backend-<idx>:"
//	    attributes the host; the dead host can serve nothing) + 0 5xx.
//	 4. Cross-side stats: membership_healthy==2 both sides; health_check.attempt>0,
//	    success>0, failure>0 both sides; upstream_rq_total==n, upstream_rq_2xx==n,
//	    upstream_rq_5xx==0 both sides; upstream_cx_active==0 (quiesced) both sides.
//
// # Cross-references
//
//   - phase 39.1 SPEC §8.1 / PLAN Task 12 (the fixture design).
//   - 0065-weighted-clusters (the cross-side HTTP shape: reference STRICT_DNS /
//     host.docker.internal, subject STATIC / 127.0.0.1; HTTPEcho backend echoing
//     "backend-<idx>:<seg>"; scrapeStats; the Bootstrap/Config builders).
//   - reference_docker_probe_bridge_network (shared bridge + STRICT_DNS hostnames;
//     the "decode ran" guard verifies the reference actually forwarded traffic).
//   - reference_differential_run_selector (target -run 'TestDifferential/0066').
//   - reference_fixture_workload_constant_desync (n + host counts single-sourced).
//   - reference_differential_asserter_dispatch (cross-side assertions via the
//     StatsAsserter path — NOT SubjectAsserter, which only runs reference-less).
package driver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0066-health-check-http"

	// In-container reference Envoy listener port for l_http. Fixtures run
	// sequentially; a distinct value avoids confusion — 0065 takes 19154,
	// this takes 19155.
	refContainerListenerPort = 19155

	refAdminPort = 9901

	// backendCount is the number of LIVE runner-spawned HTTPEcho backends. The
	// DEAD host is a separately-allocated unbound port (NOT a runner backend).
	backendCount = 2

	// healthyAfterConverge is the membership_healthy gauge value the poll waits
	// for: the 2 live hosts (the dead host is filtered). Single-sourced; the
	// stats arm reuses it.
	healthyAfterConverge = backendCount // == 2

	// n is the load-phase request count per side. 100 round-robin GET / over the
	// 2 live hosts → ~50 each; the assertion is 100%-to-live (the dead host serves
	// nothing) + 0 5xx, NOT a band, so n need not be large.
	n = 100

	// Convergence poll budget (NO fixed sleep — poll until the predicate holds).
	convergeDeadline = 30 * time.Second
	convergePoll     = 200 * time.Millisecond

	// Warmup gate: the reference's membership_healthy gauge updates on the main
	// thread BEFORE the worker-thread LB host-sets drop the dead host (a
	// propagation window), so an early request can still be round-robined to the
	// dead host → a transient 503 even after the gauge reads 2. The warmup sends
	// 503-tolerant requests until warmupStable CONSECUTIVE 200s prove the worker
	// rotation has dropped the dead host, THEN the strict measured phase runs.
	// Round-robin guarantees the dead host every 3rd pick if it is NOT filtered,
	// so an unfiltered build can never reach warmupStable consecutive 200s — the
	// gate still bites the deliberate breaks.
	warmupStable   = 10
	warmupDeadline = 15 * time.Second
)

func init() {
	fixture.RegisterFixture(fixtureName, &hcDriver{})
}

// hcDriver is STATEFUL: the Drive hooks stash the per-side listener addrs (the
// reference listener mapped port is only knowable at DriveReference; the subject
// listener + admin are knowable at SubjectConfig) so AssertStats — the only hook
// holding BOTH admin addrs — can poll-converge, load, and assert.
type hcDriver struct {
	mu           sync.Mutex
	refListener  string // host:port of the reference l_http listener (from DriveReference)
	subjListener string // 127.0.0.1:<port> of the subject l_http listener (from SubjectConfig)
	deadPort     int    // the unbound host port shared by both sides' dead endpoint
}

func (*hcDriver) BackendCount() int                { return backendCount }
func (*hcDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (*hcDriver) SubjectListenerName() string      { return "l_http" }
func (*hcDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// allocDeadPort binds 0.0.0.0:0, captures the assigned port, then CLOSES the
// listener so the port stays unbound — a connect to it is refused (the dead-host
// probe-failure mechanism). Memoized: both ReferenceBootstrap and SubjectConfig
// call it; they must agree on the SAME port number.
func (d *hcDriver) allocDeadPort() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.deadPort != 0 {
		return d.deadPort
	}
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		panic(fmt.Sprintf("%s: alloc dead port: %v", fixtureName, err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // release → nothing listens → connect refused → probe fails
	d.deadPort = port
	return port
}

// healthChecksBlock is the shared cluster health_checks YAML (identical on both
// sides — NAT-transparent static config). One HTTP checker, /health probe path,
// fast convergence (interval/timeout 0.5s, thresholds 1/1).
const healthChecksBlock = `      health_checks:
        - interval: 0.5s
          timeout: 0.5s
          unhealthy_threshold: 1
          healthy_threshold: 1
          http_health_check:
            path: /health`

// routeTable routes / to c_hc (the data path). Identical on both sides.
const routeTable = `                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_hc }`

func (d *hcDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	dead := d.allocDeadPort()
	// STRICT_DNS + host.docker.internal (the 0065 reference shape). c_hc over the
	// 2 live backends + the dead host, with the active HTTP health checker.
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
    - name: c_hc
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
%s
      load_assignment:
        cluster_name: c_hc
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, refAdminPort, refContainerListenerPort, routeTable, healthChecksBlock, backendPorts[0], backendPorts[1], dead)
}

func (d *hcDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	dead := d.allocDeadPort()
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	// STATIC + 127.0.0.1 (the 0065 subject shape). c_hc over the 2 live backends
	// + the dead host, with the active HTTP health checker.
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0066, cluster: envoy-go-differential }
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
    - name: c_hc
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s
      load_assignment:
        cluster_name: c_hc
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, routeTable, healthChecksBlock, backendPorts[0], backendPorts[1], dead)
}

// DriveReference stashes the reference listener addr and returns the fixed
// byte-equiv stream. The real load runs in AssertStats (after convergence).
func (d *hcDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

// DriveSubject returns the fixed byte-equiv stream (the subject listener addr was
// already stashed in SubjectConfig). The real load runs in AssertStats.
func (d *hcDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0065 raw
// /ready probe, verbatim).
func (*hcDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// pollMembershipHealthy scrapes adminAddr/stats every convergePoll until
// cluster.c_hc.membership_healthy == healthyAfterConverge or the deadline trips.
func pollMembershipHealthy(side, adminAddr string) error {
	deadline := time.Now().Add(convergeDeadline)
	var last int64 = -1
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			if v, ok := st["cluster.c_hc.membership_healthy"]; ok {
				last = int64(v)
				if v == healthyAfterConverge {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: cluster.c_hc.membership_healthy did not converge to %d within %s (last seen %d) — dead host not detected? (live backends 200 on /health? dead host actually refuses? subject checker started post-Freeze?)",
				side, healthyAfterConverge, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

// backendIdxFromBody parses the HTTPEcho canned body "backend-<idx>:<seg>" and
// returns the embedded backend idx (the host-attribution signal).
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

// loadAndTally sends n GET / to addr and returns per-live-backend hit counts. A
// non-200 (e.g. a 5xx from a connection-failure to the dead host) is a hard error
// — the dead host MUST be filtered, so every request MUST be served 200 by a live
// backend with a "backend-<idx>:" body.
func loadAndTally(ctx context.Context, side, addr string) ([backendCount]int, error) {
	var counts [backendCount]int
	for i := 0; i < n; i++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err != nil {
			return counts, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if resp.StatusCode != http.StatusOK {
			return counts, fmt.Errorf("%s: GET /[%d]: status %d, want 200 (dead host NOT filtered → connection-failure 5xx?)", side, i, resp.StatusCode)
		}
		idx, err := backendIdxFromBody(body)
		if err != nil {
			return counts, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if idx < 0 || idx >= backendCount {
			return counts, fmt.Errorf("%s: GET /[%d]: backend idx %d out of LIVE range [0,%d) — a request reached a host outside the 2 live backends", side, i, idx, backendCount)
		}
		counts[idx]++
	}
	return counts, nil
}

// warmupUntilStable sends GET / tolerating transient 503s until warmupStable
// CONSECUTIVE 200s, or the deadline trips. It closes the reference's
// gauge→worker-set propagation window (membership_healthy reads 2 before the
// worker LB drops the dead host) so the strict measured phase is race-free. An
// unfiltered build (deliberate break) round-robins to the dead host every 3rd
// pick → never warmupStable consecutive 200s → this errors, preserving liveness.
func warmupUntilStable(ctx context.Context, side, addr string) error {
	deadline := time.Now().Add(warmupDeadline)
	consecutive := 0
	lastCode, lastErr := -1, error(nil)
	for {
		resp, _, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err == nil && resp.StatusCode == http.StatusOK {
			consecutive++
			if consecutive >= warmupStable {
				return nil
			}
		} else {
			consecutive = 0
			lastErr = err
			if resp != nil {
				lastCode = resp.StatusCode
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: data path did not stabilize to %d consecutive 200s within %s (last status %d, err %v) — dead host still in worker rotation?",
				side, warmupStable, warmupDeadline, lastCode, lastErr)
		}
	}
}

// AssertStats is the in-band poll-converge + load + assert (the only hook holding
// both admin addrs). See the package doc for the four-step shape.
func (d *hcDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ctx := context.Background()

	d.mu.Lock()
	refListener := d.refListener
	subjListener := d.subjListener
	d.mu.Unlock()
	if refListener == "" || subjListener == "" {
		t.Fatalf("listener addrs not stashed (ref=%q subj=%q) — Drive hooks did not run?", refListener, subjListener)
	}

	// 1. Poll-to-converge: both sides must filter the dead host BEFORE load.
	if err := pollMembershipHealthy("reference", refAdminAddr); err != nil {
		t.Fatalf("converge: %v", err)
	}
	if err := pollMembershipHealthy("subject", subjAdminAddr); err != nil {
		t.Fatalf("converge: %v", err)
	}

	// 1b. Warmup: close the gauge→worker-set propagation window before measuring.
	if err := warmupUntilStable(ctx, "reference", refListener); err != nil {
		t.Fatalf("warmup: %v", err)
	}
	if err := warmupUntilStable(ctx, "subject", subjListener); err != nil {
		t.Fatalf("warmup: %v", err)
	}

	// 1c. Baselines AFTER warmup: the per-request counters are measured as a DELTA
	// over the load phase (the convergence-poll + warmup requests also increment
	// upstream_rq_*, so absolute counts would over-count by a variable amount).
	refBase, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref baseline /stats: %v", err)
	}
	subjBase, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj baseline /stats: %v", err)
	}

	// 2. Load: n GET / on each side, after convergence + warmup.
	refCounts, err := loadAndTally(ctx, "reference", refListener)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	subjCounts, err := loadAndTally(ctx, "subject", subjListener)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// 3. 100%-to-live (per side): every request was served 200 by a live backend
	// (loadAndTally already failed on any non-200 / dead-host hit); the tally must
	// sum to n and both live backends must be touched (ROUND_ROBIN over 2 live).
	for _, sd := range []struct {
		side   string
		counts [backendCount]int
	}{{"reference", refCounts}, {"subject", subjCounts}} {
		sum := 0
		for _, c := range sd.counts {
			sum += c
		}
		if sum != n {
			t.Errorf("%s: live-backend tally sum %d != %d (some requests not served by a live backend?)", sd.side, sum, n)
		}
		for i, c := range sd.counts {
			if c == 0 {
				t.Errorf("%s: live backend[%d] served 0 requests — ROUND_ROBIN did not spread over the 2 live hosts (dead host not actually filtered, or a live host wrongly marked unhealthy?)", sd.side, i)
			}
		}
	}

	// 4. Cross-side stats. Scrape AFTER load (quiesce + final counters).
	ref, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}

	// "decode ran" guard (reference_docker_probe_bridge_network): if the reference
	// container forwarded nothing, the readout is untrustworthy.
	if ref["cluster.c_hc.upstream_rq_total"] == 0 {
		t.Fatalf("reference did NOT decode: cluster.c_hc.upstream_rq_total == 0 (container could not reach live backends — bridge network / host.docker.internal?)")
	}

	for _, sd := range []struct {
		side string
		st   map[string]uint64
		base map[string]uint64
	}{{"reference", ref, refBase}, {"subject", subj, subjBase}} {
		// Membership: exactly the 2 live hosts remain (dead host filtered).
		assertEq(t, sd.side, sd.st, "cluster.c_hc.membership_healthy", healthyAfterConverge)
		// Membership total stays at the full 3 endpoints (filtering, not removal).
		assertEq(t, sd.side, sd.st, "cluster.c_hc.membership_total", 3)
		// The health checker ran: ≥1 attempt, ≥1 success (live hosts), ≥1 failure
		// (the dead host fails every probe).
		assertPositive(t, sd.side, sd.st, "cluster.c_hc.health_check.attempt")
		assertPositive(t, sd.side, sd.st, "cluster.c_hc.health_check.success")
		assertPositive(t, sd.side, sd.st, "cluster.c_hc.health_check.failure")
		// Load conservation (DELTA over the measured load, baseline post-warmup):
		// all n requests routed to a live host, all 2xx, 0 5xx in the load phase.
		assertDelta(t, sd.side, sd.st, sd.base, "cluster.c_hc.upstream_rq_total", n)
		assertDelta(t, sd.side, sd.st, sd.base, "cluster.c_hc.upstream_rq_2xx", n)
		// 0 5xx during the load phase (warmup-window 503s are in the baseline, not
		// the delta — the strict measured phase must be 503-free).
		assertDelta(t, sd.side, sd.st, sd.base, "cluster.c_hc.upstream_rq_5xx", 0)
		// Quiesced (Connection: close → each request is a fresh dial that completes).
		assertEq(t, sd.side, sd.st, "cluster.c_hc.upstream_cx_active", 0)
	}
}

func assertEq(t fixture.TB, side string, st map[string]uint64, key string, want uint64) {
	t.Helper()
	v, ok := st[key]
	if !ok {
		t.Errorf("%s: %s ABSENT in /stats", side, key)
		return
	}
	if v != want {
		t.Errorf("%s: %s = %d, want %d", side, key, v, want)
	}
}

// assertDelta asserts (final[key] - base[key]) == want — the change in a counter
// over the measured load phase. Absent keys read as 0 (reference Envoy lazily
// allocates per-response-class counters), so a 0-want passes when the class was
// never touched in either scrape.
func assertDelta(t fixture.TB, side string, st, base map[string]uint64, key string, want uint64) {
	t.Helper()
	got := st[key] - base[key]
	if got != want {
		t.Errorf("%s: %s delta = %d, want %d (final %d, base %d)", side, key, got, want, st[key], base[key])
	}
}

func assertPositive(t fixture.TB, side string, st map[string]uint64, key string) {
	t.Helper()
	v, ok := st[key]
	if !ok {
		t.Errorf("%s: %s ABSENT in /stats", side, key)
		return
	}
	if v == 0 {
		t.Errorf("%s: %s = 0, want > 0", side, key)
	}
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0065 driver scrapeStats,
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
	_ fixture.Driver           = (*hcDriver)(nil)
	_ fixture.StatsAsserter    = (*hcDriver)(nil)
	_ fixture.BackendKindAware = (*hcDriver)(nil)
)
