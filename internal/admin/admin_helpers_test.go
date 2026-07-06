package admin

import (
	"strings"
	"testing"

	"github.com/pgdad/envoy-go/internal/bootstrap"
	"github.com/pgdad/envoy-go/internal/cluster"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/http/router"
	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/filter/network/builtins"
	"github.com/pgdad/envoy-go/internal/listener"
	"github.com/pgdad/envoy-go/internal/listener/listenerfilter"
	"github.com/pgdad/envoy-go/internal/stats"
)

// mustBuiltinsNetReg returns a frozen *network.Registry with the four built-in
// network filters (echo, direct_response, tcp_proxy, HCM) registered via the
// Task-9 builtins seam. Post-Task-10 the listener manager requires a populated
// netReg to build tcp_proxy / HCM filter chains. Shared by the admin tests'
// listener-manager bootstraps.
func mustBuiltinsNetReg(cm *cluster.Manager, registry *stats.Registry, httpReg *filter_http.HTTPRegistry) *network.Registry {
	r := network.NewRegistry()
	builtins.RegisterBuiltins(r, builtins.Deps{ClusterManager: cm, StatsRegistry: registry, HTTPRegistry: httpReg})
	r.Freeze()
	return r
}

// minimalBootstrapYAML is the SPEC §7.3 fixture bootstrap (admin :9901,
// listener :10000, cluster c_backend with 2 endpoints :18001 + :18002).
// Used by the 08.1 admin handler tests as the canonical test bootstrap.
//
//nolint:unused // PLAN Task 5 scaffolding; consumed by Tasks 6-9 handler tests.
const minimalBootstrapYAML = `admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}

static_resources:
  listeners:
    - name: l_main
      address: {socket_address: {address: 127.0.0.1, port_value: 10000}}
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress_http
                route_config:
                  name: rc_main
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: {prefix: /}
                          route: {cluster: c_backend}
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STATIC
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: 18001}}}
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: 18002}}}
`

// mustMinimalBs returns the parsed *bootstrap.Bootstrap for the §7.3 fixture.
// Sets ConfigPath to "/test/envoy-go.yaml" so /server_info tests can assert
// the field is threaded.
//
//nolint:unused // PLAN Task 5 scaffolding; consumed by Tasks 6-9 handler tests.
func mustMinimalBs(t *testing.T) *bootstrap.Bootstrap {
	t.Helper()
	bs, err := bootstrap.Load(strings.NewReader(minimalBootstrapYAML))
	if err != nil {
		t.Fatalf("bootstrap.Load: %v", err)
	}
	bs.ConfigPath = "/test/envoy-go.yaml"
	return bs
}

// mustMinimalCM returns a *cluster.Manager built from the §7.3 fixture.
//
//nolint:unused // PLAN Task 5 scaffolding; consumed by Tasks 6-9 handler tests.
func mustMinimalCM(t *testing.T, bs *bootstrap.Bootstrap) *cluster.Manager {
	t.Helper()
	cm, err := cluster.NewManager(bs.Proto, bs.Stats)
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	return cm
}

// mustMinimalLM returns a *listener.Manager built from the §7.3 fixture.
// Threads a frozen HTTPRegistry containing only the router terminal filter
// (required for the HCM filter chain build) and an empty-but-frozen
// listener-filter registry. The §7.3 fixture has no listener_filters[].
//
//nolint:unused // PLAN Task 5 scaffolding; consumed by Tasks 6-9 handler tests.
func mustMinimalLM(t *testing.T, bs *bootstrap.Bootstrap, cm *cluster.Manager) *listener.Manager {
	t.Helper()
	httpReg := filter_http.NewHTTPRegistry()
	httpReg.Register(router.TypeURL, router.New)
	httpReg.Freeze()
	lfReg := listenerfilter.NewListenerFilterRegistry()
	lfReg.Freeze()
	netReg := mustBuiltinsNetReg(cm, bs.Stats, httpReg)
	lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, "", false, bs.Stats, nil, httpReg, lfReg, nil, nil, netReg)
	if err != nil {
		t.Fatalf("listener.NewManagerWithBaseDirAndAllowH2C: %v", err)
	}
	return lm
}
