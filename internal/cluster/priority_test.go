package cluster

import (
	"fmt"
	"math"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

func TestTierCapacity_MinCapsAt100(t *testing.T) {
	cases := []struct {
		frac float64
		opf  uint32
		want float64
	}{
		{1.00, 140, 100}, // caps at 100 even though 1.00*140=140
		{0.80, 140, 100}, // 0.80*140=112, caps at 100
		{0.60, 140, 84},
		{0.40, 140, 56},
		{0.20, 140, 28},
		{0.00, 140, 0},
		{1.00, 100, 100},
		{0.50, 100, 50}, // OPF=100 has no plateau margin
	}
	for _, c := range cases {
		got := tierCapacity(c.frac, c.opf)
		if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("tierCapacity(%.2f, %d) = %v, want %v", c.frac, c.opf, got, c.want)
		}
	}
}

func TestCascadeLoads_TwoTier_FullyHealthy_AllToP0(t *testing.T) {
	loads, sum := cascadeLoads([]float64{100, 100})
	if loads[0] != 100 || loads[1] != 0 {
		t.Errorf("loads = %v, want [100, 0]", loads)
	}
	if sum != 200 {
		t.Errorf("capacitySum = %v, want 200", sum)
	}
}

func TestCascadeLoads_TwoTier_PartialP0_Cascades(t *testing.T) {
	// SPEC §11.4 scenario d: P0=40% healthy (capacity 56), P1=100% (capacity 100).
	loads, sum := cascadeLoads([]float64{56, 100})
	if diff := loads[0] - 56; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("loads[0] = %v, want 56", loads[0])
	}
	if diff := loads[1] - 44; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("loads[1] = %v, want 44", loads[1])
	}
	if sum != 156 {
		t.Errorf("capacitySum = %v, want 156", sum)
	}
}

func TestCascadeLoads_ExactlyBoundary100_NotABypass(t *testing.T) {
	// SPEC §11.1/§11.4 scenario f: P0=0% (capacity 0), P1=100%. capacitySum ==
	// EXACTLY 100 — the confirmed boundary that does NOT trigger the AMEND-P1
	// bypass (Pick's own "< 100" check, Task 5, is the actual gate; this test
	// only proves cascadeLoads reports the exact sum Pick's comparison needs).
	loads, sum := cascadeLoads([]float64{0, 100})
	if loads[0] != 0 || loads[1] != 100 {
		t.Errorf("loads = %v, want [0, 100]", loads)
	}
	if sum != 100 {
		t.Errorf("capacitySum = %v, want exactly 100 (the boundary)", sum)
	}
}

func TestCascadeLoads_BelowBoundary_BypassCondition(t *testing.T) {
	// SPEC §11.1 scenario g: both tiers at 20% healthy (capacity 28 each).
	// capacitySum = 56 < 100 — Pick's bypass fires on this value (Task 5).
	_, sum := cascadeLoads([]float64{28, 28})
	if sum != 56 {
		t.Errorf("capacitySum = %v, want 56 (< 100 — the bypass condition)", sum)
	}
}

func TestCascadeLoads_ThreeTier_CorrectedNotNaiveRecursive(t *testing.T) {
	// SPEC §11.4 AMEND-P4: the decisive 3-tier probe. P0=40%(cap 56),
	// P1=60%(cap 84), P2=100%(cap 100). The NAIVE recursive-fraction reading
	// predicts P1=84×44%=36.96, P2=63.04 — REFUTED live. The CORRECTED
	// reading (min-against-remaining-budget) predicts P0=56, P1=44, P2=0 —
	// matching the observed P0=55.0%, P1=45.0%, P2=0.0% almost exactly.
	loads, sum := cascadeLoads([]float64{56, 84, 100})
	want := []float64{56, 44, 0}
	for i, w := range want {
		if diff := loads[i] - w; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("loads[%d] = %v, want %v (CORRECTED cascade, NOT the naive-recursive 36.96/63.04 reading)", i, loads[i], w)
		}
	}
	if sum != 240 {
		t.Errorf("capacitySum = %v, want 240 (56+84+100)", sum)
	}
}

