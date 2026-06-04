package mongoproxy

import (
	"sync"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// newTestDecoder wires a decoder over a fresh roster (stat_prefix "p").
func newTestDecoder(t *testing.T, commands ...string) (*decoder, *mongoStats) {
	t.Helper()
	reg := stats.NewRegistry()
	cfg := &compiledConfig{statPrefix: "p", commands: map[string]bool{}}
	if len(commands) == 0 {
		commands = defaultCommands
	}
	for _, c := range commands {
		cfg.commands[c] = true
	}
	ms := newMongoStats(reg, "p")
	// NOTE: the PLAN Task-7 helper writes `cfg.stats = ms` here, but the
	// compiledConfig.stats field is DEFERRED to Task 10's NewFactory (config.go
	// carries the deferral note; Task 7 must not touch config.go). The decoder
	// receives the roster via the newDecoder ms param, so the line is dead —
	// dropped to keep Task 7 within its file scope. (Controller: re-add at Task
	// 10 once compiledConfig.stats lands, if desired.)
	return newDecoder(cfg, ms), ms
}

// msg builds a full mongo wire message: 16-byte LE header + body. messageLength
// includes the header. (The 0049 driver builders at Task 14 generalize this.)
func msg(reqID, opCode int32, body []byte) []byte {
	total := int32(16 + len(body))
	out := append(leI32(total), leI32(reqID)...)
	out = append(out, leI32(0)...)      // responseTo
	out = append(out, leI32(opCode)...) // opCode
	return append(out, body...)
}

// respMsg builds a response wire message with an EXPLICIT responseTo (the msg()
// helper hardcodes responseTo=0; the response path correlates on it).
func respMsg(reqID, responseTo, opCode int32, body []byte) []byte {
	total := int32(16 + len(body))
	out := append(leI32(total), leI32(reqID)...)
	out = append(out, leI32(responseTo)...)
	out = append(out, leI32(opCode)...)
	return append(out, body...)
}

// opReplyBody: responseFlags(int32) + cursorID(int64) + startingFrom(int32) +
// numberReturned(int32) + numberReturned BSON docs.
func opReplyBody(flags int32, cursorID int64, docs ...[]byte) []byte {
	out := append(leI32(flags), leI64(cursorID)...)
	out = append(out, leI32(0)...)                // startingFrom
	out = append(out, leI32(int32(len(docs)))...) // numberReturned
	for _, dc := range docs {
		out = append(out, dc...)
	}
	return out
}

func TestDecodeReply_Counters(t *testing.T) {
	cases := []struct {
		name     string
		flags    int32
		cursorID int64
		ndocs    int
		want     map[string]uint64
	}{
		{"plain-empty", 0, 0, 0, map[string]uint64{"op_reply": 1}},
		{"cursor-not-found", 0x01, 0, 0, map[string]uint64{"op_reply": 1, "op_reply_cursor_not_found": 1}},
		{"query-failure", 0x02, 0, 0, map[string]uint64{"op_reply": 1, "op_reply_query_failure": 1}},
		{"valid-cursor", 0, 42, 1, map[string]uint64{"op_reply": 1, "op_reply_valid_cursor": 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, ms := newTestDecoder(t)
			docs := make([][]byte, tc.ndocs)
			for i := range docs {
				docs[i] = simpleQuery()
			}
			d.decodeOnWrite(respMsg(7, 0, 1, opReplyBody(tc.flags, tc.cursorID, docs...)))
			for suf, want := range tc.want {
				if got := ms.counters[suf].Load(); got != want {
					t.Errorf("%s = %d, want %d", suf, got, want)
				}
			}
		})
	}
}

func TestDecodeReply_MalformedBodyIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	// numberReturned claims 1 doc, but the body carries a truncated BSON doc.
	body := append(leI32(0), leI64(0)...) // flags + cursorID
	body = append(body, leI32(0)...)      // startingFrom
	body = append(body, leI32(1)...)      // numberReturned = 1
	body = append(body, leI32(99)...)     // a doc claiming 99 bytes, none follow
	d.decodeOnWrite(respMsg(7, 0, 1, body))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a malformed OP_REPLY doc must be a decoding_error")
	}
	// The charge-after-successful-decode ordering: a malformed reply must NOT
	// charge op_reply (this would catch a partial-charge regression where the
	// inc is wrongly placed before the doc-walk loop).
	if ms.counters["op_reply"].Load() != 0 {
		t.Errorf("a malformed OP_REPLY must NOT charge op_reply; got %d", ms.counters["op_reply"].Load())
	}
}

