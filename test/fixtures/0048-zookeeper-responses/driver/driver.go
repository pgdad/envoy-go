// Package driver registers the 0048-zookeeper-responses cross-side differential
// fixture with the runner per phase 28.2 SPEC §5.2 (R4/R5).
//
// ============================================================================
// The FIRST cross-side fixture for the zookeeper_proxy RESPONSE decoder (28.2)
// ============================================================================
//
// 0046-zookeeper-requests proved the request side against a SILENT sink. This
// fixture proves the 28.2 response decoder (ADR-0223): each listener's filter
// chain is [zookeeper_proxy, tcp_proxy] targeting a driver-controlled
// ZooKeeper-aware canned-response backend (BackendKind=TCPZKResponder; the
// runner's acceptZKResponder), and the driver asserts per-opcode RESPONSE
// counter + latency-bucket + response-bytes parity across both reference Envoy
// v1.37.2 (dockerized) and envoy-go via the StatsAsserter interface.
//
// # Cross-side dispatch (ONE runner branch)
//
// Per reference_differential_fixture_dispatch_constraint (one fixture dir = ONE
// runner branch), this fixture is exclusively cross-side: both reference Envoy
// and envoy-go run the SAME 8-arm workload and assert the SAME stats. There are
// NO boot-reject arms here (those live in 0047). The asserter is StatsAsserter
// (NOT SubjectAsserter, which only runs on the reference-less path and would be
// a dead vacuous assertion on a cross-side fixture —
// reference_differential_asserter_dispatch).
//
// # Backend choice: TCPZKResponder (not TCPSink)
//
// 0046's TCPSink is request-side-only: a silent sink writes nothing, so no
// response bytes ever traverse the filter chain and the response decoder is
// never exercised. This fixture needs a backend that writes CORRELATED
// ZooKeeper response frames back through the chain so reference Envoy's onWrite
// response decoder (and envoy-go's 28.2 OnWrite glue) tick the *_resp / latency
// / response_bytes counters. TCPZKResponder (runner acceptZKResponder) is that
// backend: for every request frame it reads it waits a FIXED 10ms delay then
// writes a correlated canned response.
//
// # Deterministic-threshold construction (D-P9)
//
// The responder's fixed 10ms pre-response delay means every measured latency on
// BOTH sides is ≥ 10ms. A listener configured with default_latency_threshold
// 3600s therefore buckets EVERY response FAST (10ms ≤ 3600s); a listener with
// default_latency_threshold 1ms buckets EVERY response SLOW (10ms > 1ms). No
// cross-side timing nondeterminism: the slow-arm threshold (1ms) and the
// fast-arm/override threshold (3600s) straddle the fixed delay with a 1000x and
// 360000x margin respectively.
//
// # Trigger-opcode encoding (D-S28.2-2)
//
// The responder peeks the request opcode (frame bytes 4-8) for data requests:
//   - getacl (wire op 6) → response with xid+1000 (uncorrelated on both sides
//     → decoder_error; getacl_resp stays 0). The same connection then survives
//     and a follow-up sync decodes normally (abandon-no-resync recovery).
//   - exists (wire op 3) → a NORMAL correlated response, THEN an unsolicited
//     watch-event push (xid −1) in the FULL ReplyHeader format
//     (D-S28.2-1: xid+zxid+error+event_type+client_state+path, 37 bytes ≥ the
//     28-byte upstream parseWatchEvent minimum). Both reference Envoy and
//     envoy-go accept this format and tick watch_event.
//
// # Round-trip driving discipline
//
// Each arm WRITES a request frame then READS the expected number of response
// frames before proceeding (driveRoundTrips). This gives deterministic
// cross-side decode ordering and natural backpressure: the request decoder, the
// responder's fixed delay, and the response decoder all complete for frame i
// before frame i+1 is written, so both sides observe identical interleavings.
//
// # Four listeners / four stat_prefixes
//
//   - l_resp   (zk_resp):   defaults — NO latency metrics, NO resp-bytes flag.
//     Carries arms 1-3 (round-trips, watch event, wrong-xid + survival).
//   - l_fast   (zk_fast):   enable_latency_threshold_metrics + default 3600s →
//     every response FAST. Carries arm 4.
//   - l_slow   (zk_slow):   enable_latency_threshold_metrics + default 1ms +
//     GetData override 3600s → every response SLOW except getdata (override
//     beats the default → FAST). Carries arms 5-6.
//   - l_rflags (zk_rflags): enable_per_opcode_response_bytes_metrics → the
//     flag-gated <op>_resp_bytes counter. Carries arm 7.
//
// One shared TCPZKResponder backend (cluster c_zk) serves all four listeners;
// tcp_proxy needs an upstream cluster and a zero-cluster boot is rejected by
// both sides (reference_network_filter_typeurl_extensions), so c_zk doubles as
// the boot-satisfying cluster.
//
// # R4 deliberate-break protocol (arm 8; -count=1)
//
// Arm 8 is a recorded procedure (no live traffic). The AssertStats expectations
// were proven non-vacuous against the green baseline by (a) flipping an expected
// value (FAIL on both sides) and (b) commenting out a production counter
// increment (subject-side divergence). Both runs use `go test -count=1` (result
// caching otherwise serves a stale PASS after a deliberate break —
// differential_break_protocol_count1). See PROGRESS.md Task 9 + the README.
//
// # Cross-references
//
//   - phase 28.2 SPEC §5.2 (cross-side zookeeper-responses fixture scope)
//   - 28.2 PLAN Task 9 (this file + AssertStats + 8 arms)
//   - fixture-0046-zookeeper-requests (the request-side template)
//   - ADR-0223 (zookeeper_proxy response decoder)
//   - project memory reference_differential_asserter_dispatch / _fixture_dispatch_constraint
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
	fixtureName = "0048-zookeeper-responses"

	refAdminPort = 9901

	// In-container reference Envoy listener ports (Task 1 pin).
	refLRespPort   = 15050
	refLFastPort   = 15051
	refLSlowPort   = 15052
	refLRflagsPort = 15053

	// stat_prefix roots for each listener's zookeeper_proxy config.
	statPrefixResp   = "zk_resp"
	statPrefixFast   = "zk_fast"
	statPrefixSlow   = "zk_slow"
	statPrefixRflags = "zk_rflags"
)

