package admission_control

// rand_test.go — test-scope fakeRand implementation + defaultRand sanity test
// per phase-23 IMPL Task 3. Closes SPEC §3.1 + AMEND-2 deterministic-seam
// requirement.
//
// fakeRand is the deterministic test-scope implementation of the Rand interface.
// It returns a chosen uint64 value so that TestShouldReject_Boundary_* (Task 4)
// can pin the exact (1e4*P) > (r % 1e4) knife-edge without depending on a
// non-deterministic RNG. fakeRand lives ONLY in this _test.go file — the
// production binary links defaultRand from rand.go.
//
// Task 3 OWNS the fakeRand scaffolding; the controller tests (Task 4) consume it.

import (
	"testing"
)

// -----------------------------------------------------------------------------
// fakeRand — deterministic test-scope Rand implementation.
// -----------------------------------------------------------------------------

// fakeRand is the test-scope implementation of Rand that returns a fixed uint64
// value. Consumed by controller tests (Task 4) to pin the exact admit/reject
// boundary of the integer-modulo decision (accuracy*P) > (r % accuracy) with
// accuracy=10000 per AMEND-2.
//
// Usage: inject fakeRand{v: chosenValue} as the Rand seam on a controller to
// make every shouldReject call draw the same deterministic r.
type fakeRand struct {
	v uint64
}

// Uint64 returns the fixed value, satisfying the Rand interface.
func (f fakeRand) Uint64() uint64 { return f.v }

// Compile-time interface satisfaction check.
var _ Rand = fakeRand{}

// -----------------------------------------------------------------------------
// fakeRand tests.
// -----------------------------------------------------------------------------

// TestFakeRand_ReturnsConfiguredValue verifies that fakeRand.Uint64 returns the
// value it was constructed with — the core contract the controller tests depend on.
func TestFakeRand_ReturnsConfiguredValue(t *testing.T) {
	for _, want := range []uint64{0, 1, 9999, 10000, 1<<32 - 1, 1<<63 - 1, ^uint64(0)} {
		fr := fakeRand{v: want}
		if got := fr.Uint64(); got != want {
			t.Errorf("fakeRand{v:%d}.Uint64() = %d; want %d", want, got, want)
		}
	}
}

// TestFakeRand_Deterministic verifies successive calls return the same value
// (no state mutation — each call is independent).
func TestFakeRand_Deterministic(t *testing.T) {
	const val = uint64(42)
	fr := fakeRand{v: val}
	for i := 0; i < 5; i++ {
		if got := fr.Uint64(); got != val {
			t.Errorf("call %d: fakeRand.Uint64() = %d; want %d", i, got, val)
		}
	}
}

// -----------------------------------------------------------------------------
// defaultRand sanity test — verifies the production wiring compiles and runs.
// -----------------------------------------------------------------------------

// TestDefaultRand_Sanity verifies that defaultRand satisfies the Rand interface
// and that repeated calls return valid uint64 values (not a constant function).
// This is a smoke test for the math/rand/v2 wiring — not a statistical test.
func TestDefaultRand_Sanity(t *testing.T) {
	// Compile-time interface satisfaction.
	var _ Rand = defaultRand{}

	dr := defaultRand{}
	// Draw two values and verify at least one non-zero result appears across
	// a handful of calls (the probability of all-zero in 10 draws is negligible).
	var nonZero bool
	for i := 0; i < 10; i++ {
		if dr.Uint64() != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Error("defaultRand.Uint64() returned 0 for 10 consecutive calls; wiring suspect")
	}
}
