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
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/http2/hpack"

	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/http/router"
	"github.com/pgdad/envoy-go/internal/stats"
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

// TestH2Dispatcher_Match_DirectResponse_WireOutputAndAccessLog covers the
// chain-mediated H2 dispatch path for a matched direct_response route. The
// chainDispatchAction allocates a per-stream FilterChain, drives decode
// iteration through the terminal router, invokes RunAction (which calls the
// directResponseAction's writeH2 via H2Action), and emits a single access-log
// record from the chain-completion hook.
//
// Scope note: this test asserts wire output + access-log emit + bucket-Inc
// only — it does NOT install a recording filter, so it does not verify
// chain-mediation order. Chain-mediation order (decode/encode invocation
// sequencing across multiple filters) is a Task-17 responsibility, exercised
// by chain_integration_test.go.
//
// Asserted:
//   - WriteH2 returns nil (no action error).
//   - The capture writer received HEADERS+DATA with :status=200.
//   - One access-log record was submitted with ResponseCode=200, Protocol=HTTP/2.0,
//     UpstreamHost=empty (direct_response).
//   - downstream_rq_2xx was Inc'd to 1.
func TestH2Dispatcher_Match_DirectResponse_WireOutputAndAccessLog(t *testing.T) {
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
	return func(_ context.Context, _ h2.H2Request) (router.ActionResponse, cluster.Endpoint, error) {
		return router.ActionResponse{Status: 0}, cluster.Endpoint{}, sentinel
	}
}

// faultyAction is a routeAction whose asRouterActionH2 returns a faultyH2Action
// closure. asRouterAction returns a no-op H1 action so the routeAction
// interface is satisfied (chain-mediated dispatch goes through
// asRouterActionH2 + RunAction).
type faultyAction struct {
	sentinel error
}