// Wire opcode values local to the driver. Driver packages cannot import
// internal/ (which would create an import cycle through the test package), so
// the opcodes used by the 8 arms are redeclared here with explicit values
// matching internal/filter/network/zookeeperproxy/config.go.
const (
	drvOpCreate  int32 = 1
	drvOpDelete  int32 = 2
	drvOpExists  int32 = 3 // the watch-event-push trigger (D-S28.2-2)
	drvOpGetData int32 = 4
	drvOpSetData int32 = 5
	drvOpGetACL  int32 = 6 // the wrong-xid trigger (D-S28.2-2)
	drvOpSync    int32 = 9
	drvOpPing    int32 = 11
	drvOpClose   int32 = -11
)

func init() {
	fixture.RegisterFixture(fixtureName, &zkResponsesDriver{})
}

// zkResponsesDriver carries no mutable cross-arm state — the multi-listener
// matrix is fully deterministic.
type zkResponsesDriver struct{}

// --- fixture.Driver (required) ---

// BackendCount returns 1: a single TCPZKResponder backend serves all four
// listeners (c_zk cluster).
func (*zkResponsesDriver) BackendCount() int { return 1 }

// SubjectListenerName returns the primary listener name (l_resp). Required by
// the Driver interface; MultiListenerDriver takes precedence at runtime.
func (*zkResponsesDriver) SubjectListenerName() string { return "l_resp" }

// ReferenceListenerPort returns the primary reference listener port (l_resp).
func (*zkResponsesDriver) ReferenceListenerPort() int { return refLRespPort }

// ReferenceBootstrap renders the four-listener reference bootstrap. c_zk points
// at host.docker.internal:<backend> (ADR-0010 STRICT_DNS).
func (*zkResponsesDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("0.0.0.0, port_value: %d", refAdminPort),
		listenAddr:  "0.0.0.0",
		respPort:    refLRespPort,
		fastPort:    refLFastPort,
		slowPort:    refLSlowPort,
		rflagsPort:  refLRflagsPort,
		clusterType: "STRICT_DNS",
		dnsLine:     "      dns_lookup_family: V4_ONLY\n",
		backendHost: "host.docker.internal",
		backendPort: backendPorts[0],
		nodeLine:    "",
	})
}

