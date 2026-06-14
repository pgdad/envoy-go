// Package driver registers the 0065-weighted-clusters cross-side differential
// fixture (phase 38.2 SPEC D-WC-IMPL-3 / PLAN Task 8).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture over THREE
// clusters — c_a (weight 50), c_b (weight 30), c_sub (weight 20, with
// lb_subset_config + ClusterWeight.metadata_match{version:v1}) — that proves
// RouteAction.weighted_clusters routing + WeightedCluster.ClusterWeight.
// metadata_match composition on BOTH sides (the 0003/0064 HTTP shape: reference
// STRICT_DNS / host.docker.internal, subject STATIC / 127.0.0.1). c_sub is a
// cluster with lb_subset_config{fallback_policy:ANY_ENDPOINT,
// subset_selectors:[{keys:[version]}]} over backend2(version:v1) +
// backend3(version:v2). The ClusterWeight.metadata_match{version:v1} applied at
// the routing plane constrains c_sub to ONLY the v1 subset (backend2); the
// composition affinity forces backend3==0.
//
// # Topology: 4 backends / 3 clusters
//
//   - backend0 → c_a         (weight 50; the heavyweight direct cluster)
//   - backend1 → c_b         (weight 30; the mid-weight direct cluster)
//   - backend2 → c_sub/v1   (weight 20 via c_sub; v1 subset — the ONLY routed sub-member)
//   - backend3 → c_sub/v2   (weight  0 effectively; excluded by metadata_match composition)
//
// # The workload (per side)
//
// n=500 GET /w (tally served "backend-<idx>" bodies — the distribution sample)
// + healthReqs=8 GET /health (direct_response "OK\n" — the byte-equiv stream).
// /health NEVER reaches a backend (direct_response) — excluded from the routed sum.
//
// # Distribution assertion (per-side band)
//
// The runner's aggregate AssertDistribution channel sees per-backend accept TOTALS
// across the WHOLE workload. /w is the ONLY backend-routed traffic; /health is
// direct_response. Per-side bands (n=500, ~4.5σ margin):
//
//	backend0 (c_a,  p=0.50): mean 250 σ≈11.2 → [200, 300]
//	backend1 (c_b,  p=0.30): mean 150 σ≈10.3 → [104, 196]
//	backend2 (c_sub/v1, p=0.20): mean 100 σ≈8.9 → [60, 140]
//	backend3 (c_sub/v2): ==0 (composition affinity: metadata_match{version:v1} excludes v2)
//
// Both sides run INDEPENDENT RNG streams → per-request picks differ, but the
// aggregate distribution matches the weights on each side. NEVER assert cross-side
// per-request equality (reference_differential_hash_key_cross_side_infeasible).
//
// # Stats assertion (cross-EQUAL conservation, per-cluster)
//
// Σ of cluster.{c_a,c_b,c_sub}.upstream_rq_total == n on each side (HTTP router
// charges rq_total once per upstream request). Each cluster's upstream_cx_active==0
// (quiesced). The per-cluster split is PER-SIDE (RNG → not cross-equal); only the
// conservation Σ is asserted.
//
// # Cross-references
//
//   - phase 38.2 SPEC D-WC-IMPL-3 (the constants / bands / composition affinity).
//   - 0064-lb-subset (the cross-side HTTP-route shape: reference STRICT_DNS /
//     host.docker.internal, subject STATIC / 127.0.0.1; the HTTPEcho backend
//     echoing "backend-<idx>:<seg>"; the routeTable + Bootstrap/Config builders).
//   - 0060-lb-random (the PER-SIDE band asserter structure: σ-margin reasoning,
//     per-side independence, conservation check).
//   - reference_differential_band_sigma_margin (~4.5σ margins for flake-free bands).
//   - reference_differential_hash_key_cross_side_infeasible (per-side-only bands).
//   - reference_fixture_workload_constant_desync (totalReqs DERIVED from n).
//   - reference_docker_probe_bridge_network (the stats prong verifies decode ran).
//   - reference_differential_run_selector (target -run 'TestDifferential/0065').
package driver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0065-weighted-clusters"

	// In-container reference Envoy listener port for l_http. Fixtures run
	// sequentially; a distinct value avoids confusion — 0064-lb-subset takes
	// 19153, this takes 19154.
	refContainerListenerPort = 19154

	refAdminPort = 9901

	backendCount = 4 // backend0=c_a, backend1=c_b, backend2=c_sub/v1, backend3=c_sub/v2

	n          = 500 // GET /w per side (the distribution sample)
	healthReqs = 8

	// Cluster weights (documentation only — load-bearing as route config below).
	// wA=50, wB=30, wSub=20 → the weighted_clusters total weight = 100 (implicit).
	// p(c_a)=0.50, p(c_b)=0.30, p(c_sub)=0.20 — c_sub routes ONLY to v1 (backend2).

	// Per-backend PER-SIDE bands: mean = n·p, σ = √(n·p·(1−p)), ~4.5σ margin.
	//   backend0 p=.50 mean 250 σ≈11.2 → [200,300]   (c_a weight 50)
	//   backend1 p=.30 mean 150 σ≈10.25 → [104,196]  (c_b weight 30)
	//   backend2 p=.20 mean 100 σ≈8.94 → [60,140]    (c_sub weight 20, all → v1)
	//   backend3        mean 0          → ==0          (composition affinity)
	b0Lo, b0Hi = 200, 300
	b1Lo, b1Hi = 104, 196
	b2Lo, b2Hi = 60, 140

	totalReqs = n // DERIVED; /health is direct_response (no backend)
)

