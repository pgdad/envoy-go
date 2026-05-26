// Tests for the NEW internal/filterstate/ FilterStateObject interface +
// StateType discriminator per 25.2 SPEC §3.2 + ADR-0207 + R-25.2-6 + Q7 +
// AMEND-B4. Covers interface conformance (compile-time assertion via
// `var _ FilterStateObject = (*testFSO)(nil)`) + edge cases on the test
// FilterStateObject implementation (HasData / StateType / Marshal /
// Unmarshal); the production *Bucket plumbing lives in filterstate_test.go.
//
// The test type `testFSO` lives here (rather than in filterstate_test.go)
// so it is reusable across the package's three test files (filterstate_test.go
// + bucket_concurrency_test.go) without re-declaration. Go-test convention
// permits cross-test-file struct sharing inside the same _test package
// (the test files compile as a unit).
package filterstate

import (
	"bytes"
	"testing"
)

// testFSO is the canonical test FilterStateObject used across the package's
// three test files. Carries a data slice + a mutable bool that flips the
// StateType discriminator between ReadOnly + Mutable. Marshal returns a
// defensive copy of data (immutable from the consumer's perspective);
// Unmarshal overwrites data with a defensive copy of the input. HasData
// returns true iff len(data) > 0 (matches upstream Envoy's HasData
// semantic — "the object carries serializable data").
type testFSO struct {
	data    []byte
	mutable bool
}

// Compile-time interface conformance assertion. Documents the contract
// at the test layer; the production *Bucket consumers rely on the same
// satisfaction. The cast does NOT panic at runtime — it is purely a
// compile-time witness.
var _ FilterStateObject = (*testFSO)(nil)

func (t *testFSO) Marshal() ([]byte, error) {
	if t == nil {
		return nil, nil
	}
	if t.data == nil {
		return nil, nil
	}
	// Defensive copy — Marshal MUST NOT expose the underlying buffer
	// (consumer mutation of the returned slice MUST NOT mutate the
	// FilterStateObject; mirrors upstream Envoy's StreamInfo::FilterState
	// serializeAsString returning a fresh string).
	out := make([]byte, len(t.data))
	copy(out, t.data)
	return out, nil
}

func (t *testFSO) Unmarshal(b []byte) error {
	if t == nil {
		return nil
	}
	if b == nil {
		t.data = nil
		return nil
	}
	t.data = make([]byte, len(b))
	copy(t.data, b)
	return nil
}

func (t *testFSO) HasData() bool {
	if t == nil {
		return false
	}
	return len(t.data) > 0
}

func (t *testFSO) StateType() StateType {
	if t == nil {
		return StateTypeReadOnly
	}
	if t.mutable {
		return StateTypeMutable
	}
	return StateTypeReadOnly
}

// TestFilterStateObject_InterfaceConformance documents the compile-time
// satisfaction assertion at the test layer. The assertion above
// (`var _ FilterStateObject = (*testFSO)(nil)`) is the load-bearing
// witness — this test exists so the assertion is exercised at a named
// test boundary AND so each interface method is invoked through the
// interface dispatch path (not just the concrete-type path).
func TestFilterStateObject_InterfaceConformance(t *testing.T) {
	var fso FilterStateObject = &testFSO{data: []byte("witness")}
	// Exercise each method through the interface (not the concrete type)
	// so a future refactor that breaks the interface dispatch surface
	// fails here at a named-test boundary.
	if !fso.HasData() {
		t.Errorf("fso.HasData() = false; want true")
	}
	if got := fso.StateType(); got != StateTypeReadOnly {
		t.Errorf("fso.StateType() = %d; want %d", got, StateTypeReadOnly)
	}
	b, err := fso.Marshal()
	if err != nil {
		t.Fatalf("fso.Marshal() err = %v; want nil", err)
	}
	if string(b) != "witness" {
		t.Errorf("fso.Marshal() = %q; want %q", b, "witness")
	}
	if err := fso.Unmarshal([]byte("rebuilt")); err != nil {
		t.Fatalf("fso.Unmarshal() err = %v; want nil", err)
	}
}

// TestFilterStateObject_HasData_OnEmpty verifies HasData returns false
// when the object carries no serializable data (per upstream Envoy
// HasData semantic).
func TestFilterStateObject_HasData_OnEmpty(t *testing.T) {
	obj := &testFSO{}
	if obj.HasData() {
		t.Errorf("HasData() on empty testFSO = true; want false")
	}
}

// TestFilterStateObject_HasData_OnPopulated verifies HasData returns true
// when the object carries any non-empty data payload.
func TestFilterStateObject_HasData_OnPopulated(t *testing.T) {
	obj := &testFSO{data: []byte("hello")}
	if !obj.HasData() {
		t.Errorf("HasData() on populated testFSO = false; want true")
	}
}

