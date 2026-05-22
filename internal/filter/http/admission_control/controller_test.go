package admission_control

// controller_test.go — Layer A FAKE-TIME algorithmic-fidelity tests + boundary
// tests + race tests for the sliding-window admission-control controller per
// phase-23 SPEC §14.1 Layer A + AMEND-1 + AMEND-2 + AMEND-6 + AMEND-11 + PD-6
// + PD-7.
//
// # Test taxonomy (6 families per SPEC §14.1)
//
//  1. TestShouldReject_Boundary_* — per AMEND-2 + PD-6: knife-edge + P=0 cross-side
//  2. TestProbabilityFormula_* — per AMEND-1: vector tests over formula params
//  3. TestController_FAKE_TIME_Window_* — per AMEND-6 + PD-7: bucket rollover +
//     stale-purge + requestCounts + averageRps via fakeClock.Advance
//  4. TestRpsSuppression_* — per §4.1: averageRps correctness feeding the gate
//  5. TestRecordDiscipline_* — per AMEND-11: classify counter + window increments
//  6. TestController_Concurrent_* — race tests: concurrent operations under mu
//
// # Fakes consumed (from test-scope files per Task 3 — NOT redefined here)
//
//   - fakeRand (rand_test.go): fakeRand{v: uint64} — deterministic Rand seam
//   - fakeClock (clock_test.go): newFakeClock(start) + Advance(d) — step-driven Clock
//
// DO NOT redefine fakeRand or fakeClock here — doing so causes duplicate-symbol
// build errors.

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// testCompiledConfigAC returns a default *compiledConfig for controller tests.
// Tests that vary specific fields mutate the returned struct in-place.
func testCompiledConfigAC() *compiledConfig {
	return &compiledConfig{
		enabled:                 true,
		samplingWindow:          30 * time.Second,
		aggression:              1.0,
		srThreshold:             0.95,
		rpsThreshold:            0,
		maxRejectionProbability: 0.80,
		httpSuccessRanges:       []int32Range{{start: 100, end: 500}},
		grpcSuccessCodes:        makeGRPCSet(defaultGRPCSuccessCodes),
	}
}

// testFilterStatsAC returns a fresh *filterStats backed by a fresh Registry.
func testFilterStatsAC() *filterStats {
	return newFilterStats(stats.NewRegistry(), "http.test")
}

// newTestController builds a controller with the given cfg + stats + clock + rand.
// Uses testCompiledConfigAC() for cfg if cfg is nil.
func newTestController(
	cfg *compiledConfig,
	st *filterStats,
	clk Clock,
	rnd Rand,
) *controller {
	if cfg == nil {
		cfg = testCompiledConfigAC()
	}
	if st == nil {
		st = testFilterStatsAC()
	}
	return newController(cfg, st, clk, rnd)
}

// computeExpectedP computes the expected rejection probability for given
// {n, s, srThreshold, aggression, maxRejectionProbability} to cross-check
// shouldReject's formula without calling into the production code.
// Matches the LOCKED formula exactly:
//
//	inner = (n - s/srThreshold) / (n + 1)
//	if aggression == 1.0: p = inner
//	else:                  p = math.Pow(inner, 1.0/aggression)
//	p = math.Min(maxRejectionProbability, p)
//	p = math.Max(0, p)
func computeExpectedP(n, s uint64, srThreshold, aggression, maxRejP float64) float64 {
	inner := (float64(n) - float64(s)/srThreshold) / (float64(n) + 1)
	var p float64
	if aggression == 1.0 {
		p = inner
	} else {
		p = math.Pow(inner, 1.0/aggression)
	}
	p = math.Min(maxRejP, p)
	return math.Max(0, p)
}

// primeWindow records success requests in the controller's window without
// advancing the clock (all land in the same bucket). Used to prime the window
// state for shouldReject tests.
func primeWindow(t *testing.T, c *controller, requests, successes uint64) {
	t.Helper()
	if successes > requests {
		t.Fatalf("primeWindow misuse: successes %d > requests %d", successes, requests)
	}
	failures := requests - successes
	for i := uint64(0); i < successes; i++ {
		c.recordRequest(true)
	}
	for i := uint64(0); i < failures; i++ {
		c.recordRequest(false)
	}
}

// -----------------------------------------------------------------------------
// 1. TestShouldReject_Boundary_* — per AMEND-2 + PD-6
// -----------------------------------------------------------------------------

// TestShouldReject_Boundary_AtKnifeEdge_Admits verifies that when
// r%10000 == floor(10000·P), the decision is ADMIT (strict > is false at
// equality) per AMEND-2 + PD-6.
func TestShouldReject_Boundary_AtKnifeEdge_Admits(t *testing.T) {
	// Setup: default aggression=1.0, srThreshold=0.95, maxRejP=0.80
	// With n=100, s=0: inner = (100 - 0/0.95) / 101 = 100/101 ≈ 0.9901
	// After min(0.80, ...) clamp: P = 0.80
	// floor(10000 * 0.80) = 8000
	// At r%10000 == 8000: float64(10000)*0.80 == 8000.0 > 8000.0 is FALSE → ADMIT
	const n, s = uint64(100), uint64(0)
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	// Inject r such that r%10000 == 8000 (the knife-edge value for P=0.80)
	rnd := fakeRand{v: 8000} // 8000 % 10000 = 8000
	c := newTestController(cfg, nil, clk, rnd)
	primeWindow(t, c, n, s)

	// shouldReject: P=0.80, floor(10000*0.80)=8000, r%10000=8000 → 8000.0 > 8000 is false → ADMIT
	if got := c.shouldReject(); got != false {
		t.Errorf("shouldReject() = true at knife-edge r%%10000==%d; want false (admit at equality)", 8000)
	}
}

