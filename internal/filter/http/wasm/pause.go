package wasm

// pause.go — honored-Pause stream-control bookkeeping, per phase-82 S1 (honor
// Action::Pause) + S9 (bound the park), extended to the two BODY arms at
// phase-83 S1 and the two TRAILERS arms at phase-83 S2.
//
// # Scope: ALL SIX production Pause arms
//
// The repository has SIX production `abi.ProxyActionPause` arms. As of
// phase-83 every one of them books the pause here before returning its
// parking status:
//
//	decode_headers.go -> StopIteration              (decode headers)  — S1, phase 82
//	encode_headers.go -> StopIteration              (encode headers)  — S1, phase 82
//	body.go           -> DataStopIterationAndBuffer (decode body)     — S1, phase 83
//	body.go           -> DataStopIterationAndBuffer (encode body)     — S1, phase 83
//	trailers.go       -> TrailersStopIteration      (decode trailers) — S2, phase 83
//	trailers.go       -> TrailersStopIteration      (encode trailers) — S2, phase 83
//
// ⚠️ THE PHASE-82 TEXT HERE WAS WRONG AND IS CORRECTED IN PLACE BY THE COMMITS
// THAT FALSIFIED IT. It said the four non-headers arms "carry ZERO paused-state
// bookkeeping, and they stay that way", because "differential fixture 0036
// scenario (n) (n_body_cap_exceeded) depends on the decode-body arm pausing
// indefinitely by design so the host reaches body_buffer_cap_bytes." That
// dependency is INVERTED: DecodeData appends to the accumulator and checks
// bodyBufferCapBytes BEFORE the HasGlobalFunc(proxyOnRequestBody) dispatch
// gate, so (n) returns at the cap arm and never reaches the ProxyAction switch
// at all. (n) depends on the arm NEVER BEING REACHED. The entire stated basis
// for freezing body.go and trailers.go was void, and the freeze cost FOUR
// unbounded parks — measured at phase-83 as RunDecodeData, RunEncodeData,
// RunDecodeTrailers and RunEncodeTrailers each failing to return in 10 s.
//
// ⚠️ THE PHASE-82 MITIGATION FOR THAT FREEZE WAS ALSO FALSE. It read "ZERO of
// the 35 guest crates in this repository reference resume_http_request /
// resume_http_response / proxy_continue_stream, so no guest can reach that
// path today." Re-derived over all 35 `.rs` crates at this tip (denominator
// printed, 35/35): exactly ONE does. The scenario-(l) crate under
// test/fixtures/0036-http-wasm-body-and-advanced/scripts/l_httpcall_success
// calls `self.resume_http_request()` from its on_http_call_response, and it
// was added by the same commit that wrote the claim. A silently dropped resume
// was reachable from the shipped corpus, not merely in theory.
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

