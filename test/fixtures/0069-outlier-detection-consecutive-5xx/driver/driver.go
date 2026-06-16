// Package driver registers the 0069-outlier-detection-consecutive-5xx cross-side
// differential fixture (phase 40.1 SPEC §8 / PLAN Task 10).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture over ONE
// cluster c_od (lb_policy ROUND_ROBIN) with PASSIVE outlier detection
// (consecutive_5xx) over THREE endpoints — 2 HEALTHY HTTPEcho backends + 1
// always-503 host (the HTTP503Responder, BackendKind 35). It proves that an
// upstream host that returns consecutive 5xx responses is DETECTED by passive
// outlier detection and EJECTED from LB rotation on BOTH the envoy-go (subject)
// side and the reference-Envoy side.
//
// # Topology: 2 HEALTHY + 1 ALWAYS-503 (all runner-spawned)
//
//   - backend0 → c_od endpoint 0 (HTTPEcho; 200s every path; serves load)
//   - backend1 → c_od endpoint 1 (HTTPEcho; 200s every path; serves load)
//   - backend2 → c_od endpoint 2 (HTTP503Responder; always 503; gets ejected)
//
// Unlike 0066 (whose dead host is an unbound port), ALL THREE hosts here are
// runner-spawned backends — the 503 host is a live listener that answers 503.
// The runner selects it via PerHostBackendKind (BackendKindAt(2) → HTTP503
// Responder). BackendCount() is 3.
//
// # Cluster shape (both sides)
//
//		c_od: lb_policy ROUND_ROBIN, 3 endpoints, outlier_detection: {
//		        consecutive_5xx: 5, interval: 10s,
//		        base_ejection_time: 30s, max_ejection_percent: 100 }
//
//	  - Subject (envoy-go): type STATIC, endpoints = 127.0.0.1:<h0,h1,h2>
//	    (envoy-go's buildCluster ONLY supports STATIC).
//	  - Reference (Envoy): type STRICT_DNS, endpoints = host.docker.internal:<h0,
//	    h1,h2> (the 0066 reference shape; the reference MUST be STRICT_DNS).
//
// # The driver: ejection-drive + poll-to-converge + warmup (the determinism)
//
// The runner's hooks are DriveReference/DriveSubject (the byte-equiv stream, run
// FIRST) then AssertStats (run LAST, holding BOTH admin addrs). Because the
// ejection must be driven, observed, and the worker rotation drained BEFORE the
// strict measured phase, ALL of it runs inside AssertStats (the only hook
// holding both admin addrs). The Drive hooks STASH their listener addrs and
// return a fixed, address-independent byte stream ("READY\n") for the runner's
// CompareBytes gate.
//
//	AssertStats:
//	 1. Ejection drive: send ejectDriveRequests 503-TOLERANT GET / round-robin to
//	    each side's listener. Under strict round-robin over 3 hosts, host2 (the
//	    503) is picked every 3rd request; consec5xx is PER-HOST and is only reset
//	    by a 2xx FROM host2 (which never comes), so host2 accrues consecutive 5xx
//	    until it crosses consecutive_5xx (5) and is ejected.
//	 2. Poll /stats on BOTH sides until
//	    cluster.c_od.outlier_detection.ejections_active == 1 (deadline 30s, poll
//	    200ms; NO fixed sleep — fail clearly with the last value on timeout).
//	 3. Warmup: after the gauge reads 1, send 503-tolerant requests until
//	    warmupStable CONSECUTIVE 200s prove the worker rotation has dropped host2,
//	    on BOTH sides (closes the main→worker propagation window, the 0066
//	    reference_health_check_propagation_warmup mechanism).
//	 4. Measured load: baseline the per-request counters post-warmup, send n GET /
//	    on each side; assert (delta) upstream_rq_2xx == n, upstream_rq_5xx == 0,
//	    and every body is backend-0:/backend-1: (NEVER backend-2: — the ejected
//	    503 host serves nothing in the measured phase).
//	 5. Cross-side stat parity (both sides): ejections_active == 1,
//	    ejections_enforced_total >= 1, ejections_enforced_consecutive_5xx >= 1,
//	    ejections_detected_consecutive_5xx >= 1; upstream_rq_total > 0 reference
//	    (decode-ran guard). The recovery / un-eject arm is DEFERRED (AMEND-OD1).
//
// # Cross-references
//
//   - phase 40.1 SPEC §8 / PLAN Task 10 (the fixture design).
//   - 0066-health-check-http (the cross-side poll-to-converge + warmup + delta-
//     counter template: reference STRICT_DNS / host.docker.internal, subject
//     STATIC / 127.0.0.1; scrapeStats; the Bootstrap/Config builders;
//     backendIdxFromBody host attribution).
//   - reference_health_check_propagation_warmup (poll-the-gauge + the warmup-
//     until-K-consecutive-200s gate + delta per-request counters).
//   - reference_docker_probe_bridge_network (shared bridge + STRICT_DNS
//     hostnames; the "decode ran" guard verifies the reference forwarded
//     traffic).
//   - reference_differential_run_selector (target -run 'TestDifferential/0069').
//   - reference_fixture_workload_constant_desync (counts single-sourced — D-S40.1-4).
//   - reference_differential_asserter_dispatch (cross-side assertions via the
//     StatsAsserter path — NOT SubjectAsserter, which only runs reference-less).
//   - AMEND-OD1 (recovery/un-eject arm deferred; lazy-vs-sweep diverges
//     cross-side). AMEND-OD4 (do NOT assert ejections_detected_consecutive_
//     gateway_failure; envoy-go has no gateway detector at 40.1).
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
	fixtureName = "0069-outlier-detection-consecutive-5xx"

	// In-container reference Envoy listener port for l_http. Fixtures run
	// sequentially; a distinct value avoids confusion (0066 took 19155, the
	// 39.2 leg took up to 19157, this takes 19158).
	refContainerListenerPort = 19158

	refAdminPort = 9901

	// backendCount is the total number of runner-spawned backends (ALL three are
	// live listeners; host2 answers 503). healthyBackendCount is the count that
	// serve the measured load after host2 is ejected.
	backendCount        = 3
	healthyBackendCount = 2

	// outlier_detection config (single-sourced; the Bootstrap/Config builders +
	// the stat assertions read these — D-S40.1-4).
	consec5xxThreshold = 5                // consecutive_5xx ejection threshold
	interval           = 10 * time.Second // detection interval (parse-accepted)
	baseEjectionTime   = 30 * time.Second // base_ejection_time (recovery DEFERRED)
	maxEjectionPercent = 100              // allow the single 503 host to be ejected

	// ejectDriveRequests is the 503-tolerant ejection-drive count. Under strict
	// round-robin over 3 hosts, host2 is picked every 3rd request, so it crosses
	// consec5xxThreshold after ~consec5xxThreshold*3 requests. A margin guarantees
	// ejection even if early ordering is not perfectly round-robin.
	ejectDriveRequests = consec5xxThreshold*3 + 9 // 24

	// n is the measured-phase request count per side. After host2 is ejected the
	// load lands 100% on the 2 healthy hosts; the assertion is delta 2xx==n /
	// 5xx==0 + body idx ∈ {0,1}, NOT a band, so n need not be large.
	n = 60

	// Convergence poll budget (NO fixed sleep — poll until the predicate holds).
	convergeDeadline = 30 * time.Second
	convergePoll     = 200 * time.Millisecond

	// Warmup gate: the gauge updates before the worker LB host-sets drop the
	// ejected host (a propagation window), so warmup sends 503-tolerant requests
	// until warmupStable CONSECUTIVE 200s prove the worker rotation has dropped
	// host2, THEN the strict measured phase runs.
	warmupStable   = 10
	warmupDeadline = 15 * time.Second

	// Gauge / counter stat keys (single-sourced).
	statEjectionsActive    = "cluster.c_od.outlier_detection.ejections_active"
	statEjectEnforcedTotal = "cluster.c_od.outlier_detection.ejections_enforced_total"
	statEjectEnforced5xx   = "cluster.c_od.outlier_detection.ejections_enforced_consecutive_5xx"
	statEjectDetected5xx   = "cluster.c_od.outlier_detection.ejections_detected_consecutive_5xx"
	statUpstreamRqTotal    = "cluster.c_od.upstream_rq_total"
	statUpstreamRq2xx      = "cluster.c_od.upstream_rq_2xx"
	statUpstreamRq5xx      = "cluster.c_od.upstream_rq_5xx"
)

