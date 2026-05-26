// dynamic_stats.go — per-*RootVM dynamic-stats wrapper per 25.2 SPEC §3.1
// dynamic_stats.go bullet + §5.1 #31-34 + AMEND-B2 + R-25.2-2 + R-25.2-7 +
// ADR-0208.
//
// # Design (per AMEND-B2 + R-25.2-7 + ADR-0208)
//
// Each *RootVM owns at most ONE per-plugin *dynamic.Registry (from
// internal/stats/dynamic/). The registry is wired at NewRootVM time via
// WithRootDynamicStats; if no option is supplied, the dynStats field is nil
// and every metric call returns InternalFailure (envoy-go-strict default-
// deny posture — operator opts in by wiring the registry at compiledConfig
// construction time at Task 14).
//
// The four methods below — DefineMetric / IncrementMetric / RecordMetric /
// GetMetric — are the host-side dispatch targets for the four metric-family
// hostcall shims at internal/wasm/abi/metrics.go. Each method:
//
//	1. Nil-checks rv.dynStats (default-deny if unset).
//	2. Delegates to the corresponding *dynamic.Registry method.
//	3. Translates the *dynamic.Registry sentinel error (ErrCapExceeded /
//	   ErrBadArgument / ErrNotFound) into the proxy-wasm WasmResult per the
//	   AMEND-B2 status matrix (see metrics.go for the full matrix).
//
// # MetricType byte-pin per AMEND-B2 + R-25.2-2
//
// The metricType uint32 argument to DefineMetric maps directly to
// dynamic.MetricType (Counter=0, Gauge=1, Histogram=2). The cast is
// `dynamic.MetricType(metricType)`; the *dynamic.Registry.Register
// validates the value is in {0,1,2} and rejects out-of-range with
// ErrBadArgument — this wrapper propagates as WasmResultBadArgument.
//
// # ErrCapExceeded → InternalFailure + Task 17 counter wiring
//
// 25.2 SPEC §7.1 #13 + AMEND-B2 mandate two counters on the
// `proxy_define_metric` cap-exceeded path:
//
//	(a) `wasm.<plugin>.dynamic_stats_cap_exceeded` envoy-go-strict counter
//	(b) `envoy_go.failures` co-increment per §2.25
//
// Both counter increments are TODO Task 17 (per-plugin stats.go EXTEND).
// At Task 12 the wire-shape contract is pinned (InternalFailure return);
// the counter integration touchpoints are marked with TODO Task 17
// references in the ErrCapExceeded branch below.
//
// # Counter negative-delta rejection (Task 11 deviation pinned)
//
// AMEND-B2 pins proxy_increment_metric delta as SIGNED int64 (allows
// negative gauge deltas). The Counter primitive is monotonic-non-negative
// per the underlying *stats.Counter contract (counter.go Add(uint64));
// *dynamic.Registry.Increment rejects negative deltas on Counter-typed
// metrics with ErrBadArgument (Task 11 deviation documented in PROGRESS).
// This wrapper propagates as WasmResultBadArgument unchanged.
//
// # Goroutine-safety
//
// The four methods inherit *dynamic.Registry's thread-safety contract
// (sync.RWMutex on Register; lock-free atomics on Increment / Record /
// Get). The methods are SAFE to call from any goroutine independently of
// dispatchMu — typical callers are the per-stream filter-dispatch
// goroutines via the proxy_define_metric / proxy_increment_metric /
// proxy_record_metric / proxy_get_metric shim invocations from inside the
// dispatchMu-held frame, but the wrapper does NOT itself acquire
// dispatchMu (the shim envelope's runWithPanicWrapper already provides
// panic-recovery; no additional locking is needed).

package wasm

