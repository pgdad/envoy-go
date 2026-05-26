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
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filterstate"
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
	// Build a real RootVM with a sink so Log routes through rootVM.LogProxy.
	// REVISED at 25.2 Task 18 for the root-VM model: the log sink lives on
	// the per-compiledConfig shared *RootVM rather than per-stream (a
	// per-listener resource, not a per-stream resource).
	var sink bytes.Buffer
	ctx := context.Background()
	cache := internalwasm.NewCompileCache(ctx)
	defer func() { _ = cache.Close() }()
	mod, err := internalwasm.CompileModule(ctx, buildMinimalProxyWasm(), cache)
	if err != nil {
		t.Fatalf("CompileModule: %v", err)
	}
	rv, err := internalwasm.NewRootVM(ctx, mod, 1, internalwasm.WithRootLogSink(&sink))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	cc := &compiledConfig{rootVM: rv}
	f := &filter{cfg: cc}
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
	// REVISED at 25.2 Task 18: the rootVM lives on cfg, not directly on
	// filter. Nil-rootVM path = nil-cfg path = nil-filter path; all
	// degrade to a no-op log drop.
	cb := &abiCallbacks{filter: &filter{cfg: &compiledConfig{rootVM: nil}}}

	// Must not panic on nil rootVM.
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

// =============================================================================
// 25.2 IMPL Task 15 — extensions per 25.2 SPEC §3.6 + §5.3 + AMEND-B4.
//
// Added at Task 15:
//
//   12. 5 buffer-related ABICallbacks methods (per Task 4 interface
//       extension). Task 15 STUBS each method — tests pin the stub return so
//       any regression at Task 16 wiring re-fails.
//
//   13. 7 NEW consumer-side dispatch helpers per §5.3 C14-C20 (OnRequest
//       Body / OnResponseBody / OnRequestTrailers / OnResponseTrailers /
//       OnTick / OnHttpCallResponse / OnForeignFunction). Task 15 STUBS each
//       — tests pin the stub return (Continue / no-op) so Task 16 wiring
//       fails the unit tests when the real dispatch lands.
//
//   14. GetProperty extended to delegate via wasm.ResolveProperty +
//       filterPropertyResolver per AMEND-B4. Tests cover the 4 RE-USE
//       primitive integration:
//         (a) ADR-0144 DownstreamPrincipal → connection.subject_peer_-
//             certificate + connection.tls_version round-trip
//         (b) ADR-0177 httpclient stubs return absent at Task 15 (Task 18
//             wires the per-stream upstream-host binding) — tested as
//             NotFound for upstream.address
//         (c) ADR-0190 dynamicmetadata → metadata.<filter>.<key> round-trip
//         (d) NEW ADR-0207 filterstate → filter_state.<key> round-trip
// =============================================================================

// -----------------------------------------------------------------------------
// 12. 5 buffer-related methods (stub behavior).
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetBuffer_StubReturnsNilNil(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	for _, bt := range []abi.WasmBufferType{
		abi.WasmBufferTypeHttpRequestBody,
		abi.WasmBufferTypeHttpResponseBody,
		abi.WasmBufferTypeHttpCallResponseBody,
	} {
		got, err := cb.GetBuffer(context.Background(), 1, bt)
		if err != nil {
			t.Errorf("GetBuffer(bt=%d) err = %v; want nil (stub)", bt, err)
		}
		if got != nil {
			t.Errorf("GetBuffer(bt=%d) = %v; want nil (stub)", bt, got)
		}
	}
}

func TestAbiCallbacks_SetBuffer_StubReturnsUnimplemented(t *testing.T) {
	t.Parallel()
	// Task 16 ACTIVATED SetBuffer (Task 15 stub replaced): SetBuffer on
	// WasmBufferTypeHttpRequestBody splices into f.decodeBody + returns Ok.
	// Test re-pinned at Task 16 activation: returns WasmResultOk on the
	// activated decode-side body splice; HttpCallResponseBody (host-written)
	// returns WasmResultBadArgument.
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	if got := cb.SetBuffer(context.Background(), 1, abi.WasmBufferTypeHttpRequestBody, 0, []byte("data")); got != abi.WasmResultOk {
		t.Errorf("SetBuffer(HttpRequestBody) = %v; want Ok (Task 16 activated)", got)
	}
	if got := cb.SetBuffer(context.Background(), 1, abi.WasmBufferTypeHttpCallResponseBody, 0, []byte("data")); got != abi.WasmResultBadArgument {
		t.Errorf("SetBuffer(HttpCallResponseBody) = %v; want BadArgument (host-written)", got)
	}
}

