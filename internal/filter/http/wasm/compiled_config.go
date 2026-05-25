package wasm

// compiled_config.go — `compiledConfig` per-listener immutable post-parse
// config + `buildCompiledConfig` 18-arm PARSE-REJECT roster per parent SPEC
// §6.2 + D-P5 closure (byte-stable wording finalization at Task 9). Lands
// the per-listener config struct that Task 12's decode_headers.go /
// encode_headers.go hot-path consumes via the per-stream *filter.cfg
// pointer closure-captured at the (Task 12) New factory body.
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
//   - Arm 9  (plugin-failure-policy-fail-reload-deferred)  — IMPL HERE
//   - Arm 10 (plugin-fail-open-deferred)                   — IMPL HERE
//   - Arm 11 (vm-config-runtime-discriminator)             — IMPL HERE (%q-formatted)
//   - Arm 12 (vm-config-vm-id-duplicate)                   — RESERVED at 25.1
//                                                            (single-plugin-per-
//                                                            listener model;
//                                                            arm activates at
//                                                            25.3 multi-plugin
//                                                            VM-sharing registry).
//                                                            Constant byte-stable
//                                                            pinned by
//                                                            TestParseRejectConstants_
//                                                            ByteStable; no
//                                                            production trigger
//                                                            path at 25.1.
//   - Arm 13 (vm-config-environment-variables-deferred)    — IMPL HERE
//   - Arm 14 (vm-config-allow-precompiled-rejected)        — IMPL HERE
//   - Arm 15 (vm-config-nack-on-code-cache-miss-rejected)  — IMPL HERE
//   - Arm 16 (module-abi-version-rejected)                 — IMPL HERE (via
//                                                            errors.Is on
//                                                            wasm.ErrUnsupportedAbiVersion;
//                                                            requires real wasm
//                                                            bytecode at Task
//                                                            10 / Task 15 fixture-
//                                                            0034 integration)
//   - Arm 17 (module-compile-failed)                       — IMPL HERE (%w-wrapped
//                                                            wazero compile error;
//                                                            requires real wasm
//                                                            bytecode at Task 10)
//   - Arm 18 (per-route-deferred-to-25-3)                  — IMPL VIA
//                                                            validatePerRouteWasm
//                                                            in wasm.go per ADR-0110
//                                                            single-chokepoint
//                                                            (constant
//                                                            parseRejectPerRouteDeferredTo253
//                                                            here is the canonical
//                                                            source-of-truth alias
//                                                            of parseRejectPerRouteUnsupported
//                                                            consumed by wasm.go;
//                                                            TestParseRejectArm18_
//                                                            AliasedFromWasmGo
//                                                            pins the byte-identity).
//
// # D-P5 closure (per 25.1 SPEC §12-D-P5 + parent §6.2)
//
// At this Task 9, the byte-stable wording for ALL 18 arms is pinned via the
// package-private `parseReject*` constants below. `TestParseRejectConstants_
// ByteStable` (compiled_config_test.go) is a table-driven test that asserts
// each constant byte-exact against the SPEC wording. Per ADR-0044 atomic-edit
// discipline: any wording change touches both this file + the byte-exact
// roster in parent SPEC §6.2 in a single commit.
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
//   - AMEND-A1 (SanitizationConfig accept-empty discipline; ignored at 25.1)
//   - AMEND-A2 (5-counter stat surface; HCM-stats_prefix DROPPED — pluginName
//     drives Group-C envoy-go-strict per-plugin keys)
//   - AMEND-A5 (default-deny capability sandbox — INVERTS upstream empty-map-
//     allow-all)
//   - AMEND-A6 (envoy-go-strict-stricter ABI rejection — v0.1.0 + v0.2.0 +
//     missing-sentinel surfaces as arm 16)
//   - parent SPEC §6.1 (wording discipline) + §6.2 (18-arm roster) + §12-D-P5
//     (D-P5 closure anchor at this Task)
//   - 25.1 SPEC §4.2 (compiledConfig + filterStats wiring)

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	wasmcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
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

	// Arm 9: PluginConfig.failure_policy = FAIL_RELOAD (or reload_config set).
	// FAIL_RELOAD + the paired reload_config lands at 25.3 alongside multi-
	// plugin VM-sharing per Q3 BRAINSTORM phasing.
	parseRejectPluginFailurePolicyFailReloadDeferred = "wasm: config.failure_policy = FAIL_RELOAD (or reload_config set) is not yet supported (lands in phase 25.3)"

	// Arm 10: PluginConfig.fail_open. The fail_open knob is upstream-deprecated
	// in favor of failure_policy = FAIL_OPEN; at 25.1 BOTH are deferred to 25.3.
	parseRejectPluginFailOpenDeferred = "wasm: config.fail_open is not yet supported (deprecated upstream; lands in phase 25.3 via failure_policy = FAIL_OPEN)"

	// Arm 11: VmConfig.runtime discriminator. envoy-go uses wazero exclusively
	// per AMEND-A1; upstream values "envoy.wasm.runtime.v8" / ".wasmtime" /
	// ".wamr" / ".null" PARSE-REJECT. Empty string passes through (default-
	// wazero per AMEND-A1). %q-formatted at the use site with the unsupported
	// runtime name for operator diagnostics.
	parseRejectVmConfigRuntimeDiscriminator = "wasm: config.vm_config.runtime %q is not supported (envoy-go uses wazero exclusively; envoy-go-strict departure)"

	// Arm 12: VmConfig.vm_id duplicate across PluginConfig entries. RESERVED
	// at 25.1: the single-plugin-per-listener model has no duplicate trigger
	// path; 25.3 multi-plugin VM-sharing wires the process-wide vm_id registry
	// that activates this arm. Constant byte-stable pinned for forward-compat
	// per ADR-0044 atomic-edit discipline.
	//nolint:unused // reserved for 25.3 multi-plugin VM-sharing registry; arm 12 has no production trigger at 25.1
	parseRejectVmConfigVmIdDuplicate = "wasm: config.vm_config.vm_id %q is duplicated across PluginConfig entries (multi-plugin VM-sharing lands in phase 25.3)"

	// Arm 13: VmConfig.environment_variables is not yet supported. The full
	// host-env / key-value injection surface lands at 25.3.
	parseRejectVmConfigEnvironmentVariablesDeferred = "wasm: config.vm_config.environment_variables is not yet supported (lands in phase 25.3)"

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

	// Arm 18: per-route configuration deferred to 25.3. The ACTUAL trigger
	// path is the validatePerRouteWasm one-liner in wasm.go (registered via
	// RegisterPerRouteValidator at boot per ADR-0110 single-chokepoint). This
	// constant is the canonical source-of-truth for the wording; the
	// parseRejectPerRouteUnsupported constant in wasm.go is byte-equal (pinned
	// by TestParseRejectArm18_AliasedFromWasmGo).
	parseRejectPerRouteDeferredTo253 = "wasm: per-route configuration is not yet supported (lands in phase 25.3)"
)

