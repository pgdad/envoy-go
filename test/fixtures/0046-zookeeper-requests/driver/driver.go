// Package driver registers the 0046-zookeeper-requests cross-side differential
// fixture with the runner per phase 28.1 SPEC §8.1 + PLAN Task 15 (part 1)
// and Task 16 (part 2).
//
// ============================================================================
// DISABLED at 28.1a — re-enabled at 28.1b
// ============================================================================
//
// This driver's blank-import in test/differential/runner_test.go is commented
// out at the 28.1a closure (the ADR-0045 28.1a/28.1b split, user-approved
// 2026-06-02). The fixture code is correct but its multi-frame arms (2, 3, 4)
// are RED on envoy-go: the network chain runtime exits its read loop
// permanently at terminal handoff (manager.go serveNetworkChain:
// TerminalReady() -> HandleTerminal() -> return), so a
// [zookeeper_proxy, tcp_proxy] chain delivers only the FIRST socket read's
// bytes to zookeeper_proxy's OnData. Reference Envoy re-iterates read filters
// on every read for the connection's lifetime, so multi-frame connections show
// cross-side stat divergence (reference counts all frames; envoy-go counts
// only the first). 28.1b designs the symmetric READ-side seam (the SPEC
// designed only the WriteFilter seam), then re-enables this import and proves
// the fixture green. See PROGRESS.md Task 16 BLOCKED analysis + the 28.1a
// closure entry.
//
// It is the FIRST cross-side fixture for the
// zookeeper_proxy network filter (ADR-0222): each listener's filter chain is
// [zookeeper_proxy, tcp_proxy] targeting a SILENT TCP sink backend
// (BackendKind=TCPSink; D-S28.1-5), and the driver asserts per-opcode counter
// parity across both reference Envoy v1.37.2 (dockerized) and envoy-go via the
// StatsAsserter interface.
//
// # Backend choice: TCPSink (not TCPEcho)
//
// A TCPEcho backend would push the echoed ZK request bytes back through
// reference Envoy's zookeeper_proxy onWrite response decoder, counting
// *_resp / decoder_error increments that envoy-go's 28.1 OnWrite no-op stub
// never mirrors → cross-side stat divergence. A silent sink drains reads
// without writing, so no response bytes traverse the filter chain on either
// side, and the 28.1 scope is request-only (SPEC §8.1.1).
//
// # Two listeners, two stat_prefixes
//
//   - l_plain: stat_prefix=zk_plain, all flags default (false). Exercises the
//     basic request counters (connect_rq, ping_rq, getdata_rq, create_rq,
//     close_rq, etc.) without per-opcode bytes or per-opcode decoder_error
//     metrics — the flag-off path.
//
//   - l_flags: stat_prefix=zk_flags, enable_per_opcode_request_bytes_metrics=
//     true, enable_per_opcode_decoder_error_metrics=true. Exercises the
//     flag-gated per-opcode *_rq_bytes and *_decoder_error counters — the
//     flag-on path. The global request_bytes is always present; the per-opcode
//     getdata_rq_bytes is only created (eagerly) + incremented when the flag is
//     true.
//
// # Seven-arm fixture partition (PLAN Task 16 arm table is authoritative)
//
// Arms drive the listener named in each entry — NOT necessarily both listeners.
// Each arm drives BOTH sides identically (DriveReferenceMulti / DriveSubjectMulti
// both call driveProxy) and emits a side-independent verdict line so equivalent
// behavior produces byte-identical drive output.
//
//   - Arm 1 (connect): l_plain only — one fresh connection, one
//     connectFrame(false) → zk_plain.zookeeper.connect_rq == 1 on both sides.
//
//   - Arm 2 (multi-opcode): l_plain — one connection: connect + ping +
//     getdata(xid 1) + create(xid 2) + close(xid 3), each sent as a separate
//     write with ~50 ms between writes. Asserts cumulative per-opcode counters
//     (connect_rq==2 cumulative with arm 1) AND request_bytes cross-side
//     equality (the wire-footprint-as-bytes discipline per SPEC §4.5 item 4 —
//     arm 2 is the load-bearing equality proof for request_bytes).
//
//   - Arm 3 (digit-suffixed): l_plain — one connection: create2(xid 4) +
//     getchildren2(xid 5) + setwatches2(xid 6, sent as a DATA request with a
//     positive xid → wire op 105; the setWatchesXid=-8 special path is NOT
//     exercised here). Asserts create2_rq==1, getchildren2_rq==1,
//     setwatches2_rq==1.
//
//   - Arm 4 (garbage + survival): l_plain — one connection: oversized length
//     prefix (2 MiB > the 1 MiB default max_packet_bytes → decoder_error) →
//     pause ≥200 ms → valid getdata ON THE SAME CONNECTION. Asserts
//     decoder_error==1 + getdata_rq==2 (cumulative: arm 2's getdata is the
//     first). The connection is NOT closed: the AMEND-A8 no-resync path
//     abandons the buffer but leaves the connection open, and the post-garbage
//     getdata (appended past the decoder high-water mark) still decodes.
//
//   - Arm 5 (flag-gated bytes): l_flags only — one getdata → asserts
//     zk_flags.zookeeper.getdata_rq_bytes > 0 on both sides (cross-side
//     equality). zk_plain.zookeeper.getdata_rq_bytes == 0 (the flag is off on
//     l_plain, so the counter was created but never incremented).
//
//   - Arm 6 (exists-at-zero): no traffic (assertion-only — lives in
//     AssertStats). The response-side counters (getdata_resp, getdata_resp_fast,
//     watch_event, response_bytes) are PRESENT and 0 on both sides for both
//     prefixes (eager roster creation; SPEC §4.3 exists-at-zero).
//
//   - Arm 7 (deliberate-break): recorded procedure (PROGRESS.md + README). Flips
//     an expected counter value to a wrong number / removes the name.go
//     .zookeeper. arm and confirms the test FAILS, proving AssertStats is not
//     vacuous.
//
// # StatsAsserter (asserter-dispatch memory: cross-side MUST use StatsAsserter)
//
// AssertStats scrapes /stats/prometheus from BOTH admin endpoints and asserts
// per-opcode counters. Per the asserter-dispatch project memory: SubjectAsserter
// only runs on the reference-less path (RequiresReference == false) and would
// produce dead vacuous assertions on a cross-side fixture. StatsAsserter is the
// correct interface, and the runner invokes AssertStats ONCE with both admin
// addresses (runner_test.go:1063-1064) — the driver scrapes both sides in-band.
//
// # Bootstrap discipline
//
// Both listeners' filter chains are [zookeeper_proxy, tcp_proxy]. The tcp_proxy
// terminal needs an upstream cluster — c_sink (the runner's TCPSink backend).
// A zero-cluster boot is rejected by both sides
// (reference_network_filter_typeurl_extensions memory). The zookeeper_proxy
// @type URL carries the extensions. segment:
// "type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy"
//
// # Cross-references
//
//   - phase 28.1 SPEC §8.1 (cross-side zookeeper-requests fixture scope)
//   - 28.1 PLAN Task 15 (this file) + Task 16 (AssertStats + 7 arms)
//   - fixture-0043-network-rbac (cross-side network filter + StatsAsserter template)
//   - fixture-0040-network-echo / 0001-tcp-proxy-rr (network bootstrap shape)
//   - ADR-0222 (zookeeper_proxy filter architecture)
//   - project memory reference_differential_asserter_dispatch (StatsAsserter
//     is load-bearing for cross-side; SubjectAsserter would be vacuous here)
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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0046-zookeeper-requests"

	refAdminPort = 9901

	// In-container reference Envoy listener ports. Convention "150NN" for
	// fixture "00NN" — 0046 takes 15047 for l_plain (15046 is already taken by
	// 0044-network-rbac-boot-reject) and 15048 for l_flags.
	refLPlainPort = 15047
	refLFlagsPort = 15048

	// stat_prefix roots for each listener's zookeeper_proxy config.
	statPrefixPlain = "zk_plain"
	statPrefixFlags = "zk_flags"
)

