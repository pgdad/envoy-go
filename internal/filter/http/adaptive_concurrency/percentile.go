package adaptive_concurrency

// percentile.go — sorted-slice percentile aggregation helper per phase-21
// SPEC §6.8 + BRAINSTORM §8 item 4 carve-out + ADR-0186 §Decision sub-
// paragraph (envoy-go-strict departure: sorted-slice + index interpolation,
// NOT CircllHist; ≤ 1 bin-width divergence at the percentile boundary is
// acceptable per ADR-0186).
//
// Consumed intra-package by the gradient controller (Task 3 landing) at two
// callsites:
//
//   - gradientController.concurrencyUpdateTick() per SPEC §4.2 — the per-
//     tick sample-RTT aggregation (p = sampleAggregatePercentile, e.g. 0.5).
//   - gradientController.updateMinRTT() per SPEC §4.5 — the per-recalc-
//     window minRTT aggregation per AMEND-2 C1 (p =
//     sampleAggregatePercentile; NOT MIN as a naive reading of upstream
//     gradient_controller.cc:281 might suggest — see ADR-0186 §Decision +
//     AMEND-2 lemma C1).
//
// SPEC §12 item B5 RATIFIES at Task 7 per D15 — numeric outputs may diverge
// from upstream CircllHist by ≤ 1 bin-width at the percentile boundary; the
// vector tests in percentile_test.go assert exact outputs for the D10 roster.
//
// # Naming convention — package-public per SPEC §6.8
//
// SPEC §6.8 names the helper `Quantile`. The consumer (controller.go) lives
// in the same package so a lowercase identifier would technically suffice,
// but the SPEC shape + phase-20 oauth2 precedent (cookies.go's `Encode` /
// `Decode` are package-exported despite intra-package use; ADR-0114
// stylistic license) settles the casing at package-public.

import (
	"sort"
	"time"
)

// Quantile returns the p-quantile of samples (a slice of time.Duration
// values) using the sorted-slice + index-interpolation method per phase-21
// SPEC §6.8 + BRAINSTORM §8 item 4 carve-out (NOT CircllHist; ≤ 1 bin-width
// divergence acceptable per ADR-0186 §Decision).
//
// Algorithm:
//   - len(samples) == 0 → return 0 (no-panic edge case per D10 row 1).
//   - len(samples) == 1 → return samples[0] (per D10 row 2).
//   - p is clamped to [0.0, 1.0] (per D10 rows 6 + 7).
//   - The caller's samples slice is NOT mutated — Quantile copies before
//     sorting (per D10 row 8 invariant).
//   - sort ascending.
//   - idx := int(p * float64(len(sorted)-1)) — integer truncation; the ≤ 1
//     bin-width divergence vs CircllHist arises here at the boundary.
//   - return sorted[idx].
//
// Consumed by gradientController.concurrencyUpdateTick() per SPEC §4.2 (per-
// tick sample-RTT aggregation; p = sampleAggregatePercentile e.g. 0.5) and
// gradientController.updateMinRTT() per SPEC §4.5 (per-recalc-window minRTT
// aggregation per AMEND-2 C1; p = sampleAggregatePercentile NOT MIN).
//
// SPEC §12 item B5 RATIFIED via D15: numeric outputs may diverge from
// upstream CircllHist by ≤ 1 bin-width at the percentile boundary (envoy-go-
// strict departure per BRAINSTORM §8 item 4).
func Quantile(samples []time.Duration, p float64) time.Duration {
	if len(samples) <= 1 {
		return quantileInPlace(samples, p) // 0- and 1-element paths never sort
	}
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	return quantileInPlace(sorted, p)
}

// quantileInPlace is Quantile without the defensive copy: it sorts samples
// IN PLACE and returns the p-quantile. Used by the gradientController's two
// production call sites (concurrencyUpdateTick + updateMinRTTLocked), which
// reset the source slice to [:0] immediately after the call — the copy
// Quantile makes to preserve the caller's ordering is pure waste there (one
// alloc + O(n) copy per tick / per window close). Same numeric result as
// Quantile; Quantile remains the exported test-vector contract (D10 row 8's
// caller-slice-not-mutated invariant applies to Quantile only).
func quantileInPlace(samples []time.Duration, p float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	if len(samples) == 1 {
		return samples[0]
	}
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	idx := int(p * float64(len(samples)-1))
	return samples[idx]
}
