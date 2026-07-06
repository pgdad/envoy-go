// Package driver registers the 0071-outlier-detection-local-origin cross-side
// differential fixture (phase 40.2 SPEC §10 / PLAN Task 8).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture over ONE
// cluster c_lo (lb_policy ROUND_ROBIN) with PASSIVE outlier detection over THREE
// endpoints — 2 LIVE HTTPEcho backends + 1 DEAD host (a host:port with no
// listener → connect refused). With split_external_local_origin_errors: true,
// each connect-refused to the dead host is a LOCAL-ORIGIN failure
// (LocalOriginErr: true), so the consecutive_local_origin_failure detector ejects
// it after consecutive_local_origin_failure failures — on BOTH the envoy-go
// (subject) side and the reference-Envoy side.
//
// # The split (the load-bearing assertion)
//
// split_external_local_origin_errors: true routes the dead-host connect-refused
// to the LOCAL-ORIGIN detector ONLY. The consecutive_5xx and consecutive_gateway_
// failure detectors therefore NEVER fire over the eject-drive — the fixture
// asserts, cross-side, that ejections_detected_consecutive_5xx == 0 while the
// local-origin counters fire. If the split branch regressed and the local-origin
// failure were instead routed to the gateway/5xx detector, that counter would be
// present > 0 and the equality would bite (the assertEq absent-as-0 accommodation
// only swallows ABSENT, not present > 0).
//
// # Topology: 2 LIVE backends (runner-spawned) + 1 DEAD host (unbound port)
//
//   - backend0 → c_lo endpoint 0 (LIVE; HTTPEcho 200s every path; serves load)
//   - backend1 → c_lo endpoint 1 (LIVE; HTTPEcho 200s every path; serves load)
//   - deadPort → c_lo endpoint 2 (DEAD; no listener → connect refused → LocalOriginErr)
//
// The DEAD host is NOT a runner backend (the runner spawns BackendCount()==2 live
// HTTPEcho backends — the 0066 shape, NO PerHostBackendKind). The driver allocates
// a host port, binds it to learn the number, then CLOSES the listener so the port
// stays unbound for the run — both sides reference that same port number
// (reference via host.docker.internal:<dead>, subject via 127.0.0.1:<dead>), and a
// connect to it is refused on both sides. On the subject side the refused connect
// reaches the H1 AcquireH1 connect-failure seam (Task 6) → RecordUpstreamResult
// with LocalOriginErr: true → the local-origin detector.
//
// # Cluster shape (both sides)
//
//		c_lo: lb_policy ROUND_ROBIN, 3 endpoints, outlier_detection: {
//		        consecutive_local_origin_failure: 5,
//		        enforcing_consecutive_local_origin_failure: 100,
//		        split_external_local_origin_errors: true,
//		        interval: 10s, base_ejection_time: 30s, max_ejection_percent: 100 }
//
//	  - Subject (envoy-go): type STATIC, endpoints = 127.0.0.1:<h0,h1,dead>.
//	  - Reference (Envoy): type STRICT_DNS, endpoints = host.docker.internal:<h0,
//	    h1,dead> (the 0066/0070 reference shape; the reference MUST be STRICT_DNS).
//
// # The driver: ejection-drive + poll-to-converge + warmup (the 0070 template)
//
//	AssertStats:
//	 1. Ejection drive: send ejectDriveRequests 503/502-TOLERANT GET / round-robin
//	    to each side's listener. Under strict round-robin over 3 endpoints, the
//	    dead host is picked every 3rd request; consecLO is PER-HOST and only resets
//	    on a COMPLETED external response from the dead host (which never comes), so
//	    it accrues consecutive local-origin failures until it crosses
//	    consecutive_local_origin_failure (5) and is ejected. (Requests to the dead
//	    host return 503/502 to the client — tolerated during the drive.)
//	 2. Poll /stats on BOTH sides until
//	    cluster.c_lo.outlier_detection.ejections_active == 1 (deadline,
//	    poll 200ms; NO fixed sleep — fail clearly with the last value on timeout).
//	 3. Warmup: after the gauge reads 1, send 5xx-tolerant requests until
//	    warmupStable CONSECUTIVE 200s prove the worker rotation has dropped the dead
//	    host, on BOTH sides (closes the main→worker propagation window).
//	 4. Measured load: baseline the per-request counters post-warmup, send n GET /
//	    on each side; assert (delta) upstream_rq_2xx == n, upstream_rq_5xx == 0,
//	    every body backend-0:/backend-1: (NEVER the dead host).
//	 5. Cross-side stat parity (both sides): ejections_active == 1,
//	    ejections_enforced_total >= 1, ejections_detected_consecutive_local_origin_
//	    failure >= 1, ejections_enforced_consecutive_local_origin_failure >= 1, AND
//	    ejections_detected_consecutive_5xx == 0 (split=true routes the local-origin
//	    failures away from the 5xx/gateway detectors). upstream_rq_total > 0
//	    reference (decode-ran guard). The recovery / un-eject arm is DEFERRED.
//
// # Cross-references
//
//   - phase 40.2 SPEC §10 / PLAN Task 8 (the fixture design).
//   - 0066-health-check-http (the dead-host mechanism: allocDeadPort, BackendCount
//     ==2, the dead host is an INJECTED endpoint not a runner backend; the 503-
//     tolerant warmup).
//   - 0070-outlier-detection-consecutive-gateway-failure (the outlier StatsAsserter
//     shape: eject-drive + poll-to-converge + warmup + delta-counter flow; the
//     assertEq absent-counter-as-0 accommodation; StatsAsserter wiring).
//   - reference_health_check_propagation_warmup (poll-the-gauge + warmup gate).
//   - reference_docker_probe_bridge_network (shared bridge + STRICT_DNS host).
//   - reference_differential_run_selector (target -run 'TestDifferential/0071').
//   - reference_fixture_workload_constant_desync (counts single-sourced).
//   - reference_differential_asserter_dispatch (cross-side via StatsAsserter).
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
	fixtureName = "0071-outlier-detection-local-origin"

	// In-container reference Envoy listener port for l_http. Fixtures run
	// sequentially; a distinct value avoids confusion (0070 took 19159, this
	// takes the next-free 19160).
	refContainerListenerPort = 19160

	refAdminPort = 9901

	// backendCount is the number of LIVE runner-spawned HTTPEcho backends. The
	// DEAD host is a separately-allocated unbound port (NOT a runner backend — the
	// 0066 shape, NO PerHostBackendKind). healthyBackendCount is the count that
	// serves the measured load after the dead host is ejected.
	backendCount        = 2
	healthyBackendCount = 2

	// endpointCount is the total endpoints in c_lo (2 live + 1 dead). The dead
	// host occupies the last index (== 2); it is an injected unbound endpoint,
	// NOT a runner backend.
	endpointCount = 3

	// outlier_detection config (single-sourced; the Bootstrap/Config builders +
	// the stat assertions read these). split=true routes the dead-host connect-
	// refused (a LocalOriginErr) to the LOCAL-ORIGIN detector ONLY; the
	// consecutive_5xx / gateway detectors never fire (→ detected_consecutive_5xx
	// == 0).
	splitLocalOrigin   = true             // split_external_local_origin_errors
	consecLOThreshold  = 5                // consecutive_local_origin_failure ejection threshold
	enforcingLOPercent = 100              // enforcing_consecutive_local_origin_failure (enforce every LO detection)
	interval           = 10 * time.Second // detection interval (parse-accepted)
	baseEjectionTime   = 30 * time.Second // base_ejection_time (recovery DEFERRED)
	maxEjectionPercent = 100              // allow the single dead host to be ejected

	// ejectDriveRequests is the 5xx-tolerant ejection-drive count. Under strict
	// round-robin over 3 endpoints, the dead host is picked every 3rd request, so
	// it crosses consecLOThreshold after ~consecLOThreshold*3 requests. A margin
	// guarantees ejection even if early ordering is not perfectly round-robin.
	ejectDriveRequests = consecLOThreshold*3 + 9 // 24

	// n is the measured-phase request count per side. After the dead host is
	// ejected the load lands 100% on the 2 live hosts; the assertion is delta
	// 2xx==n / 5xx==0 + body idx ∈ {0,1}, NOT a band, so n need not be large.
	n = 60

	// Convergence poll budget (NO fixed sleep — poll until the predicate holds).
	// A dead-host connect-refused can be slower to accrue than an HTTP 503 (the
	// connect attempt + connect_timeout per pick), so the deadline carries ample
	// headroom over the 0070 gateway budget.
	convergeDeadline = 45 * time.Second
	convergePoll     = 200 * time.Millisecond

	// Warmup gate: the gauge updates before the worker LB host-sets drop the
	// ejected host (a propagation window), so warmup sends 5xx-tolerant requests
	// until warmupStable CONSECUTIVE 200s prove the worker rotation has dropped the
	// dead host, THEN the strict measured phase runs.
	warmupStable   = 10
	warmupDeadline = 20 * time.Second

	// Gauge / counter stat keys (single-sourced).
	statEjectionsActive    = "cluster.c_lo.outlier_detection.ejections_active"
	statEjectEnforcedTotal = "cluster.c_lo.outlier_detection.ejections_enforced_total"
	statEjectDetectedLO    = "cluster.c_lo.outlier_detection.ejections_detected_consecutive_local_origin_failure"
	statEjectEnforcedLO    = "cluster.c_lo.outlier_detection.ejections_enforced_consecutive_local_origin_failure"
	statEjectDetected5xx   = "cluster.c_lo.outlier_detection.ejections_detected_consecutive_5xx"
	statUpstreamRqTotal    = "cluster.c_lo.upstream_rq_total"
	statUpstreamRq2xx      = "cluster.c_lo.upstream_rq_2xx"
	statUpstreamRq5xx      = "cluster.c_lo.upstream_rq_5xx"
)

