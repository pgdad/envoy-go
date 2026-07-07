package adaptive_concurrency

// percentile_test.go — vector + edge-case tests for the sorted-slice Quantile
// helper per phase-21 IMPL Task 7 + planner-time D10 (sorted-slice quantile
// edge-case enumeration LOCKED at this roster). SPEC §12 item B5 RATIFIES
// here per D15 (numeric outputs may diverge from upstream CircllHist by ≤ 1
// bin-width at the percentile boundary — envoy-go-strict departure per
// BRAINSTORM §8 item 4 + ADR-0186 §Decision).
//
// 8 D10 vector tests + 3 extra tail/wider-distribution vectors per implementer
// discretion (TestPercentile_SortedSlice_P95_TailVector +
// TestPercentile_SortedSlice_P99_TailVector +
// TestPercentile_SortedSlice_100Sample_Linear). Total stays within the SPEC
// §6.8 ~80-120 LoC envelope.

import (
	"testing"
	"time"
)

// D10 row 1 — Empty slice returns 0 (no-panic edge case).
func TestPercentile_SortedSlice_Empty(t *testing.T) {
	got := Quantile(nil, 0.5)
	if got != 0 {
		t.Errorf("Quantile(nil, 0.5) = %v; want 0", got)
	}
}

// D10 row 2 — Single sample returns that sample regardless of p.
func TestPercentile_SortedSlice_SingleSample(t *testing.T) {
	got := Quantile([]time.Duration{42 * time.Millisecond}, 0.5)
	if got != 42*time.Millisecond {
		t.Errorf("Quantile([42ms], 0.5) = %v; want 42ms", got)
	}
}

// D10 row 3 — P50 of [1ms,2ms,3ms,4ms,5ms] = 3ms (idx = int(0.5 * 4) = 2).
func TestPercentile_SortedSlice_P50_KnownSet(t *testing.T) {
	samples := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
		4 * time.Millisecond,
		5 * time.Millisecond,
	}
	got := Quantile(samples, 0.5)
	if got != 3*time.Millisecond {
		t.Errorf("Quantile([1,2,3,4,5]ms, 0.5) = %v; want 3ms", got)
	}
}

// D10 row 4 — P0 returns the min element after sorting unsorted input.
func TestPercentile_SortedSlice_P0_ReturnsMin(t *testing.T) {
	samples := []time.Duration{5, 3, 1, 4, 2} // intentionally unsorted
	got := Quantile(samples, 0.0)
	if got != 1 {
		t.Errorf("Quantile([5,3,1,4,2], 0.0) = %v; want 1 (min)", got)
	}
}

// D10 row 5 — P1.0 returns the max element after sorting unsorted input.
func TestPercentile_SortedSlice_P1_ReturnsMax(t *testing.T) {
	samples := []time.Duration{5, 3, 1, 4, 2}
	got := Quantile(samples, 1.0)
	if got != 5 {
		t.Errorf("Quantile([5,3,1,4,2], 1.0) = %v; want 5 (max)", got)
	}
}

// D10 row 6 — Negative p clamps to 0 → returns min.
func TestPercentile_SortedSlice_PNegative_ClampsToZero(t *testing.T) {
	samples := []time.Duration{10, 20, 30}
	got := Quantile(samples, -0.5)
	if got != 10 {
		t.Errorf("Quantile([10,20,30], -0.5) = %v; want 10 (clamped to 0; min)", got)
	}
}

// D10 row 7 — p > 1 clamps to 1 → returns max.
func TestPercentile_SortedSlice_PGreaterThanOne_ClampsToOne(t *testing.T) {
	samples := []time.Duration{10, 20, 30}
	got := Quantile(samples, 1.5)
	if got != 30 {
		t.Errorf("Quantile([10,20,30], 1.5) = %v; want 30 (clamped to 1; max)", got)
	}
}

