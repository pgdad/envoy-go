package wasm

// pause_body_test.go — phase-83 S1: the TWO BODY Pause arms.
//
// # What was broken
//
// body.go's two `abi.ProxyActionPause` arms returned
// DataStopIterationAndBuffer and set NO pause bookkeeping at all. That is a
// status that PARKS: FilterChain.RunDecodeData appends the chunk to
// c.decodeBuf and then blocks in parkDecode, and RunEncodeData does the same
// through parkEncode. Because no flag was set and no watchdog was armed, the
// park had NO resumer of any kind:
//
//   - the S9 watchdog was never armed, so nothing bounded it;
//   - f.decodePaused stayed FALSE, so the guest's own proxy_continue_stream
//     lost resumeDecode's CompareAndSwap and fired NOTHING — the ABI call
//     still returned WasmResult::Ok, so the guest could not even observe the
//     loss.
//
// The three chain-level anchors below drove that: pre-fix each one hung for
// its full 10 s budget and reported the park; post-fix each returns in the
// watchdog window.
//
// # Why a synthetic fixture is unavoidable
//
// Re-derived at this task, not inherited: NOTHING in this repository reaches
// either body Pause arm. No in-tree guest crate returns Action::Pause from
// on_http_request_body / on_http_response_body, no vendored blob does, and
// the one differential fixture with a wasm body callback (0036 (n)) trips
// body.go's byte cap BEFORE any guest dispatch, so it returns at the cap arm
// and never reaches the ProxyAction switch. Without buildPauseRequestBody
// ProxyWasm / buildPauseResponseBodyProxyWasm there is literally nothing to
// make RED.
//
// # The three fixes, and which test isolates which
//
//	beginDecodePauseOnce / beginEncodePauseOnce   TestFilter_*Data_Pause_ArmsOnce
//	disarmDecodePause / disarmEncodePause on the
//	  Continue arm                                TestFilter_*Data_ContinueDisarms*
//	  and on the FAIL-OPEN err arm                TestFilter_DecodeData_FailOpen_DisarmsPause
//
// The "no fix at all" RED does NOT discriminate between them, which is why
// each has its own isolating assertion and its own negative control.

import (
	"context"
	"net/http"
	"testing"
	"time"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// p83BodyParkBudget is how long a chain-level anchor waits for Run*Data to
// return before declaring the park unbounded. It is DELIBERATELY much larger
// than p83BodyWatchdog: on the fixed tree the wait costs ~p83BodyWatchdog, and
// the budget is spent ONLY on the failing tree.
const p83BodyParkBudget = 10 * time.Second

// p83BodyWatchdog is the per-stream watchdog override used by the chain-level
// anchors. Short enough that a green run is fast, long enough that the
// "did not return EARLY" control below is not a coin flip.
const p83BodyWatchdog = 400 * time.Millisecond

// p83BodyFilter builds a wasm *filter around modBytes, wires it into a
// single-filter FilterChain, and runs decode headers to completion so the
// per-stream StreamContext exists.
//
// ⚠️ RunDecodeHeaders MUST complete before any body test: the per-stream
// StreamContext is constructed only in DecodeHeaders, and body.go
// short-circuits to DataContinue on a nil f.streamCtx. A body test that skips
// it reads green with the arm under test never executed.
//
// The returned filter has HasGlobalFunc(wantFunc) already asserted — the
// CAPABILITY liveness barrier. It is NOT the `executions` counter: that
// counter is bumped BEFORE CallProxyOnRequestHeaders and the capability gate
// lives INSIDE that call, so `executions == 1` is measurable with every
// capability denied.
func p83BodyFilter(t *testing.T, modBytes []byte, pluginName, wantFunc string) (*filter, *envoyhttp.FilterChain, *stats.Registry) {
	t.Helper()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfigWithCaps(t, modBytes, pluginName, reg)
	t.Cleanup(func() { _ = cc.Close() })

	wf := &filter{cfg: cc}
	wf.pauseWatchdog = p83BodyWatchdog

	chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
		{Name: "wasm", Decoder: wf, Encoder: wf},
	}, nil)
	t.Cleanup(chain.Destroy)

	h := http.Header{}
	h.Set(":method", "POST")
	terminated, err := chain.RunDecodeHeaders(context.Background(), h, false)
	if err != nil || !terminated {
		t.Fatalf("RunDecodeHeaders(terminated=%v, err=%v); want (true, nil) — the fixture's request-headers callback must Continue so the body arm is reachable", terminated, err)
	}
	if wf.streamCtx == nil {
		t.Fatalf("streamCtx is nil after RunDecodeHeaders; the per-stream context was never built and every body dispatch would short-circuit")
	}
	if !wf.streamCtx.HasGlobalFunc(wantFunc) {
		t.Fatalf("HasGlobalFunc(%q) = false; the guest does not export the callback under test and body.go would return DataContinue without ever reaching the ProxyAction switch", wantFunc)
	}
	return wf, chain, reg
}

