// Package driver registers the 0051-mongo-responses cross-side differential
// fixture with the runner per phase 29.2 SPEC §6.2 + IMPL Task 11A. It is the
// RESPONSE-SIDE sibling of 0049-mongo-requests: a single listener
// (l_resp, stat_prefix mongo_r) whose filter chain is [mongo_proxy, tcp_proxy]
// fronts a MongoDB-aware canned-RESPONSE backend (BackendKind=TCPMongoResponder).
//
// ============================================================================
// What this fixture proves (29.2 response-side scope)
// ============================================================================
//
// 29.1 proved the REQUEST-side decode (op_query / cmd / collection / callsite /
// decoding_error families). 29.2 adds the RESPONSE side: the responder backend
// writes correlated OP_REPLY / OP_COMMANDREPLY frames back through the chain, so
// reference Envoy's onWrite response decoder (and envoy-go's mirror) fires:
//
//   - op_reply + the reply-flag/cursor counters (cursor_not_found / query_failure
//     / valid_cursor)
//   - op_command_reply
//   - decoding_error on a malformed reply
//   - the op_query_active GAUGE: Inc on each OP_QUERY, Dec on the correlated
//     reply (first-match-erase by responseTo), residual drain at connection close.
//
// The driver only SENDS REQUESTS (with marker requestIDs the responder
// recognizes); the RESPONDER emits the replies, which traverse the chain and are
// decoded by the response-side decoder on both sides. AssertStats then asserts
// per-stat counter parity + the gauge quiesced-point (== 0 at rest) + the
// cx_destroy presence boundary (D-P4).
//
// # Backend choice: TCPMongoResponder (not TCPSink)
//
// 29.1's 0049 used a silent TCPSink so NO response bytes traversed the chain
// (request-only scope). 29.2 is response-side, so it NEEDS a backend that writes
// correlated replies — TCPMongoResponder (fixture.go:520; the acceptMongoResponder
// loop in runner_test.go). It parses messageLength + requestID + opCode only and
// writes a correlated frame whose responseTo echoes the request requestID; marker
// requestIDs select the reply variant (the dMarker* consts below MIRROR the
// responder's mongoMarker* values so both sides send identical bytes).
//
// # Single listener, one stat_prefix
//
//   - l_resp: stat_prefix=mongo_r. All six arms share this listener (the
//     cumulative arm-accounting table above driveProxy is authoritative).
//
// # Cross-references
//
//   - phase 29.2 SPEC §6.2 (the six arms) + §3.3/§3.4 (correlation + gauge)
//   - 29.2 IMPL Task 11A (this file)
//   - fixture-0049-mongo-requests (the STRUCTURAL TEMPLATE — wire builders,
//     bootstrap rendering, the label-aware scrape, AssertStats shape)
//   - ADR-0224 (mongo_proxy filter architecture)
//   - project memory reference_close_direction_framework_gap (cx_destroy_* is a
//     framework gap — presence-only at 29.2, deferred to 29.3)
//   - project memory reference_differential_asserter_dispatch (StatsAsserter is
//     load-bearing for cross-side; SubjectAsserter would be vacuous here)
package driver

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0051-mongo-responses"

	refAdminPort = 9901

	// In-container reference Envoy listener port for l_resp. Distinct from
	// 0049's 19140/19141 (a free port in the same family per SPEC §6.2).
	refLRespPort = 19142

	// stat_prefix root for the l_resp listener's mongo_proxy config.
	statPrefixResp = "mongo_r"
)

func init() {
	fixture.RegisterFixture(fixtureName, &mongoResponsesDriver{})
}

// mongoResponsesDriver carries no mutable cross-arm state. The unanswered-gauge
// while-open assertion uses approach (B) (unit-covered; see the README), so no
// connection is held across arms in a driver field.
type mongoResponsesDriver struct{}

