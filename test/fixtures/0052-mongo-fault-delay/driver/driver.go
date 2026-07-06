// Package driver registers the 0052-mongo-fault-delay cross-side differential
// fixture with the runner per phase 29.3 SPEC §6.1 + IMPL Task 11. It is the
// CROSS-SIDE proof of the whole 29.3 phase: fault-delay injection (pre + post
// handoff), the cx_drain_close reply-completion drain stat, and the
// cx_destroy_* close-direction VALUE parity (D-P4 CLOSED).
//
// ============================================================================
// What this fixture proves (29.3 scope)
// ============================================================================
//
// 29.1 proved the request-side decode; 29.2 the response-side decode + the
// op_query_active gauge. 29.3 adds the async halt/resume seam (fault delay) +
// the close-direction-keyed destroy counters + the drain-close stat:
//
//   - fault delay: a mongo_proxy with delay {fixed_delay: 0.100s, 100%} arms a
//     timer per request message → delays_injected counts the arms. The first
//     delay fires PRE-handoff (in the read loop), the second POST-handoff (in
//     replayRead) on the SAME connection. Both replies still arrive (the
//     passthrough is never broken). TIMING IS NEVER COMPARED — only the
//     delays_injected VALUE + the traffic-completes verdict.
//
//   - cx_destroy_*: an in-flight (unanswered) query at connection close keys
//     cx_destroy_local_with_active_rq (the DRIVER/downstream closed) or
//     cx_destroy_remote_with_active_rq (the RESPONDER/upstream closed). An
//     all-answered close (empty list) increments NEITHER.
//
//   - cx_drain_close (BEST-EFFORT → PRESENCE-DOWNGRADED, D-S29.3-8): the
//     differential drive cannot deterministically reproduce cross-side drain
//     timing (the driver's drive phase has no admin addr; the reply-completion-
//     while-draining window is racy), so this arm is DOWNGRADED to a PRESENCE +
//     exists-at-zero assertion. The load-bearing ratification is the Task-9
//     UNIT value proof (TestFilter_DrainCloseOnEmptyListWhenDraining /
//     TestFilter_NoDrainCloseWhenNotDraining), which is deterministic. See the
//     README + PROGRESS.md for the downgrade record.
//
// # Two listeners, two stat_prefixes
//
//   - mongo_d  (stat_prefix=mongo_d):  delay {fixed_delay: 0.100s, 100%}. The
//     fault-delay arm (delays_injected == 2).
//   - mongo_nd (stat_prefix=mongo_nd): NO delay. The seam-non-perturbation arm
//     (delays_injected == 0 — R1 live equivalence) AND the cx_destroy_* arms
//     (LOCAL / REMOTE / all-answered).
//
// Both listeners' filter chains are [mongo_proxy, tcp_proxy] → the SAME
// TCPMongoResponder backend (it writes correlated OP_REPLY frames whose
// responseTo echoes the request requestID — both sides see identical bytes;
// reference_wire_format_both_sides_see_same_bytes).
//
// # Cross-references
//
//   - phase 29.3 SPEC §6.1 (the cross-side arms) + §3.2/§3.4/§3.5
//   - 29.3 IMPL Task 11 (this file) + D-S29.3-8 (the cx_drain_close downgrade)
//   - fixture-0049-mongo-requests (MultiListenerDriver + bootstrap template)
//   - fixture-0051-mongo-responses (TCPMongoResponder + correlated-reply read)
//   - project memory reference_differential_asserter_dispatch (StatsAsserter is
//     load-bearing for cross-side; SubjectAsserter would be vacuous here)
//   - project memory reference_close_direction_framework_gap (cx_destroy_* keyed
//     by close direction — VALUE parity at 29.3, D-P4 CLOSED)
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
	fixtureName = "0052-mongo-fault-delay"

	refAdminPort = 9901

	// In-container reference Envoy listener ports for the two listeners. Distinct
	// from 0049 (19140/19141) and 0051 (19142) — a free pair in the same family.
	refLDelayedPort = 19143
	refLNoDelayPort = 19144

	// stat_prefix roots for each listener's mongo_proxy config (SPEC §6.1).
	statPrefixDelayed = "mongo_d"
	statPrefixNoDelay = "mongo_nd"
)

