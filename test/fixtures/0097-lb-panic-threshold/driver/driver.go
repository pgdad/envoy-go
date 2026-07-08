// Package driver registers the 0097-lb-panic-threshold cross-side
// differential fixture (phase 54 SPEC §10 / PLAN Task 7).
//
// Cross-side [http_connection_manager + router] fixture over ONE HTTP
// listener (l_http) path-routing to THREE STATIC clusters (c_pt_a / c_pt_b /
// c_pt_c), 5 hosts each, each with an active HTTP health checker (path
// /healthz, fast convergence: interval/timeout 1s, thresholds 1/1 — the
// 0096-lb-priority healthChecksBlock precedent). All 15 hosts are
// DRIVER-OWNED toggleable HTTP responders (the 0095/0096 toggleResponder
// precedent, extended here to carry a cluster label alongside the host
// index, since this fixture has THREE clusters instead of one/two tiers).
//
// The ONLY per-cluster configuration difference is
// common_lb_config.healthy_panic_threshold:
//
//	c_pt_a: { value: 80 }    — PANICS   (60% healthy < 80% threshold)
//	c_pt_b: absent            — NO panic (60% healthy >= the 50% default)
//	c_pt_c: { value: 60.9 }  — NO panic (floor(60.9)=60; 60% < 60% is FALSE
//	                            — the AMEND-PT1 integer-truncation
//	                            discriminator: a naive float-fraction compare
//	                            would have panicked here since 60% < 60.9%)
//
// Each cluster is degraded to the SAME fixed 2-of-5 hosts (indices 3 and 4 —
// degradedPerCluster), giving 3/5 = 60% healthy in every cluster. This is
// the FIRST cross-side proof of the panic construct: three clusters at an
// IDENTICAL healthy percentage, differing only in the configured threshold,
// demonstrating that healthy_panic_threshold — not some other per-cluster
// difference — is what flips c_pt_a's routing behavior relative to c_pt_b
// and c_pt_c.
//
// BackendCount() returns 1: a single throwaway runner-spawned HTTPEcho
// backend that NO cluster in this fixture's bootstrap references (the
// "spawn-but-don't-use" pattern — runner_test.go:221 requires BackendCount()
// >= 1; see the 0018-http-rbac precedent for a backend spawned but only
// conditionally used by some scenarios). BackendKind() stays HTTPEcho — no
// new BackendKind is introduced (the tail stays at 38,
// H2GoawayResponder).
//
// AssertStats drives all three arms in-band (the only hook holding both
// admin addrs):
//
//  1. degrade indices 3 and 4 in EACH of c_pt_a/b/c (toggle /healthz to 503).
//  2. pollMembershipHealthy(side, adminAddr, cluster, 3) for each cluster.
//  3. warmupUntilStable per cluster path — c_pt_b/c_pt_c exclude the
//     degraded hosts' bodies (proving the health-check-driven exclusion has
//     propagated to the per-worker LB host set, the
//     reference_health_check_propagation_warmup precedent); c_pt_a uses NO
//     exclusion (panic mode serves ALL hosts by design, so "wait for the
//     degraded hosts to stop appearing" would never converge there — the
//     warmup is a fixed settling burst; the membership_healthy poll and delta
//     assertion tolerance provide the actual synchronization).
//  4. drive loadPerCluster (200) requests to each of /a, /b, /c, tallying
//     per (cluster, host) via classifyBody.
//  5. scrapeStats both admin endpoints and assert the three arms' outcomes.
//
// Cross-references: phase 54 SPEC §10 (AMEND-PT1, the integer-truncation
// finding); 0096-lb-priority (the toggleResponder / pollMembershipHealthy /
// warmupUntilStable / scrapeStats / assertEq harness, reused directly);
// reference_health_check_propagation_warmup;
// reference_docker_probe_bridge_network (host.docker.internal addressing);
// reference_round_robin_offset_randomized (offset-invariant all-hosts-served
// assertions, never host identity/sequence);
// reference_admin_interface_wire_name_collision (per-name stat accessors,
// hence three DISTINCT cluster names);
// reference_fixture_workload_constant_desync (every workload count is a
// named, TestConstants-guarded constant);
// reference_percent_threshold_integer_truncation (the sibling memory note
// this fixture cross-side-proves).
package driver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0097-lb-panic-threshold"

	refContainerListenerPort = 19172
	refAdminPort             = 9901

	// backendCount is the number of runner-spawned HTTPEcho backends: ONE
	// throwaway backend no cluster references (BackendCount() must return
	// >= 1 per runner_test.go:221; all 15 real hosts in this fixture are
	// driver-owned toggle responders, below).
	backendCount = 1

	clusterCount       = 3 // c_pt_a (thr 80) / c_pt_b (absent/50) / c_pt_c (thr 60.9)
	hostsPerCluster    = 5
	degradedPerCluster = 2                                    // -> 3/5 = 60% healthy per cluster
	healthyPerCluster  = hostsPerCluster - degradedPerCluster // 3
	loadPerCluster     = 200                                  // requests driven per cluster path (offset-invariant / count assertions)

	convergeDeadline = 30 * time.Second
	convergePoll     = 200 * time.Millisecond
	warmupStable     = 60
	warmupDeadline   = 15 * time.Second
)

