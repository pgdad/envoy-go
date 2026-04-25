package driver

import "testing"

func TestAssertDistribution_Happy(t *testing.T) {
	d := httpDriver{}
	if err := d.AssertDistribution([]uint64{3, 3, 3}, []uint64{3, 3, 3}); err != nil {
		t.Errorf("happy: %v", err)
	}
}

func TestAssertDistribution_SkewFails(t *testing.T) {
	d := httpDriver{}
	if err := d.AssertDistribution([]uint64{3, 3, 3}, []uint64{4, 3, 2}); err == nil {
		t.Error("expected error on [4,3,2], got nil")
	}
}

func TestAssertDistribution_WrongLengthFails(t *testing.T) {
	d := httpDriver{}
	if err := d.AssertDistribution([]uint64{3, 3, 3}, []uint64{3, 3, 3, 0, 0, 0}); err == nil {
		t.Error("expected error on length mismatch, got nil")
	}
}