// SubjectConfig renders the four-listener subject bootstrap. The four subject
// listeners get consecutive ports starting from subjListenerPort
// (resp=+0, fast=+1, slow=+2, rflags=+3) per the multi-listener port-offset
// precedent (0046/0043).
func (*zkResponsesDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("127.0.0.1, port_value: %d", subjAdminPort),
		listenAddr:  "127.0.0.1",
		respPort:    subjListenerPort,
		fastPort:    subjListenerPort + 1,
		slowPort:    subjListenerPort + 2,
		rflagsPort:  subjListenerPort + 3,
		clusterType: "STATIC",
		dnsLine:     "",
		backendHost: "127.0.0.1",
		backendPort: backendPorts[0],
		nodeLine:    "node: { id: envoy-go-subject-0048, cluster: envoy-go-differential }\n",
	})
}

// --- fixture.MultiListenerDriver ---

func (*zkResponsesDriver) SubjectListenerNames() []string {
	return []string{"l_resp", "l_fast", "l_slow", "l_rflags"}
}

func (*zkResponsesDriver) ReferenceListenerPorts() []int {
	return []int{refLRespPort, refLFastPort, refLSlowPort, refLRflagsPort}
}

func (d *zkResponsesDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.DriveReferenceMulti(ctx, map[string]string{"l_resp": addr})
}

func (d *zkResponsesDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.DriveSubjectMulti(ctx, map[string]string{"l_resp": addr})
}

func (d *zkResponsesDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "ref")
}

