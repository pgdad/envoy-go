package tracing

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

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
	got := ResolveCustomTags(specs, lookup, nil, nil)

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
	got := ResolveCustomTags(specs, nil, nil, nil)
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
	got := ResolveCustomTags(specs, lookup, nil, nil)
	if len(got) != 1 || got[0].Key != "e" || got[0].Str != "" {
		t.Errorf("resolved = %+v, want one {e, \"\"} (present empty, not the default)", got)
	}
}

// TestResolveCustomTagsEmpty: no specs → nil (byte-stable no-tags path).
func TestResolveCustomTagsEmpty(t *testing.T) {
	if got := ResolveCustomTags(nil, lookupFunc(nil), nil, nil); got != nil {
		t.Errorf("ResolveCustomTags(nil, ...) = %+v, want nil", got)
	}
}

// TestResolveCustomTagsEnvironment drives the kindEnvironment source arm: env
// present → the env value (the default is IGNORED); env absent + default → the
// default; env absent + no default → OMIT; env PRESENT-but-EMPTY → OMIT (SPEC-63 §11
// arm G — present-ness is honored via os.LookupEnv, the default is NOT used, and an
// empty resolved value is omitted). Together: OMIT iff the resolved value is empty.
// headerLookup is IGNORED by this arm (passed nil). t.Setenv gives hermetic env
// control. Errorf per subtest so one failing case does not mask the rest.
func TestResolveCustomTagsEnvironment(t *testing.T) {
	// A name that is not set in the test environment (absent cases).
	const absent = "ENVOY_GO_TEST_ABSENT_XYZ"

	t.Run("present-uses-env-value", func(t *testing.T) {
		t.Setenv("ENVOY_GO_TEST_PRESENT", "PRESENT-VAL")
		specs := []CustomTagSpec{{Key: "e", Kind: kindEnvironment, EnvName: "ENVOY_GO_TEST_PRESENT", DefaultValue: "def"}}
		got := ResolveCustomTags(specs, nil, nil, nil)
		if len(got) != 1 || got[0].Key != "e" || got[0].Str != "PRESENT-VAL" {
			t.Errorf("present: got %+v, want one {e, PRESENT-VAL} (env value, default ignored)", got)
		}
	})
	t.Run("absent-with-default", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "e", Kind: kindEnvironment, EnvName: absent, DefaultValue: "def-m"}}
		got := ResolveCustomTags(specs, nil, nil, nil)
		if len(got) != 1 || got[0].Str != "def-m" {
			t.Errorf("absent+default: got %+v, want one {e, def-m}", got)
		}
	})
	t.Run("absent-no-default-omits", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "e", Kind: kindEnvironment, EnvName: absent}}
		got := ResolveCustomTags(specs, nil, nil, nil)
		if len(got) != 0 {
			t.Errorf("absent+no-default: got %+v, want OMITTED (empty resolved value)", got)
		}
	})
	t.Run("present-empty-omits", func(t *testing.T) {
		t.Setenv("ENVOY_GO_TEST_EMPTY", "") // present, empty string
		specs := []CustomTagSpec{{Key: "e", Kind: kindEnvironment, EnvName: "ENVOY_GO_TEST_EMPTY", DefaultValue: "def-empty"}}
		got := ResolveCustomTags(specs, nil, nil, nil)
		if len(got) != 0 {
			t.Errorf("present-empty: got %+v, want OMITTED (present-empty ignores the default, arm G)", got)
		}
	})
}

// metaFunc builds a metaLookup from a (ns,key)→*structpb.Value map keyed on
// "ns\x00key". Present-ness is the map membership (a present nil value still
// reports ok=true, mirroring a resolved-but-boundary metadata value).
func metaFunc(m map[string]*structpb.Value) func(ns, key string) (*structpb.Value, bool) {
	return func(ns, key string) (*structpb.Value, bool) {
		v, ok := m[ns+"\x00"+key]
		return v, ok
	}
}

