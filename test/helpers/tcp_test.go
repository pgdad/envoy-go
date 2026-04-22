package helpers

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestTCPRoundTrip_EchoBackend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		buf := make([]byte, 1024)
		n, _ := c.Read(buf)
		_, _ = c.Write(buf[:n])
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := TCPRoundTrip(ctx, ln.Addr().String(), []byte("hello"), 200*time.Millisecond)
	if err != nil {
		t.Fatalf("TCPRoundTrip: %v", err)
	}
	if string(resp) != "hello" {
		t.Errorf("got %q, want %q", resp, "hello")
	}
}
