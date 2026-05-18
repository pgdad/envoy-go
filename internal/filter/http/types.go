package http

import (
	"net/http"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/httpclient"
	"github.com/esalaine/envoy-go/internal/stats"
)

// FilterHeadersStatus is returned by DecodeHeaders / EncodeHeaders to signal
// iteration control. Per ADR-0071 (Envoy-faithful subset; ContinueAndDontEndStream
// out of MVP).
type FilterHeadersStatus int

// FilterHeadersStatus enum values per ADR-0071.
const (
	// Continue advances to the next filter in the chain.
	Continue FilterHeadersStatus = iota
	// StopIteration parks the chain; resume via cb.ContinueDecoding/ContinueEncoding.
	StopIteration
)

// FilterDataStatus is returned by DecodeData / EncodeData to signal iteration
// control + per-filter buffering. Per ADR-0071 (watermark variants out of MVP).
type FilterDataStatus int

// FilterDataStatus enum values per ADR-0071.
const (
	// DataContinue advances to the next filter.
	DataContinue FilterDataStatus = iota
	// DataStopIterationAndBuffer parks the chain and accumulates body chunks until end_stream.
	DataStopIterationAndBuffer
	// DataStopIterationNoBuffer parks the chain without accumulating body bytes.
	DataStopIterationNoBuffer
)

// FilterTrailersStatus is returned by DecodeTrailers / EncodeTrailers.
type FilterTrailersStatus int

