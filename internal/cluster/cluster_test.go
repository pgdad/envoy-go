package cluster

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// echoConn reads bytes from c and writes them back until the connection closes.
// Uses an explicit read/write loop rather than io.Copy to avoid the Linux
// splice-on-loopback deadlock when src == dst is a *net.TCPConn.
func echoConn(c net.Conn) {
	defer func() { _ = c.Close() }()
	buf := make([]byte, 4096)
	for {
		n, err := c.Read(buf)
		if n > 0 {
			_, _ = c.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// mkTestCluster builds a minimal Cluster for unit tests, bypassing manager
// validation. eps must have at least one entry.
func mkTestCluster(name string, upstreamCfg *stdtls.Config, eps ...Endpoint) *Cluster {
	return &Cluster{
		name:           name,
		connectTimeout: time.Second,
		endpoints:      eps,
		lb:             &roundRobin{endpoints: eps},
		upstreamCfg:    upstreamCfg,
	}
}

// listenTCP starts a plaintext TCP echo server on a random loopback port and
// returns the listener. The caller is responsible for closing it.
func listenTCP(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go echoConn(c)
		}
	}()
	return ln
}

// listenTLS starts a TLS echo server on a random loopback port and returns
// the listener. The caller is responsible for closing it.
func listenTLS(t *testing.T, cfg *stdtls.Config) net.Listener {
	t.Helper()
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go echoConn(c)
		}
	}()
	return ln
}

// endpointFromAddr parses a "host:port" net.Addr into an Endpoint.
func endpointFromAddr(addr net.Addr) Endpoint {
	host, portStr, _ := net.SplitHostPort(addr.String())
	port := uint32(0)
	for _, b := range portStr {
		port = port*10 + uint32(b-'0')
	}
	return Endpoint{Host: host, Port: port}
}

// ---------------------------------------------------------------------------
// Dial — plaintext
// ---------------------------------------------------------------------------

func TestCluster_Dial_Plaintext(t *testing.T) {
	ln := listenTCP(t)
	defer func() { _ = ln.Close() }()

	ep := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test", nil, ep)

	conn, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, _ = conn.Write([]byte("ping"))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "ping" {
		t.Errorf("got %q, want %q", buf, "ping")
	}
}

// ---------------------------------------------------------------------------
// Dial — TLS
// ---------------------------------------------------------------------------

func TestCluster_Dial_TLS(t *testing.T) {
	caPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}
	certPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.pem")
	if err != nil {
		t.Fatalf("read upstream-alpha.pem: %v", err)
	}
	keyPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.key.pem")
	if err != nil {
		t.Fatalf("read upstream-alpha.key.pem: %v", err)
	}

	pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	ln := listenTLS(t, &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		MinVersion:   stdtls.VersionTLS12,
	})
	defer func() { _ = ln.Close() }()

	// Build upstream *stdtls.Config against this server.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	upCfg := &stdtls.Config{
		ServerName: "alpha.envoy-go.test",
		RootCAs:    pool,
		MinVersion: stdtls.VersionTLS12,
		MaxVersion: stdtls.VersionTLS13,
	}

	ep := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test-tls", upCfg, ep)

	conn, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, ok := conn.(*stdtls.Conn); !ok {
		t.Errorf("want *stdtls.Conn, got %T", conn)
	}

	_, _ = conn.Write([]byte("secret"))
	buf := make([]byte, 6)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "secret" {
		t.Errorf("got %q, want %q", buf, "secret")
	}
}

// ---------------------------------------------------------------------------
// Dial — TLS handshake failure
// ---------------------------------------------------------------------------

func TestCluster_Dial_TLS_HandshakeFailure(t *testing.T) {
	certPEM, _ := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.pem")
	keyPEM, _ := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.key.pem")
	pair, _ := stdtls.X509KeyPair(certPEM, keyPEM)

	ln := listenTLS(t, &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		MinVersion:   stdtls.VersionTLS12,
	})
	defer func() { _ = ln.Close() }()

	// Upstream config with an empty cert pool — handshake fails.
	ep := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test-bad-ca", &stdtls.Config{
		ServerName: "alpha.envoy-go.test",
		RootCAs:    x509.NewCertPool(),
		MinVersion: stdtls.VersionTLS12,
	}, ep)

	_, err := c.Dial(context.Background())
	if err == nil || !strings.HasPrefix(err.Error(), "cluster: tls: handshake:") {
		t.Errorf("want cluster: tls: handshake: prefix, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Dial — context already canceled
// ---------------------------------------------------------------------------

func TestCluster_Dial_CtxCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ep := Endpoint{Host: "127.0.0.1", Port: 1} // unreachable
	c := mkTestCluster("test", nil, ep)

	_, err := c.Dial(ctx)
	if err == nil {
		t.Error("want ctx error")
	}
}
