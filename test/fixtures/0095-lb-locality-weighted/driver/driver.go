// Package driver registers the 0095-lb-locality-weighted cross-side
// differential fixture (phase 52 SPEC §8.1 / PLAN Tasks 7-9).
//
// Cross-side [http_connection_manager + router] fixture over ONE cluster
// c_lw (common_lb_config.locality_weighted_lb_config: {}) with TWO
// LocalityLbEndpoints groups — region "a" (load_balancing_weight: 2, 5
// hosts) and region "b" (load_balancing_weight: 1, 5 hosts) — plus an
// active HTTP health checker (path /healthz, fast convergence).
//
// Region B's 5 hosts are runner-spawned HTTPEcho backends (BackendCount()
// ==5, always healthy). Region A's 5 hosts are DRIVER-OWNED toggleable HTTP
// responders (self-managed net.Listeners, NOT part of the runner's backend
// pool — the 0066 "dead port" precedent generalized to a LIVE, TOGGLEABLE
// host): each answers 200 "region-a:<idx>" on any data path and 200/503 on
// /healthz depending on an atomic healthy flag the driver flips mid-run.
//
// AssertStats drives BOTH arms in-band (the only hook holding both admin
// addrs):
//
//	arm (a) — static ratio (all 10 hosts healthy): poll membership_healthy
//	  ==10 on both sides, warmup, send staticLoadCount requests, assert
//	  region A's share is within a ~5σ band of the confirmed 100%-healthy
//	  formula prediction (66.7%/33.3% — SPEC §11.3).
//	arm (b) — health-degradation shift: toggle 3 of region A's 5
//	  driver-owned hosts to FAIL /healthz, poll membership_healthy==7 on
//	  both sides, re-warmup, send degradedLoadCount MORE requests, assert
//	  the region share matches the confirmed 40%-healthy prediction
//	  (52.8%/47.2%).
//
// Cross-references: phase 52 SPEC §8.1/§11.3; 0066-health-check-http (the
// poll-to-converge + warmup pattern, reused verbatim);
// reference_health_check_propagation_warmup; reference_docker_probe_bridge_
// network (host.docker.internal addressing for BOTH the runner-spawned
// region-B backends AND the driver-owned region-A toggle responders);
// reference_differential_band_sigma_margin (~5σ bands);
// reference_differential_run_selector (-run 'TestDifferential/0095');
// reference_fixture_workload_constant_desync;
// reference_differential_asserter_dispatch (StatsAsserter, cross-side).
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
	fixtureName = "0095-lb-locality-weighted"

	refContainerListenerPort = 19170
	refAdminPort             = 9901

	// backendCount is the number of runner-spawned HTTPEcho backends — region
	// B's 5 ALWAYS-healthy hosts. Region A's 5 hosts are driver-owned (below).
	backendCount = 5

	regionAHosts   = 5
	regionBHosts   = 5
	degradedAHosts = 3 // toggled to failing in arm (b): 2/5 = 40% healthy in region A

	weightA, weightB = 2, 1

	// staticLoadCount / degradedLoadCount are the per-arm request counts —
	// the SPEC §11.3 live-probe count (900), reused for continuity with the
	// confirmed data points (reference_fixture_workload_constant_desync).
	staticLoadCount   = 900
	degradedLoadCount = 900

	// The confirmed AMEND-LW3 predictions (SPEC §11.3) + the ~5σ band
	// half-width at n=900 (reference_differential_band_sigma_margin): std ≈
	// sqrt(n·p·(1-p)); the band below is ~5σ as a percentage-point margin,
	// computed once here (not re-derived per assertion) for auditability.
	staticShareA    = 0.667 // 100%-healthy: 2/(2+1)
	staticBandPct   = 8.0   // 5σ at n=900,p=0.667 ≈ 7.85pp
	degradedShareA  = 0.528 // 40%-healthy region A (SPEC §11.3 EXACT match)
	degradedBandPct = 8.5   // 5σ at n=900,p=0.528 ≈ 8.3pp

	membershipTotal = regionAHosts + regionBHosts // 10, unaffected by health

	convergeDeadline = 30 * time.Second
	convergePoll     = 200 * time.Millisecond
	warmupStable     = 10
	warmupDeadline   = 15 * time.Second
)

