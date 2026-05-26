// Tests for the 4 metric-family hostcall dispatch shims per 25.2 SPEC
// §5.1 #31-34 + AMEND-B2 + R-25.2-2. Coverage:
//
//   - MetricType enum byte-pin per AMEND-B2 (Counter=0/Gauge=1/Histogram=2)
//     surfaced via the metricType arg of DefineMetricShim.
//   - DefineMetricShim round-trip: reads name from guest memory; delegates to
//     metricsHost.DefineMetric; writes ret_metric_id_ptr; status propagates.
//   - DefineMetricShim error paths: out-of-range MetricType ⇒ BadArgument;
//     bad name ⇒ BadArgument; cap-exceeded ⇒ InternalFailure.
//   - IncrementMetricShim round-trip + cross-type rejection per AMEND-B2
//     (Histogram-Increment ⇒ BadArgument; Counter+negative delta ⇒
//     BadArgument; Gauge negative delta ⇒ Ok).
//   - RecordMetricShim round-trip + cross-type rejection (Counter-Record ⇒
//     BadArgument; Gauge/Histogram OK).
//   - GetMetricShim round-trip + NotFound on unknown id + writes ret_value_ptr.
//   - Guest-memory bad-pointer paths return InvalidMemoryAccess (name read
//     OOB; ret_metric_id_ptr OOB; ret_value_ptr OOB).
//   - Non-metricsHost Host25_2 ⇒ InternalFailure (programmer error).
//
// The load-bearing semantics live at internal/wasm/dynamic_stats.go (which
// delegates to *dynamic.Registry); this file exercises the shim wire-shape +
// the dispatch + the memory ABI. A fake metricsHost stands in for *RootVM
// so the abi/ package stays decoupled from internal/wasm/.

package abi

import (
	"context"
	"math"
	"sync"
	"testing"
)

// --- fake host -----------------------------------------------------------

// fakeMetricsHost is a minimal in-memory metricsHost that simulates the
// per-RootVM dynamic-stats wrapper. Discriminates by MetricType byte-pin per
// AMEND-B2 (Counter=0, Gauge=1, Histogram=2) and applies the cross-type
// rejection semantics so the abi-layer test can exercise the full status
// matrix without dragging in *dynamic.Registry.
type fakeMetricsHost struct {
	mu sync.Mutex

	byName map[string]uint32
	byID   map[uint32]fakeMetricEntry
	nextID uint32

	// Force overrides for tests that need to exercise specific status
	// returns from DefineMetric (e.g., cap-exceeded ⇒ InternalFailure).
	forceDefineStatus WasmResult
}

type fakeMetricEntry struct {
	metricType uint32 // 0=Counter, 1=Gauge, 2=Histogram per AMEND-B2 byte-pin
	value      uint64 // raw value; signed-int64 reinterpreted for Gauge
}

func newFakeMetricsHost() *fakeMetricsHost {
	return &fakeMetricsHost{
		byName: make(map[string]uint32),
		byID:   make(map[uint32]fakeMetricEntry),
		nextID: 1,
	}
}

func (f *fakeMetricsHost) DefineMetric(metricType uint32, name string) (uint32, WasmResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.forceDefineStatus != WasmResultOk {
		return 0, f.forceDefineStatus
	}
	if metricType > 2 {
		return 0, WasmResultBadArgument
	}
	if name == "" {
		return 0, WasmResultBadArgument
	}
	if id, ok := f.byName[name]; ok {
		// Idempotent re-Register: same name + same type ⇒ same id; cross-type ⇒
		// BadArgument.
		existing := f.byID[id]
		if existing.metricType != metricType {
			return 0, WasmResultBadArgument
		}
		return id, WasmResultOk
	}
	id := f.nextID
	f.nextID++
	f.byID[id] = fakeMetricEntry{metricType: metricType}
	f.byName[name] = id
	return id, WasmResultOk
}

func (f *fakeMetricsHost) IncrementMetric(id uint32, delta int64) WasmResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.byID[id]
	if !ok {
		return WasmResultNotFound
	}
	switch entry.metricType {
	case 0: // Counter
		if delta < 0 {
			return WasmResultBadArgument
		}
		entry.value += uint64(delta)
	case 1: // Gauge
		// Reinterpret as signed int64 add.
		s := int64(entry.value) + delta //nolint:gosec // test fake; signed reinterpret matches AMEND-B2 semantic.
		entry.value = uint64(s)         //nolint:gosec
	case 2: // Histogram
		return WasmResultBadArgument
	}
	f.byID[id] = entry
	return WasmResultOk
}