func TestCascadeLoads_ThreeTier_SecondScenario(t *testing.T) {
	// SPEC §11.4: P0=20%(cap 28), P1=20%(cap 28), P2=100%(cap 100). CORRECTED
	// prediction: P0=28, P1=28, P2=44 — matching observed 29.0/28.7/42.3.
	loads, sum := cascadeLoads([]float64{28, 28, 100})
	want := []float64{28, 28, 44}
	for i, w := range want {
		if diff := loads[i] - w; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("loads[%d] = %v, want %v", i, loads[i], w)
		}
	}
	if sum != 156 {
		t.Errorf("capacitySum = %v, want 156 (28+28+100)", sum)
	}
}

func TestCascadeLoads_EmptyCapacities(t *testing.T) {
	loads, sum := cascadeLoads(nil)
	if len(loads) != 0 {
		t.Errorf("loads = %v, want empty", loads)
	}
	if sum != 0 {
		t.Errorf("capacitySum = %v, want 0", sum)
	}
}

func TestTierHealth_SharesStatesButDisablesPanic(t *testing.T) {
	ep := Endpoint{Host: "a", Port: 1}
	shared := newClusterHealth([]Endpoint{ep}, 0.5)
	view := tierHealth(shared)
	if view.panicThreshold != 0 {
		t.Errorf("view.panicThreshold = %v, want 0 (panic permanently disabled)", view.panicThreshold)
	}
	// The states map is the SAME map (a reference type) — mutating the shared
	// registry's host state must be visible through the view.
	shared.states[ep.Addr()].healthy.Store(false)
	if view.isHealthy(ep) {
		t.Error("view must observe LIVE health-check results via the shared states map")
	}
	// panicThreshold=0 means inPanic can never fire: a fraction is never
	// strictly < 0, even at 0% available (AMEND-P1-COROLLARY's structural
	// guarantee — this is what makes a tier's own child incapable of
	// internally flattening, Task 7's Pick-level proof).
	if view.inPanic([]Endpoint{ep}) {
		t.Error("tierHealth's view must NEVER report inPanic, even at 0% available")
	}
	// membershipHealthy/panicCounter/ejectionsActive stay nil (this view never
	// emits stats — priorityLB.Pick itself is the sole caller of
	// health.panicInc(), on its own capacity-sum bypass, Task 5).
	if view.membershipHealthy != nil || view.panicCounter != nil || view.ejectionsActive != nil {
		t.Error("tierHealth's view must not carry any stat handles")
	}
}

func TestTierHealth_NowNanosShared(t *testing.T) {
	// nowNanos is shared so a tier's outlier-ejection lazy-uneject check (if
	// ever exercised through this view) uses the SAME injectable clock as the
	// real registry — a unit-test determinism requirement, not a priority-53
	// feature (outlier detection composition with priority tiers is out of
	// scope this phase, matching SPEC's non-purposes list).
	shared := newClusterHealth(nil, 0.5)
	var fixedNow int64 = 42
	shared.nowNanos = func() int64 { return fixedNow }
	view := tierHealth(shared)
	if view.nowNanos() != 42 {
		t.Errorf("view.nowNanos() = %d, want 42 (shared clock)", view.nowNanos())
	}
}

// trackingPriorityFactory returns a priorityLeafFactory that builds ONE
// stubLB per distinct call, recording each call's endpoint sub-slice (by
// port-sum fingerprint) AND the health VIEW it was given (by pointer), so
// tests can assert what newPriorityLB built and which health view each tier
// vs. the flat child received.
type priorityFactoryCall struct {
	n      int
	sum    uint32
	health *clusterHealth
}

func trackingPriorityFactory() (priorityLeafFactory, *[]priorityFactoryCall) {
	var calls []priorityFactoryCall
	f := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		var sum uint32
		for _, ep := range sub {
			sum += ep.Port
		}
		calls = append(calls, priorityFactoryCall{n: len(sub), sum: sum, health: h})
		return &stubLB{ep: Endpoint{Host: "child", Port: sum}}, nil
	}
	return f, &calls
}