// clusterNames is the fixed, ordered set of the three cluster names — DISTINCT
// per reference_admin_interface_wire_name_collision (per-name stat accessors
// disambiguate; no shared/collapsed wire name across clusters).
var clusterNames = [clusterCount]string{"c_pt_a", "c_pt_b", "c_pt_c"}

// degradedIdx is the FIXED set of host indices degraded in EVERY cluster
// (the SAME 2 of 5, so the "degraded host tally==0" assertions can name them
// directly — reference_fixture_workload_constant_desync).
var degradedIdx = [degradedPerCluster]int{3, 4}

func init() {
	fixture.RegisterFixture(fixtureName, &ptDriver{})
}

// toggleResponder is a driver-owned, self-managed HTTP/1.1 responder for ONE
// (cluster, host) pair: 200 "<cluster>:<idx>" on any data path; on /healthz,
// 200 while healthy.Load()==true, 503 once SetHealthy(false) has been called.
// Extends the 0096-lb-priority toggleResponder (which carries only an idx)
// with a cluster label — this fixture has THREE clusters of 5 hosts each,
// not one/two tiers, so the response body must identify BOTH dimensions for
// classifyBody to attribute load-phase tallies correctly.
type toggleResponder struct {
	cluster string
	idx     int
	ln      net.Listener
	srv     *http.Server
	healthy atomic.Bool
}

func newToggleResponder(cluster string, idx int) (*toggleResponder, error) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("%s: toggle responder[%s:%d]: listen: %w", fixtureName, cluster, idx, err)
	}
	r := &toggleResponder{cluster: cluster, idx: idx, ln: ln}
	r.healthy.Store(true)
	r.srv = &http.Server{Handler: http.HandlerFunc(r.handle)}
	go func() { _ = r.srv.Serve(ln) }() // best-effort; process-lifetime fixture, no explicit teardown
	return r, nil
}

func (r *toggleResponder) handle(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/healthz" {
		if r.healthy.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "%s:%d", r.cluster, r.idx)
}

func (r *toggleResponder) port() int { return r.ln.Addr().(*net.TCPAddr).Port }

// SetHealthy flips the /healthz response (the controlled-degradation trigger).
func (r *toggleResponder) SetHealthy(v bool) { r.healthy.Store(v) }

// ptDriver is STATEFUL: it owns the 15 driver-owned toggleResponders (built
// once, memoized) and stashes the per-side listener addrs from the Drive
// hooks so AssertStats — the only hook holding both admin addrs — can run
// all three arms.
type ptDriver struct {
	mu           sync.Mutex
	refListener  string
	subjListener string
	// hosts[cluster][idx] — 3 clusters x 5 hosts, indexed by clusterNames position.
	hosts [clusterCount][hostsPerCluster]*toggleResponder
}

