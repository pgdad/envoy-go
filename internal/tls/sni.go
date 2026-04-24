package tls

import "strings"

// MatchServerName reports whether sni matches any of the given patterns under
// Envoy's server_names[] semantics. Patterns and sni are compared
// case-insensitively. Three pattern shapes are recognized:
//
//   - Exact pattern (e.g. "alpha.envoy-go.test") matches sni iff equal.
//   - Suffix wildcard (e.g. "*.envoy-go.test") matches sni iff sni's label
//     count strictly exceeds the pattern's label count and sni ends with the
//     pattern's non-wildcard suffix at a label boundary. "*.envoy-go.test"
//     thus matches "alpha.envoy-go.test" and "a.b.envoy-go.test" but not
//     "envoy-go.test" itself.
//   - Universal wildcard "*" matches any sni, including the empty string.
//
// The function is pure and order-insensitive: if any pattern matches, it
// returns true. Callers that need most-specific-first dispatch perform the
// ordering themselves (see internal/listener GetConfigForClient).
func MatchServerName(patterns []string, sni string) bool {
	sniLower := strings.ToLower(sni)
	for _, p := range patterns {
		pLower := strings.ToLower(p)
		switch {
		case pLower == "*":
			return true
		case strings.HasPrefix(pLower, "*."):
			suffix := pLower[1:] // keeps the leading '.'
			if len(sniLower) > len(suffix) && strings.HasSuffix(sniLower, suffix) {
				return true
			}
		default:
			if pLower == sniLower {
				return true
			}
		}
	}
	return false
}
