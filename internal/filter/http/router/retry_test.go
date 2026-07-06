package router

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	"github.com/pgdad/envoy-go/internal/stats"
)

// scriptedBackend is a controllable HTTP/1.1 backend for the retryExecutorH1
// loop tests. Each accepted connection reads ONE full request (draining the
// body up to Content-Length), records the body bytes it received, then writes a
// single response whose status is taken from statusFor(connIndex). Every
// response carries Connection: close so doH1ClusterAction opens a fresh
// connection per attempt (matching the no-keepalive retry shape). An optional
// per-connection delay widens the concurrency window for the budget test.
type scriptedBackend struct {
	addr      string
	stop      func()
	mu        sync.Mutex
	bodies    [][]byte // body bytes received, one entry per served request
	conns     int64    // accepted-and-served request count (atomic)
	statusFor func(connIndex int64) int
	delay     time.Duration
}

func newScriptedBackend(t *testing.T, statusFor func(connIndex int64) int, delay time.Duration) *scriptedBackend {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := &scriptedBackend{addr: ln.Addr().String(), statusFor: statusFor, delay: delay}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go b.serve(conn)
		}
	}()
	b.stop = func() { _ = ln.Close(); <-done }
	return b
}

func (b *scriptedBackend) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	body, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	idx := atomic.AddInt64(&b.conns, 1) - 1
	b.mu.Lock()
	b.bodies = append(b.bodies, body)
	b.mu.Unlock()
	if b.delay > 0 {
		time.Sleep(b.delay)
	}
	status := b.statusFor(idx)
	text := http.StatusText(status)
	if text == "" {
		text = "Status"
	}
	respBody := fmt.Sprintf("resp:%d", status)
	resp := fmt.Sprintf(
		"HTTP/1.1 %d %s\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		status, text, len(respBody), respBody,
	)
	_, _ = c.Write([]byte(resp))
}

func (b *scriptedBackend) recordedBodies() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([][]byte, len(b.bodies))
	copy(out, b.bodies)
	return out
}

// mkRetryPolicy builds a RetryPolicy with a tiny base_interval so the loop's
// backoff sleeps stay sub-millisecond (the differential is count-based; the
// delay is irrelevant to the increment assertions but keeps the unit test fast).
func mkRetryPolicy(t *testing.T, retryOn string, numRetries uint32) *RetryPolicy {
	t.Helper()
	rp, err := NewRetryPolicy(retryOn, numRetries, nil, time.Microsecond, time.Millisecond, 0)
	if err != nil {
		t.Fatalf("NewRetryPolicy: %v", err)
	}
	return rp
}

// mkRetryPolicyPTT builds a RetryPolicy carrying a per_try_timeout (the 42.2a
// per-attempt deadline) plus the same tiny base/max intervals as mkRetryPolicy
// (sub-ms backoff). A sibling helper rather than extending mkRetryPolicy so the
// existing callers stay untouched.
func mkRetryPolicyPTT(t *testing.T, retryOn string, numRetries uint32, ptt time.Duration) *RetryPolicy {
	t.Helper()
	rp, err := NewRetryPolicy(retryOn, numRetries, nil, time.Microsecond, time.Millisecond, ptt)
	if err != nil {
		t.Fatalf("NewRetryPolicy: %v", err)
	}
	return rp
}

// retryBudgetCB returns a circuit_breakers proto whose DEFAULT threshold carries
// a retry_budget with budget_percent:0 (⇒ cap floors at min_retry_concurrency:1).
func retryBudgetCB() *clusterv3.CircuitBreakers {
	return &clusterv3.CircuitBreakers{
		Thresholds: []*clusterv3.CircuitBreakers_Thresholds{{
			Priority: corev3.RoutingPriority_DEFAULT,
			RetryBudget: &clusterv3.CircuitBreakers_Thresholds_RetryBudget{
				BudgetPercent:       &typev3.Percent{Value: 0},
				MinRetryConcurrency: &wrapperspb.UInt32Value{Value: 1},
			},
		}},
	}
}

// TestRetryExecutorH1_Exhaustion — an always-503 backend under retry_on:5xx,
// num_retries:3 ⇒ 4 attempts, final 503, the increment ledger per D-S42-7.
func TestRetryExecutorH1_Exhaustion(t *testing.T) {
	b := newScriptedBackend(t, func(int64) int { return 503 }, 0)
	defer b.stop()

	c, reg := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicy(t, "5xx", 3)}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	resp, _, err := retryExecutorH1(req.Context(), a, req)
	if err != nil {
		t.Fatalf("retryExecutorH1: %v", err)
	}
	if resp.Status != 503 {
		t.Errorf("final status = %d, want 503", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry", 3)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_limit_exceeded", 1)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_backoff_exponential", 3)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_success", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_total", 4)
}

