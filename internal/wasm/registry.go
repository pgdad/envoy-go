// registry.go — process-global VM-sharing registry per AMEND-C2 + ADR-0211.
//
// Multiple wasm plugins that share the same (vm_id, vm_configuration, code)
// reuse ONE *RootVM via this registry with a refcount. The registry key
// mirrors proxy-wasm-cpp-host makeVmKey (src/wasm.cc:90-92):
//
//	Sha256(vm_id || "||" || vm_configuration || "||" || code)
//
// Runtime is NOT part of the key (envoy-go is wazero-single-runtime per
// AMEND-C2). The raw 32-byte digest is used as a map key (not hex-encoded)
// for allocation efficiency; makeVMKeyHex wraps it for human-readable output.
//
// # Collapse of cpp-host two-layer model
//
// cpp-host keeps a process-global base_wasms map + per-thread-worker
// local_wasms map. envoy-go has no Envoy thread-local worker model (Go
// goroutines are not pinned), so both layers collapse into ONE process-global
// map here per AMEND-C2.
//
// # Refcount lifecycle
//
//   - AcquireRootVM: increment refcount (or create with refcount=1). Factory
//     is called only on the miss path (refcount 0 → 1).
//   - Release: decrement refcount. At refcount 0 the entry is removed from
//     the map and Close is called on the *RootVM.

package wasm

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// makeVMKey mirrors proxy-wasm-cpp-host makeVmKey (src/wasm.cc:90-92):
// Sha256(vm_id || "||" || vm_configuration || "||" || code). Runtime is NOT
// in the key (envoy-go is wazero-single-runtime per AMEND-C2). Returns the
// raw 32-byte digest as a string for use as a map key.
//
// The key is computed over the received wire bytes (vmID + vm_configuration
// serialized bytes + code) and is therefore stable for byte-identical wire
// input but NOT canonical-form-stable: two semantically-identical configs
// delivered with different proto field ordering produce different keys and
// fail to share a VM (a benign false-negative, matching cpp-host parity).
func makeVMKey(vmID string, vmConfig, code []byte) string {
	h := sha256.New()
	h.Write([]byte(vmID))
	h.Write([]byte("||"))
	h.Write(vmConfig)
	h.Write([]byte("||"))
	h.Write(code)
	return string(h.Sum(nil))
}

// makeVMKeyHex is the hex-encoded form of makeVMKey (test/observability helper).
func makeVMKeyHex(vmID string, vmConfig, code []byte) string {
	return hex.EncodeToString([]byte(makeVMKey(vmID, vmConfig, code)))
}

// registryEntry holds a shared *RootVM and its reference count.
type registryEntry struct {
	rootVM   *RootVM
	refcount int
	// pending is non-nil while the entry is a construction SENTRY: the
	// factory for this key is still running OUTSIDE r.mu (see AcquireRootVM).
	// Closed exactly once when construction completes — publish (rootVM set)
	// or removal (factory error). Concurrent same-key acquirers block on it
	// outside r.mu, then re-check the map. nil once published.
	pending chan struct{}
}

// Registry is the process-global VM-sharing registry per AMEND-C2 + ADR-0211.
// It collapses cpp-host's two-layer (process-global base_wasms + thread-local
// local_wasms) into ONE process-global map because Go has no Envoy thread-local
// worker model.
type Registry struct {
	mu         sync.Mutex
	entries    map[string]*registryEntry
	sharedData map[string]*sharedDataStore
}

// DefaultRegistry is the process-global singleton consumed by compiledConfig.
var DefaultRegistry = NewRegistry()

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		entries:    map[string]*registryEntry{},
		sharedData: map[string]*sharedDataStore{},
	}
}

