package wasm

// pause_test.go — phase-82 Tasks 9 / 11 / 12 and the Task-7 (S9) leak
// regression, for the honored-Pause stream control landed in pause.go.
//
// Test inventory:
//
//	T9  TestFilter_EncodeHeaders_Pause_StopsIteration
//	    TestFilter_Pause_CensusOfHonoredArms                (documents the
//	                                                         four pre-existing
//	                                                         honored arms)
//	T11 TestAbiCallbacks_ContinueStream_SixArms             (counting cbs)
//	    TestAbiCallbacks_ContinueStream_NoSpuriousCrossFilterResume
//	T12 TestFilter_PauseFlag_CrossGoroutineResume_NoRace
//	T7  TestFilter_DecodeHeaders_Pause_WatchdogUnparksChain
//	    TestFilter_OnDestroy_DisarmsPauseWatchdogs
//
// The decode-side T9 anchor lives in dispatch_test.go
// (TestFilter_DecodeHeaders_Pause_StopsIteration) — it is the flipped form of
// the pre-phase-82 test that asserted the log-and-continue behavior.

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// -----------------------------------------------------------------------------
// Counting framework-callback doubles. No counting DecoderFilterCallbacks
// existed in this package before phase-82 — every double swallowed
// ContinueDecoding silently, which is why nothing asserted that ContinueStream
// actually resumes anything.
// -----------------------------------------------------------------------------

type countingDecoderCb struct {
	fakeDecoderCb
	continues atomic.Int32
}

func (c *countingDecoderCb) ContinueDecoding() { c.continues.Add(1) }

type countingEncoderCb struct {
	fakeEncoderCb
	continues atomic.Int32
}

func (c *countingEncoderCb) ContinueEncoding() { c.continues.Add(1) }

var (
	_ envoyhttp.DecoderFilterCallbacks = (*countingDecoderCb)(nil)
	_ envoyhttp.EncoderFilterCallbacks = (*countingEncoderCb)(nil)
)

// -----------------------------------------------------------------------------
// T9 — the encode-side pause arm.
// -----------------------------------------------------------------------------

// TestFilter_EncodeHeaders_Pause_StopsIteration exercises the ENCODE half of
// S1 against buildPauseResponseProxyWasm.
//
// UNEXERCISED IN THE DIFFERENTIAL: zero of the 35 guest crates in this
// repository return Action::Pause from proxy_on_response_headers, so this
// synthetic fixture is the arm's ONLY exercise. Do not read this as
// differential coverage.
//
// The executions counter is the liveness barrier: it is bumped on the
// decode-side dispatch, so a 0 there means the capability gate short-circuited
// and neither headers callback ran at all (measured at phase-82 Task 8: the
// package's dispatch tests were cap-denied at 123 of 129 gate sites).
func TestFilter_EncodeHeaders_Pause_StopsIteration(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfigWithCaps(t, buildPauseResponseProxyWasm(), "plugin_pause_resp", reg)
	t.Cleanup(func() { _ = cc.Close() })

	ecb := &countingEncoderCb{}
	f := &filter{cfg: cc}
	f.pauseWatchdog = time.Hour // never fires within the test
	f.SetDecoderCallbacks(fakeDecoderCb{})
	f.SetEncoderCallbacks(ecb)
	t.Cleanup(f.OnDestroy)

	reqHeaders := http.Header{}
	reqHeaders.Set(":method", "GET")
	if got := f.DecodeHeaders(reqHeaders, true); got != envoyhttp.Continue {
		t.Errorf("DecodeHeaders = %v; want Continue (the fixture's request-headers callback returns ProxyActionContinue)", got)
	}
	if got := findStatCounterValue(reg, "wasm.plugin_pause_resp.executions"); got != 1 {
		t.Errorf("executions = %d; want 1 (a 0 means the capability gate short-circuited and NEITHER headers arm was reached)", got)
	}
	if f.decodePaused.Load() {
		t.Errorf("decodePaused = true after a Continue-returning request-headers callback; want false")
	}

	respHeaders := http.Header{}
	respHeaders.Set("Content-Type", "text/plain")
	if got := f.EncodeHeaders(respHeaders, true); got != envoyhttp.StopIteration {
		t.Errorf("EncodeHeaders = %v; want StopIteration (phase-82 S1 honors Action::Pause on response headers)", got)
	}
	if !f.encodePaused.Load() {
		t.Errorf("encodePaused = false after the PAUSE arm; want true (the filter owes the chain one resume)")
	}
	// The pause itself must NOT have signaled a resume.
	if got := ecb.continues.Load(); got != 0 {
		t.Errorf("ContinueEncoding calls after the pause = %d; want 0", got)
	}
	// And the decode side must be untouched — the two flags are independent.
	if f.decodePaused.Load() {
		t.Errorf("decodePaused = true after an ENCODE-side pause; want false")
	}
}

