package cluster

import "testing"

// trackingFactory returns a leafFactory that builds ONE stubLB per distinct
// call, recording each call's endpoint sub-slice (by pointer-stable Port sum,
// a cheap fingerprint) so tests can assert what newLocalityWeightedLB built.
type factoryCall struct {
	n   int
	sum uint32 // sum of Port across the sub-slice — a cheap "which slice" fingerprint
}

func trackingFactory() (leafFactory, *[]factoryCall) {
	var calls []factoryCall
	f := func(sub []Endpoint) (loadBalancer, error) {
		var sum uint32
		for _, ep := range sub {
			sum += ep.Port
		}
		calls = append(calls, factoryCall{n: len(sub), sum: sum})
		return &stubLB{ep: Endpoint{Host: "child", Port: sum}}, nil
	}
	return f, &calls
}

func TestNewLocalityWeightedLB_GroupsByLocality(t *testing.T) {
	epsA := []Endpoint{
		{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 2},
		{Host: "a1", Port: 2, Locality: LocalityID{Region: "a"}, LocalityWeight: 2},
	}
	epsB := []Endpoint{{Host: "b0", Port: 3, Locality: LocalityID{Region: "b"}, LocalityWeight: 1}}
	all := append(append([]Endpoint{}, epsA...), epsB...)
	factory, calls := trackingFactory()
	lw, err := newLocalityWeightedLBWithRNG(all, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if len(lw.groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(lw.groups))
	}
	byRegion := map[string]localityGroup{}
	for _, g := range lw.groups {
		byRegion[g.id.Region] = g
	}
	if g, ok := byRegion["a"]; !ok || len(g.endpoints) != 2 || g.weight != 2 {
		t.Errorf("region a group = %+v, want 2 endpoints weight 2", g)
	}
	if g, ok := byRegion["b"]; !ok || len(g.endpoints) != 1 || g.weight != 1 {
		t.Errorf("region b group = %+v, want 1 endpoint weight 1", g)
	}
	// factory called 3 times: region a (2 eps), region b (1 ep), flat (3 eps).
	if len(*calls) != 3 {
		t.Fatalf("factory calls = %d, want 3 (2 groups + 1 flat)", len(*calls))
	}
	var sawFlat bool
	for _, c := range *calls {
		if c.n == 3 {
			sawFlat = true
		}
	}
	if !sawFlat {
		t.Errorf("no factory call spanned all 3 endpoints (the flat fallback) — calls: %+v", *calls)
	}
}

func TestNewLocalityWeightedLB_DuplicateLocality_LastWriteWins(t *testing.T) {
	// D-LW-DUP: two endpoints sharing an identical LocalityID but DIFFERENT
	// LocalityWeight (simulating two source LocalityLbEndpoints groups that
	// collapsed to the same identity) — the LAST-encountered weight wins, and
	// BOTH endpoints merge into the SAME group (grouping is by identity, not
	// by source-group index).
	eps := []Endpoint{
		{Host: "h1", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 5},
		{Host: "h2", Port: 2, Locality: LocalityID{Region: "a"}, LocalityWeight: 9},
	}
	factory, _ := trackingFactory()
	lw, err := newLocalityWeightedLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if len(lw.groups) != 1 {
		t.Fatalf("groups = %d, want 1 (both endpoints share LocalityID{Region:a})", len(lw.groups))
	}
	if lw.groups[0].weight != 9 {
		t.Errorf("weight = %d, want 9 (last-write-wins)", lw.groups[0].weight)
	}
	if len(lw.groups[0].endpoints) != 2 {
		t.Errorf("endpoints = %d, want 2 (both merge into the same group)", len(lw.groups[0].endpoints))
	}
}

