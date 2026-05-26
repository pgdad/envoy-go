// Package abi — stubs_25_2.go: the `Host25_2 any` host-handle type-anchor
// for the 14 NEW 25.2 hostcall dispatch shims per 25.2 SPEC §5.1 +
// AMEND-B1..B5.
//
// # File history
//
// At Task 3 this file held 14 placeholder shim bodies (each panicking with
// "Task N not yet landed" — replaced by the real impls at Tasks 4-8 + 12).
// As each Task landed its family the corresponding placeholder line was
// deleted; with Task 12 (metrics family) the last 4 placeholders disappear.
//
// What remains is the `Host25_2 any` type-anchor that every shim across the
// abi/ family files takes as its host-handle parameter. The type-anchor MUST
// live in exactly one file (this one) so the family-file shims compile
// against the same name; future renames flow through a single source.
//
// Per-family activated shims as of Task 12 (all 14 LANDED):
//
//   - body+buffer (3): GetBufferBytesShim, SetBufferBytesShim,
//     GetBufferStatusShim — body_bridge.go
//   - stream-control (2): ContinueStreamShim, CloseStreamShim —
//     stream_control.go
//   - timer (1): SetTickPeriodMillisecondsShim — timer.go
//   - shared-data (2): SetSharedDataShim, GetSharedDataShim — shared_data.go
//   - foreign-function (1): CallForeignFunctionShim — foreign.go
//   - httpCall (1): HttpCallShim — http_call.go
//   - metrics (4): DefineMetricShim, IncrementMetricShim, RecordMetricShim,
//     GetMetricShim — metrics.go (Task 12 LANDED — this file's last 4
//     placeholders deleted)
//
// # Design constraint (preserved from Task 3 doc)
//
// The abi/ package MUST NOT import the wasm/ package (would induce a
// circular dependency since wasm imports abi). To stay decoupled, each shim
// accepts a `Host25_2 any` parameter — the registration.go envelope passes
// its `*RootVM` through unchanged, and each shim type-asserts to a
// package-private per-family interface (e.g., metricsHost for the metric
// shims; sharedDataHost for the shared-data shims) at the call site.
package abi

// Host25_2 is the opaque host-handle type for the 14 25.2 hostcall dispatch
// shims. Implemented by *wasm.RootVM at the call site (the registration.go
// shim envelope passes its *RootVM through unchanged). The abi/ package
// cannot reference *wasm.RootVM directly (circular import); each shim
// type-asserts to its required package-private per-family interface at
// dispatch time (see body_bridge.go bufferHost + stream_control.go
// streamHost + timer.go timerHost + shared_data.go sharedDataHost +
// foreign.go foreignHost + http_call.go httpCallHost + metrics.go
// metricsHost for the activated examples).
type Host25_2 any