// Wire opcode values local to the driver. Driver packages cannot import
// internal/ (which would create an import cycle through the test package), so
// the 8 opcodes used by the 7 arms are redeclared here with explicit values
// matching internal/filter/network/zookeeperproxy/config.go.
const (
	drvOpCreate       int32 = 1
	drvOpGetData      int32 = 4
	drvOpGetChildren2 int32 = 12
	drvOpCreate2      int32 = 15
	drvOpSetWatches2  int32 = 105
	drvOpClose        int32 = -11
	drvOpPing         int32 = 11
)

func init() {
	fixture.RegisterFixture(fixtureName, &zkRequestsDriver{})
}

// zkRequestsDriver carries no mutable cross-arm state — the multi-listener
// matrix is fully deterministic.
type zkRequestsDriver struct{}

// --- fixture.Driver (required) ---

// BackendCount returns 1: a single TCPSink backend serves both listeners
// (c_sink cluster). The sink is silent, so backend accept counts are a
// secondary signal; the primary assertions are the Prometheus stat counters.
func (*zkRequestsDriver) BackendCount() int { return 1 }

// SubjectListenerName returns the primary listener name (l_plain). Required by
// the Driver interface; MultiListenerDriver takes precedence at runtime.
func (*zkRequestsDriver) SubjectListenerName() string { return "l_plain" }

