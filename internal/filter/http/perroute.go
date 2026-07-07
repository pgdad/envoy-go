package http

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// RouteScope carries the typed_per_filter_config maps from a single Route +
// its containing VirtualHost. The PerRouteConfig holds one RouteScope per
// matched route + the RouteConfiguration-level map.
//
// Exported (Task 13) so the HCM config parser in internal/filter/hcm can
// construct one without a constructor-call dance. Pre-Task-13 this type was
// package-private (named routeScope); the routeScope alias below preserves
// the existing fuzzer + perroute_test.go bodies verbatim through the Task 13
// cycle (Task 14 sweeps the lowercase references).
type RouteScope struct {
	VHost map[string]*anypb.Any // typed_per_filter_config from the containing virtual_host
	Route map[string]*anypb.Any // typed_per_filter_config from the route itself

	// VHostIndex + RouteIndex are the scope's coordinates within the
	// RouteConfiguration: the index of the containing virtual_host, and the
	// index of the route within that virtual_host. Consumed only by the
	// envoy-go-internal boot-error location strings in BuildPerRouteConfig
	// (scopes are one-per-route, so the flat scope index previously stood in
	// for BOTH coordinates and misreported the location for any config with
	// more than one route per vhost). Zero-value construction (tests) yields
	// virtual_hosts[0].routes[0], matching the single-scope shape.
	VHostIndex int
	RouteIndex int
}

// routeScope is a transitional alias preserved so the fuzzer's internal seed
// body and perroute_test.go can keep using the lowercase form during the
// Task 13 cycle. Removed in Task 14 when the test bodies are swept.
type routeScope = RouteScope

// PerRouteConfig is the parsed-and-validated per-route config tree, built
// once at HCM-build time. Resolve performs the merge + unmarshal at
// request-time with a lazy cache.
type PerRouteConfig struct {
	rc     map[string]proto.Message   // RouteConfiguration-scope, parsed
	scopes []scopeParsed              // one per route, parsed
	mu     sync.Mutex                 // guards cache
	cache  map[cacheKey]proto.Message // (filterName, routeIdx) → resolved merge
}

type scopeParsed struct {
	vhost map[string]proto.Message
	route map[string]proto.Message

	// vhostIdx + routeIdx carry the RouteScope coordinates through to the
	// per-route-validator error wrappers below.
	vhostIdx int
	routeIdx int
}

type cacheKey struct {
	filterName string
	routeIdx   int
}

// BuildPerRouteConfig parses each scope's typed_per_filter_config map (Anypb
// blobs) into proto.Message values, validating that all keys reference filter
// names present in chainNames. Returns an error with the SPEC §4.4 + §13.1
// canonical message on unknown filter names.
//
// Most-specific override on Resolve: Route > VirtualHost > RouteConfiguration.
// No field-merge per ADR-0073.
//
// The reg parameter is optional (may be nil). When non-nil, BuildPerRouteConfig
// consults reg.PerRouteValidator(filterName) for each filter name in chainNames
// and runs the validator on each parsed proto.Message at each tier, surfacing
// per-route protected-header violations as boot-time errors per ADR-0110 +
// ADR-0111 + planner-time decision 3.
func BuildPerRouteConfig(rcCfg map[string]*anypb.Any, scopes []routeScope, chainNames []string, reg *HTTPRegistry) (*PerRouteConfig, error) {
	chainSet := make(map[string]struct{}, len(chainNames))
	for _, n := range chainNames {
		chainSet[n] = struct{}{}
	}
	out := &PerRouteConfig{cache: make(map[cacheKey]proto.Message)}
	parseMap := func(in map[string]*anypb.Any, location string) (map[string]proto.Message, error) {
		if in == nil {
			return nil, nil
		}
		m := make(map[string]proto.Message, len(in))
		for k, a := range in {
			if _, ok := chainSet[k]; !ok {
				return nil, fmt.Errorf("hcm: %s: typed_per_filter_config: unknown filter name %q (chain has %v)", location, k, chainNames)
			}
			msg, err := a.UnmarshalNew()
			if err != nil {
				return nil, fmt.Errorf("hcm: %s: typed_per_filter_config[%q]: unmarshal: %w", location, k, err)
			}
			m[k] = msg
		}
		return m, nil
	}
	var err error
	out.rc, err = parseMap(rcCfg, "route_config")
	if err != nil {
		return nil, err
	}
	out.scopes = make([]scopeParsed, len(scopes))
	for i, s := range scopes {
		vh, err := parseMap(s.VHost, fmt.Sprintf("route_config.virtual_hosts[%d]", s.VHostIndex))
		if err != nil {
			return nil, err
		}
		rt, err := parseMap(s.Route, fmt.Sprintf("route_config.virtual_hosts[%d].routes[%d]", s.VHostIndex, s.RouteIndex))
		if err != nil {
			return nil, err
		}
		out.scopes[i] = scopeParsed{vhost: vh, route: rt, vhostIdx: s.VHostIndex, routeIdx: s.RouteIndex}
	}
	// Per-route validation hook per ADR-0110 + planner-time decision 3:
	// for each filter name in chainNames that has a registered validator,
	// run the validator against each parsed proto.Message at each tier.
	// Surfaces per-route protected-header violations (per ADR-0111) as
	// boot-time errors mirroring Envoy v1.37.2's CONFIG-LOAD-TIME
	// enforcement per phase 10 SPEC §11.1.
	if reg != nil {
		for _, name := range chainNames {
			v := reg.PerRouteValidator(name)
			if v == nil {
				continue
			}
			if msg, ok := out.rc[name]; ok {
				if err := v(msg); err != nil {
					return nil, fmt.Errorf("hcm: route_config: typed_per_filter_config[%q]: %w", name, err)
				}
			}
			for _, sp := range out.scopes {
				if msg, ok := sp.vhost[name]; ok {
					if err := v(msg); err != nil {
						return nil, fmt.Errorf("hcm: route_config.virtual_hosts[%d]: typed_per_filter_config[%q]: %w", sp.vhostIdx, name, err)
					}
				}
				if msg, ok := sp.route[name]; ok {
					if err := v(msg); err != nil {
						return nil, fmt.Errorf("hcm: route_config.virtual_hosts[%d].routes[%d]: typed_per_filter_config[%q]: %w", sp.vhostIdx, sp.routeIdx, name, err)
					}
				}
			}
		}
	}
	return out, nil
}

