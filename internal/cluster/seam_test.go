package cluster

import (
	"context"
	"testing"
)

func TestWithHashKey_RoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, ok := hashKeyFrom(ctx); ok {
		t.Fatal("bare ctx must report hasHash==false")
	}
	ctx = WithHashKey(ctx, 0xDEADBEEF)
	v, ok := hashKeyFrom(ctx)
	if !ok || v != 0xDEADBEEF {
		t.Errorf("hashKeyFrom = (%#x, %v), want (0xDEADBEEF, true)", v, ok)
	}
}

func TestIncumbentPolicies_IgnoreHashParams(t *testing.T) {
	// roundRobin is deterministic: the same 4-pick sequence must result whether
	// hasHash is false or an arbitrary key+true (behavior-neutrality in miniature).
	mk := func() loadBalancer { return &roundRobin{endpoints: eps(3)} }
	pick4 := func(lb loadBalancer, key uint64, has bool) []Endpoint {
		var out []Endpoint
		for i := 0; i < 4; i++ {
			ep, rel, err := lb.Pick(key, has, SubsetMatch{}, false)
			if err != nil {
				t.Fatal(err)
			}
			rel()
			out = append(out, ep)
		}
		return out
	}
	a := pick4(mk(), 0, false)
	b := pick4(mk(), 0x12345, true)
	for i := range a {
		if a[i].Addr() != b[i].Addr() {
			t.Errorf("roundRobin pick %d changed with hash args: %v vs %v", i, a[i], b[i])
		}
	}
}
