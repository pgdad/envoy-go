package wasm

// dispatch_test.go — Task 12 end-to-end integration tests for
// DecodeHeaders + EncodeHeaders + OnDestroy + the New factory full body.
// Per PLAN Task 12 test surface:
//
//   1. TestFilter_DecodeHeaders_EndToEnd — construct compiledConfig with
//      a valid wasm module, construct filter, call DecodeHeaders with
//      mock headers, assert HeaderContinue + executions counter
//      incremented + active counter == 1.
//
//   2. TestFilter_EncodeHeaders_EndToEnd — similar for encode-side.
//
//   3. TestFilter_OnDestroy_ReleasesVM — verify vm.Close called +
//      active counter decremented.
//
//   4. TestFilter_SendLocalResponse_TriggersStopIteration — mock module
//      that calls proxy_send_local_response in proxy_on_request_headers;
//      assert filter returns StopIteration + decoderCb.SendLocalReply
//      called.
//
//   5. TestFilter_ConcurrentStreams_NoSharedState — N goroutines each
//      constructing + dispatching a filter; assert no cross-filter leak;
//      race-clean.
//
//   6. TestNew_ReturnsWorkingFactory — `New` with valid config returns a
//      non-nil FilterInstanceFactory; the closure produces fresh filter
//      instances.

import (
	"context"
	"fmt"
	gohttp "net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	wasmcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// -----------------------------------------------------------------------------
// Test helpers: build a *compiledConfig from a wasm module bytes + a stats
// registry; build a *filter ready for DecodeHeaders / EncodeHeaders.
// -----------------------------------------------------------------------------

// testVMIDCounter hands out a unique vm_id per generic test-config build so
// independent tests do NOT collide on a single process-global registry key.
// The DefaultRegistry SHARES *RootVM by (vm_id, vm_config, code); a fixed vm_id
// here would make the suite ordering-dependent (a stale shared VM wired to the
// first test's stats/dispatcher served on a later same-key acquire), which is
// the exact -count=2 contamination this counter prevents. Tests that
// DELIBERATELY exercise vm_id SHARING use newTestCompiledConfigVmId with an
// explicit vm_id instead.
var testVMIDCounter atomic.Uint64

// newTestCompiledConfig wraps the supplied wasm module bytes in a *anypb.Any
// envelope + drives buildCompiledConfig through the full parse pipeline.
// Returns the SHARED *compiledConfig that per-stream filters bind to via
// closure capture. The vm_id is made unique per call (testVMIDCounter) so the
// process-global registry never shares a *RootVM across unrelated tests.
func newTestCompiledConfig(t *testing.T, modBytes []byte, pluginName string, reg *stats.Registry) *compiledConfig {
	t.Helper()
	cfg := &wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:   pluginName,
			RootId: "test_root",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    fmt.Sprintf("test_vm_%d", testVMIDCounter.Add(1)),
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

// testLifecycleCapabilities is the proxy_on_* lifecycle + per-stream dispatch
// capability set that must be ALLOWED for a guest to actually be invoked.
//
// Rationale (measured at phase 82): SandboxConfig is StrictDefaultDeny per
// AMEND-A5, so a *compiledConfig built WITHOUT a CapabilityRestrictionConfig
// (i.e. newTestCompiledConfig) never reaches the guest at all — StreamContext.
// dispatchGuest short-circuits at `!rv.sandbox.IsAllowed(capKey)` and every
// CallProxyOn* returns its benign per-callback default. A test built that way
// can only ever observe the default arm of a post-dispatch action switch, which
// silently makes any non-default arm (e.g. ProxyActionPause) DEAD CODE.
var testLifecycleCapabilities = []string{
	"proxy_on_vm_start",
	"proxy_on_configure",
	"proxy_on_context_create",
	"proxy_on_request_headers",
	"proxy_on_response_headers",
	"proxy_on_request_body",
	"proxy_on_response_body",
	// ⚠️ THE TWO TRAILERS CAPABILITIES WERE MISSING UNTIL PHASE-83 S2, AND
	// THEIR ABSENCE WAS SILENT. StrictDefaultDeny is enforced INSIDE
	// StreamContext.dispatchGuest, so a denied trailers callback returns
	// (ProxyActionContinue, nil) — no error, no envoy_go.failures bump, no
	// log. Any S2 test built on newTestCompiledConfigWithCaps without naming
	// them explicitly would have read GREEN with the Pause arm never executed.
	// Adding them is NOT a global change: measured at this tip, 462 → 462 PASS
	// with a ZERO-LINE per-test result diff, because none of the pre-existing
	// fixture builders in wasm_fixtures_test.go exports either trailers
	// callback.
	"proxy_on_request_trailers",
	"proxy_on_response_trailers",
	"proxy_on_http_call_response",
	"proxy_on_done",
	"proxy_on_log",
	"proxy_on_delete",
}

// newTestCompiledConfigWithCaps is newTestCompiledConfig with a populated
// CapabilityRestrictionConfig, so the guest is ACTUALLY DISPATCHED rather than
// short-circuited by the StrictDefaultDeny sandbox posture.
//
// Prefer this helper over newTestCompiledConfig for ANY test whose assertion
// depends on the guest running — a guest-return value, a hostcall side effect,
// or a non-default arm of the ProxyAction switch. newTestCompiledConfig remains
// correct only for tests that deliberately assert the cap-denied / no-dispatch
// posture, or that never dispatch at all.
//
// testLifecycleCapabilities is always allowed. extraCaps adds hostcall
// capabilities (e.g. "proxy_send_local_response", "proxy_get_header_map_pairs",
// "proxy_add_header_map_value") for guests that invoke them; a hostcall whose
// capability is denied is not even registered as a host import, so a guest that
// imports it fails to instantiate.
//
// The vm_id is unique per call (testVMIDCounter), as in newTestCompiledConfig.
// The caller owns the returned *compiledConfig and must Close it (typically
// `t.Cleanup(func() { _ = cc.Close() })`), matching newTestCompiledConfig.
func newTestCompiledConfigWithCaps(t *testing.T, modBytes []byte, pluginName string, reg *stats.Registry, extraCaps ...string) *compiledConfig {
	t.Helper()
	return newTestCompiledConfigWithCapsAndFactoryCtx(t, modBytes, pluginName,
		envoyhttp.FactoryCtx{Stats: reg}, extraCaps...)
}

// newTestCompiledConfigWithCapsAndFactoryCtx is newTestCompiledConfigWithCaps
// with a caller-supplied FactoryCtx, for tests that need more of the factory
// surface than Stats.
//
// The motivating case (phase 82 Task 10): compiled_config.go wires the
// production wasm.HTTPDispatcher ONLY when BOTH factoryCtx.ClusterManager and
// factoryCtx.HTTPClient are non-nil. With the Stats-only FactoryCtx above,
// proxy_http_call returns InternalFailure, the proxy-wasm-rust-sdk records no
// token for the call, and a subsequent proxy_on_http_call_response TRAPS inside
// the SDK's own token lookup (`expect` on a missing entry) before any of the
// host's response wiring is reached. A test that wants the real guest to
// receive a real http-call response must supply both pointers.
func newTestCompiledConfigWithCapsAndFactoryCtx(t *testing.T, modBytes []byte, pluginName string, factoryCtx envoyhttp.FactoryCtx, extraCaps ...string) *compiledConfig {
	t.Helper()
	allowed := make(map[string]*wasmcommonv3.SanitizationConfig,
		len(testLifecycleCapabilities)+len(extraCaps))
	for _, c := range testLifecycleCapabilities {
		allowed[c] = &wasmcommonv3.SanitizationConfig{}
	}
	for _, c := range extraCaps {
		allowed[c] = &wasmcommonv3.SanitizationConfig{}
	}
	cfg := &wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:   pluginName,
			RootId: "test_root",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    fmt.Sprintf("test_vm_%d", testVMIDCounter.Add(1)),
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
				AllowedCapabilities: allowed,
			},
		},
	}
	tc, err := anypb.New(cfg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	cc, err := buildCompiledConfig(context.Background(), tc, factoryCtx)
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	return cc
}

