// Package driver registers the 0057-thrift-roundtrip cross-side differential
// fixture with the runner per phase 33 SPEC §8.1 + PLAN Task 12.
//
// This is a CROSS-SIDE fixture: it boots BOTH the contrib reference Envoy
// (envoyproxy/envoy:contrib-v1.37.2) AND the envoy-go subprocess, drives the
// SAME framed-binary Thrift CALL frames against both, and compares. The filter
// chain on BOTH sides is a [thrift_proxy] TERMINAL listener (NO tcp_proxy behind
// it): thrift_proxy terminates the downstream connection and owns the upstream
// dial (the redis_proxy 0055 precedent).
//
// ============================================================================
// Bootstraps (both sides)
// ============================================================================
//
// Listener l_thrift: filter chain = [thrift_proxy TERMINAL] — NO tcp_proxy.
// thrift_proxy config (SPEC §11.2 working YAML, reusable verbatim):
//
//	stat_prefix: thrift_r
//	transport:   FRAMED
//	protocol:    BINARY
//	payload_passthrough: true
//	route_config: routes:
//	  - { match: { method_name: "Ping" }, route: { cluster: thrift_cluster } }
//	  - { match: { method_name: "boom" }, route: { cluster: thrift_cluster } }
//
// The route table is DELIBERATELY method-keyed (NO match-all "") so that:
//   - "Ping" HITS  → round-trips → backend REPLY void-success (arm 1).
//   - "boom" HITS  → round-trips → backend EXCEPTION reply (arm 3, the Task-11
//     thriftMarkerException trigger).
//   - "Pong" MISSES → local UnknownMethod exception, NO dial (arm 2).
//
// Cluster thrift_cluster → the runner's TCPThriftResponder backend (BackendKind
// 33, Task 11). ≥1 cluster satisfies the zero-cluster boot reject
// (reference_network_filter_typeurl_extensions memory).
//
// # Cross-references
//
//   - phase 33 SPEC §8.1 (cross-side thrift-roundtrip fixture scope) + §11.2
//     (the working bootstrap YAML) + Appendix A (the framed×binary wire format).
//   - 0055-redis-roundtrip (the STRUCTURAL TEMPLATE — cross-side StatsAsserter,
//     single-listener bootstrap, STRICT_DNS reference / STATIC subject, the
//     flat /stats scrape, the deliberate-break liveness discipline).
//   - reference_network_filter_typeurl_extensions (network-filter @type URLs
//     carry the extensions. segment; thrift_proxy TypeURL confirmed via
//     proto.MessageName — D-T1).
//   - reference_differential_asserter_dispatch (cross-side MUST use
//     StatsAsserter; SubjectAsserter would be a dead vacuous assertion).
//   - reference_wire_format_both_sides_see_same_bytes (the wire is shared;
//     the driver sends byte-identical framed-binary frames to both sides).
//   - reference_close_direction_framework_gap (the miss-arm
//     cx_destroy_local_with_active_rq / downstream_response_drain_close are
//     per-side, NOT cross-equal — the reference moves them, the subject pins 0).
//   - reference_differential_reference_parses_full_message (send FULLY-VALID
//     frames; the reference parses the whole message).
package driver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
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
	fixtureName = "0057-thrift-roundtrip"

	refAdminPort = 9901

	// In-container reference Envoy listener port for l_thrift.
	// 0055-redis takes 19146; 0057-thrift takes 19147.
	refLThriftPort = 19147

	// stat_prefix for the l_thrift listener's thrift_proxy config.
	statPrefixThrift = "thrift_r"

	// clusterName is the single upstream cluster the routes point at.
	clusterName = "thrift_cluster"

	// readReplyTimeout bounds conn.Read calls when waiting for a reply frame.
	readReplyTimeout = 2 * time.Second

	// settleDelay lets the async stat pipeline on both sides catch up before
	// AssertStats scrapes (the 0055 sleep-to-settle precedent).
	settleDelay = 750 * time.Millisecond
)

func init() {
	fixture.RegisterFixture(fixtureName, &thriftRoundtripDriver{})
}

// thriftRoundtripDriver is stateless — all arms use a fresh connection (the 0055
// per-conn precedent). request_active is asserted QUIESCED-to-0 post-workload
// (D-S33-3), so NO held-open connection is needed.
type thriftRoundtripDriver struct{}

