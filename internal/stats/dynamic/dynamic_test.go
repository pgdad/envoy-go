// Tests for the NEW internal/stats/dynamic/ *Registry per 25.2 SPEC §3.3 +
// ADR-0208 + AMEND-B2 + R-25.2-7. Covers:
//
//   - Register/Increment/Record/Get round-trip on Counter + Gauge + Histogram
//     (TestRegistry_Register_*, TestRegistry_Increment_*, TestRegistry_Record_*,
//     TestRegistry_Get_*).
//   - Idempotent re-Register of the same (name,type) returning the cached
//     MetricID + the same underlying stats primitive (TestRegistry_Register_
//     Idempotent_*).
//   - Cross-type re-Register of same name rejected with ErrBadArgument
//     (TestRegistry_Register_CrossType_Rejected).
//   - Signed-int64 delta semantics per AMEND-B2: negative gauge deltas + the
//     int64 extremes (math.MinInt64, math.MaxInt64) — Counter accepts only
//     non-negative deltas because *stats.Counter is monotonic-non-negative
//     (TestRegistry_Increment_Gauge_Negative_*, TestRegistry_Increment_Counter_
//     NegativeDelta_Rejected).
//   - Cross-type operation rejection per AMEND-B2: Increment on Histogram +
//     Record on Counter (TestRegistry_Increment_Histogram_*,
//     TestRegistry_Record_Counter_*).
//   - ErrCapExceeded threshold at maxEntries (TestRegistry_Register_CapBoundary_*).
//   - Post-cap idempotent re-Register of an existing name STILL OK
//     (TestRegistry_Register_PostCap_Idempotent_OK).
//   - ErrNotFound on unknown MetricID at Increment/Record/Get
//     (TestRegistry_*_UnknownID_NotFound).
//   - Invalid user name rejection (empty / leading-dot / disallowed-char)
//     mirroring the parent stats package's validation discipline
//     (TestRegistry_Register_InvalidName_*).
//
// The 25.2 SPEC §3.3 production signatures pin the API shape; the tests
// pin the per-row behavior. The pluginScope construction is materialized
// at Task 14 (compiled_config.go); these tests use a synthetic prefix
// e.g. "wasm.testplugin" matching the §7.3 admin-side enumeration shape.
package dynamic

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// newTestRegistry constructs a Registry for testing with a fresh
// *stats.Registry + a synthetic plugin scope prefix.
func newTestRegistry(t *testing.T, maxEntries uint32) (*Registry, *stats.Registry) {
	t.Helper()
	reg := stats.NewRegistry()
	dyn := NewRegistry(reg, "wasm.testplugin", maxEntries)
	if dyn == nil {
		t.Fatalf("NewRegistry returned nil; want non-nil *Registry")
	}
	return dyn, reg
}

// -----------------------------------------------------------------------------
// Register basics — Counter / Gauge / Histogram round-trip.
// -----------------------------------------------------------------------------

// TestRegistry_Register_Counter verifies Register on a fresh Counter returns
// a non-zero MetricID + nil error.
func TestRegistry_Register_Counter(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeCounter, "my_counter")
	if err != nil {
		t.Fatalf("Register(Counter, my_counter) err = %v; want nil", err)
	}
	if id == 0 {
		t.Errorf("Register(Counter, my_counter) id = 0; want non-zero MetricID")
	}
}

// TestRegistry_Register_Gauge verifies Register on a fresh Gauge returns a
// non-zero MetricID + nil error.
func TestRegistry_Register_Gauge(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeGauge, "my_gauge")
	if err != nil {
		t.Fatalf("Register(Gauge, my_gauge) err = %v; want nil", err)
	}
	if id == 0 {
		t.Errorf("Register(Gauge, my_gauge) id = 0; want non-zero MetricID")
	}
}

// TestRegistry_Register_Histogram verifies Register on a fresh Histogram
// returns a non-zero MetricID + nil error. The histogram primitive at
// 25.2 IMPL is an in-package stub that stores the most recently Record-ed
// value (ADR-0060 deferred full histogram primitive); Get returns that
// value.
func TestRegistry_Register_Histogram(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeHistogram, "my_histogram")
	if err != nil {
		t.Fatalf("Register(Histogram, my_histogram) err = %v; want nil", err)
	}
	if id == 0 {
		t.Errorf("Register(Histogram, my_histogram) id = 0; want non-zero MetricID")
	}
}

