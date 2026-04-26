package cluster

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	stdtls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

// h2TestPKI carries an in-memory CA + leaf cert/key pair for the dial_h2
// tests' in-process h2-over-TLS server. The PKI is generated per-test (cheap
// with P-256) rather than cached; tests run in parallel and the overhead is
// negligible.
type h2TestPKI struct {
	caPool      *x509.CertPool
	leafCertPEM []byte
	leafKeyPEM  []byte
}

// mkH2TestPKI builds a CA + a single leaf cert with CN/SAN
// "alpha.envoy-go.test", sufficient for a TLS handshake against a test
// listener on 127.0.0.1 with ServerName="alpha.envoy-go.test".
func mkH2TestPKI(t *testing.T) *h2TestPKI {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "envoy-go h2 test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("ca cert: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "alpha.envoy-go.test"},
		DNSNames:     []string{"alpha.envoy-go.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("leaf cert: %v", err)
	}
	leafCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	leafKeyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatalf("leaf key marshal: %v", err)
	}
	leafKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyDER})

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	return &h2TestPKI{caPool: pool, leafCertPEM: leafCertPEM, leafKeyPEM: leafKeyPEM}
}

// upstreamCfgForTest returns a *stdtls.Config the dialing side uses to verify
// the in-process h2 server's leaf against pki.caPool. NextProtos=["h2"]
// requests h2 via ALPN.
func upstreamCfgForTest(pki *h2TestPKI, alpn []string) *stdtls.Config {
	return &stdtls.Config{
		ServerName: "alpha.envoy-go.test",
		RootCAs:    pki.caPool,
		NextProtos: alpn,
		MinVersion: stdtls.VersionTLS12,
		MaxVersion: stdtls.VersionTLS13,
	}
}

// h2ServerPrefacePeer reads the client preface + client SETTINGS, writes
// the server's initial SETTINGS, reads the client's SETTINGS_ACK, then
// writes its own SETTINGS_ACK. Mirrors h2/client_test.go's
// runFakeServerPeerForClientHandshake but used here as a from-scratch
// driver-side h2 peer over TLS.
//
// Driver-side use of golang.org/x/net/http2.Framer is permitted in test
// code per D-3.2 (which scopes the no-stdlib-http2 rule to RUNTIME code).
//
// Bidirectional ACK ordering (read client's ACK before writing our own)
// avoids a synchronous-write deadlock; RFC 9113 §6.5 imposes no ordering
// between the two independent ACKs.
func h2ServerPrefacePeer(conn net.Conn) error {
	prefaceBuf := make([]byte, 24)
	if _, err := io.ReadFull(conn, prefaceBuf); err != nil {
		return fmt.Errorf("preface: %w", err)
	}
	if string(prefaceBuf) != "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n" {
		return fmt.Errorf("bad preface: %q", prefaceBuf)
	}
	fr := http2.NewFramer(conn, conn)
	// Read client SETTINGS.
	frame, err := fr.ReadFrame()
	if err != nil {
		return fmt.Errorf("read client SETTINGS: %w", err)
	}
	if _, ok := frame.(*http2.SettingsFrame); !ok {
		return fmt.Errorf("expected SETTINGS, got %T", frame)
	}
	// Write server initial SETTINGS.
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16384}); err != nil {
		return fmt.Errorf("write server SETTINGS: %w", err)
	}
	// Read client's SETTINGS_ACK first to avoid the net-pipe-style write
	// deadlock under synchronous writes (real TCP tolerates this; the
	// pattern is preserved for symmetry with the h2/client_test fixture).
	if _, err := fr.ReadFrame(); err != nil {
		return fmt.Errorf("read client SETTINGS_ACK: %w", err)
	}
	// Write SETTINGS_ACK for client's SETTINGS.
	if err := fr.WriteSettingsAck(); err != nil {
		return fmt.Errorf("write SETTINGS_ACK: %w", err)
	}
	return nil
}

// runH2Server accepts conns on ln (a TLS listener) and runs the from-scratch
// driver-side h2 handshake on each, then drains the conn until close.
// Returns when ln is closed.
func runH2Server(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(conn net.Conn) {
			defer func() { _ = conn.Close() }()
			if err := h2ServerPrefacePeer(conn); err != nil {
				return
			}
			// Post-handshake: drain bytes (including the client's GOAWAY on
			// Close) until the client closes the conn.
			_, _ = io.Copy(io.Discard, conn)
		}(c)
	}
}

// listenH2 starts an in-process h2-over-TLS listener with NextProtos:
// `alpn`. Returns the listener (caller must Close).
func listenH2(t *testing.T, pki *h2TestPKI, alpn []string) net.Listener {
	t.Helper()
	pair, err := stdtls.X509KeyPair(pki.leafCertPEM, pki.leafKeyPEM)
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	cfg := &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		NextProtos:   alpn,
		MinVersion:   stdtls.VersionTLS12,
		MaxVersion:   stdtls.VersionTLS13,
	}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}
	go runH2Server(ln)
	return ln
}

// listenALPN starts a TLS listener with the given NextProtos but does NOT
// run an H2 server on it; instead the listener accepts conns, completes the
// TLS handshake, and discards bytes. Sufficient for ALPN-mismatch checks
// where the test only needs the post-handshake ALPN value.
func listenALPN(t *testing.T, pki *h2TestPKI, alpn []string) net.Listener {
	t.Helper()
	pair, err := stdtls.X509KeyPair(pki.leafCertPEM, pki.leafKeyPEM)
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	cfg := &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		NextProtos:   alpn,
		MinVersion:   stdtls.VersionTLS12,
		MaxVersion:   stdtls.VersionTLS13,
	}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer func() { _ = conn.Close() }()
				// Force handshake completion so the client side sees the
				// negotiated ALPN value.
				if tc, ok := conn.(*stdtls.Conn); ok {
					_ = tc.Handshake()
				}
				// Drain inbound bytes so the conn lingers; client closes after
				// the ALPN check fails inside DialH2.
				_, _ = io.Copy(io.Discard, conn)
			}(c)
		}
	}()
	return ln
}