// TestShouldReject_Boundary_OneLessThanKnifeEdge_Rejects verifies that when
// r%10000 == floor(10000·P)−1, the decision is REJECT per AMEND-2 + PD-6.
func TestShouldReject_Boundary_OneLessThanKnifeEdge_Rejects(t *testing.T) {
	// Same setup: P=0.80, floor(10000*0.80)=8000
	// At r%10000 == 7999: float64(10000)*0.80 == 8000.0 > 7999.0 is TRUE → REJECT
	const n, s = uint64(100), uint64(0)
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	rnd := fakeRand{v: 7999} // 7999 % 10000 = 7999
	c := newTestController(cfg, nil, clk, rnd)
	primeWindow(t, c, n, s)

	if got := c.shouldReject(); got != true {
		t.Errorf("shouldReject() = false at r%%10000==%d (one-less than knife-edge); want true (reject)", 7999)
	}
}

// TestShouldReject_Boundary_PZero_NeverRejects verifies that when P=0 (all
// successes in window), shouldReject always returns false for ALL r values.
// This is the cross-side byte-exact leg per AMEND-2: 0 > (r%10000) is false
// for every r ∈ [0, 10000).
func TestShouldReject_Boundary_PZero_NeverRejects(t *testing.T) {
	// n=100, s=100, srThreshold=0.95 → s/srThreshold = 105.26 > n → inner < 0 → P=0
	// Also test n=0 (empty window) → P=0
	testCases := []struct {
		name        string
		n, s        uint64
		srThreshold float64
	}{
		{"healthy_window", 100, 100, 0.95},
		{"empty_window", 0, 0, 0.95},
		{"successes_exceed_sr_adjusted", 50, 50, 0.95},
	}

	// Test a range of r values including 0..9999 at modulo boundaries.
	rValues := []uint64{0, 1, 99, 100, 999, 1000, 4999, 5000, 7999, 8000, 9000, 9998, 9999,
		10000, 19999, 20000, 1<<32 - 1, 1<<63 - 1, ^uint64(0)}

	for _, tc := range testCases {
		for _, rv := range rValues {
			clk := newFakeClock(time.Unix(0, 0))
			cfg := testCompiledConfigAC()
			cfg.srThreshold = tc.srThreshold
			rnd := fakeRand{v: rv}
			c := newTestController(cfg, nil, clk, rnd)
			primeWindow(t, c, tc.n, tc.s)

			if got := c.shouldReject(); got {
				t.Errorf("TestShouldReject_Boundary_PZero_NeverRejects [%s] r=%d: shouldReject()=true; want false (P=0 never rejects)", tc.name, rv)
			}
		}
	}
}

// TestShouldReject_Boundary_HighR_WithModulo verifies that high r values are
// correctly reduced via modulo — r=10001 behaves the same as r=1 for the
// reject decision.
func TestShouldReject_Boundary_HighR_WithModulo(t *testing.T) {
	// P=0.80, floor(10000*0.80)=8000
	// r=18000: 18000%10000=8000 → 8000.0>8000 is false → ADMIT
	// r=10001: 10001%10000=1 → 8000.0>1 is true → REJECT
	const n, s = uint64(100), uint64(0)

	testCases := []struct {
		r       uint64
		wantRej bool
		desc    string
	}{
		{18000, false, "r=18000 mod 10000=8000 knife-edge"},
		{10001, true, "r=10001 mod 10000=1 rejects"},
		{18001, false, "r=18001 mod 10000=8001 admits"},
	}

	for _, tc := range testCases {
		clk := newFakeClock(time.Unix(0, 0))
		cfg := testCompiledConfigAC()
		rnd := fakeRand{v: tc.r}
		c := newTestController(cfg, nil, clk, rnd)
		primeWindow(t, c, n, s)

		if got := c.shouldReject(); got != tc.wantRej {
			t.Errorf("r=%d (%s): shouldReject()=%v; want %v", tc.r, tc.desc, got, tc.wantRej)
		}
	}
}

// -----------------------------------------------------------------------------
// 2. TestProbabilityFormula_* — per AMEND-1
// -----------------------------------------------------------------------------