func TestNewPriorityLB_GroupsByPriority(t *testing.T) {
	tier0 := []Endpoint{
		{Host: "p0a", Port: 1, Priority: 0},
		{Host: "p0b", Port: 2, Priority: 0},
	}
	tier1 := []Endpoint{{Host: "p1a", Port: 3, Priority: 1}}
	all := append(append([]Endpoint{}, tier0...), tier1...)
	factory, calls := trackingPriorityFactory()
	shared := newClusterHealth(all, 0.5)
	pr, err := newPriorityLBWithRNG(all, shared, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(pr.groups))
	}
	if pr.groups[0].priority != 0 || pr.groups[1].priority != 1 {
		t.Errorf("groups must be sorted ASCENDING by priority: got [%d, %d]", pr.groups[0].priority, pr.groups[1].priority)
	}
	if len(pr.groups[0].endpoints) != 2 || len(pr.groups[1].endpoints) != 1 {
		t.Errorf("group sizes = [%d, %d], want [2, 1]", len(pr.groups[0].endpoints), len(pr.groups[1].endpoints))
	}
	// factory called 3 times: tier 0 (2 eps), tier 1 (1 ep), flat (3 eps).
	if len(*calls) != 3 {
		t.Fatalf("factory calls = %d, want 3 (2 tiers + 1 flat)", len(*calls))
	}
	var sawFlat bool
	var tierHealthViews []*clusterHealth
	for _, c := range *calls {
		if c.n == 3 {
			sawFlat = true
			if c.health != nil {
				t.Error("the flat child must be built with a NIL health registry (AMEND-P1)")
			}
		} else {
			tierHealthViews = append(tierHealthViews, c.health)
		}
	}
	if !sawFlat {
		t.Errorf("no factory call spanned all 3 endpoints (the flat fallback) — calls: %+v", *calls)
	}
	if len(tierHealthViews) != 2 || tierHealthViews[0] != tierHealthViews[1] {
		t.Error("both tier children must share the SAME tierHealth view instance (ONE shared panic-disabled view, AMEND-P1-COROLLARY)")
	}
	if tierHealthViews[0] == shared {
		t.Error("tier children must NOT receive the raw shared *clusterHealth directly — they must receive tierHealth(shared)'s panic-disabled VIEW")
	}
	if tierHealthViews[0].panicThreshold != 0 {
		t.Errorf("tier children's health view panicThreshold = %v, want 0", tierHealthViews[0].panicThreshold)
	}
}

func TestNewPriorityLB_NilHealth_TierViewAlsoNil(t *testing.T) {
	// A cluster with zero health_checks configured but multi-tier priority
	// still gets a widened, non-nil health at the manager.go call site
	// (D-P-HEALTHALLOC, Task 6) — but this constructor must handle a nil
	// health parameter defensively (unit-test convenience + defense in depth).
	eps := []Endpoint{{Host: "p0a", Port: 1, Priority: 0}, {Host: "p1a", Port: 2, Priority: 1}}
	factory, calls := trackingPriorityFactory()
	_, err := newPriorityLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range *calls {
		if c.health != nil {
			t.Errorf("with a nil shared health, every factory call (tiers AND flat) must receive nil, got %+v", c)
		}
	}
}

func TestNewPriorityLB_DuplicatePriority_EndpointsMerge(t *testing.T) {
	// D-P-DUP: two endpoints sharing the SAME Priority value (simulating two
	// source LocalityLbEndpoints groups that collapsed to the same tier in
	// extractEndpoints's output) — SIMPLER than locality's D-LW-DUP (no
	// per-group scalar to conflict over): both endpoints simply MERGE into
	// the SAME priorityGroup.
	eps := []Endpoint{
		{Host: "h1", Port: 1, Priority: 0},
		{Host: "h2", Port: 2, Priority: 0},
	}
	factory, _ := trackingPriorityFactory()
	pr, err := newPriorityLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.groups) != 1 {
		t.Fatalf("groups = %d, want 1 (both endpoints share Priority 0)", len(pr.groups))
	}
	if len(pr.groups[0].endpoints) != 2 {
		t.Errorf("endpoints = %d, want 2 (both merge into the same tier)", len(pr.groups[0].endpoints))
	}
}

