package thriftproxy

import (
	"errors"

	thrift_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
)

const (
	errRouteMatchRequired    = "thrift_proxy: route_config.routes[].match is required"
	errRouteActionRequired   = "thrift_proxy: route_config.routes[].route is required"
	errRouteClusterRequired  = "thrift_proxy: route_config.routes[].route.cluster is required"
	errRouteMatchUnsupported = "thrift_proxy: only method_name route matches are supported"
)

// routeEntry is one method_name→cluster route. An empty methodName is match-all.
type routeEntry struct {
	methodName string
	cluster    string
}

// routeTable is the ordered method-routing table (SPEC §3.2). match() returns the
// FIRST entry whose methodName equals the request method, else the FIRST match-all
// (empty methodName) entry, else ("",false). First-match ordering mirrors the
// reference's sequential route scan.
type routeTable struct {
	entries []routeEntry
}

func parseRouteConfig(rc *thrift_proxyv3.RouteConfiguration) (*routeTable, error) {
	rt := &routeTable{}
	if rc == nil {
		return rt, nil // absent route_config → all-miss (AMEND-T7)
	}
	for _, r := range rc.GetRoutes() {
		m := r.GetMatch()
		if m == nil {
			return nil, errors.New(errRouteMatchRequired)
		}
		mn, ok := m.GetMatchSpecifier().(*thrift_proxyv3.RouteMatch_MethodName)
		if !ok {
			return nil, errors.New(errRouteMatchUnsupported) // service_name/headers deferred
		}
		a := r.GetRoute()
		if a == nil {
			return nil, errors.New(errRouteActionRequired)
		}
		cl, ok := a.GetClusterSpecifier().(*thrift_proxyv3.RouteAction_Cluster)
		if !ok || cl.Cluster == "" {
			return nil, errors.New(errRouteClusterRequired)
		}
		rt.entries = append(rt.entries, routeEntry{methodName: mn.MethodName, cluster: cl.Cluster})
	}
	return rt, nil
}

// match returns the cluster for method: an exact methodName match first, then a
// match-all (empty methodName) entry. First-match wins within each tier.
func (rt *routeTable) match(method string) (string, bool) {
	var fallback string
	haveFallback := false
	for _, e := range rt.entries {
		if e.methodName == method {
			return e.cluster, true
		}
		if e.methodName == "" && !haveFallback {
			fallback, haveFallback = e.cluster, true
		}
	}
	if haveFallback {
		return fallback, true
	}
	return "", false
}