// TestFilterStateObject_StateType_ConstValuesPinned verifies the
// StateType const values are pinned to their documented integers
// (StateTypeReadOnly = 0; StateTypeMutable = 1) per 25.2 SPEC §3.2 +
// ADR-0207 §Context. The pin is load-bearing: external consumers (the
// phase-22.2 lua MIGRATION at Task 10 + the 25.2 wasm consumer at
// Task 13) read these constants as part of their adapter logic.
func TestFilterStateObject_StateType_ConstValuesPinned(t *testing.T) {
	if StateTypeReadOnly != 0 {
		t.Errorf("StateTypeReadOnly = %d; want 0", StateTypeReadOnly)
	}
	if StateTypeMutable != 1 {
		t.Errorf("StateTypeMutable = %d; want 1", StateTypeMutable)
	}
}

// TestFilterStateObject_StateType_DiscriminatorConsistency verifies
// StateType() returns the same value across multiple calls on the same
// object (no internal mutation; pure accessor).
func TestFilterStateObject_StateType_DiscriminatorConsistency(t *testing.T) {
	cases := []struct {
		name string
		obj  *testFSO
		want StateType
	}{
		{"readonly_zero", &testFSO{mutable: false}, StateTypeReadOnly},
		{"mutable_one", &testFSO{mutable: true}, StateTypeMutable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first := tc.obj.StateType()
			second := tc.obj.StateType()
			if first != tc.want {
				t.Errorf("StateType() first call = %d; want %d", first, tc.want)
			}
			if second != tc.want {
				t.Errorf("StateType() second call = %d; want %d", second, tc.want)
			}
			if first != second {
				t.Errorf("StateType() inconsistent across calls: first=%d second=%d", first, second)
			}
		})
	}
}

// TestFilterStateObject_Marshal_DefensiveCopy verifies Marshal returns a
// fresh byte slice (consumer mutation of the returned slice does NOT
// mutate the FilterStateObject's internal buffer). Documents the
// contract at the test layer; production wasm consumer relies on this
// to pass the bytes through guest-memory write helpers without worry.
func TestFilterStateObject_Marshal_DefensiveCopy(t *testing.T) {
	obj := &testFSO{data: []byte("hello")}
	out, err := obj.Marshal()
	if err != nil {
		t.Fatalf("Marshal() err = %v; want nil", err)
	}
	if !bytes.Equal(out, []byte("hello")) {
		t.Errorf("Marshal() = %q; want %q", out, "hello")
	}
	// Mutate the returned slice; verify the underlying obj.data is
	// unaffected.
	out[0] = 'X'
	if !bytes.Equal(obj.data, []byte("hello")) {
		t.Errorf("after mutating Marshal() return, obj.data = %q; want %q (defensive copy violated)",
			obj.data, "hello")
	}
}

// TestFilterStateObject_Unmarshal_RoundTrip verifies Marshal/Unmarshal
// round-trip preserves the data payload byte-for-byte.
func TestFilterStateObject_Unmarshal_RoundTrip(t *testing.T) {
	original := &testFSO{data: []byte("hello world")}
	marshaled, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal() err = %v; want nil", err)
	}
	restored := &testFSO{}
	if err := restored.Unmarshal(marshaled); err != nil {
		t.Fatalf("Unmarshal() err = %v; want nil", err)
	}
	if !bytes.Equal(restored.data, original.data) {
		t.Errorf("Unmarshal round-trip: restored = %q; want %q", restored.data, original.data)
	}
}

// TestFilterStateObject_Unmarshal_NilInput verifies Unmarshal of a nil
// slice clears the underlying data (matches the empty-payload semantic).
func TestFilterStateObject_Unmarshal_NilInput(t *testing.T) {
	obj := &testFSO{data: []byte("preexisting")}
	if err := obj.Unmarshal(nil); err != nil {
		t.Fatalf("Unmarshal(nil) err = %v; want nil", err)
	}
	if obj.data != nil {
		t.Errorf("after Unmarshal(nil), obj.data = %q; want nil", obj.data)
	}
	if obj.HasData() {
		t.Errorf("after Unmarshal(nil), HasData() = true; want false")
	}
}

// TestFilterStateObject_Marshal_OnEmpty verifies Marshal on a zero-data
// FilterStateObject returns nil + nil error (matches the empty-payload
// semantic; consumers may distinguish empty from absent via HasData).
func TestFilterStateObject_Marshal_OnEmpty(t *testing.T) {
	obj := &testFSO{}
	out, err := obj.Marshal()
	if err != nil {
		t.Fatalf("Marshal() on empty err = %v; want nil", err)
	}
	if out != nil {
		t.Errorf("Marshal() on empty = %v; want nil", out)
	}
}
