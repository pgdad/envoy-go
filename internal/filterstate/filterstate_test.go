// Tests for the NEW internal/filterstate/ *Bucket accessor per 25.2 SPEC
// §3.2 + ADR-0207 + R-25.2-6 + Q7 + AMEND-B4. Covers Set/Get/Keys round-
// trip + read-only-vs-mutable conflict semantics + nil-handling +
// Marshal/Unmarshal round-trip via test FilterStateObject (defined in
// filterstateobject_test.go).
//
// The conflict semantic per ADR-0207 §Context: Mutable-state-type entries
// OVERRIDE existing entries; read-only-state-type entries with the same
// key as a Mutable entry are REJECTED (Set returns error). This mirrors
// upstream Envoy v1.37.2 FilterState::setData(...) semantics.
package filterstate

import (
	"bytes"
	"errors"
	"testing"
)

// TestBucket_NewBucket_NonNilEmpty verifies NewBucket returns a non-nil
// *Bucket with no entries (Keys() returns an empty slice).
func TestBucket_NewBucket_NonNilEmpty(t *testing.T) {
	b := NewBucket()
	if b == nil {
		t.Fatalf("NewBucket() returned nil; want non-nil *Bucket")
	}
	if keys := b.Keys(); len(keys) != 0 {
		t.Errorf("NewBucket().Keys() = %v; want empty", keys)
	}
}

// TestBucket_Get_OnEmpty_ReturnsNilFalse verifies Get on an empty bucket
// returns (nil, false) for any key.
func TestBucket_Get_OnEmpty_ReturnsNilFalse(t *testing.T) {
	b := NewBucket()
	cases := []string{"", "missing", "envoy.filters.http.lua", "deeply.nested.key"}
	for _, key := range cases {
		t.Run("key="+key, func(t *testing.T) {
			obj, ok := b.Get(key)
			if obj != nil {
				t.Errorf("Get(%q) obj = %v; want nil", key, obj)
			}
			if ok {
				t.Errorf("Get(%q) ok = true; want false", key)
			}
		})
	}
}

// TestBucket_SetGet_RoundTrip_ReadOnly verifies Set followed by Get
// returns the same object identity (no defensive copy at the *Bucket
// level — the FilterStateObject is stored by reference).
func TestBucket_SetGet_RoundTrip_ReadOnly(t *testing.T) {
	b := NewBucket()
	obj := &testFSO{data: []byte("payload"), mutable: false}
	if err := b.Set("key1", obj); err != nil {
		t.Fatalf("Set(%q, ...) err = %v; want nil", "key1", err)
	}
	got, ok := b.Get("key1")
	if !ok {
		t.Fatalf("Get(%q) ok = false; want true", "key1")
	}
	gotFSO, isTestFSO := got.(*testFSO)
	if !isTestFSO {
		t.Fatalf("Get(%q) returned %T; want *testFSO", "key1", got)
	}
	if gotFSO != obj {
		t.Errorf("Get(%q) returned different pointer identity: got %p, want %p",
			"key1", gotFSO, obj)
	}
	if !bytes.Equal(gotFSO.data, []byte("payload")) {
		t.Errorf("Get(%q).data = %q; want %q", "key1", gotFSO.data, "payload")
	}
	if gotFSO.StateType() != StateTypeReadOnly {
		t.Errorf("Get(%q).StateType() = %d; want %d", "key1",
			gotFSO.StateType(), StateTypeReadOnly)
	}
}

// TestBucket_SetGet_RoundTrip_Mutable verifies Set/Get for a mutable
// FilterStateObject (StateType = StateTypeMutable).
func TestBucket_SetGet_RoundTrip_Mutable(t *testing.T) {
	b := NewBucket()
	obj := &testFSO{data: []byte("mutable-payload"), mutable: true}
	if err := b.Set("k", obj); err != nil {
		t.Fatalf("Set err = %v; want nil", err)
	}
	got, ok := b.Get("k")
	if !ok {
		t.Fatalf("Get ok = false; want true")
	}
	if got.StateType() != StateTypeMutable {
		t.Errorf("Get().StateType() = %d; want %d", got.StateType(), StateTypeMutable)
	}
}

