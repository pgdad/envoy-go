package buffer

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"testing"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// --- Group 1: New factory PGV-mirror ---

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("expected error on nil typed_config")
	}
	if !strings.Contains(err.Error(), "buffer:") {
		t.Errorf("error wording missing 'buffer:' prefix: %v", err)
	}
}

func TestNew_MalformedTC(t *testing.T) {
	any := &anypb.Any{TypeUrl: TypeURL, Value: []byte("not-a-valid-proto")}
	_, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("expected error on malformed typed_config")
	}
}

func TestNew_MaxRequestBytesNil_RejectAtParseTime(t *testing.T) {
	cfg := &bufferv3.Buffer{} // MaxRequestBytes nil
	any := mustMarshalAny(t, cfg)
	_, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil || !strings.Contains(err.Error(), "max_request_bytes is required") {
		t.Errorf("expected 'max_request_bytes is required' error, got: %v", err)
	}
}

func TestNew_MaxRequestBytesZero_RejectAtParseTime(t *testing.T) {
	cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(0)}
	any := mustMarshalAny(t, cfg)
	_, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil || !strings.Contains(err.Error(), "must be > 0") {
		t.Errorf("expected 'must be > 0' error, got: %v", err)
	}
}

func TestNew_MaxRequestBytesOverCap_RejectAtParseTime(t *testing.T) {
	cases := []uint32{1048577, 2 * 1024 * 1024, 5 * 1024 * 1024}
	for _, v := range cases {
		v := v
		t.Run("", func(t *testing.T) {
			cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(v)}
			any := mustMarshalAny(t, cfg)
			_, err := New(any, envoyhttp.FactoryCtx{})
			if err == nil || !strings.Contains(err.Error(), "exceeds envoy-go cap of 1048576 bytes") {
				t.Errorf("expected over-cap error for v=%d, got: %v", v, err)
			}
		})
	}
}

func TestNew_MaxRequestBytesBoundary_Accepted(t *testing.T) {
	cases := []uint32{1, 65536, 1048576}
	for _, v := range cases {
		v := v
		t.Run("", func(t *testing.T) {
			cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(v)}
			any := mustMarshalAny(t, cfg)
			factory, err := New(any, envoyhttp.FactoryCtx{})
			if err != nil {
				t.Fatalf("expected accept for v=%d, got error: %v", v, err)
			}
			if factory == nil {
				t.Fatal("expected non-nil factory")
			}
		})
	}
}

func TestNew_HappyPath_Round(t *testing.T) {
	cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(1024)}
	any := mustMarshalAny(t, cfg)
	factory, err := New(any, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	hf := factory()
	if hf.Decoder == nil || hf.Encoder != nil {
		t.Errorf("expected decoder-only HTTPFilter (Decoder!=nil, Encoder==nil), got %+v", hf)
	}
}

// --- Group 2: parsePerRoute PGV-mirror discipline ---

func TestParsePerRoute_Disabled_Parses(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Disabled{Disabled: true}}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if !cpr.disabled || cpr.maxOverride != nil {
		t.Errorf("expected disabled=true, maxOverride=nil; got %+v", cpr)
	}
}

func TestParsePerRoute_BufferOverride_Parses(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Buffer{Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(65536)}}}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if cpr.disabled || cpr.maxOverride == nil || *cpr.maxOverride != 65536 {
		t.Errorf("expected disabled=false, maxOverride=&65536; got %+v", cpr)
	}
}

func TestParsePerRoute_BufferOverride_Zero_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Buffer{Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(0)}}}
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "must be > 0") {
		t.Errorf("expected zero-rejection, got: %v", err)
	}
}

func TestParsePerRoute_BufferOverride_OverCap_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Buffer{Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(5 * 1024 * 1024)}}}
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "exceeds envoy-go cap") {
		t.Errorf("expected over-cap rejection, got: %v", err)
	}
}

func TestParsePerRoute_OneofUnset_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{} // Override nil
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "override oneof is required") {
		t.Errorf("expected oneof-required rejection, got: %v", err)
	}
}

