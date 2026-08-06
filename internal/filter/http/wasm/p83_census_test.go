package wasm

// p83_census_test.go — phase-83 S4. The REPLACEMENT for
// TestFilter_Pause_CensusOfHonoredArms, which was deleted from pause_test.go
// by the commit that added this file.
//
// ⚠️ WHY REPLACED RATHER THAN REPAIRED — the old test contained TWO FALSE
// SENTENCES FIFTEEN LINES APART, and each was load-bearing:
//
//   1. "This is a behavioral assertion, not a grep: it drives the real
//      dispatch and reads the returned status." It did neither. Its whole body
//      was three comparisons of PACKAGE CONSTANTS
//      (DataStopIterationAndBuffer == DataContinue, and two siblings). It
//      constructed no *filter, no FilterChain and no guest, and dispatched
//      nothing.
//
//   2. "Asserting the CONSTANTS keeps this cheap and makes an accidental
//      change to the returned disposition a compile-or-assert failure at this
//      site." False on its own terms, and the falsity is visible without
//      running anything: changing which constant an arm RETURNS neither fails
//      to compile nor changes any relation BETWEEN the constants, so the
//      comparisons cannot observe it.
//
// PROVEN BY BREAK, NOT BY READING, at the pre-S1/S2 tip (1afc41cc, the tree
// the old test was written against):
//
//   flip body.go's decode Pause arm to DataContinue  → census PASS,
//                                                      package 445 PASS / 0 FAIL
//   flip ALL FOUR body+trailers arms simultaneously  → census PASS,
//                                                      package 445 PASS / 0 FAIL,
//                                                      `ok … 0.323-0.325s`
//   the same four-arm flip, widened                  → 26 packages ok / 0 FAIL,
//                                                      the FilterChain itself and
//                                                      test/conformance/proxy-wasm
//                                                      among them
//
// Which assertion fired under the four-arm break: NONE. That is broken-gate
// shape 25 — a gate whose own docstring claims it is behavioral when it is a
// tautology over constants — and it was the NAMED guard against exactly the
// edit phase-83 S1/S2 make.
//
// ⚠️ AND THE CONTRACT IS RE-INVERTED. The pre-S1/S2 replacement sketch
// asserted that decodePaused / encodePaused stay FALSE on the four "frozen"
// arms. S1 (body.go ×2) and S2 (trailers.go ×2) INVERT that: all SIX
// abi.ProxyActionPause arms in this package now set their side's flag and arm
// the S9 watchdog. A version kept from the pre-S1/S2 sketch would ship a
// gate that contradicts the code it guards.
//
// WHAT THIS FILE ASSERTS, per arm, on all six:
//
//   (a) the arm returns the PARKING status for its callback family;
//   (b) the arm's side flag transitions FALSE → TRUE across the call;
//   (c) the arm ARMS the watchdog (a non-nil *time.Timer handle) and bumps the
//       side's pause generation;
//   (d) the OTHER side is untouched, and no ContinueDecoding / ContinueEncoding
//       is signaled by the pause itself;
//   (e) envoy_go.failures stays 0 — so a status assertion that passes did so
//       because the guest PAUSED, not because the host fail-OPENed.
//
// (a) and (b)+(c) are separately load-bearing and neither subsumes the other:
// flipping the returned status leaves the flag and the timer correct, and
// dropping the begin*Pause call leaves the status correct. A roster with only
// one flavor leaves the other half of the contract unproven.

