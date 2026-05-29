package wasm

// perroute_test.go — TDD tests for perroute.go per phase-25.3 Task 6.
//
// # Test surface
//
//   - TestParsePerRouteWasm_WholesaleCompile — exercises parsePerRouteWasm:
//     (a) valid *wasmv3.Wasm with real wazero-compilable bytecode → non-nil
//         *compiledConfig, nil error.
//     (b) invalid *wasmv3.Wasm (missing PluginConfig) → the SAME
//         byte-stable PARSE-REJECT wording as buildCompiledConfig arm 3
//         (parseRejectConfigRequired), proving delegation.
//     (c) invalid *wasmv3.Wasm (missing vm_config) → arm 4 wording,
//         proving delegation.
//     (d) wrong-type proto.Message → type-mismatch error.
//
//   - TestResolvePerRoute_PrecedenceOverListener — exercises resolvePerRoute:
//     route!=nil wins over listener; nil route falls back to listener.
//
// # Valid-compile strategy
//
// We use buildContinueProxyWasm() (wasm_fixtures_test.go, same package) to
// get a minimal fully ABI-0.2.1-compliant wazero-compilable module. The
// helper buildWasmProtoInlineBytes (below) wraps it in a *wasmv3.Wasm with a
// unique plugin name to avoid arm-26 cross-test duplicate-name collisions in
// the process-wide plugin-name registry.
//
// For the invalid path we pass a *wasmv3.Wasm with Config=nil (arm 3) or
// with Config.Vm=nil (arm 4) — both fire inside buildCompiledConfig, proving
// the delegation via anypb round-trip.

import (
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	wasmcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TestParsePerRouteWasm_WholesaleCompile exercises the outcomes of
// parsePerRouteWasm: valid-compile, invalid-config-delegation,
// and wrong-type proto.
func TestParsePerRouteWasm_WholesaleCompile(t *testing.T) {
	t.Parallel()

	t.Run("valid_real_bytecode_compiles", func(t *testing.T) {
		t.Parallel()
		// Use a unique plugin name to avoid arm-26 duplicate-name collisions
		// with other tests running concurrently in the same process.
		modBytes := buildContinueProxyWasm()
		reg := stats.NewRegistry()
		// Confirm the bytecode works via newTestCompiledConfig (dispatch_test.go
		// helper) — same bytecode the existing dispatch tests exercise.
		cc := newTestCompiledConfig(t, modBytes, "perroute_test_sanity", reg)
		// cc.Close() Releases the shared *RootVM (refcount-- → Close at 0) AND
		// closes the compile cache — the single teardown chokepoint. Do NOT also
		// call cc.rootVM.Close() (double-close) or bare compileCache.Close (leaks
		// the registry refcount).
		t.Cleanup(func() { _ = cc.Close() })

		// Now call parsePerRouteWasm directly with the same Wasm proto shape.
		w := buildWasmProtoInlineBytes(modBytes, "perroute_test_valid2")
		cfg, err := parsePerRouteWasm(w, envoyhttp.FactoryCtx{})
		if err != nil {
			t.Fatalf("parsePerRouteWasm valid input: unexpected error: %v", err)
		}
		if cfg == nil {
			t.Fatal("parsePerRouteWasm valid input: got nil *compiledConfig; want non-nil")
		}
		// Release the per-route compiledConfig (Releases the shared *RootVM
		// refcount → Close at 0, and closes the compile cache) to avoid leaking
		// goroutines + a stale registry entry across test runs.
		_ = cfg.Close()
	})

	t.Run("invalid_config_nil_delegates_to_buildCompiledConfig", func(t *testing.T) {
		t.Parallel()
		// A *wasmv3.Wasm with no Config (PluginConfig nil) fires arm 3
		// (parseRejectConfigRequired) inside buildCompiledConfig — the SAME
		// error the listener path returns, proving delegation via anypb round-trip.
		w := &wasmv3.Wasm{Config: nil}
		_, err := parsePerRouteWasm(w, envoyhttp.FactoryCtx{})
		if err == nil {
			t.Fatal("parsePerRouteWasm nil-config: expected error; got nil")
		}
		if err.Error() != parseRejectConfigRequired {
			t.Fatalf("parsePerRouteWasm nil-config error = %q; want %q (arm 3 = same as listener path)",
				err.Error(), parseRejectConfigRequired)
		}
	})

	t.Run("invalid_vm_config_nil_delegates_to_buildCompiledConfig", func(t *testing.T) {
		t.Parallel()
		// A *wasmv3.Wasm with Config.Vm nil fires arm 4 (parseRejectVmConfigRequired).
		w := &wasmv3.Wasm{
			Config: &wasmcommonv3.PluginConfig{
				Name: "perroute_test_invalid_vm",
				// Vm field left nil → arm 4.
			},
		}
		_, err := parsePerRouteWasm(w, envoyhttp.FactoryCtx{})
		if err == nil {
			t.Fatal("parsePerRouteWasm nil-vm-config: expected error; got nil")
		}
		if err.Error() != parseRejectVmConfigRequired {
			t.Fatalf("parsePerRouteWasm nil-vm-config error = %q; want %q (arm 4 = same as listener path)",
				err.Error(), parseRejectVmConfigRequired)
		}
	})

	t.Run("wrong_type_returns_type_mismatch_error", func(t *testing.T) {
		t.Parallel()
		// Pass a non-*wasmv3.Wasm proto (e.g. *wrapperspb.StringValue).
		// parsePerRouteWasm must return a non-nil error containing type info.
		wrongType := wrapperspb.String("not-a-wasm-config")
		_, err := parsePerRouteWasm(wrongType, envoyhttp.FactoryCtx{})
		if err == nil {
			t.Fatal("parsePerRouteWasm wrong-type: expected type-mismatch error; got nil")
		}
		// Error must mention the expected type so the operator knows what was
		// wanted. Mirrors parsePerRouteLua's "expected *luav3.LuaPerRoute, got %T"
		// shape.
		if !strings.Contains(err.Error(), "wasmv3.Wasm") {
			t.Fatalf("parsePerRouteWasm wrong-type error = %q; want mention of *wasmv3.Wasm expected type",
				err.Error())
		}
	})
}

// TestResolvePerRoute_PrecedenceOverListener exercises the 2-tier most-
// specific-wins resolution: per-route present → use per-route; nil per-route
// → fall back to listener.
func TestResolvePerRoute_PrecedenceOverListener(t *testing.T) {
	t.Parallel()

	L := &compiledConfig{}
	R := &compiledConfig{}

	if got := resolvePerRoute(R, L); got != R {
		t.Fatal("resolvePerRoute(R, L): per-route must win over listener; got listener")
	}
	if got := resolvePerRoute(nil, L); got != L {
		t.Fatal("resolvePerRoute(nil, L): nil per-route must fall back to listener; got wrong config")
	}
}

// buildWasmProtoInlineBytes is a test helper that constructs a *wasmv3.Wasm
// proto embedding the given wasm module bytes inline, with the given plugin
// name. Used by TestParsePerRouteWasm_WholesaleCompile to build a per-route
// Wasm proto for the valid-compile sub-test.
//
// The proto shape mirrors newTestCompiledConfig in dispatch_test.go; the
// unique pluginName parameter avoids arm-26 cross-test duplicate-name
// collisions from the process-wide registry.
func buildWasmProtoInlineBytes(modBytes []byte, pluginName string) *wasmv3.Wasm {
	return &wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:   pluginName,
			RootId: "test_root",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    "test_vm_" + pluginName,
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{
									InlineBytes: modBytes,
								},
							},
						},
					},
				},
			},
		},
	}
}