func init() {
	fixture.RegisterFixture(fixtureName, &odDriver{})
}

// odDriver is STATEFUL: the Drive hooks stash the per-side listener addrs (the
// reference listener mapped port is only knowable at DriveReference; the subject
// listener is knowable at SubjectConfig) so AssertStats — the only hook holding
// BOTH admin addrs — can drive the ejection, poll-converge, warm up, and assert.
type odDriver struct {
	mu           sync.Mutex
	refListener  string // host:port of the reference l_http listener (from DriveReference)
	subjListener string // 127.0.0.1:<port> of the subject l_http listener (from SubjectConfig)
}

func (*odDriver) BackendCount() int                { return backendCount }
func (*odDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }

// BackendKindAt implements fixture.PerHostBackendKind: hosts 0/1 are healthy
// HTTPEcho, host 2 is the always-503 responder the outlier detector ejects.
func (*odDriver) BackendKindAt(i int) fixture.BackendKind {
	if i == 2 {
		return fixture.HTTP503Responder
	}
	return fixture.HTTPEcho
}

func (*odDriver) SubjectListenerName() string { return "l_http" }
func (*odDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

// outlierBlock is the shared cluster outlier_detection YAML (identical on both
// sides — NAT-transparent static config). consecutive_5xx 5, interval 10s,
// base_ejection_time 30s, max_ejection_percent 100.
var outlierBlock = fmt.Sprintf(`      outlier_detection:
        consecutive_5xx: %d
        interval: %s
        base_ejection_time: %s
        max_ejection_percent: %d`,
	consec5xxThreshold,
	durSeconds(interval),
	durSeconds(baseEjectionTime),
	maxEjectionPercent)

// durSeconds renders a whole-second duration as the protobuf-duration string
// Envoy accepts (e.g. "10s").
func durSeconds(d time.Duration) string {
	return strconv.Itoa(int(d/time.Second)) + "s"
}

// routeTable routes / to c_od (the data path). Identical on both sides.
const routeTable = `                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_od }`

func (d *odDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	// STRICT_DNS + host.docker.internal (the 0066 reference shape). c_od over the
	// 2 healthy backends + the always-503 host, with passive outlier detection.
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
    - name: c_od
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
%s
      load_assignment:
        cluster_name: c_od
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, refAdminPort, refContainerListenerPort, routeTable, outlierBlock, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (d *odDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	// STATIC + 127.0.0.1 (the 0066 subject shape). c_od over the 2 healthy
	// backends + the always-503 host, with passive outlier detection.
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0069, cluster: envoy-go-differential }
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
    - name: c_od
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s
      load_assignment:
        cluster_name: c_od
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, routeTable, outlierBlock, backendPorts[0], backendPorts[1], backendPorts[2])
}

