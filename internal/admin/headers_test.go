package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteAdminHeaders_SetsFourConstantHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	writeAdminHeaders(w, "application/json")
	h := w.Header()
	cases := []struct{ key, want string }{
		{"Content-Type", "application/json"},
		{"Cache-Control", "no-cache, max-age=0"},
		{"X-Content-Type-Options", "nosniff"},
		{"Server", "envoy"},
	}
	for _, c := range cases {
		if got := h.Get(c.key); got != c.want {
			t.Errorf("header %q: got %q, want %q", c.key, got, c.want)
		}
	}
}

func TestWriteAdminHeaders_DoesNotSetDateOrContentLength(t *testing.T) {
	w := httptest.NewRecorder()
	writeAdminHeaders(w, "text/plain")
	if got := w.Header().Get("Date"); got != "" {
		t.Errorf("Date should be empty (auto-added by net/http); got %q", got)
	}
	if got := w.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length should be empty (auto-added by net/http); got %q", got)
	}
}

func TestWriteAdminHeaders_OverwritePreviousContentType(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "previous/value")
	writeAdminHeaders(w, "application/json")
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", got, "application/json")
	}
}

// TestWriteAdminHeaders_AppliedThroughHTTPServer is an end-to-end check that
// the headers reach the wire (net/http does not strip them; case-canonicalisation
// happens at write time per the standard library's contract).
func TestWriteAdminHeaders_AppliedThroughHTTPServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeAdminHeaders(w, "text/plain; charset=UTF-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Server"); got != "envoy" {
		t.Errorf("end-to-end Server: got %q, want %q", got, "envoy")
	}
	if got := resp.Header.Get("Date"); got == "" {
		t.Errorf("end-to-end Date: empty; want net/http auto-add")
	}
}