// -----------------------------------------------------------------------------
// compiledConfig — per-listener immutable post-parse config per 25.1 SPEC §4.2.
// -----------------------------------------------------------------------------

// compiledConfig is the per-listener immutable post-parse Wasm filter config
// per 25.1 SPEC §4.2. Populated by buildCompiledConfig at HCM-build time per
// ADR-0072 boot-time-fail-fast. Closure-captured by the per-stream *filter.cfg
// pointer allocated at the (Task 12) New factory body.
//
// Field-final at 25.1; 25.2 extends with body/trailer/property-tree state;
// 25.3 extends with per-route + multi-plugin VM-sharing state. 25.1
// reserves no fields for future deltas; field-additive growth at 25.2 + 25.3.
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

	// stats is the SHARED 5-counter stat-surface per AMEND-A2. Populated by
	// newFilterStats(reg, pluginName) inside buildCompiledConfig when
	// factoryCtx.Stats is non-nil (per ADR-0085 nil-tolerance); nil under
	// test-double paths.
	stats *filterStats
}

// rootContextIDCounter allocates fresh u32 root context IDs at compiledConfig
// construction time. Per SPEC §4.2: PluginConfig.root_id is a STRING; the
// runtime uses a u32 ID for guest-export calls. We allocate a monotonic
// counter per-process. atomic.Uint32 zero value is valid + safe under
// concurrent allocation per the atomic.Uint32 contract.
var rootContextIDCounter atomic.Uint32

// -----------------------------------------------------------------------------
// buildCompiledConfig — full proto-parser body per 25.1 SPEC §6 Task 9 +
// parent §6.2 18-arm PARSE-REJECT roster.
// -----------------------------------------------------------------------------

