package wasm

// bugfix_test.go — regression tests for the THREE production bugs surfaced by
// the differential test against reference Envoy v1.37.2 (phase-25.3 Task
// 11-fix). These tests exercise the REAL conditions the pre-existing unit
// tests missed:
//
//   - BUG-1: per-route build at first request runs newFilterStats against a
//     FROZEN stats.Registry (post-boot). The pre-existing tests used a
//     never-frozen test registry, so the NewCounter panic on a frozen
//     registry never surfaced. These tests freeze the registry first.
//
//   - BUG-2/BUG-3: the FAIL_RELOAD reload triplet + the trap-poison Close
//     guard, driven by a REAL trapping guest (buildTrappingProxyWasm executes
//     `unreachable`), NOT by injected reload state. The pre-existing reload
//     test called rv.NoteReloadRuntimeError() directly, never re-entering the
//     poisoned instance via teardown — so the Close cascade never surfaced.

import (
	"context"
	"fmt"
	gohttp "net/http"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	wasmcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/clock"
	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// BUG-1 — freeze-tolerant per-route / per-plugin stats registration.
// -----------------------------------------------------------------------------

// TestNewFilterStats_FrozenRegistry_NoPanic is the minimal direct repro of
// BUG-1: newFilterStats against a FROZEN registry must NOT panic. Before the
// fix, the Group-A per-plugin reg.NewCounter(base+...) calls hit
// checkFrozenLocked → panic("stats: registry frozen: cannot register ...").
func TestNewFilterStats_FrozenRegistry_NoPanic(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	reg.Freeze()

	var fs *filterStats
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("newFilterStats panicked on a FROZEN registry (BUG-1): %v", r)
			}
		}()
		fs = newFilterStats(reg, "perroute_plugin")
	}()

	if fs == nil {
		t.Fatal("newFilterStats returned nil for a non-nil frozen registry; want a populated *filterStats")
	}
	// The per-plugin counters must be allocated + usable (Inc must not panic),
	// per the post-Freeze NewCounterIfAbsent contract (it registers under lock
	// even when frozen).
	if fs.executions == nil || fs.envoyGoFailures == nil || fs.vmReloadBackoff == nil {
		t.Fatalf("frozen-registry filterStats has nil counters: executions=%v failures=%v backoff=%v",
			fs.executions, fs.envoyGoFailures, fs.vmReloadBackoff)
	}
	fs.executions.Inc()
	if got := fs.executions.Load(); got != 1 {
		t.Errorf("executions counter Inc/Load = %d; want 1 (counter must be usable post-freeze)", got)
	}
	// The post-freeze NewCounterIfAbsent contract appends under lock, so the
	// counter IS visible in a Walk of the frozen registry.
	if got := findStatCounterValue(reg, "wasm.perroute_plugin.executions"); got != 1 {
		t.Errorf("Walk(wasm.perroute_plugin.executions) = %d; want 1 (NewCounterIfAbsent registers post-freeze)", got)
	}
}

