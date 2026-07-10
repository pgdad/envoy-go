package headermatch

import (
	"net/http"
	"testing"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	tmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

func hdr(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h[kv[i]] = append(h[kv[i]], kv[i+1]) // raw: keys are already lowercase
	}
	return h
}

func TestLowercase_CopiesAndLowers(t *testing.T) {
	src := http.Header{"X-Tap": {"yes"}, ":method": {"GET"}}
	got := Lowercase(src)
	if _, ok := got["x-tap"]; !ok { //nolint:staticcheck
		t.Errorf("want lowercased key x-tap; got %v", got)
	}
	if _, ok := got[":method"]; !ok {
		t.Errorf("want pseudo-header :method preserved; got %v", got)
	}
	got["x-new"] = []string{"1"}           //nolint:staticcheck
	if _, leaked := src["x-new"]; leaked { //nolint:staticcheck
		t.Errorf("Lowercase must return a COPY; source was mutated")
	}
}

func TestMatcher_Arms(t *testing.T) {
	cases := []struct {
		name string
		hm   *routev3.HeaderMatcher
		h    http.Header
		want bool
	}{
		{"exact_hit", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "yes"}}, hdr("x-tap", "yes"), true},
		{"exact_miss", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "yes"}}, hdr("x-tap", "no"), false},
		{"pseudo_status_exact", &routev3.HeaderMatcher{Name: ":status",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "204"}}, hdr(":status", "204"), true},
		{"present_true_hit", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{PresentMatch: true}}, hdr("x-tap", "no"), true},
		{"present_true_absent", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{PresentMatch: true}}, hdr(), false},
		{"present_false_absent", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{PresentMatch: false}}, hdr(), true},
		{"prefix", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_PrefixMatch{PrefixMatch: "ye"}}, hdr("x-tap", "yes"), true},
		{"suffix", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_SuffixMatch{SuffixMatch: "es"}}, hdr("x-tap", "yes"), true},
		{"contains", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ContainsMatch{ContainsMatch: "e"}}, hdr("x-tap", "yes"), true},
		{"safe_regex", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_SafeRegexMatch{SafeRegexMatch: &tmatcherv3.RegexMatcher{Regex: "y.s"}}}, hdr("x-tap", "yes"), true},
		{"string_match", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{StringMatch: &tmatcherv3.StringMatcher{
				MatchPattern: &tmatcherv3.StringMatcher_Exact{Exact: "yes"}}}}, hdr("x-tap", "yes"), true},
		{"range_in", &routev3.HeaderMatcher{Name: "x-n",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_RangeMatch{RangeMatch: &typev3.Int64Range{Start: 1, End: 10}}}, hdr("x-n", "5"), true},
		{"range_end_exclusive", &routev3.HeaderMatcher{Name: "x-n",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_RangeMatch{RangeMatch: &typev3.Int64Range{Start: 1, End: 10}}}, hdr("x-n", "10"), false},
		{"range_unparsable", &routev3.HeaderMatcher{Name: "x-n",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_RangeMatch{RangeMatch: &typev3.Int64Range{Start: 1, End: 10}}}, hdr("x-n", "abc"), false},
		{"multi_value_joined", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "a,b"}},
			http.Header{"x-tap": {"a", "b"}}, true},
		{"invert", &routev3.HeaderMatcher{Name: "x-tap", InvertMatch: true,
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "yes"}}, hdr("x-tap", "no"), true},
		{"missing_no_treat", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: ""}}, hdr(), false},
		{"missing_treat_as_empty", &routev3.HeaderMatcher{Name: "x-tap", TreatMissingHeaderAsEmpty: true,
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: ""}}, hdr(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := New(tc.hm)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if got := m.Match(tc.h); got != tc.want {
				t.Errorf("Match() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatcher_Rejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		hm   *routev3.HeaderMatcher
	}{
		{"nil", nil},
		{"empty_name", &routev3.HeaderMatcher{HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{PresentMatch: true}}},
		{"no_arm", &routev3.HeaderMatcher{Name: "x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.hm); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
