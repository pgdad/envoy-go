package wasm

// trailers_test.go — Task 16 unit tests for trailers.go DecodeTrailers +
// EncodeTrailers per 25.2 SPEC §4.3 + §5.3 C16 + C17.
//
// Test surface (per PLAN Task 16 + acceptance criteria):
//
//   1. TestTrailers_DecodeTrailers_NoOpWhenGuestNotExported — streamCtx is
//      nil OR HasGlobalFunc returns false → TrailersContinue without
//      dispatch attempt.
//
//   2. TestTrailers_DecodeTrailers_NilCfg_PassesThrough — defensive nil-cfg
//      pass-through.
//
//   3. TestTrailers_EncodeTrailers_NoOpWhenGuestNotExported — encode-side
//      no-op when guest didn't opt in.
//
//   4. TestTrailers_NumTrailers_MultiValueExpansion — numTrailers wire arg
//      is the TOTAL value count (multi-value trailers expand per §5.3 C16).
//
//   5. TestTrailers_DecodeTrailers_NilDecoderCb_GracefulDegrade — nil
//      decoderCb does not panic on the captured-local-response path.

import (
	"context"
	gohttp "net/http"
	"testing"
	"time"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// newTrailersTestCompiledConfig is identical to newBodyTestCompiledConfig
// for trailers tests — the cap field is unused on the trailer path but the
// stats wiring + pluginName threading match.
func newTrailersTestCompiledConfig(t *testing.T, pluginName string, reg *stats.Registry) *compiledConfig {
	t.Helper()
	return &compiledConfig{
		pluginName:         pluginName,
		bodyBufferCapBytes: 1 << 20, // 1 MiB cap; unused on trailer path
		stats:              newFilterStats(reg, pluginName),
	}
}

// -----------------------------------------------------------------------------
// 1. NO-op when guest did not export proxy_on_request_trailers.
// -----------------------------------------------------------------------------

// TestTrailers_DecodeTrailers_NoOpWhenGuestNotExported asserts that with
// streamCtx == nil (guest didn't construct one), DecodeTrailers returns
// TrailersContinue without an attempted dispatch.
func TestTrailers_DecodeTrailers_NoOpWhenGuestNotExported(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTrailersTestCompiledConfig(t, "plugin_trailers_noop", reg)
	f := &filter{cfg: cc} // streamCtx nil

	trailers := gohttp.Header{}
	trailers.Set("X-Trailer-1", "value1")
	trailers.Set("X-Trailer-2", "value2")

	if got := f.DecodeTrailers(trailers); got != envoyhttp.TrailersContinue {
		t.Errorf("DecodeTrailers (no streamCtx) = %v; want TrailersContinue", got)
	}

	// No failures bumped on the NO-op path.
	if got := findStatCounterValue(reg, "wasm.plugin_trailers_noop.envoy_go.failures"); got != 0 {
		t.Errorf("envoy_go.failures = %d; want 0 (NO-op should not bump failures)", got)
	}
}

// -----------------------------------------------------------------------------
// 2. Nil cfg defensive pass-through.
// -----------------------------------------------------------------------------

// TestTrailers_DecodeTrailers_NilCfg_PassesThrough asserts the defensive
// nil-cfg pass-through.
func TestTrailers_DecodeTrailers_NilCfg_PassesThrough(t *testing.T) {
	t.Parallel()
	f := &filter{} // cfg nil

	if got := f.DecodeTrailers(gohttp.Header{}); got != envoyhttp.TrailersContinue {
		t.Errorf("DecodeTrailers (nil cfg) = %v; want TrailersContinue", got)
	}
	if got := f.EncodeTrailers(gohttp.Header{}); got != envoyhttp.TrailersContinue {
		t.Errorf("EncodeTrailers (nil cfg) = %v; want TrailersContinue", got)
	}
}

// -----------------------------------------------------------------------------
// 3. EncodeTrailers NO-op when guest did not export proxy_on_response_trailers.
// -----------------------------------------------------------------------------

// TestTrailers_EncodeTrailers_NoOpWhenGuestNotExported asserts the encode-
// side NO-op pass-through when streamCtx is nil.
func TestTrailers_EncodeTrailers_NoOpWhenGuestNotExported(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTrailersTestCompiledConfig(t, "plugin_trailers_enc_noop", reg)
	f := &filter{cfg: cc}

	trailers := gohttp.Header{}
	trailers.Set("Grpc-Status", "0")
	trailers.Set("Grpc-Message", "")

	if got := f.EncodeTrailers(trailers); got != envoyhttp.TrailersContinue {
		t.Errorf("EncodeTrailers (no streamCtx) = %v; want TrailersContinue", got)
	}
}

// -----------------------------------------------------------------------------
// 4. numTrailers multi-value expansion per §5.3 C16.
// -----------------------------------------------------------------------------

// TestTrailers_NumTrailers_MultiValueExpansion exercises the numHeaderValues
// helper (which trailers.go calls to compute the wire numTrailers arg). The
// expansion semantic: multi-value trailers expand to one entry per value,
// matching the GetHeaderMap pair-emission shape per §5.3 C16.
//
// At Task 16 the only observable surface is the count itself; once Task 18
// wires the streamCtx end-to-end, this can be extended to a full dispatch
// assertion via mock guest.
func TestTrailers_NumTrailers_MultiValueExpansion(t *testing.T) {
	t.Parallel()
	trailers := gohttp.Header{}
	trailers.Add("X-Multi", "value1")
	trailers.Add("X-Multi", "value2")
	trailers.Add("X-Multi", "value3")
	trailers.Set("X-Single", "soloval")

	got := numHeaderValues(trailers)
	want := uint32(4) // 3 X-Multi + 1 X-Single

	if got != want {
		t.Errorf("numHeaderValues(multi-value) = %d; want %d", got, want)
	}
}

// -----------------------------------------------------------------------------
// 5. Nil decoderCb defensive pass-through.
// -----------------------------------------------------------------------------

// TestTrailers_DecodeTrailers_NilDecoderCb_DoesNotPanic asserts that when
// streamCtx is nil + decoderCb is nil, DecodeTrailers does not panic on
// the captured-local-response branch (the branch is short-circuited by
// the streamCtx-nil gate above it).
func TestTrailers_DecodeTrailers_NilDecoderCb_DoesNotPanic(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTrailersTestCompiledConfig(t, "plugin_trailers_nil_dcb", reg)
	f := &filter{cfg: cc} // streamCtx + decoderCb both nil

	if got := f.DecodeTrailers(gohttp.Header{}); got != envoyhttp.TrailersContinue {
		t.Errorf("DecodeTrailers (nil streamCtx + decoderCb) = %v; want TrailersContinue", got)
	}
}

// -----------------------------------------------------------------------------
// 6. EncodeTrailers + EncodeData mirror behavior is exercised at body_test.go.
// -----------------------------------------------------------------------------

// (Encode-side trailer mirror tests would follow the same pattern as the
// decode tests above; the NO-op-when-streamCtx-nil + nil-cfg patterns are
// the only observable behaviors at Task 16. End-to-end streamCtx-driven
// scenarios land at Task 18 dispatch_test.go EXTEND.)

// -----------------------------------------------------------------------------
// PHASE-83 S2 — THE TWO TRAILERS PAUSE ARMS.
// -----------------------------------------------------------------------------
//
// # What was broken
//
// trailers.go's two `abi.ProxyActionPause` arms returned TrailersStopIteration
// and set NO pause bookkeeping at all. That is a status that PARKS:
// FilterChain.RunDecodeTrailers blocks in parkDecode and RunEncodeTrailers
// blocks in parkEncode. Because no flag was set and no watchdog was armed, the
// park had NO resumer of any kind:
//
//   - the S9 watchdog was never armed, so nothing bounded it, and there is no
//     stream idle timeout ANYWHERE in this tree to bound it from the outside
//     either (re-derived at this task: `idle_timeout` has exactly two non-test
//     occurrences repo-wide, listener/manager.go and http/buffer/doc.go, and
//     BOTH are comments recording it as deferred);
//   - f.decodePaused stayed FALSE, so the guest's own proxy_continue_stream
//     lost resumeDecode's CompareAndSwap and fired NOTHING — while the ABI
//     call still returned WasmResult::Ok, so the guest could not observe the
//     loss.
//
// ⇒ the park was PERMANENTLY UNRESUMABLE, bounded by nothing at all.
//
// # Why the anchors are synthetic, and must be
//
// Re-derived at this task with the command, not inherited: Run*Trailers is
// DEAD PRODUCTION CODE. Every non-test occurrence of RunDecodeTrailers /
// RunEncodeTrailers in the repository is a comment (hcm/connection.go,
// hcm/h2dispatch.go, http/router/router.go, wasm/body.go, and the 0007b
// fixture driver) or a definition (chain.go). Nothing in HCM ever calls
// either. There is likewise no in-tree guest crate, no vendored blob and no
// differential fixture that returns Action::Pause from a trailers callback.
// Without buildPauseRequestTrailersProxyWasm / buildPauseResponseTrailers
// ProxyWasm and a hand-driven FilterChain there is literally nothing to make
// RED.
//
// # ⚠️ S2 MAKES THE PARK BOUNDED, NOT USEFUL
//
// A guest that pauses from a trailers callback is BLIND, for two independent
// reasons, and NEITHER is in this row's charter:
//
//  1. abi_callbacks.headerMapForType routes WasmHeaderMapType 1/3/4/5 to
//     `default: return nil, false, false`, so the guest cannot read the
//     trailer map it was called about; and
//  2. trailers.go never captures the trailers at all — `_ = trailers` at
//     :101 and :165 — so fixing the routing alone would still hand the guest
//     an empty map.
//
// Both are DEFERRED BY NAME in the phase-83 PLAN §11. Bounding the park is
// still worth landing on its own: an unresumable park is a connection and
// goroutine leak whether or not the guest could have done anything useful
// while parked.

// p83TrailersParkBudget is how long a chain-level anchor waits for
// Run*Trailers to return before declaring the park unbounded. DELIBERATELY
// much larger than p83TrailersWatchdog: on the fixed tree the wait costs
// ~p83TrailersWatchdog, and the budget is spent ONLY on the failing tree.
const p83TrailersParkBudget = 10 * time.Second

// p83TrailersWatchdog is the per-stream watchdog override used by the
// chain-level anchors.
const p83TrailersWatchdog = 400 * time.Millisecond

// p83TrailersFilter builds a wasm *filter around modBytes, wires it into a
// single-filter FilterChain, and runs decode headers to completion so the
// per-stream StreamContext exists.
//
// ⚠️ RunDecodeHeaders MUST complete before ANY trailers test, on BOTH sides.
// The per-stream StreamContext is constructed only in DecodeHeaders (encode_
// headers.go:56 short-circuits the whole encode dispatch on a nil
// f.streamCtx, and trailers.go's own nil guard returns TrailersContinue), so
// an encode-side trailers test that skips the decode leg reads GREEN with the
// arm under test never executed. That is what forces this shared helper.
//
// ⚠️ THE LIVENESS BARRIER IS HasGlobalFunc, NOT THE `executions` COUNTER.
// decode_headers.go increments stats.executions at :213, BEFORE the
// CallProxyOnRequestHeaders dispatch at :225 — and the capability gate lives
// INSIDE that call, in StreamContext.dispatchGuest. `executions == 1` is
// therefore measurable with EVERY capability denied: it is green on exactly
// the failure it would exist to catch.
func p83TrailersFilter(t *testing.T, modBytes []byte, pluginName, wantFunc string) (*filter, *envoyhttp.FilterChain, *stats.Registry) {
	t.Helper()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfigWithCaps(t, modBytes, pluginName, reg)
	t.Cleanup(func() { _ = cc.Close() })

	wf := &filter{cfg: cc}
	wf.pauseWatchdog = p83TrailersWatchdog

	chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
		{Name: "wasm", Decoder: wf, Encoder: wf},
	}, nil)
	t.Cleanup(chain.Destroy)

	h := gohttp.Header{}
	h.Set(":method", "POST")
	terminated, err := chain.RunDecodeHeaders(context.Background(), h, false)
	if err != nil || !terminated {
		t.Fatalf("RunDecodeHeaders(terminated=%v, err=%v); want (true, nil) — the fixture's request-headers callback must Continue so the trailers arm is reachable", terminated, err)
	}
	if wf.streamCtx == nil {
		t.Fatalf("streamCtx is nil after RunDecodeHeaders; the per-stream context was never built and every trailers dispatch would short-circuit")
	}
	if !wf.streamCtx.HasGlobalFunc(wantFunc) {
		t.Fatalf("HasGlobalFunc(%q) = false; the guest does not export the callback under test and trailers.go would return TrailersContinue without ever reaching the ProxyAction switch", wantFunc)
	}
	return wf, chain, reg
}

