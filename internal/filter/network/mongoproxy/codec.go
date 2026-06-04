package mongoproxy

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

// Wire opcodes (parent §11.4; codec.h:24-35 + the modern OP_MSG=2013). The
// dispatch decodes EXACTLY 7 (Reply, Query, GetMore, Insert, KillCursors,
// Command, CommandReply); everything else → decoding_error (AMEND-B5).
const (
	opReply        int32 = 1
	opUpdate       int32 = 2001
	opInsert       int32 = 2002
	opQuery        int32 = 2004
	opGetMore      int32 = 2005
	opDelete       int32 = 2006
	opKillCursors  int32 = 2007
	opCommand      int32 = 2010
	opCommandReply int32 = 2011
	opMsg          int32 = 2013
)

// activeQuery carries what the response-side correlation + the dynamic reply-side
// stats need. Written on every decoded OP_QUERY (appendQuery); read+erased on a
// correlated reply (takeQuery) and drained at connection destroy (onDestroy).
type activeQuery struct {
	requestID  int32
	collection string
	command    string    // empty for non-$cmd queries
	callsite   string    // empty unless a $comment callsite was present
	start      time.Time // recorded at 29.1 (cheap; avoids a 29.2 struct revision)
}

// decoder is the per-connection request-side wire decoder. It owns its OWN
// readBuf (the chain Buffer is read, NEVER drained — R3). chainConsumed is the
// high-water mark against Buffer.TotalAppended() (D-S29.1-4; the
// zookeeperproxy/decoder.go:40-48,99-104 mechanism adapted verbatim). The
// active-query list lives HERE (not on the filter — PLAN disposition; the
// zookeeper requestsByXid-on-the-decoder precedent), so the codec is unit-testable
// in isolation. The ADR-0223 per-connection mutex (mu, below) guards the
// active-query list against the cross-goroutine OnWrite (pump B) reader.
type decoder struct {
	cfg           *compiledConfig
	stats         *mongoStats
	chainConsumed int64
	readBuf       []byte
	sniffing      atomic.Bool // starts true; CAS true→false on the first decode error (connection lifetime, direction-shared).
	queries       []activeQuery
	// 29.2 cross-goroutine state:
	mu       sync.Mutex          // guards EXACTLY queries (append/take/drain). ADR-0223 — narrowed to one list.
	writeBuf []byte              // goroutine-B-only response reassembly buffer (no high-water mark — 28.2 asymmetry).
	dynMeta  map[string][]string // this-pass collection→ops accumulator (emit_dynamic_metadata; written by recordOp, read by filter.emitDynamicMetadata).
}

// appendQuery records a decoded OP_QUERY under mu and Incs the op_query_active
// gauge (the list-size↔gauge invariant; §3.4). The append + Inc are co-located
// under the request-path lock; the atomic Inc is lock-free but kept here so the
// invariant holds at every quiesced point. The pre-handoff request path takes
// the lock too (uniformity over cleverness — ADR-0223 §Decision item 3).
func (d *decoder) appendQuery(aq activeQuery) {
	d.mu.Lock()
	d.queries = append(d.queries, aq)
	d.mu.Unlock()
	d.stats.opQueryActive.Inc()
}

// takeQuery removes + returns (by value) the FIRST active query whose requestID
// matches the reply's responseTo (upstream first-match-erase; parent §11.4 item
// 7). Holds mu for the scan + erase ONLY; the returned copy (incl. start, for
// the 29.3-deferred latency) is used outside the lock. ok=false → uncorrelated.
func (d *decoder) takeQuery(responseTo int32) (activeQuery, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.queries {
		if d.queries[i].requestID == responseTo {
			aq := d.queries[i]
			d.queries = append(d.queries[:i], d.queries[i+1:]...)
			return aq, true
		}
	}
	return activeQuery{}, false
}