// -----------------------------------------------------------------------------
// FIXTURE LIVENESS — run FIRST, because every other test in this file is
// vacuous if it fails.
// -----------------------------------------------------------------------------

// TestFixture_PauseBody_ActionLiveness asserts the two new fixtures actually
// return ProxyActionPause with a NIL error.
//
// ⚠️ THIS IS NOT PARANOIA. A wasm module with a wrong type-section entry
// INSTANTIATES FINE and fails only at call time inside wazero; the host
// fail-OPENs on that error and returns DataContinue. Every downstream test
// would then silently skip the arm and read GREEN. `err == nil` is what
// separates "the guest paused" from "the guest could not be called".
func TestFixture_PauseBody_ActionLiveness(t *testing.T) {
	t.Parallel()

	t.Run("request_body", func(t *testing.T) {
		wf, _, _ := p83BodyFilter(t, buildPauseRequestBodyProxyWasm(), "p83_fixlive_req", proxyOnRequestBody)
		act, err := wf.streamCtx.CallProxyOnRequestBody(context.Background(), 4, false)
		if err != nil {
			t.Errorf("CallProxyOnRequestBody(endStream=false) err = %v; want nil (a non-nil error means the host FAIL-OPENED and the Pause arm is dead code in every test below)", err)
		}
		if act != abi.ProxyActionPause {
			t.Errorf("CallProxyOnRequestBody(endStream=false) = %v; want ProxyActionPause", act)
		}
		act, err = wf.streamCtx.CallProxyOnRequestBody(context.Background(), 4, true)
		if err != nil {
			t.Errorf("CallProxyOnRequestBody(endStream=true) err = %v; want nil", err)
		}
		if act != abi.ProxyActionContinue {
			t.Errorf("CallProxyOnRequestBody(endStream=true) = %v; want ProxyActionContinue (the `1 - end_of_stream` body is what lets ONE fixture gate both the Pause arm and its disarm)", act)
		}
	})

	t.Run("response_body", func(t *testing.T) {
		wf, _, _ := p83BodyFilter(t, buildPauseResponseBodyProxyWasm(), "p83_fixlive_resp", proxyOnResponseBody)
		act, err := wf.streamCtx.CallProxyOnResponseBody(context.Background(), 4, false)
		if err != nil {
			t.Errorf("CallProxyOnResponseBody(endStream=false) err = %v; want nil", err)
		}
		if act != abi.ProxyActionPause {
			t.Errorf("CallProxyOnResponseBody(endStream=false) = %v; want ProxyActionPause", act)
		}
		act, err = wf.streamCtx.CallProxyOnResponseBody(context.Background(), 4, true)
		if err != nil {
			t.Errorf("CallProxyOnResponseBody(endStream=true) err = %v; want nil", err)
		}
		if act != abi.ProxyActionContinue {
			t.Errorf("CallProxyOnResponseBody(endStream=true) = %v; want ProxyActionContinue", act)
		}
	})
}