// p83TrailersFixture returns a two-entry trailer map used by every anchor
// below. Two entries rather than one so numHeaderValues is not incidentally
// zero on a path where a zero would be indistinguishable from "not called".
func p83TrailersFixture() gohttp.Header {
	h := gohttp.Header{}
	h.Set("Grpc-Status", "0")
	h.Set("X-P83-Trailer", "v")
	return h
}

// -----------------------------------------------------------------------------
// GATE 0 — THE TYPE SECTION. Run FIRST; every anchor below is vacuous if it
// fails.
// -----------------------------------------------------------------------------

// TestFixture_PauseTrailers_ActionLiveness asserts the two new trailers
// fixtures actually return ProxyActionPause with a NIL error.
//
// ⚠️ THIS GATES THE TYPE SECTION SPECIFICALLY, AND IT IS NOT PARANOIA.
// CallProxyOnRequestTrailers / CallProxyOnResponseTrailers pass exactly TWO
// i32 arguments (internal/wasm/stream_context.go), unlike every other fixture
// in wasm_fixtures_test.go, whose callbacks are (i32,i32,i32) -> i32. A module
// that declares the wrong signature INSTANTIATES FINE: wazero rejects it only
// at CALL time ("expected 3 params, but passed 2"), trailers.go fail-OPENs on
// that error and returns TrailersContinue, and every anchor below then
// silently skips the arm and reads GREEN — distinguishable only by an
// envoy_go.failures bump, and under a denied capability not even by that
// (err = nil, failures = 0).
//
// ⇒ asserting that the module instantiates proves NOTHING. `action = Pause`
// AND `err = nil` is what separates "the guest paused" from "the guest could
// not be called".
func TestFixture_PauseTrailers_ActionLiveness(t *testing.T) {
	t.Parallel()

	t.Run("request_trailers", func(t *testing.T) {
		wf, _, reg := p83TrailersFilter(t, buildPauseRequestTrailersProxyWasm(), "p83_trl_fixlive_req", proxyOnRequestTrailers)
		act, err := wf.streamCtx.CallProxyOnRequestTrailers(context.Background(), 2)
		if err != nil {
			t.Errorf("CallProxyOnRequestTrailers err = %v; want nil (a non-nil error means the type section is wrong, the host FAIL-OPENED, and the Pause arm is dead code in every anchor below)", err)
		}
		if act != abi.ProxyActionPause {
			t.Errorf("CallProxyOnRequestTrailers = %v; want ProxyActionPause (=1)", act)
		}
		// The fail-OPEN arm is the one that would hide a wrong type section,
		// and it bumps this counter. A zero delta is the second, independent
		// witness that the call really reached the guest.
		if got := findStatCounterValue(reg, "wasm.p83_trl_fixlive_req.envoy_go.failures"); got != 0 {
			t.Errorf("envoy_go.failures = %d; want 0 — a non-zero value means the dispatch errored and trailers.go fail-OPENed", got)
		}
	})

	t.Run("response_trailers", func(t *testing.T) {
		wf, _, reg := p83TrailersFilter(t, buildPauseResponseTrailersProxyWasm(), "p83_trl_fixlive_resp", proxyOnResponseTrailers)
		act, err := wf.streamCtx.CallProxyOnResponseTrailers(context.Background(), 2)
		if err != nil {
			t.Errorf("CallProxyOnResponseTrailers err = %v; want nil", err)
		}
		if act != abi.ProxyActionPause {
			t.Errorf("CallProxyOnResponseTrailers = %v; want ProxyActionPause (=1)", act)
		}
		if got := findStatCounterValue(reg, "wasm.p83_trl_fixlive_resp.envoy_go.failures"); got != 0 {
			t.Errorf("envoy_go.failures = %d; want 0", got)
		}
	})
}