// ReferenceListenerPort returns the primary reference listener port (l_plain).
// Required by the Driver interface even though MultiListenerDriver dispatches at
// runtime.
func (*zkRequestsDriver) ReferenceListenerPort() int { return refLPlainPort }

// ReferenceBootstrap renders the two-listener reference bootstrap. c_sink
// points at host.docker.internal:<backend> (ADR-0010 STRICT_DNS).
func (*zkRequestsDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("0.0.0.0, port_value: %d", refAdminPort),
		listenAddr:  "0.0.0.0",
		plainPort:   refLPlainPort,
		flagsPort:   refLFlagsPort,
		clusterType: "STRICT_DNS",
		dnsLine:     "      dns_lookup_family: V4_ONLY\n",
		backendHost: "host.docker.internal",
		backendPort: backendPorts[0],
		nodeLine:    "",
	})
}

// SubjectConfig renders the two-listener subject bootstrap. The two subject
// listeners get consecutive ports starting from subjListenerPort
// (plain=subjListenerPort, flags=+1) per the fixture-0043 multi-listener
// port-offset precedent.
func (*zkRequestsDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("127.0.0.1, port_value: %d", subjAdminPort),
		listenAddr:  "127.0.0.1",
		plainPort:   subjListenerPort,
		flagsPort:   subjListenerPort + 1,
		clusterType: "STATIC",
		dnsLine:     "",
		backendHost: "127.0.0.1",
		backendPort: backendPorts[0],
		nodeLine:    "node: { id: envoy-go-subject-0046, cluster: envoy-go-differential }\n",
	})
}

// --- fixture.MultiListenerDriver ---

func (*zkRequestsDriver) SubjectListenerNames() []string {
	return []string{"l_plain", "l_flags"}
}

func (*zkRequestsDriver) ReferenceListenerPorts() []int {
	return []int{refLPlainPort, refLFlagsPort}
}

func (d *zkRequestsDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.DriveReferenceMulti(ctx, map[string]string{"l_plain": addr})
}

func (d *zkRequestsDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.DriveSubjectMulti(ctx, map[string]string{"l_plain": addr})
}

func (d *zkRequestsDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "ref")
}