// TestFilter_Pause_CensusOfHonoredArms pins the phase-82 finding that FOUR of
// the SIX production abi.ProxyActionPause arms already honored Pause before
// this row, so the blanket "stream control is deferred" claim was false. The
// four are body.go x2 (DataStopIterationAndBuffer) and trailers.go x2
// (TrailersStopIteration); this row moved only the two headers arms.
//
// This is a behavioral assertion, not a grep: it drives the real dispatch and
// reads the returned status. It is also the guard that keeps a future edit from
// silently changing the frozen body/trailers dispositions — differential
// fixture 0036 scenario (n) depends on the decode-body arm pausing.
func TestFilter_Pause_CensusOfHonoredArms(t *testing.T) {
	t.Parallel()
	// Statuses the four frozen arms return. Asserting the CONSTANTS keeps this
	// cheap and makes an accidental change to the returned disposition a
	// compile-or-assert failure at this site rather than a differential red.
	if envoyhttp.DataStopIterationAndBuffer == envoyhttp.DataContinue {
		t.Errorf("DataStopIterationAndBuffer == DataContinue; the body pause arms would be no-ops")
	}
	if envoyhttp.TrailersStopIteration == envoyhttp.TrailersContinue {
		t.Errorf("TrailersStopIteration == TrailersContinue; the trailers pause arms would be no-ops")
	}
	if envoyhttp.StopIteration == envoyhttp.Continue {
		t.Errorf("StopIteration == Continue; the headers pause arms would be no-ops")
	}
}

// -----------------------------------------------------------------------------
// T11 — ContinueStream, all six arms.
// -----------------------------------------------------------------------------

