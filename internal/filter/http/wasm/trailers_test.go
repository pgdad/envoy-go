package wasm

// trailers_test.go — Task 16 unit tests for trailers.go DecodeTrailers +
// EncodeTrailers per 25.2 SPEC §4.3 + §5.3 C16 + C17.
//
// Test surface (per PLAN Task 16 + acceptance criteria):
//
//   1. TestTrailers_DecodeTrailers_NoOpWhenGuestNotExported — streamCtx is
//      nil OR HasGlobalFunc returns false → TrailersContinue without
//      dispatch attempt.
//
//   2. TestTrailers_DecodeTrailers_NilCfg_PassesThrough — defensive nil-cfg
//      pass-through.
//
//   3. TestTrailers_EncodeTrailers_NoOpWhenGuestNotExported — encode-side
//      no-op when guest didn't opt in.
//
//   4. TestTrailers_NumTrailers_MultiValueExpansion — numTrailers wire arg
//      is the TOTAL value count (multi-value trailers expand per §5.3 C16).
//
//   5. TestTrailers_DecodeTrailers_NilDecoderCb_GracefulDegrade — nil
//      decoderCb does not panic on the captured-local-response path.

import (
	gohttp "net/http"
	"testing"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

// newTrailersTestCompiledConfig is identical to newBodyTestCompiledConfig
// for trailers tests — the cap field is unused on the trailer path but the
// stats wiring + pluginName threading match.
func newTrailersTestCompiledConfig(t *testing.T, pluginName string, reg *stats.Registry) *compiledConfig {
	t.Helper()
	return &compiledConfig{
		pluginName:         pluginName,
		bodyBufferCapBytes: 1 << 20, // 1 MiB cap; unused on trailer path
		stats:              newFilterStats(reg, pluginName),
	}
}

// -----------------------------------------------------------------------------
// 1. NO-op when guest did not export proxy_on_request_trailers.
// -----------------------------------------------------------------------------

// TestTrailers_DecodeTrailers_NoOpWhenGuestNotExported asserts that with
// streamCtx == nil (guest didn't construct one), DecodeTrailers returns
// TrailersContinue without an attempted dispatch.
func TestTrailers_DecodeTrailers_NoOpWhenGuestNotExported(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTrailersTestCompiledConfig(t, "plugin_trailers_noop", reg)
	f := &filter{cfg: cc} // streamCtx nil

	trailers := gohttp.Header{}
	trailers.Set("X-Trailer-1", "value1")
	trailers.Set("X-Trailer-2", "value2")

	if got := f.DecodeTrailers(trailers); got != envoyhttp.TrailersContinue {
		t.Errorf("DecodeTrailers (no streamCtx) = %v; want TrailersContinue", got)
	}

	// No failures bumped on the NO-op path.
	if got := findStatCounterValue(reg, "wasm.plugin_trailers_noop.envoy_go.failures"); got != 0 {
		t.Errorf("envoy_go.failures = %d; want 0 (NO-op should not bump failures)", got)
	}
}

// -----------------------------------------------------------------------------
// 2. Nil cfg defensive pass-through.
// -----------------------------------------------------------------------------

// TestTrailers_DecodeTrailers_NilCfg_PassesThrough asserts the defensive
// nil-cfg pass-through.
func TestTrailers_DecodeTrailers_NilCfg_PassesThrough(t *testing.T) {
	t.Parallel()
	f := &filter{} // cfg nil

	if got := f.DecodeTrailers(gohttp.Header{}); got != envoyhttp.TrailersContinue {
		t.Errorf("DecodeTrailers (nil cfg) = %v; want TrailersContinue", got)
	}
	if got := f.EncodeTrailers(gohttp.Header{}); got != envoyhttp.TrailersContinue {
		t.Errorf("EncodeTrailers (nil cfg) = %v; want TrailersContinue", got)
	}
}

// -----------------------------------------------------------------------------
// 3. EncodeTrailers NO-op when guest did not export proxy_on_response_trailers.
// -----------------------------------------------------------------------------

// TestTrailers_EncodeTrailers_NoOpWhenGuestNotExported asserts the encode-
// side NO-op pass-through when streamCtx is nil.
func TestTrailers_EncodeTrailers_NoOpWhenGuestNotExported(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTrailersTestCompiledConfig(t, "plugin_trailers_enc_noop", reg)
	f := &filter{cfg: cc}

	trailers := gohttp.Header{}
	trailers.Set("Grpc-Status", "0")
	trailers.Set("Grpc-Message", "")

	if got := f.EncodeTrailers(trailers); got != envoyhttp.TrailersContinue {
		t.Errorf("EncodeTrailers (no streamCtx) = %v; want TrailersContinue", got)
	}
}

// -----------------------------------------------------------------------------
// 4. numTrailers multi-value expansion per §5.3 C16.
// -----------------------------------------------------------------------------

// TestTrailers_NumTrailers_MultiValueExpansion exercises the numHeaderValues
// helper (which trailers.go calls to compute the wire numTrailers arg). The
// expansion semantic: multi-value trailers expand to one entry per value,
// matching the GetHeaderMap pair-emission shape per §5.3 C16.
//
// At Task 16 the only observable surface is the count itself; once Task 18
// wires the streamCtx end-to-end, this can be extended to a full dispatch
// assertion via mock guest.
func TestTrailers_NumTrailers_MultiValueExpansion(t *testing.T) {
	t.Parallel()
	trailers := gohttp.Header{}
	trailers.Add("X-Multi", "value1")
	trailers.Add("X-Multi", "value2")
	trailers.Add("X-Multi", "value3")
	trailers.Set("X-Single", "soloval")

	got := numHeaderValues(trailers)
	want := uint32(4) // 3 X-Multi + 1 X-Single

	if got != want {
		t.Errorf("numHeaderValues(multi-value) = %d; want %d", got, want)
	}
}

// -----------------------------------------------------------------------------
// 5. Nil decoderCb defensive pass-through.
// -----------------------------------------------------------------------------

// TestTrailers_DecodeTrailers_NilDecoderCb_DoesNotPanic asserts that when
// streamCtx is nil + decoderCb is nil, DecodeTrailers does not panic on
// the captured-local-response branch (the branch is short-circuited by
// the streamCtx-nil gate above it).
func TestTrailers_DecodeTrailers_NilDecoderCb_DoesNotPanic(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTrailersTestCompiledConfig(t, "plugin_trailers_nil_dcb", reg)
	f := &filter{cfg: cc} // streamCtx + decoderCb both nil

	if got := f.DecodeTrailers(gohttp.Header{}); got != envoyhttp.TrailersContinue {
		t.Errorf("DecodeTrailers (nil streamCtx + decoderCb) = %v; want TrailersContinue", got)
	}
}

// -----------------------------------------------------------------------------
// 6. EncodeTrailers + EncodeData mirror behavior is exercised at body_test.go.
// -----------------------------------------------------------------------------

// (Encode-side trailer mirror tests would follow the same pattern as the
// decode tests above; the NO-op-when-streamCtx-nil + nil-cfg patterns are
// the only observable behaviors at Task 16. End-to-end streamCtx-driven
// scenarios land at Task 18 dispatch_test.go EXTEND.)
