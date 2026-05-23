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
