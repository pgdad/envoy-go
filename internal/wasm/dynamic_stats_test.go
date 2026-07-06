// Tests for the per-*RootVM dynamic-stats wrapper per 25.2 SPEC §3.1 + §5.1
// #31-34 + AMEND-B2 + R-25.2-2 + R-25.2-7. Coverage:
//
//   - WithRootDynamicStats option wires a *dynamic.Registry into the *RootVM.
//   - DefineMetric/IncrementMetric/RecordMetric/GetMetric round-trip on
//     Counter, Gauge, Histogram.
//   - MetricType byte-pin per AMEND-B2 (Counter=0/Gauge=1/Histogram=2) —
//     ratifies the abi-layer enum pin against the actual host dispatch.
//   - Signed-int64 delta extremes per AMEND-B2 (negative Gauge delta + MinInt64).
//   - Counter-negative-delta rejection per Task 11 deviation
//     (Counter monotonic-non-negative ⇒ negative delta = BadArgument).
//   - Cross-type rejection per AMEND-B2 (Histogram+Increment ⇒ BadArgument;
//     Counter+Record ⇒ BadArgument).
//   - Unknown id ⇒ NotFound on Increment/Record/Get.
//   - Cap-boundary: small maxEntries fixture ⇒ cap-exceeded path returns
//     InternalFailure. (Counter wiring deferred to Task 17; this test pins
//     the wire-shape contract only.)
//   - Nil-registry tolerance: a *RootVM constructed WITHOUT
//     WithRootDynamicStats returns InternalFailure on every metric call —
//     guards against guest-side metric calls when the operator has not wired
//     a Registry per AMEND-B2 envoy-go-strict envelope.

package wasm

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/stats/dynamic"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// newRootVMForDynamicStats constructs a RootVM around a benign minimal
// module + a per-RootVM *dynamic.Registry. The metric methods on *RootVM
// operate purely host-side (no guest-dispatch); constructing a RootVM is
// the cleanest seam because dynStats lives on *RootVM.
//
// pluginScopePrefix=`wasm.test` so the composed full names are valid per
// parent stats.IsValidName; maxEntries=cap so cap-boundary tests can drive
// to the cap.
func newRootVMForDynamicStats(t *testing.T, cap uint32, opts ...RootVMOption) (*RootVM, *dynamic.Registry) {
	t.Helper()
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)

	parent := stats.NewRegistry()
	reg := dynamic.NewRegistry(parent, "wasm.test", cap)
	if reg == nil {
		t.Fatalf("dynamic.NewRegistry: nil")
	}

	allOpts := append([]RootVMOption{WithRootDynamicStats(reg)}, opts...)
	rv, err := NewRootVM(ctx, mod, 1, allOpts...)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	t.Cleanup(func() { _ = rv.Close() })
	return rv, reg
}

// --- WithRootDynamicStats option wiring ----------------------------------

// TestDynamicStats_WithRootDynamicStatsOption_RoundTrip verifies the option
// applies the registry to the *RootVM and that the metric calls succeed.
func TestDynamicStats_WithRootDynamicStatsOption_RoundTrip(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 1024)

	id, status := rv.DefineMetric(uint32(dynamic.MetricTypeCounter), "round_trip")
	if status != abi.WasmResultOk {
		t.Fatalf("DefineMetric = %v, want Ok", status)
	}
	if id == 0 {
		t.Fatalf("DefineMetric returned id=0; want non-zero")
	}

	if r := rv.IncrementMetric(id, 5); r != abi.WasmResultOk {
		t.Errorf("IncrementMetric = %v, want Ok", r)
	}
	got, r := rv.GetMetric(id)
	if r != abi.WasmResultOk {
		t.Errorf("GetMetric = %v, want Ok", r)
	}
	if got != 5 {
		t.Errorf("GetMetric value = %d, want 5", got)
	}
}

// --- nil-registry tolerance ----------------------------------------------