func TestCorrelation_FirstMatchEraseDecsGauge(t *testing.T) {
	d, ms := newTestDecoder(t)
	// Two OP_QUERYs (requestIDs 11, 12) → gauge 2, list len 2.
	q1 := msg(11, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	q2 := msg(12, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	both := append(q1, q2...)
	d.decodeOnData(both, int64(len(both)))
	if ms.opQueryActive.Load() != 2 || len(d.queries) != 2 {
		t.Fatalf("setup: gauge=%d len=%d, want 2/2", ms.opQueryActive.Load(), len(d.queries))
	}
	// A reply with responseTo=11 correlates the first query → erase + gauge Dec.
	d.decodeOnWrite(respMsg(99, 11, 1, opReplyBody(0, 0)))
	if ms.opQueryActive.Load() != 1 {
		t.Errorf("gauge = %d after one correlated reply, want 1", ms.opQueryActive.Load())
	}
	if len(d.queries) != 1 || d.queries[0].requestID != 12 {
		t.Errorf("first-match-erase failed: %+v", d.queries)
	}
}

func TestCorrelation_UncorrelatedMissChargesFixedOnly(t *testing.T) {
	d, ms := newTestDecoder(t)
	q1 := msg(11, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	d.decodeOnData(q1, int64(len(q1)))
	// A reply whose responseTo (777) matches NO pending query: op_reply +1, gauge UNCHANGED.
	d.decodeOnWrite(respMsg(99, 777, 1, opReplyBody(0, 0)))
	if ms.counters["op_reply"].Load() != 1 {
		t.Errorf("op_reply must still fire for an uncorrelated reply")
	}
	if ms.opQueryActive.Load() != 1 {
		t.Errorf("an uncorrelated reply must NOT change the gauge (still 1 in-flight query)")
	}
	if len(d.queries) != 1 {
		t.Errorf("an uncorrelated reply must not erase any entry")
	}
}

func TestCorrelation_CommandReplyDoesNotCorrelate(t *testing.T) {
	d, ms := newTestDecoder(t)
	q1 := msg(11, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	d.decodeOnData(q1, int64(len(q1)))
	// An OP_COMMANDREPLY echoing responseTo=11 must NOT erase the OP_QUERY entry
	// (only OP_REPLY correlates — parent §11.4 item 7).
	d.decodeOnWrite(respMsg(99, 11, 2011, opCommandReplyBody()))
	if ms.opQueryActive.Load() != 1 || len(d.queries) != 1 {
		t.Errorf("OP_COMMANDREPLY must not correlate against the active-query list")
	}
}

func opCommandReplyBody(outputDocs ...[]byte) []byte {
	out := append(bsonDocEmpty(), bsonDocEmpty()...) // metadata + commandReply (both empty docs)
	for _, dc := range outputDocs {
		out = append(out, dc...)
	}
	return out
}

// bsonDocEmpty is a 5-byte empty BSON document {len=5}{0x00}.
func bsonDocEmpty() []byte { return doc() }

func TestDecodeCommandReply_Counter(t *testing.T) {
	d, ms := newTestDecoder(t)
	d.decodeOnWrite(respMsg(7, 0, 2011, opCommandReplyBody()))
	if ms.counters["op_command_reply"].Load() != 1 {
		t.Errorf("op_command_reply = %d, want 1", ms.counters["op_command_reply"].Load())
	}
	// OP_COMMANDREPLY does NOT touch the gauge (no correlation).
	if ms.opQueryActive.Load() != 0 {
		t.Errorf("OP_COMMANDREPLY must not touch the gauge")
	}
}

func TestDecodeCommandReply_MalformedIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	body := append(leI32(99), make([]byte, 4)...) // metadata claims 99 bytes, none follow
	d.decodeOnWrite(respMsg(7, 0, 2011, body))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a malformed OP_COMMANDREPLY must be a decoding_error")
	}
	// Charge-after-successful-decode: a malformed command reply must NOT charge
	// op_command_reply (guards against a pre-loop partial-charge regression).
	if ms.counters["op_command_reply"].Load() != 0 {
		t.Errorf("a malformed OP_COMMANDREPLY must NOT charge op_command_reply; got %d", ms.counters["op_command_reply"].Load())
	}
}

func TestDecodeOnWrite_PartialFrameReassembly(t *testing.T) {
	d, ms := newTestDecoder(t)
	full := respMsg(7, 1, 1, opReplyBody(0, 0)) // a minimal empty OP_REPLY (responseTo 1)
	// Feed the first 10 bytes (partial header) — nothing decoded.
	d.decodeOnWrite(full[:10])
	if ms.counters["op_reply"].Load() != 0 {
		t.Fatalf("op_reply fired on a partial write frame")
	}
	// Feed the rest (cumulative is NOT used on the write side — fresh per-Write
	// buffers; feed only the remaining bytes).
	d.decodeOnWrite(full[10:])
	if ms.counters["op_reply"].Load() != 1 {
		t.Errorf("op_reply = %d after full write frame, want 1", ms.counters["op_reply"].Load())
	}
}

func TestDecodeOnWrite_ShortMessageLengthIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	bad := append(leI32(8), make([]byte, 12)...) // messageLength 8 < 16
	d.decodeOnWrite(bad)
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a messageLength < 16 on the write side must be a decoding_error")
	}
}