// -----------------------------------------------------------------------------
// ANCHOR 1 + 2 — the chain-level unbounded park, both sides.
// -----------------------------------------------------------------------------

// TestFilterChain_DecodeTrailers_Pause_ParkIsBounded is the decode-side S2
// regression, driven through a REAL FilterChain.
//
// ⚠️ IT MUST BE DRIVEN THROUGH THE CHAIN. Calling f.DecodeTrailers directly
// returns a status immediately — the park is a property of RunDecodeTrailers,
// not of the filter method, so a filter-level test cannot observe it at all.
//
// ⚠️ THE BARRIER IS "RunDecodeTrailers HAS NOT RETURNED", NEVER THE PAUSE
// FLAG. A t.Fatalf on decodePaused would fire FIRST on the unfixed tree (the
// unfixed arm sets no flag) and make the MEASURED HAZARD assertion below DEAD
// CODE. The unfixed tree must reach the hazard assertion and fail on IT.
func TestFilterChain_DecodeTrailers_Pause_ParkIsBounded(t *testing.T) {
	t.Parallel()
	wf, chain, _ := p83TrailersFilter(t, buildPauseRequestTrailersProxyWasm(), "p83_trl_park_decode", proxyOnRequestTrailers)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = chain.RunDecodeTrailers(context.Background(), p83TrailersFixture())
	}()

	// Force-release on the way out. Without this, EVERY red run leaks a parked
	// dispatch goroutine for the remainder of the package.
	t.Cleanup(func() {
		select {
		case <-done:
			return
		default:
		}
		if cb := wf.decoderCb; cb != nil {
			cb.ContinueDecoding()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("the parked RunDecodeTrailers goroutine could not be reaped even with a direct ContinueDecoding")
		}
	})

	// CONTROL: the pause must be HONORED, i.e. the chain must still be parked
	// well inside the watchdog window. A chain that returns immediately would
	// satisfy the hazard assertion below for the wrong reason.
	select {
	case <-done:
		t.Errorf("RunDecodeTrailers returned within %v — far inside the %v watchdog window; the Pause was not honored at all", p83TrailersWatchdog/8, p83TrailersWatchdog)
	case <-time.After(p83TrailersWatchdog / 8):
	}

	start := time.Now()
	select {
	case <-done:
		t.Logf("MEASURED: RunDecodeTrailers released after %v (watchdog %v)", time.Since(start)+p83TrailersWatchdog/8, p83TrailersWatchdog)
	case <-time.After(p83TrailersParkBudget):
		t.Errorf("MEASURED HAZARD: RunDecodeTrailers never returned: the guest paused from proxy_on_request_trailers and NOTHING bounded the park — no watchdog was armed, the guest's own proxy_continue_stream loses resumeDecode's CAS because no flag was set, and this tree has no stream idle timeout")
		return
	}

	if wf.decodePaused.Load() {
		t.Errorf("decodePaused = true after the watchdog released the park; want false (the watchdog consumes the flag through the same CAS the guest would)")
	}
	wf.pauseMu.Lock()
	handle := wf.decodePauseTimer
	wf.pauseMu.Unlock()
	if handle == nil {
		t.Errorf("decodePauseTimer = nil after a trailers pause; want the armed handle (a nil handle means no watchdog was ever armed and the release above came from somewhere else)")
	}
}

