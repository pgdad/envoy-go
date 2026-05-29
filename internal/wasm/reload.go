// reload.go — the per-*RootVM reload state machine for failure_policy =
// FAIL_RELOAD per phase-25.3 Task 3 (AMEND-C3; R-25.3-2/5; D-25.3-P3).
//
// # Upstream model (proxy-wasm-cpp-host wasm.cc)
//
// When a guest hits a RuntimeError (a wazero trap) under FAIL_RELOAD, the VM
// transitions to a Failed state; the NEXT request-driven dispatch attempts a
// reload (re-instantiate the module + replay proxy_on_vm_start +
// proxy_on_configure), rate-limited by a backoff. envoy-go-strict adds a
// 100ms floor on the backoff base interval. Three counters track the path:
// vm_reload_success / vm_reload_runtime_failure / vm_reload_backoff.
//
// # This file — the PURE state machine (fully unit-testable)
//
// reload.go is deliberately free of any wazero/runtime coupling: it models
// the Running → Failed → (backoff | attempt) → Running/Failed transitions
// over a clock.Clock seam so the transitions can be driven deterministically
// with a FakeClock. The production re-instantiation primitive + the
// dispatchMu-serialized dispatch hook live on *RootVM (root_vm.go); the
// state machine here is the load-bearing decision logic those wire into.
//
// # Determinism choice (envoy-go-strict)
//
// Upstream uses a JitteredLowerBoundBackOffStrategy whose interval is
// base_interval + a random jitter in [0, base_interval). envoy-go-strict
// OMITS the jitter (NextInterval returns exactly baseInterval()) so the
// backoff window is deterministic for differential-fixture + unit-test
// fake-time control. FAIL_RELOAD does not exponentially grow the interval
// (base_interval is the floor AND the steady-state value); upstream's
// max_interval is DEAD for this path and is not implemented.

package wasm

import (
	"sync"
	"time"

	"github.com/esalaine/envoy-go/internal/clock"
)

// reloadStateEnum is the lifecycle state of a *RootVM's reload machine.
type reloadStateEnum int

const (
	// reloadRunning — the VM is live + serving; no reload pending.
	reloadRunning reloadStateEnum = iota
	// reloadReloading — a reload attempt is in flight. Declared for parity
	// with the upstream tri-state; the dispatchMu serialization means the
	// attempt completes synchronously inside the held frame, so callers
	// observe only Running ↔ Failed in practice. Retained as a named state
	// for future async-reload extensions.
	reloadReloading //nolint:unused // reserved for future async-reload state
	// reloadFailed — the VM trapped under FAIL_RELOAD; the next dispatch
	// past the backoff window attempts a reload.
	reloadFailed
)

// reloadDecision is the verdict returned by reloadState.decide() at a
// dispatch entry.
//
// NOTE: reloadDecisionServe MUST be the zero value (iota 0) so that an
// uninitialized reloadDecision never accidentally triggers a reload attempt
// or a backoff counter — the benign pass-through is the safe default.
type reloadDecision int

const (
	// reloadDecisionServe — Running state: dispatch proceeds normally.
	reloadDecisionServe reloadDecision = iota
	// reloadDecisionAttempt — past the backoff window in Failed state: the
	// caller should attempt a re-instantiation now.
	reloadDecisionAttempt
	// reloadDecisionBackoff — in Failed state but still inside the backoff
	// window: the caller should NOT attempt; serve per failure policy +
	// count vm_reload_backoff.
	reloadDecisionBackoff
)

// FailurePolicy mirrors the upstream proxy-wasm failure-policy enum ordering
// (envoy.extensions.wasm.v3.PluginConfig.FailurePolicy). Task 7 consumes
// these; declared here as the canonical wasm-package source of truth.
type FailurePolicy int

const (
	// FailurePolicyUnspecified is the proto default (treated as FAIL_CLOSED
	// upstream).
	FailurePolicyUnspecified FailurePolicy = 0
	// FailurePolicyFailReload re-instantiates the VM on a RuntimeError,
	// rate-limited by the reload backoff.
	FailurePolicyFailReload FailurePolicy = 1
	// FailurePolicyFailClosed fails the stream (never reloads).
	FailurePolicyFailClosed FailurePolicy = 2
	// FailurePolicyFailOpen bypasses the filter (never reloads).
	FailurePolicyFailOpen FailurePolicy = 3
)

// FailState classifies WHY a VM is failed. Only FailStateRuntimeError is
// reload-eligible (and only under FAIL_RELOAD); start/config failures are
// terminal (a reload would just re-trip them).
type FailState int

const (
	// FailStateOK — the VM is healthy.
	FailStateOK FailState = iota
	// FailStateRuntimeError — a guest trap (wazero RuntimeError) at dispatch.
	// The ONLY reload-eligible fail-state.
	FailStateRuntimeError
	// FailStateStartFailed — proxy_on_vm_start / instantiation failed.
	// Terminal; not reload-eligible.
	FailStateStartFailed
)