func TestAbiCallbacks_GetBufferStatus_StubReturnsZeros(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	size, flags, err := cb.GetBufferStatus(context.Background(), 1, abi.WasmBufferTypeHttpRequestBody)
	if err != nil {
		t.Errorf("GetBufferStatus err = %v; want nil (stub)", err)
	}
	if size != 0 || flags != 0 {
		t.Errorf("GetBufferStatus = (%d, %d); want (0, 0) (stub)", size, flags)
	}
}

func TestAbiCallbacks_ContinueStream_StubReturnsUnimplemented(t *testing.T) {
	t.Parallel()
	// Task 16 ACTIVATED ContinueStream (Task 15 stub replaced):
	// streamType=0 (decode) routes to decoderCb.ContinueDecoding; nil cb
	// returns InternalFailure. Test re-pinned at Task 16 activation:
	// with nil decoderCb the activated method returns InternalFailure;
	// streamType > 1 returns BadArgument.
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	if got := cb.ContinueStream(context.Background(), 1, 0); got != abi.WasmResultInternalFailure {
		t.Errorf("ContinueStream(streamType=0, nil decoderCb) = %v; want InternalFailure (Task 16 activated)", got)
	}
	if got := cb.ContinueStream(context.Background(), 1, 99); got != abi.WasmResultBadArgument {
		t.Errorf("ContinueStream(streamType=99) = %v; want BadArgument (Task 16 activated)", got)
	}
}

func TestAbiCallbacks_CloseStream_StubReturnsUnimplemented(t *testing.T) {
	t.Parallel()
	// Task 16 ACTIVATED CloseStream (Task 15 stub replaced):
	// streamType=0 (decode) emits SendLocalReply(503); nil decoderCb
	// returns InternalFailure; streamType=1 (encode) returns Unimplemented
	// (SendLocalReply unavailable on encode side). Test re-pinned at Task
	// 16 activation: with nil decoderCb the streamType=0 path returns
	// InternalFailure; streamType=1 still returns Unimplemented.
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	if got := cb.CloseStream(context.Background(), 1, 0); got != abi.WasmResultInternalFailure {
		t.Errorf("CloseStream(streamType=0, nil decoderCb) = %v; want InternalFailure (Task 16 activated)", got)
	}
	if got := cb.CloseStream(context.Background(), 1, 1); got != abi.WasmResultUnimplemented {
		t.Errorf("CloseStream(streamType=1) = %v; want Unimplemented (encode-side; Task 16 activated)", got)
	}
}

// -----------------------------------------------------------------------------
// 13. 7 NEW consumer-side dispatch helpers — stub behavior.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_OnRequestBody_StubReturnsContinue(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	if got := cb.OnRequestBody(1, 0, false); got != abi.ProxyActionContinue {
		t.Errorf("OnRequestBody empty = %v; want Continue (stub)", got)
	}
	if got := cb.OnRequestBody(1, 100, true); got != abi.ProxyActionContinue {
		t.Errorf("OnRequestBody endOfStream = %v; want Continue (stub)", got)
	}
}

func TestAbiCallbacks_OnRequestBody_NilFilterReturnsContinue(t *testing.T) {
	t.Parallel()
	cb := &abiCallbacks{filter: nil}

	if got := cb.OnRequestBody(1, 0, true); got != abi.ProxyActionContinue {
		t.Errorf("OnRequestBody(nil filter) = %v; want Continue (defensive)", got)
	}
}

func TestAbiCallbacks_OnResponseBody_StubReturnsContinue(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	if got := cb.OnResponseBody(1, 0, false); got != abi.ProxyActionContinue {
		t.Errorf("OnResponseBody = %v; want Continue (stub)", got)
	}
}