func TestNewLocalityWeightedLB_FactoryErrorPropagates(t *testing.T) {
	eps := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}}}
	wantErr := errNoEndpoints
	factory := func(sub []Endpoint) (loadBalancer, error) { return nil, wantErr }
	if _, err := newLocalityWeightedLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 }); err != wantErr {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestNewLocalityWeightedLB_OverprovisioningFactor_DefaultsOnAbsent(t *testing.T) {
	factory, _ := trackingFactory()
	lw, err := newLocalityWeightedLBWithRNG([]Endpoint{{Host: "a", Port: 1}}, nil, 0, false /* hasOPF */, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if lw.overprovisioningFactor != defaultOverprovisioningFactor {
		t.Errorf("overprovisioningFactor = %d, want %d (absent → default)", lw.overprovisioningFactor, defaultOverprovisioningFactor)
	}
}

func TestNewLocalityWeightedLB_OverprovisioningFactor_HonorsExplicitZero(t *testing.T) {
	// D-LW-OPF0: an EXPLICIT {value: 0} is honored literally, NOT defaulted.
	factory, _ := trackingFactory()
	lw, err := newLocalityWeightedLBWithRNG([]Endpoint{{Host: "a", Port: 1}}, nil, 0, true /* hasOPF */, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if lw.overprovisioningFactor != 0 {
		t.Errorf("overprovisioningFactor = %d, want 0 (explicit zero honored, not defaulted)", lw.overprovisioningFactor)
	}
}

func TestPick_HealthyLocality_DelegatesToItsOwnChild(t *testing.T) {
	epsA := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 1}}
	epsB := []Endpoint{{Host: "b0", Port: 2, Locality: LocalityID{Region: "b"}, LocalityWeight: 0}} // zero weight → never drawn
	all := append(append([]Endpoint{}, epsA...), epsB...)
	stubs := map[string]*stubLB{}
	factory := func(sub []Endpoint) (loadBalancer, error) {
		key := "flat"
		if len(sub) != len(all) {
			key = sub[0].Locality.Region
		}
		s := &stubLB{}
		stubs[key] = s
		return s, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(all, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := lw.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if stubs["a"].active.Load() != 1 {
		t.Errorf("region a (the only nonzero-weight locality) must have been picked; active = %d", stubs["a"].active.Load())
	}
	if stubs["b"].active.Load() != 0 {
		t.Errorf("region b (weight 0) must NEVER be picked; active = %d", stubs["b"].active.Load())
	}
	if stubs["flat"].active.Load() != 0 {
		t.Errorf("flat fallback must not fire when a nonzero-weight locality exists; active = %d", stubs["flat"].active.Load())
	}
}

func TestPick_PanicBypassesLocalityWeighting(t *testing.T) {
	epsA := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 2}}
	epsB := []Endpoint{{Host: "b0", Port: 2, Locality: LocalityID{Region: "b"}, LocalityWeight: 1}}
	all := append(append([]Endpoint{}, epsA...), epsB...)
	health := newClusterHealth(all, 0.5)
	health.states["a0:1"].healthy.Store(false) // 1/2 = 50%, NOT strictly < 0.5 yet...
	health.states["b0:2"].healthy.Store(false) // ...now 0/2 = 0% < 0.5 → cluster-wide panic
	stubs := map[string]*stubLB{}
	factory := func(sub []Endpoint) (loadBalancer, error) {
		key := "flat"
		if len(sub) != len(all) {
			key = sub[0].Locality.Region
		}
		s := &stubLB{}
		stubs[key] = s
		return s, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := lw.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if stubs["flat"].active.Load() != 1 {
		t.Errorf("cluster-wide panic (0%% healthy) must delegate to flat; flat active = %d", stubs["flat"].active.Load())
	}
	if stubs["a"].active.Load() != 0 || stubs["b"].active.Load() != 0 {
		t.Errorf("panic must bypass per-locality children entirely: a=%d b=%d", stubs["a"].active.Load(), stubs["b"].active.Load())
	}
	if got := health.panicCounter; got != nil {
		t.Errorf("panicCounter is nil-guarded in this unit construction — unexpected non-nil")
	}
}

func TestPick_ZeroTotalEffectiveWeight_FallsBackToFlat(t *testing.T) {
	// AMEND-LW2: every locality's raw weight is 0 (the "all omitted" outcome).
	epsA := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 0}}
	epsB := []Endpoint{{Host: "b0", Port: 2, Locality: LocalityID{Region: "b"}, LocalityWeight: 0}}
	all := append(append([]Endpoint{}, epsA...), epsB...)
	stubs := map[string]*stubLB{}
	factory := func(sub []Endpoint) (loadBalancer, error) {
		key := "flat"
		if len(sub) != len(all) {
			key = sub[0].Locality.Region
		}
		s := &stubLB{}
		stubs[key] = s
		return s, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(all, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, _, err := lw.Pick(0, false, SubsetMatch{}, false); err != nil {
			t.Fatal(err)
		}
	}
	if stubs["flat"].active.Load() != 10 {
		t.Errorf("all-zero-weight → EVERY Pick must fall back to flat; flat active = %d", stubs["flat"].active.Load())
	}
}

func TestPick_ForwardsHashKeyAndMatchUnchanged(t *testing.T) {
	// D-LW7 / SPEC §3.2: hashKey/match pass straight through to the chosen child.
	eps := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 1}}
	var gotHashKey uint64
	var gotHasHash bool
	var gotMatch SubsetMatch
	var gotHasMatch bool
	factory := func(sub []Endpoint) (loadBalancer, error) {
		return &argRecordingLB{onPick: func(hk uint64, hh bool, m SubsetMatch, hm bool) {
			gotHashKey, gotHasHash, gotMatch, gotHasMatch = hk, hh, m, hm
		}}, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	wantMatch := NewSubsetMatch(map[string]SubsetValue{"k": {Kind: subsetString, Str: "v"}})
	if _, _, err := lw.Pick(42, true, wantMatch, true); err != nil {
		t.Fatal(err)
	}
	if gotHashKey != 42 || !gotHasHash || gotMatch.Key() != wantMatch.Key() || !gotHasMatch {
		t.Errorf("forwarded (hashKey=%d, hasHash=%v, match=%v, hasMatch=%v), want (42, true, %v, true)", gotHashKey, gotHasHash, gotMatch, gotHasMatch, wantMatch)
	}
}

