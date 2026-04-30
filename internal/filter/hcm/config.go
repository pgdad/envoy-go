package hcm

import (
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/stats"
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

// ListenerCtx carries listener-side context the HCM filter constructor uses
// at build time. Phase 05.1 added this for the --allow-h2c flag plumbing
// (per ADR-0049 + ADR-0050). Future phases may extend.
type ListenerCtx struct {
	HasTLS   bool
	AllowH2C bool
}

// Filter is the per-listener HTTP connection manager. It owns the resolved
// route table, the cluster manager handle, and the configured stat_prefix
// (forward-look for phase 06 stats per ADR-0041). NewFilter and Handle are
// declared in filter.go (Task 8).
//
// 06.1 Task 11: Filter gains 5 HCM-scope per-instance metric pointers per
// SPEC §6 ("HCM — 5 names"). Allocated by NewFilter from the supplied
// *stats.Registry at filter-build time (pre-Freeze, per SPEC §5.4 boot
// ordering); incremented from the H1 connection.go and H2 h2dispatch.go
// hot paths per SPEC §5.5.
type Filter struct {
	table      *routeTable
	clusters   *cluster.Manager
	statPrefix string
	codecType  hcmv3.HttpConnectionManager_CodecType

	// 06.1 metric fields (per SPEC §6 — HCM-scope; 5 metrics per HCM
	// instance). Allocated by NewFilter at build time; pre-Freeze.
	downstreamRqTotal *stats.Counter
	downstreamRq2xx   *stats.Counter
	downstreamRq3xx   *stats.Counter
	downstreamRq4xx   *stats.Counter
	downstreamRq5xx   *stats.Counter

	// accessLog holds the configured access-log sinks. Nil when no sinks are
	// configured (pre-Task 14) or for listeners without access_log[] entries.
	// Plumbed via parseFilterWithCtx; Task 14 wires real AsyncFileSinks.
	accessLog []accesslog.Sink
}

// downstreamStatusClassCounter returns the downstream_rq_<Nxx> counter for the
// given HTTP status code per the integer-divide code/100 discipline (Rule SN4
// of SPEC §10.1). Returns nil for codes outside [200, 599] (1xx informational
// responses are NOT bucketed per SPEC §2.1; status-class counters cover only
// the response-terminating range). Mirrors (*Cluster).statusClassCounter from
// Task 8.
func (f *Filter) downstreamStatusClassCounter(code int) *stats.Counter {
	switch code / 100 {
	case 2:
		return f.downstreamRq2xx
	case 3:
		return f.downstreamRq3xx
	case 4:
		return f.downstreamRq4xx
	case 5:
		return f.downstreamRq5xx
	default:
		return nil
	}
}

// NewFilterWithCtx is the phase-05.1 constructor variant. The existing
// NewFilter delegates with the zero-value ListenerCtx (allowH2C=false,
// hasTLS=false), preserving phase-04 semantics.
//
// Phase 06.1 Task 11 widened the signature with a trailing *stats.Registry;
// the constructor allocates the 5 HCM-scope per-instance metrics on the
// supplied Registry (pre-Freeze; SPEC §5.4 + §6).
func NewFilterWithCtx(tc *anypb.Any, clusters *cluster.Manager, lc ListenerCtx, registry *stats.Registry) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, lc, registry, nil)
}

// parseFilter is the legacy entry point retained for existing tests.
// It delegates to parseFilterWithCtx with a zero-value ListenerCtx and a
// fresh throwaway Registry (legacy callers do not exercise the metric
// pointers).
func parseFilter(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, ListenerCtx{}, stats.NewRegistry(), nil)
}

