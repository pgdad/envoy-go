package hcm

// h2dispatch_test.go — Phase 06.1 Task 11 carries forward the 05.2 REVIEW
// Minor M-9 finding ("Missing log line in `h2RouterActionAdapter.WriteH2`
// on `doH2` error") per SPEC §13.1. The fix lives in h2dispatch.go where
// h2RouterActionAdapter is defined; the test file location settled at
// SPEC §11.4 + §12 #5 ("internal/filter/hcm/h2/router_action_test.go (new
// or appended to existing test file)") points at the h2 sub-package, but
// h2RouterActionAdapter is unexported in package hcm so the test MUST live
// in package hcm; this file substitutes for the planned h2-sub-package test
// per the SPEC's "appended to existing test file" relaxation. The deviation
// is recorded in PROGRESS Task 11 Notes.
//
// Phase 06.2 Task 13: H2 emit-deferral tests added below (TestH2DirectResponse*
// and TestRouterActionH2_EmitsAccessLog*).

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TestH2RouterActionAdapter_WriteH2_LogsOnDoH2Error covers the M-9 carry-
// forward (SPEC §11.4 + §13.1). When h2RouterActionAdapter.WriteH2 invokes
// doH2 and the call returns an error, a log line "h2: doH2 error: <err>"
// is emitted to the standard logger BEFORE the function returns AND the
// underlying error is propagated to the caller verbatim.
//
// Test mechanism: the adapter exposes a `doH2Fn` function-typed field
// (default-bound to a.a.doH2 at construction) so the test can substitute
// a sentinel-failing function. log.SetOutput captures the output; the
// assertion checks the captured string for the expected prefix + the
// underlying error text.
func TestH2RouterActionAdapter_WriteH2_LogsOnDoH2Error(t *testing.T) {
	// Capture log output.
	var logBuf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	// Build an adapter wrapping a no-op routerActionH2 (the cluster is
	// unused because we substitute doH2Fn before calling WriteH2). The
	// Filter pointer is required because the adapter Inc's downstream_rq_<Nxx>
	// on the response-class hook before logging — but on the doH2 error
	// path the returned status is 0 (no finalized response) so the bucket
	// Inc is skipped and we only need a non-nil Filter for the field
	// reference; the metric pointers themselves are unused on this code
	// path. We allocate them anyway to keep the assertion robust against
	// future refactors that might Inc on the error path too.
	r := stats.NewRegistry()
	prefix := "http.test_m9."
	f := &Filter{
		downstreamRqTotal: r.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   r.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   r.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   r.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   r.NewCounter(prefix + "downstream_rq_5xx"),
	}
	a := &h2RouterActionAdapter{a: &routerActionH2{cluster: nil}, f: f}

	// Inject a sentinel-failing doH2 substitute so we don't dial anything.
	// status==0 mirrors the production ctx-cancel / unrecoverable-error
	// shape; the M-9 test cares about the log line + error propagation.
	sentinel := errors.New("sentinel doH2 failure for M-9 test")
	a.doH2Fn = func(_ context.Context, _ h2.H2Request, _ h2.StreamWriter) (int, error) {
		return 0, sentinel
	}

	// Drive WriteH2; expect the sentinel error to propagate.
	err := a.WriteH2(context.Background(), h2.H2Request{}, &captureH2Writer{})
	if err == nil {
		t.Fatal("WriteH2 returned nil; want sentinel error to propagate")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("WriteH2 returned %v; want sentinel error", err)
	}

	// Assert the M-9 log line was emitted.
	got := logBuf.String()
	if !strings.Contains(got, "h2: doH2 error:") {
		t.Errorf("missing M-9 log prefix in captured log output; got: %q", got)
	}
	if !strings.Contains(got, sentinel.Error()) {
		t.Errorf("missing sentinel error text in captured log output; got: %q", got)
	}
}

