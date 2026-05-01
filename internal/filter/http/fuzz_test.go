package http

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// FuzzFilterChainParse exercises BuildPerRouteConfig against adversarial
// typed_per_filter_config maps + chain-name slices. Asserts: no panic; the
// function returns either nil or an error — no crashes — and never deadlocks.
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
			_, _ = BuildPerRouteConfig(rcCfg, []routeScope{{vhost: vh, route: rt}}, chain)
		}
	})
}
