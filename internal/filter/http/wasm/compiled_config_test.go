package wasm

// compiled_config_test.go — Task 9 RIGID-TDD test surface per 25.1 PLAN
// Task 9 + parent SPEC §6.2 18-arm PARSE-REJECT roster + D-P5 closure
// (25.1 byte-stable wording finalization at Task 9). EXTENDED at 25.2 Task
// 14 (D-25.2-P5 closure) with 6 NEW arms (19, 20, 21, 22, 23, 26) + 4
// envoy-go-strict-only PluginConfig cap-field validators + RootVM/Registry/
// foreignReg construction smoke coverage.
//
// # Test surface coverage
//
//   - TestParseRejectConstants_ByteStable — table-driven; 24 rows (18 from
//     25.1 + 6 from 25.2); asserts each `parseReject*` package-private
//     constant matches the SPEC wording byte-exact. D-P5 + D-25.2-P5 closure
//     enforcement at commit time.
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
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	wasmcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
	internalwasm "github.com/pgdad/envoy-go/internal/wasm"
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
// of the 24 PARSE-REJECT arms per parent §6.2 (arms 1-18) + 25.2 SPEC §6.2
// (arms 19-23, 26) + D-P5 closure at 25.1 Task 9 + D-25.2-P5 closure at
// 25.2 Task 14. Any drift in a constant requires a parent-SPEC §6.2 /
// 25.2 SPEC §6.2 + ADR-0203 / ADR-0208 lockstep edit per ADR-0044 atomic-
// edit discipline.
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
		// Arms 9, 10, 12, 13, 18 LIFTED at phase-25.3 Task 7 (failure_policy /
		// reload_config / fail_open / environment_variables / per-route now
		// CONSUMED); their deferral constants are RETIRED + dropped from the
		// roster. The residual rejects on those surfaces are the NEW arms A/B/C.
		{"Arm11_VmConfigRuntimeDiscriminator", parseRejectVmConfigRuntimeDiscriminator, "wasm: config.vm_config.runtime %q is not supported (envoy-go uses wazero exclusively; envoy-go-strict departure)"},
		{"Arm14_VmConfigAllowPrecompiledRejected", parseRejectVmConfigAllowPrecompiledRejected, "wasm: config.vm_config.allow_precompiled is not supported (incompatible with wazero interpreter-default; envoy-go-strict departure)"},
		{"Arm15_VmConfigNackOnCodeCacheMissRejected", parseRejectVmConfigNackOnCodeCacheMissRejected, "wasm: config.vm_config.nack_on_code_cache_miss is not supported (paired with code.remote; envoy-go-strict departure)"},
		{"Arm16_ModuleAbiVersionRejected", parseRejectModuleAbiVersionRejected, "wasm: module: required proxy_abi_version_0_2_1 export not found (envoy-go-strict targets ABI v0.2.1 only; v0.1.0 + v0.2.0 + missing sentinel rejected)"},
		{"Arm17_ModuleCompileFailed", parseRejectModuleCompileFailed, "wasm: config.vm_config.code: compile: %w"},

		// 25.2 NEW arms 19-23 + 26 per D-25.2-P5 closure at Task 14.
		{"Arm19_EnvoyGoStrictBodyBufferCapBytesZero", parseRejectEnvoyGoStrictBodyBufferCapBytesZero, "wasm: config.envoy_go_strict_body_buffer_cap_bytes must be > 0 (envoy-go-strict)"},
		{"Arm20_EnvoyGoStrictSharedDataValueCapBytesZero", parseRejectEnvoyGoStrictSharedDataValueCapBytesZero, "wasm: config.envoy_go_strict_shared_data_value_cap_bytes must be > 0 (envoy-go-strict)"},
		{"Arm21_EnvoyGoStrictSharedDataMaxEntriesZero", parseRejectEnvoyGoStrictSharedDataMaxEntriesZero, "wasm: config.envoy_go_strict_shared_data_max_entries must be > 0 (envoy-go-strict)"},
		{"Arm22_EnvoyGoStrictDynamicStatsMaxEntriesZero", parseRejectEnvoyGoStrictDynamicStatsMaxEntriesZero, "wasm: config.envoy_go_strict_dynamic_stats_max_entries must be > 0 (envoy-go-strict)"},
		{"Arm23_EnvoyGoStrictBodyBufferCapBytesOverlarge", parseRejectEnvoyGoStrictBodyBufferCapBytesOverlarge, "wasm: config.envoy_go_strict_body_buffer_cap_bytes %d exceeds 1 GiB ceiling (envoy-go-strict)"},
		{"Arm26_CrossPluginConfigDuplicatePluginConfigName", parseRejectCrossPluginConfigDuplicatePluginConfigName, "wasm: config.name %q is duplicated across PluginConfig entries (per-plugin stat-scope uniqueness; envoy-go-strict)"},

		// 25.3 NEW arms A/B/C per D-25.3-P2 closure at Task 7.
		{"ArmA_EnvVarsKeyCollision", parseRejectEnvVarsKeyCollision, "wasm: config.vm_config.environment_variables: key %q is duplicated across host_env_keys and key_values (all keys must be unique)"},
		{"ArmB_FailOpenAndFailurePolicyBothSet", parseRejectFailOpenAndFailurePolicyBothSet, "wasm: only one of config.fail_open or config.failure_policy can be set"},
		{"ArmC_EnvVarsCapExceeded", parseRejectEnvVarsCapExceeded, "wasm: config.vm_config.environment_variables exceeds the envoy-go-strict cap (max 64 entries, max 4096 bytes per value)"},
	}

	// Roster size: 24 (18 from 25.1 + 6 from 25.2) − 5 LIFTED (arms 9, 10, 12,
	// 13, 18) + 3 NEW 25.3 arms (A, B, C) = 22.
	if len(cases) != 22 {
		t.Fatalf("TestParseRejectConstants_ByteStable: expected 22 rows (24 − 5 lifted at 25.3 Task 7 + 3 NEW arms A/B/C); got %d", len(cases))
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

// TestEnvoyGoStrictKeyConstants_ByteStable pins the byte-exact wire-key
// strings carried inside the typed Struct at PluginConfig.configuration per
// the parsing-mechanism documented at compiled_config.go. Operator configs
// REFERENCE these keys; any drift would silently break operator wire
// configs.
func TestEnvoyGoStrictKeyConstants_ByteStable(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"TopLevelKey", envoyGoStrictKey, "envoy_go_strict"},
		{"BodyBufferCapBytesKey", envoyGoStrictBodyBufferCapBytesKey, "body_buffer_cap_bytes"},
		{"SharedDataValueCapBytesKey", envoyGoStrictSharedDataValueCapBytesKey, "shared_data_value_cap_bytes"},
		{"SharedDataMaxEntriesKey", envoyGoStrictSharedDataMaxEntriesKey, "shared_data_max_entries"},
		{"DynamicStatsMaxEntriesKey", envoyGoStrictDynamicStatsMaxEntriesKey, "dynamic_stats_max_entries"},
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

// TestEnvoyGoStrictDefaults_ByteStable pins the 4 default cap values per Qs
// 2/6/9 + 25.2 SPEC §7.4. Any operator-observable shift requires a SPEC +
// behavior-contract atomic edit per ADR-0044.
func TestEnvoyGoStrictDefaults_ByteStable(t *testing.T) {
	if defaultBodyBufferCapBytes != 16*1024*1024 {
		t.Fatalf("defaultBodyBufferCapBytes = %d; want 16777216 (16 MiB per Q2)", defaultBodyBufferCapBytes)
	}
	if defaultSharedDataValueCapBytes != 1024*1024 {
		t.Fatalf("defaultSharedDataValueCapBytes = %d; want 1048576 (1 MiB per Q6)", defaultSharedDataValueCapBytes)
	}
	if defaultSharedDataMaxEntries != 1024 {
		t.Fatalf("defaultSharedDataMaxEntries = %d; want 1024 (Q6)", defaultSharedDataMaxEntries)
	}
	if defaultDynamicStatsMaxEntries != 1024 {
		t.Fatalf("defaultDynamicStatsMaxEntries = %d; want 1024 (Q9)", defaultDynamicStatsMaxEntries)
	}
	if bodyBufferCapBytesCeiling != 1<<30 {
		t.Fatalf("bodyBufferCapBytesCeiling = %d; want 1073741824 (1 GiB ceiling per arm 23)", bodyBufferCapBytesCeiling)
	}
}

// TestValidatePerRouteWasm_LiftedArm18 verifies that arm 18 is LIFTED at
// phase-25.3 Task 7: validatePerRouteWasm now validates the per-route Wasm
// shape (a valid per-route override ACCEPTS; an invalid one rejects with the
// SAME byte-stable buildCompiledConfig wording — single source of truth).
// The old "per-route not yet supported" deferral wording is RETIRED.
func TestValidatePerRouteWasm_LiftedArm18(t *testing.T) {
	t.Run("invalid_missing_config_delegates_arm3", func(t *testing.T) {
		// A *wasmv3.Wasm with Config=nil must reject with arm-3 wording,
		// proving the validate-only delegation to buildCompiledConfig.
		err := validatePerRouteWasm(&wasmv3.Wasm{Config: nil})
		if err == nil {
			t.Fatal("validatePerRouteWasm(invalid) returned nil; want arm-3 PARSE-REJECT")
		}
		if err.Error() != parseRejectConfigRequired {
			t.Fatalf("validatePerRouteWasm err = %q; want %q (arm-3, single source of truth)", err.Error(), parseRejectConfigRequired)
		}
	})

	t.Run("valid_shape_accepts_and_does_not_leak", func(t *testing.T) {
		// A valid per-route Wasm (real bytecode) must ACCEPT (nil error) +
		// must NOT acquire a registry refcount NOR claim the plugin name (so a
		// second validation of the SAME name does not arm-26-FALSE-reject).
		modBytes := buildContinueProxyWasm()
		name := "perroute_validate_only_plugin"
		w := &wasmv3.Wasm{
			Config: &wasmcommonv3.PluginConfig{
				Name: name,
				Vm: &wasmcommonv3.PluginConfig_VmConfig{
					VmConfig: &wasmcommonv3.VmConfig{
						VmId:    "perroute_validate_vm",
						Runtime: "envoy.wasm.runtime.wazero",
						Code: &corev3.AsyncDataSource{
							Specifier: &corev3.AsyncDataSource_Local{
								Local: &corev3.DataSource{
									Specifier: &corev3.DataSource_InlineBytes{InlineBytes: modBytes},
								},
							},
						},
					},
				},
			},
		}
		if err := validatePerRouteWasm(w); err != nil {
			t.Fatalf("validatePerRouteWasm(valid) = %v; want nil (lifted arm 18 accepts valid shape)", err)
		}
		// Re-validate the SAME name: must still ACCEPT (no arm-26 claim leaked).
		// A build-then-discard validator would have claimed `name` in the
		// process-wide append-only registry on the first call; the second call
		// would then arm-26-FALSE-reject. validate-only skips the claim, so the
		// second call accepts — this is the load-bearing leak-avoidance check.
		if err := validatePerRouteWasm(w); err != nil {
			t.Fatalf("validatePerRouteWasm(valid, 2nd time same name) = %v; want nil (validate-only must NOT claim the plugin name)", err)
		}
	})

	t.Run("wrong_proto_type", func(t *testing.T) {
		err := validatePerRouteWasm(nil)
		if err == nil {
			t.Fatal("validatePerRouteWasm(nil) returned nil; want type-assert error")
		}
		if !strings.Contains(err.Error(), "expected *wasmv3.Wasm") {
			t.Fatalf("validatePerRouteWasm(nil) err = %q; want type-mismatch wording", err.Error())
		}
	})
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
		// ---- Arm 9 LIFTED: failure_policy = FAIL_RELOAD now CONSUMED ----
		// The config is otherwise valid, so it flows THROUGH to arm-17
		// (compile-failed) since the InlineString is not valid wasm. (The
		// FAIL_RELOAD → wasm.FailurePolicyFailReload mapping itself is covered
		// by TestFailurePolicy_Mapping.)
		{
			name: "Arm09_FailurePolicy_FailReload_Consumed",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.Name = "Arm09_FailurePolicy_FailReload_Consumed"
				m.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_RELOAD
				return toAny(t, m)
			},
			wantErrHasPrefix: "wasm: config.vm_config.code: compile: ",
		},
		// ---- Arm 9 variant LIFTED: reload_config set now CONSUMED ----
		{
			name: "Arm09_ReloadConfig_Set_Consumed",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.Name = "Arm09_ReloadConfig_Set_Consumed"
				m.Config.ReloadConfig = &wasmcommonv3.ReloadConfig{}
				return toAny(t, m)
			},
			wantErrHasPrefix: "wasm: config.vm_config.code: compile: ",
		},
		// ---- Arm 10 LIFTED: fail_open now CONSUMED (→ FAIL_OPEN) ----
		{
			name: "Arm10_FailOpen_Consumed",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.Name = "Arm10_FailOpen_Consumed"
				m.Config.FailOpen = true //nolint:staticcheck // SA1019: fail_open is deprecated but envoy-go still PARSES it (→ FAIL_OPEN) per AMEND-C3.
				return toAny(t, m)
			},
			wantErrHasPrefix: "wasm: config.vm_config.code: compile: ",
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
		// ---- Arm 13 LIFTED: environment_variables now CONSUMED ----
		// A well-formed (non-colliding, under-cap) env block is assembled +
		// fed to the RootVM; the config then flows THROUGH to arm-17 since the
		// InlineString is not valid wasm. (Collision + cap rejects are covered
		// by TestParse_EnvVarsCollisionReject + TestParse_EnvVarsCapExceededReject.)
		{
			name: "Arm13_EnvironmentVariables_Consumed",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validWasmConfig()
				m.Config.Name = "Arm13_EnvironmentVariables_Consumed"
				m.Config.GetVmConfig().EnvironmentVariables = &wasmcommonv3.EnvironmentVariables{
					HostEnvKeys: []string{"PATH"},
					KeyValues:   map[string]string{"GUEST_KEY": "guest_value"},
				}
				return toAny(t, m)
			},
			wantErrHasPrefix: "wasm: config.vm_config.code: compile: ",
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
// Arm-12 RETIRED → multi-plugin VM-sharing (phase-25.3 Task 7).
// -----------------------------------------------------------------------------

