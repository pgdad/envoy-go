// Package driver registers the 0060-lb-random cross-side differential fixture
// (phase 35 SPEC §10 / PLAN Task 5).
//
// This is a CROSS-SIDE [tcp_proxy] fixture over ONE 3-endpoint cluster with
// lb_policy: RANDOM on BOTH sides (the 0001 shape: reference STRICT_DNS/
// host.docker.internal, subject STATIC/127.0.0.1). It REUSES the 0059 hold-4 +
// burst-60 + drain workload UNCHANGED, but INVERTS the distribution assertion:
// where 0059 (least_request) asserts that the loaded backend is AVOIDED
// (starvation + concentration skew), 0060 asserts the CONTRAPOSITIVE — RANDOM
// IGNORES load, so all three backends stay in a fair-share ANTI-SKEW band
// DESPITE the same held-conn load. It drives the workload against each side and
// asserts:
//
//   - byte-equivalence of the concatenated echo stream (the runner's CompareBytes
//     gate — deterministic payloads echoed by the streaming TCPEcho backend);
//   - a PER-SIDE anti-skew band on the per-backend accept counts via
//     AssertDistribution (conservation / uniform-floor 6 / uniform-ceiling 40 —
//     the SECOND band-based AssertDistribution; never cross-side-exact because the
//     two sides run independent RNG streams — SPEC §10 / the 0059 per-side
//     precedent);
//   - a cross-side StatsAsserter prong (cross-equal upstream_cx_total / membership
//     _total / upstream_cx_active + per-side upstream_rq_total — SPEC §7 / §10 /
//     AMEND-R4), IDENTICAL to 0059.
//
// NO new BackendKind (reuses TCPEcho = 0 — AMEND-R1). NO boot-reject dir. RANDOM
// has no config message (AMEND-R1 — least_request_lb_config is DELETED on both
// sides).
//
// # Cross-references
//
//   - phase 35 SPEC §10 (the REUSED workload + the anti-skew band + the stats-prong
//     design) + AMEND-R1 (RANDOM has no lb_config; reuses TCPEcho) + AMEND-R4 (the
//     unchanged stat surface + the per-side upstream_rq_total boundary).
//   - 0059-lb-least-request (the TRANSPOSED template — same workload/stats, the
//     INVERTED distribution band).
//   - 0001-tcp-proxy-rr (the [tcp_proxy] harness shape — STRICT_DNS reference /
//     STATIC subject, the raw-socket /ready probe, the DistributionAsserter).
//   - reference_differential_break_protocol_count1 (the band liveness proofs at
//     Task 6 run with -count=1 to defeat go-test caching).
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

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0060-lb-random"

	// In-container reference Envoy listener port for l_tcp. Fixtures run
	// sequentially so the in-container port may be reused, but a distinct value
	// avoids confusion — 0059-lb-least-request takes 19148, this takes 19149.
	refContainerListenerPort = 19149

	refAdminPort = 9901

	clusterName = "c_echo"

	heldConns  = 4                      // K — the hold phase (REUSED from 0059)
	burstConns = 60                     // S — the burst phase (REUSED from 0059)
	totalConns = heldConns + burstConns // 64 — the conservation target

	// Anti-skew band bounds on the per-side accept counts. The SYMMETRIC band
	// {floor 6, ceiling 40} — every backend stays in the fair-share band DESPITE
	// the held load (RANDOM ignores load). The CONTRAPOSITIVE of 0059. Single
	// source of truth — used in BOTH the check and its error message.
	//
	// Mean per backend = 64/3 = 21.3; σ ≈ sqrt(64 · 1/3 · 2/3) ≈ 3.77. The band is
	// WIDE on purpose: the runner draws 3 backends × 2 sides = 6 independent
	// uniform-binomial(64, 1/3) samples per run, so a TIGHT band flakes on the
	// natural ~2.5σ tail. Task-6 tuning REPLACED the originally-pinned {12, 32}
	// (per-sample floor-viol P≈0.0032, ceiling-viol P≈0.0020 → ~30%/~20% flake over
	// a -count=20 batch of 120 samples; EMPIRICALLY flaked 2/20) with the present
	// {6, 40} (per-sample floor-viol P≈2e-6, ceiling-viol P≈3e-7 → <0.03% over 120
	// samples), which is flake-free over repeat batches AND still bites every break:
	//   floor   6 = 21.3 − ~4.05σ: VIOLATED by a load-skewing policy starving a
	//                              loaded backend (0059's c1≈2 < 6), and by single-
	//                              host pin (two backends at 0 < 6).
	//   ceiling 40 = 21.3 + ~4.95σ: VIOLATED by single-host pin (one backend ~64 > 40).
	uniformFloor   = 6
	uniformCeiling = 40

	// readEchoTimeout bounds the hold-phase establishment read.
	readEchoTimeout = 2 * time.Second
	// rtTimeout bounds each burst round-trip.
	rtTimeout = 2 * time.Second
	// settleDelay lets the async stat pipeline settle before AssertStats scrapes
	// (the 0059 sleep-to-settle precedent), and lets the drained held conns'
	// upstream closes propagate so upstream_cx_active quiesces to 0.
	settleDelay = 750 * time.Millisecond
)

