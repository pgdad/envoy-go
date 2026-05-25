package wasm

// compiled_config_test.go — Task 9 RIGID-TDD test surface per 25.1 PLAN
// Task 9 + parent SPEC §6.2 18-arm PARSE-REJECT roster + D-P5 closure
// (byte-stable wording finalization at this task).
//
// # Test surface coverage
//
//   - TestParseRejectConstants_ByteStable — table-driven; 18 rows; asserts
//     each `parseReject*` package-private constant matches the SPEC wording
//     byte-exact. D-P5 closure enforcement at commit time.
//
//   - TestBuildCompiledConfig_PARSE_REJECT — one subtest per arm. For each
//     arm, construct a *wasmv3.Wasm proto + an *anypb.Any wrapper that
//     triggers ONLY that arm; call buildCompiledConfig; assert the returned
//     error message matches the byte-stable wording.
//
//   - TestBuildCompiledConfig_DataSource_Forward_Stub — at Task 9 the
//     resolveDataSource is a forward stub (Task 10 lands the full body);
//     this row verifies the parse-through-resolveDataSource sentinel surface.
//
//   - TestBuildSandboxConfig — verifies the zero-value (nil
//     CapabilityRestrictionConfig) StrictDefaultDeny path + the populated-
//     map path (AMEND-A1 SanitizationConfig accept-empty discipline).
//
//   - TestRootContextIDCounter_Monotonic — verifies the per-process
//     monotonic u32 counter allocates fresh IDs at compiledConfig
//     construction time per SPEC §4.2.
//
// # Arm-by-arm reachability matrix at Task 9
//
// Arms 1, 3, 4, 5, 9, 10, 11, 13, 14, 15 fire BEFORE resolveDataSource ⇒
// directly testable with the forward-stub. Arm 2 (typed-config-unmarshal)
// fires on the Any.UnmarshalTo before any field access. Arms 6 (Remote)
// + 7 (WatchedDirectory) + 8 (specifier-required) inspect the DataSource
// shape but fire BEFORE resolveDataSource is called. Arm 12 (vm_id
// duplicate) is unreachable-by-design at 25.1 (single-plugin-per-listener);
// the constant is asserted byte-exact by ByteStable + a comment-only test
// row documents the unreachability. Arms 16 + 17 (ABI + compile-failure)
// require the resolveDataSource real body — deferred to Task 10/12
// integration tests; at Task 9 the corresponding rows verify that the
// SENTINEL stub error bubbles up (the test asserts the stub-error rather
// than the production arm-16/17 wording, to isolate Task 9 from Task 10).

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	wasmcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
)

// validWasmConfig returns a baseline *wasmv3.Wasm proto with a populated
// PluginConfig.VmConfig.Code (InlineString DataSource) so the parse path
// reaches the resolveDataSource forward stub. Each PARSE-REJECT row's
// `mutate` closure modifies this baseline to trigger ONE specific arm.
func validWasmConfig() *wasmv3.Wasm {
	return &wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:   "test_plugin",
			RootId: "test_root",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    "test_vm",
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineString{
									InlineString: "some-non-wasm-bytes-stub",
								},
							},
						},
					},
				},
			},
		},
	}
}

// toAny wraps the *wasmv3.Wasm proto in an *anypb.Any envelope per the
// buildCompiledConfig signature contract.
func toAny(t *testing.T, msg *wasmv3.Wasm) *anypb.Any {
	t.Helper()
	any, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New failed: %v", err)
	}
	return any
}

// -----------------------------------------------------------------------------
// TestParseRejectConstants_ByteStable — D-P5 closure enforcement.
// -----------------------------------------------------------------------------

