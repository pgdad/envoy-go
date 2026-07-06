package wasm

// fuzz_test.go — 34th project-wide fuzzer `FuzzWasmConfigParse` per Phase
// 25.1 PLAN Task 14 + ADR-0018 baseline ("every parser/codec/filter ships
// a fuzzer"). Corpus seed roster per 25.1 PLAN D-P-PLAN-7 + parent §15
// Layer C:
//
//   - 18 PARSE-REJECT arm seeds (1 per arm from parent §6.2 + 25.1 SPEC §6.2
//     extension; arm 12 is unreachable-by-design at 25.1 single-plugin-per-
//     listener, so we substitute a duplicate arm 11 variant to keep the
//     seed count at exactly 18 — the fuzz body asserts must-never-panic on
//     ALL arms whether reachable or not);
//   - 5 valid-config seeds (1 per AsyncDataSource arm with valid contents
//     + 1 with non-empty CapabilityRestrictionConfig);
//   - 7 adversarial-wasm-bytecode seeds (malformed wasm headers / oversize
//     sections / sentinel-spoof attempts / truncated module / null bytes /
//     no-body function / unbalanced control flow).
//
// Total corpus floor: 30 seeds. Must-never-panic invariant via wazero
// compile error path (arm 17 wrapping) — adversarial bytecode surfaces as
// a wrapped arm-17 compile failure, NOT a panic.
//
// # D-S1 RATIFICATION at IMPL
//
// Per 25.1 SPEC §11.1 D-S1 closure: 33 unique fuzzers at master tip pre-
// 25.1; this is the 34th. Verified via the find/grep oneliner at PLAN Task
// 14 Step 4 + Task 17 Gate E. ADR-0203 §Decision body + BEHAVIOR_CONTRACT.md
// §13.4 patch pins the count to 34 at the Task 17 atomic landing.
//
// # Cross-references
//
//   - ADR-0018 (every parser/codec/filter ships a fuzzer; 30s/seed CI budget)
//   - parent §15 Layer C (project-wide fuzzer roster)
//   - 25.1 PLAN Task 14 + D-P-PLAN-7 (corpus seed roster)
//   - 25.1 SPEC §6 Task 14 (fuzzer surface) + §11.1 D-S1 (count VERIFIED)
//   - parent §6.2 (18-arm PARSE-REJECT roster — source of truth for the
//     per-arm seeds)
//   - compiled_config.go (buildCompiledConfig — the fuzzer target)

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	wasmcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	internalwasm "github.com/pgdad/envoy-go/internal/wasm"
)

