package wasm

// stats.go — 5-counter stat surface per AMEND-A2 tri-group prefix structure +
// 25.1 SPEC §4.2 + parent §7. Lands the per-listener stat-counter holder + the
// `newFilterStats` constructor wired from `wasm.New` at Task 9 (Task 8 only
// defines the type + the constructor; the wiring lands at Task 9).
//
// AMEND-A2 tri-group prefix structure (DIVERGES from the dominant §9 family-
// row pattern — recorded at BEHAVIOR_CONTRACT.md final-Task 6-edit bundle per
// 25.1 SPEC §13.5 edit #3 as a structural-note row):
//
//   - Group B (`wasm.<runtime>.*` prefix; `<runtime>` = `"wazero"` uniformly
//     at 25.1, since wazero is the sole runtime per AMEND-A1): upstream-parity
//     counters mirroring the `WasmRuntimeStats` shape from
//     `source/extensions/common/wasm/stats.h`. Two stats at 25.1:
//
//       * `wasm.wazero.created` — counter; incremented per VM construction.
//       * `wasm.wazero.active`  — gauge; tracks live VM count
//                                (incr on construct, decr on Close).
//
//   - envoy-go-strict extensions (`wasm.<plugin_name>.*` prefix; `<plugin_name>`
//     is the `PluginConfig.name` discriminator per AMEND-A2 Group C). Three
//     envoy-go-strict counters at 25.1 — each requires a BEHAVIOR_CONTRACT.md
//     §13.5 envoy-go-strict departure record at Task 17 atomic landing:
//
//       * `wasm.<plugin_name>.executions`        — per `proxy_on_request_headers`
//                                                  invocation.
//       * `wasm.<plugin_name>.hostcall_denied`   — per default-denied hostcall
//                                                  invocation (anchors the
//                                                  AMEND-A5 default-deny
//                                                  capability sandbox).
//       * `wasm.<plugin_name>.envoy_go.failures` — per VM-failure event
//                                                  (panic-wrapper bumps).
//
// HCM-injected `stats_prefix` is DROPPED per AMEND-A2 — the wasm filter row
// DIVERGES from the dominant §9 family-row pattern. Upstream-parity
// preservation, NOT envoy-go-strict (upstream's
// `source/extensions/filters/http/wasm/config.h:51-53` does NOT inject the
// HCM `stats_prefix` either; the wasm filter's stat surface is rooted at
// `wasm.<runtime>.*` and `wasm.<plugin_name>.*` directly). The structural-
// note row at BEHAVIOR_CONTRACT.md captures the divergence-from-family
// shape (not from upstream).
//
// # Compile-time stat-name guards per ADR-0143 SN2-reuse
//
// Two-layer guard:
//
//   1. Per-stat `statName*` const declarations pin the byte-exact wire suffix
//      at compile time. A regression on any wire name surfaces immediately at
//      build time (the constructor uses these constants).
//
//   2. The `TestStatNames_Equal_*` table-driven assertions (in wasm_test.go)
//      pin each constant to its string literal — preventing a future refactor
//      from silently renaming the constant alongside the string literal.
//
// # Cross-references
//
//   - ADR-0085 (nil-tolerance discipline; `newFilterStats(nil, ...)` returns nil)
//   - ADR-0117 (NewCounterIfAbsent / NewGaugeIfAbsent post-Freeze idempotent
//     registration — used for the Group-B `wasm.wazero.*` shared-namespace
//     keys so multiple plugin configs on the same listener don't duplicate-
//     register, while the envoy-go-strict per-plugin keys use plain
//     NewCounter since `<plugin_name>` discriminates the key)
//   - ADR-0143 SN2-reuse (stat name as Go const reused by production + test)
//   - AMEND-A2 (tri-group prefix structure + HCM-stats_prefix DROPPED)
//   - AMEND-A5 (default-deny capability sandbox — anchors hostcall_denied)
//   - 25.1 SPEC §4.2 (compiledConfig + filterStats wiring) + §7 (stat surface
//     cross-reference) + §13.5 edit #3 (BEHAVIOR_CONTRACT departure record)

import (
	"github.com/esalaine/envoy-go/internal/stats"
	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
)

// -----------------------------------------------------------------------------
// Stat-name compile-time constants per ADR-0143 SN2-reuse + AMEND-A2.
// -----------------------------------------------------------------------------

