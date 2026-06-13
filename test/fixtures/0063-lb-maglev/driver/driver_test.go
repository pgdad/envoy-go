package driver

import "testing"

// The drive() loop sends N=hashValues (16) distinct X-Hash values × K=repeatPerVal
// (16) repeats = totalReqs (256) routed requests, so AssertDistribution's
// conservation leg requires every side to sum to 256. These unit fixtures use
// sum-256 multiple-of-16 distributions (e.g. {128,64,64}) as the "good" side so
// each negative test bites its INTENDED leg (affinity / spread / conservation),
// not a stale conservation mismatch. AssertDistribution's check order is:
// length → per-backend affinity (multiple-of-16) → conservation (sum==256) →
// spread (>=2 nonzero); the good side is chosen valid on every earlier leg so the
// asserted break is the one under test.

// TestAssertDistribution_BothSideAffinity: a representative distribution where each
// per-backend count is a multiple of repeatPerVal (16), sums to totalReqs (256), and
// >= 2 backends are nonzero (the consistent-hash affinity+spread+conservation
// invariant) passes on BOTH sides. As in 0062, the reference is ALSO held to the
// affinity invariant (the X-Hash header is NAT-transparent).
func TestAssertDistribution_BothSideAffinity(t *testing.T) {
	d := maglevDriver{}
	// 16 X-Hash values over 3 backends, e.g. eight values → backend[0] (128), four
	// each → backend[1]/backend[2] (64/64). Both sides affine (NAT-transparent header).
	if err := d.AssertDistribution([]uint64{128, 64, 64}, []uint64{64, 128, 64}); err != nil {
		t.Fatalf("expected pass for affine distributions on both sides, got: %v", err)
	}
}

// TestAssertDistribution_ScatterBitesAffinity_Reference: a REFERENCE count NOT a
// multiple of 16 (a value split across backends — the scatter break) FAILS the
// affinity leg. The subject side is a valid sum-256 affine distribution, so the
// asserted break is the reference's scatter, not a conservation miss.
func TestAssertDistribution_ScatterBitesAffinity_Reference(t *testing.T) {
	d := maglevDriver{}
	// reference {116, 124, 16}: sum 256 (conserves) but 116 and 124 are not multiples
	// of 16 → the affinity leg (checked before conservation) bites.
	if err := d.AssertDistribution([]uint64{116, 124, 16}, []uint64{128, 64, 64}); err == nil {
		t.Fatal("expected affinity failure on a scattered reference distribution")
	}
}

// TestAssertDistribution_ScatterBitesAffinity_Subject: a SUBJECT count NOT a multiple
// of 16 FAILS the affinity leg. The reference side is a valid sum-256 affine
// distribution (passes every leg), so the asserted break is the subject's scatter.
func TestAssertDistribution_ScatterBitesAffinity_Subject(t *testing.T) {
	d := maglevDriver{}
	// subject {116, 124, 16}: sum 256 but 116/124 not multiples of 16 → affinity bites.
	if err := d.AssertDistribution([]uint64{128, 64, 64}, []uint64{116, 124, 16}); err == nil {
		t.Fatal("expected affinity failure on a scattered subject distribution")
	}
}

// TestAssertDistribution_CollapseBitesSpread: a distribution with all 256 on ONE
// backend (a collapsed table) conserves AND is a multiple of 16, but only ONE backend
// is nonzero → FAILS the spread leg. Checked on the subject side here; the reference
// is a valid sum-256 affine distribution, so the asserted break is the collapse.
func TestAssertDistribution_CollapseBitesSpread(t *testing.T) {
	d := maglevDriver{}
	if err := d.AssertDistribution([]uint64{128, 64, 64}, []uint64{256, 0, 0}); err == nil {
		t.Fatal("expected spread failure on a collapsed (single-backend) subject distribution")
	}
}

// TestAssertDistribution_Conservation: a subject distribution of all-multiples-of-16
// that does NOT sum to totalReqs (256) fails the conservation leg. The reference is a
// valid sum-256 affine distribution; the subject {64,64,64}=192 is affine + spread but
// under-sums → the conservation leg (checked after affinity) bites.
func TestAssertDistribution_Conservation(t *testing.T) {
	d := maglevDriver{}
	if err := d.AssertDistribution([]uint64{128, 64, 64}, []uint64{64, 64, 64}); err == nil {
		t.Fatal("expected conservation failure on a sub-256 subject sum (192)")
	}
}

// TestAssertDistribution_WrongLength: a non-3 count slice fails.
func TestAssertDistribution_WrongLength(t *testing.T) {
	d := maglevDriver{}
	if err := d.AssertDistribution([]uint64{128, 64}, []uint64{128, 64, 64}); err == nil {
		t.Fatal("expected error on wrong-length reference counts")
	}
}
