// Package ratelimit implements the envoy.filters.http.ratelimit HTTP filter
// under the 07.1 HTTP filter framework.
//
// Phase 24.1: the CORE decision path of the external-gRPC global rate-limit
// filter — the SEVENTEENTH ADR-0072-registered HTTP filter on the §9 table
// (17 at 24.1; alphabetical neighbor between oauth2 and rbac in the boot
// registry, Task 7). Lands the descriptor-action engine for the 5 CORE actions
// (generic_key, request_headers, remote_address, destination_cluster,
// header_value_match), the OK/OVER_LIMIT/error dispositions, the cluster-
// scoped 4-counter stat surface, and the gRPC RateLimit Service round-trip
// per ADR-0197 (CORE slice).
//
// # TypeURL
//
// The canonical proto TypeURL registered at boot per ADR-0143 SN1 +
// ADR-0072:
//
//	type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit
//
// # 24.1 / 24.2 split boundary
//
// Phase 24 was split at the PLAN-time ADR-0201 carve. 24.1 lands the CORE
// decision path; 24.2 (`24.2-global-ratelimit-perroute-and-headers`) lands
// the X-RateLimit DRAFT_VERSION_03 response headers (`encode.go` +
// `headers.go`), the `RateLimitPerRoute` typed-per-route 10th canonical
// (`compiled_perroute.go`), the remaining 5 descriptor actions
// (`source_cluster`, `masked_remote_address`, `metadata`, `query_parameters`,
// `query_parameter_value_match`), the `stage` multi-stage filtering, and the
// Axis-B `vh_rate_limits` cross-tier composition decision table.
//
// 24.1 parses `enable_x_ratelimit_headers` into `compiledConfig` (it is a
// PARSE-time field per AMEND-3) but does NOT emit the X-RateLimit response
// headers — the encode-side injection point is STUBBED with a forward
// pointer to 24.2 per D-RL7.
//
// # Cross-references
//
//   - ADR-0197 (CORE slice landed at 24.1): filter package shape +
//     5-core-action engine + RateLimitClient + OK/OVER_LIMIT/error
//     dispositions + cluster-scoped 4-counter stat surface + deterministic
//     shared-fake differential.
//   - ADR-0198 (DELTA-2 HCM route-table `rate_limits` exposure):
//     `RouteRateLimits()` + `VirtualHostRateLimits()` accessor pair seeded
//     onto the per-stream FilterChain at HCM dispatch (Task 5).
//   - ADR-0200 (§5 PARSE-REJECT roster): the FULL §5.1 + §5.2 byte-stable
//     roster (Task 3); 15 → 18 envoy-go-strict departures.
//   - Parent SPEC §1.1 AMEND-1..11 catalog; parent §4 descriptor-action
//     engine; parent §5 PARSE-REJECT roster; parent §6 code shapes;
//     parent §11 D1–D7 empirical-pin matrix.
//   - 24.1 SPEC §4 source-file roster; 24.1 SPEC §6 six-gate checklist.
package ratelimit
