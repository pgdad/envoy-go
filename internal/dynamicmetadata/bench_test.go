// Informational micro-benchmarks for the NEW internal/dynamicmetadata/
// Bucket per SPEC §3.1 + ADR-0190. No perf gate at 22.2 PLAN; these
// benchmarks document baseline cost of Get/Set/Snapshot under per-
// stream sequential access (per ADR-0033) for cross-phase consumers.
package dynamicmetadata

import (
	"strconv"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

// BenchmarkBucket_Get measures the cost of a single Get call on a
// pre-populated bucket with a representative (16-filter × 16-key) shape.
func BenchmarkBucket_Get(b *testing.B) {
	bucket := newPopulatedBucket(16, 16)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bucket.Get("filter.8", "key.8")
	}
}

// BenchmarkBucket_Set measures the cost of a single Set call (overwrite
// path — the inner map already exists for the target filterName).
func BenchmarkBucket_Set(b *testing.B) {
	bucket := newPopulatedBucket(16, 16)
	v := structpb.NewStringValue("benchmark")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bucket.Set("filter.8", "key.8", v)
	}
}

// BenchmarkBucket_Snapshot measures the cost of one Snapshot call on a
// (16-filter × 16-key) bucket. The defensive copy is O(filters × keys)
// allocation; the *structpb.Value pointers are shallow-shared.
func BenchmarkBucket_Snapshot(b *testing.B) {
	bucket := newPopulatedBucket(16, 16)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bucket.Snapshot()
	}
}

// newPopulatedBucket builds a Bucket with nFilters × nKeys entries
// populated by structpb.NewStringValue. Used as the shared bench
// fixture; allocation is outside the b.N loop via b.ResetTimer in each
// caller.
func newPopulatedBucket(nFilters, nKeys int) *Bucket {
	bucket := NewBucket()
	for f := 0; f < nFilters; f++ {
		filterName := "filter." + strconv.Itoa(f)
		for k := 0; k < nKeys; k++ {
			key := "key." + strconv.Itoa(k)
			bucket.Set(filterName, key, structpb.NewStringValue("v"))
		}
	}
	return bucket
}
