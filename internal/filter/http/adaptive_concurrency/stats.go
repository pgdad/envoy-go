package adaptive_concurrency

// stats.go — 7-name wire-exact filterStats per phase-21 SPEC §6.6 + AMEND-3
// + ADR-0059 §Decision AMENDMENT (float-valued-gauge int64 encoding
// convention).
//
// # Stat-surface roster (7 names: 1 counter + 6 gauges)
//
// Wire-equivalent to upstream Envoy's gradient_controller stat surface
// (per phase-21 SPEC §6.6 + AMEND-3 C2 stat-prefix template):
//
//   - rq_blocked                  counter (int64 monotonic)
//   - concurrency_limit           gauge   (int64 raw uint32 value)
//   - gradient                    gauge   (int64 ×1000 per ADR-0059 AMENDMENT)
//   - burst_queue_size            gauge   (int64 signed)
//   - sample_rtt_msecs            gauge   (int64 nanoseconds per AMEND-3 C3 —
//     envoy-go-strict departure from upstream's milliseconds; NAME byte-exact)
//   - min_rtt_msecs               gauge   (int64 nanoseconds per AMEND-3 C3 —
//     same envoy-go-strict departure)
//   - min_rtt_calculation_active  gauge   (int64 0/1 via stats.BoolToInt)
//
// # HCM-rooted stat-prefix per AMEND-3 C2 + ADR-0143 SN2-reuse
//
// The stat-prefix shape mirrors the established §9 family-row convention
// (extauthz / extproc / jwt_authn / oauth2 precedent):
//
//	http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.<stat>
//
// The `http.<HCM_stat_prefix>` portion is HCM-injected via the hcmPrefix
// constructor argument (e.g., "http.ingress_http" for an HCM with
// stat_prefix="ingress_http") per ADR-0143 SN2-reuse.
//
// # Compile-time stat-name guards per planner-time D5 + SPEC §6.6
//
// Two-layer guard:
//
//  1. Per-stat `const statName*` declarations pin the byte-exact wire
//     names at compile time. A regression on any wire name surfaces
//     immediately at build time (the constructor uses these constants).
//
//  2. The `TestStatNames_Equal_*` Group-9 assertion tests (in
//     adaptive_concurrency_test.go at Task 9) pin each constant to its
//     string literal — preventing a future refactor from silently
//     renaming the constant alongside the string literal.
//
// # Float-valued-gauge int64 encoding convention per ADR-0059 §Decision AMENDMENT
//
// Three of the six gauges carry non-int64-natural values. The encoding
// convention layered atop the unchanged *stats.Gauge primitive (int64-only
// per ADR-0059 §Decision):
//
//   - Time-typed (sample_rtt_msecs + min_rtt_msecs): encoded as int64
//     nanoseconds direct via Gauge.Set(d.Nanoseconds()). Operators
//     divide by 1e9 to recover seconds. envoy-go-strict departure from
//     upstream's milliseconds — the stat NAMES preserve upstream
//     byte-exact (`sample_rtt_msecs` / `min_rtt_msecs`) per
//     `ALL_GRADIENT_CONTROLLER_STATS`; the per-metric # HELP text
//     (Rule SN6) documents the unit divergence. Documented as
//     envoy-go-strict departure record at BEHAVIOR_CONTRACT.md
//     (phase-21 SPEC §13 item C4; lands at Task 13).
//
//   - Ratio-typed (gradient): encoded as int64 ×1000 via
//     Gauge.Set(int64(gradient * 1000)); gradient=1.5 → stored 1500.
//     Operators divide by 1000 to recover the ratio. The ×1000 scale
//     matches upstream Envoy's `gradient_controller.cc:176` integer-
//     millis convention (`stats_.gradient_.set(gradient * 1000)`).
//
//   - Bool-typed (min_rtt_calculation_active): encoded as int64 0/1 via
//     Gauge.Set(stats.BoolToInt(b)) — sibling helper at internal/stats/
//     conv.go. The encoding callsite lives at controller.go's
//     onMinRTTRecalcStart/End (lands at Task 3).
//
// The 7-name surface allocates unconditionally at filter buildCompiledConfig
// time. Predeclared empty gauges/counters give operators a scrape-stable
// stat surface (Prometheus best practice; mirrors the established §9
// family-row discipline).

