package wasm

// body_test.go — Task 16 unit tests for body.go DecodeData + EncodeData per
// 25.2 SPEC §4.3 + Q1 + Q2.
//
// Test surface (per PLAN Task 16 + acceptance criteria):
//
//   1. TestBody_DecodeData_AccumulatesChunks — multiple DecodeData calls
//      grow f.decodeBody monotonically; bodySize passed to the downstream
//      dispatch matches the accumulated total.
//
//   2. TestBody_DecodeData_CapExceeded_SendsLocalReply_413 — cap-exceeded
//      fires sticky flag + body_buffer_cap_exceeded counter +
//      envoy_go.failures counter + decoderCb.SendLocalReply(413) +
//      DataStopIterationNoBuffer.
//
//   3. TestBody_DecodeData_StickyCapExceeded_NoReDispatch — after first
//      cap-exceeded event, subsequent DecodeData calls return DataStop
//      IterationNoBuffer WITHOUT re-bumping counters OR re-invoking
//      SendLocalReply.
//
//   4. TestBody_DecodeData_NoOpWhenGuestNotExported — streamCtx is nil OR
//      HasGlobalFunc returns false → DataContinue without dispatch attempt.
//
//   5. TestBody_DecodeData_CapEnforcedEvenWithoutGuestOptIn — cap fires
//      whether or not the guest exported proxy_on_request_body (host
//      policy, not guest policy).
//
//   6. TestBody_EncodeData_MirrorsDecodeSide — encode-side cap-exceeded
//      bumps counters + StopAllIteration but does NOT invoke SendLocalReply
//      (EncoderFilterCallbacks does not expose it).
//
//   7. TestBody_EncodeData_StickyCapExceeded — encode-side sticky flag.

