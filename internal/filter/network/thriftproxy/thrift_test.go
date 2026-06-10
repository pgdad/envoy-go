package thriftproxy

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"testing"
)

// framedBinaryCall builds a framed-binary CALL frame (Appendix A).
func framedBinaryCall(msgType uint8, method string, seqID int32) []byte {
	var p bytes.Buffer
	p.Write([]byte{0x80, 0x01, 0x00, msgType})
	_ = binary.Write(&p, binary.BigEndian, int32(len(method)))
	p.WriteString(method)
	_ = binary.Write(&p, binary.BigEndian, seqID)
	p.WriteByte(0x00) // empty struct STOP
	var f bytes.Buffer
	_ = binary.Write(&f, binary.BigEndian, int32(p.Len()))
	f.Write(p.Bytes())
	return f.Bytes()
}

func TestDecodeFrame_Call(t *testing.T) {
	in := framedBinaryCall(msgTypeCall, "ping", 1)
	r := bufio.NewReader(bytes.NewReader(in))
	m, err := decodeFrame(r)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.msgType != msgTypeCall || m.method != "ping" || m.seqID != 1 {
		t.Fatalf("got %+v", m)
	}
	if !bytes.Equal(m.raw, in) {
		t.Fatalf("raw frame not preserved verbatim")
	}
}

func TestDecodeFrame_Errors(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
	}{
		{"empty", nil},
		{"truncated-length", []byte{0x00, 0x00}},
		{"truncated-payload", append([]byte{0x00, 0x00, 0x00, 0x11}, 0x80, 0x01)},
		{"bad-magic", func() []byte { b := framedBinaryCall(msgTypeCall, "x", 1); b[4] = 0x00; b[5] = 0x00; return b }()},
		{"bad-msgtype", func() []byte { b := framedBinaryCall(0x09, "x", 1); return b }()},
		{"zero-length", []byte{0x00, 0x00, 0x00, 0x00}},
		{"oversized-length", []byte{0x7f, 0xff, 0xff, 0xff}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReader(bytes.NewReader(tc.in))
			if _, err := decodeFrame(r); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

// framedBinaryReply builds a framed-binary REPLY frame (Appendix A) with an
// opaque body (the result-struct bytes the classifier peeks at).
func framedBinaryReply(method string, seqID int32, body []byte) []byte {
	var p bytes.Buffer
	p.Write([]byte{0x80, 0x01, 0x00, msgTypeReply})
	_ = binary.Write(&p, binary.BigEndian, int32(len(method)))
	p.WriteString(method)
	_ = binary.Write(&p, binary.BigEndian, seqID)
	p.Write(body)
	var f bytes.Buffer
	_ = binary.Write(&f, binary.BigEndian, int32(p.Len()))
	f.Write(p.Bytes())
	return f.Bytes()
}

func mustDecode(t *testing.T, frame []byte) *thriftMessage {
	t.Helper()
	m, err := decodeFrame(bufio.NewReader(bytes.NewReader(frame)))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m
}

func TestClassifyReply(t *testing.T) {
	// void success: body is a single STOP byte
	m := mustDecode(t, framedBinaryReply("ping", 1, []byte{0x00}))
	if got := classifyReply(m); got != replySuccess {
		t.Fatalf("void reply class = %v want success", got)
	}
	// error: first field id 1 (type STRING 0x0b, id 0x0001), then value+STOP
	errBody := []byte{0x0b, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 'x', 0x00}
	m2 := mustDecode(t, framedBinaryReply("ping", 1, errBody))
	if got := classifyReply(m2); got != replyError {
		t.Fatalf("field-id-1 reply class = %v want error", got)
	}
}

func TestEncodeUnknownMethodException(t *testing.T) {
	// SPEC Appendix A live-captured layout for method "somethingelse", seq 1.
	got := encodeUnknownMethod("somethingelse", 1)
	want := []byte{
		0x00, 0x00, 0x00, 0x4b, // frame len (75 = assembled payload bytes; recomputed at IMPL)
		0x80, 0x01, 0x00, 0x03, // version + EXCEPTION(3)
		0x00, 0x00, 0x00, 0x0d, // name-len 13
		's', 'o', 'm', 'e', 't', 'h', 'i', 'n', 'g', 'e', 'l', 's', 'e',
		0x00, 0x00, 0x00, 0x01, // seq_id 1
		0x0b, 0x00, 0x01, 0x00, 0x00, 0x00, 0x23, // STRING id 1, len 0x23=35
		'n', 'o', ' ', 'r', 'o', 'u', 't', 'e', ' ', 'f', 'o', 'r', ' ', 'm', 'e', 't', 'h', 'o', 'd', ' ',
		'\'', 's', 'o', 'm', 'e', 't', 'h', 'i', 'n', 'g', 'e', 'l', 's', 'e', '\'',
		0x08, 0x00, 0x02, 0x00, 0x00, 0x00, 0x01, // I32 id 2, value 1 (UnknownMethod)
		0x00, // STOP
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("exception bytes mismatch:\n got %x\nwant %x", got, want)
	}
}
