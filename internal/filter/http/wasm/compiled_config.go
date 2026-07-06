package wasm

// compiled_config.go — `compiledConfig` per-listener immutable post-parse
// config + `buildCompiledConfig` PARSE-REJECT roster per parent SPEC §6.2 +
// 25.2 SPEC §6 + D-P5 closure (25.1 byte-stable wording finalization at
// 25.1 Task 9) + 25.2 D-25.2-P5 closure (6 NEW arms wording finalization at
// 25.2 Task 14). Lands the per-listener config struct that Task 12 (25.1)
// + Tasks 15-18 (25.2) decode_headers.go / encode_headers.go hot-path
// consume via the per-stream *filter.cfg pointer closure-captured at the
// New factory body.
//
// # 18-arm PARSE-REJECT roster (per parent §6.2 + D-P5 byte-stable wording)
//
//   - Arm 1  (typed-config-required)                       — IMPL HERE
//   - Arm 2  (typed-config-unmarshal)                      — IMPL HERE (%w-wrapped)
//   - Arm 3  (config-required)                             — IMPL HERE
//   - Arm 4  (vm-config-required)                          — IMPL HERE
//   - Arm 5  (vm-config-code-required)                     — IMPL HERE
//   - Arm 6  (vm-config-code-remote-deferred)              — IMPL HERE
//   - Arm 7  (data-source-watched-directory-deferred)      — IMPL HERE
//   - Arm 8  (data-source-specifier-required)              — IMPL HERE
//   - Arm 9  (plugin-failure-policy / reload_config)       — LIFTED at 25.3 Task 7:
//                                                            failure_policy +
//                                                            reload_config now
//                                                            PARSED + CONSUMED via
//                                                            parseFailurePolicy.
//   - Arm 10 (plugin-fail-open)                            — LIFTED at 25.3 Task 7:
//                                                            fail_open now maps to
//                                                            FailurePolicyFailOpen.
//   - Arm 11 (vm-config-runtime-discriminator)             — IMPL HERE (%q-formatted)
//   - Arm 12 (vm-config-vm-id-duplicate)                   — RETIRED at 25.3 Task 7:
//                                                            duplicate vm_id now
//                                                            SHARES one *RootVM via
//                                                            the registry refcount
//                                                            (AcquireFor). No reject.
//   - Arm 13 (vm-config-environment-variables)             — LIFTED at 25.3 Task 7:
//                                                            now PARSED + CONSUMED via
//                                                            wasm.AssembleEnvVars +
//                                                            WithRootEnv.
//   - Arm 14 (vm-config-allow-precompiled-rejected)        — IMPL HERE
//   - Arm 15 (vm-config-nack-on-code-cache-miss-rejected)  — IMPL HERE
//   - Arm 16 (module-abi-version-rejected)                 — IMPL HERE (via
//                                                            errors.Is on
//                                                            wasm.ErrUnsupportedAbiVersion)
//   - Arm 17 (module-compile-failed)                       — IMPL HERE (%w-wrapped
//                                                            wazero compile error)
//   - Arm 18 (per-route)                                   — LIFTED at 25.3 Task 7:
//                                                            validatePerRouteWasm now
//                                                            validates the per-route
//                                                            Wasm shape via
//                                                            validateWasmConfigShape
//                                                            (validate-only run; no
//                                                            name-claim, no refcount).
//
// # 3 NEW 25.3 PARSE-REJECT arms (per phase-25.3 Task 7 + D-25.3-P2 closure)
//
//   - Arm A  (env-vars-key-collision)                      — IMPL HERE (%q-formatted)
//   - Arm B  (fail-open-and-failure-policy-both-set)       — IMPL HERE
//   - Arm C  (env-vars-cap-exceeded)                       — IMPL HERE
//
// # 6 NEW 25.2 PARSE-REJECT arms (per 25.2 SPEC §6.2 + D-25.2-P5 closure)
//
// The 25.2 EXTENSION adds 6 NEW arms covering 25.2-introduced surfaces (the
// 4 envoy-go-strict-only `PluginConfig` config fields per Qs 2/6/9 + the
// cross-PluginConfig duplicate-name validator that the 5-counter envoy-go-
// strict tri-group bundle + per-plugin dynamic-stats Registry both depend
// on). The 25.1 18-arm roster STAYS active at 25.2 verbatim per ADR-0080
// byte-stable wording discipline.
//
//   - Arm 19 (envoy-go-strict-body-buffer-cap-bytes-zero)         — IMPL HERE
//   - Arm 20 (envoy-go-strict-shared-data-value-cap-bytes-zero)   — IMPL HERE
//   - Arm 21 (envoy-go-strict-shared-data-max-entries-zero)       — IMPL HERE
//   - Arm 22 (envoy-go-strict-dynamic-stats-max-entries-zero)     — IMPL HERE
//   - Arm 23 (envoy-go-strict-body-buffer-cap-bytes-overlarge)    — IMPL HERE
//                                                                   (>1 GiB ceiling
//                                                                   per defense-in-
//                                                                   depth; operators
//                                                                   wanting >1 GiB
//                                                                   body buffers
//                                                                   should re-architect
//                                                                   via body-streaming
//                                                                   rather than buffer-
//                                                                   and-process)
//   - Arm 26 (cross-pluginconfig-duplicate-pluginconfig-name)     — IMPL HERE
//                                                                   (process-wide
//                                                                   registry consulted
//                                                                   at buildCompiledConfig
//                                                                   per-plugin scope-
//                                                                   uniqueness; the
//                                                                   5-counter envoy-go-
//                                                                   strict tri-group
//                                                                   bundle + the per-
//                                                                   plugin *dynamic.
//                                                                   Registry both key
//                                                                   off PluginConfig.
//                                                                   name)
//
// # D-P5 + D-25.2-P5 closure (per 25.1 SPEC §12-D-P5 + parent §6.2 + 25.2 §6.2)
//
// At 25.1 Task 9, the byte-stable wording for the original 18 arms was pinned
// via the package-private `parseReject*` constants below. At 25.2 Task 14
// (D-25.2-P5 closure), the byte-stable wording for the 6 NEW arms is pinned
// via the same constant-as-source-of-truth discipline. `TestParseRejectConstants_
// ByteStable` (compiled_config_test.go) is a table-driven test that asserts
// each of the 24 constants (18 from 25.1 + 6 NEW from 25.2) byte-exact against
// the SPEC wording. Per ADR-0044 atomic-edit discipline: any wording change
// touches both this file + the byte-exact roster in parent SPEC §6.2 / 25.2
// SPEC §6.2 in a single commit.
//
// # CompileCache scope per D-P-PLAN-5
//
// The `*wasm.CompileCache` is owned by the `*compiledConfig` (filter-config-
// instance scope; one cache per listener filter-chain mounting a wasm filter);
// GC-driven eviction (the cache lifetime equals the `compiledConfig` lifetime;
// eviction happens when the listener drains + the `compiledConfig` is released).
// NO cross-listener / cross-process global cache. Mirrors phase-22.1 D-P5
// disposition.
//
// # Forward-stub: resolveDataSource (Task 10)
//
// At Task 9 `resolveDataSource` is a package-private FORWARD STUB returning a
// sentinel error so the buildCompiledConfig pipeline compiles + the
// pre-resolveDataSource PARSE-REJECT arms (1-15 except 12) can be tested in
// isolation. Task 10 (`datasource.go`) replaces this stub with the full 4-arm
// AsyncDataSource.Local resolution body (InlineBytes / InlineString /
// Filename / EnvironmentVariable) + the wrapped PARSE-REJECT arms for
// resolution failure paths (file-not-found, env-var-not-set, etc.).
//
// # Cross-references
//
//   - ADR-0072 (boot-time-fail-fast — buildCompiledConfig runs at HCM-build,
//     errors surface to the operator as boot-time config mistakes)
//   - ADR-0080 (envoy-go-strict departure discipline + byte-stable error wording)
//   - ADR-0085 (nil-tolerance discipline — CompileCache lifetime tied to
//     compiledConfig; no nil-cache crash path)
//   - ADR-0202 (NEW internal/wasm/ framework primitive — CompileModule +
//     CompileCache contract; SandboxConfig + SanitizationConfig)
//   - ADR-0203 (NEW internal/filter/http/wasm/ package shape — §Decision body
//     records the 18-arm D-P5 wording finalization at Task 17)
//   - ADR-0204 (default-deny capability sandbox — anchors buildSandboxConfig
//     zero-value StrictDefaultDeny)
//   - ADR-0205 (root-VM lifecycle evolution; rootVM field + NewRootVM at New)
//   - ADR-0206 (25.2 ABI extensions; cap fields + foreignReg field + dynStats field)
//   - ADR-0208 (NEW internal/filter/http/wasm/ 25.2 package extensions — 4
//     envoy-go-strict-only config fields + 6 NEW PARSE-REJECT arms + per-
//     plugin *dynamic.Registry wiring; §Decision body records the D-25.2-P5
//     6-NEW-arm wording finalization at Task 14)
//   - AMEND-A1 (SanitizationConfig accept-empty discipline; ignored at 25.1)
//   - AMEND-A2 (5-counter stat surface; HCM-stats_prefix DROPPED — pluginName
//     drives Group-C envoy-go-strict per-plugin keys)
//   - AMEND-A5 (default-deny capability sandbox — INVERTS upstream empty-map-
//     allow-all)
//   - AMEND-A6 (envoy-go-strict-stricter ABI rejection — v0.1.0 + v0.2.0 +
//     missing-sentinel surfaces as arm 16)
//   - AMEND-A9 (foreign-function 0-vs-10 default registry — foreignReg field
//     points at wasm.DefaultForeignFunctionRegistry by default)
//   - AMEND-B2 (signed-i64 increment + unsigned-u64 record; dynamic-stats
//     namespace `wasmcustom.<custom_name>` via per-plugin Registry SCOPE)
//   - parent SPEC §6.1 (wording discipline) + §6.2 (18-arm roster) + §12-D-P5
//     (D-P5 closure anchor at 25.1 Task 9)
//   - 25.1 SPEC §4.2 (compiledConfig + filterStats wiring)
//   - 25.2 SPEC §4.2 (compiledConfig EXTENDED with rootVM + 4 envoy-go-strict-
//     only cap fields + dynStats + foreignReg) + §6.2 (6 NEW PARSE-REJECT
//     arms) + §7.4 (4 envoy-go-strict-only PluginConfig config fields per Qs
//     2/6/9) + §12-D-25.2-P5 (6 NEW arm byte-stable wording closure at Task 14)

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	wasmcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats/dynamic"
	internalwasm "github.com/pgdad/envoy-go/internal/wasm"
)

// -----------------------------------------------------------------------------
// PARSE-REJECT byte-stable wordings per parent SPEC §6.2 + D-P5 closure.
//
// EVERY constant prefix is `wasm:` per parent §6.1 + ADR-0080. Byte-exact
// wording pinned at compiled_config_test.go::TestParseRejectConstants_
// ByteStable. Any drift requires a parent-SPEC §6.2 + ADR-0203 lockstep
// edit per ADR-0044 atomic-edit discipline.
// -----------------------------------------------------------------------------

