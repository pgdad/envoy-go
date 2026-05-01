package hcm

import (
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/stats"
)

const (
	// TypeURL is the proto descriptor URL for HttpConnectionManager. Registered
	// in internal/listener/manager.go's filterRegistry alongside tcpproxy.TypeURL
	// (Task 8).
	TypeURL = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"

	// routerFilterName is the canonical Envoy name for the router HTTP filter.
	// SPEC §4.1 and ADR-0040 (now superseded by ADR-0071) require it as the
	// terminal entry in http_filters[]. Per ADR-0071 the chain may contain
	// additional non-terminal filter entries before the router (cors, probe).
	routerFilterName = "envoy.filters.http.router"
)

// chainEntry is one (name, factory) pair in the HCM Filter's resolved
// http_filters[] chain. The factory is the per-instance allocator returned by
// HTTPFilterFactory at HCM-build time per ADR-0071's two-step factory pattern;
// it is invoked once per request to allocate a fresh filter instance bound to
// the parsed config. Task 13 introduces this type; Task 15 (H1 dispatch) and
// Task 16 (H2 dispatch) consume it via FilterChain construction.
type chainEntry struct {
	name    string
	factory filter_http.FilterInstanceFactory
}

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

	// chainConfig is the resolved http_filters[] chain in declaration order.
	// Populated by parseFilterWithCtx (Task 13); consumed by H1/H2 dispatch
	// (Tasks 15/16) which calls each entry's factory once per request to
	// allocate a fresh per-stream filter instance and assembles them into a
	// *filter_http.FilterChain. Per ADR-0071's two-step factory pattern.
	//
	// Pre-emptively added in Task 13 (PLAN's Task 14 Step 1 specifies these
	// fields land in filter.go / Task 14, but Task 13 is the parser side that
	// populates them — adding the field here lets the parser write into the
	// *Filter directly without an intermediate tuple-return refactor at
	// Task 14). See PROGRESS Task 13 PLAN-deviation note.
	chainConfig []chainEntry

	// perRouteConfig holds the parsed-and-validated typed_per_filter_config
	// tree from RouteConfiguration / VirtualHost / Route scopes (per ADR-0073's
	// 3-tier merge model). nil if no typed_per_filter_config is present
	// anywhere. Populated by parseFilterWithCtx; consumed by H1/H2 dispatch
	// at filter-instantiation time via FilterChain.SetRequestCtx + the chain's
	// internal Resolve cache.
	perRouteConfig *filter_http.PerRouteConfig
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
//
// Task 13 transitional: this legacy constructor builds a default router-only
// HTTPRegistry so the http_filters[] chain validates clean for callers that
// have not yet been swept to the registry-aware constructor (Task 14 sweeps
// all callers and DELETES this function).
func NewFilterWithCtx(tc *anypb.Any, clusters *cluster.Manager, lc ListenerCtx, registry *stats.Registry) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, lc, registry, nil, defaultRouterOnlyHTTPRegistry())
}

// NewFilterWithCtxAndSinks is the phase-06.2 constructor variant. It extends
// NewFilterWithCtx with an accessLogSinks slice so that cmd/envoy-go/main.go
// can thread the opened AsyncFileSinks through to the HCM filter at build
// time. A nil or empty slice is treated as "no access logging" and is safe
// to pass — NewFilterWithCtx delegates here with nil.
//
// Task 13 transitional: see NewFilterWithCtx note re: default registry.
func NewFilterWithCtxAndSinks(tc *anypb.Any, clusters *cluster.Manager, lc ListenerCtx, registry *stats.Registry, accessLogSinks []accesslog.Sink) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, lc, registry, accessLogSinks, defaultRouterOnlyHTTPRegistry())
}

// parseFilter is the legacy entry point retained for existing tests.
// It delegates to parseFilterWithCtx with a zero-value ListenerCtx and a
// fresh throwaway Registry (legacy callers do not exercise the metric
// pointers).
//
// Task 13 transitional: see NewFilterWithCtx note re: default registry.
func parseFilter(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, ListenerCtx{}, stats.NewRegistry(), nil, defaultRouterOnlyHTTPRegistry())
}

// defaultRouterOnlyHTTPRegistry returns a freshly-allocated, frozen
// *HTTPRegistry containing only the router filter (envoy.filters.http.router
// → router.New). Used by the Task 13 transitional legacy constructors so
// callers that have not yet been swept to the registry-aware constructor
// (Task 14 sweep) still produce a chain that validates clean against the
// four chain-shape rules. Task 14 deletes the legacy constructors and this
// helper along with them.
func defaultRouterOnlyHTTPRegistry() *filter_http.HTTPRegistry {
	r := filter_http.NewHTTPRegistry()
	r.Register(router.TypeURL, router.New)
	r.Freeze()
	return r
}

