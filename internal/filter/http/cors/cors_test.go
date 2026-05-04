package cors

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// makeCorsPolicy builds a CorsPolicy with the standard §11.2 probe shape:
// allow_origin_string_match=[exact: "https://example.test"], allow_methods,
// allow_headers, expose_headers, allow_credentials=true, max_age.
func makeCorsPolicy(allowedOrigins []string) *corsv3.CorsPolicy {
	matchers := make([]*matcherv3.StringMatcher, 0, len(allowedOrigins))
	for _, o := range allowedOrigins {
		matchers = append(matchers, &matcherv3.StringMatcher{
			MatchPattern: &matcherv3.StringMatcher_Exact{Exact: o},
		})
	}
	return &corsv3.CorsPolicy{
		AllowOriginStringMatch: matchers,
		AllowMethods:           "GET, POST, OPTIONS",
		AllowHeaders:           "x-foo, x-bar",
		ExposeHeaders:          "x-baz",
		MaxAge:                 "600",
		AllowCredentials:       wrapperspb.Bool(true),
	}
}

// recordingTerminal is a test-only filter that captures the response shape
// surfaced through the encode chain. Stand-in for the router's terminal step
// in cors-only chain tests.
type recordingTerminal struct {
	encodeHeaders http.Header
	encodeBody    []byte
	encodeCalled  bool
}

func (r *recordingTerminal) DecodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
}
func (r *recordingTerminal) DecodeData([]byte, bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}
func (r *recordingTerminal) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (r *recordingTerminal) SetDecoderCallbacks(envoyhttp.DecoderFilterCallbacks) {}

func (r *recordingTerminal) EncodeHeaders(h http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Capture the headers the chain hands to the terminal step (post-encode-chain).
	// The chain visits the terminal step LAST on encode side (reverse order).
	r.encodeHeaders = h
	r.encodeCalled = true
	return envoyhttp.Continue
}
func (r *recordingTerminal) EncodeData(d []byte, _ bool) envoyhttp.FilterDataStatus {
	r.encodeBody = append(r.encodeBody, d...)
	return envoyhttp.DataContinue
}
func (r *recordingTerminal) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (r *recordingTerminal) SetEncoderCallbacks(envoyhttp.EncoderFilterCallbacks) {}
func (r *recordingTerminal) OnDestroy()                                           {}

// buildChain assembles a FilterChain with the cors filter ahead of a recording
// terminal. The cors filter is at index 0; the terminal at index 1. Per-route
// config supplied via the perroute.PerRouteConfig built from a single scope.
func buildChain(t *testing.T, policy *corsv3.CorsPolicy) (*envoyhttp.FilterChain, *recordingTerminal, *filter) {
	t.Helper()
	cors := &filter{}
	term := &recordingTerminal{}

	// Build the per-route config. The policy hangs off the route scope under
	// the cors filter name. perroute.BuildPerRouteConfig validates keys against
	// the chain filter names.
	policyAny, err := anypb.New(policy)
	if err != nil {
		t.Fatalf("anypb.New(policy): %v", err)
	}
	scope := envoyhttp.RouteScope{
		Route: map[string]*anypb.Any{"envoy.filters.http.cors": policyAny},
	}
	chainNames := []string{"envoy.filters.http.cors", "test.terminal"}
	pr, err := envoyhttp.BuildPerRouteConfig(nil, []envoyhttp.RouteScope{scope}, chainNames, nil)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}

	chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
		{Name: "envoy.filters.http.cors", Decoder: cors, Encoder: cors},
		{Name: "test.terminal", Decoder: term, Encoder: term},
	}, pr)
	chain.SetRequestCtx(context.Background(), 0)
	return chain, term, cors
}

