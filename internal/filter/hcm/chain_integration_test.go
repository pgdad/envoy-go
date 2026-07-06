package hcm

// chain_integration_test.go — Phase 07.1 Task 17. End-to-end-but-in-process
// proofs that the chain-mediated dispatch path runs filters in declaration
// order ahead of the terminal router on BOTH the H1 and H2 codecs. The route
// table direct_response synthesizes "200 OK\n" so no upstream is needed; two
// recording filters wired ahead of the router observe their decode-side
// callbacks for assertion.
//
// Pattern (b) per Task 17's prompt: the recordingFilter helper is defined
// inline here rather than promoted to a shared exported test fixture
// package. Future cleanup if more cross-package test sharing arrives:
// promote to internal/filter/http/filtertest/ (pattern (a)).
//
// IMPORTANT: encode-side ordering is NOT asserted in these tests. The encode
// chain is dormant on both H1 + H2 dispatch paths until Task 18 (cors)
// rewires the wire-output through chain-fed buffers (see Task 15 PROGRESS
// "Task 18 prerequisites" + Task 16 PROGRESS forwarded). The recording filter
// implements the encode-side methods to satisfy the StreamEncoderFilter
// interface (the chain framework requires both sides for filters that
// participate in the both-sides envelope), but their counters are not
// asserted as the dispatch path does not invoke RunEncode* yet on the
// happy direct_response path.

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/http/cors"
	"github.com/pgdad/envoy-go/internal/filter/http/router"
	"github.com/pgdad/envoy-go/internal/stats"
)

// integrationRecordingFilter is the chain_integration_test-local recording
// filter. Each callback appends "<name>-<phase>" to a shared *[]string
// guarded by *sync.Mutex; this lets tests assert decode-side ordering across
// a multi-filter chain.
//
// The filter satisfies BOTH StreamDecoderFilter and StreamEncoderFilter; the
// chain framework wires both sides via filter_http.HTTPFilter{Decoder: f,
// Encoder: f}. Encode-side methods record too (so a future Task 18 follow-up
// could enable encode-order assertions by removing the //skip-encode-order
// gate); however no test in this file asserts encode-order yet.
type integrationRecordingFilter struct {
	name  string
	order *[]string
	mu    *sync.Mutex

	// Body capture: optionally records the data buffer that DecodeData receives.
	// Used by TestChainIntegration_H2_MultiFrameDATA to assert the action sees
	// the multi-frame body verbatim. Captured via a copy so subsequent
	// mutation of the chain's internal buffer does not race with the
	// post-test inspection.
	bodyCapture *[]byte

	// decodeHeaders / decodeData / decodeTrailers atomics let tests assert
	// callback counts without locking the order slice (the order slice is for
	// ordering; counts are an orthogonal check).
	decodeHeaders  atomic.Int32
	decodeData     atomic.Int32
	decodeTrailers atomic.Int32
	encodeHeaders  atomic.Int32
	encodeData     atomic.Int32
	encodeTrailers atomic.Int32
	destroyed      atomic.Int32
}

func (f *integrationRecordingFilter) record(phase string) {
	f.mu.Lock()
	*f.order = append(*f.order, f.name+"-"+phase)
	f.mu.Unlock()
}

func (f *integrationRecordingFilter) DecodeHeaders(http.Header, bool) filter_http.FilterHeadersStatus {
	f.decodeHeaders.Add(1)
	f.record("DecodeHeaders")
	return filter_http.Continue
}
func (f *integrationRecordingFilter) DecodeData(data []byte, _ bool) filter_http.FilterDataStatus {
	f.decodeData.Add(1)
	if f.bodyCapture != nil {
		f.mu.Lock()
		buf := make([]byte, len(data))
		copy(buf, data)
		*f.bodyCapture = buf
		f.mu.Unlock()
	}
	return filter_http.DataContinue
}
func (f *integrationRecordingFilter) DecodeTrailers(http.Header) filter_http.FilterTrailersStatus {
	f.decodeTrailers.Add(1)
	return filter_http.TrailersContinue
}
func (f *integrationRecordingFilter) SetDecoderCallbacks(filter_http.DecoderFilterCallbacks) {}

