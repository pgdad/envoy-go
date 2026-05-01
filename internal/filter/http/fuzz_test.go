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

// FuzzFilterChainParse exercises BuildPerRouteConfig + Resolve against
// adversarial typed_per_filter_config maps + chain-name slices. Asserts:
// no panic; errors carry the canonical `hcm:` prefix (matches the prior
// fuzzer discipline in internal/filter/hcm/fuzz_test.go and
// internal/filter/tcpproxy/fuzz_test.go); on success, Resolve runs at
// boundary routeIdx values without crashing or deadlocking.
//
// Per ADR-0018: short-budget (30s in CI; arbitrary local time). Seed corpus
// gives the fuzzer three starting points: one well-formed filter name + four
// payload slices, one all-empty (zero-length non-nil) shape, and one
// binary-noise shape (high bytes / NULs in name + payloads). Task 13 adds
// FuzzFilterChainParse_ChainShape (sibling fuzzer below) which extends the
// 30s budget per ADR-0018 to the chain-shape parser (ValidateChainShape).
func FuzzFilterChainParse(f *testing.F) {
	f.Add([]byte("envoy.filters.http.cors"), []byte("rc-payload"), []byte("vh-payload"), []byte("rt-payload"))
	f.Add([]byte{}, []byte{}, []byte{}, []byte{})
	f.Add([]byte("\x00\x01\x02"), []byte("\xff\xfe"), []byte{}, []byte{})
	f.Fuzz(func(t *testing.T, filterName, rcVal, vhVal, rtVal []byte) {
		// Build adversarial typed_per_filter_config maps. mk returns nil when
		// the StringValue Any cannot be marshalled (defensive — wrapperspb
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
			pc, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vh, Route: rt}}, chain)
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
	})
}

// FuzzFilterChainParse_ChainShape exercises ValidateChainShape (the four-rule
// chain-shape validator that backs hcm.parseFilterWithCtx's http_filters[]
// walk) against adversarial entry slices. Asserts: no panic; every error
// carries the canonical `hcm:` prefix; the four-rule validator produces a
// structured error rather than a crash for any (filterName1, typeURL1,
// filterName2, typeURL2, count) tuple.
//
// Per ADR-0018: short-budget (30s in CI; arbitrary local time). The two
// FuzzFilterChainParse* targets in this file run under one combined 30s
// budget per the PLAN's "logically a single FuzzFilterChainParse target with
// two seed corpora" framing. Seed corpus gives three starting points:
// well-formed (router-terminal valid chain), all-empty (triggers rule #1),
// and binary-noise (high-byte filter names + arbitrary type_urls).
//
// The fuzzer constructs a fresh empty *HTTPRegistry per iteration so unknown-
// type_url is the default failure mode for any fuzzer-supplied TypeURL; the
// well-formed seed registers the router type_url to exercise the success path.
func FuzzFilterChainParse_ChainShape(f *testing.F) {
	// Well-formed chain (1 entry: router) — should validate clean.
	f.Add([]byte(fuzzRouterName), []byte(fuzzRouterTypeURL), []byte("filter-a"), []byte("type.example/A"), uint8(1), true)
	// All-empty (count=0) — triggers rule #1 (empty chain).
	f.Add([]byte{}, []byte{}, []byte{}, []byte{}, uint8(0), false)
	// Binary-noise filter names + arbitrary type_urls.
	f.Add([]byte("\x00\x01\x02"), []byte("type.example/B"), []byte("\xff\xfe"), []byte("type.example/C"), uint8(3), false)

	f.Fuzz(func(t *testing.T, name1, typeURL1, name2, typeURL2 []byte, count uint8, registerRouter bool) {
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
	})
}