// D10 row 8 — Caller's input slice MUST NOT be mutated (Quantile copies before
// sorting). This invariant matters because callers (the controller's
// concurrencyUpdateTick + updateMinRTT per SPEC §4.2 + §4.5) hand off the
// sample buffer + immediately reset it; if Quantile mutated in place, a
// concurrent reader would observe partially-sorted state.
func TestPercentile_SortedSlice_UnsortedInput_DoesNotMutate(t *testing.T) {
	samples := []time.Duration{
		5 * time.Millisecond,
		3 * time.Millisecond,
		1 * time.Millisecond,
	}
	orig := make([]time.Duration, len(samples))
	copy(orig, samples)
	_ = Quantile(samples, 0.5)
	for i := range samples {
		if samples[i] != orig[i] {
			t.Errorf("samples[%d] mutated: %v -> %v", i, orig[i], samples[i])
		}
	}
}

// Tail vector — P95 of a 20-sample linear distribution [1ms..20ms]:
// idx = int(0.95 * 19) = int(18.05) = 18; sorted[18] = 19ms.
func TestPercentile_SortedSlice_P95_TailVector(t *testing.T) {
	samples := make([]time.Duration, 20)
	for i := 0; i < 20; i++ {
		samples[i] = time.Duration(i+1) * time.Millisecond
	}
	got := Quantile(samples, 0.95)
	if got != 19*time.Millisecond {
		t.Errorf("Quantile([1..20]ms, 0.95) = %v; want 19ms (idx=18)", got)
	}
}

// Tail vector — P99 of an 11-sample linear distribution [1ms..11ms]:
// idx = int(0.99 * 10) = int(9.9) = 9; sorted[9] = 10ms.
func TestPercentile_SortedSlice_P99_TailVector(t *testing.T) {
	samples := []time.Duration{
		1 * time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond,
		4 * time.Millisecond, 5 * time.Millisecond, 6 * time.Millisecond,
		7 * time.Millisecond, 8 * time.Millisecond, 9 * time.Millisecond,
		10 * time.Millisecond, 11 * time.Millisecond,
	}
	got := Quantile(samples, 0.99)
	if got != 10*time.Millisecond {
		t.Errorf("Quantile([1..11]ms, 0.99) = %v; want 10ms (idx=9)", got)
	}
}

// Wider distribution — P50 of a 100-sample linear set [1ns..100ns]:
// idx = int(0.5 * 99) = 49; sorted[49] = 50ns. Confirms the integer-truncation
// boundary at a larger sample count (no rounding-up to 50).
func TestPercentile_SortedSlice_100Sample_Linear(t *testing.T) {
	samples := make([]time.Duration, 100)
	for i := 0; i < 100; i++ {
		samples[i] = time.Duration(i + 1)
	}
	got := Quantile(samples, 0.5)
	if got != 50 {
		t.Errorf("Quantile([1..100]ns, 0.5) = %v; want 50ns (idx=49)", got)
	}
}

// TestQuantileInPlace_MatchesQuantile verifies the in-place production
// variant returns the same value as the exported copy-first Quantile for a
// spread of sample shapes + percentiles (maintenance-pass item: the two
// gradientController call sites use quantileInPlace since they reset the
// source slice immediately after aggregating).
func TestQuantileInPlace_MatchesQuantile(t *testing.T) {
	cases := [][]time.Duration{
		nil,
		{7 * time.Millisecond},
		{5 * time.Millisecond, 1 * time.Millisecond, 3 * time.Millisecond},
		{40 * time.Millisecond, 60 * time.Millisecond, 80 * time.Millisecond, 20 * time.Millisecond},
	}
	for _, p := range []float64{-0.5, 0, 0.5, 0.95, 1, 2} {
		for _, samples := range cases {
			want := Quantile(samples, p) // computes on a copy; samples untouched
			in := make([]time.Duration, len(samples))
			copy(in, samples)
			if got := quantileInPlace(in, p); got != want {
				t.Errorf("quantileInPlace(%v, %v) = %v; Quantile = %v", samples, p, got, want)
			}
		}
	}
}
