package http

import (
	"net/http"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
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
	//
	// Per Task 18 review (SPEC §11.2 ordered-headers compliance): the headers
	// parameter is an ordered (name, value) carrier (OrderedHeaders) so the
	// caller-supplied insertion order survives the chain's encode-iteration
	// and the wire-write layer. The unordered http.Header map cannot preserve
	// the §11.2 verbatim 6-header order — Go map iteration is non-deterministic
	// and net/http's Header.Write emits keys alphabetically sorted.
	SendLocalReply(status int, body string, headers OrderedHeaders)

	// RequestRouteConfig returns the merged proto.Message for the calling
	// filter's name (Route > VirtualHost > RouteConfiguration most-specific
	// override per ADR-0073). Nil if no per-route config applies. Lazy-cached
	// on first lookup per request.
	RequestRouteConfig() proto.Message

	// RequestRouteConfigsAllTiers returns the parsed per-route config at each
	// of the three tiers (Route, VirtualHost, RouteConfiguration), UNMERGED.
	// Used by filters whose semantics require multi-tier evaluation rather
	// than most-specific override — primarily envoy.filters.http.header_mutation
	// per its most_specific_header_mutations_wins flag (see ADR-0110 amending
	// ADR-0073). The default RequestRouteConfig method (per ADR-0073) remains
	// the canonical accessor for filters that use most-specific override
	// (cors, fault).
	//
	// Per phase 10 PLAN planner-time decision 1: this callback lives ONLY on
	// DecoderFilterCallbacks (NOT on EncoderFilterCallbacks). Filters that
	// need it on the encode side use the dcb reference set via
	// SetDecoderCallbacks (the framework wires both dcb and ecb on a both-
	// sides filter). The cors precedent at cors.go:163 (routePolicy) calls
	// f.dcb.RequestRouteConfig() from EncodeHeaders — same pattern applies.
	//
	// Returns (nil, nil, nil) when:
	//   - the chain has no perRoute config;
	//   - no scope at any tier carries an entry for the calling filter's name.
	RequestRouteConfigsAllTiers() (route, vhost, rc proto.Message)

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

	// OverwriteBody registers a replacement encode-side body. Filters MUST
	// call this only from inside their EncodeData(data, endStream)
	// implementation; the chain dispatch substitutes resp.Body before the
	// wire-write path consumes it. Not goroutine-safe — the encode chain
	// runs synchronously in the dispatch goroutine.
	//
	// Per ADR-0131 §Decision (vi); first encode-side framework primitive in
	// envoy-go (phase 14; symmetric to phase-13 ADR-0128 decode-side
	// primitives).
	OverwriteBody(b []byte)
}

// Compile-time assertion: a real concrete proto type satisfies proto.Message,
// confirming the import wiring (the package's RequestRouteConfig contract
// returns a value of this interface type).
var _ proto.Message = (*anypb.Any)(nil)