// TestParseRejectConstants_ByteStable pins the byte-exact wording for each
// of the 18 PARSE-REJECT arms per parent §6.2 + D-P5 closure at Task 9.
// Any drift in a constant requires a parent-SPEC §6.2 + ADR-0203 lockstep
// edit per ADR-0044 atomic-edit discipline.
func TestParseRejectConstants_ByteStable(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Arm01_TypedConfigRequired", parseRejectTypedConfigRequired, "wasm: typed_config required"},
		{"Arm02_TypedConfigUnmarshal", parseRejectTypedConfigUnmarshal, "wasm: typed_config unmarshal: %w"},
		{"Arm03_ConfigRequired", parseRejectConfigRequired, "wasm: config (PluginConfig) is required"},
		{"Arm04_VmConfigRequired", parseRejectVmConfigRequired, "wasm: config.vm_config is required"},
		{"Arm05_VmConfigCodeRequired", parseRejectVmConfigCodeRequired, "wasm: config.vm_config.code is required"},
		{"Arm06_VmConfigCodeRemoteDeferred", parseRejectVmConfigCodeRemoteDeferred, "wasm: config.vm_config.code.remote is not yet supported (lands in a future Runtime/RTDS family phase)"},
		{"Arm07_DataSourceWatchedDirectoryDeferred", parseRejectDataSourceWatchedDirectoryDeferred, "wasm: config.vm_config.code.local.watched_directory is not yet supported (lands in a future Runtime/hot-reload phase)"},
		{"Arm08_DataSourceSpecifierRequired", parseRejectDataSourceSpecifierRequired, "wasm: config.vm_config.code.local: specifier oneof required"},
		{"Arm09_PluginFailurePolicyFailReloadDeferred", parseRejectPluginFailurePolicyFailReloadDeferred, "wasm: config.failure_policy = FAIL_RELOAD (or reload_config set) is not yet supported (lands in phase 25.3)"},
		{"Arm10_PluginFailOpenDeferred", parseRejectPluginFailOpenDeferred, "wasm: config.fail_open is not yet supported (deprecated upstream; lands in phase 25.3 via failure_policy = FAIL_OPEN)"},
		{"Arm11_VmConfigRuntimeDiscriminator", parseRejectVmConfigRuntimeDiscriminator, "wasm: config.vm_config.runtime %q is not supported (envoy-go uses wazero exclusively; envoy-go-strict departure)"},
		{"Arm12_VmConfigVmIdDuplicate", parseRejectVmConfigVmIdDuplicate, "wasm: config.vm_config.vm_id %q is duplicated across PluginConfig entries (multi-plugin VM-sharing lands in phase 25.3)"},
		{"Arm13_VmConfigEnvironmentVariablesDeferred", parseRejectVmConfigEnvironmentVariablesDeferred, "wasm: config.vm_config.environment_variables is not yet supported (lands in phase 25.3)"},
		{"Arm14_VmConfigAllowPrecompiledRejected", parseRejectVmConfigAllowPrecompiledRejected, "wasm: config.vm_config.allow_precompiled is not supported (incompatible with wazero interpreter-default; envoy-go-strict departure)"},
		{"Arm15_VmConfigNackOnCodeCacheMissRejected", parseRejectVmConfigNackOnCodeCacheMissRejected, "wasm: config.vm_config.nack_on_code_cache_miss is not supported (paired with code.remote; envoy-go-strict departure)"},
		{"Arm16_ModuleAbiVersionRejected", parseRejectModuleAbiVersionRejected, "wasm: module: required proxy_abi_version_0_2_1 export not found (envoy-go-strict targets ABI v0.2.1 only; v0.1.0 + v0.2.0 + missing sentinel rejected)"},
		{"Arm17_ModuleCompileFailed", parseRejectModuleCompileFailed, "wasm: config.vm_config.code: compile: %w"},
		{"Arm18_PerRouteDeferredTo253", parseRejectPerRouteDeferredTo253, "wasm: per-route configuration is not yet supported (lands in phase 25.3)"},
	}

	if len(cases) != 18 {
		t.Fatalf("TestParseRejectConstants_ByteStable: expected 18 rows; got %d", len(cases))
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("%s = %q; want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestParseRejectArm18_AliasedFromWasmGo verifies that the shared arm-18
// constant declared in compiled_config.go is the SAME byte-string as the
// one already consumed by validatePerRouteWasm in wasm.go (Task 8). If the
// Task 8 implementer used a DIFFERENT constant name (parseRejectPerRouteUnsupported),
// the body of validatePerRouteWasm must continue to return the SAME byte-
// string — this test pins that invariant.
func TestParseRejectArm18_AliasedFromWasmGo(t *testing.T) {
	if parseRejectPerRouteDeferredTo253 != parseRejectPerRouteUnsupported {
		t.Fatalf("parseRejectPerRouteDeferredTo253 = %q must equal parseRejectPerRouteUnsupported = %q (single source of truth for arm 18)", parseRejectPerRouteDeferredTo253, parseRejectPerRouteUnsupported)
	}
}

// -----------------------------------------------------------------------------
// TestBuildCompiledConfig — table-driven PARSE-REJECT roster coverage.
// -----------------------------------------------------------------------------

func TestBuildCompiledConfig(t *testing.T) {
	t.Run("PARSE_REJECT", testBuildCompiledConfigParseReject)
	t.Run("DataSourceForwardStub", testBuildCompiledConfigDataSourceForwardStub)
}

// testBuildCompiledConfigParseReject covers the 14 PARSE-REJECT arms
// reachable WITHOUT a real wasm bytecode (arms 1, 2, 3, 4, 5, 6, 7, 8, 9,
// 10, 11, 13, 14, 15). Arm 12 is unreachable-by-design at 25.1; arms 16 +
// 17 require real wasm bytecode (Task 10/12 integration tests).
func testBuildCompiledConfigParseReject(t *testing.T) {
	cases := []struct {
		name             string
		typedConfig      func(t *testing.T) *anypb.Any
		wantErrEq        string // when set, asserts err.Error() == wantErrEq
		wantErrHasPrefix string // when set, asserts err.Error() starts with this prefix
		wantErrContains  string // when set, asserts strings.Contains(err.Error(), this)
	}{
		// ---- Arm 1: typed_config required ----
		{
			name:        "Arm01_TypedConfig_Nil",
			typedConfig: func(_ *testing.T) *anypb.Any { return nil },
			wantErrEq:   parseRejectTypedConfigRequired,
		},
		// ---- Arm 2: typed_config unmarshal failure ----
		{
			name: "Arm02_TypedConfig_UnmarshalFailure",
			typedConfig: func(_ *testing.T) *anypb.Any {
				// Garbage payload bytes; UnmarshalTo into *Wasm surfaces a
				// proto decoder error wrapped with arm-2 prefix.
				return &anypb.Any{
					TypeUrl: TypeURL,
					Value:   []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
				}
			},
			wantErrHasPrefix: "wasm: typed_config unmarshal: ",
		},
		// ---- Arm 3: config (PluginConfig) is required ----
		{
			name: "Arm03_ConfigRequired",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := &wasmv3.Wasm{Config: nil}
				return toAny(t, m)
			},
			wantErrEq: parseRejectConfigRequired,
		},
		// ---- Arm 4: vm_config required ----
		{
			name: "Arm04_VmConfigRequired",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := &wasmv3.Wasm{Config: &wasmcommonv3.PluginConfig{Name: "p", Vm: nil}}
				return toAny(t, m)
			},
			wantErrEq: parseRejectVmConfigRequired,
		},
		// ---- Arm 5: vm_config.code required ----
		{
			name: "Arm05_VmConfigCodeRequired",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.Vm = &wasmcommonv3.PluginConfig_VmConfig{
					VmConfig: &wasmcommonv3.VmConfig{
						Runtime: "envoy.wasm.runtime.wazero",
						Code:    nil,
					},
				}
				return toAny(t, m)
			},
			wantErrEq: parseRejectVmConfigCodeRequired,
		},
		// ---- Arm 6: vm_config.code.remote deferred ----
		{
			name: "Arm06_VmConfigCodeRemoteDeferred",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.Vm = &wasmcommonv3.PluginConfig_VmConfig{
					VmConfig: &wasmcommonv3.VmConfig{
						Runtime: "envoy.wasm.runtime.wazero",
						Code: &corev3.AsyncDataSource{
							Specifier: &corev3.AsyncDataSource_Remote{
								Remote: &corev3.RemoteDataSource{},
							},
						},
					},
				}
				return toAny(t, m)
			},
			wantErrEq: parseRejectVmConfigCodeRemoteDeferred,
		},
		// ---- Arm 7: watched_directory deferred ----
		{
			name: "Arm07_DataSourceWatchedDirectoryDeferred",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.Vm = &wasmcommonv3.PluginConfig_VmConfig{
					VmConfig: &wasmcommonv3.VmConfig{
						Runtime: "envoy.wasm.runtime.wazero",
						Code: &corev3.AsyncDataSource{
							Specifier: &corev3.AsyncDataSource_Local{
								Local: &corev3.DataSource{
									Specifier: &corev3.DataSource_Filename{Filename: "/tmp/foo.wasm"},
									WatchedDirectory: &corev3.WatchedDirectory{
										Path: "/tmp",
									},
								},
							},
						},
					},
				}
				return toAny(t, m)
			},
			wantErrEq: parseRejectDataSourceWatchedDirectoryDeferred,
		},
		// ---- Arm 8: data_source specifier oneof required ----
		// local set, but specifier oneof unset.
		{
			name: "Arm08_DataSourceSpecifierRequired_SpecifierUnset",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.Vm = &wasmcommonv3.PluginConfig_VmConfig{
					VmConfig: &wasmcommonv3.VmConfig{
						Runtime: "envoy.wasm.runtime.wazero",
						Code: &corev3.AsyncDataSource{
							Specifier: &corev3.AsyncDataSource_Local{
								Local: &corev3.DataSource{
									// Specifier unset; WatchedDirectory unset.
								},
							},
						},
					},
				}
				return toAny(t, m)
			},
			wantErrEq: parseRejectDataSourceSpecifierRequired,
		},
		// ---- Arm 9: failure_policy = FAIL_RELOAD deferred ----
		{
			name: "Arm09_FailurePolicy_FailReload_Deferred",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_RELOAD
				return toAny(t, m)
			},
			wantErrEq: parseRejectPluginFailurePolicyFailReloadDeferred,
		},
		// ---- Arm 9 variant: reload_config set (independent trigger) ----
		{
			name: "Arm09_ReloadConfig_Set_Deferred",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.ReloadConfig = &wasmcommonv3.ReloadConfig{}
				return toAny(t, m)
			},
			wantErrEq: parseRejectPluginFailurePolicyFailReloadDeferred,
		},
		// ---- Arm 10: fail_open deferred ----
		{
			name: "Arm10_FailOpen_Deferred",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.FailOpen = true //nolint:staticcheck // SA1019: arm 10 EXISTS to PARSE-REJECT this deprecated proto field; intentional access.
				return toAny(t, m)
			},
			wantErrEq: parseRejectPluginFailOpenDeferred,
		},
		// ---- Arm 11: vm_config.runtime discriminator (only wazero supported) ----
		{
			name: "Arm11_Runtime_V8_Rejected",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.GetVmConfig().Runtime = "envoy.wasm.runtime.v8"
				return toAny(t, m)
			},
			wantErrContains: `"envoy.wasm.runtime.v8"`,
		},
		// ---- Arm 13: vm_config.environment_variables deferred ----
		{
			name: "Arm13_EnvironmentVariables_Deferred",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.GetVmConfig().EnvironmentVariables = &wasmcommonv3.EnvironmentVariables{
					HostEnvKeys: []string{"PATH"},
				}
				return toAny(t, m)
			},
			wantErrEq: parseRejectVmConfigEnvironmentVariablesDeferred,
		},
		// ---- Arm 14: vm_config.allow_precompiled rejected ----
		{
			name: "Arm14_AllowPrecompiled_Rejected",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.GetVmConfig().AllowPrecompiled = true
				return toAny(t, m)
			},
			wantErrEq: parseRejectVmConfigAllowPrecompiledRejected,
		},
		// ---- Arm 15: vm_config.nack_on_code_cache_miss rejected ----
		{
			name: "Arm15_NackOnCodeCacheMiss_Rejected",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.GetVmConfig().NackOnCodeCacheMiss = true
				return toAny(t, m)
			},
			wantErrEq: parseRejectVmConfigNackOnCodeCacheMissRejected,
		},
		// ---- Variant: runtime explicitly set to "envoy.wasm.runtime.wazero" passes the runtime
		// arm but still hits the resolveDataSource forward stub (covered by
		// the separate DataSourceForwardStub test below). ----
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			_, err := buildCompiledConfig(ctx, tc.typedConfig(t), envoyhttp.FactoryCtx{})
			if err == nil {
				t.Fatalf("buildCompiledConfig returned nil error; want PARSE-REJECT")
			}
			if tc.wantErrEq != "" && err.Error() != tc.wantErrEq {
				t.Fatalf("err.Error() = %q; want %q", err.Error(), tc.wantErrEq)
			}
			if tc.wantErrHasPrefix != "" && !strings.HasPrefix(err.Error(), tc.wantErrHasPrefix) {
				t.Fatalf("err.Error() = %q; want prefix %q", err.Error(), tc.wantErrHasPrefix)
			}
			if tc.wantErrContains != "" && !strings.Contains(err.Error(), tc.wantErrContains) {
				t.Fatalf("err.Error() = %q; want contains %q", err.Error(), tc.wantErrContains)
			}
		})
	}
}

