package hcm

import (
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

func mkRouter() *anypb.Any {
	a, _ := anypb.New(&routerv3.Router{})
	return a
}

// mkHCM builds a minimal valid HCM Any. modify (optional) mutates the proto
// before serialization to provoke a targeted error class.
func mkHCM(modify func(*hcmv3.HttpConnectionManager)) *anypb.Any {
	hcm := &hcmv3.HttpConnectionManager{
		CodecType:  hcmv3.HttpConnectionManager_HTTP1,
		StatPrefix: "ingress_http",
		RouteSpecifier: &hcmv3.HttpConnectionManager_RouteConfig{
			RouteConfig: &routev3.RouteConfiguration{
				VirtualHosts: []*routev3.VirtualHost{{
					Name:    "vh_default",
					Domains: []string{"*"},
					Routes: []*routev3.Route{{
						Match: &routev3.RouteMatch{PathSpecifier: &routev3.RouteMatch_Path{Path: "/health"}},
						Action: &routev3.Route_DirectResponse{DirectResponse: &routev3.DirectResponseAction{
							Status: 200,
							Body:   &corev3.DataSource{Specifier: &corev3.DataSource_InlineString{InlineString: "OK\n"}},
						}},
					}},
				}},
			},
		},
		HttpFilters: []*hcmv3.HttpFilter{{
			Name:       "envoy.filters.http.router",
			ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: mkRouter()},
		}},
	}
	if modify != nil {
		modify(hcm)
	}
	any, _ := anypb.New(hcm)
	return any
}

// mkClusterManager builds a one-cluster Manager with a STATIC cluster
// "c_test" pointing at 127.0.0.1:1 (unreachable; tests don't dial).
func mkClusterManager(t *testing.T) *cluster.Manager {
	t.Helper()
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 "c_test",
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: "c_test",
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       "127.0.0.1",
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 1},
									},
								}},
							}},
						}},
					}},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs)
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	return cm
}

// expectErr runs parseFilter with the modifier-built Any and asserts the
// returned error has the "hcm:" prefix and contains wantSubstr.
func expectErr(t *testing.T, modify func(*hcmv3.HttpConnectionManager), wantSubstr string) {
	t.Helper()
	cm := mkClusterManager(t)
	_, err := parseFilter(mkHCM(modify), cm)
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wantSubstr)
	}
	if !strings.HasPrefix(err.Error(), "hcm:") {
		t.Errorf("error must begin with %q, got: %v", "hcm:", err)
	}
	if wantSubstr != "" && !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("error %q does not contain %q", err.Error(), wantSubstr)
	}
}

func TestParseFilter_Happy(t *testing.T) {
	cm := mkClusterManager(t)
	if _, err := parseFilter(mkHCM(nil), cm); err != nil {
		t.Fatalf("happy: %v", err)
	}
}

func TestParseFilter_WrongTypeURL(t *testing.T) {
	cm := mkClusterManager(t)
	other, _ := anypb.New(&wrapperspb.StringValue{Value: "x"})
	_, err := parseFilter(other, cm)
	if err == nil || !strings.HasPrefix(err.Error(), "hcm: wrong type_url") {
		t.Errorf("expected hcm: wrong type_url ..., got: %v", err)
	}
}

func TestParseFilter_CodecTypeHTTP2(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP2 }, "codec_type HTTP2")
}

func TestParseFilter_CodecTypeHTTP3(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP3 }, "codec_type HTTP3")
}

func TestParseFilter_CodecTypeAUTO(t *testing.T) {
	cm := mkClusterManager(t)
	if _, err := parseFilter(mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_AUTO }), cm); err != nil {
		t.Errorf("AUTO should be accepted as alias for HTTP1: %v", err)
	}
}

func TestParseFilter_MissingStatPrefix(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) { h.StatPrefix = "" }, "stat_prefix")
}

func TestParseFilter_RDSRouteSpecifier(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.RouteSpecifier = &hcmv3.HttpConnectionManager_Rds{Rds: &hcmv3.Rds{RouteConfigName: "rc"}}
	}, "route_specifier=rds")
}

func TestParseFilter_ScopedRoutes(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.RouteSpecifier = &hcmv3.HttpConnectionManager_ScopedRoutes{ScopedRoutes: &hcmv3.ScopedRoutes{Name: "sr"}}
	}, "route_specifier=scoped_routes")
}

func TestParseFilter_ZeroVirtualHosts(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts = nil
	}, "virtual_hosts")
}

func TestParseFilter_TwoVirtualHosts(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		extra := &routev3.VirtualHost{Name: "vh_extra", Domains: []string{"*"}}
		h.GetRouteConfig().VirtualHosts = append(h.GetRouteConfig().VirtualHosts, extra)
	}, "virtual_hosts")
}

