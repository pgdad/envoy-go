// Tests for the NEW internal/dynamicmetadata/ framework primitive per
// SPEC §3.1 + ADR-0190 §Context. Table-driven coverage of the *Bucket
// accessor: NewBucket + Get + Set + Snapshot + Reset semantics with
// nil-tolerance discipline per ADR-0085 and per-stream sequential
// access discipline per ADR-0033 (no goroutine-concurrency tests at
// this surface — per-stream sequential is the contract).
package dynamicmetadata

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

// TestBucket_NewBucket_returns_nonnil_empty verifies NewBucket returns a
// non-nil *Bucket with no entries.
func TestBucket_NewBucket_returns_nonnil_empty(t *testing.T) {
	b := NewBucket()
	if b == nil {
		t.Fatalf("NewBucket() returned nil; want non-nil *Bucket")
	}
	snap := b.Snapshot()
	if len(snap) != 0 {
		t.Errorf("NewBucket().Snapshot() has %d outer keys; want 0", len(snap))
	}
}

// TestBucket_Get_on_empty_returns_nil_false verifies Get on an empty
// bucket returns (nil, false) for any (filterName, key) coordinate.
func TestBucket_Get_on_empty_returns_nil_false(t *testing.T) {
	b := NewBucket()
	cases := []struct {
		name       string
		filterName string
		key        string
	}{
		{"empty_filter_empty_key", "", ""},
		{"named_filter_empty_key", "envoy.filters.http.lua", ""},
		{"empty_filter_named_key", "", "some.key"},
		{"named_filter_named_key", "envoy.filters.http.lua", "some.key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, ok := b.Get(tc.filterName, tc.key)
			if v != nil {
				t.Errorf("Get(%q, %q) value = %v; want nil", tc.filterName, tc.key, v)
			}
			if ok {
				t.Errorf("Get(%q, %q) ok = true; want false", tc.filterName, tc.key)
			}
		})
	}
}

// TestBucket_Set_then_Get_roundtrip verifies values written via Set are
// readable via Get at the same (filterName, key) coordinate.
func TestBucket_Set_then_Get_roundtrip(t *testing.T) {
	b := NewBucket()
	v := structpb.NewStringValue("hello")
	b.Set("envoy.filters.http.lua", "greeting", v)

	got, ok := b.Get("envoy.filters.http.lua", "greeting")
	if !ok {
		t.Fatalf("Get after Set returned ok=false; want true")
	}
	if got == nil {
		t.Fatalf("Get after Set returned nil value; want non-nil")
	}
	if gotStr, wantStr := got.GetStringValue(), v.GetStringValue(); gotStr != wantStr {
		t.Errorf("Get after Set value = %q; want %q", gotStr, wantStr)
	}
}

// TestBucket_Set_overwrites_existing verifies a second Set at the same
// coordinate overwrites the prior value (not append-only).
func TestBucket_Set_overwrites_existing(t *testing.T) {
	b := NewBucket()
	first := structpb.NewStringValue("first")
	second := structpb.NewStringValue("second")

	b.Set("envoy.filters.http.lua", "k", first)
	b.Set("envoy.filters.http.lua", "k", second)

	got, ok := b.Get("envoy.filters.http.lua", "k")
	if !ok {
		t.Fatalf("Get returned ok=false after two Sets; want true")
	}
	if got.GetStringValue() != "second" {
		t.Errorf("Get returned %q; want %q (Set must overwrite)", got.GetStringValue(), "second")
	}
}