// argRecordingLB is a test-only loadBalancer that records its Pick args via a
// caller-supplied callback and returns a fixed zero Endpoint. (Named distinctly
// from h2pool_test.go's pre-existing recordingLB — same package, different
// shape — to avoid a redeclaration collision; the PLAN's sketch used the name
// recordingLB, which was not available.)
type argRecordingLB struct {
	onPick func(uint64, bool, SubsetMatch, bool)
}

func (r *argRecordingLB) Pick(hk uint64, hh bool, m SubsetMatch, hm bool) (Endpoint, func(), error) {
	if r.onPick != nil {
		r.onPick(hk, hh, m, hm)
	}
	return Endpoint{}, noopRelease, nil
}

func TestEffectiveWeight_ZeroWeightIsAlwaysZero(t *testing.T) {
	if got := effectiveWeight(0, 1.0, 140); got != 0 {
		t.Errorf("effectiveWeight(0, ...) = %v, want 0", got)
	}
}

func TestEffectiveWeight_FullHealthNoPlateau_IsRawWeight(t *testing.T) {
	// At 100% healthy with the default OPF=140, availability caps at 1.0 —
	// effective weight equals the raw configured weight exactly.
	if got := effectiveWeight(3, 1.0, 140); got != 3 {
		t.Errorf("effectiveWeight(3, 1.0, 140) = %v, want 3", got)
	}
}

