package hcm

import (
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

const (
	// TypeURL is the proto descriptor URL for HttpConnectionManager. Registered
	// in internal/listener/manager.go's filterRegistry alongside tcpproxy.TypeURL
	// (Task 8).
	TypeURL = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"

	// routerFilterName is the canonical Envoy name for the router HTTP filter.
	// SPEC §4.1 and ADR-0040 require it as the only permitted http_filters entry.
	routerFilterName = "envoy.filters.http.router"

	// routerTypeURL is the proto descriptor URL for the Router HTTP filter.
	routerTypeURL = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
)

// Filter is the per-listener HTTP connection manager. It owns the resolved
// route table, the cluster manager handle, and the configured stat_prefix
// (forward-look for phase 06 stats per ADR-0041). NewFilter and Handle are
// declared in filter.go (Task 8).
type Filter struct {
	table      *routeTable
	clusters   *cluster.Manager
	statPrefix string
}

// parseFilter decodes the typed_config Any into a *Filter. All errors begin
// with "hcm: ". See ADR-0040 (HTTP-filter framework subset), ADR-0041
// (stat_prefix + ignored-set), ADR-0042 (HTTP-filter chain shape), ADR-0038
// (route match subset), and SPEC §2/§9.
func parseFilter(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error) {
	if got := tc.GetTypeUrl(); got != TypeURL {
		return nil, fmt.Errorf("hcm: wrong type_url %q (want %q)", got, TypeURL)
	}
	msg := &hcmv3.HttpConnectionManager{}
	if err := tc.UnmarshalTo(msg); err != nil {
		return nil, fmt.Errorf("hcm: unmarshal: %w", err)
	}

	switch msg.GetCodecType() {
	case hcmv3.HttpConnectionManager_HTTP1, hcmv3.HttpConnectionManager_AUTO:
		// ok
	default:
		return nil, fmt.Errorf("hcm: codec_type %s is not supported in phase 04 (HTTP/1.1 only)", msg.GetCodecType())
	}

	statPrefix := msg.GetStatPrefix()
	if statPrefix == "" {
		return nil, fmt.Errorf("hcm: stat_prefix is required")
	}

	rc, err := requireInlineRouteConfig(msg)
	if err != nil {
		return nil, err
	}

	if got := len(rc.GetVirtualHosts()); got != 1 {
		return nil, fmt.Errorf("hcm: route_config: virtual_hosts: got %d, want exactly 1", got)
	}
	vh := rc.GetVirtualHosts()[0]
	if domains := vh.GetDomains(); len(domains) != 1 || domains[0] != "*" {
		return nil, fmt.Errorf("hcm: route_config: virtual_hosts[0]: domains: got %v, want [\"*\"]", domains)
	}

	if err := requireRouterOnlyHTTPFilters(msg.GetHttpFilters()); err != nil {
		return nil, err
	}

	table, err := buildRouteTable(vh.GetRoutes(), clusters)
	if err != nil {
		return nil, err
	}

	return &Filter{table: table, clusters: clusters, statPrefix: statPrefix}, nil
}

func requireInlineRouteConfig(msg *hcmv3.HttpConnectionManager) (*routev3.RouteConfiguration, error) {
	switch rs := msg.GetRouteSpecifier().(type) {
	case *hcmv3.HttpConnectionManager_RouteConfig:
		if rs.RouteConfig == nil {
			return nil, fmt.Errorf("hcm: route_config is nil")
		}
		return rs.RouteConfig, nil
	case *hcmv3.HttpConnectionManager_Rds:
		return nil, fmt.Errorf("hcm: route_specifier=rds is not supported in phase 04")
	case *hcmv3.HttpConnectionManager_ScopedRoutes:
		return nil, fmt.Errorf("hcm: route_specifier=scoped_routes is not supported in phase 04")
	default:
		return nil, fmt.Errorf("hcm: route_specifier missing or of unsupported type %T", rs)
	}
}

func requireRouterOnlyHTTPFilters(filters []*hcmv3.HttpFilter) error {
	if len(filters) != 1 {
		return fmt.Errorf("hcm: http_filters: got %d entries, want exactly 1 (router only) per ADR-0042", len(filters))
	}
	f := filters[0]
	if f.GetName() != routerFilterName {
		return fmt.Errorf("hcm: http_filters[0]: name %q, want %q", f.GetName(), routerFilterName)
	}
	tc, ok := f.GetConfigType().(*hcmv3.HttpFilter_TypedConfig)
	if !ok || tc.TypedConfig == nil {
		return fmt.Errorf("hcm: http_filters[0]: typed_config is missing")
	}
	if got := tc.TypedConfig.GetTypeUrl(); got != routerTypeURL {
		return fmt.Errorf("hcm: http_filters[0]: typed_config type_url %q, want %q", got, routerTypeURL)
	}
	if err := tc.TypedConfig.UnmarshalTo(&routerv3.Router{}); err != nil {
		return fmt.Errorf("hcm: http_filters[0]: typed_config unmarshal: %w", err)
	}
	return nil
}

func buildRouteTable(routes []*routev3.Route, clusters *cluster.Manager) (*routeTable, error) {
	t := &routeTable{routes: make([]routeEntry, 0, len(routes))}
	for i, r := range routes {
		match, err := buildMatch(r.GetMatch())
		if err != nil {
			return nil, fmt.Errorf("hcm: route %d: %w", i, err)
		}
		action, err := buildAction(r.GetAction(), clusters)
		if err != nil {
			return nil, fmt.Errorf("hcm: route %d: %w", i, err)
		}
		t.routes = append(t.routes, routeEntry{match: match, action: action})
	}
	return t, nil
}

func buildMatch(m *routev3.RouteMatch) (routeMatch, error) {
	if m == nil {
		return nil, fmt.Errorf("match is missing")
	}
	if m.GetCaseSensitive() != nil && !m.GetCaseSensitive().GetValue() {
		return nil, fmt.Errorf("match.case_sensitive=false is not supported in phase 04")
	}
	if len(m.GetHeaders()) > 0 {
		return nil, fmt.Errorf("match.headers is not supported in phase 04")
	}
	if len(m.GetQueryParameters()) > 0 {
		return nil, fmt.Errorf("match.query_parameters is not supported in phase 04")
	}
	if m.GetRuntimeFraction() != nil {
		return nil, fmt.Errorf("match.runtime_fraction is not supported in phase 04")
	}
	if len(m.GetDynamicMetadata()) > 0 {
		return nil, fmt.Errorf("match.dynamic_metadata is not supported in phase 04")
	}
	if m.GetTlsContext() != nil {
		return nil, fmt.Errorf("match.tls_context is not supported in phase 04")
	}
	switch ps := m.GetPathSpecifier().(type) {
	case *routev3.RouteMatch_Path:
		if ps.Path == "" {
			return nil, fmt.Errorf("match.path is empty")
		}
		return matchPath(ps.Path), nil
	case *routev3.RouteMatch_Prefix:
		if ps.Prefix == "" {
			return nil, fmt.Errorf("match.prefix is empty")
		}
		return matchPrefix(ps.Prefix), nil
	case *routev3.RouteMatch_SafeRegex:
		return nil, fmt.Errorf("match.safe_regex is not supported in phase 04")
	case *routev3.RouteMatch_PathSeparatedPrefix:
		return nil, fmt.Errorf("match.path_separated_prefix is not supported in phase 04")
	case *routev3.RouteMatch_ConnectMatcher_:
		return nil, fmt.Errorf("match.connect_matcher is not supported in phase 04")
	default:
		return nil, fmt.Errorf("match.path_specifier is missing or of unsupported type %T", ps)
	}
}

func buildAction(a interface{}, clusters *cluster.Manager) (routeAction, error) {
	switch act := a.(type) {
	case *routev3.Route_Route:
		return buildRouterAction(act.Route, clusters)
	case *routev3.Route_DirectResponse:
		return buildDirectResponseAction(act.DirectResponse)
	case nil:
		return nil, fmt.Errorf("action is missing")
	default:
		return nil, fmt.Errorf("action %T is not supported in phase 04", act)
	}
}

func buildRouterAction(r *routev3.RouteAction, clusters *cluster.Manager) (*routerAction, error) {
	if r == nil {
		return nil, fmt.Errorf("route action is nil")
	}
	cs, ok := r.GetClusterSpecifier().(*routev3.RouteAction_Cluster)
	if !ok {
		return nil, fmt.Errorf("route action: cluster_specifier %T is not supported in phase 04 (literal cluster name only)", r.GetClusterSpecifier())
	}
	if cs.Cluster == "" {
		return nil, fmt.Errorf("route action: cluster name is empty")
	}
	c, ok := clusters.Get(cs.Cluster)
	if !ok {
		return nil, fmt.Errorf("route action: cluster %q not found", cs.Cluster)
	}
	return &routerAction{cluster: c}, nil
}

func buildDirectResponseAction(d *routev3.DirectResponseAction) (*directResponseAction, error) {
	if d == nil {
		return nil, fmt.Errorf("direct_response is nil")
	}
	if d.Status < 100 || d.Status >= 600 {
		return nil, fmt.Errorf("direct_response.status %d out of range [100, 599]", d.Status)
	}
	body := d.GetBody()
	if body == nil {
		return nil, fmt.Errorf("direct_response.body is required")
	}
	is, ok := body.GetSpecifier().(*corev3.DataSource_InlineString)
	if !ok {
		return nil, fmt.Errorf("direct_response.body: only inline_string is supported in phase 04 (got %T)", body.GetSpecifier())
	}
	if is.InlineString == "" {
		return nil, fmt.Errorf("direct_response.body.inline_string is empty")
	}
	return &directResponseAction{status: int(d.Status), body: is.InlineString}, nil
}
