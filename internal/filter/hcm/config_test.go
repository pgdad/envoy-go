package hcm

import (
	"os"
	"reflect"
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
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
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

// expectErr runs parseFilterTest with the modifier-built Any and asserts the
// returned error has the "hcm:" prefix and contains wantSubstr.
func expectErr(t *testing.T, modify func(*hcmv3.HttpConnectionManager), wantSubstr string) {
	t.Helper()
	cm := mkClusterManager(t)
	_, err := parseFilterTest(mkHCM(modify), cm)
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
	if _, err := parseFilterTest(mkHCM(nil), cm); err != nil {
		t.Fatalf("happy: %v", err)
	}
}

func TestParseFilter_WrongTypeURL(t *testing.T) {
	cm := mkClusterManager(t)
	other, _ := anypb.New(&wrapperspb.StringValue{Value: "x"})
	_, err := parseFilterTest(other, cm)
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
	_, err := parseFilterWithCtx(any, cm, ListenerCtx{HasTLS: false, AllowH2C: false}, stats.NewRegistry(), nil, testHTTPRegistry(), nil)
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
	if _, err := parseFilterWithCtx(any, cm, ListenerCtx{HasTLS: true}, stats.NewRegistry(), nil, testHTTPRegistry(), nil); err != nil {
		t.Errorf("HTTP2 + TLS should be accepted, got: %v", err)
	}
}

// TestParseFilter_CodecTypeHTTP2_AcceptsAllowH2C verifies that HTTP2 is
// accepted on a plaintext listener when AllowH2C=true.
func TestParseFilter_CodecTypeHTTP2_AcceptsAllowH2C(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP2 })
	if _, err := parseFilterWithCtx(any, cm, ListenerCtx{AllowH2C: true}, stats.NewRegistry(), nil, testHTTPRegistry(), nil); err != nil {
		t.Errorf("HTTP2 + allowH2C should be accepted, got: %v", err)
	}
}

// TestParseFilter_CodecTypeAUTO_Accepts_BothCases verifies that AUTO is
// accepted with or without TLS context.
func TestParseFilter_CodecTypeAUTO_Accepts_BothCases(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_AUTO })
	for _, lc := range []ListenerCtx{{HasTLS: false}, {HasTLS: true}} {
		if _, err := parseFilterWithCtx(any, cm, lc, stats.NewRegistry(), nil, testHTTPRegistry(), nil); err != nil {
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
		if _, err := parseFilterWithCtx(any, cm, lc, stats.NewRegistry(), nil, testHTTPRegistry(), nil); err != nil {
			t.Errorf("HTTP1 + lc=%+v should be accepted, got: %v", lc, err)
		}
	}
}

func TestParseFilter_CodecTypeHTTP3(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP3 }, "codec_type HTTP3")
}

func TestParseFilter_CodecTypeAUTO(t *testing.T) {
	cm := mkClusterManager(t)
	if _, err := parseFilterTest(mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_AUTO }), cm); err != nil {
		t.Errorf("AUTO should be accepted as alias for HTTP1: %v", err)
	}
}

func TestParseFilter_MissingStatPrefix(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) { h.StatPrefix = "" }, "stat_prefix")
}

// TestParseFilter_StatPrefixInvalidChars is the gate-(d) regression test for
// the FuzzHCMConfigParse crasher surfaced by verifier commit 1f94b74. A
// stat_prefix containing a literal space (or any character outside the
// internal metric-name regex's permitted [a-zA-Z0-9_.] class) caused
// stats.Registry.NewCounter to panic at registry.go:checkName when the
// assembled "http.<stat_prefix>.downstream_rq_total" name was registered.
// The contract: parseFilterTest MUST return an "hcm: invalid stat_prefix" error
// and MUST NOT panic. The "0000000000 0" case reproduces the verbatim
// minimized fuzz seed payload (12 bytes; literal SP at index 10).
func TestParseFilter_StatPrefixInvalidChars(t *testing.T) {
	cases := []string{
		"0000000000 0", // verbatim fuzz-seed minimized payload (gate-(d) crasher)
		"foo bar",      // simpler space form
		"foo-bar",      // dash
		"foo:bar",      // colon
		"foo/bar",      // slash
		"foo$bar",      // dollar
	}
	for _, prefix := range cases {
		t.Run(prefix, func(t *testing.T) {
			expectErr(t, func(h *hcmv3.HttpConnectionManager) { h.StatPrefix = prefix }, "invalid stat_prefix")
		})
	}
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

// TestParseFilter_HTTPFiltersEmpty verifies the rule-#1 error class per
// SPEC §1 #6 + ADR-0071: an empty http_filters[] is rejected with the
// canonical "must contain at least 1 entry (the router)" message. Renamed
// + asserted substring updated post-Task-13 (was: substring "http_filters",
// pre-Task-13's exactly-[router] rule).
func TestParseFilter_HTTPFiltersEmpty(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) { h.HttpFilters = nil }, "must contain at least 1 entry")
}

// TestParseFilter_HTTPFiltersTwoEntries verifies that two entries with the
// same name fall into rule-#3 (duplicate filter name) rather than the
// previous "exactly-[router]" rejection. The mkHCM-default + appended entry
// share name "envoy.filters.http.router" — exactly the duplicate case.
func TestParseFilter_HTTPFiltersTwoEntries(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.HttpFilters = append(h.HttpFilters, &hcmv3.HttpFilter{
			Name:       "envoy.filters.http.router",
			ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: mkRouter()},
		})
	}, "duplicate filter name")
}

// TestParseFilter_HTTPFiltersWrongName verifies the rule-#2 error class:
// when the last (and only) entry's name is not the router, the error
// reports "last entry must be ... (router)". Pre-Task-13 the substring
// was "name"; post-Task-13 the canonical wording per ADR-0071 is the
// "last entry must be" form.
func TestParseFilter_HTTPFiltersWrongName(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.HttpFilters[0].Name = "envoy.filters.http.cors"
	}, "last entry must be")
}

