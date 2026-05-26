// Tests for the per-*RootVM CAS-protected shared-data K-V map per Q6 +
// R-25.2-10 + 25.2 SPEC §3.1 shared_data.go + §5.1 #35-36.
//
// Coverage (golden table per R-25.2-10):
//
//	(a) Set("k", v, cas=0)  — new entry, returns Ok; subsequent Get returns (v, 1, Ok)
//	(b) Set("k", v2, cas=1) — match, returns Ok; Get returns (v2, 2, Ok)
//	(c) Set("k", v3, cas=99)— mismatch, returns CasMismatch; Get returns (v2, 2, Ok)
//	(d) Set("k", v4, cas=0) — unconditional, returns Ok; Get returns (v4, 3, Ok)
//	(e) Get("nonexistent")  — returns (nil, 0, NotFound)
//	(f) Entry-cap boundary  — 1024 entries Ok; 1025th returns InternalFailure
//	(g) Value-cap boundary  — value > 1 MiB returns InternalFailure
//	(h) Concurrent N=100    — independent Set("kN", vN, cas=0); all Ok; -race clean
//
// The cap-default semantic (valCap==0 → 1 MiB; maxEntries==0 → 1024) is
// exercised via a default-construction test + the cap-override tests via
// WithRootSharedDataCaps explicit settings.

package wasm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// newRootVMForSharedData constructs a RootVM around a benign minimal
// module. The shared-data store does NOT require any guest-side dispatch
// (the SetSharedData / GetSharedData methods operate purely host-side on
// the per-RootVM map); but constructing a RootVM is the cleanest seam
// because the field set lives on *RootVM.
func newRootVMForSharedData(t *testing.T, opts ...RootVMOption) *RootVM {
	t.Helper()
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1, opts...)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	t.Cleanup(func() { _ = rv.Close() })
	return rv
}

// --- (a)-(d): CAS golden table -------------------------------------------

func TestSharedData_CASGoldenTable(t *testing.T) {
	rv := newRootVMForSharedData(t)
	key := "k"

	// (a) Set("k", v, cas=0) — new entry; cas starts at 1.
	v1 := []byte("v1")
	if res := rv.SetSharedData(key, v1, 0); res != abi.WasmResultOk {
		t.Fatalf("(a) Set new entry cas=0: got %v; want Ok", res)
	}
	gotV, gotCAS, gotStatus := rv.GetSharedData(key)
	if gotStatus != abi.WasmResultOk {
		t.Fatalf("(a) Get after new: status=%v; want Ok", gotStatus)
	}
	if string(gotV) != "v1" || gotCAS != 1 {
		t.Errorf("(a) Get after new: got (%q, %d); want (%q, 1)", gotV, gotCAS, "v1")
	}

	// (b) Set("k", v2, cas=1) — match; cas increments to 2.
	v2 := []byte("v2")
	if res := rv.SetSharedData(key, v2, 1); res != abi.WasmResultOk {
		t.Fatalf("(b) Set cas-match cas=1: got %v; want Ok", res)
	}
	gotV, gotCAS, gotStatus = rv.GetSharedData(key)
	if gotStatus != abi.WasmResultOk {
		t.Fatalf("(b) Get after cas-match: status=%v; want Ok", gotStatus)
	}
	if string(gotV) != "v2" || gotCAS != 2 {
		t.Errorf("(b) Get after cas-match: got (%q, %d); want (%q, 2)", gotV, gotCAS, "v2")
	}

	// (c) Set("k", v3, cas=99) — mismatch (current cas=2); entry unchanged.
	v3 := []byte("v3")
	if res := rv.SetSharedData(key, v3, 99); res != abi.WasmResultCasMismatch {
		t.Fatalf("(c) Set cas-mismatch cas=99: got %v; want CasMismatch", res)
	}
	gotV, gotCAS, gotStatus = rv.GetSharedData(key)
	if gotStatus != abi.WasmResultOk {
		t.Fatalf("(c) Get after cas-mismatch: status=%v; want Ok", gotStatus)
	}
	if string(gotV) != "v2" || gotCAS != 2 {
		t.Errorf("(c) Get after cas-mismatch: got (%q, %d); want (%q, 2) unchanged",
			gotV, gotCAS, "v2")
	}

	// (d) Set("k", v4, cas=0) — unconditional write; cas increments to 3.
	v4 := []byte("v4")
	if res := rv.SetSharedData(key, v4, 0); res != abi.WasmResultOk {
		t.Fatalf("(d) Set unconditional cas=0: got %v; want Ok", res)
	}
	gotV, gotCAS, gotStatus = rv.GetSharedData(key)
	if gotStatus != abi.WasmResultOk {
		t.Fatalf("(d) Get after unconditional: status=%v; want Ok", gotStatus)
	}
	if string(gotV) != "v4" || gotCAS != 3 {
		t.Errorf("(d) Get after unconditional: got (%q, %d); want (%q, 3)",
			gotV, gotCAS, "v4")
	}
}