func (d *zkRequestsDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (*zkRequestsDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- fixture.BackendKindAware ---

// BackendKind returns TCPSink: a silent sink backend (accept + drain, never
// write). Required so the runner allocates a TCPSink backend instead of the
// default TCPEcho backend (which would produce response bytes that diverge
// cross-side due to the 28.1 OnWrite no-op stub).
func (*zkRequestsDriver) BackendKind() fixture.BackendKind { return fixture.TCPSink }

// --- scenario driving ---

const (
	// zkPath is the deterministic node path used by every data request. Length 8
	// keeps every frame comfortably above its per-opcode minimum (see the
	// payload builders' SAFE annotations).
	zkPath = "/zk-test"

	// interWriteDelay paces successive writes on a multi-frame connection so both
	// decoders observe the same read boundaries. Both decoders coalesce, so this
	// is for cross-side determinism, not correctness.
	interWriteDelay = 50 * time.Millisecond

	// garbagePause is the dwell between arm 4's oversized frame and the recovery
	// getdata — long enough that both proxies have processed (and counted the
	// decoder_error on) the oversized frame before the recovery frame arrives.
	garbagePause = 250 * time.Millisecond

	// settleDelay lets the async stat pipeline on both sides catch up before
	// AssertStats scrapes (the 0043 sleep-to-settle precedent).
	settleDelay = 750 * time.Millisecond
)

// driveProxy runs the 7-arm workload (SPEC §8.1.3) against both sides
// identically and returns a side-independent verdict byte stream. The "side"
// label is accepted for diagnostic logging only and is NEVER written to the
// returned bytes, so equivalent behavior yields byte-identical drive output for
// the runner's CompareBytes gate. Each arm emits one verdict line of the form
//
//	arm <name> sent=<n> verdict=<v>
//
// (the 0043 verdict-line precedent). The arms run in declared order over the
// shared listeners, so AssertStats asserts CUMULATIVE counter values.
func (d *zkRequestsDriver) driveProxy(ctx context.Context, addrs map[string]string, side string) ([]byte, error) {
	plain, ok := addrs["l_plain"]
	if !ok {
		return nil, fmt.Errorf("%s: missing addr for listener l_plain", fixtureName)
	}
	flags, ok := addrs["l_flags"]
	if !ok {
		return nil, fmt.Errorf("%s: missing addr for listener l_flags", fixtureName)
	}

	var b bytes.Buffer

	// Arm 1 (connect): one connectFrame(false) on l_plain.
	n, err := driveFrames(ctx, plain, 0, [][]byte{
		connectFrame(false),
	})
	emitArm(&b, side, "connect", n, err)

	// Arm 2 (multi-opcode): connect + ping + getdata(xid 1) + create(xid 2) +
	// close(xid 3), separate paced writes on one l_plain connection.
	n, err = driveFrames(ctx, plain, interWriteDelay, [][]byte{
		connectFrame(false),
		pingFrame(),
		dataFrame(1, drvOpGetData, getdataPayload(zkPath)),
		dataFrame(2, drvOpCreate, createPayload(zkPath)),
		dataFrame(3, drvOpClose, closePayload()),
	})
	emitArm(&b, side, "multi-opcode", n, err)

	// Arm 3 (digit-suffixed): create2(xid 4) + getchildren2(xid 5) +
	// setwatches2 as a DATA request (xid 6, wire op 105) on one l_plain conn.
	n, err = driveFrames(ctx, plain, interWriteDelay, [][]byte{
		dataFrame(4, drvOpCreate2, createPayload(zkPath)),
		dataFrame(5, drvOpGetChildren2, getchildren2Payload(zkPath)),
		dataFrame(6, drvOpSetWatches2, setwatches2Payload()),
	})
	emitArm(&b, side, "digit-suffixed", n, err)

	// Arm 4 (garbage + survival): oversized length prefix → pause → valid
	// getdata ON THE SAME CONNECTION; the connection must survive the garbage.
	n, err = driveGarbageSurvival(ctx, plain)
	emitArm(&b, side, "garbage-survival", n, err)

	// Arm 5 (flag-gated bytes): one getdata on l_flags.
	n, err = driveFrames(ctx, flags, 0, [][]byte{
		dataFrame(7, drvOpGetData, getdataPayload(zkPath)),
	})
	emitArm(&b, side, "flag-gated", n, err)

	// Arm 6 (exists-at-zero): no traffic — assertion-only (AssertStats). The
	// verdict line is constant so it stays byte-identical cross-side.
	fmt.Fprintf(&b, "arm exists-at-zero sent=0 verdict=assertion_only\n")

	// Arm 7 (deliberate-break): recorded procedure (no live traffic).
	fmt.Fprintf(&b, "arm deliberate-break sent=0 verdict=recorded\n")

	// Let the async stat pipeline settle before the runner scrapes in AssertStats.
	select {
	case <-time.After(settleDelay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return b.Bytes(), nil
}

// emitArm writes a side-independent verdict line for one arm. The side label is
// logged to stderr (diagnostic) but never to the returned byte stream.
func emitArm(b *bytes.Buffer, side, name string, sent int, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "[fixture 0046 %s] arm %s: %v\n", side, name, err)
		fmt.Fprintf(b, "arm %s sent=%d verdict=ERR\n", name, sent)
		return
	}
	fmt.Fprintf(b, "arm %s sent=%d verdict=ok\n", name, sent)
}

// driveFrames opens a fresh TCP connection to addr, writes each frame as a
// separate Write (with interDelay between writes when > 0), then closes the
// connection. Returns the number of frames written and any error.
func driveFrames(ctx context.Context, addr string, interDelay time.Duration, frames [][]byte) (int, error) {
	conn, err := dialZK(ctx, addr)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()

	sent := 0
	for i, f := range frames {
		if i > 0 && interDelay > 0 {
			if err := sleepCtx(ctx, interDelay); err != nil {
				return sent, err
			}
		}
		if _, err := conn.Write(f); err != nil {
			return sent, fmt.Errorf("write frame %d: %w", i, err)
		}
		sent++
	}
	return sent, nil
}

// driveGarbageSurvival drives arm 4: an oversized frame (2 MiB length prefix >
// the 1 MiB default max_packet_bytes) followed — ON THE SAME CONNECTION, after
// garbagePause — by a valid getdata. Both proxies must count one decoder_error
// for the oversized frame, KEEP the connection open, and still decode the
// recovery getdata (getdata_rq increments). A RST or refused write on the second
// frame would indicate the proxy closed the connection on garbage — a failure.
func driveGarbageSurvival(ctx context.Context, addr string) (int, error) {
	conn, err := dialZK(ctx, addr)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()

	// Oversized frame: length prefix 2 MiB, then 64 bytes of payload. The proxy
	// reads the length prefix, sees it exceeds max_packet_bytes, increments
	// decoder_error, and abandons its read buffer (AMEND-A8 no-resync).
	oversized := append(be32(2*1024*1024), make([]byte, 64)...)
	if _, err := conn.Write(oversized); err != nil {
		return 0, fmt.Errorf("write oversized frame: %w", err)
	}
	sent := 1

	if err := sleepCtx(ctx, garbagePause); err != nil {
		return sent, err
	}

	// Recovery getdata on the SAME connection. It is appended past the decoder's
	// high-water mark and must still decode → getdata_rq increments, proving the
	// connection survived the garbage.
	if _, err := conn.Write(dataFrame(8, drvOpGetData, getdataPayload(zkPath))); err != nil {
		return sent, fmt.Errorf("write recovery getdata (connection closed by proxy?): %w", err)
	}
	sent++
	return sent, nil
}

// dialZK opens a TCP connection to addr honoring ctx's deadline.
func dialZK(ctx context.Context, addr string) (net.Conn, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return conn, nil
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

// --- frame-crafting helpers (D-S28.1-4: small builder funcs) ---

// be32 encodes v as a 4-byte big-endian slice.
func be32(v int32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	return b
}

// be64 encodes v as an 8-byte big-endian slice.
func be64(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

// zkFrame prepends the 4-byte BE length prefix (which EXCLUDES itself) to the
// concatenated parts. The wire format is: [length(4)] [payload(length)].
func zkFrame(parts ...[]byte) []byte {
	payload := bytes.Join(parts, nil)
	return append(be32(int32(len(payload))), payload...)
}

// connectFrame builds a ZooKeeper connect request frame (length-prefix
// stripped). Layout (SPEC §4.1 / upstream decoder.cc:connect; empirically
// verified at Task 14):
//
//	protocol_version(4=0) | last_zxid(8=0) | timeout(4=30000) |
//	session_id(8=0) | password_len(4=16) | password(16 NUL bytes) |
//	[readonly(1=1) if readonly]
//
// The xid is 0 (connectXid); the frame is wrapped in the 4-byte length prefix
// by zkFrame.
func connectFrame(readonly bool) []byte {
	parts := [][]byte{
		be32(0),          // protocol_version
		be64(0),          // last_zxid
		be32(30000),      // timeout ms
		be64(0),          // session_id
		be32(16),         // password length
		make([]byte, 16), // 16 NUL bytes (password)
	}
	if readonly {
		parts = append(parts, []byte{1})
	}
	return zkFrame(parts...)
}

// dataFrame builds a ZooKeeper data-request frame (with length prefix).
// xid must be > 0 (data-request path). opcode selects the counter family.
// payload carries the opcode-specific fields.
func dataFrame(xid, opcode int32, payload []byte) []byte {
	return zkFrame(be32(xid), be32(opcode), payload)
}

// pingFrame builds a ZooKeeper ping request frame (xid=-2, opcode=11).
func pingFrame() []byte {
	return zkFrame(be32(-2), be32(drvOpPing))
}

// getdataPayload builds the payload for a GetData request (opcode 4).
// Layout: pathLen(4) | path | watch(1).
// Min frame = xid(4) + opcode(4) + pathLen(4) + watch(1) = 13 bytes.
// With path="/zk-test" (8 bytes): frame payload after xid+opcode = 4+8+1 = 13
// bytes → frame = xid(4) + opcode(4) + 13 = 21 bytes > 13 min. SAFE.
func getdataPayload(path string) []byte {
	p := append(be32(int32(len(path))), []byte(path)...)
	p = append(p, 0) // watch = false
	return p
}

// createPayload builds the payload for a Create (opcode 1) or Create2 (opcode
// 15) request. Layout: pathLen(4) | path | dataLen(4) | data | aclCount(4) |
// flags(4). Min frame = xid(4) + opcode(4) + 4*INT = 24 bytes (upstream
// decoder.cc:457). With path="/zk-test" (8) + dataLen=0 + aclCount=0 +
// flags=0: payload after xid+opcode = 4+8+4+4+4 = 24 bytes → frame = 4+4+24 =
// 32 bytes > 24 min. SAFE.
func createPayload(path string) []byte {
	p := append(be32(int32(len(path))), []byte(path)...)
	p = append(p, be32(0)...) // dataLen = 0
	p = append(p, be32(0)...) // aclCount = 0
	p = append(p, be32(0)...) // flags = 0
	return p
}

// getchildren2Payload builds the payload for a GetChildren2 (opcode 12) request.
// Layout: pathLen(4) | path | watch(1).
// Min frame = xid(4) + opcode(4) + pathLen(4) + watch(1) = 13 bytes. SAFE.
func getchildren2Payload(path string) []byte {
	p := append(be32(int32(len(path))), []byte(path)...)
	p = append(p, 0) // watch = false
	return p
}

// setwatches2Payload builds the payload for a SetWatches2 (opcode 105) sent as
// a DATA request (positive xid). The upstream decoder.cc SetWatches2 min is:
// xid(4) + opcode(4) + LONG(8) + 5*INT(20) = 36 bytes. We supply a zxid(8)
// and five INT fields (each 0) → payload after xid+opcode = 8+5*4 = 28 bytes
// → frame = 4+4+28 = 36 bytes. SAFE (meets the 36-byte minimum exactly).
func setwatches2Payload() []byte {
	var p []byte
	p = append(p, be64(0)...) // relative_zxid (LONG)
	p = append(p, be32(0)...) // dataWatchesSize
	p = append(p, be32(0)...) // existWatchesSize
	p = append(p, be32(0)...) // childWatchesSize
	p = append(p, be32(0)...) // persistentWatchesSize
	p = append(p, be32(0)...) // persistentRecursiveWatchesSize
	return p
}

// closePayload is empty — close frames carry no payload beyond xid+opcode.
// Min frame = xid(4) + opcode(4) = 8 bytes (universal min). SAFE.
func closePayload() []byte { return nil }

// --- fixture.StatsAsserter (asserter-dispatch memory: cross-side MUST use
// StatsAsserter; SubjectAsserter would be a dead vacuous assertion) ---

// AssertStats scrapes /stats/prometheus from BOTH admin endpoints and asserts
// the per-opcode zookeeper counters after the 7-arm workload (SPEC §8.1). The
// runner invokes this ONCE with both admin addresses (runner_test.go:1063-1064),
// so the scrape-and-diff for both sides happens in-band here.
//
// Counters are looked up via the FLATTENED Prometheus form
// envoy_<prefix>_zookeeper_<suffix> (no labels — AMEND-A4; the zookeeper_proxy
// filter has no tag extraction). A name-shape mismatch on EITHER side makes the
// lookup miss → ABSENT → the assertion fails, so R7 Prometheus-parity is
// intrinsic to this fixture (the name.go .zookeeper. arm is load-bearing).
//
// The expected values are CUMULATIVE over the in-order arms sharing a listener:
// arm 2's connect is the second connect on l_plain (connect_rq==2) and arm 4's
// recovery getdata is the second getdata on l_plain (getdata_rq==2: arm 2's
// xid 1 + arm 4's recovery xid 8; arm 5's getdata lands on l_flags, not
// l_plain). See the expectations table for the exact per-prefix accounting.
func (d *zkRequestsDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeZKStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref zookeeper stats: %v", err)
	}
	subjStats, err := scrapeZKStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj zookeeper stats: %v", err)
	}

	if os.Getenv("FIXTURE_0046_DUMP_STATS") != "" {
		dump := func(label string, m map[string]int64) {
			fmt.Fprintf(os.Stderr, "=== %s zookeeper stats ===\n", label)
			for k, v := range m {
				fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
			}
		}
		dump("ref", refStats)
		dump("subj", subjStats)
	}

	// Fixed-value expectations. Metric is the dotted internal form
	// <prefix>.zookeeper.<suffix>; lookupZKCounter flattens it to the Prometheus
	// name and reports presence so an ABSENT counter (name-shape failure) is
	// distinguished from a present-but-wrong value.
	type expect struct {
		metric string
		want   int64
	}
	expectations := []expect{
		// l_plain request counters (cumulative over arms 1-4):
		{"zk_plain.zookeeper.connect_rq", 2},      // arm 1 + arm 2
		{"zk_plain.zookeeper.ping_rq", 1},         // arm 2
		{"zk_plain.zookeeper.getdata_rq", 2},      // arm 2 (xid 1) + arm 4 recovery (xid 8)
		{"zk_plain.zookeeper.create_rq", 1},       // arm 2
		{"zk_plain.zookeeper.close_rq", 1},        // arm 2
		{"zk_plain.zookeeper.create2_rq", 1},      // arm 3
		{"zk_plain.zookeeper.getchildren2_rq", 1}, // arm 3
		{"zk_plain.zookeeper.setwatches2_rq", 1},  // arm 3
		{"zk_plain.zookeeper.decoder_error", 1},   // arm 4 oversized frame
		// exists-at-zero (arm 6; eager creation parity D-P5/R2):
		{"zk_plain.zookeeper.getdata_resp", 0},
		{"zk_plain.zookeeper.getdata_resp_fast", 0},
		{"zk_plain.zookeeper.watch_event", 0},
		{"zk_plain.zookeeper.response_bytes", 0},
		// flag-gating (arm 5): the per-opcode bytes counter is created on both
		// listeners but only incremented where the flag is ON (l_flags).
		{"zk_plain.zookeeper.getdata_rq_bytes", 0}, // flag OFF on l_plain
		{"zk_flags.zookeeper.getdata_rq", 1},       // arm 5
	}

	for _, sd := range []struct {
		label string
		stats map[string]int64
	}{{"ref", refStats}, {"subj", subjStats}} {
		for _, exp := range expectations {
			got, present := lookupZKCounter(sd.stats, exp.metric)
			if !present {
				t.Errorf("%s: counter %s ABSENT (creation parity / name-shape failure)", sd.label, exp.metric)
				continue
			}
			if got != exp.want {
				t.Errorf("%s %s = %d, want %d", sd.label, exp.metric, got, exp.want)
			}
		}
	}

	// Cross-side EQUALITY assertions (no fixed value — must agree and be > 0):
	//   - zk_plain.zookeeper.request_bytes: the wire-footprint sum of every frame
	//     decoded on l_plain (arm 2 is the load-bearing equality proof; SPEC §4.5).
	//   - zk_flags.zookeeper.getdata_rq_bytes: the flag-ON per-opcode bytes for
	//     arm 5's single getdata — present + equal + > 0 on both sides.
	for _, metric := range []string{
		"zk_plain.zookeeper.request_bytes",
		"zk_flags.zookeeper.getdata_rq_bytes",
	} {
		refV, refOK := lookupZKCounter(refStats, metric)
		subjV, subjOK := lookupZKCounter(subjStats, metric)
		if !refOK || !subjOK || refV != subjV || refV == 0 {
			t.Errorf("cross-side %s: ref=(%d,%v) subj=(%d,%v), want present, equal, and > 0",
				metric, refV, refOK, subjV, subjOK)
		}
	}
}

// --- stats scraping ---

// scrapeZKStats issues GET /stats/prometheus against adminAddr and returns a map
// of zookeeper-related counter values keyed by the FLATTENED Prometheus metric
// name (the `envoy_` prefix retained). Both reference Envoy v1.37.2 and envoy-go
// surface the zookeeper_proxy counters as flat names with an EMPTY label set
// (AMEND-A4: upstream applies no tag extraction to this filter), so we retain
// only lines whose name contains the `_zookeeper_` infix and key by the bare
// metric name.
func scrapeZKStats(adminAddr string) (map[string]int64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return parseZKPromBody(resp.Body)
}

// parseZKPromBody parses a Prometheus text-format body and returns a map keyed by
// the FULL metric name (with the `envoy_` prefix RETAINED) of int64 values for
// all lines whose metric name contains the `_zookeeper_` infix. The zookeeper
// counters carry no labels, so a `{...}` label set (if present at all) is empty
// and ignored; the value is the last whitespace-separated token.
func parseZKPromBody(r io.Reader) (map[string]int64, error) {
	out := map[string]int64{}
	const wantInfix = "_zookeeper_"
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var name, valueStr string
		if idx := strings.IndexByte(line, '{'); idx >= 0 {
			name = line[:idx]
			closeIdx := strings.LastIndexByte(line, '}')
			if closeIdx < 0 || closeIdx+1 >= len(line) {
				continue
			}
			valueStr = strings.TrimSpace(line[closeIdx+1:])
		} else {
			sp := strings.LastIndexByte(line, ' ')
			if sp < 0 {
				continue
			}
			name = line[:sp]
			valueStr = strings.TrimSpace(line[sp+1:])
		}
		if !strings.Contains(name, wantInfix) {
			continue
		}
		if sp := strings.IndexByte(valueStr, ' '); sp >= 0 {
			valueStr = valueStr[:sp]
		}
		f, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}
		out[name] = int64(f)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return out, nil
}

// lookupZKCounter resolves a dotted internal stat name (e.g.
// "zk_plain.zookeeper.connect_rq") against the scraped map by flattening it to
// the Prometheus form: envoy_ prefix + dots → underscores
// (envoy_zk_plain_zookeeper_connect_rq). Returns the value and whether the
// counter was PRESENT — present-vs-absent is the name-shape / creation-parity
// signal (an absent counter means the .zookeeper. name.go arm or eager roster
// creation failed, which AssertStats reports distinctly from a wrong value).
func lookupZKCounter(stats map[string]int64, dotted string) (int64, bool) {
	prom := "envoy_" + strings.ReplaceAll(dotted, ".", "_")
	v, ok := stats[prom]
	return v, ok
}

// --- bootstrap rendering ---

type bootstrapParams struct {
	adminAddr   string // "<ip>, port_value: <n>" for the admin socket_address
	listenAddr  string // listener bind address (0.0.0.0 for ref; 127.0.0.1 for subj)
	plainPort   int    // l_plain listener port
	flagsPort   int    // l_flags listener port
	clusterType string // STRICT_DNS (ref) | STATIC (subj)
	dnsLine     string // "      dns_lookup_family: V4_ONLY\n" for STRICT_DNS, else ""
	backendHost string
	backendPort int
	nodeLine    string // "node: {...}\n" for subj, "" for ref
}

// zkProxyType is the zookeeper_proxy typed_config @type URL. The network-filter
// type URLs carry the extensions. segment (memory
// reference_network_filter_typeurl_extensions); the proto FQN is
// envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy.
const zkProxyType = "type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy"
const tcpProxyType = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// renderBootstrap assembles the full two-listener bootstrap. Each listener's
// filter chain is [zookeeper_proxy, tcp_proxy] — the shallow-read + terminal
// chain. c_sink is the tcp_proxy upstream (the runner's TCPSink backend) AND
// the boot-satisfying cluster (a zero-cluster boot is rejected by both sides).
func renderBootstrap(p bootstrapParams) string {
	plainListener := fmt.Sprintf(`    - name: l_plain
      address:
        socket_address: { address: %s, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.zookeeper_proxy
              typed_config:
                "@type": %s
                stat_prefix: %s
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": %s
                stat_prefix: tcp_plain
                cluster: c_sink
`, p.listenAddr, p.plainPort, zkProxyType, statPrefixPlain, tcpProxyType)

	flagsListener := fmt.Sprintf(`    - name: l_flags
      address:
        socket_address: { address: %s, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.zookeeper_proxy
              typed_config:
                "@type": %s
                stat_prefix: %s
                enable_per_opcode_request_bytes_metrics: true
                enable_per_opcode_decoder_error_metrics: true
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": %s
                stat_prefix: tcp_flags
                cluster: c_sink
`, p.listenAddr, p.flagsPort, zkProxyType, statPrefixFlags, tcpProxyType)

	return fmt.Sprintf(`%sadmin:
  address:
    socket_address: { address: %s }
static_resources:
  listeners:
%s%s  clusters:
    - name: c_sink
      type: %s
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s      load_assignment:
        cluster_name: c_sink
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: %s, port_value: %d }
`,
		p.nodeLine,
		p.adminAddr,
		plainListener,
		flagsListener,
		p.clusterType,
		p.dnsLine,
		p.backendHost, p.backendPort,
	)
}

// Compile-time interface assertions. StatsAsserter is now satisfied by the real
// AssertStats (Task 16) — the cross-side LIVE counter-parity proof.
var (
	_ fixture.Driver              = (*zkRequestsDriver)(nil)
	_ fixture.MultiListenerDriver = (*zkRequestsDriver)(nil)
	_ fixture.BackendKindAware    = (*zkRequestsDriver)(nil)
	_ fixture.StatsAsserter       = (*zkRequestsDriver)(nil)
)