// TestFilterChain_EncodeTrailers_Pause_ParkIsBounded is the encode-side mirror.
//
// The encode side is the harsher of the two, re-derived here rather than
// inherited: all SIX of chain.go's in-iterator c.localReplyDone.Load()
// re-checks sit in RunDecodeHeaders / RunDecodeData / RunDecodeTrailers (the
// seventh occurrence in the file is the exported LocalReplyDone accessor), and
// RunEncodeTrailers carries ZERO. An encode-side trailers park has no
// secondary escape of any kind.
func TestFilterChain_EncodeTrailers_Pause_ParkIsBounded(t *testing.T) {
	t.Parallel()
	wf, chain, _ := p83TrailersFilter(t, buildPauseResponseTrailersProxyWasm(), "p83_trl_park_encode", proxyOnResponseTrailers)

	respHeaders := gohttp.Header{}
	respHeaders.Set("Content-Type", "text/plain")
	terminated, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false)
	if err != nil || !terminated {
		t.Fatalf("RunEncodeHeaders(terminated=%v, err=%v); want (true, nil)", terminated, err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = chain.RunEncodeTrailers(context.Background(), p83TrailersFixture())
	}()

	t.Cleanup(func() {
		select {
		case <-done:
			return
		default:
		}
		if cb := wf.encoderCb; cb != nil {
			cb.ContinueEncoding()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("the parked RunEncodeTrailers goroutine could not be reaped even with a direct ContinueEncoding")
		}
	})

	select {
	case <-done:
		t.Errorf("RunEncodeTrailers returned within %v — far inside the %v watchdog window; the Pause was not honored at all", p83TrailersWatchdog/8, p83TrailersWatchdog)
	case <-time.After(p83TrailersWatchdog / 8):
	}

	start := time.Now()
	select {
	case <-done:
		t.Logf("MEASURED: RunEncodeTrailers released after %v (watchdog %v)", time.Since(start)+p83TrailersWatchdog/8, p83TrailersWatchdog)
	case <-time.After(p83TrailersParkBudget):
		t.Errorf("MEASURED HAZARD: RunEncodeTrailers never returned: the guest paused from proxy_on_response_trailers and NOTHING bounded the park — and unlike the decode side there is not even a localReplyDone re-check to escape through")
		return
	}

	if wf.encodePaused.Load() {
		t.Errorf("encodePaused = true after the watchdog released the park; want false")
	}
	wf.pauseMu.Lock()
	handle := wf.encodePauseTimer
	wf.pauseMu.Unlock()
	if handle == nil {
		t.Errorf("encodePauseTimer = nil after a trailers pause; want the armed handle")
	}
}

