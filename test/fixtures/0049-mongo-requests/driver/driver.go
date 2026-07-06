// Package driver registers the 0049-mongo-requests cross-side differential
// fixture with the runner per phase 29.1 SPEC §8.1 + PLAN Task 14 (part 1:
// bootstraps + wire-byte builders + MultiListener + TCPSink skeleton), Task 15
// (the label-aware StatsAsserter + arms 1-5), and Task 16 (arms 6-9).
//
// ============================================================================
// Part 1 (Task 14) — the DRIVER SKELETON
// ============================================================================
//
// This file lands at Task 14 as a COMPILE-ONLY skeleton: the self-contained
// little-endian mongo wire/BSON builders (D-S29.1-3, shared verbatim with the
// future 29.2 0051 driver), the two-listener bootstraps, the MultiListener
// plumbing, and the BackendKindAware → TCPSink wiring. The StatsAsserter body
// and the nine arms' actual traffic land at Tasks 15-16; the Drive* methods are
// no-op skeletons here that establish the listener addrs and return an empty,
// side-independent byte stream so the runner's CompareBytes gate passes.
//
// It is the FIRST cross-side fixture for the mongo_proxy network filter
// (ADR-0224): each listener's filter chain is [mongo_proxy, tcp_proxy] targeting
// a SILENT TCP sink backend (BackendKind=TCPSink; D-S28.1-5 — request-side-only
// scope, no response bytes traverse the chain on either side), and the driver
// will assert per-opcode / per-command / per-collection counter parity across
// both reference Envoy (dockerized) and envoy-go via the StatsAsserter interface
// once the arms land.
//
// # Backend choice: TCPSink (not TCPEcho)
//
// A TCPEcho backend would push echoed mongo request bytes back through reference
// Envoy's response path, and the 29.1 scope is request-only (SPEC §2 / §8.1).
// A silent sink drains reads without writing, so no response bytes traverse the
// filter chain on either side. This mirrors the 0046-zookeeper-requests choice.
//
// # Two listeners, two stat_prefixes (SPEC §8.1 table)
//
//   - l_default: stat_prefix=mongo_a, default commands ({delete, insert,
//     update}). Exercises arms 1, 3, 4, 5, 6, 7, 8, 9 (the request-opcode +
//     query-shape + callsite + decoding-error families).
//
//   - l_commands: stat_prefix=mongo_b, commands: ["isMaster"]. Exercises arm 2
//     (the AMEND-B7 / D-P8 commands-list proof: {isMaster:1} → cmd.isMaster.total
//     in the list; {foo:1} → cmd.unknown_command.total not in the list).
//
// # Bootstrap discipline
//
// Both listeners' filter chains are [mongo_proxy, tcp_proxy]. The tcp_proxy
// terminal needs an upstream cluster — c_sink (the runner's TCPSink backend).
// A zero-cluster boot is rejected by both sides
// (reference_network_filter_typeurl_extensions memory). The mongo_proxy @type
// URL carries the extensions. segment:
// "type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy"
//
// # Cross-references
//
//   - phase 29.1 SPEC §8.1 (cross-side mongo-requests fixture scope)
//   - 29.1 PLAN Task 14 (this file, part 1) + Task 15/16 (AssertStats + 9 arms)
//   - fixture-0046-zookeeper-requests (cross-side network filter + StatsAsserter
//   - MultiListener + TCPSink structural precedent — 875 LoC)
//   - ADR-0224 (mongo_proxy filter architecture)
//   - project memory reference_differential_asserter_dispatch (StatsAsserter is
//     load-bearing for cross-side; SubjectAsserter would be vacuous here)
package driver

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
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
	fixtureName = "0049-mongo-requests"

	refAdminPort = 9901

	// In-container reference Envoy listener ports. Per SPEC §8.1: l_default →
	// 19140, l_commands → 19141.
	refLDefaultPort  = 19140
	refLCommandsPort = 19141

	// stat_prefix roots for each listener's mongo_proxy config (SPEC §8.1).
	statPrefixDefault  = "mongo_a"
	statPrefixCommands = "mongo_b"
)

func init() {
	fixture.RegisterFixture(fixtureName, &mongoRequestsDriver{})
}

// mongoRequestsDriver carries no mutable cross-arm state — the multi-listener
// matrix is fully deterministic.
type mongoRequestsDriver struct{}

// --- little-endian mongo wire builders (D-S29.1-3; shared with the 29.2 0051
// driver). These MIRROR the codec_test helpers but live in the driver package so
// the fixture is self-contained.) ---