func init() {
	fixture.RegisterFixture(fixtureName, &odDriver{})
}

// odDriver is STATEFUL: the Drive hooks stash the per-side listener addrs (the
// reference listener mapped port is only knowable at DriveReference; the subject
// listener is knowable at SubjectConfig) so AssertStats — the only hook holding
// BOTH admin addrs — can drive the ejection, poll-converge, warm up, and assert.
// deadPort is the unbound host port shared by both sides' dead endpoint.
type odDriver struct {
	mu           sync.Mutex
	refListener  string // host:port of the reference l_http listener (from DriveReference)
	subjListener string // 127.0.0.1:<port> of the subject l_http listener (from SubjectConfig)
	deadPort     int    // the unbound host port shared by both sides' dead endpoint
}

func (*odDriver) BackendCount() int                { return backendCount }
func (*odDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (*odDriver) SubjectListenerName() string      { return "l_http" }
func (*odDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// allocDeadPort binds 0.0.0.0:0, captures the assigned port, then CLOSES the
// listener so the port stays unbound — a connect to it is refused (the dead-host
// connect-failure mechanism, the 0066 precedent). Memoized: both
// ReferenceBootstrap and SubjectConfig call it; they must agree on the SAME port
// number.
func (d *odDriver) allocDeadPort() int {
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
	_ = ln.Close() // release → nothing listens → connect refused → LocalOriginErr
	d.deadPort = port
	return port
}

// outlierBlock is the shared cluster outlier_detection YAML (identical on both
// sides — NAT-transparent static config). split=true routes the local-origin
// failures to the LO detector; the LO detector is the ejection trigger.
var outlierBlock = fmt.Sprintf(`      outlier_detection:
        consecutive_local_origin_failure: %d
        enforcing_consecutive_local_origin_failure: %d
        split_external_local_origin_errors: %t
        interval: %s
        base_ejection_time: %s
        max_ejection_percent: %d`,
	consecLOThreshold,
	enforcingLOPercent,
	splitLocalOrigin,
	durSeconds(interval),
	durSeconds(baseEjectionTime),
	maxEjectionPercent)

// durSeconds renders a whole-second duration as the protobuf-duration string
// Envoy accepts (e.g. "10s").
func durSeconds(d time.Duration) string {
	return strconv.Itoa(int(d/time.Second)) + "s"
}

// routeTable routes / to c_lo (the data path). Identical on both sides.
const routeTable = `                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_lo }`

func (d *odDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	dead := d.allocDeadPort()
	// STRICT_DNS + host.docker.internal (the 0066/0070 reference shape). c_lo over
	// the 2 live backends + the dead host, with split-true passive outlier detection.
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
    - name: c_lo
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
%s
      load_assignment:
        cluster_name: c_lo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, refAdminPort, refContainerListenerPort, routeTable, outlierBlock, backendPorts[0], backendPorts[1], dead)
}

func (d *odDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	dead := d.allocDeadPort()
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	// STATIC + 127.0.0.1 (the 0066/0070 subject shape). c_lo over the 2 live
	// backends + the dead host, with split-true passive outlier detection.
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0071, cluster: envoy-go-differential }
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
    - name: c_lo
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s
      load_assignment:
        cluster_name: c_lo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, routeTable, outlierBlock, backendPorts[0], backendPorts[1], dead)
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

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0066/0070
// raw /ready probe, verbatim).
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

