package wasm

// wasm_test.go — Task 8 skeleton tests per PLAN Task 8 + 25.1 SPEC §4.1.
//
// Test surface at Task 8 (Tier B filter package skeleton):
//   1. TypeURL byte-exact pin per 25.1 SPEC §4.1 + parent §6.2.
//   2. filterName byte-exact pin per ADR-0070 envoy.filters.http.wasm.
//   3. New(nil, FactoryCtx{}) returns sentinel error per Task 8 skeleton
//      contract (real parse + factory wiring lands at Task 9 + 11 + 12).
//   4-8. Stat-name byte-exact pins per AMEND-A2 tri-group prefix structure
//      (Group B wasm.wazero.{created,active} upstream-parity +
//      envoy-go-strict suffixes {executions, hostcall_denied,
//      envoy_go.failures}).
//   9. newFilterStats(reg, "myplugin") allocates exactly 5 non-nil fields
//      + registers byte-exact wire-name strings under tri-group template.
//   10. newFilterStats(nil, ...) returns nil per ADR-0085 nil-tolerance.
//   11. Project stat-count delta +5 per call (independent of any pre-
//      existing baseline — verifies the 114 → 119 phase-level delta
//      indirectly by asserting per-call +5 on a fresh registry).
//
// This file extends across Tasks 11 + 12 to ~1500-2000 LoC by phase end;
// at Task 8 it's ~150-300 LoC.

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Constant byte-pin tests (PLAN Task 8 tests 1-2 + 4-8).
// -----------------------------------------------------------------------------

// TestTypeURL pins the byte-exact wire URL per 25.1 SPEC §4.1 + parent §6.2
// arm 6. A regression on the URL surfaces here before propagating to
// listener-config parsing.
func TestTypeURL(t *testing.T) {
	const expected = "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"
	if TypeURL != expected {
		t.Fatalf("TypeURL = %q; want %q", TypeURL, expected)
	}
}

// TestFilterName pins the registered filter-name constant per ADR-0070 +
// canonical Envoy `envoy.filters.http.wasm`.
func TestFilterName(t *testing.T) {
	const expected = "envoy.filters.http.wasm"
	if filterName != expected {
		t.Fatalf("filterName = %q; want %q", filterName, expected)
	}
}

// TestStatNames_Equal_Wazero_Created pins the full wire name for the Group-B
// upstream-parity created counter per AMEND-A2.
func TestStatNames_Equal_Wazero_Created(t *testing.T) {
	const expected = "wasm.wazero.created"
	if statNameCreated != expected {
		t.Fatalf("statNameCreated = %q; want %q", statNameCreated, expected)
	}
}

// TestStatNames_Equal_Wazero_Active pins the full wire name for the Group-B
// upstream-parity active gauge per AMEND-A2.
func TestStatNames_Equal_Wazero_Active(t *testing.T) {
	const expected = "wasm.wazero.active"
	if statNameActive != expected {
		t.Fatalf("statNameActive = %q; want %q", statNameActive, expected)
	}
}

// TestStatNames_Equal_Executions_Suffix pins the byte-exact envoy-go-strict
// executions suffix per AMEND-A2 (full wire name `wasm.<plugin_name>.executions`
// is constructed at registry-call time).
func TestStatNames_Equal_Executions_Suffix(t *testing.T) {
	const expected = "executions"
	if statNameExecutions != expected {
		t.Fatalf("statNameExecutions = %q; want %q", statNameExecutions, expected)
	}
}

// TestStatNames_Equal_HostcallDenied_Suffix pins the byte-exact envoy-go-strict
// hostcall_denied suffix per AMEND-A2 + AMEND-A5 default-deny capability
// sandbox.
func TestStatNames_Equal_HostcallDenied_Suffix(t *testing.T) {
	const expected = "hostcall_denied"
	if statNameHostcallDenied != expected {
		t.Fatalf("statNameHostcallDenied = %q; want %q", statNameHostcallDenied, expected)
	}
}

// TestStatNames_Equal_EnvoyGoFailures_Suffix pins the byte-exact envoy-go-strict
// envoy_go.failures suffix per AMEND-A2. The dotted-suffix form is intentional:
// the full wire name `wasm.<plugin>.envoy_go.failures` carries an internal
// `envoy_go.` segment marking the envoy-go-strict origin of the metric.
func TestStatNames_Equal_EnvoyGoFailures_Suffix(t *testing.T) {
	const expected = "envoy_go.failures"
	if statNameEnvoyGoFailures != expected {
		t.Fatalf("statNameEnvoyGoFailures = %q; want %q", statNameEnvoyGoFailures, expected)
	}
}