const (
	// Arm 1: typed_config required (no proto envelope provided at HCM-build).
	parseRejectTypedConfigRequired = "wasm: typed_config required"

	// Arm 2: typed_config.UnmarshalTo failure. Format string with %w to wrap
	// the inner proto-decoder error (carries proto-level context useful for
	// operator first-pass diagnostics). Applied via fmt.Errorf at the use site.
	parseRejectTypedConfigUnmarshal = "wasm: typed_config unmarshal: %w"

	// Arm 3: config (PluginConfig) is required. Wasm.config field unset.
	parseRejectConfigRequired = "wasm: config (PluginConfig) is required"

	// Arm 4: PluginConfig.vm_config is required. The oneof Vm is unset OR
	// the VmConfig sub-message is nil.
	parseRejectVmConfigRequired = "wasm: config.vm_config is required"

	// Arm 5: VmConfig.code is required. AsyncDataSource unset.
	parseRejectVmConfigCodeRequired = "wasm: config.vm_config.code is required"

	// Arm 6: VmConfig.code.remote is not yet supported. envoy-go-strict
	// requires local-only at 25.1; remote-fetch lands in a future Runtime/RTDS
	// family phase (see parent SPEC §2.1 + Q6 BRAINSTORM).
	parseRejectVmConfigCodeRemoteDeferred = "wasm: config.vm_config.code.remote is not yet supported (lands in a future Runtime/RTDS family phase)"

	// Arm 7: DataSource.watched_directory is not yet supported. Hot-reload
	// lands in a future Runtime/hot-reload phase.
	parseRejectDataSourceWatchedDirectoryDeferred = "wasm: config.vm_config.code.local.watched_directory is not yet supported (lands in a future Runtime/hot-reload phase)"

	// Arm 8: DataSource specifier oneof required. The local DataSource exists
	// but the specifier oneof (Filename/InlineBytes/InlineString/
	// EnvironmentVariable) is unset.
	parseRejectDataSourceSpecifierRequired = "wasm: config.vm_config.code.local: specifier oneof required"

	// Arm 9 / arm 10 (LIFTED at phase-25.3 Task 7): the 25.1/25.2 deferral
	// constants parseRejectPluginFailurePolicyFailReloadDeferred (failure_policy
	// = FAIL_RELOAD or reload_config set) + parseRejectPluginFailOpenDeferred
	// (fail_open) are RETIRED. failure_policy / reload_config / fail_open are
	// now PARSED + CONSUMED (see buildCompiledConfig). The only residual
	// reject on these surfaces is the NEW arm B mutual-exclusivity check
	// (parseRejectFailOpenAndFailurePolicyBothSet below).

	// Arm 11: VmConfig.runtime discriminator. envoy-go uses wazero exclusively
	// per AMEND-A1; upstream values "envoy.wasm.runtime.v8" / ".wasmtime" /
	// ".wamr" / ".null" PARSE-REJECT. Empty string passes through (default-
	// wazero per AMEND-A1). %q-formatted at the use site with the unsupported
	// runtime name for operator diagnostics.
	parseRejectVmConfigRuntimeDiscriminator = "wasm: config.vm_config.runtime %q is not supported (envoy-go uses wazero exclusively; envoy-go-strict departure)"

	// Arm 12 (RETIRED at phase-25.3 Task 7): the reserved RESERVED-at-25.1
	// duplicate-vm_id reject constant parseRejectVmConfigVmIdDuplicate is GONE.
	// 25.3 multi-plugin VM-sharing INVERTS the semantic: a duplicate
	// (vm_id, vm_configuration, code) now SHARES one *RootVM via the registry
	// refcount (wasm.DefaultRegistry.AcquireFor) rather than being rejected.
	// There is no production reject path for duplicate vm_id at 25.3.

	// Arm 13 (LIFTED at phase-25.3 Task 7): the 25.1/25.2 deferral constant
	// parseRejectVmConfigEnvironmentVariablesDeferred is RETIRED.
	// VmConfig.environment_variables is now PARSED + CONSUMED via
	// wasm.AssembleEnvVars (see buildCompiledConfig). The residual rejects on
	// this surface are the NEW arms A + C (env-vars key collision +
	// envoy-go-strict cap exceeded) below.

	// Arm 14: VmConfig.allow_precompiled. envoy-go-strict DEPARTURE: incompatible
	// with wazero's interpreter-default semantic (wazero supports precompiled
	// modules differently than upstream's V8/wasmtime); always rejected.
	parseRejectVmConfigAllowPrecompiledRejected = "wasm: config.vm_config.allow_precompiled is not supported (incompatible with wazero interpreter-default; envoy-go-strict departure)"

	// Arm 15: VmConfig.nack_on_code_cache_miss. envoy-go-strict DEPARTURE:
	// paired with code.remote (also rejected via arm 6); the knob only makes
	// sense in the remote-fetch flow. Always rejected at 25.1.
	parseRejectVmConfigNackOnCodeCacheMissRejected = "wasm: config.vm_config.nack_on_code_cache_miss is not supported (paired with code.remote; envoy-go-strict departure)"

	// Arm 16: module ABI-version rejected. envoy-go-strict-stricter per
	// AMEND-A6: only proxy_abi_version_0_2_1 supported; v0.1.0 + v0.2.0 +
	// missing-sentinel all PARSE-REJECT. The constant is the static wording;
	// wrapped at the use site via errors.Is(err, wasm.ErrUnsupportedAbiVersion)
	// (the underlying detected version is in the wrapped error from
	// CompileModule for caller debugging).
	parseRejectModuleAbiVersionRejected = "wasm: module: required proxy_abi_version_0_2_1 export not found (envoy-go-strict targets ABI v0.2.1 only; v0.1.0 + v0.2.0 + missing sentinel rejected)"

	// Arm 17: module compile failed. Format string with %w to wrap the inner
	// wazero compile error (carries wazero-level context: bad section header,
	// malformed import, etc.). Applied via fmt.Errorf at the use site.
	parseRejectModuleCompileFailed = "wasm: config.vm_config.code: compile: %w"

	// Arm 18 (LIFTED at phase-25.3 Task 7): per-route configuration is now
	// SUPPORTED. The 25.1/25.2 deferral constant parseRejectPerRouteDeferredTo253
	// (and its byte-equal alias parseRejectPerRouteUnsupported in wasm.go) are
	// RETIRED. validatePerRouteWasm now validates the per-route Wasm shape via
	// validateWasmConfigShape (a parse-only run of arms 1-23 + A/B/C that does NOT
	// mutate the process-wide name registry NOR acquire a registry RootVM
	// refcount); a valid per-route config ACCEPTS, an invalid one rejects with
	// the SAME byte-stable buildCompiledConfig wording (single source of truth).

	// -------------------------------------------------------------------------
	// 25.2 NEW PARSE-REJECT arms (per 25.2 SPEC §6.2 + D-25.2-P5 closure at
	// Task 14). 6 arms cover the 4 envoy-go-strict-only PluginConfig config
	// fields (zero-validators + body-buffer overlarge ceiling) + cross-
	// PluginConfig duplicate name registry.
	// -------------------------------------------------------------------------

	// Arm 19: envoy_go_strict_body_buffer_cap_bytes is 0. Triggered by the
	// envoy-go-strict-only field-validator at parse time. envoy-go-strict
	// departure: upstream Envoy has no equivalent in-filter body buffer cap;
	// upstream relies on HCM-level + listener-level memory ceilings. envoy-
	// go-strict adds defense-in-depth against PAUSE-loop guest patterns per
	// Q2 + 25.2 SPEC §2.3 + parent §6.2 25.2 forward-pointer.
	parseRejectEnvoyGoStrictBodyBufferCapBytesZero = "wasm: config.envoy_go_strict_body_buffer_cap_bytes must be > 0 (envoy-go-strict)"

	// Arm 20: envoy_go_strict_shared_data_value_cap_bytes is 0. Triggered by
	// the envoy-go-strict-only field-validator at parse time. envoy-go-strict
	// departure: upstream Envoy has no equivalent per-value cap on the shared-
	// data map; envoy-go-strict adds defense-in-depth against unbounded value
	// growth per Q6 + 25.2 SPEC §3.1 internal/wasm/shared_data.go.
	parseRejectEnvoyGoStrictSharedDataValueCapBytesZero = "wasm: config.envoy_go_strict_shared_data_value_cap_bytes must be > 0 (envoy-go-strict)"

	// Arm 21: envoy_go_strict_shared_data_max_entries is 0. Triggered by the
	// envoy-go-strict-only field-validator at parse time. envoy-go-strict
	// departure: upstream Envoy has no equivalent entry-count cap on the
	// shared-data map; envoy-go-strict adds defense-in-depth against
	// unbounded map-entry growth per Q6.
	parseRejectEnvoyGoStrictSharedDataMaxEntriesZero = "wasm: config.envoy_go_strict_shared_data_max_entries must be > 0 (envoy-go-strict)"

	// Arm 22: envoy_go_strict_dynamic_stats_max_entries is 0. Triggered by
	// the envoy-go-strict-only field-validator at parse time. envoy-go-strict
	// departure: upstream Envoy has no equivalent entry-count cap on the
	// dynamic-stats `wasmcustom.<custom_name>` namespace; envoy-go-strict
	// adds defense-in-depth against unbounded metric-name growth per Q9 +
	// AMEND-B2 + 25.2 SPEC §3.3 internal/stats/dynamic/.
	parseRejectEnvoyGoStrictDynamicStatsMaxEntriesZero = "wasm: config.envoy_go_strict_dynamic_stats_max_entries must be > 0 (envoy-go-strict)"

	// Arm 23: envoy_go_strict_body_buffer_cap_bytes > 1 GiB (1<<30 = 1073741824
	// bytes). %d-formatted at the use site with the offending value. Triggered
	// by the envoy-go-strict-only field-validator at parse time. envoy-go-
	// strict ceiling defense-in-depth: operators wanting >1 GiB body buffers
	// should re-architect via body-streaming rather than buffer-and-process
	// per Q2 + 25.2 SPEC §2.3.
	parseRejectEnvoyGoStrictBodyBufferCapBytesOverlarge = "wasm: config.envoy_go_strict_body_buffer_cap_bytes %d exceeds 1 GiB ceiling (envoy-go-strict)"

	// Arm 26: cross-PluginConfig duplicate PluginConfig.name. Two PluginConfig
	// entries within the same listener (or across listeners within the same
	// process) carry the same non-empty PluginConfig.name. envoy-go-strict
	// requires per-plugin scope uniqueness because the 5-counter envoy-go-
	// strict tri-group bundle (per AMEND-A2 Group-C) + the per-plugin *dynamic.
	// Registry (per AMEND-B2) both key off `wasm.<plugin_name>.*`; duplicate
	// names would collide at stat-name registration time AND obscure operator
	// metrics. %q-formatted at the use site with the offending name. Per
	// 25.2 SPEC §6.2 arm 26.
	//
	// Implementation: process-wide registered names map under a small mutex;
	// buildCompiledConfig consults the registry at parse time + adds on
	// success. Listener-drain time the compiledConfig's release path SHOULD
	// remove the entry; at 25.2 a release hook is NOT yet wired (the listener
	// lifecycle hook from the framework lands at 25.3 multi-plugin VM-sharing).
	// At 25.2 a process-wide append-only registry is acceptable — operator
	// reconfigs that rename a plugin will incur a cap-exhaustion-style growth
	// in the registry (one entry per unique name ever observed); for typical
	// operator workflows (steady-state plugin names) this is benign.
	//
	// Empty PluginConfig.name is NOT considered a collision trigger — the
	// empty name produces literal consecutive-dot wire names (per AMEND-A2
	// stat-name discipline) and the stats registry tolerates the form;
	// duplicate-empty-name collisions would surface at the underlying stats
	// registration layer as a duplicate-counter panic per ADR-0072 boot-time-
	// fail-fast. The arm-26 validator focuses on non-empty names where the
	// operator's intent is unambiguous.
	parseRejectCrossPluginConfigDuplicatePluginConfigName = "wasm: config.name %q is duplicated across PluginConfig entries (per-plugin stat-scope uniqueness; envoy-go-strict)"

	// -------------------------------------------------------------------------
	// 25.3 NEW PARSE-REJECT arms (per phase-25.3 Task 7 + D-25.3-P2 closure).
	// 3 arms cover the env_vars cross-field collision (arm A), the fail_open /
	// failure_policy mutual-exclusivity (arm B), and the env_vars envoy-go-
	// strict cap (arm C). Pinned byte-stable by TestParseRejectConstants_
	// ByteStable.
	// -------------------------------------------------------------------------

	// Arm A: VmConfig.environment_variables cross-field key collision. A key
	// present in BOTH host_env_keys AND key_values is REJECTED (NOT overridden)
	// per upstream cpp-host plugin.cc:30-42 parity. %q-formatted at the use
	// site with the colliding key. Surfaced when wasm.AssembleEnvVars returns
	// an error matching errors.Is(err, wasm.ErrEnvVarsKeyCollision).
	parseRejectEnvVarsKeyCollision = "wasm: config.vm_config.environment_variables: key %q is duplicated across host_env_keys and key_values (all keys must be unique)"

	// Arm B: PluginConfig.fail_open AND PluginConfig.failure_policy both set.
	// The deprecated fail_open knob + the modern failure_policy enum are
	// mutually exclusive — setting BOTH is ambiguous. envoy-go-strict rejects
	// rather than silently preferring one. Fires when fail_open == true AND
	// failure_policy != UNSPECIFIED.
	parseRejectFailOpenAndFailurePolicyBothSet = "wasm: only one of config.fail_open or config.failure_policy can be set"

	// Arm C: VmConfig.environment_variables exceeds the envoy-go-strict cap
	// (max 64 entries total OR any value > 4096 bytes) per AMEND-C4. Surfaced
	// when wasm.AssembleEnvVars returns an error matching
	// errors.Is(err, wasm.ErrEnvVarsCapExceeded). Task 8 adds the paired
	// env_vars_cap_exceeded counter on this reject path; Task 7 lands the
	// reject only.
	parseRejectEnvVarsCapExceeded = "wasm: config.vm_config.environment_variables exceeds the envoy-go-strict cap (max 64 entries, max 4096 bytes per value)"
)