func TestAbiCallbacks_OnRequestTrailers_StubReturnsContinue(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	if got := cb.OnRequestTrailers(1, 0); got != abi.ProxyActionContinue {
		t.Errorf("OnRequestTrailers = %v; want Continue (stub)", got)
	}
}

func TestAbiCallbacks_OnResponseTrailers_StubReturnsContinue(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	if got := cb.OnResponseTrailers(1, 0); got != abi.ProxyActionContinue {
		t.Errorf("OnResponseTrailers = %v; want Continue (stub)", got)
	}
}

func TestAbiCallbacks_OnTick_StubNoPanic(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	// MUST NOT panic — Task 16 wires the per-RootVM tick goroutine + the
	// proxy_on_tick dispatch. At Task 15 the helper is a no-op stub.
	cb.OnTick(0)
	cb.OnTick(42)
}

func TestAbiCallbacks_OnHttpCallResponse_StubNoPanic(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	// MUST NOT panic — Task 16 wires the per-call response routing.
	cb.OnHttpCallResponse(1, 0, 0, 0, 0)
	cb.OnHttpCallResponse(1, 42, 3, 1024, 0)
}

func TestAbiCallbacks_OnForeignFunction_StubNoPanic(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(nil, nil, nil, nil)

	// MUST NOT panic — Task 16 wires the per-stream foreign-function
	// response surface.
	cb.OnForeignFunction(1, 0, 0)
	cb.OnForeignFunction(1, 42, 100)
}

func TestAbiCallbacks_OnX_NilFilterNoPanic(t *testing.T) {
	t.Parallel()
	cb := &abiCallbacks{filter: nil}

	// All 7 NEW helpers MUST be nil-tolerant (ADR-0085) — defensive returns
	// preserve the Continue / no-op default for nil-filter test doubles.
	_ = cb.OnRequestBody(1, 0, false)
	_ = cb.OnResponseBody(1, 0, false)
	_ = cb.OnRequestTrailers(1, 0)
	_ = cb.OnResponseTrailers(1, 0)
	cb.OnTick(0)
	cb.OnHttpCallResponse(1, 0, 0, 0, 0)
	cb.OnForeignFunction(1, 0, 0)
}

// -----------------------------------------------------------------------------
// 14. GetProperty — 4 RE-USE primitive integration.
//
// The framework dispatcher at registration.go splits the NUL-delimited path
// into segments and invokes GetProperty(ctx, ctxID, segments). Tests below
// invoke GetProperty directly with the segments slice (matching the
// framework's call shape).
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// 14a. RE-USE #1 — ADR-0144 DownstreamPrincipal for connection.tls.*
// sub-paths. The fake decoder callback returns a fake principal + a fake
// *tls.ConnectionState carrying a Version field; the resolver projects to
// connection.subject_peer_certificate (from DownstreamPrincipal()[0]) and
// connection.tls_version (from DownstreamTLSConnectionState().Version).
// -----------------------------------------------------------------------------

// fakeDecoderCbWithTLS extends fakeDecoderCb with controllable TLS data.
type fakeDecoderCbWithTLS struct {
	fakeDecoderCb
	principal    []string
	tlsState     *tls.ConnectionState
	dynMetadata  *dynamicmetadata.Bucket
	sni          string
	peerCertDER  []byte
	remoteAddr   net.Addr
	localAddr    net.Addr
	protocol     string
	listenerPrin string
}

func (f fakeDecoderCbWithTLS) DownstreamPrincipal() []string { return f.principal }
func (f fakeDecoderCbWithTLS) DownstreamTLSConnectionState() *tls.ConnectionState {
	return f.tlsState
}
func (f fakeDecoderCbWithTLS) DynamicMetadata() *dynamicmetadata.Bucket { return f.dynMetadata }
func (f fakeDecoderCbWithTLS) DownstreamTLSServerName() string          { return f.sni }
func (f fakeDecoderCbWithTLS) DownstreamTLSPeerCertDER() []byte         { return f.peerCertDER }
func (f fakeDecoderCbWithTLS) DownstreamRemoteAddr() net.Addr           { return f.remoteAddr }
func (f fakeDecoderCbWithTLS) DownstreamLocalAddr() net.Addr            { return f.localAddr }
func (f fakeDecoderCbWithTLS) DownstreamProtocol() string               { return f.protocol }
func (f fakeDecoderCbWithTLS) ListenerPrincipal() string                { return f.listenerPrin }

