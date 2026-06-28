// Package builtins registers the eleven built-in network filters (echo,
// direct_response, tcp_proxy, HCM, rbac_network, sni_cluster, zookeeper_proxy,
// mongo_proxy, kafka_broker, redis_proxy, thrift_proxy) into a *network.Registry
// with their boot singletons captured. It lives OUTSIDE
// internal/filter/network (which the filters import) and outside
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
	"github.com/esalaine/envoy-go/internal/filter/network/kafkabroker"
	"github.com/esalaine/envoy-go/internal/filter/network/mongoproxy"
	networkrbac "github.com/esalaine/envoy-go/internal/filter/network/rbac"
	"github.com/esalaine/envoy-go/internal/filter/network/redisproxy"
	"github.com/esalaine/envoy-go/internal/filter/network/snicluster"
	"github.com/esalaine/envoy-go/internal/filter/network/thriftproxy"
	"github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy"
	"github.com/esalaine/envoy-go/internal/filter/tcpproxy"
	"github.com/esalaine/envoy-go/internal/httpclient"
	"github.com/esalaine/envoy-go/internal/stats"
	"github.com/esalaine/envoy-go/internal/tracing"
)

// Deps carries the boot singletons the terminal-filter adapters capture. The
// read filters (echo/direct_response) need none. Nil-tolerant where the
// underlying adapter/constructor is (dm, httpClient, accessLogSinks,
// TracingExporters).
type Deps struct {
	ClusterManager   *cluster.Manager
	StatsRegistry    *stats.Registry
	AccessLogSinks   []accesslog.Sink
	HTTPRegistry     *filter_http.HTTPRegistry
	DrainManager     *drain.Manager
	HTTPClient       *httpclient.Client
	TracingExporters *tracing.ExporterProvider
}

// RegisterBuiltins registers echo, direct_response, tcp_proxy, HCM,
// rbac_network, sni_cluster, zookeeper_proxy, mongo_proxy, kafka_broker,
// redis_proxy and thrift_proxy into reg. It
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
		deps.TracingExporters,
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
	// kafka_broker: the 9th built-in (31; ADR-0228; the first /contrib consumer).
	// Stats-PRIMARY filter: the registry is closure-captured (the
	// mongo_proxy/zookeeper_proxy/rbac_network precedent — FactoryCtx carries no
	// stats registry).
	reg.Register(kafkabroker.TypeURL, kafkabroker.NewFactory(deps.StatsRegistry))
	// redis_proxy: the 10th built-in (32.1; ADR-0229/ADR-0230). UNLIKE the
	// stats-only kafka/mongo/zookeeper registrations, redisproxy passes BOTH
	// deps.ClusterManager (lazy catch_all resolution → Cluster.Dial via the
	// upstream-pool seam, ADR-0230) AND deps.StatsRegistry (the redis.<sp>
	// roster) — the tcpproxy.NewNetworkFactory cluster-capture + the stats-capture
	// precedents combined. The project's FIRST terminal routing proxy.
	reg.Register(redisproxy.TypeURL, redisproxy.NewFactory(deps.ClusterManager, deps.StatsRegistry))
	// thrift_proxy: the 11th built-in (33; ADR-0231). The project's SECOND
	// terminal routing proxy. Like redisproxy it needs BOTH deps.ClusterManager
	// (route cluster → Cluster.Dial via the REUSED ADR-0230 upstream-pool seam)
	// AND deps.StatsRegistry (the thrift.<sp> roster). The FIRST row to REUSE a
	// prior framework seam unchanged. The §9 family-CLOSING built-in.
	reg.Register(thriftproxy.TypeURL, thriftproxy.NewFactory(deps.ClusterManager, deps.StatsRegistry))
}
