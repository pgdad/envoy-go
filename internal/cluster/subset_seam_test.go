package cluster

import (
	"context"
	"testing"
)

func TestSubsetMatchCtx_RoundTrips(t *testing.T) {
	m := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})
	ctx := WithSubsetMatch(context.Background(), m)
	got, ok := subsetMatchFrom(ctx)
	if !ok {
		t.Fatal("subsetMatchFrom must report ok after WithSubsetMatch")
	}
	if got.Key() != m.Key() {
		t.Errorf("round-trip mismatch: %q vs %q", got.Key(), m.Key())
	}
	if _, ok := subsetMatchFrom(context.Background()); ok {
		t.Error("bare ctx must report !ok")
	}
}

func TestLeafPolicies_IgnoreSubsetMatch(t *testing.T) {
	// The widened Pick signature is behavior-neutral for all 5 leaf policies:
	// passing a non-empty match + hasMatch=true changes nothing — only subsetLB
	// consumes (match, hasMatch); the leaves ignore it (a later task wires that up).
	m := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})

	assertIgnores := func(t *testing.T, name string, lb loadBalancer) {
		t.Helper()
		ep, rel, err := lb.Pick(0, false, m, true)
		if err != nil || ep.Port < 1000 { // eps(n) ports start at 1000
			t.Errorf("%s: leaf must ignore the match and return a valid endpoint: ep=%v err=%v", name, ep, err)
		}
		rel()
	}

	// 1. roundRobin — no constructor; built directly.
	assertIgnores(t, "roundRobin", &roundRobin{endpoints: eps(3)})

	// 2. leastRequest — use the RNG-injectable constructor (same minimal values
	//    the per-policy tests use: choiceCount 2, deterministic RNG).
	lr := newLeastRequestWithRNG(eps(3), 2, seqRNG(0, 1))
	assertIgnores(t, "leastRequest", lr)

	// 3. randomLB — use the RNG-injectable constructor.
	r := newRandomWithRNG(eps(3), seqRNG(0))
	assertIgnores(t, "randomLB", r)

	// 4. ringHashLB — use the RNG-injectable constructor with minimal config.
	rh := newRingHashWithRNG(eps(3), ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX}, seqRNG(0))
	assertIgnores(t, "ringHashLB", rh)

	// 5. maglevLB — use the RNG-injectable constructor with minimal config.
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(0))
	assertIgnores(t, "maglevLB", mg)
}
