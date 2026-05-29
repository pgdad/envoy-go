// registry_test.go — tests for registry.go: makeVMKey byte-stable pin +
// Registry refcount lifecycle + concurrent acquire/release race safety.
package wasm

import (
	"errors"
	"sync"
	"testing"
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
