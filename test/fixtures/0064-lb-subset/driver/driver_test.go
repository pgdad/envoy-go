package driver

import "testing"

// The drive() loop routes 3 routes × perRoute (16) = totalReqs (48) requests
// across the 4 backends; /v1 lands in {0,1}, /v2 in {2,3}, /none spreads over
// all 4 (ANY_ENDPOINT). AssertDistribution's aggregate channel therefore sees
// every side sum to 48 with all 4 backends nonzero (the union of the three
// routes). These unit fixtures exercise AssertDistribution's two legs
// (conservation: sum==48; coverage: all 4 nonzero) + the inline membership
// helper assertSubsetMembership (the per-route SET-membership + within-subset
// spread that drive() applies from the embedded body idxs).

// TestAssertDistribution_BothSideOK: a representative aggregate distribution that
// sums to totalReqs (48) with all 4 backends nonzero passes on BOTH sides. /v1
// over {0,1} contributes 8+8, /v2 over {2,3} contributes 8+8, /none spreads its
// 16 over the 4 → e.g. {12,12,12,12}. The reference is held to the SAME invariant
// (the metadata_match key is static config → NAT-transparent).
func TestAssertDistribution_BothSideOK(t *testing.T) {
	d := subsetDriver{}
	if err := d.AssertDistribution([]uint64{12, 12, 12, 12}, []uint64{14, 10, 11, 13}); err != nil {
		t.Fatalf("expected pass for conserving full-coverage distributions on both sides, got: %v", err)
	}
}

// TestAssertDistribution_Conservation_Reference: a REFERENCE distribution that does
// NOT sum to totalReqs (48) fails the conservation leg. The subject is valid, so
// the asserted break is the reference's under-sum.
func TestAssertDistribution_Conservation_Reference(t *testing.T) {
	d := subsetDriver{}
	// reference sums to 44 (under by perRoute/4) — conservation bites.
	if err := d.AssertDistribution([]uint64{11, 11, 11, 11}, []uint64{12, 12, 12, 12}); err == nil {
		t.Fatal("expected conservation failure on a sub-48 reference sum (44)")
	}
}

// TestAssertDistribution_Conservation_Subject: a SUBJECT distribution that does NOT
// sum to totalReqs (48) fails the conservation leg. The reference is valid.
func TestAssertDistribution_Conservation_Subject(t *testing.T) {
	d := subsetDriver{}
	// subject sums to 52 (over by 4) — conservation bites.
	if err := d.AssertDistribution([]uint64{12, 12, 12, 12}, []uint64{13, 13, 13, 13}); err == nil {
		t.Fatal("expected conservation failure on a >48 subject sum (52)")
	}
}

// TestAssertDistribution_Coverage: a distribution that conserves (sums to 48) but
// leaves a backend at zero fails the coverage leg (the routed union /v1∪/v2 must
// touch every endpoint). Checked on the subject; the reference is valid.
func TestAssertDistribution_Coverage(t *testing.T) {
	d := subsetDriver{}
	// subject {16,16,16,0}: sums to 48 but backend[3] never served — coverage bites
	// (e.g. a v2 subset that collapsed onto idx 2 only).
	if err := d.AssertDistribution([]uint64{12, 12, 12, 12}, []uint64{16, 16, 16, 0}); err == nil {
		t.Fatal("expected coverage failure on a zero-backend subject distribution")
	}
}

// TestAssertDistribution_WrongLength: a non-4 count slice fails.
func TestAssertDistribution_WrongLength(t *testing.T) {
	d := subsetDriver{}
	if err := d.AssertDistribution([]uint64{12, 12, 12}, []uint64{12, 12, 12, 12}); err == nil {
		t.Fatal("expected error on wrong-length reference counts")
	}
}

// TestAssertSubsetMembership_OK: a route whose hit set is EXACTLY the subset
// (both members hit, no leak) passes — the affinity + within-subset spread legs.
func TestAssertSubsetMembership_OK(t *testing.T) {
	if err := assertSubsetMembership("/v1", map[int]bool{0: true, 1: true}, v1Set); err != nil {
		t.Fatalf("expected pass for exact-subset hit set, got: %v", err)
	}
	if err := assertSubsetMembership("/v2", map[int]bool{2: true, 3: true}, v2Set); err != nil {
		t.Fatalf("expected pass for exact-subset hit set, got: %v", err)
	}
}

// TestAssertSubsetMembership_Leak: a hit set containing a backend OUTSIDE the
// subset fails the affinity leg (the subset boundary was breached — a /v1 request
// landed on a v2 host).
func TestAssertSubsetMembership_Leak(t *testing.T) {
	// /v1 served by backend[2] (a v2 host) → leak.
	if err := assertSubsetMembership("/v1", map[int]bool{0: true, 1: true, 2: true}, v1Set); err == nil {
		t.Fatal("expected affinity LEAK failure when a /v1 request lands on a v2 host")
	}
}

// TestAssertSubsetMembership_NoSpread: a hit set MISSING a subset member fails the
// within-subset spread leg (ROUND_ROBIN did not alternate across the 2-host subset).
func TestAssertSubsetMembership_NoSpread(t *testing.T) {
	// /v1 only ever hit backend[0] — member backend[1] never served.
	if err := assertSubsetMembership("/v1", map[int]bool{0: true}, v1Set); err == nil {
		t.Fatal("expected within-subset spread failure when only one subset member is hit")
	}
}