// TestAbiCallbacks_ContinueStream_SixArms covers the full ContinueStream
// surface. Before phase-82 only the nil-callback and BadArgument arms were
// covered and NOTHING asserted that ContinueDecoding / ContinueEncoding
// actually fire — the package had no counting DecoderFilterCallbacks at all.
//
// The six arms are decode/encode x {nil callback, live-but-not-paused,
// live-and-paused}, plus the idempotence follow-up on the paused arms: a
// SECOND ContinueStream after a completed resume must NOT latch a spurious
// token on the chain-scoped resume channel.
func TestAbiCallbacks_ContinueStream_SixArms(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("decode_nil_callback", func(t *testing.T) {
		t.Parallel()
		cb, _ := newTestABICallbacks(nil, nil, nil, fakeEncoderCb{})
		if got := cb.ContinueStream(ctx, 1, 0); got != abi.WasmResultInternalFailure {
			t.Errorf("ContinueStream(decode, nil decoderCb) = %v; want InternalFailure", got)
		}
	})

	t.Run("decode_live_not_paused", func(t *testing.T) {
		t.Parallel()
		dcb := &countingDecoderCb{}
		cb, f := newTestABICallbacks(nil, nil, dcb, nil)
		if got := cb.ContinueStream(ctx, 1, 0); got != abi.WasmResultOk {
			t.Errorf("ContinueStream(decode, not paused) = %v; want Ok", got)
		}
		if got := dcb.continues.Load(); got != 0 {
			t.Errorf("ContinueDecoding calls = %d; want 0 — an unmatched resume latches a token on the CHAIN-scoped decodeResumeCh and spuriously un-parks a sibling filter", got)
		}
		if f.decodePaused.Load() {
			t.Errorf("decodePaused = true; want false")
		}
	})

	t.Run("decode_live_paused_then_idempotent", func(t *testing.T) {
		t.Parallel()
		dcb := &countingDecoderCb{}
		cb, f := newTestABICallbacks(nil, nil, dcb, nil)
		f.decodePaused.Store(true)

		if got := cb.ContinueStream(ctx, 1, 0); got != abi.WasmResultOk {
			t.Errorf("first ContinueStream(decode, paused) = %v; want Ok", got)
		}
		if got := dcb.continues.Load(); got != 1 {
			t.Errorf("ContinueDecoding calls after the first resume = %d; want exactly 1", got)
		}
		if f.decodePaused.Load() {
			t.Errorf("decodePaused = true after a completed resume; want false (the CAS consumed it)")
		}
		// Idempotence: the guest calls proxy_continue_stream a second time.
		if got := cb.ContinueStream(ctx, 1, 0); got != abi.WasmResultOk {
			t.Errorf("second ContinueStream(decode) = %v; want Ok (a redundant resume is not an error at the ABI)", got)
		}
		if got := dcb.continues.Load(); got != 1 {
			t.Errorf("ContinueDecoding calls after the SECOND resume = %d; want still 1 — the second call must not latch a spurious token", got)
		}
	})

	t.Run("encode_nil_callback", func(t *testing.T) {
		t.Parallel()
		cb, _ := newTestABICallbacks(nil, nil, fakeDecoderCb{}, nil)
		if got := cb.ContinueStream(ctx, 1, 1); got != abi.WasmResultInternalFailure {
			t.Errorf("ContinueStream(encode, nil encoderCb) = %v; want InternalFailure", got)
		}
	})

	t.Run("encode_live_not_paused", func(t *testing.T) {
		t.Parallel()
		ecb := &countingEncoderCb{}
		cb, f := newTestABICallbacks(nil, nil, nil, ecb)
		if got := cb.ContinueStream(ctx, 1, 1); got != abi.WasmResultOk {
			t.Errorf("ContinueStream(encode, not paused) = %v; want Ok", got)
		}
		if got := ecb.continues.Load(); got != 0 {
			t.Errorf("ContinueEncoding calls = %d; want 0 (unmatched resume would latch on the chain-scoped encodeResumeCh)", got)
		}
		if f.encodePaused.Load() {
			t.Errorf("encodePaused = true; want false")
		}
	})

	t.Run("encode_live_paused_then_idempotent", func(t *testing.T) {
		t.Parallel()
		ecb := &countingEncoderCb{}
		cb, f := newTestABICallbacks(nil, nil, nil, ecb)
		f.encodePaused.Store(true)

		if got := cb.ContinueStream(ctx, 1, 1); got != abi.WasmResultOk {
			t.Errorf("first ContinueStream(encode, paused) = %v; want Ok", got)
		}
		if got := ecb.continues.Load(); got != 1 {
			t.Errorf("ContinueEncoding calls after the first resume = %d; want exactly 1", got)
		}
		if f.encodePaused.Load() {
			t.Errorf("encodePaused = true after a completed resume; want false")
		}
		if got := cb.ContinueStream(ctx, 1, 1); got != abi.WasmResultOk {
			t.Errorf("second ContinueStream(encode) = %v; want Ok", got)
		}
		if got := ecb.continues.Load(); got != 1 {
			t.Errorf("ContinueEncoding calls after the SECOND resume = %d; want still 1", got)
		}
	})

	t.Run("bad_stream_type_and_nil_filter", func(t *testing.T) {
		t.Parallel()
		dcb := &countingDecoderCb{}
		cb, _ := newTestABICallbacks(nil, nil, dcb, fakeEncoderCb{})
		for _, st := range []uint32{2, 3, 99} {
			if got := cb.ContinueStream(ctx, 1, st); got != abi.WasmResultBadArgument {
				t.Errorf("ContinueStream(streamType=%d) = %v; want BadArgument", st, got)
			}
		}
		if got := dcb.continues.Load(); got != 0 {
			t.Errorf("ContinueDecoding calls from BadArgument arms = %d; want 0", got)
		}
		nilCB := &abiCallbacks{filter: nil}
		if got := nilCB.ContinueStream(ctx, 1, 0); got != abi.WasmResultInternalFailure {
			t.Errorf("ContinueStream(nil filter) = %v; want InternalFailure", got)
		}
	})
}