// TestCors_Preflight_AllowedOriginEmits200WithSixHeaders verifies that an
// OPTIONS preflight with an allow-listed origin triggers SendLocalReply(200)
// with the six CORS headers in the §11.2 verbatim order.
//
// Order assertion: the chain's LocalReplyResponse() returns OrderedHeaders —
// a slice carrying caller-supplied insertion order through the encode chain
// and out to HCM dispatch. The test walks the slice in order and asserts the
// six §11.2 headers appear in the pinned sequence (Allow-Origin first,
// Expose-Headers last). This is the strict ORDER assertion that closes the
// Task 18 review gap (Go map iteration on http.Header was non-deterministic;
// stdlib Header.Write emits keys alphabetically — both lose §11.2 order).
func TestCors_Preflight_AllowedOriginEmits200WithSixHeaders(t *testing.T) {
	policy := makeCorsPolicy([]string{"https://example.test"})
	chain, term, _ := buildChain(t, policy)

	headers := http.Header{}
	headers.Set(":method", "OPTIONS")
	headers.Set("Origin", "https://example.test")
	headers.Set("Access-Control-Request-Method", "GET")
	headers.Set("Access-Control-Request-Headers", "x-foo,x-bar")

	terminated, err := chain.RunDecodeHeaders(context.Background(), headers, true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	// SendLocalReply triggers the encode chain inline; iteration aborts.
	if terminated {
		t.Fatalf("expected terminated=false after SendLocalReply, got true")
	}

	// The terminal's EncodeHeaders should have observed the synthesized
	// preflight response.
	if !term.encodeCalled {
		t.Fatalf("recordingTerminal.EncodeHeaders never called; encode chain did not run")
	}

	// Strict ORDER assertion (Task 18 review fix): walk the OrderedHeaders
	// returned by chain.LocalReplyResponse() and verify the six §11.2 CORS
	// headers appear in the pinned sequence. The slice may carry additional
	// framework-injected entries (Content-Length, Content-Type) AFTER the
	// caller-pinned six per beginLocalReply's reconcile step — those are not
	// part of the §11.2 pin and are tolerated, but the relative order of the
	// six MUST match.
	if !chain.LocalReplyDone() {
		t.Fatalf("chain.LocalReplyDone() = false; expected SendLocalReply path to have fired")
	}
	gotStatus, gotHeaders, gotBody := chain.LocalReplyResponse()
	if gotStatus != 200 {
		t.Errorf("status = %d, want 200", gotStatus)
	}
	if len(gotBody) != 0 {
		t.Errorf("preflight body should be empty; got %q", string(gotBody))
	}

	// The six §11.2 headers in pinned order, with verbatim values.
	wantOrder := []envoyhttp.HeaderField{
		{Name: "Access-Control-Allow-Origin", Value: "https://example.test"},
		{Name: "Access-Control-Allow-Credentials", Value: "true"},
		{Name: "Access-Control-Allow-Methods", Value: "GET, POST, OPTIONS"},
		{Name: "Access-Control-Allow-Headers", Value: "x-foo, x-bar"},
		{Name: "Access-Control-Max-Age", Value: "600"},
		{Name: "Access-Control-Expose-Headers", Value: "x-baz"},
	}
	// Filter the carrier down to entries whose canonical name is in the
	// §11.2 set, then assert slice equality (name + value, in order).
	pinSet := make(map[string]bool, len(wantOrder))
	for _, hf := range wantOrder {
		pinSet[http.CanonicalHeaderKey(hf.Name)] = true
	}
	got := make([]envoyhttp.HeaderField, 0, len(wantOrder))
	for _, hf := range gotHeaders {
		if pinSet[http.CanonicalHeaderKey(hf.Name)] {
			got = append(got, envoyhttp.HeaderField{
				Name:  http.CanonicalHeaderKey(hf.Name),
				Value: hf.Value,
			})
		}
	}
	if len(got) != len(wantOrder) {
		t.Fatalf("§11.2 header count = %d; want %d. got=%v want=%v", len(got), len(wantOrder), got, wantOrder)
	}
	for i := range wantOrder {
		if got[i].Name != wantOrder[i].Name || got[i].Value != wantOrder[i].Value {
			t.Errorf("§11.2 header[%d] = {Name:%q Value:%q}; want {Name:%q Value:%q}",
				i, got[i].Name, got[i].Value, wantOrder[i].Name, wantOrder[i].Value)
		}
	}
}

// TestCors_Preflight_DisallowedOriginPassesThrough verifies that an OPTIONS
// preflight with an origin NOT on the allow-list does NOT trigger SendLocalReply
// — the chain continues to the next filter (which would be the router's 405).
func TestCors_Preflight_DisallowedOriginPassesThrough(t *testing.T) {
	policy := makeCorsPolicy([]string{"https://example.test"})
	chain, term, _ := buildChain(t, policy)

	headers := http.Header{}
	headers.Set(":method", "OPTIONS")
	headers.Set("Origin", "https://other.test")
	headers.Set("Access-Control-Request-Method", "GET")

	terminated, err := chain.RunDecodeHeaders(context.Background(), headers, true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected terminated=true (passthrough); got false (SendLocalReply fired)")
	}
	// Encode chain should NOT have run on the decode-side passthrough — the
	// terminal's recording state stays unset.
	if term.encodeCalled {
		t.Errorf("encode chain ran on disallowed-origin preflight; expected passthrough")
	}
}

// TestCors_ActualRequest_AllowedOriginAddsThreeHeaders verifies that on the
// actual GET path with an allow-listed origin, the cors filter's EncodeHeaders
// injects the three CORS response headers (allow-origin, allow-credentials,
// expose-headers — NOT the preflight-only allow-methods/allow-headers/max-age).
func TestCors_ActualRequest_AllowedOriginAddsThreeHeaders(t *testing.T) {
	policy := makeCorsPolicy([]string{"https://example.test"})
	chain, term, _ := buildChain(t, policy)

	// Decode side: actual GET request with Origin → cors records originAllowed.
	decodeHeaders := http.Header{}
	decodeHeaders.Set(":method", "GET")
	decodeHeaders.Set("Origin", "https://example.test")

	if _, err := chain.RunDecodeHeaders(context.Background(), decodeHeaders, true); err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if term.encodeCalled {
		t.Fatalf("encode chain ran during decode; expected only on encode-side")
	}

	// Encode side: feed an upstream 200 response shape through.
	respHeaders := http.Header{}
	respHeaders.Set("Content-Type", "text/html")
	respHeaders.Set("Content-Length", "5")

	if _, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false); err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}

	// The terminal's encodeHeaders should now reflect the CORS-injected
	// headers (the chain runs reverse: terminal first, then cors mutates;
	// since the terminal captures by reference, the post-cors mutation is
	// visible).
	if got := respHeaders.Get("Access-Control-Allow-Origin"); got != "https://example.test" {
		t.Errorf("Access-Control-Allow-Origin = %q; want https://example.test", got)
	}
	if got := respHeaders.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q; want true", got)
	}
	if got := respHeaders.Get("Access-Control-Expose-Headers"); got != "x-baz" {
		t.Errorf("Access-Control-Expose-Headers = %q; want x-baz", got)
	}
	// Preflight-only headers MUST NOT appear on the actual response.
	for _, h := range []string{"Access-Control-Allow-Methods", "Access-Control-Allow-Headers", "Access-Control-Max-Age"} {
		if v := respHeaders.Get(h); v != "" {
			t.Errorf("preflight-only header %q present on actual response: %q", h, v)
		}
	}
}

