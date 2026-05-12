package bandwidthlimit

import "time"

// throttleDuration computes the kbps-per-tick throttle duration for emitting
// bodySize bytes at limitKbps KiB/s (kibibytes-per-second per proto comment
// at bandwidth_limit.pb.go:95; NOT kilobits-per-second per SPEC §1.1
// amendment 6) with fillInterval governing chunk-emit cadence.
//
// Formula (per SPEC §6.6 + §11.P15 empirical):
//
//	chunk_size_per_tick = limit_kbps × 1024 × fill_interval_seconds (bytes/tick)
//	ticks               = ceil(body_size / chunk_size_per_tick)
//	throttle_duration   = ticks × fill_interval
//
// Edge cases:
//
//   - bodySize == 0: returns (0, 0) — no throttle.
//
//   - limitKbps == 0 (listener-level operational foot-gun per §1.1 amendment 10
//     and §11 probeJ): returns (24*time.Hour, 1) — arbitrarily-large throttle
//     to match Envoy's runtime-hang behavior on listener-level missing
//     limit_kbps with active enable_mode. The ticks=1 marker lets the caller
//     still increment *_enforced by 1 (matching Envoy at the first tick).
//
//   - bodySize <= chunk_size_per_tick: returns (fillInterval, 1) — one-tick
//     floor; approximates Envoy's initial-burst capacity behavior within
//     ±70ms tolerance per §11.P9.
//
// Returns ticks alongside duration so the caller can increment *_enforced by
// ticks at stream-completion (per §11.P3 + §6.7 + §1.1 amendment 7 per-tick
// cumulative-match discipline).
//
// Empirical verification matrix (verbatim from SPEC §6.6):
//
//	| Body | limit_kbps | fill_interval | chunk_size | ticks | Predicted | Observed (§11.P9) |
//	|------|------------|---------------|------------|-------|-----------|---------------------|
//	|  100 |     10     |     50ms      | 512 bytes  |   1   |    50ms   |  5ms (initial-burst)|
//	| 1024 |     10     |     50ms      | 512 bytes  |   2   |   100ms   |    107ms            |
//	| 4000 |     10     |     50ms      | 512 bytes  |   8   |   400ms   |    359-367ms        |
//	| 4000 |      5     |     50ms      | 256 bytes  |  16   |   800ms   |    716-814ms        |
//	| 4000 |      1     |     50ms      | 51.2 bytes |  79   |  3950ms   |    3904ms           |
//
// Differences within ±70ms tolerance per §11.P9 absorb the initial-burst
// capacity approximation. The pure-function shape means no state allocation.
func throttleDuration(bodySize int, limitKbps uint64, fillInterval time.Duration) (duration time.Duration, ticks uint64) {
	if bodySize == 0 {
		return 0, 0
	}
	if limitKbps == 0 {
		// Foot-gun match per §1.1 amendment 10: arbitrarily-large throttle.
		return 24 * time.Hour, 1
	}
	fillSec := fillInterval.Seconds()
	chunkSize := uint64(float64(limitKbps) * 1024 * fillSec)
	if chunkSize == 0 {
		// Defensive (structurally unreachable given fill_interval >= 20ms PGV).
		chunkSize = 1
	}
	ticks = (uint64(bodySize) + chunkSize - 1) / chunkSize // ceil division
	duration = time.Duration(ticks) * fillInterval
	return duration, ticks
}
