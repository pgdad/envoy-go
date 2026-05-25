package wasm

// abi_callbacks_test.go — Task 11 tests for the abiCallbacks implementation
// of internalwasm.ABICallbacks for the per-stream HTTP-filter context per
// 25.1 SPEC §3.5 + §4.3 + parent §4.2 + §4.5 D6 + D-P3 ADR-0196 first
// co-consumer.
//
// Test surface (per PLAN Task 11):
//
//   1. Compile-time conformance: (*abiCallbacks)(nil) satisfies
//      internalwasm.ABICallbacks (verified via the package-level var in
//      abi_callbacks.go; this test re-pins via a fresh assertion to surface
//      regression at test-binary build).
//
//   2. Header-map dispatch (7 methods × 8 map-types):
//        - mapType 0 (request headers) routes to filter.requestHeaders;
//        - mapType 2 (response headers) routes to filter.responseHeaders;
//        - mapType 1/3/4/5/6/7 (deferred to 25.2) return Unimplemented
//          (for setters) / NotFound semantic (for getters).
//
//   3. GetHeaderMap sort discipline (parent §4.5 D6 guardrail (b)):
//      returned pairs are sorted by key.
//
//   4. GetProperty minimal-property-tree coverage (5 supported paths +
//      unknown path NotFound):
//        - request.path        → :path pseudo-header;
//        - request.method      → :method;
//        - request.host        → :authority;
//        - request.headers.<k> → named request header;
//        - response.headers.<k>→ named response header;
//        - anything else       → (nil, false).
//
//   5. SetProperty no-op-OK at 25.1 (the property tree is read-only at this
//      phase; CEL surface lands 25.2). Returns WasmResultOk.
//
//   6. SendLocalResponse captures *capturedLocalResponse on the filter
//      struct verbatim (statusCode, statusMsg, body, additionalHeaders,
//      grpcStatus all round-trip).
//
//   7. GetStatus (D-P3 ADR-0196 first co-consumer):
//        - encoderCb=nil decode-path returns (0, nil, false);
//        - encoderCb!=nil with code>0 returns (code, []byte("<code>"), true);
//        - encoderCb!=nil with code==0 returns (0, nil, false).
//
//   8. Log routes to filter log sink (vm.LogProxy via the filter's vm; at
//      Task 11 vm may be nil — defensive fallback).
//
//   9. GetLogLevel returns LogLevelInfo (simple default at 25.1).
//
//  10. GetCurrentTimeNanoseconds returns a non-zero monotonic-ish value
//      (sanity check against time.Now().UnixNano).
//
//  11. SetEffectiveContext + Done both return WasmResultOk at 25.1
//      (no-op acknowledgments; 25.2 may wire context switching).

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/proto"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// -----------------------------------------------------------------------------
// Test doubles for DecoderFilterCallbacks + EncoderFilterCallbacks.
//
// fakeDecoderCb satisfies envoyhttp.DecoderFilterCallbacks. Most methods are
// no-op stubs — abiCallbacks at Task 11 only consumes the headers-bridge
// surface (which goes through filter.requestHeaders, not via the callback).
// The interface assertion compile-fails if the interface gains a method we
// haven't stubbed; the implicit test is that the compile-time conformance
// check below succeeds.
// -----------------------------------------------------------------------------

type fakeDecoderCb struct{}

