// Package driver registers the 0075-retry-loop cross-side differential fixture
// (phase 42.1 SPEC / PLAN Task 9).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture that proves the
// HTTP retry loop behaves IDENTICALLY on the envoy-go (subject) side and the
// reference-Envoy side. It has TWO clusters + TWO routes, each exercising a
// distinct retry behavior:
//
//   - EXHAUSTION (/exhaust → c_exhaust): the cluster has ONE host, the always-503
//     responder. retry_policy{retry_on:"5xx", num_retries:3}. A single GET /exhaust
//     attempts the request, retries it 3 times (every attempt hits the same 503
//     host), exhausts the cap, and returns 503 to the client. This arm is
//     CROSS-SIDE-EXACT (no round-robin spread → no offset nondeterminism):
//     upstream_rq_retry == numRetries (3), upstream_rq_retry_limit_exceeded == 1,
//     upstream_rq_total == numRetries+1 (4).
//
//   - RECOVER (/recover → c_recover): the cluster has TWO hosts (the 503
//     responder and a healthy HTTPEcho), lb_policy ROUND_ROBIN.
//     retry_policy{retry_on:"5xx",
//     num_retries:1}. A GET /recover that first picks the 503 host retries ONCE;
//     the retry re-picks via the LB → the OTHER host (echo) → 200. A request that
//     first picks echo → 200 immediately (no retry). So with K=recoverReqs
//     requests, ALL recover to downstream 200 REGARDLESS of the randomized
//     round-robin initial offset. The CROSS-SIDE invariant is the offset-INVARIANT
//     http.<statPrefix>.downstream_rq_2xx delta == recoverReqs +
//     c_recover.upstream_rq_retry_limit_exceeded == 0. The EXACT per-host retry
//     count is NOT cross-side-deterministic (reference_round_robin_offset_
//     randomized) — it is asserted SUBJECT-SIDE only
//     (c_recover.upstream_rq_retry_success == c_recover.upstream_rq_retry && > 0).
//
// # Topology: 1 ALWAYS-503 + 1 HEALTHY (all runner-spawned)
//
//   - backend0 → HTTP503Responder (BackendKind 35; always 503; the retry target)
//
//   - backend1 → HTTPEcho (BackendKind 1; 200s every path; the recover landing)
//
//   - c_exhaust: lb_policy ROUND_ROBIN (single host), endpoints = [backend0]
//
//   - c_recover: lb_policy ROUND_ROBIN, endpoints = [backend0, backend1]
//
//   - /exhaust → c_exhaust, retry_policy{retry_on:"5xx", num_retries:3}
//
//   - /recover → c_recover, retry_policy{retry_on:"5xx", num_retries:1}
//
//   - Subject (envoy-go): type STATIC, endpoints = 127.0.0.1:<...>.
//
//   - Reference (Envoy): type STRICT_DNS, endpoints = host.docker.internal:<...>
//     (the 0069 reference shape; the reference MUST be STRICT_DNS over the bridge).
//
// # The driver: drive BOTH sides + delta the retry stats (the 0069 cross-side pattern)
//
// The runner's hooks are DriveReference/DriveSubject (the byte-equiv stream, run
// FIRST) then AssertStats (run LAST, holding BOTH admin addrs). All the measured
// work runs inside AssertStats (the only hook holding both admin addrs). The Drive
// hooks STASH their listener addrs and return a fixed "READY\n" for the runner's
// CompareBytes gate. The retry loop is SYNCHRONOUS (the GET returns only after all
// retries complete — the backoff is delay-only, changing WHEN not WHETHER/HOW-MANY),
// so NO sleep is ever needed; the assertion is COUNT-based
// (reference_differential_band_sigma_margin).
//
//	AssertStats (per side: addr=listener, adminAddr):
//	 1. baseline scrapeStats(adminAddr).
//	 2. EXHAUSTION: 1 GET /exhaust → assert 503.
//	 3. RECOVER: recoverReqs GET /recover → assert 200 each.
//	 4. fin scrapeStats(adminAddr); delta-assert:
//	      EXHAUSTION (cross-side EXACT):
//	        c_exhaust.upstream_rq_retry                 == numRetries (3)
//	        c_exhaust.upstream_rq_retry_limit_exceeded  == 1
//	        c_exhaust.upstream_rq_total                 == numRetries+1 (4)
//	        c_exhaust.upstream_rq_total                 > 0 (decode-ran guard, ref)
//	      RECOVER (cross-side offset-INVARIANT):
//	        http.<statPrefix>.downstream_rq_2xx (delta) == recoverReqs
//	        c_recover.upstream_rq_retry_limit_exceeded  == 0
//	      RECOVER (SUBJECT-side only — exact retry count not cross-side):
//	        c_recover.upstream_rq_retry_success == c_recover.upstream_rq_retry && > 0
//
// # Cross-references
//
//   - phase 42.1 SPEC / PLAN Task 9 (the fixture design).
//   - 0069-outlier-detection-consecutive-5xx (the per-host-503 + ROUND_ROBIN +
//     cross-side StatsAsserter + scrapeStats/assertDelta template).
//   - 0074-circuit-breaker-max-requests (the 2-cluster bootstrap shape skim).
//   - reference_differential_asserter_dispatch (cross-side assertions via the
//     StatsAsserter path — NOT SubjectAsserter, which only runs reference-less).
//   - reference_round_robin_offset_randomized (the reference randomizes the RR
//     initial offset → the recover-arm exact retry count is subject-side-only; the
//     cross-side recover invariant is downstream_rq_2xx == K).
//   - reference_fixture_workload_constant_desync (constants single-sourced — D-S42-6).
//   - reference_differential_run_selector (target -run 'TestDifferential/0075').
//   - reference_docker_probe_bridge_network (shared bridge + STRICT_DNS hostnames;
//     the upstream_rq_total > 0 "decode ran" guard).
package driver

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0075-retry-loop"

	// In-container reference Envoy listener port for l_http. Fixtures run
	// sequentially; a distinct value avoids confusion (0074 took 19163, this
	// takes the next-free 19164 — D-S42-6).
	refContainerListenerPort = 19164

	refAdminPort = 9901

	// statPrefix is the HCM listener stat_prefix (single-sourced: the bootstrap +
	// the http.<statPrefix>.downstream_rq_2xx assertion read this).
	statPrefix = "ingress_http"

	// Cluster names (single-sourced: the bootstrap + the stat keys read these).
	clusterExhaust = "c_exhaust" // single 503 host; /exhaust; num_retries:3
	clusterRecover = "c_recover" // 503 + echo, RR; /recover; num_retries:1

	// backendCount is the total number of runner-spawned backends: host0 is the
	// always-503 responder, host1 is a healthy HTTPEcho.
	backendCount = 2

	// numRetries is the /exhaust route's num_retries (the static retry cap). The
	// exhaustion arm is cross-side-EXACT: a single GET /exhaust attempts once +
	// retries numRetries times against the same 503 host → numRetries+1 total
	// attempts, the cap is exceeded, the client gets 503.
	numRetries = 3

	// recoverRetries is the /recover route's num_retries. It MUST be exactly 1: the
	// recover-arm invariant (retry-once-onto-fresh-host → limit_exceeded == 0) holds
	// only because the single retry always lands on the OTHER (healthy) host. Larger
	// values would break the offset-invariant cross-side assertion.
	recoverRetries = 1

	// recoverReqs is the K recover-arm request count. EVEN (D-S42-6: a balanced
	// round-robin spread regardless of the randomized initial offset). With 2
	// hosts RR + num_retries:1, EVERY request recovers to a downstream 200 (a
	// 503-first request retries onto the OTHER host → echo → 200; an echo-first
	// request 200s immediately) — the offset-invariant the cross-side assertion
	// pins. K need not be large: the assertion is delta downstream_rq_2xx == K,
	// not a distribution band.
	recoverReqs = 8

	// Stat keys (single-sourced).
	statExhaustRetry         = "cluster." + clusterExhaust + ".upstream_rq_retry"
	statExhaustRetryLimitExc = "cluster." + clusterExhaust + ".upstream_rq_retry_limit_exceeded"
	statExhaustRqTotal       = "cluster." + clusterExhaust + ".upstream_rq_total"
	statRecoverRetry         = "cluster." + clusterRecover + ".upstream_rq_retry"
	statRecoverRetrySuccess  = "cluster." + clusterRecover + ".upstream_rq_retry_success"
	statRecoverRetryLimitExc = "cluster." + clusterRecover + ".upstream_rq_retry_limit_exceeded"
	statDownstreamRq2xx      = "http." + statPrefix + ".downstream_rq_2xx"
)

