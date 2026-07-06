// Package driver registers the 0059-lb-least-request cross-side differential
// fixture (phase 34 SPEC §8.1 / PLAN Task 6).
//
// This is a CROSS-SIDE [tcp_proxy] fixture over ONE 3-endpoint cluster with
// lb_policy: LEAST_REQUEST + least_request_lb_config: { choice_count: 10 } on
// BOTH sides (the 0001 shape: reference STRICT_DNS/host.docker.internal,
// subject STATIC/127.0.0.1). It drives a hold-4 + burst-60 + drain workload
// against each side and asserts:
//
//   - byte-equivalence of the concatenated echo stream (the runner's CompareBytes
//     gate — deterministic payloads echoed by the streaming TCPEcho backend);
//   - a PER-SIDE band on the sorted per-backend accept counts via
//     AssertDistribution (conservation / starvation / concentration — the FIRST
//     band-based AssertDistribution; never cross-side-exact because the two sides
//     run independent RNG streams — SPEC §8.1 / the 0003 per-side-asymmetry
//     precedent);
//   - a cross-side StatsAsserter prong (cross-equal upstream_cx_total / membership
//     _total / upstream_cx_active + per-side upstream_rq_total — SPEC §7 / §8.1 /
//     AMEND-L4).
//
// NO new BackendKind (reuses TCPEcho = 0 — SPEC §8.3). NO boot-reject dir
// (AMEND-L5). NO new fuzzer (SPEC §8.3).
//
// # Cross-references
//
//   - phase 34 SPEC §8.1 (the workload + band + stats-prong design) + §7 (the
//     zero-delta stat surface + the StatsAsserter cross-vs-per-side set) +
//     AMEND-L2 (cx-as-rq: an open TCP-proxied conn IS one active request) +
//     AMEND-L4 (the per-side upstream_rq_total boundary).
//   - 0001-tcp-proxy-rr (the [tcp_proxy] harness shape — STRICT_DNS reference /
//     STATIC subject, the raw-socket /ready probe, the DistributionAsserter).
//   - 0057-thrift-roundtrip (the cross-side StatsAsserter + the flat /stats
//     scrapeStats, copied verbatim).
//   - reference_differential_break_protocol_count1 (the band liveness proofs at
//     Task 7 run with -count=1 to defeat go-test caching).
package driver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0059-lb-least-request"

	// In-container reference Envoy listener port for l_tcp. 0001 takes 15001;
	// fixtures run sequentially so the in-container port may be reused, but a
	// distinct value avoids confusion — 0057-thrift takes 19147, this takes 19148.
	refContainerListenerPort = 19148

	refAdminPort = 9901

	clusterName = "c_echo"

	heldConns  = 4                      // K — the hold phase (D-S34-3)
	burstConns = 60                     // S — the burst phase (D-S34-3)
	totalConns = heldConns + burstConns // 64 — the conservation target

	// Band bounds on the sorted per-side accept counts (c1 <= c2 <= c3), tuned at
	// Task 7. Single source of truth — used in BOTH the check and its error message.
	starvationMax    = 12 // c1 <= starvationMax: the most-held backend takes ~its held conns + ~0 burst; ROUND_ROBIN's c1==21 BITES the no-op-release break (leg ii). Observed c1 always 2 (margin 10).
	concentrationMin = 16 // c2 >= concentrationMin: the two least-loaded split the burst; an INVERTED P2C drives c2 to 0 (leg i). Observed c2 in 21-31 (margin >=5).

	// readEchoTimeout bounds the hold-phase establishment read.
	readEchoTimeout = 2 * time.Second
	// rtTimeout bounds each burst round-trip.
	rtTimeout = 2 * time.Second
	// settleDelay lets the async stat pipeline settle before AssertStats scrapes
	// (the 0057 sleep-to-settle precedent), and lets the drained held conns'
	// upstream closes propagate so upstream_cx_active quiesces to 0.
	settleDelay = 750 * time.Millisecond
)

func init() {
	fixture.RegisterFixture(fixtureName, &lrDriver{})
}

type lrDriver struct{}

