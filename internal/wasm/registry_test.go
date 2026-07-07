// registry_test.go — tests for registry.go: makeVMKey byte-stable pin +
// Registry refcount lifecycle + concurrent acquire/release race safety.
package wasm

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// TestMakeVMKey_ByteStable pins the registry key to cpp-host makeVmKey
// (src/wasm.cc:90-92): Sha256(vm_id || "||" || vm_configuration || "||" || code).
func TestMakeVMKey_ByteStable(t *testing.T) {
	cases := []struct {
		name           string
		vmID           string
		vmConfig, code []byte
		wantHex        string
	}{
		{"populated", "vm1", []byte("cfg"), []byte("code"),
			"31ac38d3d1be49b4258d350d2566947678c1a39a97d31ecc51c79201e0397813"},
		{"empty_vm_id", "", nil, []byte("code"),
			"0f94acb29bf7edf4c4dd4b131644d87778730b06bea27351bf4fad87de7c22a8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := makeVMKeyHex(tc.vmID, tc.vmConfig, tc.code)
			if got != tc.wantHex {
				t.Fatalf("makeVMKeyHex(%q,%q,%q) = %s, want %s",
					tc.vmID, tc.vmConfig, tc.code, got, tc.wantHex)
			}
		})
	}
}

func TestRegistry_AcquireReuseByKey(t *testing.T) {
	r := NewRegistry()
	calls := 0
	factory := func() (*RootVM, error) { calls++; return &RootVM{}, nil }
	key := makeVMKey("vm1", nil, []byte("code"))
	a, err := r.AcquireRootVM(key, factory)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.AcquireRootVM(key, factory)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("same key must return the same *RootVM")
	}
	if calls != 1 {
		t.Fatalf("factory called %d times, want 1 (reuse on hit)", calls)
	}
	if got := r.refcountFor(key); got != 2 {
		t.Fatalf("refcount = %d, want 2", got)
	}
}

// TestRegistry_AcquireFor_ReusesByComponentsAndReturnsStableKey proves that
// AcquireFor reuses a *RootVM by (vmID, vmConfig, code) and returns the SAME
// composite key on a second call (so the caller can Release deterministically).
func TestRegistry_AcquireFor_ReusesByComponentsAndReturnsStableKey(t *testing.T) {
	r := NewRegistry()
	calls := 0
	factory := func() (*RootVM, error) { calls++; return &RootVM{}, nil }

	a, key1, err := r.AcquireFor("vmF", []byte("cfg"), []byte("code"), factory)
	if err != nil {
		t.Fatal(err)
	}
	b, key2, err := r.AcquireFor("vmF", []byte("cfg"), []byte("code"), factory)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("AcquireFor with identical (vmID,cfg,code) must return the same *RootVM")
	}
	if key1 != key2 {
		t.Fatal("AcquireFor must return the same composite key for identical components")
	}
	if key1 != makeVMKey("vmF", []byte("cfg"), []byte("code")) {
		t.Fatal("AcquireFor key must equal makeVMKey of the same components")
	}
	if calls != 1 {
		t.Fatalf("factory called %d times, want 1 (reuse on hit)", calls)
	}
	if got := r.refcountFor(key1); got != 2 {
		t.Fatalf("refcount = %d, want 2", got)
	}
}

func TestRegistry_ReleaseToZeroRemovesAndCloses(t *testing.T) {
	r := NewRegistry()
	key := makeVMKey("vm2", nil, []byte("code"))
	_, _ = r.AcquireRootVM(key, func() (*RootVM, error) { return &RootVM{}, nil })
	_, _ = r.AcquireRootVM(key, func() (*RootVM, error) { return &RootVM{}, nil })
	if err := r.Release(key); err != nil {
		t.Fatal(err)
	}
	if got := r.refcountFor(key); got != 1 {
		t.Fatalf("refcount = %d, want 1 after one Release", got)
	}
	if err := r.Release(key); err != nil {
		t.Fatal(err)
	}
	if r.has(key) {
		t.Fatal("entry must be removed at refcount 0")
	}
}