// findStatCounterValue walks the registry + returns the Counter Load()
// value for the named stat. Returns 0 if the name is absent.
func findStatCounterValue(reg *stats.Registry, name string) uint64 {
	var got uint64
	reg.Walk(func(m stats.Metric) {
		if m.Name() == name {
			if c, ok := m.(*stats.Counter); ok {
				got = c.Load()
			}
		}
	})
	return got
}

// findStatGaugeValue walks the registry + returns the Gauge Load() value
// for the named stat. Returns 0 if the name is absent.
func findStatGaugeValue(reg *stats.Registry, name string) int64 {
	var got int64
	reg.Walk(func(m stats.Metric) {
		if m.Name() == name {
			if g, ok := m.(*stats.Gauge); ok {
				got = g.Load()
			}
		}
	})
	return got
}

// -----------------------------------------------------------------------------
// Test 1: DecodeHeaders end-to-end happy path (Continue).
// -----------------------------------------------------------------------------

// TestFilter_DecodeHeaders_EndToEnd exercises the full DecodeHeaders
// dispatch: initVM (NewVM + RegisterABICallbacks + Run) +
// CallProxyOnContextCreate + CallProxyOnRequestHeaders (returns Continue) +
// no captured local response → http.Continue. Asserts executions++,
// created++, active==1 at end (VM still live; OnDestroy not yet called).
func TestFilter_DecodeHeaders_EndToEnd(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfig(t, buildContinueProxyWasm(), "plugin_decode", reg)
	t.Cleanup(func() { _ = cc.Close() })

	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(fakeDecoderCb{})
	f.SetEncoderCallbacks(fakeEncoderCb{})

	headers := gohttp.Header{}
	headers.Set(":method", "GET")
	headers.Set(":path", "/")

	got := f.DecodeHeaders(headers, true)
	if got != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders = %v; want Continue", got)
	}

	if f.streamCtx == nil {
		t.Errorf("filter.streamCtx = nil after DecodeHeaders; want non-nil per initStreamContext (Task 18 root-VM model)")
	}
	if f.streamContextID == 0 {
		t.Errorf("filter.streamContextID = 0 after DecodeHeaders; want non-zero (allocated by streamContextIDCounter)")
	}
	// Stats:
	if got := findStatCounterValue(reg, "wasm.plugin_decode.executions"); got != 1 {
		t.Errorf("executions = %d; want 1", got)
	}
	if got := findStatCounterValue(reg, "wasm.wazero.created"); got != 1 {
		t.Errorf("created = %d; want 1", got)
	}
	if got := findStatGaugeValue(reg, "wasm.wazero.active"); got != 1 {
		t.Errorf("active = %d; want 1 (pre-OnDestroy)", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_decode.envoy_go.failures"); got != 0 {
		t.Errorf("envoy_go.failures = %d; want 0 (happy path)", got)
	}

	// Cleanup: OnDestroy releases the per-stream StreamContext.
	f.OnDestroy()
	if f.streamCtx != nil {
		t.Errorf("filter.streamCtx != nil after OnDestroy; want nil (Task 18 root-VM model)")
	}
	if got := findStatGaugeValue(reg, "wasm.wazero.active"); got != 0 {
		t.Errorf("active after OnDestroy = %d; want 0", got)
	}
}

// -----------------------------------------------------------------------------
// Test 2: EncodeHeaders end-to-end (reuses VM from DecodeHeaders).
// -----------------------------------------------------------------------------