func TestDecodeOnWrite_UnexpectedOpcodeIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	// A request opcode (OP_QUERY 2004) on the RESPONSE stream is malformed.
	d.decodeOnWrite(respMsg(1, 0, 2004, nil))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a non-reply opcode on the write side must be a decoding_error")
	}
}

func TestDecoder_SniffingOffIsDirectionShared(t *testing.T) {
	// An error on the READ side turns sniffing off for the connection; a
	// subsequent WRITE-side frame then decodes NOTHING (AMEND-B6 direction-shared).
	d, ms := newTestDecoder(t)
	d.decodeOnData(msg(1, 2013, nil), int64(len(msg(1, 2013, nil)))) // OP_MSG → error
	if ms.counters["decoding_error"].Load() != 1 || d.sniffing.Load() {
		t.Fatalf("read-side error did not turn sniffing off")
	}
	d.decodeOnWrite(respMsg(7, 1, 1, opReplyBody(0, 0))) // a valid reply, but sniffing is off
	if ms.counters["op_reply"].Load() != 0 {
		t.Errorf("op_reply must stay 0 — sniffing is off for the connection (direction-shared)")
	}
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("decoding_error must stay 1 (at-most-once across both directions)")
	}
}

func TestCodec_OpMsgIsDecodingError(t *testing.T) {
	d, ms := newTestDecoder(t)
	d.decodeOnData(msg(1, 2013, nil), int64(len(msg(1, 2013, nil)))) // OP_MSG
	if ms.counters["decoding_error"].Load() != 1 {
		t.Fatalf("decoding_error = %d, want 1", ms.counters["decoding_error"].Load())
	}
	if d.sniffing.Load() {
		t.Errorf("sniffing must be false after a decode error")
	}
}

func TestCodec_DecodingErrorAtMostOncePerConnection(t *testing.T) {
	d, ms := newTestDecoder(t)
	// First an OP_MSG (error), then a valid-looking second frame on the SAME
	// connection: sniffing is off → the second frame is dropped, NO 2nd error.
	frame1 := msg(1, 2013, nil)
	frame2 := msg(2, 2013, nil)
	both := append(frame1, frame2...)
	d.decodeOnData(both, int64(len(both)))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("decoding_error = %d, want exactly 1 (AMEND-B6 / D-S29.1-6)", ms.counters["decoding_error"].Load())
	}
}

func TestCodec_ReplyAndCommandReplyRecognizedNotDecoded(t *testing.T) {
	d, ms := newTestDecoder(t)
	// Feed BOTH frames with the correct cumulative totalAppended so they actually
	// route through decodeMessage (a totalAppended of 0 would append nothing to
	// readBuf and the test would pass vacuously). Reply(1) + CommandReply(2011).
	full := append(msg(1, 1, []byte{0x01, 0x02, 0x03}), msg(2, 2011, []byte{0x04, 0x05})...)
	d.decodeOnData(full, int64(len(full)))
	// recognized-not-decoded: NO decoding_error, sniffing stays on, no counters,
	// and both frames are fully consumed from readBuf (proving dispatch ran).
	if ms.counters["decoding_error"].Load() != 0 {
		t.Errorf("Reply/CommandReply must not error; got %d", ms.counters["decoding_error"].Load())
	}
	if !d.sniffing.Load() {
		t.Errorf("sniffing must stay on after recognized-not-decoded opcodes")
	}
	if len(d.readBuf) != 0 {
		t.Errorf("both recognized frames must be consumed; readBuf has %d bytes left", len(d.readBuf))
	}
}