func TestAbiCallbacks_GetProperty_ConnectionTLSVersion_DownstreamPrincipalRoundTrip(t *testing.T) {
	t.Parallel()
	dcb := fakeDecoderCbWithTLS{
		principal: []string{"spiffe://example/peer", "alt-uri"},
		tlsState: &tls.ConnectionState{
			Version: tls.VersionTLS13,
		},
	}
	cb, _ := newTestABICallbacks(http.Header{}, nil, dcb, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"connection", "tls_version"})
	if !ok {
		t.Fatalf("connection.tls_version: ok=false; want true")
	}
	if string(val) != "TLSv1.3" {
		t.Errorf("connection.tls_version = %q; want %q", val, "TLSv1.3")
	}
}

func TestAbiCallbacks_GetProperty_ConnectionSubjectPeerCertificate_DownstreamPrincipalRoundTrip(t *testing.T) {
	t.Parallel()
	dcb := fakeDecoderCbWithTLS{
		principal: []string{"spiffe://example/peer", "alt-uri"},
	}
	cb, _ := newTestABICallbacks(http.Header{}, nil, dcb, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"connection", "subject_peer_certificate"})
	if !ok {
		t.Fatalf("connection.subject_peer_certificate: ok=false; want true")
	}
	if string(val) != "spiffe://example/peer" {
		t.Errorf("connection.subject_peer_certificate = %q; want %q", val, "spiffe://example/peer")
	}
}

func TestAbiCallbacks_GetProperty_ConnectionMTLS_TLSStateNonNil_TrueWithCerts(t *testing.T) {
	t.Parallel()
	// PeerCertificates non-empty → mtls=true. Use a synthetic *x509.Certificate
	// — we don't need real DER for the True branch.
	cert := mustParseSelfSignedCert(t)
	dcb := fakeDecoderCbWithTLS{
		tlsState: &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		},
	}
	cb, _ := newTestABICallbacks(http.Header{}, nil, dcb, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"connection", "mtls"})
	if !ok {
		t.Fatalf("connection.mtls: ok=false; want true")
	}
	if !bytes.Equal(val, []byte{0x01}) {
		t.Errorf("connection.mtls = %x; want 0x01 (true)", val)
	}
}

func TestAbiCallbacks_GetProperty_ConnectionMTLS_NoTLS_FalseButOk(t *testing.T) {
	t.Parallel()
	dcb := fakeDecoderCbWithTLS{tlsState: nil}
	cb, _ := newTestABICallbacks(http.Header{}, nil, dcb, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"connection", "mtls"})
	// Plaintext: mtls=false but the answer IS known → ok=true.
	if !ok {
		t.Fatalf("connection.mtls (plaintext): ok=false; want true (known-false)")
	}
	if !bytes.Equal(val, []byte{0x00}) {
		t.Errorf("connection.mtls (plaintext) = %x; want 0x00 (false)", val)
	}
}

func TestAbiCallbacks_GetProperty_ConnectionSHA256_HexEncoded(t *testing.T) {
	t.Parallel()
	der := []byte("test-cert-der-bytes")
	dcb := fakeDecoderCbWithTLS{peerCertDER: der}
	cb, _ := newTestABICallbacks(http.Header{}, nil, dcb, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"connection", "sha256_peer_certificate_digest"})
	if !ok {
		t.Fatalf("connection.sha256_peer_certificate_digest: ok=false; want true")
	}
	// Recompute expected: sha256(der) hex-encoded.
	sum := sha256.Sum256(der)
	want := hex.EncodeToString(sum[:])
	if string(val) != want {
		t.Errorf("sha256 digest = %q; want %q", val, want)
	}
}