func (fakeDecoderCb) ContinueDecoding()                                    {}
func (fakeDecoderCb) SendLocalReply(int, string, envoyhttp.OrderedHeaders) {}
func (fakeDecoderCb) RequestRouteConfig() proto.Message                    { return nil }
func (fakeDecoderCb) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (fakeDecoderCb) EncodeHeaders(http.Header, bool)                    {}
func (fakeDecoderCb) EncodeData([]byte, bool)                            {}
func (fakeDecoderCb) EncodeTrailers(http.Header)                         {}
func (fakeDecoderCb) DownstreamPrincipal() []string                      { return nil }
func (fakeDecoderCb) DownstreamRemoteAddr() net.Addr                     { return nil }
func (fakeDecoderCb) DownstreamLocalAddr() net.Addr                      { return nil }
func (fakeDecoderCb) RouteRateLimits() []*routev3.RateLimit              { return nil }
func (fakeDecoderCb) VirtualHostRateLimits() []*routev3.RateLimit        { return nil }
func (fakeDecoderCb) RouteMetadata() *corev3.Metadata                    { return nil }
func (fakeDecoderCb) RouteIncludeVhRateLimits() bool                     { return false }
func (fakeDecoderCb) DownstreamTLSServerName() string                    { return "" }
func (fakeDecoderCb) DownstreamTLSPeerCertDER() []byte                   { return nil }
func (fakeDecoderCb) DownstreamProtocol() string                         { return "" }
func (fakeDecoderCb) ListenerPrincipal() string                          { return "" }
func (fakeDecoderCb) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (fakeDecoderCb) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

var _ envoyhttp.DecoderFilterCallbacks = fakeDecoderCb{}

// fakeEncoderCb satisfies envoyhttp.EncoderFilterCallbacks. The
// `responseStatus` field is the seed for the ADR-0196 ResponseStatus()
// accessor — Task 11's GetStatus consumes this verbatim (D-P3 first
// co-consumer of phase-23's framework primitive).
type fakeEncoderCb struct {
	responseStatus int
}

func (fakeEncoderCb) ContinueEncoding()                                  {}
func (fakeEncoderCb) EncodeHeaders(http.Header, bool)                    {}
func (fakeEncoderCb) EncodeData([]byte, bool)                            {}
func (fakeEncoderCb) EncodeTrailers(http.Header)                         {}
func (fakeEncoderCb) OverwriteBody([]byte)                               {}
func (fakeEncoderCb) BufferEncodedBody() []byte                          { return nil }
func (fakeEncoderCb) DownstreamRemoteAddr() net.Addr                     { return nil }
func (fakeEncoderCb) DownstreamLocalAddr() net.Addr                      { return nil }
func (fakeEncoderCb) DownstreamTLSServerName() string                    { return "" }
func (fakeEncoderCb) DownstreamTLSPeerCertDER() []byte                   { return nil }
func (fakeEncoderCb) DownstreamProtocol() string                         { return "" }
func (fakeEncoderCb) ListenerPrincipal() string                          { return "" }
func (fakeEncoderCb) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (fakeEncoderCb) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// ResponseStatus is the ADR-0196 accessor. Task 11's GetStatus re-consumes
// this — the FIRST co-consumer of the phase-23 framework primitive.
func (e fakeEncoderCb) ResponseStatus() int { return e.responseStatus }

var _ envoyhttp.EncoderFilterCallbacks = fakeEncoderCb{}

// -----------------------------------------------------------------------------
// Test helpers — build an abiCallbacks bound to a filter w/ controllable
// requestHeaders / responseHeaders / decoderCb / encoderCb.
// -----------------------------------------------------------------------------

func newTestABICallbacks(reqHeaders, respHeaders http.Header, dcb envoyhttp.DecoderFilterCallbacks, ecb envoyhttp.EncoderFilterCallbacks) (*abiCallbacks, *filter) {
	f := &filter{
		requestHeaders:  reqHeaders,
		responseHeaders: respHeaders,
		decoderCb:       dcb,
		encoderCb:       ecb,
	}
	cb := &abiCallbacks{filter: f}
	return cb, f
}

// -----------------------------------------------------------------------------
// 1. Compile-time conformance.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_ConformsToABICallbacks(t *testing.T) {
	t.Parallel()
	var _ internalwasm.ABICallbacks = (*abiCallbacks)(nil)
}

