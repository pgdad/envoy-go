package wasm

// dispatch_reload_perroute_test.go — phase-25.3 Task 9 integration tests for
// the two NEW dispatch-time behaviors wired at DecodeHeaders:
//
//  1. Per-route resolution: a stream whose route carries a per-route Wasm TPFC
//     override dispatches against the OVERRIDE *RootVM (not the listener one).
//
//  2. Reload-on-RuntimeError: under failure_policy = FAIL_RELOAD, a VM driven
//     into the Failed state serves FAIL_CLOSED (503) within the backoff window
//     (+ vm_reload_backoff progression); past the backoff window the next
//     request reloads → vm_reload_success + the subsequent stream succeeds.
//
//  3. FAIL_OPEN: a VM driven unavailable bypasses the filter (Continue, no 503).
//
// # Reload-test strategy (documented choice)
//
// No trapping-guest wasm fixture exists in this package (the fixtures —
// buildContinueProxyWasm / buildPauseProxyWasm / buildSendLocalResponseProxyWasm
// / buildMinimalProxyWasm — are all non-trapping; building a Rust/wasm panic
// fixture is disproportionate for this Task). Per the Task-9 PLAN's explicit
// allowance, we drive the reload DISPATCH disposition by injecting the *RootVM
// into a Failed state via the EXPORTED reload hook (rootVM.NoteReloadRuntimeError,
// which mirrors what the DecodeHeaders trap path itself calls) + a FakeClock
// (injected via withTestClock at buildCompiledConfig time) to advance past the
// deterministic backoff window. We then drive real DecodeHeaders calls and
// assert the FAIL_CLOSED (503) / FAIL_OPEN (bypass) / recover (reload-success +
// serve) dispositions + the vm_reload triplet counter progression. This
// exercises the EXACT production dispatch integration (resolveEffective →
// ReloadDispatch → applyFailureDisposition), differing from a real guest trap
// only in HOW the Failed state is reached (NoteReloadRuntimeError vs an actual
// wazero RuntimeError surfacing from CallProxyOnRequestHeaders — both call the
// same rv.reload.noteRuntimeError() under the hood).

import (
	"context"
	gohttp "net/http"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	wasmcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/esalaine/envoy-go/internal/clock"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
)

// -----------------------------------------------------------------------------
// route-config double: returns a fixed per-route proto.Message from
// RequestRouteConfig() so DecodeHeaders' resolveEffective takes the per-route
// branch. Embeds fakeDecoderCb for the rest of the DecoderFilterCallbacks
// surface; overrides RequestRouteConfig + SendLocalReply (for 503 assertions).
// -----------------------------------------------------------------------------

type routeCfgDecoderCb struct {
	fakeDecoderCb
	routeProto proto.Message

	// SendLocalReply capture (503 disposition assertions).
	localReplyCalls  int
	localReplyStatus int
}

func (r *routeCfgDecoderCb) RequestRouteConfig() proto.Message { return r.routeProto }

func (r *routeCfgDecoderCb) SendLocalReply(status int, _ string, _ envoyhttp.OrderedHeaders) {
	r.localReplyCalls++
	r.localReplyStatus = status
}

// -----------------------------------------------------------------------------
// Test 1: per-route override applies — dispatch against the override *RootVM.
// -----------------------------------------------------------------------------