// testBuildCompiledConfigDataSourceForwardStub originally (at Task 9) verified
// that the resolveDataSource FORWARD STUB bubbled its sentinel error up.
// Task 10 REPLACED the stub with the real 4-arm body; the validWasmConfig
// baseline now flows THROUGH resolveDataSource → CompileModule and surfaces
// arm 17 (compile-failed %w-wrap) since the InlineString "some-non-wasm-bytes-
// stub" is not valid wasm bytecode. This subtest is RENAMED in spirit to
// "DataSource_Through_To_Arm17_CompileFailed" but keeps its testBuild-
// CompiledConfigDataSourceForwardStub Go-name to minimize the diff against
// the Task 9 surface; the controller wires the subtest name explicitly via
// t.Run("DataSourceForwardStub", ...). Task 10's datasource_test.go adds
// the explicit TestBuildCompiledConfig_DataSource_Arm17_CompileFailed_*
// tests that exercise BOTH the InlineString + Filename arm flow-through.
func testBuildCompiledConfigDataSourceForwardStub(t *testing.T) {
	ctx := context.Background()
	m := validWasmConfig()
	_, err := buildCompiledConfig(ctx, toAny(t, m), envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("buildCompiledConfig returned nil error; want arm-17 compile-failed wrap (post-Task-10)")
	}
	// Task 10: synthetic non-wasm bytes flow through resolveDataSource +
	// surface arm 17 (compile-failed). The substring is the arm-17 prefix.
	const wantPrefix = "wasm: config.vm_config.code: compile: "
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err.Error() = %q; want prefix %q (arm-17 compile-failed via real resolveDataSource)", err.Error(), wantPrefix)
	}
}

