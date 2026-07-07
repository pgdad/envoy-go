package http

import (
	"errors"
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
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: rtCfg}}, chainNames, nil)
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
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: nil}}, chainNames, nil)
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
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames, nil)
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
	pr, err := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: nil}}, chainNames, nil)
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
	_, err := BuildPerRouteConfig(rcCfg, nil, chainNames, nil)
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
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames, nil)
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
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: rtCfg}}, chainNames, nil)
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
	pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: vhCfg, Route: rtCfg}}, chainNames, nil)
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
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames, nil)
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
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: nil}}, chainNames, nil)
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
	pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames, nil)
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
	pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: vhCfg, Route: nil}}, chainNames, nil)
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
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames, nil)
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
	pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: nil}}, chainNames, nil)
	route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
	if route != nil || vhost != nil || rc != nil {
		t.Errorf("all should be nil; got route=%v vhost=%v rc=%v", route, vhost, rc)
	}
}

func TestResolveAllTiers_RouteIdxOutOfRange(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames, nil)
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
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames, nil)
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
	pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames, nil)
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

func TestBuildPerRouteConfig_PerRouteValidator_NilSucceeds(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("ok"))}
	// nil registry → backwards-compatible (no validator consulted)
	_, err := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames, nil)
	if err != nil {
		t.Errorf("nil registry should succeed; got %v", err)
	}
}

func TestBuildPerRouteConfig_PerRouteValidator_NoValidatorRegisteredSucceeds(t *testing.T) {
	r := NewHTTPRegistry()
	r.Freeze()
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("ok"))}
	_, err := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames, r)
	if err != nil {
		t.Errorf("no validator registered should succeed; got %v", err)
	}
}

func TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnRouteTier(t *testing.T) {
	r := NewHTTPRegistry()
	sentinelErr := errors.New("validator-rejection")
	r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error {
		return sentinelErr
	})
	r.Freeze()
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("triggers-error"))}
	_, err := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames, r)
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	if !strings.Contains(err.Error(), "validator-rejection") {
		t.Errorf("error should wrap validator error; got %v", err)
	}
	if !strings.Contains(err.Error(), "routes[0]") {
		t.Errorf("error should carry route-tier location prefix; got %v", err)
	}
}

func TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnVHostTier(t *testing.T) {
	r := NewHTTPRegistry()
	r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error {
		return errors.New("validator-rejection")
	})
	r.Freeze()
	chainNames := []string{"envoy.filters.http.header_mutation"}
	vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("triggers-error"))}
	_, err := BuildPerRouteConfig(nil, []routeScope{{VHost: vhCfg, Route: nil}}, chainNames, r)
	if err == nil || !strings.Contains(err.Error(), "virtual_hosts[0]") {
		t.Errorf("expected vhost-tier location prefix; got %v", err)
	}
}

func TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnRCTier(t *testing.T) {
	r := NewHTTPRegistry()
	r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error {
		return errors.New("validator-rejection")
	})
	r.Freeze()
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("triggers-error"))}
	_, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames, r)
	if err == nil || !strings.Contains(err.Error(), "route_config") {
		t.Errorf("expected rc-tier location prefix; got %v", err)
	}
}

func TestBuildPerRouteConfig_PerRouteValidator_OnlyConsultedForRegisteredFilters(t *testing.T) {
	// Two filters in chain; only one has a validator. The non-validated one's
	// per-route configs are accepted unconditionally.
	r := NewHTTPRegistry()
	r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error {
		return errors.New("validator-rejection")
	})
	r.Freeze()
	chainNames := []string{"envoy.filters.http.cors", "envoy.filters.http.header_mutation"}
	// Cors per-route config — header_mutation NOT in any tier.
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("cors-policy"))}
	_, err := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames, r)
	if err != nil {
		t.Errorf("unrelated filter's config should not trigger validator; got %v", err)
	}
}

// --- Multi-route location-string coordinates (maintenance pass 2026-07-07) ---
//
// Scopes are one-per-route; the boot-error location strings previously reused
// the flat scope index for BOTH the virtual_hosts[...] and routes[...]
// coordinates, so route 1 of vhost 0 misreported as
// virtual_hosts[1].routes[1]. The tests below pin the corrected coordinates
// threaded through RouteScope.VHostIndex/RouteIndex.

func TestPerRoute_Build_MultiRoute_UnknownFilterName_ReportsRealCoordinates(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors"}
	okCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("ok"))}
	badCfg := map[string]*anypb.Any{"envoy.filters.http.bogus": mustAny(t, wrapperspb.String("oops"))}
	// vhost 0 carries routes 0+1; the unknown-filter map sits on route 1.
	scopes := []routeScope{
		{VHost: nil, Route: okCfg, VHostIndex: 0, RouteIndex: 0},
		{VHost: nil, Route: badCfg, VHostIndex: 0, RouteIndex: 1},
	}
	_, err := BuildPerRouteConfig(nil, scopes, chainNames, nil)
	if err == nil {
		t.Fatal("expected error on unknown filter name")
	}
	if !strings.Contains(err.Error(), "route_config.virtual_hosts[0].routes[1]") {
		t.Errorf("expected location virtual_hosts[0].routes[1]; got %q", err.Error())
	}
}

func TestPerRoute_Build_SecondVHost_UnknownFilterName_ReportsRealCoordinates(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors"}
	badVH := map[string]*anypb.Any{"envoy.filters.http.bogus": mustAny(t, wrapperspb.String("oops"))}
	// vhost 0 has 2 clean routes; vhost 1's route 0 carries the unknown-filter
	// vhost-tier map. The flat scope index of that scope is 2 — the old code
	// would have reported virtual_hosts[2].
	scopes := []routeScope{
		{VHostIndex: 0, RouteIndex: 0},
		{VHostIndex: 0, RouteIndex: 1},
		{VHost: badVH, VHostIndex: 1, RouteIndex: 0},
	}
	_, err := BuildPerRouteConfig(nil, scopes, chainNames, nil)
	if err == nil {
		t.Fatal("expected error on unknown filter name")
	}
	if !strings.Contains(err.Error(), "route_config.virtual_hosts[1]:") {
		t.Errorf("expected location virtual_hosts[1]; got %q", err.Error())
	}
}

func TestBuildPerRouteConfig_PerRouteValidator_MultiRoute_ErrorCarriesRealCoordinates(t *testing.T) {
	r := NewHTTPRegistry()
	r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error {
		if s, ok := m.(*wrapperspb.StringValue); ok && s.GetValue() == "triggers-error" {
			return errors.New("validator-rejection")
		}
		return nil
	})
	r.Freeze()
	chainNames := []string{"envoy.filters.http.header_mutation"}
	okCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("ok"))}
	badCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("triggers-error"))}
	// The rejected per-route config sits at vhost 1, route 2 (flat scope
	// index 3 — the old code would have reported virtual_hosts[3].routes[3]).
	scopes := []routeScope{
		{Route: okCfg, VHostIndex: 0, RouteIndex: 0},
		{VHostIndex: 1, RouteIndex: 0},
		{Route: okCfg, VHostIndex: 1, RouteIndex: 1},
		{Route: badCfg, VHostIndex: 1, RouteIndex: 2},
	}
	_, err := BuildPerRouteConfig(nil, scopes, chainNames, r)
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	if !strings.Contains(err.Error(), "route_config.virtual_hosts[1].routes[2]") {
		t.Errorf("expected location virtual_hosts[1].routes[2]; got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "validator-rejection") {
		t.Errorf("error should wrap validator error; got %q", err.Error())
	}
}