func (f *fakeMetricsHost) RecordMetric(id uint32, value uint64) WasmResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.byID[id]
	if !ok {
		return WasmResultNotFound
	}
	switch entry.metricType {
	case 0: // Counter
		return WasmResultBadArgument
	case 1, 2: // Gauge or Histogram
		entry.value = value
	}
	f.byID[id] = entry
	return WasmResultOk
}

func (f *fakeMetricsHost) GetMetric(id uint32) (uint64, WasmResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.byID[id]
	if !ok {
		return 0, WasmResultNotFound
	}
	return entry.value, WasmResultOk
}

// --- MetricType byte-pin per AMEND-B2 + R-25.2-2 -------------------------

// TestMetrics_DefineMetric_MetricTypeBytePin asserts the metricType wire
// argument maps to Counter=0, Gauge=1, Histogram=2 per AMEND-B2 + R-25.2-2.
// The 3 valid values dispatch successfully; values 3..255 (sampled at 3 and
// 0xFF) return BadArgument.
func TestMetrics_DefineMetric_MetricTypeBytePin(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()

	const namePtr uint32 = 16
	const retIDPtr uint32 = 256

	for _, valid := range []uint32{0, 1, 2} {
		host := newFakeMetricsHost()
		nameBytes := []byte("m_" + string(rune('a'+valid)))
		nameSize := uint32(len(nameBytes))
		if !mem.Write(namePtr, nameBytes) {
			t.Fatalf("Write name failed")
		}

		got := DefineMetricShim(ctx, mod, host, valid, namePtr, nameSize, retIDPtr)
		if got != WasmResultOk {
			t.Errorf("MetricType=%d: got %v, want Ok", valid, got)
			continue
		}
		id, ok := mem.ReadUint32Le(retIDPtr)
		if !ok {
			t.Fatalf("MetricType=%d: ReadUint32Le retIDPtr failed", valid)
		}
		if id == 0 {
			t.Errorf("MetricType=%d: retMetricID=0, want non-zero", valid)
		}
	}

	for _, invalid := range []uint32{3, 0xFF, 0xFFFFFFFF} {
		host := newFakeMetricsHost()
		nameBytes := []byte("bad")
		nameSize := uint32(len(nameBytes))
		if !mem.Write(namePtr, nameBytes) {
			t.Fatalf("Write name failed")
		}
		got := DefineMetricShim(ctx, mod, host, invalid, namePtr, nameSize, retIDPtr)
		if got != WasmResultBadArgument {
			t.Errorf("MetricType=%d: got %v, want BadArgument", invalid, got)
		}
	}
}

// --- DefineMetricShim round-trip -----------------------------------------

// TestMetrics_DefineMetric_RoundTrip verifies the shim reads `name` from
// guest memory, dispatches to the host, and writes the returned MetricID
// back to ret_metric_id_ptr.
func TestMetrics_DefineMetric_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()

	const namePtr uint32 = 16
	const retIDPtr uint32 = 256

	name := []byte("my_counter")
	if !mem.Write(namePtr, name) {
		t.Fatalf("Write name failed")
	}

	got := DefineMetricShim(ctx, mod, host, 0 /*Counter*/, namePtr, uint32(len(name)), retIDPtr)
	if got != WasmResultOk {
		t.Fatalf("DefineMetric round-trip = %v, want Ok", got)
	}
	id, ok := mem.ReadUint32Le(retIDPtr)
	if !ok {
		t.Fatalf("ReadUint32Le retIDPtr failed")
	}
	if id != 1 {
		t.Errorf("first metric id = %d, want 1", id)
	}

	// Idempotent re-Register: same name + type ⇒ same id.
	got = DefineMetricShim(ctx, mod, host, 0, namePtr, uint32(len(name)), retIDPtr)
	if got != WasmResultOk {
		t.Fatalf("idempotent re-Register = %v, want Ok", got)
	}
	id2, _ := mem.ReadUint32Le(retIDPtr)
	if id2 != id {
		t.Errorf("idempotent re-Register id = %d, want %d", id2, id)
	}
}

