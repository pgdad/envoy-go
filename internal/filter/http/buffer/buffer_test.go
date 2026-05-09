package buffer

import (
	"net/http"
	"strings"
	"testing"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

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

func TestDecodeHeaders_BodiedNonDisabled_StopIteration_EffectiveMaxStored(t *testing.T) {
	f := freshFilter(t, 1024)
	cb := newFakeCallbacks() // perRoute nil → listener fallback
	f.dcb = cb
	headers := newHeaders(map[string]string{":method": "POST", "content-length": "512"})
	status := f.DecodeHeaders(headers, false)
	if status != envoyhttp.StopIteration {
		t.Errorf("expected StopIteration on bodied + non-disabled; got %v", status)
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

func TestDecodeHeaders_BodiedPerRouteOverride_StopIteration_OverrideMaxStored(t *testing.T) {
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
	if status != envoyhttp.StopIteration {
		t.Errorf("expected StopIteration on bodied + override; got %v", status)
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
	if status != envoyhttp.StopIteration {
		t.Errorf("expected StopIteration (no CL fast-fail); got %v", status)
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

// fakeCallbacks implements envoyhttp.DecoderFilterCallbacks for unit tests.
// Mirrors phase 12 csrf_test.go fakeCallbacks pattern (localReplyArgs + fakeCallbacks).
// perRoute is a proto.Message injected by the test to simulate a per-route TPFC;
// typically a *bufferv3.BufferPerRoute. localReplyCount tracks SendLocalReply invocations.
type fakeCallbacks struct {
	perRoute        proto.Message
	localReplyCount int
	localReplyArgs  *struct {
		status  int
		body    string
		headers envoyhttp.OrderedHeaders
	}
}

func newFakeCallbacks() *fakeCallbacks { return &fakeCallbacks{} }

func (c *fakeCallbacks) ContinueDecoding() {}
func (c *fakeCallbacks) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.localReplyCount++
	c.localReplyArgs = &struct {
		status  int
		body    string
		headers envoyhttp.OrderedHeaders
	}{status: status, body: body, headers: headers}
}
func (c *fakeCallbacks) RequestRouteConfig() proto.Message { return c.perRoute }
func (c *fakeCallbacks) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *fakeCallbacks) EncodeHeaders(_ http.Header, _ bool) {}
func (c *fakeCallbacks) EncodeData(_ []byte, _ bool)         {}
func (c *fakeCallbacks) EncodeTrailers(_ http.Header)        {}
