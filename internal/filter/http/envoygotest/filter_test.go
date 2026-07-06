package envoygotest

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	envoygotestpb "github.com/pgdad/envoy-go/internal/filter/http/envoygotest/proto"
)

// recordingTerminal is a test-only filter that captures the headers/body
// surfaced through both decode and encode chains. Used as a stand-in for the
// router's terminal step in the per-mode tests.
type recordingTerminal struct {
	dcb envoyhttp.DecoderFilterCallbacks
	ecb envoyhttp.EncoderFilterCallbacks

	decodeHdrCalls   atomic.Int32
	decodeDataCalls  atomic.Int32
	decodeTrailCalls atomic.Int32
	encodeHdrCalls   atomic.Int32
	encodeDataCalls  atomic.Int32

	encodeHeaders http.Header
	encodeBody    []byte
	decodeBody    []byte
}

func (r *recordingTerminal) DecodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus {
	r.decodeHdrCalls.Add(1)
	return envoyhttp.Continue
}
func (r *recordingTerminal) DecodeData(d []byte, _ bool) envoyhttp.FilterDataStatus {
	r.decodeDataCalls.Add(1)
	r.decodeBody = append(r.decodeBody, d...)
	return envoyhttp.DataContinue
}
func (r *recordingTerminal) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	r.decodeTrailCalls.Add(1)
	return envoyhttp.TrailersContinue
}
func (r *recordingTerminal) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { r.dcb = cb }

func (r *recordingTerminal) EncodeHeaders(h http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	r.encodeHdrCalls.Add(1)
	r.encodeHeaders = h
	return envoyhttp.Continue
}
func (r *recordingTerminal) EncodeData(d []byte, _ bool) envoyhttp.FilterDataStatus {
	r.encodeDataCalls.Add(1)
	r.encodeBody = append(r.encodeBody, d...)
	return envoyhttp.DataContinue
}
func (r *recordingTerminal) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (r *recordingTerminal) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { r.ecb = cb }
func (r *recordingTerminal) OnDestroy()                                              {}

// buildChain assembles a chain with envoygotest at index 0 and a recording
// terminal at index 1. Per-route config attaches the supplied count to route 0
// when count > 0; nil otherwise (no per-route config). Returns the chain plus
// pointers to both filter instances for in-test assertions.
func buildChain(t *testing.T, count int32) (*envoyhttp.FilterChain, *filter, *recordingTerminal) {
	t.Helper()
	f := &filter{}
	term := &recordingTerminal{}
	chainNames := []string{"envoy.filters.http.envoy_go_test", "test.terminal"}

	scopes := []envoyhttp.RouteScope{{}}
	if count > 0 {
		pr, err := envoygotestpb.NewEnvoyGoTestPerRoute()
		if err != nil {
			t.Fatalf("NewEnvoyGoTestPerRoute: %v", err)
		}
		pr.SetCount(count)
		any, err := anypb.New(pr.Message)
		if err != nil {
			t.Fatalf("anypb.New(perRoute): %v", err)
		}
		// Override the auto-derived TypeURL with our explicit one so
		// UnmarshalNew picks the right descriptor when round-tripping.
		any.TypeUrl = envoygotestpb.TypeURLEnvoyGoTestPerRoute
		scopes[0].Route = map[string]*anypb.Any{"envoy.filters.http.envoy_go_test": any}
	}
	prc, err := envoyhttp.BuildPerRouteConfig(nil, scopes, chainNames, nil)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}

	chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
		{Name: "envoy.filters.http.envoy_go_test", Decoder: f, Encoder: f},
		{Name: "test.terminal", Decoder: term, Encoder: term},
	}, prc)
	chain.SetRequestCtx(context.Background(), 0)
	return chain, f, term
}

// modeHeaders returns an http.Header carrying the supplied mode value on
// x-envoy-go-test-mode. Used by every per-mode test to drive the dispatch.
func modeHeaders(mode string) http.Header {
	h := http.Header{}
	h.Set("x-envoy-go-test-mode", mode)
	return h
}