// defaultPauseWatchdog bounds how long a guest-requested pause — from ANY of
// the six arms above, not just the headers pair — may hold the stream parked
// before the host force-resumes it.
//
// ⚠️ THE VALUE STAYS; THE PHASE-82 DERIVATION IS REMOVED, BECAUSE BOTH OF ITS
// "MEASURED CONSTRAINTS" FALL. Presenting 10 s as pinned between two bounds is
// the defect being corrected here, not the number itself.
//
//	The stated LOWER BOUND was "internal/wasm.defaultHttpCallTimeout is 5 s,
//	and that is the deadline on the outbound call a paused guest is normally
//	waiting for." It does not bind. internal/wasm/http_call.go seeds the
//	per-call timeout with that 5 s default and then OVERWRITES it whenever the
//	guest passed timeout_ms > 0, so the default survives only for a guest that
//	passed 0 — and the one corpus guest that pauses behind an http call (0036
//	scenario (l)) passes Duration::from_secs(5) EXPLICITLY. Nothing on that
//	path caps a guest-supplied timeout, so a guest may ask for 60 s and no
//	watchdog value derived from 5 s can "give the call its full deadline".
//
//	The stated UPPER BOUND was that 0036 (l) "NEVER calls resume_http_request
//	from its on_http_call_response, so with Pause honored its probe is released
//	only by this watchdog." That is false about the guest: l_httpcall_success's
//	on_http_call_response calls self.resume_http_request() with a comment
//	saying why. The probe is released BY THE GUEST, so the 15 s http.Client
//	timeout in that fixture's driver — which the phase-82 text compared 10 s
//	against — never enters the picture at all. The same collapse takes the
//	sentence that followed it: (l)'s result is NOT discarded (the driver emits
//	it through emitScenario; the `_ =` arms are (k), (m) and (n)), (l) IS
//	cross-side as of phase 82, and its driver asserts http_call_response >= 1
//	AND http_call_response_after_close == 0 — so "not a verdict change" would
//	no longer follow even if the premise held.
//
// WHAT IS ACTUALLY TRUE: any value in roughly [1 s, 90 s) is
// INDISTINGUISHABLE to the current corpus. The corpus arms this watchdog
// exactly ONCE — 0036 (l) — and the guest releases it immediately (measured at
// phase-83: 747 µs), so the phase-82 prediction of "~10 s of added
// subject-side runtime on that one fixture" measures 0 s. Every other corpus
// Pause is short-circuited before it reaches a begin*Pause call: 0034 (d)
// captures a local response first, 0036 (n)'s body pause is preempted by the
// byte cap, and 0036 (a)/(b)/(c) return Pause only while !end_of_stream.
//
// 10 s is chosen INSIDE that indifference band, for two reasons — neither of
// which is a bound:
//
//   - it comfortably exceeds the 5 s defaultHttpCallTimeout CONVENTION, so a
//     guest that accepts the host default is not truncated; and
//   - it stays well inside the differential's 90 s PER-FIXTURE budget (the
//     context.WithTimeout at the head of runFixture in
//     test/differential/runner_test.go — NOT the 15 s client timeout the
//     phase-82 comment cited), so even a future fixture that did rely on the
//     watchdog could not push its own fixture over deadline.
//
// ⚠️ Do not restate this as a pinned interval. Every reference to this
// constant is non-test — no test in this tree asserts its magnitude, and
// per-stream overrides go through the pauseWatchdog field instead — so a bound
// claimed for it could not be falsified by anything here. (No count is quoted
// on purpose: a sentence that names the identifier changes its own grep
// result.)
const defaultPauseWatchdog = 10 * time.Second

// watchdogTimeout resolves the per-stream override (0 ⇒ default).
func (f *filter) watchdogTimeout() time.Duration {
	if f.pauseWatchdog > 0 {
		return f.pauseWatchdog
	}
	return defaultPauseWatchdog
}