// onDestroy drains the residual active-query list at connection teardown and
// Decs op_query_active once per still-live entry so the gauge returns to 0 when
// the connection ends (mirrors upstream's ActiveQuery destructor for every live
// entry). The list is cleared under mu; the gauge math runs OUTSIDE the lock
// (snapshot-count-then-Add; D-S29.2-5). Idempotent — a second call drains 0.
func (d *decoder) onDestroy() {
	d.mu.Lock()
	n := len(d.queries)
	d.queries = nil
	d.mu.Unlock()
	if n > 0 {
		d.stats.opQueryActive.Add(int64(-n))
	}
}

// recordOp accumulates this request pass's collection→op for emit_dynamic_metadata
// (gated; goroutine-A-only — the request decode is sequential, no lock). The map
// is reset at the top of each decodeOnData pass (per-pass clear; §3.7).
func (d *decoder) recordOp(collection, op string) {
	if !d.cfg.emitDynamicMetadata {
		return
	}
	if d.dynMeta == nil {
		d.dynMeta = map[string][]string{}
	}
	d.dynMeta[collection] = append(d.dynMeta[collection], op)
}

// newDecoder returns a fresh per-connection decoder (sniffing on).
func newDecoder(cfg *compiledConfig, ms *mongoStats) *decoder {
	d := &decoder{cfg: cfg, stats: ms}
	d.sniffing.Store(true)
	return d
}

// decodeOnData feeds the chain-buffer's NEW bytes (the trailing
// totalAppended−chainConsumed bytes) into readBuf and decodes every complete
// message. Once sniffing is off it only advances chainConsumed and drops bytes
// (AMEND-B6). It NEVER drains the chain buffer, never closes, never halts (R3).
func (d *decoder) decodeOnData(chainBytes []byte, totalAppended int64) {
	if !d.sniffing.Load() {
		d.chainConsumed = totalAppended
		d.readBuf = nil
		return
	}
	if d.cfg.emitDynamicMetadata { // per-pass clear (filter.emitDynamicMetadata reads it; harmless when nil)
		d.dynMeta = nil
	}
	if newCount := totalAppended - d.chainConsumed; newCount > 0 {
		d.readBuf = append(d.readBuf, chainBytes[int64(len(chainBytes))-newCount:]...)
		d.chainConsumed = totalAppended
	}
	for {
		m, ok := d.nextMessage()
		if !ok {
			break // no complete frame buffered (or sniffing went off mid-loop)
		}
		if !d.decodeMessage(m) {
			break // decoding_error path already ran; sniffing now off
		}
	}
	if !d.sniffing.Load() {
		d.readBuf = nil // release after a decode error (direction-local)
	}
}

// nextMessage extracts one complete wire message from readBuf (header + body;
// messageLength INCLUDES the 16-byte header). Returns ok=false on a partial
// frame (wait for more — never an error). A malformed length (< 16) → decode error.
func (d *decoder) nextMessage() ([]byte, bool) {
	if len(d.readBuf) < 16 {
		return nil, false
	}
	msgLen := int32(binary.LittleEndian.Uint32(d.readBuf[0:4]))
	if msgLen < 16 {
		d.decoderError()
		return nil, false
	}
	if int64(len(d.readBuf)) < int64(msgLen) {
		return nil, false // partial frame — wait for more bytes
	}
	m := d.readBuf[:msgLen]
	d.readBuf = d.readBuf[msgLen:]
	return m, true
}

// decodeMessage parses the MsgHeader and dispatches by opcode (AMEND-B5).
// Returns false on a decode failure (the decoding_error path has already run).
func (d *decoder) decodeMessage(m []byte) bool {
	requestID := int32(binary.LittleEndian.Uint32(m[4:8]))
	opCode := int32(binary.LittleEndian.Uint32(m[12:16]))
	body := m[16:]
	switch opCode {
	case opQuery:
		return d.decodeQuery(requestID, body)
	case opInsert:
		return d.decodeInsert(body)
	case opGetMore:
		return d.decodeGetMore(body)
	case opKillCursors:
		return d.decodeKillCursors(body)
	case opCommand:
		return d.decodeCommand(body)
	case opReply, opCommandReply:
		// recognized-not-decoded at 29.1 (§1.2): valid envelope → NOT an error;
		// the frame is consumed; body decode + counters land at 29.2.
		return true
	default:
		// Msg(1000)/Update(2001)/Delete(2006)/OP_MSG(2013)/anything else → throw
		// (upstream "invalid mongo op N" parity).
		d.decoderError()
		return false
	}
}