// TestRegistry_Register_MetricTypeOutOfRange verifies that a MetricType value
// outside {Counter, Gauge, Histogram} returns ErrBadArgument. Used to defend
// the wire-format byte-pin per AMEND-B2 (proxy-wasm guest can pass any byte
// for the metric_type field).
func TestRegistry_Register_MetricTypeOutOfRange(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	// MetricType=99 is out of range per AMEND-B2 byte-pin (0/1/2).
	_, err := dyn.Register(MetricType(99), "my_name")
	if !errors.Is(err, ErrBadArgument) {
		t.Errorf("Register(MetricType(99), my_name) err = %v; want ErrBadArgument", err)
	}
}

// -----------------------------------------------------------------------------
// Idempotent re-Register — same (name, type) → cached MetricID.
// -----------------------------------------------------------------------------

// TestRegistry_Register_Idempotent_Counter verifies re-Register of the same
// (name, Counter) returns the same MetricID + nil error.
func TestRegistry_Register_Idempotent_Counter(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id1, err := dyn.Register(MetricTypeCounter, "dup_counter")
	if err != nil {
		t.Fatalf("first Register err = %v; want nil", err)
	}
	id2, err := dyn.Register(MetricTypeCounter, "dup_counter")
	if err != nil {
		t.Fatalf("second Register err = %v; want nil", err)
	}
	if id1 != id2 {
		t.Errorf("idempotent Register IDs differ: id1=%d id2=%d; want equal", id1, id2)
	}
}

// TestRegistry_Register_Idempotent_Gauge verifies re-Register of the same
// (name, Gauge) returns the same MetricID + nil error.
func TestRegistry_Register_Idempotent_Gauge(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id1, err := dyn.Register(MetricTypeGauge, "dup_gauge")
	if err != nil {
		t.Fatalf("first Register err = %v; want nil", err)
	}
	id2, err := dyn.Register(MetricTypeGauge, "dup_gauge")
	if err != nil {
		t.Fatalf("second Register err = %v; want nil", err)
	}
	if id1 != id2 {
		t.Errorf("idempotent Register IDs differ: id1=%d id2=%d; want equal", id1, id2)
	}
}

// TestRegistry_Register_Idempotent_Histogram verifies re-Register of the same
// (name, Histogram) returns the same MetricID + nil error.
func TestRegistry_Register_Idempotent_Histogram(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id1, err := dyn.Register(MetricTypeHistogram, "dup_histogram")
	if err != nil {
		t.Fatalf("first Register err = %v; want nil", err)
	}
	id2, err := dyn.Register(MetricTypeHistogram, "dup_histogram")
	if err != nil {
		t.Fatalf("second Register err = %v; want nil", err)
	}
	if id1 != id2 {
		t.Errorf("idempotent Register IDs differ: id1=%d id2=%d; want equal", id1, id2)
	}
}

// -----------------------------------------------------------------------------
// Cross-type re-Register — same name, different type → ErrBadArgument.
// -----------------------------------------------------------------------------

// TestRegistry_Register_CrossType_Rejected verifies that re-registering an
// existing name with a different MetricType returns ErrBadArgument. The
// existing entry is preserved (subsequent Increment/Get on the original
// MetricID continues to work). Mirrors cpp-host upstream behavior where
// same-name-different-type re-Register is undefined; envoy-go FAIL-FAST
// rather than overwrite.
func TestRegistry_Register_CrossType_Rejected(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	idCounter, err := dyn.Register(MetricTypeCounter, "ambiguous")
	if err != nil {
		t.Fatalf("first Register(Counter) err = %v; want nil", err)
	}
	_, err = dyn.Register(MetricTypeGauge, "ambiguous")
	if !errors.Is(err, ErrBadArgument) {
		t.Errorf("Register(Gauge, ambiguous) err = %v; want ErrBadArgument", err)
	}
	// Original counter still intact: Increment + Get round-trip works.
	if err := dyn.Increment(idCounter, 3); err != nil {
		t.Errorf("Increment on original Counter after rejected re-Register err = %v; want nil", err)
	}
	v, err := dyn.Get(idCounter)
	if err != nil {
		t.Fatalf("Get on original Counter err = %v; want nil", err)
	}
	if v != 3 {
		t.Errorf("Get on original Counter v = %d; want 3", v)
	}
}