// -----------------------------------------------------------------------------
// TestBuildSandboxConfig — capability-restriction proto → SandboxConfig.
// -----------------------------------------------------------------------------

// TestBuildSandboxConfig_NilRestriction verifies the AMEND-A5 default-deny
// zero-value path: nil CapabilityRestrictionConfig ⇒ SandboxConfig{} (nil
// AllowedCapabilities map) ⇒ StrictDefaultDeny.
func TestBuildSandboxConfig_NilRestriction(t *testing.T) {
	sb := buildSandboxConfig(nil)
	if sb.AllowedCapabilities != nil {
		t.Fatalf("buildSandboxConfig(nil).AllowedCapabilities = %v; want nil (StrictDefaultDeny zero-value)", sb.AllowedCapabilities)
	}
	// Sanity: IsAllowed returns false for any key.
	if sb.IsAllowed("proxy_log") {
		t.Fatalf("StrictDefaultDeny SandboxConfig allowed proxy_log; want denied")
	}
}

// TestBuildSandboxConfig_EmptyAllowedCapabilities verifies the AMEND-A5
// inversion of upstream's bare-empty-map-allow-all: a non-nil but empty
// AllowedCapabilities map still produces an empty (deny-all) sandbox.
func TestBuildSandboxConfig_EmptyAllowedCapabilities(t *testing.T) {
	restrict := &wasmcommonv3.CapabilityRestrictionConfig{
		AllowedCapabilities: map[string]*wasmcommonv3.SanitizationConfig{},
	}
	sb := buildSandboxConfig(restrict)
	if len(sb.AllowedCapabilities) != 0 {
		t.Fatalf("buildSandboxConfig(empty map).AllowedCapabilities len = %d; want 0", len(sb.AllowedCapabilities))
	}
	if sb.IsAllowed("proxy_log") {
		t.Fatalf("empty-map SandboxConfig allowed proxy_log; want denied (AMEND-A5 INVERSION)")
	}
}