// reloadEligible reports whether a (policy, fail-state) pair should trigger a
// reload. True IFF policy == FAIL_RELOAD AND fs == RuntimeError. FAIL_CLOSED /
// FAIL_OPEN never reload; non-RuntimeError fail-states never reload.
func reloadEligible(policy FailurePolicy, fs FailState) bool {
	return policy == FailurePolicyFailReload && fs == FailStateRuntimeError
}

// reloadBackoff is the base_interval-only backoff used by the reload state
// machine. envoy-go-strict applies a 100ms floor to the base interval + maps
// the unspecified (op == 0) case to a 1s default.
type reloadBackoff struct {
	base time.Duration
}

const (
	// reloadBackoffFloor is the envoy-go-strict floor on the base interval
	// (a non-zero operator-configured interval below 100ms is clamped up).
	reloadBackoffFloor = 100 * time.Millisecond
	// reloadBackoffDefault is the base interval applied when the operator
	// leaves the interval unspecified (op == 0).
	reloadBackoffDefault = 1 * time.Second
)

// newReloadBackoff constructs a reloadBackoff from the operator-configured
// base interval `op`. op == 0 ⇒ 1s default; 0 < op < 100ms ⇒ 100ms floor;
// op >= 100ms ⇒ op verbatim.
func newReloadBackoff(op time.Duration) *reloadBackoff {
	return &reloadBackoff{base: op}
}

// baseInterval returns the effective base interval after the floor +
// unspecified-default rules: op==0 → 1s; 0<op<100ms → 100ms; op>=100ms → op.
func (b *reloadBackoff) baseInterval() time.Duration {
	if b.base == 0 {
		return reloadBackoffDefault
	}
	// Negative base values also fall through to this branch and are clamped up
	// to the floor — deliberate: any sub-floor (or negative) operator value is
	// silently raised to the 100ms minimum rather than panicking or erroring.
	if b.base < reloadBackoffFloor {
		return reloadBackoffFloor
	}
	return b.base
}

// NextInterval returns the backoff interval for the next reload window. Per
// the envoy-go-strict determinism choice it returns exactly baseInterval()
// (jitter OMITTED — see the file header). FAIL_RELOAD does not exponentially
// grow the interval, so this is also the steady-state value.
func (b *reloadBackoff) NextInterval() time.Duration {
	return b.baseInterval()
}

// reloadStatsHook is a LOCAL nil-tolerant counter seam for the three
// vm_reload counters. Defined here (NOT on RootStatsRecorder) so this Task
// does not force the filter-package recorder to grow before Task 8. Task 8
// makes the real recorder satisfy this shape; Task 9 wires it via
// WithReloadStats. All call sites guard with `if rv.reloadStats != nil`.
type reloadStatsHook interface {
	VmReloadSuccessInc()
	VmReloadRuntimeFailureInc()
	VmReloadBackoffInc()
}

// reloadState is the per-*RootVM reload state machine. Its own mu guards the
// st / lastLoad fields (separate from the RootVM dispatchMu; lock order is
// dispatchMu → reload.mu). clk is the time source (RealClock in production;
// FakeClock in tests).
type reloadState struct {
	clk      clock.Clock
	mu       sync.Mutex
	st       reloadStateEnum
	lastLoad time.Time
	backoff  *reloadBackoff
}

// newReloadState constructs a reloadState in the Running state with a backoff
// built from the operator base interval `op`.
func newReloadState(clk clock.Clock, op time.Duration) *reloadState {
	return &reloadState{
		clk:     clk,
		st:      reloadRunning,
		backoff: newReloadBackoff(op),
	}
}

// state returns the current reload state (lock-guarded read).
func (rs *reloadState) state() reloadStateEnum {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.st
}

// noteRuntimeError transitions Running → Failed and records lastLoad = now so
// the backoff window starts. Idempotent if already Failed (refreshes lastLoad
// — documented choice: a fresh trap restarts the window rather than letting a
// stale window expire early).
func (rs *reloadState) noteRuntimeError() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.st = reloadFailed
	rs.lastLoad = rs.clk.Now()
}

// decide returns the dispatch verdict (lock-guarded):
//   - Running                                  → Serve
//   - Failed, within the backoff window        → Backoff
//   - Failed, past the backoff window           → Attempt
func (rs *reloadState) decide() reloadDecision {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.st == reloadRunning {
		return reloadDecisionServe
	}
	// Failed (or the unused Reloading state, treated like Failed for the
	// window check): consult the backoff window.
	if rs.clk.Now().Sub(rs.lastLoad) < rs.backoff.NextInterval() {
		return reloadDecisionBackoff
	}
	return reloadDecisionAttempt
}

// markReloaded transitions back to Running after a successful re-instantiation.
func (rs *reloadState) markReloaded() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.st = reloadRunning
}

// markReloadFailed stays Failed + refreshes lastLoad so the next attempt waits
// another full backoff window from now.
func (rs *reloadState) markReloadFailed() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.st = reloadFailed
	rs.lastLoad = rs.clk.Now()
}
