package http

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// fuzzRouterTypeURL is the canonical envoy.filters.http.router type_url —
// duplicated here from internal/filter/http/router/router.go's TypeURL
// constant to avoid a fuzz-test → router import (the http package must stay
// dep-free of its sub-packages for the framework's build hygiene).
const fuzzRouterTypeURL = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
const fuzzRouterName = "envoy.filters.http.router"

// FuzzFilterChainParse is the single, consolidated 07.1-framework fuzzer per
// PLAN.md:2179 ("logically a single FuzzFilterChainParse target with two seed
// corpora") + PLAN.md:2917,2925 + Task 23 close-out gate (9 fuzzers post-07.1).
// A leading discriminator byte `mode` selects between two parser surfaces that
// share the package's adversarial input shape:
//
//   - mode == 0 → BuildPerRouteConfig + Resolve path (Task 10 surface). The
//     remaining parameters are repurposed as (filterName, rcVal, vhVal, rtVal)
//     — adversarial typed_per_filter_config payloads.
//   - mode != 0 → ValidateChainShape path (Task 13 surface). The remaining
//     parameters are repurposed as (name1, typeURL1, name2, typeURL2, count,
//     registerRouter) — adversarial chain entries fed to the four-rule
//     chain-shape validator.
//
// Pattern A consolidation (discriminator + two seed corpora, one assertion
// shape per branch) was chosen over Pattern B (run both branches per input)
// because the two branches have divergent input-arity needs — chain-shape
// wants a count + registry-toggle flag that the per-route path does not — and
// Pattern A keeps each branch's seed corpus minimal + each branch's assertion
// surface narrow (no cross-branch noise).
//
// Asserts (both branches): no panic; every error carries the canonical `hcm:`
// prefix (matches the prior fuzzer discipline in internal/filter/hcm/fuzz_test.go
// + internal/filter/tcpproxy/fuzz_test.go + internal/tls/fuzz_test.go).
//
// Per ADR-0018: short-budget (30s in CI; arbitrary local time). Six seed
// entries — three per branch — give the fuzzer well-formed, all-empty, and
// binary-noise starting points on each branch.
func FuzzFilterChainParse(f *testing.F) {
	// Branch-0 seeds — BuildPerRouteConfig + Resolve path.
	// (mode, filterName, rcVal, vhVal, rtVal, count, registerRouter)
	// count + registerRouter are unused on branch 0 — pinned to zero values.
	f.Add(byte(0), []byte("envoy.filters.http.cors"), []byte("rc-payload"), []byte("vh-payload"), []byte("rt-payload"), uint8(0), false)
	f.Add(byte(0), []byte{}, []byte{}, []byte{}, []byte{}, uint8(0), false)
	f.Add(byte(0), []byte("\x00\x01\x02"), []byte("\xff\xfe"), []byte{}, []byte{}, uint8(0), false)
	// Branch-1 seeds — ValidateChainShape path.
	// (mode, name1, typeURL1, name2, typeURL2, count, registerRouter)
	// Well-formed chain (1 entry: router) — should validate clean.
	f.Add(byte(1), []byte(fuzzRouterName), []byte(fuzzRouterTypeURL), []byte("filter-a"), []byte("type.example/A"), uint8(1), true)
	// All-empty (count=0) — triggers rule #1 (empty chain).
	f.Add(byte(1), []byte{}, []byte{}, []byte{}, []byte{}, uint8(0), false)
	// Binary-noise filter names + arbitrary type_urls.
	f.Add(byte(1), []byte("\x00\x01\x02"), []byte("type.example/B"), []byte("\xff\xfe"), []byte("type.example/C"), uint8(3), false)

	f.Fuzz(func(t *testing.T, mode byte, a, b, c, d []byte, count uint8, registerRouter bool) {
		switch mode {
		case 0:
			fuzzBuildPerRouteAndResolve(t, a, b, c, d)
		default:
			fuzzValidateChainShape(t, a, b, c, d, count, registerRouter)
		}
	})
}