// TestMultiPlugin_SameVmIdSharesRootVM verifies the arm-12 INVERSION: two
// compiledConfigs with the SAME (vm_id, vm_configuration, code) no longer
// reject — they SHARE one *RootVM via the registry refcount (AMEND-C2). Asserts
// pointer identity of the shared *RootVM + that the second build did NOT
// construct a fresh VM.
func TestMultiPlugin_SameVmIdSharesRootVM(t *testing.T) {
	modBytes := buildContinueProxyWasm()
	reg := stats.NewRegistry()

	// Two DISTINCT plugin names (arm 26 keys off name, not vm_id) but the SAME
	// vm_id + code → same composite VM key → shared *RootVM.
	cc1 := newTestCompiledConfigVmId(t, modBytes, "share_plugin_A", "shared_vm_id", reg)
	cc2 := newTestCompiledConfigVmId(t, modBytes, "share_plugin_B", "shared_vm_id", reg)
	t.Cleanup(func() {
		_ = cc1.Close()
		_ = cc2.Close()
	})

	if cc1.rootVM == nil || cc2.rootVM == nil {
		t.Fatal("both compiledConfigs must have a non-nil rootVM")
	}
	if cc1.rootVM != cc2.rootVM {
		t.Fatal("same (vm_id, vm_configuration, code) must SHARE one *RootVM (arm-12 INVERSION)")
	}
	if cc1.vmKey != cc2.vmKey {
		t.Fatalf("shared configs must carry the same vmKey; got %q vs %q", cc1.vmKey, cc2.vmKey)
	}
	if cc1.rootCB != cc2.rootCB {
		t.Fatal("shared configs must reference the SAME rootABICallbacks multiplexer (per-RootVM)")
	}
}

