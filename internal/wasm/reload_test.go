// Tests for the per-*RootVM reload state machine (reload.go) per phase-25.3
// Task 3 (R-25.3-2/5; D-25.3-P3). The pure state machine + base_interval
// backoff + 100ms floor + RuntimeError-gating are unit-testable in isolation;
// the dispatchMu-serialized hook is proven via a -race serialization test
// against a real minimal *RootVM.

package wasm

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/internal/clock"
)

func TestReloadBackoff_BaseIntervalFloor(t *testing.T) {
	cases := []struct{ op, want time.Duration }{
		{0, time.Second},
		{50 * time.Millisecond, 100 * time.Millisecond},
		{250 * time.Millisecond, 250 * time.Millisecond},
	}
	for _, tc := range cases {
		b := newReloadBackoff(tc.op)
		if got := b.baseInterval(); got != tc.want {
			t.Fatalf("op=%v: baseInterval=%v want %v", tc.op, got, tc.want)
		}
	}
}

func TestReloadStateMachine_BackoffThenReload(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0))
	rs := newReloadState(fc, 200*time.Millisecond)
	rs.noteRuntimeError()
	if rs.state() != reloadFailed {
		t.Fatal("want Failed after RuntimeError")
	}
	fc.Advance(100 * time.Millisecond)
	if d := rs.decide(); d != reloadDecisionBackoff {
		t.Fatalf("at t=100ms within 200ms window: decide=%v want backoff", d)
	}
	fc.Advance(150 * time.Millisecond)
	if d := rs.decide(); d != reloadDecisionAttempt {
		t.Fatalf("at t=250ms past 200ms window: decide=%v want attempt", d)
	}
}

func TestReload_GatedToRuntimeErrorUnderFailReload(t *testing.T) {
	if !reloadEligible(FailurePolicyFailReload, FailStateRuntimeError) {
		t.Fatal("FAIL_RELOAD + RuntimeError must be reload-eligible")
	}
	if reloadEligible(FailurePolicyFailReload, FailStateStartFailed) {
		t.Fatal("FAIL_RELOAD + StartFailed must NOT be reload-eligible")
	}
	if reloadEligible(FailurePolicyFailClosed, FailStateRuntimeError) {
		t.Fatal("FAIL_CLOSED never reloads")
	}
}

// TestReload_SerializedUnderDispatchMu proves D-25.3-P3: because the whole
// decide+attempt sequence holds dispatchMu, exactly one goroutine ever
// reloads a given VM; the rest block then observe the post-reload Running
// state and get Serve. Must be -race clean.
func TestReload_SerializedUnderDispatchMu(t *testing.T) {
	ctx := context.Background()
	fc := clock.NewFakeClock(time.Unix(0, 0))
	rv := newRootVMForTick(t, fc, WithReloadConfig(200*time.Millisecond))

	// Put the VM into Failed at t=0, then advance past the backoff window so
	// decide() returns Attempt for the first goroutine in.
	rv.reload.noteRuntimeError()
	fc.Advance(300 * time.Millisecond)

	var attempts int32
	reattempt := func(context.Context) error {
		atomic.AddInt32(&attempts, 1)
		return nil
	}

	const N = 32
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			rv.dispatchReload(ctx, reattempt)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts=%d want exactly 1 (one goroutine reloads → Running; rest Serve)", got)
	}
	if rv.reload.state() != reloadRunning {
		t.Fatalf("after successful reload: state=%v want Running", rv.reload.state())
	}
}

// TestReload_ReinstantiatePrimitive exercises the production re-instantiation
// primitive (the reattempt production callers pass to dispatchReload at Task
// 9): after a Configure, reinstantiate must re-instantiate the module + replay
// the lifecycle without error, and dispatchReload(rv.reinstantiate) on a
// Failed-past-window VM must transition it back to Running.
func TestReload_ReinstantiatePrimitive(t *testing.T) {
	ctx := context.Background()
	fc := clock.NewFakeClock(time.Unix(0, 0))
	rv := newRootVMForTick(t, fc, WithReloadConfig(200*time.Millisecond))

	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Direct primitive call (under dispatchMu, as reinstantiate requires).
	rv.dispatchMu.Lock()
	err := rv.reinstantiate(ctx)
	rv.dispatchMu.Unlock()
	if err != nil {
		t.Fatalf("reinstantiate: %v", err)
	}

	// Drive it through dispatchReload as the production reattempt.
	rv.reload.noteRuntimeError()
	fc.Advance(300 * time.Millisecond)
	if d := rv.dispatchReload(ctx, rv.reinstantiate); d != reloadDecisionAttempt {
		t.Fatalf("dispatchReload decision=%v want attempt", d)
	}
	if rv.reload.state() != reloadRunning {
		t.Fatalf("after reinstantiate via dispatchReload: state=%v want Running", rv.reload.state())
	}
}