// --- (e): Get of nonexistent key -----------------------------------------

func TestSharedData_GetNonexistentReturnsNotFound(t *testing.T) {
	rv := newRootVMForSharedData(t)
	gotV, gotCAS, gotStatus := rv.GetSharedData("nope")
	if gotStatus != abi.WasmResultNotFound {
		t.Errorf("Get nonexistent: status=%v; want NotFound", gotStatus)
	}
	if gotV != nil {
		t.Errorf("Get nonexistent: value=%v; want nil", gotV)
	}
	if gotCAS != 0 {
		t.Errorf("Get nonexistent: cas=%d; want 0", gotCAS)
	}
}

// --- (f): entry-cap boundary ---------------------------------------------

func TestSharedData_EntryCapBoundary(t *testing.T) {
	// Use a tight cap to exercise the boundary cheaply.
	const cap = uint32(4)
	rv := newRootVMForSharedData(t, WithRootSharedDataCaps(0, cap)) // valCap=0 → default 1 MiB; maxEntries=4

	for i := uint32(0); i < cap; i++ {
		key := fmt.Sprintf("k%d", i)
		if res := rv.SetSharedData(key, []byte("v"), 0); res != abi.WasmResultOk {
			t.Fatalf("Set entry %d (within cap): got %v; want Ok", i, res)
		}
	}
	// The cap+1'th entry returns InternalFailure.
	if res := rv.SetSharedData("k_overflow", []byte("v"), 0); res != abi.WasmResultInternalFailure {
		t.Errorf("Set entry beyond cap: got %v; want InternalFailure", res)
	}
	// Existing entries unchanged + still writable on CAS match.
	gotV, gotCAS, _ := rv.GetSharedData("k0")
	if string(gotV) != "v" || gotCAS != 1 {
		t.Errorf("after overflow attempt: k0 = (%q, %d); want (%q, 1)", gotV, gotCAS, "v")
	}
	// In-place update of an existing entry MUST succeed even at-cap (no new
	// slot needed).
	if res := rv.SetSharedData("k0", []byte("v2"), 1); res != abi.WasmResultOk {
		t.Errorf("at-cap in-place update: got %v; want Ok", res)
	}
}

// --- (g): value-cap boundary ---------------------------------------------

func TestSharedData_ValueCapBoundary(t *testing.T) {
	// Use a tight value cap to exercise the boundary cheaply (default would
	// require a 1 MiB+1 byte allocation per test).
	const valCap = uint32(16)
	rv := newRootVMForSharedData(t, WithRootSharedDataCaps(valCap, 0)) // maxEntries=0 → default 1024

	// At-cap exactly is OK.
	if res := rv.SetSharedData("k", make([]byte, valCap), 0); res != abi.WasmResultOk {
		t.Errorf("Set value == cap: got %v; want Ok", res)
	}
	// Above-cap returns InternalFailure.
	if res := rv.SetSharedData("k2", make([]byte, valCap+1), 0); res != abi.WasmResultInternalFailure {
		t.Errorf("Set value > cap: got %v; want InternalFailure", res)
	}
	// Existing entries unchanged.
	gotV, _, gotStatus := rv.GetSharedData("k")
	if gotStatus != abi.WasmResultOk || uint32(len(gotV)) != valCap {
		t.Errorf("Get after value-cap overflow: got (len=%d, status=%v); want (len=%d, Ok)",
			len(gotV), gotStatus, valCap)
	}
	// "k2" never existed.
	if _, _, status := rv.GetSharedData("k2"); status != abi.WasmResultNotFound {
		t.Errorf("Get k2 after rejected write: status=%v; want NotFound", status)
	}
}

// --- (h): concurrent N=100 Set under sync.RWMutex (-race clean) ----------

