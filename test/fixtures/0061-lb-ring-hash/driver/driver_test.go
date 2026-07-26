package driver

import "testing"

// TestAssertDistribution_Affinity: a representative subject distribution where each
// per-backend count is a multiple of burstPerIP (16) and >= 2 backends are nonzero
// (the consistent-hash affinity+spread invariant) passes, with the reference held to
// conservation only (single-key pin → all 256 on one backend).
func TestAssertDistribution_Affinity(t *testing.T) {
	d := ringHashDriver{}
	// subject: 16 source IPs over 3 backends, e.g. eight IPs → backend[0] (128), four
	// each → backend[1]/backend[2] (64/64). reference: single-key pin {256,0,0}.
	if err := d.AssertDistribution([]uint64{256, 0, 0}, []uint64{128, 64, 64}); err != nil {
		t.Fatalf("expected pass for an affine subject + conserving reference, got: %v", err)
	}
}

// TestAssertDistribution_ScatterBitesAffinity: a subject count NOT a multiple of 16
// (a source IP split across backends — the scatter break) FAILS the affinity leg.
func TestAssertDistribution_ScatterBitesAffinity(t *testing.T) {
	d := ringHashDriver{}
	// {20, 108, 128}: sum 256 (conserves) but 20 and 108 are not multiples of 16 —
	// one source IP's 16 conns split 4/12 across backend[0] and backend[1].
	if err := d.AssertDistribution([]uint64{256, 0, 0}, []uint64{20, 108, 128}); err == nil {
		t.Fatal("expected affinity failure on a scattered subject distribution")
	}
}

// TestAssertDistribution_CollapseBitesSpread: a subject distribution with all 256 on
// ONE backend (a collapsed ring) conserves AND is a multiple of 16, but only ONE
// backend is nonzero → FAILS the spread leg.
func TestAssertDistribution_CollapseBitesSpread(t *testing.T) {
	d := ringHashDriver{}
	if err := d.AssertDistribution([]uint64{256, 0, 0}, []uint64{256, 0, 0}); err == nil {
		t.Fatal("expected spread failure on a collapsed (single-backend) subject distribution")
	}
}

// TestAssertDistribution_SubjectConservation: a subject distribution of all-multiples
// of 16 that does NOT sum to 256 fails the conservation leg.
func TestAssertDistribution_SubjectConservation(t *testing.T) {
	d := ringHashDriver{}
	if err := d.AssertDistribution([]uint64{256, 0, 0}, []uint64{64, 64, 64}); err == nil {
		t.Fatal("expected conservation failure on a sub-256 subject sum (192)")
	}
}

// TestAssertDistribution_ReferenceConservation: a reference distribution that does
// not sum to 256 fails the reference conservation leg (the only reference check).
func TestAssertDistribution_ReferenceConservation(t *testing.T) {
	d := ringHashDriver{}
	if err := d.AssertDistribution([]uint64{128, 0, 0}, []uint64{128, 64, 64}); err == nil {
		t.Fatal("expected conservation failure on a sub-256 reference sum (128)")
	}
}

// TestAssertDistribution_WrongLength: a non-3 count slice fails.
func TestAssertDistribution_WrongLength(t *testing.T) {
	d := ringHashDriver{}
	if err := d.AssertDistribution([]uint64{256, 0}, []uint64{128, 64, 64}); err == nil {
		t.Fatal("expected error on wrong-length reference counts")
	}
}
