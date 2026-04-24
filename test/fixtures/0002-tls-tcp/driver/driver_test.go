package driver

import "testing"

func TestAssertDistribution_Exact(t *testing.T) {
	d := &tlsDriver{}
	// Both proxies show [3,3,3]/[3,3,3] — happy path.
	if err := d.AssertDistribution(
		[]uint64{3, 3, 3, 3, 3, 3},
		[]uint64{3, 3, 3, 3, 3, 3},
	); err != nil {
		t.Fatalf("expected pass, got: %v", err)
	}
}

func TestAssertDistribution_ImbalancedAlpha(t *testing.T) {
	d := &tlsDriver{}
	// Reference c_alpha imbalanced (4,3,2): should fail with informative message.
	err := d.AssertDistribution(
		[]uint64{4, 3, 2, 3, 3, 3},
		[]uint64{3, 3, 3, 3, 3, 3},
	)
	if err == nil {
		t.Fatal("expected error on imbalanced reference c_alpha counts")
	}
	if msg := err.Error(); msg == "" {
		t.Fatal("error message must not be empty")
	}
}

func TestAssertDistribution_ImbalancedBeta(t *testing.T) {
	d := &tlsDriver{}
	// Subject c_beta imbalanced: should fail.
	err := d.AssertDistribution(
		[]uint64{3, 3, 3, 3, 3, 3},
		[]uint64{3, 3, 3, 4, 3, 2},
	)
	if err == nil {
		t.Fatal("expected error on imbalanced subject c_beta counts")
	}
}

func TestAssertDistribution_AllZero(t *testing.T) {
	d := &tlsDriver{}
	// Zeroed counts are a sentinel for "no traffic flowed" and must fail.
	if err := d.AssertDistribution(
		[]uint64{0, 0, 0, 0, 0, 0},
		[]uint64{0, 0, 0, 0, 0, 0},
	); err == nil {
		t.Fatal("expected error on zero counts (sentinel of 'no traffic flowed')")
	}
}

func TestAssertDistribution_WrongLength(t *testing.T) {
	d := &tlsDriver{}
	// Wrong-length slice (5 instead of 6): must fail.
	if err := d.AssertDistribution(
		[]uint64{3, 3, 3, 3, 3},
		[]uint64{3, 3, 3, 3, 3, 3},
	); err == nil {
		t.Fatal("expected error on wrong-length reference counts")
	}
	if err := d.AssertDistribution(
		[]uint64{3, 3, 3, 3, 3, 3},
		[]uint64{3, 3, 3, 3, 3},
	); err == nil {
		t.Fatal("expected error on wrong-length subject counts")
	}
}