// TestParseFilter_HTTPFiltersWrongTypeURL verifies that when the last entry's
// name is router but typed_config.type_url is wrong, rule-#2 fires (last
// entry's (name, type_url) pair must equal the router pair). Pre-Task-13
// this surfaced as an inline typed_config type_url check; post-Task-13 the
// shape-validator reports the same condition via the unified "last entry
// must be" message — which renders the actual type_url after the comma.
func TestParseFilter_HTTPFiltersWrongTypeURL(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		other, _ := anypb.New(&wrapperspb.StringValue{Value: "x"})
		h.HttpFilters[0].ConfigType = &hcmv3.HttpFilter_TypedConfig{TypedConfig: other}
	}, "last entry must be")
}

// TestParseFilterWithCtx_RejectsEmptyChain is the rule-#1 acceptance test
// (per Task 13 §Acceptance bullet 1). Asserts the verbatim canonical error
// "hcm: http_filters: must contain at least 1 entry (the router)" — exact
// text per SPEC §1 #6 + ADR-0071's partial supersession of ADR-0042.
func TestParseFilterWithCtx_RejectsEmptyChain(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.HttpFilters = nil })
	_, err := parseFilterTest(any, cm)
	if err == nil {
		t.Fatal("expected error for empty http_filters[], got nil")
	}
	wantExact := `hcm: http_filters: must contain at least 1 entry (the router)`
	if err.Error() != wantExact {
		t.Errorf("error = %q, want exact %q", err.Error(), wantExact)
	}
}

// TestParseFilterWithCtx_RejectsNonRouterTerminal is the rule-#2 acceptance
// test (per Task 13 §Acceptance bullet 1). The chain has a single entry
// whose name is not the router — error must be the canonical "last entry
// must be ..." form.
func TestParseFilterWithCtx_RejectsNonRouterTerminal(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		h.HttpFilters[0].Name = "envoy.filters.http.cors"
	})
	_, err := parseFilterTest(any, cm)
	if err == nil {
		t.Fatal("expected error for non-router terminal, got nil")
	}
	want := `hcm: http_filters: last entry must be "envoy.filters.http.router" (router); got "envoy.filters.http.cors"`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want substring %q", err.Error(), want)
	}
}

// TestParseFilterWithCtx_RejectsDuplicateFilterName is the rule-#3 acceptance
// test (per Task 13 §Acceptance bullet 1). The chain has two entries that
// share a name — error must be the canonical "duplicate filter name" form.
func TestParseFilterWithCtx_RejectsDuplicateFilterName(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		// Append a second router-named entry — two entries with name
		// "envoy.filters.http.router" → rule #3 fires on the second iter.
		h.HttpFilters = append(h.HttpFilters, &hcmv3.HttpFilter{
			Name:       "envoy.filters.http.router",
			ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: mkRouter()},
		})
	})
	_, err := parseFilterTest(any, cm)
	if err == nil {
		t.Fatal("expected error for duplicate filter name, got nil")
	}
	wantExact := `hcm: http_filters: duplicate filter name "envoy.filters.http.router"`
	if err.Error() != wantExact {
		t.Errorf("error = %q, want exact %q", err.Error(), wantExact)
	}
}

