package helpers

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"time"
)

// TLSRoundTrip dials addr over TCP, establishes a TLS 1.2+/1.3 client
// handshake using rootCAs + serverName, writes payload, half-closes by
// sending a close_notify alert + TCP FIN, then reads until EOF or idleTimeout
// elapses. Returns all received bytes.
//
// Shape mirrors helpers.TCPRoundTrip (phase 02) with the single addition of
// the TLS wrap + handshake + SNI.
func TLSRoundTrip(ctx context.Context, addr, serverName string, rootCAs *x509.CertPool, payload []byte, idleTimeout time.Duration) ([]byte, error) {
	d := &net.Dialer{}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tls round trip: dial: %w", err)
	}
	cfg := &stdtls.Config{
		ServerName: serverName,
		RootCAs:    rootCAs,
		MinVersion: stdtls.VersionTLS12,
		MaxVersion: stdtls.VersionTLS13,
	}
	conn := stdtls.Client(raw, cfg)
	defer func() { _ = conn.Close() }()
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("tls round trip: handshake: %w", err)
	}
	if _, err := conn.Write(payload); err != nil {
		return nil, fmt.Errorf("tls round trip: write: %w", err)
	}
	if err := conn.CloseWrite(); err != nil {
		return nil, fmt.Errorf("tls round trip: close_write: %w", err)
	}
	if idleTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
	}
	got, err := io.ReadAll(conn)
	if err != nil {
		// Read deadline hit is surfaced as net.Error Timeout(); distinguish
		// from genuine read errors by checking err's Timeout method.
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return got, nil
		}
		return got, fmt.Errorf("tls round trip: read: %w", err)
	}
	return got, nil
}