func (*ptDriver) BackendCount() int                { return backendCount }
func (*ptDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (*ptDriver) SubjectListenerName() string      { return "l_http" }
func (*ptDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// ensureBackends builds the 15 toggle responders exactly once (memoized —
// both ReferenceBootstrap and SubjectConfig call it and MUST agree on the
// SAME 15 ports). Resets every responder to healthy HERE (before the
// bootstraps are rendered and the containers boot), NOT in AssertStats — the
// 0095/0096 ensureTier0 precedent: a reset placed in AssertStats arrives too
// late for Envoy's active health checker, which locks an unhealthy-on-
// first-probe host onto a slow no-traffic cadence until cluster traffic
// flows once.
func (d *ptDriver) ensureBackends() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hosts[0][0] != nil {
		for _, cluster := range d.hosts {
			for _, r := range cluster {
				r.SetHealthy(true)
			}
		}
		return
	}
	for ci, cname := range clusterNames {
		for hi := 0; hi < hostsPerCluster; hi++ {
			r, err := newToggleResponder(cname, hi)
			if err != nil {
				panic(err)
			}
			d.hosts[ci][hi] = r
		}
	}
}

// healthChecksBlock: interval/timeout 1s, thresholds 1/1 — fast convergence
// (the 0096-lb-priority healthChecksBlock precedent).
const healthChecksBlock = `      health_checks:
        - interval: 1s
          timeout: 1s
          unhealthy_threshold: 1
          healthy_threshold: 1
          no_traffic_healthy_interval: 1s
          http_health_check:
            path: /healthz`

const routeTable = `                      routes:
                        - match: { prefix: "/a" }
                          route: { cluster: c_pt_a }
                        - match: { prefix: "/b" }
                          route: { cluster: c_pt_b }
                        - match: { prefix: "/c" }
                          route: { cluster: c_pt_c }`

// panicThresholdBlock renders the per-cluster common_lb_config difference —
// the ONLY per-cluster config difference in this fixture (SPEC §10):
// c_pt_a gets { value: 80 }, c_pt_b OMITS common_lb_config entirely (the
// absent/50%-default arm), c_pt_c gets { value: 60.9 }.
func panicThresholdBlock(clusterName string) string {
	switch clusterName {
	case "c_pt_a":
		return `      common_lb_config:
        healthy_panic_threshold: { value: 80 }
`
	case "c_pt_b":
		return "" // absent common_lb_config -> the reference's 50% default
	case "c_pt_c":
		return `      common_lb_config:
        healthy_panic_threshold: { value: 60.9 }
`
	default:
		panic(fmt.Sprintf("%s: unknown cluster %q", fixtureName, clusterName))
	}
}

// endpointsBlock renders one cluster's load_assignment over its 5
// toggleResponder ports, addressed per side (addr is either
// "host.docker.internal" for the reference or "127.0.0.1" for the subject).
func endpointsBlock(clusterName, addr string, ports [hostsPerCluster]int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "      load_assignment:\n        cluster_name: %s\n        endpoints:\n          - lb_endpoints:\n", clusterName)
	for _, p := range ports {
		fmt.Fprintf(&b, "              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }\n", addr, p)
	}
	return b.String()
}

// clusterBlock renders one STATIC ROUND_ROBIN cluster with its health_checks
// block, its per-cluster panic-threshold config (or absence thereof), and its
// endpoints.
func clusterBlock(clusterName, clusterType, addr string, ports [hostsPerCluster]int) string {
	return fmt.Sprintf(`    - name: %s
      type: %s
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
%s
%s
%s`, clusterName, clusterType, healthChecksBlock, panicThresholdBlock(clusterName), endpointsBlock(clusterName, addr, ports))
}

func (d *ptDriver) portsFor() map[string][hostsPerCluster]int {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string][hostsPerCluster]int, clusterCount)
	for ci, cname := range clusterNames {
		var ports [hostsPerCluster]int
		for hi := 0; hi < hostsPerCluster; hi++ {
			ports[hi] = d.hosts[ci][hi].port()
		}
		out[cname] = ports
	}
	return out
}

func (d *ptDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.ensureBackends()
	ports := d.portsFor()
	var clusters strings.Builder
	for _, cname := range clusterNames {
		clusters.WriteString(clusterBlock(cname, "STRICT_DNS", "host.docker.internal", ports[cname]))
		clusters.WriteString("\n")
	}
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
%s
`, refAdminPort, refContainerListenerPort, routeTable, clusters.String())
}

func (d *ptDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.ensureBackends()
	ports := d.portsFor()
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	var clusters strings.Builder
	for _, cname := range clusterNames {
		clusters.WriteString(clusterBlock(cname, "STATIC", "127.0.0.1", ports[cname]))
		clusters.WriteString("\n")
	}
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0097, cluster: envoy-go-differential }
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
%s
`, subjAdminPort, subjListenerPort, routeTable, clusters.String())
}