func TestSharedData_ConcurrentSetNoRace(t *testing.T) {
	rv := newRootVMForSharedData(t) // defaults: 1 MiB value cap; 1024-entry cap (ample for N=100)
	const N = 100

	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", i)
			val := []byte(fmt.Sprintf("v%d", i))
			if res := rv.SetSharedData(key, val, 0); res != abi.WasmResultOk {
				errs <- fmt.Errorf("goroutine %d Set: got %v; want Ok", i, res)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	// All N entries distinct + recoverable via Get.
	for i := 0; i < N; i++ {
		key := fmt.Sprintf("k%d", i)
		wantV := fmt.Sprintf("v%d", i)
		gotV, gotCAS, gotStatus := rv.GetSharedData(key)
		if gotStatus != abi.WasmResultOk {
			t.Errorf("Get %q: status=%v; want Ok", key, gotStatus)
			continue
		}
		if string(gotV) != wantV {
			t.Errorf("Get %q: value=%q; want %q", key, gotV, wantV)
		}
		if gotCAS != 1 {
			t.Errorf("Get %q: cas=%d; want 1 (new-entry cas-start)", key, gotCAS)
		}
	}
}

// --- Concurrent contended-CAS exercise on a single key -------------------
//
// This is a stress probe (NOT a strict assertion) for the CAS-mismatch path
// under contention — it exercises the lock-then-compare-then-write sequence
// under -race. With N goroutines all racing to set ("k", v, cas=i), we
// expect SOME to succeed + SOME to mismatch — no fixed count is asserted
// (the scheduling is non-deterministic). Pass criterion: no race detected,
// no panic, the final entry has a consistent CAS counter.

func TestSharedData_ConcurrentSameKeyCASContended(t *testing.T) {
	rv := newRootVMForSharedData(t)
	// Seed the entry so the per-iteration CAS check has a starting value.
	if res := rv.SetSharedData("k", []byte("seed"), 0); res != abi.WasmResultOk {
		t.Fatalf("seed Set: got %v; want Ok", res)
	}

	const N = 100
	var ok, mismatch atomic.Uint64
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Snapshot the current CAS, then attempt a match-write.
			_, cur, _ := rv.GetSharedData("k")
			val := []byte(fmt.Sprintf("v%d", i))
			res := rv.SetSharedData("k", val, cur)
			switch res {
			case abi.WasmResultOk:
				ok.Add(1)
			case abi.WasmResultCasMismatch:
				mismatch.Add(1)
			default:
				t.Errorf("unexpected result %v from goroutine %d", res, i)
			}
		}(i)
	}
	wg.Wait()

	// Pass criterion: ok + mismatch == N (no surprise results), and the
	// post-state CAS = 1 (seed) + ok.
	if got := ok.Load() + mismatch.Load(); got != N {
		t.Errorf("total dispatched results: %d; want %d", got, N)
	}
	_, finalCAS, status := rv.GetSharedData("k")
	if status != abi.WasmResultOk {
		t.Fatalf("post-contended Get: status=%v; want Ok", status)
	}
	wantCAS := uint32(1 + ok.Load()) //nolint:gosec // ok <= N=100; well within uint32 range
	if finalCAS != wantCAS {
		t.Errorf("post-contended final CAS = %d; want %d (1 seed + %d ok-writes)",
			finalCAS, wantCAS, ok.Load())
	}
}

// --- Default-cap behavior ------------------------------------------------

func TestSharedData_DefaultCapsAppliedAtFirstUse(t *testing.T) {
	rv := newRootVMForSharedData(t) // no WithRootSharedDataCaps → defaults

	// 1 MiB-1 byte succeeds; 1 MiB exact succeeds; 1 MiB+1 byte fails.
	const oneMiB = 1024 * 1024
	if res := rv.SetSharedData("just-under", make([]byte, oneMiB-1), 0); res != abi.WasmResultOk {
		t.Errorf("Set 1MiB-1 with default cap: got %v; want Ok", res)
	}
	if res := rv.SetSharedData("exact", make([]byte, oneMiB), 0); res != abi.WasmResultOk {
		t.Errorf("Set exactly 1MiB with default cap: got %v; want Ok", res)
	}
	if res := rv.SetSharedData("over", make([]byte, oneMiB+1), 0); res != abi.WasmResultInternalFailure {
		t.Errorf("Set 1MiB+1 with default cap: got %v; want InternalFailure", res)
	}
}

// --- Empty value is a valid write -----------------------------------------

func TestSharedData_EmptyValueIsValid(t *testing.T) {
	rv := newRootVMForSharedData(t)
	if res := rv.SetSharedData("k", nil, 0); res != abi.WasmResultOk {
		t.Errorf("Set nil value: got %v; want Ok", res)
	}
	gotV, gotCAS, gotStatus := rv.GetSharedData("k")
	if gotStatus != abi.WasmResultOk {
		t.Errorf("Get after Set nil: status=%v; want Ok", gotStatus)
	}
	if len(gotV) != 0 {
		t.Errorf("Get after Set nil: len=%d; want 0", len(gotV))
	}
	if gotCAS != 1 {
		t.Errorf("Get after Set nil: cas=%d; want 1", gotCAS)
	}
}