// TestCors_ActualRequest_NoOriginIsNoOp verifies that a GET without an Origin
// header produces no CORS response headers (filter is a no-op).
func TestCors_ActualRequest_NoOriginIsNoOp(t *testing.T) {
	policy := makeCorsPolicy([]string{"https://example.test"})
	chain, _, _ := buildChain(t, policy)

	decodeHeaders := http.Header{}
	decodeHeaders.Set(":method", "GET")
	if _, err := chain.RunDecodeHeaders(context.Background(), decodeHeaders, true); err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}

	respHeaders := http.Header{}
	respHeaders.Set("Content-Type", "text/html")
	if _, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false); err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}
	for _, h := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Access-Control-Expose-Headers"} {
		if v := respHeaders.Get(h); v != "" {
			t.Errorf("expected no %q on no-origin response, got %q", h, v)
		}
	}
}

// TestCors_PerRouteOverride verifies that two different routes can carry
// different per-route CorsPolicy entries and the cors filter resolves the
// correct one via RequestRouteConfig().
func TestCors_PerRouteOverride(t *testing.T) {
	// Two routes: route 0 allows example.test; route 1 allows other.test.
	policy0Any, err := anypb.New(makeCorsPolicy([]string{"https://example.test"}))
	if err != nil {
		t.Fatal(err)
	}
	policy1Any, err := anypb.New(makeCorsPolicy([]string{"https://other.test"}))
	if err != nil {
		t.Fatal(err)
	}
	scopes := []envoyhttp.RouteScope{
		{Route: map[string]*anypb.Any{"envoy.filters.http.cors": policy0Any}},
		{Route: map[string]*anypb.Any{"envoy.filters.http.cors": policy1Any}},
	}
	chainNames := []string{"envoy.filters.http.cors", "test.terminal"}
	pr, err := envoyhttp.BuildPerRouteConfig(nil, scopes, chainNames, nil)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}

	// Route 0 chain: example.test should be allowed; other.test passthrough.
	cors0 := &filter{}
	term0 := &recordingTerminal{}
	chain0 := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
		{Name: "envoy.filters.http.cors", Decoder: cors0, Encoder: cors0},
		{Name: "test.terminal", Decoder: term0, Encoder: term0},
	}, pr)
	chain0.SetRequestCtx(context.Background(), 0)

	h0 := http.Header{}
	h0.Set(":method", "OPTIONS")
	h0.Set("Origin", "https://example.test")
	h0.Set("Access-Control-Request-Method", "GET")
	terminated, err := chain0.RunDecodeHeaders(context.Background(), h0, true)
	if err != nil {
		t.Fatalf("route0 RunDecodeHeaders: %v", err)
	}
	if terminated {
		t.Fatalf("route0: expected SendLocalReply (terminated=false) on allowed origin; got terminated=true")
	}
	if got := term0.encodeHeaders.Get("Access-Control-Allow-Origin"); got != "https://example.test" {
		t.Errorf("route0 allow-origin = %q; want https://example.test", got)
	}

	// Route 1 chain: example.test now disallowed (passthrough); other.test allowed.
	cors1 := &filter{}
	term1 := &recordingTerminal{}
	chain1 := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
		{Name: "envoy.filters.http.cors", Decoder: cors1, Encoder: cors1},
		{Name: "test.terminal", Decoder: term1, Encoder: term1},
	}, pr)
	chain1.SetRequestCtx(context.Background(), 1)

	h1 := http.Header{}
	h1.Set(":method", "OPTIONS")
	h1.Set("Origin", "https://example.test") // disallowed under route 1's policy
	h1.Set("Access-Control-Request-Method", "GET")
	terminated, err = chain1.RunDecodeHeaders(context.Background(), h1, true)
	if err != nil {
		t.Fatalf("route1 RunDecodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("route1: expected passthrough on disallowed origin; got SendLocalReply")
	}
	if term1.encodeCalled {
		t.Errorf("route1: encode chain should not have run on passthrough")
	}
}

// TestCors_FactoryRoundTrip verifies that the package's New function (the
// HTTPFilterFactory) accepts a *Cors anypb config and returns a working
// FilterInstanceFactory.
func TestCors_FactoryRoundTrip(t *testing.T) {
	tc, err := anypb.New(&corsv3.Cors{})
	if err != nil {
		t.Fatalf("anypb.New(Cors): %v", err)
	}
	fac, err := New(tc, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if fac == nil {
		t.Fatal("New returned nil factory")
	}
	hf := fac()
	if hf.Name != "envoy.filters.http.cors" {
		t.Errorf("filter name = %q; want envoy.filters.http.cors", hf.Name)
	}
	if hf.Decoder == nil || hf.Encoder == nil {
		t.Error("expected both Decoder + Encoder populated")
	}
	// TypeURL constant matches the message type.
	if !strings.HasSuffix(TypeURL, "envoy.extensions.filters.http.cors.v3.Cors") {
		t.Errorf("TypeURL = %q; want suffix envoy.extensions.filters.http.cors.v3.Cors", TypeURL)
	}
}
