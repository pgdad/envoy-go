package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

func TestHandlePrometheus_ContentType(t *testing.T) {
	r := stats.NewRegistry()
	c := r.NewCounter("server.live")
	c.Inc()
	h := handlePrometheus(r)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/prometheus", nil)
	h.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	if got, want := rec.Header().Get("Content-Type"), "text/plain; version=0.0.4; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
}

func TestHandlePrometheus_EmptyRegistry(t *testing.T) {
	r := stats.NewRegistry()
	h := handlePrometheus(r)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/prometheus", nil)
	h.ServeHTTP(rec, req)
	if got := rec.Code; got != http.StatusOK {
		t.Errorf("status = %d, want 200 (empty registry → empty body, still 200)", got)
	}
	if got := rec.Body.String(); got != "" {
		t.Errorf("body = %q, want empty", got)
	}
}

func TestHandlePrometheus_RoundTrip(t *testing.T) {
	r := stats.NewRegistry()
	r.NewGauge("server.live").Set(1)
	r.NewCounter("listener.0_0_0_0_10000.downstream_cx_total").Add(7)
	h := handlePrometheus(r)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/prometheus", nil)
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE envoy_server_live gauge",
		"envoy_server_live 1",
		"# TYPE envoy_listener_downstream_cx_total counter",
		`envoy_listener_downstream_cx_total{envoy_listener_address="0_0_0_0_10000"} 7`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q\n--- body ---\n%s", want, body)
		}
	}
}