// TestRetryExecutorH1_Recover — a backend that 503s on the first attempt then
// 200s ⇒ final 200, exactly one retry, _retry_success==1.
func TestRetryExecutorH1_Recover(t *testing.T) {
	b := newScriptedBackend(t, func(i int64) int {
		if i == 0 {
			return 503
		}
		return 200
	}, 0)
	defer b.stop()

	c, reg := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicy(t, "5xx", 3)}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	resp, _, err := retryExecutorH1(req.Context(), a, req)
	if err != nil {
		t.Fatalf("retryExecutorH1: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("final status = %d, want 200", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry", 1)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_success", 1)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_limit_exceeded", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_total", 2)
}

// TestRetryExecutorH1_NonRetriable — a 404 under retry_on:5xx is not retriable
// ⇒ no retry, no increments.
func TestRetryExecutorH1_NonRetriable(t *testing.T) {
	b := newScriptedBackend(t, func(int64) int { return 404 }, 0)
	defer b.stop()

	c, reg := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicy(t, "5xx", 3)}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	resp, _, err := retryExecutorH1(req.Context(), a, req)
	if err != nil {
		t.Fatalf("retryExecutorH1: %v", err)
	}
	if resp.Status != 404 {
		t.Errorf("final status = %d, want 404", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_success", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_limit_exceeded", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_total", 1)
}

// TestRetryExecutorH1_BodyReplay — a POST with a body must replay the FULL body
// on the retry attempt (the backend records each request's received body).
func TestRetryExecutorH1_BodyReplay(t *testing.T) {
	const payload = "hello-retry-body-payload"
	b := newScriptedBackend(t, func(i int64) int {
		if i == 0 {
			return 503
		}
		return 200
	}, 0)
	defer b.stop()

	c, _ := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicy(t, "5xx", 3)}

	req, _ := http.NewRequestWithContext(context.Background(), "POST", "http://upstream/x", strings.NewReader(payload))
	req.URL.Path = "/x"
	req.ContentLength = int64(len(payload))

	resp, _, err := retryExecutorH1(req.Context(), a, req)
	if err != nil {
		t.Fatalf("retryExecutorH1: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("final status = %d, want 200", resp.Status)
	}
	bodies := b.recordedBodies()
	if len(bodies) != 2 {
		t.Fatalf("backend served %d requests, want 2 (initial + 1 retry)", len(bodies))
	}
	for i, got := range bodies {
		if string(got) != payload {
			t.Errorf("attempt %d body = %q, want %q (full replay)", i, string(got), payload)
		}
	}
}

// TestRetryExecutorH1_BudgetOverflow — a retry_budget capped at 1 + concurrent
// always-503 requests ⇒ at least one upstream_rq_retry_overflow.
func TestRetryExecutorH1_BudgetOverflow(t *testing.T) {
	b := newScriptedBackend(t, func(int64) int { return 503 }, 15*time.Millisecond)
	defer b.stop()

	c, reg := singleEndpointClusterCB(t, b.addr, retryBudgetCB())
	c.EnsureRetryStats()
	rp := mkRetryPolicy(t, "5xx", 3)

	const n = 4
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			a := &routerAction{cluster: c, rp: rp}
			req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
			req.URL.Path = "/x"
			_, _, _ = retryExecutorH1(req.Context(), a, req)
		}()
	}
	wg.Wait()

	if got := counterValue(t, reg, "cluster.c_test.upstream_rq_retry_overflow"); got < 1 {
		t.Errorf("upstream_rq_retry_overflow = %d, want >= 1 (budget cap 1 + %d concurrent failing requests)", got, n)
	}
}

// TestRetryExecutorH1_PerTryTimeoutExhaustion — an always-blocking backend (delay
// far exceeds per_try_timeout) under retry_on:5xx, num_retries:3, per_try_timeout:
// 50ms ⇒ every attempt fires the child-ctx deadline, is overridden to a synthesized
// 504, classified retriable (5xx∋504), and counts toward num_retries ⇒ 4 attempts,
// final 504, the per-try-timeout ledger.
func TestRetryExecutorH1_PerTryTimeoutExhaustion(t *testing.T) {
	b := newScriptedBackend(t, func(int64) int { return 200 }, 5*time.Second)
	defer b.stop()

	c, reg := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicyPTT(t, "5xx", 3, 50*time.Millisecond)}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	resp, _, err := retryExecutorH1(req.Context(), a, req)
	if err != nil {
		t.Fatalf("retryExecutorH1: %v", err)
	}
	if resp.Status != 504 {
		t.Errorf("final status = %d, want 504 (synthesized per-try-timeout)", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_test.upstream_rq_per_try_timeout", 4)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry", 3)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_limit_exceeded", 1)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_success", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_total", 4)
}