// TestMultiPlugin_DistinctVmIdDistinctRootVM is the negation: distinct vm_ids
// (otherwise identical config) get DISTINCT *RootVMs (the key differs by vm_id).
func TestMultiPlugin_DistinctVmIdDistinctRootVM(t *testing.T) {
	modBytes := buildContinueProxyWasm()
	reg := stats.NewRegistry()

	cc1 := newTestCompiledConfigVmId(t, modBytes, "distinct_plugin_A", "vm_id_one", reg)
	cc2 := newTestCompiledConfigVmId(t, modBytes, "distinct_plugin_B", "vm_id_two", reg)
	t.Cleanup(func() {
		_ = cc1.Close()
		_ = cc2.Close()
	})

	if cc1.rootVM == cc2.rootVM {
		t.Fatal("distinct vm_ids must get distinct *RootVMs")
	}
	if cc1.vmKey == cc2.vmKey {
		t.Fatal("distinct vm_ids must produce distinct vmKeys")
	}
}

// newTestCompiledConfigVmId builds a *compiledConfig with an explicit vm_id +
// plugin name + InlineBytes module (mirrors newTestCompiledConfig but exposes
// vm_id for the VM-sharing tests).
func newTestCompiledConfigVmId(t *testing.T, modBytes []byte, pluginName, vmID string, reg *stats.Registry) *compiledConfig {
	t.Helper()
	cfg := &wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:   pluginName,
			RootId: "test_root",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    vmID,
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{InlineBytes: modBytes},
							},
						},
					},
				},
			},
		},
	}
	cc, err := buildCompiledConfig(context.Background(), toAny(t, cfg), envoyhttp.FactoryCtx{Stats: reg})
	if err != nil {
		t.Fatalf("buildCompiledConfig(vm_id=%q): %v", vmID, err)
	}
	return cc
}

