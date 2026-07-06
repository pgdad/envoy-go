package admin

import (
	"net/http"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/internal/stats"
)

// TestServer_StatsRoute_Returns200WithConstantHeaders pins the uniform
// admin-header invariant (§11.6 + ADR-0014) for the flat /stats endpoint
// added in phase 32.1 Task 14: like the four 08.1 endpoints, /stats must
// emit Content-Type plus the three constant headers (Cache-Control,
// X-Content-Type-Options, Server) via the shared writeAdminHeaders helper.
// The header set must be correct regardless of body, so we register one
// counter to give the flat surface at least one line. Assertion style
// mirrors TestAdmin_AllFourEndpointsReturn200WithCorrectHeaders.
func TestServer_StatsRoute_Returns200WithConstantHeaders(t *testing.T) {
	r := stats.NewRegistry()
	r.NewCounter("redis.egress.upstream_cx_total").Inc()

	s := New("127.0.0.1:0", r, nil, nil, nil, nil)
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(20 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/stats")
	if err != nil {
		t.Fatalf("GET /stats: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type: got %q, want %q", got, "text/plain; charset=utf-8")
	}
	// Three constant headers from §11.6 (via writeAdminHeaders).
	for _, h := range []struct{ key, want string }{
		{"Cache-Control", "no-cache, max-age=0"},
		{"X-Content-Type-Options", "nosniff"},
		{"Server", "envoy"},
	} {
		if got := resp.Header.Get(h.key); got != h.want {
			t.Errorf("header %q: got %q, want %q", h.key, got, h.want)
		}
	}
}
