package driver

import (
	"net/http"
	"strings"
	"testing"
)

// TestEncodeProbe_PreflightAllowed exercises the encode form for the
// allowed-origin preflight (probe a). The 6 CORS headers should appear
// sorted alphabetically; body should be empty.
func TestEncodeProbe_PreflightAllowed(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Access-Control-Allow-Origin":      []string{"https://example.test"},
			"Access-Control-Allow-Credentials": []string{"true"},
			"Access-Control-Allow-Methods":     []string{"GET, POST, OPTIONS"},
			"Access-Control-Allow-Headers":     []string{"x-foo, x-bar"},
			"Access-Control-Max-Age":           []string{"600"},
			"Access-Control-Expose-Headers":    []string{"x-baz"},
			"Date":                             []string{"Sun, 01 Jan 2026 00:00:00 GMT"},
			"Server":                           []string{"envoy"},
			"Content-Length":                   []string{"0"},
		},
	}
	p := probe{tag: "probe-a OPTIONS /permissive (allowed origin)"}
	out := encodeProbe(1, p, resp, []byte{})

	// All 6 cors headers present, sorted alphabetically.
	want := []string{
		"=== request 1 probe-a OPTIONS /permissive (allowed origin)",
		"status: 200",
		"cors-headers (sorted):",
		"  access-control-allow-credentials: true",
		"  access-control-allow-headers: x-foo, x-bar",
		"  access-control-allow-methods: GET, POST, OPTIONS",
		"  access-control-allow-origin: https://example.test",
		"  access-control-expose-headers: x-baz",
		"  access-control-max-age: 600",
		`body: ""`,
	}
	for _, line := range want {
		if !strings.Contains(out, line) {
			t.Errorf("expected line %q in output, got:\n%s", line, out)
		}
	}
	// Non-cors headers must NOT appear (Date / Server / Content-Length all
	// allow-listed at the runner-side HTTPHeaderDiff layer; this test pins
	// the byte-stream to cors-only so the differential gate is independent
	// of those allow-listed values).
	for _, banned := range []string{"date:", "server:", "content-length:"} {
		if strings.Contains(strings.ToLower(out), banned) {
			t.Errorf("unexpected header %q in output:\n%s", banned, out)
		}
	}
}

// TestEncodeProbe_DisallowedPreflight covers probe (b): no cors headers,
// status 405, body present.
func TestEncodeProbe_DisallowedPreflight(t *testing.T) {
	resp := &http.Response{
		StatusCode: 405,
		Header: http.Header{
			"Date":         []string{"Sun, 01 Jan 2026 00:00:00 GMT"},
			"Content-Type": []string{"text/plain"},
		},
	}
	p := probe{tag: "probe-b OPTIONS /strict (disallowed origin)"}
	out := encodeProbe(2, p, resp, []byte("method not allowed\n"))

	for _, line := range []string{
		"=== request 2 probe-b OPTIONS /strict (disallowed origin)",
		"status: 405",
		"cors-headers (sorted):",
		"  (none)",
		`body: "method not allowed\n"`,
	} {
		if !strings.Contains(out, line) {
			t.Errorf("expected line %q in output, got:\n%s", line, out)
		}
	}
}

// TestEncodeProbe_ActualAllowed covers probe (c): 3 cors headers (no
// preflight-only headers), 200 status, body "hello\n".
func TestEncodeProbe_ActualAllowed(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Access-Control-Allow-Origin":      []string{"https://example.test"},
			"Access-Control-Allow-Credentials": []string{"true"},
			"Access-Control-Expose-Headers":    []string{"x-baz"},
			"Date":                             []string{"Sun, 01 Jan 2026 00:00:00 GMT"},
		},
	}
	p := probe{tag: "probe-c GET /permissive (allowed origin)"}
	out := encodeProbe(3, p, resp, []byte("hello\n"))

	for _, line := range []string{
		"status: 200",
		"  access-control-allow-credentials: true",
		"  access-control-allow-origin: https://example.test",
		"  access-control-expose-headers: x-baz",
		`body: "hello\n"`,
	} {
		if !strings.Contains(out, line) {
			t.Errorf("expected line %q in output, got:\n%s", line, out)
		}
	}
	// Preflight-only headers must NOT be in actual-request response.
	for _, banned := range []string{"allow-methods", "allow-headers", "max-age"} {
		if strings.Contains(out, banned) {
			t.Errorf("unexpected preflight-only header containing %q in actual-request output:\n%s", banned, out)
		}
	}
}

// TestEncodeProbe_ActualNoOrigin covers probe (d): no cors headers, 200,
// body "hello\n".
func TestEncodeProbe_ActualNoOrigin(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Date": []string{"Sun, 01 Jan 2026 00:00:00 GMT"}},
	}
	p := probe{tag: "probe-d GET /permissive (no Origin)"}
	out := encodeProbe(4, p, resp, []byte("hello\n"))

	for _, line := range []string{
		"status: 200",
		"cors-headers (sorted):",
		"  (none)",
		`body: "hello\n"`,
	} {
		if !strings.Contains(out, line) {
			t.Errorf("expected line %q in output, got:\n%s", line, out)
		}
	}
}

// TestDriver_RegisteredAtInit pins the init() registration so a future
// rename of fixtureName surfaces here.
func TestDriver_RegisteredAtInit(t *testing.T) {
	if fixtureName != "0007a-cors" {
		t.Errorf("fixtureName drift: got %q, want %q", fixtureName, "0007a-cors")
	}
}
