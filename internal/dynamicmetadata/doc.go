// Package dynamicmetadata is the cross-filter dynamic-metadata accessor
// framework primitive at the top-level package `internal/dynamicmetadata/`
// per ADR-0190 (§Context anchored at phase 22.2 SPEC `0d6463e`; §Decision +
// §Consequences body lands at phase 22.2 IMPL atomic-landing Task 19 per
// ADR-0044 in-place edit discipline).
//
// # Scope-agnostic Bucket lifetime (per ADR-0217)
//
// The Bucket is SCOPE-AGNOSTIC: it carries an owner-determined lifetime —
// per-stream (HTTP filter chain) OR per-connection (network filter chain).
// The owner constructs the Bucket at its scope's entry, hands it to the
// filters via the callback accessor (HTTP: DecoderFilterCallbacks /
// EncoderFilterCallbacks.DynamicMetadata(); network:
// ReadFilterCallbacks.DynamicMetadata()), and Resets it at OnDestroy. The
// package logic is identical at either scope — there is NO per-scope code
// branch (R5: the per-connection consumer REUSES the package unchanged; the
// only difference is which owner constructs + owns the Bucket). The first
// per-connection production consumer is the rbac_network L4 filter's
// shadow-pair writes (shadow_engine_result + shadow_effective_policy_id;
// internal/filter/network/rbac, ADR-0217).
//
// # Cross-phase deferral-break rationale
//
// Phases 16 (rbac) / 17 (jwt_authn) / 18 (ext_authz) / 19 (ext_proc) /
// 20 (oauth2) each deferred dynamic-metadata access by their respective
// filters with BEHAVIOR_CONTRACT.md "operator-visibility deferred to
// future" notes. The deferral was about IMPL — each phase chose not to
// be the first to land a cross-filter state primitive — NOT about
// principle. Phase 22.2 lands the cross-filter primitive at first co-
// consumer (the HTTP Lua filter's :streamInfo():dynamicMetadata() +
// :dynamicTypedMetadata(filter_name) bridge methods) per phase-22.2
// BRAINSTORM Q3 + Q9 EXTRACT-NOW. Future phase BRAINSTORMs that need
// dynamic-metadata access from their respective filters consume
// `internal/dynamicmetadata/` rather than defer again — each prior-
// phase BEHAVIOR_CONTRACT note converts from "deferred" to "lifted via
// `internal/dynamicmetadata`" at the lift-phase's next-touchpoint.
//
// # API surface (per 22.2 SPEC §3.1 production signatures)
//
//   - Bucket — opaque scope-agnostic cross-filter dynamic-metadata
//     accessor; unexported map keyed by (filterName string, key string) →
//     *structpb.Value; NOT goroutine-safe across owning scopes.
//   - NewBucket() *Bucket — constructs an initialized empty metadata
//     bucket (per-stream OR per-connection, owner's choice).
//   - (*Bucket).Get(filterName, key) (*structpb.Value, bool) — returns
//     the value at (filterName, key); ok=false if absent. Nil-receiver
//     tolerant: returns (nil, false) per ADR-0085.
//   - (*Bucket).Set(filterName, key, value) — writes value at
//     (filterName, key); overwrites any prior value; auto-initializes
//     the inner map. Nil-receiver tolerant: no-op per ADR-0085.
//   - (*Bucket).Snapshot() map[string]map[string]*structpb.Value —
//     returns a defensive copy of the outer + inner maps for read-only
//     iteration (consumed by the Lua bridge's :dynamicTypedMetadata()
//     typed-iteration access). Mutating the snapshot does NOT mutate
//     the bucket. The *structpb.Value pointers are shallow-shared.
//     Nil-receiver tolerant: returns nil per ADR-0085.
//   - (*Bucket).Reset() — clears all entries; consumed at OnDestroy.
//     Nil-receiver tolerant: no-op per ADR-0085.
//
// # Lifecycle integration
//
// HTTP (per-stream): The FilterChain (internal/filter/http/chain.go) gains
// a new dynamicMetadata *dynamicmetadata.Bucket field. At chain
// construction (per-stream entry per ADR-0033), the field is initialized
// via NewBucket. At OnDestroy, chain.dynamicMetadata.Reset() is called.
// The filter-callback API surface (internal/filter/http/callbacks.go)
// gains two new accessors:
//
//   - DecoderFilterCallbacks.DynamicMetadata() *dynamicmetadata.Bucket
//     (returns the per-stream bucket).
//   - EncoderFilterCallbacks.DynamicMetadata() *dynamicmetadata.Bucket
//     (returns the SAME bucket — per-stream shared across decode +
//     encode dispatch).
//
// Network (per-connection, ADR-0217): the per-connection runtime context
// owns a Bucket constructed at connection entry; the network read-filter
// callback surface (internal/filter/network/callbacks.go) exposes it via
// ReadFilterCallbacks.DynamicMetadata() *dynamicmetadata.Bucket. The
// rbac_network filter is the first per-connection production WRITER
// (shadow-pair writes). The package logic is unchanged — only the owning
// scope differs (R5).
//
// # Thread-safety contract
//
// Per-stream sequential per ADR-0033 (Envoy HCM dispatches the filter
// chain on a single worker thread per stream; no cross-filter
// concurrency within a stream). The Bucket is therefore NOT goroutine-
// safe across streams (or connections) — each stream (or connection)
// owns its own Bucket. Network filter chains are likewise sequentially
// dispatched per connection (no cross-filter concurrency within a
// connection). Within a stream (or connection), sequential dispatch
// implies no mutex is required at the Bucket level (matching the
// upstream Envoy CommonComponents PerFilterStateImpl pattern);
// confirmed in SPEC §3.1 paragraph 2.
//
// # Nil-tolerance discipline (per ADR-0085)
//
// All four methods are nil-receiver tolerant per project nil-tolerance
// discipline (ADR-0085). On a nil *Bucket: Get returns (nil, false);
// Set is a no-op; Snapshot returns nil; Reset is a no-op. This allows
// consumers to call into the bucket without nil-guards in hot paths —
// a useful affordance for filters that may run before per-stream
// chain construction completes (e.g. early-error paths) or for tests
// that exercise the accessor without constructing a chain.
//
// # Cross-references
//
//   - ADR-0190 (NEW internal/dynamicmetadata/ framework primitive;
//     §Decision + §Consequences body lands at 22.2 IMPL Task 19).
//   - ADR-0217 (scope-agnostic Bucket lifetime; per-connection reuse at
//     the network filter chain — rbac_network shadow-pair writes, R5).
//   - ADR-0188 (paired prior §9 framework primitive — internal/lua/).
//   - ADR-0189 (paired prior §9 package — internal/filter/http/lua/).
//   - ADR-0033 (per-stream sequential filter dispatch).
//   - ADR-0085 (nil-tolerance discipline).
//   - ADR-0044 (ADR §Decision + §Consequences in-place body landing).
//   - 22.2 SPEC §3.1 — production API signatures.
//   - 22.2 SPEC §1.6 — cross-phase deferral-lift expectation.
//   - 22.2 BRAINSTORM Q3 + Q9 — cross-phase deferral-break trigger.
package dynamicmetadata