func TestNewPriorityLB_SortsAscending_OutOfOrderInput(t *testing.T) {
	// Endpoints arrive with tiers out of numeric order (2, 0, 1) — groups
	// must still come out ASCENDING (cascade order is load-bearing, Task 5).
	eps := []Endpoint{
		{Host: "p2", Port: 1, Priority: 2},
		{Host: "p0", Port: 2, Priority: 0},
		{Host: "p1", Port: 3, Priority: 1},
	}
	factory, _ := trackingPriorityFactory()
	pr, err := newPriorityLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(pr.groups))
	}
	for i, want := range []uint32{0, 1, 2} {
		if pr.groups[i].priority != want {
			t.Errorf("groups[%d].priority = %d, want %d (ascending)", i, pr.groups[i].priority, want)
		}
	}
}

func TestNewPriorityLB_FactoryErrorPropagates(t *testing.T) {
	eps := []Endpoint{{Host: "a0", Port: 1, Priority: 0}}
	wantErr := errNoEndpoints
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) { return nil, wantErr }
	if _, err := newPriorityLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 }); err != wantErr {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestNewPriorityLB_OverprovisioningFactor_DefaultsOnAbsent(t *testing.T) {
	factory, _ := trackingPriorityFactory()
	pr, err := newPriorityLBWithRNG([]Endpoint{{Host: "a", Port: 1}}, nil, 0, false /* hasOPF */, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if pr.overprovisioningFactor != defaultOverprovisioningFactor {
		t.Errorf("overprovisioningFactor = %d, want %d (absent → default)", pr.overprovisioningFactor, defaultOverprovisioningFactor)
	}
}

func TestNewPriorityLB_OverprovisioningFactor_HonorsExplicitZero(t *testing.T) {
	factory, _ := trackingPriorityFactory()
	pr, err := newPriorityLBWithRNG([]Endpoint{{Host: "a", Port: 1}}, nil, 0, true /* hasOPF, explicit zero */, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if pr.overprovisioningFactor != 0 {
		t.Errorf("overprovisioningFactor = %d, want 0 (explicit zero honored, not defaulted)", pr.overprovisioningFactor)
	}
}

func TestPick_AllHealthy_TwoTier_AlwaysP0(t *testing.T) {
	// No health (frac defaults to 1.0 for every tier) → capacities [100,100],
	// cascade loads [100,0] → tier 1's bucket has zero width and is excluded
	// from the walk entirely — deterministically ALWAYS tier 0, regardless of
	// rng. This is arm (a)'s exact mechanism (SPEC §8.1).
	tier0 := []Endpoint{{Host: "p0", Port: 1, Priority: 0}}
	tier1 := []Endpoint{{Host: "p1", Port: 2, Priority: 1}}
	all := append(append([]Endpoint{}, tier0...), tier1...)
	stubs := map[uint32]*stubLB{}
	var flat *stubLB
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		if len(sub) == len(all) {
			flat = &stubLB{}
			return flat, nil
		}
		s := &stubLB{}
		stubs[sub[0].Priority] = s
		return s, nil
	}
	pr, err := newPriorityLBWithRNG(all, nil, 140, true, factory, func() uint64 { return math.MaxUint64 }) // rng pinned to the MAX draw — still must land in tier 0 since tier 1's bucket is empty
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, _, err := pr.Pick(0, false, SubsetMatch{}, false); err != nil {
			t.Fatal(err)
		}
	}
	if stubs[0].active.Load() != 5 {
		t.Errorf("tier 0 must be picked every time; active = %d", stubs[0].active.Load())
	}
	if stubs[1].active.Load() != 0 {
		t.Errorf("tier 1 (zero remaining budget) must NEVER be picked; active = %d", stubs[1].active.Load())
	}
	if flat.active.Load() != 0 {
		t.Errorf("flat must not fire when capacitySum >= 100; active = %d", flat.active.Load())
	}
}

func TestPick_ExactBoundary100_NoBypass_AllToP1(t *testing.T) {
	// SPEC §11.1/§11.4 scenario f: tier 0 FULLY unhealthy (0%), tier 1 fully
	// healthy. capacitySum == EXACTLY 100 — confirmed NOT a bypass (strict <).
	// Cascade loads [0, 100] → tier 0's bucket is empty → always tier 1.
	tier0 := []Endpoint{{Host: "p0", Port: 1, Priority: 0}}
	tier1 := []Endpoint{{Host: "p1", Port: 2, Priority: 1}}
	all := append(append([]Endpoint{}, tier0...), tier1...)
	health := newClusterHealth(all, 0.5)
	health.states["p0:1"].healthy.Store(false)
	stubs := map[uint32]*stubLB{}
	var flat *stubLB
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		if len(sub) == len(all) {
			flat = &stubLB{}
			return flat, nil
		}
		s := &stubLB{}
		stubs[sub[0].Priority] = s
		return s, nil
	}
	pr, err := newPriorityLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pr.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if stubs[1].active.Load() != 1 {
		t.Error("the exactly-100 boundary must NOT bypass — tier 1 must be picked (all-healthy tier)")
	}
	if flat.active.Load() != 0 {
		t.Error("the exactly-100 boundary must NOT delegate to flat")
	}
	if stubs[0].active.Load() != 0 {
		t.Error("tier 0 (0% healthy, zero remaining budget) must never be picked")
	}
}