// -----------------------------------------------------------------------------
// Increment + Get round-trip (Counter).
// -----------------------------------------------------------------------------

// TestRegistry_Increment_Counter_PositiveDelta verifies a positive delta on
// a Counter sums correctly into the Get result.
func TestRegistry_Increment_Counter_PositiveDelta(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeCounter, "c")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Increment(id, 10); err != nil {
		t.Fatalf("Increment err = %v; want nil", err)
	}
	if err := dyn.Increment(id, 5); err != nil {
		t.Fatalf("Increment err = %v; want nil", err)
	}
	v, err := dyn.Get(id)
	if err != nil {
		t.Fatalf("Get err = %v; want nil", err)
	}
	if v != 15 {
		t.Errorf("Get v = %d; want 15", v)
	}
}

// TestRegistry_Increment_Counter_ZeroDelta verifies a zero delta on a Counter
// is a no-op (value unchanged).
func TestRegistry_Increment_Counter_ZeroDelta(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeCounter, "c0")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Increment(id, 0); err != nil {
		t.Fatalf("Increment(0) err = %v; want nil", err)
	}
	v, err := dyn.Get(id)
	if err != nil {
		t.Fatalf("Get err = %v; want nil", err)
	}
	if v != 0 {
		t.Errorf("Get v = %d; want 0", v)
	}
}

// TestRegistry_Increment_Counter_NegativeDelta_Rejected verifies that a
// negative delta on a Counter returns ErrBadArgument (the underlying
// *stats.Counter is monotonic-non-negative; AMEND-B2 signedness applies to
// Gauge — Counter MUST reject negative deltas).
func TestRegistry_Increment_Counter_NegativeDelta_Rejected(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeCounter, "cneg")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Increment(id, -1); !errors.Is(err, ErrBadArgument) {
		t.Errorf("Increment(-1) on Counter err = %v; want ErrBadArgument", err)
	}
	// Original value untouched (still zero).
	v, _ := dyn.Get(id)
	if v != 0 {
		t.Errorf("Get after rejected negative delta v = %d; want 0", v)
	}
}

// -----------------------------------------------------------------------------
// Increment + Get round-trip (Gauge) — signed-int64 delta extremes.
// -----------------------------------------------------------------------------

// TestRegistry_Increment_Gauge_Negative verifies a negative delta on a Gauge
// is permitted per AMEND-B2 (signed int64 delta allows negative gauge updates).
// The Get result is the absolute value's two's-complement reinterpretation
// (uint64(int64(-3)) = uint64(math.MaxUint64 - 2)) — the caller is
// responsible for the signed interpretation per AMEND-B2.
func TestRegistry_Increment_Gauge_Negative(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeGauge, "gneg")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Increment(id, 10); err != nil {
		t.Fatalf("Increment(+10) err = %v; want nil", err)
	}
	if err := dyn.Increment(id, -3); err != nil {
		t.Fatalf("Increment(-3) err = %v; want nil", err)
	}
	v, err := dyn.Get(id)
	if err != nil {
		t.Fatalf("Get err = %v; want nil", err)
	}
	// The Gauge primitive is signed int64 internally; Get returns
	// uint64(int64Value) per the API. Expected: 10 + (-3) = 7.
	if v != 7 {
		t.Errorf("Get after Inc(+10) + Inc(-3) v = %d; want 7", v)
	}
}

// TestRegistry_Increment_Gauge_MinInt64 verifies the int64 minimum delta is
// accepted on a Gauge (AMEND-B2 wire extreme).
func TestRegistry_Increment_Gauge_MinInt64(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeGauge, "gmin")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Increment(id, math.MinInt64); err != nil {
		t.Errorf("Increment(MinInt64) on Gauge err = %v; want nil", err)
	}
}