func leI32(v int32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return b
}
func leI64(v int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return b
}
func cstr(s string) []byte { return append([]byte(s), 0x00) }

// bsonInt32 builds a {name: int32} element; bsonString a {name: string} element.
func bsonInt32(name string, v int32) []byte {
	return append(append([]byte{0x10}, cstr(name)...), leI32(v)...)
}
func bsonString(name, v string) []byte {
	out := append([]byte{0x02}, cstr(name)...)
	out = append(out, leI32(int32(len(v)+1))...)
	out = append(out, []byte(v)...)
	return append(out, 0x00)
}
func bsonDocElem(name string, inner []byte) []byte { // {name: <document>}
	return append(append([]byte{0x03}, cstr(name)...), inner...)
}

// bsonDoc wraps element bytes into a BSON document (int32 len incl self + 0x00).
// The body is built on a defensive COPY of elems so a caller's element backing
// array is never mutated by the terminating 0x00 append (the kit is shared with
// the 29.2 0051 driver; an append-aliasing write would corrupt a reused slice).
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

// opQuery builds a complete OP_QUERY message.
func opQuery(reqID int32, fullColl string, flags int32, queryDoc []byte) []byte {
	body := append(leI32(flags), cstr(fullColl)...)
	body = append(body, leI32(0)...) // numberToSkip
	body = append(body, leI32(0)...) // numberToReturn
	body = append(body, queryDoc...)
	return mongoMsg(reqID, 2004, body)
}

// opInsert/opGetMore/opKillCursors/opCommand — the other request opcodes.
func opInsert(reqID int32, fullColl string, docs ...[]byte) []byte {
	body := append(leI32(0), cstr(fullColl)...)
	for _, d := range docs {
		body = append(body, d...)
	}
	return mongoMsg(reqID, 2002, body)
}
func opGetMore(reqID int32, fullColl string, cursorID int64) []byte {
	body := append(leI32(0), cstr(fullColl)...)
	body = append(body, leI32(10)...)
	body = append(body, leI64(cursorID)...)
	return mongoMsg(reqID, 2005, body)
}
func opKillCursors(reqID int32, cursorIDs ...int64) []byte {
	body := append(leI32(0), leI32(int32(len(cursorIDs)))...)
	for _, c := range cursorIDs {
		body = append(body, leI64(c)...)
	}
	return mongoMsg(reqID, 2007, body)
}
func opCommand(reqID int32, db, cmd string) []byte {
	body := append(cstr(db), cstr(cmd)...)
	body = append(body, bsonDoc()...)                     // metadata
	body = append(body, bsonDoc(bsonInt32(cmd, 1)...)...) // commandArgs
	return mongoMsg(reqID, 2010, body)
}
func opMsgFrame(reqID int32) []byte { return mongoMsg(reqID, 2013, nil) } // arm 6's unsupported-opcode (2013) frame

// --- fixture.Driver (required) ---

// BackendCount returns 1: a single TCPSink backend serves both listeners
// (c_sink cluster). The sink is silent, so backend accept counts are a
// secondary signal; the primary assertions are the Prometheus stat counters.
func (*mongoRequestsDriver) BackendCount() int { return 1 }

// SubjectListenerName returns the primary listener name (l_default). Required by
// the Driver interface; MultiListenerDriver takes precedence at runtime.
func (*mongoRequestsDriver) SubjectListenerName() string { return "l_default" }

// ReferenceListenerPort returns the primary reference listener port (l_default).
// Required by the Driver interface even though MultiListenerDriver dispatches at
// runtime.
func (*mongoRequestsDriver) ReferenceListenerPort() int { return refLDefaultPort }

// ReferenceBootstrap renders the two-listener reference bootstrap. c_sink
// points at host.docker.internal:<backend> (ADR-0010 STRICT_DNS).
func (*mongoRequestsDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:    fmt.Sprintf("0.0.0.0, port_value: %d", refAdminPort),
		listenAddr:   "0.0.0.0",
		defaultPort:  refLDefaultPort,
		commandsPort: refLCommandsPort,
		clusterType:  "STRICT_DNS",
		dnsLine:      "      dns_lookup_family: V4_ONLY\n",
		backendHost:  "host.docker.internal",
		backendPort:  backendPorts[0],
		nodeLine:     "",
	})
}