// TestBucket_Set_NilObject_Rejected verifies Set with a nil
// FilterStateObject returns a sentinel error (no nil-object storage; the
// per-key entry MUST always carry a usable object — Get treats absence
// as (nil, false), so storing a literal nil would confuse the
// presence/absence signal).
func TestBucket_Set_NilObject_Rejected(t *testing.T) {
	b := NewBucket()
	err := b.Set("k", nil)
	if err == nil {
		t.Fatalf("Set(%q, nil) err = nil; want non-nil sentinel error", "k")
	}
	if !errors.Is(err, ErrNilFilterStateObject) {
		t.Errorf("Set(%q, nil) err = %v; want errors.Is(err, ErrNilFilterStateObject)", "k", err)
	}
	// And the key MUST NOT have been inserted (Get returns absent).
	got, ok := b.Get("k")
	if ok || got != nil {
		t.Errorf("after Set(nil) reject, Get(%q) = (%v, %v); want (nil, false)", "k", got, ok)
	}
}

// TestBucket_Set_ReplaceMutableWithReadOnly_Rejected verifies the
// read-only-vs-mutable conflict semantic per ADR-0207 §Context — a Set
// of a StateTypeReadOnly object at a key that ALREADY holds a
// StateTypeMutable object returns a sentinel error + leaves the prior
// entry intact.
func TestBucket_Set_ReplaceMutableWithReadOnly_Rejected(t *testing.T) {
	b := NewBucket()
	mutObj := &testFSO{data: []byte("mut"), mutable: true}
	roObj := &testFSO{data: []byte("ro"), mutable: false}
	if err := b.Set("k", mutObj); err != nil {
		t.Fatalf("Set mutable err = %v; want nil", err)
	}
	err := b.Set("k", roObj)
	if err == nil {
		t.Fatalf("Set(ReadOnly) over existing Mutable: err = nil; want sentinel error")
	}
	if !errors.Is(err, ErrReadOnlyOverMutable) {
		t.Errorf("Set(ReadOnly) over Mutable err = %v; want errors.Is(err, ErrReadOnlyOverMutable)", err)
	}
	// Prior mutable entry preserved.
	got, ok := b.Get("k")
	if !ok || got != mutObj {
		t.Errorf("after rejected Set, Get(%q) = (%v, %v); want existing mutObj preserved", "k", got, ok)
	}
}

// TestBucket_Set_ReplaceReadOnlyWithMutable_OK verifies the converse —
// a Mutable Set OVER an existing ReadOnly entry is OK (the mutable
// override is allowed in this direction per ADR-0207).
func TestBucket_Set_ReplaceReadOnlyWithMutable_OK(t *testing.T) {
	b := NewBucket()
	roObj := &testFSO{data: []byte("ro"), mutable: false}
	mutObj := &testFSO{data: []byte("mut"), mutable: true}
	if err := b.Set("k", roObj); err != nil {
		t.Fatalf("Set ReadOnly err = %v; want nil", err)
	}
	if err := b.Set("k", mutObj); err != nil {
		t.Fatalf("Set Mutable over ReadOnly err = %v; want nil (override allowed)", err)
	}
	got, ok := b.Get("k")
	if !ok || got != mutObj {
		t.Errorf("after override, Get(%q) = (%v, %v); want mutObj installed", "k", got, ok)
	}
}

// TestBucket_Set_ReplaceMutableWithMutable_OK verifies same-type Mutable
// replacement is OK (the new mutable replaces the prior mutable).
func TestBucket_Set_ReplaceMutableWithMutable_OK(t *testing.T) {
	b := NewBucket()
	first := &testFSO{data: []byte("first"), mutable: true}
	second := &testFSO{data: []byte("second"), mutable: true}
	if err := b.Set("k", first); err != nil {
		t.Fatalf("Set first err = %v; want nil", err)
	}
	if err := b.Set("k", second); err != nil {
		t.Fatalf("Set second err = %v; want nil (mutable-over-mutable allowed)", err)
	}
	got, ok := b.Get("k")
	if !ok || got != second {
		t.Errorf("after replace, Get(%q) = (%v, %v); want second installed", "k", got, ok)
	}
}

