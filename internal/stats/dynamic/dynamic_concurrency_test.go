// Tests for the NEW internal/stats/dynamic/ *Registry RWMutex discipline
// + cap-boundary race per 25.2 SPEC §3.3 + ADR-0208 + AMEND-B2.
//
// These tests run under -race. The Registry uses sync.RWMutex to serialize
// the byID/byName map mutations + the nextID/maxEntries cap check; the
// underlying *stats.Counter/Gauge primitives are lock-free atomics in the
// hot Increment/Record path.
//
// Coverage:
//   - N=2000 goroutines racing to Register distinct names with maxEntries=1024
//     → EXACTLY 1024 succeed + 976 return ErrCapExceeded (cap-boundary race
//     correctness).
//   - N=100 goroutines concurrent Increment on the SAME MetricID → the post-
//     stress Get sum matches the deterministic expected total (lock-free
//     atomic Increment via *stats.Counter.Add).
//   - N=100 goroutines all Register the SAME name + type → all return the
//     same MetricID + nil error; the parent stats.Registry has EXACTLY 1
//     underlying Counter (idempotent-Register race correctness).
package dynamic

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// TestRegistry_Concurrent_Register_CapBoundary_Race verifies that with
// N=2000 goroutines racing to Register DISTINCT names against a Registry
// with maxEntries=1024, EXACTLY 1024 succeed + 976 return ErrCapExceeded.
// The race-clean assertion under -race is the load-bearing one; the exact
// count is the cap-boundary correctness assertion.
func TestRegistry_Concurrent_Register_CapBoundary_Race(t *testing.T) {
	const N = 2000
	const cap = 1024
	dyn, _ := newTestRegistry(t, cap)
	var (
		successCount uint32
		capExceeded  uint32
		otherErrs    uint32
		wg           sync.WaitGroup
	)
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("name_%d", i)
			_, err := dyn.Register(MetricTypeCounter, name)
			switch {
			case err == nil:
				atomic.AddUint32(&successCount, 1)
			case errors.Is(err, ErrCapExceeded):
				atomic.AddUint32(&capExceeded, 1)
			default:
				atomic.AddUint32(&otherErrs, 1)
			}
		}()
	}
	wg.Wait()
	if otherErrs != 0 {
		t.Errorf("unexpected non-CapExceeded errors: %d", otherErrs)
	}
	if successCount != cap {
		t.Errorf("successCount = %d; want %d (exact cap)", successCount, cap)
	}
	if capExceeded != N-cap {
		t.Errorf("capExceeded = %d; want %d (N - cap)", capExceeded, N-cap)
	}
}

// TestRegistry_Concurrent_Increment_SameID_Sum verifies that N=100 goroutines
// each calling Increment on the SAME MetricID with delta=1 yield Get=N after
// all goroutines complete (lock-free *stats.Counter.Add semantics under the
// Registry's RWMutex.RLock-bracketed path).
func TestRegistry_Concurrent_Increment_SameID_Sum(t *testing.T) {
	const N = 100
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeCounter, "concurrent_inc")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if err := dyn.Increment(id, 1); err != nil {
				t.Errorf("concurrent Increment err = %v; want nil", err)
			}
		}()
	}
	wg.Wait()
	v, err := dyn.Get(id)
	if err != nil {
		t.Fatalf("post-stress Get err = %v; want nil", err)
	}
	if v != N {
		t.Errorf("post-stress Get v = %d; want %d", v, N)
	}
}

// TestRegistry_Concurrent_Increment_LargeDelta_SameID_Sum verifies the same
// concurrent-Increment correctness with a larger delta per call (the sum
// MUST be N*delta after all goroutines complete).
func TestRegistry_Concurrent_Increment_LargeDelta_SameID_Sum(t *testing.T) {
	const N = 100
	const delta = 7
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeGauge, "concurrent_inc_large")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if err := dyn.Increment(id, delta); err != nil {
				t.Errorf("concurrent Increment err = %v; want nil", err)
			}
		}()
	}
	wg.Wait()
	v, err := dyn.Get(id)
	if err != nil {
		t.Fatalf("post-stress Get err = %v; want nil", err)
	}
	if v != uint64(N*delta) {
		t.Errorf("post-stress Get v = %d; want %d", v, N*delta)
	}
}

// TestRegistry_Concurrent_Idempotent_Register_Race verifies that N=100
// goroutines each calling Register with the SAME (name, Counter) all return
// the SAME MetricID + nil error. After the race the parent stats.Registry
// MUST have EXACTLY 1 underlying *stats.Counter (no duplicate registrations
// under contention).
func TestRegistry_Concurrent_Idempotent_Register_Race(t *testing.T) {
	const N = 100
	dyn, reg := newTestRegistry(t, 1024)
	var (
		ids  = make([]MetricID, N)
		errs uint32
		wg   sync.WaitGroup
	)
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			id, err := dyn.Register(MetricTypeCounter, "shared_name")
			if err != nil {
				atomic.AddUint32(&errs, 1)
				return
			}
			ids[i] = id
		}()
	}
	wg.Wait()
	if errs != 0 {
		t.Errorf("unexpected errors during idempotent Register race: %d", errs)
	}
	// Post-wg join: read ids without race (no in-flight writes). All
	// successful Register calls MUST have returned the SAME MetricID.
	first := ids[0]
	if first == 0 {
		t.Fatalf("ids[0] = 0; want non-zero MetricID")
	}
	for i := 1; i < N; i++ {
		if ids[i] != first {
			t.Errorf("ids[%d] = %d; want %d (all idempotent Register calls must return the same MetricID)", i, ids[i], first)
		}
	}
	// Verify EXACTLY 1 underlying Counter in the parent stats.Registry
	// (no duplicate registrations under contention).
	count := 0
	wantName := "wasm.testplugin.wasmcustom.shared_name"
	reg.Walk(func(m stats.Metric) {
		if m.Name() == wantName {
			count++
		}
	})
	if count != 1 {
		t.Errorf("underlying %q registered %d times; want 1", wantName, count)
	}
}

