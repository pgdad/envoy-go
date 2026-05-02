package tls_inspector

import (
	"crypto/tls"
	"net"
	"testing"
)

// captureClientHelloBytes runs a real crypto/tls.Client against a net.Pipe
// and returns the verbatim ClientHello bytes the client emits on its first
// frame send. Mirrors parser_test.go's captureClientHello helper —
// kept here to avoid a cross-file _test dependency.
func captureClientHelloBytes(t *testing.T, sni string, alpns []string) []byte {
	t.Helper()
	cli, srv := net.Pipe()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	recv := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := srv.Read(buf)
		recv <- buf[:n]
	}()
	go func() {
		c := tls.Client(cli, &tls.Config{
			ServerName:         sni,
			NextProtos:         alpns,
			InsecureSkipVerify: true, //nolint:gosec // test-only; we only need the ClientHello on the wire.
		})
		_ = c.Handshake()
	}()
	return <-recv
}
