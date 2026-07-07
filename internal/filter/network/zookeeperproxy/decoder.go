package zookeeperproxy

import (
	"encoding/binary"
	"sync"
	"time"

	"github.com/pgdad/envoy-go/internal/filter/network"
)

// Special xids (upstream XidCodes; AMEND-A5). The decoder dispatches per-packet
// by xid sniffing — there is no first-packet state machine.
const (
	connectXid    int32 = 0
	watchXid      int32 = -1 // response-side only (28.2); listed for completeness
	pingXid       int32 = -2
	authXid       int32 = -4
	setWatchesXid int32 = -8
)

// pendingRequest is a correlation-structure entry (AMEND-A7): written at 28.1
// on every successful request decode; consumed by the 28.2 response decoder
// (R5 — never read at 28.1).
type pendingRequest struct {
	opname     string
	wireOpcode int32
	start      time.Time
}

// decoder is the per-connection shallow decoder, BOTH directions (ADR-0222
// request side; ADR-0223 response side — upstream single-DecoderImpl parity).
// It owns its OWN reassembly buffers; the chain Buffer is read, NEVER drained
// (R3). The request path runs on goroutine A (pre-handoff: the chain read
// loop; post-handoff: the downstream→upstream pump via replayRead); the
// response path runs on goroutine B (the upstream→downstream pump via
// writeChainConn.Write → OnWrite). The two share ONLY the correlation maps,
// guarded by mu (§3.6).
type decoder struct {
	cfg   *compiledConfig
	stats *rosterStats

	// chainConsumed is the high-water mark of bytes already fed into readBuf,
	// kept against Buffer.TotalAppended() — NOT against the physical chain-buffer
	// length (28.1b §3.3 re-base; redesigns the 28.1 SPEC §4.5 basis). The
	// never-before-seen bytes are always the trailing (totalAppended −
	// chainConsumed) bytes of chainBytes: bytes are only ever appended at the
	// tail and only ever drained at the head, and the runtime never drains
	// bytes the filters have not yet been shown (the §3.3 soundness invariant).
	// int64 in lockstep with TotalAppended (D-S28.1b-1).
	chainConsumed int64

	// readBuf is the decoder-internal reassembly buffer (AMEND-A8): complete
	// frames are decoded + consumed; a trailing partial frame survives until
	// the next read; a decode failure ABANDONS it (no resync).
	readBuf []byte

	// writeBuf is the decoder-internal write-side reassembly buffer (the
	// upstream zk_filter_write_buffer_ mirror). Fed by OnWrite; complete
	// response frames are decoded + consumed; a trailing partial frame
	// survives until the next OnWrite; a decode failure ABANDONS it (no
	// resync — AMEND-A8 symmetry). Accessed ONLY by goroutine B (§3.6).
	writeBuf []byte

	// mu guards the two correlation maps below — the ONLY state shared
	// between goroutine A (request decode: replayRead → OnData) and
	// goroutine B (response decode: writeChainConn.Write → OnWrite).
	// The reassembly buffers (readBuf / writeBuf) and chainConsumed are
	// single-goroutine-owned and stay OUTSIDE the lock (§3.6). Entries are
	// COPIED OUT under the lock; counter increments + latency math happen
	// OUTSIDE it. The pre-handoff request path locks too (uniformity over
	// cleverness — pre-handoff the lock is uncontended, one atomic CAS).
	mu sync.Mutex

	// Correlation structures (AMEND-A7; written 28.1, consumed 28.2 — R5).
	requestsByXid map[int32]pendingRequest // data requests (xid > 0); insert overwrites
	// controlRequestsByXid holds per-xid FIFO queues for control requests.
	// KNOWN 28.1 boundary: control queues grow unbounded for the connection's
	// lifetime at 28.1 (nothing drains them until the 28.2 response decoder
	// consumes entries); accepted hand-off, documented in PROGRESS.
	controlRequestsByXid map[int32][]pendingRequest
}

// newDecoder returns the per-connection decoder (both-direction reassembly
// buffers + correlation structures + mu; §3.1).
func newDecoder(cfg *compiledConfig, rs *rosterStats) *decoder {
	return &decoder{
		cfg:                  cfg,
		stats:                rs,
		requestsByXid:        map[int32]pendingRequest{},
		controlRequestsByXid: map[int32][]pendingRequest{},
	}
}

