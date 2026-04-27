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

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

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
