package hcm

import (
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/drain"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/ratelimit"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/httpclient"
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
//
// Phase 18.2 Task 4 (ADR-0165): ListenerPrincipal carries the listener TLS
// server-cert principal pre-extracted by the listener manager from the
// chain's *stdtls.Config.Certificates[0] leaf cert (URI SAN[0] → DNS SAN[0]
// → Subject CN). Empty for plaintext chains. The HCM filter stores this
// string and seeds it onto every per-stream FilterChain via
// chain.SetListenerPrincipal in dispatchRequest / chainDispatchAction.WriteH2.
type ListenerCtx struct {
	HasTLS            bool
	AllowH2C          bool
	ListenerPrincipal string
	// HTTPClient is the shared `*httpclient.Client` framework primitive
	// (ADR-0177) threaded into per-filter factories via FactoryCtx.HTTPClient
	// at parseHTTPFiltersChain time. Nil-tolerant — per-consumer defaults
	// apply (jwks Fetcher allocates its own per-Fetcher default preserving
	// the phase-17-pinned 30s per-request timeout; extauthz httpAuthClient
	// does likewise). Phase 20 first-use anchor per ADR-0150 §Decision
	// AMENDMENT + ADR-0159 §Decision AMENDMENT.
	HTTPClient *httpclient.Client
}

// Filter is the per-listener HTTP connection manager. It owns the resolved
// route table, the cluster manager handle, and the configured stat_prefix
// (forward-look for phase 06 stats per ADR-0041). NewFilterWithCtxAndSinksAndRegistry
// and Handle are declared in filter.go.
//
// 06.1 Task 11: Filter gains 5 HCM-scope per-instance metric pointers per
// SPEC §6 ("HCM — 5 names"). Allocated by NewFilterWithCtxAndSinksAndRegistry
// from the supplied *stats.Registry at filter-build time (pre-Freeze, per SPEC
// §5.4 boot ordering); incremented from the H1 connection.go and H2 h2dispatch.go
// hot paths per SPEC §5.5.
type Filter struct {
	table      *routeTable
	clusters   *cluster.Manager
	statPrefix string
	codecType  hcmv3.HttpConnectionManager_CodecType

	// 06.1 metric fields (per SPEC §6 — HCM-scope; 5 metrics per HCM
	// instance). Allocated at build time; pre-Freeze.
	downstreamRqTotal *stats.Counter
	downstreamRq2xx   *stats.Counter
	downstreamRq3xx   *stats.Counter
	downstreamRq4xx   *stats.Counter
	downstreamRq5xx   *stats.Counter

	// accessLog holds the configured access-log sinks. Nil when no sinks are
	// configured or for listeners without access_log[] entries. Plumbed via
	// parseFilterWithCtx from main.go's opened AsyncFileSinks (Phase 06.2).
	accessLog []accesslog.Sink

	// dm is the central drain.Manager threaded from the listener manager.
	// Nil when no drain manager is configured (e.g. in tests that pass nil).
	// HCM calls dm.Inc() at request-begin and dm.Dec() at request-end via
	// a deferred markedInflight sentinel per ADR-0096 + ADR-0075.
	dm *drain.Manager

	// chainConfig is the resolved http_filters[] chain in declaration order.
	// Populated by parseFilterWithCtx (Task 13); consumed by H1/H2 dispatch
	// (Tasks 15/16) which calls each entry's factory once per request to
	// allocate a fresh per-stream filter instance and assembles them into a
	// *filter_http.FilterChain. Per ADR-0071's two-step factory pattern.
	//
	// Field added in Task 13 (PLAN's Task 14 Step 1 specifies these fields
	// land in filter.go / Task 14, but Task 13 is the parser side that
	// populates them — adding the field there lets the parser write into the
	// *Filter directly without an intermediate tuple-return refactor at
	// Task 14). See PROGRESS Task 13 PLAN-deviation note (i); Task 14's
	// struct-extension step is therefore a no-op.
	chainConfig []chainEntry

	// perRouteConfig holds the parsed-and-validated typed_per_filter_config
	// tree from RouteConfiguration / VirtualHost / Route scopes (per ADR-0073's
	// 3-tier merge model). nil if no typed_per_filter_config is present
	// anywhere. Populated by parseFilterWithCtx; consumed by H1/H2 dispatch
	// at filter-instantiation time via FilterChain.SetRequestCtx + the chain's
	// internal Resolve cache.
	perRouteConfig *filter_http.PerRouteConfig

	// listenerPrincipal is the TLS server-cert principal pre-extracted by the
	// listener manager at chain-build time (URI SAN[0] → DNS SAN[0] → Subject
	// CN of the chain's *stdtls.Config.Certificates[0] leaf). Empty for
	// plaintext chains. Seeded onto every per-stream FilterChain via
	// chain.SetListenerPrincipal in dispatchRequest (H1) and
	// chainDispatchAction.WriteH2 (H2) before RunDecodeHeaders dispatch.
	// Per ADR-0165 §Decision (phase-18.2 Task 4 — the ADR-0044 escape-valve
	// firing per planner-time decision D3 + D12).
	listenerPrincipal string
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
func parseFilterWithCtx(tc *anypb.Any, clusters *cluster.Manager, lc ListenerCtx, registry *stats.Registry, accessLogSinks []accesslog.Sink, httpRegistry *filter_http.HTTPRegistry, dm *drain.Manager) (*Filter, error) {
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

	chainConfig, err := parseHTTPFiltersChain(msg.GetHttpFilters(), clusters, lc.HTTPClient, httpRegistry, registry, statPrefix)
	if err != nil {
		return nil, err
	}

	chainNames := make([]string, len(chainConfig))
	for i, e := range chainConfig {
		chainNames[i] = e.name
	}
	perRoute, err := buildPerRouteFromHCM(rc, chainNames, httpRegistry)
	if err != nil {
		return nil, err
	}

	// Phase 24.1 Task 5 (DELTA-2 / ADR-0198): parse + validate the parent
	// virtual_host's rate_limits[] and propagate them through buildRouteTable
	// so the resolved routeTable retains the raw slice (the framework's first
	// exposure of route-level NON-typed_per_filter_config policy data per
	// ADR-0198). The §5.2 PARSE-REJECT roster (disable_key / extension /
	// dynamic_metadata) fires here via ratelimit.ValidateRouteRateLimits per
	// D-RL2 — boot-time fail-fast per ADR-0072, byte-stable wording per
	// ADR-0080 + Task 3's wording consts.
	vhostRateLimits := vh.GetRateLimits()
	if err := ratelimit.ValidateRouteRateLimits(vhostRateLimits); err != nil {
		return nil, fmt.Errorf("hcm: route_config: virtual_hosts[0]: %w", err)
	}

	table, err := buildRouteTable(vh.GetRoutes(), clusters)
	if err != nil {
		return nil, err
	}
	table.vhostRateLimits = vhostRateLimits

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
		dm:                dm,
		chainConfig:       chainConfig,
		perRouteConfig:    perRoute,
		listenerPrincipal: lc.ListenerPrincipal,
	}
	// Phase 07.1 Task 15: the routeTable.bindFilter post-build wiring is
	// REMOVED. Per Decision §3.1, the access-log emit-deferral hooks on
	// directResponseAction / routerAction / routerActionH2 collapsed into a
	// single chain-completion site in HCM dispatch (connection.go for H1;
	// h2dispatch.go for H2 at Task 16). The action types no longer carry a
	// *Filter backpointer.
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
//
// Phase 09 (ADR-0100 first-use anchor): registry + statPrefix are threaded
// into FactoryCtx so stats-bearing per-filter factories (fault, future
// header_mutation, jwt_authn, etc.) can register their stat names at
// HCM-build time per ADR-0061's pre-Freeze discipline. Existing 07.1 filters
// (router, cors, envoygotest) ignore the FactoryCtx Stats + StatPrefix
// fields gracefully (per ADR-0085 nil-tolerance pattern); registry may be
// non-nil unconditionally — non-stat-bearing factories simply do not consume
// it.
func parseHTTPFiltersChain(filters []*hcmv3.HttpFilter, clusters *cluster.Manager, httpClient *httpclient.Client, httpRegistry *filter_http.HTTPRegistry, registry *stats.Registry, statPrefix string) ([]chainEntry, error) {
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
		instanceFactory, err := factories[i](tcAny, filter_http.FactoryCtx{Registry: httpRegistry, Stats: registry, StatPrefix: statPrefix, ClusterManager: clusters, HTTPClient: httpClient})
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
func buildPerRouteFromHCM(rc *routev3.RouteConfiguration, chainNames []string, httpRegistry *filter_http.HTTPRegistry) (*filter_http.PerRouteConfig, error) {
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
	return filter_http.BuildPerRouteConfig(rcMap, scopes, chainNames, httpRegistry)
}

func buildRouteTable(routes []*routev3.Route, clusters *cluster.Manager) (*routeTable, error) {
	t := &routeTable{routes: make([]routeEntry, 0, len(routes))}
	for i, r := range routes {
		match, err := buildMatch(r.GetMatch())
		if err != nil {
			return nil, fmt.Errorf("hcm: route %d: %w", i, err)
		}
		action, err := buildAction(r.GetAction(), r.GetResponseHeadersToAdd(), clusters)
		if err != nil {
			return nil, fmt.Errorf("hcm: route %d: %w", i, err)
		}
		// Phase 24.1 Task 5 (DELTA-2 / ADR-0198): extract + validate the
		// route's RouteAction.rate_limits[] when the action arm is Route_Route
		// (the only arm carrying RouteAction). Direct-response routes have no
		// RouteAction and therefore no rate_limits — the rateLimits field stays
		// nil. §5.2 PARSE-REJECT (disable_key / extension / dynamic_metadata)
		// fires here via ratelimit.ValidateRouteRateLimits per D-RL2; byte-stable
		// wording per ADR-0080 + Task 3 wording consts.
		var routeRateLimits []*routev3.RateLimit
		if rr, ok := r.GetAction().(*routev3.Route_Route); ok {
			routeRateLimits = rr.Route.GetRateLimits()
			if err := ratelimit.ValidateRouteRateLimits(routeRateLimits); err != nil {
				return nil, fmt.Errorf("hcm: route %d: %w", i, err)
			}
		}
		t.routes = append(t.routes, routeEntry{match: match, action: action, rateLimits: routeRateLimits})
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

func buildAction(a interface{}, responseHeadersToAdd []*corev3.HeaderValueOption, clusters *cluster.Manager) (routeAction, error) {
	switch act := a.(type) {
	case *routev3.Route_Route:
		return buildRouterAction(act.Route, clusters)
	case *routev3.Route_DirectResponse:
		return buildDirectResponseAction(act.DirectResponse, responseHeadersToAdd)
	case nil:
		return nil, fmt.Errorf("action is missing")
	default:
		return nil, fmt.Errorf("action %T is not supported in phase 04", act)
	}
}

// buildRouterAction returns a routeAction satisfying the codec-neutral shape.
// Phase 07.1 Task 15: collapses the H1/H2 variant selection into a single
// clusterRouteAction bridge type whose router-package backend handles the
// H1 upstream-dial path. The H2 variant selection (UseH2()→routerActionH2)
// from phase 05.2 is preserved structurally but currently routes through
// the same H1 bridge — the H2 chain-mediated dispatch path lands at Task 16
// (when h2dispatch.go's type-switch on *routerActionH2 is rewritten to use
// the chain). For now, an H2-using cluster on the H1 path is a misconfiguration
// that would have been caught by phase-05.2's H2-listener wiring; this path
// is exercised only on H1-listener bootstraps where UseH2() is false.
//
// Pre-Task-15 (deleted): the H1 branch returned *routerAction (now in
// internal/filter/http/router as routerAction); the H2 branch returned
// *routerActionH2 (now in router/router_h2.go). Both are package-private
// to the router package post-Task-11; the chain-mediated dispatch reaches
// them via the router.H1ClusterAction closure-builder.
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
	return &clusterRouteAction{cluster: c}, nil
}

func buildDirectResponseAction(d *routev3.DirectResponseAction, responseHeadersToAdd []*corev3.HeaderValueOption) (*directResponseAction, error) {
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
	extras, err := buildExtraResponseHeaders(responseHeadersToAdd)
	if err != nil {
		return nil, fmt.Errorf("direct_response.response_headers_to_add: %w", err)
	}
	return &directResponseAction{
		status:       int(d.Status),
		bodyText:     is.InlineString,
		extraHeaders: extras,
	}, nil
}

// buildExtraResponseHeaders converts route-level response_headers_to_add into
// an OrderedHeaders slice the directResponseAction applies in body() with
// OVERWRITE_IF_EXISTS_OR_ADD semantics (per phase 14 Task 14 — see
// actions.go's directResponseAction GoDoc for rationale).
//
// Validation:
//   - skips entries with nil header or empty key (defensive; the proto allows
//     them, Envoy rejects at config-load time).
//   - rejects unsupported AppendAction values (only OVERWRITE_IF_EXISTS_OR_ADD
//     is supported; APPEND_IF_EXISTS_OR_ADD / ADD_IF_ABSENT / OVERWRITE_IF_EXISTS
//     are reserved for future support).
//
// Phase-14 fixture 0016 YAMLs MUST set `append_action: OVERWRITE_IF_EXISTS_OR_ADD`
// explicitly so envoy-go does not reject the config; reference Envoy honors
// the explicit append_action symmetrically.
func buildExtraResponseHeaders(opts []*corev3.HeaderValueOption) (filter_http.OrderedHeaders, error) {
	if len(opts) == 0 {
		return nil, nil
	}
	out := make(filter_http.OrderedHeaders, 0, len(opts))
	for i, o := range opts {
		if o == nil {
			continue
		}
		if o.AppendAction != corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD {
			return nil, fmt.Errorf("entry %d: append_action %v is not supported (only OVERWRITE_IF_EXISTS_OR_ADD)", i, o.AppendAction)
		}
		h := o.GetHeader()
		if h == nil {
			continue
		}
		if h.Key == "" {
			continue
		}
		out = append(out, filter_http.HeaderField{Name: h.Key, Value: h.Value})
	}
	return out, nil
}