// mustParseSelfSignedCert returns a minimal valid *x509.Certificate for tests
// that need a non-empty PeerCertificates slice. The cert is generated once
// per test — synthesized via crypto/x509 self-signed minimal-fields cert.
func mustParseSelfSignedCert(t *testing.T) *x509.Certificate {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}
	return cert
}

// -----------------------------------------------------------------------------
// 14b. RE-USE #2 — ADR-0177 httpclient for upstream.* sub-paths. At Task 15
// the per-stream upstream-host accessor is NOT on DecoderFilterCallbacks
// (lands at Task 18); the resolver returns absent for upstream.address +
// upstream.port. Tested here as NotFound so Task 18 wiring re-fails the
// test (TDD seam).
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetProperty_UpstreamAddress_StubReturnsNotFound(t *testing.T) {
	t.Parallel()
	dcb := fakeDecoderCbWithTLS{}
	cb, _ := newTestABICallbacks(http.Header{}, nil, dcb, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"upstream", "address"})
	if ok {
		t.Errorf("upstream.address: ok=true val=%q; want false (Task 18 stub)", val)
	}
}

func TestAbiCallbacks_GetProperty_XdsClusterName_StubReturnsNotFound(t *testing.T) {
	t.Parallel()
	dcb := fakeDecoderCbWithTLS{}
	cb, _ := newTestABICallbacks(http.Header{}, nil, dcb, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"xds", "cluster_name"})
	if ok {
		t.Errorf("xds.cluster_name: ok=true val=%q; want false (Task 18 stub)", val)
	}
}

// -----------------------------------------------------------------------------
// 14c. RE-USE #3 — ADR-0190 dynamicmetadata for metadata.<filter>.<key>
// branches. The fake decoder callback returns a *dynamicmetadata.Bucket
// pre-populated with a (filter, key) entry; the resolver projects to
// metadata.<filter>.<key> via the protobuf-marshaled wire bytes.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetProperty_Metadata_DynamicMetadataRoundTrip(t *testing.T) {
	t.Parallel()
	bucket := dynamicmetadata.NewBucket()
	bucket.Set("envoy.foo", "bar", structpb.NewStringValue("baz"))

	dcb := fakeDecoderCbWithTLS{dynMetadata: bucket}
	cb, _ := newTestABICallbacks(http.Header{}, nil, dcb, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"metadata", "envoy.foo", "bar"})
	if !ok {
		t.Fatalf("metadata.envoy.foo.bar: ok=false; want true")
	}
	// Validate by unmarshaling the returned bytes back into a *structpb.Value
	// + checking it decodes to "baz".
	var got structpb.Value
	if err := proto.Unmarshal(val, &got); err != nil {
		t.Fatalf("proto.Unmarshal: %v", err)
	}
	if got.GetStringValue() != "baz" {
		t.Errorf("metadata value = %q; want %q", got.GetStringValue(), "baz")
	}
}

func TestAbiCallbacks_GetProperty_Metadata_AbsentKey_NotFound(t *testing.T) {
	t.Parallel()
	bucket := dynamicmetadata.NewBucket()
	bucket.Set("envoy.foo", "present", structpb.NewStringValue("v"))

	dcb := fakeDecoderCbWithTLS{dynMetadata: bucket}
	cb, _ := newTestABICallbacks(http.Header{}, nil, dcb, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"metadata", "envoy.foo", "absent"})
	if ok {
		t.Errorf("metadata.envoy.foo.absent: ok=true val=%v; want false", val)
	}
}

// -----------------------------------------------------------------------------
// 14d. RE-USE #4 — NEW ADR-0207 filterstate for filter_state.<key> branches.
// The per-stream filterstate.Bucket lives on *filter.downstreamFilterState
// (added at Task 15 alongside the resolver). The resolver round-trips the
// FilterStateObject's Marshal() bytes.
// -----------------------------------------------------------------------------

// testFilterStateObject is a simple FilterStateObject for round-trip tests.
type testFilterStateObject struct {
	payload []byte
	st      filterstate.StateType
}

