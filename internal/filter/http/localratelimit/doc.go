// Package localratelimit implements the envoy.filters.http.local_ratelimit
// HTTP filter under the 07.1 HTTP filter framework.
//
// Phase 11: real Envoy filter, wire-shape pinned by SPEC §11.1–§11.8
// empirical scrapes of reference Envoy v1.37.2.
//
// Package-naming note (per ADR-0114): the directory name + Go package
// identifier are both `localratelimit` (no underscore), diverging from
// phase 10's header_mutation/ underscore-preserving pattern. The no-underscore
// form aligns with cors/ + fault/ whose proto type-names were already single
// tokens; the local_ratelimit proto type-name is the lone divergence in the
// §9 family-row set. ADR-0114 codifies the rationale: a single proto-name
// divergence does not justify flipping the existing 3-of-4-filters convention.
//
// Decode side (per SPEC §6.5):
//
//  1. Increment rc.stats.enabled (per-request unconditional).
//  2. Call rc.bucket.tryConsume() (lazy-refill on access; per ADR-0116).
//  3. If true: increment rc.stats.ok; return Continue.
//  4. If false: increment rc.stats.rateLimited AND rc.stats.enforced
//     (lockstep MVP invariant per ADR-0118; future shadow-mode phase widens
//     to enforced ≤ rate_limited when filter_enforced runtime-key support
//     lands per the Runtime + hot restart family).
//  5. Invoke f.dcb.SendLocalReply(rc.statusCode, rc.body,
//     OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}})
//     (per ADR-0102 + ADR-0119; reuse from phase 09 fault precedent at
//     internal/filter/http/fault/fault.go:321).
//  6. Return StopIteration.
//
// Encode side: pass-through (no encode-side state per SPEC §6.5 NOTE).
//
// Token-bucket primitive (per SPEC §6.4 + ADR-0116):
//
//   - Lazy refill on access (Option A); single sync.Mutex per bucket; no
//     per-bucket goroutine; no time.Ticker; no signal channel.
//   - time.Now().UnixNano() carries Go ≥1.9's monotonic component for
//     time.Now()-derived values; arithmetic across time.Now() calls
//     advances monotonically under wall-clock NTP corrections / leap seconds.
//   - Bucket lifetime = filter-instance lifetime (listener: process-exit;
//     per-route: process-exit since envoy-go is static-config-only per
//     BOOTSTRAP_PROMPT.md §3.1).
//   - LBP-1-adjacent: closure-capture half preserved (matches phase 06.1 /
//     09 / 10 discipline); lock-free hot-path half deliberately departs
//     since elapsed → refills → tokens is a multi-step CAS-resistant
//     computation. Mutex is the natural choice.
//
// Per-route override semantics (per SPEC §5.5 + §11.6 + ADR-0117):
//
//   - Wholesale-override per ADR-0073 + ADR-0117 (ADR-0073 amendment).
//   - Each per-route LocalRateLimit TPFC entry is parsed by the framework's
//     BuildPerRouteConfig into a generic proto.Message but is NOT recursively
//     dispatched to New; phase 11 builds each per-route *runtimeConfig (with
//     its own *tokenBucket + *filterStats) lazily at first-resolve time via a
//     per-filter sync.Map keyed by *LocalRateLimit proto pointer per ADR-0117.
//     (IMPL-1: the upstream LocalRateLimitPerRoute proto cited in SPEC does not
//     exist; per-route TPFC entries reuse the same LocalRateLimit proto directly.)
//   - The 3-tier PerRouteConfig.Resolve picks the most-specific config
//     per request; that config carries its own bucket pointer.
//   - Listener-level state is NOT touched for per-route reqs (per §11.6
//     empirical confirmation).
//
// Concurrency model (per SPEC §5.6):
//
//   - Per-bucket: single sync.Mutex guarding {tokens, lastRefillNs}.
//   - Per-filter-instance: *runtimeConfig is a *runtimeConfig reference
//     (closure-captured at boot-time New; never mutated); read-only thread-safe.
//   - Per-process: registry frozen at boot per ADR-0072; per-route TPFC
//     parsed at HCM-build time; bucket lifetime = process lifetime.
//
// Public surface (per SPEC §6.1):
//
//	TypeURL = "type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit"
//	New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)
//
// New body discipline (per ADR-0115):
//
//  1. tc must be non-nil (boot-fail-fast per ADR-0072).
//  2. Unmarshal tc to *localratelimitv3.LocalRateLimit.
//  3. Validate stat_prefix non-empty (PGV per §11.1).
//  4. Validate token_bucket non-nil + max_tokens > 0 (PGV per §11.2a).
//  5. Validate tokens_per_fill: absent → default 1 (per §11.2b-i);
//     explicit zero → reject (PGV per §11.2b-ii).
//  6. Validate fill_interval >= 50ms (FILTER-INTERNAL not PGV per §11.2c;
//     verbatim error string mirrors Envoy v1.37.2's
//     source/server/config_validation/server.cc:76 message).
//  7. Validate status.code: absent → default 429 (per §11.4); explicit
//     out-of-[400,600) → reject (PGV per §11.4).
//  8. Capture mostSpecificHeaderMutationsWins flag (NOT applicable to
//     local_ratelimit; this filter has no equivalent flag).
//  9. Construct *tokenBucket via newTokenBucket(maxTokens, tokensPerFill, fillInterval).
//  10. Construct *filterStats via newFilterStats(ctx.Stats, statPrefix).
//  11. Construct *runtimeConfig.
//  12. Return FilterInstanceFactory closure that allocates a fresh *filter
//     per request bound to *runtimeConfig.
//
// Stats: 4 counters per stat_prefix (per SPEC §6.6 + §11.5):
//
//	<stat_prefix>.http_local_rate_limit.enabled       (every req reaching the filter)
//	<stat_prefix>.http_local_rate_limit.ok            (tryConsume → true)
//	<stat_prefix>.http_local_rate_limit.rate_limited  (tryConsume → false)
//	<stat_prefix>.http_local_rate_limit.enforced      (tryConsume → false; lockstep MVP)
//
// Prometheus tag-extraction: the filter-specific tag-extractor `Rule SN9`
// (added to internal/stats/name.go's flattenToProm switch in Task 6 per
// ADR-0118 + planner-time decision 1) extracts the <stat_prefix> segment
// into the `envoy_local_http_ratelimit_prefix` Prometheus label.
//
// Cross-cutting ADR anchors:
//
//   - ADR-0114: package shape + boot registration (no-underscore directory)
//   - ADR-0115: runtimeConfig + 5/14-field decomposition + PGV table +
//     filter-internal `fill_interval >= 50ms` validation discipline
//   - ADR-0116: tokenBucket Option-A lazy-refill + LBP-1-adjacent +
//     ±10ms empirical refill-timing tolerance
//   - ADR-0117: per-route bucket isolation as ADR-0073 wholesale-override
//     consequence (FIRST stateful per-route filter; ADR-0073 amendment)
//   - ADR-0118: stat-table 22→26-name extension + `enforced == rate_limited`
//     MVP invariant + filter-specific Prometheus tag-extractor SN9
//   - ADR-0119: rate-limited response wire shape + body byte-exact +
//     4-header set + 429 default + SendLocalReply reuse from phase 09
//
// Forward-pointers (silent-ignored fields per SPEC §2.1 + ADR-0040 silent-
// ignore discipline; 14 fields organized by 8 family-clusters; full list at
// BEHAVIOR_CONTRACT §13.1 / §13.5):
//
//   - descriptors / rate_limits / always_consume_default_token_bucket /
//     max_dynamic_descriptors  — descriptor-action subsystem (couples to
//     global_ratelimit future phase)
//   - filter_enabled / filter_enforced / request_headers_to_add_when_not_enforced
//     — runtime + shadow-mode subsystem (couples to Runtime + hot restart;
//     DIVERGENCE-WINDOW: envoy-go silent-ignores; ref Envoy defaults to 0%
//     OFF; fixture configs MUST set both to 100% explicitly per SPEC §1.1)
//   - local_cluster_rate_limit  — xDS cluster-state subsystem
//   - response_headers_to_add  — response-side header injection
//   - local_rate_limit_per_downstream_connection  — per-connection lifecycle
//   - stage  — multi-stage limiting (couples to descriptor-action)
//   - enable_x_ratelimit_headers / vh_rate_limits  — X-RateLimit headers + vh policy
//   - rate_limited_as_resource_exhausted  — gRPC trailer mapping
package localratelimit