// TestFilter_EncodeHeaders_EndToEnd exercises EncodeHeaders against the
// per-stream VM constructed at DecodeHeaders entry. Asserts the VM is
// REUSED (no second created++); executions++; Continue returned.
func TestFilter_EncodeHeaders_EndToEnd(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfig(t, buildContinueProxyWasm(), "plugin_encode", reg)
	t.Cleanup(func() { _ = cc.Close() })

	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(fakeDecoderCb{})
	f.SetEncoderCallbacks(fakeEncoderCb{})

	reqHeaders := gohttp.Header{}
	reqHeaders.Set(":method", "GET")
	if got := f.DecodeHeaders(reqHeaders, true); got != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders = %v; want Continue", got)
	}

	respHeaders := gohttp.Header{}
	respHeaders.Set(":status", "200")
	got := f.EncodeHeaders(respHeaders, true)
	if got != envoyhttp.Continue {
		t.Fatalf("EncodeHeaders = %v; want Continue", got)
	}

	// Per AMEND-A2 + parent SPEC §7 line 738: `wasm.<plugin>.executions`
	// is allocated as the per-`proxy_on_request_headers`-invocation counter
	// ONLY — the encode-side dispatch does NOT increment it (the
	// DecodeHeaders body holds the lone Inc site per SPEC §4.3 line 787 +
	// §5.1 hostcall 1 commentary). Task 15+17 follow-up: removed the
	// encode-side Inc to align with the SPEC + fixture-0034 scenario (e)
	// StatsAsserter (one probe ⇒ one expected increment).
	if got := findStatCounterValue(reg, "wasm.plugin_encode.executions"); got != 1 {
		t.Errorf("executions = %d; want 1 (decode-only; per SPEC §7 line 738 the executions counter is per `proxy_on_request_headers` invocation, NOT per dispatch)", got)
	}
	// created stayed at 1 (VM was REUSED on the encode side).
	if got := findStatCounterValue(reg, "wasm.wazero.created"); got != 1 {
		t.Errorf("created = %d; want 1 (VM reused on encode)", got)
	}
	if got := findStatGaugeValue(reg, "wasm.wazero.active"); got != 1 {
		t.Errorf("active = %d; want 1 (pre-OnDestroy)", got)
	}

	f.OnDestroy()
}

// TestFilter_EncodeHeaders_WithoutDecode_ContinuePassthrough verifies the
// encode-side defensive nil-vm short-circuit: when DecodeHeaders never
// fired (e.g. encode-only test path), EncodeHeaders passes through
// without constructing a VM. Matches upstream wasm's encode-side null-vm
// parity.
func TestFilter_EncodeHeaders_WithoutDecode_ContinuePassthrough(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfig(t, buildContinueProxyWasm(), "plugin_enc_only", reg)
	t.Cleanup(func() { _ = cc.Close() })

	f := &filter{cfg: cc}
	f.SetEncoderCallbacks(fakeEncoderCb{})

	respHeaders := gohttp.Header{}
	respHeaders.Set(":status", "200")
	got := f.EncodeHeaders(respHeaders, true)
	if got != envoyhttp.Continue {
		t.Fatalf("EncodeHeaders (no prior Decode) = %v; want Continue", got)
	}
	if f.streamCtx != nil {
		t.Errorf("filter.streamCtx != nil; want nil (no per-stream context construction on encode-only path)")
	}
	// No executions / created bumps — the encode side never reached the
	// CallProxyOnResponseHeaders dispatch because vm was nil.
	if got := findStatCounterValue(reg, "wasm.plugin_enc_only.executions"); got != 0 {
		t.Errorf("executions = %d; want 0 (no VM dispatched)", got)
	}
	if got := findStatCounterValue(reg, "wasm.wazero.created"); got != 0 {
		t.Errorf("created = %d; want 0", got)
	}
}

// -----------------------------------------------------------------------------
// Test 3: OnDestroy releases VM + decrements active gauge.
// -----------------------------------------------------------------------------

// TestFilter_OnDestroy_ReleasesVM verifies the OnDestroy lifecycle:
// CallProxyOnDone + CallProxyOnLog + CallProxyOnDelete + vm.Close +
// active gauge decremented. Idempotent — second OnDestroy is a no-op.
func TestFilter_OnDestroy_ReleasesVM(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfig(t, buildContinueProxyWasm(), "plugin_destroy", reg)
	t.Cleanup(func() { _ = cc.Close() })

	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(fakeDecoderCb{})
	f.SetEncoderCallbacks(fakeEncoderCb{})

	headers := gohttp.Header{}
	headers.Set(":method", "GET")
	if got := f.DecodeHeaders(headers, true); got != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders = %v; want Continue", got)
	}
	if f.streamCtx == nil {
		t.Fatalf("streamCtx == nil after DecodeHeaders; should be non-nil (Task 18 root-VM model)")
	}
	if got := findStatGaugeValue(reg, "wasm.wazero.active"); got != 1 {
		t.Errorf("active = %d; want 1", got)
	}

	f.OnDestroy()
	if f.streamCtx != nil {
		t.Errorf("streamCtx != nil after OnDestroy; want nil (Task 18 root-VM model)")
	}
	if got := findStatGaugeValue(reg, "wasm.wazero.active"); got != 0 {
		t.Errorf("active = %d; want 0 after OnDestroy", got)
	}

	// Idempotent — second OnDestroy must not panic + not double-decrement.
	f.OnDestroy()
	if got := findStatGaugeValue(reg, "wasm.wazero.active"); got != 0 {
		t.Errorf("active = %d; want 0 after second OnDestroy", got)
	}
}

// TestFilter_OnDestroy_NilVM_NoOp verifies the OnDestroy no-op path when
// vm was never constructed (e.g. nil-cfg fall-through).
func TestFilter_OnDestroy_NilVM_NoOp(t *testing.T) {
	t.Parallel()
	f := &filter{}
	// Must not panic.
	f.OnDestroy()
}

// -----------------------------------------------------------------------------
// Test 4: SendLocalResponse → StopIteration + SendLocalReply.
// -----------------------------------------------------------------------------

// recordingDecoderCb captures SendLocalReply invocations for assertion.
type recordingDecoderCb struct {
	fakeDecoderCb
	mu     sync.Mutex
	calls  int
	status int
	body   string
	hdrs   envoyhttp.OrderedHeaders
}

func (r *recordingDecoderCb) SendLocalReply(status int, body string, hdrs envoyhttp.OrderedHeaders) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.status = status
	r.body = body
	r.hdrs = hdrs
}

