package helpers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
)

// h3SelfSignedTLS generates a fresh self-signed ECDSA P-256 leaf certificate
// (CN "envoy-go h3 test") and returns a server-side *tls.Config with
// NextProtos=["h3"] for an in-process http3.Server. The client side of these
// tests uses InsecureSkipVerify, so no CA chain is needed.
func h3SelfSignedTLS(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "envoy-go h3 test"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3"},
	}
}

// TestH3RoundTrip_GET stands up a local http3.Server and confirms H3RoundTrip
// completes a pinned-addr GET returning status + body over HTTP/3.
func TestH3RoundTrip_GET(t *testing.T) {
	serverTLS := h3SelfSignedTLS(t)
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer func() { _ = udpConn.Close() }()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("OK\n"))
	})
	srv := &http3.Server{Handler: mux, TLSConfig: serverTLS}
	go func() { _ = srv.Serve(udpConn) }()
	defer func() { _ = srv.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	clientTLS := &tls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true} //nolint:gosec // local test
	status, _, body, err := H3RoundTrip(ctx, udpConn.LocalAddr().String(), clientTLS, http.MethodGet, "/health", nil, nil)
	if err != nil {
		t.Fatalf("H3RoundTrip: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	if string(body) != "OK\n" {
		t.Errorf("body = %q, want OK\\n", string(body))
	}
}
