// Package drain owns the envoy-go drain-state machine. Phase 08.2 (SPEC §1)
// introduces this package as the LBP-1 fifth application of the explicit-
// threading discipline (after *stats.Registry per 06.1, *HTTPRegistry per
// 07.1, *ListenerFilterRegistry per 07.2, and the 08.1 *bootstrap.Bootstrap
// + *cluster.Manager + *listener.Manager triplet threaded into admin.New).
//
// The three-state drain machine (per SPEC §5.9):
//
//	LIVE   ──Drain()──→ DRAINING ──inflight==0 OR timeout──→ DRAINED
//	                                  (Done channel closes; State() still
//	                                   returns Draining — DRAINED is observable
//	                                   ONLY via channel close, not via State())
//
// The Manager is allocated once at boot in cmd/envoy-go/main.go and threaded
// into admin.New, listener.NewManagerWithBaseDirAndAllowH2C, and (via the
// listener manager's filterRegistry) into the HCM and TCP-proxy filter
// constructors. Test code that does not exercise drain semantics may pass nil.
//
// Concurrency model: hot-path operations (State, Inc, Dec, IsDraining) are
// lock-free atomic operations against atomic.Uint32 (state) and atomic.Int64
// (inflight); the only synchronization beyond atomics is sync.Once on the
// Drain trigger and sync.Once-equivalent on the done channel close.
//
// The Manager does NOT enforce its configured timeout. The caller (the
// cmd/envoy-go/main.go SIGTERM-handler) selects on time.After alongside
// Done() to bound the drain window per ADR-0095.
//
// See SPEC §6.2 for the API surface; ADR-0091 records the design.
package drain