// -----------------------------------------------------------------------------
// ANCHOR 3 + 4 — the flag and the armed watchdog, isolated from the chain.
// -----------------------------------------------------------------------------

// TestFilter_DecodeTrailers_Pause_SetsFlagAndArms isolates the bookkeeping the
// chain anchor can only observe indirectly.
//
// The chain anchor goes green as soon as SOMETHING releases the park; these
// two pin WHAT. The watchdog is set to an hour so it cannot be what clears the
// flag during the test, and OnDestroy disarms it in cleanup.
//
// The generation bump is asserted separately from the flag because they fail
// independently: beginDecodePause could set the flag without arming (the
// decoderCb == nil path does exactly that), and the S3 guard makes an
// un-bumped generation a live correctness defect rather than a cosmetic one.
func TestFilter_DecodeTrailers_Pause_SetsFlagAndArms(t *testing.T) {
	t.Parallel()
	wf, _, _ := p83TrailersFilter(t, buildPauseRequestTrailersProxyWasm(), "p83_trl_flag_decode", proxyOnRequestTrailers)
	wf.pauseWatchdog = time.Hour
	t.Cleanup(wf.OnDestroy)

	if wf.decoderCb == nil {
		t.Fatalf("decoderCb is nil; beginDecodePause would return EARLY leaving the flag set and NO timer armed, and the handle assertion below would be vacuous")
	}
	genBefore := wf.decodePauseGen.Load()

	if got := wf.DecodeTrailers(p83TrailersFixture()); got != envoyhttp.TrailersStopIteration {
		t.Fatalf("DecodeTrailers = %v; want TrailersStopIteration (the guest returned Pause)", got)
	}
	if !wf.decodePaused.Load() {
		t.Errorf("decodePaused = false after the trailers Pause arm; want true — without the flag the guest's own proxy_continue_stream loses resumeDecode's CompareAndSwap and fires NOTHING, while ContinueStream still returns WasmResult::Ok so the guest cannot observe the loss")
	}
	wf.pauseMu.Lock()
	handle := wf.decodePauseTimer
	wf.pauseMu.Unlock()
	if handle == nil {
		t.Errorf("decodePauseTimer = nil after the trailers Pause arm; want an armed watchdog — nothing else in this tree bounds the park (there is no stream idle timeout)")
	}
	if got := wf.decodePauseGen.Load(); got <= genBefore {
		t.Errorf("decodePauseGen %d → %d across the trailers Pause arm; want a BUMP (the closure captures the generation current at arm time; without the bump the S3 guard cannot tell this pause from a superseded one)", genBefore, got)
	}
}

