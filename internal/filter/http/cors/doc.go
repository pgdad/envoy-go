// Package cors implements the envoy.filters.http.cors HTTP filter.
//
// Phase 07.1 Task 18: real Envoy filter, wire-shape pinned by SPEC §11.2
// empirical scrape of reference Envoy v1.37.2.
//
// Decode side: detect preflight (OPTIONS + Origin + Access-Control-Request-
// Method); on allowed origin, SendLocalReply(200, "", corsHeaders) emits the
// six CORS preflight headers in the verbatim §11.2 order:
//
//	access-control-allow-origin
//	access-control-allow-credentials
//	access-control-allow-methods
//	access-control-allow-headers
//	access-control-max-age
//	access-control-expose-headers
//
// Disallowed-origin preflight passes through to the router (which 405s the
// OPTIONS request). The cors filter does NOT synthesize a 4xx local reply on
// disallowed origin — this matches v1.37.2's empirical behavior.
//
// Encode side (actual non-preflight request): if the request had an Origin
// matching the allow-list, the filter injects three CORS response headers on
// the upstream response:
//
//	access-control-allow-origin
//	access-control-allow-credentials
//	access-control-expose-headers
//
// (allow-methods / allow-headers / max-age are preflight-only.) No-op
// otherwise (no Origin / disallowed Origin → no header injection).
//
// Per-route policy: resolved via DecoderFilterCallbacks.RequestRouteConfig()
// which returns the merged *corsv3.CorsPolicy from the perRouteConfig 3-tier
// merge (Route > VirtualHost > RouteConfiguration; ADR-0073).
//
// References:
//   - SPEC §11.2 (verbatim wire-shape pin)
//   - ADR-0074 (filter set: cors + envoy_go_test)
//   - ADR-0075 (SendLocalReply enters encode chain at filter[len-1])
package cors