// TestH2RouterActionAdapter_WriteH2_NoLogOnSuccess verifies that on a
// successful doH2 call, no "h2: doH2 error:" line is emitted (negative
// confirmation — the M-9 fix must not be over-eager).
func TestH2RouterActionAdapter_WriteH2_NoLogOnSuccess(t *testing.T) {
	var logBuf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	r := stats.NewRegistry()
	prefix := "http.test_m9_success."
	f := &Filter{
		downstreamRqTotal: r.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   r.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   r.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   r.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   r.NewCounter(prefix + "downstream_rq_5xx"),
	}
	a := &h2RouterActionAdapter{a: &routerActionH2{cluster: nil}, f: f}
	a.doH2Fn = func(_ context.Context, _ h2.H2Request, _ h2.StreamWriter) (int, error) {
		return 200, nil
	}

	if err := a.WriteH2(context.Background(), h2.H2Request{}, &captureH2Writer{}); err != nil {
		t.Fatalf("WriteH2 returned %v; want nil on doH2 success", err)
	}
	if got := logBuf.String(); strings.Contains(got, "h2: doH2 error:") {
		t.Errorf("M-9 log line emitted on doH2 success path; got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Phase 06.2 Task 13 — H2 emit-deferral tests
// ---------------------------------------------------------------------------

// newFilterWithSink builds a minimal *Filter with the given accesslog sink
// wired in; allocates required metric fields so downstreamStatusClassCounter
// doesn't nil-deref.
func newFilterWithSink(t *testing.T, cs accesslog.Sink) *Filter {
	t.Helper()
	r := stats.NewRegistry()
	prefix := "http.test_t13."
	return &Filter{
		downstreamRqTotal: r.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   r.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   r.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   r.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   r.NewCounter(prefix + "downstream_rq_5xx"),
		accessLog:         []accesslog.Sink{cs},
	}
}

// TestH2DirectResponseAdapter_WriteH2_EmitsAccessLog verifies that
// h2DirectResponseAdapter.WriteH2 submits one access-log record with the
// correct ResponseCode and BytesSent.
func TestH2DirectResponseAdapter_WriteH2_EmitsAccessLog(t *testing.T) {
	cs := &emitCaptureSink{}
	f := newFilterWithSink(t, cs)
	a := &h2DirectResponseAdapter{
		a: &directResponseAction{status: 200, bodyText: "OK\n", filter: f},
		f: f,
	}
	req := h2.H2Request{Method: "GET", Path: "/health", Authority: "localhost"}
	if err := a.WriteH2(context.Background(), req, &captureH2Writer{}); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	if len(cs.recs) != 1 {
		t.Fatalf("captured %d records, want 1", len(cs.recs))
	}
	rec := cs.recs[0]
	if rec.ResponseCode != 200 {
		t.Errorf("ResponseCode = %d, want 200", rec.ResponseCode)
	}
	if rec.BytesSent != 3 {
		t.Errorf("BytesSent = %d, want 3 (len(\"OK\\n\"))", rec.BytesSent)
	}
	if rec.Protocol != "HTTP/2.0" {
		t.Errorf("Protocol = %q, want HTTP/2.0", rec.Protocol)
	}
	if rec.UpstreamHost != "" {
		t.Errorf("UpstreamHost = %q, want empty (direct_response)", rec.UpstreamHost)
	}
}

// TestRouterActionH2_DoH2_EmitsAccessLog_HappyPath verifies that
// routerActionH2.doH2 submits one access-log record with a non-empty
// UpstreamHost and BytesSent > 0 when the upstream responds successfully.
func TestRouterActionH2_DoH2_EmitsAccessLog_HappyPath(t *testing.T) {
	pki := mkH2BackendPKI(t)
	body := []byte("upstream-ok\n")
	ln := startH2Backend(t, pki, h2BackendOK, body)
	defer func() { _ = ln.Close() }()

	cs := &emitCaptureSink{}
	f := newFilterWithSink(t, cs)
	c := h2EndpointCluster(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c, filter: f}

	w := &captureH2Writer{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := a.doH2(ctx, h2RequestForTest(), w); err != nil {
		t.Fatalf("doH2: %v", err)
	}

	if len(cs.recs) != 1 {
		t.Fatalf("captured %d records, want 1", len(cs.recs))
	}
	rec := cs.recs[0]
	if rec.ResponseCode != 200 {
		t.Errorf("ResponseCode = %d, want 200", rec.ResponseCode)
	}
	if rec.BytesSent <= 0 {
		t.Errorf("BytesSent = %d, want > 0", rec.BytesSent)
	}
	if rec.UpstreamHost == "" {
		t.Errorf("UpstreamHost is empty, want non-empty (routed H2 request)")
	}
	if rec.Protocol != "HTTP/2.0" {
		t.Errorf("Protocol = %q, want HTTP/2.0", rec.Protocol)
	}
}

// TestRouterActionH2_DoH2_EmitsAccessLog_DialFailure verifies that
// routerActionH2.doH2 emits an access-log record with status 502 and empty
// UpstreamHost on the dial-failure path.
func TestRouterActionH2_DoH2_EmitsAccessLog_DialFailure(t *testing.T) {
	pki := mkH2BackendPKI(t)
	cs := &emitCaptureSink{}
	f := newFilterWithSink(t, cs)
	// Port 1 is always rejected, so DialH2 fails.
	c := h2EndpointCluster(t, "127.0.0.1:1", pki)
	a := &routerActionH2{cluster: c, filter: f}

	w := &captureH2Writer{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = a.doH2(ctx, h2RequestForTest(), w)

	if len(cs.recs) != 1 {
		t.Fatalf("captured %d records, want 1", len(cs.recs))
	}
	rec := cs.recs[0]
	if rec.ResponseCode != 502 {
		t.Errorf("ResponseCode = %d, want 502 (dial-failure local-reply)", rec.ResponseCode)
	}
	if rec.UpstreamHost != "" {
		t.Errorf("UpstreamHost = %q, want empty on dial failure", rec.UpstreamHost)
	}
}

// TestRouterActionH2_DoH2_CtxCancel_SkipsEmit verifies that a ctx-cancel
// during doH2 results in zero access-log records (statusForHCM==0 sentinel
// skips emission per SPEC §2.1). This test mirrors the existing
// TestRouterActionH2_CtxCancelEmitsRSTStreamCancel test but asserts the
// access-log side-effect.
func TestRouterActionH2_DoH2_CtxCancel_SkipsEmit(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2Backend(t, pki, h2BackendHang, nil)
	defer func() { _ = ln.Close() }()

	cs := &emitCaptureSink{}
	f := newFilterWithSink(t, cs)
	c := h2EndpointCluster(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c, filter: f}

	w := &captureH2Writer{}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	_, err := a.doH2(ctx, h2RequestForTest(), w)
	if err == nil {
		t.Fatal("doH2 returned nil; want stream-scoped CANCEL error")
	}

	// ctx-cancel → statusForHCM==0 → emitAccessLogH2 skips emission.
	if len(cs.recs) != 0 {
		t.Errorf("captured %d records, want 0 (ctx-cancel skips emission per SPEC §2.1)", len(cs.recs))
	}
}
