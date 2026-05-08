package csrf

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	csrfv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// helper: wrap a *CsrfPolicy in *anypb.Any with the canonical type URL.
func mustAnyFrom(t *testing.T, c *csrfv3.CsrfPolicy) *anypb.Any {
	t.Helper()
	a, err := anypb.New(c)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func newTestFactoryCtx() envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{
		Stats:      stats.NewRegistry(),
		StatPrefix: "ingress_csrf",
	}
}

// Group 1 — New factory PGV-mirror validation.

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error on nil tc")
	}
}

func TestNew_MalformedTC(t *testing.T) {
	a := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	_, err := New(a, newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error on malformed Any")
	}
}

func TestNew_FilterEnabledNil_RejectAtParseTime(t *testing.T) {
	c := &csrfv3.CsrfPolicy{}
	_, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error: filter_enabled is required (per §11.11)")
	}
}

func TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{},
	}
	_, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error: filter_enabled.default_value is required (per §11.11)")
	}
}

func TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   0,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("expected accept (silent-ignore percentage); got %v", err)
	}
}

func TestNew_FilterEnabledHundredPercent_Accepted(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("happy-path New: %v", err)
	}
}

func TestNew_ShadowEnabledAbsent_Accepted(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("shadow_enabled absent should be OK: %v", err)
	}
}

func TestNew_ShadowEnabledPresent_SilentIgnored(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
		ShadowEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 50, Denominator: typev3.FractionalPercent_HUNDRED},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("shadow_enabled present should be OK (silent-ignored): %v", err)
	}
}

// Group 2 — additional_origins parse-time discipline.

func TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse(t *testing.T) {
	tests := []struct {
		name    string
		matcher *matcherv3.StringMatcher
	}{
		{"prefix", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "pfx-"}}},
		{"suffix", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Suffix{Suffix: "-sfx"}}},
		{"contains", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Contains{Contains: "mid"}}},
		{"safe_regex", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_SafeRegex{SafeRegex: nil}}},
		{"ignore_case_with_exact", &matcherv3.StringMatcher{IgnoreCase: true, MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "case-folded.test"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &csrfv3.CsrfPolicy{
				FilterEnabled:     &corev3.RuntimeFractionalPercent{DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED}},
				AdditionalOrigins: []*matcherv3.StringMatcher{tt.matcher},
			}
			factory, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			rc := mustGetRuntimeConfig(t, factory)
			if len(rc.additionalOrigins) != 0 {
				t.Errorf("non-exact StringMatcher %q must be dropped at parse; got %v",
					tt.name, rc.additionalOrigins)
			}
		})
	}
}

func TestNew_AdditionalOrigins_EmptyExactValue_Dropped(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED}},
		AdditionalOrigins: []*matcherv3.StringMatcher{
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: ""}},
		},
	}
	factory, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rc := mustGetRuntimeConfig(t, factory)
	if len(rc.additionalOrigins) != 0 {
		t.Errorf("empty-value exact entry must be dropped; got %v", rc.additionalOrigins)
	}
}

func TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED}},
		AdditionalOrigins: []*matcherv3.StringMatcher{
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "app.example.test"}},
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "Other.Test:8443"}}, // case + port preserved
		},
	}
	factory, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rc := mustGetRuntimeConfig(t, factory)
	want := []string{"app.example.test", "Other.Test:8443"}
	if len(rc.additionalOrigins) != len(want) {
		t.Fatalf("len mismatch; got %v want %v", rc.additionalOrigins, want)
	}
	for i, w := range want {
		if rc.additionalOrigins[i] != w {
			t.Errorf("entry %d: got %q want %q (verbatim preservation)", i, rc.additionalOrigins[i], w)
		}
	}
}

// mustGetRuntimeConfig is a test-only helper that invokes the FilterInstanceFactory
// once and inspects the captured *runtimeConfig via the per-instance filter struct's
// rc field.
func mustGetRuntimeConfig(t *testing.T, factory envoyhttp.FilterInstanceFactory) *runtimeConfig {
	t.Helper()
	hf := factory()
	d, ok := hf.Decoder.(*filter)
	if !ok {
		t.Fatalf("Decoder is not *filter; got %T", hf.Decoder)
	}
	return d.rc
}

// Compile-time check: SPEC's documented public surface.
var (
	_ proto.Message               = (*csrfv3.CsrfPolicy)(nil)
	_ envoyhttp.HTTPFilterFactory = New
)
