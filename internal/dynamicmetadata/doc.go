// Package dynamicmetadata is the NEW per-stream cross-filter dynamic-
// metadata accessor framework primitive at the NEW top-level package
// `internal/dynamicmetadata/` per ADR-0190 (§Context anchored at phase
// 22.2 SPEC `0d6463e`; §Decision + §Consequences body lands at phase
// 22.2 IMPL atomic-landing Task 19 per ADR-0044 in-place edit
// discipline).
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
//   - Bucket — opaque per-stream cross-filter dynamic-metadata accessor;
//     unexported map keyed by (filterName string, key string) →
//     *structpb.Value; NOT goroutine-safe across streams.
//   - NewBucket() *Bucket — constructs an initialized empty per-stream
//     metadata bucket.
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
// The FilterChain (internal/filter/http/chain.go) gains a new
// dynamicMetadata *dynamicmetadata.Bucket field. At chain construction
// (per-stream entry per ADR-0033), the field is initialized via
// NewBucket. At OnDestroy, chain.dynamicMetadata.Reset() is called.
// The filter-callback API surface (internal/filter/http/callbacks.go)
// gains two new accessors:
//
//   - DecoderFilterCallbacks.DynamicMetadata() *dynamicmetadata.Bucket
//     (returns the per-stream bucket).
//   - EncoderFilterCallbacks.DynamicMetadata() *dynamicmetadata.Bucket
//     (returns the SAME bucket — per-stream shared across decode +
//     encode dispatch).
//
// # Thread-safety contract
//
// Per-stream sequential per ADR-0033 (Envoy HCM dispatches the filter
// chain on a single worker thread per stream; no cross-filter
// concurrency within a stream). The Bucket is therefore NOT goroutine-
// safe across streams — each stream owns its own Bucket. Within a
// stream, sequential dispatch implies no mutex is required at the
// Bucket level (matching the upstream Envoy CommonComponents
// PerFilterStateImpl pattern); confirmed in SPEC §3.1 paragraph 2.
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
//   - ADR-0188 (paired prior §9 framework primitive — internal/lua/).
//   - ADR-0189 (paired prior §9 package — internal/filter/http/lua/).
//   - ADR-0033 (per-stream sequential filter dispatch).
//   - ADR-0085 (nil-tolerance discipline).
//   - ADR-0044 (ADR §Decision + §Consequences in-place body landing).
//   - 22.2 SPEC §3.1 — production API signatures.
//   - 22.2 SPEC §1.6 — cross-phase deferral-lift expectation.
//   - 22.2 BRAINSTORM Q3 + Q9 — cross-phase deferral-break trigger.
package dynamicmetadata