// decodeOnData feeds the current chain-buffer contents into the decoder.
// totalAppended is the buffer's monotonic Buffer.TotalAppended() value; the
// high-water mark (chainConsumed) is kept against IT, not against the physical
// buffer length, so the feed is correct regardless of runtime drains (the
// 28.1b read seam drains the buffer at handoff and after every post-handoff
// replay pass). On any never-drained execution TotalAppended() == Len(), so
// this selects byte-for-byte the same slice the 28.1a feed selected (the §3.3
// equivalence — existing assertions unchanged).
func (d *decoder) decodeOnData(chainBytes []byte, totalAppended int64) {
	d.readBuf = append(d.readBuf, network.NewBytes(chainBytes, totalAppended, &d.chainConsumed)...)
	for {
		frame, ok := d.nextFrame(&d.readBuf, d.decoderError)
		if !ok {
			return // no complete frame buffered (or buffer abandoned)
		}
		if !d.decodeFrame(frame) {
			// decoder_error path already counted + readBuf abandoned.
			return
		}
	}
}

// nextFrame extracts one complete frame from *buf (the 4-byte BE length prefix
// EXCLUDES itself and is stripped from the returned frame; buf is &d.readBuf on
// the request side and &d.writeBuf on the response side — the kafkabroker
// nextFrame pointer-parameter shape). Returns ok=false when no complete frame
// is buffered. Oversized frames (len > max_packet_bytes) take the direction's
// decoder_error path (errFn: decoderError for reads, responseError for writes —
// which also abandons that direction's buffer).
func (d *decoder) nextFrame(buf *[]byte, errFn func(string)) ([]byte, bool) {
	b := *buf
	if len(b) < 4 {
		return nil, false
	}
	frameLen := int32(binary.BigEndian.Uint32(b[0:4]))
	if frameLen < 0 || uint32(frameLen) > d.cfg.maxPacketBytes {
		// "packet is too big" (parent §11.5) → decoder_error + abandon.
		errFn("")
		return nil, false
	}
	if len(b) < 4+int(frameLen) {
		return nil, false // partial frame — wait for more bytes
	}
	frame := b[4 : 4+frameLen]
	*buf = b[4+frameLen:]
	return frame, true
}

// decodeFrame dispatches one frame by xid sniffing (AMEND-A5). Returns false on
// a decode failure (the decoder_error path has already run).
func (d *decoder) decodeFrame(frame []byte) bool {
	if len(frame) < 8 {
		// universal min: xid(4) + opcode(4) ("packet is too small").
		d.decoderError("")
		return false
	}
	xid := int32(binary.BigEndian.Uint32(frame[0:4]))
	switch xid {
	case connectXid:
		return d.onConnect(frame)
	case pingXid:
		d.stats.inc("ping_rq")
		d.countRequestBytes("ping", wireFootprint(frame))
		d.recordControl(pingXid, "ping", opPing)
		return true
	case authXid:
		return d.onAuth(frame)
	case setWatchesXid:
		d.stats.inc("setwatches_rq")
		d.countRequestBytes("setwatches", wireFootprint(frame))
		d.recordControl(setWatchesXid, "setwatches", opSetWatches)
		return true
	default:
		return d.onDataRequest(xid, frame) // full version lands at Task 10
	}
}

// onConnect parses the connect special framing: protocol_version(4) +
// last_zxid(8) + timeout(4) + session_id(8) + password(4-byte len + bytes) +
// OPTIONAL trailing readonly bool(1). Readonly present AND true →
// connect_readonly_rq; else connect_rq (AMEND-A3/A5).
func (d *decoder) onConnect(frame []byte) bool {
	// Shallow validation: the fixed header is 28 bytes + password + optional readonly.
	const fixedLen = 4 + 8 + 4 + 8 + 4 // up to and including the password length
	if len(frame) < fixedLen {
		d.decoderError("connect")
		return false
	}
	pwLen := int32(binary.BigEndian.Uint32(frame[24:28]))
	if pwLen < 0 || len(frame) < fixedLen+int(pwLen) {
		d.decoderError("connect")
		return false
	}
	readonly := false
	if rest := frame[fixedLen+int(pwLen):]; len(rest) >= 1 && rest[0] == 1 {
		readonly = true
	}
	opname := "connect"
	if readonly {
		opname = "connect_readonly"
		d.stats.inc("connect_readonly_rq")
	} else {
		d.stats.inc("connect_rq")
	}
	d.countRequestBytes(opname, wireFootprint(frame))
	d.recordControl(connectXid, opname, opConnect)
	return true
}

