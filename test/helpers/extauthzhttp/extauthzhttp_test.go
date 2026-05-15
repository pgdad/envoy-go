package extauthzhttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// startTestServer is the per-test lifecycle helper: it spawns a Server on an
// ephemeral port with the supplied script and returns a cleanup func.
func startTestServer(t *testing.T, script Script) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := New(ctx, "127.0.0.1:0", script)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv
}

// doRequest performs an HTTP request with the given method, path, and body
// against the server address, returning status, body, and response headers.
func doRequest(t *testing.T, addr, method, path string, reqBody []byte, extraHeaders map[string]string) (int, []byte, http.Header) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var bodyReader io.Reader
	if reqBody != nil {
		bodyReader = bytes.NewReader(reqBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+addr+path, bodyReader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return resp.StatusCode, body, resp.Header
}

// TestNew_StartsServerOnConfiguredAddr verifies that the server binds to the
// configured address (127.0.0.1:0 → ephemeral port) and Addr() returns the
// actual bound address.
func TestNew_StartsServerOnConfiguredAddr(t *testing.T) {
	srv := startTestServer(t, FixedScript(200, nil, nil))
	addr := srv.Addr()
	if addr == "" {
		t.Fatal("Addr: empty after New")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	if host != "127.0.0.1" {
		t.Errorf("host: got %q, want %q", host, "127.0.0.1")
	}
	if port == "0" {
		t.Errorf("port: got %q, want non-zero ephemeral", port)
	}
	// Sanity: the server actually accepts connections.
	status, _, _ := doRequest(t, addr, http.MethodGet, "/", nil, nil)
	if status != 200 {
		t.Errorf("status: got %d, want 200", status)
	}
}

// TestServer_FixedScript_ReturnsStatusBodyHeaders verifies that a fixed-script
// server returns the configured status, body, and headers for any request.
func TestServer_FixedScript_ReturnsStatusBodyHeaders(t *testing.T) {
	wantStatus := 403
	wantBody := []byte("access denied by auth server")
	wantHeaders := map[string]string{
		"X-Auth-Decision": "deny",
		"X-Custom-Header": "value42",
	}
	srv := startTestServer(t, FixedScript(wantStatus, wantBody, wantHeaders))
	status, body, headers := doRequest(t, srv.Addr(), http.MethodPost, "/auth", []byte("req body"), nil)
	if status != wantStatus {
		t.Errorf("status: got %d, want %d", status, wantStatus)
	}
	if !bytes.Equal(body, wantBody) {
		t.Errorf("body: got %q, want %q", body, wantBody)
	}
	for k, v := range wantHeaders {
		if got := headers.Get(k); got != v {
			t.Errorf("header %q: got %q, want %q", k, got, v)
		}
	}
}

// TestServer_PathMethodMap_Dispatch verifies that the server dispatches to
// different handlers per-path and per-method.
func TestServer_PathMethodMap_Dispatch(t *testing.T) {
	routes := map[PathMethod]ScriptEntry{
		{Path: "/allow", Method: http.MethodPost}: {
			Status:  200,
			Body:    []byte("allowed"),
			Headers: map[string]string{"X-Decision": "allow"},
		},
		{Path: "/deny", Method: http.MethodPost}: {
			Status:  403,
			Body:    []byte("denied"),
			Headers: map[string]string{"X-Decision": "deny"},
		},
	}
	srv := startTestServer(t, RouteScript(routes, 404, nil, nil))
	cases := []struct {
		path       string
		wantStatus int
		wantBody   []byte
	}{
		{"/allow", 200, []byte("allowed")},
		{"/deny", 403, []byte("denied")},
		{"/other", 404, nil},
	}
	for _, tc := range cases {
		status, body, _ := doRequest(t, srv.Addr(), http.MethodPost, tc.path, nil, nil)
		if status != tc.wantStatus {
			t.Errorf("POST %s: status got %d, want %d", tc.path, status, tc.wantStatus)
		}
		if tc.wantBody != nil && !bytes.Equal(body, tc.wantBody) {
			t.Errorf("POST %s: body got %q, want %q", tc.path, body, tc.wantBody)
		}
	}
}

// TestServer_BodyInspectingScript verifies that a body-inspecting predicate
// script can inspect the inbound request body and decide allow/deny.
func TestServer_BodyInspectingScript(t *testing.T) {
	// Deny requests whose body contains "secret"; allow everything else.
	srv := startTestServer(t, InspectScript(func(method, path string, body []byte) (int, []byte, map[string]string) {
		if bytes.Contains(body, []byte("secret")) {
			return 403, []byte("forbidden"), nil
		}
		return 200, []byte("ok"), map[string]string{"X-Inspected": "true"}
	}))
	// Should be allowed.
	status, body, headers := doRequest(t, srv.Addr(), http.MethodPost, "/auth", []byte("safe payload"), nil)
	if status != 200 {
		t.Errorf("allow: status got %d, want 200", status)
	}
	if !bytes.Equal(body, []byte("ok")) {
		t.Errorf("allow: body got %q, want %q", body, "ok")
	}
	if got := headers.Get("X-Inspected"); got != "true" {
		t.Errorf("allow: X-Inspected got %q, want %q", got, "true")
	}
	// Should be denied.
	status, body, _ = doRequest(t, srv.Addr(), http.MethodPost, "/auth", []byte("payload with secret inside"), nil)
	if status != 403 {
		t.Errorf("deny: status got %d, want 403", status)
	}
	if !bytes.Equal(body, []byte("forbidden")) {
		t.Errorf("deny: body got %q, want %q", body, "forbidden")
	}
}

// TestServer_Stop_ClosesListener verifies that Stop() terminates the listener
// and subsequent connections fail.
func TestServer_Stop_ClosesListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := New(ctx, "127.0.0.1:0", FixedScript(200, nil, nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	addr := srv.Addr()
	// Sanity: server is alive.
	status, _, _ := doRequest(t, addr, http.MethodGet, "/", nil, nil)
	if status != 200 {
		t.Fatalf("pre-stop GET: status=%d", status)
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Post-Stop connection MUST fail.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel2()
	req, _ := http.NewRequestWithContext(ctx2, http.MethodGet, "http://"+addr+"/", nil)
	_, err = http.DefaultClient.Do(req)
	if err == nil {
		t.Fatal("post-Stop GET: want error (listener closed); got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "refused") && !strings.Contains(msg, "EOF") &&
		!strings.Contains(msg, "reset") && !strings.Contains(msg, "connection") {
		t.Logf("post-Stop GET error (accepted): %v", err)
	}
}

// TestServer_Stop_Idempotent verifies that Stop() can be called multiple times
// without error.
func TestServer_Stop_Idempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := New(ctx, "127.0.0.1:0", FixedScript(200, nil, nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop #1: %v", err)
	}
	if err := srv.Stop(); err != nil {
		t.Errorf("Stop #2 (idempotent): %v", err)
	}
}

// TestServer_ConcurrentClient_NoRace verifies that concurrent requests from
// multiple goroutines do not trigger the race detector.
func TestServer_ConcurrentClient_NoRace(t *testing.T) {
	srv := startTestServer(t, FixedScript(200, []byte("ok"), nil))
	addr := srv.Addr()
	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)
	errs := make([]error, concurrency)
	for i := 0; i < concurrency; i++ {
		i := i
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://%s/auth", addr), bytes.NewReader([]byte("body")))
			if err != nil {
				errs[i] = err
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errs[i] = err
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != 200 {
				errs[i] = fmt.Errorf("status %d", resp.StatusCode)
			}
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine[%d]: %v", i, err)
		}
	}
}