// decoderError charges decoding_error AT MOST ONCE per connection and turns
// sniffing off for the connection lifetime (direction-shared; D-S29.1-6 /
// AMEND-B6). The CompareAndSwap makes the charge exactly-once even if BOTH pumps
// hit a decode error simultaneously (29.2 — sniffing is now cross-goroutine).
// Buffer release is direction-local (each loop nils its OWN buffer when sniffing
// goes off), so goroutine B never writes goroutine A's readBuf.
func (d *decoder) decoderError() {
	if d.sniffing.CompareAndSwap(true, false) {
		d.stats.inc("decoding_error")
	}
}

// fail is the codec's error shorthand: take the decoding_error path, return false.
func (d *decoder) fail() bool { d.decoderError(); return false }

// decodeQuery decodes an OP_QUERY body (parent §11.4): flags(int32) →
// fullCollectionName(cstring) → numberToSkip(int32) → numberToReturn(int32) →
// query(BSON doc) → OPTIONAL returnFieldsSelector(BSON doc, iff bytes remain).
func (d *decoder) decodeQuery(requestID int32, body []byte) bool {
	r := &bsonReader{buf: body}
	flags, err := r.readInt32()
	if err != nil {
		return d.fail()
	}
	fullColl, err := r.readCString()
	if err != nil {
		return d.fail()
	}
	if _, err := r.readInt32(); err != nil { // numberToSkip
		return d.fail()
	}
	if _, err := r.readInt32(); err != nil { // numberToReturn
		return d.fail()
	}
	queryDoc, err := parseDocument(r)
	if err != nil {
		return d.fail()
	}
	if r.remaining() > 0 { // optional returnFieldsSelector — parse to validate
		if _, err := parseDocument(r); err != nil {
			return d.fail()
		}
	}

	d.stats.inc("op_query")
	if flags&0x02 != 0 {
		d.stats.inc("op_query_tailable_cursor")
	}
	if flags&0x10 != 0 {
		d.stats.inc("op_query_no_cursor_timeout")
	}
	if flags&0x20 != 0 {
		d.stats.inc("op_query_await_data")
	}
	if flags&0x40 != 0 {
		d.stats.inc("op_query_exhaust")
	}

	dot := strings.IndexByte(fullColl, '.')
	if dot < 0 {
		return d.fail() // "invalid full collection name" parity
	}
	collection := fullColl[dot+1:]
	aq := activeQuery{requestID: requestID, collection: collection, start: time.Now()}

	if strings.Contains(fullColl, "$cmd") {
		cmdDoc := queryDoc
		if q, ok := queryDoc.find("$query"); ok {
			if nested, ok := q.val.(bsonDoc); ok {
				cmdDoc = nested
			}
		}
		first, ok := cmdDoc.first()
		if !ok {
			return d.fail() // empty $cmd doc → "invalid query command" parity
		}
		name := normalizeCommand(first.name)
		if name != "" {
			if !d.cfg.commands[name] {
				name = "unknown_command"
			}
			// Guard: the dynamic cmd stat name is config-derived (normalizeCommand
			// of the wire command, or a configured remembered name). NewCounterIfAbsent
			// PANICS (stats: invalid metric name) on any name failing the registry
			// charset; the guard makes the decoder panic-safe on adversarial command
			// bytes by skipping the dynamic increment. The FIXED active-query append +
			// return ALWAYS run. Differential parity unaffected (fixtures use valid names).
			if stats.IsValidName(name) {
				d.stats.cmdTotal(name).Inc()
			}
			aq.command = name
			d.recordOp(collection, "query")
			d.appendQuery(aq)
			return true
		}
		// name == "" → "find": route to the query path on the command document
		// (AMEND-B7; upstream utility.cc routes find to collection stats). The
		// exact find-collection extraction is an IMPL upstream-transcription
		// detail (no 29.1 fixture exercises find); the query-shape path below is
		// the faithful default.
		queryDoc = cmdDoc
	}

	// non-command query path
	leaves, opShape := queryShape(queryDoc)
	// Guard: collection (and the $comment callsite cs below) are WIRE-derived and
	// flow into NewCounterIfAbsent, which PANICS on any name failing the registry
	// charset (hyphen, space, non-ASCII, empty, trailing dot). The guard makes the
	// decoder panic-safe on adversarial collection/callsite bytes by skipping the
	// dynamic increment. The FIXED opShape + op_query_no_max_time increments and
	// the active-query append ALWAYS run. Differential parity unaffected (fixtures
	// use valid names like collection1).
	collValid := stats.IsValidName(collection)
	if collValid {
		for _, leaf := range leaves {
			d.stats.collectionQuery(collection, leaf).Inc()
		}
	}
	if opShape != "" {
		d.stats.inc(opShape)
	}
	if maxTimeLessThanOne(queryDoc) {
		d.stats.inc("op_query_no_max_time")
	}
	if cs := callsiteName(queryDoc); cs != "" {
		aq.callsite = cs
		if collValid && stats.IsValidName(cs) {
			for _, leaf := range leaves {
				d.stats.callsiteQuery(collection, cs, leaf).Inc()
			}
		}
	}
	d.recordOp(collection, "query")
	d.appendQuery(aq)
	return true
}