// TestBucket_Snapshot_defensive_copy verifies mutating the returned
// snapshot does NOT mutate the underlying bucket. Per SPEC §3.1: the
// snapshot is a defensive copy of the outer + inner maps; the
// *structpb.Value pointers are shallow-shared (acceptable per SPEC).
func TestBucket_Snapshot_defensive_copy(t *testing.T) {
	b := NewBucket()
	b.Set("filter.a", "key.x", structpb.NewStringValue("ax"))
	b.Set("filter.a", "key.y", structpb.NewStringValue("ay"))
	b.Set("filter.b", "key.z", structpb.NewStringValue("bz"))

	snap := b.Snapshot()

	// Mutate the outer map: delete an entire filter row from the snapshot.
	delete(snap, "filter.a")
	// Mutate the inner map: insert a new key under filter.b in the snapshot.
	snap["filter.b"]["injected"] = structpb.NewStringValue("injected.value")

	// Verify the bucket itself is unchanged.
	if v, ok := b.Get("filter.a", "key.x"); !ok || v.GetStringValue() != "ax" {
		t.Errorf("after snapshot outer-mutation, bucket Get(filter.a, key.x) = (%v, %v); want (StringValue ax, true)", v, ok)
	}
	if v, ok := b.Get("filter.a", "key.y"); !ok || v.GetStringValue() != "ay" {
		t.Errorf("after snapshot outer-mutation, bucket Get(filter.a, key.y) = (%v, %v); want (StringValue ay, true)", v, ok)
	}
	if _, ok := b.Get("filter.b", "injected"); ok {
		t.Errorf("after snapshot inner-mutation, bucket Get(filter.b, injected) ok=true; want false (snapshot inner-map writes must NOT leak)")
	}
}

// TestBucket_Reset_clears_all_entries verifies Reset empties the bucket
// such that subsequent Get returns (nil, false) for previously-Set keys.
func TestBucket_Reset_clears_all_entries(t *testing.T) {
	b := NewBucket()
	b.Set("filter.a", "key.x", structpb.NewStringValue("ax"))
	b.Set("filter.b", "key.y", structpb.NewNumberValue(42))

	b.Reset()

	if v, ok := b.Get("filter.a", "key.x"); ok || v != nil {
		t.Errorf("after Reset, Get(filter.a, key.x) = (%v, %v); want (nil, false)", v, ok)
	}
	if v, ok := b.Get("filter.b", "key.y"); ok || v != nil {
		t.Errorf("after Reset, Get(filter.b, key.y) = (%v, %v); want (nil, false)", v, ok)
	}
	if snap := b.Snapshot(); len(snap) != 0 {
		t.Errorf("after Reset, Snapshot() has %d outer keys; want 0", len(snap))
	}

	// Verify the bucket is still usable after Reset (no nil-deref / no
	// destruction of the inner map shape).
	b.Set("filter.c", "key.z", structpb.NewStringValue("cz"))
	if v, ok := b.Get("filter.c", "key.z"); !ok || v.GetStringValue() != "cz" {
		t.Errorf("post-Reset Set+Get(filter.c, key.z) = (%v, %v); want (StringValue cz, true)", v, ok)
	}
}

// TestBucket_nil_tolerance verifies all four methods on a nil *Bucket
// produce safe no-op or zero-value behaviors per ADR-0085 nil-tolerance
// discipline. No panics; Get returns (nil, false); Set is a no-op;
// Snapshot returns nil; Reset is a no-op.
func TestBucket_nil_tolerance(t *testing.T) {
	var b *Bucket // nil receiver

	t.Run("Get_on_nil_returns_nil_false", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Get on nil bucket panicked: %v; want no panic", r)
			}
		}()
		v, ok := b.Get("filter.a", "key.x")
		if v != nil {
			t.Errorf("nil-bucket Get value = %v; want nil", v)
		}
		if ok {
			t.Errorf("nil-bucket Get ok = true; want false")
		}
	})

	t.Run("Set_on_nil_is_noop", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Set on nil bucket panicked: %v; want no panic", r)
			}
		}()
		b.Set("filter.a", "key.x", structpb.NewStringValue("ax"))
		// No state to verify on a nil bucket — the absence of panic is the
		// passing condition.
	})

	t.Run("Snapshot_on_nil_returns_nil", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Snapshot on nil bucket panicked: %v; want no panic", r)
			}
		}()
		snap := b.Snapshot()
		if snap != nil {
			t.Errorf("nil-bucket Snapshot = %v; want nil", snap)
		}
	})

	t.Run("Reset_on_nil_is_noop", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Reset on nil bucket panicked: %v; want no panic", r)
			}
		}()
		b.Reset()
	})
}