// -----------------------------------------------------------------------------
// compiledConfig — per-listener immutable post-parse config per 25.1 SPEC §4.2.
// -----------------------------------------------------------------------------

// compiledConfig is the per-listener immutable post-parse Wasm filter config
// per 25.1 SPEC §4.2 + 25.2 SPEC §4.2. Populated by buildCompiledConfig at
// HCM-build time per ADR-0072 boot-time-fail-fast. Closure-captured by the
// per-stream *filter.cfg pointer allocated at the New factory body.
//
// 25.2 EXTENSIONS (per 25.2 SPEC §4.2 + ADR-0205 + ADR-0206 + ADR-0208):
//
//   - rootVM: long-lived per-compiledConfig VM (replaces 25.1 per-stream
//     *wasm.VM construction); constructed at New() via wasm.NewRootVM;
//     shared across all per-stream contexts; owns tick goroutine + shared-
//     data map + httpCall routing + foreign-function view + dynamic-stats
//     Registry.
//   - bodyBufferCapBytes / sharedDataValueCapBytes / sharedDataMaxEntries /
//     dynStatsMaxEntries: 4 envoy-go-strict-only cap fields per Qs 2/6/9 +
//     25.2 SPEC §7.4 parsed from PluginConfig.configuration via the
//     `envoy_go_strict` typed sub-Struct.
//   - dynStats: per-plugin *dynamic.Registry (per Q9 + AMEND-B2) rooted at
//     pluginScope `wasm.<pluginName>`; produces `wasmcustom.<custom_name>`
//     dynamic-stats names.
//   - foreignReg: per-plugin foreign-function registry view (per AMEND-A9);
//     points at wasm.DefaultForeignFunctionRegistry by default (EMPTY at
//     boot per envoy-go-strict departure record #4); testable seam.
//
// Field-additive growth: 25.1 → 25.2 adds 8 fields (rootVM + 4 caps +
// dynStats + foreignReg). 25.3 extends with per-route + multi-plugin VM-
// sharing state.
type compiledConfig struct {
	// module is the wazero-compiled wasm bytecode + ABI-version sentinel +
	// content-hash key. Returned by wasm.CompileModule; cross-VM-reusable
	// (the compiled module is immutable; per-stream VM instantiation happens
	// at Task 12 decode_headers.go).
	module *internalwasm.Module

	// compileCache is the content-addressed compile cache owned at
	// compiledConfig-instance scope per D-P-PLAN-5. Cache lifetime equals
	// compiledConfig lifetime; eviction happens when the listener drains +
	// the compiledConfig is released. At 25.1 the cache holds ONE module
	// (the single vm_config.code); the cache's primary purpose is to
	// forward-pin the API shape for 25.3 multi-plugin VM-sharing.
	compileCache *internalwasm.CompileCache

	// sandbox is the per-listener default-deny capability sandbox per
	// AMEND-A5 + ADR-0204. Built from PluginConfig.capability_restriction_config
	// via buildSandboxConfig; zero-value AllowedCapabilities ⇒ StrictDefaultDeny.
	sandbox internalwasm.SandboxConfig

	// pluginName is the PluginConfig.name discriminator per AMEND-A2 Group C.
	// Threads into stat-name registration (`wasm.<pluginName>.executions`)
	// + into ABI hostcall log-tagging (Task 11 abi_callbacks.go).
	pluginName string

	// rootContextID is the per-process u32 ID allocated at construction
	// time via the package-level rootContextIDCounter atomic. Per SPEC §4.2:
	// PluginConfig.root_id is a STRING; the runtime uses a u32 ID for guest-
	// export calls (proxy_on_context_create etc.). Per-process monotonic;
	// fresh ID per compiledConfig.
	rootContextID uint32

	// vmConfig is the marshaled VmConfig.configuration bytes passed to the
	// guest via proxy_on_vm_start at Task 12. May be empty (vm_config.
	// configuration unset).
	vmConfig []byte

	// pluginConfig is the marshaled PluginConfig.configuration bytes passed
	// to the guest via proxy_on_configure at Task 12. May be empty
	// (PluginConfig.configuration unset).
	pluginConfig []byte

	// stats is the SHARED stat-surface per AMEND-A2 (origin: 5 counters at
	// 25.1; EXTENDED to 14 at 25.2 per Q9 + AMEND-B3; EXTENDED to 18 counters
	// at 25.3 — the vm_reload_* Group-C triplet + env_vars_cap_exceeded per
	// ADR-0211). Populated by newFilterStats(reg, pluginName) inside
	// buildCompiledConfig when factoryCtx.Stats is non-nil (per ADR-0085
	// nil-tolerance); nil under test-double paths.
	stats *filterStats

	// -------------------------------------------------------------------------
	// 25.2 EXTENSIONS (per 25.2 SPEC §4.2 + ADR-0205 + ADR-0206 + ADR-0208).
	// -------------------------------------------------------------------------

	// rootVM is the long-lived per-compiledConfig *wasm.RootVM per Q3 +
	// ADR-0205. Replaces 25.1's per-stream *wasm.VM construction (the per-
	// stream lazy-VM-construction path in decode_headers.go is RETIRED at
	// Task 18 which closes the whole-repo build per D-P-PLAN-6). Constructed
	// at New() via wasm.NewRootVM; owns the tick goroutine + shared-data
	// map + httpCall routing + foreign-function view + dynamic-stats Registry;
	// shared (read-only after Configure) across all per-stream contexts. May
	// be nil in test-double paths that exercise compiledConfig pre-RootVM-
	// construction (e.g. PARSE-REJECT-arm tests that fail BEFORE the RootVM
	// allocation site below).
	rootVM *internalwasm.RootVM

	// bodyBufferCapBytes — default 16 MiB (16777216) per Q2 + 25.2 SPEC §7.4.
	// Parsed from PluginConfig.configuration → envoy_go_strict sub-Struct →
	// "body_buffer_cap_bytes" key. Consumed by the per-stream body
	// accumulator at Task 16 body.go (cap-exceeded → 413 + body_buffer_cap_
	// exceeded counter + envoy_go.failures counter per §2.25). PARSE-REJECT
	// arm 19 (==0) + arm 23 (>1<<30).
	bodyBufferCapBytes uint32

	// sharedDataValueCapBytes — default 1 MiB (1048576) per Q6 + 25.2 SPEC
	// §7.4. Parsed from PluginConfig.configuration → envoy_go_strict sub-
	// Struct → "shared_data_value_cap_bytes" key. Threaded to the *RootVM
	// via wasm.WithRootSharedDataCaps. PARSE-REJECT arm 20 (==0).
	sharedDataValueCapBytes uint32

	// sharedDataMaxEntries — default 1024 per Q6 + 25.2 SPEC §7.4. Parsed
	// from PluginConfig.configuration → envoy_go_strict sub-Struct →
	// "shared_data_max_entries" key. Threaded to the *RootVM via
	// wasm.WithRootSharedDataCaps. PARSE-REJECT arm 21 (==0).
	sharedDataMaxEntries uint32

	// dynStatsMaxEntries — default 1024 per Q9 + 25.2 SPEC §7.4. Parsed from
	// PluginConfig.configuration → envoy_go_strict sub-Struct →
	// "dynamic_stats_max_entries" key. Passed to dynamic.NewRegistry as the
	// maxEntries cap argument. PARSE-REJECT arm 22 (==0).
	dynStatsMaxEntries uint32

	// dynStats is the per-plugin *dynamic.Registry per Q9 + AMEND-B2 +
	// ADR-0208. Constructed at New() via dynamic.NewRegistry(factoryCtx.Stats,
	// "wasm."+pluginName, dynStatsMaxEntries). The Registry produces stat
	// names `wasmcustom.<custom_name>` under the per-plugin scope; admin
	// `/stats` enumerates as `wasm.<pluginName>.wasmcustom.<custom_name>`.
	// Threaded to the *RootVM via wasm.WithRootDynamicStats; consumed by
	// proxy_define_metric / proxy_increment_metric / proxy_record_metric /
	// proxy_get_metric host shims (Task 12). May be nil in test-double paths
	// where factoryCtx.Stats is nil (per ADR-0085 nil-tolerance — dynamic.
	// NewRegistry(nil, ...) returns nil).
	dynStats *dynamic.Registry

	// foreignReg is the per-plugin foreign-function registry view per AMEND-A9
	// + Task 7 internal/wasm/foreign.go. Points at the process-global
	// wasm.DefaultForeignFunctionRegistry by default (EMPTY at boot per
	// envoy-go-strict departure record #4 — operators register via
	// wasm.RegisterForeignFunction at boot). The per-plugin field exists as
	// a testable seam (test fixtures can construct a per-plugin Registry
	// without touching the process-global state). Threaded to the *RootVM
	// via wasm.WithRootForeignRegistry; consumed by proxy_call_foreign_function
	// host shim (Task 7).
	foreignReg *internalwasm.ForeignFunctionRegistry

	// -------------------------------------------------------------------------
	// 25.3 EXTENSIONS (per phase-25.3 Task 7 + AMEND-C2/C3/C4 + R-25.3-2/3).
	// -------------------------------------------------------------------------

	// failurePolicy is the parsed + mapped wasm.FailurePolicy per AMEND-C3 +
	// R-25.3-3. Maps PluginConfig.failure_policy (UNSPECIFIED→FailClosed
	// default; FAIL_RELOAD; FAIL_CLOSED; FAIL_OPEN) WITH the fail_open override
	// (fail_open==true ⇒ FailOpen). Consumed at Task 9 dispatch to gate the
	// RuntimeError disposition (FAIL_RELOAD → reload; FAIL_CLOSED → 503;
	// FAIL_OPEN → bypass). Task 7 stores it; Task 9 reads it.
	failurePolicy internalwasm.FailurePolicy //nolint:unused // stored at Task 7; consumed at Task 9 RuntimeError-gating dispatch

	// reloadBaseInterval is the operator-configured FAIL_RELOAD backoff base
	// interval parsed from PluginConfig.reload_config.backoff.base_interval per
	// AMEND-C3 + R-25.3-2. 0 ⇒ the wasm.newReloadBackoff default (1s); the
	// 100ms floor / 1s default are applied by the RootVM's reload machine at
	// construction via WithReloadConfig. Stored here for documentation /
	// future-dispatch observability.
	reloadBaseInterval time.Duration //nolint:unused // stored at Task 7; the RootVM consumes it via WithReloadConfig

	// vmKey is the composite registry key (Sha256(vm_id||vm_configuration||
	// code)) returned by wasm.DefaultRegistry.AcquireFor per AMEND-C2. The
	// compiledConfig retains it to Release (refcount--) the shared *RootVM at
	// teardown via Close(). Empty in test-double / non-registry paths.
	vmKey string

	// envVars is the assembled guest environment map (post-collision/cap
	// validation via wasm.AssembleEnvVars) per AMEND-C4. Fed to the *RootVM
	// via WithRootEnv at construction; retained here as a testable seam. Nil
	// when VmConfig.environment_variables is unset.
	envVars map[string]string //nolint:unused // fed to the RootVM via WithRootEnv; retained as a testable seam

	// factoryCtx retains the live envoyhttp.FactoryCtx supplied at the
	// listener buildCompiledConfig per phase-25.3 Task 9. The per-route
	// dispatch-time build (resolveEffective → parsePerRouteWasm) reuses it so
	// per-route *RootVMs get the SAME live ClusterManager / HTTPClient
	// dispatcher (else proxy_http_call silently fails per the Task-6 review
	// carried-concern #1). Stored by value (FactoryCtx is a struct of
	// pointers + scalars). Zero-value on test-double paths that bypass
	// buildCompiledConfig.
	factoryCtx envoyhttp.FactoryCtx

	// perRouteMu guards perRouteCache (lazy allocation + concurrent per-stream
	// resolveEffective lookups). Mirrors the phase-22.2 lua perRouteMu/
	// perRouteChunks memoization precedent.
	perRouteMu sync.Mutex

	// perRouteCache is the per-LISTENER memo of per-route override
	// *compiledConfigs, keyed by the stable per-route proto.Message pointer
	// (the framework's PerRouteConfig.ResolveAllTiers returns a stable pointer
	// per route). Built EXACTLY ONCE per unique per-route proto (arm-26 +
	// registry-refcount discipline — building the same override twice would
	// arm-26-false-reject + leak a refcount per the Task-6 review carried-
	// concern #2). Lives on the listener cfg (shared across streams), NOT the
	// per-stream filter. Lazily allocated on first resolveEffective miss.
	// Each entry holds its own *RootVM registry refcount; Close() Releases
	// every entry per carried-concern #3.
	perRouteCache map[proto.Message]*compiledConfig

	// rootCB is the per-RootVM rootABICallbacks multiplexer registered at
	// buildCompiledConfig time via rootVM.RegisterABICallbacks per Task 18
	// closure of D-P-PLAN-6. The multiplexer holds a streamCtxID → per-
	// stream *abiCallbacks map; per-stream filters register themselves at
	// decode_headers.go initStreamContext (after rootVM.NewStreamContext
	// returns the StreamContext) + deregister at encode_headers.go
	// OnDestroy (after streamCtx.Close). Per ADR-0205 the rootABICallbacks
	// is per-RootVM; each compiledConfig owns one. May be nil in test-
	// double paths that bypass buildCompiledConfig (e.g.
	// body_test.go's newBodyTestCompiledConfig); production callers always
	// populate via buildCompiledConfig.
	rootCB *rootABICallbacks
}

