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
	return tcpRoundTrip(ctx, addr, payload, idleTimeout, true)
}

// TCPRoundTripNoHalfClose behaves like TCPRoundTrip but does NOT half-close the
// write side after sending payload; it relies solely on idleTimeout to bound
// the read. This avoids a downstream FIN racing the proxy's echo write: when a
// client half-closes immediately after writing, the simultaneous data+FIN can
// cause reference Envoy v1.37.2's echo network filter to tear the connection
// down before flushing the echoed bytes back (characterized in fixture
// 0040-network-echo: a FIN co-arriving with the payload yields zero echoed
// bytes, while ANY gap or no FIN yields a full echo). Leaving the write side
// open and draining on the idle timeout is byte-exact on BOTH the reference and
// the subject for a pure-echo (read-only) filter that never half-closes its own
// write side.
func TCPRoundTripNoHalfClose(ctx context.Context, addr string, payload []byte, idleTimeout time.Duration) ([]byte, error) {
	return tcpRoundTrip(ctx, addr, payload, idleTimeout, false)
}

func tcpRoundTrip(ctx context.Context, addr string, payload []byte, idleTimeout time.Duration, halfClose bool) ([]byte, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write(payload); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	if halfClose {
		tcp, ok := conn.(*net.TCPConn)
		if ok {
			_ = tcp.CloseWrite()
		}
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