// TestFilter_EncodeTrailers_Pause_SetsFlagAndArms is the encode-side mirror.
func TestFilter_EncodeTrailers_Pause_SetsFlagAndArms(t *testing.T) {
	t.Parallel()
	wf, chain, _ := p83TrailersFilter(t, buildPauseResponseTrailersProxyWasm(), "p83_trl_flag_encode", proxyOnResponseTrailers)
	wf.pauseWatchdog = time.Hour
	t.Cleanup(wf.OnDestroy)

	respHeaders := gohttp.Header{}
	respHeaders.Set("Content-Type", "text/plain")
	if _, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false); err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}

	if wf.encoderCb == nil {
		t.Fatalf("encoderCb is nil; beginEncodePause would return EARLY leaving the flag set and NO timer armed")
	}
	genBefore := wf.encodePauseGen.Load()

	if got := wf.EncodeTrailers(p83TrailersFixture()); got != envoyhttp.TrailersStopIteration {
		t.Fatalf("EncodeTrailers = %v; want TrailersStopIteration (the guest returned Pause)", got)
	}
	if !wf.encodePaused.Load() {
		t.Errorf("encodePaused = false after the trailers Pause arm; want true")
	}
	wf.pauseMu.Lock()
	handle := wf.encodePauseTimer
	wf.pauseMu.Unlock()
	if handle == nil {
		t.Errorf("encodePauseTimer = nil after the trailers Pause arm; want an armed watchdog")
	}
	if got := wf.encodePauseGen.Load(); got <= genBefore {
		t.Errorf("encodePauseGen %d → %d across the trailers Pause arm; want a BUMP", genBefore, got)
	}
}