import (
	"github.com/pgdad/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Stat-name compile-time constants per planner-time D5 + SPEC §6.6 + AMEND-3.
// -----------------------------------------------------------------------------

// statNameRqBlocked pins the byte-exact upstream wire name for the
// "request blocked by concurrency limit" counter per SPEC §6.6 + AMEND-3.
// Incremented per request rejected at decode-headers when the in-flight
// count is >= concurrency_limit (the 503 short-circuit per SPEC §4.6).
// Consumed by the controller's forwardingDecision Block-path (Task 3
// controller.go landing) — `//nolint:unused` cleared at that landing.
const statNameRqBlocked = "rq_blocked"

// statNameConcurrencyLimit pins the byte-exact upstream wire name for the
// current concurrency-limit gauge per SPEC §6.6 + AMEND-3. int64 raw
// uint32 value — Set via Gauge.Set(int64(currentLimit)). Updated by the
// controller's recalc tick per SPEC §4.4. Consumer landed at Task 3.
const statNameConcurrencyLimit = "concurrency_limit"

// statNameGradient pins the byte-exact upstream wire name for the
// gradient gauge per SPEC §6.6 + AMEND-3. int64 ×1000 per ADR-0059
// §Decision AMENDMENT (ratio-typed encoding); operators divide by 1000.
// Matches upstream `gradient_controller.cc:176`. Consumer landed at Task 3.
const statNameGradient = "gradient"

// statNameBurstQueueSize pins the byte-exact upstream wire name for the
// burst-queue-size gauge per SPEC §6.6 + AMEND-3. int64 signed value;
// represents the sqrt-based burst-queue derivative term in the new-limit
// formula. Consumer landed at Task 3.
const statNameBurstQueueSize = "burst_queue_size"

// statNameSampleRTTMsecs pins the byte-exact upstream wire name for the
// sample-RTT gauge per SPEC §6.6 + AMEND-3 C3. int64 nanoseconds per
// ADR-0059 §Decision AMENDMENT (time-typed encoding — envoy-go-strict
// departure from upstream's milliseconds; the NAME preserves the
// upstream-byte-exact `sample_rtt_msecs` per `ALL_GRADIENT_CONTROLLER_STATS`).
// Consumer landed at Task 3.
const statNameSampleRTTMsecs = "sample_rtt_msecs"

// statNameMinRTTMsecs pins the byte-exact upstream wire name for the
// minimum-RTT gauge per SPEC §6.6 + AMEND-3 C3. int64 nanoseconds per
// ADR-0059 §Decision AMENDMENT (same time-typed encoding convention as
// sample_rtt_msecs); NAME preserves upstream-byte-exact `min_rtt_msecs`.
// Consumer landed at Task 3.
const statNameMinRTTMsecs = "min_rtt_msecs"

// statNameMinRTTCalculationActive pins the byte-exact upstream wire
// name for the min-RTT-recalc-in-progress bool-gauge per SPEC §6.6 +
// AMEND-3. int64 0/1 per ADR-0059 §Decision AMENDMENT (bool-typed
// encoding) — Set via Gauge.Set(stats.BoolToInt(active)). Consumer landed
// at Task 3.
const statNameMinRTTCalculationActive = "min_rtt_calculation_active"

// -----------------------------------------------------------------------------
// filterStats — 7-name wire-exact stat-surface holder per SPEC §6.6 + AMEND-3.
// -----------------------------------------------------------------------------

// filterStats holds the 7-name HCM-rooted stat surface for one
// adaptive_concurrency filter instance per phase-21 SPEC §6.6 + AMEND-3.
//
// 1 counter (rq_blocked) + 6 gauges. Gauge encoding follows ADR-0059
// §Decision AMENDMENT (float-valued-gauge int64 encoding convention):
//
//   - Time-typed: int64 nanoseconds direct (envoy-go-strict departure
//     from upstream ms; NAME byte-exact upstream).
//   - Ratio-typed: int64 ×1000 (gradient bounded [0.5, 2.0]).
//   - Bool-typed: int64 0/1 via stats.BoolToInt helper at conv.go.
//
// Consumer landed at Task 3 (controller.go); Task 5 filter.go will consume
// the rqBlocked counter on the decode-headers 503 path.
type filterStats struct {
	// rqBlocked — request rejected at decode-headers (in-flight >= limit)
	// per SPEC §4.6 503-short-circuit path.
	rqBlocked *stats.Counter

	// concurrencyLimit — current concurrency limit per SPEC §4.4 recalc.
	// int64 raw uint32 value.
	concurrencyLimit *stats.Gauge

	// gradient — current gradient ratio per SPEC §4.4. int64 ×1000 per
	// ADR-0059 §Decision AMENDMENT (matches upstream gradient_controller.cc:176).
	gradient *stats.Gauge

	// burstQueueSize — sqrt-based burst-queue derivative term in the
	// new-limit formula per SPEC §4.4. int64 signed.
	burstQueueSize *stats.Gauge

	// sampleRTTMsecs — latest sample-RTT per SPEC §4.4. int64 nanoseconds
	// per ADR-0059 §Decision AMENDMENT (envoy-go-strict departure; NAME
	// preserves upstream-byte-exact `sample_rtt_msecs`).
	sampleRTTMsecs *stats.Gauge

	// minRTTMsecs — last-measured min-RTT per SPEC §4.4. int64 nanoseconds
	// per ADR-0059 §Decision AMENDMENT (same time-typed encoding as
	// sampleRTTMsecs).
	minRTTMsecs *stats.Gauge

	// minRTTCalculationActive — min-RTT recalc-in-progress flag per SPEC
	// §4.5. int64 0/1 per ADR-0059 §Decision AMENDMENT.
	minRTTCalculationActive *stats.Gauge
}

// -----------------------------------------------------------------------------
// newFilterStats — 7-name registration per AMEND-3 C2 + ADR-0143 SN2-reuse.
// -----------------------------------------------------------------------------

// newFilterStats constructs the 7-name roster under the HCM-injected
// stat prefix per AMEND-3 C2:
//
//	http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.<stat>
//
// hcmPrefix is the HCM-rooted prefix supplied by the factory context at
// filter construction (e.g., "http.ingress_http" for an HCM with
// stat_prefix="ingress_http") per ADR-0143 SN2-reuse. The constructor
// appends `.adaptive_concurrency.gradient_controller.` to hcmPrefix
// before each stat name.
//
// All 7 stats allocate unconditionally for scrape stability (Prometheus
// best practice; mirrors the established §9 family-row discipline). The
// nil-tolerance contract lives at the caller's `if ctx.Stats != nil`
// gate per ADR-0085 — this helper unconditionally dereferences `reg`.
//
// Consumer landed at Task 3 (controller.go); Task 9 boot-registration will
// wire this constructor from the HCM build path.
func newFilterStats(reg *stats.Registry, hcmPrefix string) *filterStats {
	p := hcmPrefix + ".adaptive_concurrency.gradient_controller."
	return &filterStats{
		rqBlocked:               reg.NewCounter(p + statNameRqBlocked),
		concurrencyLimit:        reg.NewGauge(p + statNameConcurrencyLimit),
		gradient:                reg.NewGauge(p + statNameGradient),
		burstQueueSize:          reg.NewGauge(p + statNameBurstQueueSize),
		sampleRTTMsecs:          reg.NewGauge(p + statNameSampleRTTMsecs),
		minRTTMsecs:             reg.NewGauge(p + statNameMinRTTMsecs),
		minRTTCalculationActive: reg.NewGauge(p + statNameMinRTTCalculationActive),
	}
}