// TestRetryExecutorH1_PerTryTimeoutNotRetriable — a blocking backend under
// retry_on:connect-failure (NOT reset/5xx), per_try_timeout:50ms ⇒ the first
// attempt times out to a synthesized 504, perTryTimeoutRetriable() is FALSE (a
// per-try-timeout is a reset, not a connect-failure), so it returns immediately:
// one per_try_timeout, ZERO retries, NO retry_success.
func TestRetryExecutorH1_PerTryTimeoutNotRetriable(t *testing.T) {
	b := newScriptedBackend(t, func(int64) int { return 200 }, 5*time.Second)
	defer b.stop()

	c, reg := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicyPTT(t, "connect-failure", 3, 50*time.Millisecond)}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	resp, _, err := retryExecutorH1(req.Context(), a, req)
	if err != nil {
		t.Fatalf("retryExecutorH1: %v", err)
	}
	if resp.Status != 504 {
		t.Errorf("final status = %d, want 504 (synthesized per-try-timeout)", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_test.upstream_rq_per_try_timeout", 1)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_success", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_limit_exceeded", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_total", 1)
}

// TestRetryExecutorH1_PerTryTimeoutResetToken — a blocking backend under
// retry_on:reset, per_try_timeout:50ms ⇒ a per-try-timeout is a reset, so it
// retries to exhaustion: 4 attempts, final 504, per_try_timeout==4, retry==3.
func TestRetryExecutorH1_PerTryTimeoutResetToken(t *testing.T) {
	b := newScriptedBackend(t, func(int64) int { return 200 }, 5*time.Second)
	defer b.stop()

	c, reg := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicyPTT(t, "reset", 3, 50*time.Millisecond)}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	resp, _, err := retryExecutorH1(req.Context(), a, req)
	if err != nil {
		t.Fatalf("retryExecutorH1: %v", err)
	}
	if resp.Status != 504 {
		t.Errorf("final status = %d, want 504 (synthesized per-try-timeout)", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_test.upstream_rq_per_try_timeout", 4)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry", 3)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_limit_exceeded", 1)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_total", 4)
}