// TestDispatch_PerRoute_FrozenRegistry_NoPanic mirrors the BUG-1 production
// stack end-to-end: a FROZEN listener registry + a per-route Wasm override
// proto whose first DecodeHeaders triggers the lazy per-route build
// (resolveEffective → parsePerRouteWasm → buildCompiledConfig → newFilterStats
// → reg.NewCounter on the FROZEN registry). Before the fix this panicked
// inside DecodeHeaders.
func TestDispatch_PerRoute_FrozenRegistry_NoPanic(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()

	// Build the listener config BEFORE freezing (boot-time registration path).
	listener := newTestCompiledConfig(t, buildContinueProxyWasm(), "plugin_listener_frozen", reg)
	listener.factoryCtx = envoyhttp.FactoryCtx{Stats: reg}
	t.Cleanup(func() { _ = listener.Close() })

	// Freeze the registry — simulating post-boot. Any per-route build now runs
	// against the frozen registry (the BUG-1 condition).
	reg.Freeze()

	overrideProto := buildWasmProtoInlineBytes(buildContinueProxyWasm(), "plugin_override_frozen")
	cb := &routeCfgDecoderCb{routeProto: overrideProto}
	f := &filter{cfg: listener}
	f.SetDecoderCallbacks(cb)
	f.SetEncoderCallbacks(fakeEncoderCb{})

	hdr := gohttp.Header{}
	hdr.Set(":method", "GET")

	var got envoyhttp.FilterHeadersStatus
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("DecodeHeaders panicked on the lazy per-route build against a FROZEN registry (BUG-1): %v", r)
			}
		}()
		got = f.DecodeHeaders(hdr, true)
	}()

	if got != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders (per-route, frozen reg) = %v; want Continue", got)
	}
	if f.eff == nil || f.eff == listener {
		t.Fatalf("f.eff did not resolve to the per-route override (eff=%p listener=%p)", f.eff, listener)
	}
	// The per-route plugin's executions counter was registered + incremented
	// post-freeze via NewCounterIfAbsent.
	if v := findStatCounterValue(reg, "wasm.plugin_override_frozen.executions"); v != 1 {
		t.Errorf("override executions (frozen reg) = %d; want 1", v)
	}
	f.OnDestroy()
}

// -----------------------------------------------------------------------------
// BUG-2 / BUG-3 — FAIL_RELOAD triplet + trap-poison Close guard, REAL trap.
// -----------------------------------------------------------------------------

// newRealTrapFailReloadCompiledConfig builds a FAIL_RELOAD compiledConfig whose
// guest TRAPS (buildTrappingProxyWasm) in proxy_on_request_headers. The
// lifecycle + per-stream + teardown capabilities are ALLOWED so the guest is
// actually reached (StrictDefaultDeny would otherwise short-circuit the
// dispatch to a benign no-op). A FakeClock is injected for deterministic
// backoff control.
func newRealTrapFailReloadCompiledConfig(t *testing.T, pluginName string, reg *stats.Registry, fc *clock.FakeClock, modBytes []byte) *compiledConfig {
	t.Helper()
	restore := withTestClock(fc)
	t.Cleanup(restore)

	cfg := &wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:          pluginName,
			RootId:        "test_root",
			FailurePolicy: wasmcommonv3.FailurePolicy_FAIL_RELOAD,
			ReloadConfig: &wasmcommonv3.ReloadConfig{
				Backoff: &corev3.BackoffStrategy{
					BaseInterval: durationpb.New(1 * time.Second),
				},
			},
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    fmt.Sprintf("test_vm_%s_%d", pluginName, testVMIDCounter.Add(1)),
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{
									InlineBytes: modBytes,
								},
							},
						},
					},
				},
			},
			CapabilityRestrictionConfig: &wasmcommonv3.CapabilityRestrictionConfig{
				AllowedCapabilities: map[string]*wasmcommonv3.SanitizationConfig{
					"proxy_on_vm_start":         {},
					"proxy_on_configure":        {},
					"proxy_on_context_create":   {},
					"proxy_on_request_headers":  {},
					"proxy_on_response_headers": {},
					"proxy_on_done":             {},
					"proxy_on_log":              {},
					"proxy_on_delete":           {},
				},
			},
		},
	}
	tc, err := anypb.New(cfg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	cc, err := buildCompiledConfig(context.Background(), tc, envoyhttp.FactoryCtx{Stats: reg})
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	return cc
}