func TestParsePerRoute_DisabledFalse_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Disabled{Disabled: false}}
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "disabled must be true") {
		t.Errorf("expected disabled-bool.const rejection, got: %v", err)
	}
}

// --- Helpers ---

func mustMarshalAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// --- Group 3: DecodeHeaders ---

func TestDecodeHeaders_HeaderOnlyEndStream_Continue(t *testing.T) {
	cases := []string{"GET", "HEAD", "OPTIONS", "POST"} // POST with endStream=true on headers (rare but legal)
	for _, method := range cases {
		method := method
		t.Run(method, func(t *testing.T) {
			f := freshFilter(t, 1024)
			headers := newHeaders(map[string]string{":method": method})
			status := f.DecodeHeaders(headers, true) // endStream=true on headers
			if status != envoyhttp.Continue {
				t.Errorf("expected Continue on header-only %s; got %v", method, status)
			}
			if f.passthrough || f.headersRef != nil || f.effectiveMax != 0 {
				t.Errorf("expected zero state touch on header-only path; got passthrough=%v, headersRef=%v, effectiveMax=%d", f.passthrough, f.headersRef, f.effectiveMax)
			}
		})
	}
}

func TestDecodeHeaders_PerRouteDisabled_Continue_PassthroughSet(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks()
	cb.perRoute = &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Disabled{Disabled: true}}
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST"})
	status := f.DecodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue on per-route disabled; got %v", status)
	}
	if !f.passthrough {
		t.Error("expected passthrough flag set")
	}
}

func TestDecodeHeaders_BodiedNonDisabled_Continue_EffectiveMaxStored(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks() // perRoute nil → listener fallback
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST", "content-length": "512"})
	status := f.DecodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue on bodied + non-disabled (ADR-0127 v2 synchronous HCM); got %v", status)
	}
	if f.passthrough {
		t.Error("expected passthrough flag NOT set")
	}
	if f.effectiveMax != 1024 {
		t.Errorf("expected effectiveMax=1024 (listener fallback); got %d", f.effectiveMax)
	}
	if f.headersRef == nil {
		t.Error("expected headersRef stored")
	}
}

func TestDecodeHeaders_BodiedPerRouteOverride_Continue_OverrideMaxStored(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks()
	cb.perRoute = &bufferv3.BufferPerRoute{
		Override: &bufferv3.BufferPerRoute_Buffer{
			Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(256)},
		},
	}
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST", "content-length": "512"})
	status := f.DecodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue on bodied + override (ADR-0127 v2 synchronous HCM); got %v", status)
	}
	if f.effectiveMax != 256 {
		t.Errorf("expected effectiveMax=256 (override wins); got %d", f.effectiveMax)
	}
}

func TestDecodeHeaders_DoesNotInspectContentLength(t *testing.T) {
	// §11.6 — even with absurd CL header, DecodeHeaders does NOT fast-fail.
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks()
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST", "content-length": "99999999999"})
	status := f.DecodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue (no CL fast-fail; ADR-0127 v2 synchronous HCM); got %v", status)
	}
	// SendLocalReply MUST NOT have been invoked.
	if cb.localReplyCount != 0 {
		t.Errorf("expected zero SendLocalReply calls in DecodeHeaders; got %d", cb.localReplyCount)
	}
}

// freshFilter constructs a minimal *filter with only the config field set, for
// DecodeHeaders + resolveEffective unit tests that don't need a full factory build.
func freshFilter(t *testing.T, maxBytes uint32) *filter {
	t.Helper()
	return &filter{config: &compiledConfig{maxRequestBytes: maxBytes}}
}

// newHeaders builds an http.Header from a map of lowercase key→value pairs.
// Mirrors phase 12 csrf_test.go newPostHeaders pattern (adapts for arbitrary headers).
func newHeaders(kv map[string]string) http.Header {
	h := make(http.Header)
	for k, v := range kv {
		h.Set(k, v)
	}
	return h
}

