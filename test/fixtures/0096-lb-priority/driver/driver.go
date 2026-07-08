// Package driver registers the 0096-lb-priority cross-side differential
// fixture (phase 53 SPEC §8.1 / PLAN Tasks 8-10).
//
// Cross-side [http_connection_manager + router] fixture over ONE cluster
// c_pri with TWO LocalityLbEndpoints groups at distinct priority values (0
// and 1), 5 hosts each, plus an active HTTP health checker (path /healthz,
// fast convergence).
//
// Tier 1's 5 hosts are runner-spawned HTTPEcho backends (BackendCount()==5,
// always healthy). Tier 0's 5 hosts are DRIVER-OWNED toggleable HTTP
// responders (the 0095-lb-locality-weighted precedent, reused directly):
// each answers 200 "tier0:<idx>" on any data path and 200/503 on /healthz
// depending on an atomic healthy flag the driver flips mid-run.
//
// AssertStats drives BOTH arms in-band (the only hook holding both admin
// addrs):
//
//	arm (a) — static (all 10 hosts healthy): poll membership_healthy==10 on
//	  both sides, warmup, send staticLoadCount requests, assert a HARD
//	  100%/0% split — ALL traffic on tier 0, NONE on tier 1 (capacitySum =
//	  100+100 = 200 >= 100, no bypass; SPEC §8.1/§11.1 scenario (a)).
//	arm (b) — full failover: fail ALL 5 of tier 0's hosts' /healthz, poll
//	  membership_healthy==5 on both sides, re-warmup, send
//	  degradedLoadCount MORE requests, assert the split flips to a HARD
//	  0%/100% — capacitySum = 0+100 = EXACTLY 100, the confirmed boundary
//	  that does NOT trigger the AMEND-P1 capacity-shortfall bypass (SPEC
//	  §11.1 scenario (f)).
//
// Cross-references: phase 53 SPEC §8.1/§11.1/§11.4;
// 0095-lb-locality-weighted (the toggleResponder + poll/warmup harness,
// reused verbatim); reference_health_check_propagation_warmup;
// reference_docker_probe_bridge_network (host.docker.internal addressing);
// reference_differential_run_selector (-run 'TestDifferential/0096');
// reference_fixture_workload_constant_desync;
// reference_differential_asserter_dispatch (StatsAsserter, cross-side);
// reference_differential_fixture_dispatch_constraint (both arms in ONE
// fixture dir — no separate boot-reject dir, SPEC §8.2/AMEND-P2).
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
	fixtureName = "0096-lb-priority"

	refContainerListenerPort = 19171
	refAdminPort             = 9901

	// backendCount is the number of runner-spawned HTTPEcho backends — tier
	// 1's 5 ALWAYS-healthy hosts. Tier 0's 5 hosts are driver-owned (below).
	backendCount = 5

	tier0Hosts = 5
	tier1Hosts = 5

	// staticLoadCount / degradedLoadCount are the per-arm request counts —
	// the SPEC §8.1 "≥300 requests" convention, matching the live-probe
	// scenario counts (reference_fixture_workload_constant_desync).
	staticLoadCount   = 300
	degradedLoadCount = 300

	membershipTotal = tier0Hosts + tier1Hosts // 10, unaffected by health

	convergeDeadline = 30 * time.Second
	convergePoll     = 200 * time.Millisecond
	// warmupStable: 60, not 10 — a Task 10 ≥20-run flake-check finding. Live
	// diagnostic instrumentation (per-request tally + body logging in
	// loadAndTally, added and removed during this task's investigation)
	// showed arm (a)'s rare leaks to tier 1 tightly CLUSTERED at the very
	// start of the 300-request load phase (observed indices 0, 2, 3 in one
	// captured run) — i.e., immediately AFTER warmupUntilStable's own
	// K-consecutive-non-tier1 streak had just completed. That means K=10
	// was occasionally satisfied by chance before the underlying selection
	// state had genuinely finished settling (a residual, narrower instance
	// of the same class of gap reference_health_check_propagation_warmup
	// describes — a main-thread-computed update that hasn't yet fully
	// propagated to the request-serving path — NOT health-check flapping:
	// a live diagnostic /stats scrape across many runs, including failing
	// ones, showed cluster.c_pri.health_check.failure staying at 0
	// throughout, and widening the health-check timeout/thresholds or
	// pinning a long dns_refresh_rate were each tried live and neither
	// reduced the leak rate — both reverted). Widening the SAME
	// warmupUntilStable mechanism's threshold (not inventing a new one)
	// gives the settling state much more real time+attempts to fully
	// resolve before the tallied load begins. Confirmed by 3 further
	// ≥20-run flake checks (60 more invocations, 60/60 PASS) after this
	// fix landed.
	warmupStable   = 60
	warmupDeadline = 15 * time.Second
)