// statNameCreated pins the byte-exact Group-B upstream-parity wire name for
// the VM-created counter. Full wire name (no further prefix): `wasm.wazero.created`.
// Incremented per VM construction at Task 12 decode_headers.go.
const statNameCreated = "wasm.wazero.created"

// statNameActive pins the byte-exact Group-B upstream-parity wire name for
// the live-VM-count gauge. Full wire name (no further prefix): `wasm.wazero.active`.
// Incremented on VM construct + decremented on VM Close at Task 12.
const statNameActive = "wasm.wazero.active"

// statNameExecutions pins the byte-exact envoy-go-strict suffix for the
// per-proxy_on_request_headers-invocation counter per AMEND-A2 Group-C-and-
// envoy-go-strict. Full wire name (assembled at registry-call time):
// `wasm.<plugin_name>.executions`. Departure record at Task 17
// BEHAVIOR_CONTRACT.md final-Task 6-edit bundle.
const statNameExecutions = "executions"

// statNameHostcallDenied pins the byte-exact envoy-go-strict suffix for the
// default-deny capability-sandbox denial counter per AMEND-A2 + AMEND-A5.
// Full wire name: `wasm.<plugin_name>.hostcall_denied`. Incremented at
// Task 11 abi_callbacks.go on any default-denied hostcall invocation.
// Departure record at Task 17.
const statNameHostcallDenied = "hostcall_denied"

// statNameEnvoyGoFailures pins the byte-exact envoy-go-strict suffix for the
// per-VM-failure-event counter per AMEND-A2. The dotted-suffix form is
// intentional: the full wire name `wasm.<plugin_name>.envoy_go.failures`
// carries an internal `envoy_go.` segment marking the envoy-go-strict origin
// of the metric. Incremented at Task 12 by the panic-wrapper after VM-trap
// or hostcall-denial-chain wazero error returns. Departure record at Task 17.
const statNameEnvoyGoFailures = "envoy_go.failures"

// statNameBodyBufferCapExceeded pins the byte-exact envoy-go-strict suffix
// for the body-buffer cap-exceeded counter per 25.2 SPEC §7.1 + Q2 +
// AMEND-B3. Full wire name: `wasm.<plugin_name>.body_buffer_cap_exceeded`.
// Incremented at Task 16 body.go DecodeData / EncodeData on the FIRST
// cap-exceeded event per stream (sticky flag prevents per-chunk re-bump).
// Co-increments wasm.<plugin>.envoy_go.failures per 25.2 §2.25 (any
// envoy-go-strict surface that fails a stream MUST also count against the
// generic failures counter so operators see one number reflecting all wasm-
// driven stream failures). The remaining 8 envoy-go-strict counters land at
// Task 17 (project stat count 119 → 128).
const statNameBodyBufferCapExceeded = "body_buffer_cap_exceeded"

// -----------------------------------------------------------------------------
// 25.2 IMPL Task 17 — 8 NEW envoy-go-strict stat-name suffixes per §7.1 +
// AMEND-B3. Combined with statNameBodyBufferCapExceeded (added at Task 16),
// these complete the 9 envoy-go-strict counters of the 25.2 stat-surface
// delta. Full wire names assembled at registry-call time:
// `wasm.<plugin_name>.<suffix>`. Departure record at Task 22 BEHAVIOR_
// CONTRACT.md final-Task atomic landing (consolidated 9-counter bundle
// envoy-go-strict departure record #1 per ADR-0208).
// -----------------------------------------------------------------------------

// statNameTickInvocations pins the byte-exact envoy-go-strict suffix for the
// per-`proxy_on_tick`-invocation counter per 25.2 SPEC §7.1 row 6 + Q5.
// Full wire name: `wasm.<plugin_name>.tick_invocations`. Incremented per
// tick by *RootVM.lockAndDispatchTick (Task 5 tick.go) — gives operators
// visibility into tick dispatch rate.
const statNameTickInvocations = "tick_invocations"