func init() {
	fixture.RegisterFixture(fixtureName, &weightedDriver{})
}

type weightedDriver struct{}

func (weightedDriver) BackendCount() int                { return backendCount }
func (weightedDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (weightedDriver) SubjectListenerName() string      { return "l_http" }
func (weightedDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// routeTable is the shared virtual_hosts.routes block:
//   - /w  → weighted_clusters: c_a(50)/c_b(30)/c_sub(20, metadata_match{version:v1})
//   - /health → direct_response "OK\n" (address-independent byte-equiv stream)
//
// Identical on both sides (NAT-transparent static config).
const routeTable = `                      routes:
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                        - match: { path: "/w" }
                          route:
                            weighted_clusters:
                              clusters:
                                - name: c_a
                                  weight: 50
                                - name: c_b
                                  weight: 30
                                - name: c_sub
                                  weight: 20
                                  metadata_match:
                                    filter_metadata:
                                      "envoy.lb": { version: "v1" }`

func (weightedDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	// STRICT_DNS + host.docker.internal (the 0064 reference shape).
	// Three clusters:
	//   c_a  → backend0 (ROUND_ROBIN, no subset)
	//   c_b  → backend1 (ROUND_ROBIN, no subset)
	//   c_sub → backend2(v1) + backend3(v2) (ROUND_ROBIN + lb_subset_config,
	//           fallback_policy:ANY_ENDPOINT so Task-9 break (c) bites).
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
    - name: c_a
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_a
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
    - name: c_b
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_b
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
    - name: c_sub
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      lb_subset_config:
        fallback_policy: ANY_ENDPOINT
        subset_selectors:
          - keys: ["version"]
      load_assignment:
        cluster_name: c_sub
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v1" } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v2" } } }
`, refAdminPort, refContainerListenerPort, routeTable, backendPorts[0], backendPorts[1], backendPorts[2], backendPorts[3])
}

func (weightedDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	// STATIC + 127.0.0.1 (the 0064 subject shape).
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0065, cluster: envoy-go-differential }
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
    - name: c_a
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_a
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
    - name: c_b
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_b
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
    - name: c_sub
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      lb_subset_config:
        fallback_policy: ANY_ENDPOINT
        subset_selectors:
          - keys: ["version"]
      load_assignment:
        cluster_name: c_sub
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v1" } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v2" } } }
`, subjAdminPort, subjListenerPort, routeTable, backendPorts[0], backendPorts[1], backendPorts[2], backendPorts[3])
}

// backendIdxFromBody parses the HTTPEcho canned body "backend-<idx>:<seg>" and
// returns the embedded backend idx.
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

// drive sends n GET /w requests (tallied into counts by backend idx) then
// healthReqs GET /health requests (returning ONLY the address-independent
// /health bytes for the runner's CompareBytes gate). The /w tally is NOT
// returned to drive() callers — the runner's aggregate AssertDistribution
// channel receives per-backend TOTALS from the backend accept counters, which
// match the /w distribution because /health is direct_response (no backend
// accept).
func drive(ctx context.Context, addr string) ([]byte, error) {
	// Send n GET /w; each response body embeds the backend idx.
	for i := 0; i < n; i++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/w", nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/w[%d]: %w", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("/w[%d]: status %d, want 200", i, resp.StatusCode)
		}
		// Parse the body to verify it came from a valid backend (structural check
		// only — band correctness is proven by AssertDistribution post-drive).
		idx, err := backendIdxFromBody(body)
		if err != nil {
			return nil, fmt.Errorf("/w[%d]: %w", i, err)
		}
		if idx < 0 || idx >= backendCount {
			return nil, fmt.Errorf("/w[%d]: backend idx %d out of range [0,%d)", i, idx, backendCount)
		}
	}

	// Byte-equiv stream: healthReqs /health direct_response bodies ("OK\n"),
	// address-independent → byte-equal cross-side. /health NEVER reaches a backend.
	var b bytes.Buffer
	for i := 0; i < healthReqs; i++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/health", nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/health[%d]: %w", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("/health[%d]: status %d, want 200", i, resp.StatusCode)
		}
		b.Write(body)
	}
	return b.Bytes(), nil
}