// TestRetryExecutorH1_PerTryTimeoutZeroByteStable — per_try_timeout==0 ⇒ no child
// ctx is ever created; a fast 200 backend behaves byte-identically to 42.1: final
// 200, per_try_timeout==0, retry==0. Guards the "0 ⇒ unchanged upstream path".
func TestRetryExecutorH1_PerTryTimeoutZeroByteStable(t *testing.T) {
	b := newScriptedBackend(t, func(int64) int { return 200 }, 0)
	defer b.stop()

	c, reg := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicyPTT(t, "5xx", 3, 0)}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	resp, _, err := retryExecutorH1(req.Context(), a, req)
	if err != nil {
		t.Fatalf("retryExecutorH1: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("final status = %d, want 200", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_test.upstream_rq_per_try_timeout", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_total", 1)
}

// TestRetryExecutorH1_FastResponseUnderLongTimeout — a fast 200 backend with a
// LONG per_try_timeout (5s) ⇒ the explicit cancel() makes attemptCtx.Err() ==
// Canceled (NOT DeadlineExceeded), so timedOut is FALSE: final 200, no
// per_try_timeout, no retry. Guards the explicit-cancel-correctness.
func TestRetryExecutorH1_FastResponseUnderLongTimeout(t *testing.T) {
	b := newScriptedBackend(t, func(int64) int { return 200 }, 0)
	defer b.stop()

	c, reg := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicyPTT(t, "5xx", 3, 5*time.Second)}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	resp, _, err := retryExecutorH1(req.Context(), a, req)
	if err != nil {
		t.Fatalf("retryExecutorH1: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("final status = %d, want 200", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_test.upstream_rq_per_try_timeout", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry", 0)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_total", 1)
}

// checkCounter asserts the named registry counter equals want (thin wrapper over
// the existing counterValue registry-introspection helper from router_test.go).
func checkCounter(t *testing.T, reg *stats.Registry, name string, want int64) {
	t.Helper()
	if got := counterValue(t, reg, name); got != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}

// TestRetryExecutorH2_Exhaustion — an always-503 in-process H2 backend under
// retry_on:gateway-error, num_retries:2 ⇒ 3 attempts (each a fresh DialH2), the
// final 503, and the increment ledger mirroring the H1 exhaustion case. The H2
// backend responds 503 on EVERY fresh connection, so each retry attempt re-dials
// and re-fails (the no-keepalive per-request-dial retry shape, ADR-0056).
func TestRetryExecutorH2_Exhaustion(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2Backend(t, pki, h2Backend503, []byte("resp:503"))
	defer func() { _ = ln.Close() }()

	c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	c.EnsureRetryStats()
	a := &routerActionH2{cluster: c, rp: mkRetryPolicy(t, "gateway-error", 2)}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, _, err := retryExecutorH2(ctx, a, h2RequestForTest())
	if err != nil {
		t.Fatalf("retryExecutorH2: %v", err)
	}
	if resp.Status != 503 {
		t.Errorf("final status = %d, want 503", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry", 2)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry_limit_exceeded", 1)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry_backoff_exponential", 2)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry_success", 0)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_total", 3)
}

// TestRetryExecutorH2_BodyReplay — the H2 analog of the H1 body-replay test.
// A POST whose req.Body []byte is buffered must re-present the FULL body on the
// retry attempt. The backend 503s on the first connection (attempt 0) then 200s
// on the retry (attempt 1) and records the request body bytes it received on
// EACH connection. Asserting both recorded bodies equal the payload PINs the
// documented buffered-body claim in retryExecutorH2: req.Body survives the retry
// unmutated (passed by value; no snapshot/reset needed). A future change that
// consumed or rewrote req.Body would make attempt-1's recorded body wrong.
func TestRetryExecutorH2_BodyReplay(t *testing.T) {
	const payload = "hello-retry-h2-body-payload"
	pki := mkH2BackendPKI(t)
	b, ln := startH2RecordingBackend(t, pki, func(i int64) int {
		if i == 0 {
			return 503
		}
		return 200
	})
	defer func() { _ = ln.Close() }()

	c, _ := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	c.EnsureRetryStats()
	a := &routerActionH2{cluster: c, rp: mkRetryPolicy(t, "5xx", 3)}

	req := h2.H2Request{
		Method:    "POST",
		Path:      "/x",
		Scheme:    "https",
		Authority: "alpha.envoy-go.test",
		Body:      []byte(payload),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, _, err := retryExecutorH2(ctx, a, req)
	if err != nil {
		t.Fatalf("retryExecutorH2: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("final status = %d, want 200 (recovered on the retry)", resp.Status)
	}
	bodies := b.recordedBodies()
	if len(bodies) != 2 {
		t.Fatalf("backend served %d connections, want 2 (initial 503 + 1 retry 200)", len(bodies))
	}
	for i, got := range bodies {
		if string(got) != payload {
			t.Errorf("attempt %d body = %q, want %q (full replay)", i, string(got), payload)
		}
	}
}

// TestRetryExecutorH2_CtxCancelNotRetried — a caller ctx-cancel mid-RoundTrip
// returns Status=0 + an *h2.Error sentinel. matches(0,false) is FALSE (0 ∉ 5xx /
// gateway-error / localOrigin), so the executor returns it AS-IS without ever
// retrying: a client cancel is NEVER retried. Proves upstream_rq_retry==0 and
// that the *h2.Error passes through unchanged.
func TestRetryExecutorH2_CtxCancelNotRetried(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2Backend(t, pki, h2BackendHang, nil)
	defer func() { _ = ln.Close() }()

	c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	c.EnsureRetryStats()
	// retry_on includes 5xx + gateway-error so a (hypothetical, wrong) retry on a
	// Status:0 outcome would be visibly counted — the assertion is meaningful.
	a := &routerActionH2{cluster: c, rp: mkRetryPolicy(t, "5xx, gateway-error", 3)}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after the dial+settings handshake so RoundTrip is blocked waiting on
	// the (never-arriving) response — the canonical client-cancel window.
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	resp, _, err := retryExecutorH2(ctx, a, h2RequestForTest())
	if err == nil {
		t.Fatal("retryExecutorH2 returned nil err; want the *h2.Error ctx-cancel sentinel passed through")
	}
	if _, ok := err.(*h2.Error); !ok {
		t.Fatalf("err is %T, want *h2.Error (ctx-cancel sentinel passed through unchanged)", err)
	}
	if resp.Status != 0 {
		t.Errorf("status = %d, want 0 (ctx-cancel sentinel)", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry", 0)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry_limit_exceeded", 0)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_total", 1)
}

// TestRetryExecutorH2_PerTryTimeoutExhaustion — an always-hanging in-process H2
// backend (accepts the stream, never responds) under a non-canceled parent ctx +
// retry_on:5xx, num_retries:2, per_try_timeout:50ms ⇒ every attempt fires the
// CHILD attemptCtx deadline (parent ctx alive), is overridden to a synthesized
// 504, classified retriable (5xx∋504), and counts toward num_retries ⇒ 3 attempts,
// final 504, the per-try-timeout ledger. The H2 mirror of
// TestRetryExecutorH1_PerTryTimeoutExhaustion (AMEND-PT4).
func TestRetryExecutorH2_PerTryTimeoutExhaustion(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2Backend(t, pki, h2BackendHang, nil)
	defer func() { _ = ln.Close() }()

	c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	c.EnsureRetryStats()
	a := &routerActionH2{cluster: c, rp: mkRetryPolicyPTT(t, "5xx", 2, 50*time.Millisecond)}

	resp, _, err := retryExecutorH2(context.Background(), a, h2RequestForTest())
	if err != nil {
		t.Fatalf("retryExecutorH2: %v", err)
	}
	if resp.Status != 504 {
		t.Errorf("final status = %d, want 504 (synthesized per-try-timeout)", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_per_try_timeout", 3)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry", 2)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry_limit_exceeded", 1)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry_success", 0)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_total", 3)
}

// TestRetryExecutorH2_ClientCancelWithPerTryTimeout — the 42.1 client-cancel
// invariant UNDER a per_try_timeout. A hanging backend + a parent ctx canceled
// after ~200ms + retry_on:5xx, num_retries:3, per_try_timeout:5s (the per-try is
// LONG so the PARENT cancel fires first). The driver returns Status:0 + an
// *h2.Error; the ctx.Err()!=nil guard makes timedOut FALSE, matches(0,false) is
// FALSE ⇒ the executor returns the Status:0 sentinel un-retried. Proves the
// per_try_timeout addition does NOT misclassify a client cancel as a per-try-
// timeout: upstream_rq_per_try_timeout==0, upstream_rq_retry==0.
func TestRetryExecutorH2_ClientCancelWithPerTryTimeout(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2Backend(t, pki, h2BackendHang, nil)
	defer func() { _ = ln.Close() }()

	c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	c.EnsureRetryStats()
	a := &routerActionH2{cluster: c, rp: mkRetryPolicyPTT(t, "5xx, gateway-error", 3, 5*time.Second)}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	resp, _, err := retryExecutorH2(ctx, a, h2RequestForTest())
	if err == nil {
		t.Fatal("retryExecutorH2 returned nil err; want the *h2.Error ctx-cancel sentinel passed through")
	}
	if _, ok := err.(*h2.Error); !ok {
		t.Fatalf("err is %T, want *h2.Error (ctx-cancel sentinel passed through unchanged)", err)
	}
	if resp.Status != 0 {
		t.Errorf("status = %d, want 0 (ctx-cancel sentinel, NOT a per-try-timeout)", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_per_try_timeout", 0)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry", 0)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry_limit_exceeded", 0)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_total", 1)
}

// TestRetryExecutorH2_PerTryTimeoutZeroByteStable — per_try_timeout==0 ⇒ no child
// ctx is ever created; an always-503 backend behaves byte-identically to 42.1:
// 3 attempts, final 503, per_try_timeout==0. Guards the "0 ⇒ unchanged upstream
// path" for the H2 executor.
func TestRetryExecutorH2_PerTryTimeoutZeroByteStable(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2Backend(t, pki, h2Backend503, []byte("resp:503"))
	defer func() { _ = ln.Close() }()

	c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	c.EnsureRetryStats()
	a := &routerActionH2{cluster: c, rp: mkRetryPolicyPTT(t, "5xx", 2, 0)}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, _, err := retryExecutorH2(ctx, a, h2RequestForTest())
	if err != nil {
		t.Fatalf("retryExecutorH2: %v", err)
	}
	if resp.Status != 503 {
		t.Errorf("final status = %d, want 503", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_per_try_timeout", 0)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry", 2)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry_limit_exceeded", 1)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_total", 3)
}

// TestH2WeightedClusterAction_RunsRetryExecutor — a weighted route with a single
// always-503 H2 entry + retry_policy ⇒ the per-entry closure threads rp through
// the H2ClusterAction closure switch and runs retryExecutorH2 (the retry counters
// fire for the weighted entry). Proves the weighted path dispatches to the
// executor when rp != nil (test c).
func TestH2WeightedClusterAction_RunsRetryExecutor(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2Backend(t, pki, h2Backend503, []byte("resp:503"))
	defer func() { _ = ln.Close() }()

	c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	c.EnsureRetryStats()

	sel := newWeightedSelectorWithRNG([]uint32{1}, newSeqRNG(0))
	wcs := []WeightedCluster{{Cluster: c}}
	act := H2WeightedClusterAction(wcs, nil, sel, mkRetryPolicy(t, "gateway-error", 2), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, _, err := act(ctx, h2RequestForTest())
	if err != nil {
		t.Fatalf("weighted H2 act: %v", err)
	}
	if resp.Status != 503 {
		t.Errorf("final status = %d, want 503", resp.Status)
	}
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry", 2)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_retry_limit_exceeded", 1)
	checkCounter(t, reg, "cluster.c_h2_backend.upstream_rq_total", 3)
}

func TestRetryParseRetryOn5xx(t *testing.T) {
	if got := parseRetryOn("5xx"); got != retry5xx {
		t.Fatalf("parseRetryOn(%q) = %d, want %d (retry5xx)", "5xx", got, retry5xx)
	}
}

func TestRetryParseRetryOnGatewayAndConnect(t *testing.T) {
	const in = "gateway-error, connect-failure"
	want := retryGatewayError | retryConnectFail
	if got := parseRetryOn(in); got != want {
		t.Fatalf("parseRetryOn(%q) = %d, want %d (gateway|connect)", in, got, want)
	}
}

func TestRetryParseRetryOnDeferredTokensIgnored(t *testing.T) {
	const in = "envoy-ratelimited grpc-internal foo"
	if got := parseRetryOn(in); got != 0 {
		t.Fatalf("parseRetryOn(%q) = %d, want 0 (all ignored)", in, got)
	}
}

func TestRetryParseRetryOnEmpty(t *testing.T) {
	if got := parseRetryOn(""); got != 0 {
		t.Fatalf("parseRetryOn(%q) = %d, want 0 (no tokens)", "", got)
	}
}

func TestRetryParseRetryOnEmptyTokensSkipped(t *testing.T) {
	// strings.FieldsFunc never yields empty tokens, so double separators
	// and leading/trailing commas must not produce spurious tokens. This
	// locks in that behavior so a future refactor to strings.Split (which
	// DOES yield empty tokens) can't silently regress.
	const in = "reset,,5xx"
	want := retryReset | retry5xx
	if got := parseRetryOn(in); got != want {
		t.Fatalf("parseRetryOn(%q) = %d, want %d (reset|5xx)", in, got, want)
	}
}

// TestRetryParseRetryOnResetBit — parseRetryOn("reset") sets retryReset and NOT
// retryConnectFail; parseRetryOn("connect-failure") sets retryConnectFail and NOT
// retryReset; parseRetryOn("connect-failure reset") sets both.
func TestRetryParseRetryOnResetBit(t *testing.T) {
	t.Run("reset-only", func(t *testing.T) {
		got := parseRetryOn("reset")
		if got&retryReset == 0 {
			t.Errorf("parseRetryOn(%q): retryReset bit not set (got %d)", "reset", got)
		}
		if got&retryConnectFail != 0 {
			t.Errorf("parseRetryOn(%q): retryConnectFail bit set unexpectedly (got %d)", "reset", got)
		}
	})
	t.Run("connect-failure-only", func(t *testing.T) {
		got := parseRetryOn("connect-failure")
		if got&retryConnectFail == 0 {
			t.Errorf("parseRetryOn(%q): retryConnectFail bit not set (got %d)", "connect-failure", got)
		}
		if got&retryReset != 0 {
			t.Errorf("parseRetryOn(%q): retryReset bit set unexpectedly (got %d)", "connect-failure", got)
		}
	})
	t.Run("both", func(t *testing.T) {
		got := parseRetryOn("connect-failure reset")
		if got&retryConnectFail == 0 {
			t.Errorf("parseRetryOn(%q): retryConnectFail bit not set (got %d)", "connect-failure reset", got)
		}
		if got&retryReset == 0 {
			t.Errorf("parseRetryOn(%q): retryReset bit not set (got %d)", "connect-failure reset", got)
		}
	})
}

// TestRetryMatchesLocalOriginBothTokens — matches(503, localOrigin=true) is TRUE
// for a policy parsed from "connect-failure" AND for one from "reset". A genuine
// dial-refusal must still retry under both tokens (42.1 behavior preserved).
func TestRetryMatchesLocalOriginBothTokens(t *testing.T) {
	for _, tok := range []string{"connect-failure", "reset"} {
		tok := tok
		t.Run(tok, func(t *testing.T) {
			rp := mkRetryPolicy(t, tok, 1)
			if !rp.matches(503, true) {
				t.Errorf("matches(503, localOrigin=true) = false for retry_on:%q; want true (dial-refusal retries)", tok)
			}
		})
	}
}

// TestPerTryTimeoutRetriable — perTryTimeoutRetriable() is TRUE for policies from
// "5xx", "gateway-error", "reset", and "retriable-status-codes" with 504 listed;
// FALSE for "connect-failure"-alone, empty retry_on, and "retriable-status-codes"
// with only 500 (504 not listed).
func TestPerTryTimeoutRetriable(t *testing.T) {
	t.Run("5xx-true", func(t *testing.T) {
		rp := mkRetryPolicy(t, "5xx", 1)
		if !rp.perTryTimeoutRetriable() {
			t.Errorf("perTryTimeoutRetriable() = false for retry_on:5xx; want true")
		}
	})
	t.Run("gateway-error-true", func(t *testing.T) {
		rp := mkRetryPolicy(t, "gateway-error", 1)
		if !rp.perTryTimeoutRetriable() {
			t.Errorf("perTryTimeoutRetriable() = false for retry_on:gateway-error; want true")
		}
	})
	t.Run("reset-true", func(t *testing.T) {
		rp := mkRetryPolicy(t, "reset", 1)
		if !rp.perTryTimeoutRetriable() {
			t.Errorf("perTryTimeoutRetriable() = false for retry_on:reset; want true")
		}
	})
	t.Run("retriable-status-codes-504-true", func(t *testing.T) {
		rp, err := NewRetryPolicy("retriable-status-codes", 1, []uint32{504}, time.Microsecond, time.Millisecond, 0)
		if err != nil {
			t.Fatalf("NewRetryPolicy: %v", err)
		}
		if !rp.perTryTimeoutRetriable() {
			t.Errorf("perTryTimeoutRetriable() = false for retry_on:retriable-status-codes with 504; want true")
		}
	})
	t.Run("connect-failure-alone-false", func(t *testing.T) {
		rp := mkRetryPolicy(t, "connect-failure", 1)
		if rp.perTryTimeoutRetriable() {
			t.Errorf("perTryTimeoutRetriable() = true for retry_on:connect-failure-alone; want false")
		}
	})
	t.Run("empty-false", func(t *testing.T) {
		rp := mkRetryPolicy(t, "", 1)
		if rp.perTryTimeoutRetriable() {
			t.Errorf("perTryTimeoutRetriable() = true for empty retry_on; want false")
		}
	})
	t.Run("retriable-status-codes-500-only-false", func(t *testing.T) {
		rp, err := NewRetryPolicy("retriable-status-codes", 1, []uint32{500}, time.Microsecond, time.Millisecond, 0)
		if err != nil {
			t.Fatalf("NewRetryPolicy: %v", err)
		}
		if rp.perTryTimeoutRetriable() {
			t.Errorf("perTryTimeoutRetriable() = true for retriable-status-codes with only 500; want false (504 not listed)")
		}
	})
}

func TestRetryMatches(t *testing.T) {
	tests := []struct {
		name           string
		on             retryOnBits
		retriableCodes map[int]bool
		status         int
		localOrigin    bool
		want           bool
	}{
		{name: "5xx/503", on: retry5xx, status: 503, want: true},
		{name: "5xx/200", on: retry5xx, status: 200, want: false},
		{name: "gateway-error/502", on: retryGatewayError, status: 502, want: true},
		{name: "gateway-error/500", on: retryGatewayError, status: 500, want: false},
		{name: "connect-failure/502-local", on: retryConnectFail, status: 502, localOrigin: true, want: true},
		{name: "connect-failure/502-upstream", on: retryConnectFail, status: 502, localOrigin: false, want: false},
		{name: "retriable-status-codes/500", on: retryStatusCodes, retriableCodes: map[int]bool{500: true}, status: 500, want: true},
		{name: "retriable-status-codes/503", on: retryStatusCodes, retriableCodes: map[int]bool{500: true}, status: 503, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rp := &RetryPolicy{on: tc.on, retriableCodes: tc.retriableCodes}
			if got := rp.matches(tc.status, tc.localOrigin); got != tc.want {
				t.Fatalf("matches(%d, %v) = %v, want %v", tc.status, tc.localOrigin, got, tc.want)
			}
		})
	}
}

func TestRetryPolicyDefaults(t *testing.T) {
	// Unset (zero) base/max intervals resolve to the AMEND-RT6 defaults:
	// base=25ms, max=10×base=250ms. num_retries threads through verbatim.
	rp, err := NewRetryPolicy("5xx", 3, nil, 0, 0, 0)
	if err != nil {
		t.Fatalf("NewRetryPolicy(...) unexpected error: %v", err)
	}
	if rp.numRetries != 3 {
		t.Errorf("numRetries = %d, want 3", rp.numRetries)
	}
	if rp.baseInterval != 25*time.Millisecond {
		t.Errorf("baseInterval = %v, want 25ms", rp.baseInterval)
	}
	if rp.maxInterval != 250*time.Millisecond {
		t.Errorf("maxInterval = %v, want 250ms", rp.maxInterval)
	}
	// The retry_on bit must still be parsed into the merged struct.
	if rp.on&retry5xx == 0 {
		t.Errorf("on = %d, want retry5xx bit set", rp.on)
	}
}

func TestRetryPolicyExplicitIntervals(t *testing.T) {
	// Explicit, valid base<max must be preserved verbatim (no default override).
	rp, err := NewRetryPolicy("", 2, []uint32{500, 503}, 50*time.Millisecond, 400*time.Millisecond, 0)
	if err != nil {
		t.Fatalf("NewRetryPolicy(...) unexpected error: %v", err)
	}
	if rp.baseInterval != 50*time.Millisecond {
		t.Errorf("baseInterval = %v, want 50ms", rp.baseInterval)
	}
	if rp.maxInterval != 400*time.Millisecond {
		t.Errorf("maxInterval = %v, want 400ms", rp.maxInterval)
	}
	if !rp.retriableCodes[500] || !rp.retriableCodes[503] {
		t.Errorf("retriableCodes = %v, want {500,503} set", rp.retriableCodes)
	}
}

func TestRetryPolicyMaxLessThanBaseRejected(t *testing.T) {
	// max < base is the D-S42-1 reject arm: non-nil error, message exactly the
	// unprefixed form (Task 4's buildRouterAction adds the route prefix).
	rp, err := NewRetryPolicy("5xx", 1, nil, 100*time.Millisecond, 50*time.Millisecond, 0)
	if err == nil {
		t.Fatalf("NewRetryPolicy(max<base) = %+v, nil; want error", rp)
	}
	if rp != nil {
		t.Errorf("NewRetryPolicy(max<base) returned non-nil policy %+v alongside error", rp)
	}
	const want = "max_interval must be greater than or equal to the base_interval"
	if err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
}

func TestRetryPolicyZeroNumRetries(t *testing.T) {
	// num_retries==0 ⇒ no retries; threaded verbatim (not coerced to a default).
	rp, err := NewRetryPolicy("5xx", 0, nil, 0, 0, 0)
	if err != nil {
		t.Fatalf("NewRetryPolicy(...) unexpected error: %v", err)
	}
	if rp.numRetries != 0 {
		t.Errorf("numRetries = %d, want 0", rp.numRetries)
	}
}

// TestRetryPolicyPerTryTimeout — NewRetryPolicy accepts a positive per_try_timeout
// and stores it verbatim; zero means "no bound" (also accepted); negative is rejected.
func TestRetryPolicyPerTryTimeout(t *testing.T) {
	t.Run("positive-stored", func(t *testing.T) {
		// 250ms per-try timeout: accepted, accessor returns the value.
		rp, err := NewRetryPolicy("5xx", 3, nil, 0, 0, 250*time.Millisecond)
		if err != nil {
			t.Fatalf("NewRetryPolicy(...) unexpected error: %v", err)
		}
		if got := rp.PerTryTimeout(); got != 250*time.Millisecond {
			t.Errorf("PerTryTimeout() = %v, want 250ms", got)
		}
	})
	t.Run("negative-rejected", func(t *testing.T) {
		// per_try_timeout < 0 must be rejected (D-S42-2).
		rp, err := NewRetryPolicy("5xx", 1, nil, 0, 0, -1)
		if err == nil {
			t.Fatalf("NewRetryPolicy(perTryTimeout=-1) = %+v, nil; want error", rp)
		}
		if rp != nil {
			t.Errorf("NewRetryPolicy(perTryTimeout=-1) returned non-nil policy %+v alongside error", rp)
		}
		if err.Error() != ErrMsgPerTryTimeoutNegative {
			t.Errorf("err = %q, want %q", err.Error(), ErrMsgPerTryTimeoutNegative)
		}
	})
	t.Run("zero-accepted-no-bound", func(t *testing.T) {
		// per_try_timeout == 0 means "no bound" and is NOT an error.
		rp, err := NewRetryPolicy("5xx", 1, nil, 0, 0, 0)
		if err != nil {
			t.Fatalf("NewRetryPolicy(perTryTimeout=0) unexpected error: %v", err)
		}
		if got := rp.PerTryTimeout(); got != 0 {
			t.Errorf("PerTryTimeout() = %v, want 0 (no bound)", got)
		}
	})
	t.Run("max-less-than-base-still-rejected-with-6th-arg", func(t *testing.T) {
		// The existing max<base reject must still fire when perTryTimeout==0 (6th arg present).
		rp, err := NewRetryPolicy("5xx", 1, nil, 100*time.Millisecond, 50*time.Millisecond, 0)
		if err == nil {
			t.Fatalf("NewRetryPolicy(max<base, perTryTimeout=0) = %+v, nil; want error", rp)
		}
		if err.Error() != ErrMsgMaxIntervalBelowBase {
			t.Errorf("err = %q, want %q", err.Error(), ErrMsgMaxIntervalBelowBase)
		}
	})
}

func TestRetryBackoffBounds(t *testing.T) {
	// backoff is full-jitter: every sample must land in [0, maxInterval]. Run
	// each attempt many times to exercise the random distribution; assert BOUNDS
	// (not a fixed value). n=40 exercises the int64 shift-overflow clamp.
	rp, err := NewRetryPolicy("5xx", 5, nil, 25*time.Millisecond, 250*time.Millisecond, 0)
	if err != nil {
		t.Fatalf("NewRetryPolicy(...) unexpected error: %v", err)
	}
	for _, n := range []int{1, 2, 3, 4, 5, 40} {
		for i := 0; i < 1000; i++ {
			d := rp.backoff(n)
			if d < 0 {
				t.Fatalf("backoff(%d) = %v, want >= 0", n, d)
			}
			if d > rp.maxInterval {
				t.Fatalf("backoff(%d) = %v, want <= maxInterval (%v)", n, d, rp.maxInterval)
			}
		}
	}
}