// TestBucket_Set_ReplaceReadOnlyWithReadOnly_OK verifies same-type
// ReadOnly replacement is OK (per ADR-0207: only the
// "ReadOnly-replacing-Mutable" direction is rejected; same-type replace
// is fine).
func TestBucket_Set_ReplaceReadOnlyWithReadOnly_OK(t *testing.T) {
	b := NewBucket()
	first := &testFSO{data: []byte("first"), mutable: false}
	second := &testFSO{data: []byte("second"), mutable: false}
	if err := b.Set("k", first); err != nil {
		t.Fatalf("Set first err = %v; want nil", err)
	}
	if err := b.Set("k", second); err != nil {
		t.Fatalf("Set second err = %v; want nil (ro-over-ro allowed)", err)
	}
	got, ok := b.Get("k")
	if !ok || got != second {
		t.Errorf("after replace, Get(%q) = (%v, %v); want second installed", "k", got, ok)
	}
}

// TestBucket_Keys_ReturnsSortedSlice verifies Keys returns the entry
// keys in lexicographic order for deterministic property-tree
// enumeration (proxy-wasm `filter_state.*` dispatch enumerates this).
// Insertion order is intentionally NOT alphabetic so the test catches a
// regression where Keys reflects insertion order.
func TestBucket_Keys_ReturnsSortedSlice(t *testing.T) {
	b := NewBucket()
	for _, k := range []string{"c", "a", "b"} {
		if err := b.Set(k, &testFSO{data: []byte(k)}); err != nil {
			t.Fatalf("Set(%q) err = %v; want nil", k, err)
		}
	}
	got := b.Keys()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("Keys() len = %d; want %d (got=%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Keys()[%d] = %q; want %q (full got=%v)", i, got[i], w, got)
		}
	}
}

// TestBucket_Keys_EmptyBucket_ReturnsEmptySlice verifies Keys on an
// empty bucket returns an empty slice (NOT nil; callers may range over
// it without a nil guard).
func TestBucket_Keys_EmptyBucket_ReturnsEmptySlice(t *testing.T) {
	b := NewBucket()
	got := b.Keys()
	if got == nil {
		t.Errorf("Keys() on empty = nil; want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("Keys() on empty has len %d; want 0", len(got))
	}
}

// TestBucket_Keys_AfterMultipleSets verifies Keys correctness for a
// realistic-size bucket population (many entries, mixed key prefixes).
func TestBucket_Keys_AfterMultipleSets(t *testing.T) {
	b := NewBucket()
	keys := []string{
		"envoy.filters.http.lua",
		"envoy.filters.http.wasm",
		"envoy.filters.http.ext_authz",
		"upstream_filter_state.cluster_specifier",
		"upstream_filter_state.lb_endpoint",
		"a.b.c",
		"a.b.d",
	}
	for _, k := range keys {
		if err := b.Set(k, &testFSO{data: []byte(k)}); err != nil {
			t.Fatalf("Set(%q) err = %v; want nil", k, err)
		}
	}
	got := b.Keys()
	if len(got) != len(keys) {
		t.Fatalf("Keys() len = %d; want %d", len(got), len(keys))
	}
	// Verify sorted.
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("Keys() not sorted at index %d: %q >= %q (full=%v)",
				i, got[i-1], got[i], got)
		}
	}
}

// TestBucket_MarshalUnmarshal_RoundTrip_ViaBucket verifies that the
// FilterStateObject Marshal/Unmarshal contract round-trips through a
// *Bucket Set/Get cycle. Documents the cross-package consumer pattern —
// the wasm consumer at Task 13 calls Marshal on a Get'd object to emit
// the property-tree byte payload.
func TestBucket_MarshalUnmarshal_RoundTrip_ViaBucket(t *testing.T) {
	b := NewBucket()
	original := &testFSO{data: []byte("byte-payload"), mutable: false}
	if err := b.Set("k", original); err != nil {
		t.Fatalf("Set err = %v; want nil", err)
	}
	got, ok := b.Get("k")
	if !ok {
		t.Fatalf("Get ok = false; want true")
	}
	marshaled, err := got.Marshal()
	if err != nil {
		t.Fatalf("Get().Marshal() err = %v; want nil", err)
	}
	if !bytes.Equal(marshaled, []byte("byte-payload")) {
		t.Errorf("Get().Marshal() = %q; want %q", marshaled, "byte-payload")
	}
	// And a fresh FilterStateObject can re-hydrate from the marshaled
	// bytes via Unmarshal.
	restored := &testFSO{}
	if err := restored.Unmarshal(marshaled); err != nil {
		t.Fatalf("Unmarshal err = %v; want nil", err)
	}
	if !bytes.Equal(restored.data, []byte("byte-payload")) {
		t.Errorf("after Unmarshal, restored.data = %q; want %q", restored.data, "byte-payload")
	}
}