// TestBuildSandboxConfig_PopulatedMap verifies that allowed capabilities
// thread through to the SandboxConfig. AMEND-A1: SanitizationConfig is
// empty (accept-empty discipline) so the value is ignored but its presence
// is the allow-signal.
func TestBuildSandboxConfig_PopulatedMap(t *testing.T) {
	restrict := &wasmcommonv3.CapabilityRestrictionConfig{
		AllowedCapabilities: map[string]*wasmcommonv3.SanitizationConfig{
			"proxy_log":                          {},
			"proxy_get_header_map_value":         nil, // accept-empty: nil value is the same as empty
			"proxy_send_local_response":          {},
			"proxy_get_current_time_nanoseconds": {},
		},
	}
	sb := buildSandboxConfig(restrict)
	if len(sb.AllowedCapabilities) != 4 {
		t.Fatalf("buildSandboxConfig.AllowedCapabilities len = %d; want 4", len(sb.AllowedCapabilities))
	}
	for _, cap := range []string{"proxy_log", "proxy_get_header_map_value", "proxy_send_local_response", "proxy_get_current_time_nanoseconds"} {
		if !sb.IsAllowed(cap) {
			t.Errorf("sb.IsAllowed(%q) = false; want true", cap)
		}
		// Verify the value is the empty SanitizationConfig per AMEND-A1.
		got, ok := sb.AllowedCapabilities[cap]
		if !ok {
			t.Errorf("sb.AllowedCapabilities[%q] missing", cap)
			continue
		}
		if got != (internalwasm.SanitizationConfig{}) {
			t.Errorf("sb.AllowedCapabilities[%q] = %+v; want empty SanitizationConfig{}", cap, got)
		}
	}
	// Negative check: any key not in the map is denied.
	if sb.IsAllowed("proxy_set_property") {
		t.Errorf("sb.IsAllowed(proxy_set_property) = true; want false (not in allow-set)")
	}
}

