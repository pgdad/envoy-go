// Package driver registers the 0055-redis-roundtrip cross-side differential
// fixture with the runner per phase 32.1 SPEC §8.1 + IMPL Task 13 (part 1:
// bootstraps + RESP request builders + the PING local-reply arm) and Task 14
// (AssertStats + the proxied-command arms).
//
// This is a CROSS-SIDE fixture: it boots BOTH the contrib reference Envoy
// (envoyproxy/envoy:contrib-v1.37.2) AND the envoy-go subprocess, drives the
// SAME RESP requests against both, and compares. The filter chain on BOTH
// sides is a [redis_proxy] TERMINAL listener (NO tcp_proxy behind it):
// redis_proxy terminates the downstream connection and owns the upstream dial.
//
// ============================================================================
// TASK 13 scope (PLAN §Task 13)
// ============================================================================
//
// Task 13 lands the driver skeleton with:
//   - The RESP request builders (shared with the 32.2 command-matrix arms).
//   - Both reference + subject bootstraps (redis_proxy terminal, STRICT_DNS /
//     STATIC cluster → TCPRedisResponder backend).
//   - The PING arm (local-reply; zero upstream): sends inline("PING") AND
//     respArray("PING") on ONE connection, captures each +PONG\r\n reply, and
//     returns the concatenated reply bytes so the runner's CompareBytes proves
//     cross-side byte-equivalence of the response the proxy GENERATED (§8.1.1).
//   - A stub AssertStats (Task 14 fills it in).
//
// ============================================================================
// Bootstraps (both sides)
// ============================================================================
//
// Listener l_redis: filter chain = [redis_proxy TERMINAL] — NO tcp_proxy.
// redis_proxy config: stat_prefix="redis_r", settings.op_timeout=5s,
// prefix_routes.catch_all_route.cluster="redis_cluster".
// Cluster redis_cluster → the runner's TCPRedisResponder backend.
// ≥1 cluster satisfies the zero-cluster boot reject
// (reference_network_filter_typeurl_extensions memory).
//
// # Cross-references
//
//   - phase 32.1 SPEC §8.1 (cross-side redis-roundtrip fixture scope)
//   - 32.1 PLAN Task 13 (this file, part 1) + Task 14 (AssertStats + arms)
//   - fixture-0053-kafka-requests (the STRUCTURAL TEMPLATE — cross-side
//     StatsAsserter, single-listener bootstrap, the 0051 arm-accounting
//     discipline, the prometheus scrape).
//   - reference_network_filter_typeurl_extensions (network-filter @type URLs
//     carry the extensions. segment; redis_proxy TypeURL confirmed via
//     proto.MessageName).
//   - reference_differential_asserter_dispatch (cross-side MUST use
//     StatsAsserter; SubjectAsserter would be a dead vacuous assertion).
//   - reference_wire_format_both_sides_see_same_bytes (the wire is shared;
//     the driver sends byte-identical RESP frames to both sides).
package driver

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0055-redis-roundtrip"

	refAdminPort = 9901

	// In-container reference Envoy listener port for l_redis.
	// 0053-kafka uses 19145; 0055-redis takes 19146.
	refLRedisPort = 19146

	// stat_prefix for the l_redis listener's redis_proxy config.
	statPrefixRedis = "redis_r"

	// readReplyTimeout bounds conn.Read calls when waiting for a reply.
	readReplyTimeout = 2 * time.Second

	// settleDelay lets the async stat pipeline on both sides catch up before
	// AssertStats scrapes (the 0051/0053 sleep-to-settle precedent).
	settleDelay = 750 * time.Millisecond
)

func init() {
	fixture.RegisterFixture(fixtureName, &redisRoundtripDriver{})
}

// redisRoundtripDriver carries no mutable cross-arm state. Each arm uses a
// fresh connection (the 0053 per-conn precedent).
type redisRoundtripDriver struct{}

// ============================================================================
// RESP request builders (D-S32.1-4 — shared with the 32.2 command-matrix arms)
// ============================================================================

// respBulk builds one "$<len>\r\n<bytes>\r\n" bulk string element.
func respBulk(s string) []byte {
	return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(s), s))
}