// TestParseFilterWithCtx_RejectsUnknownTypeURL is the rule-#4 acceptance test
// (per Task 13 §Acceptance bullet 1). A non-terminal entry's type_url is not
// in the registry — error must be the canonical "unknown type_url" form
// reporting the bad type_url + the registry's known set. We construct an
// adversarial chain with a bogus-typed-non-terminal then router as terminal
// (so rule #2 passes; rule #4 fires on the first iteration).
func TestParseFilterWithCtx_RejectsUnknownTypeURL(t *testing.T) {
	cm := mkClusterManager(t)
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		bogus, _ := anypb.New(&wrapperspb.StringValue{Value: "x"})
		// Prepend a non-terminal entry with an unknown type_url. Last entry
		// remains the default router → rule #2 passes; rule #4 fires.
		h.HttpFilters = append(
			[]*hcmv3.HttpFilter{{
				Name:       "envoy.filters.http.unknown",
				ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: bogus},
			}},
			h.HttpFilters...,
		)
	})
	_, err := parseFilterTest(any, cm)
	if err == nil {
		t.Fatal("expected error for unknown type_url, got nil")
	}
	want := `hcm: http_filters[0]: unknown type_url "type.googleapis.com/google.protobuf.StringValue"`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want substring %q", err.Error(), want)
	}
	// Also assert "registry: known are" appears so the actionable-output
	// contract from PLAN Step 2 is covered.
	if !strings.Contains(err.Error(), "registry: known are") {
		t.Errorf("error = %q, missing 'registry: known are' actionable suffix", err.Error())
	}
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
	// Now that weighted_clusters is supported (Task 6), an empty cluster list is
	// the shallowest reject path rather than the old "cluster_specifier not supported" arm.
	expectErr(t, func(h *hcmv3.HttpConnectionManager) {
		h.GetRouteConfig().VirtualHosts[0].Routes[0].Action = &routev3.Route_Route{Route: &routev3.RouteAction{
			ClusterSpecifier: &routev3.RouteAction_WeightedClusters{WeightedClusters: &routev3.WeightedCluster{}},
		}}
	}, "weighted_clusters has no clusters")
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
	if _, err := parseFilterTest(any, cm); err != nil {
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

// TestBuildRouterAction_ReturnsClusterRouteAction verifies the post-Task-15
// shape of buildRouterAction: both H1 and H2 cluster types resolve to a
// *clusterRouteAction wrapping the cluster handle. The H1/H2 variant
// selection is now deferred to the chain-mediated dispatch path: the
// router-package's H1ClusterAction (called via clusterRouteAction.asRouterAction)
// drives the H1 upstream-dial loop; the H2 chain wiring lands at Task 16.
//
// Pre-Task-15 (deleted): the test asserted distinct *routerAction (H1) vs
// *routerActionH2 (H2) return types; both are package-private to
// internal/filter/http/router post-Task-11 and the variant selection moved
// into the router package's closure-builder. The H2-side dispatch invariant
// is exercised in h2dispatch_test.go (which currently fails to build per
// Task 16's territory).
func TestBuildRouterAction_ReturnsClusterRouteAction(t *testing.T) {
	cm := mkH2ClusterManager(t)

	// H1 cluster → *clusterRouteAction wrapping c_h1.
	{
		ra := &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_h1"}}
		got, err := buildRouterAction(ra, cm)
		if err != nil {
			t.Fatalf("buildRouterAction(c_h1): %v", err)
		}
		bridge, ok := got.(*clusterRouteAction)
		if !ok {
			t.Fatalf("c_h1 → %T; want *clusterRouteAction", got)
		}
		if bridge.cluster == nil {
			t.Errorf("c_h1: bridge.cluster is nil; want non-nil cluster handle")
		}
	}

	// H2 cluster → *clusterRouteAction wrapping c_h2 (H2 chain wiring at Task 16).
	{
		ra := &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_h2"}}
		got, err := buildRouterAction(ra, cm)
		if err != nil {
			t.Fatalf("buildRouterAction(c_h2): %v", err)
		}
		bridge, ok := got.(*clusterRouteAction)
		if !ok {
			t.Fatalf("c_h2 → %T; want *clusterRouteAction", got)
		}
		if bridge.cluster == nil {
			t.Errorf("c_h2: bridge.cluster is nil; want non-nil cluster handle")
		}
	}
}

// TestFilter_AccessLogField_Plumbed verifies that the accessLog field is
// propagated from parseFilterWithCtx into the returned *Filter. This is the
// Task 11 plumbing regression test; Task 14 wires real AsyncFileSinks.
func TestFilter_AccessLogField_Plumbed(t *testing.T) {
	// Direct struct literal: field must be settable and readable.
	sink := &struct{ accesslog.Sink }{}
	_ = sink // suppress unused-variable lint; field presence is the assertion
	f := &Filter{accessLog: []accesslog.Sink{}}
	if f.accessLog == nil {
		t.Error("accessLog field: nil after setting empty slice")
	}

	// Round-trip via parseFilterWithCtx: non-nil sink slice must be stored.
	cm := mkClusterManager(t)
	any := mkHCM(nil)
	sinks := []accesslog.Sink{}
	got, err := parseFilterWithCtx(any, cm, ListenerCtx{}, stats.NewRegistry(), sinks, testHTTPRegistry(), nil)
	if err != nil {
		t.Fatalf("parseFilterWithCtx: %v", err)
	}
	if got.accessLog == nil {
		t.Error("parseFilterWithCtx: accessLog field is nil, want non-nil slice")
	}
}

// TestParseHTTPFiltersChain_FactoryCtxThreading verifies that
// parseHTTPFiltersChain threads the *stats.Registry + stat_prefix into the
// FactoryCtx supplied to each per-filter HTTPFilterFactory. Per Phase 09
// Task 2 (FactoryCtx framework extension; ADR-0100 first-use anchor): the
// extension exists so stats-bearing per-filter factories (fault, future
// header_mutation, jwt_authn, etc.) can register their stat names at
// HCM-build time per ADR-0061's pre-Freeze discipline. Existing non-stat-
// bearing filters (router, cors, envoygotest) ignore the FactoryCtx Stats +
// StatPrefix fields gracefully (per ADR-0085 nil-tolerance pattern).
func TestParseHTTPFiltersChain_FactoryCtxThreading(t *testing.T) {
	var captured filter_http.FactoryCtx
	testFactory := filter_http.HTTPFilterFactory(func(_ *anypb.Any, ctx filter_http.FactoryCtx) (filter_http.FilterInstanceFactory, error) {
		captured = ctx
		return func() filter_http.HTTPFilter { return filter_http.HTTPFilter{Name: "test.factoryctx"} }, nil
	})
	httpReg := filter_http.NewHTTPRegistry()
	httpReg.Register("type.googleapis.com/test.FactoryCtxProbe", testFactory)
	httpReg.Register(router.TypeURL, router.New)
	httpReg.Freeze()

	reg := stats.NewRegistry()
	statPrefix := "ingress_http"

	filters := []*hcmv3.HttpFilter{
		{Name: "test.factoryctx", ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: &anypb.Any{TypeUrl: "type.googleapis.com/test.FactoryCtxProbe"}}},
		{Name: "envoy.filters.http.router", ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: mkRouter()}},
	}
	_, err := parseHTTPFiltersChain(filters, nil /*clusters — not exercised by FactoryCtxProbe*/, nil /*httpClient — not exercised by FactoryCtxProbe*/, httpReg, reg, statPrefix, "" /*nodeServiceCluster — not exercised by FactoryCtxProbe*/)
	if err != nil {
		t.Fatalf("parseHTTPFiltersChain: %v", err)
	}
	if captured.Stats != reg {
		t.Errorf("FactoryCtx.Stats: got %p, want %p", captured.Stats, reg)
	}
	if captured.StatPrefix != statPrefix {
		t.Errorf("FactoryCtx.StatPrefix: got %q, want %q", captured.StatPrefix, statPrefix)
	}
}