func (d *zkResponsesDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (*zkResponsesDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// BackendKind returns TCPZKResponder: the runner's ZooKeeper-aware
// canned-response backend (acceptZKResponder; fixed 10ms delay + correlated
// responses + the getacl/exists triggers). Required so the response decoder is
// exercised cross-side (TCPSink writes nothing and would never tick *_resp).
func (*zkResponsesDriver) BackendKind() fixture.BackendKind { return fixture.TCPZKResponder }

// --- scenario driving ---

const (
	// zkPath is the deterministic node path used by every data request. Length 8
	// keeps every frame comfortably above its per-opcode minimum.
	zkPath = "/zk-test"

	// settleDelay lets the async stat pipeline on both sides catch up before
	// AssertStats scrapes (the 0046/0043 sleep-to-settle precedent).
	settleDelay = 750 * time.Millisecond

	// respReadTimeout bounds a single response-frame read on a round-trip
	// connection (the responder's 10ms delay + transit is well under this).
	respReadTimeout = 5 * time.Second
)

// driveProxy runs the 8-arm workload (SPEC §5.2) against both sides identically
// and returns a side-independent verdict byte stream. The "side" label is
// accepted for diagnostic logging only and is NEVER written to the returned
// bytes, so equivalent behavior yields byte-identical drive output for the
// runner's CompareBytes gate. Each arm emits one verdict line of the form
//
//	arm <name> sent=<n> verdict=<v>
//
// The arms run in declared order over the shared listeners, so AssertStats
// asserts CUMULATIVE counter values per listener.
func (d *zkResponsesDriver) driveProxy(ctx context.Context, addrs map[string]string, side string) ([]byte, error) {
	resp, ok := addrs["l_resp"]
	if !ok {
		return nil, fmt.Errorf("%s: missing addr for listener l_resp", fixtureName)
	}
	fast, ok := addrs["l_fast"]
	if !ok {
		return nil, fmt.Errorf("%s: missing addr for listener l_fast", fixtureName)
	}
	slow, ok := addrs["l_slow"]
	if !ok {
		return nil, fmt.Errorf("%s: missing addr for listener l_slow", fixtureName)
	}
	rflags, ok := addrs["l_rflags"]
	if !ok {
		return nil, fmt.Errorf("%s: missing addr for listener l_rflags", fixtureName)
	}

	var b bytes.Buffer

	// Arm 1 (round-trips, l_resp): connect + getdata(xid 1) + create(xid 2) +
	// ping + close(xid 3), each answered with exactly 1 response frame.
	n, err := driveRoundTrips(ctx, resp, [][]byte{
		connectFrame(false),
		dataFrame(1, drvOpGetData, getdataPayload(zkPath)),
		dataFrame(2, drvOpCreate, createPayload(zkPath)),
		pingFrame(),
		dataFrame(3, drvOpClose, closePayload()),
	}, []int{1, 1, 1, 1, 1})
	emitArm(&b, side, "round-trips", n, err)

	// Arm 2 (watch event, l_resp): exists(xid 4) → 2 frames (correlated response
	// + unsolicited watch-event push).
	n, err = driveRoundTrips(ctx, resp, [][]byte{
		dataFrame(4, drvOpExists, existsPayload(zkPath)),
	}, []int{2})
	emitArm(&b, side, "watch-event", n, err)

	// Arm 3 (wrong xid + survival, l_resp): getacl(xid 5) [wrong-xid trigger →
	// decoder_error both sides], then sync(xid 6) on the SAME connection →
	// sync_resp (abandon-no-resync recovery proof). Each gets 1 response frame.
	n, err = driveRoundTrips(ctx, resp, [][]byte{
		dataFrame(5, drvOpGetACL, getaclPayload(zkPath)),
		dataFrame(6, drvOpSync, syncPayload(zkPath)),
	}, []int{1, 1})
	emitArm(&b, side, "wrong-xid-survival", n, err)

	// Arm 4 (all-fast, l_fast — 3600s default): connect + getdata(1) + setdata(2)
	// → every response FAST.
	n, err = driveRoundTrips(ctx, fast, [][]byte{
		connectFrame(false),
		dataFrame(1, drvOpGetData, getdataPayload(zkPath)),
		dataFrame(2, drvOpSetData, setdataPayload(zkPath)),
	}, []int{1, 1, 1})
	emitArm(&b, side, "all-fast", n, err)

	// Arm 5 (all-slow, l_slow — 1ms default + ≥10ms responder delay): connect +
	// setdata(1) + delete(2) → every response SLOW.
	n, err = driveRoundTrips(ctx, slow, [][]byte{
		connectFrame(false),
		dataFrame(1, drvOpSetData, setdataPayload(zkPath)),
		dataFrame(2, drvOpDelete, deletePayload(zkPath)),
	}, []int{1, 1, 1})
	emitArm(&b, side, "all-slow", n, err)

	// Arm 6 (override, l_slow): getdata(3) → getdata_resp_FAST (GetData override
	// 3600s beats the 1ms default) while arm 5's ops were SLOW.
	n, err = driveRoundTrips(ctx, slow, [][]byte{
		dataFrame(3, drvOpGetData, getdataPayload(zkPath)),
	}, []int{1})
	emitArm(&b, side, "override", n, err)

	// Arm 7 (flag-gated resp-bytes, l_rflags): connect + getdata(1) →
	// getdata_resp_bytes > 0 (cross-side equality); on l_resp (flag off) it
	// stays 0.
	n, err = driveRoundTrips(ctx, rflags, [][]byte{
		connectFrame(false),
		dataFrame(1, drvOpGetData, getdataPayload(zkPath)),
	}, []int{1, 1})
	emitArm(&b, side, "flag-gated-resp-bytes", n, err)

	// Arm 8 (R4 deliberate-break): recorded procedure, no live traffic.
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
		fmt.Fprintf(os.Stderr, "[fixture 0048 %s] arm %s: %v\n", side, name, err)
		fmt.Fprintf(b, "arm %s sent=%d verdict=ERR\n", name, sent)
		return
	}
	fmt.Fprintf(b, "arm %s sent=%d verdict=ok\n", name, sent)
}

// driveRoundTrips opens a fresh connection to addr, then for each request frame
// frames[i]: writes it, reads back responses[i] complete response frames, then
// proceeds. Returns the number of request frames written and any error.
func driveRoundTrips(ctx context.Context, addr string, frames [][]byte, responses []int) (int, error) {
	conn, err := dialZK(ctx, addr)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	for i, frame := range frames {
		if _, err := conn.Write(frame); err != nil {
			return i, fmt.Errorf("write frame %d: %w", i, err)
		}
		for r := 0; r < responses[i]; r++ {
			if err := readZKFrame(conn); err != nil {
				return i, fmt.Errorf("read response %d of frame %d: %w", r, i, err)
			}
		}
	}
	return len(frames), nil
}

// readZKFrame reads one complete length-prefixed frame (and discards it — the
// driver asserts via stats, not bytes).
func readZKFrame(conn net.Conn) error {
	if err := conn.SetReadDeadline(time.Now().Add(respReadTimeout)); err != nil {
		return err
	}
	var lenBuf [4]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n > 1<<20 {
		return fmt.Errorf("oversized response frame: %d", n)
	}
	_, err := io.CopyN(io.Discard, conn, int64(n))
	return err
}

// dialZK opens a TCP connection to addr honoring ctx's deadline.
func dialZK(ctx context.Context, addr string) (net.Conn, error) {
	dlr := net.Dialer{}
	conn, err := dlr.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return conn, nil
}

// --- frame-crafting helpers (copied from 0046; D-S28.1-4: small builder funcs) ---

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
// stripped). Layout: protocol_version(4=0) | last_zxid(8=0) | timeout(4=30000) |
// session_id(8=0) | password_len(4=16) | password(16 NUL bytes) |
// [readonly(1=1) if readonly]. The xid is 0 (connectXid).
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
func dataFrame(xid, opcode int32, payload []byte) []byte {
	return zkFrame(be32(xid), be32(opcode), payload)
}

// pingFrame builds a ZooKeeper ping request frame (xid=-2, opcode=11).
func pingFrame() []byte {
	return zkFrame(be32(-2), be32(drvOpPing))
}

// getdataPayload builds the payload for a GetData request (opcode 4).
// Layout: pathLen(4) | path | watch(1). Min frame = xid+opcode+INT+BOOL = 13.
// With path="/zk-test" (8): frame = 4+4+4+8+1 = 21 ≥ 13. SAFE.
func getdataPayload(path string) []byte {
	p := append(be32(int32(len(path))), []byte(path)...)
	p = append(p, 0) // watch = false
	return p
}

// createPayload builds the payload for a Create (opcode 1) request.
// Layout: pathLen(4) | path | dataLen(4) | data | aclCount(4) | flags(4).
// Min frame = xid+opcode+4*INT = 24. With path (8): 4+4+4+8+4+4+4 = 32 ≥ 24. SAFE.
func createPayload(path string) []byte {
	p := append(be32(int32(len(path))), []byte(path)...)
	p = append(p, be32(0)...) // dataLen = 0
	p = append(p, be32(0)...) // aclCount = 0
	p = append(p, be32(0)...) // flags = 0
	return p
}

// existsPayload builds the payload for an Exists request (opcode 3) — the
// watch-event-push trigger. Layout: pathLen(4) | path | watch(1).
// Min frame = xid+opcode+INT+BOOL = 13. With path (8): 21 ≥ 13. SAFE.
func existsPayload(path string) []byte {
	p := append(be32(int32(len(path))), []byte(path)...)
	p = append(p, 0) // watch = false
	return p
}

// getaclPayload builds the payload for a GetAcl request (opcode 6) — the
// wrong-xid trigger. Layout: pathLen(4) | path. Min frame = xid+opcode+INT = 12.
// With path (8): 4+4+4+8 = 20 ≥ 12. SAFE.
func getaclPayload(path string) []byte {
	return append(be32(int32(len(path))), []byte(path)...)
}

// syncPayload builds the payload for a Sync request (opcode 9 — pathOnlyRequest).
// Layout: pathLen(4) | path. Min frame = xid+opcode+INT = 12. With path (8):
// 20 ≥ 12. SAFE.
func syncPayload(path string) []byte {
	return append(be32(int32(len(path))), []byte(path)...)
}

// setdataPayload builds the payload for a SetData request (opcode 5).
// Layout: pathLen(4) | path | dataLen(4) | data | version(4).
// Min frame = xid+opcode+3*INT = 20. With path (8): 4+4+4+8+4+4 = 28 ≥ 20. SAFE.
func setdataPayload(path string) []byte {
	p := append(be32(int32(len(path))), []byte(path)...)
	p = append(p, be32(0)...)  // dataLen = 0
	p = append(p, be32(-1)...) // version = -1 (any)
	return p
}

// deletePayload builds the payload for a Delete request (opcode 2).
// Layout: pathLen(4) | path | version(4). Min frame = xid+opcode+2*INT = 16.
// With path (8): 4+4+4+8+4 = 24 ≥ 16. SAFE.
func deletePayload(path string) []byte {
	p := append(be32(int32(len(path))), []byte(path)...)
	p = append(p, be32(-1)...) // version = -1 (any)
	return p
}

// closePayload is empty — close frames carry no payload beyond xid+opcode.
// Min frame = xid+opcode = 8 (universal min). SAFE.
func closePayload() []byte { return nil }

// --- fixture.StatsAsserter (asserter-dispatch memory: cross-side MUST use
// StatsAsserter; SubjectAsserter would be a dead vacuous assertion) ---

// AssertStats scrapes /stats/prometheus from BOTH admin endpoints and asserts
// the per-opcode zookeeper RESPONSE counters after the 8-arm workload (SPEC
// §5.2). The runner invokes this ONCE with both admin addresses, so the
// scrape-and-diff for both sides happens in-band here.
//
// Counters are looked up via the FLATTENED Prometheus form
// envoy_<prefix>_zookeeper_<suffix> (no labels — the zookeeper_proxy filter has
// no tag extraction). A name-shape mismatch on EITHER side makes the lookup miss
// → ABSENT → the assertion fails. Expected values are CUMULATIVE over in-order
// arms sharing a listener.
func (d *zkResponsesDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeZKStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref zookeeper stats: %v", err)
	}
	subjStats, err := scrapeZKStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj zookeeper stats: %v", err)
	}

	if os.Getenv("FIXTURE_0048_DUMP_STATS") != "" {
		dump := func(label string, m map[string]int64) {
			fmt.Fprintf(os.Stderr, "=== %s zookeeper stats ===\n", label)
			for k, v := range m {
				fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
			}
		}
		dump("ref", refStats)
		dump("subj", subjStats)
	}

	type expect struct {
		metric string
		want   int64
	}
	expectations := []expect{
		// --- l_resp (latency + resp-bytes flags OFF) — arms 1-3 cumulative ---
		{"zk_resp.zookeeper.connect_rq", 1}, {"zk_resp.zookeeper.connect_resp", 1},
		{"zk_resp.zookeeper.getdata_rq", 1}, {"zk_resp.zookeeper.getdata_resp", 1},
		{"zk_resp.zookeeper.create_rq", 1}, {"zk_resp.zookeeper.create_resp", 1},
		{"zk_resp.zookeeper.ping_rq", 1}, {"zk_resp.zookeeper.ping_resp", 1},
		{"zk_resp.zookeeper.close_rq", 1}, {"zk_resp.zookeeper.close_resp", 1},
		{"zk_resp.zookeeper.exists_rq", 1}, {"zk_resp.zookeeper.exists_resp", 1},
		{"zk_resp.zookeeper.watch_event", 1},
		{"zk_resp.zookeeper.getacl_rq", 1}, {"zk_resp.zookeeper.getacl_resp", 0},
		{"zk_resp.zookeeper.decoder_error", 1},
		{"zk_resp.zookeeper.sync_rq", 1}, {"zk_resp.zookeeper.sync_resp", 1},
		{"zk_resp.zookeeper.connect_resp_fast", 0}, {"zk_resp.zookeeper.connect_resp_slow", 0},
		{"zk_resp.zookeeper.getdata_resp_fast", 0}, {"zk_resp.zookeeper.getdata_resp_slow", 0},
		{"zk_resp.zookeeper.getdata_resp_bytes", 0},

		// --- l_fast (3600s default → ALL fast) — arm 4 ---
		{"zk_fast.zookeeper.connect_resp", 1}, {"zk_fast.zookeeper.connect_resp_fast", 1}, {"zk_fast.zookeeper.connect_resp_slow", 0},
		{"zk_fast.zookeeper.getdata_resp", 1}, {"zk_fast.zookeeper.getdata_resp_fast", 1}, {"zk_fast.zookeeper.getdata_resp_slow", 0},
		{"zk_fast.zookeeper.setdata_resp", 1}, {"zk_fast.zookeeper.setdata_resp_fast", 1}, {"zk_fast.zookeeper.setdata_resp_slow", 0},

		// --- l_slow (1ms default + ≥10ms responder delay → ALL slow; GetData
		// override 3600s → FAST) — arms 5-6 ---
		// review-fix: assert TOTAL _resp counters (not just bucket split) so a bug
		// that increments one but not the other would be caught.
		{"zk_slow.zookeeper.connect_resp", 1},
		{"zk_slow.zookeeper.connect_resp_slow", 1}, {"zk_slow.zookeeper.connect_resp_fast", 0},
		{"zk_slow.zookeeper.setdata_resp", 1},
		{"zk_slow.zookeeper.setdata_resp_slow", 1}, {"zk_slow.zookeeper.setdata_resp_fast", 0},
		{"zk_slow.zookeeper.delete_resp", 1},
		{"zk_slow.zookeeper.delete_resp_slow", 1}, {"zk_slow.zookeeper.delete_resp_fast", 0},
		{"zk_slow.zookeeper.getdata_resp", 1},                                                  // the override arm (total; review-fix)
		{"zk_slow.zookeeper.getdata_resp_fast", 1}, {"zk_slow.zookeeper.getdata_resp_slow", 0}, // the override arm

		// --- l_rflags (resp-bytes flag ON) — arm 7 ---
		// review-fix: assert connect_resp as well (arm 7 sends connect + getdata).
		{"zk_rflags.zookeeper.connect_resp", 1},
		{"zk_rflags.zookeeper.getdata_resp", 1},
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
	//   - zk_resp.zookeeper.request_bytes / response_bytes: the wire-footprint
	//     sums on l_resp; arms 1-3 are the load-bearing contributors.
	//   - zk_fast.zookeeper.request_bytes / response_bytes: review-fix — exercised
	//     by arm 4 but previously unasserted; equality is the robust choice since
	//     the exact value depends on frame sizes that should agree cross-side.
	//   - zk_rflags.zookeeper.connect_resp_bytes: review-fix — arm 7 sends connect
	//     (20-byte response + 4-byte prefix = 24 wire bytes) so the flag-ON
	//     connect_resp_bytes counter is exercised; equality > 0 catches either side
	//     failing to emit it without locking in the exact byte count.
	//   - zk_rflags.zookeeper.getdata_resp_bytes: the flag-ON per-opcode response
	//     bytes for arm 7's getdata — present + equal + > 0 on both sides.
	for _, metric := range []string{
		"zk_resp.zookeeper.request_bytes",
		"zk_resp.zookeeper.response_bytes",
		"zk_fast.zookeeper.request_bytes",        // review-fix: exercised by arm 4, now asserted
		"zk_fast.zookeeper.response_bytes",       // review-fix: exercised by arm 4, now asserted
		"zk_rflags.zookeeper.connect_resp_bytes", // review-fix: arm 7 connect frame, flag ON
		"zk_rflags.zookeeper.getdata_resp_bytes",
	} {
		refV, refOK := lookupZKCounter(refStats, metric)
		subjV, subjOK := lookupZKCounter(subjStats, metric)
		if !refOK || !subjOK || refV != subjV || refV == 0 {
			t.Errorf("cross-side %s: ref=(%d,%v) subj=(%d,%v), want present, equal, and > 0",
				metric, refV, refOK, subjV, subjOK)
		}
	}
}