// TestFilter_SendLocalResponse_TriggersStopIteration uses a wasm fixture
// whose proxy_on_request_headers invokes proxy_send_local_response(403,
// ...) before returning ProxyActionPause. Asserts:
//   - DecodeHeaders returns StopIteration.
//   - decoderCb.SendLocalReply was invoked exactly once with status=403.
//   - f.sentLocalResponse is consumed (cleared) post-dispatch.
func TestFilter_SendLocalResponse_TriggersStopIteration(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	// The SendLocalResponse capability key is gated by the sandbox; we
	// need to ALLOW it. Build a capability_restriction_config that
	// allows proxy_send_local_response.
	cfg := &wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:   "plugin_sendlocal",
			RootId: "test_root",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    fmt.Sprintf("test_vm_%d", testVMIDCounter.Add(1)),
					Runtime: "envoy.wasm.runtime.wazero",
					Code: &corev3.AsyncDataSource{
						Specifier: &corev3.AsyncDataSource_Local{
							Local: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineBytes{
									InlineBytes: buildSendLocalResponseProxyWasm(),
								},
							},
						},
					},
				},
			},
			CapabilityRestrictionConfig: &wasmcommonv3.CapabilityRestrictionConfig{
				AllowedCapabilities: map[string]*wasmcommonv3.SanitizationConfig{
					// Lifecycle capabilities so Run + dispatch reach the guest.
					"proxy_on_vm_start":         {},
					"proxy_on_configure":        {},
					"proxy_on_context_create":   {},
					"proxy_on_request_headers":  {},
					"proxy_on_response_headers": {},
					"proxy_on_done":             {},
					"proxy_on_log":              {},
					"proxy_on_delete":           {},
					// Hostcall the guest invokes.
					"proxy_send_local_response": {},
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

	rec := &recordingDecoderCb{}
	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(rec)
	f.SetEncoderCallbacks(fakeEncoderCb{})

	headers := gohttp.Header{}
	headers.Set(":method", "GET")
	got := f.DecodeHeaders(headers, true)
	if got != envoyhttp.StopIteration {
		t.Fatalf("DecodeHeaders = %v; want StopIteration", got)
	}
	rec.mu.Lock()
	calls := rec.calls
	status := rec.status
	rec.mu.Unlock()
	if calls != 1 {
		t.Errorf("SendLocalReply call count = %d; want 1", calls)
	}
	if status != 403 {
		t.Errorf("SendLocalReply status = %d; want 403", status)
	}
	if f.sentLocalResponse != nil {
		t.Errorf("f.sentLocalResponse = %v; want nil (consumed)", f.sentLocalResponse)
	}

	f.OnDestroy()
}

// -----------------------------------------------------------------------------
// Test 5: Concurrent streams — no shared state, race-clean.
// -----------------------------------------------------------------------------

// TestFilter_ConcurrentStreams_NoSharedState spins up N goroutines each
// constructing a fresh *filter bound to the SAME *compiledConfig +
// invoking DecodeHeaders + EncodeHeaders + OnDestroy. Verifies:
//   - Each filter gets a unique streamContextID.
//   - Group-B created counter == N (one VM per filter).
//   - Group-B active gauge == 0 at end (all OnDestroyed).
//
// Run with -race to surface any cross-filter state leak.
func TestFilter_ConcurrentStreams_NoSharedState(t *testing.T) {
	t.Parallel()
	const N = 8
	reg := stats.NewRegistry()
	cc := newTestCompiledConfig(t, buildContinueProxyWasm(), "plugin_concurrent", reg)
	t.Cleanup(func() { _ = cc.Close() })

	var wg sync.WaitGroup
	wg.Add(N)
	streamIDs := make([]uint32, N)

	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			f := &filter{cfg: cc}
			f.SetDecoderCallbacks(fakeDecoderCb{})
			f.SetEncoderCallbacks(fakeEncoderCb{})
			req := gohttp.Header{}
			req.Set(":method", "GET")
			if got := f.DecodeHeaders(req, true); got != envoyhttp.Continue {
				t.Errorf("goroutine %d: DecodeHeaders = %v; want Continue", i, got)
			}
			atomic.StoreUint32(&streamIDs[i], f.streamContextID)
			resp := gohttp.Header{}
			resp.Set(":status", "200")
			if got := f.EncodeHeaders(resp, true); got != envoyhttp.Continue {
				t.Errorf("goroutine %d: EncodeHeaders = %v; want Continue", i, got)
			}
			f.OnDestroy()
		}()
	}
	wg.Wait()

	// All streamContextIDs must be unique (no two filters share an ID).
	seen := make(map[uint32]bool, N)
	for i, id := range streamIDs {
		if id == 0 {
			t.Errorf("goroutine %d: streamContextID = 0 (uninitialized)", i)
		}
		if seen[id] {
			t.Errorf("streamContextID %d allocated to multiple goroutines (cross-stream leak)", id)
		}
		seen[id] = true
	}

	if got := findStatCounterValue(reg, "wasm.wazero.created"); got != N {
		t.Errorf("created = %d; want %d", got, N)
	}
	if got := findStatGaugeValue(reg, "wasm.wazero.active"); got != 0 {
		t.Errorf("active = %d; want 0 (all OnDestroyed)", got)
	}
	// Per AMEND-A2 + parent SPEC §7 line 738 + §4.3 line 787: executions
	// counts per-`proxy_on_request_headers`-invocation ONLY (decode-side
	// only). N parallel streams ⇒ N decode dispatches ⇒ N increments. The
	// encode-side dispatch does NOT increment the same counter (the SPEC
	// reserves the executions name to the decode-side invocation only;
	// updated post Task 15+17 follow-up).
	if got := findStatCounterValue(reg, "wasm.plugin_concurrent.executions"); got != N {
		t.Errorf("executions = %d; want %d (one Inc per decode-side dispatch; encode-side does NOT increment per SPEC §7 line 738)", got, N)
	}
}

// -----------------------------------------------------------------------------
// Test 6: New() returns a working FilterInstanceFactory.
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// Test 7: PAUSE-w/o-local-response → StopIteration (phase-82 S1).
// -----------------------------------------------------------------------------