func TestPick_CapacityShortfall_BypassesToFlat(t *testing.T) {
	// SPEC §11.1 scenario g: BOTH tiers at 20% healthy (5 hosts each, 1
	// healthy). capacitySum = 28+28 = 56 < 100 → the AMEND-P1 bypass fires.
	mkTier := func(pfx string, priority uint32, healthyCount int) []Endpoint {
		eps := make([]Endpoint, 5)
		for i := range eps {
			eps[i] = Endpoint{Host: fmt.Sprintf("%s%d", pfx, i), Port: uint32(i + 1), Priority: priority}
		}
		return eps
	}
	tier0 := mkTier("t0h", 0, 1)
	tier1 := mkTier("t1h", 1, 1)
	all := append(append([]Endpoint{}, tier0...), tier1...)
	health := newClusterHealth(all, 0.5)
	for _, ep := range tier0[1:] {
		health.states[ep.Addr()].healthy.Store(false)
	}
	for _, ep := range tier1[1:] {
		health.states[ep.Addr()].healthy.Store(false)
	}
	stubs := map[uint32]*stubLB{}
	var flat *stubLB
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		if len(sub) == len(all) {
			flat = &stubLB{}
			return flat, nil
		}
		s := &stubLB{}
		stubs[sub[0].Priority] = s
		return s, nil
	}
	pr, err := newPriorityLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pr.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if flat.active.Load() != 1 {
		t.Errorf("capacitySum=56<100 must bypass to flat; flat active = %d", flat.active.Load())
	}
	if stubs[0].active.Load() != 0 || stubs[1].active.Load() != 0 {
		t.Error("bypass must NOT delegate to either tier's own child")
	}
}

func TestPick_CapacityShortfall_IncrementsPanicCounter(t *testing.T) {
	// The bypass reuses the EXISTING lb_healthy_panic counter (SPEC §7/§11.5
	// — no new stat). Verified via a real *stats.Registry-backed Counter
	// (Counter.Load(), not a Registry-wide snapshot — Registry exposes no
	// such method; Walk(fn) is the only bulk accessor and is overkill for a
	// single known handle).
	reg := stats.NewRegistry()
	tier0 := []Endpoint{{Host: "p0", Port: 1, Priority: 0}}
	tier1 := []Endpoint{{Host: "p1", Port: 2, Priority: 1}}
	all := append(append([]Endpoint{}, tier0...), tier1...)
	health := newClusterHealth(all, 0.5)
	panicCounter := reg.NewCounter("test.lb_healthy_panic")
	health.panicCounter = panicCounter
	health.states["p0:1"].healthy.Store(false)
	health.states["p1:2"].healthy.Store(false) // both 0% healthy → capacitySum = 0 < 100
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) { return &stubLB{}, nil }
	pr, err := newPriorityLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pr.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if got := panicCounter.Load(); got != 1 {
		t.Errorf("lb_healthy_panic = %v, want 1", got)
	}
}

