package driver

import "testing"

// TestAssertDistribution_InBand: a representative ~uniform per-side distribution
// over 64 conns (a real Task-5-observed row {19, 22, 23} and a permuted variant)
// lands in the anti-skew band on both sides.
func TestAssertDistribution_InBand(t *testing.T) {
	d := randDriver{}
	if err := d.AssertDistribution([]uint64{19, 22, 23}, []uint64{23, 19, 22}); err != nil {
		t.Fatalf("expected pass for a uniform 64-conn distribution, got: %v", err)
	}
}

// TestAssertDistribution_FloorBitesOnSkew: a load-skewing (least_request-shaped)
// distribution {2, 26, 36} — the loaded backend starved at c1=2 — FAILS the
// uniform-floor leg (2 < 6). This is the CONTRAST with 0059: a skew that 0059
// REQUIRES is REJECTED here.
func TestAssertDistribution_FloorBitesOnSkew(t *testing.T) {
	d := randDriver{}
	if err := d.AssertDistribution([]uint64{2, 26, 36}, []uint64{19, 22, 23}); err == nil {
		t.Fatal("expected uniform-floor failure on a load-skewed reference distribution")
	}
	if err := d.AssertDistribution([]uint64{19, 22, 23}, []uint64{2, 26, 36}); err == nil {
		t.Fatal("expected uniform-floor failure on a load-skewed subject distribution")
	}
}

// TestAssertDistribution_CeilingBitesOnPin: a single-host-pin distribution
// {62, 1, 1} FAILS the uniform-ceiling leg (62 > 40). (For backend[0]=62 the loop
// checks the floor first — backend[0]=62 passes the floor but trips the ceiling.)
func TestAssertDistribution_CeilingBitesOnPin(t *testing.T) {
	d := randDriver{}
	if err := d.AssertDistribution([]uint64{62, 1, 1}, []uint64{19, 22, 23}); err == nil {
		t.Fatal("expected uniform-ceiling/floor failure on a single-host-pin reference distribution")
	}
}

// TestAssertDistribution_Conservation: counts that DO satisfy the band but do not
// sum to 64 fail the conservation leg (each in [6,40], sum 48 != 64).
func TestAssertDistribution_Conservation(t *testing.T) {
	d := randDriver{}
	if err := d.AssertDistribution([]uint64{16, 16, 16}, []uint64{19, 22, 23}); err == nil {
		t.Fatal("expected conservation failure on a sub-64 reference sum")
	}
}

// TestAssertDistribution_WrongLength: a non-3 count slice fails.
func TestAssertDistribution_WrongLength(t *testing.T) {
	d := randDriver{}
	if err := d.AssertDistribution([]uint64{18, 22}, []uint64{19, 22, 23}); err == nil {
		t.Fatal("expected error on wrong-length reference counts")
	}
}
