package hcm

import (
	"net/http"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	"github.com/pgdad/envoy-go/internal/filter/http/router"
)

// routeMatch is the predicate side of a routeEntry. Each route binds exactly
// one match implementation. Phase 04 supports two: matchPrefix (bytewise
// prefix on req.URL.Path) and matchPath (case-sensitive exact equality).
// Other Envoy match shapes (safe_regex, path_separated_prefix, headers, etc.)
// are rejected at config-parse time per ADR-0038.
type routeMatch interface {
	matches(path string) bool
}

// matchPath performs a case-sensitive exact comparison on the request URL's
// Path component. Envoy's default case_sensitive=true is the only mode
// supported in phase 04.
type matchPath string

func (m matchPath) matches(p string) bool { return string(m) == p }

// matchPrefix performs a bytewise prefix match on the request URL's Path
// component. Phase 04 documents a planned divergence from Envoy's
// segment-aware prefix semantics (ADR-0038): "/api" matches "/apifoo" under
// matchPrefix; under Envoy's segment-aware semantics it would not. The
// fixture driver issues only segment-boundary paths, so the divergence is
// not surfaced in the differential gate.
type matchPrefix string

func (m matchPrefix) matches(p string) bool { return strings.HasPrefix(p, string(m)) }

// routeAction is the action half of a routeEntry. Implementations live in
// actions.go: directResponseAction (synthesizes a local reply),
// clusterRouteAction (the cluster-dial bridge to internal/filter/http/router)
// and weightedClusterRouteAction (the weighted-selection sibling).
//
// Phase 07.1 Task 15: asRouterAction() returns a router.Action closure for
// the chain-mediated dispatch path; HCM dispatch (connection.go) injects
// this closure into the terminal router filter via *Filter.SetAction
// before iteration begins. The legacy direct-call do() shape (which wrote
// wire bytes straight into a *bufio.Writer) was production-unreachable
// after Tasks 15+16 moved both codecs onto the chain-mediated path and has
// been deleted; the byte-preservation coverage now drives the Action
// closures directly.
type routeAction interface {
	asRouterAction() router.Action
	// asRouterActionH2 returns the H2-flavored router.H2Action closure for the
	// chain-mediated H2 dispatch path (Task 16). HCM h2dispatch.go injects this
	// closure into the terminal router filter via *Filter.SetH2Action before
	// chain iteration begins. Mirrors asRouterAction's role on the H1 path.
	asRouterActionH2() router.H2Action
}

// routeEntry pairs a match predicate with the action to invoke on a hit. The
// action interface is defined above; implementations live in actions.go to
// keep route.go free of any dependency on the cluster manager.
//
// Phase 24.1 Task 5 (DELTA-2 / ADR-0198): rateLimits carries the matched
// route's RAW route.v3.RateLimit policy slice as parsed from
// RouteAction.rate_limits at HCM build time. The framework retains the raw
// proto slice — descriptor-action INTERPRETATION stays filter-owned (the
// ratelimit filter's engine consumes the slice through
// DecoderFilterCallbacks.RouteRateLimits() after HCM dispatch seeds the
// per-stream chain). nil when the route has no rate_limits[] (the
// zero-regression path; chain accessor returns nil). Per ADR-0198 §Decision —
// the FIRST exposure of route-level NON-typed_per_filter_config policy data
// to a filter (distinct from RequestRouteConfig per ADR-0073). The §5.2
// strict-reject roster (disable_key / extension / dynamic_metadata) is
// applied at HCM parse-time via ratelimit.ValidateRouteRateLimits per D-RL2;
// any retained slice has already passed the validator.
type routeEntry struct {
	match      routeMatch
	action     routeAction
	rateLimits []*routev3.RateLimit
	// Phase 24.2 Task 1 (D-RL8 — RouteMetadata accessor extension; ADR-0165
	// set-once-by-dispatch + ADR-0198 DELTA-2 chain-field plumbing template).
	// metadata carries the matched route's RAW *corev3.Metadata as parsed
	// from `Route.metadata` at HCM build time. Consumed by the `ratelimit`
	// filter's `metadata` descriptor action under MetadataSource_ROUTE_ENTRY=1
	// per parent SPEC §4.1 row 8. nil when the route has no metadata
	// (zero-regression path).
	metadata *corev3.Metadata

	// Phase 24.2 Task 4 (D-RL11 — RouteIncludeVhRateLimits accessor extension;
	// ADR-0165 set-once-by-dispatch + ADR-0198 DELTA-2 chain-field plumbing
	// template). includeVhRateLimits carries the matched route's legacy
	// `RouteAction.include_vh_rate_limits` bool as parsed at HCM build time.
	// Consumed by the `ratelimit` filter's §4.3 Axis-B vhost-walk composition
	// table per parent SPEC §4.3 + AMEND-5. false (default) when the route
	// has no include_vh_rate_limits OR when the route has a direct_response
	// action (no RouteAction).
	includeVhRateLimits bool
}

// routeTable is the resolved route_config. Routes are evaluated in
// declaration order; first match wins.
//
// Phase 24.1 Task 5 (DELTA-2 / ADR-0198): vhostRateLimits carries the parent
// virtual_host's RAW route.v3.RateLimit policy slice as parsed from
// VirtualHost.rate_limits at HCM build time. envoy-go's phase-04 envelope
// requires exactly one virtual_host per RouteConfiguration, so the vhost-
// level slice has a SINGLE owning routeTable (the storage shape parallels
// the single-vhost discipline). nil when the vhost has no rate_limits[].
// Seeded onto the per-stream FilterChain at HCM dispatch via
// chain.SetVirtualHostRateLimits, exposed to filters via
// DecoderFilterCallbacks.VirtualHostRateLimits(). Per ADR-0198 §Decision.
type routeTable struct {
	routes          []routeEntry
	vhostRateLimits []*routev3.RateLimit
}

// match walks the routes in declaration order, returning the first entry
// whose match predicate accepts req.URL.Path along with its index in the
// declared route slice. Returns (nil, -1, false) on no match. Query-string
// is excluded (URL.RawQuery is not considered).
//
// Phase 07.1 Task 15: the index is propagated to FilterChain.SetRequestCtx
// so per-route config Resolve can find the matched-route's scope (per
// ADR-0073's 3-tier merge model).
func (t *routeTable) match(req *http.Request) (*routeEntry, int, bool) {
	p := req.URL.Path
	for i := range t.routes {
		if t.routes[i].match.matches(p) {
			return &t.routes[i], i, true
		}
	}
	return nil, -1, false
}
