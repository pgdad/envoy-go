// Package abi — metrics.go: 4 metric-family hostcall dispatch shims per
// 25.2 SPEC §5.1 #31-34 + AMEND-B2 + R-25.2-2 + R-25.2-7.
//
// # Four host shims landed at Task 12
//
//   - DefineMetricShim — proxy_define_metric(metric_type, name_data, name_size,
//     ret_metric_id_ptr) -> WasmResult. Reads `name` from guest memory +
//     delegates to the per-*RootVM metricsHost.DefineMetric (which wraps
//     *dynamic.Registry from internal/stats/dynamic/) + writes the allocated
//     MetricID to ret_metric_id_ptr. MetricType byte-pin per AMEND-B2:
//     Counter=0, Gauge=1, Histogram=2.
//
//   - IncrementMetricShim — proxy_increment_metric(metric_id, delta) ->
//     WasmResult. Delta is SIGNED int64 per AMEND-B2 (allows negative Gauge
//     deltas). On Counter the underlying *stats.Counter rejects negative
//     deltas with BadArgument per the Task 11 *dynamic.Registry deviation.
//     On Histogram the host returns BadArgument (Histograms accept Record
//     only per AMEND-B2).
//
//   - RecordMetricShim — proxy_record_metric(metric_id, value) -> WasmResult.
//     Value is UNSIGNED uint64 per AMEND-B2. On Counter the host returns
//     BadArgument (Counters accept Increment only). On Gauge the value is
//     reinterpreted as int64. On Histogram the value is stored in the
//     in-package atomic.Uint64 stub per ADR-0060 deferred-primitive
//     disposition.
//
//   - GetMetricShim — proxy_get_metric(metric_id, ret_value_ptr) ->
//     WasmResult. Reads the current value via host.GetMetric + writes it as
//     uint64 LE to ret_value_ptr. Counter values are naturally unsigned;
//     Gauge values are reinterpreted from int64 to uint64 (caller's
//     responsibility per AMEND-B2); Histogram values are the latest-Record-ed
//     value per the ADR-0060 disposition.
//
// # Status matrix per AMEND-B2 + Task 11 *dynamic.Registry
//
//	DefineMetric    BadArg            empty name / invalid name / cross-type re-register / MetricType out of range
//	                InternalFailure   cap-exceeded (ErrCapExceeded; Task 17 dynamic_stats_cap_exceeded counter)
//	                Ok                first Register OR idempotent re-Register of same (name, type)
//
//	IncrementMetric NotFound          unknown metric_id
//	                BadArg            Histogram (Increment unsupported); Counter + negative delta
//	                Ok                Counter + non-negative OR Gauge (any signed delta per AMEND-B2)
//
//	RecordMetric    NotFound          unknown metric_id
//	                BadArg            Counter (Record unsupported)
//	                Ok                Gauge OR Histogram
//
//	GetMetric       NotFound          unknown metric_id
//	                InvalidMemAccess  ret_value_ptr write failed
//	                Ok                else; writes current value as uint64-LE
//
// # Decoupling from internal/wasm
//
// abi/ MUST NOT import internal/wasm (would induce a cycle — wasm imports
// abi). Each shim accepts a `Host25_2 any` parameter (declared in
// stubs_25_2.go) carrying an opaque value the registration.go envelope
// passes through unchanged (typically *RootVM). The shim type-asserts to a
// package-private metricsHost interface; if the value does not satisfy the
// interface, the shim returns WasmResultInternalFailure (programmer error).
package abi

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// metricsHost is the package-private interface the metric-family shims
// dispatch through. Satisfied by *wasm.RootVM (via the DefineMetric +
// IncrementMetric + RecordMetric + GetMetric methods on RootVM — see
// internal/wasm/dynamic_stats.go) so the registration.go envelope can
// forward its `vm` argument unchanged. The shim type-asserts the opaque
// Host25_2 argument to this interface; non-satisfaction returns
// WasmResultInternalFailure.
type metricsHost interface {
	// DefineMetric registers `name` under MetricType `metricType` (0=Counter,
	// 1=Gauge, 2=Histogram per AMEND-B2 byte-pin) + returns the allocated
	// MetricID + status. Idempotent: same (name, metricType) returns the
	// cached id + Ok. cross-type re-Register OR invalid name OR out-of-range
	// metricType returns 0 + BadArgument. Cap-exceeded returns 0 +
	// InternalFailure.
	DefineMetric(metricType uint32, name string) (uint32, WasmResult)

	// IncrementMetric applies SIGNED int64 delta to the metric identified
	// by id per AMEND-B2. Counter with negative delta returns BadArgument
	// (the underlying *stats.Counter is monotonic-non-negative); Histogram
	// returns BadArgument (Histograms accept Record only); Gauge accepts
	// any signed delta.
	IncrementMetric(id uint32, delta int64) WasmResult

	// RecordMetric sets UNSIGNED uint64 value on the metric identified by id
	// per AMEND-B2. Counter returns BadArgument (Counters accept Increment
	// only); Gauge reinterprets value as int64 + sets atomically; Histogram
	// stores in the in-package atomic.Uint64 stub per ADR-0060.
	RecordMetric(id uint32, value uint64) WasmResult

	// GetMetric returns the current value of the metric identified by id as
	// uint64. Counter values are naturally unsigned; Gauge values are
	// reinterpreted from int64 to uint64; Histogram values are the latest
	// Record-ed value per the ADR-0060 disposition.
	GetMetric(id uint32) (uint64, WasmResult)
}