// -----------------------------------------------------------------------------
// 2 + 3. Header-map dispatch + sort discipline.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetHeaderMap_RequestHeaders_Sorted(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Set("X-Beta", "two")
	req.Set("X-Alpha", "one")
	req.Set("X-Gamma", "three")
	cb, _ := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	pairs, ok := cb.GetHeaderMap(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestHeaders)
	if !ok {
		t.Fatalf("ok = false; want true")
	}
	if len(pairs) != 3 {
		t.Fatalf("len(pairs) = %d; want 3 (pairs=%v)", len(pairs), pairs)
	}
	// Sort discipline per parent §4.5 D6 guardrail (b): pairs sorted by key.
	for i := 1; i < len(pairs); i++ {
		if pairs[i-1].Key > pairs[i].Key {
			t.Errorf("pairs not sorted: %q > %q (full: %v)", pairs[i-1].Key, pairs[i].Key, pairs)
		}
	}
}

func TestAbiCallbacks_GetHeaderMap_ResponseHeaders(t *testing.T) {
	t.Parallel()
	resp := http.Header{}
	resp.Set("Content-Type", "text/plain")
	cb, _ := newTestABICallbacks(nil, resp, nil, fakeEncoderCb{})

	pairs, ok := cb.GetHeaderMap(context.Background(), 1, abi.WasmHeaderMapTypeHttpResponseHeaders)
	if !ok {
		t.Fatalf("ok = false; want true")
	}
	if len(pairs) != 1 {
		t.Fatalf("len(pairs) = %d; want 1", len(pairs))
	}
	if pairs[0].Key != "Content-Type" || pairs[0].Value != "text/plain" {
		t.Errorf("pairs[0] = %+v; want {Content-Type, text/plain}", pairs[0])
	}
}

func TestAbiCallbacks_GetHeaderMap_NilHeaders_NotFound(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	_, ok := cb.GetHeaderMap(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestHeaders)
	if ok {
		t.Errorf("request: ok = true; want false (no headers)")
	}
	_, ok = cb.GetHeaderMap(context.Background(), 1, abi.WasmHeaderMapTypeHttpResponseHeaders)
	if ok {
		t.Errorf("response: ok = true; want false (no headers)")
	}
}

func TestAbiCallbacks_GetHeaderMap_DeferredMapTypes_NotFound(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(http.Header{"X": []string{"y"}}, http.Header{"X": []string{"y"}}, fakeDecoderCb{}, fakeEncoderCb{})

	deferred := []abi.WasmHeaderMapType{
		abi.WasmHeaderMapTypeHttpRequestTrailers,
		abi.WasmHeaderMapTypeHttpResponseTrailers,
		abi.WasmHeaderMapTypeHttpCallResponseHeaders,
		abi.WasmHeaderMapTypeHttpCallResponseTrailers,
		abi.WasmHeaderMapTypeGrpcReceiveInitialMetadata,
		abi.WasmHeaderMapTypeGrpcReceiveTrailingMetadata,
	}
	for _, mt := range deferred {
		_, ok := cb.GetHeaderMap(context.Background(), 1, mt)
		if ok {
			t.Errorf("mapType %d: ok = true; want false (deferred to 25.2)", mt)
		}
	}
}

func TestAbiCallbacks_GetHeaderMapValue(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Set("X-Foo", "bar")
	cb, _ := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	val, ok := cb.GetHeaderMapValue(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestHeaders, "X-Foo")
	if !ok || val != "bar" {
		t.Errorf("GetHeaderMapValue = (%q, %v); want (\"bar\", true)", val, ok)
	}
	// Missing key.
	_, ok = cb.GetHeaderMapValue(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestHeaders, "Missing")
	if ok {
		t.Errorf("Missing key: ok = true; want false")
	}
	// Deferred map type.
	_, ok = cb.GetHeaderMapValue(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestTrailers, "X-Foo")
	if ok {
		t.Errorf("Deferred map type: ok = true; want false")
	}
}