func (lrDriver) BackendCount() int           { return 3 }
func (lrDriver) SubjectListenerName() string { return "l_tcp" }
func (lrDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

func (lrDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("%s: expected 3 backend ports, got %d", fixtureName, len(backendPorts)))
	}
	// STRICT_DNS + host.docker.internal (the 0001 reference shape) + LEAST_REQUEST.
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
      lb_policy: LEAST_REQUEST
      least_request_lb_config: { choice_count: 10 }
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, refAdminPort, refContainerListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (lrDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("%s: expected 3 backend ports, got %d", fixtureName, len(backendPorts)))
	}
	// STATIC + 127.0.0.1 (the 0001 subject shape) + LEAST_REQUEST.
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0059, cluster: envoy-go-differential }
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
      lb_policy: LEAST_REQUEST
      least_request_lb_config: { choice_count: 10 }
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
//
// Phase 1 (HOLD): open K conns; write+read-echo each (the establishment witness,
// AMEND-L2 — proves the upstream dial completed and the pick's active count is
// held), then KEEP the socket open (this elevates K endpoints' active counts so
// the burst skews away from them under P2C).
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

func (lrDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

func (lrDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
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
func (lrDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertDistribution: PER-SIDE band check on the sorted per-backend accept counts
// (the runner snapshots backend.accepts after Drive — each accept on the streaming
// TCPEcho backend counts one connection: hold + burst sum to 64). NEVER cross-side-
// exact (independent RNG streams — SPEC §8.1; the 0003 per-side-asymmetry
// precedent). The FIRST band-based AssertDistribution.
//
//	conservation:  c1 + c2 + c3 == 64
//	starvation:    c1 <= starvationMax (=12)  (the most-held backend gets ~its held
//	                           conns + ~0 burst landings; under ROUND_ROBIN c1 == 21
//	                           → BITES the no-op-release break — Task 7 leg (ii))
//	concentration: c2 >= concentrationMin (=16)  (the two least-loaded split the
//	                           burst ~31±4 each; catches an INVERTED comparison where
//	                           c2 would be ~1 — Task 7 leg (i))
//
// The band CONSTANTS (starvationMax / concentrationMin) were finalized at Task 7
// against a >=20-run flake check (c1 always 2; c2 in 21-31); unchanged from the
// plan's 12/16. Three deliberate breaks PROVE each leg is live (README).
func (lrDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	for _, sd := range []struct {
		side   string
		counts []uint64
	}{{"reference", refCounts}, {"subject", subjCounts}} {
		if len(sd.counts) != 3 {
			return fmt.Errorf("%s: expected 3 backend counts, got %d", sd.side, len(sd.counts))
		}
		c := []uint64{sd.counts[0], sd.counts[1], sd.counts[2]}
		sort.Slice(c, func(i, j int) bool { return c[i] < c[j] }) // c[0] <= c[1] <= c[2]
		if c[0]+c[1]+c[2] != totalConns {
			return fmt.Errorf("%s: conservation: sum %d != %d", sd.side, c[0]+c[1]+c[2], totalConns)
		}
		if c[0] > starvationMax {
			return fmt.Errorf("%s: starvation: c1=%d > %d (no skew? round-robin?)", sd.side, c[0], starvationMax)
		}
		if c[1] < concentrationMin {
			return fmt.Errorf("%s: concentration: c2=%d < %d (inverted comparison?)", sd.side, c[1], concentrationMin)
		}
	}
	return nil
}

// AssertStats (post-drain): SPEC §7 — cross-equal upstream_cx_total==64 +
// membership_total==3 + upstream_cx_active==0 (quiesced); PER-SIDE upstream_rq_total
// (ref=64 — tcp_proxy charges rq-per-cx, AMEND-L2; subj=0 — envoy-go's tcpproxy
// path NEVER calls IncUpstreamRqTotal — a pre-existing documented boundary,
// AMEND-L4).
func (lrDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
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

	// Per-side upstream_rq_total (NOT cross-equal — AMEND-L4). The reference's
	// tcp_proxy charges one rq per cx (rq-per-cx, AMEND-L2) → 64; envoy-go's
	// tcpproxy path never Inc's upstream_rq_total → 0.
	const rqKey = "cluster." + clusterName + ".upstream_rq_total"
	if got := ref[rqKey]; got != totalConns {
		t.Errorf("ref %s = %d, want %d (rq-per-cx — AMEND-L2)", rqKey, got, totalConns)
	}
	if got := subj[rqKey]; got != 0 {
		t.Errorf("subj %s = %d, want 0 (tcpproxy never Inc's upstream_rq_total — AMEND-L4)", rqKey, got)
	}
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0057 driver scrapeStats,
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
	_ fixture.Driver               = (*lrDriver)(nil)
	_ fixture.DistributionAsserter = (*lrDriver)(nil)
	_ fixture.StatsAsserter        = (*lrDriver)(nil)
)
