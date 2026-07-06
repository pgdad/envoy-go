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
	"github.com/pgdad/envoy-go/internal/stats"
	internalwasm "github.com/pgdad/envoy-go/internal/wasm"
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
// 25.3 IMPL Task 8 — 4 NEW stat-name suffixes per phase-25.3 AMEND-C3 +
// AMEND-C4. Group C = vm_reload triplet (3) + env_vars_cap_exceeded (1).
// Combined these push the project-wide stat-count from 128 → 132. Full wire
// names assembled at registry-call time: `wasm.<plugin_name>.<suffix>`.
// -----------------------------------------------------------------------------

// statNameVmReloadSuccess pins the byte-exact envoy-go-strict suffix for the
// per-successful-reload counter per phase-25.3 AMEND-C3 (FAIL_RELOAD reload
// triplet). Full wire name: `wasm.<plugin_name>.vm_reload_success`.
// Incremented by the reload state machine via the reloadStatsHook seam
// (Task 3) when a re-instantiation succeeds (Running ← Failed transition).
// Departure record at Task 15 BEHAVIOR_CONTRACT.md.
const statNameVmReloadSuccess = "vm_reload_success"

// statNameVmReloadRuntimeFailure pins the byte-exact envoy-go-strict suffix
// for the per-reload-attempt-that-failed counter per phase-25.3 AMEND-C3.
// Full wire name: `wasm.<plugin_name>.vm_reload_runtime_failure`. Incremented
// when a re-instantiation attempt fails (the VM stays Failed). Departure
// record at Task 15 BEHAVIOR_CONTRACT.md.
const statNameVmReloadRuntimeFailure = "vm_reload_runtime_failure"

// statNameVmReloadBackoff pins the byte-exact envoy-go-strict suffix for the
// per-dispatch-blocked-by-backoff counter per phase-25.3 AMEND-C3. Full wire
// name: `wasm.<plugin_name>.vm_reload_backoff`. Incremented at each dispatch
// entry where the VM is Failed and still inside the backoff window (no
// reload attempt on that dispatch). Departure record at Task 15.
const statNameVmReloadBackoff = "vm_reload_backoff"

// statNameEnvVarsCapExceeded pins the byte-exact envoy-go-strict suffix for
// the env-vars-cap-exceeded parse-reject counter per phase-25.3 AMEND-C4.
// Full wire name: `wasm.<plugin_name>.env_vars_cap_exceeded`. Intended to be
// incremented at the arm-C env_vars cap-reject path in compiled_config.go;
// see ordering note at compiled_config.go parseEnvVars call site (the stats
// object is constructed AFTER the arm-C check, so the increment is a
// documented limitation — the counter is allocated per plugin but cannot be
// bumped on the arm-C reject since no compiledConfig is built). Departure
// record at Task 15 BEHAVIOR_CONTRACT.md.
const statNameEnvVarsCapExceeded = "env_vars_cap_exceeded"

// -----------------------------------------------------------------------------
// filterStats — 18-counter holder per AMEND-A2 tri-group prefix structure
// (origin: 5 at 25.1; +9 at 25.2; +4 at 25.3).
// -----------------------------------------------------------------------------

// filterStats is the per-listener stat-surface holder per AMEND-A2 + 25.1
// SPEC §4.2 + parent §7 (origin: 5 counters at 25.1; EXTENDED to 14 at 25.2
// per Q9 + AMEND-B3; EXTENDED to 18 at 25.3 — the vm_reload_* Group-C triplet
// + env_vars_cap_exceeded per ADR-0211). Allocated at `wasm.New` time via
// `newFilterStats(reg, pluginName)`; SHARED across the listener's per-stream
// `*filter` instances. Per-route overrides (ADR-0210; landed at 25.3) build
// their own per-route `filterStats` via `NewCounterIfAbsent` (post-freeze
// tolerant — the BUG-1 fix).
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

	// -------------------------------------------------------------------------
	// 25.3 IMPL Task 8 EXTENSIONS — Group C: vm_reload triplet + env_vars_cap_exceeded
	// (4 NEW envoy-go-strict counters per phase-25.3 AMEND-C3/C4). These push
	// the project-wide stat-count from 128 → 132.
	// -------------------------------------------------------------------------

	// vmReloadSuccess — per successful VM re-instantiation under FAIL_RELOAD
	// (Running ← Failed transition). Incremented via the reloadStatsHook seam
	// at Task 3 (WithReloadStats). Wire name:
	// `wasm.<plugin>.vm_reload_success`.
	vmReloadSuccess *stats.Counter

	// vmReloadRuntimeFailure — per failed VM re-instantiation attempt under
	// FAIL_RELOAD (VM stays Failed). Incremented via the reloadStatsHook seam.
	// Wire name: `wasm.<plugin>.vm_reload_runtime_failure`.
	vmReloadRuntimeFailure *stats.Counter

	// vmReloadBackoff — per dispatch blocked by the reload backoff window (VM
	// is Failed but still within the backoff interval; no reload attempt on
	// that dispatch). Incremented via the reloadStatsHook seam. Wire name:
	// `wasm.<plugin>.vm_reload_backoff`.
	vmReloadBackoff *stats.Counter

	// envVarsCapExceeded — per arm-C env_vars cap-exceeded parse-reject event
	// per AMEND-C4. Allocated per-plugin; intended to be incremented at the
	// arm-C cap-reject path in compiled_config.go (see ordering note at the
	// parseEnvVars call site — the stats object is not yet constructed at the
	// arm-C check point). Wire name: `wasm.<plugin>.env_vars_cap_exceeded`.
	envVarsCapExceeded *stats.Counter
}