func TestAbiCallbacks_AddHeaderMapValue(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	cb, f := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	if got := cb.AddHeaderMapValue(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestHeaders, "X-Foo", "bar"); got != abi.WasmResultOk {
		t.Fatalf("AddHeaderMapValue = %v; want Ok", got)
	}
	if got := f.requestHeaders.Get("X-Foo"); got != "bar" {
		t.Errorf("requestHeaders[X-Foo] = %q; want bar", got)
	}
	// Add same key again — http.Header.Add appends.
	cb.AddHeaderMapValue(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestHeaders, "X-Foo", "baz")
	if vs := f.requestHeaders.Values("X-Foo"); len(vs) != 2 || vs[0] != "bar" || vs[1] != "baz" {
		t.Errorf("Values = %v; want [bar baz]", vs)
	}
	// Deferred map type returns Unimplemented.
	if got := cb.AddHeaderMapValue(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestTrailers, "X", "Y"); got != abi.WasmResultUnimplemented {
		t.Errorf("Deferred AddHeaderMapValue = %v; want Unimplemented", got)
	}
}

func TestAbiCallbacks_ReplaceHeaderMapValue(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Add("X-Foo", "old1")
	req.Add("X-Foo", "old2")
	cb, f := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	if got := cb.ReplaceHeaderMapValue(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestHeaders, "X-Foo", "new"); got != abi.WasmResultOk {
		t.Fatalf("ReplaceHeaderMapValue = %v; want Ok", got)
	}
	if vs := f.requestHeaders.Values("X-Foo"); len(vs) != 1 || vs[0] != "new" {
		t.Errorf("Values after Replace = %v; want [new]", vs)
	}
	// Deferred map type returns Unimplemented.
	if got := cb.ReplaceHeaderMapValue(context.Background(), 1, abi.WasmHeaderMapTypeHttpResponseTrailers, "X", "Y"); got != abi.WasmResultUnimplemented {
		t.Errorf("Deferred Replace = %v; want Unimplemented", got)
	}
}

func TestAbiCallbacks_RemoveHeaderMapValue(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Set("X-Foo", "bar")
	cb, f := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	if got := cb.RemoveHeaderMapValue(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestHeaders, "X-Foo"); got != abi.WasmResultOk {
		t.Fatalf("RemoveHeaderMapValue = %v; want Ok", got)
	}
	if vs := f.requestHeaders.Values("X-Foo"); len(vs) != 0 {
		t.Errorf("Values after Remove = %v; want []", vs)
	}
	// Deferred map type returns Unimplemented.
	if got := cb.RemoveHeaderMapValue(context.Background(), 1, abi.WasmHeaderMapTypeHttpCallResponseHeaders, "X"); got != abi.WasmResultUnimplemented {
		t.Errorf("Deferred Remove = %v; want Unimplemented", got)
	}
}

func TestAbiCallbacks_SetHeaderMapPairs(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Set("X-Old", "old")
	cb, f := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	pairs := []internalwasm.HeaderPair{
		{Key: "X-New", Value: "1"},
		{Key: "X-Another", Value: "2"},
	}
	if got := cb.SetHeaderMapPairs(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestHeaders, pairs); got != abi.WasmResultOk {
		t.Fatalf("SetHeaderMapPairs = %v; want Ok", got)
	}
	// Old keys gone; new keys present.
	if v := f.requestHeaders.Get("X-Old"); v != "" {
		t.Errorf("X-Old = %q after SetHeaderMapPairs; want empty (replaced)", v)
	}
	if v := f.requestHeaders.Get("X-New"); v != "1" {
		t.Errorf("X-New = %q; want 1", v)
	}
	if v := f.requestHeaders.Get("X-Another"); v != "2" {
		t.Errorf("X-Another = %q; want 2", v)
	}
	// Deferred map type returns Unimplemented.
	if got := cb.SetHeaderMapPairs(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestTrailers, pairs); got != abi.WasmResultUnimplemented {
		t.Errorf("Deferred SetHeaderMapPairs = %v; want Unimplemented", got)
	}
}