// onAuth parses the auth special framing. The real ZooKeeper auth request frame
// (length-prefix stripped) is:
//
//	xid(4) | opcode(100, 4) | type(4) | schemeLen(4) | scheme | credLen(4) | cred
//
// Upstream parseAuthRequest (decoder.cc:396-413 at v1.37.2):
//   - ensureMinLength(XID+OPCODE+INT+INT+INT = 20) (decoder.cc:397-398): xid +
//     opcode + type + scheme-len + cred-len (the minimum valid frame has an empty
//     scheme and empty credential — both length-prefixes must be present);
//   - "Skip opcode + type": offset += OPCODE_LENGTH + INT_LENGTH after xid
//     (decoder.cc:401) — so schemeLen sits at frame offset 12, scheme at 16;
//   - peekString reads schemeLen(4) + scheme bytes (decoder.cc:403);
//   - skipString skips the credential (decoder.cc:408 — shallow, not extracted).
//
// The scheme is the only payload value the shallow decoder extracts (SPEC §4.4)
// → the dynamic auth.<scheme>_rq counter (AMEND-A3), gated through the upstream
// builtin-scheme set in authSchemeCounter (a non-builtin scheme takes the
// unknown_scheme fallback). There is NO static auth_rq; auth request BYTES go to
// auth_rq_bytes via countRequestBytes("auth", ...).
func (d *decoder) onAuth(frame []byte) bool {
	if len(frame) < 20 {
		// ensureMinLength XID+OPCODE+INT+INT+INT = 20 (decoder.cc:397-398):
		// xid + opcode + type + scheme-len + cred-len.
		d.decoderError("auth")
		return false
	}
	// frame[4:8]=opcode and frame[8:12]=type are skipped ("Skip opcode + type",
	// decoder.cc:401); schemeLen is at offset 12.
	schemeLen := int32(binary.BigEndian.Uint32(frame[12:16]))
	if schemeLen < 0 || len(frame) < 16+int(schemeLen) {
		d.decoderError("auth")
		return false
	}
	scheme := string(frame[16 : 16+schemeLen])
	d.stats.authSchemeCounter(scheme).Inc()
	d.countRequestBytes("auth", wireFootprint(frame))
	d.recordControl(authXid, "auth", opSetAuth)
	return true
}

// recordControl appends to the per-xid FIFO control queue (AMEND-A7), under the
// correlation-map lock (§3.6: the request path locks unconditionally — pre-handoff
// the lock is uncontended; post-handoff goroutine B's response decode contends).
func (d *decoder) recordControl(xid int32, opname string, wireOpcode int32) {
	entry := pendingRequest{opname: opname, wireOpcode: wireOpcode, start: time.Now()}
	d.mu.Lock()
	d.controlRequestsByXid[xid] = append(d.controlRequestsByXid[xid], entry)
	d.mu.Unlock()
}

// wireFootprint is the request_bytes accounting basis: the 4-byte length
// prefix + the frame payload (SPEC §4.5 item 4 — the 0046 arm-2 cross-side
// equality proof).
func wireFootprint(frame []byte) int { return 4 + len(frame) }

// countRequestBytes increments request_bytes (always) + the flag-gated
// per-opcode <opname>_rq_bytes (AMEND-A2: flags gate increments, never creation).
func (d *decoder) countRequestBytes(opname string, wireBytes int) {
	d.stats.add("request_bytes", uint64(wireBytes))
	if d.cfg.enablePerOpcodeRequestBytesMetrics {
		d.stats.add(opname+"_rq_bytes", uint64(wireBytes))
	}
}

