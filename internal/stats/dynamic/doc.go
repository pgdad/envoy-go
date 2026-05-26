// Package dynamic is the per-plugin dynamic-stats Registry for the
// proxy-wasm `wasmcustom.<custom_name>` namespace per ADR-0208 +
// AMEND-B2 + R-25.2-7 (§Context anchored at phase-25.2 SPEC commit;
// §Decision + §Consequences body lands at phase-25.2 IMPL Task 22
// atomic-landing per ADR-0044 in-place edit discipline).
//
// # Wire-perspective name pin per AMEND-B2 REFINEMENT (REFUTES BRAINSTORM Q9)
//
// The proxy-wasm guest-defined dynamic-stats namespace is
// `wasmcustom.<custom_name>` ONLY — the plugin name is NOT prefixed
// into the wire-perspective stat namespace. BRAINSTORM Q9 hypothesized
// `wasmcustom.<plugin_name>.<custom_name>` based on a careless reading
// of upstream Envoy's stat-scope nesting; the AMEND-B2 SPEC §11.2
// empirical-pin scrape against Envoy v1.37.2 REFUTED the hypothesis:
//
//   - source/extensions/common/wasm/stats_handler.h:16:
//     constexpr absl::string_view CustomStatNamespace = "wasmcustom";
//   - source/extensions/common/wasm/context.cc:1623-1625:
//     Stats::Utility::counterFromElements(
//     *envoyWasm()->scope_,
//     {envoyWasm()->custom_stat_namespace_, stat_name})
//     — only the namespace string + the raw user-supplied stat name; the
//     plugin name is NOT interpolated as a prefix.
//
// envoy-go disposition: per-plugin isolation via per-plugin Registry
// SCOPE (NOT via wire-name prefix interpolation). Each `*compiledConfig`
// constructs its OWN `*dynamic.Registry` at config-load time, passing a
// `pluginScopePrefix` (e.g. `"wasm.<plugin_name>"`). The Registry
// registers the underlying *stats.Counter / *stats.Gauge under the full
// admin-side name `pluginScopePrefix + ".wasmcustom." + custom_name`;
// from the admin /stats operator's perspective, the metrics surface as
// `wasm.<plugin_name>.wasmcustom.<custom_name>` (the parent stat-scope
// nesting). From the wire-perspective (what the proxy-wasm guest
// observes via EnumerateForAdmin / the host-side metric_id token), the
// stat name is `wasmcustom.<custom_name>` byte-faithful to upstream.
// Cross-plugin name collisions are namespaced via the per-plugin
// pluginScopePrefix, NOT by prefix interpolation.
//
// # Signedness pin per AMEND-B2 (REFINES BRAINSTORM Q9 hostcall shapes)
//
//   - `proxy_increment_metric(metric_id, delta)`: delta is SIGNED `int64`
//     per cpp-host src/exports.cc:1065-1068 + Rust-SDK hostcalls.rs:
//     1395-1397; allows NEGATIVE gauge deltas (which would be lost if a
//     careless caller used unsigned uint64). This Registry's
//     Increment(id, delta int64) signature reflects the wire shape
//     byte-faithfully.
//
//   - `proxy_record_metric(metric_id, value)`: value is UNSIGNED `uint64`
//     per cpp-host src/exports.cc:1070-1073. This Registry's
//     Record(id, value uint64) signature reflects the wire shape
//     byte-faithfully.
//
//   - Counter / Gauge / Histogram MetricType byte-pin per AMEND-B2:
//     MetricTypeCounter=0, MetricTypeGauge=1, MetricTypeHistogram=2.
//
// # API surface (per 25.2 SPEC §3.3 production signatures)
//
//   - MetricID — uint32 opaque token returned from Register.
//
//   - MetricType — Counter / Gauge / Histogram discriminator.
//
//   - Registry — per-plugin dynamic-stats Registry; opaque struct
//     holding the byID + byName maps + the parent *stats.Registry +
//     the pluginScopePrefix + the maxEntries cap.
//
//   - NewRegistry(reg *stats.Registry, pluginScopePrefix string,
//     maxEntries uint32) *Registry — constructs a fresh Registry. Returns
//     nil if reg is nil (defensive nil-tolerance per ADR-0085).
//
//   - (*Registry).Register(metricType, name) (MetricID, error) —
//     idempotent: same (name, type) returns the cached MetricID; same
//     name with DIFFERENT type returns ErrBadArgument; new (name, type)
//     allocates a fresh MetricID + registers the underlying stats
//     primitive; new registration when the byID map already has
//     maxEntries entries returns ErrCapExceeded.
//
//   - (*Registry).Increment(id, delta int64) error — adds SIGNED delta
//     per AMEND-B2. Returns ErrBadArgument on Histogram (which doesn't
//     support increment); ErrBadArgument on Counter with negative delta
//     (the underlying *stats.Counter is monotonic-non-negative);
//     ErrNotFound on unknown id.
//
//   - (*Registry).Record(id, value uint64) error — sets UNSIGNED value
//     per AMEND-B2. Returns ErrBadArgument on Counter (which doesn't
//     support set); ErrNotFound on unknown id.
//
//   - (*Registry).Get(id) (uint64, error) — returns the current value
//     of the metric. Returns ErrNotFound on unknown id.
//
//   - (*Registry).EnumerateForAdmin(fn func(name string, value uint64))
//     walks the registry for /stats lazy enumeration. The callback
//     receives the wire-perspective name (`wasmcustom.<custom_name>`)
//     plus the current value. The admin /stats endpoint may use this for
//     proxy-wasm-guest-visible stat enumeration in a Layer-A consumer at
//     a future phase; the parent stats.Registry's Walk already surfaces
//     the same metrics under the full admin-side name
//     (`wasm.<plugin>.wasmcustom.<custom_name>`) for the operator-visible
//     Prometheus scrape.
//
// # Histogram primitive — ADR-0060 deferred-primitive disposition
//
// The parent `internal/stats/` package defers Histogram per ADR-0060
// (full histogram primitive lands at a later sub-phase). At phase 25.2
// the proxy-wasm guest may call proxy_define_metric with
// MetricTypeHistogram + proxy_record_metric to surface a histogram-like
// signal. To honor the wire-shape pin without prematurely materializing
// a heavyweight histogram primitive, this Registry stores Histogram-typed
// metric values in an IN-PACKAGE atomic.Uint64 stub (the latest Record-ed
// value is the metric's current value). The stub is NOT registered with
// the parent *stats.Registry — the admin /stats endpoint surfaces it only
// via this Registry's EnumerateForAdmin callback. A future ADR-0060
// follow-up may replace the stub with a full histogram primitive; the
// API surface (Register/Record/Get/EnumerateForAdmin) stays unchanged.
//
// # Thread-safety contract
//
// The Registry's sync.RWMutex serializes:
//   - byID + byName map mutations + the nextID allocation + the
//     maxEntries cap check at Register (Write Lock).
//   - byID lookups + the underlying-primitive pointer reads at
//     Increment / Record / Get / EnumerateForAdmin (Read Lock).
//
// The underlying *stats.Counter / *stats.Gauge primitives use lock-free
// atomic operations at Inc/Add/Set/Load (per phase-06.1 ADR-0059) so
// the hot Increment/Record path takes ONLY a RLock on this Registry +
// hits a single atomic; concurrent Increments on the same MetricID
// scale linearly under -race. Histogram Record/Get use an in-package
// atomic.Uint64.
//
// # Cap-discipline (envoy-go-strict per Q9 + AMEND-B2)
//
// envoy-go-strict 1024-entry cap on the dynamic-stats namespace. The
// per-plugin cap is enforced at Register; the consumer-side wire-up
// (Task 12 internal/wasm/dynamic_stats.go + Task 14 compiled_config.go)
// adds the `wasm.<plugin>.dynamic_stats_cap_exceeded` envoy-go-strict
// counter + the `envoy_go_strict_dynamic_stats_max_entries` envoy-go-
// strict-only config field per 25.2 SPEC §7.1 #8 + §7.4. The cap is
// a SOFT POSTURE — a guest exceeding the cap gets ErrCapExceeded
// returned by Register (and at the consumer wire-up layer the host
// callback returns WasmResult::InternalFailure + bumps the
// dynamic_stats_cap_exceeded counter + envoy_go.failures counter per
// §2.25 scope extension).
//
// # Cross-references
//
//   - ADR-0208 (NEW internal/filter/http/wasm/ 25.2 package extensions
//     consumer ADR — co-anchors this dynamic-stats Registry at the
//     §Decision body landing at 25.2 IMPL Task 22).
//   - ADR-0206 (paired ABI extensions ADR — co-anchors the 4 metric
//     hostcalls + the MetricType byte-pin per AMEND-B2).
//   - ADR-0117 (NewCounterIfAbsent / NewGaugeIfAbsent post-Freeze
//     idempotent registration — this Registry's Register uses the
//     IfAbsent variants to permit config-load-time registration
//     post-Freeze).
//   - ADR-0059 (parent stats package — lock-free atomic primitives
//     anchor the hot Increment path).
//   - ADR-0060 (Histogram deferred — this Registry uses an in-package
//     atomic.Uint64 stub for the Histogram primitive pending the
//     full-Histogram landing).
//   - ADR-0085 (nil-tolerance — NewRegistry(nil, ...) returns nil).
//   - 25.2 SPEC §3.3 — production API signatures (Registry struct
//     fields per the SPEC are conceptually mapped: the SPEC's
//     `pluginScope *stats.Scope` field maps to this implementation's
//     `pluginScopePrefix string` + the parent `*stats.Registry`,
//     because the parent stats package does NOT yet expose a Scope
//     type — the flat-Registry-with-hierarchical-dotted-names model
//     suffices for the per-plugin scoping discipline; a future
//     `*stats.Scope` ADR would refactor this transparently behind the
//     public NewRegistry signature).
//   - 25.2 SPEC §7.3 — admin /stats name format
//     `wasm.<plugin>.wasmcustom.<custom>`.
//   - 25.2 SPEC §11.2 AMEND-B2 — wire-shape pins (signed-int64 delta;
//     namespace `wasmcustom.<custom_name>` ONLY).
//   - 25.2 BRAINSTORM Q9 — plugin-prefix-interpolation hypothesis
//     REFUTED.
//   - R-25.2-2 — metric signedness + dynamic-stats namespace per
//     AMEND-B2 (this Registry materializes the host-side bookkeeping;
//     wired at Tasks 12 + 14).
//   - R-25.2-7 — NEW internal/stats/dynamic/ infrastructure package
//     per ADR-0208 + AMEND-B2 (this is the Lands-in-Task package).
package dynamic