// TestFilter_DecodeHeaders_Pause_StopsIteration exercises the PAUSE arm: the
// guest returns ProxyActionPause without invoking proxy_send_local_response and
// the dispatcher STOPS ITERATION, parking the stream until the guest calls
// proxy_continue_stream (or the S9 watchdog fires).
//
// FLIPPED AT PHASE 82 (this is the row's failing-first anchor, not a new test).
// Until this row the arm logged "stream-control deferred (parent §1
// architectural primitive 6)" and returned Continue — the guest's pause was
// silently discarded. That blanket claim was FALSE: four of the six
// abi.ProxyActionPause arms in this package (body.go x2, trailers.go x2) had
// honored Pause all along; only the two HEADERS arms were deferred. The
// assertion below was `want Continue` and now reads `want StopIteration`.
//
// Asserts: StopIteration returned; the paused flag is set (this filter owes
// the chain exactly one resume); no SendLocalReply call; no envoy_go.failures
// bump (PAUSE is not a failure).
//
// CAPABILITY-ENABLED (phase 82 Task 8). This test previously used
// newTestCompiledConfig, whose StrictDefaultDeny sandbox meant the guest was
// NEVER DISPATCHED: CallProxyOnRequestHeaders short-circuited at the capability
// gate and returned the zero-value ProxyActionContinue, so the switch below
// always took the `default:` arm and the ProxyActionPause arm this test claims
// to exercise was DEAD CODE. Measured with a discriminating cross-product: a
// panic injected into the ProxyActionPause arm did NOT fire (test still PASSed)
// while a panic injected into the `default:` arm DID. It now uses
// newTestCompiledConfigWithCaps so the guest actually runs and returns PAUSE.
//
// The executions assertion below is the liveness barrier that keeps this test
// from silently regressing to the vacuous form: executions is incremented on
// the decode-side dispatch path, so it is 1 only if dispatch actually happened.
func TestFilter_DecodeHeaders_Pause_StopsIteration(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfigWithCaps(t, buildPauseProxyWasm(), "plugin_pause", reg)
	t.Cleanup(func() { _ = cc.Close() })

	rec := &recordingDecoderCb{}
	f := &filter{cfg: cc}
	// Keep the S9 watchdog out of this test's way — the park bound is asserted
	// separately in pause_test.go.
	f.pauseWatchdog = time.Hour
	f.SetDecoderCallbacks(rec)
	f.SetEncoderCallbacks(fakeEncoderCb{})
	t.Cleanup(f.OnDestroy)

	headers := gohttp.Header{}
	headers.Set(":method", "GET")
	got := f.DecodeHeaders(headers, true)
	if got != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders = %v; want StopIteration (phase-82 S1 honors ProxyActionPause on request headers)", got)
	}
	if !f.decodePaused.Load() {
		t.Errorf("decodePaused = false after the PAUSE arm; want true (the filter owes the chain exactly one resume)")
	}
	rec.mu.Lock()
	calls := rec.calls
	rec.mu.Unlock()
	if calls != 0 {
		t.Errorf("SendLocalReply calls = %d; want 0 (no captured local-response on PAUSE arm)", calls)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_pause.envoy_go.failures"); got != 0 {
		t.Errorf("envoy_go.failures = %d; want 0 (PAUSE is not a failure)", got)
	}
	// LIVENESS BARRIER: proves the guest was actually dispatched (and hence
	// that the guest's ProxyActionPause return reached the action switch).
	// Under the pre-phase-82 StrictDefaultDeny fixture this was 0 while every
	// other assertion above still passed — see the doc comment.
	if got := findStatCounterValue(reg, "wasm.plugin_pause.executions"); got != 1 {
		t.Errorf("executions = %d; want 1 (the guest MUST be dispatched — a 0 here means the capability gate short-circuited and the PAUSE arm was never reached)", got)
	}
	f.OnDestroy()
}

// -----------------------------------------------------------------------------
// Test 8: Missing proxy_on_request_headers export → Continue (no-op success).
// -----------------------------------------------------------------------------

// TestFilter_DecodeHeaders_MissingExports_ContinueNoOp exercises the
// upstream-parity "nullptr the function pointer" discipline: when the
// guest does not export proxy_on_request_headers, the VM helper returns
// (Continue, nil) and the dispatch passes through with no envoy_go.failures
// bump. Same arm for the encode side via the symmetric VM contract.
func TestFilter_DecodeHeaders_MissingExports_ContinueNoOp(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfig(t, buildMinimalProxyWasm(), "plugin_minimal", reg)
	t.Cleanup(func() { _ = cc.Close() })

	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(fakeDecoderCb{})
	f.SetEncoderCallbacks(fakeEncoderCb{})

	headers := gohttp.Header{}
	got := f.DecodeHeaders(headers, true)
	if got != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders (missing export) = %v; want Continue", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_minimal.envoy_go.failures"); got != 0 {
		t.Errorf("envoy_go.failures = %d; want 0 (missing export is not a failure)", got)
	}
	// executions IS incremented per AMEND-A2 — the dispatch RAN, just to
	// a no-op guest. Mirrors upstream's counter discipline.
	if got := findStatCounterValue(reg, "wasm.plugin_minimal.executions"); got != 1 {
		t.Errorf("executions = %d; want 1 (dispatch ran)", got)
	}

	f.OnDestroy()
}