// statNameHttpCallDispatched pins the byte-exact envoy-go-strict suffix for
// the per-successful-proxy_http_call-dispatch counter per 25.2 SPEC §7.1 row
// 7 + Q4 + AMEND-B3. Full wire name: `wasm.<plugin_name>.http_call_
// dispatched`. Incremented per `proxy_http_call` invocation that successfully
// dispatches to an upstream cluster (cluster lookup OK; AsyncClient request
// started). Wired by Task 8 http_call.go on successful dispatch path.
const statNameHttpCallDispatched = "http_call_dispatched"

// statNameHttpCallResponse pins the byte-exact envoy-go-strict suffix for
// the per-successful-proxy_on_http_call_response counter per 25.2 SPEC §7.1
// row 8 + Q4. Full wire name: `wasm.<plugin_name>.http_call_response`.
// Incremented per `proxy_on_http_call_response` invocation (response routed
// to a live stream context). The companion late-response counter
// `http_call_response_after_close` fires when token lookup misses (i.e.,
// the originating stream has closed before the response arrived per
// AMEND-B3).
const statNameHttpCallResponse = "http_call_response"

// statNameForeignFunctionDenied pins the byte-exact envoy-go-strict suffix
// for the per-foreign-function-deny counter per 25.2 SPEC §7.1 row 9 +
// AMEND-A9. Full wire name: `wasm.<plugin_name>.foreign_function_denied`.
// Incremented per `proxy_call_foreign_function` invocation that returns
// `WasmResult::NotFound` (=1) — typically the EMPTY default registry path
// per AMEND-A9 (envoy-go ships ZERO foreign functions by default vs
// upstream's 10).
const statNameForeignFunctionDenied = "foreign_function_denied"

// statNameHttpCallDispatchUnknownCluster pins the byte-exact envoy-go-strict
// suffix for the per-unknown-cluster-dispatch counter per 25.2 SPEC §7.1 row
// 11 + Q4. Full wire name: `wasm.<plugin_name>.http_call_dispatch_unknown_
// cluster`. Incremented per `proxy_http_call` to an unknown cluster (the
// host returns BadArgument per upstream Envoy v1.37.2 context.cc:1547-1550
// CONFIRMED by AMEND-B3).
const statNameHttpCallDispatchUnknownCluster = "http_call_dispatch_unknown_cluster"

// statNameSharedDataCapExceeded pins the byte-exact envoy-go-strict suffix
// for the per-shared-data-cap-exceeded counter per 25.2 SPEC §7.1 row 12 +
// Q6. Full wire name: `wasm.<plugin_name>.shared_data_cap_exceeded`.
// Incremented when `proxy_set_shared_data` exceeds the value cap (1 MiB
// default per Q6) OR the entry-count cap (1024 default); the call returns
// `WasmResult::InternalFailure`. Co-increments `wasm.<plugin>.envoy_go.
// failures` per 25.2 §2.25.
const statNameSharedDataCapExceeded = "shared_data_cap_exceeded"

// statNameDynamicStatsCapExceeded pins the byte-exact envoy-go-strict
// suffix for the per-dynamic-stats-cap-exceeded counter per 25.2 SPEC §7.1
// row 13 + Q9. Full wire name: `wasm.<plugin_name>.dynamic_stats_cap_
// exceeded`. Incremented when `proxy_define_metric` exceeds the dynamic-
// stats entry cap (1024 default per Q9); the define call returns
// ErrCapExceeded → `WasmResult::InternalFailure`.
const statNameDynamicStatsCapExceeded = "dynamic_stats_cap_exceeded"

// statNameHttpCallResponseAfterClose pins the byte-exact envoy-go-strict
// suffix for the late-response-after-stream-closed counter per 25.2 SPEC
// §7.1 row 14 + AMEND-B3 (NEW vs BRAINSTORM Q9 8-counter tally — AMEND-B3
// added counter 14 as a defensive observability extension). Full wire name:
// `wasm.<plugin_name>.http_call_response_after_close`. Incremented when an
// outbound HTTP call's response arrives AFTER the originating stream
// context has been closed (near-zero in healthy operation; non-zero signal
// pages an operator that envoy-go's cancellation path has a bug).
const statNameHttpCallResponseAfterClose = "http_call_response_after_close"

// -----------------------------------------------------------------------------
// filterStats — 5-counter holder per AMEND-A2 tri-group prefix structure.
// -----------------------------------------------------------------------------