// rootContextIDCounter allocates fresh u32 root context IDs at compiledConfig
// construction time. Per SPEC §4.2: PluginConfig.root_id is a STRING; the
// runtime uses a u32 ID for guest-export calls. We allocate a monotonic
// counter per-process. atomic.Uint32 zero value is valid + safe under
// concurrent allocation per the atomic.Uint32 contract.
var rootContextIDCounter atomic.Uint32

// -----------------------------------------------------------------------------
// 25.2 envoy-go-strict-only cap defaults + ceilings (per Qs 2/6/9 + 25.2 SPEC
// §7.4 + ADR-0208).
// -----------------------------------------------------------------------------

const (
	// defaultBodyBufferCapBytes is the 16 MiB envoy-go-strict default body
	// buffer cap per Q2 + 25.2 SPEC §7.4. Operator-overridable via the
	// envoy_go_strict.body_buffer_cap_bytes config field (PARSE-REJECT arm
	// 19 + arm 23 enforce 0 < value ≤ bodyBufferCapBytesCeiling).
	defaultBodyBufferCapBytes uint32 = 16 * 1024 * 1024 // 16777216

	// defaultSharedDataValueCapBytes is the 1 MiB envoy-go-strict default
	// per-value cap on the shared-data map per Q6 + 25.2 SPEC §7.4.
	defaultSharedDataValueCapBytes uint32 = 1024 * 1024 // 1048576

	// defaultSharedDataMaxEntries is the 1024-entry envoy-go-strict default
	// cap on the shared-data map's entry count per Q6 + 25.2 SPEC §7.4.
	defaultSharedDataMaxEntries uint32 = 1024

	// defaultDynamicStatsMaxEntries is the 1024-entry envoy-go-strict
	// default cap on the per-plugin dynamic-stats namespace per Q9 +
	// AMEND-B2 + 25.2 SPEC §7.4.
	defaultDynamicStatsMaxEntries uint32 = 1024

	// bodyBufferCapBytesCeiling is the 1 GiB ceiling (1<<30 = 1073741824
	// bytes) for the envoy-go-strict body buffer cap field per arm 23 +
	// 25.2 SPEC §6.2. Operators wanting >1 GiB body buffers should re-
	// architect via body-streaming rather than buffer-and-process per the
	// rationale at 25.2 SPEC §6.2 arm 23.
	bodyBufferCapBytesCeiling uint32 = 1 << 30 // 1073741824
)

// -----------------------------------------------------------------------------
// envoy-go-strict sub-Struct key names (per 25.2 SPEC §7.4 + ADR-0208).
//
// Parsing mechanism (D-25.2-P5 first-action choice; per 25.2 SPEC §7.4
// anticipation): the 4 envoy-go-strict-only PluginConfig config fields are
// carried inside a typed structpb.Struct at PluginConfig.configuration.Any.
// The Any TypeURL must be "type.googleapis.com/google.protobuf.Struct" (the
// well-known Struct wire-URL). The Struct's top-level Fields map carries an
// "envoy_go_strict" key whose StructValue holds the 4 cap subfields:
//
//   envoy_go_strict:
//     body_buffer_cap_bytes:        <number> (uint32 default 16777216 = 16 MiB)
//     shared_data_value_cap_bytes:  <number> (uint32 default 1048576 = 1 MiB)
//     shared_data_max_entries:      <number> (uint32 default 1024)
//     dynamic_stats_max_entries:    <number> (uint32 default 1024)
//
// Rationale (D-25.2-P5 partial closure): mirrors the phase-22.2 lua
// `:filterState()` precedent for typed-Struct-in-Any configuration carriage
// + works with any operator wire format (YAML / JSON / xDS) that emits a
// Struct. NO custom envoy-go protobuf extension required at 25.2.
//
// PluginConfig.configuration is OPTIONAL: when unset, the 4 cap fields take
// their defaults (no PARSE-REJECT arm fires). When set but the Any TypeURL
// is NOT google.protobuf.Struct, the envoy-go-strict block is silently
// ignored (preserves operator flexibility to carry a non-Struct guest-side
// configuration payload — the guest reads cfg.pluginConfig raw bytes via
// proxy_on_configure at Task 12). When set AND the Any unwraps to a Struct
// but lacks the "envoy_go_strict" key, the 4 cap fields take their defaults.
// When set AND the "envoy_go_strict" subobject is present but a specific
// cap field is missing, that field takes its default; only EXPLICITLY-SET
// fields trigger PARSE-REJECT arms 19-23.
//
// Wire-key strings are byte-stable; renames require a parent-SPEC + ADR-0208
// lockstep edit per ADR-0044 atomic-edit discipline. Pinned via
// TestEnvoyGoStrictKeyConstants_ByteStable at compiled_config_test.go.
// -----------------------------------------------------------------------------

const (
	// envoyGoStrictKey is the top-level Struct field name carrying the
	// envoy-go-strict-only PluginConfig extensions.
	envoyGoStrictKey = "envoy_go_strict"

	// envoyGoStrictBodyBufferCapBytesKey carries the 16 MiB-default body
	// buffer cap (Q2).
	envoyGoStrictBodyBufferCapBytesKey = "body_buffer_cap_bytes"

	// envoyGoStrictSharedDataValueCapBytesKey carries the 1 MiB-default
	// per-value shared-data cap (Q6).
	envoyGoStrictSharedDataValueCapBytesKey = "shared_data_value_cap_bytes"

	// envoyGoStrictSharedDataMaxEntriesKey carries the 1024-default entry-
	// count shared-data cap (Q6).
	envoyGoStrictSharedDataMaxEntriesKey = "shared_data_max_entries"

	// envoyGoStrictDynamicStatsMaxEntriesKey carries the 1024-default
	// dynamic-stats namespace entry-count cap (Q9 + AMEND-B2).
	envoyGoStrictDynamicStatsMaxEntriesKey = "dynamic_stats_max_entries"
)

// -----------------------------------------------------------------------------
// Process-wide PluginConfig.name registry (per arm 26 + 25.2 SPEC §6.2).
//
// Cross-PluginConfig duplicate-name detection at parse time per arm 26.
// The 5-counter envoy-go-strict tri-group bundle (AMEND-A2 Group-C) + the
// per-plugin *dynamic.Registry both key off `wasm.<plugin_name>.*`; duplicate
// names would collide at stat-name registration time AND obscure operator
// metrics. The registry is process-wide + append-only at 25.2; the listener-
// release hook that removes entries lands at 25.3 multi-plugin VM-sharing.
//
// Empty PluginConfig.name skips the duplicate-check (zero-value names produce
// literal consecutive-dot wire names per AMEND-A2 — they collide elsewhere
// at the underlying stats registration layer via ADR-0072 boot-time-fail-
// fast). The arm-26 validator focuses on non-empty names where operator
// intent is unambiguous.
// -----------------------------------------------------------------------------

var (
	pluginNameRegistryMu sync.Mutex
	pluginNameRegistry   = make(map[string]struct{})
)

// registerPluginConfigName claims the given non-empty pluginConfigName
// process-wide. Returns nil on first claim; returns the arm-26 PARSE-REJECT
// error on duplicate. Empty pluginConfigName always returns nil (no
// claim, no check) per the empty-name carve-out documented above.
//
// Per ADR-0072 boot-time-fail-fast: any PARSE-REJECT surfaces at HCM-build
// time + bubbles out the New factory to the operator.
func registerPluginConfigName(pluginConfigName string) error {
	if pluginConfigName == "" {
		return nil
	}
	pluginNameRegistryMu.Lock()
	defer pluginNameRegistryMu.Unlock()
	if _, dup := pluginNameRegistry[pluginConfigName]; dup {
		return fmt.Errorf(parseRejectCrossPluginConfigDuplicatePluginConfigName, pluginConfigName)
	}
	pluginNameRegistry[pluginConfigName] = struct{}{}
	return nil
}

// resetPluginConfigNameRegistry clears the process-wide PluginConfig.name
// registry. Test-only helper; NOT exported. Production code never calls
// this — the registry is append-only at 25.2 per the comment block above.
// Test code (compiled_config_test.go) MUST call this between subtests that
// exercise duplicate-name paths to keep cross-test state clean.
func resetPluginConfigNameRegistry() {
	pluginNameRegistryMu.Lock()
	defer pluginNameRegistryMu.Unlock()
	pluginNameRegistry = make(map[string]struct{})
}

// -----------------------------------------------------------------------------
// buildCompiledConfig — full proto-parser body per 25.1 SPEC §6 Task 9 +
// parent §6.2 18-arm PARSE-REJECT roster.
// -----------------------------------------------------------------------------