// AcquireRootVM returns the *RootVM for key, incrementing its refcount. On a
// cache miss the factory is called to construct a new *RootVM (refcount starts
// at 1). On a cache hit the factory is NOT called (refcount incremented).
//
// The factory runs OUTSIDE the registry mutex via a per-key pending-sentry:
// the factory performs a full wazero compile + host-module registration +
// instantiation + guest Configure (guest code executes), so holding the
// process-global r.mu across it would stall every unrelated Acquire /
// Release / AcquireSharedData for the whole VM boot. Concurrent acquirers of
// the SAME key wait on the sentry (the factory still runs exactly once per
// successful construction); acquirers of OTHER keys proceed immediately.
func (r *Registry) AcquireRootVM(key string, factory func() (*RootVM, error)) (*RootVM, error) {
	r.mu.Lock()
	for {
		e, ok := r.entries[key]
		if !ok {
			break
		}
		if e.pending == nil {
			// Published entry — plain cache hit.
			e.refcount++
			r.mu.Unlock()
			return e.rootVM, nil
		}
		// Same-key construction in flight — wait OUTSIDE r.mu, then
		// re-check the map: on publish this becomes a cache hit; on
		// factory failure the sentry is removed and this acquirer runs
		// its own factory (miss path).
		wait := e.pending
		r.mu.Unlock()
		<-wait
		r.mu.Lock()
	}

	// Miss: insert the pending sentry + run the factory outside r.mu.
	sentry := &registryEntry{refcount: 1, pending: make(chan struct{})}
	r.entries[key] = sentry
	r.mu.Unlock()

	vm, err := factory()

	r.mu.Lock()
	if err != nil {
		// Failed factory result must NOT be cached — remove the sentry so
		// waiters (and later acquirers) take a fresh miss.
		delete(r.entries, key)
	} else {
		sentry.rootVM = vm
	}
	pending := sentry.pending
	sentry.pending = nil
	r.mu.Unlock()
	close(pending)

	if err != nil {
		return nil, err
	}
	return vm, nil
}

// AcquireFor computes the composite VM key from (vmID, vmConfig, code) and
// acquires (refcount++) or constructs the shared *RootVM via factory. Returns
// the *RootVM AND the key (the caller retains the key to Release at teardown).
// The vm_id-scoped shared-data store is obtained separately via AcquireSharedData.
//
// This is the EXPORTED cross-package entry consumed by the filter package
// (internal/filter/http/wasm/compiled_config.go) which cannot call the
// unexported makeVMKey directly.
func (r *Registry) AcquireFor(vmID string, vmConfig, code []byte, factory func() (*RootVM, error)) (*RootVM, string, error) {
	key := makeVMKey(vmID, vmConfig, code)
	vm, err := r.AcquireRootVM(key, factory)
	if err != nil {
		return nil, "", err
	}
	return vm, key, nil
}

// Release decrements the refcount for key. When the refcount reaches 0 the
// entry is removed and Close is called on the *RootVM. Each Release call MUST
// correspond to exactly one prior successful AcquireRootVM call for the same
// key; an unbalanced (double) Release produces undefined behavior (premature
// Close) rather than a panic.
func (r *Registry) Release(key string) error {
	r.mu.Lock()
	e, ok := r.entries[key]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	e.refcount--
	if e.refcount > 0 {
		r.mu.Unlock()
		return nil
	}
	delete(r.entries, key)
	r.mu.Unlock()
	if e.rootVM == nil {
		// Unbalanced Release against a still-pending construction sentry
		// (caller misuse per the doc above) — nothing to Close yet.
		return nil
	}
	return e.rootVM.Close()
}

// refcountFor returns the current refcount for key (0 if absent).
// Test/observability helper — unexported, same-package test usage only.
func (r *Registry) refcountFor(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[key]; ok {
		return e.refcount
	}
	return 0
}

// has reports whether the registry contains an entry for key.
// Test/observability helper — unexported, same-package test usage only.
func (r *Registry) has(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.entries[key]
	return ok
}

// ResetForTest drops + closes all entries and shared-data stores. Test-only:
// the process-global DefaultRegistry accumulates shared *RootVM across configs,
// so tests that build configs MUST reset between cases to avoid cross-test
// contamination (a stale shared VM served on a later same-key acquire). VMs are
// closed OUTSIDE the lock to avoid holding r.mu across Close.
func (r *Registry) ResetForTest() {
	r.mu.Lock()
	entries := r.entries
	r.entries = map[string]*registryEntry{}
	r.sharedData = map[string]*sharedDataStore{}
	r.mu.Unlock()
	for _, e := range entries {
		if e.rootVM != nil {
			_ = e.rootVM.Close()
		}
	}
}

// AcquireSharedData returns the shared-data store for the given raw vm_id,
// constructing it on first use. Distinct vm_ids get distinct stores; the
// same vm_id always yields the same store (AMEND-C2 vm_id-scope). The store
// outlives individual *RootVM lifecycles (it is keyed by vm_id, not by the
// composite VM key) — there is no refcount-driven teardown for shared-data,
// matching cpp-host (SharedData persists for the process).
func (r *Registry) AcquireSharedData(vmID string) *sharedDataStore {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sharedData[vmID]; ok {
		return s
	}
	s := newSharedDataStore()
	r.sharedData[vmID] = s
	return s
}
