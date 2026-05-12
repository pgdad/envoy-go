// Package bandwidthlimit implements envoy.filters.http.bandwidth_limit —
// Envoy v1.37.2's canonical "rate-limit body throughput in KiB/s
// (kibibytes-per-second per proto comment + SPEC §1.1 amendment 6; NOT
// kilobits-per-second)" filter, symmetric BOTH-direction (request + response)
// MVP, under the 07.1 HTTP filter framework. Phase 15. ZERO framework deltas
// (composes against phase-09 fault async-resume + phase-13 ADR-0128
// decode-side body-buffering + phase-14 ADR-0131 encode-side OverwriteBody
// primitives without amendment).
//
// Listener-level BandwidthLimit proto fields envoy-go consumes (4):
//   - stat_prefix (PGV non-empty per §11.P2 + amendment 3)
//   - enable_mode (4-value enum: DISABLED / REQUEST / RESPONSE /
//     REQUEST_AND_RESPONSE; PGV defined_only; default DISABLED=0)
//   - limit_kbps (UInt64Value; KiB/s units per amendment 6; OPTIONAL at
//     listener with foot-gun semantic per amendment 10; CODE-LEVEL REQUIRED
//     at per-route per ADR-0136)
//   - fill_interval (Duration; default 50ms; PGV [20ms, 1s] per amendment 5)
//
// Silent-ignored at parse (ADR-0040 silent-ignore discipline; 3 fields):
//   - runtime_enabled (RuntimeFeatureFlag; always-100%-active per ADR-0117 /
//     ADR-0121 / ADR-0130 precedent)
//   - enable_response_trailers (bool; trailer-emission framework primitive
//     deferred; always-no-trailers in envoy-go MVP)
//   - response_trailer_prefix (string; couples to enable_response_trailers)
//
// Operational foot-gun (per amendment 10): listener-level limit_kbps unset
// combined with active enable_mode produces a runtime hang on first body
// chunk (matches Envoy byte-equivalent; documented at §13.4 forward-pointer
// notes).
//
// HTTPFilter value: ENCODER+DECODER with the SAME *filter instance —
// Decoder: f, Encoder: f (per ADR-0135; mirrors phase-14 ADR-0129
// generalized to symmetric BOTH-direction throttle).
//
// Per-route discipline: the NEW 6th canonical bare-message-via-TPFC + CODE-
// LEVEL required-limit_kbps-at-per-route pattern per ADR-0125 §(xi)
// amendment (LANDED at SPEC commit) + ADR-0136 + ADR-0139. Same BandwidthLimit
// proto reused via TPFC by pointer-identity key into factoryState.perRoute
// sync.Map lazy-cache. Per-route stats INDEPENDENT (own *filterStats per
// stat_prefix) — mirrors phase-11 ADR-0117 + per-route allocation via
// newFilterStatsIfAbsent (post-Freeze idempotent).
//
// Body algorithm: Path B-async (buffer-then-delayed-emit) with kbps-per-tick
// throttle math per ADR-0137 + SPEC §6.6 + amendment 6. Decode/Encode-side
// DataData buffers; on endStream=true computes throttle_duration via
// chunk_size = limit_kbps × 1024 × fill_interval_seconds bytes/tick;
// ticks = ceil(body_size / chunk_size); throttle = ticks × fill_interval.
// time.AfterFunc(throttle, ...) arms the resume timer; timer-fire callback
// invokes cb.ContinueDecoding / ContinueEncoding. The buffered-return path
// emits bytes unchanged (cb.OverwriteBody NOT invoked per anticipated
// framework-survey; ZERO framework deltas per §3). The full DecodeData /
// EncodeData / OnDestroy method bodies land at Tasks 4 + 5 + 6.
//
// Stat surface: 14 active stats per stat_prefix (8 counters + 6 gauges) per
// ADR-0138 + amendment 7. Namespace
// <stat_prefix>.http_bandwidth_limit.<counter> (underscore-infix; NOT
// HCM-rooted per §11.P11). Prometheus rendering via existing default-branch
// flatten as envoy_<stat_prefix>_http_bandwidth_limit_<counter>{} (NO new
// SN10 rule per §11.P10). The 2 unconditional Envoy histograms
// (request_transfer_duration, response_transfer_duration) NOT registered in
// MVP per phase-06.1 baseline + amendment 9 divergence-window. Full
// registration lands at Task 8.
//
// Wire-shape divergence vs reference Envoy: envoy-go Path B-async
// (silent-then-blast at timer-fire) vs Envoy Path A rate-paced chunks at
// fill_interval cadence (chunk_size bytes per tick). Total wall-clock
// throttle-time observably equivalent within ±70ms tolerance per §11.P9;
// chunk-arrival-time deliberately diverges (allow-listed at §13.4).
//
// Cross-cutting ADR anchors (per ADR-0044 ADR-on-impl convention; authored
// at impl-time per phase-13 buffer pattern):
//   - ADR-0125 §(xi) amendment paragraph (LANDED at SPEC commit; 6th
//     canonical bare-message-via-TPFC + code-level-required-limit_kbps)
//   - ADR-0135 (package shape + SAME *filter HTTPFilter + 14-stat
//     filterStats + boot-registration ordering)
//   - ADR-0136 (compiledConfig shape + 4-consumed/3-silent-ignored field
//     decomposition + PGV-mirror filter-internal validation discipline +
//     CODE-LEVEL extra check at per-route for limit_kbps REQUIRED + envoy-
//     go-own error wording)
//   - ADR-0137 (Path B-async body algorithm + kbps-per-tick throttle math;
//     lands at Task 3)
//   - ADR-0138 (14-stat surface + namespace shape; full registration lands
//     at Task 8)
//   - ADR-0139 (per-route INDEPENDENT-stats ratification; lands at Task 7)
package bandwidthlimit
