package cors

import (
	"net/http"
	"strings"

	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// TypeURL is the canonical envoy.filters.http.cors typed_config type URL.
// Boot wiring in cmd/envoy-go/main.go (Task 20) registers New under this key
// in the HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors"

// New is the HTTPFilterFactory exposed at boot. The filter-level Cors message
// has no fields envoy-go consumes at 07.1; everything behavioral is per-route
// via CorsPolicy. tc may be nil; a non-nil tc is unmarshaled-and-discarded
// only to honor the ADR-0072 "factories validate typed_config shape" contract.
func New(tc *anypb.Any, _ envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc != nil {
		var c corsv3.Cors
		if err := tc.UnmarshalTo(&c); err != nil {
			return nil, err
		}
	}
	return func() envoyhttp.HTTPFilter {
		f := &filter{}
		return envoyhttp.HTTPFilter{
			Name:    "envoy.filters.http.cors",
			Decoder: f,
			Encoder: f,
		}
	}, nil
}

// filter implements both StreamDecoderFilter + StreamEncoderFilter so the
// chain framework can drive it as a both-sides filter (decode-side preflight
// detection + encode-side header injection on actual responses).
//
// originAllowed + matchedOrigin are populated during DecodeHeaders and
// consumed by EncodeHeaders. Single-goroutine-per-stream invariant per
// ADR-0071 makes the per-instance state race-free without synchronization.
type filter struct {
	dcb envoyhttp.DecoderFilterCallbacks
	ecb envoyhttp.EncoderFilterCallbacks

	// Captured during DecodeHeaders for encode-side use.
	originAllowed bool
	matchedOrigin string
}

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }
func (f *filter) OnDestroy()                                              {}

// DecodeHeaders implements the cors filter's decode-side discipline per SPEC §11.2:
//
//   - No Origin header → no-op (filter is gated on CORS-origin requests only).
//   - Origin present + method=OPTIONS + Access-Control-Request-Method present →
//     this is a preflight. If origin is allowed by the per-route policy, emit
//     SendLocalReply(200, "", corsHeaders) with the six headers in fixed order
//     per the §11.2 verbatim pin; otherwise pass through (router 405s).
//   - Origin present + non-preflight → record originAllowed; let the request
//     flow to the router; encode-side will inject three CORS response headers
//     when originAllowed.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	origin := headers.Get("Origin")
	if origin == "" {
		// Not a CORS request — no-op.
		return envoyhttp.Continue
	}

	policy := f.routePolicy()
	f.originAllowed = originAllowedByPolicy(origin, policy)
	if f.originAllowed {
		f.matchedOrigin = origin
	}

	method := strings.ToUpper(strings.TrimSpace(getMethod(headers)))
	isPreflight := method == "OPTIONS" && headers.Get("Access-Control-Request-Method") != ""

	if !isPreflight {
		// Actual request: pass through; encode-side handles header injection
		// when the upstream response comes back.
		return envoyhttp.Continue
	}

	if !f.originAllowed {
		// Disallowed-origin preflight: cors does NOT inject a 4xx local reply
		// per SPEC §11.2 empirical pin (probe b). Pass through to the router
		// which will 405 the OPTIONS request.
		return envoyhttp.Continue
	}

	// Allowed-origin preflight: synthesize the 200 response with the six
	// CORS headers in §11.2 verbatim order via SendLocalReply. The chain
	// runs the encode side inline (per ADR-0075) and aborts decode iteration.
	ph := http.Header{}
	// Order matters for wire emission downstream (HCM dispatch + writeStatusReply
	// preserve insertion / canonical order in the body() shape). The six
	// headers are populated in §11.2 order; the chain's beginLocalReply uses
	// http.Header.Add for canonicalization.
	ph.Set("Access-Control-Allow-Origin", origin)
	if policy.GetAllowCredentials().GetValue() {
		ph.Set("Access-Control-Allow-Credentials", "true")
	}
	if methods := policy.GetAllowMethods(); methods != "" {
		ph.Set("Access-Control-Allow-Methods", methods)
	}
	if hdrs := policy.GetAllowHeaders(); hdrs != "" {
		ph.Set("Access-Control-Allow-Headers", hdrs)
	}
	if maxAge := policy.GetMaxAge(); maxAge != "" {
		ph.Set("Access-Control-Max-Age", maxAge)
	}
	if expose := policy.GetExposeHeaders(); expose != "" {
		ph.Set("Access-Control-Expose-Headers", expose)
	}
	f.dcb.SendLocalReply(200, "", ph)
	return envoyhttp.StopIteration
}

// EncodeHeaders implements the cors filter's encode-side discipline per
// SPEC §11.2 probe (c): on actual responses where the request had an allowed
// Origin, inject three CORS response headers (allow-origin, allow-credentials,
// expose-headers — preflight-only headers are NOT injected on actual requests).
func (f *filter) EncodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	if !f.originAllowed {
		return envoyhttp.Continue
	}
	policy := f.routePolicy()
	headers.Set("Access-Control-Allow-Origin", f.matchedOrigin)
	if policy.GetAllowCredentials().GetValue() {
		headers.Set("Access-Control-Allow-Credentials", "true")
	}
	if expose := policy.GetExposeHeaders(); expose != "" {
		headers.Set("Access-Control-Expose-Headers", expose)
	}
	return envoyhttp.Continue
}

// DecodeData / DecodeTrailers / EncodeData / EncodeTrailers are pass-through:
// cors operates exclusively on the headers phase.
func (f *filter) DecodeData([]byte, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (f *filter) EncodeData([]byte, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// routePolicy returns the resolved per-route CorsPolicy for this request's
// matched route, or an empty policy if no per-route config carries cors.
// The chain's RequestRouteConfig delegates to the perRouteConfig 3-tier merge.
func (f *filter) routePolicy() *corsv3.CorsPolicy {
	if f.dcb == nil {
		return &corsv3.CorsPolicy{}
	}
	cfg := f.dcb.RequestRouteConfig()
	if cfg == nil {
		return &corsv3.CorsPolicy{}
	}
	p, ok := cfg.(*corsv3.CorsPolicy)
	if !ok {
		// Defensive: shouldn't happen if perroute parses anypb correctly to
		// *corsv3.CorsPolicy. Empty policy → all origins disallowed → no-op.
		return &corsv3.CorsPolicy{}
	}
	return p
}

// originAllowedByPolicy applies the per-route policy's StringMatcher list to
// origin. Supports exact / prefix / suffix matchers (the three matcher
// variants used by reference Envoy's CORS filter examples).
func originAllowedByPolicy(origin string, p *corsv3.CorsPolicy) bool {
	for _, m := range p.GetAllowOriginStringMatch() {
		if m.GetExact() == origin {
			return true
		}
		if pre := m.GetPrefix(); pre != "" && strings.HasPrefix(origin, pre) {
			return true
		}
		if suf := m.GetSuffix(); suf != "" && strings.HasSuffix(origin, suf) {
			return true
		}
	}
	return false
}

// getMethod extracts the request method from the headers map.
//
// Convention: HCM dispatch (both H1 connection.go and H2 h2dispatch.go) is
// modified at Task 18 to inject the request method into the headers map under
// the ":method" pseudo-header key BEFORE invoking RunDecodeHeaders. This lets
// chain-level filters (cors, envoygotest) read the method without needing
// codec-specific surfacing. Per SPEC §11.2 the cors filter requires method
// inspection to discriminate preflight (OPTIONS) from actual requests.
func getMethod(headers http.Header) string {
	return headers.Get(":method")
}
