package cluster

import (
	"fmt"
	"math"
	mathrand "math/rand/v2"
	"testing"
)

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

// Pinned parameters for TestRingHash_EphemeralPortRing_KeyCollapseRate (phase 76,
// the 0061-lb-ring-hash spread flake). collapseFixtureK MUST track sourceIPs in
// test/fixtures/0061-lb-ring-hash/driver/driver.go: nothing in the build links the
// two, so the linkage is a REVIEW gate, not a compile-time one.
const (
	collapseTrials   = 2000     // M independent ring draws, shared by BOTH legs
	collapseSeed     = 20260725 // deterministic pseudo-ephemeral-port stream
	collapseBar      = 1e-3     // MEASURED-leg ceiling on the observed collapse rate
	collapseFixtureK = 16       // MUST equal sourceIPs in the 0061 driver
	collapseControlK = 4        // the PRE-phase-76 sourceIPs value

	// CONTROL-leg acceptance band around the analytic 3^(1-4) = 3.70e-2
	// (expected ~74/2000).
	collapseControlLo = 0.015
	collapseControlHi = 0.070

	// Pseudo-ephemeral port draw range: the Linux default
	// net.ipv4.ip_local_port_range, i.e. the space the differential harness's
	// "0.0.0.0:0" backend binds land in (test/differential/runner_test.go).
	collapseEphemeralLo = 32768
	collapseEphemeralHi = 60999
)

// collapseDrawPorts draws 3 DISTINCT pseudo-ephemeral ports from rng, modeling
// one fresh run of the 0061 harness (three TCPEcho backends bound to "0.0.0.0:0").
func collapseDrawPorts(rng *mathrand.Rand) [3]uint32 {
	var ports [3]uint32
	for i := range ports {
		for {
			p := uint32(collapseEphemeralLo + rng.IntN(collapseEphemeralHi-collapseEphemeralLo+1))
			dup := false
			for j := 0; j < i; j++ {
				if ports[j] == p {
					dup = true
					break
				}
			}
			if !dup {
				ports[i] = p
				break
			}
		}
	}
	return ports
}

// collapseSourceKeys returns the ring_hash keys the 0061 driver produces for its
// first k source IPs (127.0.0.2 .. 127.0.0.(1+k)), via the REAL exported producer
// HashSourceIP — which STRIPS THE PORT, so the number of DISTINCT keys is exactly
// the driver's sourceIPs constant however many connections each source IP opens.
func collapseSourceKeys(k int) []uint64 {
	keys := make([]uint64, k)
	for i := range keys {
		keys[i] = HashSourceIP(fmt.Sprintf("127.0.0.%d:40000", 2+i))
	}
	return keys
}

// collapseAllSame reports whether every entry of addrs is the same string — the
// 0061 spread failure mode: all K distinct source-IP keys landing on ONE backend.
func collapseAllSame(addrs []string) bool {
	if len(addrs) == 0 {
		return true
	}
	for _, a := range addrs[1:] {
		if a != addrs[0] {
			return false
		}
	}
	return true
}

