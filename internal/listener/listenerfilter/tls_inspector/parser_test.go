package tls_inspector

import (
	"crypto/tls"
	"net"
	"testing"
)

// captureClientHello uses a real TLS handshake against a pipe to capture
// the ClientHello bytes. The resulting buffer is the verbatim handshake
// record envoy-go's parser must accept.
func captureClientHello(t *testing.T, sni string, alpns []string) []byte {
	t.Helper()
	cli, srv := net.Pipe()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	// Read the first record on the server side.
	recv := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := srv.Read(buf)
		recv <- buf[:n]
	}()
	go func() {
		c := tls.Client(cli, &tls.Config{ServerName: sni, NextProtos: alpns, InsecureSkipVerify: true})
		_ = c.Handshake() // expected to fail (server doesn't respond); we only need ClientHello bytes
	}()
	return <-recv
}

func TestParseClientHelloWithSNIAndALPN(t *testing.T) {
	buf := captureClientHello(t, "foo.example.test", []string{"h2", "http/1.1"})
	sni, alpns, ok := parseClientHello(buf)
	if !ok {
		t.Fatalf("parseClientHello: ok=false on real ClientHello")
	}
	if sni != "foo.example.test" {
		t.Errorf("SNI: got %q, want \"foo.example.test\"", sni)
	}
	if len(alpns) != 2 || alpns[0] != "h2" || alpns[1] != "http/1.1" {
		t.Errorf("ALPN: got %v, want [h2, http/1.1]", alpns)
	}
}

func TestParseClientHelloSNIOnly(t *testing.T) {
	buf := captureClientHello(t, "foo.example.test", nil)
	sni, alpns, ok := parseClientHello(buf)
	if !ok {
		t.Fatalf("parseClientHello: ok=false on SNI-only ClientHello")
	}
	if sni != "foo.example.test" {
		t.Errorf("SNI: got %q", sni)
	}
	if len(alpns) != 0 {
		t.Errorf("ALPN: got %v, want empty", alpns)
	}
}

func TestParseClientHelloALPNOnly(t *testing.T) {
	buf := captureClientHello(t, "", []string{"h2"})
	sni, alpns, ok := parseClientHello(buf)
	if !ok {
		t.Fatalf("parseClientHello: ok=false on ALPN-only ClientHello")
	}
	if sni != "" {
		t.Errorf("SNI: got %q, want \"\"", sni)
	}
	if len(alpns) != 1 || alpns[0] != "h2" {
		t.Errorf("ALPN: got %v, want [h2]", alpns)
	}
}

func TestParseClientHelloEmpty(t *testing.T) {
	_, _, ok := parseClientHello(nil)
	if ok {
		t.Errorf("parseClientHello(nil): ok=true; want false")
	}
}

func TestParseClientHelloNonTLSPreamble(t *testing.T) {
	buf := []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n")
	_, _, ok := parseClientHello(buf)
	if ok {
		t.Errorf("parseClientHello(non-TLS): ok=true; want false")
	}
}

func TestParseClientHelloTruncated(t *testing.T) {
	buf := captureClientHello(t, "foo.example.test", nil)
	for cut := 1; cut < len(buf) && cut < 50; cut++ {
		_, _, ok := parseClientHello(buf[:cut])
		if ok {
			t.Errorf("parseClientHello(truncated to %d): ok=true; want false", cut)
		}
	}
}

func TestParseClientHelloMalformedLengthPrefix(t *testing.T) {
	// TLS record header: 0x16 (handshake) 0x03 0x03 (TLS 1.2) 0xFF 0xFF (length)
	// followed by no body. Should return ok=false without panic.
	buf := []byte{0x16, 0x03, 0x03, 0xFF, 0xFF, 0x01, 0x00}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("parseClientHello panicked: %v", r)
		}
	}()
	_, _, _ = parseClientHello(buf)
}