// TestRegistry_Increment_Gauge_MaxInt64 verifies the int64 maximum delta is
// accepted on a Gauge (AMEND-B2 wire extreme).
func TestRegistry_Increment_Gauge_MaxInt64(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeGauge, "gmax")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Increment(id, math.MaxInt64); err != nil {
		t.Errorf("Increment(MaxInt64) on Gauge err = %v; want nil", err)
	}
}

// -----------------------------------------------------------------------------
// Increment on Histogram → ErrBadArgument.
// -----------------------------------------------------------------------------

// TestRegistry_Increment_Histogram_Rejected verifies that Increment on a
// Histogram-typed metric returns ErrBadArgument (Histograms accept Record,
// not Increment, per AMEND-B2).
func TestRegistry_Increment_Histogram_Rejected(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeHistogram, "h_no_inc")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Increment(id, 1); !errors.Is(err, ErrBadArgument) {
		t.Errorf("Increment on Histogram err = %v; want ErrBadArgument", err)
	}
}

// -----------------------------------------------------------------------------
// Record + Get round-trip (Gauge / Histogram).
// -----------------------------------------------------------------------------

// TestRegistry_Record_Gauge verifies Record sets the gauge value (unsigned
// uint64 per AMEND-B2).
func TestRegistry_Record_Gauge(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeGauge, "gr")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Record(id, 42); err != nil {
		t.Fatalf("Record err = %v; want nil", err)
	}
	v, err := dyn.Get(id)
	if err != nil {
		t.Fatalf("Get err = %v; want nil", err)
	}
	if v != 42 {
		t.Errorf("Get v = %d; want 42", v)
	}
	// Record overwrites (gauge Set semantic).
	if err := dyn.Record(id, 7); err != nil {
		t.Fatalf("Record err = %v; want nil", err)
	}
	v, _ = dyn.Get(id)
	if v != 7 {
		t.Errorf("Get after second Record v = %d; want 7", v)
	}
}

// TestRegistry_Record_Histogram verifies Record on a Histogram-typed metric
// stores the value (in-package stub stores the most recent value per
// ADR-0060 deferred-histogram-primitive disposition).
func TestRegistry_Record_Histogram(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeHistogram, "hr")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Record(id, 100); err != nil {
		t.Fatalf("Record err = %v; want nil", err)
	}
	v, err := dyn.Get(id)
	if err != nil {
		t.Fatalf("Get err = %v; want nil", err)
	}
	if v != 100 {
		t.Errorf("Get v = %d; want 100", v)
	}
}

// TestRegistry_Record_ZeroValue verifies Record(0) is OK.
func TestRegistry_Record_ZeroValue(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeGauge, "gz")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Record(id, 0); err != nil {
		t.Errorf("Record(0) err = %v; want nil", err)
	}
}

// TestRegistry_Record_Counter_Rejected verifies Record on a Counter-typed
// metric returns ErrBadArgument (counters accept Increment, not Record, per
// AMEND-B2).
func TestRegistry_Record_Counter_Rejected(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeCounter, "c_no_record")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	if err := dyn.Record(id, 1); !errors.Is(err, ErrBadArgument) {
		t.Errorf("Record on Counter err = %v; want ErrBadArgument", err)
	}
}

// -----------------------------------------------------------------------------
// Unknown MetricID — Increment / Record / Get → ErrNotFound.
// -----------------------------------------------------------------------------

// TestRegistry_Increment_UnknownID_NotFound verifies Increment on a
// never-registered MetricID returns ErrNotFound.
func TestRegistry_Increment_UnknownID_NotFound(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	if err := dyn.Increment(MetricID(9999), 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("Increment unknown id err = %v; want ErrNotFound", err)
	}
}

// TestRegistry_Record_UnknownID_NotFound verifies Record on a never-registered
// MetricID returns ErrNotFound.
func TestRegistry_Record_UnknownID_NotFound(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	if err := dyn.Record(MetricID(9999), 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("Record unknown id err = %v; want ErrNotFound", err)
	}
}

