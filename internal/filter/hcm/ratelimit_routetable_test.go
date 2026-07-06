package hcm

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"

	"github.com/pgdad/envoy-go/internal/accesslog"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

// Phase 24.1 Task 5 (DELTA-2 / ADR-0198) — HCM route-table rate_limits
// exposure + RouteRateLimits()/VirtualHostRateLimits() accessor pair.
//
// These tests cover the three Step-2 acceptance shapes from the IMPL plan:
//
//  1. TestRouteTableRateLimits_ParseRetainSeed — config carrying vhost +
//     route rate_limits[] parses without error; routeTable + routeEntry
//     retain the raw policies; the chain accessor pair returns the seeded
//     policies after dispatch.
//  2. TestRouteTableRateLimits_StrictReject — the §5.2 PARSE-REJECT roster
//     fires at HCM-parse time (disable_key, extension, dynamic_metadata)
//     for both route-level + vhost-level rate_limits[]; byte-stable wording
//     consts are reused from the ratelimit package via ValidateRouteRateLimits.
//  3. TestRouteTableRateLimits_ZeroRegression — a config with NO rate_limits[]
//     parses unchanged; the chain accessors return nil; the routeEntry +
//     routeTable fields are nil.

// mkHCMWithRateLimits returns an HCM Any with the supplied vhost-level +
// route-level RateLimit slices grafted onto the minimal-direct-response
// fixture from mkHCM. routeRLs is applied to the FIRST route's RouteAction
// (which requires the route to use Route_Route, not Route_DirectResponse,
// since the rate_limits field lives on RouteAction; we add a SECOND route
// for that purpose when needed by mutating the action).
func mkHCMWithRateLimits(vhostRLs []*routev3.RateLimit, routeRLs []*routev3.RateLimit) []byte {
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		rc := h.GetRouteSpecifier().(*hcmv3.HttpConnectionManager_RouteConfig).RouteConfig
		vh := rc.VirtualHosts[0]
		vh.RateLimits = vhostRLs
		if len(routeRLs) > 0 {
			// Replace the direct_response with a Route_Route that carries
			// rate_limits[]. The cluster reference is not strictly required for
			// parse-time validation here (buildRouterAction will still validate
			// the cluster name). Use the c_test cluster registered by
			// mkClusterManager.
			vh.Routes[0].Action = &routev3.Route_Route{Route: &routev3.RouteAction{
				ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_test"},
				RateLimits:       routeRLs,
			}}
		}
	})
	return any.GetValue()
}

func reqWithPath2(p string) *http.Request {
	return &http.Request{URL: &url.URL{Path: p}}
}

// rlDescriptors returns a benign RateLimit with a single 'generic_key'
// descriptor action — the simplest §5.1 CORE-action arm that ValidateRouteRateLimits
// does NOT reject. Stage val is the descriptor_value the engine would emit.
func rlBenign(val string) *routev3.RateLimit {
	return &routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
				GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: val},
			},
		}},
	}
}

// rlDisableKey returns a RateLimit with a non-empty disable_key — the §5.2
// Arm 1 reject case.
func rlDisableKey(key string) *routev3.RateLimit {
	return &routev3.RateLimit{
		DisableKey: key,
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
				GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: "x"},
			},
		}},
	}
}

// rlExtension returns a RateLimit with an extension action — the §5.2 Arm 2
// reject case.
func rlExtension() *routev3.RateLimit {
	return &routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_Extension{
				Extension: &corev3.TypedExtensionConfig{Name: "x"},
			},
		}},
	}
}

// rlDynamicMetadata returns a RateLimit with a deprecated dynamic_metadata
// action — the §5.2 Arm 3 reject case.
func rlDynamicMetadata() *routev3.RateLimit {
	return &routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_DynamicMetadata{
				DynamicMetadata: &routev3.RateLimit_Action_DynamicMetaData{DescriptorKey: "k"},
			},
		}},
	}
}

func parseHCMTestBytes(t *testing.T, hcmBytes []byte) (*Filter, error) {
	t.Helper()
	cm := mkClusterManager(t)
	// Re-wrap as Any (mkHCMWithRateLimits returns the inner bytes).
	any := mkHCM(nil)
	any.Value = hcmBytes
	return parseFilterWithCtx(any, cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil)
}