// decoderError runs the decoder_error path (AMEND-A8): increment decoder_error
// (always) + the flag-gated per-opcode counter (when the failing frame's opcode
// is known), then ABANDON the current readBuf (no resync). The connection is
// never closed; the correlation structures persist; later reads decode normally.
func (d *decoder) decoderError(opname string) {
	d.stats.inc("decoder_error")
	if opname != "" && d.cfg.enablePerOpcodeDecoderErrorMetrics {
		d.stats.inc(opname + "_decoder_error")
	}
	d.readBuf = nil
}

// dataRequestMinLength is the per-opcode minimum frame length (xid+opcode+required
// payload header), transcribed from upstream decoder.cc ensureMinLength calls at
// v1.37.2 (D-S28.1-1). Constants: XID=4, OPCODE=4, INT=4, BOOL=1, LONG=8,
// MULTI_HEADER=9. Opcodes absent from the table use the universal 8-byte minimum.
//
// SetAuth (opcode 100) is NOT in this table: the upstream data-request switch has
// NO SetAuth case (decoder.cc:134-244 v1.37.2) — a data-xid SetAuth falls to the
// default branch → onDecodeError(nullopt) before any ensureMinLength is reached.
// The 20-byte ensureMinLength at decoder.cc:398 only fires on the AuthXid (-4)
// control path (onAuth/parseAuthRequest). See opSetAuth branch in onDataRequest.
//
// Upstream decoder.cc line references:
//
//	GetData     line 418: XID+OPCODE+INT+BOOL               = 13
//	Create/Create2/CreateContainer/CreateTTL
//	            line 457: XID+OPCODE+4*INT                  = 24
//	SetData     line 490: XID+OPCODE+3*INT                  = 20
//	GetChildren/GetChildren2
//	            line 513: XID+OPCODE+INT+BOOL               = 13
//	Delete      line 535: XID+OPCODE+2*INT                  = 16
//	Exists      line 553: XID+OPCODE+INT+BOOL               = 13
//	GetAcl      line 571: XID+OPCODE+INT                    = 12
//	SetAcl      line 585: XID+OPCODE+3*INT                  = 20
//	Sync/GetEphemerals/GetAllChildrenNumber (pathOnlyRequest)
//	            line 606: XID+OPCODE+INT                    = 12
//	Check       line 615: XID+OPCODE+2*INT                  = 16
//	Multi       line 634: XID+OPCODE+MULTI_HEADER(9)        = 17
//	Reconfig    line 702: XID+OPCODE+3*INT+LONG             = 28
//	SetWatches  line 729: XID+OPCODE+LONG+3*INT             = 28
//	SetWatches2 line 757: XID+OPCODE+LONG+5*INT             = 36
//	AddWatch    line 792: XID+OPCODE+2*INT                  = 16
//	CheckWatches/RemoveWatches
//	            line 810: XID+OPCODE+2*INT                  = 16
var dataRequestMinLength = map[int32]int{
	opGetData:              13, // XID+OPCODE+INT+BOOL (decoder.cc:418)
	opCreate:               24, // XID+OPCODE+4*INT (decoder.cc:457)
	opCreate2:              24, // XID+OPCODE+4*INT (decoder.cc:457)
	opCreateContainer:      24, // XID+OPCODE+4*INT (decoder.cc:457)
	opCreateTTL:            24, // XID+OPCODE+4*INT (decoder.cc:457)
	opSetData:              20, // XID+OPCODE+3*INT (decoder.cc:490)
	opGetChildren:          13, // XID+OPCODE+INT+BOOL (decoder.cc:513)
	opGetChildren2:         13, // XID+OPCODE+INT+BOOL (decoder.cc:513)
	opDelete:               16, // XID+OPCODE+2*INT (decoder.cc:535)
	opExists:               13, // XID+OPCODE+INT+BOOL (decoder.cc:553)
	opGetACL:               12, // XID+OPCODE+INT (decoder.cc:571)
	opSetACL:               20, // XID+OPCODE+3*INT (decoder.cc:585)
	opSync:                 12, // XID+OPCODE+INT — pathOnlyRequest (decoder.cc:606)
	opCheck:                16, // XID+OPCODE+2*INT (decoder.cc:615)
	opMulti:                17, // XID+OPCODE+MULTI_HEADER(9) (decoder.cc:634)
	opReconfig:             28, // XID+OPCODE+3*INT+LONG (decoder.cc:702)
	opSetWatches:           28, // XID+OPCODE+LONG+3*INT (decoder.cc:729)
	opSetWatches2:          36, // XID+OPCODE+LONG+5*INT (decoder.cc:757)
	opAddWatch:             16, // XID+OPCODE+2*INT (decoder.cc:792)
	opCheckWatches:         16, // XID+OPCODE+2*INT (decoder.cc:810)
	opRemoveWatches:        16, // XID+OPCODE+2*INT (decoder.cc:810)
	opGetEphemerals:        12, // XID+OPCODE+INT — pathOnlyRequest (decoder.cc:606)
	opGetAllChildrenNumber: 12, // XID+OPCODE+INT — pathOnlyRequest (decoder.cc:606)
}

