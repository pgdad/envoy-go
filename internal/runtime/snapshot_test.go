package runtime

import (
	"sort"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

// mustStruct builds a *structpb.Struct from a Go map, failing the test on error.
func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	return s
}

// keysOf runs flatten over s and returns the emitted keys, sorted.
func keysOf(t *testing.T, s *structpb.Struct) []string {
	t.Helper()
	var got []string
	flatten("", s, func(k string) { got = append(got, k) })
	sort.Strings(got)
	return got
}

func TestFlatten_TerminationAndRecursion(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want []string
	}{
		// The four fixture arms, per-layer. MEASURED on the reference at the
		// phase-77 PLAN (3 fresh boots each); see PLAN §1.3.
		{"ArmA_FlatKeyWithLiteralDot", map[string]any{"ov.key": "from_L1"}, []string{"ov.key"}},
		{"ArmB_NestedTwoLeaves", map[string]any{
			"nest": map[string]any{"mid": map[string]any{"leaf1": 1, "leaf2": 2}},
		}, []string{"nest.mid.leaf1", "nest.mid.leaf2"}},
		{"ArmC_NumeratorTerminates", map[string]any{
			"frac": map[string]any{"numerator": 25, "foo": 2, "bar": 3},
		}, []string{"frac"}},
		{"ArmD_EmptyStructsAreLeaves", map[string]any{
			"emp": map[string]any{"e1": map[string]any{}, "e2": map[string]any{}},
		}, []string{"emp.e1", "emp.e2"}},

		// Discriminators. Each kills a plausible-but-wrong rule.
		{"NoTerminatorRecurses", map[string]any{
			"frac2": map[string]any{"foo": 2, "bar": 3, "baz": 4},
		}, []string{"frac2.bar", "frac2.baz", "frac2.foo"}},
		{"CaseSensitive_CapitalizedRecurses", map[string]any{
			"frac3": map[string]any{"Numerator": 25, "Denominator": "HUNDRED"},
		}, []string{"frac3.Denominator", "frac3.Numerator"}},
		{"DenominatorAloneTerminates", map[string]any{
			"frac4": map[string]any{"denominator": "HUNDRED", "foo": 1},
		}, []string{"frac4"}},
		{"TopLevelEmptyStructIsALeaf", map[string]any{
			"emp2": map[string]any{},
		}, []string{"emp2"}},
		// Values are NEVER inspected: this must TERMINATE, not error.
		{"InvalidValuesStillTerminate", map[string]any{
			"frac5": map[string]any{"numerator": "notanumber", "denominator": "NOTANENUM"},
		}, []string{"frac5"}},
		// One-field structs prove field count is irrelevant.
		{"OneFieldNonTerminatorRecurses", map[string]any{
			"k": map[string]any{"foo": 1},
		}, []string{"k.foo"}},
		{"OneFieldNumeratorTerminates", map[string]any{
			"k": map[string]any{"numerator": 25},
		}, []string{"k"}},
		// Unbounded depth; scalars and nests coexist.
		{"UnboundedDepth", map[string]any{
			"deep": map[string]any{"l2": map[string]any{"l3": map[string]any{"l4": 5}}},
		}, []string{"deep.l2.l3.l4"}},
		{"ScalarAndNestCoexist", map[string]any{
			"m": map[string]any{"n": 1}, "m2": 7,
		}, []string{"m.n", "m2"}},
		// A terminated struct nested under a recursing one.
		{"OuterRecursesInnerTerminates", map[string]any{
			"outer": map[string]any{"inner": map[string]any{"numerator": 1, "denominator": "HUNDRED"}},
		}, []string{"outer.inner"}},
	}
	if len(cases) != 14 {
		t.Fatalf("flatten roster: expected 14 rows (4 fixture arms + 10 discriminators); got %d", len(cases))
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := keysOf(t, mustStruct(t, tc.in))
			if len(got) != len(tc.want) {
				t.Errorf("%s: got %d keys %v, want %d keys %v", tc.name, len(got), got, len(tc.want), tc.want)
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("%s: key[%d] = %q, want %q (full: got %v want %v)", tc.name, i, got[i], tc.want[i], got, tc.want)
				}
			}
		})
	}
}

