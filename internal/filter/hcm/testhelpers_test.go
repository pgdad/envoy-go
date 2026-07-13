package hcm

import (
	"testing"

	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/cluster"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/http/router"
	"github.com/pgdad/envoy-go/internal/stats"
)

// testHTTPRegistry returns a freshly-allocated, frozen *filter_http.HTTPRegistry
// containing only the router terminal filter (envoy.filters.http.router →
// router.New). Used by the post-Task-14 hcm test suite as the empty-but-valid
// registry replacement for the deleted defaultRouterOnlyHTTPRegistry helper:
// every NewFilterWithCtxAndSinksAndRegistry call site that does not exercise
// the registry passes one of these.
//
// Per PLAN Task 14 Step 3 + the prompt's empty-then-frozen pattern. ADR-0072
// freeze-after-boot contract: the registry must be Frozen at construct-time
// so the parser walks a sealed map.
func testHTTPRegistry() *filter_http.HTTPRegistry {
	r := filter_http.NewHTTPRegistry()
	r.Register(router.TypeURL, router.New)
	r.Freeze()
	return r
}

// parseFilterTest is the post-Task-14 test-only wrapper around
// parseFilterWithCtx with a zero-value ListenerCtx, a fresh throwaway
// *stats.Registry, no access-log sinks, and the router-only testHTTPRegistry.
// Replaces the deleted parseFilter helper that lived in production code in
// config.go pre-Task-14; the test bodies that previously called parseFilter
// now call this helper with the same two-arg shape.
func parseFilterTest(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, ListenerCtx{}, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil)
}

// parseFilterQUIC is the QUIC-listener counterpart of parseFilterTest: it
// builds an HCM via mkHCM(modify), parses it with
// ListenerCtx{IsQUIC: true, HasTLS: true} (QUIC is mandatory-TLS per
// phase-61.1) rather than a zero-value ListenerCtx, and Fatals on error.
// Used by phase 61.2 Task 3 tests to exercise the codec_type HTTP3
// accept-on-QUIC arm.
func parseFilterQUIC(t *testing.T, modify func(*hcmv3.HttpConnectionManager)) *Filter {
	t.Helper()
	cm := mkClusterManager(t)
	f, err := parseFilterWithCtx(mkHCM(modify), cm, ListenerCtx{IsQUIC: true, HasTLS: true}, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil)
	if err != nil {
		t.Fatalf("parseFilterQUIC: unexpected error: %v", err)
	}
	return f
}