func init() {
	fixture.RegisterFixture(fixtureName, &retryDriver{})
}

// retryDriver is STATEFUL: the Drive hooks stash the per-side listener addrs (the
// reference listener mapped port is only knowable at DriveReference; the subject
// listener at SubjectConfig) so AssertStats — the only hook holding BOTH admin
// addrs — can drive both arms and delta-assert the retry stats.
type retryDriver struct {
	mu           sync.Mutex
	refListener  string // host:port of the reference l_http listener (from DriveReference)
	subjListener string // 127.0.0.1:<port> of the subject l_http listener (from SubjectConfig)
}

func (*retryDriver) BackendCount() int { return backendCount }

// BackendKindAt implements fixture.PerHostBackendKind: host 0 is the always-503
// responder (the retry target), host 1 is a healthy HTTPEcho (the recover host).
func (*retryDriver) BackendKindAt(i int) fixture.BackendKind {
	if i == 0 {
		return fixture.HTTP503Responder
	}
	return fixture.HTTPEcho
}

func (*retryDriver) SubjectListenerName() string { return "l_http" }
func (*retryDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

// routeTable routes /exhaust → c_exhaust (num_retries:3) and /recover → c_recover
// (num_retries:1). Identical on both sides (the retry_policy is static config;
// the route MATCH order puts the specific prefixes before any catch-all — there
// is no catch-all here, both prefixes are explicit). retry_on:"5xx" matches the
// always-503 host's responses.
var routeTable = fmt.Sprintf(`                      routes:
                        - match: { prefix: "/exhaust" }
                          route:
                            cluster: %s
                            retry_policy: { retry_on: "5xx", num_retries: %d }
                        - match: { prefix: "/recover" }
                          route:
                            cluster: %s
                            retry_policy: { retry_on: "5xx", num_retries: %d }`,
	clusterExhaust, numRetries, clusterRecover, recoverRetries)

func (d *retryDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	// STRICT_DNS + host.docker.internal (the 0069 reference shape). c_exhaust over
	// the single 503 host; c_recover over the 503 host + the healthy echo host.
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
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, refAdminPort, refContainerListenerPort, statPrefix, routeTable,
		clusterExhaust, clusterExhaust, backendPorts[0],
		clusterRecover, clusterRecover, backendPorts[0], backendPorts[1])
}