// filterStats is the 5-counter per-listener stat-surface holder per
// AMEND-A2 + 25.1 SPEC §4.2 + parent §7. Allocated at `wasm.New` time via
// `newFilterStats(reg, pluginName)`; SHARED across the listener's per-stream
// `*filter` instances (no per-route stats at 25.1; per-route PARSE-REJECTs
// per parent §6.2 arm 18).
//
// Field grouping mirrors the AMEND-A2 tri-group prefix structure:
//
//   - Group B (wasm.wazero.* — upstream-parity): created counter +
//     active gauge.
//
//   - envoy-go-strict extensions (wasm.<plugin_name>.* — per AMEND-A2):
//     executions + hostcallDenied + envoyGoFailures.
//
// Per ADR-0085 nil-tolerance: consumers (Tasks 11 + 12) MUST nil-check
// before incrementing — test-double paths may construct *filter values
// without stat wiring.
type filterStats struct {
	// Group B (wasm.wazero.* — upstream-parity per AMEND-A2):

	// created — per VM construction (incr at Task 12 NewVM).
	created *stats.Counter
	// active — live VM count (incr on construct; decr on Close at Task 12).
	active *stats.Gauge

	// envoy-go-strict extensions (wasm.<plugin_name>.* — per AMEND-A2):

	// executions — per `proxy_on_request_headers` invocation
	// (envoy-go-strict; departure at BEHAVIOR_CONTRACT.md Task 17).
	executions *stats.Counter
	// hostcallDenied — per default-denied hostcall invocation
	// (envoy-go-strict; anchors AMEND-A5 default-deny capability sandbox).
	hostcallDenied *stats.Counter
	// envoyGoFailures — per VM-failure event from the panic-wrapper
	// (envoy-go-strict; departure at BEHAVIOR_CONTRACT.md Task 17).
	envoyGoFailures *stats.Counter

	// 25.2 IMPL Task 16 EXTENSION (per 25.2 SPEC §7.1 + Q2 + AMEND-B3):

	// bodyBufferCapExceeded — per-stream first-cap-exceeded event from
	// Task 16 body.go (envoy-go-strict; departure at BEHAVIOR_CONTRACT.md
	// Task 22 consolidated 9-counter bundle). Sticky flag at *filter ensures
	// one increment per stream regardless of post-cap chunk count. Co-
	// incremented with envoyGoFailures per 25.2 §2.25.
	bodyBufferCapExceeded *stats.Counter

	// -------------------------------------------------------------------------
	// 25.2 IMPL Task 17 EXTENSIONS — 8 NEW envoy-go-strict counters per
	// 25.2 SPEC §7.1 + AMEND-B3. Together with bodyBufferCapExceeded above
	// (added at Task 16) these complete the 9 envoy-go-strict counters of
	// the 25.2 stat-surface delta. Consolidated departure record at
	// BEHAVIOR_CONTRACT.md Task 22 atomic-landing (record #1, per ADR-0208).
	// -------------------------------------------------------------------------

	// tickInvocations — per `proxy_on_tick` invocation incremented by
	// *RootVM.lockAndDispatchTick (Task 5 tick.go). Operator visibility into
	// tick dispatch rate. Wire name: `wasm.<plugin>.tick_invocations`.
	tickInvocations *stats.Counter

	// httpCallDispatched — per successful proxy_http_call dispatch from
	// Task 8 http_call.go (cluster lookup OK; AsyncClient request started).
	// Wire name: `wasm.<plugin>.http_call_dispatched`.
	httpCallDispatched *stats.Counter

	// httpCallResponse — per proxy_on_http_call_response invocation routed
	// to a live stream context (Task 8 http_call.go). Wire name:
	// `wasm.<plugin>.http_call_response`.
	httpCallResponse *stats.Counter

	// foreignFunctionDenied — per `proxy_call_foreign_function` returning
	// NotFound on the EMPTY default registry path per AMEND-A9. Wire name:
	// `wasm.<plugin>.foreign_function_denied`.
	foreignFunctionDenied *stats.Counter

	// httpCallDispatchUnknownCluster — per proxy_http_call to an unknown
	// cluster (BadArgument per AMEND-B3 + upstream context.cc:1547-1550).
	// Wire name: `wasm.<plugin>.http_call_dispatch_unknown_cluster`.
	httpCallDispatchUnknownCluster *stats.Counter

	// sharedDataCapExceeded — per `proxy_set_shared_data` exceeding the
	// value cap (1 MiB default per Q6) OR entry-count cap (1024 default);
	// the host returns `WasmResult::InternalFailure`. Co-incremented with
	// envoyGoFailures per 25.2 §2.25. Wire name:
	// `wasm.<plugin>.shared_data_cap_exceeded`.
	sharedDataCapExceeded *stats.Counter

	// dynamicStatsCapExceeded — per `proxy_define_metric` exceeding the
	// dynamic-stats entry cap (1024 default per Q9); the define call returns
	// `WasmResult::InternalFailure`. Wire name:
	// `wasm.<plugin>.dynamic_stats_cap_exceeded`.
	dynamicStatsCapExceeded *stats.Counter

	// httpCallResponseAfterClose — defensive observability counter per
	// AMEND-B3: increments when an outbound HTTP call's response arrives
	// AFTER the originating stream context has been closed (i.e. the cancel-
	// at-destruction race per AMEND-B3 had a stray response slip through).
	// Near-zero in healthy operation; non-zero signal pages an operator that
	// envoy-go's cancellation path has a bug. Wire name:
	// `wasm.<plugin>.http_call_response_after_close`.
	httpCallResponseAfterClose *stats.Counter
}