func TestAbiCallbacks_GetHeaderMapSize(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Set("A", "1")
	req.Set("B", "2")
	req.Add("A", "3") // 2 keys, 3 total values
	cb, _ := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	// Size returns total value count (3) — matches how a guest would
	// iterate via proxy_get_header_map_pairs.
	size := cb.GetHeaderMapSize(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestHeaders)
	if size != 3 {
		t.Errorf("GetHeaderMapSize = %d; want 3", size)
	}
	// Deferred map type → 0.
	size = cb.GetHeaderMapSize(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestTrailers)
	if size != 0 {
		t.Errorf("Deferred GetHeaderMapSize = %d; want 0", size)
	}
	// Nil headers → 0.
	cbNil, _ := newTestABICallbacks(nil, nil, nil, nil)
	size = cbNil.GetHeaderMapSize(context.Background(), 1, abi.WasmHeaderMapTypeHttpRequestHeaders)
	if size != 0 {
		t.Errorf("Nil headers GetHeaderMapSize = %d; want 0", size)
	}
}

// -----------------------------------------------------------------------------
// 4. GetProperty minimal-property-tree coverage.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetProperty_RequestPath(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Set(":path", "/index.html")
	cb, _ := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"request", "path"})
	if !ok || string(val) != "/index.html" {
		t.Errorf("request.path = (%q, %v); want (\"/index.html\", true)", val, ok)
	}
}

func TestAbiCallbacks_GetProperty_RequestMethod(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Set(":method", "POST")
	cb, _ := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"request", "method"})
	if !ok || string(val) != "POST" {
		t.Errorf("request.method = (%q, %v); want (\"POST\", true)", val, ok)
	}
}

func TestAbiCallbacks_GetProperty_RequestHost(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Set(":authority", "example.com")
	cb, _ := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"request", "host"})
	if !ok || string(val) != "example.com" {
		t.Errorf("request.host = (%q, %v); want (\"example.com\", true)", val, ok)
	}
}

func TestAbiCallbacks_GetProperty_RequestHeaders_Named(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Set("User-Agent", "curl/8.0")
	cb, _ := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"request", "headers", "User-Agent"})
	if !ok || string(val) != "curl/8.0" {
		t.Errorf("request.headers.User-Agent = (%q, %v); want (\"curl/8.0\", true)", val, ok)
	}
	// Missing header → (nil, false).
	_, ok = cb.GetProperty(context.Background(), 1, []string{"request", "headers", "Missing"})
	if ok {
		t.Errorf("Missing request.headers.* ok = true; want false")
	}
}

func TestAbiCallbacks_GetProperty_ResponseHeaders_Named(t *testing.T) {
	t.Parallel()
	resp := http.Header{}
	resp.Set("Content-Type", "application/json")
	cb, _ := newTestABICallbacks(nil, resp, nil, fakeEncoderCb{})

	val, ok := cb.GetProperty(context.Background(), 1, []string{"response", "headers", "Content-Type"})
	if !ok || string(val) != "application/json" {
		t.Errorf("response.headers.Content-Type = (%q, %v); want (\"application/json\", true)", val, ok)
	}
	// Missing header.
	_, ok = cb.GetProperty(context.Background(), 1, []string{"response", "headers", "Missing"})
	if ok {
		t.Errorf("Missing response.headers.* ok = true; want false")
	}
}

func TestAbiCallbacks_GetProperty_UnknownPaths(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Set(":path", "/")
	cb, _ := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	cases := [][]string{
		nil,                              // empty
		{},                               // empty slice
		{"unknown"},                      // top-level unknown
		{"request"},                      // truncated
		{"request", "unknown"},           // unknown sub-key
		{"request", "headers"},           // missing header name
		{"response"},                     // truncated
		{"response", "headers"},          // missing header name
		{"response", "path"},             // response has no path
		{"connection", "remote_address"}, // future CEL surface (25.2)
	}
	for _, p := range cases {
		_, ok := cb.GetProperty(context.Background(), 1, p)
		if ok {
			t.Errorf("GetProperty(%v) ok = true; want false (unknown path)", p)
		}
	}
}

