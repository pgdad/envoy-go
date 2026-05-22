package admission_control

// stats.go — 3-counter wire-exact filterStats per phase-23 SPEC §6.6 + AMEND-3.
//
// # Stat-surface roster (3 names: counters ONLY — no gauges per AMEND-3)
//
// Wire-equivalent to upstream Envoy's admission_control stat surface per
// ALL_ADMISSION_CONTROL_STATS(COUNTER) macro at admission_control.h:35-38:
//
//   - rq_rejected  counter (request rejected by admission control)
//   - rq_success   counter (upstream response classified as success)
//   - rq_failure   counter (upstream response classified as failure)
//
// NO gauges, NO histograms (AMEND-3: the macro is COUNTER-only; upstream
// publishes exactly 3 counters). Project stat count 107 → 110 (+3 counters).
//
// # HCM-rooted stat-prefix per AMEND-3 + ADR-0143 SN2-reuse
//
// The stat-prefix shape mirrors the §9 family-row convention:
//
//	http.<HCM_stat_prefix>.admission_control.<stat>
//
// The `http.<HCM_stat_prefix>` portion is HCM-injected via the hcmPrefix
// constructor argument (e.g., "http.ingress_http" for an HCM with
// stat_prefix="ingress_http") per ADR-0143 SN2-reuse. The constructor
// appends ".admission_control." to hcmPrefix before each stat name.
// The literal infix "admission_control." per config.cc:29 (no sub-infix
// like gradient_controller — admission_control has a flat stat namespace).
//
// # Compile-time stat-name guards per SPEC §6.6 + AMEND-3
//
// Two-layer guard:
//
//  1. Package-level const declarations pin the byte-exact wire names at
//     compile time. A regression on any wire name surfaces at build time
//     (the constructor uses these constants).
//
//  2. The TestStatNames_Equal_* assertion tests in stats_test.go (this Task 3)
//     pin each constant to its string literal — preventing a future refactor
//     from silently renaming the constant alongside the string literal. Task 3
//     OWNS the stat-name byte-exact assertion; Task 5's admission_control_test.go
//     does NOT re-assert stat names.

import (
	"github.com/esalaine/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Stat-name compile-time constants per SPEC §6.6 + AMEND-3.
// -----------------------------------------------------------------------------

// statNameRqRejected pins the byte-exact upstream wire name for the
// "request rejected by admission control" counter per SPEC §6.6 + AMEND-3.
// Incremented per request rejected at decode-headers (the 503 short-circuit
// per SPEC §4.1, admission_control.cc:100). Consumed by the filter's
// DecodeHeaders reject path (Task 5 landing).
const statNameRqRejected = "rq_rejected"

// statNameRqSuccess pins the byte-exact upstream wire name for the
// "upstream response classified as success" counter per SPEC §6.6 + AMEND-3.
// Incremented when the encode-side classification records a success per
// SPEC §4.4 (admission_control.h:111). Consumed by the filter's encode
// classification path (Task 6 landing).
const statNameRqSuccess = "rq_success"

// statNameRqFailure pins the byte-exact upstream wire name for the
// "upstream response classified as failure" counter per SPEC §6.6 + AMEND-3.
// NOTE: the third counter is rq_failure, NOT rq_error (AMEND-3 correction
// of the BRAINSTORM hypothesis — verified via ALL_ADMISSION_CONTROL_STATS
// macro at admission_control.h:35-38). Incremented when the encode-side
// classification records a failure (admission_control.h:116). Consumed by
// the filter's encode classification path (Task 6 landing).
const statNameRqFailure = "rq_failure"

// -----------------------------------------------------------------------------
// filterStats — 3-counter wire-exact stat-surface holder per SPEC §6.6 + AMEND-3.
// -----------------------------------------------------------------------------

// filterStats holds the 3-counter HCM-rooted stat surface for one
// admission_control filter instance per phase-23 SPEC §6.6 + AMEND-3.
// COUNTER-ONLY (no gauges, no histograms — upstream ALL_ADMISSION_CONTROL_STATS
// macro is COUNTER-only). Project stat count: 107 → 110 (+3 counters).
type filterStats struct {
	// rqRejected — request rejected at decode-headers (probabilistic shedding)
	// per SPEC §4.1 admission_control.cc:100.
	rqRejected *stats.Counter

	// rqSuccess — upstream response classified as success per SPEC §4.4
	// (HTTP status < 500 default; gRPC 11-code default per AMEND-5).
	rqSuccess *stats.Counter

	// rqFailure — upstream response classified as failure per SPEC §4.4
	// (complement of rqSuccess; NOTE: rq_failure, NOT rq_error per AMEND-3).
	rqFailure *stats.Counter
}

// -----------------------------------------------------------------------------
// newFilterStats — 3-counter registration per AMEND-3 + ADR-0143 SN2-reuse.
// -----------------------------------------------------------------------------

// newFilterStats constructs the 3-counter roster under the HCM-injected
// stat prefix per AMEND-3:
//
//	http.<HCM_stat_prefix>.admission_control.<stat>
//
// hcmPrefix is the HCM-rooted prefix supplied by the factory context at
// filter construction (e.g., "http.ingress_http" for an HCM with
// stat_prefix="ingress_http") per ADR-0143 SN2-reuse. The constructor
// appends ".admission_control." to hcmPrefix before each stat name.
//
// All 3 counters allocate unconditionally for scrape stability (Prometheus
// best practice; mirrors the established §9 family-row discipline). The
// nil-tolerance contract lives at the caller's ctx.Stats != nil gate per
// ADR-0085 — this helper unconditionally dereferences reg.
//
// Consumed at Task 5 boot-registration where the filter factory wires this
// constructor from the HCM build path.
func newFilterStats(reg *stats.Registry, hcmPrefix string) *filterStats {
	p := hcmPrefix + ".admission_control."
	return &filterStats{
		rqRejected: reg.NewCounter(p + statNameRqRejected),
		rqSuccess:  reg.NewCounter(p + statNameRqSuccess),
		rqFailure:  reg.NewCounter(p + statNameRqFailure),
	}
}