func (o *testFilterStateObject) Marshal() ([]byte, error) {
	// Defensive copy per the FilterStateObject contract.
	out := make([]byte, len(o.payload))
	copy(out, o.payload)
	return out, nil
}

func (o *testFilterStateObject) Unmarshal(b []byte) error {
	o.payload = append(o.payload[:0], b...)
	return nil
}

func (o *testFilterStateObject) HasData() bool                    { return len(o.payload) > 0 }
func (o *testFilterStateObject) StateType() filterstate.StateType { return o.st }

func TestAbiCallbacks_GetProperty_FilterState_RoundTrip(t *testing.T) {
	t.Parallel()
	bucket := filterstate.NewBucket()
	if err := bucket.Set("my-key", &testFilterStateObject{
		payload: []byte("my-value"),
		st:      filterstate.StateTypeMutable,
	}); err != nil {
		t.Fatalf("bucket.Set: %v", err)
	}

	cb, f := newTestABICallbacks(http.Header{}, nil, fakeDecoderCb{}, nil)
	f.downstreamFilterState = bucket

	val, ok := cb.GetProperty(context.Background(), 1, []string{"filter_state", "my-key"})
	if !ok {
		t.Fatalf("filter_state.my-key: ok=false; want true")
	}
	if string(val) != "my-value" {
		t.Errorf("filter_state.my-key = %q; want %q", val, "my-value")
	}
}

func TestAbiCallbacks_GetProperty_FilterState_AbsentKey_NotFound(t *testing.T) {
	t.Parallel()
	bucket := filterstate.NewBucket()
	if err := bucket.Set("present", &testFilterStateObject{
		payload: []byte("v"),
		st:      filterstate.StateTypeMutable,
	}); err != nil {
		t.Fatalf("bucket.Set: %v", err)
	}

	cb, f := newTestABICallbacks(http.Header{}, nil, fakeDecoderCb{}, nil)
	f.downstreamFilterState = bucket

	_, ok := cb.GetProperty(context.Background(), 1, []string{"filter_state", "absent"})
	if ok {
		t.Errorf("filter_state.absent: ok=true; want false")
	}
}

func TestAbiCallbacks_GetProperty_FilterState_NilBucket_NotFound(t *testing.T) {
	t.Parallel()
	// No bucket wired on *filter → all filter_state.<key> probes return
	// NotFound (matches ADR-0085 nil-tolerance).
	cb, _ := newTestABICallbacks(http.Header{}, nil, fakeDecoderCb{}, nil)

	_, ok := cb.GetProperty(context.Background(), 1, []string{"filter_state", "any-key"})
	if ok {
		t.Errorf("filter_state.any-key (nil bucket): ok=true; want false")
	}
}

func TestAbiCallbacks_GetProperty_UpstreamFilterState_RoundTrip(t *testing.T) {
	t.Parallel()
	// Distinct root per AMEND-B4: upstream_filter_state consumes a separate
	// Bucket from filter_state.
	upstream := filterstate.NewBucket()
	if err := upstream.Set("u-key", &testFilterStateObject{
		payload: []byte("u-value"),
		st:      filterstate.StateTypeReadOnly,
	}); err != nil {
		t.Fatalf("bucket.Set: %v", err)
	}

	cb, f := newTestABICallbacks(http.Header{}, nil, fakeDecoderCb{}, nil)
	f.upstreamFilterState = upstream

	val, ok := cb.GetProperty(context.Background(), 1, []string{"upstream_filter_state", "u-key"})
	if !ok {
		t.Fatalf("upstream_filter_state.u-key: ok=false; want true")
	}
	if string(val) != "u-value" {
		t.Errorf("upstream_filter_state.u-key = %q; want %q", val, "u-value")
	}
}