// fuzzBuildPerRouteAndResolve exercises BuildPerRouteConfig + Resolve against
// adversarial typed_per_filter_config maps + chain-name slices. On error,
// asserts the canonical `hcm:` prefix; on success, calls Resolve at boundary
// routeIdx values (0, -1, 999) to exercise the lazy-cache prime, negative-
// bounds, and out-of-range bounds-check paths without crashing or deadlocking.
func fuzzBuildPerRouteAndResolve(t *testing.T, filterName, rcVal, vhVal, rtVal []byte) {
	// Build adversarial typed_per_filter_config maps. mk returns nil when
	// the StringValue Any cannot be marshaled (defensive — wrapperspb
	// accepts any string, but anypb.New can in principle fail).
	mk := func(b []byte) map[string]*anypb.Any {
		a, err := anypb.New(wrapperspb.String(string(b)))
		if err != nil {
			return nil
		}
		return map[string]*anypb.Any{string(filterName): a}
	}
	rcCfg := mk(rcVal)
	vh := mk(vhVal)
	rt := mk(rtVal)
	// Validate either against an empty chain or a chain that matches.
	// The chain-name comparisons in BuildPerRouteConfig are exact-string
	// equality checks against the typed_per_filter_config keys; arbitrary
	// fuzzer-supplied bytes can collide here, which is the intended
	// adversarial surface.
	chains := [][]string{{}, {string(filterName)}, {string(filterName), "envoy.filters.http.router"}}
	for _, chain := range chains {
		pc, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vh, Route: rt}}, chain, nil)
		if err != nil {
			if !strings.HasPrefix(err.Error(), "hcm:") {
				t.Errorf("error not hcm:-prefixed: %v", err)
			}
			continue
		}
		// Exercise Resolve at valid + boundary + invalid routeIdx values.
		// Distinct routeIdx values exercise the cache-miss + bounds-check
		// paths; the lookup at routeIdx=0 also primes the lazy cache.
		_ = pc.Resolve(string(filterName), 0)
		_ = pc.Resolve(string(filterName), -1)
		_ = pc.Resolve(string(filterName), 999)
	}
}

// fuzzValidateChainShape exercises ValidateChainShape (the four-rule chain-
// shape validator that backs hcm.parseFilterWithCtx's http_filters[] walk)
// against adversarial entry slices. Asserts: no panic; every error carries
// the canonical `hcm:` prefix; the four-rule validator produces a structured
// error rather than a crash for any (filterName1, typeURL1, filterName2,
// typeURL2, count, registerRouter) tuple. Constructs a fresh empty
// *HTTPRegistry per iteration so unknown-type_url is the default failure
// mode for any fuzzer-supplied TypeURL; the well-formed seed registers the
// router type_url to exercise the success path.
func fuzzValidateChainShape(t *testing.T, name1, typeURL1, name2, typeURL2 []byte, count uint8, registerRouter bool) {
	// Cap count to a sane upper bound to avoid OOM from a maliciously
	// large entries slice. The chain-shape validator is O(N) in entries,
	// but we don't need a >256-entry slice to exercise the four rules.
	if count > 16 {
		count = 16
	}
	entries := make([]ChainShapeEntry, 0, int(count))
	for i := uint8(0); i < count; i++ {
		// Alternate between (name1, typeURL1) and (name2, typeURL2) so
		// the fuzzer can exercise duplicate-name (when count>=2 and the
		// two name byte slices are equal) plus mixed-name (when they
		// differ) shapes.
		if i%2 == 0 {
			entries = append(entries, ChainShapeEntry{Name: string(name1), TypeURL: string(typeURL1)})
		} else {
			entries = append(entries, ChainShapeEntry{Name: string(name2), TypeURL: string(typeURL2)})
		}
	}

	registry := NewHTTPRegistry()
	if registerRouter {
		// Register a no-op factory under the router type_url so the
		// success-path (last-entry-is-router-and-known-type_url) is
		// reachable.
		registry.Register(fuzzRouterTypeURL, func(_ *anypb.Any, _ FactoryCtx) (FilterInstanceFactory, error) {
			return func() HTTPFilter { return HTTPFilter{Name: fuzzRouterName} }, nil
		})
	}
	registry.Freeze()

	_, err := ValidateChainShape(entries, registry, fuzzRouterName, fuzzRouterTypeURL)
	if err != nil && !strings.HasPrefix(err.Error(), "hcm:") {
		t.Errorf("error not hcm:-prefixed: %v", err)
	}
}
