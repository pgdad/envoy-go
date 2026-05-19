package lua

// stats.go — 3-counter HCM-rooted stat surface per 22.1 SPEC §4.2 + parent §7 +
// AMEND-2 + AMEND-3. Lands the per-listener stat-counter holder + the
// `newFilterStats` constructor wired from `lua.New` at Task 10. Field-final at
// 22.1; 22.2 may add httpCall counters (settles at 22.2 SPEC); 22.3 adds 0 net-
// new (SHARED-vacuous per the 9th canonical's SHARED-stats discipline).
//
// # 3-counter stat-surface roster (per parent §7.1 + AMEND-3)
//
//   - errors        — counter — Script execution errors (panic-wrapper catches;
//                                upstream-parity per AMEND-3 + lua_filter.cc:811).
//   - executions    — counter — Script invocations (every envoy_on_request /
//                                envoy_on_response call; upstream-parity per
//                                AMEND-3 + lua_filter.cc:872).
//   - respond_calls — counter — :respond() short-circuit invocations
//                                (envoy-go-strict per AMEND-3 corrected;
//                                BEHAVIOR_CONTRACT.md §13.6 row 2 departure
//                                record at Task 16).
//
// All COUNTERS. Allocated unconditionally at filter New() time per the
// phase-17 jwt_authn + phase-18.1 ext_authz + phase-19.1 ext_proc + phase-20
// oauth2 + phase-21 adaptive_concurrency unconditional-allocation discipline.
// Per ADR-0085 nil-tolerance the caller `lua.New` guards `if ctx.Stats != nil`
// before invoking newFilterStats; consumers of cc.stats nil-check.
//
// # HCM-rooted stat-prefix template (per parent §7.2 + AMEND-2)
//
//   http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>
//
// `HCM_stat_prefix` is the HCM's `stat_prefix` proto field (e.g. "ingress_http"),
// supplied via `ctx.StatPrefix` at the New-call boundary; `config_stat_prefix`
// is the `Lua.stat_prefix` proto field, supplied by the per-filter config
// (defaults to "" if the operator did not set it). When `Lua.stat_prefix` is
// empty, names become `http.<HCM>.lua..errors` — literal consecutive dots —
// mirroring the phase-14 compressor empty-`<library>` precedent at
// BEHAVIOR_CONTRACT.md §line 243. AMEND-2 EXPLICITLY RATIFIES this literal
// consecutive-dot wire shape; the regex at `internal/stats/registry.go::nameRE`
// PERMITS interior consecutive dots (it only rejects trailing dots).
//
// # Compile-time stat-name guards per ADR-0143 SN2-reuse
//
// Two-layer guard:
//
//   1. Per-stat `statName*` const declarations pin the byte-exact wire suffix
//      at compile time. A regression on any wire name surfaces immediately at
//      build time (the constructor uses these constants).
//
//   2. The `TestStatNames_Equal_*` Group-9 assertion tests (in lua_test.go at
//      Task 10) pin each constant to its string literal — preventing a future
//      refactor from silently renaming the constant alongside the string
//      literal.
//
// # Cross-references
//
//   - ADR-0085 (nil-tolerance discipline; the caller guards `if ctx.Stats != nil`)
//   - ADR-0143 SN2-reuse (stat name as Go const reused by production + test)
//   - ADR-0072 (boot-time-fail-fast — newFilterStats runs at HCM-build time
//     per ADR-0061 LBP-1; the *stats.Registry must accept NewCounter pre-Freeze)
//   - parent SPEC §7 (3-counter roster) + AMEND-2 (HCM-rooted template) +
//     AMEND-3 (executions reclassification + respond_calls envoy-go-strict)
//   - 22.1 SPEC §4.2 (compiledConfig + filterStats wiring) + §8 (stat surface
//     cross-reference)

import (
	"github.com/esalaine/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Stat-name compile-time constants per ADR-0143 SN2-reuse + parent §7.1.
// -----------------------------------------------------------------------------

// statNameErrors pins the byte-exact upstream wire name for the script-
// execution-errors counter per parent §7.1 + AMEND-3. Incremented when the
// gopher-lua chunk run fails (script top-level error) OR when an
// envoy_on_request / envoy_on_response hook raises a runtime error. Matches
// upstream `lua_filter.cc:811` `stats_.errors_.inc()`. Consumed by
// decode_headers.go + encode_headers.go (Task 9).
const statNameErrors = "errors"

// statNameExecutions pins the byte-exact upstream wire name for the script-
// invocations counter per parent §7.1 + AMEND-3. Incremented per invocation
// of envoy_on_request / envoy_on_response (NOT per-success — matches
// upstream's instrumentation order). Matches upstream `lua_filter.cc:872`
// `stats_.executions_.inc()`. Consumed by decode_headers.go + encode_headers.go
// (Task 9).
const statNameExecutions = "executions"

// statNameRespondCalls pins the byte-exact wire name for the :respond()
// short-circuit invocations counter — **envoy-go-strict extension per
// AMEND-3 corrected** (NOT in upstream's `ALL_LUA_FILTER_STATS` macro;
// collision-free verified at parent §11.5.3). Provides operator visibility
// into how often Lua short-circuits the upstream call. Documented as
// envoy-go-strict departure record at BEHAVIOR_CONTRACT.md §13.6 row 2 per
// §14 edit #4 (lands at Task 16). Consumed by decode_headers.go (Task 9).
const statNameRespondCalls = "respond_calls"

// -----------------------------------------------------------------------------
// newFilterStats — 3-counter HCM-rooted registration per parent §7.2 +
// AMEND-2 + ADR-0143 SN2-reuse.
// -----------------------------------------------------------------------------

// newFilterStats constructs the 3 *stats.Counter handles under the HCM-rooted
// template per parent §7.2 + AMEND-2:
//
//	http.<hcmStatPrefix>.lua.<configStatPrefix>.<stat>
//
// hcmStatPrefix is the HCM's `stat_prefix` proto field (e.g. "ingress_http"),
// supplied via `ctx.StatPrefix` at the lua.New boundary. configStatPrefix is
// the `Lua.stat_prefix` proto field, extracted at lua.New from the unmarshaled
// Lua proto envelope. Empty configStatPrefix produces literal consecutive-dot
// wire names (e.g. `http.<HCM>.lua..errors`) — mirrors the phase-14 compressor
// empty-`<library>` precedent at BEHAVIOR_CONTRACT.md §line 243. The
// `internal/stats/registry.go::nameRE` regex PERMITS interior consecutive dots
// (it only rejects trailing dots), so the consecutive-dot pattern registers
// cleanly without per-prefix conditional logic.
//
// All 3 counters allocate unconditionally for scrape stability (Prometheus
// best practice; mirrors the established §9 family-row discipline). The
// nil-tolerance contract lives at the caller's `if ctx.Stats != nil` gate
// per ADR-0085 — this helper unconditionally dereferences `reg`.
//
// Cardinality: exactly 3 counters per filter instance. Asserted at
// `TestNewFilterStats_CardinalityAssertion` (lua_test.go).
//
// Consumer landed at Task 9 (decode_headers.go + encode_headers.go) +
// Task 10 (lua.New factory body wiring).
func newFilterStats(reg *stats.Registry, hcmStatPrefix, configStatPrefix string) *filterStats {
	base := "http." + hcmStatPrefix + ".lua." + configStatPrefix + "."
	return &filterStats{
		errors:       reg.NewCounter(base + statNameErrors),
		executions:   reg.NewCounter(base + statNameExecutions),
		respondCalls: reg.NewCounter(base + statNameRespondCalls),
	}
}
