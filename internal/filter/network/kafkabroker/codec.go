package kafkabroker

import (
	"encoding/binary"
	"errors"
	"sync"
)

// maxFrameLen caps a Kafka frame's declared body length. A 4-byte INT32 length
// prefix can declare up to ~2GiB; a value beyond this sane cap is treated as
// framing garbage (resync, see nextFrame). 100 MiB comfortably exceeds any real
// Kafka request (the broker's default socket.request.max.bytes is 100 MiB).
const maxFrameLen = 100 << 20

// errShortBuffer is the truncation sentinel: a read ran past the end of the
// available bytes. WITHIN a complete frame this means the frame body was shorter
// than the header it claims to carry — i.e. malformed (counted as request.failure).
// At the FRAMING layer (nextFrame, which inspects raw reassembly bytes) it instead
// means "wait for more bytes" — nextFrame never surfaces a reader error.
var errShortBuffer = errors.New("kafkabroker: short buffer")

// errMalformed is the distinct decode-failure sentinel: a value was structurally
// invalid within a fully-received frame (a nullable-string length < -1, a length
// exceeding the frame body, a tagged-field size past the frame end). Counted as
// request.failure.
var errMalformed = errors.New("kafkabroker: malformed")

// respSpec is the (api_key, api_version) recovered on the response side (Task 8)
// via the correlation map.
type respSpec struct {
	apiKey     int16
	apiVersion int16
}

// decoder is the per-connection Kafka wire decoder. It owns its OWN reassembly
// buffers (the chain Buffer is read, NEVER drained). chainConsumed is the
// high-water mark against the chain Buffer's TotalAppended() (the mongoproxy
// decoder mechanism). The correlation map (expected) is written on the request
// side and read+erased on the response side (Task 8); it is the ONLY field shared
// across the two pumps, guarded by mu (ADR-0223 — the mongoproxy per-connection
// mutex narrowed to one map).
type decoder struct {
	stats         *kafkaStats
	chainConsumed int64  // high-water vs the chain Buffer's cumulative appended count
	readBuf       []byte // private request reassembly buffer
	writeBuf      []byte // private response reassembly buffer (write goroutine only; no high-water — the write seam is incremental, see decodeOnWrite)

	mu       sync.Mutex         // guards EXACTLY `expected` (ADR-0223)
	expected map[int32]respSpec // correlation_id -> (api_key, api_version)
}

// newDecoder returns a fresh per-connection decoder.
func newDecoder(ks *kafkaStats) *decoder {
	return &decoder{stats: ks, expected: make(map[int32]respSpec)}
}

// register records corr -> (key, ver) under mu. Called BEFORE attempting the
// client_id (D-PLAN-2): a request that fails on a malformed client_id still
// leaves its registration (upstream expectResponse-on-onFailedParse parity).
func (d *decoder) register(corr int32, key, ver int16) {
	d.mu.Lock()
	d.expected[corr] = respSpec{key, ver}
	d.mu.Unlock()
}

// lookupAndErase returns + removes the spec for corr (the response-side
// first-match-erase; Task 8 consumer). ok=false → no pending request.
func (d *decoder) lookupAndErase(corr int32) (respSpec, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.expected[corr]
	if ok {
		delete(d.expected, corr)
	}
	return s, ok
}

// newBytes returns ONLY the never-before-seen trailing bytes of the cumulative
// chain buffer (the bytes beyond *consumed), then advances *consumed to
// totalAppended. The filter (Task 9) calls decodeOnData(buf.Bytes(),
// buf.TotalAppended()) where buf.Bytes() is the WHOLE chain buffer so far and
// TotalAppended() is its monotonic cumulative count; this helper mirrors the
// mongoproxy high-water slice (chainBytes[len-newCount:]). codec.go stays
// decoupled from the network package — the filter passes []byte + int64 in.
func newBytes(chain []byte, totalAppended int64, consumed *int64) []byte {
	newCount := totalAppended - *consumed
	if newCount <= 0 {
		return nil
	}
	if newCount > int64(len(chain)) {
		newCount = int64(len(chain)) // defensive: never slice before the buffer start
	}
	out := chain[int64(len(chain))-newCount:]
	*consumed = totalAppended
	return out
}