// TestNew_ReturnsWorkingFactory verifies the New factory full body lands
// a non-nil FilterInstanceFactory closure + the closure produces fresh
// *filter instances bound to the SHARED *compiledConfig. Replaces the
// Task 8 sentinel-error assertion (removed at Task 12).
func TestNew_ReturnsWorkingFactory(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cfg := &wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:   "plugin_new",
			RootId: "test_root",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    fmt.Sprintf("test_vm_%d", testVMIDCounter.Add(1)),
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
	factory, err := New(tc, envoyhttp.FactoryCtx{Stats: reg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if factory == nil {
		t.Fatal("New returned nil factory; want non-nil")
	}
	// The closure produces fresh HTTPFilter instances on each call.
	hf1 := factory()
	hf2 := factory()

	// New holds the shared *compiledConfig only via the factory closure, so
	// recover it from a produced filter instance and Close it (refcount-
	// balanced DefaultRegistry.Release) at cleanup so the registry-acquired
	// *RootVM does not leak past this test. Deliberately NOT
	// DefaultRegistry.ResetForTest(): this test runs in the t.Parallel phase
	// and ResetForTest force-closes EVERY registry entry regardless of
	// refcount — including *RootVMs held by concurrently-running parallel
	// tests, which then fail with "NewStreamContext on closed RootVM"
	// (the 2-core-CI flake in TestFilter_ConcurrentStreams_NoSharedState +
	// TestFilter_RootVM_SharedAcrossStreams_NoCrossStreamLeak).
	if f, ok := hf1.Decoder.(*filter); ok && f.cfg != nil {
		t.Cleanup(func() { _ = f.cfg.Close() })
	} else {
		// Fail loudly rather than silently leaking the registry RootVM: if a
		// refactor changes the Decoder's concrete type or leaves cfg nil, the
		// cleanup contract above is broken and must be re-wired, not skipped.
		t.Fatalf("expected hf1.Decoder to be *filter with non-nil cfg; cleanup contract broken")
	}
	if hf1.Name != filterName {
		t.Errorf("hf1.Name = %q; want %q", hf1.Name, filterName)
	}
	if hf2.Name != filterName {
		t.Errorf("hf2.Name = %q; want %q", hf2.Name, filterName)
	}
	if hf1.Decoder == nil || hf1.Encoder == nil {
		t.Errorf("hf1 missing Decoder/Encoder: %+v", hf1)
	}
	if hf1.Decoder == hf2.Decoder {
		t.Errorf("hf1.Decoder == hf2.Decoder; want distinct per-stream instances")
	}
	// Both filters share the SAME *compiledConfig closure capture.
	f1 := hf1.Decoder.(*filter)
	f2 := hf2.Decoder.(*filter)
	if f1.cfg != f2.cfg {
		t.Errorf("f1.cfg != f2.cfg; want shared *compiledConfig across filters")
	}
}

// =============================================================================
// 25.2 IMPL Task 18 — root-VM-model integration tests.
//
// These tests exercise the per-stream context lifecycle under the SHARED
// per-compiledConfig *RootVM model (post-Task-1 + post-Task-18 closure of
// D-P-PLAN-6). The 25.1 per-stream wasm.NewVM construction is GONE; the
// per-stream cost is now microseconds (just NewStreamContext bookkeeping
// + a proxy_on_context_create dispatch on the shared module instance).
//
// Test surface:
//
//   - TestFilter_RootVM_SharedAcrossStreams_NoCrossStreamLeak: N=100 streams
//     on one *compiledConfig share the same *RootVM; per-stream contexts
//     have distinct streamContextID; cleanup leaves active=0 + the
//     rootABICallbacks multiplexer map empty.
//
//   - TestFilter_RootVM_LifecycleViaStreamContext: end-to-end DecodeHeaders
//     → EncodeHeaders → DecodeData (under cap) → EncodeData (under cap) →
//     DecodeTrailers → EncodeTrailers → OnDestroy on a single stream; the
//     streamCtx is the same instance throughout + closes cleanly.
//
//   - TestFilter_RootVM_BodyCapEnforcement_DecodeSide: body chunk over cap
//     fires the 413 SendLocalReply + body_buffer_cap_exceeded counter +
//     envoy_go.failures co-increment; subsequent chunks short-circuit
//     without re-incrementing.
//
//   - TestFilter_RootVM_OnDestroyUnregistersFromMultiplexer: after OnDestroy
//     the rootCB.lookup(streamContextID) returns false.
// =============================================================================

// TestFilter_RootVM_SharedAcrossStreams_NoCrossStreamLeak spins up N=100
// per-stream filters bound to the SAME *compiledConfig (and thus the SAME
// *RootVM). Verifies:
//
//   - Each stream allocates a unique streamContextID via
//     rootVM.NewStreamContext (no two streams share an ID).
//   - The Group-B created counter == N (one per-stream-context construction
//     per stream).
//   - The Group-B active gauge == 0 after all streams have OnDestroyed
//     (cleanup is correct + the multiplexer entries are released).
//   - The rootABICallbacks multiplexer map is empty post-cleanup (no
//     leaked per-stream entries).
//
// Run with -race to surface any cross-stream state leak in the
// rootABICallbacks multiplexer or the per-stream *abiCallbacks.
func TestFilter_RootVM_SharedAcrossStreams_NoCrossStreamLeak(t *testing.T) {
	t.Parallel()
	const N = 100
	reg := stats.NewRegistry()
	// Unique plugin name per invocation so the process-global arm-26 name claim
	// does not collide under -count>1 (the vm_id is already counter-unique via
	// newTestCompiledConfig; the plugin name is the remaining shared-process key).
	pluginName := fmt.Sprintf("plugin_rootvm_concurrent_%d", testVMIDCounter.Add(1))
	cc := newTestCompiledConfig(t, buildContinueProxyWasm(), pluginName, reg)
	t.Cleanup(func() { _ = cc.Close() })

	var wg sync.WaitGroup
	wg.Add(N)
	streamIDs := make([]uint32, N)

	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			f := &filter{cfg: cc}
			f.SetDecoderCallbacks(fakeDecoderCb{})
			f.SetEncoderCallbacks(fakeEncoderCb{})
			req := gohttp.Header{}
			req.Set(":method", "GET")
			if got := f.DecodeHeaders(req, true); got != envoyhttp.Continue {
				t.Errorf("goroutine %d: DecodeHeaders = %v; want Continue", i, got)
			}
			atomic.StoreUint32(&streamIDs[i], f.streamContextID)
			resp := gohttp.Header{}
			resp.Set(":status", "200")
			if got := f.EncodeHeaders(resp, true); got != envoyhttp.Continue {
				t.Errorf("goroutine %d: EncodeHeaders = %v; want Continue", i, got)
			}
			f.OnDestroy()
		}()
	}
	wg.Wait()

	// All streamContextIDs must be unique (no two streams share an ID).
	seen := make(map[uint32]bool, N)
	for i, id := range streamIDs {
		if id == 0 {
			t.Errorf("goroutine %d: streamContextID = 0 (uninitialized)", i)
		}
		if seen[id] {
			t.Errorf("streamContextID %d allocated to multiple goroutines (cross-stream leak)", id)
		}
		seen[id] = true
	}

	// Group-B counters: N created, 0 active at end (all OnDestroyed).
	if got := findStatCounterValue(reg, "wasm.wazero.created"); got != N {
		t.Errorf("created = %d; want %d", got, N)
	}
	if got := findStatGaugeValue(reg, "wasm.wazero.active"); got != 0 {
		t.Errorf("active = %d; want 0 (all OnDestroyed; no leaked stream contexts)", got)
	}
	// executions = N (one Inc per decode-side dispatch).
	if got := findStatCounterValue(reg, "wasm."+pluginName+".executions"); got != N {
		t.Errorf("executions = %d; want %d", got, N)
	}

	// rootABICallbacks multiplexer map should be empty post-cleanup —
	// every OnDestroy unregisters its per-stream entry.
	cc.rootCB.mu.RLock()
	mapSize := len(cc.rootCB.perCB)
	cc.rootCB.mu.RUnlock()
	if mapSize != 0 {
		t.Errorf("rootABICallbacks map size post-cleanup = %d; want 0 (leaked per-stream entries)", mapSize)
	}
}

