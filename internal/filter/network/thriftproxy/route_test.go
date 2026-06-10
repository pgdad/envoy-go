package thriftproxy

import (
	"testing"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
)

func mkRouteConfig(rs ...*routev3.Route) *routev3.RouteConfiguration {
	return &routev3.RouteConfiguration{Routes: rs}
}
func mkRoute(method, cluster string) *routev3.Route {
	return &routev3.Route{
		Match: &routev3.RouteMatch{MatchSpecifier: &routev3.RouteMatch_MethodName{MethodName: method}},
		Route: &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: cluster}},
	}
}

func TestRouteMatch(t *testing.T) {
	rt, err := parseRouteConfig(mkRouteConfig(mkRoute("Ping", "c1"), mkRoute("", "fallback")))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c, ok := rt.match("Ping"); !ok || c != "c1" {
		t.Fatalf("match(Ping) = %q,%v want c1,true", c, ok)
	}
	if c, ok := rt.match("Other"); !ok || c != "fallback" {
		t.Fatalf("match(Other) = %q,%v want fallback,true", c, ok)
	}
}

func TestRouteMiss(t *testing.T) {
	rt, _ := parseRouteConfig(mkRouteConfig(mkRoute("Ping", "c1")))
	if c, ok := rt.match("Other"); ok {
		t.Fatalf("match(Other) = %q,%v want \"\",false", c, ok)
	}
	rt2, err := parseRouteConfig(nil)
	if err != nil {
		t.Fatalf("nil route config should parse OK: %v", err)
	}
	if _, ok := rt2.match("anything"); ok {
		t.Fatalf("nil route config should all-miss")
	}
}

func TestRouteParseRejects(t *testing.T) {
	tests := []struct {
		name    string
		rc      *routev3.RouteConfiguration
		wantErr string
	}{
		{"no-match", mkRouteConfig(&routev3.Route{Route: &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c"}}}), errRouteMatchRequired},
		{"no-action", mkRouteConfig(&routev3.Route{Match: &routev3.RouteMatch{MatchSpecifier: &routev3.RouteMatch_MethodName{MethodName: "m"}}}), errRouteActionRequired},
		{"empty-cluster", mkRouteConfig(&routev3.Route{Match: &routev3.RouteMatch{MatchSpecifier: &routev3.RouteMatch_MethodName{MethodName: "m"}}, Route: &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: ""}}}), errRouteClusterRequired},
		{"service-name-unsupported", mkRouteConfig(&routev3.Route{Match: &routev3.RouteMatch{MatchSpecifier: &routev3.RouteMatch_ServiceName{ServiceName: "s"}}, Route: &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c"}}}), errRouteMatchUnsupported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseRouteConfig(tc.rc); err == nil || err.Error() != tc.wantErr {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