// ============================================================================
// Framed-binary Thrift frame builders (Appendix A — DUPLICATED here, NOT
// imported from internal/filter/network/thriftproxy; the 0055 self-contained
// redis-builder precedent + the runner's thriftReqFrame precedent).
// ============================================================================

const (
	binaryMagic      = 0x8001 // strict binary protocol version magic
	msgTypeCall      = 1
	msgTypeReply     = 2
	msgTypeException = 3
)

// thriftCallFrame builds a framed-binary CALL request frame (Appendix A):
// 4-byte BE frame-length + binary message-begin (magic 0x8001 + zero + CALL(1) +
// i32 name-len + name + i32 seq_id) + a single STOP(0x00) void body. The same
// wire format the runner's thriftReqFrame builds (the responder reads it).
func thriftCallFrame(method string, seqID int32) []byte {
	var p []byte
	p = append(p, 0x80, 0x01, 0x00, msgTypeCall)
	p = appendI32(p, int32(len(method)))
	p = append(p, method...)
	p = appendI32(p, seqID)
	p = append(p, 0x00) // STOP — void body
	frame := appendI32(nil, int32(len(p)))
	return append(frame, p...)
}

func appendI32(b []byte, v int32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// ============================================================================
// fixture.Driver (required)
// ============================================================================

// BackendCount returns 1: a single TCPThriftResponder backend (thrift_cluster).
func (*thriftRoundtripDriver) BackendCount() int { return 1 }

// SubjectListenerName returns the single listener name (l_thrift).
func (*thriftRoundtripDriver) SubjectListenerName() string { return "l_thrift" }

// ReferenceListenerPort returns the reference listener port (l_thrift).
func (*thriftRoundtripDriver) ReferenceListenerPort() int { return refLThriftPort }

// BackendKind returns TCPThriftResponder: the framed-binary canned-response
// backend (Task 11; SPEC §8.3).
func (*thriftRoundtripDriver) BackendKind() fixture.BackendKind {
	return fixture.TCPThriftResponder
}

// ReferenceBootstrap renders the single-listener reference bootstrap.
// thrift_cluster points at host.docker.internal:<backend> (STRICT_DNS) so the
// dockerized reference can reach the host-side responder backend
// (reference_docker_probe_bridge_network — mirror 0055).
func (*thriftRoundtripDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("0.0.0.0, port_value: %d", refAdminPort),
		listenAddr:  "0.0.0.0",
		thriftPort:  refLThriftPort,
		clusterType: "STRICT_DNS",
		dnsLine:     "      dns_lookup_family: V4_ONLY\n",
		backendHost: "host.docker.internal",
		backendPort: backendPorts[0],
		nodeLine:    "",
	})
}

// SubjectConfig renders the single-listener subject bootstrap.
func (*thriftRoundtripDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("127.0.0.1, port_value: %d", subjAdminPort),
		listenAddr:  "127.0.0.1",
		thriftPort:  subjListenerPort,
		clusterType: "STATIC",
		dnsLine:     "",
		backendHost: "127.0.0.1",
		backendPort: backendPorts[0],
		nodeLine:    "node: { id: envoy-go-subject-0057, cluster: envoy-go-differential }\n",
	})
}

// DriveReference / DriveSubject run the identical arm workload against each
// side's l_thrift listener and return a side-independent verdict byte stream.
func (d *thriftRoundtripDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr)
}

