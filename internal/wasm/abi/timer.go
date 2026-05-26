// Package abi — timer.go: proxy_set_tick_period_milliseconds hostcall
// dispatch shim per 25.2 SPEC §5.1 #30 + Q5 envoy-go-strict 10ms floor +
// R-25.2-9.
//
// # The single host shim landed at Task 5
//
//   - SetTickPeriodMillisecondsShim — proxy_set_tick_period_milliseconds(
//     period_ms) -> WasmResult. Validates the period (uint32; 0 cancels;
//     1-9 clamps to 10ms host-side); delegates to the per-*RootVM tick
//     dispatcher via the timerHost interface.
//
// # Q5 envoy-go-strict 10ms floor (per R-25.2-9)
//
// Below-floor periods are silently clamped host-side (NOT rejected). This
// matches upstream cpp-host's permissive semantic but adds the 10ms floor
// per envoy-go-strict defaults (prevents guest-driven CPU spin attacks via
// `proxy_set_tick_period_milliseconds(1)`-style abuse). The clamp happens
// at the per-RootVM SetTickPeriod method (tick.go); this shim's job is
// simply to forward the uint32 → time.Duration conversion.
//
// # Decoupling from internal/wasm
//
// abi/ MUST NOT import internal/wasm (would induce a cycle — wasm imports
// abi). The shim accepts a `Host25_2 any` parameter (declared in
// stubs_25_2.go) carrying an opaque value the registration.go envelope
// passes through unchanged (typically `*RootVM`). The shim type-asserts to
// a package-private `timerHost` interface; if the value does not satisfy
// the interface, the shim returns WasmResultInternalFailure (programmer
// error — wrong host wired in).
package abi

import (
	"context"
	"time"

	"github.com/tetratelabs/wazero/api"
)

// timerHost is the package-private interface the timer shim dispatches
// through. Satisfied by `*wasm.RootVM` (via the SetTickPeriod method on
// the RootVM type — see internal/wasm/tick.go) so the registration.go
// envelope can forward its `vm` argument unchanged. The shim type-asserts
// the opaque Host25_2 argument to this interface; non-satisfaction returns
// WasmResultInternalFailure.
type timerHost interface {
	// SetTickPeriod (re-)schedules the per-*RootVM tick goroutine.
	// period <= 0 cancels; 0 < period < TickPeriodFloor clamps to the
	// floor; period >= TickPeriodFloor uses as-is. See internal/wasm/
	// tick.go for the full semantic.
	SetTickPeriod(period time.Duration)
}

// SetTickPeriodMillisecondsShim — proxy_set_tick_period_milliseconds
// dispatch per §5.1 #30 + Q5 envoy-go-strict 10ms floor + R-25.2-9.
//
// Wire-shape (cpp-host include/proxy-wasm/exports.h):
//
//	Word set_tick_period_milliseconds(Word period_ms)
//
// Semantics (per Q5):
//
//   - periodMs == 0 ⇒ CANCEL (host-side: SetTickPeriod(0) stops the tick
//     goroutine; no replacement spawned).
//   - 0 < periodMs < 10 ⇒ CLAMP to 10ms host-side (envoy-go-strict floor;
//     prevents guest-driven CPU spin attacks).
//   - periodMs >= 10 ⇒ USE AS-IS (host-side: SetTickPeriod re-schedules
//     the tick goroutine with the requested cadence).
//
// Always returns WasmResultOk — the host accepts every periodMs value
// (clamp is silent). Matches upstream cpp-host semantic
// (src/exports.cc:set_tick_period_milliseconds).
//
// The `_ api.Module` parameter is present for wire-shape parity with the
// other 25.2 shims (the registration.go envelope passes mod
// unconditionally); the timer shim does NOT read guest memory.
func SetTickPeriodMillisecondsShim(_ context.Context, _ api.Module, host Host25_2, periodMs uint32) WasmResult {
	th, ok := host.(timerHost)
	if !ok {
		return WasmResultInternalFailure
	}
	// uint32 → time.Duration conversion. periodMs == 0 cancels; below-
	// floor clamping happens host-side at SetTickPeriod per Q5.
	//
	//nolint:gosec // uint32 → int64 fits (max ~4.29B ms ~ 49 days; no overflow).
	th.SetTickPeriod(time.Duration(periodMs) * time.Millisecond)
	return WasmResultOk
}