// -----------------------------------------------------------------------------
// ANCHOR 1 + 2 — the chain-level unbounded park, both sides.
// -----------------------------------------------------------------------------

// TestFilterChain_DecodeData_Pause_ParkIsBounded is the decode-side S1
// regression, driven through a REAL FilterChain.
//
// ⚠️ IT MUST BE DRIVEN THROUGH THE CHAIN. Calling f.DecodeData directly
// returns a status and returns immediately — the park is a property of
// RunDecodeData, not of the filter method, so a filter-level test cannot
// observe it at all.
//
// ⚠️ THE BARRIER IS "RunDecodeData HAS NOT RETURNED", NOT THE PAUSE FLAG. A
// t.Fatalf on decodePaused would fire FIRST on the unfixed tree (the unfixed
// arm sets no flag) and make the hazard assertion below DEAD CODE. The
// unfixed tree must reach the hazard assertion and fail on IT.
func TestFilterChain_DecodeData_Pause_ParkIsBounded(t *testing.T) {
	t.Parallel()
	wf, chain, _ := p83BodyFilter(t, buildPauseRequestBodyProxyWasm(), "p83_park_decode", proxyOnRequestBody)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = chain.RunDecodeData(context.Background(), []byte("chunk-1"), false)
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
			t.Errorf("the parked RunDecodeData goroutine could not be reaped even with a direct ContinueDecoding")
		}
	})

	// CONTROL: the pause must be HONORED, i.e. the chain must still be parked
	// well inside the watchdog window. A chain that returns immediately would
	// satisfy the hazard assertion below for the wrong reason.
	select {
	case <-done:
		t.Errorf("RunDecodeData returned within %v — far inside the %v watchdog window; the Pause was not honored at all", p83BodyWatchdog/8, p83BodyWatchdog)
	case <-time.After(p83BodyWatchdog / 8):
	}

	start := time.Now()
	select {
	case <-done:
		t.Logf("MEASURED: RunDecodeData released after %v (watchdog %v)", time.Since(start)+p83BodyWatchdog/8, p83BodyWatchdog)
	case <-time.After(p83BodyParkBudget):
		t.Errorf("MEASURED HAZARD: RunDecodeData never returned: the guest paused from proxy_on_request_body and nothing bounded the park")
		return
	}

	if wf.decodePaused.Load() {
		t.Errorf("decodePaused = true after the watchdog released the park; want false (the watchdog consumes the flag through the same CAS the guest would)")
	}
	wf.pauseMu.Lock()
	handle := wf.decodePauseTimer
	wf.pauseMu.Unlock()
	if handle == nil {
		t.Errorf("decodePauseTimer = nil after a body pause; want the armed handle (a nil handle means no watchdog was ever armed and the release above came from somewhere else)")
	}
}

