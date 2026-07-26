// Package driver registers the 0061-lb-ring-hash cross-side differential fixture
// (phase 36 SPEC §8.1 / 36.1 PLAN Task 7).
//
// This is a CROSS-SIDE [tcp_proxy] fixture over ONE 3-endpoint cluster with
// lb_policy: RING_HASH + ring_hash_lb_config: {} (defaults) + the tcp_proxy
// hash_policy: [{source_ip: {}}] on BOTH sides (the 0001 shape: reference
// STRICT_DNS / host.docker.internal, subject STATIC / 127.0.0.1). It is the
// end-to-end proof of Tasks 3-6 (the source_ip hash_policy → WithHashKey →
// ringHashLB pick → affinity).
//
// The driver binds outgoing connections to source IPs 127.0.0.2..17 (via
// net.Dialer.LocalAddr — feasible on host loopback, SPEC §11.8), 16 connections
// per source IP = 256 total (the 0059/0060 conservation target). It drives the
// workload against each side and asserts:
//
//   - byte-equivalence of the echoed payload (the runner's CompareBytes gate —
//     deterministic payloads echoed by the streaming TCPEcho backend; byte-equal
//     regardless of WHICH endpoint serves WHICH connection);
//   - the SUBJECT-SIDE affinity+spread via the aggregate-count MODULAR INVARIANT
//     (D-S36-4): every subject per-backend count ≡ 0 mod 16 (affinity — one source
//     IP → one key → one ring point → one backend, so each source IP contributes
//     all 16 or 0 to a given backend) AND >= 2 backends nonzero (spread). The
//     REFERENCE is Docker-NAT'd to ONE gateway source IP → single-key pin → all 256
//     on ONE backend, so it is asserted on conservation only (cross-side host
//     identity INFEASIBLE — AMEND-RH8 / reference_differential_hash_key_cross_side_
//     infeasible);
//   - a cross-side StatsAsserter prong (cross-equal upstream_cx_total / membership
//     _total / upstream_cx_active + the 3 ring_hash_lb.* gauges 1026/342/342 — which
//     depend only on ring-config + host COUNT, NOT addresses, so they ARE cross-equal
//     — + per-side upstream_rq_total — SPEC §7).
//
// NO new BackendKind (reuses TCPEcho = 0). NO boot-reject dir. FIRST consistent-hash
// fixture; FIRST non-zero LB-stat delta (+3 gauges → surface 1119).
//
// # Cross-references
//
//   - phase 36 SPEC §7 (the +3 ring_hash_lb gauges + the cross-side stats set) +
//     §8.1 (the 0061 fixture design + the multi-source-IP workload + the reference
//     asymmetry) + AMEND-RH8 (cross-side host identity infeasible; subject-side
//     affinity).
//   - 0060-lb-random (the [tcp_proxy] harness template — STRICT_DNS reference /
//     STATIC subject, the raw-socket /ready probe, the scrapeStats helper, the
//     cross-equal stats loop).
//   - reference_differential_hash_key_cross_side_infeasible (the Docker-NAT single-
//     source-IP collapse → subject-side affinity only).
//   - reference_differential_run_selector (target -run 'TestDifferential/0061').
//   - reference_differential_break_protocol_count1 (the Task-8 liveness proofs run
//     with -count=1 to defeat go-test caching).
package driver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0061-lb-ring-hash"

	// In-container reference Envoy listener port for l_tcp. Fixtures run
	// sequentially so the in-container port may be reused, but a distinct value
	// avoids confusion — 0059 takes 19148, 0060 takes 19149, this takes 19150.
	refContainerListenerPort = 19150

	refAdminPort = 9901

	clusterName = "c_echo"

	sourceIPs  = 16                     // 127.0.0.2 .. 127.0.0.17
	burstPerIP = 16                     // connections per source IP
	totalConns = sourceIPs * burstPerIP // 256 — the conservation target

	// readEchoTimeout bounds each per-conn echo read.
	readEchoTimeout = 2 * time.Second
	// settleDelay lets the async stat pipeline settle before AssertStats scrapes
	// (the 0059/0060 sleep-to-settle precedent), and lets the upstream closes
	// propagate so upstream_cx_active quiesces to 0.
	settleDelay = 750 * time.Millisecond
)

func init() {
	fixture.RegisterFixture(fixtureName, &ringHashDriver{})
}

type ringHashDriver struct{}

