package hcm

import (
	stdtls "crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
)

// mkH3DirectResponseFilter builds a minimal *Filter with a single
// direct_response route at path → (status, bodyText), reusing the H1 test
// harness mkFilterForTable (router-only chain). Shared by the runH3 dispatch
// tests below.
func mkH3DirectResponseFilter(t *testing.T, path string, status int, bodyText string) *Filter {
	t.Helper()
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath(path), action: &directResponseAction{status: status, bodyText: bodyText}},
	}}
	return mkFilterForTable(t, tt)
}

// TestRunH3_GET_DirectResponse verifies a routed GET is dispatched through the
// chain and the direct_response body + status are written to the ResponseWriter.
func TestRunH3_GET_DirectResponse(t *testing.T) {
	f := mkH3DirectResponseFilter(t, "/probe", 200, "h3-ok")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/probe", nil)
	req.TLS = &stdtls.ConnectionState{Version: stdtls.VersionTLS13, HandshakeComplete: true, NegotiatedProtocol: "h3"}
	rec := httptest.NewRecorder()
	status, err := f.runH3(req.Context(), rec, req)
	if err != nil {
		t.Fatalf("runH3: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	if rec.Body.String() != "h3-ok" {
		t.Errorf("body = %q, want h3-ok", rec.Body.String())
	}
}

// TestRunH3_NoMatch_404 verifies an unmatched path returns a 404 (mirrors the
// H1 dispatchRequest no-match branch). r.TLS is left nil to exercise the
// unit-test plaintext path — every TLS seeder must nil-tolerate.
func TestRunH3_NoMatch_404(t *testing.T) {
	f := mkH3DirectResponseFilter(t, "/probe", 200, "h3-ok")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/nope", nil)
	rec := httptest.NewRecorder()
	status, err := f.runH3(req.Context(), rec, req)
	if err != nil {
		t.Fatalf("runH3: %v", err)
	}
	if status != 404 {
		t.Errorf("status = %d, want 404", status)
	}
	// The 4xx class counter must have moved for the no-match response.
	if got := f.downstreamRq4xx.Load(); got != 1 {
		t.Errorf("downstream_rq_4xx = %d, want 1", got)
	}
}

// TestRunH3_POST_Body verifies a request WITH a body flows through the
// decode-data loop and the routed direct_response is still written.
func TestRunH3_POST_Body(t *testing.T) {
	f := mkH3DirectResponseFilter(t, "/probe", 200, "h3-ok")
	req := httptest.NewRequest(http.MethodPost, "https://example.test/probe", strings.NewReader("payload-bytes"))
	rec := httptest.NewRecorder()
	status, err := f.runH3(req.Context(), rec, req)
	if err != nil {
		t.Fatalf("runH3: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	if rec.Body.String() != "h3-ok" {
		t.Errorf("body = %q, want h3-ok", rec.Body.String())
	}
}

// TestServeH3_GET verifies the http.HandlerFunc entry point dispatches through
// runH3 and writes the response (the quic-go http3.Server handler seam).
func TestServeH3_GET(t *testing.T) {
	f := mkH3DirectResponseFilter(t, "/probe", 200, "h3-ok")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/probe", nil)
	rec := httptest.NewRecorder()
	f.ServeH3(rec, req)
	if rec.Result().StatusCode != 200 {
		t.Errorf("status = %d, want 200", rec.Result().StatusCode)
	}
	if rec.Body.String() != "h3-ok" {
		t.Errorf("body = %q, want h3-ok", rec.Body.String())
	}
}

// TestWriteH3Reply_StatusHeadersBody verifies the ActionResponse → ResponseWriter
// projection: status code, response headers, and body are written; HTTP pseudo-
// headers (":status" etc.) are NOT leaked into the response header map.
func TestWriteH3Reply_StatusHeadersBody(t *testing.T) {
	rec := httptest.NewRecorder()
	hdrs := filter_http.OrderedHeaders{
		{Name: "content-type", Value: "text/plain"},
		{Name: "x-custom", Value: "v1"},
		{Name: ":status", Value: "200"}, // pseudo-header — must be dropped
	}
	if err := writeH3Reply(rec, 200, hdrs, []byte("h3-ok")); err != nil {
		t.Fatalf("writeH3Reply: %v", err)
	}
	res := rec.Result()
	if res.StatusCode != 200 {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("content-type"); got != "text/plain" {
		t.Errorf("content-type = %q, want text/plain", got)
	}
	if got := res.Header.Get("x-custom"); got != "v1" {
		t.Errorf("x-custom = %q, want v1", got)
	}
	if _, leaked := res.Header[":status"]; leaked {
		t.Errorf(":status pseudo-header leaked into the response header map")
	}
	if body := rec.Body.String(); body != "h3-ok" {
		t.Errorf("body = %q, want h3-ok", body)
	}
}

// TestWriteH3Reply_EmptyBody verifies a headers-only response (no body) writes
// the status with no panic and an empty body.
func TestWriteH3Reply_EmptyBody(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := writeH3Reply(rec, 204, nil, nil); err != nil {
		t.Fatalf("writeH3Reply: %v", err)
	}
	if rec.Result().StatusCode != 204 {
		t.Errorf("status = %d, want 204", rec.Result().StatusCode)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body len = %d, want 0", rec.Body.Len())
	}
}
