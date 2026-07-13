package helpers

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// loadPKIFile reads a PEM file from test/fixtures/0002-tls-tcp/pki/. Called
// from every subtest to avoid tying the package's init to a fixture.
func loadPKIFile(t *testing.T, name string) []byte {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(thisFile), "..", "fixtures", "0002-tls-tcp", "pki", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("pki read: %v", err)
	}
	return b
}

func TestTLSRoundTrip_Echo(t *testing.T) {
	caPEM := loadPKIFile(t, "ca.pem")
	certPEM := loadPKIFile(t, "upstream-alpha.pem")
	keyPEM := loadPKIFile(t, "upstream-alpha.key.pem")

	pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	srvCfg := &stdtls.Config{Certificates: []stdtls.Certificate{pair}, MinVersion: stdtls.VersionTLS12}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", srvCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(conn, conn) // echo until half-close/EOF
	}()

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("append ca")
	}
	addr := ln.Addr().String()
	got, err := TLSRoundTrip(context.Background(), addr, "alpha.envoy-go.test", pool, []byte("hello"), 2*time.Second)
	if err != nil {
		t.Fatalf("TLSRoundTrip: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
	_ = ln.Close()
	wg.Wait()
}

func TestTLSRoundTrip_WrongSNI(t *testing.T) {
	caPEM := loadPKIFile(t, "ca.pem")
	certPEM := loadPKIFile(t, "upstream-alpha.pem")
	keyPEM := loadPKIFile(t, "upstream-alpha.key.pem")
	pair, _ := stdtls.X509KeyPair(certPEM, keyPEM)
	srvCfg := &stdtls.Config{Certificates: []stdtls.Certificate{pair}, MinVersion: stdtls.VersionTLS12}
	ln, _ := stdtls.Listen("tcp", "127.0.0.1:0", srvCfg)
	defer func() { _ = ln.Close() }()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.(*stdtls.Conn).Handshake() //nolint:forcetypeassert
	}()

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	_, err := TLSRoundTrip(context.Background(), ln.Addr().String(), "beta.envoy-go.test", pool, []byte("x"), 500*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "x509") {
		t.Errorf("want x509 verify error, got: %v", err)
	}
}

// TestTLSServedLeaf_ReturnsServerCert proves TLSServedLeaf performs a
// handshake-only (no application I/O) dial and returns the leaf certificate
// the server actually presented, identified by comparing SerialNumber
// against the known upstream-alpha.pem fixture leaf.
func TestTLSServedLeaf_ReturnsServerCert(t *testing.T) {
	caPEM := loadPKIFile(t, "ca.pem")
	certPEM := loadPKIFile(t, "upstream-alpha.pem")
	keyPEM := loadPKIFile(t, "upstream-alpha.key.pem")

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("pem.Decode(upstream-alpha.pem): nil block")
	}
	wantLeaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}

	pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	srvCfg := &stdtls.Config{Certificates: []stdtls.Certificate{pair}, MinVersion: stdtls.VersionTLS12}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", srvCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.(*stdtls.Conn).Handshake() //nolint:forcetypeassert
	}()

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("append ca")
	}

	got, err := TLSServedLeaf(context.Background(), ln.Addr().String(), "alpha.envoy-go.test", pool)
	if err != nil {
		t.Fatalf("TLSServedLeaf: %v", err)
	}
	if got == nil {
		t.Fatal("TLSServedLeaf: got nil cert, nil error")
	}
	if got.SerialNumber.Cmp(wantLeaf.SerialNumber) != 0 {
		t.Errorf("SerialNumber: got %v, want %v", got.SerialNumber, wantLeaf.SerialNumber)
	}
}

func TestTLSServedLeaf_DialFailure(t *testing.T) {
	pool := x509.NewCertPool()
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	_ = l.Close()

	_, err := TLSServedLeaf(context.Background(), addr, "alpha.envoy-go.test", pool)
	if err == nil {
		t.Error("want dial error")
	}
}

func TestTLSRoundTrip_DialFailure(t *testing.T) {
	pool := x509.NewCertPool()
	// closed address: bind-and-close to get a reliably-refused address.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	_ = l.Close()

	_, err := TLSRoundTrip(context.Background(), addr, "alpha.envoy-go.test", pool, []byte("x"), 200*time.Millisecond)
	if err == nil {
		t.Error("want dial error")
	}
}