// TestRegistry_ResetForTestClearsEntries proves ResetForTest drops + closes
// all entries (and shared-data stores) so a later same-key acquire is a MISS
// rather than serving a stale shared *RootVM. This is the test-isolation hook
// the wasm filter suite relies on between cases.
func TestRegistry_ResetForTestClearsEntries(t *testing.T) {
	r := NewRegistry()
	key := makeVMKey("vmreset", nil, []byte("code"))
	_, _ = r.AcquireRootVM(key, func() (*RootVM, error) { return &RootVM{}, nil })
	_ = r.AcquireSharedData("vmreset")
	if !r.has(key) {
		t.Fatal("precondition: entry must exist before ResetForTest")
	}
	r.ResetForTest()
	if r.has(key) {
		t.Fatal("ResetForTest must drop all entries")
	}
	if got := r.refcountFor(key); got != 0 {
		t.Fatalf("refcount = %d, want 0 after ResetForTest", got)
	}
	// A subsequent acquire is a fresh MISS: the factory runs again.
	calls := 0
	_, _ = r.AcquireRootVM(key, func() (*RootVM, error) { calls++; return &RootVM{}, nil })
	if calls != 1 {
		t.Fatalf("post-reset acquire factory calls = %d, want 1 (fresh miss)", calls)
	}
}

func TestRegistry_FactoryErrorNotCached(t *testing.T) {
	r := NewRegistry()
	key := makeVMKey("vmerr", nil, []byte("code"))

	// First call: factory returns an error — must NOT cache the entry.
	_, err := r.AcquireRootVM(key, func() (*RootVM, error) {
		return nil, errors.New("factory failure")
	})
	if err == nil {
		t.Fatal("expected non-nil error from failing factory")
	}
	if r.has(key) {
		t.Fatal("failed factory result must not be inserted into registry")
	}

	// Second call: succeeding factory — miss path must run again.
	calls := 0
	vm, err := r.AcquireRootVM(key, func() (*RootVM, error) {
		calls++
		return &RootVM{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("factory called %d times on retry, want 1", calls)
	}
	if vm == nil {
		t.Fatal("expected non-nil *RootVM on success")
	}
	if !r.has(key) {
		t.Fatal("entry must be present after successful acquire")
	}
}

// TestRegistry_FactoryRunsOutsideLock proves the pending-sentry discipline:
// an in-flight factory for one key MUST NOT hold the process-global registry
// mutex (the factory performs a full VM boot including guest code execution),
// so an acquire for a DIFFERENT key completes while the first factory is
// still running.
func TestRegistry_FactoryRunsOutsideLock(t *testing.T) {
	r := NewRegistry()
	keyA := makeVMKey("vmlockA", nil, []byte("a"))
	keyB := makeVMKey("vmlockB", nil, []byte("b"))

	factoryEntered := make(chan struct{})
	releaseFactory := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := r.AcquireRootVM(keyA, func() (*RootVM, error) {
			close(factoryEntered)
			<-releaseFactory
			return &RootVM{}, nil
		})
		done <- err
	}()
	<-factoryEntered

	// While keyA's factory is in flight, an unrelated key must not block.
	acquired := make(chan struct{})
	go func() {
		if _, err := r.AcquireRootVM(keyB, func() (*RootVM, error) { return &RootVM{}, nil }); err != nil {
			t.Errorf("AcquireRootVM(keyB): %v", err)
		}
		close(acquired)
	}()
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("AcquireRootVM(keyB) blocked behind keyA's in-flight factory (registry mutex held across factory)")
	}

	close(releaseFactory)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := r.refcountFor(keyA); got != 1 {
		t.Fatalf("refcount(keyA) = %d, want 1", got)
	}
}

// TestRegistry_ConcurrentSameKeyAcquire_SingleFactory proves concurrent
// same-key acquirers wait on the construction sentry: the factory runs
// exactly once and every acquirer receives the same *RootVM with the full
// refcount.
func TestRegistry_ConcurrentSameKeyAcquire_SingleFactory(t *testing.T) {
	r := NewRegistry()
	key := makeVMKey("vmonce", nil, []byte("code"))
	var calls atomic.Int32
	const N = 16
	vms := make([]*RootVM, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			vm, err := r.AcquireRootVM(key, func() (*RootVM, error) {
				calls.Add(1)
				time.Sleep(5 * time.Millisecond) // widen the construction window
				return &RootVM{}, nil
			})
			if err != nil {
				t.Error(err)
			}
			vms[i] = vm
		}(i)
	}
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("factory calls = %d, want 1 (same-key acquirers must wait on the sentry)", got)
	}
	for i := 1; i < N; i++ {
		if vms[i] != vms[0] {
			t.Fatal("all same-key acquirers must share one *RootVM")
		}
	}
	if got := r.refcountFor(key); got != N {
		t.Fatalf("refcount = %d, want %d", got, N)
	}
}