// onDataRequest is the full Task-10 form: opcode lookup, min-length validation
// (D-S28.1-1), per-opcode _rq + bytes + correlation writes.
func (d *decoder) onDataRequest(xid int32, frame []byte) bool {
	opcode := int32(binary.BigEndian.Uint32(frame[4:8]))
	opname, known := wireOpcodeToOpname[opcode]
	if !known {
		d.decoderError("") // unknown opcode: no per-opcode counter
		return false
	}
	// SetAuth-as-data-request: upstream's data-request switch has NO
	// OpCodes::SetAuth case (decoder.cc:134-244 at v1.37.2) — it falls to the
	// default branch → onDecodeError(absl::nullopt) → plain decoder_error (no
	// per-opcode counter, no correlation write). Mirror that exactly: the only
	// sanctioned auth-request path is the AuthXid (-4) control path (onAuth).
	// (The wireOpcodeToOpname "auth" entry documents the macro opname; it is
	// NOT a data-dispatch authorization.)
	if opcode == opSetAuth {
		d.decoderError("")
		return false
	}
	if minLen, ok := dataRequestMinLength[opcode]; ok && len(frame) < minLen {
		d.decoderError(opname) // known opcode: flag-gated per-opcode counter fires
		return false
	}
	d.stats.inc(opname + "_rq")
	d.countRequestBytes(opname, wireFootprint(frame))
	entry := pendingRequest{opname: opname, wireOpcode: opcode, start: time.Now()}
	d.mu.Lock()
	d.requestsByXid[xid] = entry
	d.mu.Unlock()
	return true
}

// --- the response side (28.2; ADR-0223) ---

// decodeOnWrite feeds upstream→downstream bytes into the decoder's write-side
// reassembly buffer and decodes every complete response frame (SPEC §3.2).
// Each OnWrite call passes a fresh per-Write buffer's contents
// (writeChainConn.Write allocates one per call — writeconn.go:35), so p is
// appended directly: every byte arrives exactly once by construction; no
// write-side TotalAppended high-water mark exists (the §3.2 item-1 structural
// asymmetry vs the read side, recorded in ADR-0223). Runs ONLY on goroutine B.
func (d *decoder) decodeOnWrite(p []byte) {
	d.writeBuf = append(d.writeBuf, p...)
	for {
		frame, ok := d.nextFrame(&d.writeBuf, d.responseError)
		if !ok {
			return // no complete frame buffered (or buffer abandoned)
		}
		if !d.decodeResponseFrame(frame) {
			// decoder_error path already counted + writeBuf abandoned.
			return
		}
	}
}

// responseError runs the decoder_error path for the response side (AMEND-A8
// symmetry; the decoderError counting pattern with the WRITE-side abandon
// target): increment decoder_error (always) + the flag-gated per-opcode counter
// (when opname is known — SPEC §3.3: "from a correlation hit"), then ABANDON
// writeBuf (no resync). The connection is never closed; later writes decode
// normally; the correlation maps persist.
func (d *decoder) responseError(opname string) {
	d.stats.inc("decoder_error")
	if opname != "" && d.cfg.enablePerOpcodeDecoderErrorMetrics {
		d.stats.inc(opname + "_decoder_error")
	}
	d.writeBuf = nil
}