// TestParseRouteHashPolicies asserts the §6 DEPARTURE-reject roster: header +
// connection_properties.source_ip are SUPPORTED and lower into proto-free
// router.HashPolicy descriptors; cookie / query_parameter / filter_state +
// a configured regex_rewrite + source_ip==false DEPARTURE-reject fail-fast;
// an empty header_name PARITY-rejects the PGV min_len=1. Header names lower.
func TestParseRouteHashPolicies(t *testing.T) {
	hdr := func(name string) *routev3.RouteAction_HashPolicy {
		return &routev3.RouteAction_HashPolicy{PolicySpecifier: &routev3.RouteAction_HashPolicy_Header_{
			Header: &routev3.RouteAction_HashPolicy_Header{HeaderName: name}}}
	}
	hdrTerminal := func(name string) *routev3.RouteAction_HashPolicy {
		hp := hdr(name)
		hp.Terminal = true
		return hp
	}
	hdrRegex := func(name string) *routev3.RouteAction_HashPolicy {
		return &routev3.RouteAction_HashPolicy{PolicySpecifier: &routev3.RouteAction_HashPolicy_Header_{
			Header: &routev3.RouteAction_HashPolicy_Header{
				HeaderName:   name,
				RegexRewrite: &matcherv3.RegexMatchAndSubstitute{Substitution: "x"},
			}}}
	}
	srcip := func(v bool) *routev3.RouteAction_HashPolicy {
		return &routev3.RouteAction_HashPolicy{PolicySpecifier: &routev3.RouteAction_HashPolicy_ConnectionProperties_{
			ConnectionProperties: &routev3.RouteAction_HashPolicy_ConnectionProperties{SourceIp: v}}}
	}
	cookie := &routev3.RouteAction_HashPolicy{PolicySpecifier: &routev3.RouteAction_HashPolicy_Cookie_{
		Cookie: &routev3.RouteAction_HashPolicy_Cookie{Name: "sess"}}}
	queryParam := &routev3.RouteAction_HashPolicy{PolicySpecifier: &routev3.RouteAction_HashPolicy_QueryParameter_{
		QueryParameter: &routev3.RouteAction_HashPolicy_QueryParameter{Name: "q"}}}
	filterState := &routev3.RouteAction_HashPolicy{PolicySpecifier: &routev3.RouteAction_HashPolicy_FilterState_{
		FilterState: &routev3.RouteAction_HashPolicy_FilterState{Key: "k"}}}

	tests := []struct {
		name    string
		in      []*routev3.RouteAction_HashPolicy
		wantErr string // "" = accept
		want    []router.HashPolicy
	}{
		{"nil", nil, "", nil},
		{"header", []*routev3.RouteAction_HashPolicy{hdr("x-hash")}, "", []router.HashPolicy{{Kind: router.HashKindHeader, HeaderName: "x-hash"}}},
		{"header-upper-lowered", []*routev3.RouteAction_HashPolicy{hdr("X-Hash")}, "", []router.HashPolicy{{Kind: router.HashKindHeader, HeaderName: "x-hash"}}},
		{"source_ip", []*routev3.RouteAction_HashPolicy{srcip(true)}, "", []router.HashPolicy{{Kind: router.HashKindSourceIP}}},
		{"header-terminal", []*routev3.RouteAction_HashPolicy{hdrTerminal("x-hash")}, "", []router.HashPolicy{{Kind: router.HashKindHeader, HeaderName: "x-hash", Terminal: true}}},
		{"source_ip-terminal", []*routev3.RouteAction_HashPolicy{func() *routev3.RouteAction_HashPolicy {
			hp := srcip(true)
			hp.Terminal = true
			return hp
		}()}, "", []router.HashPolicy{{Kind: router.HashKindSourceIP, Terminal: true}}},
		{"header-then-source_ip", []*routev3.RouteAction_HashPolicy{hdr("x-hash"), srcip(true)}, "", []router.HashPolicy{
			{Kind: router.HashKindHeader, HeaderName: "x-hash"},
			{Kind: router.HashKindSourceIP},
		}},
		{"empty-header-name", []*routev3.RouteAction_HashPolicy{hdr("")}, "value length must be at least 1 runes", nil},
		{"regex-rewrite", []*routev3.RouteAction_HashPolicy{hdrRegex("x-hash")}, "regex_rewrite is not supported", nil},
		{"source_ip-false", []*routev3.RouteAction_HashPolicy{srcip(false)}, "connection_properties without source_ip", nil},
		{"cookie", []*routev3.RouteAction_HashPolicy{cookie}, "is not supported (only header, connection_properties.source_ip)", nil},
		{"query_parameter", []*routev3.RouteAction_HashPolicy{queryParam}, "is not supported (only header, connection_properties.source_ip)", nil},
		{"filter_state", []*routev3.RouteAction_HashPolicy{filterState}, "is not supported (only header, connection_properties.source_ip)", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRouteHashPolicies(tt.in)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("accept expected, got %v", err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Fatalf("descriptor: got %+v want %+v", got, tt.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want err containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// TestParseRouteSubsetMatch_LowersScalars verifies buildRouterAction lowers a
// route's metadata_match["envoy.lb"] scalar struct to a route-static
// cluster.SubsetMatch on the clusterRouteAction (ADR-0239 — the producer half).
func TestParseRouteSubsetMatch_LowersScalars(t *testing.T) {
	cm := mkH2ClusterManager(t)
	md, _ := structpb.NewStruct(map[string]any{"version": "v1"})
	ra := &routev3.RouteAction{
		ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_h1"},
		MetadataMatch:    &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"envoy.lb": md}},
	}
	act, err := buildRouterAction(ra, cm)
	if err != nil {
		t.Fatal(err)
	}
	cra := act.(*clusterRouteAction)
	// Build the expected match the SAME way the producer does (the SubsetValue
	// Kind constants are unexported in the cluster package — can't be named here).
	scalars, _ := cluster.ScalarsFromStruct(md)
	want := cluster.NewSubsetMatch(scalars)
	if cra.subsetMatch.Key() != want.Key() {
		t.Errorf("subsetMatch mismatch: %q vs %q", cra.subsetMatch.Key(), want.Key())
	}
}

// TestParseRouteSubsetMatch_RejectsNonScalar verifies a non-scalar
// metadata_match["envoy.lb"] value fail-fast-rejects at config-build (the
// scalar MVP boundary — router-metadata-match-nonscalar; ADR-0239 / SPEC §6.2).
func TestParseRouteSubsetMatch_RejectsNonScalar(t *testing.T) {
	cm := mkH2ClusterManager(t)
	md, _ := structpb.NewStruct(map[string]any{"version": map[string]any{"nested": 1}}) // struct value
	ra := &routev3.RouteAction{
		ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_h1"},
		MetadataMatch:    &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"envoy.lb": md}},
	}
	_, err := buildRouterAction(ra, cm)
	if err == nil || !strings.Contains(err.Error(), "only scalar values") {
		t.Errorf("err = %v, want the non-scalar reject (router-metadata-match-nonscalar)", err)
	}
}

// TestParseRouteSubsetMatch_NoMetadataIsEmpty verifies a route with no
// metadata_match yields the empty (no-match) SubsetMatch — the fallback path.
func TestParseRouteSubsetMatch_NoMetadataIsEmpty(t *testing.T) {
	cm := mkH2ClusterManager(t)
	ra := &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_h1"}}
	act, err := buildRouterAction(ra, cm)
	if err != nil {
		t.Fatal(err)
	}
	if !act.(*clusterRouteAction).subsetMatch.Empty() {
		t.Error("no metadata_match → empty subsetMatch (fallback path)")
	}
}

func TestMergeRouteSubsetMatch(t *testing.T) {
	mdLB := func(kv map[string]any) *corev3.Metadata {
		fields := map[string]*structpb.Value{}
		for k, v := range kv {
			val, _ := structpb.NewValue(v)
			fields[k] = val
		}
		return &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{
			"envoy.lb": {Fields: fields},
		}}
	}
	// route {version:v1, stage:prod}, entry {version:v2} → merged {version:v2, stage:prod}.
	route := mdLB(map[string]any{"version": "v1", "stage": "prod"})
	entry := mdLB(map[string]any{"version": "v2"})
	merged, err := mergeRouteSubsetMatch(route, entry)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	expect := mergeExpect(t, map[string]any{"version": "v2", "stage": "prod"})
	if merged.Key() != expect.Key() {
		t.Errorf("merged.Key()=%q want %q (entry precedence)", merged.Key(), expect.Key())
	}
	// non-scalar on the entry side rejects.
	bad := &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{
		"envoy.lb": {Fields: map[string]*structpb.Value{
			"list": structpb.NewListValue(&structpb.ListValue{}),
		}},
	}}
	if _, err := mergeRouteSubsetMatch(nil, bad); err == nil {
		t.Errorf("expected non-scalar reject")
	}
}

