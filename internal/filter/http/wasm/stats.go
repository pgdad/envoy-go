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
// Cardinality: exactly 5 stats per plugin instance at 25.1 phase-done (2 shared
// Group-B + 3 envoy-go-strict per-plugin). The project-wide stat-count delta is
// 114 → 119 per parent §7 (verified indirectly at the +5 per-call delta test
// in wasm_test.go).
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
		created:         created,
		active:          active,
		executions:      reg.NewCounter(base + statNameExecutions),
		hostcallDenied:  reg.NewCounter(base + statNameHostcallDenied),
		envoyGoFailures: reg.NewCounter(base + statNameEnvoyGoFailures),
	}
}