func TestCodec_PartialFrameReassembly(t *testing.T) {
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("db.c", 0, simpleQuery())) // a valid OP_QUERY
	// Feed the first 10 bytes (partial header) — nothing decoded yet.
	d.decodeOnData(full[:10], 10)
	if ms.counters["op_query"].Load() != 0 {
		t.Fatalf("op_query fired on a partial frame")
	}
	// Feed the rest — now the complete frame decodes exactly once.
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_query"].Load() != 1 {
		t.Errorf("op_query = %d after full frame, want 1", ms.counters["op_query"].Load())
	}
}

func TestCodec_MultiReadNoDoubleCount(t *testing.T) {
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("db.c", 0, simpleQuery()))
	// Two OnData calls with the CUMULATIVE chain slice (the 28.1b re-base feed):
	// totalAppended advances; only the new tail bytes are appended to readBuf.
	d.decodeOnData(full, int64(len(full)))
	d.decodeOnData(full, int64(len(full))) // same totalAppended → no new bytes
	if ms.counters["op_query"].Load() != 1 {
		t.Errorf("op_query = %d, want 1 (no double-count across reads)", ms.counters["op_query"].Load())
	}
}

func TestDecoder_GaugeIncsPerActiveQuery(t *testing.T) {
	// Each decoded OP_QUERY appends to the active-query list AND Incs the gauge;
	// the list-size↔gauge invariant holds on the request-only path (§3.4).
	d, ms := newTestDecoder(t)
	f1 := msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	f2 := msg(2, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	both := append(f1, f2...)
	d.decodeOnData(both, int64(len(both)))
	if got := ms.opQueryActive.Load(); got != 2 {
		t.Errorf("op_query_active = %d, want 2 (one Inc per active query)", got)
	}
	if len(d.queries) != 2 {
		t.Errorf("active-query list = %d, want 2 (gauge must track list size)", len(d.queries))
	}
}

func TestOnDestroy_DrainsResidualGauge(t *testing.T) {
	// Two never-answered queries → gauge 2; onDestroy drains the residual list and
	// Decs the gauge per entry → gauge 0 (the connection-close teardown, §3.4).
	d, ms := newTestDecoder(t)
	q := append(
		msg(11, 2004, opQueryBody("db.collection1", 0, simpleQuery())),
		msg(12, 2004, opQueryBody("db.collection1", 0, simpleQuery()))...,
	)
	d.decodeOnData(q, int64(len(q)))
	if ms.opQueryActive.Load() != 2 {
		t.Fatalf("setup: gauge=%d, want 2", ms.opQueryActive.Load())
	}
	d.onDestroy()
	if ms.opQueryActive.Load() != 0 {
		t.Errorf("gauge = %d after onDestroy, want 0 (residual drain Dec)", ms.opQueryActive.Load())
	}
	if len(d.queries) != 0 {
		t.Errorf("onDestroy must clear the residual list")
	}
}

func TestGaugeLifecycle_Invariant(t *testing.T) {
	// inc(2) → dec(1 correlated) → destroy(drains 1) → 0. The list-size↔gauge
	// invariant holds at each step.
	d, ms := newTestDecoder(t)
	q := append(
		msg(11, 2004, opQueryBody("db.c1", 0, simpleQuery())),
		msg(12, 2004, opQueryBody("db.c1", 0, simpleQuery()))...,
	)
	d.decodeOnData(q, int64(len(q)))
	d.decodeOnWrite(respMsg(99, 11, 1, opReplyBody(0, 0))) // answer query 11
	if ms.opQueryActive.Load() != int64(len(d.queries)) || ms.opQueryActive.Load() != 1 {
		t.Fatalf("after one answer: gauge=%d len=%d, want 1/1", ms.opQueryActive.Load(), len(d.queries))
	}
	d.onDestroy()
	if ms.opQueryActive.Load() != 0 {
		t.Errorf("gauge = %d at end of connection, want 0", ms.opQueryActive.Load())
	}
}

func simpleQuery() []byte {
	return doc(append(append([]byte{0x10}, cstr("a")...), leI32(1)...)...)
}

func opQueryBody(fullColl string, flags int32, queryDoc []byte) []byte {
	out := append(leI32(flags), cstr(fullColl)...)
	out = append(out, leI32(0)...) // numberToSkip
	out = append(out, leI32(0)...) // numberToReturn
	return append(out, queryDoc...)
}

// helpers building richer query docs
func queryWithID(idType byte, idVal []byte) []byte {
	return doc(append(append([]byte{idType}, cstr("_id")...), idVal...)...)
}

func TestDecodeQuery_PlainScatterGet(t *testing.T) {
	// arm 1: db.collection1 {a:1}, no _id, no maxTime.
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	d.decodeOnData(full, int64(len(full)))
	for _, suf := range []string{"op_query", "op_query_scatter_get", "op_query_no_max_time"} {
		if ms.counters[suf].Load() != 1 {
			t.Errorf("%s = %d, want 1", suf, ms.counters[suf].Load())
		}
	}
	if ms.collectionQuery("collection1", "total").Load() != 1 {
		t.Errorf("collection1.query.total != 1")
	}
	if ms.collectionQuery("collection1", "scatter_get").Load() != 1 {
		t.Errorf("collection1.query.scatter_get != 1")
	}
	if len(d.queries) != 1 || d.queries[0].collection != "collection1" {
		t.Errorf("active-query list not populated: %+v", d.queries)
	}
}

func TestDecodeQuery_PrimaryKeyScalarID(t *testing.T) {
	// arm 3a: {_id: 7} scalar → only query.total (no scatter/multi).
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("db.collection1", 0, queryWithID(0x10, leI32(7))))
	d.decodeOnData(full, int64(len(full)))
	if ms.collectionQuery("collection1", "total").Load() != 1 {
		t.Errorf("query.total != 1")
	}
	if ms.counters["op_query_scatter_get"].Load() != 0 || ms.counters["op_query_multi_get"].Load() != 0 {
		t.Errorf("scalar _id must be PrimaryKey (no scatter/multi)")
	}
}