// DriveReference stashes the reference listener addr and returns the fixed
// byte-equiv stream. The real work runs in AssertStats.
func (d *odDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

// DriveSubject returns the fixed byte-equiv stream (the subject listener addr was
// already stashed in SubjectConfig). The real work runs in AssertStats.
func (d *odDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0066 raw
// /ready probe, verbatim).
func (*odDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// backendIdxFromBody parses the canned body "backend-<idx>:<seg>" (both HTTPEcho
// and HTTP503Responder use this format) and returns the embedded backend idx
// (the host-attribution signal).
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

// driveEjection sends ejectDriveRequests 503-TOLERANT GET / to addr (the 503
// host's responses ARE 503 until it is ejected — that is the point). A transport
// error is a hard failure; a 503 status is expected and tolerated.
func driveEjection(ctx context.Context, side, addr string) error {
	for i := 0; i < ejectDriveRequests; i++ {
		resp, _, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err != nil {
			return fmt.Errorf("%s: eject-drive GET /[%d]: %w", side, i, err)
		}
		// 200 (healthy host) and 503 (the always-503 host) are both expected
		// during the drive; only a transport error aborts.
		_ = resp
	}
	return nil
}

// pollEjectionsActive scrapes adminAddr/stats every convergePoll until
// statEjectionsActive == 1 or the deadline trips.
func pollEjectionsActive(side, adminAddr string) error {
	deadline := time.Now().Add(convergeDeadline)
	var last int64 = -1
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			if v, ok := st[statEjectionsActive]; ok {
				last = int64(v)
				if v == 1 {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: %s did not converge to 1 within %s (last seen %d) — 503 host not ejected? (consecutive_5xx accrued? detector wired? RecordUpstreamResult firing?)",
				side, statEjectionsActive, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

// warmupUntilStable sends GET / tolerating transient 503s until warmupStable
// CONSECUTIVE 200s, or the deadline trips. It closes the gauge→worker-set
// propagation window (ejections_active reads 1 before the worker LB drops the
// ejected host). An un-ejected build (deliberate break) round-robins to the 503
// host every 3rd pick → never warmupStable consecutive 200s → this errors,
// preserving liveness.
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
			return fmt.Errorf("%s: data path did not stabilize to %d consecutive 200s within %s (last status %d, err %v) — 503 host still in worker rotation?",
				side, warmupStable, warmupDeadline, lastCode, lastErr)
		}
	}
}

// loadAndTally sends n GET / to addr and returns per-host hit counts. A non-200
// is a hard error (the ejected 503 host must serve nothing), as is any body
// attributing host2 — in the measured phase only hosts 0/1 may answer.
func loadAndTally(ctx context.Context, side, addr string) ([backendCount]int, error) {
	var counts [backendCount]int
	for i := 0; i < n; i++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err != nil {
			return counts, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if resp.StatusCode != http.StatusOK {
			return counts, fmt.Errorf("%s: GET /[%d]: status %d, want 200 (503 host NOT ejected → it answered 503?)", side, i, resp.StatusCode)
		}
		idx, err := backendIdxFromBody(body)
		if err != nil {
			return counts, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if idx < 0 || idx >= backendCount {
			return counts, fmt.Errorf("%s: GET /[%d]: backend idx %d out of range [0,%d)", side, i, idx, backendCount)
		}
		if idx == 2 {
			return counts, fmt.Errorf("%s: GET /[%d]: served by host2 (the ejected 503 host) — ejection not enforced in the data path", side, i)
		}
		counts[idx]++
	}
	return counts, nil
}

// AssertStats is the in-band eject-drive + poll-converge + warmup + measured-load
// + stat-parity flow (the only hook holding both admin addrs). See the package
// doc for the five-step shape.
func (d *odDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ctx := context.Background()

	d.mu.Lock()
	refListener := d.refListener
	subjListener := d.subjListener
	d.mu.Unlock()
	if refListener == "" || subjListener == "" {
		t.Fatalf("listener addrs not stashed (ref=%q subj=%q) — Drive hooks did not run?", refListener, subjListener)
	}

	// 1. Ejection drive: 503-tolerant load to accrue consecutive 5xx on host2.
	if err := driveEjection(ctx, "reference", refListener); err != nil {
		t.Fatalf("eject-drive: %v", err)
	}
	if err := driveEjection(ctx, "subject", subjListener); err != nil {
		t.Fatalf("eject-drive: %v", err)
	}

	// 2. Poll-to-converge: both sides must eject host2 BEFORE the measured load.
	if err := pollEjectionsActive("reference", refAdminAddr); err != nil {
		t.Fatalf("converge: %v", err)
	}
	if err := pollEjectionsActive("subject", subjAdminAddr); err != nil {
		t.Fatalf("converge: %v", err)
	}

	// 3. Warmup: close the gauge→worker-set propagation window before measuring.
	if err := warmupUntilStable(ctx, "reference", refListener); err != nil {
		t.Fatalf("warmup: %v", err)
	}
	if err := warmupUntilStable(ctx, "subject", subjListener); err != nil {
		t.Fatalf("warmup: %v", err)
	}

	// 3b. Baselines AFTER warmup: the per-request counters are measured as a
	// DELTA over the measured phase (the eject-drive + warmup also increment
	// upstream_rq_*, so absolute counts would over-count by a variable amount).
	refBase, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref baseline /stats: %v", err)
	}
	subjBase, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj baseline /stats: %v", err)
	}

	// 4. Measured load: n GET / on each side, after ejection + warmup.
	refCounts, err := loadAndTally(ctx, "reference", refListener)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	subjCounts, err := loadAndTally(ctx, "subject", subjListener)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// 4b. 100%-to-healthy (per side): every request was served 200 by a healthy
	// host (loadAndTally already failed on any non-200 / host2 hit); the tally
	// must sum to n and BOTH healthy hosts must be touched (ROUND_ROBIN over 2).
	for _, sd := range []struct {
		side   string
		counts [backendCount]int
	}{{"reference", refCounts}, {"subject", subjCounts}} {
		sum := 0
		for _, c := range sd.counts {
			sum += c
		}
		if sum != n {
			t.Errorf("%s: healthy-host tally sum %d != %d", sd.side, sum, n)
		}
		if sd.counts[2] != 0 {
			t.Errorf("%s: host2 (ejected 503) served %d requests in the measured phase, want 0", sd.side, sd.counts[2])
		}
		for i := 0; i < healthyBackendCount; i++ {
			if sd.counts[i] == 0 {
				t.Errorf("%s: healthy host[%d] served 0 requests — ROUND_ROBIN did not spread over the 2 healthy hosts (was host2 actually ejected, or a healthy host wrongly ejected?)", sd.side, i)
			}
		}
	}

	// 5. Cross-side stats. Scrape AFTER load (final counters).
	ref, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}

	// "decode ran" guard (reference_docker_probe_bridge_network).
	if ref[statUpstreamRqTotal] == 0 {
		t.Fatalf("reference did NOT decode: %s == 0 (container could not reach backends — bridge network / host.docker.internal?)", statUpstreamRqTotal)
	}

	for _, sd := range []struct {
		side string
		st   map[string]uint64
		base map[string]uint64
	}{{"reference", ref, refBase}, {"subject", subj, subjBase}} {
		// Outlier-detection parity: host2 ejected and held, enforced + detected.
		assertEq(t, sd.side, sd.st, statEjectionsActive, 1)
		assertAtLeast(t, sd.side, sd.st, statEjectEnforcedTotal, 1)
		assertAtLeast(t, sd.side, sd.st, statEjectEnforced5xx, 1)
		assertAtLeast(t, sd.side, sd.st, statEjectDetected5xx, 1)
		// NOTE: ejections_detected_consecutive_gateway_failure is NOT asserted —
		// envoy-go has no gateway detector at 40.1; the reference trips it on 503
		// (AMEND-OD4). Recovery / un-eject is DEFERRED (AMEND-OD1).

		// Measured-phase load conservation (DELTA, baseline post-warmup): all n
		// requests routed to a healthy host, all 2xx, 0 5xx in the measured phase
		// (the eject-drive + warmup 503s are in the baseline, not the delta).
		assertDelta(t, sd.side, sd.st, sd.base, statUpstreamRq2xx, n)
		assertDelta(t, sd.side, sd.st, sd.base, statUpstreamRq5xx, 0)
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

// assertAtLeast asserts st[key] >= want (the enforced/detected counters fire at
// least once; the exact count can differ cross-side by detection timing).
func assertAtLeast(t fixture.TB, side string, st map[string]uint64, key string, want uint64) {
	t.Helper()
	v, ok := st[key]
	if !ok {
		t.Errorf("%s: %s ABSENT in /stats", side, key)
		return
	}
	if v < want {
		t.Errorf("%s: %s = %d, want >= %d", side, key, v, want)
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

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0066 driver scrapeStats,
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
	_ fixture.Driver             = (*odDriver)(nil)
	_ fixture.StatsAsserter      = (*odDriver)(nil)
	_ fixture.BackendKindAware   = (*odDriver)(nil)
	_ fixture.PerHostBackendKind = (*odDriver)(nil)
)