// buildCompiledConfig parses the Wasm proto envelope (wrapped in *anypb.Any) +
// constructs the per-listener compiledConfig per 25.1 SPEC §4.2 + parent §6.2
// 18-arm PARSE-REJECT roster. Per ADR-0072 boot-time-fail-fast: any validation
// failure returns the byte-stable error string verbatim (or %w-wrapped for
// arms 2 + 17).
//
// Arm ordering (parent §6.2):
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
//  16. resolveDataSource (Task 10): produces wasm src bytes OR arms 6-15
//     DataSource-resolution sub-arms (lands at Task 10)
//  17. arm 16 — wasm.CompileModule fails with errors.Is(err, ErrUnsupportedAbiVersion)
//  18. arm 17 — wasm.CompileModule fails with any other error (wrapped)
//
// arm 18 (per-route) is enforced via the separate HCM RegisterPerRouteValidator
// path in wasm.go::validatePerRouteWasm per ADR-0110 single-chokepoint; not a
// buildCompiledConfig concern.
//
// At Task 9 `resolveDataSource` is a FORWARD STUB returning a sentinel error
// so the buildCompiledConfig pipeline compiles + the pre-resolveDataSource
// arms can be tested in isolation. Task 10 (`datasource.go`) replaces the
// stub with the full 4-arm body.
func buildCompiledConfig(ctx context.Context, typedConfig *anypb.Any, factoryCtx envoyhttp.FactoryCtx) (*compiledConfig, error) {
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

	// Arm 9: plugin-failure-policy = FAIL_RELOAD (or reload_config set).
	// Both triggers funnel to the same deferred wording per parent §6.2 arm 9.
	if pc.GetFailurePolicy() == wasmcommonv3.FailurePolicy_FAIL_RELOAD || pc.GetReloadConfig() != nil {
		return nil, errors.New(parseRejectPluginFailurePolicyFailReloadDeferred)
	}

	// Arm 10: plugin-fail-open deferred. SA1019 deliberate: this arm EXISTS
	// to PARSE-REJECT the deprecated proto field per parent §6.2 arm 10.
	if pc.GetFailOpen() { //nolint:staticcheck // SA1019: arm 10 EXISTS to PARSE-REJECT this deprecated proto field; intentional access.
		return nil, errors.New(parseRejectPluginFailOpenDeferred)
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

	// Arm 13: VmConfig.environment_variables deferred to 25.3.
	if vm.GetEnvironmentVariables() != nil {
		return nil, errors.New(parseRejectVmConfigEnvironmentVariablesDeferred)
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

	// Arm 12: VmConfig.vm_id duplicate. RESERVED at 25.1: the single-plugin-
	// per-listener model has no duplicate trigger path. 25.3 multi-plugin
	// VM-sharing wires the process-wide vm_id registry that activates this
	// arm. No production check at 25.1; the constant byte-stability is
	// pinned by TestParseRejectConstants_ByteStable.
	// (Intentional no-op; documented for forward-compat.)

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

	// Resolve DataSource bytes — DELEGATED to datasource.go at Task 10.
	// At Task 9 this is a forward stub returning a sentinel error; Task 10
	// implements the real 4-arm body (InlineBytes / InlineString / Filename /
	// EnvironmentVariable) + the wrapped PARSE-REJECT arms for resolution
	// failure paths (file-not-found, env-var-not-set, etc.).
	src, err := resolveDataSource(local)
	if err != nil {
		// PARSE-REJECT bubbles up. Task 10's resolveDataSource arms each
		// already carry the byte-stable parseRejectDataSource* wording; at
		// Task 9 the stub-error sentinel surfaces verbatim.
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
			return nil, errors.New(parseRejectModuleAbiVersionRejected)
		}
		// Arm 17: any other compile failure (wazero parse error, bad section
		// header, malformed import, etc.). Wrap with the byte-stable arm-17
		// prefix; the inner error carries wazero-level context.
		_ = cache.Close()
		return nil, fmt.Errorf(parseRejectModuleCompileFailed, err)
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

	return &compiledConfig{
		module:        mod,
		compileCache:  cache,
		sandbox:       sandbox,
		pluginName:    pc.GetName(),
		rootContextID: rootCtxID,
		vmConfig:      vm.GetConfiguration().GetValue(),
		pluginConfig:  pc.GetConfiguration().GetValue(),
		stats:         stats,
	}, nil
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