// TestDispatch_PerRouteOverrideApplies builds a listener compiledConfig + a
// DISTINCT per-route *wasmv3.Wasm override (different vm_id + plugin name → a
// different registry *RootVM). A stream whose route returns the override proto
// from RequestRouteConfig must resolve f.eff to the per-route compiledConfig and
// dispatch its per-stream StreamContext against the OVERRIDE *RootVM — NOT the
// listener one. We prove this by asserting f.eff.rootVM != listener.rootVM and
// that the per-route plugin's executions counter (not the listener's) increments.
func TestDispatch_PerRouteOverrideApplies(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()

	// Listener config.
	listener := newTestCompiledConfig(t, buildContinueProxyWasm(), "plugin_listener_pr", reg)
	// Retain the live FactoryCtx (with the stats registry) so the per-route
	// build threads the same stats surface — mirrors production buildCompiledConfig.
	listener.factoryCtx = envoyhttp.FactoryCtx{Stats: reg}
	t.Cleanup(func() { _ = listener.Close() })

	// Per-route override proto: distinct plugin name + vm_id (so the registry
	// hands back a DIFFERENT *RootVM than the listener's).
	overrideProto := buildWasmProtoInlineBytes(buildContinueProxyWasm(), "plugin_override_pr")

	cb := &routeCfgDecoderCb{routeProto: overrideProto}
	f := &filter{cfg: listener}
	f.SetDecoderCallbacks(cb)
	f.SetEncoderCallbacks(fakeEncoderCb{})

	hdr := gohttp.Header{}
	hdr.Set(":method", "GET")
	if got := f.DecodeHeaders(hdr, true); got != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders = %v; want Continue", got)
	}

	// f.eff must be the per-route override (NOT the listener cfg).
	if f.eff == nil {
		t.Fatal("f.eff == nil after DecodeHeaders; want the per-route override config")
	}
	if f.eff == listener {
		t.Fatal("f.eff == listener cfg; want the per-route OVERRIDE config")
	}
	if f.eff.rootVM == listener.rootVM {
		t.Fatal("f.eff.rootVM == listener.rootVM; want a DISTINCT per-route *RootVM (override swaps the whole VM)")
	}
	if f.eff.pluginName != "plugin_override_pr" {
		t.Errorf("f.eff.pluginName = %q; want plugin_override_pr (per-route override)", f.eff.pluginName)
	}

	// The OVERRIDE plugin's executions counter incremented (the dispatch ran
	// against the override VM); the listener plugin's did NOT.
	if got := findStatCounterValue(reg, "wasm.plugin_override_pr.executions"); got != 1 {
		t.Errorf("override executions = %d; want 1 (dispatched against the override VM)", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_listener_pr.executions"); got != 0 {
		t.Errorf("listener executions = %d; want 0 (override took over the stream)", got)
	}

	// EncodeHeaders + OnDestroy must reuse the SAME effective config (the
	// per-stream StreamContext belongs to the override VM). Assert no panic +
	// active gauge returns to 0 on the override plugin.
	resp := gohttp.Header{}
	resp.Set(":status", "200")
	if got := f.EncodeHeaders(resp, true); got != envoyhttp.Continue {
		t.Errorf("EncodeHeaders = %v; want Continue", got)
	}
	f.OnDestroy()
	if got := findStatGaugeValue(reg, "wasm.wazero.active"); got != 0 {
		t.Errorf("active = %d after OnDestroy; want 0", got)
	}

	// Memo: a second stream on the same override proto pointer must REUSE the
	// memoized per-route config (build exactly once → no arm-26 false-reject).
	f2 := &filter{cfg: listener}
	f2.SetDecoderCallbacks(&routeCfgDecoderCb{routeProto: overrideProto})
	f2.SetEncoderCallbacks(fakeEncoderCb{})
	if got := f2.DecodeHeaders(hdr, true); got != envoyhttp.Continue {
		t.Fatalf("second stream DecodeHeaders = %v; want Continue", got)
	}
	if f2.eff != f.eff {
		t.Errorf("second stream f.eff = %p; want the SAME memoized per-route config %p", f2.eff, f.eff)
	}
	f2.OnDestroy()
}

// -----------------------------------------------------------------------------
// FAIL_RELOAD compiledConfig builder with an injected FakeClock.
// -----------------------------------------------------------------------------

