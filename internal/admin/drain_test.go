package admin

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/drain"
	"github.com/esalaine/envoy-go/internal/stats"
)

func TestHandleDrainListeners_PostFires(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 200 {
		t.Errorf("status: got %d, want 200", got)
	}
	if got := w.Body.String(); got != "OK\n" {
		t.Errorf("body: got %q, want %q", got, "OK\n")
	}
	if dm.State() != drain.StateDraining {
		t.Errorf("dm.State post-POST: got %v, want StateDraining", dm.State())
	}
}

func TestHandleDrainListeners_BodyExact(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	body := w.Body.Bytes()
	if len(body) != 3 || body[0] != 'O' || body[1] != 'K' || body[2] != '\n' {
		t.Errorf("body byte-exact: got %q (len=%d), want %q (len=3)", body, len(body), "OK\n")
	}
}

func TestHandleDrainListeners_Idempotent(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/drain_listeners", nil)
		s.handleDrainListeners(w, r)
		if got := w.Code; got != 200 {
			t.Errorf("iteration %d status: got %d, want 200", i, got)
		}
		if got := w.Body.String(); got != "OK\n" {
			t.Errorf("iteration %d body: got %q, want %q", i, got, "OK\n")
		}
	}
}

func TestHandleDrainListeners_GraceQueryParamSilentlyIgnored(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/drain_listeners?graceful=true", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 200 {
		t.Errorf("status: got %d, want 200", got)
	}
	if got := w.Body.String(); got != "OK\n" {
		t.Errorf("body: got %q, want %q", got, "OK\n")
	}
}

func TestHandleDrainListeners_NilDrainManager(t *testing.T) {
	// Per planner-time decision 10 (defensive 500 vs no-op 200): defensive 500
	// with body "drain manager not configured\n" — the operator gets a clear
	// signal that the drain machinery is not wired.
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 500 {
		t.Errorf("nil-dm status: got %d, want 500", got)
	}
	if got := w.Body.String(); got != "drain manager not configured\n" {
		t.Errorf("nil-dm body: got %q, want %q", got, "drain manager not configured\n")
	}
}

func TestHandleDrainListeners_GetReturns405(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 405 {
		t.Errorf("GET status: got %d, want 405", got)
	}
	if got := w.Body.String(); got != "Method GET not allowed, POST required.\n" {
		t.Errorf("GET body: got %q, want %q", got, "Method GET not allowed, POST required.\n")
	}
	if dm.State() != drain.StateLive {
		t.Errorf("dm.State after GET 405: got %v, want StateLive (no side effect)", dm.State())
	}
}

func TestHandleDrainListeners_PutReturns405(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 405 {
		t.Errorf("PUT status: got %d, want 405", got)
	}
	if got := w.Body.String(); got != "Method PUT not allowed, POST required.\n" {
		t.Errorf("PUT body: got %q", got)
	}
}

func TestHandleDrainListeners_DeleteReturns405(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 405 {
		t.Errorf("DELETE status: got %d, want 405", got)
	}
	if got := w.Body.String(); got != "Method DELETE not allowed, POST required.\n" {
		t.Errorf("DELETE body: got %q", got)
	}
}

func TestHandleDrainListeners_HeadReturns405(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("HEAD", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	if got := w.Code; got != 405 {
		t.Errorf("HEAD status: got %d, want 405", got)
	}
	// HEAD semantics — we still emit the body to httptest.Recorder; net/http's
	// Server elides the body on the wire for HEAD per RFC 9110, but the
	// handler itself writes the same bytes; the headers are what matter.
}

func TestHandleDrainListeners_HeaderSet(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	s := New("127.0.0.1:0", stats.NewRegistry(), nil, nil, nil, dm)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/drain_listeners", nil)
	s.handleDrainListeners(w, r)
	h := w.Header()
	cases := []struct{ key, want string }{
		{"Content-Type", "text/plain; charset=UTF-8"},
		{"Cache-Control", "no-cache, max-age=0"},
		{"X-Content-Type-Options", "nosniff"},
		{"Server", "envoy"},
	}
	for _, c := range cases {
		if got := h.Get(c.key); got != c.want {
			t.Errorf("header %q: got %q, want %q", c.key, got, c.want)
		}
	}
	// Body should be present in the recorder
	if !strings.HasPrefix(w.Body.String(), "OK") {
		t.Errorf("body prefix: got %q, want starts with OK", w.Body.String())
	}
}
