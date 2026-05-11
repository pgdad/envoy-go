package echobackend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// startTestServer binds the echobackend server on an ephemeral port and
// returns its base URL plus a cleanup func that stops the server.
func startTestServer(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := New()
	go func() { _ = srv.Serve(ln) }()
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	return fmt.Sprintf("http://%s", ln.Addr().String()), cleanup
}

// doRequest issues req against base+path with the supplied headers and
// returns the decoded echoRecord.
func doRequest(t *testing.T, base, method, path string, hdrs http.Header) echoRecord {
	t.Helper()
	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, vs := range hdrs {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", got)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var rec echoRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		t.Fatalf("unmarshal %q: %v", string(body), err)
	}
	return rec
}

func TestEcho_MethodAndPath(t *testing.T) {
	base, stop := startTestServer(t)
	defer stop()
	cases := []struct {
		method, path string
	}{
		{"GET", "/"},
		{"GET", "/foo"},
		{"POST", "/bar/baz"},
		{"DELETE", "/x/y/z"},
	}
	for _, c := range cases {
		rec := doRequest(t, base, c.method, c.path, nil)
		if rec.Method != c.method {
			t.Errorf("method: got %q, want %q", rec.Method, c.method)
		}
		if rec.Path != c.path {
			t.Errorf("path: got %q, want %q", rec.Path, c.path)
		}
	}
}

func TestEcho_HeaderKeysLowercased(t *testing.T) {
	base, stop := startTestServer(t)
	defer stop()
	rec := doRequest(t, base, "GET", "/", http.Header{
		"X-Mixed-Case-Header": {"value1"},
		"Accept-Encoding":     {"gzip"},
	})
	// Every echoed header key must be lowercase per ADR-0072.
	for k := range rec.Headers {
		if k != strings.ToLower(k) {
			t.Errorf("header key %q not lowercased", k)
		}
	}
	if got := rec.Headers["x-mixed-case-header"]; got != "value1" {
		t.Errorf("x-mixed-case-header: got %q, want %q", got, "value1")
	}
	if got := rec.Headers["accept-encoding"]; got != "gzip" {
		t.Errorf("accept-encoding: got %q, want %q", got, "gzip")
	}
}

func TestEcho_MultiValueHeaderJoined(t *testing.T) {
	base, stop := startTestServer(t)
	defer stop()
	rec := doRequest(t, base, "GET", "/", http.Header{
		"X-Multi": {"alpha", "beta", "gamma"},
	})
	got := rec.Headers["x-multi"]
	// Per RFC 7230 §3.2.2, comma-joined.
	want := "alpha, beta, gamma"
	if got != want {
		t.Errorf("x-multi: got %q, want %q", got, want)
	}
}

func TestEcho_EmptyHeaderSetTolerated(t *testing.T) {
	base, stop := startTestServer(t)
	defer stop()
	rec := doRequest(t, base, "GET", "/", nil)
	// The Go http client adds a few default headers (Host via req.Host, User-Agent,
	// Accept-Encoding). The contract is: handler does not panic and always
	// produces a valid JSON response.
	if rec.Method != "GET" {
		t.Errorf("method: got %q, want GET", rec.Method)
	}
	if _, ok := rec.Headers["host"]; !ok {
		t.Errorf("expected host header in echoed map; got keys %v", rec.Headers)
	}
}

func TestEcho_LargeHeaderSetTolerated(t *testing.T) {
	base, stop := startTestServer(t)
	defer stop()
	hdrs := http.Header{}
	for i := 0; i < 50; i++ {
		hdrs.Set(fmt.Sprintf("X-Hdr-%d", i), fmt.Sprintf("value-%d", i))
	}
	rec := doRequest(t, base, "GET", "/", hdrs)
	for i := 0; i < 50; i++ {
		k := strings.ToLower(fmt.Sprintf("X-Hdr-%d", i))
		want := fmt.Sprintf("value-%d", i)
		if got := rec.Headers[k]; got != want {
			t.Errorf("%s: got %q, want %q", k, got, want)
		}
	}
}

func TestEcho_HostHeaderEchoed(t *testing.T) {
	base, stop := startTestServer(t)
	defer stop()
	// The Go http client populates req.Host from the URL; the handler should
	// surface it under the "host" key (lowercased canonical) per net/http
	// convention (Host is stripped from req.Header by the server).
	rec := doRequest(t, base, "GET", "/", nil)
	host, ok := rec.Headers["host"]
	if !ok {
		t.Fatalf("host header absent; got keys %v", rec.Headers)
	}
	if !strings.HasPrefix(base, "http://"+host) {
		t.Errorf("host echo: got %q, base=%q", host, base)
	}
}

func TestListen_BindsRequestedPort(t *testing.T) {
	// Allocate a free port first by binding/closing.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()

	ln, err := Listen(port)
	if err != nil {
		t.Fatalf("Listen(%d): %v", port, err)
	}
	defer func() { _ = ln.Close() }()
	got := ln.Addr().(*net.TCPAddr).Port
	if got != port {
		t.Errorf("port: got %d, want %d", got, port)
	}
}
