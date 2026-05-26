// stats_recorder.go — RootStatsRecorder interface + no-op default impl per
// 25.2 IMPL Task 20 follow-up (Concern 2 — counter-glue wiring gap).
//
// # Design intent
//
// The 9 NEW envoy-go-strict counters added at Tasks 16-17 (per 25.2 SPEC §7.1
// + AMEND-B3) live on the filter-package-private *filterStats struct (at
// internal/filter/http/wasm/stats.go). The internal/wasm/ package — which
// houses the hostcall bodies that MUST increment those counters per §7.1 —
// cannot import internal/filter/http/wasm (would induce a cycle: the filter
// package already imports internal/wasm to construct *RootVM at
// compiled_config.New). To decouple the increment sites from the concrete
// counter holder, we define a small package-local interface that *filterStats
// implements via thin wrappers — the same decoupling pattern used by
// host_bridge_25_2.go to satisfy the abi/* package-private interfaces.
//
// # Interface roster (9 counters; matches §7.1 + AMEND-B3)
//
// 9 envoy-go-strict counter increments + 1 RE-USE of the existing 25.1
// envoy_go.failures counter for the §2.25 co-increment discipline:
//
//   - TickInvocationsInc                  — tick.go lockAndDispatchTick per fire
//   - HttpCallDispatchedInc               — http_call.go DispatchHttpCall OK
//   - HttpCallResponseInc                 — http_call.go handleHttpCallResponse OK
//   - ForeignFunctionDeniedInc            — root_vm.go CallForeignFunction NotFound
//   - BodyBufferCapExceededInc            — already wired at Task 16 (re-exposed for parity)
//   - HttpCallDispatchUnknownClusterInc   — http_call.go DispatchHttpCall unknown cluster
//   - SharedDataCapExceededInc            — shared_data.go SetSharedData cap-exceeded
//   - DynamicStatsCapExceededInc          — dynamic_stats.go DefineMetric cap-exceeded
//   - HttpCallResponseAfterCloseInc       — http_call.go handleHttpCallResponse token-miss
//   - EnvoyGoFailuresInc                  — RE-USE of 25.1 counter; §2.25 co-increment
//                                           on shared_data + dynamic_stats cap-exceeded +
//                                           foreign-function panic-recovery
//
// # Lifecycle
//
// Wired ONCE at NewRootVM via WithRootStats(recorder). If no option is
// supplied (typical for unit-test fixtures + the standalone wasm-package
// tests that don't need to observe counter increments), rv.stats defaults to
// noopStatsRecorder so the .Inc() calls are zero-cost no-ops + the test
// fixtures don't need to construct a *filterStats. The production wiring at
// internal/filter/http/wasm/compiled_config.go New() passes
// WithRootStats(cfg.stats) — *filterStats satisfies the interface via thin
// per-counter wrapper methods on the receiver.
//
// # Goroutine-safety
//
// All recorder methods are invoked from hostcall body frames (under
// dispatchMu) OR from the response goroutine + tick goroutine (which also
// acquire dispatchMu before invoking handleHttpCallResponse / proxy_on_tick).
// The underlying *stats.Counter primitives (internal/stats/counter.go) are
// lock-free atomic.Uint64 stores; the recorder methods do not introduce
// additional locking discipline beyond what the counter primitive already
// provides.

package wasm