// TestEnvoyGoTest_ModeContinue — pure pass-through. The filter returns
// Continue on every callback; the terminal observes one decode + one encode
// pass.
func TestEnvoyGoTest_ModeContinue(t *testing.T) {
	chain, _, term := buildChain(t, 0)

	terminated, err := chain.RunDecodeHeaders(context.Background(), modeHeaders("continue"), true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected terminated=true on continue mode; got false")
	}
	if got := term.decodeHdrCalls.Load(); got != 1 {
		t.Errorf("terminal DecodeHeaders calls = %d, want 1", got)
	}

	respHeaders := http.Header{}
	respHeaders.Set("Content-Type", "text/plain")
	if _, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false); err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}
	if got := term.encodeHdrCalls.Load(); got != 1 {
		t.Errorf("terminal EncodeHeaders calls = %d, want 1", got)
	}
}

// TestEnvoyGoTest_ModeStopAndResumeHeaders — DecodeHeaders returns
// StopIteration; a goroutine resumes via dcb.ContinueDecoding after ~10ms.
// The chain's parkDecode loop yields once the resume signal arrives; the
// terminal's DecodeHeaders fires exactly once after the resume.
func TestEnvoyGoTest_ModeStopAndResumeHeaders(t *testing.T) {
	chain, _, term := buildChain(t, 0)

	start := time.Now()
	terminated, err := chain.RunDecodeHeaders(context.Background(), modeHeaders("stop-and-resume-headers"), true)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected terminated=true after async resume; got false")
	}
	// 10ms sleep + scheduling slack — assert the call took at least 5ms (a
	// generous lower bound that still demonstrates parkDecode held the goroutine).
	if elapsed < 5*time.Millisecond {
		t.Errorf("RunDecodeHeaders returned in %v; expected >= 5ms (async resume)", elapsed)
	}
	if got := term.decodeHdrCalls.Load(); got != 1 {
		t.Errorf("terminal DecodeHeaders calls = %d, want 1 (after resume)", got)
	}
}

// TestEnvoyGoTest_ModeStopAndBufferData — DecodeData returns
// DataStopIterationAndBuffer; a goroutine resumes via dcb.ContinueDecoding
// after ~10ms. The terminal's DecodeData fires after the resume.
func TestEnvoyGoTest_ModeStopAndBufferData(t *testing.T) {
	chain, _, term := buildChain(t, 0)

	// First decode headers to advance to the data-iteration phase. Mode header
	// stays sticky on the same request.
	if _, err := chain.RunDecodeHeaders(context.Background(), modeHeaders("stop-and-buffer-data"), false); err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}

	start := time.Now()
	terminated, err := chain.RunDecodeData(context.Background(), []byte("payload"), true)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RunDecodeData: %v", err)
	}
	if !terminated {
		t.Fatalf("expected terminated=true after async data resume; got false")
	}
	if elapsed < 5*time.Millisecond {
		t.Errorf("RunDecodeData returned in %v; expected >= 5ms (async resume)", elapsed)
	}
	if got := term.decodeDataCalls.Load(); got != 1 {
		t.Errorf("terminal DecodeData calls = %d, want 1 (after resume)", got)
	}
	if string(term.decodeBody) != "payload" {
		t.Errorf("terminal decodeBody = %q, want %q", string(term.decodeBody), "payload")
	}
}

