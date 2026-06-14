// Package driver registers the 0064-lb-subset cross-side differential fixture
// (phase 38.1 SPEC §8.1 / 38.1 PLAN Task 11).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture over ONE
// 4-endpoint cluster with lb_policy: ROUND_ROBIN + lb_subset_config:
// {fallback_policy: ANY_ENDPOINT, subset_selectors: [{keys: ["version"]}]} +
// 4 endpoints tagged with envoy.lb metadata (version: v1 ×2 — the first two
// endpoints idx 0,1; version: v2 ×2 — idx 2,3) on BOTH sides (the 0003 HTTP
// shape: reference STRICT_DNS / host.docker.internal, subject STATIC /
// 127.0.0.1). It is the end-to-end proof of the subset plane (the HTTP route
// RouteAction.metadata_match["envoy.lb"] producer → cluster.SubsetMatch threaded
// onto ctx at dispatch → subsetLB.Pick → the matching version subset's
// ROUND_ROBIN leaf → within-subset affinity).
//
// # The KEY insight vs maglev/ring_hash (0061/0062/0063): NAT-transparent affinity
//
// The subset key is STATIC ROUTE CONFIG (RouteAction.metadata_match), IDENTICAL on
// both sides — it is NOT a wire-derived hash key. So it is NAT-transparent in the
// STRONGEST sense: not only does the key survive the Docker hop, the version→idx
// MAP is the SAME on both sides (the driver BUILDS both bootstraps, tagging
// endpoints idx 0,1 = v1 and idx 2,3 = v2 in the SAME order the runner spawns the
// backends, and the HTTPEcho backend embeds its OWN idx in the body
// `backend-<idx>:<seg>`). So SET-MEMBERSHIP affinity holds TRUE on BOTH sides AND
// is host-ATTRIBUTABLE per side — strictly cleaner than the per-side modular
// invariant maglev/ring_hash needed (reference_differential_hash_key_cross_side_
// infeasible is INVERTED here: identity IS feasible because the map is static
// config, not an address-keyed hash table).
//
// The driver sends, per side: K=16 GET /v1 + K=16 GET /v2 + K=16 GET /none
// (the ANY_ENDPOINT fallback arm — version "v9" matches NO subset) = 3*K routed
// GETs, then 8 GET /health. totalReqs is DERIVED from the named constants
// (perRoute, healthReqs) — never a literal (reference_fixture_workload_constant_
// desync).
//
// Per-route SET-membership / within-subset spread / fallback spread are asserted
// INSIDE drive() by parsing each routed response body's embedded backend idx (the
// runner's aggregate AssertDistribution channel only sees per-backend TOTALS across
// the whole workload, so it cannot attribute by route — it carries the
// CONSERVATION prong instead). The asserted behaviors:
//
//   - SET-membership affinity: every backend serving a /v1 request ∈ {0,1}; every
//     /v2 request ∈ {2,3}. 100% deterministic under metadata_match — a leak across
//     the subset boundary fails the drive.
//   - within-subset spread: both members of each 2-host subset are hit across K=16
//     (ROUND_ROBIN alternates → ≥1 each member).
//   - fallback spread: /none (ANY_ENDPOINT over all 4 hosts) hits ≥2 of the 4
//     backends.
//   - conservation (AssertDistribution): the routed per-backend counts sum to the
//     routed total (3*perRoute) on each side; /health never reaches a backend
//     (direct_response) — accounted for explicitly (the routed sum EXCLUDES the 8
//     health round-trips).
//
// The /health direct_response ("OK\n") bodies are the ONLY bytes returned to the
// runner's CompareBytes gate — address-independent → byte-equal cross-side (the
// 0003 byte-equiv precedent). The routed bodies are NOT concatenated: ROUND_ROBIN
// alternates within a subset, so the per-request order of idx 0 vs 1 (and the
// fallback order over all 4) may differ cross-side even though the SET is identical.
//
// The cross-side StatsAsserter prong (SPEC §8.1): cross-EQUAL upstream_cx_total /
// upstream_rq_total (= routed total = 3*perRoute) / membership_total (= 4) /
// upstream_cx_active (= 0) / lb_subsets_selected (= /v1+/v2 = 2*perRoute) /
// lb_subsets_fallback (= /none = perRoute) — these are REQUEST-COUNTED so they
// cross-equal. NOT cross-equaled: lb_subsets_active / lb_subsets_created (the
// reference contrib build's value differs from envoy-go's ×1-per-distinct-subset
// accounting) — instead UNIT-assert the SUBJECT side's lb_subsets_active == 2 (the
// 2 version subsets) separately. Per reference_docker_probe_bridge_network the
// stats prong FIRST verifies the reference actually decoded (upstream_cx_total > 0).
//
// NO new BackendKind (reuses HTTPEcho — the 0003 backend; the tail STAYS 33). NO
// new fuzzer (the subset key is static route config, not an untrusted wire frame;
// the subset enumeration/Pick property tests are UNIT-level).
//
// # Cross-references
//
//   - phase 38.1 SPEC §8.1 (the subset routing plane) + the cross-side stats set.
//   - 0063-lb-maglev (the dir-layout + harness template — STRICT_DNS reference /
//     STATIC subject, the /health direct_response byte-equiv prong, the HTTPEcho
//     backend, the per-request fresh-dial accept accounting, the StatsAsserter
//     scrape-and-diff shape).
//   - reference_differential_hash_key_cross_side_infeasible (INVERTED here — the
//     metadata_match is static config → SET-membership affinity is host-attributable
//     per side, identity IS feasible).
//   - reference_fixture_workload_constant_desync (totalReqs DERIVED from constants).
//   - reference_docker_probe_bridge_network (the stats prong verifies the reference
//     decoded — upstream_cx_total > 0 — before trusting the readout).
//   - reference_differential_run_selector (target -run 'TestDifferential/0064').
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
	fixtureName = "0064-lb-subset"

	// In-container reference Envoy listener port for l_http. Fixtures run
	// sequentially so the in-container port may be reused, but a distinct value
	// avoids confusion — 0063-lb-maglev takes 19152, this takes 19153.
	refContainerListenerPort = 19153

	refAdminPort = 9901

	clusterName = "c_echo"

	backendCount = 4 // 4 endpoints: idx 0,1 → v1; idx 2,3 → v2

	perRoute   = 16                // K — GETs per subset/fallback route
	routes     = 3                 // /v1 + /v2 + /none (the routed routes)
	totalReqs  = routes * perRoute // 48 — the routed conservation target (DERIVED)
	healthReqs = 8                 // /health direct_response round-trips (byte-equiv stream)

	selectedReqs = 2 * perRoute // /v1 + /v2 → lb_subsets_selected (32)
	fallbackReqs = perRoute     // /none    → lb_subsets_fallback (16)

	subjectActiveSubsets = 2 // the 2 version subsets (v1, v2) — subject-side unit assert
)