// decodeOnData appends only the never-before-seen trailing bytes (high-water),
// then drains complete frames. Each complete frame decodes ONE request header.
// It NEVER drains the chain buffer, never closes, never halts.
func (d *decoder) decodeOnData(chain []byte, totalAppended int64) {
	d.readBuf = append(d.readBuf, newBytes(chain, totalAppended, &d.chainConsumed)...)
	for {
		frame, ok := d.nextFrame(&d.readBuf) // peels 4-byte length + body; ok=false → wait/reset
		if !ok {
			return
		}
		d.decodeRequestFrame(frame)
	}
}

// nextFrame peels one complete Kafka frame (4-byte INT32 length prefix + body)
// off *buf. Returns ok=false to STOP the drain loop:
//   - len < 4: partial length prefix — WAIT (buffer left intact for next OnData).
//   - N < 0 or N > maxFrameLen: framing garbage — reset *buf=nil (resync) and
//     stop. This is FRAMING-level corruption, NOT a header-decode failure, so it
//     is NOT counted as request.failure (request.failure is reserved for
//     header-level decode errors on a fully-received frame). The mongoproxy
//     precedent counts a decoding_error on a bad message length, but kafka's
//     request.failure is explicitly the per-request header-decode bucket
//     (D-PLAN-3), and there is no framing-error counter in the roster — so a
//     negative/absurd length is a silent resync (matches "count nothing for pure
//     framing garbage"). Documented choice.
//   - len < 4+N: partial frame — WAIT.
//
// Otherwise it slices out the N-byte body, advances *buf, and returns (frame, true).
func (d *decoder) nextFrame(buf *[]byte) ([]byte, bool) {
	b := *buf
	if len(b) < 4 {
		return nil, false // partial length prefix — wait
	}
	n := int32(binary.BigEndian.Uint32(b[0:4]))
	if n < 0 || int64(n) > maxFrameLen {
		*buf = nil // framing garbage — resync, stop the loop
		return nil, false
	}
	if int64(len(b)) < int64(4)+int64(n) {
		return nil, false // partial frame — wait for more bytes
	}
	frame := b[4 : 4+n]
	*buf = b[4+n:]
	return frame, true
}

// decodeRequestFrame decodes ONE complete request frame's header. The frame body
// is already fully received, so ANY read error (truncation OR structural) is
// malformed → request.failure. Registration of the correlation triple happens
// BEFORE the client_id attempt (D-PLAN-2).
func (d *decoder) decodeRequestFrame(frame []byte) {
	r := newReader(frame)
	key, err := r.int16()
	if err != nil {
		d.stats.incRequestFailure()
		return
	}
	ver, err := r.int16()
	if err != nil {
		d.stats.incRequestFailure()
		return
	}
	corr, err := r.int32()
	if err != nil {
		d.stats.incRequestFailure()
		return
	}
	d.register(corr, key, ver) // register BEFORE client_id (the onFailedParse parity)
	if _, err := r.nullableString(); err != nil {
		d.stats.incRequestFailure()
		return
	}
	if requestUsesTaggedFieldsInHeader(key, ver) {
		if err := r.skipTaggedFields(); err != nil {
			d.stats.incRequestFailure()
			return
		}
	}
	root, ok := apiKeyName(key)
	maxV, okv := apiKeyMaxVersion(key)
	if !ok || !okv || ver > maxV {
		d.stats.incRequestUnknown()
		return
	}
	d.stats.incRequest(root)
}

// decodeOnWrite appends the write-direction (upstream→downstream) bytes and
// drains complete response frames. Each complete frame recovers ONE correlation.
//
// Calling convention — INCREMENTAL (no high-water), mirroring mongoproxy's
// decodeOnWrite(p []byte): the filter (Task 9) feeds buf.Bytes() from the write
// seam, and writeChainConn.Write allocates a FRESH *Buffer per Write call
// (internal/filter/network/writeconn.go: `buf := &Buffer{}` then `buf.Append(p)`,
// ADR-0221). So each chunk arrives EXACTLY ONCE and buf.Bytes() is that chunk
// alone (never cumulative). We therefore append `chain` directly to writeBuf and
// reassemble across calls there — NO TotalAppended high-water mark is needed on
// the write side (the 28.2 read/write asymmetry; ADR-0225). This is the
// EXCEPTION the PLAN allowed: mongoproxy's OnWrite (and the writeConn source)
// prove the seam is incremental. NEVER drains the chain buffer, never halts.
func (d *decoder) decodeOnWrite(chain []byte) {
	d.writeBuf = append(d.writeBuf, chain...)
	for {
		frame, ok := d.nextFrame(&d.writeBuf)
		if !ok {
			return
		}
		d.decodeResponseFrame(frame)
	}
}