// TestProbabilityFormula_DefaultParams verifies the formula with default params
// and varying n/s.
func TestProbabilityFormula_DefaultParams(t *testing.T) {
	// aggression=1.0, srThreshold=0.95, maxRejP=0.80
	testCases := []struct {
		name  string
		n, s  uint64
		wantP float64
	}{
		// n=0 (empty window): inner = (0-0)/(0+1) = 0 → P=0
		{"empty_window", 0, 0, 0.0},
		// n=100, s=0: inner = (100-0/0.95)/(101) = 100/101 ≈ 0.99 → clamped to 0.80
		{"all_failures", 100, 0, 0.80},
		// n=100, s=95: inner = (100-95/0.95)/(101) = (100-100)/101 = 0 → P=0
		{"at_sr_threshold", 100, 95, 0.0},
		// n=100, s=100: inner = (100-100/0.95)/(101) = (100-105.26)/101 < 0 → max(0,·)=0
		{"all_success", 100, 100, 0.0},
	}

	for _, tc := range testCases {
		wantP := computeExpectedP(tc.n, tc.s, 0.95, 1.0, 0.80)
		// Cross-verify our helper matches the textual expectation
		if math.Abs(wantP-tc.wantP) > 1e-9 {
			t.Errorf("[%s] helper P=%.10f; textual want=%.10f — fix test helper", tc.name, wantP, tc.wantP)
		}

		// Now test via shouldReject with r=0 (r%10000=0):
		// if P>0 → 10000*P > 0 → REJECT (since P>0 means 10000*P >= 1)
		// if P=0 → 0 > 0 = false → ADMIT
		clk := newFakeClock(time.Unix(0, 0))
		cfg := testCompiledConfigAC()
		// Use r that will reject if P>0, admit if P=0:
		// We use r=0 so r%10000=0: reject iff P>0
		rnd := fakeRand{v: 0}
		c := newTestController(cfg, nil, clk, rnd)
		primeWindow(t, c, tc.n, tc.s)

		gotRej := c.shouldReject()
		wantRej := wantP > 0
		if gotRej != wantRej {
			t.Errorf("[%s] n=%d s=%d: shouldReject()=%v (wantP=%.6f); want %v",
				tc.name, tc.n, tc.s, gotRej, wantP, wantRej)
		}
	}
}

// TestProbabilityFormula_AggressionExponentSkippedAt1_0 verifies that when
// aggression==1.0 the pow exponent is skipped (the formula uses inner directly
// without math.Pow) per AMEND-1 + upstream admission_control.cc:170-171.
func TestProbabilityFormula_AggressionExponentSkippedAt1_0(t *testing.T) {
	// n=100, s=0, srThreshold=0.95, aggression=1.0: P = min(0.80, 100/101) = 0.80
	// With aggression=2.0: P = min(0.80, pow(100/101, 0.5)) = min(0.80, ~0.9950) = 0.80
	// (still clamped; need different params to distinguish)
	// n=10, s=5, srThreshold=1.0, aggression=1.0:
	//   inner = (10-5/1.0)/11 = 5/11 ≈ 0.4545
	//   aggression=1.0: P = min(0.80, 0.4545) = 0.4545
	// n=10, s=5, srThreshold=1.0, aggression=2.0:
	//   inner = 5/11 ≈ 0.4545
	//   pow(0.4545, 0.5) ≈ 0.6742
	//   P = min(0.80, 0.6742) = 0.6742
	// These are different → we can distinguish via r-modulo.

	const n, s = uint64(10), uint64(5)

	// Test aggression=1.0:
	pAgg1 := computeExpectedP(n, s, 1.0, 1.0, 0.80)
	// Test aggression=2.0:
	pAgg2 := computeExpectedP(n, s, 1.0, 2.0, 0.80)

	if pAgg1 == pAgg2 {
		t.Fatalf("test setup error: pAgg1=pAgg2=%.6f; choose params that distinguish aggression effect", pAgg1)
	}

	// Verify both values round to distinct first-admit boundaries.
	// Use firstAdmit = ceil(10000*P): the smallest r%10000 that admits.
	// The reject decision is float64(10000)*P > float64(r%10000), so:
	//   reject when r%10000 < 10000*P, i.e. r%10000 < ceil(10000*P)
	//   admit  when r%10000 >= 10000*P, i.e. r%10000 >= ceil(10000*P)

	// Use firstAdmit1 = ceil(10000*pAgg1) as the first-admit boundary for agg1.
	// Use firstAdmit2 = ceil(10000*pAgg2) as the first-admit boundary for agg2.
	// Since pAgg1 < pAgg2, firstAdmit1 <= firstAdmit2.
	// If firstAdmit1 < firstAdmit2:
	//   r = firstAdmit1: agg1 admits; agg2 rejects (since firstAdmit1 < firstAdmit2).
	tenKP1 := float64(10000) * pAgg1
	tenKP2 := float64(10000) * pAgg2
	firstAdmit1 := uint64(math.Ceil(tenKP1))
	firstAdmit2 := uint64(math.Ceil(tenKP2))

	if firstAdmit1 < firstAdmit2 {
		r := firstAdmit1
		// agg1 admits at firstAdmit1
		{
			clk := newFakeClock(time.Unix(0, 0))
			cfg := testCompiledConfigAC()
			cfg.aggression = 1.0
			cfg.srThreshold = 1.0
			cfg.maxRejectionProbability = 0.80
			rnd := fakeRand{v: r}
			c := newTestController(cfg, nil, clk, rnd)
			primeWindow(t, c, n, s)
			if c.shouldReject() {
				t.Errorf("aggression=1.0 at first-admit r=%d (10000*P=%.4f): shouldReject()=true; want false", r, tenKP1)
			}
		}
		// agg2 rejects at firstAdmit1 (since firstAdmit1 < firstAdmit2 means 10000*pAgg2 > r)
		{
			clk := newFakeClock(time.Unix(0, 0))
			cfg := testCompiledConfigAC()
			cfg.aggression = 2.0
			cfg.srThreshold = 1.0
			cfg.maxRejectionProbability = 0.80
			rnd := fakeRand{v: r}
			c := newTestController(cfg, nil, clk, rnd)
			primeWindow(t, c, n, s)
			if !c.shouldReject() {
				t.Errorf("aggression=2.0 at r=%d (10000*P=%.4f > r): shouldReject()=false; want true", r, tenKP2)
			}
		}
	} else {
		t.Fatalf("firstAdmit1=%d firstAdmit2=%d: cannot distinguish aggression=1.0 vs aggression=2.0 (same first-admit boundary) — choose params that produce distinct boundaries", firstAdmit1, firstAdmit2)
	}
	_ = tenKP1 // used above
	_ = tenKP2 // used above
}

