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
// 25.2 IMPL Task 17 — 8 NEW envoy-go-strict counter byte-exact pins per
// 25.2 SPEC §7.1 + AMEND-B3 (combined with statNameBodyBufferCapExceeded
// landed at Task 16, these complete the 9-counter delta). Each test mirrors
// the Task-8 5-counter pattern: a single const != literal assertion.
// -----------------------------------------------------------------------------

// TestStatNames_Equal_Wasm_BodyBufferCapExceeded pins the byte-exact
// envoy-go-strict suffix landed at Task 16 (the FIRST of the 9 counters).
// Included in the Task-17 pin block for the consolidated 9-counter coverage.
func TestStatNames_Equal_Wasm_BodyBufferCapExceeded(t *testing.T) {
	const expected = "body_buffer_cap_exceeded"
	if statNameBodyBufferCapExceeded != expected {
		t.Fatalf("statNameBodyBufferCapExceeded = %q; want %q", statNameBodyBufferCapExceeded, expected)
	}
}

// TestStatNames_Equal_Wasm_TickInvocations pins the byte-exact envoy-go-strict
// suffix for counter 6 per 25.2 SPEC §7.1 + Q5.
func TestStatNames_Equal_Wasm_TickInvocations(t *testing.T) {
	const expected = "tick_invocations"
	if statNameTickInvocations != expected {
		t.Fatalf("statNameTickInvocations = %q; want %q", statNameTickInvocations, expected)
	}
}

// TestStatNames_Equal_Wasm_HttpCallDispatched pins the byte-exact envoy-go-
// strict suffix for counter 7 per 25.2 SPEC §7.1 + Q4 + AMEND-B3.
func TestStatNames_Equal_Wasm_HttpCallDispatched(t *testing.T) {
	const expected = "http_call_dispatched"
	if statNameHttpCallDispatched != expected {
		t.Fatalf("statNameHttpCallDispatched = %q; want %q", statNameHttpCallDispatched, expected)
	}
}

// TestStatNames_Equal_Wasm_HttpCallResponse pins the byte-exact envoy-go-
// strict suffix for counter 8 per 25.2 SPEC §7.1 + Q4.
func TestStatNames_Equal_Wasm_HttpCallResponse(t *testing.T) {
	const expected = "http_call_response"
	if statNameHttpCallResponse != expected {
		t.Fatalf("statNameHttpCallResponse = %q; want %q", statNameHttpCallResponse, expected)
	}
}

// TestStatNames_Equal_Wasm_ForeignFunctionDenied pins the byte-exact envoy-
// go-strict suffix for counter 9 per 25.2 SPEC §7.1 + AMEND-A9.
func TestStatNames_Equal_Wasm_ForeignFunctionDenied(t *testing.T) {
	const expected = "foreign_function_denied"
	if statNameForeignFunctionDenied != expected {
		t.Fatalf("statNameForeignFunctionDenied = %q; want %q", statNameForeignFunctionDenied, expected)
	}
}

// TestStatNames_Equal_Wasm_HttpCallDispatchUnknownCluster pins the byte-
// exact envoy-go-strict suffix for counter 11 per 25.2 SPEC §7.1 + Q4.
func TestStatNames_Equal_Wasm_HttpCallDispatchUnknownCluster(t *testing.T) {
	const expected = "http_call_dispatch_unknown_cluster"
	if statNameHttpCallDispatchUnknownCluster != expected {
		t.Fatalf("statNameHttpCallDispatchUnknownCluster = %q; want %q", statNameHttpCallDispatchUnknownCluster, expected)
	}
}

// TestStatNames_Equal_Wasm_SharedDataCapExceeded pins the byte-exact
// envoy-go-strict suffix for counter 12 per 25.2 SPEC §7.1 + Q6.
func TestStatNames_Equal_Wasm_SharedDataCapExceeded(t *testing.T) {
	const expected = "shared_data_cap_exceeded"
	if statNameSharedDataCapExceeded != expected {
		t.Fatalf("statNameSharedDataCapExceeded = %q; want %q", statNameSharedDataCapExceeded, expected)
	}
}

// TestStatNames_Equal_Wasm_DynamicStatsCapExceeded pins the byte-exact
// envoy-go-strict suffix for counter 13 per 25.2 SPEC §7.1 + Q9.
func TestStatNames_Equal_Wasm_DynamicStatsCapExceeded(t *testing.T) {
	const expected = "dynamic_stats_cap_exceeded"
	if statNameDynamicStatsCapExceeded != expected {
		t.Fatalf("statNameDynamicStatsCapExceeded = %q; want %q", statNameDynamicStatsCapExceeded, expected)
	}
}