// newTestManager builds a *cluster.Manager containing one minimal STATIC cluster
// per name (single endpoint 127.0.0.1:1 — never dialed; tests only resolve by name).
func newTestManager(t *testing.T, names ...string) *cluster.Manager {
	t.Helper()
	clusters := make([]*clusterv3.Cluster, 0, len(names))
	for _, name := range names {
		clusters = append(clusters, &clusterv3.Cluster{
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
									PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 1},
								},
							}},
						}},
					}},
				}},
			},
		})
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{Clusters: clusters},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("newTestManager: %v", err)
	}
	return cm
}

func TestBuildWeightedRouterAction_Rejects(t *testing.T) {
	mgr := newTestManager(t, "c_a", "c_b")
	w := func(clusters ...*routev3.WeightedCluster_ClusterWeight) *routev3.RouteAction {
		return &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_WeightedClusters{
			WeightedClusters: &routev3.WeightedCluster{Clusters: clusters},
		}}
	}
	cw := func(name string, weight *wrapperspb.UInt32Value) *routev3.WeightedCluster_ClusterWeight {
		return &routev3.WeightedCluster_ClusterWeight{Name: name, Weight: weight}
	}
	u := func(v uint32) *wrapperspb.UInt32Value { return wrapperspb.UInt32(v) }

	cases := []struct {
		name string
		ra   *routev3.RouteAction
		want string // substring of the expected error
	}{
		{"empty-clusters", w(), "weighted_clusters has no clusters"},
		{"name-required", w(cw("", u(1))), "name is required"},
		{"weight-required", w(cw("c_a", nil)), "weight is required"},
		{"sum-zero", w(cw("c_a", u(0)), cw("c_b", u(0))), "total weight must be > 0"},
		{"dangling", w(cw("ghost", u(1))), `cluster "ghost" not found`},
		{"cluster-header", func() *routev3.RouteAction {
			// Name set AND cluster_header set → after the name-presence check passes,
			// the cluster_header-unsupported arm fires (the reference's both-set reject).
			r := w(cw("c_a", u(1)))
			r.GetWeightedClusters().Clusters[0].ClusterHeader = "x-cluster"
			return r
		}(), "cluster_header is not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildRouterAction(tc.ra, mgr)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want substring %q", err, tc.want)
			}
		})
	}
}

func TestBuildWeightedRouterAction_AcceptsRecognized(t *testing.T) {
	mgr := newTestManager(t, "c_a", "c_b")
	ra := &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_WeightedClusters{
		WeightedClusters: &routev3.WeightedCluster{Clusters: []*routev3.WeightedCluster_ClusterWeight{
			{Name: "c_a", Weight: wrapperspb.UInt32(50)},
			{Name: "c_b", Weight: wrapperspb.UInt32(50)},
		}},
	}}
	act, err := buildRouterAction(ra, mgr)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, ok := act.(*weightedClusterRouteAction); !ok {
		t.Fatalf("got %T want *weightedClusterRouteAction", act)
	}
}