// FilterTrailersStatus enum values per ADR-0071.
const (
	// TrailersContinue advances to the next filter.
	TrailersContinue FilterTrailersStatus = iota
	// TrailersStopIteration parks the chain; resume via cb.Continue*.
	TrailersStopIteration
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
//
//nolint:revive // ADR-0071 reserves the HTTPFilter name for the iteration-protocol surface.
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

// OrderedHeadersFromHTTPHeader projects an http.Header (Go map) into an
// OrderedHeaders carrier with deterministic alphabetical key ordering. Used by
// the action-driven dispatch path (clusterRouteAction's upstream response
// projection, directResponseAction's framework-locally-synthesized headers)
// where the source is an unordered map and a deterministic carrier-order is
// required for the wire-write layer.
//
// The original-wire-order of upstream response headers is LOST inside Go's
// net/http response parser (resp.Header is a map; iteration is non-
// deterministic). Sorting alphabetically gives a deterministic result that
// passes byte-equality checks across runs of the same fixture, but does NOT
// preserve the upstream wire order. This trade-off is documented in PROGRESS:
// reference Envoy preserves upstream wire order on the actual-request path;
// envoy-go's actual-request path uses alphabetical-by-canonical-name. Cors's
// 3-header injection appends after this base set (preserving the cors order
// inside its own append) — the mixed result is alphabetical for upstream-
// originating keys + cors-order for the cors-appended set.
func OrderedHeadersFromHTTPHeader(h http.Header) OrderedHeaders {
	if len(h) == 0 {
		return nil
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	// Insertion sort — typically <20 keys; avoids importing "sort" for this
	// hot-path helper. Mirrors the helper inside chain.go's
	// reconcileOrderedHeaders.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	out := make(OrderedHeaders, 0, len(h))
	for _, k := range keys {
		canon := http.CanonicalHeaderKey(k)
		for _, v := range h[k] {
			out = append(out, HeaderField{Name: canon, Value: v})
		}
	}
	return out
}

// ReconcileOrderedHeaders merges encode-chain mutations on `merged` (an
// http.Header map view) back onto the caller-supplied ordered carrier
// `original`. For each name in `original` (looked up canonically), emits the
// post-encode value(s) from `merged` in the original order; then appends any
// net-new header keys from `merged` (canonical key already, since merged was
// built via Add which canonicalizes) in deterministic alphabetical order.
//
// This is the order-preservation bridge between the http.Header-mutation-
// friendly encode-chain API (per ADR-0071's filter API stability:
// EncodeHeaders takes http.Header, not OrderedHeaders) and the order-pinned
// wire-emit shape required by SPEC §11.2.
//
// Phase 07.1 Task 19 (I-3 prereq): exported so HCM dispatch's action-driven
// path (writeH1Reply / writeH2Reply on ActionResponse) can use the same
// reconcile machinery as chain.beginLocalReply's SendLocalReply path. Pre-
// Task-19 only chain.beginLocalReply used this; Task 19 promotes
// ActionResponse.Headers from http.Header to OrderedHeaders and the action-
// driven encode chain reconciles via this helper.
//
// Net-new keys (cors's encode-side allow-origin/allow-credentials/expose-
// headers Set()s on the actual-request path; framework-injected
// Content-Length / Content-Type on the SendLocalReply path) sort
// alphabetically AFTER the original carrier's order. This is deterministic
// (Go map iteration is non-deterministic) but does NOT preserve cors's
// internal Set-order; reference Envoy's actual-request 3-header order is
// approximated, not byte-exact. Documented in PROGRESS as the I-3 close-out
// trade-off.
func ReconcileOrderedHeaders(original OrderedHeaders, merged http.Header) OrderedHeaders {
	out := make(OrderedHeaders, 0, len(merged))
	seen := make(map[string]bool, len(original))
	for _, hf := range original {
		canon := http.CanonicalHeaderKey(hf.Name)
		if seen[canon] {
			continue // dedupe duplicates in original; first occurrence wins
		}
		seen[canon] = true
		if vs, ok := merged[canon]; ok {
			for _, v := range vs {
				out = append(out, HeaderField{Name: canon, Value: v})
			}
		}
	}
	newKeys := make([]string, 0)
	for k := range merged {
		if !seen[k] {
			newKeys = append(newKeys, k)
		}
	}
	for i := 1; i < len(newKeys); i++ {
		for j := i; j > 0 && newKeys[j-1] > newKeys[j]; j-- {
			newKeys[j-1], newKeys[j] = newKeys[j], newKeys[j-1]
		}
	}
	for _, k := range newKeys {
		for _, v := range merged[k] {
			out = append(out, HeaderField{Name: k, Value: v})
		}
	}
	return out
}

// HTTPFilterFactory parses + validates typed_config once at HCM-build time and
// returns a per-request FilterInstanceFactory closure. Per ADR-0071 two-step
// factory pattern.
//
//nolint:revive // ADR-0071 reserves the HTTPFilterFactory name for the boot-time factory surface.
type HTTPFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)

// FilterInstanceFactory allocates a fresh filter instance bound to the parsed
// config. Called once per request.
type FilterInstanceFactory func() HTTPFilter

// FactoryCtx carries the registry pointer + parsed proto-helpers needed by
// per-filter parsers.
type FactoryCtx struct {
	Registry *HTTPRegistry // optional reference for filter factories that need to look up sibling filters
	// Stats is the *stats.Registry the per-filter factory uses for stat-name
	// registration. Non-nil at HCM-build time per ADR-0061's pre-Freeze
	// discipline. May be nil in test code that does not exercise stat-bearing
	// filters; per ADR-0085 nil-tolerance pattern. Phase 09 first-use anchor;
	// ADR-0100 §Consequences records the framework consequence.
	Stats *stats.Registry
	// StatPrefix is the HCM's stat_prefix per ADR-0061's
	// "http.<stat_prefix>.<metric>" discipline. Empty in test code that does
	// not exercise stat-bearing filters. Phase 09 first-use anchor.
	StatPrefix string
	// ClusterManager is the bootstrap-time cluster manager (per ADR-0158)
	// threaded into per-filter factories that need to resolve an upstream
	// cluster name to a `*cluster.Cluster` (e.g. ext_authz `grpc_service.
	// envoy_grpc.cluster_name`). Non-nil at HCM-build time per the phase-18.2
	// landing; may be nil in test code that does not exercise cluster-bearing
	// filters (per ADR-0085 nil-tolerance — the filter factory checks for nil
	// before calling Get/Drain). Phase 18.2 first-use anchor.
	ClusterManager *cluster.Manager
	// HTTPClient is the shared `*httpclient.Client` framework primitive (per
	// ADR-0177) threaded into per-filter factories that need to issue outbound
	// HTTP requests (jwks Fetcher post-ADR-0150 §Decision AMENDMENT;
	// extauthz httpAuthClient post-ADR-0159 §Decision AMENDMENT + §Future
	// Work CLOSURE; oauth2 token_endpoint POST per ADR-0185). Non-nil at
	// HCM-build time per the phase-20 IMPL landing; may be nil in test code
	// that does not exercise outbound-HTTP-bearing filters (per ADR-0085
	// nil-tolerance — each consumer applies its own per-consumer default per
	// ADR-0150 §Decision AMENDMENT / ADR-0159 §Decision AMENDMENT).
	// Phase 20 first-use anchor.
	HTTPClient *httpclient.Client
	// Future extensions (accesslog sinks) added per-family-phase as filter
	// implementations require them.
}
