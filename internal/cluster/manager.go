package cluster

import (
	"fmt"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"

	internaltls "github.com/esalaine/envoy-go/internal/tls"
)

// upstreamTLSContextTypeURL is the well-known Any type URL for
// UpstreamTlsContext. Declared locally to avoid adding an exported symbol to
// internal/tls just for this comparison.
const upstreamTLSContextTypeURL = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext"

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
func NewManager(bs *bootstrapv3.Bootstrap) (*Manager, error) {
	return NewManagerWithBaseDir(bs, "")
}

// NewManagerWithBaseDir is the phase-03 variant of NewManager. baseDir is
// passed to internal/tls.NewUpstreamConfig so that filename-based DataSources
// in transport_socket are resolved relative to the config file location. Pass
// "" to resolve relative to the process working directory (phase-02 compat).
func NewManagerWithBaseDir(bs *bootstrapv3.Bootstrap, baseDir string) (*Manager, error) {
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
		m.clusters[built.name] = built
	}
	return m, nil
}

// Get looks up a cluster by name. Returns (nil, false) if not found.
func (m *Manager) Get(name string) (*Cluster, bool) {
	c, ok := m.clusters[name]
	return c, ok
}

func buildCluster(c *clusterv3.Cluster, idx int, baseDir string) (*Cluster, error) {
	name := c.GetName()
	if name == "" {
		return nil, fmt.Errorf("cluster: clusters[%d]: missing name", idx)
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
	return cl, nil
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