// mergeExpect builds a SubsetMatch from a scalar map via the production lowering.
func mergeExpect(t *testing.T, kv map[string]any) cluster.SubsetMatch {
	t.Helper()
	fields := map[string]*structpb.Value{}
	for k, v := range kv {
		val, _ := structpb.NewValue(v)
		fields[k] = val
	}
	m, non := cluster.ScalarsFromStruct(&structpb.Struct{Fields: fields})
	if len(non) > 0 {
		t.Fatalf("mergeExpect non-scalar %v", non)
	}
	return cluster.NewSubsetMatch(m)
}

// TestBuildWeightedRouterAction_AcceptCases covers two edge-case accept paths
// that are NOT exercised by TestBuildWeightedRouterAction_AcceptsRecognized:
//
//  1. An explicit weight:0 on ONE entry is accepted (sum > 0).
//  2. A non-matching TotalWeight field is ignored (deprecated; envoy-go uses the sum).
func TestBuildWeightedRouterAction_AcceptCases(t *testing.T) {
	mgr := newTestManager(t, "c_a", "c_b")
	mk := func(mut func(*routev3.WeightedCluster)) *routev3.RouteAction {
		wc := &routev3.WeightedCluster{Clusters: []*routev3.WeightedCluster_ClusterWeight{
			{Name: "c_a", Weight: wrapperspb.UInt32(50)},
			{Name: "c_b", Weight: wrapperspb.UInt32(50)},
		}}
		if mut != nil {
			mut(wc)
		}
		return &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_WeightedClusters{WeightedClusters: wc}}
	}
	// explicit-0 entry accepts (sum>0).
	if act, err := buildRouterAction(mk(func(wc *routev3.WeightedCluster) {
		wc.Clusters[0].Weight = wrapperspb.UInt32(0)
	}), mgr); err != nil {
		t.Errorf("explicit-0 entry should accept: %v", err)
	} else if _, ok := act.(*weightedClusterRouteAction); !ok {
		t.Errorf("explicit-0: got %T want *weightedClusterRouteAction", act)
	}
	// non-matching total_weight ignored (field is deprecated; envoy-go uses the sum).
	if _, err := buildRouterAction(mk(func(wc *routev3.WeightedCluster) {
		wc.TotalWeight = wrapperspb.UInt32(999) //nolint:staticcheck // intentional: deprecated total_weight ignored per ADR-0080
	}), mgr); err != nil {
		t.Errorf("non-matching total_weight should accept (deprecated/ignored): %v", err)
	}
}

// TestBuildWeightedRouterAction_MergeRuns confirms the wired merge path (route-level
// metadata_match + entry-level ClusterWeight.metadata_match both set) builds without
// error and returns a *weightedClusterRouteAction. Merge-precedence values are
// unit-tested in TestMergeRouteSubsetMatch (Task 3); behavioral proof is in the
// 0065 fixture (Task 8). This test only proves the wired path doesn't error.
func TestBuildWeightedRouterAction_MergeRuns(t *testing.T) {
	mgr := newTestManager(t, "c_a", "c_b")

	// route-level metadata_match: envoy.lb {version:v1, stage:prod}
	routeMD, _ := structpb.NewStruct(map[string]any{"version": "v1", "stage": "prod"})
	// entry-level metadata_match for c_a: envoy.lb {version:v2} (overrides version)
	entryMD, _ := structpb.NewStruct(map[string]any{"version": "v2"})

	ra := &routev3.RouteAction{
		MetadataMatch: &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"envoy.lb": routeMD}},
		ClusterSpecifier: &routev3.RouteAction_WeightedClusters{
			WeightedClusters: &routev3.WeightedCluster{
				Clusters: []*routev3.WeightedCluster_ClusterWeight{
					{
						Name:          "c_a",
						Weight:        wrapperspb.UInt32(50),
						MetadataMatch: &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"envoy.lb": entryMD}},
					},
					{Name: "c_b", Weight: wrapperspb.UInt32(50)},
				},
			},
		},
	}
	act, err := buildRouterAction(ra, mgr)
	if err != nil {
		t.Fatalf("merge-runs: unexpected error: %v", err)
	}
	if _, ok := act.(*weightedClusterRouteAction); !ok {
		t.Fatalf("merge-runs: got %T want *weightedClusterRouteAction", act)
	}
}

// TestBuildWeightedRouterAction_EntryNonScalarRejects proves the non-scalar reject
// path fires through the full buildRouterAction → buildWeightedRouterAction →
// mergeRouteSubsetMatch wiring — not just in the helper in isolation.
//
// Two sub-cases for symmetry:
//  1. Entry ClusterWeight.metadata_match carries a non-scalar (list) → reject.
//  2. Route-level RouteAction.metadata_match carries a non-scalar (list) → reject.
func TestBuildWeightedRouterAction_EntryNonScalarRejects(t *testing.T) {
	mgr := newTestManager(t, "c_a", "c_b")

	nonScalarMD := func() *corev3.Metadata {
		return &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{
			"envoy.lb": {Fields: map[string]*structpb.Value{
				"bad": structpb.NewListValue(&structpb.ListValue{}),
			}},
		}}
	}

	// Case 1: non-scalar on the ENTRY (ClusterWeight.metadata_match).
	t.Run("entry-non-scalar", func(t *testing.T) {
		ra := &routev3.RouteAction{
			ClusterSpecifier: &routev3.RouteAction_WeightedClusters{
				WeightedClusters: &routev3.WeightedCluster{
					Clusters: []*routev3.WeightedCluster_ClusterWeight{
						{Name: "c_a", Weight: wrapperspb.UInt32(50), MetadataMatch: nonScalarMD()},
						{Name: "c_b", Weight: wrapperspb.UInt32(50)},
					},
				},
			},
		}
		_, err := buildRouterAction(ra, mgr)
		if err == nil || !strings.Contains(err.Error(), "only scalar values") {
			t.Errorf("entry non-scalar: err=%v want substring %q", err, "only scalar values")
		}
	})

	// Case 2: non-scalar on the ROUTE level (RouteAction.metadata_match).
	t.Run("route-non-scalar", func(t *testing.T) {
		ra := &routev3.RouteAction{
			MetadataMatch: nonScalarMD(),
			ClusterSpecifier: &routev3.RouteAction_WeightedClusters{
				WeightedClusters: &routev3.WeightedCluster{
					Clusters: []*routev3.WeightedCluster_ClusterWeight{
						{Name: "c_a", Weight: wrapperspb.UInt32(50)},
						{Name: "c_b", Weight: wrapperspb.UInt32(50)},
					},
				},
			},
		}
		_, err := buildRouterAction(ra, mgr)
		if err == nil || !strings.Contains(err.Error(), "only scalar values") {
			t.Errorf("route non-scalar: err=%v want substring %q", err, "only scalar values")
		}
	})
}