// queryShape classifies a non-command query (parent §11.3): no _id → ScatterGet;
// Document/Array _id → MultiGet; scalar _id → PrimaryKey. Returns the collection
// leaf set (always incl. "total") + the op_query_* shape counter ("" for
// PrimaryKey). The callsite family double-counts the SAME leaves (AMEND-C3).
func queryShape(queryDoc bsonDoc) (leaves []string, opCounter string) {
	leaves = []string{"total"}
	idElem, hasID := queryDoc.find("_id")
	switch {
	case !hasID:
		return append(leaves, "scatter_get"), "op_query_scatter_get"
	case idElem.typ == 0x03 || idElem.typ == 0x04: // Document or Array
		return append(leaves, "multi_get"), "op_query_multi_get"
	default:
		return leaves, "" // scalar _id → PrimaryKey: only total
	}
}

// maxTimeLessThanOne returns true when the query's $maxTimeMS (fallback
// maxTimeMS; Int32/Int64/Double) is < 1 — including absent (defaults to 0).
// Non-command queries with maxTime < 1 → op_query_no_max_time (§11.2).
func maxTimeLessThanOne(queryDoc bsonDoc) bool {
	for _, key := range []string{"$maxTimeMS", "maxTimeMS"} {
		if e, ok := queryDoc.find(key); ok {
			if v, ok := asInt64(e.val); ok {
				return v < 1
			}
		}
	}
	return true // absent → 0 → < 1
}

// callsiteName extracts the $comment callsite (AMEND-C3): $comment (String)
// parsed as JSON → field "callingFunction". Any parse failure → "" (no callsite).
func callsiteName(queryDoc bsonDoc) string {
	e, ok := queryDoc.find("$comment")
	if !ok {
		return ""
	}
	s, ok := e.val.(string)
	if !ok {
		return ""
	}
	var parsed struct {
		CallingFunction string `json:"callingFunction"`
	}
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return ""
	}
	return parsed.CallingFunction
}