// TestFilterChain_EncodeData_Pause_ParkIsBounded is the encode-side mirror.
//
// The encode side is the harsher of the two, re-derived here rather than
// inherited: all SIX of chain.go's in-iterator c.localReplyDone.Load()
// re-checks are in RunDecodeHeaders / RunDecodeData / RunDecodeTrailers (the
// seventh occurrence in the file is the exported LocalReplyDone accessor), and
// the three RunEncode* iterators carry ZERO. An encode-side park has no
// secondary escape at all.
func TestFilterChain_EncodeData_Pause_ParkIsBounded(t *testing.T) {
	t.Parallel()
	wf, chain, _ := p83BodyFilter(t, buildPauseResponseBodyProxyWasm(), "p83_park_encode", proxyOnResponseBody)

	respHeaders := http.Header{}
	respHeaders.Set("Content-Type", "text/plain")
	terminated, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false)
	if err != nil || !terminated {
		t.Fatalf("RunEncodeHeaders(terminated=%v, err=%v); want (true, nil)", terminated, err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = chain.RunEncodeData(context.Background(), []byte("chunk-1"), false)
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
			t.Errorf("the parked RunEncodeData goroutine could not be reaped even with a direct ContinueEncoding")
		}
	})

	select {
	case <-done:
		t.Errorf("RunEncodeData returned within %v — far inside the %v watchdog window; the Pause was not honored at all", p83BodyWatchdog/8, p83BodyWatchdog)
	case <-time.After(p83BodyWatchdog / 8):
	}

	start := time.Now()
	select {
	case <-done:
		t.Logf("MEASURED: RunEncodeData released after %v (watchdog %v)", time.Since(start)+p83BodyWatchdog/8, p83BodyWatchdog)
	case <-time.After(p83BodyParkBudget):
		t.Errorf("MEASURED HAZARD: RunEncodeData never returned: the guest paused from proxy_on_response_body and nothing bounded the park")
		return
	}

	if wf.encodePaused.Load() {
		t.Errorf("encodePaused = true after the watchdog released the park; want false")
	}
	wf.pauseMu.Lock()
	handle := wf.encodePauseTimer
	wf.pauseMu.Unlock()
	if handle == nil {
		t.Errorf("encodePauseTimer = nil after a body pause; want the armed handle")
	}
}

// -----------------------------------------------------------------------------
// ANCHOR 3 — the guest's OWN resume.
// -----------------------------------------------------------------------------

// TestAbiCallbacks_ContinueStream_ResumesBodyPause proves the OTHER half of
// the defect, the half the watchdog hides: before S1 the body arm set no
// flag, so resumeDecode's CompareAndSwap FAILED and proxy_continue_stream
// fired nothing — while still returning WasmResult::Ok to the guest, which
// therefore could not detect that its resume was dropped.
//
// The watchdog is pinned to an hour here precisely so it cannot be what
// releases the park. If this test goes green with a short watchdog it proves
// nothing about ContinueStream.
func TestAbiCallbacks_ContinueStream_ResumesBodyPause(t *testing.T) {
	t.Parallel()
	wf, chain, _ := p83BodyFilter(t, buildPauseRequestBodyProxyWasm(), "p83_continuestream_body", proxyOnRequestBody)
	wf.pauseWatchdog = time.Hour

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = chain.RunDecodeData(context.Background(), []byte("chunk-1"), false)
	}()

	t.Cleanup(func() {
		select {
		case <-done:
			return
		default:
		}
		if cb := wf.decoderCb; cb != nil {
			cb.ContinueDecoding()
		}
		<-done
	})

	// Give the dispatch goroutine time to reach the park. This is a settle,
	// not the assertion: the assertion is that the resume below releases it.
	time.Sleep(50 * time.Millisecond)

	acb := &abiCallbacks{filter: wf}
	if got := acb.ContinueStream(context.Background(), 1, 0); got != abi.WasmResultOk {
		t.Errorf("ContinueStream = %v; want Ok", got)
	}

	select {
	case <-done:
	case <-time.After(p83BodyParkBudget):
		t.Errorf("MEASURED HAZARD: RunDecodeData never returned after the guest's proxy_continue_stream: the body Pause arm set no decodePaused flag, so resumeDecode's CompareAndSwap failed and NO ContinueDecoding was fired — and ContinueStream still returned Ok, so the guest cannot observe the loss")
	}

	if wf.decodePaused.Load() {
		t.Errorf("decodePaused = true after the guest resumed; want false")
	}
}

// -----------------------------------------------------------------------------
// ARM-ONCE — isolating anchor for beginDecodePauseOnce / beginEncodePauseOnce.
// -----------------------------------------------------------------------------

