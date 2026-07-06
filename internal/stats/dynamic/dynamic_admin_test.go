// Tests for the NEW internal/stats/dynamic/ admin /stats enumeration round-
// trip + name-format pin per R-25.2-2 + AMEND-B2 + 25.2 SPEC §7.3.
//
// Per AMEND-B2 REFINEMENT (REFUTES BRAINSTORM Q9 plugin-prefix hypothesis):
// the wire-perspective stat name is "wasmcustom.<custom_name>" ONLY (NO
// plugin prefix). Per-plugin isolation happens via the per-plugin Registry
// scope — each Registry is constructed with a pluginScopePrefix (e.g.
// "wasm.<plugin_name>") so the admin /stats endpoint enumerates the
// concrete underlying stats.Counter / stats.Gauge with the full name
// "wasm.<plugin_name>.wasmcustom.<custom_name>". The dynamic.Registry's
// EnumerateForAdmin callback receives the wire-perspective name (the
// byte-faithful proxy-wasm view) — "wasmcustom.<custom_name>" — for the
// proxy-wasm guest's introspection use case.
//
// These tests pin:
//
//   - The Counter/Gauge registered against the parent *stats.Registry has the
//     full name "wasm.<plugin>.wasmcustom.<name>" (verified via Walk).
//   - EnumerateForAdmin's callback receives the wire-perspective name
//     "wasmcustom.<name>" + the current value.
//   - The callback is invoked exactly once per registered metric.
//   - Histogram-typed metrics also enumerate (the stub primitive surfaces
//     the most recently Record-ed value).
//   - Empty registry → callback never invoked.
package dynamic

import (
	"sort"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// TestEnumerateForAdmin_EmptyRegistry_NoCallback verifies that
// EnumerateForAdmin on an empty Registry never invokes the callback.
func TestEnumerateForAdmin_EmptyRegistry_NoCallback(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	called := 0
	dyn.EnumerateForAdmin(func(name string, value uint64) {
		called++
		t.Errorf("unexpected callback for name=%q value=%d on empty registry", name, value)
	})
	if called != 0 {
		t.Errorf("callback called %d times; want 0", called)
	}
}

// TestEnumerateForAdmin_SingleCounter_NameAndValue verifies the
// wire-perspective name format "wasmcustom.<custom_name>" + the
// Counter value after Increment.
func TestEnumerateForAdmin_SingleCounter_NameAndValue(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeCounter, "my_counter")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Increment(id, 5); err != nil {
		t.Fatalf("Increment err = %v; want nil", err)
	}
	type row struct {
		name  string
		value uint64
	}
	var got []row
	dyn.EnumerateForAdmin(func(name string, value uint64) {
		got = append(got, row{name: name, value: value})
	})
	want := []row{{name: "wasmcustom.my_counter", value: 5}}
	if len(got) != len(want) {
		t.Fatalf("enumerated %d rows; want %d (got=%+v)", len(got), len(want), got)
	}
	if got[0] != want[0] {
		t.Errorf("row 0 = %+v; want %+v", got[0], want[0])
	}
}

// TestEnumerateForAdmin_NamePin_NoPluginPrefix per R-25.2-2 + AMEND-B2.
// The wire-perspective name passed to the callback is "wasmcustom.<name>"
// ONLY — the plugin name does NOT appear in the namespace prefix. This
// pins the AMEND-B2 REFINEMENT byte-for-byte (BRAINSTORM Q9 hypothesized
// "wasmcustom.<plugin_name>.<custom_name>" — REFUTED per Envoy v1.37.2
// source/extensions/common/wasm/stats_handler.h:16 +
// context.cc:1623-1625).
func TestEnumerateForAdmin_NamePin_NoPluginPrefix(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, _ := dyn.Register(MetricTypeCounter, "metric_a")
	_ = dyn.Increment(id, 1)
	var name string
	dyn.EnumerateForAdmin(func(n string, _ uint64) { name = n })
	// MUST NOT contain "testplugin" — the plugin name is NOT in the
	// wire-perspective stat name per AMEND-B2.
	if got := name; got != "wasmcustom.metric_a" {
		t.Errorf("wire-perspective name = %q; want %q", got, "wasmcustom.metric_a")
	}
}

// TestEnumerateForAdmin_MultipleMetrics_AllEnumerated verifies that
// multiple registered metrics each surface exactly once with the
// correct (name, value) tuple.
func TestEnumerateForAdmin_MultipleMetrics_AllEnumerated(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)

	idC, _ := dyn.Register(MetricTypeCounter, "requests")
	_ = dyn.Increment(idC, 7)

	idG, _ := dyn.Register(MetricTypeGauge, "queue_depth")
	_ = dyn.Record(idG, 42)

	idH, _ := dyn.Register(MetricTypeHistogram, "latency_ms")
	_ = dyn.Record(idH, 100)

	type row struct {
		name  string
		value uint64
	}
	var got []row
	dyn.EnumerateForAdmin(func(name string, value uint64) {
		got = append(got, row{name: name, value: value})
	})

	// Sort by name for deterministic comparison.
	sort.Slice(got, func(i, j int) bool { return got[i].name < got[j].name })

	want := []row{
		{name: "wasmcustom.latency_ms", value: 100},
		{name: "wasmcustom.queue_depth", value: 42},
		{name: "wasmcustom.requests", value: 7},
	}
	if len(got) != len(want) {
		t.Fatalf("enumerated %d rows; want %d (got=%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v; want %+v", i, got[i], want[i])
		}
	}
}