// --- stats scraping (copied from 0046) ---

// scrapeZKStats issues GET /stats/prometheus against adminAddr and returns a map
// of zookeeper-related counter values keyed by the FLATTENED Prometheus metric
// name (the `envoy_` prefix retained). Both reference Envoy v1.37.2 and envoy-go
// surface the zookeeper_proxy counters as flat names with an EMPTY label set
// (no tag extraction), so we retain only lines whose name contains the
// `_zookeeper_` infix and key by the bare metric name.
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
// all lines whose metric name contains the `_zookeeper_` infix.
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
// "zk_resp.zookeeper.connect_resp") against the scraped map by flattening it to
// the Prometheus form: envoy_ prefix + dots → underscores. Returns the value and
// whether the counter was PRESENT — present-vs-absent is the name-shape /
// creation-parity signal.
func lookupZKCounter(stats map[string]int64, dotted string) (int64, bool) {
	prom := "envoy_" + strings.ReplaceAll(dotted, ".", "_")
	v, ok := stats[prom]
	return v, ok
}

// --- bootstrap rendering ---

type bootstrapParams struct {
	adminAddr   string // "<ip>, port_value: <n>" for the admin socket_address
	listenAddr  string // listener bind address (0.0.0.0 for ref; 127.0.0.1 for subj)
	respPort    int    // l_resp listener port
	fastPort    int    // l_fast listener port
	slowPort    int    // l_slow listener port
	rflagsPort  int    // l_rflags listener port
	clusterType string // STRICT_DNS (ref) | STATIC (subj)
	dnsLine     string // "      dns_lookup_family: V4_ONLY\n" for STRICT_DNS, else ""
	backendHost string
	backendPort int
	nodeLine    string // "node: {...}\n" for subj, "" for ref
}