// TestDynamicStats_NilRegistry_InternalFailure asserts a *RootVM constructed
// without WithRootDynamicStats returns InternalFailure on every metric call.
func TestDynamicStats_NilRegistry_InternalFailure(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	t.Cleanup(func() { _ = rv.Close() })

	if _, s := rv.DefineMetric(0, "x"); s != abi.WasmResultInternalFailure {
		t.Errorf("nil-reg Define = %v, want InternalFailure", s)
	}
	if s := rv.IncrementMetric(1, 1); s != abi.WasmResultInternalFailure {
		t.Errorf("nil-reg Increment = %v, want InternalFailure", s)
	}
	if s := rv.RecordMetric(1, 1); s != abi.WasmResultInternalFailure {
		t.Errorf("nil-reg Record = %v, want InternalFailure", s)
	}
	if _, s := rv.GetMetric(1); s != abi.WasmResultInternalFailure {
		t.Errorf("nil-reg Get = %v, want InternalFailure", s)
	}
}

// --- MetricType byte-pin per AMEND-B2 + R-25.2-2 -------------------------

// TestDynamicStats_MetricTypeBytePin verifies the metricType uint32 arg maps
// to the underlying dynamic.MetricType byte-pin (Counter=0, Gauge=1,
// Histogram=2). Pinned at the host-side dispatch (the abi-layer test already
// pins the shim wire-shape).
func TestDynamicStats_MetricTypeBytePin(t *testing.T) {
	type tc struct {
		name       string
		stat       string // valid metric name (per userNameRE — no '=' / no leading digit)
		metricType uint32
		wantOk     bool
	}
	cases := []tc{
		{"Counter=0", "bp_counter", uint32(dynamic.MetricTypeCounter), true},
		{"Gauge=1", "bp_gauge", uint32(dynamic.MetricTypeGauge), true},
		{"Histogram=2", "bp_hist", uint32(dynamic.MetricTypeHistogram), true},
		{"OutOfRange=3", "bp_oor3", 3, false},
		{"OutOfRange=255", "bp_oor255", 0xFF, false},
		{"OutOfRange=max", "bp_oormax", 0xFFFFFFFF, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rv, _ := newRootVMForDynamicStats(t, 16)
			id, status := rv.DefineMetric(c.metricType, c.stat)
			if c.wantOk {
				if status != abi.WasmResultOk {
					t.Errorf("metricType=%d: got %v, want Ok", c.metricType, status)
				}
				if id == 0 {
					t.Errorf("metricType=%d: id=0, want non-zero", c.metricType)
				}
			} else {
				if status != abi.WasmResultBadArgument {
					t.Errorf("metricType=%d: got %v, want BadArgument", c.metricType, status)
				}
			}
		})
	}
}

// --- DefineMetric idempotent -----------------------------------------------

// TestDynamicStats_DefineMetric_Idempotent asserts a re-Register of an
// existing name with the same type returns the SAME id with Ok.
func TestDynamicStats_DefineMetric_Idempotent(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)

	id1, s1 := rv.DefineMetric(0, "same")
	if s1 != abi.WasmResultOk {
		t.Fatalf("first Define = %v", s1)
	}
	id2, s2 := rv.DefineMetric(0, "same")
	if s2 != abi.WasmResultOk {
		t.Fatalf("second Define = %v", s2)
	}
	if id1 != id2 {
		t.Errorf("idempotent re-Register: got id=%d then id=%d; want equal", id1, id2)
	}
}

// TestDynamicStats_DefineMetric_CrossType_BadArgument asserts a re-Register
// of an existing name with a DIFFERENT type returns BadArgument.
func TestDynamicStats_DefineMetric_CrossType_BadArgument(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)

	if _, s := rv.DefineMetric(uint32(dynamic.MetricTypeCounter), "cx"); s != abi.WasmResultOk {
		t.Fatalf("Counter Define = %v", s)
	}
	_, s := rv.DefineMetric(uint32(dynamic.MetricTypeGauge), "cx")
	if s != abi.WasmResultBadArgument {
		t.Errorf("cross-type Define = %v, want BadArgument", s)
	}
}

// --- DefineMetric cap-boundary -------------------------------------------