func TestEffectiveWeight_ConfirmedDegradationCurve(t *testing.T) {
	// SPEC §11.3 AMEND-LW3: region A weight=2, region B weight=1 (always 100%
	// healthy), OPF=140 (the reference default). Each row: A's healthy
	// fraction → A's predicted share of the 2-locality draw, matching the
	// live-probed values within the SPEC's own reported <1-percentage-point
	// tolerance.
	const weightA, weightB, opf = 2, 1, 140
	cases := []struct {
		fracA     float64
		wantShare float64
	}{
		{1.00, 0.667},
		{0.80, 0.667}, // ABOVE the 100/140=71.4% plateau threshold — no degradation
		{0.60, 0.627},
		{0.40, 0.528},
		{0.20, 0.359},
		{0.00, 0.000},
	}
	for _, c := range cases {
		effA := effectiveWeight(weightA, c.fracA, opf)
		effB := effectiveWeight(weightB, 1.0, opf)
		total := effA + effB
		var share float64
		if total > 0 {
			share = effA / total
		}
		if diff := share - c.wantShare; diff > 0.01 || diff < -0.01 {
			t.Errorf("fracA=%.2f: share = %.4f, want %.3f (±0.01)", c.fracA, share, c.wantShare)
		}
	}
}

func TestEffectiveWeight_OPF100vs140_AB(t *testing.T) {
	// SPEC §11.3: an identical 60%-healthy state, OPF=140 (default) vs
	// OPF=100 (no plateau margin), must produce CLEARLY DIFFERENT shares —
	// proving the factor is genuinely consumed, not hardcoded.
	const weightA, weightB, frac = 2, 1, 0.60
	share := func(opf uint32) float64 {
		effA := effectiveWeight(weightA, frac, opf)
		effB := effectiveWeight(weightB, 1.0, opf)
		return effA / (effA + effB)
	}
	share140, share100 := share(140), share(100)
	if diff := share140 - share100; diff < 0.05 {
		t.Errorf("share(OPF=140)=%.4f vs share(OPF=100)=%.4f: difference %.4f too small, want a CLEARLY different result (SPEC observed 62.8%% vs 54.0%%)", share140, share100, diff)
	}
	if d := share140 - 0.627; d > 0.01 || d < -0.01 {
		t.Errorf("share(OPF=140) = %.4f, want 0.627 ± 0.01", share140)
	}
	if d := share100 - 0.545; d > 0.01 || d < -0.01 {
		t.Errorf("share(OPF=100) = %.4f, want 0.545 ± 0.01", share100)
	}
}