func init() {
	fixture.RegisterFixture(fixtureName, &lwDriver{})
}

// toggleResponder is a driver-owned, self-managed HTTP/1.1 responder for ONE
// region-A host: 200 "region-a:<idx>" on any data path; on /healthz, 200
// while healthy.Load()==true, 503 once SetHealthy(false) has been called
// (arm (b)'s controlled-degradation trigger). Unlike the runner's HTTPEcho
// pool (fixed behavior, spawned/owned by the runner), this is spun up by the
// driver itself — the 0066 allocDeadPort precedent generalized from
// "permanently closed" to "live and toggleable".
type toggleResponder struct {
	idx     int
	ln      net.Listener
	srv     *http.Server
	healthy atomic.Bool
}

func newToggleResponder(idx int) (*toggleResponder, error) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("%s: toggle responder[%d]: listen: %w", fixtureName, idx, err)
	}
	r := &toggleResponder{idx: idx, ln: ln}
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
	_, _ = fmt.Fprintf(w, "region-a:%d", r.idx)
}

func (r *toggleResponder) port() int { return r.ln.Addr().(*net.TCPAddr).Port }

// SetHealthy flips the /healthz response (arm (b)'s controlled-failure trigger).
func (r *toggleResponder) SetHealthy(v bool) { r.healthy.Store(v) }

// lwDriver is STATEFUL: it owns the 5 region-A toggleResponders (built once,
// memoized) and stashes the per-side listener addrs from the Drive hooks so
// AssertStats — the only hook holding both admin addrs — can run both arms.
type lwDriver struct {
	mu           sync.Mutex
	refListener  string
	subjListener string
	regionA      []*toggleResponder
}

