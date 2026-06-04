package mongoproxy

import (
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

func TestCodec_OpMsgIsDecodingError(t *testing.T) {
	d, ms := newTestDecoder(t)
	d.decodeOnData(msg(1, 2013, nil), int64(len(msg(1, 2013, nil)))) // OP_MSG
	if ms.counters["decoding_error"].Load() != 1 {
		t.Fatalf("decoding_error = %d, want 1", ms.counters["decoding_error"].Load())
	}
	if d.sniffing {
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
	if !d.sniffing {
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