func mustStruct(t *testing.T, m map[string]interface{}) *structpb.Value {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatalf("structpb.NewStruct(%v): %v", m, err)
	}
	return structpb.NewStructValue(s)
}

func mustList(t *testing.T, vs ...interface{}) *structpb.Value {
	t.Helper()
	l, err := structpb.NewList(vs)
	if err != nil {
		t.Fatalf("structpb.NewList(%v): %v", vs, err)
	}
	return structpb.NewListValue(l)
}

// TestResolveCustomTagsMetadataPathWalk drives the kindMetadata path walk: a
// single-segment lookup, a multi-segment nested descent, and an unresolvable
// middle segment that falls to the default.
func TestResolveCustomTagsMetadataPathWalk(t *testing.T) {
	t.Run("single-segment", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k"}}}
		ml := metaFunc(map[string]*structpb.Value{"ns\x00k": structpb.NewStringValue("v")})
		got := ResolveCustomTags(specs, nil, ml, nil)
		if len(got) != 1 || got[0].Key != "m" || got[0].Str != "v" {
			t.Errorf("single: got %+v, want one {m, v}", got)
		}
	})
	t.Run("multi-segment", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k", "a", "b"}}}
		// nested {a:{b:"deep"}}
		nested := mustStruct(t, map[string]interface{}{"a": map[string]interface{}{"b": "deep"}})
		ml := metaFunc(map[string]*structpb.Value{"ns\x00k": nested})
		got := ResolveCustomTags(specs, nil, ml, nil)
		if len(got) != 1 || got[0].Str != "deep" {
			t.Errorf("multi: got %+v, want one {m, deep}", got)
		}
	})
	t.Run("unresolvable-middle-falls-to-default", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k", "MISSING", "b"}, DefaultValue: "DEF", HasDefault: true}}
		nested := mustStruct(t, map[string]interface{}{"a": map[string]interface{}{"b": "deep"}})
		ml := metaFunc(map[string]*structpb.Value{"ns\x00k": nested})
		got := ResolveCustomTags(specs, nil, ml, nil)
		if len(got) != 1 || got[0].Str != "DEF" {
			t.Errorf("unresolvable-middle: got %+v, want one {m, DEF} (default)", got)
		}
	})
	t.Run("unresolvable-middle-no-default-omits", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k", "MISSING"}}}
		nested := mustStruct(t, map[string]interface{}{"a": map[string]interface{}{"b": "deep"}})
		ml := metaFunc(map[string]*structpb.Value{"ns\x00k": nested})
		got := ResolveCustomTags(specs, nil, ml, nil)
		if len(got) != 0 {
			t.Errorf("unresolvable-middle-no-default: got %+v, want OMITTED", got)
		}
	})
}