// TestRegistry_UnderlyingStats_PluginScopePrefix verifies the underlying
// *stats.Counter / *stats.Gauge registered against the parent
// *stats.Registry carries the FULL admin-side name
// "wasm.<plugin_name>.wasmcustom.<custom_name>" — this is what the admin
// /stats endpoint enumerates per 25.2 SPEC §7.3 (admin operator sees
// "wasm.<plugin>.wasmcustom.<custom>"; proxy-wasm guest sees
// "wasmcustom.<custom>" via EnumerateForAdmin).
func TestRegistry_UnderlyingStats_PluginScopePrefix(t *testing.T) {
	dyn, reg := newTestRegistry(t, 1024)
	_, err := dyn.Register(MetricTypeCounter, "my_counter")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	// Walk the parent stats.Registry; expect the full prefixed name.
	var names []string
	reg.Walk(func(m stats.Metric) {
		names = append(names, m.Name())
	})
	wantFull := "wasm.testplugin.wasmcustom.my_counter"
	found := false
	for _, n := range names {
		if n == wantFull {
			found = true
		}
	}
	if !found {
		t.Errorf("parent stats.Registry does NOT contain %q; got names=%v", wantFull, names)
	}
}

// TestRegistry_UnderlyingStats_PluginScopePrefix_Gauge — same admin-side
// pin for Gauge primitives (Walk reports the full name).
func TestRegistry_UnderlyingStats_PluginScopePrefix_Gauge(t *testing.T) {
	dyn, reg := newTestRegistry(t, 1024)
	_, err := dyn.Register(MetricTypeGauge, "my_gauge")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	var names []string
	reg.Walk(func(m stats.Metric) {
		names = append(names, m.Name())
	})
	wantFull := "wasm.testplugin.wasmcustom.my_gauge"
	found := false
	for _, n := range names {
		if n == wantFull {
			found = true
		}
	}
	if !found {
		t.Errorf("parent stats.Registry does NOT contain %q; got names=%v", wantFull, names)
	}
}

// TestEnumerateForAdmin_AfterIncrement_ValueReflectsLatest verifies that
// repeated EnumerateForAdmin calls reflect the latest value (the callback
// reads the current atomic value at the time of the call).
func TestEnumerateForAdmin_AfterIncrement_ValueReflectsLatest(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, _ := dyn.Register(MetricTypeCounter, "live")

	_ = dyn.Increment(id, 1)
	var v1 uint64
	dyn.EnumerateForAdmin(func(_ string, value uint64) { v1 = value })
	if v1 != 1 {
		t.Errorf("after Increment(+1) v1 = %d; want 1", v1)
	}

	_ = dyn.Increment(id, 9)
	var v2 uint64
	dyn.EnumerateForAdmin(func(_ string, value uint64) { v2 = value })
	if v2 != 10 {
		t.Errorf("after Increment(+9) v2 = %d; want 10", v2)
	}
}

// TestEnumerateForAdmin_EmptyPluginScope_Permitted verifies that an empty
// pluginScopePrefix is permitted — the underlying stats name becomes
// ".wasmcustom.<custom>" (interior consecutive dots; rule SN-permissive
// per parent stats.nameRE which only rejects trailing dots).
//
// Empty pluginScopePrefix mirrors the phase-22.1 lua empty-stat-prefix
// precedent — the PARSE-REJECT decision lives at the *consumer* (Task 14
// compiled_config.go) not at the dynamic.Registry layer.
//
// REGRESSION: parent stats.nameRE actually rejects names starting with a
// dot — so the leading-dot path MUST be defended by the consumer. Here
// we verify the behavior — empty pluginScopePrefix passed to NewRegistry
// returns a Registry that yields ErrBadArgument at Register time (the
// internal full-name validation rejects the leading dot).
func TestEnumerateForAdmin_EmptyPluginScope_Permitted(t *testing.T) {
	reg := stats.NewRegistry()
	dyn := NewRegistry(reg, "", 1024)
	if dyn == nil {
		t.Fatalf("NewRegistry with empty pluginScope returned nil; want non-nil")
	}
	// With empty scope, full underlying name would be
	// ".wasmcustom.<name>" — leading dot is rejected by parent
	// stats.nameRE. The Register call surfaces this as ErrBadArgument
	// per the dynamic.Registry's defensive name validation contract.
	_, err := dyn.Register(MetricTypeCounter, "any")
	if err == nil {
		t.Errorf("Register with empty pluginScope err = nil; want ErrBadArgument (leading-dot full name)")
	}
}