func TestAbiCallbacks_GetProperty_NilHeaders(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	for _, p := range [][]string{
		{"request", "path"},
		{"request", "method"},
		{"request", "host"},
		{"request", "headers", "X"},
		{"response", "headers", "X"},
	} {
		_, ok := cb.GetProperty(context.Background(), 1, p)
		if ok {
			t.Errorf("GetProperty(%v) on nil headers: ok = true; want false", p)
		}
	}
}

// -----------------------------------------------------------------------------
// 5. SetProperty — 25.1 returns Ok no-op (CEL surface lands 25.2).
// -----------------------------------------------------------------------------

func TestAbiCallbacks_SetProperty_NoOpOk(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	got := cb.SetProperty(context.Background(), 1, []string{"foo", "bar"}, []byte("value"))
	if got != abi.WasmResultOk {
		t.Errorf("SetProperty = %v; want Ok (25.1 no-op)", got)
	}
}

// -----------------------------------------------------------------------------
// 6. SendLocalResponse capture.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_SendLocalResponse_Captures(t *testing.T) {
	t.Parallel()
	cb, f := newTestABICallbacks(nil, nil, fakeDecoderCb{}, nil)

	addl := []internalwasm.HeaderPair{
		{Key: "X-Reason", Value: "rejected"},
	}
	got := cb.SendLocalResponse(context.Background(), 1, 503, "Service Unavailable", "denied", addl, -1)
	if got != abi.WasmResultOk {
		t.Fatalf("SendLocalResponse = %v; want Ok", got)
	}
	if f.sentLocalResponse == nil {
		t.Fatalf("f.sentLocalResponse = nil; want non-nil capture")
	}
	if f.sentLocalResponse.statusCode != 503 {
		t.Errorf("statusCode = %d; want 503", f.sentLocalResponse.statusCode)
	}
	if f.sentLocalResponse.statusMsg != "Service Unavailable" {
		t.Errorf("statusMsg = %q; want \"Service Unavailable\"", f.sentLocalResponse.statusMsg)
	}
	if f.sentLocalResponse.body != "denied" {
		t.Errorf("body = %q; want \"denied\"", f.sentLocalResponse.body)
	}
	if len(f.sentLocalResponse.additionalHeaders) != 1 || f.sentLocalResponse.additionalHeaders[0].Key != "X-Reason" {
		t.Errorf("additionalHeaders = %v; want [X-Reason rejected]", f.sentLocalResponse.additionalHeaders)
	}
	if f.sentLocalResponse.grpcStatus != -1 {
		t.Errorf("grpcStatus = %d; want -1", f.sentLocalResponse.grpcStatus)
	}
}

func TestAbiCallbacks_SendLocalResponse_NilFilter_InternalFailure(t *testing.T) {
	t.Parallel()
	cb := &abiCallbacks{filter: nil}

	got := cb.SendLocalResponse(context.Background(), 1, 503, "", "", nil, -1)
	if got != abi.WasmResultInternalFailure {
		t.Errorf("SendLocalResponse with nil filter = %v; want InternalFailure", got)
	}
}

// -----------------------------------------------------------------------------
// 7. GetStatus — D-P3 ADR-0196 first co-consumer.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetStatus_EncoderCbNil_NotFound(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, fakeDecoderCb{}, nil)

	code, val, ok := cb.GetStatus(context.Background(), 1)
	if ok || code != 0 || val != nil {
		t.Errorf("GetStatus(decode-path) = (%d, %v, %v); want (0, nil, false)", code, val, ok)
	}
}

func TestAbiCallbacks_GetStatus_EncoderCbCodeZero_NotFound(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, fakeEncoderCb{responseStatus: 0})

	code, val, ok := cb.GetStatus(context.Background(), 1)
	if ok || code != 0 || val != nil {
		t.Errorf("GetStatus(zero status) = (%d, %v, %v); want (0, nil, false)", code, val, ok)
	}
}

