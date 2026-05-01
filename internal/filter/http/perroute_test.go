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
