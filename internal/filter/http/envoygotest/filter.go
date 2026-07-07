package envoygotest

import (
	"net/http"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	envoygotestpb "github.com/pgdad/envoy-go/internal/filter/http/envoygotest/proto"
)

// TypeURL is the canonical envoy.filters.http.envoy_go_test typed_config type
// URL. Boot wiring in cmd/envoy-go/main.go (Task 20) registers New under this
// key in the HTTPRegistry per ADR-0072.
//
// Mirrors envoygotestpb.TypeURLEnvoyGoTest exactly; duplicated here to avoid
// callers needing to import the proto subpackage to read the type URL.
const TypeURL = envoygotestpb.TypeURLEnvoyGoTest

// modeHeaderName is the per-request mode-dispatch header. Lowercased per
// HTTP/1.x case-insensitive lookup conventions; http.Header.Get applies
// canonical-MIME canonicalization on lookup so case is normalized at the
// caller site.
const modeHeaderName = "x-envoy-go-test-mode"

// asyncResumeDelay is the wall-clock delay between SendLocalReply / parkDecode
// entry and the corresponding ContinueDecoding fire on the async-resume modes
// (stop-and-resume-headers, stop-and-buffer-data, stop-trailers). Per PLAN
// Task 19 step 3: 10ms is realistic enough to exercise the framework's
// parkDecode loop through a real wall-clock delay (not just a synchronous
// send-then-receive race) without inflating test runtimes.
const asyncResumeDelay = 10 * time.Millisecond

// New is the HTTPFilterFactory exposed at boot. The filter-level config carries
// a `mode_default` field used as the per-request mode when the
// `x-envoy-go-test-mode` header is absent. tc may be nil (default empty
// config); a non-nil tc unmarshals into a *EnvoyGoTest for mode_default
// extraction.
func New(tc *anypb.Any, _ envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	modeDefault := ""
	if tc != nil {
		cfg, err := envoygotestpb.NewEnvoyGoTest()
		if err != nil {
			return nil, err
		}
		if err := tc.UnmarshalTo(cfg.Message); err != nil {
			return nil, err
		}
		modeDefault = cfg.GetModeDefault()
	}
	return func() envoyhttp.HTTPFilter {
		f := &filter{modeDefault: modeDefault}
		return envoyhttp.HTTPFilter{
			Name:    "envoy.filters.http.envoy_go_test",
			Decoder: f,
			Encoder: f,
		}
	}, nil
}

// filter implements both StreamDecoderFilter + StreamEncoderFilter. The mode
// for the current request is captured during DecodeHeaders (from the
// x-envoy-go-test-mode header or the modeDefault fallback) and persists
// through subsequent DecodeData / DecodeTrailers / EncodeHeaders / EncodeData
// callbacks.
//
// Single-goroutine-per-stream invariant per ADR-0071 makes the per-instance
// state race-free without synchronization. The async-resume modes spawn a
// goroutine that calls dcb.ContinueDecoding after asyncResumeDelay; the
// goroutine is fire-and-forget — its only side effect is the resume signal
// channel send, which is safe from any goroutine per the chain framework's
// async-resume contract.
type filter struct {
	dcb envoyhttp.DecoderFilterCallbacks

	// modeDefault is the filter-level config's mode_default field, set at
	// factory time. Used when the per-request mode header is absent.
	modeDefault string

	// mode is captured during DecodeHeaders for use across subsequent
	// callbacks (DecodeData / DecodeTrailers / EncodeHeaders / EncodeData).
	mode string
}

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }

// SetEncoderCallbacks is a no-op retained to satisfy StreamEncoderFilter —
// the encode-side probes mutate headers/body in place and read per-route
// config through f.dcb (see routeCount), never the encoder callbacks.
func (f *filter) SetEncoderCallbacks(envoyhttp.EncoderFilterCallbacks) {}
func (f *filter) OnDestroy()                                           {}

// DecodeHeaders is the entry point for per-request mode dispatch. The mode is
// resolved from the x-envoy-go-test-mode header (case-insensitive match via
// http.Header.Get's canonicalization), falling back to f.modeDefault when
// absent. Unknown modes return Continue (defensive default for fixture
// debugging).
//
// The dispatch is an explicit `switch` per Decision §3.7 (no map): the Go
// compiler emits a jump table; a single anchor for adding modes; cold paths
// (StopIteration + ContinueDecoding) are visually grouped.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	mode := headers.Get(modeHeaderName)
	if mode == "" {
		mode = f.modeDefault
	}
	f.mode = mode

	switch mode {
	case "continue":
		return envoyhttp.Continue
	case "stop-and-resume-headers":
		go func() {
			time.Sleep(asyncResumeDelay)
			f.dcb.ContinueDecoding()
		}()
		return envoyhttp.StopIteration
	case "stop-and-buffer-data":
		// Headers pass through; the buffer-resume happens on DecodeData.
		return envoyhttp.Continue
	case "local-reply-decode":
		f.dcb.SendLocalReply(418, "i am a teapot\n", nil)
		return envoyhttp.StopIteration
	case "local-reply-decode-data":
		// Headers pass through; the local reply fires from DecodeData.
		return envoyhttp.Continue
	case "modify-encode-headers":
		// No decode-side action; encode-side will mutate.
		return envoyhttp.Continue
	case "modify-encode-data":
		// No decode-side action; encode-side will mutate.
		return envoyhttp.Continue
	case "stop-trailers":
		// Headers pass through; the trailers-stop happens on DecodeTrailers.
		return envoyhttp.Continue
	default:
		// Unknown mode → pass through (defensive default for debugging).
		return envoyhttp.Continue
	}
}