// decodeResponseFrame dispatches one response frame by its leading int32
// (SPEC §3.3 xid sniffing). Returns false on a decode failure (the
// decoder_error path has already run).
func (d *decoder) decodeResponseFrame(frame []byte) bool {
	if len(frame) < 4 {
		// universal min: the leading int32 ("packet is too small").
		d.responseError("")
		return false
	}
	leading := int32(binary.BigEndian.Uint32(frame[0:4]))
	switch {
	case leading == connectXid:
		return d.onConnectResponse(frame)
	case leading == watchXid:
		return d.onWatchEvent(frame)
	case leading == pingXid || leading == authXid || leading == setWatchesXid:
		return d.onControlResponse(leading, frame)
	case leading > 0:
		return d.onDataResponse(leading, frame)
	default:
		// Any other negative xid: unknown → decoder_error + abandon (upstream
		// unknown-xid onDecodeError parity — SPEC §3.3 row 5).
		d.responseError("")
		return false
	}
}

// onWatchEvent handles the server-initiated watch-event push (xid −1; SPEC §3.3
// row 2): never correlated, no per-opcode counter, no latency. Like every
// non-connect response, the frame carries the full ReplyHeader — xid(4) +
// zxid(8) + error(4) — followed by event_type(4) + client_state(4) +
// path-len(4): minimum 28 bytes (upstream parseWatchEvent
// SERVER_HEADER_LENGTH + 3*INT_LENGTH; D-S28.2-1 — the SPEC's 16-byte pin was
// corrected to upstream's value at IMPL). Shallow decode: zxid/error/event
// fields are read past for min-length only, never extracted.
// Byte accounting: response_bytes only (watch events have no per-opcode
// attribution — SPEC §3.3 byte-accounting note).
func (d *decoder) onWatchEvent(frame []byte) bool {
	if len(frame) < 28 {
		d.responseError("")
		return false
	}
	d.stats.inc("watch_event")
	d.stats.add("response_bytes", uint64(wireFootprint(frame)))
	return true
}

// respOpname maps a popped entry's opname to its response-side roster opname
// (SPEC §3.4 item 4 — THE CLOSED-ROSTER PANIC TRAP): connect_readonly → connect
// (respOpNames has NO connect_readonly_resp; upstream onConnectResponse always
// increments connect_resp). Everything else passes through unchanged.
func respOpname(entryOpname string) string {
	if entryOpname == "connect_readonly" {
		return "connect"
	}
	return entryOpname
}

// popControl FIFO-pops the front entry of the control queue for xid under the
// correlation-map lock and returns a COPY (§3.6: entries copied out under the
// lock; counter increments + latency math happen OUTSIDE it). ok=false → empty
// queue (no correlation hit).
func (d *decoder) popControl(xid int32) (pendingRequest, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	q := d.controlRequestsByXid[xid]
	if len(q) == 0 {
		return pendingRequest{}, false
	}
	entry := q[0]
	d.controlRequestsByXid[xid] = q[1:]
	return entry, true
}

// takeData looks up + ERASES the data-map entry for xid under the correlation-map
// lock (erase-on-lookup — upstream parity: a second response with the same xid
// finds nothing). ok=false → missing xid (no correlation hit).
func (d *decoder) takeData(xid int32) (pendingRequest, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	entry, ok := d.requestsByXid[xid]
	if ok {
		delete(d.requestsByXid, xid)
	}
	return entry, ok
}

// countResponse runs the per-opcode counting + byte accounting for a
// successfully decoded, correlated response frame (SPEC §3.3 byte accounting):
// <opname>_resp (always) + response_bytes (always, ungated) + the flag-gated
// <opname>_resp_bytes. The respOpname MUST already be roster-mapped (§3.4 item 4).
func (d *decoder) countResponse(op string, frame []byte) {
	d.stats.inc(op + "_resp")
	d.stats.add("response_bytes", uint64(wireFootprint(frame)))
	if d.cfg.enablePerOpcodeResponseBytesMetrics {
		d.stats.add(op+"_resp_bytes", uint64(wireFootprint(frame)))
	}
}