// buildCompiledConfig parses the Wasm proto envelope (wrapped in *anypb.Any) +
// constructs the per-listener compiledConfig per 25.1 SPEC §4.2 + 25.2 SPEC
// §4.2 + parent §6.2 18-arm + 25.2 §6.2 6-NEW-arm PARSE-REJECT roster (24
// arms cumulative). Per ADR-0072 boot-time-fail-fast: any validation failure
// returns the byte-stable error string verbatim (or %w-wrapped for arms 2 +
// 17; %d-formatted for arm 23; %q-formatted for arms 11 + 26).
//
// Arm ordering (parent §6.2 + 25.2 §6.2 EXTENSION):
//
//  1. arm 1  — typedConfig nil
//  2. arm 2  — typedConfig.UnmarshalTo(*Wasm) fails (wrapped)
//  3. arm 3  — Wasm.config (PluginConfig) nil
//  4. arm 9  — PluginConfig.failure_policy = FAIL_RELOAD or reload_config set
//  5. arm 10 — PluginConfig.fail_open = true
//  6. arm 4  — PluginConfig.vm_config nil
//  7. arm 11 — VmConfig.runtime not in {"", "envoy.wasm.runtime.wazero"}
//  8. arm 13 — VmConfig.environment_variables non-nil
//  9. arm 14 — VmConfig.allow_precompiled true
//  10. arm 15 — VmConfig.nack_on_code_cache_miss true
//  11. arm 12 — VmConfig.vm_id duplicate (RESERVED at 25.1; no production path)
//  12. arm 5  — VmConfig.code nil
//  13. arm 6  — AsyncDataSource.Remote set
//  14. arm 7  — DataSource.watched_directory set
//  15. arm 8  — DataSource specifier oneof unset
//  16. arms 19-23 (25.2 NEW) — envoy-go-strict-only PluginConfig cap field
//     validators. Parse PluginConfig.configuration → envoy_go_strict sub-
//     Struct + apply defaults. Arms ordered by field appearance in the
//     parse-envoy-go-strict pipeline (body-buffer-zero → shared-data-value-
//     cap-zero → shared-data-max-entries-zero → dynamic-stats-max-entries-
//     zero → body-buffer-overlarge).
//  17. arm 26 (25.2 NEW) — cross-PluginConfig duplicate PluginConfig.name.
//     Fires AFTER cap validators succeed so the byte-stable wording for
//     arms 19-23 wins on configs that trigger BOTH a cap-failure AND a
//     duplicate name (arms 19-23 are the more-actionable error; arm 26
//     would mask a config typo if it fired first). Ordered BEFORE the
//     resolveDataSource pipeline so the duplicate-name registry isn't
//     polluted by a name that later fails arm-16/17 ABI compilation.
//  18. resolveDataSource (Task 10): produces wasm src bytes OR arms 6-15
//     DataSource-resolution sub-arms.
//  19. arm 16 — wasm.CompileModule fails with errors.Is(err, ErrUnsupportedAbiVersion)
//  20. arm 17 — wasm.CompileModule fails with any other error (wrapped)
//  21. wasm.NewRootVM construction (25.2 NEW): replaces 25.1's per-stream
//     wasm.NewVM. Failure surfaces as a non-byte-stable wrapped error (the
//     RootVM construction path is not in the PARSE-REJECT byte-stable roster;
//     a failure here indicates a runtime issue + an unwrapped error from
//     wasm.NewRootVM is returned).
//
// arm 18 (per-route, LIFTED at phase-25.3 Task 7) is enforced via the separate
// HCM RegisterPerRouteValidator path in wasm.go::validatePerRouteWasm per
// ADR-0110 single-chokepoint, which delegates to validateWasmConfigShape (a
// validate-only run of this body); the full build below is the listener +
// per-route-dispatch (Task 9) path.
func buildCompiledConfig(ctx context.Context, typedConfig *anypb.Any, factoryCtx envoyhttp.FactoryCtx) (*compiledConfig, error) {
	return buildCompiledConfigImpl(ctx, typedConfig, factoryCtx, false)
}

// validateWasmConfigShape runs every PARSE-REJECT arm (1-23 + the NEW
// failure_policy/fail_open/env_vars arms A/B/C) against a typed_config WITHOUT
// permanently mutating any process-wide state: it does NOT call
// registerPluginConfigName (so a valid config can be re-validated / used on a
// second route without arm-26-FALSE-rejecting) and it does NOT acquire a
// registry *RootVM refcount (no VM instantiation, no goroutine, no leak).
//
// This is the per-route HCM-build shape-check chokepoint per ADR-0072 fail-
// fast (lifted arm 18). A valid per-route Wasm config returns nil; an invalid
// one returns the SAME byte-stable wording as the listener build path (single
// source of truth = buildCompiledConfigImpl). The ACTUAL per-route
// compiledConfig + VM build happens at Task 9 dispatch resolution.
//
// arm 26 (cross-PluginConfig duplicate name) is DELIBERATELY NOT enforced
// here: it is a cross-config global-uniqueness check that requires CLAIMING
// the name, which a pure validation pass must not do. The real build (listener
// or per-route at Task 9) is the arm-26 enforcement point.
func validateWasmConfigShape(typedConfig *anypb.Any) error {
	_, err := buildCompiledConfigImpl(context.Background(), typedConfig, envoyhttp.FactoryCtx{}, true)
	return err
}