// -----------------------------------------------------------------------------
// newFilterStats — 18-counter registration per AMEND-A2 tri-group structure
// (origin: 5 at 25.1; +9 at 25.2; +4 at 25.3).
// -----------------------------------------------------------------------------

// newFilterStats constructs the 18-counter surface under the AMEND-A2 tri-
// group prefix structure (the 5 origin 25.1 counters shown below, plus the
// 25.2 +9 and 25.3 +4 extensions registered in the same constructor):
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
// Cardinality at 25.2 phase-done (Task 17 + Task 16 wiring): exactly 14
// stats per plugin instance — 2 shared Group-B + 12 envoy-go-strict per-plugin
// (3 from 25.1 + 9 NEW per 25.2 SPEC §7.1 + AMEND-B3). The project-wide
// stat-count delta is 119 → 128 (+9 per fresh-registry call).
//
// Cardinality at 25.3 Task 8 phase-done (this Task 8): exactly 18 stats per
// plugin instance — 2 shared Group-B + 16 envoy-go-strict per-plugin (12 from
// 25.2 + 4 NEW per AMEND-C3/C4: vm_reload_success + vm_reload_runtime_failure
// + vm_reload_backoff + env_vars_cap_exceeded). The project-wide stat-count
// delta is 128 → 132 (+4 per fresh-registry call; verified at
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
	//
	// BUG-1 fix (phase-25.3 Task 11-fix): these per-plugin counters are
	// registered via NewCounterIfAbsent rather than NewCounter so the
	// registration is FREEZE-TOLERANT. The per-route HTTP filter build path
	// (DecodeHeaders → resolveEffective → parsePerRouteWasm →
	// buildCompiledConfig → newFilterStats) runs POST-BOOT, when the shared
	// *stats.Registry is already frozen — NewCounter would panic via
	// checkFrozenLocked ("stats: registry frozen: cannot register %q
	// post-boot"). NewCounterIfAbsent is documented (registry.go) as PERMITTED
	// post-Freeze for exactly this data-driven-per-route case; it appends under
	// r.mu even when frozen. The per-plugin names are unique (discriminated by
	// `<pluginName>`), so idempotent registration is harmless on the normal
	// boot path AND safe on the post-freeze per-route path.
	base := "wasm." + pluginName + "."
	return &filterStats{
		created:               created,
		active:                active,
		executions:            reg.NewCounterIfAbsent(base + statNameExecutions),
		hostcallDenied:        reg.NewCounterIfAbsent(base + statNameHostcallDenied),
		envoyGoFailures:       reg.NewCounterIfAbsent(base + statNameEnvoyGoFailures),
		bodyBufferCapExceeded: reg.NewCounterIfAbsent(base + statNameBodyBufferCapExceeded),

		// 25.2 IMPL Task 17 — 8 NEW envoy-go-strict counters per §7.1 +
		// AMEND-B3 (combined with bodyBufferCapExceeded above completes the
		// 9-counter delta; project total 119 → 128 per AMEND-B3).
		tickInvocations:                reg.NewCounterIfAbsent(base + statNameTickInvocations),
		httpCallDispatched:             reg.NewCounterIfAbsent(base + statNameHttpCallDispatched),
		httpCallResponse:               reg.NewCounterIfAbsent(base + statNameHttpCallResponse),
		foreignFunctionDenied:          reg.NewCounterIfAbsent(base + statNameForeignFunctionDenied),
		httpCallDispatchUnknownCluster: reg.NewCounterIfAbsent(base + statNameHttpCallDispatchUnknownCluster),
		sharedDataCapExceeded:          reg.NewCounterIfAbsent(base + statNameSharedDataCapExceeded),
		dynamicStatsCapExceeded:        reg.NewCounterIfAbsent(base + statNameDynamicStatsCapExceeded),
		httpCallResponseAfterClose:     reg.NewCounterIfAbsent(base + statNameHttpCallResponseAfterClose),

		// 25.3 IMPL Task 8 — 4 NEW envoy-go-strict counters per AMEND-C3/C4
		// (Group C: vm_reload triplet + env_vars_cap_exceeded; project total
		// 128 → 132).
		vmReloadSuccess:        reg.NewCounterIfAbsent(base + statNameVmReloadSuccess),
		vmReloadRuntimeFailure: reg.NewCounterIfAbsent(base + statNameVmReloadRuntimeFailure),
		vmReloadBackoff:        reg.NewCounterIfAbsent(base + statNameVmReloadBackoff),
		envVarsCapExceeded:     reg.NewCounterIfAbsent(base + statNameEnvVarsCapExceeded),
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
// newFilterStats always populates all 18 fields at 25.3 Task 8).
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

// -----------------------------------------------------------------------------
// 25.3 IMPL Task 8 — 4 NEW Inc methods (Group C: vm_reload triplet +
// env_vars_cap_exceeded). The first 3 satisfy the internal/wasm.reloadStatsHook
// interface so *filterStats can be passed via WithReloadStats(cfg.stats) to
// wire the reload state machine counters. All follow the ADR-0085 nil-
// tolerance pattern.
// -----------------------------------------------------------------------------

// VmReloadSuccessInc increments the vm_reload_success counter per phase-25.3
// AMEND-C3. Wired via WithReloadStats into the reload state machine's
// Running ← Failed success branch (dispatchReload in root_vm.go).
func (fs *filterStats) VmReloadSuccessInc() {
	if fs == nil || fs.vmReloadSuccess == nil {
		return
	}
	fs.vmReloadSuccess.Inc()
}

// VmReloadRuntimeFailureInc increments the vm_reload_runtime_failure counter
// per phase-25.3 AMEND-C3. Wired via WithReloadStats into the reload state
// machine's re-instantiation-failed branch (dispatchReload in root_vm.go).
func (fs *filterStats) VmReloadRuntimeFailureInc() {
	if fs == nil || fs.vmReloadRuntimeFailure == nil {
		return
	}
	fs.vmReloadRuntimeFailure.Inc()
}

// VmReloadBackoffInc increments the vm_reload_backoff counter per phase-25.3
// AMEND-C3. Wired via WithReloadStats into the reload state machine's
// dispatch-blocked-by-backoff path (dispatchReload in root_vm.go).
func (fs *filterStats) VmReloadBackoffInc() {
	if fs == nil || fs.vmReloadBackoff == nil {
		return
	}
	fs.vmReloadBackoff.Inc()
}

// EnvVarsCapExceededInc increments the env_vars_cap_exceeded counter per
// phase-25.3 AMEND-C4. Intended for the arm-C cap-reject path in
// compiled_config.go; see ordering note at the parseEnvVars call site (the
// stats object is not yet constructed at arm-C check time, so this increment
// is a documented limitation on that path). Available for future call sites
// where stats are live.
func (fs *filterStats) EnvVarsCapExceededInc() {
	if fs == nil || fs.envVarsCapExceeded == nil {
		return
	}
	fs.envVarsCapExceeded.Inc()
}

// Compile-time guard: *filterStats satisfies internalwasm.RootStatsRecorder.
// Surfaces a build error immediately if the interface roster drifts or if
// any of the 10 RootStatsRecorder wrapper methods are renamed without
// coordinating with the interface declaration in
// internal/wasm/stats_recorder.go. (The 25.3 vm_reload_* triplet +
// env_vars_cap_exceeded wrappers are filter-package-only — they do NOT flow
// through the RootStatsRecorder interface, so the interface roster stays at
// 10 methods.)
var _ internalwasm.RootStatsRecorder = (*filterStats)(nil)
