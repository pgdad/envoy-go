package helpers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/http2"
)

// TestH2RoundTrip_HappyPath verifies the helper against an in-process h2-over-TLS server.
func TestH2RoundTrip_HappyPath(t *testing.T) {
	// Stand up an httptest.NewUnstartedServer with TLS + http2.ConfigureServer
	// driver-side (test code, not runtime). Handler returns "ok" with status 200.
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	if err := http2.ConfigureServer(srv.Config, &http2.Server{}); err != nil {
		t.Fatalf("ConfigureServer: %v", err)
	}
	srv.TLS = &tls.Config{NextProtos: []string{"h2"}}
	srv.StartTLS()
	defer srv.Close()

	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	tlsConf := &tls.Config{RootCAs: pool, NextProtos: []string{"h2"}, ServerName: "127.0.0.1"}

	addr := srv.Listener.Addr().String()
	status, _, body, err := H2RoundTrip(context.Background(), addr, tlsConf, "GET", "/", nil, nil)
	if err != nil {
		t.Fatalf("H2RoundTrip: %v", err)
	}
	if status != 200 {
		t.Errorf("status: got %d, want 200", status)
	}
	if string(body) != "ok" {
		t.Errorf("body: got %q, want %q", body, "ok")
	}
}