// decodeInsert: flags(int32) → fullCollectionName(cstring) → 1..N BSON docs
// (loop to end of body). Validate-and-consume; op_insert.
func (d *decoder) decodeInsert(body []byte) bool {
	r := &bsonReader{buf: body}
	if _, err := r.readInt32(); err != nil { // flags
		return d.fail()
	}
	fullColl, err := r.readCString() // fullCollectionName
	if err != nil {
		return d.fail()
	}
	for r.remaining() > 0 {
		if _, err := parseDocument(r); err != nil {
			return d.fail()
		}
	}
	d.stats.inc("op_insert")
	if dot := strings.IndexByte(fullColl, '.'); dot >= 0 {
		d.recordOp(fullColl[dot+1:], "insert")
	}
	return true
}

// decodeGetMore: ZERO(int32) → fullCollectionName(cstring) → numberToReturn(int32)
// → cursorID(int64). op_get_more.
func (d *decoder) decodeGetMore(body []byte) bool {
	r := &bsonReader{buf: body}
	if _, err := r.readInt32(); err != nil { // ZERO
		return d.fail()
	}
	if _, err := r.readCString(); err != nil { // fullCollectionName
		return d.fail()
	}
	if _, err := r.readInt32(); err != nil { // numberToReturn
		return d.fail()
	}
	if _, err := r.readInt64(); err != nil { // cursorID
		return d.fail()
	}
	d.stats.inc("op_get_more")
	return true
}

// decodeKillCursors: ZERO(int32) → numberOfCursorIDs(int32) → cursorIDs(int64…).
// op_kill_cursors.
func (d *decoder) decodeKillCursors(body []byte) bool {
	r := &bsonReader{buf: body}
	if _, err := r.readInt32(); err != nil { // ZERO
		return d.fail()
	}
	n, err := r.readInt32()
	if err != nil {
		return d.fail()
	}
	for i := int32(0); i < n; i++ {
		if _, err := r.readInt64(); err != nil {
			return d.fail()
		}
	}
	d.stats.inc("op_kill_cursors")
	return true
}

// decodeCommand: database(cstring) → commandName(cstring) → metadata(BSON) →
// commandArgs(BSON) → 0..N inputDocs(BSON, loop). op_command.
func (d *decoder) decodeCommand(body []byte) bool {
	r := &bsonReader{buf: body}
	if _, err := r.readCString(); err != nil { // database
		return d.fail()
	}
	if _, err := r.readCString(); err != nil { // commandName
		return d.fail()
	}
	if _, err := parseDocument(r); err != nil { // metadata
		return d.fail()
	}
	if _, err := parseDocument(r); err != nil { // commandArgs
		return d.fail()
	}
	for r.remaining() > 0 { // inputDocs
		if _, err := parseDocument(r); err != nil {
			return d.fail()
		}
	}
	d.stats.inc("op_command")
	return true
}

// decodeOnWrite feeds the response decoder the write-direction (upstream→
// downstream) bytes. writeChainConn.Write allocates a FRESH *Buffer per Write
// (ADR-0221), so p arrives exactly once — appended directly to writeBuf with NO
// high-water mark (the 28.2 structural asymmetry; ADR-0225). NEVER drains the
// chain buffer, never closes, never halts (R3 extended to the write side).
func (d *decoder) decodeOnWrite(p []byte) {
	if !d.sniffing.Load() {
		d.writeBuf = nil
		return
	}
	d.writeBuf = append(d.writeBuf, p...)
	for {
		m, ok := d.nextWriteMessage()
		if !ok {
			break
		}
		if !d.decodeResponseMessage(m) {
			break
		}
	}
	if !d.sniffing.Load() {
		d.writeBuf = nil
	}
}