// -----------------------------------------------------------------------------
// TestRootContextIDCounter_Monotonic — SPEC §4.2 monotonic u32 counter.
// -----------------------------------------------------------------------------

// TestRootContextIDCounter_Monotonic verifies the package-level counter
// allocates fresh strictly-increasing u32 IDs across calls. Per SPEC §4.2
// the counter is per-process; this test must observe distinct IDs from
// any two consecutive Add() calls — even if other tests ran before this
// one and bumped the counter.
func TestRootContextIDCounter_Monotonic(t *testing.T) {
	id1 := rootContextIDCounter.Add(1)
	id2 := rootContextIDCounter.Add(1)
	id3 := rootContextIDCounter.Add(1)
	if id2 != id1+1 || id3 != id2+1 {
		t.Fatalf("rootContextIDCounter not monotonic: id1=%d id2=%d id3=%d", id1, id2, id3)
	}
}

// TestRootContextIDCounter_Concurrent verifies the counter is safe under
// concurrent allocation (per atomic.Uint32 contract) and produces N unique
// IDs from N goroutines.
func TestRootContextIDCounter_Concurrent(t *testing.T) {
	const N = 100
	seen := make(map[uint32]struct{}, N)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			id := rootContextIDCounter.Add(1)
			mu.Lock()
			seen[id] = struct{}{}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(seen) != N {
		t.Fatalf("rootContextIDCounter concurrent: %d unique IDs; want %d", len(seen), N)
	}
}