// -----------------------------------------------------------------------------
// newFilterStats — 5-counter registration per AMEND-A2 tri-group structure.
// -----------------------------------------------------------------------------

// newFilterStats constructs the 5-counter surface under the AMEND-A2 tri-
// group prefix structure:
//
//   - Group B (shared per-runtime; runtime = "wazero" uniformly at 25.1):
//     `wasm.wazero.created` + `wasm.wazero.active`. Registered via
//     NewCounterIfAbsent / NewGaugeIfAbsent (ADR-0117 idempotent registration)
//     so multiple plugin configs on the same listener share the same Group-B
//     counters — they are PER-RUNTIME, not per-plugin.
//
//   - envoy-go-strict per-plugin (`wasm.<plugin_name>.*`):
//     `wasm.<pluginName>.executions` + `wasm.<pluginName>.hostcall_denied` +
//     `wasm.<pluginName>.envoy_go.failures`. Registered via NewCounter
//     (each plugin config produces a fresh per-plugin namespace; collisions
//     between identically-named plugins on the same listener would surface as
//     a registry panic — that is the intended boot-time-fail-fast posture
//     per ADR-0072).
//
// Per ADR-0085 nil-tolerance: returns nil if `reg` is nil; consumers nil-check
// before incrementing (Tasks 11 + 12). HCM-injected `stats_prefix` is NOT
// consumed here — the AMEND-A2 structural decision DROPS it; the wasm filter
// row DIVERGES from the dominant §9 family-row pattern (recorded at
// BEHAVIOR_CONTRACT.md final-Task structural-note row).
//
// `pluginName` is the `PluginConfig.name` discriminator per AMEND-A2 Group C.
// Empty pluginName produces literal consecutive-dot wire names (e.g.
// `wasm..executions`) — mirrors the phase-22.1 lua empty-`<config_stat_prefix>`
// precedent; the `internal/stats/registry.go::nameRE` regex PERMITS interior
// consecutive dots (it only rejects trailing dots). Whether to reject empty
// pluginName at parse time is a Task 9 PARSE-REJECT-roster decision (anticipated
// at parent §6.2; not enforced here at the stats-registration layer).
//
// Cardinality at 25.1 phase-done: exactly 5 stats per plugin instance (2
// shared Group-B + 3 envoy-go-strict per-plugin). The project-wide stat-count
// delta was 114 → 119.
//
// Cardinality at 25.2 phase-done (this Task 17 + Task 16 wiring): exactly 14
// stats per plugin instance — 2 shared Group-B + 12 envoy-go-strict per-plugin
// (3 from 25.1 + 9 NEW per 25.2 SPEC §7.1 + AMEND-B3). The project-wide
// stat-count delta is 119 → 128 (+9 per fresh-registry call; verified at
// TestNewFilterStats_ProjectStatCountDelta in wasm_test.go).
func newFilterStats(reg *stats.Registry, pluginName string) *filterStats {
	if reg == nil {
		return nil
	}
	// Group B (shared per-runtime; ADR-0117 idempotent NewCounterIfAbsent /
	// NewGaugeIfAbsent so multiple plugins on the same listener don't panic
	// on duplicate-register).
	created := reg.NewCounterIfAbsent(statNameCreated)
	active := reg.NewGaugeIfAbsent(statNameActive)

	// envoy-go-strict per-plugin (`wasm.<pluginName>.*`).
	base := "wasm." + pluginName + "."
	return &filterStats{
		created:               created,
		active:                active,
		executions:            reg.NewCounter(base + statNameExecutions),
		hostcallDenied:        reg.NewCounter(base + statNameHostcallDenied),
		envoyGoFailures:       reg.NewCounter(base + statNameEnvoyGoFailures),
		bodyBufferCapExceeded: reg.NewCounter(base + statNameBodyBufferCapExceeded),

		// 25.2 IMPL Task 17 — 8 NEW envoy-go-strict counters per §7.1 +
		// AMEND-B3 (combined with bodyBufferCapExceeded above completes the
		// 9-counter delta; project total 119 → 128 per AMEND-B3).
		tickInvocations:                reg.NewCounter(base + statNameTickInvocations),
		httpCallDispatched:             reg.NewCounter(base + statNameHttpCallDispatched),
		httpCallResponse:               reg.NewCounter(base + statNameHttpCallResponse),
		foreignFunctionDenied:          reg.NewCounter(base + statNameForeignFunctionDenied),
		httpCallDispatchUnknownCluster: reg.NewCounter(base + statNameHttpCallDispatchUnknownCluster),
		sharedDataCapExceeded:          reg.NewCounter(base + statNameSharedDataCapExceeded),
		dynamicStatsCapExceeded:        reg.NewCounter(base + statNameDynamicStatsCapExceeded),
		httpCallResponseAfterClose:     reg.NewCounter(base + statNameHttpCallResponseAfterClose),
	}
}

