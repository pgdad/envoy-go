package http

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// HTTPRegistry is the boot-time-populated, freeze-after-boot extension
// registry mapping typed_config type_urls to HTTPFilterFactory functions.
// Per ADR-0072: explicit threaded constructor map (no package-global init);
// freeze-after-boot invariant mirrors *stats.Registry LBP-1 from ADR-0059.
//
//nolint:revive // ADR-0072 reserves the HTTPRegistry name for the boot-time HTTP-filter registry.
type HTTPRegistry struct {
	mu        sync.RWMutex
	byTypeURL map[string]HTTPFilterFactory
	frozen    atomic.Bool
}

// NewHTTPRegistry allocates an empty registry. Call Register for each filter
// factory at boot, then Freeze before listenerManager.New.
func NewHTTPRegistry() *HTTPRegistry {
	return &HTTPRegistry{byTypeURL: make(map[string]HTTPFilterFactory)}
}

// Register adds a filter factory under typeURL. Panics if the registry is
// frozen or if typeURL is already registered (caller bug).
func (r *HTTPRegistry) Register(typeURL string, f HTTPFilterFactory) {
	if r.frozen.Load() {
		panic(fmt.Sprintf("filter: registry frozen: cannot register %q post-boot", typeURL))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byTypeURL[typeURL]; exists {
		panic(fmt.Sprintf("filter: duplicate Register for type_url %q", typeURL))
	}
	r.byTypeURL[typeURL] = f
}

// Lookup returns the registered factory for typeURL, or false if absent.
// Safe to call from any goroutine; lock-free post-Freeze (the RWMutex's
// RLock degenerates to read-only access on a frozen map).
func (r *HTTPRegistry) Lookup(typeURL string) (HTTPFilterFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.byTypeURL[typeURL]
	return f, ok
}

// Freeze marks the registry sealed; subsequent Register panics. Idempotent.
// Called once from cmd/envoy-go/main.go after all Register calls and BEFORE
// listenerManager.New.
func (r *HTTPRegistry) Freeze() {
	r.frozen.Store(true)
}

// KnownTypeURLs returns a sorted slice of registered type_urls. Used by the
// HCM parser's error messages on unknown type_url to give actionable output.
func (r *HTTPRegistry) KnownTypeURLs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byTypeURL))
	for k := range r.byTypeURL {
		out = append(out, k)
	}
	// Sort the slice for deterministic error messages.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