// beginDecodePause marks the decode side parked and arms the S9 watchdog.
//
// Callers, re-derived at this tip rather than inherited: DecodeHeaders (before
// envoyhttp.StopIteration) and DecodeTrailers (before
// envoyhttp.TrailersStopIteration) call it DIRECTLY; DecodeData reaches it
// through beginDecodePauseOnce. The phase-82 text named only DecodeHeaders,
// which phase-83 S1 and S2 falsified.
//
// # Generations (S3)
//
// Every pause takes a fresh generation and the watchdog closure captures it.
// A closure whose generation is no longer current belongs to a SUPERSEDED
// pause and must not fire: without the check it wins the CAS against a LATER
// pause's flag and calls ContinueDecoding for a pause it does not own, and
// because the chain's resume channel is buffered-1 that unmatched token
// spuriously un-parks a different filter in the same stream. Measured over 300
// superseded generations at watchdog 100 ms / pace 1 ms: 178 spurious fires
// without the guard, 1 with it.
//
// ⚠️ THE GENERATION CHECK MUST PRECEDE THE CAS — this is a correctness
// constraint, not style. CAS-first is WORSE than the bug it fixes: a
// superseded closure consumes the CURRENT pause's flag and only THEN bails on
// the generation check, so the current pause is left with no resumer at all,
// the guest's proxy_continue_stream loses its own CAS, and the chain parks
// FOREVER. Measured CAS-first: 0 fires and 299 LOST RESUMES. It converts a
// spurious resume into the unbounded park this bookkeeping exists to close.
//
// ⚠️ AND THE Add(1) MUST PRECEDE THE Store(true), so no window exists in which
// the flag is set under a generation a pending closure still considers current.
//
// Stop() on the displaced handle is RESOURCE HYGIENE, not correctness: measured
// across seven pacer configurations it changes the fire count by ZERO (the
// guard already rejects every superseded closure). What it does buy is that a
// superseded closure stops retaining the chain's *decoderCb — over 200
// back-to-back pauses, 0 pending closures with Stop() versus 199 without.
//
// # cbName
//
// cbName is the guest export the pause came FROM, and it appears verbatim in
// the watchdog's operator-facing warning. It is a parameter rather than a
// constant because phase-83 S1 and S2 gave this function callers on the BODY
// and TRAILERS arms: the previously hard-coded "proxy_on_request_headers" was
// captured for a headers pause and would have been printed for a body or
// trailers pause too — an operator chasing the wrong guest callback. Nothing
// in this package asserts any log string, so this correction ships UNGATED.
func (f *filter) beginDecodePause(cbName string) {
	gen := f.decodePauseGen.Add(1) // FIRST — before Store(true)
	f.decodePaused.Store(true)
	cb := f.decoderCb
	if cb == nil {
		// No framework callback ⇒ nothing parks (test-double paths that
		// construct *filter{} directly). Leave the flag set so a later
		// ContinueStream still reports the transition; nothing to arm.
		return
	}
	t := time.AfterFunc(f.watchdogTimeout(), func() {
		if f.decodePauseGen.Load() != gen {
			return // superseded by a later pause OR by a disarm
		}
		if !f.decodePaused.CompareAndSwap(true, false) {
			return // already resumed by the guest; the token is not ours to fire
		}
		logf("WARN wasm: decode-side pause watchdog fired (stream=%d) — the guest returned PAUSE from %s and never called proxy_continue_stream; force-resuming the stream to avoid a parked-connection leak",
			f.streamContextID, cbName)
		cb.ContinueDecoding()
	})
	f.pauseMu.Lock()
	old := f.decodePauseTimer
	f.decodePauseTimer = t
	f.pauseMu.Unlock()
	if old != nil {
		old.Stop() // return value IGNORED — it is not a signal
	}
}

// beginEncodePause is the encode-side mirror of beginDecodePause. Callers:
// EncodeHeaders and EncodeTrailers directly, EncodeData via
// beginEncodePauseOnce.
//
// UNEXERCISED by any guest in this repository: zero of the 35 guest crates
// return Action::Pause from proxy_on_response_headers, so this half has NO
// differential coverage. Re-verified at this tip, and the same holds for the
// two arms phase-83 added: no crate in the corpus even DECLARES
// on_http_response_body or on_http_response_trailers, so the encode side has
// no guest-driven Pause of any kind. It is exercised only by the synthetic
// unit fixtures buildPauseResponseProxyWasm / buildPauseResponseBodyProxyWasm
// / buildPauseResponseTrailersProxyWasm (wasm_fixtures_test.go).
func (f *filter) beginEncodePause(cbName string) {
	gen := f.encodePauseGen.Add(1) // FIRST — before Store(true)
	f.encodePaused.Store(true)
	cb := f.encoderCb
	if cb == nil {
		return
	}
	t := time.AfterFunc(f.watchdogTimeout(), func() {
		if f.encodePauseGen.Load() != gen {
			return // superseded by a later pause OR by a disarm
		}
		if !f.encodePaused.CompareAndSwap(true, false) {
			return
		}
		logf("WARN wasm: encode-side pause watchdog fired (stream=%d) — the guest returned PAUSE from %s and never called proxy_continue_stream; force-resuming the stream to avoid a parked-connection leak",
			f.streamContextID, cbName)
		cb.ContinueEncoding()
	})
	f.pauseMu.Lock()
	old := f.encodePauseTimer
	f.encodePauseTimer = t
	f.pauseMu.Unlock()
	if old != nil {
		old.Stop() // return value IGNORED — it is not a signal
	}
}

