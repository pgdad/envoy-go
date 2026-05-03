package cluster

import (
	stdtls "crypto/tls"
	"fmt"
	"sort"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	upstreamshttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"

	"github.com/esalaine/envoy-go/internal/stats"
	internaltls "github.com/esalaine/envoy-go/internal/tls"
)

// upstreamTLSContextTypeURL is the well-known Any type URL for
// UpstreamTlsContext. Declared locally to avoid adding an exported symbol to
// internal/tls just for this comparison.
const upstreamTLSContextTypeURL = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext"

// httpProtocolOptionsKey is the well-known key under which the cluster-side
// HttpProtocolOptions extension lives in
// cluster.typed_extension_protocol_options. The Envoy proto authoritatively
// reserves this key on the v3 cluster surface; see SPEC §5.5.
const httpProtocolOptionsKey = "envoy.extensions.upstreams.http.v3.HttpProtocolOptions"

// Manager owns every Cluster materialized from static_resources.clusters[].
// Get is the dataplane-side lookup the TCP proxy filter uses to resolve a
// cluster name at filter-construction time.
type Manager struct {
	clusters map[string]*Cluster
}

// NewManager walks bs.GetStaticResources().GetClusters() and materializes one
// Cluster per entry. Errors are returned at the first violation; subsequent
// clusters are not validated. Every error begins with "cluster: ".
//
// Phase-02 surface (SPEC §2 + §5.4):
//   - cluster.type must be STATIC. STRICT_DNS, LOGICAL_DNS, EDS, ORIGINAL_DST
//     all error explicitly.
//   - cluster.lb_policy must be unset (proto default ROUND_ROBIN) or explicitly
//     ROUND_ROBIN. Anything else errors.
//   - load_assignment.endpoints[*].lb_endpoints[*] must collectively contain
//     ≥1 endpoint, each with endpoint.address.socket_address (no pipe, no
//     envoy_internal_address). Total endpoint count across all locality
//     groups must be ≥1.
//   - cluster.connect_timeout, when unset, defaults to 5s (defaultConnectTimeout).
//
// Phase-02 callers that do not need baseDir resolution should continue using
// NewManager. Phase-03 callers that load filename-based DataSources should use
// NewManagerWithBaseDir and pass filepath.Dir(configPath).
//
// Phase 06.1 (Task 8) widened the signature to accept a *stats.Registry; for
// each cluster the manager allocates the 8 cluster-scope metrics from SPEC §6
// (per ADR-0063 — cluster-level only) on the supplied Registry at boot time.
// The caller MUST pass a non-nil Registry; the Registry MUST not yet be
// Frozen (cmd/envoy-go/main.go's boot sequence freezes only after the listener
// manager and admin server are up — see SPEC §5.4).
func NewManager(bs *bootstrapv3.Bootstrap, registry *stats.Registry) (*Manager, error) {
	return NewManagerWithBaseDir(bs, "", registry)
}

// NewManagerWithBaseDir is the phase-03 variant of NewManager. baseDir is
// passed to internal/tls.NewUpstreamConfig so that filename-based DataSources
// in transport_socket are resolved relative to the config file location. Pass
// "" to resolve relative to the process working directory (phase-02 compat).
//
// Phase 06.1 (Task 8): see NewManager doc — same Registry contract applies.
func NewManagerWithBaseDir(bs *bootstrapv3.Bootstrap, baseDir string, registry *stats.Registry) (*Manager, error) {
	cs := bs.GetStaticResources().GetClusters()
	if len(cs) == 0 {
		return nil, fmt.Errorf("cluster: zero clusters in bootstrap")
	}
	m := &Manager{clusters: make(map[string]*Cluster, len(cs))}
	for i, c := range cs {
		built, err := buildCluster(c, i, baseDir)
		if err != nil {
			return nil, err
		}
		if _, dup := m.clusters[built.name]; dup {
			return nil, fmt.Errorf("cluster: duplicate cluster name %q", built.name)
		}
		registerClusterMetrics(registry, built)
		m.clusters[built.name] = built
	}
	return m, nil
}