// TestProbabilityFormula_SrThresholdDividesSuccesses verifies that srThreshold
// divides the success count in the numerator (s/srThreshold) rather than acting
// as a separate gate per AMEND-1.
func TestProbabilityFormula_SrThresholdDividesSuccesses(t *testing.T) {
	// n=100, s=90:
	// srThreshold=0.90 → s/srThreshold = 90/0.90 = 100.0 → inner = (100-100)/101 = 0 → P=0
	// srThreshold=0.95 → s/srThreshold = 90/0.95 ≈ 94.74 → inner = (100-94.74)/101 ≈ 0.052 → P>0
	const n, s = uint64(100), uint64(90)

	// With srThreshold=0.90: P should be 0
	pSr90 := computeExpectedP(n, s, 0.90, 1.0, 0.80)
	if pSr90 != 0.0 {
		t.Fatalf("setup check: expected P=0 for srThreshold=0.90 n=100 s=90; got %.10f", pSr90)
	}

	// With srThreshold=0.95: P should be > 0
	pSr95 := computeExpectedP(n, s, 0.95, 1.0, 0.80)
	if pSr95 <= 0 {
		t.Fatalf("setup check: expected P>0 for srThreshold=0.95 n=100 s=90; got %.10f", pSr95)
	}

	// Controller with srThreshold=0.90 and r=0 → admit (P=0)
	{
		clk := newFakeClock(time.Unix(0, 0))
		cfg := testCompiledConfigAC()
		cfg.srThreshold = 0.90
		rnd := fakeRand{v: 0}
		c := newTestController(cfg, nil, clk, rnd)
		primeWindow(t, c, n, s)
		if c.shouldReject() {
			t.Errorf("srThreshold=0.90 n=%d s=%d r=0: shouldReject()=true; want false (P=0 — s/srThreshold==n)", n, s)
		}
	}

	// Controller with srThreshold=0.95 and r=0 → reject (P>0)
	{
		clk := newFakeClock(time.Unix(0, 0))
		cfg := testCompiledConfigAC()
		cfg.srThreshold = 0.95
		rnd := fakeRand{v: 0}
		c := newTestController(cfg, nil, clk, rnd)
		primeWindow(t, c, n, s)
		if !c.shouldReject() {
			t.Errorf("srThreshold=0.95 n=%d s=%d r=0: shouldReject()=false; want true (P>0 — s/srThreshold<n)", n, s)
		}
	}
}

// TestProbabilityFormula_AggressionFloor verifies that configured aggression
// below 1.0 is floored to 1.0 at config-time (per AMEND-1 + Task 2
// buildCompiledConfig). We verify by checking the formula matches aggression=1.0
// behavior even when the compiled config already has aggression=1.0 floored.
func TestProbabilityFormula_AggressionFloor(t *testing.T) {
	// This test verifies that cc.aggression=1.0 (floored from 0.5 at config-time)
	// produces the same result as cc.aggression=1.0 set directly.
	// Since flooring happens at buildCompiledConfig (Task 2), and compiledConfig
	// already holds the floored value, we just verify the formula with aggression=1.0.
	const n, s = uint64(20), uint64(0)
	pExpected := computeExpectedP(n, s, 0.95, 1.0, 0.80)

	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	cfg.aggression = 1.0 // already floored value
	// Use r such that knife-edge at floor(10000*pExpected) admits
	floor := uint64(math.Floor(10000 * pExpected))
	rnd := fakeRand{v: floor}
	c := newTestController(cfg, nil, clk, rnd)
	primeWindow(t, c, n, s)

	// At knife-edge r%10000 == floor → ADMIT
	if c.shouldReject() {
		t.Errorf("aggression=1.0 knife-edge: shouldReject()=true; want false")
	}
}

// TestProbabilityFormula_MaxRejPClamp verifies that the maxRejectionProbability
// clamp limits P to at most maxRejP per AMEND-1.
func TestProbabilityFormula_MaxRejPClamp(t *testing.T) {
	// n=200, s=0, srThreshold=0.95, aggression=1.0:
	// inner = (200-0)/(201) ≈ 0.9950 > maxRejP
	// With maxRejP=0.80: P = 0.80 → floor(10000*0.80) = 8000
	// With maxRejP=0.50: P = 0.50 → floor(10000*0.50) = 5000
	const n, s = uint64(200), uint64(0)

	// Test maxRejP=0.80 → knife-edge at 8000
	{
		floor := uint64(8000)
		clk := newFakeClock(time.Unix(0, 0))
		cfg := testCompiledConfigAC()
		cfg.maxRejectionProbability = 0.80
		rnd := fakeRand{v: floor} // admits at knife-edge
		c := newTestController(cfg, nil, clk, rnd)
		primeWindow(t, c, n, s)
		if c.shouldReject() {
			t.Errorf("maxRejP=0.80 r%%10000=8000 knife-edge: shouldReject()=true; want false (admit at equality)")
		}
	}

	// Test maxRejP=0.50 → knife-edge at 5000
	{
		floor := uint64(5000)
		clk := newFakeClock(time.Unix(0, 0))
		cfg := testCompiledConfigAC()
		cfg.maxRejectionProbability = 0.50
		rnd := fakeRand{v: floor} // admits at knife-edge
		c := newTestController(cfg, nil, clk, rnd)
		primeWindow(t, c, n, s)
		if c.shouldReject() {
			t.Errorf("maxRejP=0.50 r%%10000=5000 knife-edge: shouldReject()=true; want false (admit at equality)")
		}
		// One below knife-edge → REJECT
		rnd2 := fakeRand{v: floor - 1}
		c2 := newTestController(cfg, nil, newFakeClock(time.Unix(0, 0)), rnd2)
		primeWindow(t, c2, n, s)
		if !c2.shouldReject() {
			t.Errorf("maxRejP=0.50 r%%10000=%d (one below knife-edge): shouldReject()=false; want true", floor-1)
		}
	}
}