// TestBucket_structpb_payload_variations verifies the bucket round-trips
// every structpb.Value kind: null, number, string, bool, list, struct.
// Confirms the bucket is payload-agnostic (the contract is on the
// (filterName, key) coordinate, not the value type).
func TestBucket_structpb_payload_variations(t *testing.T) {
	listVal, err := structpb.NewList([]any{"a", float64(1), true})
	if err != nil {
		t.Fatalf("structpb.NewList setup error: %v", err)
	}
	structVal, err := structpb.NewStruct(map[string]any{
		"nested.string": "hello",
		"nested.number": float64(3.14),
	})
	if err != nil {
		t.Fatalf("structpb.NewStruct setup error: %v", err)
	}

	cases := []struct {
		name  string
		key   string
		value *structpb.Value
	}{
		{"null", "k.null", structpb.NewNullValue()},
		{"number", "k.number", structpb.NewNumberValue(3.14)},
		{"string", "k.string", structpb.NewStringValue("hello")},
		{"bool_true", "k.bool_true", structpb.NewBoolValue(true)},
		{"bool_false", "k.bool_false", structpb.NewBoolValue(false)},
		{"list", "k.list", structpb.NewListValue(listVal)},
		{"struct", "k.struct", structpb.NewStructValue(structVal)},
	}

	b := NewBucket()
	for _, tc := range cases {
		b.Set("filter.a", tc.key, tc.value)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := b.Get("filter.a", tc.key)
			if !ok {
				t.Fatalf("Get(filter.a, %q) ok=false; want true", tc.key)
			}
			if got == nil {
				t.Fatalf("Get(filter.a, %q) value=nil; want non-nil", tc.key)
			}
			// Pointer equality is sufficient since Set stores the pointer.
			if got != tc.value {
				t.Errorf("Get(filter.a, %q) value pointer mismatch; got %p want %p", tc.key, got, tc.value)
			}
		})
	}
}

// TestBucket_cross_filter_key_independence verifies the same key under
// different filterNames remains independent: a write under filter.a does
// NOT shadow or affect a read under filter.b at the same key.
func TestBucket_cross_filter_key_independence(t *testing.T) {
	b := NewBucket()
	b.Set("filter.a", "shared.key", structpb.NewStringValue("from.a"))
	b.Set("filter.b", "shared.key", structpb.NewStringValue("from.b"))

	vA, okA := b.Get("filter.a", "shared.key")
	if !okA || vA.GetStringValue() != "from.a" {
		t.Errorf("Get(filter.a, shared.key) = (%v, %v); want (StringValue from.a, true)", vA, okA)
	}

	vB, okB := b.Get("filter.b", "shared.key")
	if !okB || vB.GetStringValue() != "from.b" {
		t.Errorf("Get(filter.b, shared.key) = (%v, %v); want (StringValue from.b, true)", vB, okB)
	}

	// Overwriting filter.a's shared.key must NOT affect filter.b's shared.key.
	b.Set("filter.a", "shared.key", structpb.NewStringValue("from.a.updated"))

	vB2, okB2 := b.Get("filter.b", "shared.key")
	if !okB2 || vB2.GetStringValue() != "from.b" {
		t.Errorf("after filter.a overwrite, Get(filter.b, shared.key) = (%v, %v); want (StringValue from.b, true)", vB2, okB2)
	}

	// Snapshot shape confirmation: two distinct outer keys.
	snap := b.Snapshot()
	if got, want := len(snap), 2; got != want {
		t.Errorf("Snapshot outer-map len = %d; want %d", got, want)
	}
	if _, ok := snap["filter.a"]; !ok {
		t.Errorf("Snapshot missing outer key %q", "filter.a")
	}
	if _, ok := snap["filter.b"]; !ok {
		t.Errorf("Snapshot missing outer key %q", "filter.b")
	}
}