import (
	"errors"

	"github.com/esalaine/envoy-go/internal/stats/dynamic"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// DefineMetric registers `name` under MetricType `metricType` (Counter=0,
// Gauge=1, Histogram=2 per AMEND-B2 byte-pin) + returns the allocated
// MetricID + status. Idempotent: same (name, metricType) returns the cached
// id + WasmResultOk; cross-type re-Register returns 0 + WasmResultBadArgument.
//
// Returns:
//
//   - (MetricID, WasmResultOk) on a successful Register (NEW entry OR
//     idempotent re-Register of an existing (name, type) pair).
//   - (0, WasmResultBadArgument) on out-of-range MetricType, invalid name
//     (empty / leading-dot / trailing-dot / disallowed-char per
//     dynamic.userNameRE), OR same-name-different-MetricType re-Register.
//   - (0, WasmResultInternalFailure) on ErrCapExceeded (the per-plugin
//     dynamic-stats namespace size cap was reached; default 1024 per
//     AMEND-B2 + Q9). The Task 17 wiring will ALSO increment two counters
//     on this path per §7.1 #13 + §2.25 (counter wiring deferred).
//   - (0, WasmResultInternalFailure) if rv.dynStats is nil (no per-plugin
//     *dynamic.Registry wired at compiledConfig construction).
func (rv *RootVM) DefineMetric(metricType uint32, name string) (uint32, abi.WasmResult) {
	if rv.dynStats == nil {
		return 0, abi.WasmResultInternalFailure
	}

	id, err := rv.dynStats.Register(dynamic.MetricType(int(metricType)), name) //nolint:gosec // wire-perspective byte-pin per AMEND-B2.
	if err == nil {
		return uint32(id), abi.WasmResultOk
	}
	switch {
	case errors.Is(err, dynamic.ErrCapExceeded):
		// envoy-go-strict counter increment per §7.1 #13 (counter 13 of 14)
		// + envoy_go.failures co-increment per §2.25. Wired via
		// RootStatsRecorder per 25.2 IMPL Task 20 follow-up (Concern 2).
		rv.stats.DynamicStatsCapExceededInc()
		rv.stats.EnvoyGoFailuresInc()
		return 0, abi.WasmResultInternalFailure
	case errors.Is(err, dynamic.ErrBadArgument):
		return 0, abi.WasmResultBadArgument
	default:
		return 0, abi.WasmResultInternalFailure
	}
}

// IncrementMetric applies SIGNED int64 delta to the metric identified by
// `id` per AMEND-B2 (allows negative gauge deltas).
//
// Returns:
//
//   - WasmResultOk on Counter (non-negative delta) OR Gauge (any signed
//     delta per AMEND-B2). The underlying Counter/Gauge primitive Add is
//     lock-free atomic.
//   - WasmResultNotFound on unknown `id` (the *dynamic.Registry sentinel
//     ErrNotFound translates here; the proxy-wasm guest may pass any uint32
//     as a metric_id argument).
//   - WasmResultBadArgument on:
//     (a) Counter with negative delta (Task 11 deviation — Counter is
//     monotonic-non-negative per the underlying *stats.Counter contract);
//     (b) Histogram (Histograms accept Record only per AMEND-B2);
//     (c) any other ErrBadArgument from *dynamic.Registry.
//   - WasmResultInternalFailure if rv.dynStats is nil.
func (rv *RootVM) IncrementMetric(id uint32, delta int64) abi.WasmResult {
	if rv.dynStats == nil {
		return abi.WasmResultInternalFailure
	}
	err := rv.dynStats.Increment(dynamic.MetricID(id), delta)
	if err == nil {
		return abi.WasmResultOk
	}
	switch {
	case errors.Is(err, dynamic.ErrNotFound):
		return abi.WasmResultNotFound
	case errors.Is(err, dynamic.ErrBadArgument):
		return abi.WasmResultBadArgument
	default:
		return abi.WasmResultInternalFailure
	}
}

// RecordMetric sets UNSIGNED uint64 value on the metric identified by `id`
// per AMEND-B2 (overwrite semantic on Gauge / Histogram; Counter rejected).
//
// Returns:
//
//   - WasmResultOk on Gauge (value reinterpreted as int64 + stored) OR
//     Histogram (value stored in the in-package atomic.Uint64 stub per
//     ADR-0060 deferred-primitive disposition).
//   - WasmResultNotFound on unknown `id`.
//   - WasmResultBadArgument on Counter (Counters accept Increment only per
//     AMEND-B2).
//   - WasmResultInternalFailure if rv.dynStats is nil.
func (rv *RootVM) RecordMetric(id uint32, value uint64) abi.WasmResult {
	if rv.dynStats == nil {
		return abi.WasmResultInternalFailure
	}
	err := rv.dynStats.Record(dynamic.MetricID(id), value)
	if err == nil {
		return abi.WasmResultOk
	}
	switch {
	case errors.Is(err, dynamic.ErrNotFound):
		return abi.WasmResultNotFound
	case errors.Is(err, dynamic.ErrBadArgument):
		return abi.WasmResultBadArgument
	default:
		return abi.WasmResultInternalFailure
	}
}

// GetMetric returns the current value of the metric identified by `id` as
// uint64. Counter values are naturally unsigned; Gauge values are
// reinterpreted from int64 to uint64 (caller is responsible for the signed
// interpretation if the gauge can carry negative values per AMEND-B2);
// Histogram values are the latest-Record-ed value per the ADR-0060
// disposition.
//
// Returns:
//
//   - (value, WasmResultOk) on a known `id`.
//   - (0, WasmResultNotFound) on unknown `id`.
//   - (0, WasmResultInternalFailure) if rv.dynStats is nil OR any other
//     *dynamic.Registry error.
func (rv *RootVM) GetMetric(id uint32) (uint64, abi.WasmResult) {
	if rv.dynStats == nil {
		return 0, abi.WasmResultInternalFailure
	}
	val, err := rv.dynStats.Get(dynamic.MetricID(id))
	if err == nil {
		return val, abi.WasmResultOk
	}
	if errors.Is(err, dynamic.ErrNotFound) {
		return 0, abi.WasmResultNotFound
	}
	return 0, abi.WasmResultInternalFailure
}
