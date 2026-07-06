package ratelimit

// stats.go — cluster-scoped cross-namespace 4-counter filterStats per phase-24
// SPEC §6.8 + AMEND-1 + AMEND-10. The FIRST landed cross-namespace cluster-
// stat-charge in the codebase: stats anchor at the UPSTREAM RLS cluster's
// stat scope, NOT the HCM stat_prefix where the filter is mounted.
//
// # Stat-surface roster (4 names: counters ONLY -- no gauges per AMEND-1)
//
// Wire-equivalent to upstream Envoy's COMMON ratelimit lib stat surface per
// source/extensions/filters/common/ratelimit/stat_names.h:15-18,30-33:
//
//   - ok                    counter (response decoded OK)
//   - error                 counter (rls call returned error)
//   - over_limit            counter (response decoded OVER_LIMIT)
//   - failure_mode_allowed  counter (failure_mode_deny=false admit-on-error)
//
// NO gauges, NO histograms, NO `cluster_not_found` counter (the BRAINSTORM
// hypothesis was REFUTED by AMEND-1 — the upstream macro is COUNTER-only;
// upstream publishes exactly 4 counters). Project stat count 110 -> 114
// (+4 counters; cluster-scoped per AMEND-1).
//
// # Cluster-scoped prefix per AMEND-1 + the upstream createPoolStatName shape
//
// The fully-qualified metric prefix mirrors stat_names.h:24-27's
// createPoolStatName: `ratelimit` + (statPrefix.empty() ? "" : "." + statPrefix)
// + "." + statName, resolved against `cluster_->statsScope()` at the upstream
// COMMON lib. Materialized in envoy-go as the absolute name:
//
//	cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<stat>
//
// where:
//   - `cluster.<rls_cluster_name>.` is the cluster-scope prefix synthesized at
//     this filter (no `*Cluster` helper exists — see AMEND-10 below).
//   - `ratelimit` is the literal infix (matches upstream `STAT_PREFIX` token
//     at stat_names.h:24).
//   - `[.<stat_prefix>]` is the optional filter-config `stat_prefix` modulator
//     (AMEND-1; empty by default — when empty the segment is elided wholesale,
//     including the leading dot).
//   - `<stat>` is one of the 4 leaf names below.
//
// # AMEND-10 cross-namespace write — NEW mechanism this filter LANDS
//
// `internal/cluster.Cluster` exposes NO Registry/scope/registration helper
// (only typed `IncUpstreamRqTotal()`/`IncStatusClass()` over its OWN fixed
// counters + `Name()`); NO existing filter writes outside its own
// `http.<HCM_stat_prefix>.<filter>.*` namespace. To emit the cluster-scoped
// roster, this filter self-registers via `(*stats.Registry).NewCounterIfAbsent`
// (registry.go:157; post-Freeze-safe idempotent registration per ADR-0117).
// Idempotency is load-bearing: when >=2 HCMs each mount a ratelimit filter
// pointing at the SAME RLS cluster, both filter-instances must charge into
// the SAME 4 counters — `NewCounterIfAbsent` re-returns the same `*Counter`
// handle on duplicate-name resolution (registry.go:161-164).
//
// This is the SAME novel cross-namespace pattern that ext_authz's
// `charge_cluster_response_stats` DEFERRED (parent BEHAVIOR_CONTRACT §6
// amendment 8: `cluster.<upstream>.ext_authz.{ok,denied,error}`); phase 24
// LANDS the mechanism (the global filter's stats have NO `http.`-scope
// alternative — these are the filter's ONLY stats). ADR-0197 §Decision
// explicitly sanctions the cross-namespace write.
//
// # Compile-time stat-name guards per SPEC §6.8 + ADR-0143 SN2-reuse
//
// Two-layer guard:
//
//  1. Package-level const declarations pin the byte-exact wire leaf names at
//     compile time. A regression on any wire name surfaces at build time
//     (the constructor uses these constants).
//
//  2. The TestStatNames_ByteStable assertion test in stats_test.go (this Task 4)
//     pins each constant to its string literal — preventing a future refactor
//     from silently renaming the const alongside the literal. Task 4 OWNS the
//     stat-name byte-exact assertion; later Tasks (5/6/7) do NOT re-assert
//     stat names.
//
// # ADR-0085 nil-tolerance
//
// `newFilterStats(nil, ...)` returns a non-nil `*filterStats` with all-nil
// counter fields (mirroring the fault filter precedent at
// internal/filter/http/fault/fault.go:234 `registerFaultStats`). Test code
// paths that do not allocate a `*stats.Registry` proceed without registration;
// the Task-7 disposition path nil-guards each `Inc()` call.