// beginDecodePauseOnce arms the decode side only if it is not already paused.
// It is what the BODY arms call, and the distinction from beginDecodePause is
// NOT cosmetic.
//
// A headers callback fires once per stream, so its pause can only ever be the
// first. A body callback fires once per CHUNK and may return Pause on every
// one of them. beginDecodePause installs a FRESH time.AfterFunc and bumps the
// generation on every call, so a per-chunk pause would (a) push the watchdog
// deadline out by a full window on each chunk — the bound is never reached,
// which is the very leak this bookkeeping exists to close — and (b) supersede
// the closure that was going to do the bounding.
//
// The window is real: beginDecodePause returns EARLY, leaving the flag set and
// NO timer armed, whenever decoderCb is nil. In that shape nothing ever clears
// the flag, so every subsequent chunk sees decodePaused already true.
func (f *filter) beginDecodePauseOnce(cbName string) {
	if f.decodePaused.Load() {
		return // already parked on this side; the armed watchdog still owns it
	}
	f.beginDecodePause(cbName)
}

// beginEncodePauseOnce is the encode-side mirror.
func (f *filter) beginEncodePauseOnce(cbName string) {
	if f.encodePaused.Load() {
		return
	}
	f.beginEncodePause(cbName)
}

// disarmDecodePause releases a decode-side pause that this filter is NOT going
// to park on after all.
//
// Every body-dispatch arm that returns DataContinue RELEASES the chain —
// FilterChain.RunDecodeData advances its cursor to the next filter — and that
// includes the ADR-0072 fail-OPEN arm, not only the guest's Continue. Leaving
// the flag set and the watchdog armed past that point is a live spurious-
// resume source: the watchdog fires into a stream that is no longer parked and
// latches an unmatched token on the chain's buffered-1 resume channel, which
// then un-parks some OTHER filter in the same stream.
//
// stopPauseTimer is what makes the disarm authoritative — it bumps the
// generation, so a closure that has ALREADY ENTERED is superseded and returns
// before its CAS. Timer.Stop() alone cannot cancel one (measured 400/400 late
// fires with the handle-only disarm, 0/400 with the bump).
//
// The early return is a cost guard on the overwhelmingly common case — a guest
// that never paused — and it is safe: a live handle cannot outlive the flag
// except when the watchdog already fired and consumed it, and there is nothing
// left to cancel in that case.
func (f *filter) disarmDecodePause() {
	if !f.decodePaused.Load() {
		return
	}
	f.decodePaused.Store(false)
	f.stopPauseTimer(true)
}

// disarmEncodePause is the encode-side mirror of disarmDecodePause.
func (f *filter) disarmEncodePause() {
	if !f.encodePaused.Load() {
		return
	}
	f.encodePaused.Store(false)
	f.stopPauseTimer(false)
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
// call when no timer was armed, and Stop's return value is ignored.
//
// ⚠️ THE GENERATION BUMP IS THE DISARM (S9). Timer.Stop() CANNOT cancel a
// closure that has already entered, so the handle alone does not make this
// call authoritative. The pre-S9 claim that an already-fired watchdog "is
// harmless regardless, because its body re-checks the CAS" was FALSE: the flag
// is still true at that point, the CAS succeeds, and the closure calls
// ContinueDecoding into a stream that is being torn down — an unmatched resume
// latched into the chain's buffered-1 channel. Measured with the handle-only
// disarm, 400/400 trials fired late; with the generation bump, 0/400.
//
// The bump therefore sits AHEAD of the pauseMu acquisition, deliberately: an
// in-flight closure must be superseded the instant the disarm is REQUESTED,
// not whenever this call happens to win the lock.
func (f *filter) stopPauseTimer(decode bool) {
	if decode {
		f.decodePauseGen.Add(1) // make the disarm authoritative
	} else {
		f.encodePauseGen.Add(1)
	}
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