// The version→idx map. The driver BUILDS both bootstraps tagging endpoints in
// this exact order, and the runner spawns backends in backendPorts order, so the
// HTTPEcho body's embedded idx maps directly back here on BOTH sides.
var (
	v1Set = map[int]bool{0: true, 1: true}
	v2Set = map[int]bool{2: true, 3: true}
)

func init() {
	fixture.RegisterFixture(fixtureName, &subsetDriver{})
}

type subsetDriver struct{}

func (subsetDriver) BackendCount() int                { return backendCount }
func (subsetDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (subsetDriver) SubjectListenerName() string      { return "l_http" }
func (subsetDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// routeTable is the shared virtual_hosts.routes block — /v1, /v2 → version
// subsets via metadata_match["envoy.lb"]; /none → version "v9" matching NO
// subset → the ANY_ENDPOINT fallback; /health → direct_response (address-
// independent byte-equiv stream). Identical on both sides (NAT-transparent).
const routeTable = `                      routes:
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                        - match: { path: "/v1" }
                          route:
                            cluster: c_echo
                            metadata_match:
                              filter_metadata:
                                "envoy.lb": { version: "v1" }
                        - match: { path: "/v2" }
                          route:
                            cluster: c_echo
                            metadata_match:
                              filter_metadata:
                                "envoy.lb": { version: "v2" }
                        - match: { path: "/none" }
                          route:
                            cluster: c_echo
                            metadata_match:
                              filter_metadata:
                                "envoy.lb": { version: "v9" }`

func (subsetDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	// STRICT_DNS + host.docker.internal (the 0063 reference shape) + ROUND_ROBIN
	// + lb_subset_config (ANY_ENDPOINT fallback, version selector) + the 4 tagged
	// endpoints (idx 0,1 → v1; idx 2,3 → v2).
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
    - name: c_echo
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      lb_subset_config:
        fallback_policy: ANY_ENDPOINT
        subset_selectors:
          - keys: ["version"]
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v1" } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v1" } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v2" } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v2" } } }
`, refAdminPort, refContainerListenerPort, routeTable, backendPorts[0], backendPorts[1], backendPorts[2], backendPorts[3])
}

func (subsetDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	// STATIC + 127.0.0.1 (the 0063 subject shape) + ROUND_ROBIN + lb_subset_config
	// + the 4 tagged endpoints (idx 0,1 → v1; idx 2,3 → v2).
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0064, cluster: envoy-go-differential }
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
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      lb_subset_config:
        fallback_policy: ANY_ENDPOINT
        subset_selectors:
          - keys: ["version"]
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v1" } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v1" } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v2" } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
                metadata: { filter_metadata: { "envoy.lb": { version: "v2" } } }
`, subjAdminPort, subjListenerPort, routeTable, backendPorts[0], backendPorts[1], backendPorts[2], backendPorts[3])
}

