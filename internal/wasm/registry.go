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
}

// Registry is the process-global VM-sharing registry per AMEND-C2 + ADR-0211.
// It collapses cpp-host's two-layer (process-global base_wasms + thread-local
// local_wasms) into ONE process-global map because Go has no Envoy thread-local
// worker model.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*registryEntry
}

// DefaultRegistry is the process-global singleton consumed by compiledConfig.
var DefaultRegistry = NewRegistry()

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry { return &Registry{entries: map[string]*registryEntry{}} }

// AcquireRootVM returns the *RootVM for key, incrementing its refcount. On a
// cache miss the factory is called to construct a new *RootVM (refcount starts
// at 1). On a cache hit the factory is NOT called (refcount incremented).
func (r *Registry) AcquireRootVM(key string, factory func() (*RootVM, error)) (*RootVM, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[key]; ok {
		e.refcount++
		return e.rootVM, nil
	}
	vm, err := factory()
	if err != nil {
		return nil, err
	}
	r.entries[key] = &registryEntry{rootVM: vm, refcount: 1}
	return vm, nil
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
