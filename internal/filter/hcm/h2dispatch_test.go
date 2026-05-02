package hcm

// h2dispatch_test.go — Phase 07.1 Task 16 rewrite. The pre-Task-16 file
// asserted directly against h2RouterActionAdapter / h2DirectResponseAdapter /
// routerActionH2 — all of which were moved to the router package (Task 11)
// or replaced by the chain-mediated dispatch path (Task 16). This file
// exercises the post-Task-16 surface: h2Dispatcher.Match → chainDispatchAction.WriteH2
// runs the per-stream FilterChain, invokes RunAction on the terminal router
// filter, emits the access-log record from the chain-completion hook, and Inc's
// the HCM-scope downstream_rq_<Nxx> bucket once per finalized status.
//
// The build-tag gate from Task 15's deliberate-red state (envoy_go_hcm_h2_legacy_tests)
// is GONE; this file builds and runs unconditionally.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/http2/hpack"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/stats"
)

// captureH2Writer is a fake h2.StreamWriter that records every call. Mirrors
// the helper in router/router_h2_test.go (kept package-private here so the
// hcm package's tests do not depend on the router package's test helpers,
// which are unexported).
type captureH2Writer struct {
	mu        sync.Mutex
	headers   [][]hpack.HeaderField
	data      [][]byte
	endStream []bool
	order     []string
}

func (c *captureH2Writer) WriteHeaders(headers []hpack.HeaderField, endStream bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]hpack.HeaderField, len(headers))
	copy(cp, headers)
	c.headers = append(c.headers, cp)
	c.endStream = append(c.endStream, endStream)
	c.order = append(c.order, "headers")
	return nil
}

func (c *captureH2Writer) WriteData(b []byte, endStream bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := append([]byte(nil), b...)
	c.data = append(c.data, cp)
	c.endStream = append(c.endStream, endStream)
	c.order = append(c.order, "data")
	return nil
}

// statusOf returns the :status value from the first headers call, or "" if
// no headers were recorded.
func (c *captureH2Writer) statusOf() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.headers) == 0 {
		return ""
	}
	for _, h := range c.headers[0] {
		if h.Name == ":status" {
			return h.Value
		}
	}
	return ""
}

// newH2DispatchFilter builds a *Filter wired with the given route table +
// chain config + access-log sinks. The 5 HCM-scope per-instance metric
// pointers are allocated against a fresh registry so downstreamStatusClassCounter
// returns non-nil counters that can be Inc'd without a nil-deref.
func newH2DispatchFilter(t *testing.T, table *routeTable, chainConfig []chainEntry, sinks []accesslog.Sink) *Filter {
	t.Helper()
	r := stats.NewRegistry()
	prefix := "http.test_h2dispatch."
	return &Filter{
		table:             table,
		statPrefix:        "test_h2dispatch",
		downstreamRqTotal: r.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   r.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   r.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   r.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   r.NewCounter(prefix + "downstream_rq_5xx"),
		accessLog:         sinks,
		chainConfig:       chainConfig,
	}
}

// routerOnlyChain returns a single-entry chainConfig with the router as the
// terminal filter. Used by tests that don't need a multi-filter chain.
func routerOnlyChain(t *testing.T) []chainEntry {
	t.Helper()
	rfFactory, err := router.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	return []chainEntry{{name: "envoy.filters.http.router", factory: rfFactory}}
}

// TestH2Dispatcher_Match_DirectResponse_RunsChainAndEmitsAccessLog covers the
// chain-mediated H2 dispatch path for a matched direct_response route. The
// chainDispatchAction allocates a per-stream FilterChain, drives decode
// iteration through the terminal router, invokes RunAction (which calls the
// directResponseAction's writeH2 via H2Action), and emits a single access-log
// record from the chain-completion hook.
//
// Asserted:
//   - WriteH2 returns nil (no action error).
//   - The capture writer received HEADERS+DATA with :status=200.
//   - One access-log record was submitted with ResponseCode=200, Protocol=HTTP/2.0,
//     UpstreamHost=empty (direct_response).
//   - downstream_rq_2xx was Inc'd to 1.
func TestH2Dispatcher_Match_DirectResponse_RunsChainAndEmitsAccessLog(t *testing.T) {
	cs := &emitCaptureSink{}
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	f := newH2DispatchFilter(t, tt, routerOnlyChain(t), []accesslog.Sink{cs})

	disp := newH2Dispatcher(f)
	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/2.0"
	action, ok := disp.Match(req)
	if !ok {
		t.Fatal("Match returned ok=false; want true on matched route")
	}

	w := &captureH2Writer{}
	h2req := h2.H2Request{Method: "GET", Path: "/health", Authority: "localhost"}
	if err := action.WriteH2(context.Background(), h2req, w); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	if got := w.statusOf(); got != "200" {
		t.Errorf(":status = %q, want 200", got)
	}

	if len(cs.recs) != 1 {
		t.Fatalf("captured %d access-log records, want 1", len(cs.recs))
	}
	rec := cs.recs[0]
	if rec.ResponseCode != 200 {
		t.Errorf("ResponseCode = %d, want 200", rec.ResponseCode)
	}
	if rec.Protocol != "HTTP/2.0" {
		t.Errorf("Protocol = %q, want HTTP/2.0", rec.Protocol)
	}
	if rec.UpstreamHost != "" {
		t.Errorf("UpstreamHost = %q, want empty (direct_response)", rec.UpstreamHost)
	}
	if rec.BytesSent != 3 {
		t.Errorf("BytesSent = %d, want 3 (len(\"OK\\n\"))", rec.BytesSent)
	}

	if got := f.downstreamRq2xx.Load(); got != 1 {
		t.Errorf("downstream_rq_2xx = %d, want 1", got)
	}
	if got := f.downstreamRqTotal.Load(); got != 1 {
		t.Errorf("downstream_rq_total = %d, want 1", got)
	}
}