// SubjectConfig renders the two-listener subject bootstrap. The two subject
// listeners get consecutive ports starting from subjListenerPort
// (default=subjListenerPort, commands=+1) per the fixture-0046 multi-listener
// port-offset precedent.
func (*mongoRequestsDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:    fmt.Sprintf("127.0.0.1, port_value: %d", subjAdminPort),
		listenAddr:   "127.0.0.1",
		defaultPort:  subjListenerPort,
		commandsPort: subjListenerPort + 1,
		clusterType:  "STATIC",
		dnsLine:      "",
		backendHost:  "127.0.0.1",
		backendPort:  backendPorts[0],
		nodeLine:     "node: { id: envoy-go-subject-0049, cluster: envoy-go-differential }\n",
	})
}

// --- fixture.MultiListenerDriver ---

func (*mongoRequestsDriver) SubjectListenerNames() []string {
	return []string{"l_default", "l_commands"}
}

func (*mongoRequestsDriver) ReferenceListenerPorts() []int {
	return []int{refLDefaultPort, refLCommandsPort}
}

func (d *mongoRequestsDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.DriveReferenceMulti(ctx, map[string]string{"l_default": addr})
}

func (d *mongoRequestsDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.DriveSubjectMulti(ctx, map[string]string{"l_default": addr})
}

func (d *mongoRequestsDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "ref")
}