// parseFilterWithCtx decodes the typed_config Any into a *Filter. All errors
// begin with "hcm: ". See ADR-0040 (HTTP-filter framework subset), ADR-0041
// (stat_prefix + ignored-set), ADR-0042 (HTTP-filter chain shape), ADR-0038
// (route match subset), ADR-0050 (ALPN dispatch), and SPEC §2/§9.
//
// Task 11: registry is the *stats.Registry the 5 HCM-scope per-instance
// metrics are allocated on. Must be non-nil and non-Frozen.
func parseFilterWithCtx(tc *anypb.Any, clusters *cluster.Manager, lc ListenerCtx, registry *stats.Registry, accessLogSinks []accesslog.Sink) (*Filter, error) {
	if got := tc.GetTypeUrl(); got != TypeURL {
		return nil, fmt.Errorf("hcm: wrong type_url %q (want %q)", got, TypeURL)
	}
	msg := &hcmv3.HttpConnectionManager{}
	if err := tc.UnmarshalTo(msg); err != nil {
		return nil, fmt.Errorf("hcm: unmarshal: %w", err)
	}

	codecType := msg.GetCodecType()
	switch codecType {
	case hcmv3.HttpConnectionManager_HTTP1, hcmv3.HttpConnectionManager_AUTO:
		// ok — H1 or ALPN-driven
	case hcmv3.HttpConnectionManager_HTTP2:
		if !lc.HasTLS && !lc.AllowH2C {
			return nil, fmt.Errorf("hcm: codec_type HTTP2 requires TLS transport_socket (or --allow-h2c for conformance testing)")
		}
	default:
		return nil, fmt.Errorf("hcm: codec_type %s is not supported in phase 05.1", codecType)
	}

	statPrefix := msg.GetStatPrefix()
	if statPrefix == "" {
		return nil, fmt.Errorf("hcm: stat_prefix is required")
	}
	// Validate the assembled metric-name shape before any registry write.
	// stat_prefix is propagated into "http.<stat_prefix>.<metric>" counter
	// names at the bottom of this function; if it contains characters outside
	// the metric-name regex's permitted [a-zA-Z0-9_.] class (or otherwise
	// produces an invalid assembled name), Registry.NewCounter would panic
	// per ADR-0059's boot-time panic discipline. We reject at the user-input
	// boundary instead so the failure surfaces as a hcm:-prefixed error per
	// the FuzzHCMConfigParse contract.
	if !stats.IsValidName("http." + statPrefix + ".downstream_rq_total") {
		return nil, fmt.Errorf("hcm: invalid stat_prefix: %q (must contain only ASCII letters, digits, underscore, or dot, and form a valid metric-name segment)", statPrefix)
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

	prefix := "http." + statPrefix + "."
	f := &Filter{
		table:             table,
		clusters:          clusters,
		statPrefix:        statPrefix,
		codecType:         codecType,
		downstreamRqTotal: registry.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   registry.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   registry.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   registry.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   registry.NewCounter(prefix + "downstream_rq_5xx"),
		accessLog:         accessLogSinks,
	}
	// Task 12: bind the filter backpointer on every action so emit-deferral
	// sites in directResponseAction.do, routerAction.do, and routerActionH2.doH2
	// can call f.emitAccessLog / f.emitAccessLogH2. The actions are built before
	// the Filter exists (buildRouteTable is called above), so this post-build
	// step completes the wiring.
	table.bindFilter(f)
	return f, nil
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

// buildRouterAction returns a routeAction satisfying the codec-neutral shape.
// Phase 05.2: when the resolved cluster's UseH2() reports true, the action
// variant is *routerActionH2 (per SPEC §5.5 + §4.1); otherwise the existing
// *routerAction (H1). Both satisfy the routeAction interface — the H2 variant
// via a defensive 500 stub (never reached on H1 path in well-formed bootstraps;
// see actions.go:routerActionH2.do) so the route-table machinery stays
// codec-neutral.
func buildRouterAction(r *routev3.RouteAction, clusters *cluster.Manager) (routeAction, error) {
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
	if c.UseH2() {
		return &routerActionH2{cluster: c}, nil
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
	return &directResponseAction{status: int(d.Status), bodyText: is.InlineString}, nil
}