// localReplyRecord captures a single SendLocalReply call's arguments.
// hasConnectionClose checks whether the 413 wire shape includes Connection: close.
type localReplyRecord struct {
	status  int
	body    string
	headers envoyhttp.OrderedHeaders
}

func (r *localReplyRecord) hasConnectionClose() bool {
	return r.headers.Get("Connection") == "close"
}

// fakeCallbacks implements envoyhttp.DecoderFilterCallbacks for unit tests.
// Mirrors phase 12 csrf_test.go fakeCallbacks pattern (localReplyArgs + fakeCallbacks).
// perRoute is a proto.Message injected by the test to simulate a per-route TPFC;
// always a *bufferv3.BufferPerRoute. localReplyCount tracks SendLocalReply invocations.
// resolveCount tracks RequestRouteConfig() call count (for TestPerRoute_ResolveCalledOncePerStream).
type fakeCallbacks struct {
	perRoute        proto.Message
	localReplyCount int
	resolveCount    int
	localReplyArgs  *localReplyRecord
}

func newFakeCallbacks() *fakeCallbacks { return &fakeCallbacks{} }

func (c *fakeCallbacks) ContinueDecoding() {}
func (c *fakeCallbacks) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.localReplyCount++
	c.localReplyArgs = &localReplyRecord{status: status, body: body, headers: headers}
}
func (c *fakeCallbacks) RequestRouteConfig() proto.Message {
	c.resolveCount++
	return c.perRoute
}
func (c *fakeCallbacks) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *fakeCallbacks) EncodeHeaders(_ http.Header, _ bool) {}
func (c *fakeCallbacks) EncodeData(_ []byte, _ bool)         {}
func (c *fakeCallbacks) EncodeTrailers(_ http.Header)        {}
func (c *fakeCallbacks) DownstreamPrincipal() []string       { return nil }

// ADR-0165 callback-surface extension stubs (phase-18.2 Task 4).
func (c *fakeCallbacks) DownstreamRemoteAddr() net.Addr   { return nil }
func (c *fakeCallbacks) DownstreamLocalAddr() net.Addr    { return nil }
func (c *fakeCallbacks) DownstreamTLSServerName() string  { return "" }
func (c *fakeCallbacks) DownstreamTLSPeerCertDER() []byte { return nil }
func (c *fakeCallbacks) DownstreamProtocol() string       { return "" }
func (c *fakeCallbacks) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5).
func (c *fakeCallbacks) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (c *fakeCallbacks) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// newBuffer constructs a []byte body chunk for DecodeData tests. nil → empty body (§11.11).
func newBuffer(b []byte) []byte {
	return b
}

// --- Group 4: DecodeData accumulation + cap predicate ---

func TestDecodeData_PassthroughFlag_DataContinue(t *testing.T) {
	f := freshFilter(t, 1024)
	f.passthrough = true
	for i := 0; i < 5; i++ {
		status := f.DecodeData(newBuffer(make([]byte, 4096)), false)
		if status != envoyhttp.DataContinue {
			t.Errorf("expected DataContinue per chunk on passthrough; got %v", status)
		}
	}
}

func TestDecodeData_SingleChunkFits_EndStream_DataContinue(t *testing.T) {
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "512"})
	status := f.DecodeData(newBuffer(make([]byte, 512)), true) // endStream=true; fits
	if status != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue on single fit chunk; got %v", status)
	}
}

func TestDecodeData_SingleChunkExactCap_EndStream_DataContinue(t *testing.T) {
	// §11.2 — predicate is `>` strict; accumulated == effectiveMax must NOT trip.
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "1024"})
	status := f.DecodeData(newBuffer(make([]byte, 1024)), true)
	if status != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue on exact-cap fit; got %v", status)
	}
}