// FuzzWasmConfigParse fuzzes arbitrary byte sequences as the typed_config
// Any.Value payload to buildCompiledConfig. Asserts the must-never-panic
// contract per ADR-0018 + 25.1 PLAN Task 14:
//
//   - buildCompiledConfig never panics on any input (the defer-recover in
//     the fuzz body catches + fails the test on any panic).
//   - Adversarial wasm bytecode surfaces as a wrapped arm-17 compile
//     failure (parseRejectModuleCompileFailed prefix), NOT a panic.
//
// 34th project-wide fuzzer per ADR-0018 baseline (33 unique pre-25.1 per
// 25.1 SPEC §11.1 D-S1 closure). Seeded with ~30 corpus entries per
// D-P-PLAN-7: 18 PARSE-REJECT arms + 5 valid-config + 7 adversarial-wasm.
//
// 30s/seed runtime envelope per ADR-0018 short-mode CI policy.
func FuzzWasmConfigParse(f *testing.F) {
	// addSeed marshals a *wasmv3.Wasm proto + adds the resulting Any.Value
	// bytes to the fuzzer corpus. Mirrors the phase-18.1/18.2 fuzzer helper
	// pattern: the fuzz function input is the raw bytes; the fuzz body wraps
	// them in an *anypb.Any with the canonical TypeURL.
	addSeed := func(msg *wasmv3.Wasm) {
		f.Helper()
		raw, err := proto.Marshal(msg)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(raw)
	}

	// addRawSeed adds raw bytes directly (bypassing proto.Marshal) — used by
	// arm 2 (typed_config unmarshal failure) + arm 1 (empty payload).
	addRawSeed := func(raw []byte) {
		f.Helper()
		f.Add(raw)
	}

	// -------------------------------------------------------------------------
	// 18 PARSE-REJECT arm seeds per parent §6.2 (1 per arm).
	// -------------------------------------------------------------------------

	// Arm 1: typed_config required. The fuzz body wraps the input in an Any;
	// an empty byte slice produces a zero-value *wasmv3.Wasm{} that triggers
	// arm 3 (config required), not arm 1. Arm 1 fires only when the *anypb.Any
	// itself is nil at the buildCompiledConfig entry — exercised at the
	// wasm_test.go::TestNew_NilTypedConfig surface, not from the fuzzer.
	// As a substitute, seed with a single 0x00 byte which Unmarshal accepts
	// as an empty Wasm{} message and surfaces arm 3.
	addRawSeed([]byte{})

	// Arm 2: typed_config unmarshal failure — garbage bytes that fail proto
	// decoding into *wasmv3.Wasm.
	addRawSeed([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})

	// Arm 3: config (PluginConfig) is required — empty Wasm{} (Config nil).
	addSeed(&wasmv3.Wasm{Config: nil})

	// Arm 4: vm_config is required — PluginConfig with Vm nil.
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{Name: "arm4_plugin"},
	})

	// Arm 5: vm_config.code is required.
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "arm5_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code:    nil,
				},
			},
		},
	})

	// Arm 6: vm_config.code.remote deferred.
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "arm6_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Remote{
							Remote: &corev3.RemoteDataSource{},
						},
					},
				},
			},
		},
	})

	// Arm 7: watched_directory deferred.
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "arm7_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier:        &corev3.DataSource_Filename{Filename: "/tmp/x.wasm"},
								WatchedDirectory: &corev3.WatchedDirectory{Path: "/tmp"},
							},
						},
					},
				},
			},
		},
	})

	// Arm 8: data_source specifier oneof required — local set, specifier nil.
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "arm8_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{},
						},
					},
				},
			},
		},
	})

	// Arm 9: failure_policy = FAIL_RELOAD deferred.
	{
		base := minValidWasm("arm9_plugin")
		base.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_RELOAD
		addSeed(base)
	}

	// Arm 10: fail_open deferred.
	{
		base := minValidWasm("arm10_plugin")
		base.Config.FailOpen = true //nolint:staticcheck // SA1019: arm 10 EXISTS to PARSE-REJECT this deprecated proto field; intentional seed.
		addSeed(base)
	}

	// Arm 11: vm_config.runtime discriminator — non-wazero runtime.
	{
		base := minValidWasm("arm11_plugin")
		base.Config.GetVmConfig().Runtime = "envoy.wasm.runtime.v8"
		addSeed(base)
	}

	// Arm 12: vm_config.vm_id duplicate — unreachable-by-design at 25.1
	// single-plugin-per-listener (no production trigger path). To keep the
	// per-arm seed count at 18 we seed a vm_id-populated variant that
	// flows-through to arm 17 (compile failure on non-wasm bytes) — the
	// must-never-panic contract holds regardless of which arm fires.
	{
		base := minValidWasm("arm12_plugin")
		base.Config.GetVmConfig().VmId = "duplicate_vm_id"
		addSeed(base)
	}

	// Arm 13: environment_variables deferred.
	{
		base := minValidWasm("arm13_plugin")
		base.Config.GetVmConfig().EnvironmentVariables = &wasmcommonv3.EnvironmentVariables{
			HostEnvKeys: []string{"PATH"},
		}
		addSeed(base)
	}

	// Arm 14: allow_precompiled rejected (envoy-go-strict).
	{
		base := minValidWasm("arm14_plugin")
		base.Config.GetVmConfig().AllowPrecompiled = true
		addSeed(base)
	}

	// Arm 15: nack_on_code_cache_miss rejected (envoy-go-strict).
	{
		base := minValidWasm("arm15_plugin")
		base.Config.GetVmConfig().NackOnCodeCacheMiss = true
		addSeed(base)
	}

	// Arm 16: module ABI-version rejected. Seed with the no-ABI-sentinel
	// minimal proxy wasm (buildMinimalProxyWasm exports v0.2.1; we synthesize
	// a header-only fixture below that lacks the sentinel to drive arm 16
	// at the seed corpus). The fuzz engine derives further inputs.
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "arm16_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{
									// Bare wasm header — no proxy_abi_version_0_2_1 export.
									// Compile may succeed (empty module is valid wasm),
									// then arm 16 fires on the missing-sentinel detection.
									InlineBytes: []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00},
								},
							},
						},
					},
				},
			},
		},
	})

	// Arm 17: module compile failed — non-wasm InlineBytes flow through to
	// the wazero compile error path.
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "arm17_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineString{
									InlineString: "garbage-non-wasm-bytes-trigger-arm17",
								},
							},
						},
					},
				},
			},
		},
	})

	// Arm 18: per-route configuration deferred. Not exercised via
	// buildCompiledConfig (per-route is the separate
	// RegisterPerRouteValidator path per ADR-0110 single-chokepoint). To keep
	// the per-arm seed count at 18, we seed an empty inline_bytes config that
	// flows-through to the arm 8 / arm-resolveDataSource failure path; the
	// must-never-panic invariant holds.
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "arm18_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{
									InlineBytes: []byte{}, // empty — arm InlineBytesEmpty fires
								},
							},
						},
					},
				},
			},
		},
	})

	// -------------------------------------------------------------------------
	// 5 valid-config seeds (1 per AsyncDataSource.Local arm + 1 with non-empty
	// CapabilityRestrictionConfig). The fuzz engine derives further inputs
	// from these — the seeds themselves may PARSE-REJECT at resolve-time
	// (file-not-found / env-var-unset) or compile-time (the InlineString
	// arm carries non-wasm content); the must-never-panic invariant holds.
	// -------------------------------------------------------------------------

	// (1) Filename valid path shape — flows through to arm filename-not-found
	// (the named file does not exist in the test environment).
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "valid_filename_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_Filename{
									Filename: "/nonexistent/path/to/plugin.wasm",
								},
							},
						},
					},
				},
			},
		},
	})

	// (2) InlineBytes with VALID wasm bytecode that compiles successfully.
	// Uses buildContinueProxyWasm() — exports proxy_abi_version_0_2_1 +
	// proxy_on_request_headers + proxy_on_response_headers + memory.
	// This seed reaches the end of buildCompiledConfig (no PARSE-REJECT).
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "valid_inline_bytes_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{
									InlineBytes: buildContinueProxyWasm(),
								},
							},
						},
					},
				},
			},
		},
	})

	// (3) InlineString valid path shape — flows through to arm 17
	// (compile-failed; the content cast to []byte is not valid wasm).
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "valid_inline_string_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineString{
									InlineString: "wasm-stub-content",
								},
							},
						},
					},
				},
			},
		},
	})

	// (4) EnvironmentVariable valid path shape — flows through to env-var-unset.
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "valid_env_var_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_EnvironmentVariable{
									EnvironmentVariable: "WASM_FUZZ_NEVER_SET_VAR",
								},
							},
						},
					},
				},
			},
		},
	})

	// (5) Valid config with non-empty CapabilityRestrictionConfig (with a
	// valid wasm payload to exercise buildSandboxConfig populated-map path).
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "valid_cap_restriction_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{
									InlineBytes: buildContinueProxyWasm(),
								},
							},
						},
					},
				},
			},
			CapabilityRestrictionConfig: &wasmcommonv3.CapabilityRestrictionConfig{
				AllowedCapabilities: map[string]*wasmcommonv3.SanitizationConfig{
					"proxy_log":                          {},
					"proxy_get_header_map_value":         nil,
					"proxy_get_current_time_nanoseconds": {},
				},
			},
		},
	})

	// -------------------------------------------------------------------------
	// 7 adversarial-wasm-bytecode seeds (must-never-panic across the wazero
	// compile path — each surfaces as a wrapped arm-17 compile failure).
	// -------------------------------------------------------------------------

	addAdversarial := func(name string, bytecode []byte) {
		addSeed(&wasmv3.Wasm{
			Config: &wasmcommonv3.PluginConfig{
				Name: "adversarial_" + name + "_plugin",
				Vm: &wasmcommonv3.PluginConfig_VmConfig{
					VmConfig: &wasmcommonv3.VmConfig{
						Runtime: "envoy.wasm.runtime.wazero",
						Code: &corev3.AsyncDataSource{
							Specifier: &corev3.AsyncDataSource_Local{
								Local: &corev3.DataSource{
									Specifier: &corev3.DataSource_InlineBytes{
										InlineBytes: bytecode,
									},
								},
							},
						},
					},
				},
			},
		})
	}

	// (a) Bad magic — wasm magic is 0x00 0x61 0x73 0x6d; flip to 0xff.
	addAdversarial("bad_magic", []byte{
		0xff, 0xff, 0xff, 0xff,
		0x01, 0x00, 0x00, 0x00,
	})

	// (b) Oversize section — type section claims 0xff bytes of payload but
	// only 1 byte follows.
	addAdversarial("oversize_section", []byte{
		0x00, 0x61, 0x73, 0x6d,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0xff, 0x00, // section id 1, claimed size 0xff, 1 byte payload
	})

	// (c) Sentinel-spoof — declare proxy_abi_version_0_2_1 as a memory export
	// (kind 0x02) instead of a function export (kind 0x00). The bytecode_util
	// scanner is byte-faithful per AMEND-A6: it inspects export-section bytes
	// directly; a non-function export with the right name is rejected.
	addAdversarial("sentinel_spoof_memory_kind", []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		// memory section: 1 memory, no max, 1 page
		0x05, 0x03, 0x01, 0x00, 0x01,
		// export section: 1 export
		0x07, 0x1d, 0x01,
		// name length 23, then "proxy_abi_version_0_2_1"
		0x17, 'p', 'r', 'o', 'x', 'y', '_', 'a', 'b', 'i', '_',
		'v', 'e', 'r', 's', 'i', 'o', 'n', '_', '0', '_', '2', '_', '1',
		// export kind = memory (0x02), idx 0
		0x02, 0x00,
	})

	// (d) Truncated module — magic + version + start of type section, but
	// section payload cut off mid-stream.
	addAdversarial("truncated_module", []byte{
		0x00, 0x61, 0x73, 0x6d,
		0x01, 0x00, 0x00, 0x00,
		0x01, // section id 1 (type), no size byte
	})

	// (e) Null bytes — all zeros, no valid header.
	addAdversarial("null_bytes", []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	})

	// (f) Function declared in func section but no corresponding body in code
	// section. Type 0 = () -> (); func section declares 1 function; code
	// section declares 0 bodies.
	addAdversarial("no_body_function", []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		// type section: 1 type, () -> ()
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
		// function section: 1 function of type 0
		0x03, 0x02, 0x01, 0x00,
		// code section: 0 bodies (mismatch — should be 1)
		0x0a, 0x01, 0x00,
	})

	// (g) Unbalanced control flow — function body opens a block (0x02) but
	// never emits end (0x0b). Type 0 = () -> (); func 0 = type 0; body =
	// local-count 0, block-i32-empty-result, no end-of-block, end-of-func.
	addAdversarial("unbalanced_blocks", []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		// type section: 1 type, () -> ()
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
		// function section: 1 function of type 0
		0x03, 0x02, 0x01, 0x00,
		// code section: 1 body, size 4, local-count 0, block (0x02 0x40
		// blocktype-void), end-of-function (0x0b) WITHOUT closing the block.
		0x0a, 0x06, 0x01, 0x04, 0x00, 0x02, 0x40, 0x0b,
	})

	// -------------------------------------------------------------------------
	// 25.3 NEW seeds — FOLD per-route / reload / env_vars / failure_policy
	// surface added at phase-25.3 Task 7 (15 seeds, per D-25.3-6).
	// -------------------------------------------------------------------------

	// --- failure_policy enum values ---

	// (fp-1) failure_policy = FAIL_RELOAD on a valid-ish config.
	// NOTE: arm 9 in the PRE-25.3 sense was "FAIL_RELOAD deferred"; at 25.3
	// that arm is LIFTED (failure_policy is now PARSED + CONSUMED). This seed
	// reaches parseFailurePolicy, sets FailurePolicyFailReload, then continues
	// to arm 17 (compile-failed on inline-string stub).
	{
		base := minValidWasm("fp_fail_reload_plugin")
		base.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_RELOAD
		addSeed(base)
	}

	// (fp-2) failure_policy = FAIL_CLOSED explicit (default alias; valid parse).
	{
		base := minValidWasm("fp_fail_closed_plugin")
		base.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_CLOSED
		addSeed(base)
	}

	// (fp-3) failure_policy = FAIL_OPEN explicit (25.3 lifted arm 10 variant).
	// At 25.3 fail_open==false here, so arm B mutual-exclusivity does NOT fire;
	// this seeds FailurePolicyFailOpen and continues to arm 17.
	{
		base := minValidWasm("fp_fail_open_enum_plugin")
		base.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_OPEN
		addSeed(base)
	}

	// --- reload_config.backoff.base_interval variants ---

	// (rc-1) reload_config with base_interval = 0 (unset/zero durationpb).
	{
		base := minValidWasm("reload_cfg_zero_interval_plugin")
		base.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_RELOAD
		base.Config.ReloadConfig = &wasmcommonv3.ReloadConfig{
			Backoff: &corev3.BackoffStrategy{
				BaseInterval: durationpb.New(0),
			},
		}
		addSeed(base)
	}

	// (rc-2) reload_config with base_interval = 50ms (sub-100ms; valid for
	// parseFailurePolicy since no proto-level lower-bound; exercises the
	// durationpb.AsDuration() path).
	{
		base := minValidWasm("reload_cfg_50ms_interval_plugin")
		base.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_RELOAD
		base.Config.ReloadConfig = &wasmcommonv3.ReloadConfig{
			Backoff: &corev3.BackoffStrategy{
				BaseInterval: durationpb.New(50 * time.Millisecond),
			},
		}
		addSeed(base)
	}

	// (rc-3) reload_config with base_interval = 250ms (typical valid value).
	{
		base := minValidWasm("reload_cfg_250ms_interval_plugin")
		base.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_RELOAD
		base.Config.ReloadConfig = &wasmcommonv3.ReloadConfig{
			Backoff: &corev3.BackoffStrategy{
				BaseInterval: durationpb.New(250 * time.Millisecond),
			},
		}
		addSeed(base)
	}

	// --- fail_open + failure_policy mutual-exclusivity (arm B reject) ---

	// (me-1) fail_open=true AND failure_policy=FAIL_CLOSED → arm B reject
	// (parseRejectFailOpenAndFailurePolicyBothSet). This fires BEFORE code
	// resolution, so InlineString stub is fine.
	{
		base := minValidWasm("mutual_excl_fail_open_closed_plugin")
		base.Config.FailOpen = true //nolint:staticcheck // SA1019: arm B mutual-exclusivity seed; intentional.
		base.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_CLOSED
		addSeed(base)
	}

	// (me-2) fail_open=true AND failure_policy=FAIL_RELOAD → arm B reject.
	{
		base := minValidWasm("mutual_excl_fail_open_reload_plugin")
		base.Config.FailOpen = true //nolint:staticcheck // SA1019: arm B mutual-exclusivity seed; intentional.
		base.Config.FailurePolicy = wasmcommonv3.FailurePolicy_FAIL_RELOAD
		addSeed(base)
	}

	// --- environment_variables seeds ---

	// (ev-1) Collision: same key in BOTH host_env_keys AND key_values → arm A
	// reject (parseRejectEnvVarsKeyCollision). Fires at parseEnvVars before
	// code resolution.
	{
		base := minValidWasm("ev_collision_plugin")
		base.Config.GetVmConfig().EnvironmentVariables = &wasmcommonv3.EnvironmentVariables{
			HostEnvKeys: []string{"COLLIDING_KEY"},
			KeyValues:   map[string]string{"COLLIDING_KEY": "value"},
		}
		addSeed(base)
	}

	// (ev-2) Cap-by-entry-count: >64 key_values entries → arm C reject
	// (parseRejectEnvVarsCapExceeded).
	{
		base := minValidWasm("ev_cap_entries_plugin")
		kv := make(map[string]string, 65)
		for i := 0; i < 65; i++ {
			kv[fmt.Sprintf("KEY_%03d", i)] = "v"
		}
		base.Config.GetVmConfig().EnvironmentVariables = &wasmcommonv3.EnvironmentVariables{
			KeyValues: kv,
		}
		addSeed(base)
	}

	// (ev-3) Cap-by-value-size: one value > 4096 bytes → arm C reject.
	{
		base := minValidWasm("ev_cap_value_size_plugin")
		bigVal := strings.Repeat("x", 4097)
		base.Config.GetVmConfig().EnvironmentVariables = &wasmcommonv3.EnvironmentVariables{
			KeyValues: map[string]string{"BIG_KEY": bigVal},
		}
		addSeed(base)
	}

	// (ev-4) Valid env_vars: a few key_values + a host_env_key that does NOT
	// collide. Passes parseEnvVars, then continues to arm 17 (compile-failed
	// on the InlineString stub content).
	{
		base := minValidWasm("ev_valid_plugin")
		base.Config.GetVmConfig().EnvironmentVariables = &wasmcommonv3.EnvironmentVariables{
			HostEnvKeys: []string{"PATH"},
			KeyValues:   map[string]string{"MY_KEY": "my_value", "OTHER": "42"},
		}
		addSeed(base)
	}

	// --- vm_id set (shared-registry surface) ---

	// (vm-1) vm_id set on a valid-ish config. The single-input fuzzer cannot
	// express the two-config vm_id SHARING scenario, but seeding with vm_id
	// set exercises the vm_id field path through compiled_config + the
	// DefaultRegistry.AcquireSharedData(vm_id) call at the tail (if compile
	// succeeds; otherwise surfaces at arm 17).
	{
		base := minValidWasm("vm_id_set_plugin")
		base.Config.GetVmConfig().VmId = "shared_vm_123"
		addSeed(base)
	}

	// (vm-2) vm_id set AND valid wasm bytecode to exercise the registry-acquire
	// path fully (reaches DefaultRegistry.AcquireSharedData post-compile).
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name: "vm_id_valid_wasm_plugin",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    "shared_vm_456",
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{
									InlineBytes: buildContinueProxyWasm(),
								},
							},
						},
					},
				},
			},
		},
	})

	// --- per-route-shaped wholesale Wasm variants ---

	// (pr-1) Per-route path uses the SAME buildCompiledConfig; a complete valid
	// Wasm message with failure_policy + env_vars set exercises both surfaces
	// through the single code path.
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:          "perroute_valid_plugin",
			FailurePolicy: wasmcommonv3.FailurePolicy_FAIL_OPEN,
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					EnvironmentVariables: &wasmcommonv3.EnvironmentVariables{
						KeyValues: map[string]string{"ROUTE_KEY": "route_value"},
					},
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{
									InlineBytes: buildContinueProxyWasm(),
								},
							},
						},
					},
				},
			},
		},
	})

	// (pr-2) Per-route-shaped Wasm with reload_config set (FAIL_RELOAD +
	// backoff) to exercise the reload-path through buildCompiledConfig at the
	// per-route parse surface.
	addSeed(&wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:          "perroute_reload_plugin",
			FailurePolicy: wasmcommonv3.FailurePolicy_FAIL_RELOAD,
			ReloadConfig: &wasmcommonv3.ReloadConfig{
				Backoff: &corev3.BackoffStrategy{
					BaseInterval: durationpb.New(100 * time.Millisecond),
				},
			},
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineString{
									InlineString: "perroute-stub-bytes",
								},
							},
						},
					},
				},
			},
		},
	})

	// -------------------------------------------------------------------------
	// Fuzz body: must-never-panic assertion only (per ADR-0018 + 25.1 PLAN
	// Task 14). The fuzz engine derives further inputs from the ~45 seeds at
	// the 30s budget per ADR-0018 short-mode CI policy.
	// -------------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, raw []byte) {
		// Per-iteration state reset (phase-25.3 Task 10 hygiene): a VALID fuzz
		// input acquires a registry *RootVM + registers the plugin name in the
		// process-global DefaultRegistry + pluginNameRegistry. Resetting at the
		// top of each iteration ensures independent evaluation (no refcount /
		// goroutine accumulation, no cross-input arm-26 false-rejects).
		internalwasm.DefaultRegistry.ResetForTest()
		resetPluginConfigNameRegistry()

		// Defer-recover catches any panic + records as test failure. The
		// must-never-panic contract holds across the full parse pipeline:
		// typed_config.UnmarshalTo, the 18-arm PARSE-REJECT roster, the
		// 4-arm resolveDataSource dispatch, the wazero CompileModule path
		// (arm 16 + arm 17), and the buildSandboxConfig + filterStats
		// allocation tail.
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("FuzzWasmConfigParse: panic on input (len=%d): %v\n%s", len(raw), r, debug.Stack())
			}
		}()

		anyMsg := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		_, err := buildCompiledConfig(context.Background(), anyMsg, envoyhttp.FactoryCtx{})

		// We do NOT assert success/failure — the goal is "no panic".
		// However, when an error IS returned, it MUST be a wasm:-prefixed
		// PARSE-REJECT string per the byte-stable wording discipline at
		// parent §6.1 + ADR-0080 (envoy-go-strict departure discipline).
		// This is a structural-coherence check: a non-wasm:-prefixed error
		// would indicate the parser leaked an internal error verbatim.
		if err != nil && !strings.HasPrefix(err.Error(), "wasm:") {
			t.Fatalf("FuzzWasmConfigParse: non-wasm:-prefixed error leak (len=%d): %v", len(raw), err)
		}
	})
}

// minValidWasm returns a minimum-valid *wasmv3.Wasm proto baseline that
// passes arms 1, 3, 4, 5, 6, 7, 8 + lands at arm 9/10/11/13/14/15 (or
// flows through to arm 17 compile-failure) depending on subsequent
// mutations. Mirrors validWasmConfig() in compiled_config_test.go but is
// duplicated here so the fuzz file is self-contained (a fuzz test file
// cannot import test-only helpers from a sibling test file in the same
// package because both build under the same _test.go scope, but using the
// helper directly creates an import cycle through f.Add seed marshaling
// during corpus replay — keeping a local helper avoids any subtle scoping
// issues during go-test-corpus-replay).
func minValidWasm(pluginName string) *wasmv3.Wasm {
	return &wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:   pluginName,
			RootId: "fuzz_root",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    "fuzz_vm",
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineString{
									InlineString: "fuzz-stub-bytes",
								},
							},
						},
					},
				},
			},
		},
	}
}