// -----------------------------------------------------------------------------
// New() arm-1 PARSE-REJECT contract (PLAN Task 12; replaces Task 8 skeleton).
// -----------------------------------------------------------------------------

// TestNew_NilTypedConfig_ReturnsArm1ParseReject pins the ADR-0072 boot-time-
// fail-fast surface: the factory returns the arm-1 PARSE-REJECT verbatim
// when typed_config is nil. Replaces the Task 8 skeleton sentinel
// assertion; the sentinel was removed at Task 12 when the New body was
// wired through to buildCompiledConfig.
func TestNew_NilTypedConfig_ReturnsArm1ParseReject(t *testing.T) {
	f, err := New(nil, envoyhttp.FactoryCtx{})
	if f != nil {
		t.Errorf("New returned non-nil factory %v; want nil at arm-1 PARSE-REJECT", f)
	}
	if err == nil {
		t.Fatal("New returned nil error; want arm-1 PARSE-REJECT")
	}
	const wantWording = "wasm: typed_config required"
	if err.Error() != wantWording {
		t.Errorf("New err = %q; want %q", err.Error(), wantWording)
	}
	// Sanity: the byte-stable arm-1 wording is the canonical
	// parseRejectTypedConfigRequired constant from compiled_config.go.
	if err.Error() != parseRejectTypedConfigRequired {
		t.Errorf("New err does not match parseRejectTypedConfigRequired const")
	}
	// Substring check kept for grep-discoverability across phases.
	if !strings.Contains(err.Error(), "typed_config") {
		t.Errorf("New err = %q; want substring 'typed_config'", err.Error())
	}
}

// TestNew_WrongTypeURL_ReturnsArm2Unmarshal verifies the factory bubbles
// up the arm-2 (typed_config unmarshal) PARSE-REJECT when the supplied
// *anypb.Any envelope cannot be unmarshaled to *wasmv3.Wasm. The arm-2
// constant is %w-wrapped so we substring-check rather than byte-equal.
func TestNew_WrongTypeURL_ReturnsArm2Unmarshal(t *testing.T) {
	tc := &anypb.Any{TypeUrl: "type.googleapis.com/some.unknown.Type", Value: []byte{0x01}}
	f, err := New(tc, envoyhttp.FactoryCtx{})
	if f != nil {
		t.Errorf("New returned non-nil factory; want nil at arm-2 PARSE-REJECT")
	}
	if err == nil {
		t.Fatal("New returned nil error; want arm-2 PARSE-REJECT")
	}
	const wantPrefix = "wasm: typed_config unmarshal:"
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Errorf("New err = %q; want prefix %q", err.Error(), wantPrefix)
	}
}

// -----------------------------------------------------------------------------
// validatePerRouteWasm arm-18 PARSE-REJECT (Task 8 skeleton).
// -----------------------------------------------------------------------------

// TestValidatePerRouteWasm_RejectsWithArm18Wording pins the byte-stable
// arm-18 PARSE-REJECT wording per parent §6.2 arm 18 + AMEND-A3 5th-
// canonical REUSE-by-absence. 25.3 IMPL replaces the body with the real
// per-route shape validator; at 25.1 ANY input rejects.
func TestValidatePerRouteWasm_RejectsWithArm18Wording(t *testing.T) {
	const expectedWording = "wasm: per-route configuration is not yet supported (lands in phase 25.3)"
	err := validatePerRouteWasm(nil)
	if err == nil {
		t.Fatal("validatePerRouteWasm(nil) returned nil; want arm-18 PARSE-REJECT")
	}
	if err.Error() != expectedWording {
		t.Fatalf("validatePerRouteWasm err = %q; want %q", err.Error(), expectedWording)
	}
}

// -----------------------------------------------------------------------------
// newFilterStats 5-counter allocation surface (PLAN Task 8 tests 9-11).
// -----------------------------------------------------------------------------