func TestDecodeQuery_MultiGetDocumentID(t *testing.T) {
	// arm 3b: {_id: {x:1}} Document-typed → MultiGet.
	d, ms := newTestDecoder(t)
	inner := doc(append(append([]byte{0x10}, cstr("x")...), leI32(1)...)...)
	full := msg(1, 2004, opQueryBody("db.collection1", 0, queryWithID(0x03, inner)))
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_query_multi_get"].Load() != 1 || ms.collectionQuery("collection1", "multi_get").Load() != 1 {
		t.Errorf("Document-typed _id must be MultiGet")
	}
}

func TestDecodeQuery_FlagCounters(t *testing.T) {
	// arm 3c: flags 0x02|0x10|0x20|0x40 → the four flag counters.
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("db.c", 0x02|0x10|0x20|0x40, simpleQuery()))
	d.decodeOnData(full, int64(len(full)))
	for _, suf := range []string{"op_query_tailable_cursor", "op_query_no_cursor_timeout", "op_query_await_data", "op_query_exhaust"} {
		if ms.counters[suf].Load() != 1 {
			t.Errorf("%s = %d, want 1", suf, ms.counters[suf].Load())
		}
	}
}

func TestDecodeQuery_CmdIsMasterAndUnknown(t *testing.T) {
	// arm 2: commands:[isMaster]. {isMaster:1} → cmd.isMaster.total; {foo:1} → unknown.
	d, ms := newTestDecoder(t, "isMaster")
	cmdDoc := func(name string) []byte { return doc(append(append([]byte{0x10}, cstr(name)...), leI32(1)...)...) }
	f1 := msg(1, 2004, opQueryBody("admin.$cmd", 0, cmdDoc("isMaster")))
	f2 := msg(2, 2004, opQueryBody("admin.$cmd", 0, cmdDoc("foo")))
	both := append(f1, f2...)
	d.decodeOnData(both, int64(len(both)))
	if ms.cmdTotal("isMaster").Load() != 1 {
		t.Errorf("cmd.isMaster.total != 1")
	}
	if ms.cmdTotal("unknown_command").Load() != 1 {
		t.Errorf("cmd.unknown_command.total != 1")
	}
	if ms.counters["op_query"].Load() != 2 {
		t.Errorf("op_query = %d, want 2", ms.counters["op_query"].Load())
	}
	if ms.counters["op_query_no_max_time"].Load() != 0 {
		t.Errorf("$cmd queries must NOT increment op_query_no_max_time (§11.2)")
	}
}