// -----------------------------------------------------------------------------
// T11 — the cross-filter spurious-resume property, at the CHAIN.
// -----------------------------------------------------------------------------

// parkingProbeFilter is a minimal decode-side filter that parks the chain on
// its first DecodeHeaders and records how many times it has been entered. It
// stands in for the real sibling parkers (extauthz / ratelimit / oauth2 /
// bandwidthlimit) that share the chain-scoped decodeResumeCh.
type parkingProbeFilter struct {
	cb      envoyhttp.DecoderFilterCallbacks
	entered atomic.Int32
	resumed chan struct{} // closed by the test when it wants the probe released
}

func (p *parkingProbeFilter) DecodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus {
	p.entered.Add(1)
	return envoyhttp.StopIteration
}
func (p *parkingProbeFilter) DecodeData([]byte, bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}
func (p *parkingProbeFilter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	p.entered.Add(1)
	return envoyhttp.TrailersStopIteration
}
func (p *parkingProbeFilter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { p.cb = cb }
func (p *parkingProbeFilter) OnDestroy()                                              {}

var _ envoyhttp.StreamDecoderFilter = (*parkingProbeFilter)(nil)

// TestAbiCallbacks_ContinueStream_NoSpuriousCrossFilterResume is the
// discriminating proof for the CompareAndSwap gate, run through a REAL
// FilterChain rather than against the flag in isolation.
//
// Shape: chain = [wasm(pause guest), parkingProbe]. The wasm filter parks the
// chain; a simulated guest resume advances iteration to the probe, which parks
// again on the SAME chain-scoped decodeResumeCh. The test then issues a SECOND
// proxy_continue_stream. With the gate, that call fires nothing and the probe
// stays parked. WITHOUT the gate (unconditional decoderCb.ContinueDecoding),
// the second call latches a token on the buffered-1 channel and un-parks the
// probe — a filter that never asked to be resumed.
//
// The "still parked" assertion also proves the buffered-1 channel does not
// lose an early resume: the FIRST resume below is issued while the chain is
// demonstrably parked, and the probe's entered-count moving from 1 to 2 is what
// shows the wakeup was delivered.
func TestAbiCallbacks_ContinueStream_NoSpuriousCrossFilterResume(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfigWithCaps(t, buildPauseProxyWasm(), "plugin_pause_chain", reg)
	t.Cleanup(func() { _ = cc.Close() })

	wf := &filter{cfg: cc}
	wf.pauseWatchdog = time.Hour // the watchdog must not be what resumes us here
	probe := &parkingProbeFilter{resumed: make(chan struct{})}

	chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
		{Name: "wasm", Decoder: wf, Encoder: wf},
		{Name: "probe", Decoder: probe},
	}, nil)
	t.Cleanup(chain.Destroy)

	done := make(chan struct{})
	go func() {
		defer close(done)
		h := http.Header{}
		h.Set(":method", "GET")
		_, _ = chain.RunDecodeHeaders(context.Background(), h, true)
	}()

	// Wait for the wasm filter to park.
	if !waitFor(t, 5*time.Second, func() bool { return wf.decodePaused.Load() }) {
		t.Fatalf("wasm filter never set decodePaused; the PAUSE arm was not reached")
	}
	if got := probe.entered.Load(); got != 0 {
		t.Errorf("probe entered = %d before the resume; want 0 (the chain must be parked at the wasm filter)", got)
	}

	// Simulated guest resume #1 — the one the wasm filter actually owes.
	acb := &abiCallbacks{filter: wf}
	if got := acb.ContinueStream(context.Background(), 1, 0); got != abi.WasmResultOk {
		t.Errorf("ContinueStream #1 = %v; want Ok", got)
	}
	if !waitFor(t, 5*time.Second, func() bool { return probe.entered.Load() == 1 }) {
		t.Fatalf("probe never entered after the owed resume (entered=%d); the resume was not delivered", probe.entered.Load())
	}

	// The probe has now parked on the SAME chain-scoped channel. Resume #2 is
	// the spurious one: the wasm filter owes nothing.
	if got := acb.ContinueStream(context.Background(), 1, 0); got != abi.WasmResultOk {
		t.Errorf("ContinueStream #2 = %v; want Ok", got)
	}

	// Settle, then assert the probe is STILL parked. Without the CAS gate the
	// latched token releases it and RunDecodeHeaders completes here.
	select {
	case <-done:
		t.Errorf("RunDecodeHeaders completed after a SPURIOUS ContinueStream — the unmatched resume latched a token on the chain-scoped decodeResumeCh and un-parked the sibling filter")
	case <-time.After(300 * time.Millisecond):
		// still parked: correct
	}

	// Release the probe through its OWN callback so the goroutine exits.
	probe.cb.ContinueDecoding()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Errorf("RunDecodeHeaders did not complete after the probe's own ContinueDecoding")
	}
}