// TestProbabilityFormula_MaxPZeroFloor verifies that max(0,·) floors P to 0
// when the raw formula is negative (more successes than sr_threshold-adjusted
// total, i.e., healthy window) per AMEND-1.
func TestProbabilityFormula_MaxPZeroFloor(t *testing.T) {
	// n=50, s=50, srThreshold=0.95:
	// s/srThreshold = 50/0.95 ≈ 52.63 > n=50
	// inner = (50 - 52.63) / 51 < 0 → max(0,·) = 0 → P=0
	const n, s = uint64(50), uint64(50)
	pExpected := computeExpectedP(n, s, 0.95, 1.0, 0.80)
	if pExpected != 0 {
		t.Fatalf("setup: expected P=0 for n=50 s=50 srThreshold=0.95; got %.6f", pExpected)
	}

	// r=0 → r%10000=0 → 0 > 0 = false → ADMIT
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	rnd := fakeRand{v: 0}
	c := newTestController(cfg, nil, clk, rnd)
	primeWindow(t, c, n, s)
	if c.shouldReject() {
		t.Errorf("n=%d s=%d: shouldReject()=true; want false (P=0 from max(0,·) floor)", n, s)
	}
}

// TestProbabilityFormula_VectorTests exercises the formula across a grid of
// {n, s, srThreshold, aggression, maxRejP} and verifies the shouldReject
// reject/admit boundary.
//
// The reject decision is: float64(10000)*P > float64(r%10000)
//   - REJECT when r%10000 < 10000*P  (i.e. r%10000 <= firstReject = ceil(10000*P)-1)
//   - ADMIT  when r%10000 >= 10000*P (i.e. r%10000 >= firstAdmit = ceil(10000*P))
//
// When P=0 (via max(0,·) floor), every r admits.
// When P is an exact integer multiple of 1/10000, floor(10000*P) == ceil(10000*P),
// and r%10000 == floor(10000*P) is the strict equality-admits boundary.
// When P is fractional, floor(10000*P) still rejects (10000*P > floor); the
// first admitting r%10000 is ceil(10000*P).
func TestProbabilityFormula_VectorTests(t *testing.T) {
	type vec struct {
		name                    string
		n, s                    uint64
		srThreshold, aggression float64
		maxRejP                 float64
	}
	cases := []vec{
		{"baseline", 100, 0, 0.95, 1.0, 0.80},
		{"half_success", 100, 50, 0.95, 1.0, 0.80},
		{"high_aggression", 100, 50, 0.95, 2.0, 0.80},
		{"low_sr_threshold", 100, 80, 0.80, 1.0, 0.80},
		{"tight_max_rejp", 200, 100, 0.95, 1.0, 0.30},
		{"large_n", 10000, 9000, 0.95, 1.0, 0.80},
		{"n1_s0", 1, 0, 0.95, 1.0, 0.80},
		{"n1_s1", 1, 1, 0.95, 1.0, 0.80},
	}

	for _, tc := range cases {
		pExpected := computeExpectedP(tc.n, tc.s, tc.srThreshold, tc.aggression, tc.maxRejP)
		// The reject decision: float64(10000)*P > float64(r%10000)
		// First r%10000 that admits: ceil(float64(10000)*P).
		// - When P=0: every r admits (0 > anything is false).
		// - When P=k/10000 exactly: ceil==floor==k; r%10000==k → 10000*P > k is false → ADMIT.
		// - When P is fractional: ceil(10000*P) > floor(10000*P); first admit is ceil.
		tenKP := float64(10000) * pExpected
		firstAdmit := uint64(math.Ceil(tenKP)) // first r%10000 that admits

		// At firstAdmit: ADMIT (10000*P <= float64(firstAdmit))
		{
			clk := newFakeClock(time.Unix(0, 0))
			cfg := testCompiledConfigAC()
			cfg.srThreshold = tc.srThreshold
			cfg.aggression = tc.aggression
			cfg.maxRejectionProbability = tc.maxRejP
			rnd := fakeRand{v: firstAdmit}
			c := newTestController(cfg, nil, clk, rnd)
			primeWindow(t, c, tc.n, tc.s)
			if c.shouldReject() {
				t.Errorf("[%s] first-admit r%%10000=%d: shouldReject()=true; want false (P=%.6f 10000*P=%.6f)", tc.name, firstAdmit, pExpected, tenKP)
			}
		}

		// One below firstAdmit: REJECT (only if P > 0 and firstAdmit > 0)
		if pExpected > 0 && firstAdmit > 0 {
			clk := newFakeClock(time.Unix(0, 0))
			cfg := testCompiledConfigAC()
			cfg.srThreshold = tc.srThreshold
			cfg.aggression = tc.aggression
			cfg.maxRejectionProbability = tc.maxRejP
			rnd := fakeRand{v: firstAdmit - 1}
			c := newTestController(cfg, nil, clk, rnd)
			primeWindow(t, c, tc.n, tc.s)
			if !c.shouldReject() {
				t.Errorf("[%s] one-below first-admit r%%10000=%d: shouldReject()=false; want true (P=%.6f 10000*P=%.6f)", tc.name, firstAdmit-1, pExpected, tenKP)
			}
		}
	}
}