// dMarker* MIRROR the responder's mongoMarker* requestID values (runner_test.go,
// a DIFFERENT package — duplicated here so both sides send byte-identical frames).
// A plain (non-marker) reqID → a plain empty OP_REPLY.
const (
	dMarkerWithhold       int32 = 7777 // responder withholds the reply (unanswered-query)
	dMarkerCursorNotFound int32 = 7001 // OP_REPLY responseFlags 0x01
	dMarkerQueryFailure   int32 = 7002 // OP_REPLY responseFlags 0x02
	dMarkerValidCursor    int32 = 7003 // OP_REPLY cursorID 4242 + 1 doc
	dMarkerMalformedReply int32 = 7004 // OP_REPLY numberReturned=1 with NO doc → decoding_error
	dMarkerUncorrelated   int32 = 7005 // OP_REPLY responseTo = reqID+50000 (correlation miss)
)

// --- little-endian mongo wire builders (D-S29.1-3; copied verbatim from the
// 0049 driver so the fixture is self-contained). The driver only SENDS requests,
// so only the request builders (opQuery / opCommand) are needed here. ---

func leI32(v int32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return b
}
func cstr(s string) []byte { return append([]byte(s), 0x00) }

// bsonInt32 builds a {name: int32} element.
func bsonInt32(name string, v int32) []byte {
	return append(append([]byte{0x10}, cstr(name)...), leI32(v)...)
}

// bsonDoc wraps element bytes into a BSON document (int32 len incl self + 0x00).
// The body is built on a defensive COPY of elems so a caller's element backing
// array is never mutated by the terminating 0x00 append.
func bsonDoc(elems ...byte) []byte {
	body := append(append([]byte(nil), elems...), 0x00)
	return append(leI32(int32(4+len(body))), body...)
}

// mongoMsg wraps a body in a 16-byte LE MsgHeader (messageLength incl header).
func mongoMsg(reqID, opCode int32, body []byte) []byte {
	out := append(leI32(int32(16+len(body))), leI32(reqID)...)
	out = append(out, leI32(0)...)      // responseTo
	out = append(out, leI32(opCode)...) // opCode
	return append(out, body...)
}

// opQuery builds a complete OP_QUERY (2004) message.
func opQuery(reqID int32, fullColl string, flags int32, queryDoc []byte) []byte {
	body := append(leI32(flags), cstr(fullColl)...)
	body = append(body, leI32(0)...) // numberToSkip
	body = append(body, leI32(0)...) // numberToReturn
	body = append(body, queryDoc...)
	return mongoMsg(reqID, 2004, body)
}

// opCommand builds a complete OP_COMMAND (2010) message.
func opCommand(reqID int32, db, cmd string) []byte {
	body := append(cstr(db), cstr(cmd)...)
	body = append(body, bsonDoc()...)                     // metadata
	body = append(body, bsonDoc(bsonInt32(cmd, 1)...)...) // commandArgs
	return mongoMsg(reqID, 2010, body)
}

// --- fixture.Driver (required) ---

// BackendCount returns 1: a single TCPMongoResponder backend (c_resp cluster).
func (*mongoResponsesDriver) BackendCount() int { return 1 }

// SubjectListenerName returns the single listener name (l_resp).
func (*mongoResponsesDriver) SubjectListenerName() string { return "l_resp" }

// ReferenceListenerPort returns the reference listener port (l_resp).
func (*mongoResponsesDriver) ReferenceListenerPort() int { return refLRespPort }

// BackendKind returns TCPMongoResponder: the canned-response backend that writes
// correlated OP_REPLY / OP_COMMANDREPLY frames (29.2 response-side scope).
func (*mongoResponsesDriver) BackendKind() fixture.BackendKind { return fixture.TCPMongoResponder }

// ReferenceBootstrap renders the single-listener reference bootstrap. c_resp
// points at host.docker.internal:<backend> (ADR-0010 STRICT_DNS) so the
// dockerized reference can reach the host-side responder backend.
func (*mongoResponsesDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("0.0.0.0, port_value: %d", refAdminPort),
		listenAddr:  "0.0.0.0",
		respPort:    refLRespPort,
		clusterType: "STRICT_DNS",
		dnsLine:     "      dns_lookup_family: V4_ONLY\n",
		backendHost: "host.docker.internal",
		backendPort: backendPorts[0],
		nodeLine:    "",
	})
}

// SubjectConfig renders the single-listener subject bootstrap.
func (*mongoResponsesDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("127.0.0.1, port_value: %d", subjAdminPort),
		listenAddr:  "127.0.0.1",
		respPort:    subjListenerPort,
		clusterType: "STATIC",
		dnsLine:     "",
		backendHost: "127.0.0.1",
		backendPort: backendPorts[0],
		nodeLine:    "node: { id: envoy-go-subject-0051, cluster: envoy-go-differential }\n",
	})
}