// -----------------------------------------------------------------------------
// Arm-12 unreachability documentation.
// -----------------------------------------------------------------------------

// TestParseRejectArm12_UnreachableByDesignAt251 documents the arm-12
// "unreachable-by-design at 25.1" disposition. The constant exists +
// is byte-stable (asserted by TestParseRejectConstants_ByteStable), but
// there is no production trigger path in buildCompiledConfig at the
// single-plugin-per-listener model. Phase 25.3 wires the multi-plugin
// VM-sharing registry that activates this arm. The test merely asserts
// the constant is non-empty so any future deletion surfaces here.
func TestParseRejectArm12_UnreachableByDesignAt251(t *testing.T) {
	if parseRejectVmConfigVmIdDuplicate == "" {
		t.Fatal("parseRejectVmConfigVmIdDuplicate is empty; constant MUST exist (reserved for 25.3 multi-plugin VM-sharing registry)")
	}
}

// -----------------------------------------------------------------------------
// Arm-16 + Arm-17 deferred-test markers.
// -----------------------------------------------------------------------------

// TestParseRejectArm16_DeferredToIntegration documents that arm-16
// (module-abi-version-rejected) cannot be exercised at Task 9 without
// real wasm bytecode (Task 10 datasource.go body + Task 15 fixture-0034
// vendored .wasm). The constant byte-stability is asserted by
// TestParseRejectConstants_ByteStable; the production trigger path is
// the errors.Is(err, wasm.ErrUnsupportedAbiVersion) branch in
// buildCompiledConfig — verified at integration time.
func TestParseRejectArm16_DeferredToIntegration(t *testing.T) {
	if parseRejectModuleAbiVersionRejected == "" {
		t.Fatal("parseRejectModuleAbiVersionRejected is empty; constant MUST exist")
	}
	// Sanity: ensure the wasm.ErrUnsupportedAbiVersion sentinel exists in
	// the internal/wasm/ package so the buildCompiledConfig branch compiles.
	if !errors.Is(internalwasm.ErrUnsupportedAbiVersion, internalwasm.ErrUnsupportedAbiVersion) {
		t.Fatal("internalwasm.ErrUnsupportedAbiVersion sentinel missing or broken")
	}
}

// TestParseRejectArm17_DeferredToIntegration documents the same deferral
// for arm-17 (module-compile-failed). The %w format string requires a
// wazero compile error from real wasm bytecode.
func TestParseRejectArm17_DeferredToIntegration(t *testing.T) {
	if parseRejectModuleCompileFailed == "" {
		t.Fatal("parseRejectModuleCompileFailed is empty; constant MUST exist")
	}
}