// TestStreamContext_RealTrap_NoCloseCascade is the minimal BUG-3 repro driven
// through the filter: a real trapping guest traps in proxy_on_request_headers
// (DecodeHeaders returns a 503 disposition under FAIL_CLOSED-shared semantics);
// the subsequent OnDestroy → streamCtx.Close MUST NOT cascade by re-entering
// the poisoned instance via proxy_on_done/log/delete. Before the fix, Close
// fired the teardown triplet unconditionally → second trap → cascade.
func TestStreamContext_RealTrap_NoCloseCascade(t *testing.T) {
	// NOT t.Parallel(): withTestClock mutates package-level state.
	reg := stats.NewRegistry()
	fc := clock.NewFakeClock(time.Unix(3000, 0))
	cc := newRealTrapFailReloadCompiledConfig(t, "plugin_realtrap_close", reg, fc, buildTrappingProxyWasm())
	t.Cleanup(func() { _ = cc.Close() })

	cb := &routeCfgDecoderCb{}
	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(cb)
	f.SetEncoderCallbacks(fakeEncoderCb{})

	hdr := gohttp.Header{}
	hdr.Set(":method", "GET")

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("DecodeHeaders + OnDestroy cascaded a panic on a trapped instance (BUG-3): %v", r)
			}
		}()
		// The guest traps in proxy_on_request_headers → FAIL_CLOSED 503.
		status := f.DecodeHeaders(hdr, true)
		if status != envoyhttp.StopIteration {
			t.Errorf("DecodeHeaders (trap) = %v; want StopIteration (FAIL_CLOSED 503)", status)
		}
		// OnDestroy → streamCtx.Close MUST skip the teardown triplet (the
		// instance is poisoned). No cascade.
		f.OnDestroy()
	}()

	if cb.localReplyStatus != gohttp.StatusServiceUnavailable {
		t.Errorf("SendLocalReply status = %d; want 503", cb.localReplyStatus)
	}
}

// TestDispatch_RealTrap_ReloadTripletEngages is the core BUG-2 verification: a
// REAL trapping guest under FAIL_RELOAD, driven through the production
// DecodeHeaders dispatch path + a FakeClock, must engage the vm_reload triplet
// end-to-end:
//
//   - req1: guest traps → 503 → NoteReloadRuntimeError marks Failed (no
//     triplet counter yet — runtime_failure counts FAILED reload ATTEMPTS).
//   - req2: within backoff → vm_reload_backoff++ → 503 (no reload attempt).
//   - advance clock past backoff.
//   - req3: past backoff → reload Attempt → reinstantiate (fresh instance) →
//     but the SAME trapping module re-traps → still 503. We then verify the
//     attempt fired by asserting vm_reload_success>=1 OR runtime_failure>=1.
//
// Because the trapping module re-traps after reinstantiation (reinstantiate
// only replays the lifecycle, which succeeds — the trap is in
// proxy_on_request_headers, NOT in vm_start), the reinstantiation itself
// SUCCEEDS → vm_reload_success++. The re-trap on req3's header dispatch then
// re-arms Failed for the next window. The load-bearing assertion: the triplet
// engages (backoff>=1 AND success>=1) — proving the reload machine drives a
// real reinstantiation through the real dispatch path.
func TestDispatch_RealTrap_ReloadTripletEngages(t *testing.T) {
	// NOT t.Parallel(): withTestClock mutates package-level state.
	reg := stats.NewRegistry()
	fc := clock.NewFakeClock(time.Unix(4000, 0))
	cc := newRealTrapFailReloadCompiledConfig(t, "plugin_realtrap_reload", reg, fc, buildTrappingProxyWasm())
	t.Cleanup(func() { _ = cc.Close() })

	base := "wasm.plugin_realtrap_reload."

	// req1: guest traps → 503; the reload machine arms Failed.
	status1, cb1 := runDecode(cc)
	if status1 != envoyhttp.StopIteration {
		t.Fatalf("req1 (trap) = %v; want StopIteration (503)", status1)
	}
	if cb1.localReplyStatus != gohttp.StatusServiceUnavailable {
		t.Errorf("req1 SendLocalReply status = %d; want 503", cb1.localReplyStatus)
	}
	if got := findStatCounterValue(reg, base+"vm_reload_backoff"); got != 0 {
		t.Errorf("after req1: vm_reload_backoff = %d; want 0 (no dispatch within backoff yet)", got)
	}

	// req2: within the 1s backoff window (clock not advanced) → Backoff → 503.
	status2, _ := runDecode(cc)
	if status2 != envoyhttp.StopIteration {
		t.Fatalf("req2 (within backoff) = %v; want StopIteration (503)", status2)
	}
	if got := findStatCounterValue(reg, base+"vm_reload_backoff"); got < 1 {
		t.Errorf("after req2: vm_reload_backoff = %d; want >=1 (within-window dispatch)", got)
	}

	// Advance past the 1s backoff window.
	fc.Advance(1500 * time.Millisecond)

	// req3: past backoff → reload Attempt → reinstantiate succeeds (lifecycle
	// replay; the trap is in header dispatch) → vm_reload_success++. The fresh
	// instance then re-traps on the header dispatch → 503 again + re-arm.
	status3, _ := runDecode(cc)
	if status3 != envoyhttp.StopIteration {
		t.Fatalf("req3 (post-backoff, re-trap) = %v; want StopIteration (fresh instance re-traps)", status3)
	}

	backoff := findStatCounterValue(reg, base+"vm_reload_backoff")
	success := findStatCounterValue(reg, base+"vm_reload_success")
	failure := findStatCounterValue(reg, base+"vm_reload_runtime_failure")
	t.Logf("triplet after req3: backoff=%d success=%d runtime_failure=%d", backoff, success, failure)

	if backoff < 1 {
		t.Errorf("vm_reload_backoff = %d; want >=1 (BUG-2: backoff must engage)", backoff)
	}
	if success < 1 {
		t.Errorf("vm_reload_success = %d; want >=1 (BUG-2: a reinstantiation attempt past the backoff window must succeed)", success)
	}
}