// newFailReloadCompiledConfig builds a compiledConfig whose *RootVM has
// failure_policy = FAIL_RELOAD + a 1s reload backoff base interval, with the
// supplied FakeClock injected so the backoff window is deterministic. Returns
// the cc; the caller drives the reload state via cc.rootVM.NoteReloadRuntimeError
// + fc.Advance.
func newFailReloadCompiledConfig(t *testing.T, pluginName string, reg *stats.Registry, fc *clock.FakeClock) *compiledConfig {
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
					VmId:    "test_vm_" + pluginName,
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{
									InlineBytes: buildContinueProxyWasm(),
								},
							},
						},
					},
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
	if cc.failurePolicy != internalwasm.FailurePolicyFailReload {
		t.Fatalf("cc.failurePolicy = %d; want FailReload", cc.failurePolicy)
	}
	return cc
}

// runDecode drives a fresh per-stream filter through DecodeHeaders against the
// supplied cc + returns the status + the route-config double (for 503 capture).
func runDecode(cc *compiledConfig) (envoyhttp.FilterHeadersStatus, *routeCfgDecoderCb) {
	cb := &routeCfgDecoderCb{} // routeProto nil → listener cfg (no per-route)
	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(cb)
	f.SetEncoderCallbacks(fakeEncoderCb{})
	hdr := gohttp.Header{}
	hdr.Set(":method", "GET")
	status := f.DecodeHeaders(hdr, true)
	f.OnDestroy()
	return status, cb
}

// -----------------------------------------------------------------------------
// Test 2: FAIL_RELOAD trap → 503 within backoff → recover after window.
// -----------------------------------------------------------------------------