// TestAbiCallbacks_GetStatus_AdrAccessor exercises the D-P3 ADR-0196 first
// co-consumer path: encoderCb.ResponseStatus() returns the HTTP response
// status code (set-once by HCM dispatch per phase-23 IMPL Task 9a); GetStatus
// projects to (uint32(code), []byte("<code>"), true).
//
// This is the FIRST co-consumer of the phase-23 framework primitive
// (admission_control's encode.go is the FIRST consumer; Task 11 wasm is the
// SECOND co-consumer — RATIFIES the extraction discipline analogous to
// phase-22.2's first co-consumer of phase-20 internal/httpclient/).
func TestAbiCallbacks_GetStatus_AdrAccessor_FirstCoConsumer(t *testing.T) {
	t.Parallel()
	for _, status := range []int{200, 301, 404, 500, 503} {
		cb, _ := newTestABICallbacks(nil, nil, nil, fakeEncoderCb{responseStatus: status})

		code, val, ok := cb.GetStatus(context.Background(), 1)
		if !ok {
			t.Errorf("status=%d: ok=false; want true", status)
		}
		if int(code) != status {
			t.Errorf("status=%d: code = %d; want %d", status, code, status)
		}
		want := strconv.Itoa(status)
		if string(val) != want {
			t.Errorf("status=%d: val = %q; want %q", status, string(val), want)
		}
	}
}

// -----------------------------------------------------------------------------
// 8. Log routing.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_Log_RoutesViaVMLogSink(t *testing.T) {
	t.Parallel()
	// Build a real VM with a sink so Log routes through vm.LogProxy.
	var sink bytes.Buffer
	vm := internalwasm.NewVM(context.Background(), internalwasm.WithLogSink(&sink))
	defer func() { _ = vm.Close() }()

	f := &filter{vm: vm}
	cb := &abiCallbacks{filter: f}

	cb.Log(context.Background(), 1, abi.LogLevelInfo, "hello world")

	out := sink.String()
	if !bytes.Contains([]byte(out), []byte("hello world")) {
		t.Errorf("sink does not contain log message; got %q", out)
	}
	if !bytes.Contains([]byte(out), []byte("info")) {
		t.Errorf("sink does not contain level name; got %q", out)
	}
}

func TestAbiCallbacks_Log_NilVM_NoCrash(t *testing.T) {
	t.Parallel()
	cb := &abiCallbacks{filter: &filter{vm: nil}}

	// Must not panic on nil vm.
	cb.Log(context.Background(), 1, abi.LogLevelError, "should not crash")
}

func TestAbiCallbacks_Log_NilFilter_NoCrash(t *testing.T) {
	t.Parallel()
	cb := &abiCallbacks{filter: nil}

	// Must not panic on nil filter.
	cb.Log(context.Background(), 1, abi.LogLevelError, "should not crash")
}

// -----------------------------------------------------------------------------
// 9. GetLogLevel — simple default at 25.1.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetLogLevel(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	if got := cb.GetLogLevel(context.Background()); got != abi.LogLevelInfo {
		t.Errorf("GetLogLevel = %v; want LogLevelInfo", got)
	}
}

// -----------------------------------------------------------------------------
// 10. GetCurrentTimeNanoseconds — sanity-check non-zero, monotonic-ish.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetCurrentTimeNanoseconds(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	before := uint64(time.Now().UnixNano())
	got := cb.GetCurrentTimeNanoseconds(context.Background())
	after := uint64(time.Now().UnixNano())

	if got == 0 {
		t.Errorf("GetCurrentTimeNanoseconds = 0; want non-zero")
	}
	if got < before || got > after {
		t.Errorf("GetCurrentTimeNanoseconds = %d; want in [%d, %d]", got, before, after)
	}
}

// -----------------------------------------------------------------------------
// 11. SetEffectiveContext + Done — 25.1 no-op acknowledgments.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_SetEffectiveContext_Ok(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	if got := cb.SetEffectiveContext(context.Background(), 42); got != abi.WasmResultOk {
		t.Errorf("SetEffectiveContext = %v; want Ok", got)
	}
}

func TestAbiCallbacks_Done_Ok(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	if got := cb.Done(context.Background(), 42); got != abi.WasmResultOk {
		t.Errorf("Done = %v; want Ok", got)
	}
}
