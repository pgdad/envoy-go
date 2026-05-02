package listenerfilter

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// ListenerFilterRegistry maps type_url → ListenerFilterFactory. The registry
// is populated once at boot from cmd/envoy-go/main.go (per ADR-0079 +
// ADR-0072 threading discipline mirrored from 07.1), Freeze()'d before the
// listener manager starts accepting, and read concurrently from the
// listener-manager's per-listener constructor at boot.
//
// Post-Freeze Register panics; Lookup remains lock-free (atomic.Bool guard
// + sync.RWMutex RLock for the map read).
//
//nolint:revive // ADR-0079 reserves the ListenerFilterRegistry name for the boot-time listener-filter registry.
type ListenerFilterRegistry struct {
	mu        sync.RWMutex
	byTypeURL map[string]ListenerFilterFactory
	frozen    atomic.Bool
}

// NewListenerFilterRegistry allocates an empty, unfrozen registry.
func NewListenerFilterRegistry() *ListenerFilterRegistry {
	return &ListenerFilterRegistry{byTypeURL: make(map[string]ListenerFilterFactory)}
}

// Register adds f under typeURL. Panics if the registry is frozen, or if
// typeURL is already registered. Boot-time only.
func (r *ListenerFilterRegistry) Register(typeURL string, f ListenerFilterFactory) {
	if r.frozen.Load() {
		panic(fmt.Sprintf("listenerfilter: registry frozen: cannot register %q post-boot", typeURL))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byTypeURL[typeURL]; dup {
		panic(fmt.Sprintf("listenerfilter: duplicate factory for %q", typeURL))
	}
	r.byTypeURL[typeURL] = f
}

// Lookup returns the factory for typeURL plus an ok flag. Safe for
// concurrent use post-Freeze (and pre-Freeze, though typically not called
// pre-Freeze in production).
func (r *ListenerFilterRegistry) Lookup(typeURL string) (ListenerFilterFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.byTypeURL[typeURL]
	return f, ok
}

// Freeze marks the registry as immutable. Subsequent Register calls panic;
// Lookup calls remain valid. Idempotent.
func (r *ListenerFilterRegistry) Freeze() {
	r.frozen.Store(true)
}
