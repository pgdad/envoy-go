package network

import (
	"io"
	"testing"
)

func TestPrefixConnReplaysPrefixThenLive(t *testing.T) {
	pc := newPrefixConn(scriptedConn([]byte("LIVE")), []byte("PREFIX"))
	got, _ := io.ReadAll(pc)
	if string(got) != "PREFIXLIVE" {
		t.Fatalf("prefixConn read %q, want PREFIXLIVE", got)
	}
}

func TestPrefixConnEmptyPrefixIsPassthrough(t *testing.T) {
	pc := newPrefixConn(scriptedConn([]byte("X")), nil)
	got, _ := io.ReadAll(pc)
	if string(got) != "X" {
		t.Fatalf("empty-prefix passthrough broken: %q", got)
	}
}

func TestPrefixConnSmallBufferReadsAcrossBoundary(t *testing.T) {
	// b smaller than the prefix forces multiple Reads to drain the prefix one
	// chunk at a time, then the live tail — proving p.prefix = p.prefix[n:]
	// advances correctly across the prefix/live boundary. io.ReadAll's >=512B
	// buffer would swallow the prefix in one Read and never exercise this path.
	pc := newPrefixConn(scriptedConn([]byte("LIVE")), []byte("PREFIX"))
	var got []byte
	// Buffer is smaller than the 6-byte prefix (forces multiple prefix Reads,
	// advancing p.prefix = p.prefix[n:]) but >= the 4-byte live tail, since
	// scriptConn delivers the whole live payload in its single Read.
	b := make([]byte, 4)
	// scriptConn returns (n, nil) for the live tail and (0, io.EOF) afterward, so
	// the loop breaks on the EOF Read. The bound guards against a runaway loop if
	// that contract ever regresses.
	for i := 0; i < 100; i++ {
		n, err := pc.Read(b)
		got = append(got, b[:n]...)
		if err != nil { // scriptedConn returns io.EOF after the live bytes
			break
		}
	}
	if string(got) != "PREFIXLIVE" {
		t.Fatalf("small-buffer read across boundary = %q, want PREFIXLIVE", got)
	}
}