// RootStatsRecorder is the per-RootVM counter sink for the 9 NEW
// envoy-go-strict counters per 25.2 SPEC §7.1 + AMEND-B3 + the §2.25
// co-increment of envoy_go.failures. Per the file-header doc: *filterStats
// (internal/filter/http/wasm/stats.go) satisfies this interface via thin
// per-counter wrapper methods; the wasm package imports nothing from the
// filter package.
//
// All methods are nil-safe: the recorder is set once at NewRootVM (defaulting
// to noopStatsRecorder if no option supplied) so callers may invoke
// rv.stats.XxxInc() without nil-checks.
type RootStatsRecorder interface {
	// TickInvocationsInc bumps the per-`proxy_on_tick` invocation counter
	// per 25.2 SPEC §7.1 row 6 + Q5. Wire name:
	// `wasm.<plugin>.tick_invocations`. Called from tick.go
	// lockAndDispatchTick on every tick fire, INSIDE the dispatchMu frame.
	TickInvocationsInc()

	// HttpCallDispatchedInc bumps the per-successful-proxy_http_call
	// dispatch counter per 25.2 SPEC §7.1 row 7 + Q4 + AMEND-B3. Wire name:
	// `wasm.<plugin>.http_call_dispatched`. Called from http_call.go
	// DispatchHttpCall on the OK path, INSIDE the dispatchMu frame.
	HttpCallDispatchedInc()

	// HttpCallResponseInc bumps the per-`proxy_on_http_call_response`
	// counter per 25.2 SPEC §7.1 row 8 + Q4. Wire name:
	// `wasm.<plugin>.http_call_response`. Called from http_call.go
	// handleHttpCallResponse on the live-stream path, INSIDE the dispatchMu
	// frame (the response goroutine acquires dispatchMu before calling).
	HttpCallResponseInc()

	// ForeignFunctionDeniedInc bumps the per-`proxy_call_foreign_function`
	// NotFound counter per 25.2 SPEC §7.1 row 9 + AMEND-A9. Wire name:
	// `wasm.<plugin>.foreign_function_denied`. Called from root_vm.go
	// CallForeignFunction on the EMPTY-default-registry NotFound path,
	// INSIDE the dispatchMu frame.
	ForeignFunctionDeniedInc()

	// BodyBufferCapExceededInc bumps the per-stream first-cap-exceeded
	// counter per 25.2 SPEC §7.1 + Q2 + AMEND-B3. Wire name:
	// `wasm.<plugin>.body_buffer_cap_exceeded`. Wired at Task 16
	// body.go (not from the wasm package directly; included on the
	// interface for parity + so the filter package can satisfy a single
	// surface). Production callers should ALSO call EnvoyGoFailuresInc per
	// §2.25.
	BodyBufferCapExceededInc()

	// HttpCallDispatchUnknownClusterInc bumps the per-unknown-cluster
	// dispatch counter per 25.2 SPEC §7.1 row 11 + Q4 + AMEND-B3. Wire
	// name: `wasm.<plugin>.http_call_dispatch_unknown_cluster`. Called from
	// http_call.go DispatchHttpCall on the cluster-lookup-miss path,
	// INSIDE the dispatchMu frame.
	HttpCallDispatchUnknownClusterInc()

	// SharedDataCapExceededInc bumps the per-cap-exceeded counter per
	// 25.2 SPEC §7.1 row 12 + Q6. Wire name:
	// `wasm.<plugin>.shared_data_cap_exceeded`. Called from shared_data.go
	// SetSharedData on the value-cap OR entry-cap overflow path. Per
	// §2.25 the caller MUST also call EnvoyGoFailuresInc.
	SharedDataCapExceededInc()

	// DynamicStatsCapExceededInc bumps the per-cap-exceeded counter per
	// 25.2 SPEC §7.1 row 13 + Q9. Wire name:
	// `wasm.<plugin>.dynamic_stats_cap_exceeded`. Called from
	// dynamic_stats.go DefineMetric on the ErrCapExceeded path. Per §2.25
	// the caller MUST also call EnvoyGoFailuresInc.
	DynamicStatsCapExceededInc()

	// HttpCallResponseAfterCloseInc bumps the defensive late-response
	// counter per 25.2 SPEC §7.1 row 14 + AMEND-B3. Wire name:
	// `wasm.<plugin>.http_call_response_after_close`. Called from
	// http_call.go handleHttpCallResponse on the token-miss / stream-gone
	// path. Near-zero in healthy operation; non-zero signal pages an
	// operator that envoy-go's cancellation has a bug.
	HttpCallResponseAfterCloseInc()

	// EnvoyGoFailuresInc bumps the existing 25.1 envoy_go.failures counter.
	// Wire name: `wasm.<plugin>.envoy_go.failures`. Per §2.25 ANY envoy-go-
	// strict surface that fails a stream MUST also count against this
	// generic counter so operators see one number reflecting all wasm-
	// driven stream failures. Co-incremented with SharedDataCapExceededInc
	// + DynamicStatsCapExceededInc + the panic-recovery path in
	// CallForeignFunction.
	EnvoyGoFailuresInc()
}

// noopStatsRecorder is the default RootStatsRecorder used when no
// WithRootStats option is applied at NewRootVM. All methods are no-ops; the
// underlying counter writes are skipped at zero cost. Used by the standalone
// wasm-package tests that don't need to observe counter increments + by
// fixtures that don't construct a *filterStats.
//
// Per Q9 + ADR-0085 nil-tolerance: the no-op default ensures rv.stats is
// NEVER nil, so hostcall bodies can call rv.stats.XxxInc() without a
// nil-check guard.
type noopStatsRecorder struct{}

func (noopStatsRecorder) TickInvocationsInc()                {}
func (noopStatsRecorder) HttpCallDispatchedInc()             {}
func (noopStatsRecorder) HttpCallResponseInc()               {}
func (noopStatsRecorder) ForeignFunctionDeniedInc()          {}
func (noopStatsRecorder) BodyBufferCapExceededInc()          {}
func (noopStatsRecorder) HttpCallDispatchUnknownClusterInc() {}
func (noopStatsRecorder) SharedDataCapExceededInc()          {}
func (noopStatsRecorder) DynamicStatsCapExceededInc()        {}
func (noopStatsRecorder) HttpCallResponseAfterCloseInc()     {}
func (noopStatsRecorder) EnvoyGoFailuresInc()                {}

// WithRootStats installs the per-RootVM RootStatsRecorder. The filter-side
// *filterStats satisfies this interface via thin per-counter wrapper methods
// at internal/filter/http/wasm/stats.go; compiled_config.New passes
// WithRootStats(cfg.stats) to thread the per-plugin counters into the
// RootVM.
//
// Default (when no option supplied): noopStatsRecorder. All recorder method
// invocations are no-ops; useful for unit-test fixtures that don't need to
// observe counter increments.
func WithRootStats(r RootStatsRecorder) RootVMOption {
	return func(rv *RootVM) {
		if r == nil {
			rv.stats = noopStatsRecorder{}
			return
		}
		rv.stats = r
	}
}

// Compile-time guard: noopStatsRecorder satisfies RootStatsRecorder.
var _ RootStatsRecorder = noopStatsRecorder{}