// decodeResponseFrame recovers ONE complete response frame. The frame is fully
// received, so a correlation_id read error is a malformed response →
// response.failure. The response side reads ONLY correlation_id (D-PLAN-3): it
// recovers (api_key, api_version) from the correlation map (first-match-erase)
// rather than parsing the response header tagged-fields.
//   - correlation_id read error (on a COMPLETE frame) → response.failure.
//   - unregistered correlation_id (no pending request) → response.failure
//     (AMEND-K4 unregistered-correlation arm).
//   - recovered key unknown OR api_version > maxVersion(key) → response.unknown.
//   - otherwise → response.<root>_response.
func (d *decoder) decodeResponseFrame(frame []byte) {
	r := newReader(frame)
	corr, err := r.int32()
	if err != nil {
		d.stats.incResponseFailure()
		return
	}
	spec, ok := d.lookupAndErase(corr)
	if !ok { // unregistered correlation_id → response.failure (AMEND-K4)
		d.stats.incResponseFailure()
		return
	}
	root, okName := apiKeyName(spec.apiKey)
	maxV, okMax := apiKeyMaxVersion(spec.apiKey)
	if !okName || !okMax || spec.apiVersion > maxV {
		d.stats.incResponseUnknown()
		return
	}
	d.stats.incResponse(root)
}

// reader is a big-endian primitive cursor over a single complete frame body.
// All reads past the end return errShortBuffer (which, inside a complete frame,
// IS a malformed-frame failure).
type reader struct {
	buf []byte
	pos int
}

func newReader(b []byte) *reader { return &reader{buf: b} }

func (r *reader) remaining() int { return len(r.buf) - r.pos }

// int16 reads a 2-byte big-endian signed integer.
func (r *reader) int16() (int16, error) {
	if r.remaining() < 2 {
		return 0, errShortBuffer
	}
	v := int16(binary.BigEndian.Uint16(r.buf[r.pos : r.pos+2]))
	r.pos += 2
	return v, nil
}

// int32 reads a 4-byte big-endian signed integer.
func (r *reader) int32() (int32, error) {
	if r.remaining() < 4 {
		return 0, errShortBuffer
	}
	v := int32(binary.BigEndian.Uint32(r.buf[r.pos : r.pos+4]))
	r.pos += 4
	return v, nil
}

// nullableString reads a NULLABLE_STRING: an INT16 length L then L bytes.
// L == -1 → null (returns nil, nil). L < -1 → errMalformed. L > remaining bytes
// (within a complete frame) → errMalformed. A truncated length prefix →
// errShortBuffer.
func (r *reader) nullableString() ([]byte, error) {
	l, err := r.int16()
	if err != nil {
		return nil, err
	}
	if l == -1 {
		return nil, nil
	}
	if l < -1 {
		return nil, errMalformed
	}
	if int(l) > r.remaining() {
		return nil, errMalformed // length exceeds the complete frame body
	}
	s := r.buf[r.pos : r.pos+int(l)]
	r.pos += int(l)
	return s, nil
}

// unsignedVarint reads a Kafka UNSIGNED_VARINT (7 bits per byte, MSB
// continuation). Truncation → errShortBuffer. An over-long encoding (more than 5
// bytes for a uint32) → errMalformed.
func (r *reader) unsignedVarint() (uint32, error) {
	var v uint32
	var shift uint
	for i := 0; ; i++ {
		if r.remaining() < 1 {
			return 0, errShortBuffer
		}
		b := r.buf[r.pos]
		r.pos++
		if i >= 5 {
			return 0, errMalformed // a uint32 varint is at most 5 bytes
		}
		v |= uint32(b&0x7F) << shift
		if b&0x80 == 0 {
			return v, nil
		}
		shift += 7
	}
}

// skipTaggedFields consumes a tagged-fields section: an UNSIGNED_VARINT count C,
// then C fields each = tag UNSIGNED_VARINT + size UNSIGNED_VARINT + `size` bytes.
// A size exceeding the remaining frame bytes → errMalformed. Truncation while
// reading a varint → errShortBuffer.
func (r *reader) skipTaggedFields() error {
	count, err := r.unsignedVarint()
	if err != nil {
		return err
	}
	for i := uint32(0); i < count; i++ {
		if _, err := r.unsignedVarint(); err != nil { // tag
			return err
		}
		size, err := r.unsignedVarint() // size
		if err != nil {
			return err
		}
		if int64(size) > int64(r.remaining()) {
			return errMalformed
		}
		r.pos += int(size)
	}
	return nil
}
