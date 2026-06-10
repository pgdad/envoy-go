package thriftproxy

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

const (
	maxFrameSize  = 100 * 1024 * 1024 // 100 MiB frame guard (the reference default order)
	binaryVersion = 0x8001            // strict binary protocol version magic

	msgTypeCall      uint8 = 1
	msgTypeReply     uint8 = 2
	msgTypeException uint8 = 3
	msgTypeOneway    uint8 = 4

	minMessageBeginLength = 12 // magic(2)+zero(1)+msgtype(1)+namelen(4)+seqid(4) with empty name
)

// errDecode is the internal framing error (never byte-compared — the differential
// proof is the request_decoding_error COUNT, not the message; the redisproxy
// errProtocol precedent). errInvalidType is a DISTINCT sentinel for an
// out-of-range message type (→ request_invalid_type, NOT request_decoding_error)
// so Task 8 can switch on errors.Is(err, errInvalidType).
var (
	errDecode      = errors.New("thrift_proxy: frame decode error")
	errInvalidType = errors.New("thrift_proxy: invalid message type")
)

// thriftMessage is one decoded framed-binary message-begin plus the RAW frame
// (forwarded VERBATIM up/downstream — §8) and the opaque passthrough body.
type thriftMessage struct {
	msgType uint8
	method  string
	seqID   int32
	raw     []byte // the full frame: 4-byte length prefix + payload
	body    []byte // the opaque struct bytes after the message-begin (passthrough)
}

// decodeFrame reads ONE framed-binary frame (Appendix A) from r and decodes the
// message-begin, keeping the raw frame + the opaque body. Blocks on partial
// frames; never panics on arbitrary bytes; bounds-checks the length prefix BEFORE
// allocating. A frame-boundary io.EOF (no bytes) is returned verbatim (clean end).
func decodeFrame(r *bufio.Reader) (*thriftMessage, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err // io.EOF here = clean end between frames
	}
	frameLen := int32(binary.BigEndian.Uint32(lenBuf[:]))
	if frameLen <= 0 || int64(frameLen) > maxFrameSize {
		return nil, errDecode
	}
	payload := make([]byte, frameLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, errDecode // a mid-frame short read is a framing error
	}
	if len(payload) < minMessageBeginLength {
		return nil, errDecode
	}
	if binary.BigEndian.Uint16(payload[0:2]) != binaryVersion {
		return nil, errDecode // bad magic / non-strict / wrong version
	}
	mt := payload[3]
	if mt < msgTypeCall || mt > msgTypeOneway {
		return nil, errInvalidType // distinct sentinel → request_invalid_type (Task 8 errors.Is)
	}
	nameLen := int32(binary.BigEndian.Uint32(payload[4:8]))
	if nameLen < 0 || 8+int64(nameLen)+4 > int64(len(payload)) {
		return nil, errDecode
	}
	method := string(payload[8 : 8+nameLen])
	off := 8 + nameLen
	seqID := int32(binary.BigEndian.Uint32(payload[off : off+4]))
	off += 4
	raw := make([]byte, 0, 4+len(payload))
	raw = append(raw, lenBuf[:]...)
	raw = append(raw, payload...)
	return &thriftMessage{msgType: mt, method: method, seqID: seqID, raw: raw, body: payload[off:]}, nil
}

type replyClass int

const (
	replySuccess replyClass = iota
	replyError
)

// classifyReply peeks the first result-struct field header in a REPLY body
// (D-S33-4): field-type STOP (empty struct = void) OR field-id 0 → success;
// field-id >= 1 → error. Under payload_passthrough the body is opaque bytes; this
// reads only the leading field header (1 type byte + 2 id bytes).
func classifyReply(m *thriftMessage) replyClass {
	if len(m.body) == 0 || m.body[0] == 0x00 { // STOP → void success
		return replySuccess
	}
	if len(m.body) < 3 {
		return replySuccess // malformed-short → treat as success (no field id readable)
	}
	fieldID := int16(binary.BigEndian.Uint16(m.body[1:3]))
	if fieldID == 0 {
		return replySuccess
	}
	return replyError
}

// encodeUnknownMethod builds the local UnknownMethod AppException EXCEPTION frame
// (Appendix A): framed-binary EXCEPTION(3) echoing method + seqID, carrying the
// TStruct {1: string "no route for method '<method>'", 2: i32 1}. Byte-identical
// to the reference's miss-path reply (the 0057 miss-arm byte-equivalence proof).
func encodeUnknownMethod(method string, seqID int32) []byte {
	msg := "no route for method '" + method + "'"
	var p []byte
	p = append(p, 0x80, 0x01, 0x00, msgTypeException)
	p = appendI32(p, int32(len(method)))
	p = append(p, method...)
	p = appendI32(p, seqID)
	// field 1: STRING (0x0b) id 1
	p = append(p, 0x0b, 0x00, 0x01)
	p = appendI32(p, int32(len(msg)))
	p = append(p, msg...)
	// field 2: I32 (0x08) id 2, value 1 (AppExceptionType::UnknownMethod)
	p = append(p, 0x08, 0x00, 0x02)
	p = appendI32(p, 1)
	// STOP
	p = append(p, 0x00)
	frame := appendI32(nil, int32(len(p)))
	return append(frame, p...)
}

func appendI32(b []byte, v int32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