func TestDecodeQuery_CallsiteDoubleCount(t *testing.T) {
	// arm 5: {a:1, $comment:"{\"callingFunction\":\"fixtureFn\"}"} → callsite + plain.
	d, ms := newTestDecoder(t)
	var q []byte
	q = append(q, 0x10)
	q = append(q, cstr("a")...)
	q = append(q, leI32(1)...)
	q = append(q, 0x02)
	q = append(q, cstr("$comment")...)
	q = append(q, bstr(`{"callingFunction": "fixtureFn"}`)...)
	full := msg(1, 2004, opQueryBody("db.collection1", 0, doc(q...)))
	d.decodeOnData(full, int64(len(full)))
	if ms.collectionQuery("collection1", "total").Load() != 1 {
		t.Errorf("plain collection.query.total != 1")
	}
	if ms.callsiteQuery("collection1", "fixtureFn", "total").Load() != 1 {
		t.Errorf("callsite query.total != 1 (AMEND-C3 double-count)")
	}
	if ms.callsiteQuery("collection1", "fixtureFn", "scatter_get").Load() != 1 {
		t.Errorf("callsite query.scatter_get != 1")
	}
	if d.queries[0].callsite != "fixtureFn" {
		t.Errorf("active-query callsite = %q, want fixtureFn", d.queries[0].callsite)
	}
}

func TestDecodeQuery_NoDotCollectionIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("nodotcollection", 0, simpleQuery()))
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a fullCollectionName with no dot must be a decoding_error")
	}
}

func TestDecodeQuery_EmptyCmdDocIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("admin.$cmd", 0, doc())) // empty doc
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("an empty $cmd document must be a decoding_error")
	}
}

func TestDecodeQuery_InvalidCollectionNameSkipsDynamicNoPanic(t *testing.T) {
	// A collection segment with an out-of-charset byte ("bad-name", hyphen) would
	// panic NewCounterIfAbsent. The guard must skip the dynamic stat WITHOUT panic;
	// fixed counters + the active-query list still record the query.
	d, ms := newTestDecoder(t)
	full := msg(1, 2004, opQueryBody("db.bad-name", 0, simpleQuery()))
	d.decodeOnData(full, int64(len(full))) // must NOT panic
	if ms.counters["op_query"].Load() != 1 {
		t.Errorf("op_query = %d, want 1 (fixed counters fire regardless of collection nameability)", ms.counters["op_query"].Load())
	}
	if ms.counters["op_query_scatter_get"].Load() != 1 {
		t.Errorf("op_query_scatter_get must still fire for an un-nameable collection")
	}
	if ms.counters["decoding_error"].Load() != 0 {
		t.Errorf("an un-nameable collection is NOT a decode error; got %d", ms.counters["decoding_error"].Load())
	}
	if len(d.queries) != 1 || d.queries[0].collection != "bad-name" {
		t.Errorf("active-query list must still record the raw collection: %+v", d.queries)
	}
}

func TestDecodeQuery_InvalidCommandNameSkipsCmdStatNoPanic(t *testing.T) {
	// A configured command name with an out-of-charset byte ("bad-cmd", hyphen)
	// reaches cmdTotal→NewCounterIfAbsent, which would panic on the invalid name.
	// The guard must skip the cmd stat WITHOUT panic; op_query still fires and the
	// active-query list still records the raw command. (Mirrors the collection
	// guard above for the $cmd path — the third wire/config-derived dynamic name.)
	d, ms := newTestDecoder(t, "bad-cmd")
	cmdDoc := doc(append(append([]byte{0x10}, cstr("bad-cmd")...), leI32(1)...)...)
	full := msg(1, 2004, opQueryBody("admin.$cmd", 0, cmdDoc))
	d.decodeOnData(full, int64(len(full))) // must NOT panic
	if ms.counters["op_query"].Load() != 1 {
		t.Errorf("op_query = %d, want 1 (fixed counter fires regardless of command nameability)", ms.counters["op_query"].Load())
	}
	if ms.counters["decoding_error"].Load() != 0 {
		t.Errorf("an un-nameable command is NOT a decode error; got %d", ms.counters["decoding_error"].Load())
	}
	if len(d.queries) != 1 || d.queries[0].command != "bad-cmd" {
		t.Errorf("active-query list must still record the raw command: %+v", d.queries)
	}
}