// TestResolveCustomTagsMetadataSerialize drives the P3 serialization table:
// string→raw (no quotes), number→decimal, bool→true|false, struct/list→compact
// JSON (json.Compact-compared EXACT), NullValue→unresolvable (default/omit).
func TestResolveCustomTagsMetadataSerialize(t *testing.T) {
	cases := []struct {
		name string
		val  *structpb.Value
		want string // "" with wantOmit means omitted
	}{
		{"string-raw", structpb.NewStringValue("x"), "x"},
		{"number-int", structpb.NewNumberValue(42), "42"},
		{"number-float", structpb.NewNumberValue(3.14), "3.14"},
		{"bool-true", structpb.NewBoolValue(true), "true"},
		{"bool-false", structpb.NewBoolValue(false), "false"},
		{"struct", mustStruct(t, map[string]interface{}{"a": "b"}), `{"a":"b"}`},
		{"list", mustList(t, "x", "y", "z"), `["x","y","z"]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k"}}}
			ml := metaFunc(map[string]*structpb.Value{"ns\x00k": c.val})
			got := ResolveCustomTags(specs, nil, ml, nil)
			if len(got) != 1 || got[0].Str != c.want {
				t.Errorf("%s: got %+v, want one {m, %q}", c.name, got, c.want)
			}
		})
	}
	// NullValue → boundary → default/omit (here: default).
	t.Run("null-falls-to-default", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k"}, DefaultValue: "DEF", HasDefault: true}}
		ml := metaFunc(map[string]*structpb.Value{"ns\x00k": structpb.NewNullValue()})
		got := ResolveCustomTags(specs, nil, ml, nil)
		if len(got) != 1 || got[0].Str != "DEF" {
			t.Errorf("null: got %+v, want one {m, DEF} (default)", got)
		}
	})
}

// TestResolveCustomTagsMetadataMatrix drives the P4 present/absent/default matrix:
// present-non-empty→emit; present-EMPTY (structpb "")→emit "" (NOT default);
// absent+HasDefault→default; absent+no-default→omit.
func TestResolveCustomTagsMetadataMatrix(t *testing.T) {
	t.Run("present-non-empty-emits", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k"}, DefaultValue: "DEF", HasDefault: true}}
		ml := metaFunc(map[string]*structpb.Value{"ns\x00k": structpb.NewStringValue("V")})
		got := ResolveCustomTags(specs, nil, ml, nil)
		if len(got) != 1 || got[0].Str != "V" {
			t.Errorf("present-non-empty: got %+v, want one {m, V}", got)
		}
	})
	t.Run("present-empty-emits-empty", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k"}, DefaultValue: "DEF", HasDefault: true}}
		ml := metaFunc(map[string]*structpb.Value{"ns\x00k": structpb.NewStringValue("")})
		got := ResolveCustomTags(specs, nil, ml, nil)
		if len(got) != 1 || got[0].Key != "m" || got[0].Str != "" {
			t.Errorf("present-empty: got %+v, want one {m, \"\"} (present-empty EMITS \"\", NOT the default)", got)
		}
	})
	t.Run("absent-with-default", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k"}, DefaultValue: "DEF", HasDefault: true}}
		ml := metaFunc(map[string]*structpb.Value{}) // absent
		got := ResolveCustomTags(specs, nil, ml, nil)
		if len(got) != 1 || got[0].Str != "DEF" {
			t.Errorf("absent+default: got %+v, want one {m, DEF}", got)
		}
	})
	t.Run("absent-no-default-omits", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k"}}}
		ml := metaFunc(map[string]*structpb.Value{}) // absent
		got := ResolveCustomTags(specs, nil, ml, nil)
		if len(got) != 0 {
			t.Errorf("absent+no-default: got %+v, want OMITTED", got)
		}
	})
}

// TestResolveCustomTagsMetadataNilLookup: a kindMetadata spec with a nil
// metaLookup falls to default / omit, no panic.
func TestResolveCustomTagsMetadataNilLookup(t *testing.T) {
	t.Run("nil-lookup-with-default", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k"}, DefaultValue: "DEF", HasDefault: true}}
		got := ResolveCustomTags(specs, nil, nil, nil)
		if len(got) != 1 || got[0].Str != "DEF" {
			t.Errorf("nil-lookup+default: got %+v, want one {m, DEF}", got)
		}
	})
	t.Run("nil-lookup-no-default-omits", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadata, MetaNamespace: "ns", MetaPath: []string{"k"}}}
		got := ResolveCustomTags(specs, nil, nil, nil)
		if len(got) != 0 {
			t.Errorf("nil-lookup+no-default: got %+v, want OMITTED", got)
		}
	})
}

// routeMetaFunc builds a routeMetaLookup from a ns→*structpb.Value map. Unlike
// metaFunc (which is keyed on ns+key, pre-keyed to MetaPath[0]), a ROUTE lookup
// returns the WHOLE namespace struct — the resolve arm descends the FULL MetaPath
// from it (RD-ROUTE-ARM, distinct from the REQUEST arm's MetaPath[1:]).
func routeMetaFunc(m map[string]*structpb.Value) func(ns string) (*structpb.Value, bool) {
	return func(ns string) (*structpb.Value, bool) {
		v, ok := m[ns]
		return v, ok
	}
}

// TestResolveCustomTagsRouteMetadataPathWalk drives the kindMetadataRoute path
// walk: a single-segment descent from the namespace struct, a multi-segment
// nested descent, and an unresolvable segment that falls to the default/omit.
// The ROUTE fake returns the WHOLE namespace struct (not pre-keyed to
// MetaPath[0] like the REQUEST metaFunc) — a single-segment path ["k"] descends
// ONE level from the namespace struct.
func TestResolveCustomTagsRouteMetadataPathWalk(t *testing.T) {
	t.Run("single-segment", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"k"}}}
		nsStruct := mustStruct(t, map[string]interface{}{"k": "v"})
		rl := routeMetaFunc(map[string]*structpb.Value{"ns": nsStruct})
		got := ResolveCustomTags(specs, nil, nil, rl)
		if len(got) != 1 || got[0].Key != "m" || got[0].Str != "v" {
			t.Errorf("single: got %+v, want one {m, v}", got)
		}
	})
	t.Run("multi-segment", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"a", "b"}}}
		nsStruct := mustStruct(t, map[string]interface{}{"a": map[string]interface{}{"b": "deep"}})
		rl := routeMetaFunc(map[string]*structpb.Value{"ns": nsStruct})
		got := ResolveCustomTags(specs, nil, nil, rl)
		if len(got) != 1 || got[0].Str != "deep" {
			t.Errorf("multi: got %+v, want one {m, deep}", got)
		}
	})
	t.Run("unresolvable-segment-falls-to-default", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"MISSING"}, DefaultValue: "DEF", HasDefault: true}}
		nsStruct := mustStruct(t, map[string]interface{}{"a": map[string]interface{}{"b": "deep"}})
		rl := routeMetaFunc(map[string]*structpb.Value{"ns": nsStruct})
		got := ResolveCustomTags(specs, nil, nil, rl)
		if len(got) != 1 || got[0].Str != "DEF" {
			t.Errorf("unresolvable: got %+v, want one {m, DEF} (default)", got)
		}
	})
	t.Run("unresolvable-segment-no-default-omits", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"MISSING"}}}
		nsStruct := mustStruct(t, map[string]interface{}{"a": map[string]interface{}{"b": "deep"}})
		rl := routeMetaFunc(map[string]*structpb.Value{"ns": nsStruct})
		got := ResolveCustomTags(specs, nil, nil, rl)
		if len(got) != 0 {
			t.Errorf("unresolvable-no-default: got %+v, want OMITTED", got)
		}
	})
}

// TestResolveCustomTagsRouteMetadataSerialize drives the P3 serialization table
// for the ROUTE arm (a thin re-assert of the shared structpbValueToString path,
// NOT a re-derivation): string→raw (no quotes); number→decimal; bool→true|false;
// struct/list→compact JSON (json.Compact-compared EXACT); NullValue→unresolvable
// (default/omit).
func TestResolveCustomTagsRouteMetadataSerialize(t *testing.T) {
	cases := []struct {
		name string
		val  *structpb.Value
		want string
	}{
		{"string-raw", structpb.NewStringValue("x"), "x"},
		{"number-int", structpb.NewNumberValue(42), "42"},
		{"number-float", structpb.NewNumberValue(3.14), "3.14"},
		{"bool-true", structpb.NewBoolValue(true), "true"},
		{"bool-false", structpb.NewBoolValue(false), "false"},
		{"struct", mustStruct(t, map[string]interface{}{"a": "b"}), `{"a":"b"}`},
		{"list", mustList(t, "x", "y", "z"), `["x","y","z"]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"k"}}}
			// Build the namespace struct with field "k" set to c.val directly
			// (structpb.NewStruct cannot hold arbitrary *structpb.Value, so build
			// the wrapping struct by hand via the Fields map).
			wrapped := &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
				Fields: map[string]*structpb.Value{"k": c.val},
			}}}
			rl := routeMetaFunc(map[string]*structpb.Value{"ns": wrapped})
			got := ResolveCustomTags(specs, nil, nil, rl)
			if len(got) != 1 || got[0].Str != c.want {
				t.Errorf("%s: got %+v, want one {m, %q}", c.name, got, c.want)
			}
		})
	}
	// NullValue → boundary → default/omit (here: default).
	t.Run("null-falls-to-default", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"k"}, DefaultValue: "DEF", HasDefault: true}}
		wrapped := &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
			Fields: map[string]*structpb.Value{"k": structpb.NewNullValue()},
		}}}
		rl := routeMetaFunc(map[string]*structpb.Value{"ns": wrapped})
		got := ResolveCustomTags(specs, nil, nil, rl)
		if len(got) != 1 || got[0].Str != "DEF" {
			t.Errorf("null: got %+v, want one {m, DEF} (default)", got)
		}
	})
}