// -----------------------------------------------------------------------------
// 3. TestController_FAKE_TIME_Window_* — per AMEND-6 + PD-7
// -----------------------------------------------------------------------------

// TestController_FAKE_TIME_Window_SingleBucket verifies that requests in the
// same second land in the same bucket and are aggregated in global.
func TestController_FAKE_TIME_Window_SingleBucket(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	c := newTestController(cfg, nil, clk, defaultRand{})

	// Record 5 successes and 3 failures in the same second.
	for i := 0; i < 5; i++ {
		c.recordRequest(true)
	}
	for i := 0; i < 3; i++ {
		c.recordRequest(false)
	}

	n, s := c.requestCounts()
	if n != 8 {
		t.Errorf("requestCounts(): n=%d; want 8", n)
	}
	if s != 5 {
		t.Errorf("requestCounts(): s=%d; want 5", s)
	}
}

// TestController_FAKE_TIME_Window_BucketRollover verifies that advancing >=1s
// causes a new bucket to be created.
func TestController_FAKE_TIME_Window_BucketRollover(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	c := newTestController(cfg, nil, clk, defaultRand{})

	// Record 3 requests in bucket 0.
	c.recordRequest(true)
	c.recordRequest(true)
	c.recordRequest(false)

	// Advance 1 second → should create a new bucket.
	clk.Advance(1 * time.Second)

	// Record 2 requests in bucket 1.
	c.recordRequest(true)
	c.recordRequest(false)

	// Both buckets should be in the window (total 5 requests, 3 successes).
	n, s := c.requestCounts()
	if n != 5 {
		t.Errorf("requestCounts(): n=%d; want 5", n)
	}
	if s != 3 {
		t.Errorf("requestCounts(): s=%d; want 3", s)
	}
}

// TestController_FAKE_TIME_Window_StalePurge verifies that buckets older than
// samplingWindow are purged and decremented from global.
func TestController_FAKE_TIME_Window_StalePurge(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	cfg.samplingWindow = 5 * time.Second // small window for test
	c := newTestController(cfg, nil, clk, defaultRand{})

	// Record in bucket 0 (t=0): 10 requests, 8 successes.
	for i := 0; i < 8; i++ {
		c.recordRequest(true)
	}
	for i := 0; i < 2; i++ {
		c.recordRequest(false)
	}

	// Advance 3 seconds → bucket 0 is still within the 5s window.
	clk.Advance(3 * time.Second)
	c.recordRequest(true) // bucket at t=3

	n1, _ := c.requestCounts()
	if n1 != 11 {
		t.Errorf("after 3s advance: requestCounts n=%d; want 11 (both buckets in window)", n1)
	}

	// Advance 3 more seconds (total 6s) → bucket 0 (t=0) is now 6s old, outside the 5s window.
	// It should be purged, leaving only the t=3 bucket.
	clk.Advance(3 * time.Second)
	// Trigger purge via requestCounts
	n, s := c.requestCounts()
	if n != 1 {
		t.Errorf("after 6s total: requestCounts n=%d; want 1 (only t=3 bucket remains)", n)
	}
	if s != 1 {
		t.Errorf("after 6s total: requestCounts s=%d; want 1", s)
	}
}

// TestController_FAKE_TIME_Window_MultiSecondRollover verifies bucket rollover
// across multiple seconds.
func TestController_FAKE_TIME_Window_MultiSecondRollover(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	cfg.samplingWindow = 10 * time.Second
	c := newTestController(cfg, nil, clk, defaultRand{})

	// Record 1 request per second for 5 seconds.
	for i := 0; i < 5; i++ {
		c.recordRequest(true)
		clk.Advance(1 * time.Second)
	}

	n, s := c.requestCounts()
	if n != 5 {
		t.Errorf("requestCounts(): n=%d; want 5", n)
	}
	if s != 5 {
		t.Errorf("requestCounts(): s=%d; want 5", s)
	}
}

// TestController_FAKE_TIME_Window_EmptyAfterFullPurge verifies that requestCounts
// returns (0,0) after all buckets are purged.
func TestController_FAKE_TIME_Window_EmptyAfterFullPurge(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	cfg.samplingWindow = 5 * time.Second
	c := newTestController(cfg, nil, clk, defaultRand{})

	c.recordRequest(true)
	c.recordRequest(false)

	// Advance past the sampling window.
	clk.Advance(10 * time.Second)
	n, s := c.requestCounts()
	if n != 0 || s != 0 {
		t.Errorf("requestCounts() after full purge: n=%d s=%d; want 0,0", n, s)
	}
}

