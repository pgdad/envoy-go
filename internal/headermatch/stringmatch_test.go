package headermatch

import (
	"testing"

	tmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

func TestStringMatcher_Arms(t *testing.T) {
	re := func(s string) *tmatcherv3.RegexMatcher {
		return &tmatcherv3.RegexMatcher{
			EngineType: &tmatcherv3.RegexMatcher_GoogleRe2{GoogleRe2: &tmatcherv3.RegexMatcher_GoogleRE2{}},
			Regex:      s,
		}
	}
	cases := []struct {
		name  string
		sm    *tmatcherv3.StringMatcher
		value string
		want  bool
	}{
		{"exact_hit", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Exact{Exact: "yes"}}, "yes", true},
		{"exact_miss", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Exact{Exact: "yes"}}, "no", false},
		{"exact_case_sensitive", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Exact{Exact: "yes"}}, "YES", false},
		{"exact_ignore_case", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Exact{Exact: "yes"}, IgnoreCase: true}, "YES", true},
		{"prefix_hit", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Prefix{Prefix: "ab"}}, "abc", true},
		{"prefix_ignore_case", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Prefix{Prefix: "ab"}, IgnoreCase: true}, "ABC", true},
		{"suffix_hit", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Suffix{Suffix: "bc"}}, "abc", true},
		{"contains_hit", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Contains{Contains: "b"}}, "abc", true},
		{"contains_ignore_case", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Contains{Contains: "b"}, IgnoreCase: true}, "ABC", true},
		{"safe_regex_full_match", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_SafeRegex{SafeRegex: re("a.c")}}, "abc", true},
		{"safe_regex_is_anchored", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_SafeRegex{SafeRegex: re("b")}}, "abc", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := newStringMatcher(tc.sm)
			if err != nil {
				t.Fatalf("newStringMatcher: %v", err)
			}
			if got := m.match(tc.value); got != tc.want {
				t.Errorf("match(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestStringMatcher_Rejects(t *testing.T) {
	cases := []struct {
		name string
		sm   *tmatcherv3.StringMatcher
	}{
		{"nil", nil},
		{"no_arm", &tmatcherv3.StringMatcher{}},
		{"custom_arm", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Custom{}}},
		{"bad_regex", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_SafeRegex{
			SafeRegex: &tmatcherv3.RegexMatcher{Regex: "a(("},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newStringMatcher(tc.sm); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
