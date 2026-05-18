package httpclient_test

// httpclient_test.go — Group 6 unit tests per phase-20 SPEC §14.1 + ADR-0177.
//
// Coverage roster (planner-time D5 + SPEC §14.1 Group 6):
//   - Options zero-value: no timeout, no retries, nil TLSConfig — pass-through
//   - Client.Do happy-path 200 OK
//   - Zero-retry default: single attempt even on 5xx
//   - Retry envelope: status-driven retry; attempt count == Attempts + 1
//   - Context cancellation mid-Do via context.WithTimeout(1ms) → DeadlineExceeded
//   - TLSConfig wired through (httptest.NewTLSServer + InsecureSkipVerify)
//   - Request-error propagation
//   - New(zero Options) constructs a usable Client (no panic)
//   - Retry honors context cancellation between attempts (no retry-after-cancel)
//   - Non-retryable status passes through on first hit
//
// Implements the SPEC §3.1 + ADR-0177 §Context public surface:
//   Options{Timeout, RetryPolicy, TLSConfig}
//   RetryPolicy{Attempts, PerAttemptDelay, RetryOnStatus}
//   Client; New(opts); (*Client).Do(req)

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/httpclient"
)

// ----------------------------------------------------------------------------
// Group 6 — internal/httpclient tests
// ----------------------------------------------------------------------------

// TestOptions_ZeroValue_NoOpDefaults verifies that the zero-Options Client has
// no client-imposed timeout, no retries, and accepts nil TLSConfig (delegating
// to the stdlib's default crypto/tls posture). The Client must construct
// without panic.
func TestOptions_ZeroValue_NoOpDefaults(t *testing.T) {
	t.Parallel()
	c := httpclient.New(httpclient.Options{})
	if c == nil {
		t.Fatalf("New(zero Options): want non-nil Client")
	}
	// A trivial GET against an httptest server should succeed without timeout
	// triggering.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do(zero Options): %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
}

// TestDo_HappyPath_200 verifies that a basic GET against a 200-OK server
// returns the response body verbatim.
func TestDo_HappyPath_200(t *testing.T) {
	t.Parallel()
	const want = "hello, httpclient"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, want)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != want {
		t.Errorf("body: want %q, got %q", want, string(body))
	}
}

// TestZeroRetry_Default_SingleAttempt verifies that with the zero-Options
// (no RetryPolicy), a 5xx response from the server is RETURNED VERBATIM as
// the response (no retry attempted; single attempt only). Per SPEC §20.P1
// RATIFIED — upstream Envoy v1.37.2 wire default is zero retry.
func TestZeroRetry_Default_SingleAttempt(t *testing.T) {
	t.Parallel()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("attempts: want 1 (zero-retry default), got %d", got)
	}
}

// TestRetry_StatusDriven_AttemptCount verifies that with a non-zero
// RetryPolicy{Attempts: 2, RetryOnStatus: [500, 502, 503, 504]}, a server
// returning 503 every time yields exactly Attempts+1 == 3 total attempts.
func TestRetry_StatusDriven_AttemptCount(t *testing.T) {
	t.Parallel()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		RetryPolicy: httpclient.RetryPolicy{
			Attempts:        2, // 2 retries → 3 total attempts
			PerAttemptDelay: 1 * time.Millisecond,
			RetryOnStatus:   []int{500, 502, 503, 504},
		},
	})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status: want 503, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("attempts: want 3 (Attempts=2 ⇒ Attempts+1 total), got %d", got)
	}
}

// TestRetry_NonRetryableStatus_NoRetry verifies that a status NOT in
// RetryOnStatus is RETURNED VERBATIM on first hit (no retry).
func TestRetry_NonRetryableStatus_NoRetry(t *testing.T) {
	t.Parallel()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusNotFound) // 404 — NOT in RetryOnStatus
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		RetryPolicy: httpclient.RetryPolicy{
			Attempts:        3,
			PerAttemptDelay: 1 * time.Millisecond,
			RetryOnStatus:   []int{500, 502, 503, 504},
		},
	})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: want 404, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("attempts: want 1 (non-retryable status), got %d", got)
	}
}

// TestRetry_SucceedsOnRetry verifies that a server returning 503 once then
// 200 yields a 200 response after exactly 2 attempts.
func TestRetry_SucceedsOnRetry(t *testing.T) {
	t.Parallel()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		RetryPolicy: httpclient.RetryPolicy{
			Attempts:        2,
			PerAttemptDelay: 1 * time.Millisecond,
			RetryOnStatus:   []int{503},
		},
	})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Errorf("attempts: want 2, got %d", got)
	}
}