func buildCompiledConfigImpl(ctx context.Context, typedConfig *anypb.Any, factoryCtx envoyhttp.FactoryCtx, validateOnly bool) (*compiledConfig, error) {
	// Arm 1: typed_config required.
	if typedConfig == nil {
		return nil, errors.New(parseRejectTypedConfigRequired)
	}

	// Arm 2: typed_config.UnmarshalTo(*Wasm) failure.
	cfg := &wasmv3.Wasm{}
	if err := typedConfig.UnmarshalTo(cfg); err != nil {
		return nil, fmt.Errorf(parseRejectTypedConfigUnmarshal, err)
	}

	// Arm 3: config (PluginConfig) is required.
	pc := cfg.GetConfig()
	if pc == nil {
		return nil, errors.New(parseRejectConfigRequired)
	}

	// Arms 9/10 LIFTED (phase-25.3 Task 7): failure_policy / reload_config /
	// fail_open are now PARSED + CONSUMED. Map them to the wasm-package
	// FailurePolicy + the reload base interval, applying the NEW arm B
	// mutual-exclusivity reject (fail_open AND failure_policy both set).
	failurePolicy, reloadBaseInterval, err := parseFailurePolicy(pc)
	if err != nil {
		return nil, err
	}

	// Arm 4: vm_config is required.
	vm := pc.GetVmConfig()
	if vm == nil {
		return nil, errors.New(parseRejectVmConfigRequired)
	}

	// Arm 11: VmConfig.runtime discriminator. Empty string passes through
	// (default-wazero per AMEND-A1); only "envoy.wasm.runtime.wazero" is
	// explicitly accepted. Any other value PARSE-REJECTs with the
	// unsupported-runtime name formatted via %q for operator diagnostics.
	runtime := vm.GetRuntime()
	if runtime != "" && runtime != "envoy.wasm.runtime.wazero" {
		return nil, fmt.Errorf(parseRejectVmConfigRuntimeDiscriminator, runtime)
	}

	// Arm 13 LIFTED (phase-25.3 Task 7): VmConfig.environment_variables is now
	// PARSED + CONSUMED via wasm.AssembleEnvVars. The NEW arm A (cross-field
	// key collision) + arm C (envoy-go-strict cap exceeded) are the residual
	// rejects on this surface. On success the assembled map is fed to the
	// RootVM via WithRootEnv at construction.
	envVars, err := parseEnvVars(vm.GetEnvironmentVariables())
	if err != nil {
		return nil, err
	}

	// Arm 14: VmConfig.allow_precompiled rejected (envoy-go-strict).
	if vm.GetAllowPrecompiled() {
		return nil, errors.New(parseRejectVmConfigAllowPrecompiledRejected)
	}

	// Arm 15: VmConfig.nack_on_code_cache_miss rejected (envoy-go-strict;
	// paired with code.remote which is also rejected via arm 6).
	if vm.GetNackOnCodeCacheMiss() {
		return nil, errors.New(parseRejectVmConfigNackOnCodeCacheMissRejected)
	}

	// Arm 12 RETIRED (phase-25.3 Task 7): a duplicate (vm_id, vm_configuration,
	// code) no longer rejects — it SHARES one *RootVM via the registry
	// refcount at the AcquireFor construction site below (AMEND-C2). No
	// production reject path for duplicate vm_id at 25.3.

	// Arm 5: VmConfig.code is required.
	code := vm.GetCode()
	if code == nil {
		return nil, errors.New(parseRejectVmConfigCodeRequired)
	}

	// Arm 6: AsyncDataSource.Remote not yet supported. envoy-go-strict
	// requires local-only at 25.1 per parent §2.1.
	if code.GetRemote() != nil {
		return nil, errors.New(parseRejectVmConfigCodeRemoteDeferred)
	}

	local := code.GetLocal()

	// Arm 7: DataSource.watched_directory not yet supported. Inspected
	// BEFORE arm 8 because watched_directory can be set independently of the
	// specifier oneof; we want the more-specific arm to fire first when both
	// triggers are present.
	if local != nil && local.GetWatchedDirectory() != nil {
		return nil, errors.New(parseRejectDataSourceWatchedDirectoryDeferred)
	}

	// Arm 8: DataSource specifier oneof required. Fires when:
	//   - local is nil (no AsyncDataSource.Local oneof set), OR
	//   - local is non-nil but specifier oneof is unset.
	// Either path means we have no actual source bytes to load.
	if local == nil || local.GetSpecifier() == nil {
		return nil, errors.New(parseRejectDataSourceSpecifierRequired)
	}

	// -------------------------------------------------------------------------
	// 25.2 EXTENSION — envoy-go-strict-only cap fields + cross-PluginConfig
	// duplicate-name validators (arms 19-23 + arm 26 per 25.2 SPEC §6.2 +
	// D-25.2-P5).
	//
	// Parsing order:
	//
	//  (a) Parse PluginConfig.configuration → envoy_go_strict sub-Struct +
	//      apply defaults (arms 19-23 fire if EXPLICITLY-SET fields are out
	//      of range; defaults skip the arms).
	//
	//  (b) Cross-PluginConfig duplicate-name registry consult (arm 26).
	//      Ordered AFTER cap validators so the more-actionable cap-error
	//      surfaces first on configs that trigger BOTH a cap-failure AND a
	//      duplicate name.
	// -------------------------------------------------------------------------

	bodyBufferCap, sharedDataValueCap, sharedDataMaxEntries, dynStatsMaxEntries, err := parseEnvoyGoStrictFields(pc.GetConfiguration())
	if err != nil {
		// PARSE-REJECT arms 19-23 bubble up byte-stable per D-25.2-P5.
		return nil, err
	}

	// Arm 26: cross-PluginConfig duplicate PluginConfig.name registry consult.
	// Empty pluginName skips the check per the empty-name carve-out at
	// registerPluginConfigName + the comment block at the registry decl.
	//
	// SKIPPED in validate-only mode (per-route shape-check): claiming the name
	// would FALSE-reject a legitimate second use of the same override, and a
	// pure validation pass must not permanently mutate the process-wide
	// registry. The real build (listener or per-route at Task 9) is the arm-26
	// enforcement point.
	if !validateOnly {
		if err := registerPluginConfigName(pc.GetName()); err != nil {
			return nil, err
		}
	}

	// Resolve DataSource bytes — DELEGATED to datasource.go at Task 10.
	// At Task 9 this is a forward stub returning a sentinel error; Task 10
	// implements the real 4-arm body (InlineBytes / InlineString / Filename /
	// EnvironmentVariable) + the wrapped PARSE-REJECT arms for resolution
	// failure paths (file-not-found, env-var-not-set, etc.).
	src, err := resolveDataSource(local)
	if err != nil {
		// PARSE-REJECT bubbles up. Task 10's resolveDataSource arms each
		// already carry the byte-stable parseRejectDataSource* wording; at
		// Task 9 the stub-error sentinel surfaces verbatim. Unregister the
		// PluginConfig.name from the cross-plugin registry so a subsequent
		// retry with the same name (after the operator fixes the data-source
		// resolution failure) doesn't phantom-trigger arm 26 per the same
		// rollback discipline applied at the NewRootVM failure path below.
		unregisterPluginConfigName(pc.GetName())
		return nil, err
	}

	// Compile the module via Task 5's CompileCache, gating ABI version per
	// AMEND-A6. The cache scope is compiledConfig-instance (allocated fresh
	// per call) per D-P-PLAN-5; cache.Close should be called when the
	// compiledConfig is no longer needed (typically at listener-drain time).
	cache := internalwasm.NewCompileCache(ctx)
	mod, err := internalwasm.CompileModule(ctx, src, cache)
	if err != nil {
		// Arm 16: unsupported ABI version (envoy-go-strict-stricter per
		// AMEND-A6). errors.Is composes through the %w-wrapped sentinel
		// returned by CompileModule when GetAbiVersion produced anything
		// other than AbiVersion_0_2_1.
		if errors.Is(err, internalwasm.ErrUnsupportedAbiVersion) {
			// Release the cache before returning — the listener never gets
			// a compiledConfig back, so nothing else will Close it.
			_ = cache.Close()
			unregisterPluginConfigName(pc.GetName())
			return nil, errors.New(parseRejectModuleAbiVersionRejected)
		}
		// Arm 17: any other compile failure (wazero parse error, bad section
		// header, malformed import, etc.). Wrap with the byte-stable arm-17
		// prefix; the inner error carries wazero-level context.
		_ = cache.Close()
		unregisterPluginConfigName(pc.GetName())
		return nil, fmt.Errorf(parseRejectModuleCompileFailed, err)
	}

	// Validate-only short-circuit (per-route shape-check via lifted arm 18):
	// every PARSE-REJECT arm 1-23 + A/B/C has fired by now + the module compiled
	// (arms 16/17). We do NOT instantiate a *RootVM (no AcquireFor refcount, no
	// goroutine), and we DID NOT register the plugin name (arm 26 skipped
	// above). Close the freshly-built cache + return nil — the config shape is
	// valid. The real per-route build happens at Task 9 dispatch.
	if validateOnly {
		_ = cache.Close()
		return nil, nil
	}

	// Build SandboxConfig from PluginConfig.capability_restriction_config.
	// Per AMEND-A5: nil restriction OR empty map = StrictDefaultDeny.
	sandbox := buildSandboxConfig(pc.GetCapabilityRestrictionConfig())

	// Allocate root context ID (per SPEC §4.2: monotonic u32 counter).
	rootCtxID := rootContextIDCounter.Add(1)

	// Build filterStats from the registry per AMEND-A2 + ADR-0085 nil-
	// tolerance. The Stats field may be nil in test-double paths;
	// newFilterStats handles nil-registry by returning nil.
	stats := newFilterStats(factoryCtx.Stats, pc.GetName())

	// -------------------------------------------------------------------------
	// 25.2 EXTENSION — per-plugin *dynamic.Registry + per-plugin
	// *ForeignFunctionRegistry view + long-lived *RootVM construction (per
	// 25.2 SPEC §4.2 + ADR-0205 + ADR-0206 + ADR-0208 + Tasks 1 / 7 / 11).
	// -------------------------------------------------------------------------

	// dynStats — per-plugin Registry per Q9 + AMEND-B2. Per ADR-0085 nil-
	// tolerance: factoryCtx.Stats may be nil under test-double paths;
	// dynamic.NewRegistry(nil, ...) returns nil (consumers nil-check).
	// pluginScopePrefix is `wasm.<pluginName>` per AMEND-B2 (the per-plugin
	// scope under which the `wasmcustom.<custom_name>` namespace lives —
	// admin /stats enumerates as `wasm.<pluginName>.wasmcustom.<custom_name>`
	// while the wire-shape from the proxy-wasm perspective is byte-faithful
	// to `wasmcustom.<custom_name>`).
	dynStats := dynamic.NewRegistry(factoryCtx.Stats, "wasm."+pc.GetName(), dynStatsMaxEntries)

	// foreignReg — per-plugin view of the process-global ForeignFunctionRegistry
	// per AMEND-A9 + Task 7. EMPTY default registry at boot per envoy-go-
	// strict departure record #4; operators register via
	// wasm.RegisterForeignFunction at boot. The per-plugin field exists as
	// a testable seam.
	foreignReg := internalwasm.DefaultForeignFunctionRegistry

	// rootVM — long-lived per-compiledConfig VM per Q3 + ADR-0205. Replaces
	// 25.1's per-stream wasm.NewVM. Constructed ONCE at New() per
	// compiledConfig; the tick goroutine starts immediately (idle until
	// proxy_set_tick_period_milliseconds fires); the shared-data map is
	// initialized empty; the httpCall routing state is empty; the foreign-
	// function registry view + dynamic-stats Registry are wired via the
	// per-option setters below. The wazero.CompilationCache from the per-
	// compiledConfig CompileCache (Task 5) is wired in via
	// WithRootCompilationCache so the RootVM's internal re-compile of
	// module.Source against the runtime hits the shared codegen cache as a
	// sub-ms lookup.
	rootOpts := []internalwasm.RootVMOption{
		internalwasm.WithRootSandboxConfig(sandbox),
		internalwasm.WithRootSharedDataCaps(sharedDataValueCap, sharedDataMaxEntries),
		internalwasm.WithRootForeignRegistry(foreignReg),
	}
	if dynStats != nil {
		rootOpts = append(rootOpts, internalwasm.WithRootDynamicStats(dynStats))
	}
	// Wire the per-plugin RootStatsRecorder per 25.2 IMPL Task 20 follow-up
	// (Concern 2 — counter-glue wiring gap). *filterStats satisfies the
	// internalwasm.RootStatsRecorder interface via thin per-counter wrapper
	// methods at internal/filter/http/wasm/stats.go; the wasm package holds
	// the recorder reference indirectly so it can bump the 9 NEW envoy-go-
	// strict counters per §7.1 + AMEND-B3 from its hostcall bodies without
	// importing the filter package (avoids cycle). Per ADR-0085 nil-
	// tolerance: when stats is nil (test-double paths), WithRootStats
	// converts internally to noopStatsRecorder; the hostcall bodies' .Inc()
	// calls become zero-cost no-ops.
	if stats != nil {
		rootOpts = append(rootOpts, internalwasm.WithRootStats(stats))
	}
	// Wire the production HTTPDispatcher adapter per 25.2 IMPL Task 20 fix-up
	// #2 (closes the wiring gap that blocked fixture-0036 arms (l) httpCall-
	// success + (m) httpCall-unknown-cluster). The adapter wraps the per-
	// listener *cluster.Manager + the shared *httpclient.Client framework
	// primitive (the SAME phase-20 ADR-0177 surface consumed by phase-22.2
	// lua's :httpCall() at the 2nd co-consumer). Per ADR-0085 nil-tolerance:
	// when EITHER FactoryCtx pointer is nil (test-double paths that bypass
	// full FactoryCtx wiring), the dispatcher is NOT wired + proxy_http_call
	// returns WasmResultInternalFailure per the documented no-dispatcher
	// contract. Production callers always supply both pointers per the
	// phase-18.2 + phase-20 first-use anchors.
	if factoryCtx.ClusterManager != nil && factoryCtx.HTTPClient != nil {
		rootOpts = append(rootOpts, internalwasm.WithRootHTTPDispatcher(
			newWasmHTTPDispatcher(factoryCtx.ClusterManager, factoryCtx.HTTPClient),
		))
	}
	if wc := cache.WazeroCompilationCache(); wc != nil {
		rootOpts = append(rootOpts, internalwasm.WithRootCompilationCache(wc))
	}
	// Clock seam injection per 25.2 IMPL Task 16 tick_clock.go. The package-
	// level resolveClock() returns nil under production callers (the
	// framework-layer default at wasm.NewRootVM fires + rv.clk = clock.
	// RealClock{}); test callers inject a clock.FakeClock via withTestClock
	// for deterministic fixture-0036 tick-fires-counter coverage.
	if clk := resolveClock(); clk != nil {
		rootOpts = append(rootOpts, internalwasm.WithRootClock(clk))
	}

	// 25.3 EXTENSION wiring (AMEND-C2/C3/C4):
	//
	//   - WithRootEnv:        feeds the assembled VmConfig.environment_variables
	//     map (post collision/cap validation) to the guest's WASI environ shims.
	//   - WithReloadConfig:   sets the FAIL_RELOAD backoff base interval; the
	//     100ms floor / 1s default are applied inside newReloadBackoff.
	//   - WithReloadStats:    wires the vm_reload triplet counters (Task 8
	//     Group C) into the reload state machine via the reloadStatsHook seam
	//     (Task 3). *filterStats satisfies reloadStatsHook structurally via
	//     VmReloadSuccessInc / VmReloadRuntimeFailureInc / VmReloadBackoffInc.
	//     Guarded by stats != nil per ADR-0085 nil-tolerance.
	//   - WithSharedDataStore: injects the vm_id-scoped registry-owned shared-
	//     data store so plugins under the SAME vm_id share one namespace.
	if len(envVars) > 0 {
		rootOpts = append(rootOpts, internalwasm.WithRootEnv(envVars))
	}
	rootOpts = append(rootOpts, internalwasm.WithReloadConfig(reloadBaseInterval))
	if stats != nil {
		rootOpts = append(rootOpts, internalwasm.WithReloadStats(stats))
	}
	rootOpts = append(rootOpts, internalwasm.WithSharedDataStore(
		internalwasm.DefaultRegistry.AcquireSharedData(vm.GetVmId()),
	))

	// Multi-plugin VM-sharing per AMEND-C2: acquire (refcount++) or construct
	// the SHARED *RootVM keyed by (vm_id, vm_configuration, code). The factory
	// closure runs ONLY on a registry miss; on a hit it is NOT called + the
	// existing *RootVM (already Configured, with its rootABICallbacks
	// multiplexer + tick goroutine) is reused.
	//
	// vm-key byte-source decision (D-25.3 first-action): `vmConfig` component
	// is vm.GetConfiguration().GetValue() (the serialized VmConfig.configuration
	// bytes — the SAME bytes stored on cfg.vmConfig + fed to RootVM.Configure);
	// `code` component is the resolved module source bytes `src` (the SAME
	// bytes fed to CompileModule + re-compiled inside NewRootVM). vm_id is the
	// raw VmConfig.vm_id string.
	var (
		rootCB     *rootABICallbacks
		factoryRan bool
	)
	factory := func() (*internalwasm.RootVM, error) {
		factoryRan = true
		rv, err := internalwasm.NewRootVM(ctx, mod, rootCtxID, rootOpts...)
		if err != nil {
			return nil, fmt.Errorf("wasm: NewRootVM: %w", err)
		}
		// Per Task 18 closure of D-P-PLAN-6: construct the per-RootVM
		// rootABICallbacks multiplexer + register it so guest hostcalls route
		// through the multiplexer's streamCtxID lookup. One multiplexer per
		// shared *RootVM (streamCtxIDs are per-RootVM, so configs sharing the
		// VM share this multiplexer too).
		rootCB = newRootABICallbacks(rv)
		rv.RegisterABICallbacks(rootCB)
		// Drive the per-RootVM Configure lifecycle (_initialize/_start +
		// proxy_on_context_create(rootCtxID, 0) + proxy_on_vm_start +
		// proxy_on_configure). A Configure failure tears down the RootVM so
		// the registry never caches a half-built VM.
		if cerr := rv.Configure(ctx, vm.GetConfiguration().GetValue(), pc.GetConfiguration().GetValue()); cerr != nil {
			_ = rv.Close()
			return nil, fmt.Errorf("wasm: RootVM.Configure: %w", cerr)
		}
		return rv, nil
	}

	rootVM, vmKey, err := internalwasm.DefaultRegistry.AcquireFor(
		vm.GetVmId(), vm.GetConfiguration().GetValue(), src, factory)
	if err != nil {
		// Construction failed (NewRootVM or Configure). Release the freshly-
		// built cache + unregister the PluginConfig.name so a retry with the
		// same name doesn't phantom-trigger arm 26.
		_ = cache.Close()
		unregisterPluginConfigName(pc.GetName())
		return nil, err
	}

	if !factoryRan {
		// Registry HIT: the shared *RootVM already exists (built by an earlier
		// compiledConfig). The freshly-compiled cache/module for THIS call are
		// redundant — the shared VM owns its own compile cache. Close this
		// orphan cache to avoid leaking the wazero compilation cache, and
		// retrieve the SHARED multiplexer the original config registered so
		// per-stream registrations route through the one table the VM knows.
		_ = cache.Close()
		if rb, ok := rootVM.RegisteredABICallbacks().(*rootABICallbacks); ok {
			rootCB = rb
		}
	}

	return &compiledConfig{
		module:                  mod,
		compileCache:            cache,
		sandbox:                 sandbox,
		pluginName:              pc.GetName(),
		rootContextID:           rootCtxID,
		vmConfig:                vm.GetConfiguration().GetValue(),
		pluginConfig:            pc.GetConfiguration().GetValue(),
		stats:                   stats,
		rootVM:                  rootVM,
		bodyBufferCapBytes:      bodyBufferCap,
		sharedDataValueCapBytes: sharedDataValueCap,
		sharedDataMaxEntries:    sharedDataMaxEntries,
		dynStatsMaxEntries:      dynStatsMaxEntries,
		dynStats:                dynStats,
		foreignReg:              foreignReg,
		rootCB:                  rootCB,
		failurePolicy:           failurePolicy,
		reloadBaseInterval:      reloadBaseInterval,
		vmKey:                   vmKey,
		envVars:                 envVars,
		factoryCtx:              factoryCtx,
	}, nil
}