func (d *mongoRequestsDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (*mongoRequestsDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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
// default TCPEcho backend (which would produce response bytes outside the
// request-only 29.1 scope).
func (*mongoRequestsDriver) BackendKind() fixture.BackendKind { return fixture.TCPSink }

// --- scenario driving (Task 15: arms 1-5; arms 6-9 land at Task 16) ---

const (
	// dbColl is the full collection name for the db.collection1 query arms. The
	// collection token after the first '.' (collection1) is the tag-extracted
	// envoy_mongo_collection label value (§7.4).
	dbColl = "db.collection1"

	// adminCmd is the full collection name for the $cmd command arms (arm 2). The
	// "$cmd" infix routes the OP_QUERY to the command path (cmd.<name>.total).
	adminCmd = "admin.$cmd"

	// interWriteDelay paces successive writes on a multi-frame connection so both
	// decoders observe the same read boundaries (the 0046 precedent). Both
	// decoders coalesce, so this is for cross-side determinism, not correctness.
	interWriteDelay = 50 * time.Millisecond

	// settleDelay lets the async stat pipeline on both sides catch up before
	// AssertStats scrapes (the 0043/0046 sleep-to-settle precedent).
	settleDelay = 750 * time.Millisecond
)

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ CUMULATIVE ARM-ACCOUNTING TABLE (the 0046 discipline) — arms 1-8          │
// ├──────────────────────────────────────────────────────────────────────────┤
// │ Per-prefix counters are CUMULATIVE over all arms sharing a listener. arms  │
// │ 1, 3, 4, 5, 6, 7 share l_default (mongo_a); arm 2 is alone on l_commands   │
// │ (mongo_b); arm 8 is assertion-only (no traffic). Each arm's contribution   │
// │ (✓ = +1) and the resulting cumulative `want` (asserted in AssertStats):    │
// │                                                                            │
// │  mongo_a (l_default)        a1   a3a  a3b  a3c  a4   a5  a6  a7 | cum want  │
// │   query shape:            scat  pk  multi scat  —  scat err err|           │
// │  ──────────────────────── ───  ───  ───  ───  ──  ──── ─── ───| ───────────│
// │  op_query                  ✓    ✓    ✓    ✓    .   ✓   .   .  | 5          │
// │  op_query_scatter_get      ✓    .    .    ✓    .   ✓   .   .  | 3          │
// │  op_query_multi_get        .    .    ✓    .    .   .   .   .  | 1          │
// │  op_query_no_max_time      ✓    ✓    ✓    ✓    .   ✓   .   .  | 5          │
// │  op_query_tailable_cursor  .    .    .    ✓    .   .   .   .  | 1 (0x02)   │
// │  op_query_no_cursor_timeout.    .    .    ✓    .   .   .   .  | 1 (0x10)   │
// │  op_query_await_data       .    .    .    ✓    .   .   .   .  | 1 (0x20)   │
// │  op_query_exhaust          .    .    .    ✓    .   .   .   .  | 1 (0x40)   │
// │  op_insert                 .    .    .    .    ✓   .   .   .  | 1 (arm 4)  │
// │  op_get_more               .    .    .    .    ✓   .   .   .  | 1 (arm 4)  │
// │  op_kill_cursors           .    .    .    .    ✓   .   .   .  | 1 (arm 4)  │
// │  op_command                .    .    .    .    ✓   .   .   .  | 1 (arm 4)  │
// │  decoding_error            .    .    .    .    .   .   ✓   ✓  | 2 (a6+a7)  │
// │  collection1.query.total   ✓    ✓    ✓    ✓    .   ✓   .   .  | 5          │
// │  collection1.query.scatter ✓    .    .    ✓    .   ✓   .   .  | 3          │
// │  collection1.query.multi   .    .    ✓    .    .   .   .   .  | 1          │
// │  callsite fixtureFn .total .    .    .    .    .   ✓   .   .  | 1 (AMEND-C3)│
// │  callsite fixtureFn .scat  .    .    .    .    .   ✓   .   .  | 1 (AMEND-C3)│
// │                                                                            │
// │  mongo_b (l_commands)       a2(isMaster)  a2(foo)  | cumulative want       │
// │  ─────────────────────────  ───────────  ───────  | ──────────────────────│
// │  op_query                        ✓           ✓    | 2                     │
// │  op_query_no_max_time            .           .    | 0  ($cmd excluded)    │
// │  cmd.isMaster.total              ✓           .    | 1                     │
// │  cmd.unknown_command.total       .           ✓    | 1                     │
// └──────────────────────────────────────────────────────────────────────────┘
//
// Arm 3c is a no-_id ScatterGet query (so it adds to scatter_get / .query.total
// / .query.scatter_get / op_query_no_max_time) that ALSO carries the four query
// flag bits 0x02|0x10|0x20|0x40 → the four op_query_* flag counters. Arm 4 sends
// no OP_QUERY (only insert/getMore/killCursors/command), so it adds nothing to
// any op_query* / collection* counter. Arm 6 (unsupported opcode 2013) +
// arm 7 (garbage BSON) each contribute +1 to decoding_error on SEPARATE FRESH
// connections (decoding_error is at-most-once per connection lifetime;
// D-S29.1-6) → cumulative 2. Arm 6's follow-up OP_QUERY on the SAME connection
// adds NOTHING (sniffing turned off by the first decode error — AMEND-B6), which
// is why op_query stays at 5, not 6. Arm 8 sends no traffic. The above `want`
// values are the GROUND TRUTH reasoned per §11.2 and re-verified live cross-side.

// driveProxy runs the arms-1-7 workload (SPEC §8.1.3; arm 8 is assertion-only,
// arm 9 is the recorded R4 deliberate-break) against both sides
// identically and returns a side-independent verdict byte stream. The "side"
// label is accepted for diagnostic logging only and is NEVER written to the
// returned bytes, so equivalent behavior yields byte-identical drive output for
// the runner's CompareBytes gate. The arms run in declared order over the shared
// listeners, so AssertStats asserts CUMULATIVE counter values (the table above).
func (d *mongoRequestsDriver) driveProxy(ctx context.Context, addrs map[string]string, side string) ([]byte, error) {
	def, ok := addrs["l_default"]
	if !ok {
		return nil, fmt.Errorf("%s: missing addr for listener l_default", fixtureName)
	}
	cmds, ok := addrs["l_commands"]
	if !ok {
		return nil, fmt.Errorf("%s: missing addr for listener l_commands", fixtureName)
	}

	var b bytes.Buffer

	// Arm 1 (plain scatter-get query): OP_QUERY db.collection1 {a:1} (no _id, no
	// maxTimeMS) on a fresh l_default connection.
	n, err := driveFrames(ctx, def, 0, [][]byte{
		opQuery(1, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)),
	})
	emitArm(&b, side, "plain-query", n, err)

	// Arm 2 (commands-list semantics): two FRESH l_commands connections — (i)
	// {isMaster:1} → cmd.isMaster.total (in the list); (ii) {foo:1} →
	// cmd.unknown_command.total (not in the list). Both are $cmd queries → both
	// bump op_query but NEITHER bumps op_query_no_max_time.
	n, err = driveFrames(ctx, cmds, 0, [][]byte{
		opQuery(2, adminCmd, 0, bsonDoc(bsonInt32("isMaster", 1)...)),
	})
	emitArm(&b, side, "cmd-isMaster", n, err)
	n, err = driveFrames(ctx, cmds, 0, [][]byte{
		opQuery(3, adminCmd, 0, bsonDoc(bsonInt32("foo", 1)...)),
	})
	emitArm(&b, side, "cmd-unknown", n, err)

	// Arm 3 (query-shape variants) on l_default, one connection, paced writes:
	//   3a {_id: 7}            → PrimaryKey (only .query.total)
	//   3b {_id: {x:1}}        → MultiGet   (op_query_multi_get + .query.multi_get)
	//   3c {a: 1} w/ all flags → ScatterGet + the four op_query_* flag counters
	flagBits := int32(0x02 | 0x10 | 0x20 | 0x40)
	n, err = driveFrames(ctx, def, interWriteDelay, [][]byte{
		opQuery(4, dbColl, 0, bsonDoc(bsonInt32("_id", 7)...)),
		opQuery(5, dbColl, 0, bsonDoc(bsonDocElem("_id", bsonDoc(bsonInt32("x", 1)...))...)),
		opQuery(6, dbColl, flagBits, bsonDoc(bsonInt32("a", 1)...)),
	})
	emitArm(&b, side, "query-shapes", n, err)

	// Arm 4 (other request opcodes) on l_default, one connection, paced writes:
	// OP_INSERT / OP_GET_MORE / OP_KILL_CURSORS / OP_COMMAND — each +1 to its own
	// op_* counter; NONE bumps op_query or any collection counter.
	n, err = driveFrames(ctx, def, interWriteDelay, [][]byte{
		opInsert(7, dbColl, bsonDoc(bsonInt32("a", 1)...)),
		opGetMore(8, dbColl, 0),
		opKillCursors(9, 1),
		opCommand(10, "db", "ping"),
	})
	emitArm(&b, side, "other-opcodes", n, err)

	// Arm 5 ($comment callsite double-count) on a fresh l_default connection:
	// OP_QUERY db.collection1 {a:1, $comment:"{\"callingFunction\":\"fixtureFn\"}"}
	// → both collection1.query.* AND collection1.callsite.fixtureFn.query.*
	// (AMEND-C3). The query has no _id → ScatterGet shape (so .scatter_get too).
	comment := `{"callingFunction": "fixtureFn"}`
	commentElems := append(bsonInt32("a", 1), bsonString("$comment", comment)...)
	n, err = driveFrames(ctx, def, 0, [][]byte{
		opQuery(11, dbColl, 0, bsonDoc(commentElems...)),
	})
	emitArm(&b, side, "callsite", n, err)

	// Arm 6 (unsupported-opcode + sniffing-off proof) on a FRESH l_default
	// connection: an OP_MSG (2013) frame is an UNSUPPORTED opcode → decoding_error
	// +1 (at-most-once per connection lifetime; D-S29.1-6). A follow-up VALID
	// OP_QUERY on the SAME connection increments NOTHING because the first decode
	// error turned sniffing OFF for the connection lifetime (AMEND-B6). The proof
	// is in AssertStats: mongo_a op_query stays at 5 (NOT 6) and decoding_error
	// gains +1. The bytes still pass through to the backend because mongo_proxy
	// always returns Continue (R3) and the tcp_proxy terminal forwards them — this
	// passthrough is a structural property of the chain, not separately asserted
	// here; the cross-side signal is the stat parity below.
	n, err = driveFrames(ctx, def, interWriteDelay, [][]byte{
		opMsgFrame(12),
		opQuery(13, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)),
	})
	emitArm(&b, side, "unsupported-opcode", n, err)

	// Arm 7 (garbage-BSON) on a FRESH l_default connection: a well-FRAMED OP_QUERY
	// whose query document carries an element with a BAD type byte 0x13 (not a
	// valid BSON element type) → the BSON walker fails → decoding_error +1 (a
	// SEPARATE connection from arm 6, so a second independent +1). The frame is
	// length-correct (mongoMsg/opQuery frame it) but the inner element type is
	// garbage, so the codec's BSON parse throws.
	badElem := append(append([]byte{0x13}, cstr("a")...), leI32(1)...) // type 0x13 = invalid
	n, err = driveFrames(ctx, def, 0, [][]byte{
		opQuery(14, dbColl, 0, bsonDoc(badElem...)),
	})
	emitArm(&b, side, "garbage-bson", n, err)

	// Arm 8 is assertion-only (exists-at-zero + gauge-TYPE + cx_destroy presence);
	// it sends no traffic — see AssertStats. All connections opened by arms 1-7 are
	// closed (driveFrames closes each), so the response-side counters and the
	// op_query_active gauge are observed in their at-rest (==0) state.

	// Let the async stat pipeline settle before the runner scrapes in AssertStats.
	if err := sleepCtx(ctx, settleDelay); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// emitArm writes a side-independent verdict line for one arm. The side label is
// logged to stderr (diagnostic) but never to the returned byte stream.
func emitArm(b *bytes.Buffer, side, name string, sent int, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "[fixture 0049 %s] arm %s: %v\n", side, name, err)
		fmt.Fprintf(b, "arm %s sent=%d verdict=ERR\n", name, sent)
		return
	}
	fmt.Fprintf(b, "arm %s sent=%d verdict=ok\n", name, sent)
}

