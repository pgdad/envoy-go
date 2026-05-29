// Package adaptive_concurrency implements the envoy.filters.http.
// adaptive_concurrency HTTP filter under the 07.1 HTTP filter framework.
//
// Phase 21: canonical Envoy v1.37.2 Gradient-1 adaptive-concurrency filter
// per ADR-0186. Estimates minRTT from sampled request RTTs + continuously
// adjusts a per-HCM-instance concurrency limit to bound tail latency under
// load.
//
// # State machine (per SPEC §4 + ADR-0186 §Decision)
//
//   - Hot-path lock-free CAS on numRqOutstanding via atomic.Uint32.
//   - Cold-path sync.Mutex over latencySamples / minRTTSamples /
//     deferredLimitValue / consecutiveMinConcurrencySet.
//   - Periodic concurrency-update tick (cfg.concurrencyUpdateInterval).
//   - Periodic minRTT-recalc trigger (applyJitter(cfg.minRTTCalcInterval,
//     cfg.minRTTJitterPct)) + 5-consecutive-min forced-recalc per AMEND-2 C3.
//   - First-tick semantics per AMEND-2 C4: at construction the controller
//     immediately enters the minRTT sampling window pinned at minConcurrency.
//
// # 7-name HCM-rooted stat surface (per SPEC §6.6 + AMEND-3)
//
//   - 1 counter: rq_blocked (incremented on Block path).
//   - 6 gauges: concurrency_limit, gradient (×1000 per ADR-0059 §Decision
//     AMENDMENT), burst_queue_size, sample_rtt_msecs (ns; envoy-go-strict
//     departure from upstream ms — name preserves byte-exact), min_rtt_msecs
//     (ns), min_rtt_calculation_active (0/1 via stats.BoolToInt).
//
// Stat-prefix template per AMEND-3 C2:
//
//	http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.<stat>
//
// # 503-overflow wire shape (per AMEND-6 + §21.P1 RATIFIED)
//
//	status default 503 + body "reached concurrency limit" (25 bytes verbatim;
//	NO trailing newline) + content-type: text/plain + content-length: 25.
//
// # Per-HCM-instance controller semantics
//
// Per the Task-9 factory wiring + ADR-0186 §Decision: ONE *gradientController
// per HCM filter chain mounting this filter (shared across every per-stream
// *filter constructed by the returned FilterInstanceFactory closure). The
// controller's atomic hot-path + mu-protected cold-path serialize concurrent
// forwarders per planner-time D17.
//
// # Per-route discipline (REUSE-by-absence per SPEC §5.4)
//
// NO AdaptiveConcurrencyPerRoute proto message exists at v1.32.4 OR v1.37.x;
// any TPFC placement at route or virtualHost level PARSE-REJECTs at proto-
// deserialization-time via the existing HCM framework. NO ADR-0125 amendment
// fired (FOURTH CONSECUTIVE §9 row to skip).
//
// # Cross-references
//
//   - ADR-0186 (§Decision + §Consequences anchored at IMPL Task 3 —
//     Gradient-1 controller state machine + the Clock seam, migrated to the
//     unified internal/clock superset per §Consequences (g) EXTRACT-NOW).
//   - ADR-0187 (§Decision + §Consequences anchored at IMPL Task 2 —
//     enabled.runtime_key deferral PARSE-REJECT).
//   - ADR-0059 (§Decision AMENDMENT body anchored at IMPL Task 4 —
//     float-valued-gauge int64 encoding convention).
//   - SPEC §1.1 AMEND-1..AMEND-7 amendment block.
//   - SPEC §6 code shapes; SPEC §7 differential fixture
//     0025-http-adaptive-concurrency (lands at Task 10).
package adaptive_concurrency
