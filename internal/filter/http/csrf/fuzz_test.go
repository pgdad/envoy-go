package csrf

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	csrfv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// FuzzCsrfPolicyConfigParse fuzzes arbitrary byte sequences as the tc
// *anypb.Any parameter to New. Asserts: never panic; never return (nil, nil);
// on success the factory invokes without panic and produces a non-nil Decoder.
//
// Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the csrf
// filter's New factory is a parser. 30s budget per ADR-0018 short-mode CI
// policy. Sixteenth fuzzer overall (post phase-11's fifteenth
// FuzzLocalRateLimitConfigParse).
func FuzzCsrfPolicyConfigParse(f *testing.F) {
	// Seed corpus: well-formed minimal policy (filter_enabled @ 100/HUNDRED).
	// Seed corpus: well-formed policy with mixed StringMatcher variants —
	// exercises the parse-time-drop path for non-Exact + empty-Exact.
	// Seed corpus: empty CsrfPolicy (missing filter_enabled — must reject
	// cleanly with err != nil, never panic, never (nil, nil)).
	seeds := []*csrfv3.CsrfPolicy{
		{
			FilterEnabled: &corev3.RuntimeFractionalPercent{
				DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			},
		},
		{
			FilterEnabled: &corev3.RuntimeFractionalPercent{
				DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			},
			AdditionalOrigins: []*matcherv3.StringMatcher{
				{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "app.example.test"}},
				{MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "pfx-"}}, // dropped at parse
				{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: ""}},       // dropped (empty)
			},
		},
		{}, // missing filter_enabled — should reject cleanly
	}
	for _, s := range seeds {
		raw, err := proto.Marshal(s)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		tc := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		factory, err := New(tc, newTestFactoryCtx())
		// Contract: never (nil, nil); never panic.
		if factory == nil && err == nil {
			t.Fatalf("New returned (nil, nil) for input len=%d", len(raw))
		}
		if factory != nil {
			// Smoke: factory must not panic when invoked.
			hf := factory()
			if hf.Decoder == nil {
				t.Fatal("Decoder must be non-nil")
			}
		}
	})
}