// DecodeData is the body-side dispatch. Branches on the mode captured during
// DecodeHeaders (stored in f.mode). Two modes have body-side behavior:
// stop-and-buffer-data (DataStopIterationAndBuffer + spawn resumer) and
// local-reply-decode-data (SendLocalReply + StopIteration).
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	switch f.mode {
	case "stop-and-buffer-data":
		go func() {
			time.Sleep(asyncResumeDelay)
			f.dcb.ContinueDecoding()
		}()
		return envoyhttp.DataStopIterationAndBuffer
	case "local-reply-decode-data":
		f.dcb.SendLocalReply(418, "i am a teapot\n", nil)
		return envoyhttp.DataStopIterationNoBuffer
	default:
		return envoyhttp.DataContinue
	}
}

// DecodeTrailers is the trailer-side dispatch. Single mode has trailer-side
// behavior: stop-trailers (TrailersStopIteration + spawn resumer).
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	switch f.mode {
	case "stop-trailers":
		go func() {
			time.Sleep(asyncResumeDelay)
			f.dcb.ContinueDecoding()
		}()
		return envoyhttp.TrailersStopIteration
	default:
		return envoyhttp.TrailersContinue
	}
}

// EncodeHeaders is the encode-side dispatch. Two modes have encode-side
// behavior: modify-encode-headers (set x-envoy-go-test-encoded: yes); the
// per-route count echo (set x-envoy-go-test-route-count: <N>) is universal —
// it fires on every request with non-zero per-route count, regardless of
// mode.
func (f *filter) EncodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Universal: per-route count echo.
	if count := f.routeCount(); count > 0 {
		headers.Set("x-envoy-go-test-route-count", strconv.FormatInt(int64(count), 10))
	}

	switch f.mode {
	case "modify-encode-headers":
		headers.Set("x-envoy-go-test-encoded", "yes")
	}
	return envoyhttp.Continue
}

// EncodeData is the encode-side body dispatch. Single mode has body-side
// behavior: modify-encode-data (overwrite the leading body bytes with
// "MODIFIED\n" in place on the slice's backing array).
//
// The probe writes up to len(data) bytes from "MODIFIED\n" — for slices
// shorter than 9 bytes, only the prefix lands. The wire body keeps its
// ORIGINAL length: the tail beyond the replacement is NUL-padded, not
// truncated (the slice header cannot shrink through an in-place mutation,
// and the fixture pins this padded shape). The chain framework hands filters
// the same backing array forward through the encode iteration, so this
// in-place mutation is visible to subsequent encode-side filters / the
// wire-write layer.
func (f *filter) EncodeData(data []byte, _ bool) envoyhttp.FilterDataStatus {
	if f.mode == "modify-encode-data" {
		const replacement = "MODIFIED\n"
		n := copy(data, replacement)
		// NUL-pad the tail beyond the replacement — length preserved (the
		// chain forwards the slice header, not just the content).
		clear(data[n:])
	}
	return envoyhttp.DataContinue
}

// EncodeTrailers is the encode-side trailer dispatch — pass-through.
func (f *filter) EncodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// routeCount returns the per-route count config, or 0 if absent / wrong type.
// The decoder callback's RequestRouteConfig returns proto.Message; we read
// the `count` field via protoreflect so both the typed wrapper
// (*EnvoyGoTestPerRoute) and a raw *dynamicpb.Message (returned by
// anypb.Any.UnmarshalNew when the global proto registry resolves the type
// URL) work uniformly.
//
// Per the encode-side dispatch's order (chain visits encode filters in
// REVERSE declaration order), this method may be called via the encode
// callback's RequestRouteConfig — but the framework wires
// RequestRouteConfig only on the decoder callback (per ADR-0073). We
// therefore reach into the decoder callback (f.dcb) which the chain wires at
// SetDecoderCallbacks time, regardless of which side is currently iterating.
func (f *filter) routeCount() int32 {
	if f.dcb == nil {
		return 0
	}
	cfg := f.dcb.RequestRouteConfig()
	if cfg == nil {
		return 0
	}
	// First try the typed wrapper.
	if pr, ok := cfg.(*envoygotestpb.EnvoyGoTestPerRoute); ok {
		return pr.GetCount()
	}
	// Fallback: read via protoreflect. Works for both *dynamicpb.Message
	// (when UnmarshalNew used the global type registry's NewFunc) and any
	// other proto.Message backed by our descriptor.
	return countViaReflect(cfg)
}

// countViaReflect walks msg's ProtoReflect descriptor for a field named
// "count" and returns its int32 value, or 0 if absent / type mismatch.
func countViaReflect(msg proto.Message) int32 {
	if msg == nil {
		return 0
	}
	pm := msg.ProtoReflect()
	fd := pm.Descriptor().Fields().ByName("count")
	if fd == nil {
		return 0
	}
	v := pm.Get(fd)
	return int32(v.Int())
}