import (
	"testing"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

// recordingDecoderCbCap captures SendLocalReply invocations for assertion in
// body_test.go cap-exceeded tests. Embeds fakeDecoderCb (from
// abi_callbacks_test.go) for the no-op satisfiers; overrides SendLocalReply
// to record the call.
type recordingDecoderCbCap struct {
	fakeDecoderCb
	calls       int
	lastStatus  int
	lastBody    string
	lastHeaders envoyhttp.OrderedHeaders
}

func (r *recordingDecoderCbCap) SendLocalReply(status int, body string, hdrs envoyhttp.OrderedHeaders) {
	r.calls++
	r.lastStatus = status
	r.lastBody = body
	r.lastHeaders = hdrs
}

// newBodyTestCompiledConfig constructs a minimal *compiledConfig suitable
// for body.go tests. The cap is exposed as a parameter so individual tests
// can configure tiny caps for cap-exceeded coverage.
//
// NOTE: This helper does NOT construct a real *RootVM (the package compile-
// blocks until Task 18 closes it; once unblocked, the helper can be upgraded
// to build a full *RootVM for streamCtx-driven scenarios). At Task 16 the
// helper exercises only the cap-enforcement + NO-op-when-streamCtx-nil
// branches.
func newBodyTestCompiledConfig(t *testing.T, capBytes uint32, pluginName string, reg *stats.Registry) *compiledConfig {
	t.Helper()
	return &compiledConfig{
		pluginName:         pluginName,
		bodyBufferCapBytes: capBytes,
		stats:              newFilterStats(reg, pluginName),
	}
}

// -----------------------------------------------------------------------------
// 1. Accumulation under cap.
// -----------------------------------------------------------------------------

// TestBody_DecodeData_AccumulatesChunks asserts that successive DecodeData
// calls grow f.decodeBody monotonically. The cap is generous (1 MiB) +
// streamCtx is nil → DataContinue per the NO-op path. The test focuses on
// the accumulator growth.
func TestBody_DecodeData_AccumulatesChunks(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newBodyTestCompiledConfig(t, 1<<20 /* 1 MiB */, "plugin_acc", reg)
	f := &filter{cfg: cc}

	chunk1 := []byte("hello ")
	chunk2 := []byte("world!")
	chunk3 := []byte(" final.")

	if got := f.DecodeData(chunk1, false); got != envoyhttp.DataContinue {
		t.Fatalf("DecodeData chunk1 = %v; want DataContinue (NO-op when streamCtx nil)", got)
	}
	if got := f.DecodeData(chunk2, false); got != envoyhttp.DataContinue {
		t.Fatalf("DecodeData chunk2 = %v; want DataContinue", got)
	}
	if got := f.DecodeData(chunk3, true); got != envoyhttp.DataContinue {
		t.Fatalf("DecodeData chunk3 = %v; want DataContinue", got)
	}

	wantTotal := len(chunk1) + len(chunk2) + len(chunk3)
	if len(f.decodeBody) != wantTotal {
		t.Errorf("accumulated decodeBody length = %d; want %d", len(f.decodeBody), wantTotal)
	}
	if got := string(f.decodeBody); got != "hello world! final." {
		t.Errorf("accumulated decodeBody bytes = %q; want %q", got, "hello world! final.")
	}

	// Sticky flag NOT fired (cap not exceeded).
	if f.decodeBodyCapExceeded {
		t.Errorf("decodeBodyCapExceeded = true; want false (cap not exceeded under 1 MiB)")
	}

	// Counters not bumped.
	if got := findStatCounterValue(reg, "wasm.plugin_acc.body_buffer_cap_exceeded"); got != 0 {
		t.Errorf("body_buffer_cap_exceeded = %d; want 0 (cap not exceeded)", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_acc.envoy_go.failures"); got != 0 {
		t.Errorf("envoy_go.failures = %d; want 0 (no failures)", got)
	}
}

// -----------------------------------------------------------------------------
// 2. Cap-exceeded → 413 + counters + sticky flag.
// -----------------------------------------------------------------------------

// TestBody_DecodeData_CapExceeded_SendsLocalReply_413 asserts that a single
// over-cap chunk fires the sticky flag + body_buffer_cap_exceeded counter +
// envoy_go.failures co-increment + decoderCb.SendLocalReply(413, "Payload
// Too Large", nil) + DataStopIterationNoBuffer.
func TestBody_DecodeData_CapExceeded_SendsLocalReply_413(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newBodyTestCompiledConfig(t, 16 /* tiny 16-byte cap */, "plugin_cap_413", reg)
	rdcb := &recordingDecoderCbCap{}
	f := &filter{cfg: cc, decoderCb: rdcb}

	// 32-byte chunk: exceeds the 16-byte cap immediately.
	oversize := make([]byte, 32)
	for i := range oversize {
		oversize[i] = 'A'
	}

	got := f.DecodeData(oversize, false)
	if got != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("DecodeData over-cap = %v; want DataStopIterationNoBuffer", got)
	}

	// Sticky flag set.
	if !f.decodeBodyCapExceeded {
		t.Errorf("decodeBodyCapExceeded = false; want true after over-cap chunk")
	}

	// Counters bumped.
	if got := findStatCounterValue(reg, "wasm.plugin_cap_413.body_buffer_cap_exceeded"); got != 1 {
		t.Errorf("body_buffer_cap_exceeded = %d; want 1", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_cap_413.envoy_go.failures"); got != 1 {
		t.Errorf("envoy_go.failures = %d; want 1 (§2.25 co-increment)", got)
	}

	// SendLocalReply invoked with 413 + "Payload Too Large".
	if rdcb.calls != 1 {
		t.Errorf("SendLocalReply call count = %d; want 1", rdcb.calls)
	}
	if rdcb.lastStatus != 413 {
		t.Errorf("SendLocalReply status = %d; want 413", rdcb.lastStatus)
	}
	if rdcb.lastBody != "Payload Too Large" {
		t.Errorf("SendLocalReply body = %q; want \"Payload Too Large\"", rdcb.lastBody)
	}
}

// -----------------------------------------------------------------------------
// 3. Sticky flag — no re-dispatch after first cap-exceeded.
// -----------------------------------------------------------------------------

// TestBody_DecodeData_StickyCapExceeded_NoReDispatch asserts that after the
// first cap-exceeded event, subsequent DecodeData calls return DataStop
// IterationNoBuffer WITHOUT re-bumping counters OR re-invoking SendLocalReply.
func TestBody_DecodeData_StickyCapExceeded_NoReDispatch(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newBodyTestCompiledConfig(t, 8 /* 8-byte cap */, "plugin_sticky", reg)
	rdcb := &recordingDecoderCbCap{}
	f := &filter{cfg: cc, decoderCb: rdcb}

	// First over-cap chunk fires the sticky flag.
	chunk1 := make([]byte, 16)
	if got := f.DecodeData(chunk1, false); got != envoyhttp.DataStopIterationNoBuffer {
		t.Fatalf("chunk1 DecodeData = %v; want DataStopIterationNoBuffer", got)
	}
	if rdcb.calls != 1 || findStatCounterValue(reg, "wasm.plugin_sticky.body_buffer_cap_exceeded") != 1 {
		t.Fatalf("pre-condition failed: SendLocalReply calls=%d, body_buffer_cap_exceeded=%d; want 1 each",
			rdcb.calls, findStatCounterValue(reg, "wasm.plugin_sticky.body_buffer_cap_exceeded"))
	}

	// Subsequent chunks: STILL StopAllIteration, but NO re-fire of counters
	// or SendLocalReply.
	for i := 0; i < 3; i++ {
		chunkN := make([]byte, 4)
		if got := f.DecodeData(chunkN, false); got != envoyhttp.DataStopIterationNoBuffer {
			t.Errorf("chunk %d (post-sticky) DecodeData = %v; want DataStopIterationNoBuffer", i+2, got)
		}
	}
	endChunk := make([]byte, 0)
	if got := f.DecodeData(endChunk, true); got != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("end chunk DecodeData = %v; want DataStopIterationNoBuffer", got)
	}

	// Counters STILL at 1; SendLocalReply STILL at 1 call.
	if rdcb.calls != 1 {
		t.Errorf("SendLocalReply call count = %d; want 1 (sticky must not re-fire)", rdcb.calls)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_sticky.body_buffer_cap_exceeded"); got != 1 {
		t.Errorf("body_buffer_cap_exceeded = %d; want 1 (sticky must not re-bump)", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_sticky.envoy_go.failures"); got != 1 {
		t.Errorf("envoy_go.failures = %d; want 1 (sticky must not re-bump)", got)
	}
}

// -----------------------------------------------------------------------------
// 4. NO-op when guest did not export proxy_on_request_body.
// -----------------------------------------------------------------------------

// TestBody_DecodeData_NoOpWhenGuestNotExported asserts that when streamCtx
// is nil (guest didn't construct one OR didn't export the callback), the
// dispatch path is skipped + DataContinue returns.
func TestBody_DecodeData_NoOpWhenGuestNotExported(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newBodyTestCompiledConfig(t, 1<<20, "plugin_noop", reg)
	rdcb := &recordingDecoderCbCap{}
	f := &filter{cfg: cc, decoderCb: rdcb}

	// streamCtx is nil → NO-op path; DecodeData accumulates + returns
	// DataContinue without invoking SendLocalReply.
	if got := f.DecodeData([]byte("payload"), true); got != envoyhttp.DataContinue {
		t.Errorf("DecodeData (no streamCtx) = %v; want DataContinue", got)
	}
	if rdcb.calls != 0 {
		t.Errorf("SendLocalReply call count = %d; want 0 (NO-op should not invoke)", rdcb.calls)
	}
	if string(f.decodeBody) != "payload" {
		t.Errorf("accumulated decodeBody = %q; want \"payload\"", string(f.decodeBody))
	}
}

// -----------------------------------------------------------------------------
// 5. Cap fires regardless of guest opt-in (HOST policy).
// -----------------------------------------------------------------------------

// TestBody_DecodeData_CapEnforcedEvenWithoutGuestOptIn asserts that cap-
// exceeded enforcement fires even when the guest did not opt into body
// callbacks (streamCtx nil). The cap is a HOST policy, not a guest policy
// per 25.2 SPEC §4.3 + Q2.
func TestBody_DecodeData_CapEnforcedEvenWithoutGuestOptIn(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newBodyTestCompiledConfig(t, 4, "plugin_host_cap", reg)
	rdcb := &recordingDecoderCbCap{}
	f := &filter{cfg: cc, decoderCb: rdcb}
	// f.streamCtx is nil — guest did NOT opt into body callbacks.

	oversize := make([]byte, 16)
	if got := f.DecodeData(oversize, false); got != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("DecodeData over-cap (no streamCtx) = %v; want DataStopIterationNoBuffer", got)
	}
	if !f.decodeBodyCapExceeded {
		t.Errorf("decodeBodyCapExceeded = false; want true (cap is HOST policy)")
	}
	if got := findStatCounterValue(reg, "wasm.plugin_host_cap.body_buffer_cap_exceeded"); got != 1 {
		t.Errorf("body_buffer_cap_exceeded = %d; want 1", got)
	}
	if rdcb.calls != 1 || rdcb.lastStatus != 413 {
		t.Errorf("SendLocalReply: calls=%d, status=%d; want 1 / 413", rdcb.calls, rdcb.lastStatus)
	}
}

// -----------------------------------------------------------------------------
// 6. EncodeData mirror — cap-exceeded; SendLocalReply unavailable on encode.
// -----------------------------------------------------------------------------

// TestBody_EncodeData_MirrorsDecodeSide asserts that the encode-side cap-
// exceeded bumps counters + StopAllIteration but does NOT invoke any
// decoderCb.SendLocalReply (EncoderFilterCallbacks does not expose it).
func TestBody_EncodeData_MirrorsDecodeSide(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newBodyTestCompiledConfig(t, 8, "plugin_enc_cap", reg)
	rdcb := &recordingDecoderCbCap{}
	f := &filter{cfg: cc, decoderCb: rdcb}

	oversize := make([]byte, 16)
	if got := f.EncodeData(oversize, false); got != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("EncodeData over-cap = %v; want DataStopIterationNoBuffer", got)
	}

	// Sticky flag on encode side.
	if !f.encodeBodyCapExceeded {
		t.Errorf("encodeBodyCapExceeded = false; want true after over-cap")
	}

	// Counters bumped (same counter as decode side per §7.1).
	if got := findStatCounterValue(reg, "wasm.plugin_enc_cap.body_buffer_cap_exceeded"); got != 1 {
		t.Errorf("body_buffer_cap_exceeded (encode) = %d; want 1", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_enc_cap.envoy_go.failures"); got != 1 {
		t.Errorf("envoy_go.failures (encode) = %d; want 1", got)
	}

	// SendLocalReply NOT invoked on encode side (the warning logs instead).
	if rdcb.calls != 0 {
		t.Errorf("SendLocalReply call count on encode = %d; want 0 (encode side has no SendLocalReply)", rdcb.calls)
	}
}

// -----------------------------------------------------------------------------
// 7. EncodeData sticky flag.
// -----------------------------------------------------------------------------

// TestBody_EncodeData_StickyCapExceeded asserts the encode-side sticky flag
// prevents per-chunk re-bumps of the counters.
func TestBody_EncodeData_StickyCapExceeded(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newBodyTestCompiledConfig(t, 4, "plugin_enc_sticky", reg)
	f := &filter{cfg: cc}

	if got := f.EncodeData(make([]byte, 16), false); got != envoyhttp.DataStopIterationNoBuffer {
		t.Fatalf("first over-cap EncodeData = %v; want DataStopIterationNoBuffer", got)
	}

	for i := 0; i < 3; i++ {
		if got := f.EncodeData(make([]byte, 4), false); got != envoyhttp.DataStopIterationNoBuffer {
			t.Errorf("post-sticky chunk %d EncodeData = %v; want DataStopIterationNoBuffer", i+2, got)
		}
	}

	if got := findStatCounterValue(reg, "wasm.plugin_enc_sticky.body_buffer_cap_exceeded"); got != 1 {
		t.Errorf("body_buffer_cap_exceeded = %d; want 1 (sticky must not re-bump)", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_enc_sticky.envoy_go.failures"); got != 1 {
		t.Errorf("envoy_go.failures = %d; want 1 (sticky must not re-bump)", got)
	}
}

// -----------------------------------------------------------------------------
// 8. Cap-exceeded with no decoderCb: defensive nil-tolerance.
// -----------------------------------------------------------------------------

// TestBody_DecodeData_NilDecoderCb_GracefulDegrade asserts that when
// decoderCb is nil, cap-exceeded still bumps counters + sets sticky flag +
// returns StopAllIteration — only the SendLocalReply step is skipped (with
// a warning log).
func TestBody_DecodeData_NilDecoderCb_GracefulDegrade(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newBodyTestCompiledConfig(t, 4, "plugin_nil_dcb", reg)
	f := &filter{cfg: cc} // decoderCb left nil

	if got := f.DecodeData(make([]byte, 16), false); got != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("DecodeData (nil decoderCb) = %v; want DataStopIterationNoBuffer", got)
	}
	if !f.decodeBodyCapExceeded {
		t.Errorf("decodeBodyCapExceeded = false; want true (cap fires regardless of decoderCb)")
	}
	if got := findStatCounterValue(reg, "wasm.plugin_nil_dcb.body_buffer_cap_exceeded"); got != 1 {
		t.Errorf("body_buffer_cap_exceeded = %d; want 1", got)
	}
}

// -----------------------------------------------------------------------------
// 9. Nil cfg defensive pass-through.
// -----------------------------------------------------------------------------

// TestBody_DecodeData_NilCfg_PassesThrough asserts the defensive nil-cfg
// pass-through (test-double paths constructing *filter{} directly).
func TestBody_DecodeData_NilCfg_PassesThrough(t *testing.T) {
	t.Parallel()
	f := &filter{} // cfg = nil

	if got := f.DecodeData([]byte("anything"), false); got != envoyhttp.DataContinue {
		t.Errorf("DecodeData (nil cfg) = %v; want DataContinue", got)
	}
	if got := f.EncodeData([]byte("anything"), false); got != envoyhttp.DataContinue {
		t.Errorf("EncodeData (nil cfg) = %v; want DataContinue", got)
	}
}

// -----------------------------------------------------------------------------
// Per-route effective config: cap + stats must bind to f.eff, not f.cfg.
// -----------------------------------------------------------------------------

// TestBody_DecodeData_PerRouteOverride_UsesEffectiveConfig asserts that when
// a per-route override is active (f.eff resolved at DecodeHeaders), body-cap
// enforcement uses the OVERRIDE's cap and the body_buffer_cap_exceeded /
// envoy_go.failures counters increment on the OVERRIDE plugin's stat scope —
// not the listener config's (which `executions` already bound to correctly;
// pre-fix the body path read f.cfg for both).
func TestBody_DecodeData_PerRouteOverride_UsesEffectiveConfig(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	// Listener config: generous 1 MiB cap. Per-route override: tiny 8-byte cap.
	listener := newBodyTestCompiledConfig(t, 1<<20, "plugin_listener", reg)
	override := newBodyTestCompiledConfig(t, 8, "plugin_route", reg)
	rdcb := &recordingDecoderCbCap{}
	f := &filter{cfg: listener, eff: override, decoderCb: rdcb}

	// 16-byte chunk: exceeds the override's 8-byte cap but NOT the
	// listener's 1 MiB cap — pre-fix this passed through untouched.
	chunk := make([]byte, 16)
	if got := f.DecodeData(chunk, false); got != envoyhttp.DataStopIterationNoBuffer {
		t.Fatalf("DecodeData over override cap = %v; want DataStopIterationNoBuffer (override cap must apply)", got)
	}
	if !f.decodeBodyCapExceeded {
		t.Error("decodeBodyCapExceeded = false; want true (override's 8-byte cap exceeded)")
	}
	if rdcb.calls != 1 || rdcb.lastStatus != 413 {
		t.Errorf("SendLocalReply calls=%d status=%d; want 1 call with 413", rdcb.calls, rdcb.lastStatus)
	}

	// Counters land on the OVERRIDE plugin's scope...
	if got := findStatCounterValue(reg, "wasm.plugin_route.body_buffer_cap_exceeded"); got != 1 {
		t.Errorf("wasm.plugin_route.body_buffer_cap_exceeded = %d; want 1", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_route.envoy_go.failures"); got != 1 {
		t.Errorf("wasm.plugin_route.envoy_go.failures = %d; want 1", got)
	}
	// ...and NOT on the listener plugin's scope.
	if got := findStatCounterValue(reg, "wasm.plugin_listener.body_buffer_cap_exceeded"); got != 0 {
		t.Errorf("wasm.plugin_listener.body_buffer_cap_exceeded = %d; want 0 (wrong plugin scope)", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_listener.envoy_go.failures"); got != 0 {
		t.Errorf("wasm.plugin_listener.envoy_go.failures = %d; want 0 (wrong plugin scope)", got)
	}
}

// TestBody_EncodeData_PerRouteOverride_UsesEffectiveConfig mirrors the
// decode-side per-route assertion for EncodeData.
func TestBody_EncodeData_PerRouteOverride_UsesEffectiveConfig(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	listener := newBodyTestCompiledConfig(t, 1<<20, "plugin_enc_listener", reg)
	override := newBodyTestCompiledConfig(t, 8, "plugin_enc_route", reg)
	f := &filter{cfg: listener, eff: override}

	chunk := make([]byte, 16)
	if got := f.EncodeData(chunk, false); got != envoyhttp.DataStopIterationNoBuffer {
		t.Fatalf("EncodeData over override cap = %v; want DataStopIterationNoBuffer (override cap must apply)", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_enc_route.body_buffer_cap_exceeded"); got != 1 {
		t.Errorf("wasm.plugin_enc_route.body_buffer_cap_exceeded = %d; want 1", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_enc_listener.body_buffer_cap_exceeded"); got != 0 {
		t.Errorf("wasm.plugin_enc_listener.body_buffer_cap_exceeded = %d; want 0 (wrong plugin scope)", got)
	}
}