// TestBuildRouterAction_RetryPolicyParse exercises the phase-42.1 Task 4
// retry_policy parse + the vhost→route fallback + the rejects. The single-cluster
// arm (mkH2ClusterManager's c_h1) carries the parsed *router.RetryPolicy on the
// returned *clusterRouteAction. The fallback is fed through buildRouterAction's
// vhRetryPolicy param (the same value buildRouteTable threads from
// vh.GetRetryPolicy()).
func TestBuildRouterAction_RetryPolicyParse(t *testing.T) {
	cm := mkH2ClusterManager(t)
	mkRA := func() *routev3.RouteAction {
		return &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_h1"}}
	}
	getRP := func(t *testing.T, got routeAction) *router.RetryPolicy {
		t.Helper()
		bridge, ok := got.(*clusterRouteAction)
		if !ok {
			t.Fatalf("got %T; want *clusterRouteAction", got)
		}
		return bridge.retryPolicy
	}

	// Route-level retry_policy{retry_on:"5xx", num_retries:3} ⇒ parsed, numRetries==3.
	t.Run("route-level", func(t *testing.T) {
		ra := mkRA()
		ra.RetryPolicy = &routev3.RetryPolicy{RetryOn: "5xx", NumRetries: wrapperspb.UInt32(3)}
		got, err := buildRouterAction(ra, cm)
		if err != nil {
			t.Fatalf("buildRouterAction: %v", err)
		}
		rp := getRP(t, got)
		if rp == nil {
			t.Fatal("retryPolicy is nil; want a parsed policy")
		}
		if rp.NumRetries() != 3 {
			t.Errorf("NumRetries()=%d; want 3", rp.NumRetries())
		}
	})

	// No route-level retry_policy + a vhost retry_policy ⇒ inherit the vhost.
	t.Run("vhost-inherit", func(t *testing.T) {
		vh := &routev3.RetryPolicy{RetryOn: "5xx", NumRetries: wrapperspb.UInt32(5)}
		got, err := buildRouterActionWithVH(mkRA(), "r_inherit", cm, vh, nil)
		if err != nil {
			t.Fatalf("buildRouterAction: %v", err)
		}
		rp := getRP(t, got)
		if rp == nil {
			t.Fatal("retryPolicy is nil; want the inherited vhost policy")
		}
		if rp.NumRetries() != 5 {
			t.Errorf("NumRetries()=%d; want 5 (inherited vhost)", rp.NumRetries())
		}
	})

	// Route-level retry_policy OVERRIDES the vhost.
	t.Run("route-overrides-vhost", func(t *testing.T) {
		ra := mkRA()
		ra.RetryPolicy = &routev3.RetryPolicy{RetryOn: "5xx", NumRetries: wrapperspb.UInt32(2)}
		vh := &routev3.RetryPolicy{RetryOn: "5xx", NumRetries: wrapperspb.UInt32(9)}
		got, err := buildRouterActionWithVH(ra, "r_override", cm, vh, nil)
		if err != nil {
			t.Fatalf("buildRouterAction: %v", err)
		}
		rp := getRP(t, got)
		if rp == nil {
			t.Fatal("retryPolicy is nil; want the route override")
		}
		if rp.NumRetries() != 2 {
			t.Errorf("NumRetries()=%d; want 2 (route overrides vhost)", rp.NumRetries())
		}
	})

	// Neither route nor vhost ⇒ retryPolicy stays nil (byte-stable path).
	t.Run("neither", func(t *testing.T) {
		got, err := buildRouterAction(mkRA(), cm)
		if err != nil {
			t.Fatalf("buildRouterAction: %v", err)
		}
		if rp := getRP(t, got); rp != nil {
			t.Errorf("retryPolicy=%v; want nil when no retry_policy is set", rp)
		}
	})

	// retry_back_off{base:100ms, max:50ms} ⇒ the max<base reject.
	t.Run("max-lt-base-reject", func(t *testing.T) {
		ra := mkRA()
		ra.RetryPolicy = &routev3.RetryPolicy{
			RetryOn: "5xx",
			RetryBackOff: &routev3.RetryPolicy_RetryBackOff{
				BaseInterval: durationpb.New(100 * time.Millisecond),
				MaxInterval:  durationpb.New(50 * time.Millisecond),
			},
		}
		_, err := buildRouterAction(ra, cm)
		if err == nil || !strings.Contains(err.Error(), "max_interval must be greater than or equal to the base_interval") {
			t.Errorf("err=%v; want the max<base reject", err)
		}
	})

	// retry_back_off set with base_interval=0 (explicitly) ⇒ the base reject.
	t.Run("base-required-reject", func(t *testing.T) {
		ra := mkRA()
		ra.RetryPolicy = &routev3.RetryPolicy{
			RetryOn: "5xx",
			RetryBackOff: &routev3.RetryPolicy_RetryBackOff{
				BaseInterval: durationpb.New(0),
			},
		}
		_, err := buildRouterAction(ra, cm)
		if err == nil || !strings.Contains(err.Error(), "base_interval must be greater than 0s") {
			t.Errorf("err=%v; want the base_interval-required reject", err)
		}
	})

	// per_try_timeout: 250ms ⇒ PerTryTimeout()==250ms (Task 4 parse).
	t.Run("per-try-timeout-set", func(t *testing.T) {
		ra := mkRA()
		ra.RetryPolicy = &routev3.RetryPolicy{
			RetryOn:       "5xx",
			NumRetries:    wrapperspb.UInt32(2),
			PerTryTimeout: durationpb.New(250 * time.Millisecond),
		}
		got, err := buildRouterAction(ra, cm)
		if err != nil {
			t.Fatalf("buildRouterAction: %v", err)
		}
		rp := getRP(t, got)
		if rp == nil {
			t.Fatal("retryPolicy is nil; want a parsed policy")
		}
		if rp.PerTryTimeout() != 250*time.Millisecond {
			t.Errorf("PerTryTimeout()=%v; want 250ms", rp.PerTryTimeout())
		}
	})

	// per_try_timeout: -1s ⇒ route-scoped negative-per-try-timeout reject.
	t.Run("per-try-timeout-negative-reject", func(t *testing.T) {
		ra := mkRA()
		ra.RetryPolicy = &routev3.RetryPolicy{
			RetryOn:       "5xx",
			PerTryTimeout: durationpb.New(-1 * time.Second),
		}
		_, err := buildRouterAction(ra, cm)
		want := `route: "c_h1": retry_policy: per_try_timeout must not be negative`
		if err == nil || err.Error() != want {
			t.Errorf("err=%v; want %q", err, want)
		}
	})

	// per_try_timeout unset ⇒ PerTryTimeout()==0 (no per-attempt bound).
	t.Run("per-try-timeout-unset", func(t *testing.T) {
		ra := mkRA()
		ra.RetryPolicy = &routev3.RetryPolicy{RetryOn: "5xx", NumRetries: wrapperspb.UInt32(1)}
		got, err := buildRouterAction(ra, cm)
		if err != nil {
			t.Fatalf("buildRouterAction: %v", err)
		}
		rp := getRP(t, got)
		if rp == nil {
			t.Fatal("retryPolicy is nil; want a parsed policy")
		}
		if rp.PerTryTimeout() != 0 {
			t.Errorf("PerTryTimeout()=%v; want 0 (unset)", rp.PerTryTimeout())
		}
	})
}

