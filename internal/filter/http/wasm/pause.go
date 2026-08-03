package wasm

// pause.go — honored-Pause stream-control bookkeeping for the two HEADERS
// arms, per phase-82 S1 (honor Action::Pause) + S9 (bound the park).
//
// # Scope: the two HEADERS arms ONLY
//
// The repository has SIX production `abi.ProxyActionPause` arms. FOUR of them
// ALREADY honored Pause before phase-82 and are NOT touched here:
//
//	body.go:227      -> DataStopIterationAndBuffer   (decode body)
//	body.go:314      -> DataStopIterationAndBuffer   (encode body)
//	trailers.go:143  -> TrailersStopIteration        (decode trailers)
//	trailers.go:200  -> TrailersStopIteration        (encode trailers)
//
// The blanket claim "stream control is deferred" was therefore FALSE; only
// decode_headers.go and encode_headers.go were still logging-and-continuing.
// The four landed siblings carry ZERO paused-state bookkeeping, and they stay
// that way: differential fixture 0036 scenario (n) (n_body_cap_exceeded)
// depends on the decode-body arm pausing indefinitely by design so the host
// reaches body_buffer_cap_bytes. Those two files are frozen.
//
// CONSEQUENCE, stated plainly: a guest that pauses from a BODY or TRAILERS
// callback does not set decodePaused/encodePaused, so ContinueStream's CAS
// gate will not fire a resume for it. Measured at phase-82: ZERO of the 35
// guest crates in this repository reference `resume_http_request` /
// `resume_http_response` / `proxy_continue_stream`, so no guest can reach
// that path today. Extending the flag to the body/trailers arms is a
// follow-up that must land together with a re-run of fixture 0036 (n).
//
// # Why a watchdog exists at all (S9)
//
// FilterChain.parkDecode waits on the chain's resume channel OR ctx.Done().
// The ctx handed to it by HCM dispatch is the CONNECTION context, and nothing
// cancels it on downstream disconnect — the H1 read loop only notices the peer
// is gone on the NEXT http.ReadRequest, which never runs because the dispatch
// goroutine is parked inside dispatchRequest. Honoring Pause without a bound
// is therefore an UNBOUNDED connection + goroutine leak, one per request:
// measured with a standalone subject, the client gave up at 10 s and 30+ s
// later downstream_cx_active was still 1, wasm_wazero_active was still 1, and
// no OnDestroy had fired.
//
// A guest pause has no other guaranteed resumer: unlike extauthz / ratelimit /
// oauth2 / bandwidthlimit — each of which parks behind its own bounded RPC —
// nothing obliges a wasm guest to ever call proxy_continue_stream. The bound
// therefore belongs to the WASM FILTER, not to the chain: fixing parkDecode
// itself would change the park semantics of every other parking filter.
//
// A watchdog fire and a guest resume race; both go through the same
// CompareAndSwap, so exactly one of them signals the chain.

import "time"

// defaultPauseWatchdog bounds how long a guest-requested headers pause may
// hold the stream parked before the host force-resumes it.
//
// The value is pinned between two MEASURED constraints, not chosen by feel:
//
//	LOWER BOUND — internal/wasm.defaultHttpCallTimeout is 5 s, and that is the
//	deadline on the outbound call a paused guest is normally waiting for. A
//	watchdog at or below 5 s would truncate a legitimate pause. 10 s gives the
//	call its full deadline plus the same again for the callback to run.
//
//	UPPER BOUND — differential fixture 0036 scenario (l) (l_httpcall_success)
//	returns Action::Pause from on_http_request_headers and NEVER calls
//	resume_http_request from its on_http_call_response, so with Pause honored
//	its probe is released only by this watchdog. That fixture's driver
//	(test/fixtures/0036-.../inputs/driver.go) uses an http.Client with
//	Timeout: 15 * time.Second. A 15 s watchdog is a dead heat with that client
//	timeout; 10 s lets the probe complete with a real response instead.
//
// Scenario (l)'s wire stream is a constant token and its result is discarded
// (`_ = runScenarioGet(...)`), so its StatsAsserter arms — http_call_dispatched
// and http_call_response, both of which fire at dispatch time — hold either
// way. The cost of honoring Pause there is ~10 s of added subject-side runtime
// on that one fixture, not a verdict change.
const defaultPauseWatchdog = 10 * time.Second

