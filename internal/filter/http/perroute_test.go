package http

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func mustAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func TestPerRoute_BuildAndResolve_RouteWins(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors", "envoy.filters.http.router"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("rc-level"))}
	vhCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("vh-level"))}
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("route-level"))}
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: rtCfg}}, chainNames)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	got := pr.Resolve("envoy.filters.http.cors", 0)
	if got == nil {
		t.Fatalf("expected non-nil resolve")
	}
	gotS, ok := got.(*wrapperspb.StringValue)
	if !ok || gotS.GetValue() != "route-level" {
		t.Fatalf("expected route-level wins; got %v", got)
	}
}

func TestPerRoute_BuildAndResolve_VHostFallback(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("rc-level"))}
	vhCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("vh-level"))}
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: nil}}, chainNames)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	got := pr.Resolve("envoy.filters.http.cors", 0).(*wrapperspb.StringValue)
	if got.GetValue() != "vh-level" {
		t.Fatalf("expected vh-level; got %s", got.GetValue())
	}
}

func TestPerRoute_BuildAndResolve_RCFallback(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("rc-level"))}
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	got := pr.Resolve("envoy.filters.http.cors", 0).(*wrapperspb.StringValue)
	if got.GetValue() != "rc-level" {
		t.Fatalf("expected rc-level; got %s", got.GetValue())
	}
}

func TestPerRoute_BuildAndResolve_NilOnAbsent(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors"}
	pr, err := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: nil}}, chainNames)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	if got := pr.Resolve("envoy.filters.http.cors", 0); got != nil {
		t.Fatalf("expected nil resolve when no scope carries a config; got %v", got)
	}
}

func TestPerRoute_BuildRejectsUnknownFilterName(t *testing.T) {
	chainNames := []string{"envoy.filters.http.router"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("oops"))}
	_, err := BuildPerRouteConfig(rcCfg, nil, chainNames)
	if err == nil {
		t.Fatalf("expected error on unknown filter name")
	}
	if !strings.Contains(err.Error(), "unknown filter name") || !strings.Contains(err.Error(), "envoy.filters.http.cors") {
		t.Fatalf("expected error to mention 'unknown filter name' + the filter name; got %q", err.Error())
	}
}

func TestPerRoute_LazyCacheHitMiss(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("rc"))}
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	a := pr.Resolve("envoy.filters.http.cors", 0)
	b := pr.Resolve("envoy.filters.http.cors", 0)
	if a != b {
		t.Fatalf("expected lazy-cache to return same proto.Message pointer on repeated lookup")
	}
}

func TestResolveAllTiers_AllThreeSet(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc-level"))}
	vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("vh-level"))}
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route-level"))}
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: rtCfg}}, chainNames)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if rs, ok := route.(*wrapperspb.StringValue); !ok || rs.GetValue() != "route-level" {
		t.Errorf("route: got %v, want route-level", route)
	}
	if vs, ok := vhost.(*wrapperspb.StringValue); !ok || vs.GetValue() != "vh-level" {
		t.Errorf("vhost: got %v, want vh-level", vhost)
	}
	if rcs, ok := rc.(*wrapperspb.StringValue); !ok || rcs.GetValue() != "rc-level" {
		t.Errorf("rc: got %v, want rc-level", rc)
	}
}

func TestResolveAllTiers_RouteAndVHostOnly(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("vh"))}
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route"))}
	pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: vhCfg, Route: rtCfg}}, chainNames)
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if route == nil || vhost == nil {
		t.Errorf("route+vhost should be non-nil; got route=%v vhost=%v", route, vhost)
	}
	if rc != nil {
		t.Errorf("rc should be nil; got %v", rc)
	}
}

func TestResolveAllTiers_RouteAndRCOnly(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route"))}
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames)
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if route == nil || rc == nil {
		t.Errorf("route+rc should be non-nil; got route=%v rc=%v", route, rc)
	}
	if vhost != nil {
		t.Errorf("vhost should be nil; got %v", vhost)
	}
}

func TestResolveAllTiers_VHostAndRCOnly(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
	vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("vh"))}
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: nil}}, chainNames)
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if vhost == nil || rc == nil {
		t.Errorf("vhost+rc should be non-nil; got vhost=%v rc=%v", vhost, rc)
	}
	if route != nil {
		t.Errorf("route should be nil; got %v", route)
	}
}

func TestResolveAllTiers_RouteOnly(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route"))}
	pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames)
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if route == nil {
		t.Errorf("route should be non-nil")
	}
	if vhost != nil || rc != nil {
		t.Errorf("vhost + rc should be nil; got vhost=%v rc=%v", vhost, rc)
	}
}

func TestResolveAllTiers_VHostOnly(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("vh"))}
	pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: vhCfg, Route: nil}}, chainNames)
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if vhost == nil {
		t.Errorf("vhost should be non-nil")
	}
	if route != nil || rc != nil {
		t.Errorf("route + rc should be nil; got route=%v rc=%v", route, rc)
	}
}

func TestResolveAllTiers_RCOnly(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames)
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if rc == nil {
		t.Errorf("rc should be non-nil")
	}
	if route != nil || vhost != nil {
		t.Errorf("route + vhost should be nil; got route=%v vhost=%v", route, vhost)
	}
}

func TestResolveAllTiers_NoneSet(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: nil}}, chainNames)
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if route != nil || vhost != nil || rc != nil {
		t.Errorf("all should be nil; got route=%v vhost=%v rc=%v", route, vhost, rc)
	}
}

func TestResolveAllTiers_RouteIdxOutOfRange(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames)
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 99)
	if route != nil || vhost != nil {
		t.Errorf("route+vhost should be nil for out-of-range routeIdx; got route=%v vhost=%v", route, vhost)
	}
	// RC is still consulted (not per-scope).
	if rc == nil {
		t.Errorf("rc should be non-nil even with out-of-range routeIdx")
	}
}

func TestResolveAllTiers_FilterNameNotPresent(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors", "envoy.filters.http.header_mutation"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("cors-rc"))}
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames)
	// Look up a filter name not present at any tier.
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if route != nil || vhost != nil || rc != nil {
		t.Errorf("all should be nil for absent filterName; got route=%v vhost=%v rc=%v", route, vhost, rc)
	}
}

func TestResolveAllTiers_DoesNotPolluteResolveCache(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route"))}
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames)
	// Call ResolveAllTiers first.
	route, _, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if route == nil || rc == nil {
		t.Fatalf("setup: route+rc should be non-nil")
	}
	// Then call Resolve and verify it returns route-level (most-specific override per ADR-0073).
	msg := pr.Resolve("envoy.filters.http.header_mutation", 0)
	if rs, ok := msg.(*wrapperspb.StringValue); !ok || rs.GetValue() != "route" {
		t.Errorf("Resolve after ResolveAllTiers should return route-level; got %v", msg)
	}
}

func TestResolveAllTiers_NilReceiver(t *testing.T) {
	var pr *PerRouteConfig
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if route != nil || vhost != nil || rc != nil {
		t.Errorf("nil receiver should return all-nil; got route=%v vhost=%v rc=%v", route, vhost, rc)
	}
}
