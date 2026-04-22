package helpers

import (
	"strings"
	"testing"
)

func TestParseHTTPResponse_Simple(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 5\r\nServer: envoy\r\n\r\nLIVE\n")
	r, err := ParseHTTPResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.StatusLine != "HTTP/1.1 200 OK" {
		t.Errorf("status: %q", r.StatusLine)
	}
	if string(r.Body) != "LIVE\n" {
		t.Errorf("body: %q", r.Body)
	}
	if r.Headers["Content-Type"] != "text/plain" {
		t.Errorf("Content-Type: %q", r.Headers["Content-Type"])
	}
	if r.Headers["Server"] != "envoy" {
		t.Errorf("Server: %q", r.Headers["Server"])
	}
}

func TestParseHTTPResponse_MultiValueHeader(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\nX-A: 1\r\nX-A: 2\r\n\r\n")
	r, err := ParseHTTPResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(r.Headers["X-A"], "1") || !strings.Contains(r.Headers["X-A"], "2") {
		t.Errorf("multi-value: %q", r.Headers["X-A"])
	}
}

func TestParseHTTPResponse_Malformed(t *testing.T) {
	_, err := ParseHTTPResponse([]byte("not an http response"))
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