func init() {
	fixture.RegisterFixture(fixtureName, &mongoFaultDelayDriver{})
}

// mongoFaultDelayDriver carries no mutable cross-arm state — the matrix is fully
// deterministic (100%-probability delay; correlated replies).
type mongoFaultDelayDriver struct{}

// dMarker* MIRROR the responder's mongoMarker* requestID values (runner_test.go,
// a DIFFERENT package — duplicated here so both sides send byte-identical frames).
// A plain (non-marker) reqID → a plain empty OP_REPLY.
const (
	dMarkerWithhold    int32 = 7777 // responder withholds the reply, keeps conn OPEN (LOCAL-close arm)
	dMarkerRemoteClose int32 = 7006 // responder reads then closes its conn WITHOUT replying (REMOTE-close arm)
)

// --- little-endian mongo wire builders (D-S29.1-3; copied verbatim from the
// 0049/0051 drivers so the fixture is self-contained). The driver only SENDS
// requests; the responder emits the correlated replies. ---

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
// Built on a defensive COPY of elems so a caller's element backing array is never
// mutated by the terminating 0x00 append.
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

// --- fixture.Driver (required) ---

// BackendCount returns 1: a single TCPMongoResponder backend serves both
// listeners (c_resp cluster).
func (*mongoFaultDelayDriver) BackendCount() int { return 1 }

// SubjectListenerName returns the primary listener name (l_delayed). Required by
// the Driver interface; MultiListenerDriver takes precedence at runtime.
func (*mongoFaultDelayDriver) SubjectListenerName() string { return "l_delayed" }

// ReferenceListenerPort returns the primary reference listener port (l_delayed).
func (*mongoFaultDelayDriver) ReferenceListenerPort() int { return refLDelayedPort }

// BackendKind returns TCPMongoResponder: the canned-response backend that writes
// correlated OP_REPLY frames (no new BackendKind — reuse 30).
func (*mongoFaultDelayDriver) BackendKind() fixture.BackendKind { return fixture.TCPMongoResponder }

// ReferenceBootstrap renders the two-listener reference bootstrap. c_resp points
// at host.docker.internal:<backend> (ADR-0010 STRICT_DNS).
func (*mongoFaultDelayDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("0.0.0.0, port_value: %d", refAdminPort),
		listenAddr:  "0.0.0.0",
		delayedPort: refLDelayedPort,
		noDelayPort: refLNoDelayPort,
		clusterType: "STRICT_DNS",
		dnsLine:     "      dns_lookup_family: V4_ONLY\n",
		backendHost: "host.docker.internal",
		backendPort: backendPorts[0],
		nodeLine:    "",
	})
}

// SubjectConfig renders the two-listener subject bootstrap. The two subject
// listeners get consecutive ports starting from subjListenerPort (l_delayed at
// the base, l_nodelay at +1) per the 0049 multi-listener port-offset precedent.
func (*mongoFaultDelayDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("127.0.0.1, port_value: %d", subjAdminPort),
		listenAddr:  "127.0.0.1",
		delayedPort: subjListenerPort,
		noDelayPort: subjListenerPort + 1,
		clusterType: "STATIC",
		dnsLine:     "",
		backendHost: "127.0.0.1",
		backendPort: backendPorts[0],
		nodeLine:    "node: { id: envoy-go-subject-0052, cluster: envoy-go-differential }\n",
	})
}

// --- fixture.MultiListenerDriver ---

func (*mongoFaultDelayDriver) SubjectListenerNames() []string {
	return []string{"l_delayed", "l_nodelay"}
}

func (*mongoFaultDelayDriver) ReferenceListenerPorts() []int {
	return []int{refLDelayedPort, refLNoDelayPort}
}

func (d *mongoFaultDelayDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.DriveReferenceMulti(ctx, map[string]string{"l_delayed": addr})
}