// -----------------------------------------------------------------------------
// RootStatsRecorder satisfaction per 25.2 IMPL Task 20 follow-up (Concern 2).
// -----------------------------------------------------------------------------
//
// *filterStats satisfies the internal/wasm.RootStatsRecorder interface via
// thin per-counter wrapper methods on a POINTER receiver. The wasm package
// holds the *filterStats reference indirectly (via the RootStatsRecorder
// interface field on *RootVM) so the per-RootVM hostcall bodies can call
// rv.stats.XxxInc() without importing the filter package (would cycle:
// filter imports wasm).
//
// Each wrapper guards against a nil counter field via a nil-check (per
// ADR-0085 nil-tolerance — test-double paths may construct a partial
// *filterStats with some counter fields unset; production wiring at
// newFilterStats always populates all 10 fields).
//
// Per 25.2 IMPL Task 20 follow-up the production wiring path is:
//
//	rootOpts := []internalwasm.RootVMOption{ ..., internalwasm.WithRootStats(stats), ... }
//	rootVM, err := internalwasm.NewRootVM(ctx, mod, rootCtxID, rootOpts...)
//
// where `stats` is the *filterStats constructed by newFilterStats. The
// RootStatsRecorder interface contract is satisfied at compile time by the
// methods below; the runtime invocation path is the rv.stats.XxxInc() call
// at the corresponding hostcall body (tick.go / shared_data.go / foreign.go /
// http_call.go / dynamic_stats.go).

// TickInvocationsInc increments the tick_invocations counter per 25.2 SPEC
// §7.1 row 6 + Q5. Wired into tick.go lockAndDispatchTick via the
// RootStatsRecorder interface.
func (fs *filterStats) TickInvocationsInc() {
	if fs == nil || fs.tickInvocations == nil {
		return
	}
	fs.tickInvocations.Inc()
}

// HttpCallDispatchedInc increments the http_call_dispatched counter per
// 25.2 SPEC §7.1 row 7 + Q4 + AMEND-B3. Wired into http_call.go
// DispatchHttpCall OK path.
func (fs *filterStats) HttpCallDispatchedInc() {
	if fs == nil || fs.httpCallDispatched == nil {
		return
	}
	fs.httpCallDispatched.Inc()
}

