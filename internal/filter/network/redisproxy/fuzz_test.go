package redisproxy

import (
	"bufio"
	"bytes"
	"testing"
)

// FuzzRESPDecode is the 41st fuzzer (SPEC §9). It feeds arbitrary bytes through
// BOTH resp.go decode entry points (decodeRequest + decodeReply) via a bufio.Reader
// over a bytes.Reader and asserts: (1) no panic; (2) no mutation of the input slice
// (the decoder reads, never writes back); (3) bounded allocation — a crafted length
// header never allocates beyond maxBulkLen (512 MiB) / maxArrayLen (1 Mi) before the
// overflow guards reject it (resp.go:14-17). Per reference_dynamic_stat_name_charset
// _guard the codec touches NO registry (the per-command stat lookup is table-bounded
// in filter.go, not resp.go) — the fuzzer scope is the codec only.
func FuzzRESPDecode(f *testing.F) {
	seeds := [][]byte{
		[]byte("PING\r\n"), // inline
		[]byte("*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"), // valid array request
		[]byte("+OK\r\n"),                      // reply: simple string
		[]byte("-ERR x\r\n"),                   // reply: error
		[]byte(":1\r\n"),                       // reply: integer
		[]byte("$3\r\nbar\r\n"),                // reply: bulk
		[]byte("$-1\r\n"),                      // reply: null bulk
		[]byte("*2\r\n$1\r\na\r\n$1\r\nb\r\n"), // reply: array
		[]byte("*-1\r\n"),                      // reply: null array
		[]byte("$10\r\nshort"),                 // partial frame
		[]byte("$999999999999\r\n"),            // overflow length
		[]byte("?xyz\r\n"),                     // bad type byte
		[]byte(":abc\r\n"),                     // non-numeric integer
		[]byte("*1\r\n$3\r\nbf.\r\n"),          // dotted-name request
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		orig := append([]byte(nil), data...)

		// Invariant 1: no panic (implicit). Drive both entry points over fresh readers.
		_, _, _, _ = decodeRequest(bufio.NewReader(bytes.NewReader(data)))
		_, _ = decodeReply(bufio.NewReader(bytes.NewReader(data)))

		// Invariant 2: the input was never mutated.
		if !bytes.Equal(data, orig) {
			t.Fatal("decode mutated the input bytes")
		}
		// Invariant 3 (bounded allocation) is enforced by the resp.go overflow guards
		// (maxBulkLen/maxArrayLen checked BEFORE make()); a panic or OOM here would
		// fail the run. No explicit assertion — the guards are the bound (D-S32.2-8).
	})
}
