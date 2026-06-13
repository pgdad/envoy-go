// Package driver registers the 0063-lb-maglev cross-side differential
// fixture (phase 37 SPEC §10 / 37 PLAN Task 7).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture over ONE
// 3-endpoint cluster with lb_policy: MAGLEV + maglev_lb_config: {}
// (default table_size 65537) + a ROUTE-LEVEL hash_policy: [{header: {header_name: "x-hash"}}]
// on BOTH sides (the 0003 HTTP shape: reference STRICT_DNS / host.docker.internal,
// subject STATIC / 127.0.0.1). It is the end-to-end proof of the maglev plane (the
// HTTP route hash_policy producer → cluster.HashHeaderValues digest → applyHashKey
// → cluster.WithHashKey → maglevLB pick → per-header-value affinity over a Maglev
// TABLE).
//
// It is the MAGLEV transposition of 0062-lb-ring-hash-http (the ring_hash HTTP
// plane): SAME X-Hash header workload, SAME both-side modular invariant, retargeted
// from RING_HASH to MAGLEV. As in 0062, the X-Hash HEADER is NAT-TRANSPARENT — it
// survives the Docker hop verbatim — so the consistent-hash affinity invariant holds
// on BOTH the envoy-go subject AND the reference Envoy. This is a TRUE cross-side
// affinity proof.
//
// The driver sends N=16 distinct X-Hash values × K=16 repeats = 256 routed GETs
// against each side and asserts (totalReqs is DERIVED from the named constants —
// the cx/rq stat expectations track it, never a literal):
//
//   - byte-equivalence of the concatenated /health direct_response bodies (the
//     runner's CompareBytes gate). The ROUTED bodies are NOT concatenated: the
//     HTTPEcho backend embeds its own idx in the body (`backend-<idx>:<seg>`), so
//     ref and subj — whose tables are built over DIFFERENT endpoint address strings
//     — may land a given X-Hash value on a DIFFERENT backend idx, diverging the
//     bytes. Routing/affinity is covered by AssertDistribution; the byte-equiv
//     prong rides the address-independent /health direct_response (the 0003
//     precedent);
//   - the BOTH-SIDE affinity+spread via the aggregate-count MODULAR INVARIANT:
//     every per-backend count ≡ 0 mod K=16 (affinity — one header value → one digest
//     → one table slot → one backend, so each value contributes all 16 or 0 to a
//     given backend) AND >= 2 backends nonzero (spread). This holds on BOTH sides
//     because the X-Hash header is NAT-transparent;
//   - a cross-side StatsAsserter prong: cross-equal upstream_cx_total==totalReqs /
//     membership_total==3 / upstream_cx_active==0 + the 2 maglev_lb.* gauges
//     (21845/21846 — they depend ONLY on table_size + host COUNT, NOT addresses →
//     cross-equal) + the cross-equal upstream_rq_total==totalReqs prong (the HTTP
//     plane DOES Inc upstream_rq_total). SPEC §7.
//
// NO new BackendKind (reuses HTTPEcho — the 0003 backend; the tail STAYS 33). NO
// boot-reject dir (the maglev table_size / route hash_policy reject arms land
// UNIT-LEVEL). NOT asserted: cross-side host IDENTITY (the two tables are built
// over different endpoint address strings — reference_differential_hash_key_cross_
// side_infeasible; the modular invariant proves affinity WITHOUT host attribution).
//
// # Cross-references
//
//   - phase 37 SPEC §10 (the HTTP route hash_policy plane over MAGLEV) + §7 (the
//     cross-side stats set incl upstream_rq_total) + D-M4 (the default-65537
//     entries-per-host gauges 21845/21846).
//   - 0062-lb-ring-hash-http (the ring_hash HTTP sibling — SAME workload + modular
//     invariant; this is the MAGLEV retarget).
//   - 0003-http11-routing (the HCM+router HTTP harness template — STRICT_DNS
//     reference / STATIC subject, the /health direct_response byte-equiv prong,
//     the HTTPEcho backend, the per-request fresh-dial accept accounting).
//   - reference_differential_hash_key_cross_side_infeasible (host identity
//     infeasible → assert affinity via the modular invariant, not host attribution).
//   - reference_differential_run_selector (target -run 'TestDifferential/0063-lb-maglev').
//   - reference_differential_break_protocol_count1 (the Task-8 liveness proofs run
//     with -count=1 to defeat go-test caching).
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
	fixtureName = "0063-lb-maglev"

	// In-container reference Envoy listener port for l_http. Fixtures run
	// sequentially so the in-container port may be reused, but a distinct value
	// avoids confusion — 0062-lb-ring-hash-http takes 19151, this takes 19152.
	refContainerListenerPort = 19152

	refAdminPort = 9901

	clusterName = "c_echo"

	hashHeader   = "x-hash"                  // the route hash_policy header_name
	hashValues   = 16                        // N — distinct X-Hash values (hv-0..15)
	repeatPerVal = 16                        // K — requests per distinct value (the modular base)
	totalReqs    = hashValues * repeatPerVal // 256 — the conservation target (N*K)

	healthReqs = 8 // /health direct_response round-trips for the byte-equiv stream
)

