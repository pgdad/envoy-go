package listenerfilter

import (
	"bytes"
	"io"
	"net"
	"testing"
)

// pipeConn is a net.Conn backed by an in-memory pipe. Useful for unit-
// testing peekerConn without binding sockets.
//
//nolint:unused // PLAN-verbatim test scaffolding; reserved for future peekerConn tests.
type pipeConn struct{ net.Conn }

func newPipePair() (client, server net.Conn) {
	client, server = net.Pipe()
	return
}

func TestPeekerConnPeekDoesNotConsume(t *testing.T) {
	cli, srv := newPipePair()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	go func() { _, _ = cli.Write([]byte("HELLO_WORLD")); _ = cli.Close() }()
	pc := NewPeekerConn(srv)
	peeker := AsPeeker(pc)
	if peeker == nil {
		t.Fatal("AsPeeker returned nil; expected non-nil")
	}
	first5, err := peeker.Peek(5)
	if err != nil {
		t.Fatalf("Peek(5): %v", err)
	}
	if !bytes.Equal(first5, []byte("HELLO")) {
		t.Errorf("Peek(5)=%q, want %q", first5, "HELLO")
	}
	// Subsequent Read returns the same bytes (peek didn't consume).
	buf := make([]byte, 11)
	n, err := io.ReadFull(pc, buf)
	if err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if n != 11 || !bytes.Equal(buf, []byte("HELLO_WORLD")) {
		t.Errorf("ReadFull: got %d bytes %q, want 11 %q", n, buf, "HELLO_WORLD")
	}
}

func TestPeekerConnPeekBeyondBuffer(t *testing.T) {
	cli, srv := newPipePair()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	go func() { _, _ = cli.Write(make([]byte, 5000)); _ = cli.Close() }()
	pc := NewPeekerConnSize(srv, 256)
	peeker := AsPeeker(pc)
	_, err := peeker.Peek(257)
	if err == nil {
		t.Errorf("Peek(257) on 256-byte buffer; want bufio.ErrBufferFull, got nil")
	}
}

func TestNewPeekerConnSizeClamps(t *testing.T) {
	cli, srv := newPipePair()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	// size=100 clamps to 256.
	pc := NewPeekerConnSize(srv, 100)
	go func() { _, _ = cli.Write(make([]byte, 256)); _ = cli.Close() }()
	peeker := AsPeeker(pc)
	_, err := peeker.Peek(256)
	if err != nil {
		t.Errorf("Peek(256) after clamp-to-256; got %v, want nil", err)
	}
}
