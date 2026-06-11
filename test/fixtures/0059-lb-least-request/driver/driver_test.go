package driver

import "testing"

// TestAssertDistribution_InBand: a representative cc=10 skewed per-side
// distribution over 64 conns (a real Task-6-observed sorted row {2, 26, 36} —
// c1=2 / c2=26 / c3=36 — and a permuted variant) lands in the band on both sides.
func TestAssertDistribution_InBand(t *testing.T) {
	d := lrDriver{}
	if err := d.AssertDistribution([]uint64{2, 26, 36}, []uint64{36, 2, 26}); err != nil {
		t.Fatalf("expected pass for a skewed 64-conn distribution, got: %v", err)
	}
}

// TestAssertDistribution_RoundRobinStarvationBites: a ~uniform round-robin
// distribution {21,21,22} (the no-op-release break analog, Task 7 leg (ii))
// FAILS the starvation leg (c1 > 12).
func TestAssertDistribution_RoundRobinStarvationBites(t *testing.T) {
	d := lrDriver{}
	if err := d.AssertDistribution([]uint64{21, 21, 22}, []uint64{6, 27, 31}); err == nil {
		t.Fatal("expected starvation failure on a uniform reference distribution")
	}
	if err := d.AssertDistribution([]uint64{6, 27, 31}, []uint64{21, 21, 22}); err == nil {
		t.Fatal("expected starvation failure on a uniform subject distribution")
	}
}

// TestAssertDistribution_InvertedConcentrationBites: an inverted-comparison
// distribution where the burst concentrates on ONE host {62,1,1} (Task 7 leg
// (i)) FAILS the concentration leg (c2 < 16).
func TestAssertDistribution_InvertedConcentrationBites(t *testing.T) {
	d := lrDriver{}
	if err := d.AssertDistribution([]uint64{62, 1, 1}, []uint64{6, 27, 31}); err == nil {
		t.Fatal("expected concentration failure on an inverted reference distribution")
	}
}

// TestAssertDistribution_Conservation: counts that do not sum to 64 fail the
// conservation leg.
func TestAssertDistribution_Conservation(t *testing.T) {
	d := lrDriver{}
	if err := d.AssertDistribution([]uint64{18, 2, 10}, []uint64{6, 27, 31}); err == nil {
		t.Fatal("expected conservation failure on a sub-64 reference sum")
	}
}

// TestAssertDistribution_WrongLength: a non-3 count slice fails.
func TestAssertDistribution_WrongLength(t *testing.T) {
	d := lrDriver{}
	if err := d.AssertDistribution([]uint64{18, 2}, []uint64{6, 27, 31}); err == nil {
		t.Fatal("expected error on wrong-length reference counts")
	}
}