// TestRegistry_Concurrent_Register_Get_NoRace exercises concurrent Register +
// Get + Increment + EnumerateForAdmin against the same Registry. The race-
// clean assertion under -race is the load-bearing one; no specific final
// state is asserted (the concurrent dispatch makes individual outcomes
// non-deterministic).
func TestRegistry_Concurrent_Register_Get_NoRace(t *testing.T) {
	const N = 50
	dyn, _ := newTestRegistry(t, 4096)
	var wg sync.WaitGroup
	wg.Add(N * 4)
	for i := 0; i < N; i++ {
		i := i
		// Register some names concurrently.
		go func() {
			defer wg.Done()
			_, _ = dyn.Register(MetricTypeCounter, fmt.Sprintf("rg_%d", i%10))
		}()
		// Get on the names (may or may not be registered yet).
		go func() {
			defer wg.Done()
			id, err := dyn.Register(MetricTypeCounter, fmt.Sprintf("rg_%d", i%10))
			if err == nil {
				_, _ = dyn.Get(id)
			}
		}()
		// Increment concurrently.
		go func() {
			defer wg.Done()
			id, err := dyn.Register(MetricTypeCounter, fmt.Sprintf("rg_%d", i%10))
			if err == nil {
				_ = dyn.Increment(id, 1)
			}
		}()
		// EnumerateForAdmin concurrently.
		go func() {
			defer wg.Done()
			dyn.EnumerateForAdmin(func(_ string, _ uint64) {})
		}()
	}
	wg.Wait()
}

// TestRegistry_Concurrent_RecordAndEnumerate_NoRace exercises concurrent
// Record on a Gauge while EnumerateForAdmin reads. Under -race we MUST see
// no data race (the gauge value is read via the lock-free atomic in
// stats.Gauge.Load).
func TestRegistry_Concurrent_RecordAndEnumerate_NoRace(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeGauge, "record_race")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	var wg sync.WaitGroup
	wg.Add(200)
	for i := 0; i < 100; i++ {
		i := i
		go func() {
			defer wg.Done()
			_ = dyn.Record(id, uint64(i))
		}()
		go func() {
			defer wg.Done()
			dyn.EnumerateForAdmin(func(_ string, _ uint64) {})
		}()
	}
	wg.Wait()
}

// TestRegistry_Concurrent_HistogramRecord_NoRace exercises concurrent Record
// on a Histogram-typed metric. Under -race the atomic Store/Load in the
// in-package histogram stub MUST be race-clean.
func TestRegistry_Concurrent_HistogramRecord_NoRace(t *testing.T) {
	dyn, _ := newTestRegistry(t, 1024)
	id, err := dyn.Register(MetricTypeHistogram, "histo_race")
	if err != nil {
		t.Fatalf("Register err = %v; want nil", err)
	}
	var wg sync.WaitGroup
	wg.Add(200)
	for i := 0; i < 100; i++ {
		i := i
		go func() {
			defer wg.Done()
			_ = dyn.Record(id, uint64(i))
		}()
		go func() {
			defer wg.Done()
			_, _ = dyn.Get(id)
		}()
	}
	wg.Wait()
}

// TestRegistry_Concurrent_CapBoundary_NoSpuriousSlot — a regression
// guardrail against an off-by-one where the cap check is performed under
// only the RLock (which would let two goroutines both observe `len < cap`
// and both allocate, exceeding the cap). With Write-Lock-protected
// cap+allocate, the post-race underlying stats.Registry has EXACTLY cap
// entries (no spurious overflow slot).
func TestRegistry_Concurrent_CapBoundary_NoSpuriousSlot(t *testing.T) {
	const cap = 64
	const N = 256
	dyn, reg := newTestRegistry(t, cap)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			_, _ = dyn.Register(MetricTypeCounter, fmt.Sprintf("slot_%d", i))
		}()
	}
	wg.Wait()
	// Walk the parent stats.Registry; count distinct wasmcustom entries.
	count := 0
	wantPrefix := "wasm.testplugin.wasmcustom."
	reg.Walk(func(m stats.Metric) {
		if strings.HasPrefix(m.Name(), wantPrefix) {
			count++
		}
	})
	if count != cap {
		t.Errorf("underlying registered entries = %d; want %d (exact cap; no spurious slot)", count, cap)
	}
}