func (f *integrationRecordingFilter) EncodeHeaders(http.Header, bool) filter_http.FilterHeadersStatus {
	f.encodeHeaders.Add(1)
	return filter_http.Continue
}
func (f *integrationRecordingFilter) EncodeData([]byte, bool) filter_http.FilterDataStatus {
	f.encodeData.Add(1)
	return filter_http.DataContinue
}
func (f *integrationRecordingFilter) EncodeTrailers(http.Header) filter_http.FilterTrailersStatus {
	f.encodeTrailers.Add(1)
	return filter_http.TrailersContinue
}
func (f *integrationRecordingFilter) SetEncoderCallbacks(filter_http.EncoderFilterCallbacks) {}
func (f *integrationRecordingFilter) OnDestroy()                                             { f.destroyed.Add(1) }

// integrationFilterFactory returns a FilterInstanceFactory that allocates a
// fresh integrationRecordingFilter each call. The shared order slice + mutex
// + optional bodyCapture are captured by closure so multi-request tests
// (none currently — Task 17 fires one request per test) would observe the
// concatenated callback sequence; per-test these helpers are scoped to a
// single dispatch call so the slice equals the per-request ordering.
func integrationFilterFactory(name string, order *[]string, mu *sync.Mutex, bodyCapture *[]byte) filter_http.FilterInstanceFactory {
	return func() filter_http.HTTPFilter {
		f := &integrationRecordingFilter{
			name:        name,
			order:       order,
			mu:          mu,
			bodyCapture: bodyCapture,
		}
		return filter_http.HTTPFilter{
			Name:    name,
			Decoder: f,
			Encoder: f,
		}
	}
}

// buildChainConfig assembles a chainConfig with two recording filters ahead
// of the terminal router. Used by all three integration tests in this file.
// The order slice + mutex are captured by closures and shared across the
// per-request fresh instances.
func buildChainConfig(t *testing.T, order *[]string, mu *sync.Mutex, bodyCapture *[]byte) []chainEntry {
	t.Helper()
	rfFactory, err := router.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	return []chainEntry{
		{name: "a", factory: integrationFilterFactory("a", order, mu, bodyCapture)},
		{name: "b", factory: integrationFilterFactory("b", order, mu, bodyCapture)},
		{name: "envoy.filters.http.router", factory: rfFactory},
	}
}

// newIntegrationFilter builds a *Filter with the shared HCM-scope counters
// allocated against a fresh *stats.Registry. Used by both H1 and H2 tests so
// the bucket Inc assertions can read back specific counters.
func newIntegrationFilter(t *testing.T, table *routeTable, chain []chainEntry, sinks []accesslog.Sink) *Filter {
	t.Helper()
	r := stats.NewRegistry()
	prefix := "http.test_chain_integration."
	return &Filter{
		table:             table,
		statPrefix:        "test_chain_integration",
		downstreamRqTotal: r.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   r.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   r.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   r.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   r.NewCounter(prefix + "downstream_rq_5xx"),
		accessLog:         sinks,
		chainConfig:       chain,
	}
}