func TestPick_PerTierChild_NeverSpraysUnhealthyHosts(t *testing.T) {
	// AMEND-P1-COROLLARY, the Pick-level structural proof: tier 0 at 20%
	// healthy (1/5) built with a REAL roundRobin child over tierHealth(health)
	// — the confirmed reference property is that a degraded tier NEVER
	// internally flattens: its 4 unhealthy hosts get ZERO traffic, its 1
	// healthy host gets ALL of the tier's share. Tier 1 stays fully healthy
	// (capacitySum = 28+100 = 128 >= 100 — no bypass, matching SPEC §11.1
	// scenario e). rng is pinned so the cascade draw always lands in tier 0's
	// bucket (r < 28).
	tier0 := make([]Endpoint, 5)
	for i := range tier0 {
		tier0[i] = Endpoint{Host: fmt.Sprintf("t0h%d", i), Port: uint32(i + 1), Priority: 0}
	}
	tier1 := []Endpoint{{Host: "t1h0", Port: 100, Priority: 1}}
	all := append(append([]Endpoint{}, tier0...), tier1...)
	health := newClusterHealth(all, 0.5)
	for _, ep := range tier0[1:] {
		health.states[ep.Addr()].healthy.Store(false) // 4 of 5 unhealthy → 20% healthy
	}
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		return &roundRobin{endpoints: sub, health: h}, nil
	}
	pr, err := newPriorityLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 }) // r == 0 → always the first (lowest-cum) bucket, tier 0
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for i := 0; i < 50; i++ {
		ep, release, err := pr.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatal(err)
		}
		release()
		seen[ep.Host]++
	}
	if seen["t0h0"] != 50 {
		t.Errorf("the ONE healthy host in tier 0 must receive ALL 50 picks; seen = %+v", seen)
	}
	for _, unhealthy := range []string{"t0h1", "t0h2", "t0h3", "t0h4"} {
		if seen[unhealthy] != 0 {
			t.Errorf("unhealthy host %q must receive ZERO traffic (AMEND-P1-COROLLARY — no per-tier local panic spray); got %d", unhealthy, seen[unhealthy])
		}
	}
}

func TestPriorityLBPick_ForwardsHashKeyAndMatchUnchanged(t *testing.T) {
	// D-... / SPEC §3.2: hashKey/match pass straight through to the chosen child.
	eps := []Endpoint{{Host: "p0", Port: 1, Priority: 0}}
	var gotHashKey uint64
	var gotHasHash bool
	var gotMatch SubsetMatch
	var gotHasMatch bool
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		return &argRecordingLB{onPick: func(hk uint64, hh bool, m SubsetMatch, hm bool) {
			gotHashKey, gotHasHash, gotMatch, gotHasMatch = hk, hh, m, hm
		}}, nil
	}
	pr, err := newPriorityLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	wantMatch := NewSubsetMatch(map[string]SubsetValue{"k": {Kind: subsetString, Str: "v"}})
	if _, _, err := pr.Pick(42, true, wantMatch, true); err != nil {
		t.Fatal(err)
	}
	if gotHashKey != 42 || !gotHasHash || gotMatch.Key() != wantMatch.Key() || !gotHasMatch {
		t.Errorf("forwarded (hashKey=%d, hasHash=%v, match=%v, hasMatch=%v), want (42, true, %v, true)", gotHashKey, gotHasHash, gotMatch, gotHasMatch, wantMatch)
	}
}