// backendIdxFromBody parses the HTTPEcho canned body "backend-<idx>:<seg>" and
// returns the embedded backend idx. The runner spawns backends in backendPorts
// order, so the idx maps directly back to the version→idx map (v1Set/v2Set).
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

// sendRoute issues perRoute GET requests against path and returns the set of
// distinct backend idxs that served them (parsed from the HTTPEcho body). Every
// request must be 200.
func sendRoute(ctx context.Context, addr, path string) (map[int]bool, error) {
	hit := make(map[int]bool)
	for i := 0; i < perRoute; i++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", path, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", path, i, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s[%d]: status %d, want 200", path, i, resp.StatusCode)
		}
		idx, err := backendIdxFromBody(body)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", path, i, err)
		}
		hit[idx] = true
	}
	return hit, nil
}

// drive sends the subset workload (/v1, /v2, /none ×perRoute) — asserting
// SET-membership affinity + within-subset spread + fallback spread INLINE per side
// from the embedded backend idxs — then healthReqs /health direct_response
// round-trips, returning ONLY the (address-independent, byte-equal) /health bodies
// for the runner's CompareBytes gate. SHARED by DriveReference and DriveSubject:
// because the version→idx map is STATIC config, the membership invariant holds
// IDENTICALLY on both sides (the NAT-transparent-config insight).
func drive(ctx context.Context, addr string) ([]byte, error) {
	// /v1 → must land ONLY in v1Set {0,1}, and hit BOTH members (within-subset spread).
	v1hit, err := sendRoute(ctx, addr, "/v1")
	if err != nil {
		return nil, err
	}
	if err := assertSubsetMembership("/v1", v1hit, v1Set); err != nil {
		return nil, err
	}

	// /v2 → must land ONLY in v2Set {2,3}, and hit BOTH members.
	v2hit, err := sendRoute(ctx, addr, "/v2")
	if err != nil {
		return nil, err
	}
	if err := assertSubsetMembership("/v2", v2hit, v2Set); err != nil {
		return nil, err
	}

	// /none → version "v9" matches NO subset → ANY_ENDPOINT fallback over all 4
	// hosts → must hit >= 2 distinct backends (fallback spread).
	noneHit, err := sendRoute(ctx, addr, "/none")
	if err != nil {
		return nil, err
	}
	if len(noneHit) < 2 {
		return nil, fmt.Errorf("/none fallback spread: only %d backend(s) hit %v, want >= 2 (ANY_ENDPOINT collapsed?)", len(noneHit), sortedKeys(noneHit))
	}

	// Byte-equiv stream: healthReqs /health direct_response bodies ("OK\n"),
	// address-independent → byte-equal cross-side. /health NEVER reaches a backend.
	var b bytes.Buffer
	for n := 0; n < healthReqs; n++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/health", nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/health[%d]: %w", n, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("/health[%d]: status %d, want 200", n, resp.StatusCode)
		}
		b.Write(body)
	}
	return b.Bytes(), nil
}

// assertSubsetMembership verifies that every backend that served the route is a
// member of want (SET-membership affinity — a leak across the subset boundary is a
// hard fail) AND that ALL members of want were hit (within-subset spread —
// ROUND_ROBIN must alternate across the 2-host subset over perRoute=16 requests).
func assertSubsetMembership(route string, hit, want map[int]bool) error {
	for idx := range hit {
		if !want[idx] {
			return fmt.Errorf("%s affinity LEAK: backend[%d] served a %s request but is not in the subset %v (subset boundary breached)", route, idx, route, sortedKeys(want))
		}
	}
	for idx := range want {
		if !hit[idx] {
			return fmt.Errorf("%s within-subset spread: member backend[%d] never served (got %v, want all of %v) — ROUND_ROBIN not alternating?", route, idx, sortedKeys(hit), sortedKeys(want))
		}
	}
	return nil
}

func sortedKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// tiny n (<= 4) — insertion sort to avoid an import for determinism in messages
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func (subsetDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

func (subsetDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0003
// raw-socket /ready probe, verbatim).
func (subsetDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertDistribution (post-drive, BOTH sides via the runner's aggregate accept
// channel): the CONSERVATION prong. The runner snapshots per-backend accept totals
// across the WHOLE workload (it cannot attribute by route — that is done INLINE in
// drive() from the embedded body idxs), so this leg asserts:
//
//   - conservation: each side's per-backend counts sum to the ROUTED total
//     (3*perRoute = 48) — /health is a direct_response that NEVER reaches a backend
//     (no accept, no upstream_cx), so it is EXCLUDED from the routed sum
//     (accounted for explicitly: the 8 health round-trips contribute 0 accepts);
//   - full-roster coverage: all 4 backends are nonzero — /v1 hits {0,1}, /v2 hits
//     {2,3}, so the union over the routed workload touches EVERY endpoint (a
//     stronger statement than the per-route spread already checked in drive()).
//
// DETERMINISTIC/EXACT — not a σ-band (the subset key is static config, not RNG).
func (subsetDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	for _, sd := range []struct {
		side   string
		counts []uint64
	}{{"reference", refCounts}, {"subject", subjCounts}} {
		if len(sd.counts) != backendCount {
			return fmt.Errorf("%s: expected %d backend counts, got %d", sd.side, backendCount, len(sd.counts))
		}
		var sum, nonzero uint64
		for _, c := range sd.counts {
			sum += c
			if c > 0 {
				nonzero++
			}
		}
		if sum != totalReqs {
			return fmt.Errorf("%s conservation: routed sum %d != %d (3*perRoute; /health must NOT reach a backend)", sd.side, sum, totalReqs)
		}
		if nonzero != backendCount {
			return fmt.Errorf("%s coverage: %d backend(s) nonzero, want all %d (/v1∪/v2 must touch every endpoint)", sd.side, nonzero, backendCount)
		}
	}
	return nil
}

// AssertStats (post-drive): SPEC §8.1. The cross-EQUAL set (request-counted →
// identical on both sides) + the subject-side lb_subsets_active==2 unit assert
// (NOT cross-equaled — the reference contrib build's active/created accounting
// differs from envoy-go's ×1-per-distinct-subset). Per
// reference_docker_probe_bridge_network the cross-equal upstream_cx_total>0 also
// proves the reference container actually decoded. The cx/rq/selected/fallback
// wants are DERIVED constants, never hardcoded literals.
func (subsetDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	ref, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}

	pfx := "cluster." + clusterName + "."

	// "decode ran" guard (reference_docker_probe_bridge_network): if the reference
	// container failed to reach the backends, upstream_cx_total is 0 and the whole
	// readout is untrustworthy — bite explicitly before the cross-equal loop.
	if rv := ref[pfx+"upstream_cx_total"]; rv == 0 {
		t.Fatalf("reference did NOT decode: %supstream_cx_total == 0 (container could not reach the backends — bridge network / host.docker.internal?)", pfx)
	}

	// Cross-equal stats (request-counted → identical on both sides).
	for _, p := range []struct {
		name string
		want uint64
	}{
		{pfx + "upstream_cx_total", totalReqs},      // per-request fresh dial → routed total
		{pfx + "upstream_rq_total", totalReqs},      // HTTP plane Inc's rq_total on both sides
		{pfx + "membership_total", backendCount},    // the static 4-endpoint roster
		{pfx + "upstream_cx_active", 0},             // quiesced (Connection: close)
		{pfx + "lb_subsets_selected", selectedReqs}, // /v1+/v2 routed to a subset
		{pfx + "lb_subsets_fallback", fallbackReqs}, // /none → ANY_ENDPOINT fallback
	} {
		rv, rok := ref[p.name]
		sv, sok := subj[p.name]
		if !rok {
			t.Errorf("ref: %s ABSENT in /stats", p.name)
			continue
		}
		if !sok {
			t.Errorf("subj: %s ABSENT in /stats", p.name)
			continue
		}
		if rv != sv {
			t.Errorf("cross-side mismatch %s: ref=%d subj=%d", p.name, rv, sv)
		}
		if rv != p.want {
			t.Errorf("ref %s = %d, want %d", p.name, rv, p.want)
		}
		if sv != p.want {
			t.Errorf("subj %s = %d, want %d", p.name, sv, p.want)
		}
	}

	// SUBJECT-side unit assert: lb_subsets_active == 2 (the 2 version subsets).
	// NOT cross-equaled (the reference's active/created accounting differs from
	// envoy-go's ×1-per-distinct-subset count — phase 38.1 SPEC §8.1).
	if sv, ok := subj[pfx+"lb_subsets_active"]; !ok {
		t.Errorf("subj: %slb_subsets_active ABSENT in /stats", pfx)
	} else if sv != subjectActiveSubsets {
		t.Errorf("subj %slb_subsets_active = %d, want %d (the 2 version subsets)", pfx, sv, subjectActiveSubsets)
	}
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0063 driver scrapeStats,
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
	_ fixture.Driver               = (*subsetDriver)(nil)
	_ fixture.DistributionAsserter = (*subsetDriver)(nil)
	_ fixture.StatsAsserter        = (*subsetDriver)(nil)
	_ fixture.BackendKindAware     = (*subsetDriver)(nil)
)
