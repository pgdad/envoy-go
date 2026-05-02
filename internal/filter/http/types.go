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

// HeaderField is one (name, value) header entry for the SendLocalReply path.
// Pairs with OrderedHeaders below to carry deterministic insertion order from
// the calling filter through the chain's encode iteration to the wire-write
// layer (writeH1Reply / writeH2Reply). Per Task 18 review: the unordered
// http.Header map cannot preserve the SPEC §11.2 verbatim 6-header order on
// the wire (Go map iteration is non-deterministic; net/http's headers.Write
// emits alphabetically sorted). HeaderField + OrderedHeaders is the ordered
// carrier that closes that gap.
type HeaderField struct {
	Name  string
	Value string
}

// OrderedHeaders is the ordered (name, value) carrier used by SendLocalReply
// and the local-reply wire-write path. The carrier preserves caller insertion
// order so reference Envoy's CORS preflight 6-header §11.2 order survives
// chain encode-iteration and lands on the wire byte-for-byte. Encode-side
// filters operating on the SendLocalReply path may mutate values via the
// http.Header view the chain hands them; the chain reconciles mutations back
// onto the OrderedHeaders carrier after RunEncodeHeaders returns (preserving
// caller-order for known names; appending net-new keys after the original set).
//
// Per Task 18 review: an http.Header carrier loses order on the wire — Go map
// iteration is non-deterministic and net/http's stdlib Header.Write emits
// keys alphabetically sorted. OrderedHeaders carries the §11.2 verbatim
// 6-header order from cors.go through the chain to the wire-write layer.
type OrderedHeaders []HeaderField

// Get returns the first value for name (case-insensitive comparison via
// http.CanonicalHeaderKey) or "" if absent. Mirrors http.Header.Get for the
// ordered carrier.
func (oh OrderedHeaders) Get(name string) string {
	canon := http.CanonicalHeaderKey(name)
	for _, hf := range oh {
		if http.CanonicalHeaderKey(hf.Name) == canon {
			return hf.Value
		}
	}
	return ""
}

// ToHTTPHeader returns an http.Header view of the ordered carrier. Used by
// the chain to feed encode-side filter EncodeHeaders calls (which still
// operate on the http.Header API). Mutations on the returned map are
// reconciled back to the carrier by the chain after iteration completes.
func (oh OrderedHeaders) ToHTTPHeader() http.Header {
	h := make(http.Header, len(oh))
	for _, hf := range oh {
		h.Add(hf.Name, hf.Value)
	}
	return h
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