// TestFilter_RootVM_LifecycleViaStreamContext exercises the full per-
// stream lifecycle under the root-VM model: DecodeHeaders → EncodeHeaders
// → DecodeData → EncodeData → DecodeTrailers → EncodeTrailers → OnDestroy.
// Asserts the streamCtx is the same instance throughout (no per-callback
// re-construction); OnDestroy releases cleanly + the multiplexer entry
// is unregistered.
func TestFilter_RootVM_LifecycleViaStreamContext(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfig(t, buildContinueProxyWasm(), "plugin_rootvm_lifecycle", reg)
	t.Cleanup(func() { _ = cc.Close() })

	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(fakeDecoderCb{})
	f.SetEncoderCallbacks(fakeEncoderCb{})

	// 1. DecodeHeaders constructs the StreamContext.
	reqHdr := gohttp.Header{}
	reqHdr.Set(":method", "POST")
	if got := f.DecodeHeaders(reqHdr, false); got != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders = %v; want Continue", got)
	}
	sc1 := f.streamCtx
	if sc1 == nil {
		t.Fatal("streamCtx nil after DecodeHeaders")
	}

	// 2. DecodeData under cap — NO-op (guest doesn't export proxy_on_request_body).
	if got := f.DecodeData([]byte("hello"), false); got != envoyhttp.DataContinue {
		t.Errorf("DecodeData = %v; want DataContinue", got)
	}
	if f.streamCtx != sc1 {
		t.Errorf("streamCtx changed after DecodeData; want same instance")
	}

	// 3. DecodeTrailers — NO-op (guest doesn't export proxy_on_request_trailers).
	trl := gohttp.Header{}
	trl.Set("Grpc-Status", "0")
	if got := f.DecodeTrailers(trl); got != envoyhttp.TrailersContinue {
		t.Errorf("DecodeTrailers = %v; want TrailersContinue", got)
	}

	// 4. EncodeHeaders — REUSES the same streamCtx (NO re-construction).
	respHdr := gohttp.Header{}
	respHdr.Set(":status", "200")
	if got := f.EncodeHeaders(respHdr, false); got != envoyhttp.Continue {
		t.Errorf("EncodeHeaders = %v; want Continue", got)
	}
	if f.streamCtx != sc1 {
		t.Errorf("streamCtx changed after EncodeHeaders; want same instance")
	}

	// 5. EncodeData — NO-op.
	if got := f.EncodeData([]byte("world"), false); got != envoyhttp.DataContinue {
		t.Errorf("EncodeData = %v; want DataContinue", got)
	}

	// 6. EncodeTrailers — NO-op.
	if got := f.EncodeTrailers(trl); got != envoyhttp.TrailersContinue {
		t.Errorf("EncodeTrailers = %v; want TrailersContinue", got)
	}

	// 7. OnDestroy — releases the StreamContext + unregisters from
	// multiplexer.
	streamID := f.streamContextID
	f.OnDestroy()
	if f.streamCtx != nil {
		t.Errorf("streamCtx != nil after OnDestroy")
	}

	// Multiplexer entry unregistered.
	if _, ok := cc.rootCB.lookup(streamID); ok {
		t.Errorf("rootCB.lookup(streamID=%d) = true after OnDestroy; want false (entry should be unregistered)", streamID)
	}

	// active gauge decremented to 0.
	if got := findStatGaugeValue(reg, "wasm.wazero.active"); got != 0 {
		t.Errorf("active = %d post-OnDestroy; want 0", got)
	}
}

