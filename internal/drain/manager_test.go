package drain

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStateTransitions(t *testing.T) {
	m := New(10 * time.Millisecond)
	if got := m.State(); got != StateLive {
		t.Errorf("initial State: got %v, want StateLive", got)
	}
	m.Drain()
	if got := m.State(); got != StateDraining {
		t.Errorf("post-Drain State: got %v, want StateDraining", got)
	}
	// Idempotent: another Drain stays in Draining.
	m.Drain()
	if got := m.State(); got != StateDraining {
		t.Errorf("post-second-Drain State: got %v, want StateDraining", got)
	}
}

func TestInflightBalance(t *testing.T) {
	m := New(10 * time.Millisecond)
	m.Inc()
	m.Inc()
	m.Dec()
	m.Dec()
	// No public accessor for raw inflight; balance is observed via Done()
	// rendezvous: after Drain(), if balance is 0, Done() closes.
	m.Drain()
	select {
	case <-m.Done():
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Done() did not fire after balanced Inc/Dec + Drain")
	}
}

func TestDrainCompletionRendezvous(t *testing.T) {
	m := New(10 * time.Millisecond)
	m.Inc()
	m.Drain()
	// Done() should NOT fire while inflight > 0.
	select {
	case <-m.Done():
		t.Fatalf("Done() fired while inflight > 0")
	case <-time.After(20 * time.Millisecond):
		// expected
	}
	m.Dec()
	select {
	case <-m.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Done() did not fire after Dec → 0 post-Drain")
	}
}

func TestDrainTimeout_NoInflight(t *testing.T) {
	m := New(1 * time.Hour) // huge timeout; no Inc; Done() should close immediately
	m.Drain()
	select {
	case <-m.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Done() did not fire when inflight already 0 at Drain time")
	}
}

func TestDrainTimeout_StuckInflight_CallerEnforces(t *testing.T) {
	// Per ADR-0095 design: the Manager itself does NOT enforce timeout.
	// The caller (cmd/envoy-go/main.go) selects on time.After alongside Done().
	m := New(10 * time.Millisecond)
	m.Inc() // never Dec'd
	m.Drain()
	select {
	case <-m.Done():
		t.Fatalf("Done() fired with stuck inflight; Manager should NOT auto-timeout")
	case <-time.After(time.Until(time.Now().Add(50 * time.Millisecond))):
		// expected — Manager does not enforce timeout itself
	}
	if got := m.Timeout(); got != 10*time.Millisecond {
		t.Errorf("Timeout(): got %v, want 10ms", got)
	}
}

func TestIdempotentDrain(t *testing.T) {
	m := New(10 * time.Millisecond)
	m.Drain()
	m.Drain()
	m.Drain()
	// Multiple Drain calls; only one transition fires; Done() closes once
	// (closeOnce-guarded). A re-close would panic — the test passes if no
	// panic occurs across the goroutine fan-in below.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m.Drain() }()
	}
	wg.Wait()
	select {
	case <-m.Done():
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Done() did not fire under concurrent Drain")
	}
}

func TestIsDrainingFastPath(t *testing.T) {
	m := New(10 * time.Millisecond)
	if m.IsDraining() {
		t.Errorf("IsDraining() pre-Drain: got true, want false")
	}
	m.Drain()
	if !m.IsDraining() {
		t.Errorf("IsDraining() post-Drain: got false, want true")
	}
}

func TestNilSafety(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on nil-receiver method call; got none")
		}
	}()
	var m *Manager
	_ = m.IsDraining() // pointer-receiver method on nil panics; this is documented behavior
}

func TestConcurrentIncDec(t *testing.T) {
	m := New(1 * time.Second)
	const N = 100
	const M = 1000
	var wg sync.WaitGroup
	var balanced atomic.Int64
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				m.Inc()
				balanced.Add(1)
				m.Dec()
				balanced.Add(-1)
			}
		}()
	}
	wg.Wait()
	if got := balanced.Load(); got != 0 {
		t.Errorf("Inc/Dec balance: got %d, want 0", got)
	}
	// After all Inc/Dec, drain should rendezvous immediately.
	m.Drain()
	select {
	case <-m.Done():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("Done() did not fire after balanced concurrent Inc/Dec + Drain")
	}
}
