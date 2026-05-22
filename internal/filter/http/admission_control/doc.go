// Package admission_control implements the envoy.filters.http.admission_control
// HTTP filter under the 07.1 HTTP filter framework.
//
// Phase 23: canonical Envoy v1.37.2 SRE-book client-side probabilistic
// admission-control filter per ADR-0194. Over a per-HCM-instance sliding
// sampling window of {requests, successes} bucket counts, it computes a
// rejection probability and probabilistically short-circuits requests with
// an HTTP 503 to shed load when the downstream success rate drops.
//
// # TypeURL
//
// The canonical proto TypeURL registered at boot per ADR-0143 SN1:
//
//	type.googleapis.com/envoy.extensions.filters.http.admission_control.v3.AdmissionControl
//
// # Per-HCM-instance sliding-window controller semantics
//
// One *controller per HCM filter chain mounting this filter (shared across
// every per-stream *filter constructed by the returned FilterInstanceFactory
// closure). The controller owns a std::deque-mirror of per-second
// {requests, successes} buckets per AMEND-6: stale buckets older than
// sampling_window are purged on each record; a new bucket rolls over once
// the newest is >=1s old. requestCounts() returns the running aggregate;
// averageRps() = global.requests / max(samplingWindow, ageOfOldestBucket)
// in whole seconds. The rejection probability formula (line-cited at
// admission_control.cc:161-179 per ADR-0194) is:
//
//	P = max(0, min(max_rej, ((n - s/sr_threshold) / (n+1))^(1/aggression)))
//
// # Both-sides decode-gate / encode-classify discipline
//
// The SAME *filter instance is wired as HTTPFilter{Decoder: f, Encoder: f}
// per PD-4. On DecodeHeaders: disabled pass-through (gate 1) -> RPS-
// suppression short-circuit (gate 2) -> shouldReject() probabilistic gate
// (gate 3); a reject increments rq_rejected and emits a 503 local reply
// with an empty body. On EncodeHeaders (+ EncodeTrailers for the gRPC-
// status-in-trailers case per AMEND-10): classifies the upstream response
// per success_criteria and records into the current bucket via classify().
// Rejected / disabled / RPS-suppressed requests set record=false and are
// deliberately NOT recorded into the window per AMEND-11.
//
// # 3-counter stat surface (per AMEND-3; NO gauges)
//
// Stat-prefix template:
//
//	http.<HCM_stat_prefix>.admission_control.<stat>
//
// Exactly 3 counters: rq_rejected, rq_success, rq_failure. No gauges,
// no histograms (the upstream ALL_ADMISSION_CONTROL_STATS macro is
// COUNTER-only per admission_control.h:35-38; no sub-infix like
// gradient_controller).
//
// # Cross-references
//
//   - ADR-0194 (algorithm + package shape + inline Rand/Clock seams +
//     deque-window + integer-modulo decision + classification +
//     3-counter stat surface + deterministic-regime differential strategy;
//     line-cited against admission_control.cc + thread_local_controller.cc +
//     success_criteria_evaluator.cc).
//   - ADR-0195 (RTDS runtime_key deferral PARSE-REJECT — 5 arms;
//     enabled-absent => ENABLED per AMEND-4; single envoy-go-strict
//     departure).
//   - SPEC §6.8 source-file roster; SPEC §7 differential fixtures
//     0030-http-admission-control (cross-side) + 0031-http-admission-
//     control-boot-reject.
package admission_control