// TestFilterChain_EarlyResumeIsLatchedNotLost VERIFIES BY EXECUTION the
// premise the CAS gate rests on: FilterChain.decodeResumeCh is buffered-1 with
// a non-blocking send, so a resume that arrives BEFORE the chain parks is
// LATCHED rather than lost — there is no lost-wakeup race to defend against,
// only a spurious-token one. (That property had been established by code-read
// only; this drives it.)
//
// Shape: fire ContinueDecoding while nothing is parked, THEN start iteration
// with a filter that parks. The latched token releases the park immediately.
// If the send were dropped instead of buffered, RunDecodeHeaders would hang.
func TestFilterChain_EarlyResumeIsLatchedNotLost(t *testing.T) {
	t.Parallel()
	probe := &parkingProbeFilter{resumed: make(chan struct{})}
	chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
		{Name: "probe", Decoder: probe},
	}, nil)
	t.Cleanup(chain.Destroy)

	// Resume BEFORE anything parks.
	probe.cb.ContinueDecoding()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	}()

	select {
	case <-done:
		// The early token was latched and consumed by the park. Correct.
	case <-time.After(3 * time.Second):
		t.Errorf("RunDecodeHeaders never returned: an early ContinueDecoding was LOST rather than latched on the buffered-1 decodeResumeCh — the CAS gate's premise does not hold")
		probe.cb.ContinueDecoding() // unblock the goroutine so the suite finishes
		<-done
	}

	// And the buffer really is depth 1 (coalescing), not deeper: TWO sends
	// followed by TWO parks must leave the chain parked on the second, because
	// the non-blocking send drops the surplus token.
	//
	// RunDecodeTrailers, not RunDecodeHeaders, for the repeat: only the Data
	// and Trailers entry points reset c.decodeIdx: a second RunDecodeHeaders
	// finds the cursor already past len(filters) and returns without iterating
	// (measured — that is what made the first draft of this assertion vacuous).
	probe.cb.ContinueDecoding()
	probe.cb.ContinueDecoding()
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		_, _ = chain.RunDecodeTrailers(context.Background(), http.Header{})
		_, _ = chain.RunDecodeTrailers(context.Background(), http.Header{})
	}()
	select {
	case <-done2:
		t.Errorf("two ContinueDecoding sends satisfied TWO parks — decodeResumeCh is deeper than the buffered-1 the coalescing discipline assumes")
	case <-time.After(300 * time.Millisecond):
		// still parked on the second park: buffer depth is 1 (coalesced)
	}
	probe.cb.ContinueDecoding()
	select {
	case <-done2:
	case <-time.After(3 * time.Second):
		t.Errorf("second park never released")
	}
}