// TestStatNames_Equal_Wasm_HttpCallResponseAfterClose pins the byte-exact
// envoy-go-strict suffix for counter 14 per 25.2 SPEC §7.1 + AMEND-B3 (the
// AMEND-B3-added defensive observability counter; NEW vs BRAINSTORM Q9).
func TestStatNames_Equal_Wasm_HttpCallResponseAfterClose(t *testing.T) {
	const expected = "http_call_response_after_close"
	if statNameHttpCallResponseAfterClose != expected {
		t.Fatalf("statNameHttpCallResponseAfterClose = %q; want %q", statNameHttpCallResponseAfterClose, expected)
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
// constructs all 5 of the 25.1-era fields non-nil + registers them under
// the tri-group template (Group B wasm.wazero.* upstream-parity +
// envoy-go-strict wasm.<plugin>.*). Test name kept stable across phases
// for git-blame continuity; the 25.2 EXTENSIONS (9 NEW envoy-go-strict
// counters) are exercised at TestNewFilterStats_AllocatesAll14Counters
// + TestNewFilterStats_ProjectStatCountDelta below.
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

	// Pin the exact registered wire names via Walk-introspection — the
	// 25.1 5-stat baseline (the 9 NEW 25.2 counters are pinned below).
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

// TestNewFilterStats_AllocatesAll14Counters verifies that newFilterStats
// constructs ALL 14 stat fields non-nil at 25.2 phase-done (2 shared
// Group-B + 12 envoy-go-strict per-plugin = 3 from 25.1 + 9 NEW from 25.2
// §7.1 + AMEND-B3) + registers the 9 NEW envoy-go-strict wire names.
// Companion to TestStatNames_Equal_Wasm_* (each constant byte-pinned
// individually) + TestNewFilterStats_ProjectStatCountDelta (per-call
// stat-count delta verified).
func TestNewFilterStats_AllocatesAll14Counters(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "myplugin")
	if fs == nil {
		t.Fatal("newFilterStats returned nil with non-nil registry")
	}
	// 25.2-era counter fields (Task 16 + Task 17). Field nil-checks ensure
	// each counter is constructed; a regression that skips a counter
	// surfaces here immediately.
	if fs.bodyBufferCapExceeded == nil {
		t.Error("filterStats.bodyBufferCapExceeded is nil; want non-nil counter (Task 16)")
	}
	if fs.tickInvocations == nil {
		t.Error("filterStats.tickInvocations is nil; want non-nil counter (Task 17)")
	}
	if fs.httpCallDispatched == nil {
		t.Error("filterStats.httpCallDispatched is nil; want non-nil counter (Task 17)")
	}
	if fs.httpCallResponse == nil {
		t.Error("filterStats.httpCallResponse is nil; want non-nil counter (Task 17)")
	}
	if fs.foreignFunctionDenied == nil {
		t.Error("filterStats.foreignFunctionDenied is nil; want non-nil counter (Task 17)")
	}
	if fs.httpCallDispatchUnknownCluster == nil {
		t.Error("filterStats.httpCallDispatchUnknownCluster is nil; want non-nil counter (Task 17)")
	}
	if fs.sharedDataCapExceeded == nil {
		t.Error("filterStats.sharedDataCapExceeded is nil; want non-nil counter (Task 17)")
	}
	if fs.dynamicStatsCapExceeded == nil {
		t.Error("filterStats.dynamicStatsCapExceeded is nil; want non-nil counter (Task 17)")
	}
	if fs.httpCallResponseAfterClose == nil {
		t.Error("filterStats.httpCallResponseAfterClose is nil; want non-nil counter (Task 17, AMEND-B3)")
	}

	// Pin the 9 NEW envoy-go-strict wire names (full wire form for the
	// "myplugin" plugin discriminator).
	want := map[string]bool{
		"wasm.myplugin.body_buffer_cap_exceeded":           false,
		"wasm.myplugin.tick_invocations":                   false,
		"wasm.myplugin.http_call_dispatched":               false,
		"wasm.myplugin.http_call_response":                 false,
		"wasm.myplugin.foreign_function_denied":            false,
		"wasm.myplugin.http_call_dispatch_unknown_cluster": false,
		"wasm.myplugin.shared_data_cap_exceeded":           false,
		"wasm.myplugin.dynamic_stats_cap_exceeded":         false,
		"wasm.myplugin.http_call_response_after_close":     false,
	}
	reg.Walk(func(m stats.Metric) {
		if _, ok := want[m.Name()]; ok {
			want[m.Name()] = true
		}
	})
	for name, seen := range want {
		if !seen {
			t.Errorf("registry missing expected 25.2 stat name %q", name)
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

// TestNewFilterStats_ProjectStatCountDelta verifies the +14 stat-count delta
// per call against a fresh registry at 25.2 phase-done (the 119 → 128
// project-level delta is the sum of this +14 minus the 5 already accounted
// at 25.1 — i.e., the per-call delta on a FRESH registry is +14 = 2 Group-B
// (created counter + active gauge) + 12 envoy-go-strict per-plugin
// (executions + hostcall_denied + envoy_go.failures + body_buffer_cap_exceeded
// + tick_invocations + http_call_dispatched + http_call_response +
// foreign_function_denied + http_call_dispatch_unknown_cluster +
// shared_data_cap_exceeded + dynamic_stats_cap_exceeded +
// http_call_response_after_close)). The 25.1 +5 baseline → 25.2 +14 reflects
// the +9 NEW envoy-go-strict counters per 25.2 SPEC §7.1 + AMEND-B3
// (counter 14 added by AMEND-B3 over BRAINSTORM Q9's 8-counter tally).
//
// Project stat count assertion: 25.1 baseline 119 (the wasm filter accounts
// for +5 of that) → 25.2 baseline 128 (the wasm filter accounts for +14 of
// that, since the 25.1 +5 stays + the 25.2 +9 lands). Verified at this
// per-call delta + at the wire-name pins above.
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
	const wantDelta = 14 // 25.2 phase-done; was 5 at 25.1 — +9 from §7.1
	if post-baseline != wantDelta {
		t.Errorf("stat-count delta = %d; want +%d (25.2 per AMEND-A2 + §7.1 14-stat surface = 2 Group B + 12 envoy-go-strict per plugin)", post-baseline, wantDelta)
	}
}

// TestProjectStatCount_Wasm25_2 asserts the project-level stat-count
// contribution of the wasm filter at 25.2 phase-done is exactly 14 per
// plugin instance (2 shared Group-B + 12 envoy-go-strict per-plugin),
// rolling up to the 119 → 128 project total per 25.2 SPEC §7 + AMEND-B3.
// This is the byte-exact pin for the AMEND-B3 + Q9 9-counter tally — a
// regression that adds/removes a counter surfaces here before propagating
// to the project-wide BEHAVIOR_CONTRACT.md stat tally row.
func TestProjectStatCount_Wasm25_2(t *testing.T) {
	reg := stats.NewRegistry()
	_ = newFilterStats(reg, "plugin_assert")

	const wantTotal = 14 // 2 Group-B + 12 envoy-go-strict per plugin
	got := 0
	reg.Walk(func(stats.Metric) { got++ })
	if got != wantTotal {
		t.Errorf("wasm filter stat-count = %d; want %d (25.2 §7.1 9-counter delta over 25.1 5-baseline)", got, wantTotal)
	}

	// Also assert the 9 envoy-go-strict + 3 25.1-baseline + 2 Group-B
	// shape — i.e., 14 = 2 + 3 + 9. A delta on any sub-group bumps the
	// assertion-failure with a discriminating message via the wantNames
	// table pinned below (12 envoy-go-strict per-plugin + 2 Group B).
	wantNames := map[string]bool{
		// Group B (2):
		"wasm.wazero.created": false,
		"wasm.wazero.active":  false,
		// 25.1 envoy-go-strict (3):
		"wasm.plugin_assert.executions":        false,
		"wasm.plugin_assert.hostcall_denied":   false,
		"wasm.plugin_assert.envoy_go.failures": false,
		// 25.2 envoy-go-strict §7.1 + AMEND-B3 (9):
		"wasm.plugin_assert.body_buffer_cap_exceeded":           false,
		"wasm.plugin_assert.tick_invocations":                   false,
		"wasm.plugin_assert.http_call_dispatched":               false,
		"wasm.plugin_assert.http_call_response":                 false,
		"wasm.plugin_assert.foreign_function_denied":            false,
		"wasm.plugin_assert.http_call_dispatch_unknown_cluster": false,
		"wasm.plugin_assert.shared_data_cap_exceeded":           false,
		"wasm.plugin_assert.dynamic_stats_cap_exceeded":         false,
		"wasm.plugin_assert.http_call_response_after_close":     false,
	}
	reg.Walk(func(m stats.Metric) {
		if _, ok := wantNames[m.Name()]; ok {
			wantNames[m.Name()] = true
		}
	})
	for name, seen := range wantNames {
		if !seen {
			t.Errorf("registry missing expected stat name %q (25.2 §7.1 + AMEND-B3 14-stat surface)", name)
		}
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
