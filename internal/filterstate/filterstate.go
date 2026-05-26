package filterstate

// File filterstate.go — generic per-stream filter-state primitive per
// 25.2 SPEC §3.2 + ADR-0207 §Context. Materializes the FilterStateObject
// interface + StateType discriminator + Bucket accessor with Set/Get/Keys
// methods. Package-level doc lives in doc.go.

import (
	"errors"
	"sort"
	"sync"
)

// FilterStateObject is the per-key interface for state objects stored in
// a Bucket. Implementations carry the typed data + serialization
// (Marshal / Unmarshal) + the StateType discriminator (read-only vs
// mutable).
//
// Production implementations:
//   - phase-22.2 lua adapter at internal/filter/http/lua/ wraps a Lua
//     value via the typed Lua-value marshaler (Task 10 MIGRATION).
//   - phase-25.2 wasm adapter at internal/filter/http/wasm/ wraps a
//     proxy-wasm property byte array (Task 13).
//
// Marshal MUST return a defensive copy of any underlying byte buffer
// (consumer mutation of the returned slice MUST NOT mutate the
// FilterStateObject's internal state). Unmarshal MUST tolerate a nil
// input (treated as the empty payload).
//
// The exported name is pinned at ADR-0207 §Context + 25.2 SPEC §3.2 +
// the consumer-site adapter signatures (phase-22.2 lua MIGRATION at
// Task 10 + 25.2 wasm at Task 13 both reference the long form for
// clarity at the consumer boundary). The revive `stutters` advice is
// suppressed accordingly.
//
//nolint:revive // name pinned at ADR-0207 + 25.2 SPEC §3.2.
type FilterStateObject interface {
	// Marshal returns the serialized payload. Implementations return a
	// fresh byte slice on each call (defensive copy).
	Marshal() ([]byte, error)
	// Unmarshal re-hydrates the object from the serialized payload.
	// nil input is treated as the empty payload.
	Unmarshal([]byte) error
	// HasData reports whether the object carries a non-empty
	// serializable payload (mirrors upstream Envoy StreamInfo::
	// FilterState::Object::hasData semantic).
	HasData() bool
	// StateType discriminates read-only vs mutable entries. The Bucket
	// consults this to enforce the conflict semantic at Set.
	StateType() StateType
}

// StateType discriminates read-only vs mutable FilterStateObject
// entries. Pinned at ADR-0207 §Context — external consumers read these
// as ordinary integer constants. The const values MUST NOT change
// without an ADR revision.
type StateType int

const (
	// StateTypeReadOnly entries cannot be replaced by another ReadOnly
	// over an existing Mutable; same-type ReadOnly-over-ReadOnly is OK.
	StateTypeReadOnly StateType = iota // = 0

	// StateTypeMutable entries can be replaced freely; a Mutable Set
	// over an existing ReadOnly entry succeeds (mutable override).
	StateTypeMutable // = 1
)

// Bucket is the per-stream filter-state accessor. One Bucket per stream
// per root — one for downstream `filter_state`, one for upstream
// `upstream_filter_state` per AMEND-B4 (the *compiledConfig constructs
// both at 25.2 IMPL Task 14).
//
// Bucket is NOT goroutine-safe at the per-stream level by design
// (envoy-go's filter dispatch model serializes per-stream access per
// ADR-0033). The internal sync.RWMutex is DEFENSIVE — it covers rare
// cross-goroutine touches (the wasm tick-goroutine + dispatch-goroutine
// concurrent access; future cross-filter primitives that may dispatch
// concurrently; test-time concurrent-stress harnesses).
type Bucket struct {
	mu    sync.RWMutex
	items map[string]FilterStateObject
}

// Sentinel errors returned by Set. Callers use errors.Is to discriminate
// the rejection reason without parsing error strings.
var (
	// ErrNilFilterStateObject is returned by Set when the supplied
	// FilterStateObject is nil. A nil object would confuse the
	// presence/absence signal at Get (which uses (nil, false) to mean
	// "absent") so the Bucket explicitly rejects nil at Set time.
	ErrNilFilterStateObject = errors.New("filterstate: nil FilterStateObject")

	// ErrReadOnlyOverMutable is returned by Set when the caller
	// attempts to replace an existing StateTypeMutable entry with a
	// new StateTypeReadOnly object. Mirrors upstream Envoy v1.37.2
	// FilterState::setData(...) semantics — a read-only Set against a
	// key with a Mutable existing value is rejected. The reverse
	// (Mutable over ReadOnly) IS allowed (the mutable override).
	ErrReadOnlyOverMutable = errors.New("filterstate: cannot replace mutable entry with read-only object")
)

// NewBucket constructs an initialized empty *Bucket. The returned
// Bucket is ready for Set/Get/Keys without further initialization.
func NewBucket() *Bucket {
	return &Bucket{
		items: make(map[string]FilterStateObject),
	}
}

// Set inserts or replaces the entry at key. Conflict semantics per
// ADR-0207 §Context:
//
//   - obj == nil → returns ErrNilFilterStateObject (no insertion).
//   - existing entry has StateTypeMutable AND new obj has
//     StateTypeReadOnly → returns ErrReadOnlyOverMutable (no replacement;
//     existing entry preserved).
//   - all other combinations → replace-or-insert OK; returns nil.
//
// The conflict semantic mirrors upstream Envoy v1.37.2
// FilterState::setData(key, obj, state_type, life_span) — a read-only
// Set against a key with a Mutable existing value is rejected; the
// reverse (Mutable over ReadOnly) is allowed.
func (b *Bucket) Set(key string, obj FilterStateObject) error {
	if obj == nil {
		return ErrNilFilterStateObject
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existing, ok := b.items[key]; ok {
		// Conflict check: only the "ReadOnly replacing Mutable" direction
		// is rejected; all other transitions (ReadOnly→Mutable,
		// Mutable→Mutable, ReadOnly→ReadOnly) are allowed.
		if existing.StateType() == StateTypeMutable && obj.StateType() == StateTypeReadOnly {
			return ErrReadOnlyOverMutable
		}
	}
	b.items[key] = obj
	return nil
}

// Get returns the FilterStateObject at key + a presence bool. Absent
// key → (nil, false). The returned object is the same pointer identity
// stored at Set (no defensive copy at the Bucket level — the
// FilterStateObject impl is responsible for any defensive copying at
// Marshal time).
func (b *Bucket) Get(key string) (FilterStateObject, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	obj, ok := b.items[key]
	return obj, ok
}

// Keys returns a sorted slice of all entry keys. The sort order is
// lexicographic (byte-wise) for deterministic property-tree enumeration
// at the proxy-wasm `filter_state.*` dispatch (Task 13 consumer).
// Empty bucket returns a non-nil empty slice (callers may range without
// a nil guard).
func (b *Bucket) Keys() []string {
	b.mu.RLock()
	keys := make([]string, 0, len(b.items))
	for k := range b.items {
		keys = append(keys, k)
	}
	b.mu.RUnlock()
	sort.Strings(keys)
	return keys
}
