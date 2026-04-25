// Package hcm parses Envoy v3 HttpConnectionManager protos from a network
// filter's typed_config Any, validates a router-only HTTP-filter chain,
// resolves an inline route_config's single virtual_host's routes into an
// in-memory route table supporting match.prefix and match.path predicates,
// and dispatches matched requests through direct_response (synthesized local
// reply) or route (router action — dialing the named cluster via
// Cluster.Dial(ctx) per request, fresh).
//
// Phase 04 surface: see docs/envoy-go/phases/04-http-1.1/SPEC.md §4.1. Doctrine:
// see docs/envoy-go/DECISIONS.md ADR-0037 (HTTP/1.1 wire codec source: stdlib
// net/http), ADR-0038 (route match subset: prefix + path), ADR-0039 (per-request
// fresh upstream dial), ADR-0040 (HTTP-filter framework subset),
// ADR-0041 (stat_prefix + ignored-set), ADR-0042 (HTTP-filter chain shape
// [router]), ADR-0044 (BEHAVIOR_CONTRACT HTTP/1.1 subsection).
//
// Error-prefix discipline: every error returned by this package begins with
// "hcm: ". Errors crossing the listener-manager boundary are further wrapped
// with "listener: %q: filter_chains[%d]: " by the caller (see
// internal/listener/manager.go).
//
// What this package does NOT do: it does NOT use net/http.Server, does NOT
// use the http.Handler interface, and does NOT call ServeHTTP. The connection
// loop is driven explicitly under Filter.Handle. See doctrine D-3.2.
package hcm
