// Package header_mutation implements the envoy.filters.http.header_mutation
// HTTP filter under the 07.1 HTTP filter framework.
//
// Phase 10: real Envoy filter, wire-shape pinned by SPEC §11.1–§11.5
// empirical scrapes of reference Envoy v1.37.2.
//
// Decode side (per SPEC §6.6):
//
//  1. Apply listener-level cfg.requestOps in proto-declared order
//     (per the proto comment at header_mutation.pb.go:141–142:
//     "filter configuration will always be applied first").
//  2. Resolve all 3 per-route tiers via dcb.RequestRouteConfigsAllTiers
//     (Route, VirtualHost, RouteConfiguration; unmerged).
//  3. Compile each non-nil tier's request_mutations into compiledMutationOp
//     slices.
//  4. Apply tiers in flag-controlled order:
//     - mostSpecificHeaderMutationsWins=false (DEFAULT):
//     Route → VirtualHost → RouteConfiguration
//     (least-specific applied LAST, wins overlap)
//     - mostSpecificHeaderMutationsWins=true:
//     RouteConfiguration → VirtualHost → Route
//     (most-specific applied LAST, wins overlap)
//  5. Return Continue.
//
// Encode side (per SPEC §6.8): symmetric algorithm against response_mutations
// using the SAME dcb.RequestRouteConfigsAllTiers callback (per planner-time
// decision 1 — DECODER-ONLY callback used from BOTH decode AND encode bodies;
// mirrors cors precedent at cors.go:163).
//
// Concurrency model (per SPEC §5.7): per-instance state is race-free by the
// single-goroutine-per-stream invariant per ADR-0071 (no synchronization
// needed); *runtimeConfig is read-only after New (multiple per-request *filter
// instances share via closure capture — read-only sharing is race-free); no
// timer goroutines, no shared atomic state, no SendLocalReply path. The
// maximally simple concurrency model.
//
// Public surface (per SPEC §6.1):
//
//	TypeURL = "type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation"
//	New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)
//
// New body discipline (per ADR-0109 + ADR-0111):
//
//  1. tc must be non-nil (boot-fail-fast per ADR-0072).
//  2. Unmarshal tc to *envoyextensionsfiltershttpheadermutationv3.HeaderMutation.
//  3. Compile listener-level mutations.{request,response}_mutations via
//     compileOps; each headerName validated against the protected set per
//     §11.1 (the 6-name set: ":method", ":path", ":authority", ":scheme",
//     ":status", "host" case-insensitive on host). Returns error on first
//     protected-header violation with verbatim format
//     "header_mutation: %q is :-prefixed or host; may not be modified"
//     mirroring Envoy v1.37.2's source/server/server.cc:453 message.
//  4. Capture mostSpecificHeaderMutationsWins flag.
//  5. Construct *runtimeConfig.
//  6. Register per-route validator via ctx.Registry.RegisterPerRouteValidator
//     (per ADR-0110 + planner-time decision 3) so HCM-build-time
//     BuildPerRouteConfig surfaces per-route protected-header violations
//     identical-in-effect to listener-level (boot-fail-fast).
//  7. Return FilterInstanceFactory closure.
//
// Stats: ZERO emitted (per SPEC §11.3 confirmation; analogous to cors per
// ADR-0074). The 3-field FactoryCtx per ADR-0100 stays as-is; phase 10 does
// not consume ctx.Stats or ctx.StatPrefix.
//
// Cross-cutting ADR anchors:
//
//   - ADR-0108: package shape + boot registration
//   - ADR-0109: runtimeConfig + compiledMutationOp + AppendAction × 4 +
//     keep_empty_value semantics + multi-value collapse / preserve per §11.4
//   - ADR-0110: multi-tier per-route evaluation (ResolveAllTiers +
//     RequestRouteConfigsAllTiers + RegisterPerRouteValidator + accessor-
//     choice discipline + cross-tier algorithm); amends ADR-0073
//   - ADR-0111: protected-header set + CONFIG-LOAD-TIME rejection (MAJOR
//     amendment to BRAINSTORM Decision 11)
//   - ADR-0112: mutations.query_parameter_mutations[] DEFERRED
//   - ADR-0113: header-value formatter substitution DEFERRED
//
// Forward-pointers (deferred per ADR-0040 format):
//
//   - mutations.query_parameter_mutations[] (KeyValueMutation triple +
//     path-query rewriting subsystem) — silently parsed; no behavioral
//     effect (ADR-0112).
//   - Header-value formatter substitution syntax (%REQ(:path)% etc) — values
//     materialized verbatim as static strings (ADR-0113).
package header_mutation