// -----------------------------------------------------------------------------
// phase-25.3 Task 7: failure_policy / reload_config / fail_open + env_vars.
// -----------------------------------------------------------------------------

// TestFailurePolicy_Mapping verifies the proto FailurePolicy → wasm.FailurePolicy
// mapping (UNSPECIFIED → FailClosed default; FAIL_RELOAD; FAIL_CLOSED; FAIL_OPEN)
// + the reload base interval parse from reload_config.backoff.base_interval.
func TestFailurePolicy_Mapping(t *testing.T) {
	cases := []struct {
		name       string
		proto      wasmcommonv3.FailurePolicy
		wantPolicy internalwasm.FailurePolicy
	}{
		{"unspecified_defaults_failclosed", wasmcommonv3.FailurePolicy_UNSPECIFIED, internalwasm.FailurePolicyFailClosed},
		{"fail_reload", wasmcommonv3.FailurePolicy_FAIL_RELOAD, internalwasm.FailurePolicyFailReload},
		{"fail_closed", wasmcommonv3.FailurePolicy_FAIL_CLOSED, internalwasm.FailurePolicyFailClosed},
		{"fail_open", wasmcommonv3.FailurePolicy_FAIL_OPEN, internalwasm.FailurePolicyFailOpen},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pc := &wasmcommonv3.PluginConfig{FailurePolicy: tc.proto}
			got, base, err := parseFailurePolicy(pc)
			if err != nil {
				t.Fatalf("parseFailurePolicy: unexpected error: %v", err)
			}
			if got != tc.wantPolicy {
				t.Fatalf("policy = %v; want %v", got, tc.wantPolicy)
			}
			if base != 0 {
				t.Fatalf("base interval = %v; want 0 (no reload_config)", base)
			}
		})
	}

	t.Run("reload_config_base_interval_parsed", func(t *testing.T) {
		pc := &wasmcommonv3.PluginConfig{
			FailurePolicy: wasmcommonv3.FailurePolicy_FAIL_RELOAD,
			ReloadConfig: &wasmcommonv3.ReloadConfig{
				Backoff: &corev3.BackoffStrategy{
					BaseInterval: durationpb.New(250 * time.Millisecond),
				},
			},
		}
		got, base, err := parseFailurePolicy(pc)
		if err != nil {
			t.Fatalf("parseFailurePolicy: unexpected error: %v", err)
		}
		if got != internalwasm.FailurePolicyFailReload {
			t.Fatalf("policy = %v; want FailReload", got)
		}
		if base != 250*time.Millisecond {
			t.Fatalf("base interval = %v; want 250ms", base)
		}
	})
}

// TestFailOpen_MapsToFailOpen verifies the deprecated fail_open knob maps to
// FailurePolicyFailOpen (when failure_policy is UNSPECIFIED).
func TestFailOpen_MapsToFailOpen(t *testing.T) {
	pc := &wasmcommonv3.PluginConfig{
		FailOpen: true, //nolint:staticcheck // SA1019: fail_open deprecated but PARSED per AMEND-C3.
	}
	got, _, err := parseFailurePolicy(pc)
	if err != nil {
		t.Fatalf("parseFailurePolicy: unexpected error: %v", err)
	}
	if got != internalwasm.FailurePolicyFailOpen {
		t.Fatalf("policy = %v; want FailOpen (fail_open=true)", got)
	}
}