func (d *thriftRoundtripDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (*thriftRoundtripDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// driveProxy runs the three arms in declared order against the proxy listener at
// addr. Each arm uses a FRESH connection (the framed single-flight one-call-per-
// connection MVP contract — SPEC §8 caveat (iii)). The returned bytes are the
// concatenated downstream reply frames (side-independent), so equivalent behavior
// yields byte-identical output for the runner's CompareBytes gate (§8.1.1).
func (d *thriftRoundtripDriver) driveProxy(ctx context.Context, addr string) ([]byte, error) {
	var b bytes.Buffer

	// Arm 1 — route-HIT round-trip. CALL Ping seq 1 → matches the "Ping" route →
	// round-trips to the backend → backend echoes a framed-binary REPLY (void
	// success: msgtype 2, method "Ping", seq 1, single-STOP body) → the proxy
	// forwards it downstream byte-identically (§8.1.1). Drives request/request_call/
	// request_passthrough/response/response_reply/response_success/response_passthrough
	// +1 and cluster.<name>.upstream_cx_total/upstream_rq_total +1 (cross-equal).
	hitReply, err := driveOneCall(ctx, addr, thriftCallFrame("Ping", 1))
	if err != nil {
		return nil, fmt.Errorf("arm hit (Ping): %w", err)
	}
	b.Write(hitReply)

	// Arm 2 — route-MISS local-exception. CALL Pong seq 2 → matches NEITHER route
	// (no match-all) → local UnknownMethod EXCEPTION (msgtype 3, AppException
	// {1: "no route for method 'Pong'", 2: i32 1}), NO backend dial. The downstream
	// EXCEPTION bytes are byte-identical cross-side (Appendix A). Drives route_missing/
	// response_exception +1; cluster upstream stays 0. The reference ALSO moves
	// cx_destroy_local_with_active_rq + downstream_response_drain_close (per-side,
	// NOT cross-asserted — the subject keeps its local-reply conn open, AMEND-T6).
	missReply, err := driveOneCall(ctx, addr, thriftCallFrame("Pong", 2))
	if err != nil {
		return nil, fmt.Errorf("arm miss (Pong): %w", err)
	}
	b.Write(missReply)

	// Arm 3 — reply-EXCEPTION (D-S33-2). CALL boom seq 3 → matches the "boom" route →
	// round-trips to the backend → the TCPThriftResponder (Task 11) answers an
	// EXCEPTION (msgtype 3) carrying AppException {1: "backend exception", 2: i32 6}
	// → the proxy forwards it downstream byte-identically. Drives a SECOND
	// response_exception (distinct from arm 2's LOCAL miss exception) + a second
	// upstream cx/rq. This is the backend-exception path the local-miss path can NOT
	// reach.
	excReply, err := driveOneCall(ctx, addr, thriftCallFrame("boom", 3))
	if err != nil {
		return nil, fmt.Errorf("arm reply-exception (boom): %w", err)
	}
	b.Write(excReply)

	// Let the async stat pipeline settle before the runner scrapes in AssertStats.
	if err := sleepCtx(ctx, settleDelay); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// driveOneCall opens ONE fresh connection, writes req (a single framed-binary
// CALL), reads ONE complete framed-binary reply frame (length prefix + payload),
// and returns the FULL reply frame bytes (the cross-side byte-equivalence signal —
// both sides must produce identical frames). The framed single-flight MVP answers
// exactly one reply per call.
func driveOneCall(ctx context.Context, addr string, req []byte) ([]byte, error) {
	dl := net.Dialer{}
	conn, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write(req); err != nil {
		return nil, fmt.Errorf("write call: %w", err)
	}
	return readFrame(conn)
}

// readFrame reads ONE complete framed-binary Thrift frame: the 4-byte BE length
// prefix followed by exactly that many payload bytes. The full frame (prefix +
// payload) is returned so CompareBytes proves the WHOLE downstream reply frame is
// byte-identical cross-side. Bounded by readReplyTimeout.
func readFrame(conn net.Conn) ([]byte, error) {
	_ = conn.SetReadDeadline(time.Now().Add(readReplyTimeout))
	r := bufio.NewReader(conn)
	var lenPfx [4]byte
	if _, err := io.ReadFull(r, lenPfx[:]); err != nil {
		return nil, fmt.Errorf("read frame length: %w", err)
	}
	n := int64(binary.BigEndian.Uint32(lenPfx[:]))
	if n < 12 || n > 1<<20 {
		return nil, fmt.Errorf("frame length %d out of range", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("read frame payload (%d bytes): %w", n, err)
	}
	frame := make([]byte, 0, 4+n)
	frame = append(frame, lenPfx[:]...)
	frame = append(frame, payload...)
	return frame, nil
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
// fixture.StatsAsserter (SPEC §8.1 arms 1-3; the load-bearing cross-side prong)
// ============================================================================

// AssertStats scrapes the FLAT /stats admin endpoint from BOTH admin endpoints
// and asserts the thrift_proxy roster after the three-arm workload (HIT Ping +
// MISS Pong + reply-EXCEPTION boom).
//
// # Cross-side EQUAL counters (both sides identical)
//
// thrift.thrift_r.request                 — 3 (Ping + Pong + boom; every serviced request)
// thrift.thrift_r.request_call            — 3 (all three are CALL msgtype)
// thrift.thrift_r.request_passthrough     — 3 (payload_passthrough)
// thrift.thrift_r.response                — 2 (Ping REPLY + boom EXCEPTION; the miss has no upstream response)
// thrift.thrift_r.response_reply          — 1 (Ping REPLY void-success)
// thrift.thrift_r.response_success        — 1 (Ping void-success)
// thrift.thrift_r.response_passthrough    — 2 (Ping + boom upstream replies)
// thrift.thrift_r.response_exception      — 2 (Pong LOCAL miss + boom BACKEND exception)
// thrift.thrift_r.route_missing           — 1 (Pong)
// cluster.thrift_cluster.upstream_cx_total — 2 (Ping + boom dial; cross-equal D-T9b)
// cluster.thrift_cluster.upstream_rq_total — 2 (Ping + boom; cross-equal D-T9b)
//
// # Decode-ran witness (SPEC §8 caveat (i))
//
// thrift_proxy emits NO listener downstream_cx_rx_bytes_total (that is an HCM
// stat); the round-trip-ran proof is cluster.thrift_cluster.upstream_cx_rx_bytes_total
// > 0 AND thrift.thrift_r.request_call > 0 (asserted below on BOTH sides).
//
// # Per-side coverage boundary (NOT cross-equal — reference_close_direction_framework_gap)
//
// On the MISS arm the REFERENCE moves cx_destroy_local_with_active_rq +
// downstream_response_drain_close (it FlushWrite-closes after the local exception
// with the rq active); the SUBJECT keeps its local-reply connection OPEN (no
// drain-close) so both stay 0. These are asserted PER-SIDE (subject == 0; the
// reference is NOT asserted — it legitimately moves them, AMEND-T6 / D-T8).
//
// # request_active gauge (D-S33-3)
//
// Quiesced-to-0 post-workload on BOTH sides (the redis downstream_rq_active
// precedent). Asserted PRESENT (eager-created) AND == 0 (a non-present check would
// pass vacuously). The reference's request_active is NOT a counter so it lives in
// flat /stats as "thrift.thrift_r.request_active: 0".
//
// # R-break liveness proof (PLAN Task 12 Step 4, LIVE-VERIFIED with -count=1)
//
// Per reference_differential_break_protocol_count1 (go-test caching defeated with
// -count=1). DRIVER-only and PRODUCTION breaks recorded in the README; one
// production break (filter.go response_success inc commented out) was run to prove
// the cross-side StatsAsserter is wired to the real subject counters, then RESTORED
// and the production tree verified unchanged via git.
func (d *thriftRoundtripDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	ref, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}

	sp := "thrift." + statPrefixThrift + "."

	// ── Cross-side EQUAL counters ───────────────────────────────────────────────
	// These move identically on BOTH sides after the three-arm workload (the wire
	// is shared; both proxies classify the same frames identically). The WANT
	// values are pinned exactly (non-vacuous, R-break-able per side) AND the
	// cross-side equality is asserted (the load-bearing two-pronged proof).
	//
	// response            = 2 — Ping REPLY + boom EXCEPTION (the Pong miss has no upstream response).
	// response_reply      = 1 — Ping REPLY (boom is an EXCEPTION, not a REPLY).
	// response_success    = 1 — Ping void-success.
	// response_passthrough= 2 — Ping + boom upstream replies (payload_passthrough).
	// response_exception  = 2 — Pong LOCAL miss + boom BACKEND exception (the two distinct exception paths).
	// route_missing       = 1 — Pong.
	// upstream_rq_total   = 2 — Ping + boom (request COUNT is pooling-independent — cross-equal D-T9b).
	type counterPin struct {
		name string
		want uint64
	}
	crossEqual := []counterPin{
		{sp + "response", 2},
		{sp + "response_reply", 1},
		{sp + "response_success", 1},
		{sp + "response_passthrough", 2},
		{sp + "response_exception", 2},
		{sp + "route_missing", 1},
		{"cluster." + clusterName + ".upstream_rq_total", 2},
	}
	for _, p := range crossEqual {
		refVal, refOK := ref[p.name]
		subjVal, subjOK := subj[p.name]
		if !refOK {
			t.Errorf("ref: counter %s ABSENT in /stats", p.name)
			continue
		}
		if !subjOK {
			t.Errorf("subj: counter %s ABSENT in /stats", p.name)
			continue
		}
		// Cross-side equality (the wire is shared) AND the exact pin (non-vacuous).
		if refVal != subjVal {
			t.Errorf("cross-side mismatch %s: ref=%d subj=%d", p.name, refVal, subjVal)
		}
		if refVal != p.want {
			t.Errorf("ref %s = %d, want %d", p.name, refVal, p.want)
		}
		if subjVal != p.want {
			t.Errorf("subj %s = %d, want %d", p.name, subjVal, p.want)
		}
	}

	// ── PER-SIDE request* divergence (NOT cross-equal) ──────────────────────────
	// The REFERENCE does NOT count request/request_call/request_passthrough on a
	// routing MISS (the miss is accounted ONLY via route_missing+response_exception
	// — SPEC §7.3 / D-T5, live-confirmed: "request* do NOT move" on the miss). So
	// the reference counts ONLY the two HIT calls (Ping + boom) → 2. The SUBJECT's
	// pump increments request*/request_call/request_passthrough at the TOP of
	// serveRequest, BEFORE the route match (PLAN Task 8 "decode → count request →
	// match route"), so it counts ALL THREE calls (Ping + Pong + boom) → 3. This is
	// a documented per-side behavioral divergence (the reference's miss-not-counted
	// semantics vs the subject's count-before-match pump), NOT a subject bug —
	// pinned EXACTLY per side so each is non-vacuous and R-break-able.
	for _, suf := range []string{"request", "request_call", "request_passthrough"} {
		if got := ref[sp+suf]; got != 2 {
			t.Errorf("ref %s%s = %d, want 2 (HIT-only: Ping+boom; miss not counted — SPEC §7.3)", sp, suf, got)
		}
		if got := subj[sp+suf]; got != 3 {
			t.Errorf("subj %s%s = %d, want 3 (count-before-match: Ping+Pong+boom)", sp, suf, got)
		}
	}

	// ── PER-SIDE upstream_cx_total pooling divergence (NOT cross-equal) ──────────
	// The REFERENCE pools upstream connections at the cluster level → ONE reused
	// upstream conn serves both proxied calls (Ping + boom) → upstream_cx_total == 1.
	// The SUBJECT uses the one-conn-per-downstream upstream seam (each fresh
	// downstream conn lazily dials its OWN dedicated upstream) → the 2 distinct
	// HIT downstream conns each dial 1 upstream → upstream_cx_total == 2. The
	// redis D-P32-9 per-side pooling precedent (D-T9b): the request COUNT
	// (upstream_rq_total == 2) is pooling-independent and stays cross-equal above;
	// only the CONNECTION count diverges. Pinned EXACTLY per side.
	const cxKey = "cluster." + clusterName + ".upstream_cx_total"
	if got := ref[cxKey]; got != 1 {
		t.Errorf("ref %s = %d, want 1 (pooled: one reused upstream conn — D-T9b)", cxKey, got)
	}
	if got := subj[cxKey]; got != 2 {
		t.Errorf("subj %s = %d, want 2 (one-conn-per-downstream: Ping+boom — D-T9b)", cxKey, got)
	}

	// ── Decode-ran witness (SPEC §8 caveat (i)) ─────────────────────────────────
	// The round-trip ACTUALLY ran. thrift_proxy emits NO listener
	// downstream_cx_rx_bytes_total (that is an HCM stat). The witnesses:
	//   - REFERENCE: cluster.<name>.upstream_cx_rx_bytes_total > 0 (the cluster
	//     received reply bytes from the backend; SPEC §8 caveat — ref=73 observed).
	//     The SUBJECT's cluster package does NOT emit upstream_cx_rx_bytes_total
	//     (SPEC §7.5 — the subject reuses only upstream_cx_total/upstream_rq_total),
	//     so this witness is reference-side only.
	//   - BOTH sides: request_call > 0 (a CALL was decoded + serviced).
	const rxBytesKey = "cluster." + clusterName + ".upstream_cx_rx_bytes_total"
	if got, ok := ref[rxBytesKey]; !ok || got == 0 {
		t.Errorf("ref %s = %d (present=%v), want > 0 (round-trip-ran witness)", rxBytesKey, got, ok)
	}
	for _, sd := range []struct {
		label string
		stats map[string]uint64
	}{{"ref", ref}, {"subj", subj}} {
		if got := sd.stats[sp+"request_call"]; got == 0 {
			t.Errorf("%s: %srequest_call = 0, want > 0 (decode-ran witness)", sd.label, sp)
		}
	}

	// request_active gauge — QUIESCED to 0 post-workload (D-S33-3). Assert PRESENT
	// (eager-created) AND == 0 on BOTH sides (a non-present check would pass
	// vacuously). The flat /stats renders the gauge as "thrift.thrift_r.request_active: 0".
	for _, sd := range []struct {
		label string
		stats map[string]uint64
	}{{"ref", ref}, {"subj", subj}} {
		got, ok := sd.stats[sp+"request_active"]
		if !ok {
			t.Errorf("%s: %srequest_active ABSENT (eager-created — should render)", sd.label, sp)
		} else if got != 0 {
			t.Errorf("%s: %srequest_active = %d, want 0 (quiesced post-workload)", sd.label, sp, got)
		}
	}

	// Per-side close-direction coverage boundary (reference_close_direction_framework
	// _gap / AMEND-T6 / D-T8). On the MISS arm the REFERENCE moves these (FlushWrite-
	// close after the local exception with the rq active); the SUBJECT keeps its
	// local-reply conn OPEN so both stay 0. Asserted SUBJECT == 0 ONLY (PRESENT, since
	// eager-created — non-vacuous: proves the subject renders the counter AND never
	// spuriously incremented it). The reference is NOT asserted (it legitimately
	// moves them). NOT cross-equal.
	for _, suf := range []string{"cx_destroy_local_with_active_rq", "downstream_response_drain_close"} {
		got, ok := subj[sp+suf]
		if !ok {
			t.Errorf("subj: %s%s ABSENT (eager-created — should render)", sp, suf)
		} else if got != 0 {
			t.Errorf("subj: %s%s = %d, want 0 (subject-side coverage boundary, AMEND-T6)", sp, suf, got)
		}
	}
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text, NOT
// /stats/prometheus) and parses "name: value" lines into a map[name]uint64.
// All stat names are retained (the caller filters by checking the desired keys).
// (The 0055 redis-driver scrapeStats, verbatim.)
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
			continue // skip non-numeric (histograms, special formats)
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
	thriftPort  int    // l_thrift listener port
	clusterType string // STRICT_DNS (ref) | STATIC (subj)
	dnsLine     string // "      dns_lookup_family: V4_ONLY\n" for STRICT_DNS, else ""
	backendHost string
	backendPort int
	nodeLine    string // "node: {...}\n" for subj, "" for ref
}

// thriftProxyType is the thrift_proxy typed_config @type URL. The network-filter
// type URLs carry the extensions. segment
// (reference_network_filter_typeurl_extensions). thrift_proxy is a CORE /envoy
// extension (D-T1; ZERO new go.mod dep); the FQN is
// envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy.
const thriftProxyType = "type.googleapis.com/envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy"

// renderBootstrap assembles the single-listener bootstrap. The l_thrift filter
// chain is [thrift_proxy TERMINAL] — NO tcp_proxy. The route table is method-keyed
// (Ping + boom → thrift_cluster; NO match-all so Pong misses). thrift_cluster
// points at the TCPThriftResponder backend AND satisfies the zero-cluster boot
// reject (reference_network_filter_typeurl_extensions memory).
func renderBootstrap(p bootstrapParams) string {
	thriftListener := fmt.Sprintf(`    - name: l_thrift
      address:
        socket_address: { address: %s, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.thrift_proxy
              typed_config:
                "@type": %s
                stat_prefix: %s
                transport: FRAMED
                protocol: BINARY
                payload_passthrough: true
                route_config:
                  name: thrift_routes
                  routes:
                    - match: { method_name: "Ping" }
                      route: { cluster: %s }
                    - match: { method_name: "boom" }
                      route: { cluster: %s }
`, p.listenAddr, p.thriftPort, thriftProxyType, statPrefixThrift, clusterName, clusterName)

	return fmt.Sprintf(`%sadmin:
  address:
    socket_address: { address: %s }
static_resources:
  listeners:
%s  clusters:
    - name: %s
      type: %s
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s      load_assignment:
        cluster_name: %s
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: %s, port_value: %d }
`,
		p.nodeLine,
		p.adminAddr,
		thriftListener,
		clusterName,
		p.clusterType,
		p.dnsLine,
		clusterName,
		p.backendHost, p.backendPort,
	)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*thriftRoundtripDriver)(nil)
	_ fixture.BackendKindAware = (*thriftRoundtripDriver)(nil)
	_ fixture.StatsAsserter    = (*thriftRoundtripDriver)(nil)
)