func init() {
	fixture.RegisterFixture(fixtureName, &maglevDriver{})
}

type maglevDriver struct{}

func (maglevDriver) BackendCount() int                { return 3 }
func (maglevDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (maglevDriver) SubjectListenerName() string      { return "l_http" }
func (maglevDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

func (maglevDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("%s: expected 3 backend ports, got %d", fixtureName, len(backendPorts)))
	}
	// STRICT_DNS + host.docker.internal (the 0003 reference shape) + MAGLEV
	// (maglev_lb_config: {} — default table_size 65537) + the route-level
	// header hash_policy.
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
                      routes:
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                        - match: { prefix: "/" }
                          route:
                            cluster: c_echo
                            hash_policy:
                              - header: { header_name: "x-hash" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_echo
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: MAGLEV
      maglev_lb_config: {}
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, refAdminPort, refContainerListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (maglevDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("%s: expected 3 backend ports, got %d", fixtureName, len(backendPorts)))
	}
	// STATIC + 127.0.0.1 (the 0003 subject shape) + MAGLEV (maglev_lb_config:
	// {} — default table_size 65537) + the route-level header hash_policy.
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0063, cluster: envoy-go-differential }
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
                      routes:
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                        - match: { prefix: "/" }
                          route:
                            cluster: c_echo
                            hash_policy:
                              - header: { header_name: "x-hash" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      lb_policy: MAGLEV
      maglev_lb_config: {}
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

// drive sends N distinct X-Hash values × K repeats = totalReqs routed GET /get
// requests (each carrying its X-Hash header — the route hash_policy key), then healthReqs
// /health direct_response round-trips. SHARED by both DriveReference and
// DriveSubject.
//
// The ROUTED bodies are intentionally NOT concatenated into the returned stream:
// the HTTPEcho backend embeds its own idx (`backend-<idx>:<seg>`), and ref/subj
// tables — built over different endpoint address strings — may land a value on a
// different backend idx, so the per-request bytes diverge cross-side (host identity
// infeasible). Affinity is proven by AssertDistribution; the returned stream is the
// address-INDEPENDENT /health direct_response bodies ("OK\n"), which are byte-equal
// across the two proxies (the 0003 byte-equiv precedent).
//
// Per-request fresh dial (HTTPRoundTrip sets Connection: close), so each routed
// request is one upstream connection → the HTTPEcho backend's accept counter
// increments once per routed request, giving per-backend request counts directly.
func drive(ctx context.Context, addr string) ([]byte, error) {
	// Routed requests: totalReqs (N*K) hash-keyed GETs (NOT concatenated — see doc comment).
	for v := 0; v < hashValues; v++ {
		hv := fmt.Sprintf("hv-%d", v)
		hdr := http.Header{}
		hdr.Set(hashHeader, hv)
		for i := 0; i < repeatPerVal; i++ {
			resp, _, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/get", hdr, nil)
			if err != nil {
				return nil, fmt.Errorf("routed %s[%d]: %w", hv, i, err)
			}
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("routed %s[%d]: status %d, want 200", hv, i, resp.StatusCode)
			}
		}
	}

	// Byte-equiv stream: healthReqs /health direct_response bodies ("OK\n"),
	// address-independent → byte-equal cross-side.
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

func (maglevDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

func (maglevDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0003
// raw-socket /ready probe, verbatim).
func (maglevDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertDistribution: BOTH-SIDE affinity (every per-backend count is a multiple of
// repeatPerVal=16 — the consistent-hash invariant: one X-Hash value → one digest →
// one table slot → one backend, so each value contributes all 16 or 0 to a given
// backend) + SPREAD (>= 2 distinct backends nonzero). DETERMINISTIC/EXACT — not a
// σ-band (reference_differential_band_sigma_margin governs RNG bands; affinity is
// not one). The invariant holds on BOTH sides because the X-Hash header is
// NAT-transparent (it survives the Docker hop verbatim). A scattered key (a value
// splitting across backends) breaks the multiple-of-16 invariant; a collapsed table
// breaks spread. Cross-side host IDENTITY is NOT asserted (the two tables are over
// different endpoint address strings —
// reference_differential_hash_key_cross_side_infeasible).
func (maglevDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	for _, sd := range []struct {
		side   string
		counts []uint64
	}{{"reference", refCounts}, {"subject", subjCounts}} {
		if len(sd.counts) != 3 {
			return fmt.Errorf("%s: expected 3 backend counts, got %d", sd.side, len(sd.counts))
		}
		var sum, nonzero uint64
		for i, c := range sd.counts {
			sum += c
			if c%repeatPerVal != 0 {
				return fmt.Errorf("%s affinity: backend[%d]=%d not a multiple of %d (key scattered? an X-Hash value split across backends)", sd.side, i, c, repeatPerVal)
			}
			if c > 0 {
				nonzero++
			}
		}
		if sum != totalReqs {
			return fmt.Errorf("%s conservation: sum %d != %d", sd.side, sum, totalReqs)
		}
		if nonzero < 2 {
			return fmt.Errorf("%s spread: only %d backend(s) nonzero, want >= 2 (table collapsed?)", sd.side, nonzero)
		}
	}
	return nil
}

// AssertStats (post-drive): SPEC §7 — cross-equal upstream_cx_total==totalReqs +
// membership_total==3 + upstream_cx_active==0 (quiesced; per-request fresh dial +
// Connection: close on both sides) + the TWO maglev_lb gauges
// (min_entries_per_host==21845 / max_entries_per_host==21846 — they depend ONLY on
// table_size + host COUNT, NOT addresses, so they ARE cross-equal) + upstream_rq
// _total==totalReqs (cross-equal — the HTTP plane Inc's upstream_rq_total on BOTH
// sides). The cx/rq wants are the DERIVED totalReqs constant (N*K), never a
// hardcoded literal.
func (maglevDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
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

	// Cross-equal stats (per-request fresh dial → totalReqs upstream conns + totalReqs
	// rqs on both sides; membership is the static 3-endpoint roster; cx_active quiesces
	// post-drive; the 2 maglev_lb gauges are address-independent → identical).
	for _, p := range []struct {
		name string
		want uint64
	}{
		{pfx + "upstream_cx_total", totalReqs},
		{pfx + "upstream_rq_total", totalReqs},
		{pfx + "membership_total", 3},
		{pfx + "upstream_cx_active", 0},
		{pfx + "maglev_lb.min_entries_per_host", 21845}, // floor(65537/3) — cross-side-exact (D-M4)
		{pfx + "maglev_lb.max_entries_per_host", 21846}, // ceil(65537/3)
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
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0062 driver scrapeStats,
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
	_ fixture.Driver               = (*maglevDriver)(nil)
	_ fixture.DistributionAsserter = (*maglevDriver)(nil)
	_ fixture.StatsAsserter        = (*maglevDriver)(nil)
	_ fixture.BackendKindAware     = (*maglevDriver)(nil)
)