func TestAbiCallbacks_GetProperty_Wasm_ProxyDownstreamFirst(t *testing.T) {
	t.Parallel()
	// wasm.<key> proxies via downstream filter_state FIRST, then upstream
	// (per AMEND-B4 + cpp-host context.cc:987-1019). Same key in both
	// buckets resolves from downstream.
	downstream := filterstate.NewBucket()
	if err := downstream.Set("shared", &testFilterStateObject{
		payload: []byte("downstream-wins"),
		st:      filterstate.StateTypeMutable,
	}); err != nil {
		t.Fatalf("downstream Set: %v", err)
	}
	upstream := filterstate.NewBucket()
	if err := upstream.Set("shared", &testFilterStateObject{
		payload: []byte("upstream-loses"),
		st:      filterstate.StateTypeMutable,
	}); err != nil {
		t.Fatalf("upstream Set: %v", err)
	}

	cb, f := newTestABICallbacks(http.Header{}, nil, fakeDecoderCb{}, nil)
	f.downstreamFilterState = downstream
	f.upstreamFilterState = upstream

	val, ok := cb.GetProperty(context.Background(), 1, []string{"wasm", "shared"})
	if !ok {
		t.Fatalf("wasm.shared: ok=false; want true")
	}
	if string(val) != "downstream-wins" {
		t.Errorf("wasm.shared = %q; want %q (downstream wins per cpp-host fallthrough)", val, "downstream-wins")
	}
}

func TestAbiCallbacks_GetProperty_Wasm_FallthroughToUpstream(t *testing.T) {
	t.Parallel()
	// Key absent in downstream → fall through to upstream per cpp-host
	// context.cc:987-1019.
	downstream := filterstate.NewBucket()
	upstream := filterstate.NewBucket()
	if err := upstream.Set("only-upstream", &testFilterStateObject{
		payload: []byte("upstream-value"),
		st:      filterstate.StateTypeMutable,
	}); err != nil {
		t.Fatalf("upstream Set: %v", err)
	}

	cb, f := newTestABICallbacks(http.Header{}, nil, fakeDecoderCb{}, nil)
	f.downstreamFilterState = downstream
	f.upstreamFilterState = upstream

	val, ok := cb.GetProperty(context.Background(), 1, []string{"wasm", "only-upstream"})
	if !ok {
		t.Fatalf("wasm.only-upstream: ok=false; want true (fallthrough)")
	}
	if string(val) != "upstream-value" {
		t.Errorf("wasm.only-upstream = %q; want %q", val, "upstream-value")
	}
}

// -----------------------------------------------------------------------------
// 14e. request.* + source.* + destination.* — stream-local accessors verified
// via the existing fakeDecoderCb / request-header surfaces. These confirm
// the property-tree dispatch routes correctly for the non-RE-USE paths.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetProperty_RequestMethod_PseudoHeader(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req[":method"] = []string{"POST"}
	cb, _ := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"request", "method"})
	if !ok {
		t.Fatalf("request.method: ok=false; want true")
	}
	if string(val) != "POST" {
		t.Errorf("request.method = %q; want POST", val)
	}
}

func TestAbiCallbacks_GetProperty_RequestHeader_Named(t *testing.T) {
	t.Parallel()
	req := http.Header{}
	req.Set("X-Custom", "hello")
	cb, _ := newTestABICallbacks(req, nil, fakeDecoderCb{}, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"request", "headers", "X-Custom"})
	if !ok {
		t.Fatalf("request.headers.X-Custom: ok=false; want true")
	}
	if string(val) != "hello" {
		t.Errorf("request.headers.X-Custom = %q; want hello", val)
	}
}

func TestAbiCallbacks_GetProperty_SourceAddress_FromRemoteAddr(t *testing.T) {
	t.Parallel()
	dcb := fakeDecoderCbWithTLS{
		remoteAddr: &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 54321},
	}
	cb, _ := newTestABICallbacks(http.Header{}, nil, dcb, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"source", "address"})
	if !ok {
		t.Fatalf("source.address: ok=false; want true")
	}
	if string(val) != "10.0.0.1" {
		t.Errorf("source.address = %q; want 10.0.0.1", val)
	}

	pval, pok := cb.GetProperty(context.Background(), 1, []string{"source", "port"})
	if !pok {
		t.Fatalf("source.port: ok=false; want true")
	}
	// uint64 → 8-byte LE encoding per property.go.
	if len(pval) != 8 {
		t.Errorf("source.port byte length = %d; want 8", len(pval))
	}
	gotPort := binary.LittleEndian.Uint64(pval)
	if gotPort != 54321 {
		t.Errorf("source.port = %d; want 54321", gotPort)
	}
}