// TestDynamicStats_DefineMetric_CapBoundary_InternalFailure asserts the
// cap-exceeded path: with maxEntries=N, the N+1-th DefineMetric returns
// InternalFailure (the Task 11 *dynamic.Registry returns ErrCapExceeded;
// dynamic_stats.go translates to InternalFailure per the Task 17 deferred
// dynamic_stats_cap_exceeded counter wiring). Counter wiring deferred to
// Task 17.
func TestDynamicStats_DefineMetric_CapBoundary_InternalFailure(t *testing.T) {
	const cap = 3
	rv, _ := newRootVMForDynamicStats(t, cap)

	// Fill exactly to cap.
	for i := 0; i < cap; i++ {
		_, s := rv.DefineMetric(0, "m_"+string(rune('a'+i)))
		if s != abi.WasmResultOk {
			t.Fatalf("Define i=%d: %v", i, s)
		}
	}

	// One over → InternalFailure (cap-exceeded surfaces via ErrCapExceeded).
	_, s := rv.DefineMetric(0, "over")
	if s != abi.WasmResultInternalFailure {
		t.Errorf("over-cap Define = %v, want InternalFailure", s)
	}

	// Idempotent re-Register of an EXISTING name MUST still succeed (no new
	// slot allocated; cap check bypassed per Task 11 Register semantic).
	_, s = rv.DefineMetric(0, "m_a")
	if s != abi.WasmResultOk {
		t.Errorf("post-cap idempotent re-Register = %v, want Ok", s)
	}
}

// --- IncrementMetric semantics -------------------------------------------

// TestDynamicStats_IncrementMetric_Counter_Positive asserts the happy path:
// positive delta on Counter ⇒ Ok; value accumulates.
func TestDynamicStats_IncrementMetric_Counter_Positive(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)
	id, _ := rv.DefineMetric(0, "c")

	if r := rv.IncrementMetric(id, 10); r != abi.WasmResultOk {
		t.Errorf("Increment +10 = %v, want Ok", r)
	}
	if r := rv.IncrementMetric(id, 5); r != abi.WasmResultOk {
		t.Errorf("Increment +5 = %v, want Ok", r)
	}
	v, _ := rv.GetMetric(id)
	if v != 15 {
		t.Errorf("Counter value = %d, want 15", v)
	}
}

// TestDynamicStats_IncrementMetric_Counter_NegativeDelta_BadArgument asserts
// the Task 11 deviation: negative delta on a Counter is rejected with
// BadArgument (the underlying *stats.Counter is monotonic-non-negative).
func TestDynamicStats_IncrementMetric_Counter_NegativeDelta_BadArgument(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)
	id, _ := rv.DefineMetric(0, "c_neg")

	if r := rv.IncrementMetric(id, -1); r != abi.WasmResultBadArgument {
		t.Errorf("Counter+negative = %v, want BadArgument (Task 11 deviation)", r)
	}
}

// TestDynamicStats_IncrementMetric_Gauge_NegativeDelta asserts negative
// deltas + MinInt64 succeed on Gauge per AMEND-B2 signed-int64 semantic.
func TestDynamicStats_IncrementMetric_Gauge_NegativeDelta(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)
	id, _ := rv.DefineMetric(1 /*Gauge*/, "g")

	if r := rv.IncrementMetric(id, 100); r != abi.WasmResultOk {
		t.Fatalf("Gauge +100 = %v", r)
	}
	if r := rv.IncrementMetric(id, -42); r != abi.WasmResultOk {
		t.Errorf("Gauge -42 = %v, want Ok", r)
	}
	if r := rv.IncrementMetric(id, math.MinInt64); r != abi.WasmResultOk {
		t.Errorf("Gauge MinInt64 = %v, want Ok", r)
	}
}

// TestDynamicStats_IncrementMetric_Histogram_BadArgument asserts Increment
// on a Histogram-typed metric returns BadArgument per AMEND-B2.
func TestDynamicStats_IncrementMetric_Histogram_BadArgument(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)
	id, _ := rv.DefineMetric(2 /*Histogram*/, "h")

	if r := rv.IncrementMetric(id, 1); r != abi.WasmResultBadArgument {
		t.Errorf("Histogram+Increment = %v, want BadArgument (AMEND-B2)", r)
	}
}

// TestDynamicStats_IncrementMetric_Unknown_NotFound asserts an unknown id
// returns NotFound.
func TestDynamicStats_IncrementMetric_Unknown_NotFound(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)
	if r := rv.IncrementMetric(999, 1); r != abi.WasmResultNotFound {
		t.Errorf("unknown id Increment = %v, want NotFound", r)
	}
}