func TestParseFilter_VHostDomainsEmpty(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Domains = nil
	}, "domains")
}

func TestParseFilter_VHostDomainsNotStarOnly(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Domains = []string{"example.com"}
	}, "domains")
}

func TestParseFilter_HTTPFiltersEmpty(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) { h.HttpFilters = nil }, "http_filters")
}

func TestParseFilter_HTTPFiltersTwoEntries(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.HttpFilters = append(h.HttpFilters, &hcmv3.HttpFilter{
			Name:       "envoy.filters.http.router",
			ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: mkRouter()},
		})
	}, "http_filters")
}

func TestParseFilter_HTTPFiltersWrongName(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.HttpFilters[0].Name = "envoy.filters.http.cors"
	}, "name")
}

func TestParseFilter_HTTPFiltersWrongTypeURL(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		other, _ := anypb.New(&wrapperspb.StringValue{Value: "x"})
		h.HttpFilters[0].ConfigType = &hcmv3.HttpFilter_TypedConfig{TypedConfig: other}
	}, "type_url")
}

func TestParseFilter_RouteUnknownAction(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Action = &routev3.Route_Redirect{
			Redirect: &routev3.RedirectAction{},
		}
	}, "action")
}

func TestParseFilter_RouteSafeRegex(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Match = &routev3.RouteMatch{
			PathSpecifier: &routev3.RouteMatch_SafeRegex{SafeRegex: &matcherv3.RegexMatcher{Regex: ".*"}},
		}
	}, "safe_regex")
}

func TestParseFilter_RoutePathSeparatedPrefix(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Match = &routev3.RouteMatch{
			PathSpecifier: &routev3.RouteMatch_PathSeparatedPrefix{PathSeparatedPrefix: "/api"},
		}
	}, "path_separated_prefix")
}

func TestParseFilter_RouteCaseSensitiveFalse(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Match.CaseSensitive = wrapperspb.Bool(false)
	}, "case_sensitive=false")
}

func TestParseFilter_RouteHeadersSet(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Match.Headers = []*routev3.HeaderMatcher{{Name: "x-test"}}
	}, "headers")
}

func TestParseFilter_RouteQueryParamsSet(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Match.QueryParameters = []*routev3.QueryParameterMatcher{{Name: "q"}}
	}, "query_parameters")
}

func TestParseFilter_RouteRuntimeFraction(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Match.RuntimeFraction = &corev3.RuntimeFractionalPercent{}
	}, "runtime_fraction")
}

func TestParseFilter_DirectResponseStatusZero(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Action.(*routev3.Route_DirectResponse).DirectResponse.Status = 0
	}, "status")
}

func TestParseFilter_DirectResponseStatus600(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Action.(*routev3.Route_DirectResponse).DirectResponse.Status = 600
	}, "status")
}

func TestParseFilter_DirectResponseInlineBytes(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Action.(*routev3.Route_DirectResponse).DirectResponse.Body = &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte("x")},
		}
	}, "inline_string")
}

func TestParseFilter_DirectResponseFilename(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Action.(*routev3.Route_DirectResponse).DirectResponse.Body = &corev3.DataSource{
			Specifier: &corev3.DataSource_Filename{Filename: "/tmp/x"},
		}
	}, "inline_string")
}

func TestParseFilter_DirectResponseEmptyBody(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Action.(*routev3.Route_DirectResponse).DirectResponse.Body = &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{InlineString: ""},
		}
	}, "inline_string")
}

func TestParseFilter_RouterActionWeightedClusters(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Action = &routev3.Route_Route{Route: &routev3.RouteAction{
			ClusterSpecifier: &routev3.RouteAction_WeightedClusters{WeightedClusters: &routev3.WeightedCluster{}},
		}}
	}, "cluster_specifier")
}

func TestParseFilter_RouterActionClusterHeader(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Action = &routev3.Route_Route{Route: &routev3.RouteAction{
			ClusterSpecifier: &routev3.RouteAction_ClusterHeader{ClusterHeader: "x-cluster"},
		}}
	}, "cluster_specifier")
}

func TestParseFilter_RouterActionUnknownCluster(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Action = &routev3.Route_Route{Route: &routev3.RouteAction{
			ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_missing"},
		}}
	}, "c_missing")
}

func TestParseFilter_RouterActionHappy(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Match = &routev3.RouteMatch{PathSpecifier: &routev3.RouteMatch_Prefix{Prefix: "/api"}}
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Action = &routev3.Route_Route{Route: &routev3.RouteAction{
			ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_test"},
		}}
	})
	if _, err := parseFilter(any, cm); err != nil {
		t.Errorf("router-action happy: %v", err)
	}
}
