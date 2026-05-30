// internal/filter/network/registry.go — freeze-after-boot threaded-constructor registry

package network

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Registry maps type_url → NetworkFilterFactory. Populated once at boot from
// cmd/envoy-go/main.go, Freeze()'d before the listener manager starts, read
// concurrently from the per-listener constructor. Byte-identical discipline to
// listenerfilter.ListenerFilterRegistry + http.HTTPRegistry (ADR-0072/0079):
// post-Freeze Register panics; Lookup is lock-free post-Freeze (atomic.Bool
// guard + sync.RWMutex RLock for the map read). NO package global init().
//
//nolint:revive // ADR-0214 reserves the network.Registry name for the boot-time network-filter registry.
type Registry struct {
	mu        sync.RWMutex
	byTypeURL map[string]NetworkFilterFactory
	frozen    atomic.Bool
}

// NewRegistry allocates an empty, unfrozen registry.
func NewRegistry() *Registry {
	return &Registry{byTypeURL: make(map[string]NetworkFilterFactory)}
}

// Register adds f under typeURL. Panics if the registry is frozen, or if
// typeURL is already registered. Boot-time only.
func (r *Registry) Register(typeURL string, f NetworkFilterFactory) {
	if r.frozen.Load() {
		panic(fmt.Sprintf("network: registry frozen: cannot register %q post-boot", typeURL))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byTypeURL[typeURL]; dup {
		panic(fmt.Sprintf("network: duplicate factory for %q", typeURL))
	}
	r.byTypeURL[typeURL] = f
}

// Lookup returns the factory for typeURL plus an ok flag. Safe for concurrent
// use post-Freeze.
func (r *Registry) Lookup(typeURL string) (NetworkFilterFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.byTypeURL[typeURL]
	return f, ok
}

// Freeze marks the registry as immutable. Subsequent Register calls panic;
// Lookup calls remain valid. Idempotent.
func (r *Registry) Freeze() { r.frozen.Store(true) }

// KnownTypeURLs returns a sorted slice of registered type_urls for boot-reject
// error messages. Mirrors internal/filter/http/registry.go (insertion sort).
func (r *Registry) KnownTypeURLs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byTypeURL))
	for k := range r.byTypeURL {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