import (
	"github.com/pgdad/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Stat-name compile-time leaf constants per SPEC §6.8 + AMEND-1.
// Byte-exact wire names from upstream stat_names.h:15-18,30-33.
// -----------------------------------------------------------------------------

// statNameOK pins the byte-exact upstream wire-leaf name for the OK-decoded
// rate-limit response counter per AMEND-1. Incremented when the RLS returns
// `overall_code = OK` (admit-the-request path). Consumed by the Task-7
// dispositions.go OK arm.
const statNameOK = "ok"

// statNameError pins the byte-exact upstream wire-leaf name for the
// rate-limit-call-error counter per AMEND-1. Incremented when the gRPC
// ShouldRateLimit call returns a non-nil error (transport error / RLS
// internal error / timeout). Consumed by the Task-7 dispositions.go error arm
// (paired with statNameFailureModeAllowed when failure_mode_deny=false).
const statNameError = "error"

// statNameOverLimit pins the byte-exact upstream wire-leaf name for the
// OVER_LIMIT-decoded rate-limit response counter per AMEND-1. Incremented when
// the RLS returns `overall_code = OVER_LIMIT` (reject-the-request path,
// rate_limited_status default 429). Consumed by the Task-7 dispositions.go
// OVER_LIMIT arm.
const statNameOverLimit = "over_limit"

// statNameFailureModeAllowed pins the byte-exact upstream wire-leaf name for
// the fail-open admit counter per AMEND-1. Incremented INSTEAD of (alongside)
// the statNameError counter when the RLS call errored AND failure_mode_deny
// is false (the default) — the request is admitted (fail-open). When
// failure_mode_deny is true, only statNameError is incremented and the
// request is rejected with status_on_error. Consumed by the Task-7
// dispositions.go error+fail-open arm.
const statNameFailureModeAllowed = "failure_mode_allowed"

// -----------------------------------------------------------------------------
// filterStats — 4-counter wire-exact stat-surface holder per SPEC §6.8 + AMEND-1.
// -----------------------------------------------------------------------------

// filterStats holds the 4-counter cluster-scoped stat surface for one
// ratelimit filter instance per phase-24 SPEC §6.8 + AMEND-1 + AMEND-10.
// COUNTER-ONLY (no gauges, no histograms — upstream stat_names.h:15-18,30-33
// is COUNTER-only). Project stat count 110 -> 114 (+4 counters; cluster-scoped).
//
// All 4 counters share the SAME absolute prefix
// `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].` and are
// scrape-stable from listener-load onward (the constructor allocates them
// unconditionally — Prometheus best practice).
//
//nolint:unused // populated at Task 7 New-factory closure capture (the disposition path Inc-s these)
type filterStats struct {
	// ok — RLS returned `overall_code = OK` (admit-the-request) per AMEND-1.
	ok *stats.Counter

	// error — ShouldRateLimit gRPC call returned a transport/RLS error per
	// AMEND-1. Field name uses the predeclared identifier `error` for symmetry
	// with the stat-name leaf; the redeclaration is local to this struct and
	// does NOT shadow the package-level `error` interface outside this file.
	error *stats.Counter

	// overLimit — RLS returned `overall_code = OVER_LIMIT` (reject-the-request)
	// per AMEND-1.
	overLimit *stats.Counter

	// failureModeAllowed — RLS call errored AND failure_mode_deny=false
	// (fail-open admit) per AMEND-1. Incremented INSTEAD of (alongside) `error`
	// on the error-fail-open arm; not incremented on the error-fail-closed arm.
	failureModeAllowed *stats.Counter
}

// -----------------------------------------------------------------------------
// newFilterStats — cluster-scoped idempotent 4-counter registration per
// AMEND-1 + AMEND-10 + ADR-0085 nil-tolerance.
// -----------------------------------------------------------------------------

// newFilterStats constructs the 4-counter cluster-scoped roster under the
// AMEND-1 prefix template:
//
//	cluster.<clusterName>.ratelimit[.<statPrefix>].<leaf>
//
// where:
//   - `clusterName` is the upstream RLS cluster's name
//     (`compiledConfig.rlsClusterName` captured at Task 3 via
//     `rate_limit_service.grpc_service.envoy_grpc.cluster_name`).
//   - `statPrefix` is the optional filter-config `stat_prefix` field
//     (AMEND-3 13th field; empty by default — when empty the segment is
//     elided wholesale, including the leading dot).
//
// Registration uses `(*stats.Registry).NewCounterIfAbsent` (registry.go:157),
// the POST-Freeze-safe idempotent path per ADR-0117 — load-bearing across
// multiple listeners sharing one RLS cluster (each filter-instance gets the
// SAME `*Counter` handle for each leaf name; charges from all instances
// aggregate into the same atomic cell).
//
// ADR-0085 nil-tolerance: when `reg == nil` (test code paths without a
// Registry), returns a non-nil `*filterStats` with all-nil counter fields;
// the Task-7 disposition path nil-guards each `Inc()` call (mirrors the
// fault filter precedent at internal/filter/http/fault/fault.go:234
// `registerFaultStats`).
//
// Consumed at Task 7's `New` factory closure where the filter factory wires
// this constructor from the HCM build path (under an `if ctx.Stats != nil`
// gate at the caller side OR direct invocation with the nil-tolerance arm
// taking effect — both shapes are supported).
//
//nolint:unused // consumed at Task 7 New-factory closure (filter-stats registration in the per-stream factory)
func newFilterStats(reg *stats.Registry, clusterName, statPrefix string) *filterStats {
	if reg == nil {
		return &filterStats{} // all-nil; Task-7 disposition path nil-guards each Inc.
	}
	base := "cluster." + clusterName + ".ratelimit."
	if statPrefix != "" {
		base += statPrefix + "."
	}
	return &filterStats{
		ok:                 reg.NewCounterIfAbsent(base + statNameOK),
		error:              reg.NewCounterIfAbsent(base + statNameError),
		overLimit:          reg.NewCounterIfAbsent(base + statNameOverLimit),
		failureModeAllowed: reg.NewCounterIfAbsent(base + statNameFailureModeAllowed),
	}
}