func (a *faultyAction) asRouterAction() router.Action {
	return func(context.Context, *http.Request) (router.ActionResponse, cluster.Endpoint, error) {
		return router.ActionResponse{Status: 500}, cluster.Endpoint{}, nil
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

// TestH2Dispatcher_Status0Sentinel_SkipsAccessLog verifies the post-RunAction
// status==0 sentinel guard (SPEC §2.1 last bullet): when the action returned
// status==0 (regardless of cause — could be ctx-cancel, could be a
// sentinel-error path; this test exercises a faultyAction returning
// (0, 0, {}, sentinel) under context.Background()), the chain-completion
// access-log emit hook is a no-op and the per-bucket downstream_rq_Nxx
// counters are not Inc'd. Mirrors the pre-Task-16
// TestRouterActionH2_DoH2_CtxCancel_SkipsEmit assertion (renamed for
// honesty: ctx is never canceled here; the guard fires on the status==0
// shape itself).
func TestH2Dispatcher_Status0Sentinel_SkipsAccessLog(t *testing.T) {
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

// TestH2Dispatcher_ActionError_MalformedTrailers_NoHeadersWritten is the hcm-
// layer half of the phase 84.1 Task 7 routed obligation ("downstream
// RST_STREAM(INTERNAL_ERROR) on malformed trailers is REASONED not
// MEASURED"). The actual RST_STREAM wire write happens one layer down, in
// h2.serverStream.dispatch (proved directly against a fakeConn by
// TestServerStream_Dispatch_MalformedTrailersError_EmitsRSTStreamInternalError
// in internal/filter/hcm/h2/stream_test.go — captureH2Writer has no
// WriteRSTStream method, so this layer cannot observe the RST frame itself).
// What THIS layer can and must prove is the other half of the obligation:
// chainDispatchAction.WriteH2 propagates the *h2.Error UNCHANGED (so
// serverStream.dispatch's bare `writeErr.(*Error)` type assertion — the
// thing that selects the RST code — actually fires on it) and, per the
// `rf.ActionRan() && actionErr == nil && status > 0` guard in WriteH2 above
// writeH2Reply's call site, writes NO HEADERS/DATA frame at all — i.e. no
// 502 local reply rides the wire alongside (or instead of) the reset.
//
// sentinel is built the same way router_h2.go's doH2ClusterAction returns it
// unwrapped from its errors.Is(err, h2.ErrMalformedTrailers) arm: an
// *h2.Error whose Underlying is the exported h2.ErrMalformedTrailers
// sentinel, constructed via the h2 package's exported *Error struct fields
// (malformedTrailersError itself is unexported outside the h2 package).
func TestH2Dispatcher_ActionError_MalformedTrailers_NoHeadersWritten(t *testing.T) {
	sentinel := &h2.Error{
		Code:       h2.ErrInternalError,
		Stream:     11,
		Msg:        "trailing HEADERS block without END_STREAM",
		Underlying: h2.ErrMalformedTrailers,
	}
	if !errors.Is(sentinel, h2.ErrMalformedTrailers) {
		t.Fatalf("errors.Is(sentinel, h2.ErrMalformedTrailers) = false, want true (test fixture does not match the production shape)")
	}

	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/trailers"), action: &faultyAction{sentinel: sentinel}},
	}}
	f := newH2DispatchFilter(t, tt, routerOnlyChain(t), nil /* no sinks */)

	disp := newH2Dispatcher(f)
	req, _ := http.NewRequest("GET", "/trailers", nil)
	req.Proto = "HTTP/2.0"
	action, ok := disp.Match(req)
	if !ok {
		t.Fatal("Match returned ok=false; want true on matched route")
	}

	w := &captureH2Writer{}
	h2req := h2.H2Request{Method: "GET", Path: "/trailers"}
	err := action.WriteH2(context.Background(), h2req, w)
	if err == nil {
		t.Fatal("WriteH2 returned nil; want the malformed-trailers *h2.Error to propagate")
	}
	if !errors.Is(err, h2.ErrMalformedTrailers) {
		t.Errorf("errors.Is(err, h2.ErrMalformedTrailers) = false for %v, want true (dispatch must propagate the sentinel unchanged)", err)
	}
	hErr, isH2Err := err.(*h2.Error)
	if !isH2Err {
		t.Fatalf("WriteH2 error type = %T, want *h2.Error (serverStream.dispatch's bare type assertion needs this exact shape to select the RST code)", err)
	}
	if hErr.Code != h2.ErrInternalError {
		t.Errorf("propagated *h2.Error.Code = %v, want INTERNAL_ERROR", hErr.Code)
	}

	if len(w.order) != 0 {
		t.Errorf("frames written to the downstream H2 writer = %v, want none (malformed-trailers rejection must not write a 502 HEADERS frame)", w.order)
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

// ---------------------------------------------------------------------------
// Phase 22.2 Task 6 (ADR-0192) — H2 tlsConnectionState seeding symmetric to H1
// ---------------------------------------------------------------------------

// h2TLSStateChainConfig wires a chainConfig=[capture, router] for the H2
// seeding tests. Symmetric to the H1 mkTLSStateCapturingFilterForTable helper
// in connection_test.go (uses the same *tlsStateCapturingFilter test-double
// + shared-instance factory pattern).
func h2TLSStateChainConfig(t *testing.T) (*tlsStateCapturingFilter, []chainEntry) {
	t.Helper()
	capture := &tlsStateCapturingFilter{}
	captureFactory := func() filter_http.HTTPFilter {
		return filter_http.HTTPFilter{
			Name:    "tls-state-capture",
			Decoder: capture,
			Encoder: capture,
		}
	}
	rfFactory, err := router.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	return capture, []chainEntry{
		{name: "tls-state-capture", factory: captureFactory},
		{name: "envoy.filters.http.router", factory: rfFactory},
	}
}

// TestH2Dispatch_runH2_seeds_tlsConnectionState_symmetric drives the H2
// dispatch path's chainDispatchAction.WriteH2 with the h2Dispatcher's
// tlsConnectionState field pre-seeded (mirroring the runH2 connection-build-time
// extraction). Asserts the chain's tlsConnectionState surfaces through to the
// per-stream filter via decoderCB.DownstreamTLSConnectionState().
//
// Test mechanism: build the in-process TLS handshake-complete state via
// runInProcessTLSHandshake (shared with the H1 connection_test path), pin its
// *tls.ConnectionState onto a fresh h2Dispatcher.tlsConnectionState, run
// Match → WriteH2 once, then assert capture.captured == the same
// *tls.ConnectionState. Symmetric to the H1 seeding test.
//
// The runH2 helper itself is exercised separately (filter_test.go covers the
// connection-build-time extraction at the runH2 site); this test asserts the
// chainDispatchAction.WriteH2 SEAM seeds the chain field correctly given a
// pre-populated dispatcher.
func TestH2Dispatch_runH2_seeds_tlsConnectionState_symmetric(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	capture, chainCfg := h2TLSStateChainConfig(t)
	f := newH2DispatchFilter(t, tt, chainCfg, nil /* no sinks */)

	serverTLS, cleanup := runInProcessTLSHandshake(t, "h2-server-test", "h2.sni.envoy-go.test")
	defer cleanup()
	// Pre-extract the state once (mirrors runH2's connection-build-time
	// extraction). The pointer identity is what we'll assert downstream.
	state := downstreamTLSConnectionState(serverTLS)
	if state == nil {
		t.Fatal("downstreamTLSConnectionState returned nil on post-handshake conn; helper invariant violated")
	}

	disp := newH2Dispatcher(f)
	disp.tlsConnectionState = state // simulates the runH2 connection-build seed

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/2.0"
	action, ok := disp.Match(req)
	if !ok {
		t.Fatal("Match returned ok=false; want true on matched route")
	}

	w := &captureH2Writer{}
	h2req := h2.H2Request{Method: "GET", Path: "/health", Authority: "h2.sni.envoy-go.test"}
	if err := action.WriteH2(context.Background(), h2req, w); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	if capture.captured == nil {
		t.Fatal("H2 path: captured = nil; want non-nil seeded *tls.ConnectionState")
	}
	if capture.captured != state {
		t.Errorf("H2 path: captured pointer = %p; want %p (verbatim threading from dispatcher → chainDispatchAction → chain.SetTLSConnectionState)", capture.captured, state)
	}
	if capture.captured.ServerName != "h2.sni.envoy-go.test" {
		t.Errorf("H2 path: captured.ServerName = %q; want %q", capture.captured.ServerName, "h2.sni.envoy-go.test")
	}
}

// TestH2Dispatch_runH2_seeds_nil_for_plaintext_symmetric exercises the H2 plaintext
// path: an h2Dispatcher with tlsConnectionState=nil (mirroring runH2 when the
// downstream conn is not a *tls.Conn). The chain's tlsConnectionState stays
// nil — symmetric to the H1 plaintext path in connection_test.go.
func TestH2Dispatch_runH2_seeds_nil_for_plaintext_symmetric(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	capture, chainCfg := h2TLSStateChainConfig(t)
	f := newH2DispatchFilter(t, tt, chainCfg, nil)

	disp := newH2Dispatcher(f)
	// disp.tlsConnectionState stays nil — plaintext.

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/2.0"
	action, ok := disp.Match(req)
	if !ok {
		t.Fatal("Match: ok=false")
	}

	w := &captureH2Writer{}
	h2req := h2.H2Request{Method: "GET", Path: "/health"}
	if err := action.WriteH2(context.Background(), h2req, w); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	if capture.captured != nil {
		t.Errorf("plaintext H2: captured = %#v; want nil", capture.captured)
	}
}

// TestWriteH2Reply_FrameSequence pins the FRAME SEQUENCE writeH2Reply emits as
// an ordered (frame-kind, end_stream) tuple, over the full
// {trailers, no trailers} x {body, no body} matrix. Phase 84.1 Task 6 landed
// the two body-bearing cells; Task 7 adds the two bodyless cells (this table
// is the extension point — add rows, not a second test).
//
// D-84-ENDSTREAM: END_STREAM moves off the last body-bearing frame ONLY when a
// trailer block is present. The no-trailers rows are the byte-identical-to-
// today control: frame kinds, flags and ordering must be unchanged by this row.
//
// ⚠️ LOAD-BEARING: the "no_body_with_trailers" cell is the ONLY cell in this
// table that discriminates the `&& !hasTrailers` conjunct in
// `endStream := len(body) == 0 && !hasTrailers` (h2dispatch.go writeH2Reply).
// Every with-body cell has len(body)==0 evaluate to false regardless of
// hasTrailers, so a break that drops the conjunct is invisible to them.
//
// Dual indexing convention (captureH2Writer): w.headers is indexed by
// HEADERS-FRAME ORDINAL (0 = response block, 1 = trailing block when
// present), while w.order / w.endStream index ALL frames written (HEADERS
// and DATA interleaved) in wire order — the two index spaces are NOT the
// same length when a DATA frame is emitted.
func TestWriteH2Reply_FrameSequence(t *testing.T) {
	trailerBlock := []hpack.HeaderField{
		{Name: "grpc-status", Value: "0"},
		{Name: "grpc-message", Value: "ok"},
	}

	cases := []struct {
		name          string
		body          []byte
		trailers      []hpack.HeaderField
		wantOrder     []string
		wantEndStream []bool
	}{
		{
			// Control: today's behavior, must stay byte-identical.
			name:          "body_no_trailers",
			body:          []byte("hello"),
			trailers:      nil,
			wantOrder:     []string{"headers", "data"},
			wantEndStream: []bool{false, true},
		},
		{
			name:          "body_with_trailers",
			body:          []byte("hello"),
			trailers:      trailerBlock,
			wantOrder:     []string{"headers", "data", "headers"},
			wantEndStream: []bool{false, false, true},
		},
		{
			// Control: bodyless, no trailers — a single HEADERS frame carries
			// END_STREAM itself (no DATA, no trailing block).
			name:          "no_body_no_trailers",
			body:          nil,
			trailers:      nil,
			wantOrder:     []string{"headers"},
			wantEndStream: []bool{true},
		},
		{
			// ⚠️ LOAD-BEARING (see func doc): the only cell where
			// len(body)==0 is true, so this is the only cell that can
			// discriminate the `&& !hasTrailers` conjunct. No DATA frame is
			// emitted (body is empty); END_STREAM rides the trailing HEADERS
			// block, and the response HEADERS block carries end_stream=false.
			name:          "no_body_with_trailers",
			body:          nil,
			trailers:      trailerBlock,
			wantOrder:     []string{"headers", "headers"},
			wantEndStream: []bool{false, true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &captureH2Writer{}
			hdrs := filter_http.OrderedHeaders{
				{Name: "content-type", Value: "application/grpc"},
			}
			if err := writeH2Reply(w, 200, hdrs, tc.body, tc.trailers); err != nil {
				t.Fatalf("writeH2Reply: %v", err)
			}

			if got := strings.Join(w.order, ","); got != strings.Join(tc.wantOrder, ",") {
				t.Errorf("frame order = [%s]; want [%s]", got, strings.Join(tc.wantOrder, ","))
			}
			if len(w.endStream) != len(tc.wantEndStream) {
				t.Fatalf("end_stream flag count = %d (%v); want %d (%v)", len(w.endStream), w.endStream, len(tc.wantEndStream), tc.wantEndStream)
			}
			for i, want := range tc.wantEndStream {
				if w.endStream[i] != want {
					t.Errorf("frame %d (%s): end_stream = %v; want %v (full seq %v)", i, w.order[i], w.endStream[i], want, w.endStream)
				}
			}

			// The response HEADERS block must never carry the trailer fields,
			// and the trailing block must carry exactly them (no :status, no
			// date/server defaults) when present.
			for _, h := range w.headers[0] {
				if h.Name == "grpc-status" || h.Name == "grpc-message" {
					t.Errorf("response HEADERS block carries trailer field %q; trailers must ride the trailing block only", h.Name)
				}
			}
			if len(tc.trailers) > 0 {
				if len(w.headers) != 2 {
					t.Fatalf("HEADERS frame count = %d; want 2 (response block + trailing block)", len(w.headers))
				}
				got := w.headers[1]
				if len(got) != len(tc.trailers) {
					t.Fatalf("trailing block len = %d (%v); want %d (%v)", len(got), got, len(tc.trailers), tc.trailers)
				}
				for i := range got {
					if got[i] != tc.trailers[i] {
						t.Errorf("trailing block[%d] = %+v; want %+v", i, got[i], tc.trailers[i])
					}
				}
			} else if len(w.headers) != 1 {
				t.Errorf("HEADERS frame count = %d; want 1 (no trailers ⇒ no trailing block)", len(w.headers))
			}
		})
	}
}

// encodeSignalFilter is an encode-side spy that records the (callback,
// end_stream) tuple of every encode-chain event it observes, in invocation
// order, as "EncodeHeaders(<bool>)" / "EncodeData(<bool>)" strings. It is the
// chain-side counterpart of captureH2Writer: captureH2Writer observes what the
// WIRE is told, this observes what the encode FILTERS are told.
//
// Phase 84.1 Task 8. Deliberately records the boolean rather than a count —
// the defect this fixture exists to catch (an encode filter observing
// end_stream=true while a trailing HEADERS block is still to come) is
// invisible to a call COUNT: the counts are identical with and without the
// fix (see reference: a COUNTER cannot gate a VALUE).
type encodeSignalFilter struct {
	mu     *sync.Mutex
	events *[]string
}

func (f *encodeSignalFilter) record(s string) {
	f.mu.Lock()
	*f.events = append(*f.events, s)
	f.mu.Unlock()
}

func (f *encodeSignalFilter) DecodeHeaders(http.Header, bool) filter_http.FilterHeadersStatus {
	return filter_http.Continue
}
func (f *encodeSignalFilter) DecodeData([]byte, bool) filter_http.FilterDataStatus {
	return filter_http.DataContinue
}
func (f *encodeSignalFilter) DecodeTrailers(http.Header) filter_http.FilterTrailersStatus {
	return filter_http.TrailersContinue
}
func (f *encodeSignalFilter) SetDecoderCallbacks(filter_http.DecoderFilterCallbacks) {}

func (f *encodeSignalFilter) EncodeHeaders(_ http.Header, endStream bool) filter_http.FilterHeadersStatus {
	f.record(fmt.Sprintf("EncodeHeaders(%v)", endStream))
	return filter_http.Continue
}
func (f *encodeSignalFilter) EncodeData(_ []byte, endStream bool) filter_http.FilterDataStatus {
	f.record(fmt.Sprintf("EncodeData(%v)", endStream))
	return filter_http.DataContinue
}
func (f *encodeSignalFilter) EncodeTrailers(http.Header) filter_http.FilterTrailersStatus {
	f.record("EncodeTrailers")
	return filter_http.TrailersContinue
}
func (f *encodeSignalFilter) SetEncoderCallbacks(filter_http.EncoderFilterCallbacks) {}
func (f *encodeSignalFilter) OnDestroy()                                             {}

// encodeSignalChain builds a two-entry chainConfig: the encode-signal spy
// ahead of the terminal router. The events slice + mutex are captured by
// closure so the per-request fresh instance writes into the test's slice.
func encodeSignalChain(t *testing.T, events *[]string, mu *sync.Mutex) []chainEntry {
	t.Helper()
	rfFactory, err := router.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	spy := func() filter_http.HTTPFilter {
		f := &encodeSignalFilter{mu: mu, events: events}
		return filter_http.HTTPFilter{Name: "encode_signal", Decoder: f, Encoder: f}
	}
	return []chainEntry{
		{name: "encode_signal", factory: spy},
		{name: "envoy.filters.http.router", factory: rfFactory},
	}
}

// trailerStubAction is a routeAction whose H2Action returns a fixed
// ActionResponse with a caller-chosen body and trailing block. It exists so a
// dispatch-level test can drive the encode chain over all four cells of the
// {trailers, no trailers} x {body, no body} matrix WITHOUT standing up a real
// upstream (the production populator is doH2ClusterAction, phase 84.1 Task 5).
//
// The H1 arm (asRouterAction) returns a bodyless 200 with NO trailers: the
// Trailers carrier is H2-only (router.ActionResponse doc) and this action is
// never routed on the H1 path by any test here.
type trailerStubAction struct {
	body     []byte
	trailers []hpack.HeaderField
}

func (a *trailerStubAction) asRouterAction() router.Action {
	return func(context.Context, *http.Request) (router.ActionResponse, cluster.Endpoint, error) {
		return router.ActionResponse{Status: 200}, cluster.Endpoint{}, nil
	}
}

func (a *trailerStubAction) asRouterActionH2() router.H2Action {
	return func(context.Context, h2.H2Request) (router.ActionResponse, cluster.Endpoint, error) {
		return router.ActionResponse{
			Status:   200,
			Headers:  filter_http.OrderedHeaders{{Name: "content-type", Value: "application/grpc"}},
			Body:     a.body,
			Trailers: a.trailers,
		}, cluster.Endpoint{}, nil
	}
}

// TestH2Dispatch_EncodeChain_EndStreamSignal pins what the ENCODE CHAIN is
// told about end-of-stream, across the same four cells
// TestWriteH2Reply_FrameSequence pins for the wire. Phase 84.1 Task 8,
// disposition (ii) (fix) per the PLAN's recommendation.
//
// The contract: no encode filter may observe end_stream=true while more
// response data (the trailing HEADERS block) is still to come. Because
// ADR-0273's boot-reject keeps RunEncodeTrailers deliberately unwired, the
// honest signal in the two trailers cells is that the chain's LAST observed
// event carries end_stream=false — the chain is simply never told the stream
// ended, rather than being told it ended early. Asserting a false-tail is the
// point, not an oversight.
//
// ⚠️ LOAD-BEARING: the two trailers rows are the RED anchors. The two
// no-trailers rows are the controls — they must stay byte-identical to the
// pre-84.1 signal (EncodeHeaders(len(body)==0), EncodeData(true)), so a fix
// that unconditionally passes `false` reddens them.
//
// A call-COUNT assertion cannot decide any of this: all four cells emit the
// same number of encode events before and after the fix. The assertion is on
// the ordered (callback, end_stream) SEQUENCE.
func TestH2Dispatch_EncodeChain_EndStreamSignal(t *testing.T) {
	trailerBlock := []hpack.HeaderField{
		{Name: "grpc-status", Value: "0"},
		{Name: "grpc-message", Value: "ok"},
	}

	cases := []struct {
		name     string
		body     []byte
		trailers []hpack.HeaderField
		want     []string
	}{
		{
			// Control: today's signal, must be unchanged by this row.
			name:     "body_no_trailers",
			body:     []byte("hello"),
			trailers: nil,
			want:     []string{"EncodeHeaders(false)", "EncodeData(true)"},
		},
		{
			// RED anchor: pre-fix this reads EncodeData(true) while the
			// trailing HEADERS block is still to come.
			name:     "body_with_trailers",
			body:     []byte("hello"),
			trailers: trailerBlock,
			want:     []string{"EncodeHeaders(false)", "EncodeData(false)"},
		},
		{
			// Control: bodyless, no trailers — HEADERS is the whole response,
			// so end_stream=true on it is honest. No EncodeData (the dispatch
			// site skips RunEncodeData on an empty body).
			name:     "no_body_no_trailers",
			body:     nil,
			trailers: nil,
			want:     []string{"EncodeHeaders(true)"},
		},
		{
			// RED anchor: pre-fix this reads EncodeHeaders(true) with a whole
			// trailing block still to come — the filter-visible contract break
			// the PLAN cites as the reason to pick disposition (ii).
			name:     "no_body_with_trailers",
			body:     nil,
			trailers: trailerBlock,
			want:     []string{"EncodeHeaders(false)"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			var mu sync.Mutex

			tt := &routeTable{routes: []routeEntry{
				{match: matchPath("/grpc"), action: &trailerStubAction{body: tc.body, trailers: tc.trailers}},
			}}
			f := newH2DispatchFilter(t, tt, encodeSignalChain(t, &events, &mu), nil /* no sinks */)

			disp := newH2Dispatcher(f)
			req, _ := http.NewRequest("POST", "/grpc", nil)
			req.Proto = "HTTP/2.0"
			action, ok := disp.Match(req)
			if !ok {
				t.Fatal("Match returned ok=false; want true on matched route")
			}

			w := &captureH2Writer{}
			h2req := h2.H2Request{Method: "POST", Path: "/grpc", Authority: "localhost"}
			if err := action.WriteH2(context.Background(), h2req, w); err != nil {
				t.Fatalf("WriteH2: %v", err)
			}

			mu.Lock()
			got := append([]string(nil), events...)
			mu.Unlock()

			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("encode-chain signal = [%s]; want [%s]", strings.Join(got, ","), strings.Join(tc.want, ","))
			}

			// Cross-check against the wire: whatever the chain is told, the
			// wire sequence Task 6/7 landed must be unchanged by Task 8. This
			// is the guard that a Task-8 fix did not reach into writeH2Reply.
			wantWireEnd := []bool{len(tc.trailers) == 0}
			if len(tc.body) > 0 {
				wantWireEnd = []bool{false, len(tc.trailers) == 0}
			}
			if len(tc.trailers) > 0 {
				wantWireEnd = append(wantWireEnd, true)
			}
			if len(w.endStream) != len(wantWireEnd) {
				t.Fatalf("wire end_stream flags = %v; want %v", w.endStream, wantWireEnd)
			}
			for i, want := range wantWireEnd {
				if w.endStream[i] != want {
					t.Errorf("wire frame %d (%s): end_stream = %v; want %v (full seq %v)", i, w.order[i], w.endStream[i], want, w.endStream)
				}
			}
		})
	}
}