// watchdogTimeout resolves the per-stream override (0 ⇒ default).
func (f *filter) watchdogTimeout() time.Duration {
	if f.pauseWatchdog > 0 {
		return f.pauseWatchdog
	}
	return defaultPauseWatchdog
}

// beginDecodePause marks the decode side parked and arms the S9 watchdog.
// Called from the DecodeHeaders ProxyActionPause arm immediately before it
// returns envoyhttp.StopIteration.
func (f *filter) beginDecodePause() {
	f.decodePaused.Store(true)
	cb := f.decoderCb
	if cb == nil {
		// No framework callback ⇒ nothing parks (test-double paths that
		// construct *filter{} directly). Leave the flag set so a later
		// ContinueStream still reports the transition; nothing to arm.
		return
	}
	t := time.AfterFunc(f.watchdogTimeout(), func() {
		if !f.decodePaused.CompareAndSwap(true, false) {
			return // already resumed by the guest; the token is not ours to fire
		}
		logf("WARN wasm: decode-side pause watchdog fired (stream=%d) — the guest returned PAUSE from proxy_on_request_headers and never called proxy_continue_stream; force-resuming the stream to avoid a parked-connection leak",
			f.streamContextID)
		cb.ContinueDecoding()
	})
	f.pauseMu.Lock()
	f.decodePauseTimer = t
	f.pauseMu.Unlock()
}

// beginEncodePause is the encode-side mirror of beginDecodePause.
//
// UNEXERCISED by any guest in this repository: zero of the 35 guest crates
// return Action::Pause from proxy_on_response_headers, so this half has NO
// differential coverage. It is exercised only by the synthetic unit fixture
// buildPauseResponseProxyWasm (wasm_fixtures_test.go).
func (f *filter) beginEncodePause() {
	f.encodePaused.Store(true)
	cb := f.encoderCb
	if cb == nil {
		return
	}
	t := time.AfterFunc(f.watchdogTimeout(), func() {
		if !f.encodePaused.CompareAndSwap(true, false) {
			return
		}
		logf("WARN wasm: encode-side pause watchdog fired (stream=%d) — the guest returned PAUSE from proxy_on_response_headers and never called proxy_continue_stream; force-resuming the stream to avoid a parked-connection leak",
			f.streamContextID)
		cb.ContinueEncoding()
	})
	f.pauseMu.Lock()
	f.encodePauseTimer = t
	f.pauseMu.Unlock()
}

// resumeDecode fires at most ONE ContinueDecoding for a pause this filter
// actually owes. Returns true when this call is the one that signaled the
// chain.
//
// The CompareAndSwap is the whole point: decodeResumeCh is chain-scoped and
// buffered-1, so an unmatched send LATCHES a token that spuriously un-parks a
// different filter later in the same stream. Conversely, because the channel
// IS buffered-1, a resume that arrives before the chain has parked is latched
// rather than lost — there is no lost-wakeup hazard.
func (f *filter) resumeDecode() bool {
	if !f.decodePaused.CompareAndSwap(true, false) {
		return false
	}
	f.stopPauseTimer(true)
	if cb := f.decoderCb; cb != nil {
		cb.ContinueDecoding()
	}
	return true
}

// resumeEncode is the encode-side mirror of resumeDecode.
func (f *filter) resumeEncode() bool {
	if !f.encodePaused.CompareAndSwap(true, false) {
		return false
	}
	f.stopPauseTimer(false)
	if cb := f.encoderCb; cb != nil {
		cb.ContinueEncoding()
	}
	return true
}

// stopPauseTimer disarms + releases the named side's watchdog handle. Safe to
// call when no timer was armed. A watchdog that has already fired is harmless
// regardless (its body re-checks the CAS), so Stop's return value is ignored.
func (f *filter) stopPauseTimer(decode bool) {
	f.pauseMu.Lock()
	var t *time.Timer
	if decode {
		t, f.decodePauseTimer = f.decodePauseTimer, nil
	} else {
		t, f.encodePauseTimer = f.encodePauseTimer, nil
	}
	f.pauseMu.Unlock()
	if t != nil {
		t.Stop()
	}
}

// stopPauseWatchdogs disarms both sides. Called from OnDestroy so a completed
// stream does not keep a pending timer (and, through its closure, the chain's
// filter callbacks) alive for the remainder of the watchdog window.
func (f *filter) stopPauseWatchdogs() {
	f.stopPauseTimer(true)
	f.stopPauseTimer(false)
}