func TestPriorityFormula_ConfirmedTwoTierTable(t *testing.T) {
	// SPEC §11.4's full predicted-vs-observed table, scenarios (a)-(h) — this
	// test asserts the EXACT mathematical prediction the CORRECTED formula
	// produces (not the noisy "observed" column, which includes binomial
	// sampling noise at n=300; the differential fixture, Task 9, proves the
	// noisy live-traffic match). tier1 (P1) is ALWAYS 100% healthy except (h).
	const opf = 140
	cases := []struct {
		name      string
		fracP0    float64
		fracP1    float64
		wantCap0  float64
		wantCap1  float64
		wantLoad0 float64
		wantLoad1 float64
		wantSum   float64
	}{
		{"a_baseline", 1.00, 1.00, 100, 100, 100, 0, 200},
		{"b_80pct", 0.80, 1.00, 100, 100, 100, 0, 200}, // capped at 100 despite 0.80*140=112
		{"c_60pct", 0.60, 1.00, 84, 100, 84, 16, 184},
		{"d_40pct", 0.40, 1.00, 56, 100, 56, 44, 156},
		{"e_20pct", 0.20, 1.00, 28, 100, 28, 72, 128},
		{"f_0pct_boundary", 0.00, 1.00, 0, 100, 0, 100, 100}, // exactly 100 — the confirmed boundary
		{"h_20pct_vs_80pct", 0.20, 0.80, 28, 100, 28, 72, 128},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cap0 := tierCapacity(c.fracP0, opf)
			cap1 := tierCapacity(c.fracP1, opf)
			if diff := cap0 - c.wantCap0; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("cap0 = %v, want %v", cap0, c.wantCap0)
			}
			if diff := cap1 - c.wantCap1; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("cap1 = %v, want %v", cap1, c.wantCap1)
			}
			loads, sum := cascadeLoads([]float64{cap0, cap1})
			if diff := loads[0] - c.wantLoad0; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("load0 = %v, want %v", loads[0], c.wantLoad0)
			}
			if diff := loads[1] - c.wantLoad1; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("load1 = %v, want %v", loads[1], c.wantLoad1)
			}
			if diff := sum - c.wantSum; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("capacitySum = %v, want %v", sum, c.wantSum)
			}
		})
	}
}

func TestPriorityFormula_BypassScenario_g(t *testing.T) {
	// SPEC §11.1 scenario (g): BOTH tiers at 20% healthy → capacitySum=56<100
	// — the confirmed AMEND-P1 bypass condition (Pick's own comparison,
	// Task 5, is what actually triggers the bypass; this test isolates the
	// pure-function inputs feeding that comparison).
	cap0 := tierCapacity(0.20, 140)
	cap1 := tierCapacity(0.20, 140)
	_, sum := cascadeLoads([]float64{cap0, cap1})
	if sum != 56 {
		t.Errorf("capacitySum = %v, want 56 (< 100 — triggers the bypass)", sum)
	}
	if !(sum < 100) {
		t.Fatal("test setup invariant broken: sum must be < 100")
	}
}

func TestPriorityFormula_ThreeTier_BothConfirmedScenarios(t *testing.T) {
	// SPEC §11.4's two 3-tier scenarios, run through the REAL
	// tierCapacity→cascadeLoads pipeline end to end (not just cascadeLoads'
	// own unit test at Task 3, which hardcoded the capacities — this test
	// derives them from healthy fractions too, closing the loop).
	cases := []struct {
		name      string
		fracs     []float64
		wantCaps  []float64
		wantLoads []float64
		wantSum   float64
	}{
		{
			name:      "40_60_100",
			fracs:     []float64{0.40, 0.60, 1.00},
			wantCaps:  []float64{56, 84, 100},
			wantLoads: []float64{56, 44, 0},
			wantSum:   240,
		},
		{
			name:      "20_20_100",
			fracs:     []float64{0.20, 0.20, 1.00},
			wantCaps:  []float64{28, 28, 100},
			wantLoads: []float64{28, 28, 44},
			wantSum:   156,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			caps := make([]float64, len(c.fracs))
			for i, f := range c.fracs {
				caps[i] = tierCapacity(f, 140)
			}
			for i, want := range c.wantCaps {
				if diff := caps[i] - want; diff > 1e-9 || diff < -1e-9 {
					t.Errorf("cap[%d] = %v, want %v", i, caps[i], want)
				}
			}
			loads, sum := cascadeLoads(caps)
			for i, want := range c.wantLoads {
				if diff := loads[i] - want; diff > 1e-9 || diff < -1e-9 {
					t.Errorf("load[%d] = %v, want %v (the CORRECTED cascade — NOT the naive-recursive reading)", i, loads[i], want)
				}
			}
			if diff := sum - c.wantSum; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("capacitySum = %v, want %v", sum, c.wantSum)
			}
		})
	}
}

// NOTE on reject-arm test placement: the both-locality_weighted_lb_config
// and both-lb_subset_config composition rejects (SPEC §6.1) are TDD'd
// INLINE at Task 6 (TestManager_Reject_PriorityWithLocalityWeighted /
// TestManager_Reject_PriorityWithLbSubsetConfig, manager_test.go) — the
// phase-52 Task 5/6 precedent (locality-weighted's own PLAN made the
// identical split). They are NOT duplicated here.