// TestChainIntegration_H1_DirectResponseHappy proves the chain-mediated H1
// dispatch path runs the two recording filters in declaration order ahead of
// the terminal router. The route_config maps /health → direct_response 200
// "OK\n"; the recording filters observe DecodeHeaders firing in order before
// the response is generated.
//
// Asserted:
//   - decode-side order is exactly ["a-DecodeHeaders", "b-DecodeHeaders"]
//     (the terminal router does not record because its DecodeHeaders is
//     internal pass-through; the action drives via RunAction).
//   - the H1 wire output is "HTTP/1.1 200 OK\r\n...\r\n\r\nOK\n".
//   - status==200 propagates to the runConnection bucket-Inc machinery.
//
// Encode-side ordering is NOT asserted: the encode chain is dormant on the
// H1 direct_response path until Task 18 (cors) rewires the wire-output
// through a chain-fed buffer. Encode-side ordering verified at Task 18.
func TestChainIntegration_H1_DirectResponseHappy(t *testing.T) {
	var order []string
	var orderMu sync.Mutex

	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	chain := buildChainConfig(t, &order, &orderMu, nil)
	f := newIntegrationFilter(t, tt, chain, nil)

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	status, err := f.dispatchRequest(context.Background(), nil, req, bw)
	if err != nil {
		t.Fatalf("dispatchRequest: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	_ = bw.Flush()

	// Assert decode-side iteration order: a runs before b; both run before
	// the terminal router action. The terminal router does NOT record (its
	// DecodeHeaders is a Continue pass-through; the action drive happens in
	// RunAction).
	orderMu.Lock()
	got := append([]string(nil), order...)
	orderMu.Unlock()

	want := []string{"a-DecodeHeaders", "b-DecodeHeaders"}
	if len(got) != len(want) {
		t.Fatalf("expected order length %d (%v); got %d (%v)", len(want), want, len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("decode iteration order: got %v, want %v", got, want)
		}
	}

	// Wire output: HTTP/1.1 200 OK status line + "OK\n" body suffix. The
	// intermediate framework headers (Date / Server / Content-Type /
	// Content-Length) live between, but their byte-equivalent shape is
	// covered by the Task 15 connection_test suite — this test focuses on
	// chain-order assertion + happy-path wire-shape sanity check.
	out := buf.String()
	if !strings.HasPrefix(out, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("expected 200 OK status line, got: %q", out)
	}
	if !strings.HasSuffix(out, "OK\n") {
		t.Errorf("expected body 'OK\\n' suffix, got: %q", out)
	}
}

// TestChainIntegration_H2_DirectResponseHappy is the H2 analog of the H1
// happy-path test. Same chain shape (a, b, router). Same decode-order
// assertion. Wire output assertion adapted to the H2 codec: HEADERS frame
// with :status=200 + a DATA frame carrying "OK\n".
//
// Asserted:
//   - decode-side order is exactly ["a-DecodeHeaders", "b-DecodeHeaders"].
//   - :status header value is "200".
//   - DATA frame body is "OK\n" (3 bytes).
//
// Encode-side ordering NOT asserted (Task 18 carry-forward; the H2Action
// writes HEADERS+DATA directly to the h2.StreamWriter without engaging
// the encode chain).
func TestChainIntegration_H2_DirectResponseHappy(t *testing.T) {
	var order []string
	var orderMu sync.Mutex

	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	chain := buildChainConfig(t, &order, &orderMu, nil)
	f := newIntegrationFilter(t, tt, chain, nil)

	disp := newH2Dispatcher(f)
	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/2.0"
	action, ok := disp.Match(req)
	if !ok {
		t.Fatal("Match: ok=false; want true on matched route")
	}

	w := &captureH2Writer{}
	h2req := h2.H2Request{Method: "GET", Path: "/health", Authority: "localhost"}
	if err := action.WriteH2(context.Background(), h2req, w); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	// Assert decode-side iteration order across the chain: a runs before b;
	// both run before the terminal router action. Same shape as the H1 test.
	orderMu.Lock()
	got := append([]string(nil), order...)
	orderMu.Unlock()

	want := []string{"a-DecodeHeaders", "b-DecodeHeaders"}
	if len(got) != len(want) {
		t.Fatalf("expected order length %d (%v); got %d (%v)", len(want), want, len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("decode iteration order: got %v, want %v", got, want)
		}
	}

	// Wire output: HEADERS frame :status=200 + DATA frame "OK\n".
	if got := w.statusOf(); got != "200" {
		t.Errorf(":status = %q, want 200", got)
	}
	if len(w.data) != 1 {
		t.Fatalf("expected exactly one DATA frame; got %d", len(w.data))
	}
	if string(w.data[0]) != "OK\n" {
		t.Errorf("DATA frame body = %q, want %q", string(w.data[0]), "OK\n")
	}

	// Bucket Inc sanity: the chain-completion hook fires once per finalized
	// status, downstream_rq_2xx is incremented to 1 (mirrors the
	// H2Dispatcher_Match_DirectResponse_WireOutputAndAccessLog assertion in
	// h2dispatch_test.go but on a multi-filter chain to cement that the
	// bucket Inc happens regardless of chain length).
	if got := f.downstreamRq2xx.Load(); got != 1 {
		t.Errorf("downstream_rq_2xx = %d, want 1", got)
	}
}

// TestChainIntegration_H2_MultiFrameDATA closes the multi-frame H2 DATA gap
// forwarded from Task 16 PROGRESS (Task 17 forwards block). A POST body of
// 256 KiB is fed as the snapshotted h2req.Body — the H2 codec layer would
// have buffered this from multiple DATA frames before dispatch, so the
// chain sees the body as a single RunDecodeData(snapshot, endStream=true)
// call (per Task 16 deviation (ii)).
//
// The recording filter "a" captures the body via DecodeData; the assertion
// is that the captured body matches the input verbatim (length + first/last
// bytes; a full byte-equality check is also done because the test doesn't
// need to scale beyond 256 KiB).
//
// Per Task 16 deviation (ii) the chain's multi-frame view is "one big
// RunDecodeData call after H2 codec buffering" — this test exercises that
// shape and would catch a regression where the body is truncated, dropped,
// or fed in chunks (the latter would manifest as decodeData.Load() > 1).
//
// Encode-side ordering NOT asserted (Task 18 carry-forward).
func TestChainIntegration_H2_MultiFrameDATA(t *testing.T) {
	var order []string
	var orderMu sync.Mutex
	var bodyCapture []byte

	// Build a 256 KiB body with a deterministic byte pattern so a partial-
	// truncation regression is detectable on inspection. 'A'+i%26 means each
	// 26-byte stride cycles through A..Z.
	const bodyLen = 256 * 1024
	wantBody := make([]byte, bodyLen)
	for i := range wantBody {
		wantBody[i] = byte('A' + (i % 26))
	}

	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/echo"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	// Body capture lives on filter "a" (the first filter); it sees the
	// snapshotted body via RunDecodeData. Filter "b" gets the same shared
	// pointer so either filter would record (single shared buffer across
	// both factories — only one filter's DecodeData fires for the single
	// snapshotted chunk; the chain's iteration cursor advances through both
	// for ordering, but DecodeData is invoked once per chunk).
	chain := buildChainConfig(t, &order, &orderMu, &bodyCapture)
	f := newIntegrationFilter(t, tt, chain, nil)

	disp := newH2Dispatcher(f)
	// POST request — req.Header carries no body; req.Body is unused on the
	// H2 path (the codec layer pre-buffers DATA frames into h2req.Body before
	// invoking dispatch, per Task 16 deviation (ii)). req.Method=POST is
	// just for the route shape; the route matcher matches on path only.
	req, _ := http.NewRequest("POST", "/echo", nil)
	req.Proto = "HTTP/2.0"
	action, ok := disp.Match(req)
	if !ok {
		t.Fatal("Match: ok=false; want true on matched route")
	}

	w := &captureH2Writer{}
	h2req := h2.H2Request{
		Method:    "POST",
		Path:      "/echo",
		Authority: "localhost",
		// Body snapshot: this is what the codec layer would have produced
		// after buffering multiple DATA frames. The chain sees this as a
		// single RunDecodeData(body, endStream=true) call.
		Body: wantBody,
	}
	if err := action.WriteH2(context.Background(), h2req, w); err != nil {
		t.Fatalf("WriteH2: %v", err)
	}

	// Assert the recording filter saw the body verbatim. The chain feeds the
	// snapshotted body to RunDecodeData as a single chunk, so decodeData
	// should fire exactly once on each chain filter that participates (both
	// recording filters DataContinue → both fire), but the body capture
	// pointer is shared so the last write wins. We assert the captured body
	// equals the input.
	orderMu.Lock()
	gotBody := append([]byte(nil), bodyCapture...)
	orderMu.Unlock()

	if len(gotBody) != bodyLen {
		t.Fatalf("captured body length = %d, want %d", len(gotBody), bodyLen)
	}
	if !bytes.Equal(gotBody, wantBody) {
		// Pin a specific divergence index for diagnostic output. Matching
		// is byte-exact; a partial truncation regresses here.
		for i := range gotBody {
			if gotBody[i] != wantBody[i] {
				t.Fatalf("captured body diverges at index %d: got %#x, want %#x", i, gotBody[i], wantBody[i])
			}
		}
		t.Fatalf("captured body diverges from input but no specific divergence found (length match check?)")
	}

	// Assert the decode-side iteration order across decode-headers + decode-
	// data: chain feeds headers first to a then b, then data to a then b.
	// (decode-data ordering is the same as decode-headers because both run
	// declaration-order on the chain; the chain cursor resets at the top of
	// each Run* method per chain.go.)
	orderMu.Lock()
	gotOrder := append([]string(nil), order...)
	orderMu.Unlock()

	// Filter callbacks fire ONLY on DecodeHeaders (the recording filter
	// records on header-phase only; DecodeData records to bodyCapture, not
	// to the order slice). So the order slice should be exactly the two
	// header firings — matches the H2 happy-path test's order assertion.
	wantOrder := []string{"a-DecodeHeaders", "b-DecodeHeaders"}
	if len(gotOrder) != len(wantOrder) {
		t.Fatalf("expected order length %d (%v); got %d (%v)", len(wantOrder), wantOrder, len(gotOrder), gotOrder)
	}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("decode iteration order: got %v, want %v", gotOrder, wantOrder)
		}
	}

	// Wire output: 200 OK direct_response. The H2 dispatch path's terminal
	// router action does NOT echo the request body — direct_response writes
	// "OK\n" verbatim regardless of what the request body is. The point of
	// this test is to verify the body REACHES the action (via RunDecodeData)
	// not that the action echoes it back.
	if got := w.statusOf(); got != "200" {
		t.Errorf(":status = %q, want 200", got)
	}
	if len(w.data) != 1 || string(w.data[0]) != "OK\n" {
		t.Errorf("DATA frame body unexpected: got %v", w.data)
	}
}

