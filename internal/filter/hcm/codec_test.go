package hcm

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServerHeader(t *testing.T) {
	if got := serverHeader(); got != "envoy" {
		t.Errorf("serverHeader() = %q, want %q", got, "envoy")
	}
}

func TestDateHeader(t *testing.T) {
	got := dateHeader()
	if _, err := time.Parse(http.TimeFormat, got); err != nil {
		t.Errorf("dateHeader() = %q is not RFC 7231 IMF-fixdate parseable: %v", got, err)
	}
}

func TestWriteStatusReply(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus string
		wantCLen   string
	}{
		{"200 OK with body", 200, "OK\n", "HTTP/1.1 200 OK\r\n", "Content-Length: 3\r\n"},
		{"400 Bad Request empty body", 400, "", "HTTP/1.1 400 Bad Request\r\n", "Content-Length: 0\r\n"},
		{"404 Not Found", 404, "not found\n", "HTTP/1.1 404 Not Found\r\n", "Content-Length: 10\r\n"},
		{"417 Expectation Failed empty", 417, "", "HTTP/1.1 417 Expectation Failed\r\n", "Content-Length: 0\r\n"},
		{"500 Internal Server Error empty", 500, "", "HTTP/1.1 500 Internal Server Error\r\n", "Content-Length: 0\r\n"},
		{"502 Bad Gateway empty", 502, "", "HTTP/1.1 502 Bad Gateway\r\n", "Content-Length: 0\r\n"},
		{"503 Service Unavailable empty", 503, "", "HTTP/1.1 503 Service Unavailable\r\n", "Content-Length: 0\r\n"},
		{"501 Not Implemented empty", 501, "", "HTTP/1.1 501 Not Implemented\r\n", "Content-Length: 0\r\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeStatusReply(&buf, tc.status, tc.body); err != nil {
				t.Fatalf("writeStatusReply: %v", err)
			}
			out := buf.String()
			if !strings.HasPrefix(out, tc.wantStatus) {
				t.Errorf("status line:\n  got:  %q\n  want prefix: %q", out, tc.wantStatus)
			}
			if !strings.Contains(out, tc.wantCLen) {
				t.Errorf("missing %q in:\n%s", tc.wantCLen, out)
			}
			if !strings.Contains(out, "Server: envoy\r\n") {
				t.Errorf("missing Server header in:\n%s", out)
			}
			if !strings.Contains(out, "Content-Type: text/plain\r\n") {
				t.Errorf("missing Content-Type header in:\n%s", out)
			}
			if !strings.Contains(out, "Date: ") {
				t.Errorf("missing Date header in:\n%s", out)
			}
			if tc.body != "" {
				idx := strings.Index(out, "\r\n\r\n")
				if idx < 0 || out[idx+4:] != tc.body {
					t.Errorf("body mismatch: got %q, want %q", out[idx+4:], tc.body)
				}
			}
		})
	}
}

func TestWriteStatusReply_UnknownStatusFallsBackToEmptyReason(t *testing.T) {
	var buf bytes.Buffer
	if err := writeStatusReply(&buf, 999, ""); err != nil {
		t.Fatalf("writeStatusReply: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "HTTP/1.1 999 \r\n") {
		t.Errorf("expected empty reason phrase for unknown status, got:\n%s", out)
	}
}