// DriveReference / DriveSubject run the identical six-arm workload against each
// side's l_resp listener and return a side-independent verdict byte stream.
func (d *mongoResponsesDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "ref")
}

func (d *mongoResponsesDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (*mongoResponsesDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- scenario driving (SPEC §6.2: the six arms) ---

const (
	// dbColl is the full collection name for the request arms. The token after
	// the first '.' (collection1) is the tag-extracted collection label value.
	dbColl = "db.collection1"

	// readReplyTimeout bounds the conn.Read for a reply so a WITHHELD reply (the
	// unanswered-query arm) does not block the driver forever. The answered arms
	// read their reply well within this window; the withhold arm times out and
	// proceeds to close the connection (residual drain).
	readReplyTimeout = 300 * time.Millisecond

	// settleDelay lets the async stat pipeline on both sides catch up before
	// AssertStats scrapes (the 0049 sleep-to-settle precedent).
	settleDelay = 750 * time.Millisecond
)

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ CUMULATIVE ARM-ACCOUNTING TABLE (the 0049 discipline) — arms 1-6          │
// │ single listener l_resp (mongo_r). Per-prefix counters are CUMULATIVE.     │
// ├──────────────────────────────────────────────────────────────────────────┤
// │                          a1   a2cnf a2qf a2vc  a3   a4   a5   a6  | want   │
// │   what                  plain  7001 7002 7003 cmd  whold uncor malf|       │
// │  ──────────────────────  ────  ──── ──── ──── ───  ──── ──── ───| ────────│
// │  op_query (req side)      ✓    ✓    ✓    ✓    .    ✓    ✓    ✓  | 7? see↓ │
// │  op_command (req side)    .    .    .    .    ✓    .    .    .  | 1       │
// │  op_reply                 ✓    ✓    ✓    ✓    .    .    ✓    .  | 5       │
// │  op_reply_cursor_not_found.    ✓    .    .    .    .    .    .  | 1       │
// │  op_reply_query_failure   .    .    ✓    .    .    .    .    .  | 1       │
// │  op_reply_valid_cursor    .    .    .    ✓    .    .    .    .  | 1       │
// │  op_command_reply         .    .    .    .    ✓    .    .    .  | 1       │
// │  decoding_error           .    .    .    .    .    .    .    ✓  | 1       │
// │  op_query_active (gauge)  0    0    0    0    0    0→0  0→0  0  | 0 @rest │
// └──────────────────────────────────────────────────────────────────────────┘
//
// op_query count: arms 1,2cnf,2qf,2vc,4-withhold,5-uncorrelated,6-malformed each
// send ONE OP_QUERY = 7. Arm 3 is OP_COMMAND (not OP_QUERY). So op_query == 7,
// op_command == 1.   NOTE: the PLAN's estimate said op_query==6 counting only
// "arms 1,2×3,4ii,5"; it OMITTED arm 6's OP_QUERY (the malformed-reply arm still
// SENDS a valid OP_QUERY request — the malformation is in the RESPONSE). The
// LIVE reference is ground truth; this table re-counts to 7 and is verified
// cross-side. (Both the request AND the response are decoded; the request is a
// valid OP_QUERY on both sides regardless of the reply's malformation.)
//
// op_reply == 5: arm1 plain(1) + arm2 cnf(1)+qf(1)+vc(1) + arm5 uncorrelated(1).
// Arm 4 (withhold) sends NO reply. Arm 6 (malformed) → decoding_error not
// op_reply (the reply body fails to decode before the op_reply inc — see codec.go
// decodeReply: the parseDocument loop fails first, returning d.fail()).
//
// op_query_active gauge: the load-bearing cross-side proof is ANSWERED → 0
// (every correlated reply Decs the gauge; arms 1,2,3-cmd,5 all answer or are
// gauge-neutral). The UNANSWERED arms (4-withhold, 5-uncorrelated's residual)
// leave a query in-flight that drains at that connection's CLOSE → gauge back to
// 0. So at AssertStats time (all conns closed) the gauge is 0 on both sides. The
// unanswered==1 WHILE-OPEN point is unit-covered (Task 6 lifecycle tests);
// approach (B) per the README.

// driveProxy runs the six arms in declared order against the proxy listener at
// addr, closing each connection so the gauge quiesces, then sleeps settleDelay
// before returning. The "side" label is diagnostic-only and is NEVER written to
// the returned bytes, so equivalent behavior yields byte-identical output for the
// runner's CompareBytes gate.
func (d *mongoResponsesDriver) driveProxy(ctx context.Context, addr, side string) ([]byte, error) {
	var b bytes.Buffer

	// Arm 1 (plain reply round-trip): OP_QUERY reqID 1 (plain non-marker) → the
	// responder's empty OP_REPLY (responseTo 1) → op_reply +1; correlated → gauge
	// Inc then Dec → settles 0. driveAndReadReply reads the reply so the response
	// decode + correlation complete before the connection closes (D-P9).
	rep, err := driveAndReadReply(ctx, addr, opQuery(1, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "plain-reply", rep, err)

	// Arm 2 (reply-flag variants): three FRESH connections, marker reqIDs →
	// op_reply_cursor_not_found / _query_failure / _valid_cursor each +1; each
	// correlated → gauge Inc/Dec settles 0.
	rep, err = driveAndReadReply(ctx, addr, opQuery(dMarkerCursorNotFound, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "cursor-not-found", rep, err)
	rep, err = driveAndReadReply(ctx, addr, opQuery(dMarkerQueryFailure, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "query-failure", rep, err)
	rep, err = driveAndReadReply(ctx, addr, opQuery(dMarkerValidCursor, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "valid-cursor", rep, err)

	// Arm 3 (OP_COMMAND round-trip): OP_COMMAND reqID 20 → OP_COMMANDREPLY →
	// op_command_reply +1. OP_COMMAND does NOT append an active-query entry, so the
	// gauge is untouched (only OP_REPLY correlates against the active-query list).
	rep, err = driveAndReadReply(ctx, addr, opCommand(20, "db", "ping"))
	emitArm(&b, side, "command-reply", rep, err)

	// Arm 4 (unanswered-query gauge / residual drain): OP_QUERY reqID 7777 — the
	// responder WITHHOLDS the reply → the query stays in-flight while the
	// connection is open (gauge == 1, unit-covered). driveAndReadReply's bounded
	// read times out (no reply), then closes the connection → onDestroy drains the
	// residual → gauge back to 0. Approach (B): the while-open ==1 is unit-covered;
	// this arm exercises the residual-drain-at-close path cross-side.
	rep, err = driveAndReadReply(ctx, addr, opQuery(dMarkerWithhold, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "withhold-residual-drain", rep, err)

	// Arm 5 (uncorrelated reply): OP_QUERY reqID 7005 → the responder emits a reply
	// whose responseTo (reqID+50000) matches NO sent query → op_reply +1, the
	// correlation MISS leaves the gauge untouched. The 7005 query itself stays
	// in-flight → drains at this connection's close → gauge back to 0.
	rep, err = driveAndReadReply(ctx, addr, opQuery(dMarkerUncorrelated, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "uncorrelated", rep, err)

	// Arm 6 (malformed-reply decoding_error, FRESH conn): OP_QUERY reqID 7004 → the
	// responder emits a malformed OP_REPLY (numberReturned=1 with NO doc) →
	// decoding_error +1 on BOTH sides (same bytes). The request itself is a valid
	// OP_QUERY (counted in op_query). Sniffing turns off after the decode error, so
	// any follow-up on the same conn would add nothing — this arm sends only the one
	// frame and closes.
	rep, err = driveAndReadReply(ctx, addr, opQuery(dMarkerMalformedReply, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "malformed-reply", rep, err)

	// Let the async stat pipeline settle before the runner scrapes in AssertStats.
	if err := sleepCtx(ctx, settleDelay); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// emitArm writes a side-independent verdict line for one arm. The side label is
// logged to stderr (diagnostic) but never to the returned byte stream. A read
// error (e.g. the withhold arm's read timeout) is NOT a failure — the verdict
// records whether a reply was observed, which is itself side-independent only
// for the answered arms; the withhold arm is EXPECTED to read nothing on both
// sides, so its verdict ("noreply") is byte-identical cross-side.
func emitArm(b *bytes.Buffer, side, name string, replyLen int, err error) {
	verdict := "ok"
	if err != nil {
		fmt.Fprintf(os.Stderr, "[fixture 0051 %s] arm %s: %v\n", side, name, err)
		verdict = "ERR"
	} else if replyLen == 0 {
		verdict = "noreply"
	}
	fmt.Fprintf(b, "arm %s verdict=%s\n", name, verdict)
}

// driveAndReadReply opens a fresh TCP connection to addr, writes frame, then reads
// back any reply bytes with a bounded deadline (so a WITHHELD reply does not block
// forever), and closes. Returns the number of reply bytes read (0 if the reply was
// withheld / timed out — NOT an error for the unanswered arm). A read timeout is
// folded into a nil error + replyLen 0 so the withhold arm's verdict is
// side-independent. The connection close drives onDestroy (the residual drain).
func driveAndReadReply(ctx context.Context, addr string, frame []byte) (int, error) {
	dl := net.Dialer{}
	conn, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write(frame); err != nil {
		return 0, fmt.Errorf("write frame: %w", err)
	}

	// Bounded read: an answered arm reads its reply; the withhold arm times out
	// with 0 bytes. A timeout is NOT an error here (it is the expected withhold
	// outcome on both sides).
	_ = conn.SetReadDeadline(time.Now().Add(readReplyTimeout))
	buf := make([]byte, 4096)
	n, rerr := conn.Read(buf)
	if rerr != nil {
		if ne, ok := rerr.(net.Error); ok && ne.Timeout() {
			return 0, nil // withheld reply — expected, side-independent
		}
		if rerr == io.EOF {
			return n, nil // backend closed after writing (or without); not fatal
		}
		// Any other read error (e.g. connection reset) — record 0, no hard fail so
		// both sides remain byte-identical on the verdict line.
		return n, nil
	}
	return n, nil
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

// --- fixture.StatsAsserter (asserter-dispatch memory: cross-side MUST use
// StatsAsserter; SubjectAsserter would be a dead vacuous assertion) ---

// AssertStats scrapes /stats/prometheus from BOTH admin endpoints and asserts the
// response-side mongo counters + the gauge quiesced-point + the cx_destroy
// presence boundary after the six-arm workload (SPEC §6.2). Counters are looked
// up by the CANONICAL Prometheus form (labels sorted) so a value lookup
// intrinsically asserts BOTH name-parity AND label-extraction parity. An ABSENT
// key is reported DISTINCTLY from a present-but-wrong value (the 0049 discipline).
func (d *mongoResponsesDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeMongoStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref mongo stats: %v", err)
	}
	subjStats, err := scrapeMongoStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj mongo stats: %v", err)
	}

	if os.Getenv("FIXTURE_0051_DUMP_STATS") != "" {
		dump := func(label string, m map[string]int64) {
			fmt.Fprintf(os.Stderr, "=== %s mongo stats ===\n", label)
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(os.Stderr, "  %s = %d\n", k, m[k])
			}
		}
		dump("ref", refStats)
		dump("subj", subjStats)
	}

	// Expectations keyed by the canonical Prometheus form. Each `want` is the
	// CUMULATIVE value from the arm-accounting table above driveProxy (re-verified
	// LIVE cross-side — see the op_query==7 note in that table).
	expectations := []struct {
		key  string
		want int64
	}{
		// Request side (the requests the driver sends; all on mongo_r):
		{`envoy_mongo_op_query{envoy_mongo_prefix="mongo_r"}`, 7},   // arms 1,2×3,4,5,6 (arm3 is OP_COMMAND)
		{`envoy_mongo_op_command{envoy_mongo_prefix="mongo_r"}`, 1}, // arm 3
		// Response side (the replies the responder emits, decoded on both sides):
		{`envoy_mongo_op_reply{envoy_mongo_prefix="mongo_r"}`, 5}, // arm1 + arm2×3 + arm5
		{`envoy_mongo_op_reply_cursor_not_found{envoy_mongo_prefix="mongo_r"}`, 1},
		{`envoy_mongo_op_reply_query_failure{envoy_mongo_prefix="mongo_r"}`, 1},
		{`envoy_mongo_op_reply_valid_cursor{envoy_mongo_prefix="mongo_r"}`, 1},
		{`envoy_mongo_op_command_reply{envoy_mongo_prefix="mongo_r"}`, 1},
		{`envoy_mongo_decoding_error{envoy_mongo_prefix="mongo_r"}`, 1}, // arm 6 malformed reply
	}

	for _, sd := range []struct {
		label string
		stats map[string]int64
	}{{"ref", refStats}, {"subj", subjStats}} {
		for _, exp := range expectations {
			got, present := sd.stats[canonicalize(exp.key)]
			if !present {
				t.Errorf("%s: counter %s ABSENT (creation / name-shape / label-extraction failure)", sd.label, exp.key)
				continue
			}
			if got != exp.want {
				t.Errorf("%s %s = %d, want %d", sd.label, exp.key, got, exp.want)
			}
		}
	}

	// --- gauge quiesced-point + exists-at-zero + cx_destroy presence ---
	//
	// The op_query_active gauge: PRESENT, `# TYPE … gauge`, == 0 at rest (all
	// connections closed by AssertStats time — every answered query Dec'd the
	// gauge, every unanswered/uncorrelated residual drained at connection close).
	// This is the load-bearing cross-side gauge proof (answered → 0 + residual
	// drain → 0). The unanswered ==1 WHILE-OPEN is unit-covered (approach B).
	for _, sd := range []struct {
		label string
		stats map[string]int64
	}{{"ref", refStats}, {"subj", subjStats}} {
		gaugeKey := fmt.Sprintf("envoy_mongo_op_query_active{envoy_mongo_prefix=%q}", statPrefixResp)
		got, present := sd.stats[canonicalize(gaugeKey)]
		if !present {
			t.Errorf("%s: gauge %s ABSENT (creation failure)", sd.label, gaugeKey)
		} else if got != 0 {
			t.Errorf("%s: %s = %d, want 0 (gauge quiesced at rest — all conns closed)", sd.label, gaugeKey, got)
		}

		// delays_injected + cx_drain_close: PRESENT, == 0 both sides (the response
		// side does not inject delays or drain-close in this fixture).
		for _, suf := range []string{"delays_injected", "cx_drain_close"} {
			key := fmt.Sprintf("envoy_mongo_%s{envoy_mongo_prefix=%q}", suf, statPrefixResp)
			v, ok := sd.stats[canonicalize(key)]
			if !ok {
				t.Errorf("%s: counter %s ABSENT (exists-at-zero creation failure)", sd.label, key)
				continue
			}
			if v != 0 {
				t.Errorf("%s: %s = %d, want 0", sd.label, key, v)
			}
		}

		// cx_destroy_local/remote_with_active_rq: PRESENT both sides, value NOT
		// compared (D-P4 close-direction coverage boundary — the reference
		// increments one per query-bearing close; envoy-go increments neither until
		// 29.3, so the cross-side VALUES legitimately differ and only PRESENCE is
		// asserted; reference_close_direction_framework_gap).
		for _, suf := range []string{"cx_destroy_local_with_active_rq", "cx_destroy_remote_with_active_rq"} {
			key := fmt.Sprintf("envoy_mongo_%s{envoy_mongo_prefix=%q}", suf, statPrefixResp)
			if _, ok := sd.stats[canonicalize(key)]; !ok {
				t.Errorf("%s: counter %s ABSENT (exists-at-zero creation failure)", sd.label, key)
			}
		}
	}

	// The gauge TYPE line is keyed by the BARE metric name (the prefix lives in a
	// label on the value line, not on the `# TYPE` line). Asserted once per side.
	for _, sd := range []struct {
		label string
		addr  string
	}{{"ref", refAdminAddr}, {"subj", subjAdminAddr}} {
		typeLine, err := scrapeTypeLine(sd.addr, "envoy_mongo_op_query_active")
		if err != nil {
			t.Errorf("%s: scrape gauge TYPE line: %v", sd.label, err)
			continue
		}
		if want := "# TYPE envoy_mongo_op_query_active gauge"; typeLine != want {
			t.Errorf("%s: gauge TYPE line = %q, want %q", sd.label, typeLine, want)
		}
	}
}

// scrapeMongoStats issues GET /stats/prometheus and returns a map keyed by the
// CANONICAL form `name{k1="v1",k2="v2"}` with labels sorted by key, value parsed.
// Retains only envoy_mongo_* lines. (Copied from the 0049 driver — the label-aware
// scrape.)
func scrapeMongoStats(adminAddr string) (map[string]int64, error) {
	body, err := httpGet("http://" + adminAddr + "/stats/prometheus")
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, "envoy_mongo_") {
			continue
		}
		sp := strings.LastIndexByte(line, ' ')
		if sp < 0 {
			continue
		}
		nameLabels, valStr := line[:sp], line[sp+1:]
		v, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			continue
		}
		out[canonicalize(nameLabels)] = v
	}
	return out, nil
}

// scrapeTypeLine issues GET /stats/prometheus and returns the single
// `# TYPE <name> <type>` line for the given metric NAME. (Copied from 0049.)
func scrapeTypeLine(adminAddr, name string) (string, error) {
	body, err := httpGet("http://" + adminAddr + "/stats/prometheus")
	if err != nil {
		return "", err
	}
	want := "# TYPE " + name + " "
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, want) {
			return line, nil
		}
	}
	return "", fmt.Errorf("# TYPE line for %q not found", name)
}

// canonicalize normalizes "name{k=\"v\",...}" to "name{k1=\"v1\",k2=\"v2\"}" with
// labels sorted by key. (Copied from 0049; mongo's tags are identifier-only.)
func canonicalize(nameLabels string) string {
	open := strings.IndexByte(nameLabels, '{')
	if open < 0 {
		return nameLabels
	}
	name := nameLabels[:open]
	inner := strings.TrimSuffix(nameLabels[open+1:], "}")
	if inner == "" {
		return name
	}
	pairs := strings.Split(inner, ",")
	sort.Strings(pairs)
	return name + "{" + strings.Join(pairs, ",") + "}"
}

// httpGet issues GET url and returns the response body. (Copied from 0049.)
func httpGet(url string) ([]byte, error) {
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read %s body: %w", url, err)
	}
	return buf.Bytes(), nil
}

// --- bootstrap rendering (single-listener; trimmed from 0049's two-listener) ---

type bootstrapParams struct {
	adminAddr   string // "<ip>, port_value: <n>" for the admin socket_address
	listenAddr  string // listener bind address (0.0.0.0 for ref; 127.0.0.1 for subj)
	respPort    int    // l_resp listener port
	clusterType string // STRICT_DNS (ref) | STATIC (subj)
	dnsLine     string // "      dns_lookup_family: V4_ONLY\n" for STRICT_DNS, else ""
	backendHost string
	backendPort int
	nodeLine    string // "node: {...}\n" for subj, "" for ref
}

// mongoProxyType / tcpProxyType — the network-filter @type URLs carry the
// extensions. segment (reference_network_filter_typeurl_extensions).
const mongoProxyType = "type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy"
const tcpProxyType = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// renderBootstrap assembles the single-listener bootstrap. The l_resp filter
// chain is [mongo_proxy, tcp_proxy] → c_resp (the TCPMongoResponder backend AND
// the boot-satisfying cluster — a zero-cluster boot is rejected by both sides).
func renderBootstrap(p bootstrapParams) string {
	respListener := fmt.Sprintf(`    - name: l_resp
      address:
        socket_address: { address: %s, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.mongo_proxy
              typed_config:
                "@type": %s
                stat_prefix: %s
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": %s
                stat_prefix: tcp_resp
                cluster: c_resp
`, p.listenAddr, p.respPort, mongoProxyType, statPrefixResp, tcpProxyType)

	return fmt.Sprintf(`%sadmin:
  address:
    socket_address: { address: %s }
static_resources:
  listeners:
%s  clusters:
    - name: c_resp
      type: %s
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s      load_assignment:
        cluster_name: c_resp
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: %s, port_value: %d }
`,
		p.nodeLine,
		p.adminAddr,
		respListener,
		p.clusterType,
		p.dnsLine,
		p.backendHost, p.backendPort,
	)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*mongoResponsesDriver)(nil)
	_ fixture.BackendKindAware = (*mongoResponsesDriver)(nil)
	_ fixture.StatsAsserter    = (*mongoResponsesDriver)(nil)
)