// waitFor polls cond until it holds or the budget expires. Poll-the-predicate
// rather than sleep-a-guess.
func waitFor(t *testing.T, budget time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return cond()
}

// -----------------------------------------------------------------------------
// T12 — the cross-goroutine -race regression.
// -----------------------------------------------------------------------------

// TestFilter_PauseFlag_CrossGoroutineResume_NoRace is the regression test for
// the atomicity requirement on decodePaused / encodePaused.
//
// S1 BREAKS the ADR-0071 single-goroutine-per-stream invariant: the flag is
// written by the HCM dispatch goroutine inside DecodeHeaders and read+cleared
// by the http-call dispatch goroutine, which reaches ContinueStream through
// RootVM.handleHttpCallResponse -> proxy_on_http_call_response ->
// proxy_continue_stream. There is no happens-before edge between the two —
// dispatchMu is released before DecodeHeaders' switch runs.
//
// NEGATIVE CONTROL (retained as the discriminator; a -race test that passes
// with the bug present is worthless): with `decodePaused bool` instead of
// `decodePaused atomic.Bool`, this test reports
//
//	WARNING: DATA RACE
//	  Read at ... by goroutine N:   wasm.(*abiCallbacks).ContinueStream()
//	  Previous write at ... by goroutine M: wasm.(*filter).DecodeHeaders()
//
// The standing suites do NOT catch it — a prior stage measured that a plain
// field passes -race under the existing tests, because nothing there reads the
// flag from a second goroutine. The cross-goroutine shape below is what
// discriminates.
func TestFilter_PauseFlag_CrossGoroutineResume_NoRace(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfigWithCaps(t, buildPauseProxyWasm(), "plugin_pause_race", reg)
	t.Cleanup(func() { _ = cc.Close() })

	const iterations = 40
	for i := 0; i < iterations; i++ {
		f := &filter{cfg: cc}
		f.pauseWatchdog = time.Hour
		dcb := &countingDecoderCb{}
		f.SetDecoderCallbacks(dcb)
		f.SetEncoderCallbacks(&countingEncoderCb{})
		acb := &abiCallbacks{filter: f}

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		// Goroutine A: the HCM dispatch goroutine writing the flag.
		go func() {
			defer wg.Done()
			<-start
			h := http.Header{}
			h.Set(":method", "GET")
			_ = f.DecodeHeaders(h, true)
		}()
		// Goroutine B: the http-call dispatch goroutine reading+clearing it.
		// The only synchronization between A and B is the shared `start`
		// close, which orders each goroutine after the test but NOT relative
		// to the other.
		go func() {
			defer wg.Done()
			<-start
			_ = acb.ContinueStream(context.Background(), 1, 0)
		}()
		close(start)
		wg.Wait()

		// Whichever order won, the flag must end up in a consistent state:
		// either B saw the pause and consumed it (paused=false, 1 continue) or
		// B ran first and the pause is still outstanding (paused=true, 0
		// continues). Never "paused and also resumed".
		paused := f.decodePaused.Load()
		continues := dcb.continues.Load()
		if paused && continues != 0 {
			t.Errorf("iter %d: decodePaused=true with %d ContinueDecoding calls; the flag and the resume disagree", i, continues)
		}
		if continues > 1 {
			t.Errorf("iter %d: %d ContinueDecoding calls from a single ContinueStream; want at most 1", i, continues)
		}
		f.OnDestroy()
	}
}

// -----------------------------------------------------------------------------
// T7 (S9) — the parked-stream leak.
// -----------------------------------------------------------------------------