// TestRegistry_Get_UnknownID_NotFound verifies Get on a never-registered
// MetricID returns ErrNotFound + zero value.
func TestRegistry_Get_UnknownID_NotFound(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	v, err := dyn.Get(MetricID(9999))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get unknown id err = %v; want ErrNotFound", err)
	}
	if v != 0 {
		t.Errorf("Get unknown id v = %d; want 0", v)
	}
}

// -----------------------------------------------------------------------------
// ErrCapExceeded threshold + post-cap idempotent re-Register.
// -----------------------------------------------------------------------------

// TestRegistry_Register_CapBoundary_1024 verifies that Register at the 1024
// maxEntries cap allows entries 1-1024 and rejects entry 1025 with
// ErrCapExceeded. The cap is envoy-go-strict per Q9 + AMEND-B2.
func TestRegistry_Register_CapBoundary_1024(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	for i := 1; i <= 1024; i++ {
		name := fmt.Sprintf("name_%d", i)
		if _, err := dyn.Register(MetricTypeCounter, name); err != nil {
			t.Fatalf("Register #%d (name=%s) err = %v; want nil", i, name, err)
		}
	}
	// 1025th: rejected.
	_, err := dyn.Register(MetricTypeCounter, "overflow")
	if !errors.Is(err, ErrCapExceeded) {
		t.Errorf("Register #1025 err = %v; want ErrCapExceeded", err)
	}
}

// TestRegistry_Register_PostCap_Idempotent_OK verifies that after the cap
// is hit, an idempotent re-Register of an existing name still succeeds
// (the cap check is bypassed on the idempotent path because no new entry
// is allocated).
func TestRegistry_Register_PostCap_Idempotent_OK(t *testing.T) {
	// Smaller cap for test speed.
	dyn, _ := newTestRegistry(t, 4)
	ids := make([]MetricID, 4)
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("name_%d", i)
		id, err := dyn.Register(MetricTypeCounter, name)
		if err != nil {
			t.Fatalf("Register #%d err = %v; want nil", i, err)
		}
		ids[i] = id
	}
	// Cap hit on 5th NEW name.
	if _, err := dyn.Register(MetricTypeCounter, "overflow_new"); !errors.Is(err, ErrCapExceeded) {
		t.Fatalf("Register overflow_new err = %v; want ErrCapExceeded", err)
	}
	// Idempotent re-Register of existing name STILL OK.
	id, err := dyn.Register(MetricTypeCounter, "name_0")
	if err != nil {
		t.Errorf("idempotent Register post-cap err = %v; want nil", err)
	}
	if id != ids[0] {
		t.Errorf("idempotent Register post-cap id = %d; want %d", id, ids[0])
	}
}

// TestRegistry_Register_CapZero_AllRejected verifies that maxEntries=0 means
// the very first Register returns ErrCapExceeded.
func TestRegistry_Register_CapZero_AllRejected(t *testing.T) {
	dyn, _ := newTestRegistry(t, 0)
	if _, err := dyn.Register(MetricTypeCounter, "any"); !errors.Is(err, ErrCapExceeded) {
		t.Errorf("Register at maxEntries=0 err = %v; want ErrCapExceeded", err)
	}
}

// -----------------------------------------------------------------------------
// Invalid user-name rejection.
// -----------------------------------------------------------------------------

// TestRegistry_Register_InvalidName_Empty verifies empty name → ErrBadArgument.
func TestRegistry_Register_InvalidName_Empty(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	if _, err := dyn.Register(MetricTypeCounter, ""); !errors.Is(err, ErrBadArgument) {
		t.Errorf("Register empty name err = %v; want ErrBadArgument", err)
	}
}

// TestRegistry_Register_InvalidName_LeadingDot verifies a name starting with
// "." returns ErrBadArgument (mirrors parent stats.nameRE).
func TestRegistry_Register_InvalidName_LeadingDot(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	if _, err := dyn.Register(MetricTypeCounter, ".leading"); !errors.Is(err, ErrBadArgument) {
		t.Errorf("Register leading-dot name err = %v; want ErrBadArgument", err)
	}
}