import (
	"context"
	gohttp "net/http"
	"testing"
	"time"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// p83CensusWatchdog is deliberately far longer than any test runtime. The
// census asserts that the watchdog was ARMED, never that it FIRED; a firing
// watchdog would clear the very flag (b) asserts and race every subtest.
const p83CensusWatchdog = time.Hour

// -----------------------------------------------------------------------------
// The six-callback guest.
// -----------------------------------------------------------------------------

// buildPauseAllSixArmsProxyWasm constructs the ONE module that reaches all six
// of this package's abi.ProxyActionPause arms.
//
// ⚠️ proxy_on_request_headers is the only callback that is NOT a constant
// Pause, and that is structural rather than stylistic. The per-stream
// StreamContext is constructed ONLY in DecodeHeaders (decode_headers.go's
// `if f.streamCtx == nil` guard), and body.go / trailers.go / encode_headers.go
// all short-circuit to their Continue disposition on a nil f.streamCtx. Every
// arm therefore needs a request-headers leg that CONTINUES before it can be
// reached at all — while the decode-headers arm itself needs one that PAUSES.
// `1 - end_of_stream` (fixPauseUntilEndOfStreamBody, param 2 of the
// (context_id, num_headers, end_of_stream) signature) gives both from a single
// export: endStream=true primes, endStream=false drives the arm.
//
// ⚠️ THE TRAILERS PAIR USES A DIFFERENT TYPE ENTRY, AND GETTING IT WRONG IS
// SILENT. CallProxyOnRequestTrailers / CallProxyOnResponseTrailers pass
// exactly (streamCtxID, numTrailers) — (i32,i32) -> i32, fixTypeTrailersCallback
// — where every other callback here is (i32,i32,i32) -> i32. A module that
// declares the type-1 signature for them INSTANTIATES FINE and fails only at
// call time inside wazero ("expected 3 params, but passed 2"), whereupon
// trailers.go fail-OPENs and returns TrailersContinue. That is why every arm
// below asserts envoy_go.failures == 0 alongside its status: without it, a
// fail-OPEN and a genuinely unhonored Pause are indistinguishable.
func buildPauseAllSixArmsProxyWasm() []byte {
	return fixBuildModule(
		fixTypeSection(
			[2][]byte{nil, nil}, // type 0: () -> ()
			[2][]byte{{fixTypeI32, fixTypeI32, fixTypeI32}, {fixTypeI32}}, // type 1: (i32,i32,i32) -> i32
			fixTypeTrailersCallback(),                                     // type 2: (i32,i32) -> i32
		),
		// fn 0: _initialize                 (type 0)
		// fn 1: proxy_abi_version_0_2_1     (type 0)
		// fn 2: proxy_on_request_headers    (type 1) → 1 - end_of_stream
		// fn 3: proxy_on_response_headers   (type 1) → Pause
		// fn 4: proxy_on_request_body       (type 1) → Pause
		// fn 5: proxy_on_response_body      (type 1) → Pause
		// fn 6: proxy_on_request_trailers   (type 2) → Pause
		// fn 7: proxy_on_response_trailers  (type 2) → Pause
		fixFunctionSection([]uint32{0, 0, 1, 1, 1, 1, 2, 2}),
		fixMemorySection(1),
		fixExportSection([]fixExport{
			{name: "_initialize", kind: fixExtFunction, idx: 0},
			{name: "proxy_abi_version_0_2_1", kind: fixExtFunction, idx: 1},
			{name: "proxy_on_request_headers", kind: fixExtFunction, idx: 2},
			{name: "proxy_on_response_headers", kind: fixExtFunction, idx: 3},
			{name: "proxy_on_request_body", kind: fixExtFunction, idx: 4},
			{name: "proxy_on_response_body", kind: fixExtFunction, idx: 5},
			{name: "proxy_on_request_trailers", kind: fixExtFunction, idx: 6},
			{name: "proxy_on_response_trailers", kind: fixExtFunction, idx: 7},
			{name: "memory", kind: fixExtMemory, idx: 0},
		}),
		fixCodeSection([][]byte{
			fixFuncBody(),
			fixFuncBody(),
			fixPauseUntilEndOfStreamBody(), // request headers: Pause unless end_of_stream
			fixFuncBody(fixI32Const(1)),    // response headers  → ProxyActionPause
			fixFuncBody(fixI32Const(1)),    // request body      → ProxyActionPause
			fixFuncBody(fixI32Const(1)),    // response body     → ProxyActionPause
			fixFuncBody(fixI32Const(1)),    // request trailers  → ProxyActionPause
			fixFuncBody(fixI32Const(1)),    // response trailers → ProxyActionPause
		}),
	)
}

// -----------------------------------------------------------------------------
// Per-arm setup.
// -----------------------------------------------------------------------------

// p83CensusFilter builds a *filter over the six-callback guest with BOTH
// framework callback doubles installed, and primes it by running the decode-
// headers leg to a Continue.
//
// ⚠️ BOTH DOUBLES ARE LOAD-BEARING, NOT DECORATION. beginDecodePause /
// beginEncodePause return EARLY — flag set, NO timer armed — when their side's
// framework callback is nil (pause.go's `cb == nil` guard). A census built on
// a filter with a nil encoderCb would assert (c) against a shape that CANNOT
// arm, and the encode arms would read as broken on a correct tree.
//
// ⚠️ THE LIVENESS BARRIER IS HasGlobalFunc, NOT THE `executions` COUNTER.
// decode_headers.go bumps stats.executions BEFORE CallProxyOnRequestHeaders,
// and the StrictDefaultDeny capability gate lives INSIDE that call, in
// StreamContext.dispatchGuest — `executions == 1` is measurable with EVERY
// capability denied, so that counter is green on exactly the failure a barrier
// exists to catch (broken-gate shape 26). RootVM.HasGlobalFunc applies the
// AMEND-B5 gate-at-getFunction discipline, which for the seven NEW 25.2
// callbacks — both trailers among them — makes it a DIRECT capability
// detector, and for the rest an export-presence barrier.
//
// It is a t.Fatalf and not a t.Errorf deliberately: it is a PRECONDITION, not
// a property of the arm. Every assertion the caller then makes is vacuous if
// the guest was never reachable, so continuing past it would manufacture a
// green rather than report one more failure.
func p83CensusFilter(t *testing.T, pluginName, wantFunc string) (*filter, *countingDecoderCb, *countingEncoderCb, *stats.Registry) {
	t.Helper()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfigWithCaps(t, buildPauseAllSixArmsProxyWasm(), pluginName, reg)
	t.Cleanup(func() { _ = cc.Close() })

	wf := &filter{cfg: cc}
	wf.pauseWatchdog = p83CensusWatchdog
	dcb := &countingDecoderCb{}
	ecb := &countingEncoderCb{}
	wf.SetDecoderCallbacks(dcb)
	wf.SetEncoderCallbacks(ecb)
	// Force-release: OnDestroy → stopPauseWatchdogs disarms both sides. Every
	// subtest below leaves a watchdog ARMED by construction, and a p83Census
	// Watchdog-long timer outliving the test would hold the filter (and its
	// closure over the callback doubles) for the rest of the package run.
	t.Cleanup(wf.OnDestroy)

	h := gohttp.Header{}
	h.Set(":method", "POST")
	// endStream=true ⇒ the guest's `1 - end_of_stream` returns Continue. This
	// leg exists to BUILD the StreamContext, not to test anything.
	if got := wf.DecodeHeaders(h, true); got != envoyhttp.Continue {
		t.Fatalf("priming DecodeHeaders(endStream=true) = %v; want Continue — the guest's `1 - end_of_stream` request-headers callback must Continue here or no arm below is reachable", got)
	}
	if wf.streamCtx == nil {
		t.Fatalf("streamCtx is nil after the priming DecodeHeaders; the per-stream context was never built and every dispatch below would short-circuit to its Continue disposition")
	}
	if !wf.streamCtx.HasGlobalFunc(wantFunc) {
		t.Fatalf("HasGlobalFunc(%q) = false; the guest does not export the callback under test (or its capability is denied), and the arm would return its Continue disposition without ever reaching the ProxyAction switch", wantFunc)
	}
	return wf, dcb, ecb, reg
}

// p83CensusTimer reads a side's watchdog handle under pauseMu, which guards
// both handles (wasm.go's pauseMu comment).
func p83CensusTimer(wf *filter, encodeSide bool) *time.Timer {
	wf.pauseMu.Lock()
	defer wf.pauseMu.Unlock()
	if encodeSide {
		return wf.encodePauseTimer
	}
	return wf.decodePauseTimer
}

// p83CensusTrailers returns a two-entry trailer map. Two entries rather than
// one so numTrailers is not incidentally a value that a "never called" path
// could also produce.
func p83CensusTrailers() gohttp.Header {
	h := gohttp.Header{}
	h.Set("Grpc-Status", "0")
	h.Set("X-P83-Census", "v")
	return h
}

// p83CensusArm is one of the six abi.ProxyActionPause arms.
//
// invoke drives the arm and asserts (a), the returned STATUS, inside the row —
// the three callback families return three DIFFERENT Go types
// (FilterHeadersStatus / FilterDataStatus / FilterTrailersStatus), and a table
// that flattened them to a string or an int would compare the wrong thing
// while still looking like a comparison.
type p83CensusArm struct {
	name       string // subtest name AND plugin-name suffix; ASCII + `_` only
	cbName     string // the guest export this arm dispatches (liveness barrier)
	encodeSide bool   // which side's flag / timer / generation the arm owes
	invoke     func(t *testing.T, wf *filter)
}

func p83CensusArms() []p83CensusArm {
	return []p83CensusArm{
		{
			name:   "decode_headers",
			cbName: proxyOnRequestHeaders,
			invoke: func(t *testing.T, wf *filter) {
				t.Helper()
				h := gohttp.Header{}
				h.Set(":method", "POST")
				// endStream=false ⇒ `1 - end_of_stream` = 1 = ProxyActionPause.
				// The StreamContext already exists (built by the priming leg),
				// so this second entry re-dispatches without re-constructing it.
				if got := wf.DecodeHeaders(h, false); got != envoyhttp.StopIteration {
					t.Errorf("DecodeHeaders status = %v; want StopIteration — the decode-headers Pause arm must PARK the chain", got)
				}
			},
		},
		{
			name:       "encode_headers",
			cbName:     proxyOnResponseHeaders,
			encodeSide: true,
			invoke: func(t *testing.T, wf *filter) {
				t.Helper()
				h := gohttp.Header{}
				h.Set("content-type", "text/plain")
				if got := wf.EncodeHeaders(h, false); got != envoyhttp.StopIteration {
					t.Errorf("EncodeHeaders status = %v; want StopIteration — the encode-headers Pause arm must PARK the chain", got)
				}
			},
		},
		{
			name:   "decode_body",
			cbName: proxyOnRequestBody,
			invoke: func(t *testing.T, wf *filter) {
				t.Helper()
				if got := wf.DecodeData([]byte("p83"), false); got != envoyhttp.DataStopIterationAndBuffer {
					t.Errorf("DecodeData status = %v; want DataStopIterationAndBuffer — the decode-body Pause arm must PARK the chain", got)
				}
			},
		},
		{
			name:       "encode_body",
			cbName:     proxyOnResponseBody,
			encodeSide: true,
			invoke: func(t *testing.T, wf *filter) {
				t.Helper()
				if got := wf.EncodeData([]byte("p83"), false); got != envoyhttp.DataStopIterationAndBuffer {
					t.Errorf("EncodeData status = %v; want DataStopIterationAndBuffer — the encode-body Pause arm must PARK the chain", got)
				}
			},
		},
		{
			name:   "decode_trailers",
			cbName: proxyOnRequestTrailers,
			invoke: func(t *testing.T, wf *filter) {
				t.Helper()
				if got := wf.DecodeTrailers(p83CensusTrailers()); got != envoyhttp.TrailersStopIteration {
					t.Errorf("DecodeTrailers status = %v; want TrailersStopIteration — the decode-trailers Pause arm must PARK the chain", got)
				}
			},
		},
		{
			name:       "encode_trailers",
			cbName:     proxyOnResponseTrailers,
			encodeSide: true,
			invoke: func(t *testing.T, wf *filter) {
				t.Helper()
				if got := wf.EncodeTrailers(p83CensusTrailers()); got != envoyhttp.TrailersStopIteration {
					t.Errorf("EncodeTrailers status = %v; want TrailersStopIteration — the encode-trailers Pause arm must PARK the chain", got)
				}
			},
		},
	}
}

// -----------------------------------------------------------------------------
// The census.
// -----------------------------------------------------------------------------

// TestFilter_Pause_CensusOfHonoredArms_Behavioral is the S4 replacement. It
// drives each of the six abi.ProxyActionPause arms through the real dispatch
// against a real guest and reads the real returned status — the thing the
// deleted test's docstring claimed and its body did not do.
//
// Every property is a t.Errorf rather than a t.Fatalf so that a single run
// reports EVERY property an arm broke. A t.Fatalf on (a) would make (b)-(e)
// dead code, which is precisely how a status-only break would read as "one
// failure" when it is one failure out of five checks.
//
// SCOPE, STATED HONESTLY: this drives the filter methods DIRECTLY. It asserts
// the filter-local contract — status, flag, timer, generation — and NOT that
// FilterChain actually parks and is actually released. That is chain-level
// coverage and it lives in pause_body_test.go and trailers_test.go, which
// drive real FilterChain.Run* calls and measure the release against the
// watchdog. This file is the per-arm census; those are the park anchors.
func TestFilter_Pause_CensusOfHonoredArms_Behavioral(t *testing.T) {
	t.Parallel()

	for _, arm := range p83CensusArms() {
		arm := arm
		t.Run(arm.name, func(t *testing.T) {
			t.Parallel()

			plugin := "p83_census_" + arm.name
			wf, dcb, ecb, reg := p83CensusFilter(t, plugin, arm.cbName)

			flag, gen := &wf.decodePaused, &wf.decodePauseGen
			otherFlag := &wf.encodePaused
			otherSide := "encode"
			side := "decode"
			if arm.encodeSide {
				flag, gen = &wf.encodePaused, &wf.encodePauseGen
				otherFlag = &wf.decodePaused
				otherSide, side = "decode", "encode"
			}

			// PRE-STATE. Without this the post-state `true` could be a
			// leftover from the priming leg rather than a transition caused
			// by the arm under test.
			if flag.Load() {
				t.Errorf("%sPaused = true BEFORE the arm ran; the post-state assertion below could not distinguish a transition from a leftover", side)
			}
			if got := p83CensusTimer(wf, arm.encodeSide); got != nil {
				t.Errorf("%sPauseTimer = %p BEFORE the arm ran; want nil", side, got)
			}
			genBefore := gen.Load()

			// (a) — the returned status, asserted inside the row.
			arm.invoke(t, wf)

			// (b) — the side flag transitioned to true.
			if !flag.Load() {
				t.Errorf("%sPaused = false after the Pause arm; want true — the filter owes the chain exactly one resume, and without the flag the guest's own proxy_continue_stream loses resume%s's CompareAndSwap and fires NOTHING while still returning WasmResult::Ok", side, side)
			}

			// (c) — the S9 watchdog was armed, and the generation advanced.
			// Both, because they fail independently: a begin*Pause that
			// returned early on a nil callback bumps the generation and arms
			// nothing.
			if got := p83CensusTimer(wf, arm.encodeSide); got == nil {
				t.Errorf("%sPauseTimer = nil after the Pause arm; want a live handle — with no watchdog the park is bounded by NOTHING (there is no stream idle timeout anywhere in this tree)", side)
			}
			if got := gen.Load(); got != genBefore+1 {
				t.Errorf("%sPauseGen = %d after the Pause arm; want %d — the generation is what makes a later disarm authoritative against an already-entered watchdog closure", side, got, genBefore+1)
			}

			// (d) — the other side is untouched, and the pause itself signaled
			// no resume.
			if otherFlag.Load() {
				t.Errorf("%sPaused = true after a %s-side pause; want false — the two sides are independent", otherSide, side)
			}
			if got := dcb.continues.Load(); got != 0 {
				t.Errorf("ContinueDecoding calls = %d after the Pause arm; want 0 — a pause must not resume itself", got)
			}
			if got := ecb.continues.Load(); got != 0 {
				t.Errorf("ContinueEncoding calls = %d after the Pause arm; want 0 — a pause must not resume itself", got)
			}

			// (e) — the dispatch reached the guest. A wrong type-section entry
			// (the trailers pair is (i32,i32), unlike every other callback
			// here) INSTANTIATES FINE and fails at CALL time, whereupon the
			// host fail-OPENs to the Continue disposition. Without this, a
			// failing (a) could not distinguish "the arm does not honor Pause"
			// from "the guest could not be called".
			if got := findStatCounterValue(reg, "wasm."+plugin+".envoy_go.failures"); got != 0 {
				t.Errorf("envoy_go.failures = %d; want 0 — a non-zero value means the dispatch errored and the host FAIL-OPENed, so the status above says nothing about the Pause arm", got)
			}
		})
	}
}

// TestFilter_Pause_Census_FixtureLiveness asserts the six-callback module
// really returns ProxyActionPause, with a nil error, on all six callbacks —
// measured through StreamContext directly rather than through the filter.
//
// ⚠️ IT IS THE CONTROL FOR THE CENSUS ABOVE, AND IT IS NOT REDUNDANT WITH IT.
// If a break makes a census subtest fail, this test says whether the guest
// still paused. A guest that stopped pausing (a broken fixture) and a filter
// that stopped honoring the pause (the defect) produce the SAME census
// failure; only this test separates them. It also pins the request-headers
// callback's `1 - end_of_stream` discrimination, which the priming leg and the
// decode-headers arm depend on in opposite directions.
func TestFilter_Pause_Census_FixtureLiveness(t *testing.T) {
	t.Parallel()

	wf, _, _, _ := p83CensusFilter(t, "p83_census_fixlive", proxyOnRequestHeaders)
	ctx := context.Background()

	// The request-headers callback must DISCRIMINATE on end_of_stream: the
	// priming leg needs the Continue, every arm's dispatch needs the Pause.
	if act, err := wf.streamCtx.CallProxyOnRequestHeaders(ctx, 2, true); err != nil || act != abi.ProxyActionContinue {
		t.Errorf("CallProxyOnRequestHeaders(endStream=true) = (%v, %v); want (ProxyActionContinue, nil)", act, err)
	}
	if act, err := wf.streamCtx.CallProxyOnRequestHeaders(ctx, 2, false); err != nil || act != abi.ProxyActionPause {
		t.Errorf("CallProxyOnRequestHeaders(endStream=false) = (%v, %v); want (ProxyActionPause, nil)", act, err)
	}
	if act, err := wf.streamCtx.CallProxyOnResponseHeaders(ctx, 2, false); err != nil || act != abi.ProxyActionPause {
		t.Errorf("CallProxyOnResponseHeaders = (%v, %v); want (ProxyActionPause, nil)", act, err)
	}
	if act, err := wf.streamCtx.CallProxyOnRequestBody(ctx, 3, false); err != nil || act != abi.ProxyActionPause {
		t.Errorf("CallProxyOnRequestBody = (%v, %v); want (ProxyActionPause, nil)", act, err)
	}
	if act, err := wf.streamCtx.CallProxyOnResponseBody(ctx, 3, false); err != nil || act != abi.ProxyActionPause {
		t.Errorf("CallProxyOnResponseBody = (%v, %v); want (ProxyActionPause, nil)", act, err)
	}
	// ⚠️ The two below are the (i32,i32) type-2 entries. A nil error here is
	// what proves the type section is right; a wrong one reports
	// "expected 3 params, but passed 2" at CALL time only.
	if act, err := wf.streamCtx.CallProxyOnRequestTrailers(ctx, 2); err != nil || act != abi.ProxyActionPause {
		t.Errorf("CallProxyOnRequestTrailers = (%v, %v); want (ProxyActionPause, nil)", act, err)
	}
	if act, err := wf.streamCtx.CallProxyOnResponseTrailers(ctx, 2); err != nil || act != abi.ProxyActionPause {
		t.Errorf("CallProxyOnResponseTrailers = (%v, %v); want (ProxyActionPause, nil)", act, err)
	}
}