// TestBuildRouterAction_HedgePolicyParse exercises the phase-42.2b Task 7
// hedge_policy parse + the vhost→route fallback + the initial_requests<1 reject.
// The single-cluster arm (mkH2ClusterManager's c_h1) carries the parsed
// *router.HedgePolicy on the returned *clusterRouteAction. The fallback is fed
// through buildRouterActionWithVH's vhHedgePolicy param (the same value
// buildRouteTable threads from vh.GetHedgePolicy()). Mirrors the phase-42.1
// retry_policy parse tests above.
func TestBuildRouterAction_HedgePolicyParse(t *testing.T) {
	cm := mkH2ClusterManager(t)
	mkRA := func() *routev3.RouteAction {
		return &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_h1"}}
	}
	getHP := func(t *testing.T, got routeAction) *router.HedgePolicy {
		t.Helper()
		bridge, ok := got.(*clusterRouteAction)
		if !ok {
			t.Fatalf("got %T; want *clusterRouteAction", got)
		}
		return bridge.hedgePolicy
	}

	// Route-level hedge_policy{hedge_on_per_try_timeout:true} ⇒ parsed, flag set.
	t.Run("route-level", func(t *testing.T) {
		ra := mkRA()
		ra.HedgePolicy = &routev3.HedgePolicy{HedgeOnPerTryTimeout: true}
		got, err := buildRouterAction(ra, cm)
		if err != nil {
			t.Fatalf("buildRouterAction: %v", err)
		}
		hp := getHP(t, got)
		if hp == nil {
			t.Fatal("hedgePolicy is nil; want a parsed policy")
		}
		if !hp.HedgeOnPerTryTimeout {
			t.Errorf("HedgeOnPerTryTimeout=%v; want true", hp.HedgeOnPerTryTimeout)
		}
	})

	// No route-level hedge_policy + a vhost hedge_policy ⇒ inherit the vhost.
	t.Run("vhost-inherit", func(t *testing.T) {
		vh := &routev3.HedgePolicy{HedgeOnPerTryTimeout: true}
		got, err := buildRouterActionWithVH(mkRA(), "r_inherit", cm, nil, vh)
		if err != nil {
			t.Fatalf("buildRouterAction: %v", err)
		}
		hp := getHP(t, got)
		if hp == nil {
			t.Fatal("hedgePolicy is nil; want the inherited vhost policy")
		}
		if !hp.HedgeOnPerTryTimeout {
			t.Errorf("HedgeOnPerTryTimeout=%v; want true (inherited vhost)", hp.HedgeOnPerTryTimeout)
		}
	})

	// initial_requests:0 ⇒ the gte:1 reject (route-scoped, byte-stable suffix).
	t.Run("initial-requests-zero-reject", func(t *testing.T) {
		ra := mkRA()
		ra.HedgePolicy = &routev3.HedgePolicy{InitialRequests: &wrapperspb.UInt32Value{Value: 0}}
		_, err := buildRouterAction(ra, cm)
		want := `route: "c_h1": hedge_policy: initial_requests must be greater than or equal to 1`
		if err == nil || err.Error() != want {
			t.Errorf("err=%v; want %q", err, want)
		}
	})

	// Neither route nor vhost ⇒ hedgePolicy stays nil (byte-stable path).
	t.Run("neither", func(t *testing.T) {
		got, err := buildRouterAction(mkRA(), cm)
		if err != nil {
			t.Fatalf("buildRouterAction: %v", err)
		}
		if hp := getHP(t, got); hp != nil {
			t.Errorf("hedgePolicy=%v; want nil when no hedge_policy is set", hp)
		}
	})
}