// zkProxyType is the zookeeper_proxy typed_config @type URL. The network-filter
// type URLs carry the extensions. segment (reference_network_filter_typeurl_extensions).
const zkProxyType = "type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy"
const tcpProxyType = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// zkListener renders one listener whose chain is [zookeeper_proxy, tcp_proxy] →
// cluster c_zk. extraCfg is appended (indented to the typed_config block) to add
// the per-listener latency / resp-bytes flags.
func zkListener(name, listenAddr string, port int, statPrefix, tcpStatPrefix, extraCfg string) string {
	return fmt.Sprintf(`    - name: %s
      address:
        socket_address: { address: %s, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.zookeeper_proxy
              typed_config:
                "@type": %s
                stat_prefix: %s
%s            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": %s
                stat_prefix: %s
                cluster: c_zk
`, name, listenAddr, port, zkProxyType, statPrefix, extraCfg, tcpProxyType, tcpStatPrefix)
}

// renderBootstrap assembles the full four-listener bootstrap. Each listener's
// filter chain is [zookeeper_proxy, tcp_proxy] → c_zk (the shared
// TCPZKResponder backend AND the boot-satisfying cluster).
func renderBootstrap(p bootstrapParams) string {
	respL := zkListener("l_resp", p.listenAddr, p.respPort, statPrefixResp, "tcp_resp", "")
	fastL := zkListener("l_fast", p.listenAddr, p.fastPort, statPrefixFast, "tcp_fast",
		"                enable_latency_threshold_metrics: true\n"+
			"                default_latency_threshold: 3600s\n")
	slowL := zkListener("l_slow", p.listenAddr, p.slowPort, statPrefixSlow, "tcp_slow",
		"                enable_latency_threshold_metrics: true\n"+
			"                default_latency_threshold: 0.001s\n"+
			"                latency_threshold_overrides:\n"+
			"                  - opcode: GetData\n"+
			"                    threshold: 3600s\n")
	rflagsL := zkListener("l_rflags", p.listenAddr, p.rflagsPort, statPrefixRflags, "tcp_rflags",
		"                enable_per_opcode_response_bytes_metrics: true\n")

	return fmt.Sprintf(`%sadmin:
  address:
    socket_address: { address: %s }
static_resources:
  listeners:
%s%s%s%s  clusters:
    - name: c_zk
      type: %s
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s      load_assignment:
        cluster_name: c_zk
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: %s, port_value: %d }
`,
		p.nodeLine,
		p.adminAddr,
		respL, fastL, slowL, rflagsL,
		p.clusterType,
		p.dnsLine,
		p.backendHost, p.backendPort,
	)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver              = (*zkResponsesDriver)(nil)
	_ fixture.MultiListenerDriver = (*zkResponsesDriver)(nil)
	_ fixture.BackendKindAware    = (*zkResponsesDriver)(nil)
	_ fixture.StatsAsserter       = (*zkResponsesDriver)(nil)
)
