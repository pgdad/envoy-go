package network

import (
	"errors"
	"io"
	"testing"
)

// multiReadConn yields one scripted payload per Read call, then io.EOF forever.
// (The chain_test.go scriptConn is single-read; the readconn tests need multiple
// distinct live reads.) Embeds scriptConn for the no-op net.Conn methods + the
// Write capture; only Read is overridden.
type multiReadConn struct {
	scriptConn
	payloads [][]byte
}

func (c *multiReadConn) Read(b []byte) (int, error) {
	if len(c.payloads) == 0 {
		return 0, io.EOF
	}
	n := copy(b, c.payloads[0])
	c.payloads = c.payloads[1:]
	return n, nil
}

// errConn returns a fixed non-EOF error on every Read.
type errConn struct {
	scriptConn
	err error
}

func (c *errConn) Read(_ []byte) (int, error) { return 0, c.err }

// Live reads pass through unchanged AND are replayed to the read chain BEFORE
// Read returns (replay-before-return — the §5.1 deterministic-scrape ordering).
func TestReadChainConnPassthroughAndReplay(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{f}, &fakeConn{}, connFacts{})
	inner := &multiReadConn{payloads: [][]byte{[]byte("hello")}}
	rc := newReadChainConn(inner, rt)

	buf := make([]byte, 16)
	n, err := rc.Read(buf)
	if err != nil || string(buf[:n]) != "hello" {
		t.Fatalf("Read = (%q, %v), want (hello, nil) — passthrough", buf[:n], err)
	}
	// Replay-before-return: by the time Read returned, the filter saw the bytes.
	if string(f.seen) != "hello" {
		t.Fatalf("filter saw %q, want hello (replay-before-return)", f.seen)
	}
}

// io.EOF triggers ONE final endStream replay (pre-handoff read-loop symmetry,
// internal/listener/manager.go:1073-1077 (serveNetworkChain)), in addition to replaying any bytes delivered before it.
func TestReadChainConnEOFEndStreamReplay(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{f}, &fakeConn{}, connFacts{})
	inner := &multiReadConn{payloads: [][]byte{[]byte("x")}} // one read, then EOF
	rc := newReadChainConn(inner, rt)

	buf := make([]byte, 16)
	_, _ = rc.Read(buf) // "x"
	n, err := rc.Read(buf)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("second Read = (%d, %v), want (0, io.EOF)", n, err)
	}
	// The filter saw the live bytes (endStream=false) AND a final endStream pass.
	if string(f.seen) != "x" {
		t.Fatalf("filter saw %q, want x", f.seen)
	}
	last := f.ends[len(f.ends)-1]
	if !last {
		t.Fatalf("final replay endStream = %v, want true (the EOF endStream replay)", last)
	}
}

// A non-EOF read error propagates verbatim and does NOT trigger an endStream replay.
func TestReadChainConnNonEOFErrorNoEndStreamReplay(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{f}, &fakeConn{}, connFacts{})
	myErr := errors.New("reset by peer")
	inner := &errConn{err: myErr}
	rc := newReadChainConn(inner, rt)

	buf := make([]byte, 16)
	_, err := rc.Read(buf)
	if !errors.Is(err, myErr) {
		t.Fatalf("Read err = %v, want %v (verbatim propagation)", err, myErr)
	}
	for _, e := range f.ends {
		if e {
			t.Fatal("non-EOF error must NOT trigger an endStream replay")
		}
	}
}

// bytesAndEOFConn returns its payload AND io.EOF in the SAME Read call (the
// io.Reader contract permits this), then (0, io.EOF) forever.
type bytesAndEOFConn struct {
	scriptConn
	payload []byte
	served  bool
}

func (c *bytesAndEOFConn) Read(b []byte) (int, error) {
	if c.served {
		return 0, io.EOF
	}
	c.served = true
	n := copy(b, c.payload)
	return n, io.EOF
}

// A Read returning (n>0, io.EOF) in the same call must replay the bytes
// (endStream=false) AND deliver the final endStream replay AND return both
// n and io.EOF to the caller — the two replay branches are independent, not
// either/or.
func TestReadChainConnBytesAndEOFSameCall(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{f}, &fakeConn{}, connFacts{})
	inner := &bytesAndEOFConn{payload: []byte("tail")}
	rc := newReadChainConn(inner, rt)

	buf := make([]byte, 16)
	n, err := rc.Read(buf)
	if n != 4 || !errors.Is(err, io.EOF) {
		t.Fatalf("Read = (%d, %v), want (4, io.EOF) — both bytes and EOF returned", n, err)
	}
	if string(f.seen) != "tail" {
		t.Fatalf("filter saw %q, want tail (bytes replayed even when EOF arrives in the same call)", f.seen)
	}
	if len(f.ends) != 2 || f.ends[0] != false || f.ends[1] != true {
		t.Fatalf("endStream flags = %v, want [false true] (bytes replay then EOF endStream replay)", f.ends)
	}
}

// A zero-byte read (n==0, err==nil) is NOT replayed (no empty OnData passes).
func TestReadChainConnZeroByteReadNoReplay(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{f}, &fakeConn{}, connFacts{})
	inner := &multiReadConn{payloads: [][]byte{{}}} // one zero-byte "read", then EOF
	rc := newReadChainConn(inner, rt)

	buf := make([]byte, 16)
	n, err := rc.Read(buf)
	if n != 0 || err != nil {
		t.Fatalf("zero-byte Read = (%d, %v), want (0, nil)", n, err)
	}
	if len(f.ends) != 0 {
		t.Fatalf("zero-byte read produced %d replay passes, want 0", len(f.ends))
	}
}