// HttpCallResponseInc increments the http_call_response counter per 25.2
// SPEC §7.1 row 8 + Q4. Wired into http_call.go handleHttpCallResponse
// live-stream path.
func (fs *filterStats) HttpCallResponseInc() {
	if fs == nil || fs.httpCallResponse == nil {
		return
	}
	fs.httpCallResponse.Inc()
}

// ForeignFunctionDeniedInc increments the foreign_function_denied counter
// per 25.2 SPEC §7.1 row 9 + AMEND-A9. Wired into root_vm.go
// CallForeignFunction NotFound path.
func (fs *filterStats) ForeignFunctionDeniedInc() {
	if fs == nil || fs.foreignFunctionDenied == nil {
		return
	}
	fs.foreignFunctionDenied.Inc()
}

// BodyBufferCapExceededInc increments the body_buffer_cap_exceeded counter
// per 25.2 SPEC §7.1 + Q2 + AMEND-B3. The body.go path at Task 16 already
// wires this counter directly via f.cfg.stats.bodyBufferCapExceeded.Inc();
// this wrapper exists for interface completeness so the wasm package could
// also bump it through the same recorder.
func (fs *filterStats) BodyBufferCapExceededInc() {
	if fs == nil || fs.bodyBufferCapExceeded == nil {
		return
	}
	fs.bodyBufferCapExceeded.Inc()
}

// HttpCallDispatchUnknownClusterInc increments the http_call_dispatch_
// unknown_cluster counter per 25.2 SPEC §7.1 row 11 + Q4 + AMEND-B3. Wired
// into http_call.go DispatchHttpCall unknown-cluster path.
func (fs *filterStats) HttpCallDispatchUnknownClusterInc() {
	if fs == nil || fs.httpCallDispatchUnknownCluster == nil {
		return
	}
	fs.httpCallDispatchUnknownCluster.Inc()
}

// SharedDataCapExceededInc increments the shared_data_cap_exceeded counter
// per 25.2 SPEC §7.1 row 12 + Q6. Wired into shared_data.go SetSharedData
// value-cap + entry-cap paths. Callers MUST also call EnvoyGoFailuresInc
// per §2.25.
func (fs *filterStats) SharedDataCapExceededInc() {
	if fs == nil || fs.sharedDataCapExceeded == nil {
		return
	}
	fs.sharedDataCapExceeded.Inc()
}

// DynamicStatsCapExceededInc increments the dynamic_stats_cap_exceeded
// counter per 25.2 SPEC §7.1 row 13 + Q9. Wired into dynamic_stats.go
// DefineMetric ErrCapExceeded path. Callers MUST also call EnvoyGoFailuresInc
// per §2.25.
func (fs *filterStats) DynamicStatsCapExceededInc() {
	if fs == nil || fs.dynamicStatsCapExceeded == nil {
		return
	}
	fs.dynamicStatsCapExceeded.Inc()
}

// HttpCallResponseAfterCloseInc increments the http_call_response_after_
// close counter per 25.2 SPEC §7.1 row 14 + AMEND-B3. Wired into
// http_call.go handleHttpCallResponse token-miss / stream-gone path.
// Near-zero in healthy operation; non-zero signal pages an operator that
// envoy-go's cancellation has a bug.
func (fs *filterStats) HttpCallResponseAfterCloseInc() {
	if fs == nil || fs.httpCallResponseAfterClose == nil {
		return
	}
	fs.httpCallResponseAfterClose.Inc()
}

// EnvoyGoFailuresInc increments the envoy_go.failures counter per §2.25
// co-increment discipline. ANY envoy-go-strict surface that fails a stream
// MUST call this so operators see one number reflecting all wasm-driven
// stream failures. Wired into shared_data.go + dynamic_stats.go cap-
// exceeded paths + the panic-recovery path in CallForeignFunction.
func (fs *filterStats) EnvoyGoFailuresInc() {
	if fs == nil || fs.envoyGoFailures == nil {
		return
	}
	fs.envoyGoFailures.Inc()
}

// Compile-time guard: *filterStats satisfies internalwasm.RootStatsRecorder.
// Surfaces a build error immediately if the interface roster drifts or if
// any of the 10 wrapper methods are renamed without coordinating with the
// interface declaration in internal/wasm/stats_recorder.go.
var _ internalwasm.RootStatsRecorder = (*filterStats)(nil)
