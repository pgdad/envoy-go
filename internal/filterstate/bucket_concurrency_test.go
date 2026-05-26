// Tests for the NEW internal/filterstate/ *Bucket RWMutex discipline per
// 25.2 SPEC §3.2 + ADR-0207 §Context. Although ADR-0207 §Context notes
// the per-stream access path is single-goroutine in practice (envoy-go's
// filter dispatch model serializes per-stream access), the internal
// `sync.RWMutex` is defensive against:
//   - future cross-filter concurrency (a Set from one filter while a
//     concurrent property-path Get fires from another),
//   - the wasm tick-goroutine + the dispatch-goroutine both touching the
//     same *Bucket via different StreamContext refs,
//   - test-time stress to confirm the RWMutex is correctly placed.
//
// These tests run under -race and validate the mutex placement is sound.
package filterstate

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// TestBucket_Concurrent_Get_Set_NoRace exercises N=100 goroutines doing
// Get + Set concurrently against a shared *Bucket. -race MUST observe
// no data race. The test does NOT assert a specific final state (the
// concurrent dispatch makes the final state non-deterministic); the
// race-cleanliness assertion is the load-bearing one.
func TestBucket_Concurrent_Get_Set_NoRace(t *testing.T) {
	b := NewBucket()
	const N = 100
	var wg sync.WaitGroup
	wg.Add(N * 2)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i%10) // 10 distinct keys, multi-contended
			obj := &testFSO{data: []byte(fmt.Sprintf("payload-%d", i)), mutable: true}
			_ = b.Set(key, obj)
		}()
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i%10)
			_, _ = b.Get(key)
		}()
	}
	wg.Wait()
}

// TestBucket_Concurrent_DistinctKeys_AllInserted exercises N=100
// goroutines each Setting a DISTINCT key. After all goroutines complete,
// the final *Bucket MUST hold exactly N=100 entries (no race-lost
// inserts).
func TestBucket_Concurrent_DistinctKeys_AllInserted(t *testing.T) {
	b := NewBucket()
	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("distinct-key-%03d", i)
			obj := &testFSO{data: []byte(fmt.Sprintf("payload-%d", i)), mutable: false}
			if err := b.Set(key, obj); err != nil {
				t.Errorf("Set(%q) err = %v; want nil", key, err)
			}
		}()
	}
	wg.Wait()

	keys := b.Keys()
	if len(keys) != N {
		t.Fatalf("after N=%d concurrent distinct-key Set, len(Keys()) = %d; want %d",
			N, len(keys), N)
	}
	// And each key MUST be retrievable individually.
	for i := 0; i < N; i++ {
		key := fmt.Sprintf("distinct-key-%03d", i)
		obj, ok := b.Get(key)
		if !ok {
			t.Errorf("after concurrent insertion, Get(%q) ok = false; want true", key)
		}
		if obj == nil {
			t.Errorf("after concurrent insertion, Get(%q) obj = nil; want non-nil", key)
		}
	}
}

// TestBucket_Concurrent_KeysAgainstSet exercises Keys + Set in parallel —
// Keys MUST never observe a torn state (e.g. a Set in progress that
// hasn't yet installed the key into the map). The RWMutex's Lock + RLock
// discipline ensures this.
//
// The test repeats Keys calls in one goroutine while another Sets new
// keys; each Keys return is verified to be in sorted order (the
// invariant Keys promises regardless of timing).
func TestBucket_Concurrent_KeysAgainstSet(t *testing.T) {
	b := NewBucket()
	// Pre-populate so Keys always returns >= 1 key.
	if err := b.Set("preexisting", &testFSO{data: []byte("x")}); err != nil {
		t.Fatalf("preseed Set err = %v; want nil", err)
	}

	done := atomic.Bool{}
	var wg sync.WaitGroup

	// Setter goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			key := fmt.Sprintf("setter-key-%04d", i)
			_ = b.Set(key, &testFSO{data: []byte(key)})
		}
		done.Store(true)
	}()

	// Keys-reader goroutine; runs until the setter signals done.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !done.Load() {
			keys := b.Keys()
			// Verify sorted invariant on every observation.
			for i := 1; i < len(keys); i++ {
				if keys[i-1] >= keys[i] {
					t.Errorf("Keys() observation NOT sorted at idx %d: %q >= %q",
						i, keys[i-1], keys[i])
					return
				}
			}
		}
		// One more pass after setter completes.
		keys := b.Keys()
		for i := 1; i < len(keys); i++ {
			if keys[i-1] >= keys[i] {
				t.Errorf("final Keys() observation NOT sorted at idx %d: %q >= %q",
					i, keys[i-1], keys[i])
			}
		}
	}()

	wg.Wait()
}

// TestBucket_Concurrent_ReadHeavyDispatch exercises a read-heavy
// dispatch pattern — 1 writer + N=100 concurrent readers. -race MUST
// observe no race; the RLock-allows-concurrent-readers invariant under
// sync.RWMutex is exercised.
func TestBucket_Concurrent_ReadHeavyDispatch(t *testing.T) {
	b := NewBucket()
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("seeded-key-%d", i)
		if err := b.Set(key, &testFSO{data: []byte(key)}); err != nil {
			t.Fatalf("seed Set err = %v; want nil", err)
		}
	}

	const Readers = 100
	const ReadsPerReader = 100
	var wg sync.WaitGroup

	// Single writer.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < ReadsPerReader; i++ {
			key := fmt.Sprintf("writer-key-%d", i)
			_ = b.Set(key, &testFSO{data: []byte(key)})
		}
	}()

	// N readers.
	wg.Add(Readers)
	for r := 0; r < Readers; r++ {
		r := r
		go func() {
			defer wg.Done()
			for i := 0; i < ReadsPerReader; i++ {
				key := fmt.Sprintf("seeded-key-%d", (r+i)%10)
				_, _ = b.Get(key)
				_ = b.Keys()
			}
		}()
	}

	wg.Wait()
}
