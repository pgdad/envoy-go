package helpers

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
)

// TCPRoundTrip dials addr, sends payload, half-closes the write side, then
// reads the response until EOF or until idleTimeout elapses with no new bytes.
// The returned slice is the full response stream.
func TCPRoundTrip(ctx context.Context, addr string, payload []byte, idleTimeout time.Duration) ([]byte, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write(payload); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	tcp, ok := conn.(*net.TCPConn)
	if ok {
		_ = tcp.CloseWrite()
	}

	var resp []byte
	buf := make([]byte, 4096)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
		n, err := conn.Read(buf)
		if n > 0 {
			resp = append(resp, buf[:n]...)
		}
		if err == io.EOF {
			return resp, nil
		}
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return resp, nil
		}
		if err != nil {
			return resp, fmt.Errorf("read: %w", err)
		}
		if ctx.Err() != nil {
			return resp, ctx.Err()
		}
	}
}