// respArray builds a RESP array-of-bulk-strings request frame.
// "*<n>\r\n" followed by n respBulk elements.
func respArray(parts ...string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "*%d\r\n", len(parts))
	for _, p := range parts {
		b.Write(respBulk(p))
	}
	return b.Bytes()
}

// inline builds an inline command line "<text>\r\n".
func inline(s string) []byte { return []byte(s + "\r\n") }

// ============================================================================
// fixture.Driver (required)
// ============================================================================

// BackendCount returns 1: a single TCPRedisResponder backend (redis_cluster).
func (*redisRoundtripDriver) BackendCount() int { return 1 }

// SubjectListenerName returns the single listener name (l_redis).
func (*redisRoundtripDriver) SubjectListenerName() string { return "l_redis" }

// ReferenceListenerPort returns the reference listener port (l_redis).
func (*redisRoundtripDriver) ReferenceListenerPort() int { return refLRedisPort }

// BackendKind returns TCPRedisResponder: the RESP-aware canned-response backend
// (Task 12; SPEC §8.3).
func (*redisRoundtripDriver) BackendKind() fixture.BackendKind { return fixture.TCPRedisResponder }

// ReferenceBootstrap renders the single-listener reference bootstrap.
// redis_cluster points at host.docker.internal:<backend> (STRICT_DNS) so the
// dockerized reference can reach the host-side responder backend.
func (*redisRoundtripDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("0.0.0.0, port_value: %d", refAdminPort),
		listenAddr:  "0.0.0.0",
		redisPort:   refLRedisPort,
		clusterType: "STRICT_DNS",
		dnsLine:     "      dns_lookup_family: V4_ONLY\n",
		backendHost: "host.docker.internal",
		backendPort: backendPorts[0],
		nodeLine:    "",
	})
}

// SubjectConfig renders the single-listener subject bootstrap.
func (*redisRoundtripDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("127.0.0.1, port_value: %d", subjAdminPort),
		listenAddr:  "127.0.0.1",
		redisPort:   subjListenerPort,
		clusterType: "STATIC",
		dnsLine:     "",
		backendHost: "127.0.0.1",
		backendPort: backendPorts[0],
		nodeLine:    "node: { id: envoy-go-subject-0055, cluster: envoy-go-differential }\n",
	})
}

// DriveReference / DriveSubject run the identical arm workload against each
// side's l_redis listener and return a side-independent verdict byte stream.
func (d *redisRoundtripDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "ref")
}

