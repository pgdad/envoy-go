// Tests for the proxy_set_tick_period_milliseconds host shim per §5.1 #30 +
// Q5 envoy-go-strict 10ms floor + R-25.2-9. Coverage:
//
//   - Round-trip: periodMs forwards to timerHost.SetTickPeriod as
//     time.Duration.
//   - periodMs == 0 ⇒ SetTickPeriod(0) (cancel — host-side semantic).
//   - Result is always WasmResultOk (clamp is silent host-side).
//   - Non-timerHost Host25_2 value ⇒ WasmResultInternalFailure (programmer
//     error — wrong host wired in).
//
// The 10ms floor enforcement itself is verified at internal/wasm/tick_test.go
// (TestTick_10msFloorEnforcement); this file's job is the shim-level wire-
// shape contract.

package abi

import (
	"context"
	"testing"
	"time"
)

// fakeTimerHost records the last SetTickPeriod call.
type fakeTimerHost struct {
	called     bool
	lastPeriod time.Duration
	callCount  int
	allPeriods []time.Duration
}

func (h *fakeTimerHost) SetTickPeriod(p time.Duration) {
	h.called = true
	h.lastPeriod = p
	h.callCount++
	h.allPeriods = append(h.allPeriods, p)
}

func TestTimer_SetTickPeriod_RoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		periodMs uint32
		want     time.Duration
	}{
		{"zero cancels", 0, 0},
		{"below floor (5ms) forwards as 5ms", 5, 5 * time.Millisecond},
		{"floor exact (10ms)", 10, 10 * time.Millisecond},
		{"above floor (100ms)", 100, 100 * time.Millisecond},
		{"large value (1 minute)", 60_000, 60 * time.Second},
		{"max-uint32 boundary", 4_294_967_295, time.Duration(4_294_967_295) * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			th := &fakeTimerHost{}
			res := SetTickPeriodMillisecondsShim(context.Background(), nil, th, tc.periodMs)
			if res != WasmResultOk {
				t.Errorf("result = %v; want WasmResultOk", res)
			}
			if !th.called {
				t.Errorf("SetTickPeriod not invoked")
			}
			if th.lastPeriod != tc.want {
				t.Errorf("SetTickPeriod period = %v; want %v", th.lastPeriod, tc.want)
			}
		})
	}
}

func TestTimer_SetTickPeriod_NonHostValue(t *testing.T) {
	// Pass a non-timerHost value (string) — should return InternalFailure.
	res := SetTickPeriodMillisecondsShim(context.Background(), nil, "not a host", 100)
	if res != WasmResultInternalFailure {
		t.Errorf("result = %v; want WasmResultInternalFailure", res)
	}
}

func TestTimer_SetTickPeriod_NilHost(t *testing.T) {
	// Pass nil Host25_2 — should NOT panic; should return InternalFailure.
	res := SetTickPeriodMillisecondsShim(context.Background(), nil, nil, 100)
	if res != WasmResultInternalFailure {
		t.Errorf("result = %v; want WasmResultInternalFailure", res)
	}
}
