package http

import (
	"net/http"

	"google.golang.org/protobuf/proto"
)

// DecoderFilterCallbacks is the framework-supplied callback shape for
// decode-side filters. Every method except RequestRouteConfig is safe to call
// from any goroutine (per ADR-0071's async-resume mechanics).
type DecoderFilterCallbacks interface {
	// ContinueDecoding wakes the dispatch goroutine if it is parked on a
	// StopIteration return. Idempotent: duplicate calls are coalesced via the
	// chain's per-stream resume channel (capacity 1, non-blocking send).
	ContinueDecoding()

	// SendLocalReply synthesizes a response that enters the encode chain at
	// filter[len-1] (per ADR-0075 + SPEC §11 #4 empirical pin). First-call-wins
	// via sync.Once on the chain; second-call-after-encode-started is a no-op
	// + log line.
	SendLocalReply(status int, body string, headers http.Header)

	// RequestRouteConfig returns the merged proto.Message for the calling
	// filter's name (Route > VirtualHost > RouteConfiguration most-specific
	// override per ADR-0073). Nil if no per-route config applies. Lazy-cached
	// on first lookup per request.
	RequestRouteConfig() proto.Message

	// EncodeHeaders / EncodeData / EncodeTrailers are encode-side injection
	// methods for filters that synthesize responses without using
	// SendLocalReply. Rare; intended for filters like header_manipulation
	// that need to inject encode-side material from a decode-side context.
	EncodeHeaders(headers http.Header, endStream bool)
	EncodeData(data []byte, endStream bool)
	EncodeTrailers(trailers http.Header)
}

// EncoderFilterCallbacks is the framework-supplied callback shape for
// encode-side filters.
type EncoderFilterCallbacks interface {
	// ContinueEncoding wakes the dispatch goroutine if it is parked on an
	// encode-side StopIteration return. Same coalescing discipline as
	// ContinueDecoding.
	ContinueEncoding()

	// EncodeHeaders / EncodeData / EncodeTrailers are encode-side injection
	// methods (rare).
	EncodeHeaders(headers http.Header, endStream bool)
	EncodeData(data []byte, endStream bool)
	EncodeTrailers(trailers http.Header)
}

// Static-assertion helpers: the proto.Message return type must compile with
// google.golang.org/protobuf/proto.
var _ proto.Message = nil
