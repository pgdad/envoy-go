package zookeeperproxy

import (
	"encoding/binary"
	"time"
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

// requestDecoder is the per-connection shallow request decoder (ADR-0222;
// AMEND-A5/A8; D-P2 shallow). It owns its OWN reassembly buffer; the chain
// Buffer is read, NEVER drained (R3).
type requestDecoder struct {
	cfg   *compiledConfig
	stats *rosterStats

	// chainConsumed is the high-water mark of chain-buffer bytes already fed
	// into readBuf (D-S28.1-3, PLAN-resolved: the mark lives on the decoder).
	// The chain buffer accumulates undrained bytes across reads, so
	// decodeOnData receives the FULL buffer each call and feeds only the new
	// tail — re-delivered bytes are never double-counted.
	chainConsumed int

	// readBuf is the decoder-internal reassembly buffer (AMEND-A8): complete
	// frames are decoded + consumed; a trailing partial frame survives until
	// the next read; a decode failure ABANDONS it (no resync).
	readBuf []byte

	// Correlation structures (AMEND-A7; written 28.1, consumed 28.2 — R5).
	requestsByXid map[int32]pendingRequest // data requests (xid > 0); insert overwrites
	// controlRequestsByXid holds per-xid FIFO queues for control requests.
	// KNOWN 28.1 boundary: control queues grow unbounded for the connection's
	// lifetime at 28.1 (nothing drains them until the 28.2 response decoder
	// consumes entries); accepted hand-off, documented in PROGRESS.
	controlRequestsByXid map[int32][]pendingRequest
}

func newRequestDecoder(cfg *compiledConfig, rs *rosterStats) *requestDecoder {
	return &requestDecoder{
		cfg:                  cfg,
		stats:                rs,
		requestsByXid:        map[int32]pendingRequest{},
		controlRequestsByXid: map[int32][]pendingRequest{},
	}
}

// decodeOnData feeds the FULL current chain-buffer contents into the decoder.
// Only bytes past the high-water mark are appended to readBuf (a COPY — the
// chain buffer is never aliased or mutated); then complete frames are decoded
// in a loop. Decode failures abandon readBuf (AMEND-A8 no-resync) but never
// affect the chain buffer or the connection.
func (d *requestDecoder) decodeOnData(chainBytes []byte) {
	if len(chainBytes) > d.chainConsumed {
		d.readBuf = append(d.readBuf, chainBytes[d.chainConsumed:]...)
		d.chainConsumed = len(chainBytes)
	}
	for {
		frame, ok := d.nextFrame()
		if !ok {
			return // no complete frame buffered (or buffer abandoned)
		}
		if !d.decodeFrame(frame) {
			// decoder_error path already counted + readBuf abandoned.
			return
		}
	}
}

// nextFrame extracts one complete frame from readBuf (the 4-byte BE length
// prefix EXCLUDES itself and is stripped from the returned frame). Returns
// ok=false when no complete frame is buffered. Oversized frames
// (len > max_packet_bytes) take the decoder_error path and abandon the buffer.
func (d *requestDecoder) nextFrame() ([]byte, bool) {
	if len(d.readBuf) < 4 {
		return nil, false
	}
	frameLen := int32(binary.BigEndian.Uint32(d.readBuf[0:4]))
	if frameLen < 0 || uint32(frameLen) > d.cfg.maxPacketBytes {
		// "packet is too big" (parent §11.5) → decoder_error + abandon.
		d.decoderError("")
		return nil, false
	}
	if len(d.readBuf) < 4+int(frameLen) {
		return nil, false // partial frame — wait for more bytes
	}
	frame := d.readBuf[4 : 4+frameLen]
	d.readBuf = d.readBuf[4+frameLen:]
	return frame, true
}

// decodeFrame dispatches one frame by xid sniffing (AMEND-A5). Returns false on
// a decode failure (the decoder_error path has already run).
func (d *requestDecoder) decodeFrame(frame []byte) bool {
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
func (d *requestDecoder) onConnect(frame []byte) bool {
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
func (d *requestDecoder) onAuth(frame []byte) bool {
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

// recordControl appends to the per-xid FIFO control queue (AMEND-A7).
func (d *requestDecoder) recordControl(xid int32, opname string, wireOpcode int32) {
	d.controlRequestsByXid[xid] = append(d.controlRequestsByXid[xid],
		pendingRequest{opname: opname, wireOpcode: wireOpcode, start: time.Now()})
}

// wireFootprint is the request_bytes accounting basis: the 4-byte length
// prefix + the frame payload (SPEC §4.5 item 4 — the 0046 arm-2 cross-side
// equality proof).
func wireFootprint(frame []byte) int { return 4 + len(frame) }

// countRequestBytes increments request_bytes (always) + the flag-gated
// per-opcode <opname>_rq_bytes (AMEND-A2: flags gate increments, never creation).
func (d *requestDecoder) countRequestBytes(opname string, wireBytes int) {
	d.stats.add("request_bytes", uint64(wireBytes))
	if d.cfg.enablePerOpcodeRequestBytesMetrics {
		d.stats.add(opname+"_rq_bytes", uint64(wireBytes))
	}
}

// decoderError runs the decoder_error path (AMEND-A8): increment decoder_error
// (always) + the flag-gated per-opcode counter (when the failing frame's opcode
// is known), then ABANDON the current readBuf (no resync). The connection is
// never closed; the correlation structures persist; later reads decode normally.
func (d *requestDecoder) decoderError(opname string) {
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
func (d *requestDecoder) onDataRequest(xid int32, frame []byte) bool {
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
	d.requestsByXid[xid] = pendingRequest{opname: opname, wireOpcode: opcode, start: time.Now()}
	return true
}