// DefineMetricShim — proxy_define_metric dispatch per §5.1 #31 + AMEND-B2.
//
// Wire-shape (cpp-host include/proxy-wasm/exports.h + spec @main
// abi-versions/v0.2.1/README.md):
//
//	Word define_metric(Word metric_type, Word name_data, Word name_size,
//	                   Word return_metric_id_ptr)
//
// metric_type is a uint32 carrying the MetricType byte-pin: 0=Counter,
// 1=Gauge, 2=Histogram per AMEND-B2. Values outside this range surface as
// BadArgument via the host (the host's *dynamic.Registry validates).
//
// Reads `name` from guest memory; delegates to metricsHost.DefineMetric; on
// Ok writes the allocated MetricID to ret_metric_id_ptr.
//
// Memory-read failure on `name` returns WasmResultInvalidMemoryAccess. An
// empty name (nameSize == 0) is read as the empty string + propagates to the
// host which rejects with BadArgument (mirrors the dynamic.Registry
// userNameRE rejection of empty names). Memory-write failure on
// ret_metric_id_ptr returns WasmResultInvalidMemoryAccess.
func DefineMetricShim(_ context.Context, mod api.Module, host Host25_2, metricType, nameDataPtr, nameSize, retMetricIDPtr uint32) WasmResult {
	mh, ok := host.(metricsHost)
	if !ok {
		return WasmResultInternalFailure
	}

	mem := mod.Memory()

	// Read name bytes from guest memory. nameSize == 0 is read as empty
	// string + the host's userNameRE rejects with BadArgument.
	var nameBytes []byte
	if nameSize > 0 {
		raw, ok := mem.Read(nameDataPtr, nameSize)
		if !ok {
			return WasmResultInvalidMemoryAccess
		}
		// Defensive copy: wazero's Read returns a view into live guest
		// memory; we convert to a stable string so the host's map key is
		// independent of subsequent guest mutation.
		nameBytes = make([]byte, len(raw))
		copy(nameBytes, raw)
	}

	id, status := mh.DefineMetric(metricType, string(nameBytes))
	if status != WasmResultOk {
		return status
	}

	if !mem.WriteUint32Le(retMetricIDPtr, id) {
		return WasmResultInvalidMemoryAccess
	}
	return WasmResultOk
}

// IncrementMetricShim — proxy_increment_metric dispatch per §5.1 #32 +
// AMEND-B2.
//
// Wire-shape (cpp-host src/exports.cc:1065-1068 + rust-sdk
// hostcalls.rs:1395-1397):
//
//	Word increment_metric(Word metric_id, int64_t offset)
//
// delta is SIGNED int64 per AMEND-B2 (allows negative Gauge deltas).
// Delegates straight through to metricsHost.IncrementMetric; the host
// applies the per-MetricType semantic (Counter monotonic-non-negative;
// Gauge any signed; Histogram Increment unsupported).
func IncrementMetricShim(_ context.Context, _ api.Module, host Host25_2, metricID uint32, delta int64) WasmResult {
	mh, ok := host.(metricsHost)
	if !ok {
		return WasmResultInternalFailure
	}
	return mh.IncrementMetric(metricID, delta)
}

// RecordMetricShim — proxy_record_metric dispatch per §5.1 #33 + AMEND-B2.
//
// Wire-shape (cpp-host src/exports.cc:1070-1073 + rust-sdk
// hostcalls.rs:1381-1383):
//
//	Word record_metric(Word metric_id, uint64_t value)
//
// value is UNSIGNED uint64 per AMEND-B2. Delegates straight through to
// metricsHost.RecordMetric; the host applies the per-MetricType semantic
// (Counter Record unsupported; Gauge reinterpret as int64; Histogram store
// in stub).
func RecordMetricShim(_ context.Context, _ api.Module, host Host25_2, metricID uint32, value uint64) WasmResult {
	mh, ok := host.(metricsHost)
	if !ok {
		return WasmResultInternalFailure
	}
	return mh.RecordMetric(metricID, value)
}

// GetMetricShim — proxy_get_metric dispatch per §5.1 #34.
//
// Wire-shape (cpp-host src/exports.cc:1075-1085 + rust-sdk
// hostcalls.rs:1365-1367):
//
//	Word get_metric(Word metric_id, uint64_t *return_value)
//
// Delegates to metricsHost.GetMetric; on Ok writes the value as uint64-LE
// to ret_value_ptr. NotFound + other non-Ok status returns are propagated
// unchanged WITHOUT touching ret_value_ptr (so a guest that pre-zeroes the
// slot doesn't observe stale data on the error path).
//
// Memory-write failure on ret_value_ptr returns WasmResultInvalidMemoryAccess.
func GetMetricShim(_ context.Context, mod api.Module, host Host25_2, metricID, retValuePtr uint32) WasmResult {
	mh, ok := host.(metricsHost)
	if !ok {
		return WasmResultInternalFailure
	}
	value, status := mh.GetMetric(metricID)
	if status != WasmResultOk {
		return status
	}
	if !mod.Memory().WriteUint64Le(retValuePtr, value) {
		return WasmResultInvalidMemoryAccess
	}
	return WasmResultOk
}