func (weightedDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

func (weightedDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0064
// raw-socket /ready probe, verbatim).
func (weightedDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertDistribution: PER-SIDE weighted band on the per-backend accept counts.
// The runner snapshots backend accept TOTALS after Drive — each accept on the
// HTTPEcho backend counts one /w request (direct_response /health never
// increments the accept counter). NEVER cross-side-exact (independent RNG streams
// on each side — reference_differential_hash_key_cross_side_infeasible).
//
// Per-side invariants:
//
//	band:         backend[0] ∈ [b0Lo,b0Hi], backend[1] ∈ [b1Lo,b1Hi], backend[2] ∈ [b2Lo,b2Hi]
//	composition:  backend[3] == 0 (c_sub/v2 excluded by ClusterWeight.metadata_match{version:v1})
//	conservation: Σ counts == n (totalReqs; all n /w requests route to a backend)
func (weightedDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	for _, sd := range []struct {
		side   string
		counts []uint64
	}{{"reference", refCounts}, {"subject", subjCounts}} {
		if len(sd.counts) != backendCount {
			return fmt.Errorf("%s: expected %d backend counts, got %d", sd.side, backendCount, len(sd.counts))
		}
		band := func(i, lo, hi int) error {
			if int(sd.counts[i]) < lo || int(sd.counts[i]) > hi {
				return fmt.Errorf("%s: backend[%d]=%d outside weighted band [%d,%d] (swapped weights? dropped cluster?)", sd.side, i, sd.counts[i], lo, hi)
			}
			return nil
		}
		for _, e := range []struct{ i, lo, hi int }{{0, b0Lo, b0Hi}, {1, b1Lo, b1Hi}, {2, b2Lo, b2Hi}} {
			if err := band(e.i, e.lo, e.hi); err != nil {
				return err
			}
		}
		if sd.counts[3] != 0 { // backend3 = c_sub/v2: the composition affinity (merged match → v1 only)
			return fmt.Errorf("%s: backend[3] (v2) = %d, want 0 (ClusterWeight.metadata_match{version:v1} must exclude v2)", sd.side, sd.counts[3])
		}
		var sum uint64
		for _, c := range sd.counts {
			sum += c
		}
		if sum != totalReqs {
			return fmt.Errorf("%s: conservation: routed sum %d != %d", sd.side, sum, totalReqs)
		}
	}
	return nil
}

// AssertStats (post-drive): cross-EQUAL the conservation sum across all three
// clusters (Σ upstream_rq_total == n on each side) + each cluster's
// upstream_cx_active==0 (quiesced). The per-cluster split is PER-SIDE (RNG →
// NOT cross-equaled). The "decode ran" guard (reference_docker_probe_bridge_network)
// verifies the reference actually forwarded at least one request.
func (weightedDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	ref, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}

	clusters := []string{"c_a", "c_b", "c_sub"}

	// "decode ran" guard: if the reference container failed to reach ANY backend,
	// the sum below will be zero and the readout is untrustworthy.
	var refSum uint64
	for _, cl := range clusters {
		refSum += ref["cluster."+cl+".upstream_rq_total"]
	}
	if refSum == 0 {
		t.Fatalf("reference did NOT decode: Σ upstream_rq_total == 0 (container could not reach backends — bridge network / host.docker.internal?)")
	}

	// Conservation Σ: both sides must route exactly n requests to backends.
	var subjSum uint64
	for _, cl := range clusters {
		subjSum += subj["cluster."+cl+".upstream_rq_total"]
	}
	if refSum != uint64(n) {
		t.Errorf("ref Σ upstream_rq_total = %d, want %d", refSum, n)
	}
	if subjSum != uint64(n) {
		t.Errorf("subj Σ upstream_rq_total = %d, want %d", subjSum, n)
	}

	// Per-cluster upstream_cx_active == 0 (quiesced; Connection: close means
	// each request is a fresh dial that completes before the next).
	for _, cl := range clusters {
		key := "cluster." + cl + ".upstream_cx_active"
		if rv, ok := ref[key]; !ok {
			t.Errorf("ref: %s ABSENT in /stats", key)
		} else if rv != 0 {
			t.Errorf("ref %s = %d, want 0 (not quiesced?)", key, rv)
		}
		if sv, ok := subj[key]; !ok {
			t.Errorf("subj: %s ABSENT in /stats", key)
		} else if sv != 0 {
			t.Errorf("subj %s = %d, want 0 (not quiesced?)", key, sv)
		}
	}
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0064 driver scrapeStats,
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
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read %s body: %w", url, err)
	}

	out := make(map[string]uint64)
	for _, line := range strings.Split(buf.String(), "\n") {
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
	_ fixture.Driver               = (*weightedDriver)(nil)
	_ fixture.DistributionAsserter = (*weightedDriver)(nil)
	_ fixture.StatsAsserter        = (*weightedDriver)(nil)
	_ fixture.BackendKindAware     = (*weightedDriver)(nil)
)