func init() {
	fixture.RegisterFixture(fixtureName, &randDriver{})
}

type randDriver struct{}

func (randDriver) BackendCount() int           { return 3 }
func (randDriver) SubjectListenerName() string { return "l_tcp" }
func (randDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

func (randDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("%s: expected 3 backend ports, got %d", fixtureName, len(backendPorts)))
	}
	// STRICT_DNS + host.docker.internal (the 0001 reference shape) + RANDOM (no
	// lb_config — AMEND-R1).
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
  clusters:
    - name: c_echo
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: RANDOM
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, refAdminPort, refContainerListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (randDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("%s: expected 3 backend ports, got %d", fixtureName, len(backendPorts)))
	}
	// STATIC + 127.0.0.1 (the 0001 subject shape) + RANDOM (no lb_config — AMEND-R1).
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0060, cluster: envoy-go-differential }
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
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      lb_policy: RANDOM
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

// drive runs the hold-4 + burst-60 + drain workload against addr and returns the
// concatenated echo bytes. Identical per side (deterministic payloads → byte-equal
// across the two proxies regardless of which endpoint serves which conn).
// REUSED UNCHANGED from 0059.
//
// Phase 1 (HOLD): open K conns; write+read-echo each (the establishment witness,
// AMEND-L2 — proves the upstream dial completed and the pick's active count is
// held), then KEEP the socket open. (Under RANDOM the held conns do NOT skew the
// burst — the contrast with 0059's least_request.)
//
// Phase 2 (BURST): S sequential short round-trips (close-accounting between picks).
//
// Phase 3 (DRAIN): close the K held conns (deferred). A settle delay then lets the
// upstream closes propagate so upstream_cx_active quiesces before AssertStats.
func drive(ctx context.Context, addr string) ([]byte, error) {
	var b bytes.Buffer

	held := make([]net.Conn, 0, heldConns)
	defer func() {
		for _, c := range held {
			_ = c.Close()
		}
	}()

	// Phase 1 — HOLD.
	for i := 0; i < heldConns; i++ {
		c, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("hold dial[%d]: %w", i, err)
		}
		payload := []byte(fmt.Sprintf("hold-%d\n", i))
		if _, err := c.Write(payload); err != nil {
			return nil, fmt.Errorf("hold write[%d]: %w", i, err)
		}
		echo := make([]byte, len(payload))
		if err := readFull(c, echo, readEchoTimeout); err != nil { // establishment witness
			return nil, fmt.Errorf("hold echo[%d]: %w", i, err)
		}
		b.Write(echo)
		held = append(held, c)
	}

	// Phase 2 — BURST.
	for i := 0; i < burstConns; i++ {
		resp, err := helpers.TCPRoundTrip(ctx, addr, []byte(fmt.Sprintf("burst-%d\n", i)), rtTimeout)
		if err != nil {
			return nil, fmt.Errorf("burst[%d]: %w", i, err)
		}
		b.Write(resp)
	}

	// Phase 3 — DRAIN is the deferred Close above; settle so the upstream closes
	// propagate before the runner scrapes (upstream_cx_active must quiesce to 0).
	for _, c := range held {
		_ = c.Close()
	}
	held = held[:0] // avoid a double-close in the defer
	if err := sleepCtx(ctx, settleDelay); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (randDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

func (randDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

// readFull reads exactly len(buf) bytes from c under a deadline, LEAVING the
// socket open (helpers.TCPRoundTrip half-closes and reads to EOF, so it cannot
// be reused for a held conn — the hold phase needs a bounded read that keeps the
// socket open).
func readFull(c net.Conn, buf []byte, timeout time.Duration) error {
	_ = c.SetReadDeadline(time.Now().Add(timeout))
	if _, err := io.ReadFull(c, buf); err != nil {
		return err
	}
	_ = c.SetReadDeadline(time.Time{}) // clear the deadline for the held lifetime
	return nil
}

// sleepCtx sleeps for d or returns early if ctx is canceled.
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
func (randDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertDistribution: PER-SIDE anti-skew band on the per-backend accept counts
// (the runner snapshots backend.accepts after Drive — each accept on the streaming
// TCPEcho backend counts one connection: hold + burst sum to 64). NEVER cross-side-
// exact (independent RNG streams — SPEC §10; the 0059 per-side precedent). The
// SECOND band-based AssertDistribution. The CONTRAPOSITIVE of 0059 (least_request):
// RANDOM IGNORES load, so EVERY backend stays in the fair-share band DESPITE the
// same held-conn load. The band is SYMMETRIC (check each of the three raw counts):
//
//	conservation:    c1 + c2 + c3 == 64           (hard equality; catches drop/double-count)
//	uniform floor:   each cᵢ >= 6    (mean 21.3 − ~4σ; VIOLATED by a load-skewing
//	                                  policy starving a loaded backend, and by single-host pin)
//	uniform ceiling: each cᵢ <= 40   (mean 21.3 + ~5σ; VIOLATED by single-host pin)
func (randDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	for _, sd := range []struct {
		side   string
		counts []uint64
	}{{"reference", refCounts}, {"subject", subjCounts}} {
		if len(sd.counts) != 3 {
			return fmt.Errorf("%s: expected 3 backend counts, got %d", sd.side, len(sd.counts))
		}
		var sum uint64
		for i, c := range sd.counts {
			sum += c
			if c < uniformFloor {
				return fmt.Errorf("%s: uniform floor: backend[%d]=%d < %d (load-skewing policy? single-host pin?)", sd.side, i, c, uniformFloor)
			}
			if c > uniformCeiling {
				return fmt.Errorf("%s: uniform ceiling: backend[%d]=%d > %d (single-host pin?)", sd.side, i, c, uniformCeiling)
			}
		}
		if sum != totalConns {
			return fmt.Errorf("%s: conservation: sum %d != %d", sd.side, sum, totalConns)
		}
	}
	return nil
}

// AssertStats (post-drain): SPEC §7 — cross-equal upstream_cx_total==64 +
// membership_total==3 + upstream_cx_active==0 (quiesced); PER-SIDE upstream_rq_total
// (ref=64 — tcp_proxy charges rq-per-cx, AMEND-L2; subj=0 — envoy-go's tcpproxy
// path NEVER calls IncUpstreamRqTotal — a pre-existing documented boundary,
// AMEND-R4). IDENTICAL to 0059 — the stat surface is unchanged by RANDOM.
func (randDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
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

	// Cross-equal counters (tcp_proxy is 1:1 downstream-conn→upstream-dial on both
	// sides; membership is the static 3-endpoint roster; cx_active quiesces post-drain).
	for _, p := range []struct {
		name string
		want uint64
	}{
		{pfx + "upstream_cx_total", totalConns},
		{pfx + "membership_total", 3},
		{pfx + "upstream_cx_active", 0},
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

	// Per-side upstream_rq_total (NOT cross-equal — AMEND-R4). The reference's
	// tcp_proxy charges one rq per cx (rq-per-cx, AMEND-L2) → 64; envoy-go's
	// tcpproxy path never Inc's upstream_rq_total → 0.
	const rqKey = "cluster." + clusterName + ".upstream_rq_total"
	if got := ref[rqKey]; got != totalConns {
		t.Errorf("ref %s = %d, want %d (rq-per-cx — AMEND-L2)", rqKey, got, totalConns)
	}
	if got := subj[rqKey]; got != 0 {
		t.Errorf("subj %s = %d, want 0 (tcpproxy never Inc's upstream_rq_total — AMEND-R4)", rqKey, got)
	}
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0059 driver scrapeStats,
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
	_ fixture.Driver               = (*randDriver)(nil)
	_ fixture.DistributionAsserter = (*randDriver)(nil)
	_ fixture.StatsAsserter        = (*randDriver)(nil)
)
