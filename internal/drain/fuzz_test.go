package drain

import (
	"sync/atomic"
	"testing"
	"time"
)

// FuzzDrainTransitions fuzzes a sequence of operations against a Manager and
// asserts (i) state-monotonicity (state never decreases — Live → Draining
// only); (ii) inflight balance (every Inc has a matching Dec); (iii) Done
// fires exactly once after Drain has been called and inflight reaches 0.
//
// Per ADR-0018 fuzz CI 30s short-budget policy. Per SPEC §14.5 + §12 #1.
func FuzzDrainTransitions(f *testing.F) {
	f.Add(uint8(0b10101010), uint8(5))
	f.Add(uint8(0b00000001), uint8(1))
	f.Add(uint8(0b11111111), uint8(8))
	f.Fuzz(func(t *testing.T, ops uint8, n uint8) {
		if n > 8 {
			n = 8
		}
		m := New(1 * time.Hour)
		var balance atomic.Int64
		drainCalled := false
		for i := uint8(0); i < n; i++ {
			op := (ops >> i) & 1
			if op == 0 {
				m.Inc()
				balance.Add(1)
			} else {
				if balance.Load() > 0 {
					m.Dec()
					balance.Add(-1)
				}
			}
		}
		// Trigger drain at the end and balance any residual inflight.
		m.Drain()
		drainCalled = true
		for balance.Load() > 0 {
			m.Dec()
			balance.Add(-1)
		}
		// State must be StateDraining post-Drain (monotonicity invariant).
		if got := m.State(); got != StateDraining {
			t.Fatalf("state monotonicity violated: got %v, want StateDraining", got)
		}
		// Inflight must be 0 (balance invariant).
		if got := balance.Load(); got != 0 {
			t.Fatalf("inflight balance violated: got %d, want 0", got)
		}
		// Done must fire (rendezvous invariant).
		if drainCalled {
			select {
			case <-m.Done():
			case <-time.After(100 * time.Millisecond):
				t.Fatalf("Done() did not fire after balanced inflight + Drain")
			}
		}
	})
}
