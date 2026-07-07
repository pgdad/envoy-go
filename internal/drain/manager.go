package drain

import (
	"sync"
	"sync/atomic"
	"time"
)

// State is the drain-state-machine state. Atomically loaded via atomic.Uint32.
type State uint32

const (
	// StateLive is the initial state at New(). The proxy is accepting new
	// connections and processing requests normally.
	StateLive State = iota
	// StateDraining is the post-Drain() state. The proxy rejects new
	// connections via accept-then-FIN; in-flight requests complete normally.
	//
	// There is deliberately no "drained" State value: the post-rendezvous
	// condition is NOT exposed via State() per SPEC §5.9 design — it is
	// observable only via the Done() channel close. State() continues to
	// return StateDraining post-rendezvous.
	StateDraining
)

// Manager is the drain-state machine. Allocated once at boot per phase 08.2
// SPEC §5.1 boot-order; consumed by admin.Server, listener.Manager, HCM
// filter, TCP-proxy filter (per BRAINSTORM Decision 4 surface-area).
//
// Lock-free hot path: State, IsDraining, Inc, Dec are atomic operations.
// The only synchronization beyond atomics is sync.Once on Drain (so
// concurrent triggers from handleDrainListeners + the SIGTERM-handler are
// safe) and a sync.Once-equivalent guard on the done-channel close.
type Manager struct {
	state     atomic.Uint32 // load/store of State as uint32
	inflight  atomic.Int64
	done      chan struct{}
	timeout   time.Duration
	once      sync.Once // guards Drain transition
	closeOnce sync.Once // guards close(done)
}

// New constructs a Manager in StateLive with the given drain timeout. Callers
// should pass timeout > 0; the Manager does NOT defensively panic or clamp on
// timeout <= 0 (per planner-time decision 2; SPEC §12 #2). The
// SIGTERM-handler in cmd/envoy-go/main.go always passes 30 * time.Second
// per ADR-0095; test code may pass small values like 10 * time.Millisecond.
//
// The timeout is consulted by the SIGTERM-handler (the Manager itself does
// NOT enforce the timeout — callers select on time.After(m.Timeout())
// alongside <-m.Done() per ADR-0095 design).
func New(timeout time.Duration) *Manager {
	return &Manager{
		done:    make(chan struct{}),
		timeout: timeout,
	}
}

// State atomically loads the current state. Lock-free. Returns StateLive or
// StateDraining (the post-rendezvous "drained" condition is NOT exposed via
// this method per SPEC §5.9; observe it via Done() instead).
func (m *Manager) State() State {
	return State(m.state.Load())
}

// IsDraining is the Listener Accept-loop fast-path check. Equivalent to
// State() == StateDraining. Lock-free; one atomic load.
func (m *Manager) IsDraining() bool {
	return State(m.state.Load()) == StateDraining
}

// Drain transitions the state from StateLive to StateDraining. Idempotent
// (sync.Once-guarded): on the first call, atomically transitions state and
// closes done if inflight is already 0. Subsequent calls no-op. On a Drain
// when inflight > 0, the matching Dec call (when it brings inflight to 0)
// closes done.
func (m *Manager) Drain() {
	m.once.Do(func() {
		m.state.Store(uint32(StateDraining))
		// If inflight is already 0, the rendezvous fires immediately.
		if m.inflight.Load() == 0 {
			m.closeOnce.Do(func() { close(m.done) })
		}
	})
}

// Done returns a channel that is closed when the drain rendezvous fires —
// i.e., when inflight reaches 0 after Drain has been called. If Drain has
// not been called, Done is open and never closes (until Drain is called).
// If Drain is called when inflight is already 0, Done closes immediately.
// The channel is closed exactly once (closeOnce-guarded).
func (m *Manager) Done() <-chan struct{} {
	return m.done
}

// Inc atomically increments the inflight counter. Called by HCM at request-
// begin and by TCP-proxy at conn-begin. Lock-free.
//
// Callers must check IsDraining() == false before calling Inc(); a late Inc
// arriving after the drain rendezvous has fired (Done() closed) will not
// reopen Done(), and the matching Dec will silently no-op the close-guard.
// The Listener Accept-loop fast-path (Task 5) and the HCM/TCP-proxy filter
// hooks (Tasks 9, 10) check IsDraining() before invoking Inc().
func (m *Manager) Inc() {
	m.inflight.Add(1)
}

// Dec atomically decrements the inflight counter. Called by HCM at request-
// end and by TCP-proxy at conn-end. If the decrement brings inflight to 0
// AND Drain has fired, this method closes the done channel (closeOnce-
// guarded). Lock-free.
func (m *Manager) Dec() {
	if m.inflight.Add(-1) == 0 && State(m.state.Load()) == StateDraining {
		m.closeOnce.Do(func() { close(m.done) })
	}
}

// Timeout returns the configured drain timeout (the value passed to New).
// Read-only; never changes after construction. The SIGTERM-handler in
// cmd/envoy-go/main.go calls this to set the time.After bound.
func (m *Manager) Timeout() time.Duration {
	return m.timeout
}