// TestFilter_Data_Pause_ArmsOnce pins that a SECOND paused chunk on an
// already-paused side does NOT replace the watchdog handle.
//
// WHY IT MATTERS: beginDecodePause bumps the generation and installs a FRESH
// time.AfterFunc. Re-arming on every paused chunk therefore pushes the
// deadline out by a full watchdog window each time — a guest that pauses on
// each chunk would never be bounded at all, which is the very leak S1 exists
// to close. The generation bump is the second cost: it supersedes the closure
// that was going to do the bounding.
//
// ⚠️ SCOPE, STATED HONESTLY: this drives f.DecodeData / f.EncodeData
// DIRECTLY, without a chain. Through HCM's single-goroutine dispatch the
// chain parks on the first paused chunk, so the second chunk cannot arrive
// until a resume has already cleared the flag. The reachable shapes are the
// chain-reentry case body.go's own sticky-cap comment anticipates and the
// decoderCb == nil case, where beginDecodePause leaves the flag set with NO
// timer at all. It is a cheap invariant guarding an expensive failure; it is
// not claimed to be live today.
func TestFilter_Data_Pause_ArmsOnce(t *testing.T) {
	t.Parallel()

	t.Run("decode", func(t *testing.T) {
		wf, _, _ := p83BodyFilter(t, buildPauseRequestBodyProxyWasm(), "p83_armonce_decode", proxyOnRequestBody)
		wf.pauseWatchdog = time.Hour
		t.Cleanup(wf.OnDestroy)

		if got := wf.DecodeData([]byte("a"), false); got != envoyhttp.DataStopIterationAndBuffer {
			t.Fatalf("DecodeData(chunk 1) = %v; want DataStopIterationAndBuffer", got)
		}
		if !wf.decodePaused.Load() {
			t.Errorf("decodePaused = false after the body Pause arm; want true (the filter owes the chain exactly one resume)")
		}
		wf.pauseMu.Lock()
		first := wf.decodePauseTimer
		wf.pauseMu.Unlock()
		if first == nil {
			t.Fatalf("decodePauseTimer = nil after the body Pause arm; want an armed watchdog")
		}
		genAfterFirst := wf.decodePauseGen.Load()

		if got := wf.DecodeData([]byte("b"), false); got != envoyhttp.DataStopIterationAndBuffer {
			t.Fatalf("DecodeData(chunk 2) = %v; want DataStopIterationAndBuffer", got)
		}
		wf.pauseMu.Lock()
		second := wf.decodePauseTimer
		wf.pauseMu.Unlock()
		if second != first {
			t.Errorf("decodePauseTimer moved %p → %p across two paused chunks; want the SAME handle (re-arming pushes the watchdog deadline out by a full window on every chunk, so a guest that pauses per-chunk is never bounded)", first, second)
		}
		if got := wf.decodePauseGen.Load(); got != genAfterFirst {
			t.Errorf("decodePauseGen %d → %d across two paused chunks; want it UNCHANGED (a bump supersedes the closure that was going to do the bounding)", genAfterFirst, got)
		}
	})

	t.Run("encode", func(t *testing.T) {
		wf, chain, _ := p83BodyFilter(t, buildPauseResponseBodyProxyWasm(), "p83_armonce_encode", proxyOnResponseBody)
		wf.pauseWatchdog = time.Hour
		t.Cleanup(wf.OnDestroy)

		respHeaders := http.Header{}
		respHeaders.Set("Content-Type", "text/plain")
		if _, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false); err != nil {
			t.Fatalf("RunEncodeHeaders: %v", err)
		}

		if got := wf.EncodeData([]byte("a"), false); got != envoyhttp.DataStopIterationAndBuffer {
			t.Fatalf("EncodeData(chunk 1) = %v; want DataStopIterationAndBuffer", got)
		}
		if !wf.encodePaused.Load() {
			t.Errorf("encodePaused = false after the body Pause arm; want true")
		}
		wf.pauseMu.Lock()
		first := wf.encodePauseTimer
		wf.pauseMu.Unlock()
		if first == nil {
			t.Fatalf("encodePauseTimer = nil after the body Pause arm; want an armed watchdog")
		}
		genAfterFirst := wf.encodePauseGen.Load()

		if got := wf.EncodeData([]byte("b"), false); got != envoyhttp.DataStopIterationAndBuffer {
			t.Fatalf("EncodeData(chunk 2) = %v; want DataStopIterationAndBuffer", got)
		}
		wf.pauseMu.Lock()
		second := wf.encodePauseTimer
		wf.pauseMu.Unlock()
		if second != first {
			t.Errorf("encodePauseTimer moved %p → %p across two paused chunks; want the SAME handle", first, second)
		}
		if got := wf.encodePauseGen.Load(); got != genAfterFirst {
			t.Errorf("encodePauseGen %d → %d across two paused chunks; want it UNCHANGED", genAfterFirst, got)
		}
	})
}