// Close releases the compiledConfig's hold on the SHARED *RootVM (refcount--
// via wasm.DefaultRegistry.Release) + closes the per-compiledConfig compile
// cache. The registry closes the underlying *RootVM only when its refcount
// reaches 0 (the last sharing compiledConfig releases). Per AMEND-C2 the
// compiledConfig MUST NOT call rootVM.Close() directly — that would double-
// close a *RootVM still in use by another sharing compiledConfig. Release is
// the single teardown chokepoint.
//
// Lifecycle note: at phase-25.3 there is NO per-config production teardown
// hook in the New factory (shared listener configs live for process/listener
// lifetime); Close exists so the lifecycle is correct + testable. Per-route
// compiledConfigs (Task 9) that swap per-request DO have a teardown seam that
// will call Close. Idempotency: a compiledConfig with an empty vmKey (test-
// double / non-registry path) skips the Release; the compileCache.Close is
// idempotent at the cache layer.
func (cc *compiledConfig) Close() error {
	if cc == nil {
		return nil
	}

	// Tear down every memo'd per-route compiledConfig FIRST (per the Task-9
	// carried-concern #3). Each per-route entry holds its OWN registry refcount
	// (its own *RootVM via the AcquireFor in buildCompiledConfig); Close()
	// Releases that refcount + closes that entry's compile cache. Done under
	// perRouteMu + the cache is niled to guard against a double-close (a second
	// listener-cfg Close is then a no-op over an empty map). The per-route
	// entries have an empty perRouteCache themselves (per-route configs never
	// resolve further per-route overrides), so the recursion bottoms out in one
	// level.
	cc.perRouteMu.Lock()
	prCache := cc.perRouteCache
	cc.perRouteCache = nil
	cc.perRouteMu.Unlock()
	for _, prCfg := range prCache {
		if prCfg != nil && prCfg != cc {
			_ = prCfg.Close()
		}
	}

	var relErr error
	if cc.vmKey != "" {
		relErr = internalwasm.DefaultRegistry.Release(cc.vmKey)
	}
	if cc.compileCache != nil {
		_ = cc.compileCache.Close()
	}
	return relErr
}

// resolveEffective returns the effective *compiledConfig for a stream given
// the framework's 3-tier-resolved per-route proto (or nil) per phase-25.3 Task
// 9 + AMEND-C1 (wholesale Wasm override; the ENTIRE compiledConfig, hence its
// *RootVM, swaps per-route). The receiver cc is the LISTENER config.
//
//   - routeCfgProto == nil → the listener cfg itself (cc), nil error. The
//     common path (no per-route override) is behavior-identical.
//   - else → the memoized per-route *compiledConfig built EXACTLY ONCE per
//     unique per-route proto pointer (carried-concern #2: building twice would
//     arm-26-false-reject the duplicate PluginConfig.name + leak a registry
//     refcount). The memo lives on the LISTENER cfg (cc.perRouteCache), shared
//     across streams. The per-route build reuses cc.factoryCtx (carried-
//     concern #1: the live ClusterManager / HTTPClient dispatcher).
//
// A dispatch-time build error is UNEXPECTED (the per-route proto was already
// shape-validated at HCM-build via validatePerRouteWasm → validateWasmConfigShape);
// it is surfaced to the caller rather than swallowed.
func (cc *compiledConfig) resolveEffective(routeCfgProto proto.Message) (*compiledConfig, error) {
	if routeCfgProto == nil {
		return cc, nil
	}

	// TODO(25.3-followup): a cold per-route miss builds (registry AcquireFor +
	// wazero compile) while holding perRouteMu, head-of-line-blocking other
	// streams hitting per-route routes on this listener during the first build.
	// Per-route overrides are rare; a sentry-based double-checked build is the
	// mitigation if this becomes hot.
	cc.perRouteMu.Lock()
	defer cc.perRouteMu.Unlock()

	if cc.perRouteCache != nil {
		if prCfg, ok := cc.perRouteCache[routeCfgProto]; ok {
			return resolvePerRoute(prCfg, cc), nil
		}
	}

	// Memo miss → build EXACTLY ONCE, reusing the listener's live FactoryCtx so
	// the per-route *RootVM gets the live dispatcher (carried-concern #1).
	prCfg, err := parsePerRouteWasm(routeCfgProto, cc.factoryCtx)
	if err != nil {
		return nil, err
	}

	if cc.perRouteCache == nil {
		cc.perRouteCache = make(map[proto.Message]*compiledConfig)
	}
	cc.perRouteCache[routeCfgProto] = prCfg
	return resolvePerRoute(prCfg, cc), nil
}

// -----------------------------------------------------------------------------
// parseFailurePolicy — PluginConfig.failure_policy / reload_config / fail_open
// → wasm.FailurePolicy + reload base interval per phase-25.3 Task 7
// (AMEND-C3 + R-25.3-2/3). Lands the NEW arm B (mutual-exclusivity reject).
// -----------------------------------------------------------------------------

// parseFailurePolicy maps the proto failure surfaces to the wasm-package
// FailurePolicy + the operator-configured reload backoff base interval.
//
// Mapping (per AMEND-C3 + R-25.3-3):
//
//   - fail_open == true AND failure_policy != UNSPECIFIED → arm B PARSE-REJECT
//     (parseRejectFailOpenAndFailurePolicyBothSet): the deprecated fail_open
//     knob + the modern failure_policy enum are mutually exclusive.
//   - fail_open == true (failure_policy UNSPECIFIED) → FailurePolicyFailOpen.
//   - else map failure_policy: UNSPECIFIED → FailurePolicyFailClosed (the
//     proto-documented default); FAIL_RELOAD / FAIL_CLOSED / FAIL_OPEN map
//     1:1.
//
// reload base interval: reload_config.backoff.base_interval (a durationpb)
// → time.Duration. 0 / nil ⇒ 0 (the RootVM's newReloadBackoff applies the
// 100ms floor / 1s default). Only meaningful under FAIL_RELOAD but parsed
// unconditionally (a base_interval set without FAIL_RELOAD is harmless — the
// reload machine is never consulted unless the policy is FAIL_RELOAD).
func parseFailurePolicy(pc *wasmcommonv3.PluginConfig) (internalwasm.FailurePolicy, time.Duration, error) {
	//nolint:staticcheck // SA1019: fail_open is upstream-deprecated; envoy-go
	// still PARSES it (mapping it to FAIL_OPEN) for upstream config-compat per
	// AMEND-C3. Intentional access.
	failOpen := pc.GetFailOpen()
	protoPolicy := pc.GetFailurePolicy()

	// Arm B: mutual-exclusivity. Both the deprecated knob + the modern enum set.
	if failOpen && protoPolicy != wasmcommonv3.FailurePolicy_UNSPECIFIED {
		return 0, 0, errors.New(parseRejectFailOpenAndFailurePolicyBothSet)
	}

	var policy internalwasm.FailurePolicy
	switch {
	case failOpen:
		policy = internalwasm.FailurePolicyFailOpen
	default:
		switch protoPolicy {
		case wasmcommonv3.FailurePolicy_FAIL_RELOAD:
			policy = internalwasm.FailurePolicyFailReload
		case wasmcommonv3.FailurePolicy_FAIL_OPEN:
			policy = internalwasm.FailurePolicyFailOpen
		case wasmcommonv3.FailurePolicy_FAIL_CLOSED, wasmcommonv3.FailurePolicy_UNSPECIFIED:
			// UNSPECIFIED defaults to FAIL_CLOSED per the proto documentation.
			policy = internalwasm.FailurePolicyFailClosed
		default:
			policy = internalwasm.FailurePolicyFailClosed
		}
	}

	var base time.Duration
	if rc := pc.GetReloadConfig(); rc != nil {
		if bi := rc.GetBackoff().GetBaseInterval(); bi != nil {
			base = bi.AsDuration()
		}
	}

	return policy, base, nil
}

// -----------------------------------------------------------------------------
// parseEnvVars — VmConfig.environment_variables → assembled guest env per
// phase-25.3 Task 7 (AMEND-C4). Lands the NEW arms A (key collision) + C
// (envoy-go-strict cap exceeded).
// -----------------------------------------------------------------------------

// parseEnvVars extracts host_env_keys + key_values from the proto
// EnvironmentVariables and assembles the guest-visible env map via
// wasm.AssembleEnvVars. Returns:
//
//   - nil, nil                              when ev is nil (no env block).
//   - nil, arm-A error  (key collision)     when a key appears in BOTH
//     host_env_keys AND key_values (errors.Is ErrEnvVarsKeyCollision). The
//     colliding key is %q-formatted into the arm-A wording.
//   - nil, arm-C error  (cap exceeded)       when the assembled env exceeds
//     the envoy-go-strict cap (errors.Is ErrEnvVarsCapExceeded). Task 8 adds
//     the env_vars_cap_exceeded counter on this path.
//   - assembled-map, nil                     on success.
func parseEnvVars(ev *wasmcommonv3.EnvironmentVariables) (map[string]string, error) {
	if ev == nil {
		return nil, nil
	}
	hostEnvKeys := ev.GetHostEnvKeys()
	keyValues := ev.GetKeyValues()

	assembled, err := internalwasm.AssembleEnvVars(hostEnvKeys, keyValues)
	if err != nil {
		switch {
		case errors.Is(err, internalwasm.ErrEnvVarsKeyCollision):
			// Arm A: extract the colliding key for the %q-formatted wording.
			// The colliding key is the first host_env_key that also appears in
			// key_values (matching AssembleEnvVars's detection order).
			collidingKey := firstEnvVarsCollision(hostEnvKeys, keyValues)
			return nil, fmt.Errorf(parseRejectEnvVarsKeyCollision, collidingKey)
		case errors.Is(err, internalwasm.ErrEnvVarsCapExceeded):
			// Arm C: envoy-go-strict cap (Task 8 adds the counter here).
			return nil, errors.New(parseRejectEnvVarsCapExceeded)
		default:
			// Defensive: AssembleEnvVars only returns the two sentinels above;
			// any other error surfaces verbatim (non-byte-stable) rather than
			// being swallowed.
			return nil, err
		}
	}
	return assembled, nil
}