func TestDecodeData_SingleChunkOverflow_413_StopIterationNoBuffer(t *testing.T) {
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "2048"})
	cb := newFakeCallbacks()
	f.dcb = cb
	status := f.DecodeData(newBuffer(make([]byte, 2048)), false)
	if status != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("expected DataStopIterationNoBuffer on overflow; got %v", status)
	}
	if cb.localReplyCount != 1 {
		t.Fatalf("expected 1 SendLocalReply call; got %d", cb.localReplyCount)
	}
	if cb.localReplyArgs.status != 413 {
		t.Errorf("expected status 413; got %d", cb.localReplyArgs.status)
	}
	if cb.localReplyArgs.body != "Payload Too Large" {
		t.Errorf("expected body 'Payload Too Large'; got %q", cb.localReplyArgs.body)
	}
	if !cb.localReplyArgs.hasConnectionClose() {
		t.Error("expected Connection: close header")
	}
}

func TestDecodeData_MultiChunkBelowCap_DataContinue_TerminalContinue(t *testing.T) {
	// ADR-0127 v2: envoy-go's synchronous HCM dispatch means DataContinue for
	// in-flight chunks (HCM already buffers all body bytes in bodyBuf before
	// rf.RunAction dials upstream; DataStopIterationAndBuffer is unnecessary and
	// would deadlock if DecodeHeaders returned StopIteration).
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "512"})
	// Chunks A=200, B=200, terminal C=112; total=512 < cap.
	if got := f.DecodeData(newBuffer(make([]byte, 200)), false); got != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue on chunk A; got %v", got)
	}
	if got := f.DecodeData(newBuffer(make([]byte, 200)), false); got != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue on chunk B; got %v", got)
	}
	if got := f.DecodeData(newBuffer(make([]byte, 112)), true); got != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue on terminal chunk; got %v", got)
	}
	if f.accumulated != 512 {
		t.Errorf("expected accumulated=512; got %d", f.accumulated)
	}
}

func TestDecodeData_MultiChunkOverflowMidStream_413(t *testing.T) {
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "2048"})
	cb := newFakeCallbacks()
	f.dcb = cb
	if got := f.DecodeData(newBuffer(make([]byte, 800)), false); got != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue on chunk 1 (under cap; ADR-0127 v2); got %v", got)
	}
	// Second chunk pushes accumulated past cap.
	if got := f.DecodeData(newBuffer(make([]byte, 400)), false); got != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("expected DataStopIterationNoBuffer on overflow; got %v", got)
	}
	if cb.localReplyCount != 1 {
		t.Errorf("expected 1 SendLocalReply call; got %d", cb.localReplyCount)
	}
}

func TestDecodeData_EmptyTerminalChunk_DataContinue(t *testing.T) {
	// §11.11 empty-body POST disposition.
	f := freshFilter(t, 1024)
	f.effectiveMax = 1024
	f.headersRef = newHeaders(map[string]string{"content-length": "0"})
	status := f.DecodeData(newBuffer(nil), true)
	if status != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue on empty terminal; got %v", status)
	}
}

// --- Group 5: maybeAddContentLength mirror ---

func TestMaybeAddContentLength_NoOriginalCL_InjectsCL_DropsTransferEncoding(t *testing.T) {
	f := freshFilter(t, 1024)
	f.headersRef = newHeaders(map[string]string{"transfer-encoding": "chunked"})
	f.accumulated = 10240
	f.maybeAddContentLength()
	if got := f.headersRef.Get("Content-Length"); got != "10240" {
		t.Errorf("expected content-length=10240; got %q", got)
	}
	if got := f.headersRef.Get("Transfer-Encoding"); got != "" {
		t.Errorf("expected transfer-encoding dropped; got %q", got)
	}
}

func TestMaybeAddContentLength_OriginalCLPresent_NoOp(t *testing.T) {
	f := freshFilter(t, 1024)
	f.headersRef = newHeaders(map[string]string{"content-length": "512"})
	f.accumulated = 512
	f.maybeAddContentLength()
	if got := f.headersRef.Get("Content-Length"); got != "512" {
		t.Errorf("expected content-length unchanged at 512; got %q", got)
	}
}

