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
func BuildPerRouteConfig(rcCfg map[string]*anypb.Any, scopes []routeScope, chainNames []string) (*PerRouteConfig, error) {
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
		vh, err := parseMap(s.VHost, fmt.Sprintf("route_config.virtual_hosts[%d]", i))
		if err != nil {
			return nil, err
		}
		rt, err := parseMap(s.Route, fmt.Sprintf("route_config.virtual_hosts[%d].routes[%d]", i, i))
		if err != nil {
			return nil, err
		}
		out.scopes[i] = scopeParsed{vhost: vh, route: rt}
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