// Resolve returns the merged proto.Message for filterName at routeIdx (the
// matched-route index in the chain's per-stream state). Cache-on-first-lookup;
// returns nil if no scope carries a config for filterName.
func (p *PerRouteConfig) Resolve(filterName string, routeIdx int) proto.Message {
	if p == nil {
		return nil
	}
	key := cacheKey{filterName: filterName, routeIdx: routeIdx}
	p.mu.Lock()
	defer p.mu.Unlock()
	if m, ok := p.cache[key]; ok {
		return m
	}
	var msg proto.Message
	if routeIdx >= 0 && routeIdx < len(p.scopes) {
		if m, ok := p.scopes[routeIdx].route[filterName]; ok {
			msg = m
		} else if m, ok := p.scopes[routeIdx].vhost[filterName]; ok {
			msg = m
		}
	}
	if msg == nil {
		if m, ok := p.rc[filterName]; ok {
			msg = m
		}
	}
	p.cache[key] = msg
	return msg
}

// ResolveAllTiers returns the parsed per-route config at each tier, unmerged.
// Tiers are returned in canonical proto order: Route (most specific),
// VirtualHost (intermediate), RouteConfiguration (least specific). A tier
// with no config for filterName at the matched route is nil.
//
// Used by filters whose semantics require multi-tier evaluation rather than
// most-specific override (e.g., envoy.filters.http.header_mutation per its
// most_specific_header_mutations_wins flag — see ADR-0110). The default
// Resolve method (per ADR-0073) remains the canonical accessor for filters
// that use most-specific override (cors, fault).
//
// NOTE: ResolveAllTiers does NOT consult or pollute the existing p.cache
// (which is keyed by (filterName, routeIdx) returning a single proto.Message;
// multi-tier returns 3 messages with different cache shape). The map reads
// (p.scopes[routeIdx].route, p.scopes[routeIdx].vhost, p.rc) are sub-microsecond;
// per-request re-reads are acceptable per phase 10 PLAN planner-time decision 2.
func (p *PerRouteConfig) ResolveAllTiers(filterName string, routeIdx int) (route, vhost, rc proto.Message) {
	if p == nil {
		return nil, nil, nil
	}
	if routeIdx >= 0 && routeIdx < len(p.scopes) {
		if m, ok := p.scopes[routeIdx].route[filterName]; ok {
			route = m
		}
		if m, ok := p.scopes[routeIdx].vhost[filterName]; ok {
			vhost = m
		}
	}
	if m, ok := p.rc[filterName]; ok {
		rc = m
	}
	return route, vhost, rc
}