func TestMaybeAddContentLength_HeadersRefNil_NoOp(t *testing.T) {
	f := freshFilter(t, 1024)
	f.headersRef = nil // disabled or header-only paths leave headersRef unset
	f.accumulated = 1024
	f.maybeAddContentLength() // must not panic
}

func TestMaybeAddContentLength_Idempotent(t *testing.T) {
	f := freshFilter(t, 1024)
	f.headersRef = newHeaders(map[string]string{"transfer-encoding": "chunked"})
	f.accumulated = 10240
	f.maybeAddContentLength()
	f.maybeAddContentLength() // second call: original-CL is now present (just-injected); no double-injection.
	if got := f.headersRef.Get("Content-Length"); got != "10240" {
		t.Errorf("expected content-length=10240 idempotent; got %q", got)
	}
}

// --- Group 6: Per-route integration ---

func TestPerRoute_ListenerFallback_AppliesWhenPerRouteNil(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks() // perRoute nil
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST", "content-length": "512"})
	f.DecodeHeaders(headers, false)
	if f.effectiveMax != 1024 {
		t.Errorf("expected listener fallback effectiveMax=1024; got %d", f.effectiveMax)
	}
}

func TestPerRoute_OverrideSmaller_FiresAtSmallerCap(t *testing.T) {
	f := freshFilter(t, 1024)
	override := uint32(256)
	cb := newFakeCallbacks()
	cb.perRoute = &bufferv3.BufferPerRoute{
		Override: &bufferv3.BufferPerRoute_Buffer{
			Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(override)},
		},
	}
	f.dcb = cb
	f.DecodeHeaders(newHeaders(map[string]string{":method": "POST", "content-length": "512"}), false)
	// Now drive 300 bytes past the 256-byte cap.
	status := f.DecodeData(newBuffer(make([]byte, 300)), false)
	if status != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("expected 413 at 300 bytes vs 256 cap; got %v", status)
	}
}

func TestPerRoute_OverrideLarger_FiresAtLargerCap(t *testing.T) {
	f := freshFilter(t, 256)
	override := uint32(1024)
	cb := newFakeCallbacks()
	cb.perRoute = &bufferv3.BufferPerRoute{
		Override: &bufferv3.BufferPerRoute_Buffer{
			Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(override)},
		},
	}
	f.dcb = cb
	f.DecodeHeaders(newHeaders(map[string]string{":method": "POST", "content-length": "768"}), false)
	// Listener cap 256 would have fired at 256; override raises to 1024 → 768 fits.
	status := f.DecodeData(newBuffer(make([]byte, 768)), true)
	if status != envoyhttp.DataContinue {
		t.Errorf("expected DataContinue at 768 vs 1024 override cap; got %v", status)
	}
}

func TestPerRoute_DisabledBypassesCap(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks()
	cb.perRoute = &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Disabled{Disabled: true}}
	f.dcb = cb
	f.DecodeHeaders(newHeaders(map[string]string{":method": "POST"}), false)
	if !f.passthrough {
		t.Fatal("expected passthrough flag set")
	}
	// Drive 8 MiB through DecodeData; passthrough returns DataContinue per chunk.
	for i := 0; i < 8; i++ {
		status := f.DecodeData(newBuffer(make([]byte, 1024*1024)), false)
		if status != envoyhttp.DataContinue {
			t.Errorf("expected DataContinue per MiB on disabled-route; got %v at chunk %d", status, i)
		}
	}
	if cb.localReplyCount != 0 {
		t.Errorf("expected zero SendLocalReply on disabled-route; got %d", cb.localReplyCount)
	}
}

func TestPerRoute_ResolveCalledOncePerStream(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks()
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST", "content-length": "512"})
	f.DecodeHeaders(headers, false)
	f.DecodeData(newBuffer(make([]byte, 100)), false)
	f.DecodeData(newBuffer(make([]byte, 100)), false)
	f.DecodeData(newBuffer(make([]byte, 312)), true)
	if cb.resolveCount != 1 {
		t.Errorf("expected exactly 1 RequestRouteConfig.Resolve call; got %d", cb.resolveCount)
	}
}