// TestFilter_DecodeHeaders_Pause_WatchdogUnparksChain is the S9 regression.
//
// THE LEAK: FilterChain.parkDecode waits on the chain-scoped resume channel OR
// ctx.Done(). The ctx HCM dispatch hands it is the CONNECTION context, and
// nothing cancels it when the downstream peer disconnects — the H1 read loop
// only notices on the next http.ReadRequest, which cannot run because the
// dispatch goroutine is parked inside dispatchRequest. Honoring Pause without
// a bound therefore leaks one connection + one goroutine per request: measured
// end-to-end against a standalone subject, `curl -m 10` returned zero bytes at
// 10.007 s (exit 28) and 30+ s later downstream_cx_active was still 1 and
// wasm_wazero_active was still 1, with no OnDestroy and no downstream_rq_xx.
//
// FAILING-FIRST: with the watchdog removed, RunDecodeHeaders below never
// returns and this test fails on the budget (it does not hang the suite — the
// wait is bounded and reported with t.Errorf).
//
// LAYER: the bound lives on the WASM FILTER, not on parkDecode. Every other
// parking filter (extauthz / ratelimit / oauth2 / bandwidthlimit) parks behind
// its own bounded RPC and already has a guaranteed resumer; a wasm guest has
// none. Bounding parkDecode itself would change all of their semantics.
func TestFilter_DecodeHeaders_Pause_WatchdogUnparksChain(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfigWithCaps(t, buildPauseProxyWasm(), "plugin_pause_leak", reg)
	t.Cleanup(func() { _ = cc.Close() })

	wf := &filter{cfg: cc}
	wf.pauseWatchdog = 250 * time.Millisecond

	chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
		{Name: "wasm", Decoder: wf, Encoder: wf},
	}, nil)
	t.Cleanup(chain.Destroy)

	done := make(chan struct{})
	go func() {
		defer close(done)
		h := http.Header{}
		h.Set(":method", "GET")
		_, _ = chain.RunDecodeHeaders(context.Background(), h, true)
	}()

	if !waitFor(t, 5*time.Second, func() bool { return wf.decodePaused.Load() }) {
		t.Fatalf("the PAUSE arm was never reached (decodePaused stayed false)")
	}

	// The park must NOT be released before the watchdog window elapses.
	select {
	case <-done:
		t.Errorf("RunDecodeHeaders returned before the watchdog window — the pause was not honored at all")
	case <-time.After(50 * time.Millisecond):
	}

	// ...and it MUST be released by the watchdog. An unbounded park is the leak.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Errorf("RunDecodeHeaders never returned: the guest paused and never resumed, and nothing bounded the park — this is the S9 connection+goroutine leak (one per request)")
	}

	if wf.decodePaused.Load() {
		t.Errorf("decodePaused = true after the watchdog fired; want false (the watchdog consumes the flag via the same CAS the guest would)")
	}
}

// TestFilter_OnDestroy_DisarmsPauseWatchdogs asserts the watchdog handles are
// released at stream teardown. A surviving timer keeps the chain's filter
// callbacks reachable through its closure for the remainder of the window,
// which is a (bounded) leak of its own.
func TestFilter_OnDestroy_DisarmsPauseWatchdogs(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfigWithCaps(t, buildPauseProxyWasm(), "plugin_pause_destroy", reg)
	t.Cleanup(func() { _ = cc.Close() })

	f := &filter{cfg: cc}
	f.pauseWatchdog = time.Hour
	dcb := &countingDecoderCb{}
	f.SetDecoderCallbacks(dcb)
	f.SetEncoderCallbacks(&countingEncoderCb{})

	h := http.Header{}
	h.Set(":method", "GET")
	if got := f.DecodeHeaders(h, true); got != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders = %v; want StopIteration", got)
	}
	f.pauseMu.Lock()
	armed := f.decodePauseTimer != nil
	f.pauseMu.Unlock()
	if !armed {
		t.Errorf("decodePauseTimer = nil after the PAUSE arm; want an armed watchdog")
	}

	f.OnDestroy()

	f.pauseMu.Lock()
	stillArmed := f.decodePauseTimer != nil || f.encodePauseTimer != nil
	f.pauseMu.Unlock()
	if stillArmed {
		t.Errorf("a pause watchdog survived OnDestroy; want both handles released")
	}
	// OnDestroy must be idempotent with the watchdog teardown in front of the
	// nil-streamCtx early return.
	f.OnDestroy()
}
