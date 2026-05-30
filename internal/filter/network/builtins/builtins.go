// Package builtins registers the four built-in network filters (echo,
// direct_response, tcp_proxy, HCM) into a *network.Registry with their boot
// singletons captured. It lives OUTSIDE internal/filter/network (which the
// filters import) and outside internal/listener (whose tests import this), so
// no import cycle forms (D-26.2-5 / D-26.2-7). Consumed by cmd/envoy-go/main.go
// + the listener manager's thinner constructors + the admin/manager/main test
// callers — the single place the boot-singleton wiring lives.
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

// RegisterBuiltins registers echo, direct_response, tcp_proxy, and HCM into reg.
// It mirrors the registration calls in cmd/envoy-go/main.go and does NOT Freeze
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
}