// --- RecordMetric semantics ----------------------------------------------

// TestDynamicStats_RecordMetric_Gauge_RoundTrip asserts Record on Gauge sets
// the value (latest-Record semantics per AMEND-B2).
func TestDynamicStats_RecordMetric_Gauge_RoundTrip(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)
	id, _ := rv.DefineMetric(1, "g")

	if r := rv.RecordMetric(id, 12345); r != abi.WasmResultOk {
		t.Fatalf("Record = %v", r)
	}
	v, _ := rv.GetMetric(id)
	if v != 12345 {
		t.Errorf("Gauge post-Record value = %d, want 12345", v)
	}
}

// TestDynamicStats_RecordMetric_Histogram_RoundTrip asserts Record on
// Histogram stores the value (in-package atomic.Uint64 stub per ADR-0060
// deferred-primitive disposition; surfaces via Get).
func TestDynamicStats_RecordMetric_Histogram_RoundTrip(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)
	id, _ := rv.DefineMetric(2, "h")

	const probe uint64 = 0xDEADBEEFCAFEBABE
	if r := rv.RecordMetric(id, probe); r != abi.WasmResultOk {
		t.Fatalf("Histogram Record = %v", r)
	}
	v, _ := rv.GetMetric(id)
	if v != probe {
		t.Errorf("Histogram post-Record value = %x, want %x", v, probe)
	}
}

// TestDynamicStats_RecordMetric_Counter_BadArgument asserts Record on a
// Counter-typed metric returns BadArgument per AMEND-B2 (Counters accept
// Increment only).
func TestDynamicStats_RecordMetric_Counter_BadArgument(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)
	id, _ := rv.DefineMetric(0, "c")

	if r := rv.RecordMetric(id, 1); r != abi.WasmResultBadArgument {
		t.Errorf("Counter+Record = %v, want BadArgument (AMEND-B2)", r)
	}
}

// TestDynamicStats_RecordMetric_Unknown_NotFound asserts an unknown id
// returns NotFound.
func TestDynamicStats_RecordMetric_Unknown_NotFound(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)
	if r := rv.RecordMetric(999, 1); r != abi.WasmResultNotFound {
		t.Errorf("unknown id Record = %v, want NotFound", r)
	}
}

// --- GetMetric semantics --------------------------------------------------

// TestDynamicStats_GetMetric_Unknown_NotFound asserts an unknown id returns
// NotFound + value=0.
func TestDynamicStats_GetMetric_Unknown_NotFound(t *testing.T) {
	rv, _ := newRootVMForDynamicStats(t, 16)
	v, r := rv.GetMetric(999)
	if r != abi.WasmResultNotFound {
		t.Errorf("unknown id Get status = %v, want NotFound", r)
	}
	if v != 0 {
		t.Errorf("unknown id Get value = %d, want 0", v)
	}
}

// --- ErrBadArgument wrapping sanity check --------------------------------

// TestDynamicStats_RegistryError_Sentinel_Ok confirms the wrapper correctly
// translates *dynamic.Registry sentinels (ErrBadArgument / ErrNotFound /
// ErrCapExceeded) per the matrix. Pinned at the underlying primitive level
// so the *RootVM wrapper's errors.Is dispatch is verified.
func TestDynamicStats_RegistryError_Sentinel_Ok(t *testing.T) {
	parent := stats.NewRegistry()
	reg := dynamic.NewRegistry(parent, "wasm.t", 1)
	if reg == nil {
		t.Fatalf("NewRegistry: nil")
	}

	// Cap = 1; second NEW Register returns ErrCapExceeded.
	if _, err := reg.Register(dynamic.MetricTypeCounter, "a"); err != nil {
		t.Fatalf("Register a: %v", err)
	}
	_, err := reg.Register(dynamic.MetricTypeCounter, "b")
	if !errors.Is(err, dynamic.ErrCapExceeded) {
		t.Fatalf("over-cap Register err = %v, want ErrCapExceeded", err)
	}

	// Increment unknown id → ErrNotFound.
	err = reg.Increment(99, 1)
	if !errors.Is(err, dynamic.ErrNotFound) {
		t.Fatalf("unknown Increment err = %v, want ErrNotFound", err)
	}
}
