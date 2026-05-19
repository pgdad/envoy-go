package dynamicmetadata

// File dynamicmetadata.go — per-stream cross-filter dynamic-metadata
// Bucket accessor + map keyed by (filterName, key) → *structpb.Value
// per SPEC §3.1 + ADR-0190 §Context. Per-stream sequential per
// ADR-0033 (no mutex required). All four methods nil-receiver tolerant
// per ADR-0085. Package-level doc lives in doc.go.

import "google.golang.org/protobuf/types/known/structpb"

// Bucket is a per-stream cross-filter dynamic-metadata accessor.
// Lifecycle: created at filter-chain entry; Reset at OnDestroy.
// Per-stream sequential per ADR-0033 (no cross-filter concurrency
// within a stream); NOT goroutine-safe across streams.
type Bucket struct {
	// m holds the nested (filterName → key → *structpb.Value) map.
	// Allocated lazily by Set; Get + Snapshot + Reset tolerate a nil
	// inner map. Initialized at NewBucket.
	m map[string]map[string]*structpb.Value
}

// NewBucket constructs an empty per-stream metadata bucket.
func NewBucket() *Bucket {
	return &Bucket{
		m: make(map[string]map[string]*structpb.Value),
	}
}

// Get returns the value at (filterName, key), ok=false if absent.
// Nil-receiver tolerant per ADR-0085: returns (nil, false) on a nil
// *Bucket.
func (b *Bucket) Get(filterName, key string) (*structpb.Value, bool) {
	if b == nil || b.m == nil {
		return nil, false
	}
	inner, ok := b.m[filterName]
	if !ok {
		return nil, false
	}
	v, ok := inner[key]
	if !ok {
		return nil, false
	}
	return v, true
}

// Set writes the value at (filterName, key). Overwrites any prior value
// at the same coordinate. Auto-initializes the inner map at first write
// under filterName. Nil-receiver tolerant per ADR-0085: no-op on a nil
// *Bucket.
func (b *Bucket) Set(filterName, key string, value *structpb.Value) {
	if b == nil {
		return
	}
	if b.m == nil {
		b.m = make(map[string]map[string]*structpb.Value)
	}
	inner, ok := b.m[filterName]
	if !ok {
		inner = make(map[string]*structpb.Value)
		b.m[filterName] = inner
	}
	inner[key] = value
}

// Snapshot returns a defensive copy of the bucket's contents for
// read-only iteration (consumed by the Lua bridge's
// :dynamicTypedMetadata() typed-iteration access). Mutating the
// returned outer or inner maps does NOT mutate the bucket. The
// *structpb.Value pointers are shallow-shared (per SPEC §3.1 — values
// are treated as immutable by consumers). Nil-receiver tolerant per
// ADR-0085: returns nil on a nil *Bucket.
func (b *Bucket) Snapshot() map[string]map[string]*structpb.Value {
	if b == nil || b.m == nil {
		return nil
	}
	out := make(map[string]map[string]*structpb.Value, len(b.m))
	for filterName, inner := range b.m {
		innerCopy := make(map[string]*structpb.Value, len(inner))
		for k, v := range inner {
			innerCopy[k] = v
		}
		out[filterName] = innerCopy
	}
	return out
}

// Reset clears all entries (consumed at OnDestroy). The bucket remains
// usable after Reset (subsequent Set + Get continue to function on the
// same *Bucket value). Nil-receiver tolerant per ADR-0085: no-op on a
// nil *Bucket.
func (b *Bucket) Reset() {
	if b == nil {
		return
	}
	// Clear the outer map. Re-initialize to keep the bucket usable
	// without reconstruction (matches the post-Reset Set+Get test
	// expectation).
	b.m = make(map[string]map[string]*structpb.Value)
}