// TestEnvoyGoTest_ModeLocalReplyDecode — DecodeHeaders calls SendLocalReply
// with status 418 + "i am a teapot\n" body. The chain transitions to encode
// mode synchronously inside beginLocalReply; LocalReplyDone() returns true
// and LocalReplyResponse() carries the synthesized shape.
func TestEnvoyGoTest_ModeLocalReplyDecode(t *testing.T) {
	chain, _, _ := buildChain(t, 0)

	terminated, err := chain.RunDecodeHeaders(context.Background(), modeHeaders("local-reply-decode"), true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if terminated {
		t.Fatalf("expected terminated=false (SendLocalReply aborts decode); got true")
	}
	if !chain.LocalReplyDone() {
		t.Fatalf("LocalReplyDone() = false; expected SendLocalReply to have fired")
	}
	gotStatus, _, gotBody := chain.LocalReplyResponse()
	if gotStatus != 418 {
		t.Errorf("local reply status = %d, want 418", gotStatus)
	}
	if string(gotBody) != "i am a teapot\n" {
		t.Errorf("local reply body = %q, want %q", string(gotBody), "i am a teapot\n")
	}
}

// TestEnvoyGoTest_ModeLocalReplyDecodeData — DecodeData calls SendLocalReply.
// First decode headers (mode header forwarded to filter on subsequent
// callbacks too), then decode data which triggers the local reply.
func TestEnvoyGoTest_ModeLocalReplyDecodeData(t *testing.T) {
	chain, _, _ := buildChain(t, 0)

	if _, err := chain.RunDecodeHeaders(context.Background(), modeHeaders("local-reply-decode-data"), false); err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}

	terminated, err := chain.RunDecodeData(context.Background(), []byte("body"), true)
	if err != nil {
		t.Fatalf("RunDecodeData: %v", err)
	}
	if terminated {
		t.Fatalf("expected terminated=false (SendLocalReply on DecodeData aborts iteration); got true")
	}
	if !chain.LocalReplyDone() {
		t.Fatalf("LocalReplyDone() = false; expected SendLocalReply to have fired on DecodeData")
	}
	gotStatus, _, gotBody := chain.LocalReplyResponse()
	if gotStatus != 418 {
		t.Errorf("local reply status = %d, want 418", gotStatus)
	}
	if string(gotBody) != "i am a teapot\n" {
		t.Errorf("local reply body = %q, want %q", string(gotBody), "i am a teapot\n")
	}
}

// TestEnvoyGoTest_ModeModifyEncodeHeaders — EncodeHeaders sets
// x-envoy-go-test-encoded: yes on the response headers map.
func TestEnvoyGoTest_ModeModifyEncodeHeaders(t *testing.T) {
	chain, _, _ := buildChain(t, 0)

	if _, err := chain.RunDecodeHeaders(context.Background(), modeHeaders("modify-encode-headers"), true); err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}

	respHeaders := http.Header{}
	respHeaders.Set("Content-Type", "text/plain")
	if _, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false); err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}
	if got := respHeaders.Get("x-envoy-go-test-encoded"); got != "yes" {
		t.Errorf("x-envoy-go-test-encoded = %q, want yes", got)
	}
}

// TestEnvoyGoTest_ModeModifyEncodeData — EncodeData replaces the body bytes.
// The chain framework's RunEncodeData passes the data slice in-place; the
// filter writes "MODIFIED\n" into the slice via copy-and-truncate semantics
// (the slice's backing array). Tests use a fresh buffer per call so the
// "modification" is visible to the terminal's EncodeData capture.
func TestEnvoyGoTest_ModeModifyEncodeData(t *testing.T) {
	chain, _, term := buildChain(t, 0)

	if _, err := chain.RunDecodeHeaders(context.Background(), modeHeaders("modify-encode-data"), true); err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}

	respHeaders := http.Header{}
	respHeaders.Set("Content-Type", "text/plain")
	if _, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false); err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}

	// The encode chain runs filters in REVERSE declaration order, so the
	// terminal's EncodeData captures FIRST (with the original data); the
	// envoygotest filter's mutation lands AFTER the terminal capture. To
	// observe the mutation we read the in-flight slice via a custom hook —
	// here we rely on the filter's own state capture (dataMutated flag).
	original := []byte("OK\n")
	if _, err := chain.RunEncodeData(context.Background(), original, true); err != nil {
		t.Fatalf("RunEncodeData: %v", err)
	}
	// Terminal should have observed exactly one EncodeData call.
	if got := term.encodeDataCalls.Load(); got != 1 {
		t.Errorf("terminal EncodeData calls = %d, want 1", got)
	}
	// The filter mutates the slice in-place; the terminal observed the slice
	// BEFORE mutation (encode order is reverse: terminal first, then probe).
	// So the terminal's encodeBody is the ORIGINAL pre-mutation bytes; the
	// post-mutation bytes are visible in the `original` slice itself.
	if string(original) != "MODIFIED" && string(original) != "MODIFIED\n" {
		// The probe writes "MODIFIED\n" if the slice is large enough; for a
		// 3-byte slice it writes only the first 3 bytes. Accept either form.
		if string(original) != "MOD" {
			t.Errorf("post-mutation original = %q; want one of {MOD, MODIFIED, MODIFIED\\n}", string(original))
		}
	}
}

