package tracing

import "testing"

// lookupFunc builds a header lookup from a name→values map (case-sensitive on the
// exact configured name; the production lookups are case-insensitive but the
// resolver is agnostic to that — it just calls the func it is handed).
func lookupFunc(m map[string][]string) func(string) ([]string, bool) {
	return func(name string) ([]string, bool) {
		v, ok := m[name]
		return v, ok
	}
}

// TestResolveCustomTagsMatrix drives every source/missing/multi/nil-lookup arm.
// Errorf per row so one failing case does not mask the rest
// (reference_fatalf_makes_assertions_unreachable).
func TestResolveCustomTagsMatrix(t *testing.T) {
	specs := []CustomTagSpec{
		{Key: "lit", Kind: kindLiteral, LiteralValue: "LIT-VAL"},
		{Key: "present", Kind: kindRequestHeader, HeaderName: "x-present", DefaultValue: "def-p", HasDefault: true},
		{Key: "missdef", Kind: kindRequestHeader, HeaderName: "x-missing", DefaultValue: "def-m", HasDefault: true},
		{Key: "missnodef", Kind: kindRequestHeader, HeaderName: "x-absent"}, // no default → omit
		{Key: "multi", Kind: kindRequestHeader, HeaderName: "x-multi"},
	}
	lookup := lookupFunc(map[string][]string{
		"x-present": {"PRESENT-VAL"},
		"x-multi":   {"MV-A", "MV-B"}, // multi-value → FIRST
	})
	got := ResolveCustomTags(specs, lookup)

	// Build a key→value map from the resolved KVs; assert presence + values by key
	// (omitted keys are simply absent).
	byKey := map[string]string{}
	for _, kv := range got {
		if _, dup := byKey[kv.Key]; dup {
			t.Errorf("duplicate resolved key %q", kv.Key)
		}
		byKey[kv.Key] = kv.Str
	}
	want := map[string]string{
		"lit":     "LIT-VAL",     // literal → static
		"present": "PRESENT-VAL", // header present → the header value (default ignored)
		"missdef": "def-m",       // header absent + default → the default
		"multi":   "MV-A",        // header sent twice → the FIRST value
	}
	for k, wv := range want {
		if gv, ok := byKey[k]; !ok || gv != wv {
			t.Errorf("resolved[%q] = %q (present=%v), want %q", k, gv, ok, wv)
		}
	}
	if _, ok := byKey["missnodef"]; ok {
		t.Errorf("resolved[missnodef] present, want OMITTED (header absent + no default)")
	}
	if len(got) != 4 {
		t.Errorf("len(resolved) = %d, want 4 (missnodef omitted)", len(got))
	}
}

// TestResolveCustomTagsNilLookup: a nil headerLookup (no request headers available)
// makes every request_header spec use its default / omit; literals are unaffected.
func TestResolveCustomTagsNilLookup(t *testing.T) {
	specs := []CustomTagSpec{
		{Key: "lit", Kind: kindLiteral, LiteralValue: "L"},
		{Key: "hdrdef", Kind: kindRequestHeader, HeaderName: "x", DefaultValue: "D", HasDefault: true},
		{Key: "hdrnodef", Kind: kindRequestHeader, HeaderName: "y"},
	}
	got := ResolveCustomTags(specs, nil)
	byKey := map[string]string{}
	for _, kv := range got {
		byKey[kv.Key] = kv.Str
	}
	if byKey["lit"] != "L" {
		t.Errorf("lit = %q, want L", byKey["lit"])
	}
	if byKey["hdrdef"] != "D" {
		t.Errorf("hdrdef = %q, want D (nil lookup → default)", byKey["hdrdef"])
	}
	if _, ok := byKey["hdrnodef"]; ok {
		t.Errorf("hdrnodef present, want OMITTED (nil lookup + no default)")
	}
}

// TestResolveCustomTagsEmptyPresentHeader: an EXISTING header with an empty value
// emits a present empty-string tag (NOT the default) — presence is the
// discriminator (SPEC §2 modeled edge; the lookup's bool is true for a
// present-but-empty header).
func TestResolveCustomTagsEmptyPresentHeader(t *testing.T) {
	specs := []CustomTagSpec{
		{Key: "e", Kind: kindRequestHeader, HeaderName: "x-empty", DefaultValue: "DEF", HasDefault: true},
	}
	lookup := lookupFunc(map[string][]string{"x-empty": {""}}) // present, empty value
	got := ResolveCustomTags(specs, lookup)
	if len(got) != 1 || got[0].Key != "e" || got[0].Str != "" {
		t.Errorf("resolved = %+v, want one {e, \"\"} (present empty, not the default)", got)
	}
}

// TestResolveCustomTagsEmpty: no specs → nil (byte-stable no-tags path).
func TestResolveCustomTagsEmpty(t *testing.T) {
	if got := ResolveCustomTags(nil, lookupFunc(nil)); got != nil {
		t.Errorf("ResolveCustomTags(nil, ...) = %+v, want nil", got)
	}
}