// backendIdxFromBody parses the HTTPEcho canned body "backend-<idx>:<seg>" and
// returns the embedded backend idx (the host-attribution signal). Only the 2 live
// hosts emit a body; the dead host serves nothing (its picks fail-fast to a 5xx).
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

// driveEjection sends ejectDriveRequests 5xx-TOLERANT GET / to addr. Picks of the
// dead host fail the connect → a 503/502 local reply (LocalOriginErr) until it is
// ejected — that is the point. A transport error is a hard failure; a 5xx status
// is expected and tolerated.
func driveEjection(ctx context.Context, side, addr string) error {
	for i := 0; i < ejectDriveRequests; i++ {
		resp, _, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err != nil {
			return fmt.Errorf("%s: eject-drive GET /[%d]: %w", side, i, err)
		}
		// 200 (live host) and 503/502 (the dead host's connect-failure local reply)
		// are both expected during the drive; only a transport error aborts.
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
			return fmt.Errorf("%s: %s did not converge to 1 within %s (last seen %d) — dead host not ejected? (consecutive_local_origin_failure accrued? split=true routing connect-refused to the LO detector? RecordUpstreamResult{LocalOriginErr:true} firing at AcquireH1?)",
				side, statEjectionsActive, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

// warmupUntilStable sends GET / tolerating transient 5xx until warmupStable
// CONSECUTIVE 200s, or the deadline trips. It closes the gauge→worker-set
// propagation window (ejections_active reads 1 before the worker LB drops the
// ejected host). An un-ejected build (deliberate break) round-robins to the dead
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
			return fmt.Errorf("%s: data path did not stabilize to %d consecutive 200s within %s (last status %d, err %v) — dead host still in worker rotation?",
				side, warmupStable, warmupDeadline, lastCode, lastErr)
		}
	}
}

