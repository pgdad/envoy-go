package http

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

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
// binary-noise shape (high bytes / NULs in name + payloads). Post-Task-13
// this fuzzer extends to also fuzz the parseFilterWithCtx output (chain-shape
// adversarial inputs).
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
			pc, err := BuildPerRouteConfig(rcCfg, []routeScope{{vhost: vh, route: rt}}, chain)
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