// TestRegistry_FactoryErrorWakesWaiters proves a same-key waiter blocked on
// a construction sentry retries with its OWN factory after the first
// factory fails (the failed sentry is removed, not cached).
func TestRegistry_FactoryErrorWakesWaiters(t *testing.T) {
	r := NewRegistry()
	key := makeVMKey("vmerrwake", nil, []byte("code"))

	factoryEntered := make(chan struct{})
	releaseFactory := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := r.AcquireRootVM(key, func() (*RootVM, error) {
			close(factoryEntered)
			<-releaseFactory
			return nil, errors.New("boot failure")
		})
		firstDone <- err
	}()
	<-factoryEntered

	// Second same-key acquirer parks on the sentry.
	secondDone := make(chan error, 1)
	var secondCalls atomic.Int32
	go func() {
		_, err := r.AcquireRootVM(key, func() (*RootVM, error) {
			secondCalls.Add(1)
			return &RootVM{}, nil
		})
		secondDone <- err
	}()

	close(releaseFactory)
	if err := <-firstDone; err == nil {
		t.Fatal("expected first acquire to surface the factory error")
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second acquire after failed sentry: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second acquirer never woke from the failed construction sentry")
	}
	if got := secondCalls.Load(); got != 1 {
		t.Fatalf("second factory calls = %d, want 1 (fresh miss after failure)", got)
	}
	if got := r.refcountFor(key); got != 1 {
		t.Fatalf("refcount = %d, want 1", got)
	}
}

func TestRegistry_ConcurrentAcquireRelease(t *testing.T) {
	r := NewRegistry()
	key := makeVMKey("vmrace", nil, []byte("code"))
	const N = 64
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.AcquireRootVM(key, func() (*RootVM, error) { return &RootVM{}, nil })
			if err != nil {
				t.Error(err)
			}
			_ = r.Release(key)
		}()
	}
	wg.Wait()
	if r.has(key) {
		t.Fatal("registry must be empty after balanced acquire/release")
	}
}

// Two *RootVM under the SAME vm_id but DIFFERENT composite keys MUST observe
// ONE shared-data namespace per AMEND-C2.
func TestRegistry_SharedDataAtVMIDScope(t *testing.T) {
	r := NewRegistry()
	s1 := r.AcquireSharedData("vmid-shared")
	s2 := r.AcquireSharedData("vmid-shared")
	if s1 != s2 {
		t.Fatal("same vm_id must yield the same shared-data store")
	}
	s3 := r.AcquireSharedData("vmid-other")
	if s1 == s3 {
		t.Fatal("distinct vm_id must yield distinct stores")
	}
}

// Two RootVMs constructed with the SAME injected store see each other's
// writes; a RootVM with its own (default) store does NOT.
//
// Step-3 variant choice: RootVM-level test via newRootVMForSharedData (defined
// in shared_data_test.go). The helper accepts variadic RootVMOption so
// WithSharedDataStore wires cleanly; no awkward plumbing needed.
func TestRootVM_SharedDataStoreSharing(t *testing.T) {
	store := newSharedDataStore()

	// rvA and rvB share the injected store.
	rvA := newRootVMForSharedData(t, WithSharedDataStore(store))
	rvB := newRootVMForSharedData(t, WithSharedDataStore(store))
	// rvC has its own default (private) store — must NOT see rvA's writes.
	rvC := newRootVMForSharedData(t)

	// Write via rvA.
	if res := rvA.SetSharedData("cross-vm-key", []byte("hello"), 0); res != abi.WasmResultOk {
		t.Fatalf("rvA.SetSharedData: got %v; want Ok", res)
	}

	// rvB must see the write (shared store).
	gotV, gotCAS, gotStatus := rvB.GetSharedData("cross-vm-key")
	if gotStatus != abi.WasmResultOk {
		t.Fatalf("rvB.GetSharedData: status=%v; want Ok (shared store)", gotStatus)
	}
	if string(gotV) != "hello" || gotCAS != 1 {
		t.Errorf("rvB.GetSharedData: got (%q, cas=%d); want (\"hello\", cas=1)", gotV, gotCAS)
	}

	// rvC must NOT see the write (private store).
	_, _, statusC := rvC.GetSharedData("cross-vm-key")
	if statusC != abi.WasmResultNotFound {
		t.Errorf("rvC.GetSharedData (isolated store): status=%v; want NotFound", statusC)
	}
}