func init() {
	fixture.RegisterFixture(fixtureName, &priDriver{})
}

// toggleResponder is a driver-owned, self-managed HTTP/1.1 responder for ONE
// tier-0 host: 200 "tier0:<idx>" on any data path; on /healthz, 200 while
// healthy.Load()==true, 503 once SetHealthy(false) has been called (arm
// (b)'s controlled-degradation trigger). The 0095-lb-locality-weighted
// precedent, reused directly (identical shape; only the response-body prefix
// changes: "tier0:" here vs. "region-a:" there).
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
	_, _ = fmt.Fprintf(w, "tier0:%d", r.idx)
}

func (r *toggleResponder) port() int { return r.ln.Addr().(*net.TCPAddr).Port }

// SetHealthy flips the /healthz response (arm (b)'s controlled-failure trigger).
func (r *toggleResponder) SetHealthy(v bool) { r.healthy.Store(v) }

// priDriver is STATEFUL: it owns the 5 tier-0 toggleResponders (built once,
// memoized) and stashes the per-side listener addrs from the Drive hooks so
// AssertStats — the only hook holding both admin addrs — can run both arms.
type priDriver struct {
	mu           sync.Mutex
	refListener  string
	subjListener string
	tier0        []*toggleResponder
}

func (*priDriver) BackendCount() int                { return backendCount } // tier 1 only
func (*priDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (*priDriver) SubjectListenerName() string      { return "l_http" }
func (*priDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// ensureTier0 builds the 5 tier-0 toggle responders exactly once (memoized —
// both ReferenceBootstrap and SubjectConfig call it and MUST agree on the
// SAME 5 ports).
func (d *priDriver) ensureTier0() []*toggleResponder {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tier0 != nil {
		// d.tier0 is a process-lifetime singleton — under `-count=N` (or any
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
		// deadline (the 0095-lb-locality-weighted precedent's Task 9 ≥20-run
		// flake finding, reused directly).
		for _, r := range d.tier0 {
			r.SetHealthy(true)
		}
		return d.tier0
	}
	out := make([]*toggleResponder, tier0Hosts)
	for i := range out {
		r, err := newToggleResponder(i)
		if err != nil {
			panic(err)
		}
		out[i] = r
	}
	d.tier0 = out
	return out
}

// healthChecksBlock: no_traffic_healthy_interval is set EXPLICITLY (not left
// to default to no_traffic_interval's 60s — the go-control-plane HealthCheck
// proto doc: "If no_traffic_healthy_interval is not set, it will default to
// the no traffic interval [60s] and send that interval regardless of health
// state"). Every one of this fixture's 10 hosts is HEALTHY from boot, so the
// very first health-check round immediately reschedules each host onto that
// 60s cadence BEFORE any cluster traffic has flowed; per the same doc, an
// already-scheduled no-traffic-healthy timer does not get rescheduled early
// just because traffic starts — it only downshifts to the standard
// "interval" the NEXT time it fires. That would make arm (b)'s post-toggle
// convergence poll (30s deadline) observe ZERO further health-check attempts
// after the initial round (the 0095-lb-locality-weighted precedent's exact
// finding, reused directly). Pinning no_traffic_healthy_interval to the same
// 0.2s keeps the fast cadence regardless of traffic-routing state.
const healthChecksBlock = `      health_checks:
        - interval: 0.2s
          timeout: 0.2s
          unhealthy_threshold: 1
          healthy_threshold: 1
          no_traffic_healthy_interval: 0.2s
          http_health_check:
            path: /healthz`

const routeTable = `                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_pri }`

// priorityEndpointsBlock renders the two LocalityLbEndpoints groups (distinct
// priority values 0 and 1) for the given host addressing scheme over the SAME
// 10 ports (5 tier-0 toggleResponder ports + 5 tier-1 runner backend ports).
func priorityEndpointsBlock(addr string, tier0Ports, tier1Ports []int) string {
	var b strings.Builder
	b.WriteString("      load_assignment:\n        cluster_name: c_pri\n        endpoints:\n")
	b.WriteString("          - priority: 0\n            lb_endpoints:\n")
	for _, p := range tier0Ports {
		fmt.Fprintf(&b, "              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }\n", addr, p)
	}
	b.WriteString("          - priority: 1\n            lb_endpoints:\n")
	for _, p := range tier1Ports {
		fmt.Fprintf(&b, "              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }\n", addr, p)
	}
	return b.String()
}

func (d *priDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	tier0 := d.ensureTier0()
	t0Ports := make([]int, tier0Hosts)
	for i, r := range tier0 {
		t0Ports[i] = r.port()
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
    - name: c_pri
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
%s
%s
`, refAdminPort, refContainerListenerPort, routeTable, healthChecksBlock,
		priorityEndpointsBlock("host.docker.internal", t0Ports, backendPorts))
}

func (d *priDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	tier0 := d.ensureTier0()
	t0Ports := make([]int, tier0Hosts)
	for i, r := range tier0 {
		t0Ports[i] = r.port()
	}
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0096, cluster: envoy-go-differential }
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
    - name: c_pri
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s
%s
`, subjAdminPort, subjListenerPort, routeTable, healthChecksBlock,
		priorityEndpointsBlock("127.0.0.1", t0Ports, backendPorts))
}

func (d *priDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

func (d *priDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

func (*priDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// classifyBody attributes a load response to tier "0" ("tier0:<idx>", the
// driver-owned toggleResponders) or tier "1" ("backend-<idx>:...", the
// runner-spawned HTTPEcho pool).
func classifyBody(body []byte) (tier string, err error) {
	s := string(body)
	switch {
	case strings.HasPrefix(s, "tier0:"):
		return "0", nil
	case strings.HasPrefix(s, "backend-"):
		return "1", nil
	default:
		return "", fmt.Errorf("body %q matches neither tier0: nor backend- prefix", s)
	}
}

func pollMembershipHealthy(side, adminAddr string, want int) error {
	deadline := time.Now().Add(convergeDeadline)
	var last int64 = -1
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			if v, ok := st["cluster.c_pri.membership_healthy"]; ok {
				last = int64(v)
				if v == uint64(want) {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: cluster.c_pri.membership_healthy did not converge to %d within %s (last seen %d)", side, want, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

type tierTally struct{ t0, t1 int }

func loadAndTally(ctx context.Context, side, addr string, n int) (tierTally, error) {
	var t tierTally
	for i := 0; i < n; i++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err != nil {
			return t, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if resp.StatusCode != http.StatusOK {
			return t, fmt.Errorf("%s: GET /[%d]: status %d, want 200", side, i, resp.StatusCode)
		}
		tier, err := classifyBody(body)
		if err != nil {
			return t, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if tier == "0" {
			t.t0++
		} else {
			t.t1++
		}
	}
	return t, nil
}

// tier1BackendBodies returns the exact, deterministic response-body set the
// runner's HTTPEcho backends produce for a bare "GET /" (the
// acceptHTTPEchoCounting format "backend-<idx>:<lastSegmentOfPath>",
// test/differential/runner_test.go — for path "/", lastSegmentOfPath stays
// "/" since the trailing-slash split leaves nothing after the final '/').
// Used as arm (a)'s warmup excludeBodies set: arm (a)'s HARD 100%/0% claim
// needs the SAME "wait for the split to actually land" treatment arm (b)
// needed (see warmupUntilStable's doc comment) — a live differential run
// during this task showed arm (a) leaking a small, run-to-run-variable
// fraction (9-54 of 300) to tier 1 even though ALL 10 hosts were healthy at
// boot, because a bare "any 200" gate is satisfied by EITHER tier and gives
// no signal that the worker thread's priority-load percentages have
// actually settled to the fully-healthy 100/0 split yet.
func tier1BackendBodies() map[string]bool {
	out := make(map[string]bool, tier1Hosts)
	for i := 0; i < tier1Hosts; i++ {
		out[fmt.Sprintf("backend-%d:/", i)] = true
	}
	return out
}

// warmupUntilStable polls the data path until it observes warmupStable
// CONSECUTIVE responses that are neither an error/non-200 NOR one of
// excludeBodies (SPEC §8.1's "K=10-consecutive-non-degraded-host warmup",
// reference_health_check_propagation_warmup).
//
// A plain "K consecutive 200s" gate (the brief's literal text, copied from
// the 0066/39.1 template) is a NO-OP here: this fixture's toggleResponder (by
// design, per the Task-8 package doc) answers 200 "tier0:<idx>" on the data
// path REGARDLESS of its /healthz health state — only /healthz itself flips
// 200/503. So even a host the active health checker has already marked
// unhealthy keeps satisfying a bare "is it 200" gate, and a plain-200 warmup
// converges instantly whether or not the per-worker-thread LB host set has
// actually caught up to the health-check-driven exclusion (the
// gauge-vs-host-set propagation gap this same memory note describes).
// Passing the exact response bodies of the to-be-excluded (failed-over)
// hosts gives the gate a real signal: once the worker thread's host set has
// updated, requests deterministically stop landing on those hosts, so K
// consecutive non-degraded hits is genuine proof of convergence. Confirmed by
// a live differential run during this task: WITHOUT this host-identity
// check, arm (b) (ALL of tier 0 failed) still routed 300/300 requests to
// tier 0 on the reference side — a complete failure to observe the failover
// — reproducing 0095-lb-locality-weighted's IDENTICAL documented finding
// verbatim (its warmupUntilStable doc comment describes the same symptom for
// region-A). Fixed by adding the excludeBodies parameter, mirroring 0095's
// exact shape.
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

// assertHardSplit asserts a HARD tier boundary (not a statistical band —
// SPEC §8.1's failover proof is exact by construction, since the losing
// tier's cascade-load bucket is EMPTY, not merely small): wantAll0 selects
// which tier must receive 100% (true → tier 0 gets everything, tier 1 gets
// nothing; false → the reverse, arm (b)'s failover outcome).
func assertHardSplit(t fixture.TB, side string, tally tierTally, wantAll0 bool) {
	t.Helper()
	if wantAll0 {
		if tally.t1 != 0 {
			t.Errorf("%s: tier 1 must receive ZERO traffic (arm a, tier 0 fully healthy); got t0=%d t1=%d", side, tally.t0, tally.t1)
		}
		if tally.t0 == 0 {
			t.Errorf("%s: tier 0 must receive ALL traffic (arm a); got t0=%d t1=%d", side, tally.t0, tally.t1)
		}
		return
	}
	if tally.t0 != 0 {
		t.Errorf("%s: tier 0 must receive ZERO traffic (arm b, fully failed over); got t0=%d t1=%d", side, tally.t0, tally.t1)
	}
	if tally.t1 == 0 {
		t.Errorf("%s: tier 1 must receive ALL traffic (arm b, the failover target); got t0=%d t1=%d", side, tally.t0, tally.t1)
	}
}

func (d *priDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ctx := context.Background()

	d.mu.Lock()
	refListener := d.refListener
	subjListener := d.subjListener
	tier0 := d.tier0
	d.mu.Unlock()
	if refListener == "" || subjListener == "" {
		t.Fatalf("listener addrs not stashed (ref=%q subj=%q)", refListener, subjListener)
	}

	// --- arm (a): static, all 10 hosts healthy — HARD 100%/0% ---
	if err := pollMembershipHealthy("reference", refAdminAddr, membershipTotal); err != nil {
		t.Fatalf("arm(a) converge: %v", err)
	}
	if err := pollMembershipHealthy("subject", subjAdminAddr, membershipTotal); err != nil {
		t.Fatalf("arm(a) converge: %v", err)
	}
	tier1Bodies := tier1BackendBodies()
	if err := warmupUntilStable(ctx, "reference", refListener, tier1Bodies); err != nil {
		t.Fatalf("arm(a) warmup: %v", err)
	}
	if err := warmupUntilStable(ctx, "subject", subjListener, tier1Bodies); err != nil {
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
	assertHardSplit(t, "reference/static", refStaticTally, true)
	assertHardSplit(t, "subject/static", subjStaticTally, true)

	// --- arm (b): fail ALL of tier 0, re-measure the HARD failover ---
	failedBodies := make(map[string]bool, tier0Hosts)
	for i, r := range tier0 {
		r.SetHealthy(false)
		failedBodies[fmt.Sprintf("tier0:%d", i)] = true
	}
	if err := pollMembershipHealthy("reference", refAdminAddr, membershipTotal-tier0Hosts); err != nil {
		t.Fatalf("arm(b) converge: %v", err)
	}
	if err := pollMembershipHealthy("subject", subjAdminAddr, membershipTotal-tier0Hosts); err != nil {
		t.Fatalf("arm(b) converge: %v", err)
	}
	if err := warmupUntilStable(ctx, "reference", refListener, failedBodies); err != nil {
		t.Fatalf("arm(b) warmup: %v", err)
	}
	if err := warmupUntilStable(ctx, "subject", subjListener, failedBodies); err != nil {
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
	assertHardSplit(t, "reference/failover", refDegradedTally, false)
	assertHardSplit(t, "subject/failover", subjDegradedTally, false)

	// --- cross-side deterministic stats ---
	ref, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}
	if ref["cluster.c_pri.upstream_rq_total"] == 0 {
		t.Fatalf("reference did NOT decode: cluster.c_pri.upstream_rq_total == 0")
	}
	for _, sd := range []struct {
		side string
		st   map[string]uint64
	}{{"reference", ref}, {"subject", subj}} {
		assertEq(t, sd.side, sd.st, "cluster.c_pri.membership_total", uint64(membershipTotal))
		assertEq(t, sd.side, sd.st, "cluster.c_pri.membership_healthy", uint64(membershipTotal-tier0Hosts))
		if got := sd.st["cluster.c_pri.upstream_rq_total"]; got < uint64(staticLoadCount+degradedLoadCount) {
			t.Errorf("%s: cluster.c_pri.upstream_rq_total = %d, want >= %d (the measured load alone; convergence/warmup traffic adds more)", sd.side, got, staticLoadCount+degradedLoadCount)
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
// into a map[name]uint64 (the 0066/0095 scrapeStats, verbatim).
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
	_ fixture.Driver        = (*priDriver)(nil)
	_ fixture.StatsAsserter = (*priDriver)(nil)
)
