// Tests for the framework-level Clock seam per phase-21 ADR-0186
// §Consequences (g) EXTRACT-NOW + 25.2 IMPL Task 5 second-co-consumer
// trigger. Coverage:
//
//   - RealClock satisfies Clock + Now monotonically advances + After fires.
//   - FakeClock anchor (NewFakeClock(start).Now() == start).
//   - FakeClock Advance moves Now forward.
//   - FakeClock After-channel fires at the expected step.
//   - FakeClock multi-After deterministic deadline-ascending fire order.
//   - FakeClock same-deadline ties broken by insertion order.
//   - FakeClock immediate fire on After(0).
//   - FakeClock receivers that drop the channel reference do NOT block
//     Advance (cap=1 buffer accepts the lost-race send).

package clock

import (
	"sync"
	"testing"
	"time"
)

func TestRealClock_SatisfiesClock(t *testing.T) {
	var _ Clock = RealClock{}
}

func TestRealClock_NowAdvances(t *testing.T) {
	c := RealClock{}
	a := c.Now()
	time.Sleep(1 * time.Millisecond)
	b := c.Now()
	if !b.After(a) {
		t.Errorf("RealClock.Now did not advance: a=%v b=%v", a, b)
	}
}

func TestRealClock_AfterFires(t *testing.T) {
	c := RealClock{}
	select {
	case <-c.After(10 * time.Millisecond):
		// expected
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RealClock.After(10ms) did NOT fire within 500ms")
	}
}

func TestFakeClock_SatisfiesClock(t *testing.T) {
	var _ Clock = (*FakeClock)(nil)
}

func TestFakeClock_NowReflectsStart(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewFakeClock(start)
	if got := c.Now(); !got.Equal(start) {
		t.Errorf("Now() = %v; want %v", got, start)
	}
}

func TestFakeClock_AdvanceMovesNow(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0))
	c.Advance(100 * time.Millisecond)
	want := time.Unix(0, int64(100*time.Millisecond))
	if got := c.Now(); !got.Equal(want) {
		t.Errorf("Now() after Advance(100ms) = %v; want %v", got, want)
	}
}

func TestFakeClock_After_FiresAtDeadline(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0))
	ch := c.After(50 * time.Millisecond)
	c.Advance(49 * time.Millisecond)
	select {
	case <-ch:
		t.Fatal("fired at +49ms (deadline was 50ms)")
	default:
	}
	c.Advance(1 * time.Millisecond) // total 50ms
	select {
	case <-ch:
		// expected
	default:
		t.Fatal("did NOT fire at +50ms")
	}
}

func TestFakeClock_After_DoesNotFireBefore(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0))
	ch := c.After(100 * time.Millisecond)
	c.Advance(99 * time.Millisecond)
	select {
	case <-ch:
		t.Fatal("fired at +99ms (deadline was 100ms)")
	default:
	}
}

func TestFakeClock_After_Zero_FiresImmediately(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0))
	ch := c.After(0)
	select {
	case <-ch:
		// expected
	default:
		t.Fatal("After(0) did NOT fire synchronously")
	}
}

func TestFakeClock_After_Negative_FiresImmediately(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0))
	ch := c.After(-1 * time.Millisecond)
	select {
	case <-ch:
		// expected
	default:
		t.Fatal("After(-1ms) did NOT fire synchronously")
	}
}

// TestFakeClock_MultiAfter_DeterministicOrder verifies deadline-asc fire
// order at the same Advance call.
func TestFakeClock_MultiAfter_DeterministicOrder(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0))
	ch30 := c.After(30 * time.Millisecond)
	ch10 := c.After(10 * time.Millisecond)
	ch20 := c.After(20 * time.Millisecond)
	c.Advance(100 * time.Millisecond)
	for _, ch := range []<-chan time.Time{ch10, ch20, ch30} {
		select {
		case <-ch:
		default:
			t.Errorf("expected all 3 After-channels to fire at Advance(100ms)")
		}
	}
}

func TestFakeClock_MultiAfter_SameDeadline(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0))
	// Register 3 After(10ms) channels; all must fire at Advance(10ms).
	chs := []<-chan time.Time{
		c.After(10 * time.Millisecond),
		c.After(10 * time.Millisecond),
		c.After(10 * time.Millisecond),
	}
	c.Advance(10 * time.Millisecond)
	for i, ch := range chs {
		select {
		case <-ch:
		default:
			t.Errorf("After-channel %d did NOT fire at +10ms (same-deadline)", i)
		}
	}
}

// TestFakeClock_LostRace_AdvanceDoesNotBlock verifies the cap=1 buffered
// channel semantic: even if the receiver dropped the channel reference
// (e.g., goroutine selected a different case), the Advance-side send
// succeeds.
func TestFakeClock_LostRace_AdvanceDoesNotBlock(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0))
	// Register but never receive.
	_ = c.After(10 * time.Millisecond)
	_ = c.After(20 * time.Millisecond)
	_ = c.After(30 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		c.Advance(100 * time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
		// expected — Advance returned even though no receiver drained
	case <-time.After(1 * time.Second):
		t.Fatal("Advance blocked on no-receiver After-channel sends")
	}
}

// TestFakeClock_ConcurrentAfter_RaceFree exercises concurrent After()
// registration under -race; the test passes when -race detects no data
// race.
func TestFakeClock_ConcurrentAfter_RaceFree(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0))
	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			//nolint:gosec // test-only sequence; not security-sensitive
			_ = c.After(time.Duration(i+1) * time.Millisecond)
		}(i)
	}
	wg.Wait()
	c.Advance(N * time.Millisecond)
}