func (ringHashDriver) BackendCount() int           { return 3 }
func (ringHashDriver) SubjectListenerName() string { return "l_tcp" }
func (ringHashDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

func (ringHashDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("%s: expected 3 backend ports, got %d", fixtureName, len(backendPorts)))
	}
	// STRICT_DNS + host.docker.internal (the 0001 reference shape) + RING_HASH
	// (ring_hash_lb_config: {} — defaults 1024/8388608/XX_HASH) + the tcp_proxy
	// source_ip hash_policy.
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 0.0.0.0, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
                hash_policy:
                  - source_ip: {}
  clusters:
    - name: c_echo
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: RING_HASH
      ring_hash_lb_config: {}
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, refAdminPort, refContainerListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (ringHashDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("%s: expected 3 backend ports, got %d", fixtureName, len(backendPorts)))
	}
	// STATIC + 127.0.0.1 (the 0001 subject shape) + RING_HASH (ring_hash_lb_config:
	// {} — defaults) + the tcp_proxy source_ip hash_policy.
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0061, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
                hash_policy:
                  - source_ip: {}
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      lb_policy: RING_HASH
      ring_hash_lb_config: {}
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

// drive opens burstPerIP connections from each of the 16 source IPs (bound via
// net.Dialer.LocalAddr 127.0.0.2..17 — feasible on host loopback, SPEC §11.8),
// does one echo round-trip per conn, and closes so upstream_cx_active quiesces
// before AssertStats. SHARED by both DriveReference and DriveSubject: the payloads
// are identical → the echoed bytes are byte-equal across the two proxies. The
// SUBJECT shows per-source-IP affinity (each source IP → one distinct loopback
// RemoteAddr → one source_ip key → one ring point → one backend); the REFERENCE
// collapses via Docker NAT (all conns rewritten to one gateway source IP → one
// key → one backend).
//
// Returns the concatenated echo stream (the runner's CompareBytes input).
func drive(ctx context.Context, addr string) ([]byte, error) {
	var b bytes.Buffer
	for s := 0; s < sourceIPs; s++ {
		local := &net.TCPAddr{IP: net.IPv4(127, 0, 0, byte(2+s))}
		d := &net.Dialer{LocalAddr: local}
		for i := 0; i < burstPerIP; i++ {
			c, err := d.DialContext(ctx, "tcp", addr)
			if err != nil {
				return nil, fmt.Errorf("dial from %v: %w", local.IP, err)
			}
			payload := []byte(fmt.Sprintf("rh-%d-%d\n", s, i))
			if _, err := c.Write(payload); err != nil {
				_ = c.Close()
				return nil, fmt.Errorf("write from %v: %w", local.IP, err)
			}
			echo := make([]byte, len(payload))
			if err := readFull(c, echo, readEchoTimeout); err != nil {
				_ = c.Close()
				return nil, fmt.Errorf("echo from %v: %w", local.IP, err)
			}
			b.Write(echo)
			_ = c.Close()
		}
	}
	// settle so upstream closes propagate before AssertStats scrapes (cx_active → 0).
	if err := sleepCtx(ctx, settleDelay); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (ringHashDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

func (ringHashDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

// readFull reads exactly len(buf) bytes from c under a deadline. (The 0060 helper,
// verbatim.)
func readFull(c net.Conn, buf []byte, timeout time.Duration) error {
	_ = c.SetReadDeadline(time.Now().Add(timeout))
	if _, err := io.ReadFull(c, buf); err != nil {
		return err
	}
	_ = c.SetReadDeadline(time.Time{})
	return nil
}

// sleepCtx sleeps for d or returns early if ctx is canceled. (The 0060 helper.)
func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0001
// raw-socket /ready probe, verbatim).
func (ringHashDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertDistribution: SUBJECT-SIDE affinity (every per-backend count is a multiple
// of burstPerIP — the consistent-hash invariant: one source IP → one key → one ring
// point → one backend, so each source IP contributes all 16 or 0 to each backend) +
// SPREAD (>= 2 distinct backends nonzero). AFFINITY (and conservation) is
// DETERMINISTIC/EXACT — not a σ-band (reference_differential_band_sigma_margin
// governs RNG bands; affinity is not one). SPREAD is NOT: it is PROBABILISTIC. The
// ring is keyed on each backend's EPHEMERAL-PORT address, which the harness
// re-allocates per run, so every run draws a fresh random 3-way partition and
// P(collapse) <= 3^(1-sourceIPs) = 7.0e-8 at sourceIPs=16 — a 5.27σ-equivalent
// margin (ADR-0298). That figure is ANALYTIC/EXTRAPOLATED, NOT measured: K=16 is
// unmeasurable at feasible scale (expected count 0.014 over 2e5 draws; 0/200000
// bounds the rate at ~1.5e-5 only). And 3^(1-K) is a CONSERVATIVE UPPER BOUND, not
// "the" probability — measured/analytic is 0.949 at K=4 and 0.689 at K=8. The bound
// is measured at those K by TestRingHash_EphemeralPortRing_KeyCollapseRate in
// internal/cluster, which this package's TestSourceIPsLinkedToCollapseFixtureK pins
// to THIS fixture's sourceIPs.
//
// The REFERENCE is Docker-NAT'd to one source IP → all 256 on ONE backend; it is
// asserted on conservation only (single-key pin — AMEND-RH8 /
// reference_differential_hash_key_cross_side_infeasible). Its real proof is byte-
// equiv + the cross-side stats.
func (ringHashDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	if len(subjCounts) != 3 || len(refCounts) != 3 {
		return fmt.Errorf("expected 3 backend counts, got ref=%d subj=%d", len(refCounts), len(subjCounts))
	}
	// SUBJECT: affinity (each count ≡ 0 mod burstPerIP) + spread (>=2 nonzero) + conservation.
	var subjSum, nonzero uint64
	for i, c := range subjCounts {
		subjSum += c
		if c%burstPerIP != 0 {
			return fmt.Errorf("subject affinity: backend[%d]=%d not a multiple of %d (key scattered? a source IP split across backends)", i, c, burstPerIP)
		}
		if c > 0 {
			nonzero++
		}
	}
	if subjSum != totalConns {
		return fmt.Errorf("subject conservation: sum %d != %d", subjSum, totalConns)
	}
	if nonzero < 2 {
		return fmt.Errorf("subject spread: only %d backend(s) nonzero, want >= 2 (ring collapsed?)", nonzero)
	}
	// REFERENCE: conservation only (Docker NAT → single source IP → single-key pin).
	var refSum uint64
	for _, c := range refCounts {
		refSum += c
	}
	if refSum != totalConns {
		return fmt.Errorf("reference conservation: sum %d != %d", refSum, totalConns)
	}
	return nil
}

// AssertStats (post-drain): SPEC §7 — cross-equal upstream_cx_total==256 +
// membership_total==3 + upstream_cx_active==0 (quiesced) + the THREE ring_hash_lb
// gauges (size==1026 / min_hashes_per_host==342 / max_hashes_per_host==342 — they
// depend ONLY on ring-config + host COUNT, NOT addresses, so they ARE cross-equal);
// PER-SIDE upstream_rq_total (ref=256 — tcp_proxy charges rq-per-cx; subj=0 —
// envoy-go's tcpproxy path NEVER calls IncUpstreamRqTotal, the 0059/0060 boundary).
func (ringHashDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
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

	// Cross-equal stats (tcp_proxy is 1:1 downstream-conn→upstream-dial on both
	// sides; membership is the static 3-endpoint roster; cx_active quiesces
	// post-drain; the 3 ring_hash_lb gauges are address-independent → identical).
	for _, p := range []struct {
		name string
		want uint64
	}{
		{pfx + "upstream_cx_total", totalConns},
		{pfx + "membership_total", 3},
		{pfx + "upstream_cx_active", 0},
		{pfx + "ring_hash_lb.size", 1026},
		{pfx + "ring_hash_lb.min_hashes_per_host", 342},
		{pfx + "ring_hash_lb.max_hashes_per_host", 342},
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

	// Per-side upstream_rq_total (NOT cross-equal). The reference's tcp_proxy charges
	// one rq per cx (rq-per-cx) → 256; envoy-go's tcpproxy path never Inc's
	// upstream_rq_total → 0.
	const rqKey = "cluster." + clusterName + ".upstream_rq_total"
	if got := ref[rqKey]; got != totalConns {
		t.Errorf("ref %s = %d, want %d (rq-per-cx)", rqKey, got, totalConns)
	}
	if got := subj[rqKey]; got != 0 {
		t.Errorf("subj %s = %d, want 0 (tcpproxy never Inc's upstream_rq_total)", rqKey, got)
	}
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0060 driver scrapeStats,
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
	_ fixture.Driver               = (*ringHashDriver)(nil)
	_ fixture.DistributionAsserter = (*ringHashDriver)(nil)
	_ fixture.StatsAsserter        = (*ringHashDriver)(nil)
)