// registerClusterMetrics allocates the 8 cluster-scope metrics per ADR-0063
// and stores the pointers on c. Called once per cluster at Manager build time;
// pre-Freeze (the listener manager and admin server precede the
// registry.Freeze() call in cmd/envoy-go/main.go — see SPEC §5.4 for the boot
// ordering invariant). The membership_total gauge is Set once at register
// time to len(c.endpoints) per SPEC §6 (cluster-level only; per-endpoint
// stats are deferred per ADR-0063).
func registerClusterMetrics(r *stats.Registry, c *Cluster) {
	prefix := "cluster." + c.name + "."
	c.upstreamRqTotal = r.NewCounter(prefix + "upstream_rq_total")
	c.upstreamRq2xx = r.NewCounter(prefix + "upstream_rq_2xx")
	c.upstreamRq3xx = r.NewCounter(prefix + "upstream_rq_3xx")
	c.upstreamRq4xx = r.NewCounter(prefix + "upstream_rq_4xx")
	c.upstreamRq5xx = r.NewCounter(prefix + "upstream_rq_5xx")
	c.upstreamCxTotal = r.NewCounter(prefix + "upstream_cx_total")
	c.upstreamCxActive = r.NewGauge(prefix + "upstream_cx_active")
	c.membershipTotal = r.NewGauge(prefix + "membership_total")
	c.membershipTotal.Set(int64(len(c.endpoints))) // SPEC §6: Set once at register, equals N endpoints
}

// Get looks up a cluster by name. Returns (nil, false) if not found.
func (m *Manager) Get(name string) (*Cluster, bool) {
	c, ok := m.clusters[name]
	return c, ok
}

// ClusterInfo is the public read-only summary of one cluster, returned by
// Manager.Clusters() and consumed by phase-08.1's /clusters admin handler.
// Per ADR-0087, the /clusters handler reads the snapshot once per scrape and
// formats one block per cluster (10 cluster-level lines + 18 per-endpoint
// lines per the §11.2 empirical pin).
//
//nolint:revive // ADR-0087 reserves the ClusterInfo name for the public /clusters-snapshot surface; phase-08.1 SPEC §6.2 fixes the type name verbatim.
type ClusterInfo struct {
	Name      string
	Endpoints []EndpointInfo
}

// EndpointInfo is the public read-only summary of one upstream endpoint
// within a ClusterInfo. Address is the dotted-quad / IPv6-literal host; Port
// is the TCP port. The combined "address:port" form is what the /clusters
// handler emits in the per-endpoint key prefix per SPEC §11.2.
type EndpointInfo struct {
	Address string
	Port    uint32
}