// TestFailurePolicy_FailOpenAndFailurePolicyBothSet_Reject verifies the NEW
// arm B mutual-exclusivity reject: fail_open AND failure_policy both set.
func TestFailurePolicy_FailOpenAndFailurePolicyBothSet_Reject(t *testing.T) {
	pc := &wasmcommonv3.PluginConfig{
		FailOpen:      true, //nolint:staticcheck // SA1019: deprecated; arm B asserts the both-set reject.
		FailurePolicy: wasmcommonv3.FailurePolicy_FAIL_CLOSED,
	}
	_, _, err := parseFailurePolicy(pc)
	if err == nil {
		t.Fatal("parseFailurePolicy(both set) = nil; want arm-B PARSE-REJECT")
	}
	if err.Error() != parseRejectFailOpenAndFailurePolicyBothSet {
		t.Fatalf("err = %q; want %q (arm B)", err.Error(), parseRejectFailOpenAndFailurePolicyBothSet)
	}

	// End-to-end through buildCompiledConfig: the both-set config must reject
	// with arm-B wording (fires BEFORE the compile/arm-17 flow-through).
	m := validWasmConfig()
	m.Config.FailOpen = true //nolint:staticcheck // SA1019: deprecated; arm B both-set reject.
	m.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_CLOSED
	_, err = buildCompiledConfig(context.Background(), toAny(t, m), envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("buildCompiledConfig(both set) = nil; want arm-B PARSE-REJECT")
	}
	if err.Error() != parseRejectFailOpenAndFailurePolicyBothSet {
		t.Fatalf("buildCompiledConfig err = %q; want %q (arm B)", err.Error(), parseRejectFailOpenAndFailurePolicyBothSet)
	}
}

// TestParse_EnvVarsCollisionReject verifies the NEW arm A: a key present in
// BOTH host_env_keys AND key_values rejects with the %q-formatted arm-A wording.
func TestParse_EnvVarsCollisionReject(t *testing.T) {
	ev := &wasmcommonv3.EnvironmentVariables{
		HostEnvKeys: []string{"DUPED_KEY"},
		KeyValues:   map[string]string{"DUPED_KEY": "v"},
	}
	_, err := parseEnvVars(ev)
	if err == nil {
		t.Fatal("parseEnvVars(collision) = nil; want arm-A PARSE-REJECT")
	}
	want := fmt.Sprintf(parseRejectEnvVarsKeyCollision, "DUPED_KEY")
	if err.Error() != want {
		t.Fatalf("err = %q; want %q (arm A)", err.Error(), want)
	}

	// End-to-end through buildCompiledConfig.
	m := validWasmConfig()
	m.Config.GetVmConfig().EnvironmentVariables = ev
	_, err = buildCompiledConfig(context.Background(), toAny(t, m), envoyhttp.FactoryCtx{})
	if err == nil || err.Error() != want {
		t.Fatalf("buildCompiledConfig env-collision err = %v; want %q (arm A)", err, want)
	}
}

// TestParse_EnvVarsCapExceededReject verifies the NEW arm C: an env block
// exceeding the envoy-go-strict entry cap (65 > 64) rejects with arm-C wording.
func TestParse_EnvVarsCapExceededReject(t *testing.T) {
	kv := make(map[string]string, 65)
	for i := 0; i < 65; i++ {
		kv[fmt.Sprintf("K%d", i)] = "v"
	}
	ev := &wasmcommonv3.EnvironmentVariables{KeyValues: kv}
	_, err := parseEnvVars(ev)
	if err == nil {
		t.Fatal("parseEnvVars(cap exceeded) = nil; want arm-C PARSE-REJECT")
	}
	if err.Error() != parseRejectEnvVarsCapExceeded {
		t.Fatalf("err = %q; want %q (arm C)", err.Error(), parseRejectEnvVarsCapExceeded)
	}
}