// TestH2Dispatcher_Match_NoMatch_Synthesizes404 covers the no-match path: the
// dispatcher returns a chainDispatchAction whose routeIdx=-1 short-circuits
// chain construction and writes a 404 directly to the H2 stream writer + emits
// the access-log record. Mirrors the H1 connection.go dispatchRequest no-match
// branch.
//
// Asserted:
//   - WriteH2 returns nil (404 synthesis is unconditional success at the writer).
//   - The capture writer received HEADERS+DATA with :status=404.
//   - One access-log record was submitted with ResponseCode=404.
//   - downstream_rq_4xx was Inc'd.
func TestH2Dispatcher_Match_NoMatch_Synthesizes404(t *testing.T) {
	cs := &emitCaptureSink{}
	tt := &routeTable{routes: []routeEntry{
		// No route matches "/nope" — empty table.
	}}
	f := newH2DispatchFilter(t, tt, routerOnlyChain(t), []accesslog.Sink{cs})

	disp := newH2Dispatcher(f)
	req, _ := http.NewRequest("GET", "/nope", nil)
	req.Proto = "HTTP/2.0"
	action, ok := disp.Match(req)
	if !ok {
		t.Fatal("Match returned ok=false; want true even on no-match (synthesizes 404)")
	}

	w := &captureH2Writer{}
	h2req := h2.H2Request{Method: "GET", Path: "/nope"}
	if err := action.WriteH2(context.Background(), h2req, w); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	if got := w.statusOf(); got != "404" {
		t.Errorf(":status = %q, want 404", got)
	}

	if len(cs.recs) != 1 {
		t.Fatalf("captured %d access-log records, want 1", len(cs.recs))
	}
	if cs.recs[0].ResponseCode != 404 {
		t.Errorf("ResponseCode = %d, want 404", cs.recs[0].ResponseCode)
	}
	if got := f.downstreamRq4xx.Load(); got != 1 {
		t.Errorf("downstream_rq_4xx = %d, want 1", got)
	}
}

// faultyH2Action is a router.H2Action that returns a sentinel error to drive
// the M-9 carry-forward log-line assertion. status=0 mirrors the production
// ctx-cancel / unrecoverable-error shape.
func faultyH2Action(sentinel error) router.H2Action {
	return func(_ context.Context, _ h2.H2Request, _ h2.StreamWriter) (int, int64, cluster.Endpoint, error) {
		return 0, 0, cluster.Endpoint{}, sentinel
	}
}

// faultyAction is a routeAction whose asRouterActionH2 returns a faultyH2Action
// closure. asRouterAction returns a no-op H1 action so the routeAction
// interface is satisfied; do() is never invoked on the H2 dispatch path
// (chain-mediated dispatch goes through asRouterActionH2 + RunAction).
type faultyAction struct {
	sentinel error
}

func (a *faultyAction) do(context.Context, *http.Request, *bufio.Writer) (int, error) {
	return 500, nil
}
func (a *faultyAction) asRouterAction() router.Action {
	return func(context.Context, *http.Request, *bufio.Writer) (int, int64, cluster.Endpoint, error) {
		return 500, 0, cluster.Endpoint{}, nil
	}
}
func (a *faultyAction) asRouterActionH2() router.H2Action { return faultyH2Action(a.sentinel) }