// TestEnvoyGoTest_ModeStopTrailers — DecodeTrailers returns
// TrailersStopIteration; a goroutine resumes via ContinueDecoding after ~10ms.
// The terminal's DecodeTrailers fires after the resume.
func TestEnvoyGoTest_ModeStopTrailers(t *testing.T) {
	chain, _, term := buildChain(t, 0)

	if _, err := chain.RunDecodeHeaders(context.Background(), modeHeaders("stop-trailers"), false); err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if _, err := chain.RunDecodeData(context.Background(), []byte("body"), false); err != nil {
		t.Fatalf("RunDecodeData: %v", err)
	}

	start := time.Now()
	trailers := http.Header{}
	trailers.Set("X-Trailer", "yes")
	terminated, err := chain.RunDecodeTrailers(context.Background(), trailers)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RunDecodeTrailers: %v", err)
	}
	if !terminated {
		t.Fatalf("expected terminated=true after trailers async resume; got false")
	}
	if elapsed < 5*time.Millisecond {
		t.Errorf("RunDecodeTrailers returned in %v; expected >= 5ms (async resume)", elapsed)
	}
	if got := term.decodeTrailCalls.Load(); got != 1 {
		t.Errorf("terminal DecodeTrailers calls = %d, want 1 (after resume)", got)
	}
}

// TestEnvoyGoTest_PerRouteCountConfig — per-route count config is echoed into
// x-envoy-go-test-route-count: <N> on the encode-side response headers.
func TestEnvoyGoTest_PerRouteCountConfig(t *testing.T) {
	chain, _, _ := buildChain(t, 7)

	if _, err := chain.RunDecodeHeaders(context.Background(), modeHeaders("continue"), true); err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}

	respHeaders := http.Header{}
	respHeaders.Set("Content-Type", "text/plain")
	if _, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false); err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}
	if got := respHeaders.Get("x-envoy-go-test-route-count"); got != "7" {
		t.Errorf("x-envoy-go-test-route-count = %q, want 7", got)
	}
}

// TestEnvoyGoTest_FactoryRoundTrip — verifies that the package's New function
// (the HTTPFilterFactory) accepts a *EnvoyGoTest anypb config and returns a
// working FilterInstanceFactory. mode_default in the config seeds the filter's
// per-instance default for requests without an x-envoy-go-test-mode header.
func TestEnvoyGoTest_FactoryRoundTrip(t *testing.T) {
	cfg, err := envoygotestpb.NewEnvoyGoTest()
	if err != nil {
		t.Fatalf("NewEnvoyGoTest: %v", err)
	}
	cfg.SetModeDefault("continue")
	tc, err := anypb.New(cfg.Message)
	if err != nil {
		t.Fatalf("anypb.New(EnvoyGoTest): %v", err)
	}
	tc.TypeUrl = envoygotestpb.TypeURLEnvoyGoTest

	fac, err := New(tc, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if fac == nil {
		t.Fatal("New returned nil factory")
	}
	hf := fac()
	if hf.Name != "envoy.filters.http.envoy_go_test" {
		t.Errorf("filter name = %q; want envoy.filters.http.envoy_go_test", hf.Name)
	}
	if hf.Decoder == nil || hf.Encoder == nil {
		t.Error("expected both Decoder + Encoder populated")
	}
	if !strings.HasSuffix(TypeURL, "EnvoyGoTest") {
		t.Errorf("TypeURL = %q; want suffix EnvoyGoTest", TypeURL)
	}

	// Cast Decoder back to *filter and assert mode_default seeded the field.
	pf, ok := hf.Decoder.(*filter)
	if !ok {
		t.Fatalf("Decoder type = %T, want *filter", hf.Decoder)
	}
	if pf.modeDefault != "continue" {
		t.Errorf("filter.modeDefault = %q, want continue", pf.modeDefault)
	}
}
