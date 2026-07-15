package tracing

import (
	"strings"
	"testing"
)

// TestBuildHTTPURL exercises the max_path_tag_length byte-truncation helper (SPEC-64
// §11): the :path (path+query) is byte-truncated to maxPathTagLen FIRST, then
// scheme://host is prepended (NEVER truncated). Cases: under-cap (unchanged),
// over-cap (truncated to N bytes, D-MPTL-TARGET), explicit-0 (empty path →
// scheme://host only, D-MPTL-ZERO), query-cut (a cut inside the query, D-MPTL-QUERY),
// and the exact byte boundary (== cap ⇒ unchanged, proving strict `>`; ASCII so
// byte==rune, D-MPTL-TRUNCUNIT). Errorf per row.
func TestBuildHTTPURL(t *testing.T) {
	const scheme, host = "http", "h.io"
	tests := []struct {
		name          string
		pathAndQuery  string
		maxPathTagLen uint32
		want          string
	}{
		{"under-cap-unchanged", "/short", 16, "http://h.io/short"},
		{"over-cap-truncated", "/abcdefghijKLMNOPqrstuvwxyz", 16, "http://h.io/abcdefghijKLMNO"},
		{"explicit-zero-empty-path", "/somepath?x=1", 0, "http://h.io"},
		{"query-cut-inside-query", "/p?query=abcdefghijklmnop", 16, "http://h.io/p?query=abcdefg"},
		{"exact-boundary-unchanged", "/exactly16bytes!", 16, "http://h.io/exactly16bytes!"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildHTTPURL(scheme, host, tc.pathAndQuery, tc.maxPathTagLen); got != tc.want {
				t.Errorf("BuildHTTPURL(%q, cap %d) = %q, want %q", tc.pathAndQuery, tc.maxPathTagLen, got, tc.want)
			}
		})
	}

	// D-MPTL-DEFAULT-PROOF (SPEC §8): a > 256-byte path under the default 256 cap
	// truncates to exactly 256 :path bytes — proving the reference default is honored
	// WITHOUT a second differential fixture. (The 307-byte path mirrors §11 arm 1.)
	t.Run("default-256-truncation", func(t *testing.T) {
		longPath := "/probe/" + strings.Repeat("a", 300) // 7 + 300 = 307 bytes
		got := BuildHTTPURL(scheme, host, longPath, 256)
		want := "http://h.io" + longPath[:256]
		if got != want {
			t.Errorf("default-256: got %q (len %d), want the 256-byte-:path form (len %d)", got, len(got), len(want))
		}
		if wantLen := len("http://h.io") + 256; len(got) != wantLen {
			t.Errorf("default-256: len(http.url) = %d, want %d (11-byte prefix + 256 :path)", len(got), wantLen)
		}
	})
}
