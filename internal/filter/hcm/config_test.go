package hcm

import (
	"os"
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
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	upstreamshttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/stats"
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
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
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

// TestParseFilter_CodecTypeHTTP2_RequiresTLS_RejectsPlaintext verifies that
// parseFilterWithCtx rejects codec_type=HTTP2 when both HasTLS and AllowH2C
// are false (per ADR-0050 / phase-05.1 validation).
func TestParseFilter_CodecTypeHTTP2_RequiresTLS_RejectsPlaintext(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP2 })
	_, err := parseFilterWithCtx(any, cm, ListenerCtx{HasTLS: false, AllowH2C: false}, stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error for HTTP2 + plaintext, got nil")
	}
	if !strings.Contains(err.Error(), "codec_type HTTP2 requires TLS") {
		t.Errorf("error = %q, want substring 'codec_type HTTP2 requires TLS'", err.Error())
	}
}

// TestParseFilter_CodecTypeHTTP2_AcceptsTLS verifies that HTTP2 is accepted
// when HasTLS=true.
func TestParseFilter_CodecTypeHTTP2_AcceptsTLS(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP2 })
	if _, err := parseFilterWithCtx(any, cm, ListenerCtx{HasTLS: true}, stats.NewRegistry()); err != nil {
		t.Errorf("HTTP2 + TLS should be accepted, got: %v", err)
	}
}

// TestParseFilter_CodecTypeHTTP2_AcceptsAllowH2C verifies that HTTP2 is
// accepted on a plaintext listener when AllowH2C=true.
func TestParseFilter_CodecTypeHTTP2_AcceptsAllowH2C(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP2 })
	if _, err := parseFilterWithCtx(any, cm, ListenerCtx{AllowH2C: true}, stats.NewRegistry()); err != nil {
		t.Errorf("HTTP2 + allowH2C should be accepted, got: %v", err)
	}
}

// TestParseFilter_CodecTypeAUTO_Accepts_BothCases verifies that AUTO is
// accepted with or without TLS context.
func TestParseFilter_CodecTypeAUTO_Accepts_BothCases(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_AUTO })
	for _, lc := range []ListenerCtx{{HasTLS: false}, {HasTLS: true}} {
		if _, err := parseFilterWithCtx(any, cm, lc, stats.NewRegistry()); err != nil {
			t.Errorf("AUTO + lc=%+v should be accepted, got: %v", lc, err)
		}
	}
}

// TestParseFilter_CodecTypeHTTP1_Accepts_BothCases verifies that HTTP1 is
// accepted regardless of TLS context.
func TestParseFilter_CodecTypeHTTP1_Accepts_BothCases(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP1 })
	for _, lc := range []ListenerCtx{{HasTLS: false}, {HasTLS: true}} {
		if _, err := parseFilterWithCtx(any, cm, lc, stats.NewRegistry()); err != nil {
			t.Errorf("HTTP1 + lc=%+v should be accepted, got: %v", lc, err)
		}
	}
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

// mkH2ClusterManager builds a 2-cluster manager: c_h1 (no HttpProtocolOptions,
// UseH2()==false) and c_h2 (HttpProtocolOptions.explicit_http_config.
// http2_protocol_options{} + transport_socket+tls+alpn=["h2"], UseH2()==true).
// Used by TestBuildRouterAction_PicksH2VariantByClusterUseH2 below.
func mkH2ClusterManager(t *testing.T) *cluster.Manager {
	t.Helper()
	caPEM, err := os.ReadFile("../../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}
	mkH2TS := func() *corev3.TransportSocket {
		ctx := &tlsv3.UpstreamTlsContext{
			Sni: "alpha.envoy-go.test",
			CommonTlsContext: &tlsv3.CommonTlsContext{
				AlpnProtocols: []string{"h2"},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa: &corev3.DataSource{
							Specifier: &corev3.DataSource_InlineBytes{InlineBytes: caPEM},
						},
					},
				},
			},
		}
		anyMsg, err := anypb.New(ctx)
		if err != nil {
			t.Fatalf("anypb.New(UpstreamTlsContext): %v", err)
		}
		return &corev3.TransportSocket{
			Name:       "envoy.transport_sockets.tls",
			ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: anyMsg},
		}
	}
	hpoH2 := &upstreamshttpv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
			},
		},
	}
	hpoAny, err := anypb.New(hpoH2)
	if err != nil {
		t.Fatalf("anypb.New(HttpProtocolOptions): %v", err)
	}

	mkClusterPb := func(name string, port uint32, isH2 bool) *clusterv3.Cluster {
		c := &clusterv3.Cluster{
			Name:                 name,
			ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
			LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
			ConnectTimeout:       durationpb.New(time.Second),
			LoadAssignment: &endpointv3.ClusterLoadAssignment{
				ClusterName: name,
				Endpoints: []*endpointv3.LocalityLbEndpoints{{
					LbEndpoints: []*endpointv3.LbEndpoint{{
						HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
							Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
								SocketAddress: &corev3.SocketAddress{
									Address:       "127.0.0.1",
									PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
								},
							}},
						}},
					}},
				}},
			},
		}
		if isH2 {
			c.TransportSocket = mkH2TS()
			c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
				"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": hpoAny,
			}
		}
		return c
	}

	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{
				mkClusterPb("c_h1", 1, false),
				mkClusterPb("c_h2", 2, true),
			},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	return cm
}

// TestBuildRouterAction_PicksH2VariantByClusterUseH2 verifies the variant-
// selection contract at filter-build time per SPEC §5.5 + §4.1: a route
// whose cluster has UseH2()==true gets a *routerActionH2; UseH2()==false
// gets the existing *routerAction.
func TestBuildRouterAction_PicksH2VariantByClusterUseH2(t *testing.T) {
	cm := mkH2ClusterManager(t)

	// H1 cluster → *routerAction
	{
		ra := &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_h1"}}
		got, err := buildRouterAction(ra, cm)
		if err != nil {
			t.Fatalf("buildRouterAction(c_h1): %v", err)
		}
		if _, ok := got.(*routerAction); !ok {
			t.Errorf("c_h1 → %T; want *routerAction", got)
		}
	}

	// H2 cluster → *routerActionH2
	{
		ra := &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_h2"}}
		got, err := buildRouterAction(ra, cm)
		if err != nil {
			t.Fatalf("buildRouterAction(c_h2): %v", err)
		}
		if _, ok := got.(*routerActionH2); !ok {
			t.Errorf("c_h2 → %T; want *routerActionH2", got)
		}
	}
}