// TestParse_EnvVarsValid_Consumed verifies a well-formed env block assembles
// successfully (the consumed/happy path of arm 13 LIFTED).
func TestParse_EnvVarsValid_Consumed(t *testing.T) {
	ev := &wasmcommonv3.EnvironmentVariables{
		KeyValues: map[string]string{"A": "1", "B": "2"},
	}
	got, err := parseEnvVars(ev)
	if err != nil {
		t.Fatalf("parseEnvVars(valid): unexpected error: %v", err)
	}
	if got["A"] != "1" || got["B"] != "2" {
		t.Fatalf("assembled env = %v; want A=1 B=2", got)
	}

	// Nil env block → nil map, no error.
	gotNil, err := parseEnvVars(nil)
	if err != nil || gotNil != nil {
		t.Fatalf("parseEnvVars(nil) = (%v, %v); want (nil, nil)", gotNil, err)
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

// -----------------------------------------------------------------------------
// 25.2 NEW arms 19-23 — envoy-go-strict-only PluginConfig cap-field validators
// per 25.2 SPEC §6.2 + D-25.2-P5 closure at Task 14.
// -----------------------------------------------------------------------------

// envoyGoStrictPluginConfig is a builder helper: wraps the supplied
// envoy_go_strict sub-Struct into a PluginConfig.configuration Any.
// Returns the *anypb.Any ready to assign to Config.Configuration.
func envoyGoStrictPluginConfig(t *testing.T, envoyGoStrictFields map[string]interface{}) *anypb.Any {
	t.Helper()
	strictStruct, err := structpb.NewStruct(envoyGoStrictFields)
	if err != nil {
		t.Fatalf("structpb.NewStruct(envoyGoStrictFields): %v", err)
	}
	topStruct, err := structpb.NewStruct(map[string]interface{}{
		"envoy_go_strict": strictStruct.AsMap(),
	})
	if err != nil {
		t.Fatalf("structpb.NewStruct(top): %v", err)
	}
	any, err := anypb.New(topStruct)
	if err != nil {
		t.Fatalf("anypb.New(top): %v", err)
	}
	return any
}

// wasmConfigWithEnvoyGoStrict returns a baseline valid Wasm proto whose
// PluginConfig.configuration carries the supplied envoy_go_strict subfields.
// The configuration arm-19/20/21/22/23 tests use this to trigger ONLY the
// targeted cap-field arm; the pre-arm validators (1-15) all pass.
func wasmConfigWithEnvoyGoStrict(t *testing.T, envoyGoStrictFields map[string]interface{}) *wasmv3.Wasm {
	t.Helper()
	m := validWasmConfig()
	m.Config.Configuration = envoyGoStrictPluginConfig(t, envoyGoStrictFields)
	return m
}

// TestBuildCompiledConfig_EnvoyGoStrictArms covers the 5 envoy-go-strict-only
// cap-field PARSE-REJECT arms (19, 20, 21, 22, 23). Each subtest constructs
// an input PluginConfig that triggers ONLY the targeted arm; the test
// asserts the byte-stable error wording matches the constant verbatim.
//
// The configurations all PASS arms 1-15 (the wasmConfigWithEnvoyGoStrict
// baseline + non-wasm InlineString "some-non-wasm-bytes-stub" surface arm
// 17 if execution gets that far — but the cap-field arms fire FIRST in the
// parse order at buildCompiledConfig).
//
// Arm 26 has its own table below (cross-PluginConfig duplicate-name path
// requires process-wide registry manipulation across subtests; runs
// sequentially via t.Run subtests, NOT t.Parallel()).
func TestBuildCompiledConfig_EnvoyGoStrictArms(t *testing.T) {
	cases := []struct {
		name              string
		fields            map[string]interface{}
		wantErrEq         string
		wantErrEqFmtArg   uint32 // when set, the error is formatted via fmt.Sprintf(constant, wantErrEqFmtArg)
		wantErrEqFmtConst string // %d constant; paired with wantErrEqFmtArg
	}{
		// ---- Arm 19: body_buffer_cap_bytes = 0 ----
		{
			name: "Arm19_BodyBufferCapBytes_Zero",
			fields: map[string]interface{}{
				"body_buffer_cap_bytes": float64(0),
			},
			wantErrEq: parseRejectEnvoyGoStrictBodyBufferCapBytesZero,
		},
		// ---- Arm 20: shared_data_value_cap_bytes = 0 ----
		{
			name: "Arm20_SharedDataValueCapBytes_Zero",
			fields: map[string]interface{}{
				"shared_data_value_cap_bytes": float64(0),
			},
			wantErrEq: parseRejectEnvoyGoStrictSharedDataValueCapBytesZero,
		},
		// ---- Arm 21: shared_data_max_entries = 0 ----
		{
			name: "Arm21_SharedDataMaxEntries_Zero",
			fields: map[string]interface{}{
				"shared_data_max_entries": float64(0),
			},
			wantErrEq: parseRejectEnvoyGoStrictSharedDataMaxEntriesZero,
		},
		// ---- Arm 22: dynamic_stats_max_entries = 0 ----
		{
			name: "Arm22_DynamicStatsMaxEntries_Zero",
			fields: map[string]interface{}{
				"dynamic_stats_max_entries": float64(0),
			},
			wantErrEq: parseRejectEnvoyGoStrictDynamicStatsMaxEntriesZero,
		},
		// ---- Arm 23: body_buffer_cap_bytes > 1 GiB ----
		// 2 GiB = 2147483648 > 1<<30 = 1073741824 ⇒ trips arm 23 with the
		// offending value %d-formatted into the byte-stable wording.
		{
			name: "Arm23_BodyBufferCapBytes_Overlarge_2GiB",
			fields: map[string]interface{}{
				"body_buffer_cap_bytes": float64(2 * 1024 * 1024 * 1024),
			},
			wantErrEqFmtArg:   2 * 1024 * 1024 * 1024,
			wantErrEqFmtConst: parseRejectEnvoyGoStrictBodyBufferCapBytesOverlarge,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Use a unique plugin name per subtest so cross-PluginConfig
			// duplicate-name arm 26 doesn't fire accidentally on retries.
			resetPluginConfigNameRegistry()
			m := wasmConfigWithEnvoyGoStrict(t, tc.fields)
			m.Config.Name = "TestBuildCompiledConfig_EnvoyGoStrictArms_" + tc.name
			_, err := buildCompiledConfig(context.Background(), toAny(t, m), envoyhttp.FactoryCtx{})
			if err == nil {
				t.Fatalf("buildCompiledConfig returned nil error; want PARSE-REJECT %q", tc.wantErrEq)
			}
			var want string
			if tc.wantErrEqFmtConst != "" {
				want = fmt.Sprintf(tc.wantErrEqFmtConst, tc.wantErrEqFmtArg)
			} else {
				want = tc.wantErrEq
			}
			if err.Error() != want {
				t.Fatalf("err.Error() = %q; want %q", err.Error(), want)
			}
		})
	}
}

// TestBuildCompiledConfig_EnvoyGoStrictArm23_AtCeiling verifies the boundary
// condition for arm 23: bodyBufferCapBytes == bodyBufferCapBytesCeiling
// (1<<30) is ACCEPTED (not rejected); the arm fires only on STRICTLY-GREATER
// values. The test flows through to arm 17 (compile-failed) because the
// validWasmConfig() InlineString is not real wasm bytecode — that's the
// expected downstream surface for valid cap-field inputs at Task 14.
func TestBuildCompiledConfig_EnvoyGoStrictArm23_AtCeiling(t *testing.T) {
	resetPluginConfigNameRegistry()
	m := wasmConfigWithEnvoyGoStrict(t, map[string]interface{}{
		"body_buffer_cap_bytes": float64(1 << 30), // exactly 1 GiB; AT boundary
	})
	m.Config.Name = "TestBuildCompiledConfig_EnvoyGoStrictArm23_AtCeiling"
	_, err := buildCompiledConfig(context.Background(), toAny(t, m), envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("buildCompiledConfig returned nil error; want arm-17 compile-failed wrap (non-wasm InlineString)")
	}
	// Cap-field validators passed; downstream surface is arm 17 (compile-
	// failed wrap on the synthetic non-wasm bytes).
	const wantPrefix = "wasm: config.vm_config.code: compile: "
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err.Error() = %q; want prefix %q (arm-17 compile-failed; cap-field validators passed at the 1 GiB boundary)", err.Error(), wantPrefix)
	}
}