// TestFilter_RootVM_BodyCapEnforcement_DecodeSide exercises the body
// cap-enforcement path under the root-VM model: a body chunk over the
// 16-byte cap fires SendLocalReply(413) + body_buffer_cap_exceeded +
// envoy_go.failures + DataStopIterationNoBuffer. Subsequent chunks (phase-83
// S8: the sticky short-circuit returns DataTerminateStream)
// short-circuit via the sticky flag.
func TestFilter_RootVM_BodyCapEnforcement_DecodeSide(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	// Build a real compiledConfig with a 16-byte cap override via the
	// envoy_go_strict Struct.
	cc := newTestCompiledConfig(t, buildContinueProxyWasm(), "plugin_rootvm_bodycap", reg)
	t.Cleanup(func() { _ = cc.Close() })
	cc.bodyBufferCapBytes = 16 // override for test

	rec := &recordingDecoderCb{}
	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(rec)
	f.SetEncoderCallbacks(fakeEncoderCb{})

	// DecodeHeaders constructs the streamCtx.
	hdr := gohttp.Header{}
	hdr.Set(":method", "POST")
	if got := f.DecodeHeaders(hdr, false); got != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders = %v; want Continue", got)
	}

	// 32-byte chunk exceeds the 16-byte cap → 413 + counters + StopIter.
	oversize := make([]byte, 32)
	got := f.DecodeData(oversize, false)
	if got != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("DecodeData (over-cap) = %v; want DataStopIterationNoBuffer", got)
	}
	rec.mu.Lock()
	calls := rec.calls
	status := rec.status
	rec.mu.Unlock()
	if calls != 1 {
		t.Errorf("SendLocalReply calls = %d; want 1", calls)
	}
	if status != 413 {
		t.Errorf("SendLocalReply status = %d; want 413", status)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_rootvm_bodycap.body_buffer_cap_exceeded"); got != 1 {
		t.Errorf("body_buffer_cap_exceeded = %d; want 1", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_rootvm_bodycap.envoy_go.failures"); got != 1 {
		t.Errorf("envoy_go.failures co-increment = %d; want 1", got)
	}

	// Subsequent chunk short-circuits via sticky flag — NO re-bump of
	// counters or re-invoke of SendLocalReply.
	if got := f.DecodeData(oversize, true); got != envoyhttp.DataTerminateStream {
		t.Errorf("DecodeData (post-sticky) = %v; want DataTerminateStream (phase-83 S8)", got)
	}
	rec.mu.Lock()
	calls = rec.calls
	rec.mu.Unlock()
	if calls != 1 {
		t.Errorf("SendLocalReply calls post-sticky = %d; want 1 (sticky flag prevents re-invoke)", calls)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_rootvm_bodycap.body_buffer_cap_exceeded"); got != 1 {
		t.Errorf("body_buffer_cap_exceeded post-sticky = %d; want 1 (sticky flag prevents re-bump)", got)
	}

	f.OnDestroy()
}

// TestFilter_RootVM_HostcallRoutesToOriginatingStream exercises the
// rootABICallbacks multiplexer's per-stream dispatch isolation. Two
// concurrent streams construct distinct *abiCallbacks; a hostcall fired
// with stream A's context ID returns stream A's per-stream state (e.g.,
// request headers); a hostcall with stream B's context ID returns stream
// B's state. NO cross-stream bleeding.
//
// This is the load-bearing test for the per-RootVM multiplexer design:
// without the streamCtxID-keyed lookup, hostcalls would always read the
// LAST registered *abiCallbacks (last-call-wins on rv.cb), producing
// silent data corruption across streams.
func TestFilter_RootVM_HostcallRoutesToOriginatingStream(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfig(t, buildContinueProxyWasm(), "plugin_rootvm_isolation", reg)
	t.Cleanup(func() { _ = cc.Close() })

	// Stream A.
	fA := &filter{cfg: cc}
	fA.SetDecoderCallbacks(fakeDecoderCb{})
	fA.SetEncoderCallbacks(fakeEncoderCb{})
	hdrA := gohttp.Header{}
	hdrA.Set(":method", "GET")
	hdrA.Set("X-Stream-Tag", "alpha")
	if got := fA.DecodeHeaders(hdrA, true); got != envoyhttp.Continue {
		t.Fatalf("stream A DecodeHeaders = %v; want Continue", got)
	}

	// Stream B.
	fB := &filter{cfg: cc}
	fB.SetDecoderCallbacks(fakeDecoderCb{})
	fB.SetEncoderCallbacks(fakeEncoderCb{})
	hdrB := gohttp.Header{}
	hdrB.Set(":method", "POST")
	hdrB.Set("X-Stream-Tag", "beta")
	if got := fB.DecodeHeaders(hdrB, true); got != envoyhttp.Continue {
		t.Fatalf("stream B DecodeHeaders = %v; want Continue", got)
	}

	// Verify the multiplexer routes stream A's context ID to fA's cb.
	cbA, okA := cc.rootCB.lookup(fA.streamContextID)
	if !okA {
		t.Fatalf("multiplexer lookup for stream A id=%d failed", fA.streamContextID)
	}
	if cbA.filter != fA {
		t.Errorf("multiplexer for stream A returned cb bound to wrong filter (cross-stream leak)")
	}
	// Probe per-stream state via the per-stream *abiCallbacks.
	if v, _ := cbA.GetHeaderMapValue(context.Background(), fA.streamContextID, abi.WasmHeaderMapTypeHttpRequestHeaders, "X-Stream-Tag"); v != "alpha" {
		t.Errorf("stream A GetHeaderMapValue(X-Stream-Tag) = %q; want \"alpha\"", v)
	}

	// Verify stream B routes to fB's cb.
	cbB, okB := cc.rootCB.lookup(fB.streamContextID)
	if !okB {
		t.Fatalf("multiplexer lookup for stream B id=%d failed", fB.streamContextID)
	}
	if cbB.filter != fB {
		t.Errorf("multiplexer for stream B returned cb bound to wrong filter (cross-stream leak)")
	}
	if v, _ := cbB.GetHeaderMapValue(context.Background(), fB.streamContextID, abi.WasmHeaderMapTypeHttpRequestHeaders, "X-Stream-Tag"); v != "beta" {
		t.Errorf("stream B GetHeaderMapValue(X-Stream-Tag) = %q; want \"beta\"", v)
	}

	// Stream A ID != Stream B ID (independent allocations).
	if fA.streamContextID == fB.streamContextID {
		t.Errorf("streamContextID collision: fA=%d, fB=%d", fA.streamContextID, fB.streamContextID)
	}

	fA.OnDestroy()
	fB.OnDestroy()

	// Post-cleanup: both entries unregistered.
	if _, ok := cc.rootCB.lookup(fA.streamContextID); ok {
		t.Errorf("multiplexer entry for stream A leaked post-OnDestroy")
	}
	if _, ok := cc.rootCB.lookup(fB.streamContextID); ok {
		t.Errorf("multiplexer entry for stream B leaked post-OnDestroy")
	}
}