// parseFilterWithCtx decodes the typed_config Any into a *Filter. All errors
// begin with "hcm: ". See ADR-0040 (HTTP-filter framework subset; superseded
// by ADR-0071), ADR-0041 (stat_prefix + ignored-set), ADR-0042 (HTTP-filter
// chain shape; partially superseded by ADR-0071), ADR-0071 (chain-shape
// terminal-router rule + factory pattern), ADR-0072 (HTTPRegistry threaded
// constructor map), ADR-0073 (typed_per_filter_config 3-tier merge),
// ADR-0038 (route match subset), ADR-0050 (ALPN dispatch), and SPEC §2/§9.
//
// Task 11: registry is the *stats.Registry the 5 HCM-scope per-instance
// metrics are allocated on. Must be non-nil and non-Frozen.
//
// Task 13: httpRegistry is the *filter_http.HTTPRegistry the parser uses to
// resolve each http_filters[] entry's typed_config.type_url to a per-instance
// factory closure; it must be non-nil. Must be Frozen at call time per
// ADR-0072 (boot-time-populated, freeze-after-boot). The four chain-shape
// rules per SPEC §1 #6 + ADR-0071's partial supersession of ADR-0042 are
// applied via filter_http.ValidateChainShape; on success the per-entry
// HTTPFilterFactory is invoked with the typed_config Any to allocate the
// FilterInstanceFactory closure stored on Filter.chainConfig.
func parseFilterWithCtx(tc *anypb.Any, clusters *cluster.Manager, lc ListenerCtx, registry *stats.Registry, accessLogSinks []accesslog.Sink, httpRegistry *filter_http.HTTPRegistry) (*Filter, error) {
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

	chainConfig, err := parseHTTPFiltersChain(msg.GetHttpFilters(), httpRegistry)
	if err != nil {
		return nil, err
	}

	chainNames := make([]string, len(chainConfig))
	for i, e := range chainConfig {
		chainNames[i] = e.name
	}
	perRoute, err := buildPerRouteFromHCM(rc, chainNames)
	if err != nil {
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
		chainConfig:       chainConfig,
		perRouteConfig:    perRoute,
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

// parseHTTPFiltersChain walks the http_filters[] slice in declaration order
// and returns the resolved []chainEntry. The four chain-shape rules per
// SPEC §1 #6 + ADR-0071's partial supersession of ADR-0042 are delegated to
// filter_http.ValidateChainShape:
//
//  1. Empty chain                       → "hcm: http_filters: must contain at least 1 entry (the router)"
//  2. Last entry not router             → "hcm: http_filters: last entry must be %q (router); got %q (%s)"
//  3. Duplicate filter name             → "hcm: http_filters: duplicate filter name %q"
//  4. Unknown typed_config.type_url     → "hcm: http_filters[i]: unknown type_url %q (registry: known are %v)"
//
// Per ADR-0071 the per-entry typed_config Any is then handed to the resolved
// HTTPFilterFactory, which parses + validates the typed_config once and
// returns a per-request FilterInstanceFactory closure stored on the
// chainEntry. Factory errors are wrapped with the http_filters[i] prefix.
func parseHTTPFiltersChain(filters []*hcmv3.HttpFilter, httpRegistry *filter_http.HTTPRegistry) ([]chainEntry, error) {
	// Build the (name, type_url) entries for ValidateChainShape. Defensive
	// nil-typed_config handling: the empty-string TypeURL never matches a
	// registered factory, so the rule-#4 branch fires with a clear message.
	entries := make([]filter_http.ChainShapeEntry, len(filters))
	for i, f := range filters {
		var tu string
		if tc, ok := f.GetConfigType().(*hcmv3.HttpFilter_TypedConfig); ok && tc.TypedConfig != nil {
			tu = tc.TypedConfig.GetTypeUrl()
		}
		entries[i] = filter_http.ChainShapeEntry{Name: f.GetName(), TypeURL: tu}
	}
	factories, err := filter_http.ValidateChainShape(entries, httpRegistry, routerFilterName, router.TypeURL)
	if err != nil {
		return nil, err
	}
	// Walk a second time to invoke each factory with its typed_config Any,
	// accumulating per-instance factory closures.
	chainConfig := make([]chainEntry, 0, len(filters))
	for i, f := range filters {
		var tcAny *anypb.Any
		if tc, ok := f.GetConfigType().(*hcmv3.HttpFilter_TypedConfig); ok {
			tcAny = tc.TypedConfig
		}
		instanceFactory, err := factories[i](tcAny, filter_http.FactoryCtx{Registry: httpRegistry})
		if err != nil {
			return nil, fmt.Errorf("hcm: http_filters[%d]: factory: %w", i, err)
		}
		chainConfig = append(chainConfig, chainEntry{name: f.GetName(), factory: instanceFactory})
	}
	return chainConfig, nil
}

// buildPerRouteFromHCM extracts typed_per_filter_config maps from the parsed
// RouteConfiguration / VirtualHost / Route levels and runs them through
// filter_http.BuildPerRouteConfig (per ADR-0073's 3-tier merge model). The
// downstream validator rejects any map key that is not a filter name in
// chainNames (the resolved chain). Returns nil when no maps are present at
// any level — short-circuits the gratuitous PerRouteConfig allocation that
// the common phase-04..06.2 "no typed_per_filter_config in any scope" path
// would otherwise produce.
func buildPerRouteFromHCM(rc *routev3.RouteConfiguration, chainNames []string) (*filter_http.PerRouteConfig, error) {
	rcMap := rc.GetTypedPerFilterConfig()
	// Phase-04 enforces exactly one virtual_host with domains=["*"]
	// (validated above); we still loop generally over routes inside that
	// one vhost so the per-route scopes vector is well-shaped for ADR-0073's
	// 3-tier merge cache.
	var scopes []filter_http.RouteScope
	for _, vh := range rc.GetVirtualHosts() {
		vhMap := vh.GetTypedPerFilterConfig()
		for _, r := range vh.GetRoutes() {
			scopes = append(scopes, filter_http.RouteScope{VHost: vhMap, Route: r.GetTypedPerFilterConfig()})
		}
	}
	hasAny := len(rcMap) > 0
	if !hasAny {
		for _, s := range scopes {
			if len(s.VHost) > 0 || len(s.Route) > 0 {
				hasAny = true
				break
			}
		}
	}
	if !hasAny {
		return nil, nil
	}
	return filter_http.BuildPerRouteConfig(rcMap, scopes, chainNames)
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