// Clusters returns a freshly-allocated snapshot of all configured clusters
// and their endpoints, in alphabetical-by-name order. Per-cluster endpoints
// are returned in their bootstrap-declared order (the order extractEndpoints
// preserves at NewManager time). The returned slice is safe for caller
// mutation: modifying it does not affect the Manager's internal state.
//
// Counters / gauges are NOT cached in the returned struct — phase-08.1's
// /clusters handler emits literal `0` for all 8 per-endpoint cx_*/rq_*
// counter fields per the planner-time decision (envoy-go has no per-endpoint
// stats per ADR-0063 deferral; cluster-level counters are surfaced via
// /stats/prometheus and would not partition meaningfully across endpoints).
//
// Phase 08.1 (Task 3) introduces this accessor; ADR-0087 records the design.
func (m *Manager) Clusters() []ClusterInfo {
	out := make([]ClusterInfo, 0, len(m.clusters))
	for _, c := range m.clusters {
		eps := make([]EndpointInfo, 0, len(c.endpoints))
		for _, ep := range c.endpoints {
			eps = append(eps, EndpointInfo{Address: ep.Host, Port: ep.Port})
		}
		out = append(out, ClusterInfo{Name: c.name, Endpoints: eps})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Drain closes upstream connection pools across all configured clusters.
// Best-effort — returns no error. Walks m.clusters map and calls
// c.closePool() on each cluster.
//
// Called from cmd/envoy-go/main.go AFTER <-drainMgr.Done() fires (i.e.,
// after no in-flight downstream requests remain — therefore no in-flight
// upstream requests can remain). The pool close releases socket file
// descriptors for cleanest shutdown but is not required for correctness
// (Go's runtime will close TCP sockets on process exit regardless).
//
// Idempotent; safe to call multiple times. The m.clusters map is populated
// once at construction (NewManager) and never modified afterward, so
// concurrent range here is safe. Per-cluster pool fields, when they exist
// in future hot-restart-family expansions, must provide their own close-
// idempotency (e.g., sync.Once) per planner-time decision 6.
//
// Phase 08.2 (Task 4) introduces this accessor; ADR-0096 records the
// design (the consolidated in-flight-completion ADR; Tasks 9 + 10 cite
// ADR-0096 without re-anchoring).
func (m *Manager) Drain() {
	for _, c := range m.clusters {
		c.closePool()
	}
}

func buildCluster(c *clusterv3.Cluster, idx int, baseDir string) (*Cluster, error) {
	name := c.GetName()
	if name == "" {
		return nil, fmt.Errorf("cluster: clusters[%d]: missing name", idx)
	}
	// Validate the assembled metric-name shape before any registry write.
	// cluster.<name> is propagated into eight "cluster.<name>.<metric>" names
	// at registerClusterMetrics; if the assembled name contains characters
	// outside the metric-name regex's permitted [a-zA-Z0-9_.] class (or
	// otherwise produces an invalid assembled name), Registry.NewCounter
	// would panic per ADR-0059's boot-time panic discipline. We reject at
	// the user-input boundary instead. Inherits ADR-0065's pattern by
	// reference per ADR-0065 Consequences (d) (no new ADR); symmetric to
	// the parseFilterWithCtx guard at internal/filter/hcm/config.go:143.
	// Validating the longest assembled name suffices because the other
	// seven assembled names differ only in suffixes within the regex's
	// permitted class (they pass/fail together).
	if !stats.IsValidName("cluster." + name + ".upstream_rq_total") {
		return nil, fmt.Errorf("cluster: %q: invalid cluster name (must contain only ASCII letters, digits, underscore, or dot, and form a valid metric-name segment)", name)
	}
	t, ok := c.GetClusterDiscoveryType().(*clusterv3.Cluster_Type)
	if !ok {
		return nil, fmt.Errorf("cluster: %q: cluster_discovery_type must be Type, got %T (only STATIC supported)", name, c.GetClusterDiscoveryType())
	}
	if t.Type != clusterv3.Cluster_STATIC {
		return nil, fmt.Errorf("cluster: %q: only STATIC clusters supported; got %s", name, t.Type)
	}
	if c.GetLbPolicy() != clusterv3.Cluster_ROUND_ROBIN {
		return nil, fmt.Errorf("cluster: %q: only ROUND_ROBIN lb_policy supported; got %s", name, c.GetLbPolicy())
	}
	la := c.GetLoadAssignment()
	if la == nil {
		return nil, fmt.Errorf("cluster: %q: missing load_assignment", name)
	}
	endpoints, err := extractEndpoints(la, name)
	if err != nil {
		return nil, err
	}
	timeout := defaultConnectTimeout
	if c.GetConnectTimeout() != nil {
		timeout = c.GetConnectTimeout().AsDuration()
	}
	cl := &Cluster{
		name:           name,
		endpoints:      endpoints,
		connectTimeout: timeout,
		lb:             &roundRobin{endpoints: endpoints},
	}
	if ts := c.GetTransportSocket(); ts != nil {
		if ts.GetTypedConfig() == nil {
			return nil, fmt.Errorf("cluster: %q: transport_socket without typed_config", name)
		}
		tu := ts.GetTypedConfig().GetTypeUrl()
		if tu != upstreamTLSContextTypeURL {
			return nil, fmt.Errorf("cluster: %q: unsupported transport_socket type_url %q (phase 03 supports only UpstreamTlsContext)", name, tu)
		}
		uc, err := internaltls.NewUpstreamConfig(ts, baseDir)
		if err != nil {
			return nil, fmt.Errorf("cluster: %q: %w", name, err)
		}
		cl.upstreamCfg = uc.TLSConfig
	}
	// Phase 05.2 (Task 10, SPEC §5.5): read typed_extension_protocol_options for
	// HttpProtocolOptions and decide whether this cluster originates H2 upstream.
	useH2, err := extractH2Mode(c, cl.upstreamCfg)
	if err != nil {
		return nil, err
	}
	cl.useH2 = useH2
	return cl, nil
}

// extractH2Mode reads the cluster's typed_extension_protocol_options and
// returns whether to enable H2 upstream origination. Per SPEC §5.5's behavior
// matrix:
//   - field absent → false (phase-04 baseline; no regression)
//   - explicit_http_config.http2_protocol_options{} → true (validated; build-time TLS+ALPN check)
//   - explicit_http_config.http_protocol_options{} → false (silent-ignore inner)
//   - auto_config{} → false (the 05.2 narrowing of master SPEC §5.8)
//   - nil/empty UpstreamProtocolOptions → false (defensive)
//
// When useH2==true: the cluster's transport_socket MUST be present, MUST be
// type tls, and the parsed TLS config's alpn_protocols MUST include "h2".
// Validation errors carry the diagnostics enumerated in SPEC §4.1.
//
// parsedTLS is the *stdtls.Config produced by internal/tls.NewUpstreamConfig
// from the cluster's transport_socket; its NextProtos field is populated from
// CommonTlsContext.alpn_protocols. Pass nil for plaintext clusters; the
// transport_socket-required validation will surface the diagnostic.
func extractH2Mode(c *clusterv3.Cluster, parsedTLS *stdtls.Config) (useH2 bool, err error) {
	tepo := c.GetTypedExtensionProtocolOptions()
	if tepo == nil {
		return false, nil
	}
	tepoAny, ok := tepo[httpProtocolOptionsKey]
	if !ok {
		return false, nil
	}
	var hpo upstreamshttpv3.HttpProtocolOptions
	if err := tepoAny.UnmarshalTo(&hpo); err != nil {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions: unmarshal: %w", c.GetName(), err)
	}
	switch up := hpo.UpstreamProtocolOptions.(type) {
	case *upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_:
		switch up.ExplicitHttpConfig.GetProtocolConfig().(type) {
		case *upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions:
			useH2 = true
		default:
			useH2 = false // H1 discriminator: silent-ignore inner.
		}
	case *upstreamshttpv3.HttpProtocolOptions_AutoConfig:
		useH2 = false // The 05.2 narrowing of master SPEC §5.8 (silent-ignore).
	default:
		useH2 = false // Defensive: nil / use_downstream_protocol_config / etc.
	}
	if !useH2 {
		return false, nil
	}
	// Validate transport_socket + ALPN.
	ts := c.GetTransportSocket()
	if ts == nil {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options requires transport_socket", c.GetName())
	}
	if ts.GetTypedConfig() == nil {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options requires transport_socket of type tls, got transport_socket without typed_config", c.GetName())
	}
	if tu := ts.GetTypedConfig().GetTypeUrl(); tu != upstreamTLSContextTypeURL {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options requires transport_socket of type tls, got %q", c.GetName(), tu)
	}
	if parsedTLS == nil {
		// transport_socket present but TLS parsing returned nil — internal
		// invariant. The earlier transport_socket parse path always sets
		// cl.upstreamCfg non-nil for the UpstreamTlsContext type_url, so this
		// branch is defense-in-depth.
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options: TLS parse returned nil", c.GetName())
	}
	hasH2 := false
	for _, alpn := range parsedTLS.NextProtos {
		if alpn == "h2" {
			hasH2 = true
			break
		}
	}
	if !hasH2 {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options requires alpn_protocols to include %q, got %v", c.GetName(), "h2", parsedTLS.NextProtos)
	}
	return true, nil
}

func extractEndpoints(la *endpointv3.ClusterLoadAssignment, clusterName string) ([]Endpoint, error) {
	var out []Endpoint
	for gi, group := range la.GetEndpoints() {
		for ei, lbe := range group.GetLbEndpoints() {
			ep := lbe.GetEndpoint()
			if ep == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d].endpoint is nil", clusterName, gi, ei)
			}
			addr := ep.GetAddress()
			if addr == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d].endpoint.address is nil", clusterName, gi, ei)
			}
			sa := addr.GetSocketAddress()
			if sa == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d]: only socket_address endpoints supported", clusterName, gi, ei)
			}
			out = append(out, Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue()})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cluster: %q: zero endpoints across all locality groups", clusterName)
	}
	return out, nil
}
