package admin

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestServer_ReadyState(t *testing.T) {
	s := New("127.0.0.1:0", stats.NewRegistry())
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

func TestServer_PreInit_BeforeMarkReady(t *testing.T) {
	s := New("127.0.0.1:0", stats.NewRegistry())
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)
	// No MarkReady call. /ready should return the pre-init response per
	// ADR-0015.
	resp, err := http.Get("http://" + addr + "/ready")
	if err != nil {
		t.Fatalf("GET /ready: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 200 {
		t.Errorf("pre-init status: got 200, want non-200 per ADR-0015")
	}
	// Body must match whatever ADR-0015 locks. Task 8 chose option (b):
	// PRE_INITIALIZING\n with 503 Service Unavailable.
	body, _ := io.ReadAll(resp.Body)
	if string(body) == "LIVE\n" {
		t.Errorf("pre-init body must not be LIVE\\n (would collide with ready state)")
	}
}

func TestServer_MarkReady_IsAtomic(t *testing.T) {
	s := New("127.0.0.1:0", stats.NewRegistry())
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			resp, _ := http.Get("http://" + addr + "/ready")
			if resp != nil {
				_ = resp.Body.Close()
			}
		}
		close(done)
	}()
	s.MarkReady()
	<-done
	// Final probe should be 200/LIVE.
	resp, _ := http.Get("http://" + addr + "/ready")
	if resp == nil {
		t.Fatal("final probe: nil response")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("final status: got %d, want 200", resp.StatusCode)
	}
}

func TestServer_Close_Idempotent(t *testing.T) {
	s := New("127.0.0.1:0", stats.NewRegistry())
	// Close before Start.
	if err := s.Close(); err != nil {
		t.Errorf("Close before Start: %v", err)
	}
	// Close after Start.
	s2 := New("127.0.0.1:0", stats.NewRegistry())
	_, err := s2.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s2.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	// Second Close.
	if err := s2.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestServer_StatsPrometheusRouteRegistered(t *testing.T) {
	r := stats.NewRegistry()
	srv := New("127.0.0.1:0", r)
	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = srv.Close() }()
	resp, err := http.Get("http://" + addr + "/stats/prometheus")
	if err != nil {
		t.Fatalf("GET /stats/prometheus: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestServer_LiveGaugeSetOnceFlippedAtFirstReady200(t *testing.T) {
	r := stats.NewRegistry()
	srv := New("127.0.0.1:0", r)
	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = srv.Close() }()
	// Give the accept goroutine a beat.
	time.Sleep(10 * time.Millisecond)
	// Initially: server.live == 0, /ready returns 503.
	resp503, err := http.Get("http://" + addr + "/ready")
	if err != nil {
		t.Fatalf("GET /ready (pre-MarkReady): %v", err)
	}
	if resp503 != nil {
		_ = resp503.Body.Close()
	}
	if got := srv.liveGauge.Load(); got != 0 {
		t.Errorf("server.live before MarkReady = %d, want 0", got)
	}
	srv.MarkReady()
	for i := 0; i < 3; i++ {
		resp, err := http.Get("http://" + addr + "/ready")
		if err != nil {
			t.Fatalf("GET /ready (post-MarkReady, iter %d): %v", i, err)
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
	if got := srv.liveGauge.Load(); got != 1 {
		t.Errorf("server.live after MarkReady + 3× /ready = %d, want 1", got)
	}
}