// TestBuildCompiledConfig_EnvoyGoStrictDefaults_FallThrough verifies that
// when PluginConfig.configuration is UNSET, all 4 cap fields take their
// defaults + the parse pipeline flows through to the resolveDataSource /
// CompileModule pipeline (which surfaces arm 17 on the synthetic non-wasm
// InlineString). This is the "PluginConfig.configuration = nil" path at
// parseEnvoyGoStrictFields step 1.
func TestBuildCompiledConfig_EnvoyGoStrictDefaults_FallThrough(t *testing.T) {
	resetPluginConfigNameRegistry()
	m := validWasmConfig()
	m.Config.Name = "TestBuildCompiledConfig_EnvoyGoStrictDefaults_FallThrough"
	// Explicitly leave Configuration nil.
	_, err := buildCompiledConfig(context.Background(), toAny(t, m), envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("buildCompiledConfig returned nil error; want arm-17 compile-failed wrap (non-wasm InlineString)")
	}
	const wantPrefix = "wasm: config.vm_config.code: compile: "
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err.Error() = %q; want prefix %q (defaults applied + flow through to arm 17)", err.Error(), wantPrefix)
	}
}

// TestBuildCompiledConfig_EnvoyGoStrictPluginConfig_NonStructTypeURL covers
// the parseEnvoyGoStrictFields step 2 path: PluginConfig.configuration is
// set but its Any TypeURL is NOT google.protobuf.Struct. The envoy_go_strict
// block is silently ignored; all 4 cap fields take their defaults; parse
// flows through to arm 17. This is the operator-flexibility carve-out where
// the configuration Any carries a non-Struct guest-only payload.
func TestBuildCompiledConfig_EnvoyGoStrictPluginConfig_NonStructTypeURL(t *testing.T) {
	resetPluginConfigNameRegistry()
	m := validWasmConfig()
	m.Config.Name = "TestBuildCompiledConfig_EnvoyGoStrictPluginConfig_NonStructTypeURL"
	// PluginConfig.configuration carries a guest-only payload — wrap an
	// arbitrary non-Struct proto into the Any (a Duration here works as a
	// stand-in for any guest-side proto envelope).
	guestPayload := &wasmcommonv3.PluginConfig{Name: "guest-side-config"}
	any, err := anypb.New(guestPayload)
	if err != nil {
		t.Fatalf("anypb.New(guestPayload): %v", err)
	}
	m.Config.Configuration = any
	_, err = buildCompiledConfig(context.Background(), toAny(t, m), envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("buildCompiledConfig returned nil error; want arm-17 compile-failed wrap (defaults applied; non-wasm InlineString)")
	}
	const wantPrefix = "wasm: config.vm_config.code: compile: "
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err.Error() = %q; want prefix %q (non-Struct Any silently ignored; defaults applied + arm-17 flow-through)", err.Error(), wantPrefix)
	}
}

// TestBuildCompiledConfig_EnvoyGoStrict_PartialFields verifies that only
// EXPLICITLY-SET cap fields take the supplied value; missing keys take
// defaults. The test sets only body_buffer_cap_bytes (to a valid 32 MiB)
// + leaves the other 3 keys unset; all 4 caps should validate (no PARSE-
// REJECT) + flow through to arm 17.
func TestBuildCompiledConfig_EnvoyGoStrict_PartialFields(t *testing.T) {
	resetPluginConfigNameRegistry()
	m := wasmConfigWithEnvoyGoStrict(t, map[string]interface{}{
		"body_buffer_cap_bytes": float64(32 * 1024 * 1024),
		// shared_data_value_cap_bytes, shared_data_max_entries,
		// dynamic_stats_max_entries deliberately unset.
	})
	m.Config.Name = "TestBuildCompiledConfig_EnvoyGoStrict_PartialFields"
	_, err := buildCompiledConfig(context.Background(), toAny(t, m), envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("buildCompiledConfig returned nil error; want arm-17 compile-failed wrap (partial fields valid; defaults applied for missing keys)")
	}
	const wantPrefix = "wasm: config.vm_config.code: compile: "
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err.Error() = %q; want prefix %q (partial-fields valid; defaults applied + arm-17 flow-through)", err.Error(), wantPrefix)
	}
}