// TestRegistry_Register_InvalidName_TrailingDot verifies a name ending with
// "." returns ErrBadArgument (mirrors parent stats.nameRE).
func TestRegistry_Register_InvalidName_TrailingDot(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	if _, err := dyn.Register(MetricTypeCounter, "trailing."); !errors.Is(err, ErrBadArgument) {
		t.Errorf("Register trailing-dot name err = %v; want ErrBadArgument", err)
	}
}

// TestRegistry_Register_InvalidName_SpacesRejected verifies a name with
// whitespace returns ErrBadArgument (mirrors parent stats.nameRE — only
// alphanumeric + underscore + dot allowed).
func TestRegistry_Register_InvalidName_SpacesRejected(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	if _, err := dyn.Register(MetricTypeCounter, "has space"); !errors.Is(err, ErrBadArgument) {
		t.Errorf("Register whitespace name err = %v; want ErrBadArgument", err)
	}
}

// TestRegistry_Register_ValidName_Dotted verifies a name with interior dots
// is accepted (e.g., "subsystem.counter" is a valid hierarchical name).
func TestRegistry_Register_ValidName_Dotted(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	if _, err := dyn.Register(MetricTypeCounter, "subsystem.requests"); err != nil {
		t.Errorf("Register dotted name err = %v; want nil", err)
	}
}

// -----------------------------------------------------------------------------
// NewRegistry edge cases.
// -----------------------------------------------------------------------------

// TestNewRegistry_NilStatsRegistry_ReturnsNil verifies NewRegistry returns
// nil if the *stats.Registry argument is nil (defensive nil-tolerance per
// ADR-0085).
func TestNewRegistry_NilStatsRegistry_ReturnsNil(t *testing.T) {
	dyn := NewRegistry(nil, "wasm.testplugin", 1024)
	if dyn != nil {
		t.Errorf("NewRegistry(nil, ...) = %v; want nil", dyn)
	}
}

// TestRegistry_MetricID_MonotonicallyIncreasing verifies MetricIDs are
// allocated monotonically increasing — important so external consumers can
// rely on the IDs not colliding.
func TestRegistry_MetricID_MonotonicallyIncreasing(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id1, _ := dyn.Register(MetricTypeCounter, "a")
	id2, _ := dyn.Register(MetricTypeCounter, "b")
	id3, _ := dyn.Register(MetricTypeGauge, "c")
	if !(id1 < id2 && id2 < id3) {
		t.Errorf("MetricIDs not monotonic: id1=%d id2=%d id3=%d", id1, id2, id3)
	}
}

// -----------------------------------------------------------------------------
// Sentinel-error wrapping discipline.
// -----------------------------------------------------------------------------

// TestRegistry_Sentinel_Errors_Wrap verifies all returned errors are
// errors.Is-discoverable via the exported sentinels. Callers MUST be able
// to use errors.Is(err, ErrCapExceeded) etc. without parsing strings.
func TestRegistry_Sentinel_Errors_Wrap(t *testing.T) {
	dyn, _ := newTestRegistry(t, 0)
	_, err := dyn.Register(MetricTypeCounter, "any")
	if err == nil {
		t.Fatalf("Register at cap=0 err = nil; want non-nil")
	}
	// errors.Is(err, ErrCapExceeded) must be true (already covered by
	// TestRegistry_Register_CapZero_AllRejected; here we also pin the
	// string-suffix discipline to catch silent rewording).
	if !strings.Contains(err.Error(), "dynamic:") {
		t.Errorf("error string %q lacks expected 'dynamic:' prefix", err.Error())
	}
}

// -----------------------------------------------------------------------------
// Concurrent reads tolerance (cheap test; full stress in
// dynamic_concurrency_test.go).
// -----------------------------------------------------------------------------

// TestRegistry_Concurrent_Read_NoRace verifies that concurrent Get calls
// against a stable Registry are race-free under -race. Used as a smoke
// test; the full RWMutex discipline test lives in
// dynamic_concurrency_test.go.
func TestRegistry_Concurrent_Read_NoRace(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, _ := dyn.Register(MetricTypeCounter, "c")
	_ = dyn.Increment(id, 1)
	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			_, _ = dyn.Get(id)
		}()
	}
	wg.Wait()
}