// TestRingHash_EphemeralPortRing_KeyCollapseRate MEASURES the per-run probability
// that the 0061-lb-ring-hash fixture's spread assertion (">= 2 backends nonzero")
// collapses.
//
// The 0061 ring is keyed on the backend's "IP:PORT" string (newRingHashWithRNG →
// endpoints[j].Addr()) and the harness binds every backend to "0.0.0.0:0", so the
// ring is a FRESH RANDOM 3-way partition of the hash space on every run. The number
// of DISTINCT hash keys is the driver's sourceIPs constant, because HashSourceIP
// strips the port — burstPerIP connections per source IP all reduce to ONE key. A
// run "collapses" when all K keys fall into one backend's arc: analytically
// 3*(1/3)^K = 3^(1-K), i.e. 3.7e-2 at K=4 and 7.0e-8 at K=16.
//
// TWO legs over the SAME collapseTrials ring draws:
//
//   - CONTROL (K=collapseControlK=4) — an ANTI-VACUITY leg. It asserts the observed
//     rate lands inside a band that only a genuinely re-randomized ring can hit. If
//     the ring stops being redrawn per trial, this leg reports 0 and goes RED, which
//     is the ONLY thing distinguishing "K=16 is safe" from "the harness measured
//     nothing".
//   - MEASURED (K=collapseFixtureK=16) — the result: the rate must sit below
//     collapseBar.
//
// Both legs use t.Errorf (never t.Fatalf) so a CONTROL failure still prints the
// MEASURED number.
func TestRingHash_EphemeralPortRing_KeyCollapseRate(t *testing.T) {
	maxK := collapseFixtureK
	if collapseControlK > maxK {
		maxK = collapseControlK
	}
	keys := collapseSourceKeys(maxK)

	rng := mathrand.New(mathrand.NewPCG(collapseSeed, collapseSeed))
	addrs := make([]string, maxK)
	var controlCollapses, measuredCollapses int

	for trial := 0; trial < collapseTrials; trial++ {
		ports := collapseDrawPorts(rng)
		endpoints := make([]Endpoint, len(ports))
		for i, p := range ports {
			endpoints[i] = Endpoint{Host: "127.0.0.1", Port: p}
		}
		// The REAL ring builder with the 0061 fixture's `ring_hash_lb_config: {}`
		// defaults (1024 / 8388608 / XX_HASH → 342 points per host, 1026 total —
		// the exact gauges the fixture asserts). The injected rng is never
		// consulted: every Pick below passes hasHash=true.
		rh := newRingHashWithRNG(endpoints,
			ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX},
			func() uint64 { return 0 })

		for i, key := range keys {
			ep, _, err := rh.Pick(key, true, SubsetMatch{}, false)
			if err != nil {
				// Harness precondition, not one of the two legs: a Pick error means
				// the ring was built empty and no rate is measurable at all.
				t.Fatalf("trial %d: Pick(keys[%d]): %v", trial, i, err)
			}
			addrs[i] = ep.Addr()
		}
		if collapseAllSame(addrs[:collapseControlK]) {
			controlCollapses++
		}
		if collapseAllSame(addrs[:collapseFixtureK]) {
			measuredCollapses++
		}
	}

	controlRate := float64(controlCollapses) / float64(collapseTrials)
	measuredRate := float64(measuredCollapses) / float64(collapseTrials)
	controlExpect := 3 * math.Pow(1.0/3.0, collapseControlK)
	measuredExpect := 3 * math.Pow(1.0/3.0, collapseFixtureK)

	t.Logf("CONTROL  K=%d: %d/%d collapses, rate=%.5f | analytic 3^(1-K)=%.3e → expected ~%.2f/%d | band [%g, %g]",
		collapseControlK, controlCollapses, collapseTrials, controlRate,
		controlExpect, controlExpect*collapseTrials, collapseTrials,
		collapseControlLo, collapseControlHi)
	t.Logf("MEASURED K=%d: %d/%d collapses, rate=%.5f | analytic 3^(1-K)=%.3e → expected ~%.5f/%d | bar %g",
		collapseFixtureK, measuredCollapses, collapseTrials, measuredRate,
		measuredExpect, measuredExpect*collapseTrials, collapseTrials, collapseBar)

	if controlRate < collapseControlLo || controlRate > collapseControlHi {
		t.Errorf("CONTROL leg K=%d: collapse rate %.5f (%d/%d) OUTSIDE band [%g, %g] "+
			"(analytic 3^(1-K)=%.3e). A rate of 0 HERE means the ring is NO LONGER BEING "+
			"REDRAWN PER TRIAL — the pseudo-ephemeral ports were frozen or the builder was "+
			"stubbed — and in that case the MEASURED leg below is VACUOUS: it reports 0 "+
			"collapses because nothing varies, NOT because K=%d makes collapse improbable.",
			collapseControlK, controlRate, controlCollapses, collapseTrials,
			collapseControlLo, collapseControlHi, controlExpect, collapseFixtureK)
	}

	if measuredRate >= collapseBar {
		t.Errorf("MEASURED leg K=%d: collapse rate %.5f (%d/%d) >= bar %g "+
			"(analytic 3^(1-K)=%.3e). K is the number of DISTINCT ring_hash keys the 0061 "+
			"fixture drives, i.e. the sourceIPs constant in "+
			"test/fixtures/0061-lb-ring-hash/driver/driver.go — if this leg fires, sourceIPs "+
			"SHRANK and the fixture's spread assertion (>= 2 backends nonzero) is flaky again "+
			"at this rate.",
			collapseFixtureK, measuredRate, measuredCollapses, collapseTrials, collapseBar,
			measuredExpect)
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
