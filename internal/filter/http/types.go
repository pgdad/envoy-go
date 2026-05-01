package http

import (
	"net/http"

	"google.golang.org/protobuf/types/known/anypb"
)

// FilterHeadersStatus is returned by DecodeHeaders / EncodeHeaders to signal
// iteration control. Per ADR-0071 (Envoy-faithful subset; ContinueAndDontEndStream
// out of MVP).
type FilterHeadersStatus int

const (
	Continue       FilterHeadersStatus = iota // proceed to the next filter
	StopIteration                             // park; resume via cb.ContinueDecoding/ContinueEncoding
)

// FilterDataStatus is returned by DecodeData / EncodeData to signal iteration
// control + per-filter buffering. Per ADR-0071 (watermark variants out of MVP).
type FilterDataStatus int

const (
	DataContinue              FilterDataStatus = iota // proceed
	DataStopIterationAndBuffer                        // park + accumulate body chunks until end_stream
	DataStopIterationNoBuffer                         // park; no body accumulation
)

// FilterTrailersStatus is returned by DecodeTrailers / EncodeTrailers.
type FilterTrailersStatus int

const (
	TrailersContinue       FilterTrailersStatus = iota // proceed
	TrailersStopIteration                              // park; resume via cb.Continue*
)

// StreamDecoderFilter is implemented by filters that participate in the
// downstream-to-upstream (decode) iteration. A filter implements decode-only,
// encode-only, or both; the factory's return type signals which side(s).
type StreamDecoderFilter interface {
	DecodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus
	DecodeData(data []byte, endStream bool) FilterDataStatus
	DecodeTrailers(trailers http.Header) FilterTrailersStatus
	SetDecoderCallbacks(cb DecoderFilterCallbacks)
	OnDestroy()
}

// StreamEncoderFilter is implemented by filters that participate in the
// upstream-to-downstream (encode) iteration. Encode iteration is reverse of
// decode (per ADR-0071 + SPEC §11.1 empirical pin).
type StreamEncoderFilter interface {
	EncodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus
	EncodeData(data []byte, endStream bool) FilterDataStatus
	EncodeTrailers(trailers http.Header) FilterTrailersStatus
	SetEncoderCallbacks(cb EncoderFilterCallbacks)
	OnDestroy()
}

// HTTPFilter is the tagged-union over decoder-only / encoder-only / both. The
// factory returns this; the chain dispatches per non-nil side.
type HTTPFilter struct {
	Name    string              // filter name from http_filters[].name
	Decoder StreamDecoderFilter // nil for encoder-only filters
	Encoder StreamEncoderFilter // nil for decoder-only filters
}

// HTTPFilterFactory parses + validates typed_config once at HCM-build time and
// returns a per-request FilterInstanceFactory closure. Per ADR-0071 two-step
// factory pattern.
type HTTPFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)

// FilterInstanceFactory allocates a fresh filter instance bound to the parsed
// config. Called once per request.
type FilterInstanceFactory func() HTTPFilter

// FactoryCtx carries the registry pointer + parsed proto-helpers needed by
// per-filter parsers.
type FactoryCtx struct {
	Registry *HTTPRegistry // optional reference for filter factories that need to look up sibling filters
	// Future extensions (cluster manager, stats registry, accesslog sinks) added
	// per-family-phase as filter implementations require them.
}

// HTTPRegistry is the extension registry for HTTP filters. Defined in full in
// registry.go (Task 3); forward-declared here so FactoryCtx compiles in this
// task before registry.go lands.
type HTTPRegistry struct{}
