// Package runtime materializes the bootstrap's layered_runtime static layers
// into a flat, precedence-collapsed key space.
package runtime

import (
	"sort"

	"google.golang.org/protobuf/types/known/structpb"
)

// Termination field names. The reference's flattener stops descending at a
// Struct carrying EITHER of these, matched LEXICALLY and CASE-SENSITIVELY.
// The VALUES are never inspected: {numerator: "notanumber"} terminates and
// boots cleanly on the reference, so parsing a FractionalPercent here would
// REJECT configs the reference ACCEPTS. Measured over 15 arms, 3x each, at
// the phase-77 SPEC (§3.3.2) and re-measured at the phase-77 PLAN (§1.3).
const (
	terminatorNumerator   = "numerator"
	terminatorDenominator = "denominator"
)

// flatten walks s and calls emit once per leaf key, joining path segments with
// '.'. prefix is the accumulated path ("" at the root, so root fields emit
// bare). There are exactly TWO termination branches:
//
//  1. LEXICAL — the Struct carries a field literally named "numerator" or
//     "denominator". Either alone suffices; additional fields are irrelevant;
//     field count is irrelevant ({foo: 1} recurses while {numerator: 25}
//     terminates).
//  2. EMPTY — the Struct has zero fields. An empty Struct is a COUNTED LEAF,
//     not zero keys: `e: {f: {}}` yields the single key `e.f`. This branch is
//     recorded by no document before the phase-77 SPEC (§3.3.3), and the
//     inherited three-arm pin set could not have detected its absence.
//
// A field name containing a literal '.' is NOT re-split: `ov.key` emits
// `ov.key` verbatim. Measured cross-side.
func flatten(prefix string, s *structpb.Struct, emit func(string)) {
	fields := s.GetFields()
	if _, ok := fields[terminatorNumerator]; ok {
		emit(prefix)
		return
	}
	if _, ok := fields[terminatorDenominator]; ok {
		emit(prefix)
		return
	}
	if len(fields) == 0 {
		emit(prefix)
		return
	}
	for name, v := range fields {
		child := name
		if prefix != "" {
			child = prefix + "." + name
		}
		if sv, ok := v.GetKind().(*structpb.Value_StructValue); ok {
			flatten(child, sv.StructValue, emit)
			continue
		}
		emit(child)
	}
}

// Snapshot is the precedence-collapsed key space of a bootstrap's declared
// layered_runtime static layers. It is built once, at Load time, and is
// immutable thereafter.
//
// ⚠️ envoy-go's snapshot is BOOT-FIXED where the reference's is LIVE: the
// reference's runtime.num_keys moves when an admin layer is written through
// POST /runtime_modify. envoy-go ships no write path (row 77 lands neither
// /runtime nor /runtime_modify), so the two agree — see PLAN §5.
type Snapshot struct {
	keys      []string // sorted, distinct
	numLayers int
}

// NewSnapshot flattens each layer and unions the resulting key spaces. layers
// is one field-map per DECLARED layer, in precedence order (later layers
// override earlier ones). The override VALUE is not retained: this row serves
// no /runtime endpoint, and the reference's within-layer collision winner is
// NON-DETERMINISTIC across process starts (~40/60 over 18 fresh processes,
// phase-77 SPEC §3.3.1), so a value is not a thing envoy-go can agree with
// cross-side. The distinct-key COUNT and the key SET both are.
func NewSnapshot(layers []map[string]*structpb.Value) *Snapshot {
	seen := make(map[string]struct{})
	for _, fields := range layers {
		flatten("", &structpb.Struct{Fields: fields}, func(k string) {
			if k == "" {
				return // the degenerate empty-root key; never a real runtime key
			}
			seen[k] = struct{}{}
		})
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return &Snapshot{keys: keys, numLayers: len(layers)}
}

// NumKeys is the distinct-key count across all layers (the UNION, not the
// per-layer sum). Published as runtime.num_keys.
func (s *Snapshot) NumKeys() int { return len(s.keys) }

// NumLayers is the number of DECLARED layers. Published as runtime.num_layers.
func (s *Snapshot) NumLayers() int { return s.numLayers }

// Keys returns the sorted distinct key set. The slice is freshly allocated per
// call, so a caller cannot mutate the Snapshot.
func (s *Snapshot) Keys() []string {
	out := make([]string, len(s.keys))
	copy(out, s.keys)
	return out
}