// driveFrames opens a fresh TCP connection to addr, writes each frame as a
// separate Write (with interDelay between writes when > 0), then closes the
// connection. Returns the number of frames written and any error.
func driveFrames(ctx context.Context, addr string, interDelay time.Duration, frames [][]byte) (int, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("dial %s: %w", addr, err)
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

// AssertStats will scrape /stats/prometheus from BOTH admin endpoints and assert
// the per-opcode / per-command / per-collection mongo counters after the
// nine-arm workload (SPEC §8.1 + §7.4 label table). The runner invokes this ONCE
// with both admin addresses; the scrape-and-diff for both sides happens in-band
// here.
//
// Counters are looked up by the CANONICAL Prometheus form `name{k1="v1",…}`
// (labels sorted by key — scrapeMongoStats/canonicalize) so a value lookup
// intrinsically asserts BOTH name-parity AND label-extraction parity (R7). An
// ABSENT key (name/label-shape failure) is reported DISTINCTLY from a
// present-but-wrong value (the 0046 presence-flag discipline). The expected
// values are CUMULATIVE over the in-order arms sharing a listener (the
// arm-accounting table above driveProxy is authoritative).
func (d *mongoRequestsDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeMongoStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref mongo stats: %v", err)
	}
	subjStats, err := scrapeMongoStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj mongo stats: %v", err)
	}

	if os.Getenv("FIXTURE_0049_DUMP_STATS") != "" {
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
	// CUMULATIVE per-prefix value from the arm-accounting table above driveProxy.
	expectations := []struct {
		key  string
		want int64
	}{
		// --- mongo_a (l_default): arms 1, 3, 4, 5 ---
		// op_query family (fixed stats carry only the prefix label):
		{`envoy_mongo_op_query{envoy_mongo_prefix="mongo_a"}`, 5},
		{`envoy_mongo_op_query_scatter_get{envoy_mongo_prefix="mongo_a"}`, 3},
		{`envoy_mongo_op_query_multi_get{envoy_mongo_prefix="mongo_a"}`, 1},
		{`envoy_mongo_op_query_no_max_time{envoy_mongo_prefix="mongo_a"}`, 5},
		// arm 3c query flag counters (one query bearing all four bits):
		{`envoy_mongo_op_query_tailable_cursor{envoy_mongo_prefix="mongo_a"}`, 1},
		{`envoy_mongo_op_query_no_cursor_timeout{envoy_mongo_prefix="mongo_a"}`, 1},
		{`envoy_mongo_op_query_await_data{envoy_mongo_prefix="mongo_a"}`, 1},
		{`envoy_mongo_op_query_exhaust{envoy_mongo_prefix="mongo_a"}`, 1},
		// arm 4 other request opcodes:
		{`envoy_mongo_op_insert{envoy_mongo_prefix="mongo_a"}`, 1},
		{`envoy_mongo_op_get_more{envoy_mongo_prefix="mongo_a"}`, 1},
		{`envoy_mongo_op_kill_cursors{envoy_mongo_prefix="mongo_a"}`, 1},
		{`envoy_mongo_op_command{envoy_mongo_prefix="mongo_a"}`, 1},
		// arms 6+7 decoding errors (one per FRESH connection; at-most-once each):
		{`envoy_mongo_decoding_error{envoy_mongo_prefix="mongo_a"}`, 2},
		// per-collection query family (2 labels: collection + prefix):
		{`envoy_mongo_collection_query_total{envoy_mongo_collection="collection1",envoy_mongo_prefix="mongo_a"}`, 5},
		{`envoy_mongo_collection_query_scatter_get{envoy_mongo_collection="collection1",envoy_mongo_prefix="mongo_a"}`, 3},
		{`envoy_mongo_collection_query_multi_get{envoy_mongo_collection="collection1",envoy_mongo_prefix="mongo_a"}`, 1},
		// arm 5 callsite double-count (3 labels: callsite + collection + prefix):
		{`envoy_mongo_collection_callsite_query_total{envoy_mongo_callsite="fixtureFn",envoy_mongo_collection="collection1",envoy_mongo_prefix="mongo_a"}`, 1},
		{`envoy_mongo_collection_callsite_query_scatter_get{envoy_mongo_callsite="fixtureFn",envoy_mongo_collection="collection1",envoy_mongo_prefix="mongo_a"}`, 1},

		// --- mongo_b (l_commands): arm 2 ---
		{`envoy_mongo_op_query{envoy_mongo_prefix="mongo_b"}`, 2},
		{`envoy_mongo_op_query_no_max_time{envoy_mongo_prefix="mongo_b"}`, 0}, // $cmd excluded
		{`envoy_mongo_cmd_total{envoy_mongo_cmd="isMaster",envoy_mongo_prefix="mongo_b"}`, 1},
		{`envoy_mongo_cmd_total{envoy_mongo_cmd="unknown_command",envoy_mongo_prefix="mongo_b"}`, 1},
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

	// --- Arm 8 (exists-at-zero + gauge-TYPE + cx_destroy presence) ---
	//
	// The 29.1 scope is request-only, so the response-side counters and the
	// delay/drain counters are NEVER incremented; they must nonetheless be
	// PRESENT (eagerly created at config parse; D-P1) and == 0 on BOTH sides.
	// This is the creation-parity proof that the full ALL_MONGO_PROXY_STATS
	// roster is materialized at boot (the zookeeper exists-at-zero precedent).
	// Both prefixes are checked (mongo_a on l_default, mongo_b on l_commands).
	existsAtZero := []string{
		"op_reply", "op_reply_cursor_not_found", "op_reply_query_failure",
		"op_reply_valid_cursor", "op_command_reply", "delays_injected",
		"cx_drain_close",
	}
	for _, sd := range []struct {
		label string
		stats map[string]int64
	}{{"ref", refStats}, {"subj", subjStats}} {
		for _, prefix := range []string{statPrefixDefault, statPrefixCommands} {
			for _, suf := range existsAtZero {
				key := fmt.Sprintf("envoy_mongo_%s{envoy_mongo_prefix=%q}", suf, prefix)
				got, present := sd.stats[canonicalize(key)]
				if !present {
					t.Errorf("%s: response-side counter %s ABSENT (exists-at-zero creation failure)", sd.label, key)
					continue
				}
				if got != 0 {
					t.Errorf("%s: %s = %d, want 0 (request-only scope — must not increment)", sd.label, key, got)
				}
			}

			// The op_query_active gauge: PRESENT with a `# TYPE … gauge` line and
			// value == 0 (no in-flight query at rest — increments land at 29.2).
			// The raw `# TYPE` line is dropped by scrapeMongoStats (it skips `#`
			// lines), so scrapeTypeLine re-scrapes it directly to assert the metric
			// TYPE is `gauge` (D-P1 gauge-vs-counter parity).
			gaugeKey := fmt.Sprintf("envoy_mongo_op_query_active{envoy_mongo_prefix=%q}", prefix)
			got, present := sd.stats[canonicalize(gaugeKey)]
			if !present {
				t.Errorf("%s: gauge %s ABSENT (creation failure)", sd.label, gaugeKey)
			} else if got != 0 {
				t.Errorf("%s: %s = %d, want 0 (no in-flight query at rest)", sd.label, gaugeKey, got)
			}

			// cx_destroy_local/remote_with_active_rq: PRESENT both sides, value NOT
			// compared (AMEND-C2 — the reference increments these on every
			// query-bearing connection close; envoy-go's increment lands at 29.2, so
			// at 29.1 the cross-side VALUES legitimately differ and only PRESENCE is
			// asserted here).
			for _, suf := range []string{"cx_destroy_local_with_active_rq", "cx_destroy_remote_with_active_rq"} {
				key := fmt.Sprintf("envoy_mongo_%s{envoy_mongo_prefix=%q}", suf, prefix)
				if _, present := sd.stats[canonicalize(key)]; !present {
					t.Errorf("%s: counter %s ABSENT (exists-at-zero creation failure)", sd.label, key)
				}
			}
		}
	}

	// The gauge TYPE line is keyed by the BARE metric name (the prefix lives in a
	// label on the value line, not on the `# TYPE` line). One TYPE line covers all
	// prefixes of a metric family, so it is asserted once per side.
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
// Retains only envoy_mongo_* lines. This is the label-aware generalization of the
// 0043 single-label / 0046 flat mechanics (§8.1.2): the canonical key folds the
// metric NAME and the LABEL SET together, so a value lookup intrinsically asserts
// BOTH name-parity AND label-extraction parity (R7).
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
		// split "name{labels} value" — the value is the last space-separated field.
		sp := strings.LastIndexByte(line, ' ')
		if sp < 0 {
			continue
		}
		nameLabels, valStr := line[:sp], line[sp+1:]
		v, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			continue // skip non-integer (e.g. gauge float-form — not expected for mongo)
		}
		out[canonicalize(nameLabels)] = v
	}
	return out, nil
}