// firstEnvVarsCollision returns the first key in hostEnvKeys that also appears
// in keyValues (matching wasm.AssembleEnvVars's collision-detection order so
// the arm-A %q wording names the same key the assembler tripped on). Returns
// "" when there is no collision (caller only invokes this on the collision
// error path).
func firstEnvVarsCollision(hostEnvKeys []string, keyValues map[string]string) string {
	for _, k := range hostEnvKeys {
		if _, ok := keyValues[k]; ok {
			return k
		}
	}
	return ""
}

// unregisterPluginConfigName releases the given pluginConfigName from the
// process-wide registry. Called on the construction-failure rollback path
// (arm 16/17 OR the wasm.NewRootVM error path) so a subsequent retry with
// the SAME name doesn't phantom-trigger arm 26.
//
// Empty pluginConfigName is a no-op (the empty-name carve-out at
// registerPluginConfigName means the entry was never claimed).
func unregisterPluginConfigName(pluginConfigName string) {
	if pluginConfigName == "" {
		return
	}
	pluginNameRegistryMu.Lock()
	defer pluginNameRegistryMu.Unlock()
	delete(pluginNameRegistry, pluginConfigName)
}

// -----------------------------------------------------------------------------
// parseEnvoyGoStrictFields — PluginConfig.configuration → 4 envoy-go-strict-
// only cap fields per 25.2 SPEC §7.4 + ADR-0208 + Qs 2/6/9. Lands PARSE-
// REJECT arms 19-23 byte-stable per D-25.2-P5.
// -----------------------------------------------------------------------------

// parseEnvoyGoStrictFields extracts the 4 envoy-go-strict-only cap fields
// from PluginConfig.configuration and returns them with defaults applied for
// missing keys. Returns the byte-stable PARSE-REJECT arm-19/20/21/22/23
// error when a field is EXPLICITLY-SET out of range.
//
// Parsing mechanism (per the constants block above):
//
//  1. If PluginConfig.configuration is nil (unset), all 4 fields take their
//     defaults; no PARSE-REJECT fires.
//  2. If PluginConfig.configuration is set but its Any TypeURL is NOT
//     "type.googleapis.com/google.protobuf.Struct", the envoy-go-strict
//     block is silently ignored (operator may carry a guest-only payload
//     in PluginConfig.configuration); all 4 fields take their defaults.
//  3. If the Any unwraps to a Struct but lacks the "envoy_go_strict" top-
//     level key, all 4 fields take their defaults.
//  4. If the "envoy_go_strict" sub-Struct is present, each of the 4 cap
//     subkeys is read INDIVIDUALLY: missing keys take defaults; PRESENT
//     keys validate against arms 19-23 (zero → arm 19/20/21/22; >1<<30
//     → arm 23 for body_buffer_cap_bytes).
//  5. Non-number value-types under recognized keys are PARSE-REJECTed via
//     the same zero-arm path (the typed-Struct → uint32 conversion below
//     produces 0 for non-number wrappers, which then trips the
//     "must be > 0" arm). This is a deliberate carve-out: operators who
//     pass a string instead of a number get the "must be > 0" wording.
//
// Returns (bodyBufferCap, sharedDataValueCap, sharedDataMaxEntries,
// dynStatsMaxEntries, err).
func parseEnvoyGoStrictFields(any *anypb.Any) (uint32, uint32, uint32, uint32, error) {
	// Defaults applied unconditionally first; explicit subkey values
	// overwrite below.
	bodyBufferCap := defaultBodyBufferCapBytes
	sharedDataValueCap := defaultSharedDataValueCapBytes
	sharedDataMaxEntries := defaultSharedDataMaxEntries
	dynStatsMaxEntries := defaultDynamicStatsMaxEntries

	// Step 1: PluginConfig.configuration unset ⇒ defaults.
	if any == nil {
		return bodyBufferCap, sharedDataValueCap, sharedDataMaxEntries, dynStatsMaxEntries, nil
	}

	// Step 2: PluginConfig.configuration set; unwrap to Struct OR ignore.
	// MessageIs short-circuits if the TypeURL doesn't match google.protobuf.
	// Struct. The non-Struct path returns defaults (operator's guest-only
	// payload).
	if !any.MessageIs(&structpb.Struct{}) {
		return bodyBufferCap, sharedDataValueCap, sharedDataMaxEntries, dynStatsMaxEntries, nil
	}
	configStruct := &structpb.Struct{}
	if err := any.UnmarshalTo(configStruct); err != nil {
		// Defensive: a TypeURL match should always unmarshal, but the
		// payload may be truncated/corrupted. Silently ignore the envoy-
		// go-strict block on unmarshal failure (the same envelope still
		// flows downstream to the guest via pluginConfig bytes).
		return bodyBufferCap, sharedDataValueCap, sharedDataMaxEntries, dynStatsMaxEntries, nil
	}

	// Step 3: top-level "envoy_go_strict" key. Missing ⇒ defaults.
	rawStrict, ok := configStruct.GetFields()[envoyGoStrictKey]
	if !ok || rawStrict == nil {
		return bodyBufferCap, sharedDataValueCap, sharedDataMaxEntries, dynStatsMaxEntries, nil
	}
	strictStruct := rawStrict.GetStructValue()
	if strictStruct == nil {
		// "envoy_go_strict" key present but not a StructValue (operator
		// passed e.g. a number or a string). Silently take defaults rather
		// than reject — the operator intent is ambiguous + the arms fire
		// on the per-subkey validation below if the value were a number-
		// containing Struct.
		return bodyBufferCap, sharedDataValueCap, sharedDataMaxEntries, dynStatsMaxEntries, nil
	}
	strictFields := strictStruct.GetFields()

	// Step 4: per-subkey extraction with PARSE-REJECT arms 19-23.

	// Arm 19: body_buffer_cap_bytes (zero); Arm 23: body_buffer_cap_bytes
	// (overlarge).
	if v, present := strictFields[envoyGoStrictBodyBufferCapBytesKey]; present {
		bodyBufferCap = structValueToUint32(v)
		if bodyBufferCap == 0 {
			return 0, 0, 0, 0, errors.New(parseRejectEnvoyGoStrictBodyBufferCapBytesZero)
		}
		if bodyBufferCap > bodyBufferCapBytesCeiling {
			return 0, 0, 0, 0, fmt.Errorf(parseRejectEnvoyGoStrictBodyBufferCapBytesOverlarge, bodyBufferCap)
		}
	}

	// Arm 20: shared_data_value_cap_bytes (zero).
	if v, present := strictFields[envoyGoStrictSharedDataValueCapBytesKey]; present {
		sharedDataValueCap = structValueToUint32(v)
		if sharedDataValueCap == 0 {
			return 0, 0, 0, 0, errors.New(parseRejectEnvoyGoStrictSharedDataValueCapBytesZero)
		}
	}

	// Arm 21: shared_data_max_entries (zero).
	if v, present := strictFields[envoyGoStrictSharedDataMaxEntriesKey]; present {
		sharedDataMaxEntries = structValueToUint32(v)
		if sharedDataMaxEntries == 0 {
			return 0, 0, 0, 0, errors.New(parseRejectEnvoyGoStrictSharedDataMaxEntriesZero)
		}
	}

	// Arm 22: dynamic_stats_max_entries (zero).
	if v, present := strictFields[envoyGoStrictDynamicStatsMaxEntriesKey]; present {
		dynStatsMaxEntries = structValueToUint32(v)
		if dynStatsMaxEntries == 0 {
			return 0, 0, 0, 0, errors.New(parseRejectEnvoyGoStrictDynamicStatsMaxEntriesZero)
		}
	}

	return bodyBufferCap, sharedDataValueCap, sharedDataMaxEntries, dynStatsMaxEntries, nil
}

// structValueToUint32 projects a *structpb.Value to uint32. Returns 0 if the
// value is nil OR not a NumberValue OR the NumberValue is negative OR exceeds
// the uint32 ceiling. The 0-on-malformed convention pairs with the per-arm
// "must be > 0" validators above — a non-number wrapper trips the same arm
// as a literal 0, surfacing a uniform operator wording.
//
// Per ADR-0080 byte-stable wording: the per-arm constants do NOT mention
// "wrong type" because the structpb.Value spec is sufficiently weakly-typed
// (NumberValue is a float64) that the operator intent is "I tried to pass
// a number"; "must be > 0" is the most actionable wording across both the
// literal-zero path and the wrong-type path.
//
// Overflow case (NumberValue > 2^32 - 1): converts to 0 (truncation would
// fire wrong PARSE-REJECT arms; cap-the-ceiling is the safer disposition).
func structValueToUint32(v *structpb.Value) uint32 {
	if v == nil {
		return 0
	}
	n := v.GetNumberValue()
	if n <= 0 {
		return 0
	}
	if n > float64(^uint32(0)) {
		return 0
	}
	//nolint:gosec // float64 → uint32 with bounds-checked guards immediately above
	return uint32(n)
}

// -----------------------------------------------------------------------------
// buildSandboxConfig — proto CapabilityRestrictionConfig → wasm.SandboxConfig.
// -----------------------------------------------------------------------------

// buildSandboxConfig converts the proto-level CapabilityRestrictionConfig to
// a wasm.SandboxConfig. Per AMEND-A5 + ADR-0204: nil restriction OR empty
// AllowedCapabilities map ⇒ StrictDefaultDeny (INVERTS upstream's bare-empty-
// map-allow-all). Per AMEND-A1 + parent §4.3.5: SanitizationConfig is empty
// at upstream + ignored at envoy-go; we always store an empty
// wasm.SanitizationConfig{} (presence in the map IS the allow-signal).
func buildSandboxConfig(restrict *wasmcommonv3.CapabilityRestrictionConfig) internalwasm.SandboxConfig {
	if restrict == nil {
		// Zero-value SandboxConfig (nil AllowedCapabilities map) per
		// AMEND-A5 StrictDefaultDeny.
		return internalwasm.SandboxConfig{}
	}

	rawMap := restrict.GetAllowedCapabilities()
	if len(rawMap) == 0 {
		// Empty (or nil) map ⇒ StrictDefaultDeny per AMEND-A5 INVERSION.
		// Return the zero-value sandbox for shape uniformity with the
		// nil-restriction path above.
		return internalwasm.SandboxConfig{}
	}

	allowed := make(map[string]internalwasm.SanitizationConfig, len(rawMap))
	for name, sanitConfig := range rawMap {
		// AMEND-A1: SanitizationConfig accept-empty discipline; the
		// upstream proto is empty + marked "currently unimplemented and
		// ignored". Whether the operator passed a nil value or an empty
		// SanitizationConfig{} struct, we store the empty form.
		_ = sanitConfig // parse-and-discard per AMEND-A1
		allowed[name] = internalwasm.SanitizationConfig{}
	}

	return internalwasm.SandboxConfig{AllowedCapabilities: allowed}
}

// -----------------------------------------------------------------------------
// resolveDataSource — landed at Task 10 in datasource.go.
// -----------------------------------------------------------------------------

// resolveDataSource is implemented in datasource.go (same package). It
// dispatches the AsyncDataSource.Local oneof across the 4 supported arms
// (Filename / InlineBytes / InlineString / EnvironmentVariable) per parent
// §5.4 + lands the per-arm content-empty PARSE-REJECT sub-failure wordings
// (parseRejectFilenameEmpty / parseRejectInlineBytesEmpty / etc.) as a
// D-P5 sub-arm extension at Task 10. See datasource.go for the full body
// + per-arm helpers.