// -----------------------------------------------------------------------------
// 25.2 NEW arm 26 — cross-PluginConfig duplicate PluginConfig.name per
// 25.2 SPEC §6.2 + D-25.2-P5 closure at Task 14.
// -----------------------------------------------------------------------------

// TestBuildCompiledConfig_Arm26_DuplicatePluginConfigName verifies the
// cross-PluginConfig duplicate-name registry consult at arm 26. Two
// buildCompiledConfig invocations with the same non-empty PluginConfig.name
// fire arm 26 on the SECOND invocation (the first claims the name; the
// second sees the duplicate).
func TestBuildCompiledConfig_Arm26_DuplicatePluginConfigName(t *testing.T) {
	resetPluginConfigNameRegistry()
	const name = "TestBuildCompiledConfig_Arm26_DuplicatePluginConfigName_dup"

	// First invocation: claims the name. Flows through cap-fields + name
	// registry (success); fails at arm 17 (compile-failed on non-wasm bytes).
	m1 := validWasmConfig()
	m1.Config.Name = name
	if _, err := buildCompiledConfig(context.Background(), toAny(t, m1), envoyhttp.FactoryCtx{}); err == nil {
		t.Fatal("first invocation: want arm-17 compile-failed wrap; got nil error")
	} else if !strings.HasPrefix(err.Error(), "wasm: config.vm_config.code: compile: ") {
		t.Fatalf("first invocation: err.Error() = %q; want arm-17 compile-failed prefix", err.Error())
	}
	// NOTE: at arm 17 failure path, the rollback unregisters the name; this
	// is intentional — the operator can retry the same config after fixing
	// a wasm-bytes problem. So we need a SECOND, separate scenario to verify
	// the duplicate-detection: simulate by re-registering the name directly.

	resetPluginConfigNameRegistry()
	// Pre-claim the name via the package-private registerPluginConfigName
	// helper; this simulates a previously-successful buildCompiledConfig.
	if err := registerPluginConfigName(name); err != nil {
		t.Fatalf("pre-claim registerPluginConfigName(%q): %v; want nil (fresh registry)", name, err)
	}
	// Second invocation with the same name fires arm 26 BEFORE the
	// resolveDataSource path (per buildCompiledConfig's documented arm
	// ordering: arm 26 fires AFTER cap-validators succeed + BEFORE
	// resolveDataSource).
	m2 := validWasmConfig()
	m2.Config.Name = name
	_, err := buildCompiledConfig(context.Background(), toAny(t, m2), envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatalf("second invocation: want arm-26 PARSE-REJECT; got nil error")
	}
	want := fmt.Sprintf(parseRejectCrossPluginConfigDuplicatePluginConfigName, name)
	if err.Error() != want {
		t.Fatalf("second invocation: err.Error() = %q; want %q", err.Error(), want)
	}
}

// TestBuildCompiledConfig_Arm26_EmptyName_SkipsRegistry verifies the empty-
// name carve-out at registerPluginConfigName: empty PluginConfig.name does
// NOT claim the registry + does NOT fire arm 26 even when called twice.
func TestBuildCompiledConfig_Arm26_EmptyName_SkipsRegistry(t *testing.T) {
	resetPluginConfigNameRegistry()
	// Call registerPluginConfigName twice with empty name — both must succeed.
	if err := registerPluginConfigName(""); err != nil {
		t.Fatalf("registerPluginConfigName(\"\") first call: %v; want nil (empty-name carve-out)", err)
	}
	if err := registerPluginConfigName(""); err != nil {
		t.Fatalf("registerPluginConfigName(\"\") second call: %v; want nil (empty-name carve-out)", err)
	}
}

// TestUnregisterPluginConfigName_RollbackPath verifies the rollback helper
// used on the construction-failure path (arm 16/17 OR wasm.NewRootVM
// failure). After unregister, a re-register with the SAME name must
// succeed.
func TestUnregisterPluginConfigName_RollbackPath(t *testing.T) {
	resetPluginConfigNameRegistry()
	const name = "TestUnregisterPluginConfigName_RollbackPath"
	if err := registerPluginConfigName(name); err != nil {
		t.Fatalf("registerPluginConfigName(%q) first: %v; want nil", name, err)
	}
	// Second register WITHOUT unregister fires arm 26.
	if err := registerPluginConfigName(name); err == nil {
		t.Fatalf("registerPluginConfigName(%q) second WITHOUT unregister: nil; want arm-26 error", name)
	}
	// Rollback via unregister + re-register: must succeed.
	unregisterPluginConfigName(name)
	if err := registerPluginConfigName(name); err != nil {
		t.Fatalf("registerPluginConfigName(%q) post-unregister: %v; want nil (rollback complete)", name, err)
	}
}

// TestUnregisterPluginConfigName_EmptyName_NoOp verifies the empty-name
// carve-out on the unregister path is also a no-op (mirrors register).
func TestUnregisterPluginConfigName_EmptyName_NoOp(t *testing.T) {
	resetPluginConfigNameRegistry()
	unregisterPluginConfigName("") // must not panic
}