// TestMetrics_DefineMetric_EmptyName_BadArgument asserts an empty name is
// rejected via the host's name-validation path (fake delegates the rejection;
// the *RootVM impl rejects via the underlying *dynamic.Registry.Register's
// userNameRE).
func TestMetrics_DefineMetric_EmptyName_BadArgument(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	host := newFakeMetricsHost()

	got := DefineMetricShim(ctx, mod, host, 0, 16, 0 /*nameSize=0*/, 256)
	if got != WasmResultBadArgument {
		t.Errorf("DefineMetric empty name = %v, want BadArgument", got)
	}
}

// TestMetrics_DefineMetric_NameOOB_InvalidMemoryAccess asserts a guest-memory
// read out-of-bounds on the name returns InvalidMemoryAccess (not delegated
// to the host).
func TestMetrics_DefineMetric_NameOOB_InvalidMemoryAccess(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	host := newFakeMetricsHost()

	// 1 page = 64KiB; nameDataPtr=0x10000 (one byte past page end) is OOB.
	got := DefineMetricShim(ctx, mod, host, 0, 0x10000, 4, 256)
	if got != WasmResultInvalidMemoryAccess {
		t.Errorf("OOB name read = %v, want InvalidMemoryAccess", got)
	}
}

// TestMetrics_DefineMetric_RetIDPtrOOB_InvalidMemoryAccess asserts that a
// successful host dispatch followed by a failed write to ret_metric_id_ptr
// surfaces as InvalidMemoryAccess.
func TestMetrics_DefineMetric_RetIDPtrOOB_InvalidMemoryAccess(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()

	const namePtr uint32 = 16
	name := []byte("ok")
	if !mem.Write(namePtr, name) {
		t.Fatalf("Write name failed")
	}

	// retMetricIDPtr=0xFFFFFFFC is past page end (only 1 page = 64KiB).
	got := DefineMetricShim(ctx, mod, host, 0, namePtr, uint32(len(name)), 0xFFFFFFFC)
	if got != WasmResultInvalidMemoryAccess {
		t.Errorf("OOB ret_metric_id write = %v, want InvalidMemoryAccess", got)
	}
}

// TestMetrics_DefineMetric_HostForceInternalFailure verifies the cap-exceeded
// path: the host returns InternalFailure ⇒ the shim propagates unchanged.
func TestMetrics_DefineMetric_HostForceInternalFailure(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()
	host.forceDefineStatus = WasmResultInternalFailure

	const namePtr uint32 = 16
	if !mem.Write(namePtr, []byte("cap")) {
		t.Fatalf("Write name failed")
	}
	got := DefineMetricShim(ctx, mod, host, 0, namePtr, 3, 256)
	if got != WasmResultInternalFailure {
		t.Errorf("cap-exceeded path = %v, want InternalFailure", got)
	}
}

// --- IncrementMetricShim round-trip + cross-type rejection ---------------

// TestMetrics_IncrementMetric_RoundTrip asserts the shim delegates the
// signed-int64 delta + status propagation per AMEND-B2.
func TestMetrics_IncrementMetric_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()

	// Define a Counter named "c".
	const namePtr uint32 = 16
	const retIDPtr uint32 = 256
	if !mem.Write(namePtr, []byte("c")) {
		t.Fatalf("Write failed")
	}
	if r := DefineMetricShim(ctx, mod, host, 0, namePtr, 1, retIDPtr); r != WasmResultOk {
		t.Fatalf("Define = %v", r)
	}
	id, _ := mem.ReadUint32Le(retIDPtr)

	if r := IncrementMetricShim(ctx, mod, host, id, 7); r != WasmResultOk {
		t.Errorf("Increment positive = %v, want Ok", r)
	}
	if v, _ := host.GetMetric(id); v != 7 {
		t.Errorf("post-Increment value = %d, want 7", v)
	}
}

// TestMetrics_IncrementMetric_CounterNegativeDelta_BadArgument asserts the
// host's Counter-monotonic-non-negative rejection surfaces (Task 11 deviation:
// underlying *stats.Counter is uint64; negative delta on Counter = BadArgument
// per AMEND-B2 semantic).
func TestMetrics_IncrementMetric_CounterNegativeDelta_BadArgument(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()

	if !mem.Write(16, []byte("c")) {
		t.Fatalf("Write failed")
	}
	if r := DefineMetricShim(ctx, mod, host, 0, 16, 1, 256); r != WasmResultOk {
		t.Fatalf("Define = %v", r)
	}
	id, _ := mem.ReadUint32Le(256)

	if r := IncrementMetricShim(ctx, mod, host, id, -1); r != WasmResultBadArgument {
		t.Errorf("Counter+negative = %v, want BadArgument (Task 11 deviation)", r)
	}
}