func TestRouteTableRateLimits_ParseRetainSeed(t *testing.T) {
	vhRL := rlBenign("vh-A")
	rRL := rlBenign("route-A")
	bytes := mkHCMWithRateLimits([]*routev3.RateLimit{vhRL}, []*routev3.RateLimit{rRL})

	f, err := parseHCMTestBytes(t, bytes)
	if err != nil {
		t.Fatalf("parse: unexpected error: %v", err)
	}

	// Retained on routeTable + routeEntry: the framework holds the raw protos.
	if got := len(f.table.vhostRateLimits); got != 1 {
		t.Fatalf("vhostRateLimits: got %d, want 1", got)
	}
	if got := f.table.vhostRateLimits[0]; got != vhRL {
		// Pointer equality: the framework retains the exact slice element
		// supplied by the parsed proto — no copy, no clone (raw seed per
		// ADR-0198 §Decision).
		//
		// We compare via DescriptorValue since proto unmarshal allocates a
		// fresh tree; the pointer-equality check would only hold if we
		// hand-built the proto and re-passed it through parse. The
		// unmarshal-path here produces a fresh tree, so verify by content.
		gk := got.GetActions()[0].GetGenericKey()
		if gk == nil || gk.DescriptorValue != "vh-A" {
			t.Errorf("vhostRateLimits[0]: generic_key=%q, want %q", gk.GetDescriptorValue(), "vh-A")
		}
	}
	if got := len(f.table.routes); got != 1 {
		t.Fatalf("routes: got %d, want 1", got)
	}
	rl := f.table.routes[0].rateLimits
	if got := len(rl); got != 1 {
		t.Fatalf("routes[0].rateLimits: got %d, want 1", got)
	}
	if gk := rl[0].GetActions()[0].GetGenericKey(); gk == nil || gk.DescriptorValue != "route-A" {
		t.Errorf("routes[0].rateLimits[0]: generic_key=%q, want %q", gk.GetDescriptorValue(), "route-A")
	}

	// Dispatch seed: build a per-stream chain like HCM dispatch does +
	// seed the matched route's + vhost's raw rate_limits. Then read back
	// through the accessor pair.
	entry, _, ok := f.table.match(reqWithPath2("/health"))
	if !ok {
		t.Fatal("table.match(/health): no match")
	}
	// Use empty chain for the seed test — we only need the chain field +
	// setters + accessors to round-trip.
	chain := filter_http.NewFilterChain(nil, nil)
	chain.SetRouteRateLimits(entry.rateLimits)
	chain.SetVirtualHostRateLimits(f.table.vhostRateLimits)

	// Construct a decoderCB via the chain's test-internal helper — we can't
	// directly construct decoderCB outside the http package, but we can
	// assert chain-level accessors. The decoderCB readers simply forward
	// chain.RouteRateLimits() / chain.VirtualHostRateLimits().
	got := chain.RouteRateLimits()
	if len(got) != 1 || got[0].GetActions()[0].GetGenericKey().DescriptorValue != "route-A" {
		t.Errorf("chain.RouteRateLimits(): got len=%d, content mismatch", len(got))
	}
	got = chain.VirtualHostRateLimits()
	if len(got) != 1 || got[0].GetActions()[0].GetGenericKey().DescriptorValue != "vh-A" {
		t.Errorf("chain.VirtualHostRateLimits(): got len=%d, content mismatch", len(got))
	}
}

func TestRouteTableRateLimits_StrictReject(t *testing.T) {
	// Wording consts (byte-stable per ADR-0080 / Task 3 ValidateRouteRateLimits).
	const (
		wantDisableKey = "ratelimit: rate_limits[].disable_key is not yet supported (runtime keying deferred)"
		wantExtension  = "ratelimit: the 'extension' descriptor action is not yet supported"
		wantDynMeta    = "ratelimit: the deprecated 'dynamic_metadata' descriptor action is not supported; use 'metadata'"
	)
	cases := []struct {
		name  string
		vhRLs []*routev3.RateLimit
		rRLs  []*routev3.RateLimit
		want  string
	}{
		{name: "vhost_disable_key", vhRLs: []*routev3.RateLimit{rlDisableKey("k")}, want: wantDisableKey},
		{name: "vhost_extension", vhRLs: []*routev3.RateLimit{rlExtension()}, want: wantExtension},
		{name: "vhost_dynamic_metadata", vhRLs: []*routev3.RateLimit{rlDynamicMetadata()}, want: wantDynMeta},
		{name: "route_disable_key", rRLs: []*routev3.RateLimit{rlDisableKey("k")}, want: wantDisableKey},
		{name: "route_extension", rRLs: []*routev3.RateLimit{rlExtension()}, want: wantExtension},
		{name: "route_dynamic_metadata", rRLs: []*routev3.RateLimit{rlDynamicMetadata()}, want: wantDynMeta},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bytes := mkHCMWithRateLimits(tc.vhRLs, tc.rRLs)
			_, err := parseHCMTestBytes(t, bytes)
			if err == nil {
				t.Fatalf("parse: want error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("parse: got error %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRouteTableRateLimits_ZeroRegression(t *testing.T) {
	// A config with NO rate_limits[] anywhere. mkHCM's default route is a
	// direct_response with no rate_limits and the vhost has no RateLimits.
	any := mkHCM(nil)
	cm := mkClusterManager(t)
	f, err := parseFilterWithCtx(any, cm, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.table.vhostRateLimits != nil {
		t.Errorf("vhostRateLimits: got %v, want nil", f.table.vhostRateLimits)
	}
	if got := len(f.table.routes); got != 1 {
		t.Fatalf("routes: got %d, want 1", got)
	}
	if f.table.routes[0].rateLimits != nil {
		t.Errorf("routes[0].rateLimits: got %v, want nil", f.table.routes[0].rateLimits)
	}
	// Chain accessors return nil when not seeded — the documented zero-value
	// semantics for the set-once-by-dispatch chain fields (mirrors ADR-0165).
	chain := filter_http.NewFilterChain(nil, nil)
	if got := chain.RouteRateLimits(); got != nil {
		t.Errorf("chain.RouteRateLimits(): got %v, want nil", got)
	}
	if got := chain.VirtualHostRateLimits(); got != nil {
		t.Errorf("chain.VirtualHostRateLimits(): got %v, want nil", got)
	}
}

// Silence unused-import for accesslog when this test file is the only consumer
// in the package (defensive — accesslog is imported above for parity with
// config_test.go's set; the import is preserved for future test extensions).
var _ = accesslog.Sink(nil)