func TestFlatten_EmptyRootEmitsEmptyKey(t *testing.T) {
	// A degenerate case no reference arm produces, pinned so NewSnapshot's
	// drop-empty-keys behavior (Task 2) has something to stand on.
	got := keysOf(t, mustStruct(t, map[string]any{}))
	if len(got) != 1 || got[0] != "" {
		t.Errorf("empty root: got %v, want exactly one empty-string key", got)
	}
}

// combinedLayers is the EXACT two-layer shape fixture 0118 ships. MEASURED on
// envoyproxy/envoy:contrib-v1.37.2 at the phase-77 PLAN over 3 fresh boots:
// runtime.num_keys = 6, runtime.num_layers = 2, and the isolation arms sum
// exactly (1 + 2 + 1 + 2 = 6). See PLAN §1.3.
func combinedLayers(t *testing.T) []map[string]*structpb.Value {
	t.Helper()
	l1 := mustStruct(t, map[string]any{
		"ov.key": "from_L1",
		"nest":   map[string]any{"mid": map[string]any{"leaf1": 1, "leaf2": 2}},
		"frac":   map[string]any{"numerator": 25, "foo": 2, "bar": 3},
		"emp":    map[string]any{"e1": map[string]any{}, "e2": map[string]any{}},
	})
	l2 := mustStruct(t, map[string]any{"ov.key": "from_L2"})
	return []map[string]*structpb.Value{l1.GetFields(), l2.GetFields()}
}

func TestSnapshot_UnionAcrossLayers(t *testing.T) {
	s := NewSnapshot(combinedLayers(t))

	if got, want := s.NumLayers(), 2; got != want {
		t.Errorf("NumLayers() = %d, want %d", got, want)
	}
	if got, want := s.NumKeys(), 6; got != want {
		t.Errorf("NumKeys() = %d, want %d (reference-measured)", got, want)
	}
	want := []string{"emp.e1", "emp.e2", "frac", "nest.mid.leaf1", "nest.mid.leaf2", "ov.key"}
	got := s.Keys()
	if len(got) != len(want) {
		t.Errorf("Keys() = %v (%d), want %v (%d)", got, len(got), want, len(want))
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Keys()[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestSnapshot_OverlapCountsOnce(t *testing.T) {
	// The UNION-vs-per-layer-SUM discriminator. Reference-measured A-only arm:
	// num_keys = 1, num_layers = 2.
	// ⚠️ THE OVERLAP IS LOAD-BEARING. If a future edit removes `ov.key` from
	// L2, SUM == UNION and this test goes VACUOUS while still passing.
	l1 := mustStruct(t, map[string]any{"ov.key": "from_L1"})
	l2 := mustStruct(t, map[string]any{"ov.key": "from_L2"})
	s := NewSnapshot([]map[string]*structpb.Value{l1.GetFields(), l2.GetFields()})
	if got := s.NumKeys(); got != 1 {
		t.Errorf("overlap NumKeys() = %d, want 1 (UNION, not per-layer SUM)", got)
	}
	if got := s.NumLayers(); got != 2 {
		t.Errorf("overlap NumLayers() = %d, want 2", got)
	}
	// A per-layer SUM implementation gives 2 here and 7 on combinedLayers.
	// Both numbers are asserted, so neither can drift silently.
}

func TestSnapshot_Degenerate(t *testing.T) {
	if got := NewSnapshot(nil).NumKeys(); got != 0 {
		t.Errorf("nil layers: NumKeys() = %d, want 0", got)
	}
	if got := NewSnapshot(nil).NumLayers(); got != 0 {
		t.Errorf("nil layers: NumLayers() = %d, want 0", got)
	}
	// An empty layer contributes a layer but no keys, and MUST NOT contribute
	// the empty-string key that flatten emits for an empty root.
	empty := mustStruct(t, map[string]any{})
	s := NewSnapshot([]map[string]*structpb.Value{empty.GetFields()})
	if got := s.NumLayers(); got != 1 {
		t.Errorf("empty layer: NumLayers() = %d, want 1", got)
	}
	if got := s.NumKeys(); got != 0 {
		t.Errorf("empty layer: NumKeys() = %d, want 0 (the empty root key is dropped)", got)
	}
}

func TestSnapshot_KeysIsACopy(t *testing.T) {
	s := NewSnapshot(combinedLayers(t))
	k := s.Keys()
	k[0] = "MUTATED"
	if s.Keys()[0] == "MUTATED" {
		t.Error("Keys() returned an aliased slice; a caller can corrupt the Snapshot")
	}
}