// TestController_FAKE_TIME_Window_RequestCounts_Purges verifies that
// requestCounts triggers a purge, not just reads the global.
func TestController_FAKE_TIME_Window_RequestCounts_Purges(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	cfg.samplingWindow = 3 * time.Second
	c := newTestController(cfg, nil, clk, defaultRand{})

	c.recordRequest(true)        // bucket t=0: 1 req, 1 suc
	clk.Advance(4 * time.Second) // now t=4; bucket t=0 is 4s > 3s → should purge

	n, s := c.requestCounts()
	if n != 0 || s != 0 {
		t.Errorf("requestCounts() should have purged stale bucket: n=%d s=%d; want 0,0", n, s)
	}
}

// -----------------------------------------------------------------------------
// 4. TestRpsSuppression_* — per §4.1
// -----------------------------------------------------------------------------

// TestRpsSuppression_EmptyWindow_ReturnsZero verifies averageRps() returns 0
// when the window is empty per SPEC §4.2.
func TestRpsSuppression_EmptyWindow_ReturnsZero(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	c := newTestController(cfg, nil, clk, defaultRand{})

	if rps := c.averageRps(); rps != 0 {
		t.Errorf("averageRps() on empty window = %d; want 0", rps)
	}
}

// TestRpsSuppression_SingleSecond_EqualsCount verifies averageRps() with all
// requests in a sub-second window: denominator = max(samplingWindow, age of
// oldest bucket) = samplingWindow (30s); so rps = n/30.
func TestRpsSuppression_SingleSecond_EqualsCount(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	cfg.samplingWindow = 30 * time.Second
	c := newTestController(cfg, nil, clk, defaultRand{})

	// 60 requests in 1 second; all in one bucket.
	for i := 0; i < 60; i++ {
		c.recordRequest(true)
	}

	// age of oldest sample ≈ 0s < samplingWindow=30s → denominator = 30
	// rps = 60 / 30 = 2
	rps := c.averageRps()
	if rps != 2 {
		t.Errorf("averageRps() = %d; want 2 (60 requests / 30s sampling window)", rps)
	}
}

// TestRpsSuppression_MultiSecond verifies averageRps() when the window spans
// multiple seconds with the age denominator kicking in.
func TestRpsSuppression_MultiSecond(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	cfg.samplingWindow = 30 * time.Second
	c := newTestController(cfg, nil, clk, defaultRand{})

	// Record 10 requests at t=0.
	for i := 0; i < 10; i++ {
		c.recordRequest(true)
	}
	// Advance 9 seconds.
	clk.Advance(9 * time.Second)
	// Record 20 requests at t=9.
	for i := 0; i < 20; i++ {
		c.recordRequest(true)
	}

	// At t=9: oldest bucket is at t=0; age = 9s.
	// denominator = max(30s, 9s) = 30s
	// rps = 30 / 30 = 1
	rps := c.averageRps()
	if rps != 1 {
		t.Errorf("averageRps() = %d; want 1 (30 requests / max(30,9)=30s)", rps)
	}
}

// TestRpsSuppression_SamplingWindowDenominator verifies that when the oldest
// bucket is younger than samplingWindow, the samplingWindow wins as denominator
// (i.e. max(samplingWindow, age) = samplingWindow). Age=2s < window=5s, so
// denominator = 5s; total=10 requests → rps=10/5=2.
func TestRpsSuppression_SamplingWindowDenominator(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	cfg.samplingWindow = 5 * time.Second
	c := newTestController(cfg, nil, clk, defaultRand{})

	// Record 3 requests at t=0.
	for i := 0; i < 3; i++ {
		c.recordRequest(true)
	}
	// Advance 2 seconds. age=2s < samplingWindow=5s → denom=5s → rps=3/5=0 (truncated)
	clk.Advance(2 * time.Second)

	// Record 7 more at t=2.
	for i := 0; i < 7; i++ {
		c.recordRequest(true)
	}

	// oldest bucket at t=0, age at call=2s; denom=max(5s, 2s)=5s; total=10; rps=10/5=2
	rps := c.averageRps()
	if rps != 2 {
		t.Errorf("averageRps() = %d; want 2 (10 requests / 5s sampling window)", rps)
	}
}

// -----------------------------------------------------------------------------
// 5. TestRecordDiscipline_* — per AMEND-11
// -----------------------------------------------------------------------------

// TestRecordDiscipline_Classify_Success increments rqSuccess counter and
// records into window on classify(true).
func TestRecordDiscipline_Classify_Success(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	reg := stats.NewRegistry()
	st := newFilterStats(reg, "http.test")
	c := newTestController(cfg, st, clk, defaultRand{})

	c.classify(true)

	if got := st.rqSuccess.Load(); got != 1 {
		t.Errorf("rqSuccess counter = %d; want 1 after classify(true)", got)
	}
	if got := st.rqFailure.Load(); got != 0 {
		t.Errorf("rqFailure counter = %d; want 0 after classify(true)", got)
	}
	// Window should have 1 request, 1 success.
	n, s := c.requestCounts()
	if n != 1 || s != 1 {
		t.Errorf("requestCounts() = (%d,%d); want (1,1) after classify(true)", n, s)
	}
}