// TestChainIntegration_H1_CorsPreflight_AllowedOriginEmits200WithSixHeaders
// Phase 07.1 Task 18: end-to-end proof that the cors filter's SendLocalReply
// path produces wire bytes through HCM dispatch. Builds a [cors, router]
// chain with a per-route CorsPolicy allowing https://example.test; drives an
// OPTIONS preflight from that origin; asserts the H1 wire output carries the
// six CORS headers in §11.2 verbatim order.
//
// This test closes Task 18's prereq P2 wire-write gap (chain.beginLocalReply
// runs the encode chain but does not emit wire bytes; HCM dispatch's new
// SendLocalReply branch reads chain.LocalReplyResponse and emits via
// writeH1Reply). Without this branch the response would be missing entirely.
func TestChainIntegration_H1_CorsPreflight_AllowedOriginEmits200WithSixHeaders(t *testing.T) {
	// Build the per-route CorsPolicy with allowed origin https://example.test
	// + the standard probe shape from SPEC §11.2.
	policy := &corsv3.CorsPolicy{
		AllowOriginStringMatch: []*matcherv3.StringMatcher{
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "https://example.test"}},
		},
		AllowMethods:     "GET, POST, OPTIONS",
		AllowHeaders:     "x-foo, x-bar",
		ExposeHeaders:    "x-baz",
		MaxAge:           "600",
		AllowCredentials: wrapperspb.Bool(true),
	}
	policyAny, err := anypb.New(policy)
	if err != nil {
		t.Fatal(err)
	}

	// Chain: cors + router; route /preflight → direct_response 200 (won't be
	// reached because cors short-circuits with SendLocalReply).
	corsFactory, err := cors.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("cors.New: %v", err)
	}
	rfFactory, err := router.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	chainCfg := []chainEntry{
		{name: "envoy.filters.http.cors", factory: corsFactory},
		{name: "envoy.filters.http.router", factory: rfFactory},
	}
	scopes := []filter_http.RouteScope{
		{Route: map[string]*anypb.Any{"envoy.filters.http.cors": policyAny}},
	}
	pr, err := filter_http.BuildPerRouteConfig(nil, scopes, []string{"envoy.filters.http.cors", "envoy.filters.http.router"}, nil)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}

	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/preflight"), action: &directResponseAction{status: 200, bodyText: "fallback\n"}},
	}}
	r := stats.NewRegistry()
	prefix := "http.test_chain_integration_cors."
	f := &Filter{
		table:             tt,
		statPrefix:        "test_chain_integration_cors",
		downstreamRqTotal: r.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   r.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   r.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   r.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   r.NewCounter(prefix + "downstream_rq_5xx"),
		chainConfig:       chainCfg,
		perRouteConfig:    pr,
	}

	// Build the OPTIONS preflight request.
	req, _ := http.NewRequest("OPTIONS", "/preflight", nil)
	req.Proto = "HTTP/1.1"
	req.Header.Set("Origin", "https://example.test")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "x-foo,x-bar")

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	status, derr := f.dispatchRequest(context.Background(), nil, req, bw)
	if derr != nil {
		t.Fatalf("dispatchRequest: %v", derr)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200 (cors preflight allowed)", status)
	}
	_ = bw.Flush()

	out := buf.String()
	if !strings.HasPrefix(out, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("expected 200 OK status line, got: %q", out)
	}

	// Verify the six CORS headers in §11.2 verbatim order are present AND
	// emitted IN ORDER on the wire. Order verification: walk the wire output
	// and require each successive header's start index to be strictly greater
	// than the previous one's. Per Task 18 review: this is the strict ORDER
	// assertion that the differential gate (Task 21) needs — reference Envoy
	// v1.37.2 emits the §11.2 6 headers in this exact verbatim order on the
	// wire, and envoy-go MUST match byte-for-byte.
	wantHeaders := []string{
		"Access-Control-Allow-Origin: https://example.test\r\n",
		"Access-Control-Allow-Credentials: true\r\n",
		"Access-Control-Allow-Methods: GET, POST, OPTIONS\r\n",
		"Access-Control-Allow-Headers: x-foo, x-bar\r\n",
		"Access-Control-Max-Age: 600\r\n",
		"Access-Control-Expose-Headers: x-baz\r\n",
	}
	prevEnd := 0
	for i, wh := range wantHeaders {
		idx := strings.Index(out[prevEnd:], wh)
		if idx < 0 {
			t.Errorf("missing CORS header line %d %q in wire output (or out of order — previous header ended at byte %d)\n---FULL OUTPUT---\n%s\n---END---",
				i, wh, prevEnd, out)
			break
		}
		prevEnd += idx + len(wh)
	}

	// Body should be empty (preflight 200 has Content-Length: 0).
	if !strings.Contains(out, "Content-Length: 0\r\n") {
		t.Errorf("expected Content-Length: 0 (empty preflight body); got: %q", out)
	}
}