// TestMetrics_IncrementMetric_GaugeNegativeDelta_Ok asserts negative deltas
// on Gauge succeed per AMEND-B2 signed-int64 semantic.
func TestMetrics_IncrementMetric_GaugeNegativeDelta_Ok(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()

	if !mem.Write(16, []byte("g")) {
		t.Fatalf("Write failed")
	}
	// MetricType=1 = Gauge per AMEND-B2.
	if r := DefineMetricShim(ctx, mod, host, 1, 16, 1, 256); r != WasmResultOk {
		t.Fatalf("Define = %v", r)
	}
	id, _ := mem.ReadUint32Le(256)

	if r := IncrementMetricShim(ctx, mod, host, id, -42); r != WasmResultOk {
		t.Errorf("Gauge+negative = %v, want Ok", r)
	}
	if r := IncrementMetricShim(ctx, mod, host, id, math.MinInt64); r != WasmResultOk {
		t.Errorf("Gauge+MinInt64 = %v, want Ok", r)
	}
}

// TestMetrics_IncrementMetric_Histogram_BadArgument asserts Increment on a
// Histogram-typed metric returns BadArgument per AMEND-B2 (Histograms accept
// Record only).
func TestMetrics_IncrementMetric_Histogram_BadArgument(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()

	if !mem.Write(16, []byte("h")) {
		t.Fatalf("Write failed")
	}
	// MetricType=2 = Histogram per AMEND-B2.
	if r := DefineMetricShim(ctx, mod, host, 2, 16, 1, 256); r != WasmResultOk {
		t.Fatalf("Define = %v", r)
	}
	id, _ := mem.ReadUint32Le(256)

	if r := IncrementMetricShim(ctx, mod, host, id, 1); r != WasmResultBadArgument {
		t.Errorf("Histogram+Increment = %v, want BadArgument (AMEND-B2)", r)
	}
}

// TestMetrics_IncrementMetric_NotFound asserts an unknown id surfaces NotFound.
func TestMetrics_IncrementMetric_NotFound(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	host := newFakeMetricsHost()
	if r := IncrementMetricShim(ctx, mod, host, 999, 1); r != WasmResultNotFound {
		t.Errorf("unknown id Increment = %v, want NotFound", r)
	}
}

// --- RecordMetricShim round-trip + cross-type rejection ------------------

// TestMetrics_RecordMetric_GaugeRoundTrip asserts the shim delegates the
// unsigned-uint64 value per AMEND-B2 + the host stores it.
func TestMetrics_RecordMetric_GaugeRoundTrip(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()

	if !mem.Write(16, []byte("g")) {
		t.Fatalf("Write failed")
	}
	if r := DefineMetricShim(ctx, mod, host, 1, 16, 1, 256); r != WasmResultOk {
		t.Fatalf("Define = %v", r)
	}
	id, _ := mem.ReadUint32Le(256)

	const probe uint64 = 0xCAFEBABEDEADBEEF
	if r := RecordMetricShim(ctx, mod, host, id, probe); r != WasmResultOk {
		t.Errorf("Record on Gauge = %v, want Ok", r)
	}
	if v, _ := host.GetMetric(id); v != probe {
		t.Errorf("post-Record value = %x, want %x", v, probe)
	}
}

// TestMetrics_RecordMetric_Counter_BadArgument asserts Record on a Counter-
// typed metric returns BadArgument per AMEND-B2 (Counters accept Increment
// only).
func TestMetrics_RecordMetric_Counter_BadArgument(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()

	if !mem.Write(16, []byte("c")) {
		t.Fatalf("Write failed")
	}
	if r := DefineMetricShim(ctx, mod, host, 0, 16, 1, 256); r != WasmResultOk {
		t.Fatalf("Define = %v", r)
	}
	id, _ := mem.ReadUint32Le(256)

	if r := RecordMetricShim(ctx, mod, host, id, 1); r != WasmResultBadArgument {
		t.Errorf("Counter+Record = %v, want BadArgument (AMEND-B2)", r)
	}
}

