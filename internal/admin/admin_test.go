package admin

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServer_ReadyState(t *testing.T) {
	s := New("127.0.0.1:0")
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	s.MarkReady()

	// Give the accept goroutine a beat.
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/ready")
	if err != nil {
		t.Fatalf("GET /ready: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "LIVE\n" {
		t.Errorf("body: got %q, want %q", body, "LIVE\n")
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain; charset=UTF-8" {
		t.Errorf("Content-Type: got %q, want %q", ct, "text/plain; charset=UTF-8")
	}
	if cl := resp.Header.Get("Content-Length"); cl != "5" {
		t.Errorf("Content-Length: got %q, want %q", cl, "5")
	}
	if srv := resp.Header.Get("Server"); srv != "envoy" {
		t.Errorf("Server: got %q, want %q", srv, "envoy")
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache, max-age=0" {
		t.Errorf("Cache-Control: got %q, want %q", cc, "no-cache, max-age=0")
	}
	if xc := resp.Header.Get("X-Content-Type-Options"); xc != "nosniff" {
		t.Errorf("X-Content-Type-Options: got %q, want %q", xc, "nosniff")
	}
}

// freeAddr returns a host:port string for a port that is currently free. Used
// by Task 9's concurrency tests; declared here per PLAN Step 3 so that Task 9
// does not re-introduce the same helper.
//
//nolint:unused // consumed by TestServer_ConcurrentReady in Task 9.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free addr: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
}