// loadAndTally sends n GET / to addr and returns per-host hit counts. A non-200
// is a hard error (the ejected dead host must serve nothing), as is any body
// attributing a host outside the 2 live backends — in the measured phase only
// hosts 0/1 may answer.
func loadAndTally(ctx context.Context, side, addr string) ([backendCount]int, error) {
	var counts [backendCount]int
	for i := 0; i < n; i++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err != nil {
			return counts, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if resp.StatusCode != http.StatusOK {
			return counts, fmt.Errorf("%s: GET /[%d]: status %d, want 200 (dead host NOT ejected → it answered a connect-failure 5xx?)", side, i, resp.StatusCode)
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

	// 1. Ejection drive: 5xx-tolerant load to accrue consecutive local-origin
	// failures on the dead host (each pick → connect refused → LocalOriginErr).
	if err := driveEjection(ctx, "reference", refListener); err != nil {
		t.Fatalf("eject-drive: %v", err)
	}
	if err := driveEjection(ctx, "subject", subjListener); err != nil {
		t.Fatalf("eject-drive: %v", err)
	}

	// 2. Poll-to-converge: both sides must eject the dead host BEFORE the load.
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

	// 4b. 100%-to-live (per side): every request was served 200 by a live host
	// (loadAndTally already failed on any non-200 / out-of-range hit); the tally
	// must sum to n and BOTH live hosts must be touched (ROUND_ROBIN over 2).
	for _, sd := range []struct {
		side   string
		counts [backendCount]int
	}{{"reference", refCounts}, {"subject", subjCounts}} {
		sum := 0
		for _, c := range sd.counts {
			sum += c
		}
		if sum != n {
			t.Errorf("%s: live-host tally sum %d != %d", sd.side, sum, n)
		}
		for i := 0; i < healthyBackendCount; i++ {
			if sd.counts[i] == 0 {
				t.Errorf("%s: live host[%d] served 0 requests — ROUND_ROBIN did not spread over the 2 live hosts (was the dead host actually ejected, or a live host wrongly ejected?)", sd.side, i)
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
		// Outlier-detection parity: the dead host ejected and held, enforced +
		// detected VIA THE LOCAL-ORIGIN DETECTOR.
		assertEq(t, sd.side, sd.st, statEjectionsActive, 1)
		assertAtLeast(t, sd.side, sd.st, statEjectEnforcedTotal, 1)
		assertAtLeast(t, sd.side, sd.st, statEjectDetectedLO, 1)
		assertAtLeast(t, sd.side, sd.st, statEjectEnforcedLO, 1)
		// The split invariant: split_external_local_origin_errors: true routes the
		// dead-host connect-refused (LocalOriginErr) to the LOCAL-ORIGIN detector
		// ONLY — the consecutive_5xx detector never fires, so detected_consecutive_5xx
		// MUST be exactly 0. This equality bites if the split branch regressed and the
		// local-origin failure were routed to the gateway/5xx detector (that would
		// lift detected_consecutive_5xx off 0; the assertEq absent-as-0 accommodation
		// only swallows ABSENT, not present > 0).
		assertEq(t, sd.side, sd.st, statEjectDetected5xx, 0)

		// Measured-phase load conservation (DELTA, baseline post-warmup): all n
		// requests routed to a live host, all 2xx, 0 5xx in the measured phase (the
		// eject-drive + warmup 5xx are in the baseline, not the delta).
		assertDelta(t, sd.side, sd.st, sd.base, statUpstreamRq2xx, n)
		assertDelta(t, sd.side, sd.st, sd.base, statUpstreamRq5xx, 0)
	}
}

func assertEq(t fixture.TB, side string, st map[string]uint64, key string, want uint64) {
	t.Helper()
	v, ok := st[key]
	if !ok {
		// Absent counters read as 0 (reference Envoy lazily allocates per-detector
		// counters); an absent-and-want-0 is satisfied. This swallows ONLY absence,
		// NOT a present-but-nonzero value — so detected_consecutive_5xx present > 0
		// (a split-branch regression) still bites.
		if want == 0 {
			return
		}
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
// "name: value" lines into a map[name]uint64. (The 0070 driver scrapeStats,
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

// Compile-time interface assertions. NO PerHostBackendKind — the dead host is an
// injected unbound endpoint, NOT a runner backend (the 0066 shape).
var (
	_ fixture.Driver           = (*odDriver)(nil)
	_ fixture.StatsAsserter    = (*odDriver)(nil)
	_ fixture.BackendKindAware = (*odDriver)(nil)
)