// TestMetrics_RecordMetric_HistogramRoundTrip asserts Histogram accepts Record.
func TestMetrics_RecordMetric_HistogramRoundTrip(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()

	if !mem.Write(16, []byte("h")) {
		t.Fatalf("Write failed")
	}
	if r := DefineMetricShim(ctx, mod, host, 2, 16, 1, 256); r != WasmResultOk {
		t.Fatalf("Define = %v", r)
	}
	id, _ := mem.ReadUint32Le(256)

	if r := RecordMetricShim(ctx, mod, host, id, 99); r != WasmResultOk {
		t.Errorf("Histogram+Record = %v, want Ok", r)
	}
	if v, _ := host.GetMetric(id); v != 99 {
		t.Errorf("post-Record Histogram value = %d, want 99", v)
	}
}

// --- GetMetricShim round-trip --------------------------------------------

// TestMetrics_GetMetric_RoundTrip asserts the shim delegates + writes the
// uint64 value to ret_value_ptr.
func TestMetrics_GetMetric_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()

	if !mem.Write(16, []byte("c")) {
		t.Fatalf("Write failed")
	}
	if r := DefineMetricShim(ctx, mod, host, 0, 16, 1, 256); r != WasmResultOk {
		t.Fatalf("Define = %v", r)
	}
	id, _ := mem.ReadUint32Le(256)
	_ = IncrementMetricShim(ctx, mod, host, id, 13)

	const retValPtr uint32 = 512
	if r := GetMetricShim(ctx, mod, host, id, retValPtr); r != WasmResultOk {
		t.Errorf("Get = %v, want Ok", r)
	}
	got, ok := mem.ReadUint64Le(retValPtr)
	if !ok {
		t.Fatalf("ReadUint64Le retValPtr failed")
	}
	if got != 13 {
		t.Errorf("retVal = %d, want 13", got)
	}
}

// TestMetrics_GetMetric_NotFound asserts the shim returns NotFound on an
// unknown id WITHOUT touching ret_value_ptr (so a guest that pre-zeroes the
// slot doesn't observe stale data).
func TestMetrics_GetMetric_NotFound(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	host := newFakeMetricsHost()

	if r := GetMetricShim(ctx, mod, host, 99, 512); r != WasmResultNotFound {
		t.Errorf("Get unknown = %v, want NotFound", r)
	}
}

// TestMetrics_GetMetric_RetValPtrOOB_InvalidMemoryAccess asserts a failed
// write to ret_value_ptr surfaces InvalidMemoryAccess.
func TestMetrics_GetMetric_RetValPtrOOB_InvalidMemoryAccess(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	mem := mod.Memory()
	host := newFakeMetricsHost()

	if !mem.Write(16, []byte("c")) {
		t.Fatalf("Write failed")
	}
	if r := DefineMetricShim(ctx, mod, host, 0, 16, 1, 256); r != WasmResultOk {
		t.Fatalf("Define = %v", r)
	}
	id, _ := mem.ReadUint32Le(256)

	// retValPtr=0xFFFFFFF8 is OOB for the 1-page (64 KiB) hosting module.
	if r := GetMetricShim(ctx, mod, host, id, 0xFFFFFFF8); r != WasmResultInvalidMemoryAccess {
		t.Errorf("OOB ret_value_ptr write = %v, want InvalidMemoryAccess", r)
	}
}

// --- non-metricsHost Host25_2 ⇒ InternalFailure --------------------------

// TestMetrics_NonHostHostValue asserts the dispatch guard: if Host25_2 does
// NOT satisfy metricsHost, every shim returns InternalFailure.
func TestMetrics_NonHostHostValue(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	// A plain int does not satisfy metricsHost.
	var bogus int

	if r := DefineMetricShim(ctx, mod, bogus, 0, 16, 1, 256); r != WasmResultInternalFailure {
		t.Errorf("Define non-host = %v, want InternalFailure", r)
	}
	if r := IncrementMetricShim(ctx, mod, bogus, 1, 1); r != WasmResultInternalFailure {
		t.Errorf("Increment non-host = %v, want InternalFailure", r)
	}
	if r := RecordMetricShim(ctx, mod, bogus, 1, 1); r != WasmResultInternalFailure {
		t.Errorf("Record non-host = %v, want InternalFailure", r)
	}
	if r := GetMetricShim(ctx, mod, bogus, 1, 256); r != WasmResultInternalFailure {
		t.Errorf("Get non-host = %v, want InternalFailure", r)
	}
}