// -----------------------------------------------------------------------------
// CONTINUE-PATH DISARM — isolating anchors for disarmDecodePause /
// disarmEncodePause.
// -----------------------------------------------------------------------------

// TestFilter_Data_ContinueDisarmsPause pins that a chunk on which the guest
// RETURNS Continue releases any pause the previous chunk left armed.
//
// The Continue arm returns DataContinue, which RELEASES the chain — iteration
// advances to the next filter. Leaving decodePaused true and the watchdog
// armed past that point is a live spurious-resume source: the watchdog fires
// into a stream that is no longer parked and latches an unmatched token on the
// chain's buffered-1 resume channel, which then un-parks some OTHER filter.
//
// The fixture returns `1 - end_of_stream`, so chunk 1 (endStream=false) is the
// Pause and chunk 2 (endStream=true) is the Continue — one guest, both arms,
// which is why a transition is observable at all.
func TestFilter_Data_ContinueDisarmsPause(t *testing.T) {
	t.Parallel()

	t.Run("decode", func(t *testing.T) {
		wf, _, _ := p83BodyFilter(t, buildPauseRequestBodyProxyWasm(), "p83_disarm_decode", proxyOnRequestBody)
		wf.pauseWatchdog = time.Hour
		t.Cleanup(wf.OnDestroy)

		if got := wf.DecodeData([]byte("a"), false); got != envoyhttp.DataStopIterationAndBuffer {
			t.Fatalf("DecodeData(chunk 1) = %v; want DataStopIterationAndBuffer", got)
		}
		if !wf.decodePaused.Load() {
			t.Fatalf("decodePaused = false after the Pause chunk; the disarm below would be vacuous")
		}

		if got := wf.DecodeData([]byte("b"), true); got != envoyhttp.DataContinue {
			t.Fatalf("DecodeData(chunk 2, endStream) = %v; want DataContinue", got)
		}
		if wf.decodePaused.Load() {
			t.Errorf("disarm flag: decodePaused = true after the guest Continued; want false (the DataContinue return RELEASES the chain, so a still-armed pause fires an UNMATCHED ContinueDecoding into an unparked stream)")
		}
		wf.pauseMu.Lock()
		handle := wf.decodePauseTimer
		wf.pauseMu.Unlock()
		if handle != nil {
			t.Errorf("disarm handle: decodePauseTimer = %p after the guest Continued; want nil", handle)
		}
	})

	t.Run("encode", func(t *testing.T) {
		wf, chain, _ := p83BodyFilter(t, buildPauseResponseBodyProxyWasm(), "p83_disarm_encode", proxyOnResponseBody)
		wf.pauseWatchdog = time.Hour
		t.Cleanup(wf.OnDestroy)

		respHeaders := http.Header{}
		respHeaders.Set("Content-Type", "text/plain")
		if _, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false); err != nil {
			t.Fatalf("RunEncodeHeaders: %v", err)
		}

		if got := wf.EncodeData([]byte("a"), false); got != envoyhttp.DataStopIterationAndBuffer {
			t.Fatalf("EncodeData(chunk 1) = %v; want DataStopIterationAndBuffer", got)
		}
		if !wf.encodePaused.Load() {
			t.Fatalf("encodePaused = false after the Pause chunk; the disarm below would be vacuous")
		}

		if got := wf.EncodeData([]byte("b"), true); got != envoyhttp.DataContinue {
			t.Fatalf("EncodeData(chunk 2, endStream) = %v; want DataContinue", got)
		}
		if wf.encodePaused.Load() {
			t.Errorf("disarm flag: encodePaused = true after the guest Continued; want false")
		}
		wf.pauseMu.Lock()
		handle := wf.encodePauseTimer
		wf.pauseMu.Unlock()
		if handle != nil {
			t.Errorf("disarm handle: encodePauseTimer = %p after the guest Continued; want nil", handle)
		}
	})
}