func TestDecodeInsert(t *testing.T) {
	d, ms := newTestDecoder(t)
	// flags(int32) + fullCollectionName(cstring) + 1 BSON doc
	body := append(leI32(0), cstr("db.c")...)
	body = append(body, simpleQuery()...)
	full := msg(1, 2002, body)
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_insert"].Load() != 1 {
		t.Errorf("op_insert != 1")
	}
	if len(d.queries) != 0 {
		t.Errorf("OP_INSERT must NOT append to the active-query list")
	}
}

func TestDecodeGetMore(t *testing.T) {
	d, ms := newTestDecoder(t)
	// ZERO(int32) + fullCollectionName(cstring) + numberToReturn(int32) + cursorID(int64)
	body := append(leI32(0), cstr("db.c")...)
	body = append(body, leI32(10)...)
	body = append(body, leI64(12345)...)
	full := msg(1, 2005, body)
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_get_more"].Load() != 1 {
		t.Errorf("op_get_more != 1")
	}
}

func TestDecodeKillCursors(t *testing.T) {
	d, ms := newTestDecoder(t)
	// ZERO(int32) + numberOfCursorIDs(int32) + cursorIDs(int64 each)
	body := append(leI32(0), leI32(2)...)
	body = append(body, leI64(1)...)
	body = append(body, leI64(2)...)
	full := msg(1, 2007, body)
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_kill_cursors"].Load() != 1 {
		t.Errorf("op_kill_cursors != 1")
	}
}

func TestDecodeCommand(t *testing.T) {
	d, ms := newTestDecoder(t)
	// database(cstring) + commandName(cstring) + metadata(BSON) + commandArgs(BSON)
	body := append(cstr("admin"), cstr("ping")...)
	body = append(body, doc()...)         // metadata (empty)
	body = append(body, simpleQuery()...) // commandArgs
	full := msg(1, 2010, body)
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["op_command"].Load() != 1 {
		t.Errorf("op_command != 1")
	}
}

func TestDecodeInsert_MalformedIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	// flags + collection but a truncated BSON doc.
	body := append(leI32(0), cstr("db.c")...)
	body = append(body, leI32(99)...) // claims a 99-byte doc, none follows
	full := msg(1, 2002, body)
	d.decodeOnData(full, int64(len(full)))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a malformed OP_INSERT doc must be a decoding_error")
	}
}

func TestDecoderConcurrentRequestResponseRace(t *testing.T) {
	// R9: two goroutines over ONE decoder — A drives decodeOnData with a request
	// stream, B drives decodeOnWrite with the matching response stream. With mu
	// guarding dec.queries this is race-clean; REMOVING mu MUST trip `go test
	// -race`. Run under `-race -count=5`.
	d, ms := newTestDecoder(t)
	const n = 200
	reqs := make([][]byte, n)
	reps := make([][]byte, n)
	for i := 0; i < n; i++ {
		id := int32(i + 1)
		reqs[i] = msg(id, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
		reps[i] = respMsg(int32(10000+i), id, 1, opReplyBody(0, 0))
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var total int64
		for _, r := range reqs {
			total += int64(len(r))
			d.decodeOnData(r, total)
		}
	}()
	go func() {
		defer wg.Done()
		for _, r := range reps {
			d.decodeOnWrite(r)
		}
	}()
	wg.Wait()
	// No assertion on the exact gauge value (the interleaving is nondeterministic —
	// a reply may arrive before its query); the point is race-freedom + no panic.
	// At minimum op_reply counted every fed reply.
	if ms.counters["op_reply"].Load() != uint64(n) {
		t.Errorf("op_reply = %d, want %d", ms.counters["op_reply"].Load(), n)
	}
	if ms.counters["op_query"].Load() != uint64(n) {
		t.Errorf("op_query = %d, want %d (request stream must populate dec.queries)", ms.counters["op_query"].Load(), n)
	}
}