func (*lwDriver) BackendCount() int                { return backendCount } // region B only
func (*lwDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (*lwDriver) SubjectListenerName() string      { return "l_http" }
func (*lwDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// ensureRegionA builds the 5 region-A toggle responders exactly once
// (memoized — both ReferenceBootstrap and SubjectConfig call it and MUST
// agree on the SAME 5 ports).
func (d *lwDriver) ensureRegionA() []*toggleResponder {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.regionA != nil {
		// d.regionA is a process-lifetime singleton — under `-count=N` (or any
		// repeated TestDifferential invocation in the same process) a PRIOR
		// run's arm (b) SetHealthy(false) calls would otherwise leak into this
		// run. Reset to healthy HERE (before ReferenceBootstrap/SubjectConfig
		// return their YAML and the containers boot), NOT in AssertStats:
		// Envoy's active health checker locks an unhealthy-on-first-probe host
		// onto the `no_traffic_interval` cadence (default 60s, unset in this
		// fixture's healthChecksBlock — only no_traffic_healthy_interval is
		// pinned to 0.2s, which applies to ALREADY-healthy hosts only) until
		// cluster traffic flows once. A reset placed in AssertStats (which
		// runs AFTER the container is already up and has already issued its
		// first, stale-unhealthy probe) arrives too late — the host is stuck
		// on the 60s cadence and never re-converges within the 30s poll
		// deadline. Bit the Task 9 ≥20-run flake check: run 1 passed, every
		// subsequent run failed arm(a)'s membership_healthy==10 convergence,
		// stuck at 7 — see README.md's Task 9 section.
		for _, r := range d.regionA {
			r.SetHealthy(true)
		}
		return d.regionA
	}
	out := make([]*toggleResponder, regionAHosts)
	for i := range out {
		r, err := newToggleResponder(i)
		if err != nil {
			panic(err)
		}
		out[i] = r
	}
	d.regionA = out
	return out
}

// healthChecksBlock: no_traffic_healthy_interval is set EXPLICITLY (not left
// to default to no_traffic_interval's 60s — the go-control-plane HealthCheck
// proto doc: "If no_traffic_healthy_interval is not set, it will default to
// the no traffic interval [60s] and send that interval regardless of health
// state"). Every one of this fixture's 10 hosts is HEALTHY from boot (unlike
// 0066's permanently-dead port), so the very first health-check round
// immediately reschedules each host onto that 60s cadence BEFORE any cluster
// traffic has flowed; per the same doc, an already-scheduled no-traffic-
// healthy timer does not get rescheduled early just because traffic starts —
// it only downshifts to the standard "interval" the NEXT time it fires. That
// made arm (b)'s post-toggle convergence poll (30s deadline) observe ZERO
// further health-check attempts after the initial round (confirmed via a
// temporary attempt/success/failure counter dump: attempt=10 success=10
// failure=0, unchanged for the full 30s). Pinning
// no_traffic_healthy_interval to the same 0.2s keeps the fast cadence
// regardless of traffic-routing state.
const healthChecksBlock = `      health_checks:
        - interval: 0.2s
          timeout: 0.2s
          unhealthy_threshold: 1
          healthy_threshold: 1
          no_traffic_healthy_interval: 0.2s
          http_health_check:
            path: /healthz`

const commonLbConfigBlock = `      common_lb_config:
        locality_weighted_lb_config: {}`

const routeTable = `                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_lw }`

// localityEndpointsBlock renders the two LocalityLbEndpoints groups for the
// given host addressing scheme over the SAME 10 ports (5 region-A
// toggleResponder ports + 5 region-B runner backend ports).
func localityEndpointsBlock(addr string, aPorts, bPorts []int) string {
	var b strings.Builder
	b.WriteString("      load_assignment:\n        cluster_name: c_lw\n        endpoints:\n")
	fmt.Fprintf(&b, "          - locality: { region: a }\n            load_balancing_weight: %d\n            lb_endpoints:\n", weightA)
	for _, p := range aPorts {
		fmt.Fprintf(&b, "              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }\n", addr, p)
	}
	fmt.Fprintf(&b, "          - locality: { region: b }\n            load_balancing_weight: %d\n            lb_endpoints:\n", weightB)
	for _, p := range bPorts {
		fmt.Fprintf(&b, "              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }\n", addr, p)
	}
	return b.String()
}

func (d *lwDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	regionA := d.ensureRegionA()
	aPorts := make([]int, regionAHosts)
	for i, r := range regionA {
		aPorts[i] = r.port()
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
    - name: c_lw
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
%s
%s
%s
`, refAdminPort, refContainerListenerPort, routeTable, healthChecksBlock, commonLbConfigBlock,
		localityEndpointsBlock("host.docker.internal", aPorts, backendPorts))
}

func (d *lwDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	regionA := d.ensureRegionA()
	aPorts := make([]int, regionAHosts)
	for i, r := range regionA {
		aPorts[i] = r.port()
	}
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0095, cluster: envoy-go-differential }
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
    - name: c_lw
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s
%s
%s
`, subjAdminPort, subjListenerPort, routeTable, healthChecksBlock, commonLbConfigBlock,
		localityEndpointsBlock("127.0.0.1", aPorts, backendPorts))
}

func (d *lwDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

func (d *lwDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

func (*lwDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// classifyBody attributes a load response to region "a" ("region-a:<idx>",
// the driver-owned toggleResponders) or region "b" ("backend-<idx>:...", the
// runner-spawned HTTPEcho pool).
func classifyBody(body []byte) (region string, err error) {
	s := string(body)
	switch {
	case strings.HasPrefix(s, "region-a:"):
		return "a", nil
	case strings.HasPrefix(s, "backend-"):
		return "b", nil
	default:
		return "", fmt.Errorf("body %q matches neither region-a: nor backend- prefix", s)
	}
}

func pollMembershipHealthy(side, adminAddr string, want int) error {
	deadline := time.Now().Add(convergeDeadline)
	var last int64 = -1
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			if v, ok := st["cluster.c_lw.membership_healthy"]; ok {
				last = int64(v)
				if v == uint64(want) {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: cluster.c_lw.membership_healthy did not converge to %d within %s (last seen %d)", side, want, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

type regionTally struct{ a, b int }

func loadAndTally(ctx context.Context, side, addr string, n int) (regionTally, error) {
	var t regionTally
	for i := 0; i < n; i++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err != nil {
			return t, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if resp.StatusCode != http.StatusOK {
			return t, fmt.Errorf("%s: GET /[%d]: status %d, want 200", side, i, resp.StatusCode)
		}
		region, err := classifyBody(body)
		if err != nil {
			return t, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if region == "a" {
			t.a++
		} else {
			t.b++
		}
	}
	return t, nil
}

// warmupUntilStable polls the data path until it observes warmupStable
// CONSECUTIVE responses that are neither an error/non-200 NOR one of
// excludeBodies (SPEC §8.1's "K=10-consecutive-non-degraded-host warmup",
// reference_health_check_propagation_warmup).
//
// A plain "K consecutive 200s" gate (the 0066/39.1 template this was
// originally copied from) is a NO-OP here: this fixture's toggleResponder
// (by design, per the Task-7 package doc) answers 200 "region-a:<idx>" on
// the data path REGARDLESS of its /healthz health state — only /healthz
// itself flips 200/503. So even a host the active health checker has
// already marked unhealthy keeps satisfying a bare "is it 200" gate, and a
// plain-200 warmup converges instantly whether or not the per-worker-thread
// LB host set has actually caught up to the health-check-driven exclusion
// (the gauge-vs-host-set propagation gap this same memory note describes).
// Passing the exact response bodies of the to-be-excluded (degraded) hosts
// gives the gate a real signal: once the worker threads' host sets have
// updated, requests deterministically stop landing on those hosts, so K
// consecutive non-degraded hits is genuine proof of convergence. Confirmed
// by a temporary per-host-hit debug dump during this task's live run: WITHOUT
// this host-identity check, the degraded hosts (region-a:0/1/2) kept
// receiving ~1/5 of region-A traffic each throughout the entire load phase,
// unchanged from the fully-healthy arm — i.e. NO shift at all, only fixed
// after adding this check.
func warmupUntilStable(ctx context.Context, side, addr string, excludeBodies map[string]bool) error {
	deadline := time.Now().Add(warmupDeadline)
	consecutive := 0
	for {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
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
			return fmt.Errorf("%s: data path did not stabilize to %d consecutive non-degraded 200s within %s", side, warmupStable, warmupDeadline)
		}
	}
}

// assertShareInBand asserts region A's percentage share of (t.a+t.b) falls
// within ±bandPct percentage points of wantShare (reference_differential_
// band_sigma_margin — a PER-SIDE statistical band, NOT cross-side
// per-request identity: the independent per-request RNG makes cross-side
// identity infeasible for a randomized policy, the 0060/0065 lineage).
func assertShareInBand(t fixture.TB, side string, tally regionTally, wantShare, bandPct float64) {
	t.Helper()
	total := tally.a + tally.b
	if total == 0 {
		t.Errorf("%s: zero total requests tallied", side)
		return
	}
	gotSharePct := 100 * float64(tally.a) / float64(total)
	wantSharePct := 100 * wantShare
	if gotSharePct < wantSharePct-bandPct || gotSharePct > wantSharePct+bandPct {
		t.Errorf("%s: region A share = %.2f%% (a=%d b=%d), want %.1f%% ± %.1fpp", side, gotSharePct, tally.a, tally.b, wantSharePct, bandPct)
	}
}

func (d *lwDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ctx := context.Background()

	d.mu.Lock()
	refListener := d.refListener
	subjListener := d.subjListener
	regionA := d.regionA
	d.mu.Unlock()
	if refListener == "" || subjListener == "" {
		t.Fatalf("listener addrs not stashed (ref=%q subj=%q)", refListener, subjListener)
	}

	// --- arm (a): static ratio, all 10 hosts healthy ---
	if err := pollMembershipHealthy("reference", refAdminAddr, membershipTotal); err != nil {
		t.Fatalf("arm(a) converge: %v", err)
	}
	if err := pollMembershipHealthy("subject", subjAdminAddr, membershipTotal); err != nil {
		t.Fatalf("arm(a) converge: %v", err)
	}
	if err := warmupUntilStable(ctx, "reference", refListener, nil); err != nil {
		t.Fatalf("arm(a) warmup: %v", err)
	}
	if err := warmupUntilStable(ctx, "subject", subjListener, nil); err != nil {
		t.Fatalf("arm(a) warmup: %v", err)
	}
	refStaticTally, err := loadAndTally(ctx, "reference", refListener, staticLoadCount)
	if err != nil {
		t.Fatalf("arm(a) load: %v", err)
	}
	subjStaticTally, err := loadAndTally(ctx, "subject", subjListener, staticLoadCount)
	if err != nil {
		t.Fatalf("arm(a) load: %v", err)
	}
	assertShareInBand(t, "reference/static", refStaticTally, staticShareA, staticBandPct)
	assertShareInBand(t, "subject/static", subjStaticTally, staticShareA, staticBandPct)

	// --- arm (b): degrade 3 of region A's 5 hosts, re-measure the SHIFT ---
	degradedBodies := make(map[string]bool, degradedAHosts)
	for i := 0; i < degradedAHosts; i++ {
		regionA[i].SetHealthy(false)
		degradedBodies[fmt.Sprintf("region-a:%d", i)] = true
	}
	if err := pollMembershipHealthy("reference", refAdminAddr, membershipTotal-degradedAHosts); err != nil {
		t.Fatalf("arm(b) converge: %v", err)
	}
	if err := pollMembershipHealthy("subject", subjAdminAddr, membershipTotal-degradedAHosts); err != nil {
		t.Fatalf("arm(b) converge: %v", err)
	}
	if err := warmupUntilStable(ctx, "reference", refListener, degradedBodies); err != nil {
		t.Fatalf("arm(b) warmup: %v", err)
	}
	if err := warmupUntilStable(ctx, "subject", subjListener, degradedBodies); err != nil {
		t.Fatalf("arm(b) warmup: %v", err)
	}
	refDegradedTally, err := loadAndTally(ctx, "reference", refListener, degradedLoadCount)
	if err != nil {
		t.Fatalf("arm(b) load: %v", err)
	}
	subjDegradedTally, err := loadAndTally(ctx, "subject", subjListener, degradedLoadCount)
	if err != nil {
		t.Fatalf("arm(b) load: %v", err)
	}
	assertShareInBand(t, "reference/degraded", refDegradedTally, degradedShareA, degradedBandPct)
	assertShareInBand(t, "subject/degraded", subjDegradedTally, degradedShareA, degradedBandPct)

	// --- cross-side deterministic stats ---
	ref, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}
	if ref["cluster.c_lw.upstream_rq_total"] == 0 {
		t.Fatalf("reference did NOT decode: cluster.c_lw.upstream_rq_total == 0")
	}
	for _, sd := range []struct {
		side string
		st   map[string]uint64
	}{{"reference", ref}, {"subject", subj}} {
		assertEq(t, sd.side, sd.st, "cluster.c_lw.membership_total", uint64(membershipTotal))
		assertEq(t, sd.side, sd.st, "cluster.c_lw.membership_healthy", uint64(membershipTotal-degradedAHosts))
		if got := sd.st["cluster.c_lw.upstream_rq_total"]; got < uint64(staticLoadCount+degradedLoadCount) {
			t.Errorf("%s: cluster.c_lw.upstream_rq_total = %d, want >= %d (the measured load alone; convergence/warmup traffic adds more)", sd.side, got, staticLoadCount+degradedLoadCount)
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
// into a map[name]uint64 (the 0066/0069 scrapeStats, verbatim).
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

var (
	_ fixture.Driver        = (*lwDriver)(nil)
	_ fixture.StatsAsserter = (*lwDriver)(nil)
)
