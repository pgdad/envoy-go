package tls

import "testing"

func TestMatchServerName(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		sni      string
		want     bool
	}{
		{"exact match", []string{"alpha.envoy-go.test"}, "alpha.envoy-go.test", true},
		{"exact mismatch", []string{"alpha.envoy-go.test"}, "beta.envoy-go.test", false},
		{"suffix wildcard match", []string{"*.envoy-go.test"}, "alpha.envoy-go.test", true},
		{"suffix wildcard multi-label", []string{"*.envoy-go.test"}, "a.b.envoy-go.test", true},
		{"suffix wildcard does not match bare parent", []string{"*.envoy-go.test"}, "envoy-go.test", false},
		{"universal wildcard", []string{"*"}, "anything.example", true},
		{"universal wildcard empty sni", []string{"*"}, "", true},
		{"no patterns", nil, "anything", false},
		{"empty patterns slice", []string{}, "anything", false},
		{"case insensitive sni upper", []string{"alpha.envoy-go.test"}, "ALPHA.envoy-go.test", true},
		{"case insensitive pattern upper", []string{"ALPHA.envoy-go.test"}, "alpha.envoy-go.test", true},
		{"first-match wins (exact beats wildcard)", []string{"alpha.envoy-go.test", "*.envoy-go.test"}, "alpha.envoy-go.test", true},
		{"multiple wildcards, most-specific wins", []string{"*.envoy-go.test"}, "x.envoy-go.test", true},
		{"no match across multiple patterns", []string{"a.test", "b.test"}, "c.test", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchServerName(tc.patterns, tc.sni)
			if got != tc.want {
				t.Errorf("MatchServerName(%q, %q) = %v, want %v", tc.patterns, tc.sni, got, tc.want)
			}
		})
	}
}
