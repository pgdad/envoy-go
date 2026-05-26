// Package filterstate is the generic per-stream filter-state framework
// primitive at the NEW top-level package `internal/filterstate/` per
// ADR-0207 (§Context anchored at phase-25.2 SPEC commit; §Decision +
// §Consequences body lands at phase-25.2 IMPL Task 22 atomic-landing
// per ADR-0044 in-place edit discipline).
//
// # Cross-filter primitive at second-consumer scope
//
// Phase 22.2 HTTP Lua filter shipped `:filterState()` IN-PACKAGE at
// `internal/filter/http/lua/filterstate.go` per the conservative
// EXTRACT-NOW-only-when-trigger-fires posture (no committed second
// consumer at that time). Phase 25.2 HTTP WASM filter's
// `proxy_get_property "filter_state.*"` + `"upstream_filter_state.*"`
// property branches per AMEND-B4 require the same primitive surface —
// the EXTRACT-NOW-on-second-consumer trigger fires per the discipline
// ADR-0188 established at phase-22.1 for `internal/lua/`.
//
// Consumer roster at Lands-in-Task (25.2 IMPL):
//
//   - Consumer #1: `internal/filter/http/lua/filterstate.go` MIGRATES
//     non-breaking at 25.2 IMPL Task 10 — the existing in-package
//     `map[string]any` storage layer flips to `*Bucket.Set/Get`
//     accessors via a thin adapter; the `:filterState()` Lua surface
//     stays UNCHANGED byte-identical to phase-22.2 IMPL (the 2 envoy-
//     go-strict divergences from upstream lua per phase-22.2 AMEND-22.2-4
//     — mutation exposure + typed Lua-value marshaling — carry forward
//     UNCHANGED).
//
//   - Consumer #2: `internal/filter/http/wasm/property.go` at 25.2 IMPL
//     Task 13 — the `proxy_get_property` host dispatcher's
//     `filter_state.*` + `upstream_filter_state.*` branches consume
//     two parallel `*Bucket` instances per per-stream context (per the
//     AMEND-B4 dual-root model described below).
//
// # AMEND-B4 — upstream_filter_state distinct root co-equal to filter_state
//
// The property-roster scrape against `envoyproxy/envoy@v1.37.2:source/
// extensions/filters/common/expr/context.h:87` revealed that
// `upstream_filter_state` is a DISTINCT property root co-equal to
// `filter_state` (BRAINSTORM Q7 OMITTED this; 25.2 SPEC §11.4 added it
// at the AMEND-B4 SUBSTANTIVE REFINEMENT). The Bucket primitive serves
// BOTH roots — the `*compiledConfig` constructs TWO `*Bucket` instances
// per `*RootVM` (one for downstream `filter_state`, one for upstream
// `upstream_filter_state`); wiring lands at 25.2 IMPL Task 14.
//
// # API surface (per 25.2 SPEC §3.2 production signatures)
//
//   - FilterStateObject — per-key interface; carries the typed data +
//     serialization (Marshal/Unmarshal) + the StateType discriminator.
//     Lua filter wraps Lua values; wasm filter wraps proxy-wasm property
//     byte arrays.
//
//   - StateType — read-only-vs-mutable discriminator (StateTypeReadOnly
//     = 0; StateTypeMutable = 1). Pinned at ADR-0207 §Context; external
//     consumers (Task 10 lua adapter + Task 13 wasm property dispatch)
//     read these as ordinary integer constants.
//
//   - Bucket — per-stream filter-state accessor; opaque struct holding
//     an internal `map[string]FilterStateObject` + `sync.RWMutex`.
//     One Bucket per stream per root (downstream + upstream).
//
//   - NewBucket() *Bucket — constructs an initialized empty bucket.
//
//   - (*Bucket).Set(key, obj) error — inserts or replaces the entry at
//     key. Conflict semantics per ADR-0207 §Context (read-only-vs-mutable
//     conflict mirrors upstream Envoy v1.37.2 FilterState::setData): an
//     existing StateTypeMutable + new StateTypeReadOnly is REJECTED
//     (returns ErrReadOnlyOverMutable); all other combinations replace-
//     or-insert OK. Nil obj is REJECTED (returns ErrNilFilterStateObject)
//     so the presence/absence signal at Get is unambiguous.
//
//   - (*Bucket).Get(key) (FilterStateObject, bool) — returns the entry
//     at key + a presence bool. Absent key → (nil, false).
//
//   - (*Bucket).Keys() []string — sorted slice of all entry keys for
//     deterministic property-tree enumeration at the proxy-wasm
//     `filter_state.*` dispatch. Empty bucket → non-nil empty slice
//     (callers may range without a nil guard).
//
// # Thread-safety contract
//
// Per ADR-0207 §Context: the per-stream access path is single-goroutine
// in practice (envoy-go's filter dispatch model serializes per-stream
// access per ADR-0033). The internal `sync.RWMutex` is defensive against
// rare cross-goroutine touches that may surface at:
//
//   - the wasm tick-goroutine writing filter-state while the dispatch
//     goroutine reads via a `proxy_get_property` host path,
//   - future cross-filter primitives that may dispatch concurrently,
//   - test-time concurrent-stress harnesses (see
//     bucket_concurrency_test.go).
//
// Lock order in production: the *Bucket's mu is innermost — callers
// MUST NOT call back into the *Bucket while holding any outer lock that
// can be acquired by another goroutine that also calls into the *Bucket.
//
// # EXPLICIT API-REVISION ALLOWANCE clause (per ADR-0207 §Decision body
// anchored at 25.2 IMPL Task 22)
//
// The primitive's API shape is PROVISIONAL at consumer #2 (25.2 wasm).
// Future consumers — rbac filter-state read (phase-16 + future);
// ext_authz filter-state inject (phase-18 + future); ext_proc filter-
// state pass-through (phase-19 + future); new filter families (network-
// filter / cluster-specifier / WASM host family network-filter-wasm +
// access-logger-wasm + cluster-specifier-wasm) — MAY require API
// revision after empirical validation. Mirrors phase-22.1 ADR-0188 +
// phase-22.2 ADR-0190 + phase-25.1 ADR-0202 allowance pattern at the
// symmetric scope.
//
// The future-consumer roster is STRUCTURALLY MORE COMMITTED than phase-
// 22.1 lua's speculative future-consumer roster because the EXTRACT-NOW-
// on-second-consumer trigger has ALREADY fired (consumer #1 + consumer
// #2 land in the same 25.2 IMPL session). Future consumers consume the
// same primitive without re-litigating the abstraction shape; the API
// revision risk envelope at consumer #3+ is LOWER vs phase-22.1's
// first-consumer extraction.
//
// # ADR-0188 API-revision allowance NOT consumed
//
// ADR-0188 anchors `internal/lua/` (the VM-class primitive at the
// gopher-lua runtime scope). The `internal/filterstate/` extraction is
// a DISTINCT primitive at a DIFFERENT scope (filter-state vs lua-runtime).
// The `internal/lua/` framework primitive itself is untouched at 25.2;
// only the in-package `filterstate.go` file inside
// `internal/filter/http/lua/` migrates. ADR-0188's API-REVISION
// ALLOWANCE clause stays available for future `internal/lua/`
// evolutions at the consumer-#2 scope (e.g., a future
// lua_cluster_specifier filter).
//
// # Cross-references
//
//   - ADR-0207 (NEW internal/filterstate/ framework primitive at 25.2
//     second-consumer scope; §Decision + §Consequences body lands at
//     25.2 IMPL Task 22).
//   - ADR-0188 (paired precedent — NEW internal/lua/ framework
//     primitive at phase-22.1 second-consumer scope; the cross-phase
//     EXTRACT-NOW-on-second-consumer discipline anchor).
//   - ADR-0190 (paired precedent — NEW internal/dynamicmetadata/
//     framework primitive at phase-22.2 cross-filter scope; sibling
//     cross-filter primitive at the symmetric scope).
//   - ADR-0202 (paired precedent — phase-25.1 internal/wasm/ framework
//     primitive at HTTP-WASM first-consumer scope).
//   - ADR-0205 (root VM lifecycle — the *Bucket lives off the
//     *RootVM-derived per-stream context).
//   - ADR-0206 (25.2 ABI extensions including the proxy_get_property
//     host dispatcher that consumes this primitive at filter_state.* +
//     upstream_filter_state.* branches).
//   - ADR-0208 (paired filter-package extension ADR — co-anchors the
//     consumer #2 integration at `internal/filter/http/wasm/property.go`).
//   - 25.2 SPEC §3.2 — production API signatures.
//   - 25.2 SPEC §11.4 AMEND-B4 — upstream_filter_state addition +
//     property roster.
//   - 25.2 BRAINSTORM Q7 — second-consumer EXTRACT-NOW trigger.
//   - ADR-0033 — per-stream sequential filter dispatch (anchors the
//     single-goroutine in-practice access path).
package filterstate