// TestCtxCancellation_MidDo_ReturnsDeadlineExceeded verifies that an in-flight
// request whose context expires returns context.DeadlineExceeded (or a wrapped
// equivalent) and does not hang.
func TestCtxCancellation_MidDo_ReturnsDeadlineExceeded(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the per-request deadline so the request times out.
		select {
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			// Client canceled — best-effort cleanup.
			return
		}
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{Timeout: 0}) // no Client-level timeout; rely on req ctx
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("Do: want error from ctx cancellation, got nil")
	}
	// Accept either DeadlineExceeded directly or an error whose chain contains it.
	if !errors.Is(err, context.DeadlineExceeded) {
		// net/http often wraps it in a *url.Error whose Unwrap returns the
		// ctx error; errors.Is should unwrap. If not, accept the timeout text
		// match as a fallback (some Go versions surface i/o timeout).
		if !strings.Contains(err.Error(), "deadline exceeded") &&
			!strings.Contains(err.Error(), "context canceled") &&
			!strings.Contains(err.Error(), "Client.Timeout") {
			t.Errorf("err: want DeadlineExceeded-equivalent, got %v", err)
		}
	}
}

// TestRetry_HonorsCtxCancellation_BetweenAttempts verifies that an already-
// canceled context short-circuits the inter-attempt sleep — the loop does NOT
// continue retrying after the context is done.
func TestRetry_HonorsCtxCancellation_BetweenAttempts(t *testing.T) {
	t.Parallel()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{
		Timeout: 0,
		RetryPolicy: httpclient.RetryPolicy{
			Attempts:        5,
			PerAttemptDelay: 200 * time.Millisecond, // long enough that ctx expires first
			RetryOnStatus:   []int{503},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = c.Do(req)
	if err == nil {
		t.Fatal("Do: want error from ctx cancellation, got nil")
	}
	// Must have made at most 1 full attempt (the first request succeeds
	// before ctx expires; the inter-attempt sleep then trips the ctx check).
	if got := atomic.LoadInt32(&attempts); got > 1 {
		t.Errorf("attempts: want <=1 (ctx cancels inter-attempt sleep), got %d", got)
	}
}

// TestTLSConfig_WiredThrough verifies that a *tls.Config supplied via Options
// is actually applied to the underlying http.Transport — the test starts an
// httptest.NewTLSServer (self-signed cert) and uses InsecureSkipVerify=true
// to validate the TLS path is exercised end-to-end.
func TestTLSConfig_WiredThrough(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "tls-ok")
	}))
	defer srv.Close()

	// 1. Without TLSConfig: the request should FAIL (self-signed cert; default
	//    posture rejects).
	cReject := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	req1, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp1, err := cReject.Do(req1)
	if err == nil {
		_ = resp1.Body.Close()
		t.Fatal("Do(no TLSConfig): want cert-verification error, got success")
	}

	// 2. With InsecureSkipVerify TLSConfig: the request should SUCCEED.
	cAccept := httpclient.New(httpclient.Options{
		Timeout:   5 * time.Second,
		TLSConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 — test-only
	})
	req2, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp2, err := cAccept.Do(req2)
	if err != nil {
		t.Fatalf("Do(InsecureSkipVerify): %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp2.StatusCode)
	}
	body, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != "tls-ok" {
		t.Errorf("body: want %q, got %q", "tls-ok", string(body))
	}
}

// TestDo_RequestError_Propagated verifies that a transport-level error (e.g.,
// connection refused against a closed server) propagates as an error from Do
// without panicking.
func TestDo_RequestError_Propagated(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := srv.URL
	srv.Close() // immediately close so connection is refused

	c := httpclient.New(httpclient.Options{Timeout: 1 * time.Second})
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("Do(closed server): want transport error, got nil")
	}
}

// TestDo_PostBody verifies that a POST request with a body is sent and the
// body is observable on the server side. Anchors the synchronous-per-request
// POST shape that oauth2 token_endpoint will consume at Task 10.
func TestDo_PostBody(t *testing.T) {
	t.Parallel()
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = b
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	const payload = "grant_type=authorization_code&code=abc"
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
	if string(got) != payload {
		t.Errorf("body: want %q, got %q", payload, string(got))
	}
}