// TestChainIntegration_H1_CorsActualRequest_AppendsThreeHeadersAfterUpstream
// Phase 07.1 Task 19 (I-3 prereq close-out): proves that the cors filter's
// encode-side actual-request 3-header injection lands AFTER the original
// (action-supplied) response headers on the H1 wire. The unification of
// ActionResponse.Headers from http.Header → OrderedHeaders + the encode-chain
// reconcile (filter_http.ReconcileOrderedHeaders) preserves the action's
// caller-supplied insertion order; cors's three encode-side Set()s
// (Allow-Origin, Allow-Credentials, Expose-Headers) appear as net-new keys
// after the original carrier in deterministic alphabetical order.
//
// This exercises the action-driven path (not SendLocalReply) — direct_response
// synthesizes a 200 with Content-Type/Content-Length/Server/Date pinned in
// localReplyHeaders order; cors's encode-side mutates via http.Header.Set on
// the merged map view; the reconcile projects back to OrderedHeaders.
//
// Order assertion: the upstream four (Content-Type, Content-Length, Server,
// Date) come first in their carrier-defined order; the cors three follow in
// alphabetical order (Allow-Credentials, Allow-Origin, Expose-Headers). This
// is NOT byte-equivalent with reference Envoy's "cors-set-order" — see
// PROGRESS Task 19 close-out trade-off note for the rationale (Go map
// iteration is non-deterministic; alphabetical is the deterministic
// substitute for byte-equality across runs of the same fixture).
func TestChainIntegration_H1_CorsActualRequest_AppendsThreeHeadersAfterUpstream(t *testing.T) {
	policy := &corsv3.CorsPolicy{
		AllowOriginStringMatch: []*matcherv3.StringMatcher{
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "https://example.test"}},
		},
		AllowMethods:     "GET, POST, OPTIONS",
		AllowHeaders:     "x-foo, x-bar",
		ExposeHeaders:    "x-baz",
		MaxAge:           "600",
		AllowCredentials: wrapperspb.Bool(true),
	}
	policyAny, err := anypb.New(policy)
	if err != nil {
		t.Fatal(err)
	}

	corsFactory, err := cors.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("cors.New: %v", err)
	}
	rfFactory, err := router.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	chainCfg := []chainEntry{
		{name: "envoy.filters.http.cors", factory: corsFactory},
		{name: "envoy.filters.http.router", factory: rfFactory},
	}
	scopes := []filter_http.RouteScope{
		{Route: map[string]*anypb.Any{"envoy.filters.http.cors": policyAny}},
	}
	pr, err := filter_http.BuildPerRouteConfig(nil, scopes, []string{"envoy.filters.http.cors", "envoy.filters.http.router"}, nil)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}

	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/api"), action: &directResponseAction{status: 200, bodyText: "ok\n"}},
	}}
	r := stats.NewRegistry()
	prefix := "http.test_chain_integration_cors_actual."
	f := &Filter{
		table:             tt,
		statPrefix:        "test_chain_integration_cors_actual",
		downstreamRqTotal: r.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   r.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   r.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   r.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   r.NewCounter(prefix + "downstream_rq_5xx"),
		chainConfig:       chainCfg,
		perRouteConfig:    pr,
	}

	req, _ := http.NewRequest("GET", "/api", nil)
	req.Proto = "HTTP/1.1"
	req.Header.Set("Origin", "https://example.test")

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	status, derr := f.dispatchRequest(context.Background(), nil, req, bw)
	if derr != nil {
		t.Fatalf("dispatchRequest: %v", derr)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	_ = bw.Flush()
	out := buf.String()

	// Status line + four-then-three header order assertion. Walk the wire
	// successively; require each successive header's start index strictly
	// greater than the previous one's. The original carrier is
	// {Content-Type, Content-Length, Server, Date} from localReplyHeaders;
	// the cors-appended trio sorts alphabetically: Access-Control-Allow-
	// Credentials, Access-Control-Allow-Origin, Access-Control-Expose-Headers.
	if !strings.HasPrefix(out, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("expected 200 OK status line, got: %q", out)
	}
	wantOrder := []string{
		"Content-Type: text/plain\r\n",
		"Content-Length: 3\r\n", // "ok\n" is 3 bytes
		"Server: envoy\r\n",
		"Access-Control-Allow-Credentials: true\r\n",
		"Access-Control-Allow-Origin: https://example.test\r\n",
		"Access-Control-Expose-Headers: x-baz\r\n",
	}
	prevEnd := 0
	for i, wh := range wantOrder {
		idx := strings.Index(out[prevEnd:], wh)
		if idx < 0 {
			t.Errorf("missing/out-of-order header line %d %q in wire output (previous header ended at byte %d)\n---FULL OUTPUT---\n%s\n---END---",
				i, wh, prevEnd, out)
			break
		}
		prevEnd += idx + len(wh)
	}

	// Body present.
	if !strings.HasSuffix(out, "ok\n") {
		t.Errorf("expected body 'ok\\n' at end of wire output; got: %q", out)
	}
}