func (d *mongoFaultDelayDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.DriveSubjectMulti(ctx, map[string]string{"l_delayed": addr})
}

func (d *mongoFaultDelayDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "ref")
}

func (d *mongoFaultDelayDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (*mongoFaultDelayDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- scenario driving (SPEC §6.1: the cross-side arms) ---

const (
	// dbColl is the full collection name for the query arms.
	dbColl = "db.collection1"

	// readReplyTimeout bounds a conn.Read for a reply. The delayed-listener arm
	// must allow MORE than the injected 0.100s delay (the reply only flows after
	// the delay releases the held bytes upstream + the responder round-trips); a
	// generous bound keeps the arm robust without ever comparing the timing.
	readReplyTimeout = 2 * time.Second

	// settleDelay lets the async stat pipeline on both sides catch up before
	// AssertStats scrapes (the 0049/0051 sleep-to-settle precedent). It is larger
	// than 0051's because the fault-delay arm holds two ~100ms delays in-flight.
	settleDelay = 1200 * time.Millisecond
)

// ┌──────────────────────────────────────────────────────────────────────────┐
// │ ARM-ACCOUNTING TABLE — per listener, CUMULATIVE                           │
// ├──────────────────────────────────────────────────────────────────────────┤
// │ mongo_d (l_delayed):                                                       │
// │   arm 1: two OP_QUERY on ONE conn, both answered → delays_injected += 2.   │
// │          (delay 1 fires PRE-handoff in the read loop; delay 2 fires        │
// │           POST-handoff in replayRead — same conn.) Both replies received.  │
// │          The conn closes with an EMPTY active-query list → no cx_destroy.  │
// │   ── cumulative ──                                                         │
// │     delays_injected               == 2                                     │
// │     cx_destroy_local_with_active_rq  == 0                                  │
// │     cx_destroy_remote_with_active_rq == 0                                  │
// │     op_query_active (gauge)        == 0 @rest                              │
// │                                                                            │
// │ mongo_nd (l_nodelay):                                                      │
// │   arm 2 (non-perturbation): plain OP_QUERY → reply, answered.              │
// │          delays_injected += 0 (NO delay configured — R1 equivalence).      │
// │   arm 4(i) LOCAL: OP_QUERY(withhold) → no reply, conn OPEN → DRIVER closes │
// │          → cx_destroy_local_with_active_rq += 1.                           │
// │   arm 4(ii) REMOTE: OP_QUERY(remoteClose) → responder closes its conn      │
// │          WITHOUT replying → upstream EOFs first                            │
// │          → cx_destroy_remote_with_active_rq += 1.                          │
// │   arm 4(iii) all-answered: plain OP_QUERY → reply (list empties) → close   │
// │          → NEITHER cx_destroy increments.                                  │
// │   ── cumulative ──                                                         │
// │     delays_injected                  == 0                                  │
// │     cx_destroy_local_with_active_rq  == 1                                  │
// │     cx_destroy_remote_with_active_rq == 1                                  │
// │     op_query_active (gauge)          == 0 @rest                            │
// └──────────────────────────────────────────────────────────────────────────┘
//
// cx_drain_close: PRESENCE-only on BOTH listeners (D-S29.3-8 downgrade — the
// differential drain timing is not deterministically reproducible from the
// driver's drive phase; the load-bearing proof is the Task-9 unit value test).

// driveProxy runs the arms in declared order against the two listeners and
// returns a side-independent verdict byte stream. The "side" label is diagnostic-
// only and is NEVER written to the returned bytes, so equivalent behavior yields
// byte-identical output for the runner's CompareBytes gate. TIMING is never
// recorded into the byte stream (only the reply-arrived verdict).
func (d *mongoFaultDelayDriver) driveProxy(ctx context.Context, addrs map[string]string, side string) ([]byte, error) {
	delayed, ok := addrs["l_delayed"]
	if !ok {
		return nil, fmt.Errorf("%s: missing addr for listener l_delayed", fixtureName)
	}
	nodelay, ok := addrs["l_nodelay"]
	if !ok {
		return nil, fmt.Errorf("%s: missing addr for listener l_nodelay", fixtureName)
	}

	var b bytes.Buffer

	// Arm 1 (fault-delay round-trip; pre + post handoff) on l_delayed, ONE conn:
	// OP_QUERY reqID 1 → read the correlated reply (delay 1 fires PRE-handoff in
	// the read loop); OP_QUERY reqID 2 on the SAME conn → read its reply (delay 2
	// fires POST-handoff in replayRead). delays_injected == 2 BOTH sides; both
	// replies received. The ~100ms delay is invisible to correctness — only the
	// delays_injected VALUE + the both-replies-received verdict are asserted.
	rep1, rep2, err := driveTwoOnOneConn(ctx, delayed,
		opQuery(1, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)),
		opQuery(2, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "fault-delay-rt-1", rep1, err)
	emitArm(&b, side, "fault-delay-rt-2", rep2, err)

	// Arm 2 (seam non-perturbation) on l_nodelay: a plain OP_QUERY → reply round
	// trip. delays_injected == 0 (NO delay configured — R1 live equivalence); the
	// reply is received exactly as on the delayed listener but with no halt.
	rep, err := driveAndReadReply(ctx, nodelay, opQuery(10, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "no-delay-rt", rep, err)

	// Arm 4(i) LOCAL close (on l_nodelay): OP_QUERY(withhold) — the responder
	// withholds the reply but keeps its conn OPEN; the bounded read times out;
	// then the DRIVER closes its conn (downstream/LOCAL EOF first) with the query
	// still in-flight → cx_destroy_local_with_active_rq += 1.
	rep, err = driveAndReadReply(ctx, nodelay, opQuery(dMarkerWithhold, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "local-close-active", rep, err)

	// Arm 4(ii) REMOTE close (on l_nodelay): OP_QUERY(remoteClose) — the responder
	// reads then CLOSES its conn WITHOUT replying (upstream/REMOTE EOF first). The
	// driver's bounded read returns 0 (the backend closed); the query is still
	// in-flight at the upstream close → cx_destroy_remote_with_active_rq += 1.
	rep, err = driveAndReadReply(ctx, nodelay, opQuery(dMarkerRemoteClose, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "remote-close-active", rep, err)

	// Arm 4(iii) all-answered close (on l_nodelay): a plain OP_QUERY → reply (the
	// active-query list empties on the correlated reply), then close → the list is
	// EMPTY at close → NEITHER cx_destroy increments (the control case).
	rep, err = driveAndReadReply(ctx, nodelay, opQuery(11, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))
	emitArm(&b, side, "all-answered-close", rep, err)

	// Arm 3 (cx_drain_close) is PRESENCE-DOWNGRADED (D-S29.3-8): no drive traffic.
	// AssertStats asserts the stat exists at zero on both sides; the load-bearing
	// value proof is the Task-9 unit test. See the README for the downgrade record.

	// Let the async stat pipeline settle before the runner scrapes in AssertStats.
	if err := sleepCtx(ctx, settleDelay); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// emitArm writes a side-independent verdict line for one arm. The side label is
// logged to stderr (diagnostic) but never to the returned byte stream. The
// verdict records only whether a reply was observed (NEVER the timing), which is
// side-independent: the answered arms read a reply on both sides; the withhold /
// remote-close arms read nothing on both sides.
func emitArm(b *bytes.Buffer, side, name string, replyLen int, err error) {
	verdict := "ok"
	if err != nil {
		fmt.Fprintf(os.Stderr, "[fixture 0052 %s] arm %s: %v\n", side, name, err)
		verdict = "ERR"
	} else if replyLen == 0 {
		verdict = "noreply"
	}
	fmt.Fprintf(b, "arm %s verdict=%s\n", name, verdict)
}

// driveTwoOnOneConn opens ONE fresh connection, writes frame1, reads its reply,
// writes frame2 on the SAME conn, reads its reply, then closes. Both reads are
// bounded by readReplyTimeout (generous enough to outlast the injected delay
// WITHOUT comparing it). Returns the two reply lengths. This is the load-bearing
// pre+post-handoff arm: delay 1 fires before the terminal handoff (the read loop)
// and delay 2 fires after it (replayRead) — both on this one connection.
func driveTwoOnOneConn(ctx context.Context, addr string, frame1, frame2 []byte) (int, int, error) {
	dl := net.Dialer{}
	conn, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, 0, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write(frame1); err != nil {
		return 0, 0, fmt.Errorf("write frame1: %w", err)
	}
	n1 := readReply(conn)
	if _, err := conn.Write(frame2); err != nil {
		return n1, 0, fmt.Errorf("write frame2: %w", err)
	}
	n2 := readReply(conn)
	return n1, n2, nil
}

// driveAndReadReply opens a fresh connection, writes frame, reads any reply with
// a bounded deadline, then closes (the close drives onDestroy — the cx_destroy
// path for an in-flight query). Returns the reply length (0 if withheld / the
// backend closed). A read timeout / EOF is NOT an error (it is the expected
// outcome for the withhold + remote-close arms, side-independent on both sides).
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
	return readReply(conn), nil
}

// readReply reads up to one buffer of reply bytes with a bounded deadline. A
// timeout / EOF / any read error folds to the bytes read so far (0 for a withheld
// or backend-closed reply) — never an error, so the verdict stays side-independent.
func readReply(conn net.Conn) int {
	_ = conn.SetReadDeadline(time.Now().Add(readReplyTimeout))
	buf := make([]byte, 4096)
	n, rerr := conn.Read(buf)
	if rerr != nil {
		if ne, ok := rerr.(net.Error); ok && ne.Timeout() {
			return 0 // withheld reply — expected, side-independent
		}
		if rerr == io.EOF {
			return n // backend closed (with or without bytes) — not fatal
		}
		return n // any other read error — record bytes-so-far, no hard fail
	}
	return n
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
// 29.3 fault-delay + close-direction counters after the workload (SPEC §6.1).
// Counters are looked up by the CANONICAL Prometheus form (labels sorted) so a
// value lookup intrinsically asserts BOTH name-parity AND label-extraction parity.
// An ABSENT key is reported DISTINCTLY from a present-but-wrong value (the 0049/
// 0051 discipline). TIMING/duration is NEVER scraped or compared.
func (d *mongoFaultDelayDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeMongoStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref mongo stats: %v", err)
	}
	subjStats, err := scrapeMongoStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj mongo stats: %v", err)
	}

	if os.Getenv("FIXTURE_0052_DUMP_STATS") != "" {
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
		// --- mongo_d (l_delayed): arm 1 (two delayed round-trips on one conn) ---
		{`envoy_mongo_delays_injected{envoy_mongo_prefix="mongo_d"}`, 2},
		{`envoy_mongo_cx_destroy_local_with_active_rq{envoy_mongo_prefix="mongo_d"}`, 0},
		{`envoy_mongo_cx_destroy_remote_with_active_rq{envoy_mongo_prefix="mongo_d"}`, 0},

		// --- mongo_nd (l_nodelay): arms 2 + 4(i/ii/iii) ---
		{`envoy_mongo_delays_injected{envoy_mongo_prefix="mongo_nd"}`, 0},                  // R1: no delay configured
		{`envoy_mongo_cx_destroy_local_with_active_rq{envoy_mongo_prefix="mongo_nd"}`, 1},  // arm 4(i)
		{`envoy_mongo_cx_destroy_remote_with_active_rq{envoy_mongo_prefix="mongo_nd"}`, 1}, // arm 4(ii)
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

		// --- all-quiesced roster: the op_query_active gauge == 0 at rest on BOTH
		// prefixes (the 29.2 gauge re-proven under fault load — every answered query
		// Dec'd it; every in-flight residual drained at its connection close). ---
		for _, prefix := range []string{statPrefixDelayed, statPrefixNoDelay} {
			gaugeKey := fmt.Sprintf("envoy_mongo_op_query_active{envoy_mongo_prefix=%q}", prefix)
			got, present := sd.stats[canonicalize(gaugeKey)]
			if !present {
				t.Errorf("%s: gauge %s ABSENT (creation failure)", sd.label, gaugeKey)
			} else if got != 0 {
				t.Errorf("%s: %s = %d, want 0 (gauge quiesced at rest under fault load)", sd.label, gaugeKey, got)
			}

			// cx_drain_close: PRESENCE + exists-at-zero ONLY (D-S29.3-8 downgrade —
			// the differential drain arm is not deterministically reproducible; the
			// load-bearing VALUE proof is the Task-9 unit test). Asserted == 0 here
			// because no drain is driven in this fixture.
			drainKey := fmt.Sprintf("envoy_mongo_cx_drain_close{envoy_mongo_prefix=%q}", prefix)
			v, ok := sd.stats[canonicalize(drainKey)]
			if !ok {
				t.Errorf("%s: counter %s ABSENT (exists-at-zero creation failure)", sd.label, drainKey)
			} else if v != 0 {
				t.Errorf("%s: %s = %d, want 0 (no drain driven — PRESENCE downgrade)", sd.label, drainKey, v)
			}
		}
	}

	// The gauge TYPE line is keyed by the BARE metric name. Asserted once per side.
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
// Retains only envoy_mongo_* lines. (Copied from the 0049/0051 drivers.)
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
// `# TYPE <name> <type>` line for the given metric NAME. (Copied from 0049/0051.)
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
// labels sorted by key. (Copied from 0049/0051; mongo's tags are identifier-only.)
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

// httpGet issues GET url and returns the response body. (Copied from 0049/0051.)
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

// --- bootstrap rendering (two listeners: l_delayed with a delay block, l_nodelay
// without — modeled on the 0049 two-listener render) ---

type bootstrapParams struct {
	adminAddr   string // "<ip>, port_value: <n>" for the admin socket_address
	listenAddr  string // listener bind address (0.0.0.0 for ref; 127.0.0.1 for subj)
	delayedPort int    // l_delayed listener port
	noDelayPort int    // l_nodelay listener port
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

// renderBootstrap assembles the two-listener bootstrap. l_delayed carries a
// delay {fixed_delay: 0.100s, 100%} block on its mongo_proxy; l_nodelay carries
// none. Both chains are [mongo_proxy, tcp_proxy] → c_resp (the TCPMongoResponder
// backend AND the boot-satisfying cluster — a zero-cluster boot is rejected).
func renderBootstrap(p bootstrapParams) string {
	delayedListener := fmt.Sprintf(`    - name: l_delayed
      address:
        socket_address: { address: %s, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.mongo_proxy
              typed_config:
                "@type": %s
                stat_prefix: %s
                delay:
                  fixed_delay: 0.100s
                  percentage: { numerator: 100, denominator: HUNDRED }
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": %s
                stat_prefix: tcp_delayed
                cluster: c_resp
`, p.listenAddr, p.delayedPort, mongoProxyType, statPrefixDelayed, tcpProxyType)

	noDelayListener := fmt.Sprintf(`    - name: l_nodelay
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
                stat_prefix: tcp_nodelay
                cluster: c_resp
`, p.listenAddr, p.noDelayPort, mongoProxyType, statPrefixNoDelay, tcpProxyType)

	return fmt.Sprintf(`%sadmin:
  address:
    socket_address: { address: %s }
static_resources:
  listeners:
%s%s  clusters:
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
		delayedListener,
		noDelayListener,
		p.clusterType,
		p.dnsLine,
		p.backendHost, p.backendPort,
	)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver              = (*mongoFaultDelayDriver)(nil)
	_ fixture.MultiListenerDriver = (*mongoFaultDelayDriver)(nil)
	_ fixture.BackendKindAware    = (*mongoFaultDelayDriver)(nil)
	_ fixture.StatsAsserter       = (*mongoFaultDelayDriver)(nil)
)