func TestPick_ExplicitZeroOPF_AlwaysFallsBackToFlat(t *testing.T) {
	// D-LW-OPF0: an EXPLICIT overprovisioning_factor: 0 makes
	// min(1, (0/100)*frac) == 0 for EVERY locality regardless of health
	// (even at 100%-healthy, frac=1.0: 0*1.0==0) — so the ENTIRE mechanism
	// degrades to the flat fallback UNCONDITIONALLY. This is DISTINCT from
	// the hasOPF=false (defaulted-140) case, which does NOT fall back when
	// fully healthy (TestPick_HealthyLocality_DelegatesToItsOwnChild, Task 4).
	eps := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 5}}
	stubs := map[string]*stubLB{}
	factory := func(sub []Endpoint) (loadBalancer, error) {
		// Deviation from the PLAN's sketch: with a SINGLE locality, the
		// per-group sub-slice and the flat fallback's full-endpoint slice
		// are both length 1 (== len(eps)), so a length-based key never
		// disambiguates them (it collapsed both calls onto "flat", leaving
		// stubs["a"] unset and panicking at the assertion below). Use the
		// constructor's guaranteed call ORDER instead (newLocalityWeightedLB
		// WithRNG, locality.go: one factory call per locality group in the
		// group loop, then exactly one more call for the flat fallback,
		// strictly after) — the first call is always the (only) locality's
		// group, the second is always flat.
		key := "flat"
		if len(stubs) == 0 {
			key = sub[0].Locality.Region
		}
		s := &stubLB{}
		stubs[key] = s
		return s, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(eps, nil /* health nil → frac defaults to 1.0 */, 0, true /* hasOPF, explicit zero */, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := lw.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if stubs["flat"].active.Load() != 1 {
		t.Errorf("explicit overprovisioning_factor=0 must ALWAYS fall back to flat, even at 100%% health; flat active = %d", stubs["flat"].active.Load())
	}
	if stubs["a"].active.Load() != 0 {
		t.Errorf("region a's own child must never fire when OPF=0; active = %d", stubs["a"].active.Load())
	}
}

func TestPick_ChildLocalityCanLocallyPanicIndependently(t *testing.T) {
	// DOCUMENTATION test (not a phase-52 requirement to fix): a locality's
	// child is built by the UNCHANGED buildLeafLB/roundRobin, which
	// evaluates its OWN clusterHealth.inPanic over ITS OWN endpoint
	// sub-slice (health.go's roundRobin.Pick, loadbalancer.go:44-66) — a
	// DIFFERENT scope than the wrapper's cluster-wide inPanic check
	// (Pick, Task 4). A locality's local healthy fraction can dip below the
	// SHARED panicThreshold (0.5 default) while the CLUSTER-WIDE fraction
	// stays above it, so the wrapper itself never enters panic, yet the
	// CHOSEN locality's own child independently sprays across its own
	// full sub-slice (including unhealthy hosts) once delegated to. This is
	// a structural consequence of reusing buildLeafLB's children UNCHANGED
	// (the identical posture subsetLB's children already have) and is
	// UNTESTED by the 0095 differential (its synthetic backends respond 200
	// on data paths regardless of health-check status, so this local-panic
	// behavior never perturbs the REGION-level share assertions). Recorded
	// here as a coverage boundary, found reading the real roundRobin.Pick
	// code at this PLAN's Task 6 review — not a bug to fix in phase 52
	// (fixing it would require passing a per-locality "panic disabled" view
	// into buildLeafLB's children, contradicting the ZERO-new-Pick-parameter,
	// buildLeafLB-reused-unchanged design, D-LW7).
	region := []Endpoint{
		{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 1},
		{Host: "a1", Port: 2, Locality: LocalityID{Region: "a"}, LocalityWeight: 1},
		{Host: "a2", Port: 3, Locality: LocalityID{Region: "a"}, LocalityWeight: 1},
	}
	other := []Endpoint{{Host: "b0", Port: 4, Locality: LocalityID{Region: "b"}, LocalityWeight: 1}}
	all := append(append([]Endpoint{}, region...), other...)
	health := newClusterHealth(all, 0.5)
	health.states["a1:2"].healthy.Store(false)
	health.states["a2:3"].healthy.Store(false) // region a: 1/3 ≈ 33% < 50% (locally panics); cluster-wide: 2/4 = 50%, NOT < 50% (no cluster-wide panic)
	if health.inPanic(all) {
		t.Fatal("test setup invariant broken: cluster-wide must NOT be in panic (2/4 == 50%, strict <)")
	}
	regionAChild := &roundRobin{endpoints: region, health: health}
	factory := func(sub []Endpoint) (loadBalancer, error) {
		if len(sub) == len(region) {
			return regionAChild, nil
		}
		return &roundRobin{endpoints: sub, health: health}, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 }) // rng()==0 → r==0 → the wrapper always picks the FIRST bucket (region a, the only nonzero-remaining-weight locality drawn first in encounter order)
	if err != nil {
		t.Fatal(err)
	}
	// Fully deterministic, not a statistical draw: the wrapper always delegates
	// to regionAChild (rng pinned to 0), and regionAChild's OWN panic branch
	// round-robins via a persistent counter (loadbalancer.go:44-66) that cycles
	// a0,a1,a2,a0,... on every call regardless of health — so an unhealthy host
	// (a1 or a2) is guaranteed within the first 3 draws.
	sawUnhealthyAHost := false
	for i := 0; i < 3; i++ {
		ep, release, err := lw.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatal(err)
		}
		release()
		if ep.Host == "a1" || ep.Host == "a2" {
			sawUnhealthyAHost = true
		}
	}
	if !sawUnhealthyAHost {
		t.Error("region a's local panic branch must surface an unhealthy host within 3 draws (deterministic round-robin cycling) — the documented child-local-panic coverage boundary did not reproduce")
	}
}
