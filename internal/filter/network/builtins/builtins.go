// Package builtins registers the eight built-in network filters (echo,
// direct_response, tcp_proxy, HCM, rbac_network, sni_cluster, zookeeper_proxy,
// mongo_proxy) into a *network.Registry with their boot singletons captured. It
// lives OUTSIDE internal/filter/network (which the filters import) and outside
// internal/listener (whose tests import this), so no import cycle forms
// (D-26.2-5 / D-26.2-7). Consumed by cmd/envoy-go/main.go + the listener
// manager's thinner constructors + the admin/manager/main test callers — the
// single place the boot-singleton wiring lives.
package builtins

import (
	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/drain"
	"github.com/esalaine/envoy-go/internal/filter/hcm"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/filter/network/directresponse"
	"github.com/esalaine/envoy-go/internal/filter/network/echo"
	"github.com/esalaine/envoy-go/internal/filter/network/mongoproxy"
	networkrbac "github.com/esalaine/envoy-go/internal/filter/network/rbac"
	"github.com/esalaine/envoy-go/internal/filter/network/snicluster"
	"github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy"
	"github.com/esalaine/envoy-go/internal/filter/tcpproxy"
	"github.com/esalaine/envoy-go/internal/httpclient"
	"github.com/esalaine/envoy-go/internal/stats"
)

// Deps carries the boot singletons the terminal-filter adapters capture. The
// read filters (echo/direct_response) need none. Nil-tolerant where the
// underlying adapter/constructor is (dm, httpClient, accessLogSinks).
type Deps struct {
	ClusterManager *cluster.Manager
	StatsRegistry  *stats.Registry
	AccessLogSinks []accesslog.Sink
	HTTPRegistry   *filter_http.HTTPRegistry
	DrainManager   *drain.Manager
	HTTPClient     *httpclient.Client
}

// RegisterBuiltins registers echo, direct_response, tcp_proxy, HCM,
// rbac_network, sni_cluster, zookeeper_proxy, and mongo_proxy into reg. It
// mirrors the registration calls in cmd/envoy-go/main.go and does NOT Freeze
// (the caller freezes after any additional registration). reg.Register is void
// (it panics on a frozen or duplicate registry), so there is no error to thread.
func RegisterBuiltins(reg *network.Registry, deps Deps) {
	reg.Register(echo.TypeURL, echo.New)
	reg.Register(directresponse.TypeURL, directresponse.New)
	reg.Register(tcpproxy.TypeURL, tcpproxy.NewNetworkFactory(deps.ClusterManager, deps.DrainManager))
	reg.Register(hcm.TypeURL, hcm.NewNetworkFactory(
		deps.ClusterManager,
		deps.StatsRegistry,
		deps.AccessLogSinks,
		deps.HTTPRegistry,
		deps.DrainManager,
		deps.HTTPClient,
	))
	// rbac_network: the 5th built-in (D-26.3-3). The stats Registry is
	// closure-captured from deps.StatsRegistry (the network FactoryCtx carries no
	// stats registry), mirroring tcpproxy.NewNetworkFactory / hcm.NewNetworkFactory.
	reg.Register(networkrbac.TypeURL, networkrbac.NewFactory(deps.StatsRegistry))
	// sni_cluster: the 6th built-in (config-less, no boot singletons — like
	// echo/direct_response; ADR-0220). No Deps needed.
	reg.Register(snicluster.TypeURL, snicluster.New)
	// zookeeper_proxy: the 7th built-in (28.1; ADR-0222). Stats-PRIMARY filter:
	// the registry is closure-captured (the rbac_network/D-26.3-3 precedent —
	// FactoryCtx carries no stats registry). The first both-directions
	// (ReadFilter + WriteFilter) production filter (ADR-0221 consumer #1).
	reg.Register(zookeeperproxy.TypeURL, zookeeperproxy.NewFactory(deps.StatsRegistry))
	// mongo_proxy: the 8th built-in (29.1; ADR-0224). Stats-PRIMARY filter: the
	// registry is closure-captured (the zookeeper_proxy/rbac_network precedent —
	// FactoryCtx carries no stats registry). The second both-directions
	// (ReadFilter + WriteFilter) production filter (ADR-0221 consumer #2).
	reg.Register(mongoproxy.TypeURL, mongoproxy.NewFactory(deps.StatsRegistry))
}
