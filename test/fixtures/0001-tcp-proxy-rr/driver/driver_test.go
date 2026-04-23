package driver

import "testing"

func TestAssertDistribution_Exact(t *testing.T) {
	d := rrDriver{}
	if err := d.AssertDistribution([]uint64{3, 3, 3}, []uint64{3, 3, 3}); err != nil {
		t.Fatalf("expected pass, got: %v", err)
	}
}

func TestAssertDistribution_Imbalanced(t *testing.T) {
	d := rrDriver{}
	if err := d.AssertDistribution([]uint64{4, 3, 2}, []uint64{3, 3, 3}); err == nil {
		t.Fatal("expected error on imbalanced reference counts")
	}
	if err := d.AssertDistribution([]uint64{3, 3, 3}, []uint64{4, 3, 2}); err == nil {
		t.Fatal("expected error on imbalanced subject counts")
	}
}

func TestAssertDistribution_AllZero(t *testing.T) {
	d := rrDriver{}
	if err := d.AssertDistribution([]uint64{0, 0, 0}, []uint64{0, 0, 0}); err == nil {
		t.Fatal("expected error on zero counts (sentinel of 'no traffic flowed')")
	}
}

func TestAssertDistribution_WrongLength(t *testing.T) {
	d := rrDriver{}
	if err := d.AssertDistribution([]uint64{3, 3}, []uint64{3, 3, 3}); err == nil {
		t.Fatal("expected error on wrong-length reference counts")
	}
}