// -----------------------------------------------------------------------------
// BUG-4 — context-create poison: reload reinstantiate never ran because
// proxy_on_context_create trapped on the poisoned instance FIRST.
// -----------------------------------------------------------------------------

// TestDispatch_ContextCreatePoison_ReloadStillRecovers is the BUG-4 repro. The
// existing TestDispatch_RealTrap_ReloadTripletEngages uses a guest that traps
// ONLY in proxy_on_request_headers — it does NOT poison proxy_on_context_create,
// so it missed BUG-4. This test uses buildContextCreatePoisonProxyWasm: the
// guest sets a mutable wasm global `poisoned=1` then traps in
// proxy_on_request_headers; proxy_on_context_create then traps too (on the SAME
// poisoned instance), exactly replicating the Rust-SDK RefCell-left-borrowed
// condition the differential test surfaced.
//
// Pre-fix DecodeHeaders order: initStreamContext (proxy_on_context_create) ran
// BEFORE ReloadDispatch. So req2/req3's context-create trapped on the poisoned
// instance → initStreamContext returned an error → DecodeHeaders fail-OPENed
// (Continue) at that branch → ReloadDispatch was NEVER reached → the reload
// triplet stayed 0 forever.
//
// Post-fix order: ReloadDispatch runs FIRST. A Failed VM past the backoff window
// reinstantiates a FRESH (un-poisoned) instance, THEN initStreamContext fires
// proxy_on_context_create on that fresh instance (global reset to 0) → it
// succeeds → the guest serves. The triplet (backoff>=1, success>=1) engages.
//
// Sequence:
//   - req1: proxy_on_request_headers poisons the instance + traps → 503 + Failed.
//   - req2 (within backoff): ReloadDispatch → vm_reload_backoff++ → 503 (NOT a
//     fail-OPEN-at-initStreamContext that skips the reload machine).
//   - advance the FakeClock past the backoff window.
//   - req3 (post-backoff): ReloadDispatch → reinstantiate (fresh instance, poison
//     cleared) → proxy_on_context_create succeeds → guest's request-headers hook
//     runs. With this poisoning guest the fresh instance's header hook poisons +
//     traps AGAIN → 503 + re-arm — but vm_reload_success++ recorded the
//     successful reinstantiation. The load-bearing assertion is the triplet
//     engaging (backoff>=1 AND success>=1), proving ReloadDispatch was reached
//     and reinstantiation ran on a context-create-poisoning guest.
//
// VERIFY: this test FAILS on the pre-reorder code (triplet stays 0 because
// req2/req3 fail at initStreamContext before ReloadDispatch) and PASSES after
// the reorder.
func TestDispatch_ContextCreatePoison_ReloadStillRecovers(t *testing.T) {
	// NOT t.Parallel(): withTestClock mutates package-level state.
	reg := stats.NewRegistry()
	fc := clock.NewFakeClock(time.Unix(5000, 0))
	cc := newRealTrapFailReloadCompiledConfig(t, "plugin_ctxcreate_poison", reg, fc, buildContextCreatePoisonProxyWasm())
	t.Cleanup(func() { _ = cc.Close() })

	base := "wasm.plugin_ctxcreate_poison."

	// req1: the guest poisons the instance + traps in proxy_on_request_headers
	// → 503 (FAIL_RELOAD shares FAIL_CLOSED for the trapping request) + the
	// reload machine arms Failed.
	status1, cb1 := runDecode(cc)
	if status1 != envoyhttp.StopIteration {
		t.Fatalf("req1 (poison+trap) = %v; want StopIteration (503)", status1)
	}
	if cb1.localReplyStatus != gohttp.StatusServiceUnavailable {
		t.Errorf("req1 SendLocalReply status = %d; want 503", cb1.localReplyStatus)
	}
	if got := findStatCounterValue(reg, base+"vm_reload_backoff"); got != 0 {
		t.Errorf("after req1: vm_reload_backoff = %d; want 0 (no dispatch within backoff yet)", got)
	}

	// req2: within the 1s backoff window (clock not advanced). The instance is
	// poisoned, so the PRE-FIX order would trap in proxy_on_context_create
	// (initStreamContext) and fail-OPEN before reaching ReloadDispatch. The
	// POST-FIX order reaches ReloadDispatch first → Backoff → 503.
	status2, _ := runDecode(cc)
	if status2 != envoyhttp.StopIteration {
		t.Fatalf("req2 (within backoff, poisoned instance) = %v; want StopIteration (503 from ReloadDispatch Backoff). "+
			"A Continue here is the BUG-4 signature: context-create trapped + fail-OPENed before ReloadDispatch", status2)
	}
	if got := findStatCounterValue(reg, base+"vm_reload_backoff"); got < 1 {
		t.Errorf("after req2: vm_reload_backoff = %d; want >=1. BUG-4: req2's proxy_on_context_create trapped on the "+
			"poisoned instance + fail-OPENed BEFORE ReloadDispatch ran, so the backoff counter never incremented", got)
	}

	// Advance past the 1s backoff window.
	fc.Advance(1500 * time.Millisecond)

	// req3: post-backoff → ReloadDispatch reinstantiates a FRESH (un-poisoned)
	// instance → proxy_on_context_create on the fresh instance succeeds → the
	// guest's request-headers hook runs (and, being this poisoning guest, traps
	// again → 503 + re-arm). vm_reload_success records the reinstantiation.
	status3, _ := runDecode(cc)
	if status3 != envoyhttp.StopIteration {
		t.Fatalf("req3 (post-backoff) = %v; want StopIteration (fresh instance re-traps in header hook)", status3)
	}

	backoff := findStatCounterValue(reg, base+"vm_reload_backoff")
	success := findStatCounterValue(reg, base+"vm_reload_success")
	failure := findStatCounterValue(reg, base+"vm_reload_runtime_failure")
	t.Logf("triplet after req3: backoff=%d success=%d runtime_failure=%d", backoff, success, failure)

	if backoff < 1 {
		t.Errorf("vm_reload_backoff = %d; want >=1 (BUG-4: backoff must engage even when context-create would trap)", backoff)
	}
	if success < 1 {
		t.Errorf("vm_reload_success = %d; want >=1 (BUG-4: reinstantiation must run on a fresh instance BEFORE context-create)", success)
	}
}