func TestAbiCallbacks_GetProperty_DestinationAddress_FromLocalAddr(t *testing.T) {
	t.Parallel()
	dcb := fakeDecoderCbWithTLS{
		localAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080},
	}
	cb, _ := newTestABICallbacks(http.Header{}, nil, dcb, nil)

	val, ok := cb.GetProperty(context.Background(), 1, []string{"destination", "address"})
	if !ok {
		t.Fatalf("destination.address: ok=false; want true")
	}
	if string(val) != "127.0.0.1" {
		t.Errorf("destination.address = %q; want 127.0.0.1", val)
	}
}

// -----------------------------------------------------------------------------
// 14f. response.code — ADR-0196 second co-consumer (GetStatus is FIRST).
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetProperty_ResponseCode_ADR0196SecondCoConsumer(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(http.Header{}, http.Header{}, fakeDecoderCb{}, fakeEncoderCb{responseStatus: 503})

	val, ok := cb.GetProperty(context.Background(), 1, []string{"response", "code"})
	if !ok {
		t.Fatalf("response.code: ok=false; want true")
	}
	if len(val) != 8 {
		t.Fatalf("response.code byte length = %d; want 8 (uint64 LE)", len(val))
	}
	got := binary.LittleEndian.Uint64(val)
	if got != 503 {
		t.Errorf("response.code = %d; want 503", got)
	}
}

// -----------------------------------------------------------------------------
// 14g. Direct tokens — plugin_name + plugin_root_id; plugin_vm_id stays
// absent at Task 15 (no VM-ID surface).
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetProperty_PluginName_DirectToken(t *testing.T) {
	t.Parallel()
	cb, f := newTestABICallbacks(http.Header{}, nil, fakeDecoderCb{}, nil)
	f.cfg = &compiledConfig{pluginName: "my-plugin"}

	val, ok := cb.GetProperty(context.Background(), 1, []string{"plugin_name"})
	if !ok {
		t.Fatalf("plugin_name: ok=false; want true")
	}
	if string(val) != "my-plugin" {
		t.Errorf("plugin_name = %q; want my-plugin", val)
	}
}

func TestAbiCallbacks_GetProperty_PluginVMID_Absent(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(http.Header{}, nil, fakeDecoderCb{}, nil)

	_, ok := cb.GetProperty(context.Background(), 1, []string{"plugin_vm_id"})
	if ok {
		t.Errorf("plugin_vm_id: ok=true; want false at Task 15 (no VM-ID surface)")
	}
}

// -----------------------------------------------------------------------------
// 14h. Empty path / unknown root — both return NotFound semantics.
// -----------------------------------------------------------------------------

func TestAbiCallbacks_GetProperty_EmptyPath_NotFound(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(http.Header{}, nil, fakeDecoderCb{}, nil)

	_, ok := cb.GetProperty(context.Background(), 1, nil)
	if ok {
		t.Errorf("empty path: ok=true; want false")
	}
	_, ok = cb.GetProperty(context.Background(), 1, []string{})
	if ok {
		t.Errorf("zero-length path: ok=true; want false")
	}
}

func TestAbiCallbacks_GetProperty_UnknownRoot_NotFound(t *testing.T) {
	t.Parallel()
	cb, _ := newTestABICallbacks(http.Header{}, nil, fakeDecoderCb{}, nil)

	_, ok := cb.GetProperty(context.Background(), 1, []string{"unknown-root", "sub"})
	if ok {
		t.Errorf("unknown root: ok=true; want false")
	}
}

// -----------------------------------------------------------------------------
// 14i. Compile-time conformance — filterPropertyResolver satisfies the
// 60-method internalwasm.PropertyResolver interface.
// -----------------------------------------------------------------------------

func TestFilterPropertyResolver_ConformsToPropertyResolver(t *testing.T) {
	t.Parallel()
	var _ internalwasm.PropertyResolver = (*filterPropertyResolver)(nil)
}