func (d *ptDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

func (d *ptDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

func (*ptDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// classifyBody parses a toggleResponder body of the form "<cluster>:<idx>"
// (e.g. "c_pt_a:3") into its cluster name and host index.
func classifyBody(body []byte) (cluster string, host int, err error) {
	s := string(body)
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return "", 0, fmt.Errorf("body %q missing ':' separator", s)
	}
	c := s[:idx]
	hStr := s[idx+1:]
	found := false
	for _, cn := range clusterNames {
		if cn == c {
			found = true
			break
		}
	}
	if !found {
		return "", 0, fmt.Errorf("body %q: unrecognized cluster %q", s, c)
	}
	h, err := strconv.Atoi(hStr)
	if err != nil {
		return "", 0, fmt.Errorf("body %q: bad host index: %w", s, err)
	}
	return c, h, nil
}

func pollMembershipHealthy(side, adminAddr, cluster string, want int) error {
	deadline := time.Now().Add(convergeDeadline)
	key := "cluster." + cluster + ".membership_healthy"
	var last int64 = -1
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			if v, ok := st[key]; ok {
				last = int64(v)
				if v == uint64(want) {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: %s did not converge to %d within %s (last seen %d)", side, key, want, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

// loadAndTally drives n GET requests against addr's path and tallies each
// response body's (cluster, host) attribution via classifyBody. Only
// responses attributed to wantCluster are tallied into the per-host slice;
// any response classifying as a DIFFERENT cluster is a hard error (this
// fixture's three route prefixes are mutually exclusive by construction).
func loadAndTally(ctx context.Context, side, addr, path, wantCluster string, n int) ([hostsPerCluster]int, error) {
	var tally [hostsPerCluster]int
	for i := 0; i < n; i++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", path, nil, nil)
		if err != nil {
			return tally, fmt.Errorf("%s: GET %s[%d]: %w", side, path, i, err)
		}
		if resp.StatusCode != http.StatusOK {
			return tally, fmt.Errorf("%s: GET %s[%d]: status %d, want 200", side, path, i, resp.StatusCode)
		}
		cluster, host, err := classifyBody(body)
		if err != nil {
			return tally, fmt.Errorf("%s: GET %s[%d]: %w", side, path, i, err)
		}
		if cluster != wantCluster {
			return tally, fmt.Errorf("%s: GET %s[%d]: body attributed to cluster %q, want %q", side, path, i, cluster, wantCluster)
		}
		if host < 0 || host >= hostsPerCluster {
			return tally, fmt.Errorf("%s: GET %s[%d]: host index %d out of range", side, path, i, host)
		}
		tally[host]++
	}
	return tally, nil
}

// degradedBodies returns the response bodies of cluster's degraded hosts
// (the excludeBodies set for warmupUntilStable's no-panic-arm convergence
// gate — c_pt_b/c_pt_c).
func degradedBodies(cluster string) map[string]bool {
	out := make(map[string]bool, degradedPerCluster)
	for _, idx := range degradedIdx {
		out[fmt.Sprintf("%s:%d", cluster, idx)] = true
	}
	return out
}

// warmupUntilStable polls path until it observes warmupStable CONSECUTIVE
// responses that are neither an error/non-200 NOR one of excludeBodies (the
// 0095/0096 warmupUntilStable precedent — reference_health_check_propagation
// _warmup). A NIL/empty excludeBodies set (c_pt_a's panic arm, which serves
// ALL hosts including the degraded ones by design) degenerates to a plain
// "K consecutive 200s" settling gate.
func warmupUntilStable(ctx context.Context, side, addr, path string, excludeBodies map[string]bool) error {
	deadline := time.Now().Add(warmupDeadline)
	consecutive := 0
	for {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", path, nil, nil)
		ok := err == nil && resp.StatusCode == http.StatusOK && !excludeBodies[string(body)]
		if ok {
			consecutive++
			if consecutive >= warmupStable {
				return nil
			}
		} else {
			consecutive = 0
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: %s did not stabilize to %d consecutive non-degraded 200s within %s", side, path, warmupStable, warmupDeadline)
		}
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

// scrapeStats issues GET http://<addr>/stats and parses "name: value" lines
// into a map[name]uint64 (the 0066/0095/0096 scrapeStats, verbatim).
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
			continue
		}
		out[name] = v
	}
	return out, nil
}

// degradeAll toggles indices in degradedIdx to unhealthy in EVERY cluster.
func (d *ptDriver) degradeAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for ci := range clusterNames {
		for _, idx := range degradedIdx {
			d.hosts[ci][idx].SetHealthy(false)
		}
	}
}

func (d *ptDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ctx := context.Background()

	d.ensureBackends()
	d.mu.Lock()
	refListener := d.refListener
	subjListener := d.subjListener
	d.mu.Unlock()
	if refListener == "" || subjListener == "" {
		t.Fatalf("listener addrs not stashed (ref=%q subj=%q)", refListener, subjListener)
	}

	// 1. Degrade the fixed 2-of-5 hosts in EVERY cluster -> 60% healthy.
	d.degradeAll()

	// 2. Poll membership_healthy == healthyPerCluster (3) for each cluster,
	// both sides.
	for _, cname := range clusterNames {
		if err := pollMembershipHealthy("reference", refAdminAddr, cname, healthyPerCluster); err != nil {
			t.Fatalf("converge: %v", err)
		}
		if err := pollMembershipHealthy("subject", subjAdminAddr, cname, healthyPerCluster); err != nil {
			t.Fatalf("converge: %v", err)
		}
	}

	// 3. Warm up each cluster's path. c_pt_b/c_pt_c (no-panic arms) exclude
	// the degraded hosts' bodies -- proof the exclusion has propagated to
	// the LB host set. c_pt_a (the panic arm) uses no exclusion: panic mode
	// serves ALL hosts by design, so "wait for degraded hosts to stop
	// appearing" would never converge there.
	paths := map[string]string{"c_pt_a": "/a", "c_pt_b": "/b", "c_pt_c": "/c"}
	excludes := map[string]map[string]bool{
		"c_pt_a": {},
		"c_pt_b": degradedBodies("c_pt_b"),
		"c_pt_c": degradedBodies("c_pt_c"),
	}
	for _, cname := range clusterNames {
		if err := warmupUntilStable(ctx, "reference", refListener, paths[cname], excludes[cname]); err != nil {
			t.Fatalf("warmup: %v", err)
		}
		if err := warmupUntilStable(ctx, "subject", subjListener, paths[cname], excludes[cname]); err != nil {
			t.Fatalf("warmup: %v", err)
		}
	}

	// 3.5. Snapshot cluster.c_pt_a.lb_healthy_panic BEFORE driving the
	// tallied load. c_pt_a's warmup (step 3, above) has NO excludeBodies
	// signal to converge on -- panic mode serves every host identically
	// whether the per-worker health state has truly caught up to "panicking"
	// or simply hasn't yet propagated the newly-unhealthy hosts at all (both
	// states look IDENTICAL on the wire: every host answers). So warmup's own
	// requests may ALSO increment lb_healthy_panic once panic genuinely
	// engages, by an amount that is not deterministically knowable in
	// advance (it depends on exactly when per-worker convergence completes
	// relative to warmup's request stream). The exact-count assertion below
	// therefore uses a DELTA (post-load minus this baseline), not an
	// absolute value -- mirroring reference_delta_sink_differential_
	// stability_barrier's "measure the delta the phase itself causes"
	// discipline. c_pt_b/c_pt_c need no such snapshot: their counter never
	// increments outside genuine panic, which they never enter, so it stays
	// unambiguously 0 through warmup AND load.
	refPanicBefore, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats (panic baseline): %v", err)
	}
	subjPanicBefore, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats (panic baseline): %v", err)
	}
	refPanicBase := refPanicBefore["cluster.c_pt_a.lb_healthy_panic"]
	subjPanicBase := subjPanicBefore["cluster.c_pt_a.lb_healthy_panic"]

	// 4. Drive loadPerCluster requests to each of /a, /b, /c and tally.
	type tallies struct{ ref, subj [hostsPerCluster]int }
	tallyByCluster := make(map[string]tallies, clusterCount)
	for _, cname := range clusterNames {
		refTally, err := loadAndTally(ctx, "reference", refListener, paths[cname], cname, loadPerCluster)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		subjTally, err := loadAndTally(ctx, "subject", subjListener, paths[cname], cname, loadPerCluster)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		tallyByCluster[cname] = tallies{ref: refTally, subj: subjTally}
	}

	// 5. Scrape /stats both sides.
	ref, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}

	// Decode-ran guard first (reference_docker_probe_bridge_network).
	if ref["cluster.c_pt_a.upstream_rq_total"] == 0 {
		t.Fatalf("reference did NOT decode: cluster.c_pt_a.upstream_rq_total == 0")
	}

	for _, sd := range []struct {
		side     string
		st       map[string]uint64
		t        [hostsPerCluster]int
		baseline uint64
	}{
		{"reference", ref, tallyByCluster["c_pt_a"].ref, refPanicBase},
		{"subject", subj, tallyByCluster["c_pt_a"].subj, subjPanicBase},
	} {
		// c_pt_a PANICS (60% < 80%): every one of the 5 hosts (incl the 2
		// degraded) must have been served -- offset-invariant, NEVER host
		// identity/sequence (reference_round_robin_offset_randomized).
		for h := 0; h < hostsPerCluster; h++ {
			if sd.t[h] == 0 {
				t.Errorf("%s c_pt_a host %d got 0 -- panic must serve ALL hosts incl unhealthy (offset-invariant)", sd.side, h)
			}
		}
		got, ok := sd.st["cluster.c_pt_a.lb_healthy_panic"]
		if !ok {
			t.Errorf("%s: cluster.c_pt_a.lb_healthy_panic ABSENT in /stats", sd.side)
			continue
		}
		if delta := got - sd.baseline; delta != uint64(loadPerCluster) {
			t.Errorf("%s: cluster.c_pt_a.lb_healthy_panic delta (post-load %d - baseline %d) = %d, want %d", sd.side, got, sd.baseline, delta, loadPerCluster)
		}
	}

	for _, sd := range []struct {
		side string
		st   map[string]uint64
		t    [hostsPerCluster]int
	}{
		{"reference", ref, tallyByCluster["c_pt_b"].ref},
		{"subject", subj, tallyByCluster["c_pt_b"].subj},
	} {
		// c_pt_b: NO panic (60% >= 50%, absent-default arm) -- the 2
		// degraded hosts must have tally == 0.
		for _, idx := range degradedIdx {
			if sd.t[idx] != 0 {
				t.Errorf("%s c_pt_b host %d (degraded) got %d, want 0 -- no panic must exclude unhealthy hosts", sd.side, idx, sd.t[idx])
			}
		}
		assertEq(t, sd.side, sd.st, "cluster.c_pt_b.lb_healthy_panic", 0)
	}

	for _, sd := range []struct {
		side string
		st   map[string]uint64
		t    [hostsPerCluster]int
	}{
		{"reference", ref, tallyByCluster["c_pt_c"].ref},
		{"subject", subj, tallyByCluster["c_pt_c"].subj},
	} {
		// c_pt_c: NO panic (floor(60.9)=60; 60% < 60% is FALSE -- the
		// integer-truncation discriminator) -- the 2 degraded hosts must
		// have tally == 0.
		for _, idx := range degradedIdx {
			if sd.t[idx] != 0 {
				t.Errorf("%s c_pt_c host %d (degraded) got %d, want 0 -- no panic must exclude unhealthy hosts", sd.side, idx, sd.t[idx])
			}
		}
		assertEq(t, sd.side, sd.st, "cluster.c_pt_c.lb_healthy_panic", 0)
	}

	for _, sd := range []struct {
		side string
		st   map[string]uint64
	}{{"reference", ref}, {"subject", subj}} {
		assertEq(t, sd.side, sd.st, "cluster.c_pt_a.membership_healthy", uint64(healthyPerCluster))
		assertEq(t, sd.side, sd.st, "cluster.c_pt_b.membership_healthy", uint64(healthyPerCluster))
		assertEq(t, sd.side, sd.st, "cluster.c_pt_c.membership_healthy", uint64(healthyPerCluster))
	}
}

var (
	_ fixture.Driver        = (*ptDriver)(nil)
	_ fixture.StatsAsserter = (*ptDriver)(nil)
)