// TestFilter_DecodeData_FailOpen_DisarmsPause covers the arm the Continue
// fixture CANNOT reach: `err != nil`.
//
// body.go fail-OPENs on a guest error per ADR-0072 and returns DataContinue —
// the SAME chain-releasing status as the Continue arm — so it owes the SAME
// disarm. A Continue-returning guest can never exercise it, so without the
// pause-then-trap fixture this arm would ship on reasoning alone.
//
// The envoy_go.failures delta is the DISCRIMINATOR: it is what proves the
// fail-open arm ran rather than the Continue arm. Without it this test is
// indistinguishable from the Continue-path one above.
func TestFilter_DecodeData_FailOpen_DisarmsPause(t *testing.T) {
	t.Parallel()
	wf, _, reg := p83BodyFilter(t, buildPauseThenTrapRequestBodyProxyWasm(), "p83_failopen_decode", proxyOnRequestBody)
	wf.pauseWatchdog = time.Hour
	t.Cleanup(wf.OnDestroy)

	// A nil *stats.Counter .Inc() is an unrecovered process crash, and the
	// fail-open arm increments one. Assert the pointer before driving it.
	if wf.cfg == nil || wf.cfg.stats == nil || wf.cfg.stats.envoyGoFailures == nil {
		t.Fatalf("cfg.stats.envoyGoFailures is nil; the fail-open arm's .Inc() would crash the process, not fail the test")
	}
	const failuresStat = "wasm.p83_failopen_decode.envoy_go.failures"
	before := findStatCounterValue(reg, failuresStat)

	if got := wf.DecodeData([]byte("a"), false); got != envoyhttp.DataStopIterationAndBuffer {
		t.Fatalf("DecodeData(chunk 1) = %v; want DataStopIterationAndBuffer", got)
	}
	if !wf.decodePaused.Load() {
		t.Fatalf("decodePaused = false after the Pause chunk; the disarm below would be vacuous")
	}

	// endStream=true makes the guest TRAP; the host fail-OPENs to DataContinue.
	if got := wf.DecodeData([]byte("b"), true); got != envoyhttp.DataContinue {
		t.Fatalf("DecodeData(chunk 2, trapping) = %v; want DataContinue (the ADR-0072 fail-open disposition)", got)
	}
	if delta := findStatCounterValue(reg, failuresStat) - before; delta != 1 {
		t.Errorf("%s delta = %d; want 1 — WITHOUT this the test cannot tell the FAIL-OPEN arm from the Continue arm and would pass on either", failuresStat, delta)
	}
	if wf.decodePaused.Load() {
		t.Errorf("disarm flag: decodePaused = true after the FAIL-OPEN arm returned DataContinue; want false (fail-open releases the chain exactly like Continue does and owes the same disarm)")
	}
	wf.pauseMu.Lock()
	handle := wf.decodePauseTimer
	wf.pauseMu.Unlock()
	if handle != nil {
		t.Errorf("disarm handle: decodePauseTimer = %p after the FAIL-OPEN arm; want nil", handle)
	}
}