// TestResolveCustomTagsRouteMetadataMatrix drives the P4 present/absent/default
// matrix for the ROUTE arm: present-non-empty→emit; present-EMPTY (structpb "")
// →emit "" (NOT default); absent namespace+HasDefault→default;
// absent+no-default→omit.
func TestResolveCustomTagsRouteMetadataMatrix(t *testing.T) {
	t.Run("present-non-empty-emits", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"k"}, DefaultValue: "DEF", HasDefault: true}}
		nsStruct := mustStruct(t, map[string]interface{}{"k": "V"})
		rl := routeMetaFunc(map[string]*structpb.Value{"ns": nsStruct})
		got := ResolveCustomTags(specs, nil, nil, rl)
		if len(got) != 1 || got[0].Str != "V" {
			t.Errorf("present-non-empty: got %+v, want one {m, V}", got)
		}
	})
	t.Run("present-empty-emits-empty", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"k"}, DefaultValue: "DEF", HasDefault: true}}
		nsStruct := mustStruct(t, map[string]interface{}{"k": ""})
		rl := routeMetaFunc(map[string]*structpb.Value{"ns": nsStruct})
		got := ResolveCustomTags(specs, nil, nil, rl)
		if len(got) != 1 || got[0].Key != "m" || got[0].Str != "" {
			t.Errorf("present-empty: got %+v, want one {m, \"\"} (present-empty EMITS \"\", NOT the default)", got)
		}
	})
	t.Run("absent-namespace-with-default", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"k"}, DefaultValue: "DEF", HasDefault: true}}
		rl := routeMetaFunc(map[string]*structpb.Value{}) // absent
		got := ResolveCustomTags(specs, nil, nil, rl)
		if len(got) != 1 || got[0].Str != "DEF" {
			t.Errorf("absent+default: got %+v, want one {m, DEF}", got)
		}
	})
	t.Run("absent-namespace-no-default-omits", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"k"}}}
		rl := routeMetaFunc(map[string]*structpb.Value{}) // absent
		got := ResolveCustomTags(specs, nil, nil, rl)
		if len(got) != 0 {
			t.Errorf("absent+no-default: got %+v, want OMITTED", got)
		}
	})
}

// TestResolveCustomTagsRouteMetadataNilLookup: a kindMetadataRoute spec with a
// nil routeMetaLookup falls to default / omit, no panic.
func TestResolveCustomTagsRouteMetadataNilLookup(t *testing.T) {
	t.Run("nil-lookup-with-default", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"k"}, DefaultValue: "DEF", HasDefault: true}}
		got := ResolveCustomTags(specs, nil, nil, nil)
		if len(got) != 1 || got[0].Str != "DEF" {
			t.Errorf("nil-lookup+default: got %+v, want one {m, DEF}", got)
		}
	})
	t.Run("nil-lookup-no-default-omits", func(t *testing.T) {
		specs := []CustomTagSpec{{Key: "m", Kind: kindMetadataRoute, MetaNamespace: "ns", MetaPath: []string{"k"}}}
		got := ResolveCustomTags(specs, nil, nil, nil)
		if len(got) != 0 {
			t.Errorf("nil-lookup+no-default: got %+v, want OMITTED", got)
		}
	})
}