// onConnectResponse handles the connect special framing (SPEC §3.3 row 1):
// proto_version(4) + timeout(4) + session_id(8) + password(4-byte len + bytes)
// — NO zxid, NO error (upstream parseConnectResponse: "Connect responses are
// special, they have no full reply header"; min = 20, verified D-S28.2-1).
// Correlates by FIFO-popping controlRequestsByXid[connectXid]; counters ALWAYS
// use opname "connect" regardless of the popped entry's opname (§3.4 item 4).
// Correlate-then-validate order (upstream parity): the pop happens first, so a
// malformed connect response still consumes the pending entry and fires the
// flag-gated connect_decoder_error.
func (d *decoder) onConnectResponse(frame []byte) bool {
	entry, ok := d.popControl(connectXid)
	if !ok {
		d.responseError("") // empty queue — no correlation hit
		return false
	}
	const fixedLen = 4 + 4 + 8 + 4 // proto_version + timeout + session_id + password length
	if len(frame) < fixedLen {
		d.responseError("connect")
		return false
	}
	pwLen := int32(binary.BigEndian.Uint32(frame[16:20]))
	if pwLen < 0 || len(frame) < fixedLen+int(pwLen) {
		d.responseError("connect")
		return false
	}
	d.countResponse("connect", frame)
	d.recordLatency("connect", entry.wireOpcode, time.Since(entry.start))
	return true
}

// onControlResponse handles control-xid responses (ping −2 / auth −4 /
// setwatches −8; SPEC §3.3 row 3): standard framing xid(4) + zxid(8) + error(4)
// = 16 minimum, correlated by FIFO pop (control xids repeat — AMEND-A7).
func (d *decoder) onControlResponse(xid int32, frame []byte) bool {
	entry, ok := d.popControl(xid)
	if !ok {
		d.responseError("") // empty queue — no correlation hit
		return false
	}
	op := respOpname(entry.opname)
	if len(frame) < 16 {
		// Correlate-then-validate: the entry is consumed; the per-opcode
		// counter fires (flag-gated) — the opname is known from the hit.
		d.responseError(op)
		return false
	}
	d.countResponse(op, frame)
	d.recordLatency(op, entry.wireOpcode, time.Since(entry.start))
	return true
}

// onDataResponse handles data-xid responses (xid > 0; SPEC §3.3 row 4): standard
// framing, correlated against requestsByXid with erase-on-lookup. The zxid(8) +
// error(4) fields are read past for min-length only — neither is extracted
// (shallow decode; D-S28.2-5: upstream feeds error only to dynamic metadata,
// which is deferred — no counter is keyed on it).
func (d *decoder) onDataResponse(xid int32, frame []byte) bool {
	entry, ok := d.takeData(xid)
	if !ok {
		d.responseError("") // missing xid — upstream InvalidArgumentError parity
		return false
	}
	op := respOpname(entry.opname)
	if len(frame) < 16 {
		d.responseError(op)
		return false
	}
	d.countResponse(op, frame)
	d.recordLatency(op, entry.wireOpcode, time.Since(entry.start))
	return true
}

// recordLatency mirrors upstream errorBudgetDecision (filter.cc:134-154 —
// parent §11.7 / AMEND-A10): flag-gated; threshold = the wire-opcode-keyed
// override else the default; latency <= threshold → FAST (INCLUSIVE).
// op must already be roster-mapped (§3.4 item 4). The measured latency
// is a parameter (not computed here) so unit tests inject exact boundary values.
// Runs OUTSIDE the correlation-map lock (§3.6 item 1).
func (d *decoder) recordLatency(op string, wireOpcode int32, latency time.Duration) {
	if !d.cfg.enableLatencyThresholdMetrics {
		return // the flag gates INCREMENTS, not creation (AMEND-A2)
	}
	threshold, ok := d.cfg.latencyThresholdOverrides[wireOpcode]
	if !ok {
		threshold = d.cfg.defaultLatencyThreshold
	}
	if latency <= threshold {
		d.stats.inc(op + "_resp_fast")
	} else {
		d.stats.inc(op + "_resp_slow")
	}
}