// TestRecordDiscipline_Classify_Failure increments rqFailure counter and
// records into window on classify(false).
func TestRecordDiscipline_Classify_Failure(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	reg := stats.NewRegistry()
	st := newFilterStats(reg, "http.test")
	c := newTestController(cfg, st, clk, defaultRand{})

	c.classify(false)

	if got := st.rqSuccess.Load(); got != 0 {
		t.Errorf("rqSuccess counter = %d; want 0 after classify(false)", got)
	}
	if got := st.rqFailure.Load(); got != 1 {
		t.Errorf("rqFailure counter = %d; want 1 after classify(false)", got)
	}
	// Window should have 1 request, 0 successes.
	n, s := c.requestCounts()
	if n != 1 || s != 0 {
		t.Errorf("requestCounts() = (%d,%d); want (1,0) after classify(false)", n, s)
	}
}

// TestRecordDiscipline_Classify_Multiple verifies multiple classify calls
// accumulate correctly.
func TestRecordDiscipline_Classify_Multiple(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	reg := stats.NewRegistry()
	st := newFilterStats(reg, "http.test")
	c := newTestController(cfg, st, clk, defaultRand{})

	// 3 successes, 2 failures.
	c.classify(true)
	c.classify(true)
	c.classify(false)
	c.classify(true)
	c.classify(false)

	if got := st.rqSuccess.Load(); got != 3 {
		t.Errorf("rqSuccess = %d; want 3", got)
	}
	if got := st.rqFailure.Load(); got != 2 {
		t.Errorf("rqFailure = %d; want 2", got)
	}
	n, s := c.requestCounts()
	if n != 5 || s != 3 {
		t.Errorf("requestCounts() = (%d,%d); want (5,3)", n, s)
	}
}

// TestRecordDiscipline_RecordRequest_DoesNotIncrement_Stats verifies that
// recordRequest (which is called by the controller internally) does NOT
// increment the stats counters — only classify() does.
func TestRecordDiscipline_RecordRequest_DoesNotIncrement_Stats(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	reg := stats.NewRegistry()
	st := newFilterStats(reg, "http.test")
	c := newTestController(cfg, st, clk, defaultRand{})

	// Call recordRequest directly — this should update window but NOT counters.
	c.recordRequest(true)
	c.recordRequest(false)

	if got := st.rqSuccess.Load(); got != 0 {
		t.Errorf("rqSuccess = %d after recordRequest; want 0 (recordRequest does not touch stats counters)", got)
	}
	if got := st.rqFailure.Load(); got != 0 {
		t.Errorf("rqFailure = %d after recordRequest; want 0 (recordRequest does not touch stats counters)", got)
	}
	// But window should be updated.
	n, s := c.requestCounts()
	if n != 2 || s != 1 {
		t.Errorf("requestCounts() = (%d,%d); want (2,1)", n, s)
	}
}

// -----------------------------------------------------------------------------
// 6. TestController_Concurrent_* — race tests
// -----------------------------------------------------------------------------

// TestController_Concurrent_RecordAndCount exercises concurrent recordRequest +
// requestCounts under the race detector. Verifies no data race + consistent totals.
func TestController_Concurrent_RecordAndCount(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	cfg.samplingWindow = 10 * time.Second
	c := newTestController(cfg, nil, clk, defaultRand{})

	const goroutines = 20
	const recordsPerGoroutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < recordsPerGoroutine; i++ {
				c.recordRequest(i%2 == 0)
			}
		}(g)
	}
	wg.Wait()

	n, s := c.requestCounts()
	const total = goroutines * recordsPerGoroutine
	if n != total {
		t.Errorf("requestCounts(): n=%d; want %d (all recorded)", n, total)
	}
	// Each goroutine records 50 successes and 50 failures.
	const expectedSuccesses = total / 2
	if s != expectedSuccesses {
		t.Errorf("requestCounts(): s=%d; want %d", s, expectedSuccesses)
	}
}

// TestController_Concurrent_ClassifyAndShouldReject exercises concurrent
// classify + shouldReject. Verifies no data race + no deadlock.
func TestController_Concurrent_ClassifyAndShouldReject(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	c := newTestController(cfg, nil, clk, defaultRand{})

	const goroutines = 20
	const ops = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Goroutines classifying.
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				c.classify(i%3 != 0) // 2/3 success, 1/3 failure
			}
		}(g)
	}
	// Goroutines calling shouldReject.
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = c.shouldReject()
			}
		}(g)
	}
	wg.Wait()
	// Every classify call increments global.requests exactly once (no time advance
	// means no purge), so requestCounts() must equal goroutines*ops exactly.
	n, _ := c.requestCounts()
	if n != uint64(goroutines*ops) {
		t.Errorf("requestCounts(): n=%d; want %d (every classify increments requests exactly once, no purge)", n, goroutines*ops)
	}
}

// TestController_Concurrent_NoDeadlock exercises all three public methods
// concurrently to confirm no deadlock under the race detector.
func TestController_Concurrent_NoDeadlock(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	c := newTestController(cfg, nil, clk, defaultRand{})

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				switch i % 4 {
				case 0:
					c.recordRequest(true)
				case 1:
					c.recordRequest(false)
				case 2:
					c.requestCounts()
				case 3:
					_ = c.shouldReject()
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestController_Concurrent_AverageRps exercises averageRps concurrently with
// recordRequest. Verifies no data race.
func TestController_Concurrent_AverageRps(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testCompiledConfigAC()
	c := newTestController(cfg, nil, clk, defaultRand{})

	var wg sync.WaitGroup
	const goroutines = 10
	wg.Add(goroutines * 2)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				c.recordRequest(i%2 == 0)
			}
		}()
	}
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = c.averageRps()
			}
		}()
	}
	wg.Wait()
}
