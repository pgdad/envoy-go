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
	gohttp "net/http"
	"sync"
	"sync/atomic"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	wasmcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Test helpers: build a *compiledConfig from a wasm module bytes + a stats
// registry; build a *filter ready for DecodeHeaders / EncodeHeaders.
// -----------------------------------------------------------------------------

// newTestCompiledConfig wraps the supplied wasm module bytes in a *anypb.Any
// envelope + drives buildCompiledConfig through the full parse pipeline.
// Returns the SHARED *compiledConfig that per-stream filters bind to via
// closure capture.
func newTestCompiledConfig(t *testing.T, modBytes []byte, pluginName string, reg *stats.Registry) *compiledConfig {
	t.Helper()
	cfg := &wasmv3.Wasm{
		Config: &wasmcommonv3.PluginConfig{
			Name:   pluginName,
			RootId: "test_root",
			Vm: &wasmcommonv3.PluginConfig_VmConfig{
				VmConfig: &wasmcommonv3.VmConfig{
					VmId:    "test_vm",
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
	t.Cleanup(func() { _ = cc.compileCache.Close() })

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

	if f.vm == nil {
		t.Errorf("filter.vm = nil after DecodeHeaders; want non-nil per initVM")
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

	// Cleanup: OnDestroy releases the VM.
	f.OnDestroy()
	if f.vm != nil {
		t.Errorf("filter.vm != nil after OnDestroy; want nil")
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
	t.Cleanup(func() { _ = cc.compileCache.Close() })

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
	t.Cleanup(func() { _ = cc.compileCache.Close() })

	f := &filter{cfg: cc}
	f.SetEncoderCallbacks(fakeEncoderCb{})

	respHeaders := gohttp.Header{}
	respHeaders.Set(":status", "200")
	got := f.EncodeHeaders(respHeaders, true)
	if got != envoyhttp.Continue {
		t.Fatalf("EncodeHeaders (no prior Decode) = %v; want Continue", got)
	}
	if f.vm != nil {
		t.Errorf("filter.vm != nil; want nil (no VM construction on encode-only path)")
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
	t.Cleanup(func() { _ = cc.compileCache.Close() })

	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(fakeDecoderCb{})
	f.SetEncoderCallbacks(fakeEncoderCb{})

	headers := gohttp.Header{}
	headers.Set(":method", "GET")
	if got := f.DecodeHeaders(headers, true); got != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders = %v; want Continue", got)
	}
	if f.vm == nil {
		t.Fatalf("vm == nil after DecodeHeaders; should be non-nil")
	}
	if got := findStatGaugeValue(reg, "wasm.wazero.active"); got != 1 {
		t.Errorf("active = %d; want 1", got)
	}

	f.OnDestroy()
	if f.vm != nil {
		t.Errorf("vm != nil after OnDestroy; want nil")
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
					VmId:    "test_vm",
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
	t.Cleanup(func() { _ = cc.compileCache.Close() })

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
	t.Cleanup(func() { _ = cc.compileCache.Close() })

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
// Test 7: PAUSE-w/o-local-response → log + Continue (stream-control deferred).
// -----------------------------------------------------------------------------

// TestFilter_DecodeHeaders_Pause_LogAndContinue exercises the PAUSE arm
// per 25.1 SPEC §4.3 + parent §1 architectural primitive 6: the guest
// returns ProxyActionPause without invoking proxy_send_local_response;
// the dispatcher logs + Continues (stream-control deferred to 25.2).
// Asserts: Continue returned; no SendLocalReply call; no envoy_go.failures
// bump (PAUSE is not a failure, just a deferred surface).
func TestFilter_DecodeHeaders_Pause_LogAndContinue(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfig(t, buildPauseProxyWasm(), "plugin_pause", reg)
	t.Cleanup(func() { _ = cc.compileCache.Close() })

	rec := &recordingDecoderCb{}
	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(rec)
	f.SetEncoderCallbacks(fakeEncoderCb{})

	headers := gohttp.Header{}
	headers.Set(":method", "GET")
	got := f.DecodeHeaders(headers, true)
	if got != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders = %v; want Continue (PAUSE w/o local-response → log + Continue per §1 primitive 6)", got)
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
	t.Cleanup(func() { _ = cc.compileCache.Close() })

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
					VmId:    "test_vm",
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