func TestDispatch_ReloadOnRuntimeError_FailClosedThenRecover(t *testing.T) {
	// NOT t.Parallel(): withTestClock mutates package-level testClock state
	// read synchronously by buildCompiledConfig's resolveClock(); a parallel
	// test swapping testClock would corrupt the clock injection. Mirrors the
	// tick_clock_test.go non-parallel discipline.
	reg := stats.NewRegistry()
	fc := clock.NewFakeClock(time.Unix(1000, 0))
	cc := newFailReloadCompiledConfig(t, "plugin_failreload", reg, fc)
	t.Cleanup(func() { _ = cc.Close() })

	// Stream 0: VM Running, guest OK → Continue (common path under FAIL_RELOAD).
	if status, _ := runDecode(cc); status != envoyhttp.Continue {
		t.Fatalf("stream 0 (Running) = %v; want Continue", status)
	}

	// Drive the VM into the Failed state (simulating a guest RuntimeError trap
	// under FAIL_RELOAD — the production trap path calls the SAME hook). This
	// records lastLoad = fc.Now() so the 1s backoff window starts now.
	cc.rootVM.NoteReloadRuntimeError()

	// Stream 1: still WITHIN the backoff window (clock not advanced) → the
	// reload machine reports Backoff → FAIL_CLOSED 503 + StopIteration +
	// vm_reload_backoff increments. No reload attempt.
	status1, cb1 := runDecode(cc)
	if status1 != envoyhttp.StopIteration {
		t.Fatalf("stream 1 (within backoff) = %v; want StopIteration (FAIL_CLOSED 503)", status1)
	}
	if cb1.localReplyStatus != gohttp.StatusServiceUnavailable {
		t.Errorf("stream 1 SendLocalReply status = %d; want 503", cb1.localReplyStatus)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_failreload.vm_reload_backoff"); got != 1 {
		t.Errorf("vm_reload_backoff = %d; want 1 (one within-window dispatch)", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_failreload.vm_reload_success"); got != 0 {
		t.Errorf("vm_reload_success = %d; want 0 (no reload attempted yet)", got)
	}

	// Advance the clock PAST the 1s backoff window.
	fc.Advance(1500 * time.Millisecond)

	// Stream 2: past the backoff window → the reload machine attempts a reload;
	// reinstantiate of the (valid) module succeeds → markReloaded (Running) →
	// vm_reload_success + the guest dispatch then runs → Continue.
	status2, _ := runDecode(cc)
	if status2 != envoyhttp.Continue {
		t.Fatalf("stream 2 (post-backoff reload) = %v; want Continue (recovered)", status2)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_failreload.vm_reload_success"); got != 1 {
		t.Errorf("vm_reload_success = %d; want 1 (reload succeeded past the backoff window)", got)
	}

	// Stream 3: VM is Running again → Continue, no further backoff/success bumps.
	status3, _ := runDecode(cc)
	if status3 != envoyhttp.Continue {
		t.Fatalf("stream 3 (recovered) = %v; want Continue", status3)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_failreload.vm_reload_backoff"); got != 1 {
		t.Errorf("vm_reload_backoff = %d; want 1 (no new within-window dispatch)", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_failreload.vm_reload_success"); got != 1 {
		t.Errorf("vm_reload_success = %d; want 1 (no second reload)", got)
	}
}

// -----------------------------------------------------------------------------
// Test 3: FAIL_OPEN unavailable VM → bypass (Continue, no 503).
// -----------------------------------------------------------------------------

// TestDispatch_FailOpenBypass builds a FAIL_OPEN compiledConfig + drives the
// reload state into a non-running state, then asserts the dispatch BYPASSES the
// filter (Continue) WITHOUT a 503. FAIL_OPEN is never reload-eligible, so the
// VM is not reloaded; the disposition is a clean pass-through.
//
// Note: under FAIL_OPEN the reload machine is constructed but ReloadEligible is
// false, so a real guest trap would NOT arm a reload. To exercise the
// applyFailureDisposition(FAIL_OPEN) branch deterministically we mark the
// reload machine Failed directly (NoteReloadRuntimeError) — this is a test-only
// way to reach the "VM unavailable" dispatch state; the production FAIL_OPEN
// trap path reaches applyFailureDisposition via the CallProxyOnRequestHeaders
// error branch instead, which returns the SAME Continue bypass.
func TestDispatch_FailOpenBypass(t *testing.T) {
	// NOT t.Parallel(): withTestClock package-level state (see the reload test).
	reg := stats.NewRegistry()
	fc := clock.NewFakeClock(time.Unix(2000, 0))
	restore := withTestClock(fc)
	t.Cleanup(restore)

	cfg := &wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:          "plugin_failopen",
			RootId:        "test_root",
			FailurePolicy: wasmcommonv3.FailurePolicy_FAIL_OPEN,
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    "test_vm_failopen",
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{
									InlineBytes: buildContinueProxyWasm(),
								},
							},
						},
					},
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
	t.Cleanup(func() { _ = cc.Close() })
	if cc.failurePolicy != internalwasm.FailurePolicyFailOpen {
		t.Fatalf("cc.failurePolicy = %d; want FailOpen", cc.failurePolicy)
	}

	// Drive the VM unavailable (Failed within the backoff window).
	cc.rootVM.NoteReloadRuntimeError()

	// Dispatch: FAIL_OPEN → bypass (Continue), NO 503, NO failure counter bump.
	status, cb := runDecode(cc)
	if status != envoyhttp.Continue {
		t.Fatalf("FAIL_OPEN unavailable dispatch = %v; want Continue (bypass)", status)
	}
	if cb.localReplyCalls != 0 {
		t.Errorf("SendLocalReply calls = %d; want 0 (FAIL_OPEN bypass, no 503)", cb.localReplyCalls)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_failopen.envoy_go.failures"); got != 0 {
		t.Errorf("envoy_go.failures = %d; want 0 (FAIL_OPEN bypass is not a failure)", got)
	}
	// FAIL_OPEN never drives a reload attempt: vm_reload_success must stay 0
	// (ReloadDispatch is called and may report Backoff — vm_reload_backoff may
	// increment — but no re-instantiation fires; the disposition is bypass, not
	// 503, confirming the null/skip-reload path for non-FAIL_RELOAD policies).
	if got := findStatCounterValue(reg, "wasm.plugin_failopen.vm_reload_success"); got != 0 {
		t.Errorf("vm_reload_success = %d; want 0 (FAIL_OPEN bypass triggers no reload attempt)", got)
	}
}