// TestH2Dispatcher_ActionError_LogsM9 verifies the M-9 carry-forward (SPEC §13.1):
// when the H2Action returns an error, chainDispatchAction.WriteH2 emits a
// "h2: action error: <err>" log line BEFORE returning the error. Mirrors the
// pre-Task-16 h2RouterActionAdapter "h2: doH2 error" log line; renamed to
// "action error" since the chain-mediated dispatch path's terminal invocation
// is rf.RunAction (not directly doH2).
//
// Test mechanism: a stub routeAction whose asRouterActionH2 returns a sentinel-
// failing closure. log.SetOutput captures; assertion checks the captured
// string for the expected prefix + the underlying error text.
func TestH2Dispatcher_ActionError_LogsM9(t *testing.T) {
	var logBuf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	sentinel := errors.New("sentinel action failure for M-9 test")
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/fail"), action: &faultyAction{sentinel: sentinel}},
	}}
	f := newH2DispatchFilter(t, tt, routerOnlyChain(t), nil /* no sinks */)

	disp := newH2Dispatcher(f)
	req, _ := http.NewRequest("GET", "/fail", nil)
	req.Proto = "HTTP/2.0"
	action, ok := disp.Match(req)
	if !ok {
		t.Fatal("Match returned ok=false; want true on matched route")
	}

	w := &captureH2Writer{}
	h2req := h2.H2Request{Method: "GET", Path: "/fail"}
	err := action.WriteH2(context.Background(), h2req, w)
	if err == nil {
		t.Fatal("WriteH2 returned nil; want sentinel error to propagate")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("WriteH2 returned %v; want sentinel error", err)
	}

	got := logBuf.String()
	if !strings.Contains(got, "h2: action error:") {
		t.Errorf("missing M-9 log prefix in captured log output; got: %q", got)
	}
	if !strings.Contains(got, sentinel.Error()) {
		t.Errorf("missing sentinel error text in captured log output; got: %q", got)
	}
}

// TestH2Dispatcher_CtxCancel_Status0_SkipsAccessLog verifies the H2 ctx-cancel
// sentinel discipline (SPEC §2.1 last bullet): when the H2Action returns
// status==0 (the canonical "ctx canceled, no terminating status" shape), the
// chain-completion access-log emit hook is a no-op. Mirrors the pre-Task-16
// TestRouterActionH2_DoH2_CtxCancel_SkipsEmit assertion.
func TestH2Dispatcher_CtxCancel_Status0_SkipsAccessLog(t *testing.T) {
	cs := &emitCaptureSink{}
	sentinel := h2.NewStreamError(h2.ErrCancel, 0, "ctx canceled (test)")
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/cancel"), action: &faultyAction{sentinel: sentinel}},
	}}
	f := newH2DispatchFilter(t, tt, routerOnlyChain(t), []accesslog.Sink{cs})

	disp := newH2Dispatcher(f)
	req, _ := http.NewRequest("GET", "/cancel", nil)
	req.Proto = "HTTP/2.0"
	action, ok := disp.Match(req)
	if !ok {
		t.Fatal("Match: ok=false")
	}

	w := &captureH2Writer{}
	h2req := h2.H2Request{Method: "GET", Path: "/cancel"}
	if err := action.WriteH2(context.Background(), h2req, w); err == nil {
		t.Fatal("WriteH2 returned nil; want sentinel error")
	}

	if len(cs.recs) != 0 {
		t.Errorf("captured %d access-log records, want 0 (status==0 sentinel skips emit per SPEC §2.1)", len(cs.recs))
	}
	// status==0 also skips the bucket Inc per the chain-completion hook's
	// "status > 0" guard.
	for _, code := range []int{2, 3, 4, 5} {
		var c *stats.Counter
		switch code {
		case 2:
			c = f.downstreamRq2xx
		case 3:
			c = f.downstreamRq3xx
		case 4:
			c = f.downstreamRq4xx
		case 5:
			c = f.downstreamRq5xx
		}
		if c.Load() != 0 {
			t.Errorf("downstream_rq_%dxx = %d, want 0 (status==0 skips Inc)", code, c.Load())
		}
	}
}

// TestH2Dispatcher_Match_IncDownstreamRqTotal verifies that Match Inc's the
// HCM-scope downstream_rq_total counter unconditionally — once per request,
// even on no-match. Per SPEC §12 #1 site (a)'s H2 analog (the once-per-request
// dispatch-entry hook fires before route resolution).
func TestH2Dispatcher_Match_IncDownstreamRqTotal(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{}}
	f := newH2DispatchFilter(t, tt, routerOnlyChain(t), nil)
	disp := newH2Dispatcher(f)

	req, _ := http.NewRequest("GET", "/anywhere", nil)
	disp.Match(req)
	disp.Match(req)
	disp.Match(req)

	if got := f.downstreamRqTotal.Load(); got != 3 {
		t.Errorf("downstream_rq_total = %d, want 3 (one per Match call)", got)
	}
}
