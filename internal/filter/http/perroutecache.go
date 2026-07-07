package http

// perroutecache.go — generic lazy per-route compiled-config cache shared by
// the stateful per-route filters (localratelimit, bandwidthlimit, buffer).
//
// The three consumers previously carried line-for-line copies of the phase-11
// resolvePerRouteConfig shape (Load → build → LoadOrStore, inherit-listener-
// on-error) introduced at internal/filter/http/localratelimit per ADR-0117 +
// IMPL-1 and mirrored verbatim at internal/filter/http/bandwidthlimit per
// ADR-0139. This file extracts the shared mechanics; the per-filter build
// closures (validation + stats registration) stay filter-local so error
// strings and stat names remain byte-identical.
//
// Cache key: the resolved per-route proto message's POINTER identity (each
// distinct typed_per_filter_config entry unmarshals once at route-table parse
// time into a distinct proto pointer, so pointer identity ≡ distinct
// per-route entry per ADR-0117 IMPL-1).

import (
	"sync"

	"google.golang.org/protobuf/proto"
)

// PerRouteCache is a lazy cache mapping a per-route proto message (pointer
// identity) to its compiled per-route config. The zero value is ready to use.
//
// M is the filter's per-route proto pointer type (e.g. *LocalRateLimit);
// C is the filter's compiled-config struct type.
//
// Race-safe via sync.Map's LoadOrStore contract: if two goroutines race to
// compile the same per-route entry, only one allocation wins; the loser's
// value is dropped (any stats it registered are idempotent via the
// *IfAbsent registry constructors, so winner and loser observe the same
// metric instances).
type PerRouteCache[M proto.Message, C any] struct {
	m sync.Map // map[M]*C
}

// Resolve returns the compiled config for the given resolved per-route TPFC
// message (as returned by DecoderFilterCallbacks.RequestRouteConfig), lazily
// compiling it via build on first sight of the proto pointer.
//
// Fallback discipline (mirrors the phase-11 shape verbatim):
//   - msg == nil (no per-route config)          → fallback
//   - msg is not of type M (unexpected message) → fallback
//   - build fails (per-route parse error)       → fallback, NOT cached —
//     request flow stays alive (ADR-0072 boot-fail-fast applies only at
//     boot time, not request time; NO panic).
func (c *PerRouteCache[M, C]) Resolve(msg proto.Message, fallback *C, build func(M) (*C, error)) *C {
	if msg == nil {
		return fallback
	}
	perRoute, ok := msg.(M)
	if !ok {
		return fallback
	}
	if cached, ok := c.m.Load(perRoute); ok {
		return cached.(*C)
	}
	fresh, err := build(perRoute)
	if err != nil {
		return fallback
	}
	actual, _ := c.m.LoadOrStore(perRoute, fresh)
	return actual.(*C)
}

// Range iterates the cached entries (delegates to sync.Map.Range). Test-
// observability seam — consumers assert entry counts after multi-resolve.
func (c *PerRouteCache[M, C]) Range(f func(key, value any) bool) {
	c.m.Range(f)
}
