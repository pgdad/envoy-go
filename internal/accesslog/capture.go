package accesslog

import "strings"

// CaptureHeaders builds the lowercase-keyed captured-header map for the named
// headers per AMEND-HDR-1/2/3: for each configured name, lookup returns the
// header's values and whether it is PRESENT. A present header (even with an
// empty value) becomes a map entry with the comma-joined values (no space, in
// the lookup's order — wire order). An absent header is OMITTED (the
// discriminator is presence, not value emptiness). The caller passes
// already-lowercased names (lowercased once at parse time). Returns nil when
// names is empty (the byte-stable no-capture sentinel — keeps Record maps nil).
func CaptureHeaders(names []string, lookup func(name string) ([]string, bool)) map[string]string {
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]string, len(names))
	for _, name := range names {
		if vals, ok := lookup(name); ok {
			out[name] = strings.Join(vals, ",")
		}
	}
	return out
}
