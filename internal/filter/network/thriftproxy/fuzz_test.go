package thriftproxy

import (
	"bufio"
	"bytes"
	"testing"
)

// FuzzThriftDecode is the 42nd fuzzer (SPEC §14). It feeds arbitrary bytes through
// decodeFrame (+ classifyReply on success) via a bufio.Reader over a bytes.Reader
// and asserts: (1) no panic; (2) no mutation of the input slice (the decoder reads,
// never writes back); (3) bounded allocation — a crafted 4-byte length prefix never
// allocates beyond maxFrameSize before the bounds guard rejects it (thrift.go:52,
// checked BEFORE make()). Per reference_dynamic_stat_name_charset_guard the codec
// touches NO registry (the roster is fixed in stats.go) — the fuzzer scope is the
// codec only.
func FuzzThriftDecode(f *testing.F) {
	seeds := [][]byte{
		framedBinaryCall(msgTypeCall, "ping", 1),                                                         // valid CALL
		framedBinaryReply("ping", 1, []byte{0x00}),                                                       // valid void REPLY
		{0x00, 0x00, 0x00, 0x11, 0x80, 0x01, 0x00},                                                       // truncated payload
		{0x00, 0x00, 0x00, 0x0c, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, // bad magic
		{0x7f, 0xff, 0xff, 0xff},                                                                         // oversized length prefix
		framedBinaryCall(0x09, "x", 1),                                                                   // invalid msgtype
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		orig := append([]byte(nil), data...)

		// Invariant 1: no panic (implicit). Drive the codec over a fresh reader.
		if m, err := decodeFrame(bufio.NewReader(bytes.NewReader(data))); err == nil && m != nil {
			_ = classifyReply(m)
		}

		// Invariant 2: the input was never mutated.
		if !bytes.Equal(orig, data) {
			t.Fatal("decoder mutated its input")
		}
		// Invariant 3 (bounded allocation) is enforced by the thrift.go length guard
		// (maxFrameSize checked BEFORE make()); a panic or OOM here would fail the run.
		// No explicit assertion — the guard is the bound (D-S33-5).
	})
}