// nextWriteMessage extracts one complete wire message from writeBuf (the
// nextMessage shape over the write-side buffer). Partial frame → wait; a
// messageLength < 16 → decode error.
func (d *decoder) nextWriteMessage() ([]byte, bool) {
	if len(d.writeBuf) < 16 {
		return nil, false
	}
	msgLen := int32(binary.LittleEndian.Uint32(d.writeBuf[0:4]))
	if msgLen < 16 {
		d.decoderError()
		return nil, false
	}
	if int64(len(d.writeBuf)) < int64(msgLen) {
		return nil, false // partial frame — wait for more bytes
	}
	m := d.writeBuf[:msgLen]
	d.writeBuf = d.writeBuf[msgLen:]
	return m, true
}

// decodeResponseMessage parses the MsgHeader of a response frame and dispatches
// by opcode. responseTo (m[8:12]) drives correlation (§3.3). Only OP_REPLY(1) and
// OP_COMMANDREPLY(2011) are valid on the response stream; any other opcode is a
// decoding_error (the request side's default-throw parity). Returns false on a
// decode failure (the decoding_error path has already run).
func (d *decoder) decodeResponseMessage(m []byte) bool {
	responseTo := int32(binary.LittleEndian.Uint32(m[8:12]))
	opCode := int32(binary.LittleEndian.Uint32(m[12:16]))
	body := m[16:]
	switch opCode {
	case opReply:
		return d.decodeReply(responseTo, body)
	case opCommandReply:
		return d.decodeCommandReply(body)
	default:
		d.decoderError()
		return false
	}
}

// decodeReply decodes an OP_REPLY body (parent §11.4): responseFlags(int32) +
// cursorID(int64) + startingFrom(int32) + numberReturned(int32) + exactly
// numberReturned BSON documents. Charges op_reply + the flag/cursor counters; a
// malformed body → decoding_error. On success, a requestID↔responseTo correlation
// hit (takeQuery) Decs the op_query_active gauge outside the lock (§3.3).
func (d *decoder) decodeReply(responseTo int32, body []byte) bool {
	r := &bsonReader{buf: body}
	flags, err := r.readInt32()
	if err != nil {
		return d.fail()
	}
	cursorID, err := r.readInt64()
	if err != nil {
		return d.fail()
	}
	if _, err := r.readInt32(); err != nil { // startingFrom
		return d.fail()
	}
	numReturned, err := r.readInt32()
	if err != nil {
		return d.fail()
	}
	for i := int32(0); i < numReturned; i++ {
		if _, err := parseDocument(r); err != nil {
			return d.fail()
		}
	}
	d.stats.inc("op_reply")
	if flags&0x01 != 0 {
		d.stats.inc("op_reply_cursor_not_found")
	}
	if flags&0x02 != 0 {
		d.stats.inc("op_reply_query_failure")
	}
	if cursorID != 0 {
		d.stats.inc("op_reply_valid_cursor")
	}
	// Correlation (§3.3): a first-match-erase hit Decs the gauge OUTSIDE the lock
	// (the entry is already copied out). A miss leaves the gauge untouched —
	// uncorrelated replies (no pending query) charge only the fixed op_reply*
	// counters above. The copied entry's start time.Time rides for 29.3 latency.
	if _, ok := d.takeQuery(responseTo); ok {
		d.stats.opQueryActive.Dec()
	}
	return true
}

// decodeCommandReply decodes an OP_COMMANDREPLY body (parent §11.4):
// metadata(BSON) + commandReply(BSON) + 0..N outputDocs(BSON, loop to end of
// body). Charges op_command_reply. Does NOT correlate + does NOT touch the gauge
// (only OP_REPLY correlates against the active-query list — parent §11.4 item 7).
func (d *decoder) decodeCommandReply(body []byte) bool {
	r := &bsonReader{buf: body}
	if _, err := parseDocument(r); err != nil { // metadata
		return d.fail()
	}
	if _, err := parseDocument(r); err != nil { // commandReply
		return d.fail()
	}
	for r.remaining() > 0 { // outputDocs
		if _, err := parseDocument(r); err != nil {
			return d.fail()
		}
	}
	d.stats.inc("op_command_reply")
	return true
}