func (d *redisRoundtripDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (*redisRoundtripDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// ============================================================================
// driveProxy — the arm workload
// ============================================================================

// driveProxy runs the arms in declared order against the proxy listener at
// addr. Each arm uses a fresh connection. The "side" label is diagnostic-only
// and is NEVER written to the returned bytes, so equivalent behavior yields
// byte-identical output for the runner's CompareBytes gate.
func (d *redisRoundtripDriver) driveProxy(ctx context.Context, addr, side string) ([]byte, error) {
	var b bytes.Buffer

	// Arm 1 — PING local-reply (Task 13).
	// Send inline("PING") AND respArray("PING") on ONE connection, read each
	// +PONG\r\n reply. redis_proxy answers PING locally (zero upstream — AMEND-R5).
	// The concatenated reply bytes are written to the verdict stream so the
	// runner's CompareBytes proves cross-side byte-equivalence of the response
	// the proxy GENERATED (§8.1.1).
	pingReply, err := drivePingArm(ctx, addr)
	emitArmBytes(&b, side, "ping", pingReply, err)

	// Arm 2 — proxied SET/GET round-trip (Task 14, §8.1.1).
	// Send respArray("SET","foo","bar") then respArray("GET","foo") on ONE fresh
	// connection, read each reply in order. redis_proxy forwards these to the
	// TCPRedisResponder backend (lazy-dials exactly ONE upstream connection on
	// first SET — AMEND-R5 only PING is local). The backend returns:
	//   SET → +OK\r\n
	//   GET → $3\r\nbar\r\n
	// The concatenated reply bytes are written to the verdict stream so the
	// runner's CompareBytes proves cross-side byte-equivalence (§8.1.1).
	// This arm makes upstream_cx_total==1 and upstream_rq_total==2 on both sides.
	setGetReply, err := driveSetGetArm(ctx, addr)
	emitArmBytes(&b, side, "set-get", setGetReply, err)

	// Let the async stat pipeline settle before the runner scrapes in AssertStats.
	if err := sleepCtx(ctx, settleDelay); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// drivePingArm opens ONE fresh connection, sends inline("PING") then
// respArray("PING"), reads a +PONG\r\n reply after EACH write, and returns the
// concatenated reply bytes. This proves the proxy generates an identical RESP
// response on both sides for both PING request forms (byte-equivalence proof).
func drivePingArm(ctx context.Context, addr string) ([]byte, error) {
	dl := net.Dialer{}
	conn, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	var replies bytes.Buffer

	// Send inline PING and read reply.
	if _, err := conn.Write(inline("PING")); err != nil {
		return nil, fmt.Errorf("write inline PING: %w", err)
	}
	reply1, err := readReply(conn)
	if err != nil {
		return nil, fmt.Errorf("read reply to inline PING: %w", err)
	}
	replies.Write(reply1)

	// Send array PING and read reply.
	if _, err := conn.Write(respArray("PING")); err != nil {
		return nil, fmt.Errorf("write array PING: %w", err)
	}
	reply2, err := readReply(conn)
	if err != nil {
		return nil, fmt.Errorf("read reply to array PING: %w", err)
	}
	replies.Write(reply2)

	return replies.Bytes(), nil
}

// driveSetGetArm opens ONE fresh connection, sends respArray("SET","foo","bar")
// then respArray("GET","foo"), reads a reply after EACH write, and returns the
// concatenated reply bytes. The TCPRedisResponder returns +OK\r\n for SET and
// $3\r\nbar\r\n for GET. This proves the proxy forwards proxied data commands
// to the upstream and returns the backend's response byte-identically on both
// sides (§8.1.1). It also makes upstream_cx_total==1 (one lazy dial on SET)
// and upstream_rq_total==2 (SET + GET) on both sides for AssertStats.
func driveSetGetArm(ctx context.Context, addr string) ([]byte, error) {
	dl := net.Dialer{}
	conn, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	var replies bytes.Buffer

	// Send SET and read reply (+OK\r\n).
	if _, err := conn.Write(respArray("SET", "foo", "bar")); err != nil {
		return nil, fmt.Errorf("write SET: %w", err)
	}
	setReply, err := readReply(conn)
	if err != nil {
		return nil, fmt.Errorf("read reply to SET: %w", err)
	}
	replies.Write(setReply)

	// Send GET and read reply ($3\r\nbar\r\n).
	if _, err := conn.Write(respArray("GET", "foo")); err != nil {
		return nil, fmt.Errorf("write GET: %w", err)
	}
	getReply, err := readGetReply(conn)
	if err != nil {
		return nil, fmt.Errorf("read reply to GET: %w", err)
	}
	replies.Write(getReply)

	return replies.Bytes(), nil
}

// readGetReply reads a RESP bulk-string reply ($3\r\nbar\r\n) from conn.
// A bulk reply spans two lines: the $<len>\r\n length header followed by
// <bytes>\r\n data. The read is bounded by readReplyTimeout.
func readGetReply(conn net.Conn) ([]byte, error) {
	_ = conn.SetReadDeadline(time.Now().Add(readReplyTimeout))
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if n > 0 {
		return buf[:n], nil
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("zero bytes read for GET reply")
}

// readReply reads one RESP simple-string reply line ("<type><data>\r\n") from
// conn. The read is bounded by readReplyTimeout.
func readReply(conn net.Conn) ([]byte, error) {
	_ = conn.SetReadDeadline(time.Now().Add(readReplyTimeout))
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if n > 0 {
		return buf[:n], nil
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("zero bytes read")
}

// emitArmBytes writes the RESP reply bytes directly into the verdict buffer.
// The reply bytes ARE the cross-side equivalence signal for the PING arm
// (byte-equivalence of the proxy-generated response — §8.1.1). An error is
// logged to stderr and a sentinel "ERR" line is written instead.
func emitArmBytes(b *bytes.Buffer, side, name string, replyBytes []byte, err error) {
	if err != nil {
		fmt.Fprintf(b, "arm %s verdict=ERR err=%v\n", name, err)
		return
	}
	// Write the reply bytes directly — side-independent (both sides must produce
	// the identical +PONG\r\n sequence). The side label is diagnostic only.
	_ = side
	b.Write(replyBytes)
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

// ============================================================================
// fixture.StatsAsserter (Task 14 — D-S32.1-5)
// ============================================================================

// AssertStats scrapes the FLAT /stats admin endpoint from BOTH admin endpoints
// and asserts the redis_proxy counters after the two-arm workload (PING + SET/GET).
//
// # Asserted counters (both sides must be EQUAL)
//
// redis.redis_r.downstream_cx_total         — total downstream connections
// redis.redis_r.downstream_rq_total         — total downstream requests (PING×2 + SET + GET = 4)
// redis.redis_r.downstream_cx_rx_bytes_total — bytes received from downstream
// redis.redis_r.downstream_cx_tx_bytes_total — bytes sent to downstream
// cluster.redis_cluster.upstream_cx_total   — upstream connections (==1, lazy dial on SET)
// cluster.redis_cluster.upstream_rq_total   — upstream requests (==2: SET + GET)
//
// PING is handled locally by redis_proxy (AMEND-R5) → zero upstream requests for
// the PING arm; only the SET/GET arm triggers the lazy upstream dial.
//
// Scrape: GET http://<addr>/stats → flat text "name: value" lines (NOT /stats/prometheus;
// the redis. Prometheus tag-extractor arm is 32.2 — §8.1.2).
//
// R6 deliberate-break liveness proof (PLAN Task 14 Step 4, LIVE-VERIFIED):
//
// Each assertion was proven live by temporarily perturbing the driver and
// running `go test ./test/differential/ -run 'TestDifferential/0055' -count=1 -v`
// (always with -count=1 to defeat go test caching per reference_differential_break_protocol_count1).
//
//	Break 1 — downstream_rq_total (counter cross-side equality):
//	  Added `if name == "redis.redis_r.downstream_rq_total" && refVal != 99 { t.Errorf(...) }`
//	  → FAIL: "R6-BREAK ref redis.redis_r.downstream_rq_total = 4, want 99"
//	  Actual value confirmed: 4 (inline PING + array PING [arm 1] + SET + GET [arm 2]).
//	  Reverted → PASS.
//
//	Break 2 — upstream_rq_total (counter cross-side equality):
//	  Added `if name == "cluster.redis_cluster.upstream_rq_total" && refVal != 99 { t.Errorf(...) }`
//	  → FAIL: "R6-BREAK ref cluster.redis_cluster.upstream_rq_total = 2, want 99"
//	  Actual value confirmed: 2 (SET + GET forwarded to backend).
//	  Reverted → PASS.
//
//	Break 3 — SET/GET reply bytes (CompareBytes prong):
//	  Added `if side == "subj" && err == nil { setGetReply = append(setGetReply, '!') }` in driveProxy.
//	  → FAIL: "differential mismatch: first divergence at offset 28"
//	  Proves the byte-equivalence verdict is live — the runner's CompareBytes caught the 1-byte delta.
//	  Reverted → PASS.
//
//	Break 4 — PING reply bytes (CompareBytes prong):
//	  Added `if side == "subj" && err == nil { pingReply = append(pingReply, 'X') }` in driveProxy.
//	  → FAIL: "differential mismatch: first divergence at offset 14"
//	  Proves the PING byte-equivalence verdict is live.
//	  Reverted → PASS.
//
//	Break 5 — upstream_cx_total (counter cross-side equality):
//	  Added `if name == "cluster.redis_cluster.upstream_cx_total" && refVal != 99 { t.Errorf(...) }`
//	  → FAIL: "R6-BREAK ref cluster.redis_cluster.upstream_cx_total = 1, want 99"
//	  Actual value confirmed: 1 (one lazy dial on SET — AMEND-R5 PING is local, no upstream).
//	  Reverted → PASS.
func (d *redisRoundtripDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	ref, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}

	// The six counters that must be EQUAL on both sides after the two-arm workload.
	// stat_prefix is "redis_r" (const statPrefixRedis); cluster name is "redis_cluster".
	//
	// downstream_rq_total == 4: inline PING + array PING (arm 1) + SET + GET (arm 2).
	// upstream_cx_total   == 1: one lazy dial (SET is the first proxied command).
	// upstream_rq_total   == 2: SET + GET forwarded to backend.
	counters := []string{
		"redis." + statPrefixRedis + ".downstream_cx_total",
		"redis." + statPrefixRedis + ".downstream_rq_total",
		"redis." + statPrefixRedis + ".downstream_cx_rx_bytes_total",
		"redis." + statPrefixRedis + ".downstream_cx_tx_bytes_total",
		"cluster.redis_cluster.upstream_cx_total",
		"cluster.redis_cluster.upstream_rq_total",
	}

	for _, name := range counters {
		refVal, refOK := ref[name]
		subjVal, subjOK := subj[name]
		if !refOK {
			t.Errorf("ref: counter %s ABSENT in /stats", name)
			continue
		}
		if !subjOK {
			t.Errorf("subj: counter %s ABSENT in /stats", name)
			continue
		}
		if refVal != subjVal {
			t.Errorf("cross-side mismatch %s: ref=%d subj=%d", name, refVal, subjVal)
		}
	}
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text, NOT
// /stats/prometheus) and parses "name: value" lines into a map[name]uint64.
// All stat names are retained (the caller filters by checking the desired keys).
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
		// Flat /stats format: "name: value" (colon-space separator).
		idx := strings.LastIndex(line, ": ")
		if idx < 0 {
			continue
		}
		name := line[:idx]
		valStr := strings.TrimSpace(line[idx+2:])
		v, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue // skip non-numeric (histograms, gauges with special formats)
		}
		out[name] = v
	}
	return out, nil
}

// ============================================================================
// Bootstrap rendering
// ============================================================================

type bootstrapParams struct {
	adminAddr   string // "<ip>, port_value: <n>" for the admin socket_address
	listenAddr  string // listener bind address (0.0.0.0 for ref; 127.0.0.1 for subj)
	redisPort   int    // l_redis listener port
	clusterType string // STRICT_DNS (ref) | STATIC (subj)
	dnsLine     string // "      dns_lookup_family: V4_ONLY\n" for STRICT_DNS, else ""
	backendHost string
	backendPort int
	nodeLine    string // "node: {...}\n" for subj, "" for ref
}

// redisProxyType is the redis_proxy typed_config @type URL. The network-filter
// type URLs carry the extensions. segment
// (reference_network_filter_typeurl_extensions). redis_proxy is a CORE /envoy
// extension (AMEND-R1; ZERO new go.mod dep); the FQN is
// envoy.extensions.filters.network.redis_proxy.v3.RedisProxy.
const redisProxyType = "type.googleapis.com/envoy.extensions.filters.network.redis_proxy.v3.RedisProxy"

// renderBootstrap assembles the single-listener bootstrap. The l_redis filter
// chain is [redis_proxy TERMINAL] — NO tcp_proxy. redis_cluster points at the
// TCPRedisResponder backend AND satisfies the zero-cluster boot reject
// (reference_network_filter_typeurl_extensions memory).
func renderBootstrap(p bootstrapParams) string {
	redisListener := fmt.Sprintf(`    - name: l_redis
      address:
        socket_address: { address: %s, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.redis_proxy
              typed_config:
                "@type": %s
                stat_prefix: %s
                settings:
                  op_timeout: 5s
                prefix_routes:
                  catch_all_route:
                    cluster: redis_cluster
`, p.listenAddr, p.redisPort, redisProxyType, statPrefixRedis)

	return fmt.Sprintf(`%sadmin:
  address:
    socket_address: { address: %s }
static_resources:
  listeners:
%s  clusters:
    - name: redis_cluster
      type: %s
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s      load_assignment:
        cluster_name: redis_cluster
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: %s, port_value: %d }
`,
		p.nodeLine,
		p.adminAddr,
		redisListener,
		p.clusterType,
		p.dnsLine,
		p.backendHost, p.backendPort,
	)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*redisRoundtripDriver)(nil)
	_ fixture.BackendKindAware = (*redisRoundtripDriver)(nil)
	_ fixture.StatsAsserter    = (*redisRoundtripDriver)(nil)
)