func (d *retryDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	// STATIC + 127.0.0.1 (the 0069 subject shape). Same cluster/route topology.
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0075, cluster: envoy-go-differential }
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
    - name: %s
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: %s
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, statPrefix, routeTable,
		clusterExhaust, clusterExhaust, backendPorts[0],
		clusterRecover, clusterRecover, backendPorts[0], backendPorts[1])
}

// DriveReference stashes the reference listener addr and returns the fixed
// byte-equiv stream. The real work runs in AssertStats.
func (d *retryDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

// DriveSubject returns the fixed byte-equiv stream (the subject listener addr was
// already stashed in SubjectConfig). The real work runs in AssertStats.
func (d *retryDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0069 raw
// /ready probe, verbatim).
func (*retryDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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
// and HTTP503Responder use this format) and returns the embedded backend idx.
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

// AssertStats drives BOTH arms on BOTH sides and delta-asserts the retry stats
// (the only hook holding both admin addrs; the 0069 cross-side pattern). See the
// package doc for the per-side flow.
func (d *retryDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ctx := context.Background()

	d.mu.Lock()
	refListener := d.refListener
	subjListener := d.subjListener
	d.mu.Unlock()
	if refListener == "" || subjListener == "" {
		t.Fatalf("listener addrs not stashed (ref=%q subj=%q) — Drive hooks did not run?", refListener, subjListener)
	}

	for _, sd := range []struct {
		side     string
		listener string
		admin    string
		subject  bool
	}{
		{"reference", refListener, refAdminAddr, false},
		{"subject", subjListener, subjAdminAddr, true},
	} {
		base, err := scrapeStats(sd.admin)
		if err != nil {
			t.Fatalf("%s: scrape baseline /stats: %v", sd.side, err)
		}

		// EXHAUSTION: 1 GET /exhaust → 503 (single 503 host, retries exhausted).
		resp, _, err := helpers.HTTPRoundTrip(ctx, sd.listener, "GET", "/exhaust", nil, nil)
		if err != nil {
			t.Fatalf("%s: GET /exhaust: %v", sd.side, err)
		}
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s: GET /exhaust status %d, want 503 (retries should exhaust against the single 503 host)", sd.side, resp.StatusCode)
		}

		// RECOVER: recoverReqs GET /recover → all 200 (the retry re-picks the
		// other host on a 503-first request; offset-invariant).
		for i := 0; i < recoverReqs; i++ {
			rresp, body, rerr := helpers.HTTPRoundTrip(ctx, sd.listener, "GET", "/recover", nil, nil)
			if rerr != nil {
				t.Fatalf("%s: GET /recover[%d]: %v", sd.side, i, rerr)
			}
			if rresp.StatusCode != http.StatusOK {
				t.Errorf("%s: GET /recover[%d] status %d, want 200 (the 503-first request must retry onto the healthy host — retry host re-pick parity)", sd.side, i, rresp.StatusCode)
			}
			// A 200 must come from the healthy echo host (idx 1), never the 503
			// host (idx 0). The 503 host never produces a 200 body; this also
			// proves the retry landed on a FRESH host.
			if rresp.StatusCode == http.StatusOK {
				idx, berr := backendIdxFromBody(body)
				if berr != nil {
					t.Errorf("%s: GET /recover[%d]: %v", sd.side, i, berr)
				} else if idx != 1 {
					t.Errorf("%s: GET /recover[%d]: 200 served by host%d, want host1 (the healthy echo)", sd.side, i, idx)
				}
			}
		}

		fin, err := scrapeStats(sd.admin)
		if err != nil {
			t.Fatalf("%s: scrape final /stats: %v", sd.side, err)
		}

		// "decode ran" guard (reference_docker_probe_bridge_network): the ref
		// container must have reached the backends over the bridge.
		if sd.side == "reference" && fin[statExhaustRqTotal] == 0 {
			t.Fatalf("reference did NOT decode: %s == 0 (container could not reach backends — bridge network / host.docker.internal?)", statExhaustRqTotal)
		}

		// EXHAUSTION asserts (cross-side EXACT — single host, no RR offset).
		assertDelta(t, sd.side, fin, base, statExhaustRetry, numRetries)     // 3
		assertDelta(t, sd.side, fin, base, statExhaustRetryLimitExc, 1)      // exhausted once
		assertDelta(t, sd.side, fin, base, statExhaustRqTotal, numRetries+1) // 4 attempts

		// RECOVER asserts (cross-side offset-INVARIANT).
		assertDelta(t, sd.side, fin, base, statDownstreamRq2xx, recoverReqs) // every client recovered
		// limit_exceeded == 0 is safe CROSS-SIDE (unlike the exact retry count,
		// which is subject-only): no /recover request ever exhausts its 1-retry
		// budget because the single retry always lands on the fresh healthy host,
		// so this holds regardless of the reference's randomized RR offset — keep
		// it here, NOT in the subject-only block below.
		assertDelta(t, sd.side, fin, base, statRecoverRetryLimitExc, 0) // none exhausted

		// RECOVER subject-side only: the EXACT retry count is NOT cross-side
		// (reference_round_robin_offset_randomized) — the reference randomizes the
		// RR initial offset, so the number of 503-first requests (== retries) is
		// not deterministic cross-side. Subject-side, every retry recovered (the
		// retry re-picked the healthy host), so retry_success == retry, and at
		// least one request hit the 503 host first (so retry > 0).
		if sd.subject {
			gotRetry := fin[statRecoverRetry] - base[statRecoverRetry]
			gotSuccess := fin[statRecoverRetrySuccess] - base[statRecoverRetrySuccess]
			if gotRetry == 0 {
				t.Errorf("subject: %s delta == 0, want > 0 (with %d RR requests over 2 hosts, some must pick the 503 host first and retry)", statRecoverRetry, recoverReqs)
			}
			if gotSuccess != gotRetry {
				t.Errorf("subject: %s delta (%d) != %s delta (%d) — every retry must recover onto the healthy host (retry host re-pick)", statRecoverRetrySuccess, gotSuccess, statRecoverRetry, gotRetry)
			}
		}
	}
}

// assertDelta asserts (final[key] - base[key]) == want — the change in a counter
// over the measured phase. Absent keys read as 0 (reference Envoy lazily allocates
// per-response-class counters), so a 0-want passes when the class was never
// touched in either scrape.
func assertDelta(t fixture.TB, side string, st, base map[string]uint64, key string, want uint64) {
	t.Helper()
	got := st[key] - base[key]
	if got != want {
		t.Errorf("%s: %s delta = %d, want %d (final %d, base %d)", side, key, got, want, st[key], base[key])
	}
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0069 driver scrapeStats,
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
	_ fixture.Driver             = (*retryDriver)(nil)
	_ fixture.StatsAsserter      = (*retryDriver)(nil)
	_ fixture.PerHostBackendKind = (*retryDriver)(nil)
)