// listenTLSCloseOnAccept starts a TCP listener that accepts a connection then
// closes it immediately, before any TLS handshake bytes are exchanged. The
// dialing client's HandshakeContext returns an EOF or "connection reset"
// style error. Used for the TLS-handshake-failure case.
func listenTLSCloseOnAccept(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return ln
}

// ---------------------------------------------------------------------------
// DialH2 — happy path
// ---------------------------------------------------------------------------

// TestCluster_DialH2_HappyPath verifies DialH2 against an in-process h2
// backend negotiating ALPN h2. Asserts a non-nil *h2.ClientConn and nil err.
func TestCluster_DialH2_HappyPath(t *testing.T) {
	pki := mkH2TestPKI(t)
	ln := listenH2(t, pki, []string{"h2"})
	defer func() { _ = ln.Close() }()

	upCfg := upstreamCfgForTest(pki, []string{"h2"})
	ep := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test-h2", upCfg, ep)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cc, err := c.DialH2(ctx)
	if err != nil {
		t.Fatalf("DialH2: %v", err)
	}
	if cc == nil {
		t.Fatal("DialH2 returned nil ClientConn with nil err")
	}
	defer func() { _ = cc.Close() }()
}

// ---------------------------------------------------------------------------
// DialH2 — ALPN mismatch
// ---------------------------------------------------------------------------

// TestCluster_DialH2_ALPNMismatch verifies DialH2 errors when the backend
// negotiates http/1.1 instead of h2.
func TestCluster_DialH2_ALPNMismatch(t *testing.T) {
	pki := mkH2TestPKI(t)
	ln := listenALPN(t, pki, []string{"http/1.1"})
	defer func() { _ = ln.Close() }()

	// Client offers both h2 (preferred) and http/1.1; server only supports
	// http/1.1, so the negotiated value is "http/1.1".
	upCfg := upstreamCfgForTest(pki, []string{"h2", "http/1.1"})
	ep := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test-alpn-mismatch", upCfg, ep)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.DialH2(ctx)
	if err == nil {
		t.Fatal("DialH2: want error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "alpn negotiated") {
		t.Errorf("err = %q, want substring %q", msg, "alpn negotiated")
	}
	if !strings.Contains(msg, `want "h2"`) {
		t.Errorf("err = %q, want substring %q", msg, `want "h2"`)
	}
}

// ---------------------------------------------------------------------------
// DialH2 — not a TLS conn
// ---------------------------------------------------------------------------

// TestCluster_DialH2_NotTLS verifies DialH2 errors when Cluster.Dial returns
// a plain net.Conn (no upstream TLS configured).
func TestCluster_DialH2_NotTLS(t *testing.T) {
	ln := listenTCP(t)
	defer func() { _ = ln.Close() }()

	ep := endpointFromAddr(ln.Addr())
	// upstreamCfg=nil → Cluster.Dial returns the raw TCP conn.
	c := mkTestCluster("test-not-tls", nil, ep)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.DialH2(ctx)
	if err == nil {
		t.Fatal("DialH2: want error, got nil")
	}
	if !strings.Contains(err.Error(), "not a TLS conn") {
		t.Errorf("err = %q, want substring %q", err.Error(), "not a TLS conn")
	}
}

// ---------------------------------------------------------------------------
// DialH2 — context canceled before dial completes
// ---------------------------------------------------------------------------

// TestCluster_DialH2_CtxCancel verifies that a canceled ctx propagates
// through Cluster.Dial → DialH2.
func TestCluster_DialH2_CtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call

	ep := Endpoint{Host: "127.0.0.1", Port: 1} // unreachable, but Dial short-circuits on ctx.Err()
	c := mkTestCluster("test-ctx-cancel", nil, ep)

	_, err := c.DialH2(ctx)
	if err == nil {
		t.Fatal("DialH2: want ctx error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want errors.Is(err, context.Canceled)", err)
	}
}

// ---------------------------------------------------------------------------
// DialH2 — TLS handshake failure
// ---------------------------------------------------------------------------

// TestCluster_DialH2_TLSHandshakeFailure verifies that a backend which
// closes its TCP conn before completing the TLS handshake produces a
// handshake error that bubbles through Cluster.Dial → DialH2.
func TestCluster_DialH2_TLSHandshakeFailure(t *testing.T) {
	pki := mkH2TestPKI(t)
	ln := listenTLSCloseOnAccept(t)
	defer func() { _ = ln.Close() }()

	upCfg := upstreamCfgForTest(pki, []string{"h2"})
	ep := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test-handshake-fail", upCfg, ep)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.DialH2(ctx)
	if err == nil {
		t.Fatal("DialH2: want handshake error, got nil")
	}
	// Cluster.Dial wraps with "cluster: tls: handshake:"; DialH2 wraps that
	// with "cluster: dial h2:". Either substring is acceptable evidence the
	// handshake error bubbled.
	msg := err.Error()
	if !strings.Contains(msg, "handshake") && !strings.Contains(msg, "tls") {
		t.Errorf("err = %q, expected handshake/tls error chain", msg)
	}
}