// scrapeTypeLine issues GET /stats/prometheus and returns the single
// `# TYPE <name> <type>` line for the given metric NAME (the raw `# TYPE` line is
// skipped by scrapeMongoStats, which drops all `#` comment lines). Used by arm 8
// to assert the op_query_active metric is declared `gauge` (not `counter`) on
// both sides — the D-P1 gauge-vs-counter creation-parity proof. Returns an error
// if the TYPE line is absent.
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
// labels sorted by key (so ref/subj label-order differences cannot cause a miss).
//
// PRECONDITION: label VALUES must be comma- and brace-free. canonicalize splits
// the label set on ',' and strips the trailing '}', so a value containing ',' or
// '}' would mis-split into bogus pairs. This holds for mongo's tags, which are
// all identifier-only (prefix/cmd/collection/callsite — no commas or braces). The
// helper is shared with the 29.2 0051 driver, whose tags share this property.
func canonicalize(nameLabels string) string {
	open := strings.IndexByte(nameLabels, '{')
	if open < 0 {
		return nameLabels // no labels
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

// httpGet issues GET url and returns the response body (the 5-line net/http GET
// the 0046 driver uses; the per-driver admin-scrape helper). A non-200 status is
// an error so a scrape against a not-yet-ready admin endpoint fails loudly.
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

// --- bootstrap rendering ---

type bootstrapParams struct {
	adminAddr    string // "<ip>, port_value: <n>" for the admin socket_address
	listenAddr   string // listener bind address (0.0.0.0 for ref; 127.0.0.1 for subj)
	defaultPort  int    // l_default listener port
	commandsPort int    // l_commands listener port
	clusterType  string // STRICT_DNS (ref) | STATIC (subj)
	dnsLine      string // "      dns_lookup_family: V4_ONLY\n" for STRICT_DNS, else ""
	backendHost  string
	backendPort  int
	nodeLine     string // "node: {...}\n" for subj, "" for ref
}

// mongoProxyType is the mongo_proxy typed_config @type URL. The network-filter
// type URLs carry the extensions. segment (memory
// reference_network_filter_typeurl_extensions); the proto FQN is
// envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy.
const mongoProxyType = "type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy"
const tcpProxyType = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// renderBootstrap assembles the full two-listener bootstrap. Each listener's
// filter chain is [mongo_proxy, tcp_proxy] — the shallow-read + terminal chain.
// c_sink is the tcp_proxy upstream (the runner's TCPSink backend) AND the
// boot-satisfying cluster (a zero-cluster boot is rejected by both sides).
func renderBootstrap(p bootstrapParams) string {
	defaultListener := fmt.Sprintf(`    - name: l_default
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
                stat_prefix: tcp_default
                cluster: c_sink
`, p.listenAddr, p.defaultPort, mongoProxyType, statPrefixDefault, tcpProxyType)

	commandsListener := fmt.Sprintf(`    - name: l_commands
      address:
        socket_address: { address: %s, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.mongo_proxy
              typed_config:
                "@type": %s
                stat_prefix: %s
                commands: ["isMaster"]
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": %s
                stat_prefix: tcp_commands
                cluster: c_sink
`, p.listenAddr, p.commandsPort, mongoProxyType, statPrefixCommands, tcpProxyType)

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
		defaultListener,
		commandsListener,
		p.clusterType,
		p.dnsLine,
		p.backendHost, p.backendPort,
	)
}

// Compile-time interface assertions. The StatsAsserter body is a Task-14 stub
// (the label-aware scrape-and-diff + the nine arms land at Tasks 15-16); the
// assertion confirms the method set is present so the runner dispatches it.
var (
	_ fixture.Driver              = (*mongoRequestsDriver)(nil)
	_ fixture.MultiListenerDriver = (*mongoRequestsDriver)(nil)
	_ fixture.BackendKindAware    = (*mongoRequestsDriver)(nil)
	_ fixture.StatsAsserter       = (*mongoRequestsDriver)(nil)
)
