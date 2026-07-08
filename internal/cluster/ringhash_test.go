package cluster

import "testing"

func TestRingHash_SameKeySameEndpoint(t *testing.T) {
	rh, err := newRingHash(eps(3), ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX})
	if err != nil {
		t.Fatal(err)
	}
	ep1, _, _ := rh.Pick(0xABCDEF, true, SubsetMatch{}, false)
	ep2, _, _ := rh.Pick(0xABCDEF, true, SubsetMatch{}, false)
	if ep1.Addr() != ep2.Addr() {
		t.Errorf("same key picked different endpoints: %v vs %v", ep1, ep2)
	}
}

// TestRingHash_HealthAware_WalkToNextHealthy proves the ADR-0243 ring walk:
// when the host a fixed key resolves to is marked unhealthy, the SAME key now
// resolves to a different, healthy host (the next healthy point forward on the
// ring), rather than the unhealthy primary.
func TestRingHash_HealthAware_WalkToNextHealthy(t *testing.T) {
	e := eps(3)
	rh, err := newRingHash(e, ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX})
	if err != nil {
		t.Fatal(err)
	}
	const key = uint64(0xABCDEF)

	// Baseline: the key's primary host (no health -> fast path).
	primary, _, _ := rh.Pick(key, true, SubsetMatch{}, false)

	// Attach a health registry and mark the primary unhealthy. 2/3 healthy = 66%
	// > 50% -> NOT in panic; the ring walk skips the unhealthy primary.
	ch := newClusterHealth(e, 50)
	rh.health = ch
	ch.states[primary.Addr()].healthy.Store(false)

	got, _, err := rh.Pick(key, true, SubsetMatch{}, false)
	if err != nil {
		t.Fatalf("health-aware pick: %v", err)
	}
	if got.Addr() == primary.Addr() {
		t.Fatalf("ring returned the unhealthy primary %q; expected a walk to the next healthy host", primary.Addr())
	}
	if !ch.isHealthy(got) {
		t.Fatalf("ring returned an unhealthy host %q", got.Addr())
	}
}

func TestRingHash_DistinctKeysSpread(t *testing.T) {
	rh, _ := newRingHash(eps(3), ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX})
	seen := map[string]bool{}
	// Spread distinct keys across the FULL uint64 ring space: the 64-bit
	// golden-ratio mixing constant. (The 32-bit Knuth constant maxes out
	// at ~5e11 — below every ring point ~1.4e16 — so all keys would collapse
	// onto ring[0], which is correct LB behavior but a degenerate test.)
	for k := uint64(0); k < 200; k++ {
		ep, _, _ := rh.Pick(k*0x9E3779B97F4A7C15, true, SubsetMatch{}, false)
		seen[ep.Addr()] = true
	}
	if len(seen) < 2 {
		t.Errorf("200 distinct keys covered only %d endpoints (degenerate ring?)", len(seen))
	}
}

func TestRingHash_WrapAround(t *testing.T) {
	rh, _ := newRingHash(eps(3), ringHashCfg{minRingSize: 12, maxRingSize: 8388608, hashFunc: hashXX})
	epWrap, _, _ := rh.Pick(^uint64(0), true, SubsetMatch{}, false)
	epZero := rh.endpoints[rh.ring[0].ep]
	if epWrap.Addr() != epZero.Addr() {
		t.Errorf("wrap: key=MaxUint64 picked %v, want ring[0] endpoint %v", epWrap, epZero)
	}
}

func TestRingHash_NoHashFallbackUsesRNG(t *testing.T) {
	rh := newRingHashWithRNG(eps(3), ringHashCfg{minRingSize: 12, maxRingSize: 8388608, hashFunc: hashXX}, seqRNG(0))
	epA, _, errA := rh.Pick(0, false, SubsetMatch{}, false) // rng()=0 → ring[0] (first point >= 0)
	if errA != nil {
		t.Fatal(errA)
	}
	if epA.Addr() != rh.endpoints[rh.ring[0].ep].Addr() {
		t.Errorf("no-hash fallback with rng()=0 picked %v, want ring[0]", epA)
	}
}

func TestRingHash_EmptySet(t *testing.T) {
	rh := newRingHashWithRNG(nil, ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX}, seqRNG(0))
	_, release, err := rh.Pick(123, true, SubsetMatch{}, false)
	if err != errNoEndpoints {
		t.Errorf("err = %v, want errNoEndpoints", err)
	}
	if release == nil {
		t.Error("release must be non-nil even on error (interface contract)")
	}
}

func TestRingHash_DefaultBuildMatchesReference(t *testing.T) {
	// D-RH4b / D-S36-5: default minimum_ring_size=1024 over 3 equal hosts builds
	// size=3*ceil(1024/3)=3*342=1026, min=max=342 (the live reference values).
	rh, _ := newRingHash(eps(3), ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX})
	if rh.size != 1026 || rh.minPerHost != 342 || rh.maxPerHost != 342 {
		t.Errorf("gauges = {size:%d min:%d max:%d}, want {1026 342 342}", rh.size, rh.minPerHost, rh.maxPerHost)
	}
	if uint64(len(rh.ring)) != rh.size {
		t.Errorf("len(ring)=%d != size gauge %d", len(rh.ring), rh.size)
	}
	for i := 1; i < len(rh.ring); i++ {
		if rh.ring[i].hash < rh.ring[i-1].hash {
			t.Fatalf("ring not sorted ascending at %d", i)
		}
	}
}

func TestRingHash_RandomKeysNeverPanicAlwaysValid(t *testing.T) {
	rh, _ := newRingHash(eps(3), ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX})
	rng := seqRNG(1, 2, 3, 1<<63, ^uint64(0), 0) // cycles
	for i := 0; i < 1000; i++ {
		ep, rel, err := rh.Pick(rng(), true, SubsetMatch{}, false)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Port < 1000 { // eps(n) ports start at 1000
			t.Fatalf("pick %d: invalid endpoint %v", i, ep)
		}
		rel()
	}
}

func TestRingHash_MurmurArmBuilds(t *testing.T) {
	rh, err := newRingHash(eps(3), ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashMurmur})
	if err != nil || rh.size != 1026 {
		t.Errorf("murmur arm: err=%v size=%d (want 1026)", err, rh.size)
	}
}