// TestNewFilterStats_AllocatesFiveCounters verifies that newFilterStats
// constructs all 5 fields non-nil + registers them under the tri-group
// template (Group B wasm.wazero.* upstream-parity + envoy-go-strict
// wasm.<plugin>.*).
func TestNewFilterStats_AllocatesFiveCounters(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "myplugin")
	if fs == nil {
		t.Fatal("newFilterStats returned nil with non-nil registry")
	}
	if fs.created == nil {
		t.Error("filterStats.created is nil; want non-nil counter")
	}
	if fs.active == nil {
		t.Error("filterStats.active is nil; want non-nil gauge")
	}
	if fs.executions == nil {
		t.Error("filterStats.executions is nil; want non-nil counter")
	}
	if fs.hostcallDenied == nil {
		t.Error("filterStats.hostcallDenied is nil; want non-nil counter")
	}
	if fs.envoyGoFailures == nil {
		t.Error("filterStats.envoyGoFailures is nil; want non-nil counter")
	}

	// Pin the exact registered wire names via Walk-introspection.
	wantNames := map[string]bool{
		"wasm.wazero.created":             false,
		"wasm.wazero.active":              false,
		"wasm.myplugin.executions":        false,
		"wasm.myplugin.hostcall_denied":   false,
		"wasm.myplugin.envoy_go.failures": false,
	}
	reg.Walk(func(m stats.Metric) {
		if _, ok := wantNames[m.Name()]; ok {
			wantNames[m.Name()] = true
		}
	})
	for name, seen := range wantNames {
		if !seen {
			t.Errorf("registry missing expected stat name %q", name)
		}
	}
}

// TestNewFilterStats_NilRegistry_ReturnsNil verifies ADR-0085 nil-tolerance:
// a nil registry produces a nil *filterStats (consumers nil-check before
// incrementing per the family-wide pattern).
func TestNewFilterStats_NilRegistry_ReturnsNil(t *testing.T) {
	fs := newFilterStats(nil, "anyplugin")
	if fs != nil {
		t.Errorf("newFilterStats(nil, ...) = %v; want nil per ADR-0085", fs)
	}
}

// TestNewFilterStats_ProjectStatCountDelta verifies the +5 stat-count delta
// per call against a fresh registry (the 114 → 119 phase-level project
// delta is the sum of this +5 over the single 25.1 wasm-filter call site).
// Verifies via Walk-count introspection (the registry exposes Walk but no
// direct Count; we use a closure counter).
func TestNewFilterStats_ProjectStatCountDelta(t *testing.T) {
	reg := stats.NewRegistry()

	baseline := 0
	reg.Walk(func(stats.Metric) { baseline++ })
	if baseline != 0 {
		t.Fatalf("fresh registry baseline = %d; want 0", baseline)
	}

	fs := newFilterStats(reg, "plugin_x")
	if fs == nil {
		t.Fatal("newFilterStats returned nil")
	}

	post := 0
	reg.Walk(func(stats.Metric) { post++ })
	const wantDelta = 5
	if post-baseline != wantDelta {
		t.Errorf("stat-count delta = %d; want +%d (per AMEND-A2 5-counter surface)", post-baseline, wantDelta)
	}
}

// TestNewFilterStats_PluginNameInterpolation verifies that the plugin-name
// discriminator threads through to the envoy-go-strict counter wire names
// (Group-C stat-prefix per AMEND-A2). Two plugins with different names
// register collision-free stats.
func TestNewFilterStats_PluginNameInterpolation(t *testing.T) {
	reg := stats.NewRegistry()
	_ = newFilterStats(reg, "plugin_a")
	_ = newFilterStats(reg, "plugin_b")

	want := map[string]bool{
		// Group B stats — registered TWICE under the same name would panic the
		// registry; the first call registers, the second call's NewCounter
		// would panic on duplicate. Since both plugins share Group-B name-
		// space, this test path expects newFilterStats to use NewCounterIfAbsent
		// for the Group-B keys (created/active) — they are per-runtime,
		// shared across plugins. The envoy-go-strict per-plugin keys are
		// per-plugin and registered fresh.
		"wasm.wazero.created":             false,
		"wasm.wazero.active":              false,
		"wasm.plugin_a.executions":        false,
		"wasm.plugin_a.hostcall_denied":   false,
		"wasm.plugin_a.envoy_go.failures": false,
		"wasm.plugin_b.executions":        false,
		"wasm.plugin_b.hostcall_denied":   false,
		"wasm.plugin_b.envoy_go.failures": false,
	}
	reg.Walk(func(m stats.Metric) {
		if _, ok := want[m.Name()]; ok {
			want[m.Name()] = true
		}
	})
	for name, seen := range want {
		if !seen {
			t.Errorf("registry missing expected stat name %q", name)
		}
	}
}
